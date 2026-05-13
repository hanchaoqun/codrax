package types

import "testing"

// ── B2-T3 7 family compile tests (≥3 case per family = 21+) ───────────

// helper: minimal IR that resolves to the named family.
func irForRootCauseTrace() *AnalysisIR {
	return &AnalysisIR{
		RequestModel: RequestModel{
			Intent:    IntentRootCause,
			Scenario:  ScenarioGeneric,
			LogTriage: &LogBundle{},
		},
	}
}

func irForConfigPrecedence() *AnalysisIR {
	return &AnalysisIR{
		RequestModel: RequestModel{
			Intent:   IntentConfigQuery,
			Scenario: ScenarioGeneric,
		},
	}
}

func irForRoleLookup() *AnalysisIR {
	return &AnalysisIR{
		RequestModel: RequestModel{
			Intent:        IntentExplain,
			Scenario:      ScenarioGeneric,
			AnswerSubject: AnswerSubject{Kind: SubjectFunctionName, EntityAxes: []string{"foo"}, Confidence: 0.9},
		},
	}
}

func irForCallChain() *AnalysisIR {
	return &AnalysisIR{
		RequestModel: RequestModel{
			Intent:   IntentTrace,
			Scenario: ScenarioGeneric,
		},
	}
}

func irForEnumeration() *AnalysisIR {
	return &AnalysisIR{
		RequestModel: RequestModel{
			Intent:   IntentEnumerate,
			Scenario: ScenarioGeneric,
		},
	}
}

func irForArchitecture() *AnalysisIR {
	return &AnalysisIR{
		RequestModel: RequestModel{
			Intent:   IntentExplain,
			Scenario: ScenarioArchitectureExplain,
		},
	}
}

func irForGeneric() *AnalysisIR {
	return &AnalysisIR{
		RequestModel: RequestModel{
			Intent:   IntentExplain,
			Scenario: ScenarioGeneric,
		},
	}
}

func irForComparison() *AnalysisIR {
	return &AnalysisIR{
		RequestModel: RequestModel{
			Intent:   IntentExplain,
			Scenario: ScenarioGeneric,
			Buckets: []QuestionBucket{
				{Label: "read mode", Index: 1},
				{Label: "write mode", Index: 2},
			},
		},
	}
}

// ── QFRootCauseTrace 3 cases ───────────────────────────────────────

func TestCompileRootCauseTrace_ResolvesFamily(t *testing.T) {
	view := BuildAnswerSemanticView(irForRootCauseTrace(), nil)
	if view == nil {
		t.Fatal("view nil")
	}
	if view.Family != QFRootCauseTrace {
		t.Errorf("expected QFRootCauseTrace; got %s", view.Family)
	}
}

func TestCompileRootCauseTrace_HasSummaryAndOrderedListRequired(t *testing.T) {
	view := BuildAnswerSemanticView(irForRootCauseTrace(), nil)
	hasSummary, hasList := false, false
	for _, b := range view.RequiredBlocks {
		if b.Kind == BlockSummary && b.Required && b.MinCount >= 1 {
			hasSummary = true
		}
		if b.Kind == BlockOrderedList && b.Required {
			hasList = true
		}
	}
	if !hasSummary {
		t.Error("missing required BlockSummary with MinCount>=1")
	}
	if !hasList {
		t.Error("missing required BlockOrderedList for cause chain")
	}
}

func TestCompileRootCauseTrace_ExternalOnlyPrincipalListUsesObservedArtifact(t *testing.T) {
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Intent: IntentRootCause,
			LogTriage: &LogBundle{
				Errors: []LogError{{Type: "panic", Frames: []LogFrame{{
					File: "src/bridge/Bridge.cj",
					Line: 18,
					Func: "demo.bridge.ohSum",
				}}}},
			},
		},
	}
	plan := &AnswerSurfacePlan{
		RuntimeGroundingDisposition: SystemRuntimeGroundingDisposition(ir.RequestModel.LogTriage, nil),
	}
	view := BuildAnswerSemanticView(ir, plan)
	var list *BlockRequirement
	for i := range view.RequiredBlocks {
		if view.RequiredBlocks[i].Kind == BlockOrderedList {
			list = &view.RequiredBlocks[i]
			break
		}
	}
	if list == nil {
		t.Fatal("root-cause external-only answer still needs a principal observation list")
	}
	if !containsString(list.FacetIDs, string(FacetObservedArtifactFact)) {
		t.Fatalf("external-only list facet_ids = %v, want observed_artifact_fact", list.FacetIDs)
	}
	if containsString(list.FacetIDs, string(FacetCurrentCodePath)) {
		t.Fatalf("external-only artifact list must not require current_code_path: %v", list.FacetIDs)
	}
	if !containsClaimForm(list.AcceptableClaimForms, ClaimExternalObservation) {
		t.Fatalf("external-only list must accept external_observation claims: %+v", list.AcceptableClaimForms)
	}
}

