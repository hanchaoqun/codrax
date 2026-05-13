package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// === P1 (2026-05-10) — emit-time pre-validation chokepoint ===

// TestRunPreEmitChecks_NilSafe — nil doc / nil view return nil
// (no spurious rejections; the post-emit chain takes over).
func TestRunPreEmitChecks_NilSafe(t *testing.T) {
	if got := runPreEmitChecks(nil, nil, nil); got != nil {
		t.Errorf("nil/nil: want nil, got %v", got)
	}
	if got := runPreEmitChecks(&types.AnswerDocumentV2{}, nil, nil); got != nil {
		t.Errorf("nil view: want nil, got %v", got)
	}
	if got := runPreEmitChecks(nil, &types.AnswerSemanticView{}, nil); got != nil {
		t.Errorf("nil doc: want nil, got %v", got)
	}
}

func TestPreCheckAbsenceScopeBound_RequiresNegativeCitation(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{Status: types.AnswerExactResolutionAbsent},
		Citations: []types.Citation{
			{File: "codrax.yaml", Line: 12},
		},
	}
	if got := preCheckAbsenceScopeBound(doc); len(got) != 1 {
		t.Fatalf("expected absence-scope pre-emit hint, got %+v", got)
	}
	doc.Citations = append(doc.Citations, types.Citation{
		File: "codrax.yaml", Scope: types.ScopeNegative, NegativePattern: "legacy_config_key",
	})
	if got := preCheckAbsenceScopeBound(doc); len(got) != 0 {
		t.Fatalf("negative-scope citation should satisfy absence pre-check, got %+v", got)
	}
}

func TestPreEmitBlockCitationRoleForms_TableParticipates(t *testing.T) {
	block := types.AnswerBlock{
		ID:   "table",
		Kind: types.BlockTable,
		ClaimUses: []types.RenderedClaimUse{{
			ClaimForm: types.ClaimImportEdge,
		}},
	}
	got := preEmitBlockCitationRoleForms(block, nil)
	if len(got) != 1 || got[0] != types.ClaimImportEdge {
		t.Fatalf("table blocks should participate in typed citation-role alignment, got %+v", got)
	}
}

func TestPreCheckCallChainItemCitationRoleAlignment_SkipsChangeImpactSourcePrincipalRows(t *testing.T) {
	mu := types.NewMutableState("change impact aggregate members")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "affected source members",
		Value: "1",
		Members: []string{
			"internal/agent/analyzer.go:1935",
		},
	}})
	mu.SetInvestigationComplete("aggregate member set emitted")
	mu.AppendEvidence([]types.EvidenceItem{{
		ID:              "bad-edge-context",
		Kind:            types.EvidenceRelationship,
		Scope:           types.ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       1234,
		AnchorKind:      types.AnchorCall,
		Subject:         "ShapeValue",
		Object:          "ShapeScalar",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
	}})
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentEnumerate,
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqEnumeration)},
				Predicates:    types.SemanticPredicates{IsCategoryEnumeration: true},
				ChangeImpactProfile: &types.ChangeImpactProfile{
					IsChangeImpact:  true,
					Target:          "ShapeValue",
					RequestedOutput: types.ImpactOutputFiles,
					Scope:           types.ImpactScopeProduction,
					Confidence:      0.9,
				},
			},
		},
	}
	view := &types.AnswerSemanticView{
		Family: types.QFEnumeration,
		RequiredBlocks: []types.BlockRequirement{{
			Kind:                 types.BlockOrderedList,
			Required:             true,
			AcceptableClaimForms: []types.ClaimForm{types.ClaimCallEdge},
			SurfaceRoleHint:      types.SurfacePrincipal,
		}},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/agent/analyzer.go", Line: 1935}},
		Blocks: []types.AnswerBlock{{
			ID:          "files",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetEnumerationItem)},
			Items: []types.AnswerBlockItem{{
				ID:          "analyzer",
				Label:       "internal/agent/analyzer.go",
				Text:        "rename ShapeValue -> ShapeScalar in the comment-only affected site",
				CitationRef: 0,
			}},
		}},
	}

	if hints := preCheckCallChainItemCitationRoleAlignment(doc, view, ctx); len(hints) != 0 {
		t.Fatalf("source-location principal member should not be reinterpreted as an edge role, got %+v", hints)
	}
}

