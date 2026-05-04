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

func TestCompileRootCauseTrace_HasUncertaintyRule(t *testing.T) {
	view := BuildAnswerSemanticView(irForRootCauseTrace(), nil)
	if len(view.UncertaintyRules) == 0 {
		t.Error("expected at least one UncertaintyRule for log-source drift")
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

func TestCompileRoleLookup_HasNoDiagram(t *testing.T) {
	view := BuildAnswerSemanticView(irForRoleLookup(), nil)
	if view.DiagramPlan != nil && view.DiagramPlan.Required {
		t.Error("role lookup should NOT require a diagram (single-literal answer)")
	}
}

// ── QFCallChain 3 cases ────────────────────────────────────────────

func TestCompileCallChain_ResolvesFamily(t *testing.T) {
	view := BuildAnswerSemanticView(irForCallChain(), nil)
	if view == nil || view.Family != QFCallChain {
		t.Errorf("expected QFCallChain; got %v", view)
	}
}

func TestCompileCallChain_HasOrderedListRequired(t *testing.T) {
	view := BuildAnswerSemanticView(irForCallChain(), nil)
	hasList, hasDiagram := false, false
	for _, b := range view.RequiredBlocks {
		if b.Kind == BlockOrderedList && b.Required {
			hasList = true
		}
		if b.Kind == BlockDiagram && b.Required {
			hasDiagram = true
		}
	}
	if !hasList {
		t.Error("call chain must require ordered list of hops")
	}
	if !hasDiagram {
		t.Error("call chain must require diagram block")
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

func TestCompileArchitecture_HasSectionAndDiagramRequired(t *testing.T) {
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
	if !hasDiagram {
		t.Error("architecture must require BlockDiagram")
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
