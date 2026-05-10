package gate

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
	"github.com/hanchaoqun/codrax/internal/types"
)

// === B3 (2026-05-10) — scope-aware coherence rules ===
//
// R1.3 widened to consider PrimaryEntities ∪ PrimaryScopes
// R1.5 carve-out for scope-only sub-topics
// R1.8 scope_anchor_distribution SOFT advisory

// makeScopeIR is a thin wrapper for scope-aware tests.
func makeScopeIR(rm types.RequestModel) *types.AnalysisIR {
	return &types.AnalysisIR{
		Version:        types.AnalysisIRVersion,
		RequestModel:   rm,
		AnswerContract: types.AnswerContract{},
	}
}

// TestR1_3_ScopeAware_OverlapViaScopesPasses — sub-topic anchors a
// scope, primary names two scopes; R1.3 should pass via union check.
func TestR1_3_ScopeAware_OverlapViaScopesPasses(t *testing.T) {
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"codrax", "opencode"},
			PrimaryScopes:   []string{"codrax", "opencode"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "codrax detection mechanism", Entities: []string{"codrax"}, Scopes: []string{"codrax"}},
			{Summary: "opencode detection mechanism", Entities: []string{"opencode"}, Scopes: []string{"opencode"}},
		},
	}
	check := checkSubtopicCoherence(makeScopeIR(rm), nil)
	// R1.3 is the focus; allow R1.5 since resolver==nil makes it inert
	for _, seg := range strings.Split(check.Detail, " | ") {
		if strings.Contains(seg, "R1.3 entity_orphan") {
			t.Errorf("R1.3 should pass via PrimaryScopes overlap; got detail %q", check.Detail)
		}
	}
	if !check.Passed {
		t.Errorf("gate should pass; got %+v", check)
	}
}

// TestR1_3_ScopeAware_NoOverlap_SubTopicOnlySymbols — primary has
// scopes only, sub-topics name only internal symbols. Pre-B3 this
// failed with R1.3 entity_orphan. Post-B3 should still fail (symbols
// don't overlap with scopes), but the test pins the expected
// failure shape so we know R1.3 still works for legitimate orphans.
func TestR1_3_ScopeAware_NoOverlap_SubTopicOnlySymbols(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"codrax", "opencode"},
			PrimaryScopes:   []string{"codrax", "opencode"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "topic 1", Entities: []string{"SymbolOracle", "ContractCheck"}},
			{Summary: "topic 2", Entities: []string{"OpencodeClient"}},
		},
	}
	check := checkSubtopicCoherence(makeScopeIR(rm), nil)
	if check.Passed {
		t.Errorf("R1.3 should still fail when sub-topic.entities have no scope overlap; got %+v", check)
	}
	if !strings.Contains(check.Detail, "R1.3 entity_orphan") {
		t.Errorf("expected R1.3 entity_orphan in detail; got %q", check.Detail)
	}
}

// TestR1_3_LegacyPath_SinglePrimary_NoChange — single PrimaryEntity,
// no scopes. R1.3 behaviour byte-identical to pre-B3 (skipped because
// PrimaryEntities count < coherenceMinPrimaryEntitiesForOrphan).
func TestR1_3_LegacyPath_SinglePrimary_NoChange(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"foo"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "topic 1", Entities: []string{"bar"}},
		},
	}
	check := checkSubtopicCoherence(makeScopeIR(rm), nil)
	if !check.Passed {
		t.Errorf("legacy R1.3 single-primary case should still pass; got %+v", check)
	}
}

// TestR1_5_ScopeOnly_CarveOut_NoFalsePositive — sub-topics anchor
// only on scope tokens (not symbols). The pre-B3 R1.5 would round-
// trip them through resolver.LookupSymbol; for a sub-repo name
// that's NOT a symbol the resolver returns 0, producing false-positive
// R1.5 entity_unresolvable. Post-B3 the carve-out skips resolver for
// scope-only sub-topics.
func TestR1_5_ScopeOnly_CarveOut_NoFalsePositive(t *testing.T) {
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"opencode": {{Canonical: "OpencodeClient", Domain: "opencode"}}, // asymmetric: only opencode resolves
		},
	}
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"codrax", "opencode"},
			PrimaryScopes:   []string{"codrax", "opencode"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "codrax", Entities: []string{"codrax"}, Scopes: []string{"codrax"}},
			{Summary: "opencode", Entities: []string{"opencode"}, Scopes: []string{"opencode"}},
		},
	}
	check := checkSubtopicCoherence(makeScopeIR(rm), resolver)
	if !check.Passed {
		t.Errorf("R1.5 carve-out should let scope-only sub-topics pass; got %+v", check)
	}
	if strings.Contains(check.Detail, "R1.5") {
		t.Errorf("R1.5 should not fire for scope-only sub-topics; got %q", check.Detail)
	}
}

