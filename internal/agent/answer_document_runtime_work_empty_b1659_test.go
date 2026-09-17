package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1659EmptyContext(lang string, profileOnly bool) *types.AgentContext {
	ctx := b1659RuntimeWorkContext(lang, profileOnly)
	ctx.Mutable.ResetDispatchToolResults()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
	return ctx
}

func b1659BoundaryDocument() *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "work-boundary", Kind: types.BlockCaveat, SurfaceRole: types.SurfacePrincipal,
		FacetIDs: []string{"runtime_work_relation", "uncertainty_boundary"},
		Text:     "The capture establishes thread waits, but does not identify a measured business operation to relate to the target.",
	}}}
}

func TestB1659EmptySupplySchemaTeachingAndCoverageAgree(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, profileOnly := range []bool{false, true} {
			t.Run(lang+map[bool]string{false: "/dimension", true: "/profile"}[profileOnly], func(t *testing.T) {
				ctx := b1659EmptyContext(lang, profileOnly)
				view := types.BuildAnswerSemanticViewForAgentContext(ctx)
				if view == nil || view.RuntimeWorkRelationContract.Active() {
					t.Fatal("fixture must have an actual compiled empty work-row supply")
				}
				var schema map[string]any
				if err := json.Unmarshal(tool.BuildAnswerDocumentParametersFor(view), &schema); err != nil {
					t.Fatal(err)
				}
				props := schema["properties"].(map[string]any)["blocks"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
				if _, exists := props["runtime_work_relation"]; exists {
					t.Fatal("empty supply must not publish a fabricated work choice")
				}
				e := &answerDocumentEvaluator{}
				initial := e.BuildInitialInstruction(ctx, nil)
				missing := []types.RequestedAnswerDimension{{Role: types.RequestedAnswerDimensionRuntimeWorkRelation, Required: true}}
				for surface, text := range map[string]string{"initial": initial, "repair": requestedAnswerDimensionCoverageHint(ctx, missing, lang)} {
					if strings.Contains(text, "选择一个精确 `runtime_work_relation:{") || strings.Contains(text, "select one exact `runtime_work_relation:{") {
						t.Errorf("%s requests an absent schema choice", surface)
					}
					for _, token := range []string{`kind:"caveat"`, `surface_role:"principal"`, `facet_ids:["runtime_work_relation","uncertainty_boundary"]`} {
						if !strings.Contains(text, token) {
							t.Errorf("%s lacks executable no-evidence ownership %s", surface, token)
						}
					}
				}
				doc := b1659BoundaryDocument()
				if !requestedAnswerDimensionRoleOwnedByBlock(ctx, types.RequestedAnswerDimensionRuntimeWorkRelation, doc.Blocks[0]) {
					t.Error("dimension order and payload coverage disagree on the boundary owner")
				}
				before, _ := json.Marshal(doc)
				ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
				sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
				if sig.HintRequested || !sig.StopRequested || e.retriesUsed != 0 || e.rejectHintsUsed != 0 {
					t.Errorf("model-authored no-evidence boundary caused impossible repair: %+v", sig)
				}
				after, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
				if string(before) != string(after) {
					t.Fatal("coverage rewrote the model's answer")
				}
			})
		}
	}
}

func TestB1659EmptyBoundaryOwnershipCannotStandInForEvidence(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*types.AnswerBlock)
	}{
		{"prose only", func(b *types.AnswerBlock) { b.FacetIDs = nil }},
		{"not a boundary", func(b *types.AnswerBlock) { b.Kind = types.BlockSummary }},
		{"no boundary owner", func(b *types.AnswerBlock) { b.FacetIDs = []string{"runtime_work_relation"} }},
		{"no work owner", func(b *types.AnswerBlock) { b.FacetIDs = []string{"uncertainty_boundary"} }},
		{"support", func(b *types.AnswerBlock) { b.SurfaceRole = "" }},
		{"system generated", func(b *types.AnswerBlock) { b.SystemGeneratedKind = types.AnswerSystemGeneratedEvidenceSupplement }},
		{"empty surface", func(b *types.AnswerBlock) { b.Text = "" }},
		{"invented receipt", func(b *types.AnswerBlock) {
			b.RuntimeWorkRelation = &types.AnswerRuntimeWorkRelationReceipt{ObservationID: "scheduler-row", Conclusion: types.RuntimeWorkRelationConclusionRelationUnproven}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, doc := b1659EmptyContext("en", true), b1659BoundaryDocument()
			tc.edit(&doc.Blocks[0])
			if answerDocumentHasRuntimeWorkRelationPayload(ctx, doc) {
				t.Fatal("unowned/unsupported disclosure must not satisfy structured coverage")
			}
			if requestedAnswerDimensionRoleOwnedByBlock(ctx, types.RequestedAnswerDimensionRuntimeWorkRelation, doc.Blocks[0]) {
				t.Fatal("dimension order accepted a carrier rejected by payload coverage")
			}
		})
	}
	// Real public trace_query semantic rows must continue to require the exact
	// model-selected receipt, not the empty-supply disclosure branch.
	ctx, _ := b1706SemanticFactContext(t)
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.RuntimeWorkRelationRequested = true
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	if view == nil || !view.RuntimeWorkRelationContract.Active() {
		t.Fatal("public semantic query must publish real work choices")
	}
	if answerDocumentHasRuntimeWorkRelationPayload(ctx, b1659BoundaryDocument()) {
		t.Fatal("no-evidence owner bypassed available exact work choices")
	}
}

func TestB1659EmptyBoundaryTracksCurrentSupplyWithoutStickyAbsence(t *testing.T) {
	ctx, doc := b1659EmptyContext("en", true), b1659BoundaryDocument()
	if !answerDocumentHasRuntimeWorkRelationPayload(ctx, doc) {
		t.Fatal("initial empty supply must support an honest boundary")
	}
	b1659AddWorkSupply(ctx)
	if answerDocumentHasRuntimeWorkRelationPayload(ctx, doc) ||
		!strings.Contains(runtimeWorkRelationTeachingForContext(ctx, "en"), "select one exact") {
		t.Fatal("new semantic evidence was masked by stale empty-supply guidance")
	}
	ctx.Mutable.ResetDispatchToolResults()
	if !answerDocumentHasRuntimeWorkRelationPayload(ctx, doc) ||
		strings.Contains(runtimeWorkRelationTeachingForContext(ctx, "en"), "select one exact") {
		t.Fatal("retired semantic supply still required a nonexistent choice")
	}
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.RuntimeWorkRelationRequested = false
	if answerDocumentHasRuntimeWorkRelationPayload(ctx, doc) || renderAnswerDocRequestedAnswerDimensions(ctx) != "" {
		t.Fatal("ordinary runtime evidence acquired an unrequested work-relation obligation")
	}
}