func TestCompileRootCauseTrace_CurrentStatusDiagnosticRequiresDecisionBlock(t *testing.T) {
	ir := irForRootCauseTrace()
	ir.AnswerContract.CurrentStatusDiagnostic = &CurrentStatusDiagnosticContract{
		Required: true,
		AllowedVerdicts: []CurrentStatusVerdict{
			CurrentStatusStillPresent,
			CurrentStatusFixed,
			CurrentStatusNotEnoughEvidence,
		},
	}
	view := BuildAnswerSemanticView(ir, nil)
	if view == nil {
		t.Fatal("view nil")
	}
	if view.CurrentStatusDiagnostic == nil || !view.CurrentStatusDiagnostic.Required {
		t.Fatalf("current status diagnostic contract missing from view: %+v", view.CurrentStatusDiagnostic)
	}
	if !containsCurrentStatusVerdict(view.CurrentStatusDiagnostic.AllowedVerdicts, CurrentStatusStillPresent) ||
		!containsCurrentStatusVerdict(view.CurrentStatusDiagnostic.AllowedVerdicts, CurrentStatusFixed) ||
		!containsCurrentStatusVerdict(view.CurrentStatusDiagnostic.AllowedVerdicts, CurrentStatusNotEnoughEvidence) {
		t.Fatalf("current status verdict set missing: %+v", view.CurrentStatusDiagnostic.AllowedVerdicts)
	}
	for _, req := range view.RequiredBlocks {
		if req.Kind != BlockDecision {
			continue
		}
		if !req.Required || req.MinCount != 1 || req.MaxCount != 1 {
			t.Fatalf("decision block cardinality wrong: %+v", req)
		}
		if !containsClaimForm(req.AcceptableClaimForms, ClaimAbsenceFact) {
			t.Fatalf("decision block should allow absence verdict support: %+v", req.AcceptableClaimForms)
		}
		return
	}
	t.Fatalf("current-status diagnostic did not require BlockDecision: %+v", view.RequiredBlocks)
}

func containsCurrentStatusVerdict(in []CurrentStatusVerdict, want CurrentStatusVerdict) bool {
	for _, got := range in {
		if got == want {
			return true
		}
	}
	return false
}

func TestCompileRootCauseTrace_HasUncertaintyRule(t *testing.T) {
	view := BuildAnswerSemanticView(irForRootCauseTrace(), nil)
	if len(view.UncertaintyRules) == 0 {
		t.Error("expected at least one UncertaintyRule for log-source drift")
	}
}

func TestCompileRootCauseTrace_DemotesNearestMechanismWhenOnlyGuardEvidenceExists(t *testing.T) {
	plan := &AnswerSurfacePlan{
		FacetCoverage: &FacetCoverageContract{
			Family: QFRootCauseTrace,
			Required: []FacetRequirement{
				{
					Kind:            FacetNearestMechanism,
					Required:        FacetSoftRequired,
					AcceptableForms: []ClaimForm{ClaimDefinitionFact, ClaimCallEdge},
					SourceCandidate: []string{"ev-1"},
					Tier:            TierExpected,
					PromotionPolicy: PromotionWhenEvidenceSufficient,
				},
			},
			Optional: []FacetRequirement{},
		},
		DriftBoundedSurfaceItems: []EvidenceItem{
			{
				Kind:         EvidenceConditional,
				Source:       "internal/agent/analyzer.go",
				LineStart:    978,
				AnchorKind:   AnchorCondition,
				AnchorSymbol: "buildAnalysisIR",
				Snippet:      "if ctx == nil || ctx.Mutable == nil {",
			},
		},
	}
	view := BuildAnswerSemanticView(irForRootCauseTrace(), plan)
	if view == nil || view.FacetCoverage == nil {
		t.Fatal("view/facet coverage nil")
	}
	var got *FacetRequirement
	for i := range view.FacetCoverage.Required {
		if view.FacetCoverage.Required[i].Kind == FacetNearestMechanism {
			got = &view.FacetCoverage.Required[i]
			break
		}
	}
	if got == nil {
		t.Fatal("nearest_mechanism facet missing")
	}
	if got.Required != FacetOptional {
		t.Fatalf("nearest_mechanism requiredness = %q, want optional when only a lone guard is grounded", got.Required)
	}
	if got.EffectivePromotionPolicy() != PromotionAdvisoryOnly {
		t.Fatalf("nearest_mechanism promotion policy = %q, want advisory_only", got.EffectivePromotionPolicy())
	}
	if got.IsPromoted() {
		t.Fatal("nearest_mechanism should not stay promoted in guard-only drift mode")
	}
}

