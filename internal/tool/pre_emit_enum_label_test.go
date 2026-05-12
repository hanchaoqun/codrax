// Package tool — pre_emit_enum_label_test.go (2026-05-10).
//
// P1 of the post-sweep optimization: preCheckEnumerationLabelGrounding
// catches LLM-emitted enumeration item labels whose leading
// identifier doesn't ground in the SymbolOracle, returning a
// fix hint that lets the LLM revise within the SAME dispatch
// instead of paying a full repair-loop round downstream.
//
// 70% of post-emit repair-loop violations in the 2026-05-10 sweep
// digest were enum-label hallucinations. Catching them at the
// chokepoint cuts repair cycles dramatically.
package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// stubOracle is a hand-rolled SymbolOracle for unit tests so we
// don't pull repomap (cycle).
type stubOracle struct {
	known map[string]int // name → minTier
}

func (s *stubOracle) SymbolExists(name string) (bool, int) {
	t, ok := s.known[name]
	return ok, t
}
func (s *stubOracle) SymbolExistsFlat(name string) (bool, int) {
	t, ok := s.known[name]
	return ok, t
}

// === preEmitLabelLeadingIdentifier ===

func TestPreEmitLabelLeadingIdentifier_Empty(t *testing.T) {
	if got := preEmitLabelLeadingIdentifier(""); got != "" {
		t.Errorf("empty: expected empty; got %q", got)
	}
}

func TestPreEmitLabelLeadingIdentifier_PureProse(t *testing.T) {
	// Starts with a digit — not identifier-shape.
	if got := preEmitLabelLeadingIdentifier("1. read evidence"); got != "" {
		t.Errorf("digit-leading: expected empty; got %q", got)
	}
}

func TestPreEmitLabelLeadingIdentifier_SimpleIdent(t *testing.T) {
	if got := preEmitLabelLeadingIdentifier("checkCoverage — verifies coverage"); got != "checkCoverage" {
		t.Errorf("simple ident: got %q, want checkCoverage", got)
	}
}

func TestPreEmitLabelLeadingIdentifier_DottedIdent(t *testing.T) {
	if got := preEmitLabelLeadingIdentifier("gate.Run — entry point"); got != "gate.Run" {
		t.Errorf("dotted ident: got %q", got)
	}
}

func TestPreEmitLabelLeadingIdentifier_StripsAfterPunct(t *testing.T) {
	if got := preEmitLabelLeadingIdentifier("buildAnalysisIR(): builds the IR"); got != "buildAnalysisIR" {
		t.Errorf("strips paren: got %q", got)
	}
}

// === preCheckEnumerationLabelGrounding ===

func TestPreCheckEnumLabel_NilOracle_NoOp(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "b1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "fabricatedFunction — does nothing"},
			}},
		},
	}
	if hints := preCheckEnumerationLabelGrounding(doc, nil); hints != nil {
		t.Errorf("nil oracle: expected no-op; got %v", hints)
	}
}

func TestPreCheckEnumLabel_AllGrounded_NoHints(t *testing.T) {
	oracle := &stubOracle{known: map[string]int{
		"checkCoverage":   1,
		"checkDAGClosure": 1,
	}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "b1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "checkCoverage — coverage check"},
				{Label: "checkDAGClosure — closure check"},
			}},
		},
	}
	if hints := preCheckEnumerationLabelGrounding(doc, oracle); hints != nil {
		t.Errorf("all grounded: expected no hints; got %v", hints)
	}
}

func TestPreCheckItemCitationAlignment_PrecedenceRoleLabelsAreNotSymbols(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "cmd/root.go", Line: 1430}},
		Blocks: []types.AnswerBlock{{
			ID:   "precedence_table",
			Kind: types.BlockTable,
			ClaimUses: []types.RenderedClaimUse{{
				ClaimForm: types.ClaimPrecedenceRole,
			}},
			Items: []types.AnswerBlockItem{{
				ID:          "override",
				Label:       "override",
				Text:        "CLI flag overrides config when explicitly changed.",
				CitationRef: 0,
			}},
		}},
	}
	view := &types.AnswerSemanticView{Family: types.QFConfigPrecedence}
	ctx := &types.BusContext{Mutable: types.NewMutableState("q")}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: []types.EvidenceItem{{
		ID:              "override",
		Source:          "cmd/root.go",
		LineStart:       1430,
		AnchorKind:      types.AnchorCondition,
		AnchorSymbol:    "cmd.Flags().Changed",
		GroundingStatus: types.GroundingGrounded,
	}}})

	if hints := preCheckItemCitationAlignment(doc, view, ctx); len(hints) != 0 {
		t.Fatalf("config precedence role labels are display roles, not symbol labels; got %+v", hints)
	}
}

