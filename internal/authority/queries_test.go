package authority

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestHighestAuthorityFor_PicksStrongestAcrossOverlappingItems: when
// two evidence items cite the same anchor with different ceilings, the
// strongest (factual) wins — at least one underlying anchor supports
// the strong claim.
func TestHighestAuthorityFor_PicksStrongestAcrossOverlappingItems(t *testing.T) {
	items := []types.EvidenceItem{
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityConditional},
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityFactual},
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityHistorical},
	}
	got := HighestAuthorityFor(items, "a.go", 10)
	if got != types.AuthorityFactual {
		t.Errorf("HighestAuthorityFor = %q; want factual", got)
	}
}

// TestLowestAuthorityFor_PicksWeakestAcrossOverlappingItems mirrors
// the dual: weakest support is what limits e.g. "must include" prose
// in the upcoming contract checker.
func TestLowestAuthorityFor_PicksWeakestAcrossOverlappingItems(t *testing.T) {
	items := []types.EvidenceItem{
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityConditional},
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityFactual},
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityHistorical},
	}
	got := LowestAuthorityFor(items, "a.go", 10)
	if got != types.AuthorityHistorical {
		t.Errorf("LowestAuthorityFor = %q; want historical", got)
	}
}

// TestHighestAuthorityFor_LineSlackTolerantWithinFour: the line
// matcher allows ±4 line slack to absorb citation/anchor mismatches.
func TestHighestAuthorityFor_LineSlackTolerantWithinFour(t *testing.T) {
	items := []types.EvidenceItem{
		{Source: "a.go", LineStart: 12, Authority: types.AuthorityFactual},
	}
	if got := HighestAuthorityFor(items, "a.go", 10); got != types.AuthorityFactual {
		t.Errorf("within slack: got %q; want factual", got)
	}
	if got := HighestAuthorityFor(items, "a.go", 25); got != types.AuthorityUnknown {
		t.Errorf("outside slack: got %q; want unknown (no match)", got)
	}
}

// TestHighestAuthorityFor_MismatchedFileReturnsUnknown defends the
// path-keyed lookup.
func TestHighestAuthorityFor_MismatchedFileReturnsUnknown(t *testing.T) {
	items := []types.EvidenceItem{
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityFactual},
	}
	if got := HighestAuthorityFor(items, "b.go", 10); got != types.AuthorityUnknown {
		t.Errorf("mismatched file: got %q; want unknown", got)
	}
}

// TestAuthorityHistogram_CountsByCeiling exercises the histogram
// helper. Items with zero-value Authority bucket under the empty
// string key.
func TestAuthorityHistogram_CountsByCeiling(t *testing.T) {
	items := []types.EvidenceItem{
		{Authority: types.AuthorityFactual},
		{Authority: types.AuthorityFactual},
		{Authority: types.AuthorityConditional},
		{}, // unknown / zero
	}
	h := AuthorityHistogram(items)
	if h[types.AuthorityFactual] != 2 {
		t.Errorf("factual count = %d; want 2", h[types.AuthorityFactual])
	}
	if h[types.AuthorityConditional] != 1 {
		t.Errorf("conditional count = %d; want 1", h[types.AuthorityConditional])
	}
	if h[types.AuthorityUnknown] != 1 {
		t.Errorf("unknown count = %d; want 1", h[types.AuthorityUnknown])
	}
}

// TestHighestAuthorityFor_EmptyInputsReturnUnknown defends nil and
// empty-slice safety.
func TestHighestAuthorityFor_EmptyInputsReturnUnknown(t *testing.T) {
	if got := HighestAuthorityFor(nil, "a.go", 1); got != types.AuthorityUnknown {
		t.Errorf("nil items: got %q; want unknown", got)
	}
	if got := HighestAuthorityFor([]types.EvidenceItem{}, "", 0); got != types.AuthorityUnknown {
		t.Errorf("empty + empty file: got %q; want unknown", got)
	}
}