// ── QFConfigPrecedence 3 cases ─────────────────────────────────────

func TestCompileConfigPrecedence_ResolvesFamily(t *testing.T) {
	view := BuildAnswerSemanticView(irForConfigPrecedence(), nil)
	if view == nil || view.Family != QFConfigPrecedence {
		t.Errorf("expected QFConfigPrecedence; got %v", view)
	}
}

func TestCompileConfigPrecedence_HasSummaryAndScalarRequired(t *testing.T) {
	view := BuildAnswerSemanticView(irForConfigPrecedence(), nil)
	hasSummary, hasScalar := false, false
	for _, b := range view.RequiredBlocks {
		if b.Kind == BlockSummary {
			hasSummary = true
		}
		if b.Kind == BlockScalar && b.Required {
			hasScalar = true
		}
	}
	if !hasSummary {
		t.Error("missing required Summary")
	}
	if !hasScalar {
		t.Error("missing required Scalar for resolved value")
	}
}

func TestCompileConfigPrecedence_HasOptionalTableForLayers(t *testing.T) {
	view := BuildAnswerSemanticView(irForConfigPrecedence(), nil)
	hasTable := false
	for _, b := range view.OptionalBlocks {
		if b.Kind == BlockTable {
			hasTable = true
		}
	}
	if !hasTable {
		t.Error("expected BlockTable in OptionalBlocks for layer-by-key precedence rendering")
	}
}

func TestCompileConfigPrecedence_ExactAbsenceBecomesSummaryLed(t *testing.T) {
	ir := irForConfigPrecedence()
	ir.AnswerContract.ExactResolution = &ExactResolutionContract{
		TargetKind:   SubjectConfigKey,
		Targets:      []string{"explore_mid_loop_hint_budget"},
		AllowAbsence: true,
	}
	plan := &AnswerSurfacePlan{
		PreferredExactResolution: &AnswerExactResolution{
			Status:      AnswerExactResolutionAbsent,
			ContextMode: AnswerExactResolutionContextGroundedOnly,
		},
	}
	view := BuildAnswerSemanticView(ir, plan)
	if view == nil {
		t.Fatal("view nil")
	}
	hasRequiredScalar := false
	var summary *BlockRequirement
	for i := range view.RequiredBlocks {
		req := &view.RequiredBlocks[i]
		if req.Kind == BlockScalar && req.Required {
			hasRequiredScalar = true
		}
		if req.Kind == BlockSummary && req.Required {
			summary = req
		}
	}
	if hasRequiredScalar {
		t.Fatal("exact-absence config precedence must not require a principal scalar")
	}
	if summary == nil {
		t.Fatal("required summary block missing")
	}
	if !containsString(summary.FacetIDs, string(FacetResolvedLiteralOrSymbol)) {
		t.Fatalf("summary block must carry resolved_literal_or_symbol in exact-absence mode: %+v", summary.FacetIDs)
	}
	if len(summary.AcceptableClaimForms) == 0 || summary.AcceptableClaimForms[0] != ClaimAbsenceFact && !containsClaimForm(summary.AcceptableClaimForms, ClaimAbsenceFact) {
		t.Fatalf("summary block must accept absence_fact in exact-absence mode: %+v", summary.AcceptableClaimForms)
	}
}

// ── QFRoleLookup 3 cases ───────────────────────────────────────────

func TestCompileRoleLookup_ResolvesFamily(t *testing.T) {
	view := BuildAnswerSemanticView(irForRoleLookup(), nil)
	if view == nil || view.Family != QFRoleLookup {
		t.Errorf("expected QFRoleLookup; got %v", view)
	}
}

func TestCompileRoleLookup_HasScalarRequired(t *testing.T) {
	view := BuildAnswerSemanticView(irForRoleLookup(), nil)
	hasScalar := false
	for _, b := range view.RequiredBlocks {
		if b.Kind == BlockScalar && b.Required {
			hasScalar = true
		}
	}
	if !hasScalar {
		t.Error("role lookup must require BlockScalar for the resolved literal")
	}
}

