package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRequiredRuntimeDiagramCannotHideAvailableRelationsBehindUnprovenRoles(t *testing.T) {
	r := queryTransactionDiagramSupport(t, 1, 1.05)
	ctx := transactionRequiredDiagramContext(t, types.DiagramFlow, 1, 1.05, []types.ToolResult{r})
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	e := &answerDocumentEvaluator{mu: ctx.Mutable, language: "zh"}
	_ = e.BuildInitialInstruction(ctx, nil)
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "保留原说明，不把协议对应当根因。"},
		{ID: "handoffs", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n App[\"应用提交端\"]\n Render[\"渲染服务消费端\"]\n"},
			ParticipantBoundaries: []types.DiagramParticipantBoundary{{Participant: "应用提交端", Status: types.DiagramParticipantBoundaryUnproven}, {Participant: "渲染服务消费端", Status: types.DiagramParticipantBoundaryUnproven}}},
	}}
	if !tool.RequiredRuntimeDiagramRelationMissing(bus, doc, view) {
		t.Fatal("available native relations must not be erased by unbound reader roles")
	}
	raw, _ := json.Marshal(doc)
	result, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || result.Success || !installAnswerDocDiagramRelationRepairLease(ctx, ctx.Mutable, &result, false) {
		t.Fatalf("must reject and publish executable existing-relation repair: err=%v result=%+v", err, result)
	}
	lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
	if lease == nil || len(lease.AllowedAdditions) != 3 {
		t.Fatalf("expected the three exact native choices, not a system-chosen edge: %+v", lease)
	}
	choice := lease.AllowedAdditions[0]
	patch := map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_edge_edits": []any{map[string]any{
		"addition_ref": choice.AdditionRef, "action": "add", "edge": map[string]string{
			"from_node": choice.FromNodeIDs[0], "to_node": choice.ToNodeIDs[0], "visible_label": "协议记录对应",
		},
		"from_node_visible_label": "提交记录", "to_node_visible_label": "消费记录",
	}}}
	raw, _ = json.Marshal(patch)
	if err := toolparam.Validate(raw, (&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx)); err != nil {
		t.Fatal(err)
	}
	result, err = (&tool.EmitAnswerDocumentPatch{}).Execute(bus, raw)
	if err != nil || !result.Success {
		t.Fatalf("model-selected exact native relation repair failed: %v %+v", err, result)
	}
	got := ctx.Mutable.AnswerDocumentV2()
	if got == nil || got.Blocks[0].Text != doc.Blocks[0].Text || tool.RequiredRuntimeDiagramRelationMissing(bus, got, view) {
		t.Fatalf("repair lost sibling content or remained incomplete: %+v", got)
	}
	if edges := mermaidcompat.ParseEdges(got.Blocks[1].Diagram.Body); len(edges) != 1 {
		t.Fatalf("minimum requirement must not force all three choices: %+v", edges)
	}
	if mismatches := tool.DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(bus, got, view, nil); len(mismatches) != 0 {
		t.Fatalf("existing relation authority rejected repaired exact tuple: %+v", mismatches)
	}
}

func TestVisibilityRepairPublishedSchemaExecutesUnicodeIdentity(t *testing.T) {
	for _, id := range []string{"SubmitNode", "提交端", "Émetteur"} {
		t.Run(id, func(t *testing.T) {
			base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
				{ID: "summary", Kind: types.BlockSummary, Text: "原说明"},
				{ID: "diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n A[\"Analyzer\"]\n"}, ParticipantBoundaries: []types.DiagramParticipantBoundary{{Participant: "BusContext", Status: types.DiagramParticipantBoundaryUnproven}}},
			}}
			lease := types.WithAnswerDiagramParticipantVisibilityRepairFailures(base, nil, []types.AnswerDiagramParticipantVisibilityRepairFailure{{BlockID: "diagram", Participant: "BusContext", Issue: "boundary_participant_not_visible"}})
			mut := types.NewMutableState("visibility")
			mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
			mut.SetAnswerDiagramRelationRepairLease(lease)
			payload, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_participant_edits": []any{map[string]string{
				"participant_ref": lease.ParticipantVisibilityFailures[0].ParticipantRef, "action": "ensure_visible", "node_id": id, "visible_label": "BusContext",
			}}})
			if err := toolparam.Validate(payload, (&tool.EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: mut})); err != nil {
				t.Fatal(err)
			}
			result, err := (&tool.EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut}, payload)
			if err != nil || !result.Success {
				t.Fatalf("schema-valid Unicode declaration was not executable: %v %+v", err, result)
			}
			got := mut.AnswerDocumentV2()
			if got.Blocks[0].Text != "原说明" || !strings.Contains(got.Blocks[1].Diagram.Body, id+`["BusContext"]`) || len(mermaidcompat.ParseEdges(got.Blocks[1].Diagram.Body)) != 0 {
				t.Fatalf("visibility-only patch changed semantics: %+v", got)
			}
		})
	}
}

