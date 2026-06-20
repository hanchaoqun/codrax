package hdp

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/binder"
	"github.com/hanchaoqun/codrax/internal/analysis/priority"
	"github.com/hanchaoqun/codrax/internal/types"
)

func rm(intent types.Intent, symbols ...string) types.RequestModel {
	out := types.RequestModel{Intent: intent}
	for _, s := range symbols {
		out.TermGraph.Canonical = append(out.TermGraph.Canonical, types.CanonicalTerm{
			ID: "code:" + strings.ToLower(s), Surface: s,
			Kind: types.TermSymbol, Confidence: 0.9,
		})
	}
	return out
}

func findByStatement(hs []types.Hypothesis, needle string) *types.Hypothesis {
	for i := range hs {
		if strings.Contains(hs[i].Statement, needle) {
			return &hs[i]
		}
	}
	return nil
}

func TestPlan_RootCause_PrioritizesTopSymbol(t *testing.T) {
	hs := Plan(rm(types.IntentRootCause, "Explorer", "ShouldStop"))
	if len(hs) == 0 {
		t.Fatal("expected at least one hypothesis")
	}
	h := findByStatement(hs, "Explorer")
	if h == nil {
		t.Fatalf("expected hypothesis mentioning Explorer; got %+v", hs)
	}
	// Priority is computed by the priority package; unpack via
	// priority.Raw. A root_cause seed with strong intent match
	// should clear 30 in the human-scale range.
	if priority.Raw(h.Priority) < 30 {
		t.Fatalf("root_cause seed hypothesis should have raw priority ≥30; got %d", priority.Raw(h.Priority))
	}
	if h.Status != types.HypUnknown {
		t.Fatalf("fresh hypotheses must be unknown; got %q", h.Status)
	}
}

func TestPlan_Ambiguity_ProducesHypothesisPerClause(t *testing.T) {
	input := rm(types.IntentExplain, "Foo")
	input.Ambiguities = []types.Ambiguity{
		{Clause: "stop", Options: []string{"soft", "hard"}, Resolution: "both"},
		{Clause: "cache", Options: []string{"memory", "disk"}},
	}
	hs := Plan(input)
	clauseHits := 0
	for _, h := range hs {
		if strings.Contains(h.Statement, `"stop"`) || strings.Contains(h.Statement, `"cache"`) {
			clauseHits++
		}
	}
	if clauseHits != 2 {
		t.Fatalf("expected 2 clause-driven hypotheses; got %d (full=%+v)", clauseHits, hs)
	}
}

func TestPlan_ReturnValue_UsesScalarHypothesisNotEnumeration(t *testing.T) {
	hs := Plan(rm(types.IntentReturnValue, "WARN", "FATAL"))
	if len(hs) != 2 {
		t.Fatalf("return-value request should produce one value hypothesis per subject; got %+v", hs)
	}
	for _, h := range hs {
		if strings.Contains(h.Statement, "finite set of symbols") {
			t.Fatalf("return-value hypothesis must not force enumeration wording: %+v", h)
		}
		if !strings.Contains(h.Statement, "requested value") {
			t.Fatalf("return-value hypothesis should describe value derivation: %+v", h)
		}
	}
}

func TestPlan_HighSecurity_AddsUntrustedPathHypothesis(t *testing.T) {
	input := rm(types.IntentExplain, "Auth")
	input.RiskMatrix.Security.Level = 4
	hs := Plan(input)
	if findByStatement(hs, "un-sanitized") == nil {
		t.Fatalf("security≥3 must add untrusted-path hypothesis; got %+v", hs)
	}
}

func TestPlan_HighDataIntegrity_AddsInvariantHypothesis(t *testing.T) {
	input := rm(types.IntentRootCause, "Migration")
	input.RiskMatrix.DataIntegrity.Level = 5
	hs := Plan(input)
	if findByStatement(hs, "data invariants") == nil {
		t.Fatalf("data_integrity≥3 must add invariant hypothesis; got %+v", hs)
	}
}