func TestCompileRoleLookup_CurrentCodePathHasSectionPlacement(t *testing.T) {
	view := BuildAnswerSemanticView(irForRoleLookup(), &AnswerSurfacePlan{
		FacetCoverage: &FacetCoverageContract{
			Family: QFRoleLookup,
			Required: []FacetRequirement{
				{Kind: FacetCurrentCodePath, Required: FacetSoftRequired, SourceCandidate: []string{"call"}},
			},
		},
	})
	if view == nil {
		t.Fatal("view nil")
	}
	for _, block := range view.OptionalBlocks {
		if block.Kind != BlockSection {
			continue
		}
		if containsString(block.FacetIDs, string(FacetCurrentCodePath)) {
			return
		}
	}
	t.Fatalf("role lookup current_code_path should have a section placement hint: %+v", view.OptionalBlocks)
}

func TestCompileRoleLookup_HasNoDiagram(t *testing.T) {
	view := BuildAnswerSemanticView(irForRoleLookup(), nil)
	if view.DiagramPlan != nil && view.DiagramPlan.Required {
		t.Error("role lookup should NOT require a diagram (single-literal answer)")
	}
}

func TestCompileRoleLookup_ExactAbsenceBecomesSummaryLed(t *testing.T) {
	ir := irForRoleLookup()
	ir.AnswerContract.ExactResolution = &ExactResolutionContract{
		TargetKind:   SubjectFunctionName,
		Targets:      []string{"ParseUserRequestToIR"},
		AllowAbsence: true,
	}
	plan := &AnswerSurfacePlan{
		PreferredExactResolution: &AnswerExactResolution{
			Status:      AnswerExactResolutionAbsent,
			ContextMode: AnswerExactResolutionContextGroundedOnly,
		},
	}
	view := BuildAnswerSemanticView(ir, plan)
	if view == nil {
		t.Fatal("view nil")
	}
	for _, req := range view.RequiredBlocks {
		if req.Kind == BlockScalar && req.Required {
			t.Fatal("exact-absence role lookup must not require a principal scalar")
		}
	}
	var summary *BlockRequirement
	for i := range view.RequiredBlocks {
		if view.RequiredBlocks[i].Kind == BlockSummary && view.RequiredBlocks[i].Required {
			summary = &view.RequiredBlocks[i]
			break
		}
	}
	if summary == nil {
		t.Fatal("required summary block missing")
	}
	if !containsString(summary.FacetIDs, string(FacetResolvedLiteralOrSymbol)) {
		t.Fatalf("summary block must carry resolved_literal_or_symbol in exact-absence mode: %+v", summary.FacetIDs)
	}
	if !containsClaimForm(summary.AcceptableClaimForms, ClaimAbsenceFact) {
		t.Fatalf("summary block must accept absence_fact in exact-absence mode: %+v", summary.AcceptableClaimForms)
	}
}

// ── QFCallChain 3 cases ────────────────────────────────────────────

func TestCompileCallChain_ResolvesFamily(t *testing.T) {
	view := BuildAnswerSemanticView(irForCallChain(), nil)
	if view == nil || view.Family != QFCallChain {
		t.Errorf("expected QFCallChain; got %v", view)
	}
}

func TestCompileCallChain_HasOrderedListRequiredAndDiagramOptionalByDefault(t *testing.T) {
	view := BuildAnswerSemanticView(irForCallChain(), nil)
	hasList, hasRequiredDiagram := false, false
	for _, b := range view.RequiredBlocks {
		if b.Kind == BlockOrderedList && b.Required {
			hasList = true
		}
		if b.Kind == BlockDiagram && b.Required {
			hasRequiredDiagram = true
		}
	}
	if !hasList {
		t.Error("call chain must require ordered list of hops")
	}
	if hasRequiredDiagram {
		t.Error("call chain must not require diagram unless the user requested one")
	}
}

func TestCompileCallChain_UserDiagramContractRequiresDiagram(t *testing.T) {
	view := BuildAnswerSemanticView(irForCallChain(), planRequiringDiagram())
	hasDiagram := false
	for _, b := range view.RequiredBlocks {
		if b.Kind == BlockDiagram && b.Required {
			hasDiagram = true
		}
	}
	if !hasDiagram {
		t.Error("explicit diagram contract should require call-chain diagram block")
	}
}

