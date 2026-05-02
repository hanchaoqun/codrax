package render

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestApplyAuthorityHedging_ValueShapeSummaryGetsHedge: shape=value
// renders Summary as the lead-in before the literal. When the
// citation points at drift-bounded evidence, the Summary must
// receive the hedge prefix or the overreach gate would force
// unsatisfiable retries.
func TestApplyAuthorityHedging_ValueShapeSummaryGetsHedge(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:     types.ShapeValue,
		Summary:   "The value comes from config.go:42.",
		Value:     &types.AnswerValue{Literal: "false", CitationRef: 0},
		Citations: []types.Citation{{File: "config.go", Line: 42}},
	}
	evidence := []types.EvidenceItem{
		{Source: "config.go", LineStart: 42, Authority: types.AuthorityHistorical},
	}
	ApplyAuthorityHedging(doc, evidence, "en")
	if !strings.HasPrefix(doc.Summary, hedgeMarkerHistorical) {
		t.Errorf("value-shape summary missing historical hedge: %q", doc.Summary)
	}
}

// TestApplyAuthorityHedging_ExplanationSummaryGetsHedge: shape=
// explanation uses Summary AS the body. Drift-bounded citations
// must trigger a Summary hedge so the contract gate sees the
// sentinel.
func TestApplyAuthorityHedging_ExplanationSummaryGetsHedge(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Summary:   "The mechanism dispatches via x.go:10.",
		Citations: []types.Citation{{File: "x.go", Line: 10}},
	}
	evidence := []types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityConditional},
	}
	ApplyAuthorityHedging(doc, evidence, "en")
	if !strings.HasPrefix(doc.Summary, hedgeMarkerConditional) {
		t.Errorf("explanation summary missing conditional hedge: %q", doc.Summary)
	}
}

// TestApplyAuthorityHedging_BooleanRationaleGetsHedge: shape=boolean's
// Rationale carries the YES/NO reasoning. Drift-bounded citation =>
// rationale gets hedge.
func TestApplyAuthorityHedging_BooleanRationaleGetsHedge(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape: types.ShapeBoolean,
		Boolean: &types.AnswerBoolean{
			Decision:    true,
			Rationale:   "X is enabled because Y.",
			CitationRef: 0,
		},
		Citations: []types.Citation{{File: "x.go", Line: 10}},
	}
	evidence := []types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityHistorical},
	}
	ApplyAuthorityHedging(doc, evidence, "en")
	if !strings.HasPrefix(doc.Boolean.Rationale, hedgeMarkerHistorical) {
		t.Errorf("boolean rationale missing historical hedge: %q",
			doc.Boolean.Rationale)
	}
}

// TestApplyAuthorityHedging_BooleanRationaleFallsBackToBroaderPool:
// when Boolean.CitationRef points at a factual cite but OTHER
// citations in the pool are hedged, the rationale should still pick
// up the hedge from the broader pool.
func TestApplyAuthorityHedging_BooleanRationaleFallsBackToBroaderPool(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape: types.ShapeBoolean,
		Boolean: &types.AnswerBoolean{
			Decision:    true,
			Rationale:   "X is enabled because Y.",
			CitationRef: 0,
		},
		Citations: []types.Citation{
			{File: "factual.go", Line: 5},
			{File: "drifted.go", Line: 50},
		},
	}
	evidence := []types.EvidenceItem{
		{Source: "factual.go", LineStart: 5, Authority: types.AuthorityFactual},
		{Source: "drifted.go", LineStart: 50, Authority: types.AuthorityHistorical},
	}
	ApplyAuthorityHedging(doc, evidence, "en")
	if !strings.HasPrefix(doc.Boolean.Rationale, hedgeMarkerHistorical) {
		t.Errorf("boolean rationale missed broader pool hedge: %q",
			doc.Boolean.Rationale)
	}
}