func TestPreCheckItemCitationAlignment_SourceLocationLabelsMatchCitation(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/agent/analyzer.go", Line: 1903}},
		Blocks: []types.AnswerBlock{{
			ID:        "locations",
			Kind:      types.BlockOrderedList,
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimAssignmentFact}},
			Items: []types.AnswerBlockItem{{
				ID:          "loc",
				Label:       "internal/agent/analyzer.go:1903",
				Text:        "This location carries the requested assignment.",
				CitationRef: 0,
			}},
		}},
	}
	ctx := &types.BusContext{Mutable: types.NewMutableState("q")}
	if hints := preCheckItemCitationAlignment(doc, nil, ctx); len(hints) != 0 {
		t.Fatalf("source-location display labels should align to citation file:line, got %+v", hints)
	}
}

func TestPreCheckItemCitationAlignment_SourceLocationLabelsRejectCitationDrift(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/agent/analyzer.go", Line: 1904}},
		Blocks: []types.AnswerBlock{{
			ID:        "locations",
			Kind:      types.BlockOrderedList,
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimAssignmentFact}},
			Items: []types.AnswerBlockItem{{
				ID:          "loc",
				Label:       "internal/agent/analyzer.go:1903",
				Text:        "This location is one line away from the citation.",
				CitationRef: 0,
			}},
		}},
	}
	ctx := &types.BusContext{Mutable: types.NewMutableState("q")}
	hints := preCheckItemCitationAlignment(doc, nil, ctx)
	if len(hints) != 1 {
		t.Fatalf("source-location citation drift should produce one hint, got %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "source-location label") {
		t.Fatalf("hint should explain source-location alignment, got %+v", hints[0])
	}
}

func TestPreCheckEnumLabel_SourceLocationLabelSkipsSymbolOracle(t *testing.T) {
	oracle := &stubOracle{known: map[string]int{}}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "veryLongDirectory/foo.cpp", Line: 12}},
		Blocks: []types.AnswerBlock{{
			ID:   "locs",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:          "loc",
				Label:       "veryLongDirectory/foo.cpp:12",
				Text:        "The requested location.",
				CitationRef: 0,
			}},
		}},
	}
	if hints := preCheckEnumerationLabelGrounding(doc, oracle, &types.BusContext{}); len(hints) != 0 {
		t.Fatalf("source-location display labels should not be checked as symbols, got %+v", hints)
	}
}

func TestPreCheckItemCitationAlignment_ExternalObservationLabelsAreNotSymbols(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: ".codrax/logs/run.log", Line: 42}},
		Blocks: []types.AnswerBlock{{
			ID:   "observed_route",
			Kind: types.BlockTable,
			ClaimUses: []types.RenderedClaimUse{{
				ClaimForm: types.ClaimExternalObservation,
			}},
			Items: []types.AnswerBlockItem{{
				ID:          "route",
				Label:       "GET /api/v1/users",
				Text:        "runtime observation from the attached artifact",
				CitationRef: 0,
			}},
		}},
	}
	ctx := &types.BusContext{Mutable: types.NewMutableState("q")}
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID:              "route",
		Source:          ".codrax/logs/run.log",
		LineStart:       42,
		Origin:          types.ClaimOriginLog,
		SurfaceTerms:    []string{"GET /api/v1/users"},
		GroundingStatus: types.GroundingGrounded,
	}})

	if hints := preCheckItemCitationAlignment(doc, nil, ctx); len(hints) != 0 {
		t.Fatalf("external observation route/span labels are typed display labels, not source symbols; got %+v", hints)
	}
}