func TestPreCheckAggregateScalarValueCoverage_RequiresModelAuthoredScalarFacts(t *testing.T) {
	mu := types.NewMutableState("scalar aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateScalar,
		Label: "retry budget parameter name",
		Value: "MaxRetriesPerStage",
	}, {
		Kind:  types.AnswerAggregateScalar,
		Label: "retry attempt counter field",
		Value: "EmitStageRetryAttempt",
	}})
	mu.SetInvestigationComplete("structured scalar facts accepted")
	ctx := &types.BusContext{Mutable: mu}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "Yes. The analyze stage retries through prependEmitRetryDirective and the attempt counter is `EmitStageRetryAttempt`.",
		}},
	}

	hints := preCheckAggregateScalarValueCoverage(doc, ctx)
	if len(hints) != 1 {
		t.Fatalf("missing scalar aggregate value should produce one hint, got %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "MaxRetriesPerStage") {
		t.Fatalf("hint should name the omitted model-authored scalar, got %+v", hints[0])
	}
	if strings.Contains(hints[0].ExpectedShape, "EmitStageRetryAttempt") {
		t.Fatalf("visible scalar should not be reported missing, got %+v", hints[0])
	}

	doc.Blocks[0].Text += " The retry budget parameter is `MaxRetriesPerStage`."
	if got := preCheckAggregateScalarValueCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("all scalar aggregate values are visible; got hints %+v", got)
	}
}

func TestPreCheckAggregateMemberSetCoverage_RequiresVisibleModelAuthoredMembers(t *testing.T) {
	mu := types.NewMutableState("enum aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "public enum types",
		Value:   "3",
		Members: []string{"Intent", "QuestionFamily", "Scenario @ internal/types/analysis_ir.go:673"},
	}})
	mu.SetInvestigationComplete("structured member set accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
				AnalyzerHints: types.AnalyzerHints{
					Kind:     string(types.ReqEnumeration),
					Entities: []string{"Intent", "QuestionFamily", "Scenario"},
				},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "all public enum types",
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "list",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:    "intent",
				Label: "Intent",
			}, {
				ID:    "scenario",
				Label: "Scenario",
			}},
		}},
	}

	hints := preCheckAggregateMemberSetCoverage(doc, ctx)
	if len(hints) != 1 {
		t.Fatalf("missing member_set value should produce one hint, got %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "QuestionFamily") {
		t.Fatalf("hint should name the omitted model-authored member, got %+v", hints[0])
	}
	if strings.Contains(hints[0].ExpectedShape, "Scenario") {
		t.Fatalf("visible display prefix should satisfy member entry, got %+v", hints[0])
	}

	doc.Blocks[0].Items = append(doc.Blocks[0].Items, types.AnswerBlockItem{ID: "family", Label: "QuestionFamily"})
	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("all member_set members are visible; got hints %+v", got)
	}
}

func TestPreCheckCitationPoolIntegrity_CatchesMissingAndOutOfRangeRefs(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "files",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{
				{ID: "a", CitationRef: 0},
				{ID: "b", CitationRef: 2},
			},
		}},
	}
	hints := preCheckCitationPoolIntegrity(doc)
	if len(hints) != 1 {
		t.Fatalf("missing citation pool should produce one hint, got %+v", hints)
	}
	if !strings.Contains(hints[0].Field, "citations") {
		t.Fatalf("hint should target citations[], got %+v", hints[0])
	}

	doc.Citations = []types.Citation{
		{File: "a.go", Line: 1},
		{File: "b.go", Line: 2},
	}
	hints = preCheckCitationPoolIntegrity(doc)
	if len(hints) != 1 || !strings.Contains(hints[0].ExpectedShape, "at least 3") {
		t.Fatalf("out-of-range citation_ref should ask for enough citation entries, got %+v", hints)
	}

	doc.Citations = append(doc.Citations, types.Citation{File: "c.go", Line: 3})
	if hints := preCheckCitationPoolIntegrity(doc); len(hints) != 0 {
		t.Fatalf("complete citation pool should pass, got %+v", hints)
	}
}

func TestPreCheckSummaryLeadBlock_RequiresSummaryFirstWhenViewRequiresIt(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true, MinCount: 1},
			{Kind: types.BlockOrderedList, Required: true, MinCount: 1},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "list", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{ID: "i1", Label: "A"}}},
			{ID: "summary", Kind: types.BlockSummary, Text: "lead"},
		},
	}
	hints := preCheckSummaryLeadBlock(doc, view)
	if len(hints) != 1 {
		t.Fatalf("summary after detail blocks should produce one hint, got %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "first") {
		t.Fatalf("hint should ask for summary first, got %+v", hints[0])
	}

	doc.Blocks[0], doc.Blocks[1] = doc.Blocks[1], doc.Blocks[0]
	if hints := preCheckSummaryLeadBlock(doc, view); len(hints) != 0 {
		t.Fatalf("summary-first document should pass, got %+v", hints)
	}
}

