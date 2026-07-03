package tracequery

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeWindowSweepFixture(t *testing.T, name string, lines []string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.Join(append(lines, ""), "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func sweepSwitchLine(ts float64, prevComm string, prevPID int, prevState, nextComm string, nextPID int) string {
	return fmt.Sprintf("      %s-%d  (   %d) [001] .... %.6f: sched_switch: prev_comm=%s prev_pid=%d prev_prio=120 prev_state=%s ==> next_comm=%s next_pid=%d next_prio=120",
		prevComm, prevPID, prevPID, ts, prevComm, prevPID, prevState, nextComm, nextPID)
}

func sweepWakeupLine(ts float64, pid int) string {
	return fmt.Sprintf("    waker-10  (   10) [000] .... %.6f: sched_wakeup: comm=app pid=%d prio=53 target_cpu=001", ts, pid)
}

func sweepIRQEntryLine(ts float64) string {
	return fmt.Sprintf("      app-20  (   20) [001] .... %.6f: irq_handler_entry: irq=11 name=arch_timer", ts)
}

func sweepTraceMarkLine(ts float64) string {
	return fmt.Sprintf("      app-20  (   20) [001] .... %.6f: print: B|20|Choreographer#doFrame 170048", ts)
}

// TestStreamWindowSweepRanksHotspotsAndPinsBucketCounts pins §4.7 W3 core
// behavior on a synthetic multi-bucket density gradient: exact per-bucket
// counts, top-K ordering by global sched_switch density, and the advisory
// suggested-view mapping (above-average wakeups -> wakeup_chain,
// above-average D-state entries -> critical_blocking_calls).
func TestStreamWindowSweepRanksHotspotsAndPinsBucketCounts(t *testing.T) {
	// Bucket A [10.0,10.1): 3 switches (1 D-entry), 1 wakeup, 1 irq entry, 1 mark.
	// Bucket B [10.1,10.2): 1 switch.
	// Bucket C [10.3,10.4): 2 switches, 2 wakeups.
	path := writeWindowSweepFixture(t, "sweep_rank.systrace", []string{
		sweepSwitchLine(10.020, "app", 20, "S", "other", 30),
		sweepSwitchLine(10.040, "other", 30, "D", "app", 20),
		sweepWakeupLine(10.050, 20),
		sweepIRQEntryLine(10.060),
		sweepTraceMarkLine(10.070),
		sweepSwitchLine(10.080, "app", 20, "R", "other", 30),
		sweepSwitchLine(10.120, "other", 30, "S", "app", 20),
		sweepSwitchLine(10.320, "app", 20, "S", "other", 30),
		sweepWakeupLine(10.330, 20),
		sweepWakeupLine(10.340, 20),
		sweepSwitchLine(10.350, "other", 30, "S", "app", 20),
	})
	res, err := StreamWindowSweep(context.Background(), path, Query{
		TimeStart:    10.0,
		TimeEnd:      10.5,
		TimeStartSet: true,
		TimeEndSet:   true,
		BucketMs:     100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.View != ViewWindowSweep {
		t.Fatalf("view = %q, want %q", res.View, ViewWindowSweep)
	}
	sweep := res.WindowSweep
	if sweep == nil {
		t.Fatalf("missing WindowSweep result: %+v", res)
	}
	if sweep.BucketMs != 100 {
		t.Fatalf("bucket_ms = %g, want 100", sweep.BucketMs)
	}
	if sweep.RankBasis != WindowSweepRankBasisGlobal {
		t.Fatalf("rank basis = %q, want %q", sweep.RankBasis, WindowSweepRankBasisGlobal)
	}
	// The observed grid spans buckets [10.0,10.1) .. [10.3,10.4) -> 4 buckets.
	if sweep.BucketCount != 4 {
		t.Fatalf("bucket_count = %d, want 4", sweep.BucketCount)
	}
	if len(sweep.Hotspots) != 3 {
		t.Fatalf("expected 3 hotspots (zero-switch buckets are not hotspots), got %+v", sweep.Hotspots)
	}
	a, c, b := sweep.Hotspots[0], sweep.Hotspots[1], sweep.Hotspots[2]
	if a.Rank != 1 || a.SchedSwitches != 3 || a.SchedWakeups != 1 || a.DStateEntries != 1 || a.IRQEntries != 1 || a.TraceMarks != 1 {
		t.Fatalf("bucket A counts wrong: %+v", a)
	}
	if math.Abs(a.StartTs-10.0) > 0.011 || math.Abs(a.EndTs-10.1) > 0.011 {
		t.Fatalf("bucket A window wrong: %+v", a)
	}
	if c.Rank != 2 || c.SchedSwitches != 2 || c.SchedWakeups != 2 {
		t.Fatalf("bucket C counts wrong: %+v", c)
	}
	if math.Abs(c.StartTs-10.3) > 0.011 {
		t.Fatalf("bucket C window wrong: %+v", c)
	}
	if b.Rank != 3 || b.SchedSwitches != 1 {
		t.Fatalf("bucket B counts wrong: %+v", b)
	}
	// Advisory view mapping: A has above-average wakeups AND the only D-entry;
	// C has above-average wakeups only; B has neither.
	if got := strings.Join(a.SuggestedViews, ","); got != "window_stats,frame_window,wakeup_chain,critical_blocking_calls" {
		t.Fatalf("bucket A suggested views = %q", got)
	}
	if got := strings.Join(c.SuggestedViews, ","); got != "window_stats,frame_window,wakeup_chain" {
		t.Fatalf("bucket C suggested views = %q", got)
	}
	if got := strings.Join(b.SuggestedViews, ","); got != "window_stats,frame_window" {
		t.Fatalf("bucket B suggested views = %q", got)
	}
	// Unfolded coverage: 4 grid rows (including the quiet zero bucket).
	if sweep.CoverageFolded || len(sweep.Coverage) != 4 {
		t.Fatalf("expected 4 unfolded coverage rows, got folded=%v rows=%d", sweep.CoverageFolded, len(sweep.Coverage))
	}
	quiet := sweep.Coverage[2]
	if quiet.SchedSwitches != 0 || quiet.SchedWakeups != 0 || quiet.Buckets != 1 {
		t.Fatalf("quiet bucket row should be zero-filled: %+v", quiet)
	}
	if !containsSubstring(res.Caveats, "streamed_window_sweep=true") {
		t.Fatalf("missing streaming caveat: %+v", res.Caveats)
	}
	if !containsSubstring(res.Caveats, "advisory") {
		t.Fatalf("soft-guidance caveat must be present: %+v", res.Caveats)
	}
}

// TestStreamWindowSweepTargetPIDParticipation pins the pid lane: switch-in and
// switch-out rows both count as participation, ranking uses participation
// density, and an unobserved pid falls back to global density with a caveat.
func TestStreamWindowSweepTargetPIDParticipation(t *testing.T) {
	// Bucket A [20.0,20.1): 3 switches but only 1 involving pid 77.
	// Bucket B [20.1,20.2): 2 switches, both involving pid 77 (in + out).
	path := writeWindowSweepFixture(t, "sweep_pid.systrace", []string{
		sweepSwitchLine(20.020, "other", 30, "S", "peer", 40),
		sweepSwitchLine(20.040, "peer", 40, "S", "other", 30),
		sweepSwitchLine(20.060, "other", 30, "S", "target", 77),
		sweepSwitchLine(20.120, "target", 77, "S", "other", 30),
		sweepSwitchLine(20.140, "other", 30, "S", "target", 77),
	})
	res, err := StreamWindowSweep(context.Background(), path, Query{
		TimeStart:    20.0,
		TimeEnd:      20.3,
		TimeStartSet: true,
		TimeEndSet:   true,
		PID:          77,
		BucketMs:     100,
	})
	if err != nil {
		t.Fatal(err)
	}
	sweep := res.WindowSweep
	if sweep == nil {
		t.Fatal("missing WindowSweep result")
	}
	if sweep.TargetPID != 77 || sweep.RankBasis != WindowSweepRankBasisTargetPID {
		t.Fatalf("target pid ranking not applied: %+v", sweep)
	}
	if len(sweep.Hotspots) != 2 {
		t.Fatalf("expected 2 hotspots, got %+v", sweep.Hotspots)
	}
	first, second := sweep.Hotspots[0], sweep.Hotspots[1]
	// Bucket B leads on participation (2 > 1) despite fewer total switches.
	if first.TargetPIDSwitches != 2 || first.SchedSwitches != 2 || math.Abs(first.StartTs-20.1) > 0.011 {
		t.Fatalf("participation-ranked first hotspot wrong: %+v", first)
	}
	if second.TargetPIDSwitches != 1 || second.SchedSwitches != 3 {
		t.Fatalf("second hotspot wrong: %+v", second)
	}

	// Unobserved pid: fall back to global density, named in a caveat.
	res2, err := StreamWindowSweep(context.Background(), path, Query{
		TimeStart:    20.0,
		TimeEnd:      20.3,
		TimeStartSet: true,
		TimeEndSet:   true,
		PID:          9999,
		BucketMs:     100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res2.WindowSweep.RankBasis != WindowSweepRankBasisGlobal {
		t.Fatalf("unobserved pid must fall back to global rank basis: %+v", res2.WindowSweep)
	}
	if !containsSubstring(res2.WindowSweep.Caveats, "fell back to global sched_switch density") {
		t.Fatalf("fallback caveat missing: %+v", res2.WindowSweep.Caveats)
	}
}

// TestStreamWindowSweepFoldsCoverageOver40Buckets pins the compact coverage
// contract: >40 buckets fold equidistantly to at most 40 annotated rows while
// per-row counts stay exact sums.
func TestStreamWindowSweepFoldsCoverageOver40Buckets(t *testing.T) {
	var lines []string
	// 100 buckets at 100ms: one switch per bucket, 30.0 .. 39.9.
	for k := 0; k < 100; k++ {
		lines = append(lines, sweepSwitchLine(30.05+float64(k)*0.1, "app", 20, "S", "other", 30))
	}
	path := writeWindowSweepFixture(t, "sweep_fold.systrace", lines)
	res, err := StreamWindowSweep(context.Background(), path, Query{
		TimeStart:    30.0,
		TimeEnd:      40.0,
		TimeStartSet: true,
		TimeEndSet:   true,
		BucketMs:     100,
	})
	if err != nil {
		t.Fatal(err)
	}
	sweep := res.WindowSweep
	if sweep == nil {
		t.Fatal("missing WindowSweep result")
	}
	if sweep.BucketCount != 100 {
		t.Fatalf("bucket_count = %d, want 100", sweep.BucketCount)
	}
	if !sweep.CoverageFolded || sweep.CoverageFoldSpan != 3 {
		t.Fatalf("expected fold span 3, got folded=%v span=%d", sweep.CoverageFolded, sweep.CoverageFoldSpan)
	}
	if len(sweep.Coverage) != 34 || len(sweep.Coverage) > WindowSweepCoverageMaxRows {
		t.Fatalf("expected 34 folded rows (<=40), got %d", len(sweep.Coverage))
	}
	if sweep.Coverage[0].Buckets != 3 || sweep.Coverage[0].SchedSwitches != 3 {
		t.Fatalf("folded row must sum its buckets: %+v", sweep.Coverage[0])
	}
	last := sweep.Coverage[len(sweep.Coverage)-1]
	if last.Buckets != 1 || last.SchedSwitches != 1 {
		t.Fatalf("tail row must carry the remainder: %+v", last)
	}
	if !containsSubstring(sweep.Caveats, "coverage table folded 100 bucket(s) into 34 row(s) (3 bucket(s) per row") {
		t.Fatalf("fold caveat missing: %+v", sweep.Caveats)
	}
	// Top-K stays at the default 8 hotspots.
	if len(sweep.Hotspots) != WindowSweepDefaultTopK {
		t.Fatalf("expected default top-K %d hotspots, got %d", WindowSweepDefaultTopK, len(sweep.Hotspots))
	}
}

// TestClampWindowSweepBucketMsBoundaries pins the exact clamp: default on
// unset/non-positive, 50..500 bounds, identical handling for integer-valued
// and fractional inputs.
func TestClampWindowSweepBucketMsBoundaries(t *testing.T) {
	for input, want := range map[float64]float64{
		0:       100,
		-5:      100,
		49:      50,
		49.999:  50,
		50:      50,
		50.5:    50.5,
		100:     100,
		499.999: 499.999,
		500:     500,
		500.001: 500,
		10000:   500,
	} {
		if got := ClampWindowSweepBucketMs(input); got != want {
			t.Fatalf("ClampWindowSweepBucketMs(%g) = %g, want %g", input, got, want)
		}
	}
	if got := ClampWindowSweepBucketMs(math.NaN()); got != WindowSweepDefaultBucketMs {
		t.Fatalf("NaN must collapse to the default, got %g", got)
	}
}

// TestStreamWindowSweepClampsBucketMsEndToEnd pins that an out-of-range
// bucket_ms is clamped on the wire result and annotated, and that a scope with
// zero counted events yields the zero-signal caveat instead of fake coverage.
func TestStreamWindowSweepClampsBucketMsEndToEnd(t *testing.T) {
	path := writeWindowSweepFixture(t, "sweep_clamp.systrace", []string{
		sweepSwitchLine(50.010, "app", 20, "S", "other", 30),
		sweepSwitchLine(50.030, "other", 30, "S", "app", 20),
	})
	res, err := StreamWindowSweep(context.Background(), path, Query{
		TimeStart:    50.0,
		TimeEnd:      50.2,
		TimeStartSet: true,
		TimeEndSet:   true,
		BucketMs:     10, // below the 50ms floor
	})
	if err != nil {
		t.Fatal(err)
	}
	sweep := res.WindowSweep
	if sweep.BucketMs != WindowSweepMinBucketMs || sweep.RequestedBucketMs != 10 {
		t.Fatalf("clamp not applied: %+v", sweep)
	}
	if !containsSubstring(sweep.Caveats, "bucket_ms=10 clamped to 50 (allowed 50..500)") {
		t.Fatalf("clamp caveat missing: %+v", sweep.Caveats)
	}

	empty, err := StreamWindowSweep(context.Background(), path, Query{
		TimeStart:    900.0,
		TimeEnd:      910.0,
		TimeStartSet: true,
		TimeEndSet:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if empty.WindowSweep == nil || empty.WindowSweep.BucketCount != 0 || len(empty.WindowSweep.Hotspots) != 0 {
		t.Fatalf("empty scope must yield zero buckets: %+v", empty.WindowSweep)
	}
	if !containsSubstring(empty.WindowSweep.Caveats, "observed zero counted scheduler/irq/trace-mark events") {
		t.Fatalf("zero-signal caveat missing: %+v", empty.WindowSweep.Caveats)
	}
}

// TestStreamWindowSweepDoesNotClaimWindowedIndexParse pins the streaming-scan
// identity of the view (StreamEventSearch precedent): a windowed sweep must
// NOT surface the windowed_index_parse caveat — that caveat describes a
// bounded INDEX parse and tells the model to "omit the window to build the
// full index", i.e. it steers straight back into the index_event_limit loop
// window_sweep exists to break. The streamed_window_sweep caveat is the
// authoritative scope statement.
func TestStreamWindowSweepDoesNotClaimWindowedIndexParse(t *testing.T) {
	path := writeWindowSweepFixture(t, "sweep_not_windowed.systrace", []string{
		sweepSwitchLine(10.020, "app", 20, "S", "other", 30),
		sweepSwitchLine(10.120, "other", 30, "S", "app", 20),
	})
	res, err := StreamWindowSweep(context.Background(), path, Query{
		TimeStart:    10.0,
		TimeEnd:      10.5,
		TimeStartSet: true,
		TimeEndSet:   true,
		BucketMs:     100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if containsSubstring(res.Caveats, "windowed_index_parse") {
		t.Fatalf("window_sweep must not claim a windowed index parse: %+v", res.Caveats)
	}
	if !containsSubstring(res.Caveats, "streamed_window_sweep=true") {
		t.Fatalf("streaming scope caveat missing: %+v", res.Caveats)
	}
	if res.IndexWindowed {
		t.Fatalf("window_sweep result must not advertise a windowed index: %+v", res)
	}
}

// TestStreamWindowSweepThreadNameWithoutPIDCaveat pins the F3 transparency
// rule: a thread selector that carries no embedded pid cannot drive the
// pid-participation rank basis, and that fallback must be NAMED (same "not
// consumed" style as the pattern caveat), never silent. A selector with an
// embedded pid keeps the pid lane and gets no such caveat.
func TestStreamWindowSweepThreadNameWithoutPIDCaveat(t *testing.T) {
	path := writeWindowSweepFixture(t, "sweep_thread.systrace", []string{
		sweepSwitchLine(20.020, "other", 30, "S", "target", 77),
		sweepSwitchLine(20.120, "target", 77, "S", "other", 30),
	})
	res, err := StreamWindowSweep(context.Background(), path, Query{
		TimeStart:    20.0,
		TimeEnd:      20.3,
		TimeStartSet: true,
		TimeEndSet:   true,
		Thread:       "RenderThread",
		BucketMs:     100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.WindowSweep.RankBasis != WindowSweepRankBasisGlobal {
		t.Fatalf("pure-name thread must rank globally: %+v", res.WindowSweep)
	}
	if !containsSubstring(res.Caveats, "thread name without an embedded pid is not consumed by view=window_sweep ranking") {
		t.Fatalf("silent thread-name drop: caveat missing: %+v", res.Caveats)
	}

	// Embedded pid: consumed via the pid lane — no drop caveat.
	withPID, err := StreamWindowSweep(context.Background(), path, Query{
		TimeStart:    20.0,
		TimeEnd:      20.3,
		TimeStartSet: true,
		TimeEndSet:   true,
		Thread:       "target-77",
		BucketMs:     100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if withPID.WindowSweep.RankBasis != WindowSweepRankBasisTargetPID || withPID.WindowSweep.TargetPID != 77 {
		t.Fatalf("embedded pid must drive participation ranking: %+v", withPID.WindowSweep)
	}
	if containsSubstring(withPID.Caveats, "thread name without an embedded pid") {
		t.Fatalf("embedded-pid selector must not get the drop caveat: %+v", withPID.Caveats)
	}
}

// TestWindowSweepBucketBoundaryAttribution pins the grid-boundary ulp fix:
// a ts sitting EXACTLY on a published bucket boundary must land in the bucket
// whose advertised [StartTs, EndTs) window contains it. 4.3/0.05 floors to 85
// (85.999... one ulp low) while the published boundary float64(86)*0.05 ==
// 4.2999999999999998 <= 4.3, so pre-fix the event was attributed to a bucket
// whose published window excludes it.
func TestWindowSweepBucketBoundaryAttribution(t *testing.T) {
	bucketSec := 50.0 / 1000
	buckets := map[int64]*WindowSweepBucketCounts{}
	ts := 4.3 // what parseLineTimestamp yields for "4.300000"
	windowSweepBucketFor(buckets, ts, bucketSec).SchedSwitches++
	if len(buckets) != 1 {
		t.Fatalf("expected exactly one bucket, got %+v", buckets)
	}
	for key := range buckets {
		if key != 86 {
			t.Fatalf("boundary ts must attribute to bucket 86, got %d", key)
		}
	}
	// Property over a dense boundary range: attribution always agrees with
	// the published-window formula.
	for k := int64(1); k <= 2000; k++ {
		bts := float64(k) * bucketSec
		grid := map[int64]*WindowSweepBucketCounts{}
		windowSweepBucketFor(grid, bts, bucketSec).SchedSwitches++
		for key := range grid {
			start, end := float64(key)*bucketSec, float64(key+1)*bucketSec
			if !(bts >= start && bts < end) {
				t.Fatalf("ts=%.17g attributed to bucket %d whose published window [%.17g,%.17g) excludes it", bts, key, start, end)
			}
		}
	}

	// End-to-end: the hotspot window advertised for the boundary event must
	// contain the event's ts.
	path := writeWindowSweepFixture(t, "sweep_boundary.systrace", []string{
		sweepSwitchLine(4.300000, "app", 20, "S", "other", 30),
	})
	res, err := StreamWindowSweep(context.Background(), path, Query{
		TimeStart:    4.0,
		TimeEnd:      4.5,
		TimeStartSet: true,
		TimeEndSet:   true,
		BucketMs:     50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.WindowSweep.Hotspots) != 1 {
		t.Fatalf("expected 1 hotspot, got %+v", res.WindowSweep.Hotspots)
	}
	hot := res.WindowSweep.Hotspots[0]
	if !(hot.StartTs <= 4.3 && 4.3 < hot.EndTs) {
		t.Fatalf("boundary event ts=4.3 outside its published hotspot window [%.17g,%.17g)", hot.StartTs, hot.EndTs)
	}
}

// TestStreamWindowSweepRecordsAnchorsAndSeeksIdentically pins the P1 anchor
// reuse on the sweep channel: a cold sweep over an un-anchored file records
// anchors + flavor into the shared per-file cache (so later builds/sweeps can
// seek), and a warm re-sweep of the same window — now served via anchor seek —
// returns an identical WindowSweep section (pure latency, never counts).
func TestStreamWindowSweepRecordsAnchorsAndSeeksIdentically(t *testing.T) {
	n := 3 * traceAnchorLineInterval
	path := anchorTestTrace(t, n, 0)
	winStart := 100.0 + float64(2*traceAnchorLineInterval)*0.0001
	q := Query{
		TimeStart:    winStart,
		TimeEnd:      winStart + 0.2,
		TimeStartSet: true,
		TimeEndSet:   true,
		BucketMs:     100,
	}

	resetAnchorCaches()
	cold, err := StreamWindowSweep(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(canonicalTraceIndexPath(path))
	if err != nil {
		t.Fatal(err)
	}
	key := traceAnchorKey{path: canonicalTraceIndexPath(path), size: info.Size(), modUnix: info.ModTime().UnixNano(), version: ParserVersion}
	set := anchorCache.load(key)
	if set == nil || len(set.Anchors) == 0 || !set.FlavorSet {
		t.Fatalf("cold sweep must record anchors + flavor, got %+v", set)
	}

	warm, err := StreamWindowSweep(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cold.WindowSweep, warm.WindowSweep) {
		t.Fatalf("anchor-seek sweep diverged:\ncold=%+v\nwarm=%+v", cold.WindowSweep, warm.WindowSweep)
	}
	if cold.EventCount != warm.EventCount || cold.ScannedLineCount != warm.ScannedLineCount {
		t.Fatalf("scan bookkeeping diverged: cold events=%d lines=%d warm events=%d lines=%d",
			cold.EventCount, cold.ScannedLineCount, warm.EventCount, warm.ScannedLineCount)
	}
	if cold.TraceFlavor != warm.TraceFlavor {
		t.Fatalf("flavor must be stable across seek sweeps: %q vs %q", cold.TraceFlavor, warm.TraceFlavor)
	}
}

// TestIndexEventLimitRecoverySteersToWindowSweepOnLongWindows pins the §4.7
// denial-guidance condition: only an explicit request window STRICTLY longer
// than 1s puts window_sweep first in recovery_params; shorter windows keep
// the historical event_search-first sentence byte-compatible.
func TestIndexEventLimitRecoverySteersToWindowSweepOnLongWindows(t *testing.T) {
	path := writeWindowSweepFixture(t, "sweep_denial.systrace", []string{
		sweepWakeupLine(2.000, 20),
		sweepWakeupLine(2.001, 20),
		sweepWakeupLine(2.002, 20),
		sweepWakeupLine(2.003, 20),
	})
	buildOpts := func(end float64) BuildOptions {
		return BuildOptions{
			TimeStart:          2.0,
			TimeEnd:            end,
			TimeStartSet:       true,
			TimeEndSet:         true,
			AllowWindowedParse: true,
			MaxEvents:          3,
		}
	}
	var limitErr *IndexEventLimitError
	_, err := BuildIndexWithOptions(context.Background(), path, buildOpts(3.5))
	if !errors.As(err, &limitErr) {
		t.Fatalf("expected IndexEventLimitError, got %T %v", err, err)
	}
	long := limitErr.Error()
	for _, want := range []string{
		"recovery_params: the requested window spans 1.500s",
		"run view=window_sweep FIRST with the SAME time_start/time_end",
		"NOT subject to this index event budget",
		"then use view=event_search (streaming scan",
	} {
		if !strings.Contains(long, want) {
			t.Fatalf("long-window recovery missing %q:\n%s", want, long)
		}
	}

	// 0.5s window: strictly below the threshold — no window_sweep steer, and
	// the historical event_search-first sentence stays.
	_, err = BuildIndexWithOptions(context.Background(), path, buildOpts(2.5))
	if !errors.As(err, &limitErr) {
		t.Fatalf("expected short-window IndexEventLimitError, got %T %v", err, err)
	}
	short := limitErr.Error()
	if strings.Contains(short, ViewWindowSweep) {
		t.Fatalf("sub-1s window must not steer to window_sweep:\n%s", short)
	}
	if !strings.Contains(short, "recovery_params: use view=event_search (streaming scan") {
		t.Fatalf("historical short-window recovery sentence changed:\n%s", short)
	}

	// Exactly 1.0s: the condition is STRICTLY greater — no steer.
	_, err = BuildIndexWithOptions(context.Background(), path, buildOpts(3.0))
	if !errors.As(err, &limitErr) {
		t.Fatalf("expected 1s-window IndexEventLimitError, got %T %v", err, err)
	}
	if strings.Contains(limitErr.Error(), ViewWindowSweep) {
		t.Fatalf("exactly-1s window must not steer to window_sweep:\n%s", limitErr.Error())
	}
}
