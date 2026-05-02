package gate

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
	"github.com/hanchaoqun/codrax/internal/types"
)

// fakeSymbolResolver is a deterministic in-memory resolver used by the
// R1.4 / R1.5 tests so we can control "this entity resolves to domain
// X" without spinning up a real repomap graph. Returns hits ordered by
// the keys in the `byEntity` map (callers should pre-sort if order
// matters; no test today depends on iteration order beyond presence).
type fakeSymbolResolver struct {
	byEntity map[string][]normalizer.SymbolHit
}

func (f *fakeSymbolResolver) LookupSymbol(surface string) []normalizer.SymbolHit {
	if f == nil {
		return nil
	}
	if hits, ok := f.byEntity[strings.TrimSpace(surface)]; ok {
		return hits
	}
	return nil
}

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
	check := checkSubtopicCoherence(ir, nil)
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
	if check := checkSubtopicCoherence(ir, nil); !check.Passed {
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
	if check := checkSubtopicCoherence(ir, nil); !check.Passed {
		t.Fatalf("low-confidence terms must not contribute to domain count; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_2_PredicateContradiction_Fails(t *testing.T) {
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true},
		// no sub-topics
	}
	ir := coherenceFixtureIR(rm, types.ShapeNone)
	check := checkSubtopicCoherence(ir, nil)
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
	check := checkSubtopicCoherence(ir, nil)
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
	if check := checkSubtopicCoherence(ir, nil); !check.Passed {
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
	if check := checkSubtopicCoherence(ir, nil); !check.Passed {
		t.Fatalf("R1.3 must pass when sub-topics share at least one entity with primary; got %+v", check)
	}
}

// ── R1.5 entity_unresolvable ──────────────────────────────────────

func TestSubtopicCoherence_R1_5_AllEntitiesUnresolved_Fails(t *testing.T) {
	// 3 sub-topics; sub-topic 3's entities all return 0 resolver hits
	// (the canonical "PlanMode / mode_dispatch hallucination" pattern).
	// Sub-topics 1 and 2 each have at least one resolvable entity so
	// the failure is isolated to sub-topic 3.
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"PipelineMode": {{Canonical: "PipelineMode", Domain: "types"}},
			"analyze":      {{Canonical: "analyze", Domain: "agent"}},
			"plan":         {{Canonical: "plan", Domain: "agent"}},
			// PlanMode and mode_dispatch deliberately absent → 0 hits.
		},
	}
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true}, // bypass R1.4
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"PipelineMode", "PlanMode", "orchestrator"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "Analyze pipeline", Entities: []string{"PipelineMode", "analyze"}},
			{Summary: "Plan pipeline", Entities: []string{"PipelineMode", "plan"}},
			{Summary: "模式分发机制", Entities: []string{"PlanMode", "mode_dispatch"}},
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeExplanation)
	check := checkSubtopicCoherence(ir, resolver)
	if check.Passed {
		t.Fatalf("R1.5 must fail when a sub-topic has zero resolvable entities; got %+v", check)
	}
	if !strings.Contains(check.Detail, "R1.5") {
		t.Errorf("detail must cite R1.5; got %q", check.Detail)
	}
	if !strings.Contains(check.Detail, "sub-topic 3") {
		t.Errorf("detail must name the failing sub-topic index; got %q", check.Detail)
	}
}

func TestSubtopicCoherence_R1_5_OneEntityResolves_Passes(t *testing.T) {
	// A sub-topic with one unresolvable + one resolvable entity must
	// PASS R1.5 — the rule requires at least ONE hit per sub-topic,
	// not all entities.
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"PipelineMode": {{Canonical: "PipelineMode", Domain: "types"}},
			"PlanMode":     {{Canonical: "PlanMode", Domain: "types"}},
		},
	}
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true}, // bypass R1.4
		SubTopics: []types.SubTopic{
			{Summary: "first", Entities: []string{"PipelineMode", "garbage"}},
			{Summary: "second", Entities: []string{"PlanMode", "also_garbage"}},
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeExplanation)
	if check := checkSubtopicCoherence(ir, resolver); !check.Passed {
		t.Fatalf("R1.5 must pass when at least one entity per sub-topic resolves; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_5_NilResolver_NoOp(t *testing.T) {
	// Pre-RunOptions callers (Run, tests, write mode) pass nil resolver
	// → R1.5 must be a no-op so existing R1.1/R1.2/R1.3 behaviour is
	// preserved byte-for-byte.
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true}, // bypass R1.4
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"X", "Y"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "first", Entities: []string{"X"}},
			{Summary: "second", Entities: []string{"never-resolves"}},
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeExplanation)
	if check := checkSubtopicCoherence(ir, nil); !check.Passed {
		t.Fatalf("nil resolver must disable R1.5; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_5_AllConceptual_NoOp(t *testing.T) {
	// Audit run 2026-05-02 07:06 ("对比 read 模式的 explorer 阶段
	// 和 write 模式的 verify 阶段") emitted 2 sub-topics with
	// conceptual entities (`explorer` / `retry` / `read`) — none of
	// which the resolver could match because they are stage / phase
	// names, not Tier 1-2 symbols. Pre-2026-05-02-2 the rule fired
	// here and burned the analyzer retry budget. The refined rule
	// requires ASYMMETRY: only fire when some sub-topics resolve and
	// others don't. A uniformly-conceptual IR is the legitimate
	// cross-component case and must pass.
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			// no entries — every entity returns 0 hits
		},
	}
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true},
		SubTopics: []types.SubTopic{
			{Summary: "explorer 阶段（read 模式）的 retry 机制", Entities: []string{"explorer", "retry", "read"}},
			{Summary: "verify 阶段（write 模式）的 retry 机制", Entities: []string{"verifier", "retry", "write"}},
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeExplanation)
	if check := checkSubtopicCoherence(ir, resolver); !check.Passed {
		t.Fatalf("R1.5 must pass when all sub-topics are uniformly unresolved (no asymmetry); got %+v", check)
	}
}