func TestPlan_NeverEmpty(t *testing.T) {
	hs := Plan(types.RequestModel{})
	if len(hs) == 0 {
		t.Fatalf("Plan must never return an empty hypothesis set")
	}
}

func TestBinder_AttachesToEvidenceAndValidate(t *testing.T) {
	tg := types.TaskGraph{Nodes: []types.TaskNode{
		{ID: "n0", Type: types.NodeProbe},
		{ID: "n1", Type: types.NodeEvidence, Objective: "collect evidence for h1"},
		{ID: "n2", Type: types.NodeValidate, Objective: "validate h2"},
		{ID: "n3", Type: types.NodeFinalize, Objective: "render answer"},
	}}
	hs := []types.Hypothesis{{ID: "h1", Statement: "evidence claim"}, {ID: "h2", Statement: "validate claim"}}
	if err := binder.BindByRelevance(&tg, hs, binder.Options{}); err != nil {
		t.Fatalf("binder: %v", err)
	}
	if len(tg.Nodes[0].Hypotheses) != 0 {
		t.Fatalf("probe nodes must not be bound; got %+v", tg.Nodes[0].Hypotheses)
	}
	if len(tg.Nodes[1].Hypotheses) == 0 || len(tg.Nodes[2].Hypotheses) == 0 || len(tg.Nodes[3].Hypotheses) == 0 {
		t.Fatalf("evidence/validate/finalize must be bound; got %+v", tg.Nodes)
	}
}

func TestBinder_PreservesExistingBindings(t *testing.T) {
	tg := types.TaskGraph{Nodes: []types.TaskNode{
		{ID: "n1", Type: types.NodeEvidence, Hypotheses: []string{"h_preexisting"}},
		{ID: "n2", Type: types.NodeValidate, Objective: "validate"},
	}}
	if err := binder.BindByRelevance(&tg, []types.Hypothesis{{ID: "h1", Statement: "validate claim"}}, binder.Options{}); err != nil {
		t.Fatalf("binder: %v", err)
	}
	if tg.Nodes[0].Hypotheses[0] != "h_preexisting" {
		t.Fatalf("binder must not overwrite existing hypothesis list; got %+v", tg.Nodes[0].Hypotheses)
	}
	if len(tg.Nodes[1].Hypotheses) == 0 {
		t.Fatalf("binder must attach to unbound nodes; got %+v", tg.Nodes[1].Hypotheses)
	}
}

func TestValidate_FlagsUnboundNodes(t *testing.T) {
	tg := types.TaskGraph{Nodes: []types.TaskNode{
		{ID: "probe", Type: types.NodeProbe},
		{ID: "ev", Type: types.NodeEvidence},
		{ID: "val", Type: types.NodeValidate, Hypotheses: []string{"h1"}},
		{ID: "fin", Type: types.NodeFinalize},
	}}
	missing := Validate(tg)
	if len(missing) != 2 {
		t.Fatalf("expected 2 missing bindings (ev, fin); got %v", missing)
	}
	wantSet := map[string]bool{"ev": true, "fin": true}
	for _, id := range missing {
		if !wantSet[id] {
			t.Fatalf("unexpected missing node %q", id)
		}
	}
}

func TestValidate_AllBoundPassesEmpty(t *testing.T) {
	tg := types.TaskGraph{Nodes: []types.TaskNode{
		{ID: "probe", Type: types.NodeProbe},
		{ID: "ev", Type: types.NodeEvidence, Hypotheses: []string{"h1"}},
		{ID: "fin", Type: types.NodeFinalize, Hypotheses: []string{"h1"}},
	}}
	if len(Validate(tg)) != 0 {
		t.Fatalf("fully bound graph should pass; got %+v", Validate(tg))
	}
}