// TestPreCheckRequiredBlocks_MinCount — required block kind under-emitted.
func TestPreCheckRequiredBlocks_MinCount(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockOrderedList, Required: true, MinCount: 1, Rationale: "enumeration"},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
		},
	}
	hints := preCheckRequiredBlocks(doc, view)
	if len(hints) != 1 {
		t.Fatalf("want 1 fix hint, got %d", len(hints))
	}
	if !strings.Contains(hints[0].Field, "ordered_list") {
		t.Errorf("hint field should name the block kind; got %q", hints[0].Field)
	}
	if !strings.Contains(hints[0].ExpectedShape, "at least 1") {
		t.Errorf("hint should state the required minimum; got %q", hints[0].ExpectedShape)
	}
}

// TestPreCheckRequiredBlocks_MaxCount — required block kind over-emitted.
func TestPreCheckRequiredBlocks_MaxCount(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true, MinCount: 1, MaxCount: 1, Rationale: "lead"},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
			{ID: "s2", Kind: types.BlockSummary, Text: "y"},
		},
	}
	hints := preCheckRequiredBlocks(doc, view)
	if len(hints) != 1 {
		t.Fatalf("want 1 fix hint, got %d", len(hints))
	}
	if !strings.Contains(hints[0].ExpectedShape, "at most 1") {
		t.Errorf("hint should state the maximum cap; got %q", hints[0].ExpectedShape)
	}
}

// TestPreCheckRequiredBlocks_HappyPath — exact match returns no hints.
func TestPreCheckRequiredBlocks_HappyPath(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true, MinCount: 1, MaxCount: 1},
			{Kind: types.BlockOrderedList, Required: true, MinCount: 1},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
			{ID: "l1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{ID: "i1"}}},
		},
	}
	if hints := preCheckRequiredBlocks(doc, view); len(hints) != 0 {
		t.Errorf("happy path should produce no hints; got %v", hints)
	}
}

// TestPreCheckPrincipalClaimUse_Missing — principal block missing claim_uses.
func TestPreCheckPrincipalClaimUse_Missing(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{
				Kind:                 types.BlockOrderedList,
				Required:             true,
				MinCount:             1,
				AcceptableClaimForms: []types.ClaimForm{types.ClaimDefinitionFact, types.ClaimCallEdge},
				SurfaceRoleHint:      types.SurfacePrincipal,
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:          "l1",
				Kind:        types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				Items:       []types.AnswerBlockItem{{ID: "i1"}},
				// no ClaimUses → fix hint expected
			},
		},
	}
	hints := preCheckPrincipalClaimUse(doc, view)
	if len(hints) != 1 {
		t.Fatalf("want 1 fix hint, got %d", len(hints))
	}
	if !strings.Contains(hints[0].Field, "claim_uses") {
		t.Errorf("hint field should name claim_uses; got %q", hints[0].Field)
	}
	if !strings.Contains(hints[0].ExpectedShape, "definition_fact") {
		t.Errorf("hint should list the acceptable forms; got %q", hints[0].ExpectedShape)
	}
}

// TestPreCheckPrincipalClaimUse_SingleFormRelaxation — when contract
// declares exactly one AcceptableClaimForm AND the block carries
// structural grounding (facet_ids + cited items), the missing
// claim_uses is treated as implicit-default and NOT flagged.
func TestPreCheckPrincipalClaimUse_SingleFormRelaxation(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{
				Kind:                 types.BlockOrderedList,
				Required:             true,
				MinCount:             1,
				AcceptableClaimForms: []types.ClaimForm{types.ClaimDefinitionFact},
				SurfaceRoleHint:      types.SurfacePrincipal,
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:          "l1",
				Kind:        types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				FacetIDs:    []string{"enumeration_item"},
				Items:       []types.AnswerBlockItem{{ID: "i1", CitationRef: 0}},
			},
		},
	}
	if hints := preCheckPrincipalClaimUse(doc, view); len(hints) != 0 {
		t.Errorf("single-form relaxation should suppress the hint; got %v", hints)
	}
}

