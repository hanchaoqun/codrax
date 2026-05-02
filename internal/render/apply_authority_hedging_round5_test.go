package render

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestSynthesiseAuthorityCaveatFor_ReturnsTaggedCaveat: the IsZero
// fallback path needs a way to surface the system caveat WITHOUT
// going through ApplyAuthorityHedging. The synthesise helper returns
// the same tagged caveat string that addAuthorityCaveat would have
// emitted for the same evidence.
func TestSynthesiseAuthorityCaveatFor_ReturnsTaggedCaveat(t *testing.T) {
	evidence := []types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityHistorical},
	}
	got := SynthesiseAuthorityCaveatFor(evidence, "en")
	if got == "" {
		t.Fatal("expected non-empty caveat for non-factual evidence")
	}
	if !strings.Contains(got, authorityCaveatTag) {
		t.Errorf("synthesised caveat missing system tag: %q", got)
	}
	if !strings.Contains(got, AuthorityCaveatPrefix) {
		t.Errorf("synthesised caveat missing public prefix: %q", got)
	}
	if !strings.Contains(got, "historical") {
		t.Errorf("caveat missing severity word: %q", got)
	}
}

// TestSynthesiseAuthorityCaveatFor_FactualOnlyReturnsEmpty: when the
// evidence pool has nothing to hedge, the helper returns "" — the
// fallback path then renders without any caveat (correct: no hedge
// needed).
func TestSynthesiseAuthorityCaveatFor_FactualOnlyReturnsEmpty(t *testing.T) {
	evidence := []types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityFactual},
	}
	if got := SynthesiseAuthorityCaveatFor(evidence, "en"); got != "" {
		t.Errorf("factual-only: got non-empty caveat: %q", got)
	}
}

// TestSynthesiseAuthorityCaveatFor_ZhLanguage exercises locale
// fan-out through normalizeAnswerDocLang.
func TestSynthesiseAuthorityCaveatFor_ZhLanguage(t *testing.T) {
	evidence := []types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityConditional},
	}
	got := SynthesiseAuthorityCaveatFor(evidence, "zh")
	if !strings.Contains(got, "漂移") {
		t.Errorf("zh caveat missing zh prose: %q", got)
	}
	if !strings.Contains(got, authorityCaveatTag) {
		t.Errorf("zh caveat missing tag: %q", got)
	}
}

// TestAddAuthorityCaveat_DedupsAllTaggedEntries: defends against any
// accumulated state where multiple system caveats slipped into the
// slice. Round-5 made dedup whole-slice rather than first-match-only.
func TestAddAuthorityCaveat_DedupsAllTaggedEntries(t *testing.T) {
	stale1 := AuthorityCaveatPrefix + authorityCaveatTag + "stale 1"
	stale2 := AuthorityCaveatPrefix + authorityCaveatTag + "stale 2"
	llmCaveat := "evidence is partial."
	doc := &types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Caveats: []string{stale1, llmCaveat, stale2},
	}
	evidence := []types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityHistorical},
	}
	addAuthorityCaveat(doc, evidence, answerDocLangEN)
	taggedCount := 0
	llmPreserved := false
	for _, c := range doc.Caveats {
		if strings.Contains(c, authorityCaveatTag) {
			taggedCount++
		}
		if c == llmCaveat {
			llmPreserved = true
		}
	}
	if taggedCount != 1 {
		t.Errorf("expected 1 tagged caveat after dedup; got %d in %v",
			taggedCount, doc.Caveats)
	}
	if !llmPreserved {
		t.Errorf("LLM caveat lost in dedup: %v", doc.Caveats)
	}
}
