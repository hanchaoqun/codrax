package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC227LastMileAcceptedDocCannotReviveRejectedAttachment(t *testing.T) {
	ctx, _ := nativeDisplayBindingContext224(t, `{}`)
	rejected := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "Retain native measurements."},
		{ID: "wrong_graph", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n invented_source --> invented_cause"}, EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "invented_source", ToNode: "invented_cause", FromIdentity: "MissingSource", ToIdentity: "MissingCause", RelationKind: types.DiagramRelCall}}},
	}}
	raw, _ := json.Marshal(rejected)
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	res, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || res.Success || ctx.Mutable.LastRejectedAnswerDocumentV2() == nil || len(ctx.Mutable.AnswerDisplayAttachments()) == 0 {
		t.Fatalf("actual rejected emit did not preserve the graph: %v %+v", err, res)
	}
	res, err = (&tool.EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"replace_blocks":[{"id":"summary","kind":"summary","text":"Accepted replacement with native facts and no model graph."},{"id":"wrong_graph","kind":"section","text":"The requested relationship is unproven."}]}`))
	if err != nil || !res.Success {
		t.Fatalf("replacement must actually pass emit: %v %+v", err, res)
	}
	// A successful patch has its own existing retry-state lifecycle. Snapshot
	// after acceptance so this assertion isolates the later rendering mutation.
	failedBefore, _ := json.Marshal(ctx.Mutable.LastRejectedAnswerDocumentV2())
	acceptedBefore, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
	out, err := (&answerDocumentEvaluator{mu: ctx.Mutable, language: "en"}).ParseOutput(ctx, nil, nil, nil)
	if err != nil || out == nil || out.AnswerDegraded || strings.Contains(out.FinalAnswer, "invented_source") {
		t.Fatalf("accepted ParseOutput revived the rejected attachment: %v %+v", err, out)
	}
	if !strings.Contains(out.FinalAnswer, "9007199254741001") || !strings.Contains(out.FinalAnswer, "Accepted replacement") || strings.Count(out.FinalAnswer, tool.RecoveryDiagramIsolationNotice("en")) != 1 {
		t.Fatalf("native facts/replacement/one isolation notice missing: %s", out.FinalAnswer)
	}
	failedAfter, _ := json.Marshal(ctx.Mutable.LastRejectedAnswerDocumentV2())
	acceptedAfter, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
	if string(failedBefore) != string(failedAfter) || string(acceptedBefore) != string(acceptedAfter) {
		t.Fatal("display-only attachment isolation changed a document carrier")
	}
	for _, att := range ctx.Mutable.AnswerDisplayAttachments() {
		if strings.Contains(att.Body, "invented_source") {
			t.Fatal("unverified graph retained in the visible attachment carrier")
		}
	}
	// Re-rendering must keep the disclosure without multiplying it.
	again, err := (&answerDocumentEvaluator{mu: ctx.Mutable, language: "en"}).ParseOutput(ctx, nil, nil, nil)
	if err != nil || strings.Count(again.FinalAnswer, tool.RecoveryDiagramIsolationNotice("en")) != 1 {
		t.Fatalf("isolation disclosure was lost or duplicated on re-render: %v %+v", err, again)
	}
}

func TestHMC227LastMileAttachmentsDoNotRecheckAcceptedDocument(t *testing.T) {
	ctx, _ := nativeDisplayBindingContext224(t, `{}`)
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "Accepted document carrier remains unchanged."},
		{ID: "native", Kind: types.BlockDiagram, SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n native_from --> native_to"}},
	}}
	ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	ctx.Mutable.SetAnswerDisplayAttachments([]types.AnswerDisplayAttachment{
		{Kind: types.AnswerDisplayAttachmentDiagram, Body: "flowchart TD\n old_from --> old_to", Source: types.AnswerDisplayAttachmentSourceSystemCrossCheck},
		{Kind: types.AnswerDisplayAttachmentMarkdown, Title: "System note", Body: "Independent system text remains visible.", Source: types.AnswerDisplayAttachmentSourceSystemCrossCheck},
	})
	before, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
	out, err := (&answerDocumentEvaluator{mu: ctx.Mutable, language: "en"}).ParseOutput(ctx, nil, nil, nil)
	if err != nil || strings.Contains(out.FinalAnswer, "old_from") || !strings.Contains(out.FinalAnswer, "native_from") || !strings.Contains(out.FinalAnswer, "Independent system text") {
		t.Fatalf("last-mile attachment policy changed the document or trusted prose: %v %+v", err, out)
	}
	after, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
	if string(before) != string(after) {
		t.Fatal("normal document was rechecked/mutated through attachment cleanup")
	}
}

func TestHMC227LastMileIsolationNoticePersistsWithoutGraphAuthority(t *testing.T) {
	for _, language := range []string{"en", "zh"} {
		t.Run(language, func(t *testing.T) {
			ctx, _ := nativeDisplayBindingContext224(t, `{}`)
			ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
				DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "Accepted facts."}},
			})
			ctx.Mutable.SetAnswerDisplayAttachments([]types.AnswerDisplayAttachment{{
				Kind: types.AnswerDisplayAttachmentDiagram, Body: "flowchart TD\n forged_notice_source --> cannot_sign_graph",
				Source: types.AnswerDisplayAttachmentSourceDiagramIsolation,
			}})
			e := &answerDocumentEvaluator{mu: ctx.Mutable, language: language}
			for round := 0; round < 2; round++ {
				out, err := e.ParseOutput(ctx, nil, nil, nil)
				if err != nil || strings.Contains(out.FinalAnswer, "forged_notice_source") || strings.Count(out.FinalAnswer, tool.RecoveryDiagramIsolationNotice(language)) != 1 {
					t.Fatalf("round %d: notice lost/duplicated or source signed graph: %v %+v", round, err, out)
				}
				ctx.Mutable.SetAnswerDisplayAttachments(tool.FilterAcceptedAnswerDisplayAttachments(ctx.Mutable.AnswerDocumentV2(), ctx.Mutable.AnswerDisplayAttachments()))
			}
		})
	}
}