func TestPreCheckItemCitationAlignment_MacroLabelSnippetSatisfiesSymbolSurface(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "include/codrax_config.h", Line: 7}},
		Blocks: []types.AnswerBlock{{
			ID:   "macro_value",
			Kind: types.BlockTable,
			Items: []types.AnswerBlockItem{{
				ID:          "macro",
				Label:       "CODRAX_MAX_STEPS",
				Text:        "compile-time fallback value",
				CitationRef: 0,
			}},
		}},
	}
	ctx := &types.BusContext{Mutable: types.NewMutableState("q")}
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID:              "macro",
		Source:          "include/codrax_config.h",
		LineStart:       7,
		AnchorKind:      types.AnchorDefinition,
		Snippet:         "#define CODRAX_MAX_STEPS 50",
		GroundingStatus: types.GroundingGrounded,
	}})

	if hints := preCheckItemCitationAlignment(doc, nil, ctx); len(hints) != 0 {
		t.Fatalf("macro labels should keep symbol-like validation but pass via verbatim grounded snippet; got %+v", hints)
	}
}

func TestPreCheckEnumLabel_UserBucketLabel_NoHints(t *testing.T) {
	oracle := &stubOracle{known: map[string]int{}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "frames", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "ArkTS"},
				{Label: "仓颉帧"},
			}},
		},
	}
	ctx := &types.BusContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Buckets: []types.QuestionBucket{
					{Label: "ArkTS", Index: 1},
					{Label: "仓颉", Index: 2},
				},
			},
		},
	}
	if hints := preCheckEnumerationLabelGrounding(doc, oracle, ctx); hints != nil {
		t.Errorf("typed user bucket labels should bypass code-symbol oracle; got %v", hints)
	}
}

func TestPreCheckEnumLabel_RuntimeArtifactLabel_NoHints(t *testing.T) {
	oracle := &stubOracle{known: map[string]int{}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "frames", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "NativeBridge"},
			}},
		},
	}
	mut := types.NewMutableState("runtime label")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{
			Type: "Error",
			Frames: []types.LogFrame{{
				Func: "NativeBridge.invokeOhSum",
				Raw:  "at NativeBridge.invokeOhSum (NativeBridge.ets:33:11)",
			}},
		}},
	})
	ctx := &types.BusContext{Mutable: mut}
	if hints := preCheckEnumerationLabelGrounding(doc, oracle, ctx); hints != nil {
		t.Errorf("typed runtime artifact labels should bypass code-symbol oracle; got %v", hints)
	}
}

func TestPreCheckEnumLabel_PackageQualifiedLabelSupportedByCitedEvidence(t *testing.T) {
	oracle := &stubOracle{known: map[string]int{}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "chain",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:          "h1",
				Label:       "normalizer.Normalize",
				CitationRef: 0,
			}},
		}},
		Citations: []types.Citation{{File: "internal/agent/analyzer.go", Line: 1625}},
	}
	mut := types.NewMutableState("call chain")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		EvidenceItems: []types.EvidenceItem{{
			Kind:            types.EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       1625,
			AnchorKind:      types.AnchorCall,
			Subject:         "buildAnalysisIR",
			Object:          "normalizer.Normalize",
			GroundingStatus: types.GroundingGrounded,
		}},
	})
	ctx := &types.BusContext{Mutable: mut}
	if hints := preCheckEnumerationLabelGrounding(doc, oracle, ctx); hints != nil {
		t.Fatalf("package-qualified cited evidence label should bypass declaration oracle; got %v", hints)
	}
}

func TestPreCheckEnumLabel_QualifiedIdentitySkipsQualifierOracle(t *testing.T) {
	oracle := &stubOracle{known: map[string]int{}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "chain",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{
				{Label: "counterfactual.Expand"},
				{Label: "veryLongNamespace::Type"},
				{Label: "entry/src/main/ets/pages/Index.ets"},
				{Label: "@ohos.promptAction.showToast"},
			},
		}},
	}
	if hints := preCheckEnumerationLabelGrounding(doc, oracle); hints != nil {
		t.Fatalf("qualified code identities should be governed by evidence/citation gates, not qualifier symbol lookup; got %v", hints)
	}
}

