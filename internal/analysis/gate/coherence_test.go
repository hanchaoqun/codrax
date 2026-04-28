package gate

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// coherenceFixtureIR builds a minimum AnalysisIR that already passes
// every other check in Run, so any rejection in a coherence test is
// guaranteed to come from one of the new gates and not collateral
// damage from coverage/dag/etc. The fixture is intentionally NOT
// composed via the real compiler — coherence checks read only
// RequestModel + AnswerContract.RequiredAnswerShape, so a
// hand-rolled minimal IR is enough and lets tests isolate one
// signal at a time.
func coherenceFixtureIR(rm types.RequestModel, shape types.AnswerShape) *types.AnalysisIR {
	return &types.AnalysisIR{
		Version:      types.AnalysisIRVersion,
		RequestModel: rm,
		AnswerContract: types.AnswerContract{
			RequiredAnswerShape: shape,
		},
	}
}

// ── checkSubtopicCoherence ────────────────────────────────────────

func TestSubtopicCoherence_R1_1_DomainDivergence_Fails(t *testing.T) {
	rm := types.RequestModel{
		TermGraph: types.TermGraph{
			Canonical: []types.CanonicalTerm{
				{ID: "code:a", Surface: "A", Kind: types.TermSymbol, Domain: "agent", Confidence: 0.9},
				{ID: "code:b", Surface: "B", Kind: types.TermSymbol, Domain: "orchestrator", Confidence: 0.9},
			},
		},
		// 0 sub-topics — the LLM emitted 1 sub-topic case is also a fail; exercise the boundary.
	}
	ir := coherenceFixtureIR(rm, types.ShapeNone)
	check := checkSubtopicCoherence(ir)
	if check.Passed {
		t.Fatalf("R1.1 must fail when 2 distinct domains and ≤1 sub-topic; got %+v", check)
	}
	if !strings.Contains(check.Detail, "R1.1") || !strings.Contains(check.Detail, "agent") {
		t.Errorf("detail must cite R1.1 and the divergent domains; got %q", check.Detail)
	}
}

func TestSubtopicCoherence_R1_1_DomainDivergence_PassesWithSubTopics(t *testing.T) {
	rm := types.RequestModel{
		TermGraph: types.TermGraph{
			Canonical: []types.CanonicalTerm{
				{ID: "code:a", Surface: "A", Kind: types.TermSymbol, Domain: "agent", Confidence: 0.9},
				{ID: "code:b", Surface: "B", Kind: types.TermSymbol, Domain: "orchestrator", Confidence: 0.9},
			},
		},
		SubTopics: []types.SubTopic{
			{Summary: "agent half"},
			{Summary: "orchestrator half"},
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeExplanation)
	if check := checkSubtopicCoherence(ir); !check.Passed {
		t.Fatalf("R1.1 must pass when sub-topic count matches domain count; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_1_LowConfidenceTermsExcluded(t *testing.T) {
	// One Domain at high confidence, one at low confidence — the low-
	// confidence term should be excluded so the count stays at 1 and
	// R1.1 does NOT fire.
	rm := types.RequestModel{
		TermGraph: types.TermGraph{
			Canonical: []types.CanonicalTerm{
				{ID: "code:a", Surface: "A", Kind: types.TermSymbol, Domain: "agent", Confidence: 0.9},
				{ID: "code:b", Surface: "B", Kind: types.TermSymbol, Domain: "noisy", Confidence: 0.4},
			},
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeNone)
	if check := checkSubtopicCoherence(ir); !check.Passed {
		t.Fatalf("low-confidence terms must not contribute to domain count; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_2_PredicateContradiction_Fails(t *testing.T) {
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true},
		// no sub-topics
	}
	ir := coherenceFixtureIR(rm, types.ShapeNone)
	check := checkSubtopicCoherence(ir)
	if check.Passed {
		t.Fatalf("R1.2 must fail when IsCrossComponent=true and ≤1 sub-topic; got %+v", check)
	}
	if !strings.Contains(check.Detail, "R1.2") {
		t.Errorf("detail must cite R1.2; got %q", check.Detail)
	}
}

func TestSubtopicCoherence_R1_3_EntityOrphan_Fails(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"PrimaryX", "PrimaryY"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "drifted", Entities: []string{"Unrelated1", "Unrelated2"}},
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeExplanation)
	check := checkSubtopicCoherence(ir)
	if check.Passed {
		t.Fatalf("R1.3 must fail when sub-topic entities share no element with primary entities; got %+v", check)
	}
	if !strings.Contains(check.Detail, "R1.3") {
		t.Errorf("detail must cite R1.3; got %q", check.Detail)
	}
}

func TestSubtopicCoherence_R1_3_EntityOrphan_SinglePrimarySkipped(t *testing.T) {
	// Only 1 PrimaryEntity — orphan rule must NOT fire (a single
	// primary often legitimately fans out into different sub-topic
	// surfaces).
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"X"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "topic", Entities: []string{"Y"}},
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeExplanation)
	if check := checkSubtopicCoherence(ir); !check.Passed {
		t.Fatalf("R1.3 must pass when PrimaryEntities < 2; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_3_EntityOrphan_OverlapPasses(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"Shared", "Other"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "topic1", Entities: []string{"Shared"}},
			{Summary: "topic2", Entities: []string{"AlsoOther"}},
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeExplanation)
	if check := checkSubtopicCoherence(ir); !check.Passed {
		t.Fatalf("R1.3 must pass when sub-topics share at least one entity with primary; got %+v", check)
	}
}

// ── checkShapeSubjectCoherence ────────────────────────────────────

func TestShapeSubjectCoherence_R2_1_ScalarMultiTopic_Fails(t *testing.T) {
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsScalarAnswer: true},
		SubTopics: []types.SubTopic{
			{Summary: "first"},
			{Summary: "second"},
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeValue)
	check := checkShapeSubjectCoherence(ir)
	if check.Passed {
		t.Fatalf("R2.1 must fail when IsScalarAnswer=true and 2+ sub-topics; got %+v", check)
	}
	if !strings.Contains(check.Detail, "R2.1") {
		t.Errorf("detail must cite R2.1; got %q", check.Detail)
	}
}

