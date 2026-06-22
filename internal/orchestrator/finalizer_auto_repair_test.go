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

func TestRecoverRejectedFinalizerDraftAfterTransientFailure_MetadataOnly(t *testing.T) {
	mut := types.NewMutableState("recover rejected draft")
	mut.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/foo.go", Line: 12}},
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "Current code path is already visible as internal/foo.go:12.",
			Items: []types.AnswerBlockItem{{
				ID:          "i1",
				Label:       "Foo",
				Text:        "grounded row",
				CitationRef: 0,
			}},
		}},
	})
	mut.SetRetryState(&types.RetryState{
		ActiveViolations: []types.ScoredViolation{{
			Kind:   types.ViolFacetUncovered,
			Detail: `required facet "current_code_path" is not covered`,
		}},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut, Language: "zh"}}

	out := o.recoverRejectedFinalizerDraftAfterTransientFailure(types.AnswerContract{})
	if out == nil || strings.TrimSpace(out.FinalAnswer) == "" {
		t.Fatal("expected rejected draft to recover after metadata-only repair")
	}
	if !strings.Contains(out.FinalAnswer, "补充说明") {
		t.Fatalf("recovered answer should disclose transient recovery:\n%s", out.FinalAnswer)
	}
	doc := mut.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) == 0 {
		t.Fatalf("expected recovered document to be installed: %+v", doc)
	}
	if got := strings.Join(doc.Blocks[0].FacetIDs, ","); !strings.Contains(got, "current_code_path") {
		t.Fatalf("metadata facet not repaired: %+v", doc.Blocks[0].FacetIDs)
	}
}

func TestRecoverRejectedFinalizerDraftAfterTransientFailure_DropsRejectedTextAttachments(t *testing.T) {
	mut := types.NewMutableState("recover rejected draft")
	mut.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/foo.go", Line: 12}},
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "Current code path is already visible as internal/foo.go:12.",
			Items: []types.AnswerBlockItem{{
				ID:          "i1",
				Label:       "Foo",
				Text:        "grounded row",
				CitationRef: 0,
			}},
		}},
	})
	mut.SetAnswerDisplayAttachments([]types.AnswerDisplayAttachment{{
		Kind:   types.AnswerDisplayAttachmentMarkdown,
		Body:   "stale rejected prose that failed validation",
		Source: "emit_answer_document.rejected_payload",
		Reason: "rejected structured answer draft contained user-visible text",
	}})
	mut.SetRetryState(&types.RetryState{
		ActiveViolations: []types.ScoredViolation{{
			Kind:   types.ViolFacetUncovered,
			Detail: `required facet "current_code_path" is not covered`,
		}},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut, Language: "zh"}}

	out := o.recoverRejectedFinalizerDraftAfterTransientFailure(types.AnswerContract{})
	if out == nil || strings.TrimSpace(out.FinalAnswer) == "" {
		t.Fatal("expected rejected draft to recover after metadata-only repair")
	}
	if strings.Contains(out.FinalAnswer, "stale rejected prose") ||
		strings.Contains(out.FinalAnswer, "系统保留内容") ||
		strings.Contains(out.FinalAnswer, "保留的原文") {
		t.Fatalf("rejected text attachment leaked into recovered final answer:\n%s", out.FinalAnswer)
	}
	if got := mut.AnswerDisplayAttachments(); len(got) != 0 {
		t.Fatalf("accepted recovered doc should clear rejected text attachments, got %+v", got)
	}
}

func TestRecoverRejectedFinalizerDraftAfterTransientFailure_RefusesShapeBearingFacet(t *testing.T) {
	mut := types.NewMutableState("recover rejected draft")
	mut.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "No diagram is present.",
		}},
	})
	mut.SetRetryState(&types.RetryState{
		ActiveViolations: []types.ScoredViolation{{
			Kind:   types.ViolFacetUncovered,
			Detail: `required facet "diagram_spine" is not covered`,
		}},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut, Language: "zh"}}

	if out := o.recoverRejectedFinalizerDraftAfterTransientFailure(types.AnswerContract{}); out != nil {
		t.Fatalf("shape-bearing diagram_spine rejection must not recover as final answer: %+v", out)
	}
}