func TestPreCheckItemCitationAlignment_RejectsAdjacentCallCitationDrift(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "chain",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:          "h9",
				Label:       "gate.RunWith",
				CitationRef: 0,
			}},
		}},
		Citations: []types.Citation{{File: "internal/agent/analyzer.go", Line: 1850}},
	}
	mut := types.NewMutableState("call chain")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceConditional,
				Source:          "internal/agent/analyzer.go",
				LineStart:       1850,
				AnchorKind:      types.AnchorCall,
				Subject:         "buildAnalysisIR",
				Object:          "counterfactual.Expand",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/agent/analyzer.go",
				LineStart:       1970,
				AnchorKind:      types.AnchorCall,
				Subject:         "buildAnalysisIR",
				Object:          "gate.RunWith",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	})
	ctx := &types.BusContext{Mutable: mut}
	hints := preCheckItemCitationAlignment(doc, nil, ctx)
	if len(hints) != 1 {
		t.Fatalf("expected one citation-alignment hint, got %d: %v", len(hints), hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "gate.RunWith") ||
		!strings.Contains(hints[0].ExpectedShape, "current_citation=internal/agent/analyzer.go:1850") ||
		!strings.Contains(hints[0].ExpectedShape, "candidate_citations=[internal/agent/analyzer.go:1970]") {
		t.Fatalf("hint should name invalid current cite and typed candidate line, got %+v", hints[0])
	}
	if strings.Contains(hints[0].ExpectedShape, "cite internal/agent/analyzer.go:1850") {
		t.Fatalf("hint must not present the current invalid citation as the target, got %+v", hints[0])
	}
}

func TestPreCheckItemCitationAlignment_DoesNotPresentCurrentCitationAsTarget(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "agents",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:          "a1",
				Label:       "RegisterDefaultSubAgents",
				CitationRef: 0,
			}},
		}},
		Citations: []types.Citation{{File: "internal/agent/sub_explorer.go", Line: 31}},
	}
	mut := types.NewMutableState("which agents can call subagents")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/agent/sub_explorer.go",
				LineStart:       31,
				AnchorKind:      types.AnchorReturn,
				Subject:         "SubExplorer.Name",
				Object:          `"explorer"`,
				AnchorSymbol:    "Name",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/agent/subagent.go",
				LineStart:       63,
				AnchorKind:      types.AnchorDefinition,
				Subject:         "RegisterDefaultSubAgents",
				AnchorSymbol:    "RegisterDefaultSubAgents",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	})
	ctx := &types.BusContext{Mutable: mut}

	hints := preCheckItemCitationAlignment(doc, nil, ctx)
	if len(hints) != 1 {
		t.Fatalf("expected one citation-alignment hint, got %d: %v", len(hints), hints)
	}
	hint := hints[0].ExpectedShape
	for _, want := range []string{
		"current_citation=internal/agent/sub_explorer.go:31",
		"current_citation is INVALID",
		"candidate_citations=[internal/agent/subagent.go:63]",
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("hint missing %q: %+v", want, hints[0])
		}
	}
	if strings.Contains(hint, "should cite internal/agent/sub_explorer.go:31") {
		t.Fatalf("hint must not make the invalid current citation look like the desired target: %+v", hints[0])
	}
}

func TestPreCheckCallChainItemCitationRoleAlignment_RejectsDefinitionForNamedCallEdge(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:       "hops",
			Kind:     types.BlockOrderedList,
			FacetIDs: []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{
				ClaimForm: types.ClaimCallEdge,
			}},
			Items: []types.AnswerBlockItem{{
				ID:          "h1",
				Label:       "buildAnalysisIR",
				Text:        "`ParseOutput` -> `buildAnalysisIR`",
				CitationRef: 0,
			}},
		}},
		Citations: []types.Citation{{File: "internal/agent/analyzer.go", Line: 1289}},
	}
	mut := types.NewMutableState("panic source")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			ID:              "call-parse-build",
			Kind:            types.EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       1039,
			AnchorKind:      types.AnchorCall,
			Subject:         "ParseOutput",
			Object:          "buildAnalysisIR",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			ID:              "def-build",
			Kind:            types.EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       1289,
			AnchorKind:      types.AnchorDefinition,
			Subject:         "buildAnalysisIR",
			AnchorSymbol:    "buildAnalysisIR",
			GroundingStatus: types.GroundingGrounded,
		},
	})
	ctx := &types.BusContext{Mutable: mut}

	hints := preCheckCallChainItemCitationRoleAlignment(doc, nil, ctx)
	if len(hints) != 1 {
		t.Fatalf("expected role-alignment hint, got %d: %+v", len(hints), hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "ParseOutput -> buildAnalysisIR") ||
		!strings.Contains(hints[0].ExpectedShape, "internal/agent/analyzer.go:1039") {
		t.Fatalf("hint should name the typed call edge and its citation line, got %+v", hints[0])
	}
}