func TestShapeSubjectCoherence_R2_1_SingleTopicScalarPasses(t *testing.T) {
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsScalarAnswer: true},
	}
	ir := coherenceFixtureIR(rm, types.ShapeValue)
	if check := checkShapeSubjectCoherence(ir); !check.Passed {
		t.Fatalf("R2.1 must pass for single-topic scalar; got %+v", check)
	}
}

func TestShapeSubjectCoherence_R2_2_ExplanationScalarSubject_Fails(t *testing.T) {
	rm := types.RequestModel{
		AnswerSubject: types.AnswerSubject{
			Kind:       types.SubjectNumeric,
			Confidence: 0.85,
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeExplanation)
	check := checkShapeSubjectCoherence(ir)
	if check.Passed {
		t.Fatalf("R2.2 must fail when Explanation shape but high-confidence Numeric subject; got %+v", check)
	}
	if !strings.Contains(check.Detail, "R2.2") {
		t.Errorf("detail must cite R2.2; got %q", check.Detail)
	}
}

func TestShapeSubjectCoherence_R2_2_LowConfidencePasses(t *testing.T) {
	// Below the 0.6 confidence floor — R2.2 must NOT fire to avoid
	// noisy retries on uncertain subject inference.
	rm := types.RequestModel{
		AnswerSubject: types.AnswerSubject{
			Kind:       types.SubjectNumeric,
			Confidence: 0.40,
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeExplanation)
	if check := checkShapeSubjectCoherence(ir); !check.Passed {
		t.Fatalf("R2.2 must pass below confidence floor; got %+v", check)
	}
}

func TestShapeSubjectCoherence_NonExplanation_NotApplicable(t *testing.T) {
	// Step-list shape with a high-confidence Numeric subject is
	// unusual but R2.2 only applies to Explanation; no rejection.
	rm := types.RequestModel{
		AnswerSubject: types.AnswerSubject{
			Kind:       types.SubjectNumeric,
			Confidence: 0.95,
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeStepList)
	if check := checkShapeSubjectCoherence(ir); !check.Passed {
		t.Fatalf("non-Explanation shape must not trigger R2.2; got %+v", check)
	}
}

// ── extractDistinctTermDomains ────────────────────────────────────

func TestExtractDistinctTermDomains_DropsConcepts(t *testing.T) {
	tg := types.TermGraph{
		Canonical: []types.CanonicalTerm{
			{Kind: types.TermSymbol, Domain: "agent", Confidence: 0.9},
			{Kind: types.TermConcept, Domain: "should-not-count", Confidence: 0.9},
			{Kind: types.TermSymbol, Domain: "orchestrator", Confidence: 0.9},
		},
	}
	got := extractDistinctTermDomains(tg)
	if len(got) != 2 {
		t.Fatalf("expected 2 distinct symbol-domains; got %v", got)
	}
}

func TestExtractDistinctTermDomains_DropsBlankDomains(t *testing.T) {
	tg := types.TermGraph{
		Canonical: []types.CanonicalTerm{
			{Kind: types.TermSymbol, Domain: "", Confidence: 0.9},
			{Kind: types.TermSymbol, Domain: "  ", Confidence: 0.9},
		},
	}
	if got := extractDistinctTermDomains(tg); len(got) != 0 {
		t.Errorf("blank/whitespace domains must be excluded; got %v", got)
	}
}

func TestExtractDistinctTermDomains_Sorted(t *testing.T) {
	tg := types.TermGraph{
		Canonical: []types.CanonicalTerm{
			{Kind: types.TermSymbol, Domain: "z", Confidence: 0.9},
			{Kind: types.TermSymbol, Domain: "a", Confidence: 0.9},
			{Kind: types.TermSymbol, Domain: "m", Confidence: 0.9},
		},
	}
	got := extractDistinctTermDomains(tg)
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Errorf("output must be sorted; got %v", got)
		}
	}
}

// ── Run integration ───────────────────────────────────────────────

func TestRun_CoherenceGatesIntegrate_ReadModeOnly(t *testing.T) {
	// Build a write-mode dispatch: coherence gates must be skipped
	// even when the IR carries a structurally inconsistent shape,
	// because write mode has its own SuccessCriteria suite.
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsScalarAnswer: true},
		SubTopics: []types.SubTopic{
			{Summary: "first"},
			{Summary: "second"},
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeValue)
	// Write-mode bypass — pass any non-empty non-"read" mode.
	report := Run(ir, Thresholds{}, "apply")
	if c := findCheck(report, "shape_subject_coherence"); c != nil {
		t.Errorf("coherence gates must not run in write mode; got %+v", c)
	}
}
