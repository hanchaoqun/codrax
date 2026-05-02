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