func TestCompileCallChain_HasNoUpperBoundOnHops(t *testing.T) {
	view := BuildAnswerSemanticView(irForCallChain(), nil)
	for _, b := range view.RequiredBlocks {
		if b.Kind == BlockOrderedList && b.MaxCount != 0 {
			t.Errorf("call chain hop list must not assume max hop count; got MaxCount=%d", b.MaxCount)
		}
	}
}

// ── QFEnumeration 3 cases ──────────────────────────────────────────

func TestCompileEnumeration_ResolvesFamily(t *testing.T) {
	view := BuildAnswerSemanticView(irForEnumeration(), nil)
	if view == nil || view.Family != QFEnumeration {
		t.Errorf("expected QFEnumeration; got %v", view)
	}
}

func TestCompileEnumeration_HasOrderedListRequired(t *testing.T) {
	view := BuildAnswerSemanticView(irForEnumeration(), nil)
	hasList := false
	for _, b := range view.RequiredBlocks {
		if b.Kind == BlockOrderedList && b.Required {
			hasList = true
		}
	}
	if !hasList {
		t.Error("enumeration must require ordered/bullet list")
	}
}

func TestCompileEnumeration_HasOptionalBucketSection(t *testing.T) {
	view := BuildAnswerSemanticView(irForEnumeration(), nil)
	hasSection := false
	for _, b := range view.OptionalBlocks {
		if b.Kind == BlockSection {
			hasSection = true
		}
	}
	if !hasSection {
		t.Error("enumeration must offer optional Section for user-named buckets")
	}
}

func TestCompileEnumeration_OutranksFunctionSubjectForCategoryEnumeration(t *testing.T) {
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Intent: IntentEnumerate,
			AnswerSubject: AnswerSubject{
				Kind:       SubjectFunctionName,
				EntityAxes: []string{"Agent -> SubAgent"},
				Confidence: 0.9,
			},
			Predicates: SemanticPredicates{
				IsCategoryEnumeration: true,
				IsRelationalLookup:    true,
			},
		},
	}
	view := BuildAnswerSemanticView(ir, nil)
	if view == nil {
		t.Fatal("view nil")
	}
	if view.Family != QFEnumeration {
		t.Fatalf("family=%s, want enumeration; function-like answer_subject must not steal set-valued requests", view.Family)
	}
	for _, req := range view.RequiredBlocks {
		if req.Kind == BlockScalar {
			t.Fatalf("category enumeration must not require scalar block: %+v", view.RequiredBlocks)
		}
	}
	if !view.NeedsOrderedPrincipalList() {
		t.Fatalf("category enumeration should still require a principal member list")
	}
}

func TestCompileEnumeration_PrincipalListAllowsAllFacetPrincipalForms(t *testing.T) {
	view := BuildAnswerSemanticView(irForEnumeration(), nil)
	if view == nil {
		t.Fatal("view nil")
	}
	var list *BlockRequirement
	for i := range view.RequiredBlocks {
		if view.RequiredBlocks[i].Kind == BlockOrderedList {
			list = &view.RequiredBlocks[i]
			break
		}
	}
	if list == nil {
		t.Fatal("enumeration ordered-list requirement missing")
	}
	for _, want := range []ClaimForm{
		ClaimDefinitionFact,
		ClaimAssignmentFact,
		ClaimReturnFact,
		ClaimImportEdge,
		ClaimCallEdge,
	} {
		if !containsClaimForm(list.AcceptableClaimForms, want) {
			t.Fatalf("enumeration principal list missing acceptable claim form %s: %+v", want, list.AcceptableClaimForms)
		}
	}
}

func containsClaimForm(items []ClaimForm, want ClaimForm) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestCompileEnumeration_NoMaxBucketAssumption(t *testing.T) {
	view := BuildAnswerSemanticView(irForEnumeration(), nil)
	for _, b := range view.OptionalBlocks {
		if b.Kind == BlockSection && b.MaxCount != 0 {
			t.Errorf("enumeration bucket section must not assume max bucket count; got MaxCount=%d", b.MaxCount)
		}
	}
}

// ── QFArchitecture 3 cases ─────────────────────────────────────────

func TestCompileArchitecture_ResolvesFamily(t *testing.T) {
	view := BuildAnswerSemanticView(irForArchitecture(), nil)
	if view == nil || view.Family != QFArchitecture {
		t.Errorf("expected QFArchitecture; got %v", view)
	}
}

