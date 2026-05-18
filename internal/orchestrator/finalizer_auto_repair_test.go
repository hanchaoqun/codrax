package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestFinalizerAutoRepairAddsEvidenceSufficientFacetIDsAndStripsBackticks(t *testing.T) {
	mut := types.NewMutableState("auto repair")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:          "summary_1",
				Kind:        types.BlockSummary,
				SurfaceRole: types.SurfacePrincipal,
				Text:        "Uses `SUBSCRIBE_FEATURE`, `QueryFeatureResult`, and `FeatureTypeMgr` as ordinary context.",
				ClaimUses:   []types.RenderedClaimUse{{ClaimForm: types.ClaimDefinitionFact}},
			},
			{
				ID:              "caveat_1",
				Kind:            types.BlockCaveat,
				Text:            "bounded",
				ScopeDisclosure: types.ScopeDisclosureOutOfActiveScope,
			},
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut, Language: "zh"}}
	out := &agent.StageOutput{}

	applied := o.tryAutoRepairFinalizerAnswerDocument(out, []types.Violation{
		{
			Kind:   types.ViolFacetUncovered,
			Detail: `required facet "resolved_literal_or_symbol" (evidence-sufficient (12 typed evidence rows support this facet)) is not covered`,
		},
		{
			Kind:   types.ViolFacetUncovered,
			Detail: `required facet "current_code_path" (evidence-sufficient (12 typed evidence rows support this facet)) is not covered`,
		},
		{
			Kind:   types.ViolInlineIdentifierHallucinated,
			Detail: `block "summary_1" has 3 inline-backtick identifier(s): [block="summary_1" ident="SUBSCRIBE_FEATURE" surface="text"; block="summary_1" ident="QueryFeatureResult" surface="text"; block="summary_1" ident="FeatureTypeMgr" surface="text"]`,
		},
	})
	if !applied {
		t.Fatal("expected auto repair to apply")
	}
	doc := mut.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) == 0 {
		t.Fatalf("missing repaired document: %+v", doc)
	}
	gotFacets := strings.Join(doc.Blocks[0].FacetIDs, ",")
	for _, want := range []string{"current_code_path", "resolved_literal_or_symbol"} {
		if !strings.Contains(gotFacets, want) {
			t.Fatalf("facet %q not added to principal block: %+v", want, doc.Blocks[0].FacetIDs)
		}
	}
	if strings.Contains(doc.Blocks[0].Text, "`SUBSCRIBE_FEATURE`") ||
		strings.Contains(doc.Blocks[0].Text, "`QueryFeatureResult`") ||
		strings.Contains(doc.Blocks[0].Text, "`FeatureTypeMgr`") {
		t.Fatalf("unsupported inline-code backticks not stripped: %q", doc.Blocks[0].Text)
	}
	if strings.TrimSpace(out.FinalAnswer) == "" || strings.Contains(out.FinalAnswer, "`SUBSCRIBE_FEATURE`") {
		t.Fatalf("rendered answer not refreshed after repair:\n%s", out.FinalAnswer)
	}
}

func TestFinalizerAutoRepairDoesNotInventEssentialFacetCoverage(t *testing.T) {
	mut := types.NewMutableState("auto repair")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "summary_1",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "No structural metadata yet.",
		}},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut, Language: "zh"}}

	applied := o.tryAutoRepairFinalizerAnswerDocument(&agent.StageOutput{}, []types.Violation{{
		Kind:   types.ViolFacetUncovered,
		Detail: `required facet "diagram_spine" (essential) is not covered`,
	}})
	if applied {
		t.Fatal("essential facet coverage must not be auto-invented")
	}
	doc := mut.AnswerDocumentV2()
	if len(doc.Blocks[0].FacetIDs) != 0 {
		t.Fatalf("unexpected facet mutation: %+v", doc.Blocks[0].FacetIDs)
	}
}

func TestFinalizerAutoRepairDoesNotInventShapeBearingFacetCoverage(t *testing.T) {
	mut := types.NewMutableState("auto repair")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "summary_1",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "Plain summary without a list or diagram.",
		}},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut, Language: "zh"}}

	applied := o.tryAutoRepairFinalizerAnswerDocument(&agent.StageOutput{}, []types.Violation{{
		Kind:   types.ViolFacetUncovered,
		Detail: `required facet "enumeration_item" (evidence-sufficient (12 typed evidence rows support this facet)) is not covered`,
	}})
	if applied {
		t.Fatal("shape-bearing facet coverage must not be auto-invented")
	}
	doc := mut.AnswerDocumentV2()
	if len(doc.Blocks[0].FacetIDs) != 0 {
		t.Fatalf("unexpected facet mutation: %+v", doc.Blocks[0].FacetIDs)
	}
}