func TestRequiredRuntimeDiagramCoverageScopeAndMetadataBoundaries(t *testing.T) {
	r := queryTransactionDiagramSupport(t, 1, 1.05)
	for _, tc := range []struct {
		name    string
		mutate  func(*types.AgentContext, *types.AnswerSemanticView, *types.AnswerDocumentV2)
		missing bool
	}{
		{"required_zero_edges", nil, true},
		{"optional", func(ctx *types.AgentContext, view *types.AnswerSemanticView, _ *types.AnswerDocumentV2) {
			ctx.AnalysisIR.AnswerContract.Diagram.Required = false
			view.DiagramPlan.Required = false
		}, false},
		{"root_cause_projection", func(_ *types.AgentContext, view *types.AnswerSemanticView, _ *types.AnswerDocumentV2) {
			view.Family = types.QFRootCauseTrace
		}, false},
		{"node_only_contract", func(_ *types.AgentContext, view *types.AnswerSemanticView, _ *types.AnswerDocumentV2) {
			view.DiagramPlan.RequireStructuralEdge = false
		}, false},
		{"wrong_window", func(ctx *types.AgentContext, _ *types.AnswerSemanticView, _ *types.AnswerDocumentV2) {
			s, e := 4.0, 4.1
			ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeStart = &s
			ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = &e
		}, false},
		{"wrong_target", func(ctx *types.AgentContext, _ *types.AnswerSemanticView, _ *types.AnswerDocumentV2) {
			ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 888, Source: "user_explicit"}}
		}, false},
		{"precise_source_question", func(ctx *types.AgentContext, _ *types.AnswerSemanticView, _ *types.AnswerDocumentV2) {
			ctx.AnalysisIR.RequestModel.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{IsCurrentSourceExplanationRequested: true, SourceQuotes: []string{"internal/tracequery/parse.go"}, Modes: []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationTraceCurrentFlow}}
		}, false},
		{"metadata_without_arrow", func(ctx *types.AgentContext, _ *types.AnswerSemanticView, doc *types.AnswerDocumentV2) {
			rows := tool.RuntimeDiagramRelations(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)
			doc.Blocks[1].EdgeAnchors = transactionSupportedDiagram(rows[:1], types.DiagramFlow).Blocks[1].EdgeAnchors
		}, true},
		{"unsupported_visible_arrow", func(_ *types.AgentContext, _ *types.AnswerSemanticView, doc *types.AnswerDocumentV2) {
			doc.Blocks[1].Diagram.Body += "\n A --> B"
			doc.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", FromIdentity: "unproved-a", ToIdentity: "unproved-b", RelationKind: types.DiagramRelObserve}}
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := transactionRequiredDiagramContext(t, types.DiagramFlow, 1, 1.05, []types.ToolResult{r})
			view := types.BuildAnswerSemanticViewForAgentContext(ctx)
			doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
				{ID: "summary", Kind: types.BlockSummary, Text: "保留原说明"},
				{ID: "diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n A[\"提交端\"]\n B[\"消费端\"]"}},
			}}
			if tc.mutate != nil {
				tc.mutate(ctx, view, doc)
			}
			if got := tool.RequiredRuntimeDiagramRelationMissing(types.ToolBusContext(ctx, types.AgentFinalizer), doc, view); got != tc.missing {
				t.Fatalf("coverage missing=%t want=%t", got, tc.missing)
			}
		})
	}
}
