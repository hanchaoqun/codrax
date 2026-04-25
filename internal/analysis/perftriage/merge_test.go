package perftriage

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestMergePerfBundles_NilEmpty(t *testing.T) {
	if got := MergePerfBundles(nil, 0); got != nil {
		t.Errorf("nil input: want nil bundle, got %+v", got)
	}
	if got := MergePerfBundles([]*types.PerfBundle{}, 0); got != nil {
		t.Errorf("empty input: want nil bundle, got %+v", got)
	}
}

func TestMergePerfBundles_SingleReturnsAsIs(t *testing.T) {
	in := &types.PerfBundle{Meta: types.PerfMeta{Source: "hitrace"}}
	got := MergePerfBundles([]*types.PerfBundle{in}, 100)
	if got != in {
		t.Errorf("single input: want pass-through pointer, got %+v", got)
	}
}

func TestMergePerfBundles_SignalsUnion_FramesConcatDedup(t *testing.T) {
	a := &types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace", Signals: []string{"jank"}},
		Frames: []types.PerfFrame{
			{FrameNo: 1, TsMs: 100, DurationMs: 20.5, Janky: true},
		},
		Janks: []types.PerfJank{
			{StartTsMs: 100, DurationMs: 20.5, TriggerSpan: "DoFrame"},
		},
	}
	b := &types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace", Signals: []string{"main-thread-stall"}},
		Frames: []types.PerfFrame{
			{FrameNo: 1, TsMs: 100, DurationMs: 20.5, Janky: true}, // dup
			{FrameNo: 2, TsMs: 130, DurationMs: 8.0},
		},
		Janks: []types.PerfJank{
			{StartTsMs: 200, DurationMs: 50.0, TriggerSpan: "fetchSync"},
		},
		Stalls: []types.PerfStall{
			{StartTsMs: 200, DurationMs: 45, Symbol: "DataLoader.fetchSync",
				File: "entry/src/main/ets/svc/DataLoader.ets", Line: 42},
		},
		Startup: &types.PerfStartup{Mode: "cold", AppLaunchMs: 1500.0},
	}
	merged := MergePerfBundles([]*types.PerfBundle{a, b}, 1000)
	if merged == nil {
		t.Fatal("merged nil")
	}
	if len(merged.Frames) != 2 {
		t.Errorf("frames: want 2 (after dedup), got %d", len(merged.Frames))
	}
	if len(merged.Janks) != 2 {
		t.Errorf("janks: want 2, got %d", len(merged.Janks))
	}
	if len(merged.Stalls) != 1 {
		t.Errorf("stalls: want 1, got %d", len(merged.Stalls))
	}
	// Signals union (sorted).
	if len(merged.Meta.Signals) != 2 {
		t.Errorf("signals union: got %v", merged.Meta.Signals)
	}
	// Startup picks largest app_launch_ms.
	if merged.Startup == nil || merged.Startup.AppLaunchMs != 1500.0 {
		t.Errorf("startup: %+v", merged.Startup)
	}
	// Layer 4 derived.
	if merged.IntentHint != "performance" {
		t.Errorf("IntentHint: %q", merged.IntentHint)
	}
	if len(merged.ResolvedFiles) != 1 {
		t.Errorf("ResolvedFiles: %v", merged.ResolvedFiles)
	}
	// Coverage = 1 - 0/1000 = 1.0
	if merged.Coverage != 1.0 {
		t.Errorf("coverage: %v", merged.Coverage)
	}
}

func TestMergePerfBundles_ResidueDedup_CoverageDrops(t *testing.T) {
	a := &types.PerfBundle{
		Meta:    types.PerfMeta{Source: "atrace"},
		Residue: []string{"unparsed-line-A", "common-residue"},
	}
	b := &types.PerfBundle{
		Meta:    types.PerfMeta{Source: "atrace"},
		Residue: []string{"common-residue", "unparsed-line-B"},
	}
	merged := MergePerfBundles([]*types.PerfBundle{a, b}, 100)
	if len(merged.Residue) != 3 {
		t.Errorf("residue dedup: want 3 unique, got %d (%v)", len(merged.Residue), merged.Residue)
	}
	if merged.Coverage >= 1.0 {
		t.Errorf("coverage should drop with residue: got %v", merged.Coverage)
	}
}
