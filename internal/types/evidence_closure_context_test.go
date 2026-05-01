package types

import (
	"reflect"
	"testing"
)

// TestHasContextAroundRegion_FullyCoveredAcrossOneRange exercises
// the simplest happy path: a single merged range fully contains the
// requested [start - window, end + window] interval.
func TestHasContextAroundRegion_FullyCoveredAcrossOneRange(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadRanges(map[string][]LineRange{
		"a.go": {{Start: 100, End: 200}},
	})
	if !c.HasContextAroundRegion("a.go", 110, 190, 5) {
		t.Errorf("[110, 190]±5 inside [100, 200] must be covered")
	}
}

// TestHasContextAroundRegion_FullyCoveredAcrossAdjacentRanges checks
// that two abutting merged ranges (mergeLineRanges leaves [1,50] and
// [51,100] as one because they're adjacent — but we test the logic
// against a deliberately non-merged input via direct construction).
func TestHasContextAroundRegion_FullyCoveredAcrossAdjacentRanges(t *testing.T) {
	c := NewEvidenceClosure("")
	// SetReadRanges runs mergeLineRanges, so [1,50] + [51,100] becomes
	// [1,100]. That's the canonical state. Verify cover.
	c.SetReadRanges(map[string][]LineRange{
		"a.go": {{Start: 1, End: 50}, {Start: 51, End: 100}},
	})
	if !c.HasContextAroundRegion("a.go", 25, 75, 0) {
		t.Errorf("[25, 75] inside merged [1, 100] must be covered")
	}
}

// TestHasContextAroundRegion_GapBetweenRangesUncovered: when the
// merged ranges have a real gap inside the requested interval, the
// region is NOT fully covered.
func TestHasContextAroundRegion_GapBetweenRangesUncovered(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadRanges(map[string][]LineRange{
		"a.go": {{Start: 1, End: 50}, {Start: 80, End: 200}},
	})
	if c.HasContextAroundRegion("a.go", 40, 90, 0) {
		t.Errorf("gap at lines 51-79 must make [40, 90] uncovered")
	}
}

// TestHasContextAroundRegion_WindowExpands grows the requested
// interval by `window` on both sides — a strict ±0 might match while
// ±20 might not.
func TestHasContextAroundRegion_WindowExpands(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadRanges(map[string][]LineRange{
		"a.go": {{Start: 95, End: 105}},
	})
	if !c.HasContextAroundRegion("a.go", 100, 100, 0) {
		t.Errorf("single line 100 with window 0 inside [95, 105] must be covered")
	}
	if c.HasContextAroundRegion("a.go", 100, 100, 20) {
		t.Errorf("single line 100 with window 20 → [80, 120] is NOT inside [95, 105]")
	}
}

// TestHasContextAroundRegion_WindowClampsToOne checks the
// "expansion below line 1 stays at line 1" guard: if line 5 is
// requested with window 100, want.Start becomes 1, not -95.
func TestHasContextAroundRegion_WindowClampsToOne(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadRanges(map[string][]LineRange{
		"a.go": {{Start: 1, End: 200}},
	})
	if !c.HasContextAroundRegion("a.go", 5, 10, 100) {
		t.Errorf("window expanding below line 1 must clamp to 1, not negative")
	}
}

// TestHasContextAroundRegion_NilOrUnreadFile returns false defensively.
func TestHasContextAroundRegion_NilOrUnreadFile(t *testing.T) {
	if (*EvidenceClosure)(nil).HasContextAroundRegion("a.go", 1, 10, 0) {
		t.Error("nil closure must return false")
	}
	c := NewEvidenceClosure("")
	if c.HasContextAroundRegion("a.go", 1, 10, 0) {
		t.Error("never-read file must return false")
	}
}

// TestHasContextAroundRegion_MalformedInput returns false on
// invalid coordinates rather than silently treating them as covered.
func TestHasContextAroundRegion_MalformedInput(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadRanges(map[string][]LineRange{
		"a.go": {{Start: 1, End: 200}},
	})
	if c.HasContextAroundRegion("a.go", 0, 10, 0) {
		t.Error("start <= 0 must return false")
	}
	if c.HasContextAroundRegion("a.go", 10, 5, 0) {
		t.Error("end < start must return false")
	}
	if c.HasContextAroundRegion("", 1, 10, 0) {
		t.Error("empty file must return false")
	}
}

// TestMissingContextRegions_FullyCoveredReturnsEmpty: every requested
// region is already covered → no demand needed.
func TestMissingContextRegions_FullyCoveredReturnsEmpty(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadRanges(map[string][]LineRange{
		"a.go": {{Start: 1, End: 500}},
	})
	got := c.MissingContextRegions("a.go", []int{100, 200, 300, 400}, 10)
	if len(got) != 0 {
		t.Errorf("fully covered regions must produce empty demand, got %+v", got)
	}
}

// TestMissingContextRegions_PartiallyCoveredReturnsSlivers: a
// requested region overlapping a covered range produces demand for
// the uncovered slivers only — not the whole requested span.
func TestMissingContextRegions_PartiallyCoveredReturnsSlivers(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadRanges(map[string][]LineRange{
		"a.go": {{Start: 50, End: 100}},
	})
	// Requested [40-110] window 0 → covered slice is [50-100], missing
	// is [40-49] + [101-110].
	got := c.MissingContextRegions("a.go", []int{40, 110}, 0)
	want := []LineRange{{Start: 40, End: 49}, {Start: 101, End: 110}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("missing slivers = %+v, want %+v", got, want)
	}
}

// TestMissingContextRegions_MultipleRegionsMerged: two overlapping
// requested regions produce a single merged demand.
func TestMissingContextRegions_MultipleRegionsMerged(t *testing.T) {
	c := NewEvidenceClosure("")
	// Empty closure → every requested region's expanded window is
	// uncovered.
	got := c.MissingContextRegions("a.go", []int{100, 105, 108, 115}, 5)
	// Expanded windows: [95, 110] and [103, 120]. Merged: [95, 120].
	want := []LineRange{{Start: 95, End: 120}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merged missing = %+v, want %+v", got, want)
	}
}

// TestMissingContextRegions_WholeRegionUncovered: when nothing in
// the file is read yet, the demand is the expanded window verbatim.
func TestMissingContextRegions_WholeRegionUncovered(t *testing.T) {
	c := NewEvidenceClosure("")
	got := c.MissingContextRegions("a.go", []int{200, 220}, 5)
	want := []LineRange{{Start: 195, End: 225}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("uncovered demand = %+v, want %+v", got, want)
	}
}

// TestMissingContextRegions_MalformedSkipped: malformed pairs are
// silently dropped (consistent with the package's nil-tolerant API
// elsewhere) — they don't pollute the demand.
func TestMissingContextRegions_MalformedSkipped(t *testing.T) {
	c := NewEvidenceClosure("")
	got := c.MissingContextRegions("a.go", []int{0, 10, -5, 20, 100, 50}, 0)
	// All three pairs are malformed (start<=0, start<=0, end<start) → empty.
	if len(got) != 0 {
		t.Errorf("malformed pairs must be skipped, got %+v", got)
	}
}

// TestMissingContextRegions_OddLengthIgnoresTrailing: odd-length
// input drops the last unpaired int silently.
func TestMissingContextRegions_OddLengthIgnoresTrailing(t *testing.T) {
	c := NewEvidenceClosure("")
	got := c.MissingContextRegions("a.go", []int{100, 110, 200}, 0)
	want := []LineRange{{Start: 100, End: 110}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("odd trailing must be ignored, got %+v want %+v", got, want)
	}
}