func TestCompileArchitecture_HasSectionRequiredAndDiagramOptionalByDefault(t *testing.T) {
	view := BuildAnswerSemanticView(irForArchitecture(), nil)
	hasSection, hasDiagram := false, false
	for _, b := range view.RequiredBlocks {
		if b.Kind == BlockSection && b.Required {
			hasSection = true
		}
		if b.Kind == BlockDiagram && b.Required {
			hasDiagram = true
		}
	}
	if !hasSection {
		t.Error("architecture must require BlockSection for layers")
	}
	if hasDiagram {
		t.Error("architecture must not require BlockDiagram unless the user requested one")
	}
}

func TestCompileArchitecture_UserDiagramContractRequiresDiagram(t *testing.T) {
	view := BuildAnswerSemanticView(irForArchitecture(), planRequiringDiagram())
	hasDiagram := false
	for _, b := range view.RequiredBlocks {
		if b.Kind == BlockDiagram && b.Required {
			hasDiagram = true
		}
	}
	if !hasDiagram {
		t.Error("explicit diagram contract should require BlockDiagram")
	}
}

func TestCompileArchitecture_NoMaxLayerAssumption(t *testing.T) {
	view := BuildAnswerSemanticView(irForArchitecture(), nil)
	for _, b := range view.RequiredBlocks {
		if b.Kind == BlockSection && b.MaxCount != 0 {
			t.Errorf("architecture layer section must not assume max layer count; got MaxCount=%d", b.MaxCount)
		}
	}
}

func TestCompileArchitecture_AllowsOrderedPipelineEnrichment(t *testing.T) {
	view := BuildAnswerSemanticView(irForArchitecture(), nil)
	for _, b := range view.OptionalBlocks {
		if b.Kind == BlockOrderedList {
			return
		}
	}
	t.Fatal("architecture answers should allow an optional ordered list for grounded pipelines / handoffs")
}

// ── QFGeneric 3 cases ──────────────────────────────────────────────

func TestCompileGeneric_ResolvesFamily(t *testing.T) {
	view := BuildAnswerSemanticView(irForGeneric(), nil)
	if view == nil || view.Family != QFGeneric {
		t.Errorf("expected QFGeneric; got %v", view)
	}
}

func TestCompileGeneric_OnlySummaryRequired(t *testing.T) {
	view := BuildAnswerSemanticView(irForGeneric(), nil)
	if len(view.RequiredBlocks) != 1 {
		t.Errorf("generic must require exactly 1 block (summary); got %d", len(view.RequiredBlocks))
	}
	if view.RequiredBlocks[0].Kind != BlockSummary {
		t.Errorf("generic's only required block must be Summary; got %s", view.RequiredBlocks[0].Kind)
	}
}

func TestCompileGeneric_NoStructuralAssumptions(t *testing.T) {
	view := BuildAnswerSemanticView(irForGeneric(), nil)
	// Generic should never carry FacetCoverage / DiagramPlan unless
	// the upstream surface plan supplies them (which our minimal IR
	// does not).
	if view.DiagramPlan != nil && view.DiagramPlan.Required {
		t.Error("generic must not require diagram by default")
	}
	for _, b := range view.OptionalBlocks {
		if b.MaxCount != 0 && b.Kind != BlockDiagram {
			t.Errorf("generic optional block %s should not assume MaxCount; got %d", b.Kind, b.MaxCount)
		}
	}
}

// ── R4.4 QFComparison family compile tests ─────────────────────

// TestCompileComparison_ResolvesFamily covers the basic family
// dispatch — 2 buckets → QFComparison + non-empty RequiredBlocks.
func TestCompileComparison_ResolvesFamily(t *testing.T) {
	view := BuildAnswerSemanticView(irForComparison(), nil)
	if view == nil {
		t.Fatal("nil view")
	}
	if view.Family != QFComparison {
		t.Errorf("Family = %q, want QFComparison", view.Family)
	}
	if len(view.RequiredBlocks) == 0 {
		t.Error("RequiredBlocks empty")
	}
}

// TestCompileComparison_BucketSectionCountPinned is the
// **R4.4 load-bearing test**: bucket count → BlockSection
// MinCount/MaxCount must match exactly so the answer carries
// one section per user-named bucket (no more, no less).
func TestCompileComparison_BucketSectionCountPinned(t *testing.T) {
	cases := []int{2, 3, 4}
	for _, n := range cases {
		buckets := make([]QuestionBucket, n)
		for i := range buckets {
			buckets[i] = QuestionBucket{Label: "bucket" + string(rune('A'+i)), Index: i + 1}
		}
		ir := &AnalysisIR{
			RequestModel: RequestModel{
				Intent:  IntentExplain,
				Buckets: buckets,
			},
		}
		view := BuildAnswerSemanticView(ir, nil)
		if view == nil {
			t.Fatal("nil view")
		}
		hasSection := false
		for _, b := range view.RequiredBlocks {
			if b.Kind != BlockSection {
				continue
			}
			hasSection = true
			if b.MinCount != n || b.MaxCount != n {
				t.Errorf("buckets=%d: section MinCount=%d MaxCount=%d, want both %d",
					n, b.MinCount, b.MaxCount, n)
			}
		}
		if !hasSection {
			t.Errorf("buckets=%d: no Section block in RequiredBlocks", n)
		}
	}
}