func TestPreCheckCallChainItemCitationRoleAlignment_AcceptsMatchingCallEdge(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:       "hops",
			Kind:     types.BlockOrderedList,
			FacetIDs: []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{
				ClaimForm: types.ClaimCallEdge,
			}},
			Items: []types.AnswerBlockItem{{
				ID:          "h1",
				Label:       "buildAnalysisIR",
				Text:        "`ParseOutput` -> `buildAnalysisIR`",
				CitationRef: 0,
			}},
		}},
		Citations: []types.Citation{{File: "internal/agent/analyzer.go", Line: 1039}},
	}
	mut := types.NewMutableState("panic source")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "call-parse-build",
		Kind:            types.EvidenceDirect,
		Source:          "internal/agent/analyzer.go",
		LineStart:       1039,
		AnchorKind:      types.AnchorCall,
		Subject:         "ParseOutput",
		Object:          "buildAnalysisIR",
		GroundingStatus: types.GroundingGrounded,
	}})
	ctx := &types.BusContext{Mutable: mut}

	if hints := preCheckCallChainItemCitationRoleAlignment(doc, nil, ctx); len(hints) != 0 {
		t.Fatalf("matching call-edge citation should pass, got %+v", hints)
	}
}

func TestPreCheckCallChainItemCitationRoleAlignment_GeneralizesToImportEdge(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "imports",
			Kind: types.BlockBulletList,
			ClaimUses: []types.RenderedClaimUse{{
				ClaimForm: types.ClaimImportEdge,
			}},
			Items: []types.AnswerBlockItem{{
				ID:          "i1",
				Label:       "@kit.ArkUI",
				Text:        "`Index.ets` imports `@kit.ArkUI`",
				CitationRef: 0,
			}},
		}},
		Citations: []types.Citation{{File: "entry/src/main/ets/pages/Index.ets", Line: 20}},
	}
	mut := types.NewMutableState("imports")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			ID:              "import-arkui",
			Kind:            types.EvidenceDirect,
			Source:          "entry/src/main/ets/pages/Index.ets",
			LineStart:       4,
			AnchorKind:      types.AnchorImport,
			Subject:         "Index.ets",
			Object:          "@kit.ArkUI",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			ID:              "def-index",
			Kind:            types.EvidenceDirect,
			Source:          "entry/src/main/ets/pages/Index.ets",
			LineStart:       20,
			AnchorKind:      types.AnchorDefinition,
			Subject:         "Index",
			AnchorSymbol:    "Index",
			GroundingStatus: types.GroundingGrounded,
		},
	})
	ctx := &types.BusContext{Mutable: mut}

	hints := preCheckCallChainItemCitationRoleAlignment(doc, nil, ctx)
	if len(hints) != 1 {
		t.Fatalf("expected generic import-edge role hint, got %d: %+v", len(hints), hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "Index.ets -> @kit.ArkUI") ||
		!strings.Contains(hints[0].ExpectedShape, "entry/src/main/ets/pages/Index.ets:4") {
		t.Fatalf("hint should name typed import edge and its citation line, got %+v", hints[0])
	}
}

func TestPreCheckEnumLabel_AllHallucinated_OneFixHintPerBlock(t *testing.T) {
	oracle := &stubOracle{known: map[string]int{}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "hops", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "fabricatedFunction1 — fake"},
				{Label: "fabricatedFunction2 — also fake"},
			}},
		},
	}
	hints := preCheckEnumerationLabelGrounding(doc, oracle)
	if len(hints) != 1 {
		t.Fatalf("expected 1 fix hint per block; got %d", len(hints))
	}
	if !strings.Contains(hints[0].Field, "blocks[id=\"hops\"].items[].label") {
		t.Errorf("Field should include block id; got %q", hints[0].Field)
	}
	if !strings.Contains(hints[0].ExpectedShape, "fabricatedFunction1") {
		t.Errorf("ExpectedShape should list hallucinated names; got %q", hints[0].ExpectedShape)
	}
	if !strings.Contains(hints[0].ExpectedShape, "fabricatedFunction2") {
		t.Errorf("ExpectedShape should list both hallucinated names; got %q", hints[0].ExpectedShape)
	}
}

func TestPreCheckEnumLabel_BelowLengthFloor_Skipped(t *testing.T) {
	// short identifier (< 10 chars) → treated as stdlib helper, skipped.
	oracle := &stubOracle{known: map[string]int{}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "b1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "Sprintf — formats string"},
				{Label: "New — constructor"},
			}},
		},
	}
	if hints := preCheckEnumerationLabelGrounding(doc, oracle); hints != nil {
		t.Errorf("short identifiers below floor: expected no hints; got %v", hints)
	}
}