func TestPlan_Deterministic(t *testing.T) {
	a := Plan(rm(types.IntentRootCause, "Foo", "Bar"))
	b := Plan(rm(types.IntentRootCause, "Foo", "Bar"))
	if len(a) != len(b) {
		t.Fatalf("non-deterministic length: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Statement != b[i].Statement {
			t.Fatalf("non-deterministic hypothesis at %d: %+v vs %+v", i, a[i], b[i])
		}
	}
}

// ── multi-subject dispatch ──────────────────────────────────────

func rmWithConf(intent types.Intent, pairs ...interface{}) types.RequestModel {
	if len(pairs)%2 != 0 {
		panic("rmWithConf expects alternating (surface string, conf float32) pairs")
	}
	out := types.RequestModel{Intent: intent}
	for i := 0; i < len(pairs); i += 2 {
		surface := pairs[i].(string)
		conf := float32(pairs[i+1].(float64))
		out.TermGraph.Canonical = append(out.TermGraph.Canonical, types.CanonicalTerm{
			ID: "code:" + strings.ToLower(surface), Surface: surface,
			Kind: types.TermSymbol, Confidence: conf,
		})
	}
	return out
}

func countByFalsificationKind(hs []types.Hypothesis, kind string) int {
	n := 0
	for _, h := range hs {
		if h.FalsificationCondition.Kind == kind {
			n++
		}
	}
	return n
}

func TestPlan_TwoSymbols_EnumerateEmitsPerSubject(t *testing.T) {
	hs := Plan(rm(types.IntentEnumerate, "Blob", "Log"))
	if n := countByFalsificationKind(hs, types.CritAnswerSetUnbounded); n != 2 {
		t.Fatalf("enumerate with 2 subjects should produce 2 anchored hypotheses; got %d (full=%+v)", n, hs)
	}
	if findByStatement(hs, "anchored on Blob") == nil || findByStatement(hs, "anchored on Log") == nil {
		t.Fatalf("each subject must get its own anchored statement; got %+v", hs)
	}
}

func TestPlan_TwoSymbols_RootCauseEmitsPerSubject(t *testing.T) {
	hs := Plan(rm(types.IntentRootCause, "Alpha", "Beta"))
	if n := countByFalsificationKind(hs, types.CritNoCallSites); n != 2 {
		t.Fatalf("root_cause with 2 subjects should produce 2 NoCallSites hypotheses; got %d (%+v)", n, hs)
	}
	// Each NoCallSites falsification expr should target its own subject.
	seen := map[string]bool{}
	for _, h := range hs {
		if h.FalsificationCondition.Kind == types.CritNoCallSites {
			seen[h.FalsificationCondition.Expr] = true
		}
	}
	if !seen["Alpha"] || !seen["Beta"] {
		t.Fatalf("falsification expr must include each subject; seen=%v", seen)
	}
}

func TestPlan_TwoSymbols_ConfigQueryEmitsPerSubject(t *testing.T) {
	hs := Plan(rm(types.IntentConfigQuery, "ApiKey", "Timeout"))
	if n := countByFalsificationKind(hs, types.CritMultipleResolutionChains); n != 2 {
		t.Fatalf("config_query with 2 subjects should produce 2 hypotheses; got %d (%+v)", n, hs)
	}
	if findByStatement(hs, "config for ApiKey") == nil || findByStatement(hs, "config for Timeout") == nil {
		t.Fatalf("each subject must get its own config-resolution statement; got %+v", hs)
	}
}

func TestPlan_ThreeSymbols_CappedAtKEquals3(t *testing.T) {
	// Four symbols, equal Confidence. Expect exactly 3 anchored
	// hypotheses + whatever else the intent adds.
	hs := Plan(rm(types.IntentEnumerate, "A", "B", "C", "D"))
	if n := countByFalsificationKind(hs, types.CritAnswerSetUnbounded); n != 3 {
		t.Fatalf("top-K cap should be 3; got %d anchored hypotheses (%+v)", n, hs)
	}
	if findByStatement(hs, "anchored on D") != nil {
		t.Fatalf("D fell outside top-3; should not appear in statements: %+v", hs)
	}
}