// TestApplyAuthorityHedging_SummaryIdempotentOnRetry: re-running
// hedge on an already-hedged summary must not double-prefix.
func TestApplyAuthorityHedging_SummaryIdempotentOnRetry(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Summary:   "The mechanism dispatches via x.go:10.",
		Citations: []types.Citation{{File: "x.go", Line: 10}},
	}
	evidence := []types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityConditional},
	}
	ApplyAuthorityHedging(doc, evidence, "en")
	first := doc.Summary
	ApplyAuthorityHedging(doc, evidence, "en")
	if doc.Summary != first {
		t.Errorf("non-idempotent hedge: 1st=%q 2nd=%q", first, doc.Summary)
	}
	// Defense-in-depth: count sentinels — must be exactly one.
	if n := strings.Count(doc.Summary, hedgeMarkerConditional); n != 1 {
		t.Errorf("expected exactly 1 hedge marker after retry; got %d in %q",
			n, doc.Summary)
	}
}

// TestApplyAuthorityHedging_StrongestCitedCeilingPicksWorst: when a
// doc cites both conditional and historical evidence, the summary
// hedge should reflect HISTORICAL (the worse case), not conditional.
// Historical means "current code can't be mapped" — a stronger claim
// than "drift but still mapped".
func TestApplyAuthorityHedging_StrongestCitedCeilingPicksWorst(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Summary: "Mechanism description.",
		Citations: []types.Citation{
			{File: "drifted.go", Line: 10},
			{File: "moved.go", Line: 20},
		},
	}
	evidence := []types.EvidenceItem{
		{Source: "drifted.go", LineStart: 10, Authority: types.AuthorityConditional},
		{Source: "moved.go", LineStart: 20, Authority: types.AuthorityHistorical},
	}
	ApplyAuthorityHedging(doc, evidence, "en")
	if !strings.HasPrefix(doc.Summary, hedgeMarkerHistorical) {
		t.Errorf("worst-case ceiling not picked: %q", doc.Summary)
	}
}

// TestApplyAuthorityHedging_FactualOnlyDocSummaryUntouched: when
// every cited anchor is factual, the summary stays clean.
func TestApplyAuthorityHedging_FactualOnlyDocSummaryUntouched(t *testing.T) {
	original := "Mechanism description."
	doc := &types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Summary:   original,
		Citations: []types.Citation{{File: "x.go", Line: 10}},
	}
	evidence := []types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityFactual},
	}
	ApplyAuthorityHedging(doc, evidence, "en")
	if doc.Summary != original {
		t.Errorf("factual-only: summary was modified: %q", doc.Summary)
	}
}

// TestApplyAuthorityHedging_CeilingUpgradeOnRetry: retry-1 saw
// conditional evidence and prefixed Summary with [hedged]; retry-2
// surfaces historical evidence (worse). The hedge label MUST upgrade
// to [historical] — not stay stuck at [hedged]. Without upgrade, the
// rendered label/evidence pair drift apart and reviewer LLMs may
// mark it inconsistent.
func TestApplyAuthorityHedging_CeilingUpgradeOnRetry(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Summary:   "Mechanism dispatches via x.go:10.",
		Citations: []types.Citation{{File: "x.go", Line: 10}},
	}
	// First pass: conditional.
	ev1 := []types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityConditional},
	}
	ApplyAuthorityHedging(doc, ev1, "en")
	if !strings.HasPrefix(doc.Summary, hedgeMarkerConditional) {
		t.Fatalf("first pass: missing conditional prefix: %q", doc.Summary)
	}
	// Second pass: same anchor now grades historical (worse).
	ev2 := []types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityHistorical},
	}
	ApplyAuthorityHedging(doc, ev2, "en")
	if !strings.HasPrefix(doc.Summary, hedgeMarkerHistorical) {
		t.Errorf("second pass: ceiling did not upgrade to historical: %q", doc.Summary)
	}
	// And [hedged] must be GONE (was the previous prefix).
	if strings.HasPrefix(doc.Summary, hedgeMarkerConditional) {
		t.Errorf("second pass: stale conditional prefix retained: %q", doc.Summary)
	}
}

