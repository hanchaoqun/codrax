package types

import (
	"reflect"
	"testing"
)

func TestMergeLineRanges_EmptyAndNil(t *testing.T) {
	if got := mergeLineRanges(nil); got != nil {
		t.Errorf("nil input: got %v, want nil", got)
	}
	if got := mergeLineRanges([]LineRange{}); got != nil {
		t.Errorf("empty input: got %v, want nil", got)
	}
}

func TestMergeLineRanges_DropsMalformed(t *testing.T) {
	in := []LineRange{
		{Start: 0, End: 5},   // Start must be ≥ 1
		{Start: -3, End: 10}, // negative
		{Start: 20, End: 15}, // End < Start
		{Start: 10, End: 20}, // keep
	}
	want := []LineRange{{Start: 10, End: 20}}
	got := mergeLineRanges(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeLineRanges_SortsAndMergesOverlap(t *testing.T) {
	in := []LineRange{
		{Start: 50, End: 60},
		{Start: 10, End: 20},
		{Start: 15, End: 30},
		{Start: 55, End: 70},
	}
	want := []LineRange{
		{Start: 10, End: 30},
		{Start: 50, End: 70},
	}
	got := mergeLineRanges(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeLineRanges_MergesAdjacent(t *testing.T) {
	// End+1 == next.Start → adjacent → merge.
	in := []LineRange{
		{Start: 10, End: 20},
		{Start: 21, End: 30},
		{Start: 31, End: 40},
	}
	want := []LineRange{{Start: 10, End: 40}}
	got := mergeLineRanges(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeLineRanges_PreservesDisjoint(t *testing.T) {
	in := []LineRange{
		{Start: 10, End: 20},
		{Start: 30, End: 40}, // gap at 21-29 → no merge
	}
	got := mergeLineRanges(in)
	if !reflect.DeepEqual(got, in) {
		t.Errorf("got %v, want %v", got, in)
	}
}

func TestMergeLineRanges_Idempotent(t *testing.T) {
	merged := mergeLineRanges([]LineRange{
		{Start: 1, End: 10},
		{Start: 5, End: 15},
	})
	again := mergeLineRanges(merged)
	if !reflect.DeepEqual(merged, again) {
		t.Errorf("not idempotent: first=%v second=%v", merged, again)
	}
}

func TestMergeLineRanges_SingleLineRange(t *testing.T) {
	in := []LineRange{{Start: 42, End: 42}}
	want := []LineRange{{Start: 42, End: 42}}
	got := mergeLineRanges(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestRangesContain_Empty(t *testing.T) {
	if rangesContain(nil, 5) {
		t.Error("nil ranges must not contain anything")
	}
	if rangesContain([]LineRange{}, 5) {
		t.Error("empty ranges must not contain anything")
	}
}

func TestRangesContain_InvalidLine(t *testing.T) {
	sorted := []LineRange{{Start: 1, End: 10}}
	if rangesContain(sorted, 0) {
		t.Error("line 0 must not be contained")
	}
	if rangesContain(sorted, -5) {
		t.Error("negative line must not be contained")
	}
}

func TestRangesContain_InsideOutside(t *testing.T) {
	sorted := []LineRange{
		{Start: 10, End: 20},
		{Start: 30, End: 40},
	}
	cases := map[int]bool{
		5:  false, // before first
		10: true,  // left boundary
		15: true,  // inside first
		20: true,  // right boundary
		21: false, // in gap
		29: false, // in gap
		30: true,  // left boundary 2
		35: true,  // inside second
		40: true,  // right boundary 2
		41: false, // after last
	}
	for line, want := range cases {
		if got := rangesContain(sorted, line); got != want {
			t.Errorf("line %d: got %v, want %v", line, got, want)
		}
	}
}

func TestRangesContain_ManyRanges(t *testing.T) {
	// 20 disjoint ranges: [1,5], [11,15], [21,25], ..., [191,195].
	sorted := make([]LineRange, 0, 20)
	for i := 0; i < 20; i++ {
		sorted = append(sorted, LineRange{Start: i*10 + 1, End: i*10 + 5})
	}
	if !rangesContain(sorted, 151) {
		t.Error("151 should be the left boundary of [151, 155]")
	}
	if !rangesContain(sorted, 155) {
		t.Error("155 should be the right boundary of [151, 155]")
	}
	if rangesContain(sorted, 156) {
		t.Error("156 should be in the gap [156, 160]")
	}
	if rangesContain(sorted, 200) {
		t.Error("200 should be past the last range (ends at 195)")
	}
}

func TestCloneLineRanges_IsDefensive(t *testing.T) {
	src := []LineRange{{Start: 1, End: 5}, {Start: 10, End: 15}}
	dst := cloneLineRanges(src)
	if !reflect.DeepEqual(src, dst) {
		t.Fatalf("clone not equal: %v vs %v", src, dst)
	}
	dst[0].End = 999
	if src[0].End != 5 {
		t.Error("clone mutation leaked back into src")
	}
}

// === LineToReadFileOffset (2026-05-10 customer-reported off-by-one fix) ===

// TestLineToReadFileOffset_HappyPath pins the canonical mapping. The
// customer-reported phantom forced-read loop on context.go:1322
// reduced to: 1-based line 1322 → 0-based offset 1321.
func TestLineToReadFileOffset_HappyPath(t *testing.T) {
	cases := []struct {
		line     int
		wantOff  int
		describe string
	}{
		{1, 0, "first 1-based line maps to offset 0"},
		{1322, 1321, "customer-reported case: line 1322 → offset 1321"},
		{3928, 3927, "last line of context.go (3928 lines) → offset 3927"},
	}
	for _, tc := range cases {
		t.Run(tc.describe, func(t *testing.T) {
			got := LineToReadFileOffset(tc.line)
			if got != tc.wantOff {
				t.Errorf("LineToReadFileOffset(%d) = %d, want %d", tc.line, got, tc.wantOff)
			}
		})
	}
}

// TestLineToReadFileOffset_ZeroAndNegativeClampToZero — defensive
// against uninitialised LineRange.Start (zero value) so the helper
// is safe to call inline without an explicit pre-check at every
// emit site.
func TestLineToReadFileOffset_ZeroAndNegativeClampToZero(t *testing.T) {
	for _, line := range []int{0, -1, -1000} {
		if got := LineToReadFileOffset(line); got != 0 {
			t.Errorf("LineToReadFileOffset(%d) = %d, want 0 (clamped)", line, got)
		}
	}
}

// TestLineToReadFileOffset_RoundTripWithBanner pins the round-trip
// against read_file's banner contract:
//
//   - LLM passes offset = LineToReadFileOffset(line).
//   - read_file slices allLines[offset:] and emits a banner with
//     `showing lines (offset+1)-…` (1-based).
//   - parseReadFileBanner records line (offset+1) onward as covered.
//   - The original 1-based `line` is in the covered set, so the
//     forced-read demand is satisfied on the first attempt.
//
// Without LineToReadFileOffset, the LLM would pass the 1-based
// line directly as offset, read line+1, and never satisfy the
// demand for `line`.
func TestLineToReadFileOffset_RoundTripWithBanner(t *testing.T) {
	for _, line := range []int{1, 5, 100, 1322} {
		offset := LineToReadFileOffset(line)
		// read_file's banner reports `sliceStart+1` (1-based) where
		// sliceStart == offset (0-based). So the banner's first
		// covered line is offset+1, which must equal `line`.
		bannerFirstLine := offset + 1
		if bannerFirstLine != line {
			t.Errorf("line %d → offset %d → banner shows line %d (want %d)",
				line, offset, bannerFirstLine, line)
		}
	}
}
