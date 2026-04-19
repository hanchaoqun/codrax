package types

import (
	"reflect"
	"testing"
)

// TestHasReadLine_FileNotInReadSet locks the "never saw it" branch:
// HasReadLine must reject even when ranges were submitted for the
// file. This guards against a caller accidentally populating ranges
// without first recording the file in readSet — the range data alone
// is not a grant.
func TestHasReadLine_FileNotInReadSet(t *testing.T) {
	c := NewEvidenceClosure()
	c.AddReadRanges(map[string][]LineRange{
		"a.go": {{Start: 1, End: 100}},
	})
	if c.HasReadLine("a.go", 50) {
		t.Error("HasReadLine must reject when file is not in readSet")
	}
}

// TestHasReadLine_FileInReadSetNoRanges verifies backward compatibility:
// old tests that only call SetReadSet (no ranges) must still see a
// file-level grant from HasReadLine. Otherwise adding range support
// would break every existing chain promotion test.
func TestHasReadLine_FileInReadSetNoRanges(t *testing.T) {
	c := NewEvidenceClosure()
	c.SetReadSet(map[string]bool{"a.go": true})
	if !c.HasReadLine("a.go", 50) {
		t.Error("HasReadLine must grant file-level coverage when no ranges are tracked")
	}
	if !c.HasReadLine("a.go", 0) {
		t.Error("line == 0 must grant (no line hint)")
	}
}

// TestHasReadLine_WithRanges covers the precise row-level semantics:
// inside a range = hit, outside = miss. This is the new behaviour that
// lets CGEC I1 demote chains whose anchor falls in an unread segment.
func TestHasReadLine_WithRanges(t *testing.T) {
	c := NewEvidenceClosure()
	c.SetReadSet(map[string]bool{"a.go": true})
	c.AddReadRanges(map[string][]LineRange{
		"a.go": {
			{Start: 1, End: 30},
			{Start: 145, End: 220},
		},
	})
	cases := map[int]bool{
		1:   true,  // boundary
		15:  true,
		30:  true,  // right boundary slice 1
		31:  false, // in gap
		144: false,
		145: true, // left boundary slice 2
		200: true,
		220: true,
		221: false,
		500: false,
	}
	for line, want := range cases {
		if got := c.HasReadLine("a.go", line); got != want {
			t.Errorf("line %d: got %v, want %v", line, got, want)
		}
	}
}

// TestAddReadRanges_Accumulates mirrors read_file pagination: two
// separate calls with disjoint ranges must both end up covered.
func TestAddReadRanges_Accumulates(t *testing.T) {
	c := NewEvidenceClosure()
	c.SetReadSet(map[string]bool{"a.go": true})
	c.AddReadRanges(map[string][]LineRange{
		"a.go": {{Start: 1, End: 50}},
	})
	c.AddReadRanges(map[string][]LineRange{
		"a.go": {{Start: 100, End: 150}},
	})
	if !c.HasReadLine("a.go", 25) {
		t.Error("line 25 must be covered by the first Add call")
	}
	if !c.HasReadLine("a.go", 120) {
		t.Error("line 120 must be covered by the second Add call")
	}
	if c.HasReadLine("a.go", 75) {
		t.Error("line 75 falls in the gap between the two calls")
	}
}

// TestAddReadRanges_MergesAdjacent: two adjacent read_file calls
// ([1-100] and [101-200]) should produce a single merged range so the
// diagnostic dump stays clean.
func TestAddReadRanges_MergesAdjacent(t *testing.T) {
	c := NewEvidenceClosure()
	c.SetReadSet(map[string]bool{"a.go": true})
	c.AddReadRanges(map[string][]LineRange{
		"a.go": {{Start: 1, End: 100}},
	})
	c.AddReadRanges(map[string][]LineRange{
		"a.go": {{Start: 101, End: 200}},
	})
	got := c.ReadRanges("a.go")
	want := []LineRange{{Start: 1, End: 200}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merge failed: got %v, want %v", got, want)
	}
}

// TestSetReadRanges_ReplacesSnapshot: explorer.ParseOutput calls
// SetReadRanges once per round after extractFileCoverage — prior
// ranges must not leak forward when the tool history is the
// authoritative source.
func TestSetReadRanges_ReplacesSnapshot(t *testing.T) {
	c := NewEvidenceClosure()
	c.SetReadSet(map[string]bool{"a.go": true})
	c.AddReadRanges(map[string][]LineRange{
		"a.go": {{Start: 1, End: 50}},
	})
	c.SetReadRanges(map[string][]LineRange{
		"a.go": {{Start: 200, End: 300}},
	})
	if c.HasReadLine("a.go", 25) {
		t.Error("old range [1-50] leaked after SetReadRanges replaced the snapshot")
	}
	if !c.HasReadLine("a.go", 250) {
		t.Error("new range [200-300] must be active after SetReadRanges")
	}
}

// TestReset_ClearsReadRanges: ResetEvidenceClosure must wipe range
// records, otherwise a prior task's partial reads would grant
// row-level coverage on the next task's unrelated files.
func TestReset_ClearsReadRanges(t *testing.T) {
	c := NewEvidenceClosure()
	c.SetReadSet(map[string]bool{"a.go": true})
	c.AddReadRanges(map[string][]LineRange{
		"a.go": {{Start: 1, End: 50}},
	})
	c.Reset()
	if c.HasReadLine("a.go", 25) {
		t.Error("Reset did not clear readRanges")
	}
	if got := c.ReadRanges("a.go"); got != nil {
		t.Errorf("ReadRanges after Reset: got %v, want nil", got)
	}
}

// TestReadRanges_DefensiveCopy: mutating the returned slice must not
// corrupt closure internals.
func TestReadRanges_DefensiveCopy(t *testing.T) {
	c := NewEvidenceClosure()
	c.SetReadSet(map[string]bool{"a.go": true})
	c.AddReadRanges(map[string][]LineRange{
		"a.go": {{Start: 1, End: 50}},
	})
	got := c.ReadRanges("a.go")
	got[0].End = 999
	if again := c.ReadRanges("a.go"); again[0].End != 50 {
		t.Errorf("mutation leaked into closure: %v", again)
	}
}

// TestAddReadRanges_DropsInvalid: malformed ranges are filtered by
// the merge pass, so callers do not need to pre-validate.
func TestAddReadRanges_DropsInvalid(t *testing.T) {
	c := NewEvidenceClosure()
	c.SetReadSet(map[string]bool{"a.go": true})
	c.AddReadRanges(map[string][]LineRange{
		"a.go": {
			{Start: 0, End: 10},  // Start must be ≥ 1
			{Start: 20, End: 10}, // End < Start
			{Start: 50, End: 60}, // keep
		},
	})
	got := c.ReadRanges("a.go")
	want := []LineRange{{Start: 50, End: 60}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestHasReadLine_NilClosure: belt-and-braces defence — nil receiver
// is a common test-table pattern and must not crash.
func TestHasReadLine_NilClosure(t *testing.T) {
	var c *EvidenceClosure
	if c.HasReadLine("a.go", 5) {
		t.Error("nil closure must return false")
	}
}