// TestPreCheckUncertaintyBlock_MissingWhenRequired
func TestPreCheckUncertaintyBlock_MissingWhenRequired(t *testing.T) {
	view := &types.AnswerSemanticView{
		UncertaintyRules: []types.UncertaintyRule{{TriggerFacet: ""}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
		},
	}
	hints := preCheckUncertaintyBlock(doc, view)
	if len(hints) != 1 {
		t.Fatalf("want 1 fix hint, got %d", len(hints))
	}
	if !strings.Contains(hints[0].Field, "caveat") {
		t.Errorf("hint should name caveat block; got %q", hints[0].Field)
	}
}

// TestPreCheckUncertaintyBlock_PresentSuppresses
func TestPreCheckUncertaintyBlock_PresentSuppresses(t *testing.T) {
	view := &types.AnswerSemanticView{
		UncertaintyRules: []types.UncertaintyRule{{TriggerFacet: ""}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
			{ID: "c1", Kind: types.BlockCaveat, Text: "what we did not search"},
		},
	}
	if hints := preCheckUncertaintyBlock(doc, view); len(hints) != 0 {
		t.Errorf("caveat present should suppress; got %v", hints)
	}
}

// TestPreCheckUncertaintyBlock_NoRules
func TestPreCheckUncertaintyBlock_NoRules(t *testing.T) {
	view := &types.AnswerSemanticView{}
	doc := &types.AnswerDocumentV2{}
	if hints := preCheckUncertaintyBlock(doc, view); len(hints) != 0 {
		t.Errorf("no rules → no hints; got %v", hints)
	}
}

// TestFormatEmitFixHints_RedlineAudit — pin the rejection envelope
// prose for R6 (no internal vocab leak) + R4 (generic) + LLM-facing
// purity (no third natural language).
func TestFormatEmitFixHints_RedlineAudit(t *testing.T) {
	hints := []emitFixHint{
		{Field: "blocks[].kind=ordered_list", ExpectedShape: "emit at least 1 block of kind=ordered_list", Reason: "enumeration"},
		{Field: "blocks[id=\"l1\"].claim_uses", ExpectedShape: "add a one-element claim_uses[]", Reason: "principal claim required"},
	}
	got := formatEmitFixHints(hints)

	// R6: must not leak internal vocab.
	for _, banned := range []string{
		"orchestrator", "ViolKind", "ViolationKind", "ClusterKey",
		"SuspectedRoot", "BuildRepairPlan", "RepairCluster",
		"AnswerDocumentV2", "FacetCoverage", "AnchoredCount",
		"DeclaredCount", "TaskGraph", "BusContext",
	} {
		if strings.Contains(got, banned) {
			t.Errorf("rejection prose must not leak internal token %q; got %q", banned, got)
		}
	}

	// No third natural language.
	for _, tok := range []string{"diese", "todos", "tutti", "cette", "это", "هذا"} {
		if strings.Contains(got, tok) {
			t.Errorf("rejection prose must be EN only; banned %q in %q", tok, got)
		}
	}

	// Each hint renders verbatim Field + ExpectedShape.
	for _, h := range hints {
		if !strings.Contains(got, h.Field) {
			t.Errorf("hint Field %q missing from output; got %q", h.Field, got)
		}
		if !strings.Contains(got, h.ExpectedShape) {
			t.Errorf("hint ExpectedShape %q missing from output; got %q", h.ExpectedShape, got)
		}
	}

	// Empty list → empty string.
	if got := formatEmitFixHints(nil); got != "" {
		t.Errorf("empty hints → empty string; got %q", got)
	}
}

// TestRunPreEmitChecks_AggregatesAcrossAllAxes — happy + fail mix.
// Pin that the four checks all wire through the top-level dispatcher.
func TestRunPreEmitChecks_AggregatesAcrossAllAxes(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockOrderedList, Required: true, MinCount: 1, AcceptableClaimForms: []types.ClaimForm{types.ClaimDefinitionFact, types.ClaimCallEdge}, SurfaceRoleHint: types.SurfacePrincipal},
		},
		UncertaintyRules: []types.UncertaintyRule{{TriggerFacet: ""}},
	}
	// Doc that fails 3 of 4 checks: missing ordered_list, missing
	// caveat, missing facet declarations (when we'd add facet
	// coverage), missing principal claim_use is moot because
	// ordered_list itself is missing.
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
		},
	}
	hints := runPreEmitChecks(doc, view, nil)
	if len(hints) < 2 {
		t.Errorf("want ≥2 fix hints (block coverage + uncertainty); got %d (%v)", len(hints), hints)
	}
}