func TestPlan_ConfidenceGap_DegradesToSingleSubject(t *testing.T) {
	// Top symbol 1.0, runner-up 0.3 → gap 0.7 > 0.4 → collapse.
	hs := Plan(rmWithConf(types.IntentEnumerate, "Strong", 1.0, "Weak", 0.3))
	if n := countByFalsificationKind(hs, types.CritAnswerSetUnbounded); n != 1 {
		t.Fatalf("confidence gap > 0.4 should collapse to single subject; got %d (%+v)", n, hs)
	}
	if findByStatement(hs, "anchored on Strong") == nil {
		t.Fatalf("collapse must keep the top subject; got %+v", hs)
	}
	if findByStatement(hs, "anchored on Weak") != nil {
		t.Fatalf("weak subject must not appear after collapse; got %+v", hs)
	}
}

func TestPlan_ConfidenceGap_ShallowKeepsAll(t *testing.T) {
	// Top 1.0, runner-up 0.7 → gap 0.3 ≤ 0.4 → keep both.
	hs := Plan(rmWithConf(types.IntentEnumerate, "A", 1.0, "B", 0.7))
	if n := countByFalsificationKind(hs, types.CritAnswerSetUnbounded); n != 2 {
		t.Fatalf("shallow confidence gap should keep both subjects; got %d (%+v)", n, hs)
	}
}

func TestPlan_RelationCue_AddsRelationHypothesis(t *testing.T) {
	input := rm(types.IntentExplain, "Blob", "Log")
	input.Predicates.IsRelationalLookup = true
	hs := Plan(input)
	rel := findByStatement(hs, "A direct relation exists between")
	if rel == nil {
		t.Fatalf("IsRelationalLookup=true + 2 subjects must emit relation hypothesis; got %+v", hs)
	}
	if rel.FalsificationCondition.Kind != types.CritRelationAbsent {
		t.Fatalf("relation hypothesis must falsify via CritRelationAbsent; got %q", rel.FalsificationCondition.Kind)
	}
	if rel.FalsificationCondition.Expr != "Blob,Log" {
		t.Fatalf("relation expr must be 'A,B'; got %q", rel.FalsificationCondition.Expr)
	}
}

func TestPlan_RelationCue_SingleSubjectDoesNotTrigger(t *testing.T) {
	input := rm(types.IntentExplain, "Solo")
	input.Predicates.IsRelationalLookup = true
	hs := Plan(input)
	if findByStatement(hs, "A direct relation exists between") != nil {
		t.Fatalf("1 subject must not trigger relation hypothesis; got %+v", hs)
	}
}

func TestPlan_RelationCue_ThreeSubjectsDoesNotTrigger(t *testing.T) {
	// N-ary relation is out of scope; only pairwise fires.
	input := rm(types.IntentExplain, "A", "B", "C")
	input.Predicates.IsRelationalLookup = true
	hs := Plan(input)
	if findByStatement(hs, "A direct relation exists between") != nil {
		t.Fatalf("3 subjects must not trigger pairwise relation hypothesis; got %+v", hs)
	}
}

func TestPlan_NoRelationCue_NoRelationHypothesis(t *testing.T) {
	// Two subjects but LLM said not a relational lookup → no relation hyp.
	hs := Plan(rm(types.IntentExplain, "A", "B"))
	if findByStatement(hs, "A direct relation exists between") != nil {
		t.Fatalf("IsRelationalLookup=false must not emit relation hypothesis; got %+v", hs)
	}
}

// Multi-subject explain should still produce an anchored-on-set hypothesis
// that names every subject — priority's intentMatch scans the statement
// text, so losing a subject's surface means the hypothesis drops to the
// 0.4 fallback.
func TestPlan_MultiSubjectExplain_NamesAllSubjects(t *testing.T) {
	hs := Plan(rm(types.IntentExplain, "First", "Second"))
	h := findByStatement(hs, "anchored on the set")
	if h == nil {
		t.Fatalf("expected set-anchored explain hypothesis; got %+v", hs)
	}
	if !strings.Contains(h.Statement, "First") || !strings.Contains(h.Statement, "Second") {
		t.Fatalf("set hypothesis must name every subject; got %q", h.Statement)
	}
}

