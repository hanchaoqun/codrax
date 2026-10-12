package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC227RejectedDiagramActualEmitRecovery(t *testing.T) {
	ctx, _ := nativeDisplayBindingContext224(t, `{}`)
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "Preserve the recorded facts independently of the unverified drawing."},
		{ID: "wrong_graph", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n invented_source --> invented_cause"}, EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "invented_source", ToNode: "invented_cause", FromIdentity: "MissingSource", ToIdentity: "MissingCause", RelationKind: types.DiagramRelCall}}},
	}}
	raw, _ := json.Marshal(doc)
	result, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
	if err != nil || result.Success || result.Repair == nil || result.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON] == "" {
		t.Fatalf("fixture must reach real relation rejection: %v %+v", err, result)
	}
	if ctx.Mutable.LastRejectedAnswerDocumentV2() == nil {
		t.Fatal("real reject did not preserve the draft")
	}
	out, err := (&answerDocumentEvaluator{mu: ctx.Mutable, language: "en"}).ParseOutput(ctx, []llm.Message{{Role: "assistant", Content: "Failed repair scratch."}}, nil, nil)
	if err != nil || !out.AnswerDegraded || !out.SkipAnswerChecks || ctx.Mutable.AnswerDocumentV2() != nil {
		t.Fatalf("recovery must remain unaccepted: %v %+v", err, out)
	}
	if strings.Contains(out.FinalAnswer, "invented_source --> invented_cause") {
		t.Fatalf("rejected relation escaped through recovery: %s", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "9007199254741001") || !strings.Contains(out.FinalAnswer, doc.Blocks[0].Text) {
		t.Fatalf("diagram isolation lost native facts or unrelated prose: %s", out.FinalAnswer)
	}
	if ctx.Mutable.LastRejectedAnswerDocumentV2().Blocks[1].Diagram == nil {
		t.Fatal("display isolation mutated the raw rejected draft")
	}
}

func TestHMC227LosslessTextDoesNotAcceptUncheckedGraph(t *testing.T) {
	ctx, _ := nativeDisplayBindingContext224(t, `{}`)
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "Native facts remain useful."},
		{ID: "graph", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n invented_source --> invented_cause"}},
	}}
	raw, _ := json.Marshal(doc)
	ctx.Mutable.SetAnswerDisplayAttachments([]types.AnswerDisplayAttachment{{Kind: types.AnswerDisplayAttachmentDiagram, Body: doc.Blocks[1].Diagram.Body, Source: "orchestrator.system_crosscheck_appendix"}})
	out, err := (&answerDocumentEvaluator{mu: ctx.Mutable, language: "en"}).ParseOutput(ctx, []llm.Message{{Role: "assistant", Content: string(raw)}}, nil, nil)
	if err != nil || ctx.Mutable.AnswerDocumentV2() != nil || !out.AnswerDegraded || !out.SkipAnswerChecks {
		t.Fatalf("structural recovery promoted unchecked graph: %v %+v", err, out)
	}
	if strings.Contains(out.FinalAnswer, "invented_source") || !strings.Contains(out.FinalAnswer, "9007199254741001") {
		t.Fatalf("unchecked graph or lost native facts: %s", out.FinalAnswer)
	}
}

func TestHMC227RawProseCannotResurrectUnvalidatedGraphs(t *testing.T) {
	ctx, _ := nativeDisplayBindingContext224(t, `{}`)
	body := "flowchart TD\n invented_source --> invented_cause"
	ctx.Mutable.SetAnswerDisplayAttachments([]types.AnswerDisplayAttachment{{Kind: types.AnswerDisplayAttachmentDiagram, Body: body, Source: "emit_answer_document.retry_state"}})
	out, err := (&answerDocumentEvaluator{mu: ctx.Mutable, language: "en"}).ParseOutput(ctx, []llm.Message{{Role: "assistant", Content: "Useful prose.\n```mermaid\n" + body + "\n```"}}, nil, nil)
	if err != nil || out == nil || !out.AnswerDegraded || strings.Contains(out.FinalAnswer, "invented_source") || !strings.Contains(out.FinalAnswer, "Useful prose.") || !strings.Contains(out.FinalAnswer, "9007199254741001") {
		t.Fatalf("raw recovery escaped isolation or lost facts: %v %+v", err, out)
	}
}
