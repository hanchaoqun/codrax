package tracequery

// platform_surfaces_w1_test.go — W-1 platform 标签同报告翻转 pins (CLOSE-1 批,
// ledger real_trace_campaign_20260705.md §29.28 ② W-1, 2026-07-11).
//
// Witness isomorph (customlogs/cust710/open_gap_witness_02.txt): a
// harmony-base trace whose android-framework comms (surfaceflinger /
// android.ui) appear ONLY in unknown-type print rows while the trace_mark /
// workqueue rows carry harmony comms. Pre-fix, event_search resolved the
// platform from its QUERY-MATCHED event subset, so event_types=[trace_mark]
// said harmony while event_types=[unknown] said donghu in the same run
// (修前红 — the multi-view consistency pin below fails on the pre-fix code
// by construction).
//
// 突变自查 (each applied locally and verified red before ship):
//   (m1) consumption reverted to the matched-only event subset →
//        TestW1PlatformLabelStableAcrossEventTypeFilters;
//   (m2) from-0 minting removed → TestW1FromZeroScanMintsAnchorRecord;
//   (m3) vote hook moved AFTER the eventInQuery filter →
//        TestW1PlatformLabelStableAcrossEventTypeFilters (same red as m1).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// w1WitnessIsomorphTrace writes the witness-isomorphic artifact: harmony
// base (OS_FFRT scheduler rows + render_service trace marks) plus android
// framework comms confined to unknown-type print rows.
func w1WitnessIsomorphTrace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// The file name carries the hitrace token so the FLAVOR lane stays
	// stable for every query shape including pattern-prefiltered scans
	// (the witness artifact resolved flavor=harmony_hitrace(conf=0.98) on
	// every step; the flavor lane is deliberately untouched by this batch).
	path := filepath.Join(dir, "w1_mixed_hitrace.systrace")
	// NOTE (fixture faithfulness): the customer build typed the witness's
	// N|/I| print rows as `unknown`; current HEAD types them trace_mark
	// (remote ATRACE-TRACK batch, §29.28 W-6), so the android comms here ride
	// genuinely-unrecognized event names to KEEP them reachable only through
	// event_types=[unknown] — the load-bearing witness property.
	trace := strings.Join([]string{
		`  sample.app-36379 (36379) [004] .... 2941.780000: sched_switch: prev_comm=sample.app prev_pid=36379 prev_prio=53 prev_state=S ==> next_comm=OS_FFRT_0_0 next_pid=49634 next_prio=20`,
		`     OS_FFRT_0_0-49634 (48679) [000] .... 2941.781000: sched_wakeup: comm=render_service pid=1716 prio=97 target_cpu=004`,
		` 	render_service-1716  ( 1716) [003] .... 2941.788619: print: B|1716|H:RSUniRender::QuickPrepare`,
		` 	render_service-1716  ( 1716) [003] .... 2941.788621: print: E|1716`,
		`    kworker/u25:6-215   (   72) [001] ...1 2941.825877: workqueue_execute_start: work struct 0x582c145840: function 0x612b000728f26174`,
		`       android.ui-10851 (10756) [007] .... 2941.790188: vendor_sensor_event: from android.sensor.accelerometer`,
		`  	surfaceflinger-10377 (10377) [003] .... 2941.797149: vendor_fence_probe: fence at idx: 4 expectedPresentTime in -51.73`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestW1PlatformLabelStableAcrossEventTypeFilters is the 修前红 pin: three
// event_search queries over the SAME trace with different event_types
// filters must publish ONE platform label. Pre-fix the [trace_mark] and
// [workqueue] queries said harmony while [unknown] said donghu (the witness
// flip); the per-trace record makes all of them donghu (the file carries
// harmony base + android framework surfaces).
func TestW1PlatformLabelStableAcrossEventTypeFilters(t *testing.T) {
	path := w1WitnessIsomorphTrace(t)
	ctx := context.Background()
	platforms := map[string]string{}
	flavors := map[string]string{}
	confidences := map[string]float64{}
	// Deterministic order, trace_mark FIRST: the witness ran the harmony-only
	// filters before the android-bearing [unknown] step, and the write-once
	// record makes the FIRST determination authoritative — a mutation that
	// narrows the vote to matched rows must therefore flip the final labels.
	for _, tc := range []struct {
		name  string
		types []EventType
	}{
		{"trace_mark", []EventType{EventTraceMark}},
		{"workqueue", []EventType{EventWorkqueue}},
		{"unknown", []EventType{EventUnknown}},
	} {
		name, types := tc.name, tc.types
		res, err := StreamEventSearch(ctx, path, Query{
			View:       "event_search",
			EventTypes: types,
			Limit:      50,
		})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		platforms[name] = res.Platform
		flavors[name] = res.TraceFlavor
		confidences[name] = res.FlavorConfidence
	}
	if platforms["trace_mark"] != platforms["unknown"] || platforms["workqueue"] != platforms["unknown"] {
		t.Fatalf("W-1 修前红: the platform label flipped with the event_types filter: %+v", platforms)
	}
	if platforms["unknown"] != string(TracePlatformDonghu) {
		t.Fatalf("the mixed harmony-base + android-surface file resolves donghu on every view, got %+v", platforms)
	}
	// 负对照 (flavor conf 面不受扰): the flavor lane publishes the same
	// flavor + confidence on every one of the three views — the platform
	// record must not perturb the flavor vote.
	for name := range platforms {
		if flavors[name] != flavors["unknown"] || confidences[name] != confidences["unknown"] {
			t.Fatalf("flavor/confidence face disturbed across views: flavors=%+v confidences=%+v", flavors, confidences)
		}
	}
}

// TestW1IndexedAndStreamingViewsShareOnePlatformLabel pins the cross-lane
// half: the indexed Run lane (windowed, filtered), stream_state_cluster and
// window_sweep all consume the same per-trace record as event_search.
func TestW1IndexedAndStreamingViewsShareOnePlatformLabel(t *testing.T) {
	path := w1WitnessIsomorphTrace(t)
	ctx := context.Background()
	labels := map[string]string{}

	streamRes, err := StreamEventSearch(ctx, path, Query{View: "event_search", EventTypes: []EventType{EventTraceMark}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	labels["stream_event_search"] = streamRes.Platform

	idx, err := BuildIndex(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	runRes := Run(idx, Query{
		View:       "event_search",
		TimeStart:  2941.78,
		TimeEnd:    2941.79,
		EventTypes: []EventType{EventTraceMark},
		Limit:      10,
	})
	labels["indexed_run_windowed"] = runRes.Platform

	clusterRes, err := StreamStateCluster(ctx, path, Query{}, 8)
	if err != nil {
		t.Fatal(err)
	}
	labels["stream_state_cluster"] = clusterRes.Platform

	sweepRes, err := StreamWindowSweep(ctx, path, Query{View: "window_sweep"})
	if err != nil {
		t.Fatal(err)
	}
	labels["stream_window_sweep"] = sweepRes.Platform

	want := labels["stream_event_search"]
	for name, got := range labels {
		if got != want {
			t.Fatalf("同 trace 多 view 标签必须一致: %s=%q vs stream_event_search=%q (%+v)", name, got, want, labels)
		}
	}
	if want != string(TracePlatformDonghu) {
		t.Fatalf("the witness-isomorph file resolves donghu, got %+v", labels)
	}
}

// TestW1PureHarmonyTraceUndisturbed is the second 负对照: a trace with NO
// android framework comm keeps platform=harmony on every filter, and the
// flavor face stays byte-stable — the record lane must not invent a mixed
// candidate where no android surface exists.
func TestW1PureHarmonyTraceUndisturbed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "w1_pure.hitrace")
	trace := strings.Join([]string{
		`  sample.app-100 (100) [000] .... 10.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=OS_FFRT_0_0 next_pid=200 next_prio=20`,
		` 	render_service-1716  ( 1716) [003] .... 10.100000: print: B|1716|H:RSUniRender::QuickPrepare`,
		` 	render_service-1716  ( 1716) [003] .... 10.100500: print: E|1716`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, types := range [][]EventType{{EventTraceMark}, nil} {
		res, err := StreamEventSearch(ctx, path, Query{View: "event_search", EventTypes: types, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if res.Platform != string(TracePlatformHarmony) {
			t.Fatalf("a pure-harmony trace stays platform=harmony (filter %v): %+v", types, res.Platform)
		}
		if res.PlatformCandidate != "" {
			t.Fatalf("no mixed candidate may mint without an android surface: %q", res.PlatformCandidate)
		}
	}
}

func w1HasPartialBasisCaveat(caveats []string) bool {
	for _, c := range caveats {
		if strings.Contains(c, "platform_detection_basis=partial") {
			return true
		}
	}
	return false
}

// TestW1MintEligibilityAndPartialBasisDisclosure pins 复核 F2+F3: an
// INELIGIBLE scan (pattern prefilter / window — its parsed coverage is not
// the complete artifact) never mints the per-file record and its label is
// disclosed as partial-basis; the next ELIGIBLE scan mints the complete
// record; from then on the same ineligible query shape consumes the record
// with the right label and ZERO disclosure.
func TestW1MintEligibilityAndPartialBasisDisclosure(t *testing.T) {
	path := w1WitnessIsomorphTrace(t)
	ctx := context.Background()
	canon := canonicalTraceIndexPath(path)
	info, err := os.Stat(canon)
	if err != nil {
		t.Fatal(err)
	}
	loadRecord := func() platformSurfaceScan {
		set := anchorCache.load(traceAnchorKeyForInfo(canon, info))
		if set == nil {
			return platformSurfaceScan{}
		}
		return set.PlatformSurfaces
	}
	// 1) pattern-prefiltered scan: no mint + partial-basis disclosure.
	res1, err := StreamEventSearch(ctx, path, Query{View: "event_search", Pattern: "vendor_sensor_event", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if loadRecord().Set {
		t.Fatalf("F2: a pattern-prefiltered scan must not mint the per-file record")
	}
	if !w1HasPartialBasisCaveat(res1.Caveats) {
		t.Fatalf("F3: a record-less filtered scan must disclose its partial basis:\n%+v", res1.Caveats)
	}
	// 1b) time-windowed scan: same — no mint, disclosed.
	resW, err := StreamEventSearch(ctx, path, Query{View: "event_search", TimeStart: 2941.78, TimeEnd: 2941.79, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if loadRecord().Set {
		t.Fatalf("F2: a windowed scan must not mint the per-file record")
	}
	if !w1HasPartialBasisCaveat(resW.Caveats) {
		t.Fatalf("F3: a record-less windowed scan must disclose its partial basis:\n%+v", resW.Caveats)
	}
	// 2) eligible scan (type filters do not narrow the parse): mints the
	//    COMPLETE record, publishes with zero disclosure.
	res2, err := StreamEventSearch(ctx, path, Query{View: "event_search", EventTypes: []EventType{EventUnknown}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	record := loadRecord()
	if !record.Set || record.Scoped || !record.Android || !record.Harmony {
		t.Fatalf("F2: the eligible scan must mint the complete record: %+v", record)
	}
	if res2.Platform != string(TracePlatformDonghu) || w1HasPartialBasisCaveat(res2.Caveats) {
		t.Fatalf("a complete record publishes donghu with zero disclosure: platform=%q caveats=%+v", res2.Platform, res2.Caveats)
	}
	// 3) the formerly-ineligible query shape now consumes the record: same
	//    label, zero disclosure.
	res3, err := StreamEventSearch(ctx, path, Query{View: "event_search", Pattern: "vendor_sensor_event", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if res3.Platform != res2.Platform || w1HasPartialBasisCaveat(res3.Caveats) {
		t.Fatalf("F2 序: post-mint the filtered shape consumes the record (label %q vs %q) with zero disclosure:\n%+v", res3.Platform, res2.Platform, res3.Caveats)
	}
}

// TestW1FromZeroScanMintsAnchorRecord pins the seek precondition: the SAME
// from-0 scan that writes the flavor writes the platform record, so by the
// time any anchor seek is possible (seeks require FlavorSet) the per-file
// platform authority already exists — a seeked scan can never re-derive a
// divergent label from its suffix-only view.
func TestW1FromZeroScanMintsAnchorRecord(t *testing.T) {
	path := w1WitnessIsomorphTrace(t)
	if _, err := StreamEventSearch(context.Background(), path, Query{View: "event_search", EventTypes: []EventType{EventTraceMark}, Limit: 10}); err != nil {
		t.Fatal(err)
	}
	canon := canonicalTraceIndexPath(path)
	info, err := os.Stat(canon)
	if err != nil {
		t.Fatal(err)
	}
	set := anchorCache.load(traceAnchorKeyForInfo(canon, info))
	if set == nil {
		t.Fatalf("the from-0 scan must persist an anchor set")
	}
	if !set.FlavorSet {
		t.Fatalf("fixture drifted: the from-0 scan writes the flavor")
	}
	if !set.PlatformSurfaces.Set {
		t.Fatalf("seek 前置条件: FlavorSet without PlatformSurfaces.Set — a seeked scan could re-derive a divergent label")
	}
	if !set.PlatformSurfaces.Android || !set.PlatformSurfaces.Harmony {
		t.Fatalf("the record must carry the full-scan surface truth: %+v", set.PlatformSurfaces)
	}
}