// TestPlan_RootCause_SingleSymbolModerate_AddsAlternativeHypothesis
// pins the depth-floor injection: a Moderate-complexity root_cause
// with exactly one top symbol must end up with at least 2 hypotheses
// (the prime suspect + an alternative-cause stub) so the
// extractor's verdict pinning ladder cannot lock onto the first
// hypothesis as the only candidate.
func TestPlan_RootCause_SingleSymbolModerate_AddsAlternativeHypothesis(t *testing.T) {
	input := rm(types.IntentRootCause, "Foo")
	input.Complexity = types.ComplexityModerate
	hs := Plan(input)
	if len(hs) < 2 {
		t.Fatalf("Moderate root_cause with 1 symbol must produce ≥2 hypotheses; got %d (%+v)", len(hs), hs)
	}
	if findByStatement(hs, "alternative root cause") == nil {
		t.Fatalf("expected alternative-root-cause stub; got %+v", hs)
	}
}

// TestPlan_RootCause_SingleSymbolComplex_AddsAlternativeHypothesis:
// same contract for ComplexityComplex.
func TestPlan_RootCause_SingleSymbolComplex_AddsAlternativeHypothesis(t *testing.T) {
	input := rm(types.IntentRootCause, "Foo")
	input.Complexity = types.ComplexityComplex
	hs := Plan(input)
	if len(hs) < 2 {
		t.Fatalf("Complex root_cause with 1 symbol must produce ≥2 hypotheses; got %d", len(hs))
	}
	if findByStatement(hs, "alternative root cause") == nil {
		t.Fatalf("expected alternative-root-cause stub; got %+v", hs)
	}
}

func TestPlan_RootCause_ObservationOnlyRuntimeSkipsAlternativeHypothesis(t *testing.T) {
	input := rm(types.IntentRootCause, "UserCard")
	input.Complexity = types.ComplexityComplex
	input.LogTriage = &types.LogBundle{
		Errors: []types.LogError{{Type: "TypeError"}},
	}
	input.ExternalObservationPolicy = &types.ExternalObservationPolicy{
		CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
		ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:      []string{"只分析日志"},
		Confidence:        0.9,
	}
	hs := Plan(input)
	if findByStatement(hs, "alternative root cause") != nil {
		t.Fatalf("observation-only runtime root-cause should not inject caller-side alternative hypothesis: %+v", hs)
	}
}

// TestPlan_RootCause_SingleSymbolSimple_NoFloor preserves the
// minimal-cost path for Simple root_cause: don't add a second
// hypothesis if the analyzer judged the question simple. Pre-fix
// behaviour is byte-identical here so simple-question runs do not
// pay an extra exploration round.
func TestPlan_RootCause_SingleSymbolSimple_NoFloor(t *testing.T) {
	input := rm(types.IntentRootCause, "Foo")
	input.Complexity = types.ComplexitySimple
	hs := Plan(input)
	if findByStatement(hs, "alternative root cause") != nil {
		t.Fatalf("Simple root_cause must NOT trigger the depth floor; got %+v", hs)
	}
}

// TestPlan_RootCause_MultiSymbolModerate_NoExtraHypothesis: when
// topSymbols already returns ≥2 candidates, the depth floor is
// already met by the per-symbol hypothesis fan-out — no extra stub
// is appended.
func TestPlan_RootCause_MultiSymbolModerate_NoExtraHypothesis(t *testing.T) {
	input := rm(types.IntentRootCause, "Foo", "Bar")
	input.Complexity = types.ComplexityModerate
	hs := Plan(input)
	if findByStatement(hs, "alternative root cause") != nil {
		t.Fatalf("multi-symbol root_cause already meets the floor; alt stub must NOT be added; got %+v", hs)
	}
}