func TestSubtopicCoherence_R1_5_SingleTopic_NoOp(t *testing.T) {
	// Single-topic IR doesn't go through the multi-topic anchor
	// backbone, so R1.5 should not fire even with an unresolvable
	// entity.
	resolver := &fakeSymbolResolver{byEntity: map[string][]normalizer.SymbolHit{}}
	rm := types.RequestModel{
		SubTopics: []types.SubTopic{
			{Summary: "only", Entities: []string{"never-resolves"}},
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeExplanation)
	if check := checkSubtopicCoherence(ir, resolver); !check.Passed {
		t.Fatalf("R1.5 must skip when nSub<2; got %+v", check)
	}
}

// ── R1.4 axis_collapse ────────────────────────────────────────────

func TestSubtopicCoherence_R1_4_EnumerationCollapsesToSingleDomain_Fails(t *testing.T) {
	// Replicates the failing run: 3 sub-topics, every entity resolves
	// to the same Domain, IsCategoryEnumeration=true, IsCrossComponent
	// =false. The LLM split a single enumeration axis (mode value) into
	// per-value sub-topics — collapse repair required.
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"PipelineMode": {{Canonical: "PipelineMode", Domain: "types"}},
			"ModeRead":     {{Canonical: "ModeRead", Domain: "types"}},
			"ModePlan":     {{Canonical: "ModePlan", Domain: "types"}},
			"ModeApply":    {{Canonical: "ModeApply", Domain: "types"}},
		},
	}
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
			IsCrossComponent:      false,
		},
		SubTopics: []types.SubTopic{
			{Summary: "ModeRead", Entities: []string{"PipelineMode", "ModeRead"}},
			{Summary: "ModePlan", Entities: []string{"PipelineMode", "ModePlan"}},
			{Summary: "ModeApply", Entities: []string{"PipelineMode", "ModeApply"}},
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeExplanation)
	check := checkSubtopicCoherence(ir, resolver)
	if check.Passed {
		t.Fatalf("R1.4 must fail when enumeration sub-topics all collapse to one domain; got %+v", check)
	}
	if !strings.Contains(check.Detail, "R1.4") {
		t.Errorf("detail must cite R1.4; got %q", check.Detail)
	}
	if !strings.Contains(check.Detail, "list_of_symbols") {
		t.Errorf("detail must propose the list_of_symbols repair; got %q", check.Detail)
	}
}

func TestSubtopicCoherence_R1_4_TwoDomains_Passes(t *testing.T) {
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"Explorer": {{Canonical: "Explorer", Domain: "agent"}},
			"Resolver": {{Canonical: "Resolver", Domain: "tool"}},
		},
	}
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		SubTopics: []types.SubTopic{
			{Summary: "explorer half", Entities: []string{"Explorer"}},
			{Summary: "resolver half", Entities: []string{"Resolver"}},
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeExplanation)
	if check := checkSubtopicCoherence(ir, resolver); !check.Passed {
		t.Fatalf("R1.4 must pass when sub-topic entities span ≥2 domains; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_4_CrossComponent_NoOp(t *testing.T) {
	// IsCrossComponent=true is the LLM's explicit "this question spans
	// subsystems" judgment. Per the user-intent-over-system-gates red
	// line, we never force-collapse such a split even if domains
	// happen to overlap in the resolver.
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"PipelineMode": {{Canonical: "PipelineMode", Domain: "types"}},
			"ModePlan":     {{Canonical: "ModePlan", Domain: "types"}},
		},
	}
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
			IsCrossComponent:      true, // load-bearing — bypasses R1.4
		},
		SubTopics: []types.SubTopic{
			{Summary: "first", Entities: []string{"PipelineMode"}},
			{Summary: "second", Entities: []string{"ModePlan"}},
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeExplanation)
	if check := checkSubtopicCoherence(ir, resolver); !check.Passed {
		t.Fatalf("R1.4 must respect IsCrossComponent=true; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_4_NotEnumeration_NoOp(t *testing.T) {
	// Without IsCategoryEnumeration AND without Intent=enumerate, R1.4
	// must NOT fire — multi-aspect explanations of one component are
	// legitimate and not over-decomposition.
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"Explorer": {{Canonical: "Explorer", Domain: "agent"}},
		},
	}
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: false,
		},
		SubTopics: []types.SubTopic{
			{Summary: "behaviour", Entities: []string{"Explorer"}},
			{Summary: "implementation", Entities: []string{"Explorer"}},
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeExplanation)
	if check := checkSubtopicCoherence(ir, resolver); !check.Passed {
		t.Fatalf("R1.4 must skip non-enumeration intent; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_4_NilResolver_NoOp(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		SubTopics: []types.SubTopic{
			{Summary: "first", Entities: []string{"X"}},
			{Summary: "second", Entities: []string{"Y"}},
		},
	}
	ir := coherenceFixtureIR(rm, types.ShapeExplanation)
	if check := checkSubtopicCoherence(ir, nil); !check.Passed {
		t.Fatalf("nil resolver must disable R1.4; got %+v", check)
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
