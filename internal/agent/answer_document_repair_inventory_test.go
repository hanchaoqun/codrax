package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAnswerRepairInventoryPreservesOpaqueIDsAndPayloads(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "summary-1", Kind: types.BlockTable, Columns: []string{"Value"}, Items: []types.AnswerBlockItem{{Text: "23"}, {Text: "47"}}},
		{ID: "table-1", Kind: types.BlockBulletList, Items: []types.AnswerBlockItem{{Text: "retained"}}},
		{ID: "list-1", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Language: "mermaid", Kind: "flow", Body: "flowchart LR\n A --> B"}},
	}}
	before, _ := json.Marshal(doc)
	view := &types.AnswerSemanticView{RequiredBlocks: []types.BlockRequirement{{Kind: types.BlockSummary, Required: true, MinCount: 1, MaxCount: 1}}}
	hint := answerDocPatchContentPreservationHint(doc, view)
	for _, want := range []string{`"id":"summary-1","items":2`, `"id":"table-1","items":1`, `"id":"list-1","diagram_bytes":`, "add_blocks with new ids", "Block ids are opaque", "not proof of correctness"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("missing %q: %s", want, hint)
		}
	}
	if strings.Contains(hint, "retained\"") || strings.Contains(hint, "A --> B") {
		t.Fatal("inventory repeated payload instead of counts")
	}
	// Use the public mutation API: additions inherit every untouched carrier;
	// unrelated invalid IDs fail atomically; explicit legitimate removals and
	// whole-block replacements still work. No new blanket preservation gate.
	added, err := types.ApplyAnswerDocumentV2Patch(doc, &types.AnswerDocumentV2Patch{AddBlocks: []types.AnswerBlock{{ID: "new-lead", Kind: types.BlockSummary, Text: "Summary"}}})
	if err != nil {
		t.Fatal(err)
	}
	for i, block := range doc.Blocks {
		if !reflect.DeepEqual(block, added.Blocks[i]) {
			t.Fatal("add lost inherited payload")
		}
	}
	if got := answerDocPatchContentPreservationHint(added, view); got != "" {
		t.Fatalf("resolved deficit kept guidance: %s", got)
	}
	if _, err := types.ApplyAnswerDocumentV2Patch(doc, &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"not-present"}}); err == nil {
		t.Fatal("invalid patch accepted")
	}
	after, _ := json.Marshal(doc)
	if string(before) != string(after) {
		t.Fatal("guidance/failed patch mutated base")
	}
	replaced, err := types.ApplyAnswerDocumentV2Patch(added, &types.AnswerDocumentV2Patch{RemoveBlockIDs: []string{"list-1"}, ReplaceBlocks: []types.AnswerBlock{{ID: "table-1", Kind: types.BlockSection, Text: "deliberate rewrite"}}})
	if err != nil || len(replaced.Blocks) != 3 {
		t.Fatalf("legitimate model edit prohibited: %v", err)
	}
}

type countRepairMessageCapture struct {
	traceTeachingCaptureLLM
	round int
}

func (l *countRepairMessageCapture) Chat(ctx context.Context, messages []llm.Message, schemas []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	l.round++
	if l.round == 1 {
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "invalid-patch", Name: "emit_answer_document_patch", Params: json.RawMessage(`{"unchanged_block_ids":["not-present"]}`)}}}, nil
	}
	return l.traceTeachingCaptureLLM.Chat(ctx, messages, schemas, opts)
}

func TestAnswerRepairActualFinalizerRequestAfterRejectedPatch(t *testing.T) {
	mut := types.NewMutableState("show records")
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "summary-1", Kind: types.BlockTable, Items: []types.AnswerBlockItem{{Text: "kept"}}}}}
	mut.SetLastRejectedAnswerDocumentV2(doc)
	ctx := &types.AgentContext{Stage: types.StageFinalize, Mutable: mut, AnalysisIR: &types.AnalysisIR{}}
	reg := tool.NewRegistry()
	reg.Register(&tool.EmitAnswerDocument{})
	reg.Register(&tool.EmitAnswerDocumentPatch{})
	adapter := &countRepairMessageCapture{traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: errors.New("captured actual repair request")}}
	finalizer := NewFinalizerAgent(&Dependencies{LLM: adapter, Tools: reg, MaxIterations: 3})
	_, err := finalizer.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
	if !errors.Is(err, adapter.stop) || adapter.round != 2 {
		t.Fatalf("expected actual second request, round=%d err=%v", adapter.round, err)
	}
	var delivered strings.Builder
	for _, m := range adapter.messages {
		delivered.WriteString(m.Content)
	}
	for _, want := range []string{`"id":"summary-1","kind":"table"`, `"id":"summary-1","items":1`, "add_blocks with new ids"} {
		if !strings.Contains(delivered.String(), want) {
			t.Fatalf("actual adapter lacks %s", want)
		}
	}
	if !reflect.DeepEqual(mut.LastRejectedAnswerDocumentV2(), doc) || mut.AnswerDocumentV2() != nil {
		t.Fatal("failed patch changed original or accepted a document")
	}
}

func TestAnswerRepairInventoryRecomputedAfterPatchIDFailure(t *testing.T) {
	mut := types.NewMutableState("show records")
	mut.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "summary-1", Kind: types.BlockTable, Text: "data"}}})
	ctx := &types.AgentContext{Stage: types.StageFinalize, Mutable: mut, AnalysisIR: &types.AnalysisIR{}}
	e := &answerDocumentEvaluator{mu: mut}
	res := &types.ToolResult{ToolName: "emit_answer_document_patch", Success: false, Repair: &types.ToolRepair{Code: "patch_invalid_id"}}
	signal := e.emitPatchRejectFullRewriteSignal(ctx, LoopObservation{LastToolResult: res})
	if !signal.HintRequested || !strings.Contains(signal.Hint, "add_blocks with new ids") || !strings.Contains(signal.Hint, `"id":"summary-1","kind":"table"`) {
		t.Fatalf("lost live deficit after unrelated id error: %+v", signal)
	}
}

func TestAnswerRepairMechanicalDirectionUsesTypedCount(t *testing.T) {
	for _, tc := range []struct {
		actual int
		op     string
	}{{0, "add_blocks"}, {3, "reduce_blocks"}} {
		r := types.NewAnswerBlockCountRepair(types.BlockRequirement{Kind: types.BlockSummary, Required: true, MinCount: 1, MaxCount: 1}, tc.actual)
		sv := types.ScoredViolation{Kind: types.ViolBlockCoverageMissing, Detail: "opposite misleading prose kind=diagram", BlockCountRepair: r}
		row, ok := classifyViolationToStructuredFix(sv)
		if !ok || row.Action != tc.op || strings.Contains(row.Hint, "diagram") {
			t.Fatalf("typed direction lost: %+v", row)
		}
	}
}