// TestR1_5_MixedScopeSymbol_HitsResolver — sub-topic with both scope
// and symbol entities. Resolver still validates the symbol; carve-out
// applies only when ALL entities are scope-typed.
func TestR1_5_MixedScopeSymbol_HitsResolver(t *testing.T) {
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"SymbolOracle":   {{Canonical: "SymbolOracle", Domain: "codrax"}},
			"OpencodeClient": {{Canonical: "OpencodeClient", Domain: "opencode"}},
		},
	}
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"codrax", "opencode"},
			PrimaryScopes:   []string{"codrax", "opencode"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "codrax detection", Entities: []string{"codrax", "SymbolOracle"}, Scopes: []string{"codrax"}},
			{Summary: "opencode handling", Entities: []string{"opencode", "OpencodeClient"}, Scopes: []string{"opencode"}},
		},
	}
	check := checkSubtopicCoherence(makeScopeIR(rm), resolver)
	if !check.Passed {
		t.Errorf("mixed scope+symbol with resolver hits should pass; got %+v", check)
	}
}

// TestR1_5_LegacyPath_SymbolsOnly_StillFires — no scopes, symbol-only
// asymmetry. Legacy R1.5 behaviour preserved.
func TestR1_5_LegacyPath_SymbolsOnly_StillFires(t *testing.T) {
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"RealSymbol": {{Canonical: "RealSymbol", Domain: "agent"}},
			// "FakeSymbol" not present
		},
	}
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{PrimaryEntities: []string{"RealSymbol", "OtherSym"}},
		SubTopics: []types.SubTopic{
			{Summary: "real", Entities: []string{"RealSymbol"}},
			{Summary: "fake", Entities: []string{"FakeSymbol"}},
		},
	}
	check := checkSubtopicCoherence(makeScopeIR(rm), resolver)
	if check.Passed {
		t.Errorf("R1.5 should fire on symbol-only asymmetry; got %+v", check)
	}
	if !strings.Contains(check.Detail, "R1.5 entity_unresolvable") {
		t.Errorf("expected R1.5 entity_unresolvable; got %q", check.Detail)
	}
}

// TestR1_8_AllScopesAnchored_NoAdvisory — every PrimaryScope appears
// in some sub-topic.Scopes, R1.8 stays silent.
func TestR1_8_AllScopesAnchored_NoAdvisory(t *testing.T) {
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"codrax", "opencode"},
			PrimaryScopes:   []string{"codrax", "opencode"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "topic 1", Entities: []string{"codrax"}, Scopes: []string{"codrax"}},
			{Summary: "topic 2", Entities: []string{"opencode"}, Scopes: []string{"opencode"}},
		},
	}
	check := checkSubtopicCoherence(makeScopeIR(rm), nil)
	if !check.Passed {
		t.Errorf("R1.8 anchored case should pass; got %+v", check)
	}
	if strings.Contains(check.Detail, "R1.8") {
		t.Errorf("R1.8 should not fire when all scopes anchored; got %q", check.Detail)
	}
}

// TestR1_8_OneScopeUnanchored_AdvisoryOnly — one PrimaryScope has
// no sub-topic anchor. R1.8 emits a SOFT advisory but the gate
// passes (advisory, not hard fail).
func TestR1_8_OneScopeUnanchored_AdvisoryOnly(t *testing.T) {
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"codrax", "opencode"},
			PrimaryScopes:   []string{"codrax", "opencode"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "all about codrax", Entities: []string{"codrax"}, Scopes: []string{"codrax"}},
			{Summary: "more codrax detail", Entities: []string{"codrax"}, Scopes: []string{"codrax"}},
		},
	}
	check := checkSubtopicCoherence(makeScopeIR(rm), nil)
	if !check.Passed {
		t.Errorf("R1.8 must NOT hard-fail (advisory only); got %+v", check)
	}
	if !strings.Contains(check.Detail, "R1.8 scope_anchor_distribution") {
		t.Errorf("R1.8 advisory should appear in detail; got %q", check.Detail)
	}
	if !strings.Contains(check.Detail, "opencode") {
		t.Errorf("missing scope name should appear in advisory; got %q", check.Detail)
	}
	if check.Score >= 1.0 {
		t.Errorf("advisory should reduce Score below 1.0; got %v", check.Score)
	}
}