func TestPreCheckEnumLabel_ProseLabel_Skipped(t *testing.T) {
	// Prose label like "Step 1: read evidence" doesn't extract an
	// identifier-shape leading token.
	oracle := &stubOracle{known: map[string]int{}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "b1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "1. Step prose only"},
			}},
		},
	}
	if hints := preCheckEnumerationLabelGrounding(doc, oracle); hints != nil {
		t.Errorf("prose label: expected no hints; got %v", hints)
	}
}

func TestPreCheckEnumLabel_NonListBlocks_Skipped(t *testing.T) {
	// Diagram / section / scalar blocks have their own oracle paths;
	// the enum-label gate only fires on ordered_list / bullet_list /
	// table.
	oracle := &stubOracle{known: map[string]int{}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "diag", Kind: types.BlockDiagram, Items: []types.AnswerBlockItem{
				{Label: "fabricatedNodeName"},
			}},
			{ID: "sect", Kind: types.BlockSection, Items: []types.AnswerBlockItem{
				{Label: "fabricatedSection"},
			}},
		},
	}
	if hints := preCheckEnumerationLabelGrounding(doc, oracle); hints != nil {
		t.Errorf("non-list block kinds: expected no hints; got %v", hints)
	}
}

func TestPreCheckEnumLabel_PartialHallucination_OnlyHallucinatedReported(t *testing.T) {
	// Mixed: one real + one fake → only the fake appears in the fix
	// hint's ExpectedShape.
	oracle := &stubOracle{known: map[string]int{
		"checkCoverage": 1,
	}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "hops", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "checkCoverage — real"},
				{Label: "fabricatedFunction — fake"},
			}},
		},
	}
	hints := preCheckEnumerationLabelGrounding(doc, oracle)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint; got %d", len(hints))
	}
	if strings.Contains(hints[0].ExpectedShape, "checkCoverage") {
		t.Errorf("real identifier should NOT appear in hint; got %q", hints[0].ExpectedShape)
	}
	if !strings.Contains(hints[0].ExpectedShape, "fabricatedFunction") {
		t.Errorf("hallucinated identifier missing; got %q", hints[0].ExpectedShape)
	}
}

func TestPreCheckEnumLabel_LowConfidenceTier_TreatedAsHallucinated(t *testing.T) {
	// Tier ≥ 3 = low-confidence parse, treated same as not-found.
	oracle := &stubOracle{known: map[string]int{
		"lowConfidenceMatch": 3,
	}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "b1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "lowConfidenceMatch — questionable"},
			}},
		},
	}
	hints := preCheckEnumerationLabelGrounding(doc, oracle)
	if len(hints) != 1 {
		t.Errorf("tier ≥ 3 should be flagged as hallucinated; got %d hints", len(hints))
	}
}

func TestPreCheckEnumLabel_DedupHallucinatedIdent(t *testing.T) {
	// Same hallucinated ident appears twice in items — fix hint
	// should list it only once.
	oracle := &stubOracle{known: map[string]int{}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "b1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "fabricatedFunction — first"},
				{Label: "fabricatedFunction — duplicate"},
			}},
		},
	}
	hints := preCheckEnumerationLabelGrounding(doc, oracle)
	if len(hints) != 1 {
		t.Fatalf("got %d hints", len(hints))
	}
	occurrences := strings.Count(hints[0].ExpectedShape, "fabricatedFunction")
	if occurrences != 1 {
		t.Errorf("dedup expected 1 occurrence; got %d in %q", occurrences, hints[0].ExpectedShape)
	}
}

// === Integration: runPreEmitChecks fires the new check ===

func TestRunPreEmitChecks_EnumLabelHallucination_FiresAlongsideOtherChecks(t *testing.T) {
	oracle := &stubOracle{known: map[string]int{}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "hops", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "fabricatedFunctionName — fake"},
			}},
		},
	}
	view := &types.AnswerSemanticView{
		// No required blocks → other checks pass; only enum-label
		// gate should fire.
	}
	hints := runPreEmitChecks(doc, view, oracle)
	if len(hints) == 0 {
		t.Fatal("expected at least one hint from enum-label gate")
	}
	found := false
	for _, h := range hints {
		if strings.Contains(h.Field, "items[].label") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("enum-label hint missing from runPreEmitChecks output; got %v", hints)
	}
}
