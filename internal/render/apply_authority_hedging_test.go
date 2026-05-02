package render

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestApplyAuthorityHedging_NilDocSafe defends nil-input safety.
func TestApplyAuthorityHedging_NilDocSafe(t *testing.T) {
	if got := ApplyAuthorityHedging(nil, nil, "en"); got != nil {
		t.Errorf("nil doc: got non-nil = %+v", got)
	}
}

// TestApplyAuthorityHedging_StepConditionalGetsHedgePrefix asserts
// the dominant case: a step citing an anchor whose evidence is
// conditional gets a hedge prefix in its description.
func TestApplyAuthorityHedging_StepConditionalGetsHedgePrefix(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape: types.ShapeStepList,
		Steps: []types.AnswerStep{
			{Index: 1, Description: "X calls Y", CitationRef: 0},
		},
		Citations: []types.Citation{{File: "a.go", Line: 10}},
	}
	evidence := []types.EvidenceItem{
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityConditional},
	}
	ApplyAuthorityHedging(doc, evidence, "en")
	if !strings.HasPrefix(doc.Steps[0].Description, hedgeMarkerConditional) {
		t.Errorf("step description missing hedge prefix: %q", doc.Steps[0].Description)
	}
	if !strings.Contains(doc.Steps[0].Description, "X calls Y") {
		t.Errorf("step description lost original body: %q", doc.Steps[0].Description)
	}
}

// TestApplyAuthorityHedging_StepFactualKeepsClean: when at least one
// supporting evidence is factual, HighestAuthorityFor wins and the
// step stays un-hedged. Defends against an over-eager hedge that
// would dilute strong claims.
func TestApplyAuthorityHedging_StepFactualKeepsClean(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape: types.ShapeStepList,
		Steps: []types.AnswerStep{
			{Index: 1, Description: "do X", CitationRef: 0},
		},
		Citations: []types.Citation{{File: "a.go", Line: 10}},
	}
	evidence := []types.EvidenceItem{
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityFactual},
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityConditional},
	}
	ApplyAuthorityHedging(doc, evidence, "en")
	if doc.Steps[0].Description != "do X" {
		t.Errorf("factual support present: step description was hedged: %q",
			doc.Steps[0].Description)
	}
}

// TestApplyAuthorityHedging_SymbolHistoricalGetsHedge: AnswerSymbols
// with Authority=historical get the hedge sentinel injected into
// Rationale.
func TestApplyAuthorityHedging_SymbolHistoricalGetsHedge(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape: types.ShapeListOfSymbols,
		Symbols: []types.AnswerSymbol{
			{
				Name:      "OldHandler",
				File:      "old.go",
				Line:      30,
				Kind:      types.KindFunction,
				Rationale: "drives the legacy code path",
				Authority: types.AuthorityHistorical,
			},
		},
	}
	ApplyAuthorityHedging(doc, nil, "en")
	if !strings.HasPrefix(doc.Symbols[0].Rationale, hedgeMarkerHistorical) {
		t.Errorf("historical symbol: rationale missing hedge prefix: %q",
			doc.Symbols[0].Rationale)
	}
}

// TestApplyAuthorityHedging_AppendsCaveat asserts that a doc whose
// underlying evidence pool carries any drifted item gets a top-
// level caveat surfacing the drift to the user.
func TestApplyAuthorityHedging_AppendsCaveat(t *testing.T) {
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation}
	evidence := []types.EvidenceItem{
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityHistorical},
		{Source: "a.go", LineStart: 20, Authority: types.AuthorityConditional},
	}
	ApplyAuthorityHedging(doc, evidence, "en")
	if len(doc.Caveats) != 1 {
		t.Fatalf("Caveats len = %d; want 1", len(doc.Caveats))
	}
	if !strings.HasPrefix(doc.Caveats[0], AuthorityCaveatPrefix) {
		t.Errorf("caveat missing prefix: %q", doc.Caveats[0])
	}
	if !strings.Contains(doc.Caveats[0], "drifted") || !strings.Contains(doc.Caveats[0], "historical") {
		t.Errorf("caveat missing drifted/historical surfacing: %q", doc.Caveats[0])
	}
}

// TestApplyAuthorityHedging_DedupesCaveatOnRetry: a second call must
// REPLACE the prior authority caveat rather than appending a duplicate.
func TestApplyAuthorityHedging_DedupesCaveatOnRetry(t *testing.T) {
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation}
	evidence := []types.EvidenceItem{
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityHistorical},
	}
	ApplyAuthorityHedging(doc, evidence, "en")
	ApplyAuthorityHedging(doc, evidence, "en")
	count := 0
	for _, c := range doc.Caveats {
		if strings.HasPrefix(c, AuthorityCaveatPrefix) {
			count++
		}
	}
	if count != 1 {
		t.Errorf("authority caveat count = %d; want 1 (dedup)", count)
	}
}

// TestApplyAuthorityHedging_NoDriftNoCaveat: when every evidence
// item is factual (or the pool is empty), no caveat is added.
func TestApplyAuthorityHedging_NoDriftNoCaveat(t *testing.T) {
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation}
	evidence := []types.EvidenceItem{
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityFactual},
	}
	ApplyAuthorityHedging(doc, evidence, "en")
	for _, c := range doc.Caveats {
		if strings.HasPrefix(c, AuthorityCaveatPrefix) {
			t.Errorf("unexpected authority caveat: %q", c)
		}
	}
}

// TestApplyAuthorityHedging_ChineseTemplate sanity-checks the
// localised prose: zh language picks the Chinese hedge body.
func TestApplyAuthorityHedging_ChineseTemplate(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape: types.ShapeStepList,
		Steps: []types.AnswerStep{
			{Index: 1, Description: "执行 X", CitationRef: 0},
		},
		Citations: []types.Citation{{File: "a.go", Line: 10}},
	}
	evidence := []types.EvidenceItem{
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityConditional},
	}
	ApplyAuthorityHedging(doc, evidence, "zh")
	if !strings.Contains(doc.Steps[0].Description, "漂移") {
		t.Errorf("zh hedge body missing in description: %q", doc.Steps[0].Description)
	}
}