// TestR1_8_NotCrossComponent_Skipped — IsCrossComponent=false → R1.8
// does not fire even if scopes unanchored.
func TestR1_8_NotCrossComponent_Skipped(t *testing.T) {
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: false},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"codrax", "opencode"},
			PrimaryScopes:   []string{"codrax", "opencode"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "topic 1", Entities: []string{"codrax"}, Scopes: []string{"codrax"}},
			{Summary: "topic 2", Entities: []string{"codrax"}, Scopes: []string{"codrax"}},
		},
	}
	check := checkSubtopicCoherence(makeScopeIR(rm), nil)
	if !check.Passed {
		t.Errorf("not-cross-component should pass; got %+v", check)
	}
	if strings.Contains(check.Detail, "R1.8") {
		t.Errorf("R1.8 should not fire when IsCrossComponent=false; got %q", check.Detail)
	}
}

// TestR1_8_SingleScopeNoAdvisory — only 1 PrimaryScope → R1.8 skipped.
func TestR1_8_SingleScopeNoAdvisory(t *testing.T) {
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"codrax"},
			PrimaryScopes:   []string{"codrax"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "topic 1", Entities: []string{"codrax"}, Scopes: []string{"codrax"}},
			{Summary: "topic 2", Entities: []string{"codrax"}, Scopes: []string{"codrax"}},
		},
	}
	check := checkSubtopicCoherence(makeScopeIR(rm), nil)
	if strings.Contains(check.Detail, "R1.8") {
		t.Errorf("R1.8 should not fire for single scope; got %q", check.Detail)
	}
}

// TestSubTopicScopeOnly_AllScopeMatch — every entity is a scope.
func TestSubTopicScopeOnly_AllScopeMatch(t *testing.T) {
	st := types.SubTopic{Entities: []string{"codrax"}, Scopes: []string{"codrax"}}
	if !subTopicScopeOnly(st, []string{"codrax", "opencode"}) {
		t.Errorf("scope-only sub-topic should be detected")
	}
}

// TestSubTopicScopeOnly_MixedReturnsFalse — at least one entity not in scope set.
func TestSubTopicScopeOnly_MixedReturnsFalse(t *testing.T) {
	st := types.SubTopic{Entities: []string{"codrax", "SymbolOracle"}, Scopes: []string{"codrax"}}
	if subTopicScopeOnly(st, []string{"codrax"}) {
		t.Errorf("mixed scope+symbol must NOT be classified as scope-only")
	}
}

// TestSubTopicScopeOnly_EmptyEntitiesReturnsFalse — empty entities is
// neutral, not scope-only.
func TestSubTopicScopeOnly_EmptyEntitiesReturnsFalse(t *testing.T) {
	st := types.SubTopic{Entities: nil, Scopes: []string{"codrax"}}
	if subTopicScopeOnly(st, []string{"codrax"}) {
		t.Errorf("empty entities must not be classified scope-only")
	}
}

// TestUnionStringLists_Dedup ensures union dedupes case-sensitively.
func TestUnionStringLists_Dedup(t *testing.T) {
	got := unionStringLists([]string{"a", "b"}, []string{"b", "c"})
	if len(got) != 3 {
		t.Errorf("union dedup: want 3, got %v", got)
	}
}

// TestFlattenSubTopicEntitiesAndScopes_BothLanes
func TestFlattenSubTopicEntitiesAndScopes_BothLanes(t *testing.T) {
	subs := []types.SubTopic{
		{Entities: []string{"a", "b"}, Scopes: []string{"c"}},
		{Entities: []string{"b"}, Scopes: []string{"d"}},
	}
	got := flattenSubTopicEntitiesAndScopes(subs)
	if len(got) != 4 {
		t.Errorf("flatten: want 4 dedup'd, got %v", got)
	}
}