// TestCompileComparison_FacetBucketLabelHardPresent pins the
// FacetCoverageContract template carries a HARD FacetBucketLabel
// entry (the bucket-alignment signal). commonFacets() emits this
// when Buckets >= 2, so QFComparison piggybacks; the entry is the
// gate-side signal that every bucket Label must appear verbatim
// in the rendered answer.
//
// Read directly from familyTemplate (the FacetCoverageContract
// builder) since BuildAnswerSemanticView with nil plan doesn't
// populate view.FacetCoverage.
func TestCompileComparison_FacetBucketLabelHardPresent(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		Buckets: []QuestionBucket{
			{Label: "read mode", Index: 1},
			{Label: "write mode", Index: 2},
		},
	}
	template := familyTemplate(QFComparison, rm)
	hasBucketLabelHard := false
	for _, req := range template {
		if req.Kind == FacetBucketLabel && req.Required == FacetHardRequired {
			hasBucketLabelHard = true
			break
		}
	}
	if !hasBucketLabelHard {
		t.Error("QFComparison template missing FacetBucketLabel HARD entry (commonFacets should append one when Buckets>=2)")
	}
}

// TestCompileComparison_NoRequiredDiagram pins the design choice:
// comparisons are prose-led;diagram is NOT required.
func TestCompileComparison_NoRequiredDiagram(t *testing.T) {
	view := BuildAnswerSemanticView(irForComparison(), nil)
	if view.DiagramPlan != nil && view.DiagramPlan.Required {
		t.Error("comparison must NOT require diagram (prose-led)")
	}
	for _, b := range view.RequiredBlocks {
		if b.Kind == BlockDiagram {
			t.Error("comparison must NOT have a Required BlockDiagram")
		}
	}
}

// ── Phase 3-C4: EdgeRelations contract per family (8 family lock) ───

// TestDefaultEdgeRelationsForKind_AllKinds locks the SST mapping
// from DiagramKind to default typed edge contracts. New diagram
// kinds MUST extend this switch — the test catches drift.
func TestDefaultEdgeRelationsForKind_AllKinds(t *testing.T) {
	cases := []struct {
		kind     DiagramKind
		wantLen  int
		wantKind DiagramRelationKind // first entry's relation kind
		wantMin  int                 // first entry's Min
		wantCF   ClaimForm           // first entry's expected ClaimForm
	}{
		{DiagramFlow, 1, DiagramRelGuard, 1, ClaimGuardCondition},
		{DiagramSequence, 1, DiagramRelCall, 1, ClaimCallEdge},
		{DiagramCallDAG, 1, DiagramRelCall, 1, ClaimCallEdge},
		{DiagramArchitecture, 1, DiagramRelContain, 0, ClaimUnknown},
		{DiagramNone, 0, DiagramRelUnknown, 0, ClaimUnknown},
	}
	for _, c := range cases {
		got := DefaultEdgeRelationsForKind(c.kind)
		if len(got) != c.wantLen {
			t.Errorf("DefaultEdgeRelationsForKind(%q) len=%d, want %d (got %+v)",
				c.kind, len(got), c.wantLen, got)
			continue
		}
		if c.wantLen == 0 {
			continue
		}
		if got[0].Kind != c.wantKind {
			t.Errorf("DefaultEdgeRelationsForKind(%q)[0].Kind = %q, want %q",
				c.kind, got[0].Kind, c.wantKind)
		}
		if got[0].Min != c.wantMin {
			t.Errorf("DefaultEdgeRelationsForKind(%q)[0].Min = %d, want %d",
				c.kind, got[0].Min, c.wantMin)
		}
		if got[0].ClaimForm != c.wantCF {
			t.Errorf("DefaultEdgeRelationsForKind(%q)[0].ClaimForm = %q, want %q",
				c.kind, got[0].ClaimForm, c.wantCF)
		}
	}
}

