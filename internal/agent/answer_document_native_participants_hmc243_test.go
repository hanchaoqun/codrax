package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNativeParticipantsActualFinalizerHMC243(t *testing.T) {
	ctx := requestedThreadAgentContextHMC242(t, "target")
	rows := tool.RuntimeDiagramRelations(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)
	if len(rows) != 3 {
		t.Fatalf("native events: %+v", rows)
	}
	doc := transactionSupportedDiagram(rows, types.DiagramSequence)
	doc.Blocks[0].Text = "记录三次唤醒，不表示唤醒者造成了全部等待。"
	doc.Blocks[0].SurfaceRole = types.SurfacePrincipal
	doc.Blocks[0].TraceCausalClaimCaliber = types.TraceCausalClaimNoConclusion
	doc.Blocks[0].FacetIDs = []string{string(types.FacetObservedArtifactFact)}
	doc.Blocks[1].FacetIDs = []string{string(types.FacetDiagramSpine), string(types.FacetObservedArtifactFact)}
	b := &doc.Blocks[1]
	b.Diagram.Body += "\nparticipant T as 目标线程\nNote over T: 原始说明，不作新因果证明\n"
	b.Diagram.RuntimeParticipantsV1 = []types.RuntimeDiagramParticipantSelection{{NodeID: "T", NativeParticipant: rows[0].ToNode}}
	raw, _ := json.Marshal(doc)
	schema := (&tool.EmitAnswerDocument{}).ParametersFor(ctx)
	if !strings.Contains(string(schema), "runtime_participants_v1") || !strings.Contains(string(schema), rows[0].ToNode) {
		t.Fatal("actual finalizer schema lacks current display choices")
	}
	// The public LLM envelope is blocks-only; persisted DocumentModel is not
	// part of the executable schema.
	var envelope map[string]any
	_ = json.Unmarshal(raw, &envelope)
	delete(envelope, "document_model")
	raw, _ = json.Marshal(envelope)
	if err := toolparam.Validate(raw, schema); err != nil {
		t.Fatalf("actual schema: %v", err)
	}
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	res, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || !res.Success {
		t.Fatalf("actual emit: %v %s", err, res.Summary)
	}
	got := ctx.Mutable.AnswerDocumentV2()
	if !strings.Contains(got.Blocks[1].Diagram.Body, "Note over T: 原始说明，不作新因果证明") || len(got.Blocks[1].EdgeAnchors) != 3 {
		t.Fatalf("native events/Note lost: %+v", got.Blocks[1])
	}
	if failures := tool.DiagramParticipantCoverageMismatchesWithRuntimeContext(bus, got, types.BuildAnswerSemanticViewForBusContext(bus), nil); len(failures) > 0 {
		t.Fatalf("shared display diverged from request binding: %+v", failures)
	}
	patch, _ := json.Marshal(map[string]any{"replace_blocks": []types.AnswerBlock{got.Blocks[1]}, "unchanged_block_ids": []string{got.Blocks[0].ID}})
	if err := toolparam.Validate(patch, (&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx)); err != nil {
		t.Fatalf("actual patch schema: %v", err)
	}
	res, err = (&tool.EmitAnswerDocumentPatch{}).Execute(bus, patch)
	if err != nil || !res.Success {
		t.Fatalf("actual patch: %v %s", err, res.Summary)
	}
	ctx.Mutable = types.NewMutableState("no current native query")
	if strings.Contains(string((&tool.EmitAnswerDocument{}).ParametersFor(ctx)), "runtime_participants_v1") || strings.Contains(string((&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx)), "runtime_participants_v1") {
		t.Fatal("source-less schema publishes stale choices")
	}
	res, err = (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
	if err != nil || res.Success {
		t.Fatalf("saved JSON manufactured native identity: %v %s", err, res.Summary)
	}
}