// TestApplyAuthorityHedging_StepsUseMarkerOnly_NoBodyDilution: the
// load-bearing UX guarantee — per-step injection MUST be marker-only
// (no inline prose body). Repeating the long-form prose on every
// step dilutes the answer; the doc-level Caveat carries the
// explanation once.
func TestApplyAuthorityHedging_StepsUseMarkerOnly_NoBodyDilution(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape: types.ShapeStepList,
		Steps: []types.AnswerStep{
			{Index: 1, Description: "X dispatches Y", CitationRef: 0},
			{Index: 2, Description: "Y handles Z", CitationRef: 0},
		},
		Citations: []types.Citation{{File: "a.go", Line: 10}},
	}
	evidence := []types.EvidenceItem{
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityConditional},
	}
	ApplyAuthorityHedging(doc, evidence, "en")
	for i, step := range doc.Steps {
		if !strings.HasPrefix(step.Description, hedgeMarkerConditional) {
			t.Errorf("step[%d] missing marker: %q", i, step.Description)
		}
		// MUST NOT contain the long-form body that used to be
		// repeated per-step.
		if strings.Contains(step.Description, "based on log/perf observation") {
			t.Errorf("step[%d] contains long-form body (dilution): %q",
				i, step.Description)
		}
		if strings.Contains(step.Description, "drifted — claim hedged") {
			t.Errorf("step[%d] contains long-form body: %q",
				i, step.Description)
		}
	}
	// The long-form body MUST live exactly once, in the doc Caveat.
	if len(doc.Caveats) != 1 {
		t.Errorf("expected 1 caveat carrying the explanation; got %d", len(doc.Caveats))
	}
}

// TestStripAuthorityArtifacts_RemovesMarkersAndCaveat exercises the
// downstream-protection helper that shields SelfConsistency /
// ExternalArtifactDecoded reviewers from system-injected annotations.
//
// Round-4 refinement: the caveat is recognised by its embedded
// system-private tag, not the public "Authority: " prefix. Test
// uses the actual generated caveat text so an LLM-written caveat
// starting with the same prefix would be PRESERVED (system
// distinguishes its own caveat by tag).
func TestStripAuthorityArtifacts_RemovesMarkersAndCaveat(t *testing.T) {
	systemCaveat := authorityCaveatText(map[types.AuthorityCeiling]int{
		types.AuthorityHistorical: 1,
	}, answerDocLangEN)
	if systemCaveat == "" {
		t.Fatalf("authorityCaveatText returned empty; setup failure")
	}
	input := hedgeMarkerHistorical + " (historical observation) X is at line 10.\n" +
		"Inline " + hedgeMarkerConditional + " mid-sentence remains stripped.\n" +
		systemCaveat
	out := StripAuthorityArtifacts(input)
	if strings.Contains(out, hedgeMarkerConditional) ||
		strings.Contains(out, hedgeMarkerHistorical) ||
		strings.Contains(out, hedgeMarkerIllustrative) {
		t.Errorf("strip left a marker behind: %q", out)
	}
	if strings.Contains(out, authorityCaveatTag) {
		t.Errorf("strip left the system caveat behind: %q", out)
	}
	if !strings.Contains(out, "X is at line 10.") {
		t.Errorf("strip removed user content: %q", out)
	}
}

// TestStripAuthorityArtifacts_PreservesLLMWrittenCaveat: a caveat
// the LLM wrote that happens to start with "Authority: " (the public
// prefix) does NOT carry the system-private tag and must NOT be
// stripped. Without this guarantee, system stripping could swallow
// honest LLM disclosures.
func TestStripAuthorityArtifacts_PreservesLLMWrittenCaveat(t *testing.T) {
	llmCaveat := AuthorityCaveatPrefix + "evidence in this answer is partial."
	out := StripAuthorityArtifacts("X is at line 10.\n" + llmCaveat)
	if !strings.Contains(out, llmCaveat) {
		t.Errorf("LLM-written caveat was stripped: out=%q", out)
	}
}