// TestDefaultEdgeRelationsForKind_DefensiveCopy ensures callers can
// mutate the returned slice (e.g. append observe contract for
// root-cause-trace) without affecting subsequent calls.
func TestDefaultEdgeRelationsForKind_DefensiveCopy(t *testing.T) {
	a := DefaultEdgeRelationsForKind(DiagramSequence)
	a = append(a, DiagramEdgeRelationContract{Kind: DiagramRelObserve, Min: 0, ClaimForm: ClaimExternalObservation})
	if len(a) != 2 {
		t.Fatalf("local append failed: %d", len(a))
	}
	b := DefaultEdgeRelationsForKind(DiagramSequence)
	if len(b) != 1 {
		t.Errorf("subsequent call should yield 1 entry; got %d (defensive copy regression)", len(b))
	}
}

// 8-family lock: TestCompile<Family>_DiagramEdgeRelationsContract
// for the 3 emit-diagram families + an exhaustive negative for the 5
// no-diagram families.

// planRequiringDiagram is a minimal AnswerSurfacePlan for the
// 8-family lock test — sets Diagram.Required=true so the diagram-
// emitting families' compile_*.go calls produce a non-nil
// DiagramFacetGraph (otherwise diagramPlanFor returns nil and
// EdgeRelations is unobservable).
func planRequiringDiagram() *AnswerSurfacePlan {
	return &AnswerSurfacePlan{
		Diagram: &DiagramContract{Required: true},
	}
}

func TestCompileCallChain_DiagramEdgeRelationsContract(t *testing.T) {
	view := BuildAnswerSemanticView(irForCallChain(), planRequiringDiagram())
	if view.DiagramPlan == nil {
		t.Fatal("call_chain must produce a DiagramPlan")
	}
	got := view.DiagramPlan.EdgeRelations
	if len(got) != 1 || got[0].Kind != DiagramRelCall ||
		got[0].Min != 1 || got[0].ClaimForm != ClaimCallEdge {
		t.Errorf("call_chain EdgeRelations = %+v, want [{call 1 call_edge}]", got)
	}
}

func TestCompileArchitecture_DiagramEdgeRelationsContract(t *testing.T) {
	view := BuildAnswerSemanticView(irForArchitecture(), planRequiringDiagram())
	if view.DiagramPlan == nil {
		t.Fatal("architecture must produce a DiagramPlan")
	}
	got := view.DiagramPlan.EdgeRelations
	if len(got) != 1 || got[0].Kind != DiagramRelContain ||
		got[0].Min != 0 || got[0].ClaimForm != ClaimUnknown {
		t.Errorf("architecture EdgeRelations = %+v, want [{contain 0 (unknown)}]", got)
	}
}

func TestCompileRootCauseTrace_DiagramEdgeRelationsContract(t *testing.T) {
	view := BuildAnswerSemanticView(irForRootCauseTrace(), planRequiringDiagram())
	if view.DiagramPlan == nil {
		t.Fatal("root_cause_trace must produce a DiagramPlan")
	}
	got := view.DiagramPlan.EdgeRelations
	if len(got) != 2 {
		t.Fatalf("root_cause_trace EdgeRelations len=%d, want 2 (sequence default + observe)", len(got))
	}
	if got[0].Kind != DiagramRelCall || got[0].Min != 1 || got[0].ClaimForm != ClaimCallEdge {
		t.Errorf("root_cause_trace EdgeRelations[0] = %+v, want {call 1 call_edge}", got[0])
	}
	if got[1].Kind != DiagramRelObserve || got[1].Min != 0 || got[1].ClaimForm != ClaimExternalObservation {
		t.Errorf("root_cause_trace EdgeRelations[1] = %+v, want {observe 0 external_observation}", got[1])
	}
}

// 5 no-diagram families MUST yield nil DiagramPlan even when the
// upstream AnswerSurfacePlan offers a diagram contract — these
// families' compile_*.go simply does not call diagramPlanFor. So
// EdgeRelations is moot; the validator never reads it.
func TestCompileNoDiagramFamilies_NilDiagramPlan(t *testing.T) {
	cases := []struct {
		name    string
		builder func() *AnalysisIR
	}{
		{"config_precedence", irForConfigPrecedence},
		{"role_lookup", irForRoleLookup},
		{"enumeration", irForEnumeration},
		{"generic", irForGeneric},
		{"comparison", irForComparison},
	}
	for _, c := range cases {
		view := BuildAnswerSemanticView(c.builder(), planRequiringDiagram())
		if view.DiagramPlan != nil {
			t.Errorf("%s: DiagramPlan must be nil; got %+v", c.name, view.DiagramPlan)
		}
	}
}
