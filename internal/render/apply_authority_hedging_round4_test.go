package render

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestAuthorityCaveatTag_EmbeddedInGeneratedCaveats: every system-
// generated caveat MUST carry the private tag so dedup / strip /
// gate can distinguish system caveats from LLM-written ones.
func TestAuthorityCaveatTag_EmbeddedInGeneratedCaveats(t *testing.T) {
	for _, l := range []answerDocLang{answerDocLangEN, answerDocLangZH} {
		caveat := authorityCaveatText(map[types.AuthorityCeiling]int{
			types.AuthorityConditional: 1,
		}, l)
		if !strings.Contains(caveat, authorityCaveatTag) {
			t.Errorf("lang=%v caveat missing tag: %q", l, caveat)
		}
	}
}

// TestAddAuthorityCaveat_DedupByTag: a doc with both an LLM-written
// "Authority: ..." caveat AND the system caveat must keep BOTH (the
// dedup logic only replaces the system caveat, identified by its
// private tag).
func TestAddAuthorityCaveat_DedupByTag(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Caveats:   []string{AuthorityCaveatPrefix + "evidence is partial."}, // LLM-written
		Citations: []types.Citation{{File: "x.go", Line: 10}},
	}
	evidence := []types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityConditional},
	}
	addAuthorityCaveat(doc, evidence, answerDocLangEN)
	if len(doc.Caveats) != 2 {
		t.Fatalf("Caveats len = %d; want 2 (LLM + system)", len(doc.Caveats))
	}
	hasLLM := false
	hasSystem := false
	for _, c := range doc.Caveats {
		if strings.Contains(c, authorityCaveatTag) {
			hasSystem = true
		} else if strings.Contains(c, "evidence is partial") {
			hasLLM = true
		}
	}
	if !hasLLM {
		t.Errorf("LLM caveat lost: %v", doc.Caveats)
	}
	if !hasSystem {
		t.Errorf("system caveat missing: %v", doc.Caveats)
	}
}

// TestAddAuthorityCaveat_DedupSecondSystemCall: a second
// addAuthorityCaveat call (e.g. retry) must REPLACE the prior
// system caveat, not duplicate it.
func TestAddAuthorityCaveat_DedupSecondSystemCall(t *testing.T) {
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation}
	evidence := []types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityConditional},
	}
	addAuthorityCaveat(doc, evidence, answerDocLangEN)
	addAuthorityCaveat(doc, evidence, answerDocLangEN)
	count := 0
	for _, c := range doc.Caveats {
		if strings.Contains(c, authorityCaveatTag) {
			count++
		}
	}
	if count != 1 {
		t.Errorf("system caveat duplicated: count=%d", count)
	}
}

// TestStrongestCitedCeiling_PerContextScoping: a multi-topic doc
// where citation[0] is conditional and citation[1] is factual; a
// step that ONLY references citation[1] should not get hedged via
// the doc-wide worst case. Pre-round-4 strongestCitedCeiling took
// the whole doc; now it takes the cite subset.
func TestStrongestCitedCeiling_PerContextScoping(t *testing.T) {
	cites := []types.Citation{
		{File: "drifted.go", Line: 10},
		{File: "factual.go", Line: 20},
	}
	evidence := []types.EvidenceItem{
		{Source: "drifted.go", LineStart: 10, Authority: types.AuthorityConditional},
		{Source: "factual.go", LineStart: 20, Authority: types.AuthorityFactual},
	}
	// Only the factual cite — must return Unknown (no hedge needed).
	if got := strongestCitedCeiling([]types.Citation{cites[1]}, evidence); got != types.AuthorityUnknown {
		t.Errorf("factual-only subset: got %q; want Unknown", got)
	}
	// Whole doc — must return conditional.
	if got := strongestCitedCeiling(cites, evidence); got != types.AuthorityConditional {
		t.Errorf("full doc: got %q; want conditional", got)
	}
}

// TestStripInlineMarkerWithReason_RemovesMidLineReason exercises
// the round-4 inline-reason strip. Pre-fix, mid-line markers were
// ReplaceAll'd but adjacent `(reason)` brackets were left, polluting
// runExternalArtifactDecodedCheck token-match scans.
func TestStripInlineMarkerWithReason_RemovesMidLineReason(t *testing.T) {
	in := "lead text " + HedgeMarkerConditional + " (log-derived; code drifted) more prose."
	out := stripInlineMarkerWithReason(in, HedgeMarkerConditional)
	if strings.Contains(out, "log-derived") {
		t.Errorf("inline reason not stripped: %q", out)
	}
	if strings.Contains(out, HedgeMarkerConditional) {
		t.Errorf("marker not stripped: %q", out)
	}
	if !strings.Contains(out, "lead text") || !strings.Contains(out, "more prose") {
		t.Errorf("user content damaged: %q", out)
	}
}

// TestStripInlineMarkerWithReason_HandlesChineseBrackets covers
// the wide-char path; round-7 narrowed strip to system-canonical
// terse reasons only, so the test uses the actual terse reason
// shortHedgeReasonFor produces (not the long-form caveat body).
func TestStripInlineMarkerWithReason_HandlesChineseBrackets(t *testing.T) {
	terse := shortHedgeReasonFor(types.AuthorityHistorical, answerDocLangZH)
	in := "前文 " + HedgeMarkerHistorical + " " + terse + "后文。"
	out := stripInlineMarkerWithReason(in, HedgeMarkerHistorical)
	if strings.Contains(out, terse) {
		t.Errorf("zh terse reason not stripped: %q", out)
	}
}

// TestStripInlineMarkerWithReason_PreservesLongFormParenthetical:
// round-7 guarantee — the long-form caveat body (used only inside
// doc.Caveats[]) when found inline next to a marker is treated as
// user prose and PRESERVED. The long form should never travel
// inline anyway, but if it does, we don't corrupt the prose.
func TestStripInlineMarkerWithReason_PreservesLongFormParenthetical(t *testing.T) {
	longForm := "（旧构建中的历史观察；当前代码已重构或对应符号已不存在）"
	in := "前文 " + HedgeMarkerHistorical + " " + longForm + "后文。"
	out := stripInlineMarkerWithReason(in, HedgeMarkerHistorical)
	if !strings.Contains(out, longForm) {
		t.Errorf("long-form parenthetical (treated as user prose) was stripped: %q", out)
	}
}
