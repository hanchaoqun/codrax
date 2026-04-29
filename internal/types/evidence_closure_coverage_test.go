package types

import "testing"

// Smoke test for the per-file coverage API the multi-path parity
// gate consumes. Pre-2026-04-29 the gate compared absolute line
// counts against max(read_lines), which made a fully-read 113-line
// file fail to keep up with a partially-read 683-line sibling.
// These tests pin the new contract: per-file CoverageRatio +
// HasFullyRead are the only inputs the gate may use.

func TestEvidenceClosure_FullyReadShortFile(t *testing.T) {
	c := NewEvidenceClosure("")
	// Mirror the production trace: systrace_analyzer.py is 131 lines,
	// the LLM read 1-131 in one paginated banner.
	c.AddReadRanges(map[string][]LineRange{
		"systrace_analyzer.py": {{Start: 1, End: 131}},
	})
	c.RecordFileTotalLines("systrace_analyzer.py", 131)

	if !c.HasFullyRead("systrace_analyzer.py") {
		t.Fatalf("FullyRead must be true for a 131-line file with merged range [1,131]; got false")
	}
	if got := c.CoverageRatio("systrace_analyzer.py"); got != 1.0 {
		t.Fatalf("CoverageRatio = %.3f, want 1.0", got)
	}
	if got := c.MergedReadLines("systrace_analyzer.py"); got != 131 {
		t.Fatalf("MergedReadLines = %d, want 131", got)
	}
}

func TestEvidenceClosure_PartialReadRatio(t *testing.T) {
	c := NewEvidenceClosure("")
	// 683-line hitrace_analyzer.py with two non-contiguous read
	// slices: [1,400] and [450,500]. Merged total = 400 + 51 = 451.
	c.AddReadRanges(map[string][]LineRange{
		"hitrace_analyzer.py": {{Start: 1, End: 400}, {Start: 450, End: 500}},
	})
	c.RecordFileTotalLines("hitrace_analyzer.py", 683)

	if c.HasFullyRead("hitrace_analyzer.py") {
		t.Fatalf("FullyRead must be false for partial coverage 451/683; got true")
	}
	got := c.CoverageRatio("hitrace_analyzer.py")
	want := 451.0 / 683.0
	if got < want-0.001 || got > want+0.001 {
		t.Fatalf("CoverageRatio = %.4f, want ~%.4f", got, want)
	}
}

func TestEvidenceClosure_TotalUnknownReturnsMinusOne(t *testing.T) {
	c := NewEvidenceClosure("")
	// Range recorded but no banner total — for example a forced-read
	// injection that bypassed parseReadFileBanner.
	c.AddReadRanges(map[string][]LineRange{
		"foo.go": {{Start: 1, End: 50}},
	})
	if got := c.CoverageRatio("foo.go"); got != -1 {
		t.Fatalf("CoverageRatio without recorded total = %.3f, want -1", got)
	}
	if c.HasFullyRead("foo.go") {
		t.Fatalf("HasFullyRead without recorded total must be false")
	}
}

func TestEvidenceClosure_OverlappingRangesCoverageOne(t *testing.T) {
	c := NewEvidenceClosure("")
	// Two overlapping read calls that, after merge, cover [1,100]
	// in a 100-line file. Without merge the absolute sum would be
	// 60 + 60 = 120 — over-reporting. mergeLineRanges must collapse.
	c.AddReadRanges(map[string][]LineRange{
		"x.go": {{Start: 1, End: 60}, {Start: 41, End: 100}},
	})
	c.RecordFileTotalLines("x.go", 100)

	if got := c.MergedReadLines("x.go"); got != 100 {
		t.Fatalf("MergedReadLines = %d, want 100 (after merge)", got)
	}
	if !c.HasFullyRead("x.go") {
		t.Fatalf("HasFullyRead = false, want true")
	}
	if got := c.CoverageRatio("x.go"); got != 1.0 {
		t.Fatalf("CoverageRatio = %.3f, want 1.0", got)
	}
}

func TestEvidenceClosure_SetFileTotalLinesAtomicReplace(t *testing.T) {
	c := NewEvidenceClosure("")
	c.RecordFileTotalLines("a.go", 100)
	c.SetFileTotalLines(map[string]int{
		"b.go": 50,
		// Note: a.go intentionally absent — SetFileTotalLines is an
		// atomic replace, so a.go should be cleared.
	})
	if got := c.FileTotalLines("a.go"); got != 0 {
		t.Fatalf("after Set replace, a.go total = %d, want 0", got)
	}
	if got := c.FileTotalLines("b.go"); got != 50 {
		t.Fatalf("b.go total = %d, want 50", got)
	}
}
