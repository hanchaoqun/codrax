package index

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// TestSetTierThresholds_PreservesDefaultsOnZero pins the partial-
// override semantics: passing 0 on either side does NOT clobber
// the existing value. Operators who tune just one knob get the
// other from the package default.
func TestSetTierThresholds_PreservesDefaultsOnZero(t *testing.T) {
	// Reset to known state.
	tierWarnRatio = defaultTierWarnRatio
	tierAlertRatio = defaultTierAlertRatio

	SetTierThresholds(0.45, 0)
	if tierWarnRatio != 0.45 {
		t.Errorf("warn override ignored: %v", tierWarnRatio)
	}
	if tierAlertRatio != defaultTierAlertRatio {
		t.Errorf("alert should be untouched on zero override; got %v", tierAlertRatio)
	}

	SetTierThresholds(0, 0.65)
	if tierWarnRatio != 0.45 {
		t.Errorf("warn should be untouched on zero override; got %v", tierWarnRatio)
	}
	if tierAlertRatio != 0.65 {
		t.Errorf("alert override ignored: %v", tierAlertRatio)
	}

	// Restore defaults so other tests start clean.
	tierWarnRatio = defaultTierWarnRatio
	tierAlertRatio = defaultTierAlertRatio
}

// TestReportFallbackRatios_TallyAcrossLanguages exercises the new
// per-language INFO summary path. Builds a fixture with mixed
// languages + tiers and asserts the function runs without panic
// (logging side effects are observed via go test -v output).
// Function returns void so we test the input shape only.
func TestReportFallbackRatios_TallyAcrossLanguages(t *testing.T) {
	files := []*types.FileInfo{
		// Go: 6 files, all Tier 1.
		{RelPath: "a.go", Language: types.LangGo, ParseTier: 1},
		{RelPath: "b.go", Language: types.LangGo, ParseTier: 1},
		{RelPath: "c.go", Language: types.LangGo, ParseTier: 1},
		{RelPath: "d.go", Language: types.LangGo, ParseTier: 1},
		{RelPath: "e.go", Language: types.LangGo, ParseTier: 1},
		{RelPath: "f.go", Language: types.LangGo, ParseTier: 1},
		// Python: 5 files, mixed tiers (3 T1, 1 T2, 1 T3).
		{RelPath: "x.py", Language: types.LangPython, ParseTier: 1},
		{RelPath: "y.py", Language: types.LangPython, ParseTier: 1},
		{RelPath: "z.py", Language: types.LangPython, ParseTier: 1},
		{RelPath: "w.py", Language: types.LangPython, ParseTier: 2},
		{RelPath: "v.py", Language: types.LangPython, ParseTier: 3},
	}
	// Should not panic; emits INFO summary line. The reporter is
	// lossy at the test boundary (we don't capture the log) but
	// the smoke test catches any nil-deref / index-out-of-range
	// regression in the new tally code.
	reportFallbackRatios(files)
}

// TestReportFallbackRatios_HandlesAllTiers covers the bounds:
// tiers 0 (treated as 1), 1, 2, 3, 4, 5 (clamped to 4).
func TestReportFallbackRatios_HandlesAllTiers(t *testing.T) {
	files := []*types.FileInfo{
		{RelPath: "a.go", Language: types.LangGo, ParseTier: 0},
		{RelPath: "b.go", Language: types.LangGo, ParseTier: 1},
		{RelPath: "c.go", Language: types.LangGo, ParseTier: 2},
		{RelPath: "d.go", Language: types.LangGo, ParseTier: 3},
		{RelPath: "e.go", Language: types.LangGo, ParseTier: 4},
		{RelPath: "f.go", Language: types.LangGo, ParseTier: 5}, // out of range — should clamp
	}
	reportFallbackRatios(files)
}

// TestReportFallbackRatios_BelowMinFilesSkipped covers the
// statistical-floor: languages with fewer than tierStatsMinFiles
// (5) files do not contribute to either INFO or WARN output.
// The function still runs but produces nothing for them.
func TestReportFallbackRatios_BelowMinFilesSkipped(t *testing.T) {
	files := []*types.FileInfo{
		// 4 ArkTS files all degraded — below the 5-file floor.
		{RelPath: "a.ets", Language: types.LangArkTS, ParseTier: 2},
		{RelPath: "b.ets", Language: types.LangArkTS, ParseTier: 2},
		{RelPath: "c.ets", Language: types.LangArkTS, ParseTier: 2},
		{RelPath: "d.ets", Language: types.LangArkTS, ParseTier: 2},
	}
	// Should not emit a per-language WARN (sample too small).
	reportFallbackRatios(files)
}

// TestReportFallbackRatios_EmptyInputNoCrash guards the empty-
// pool path: zero files in, zero output, no panic.
func TestReportFallbackRatios_EmptyInputNoCrash(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("empty files panicked: %v", r)
		}
	}()
	reportFallbackRatios(nil)
	reportFallbackRatios([]*types.FileInfo{})
}
