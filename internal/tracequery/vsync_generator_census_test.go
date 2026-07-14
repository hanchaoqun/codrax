package tracequery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// vsync_generator_census_test.go — SA-F2 (DISPATCH-IND 批4, 2026-07-14)
// engine pins. Every expectation below is PRODUCTION-MINTED ground truth from
// the two real customer captures in eval/fixtures/real_traces, cross-checked
// against raw shell surveys recorded in the batch ledger:
//
//	donghu_tieba_frame.systrace — VSyncGenerator-1682 (tgid 1252):
//	  32 emitted rows (irq 2 + sched_switch 4 + sched_wakeup 3 + trace_mark 23),
//	  2 sched_wakeup rows targeting it, 2 GenerateVsyncCount prints, all
//	  period:16552213 (currRefreshRate_:0 — no set rate in this capture).
//	donghu.ftrace — VSyncGenerator-2179 (tgid 1864): 531 emitted rows,
//	  46 wakeups targeting it, 39 GenerateVsyncCount prints, all
//	  period:11091679 with currRefreshRate:90; plus hwc_vsync_threa-9443
//	  (tgid 9394): 88 emitted rows, 22 wakeups targeting it, zero period
//	  prints.
//
// Raw-survey cross-checks (run from eval/fixtures/real_traces/):
//
//	grep -cE '^\s*VSyncGenerator-[0-9]+' donghu_tieba_frame.systrace → 32
//	grep -c 'comm=VSyncGenerator pid=1682' donghu_tieba_frame.systrace → 2
//	grep -c 'GenerateVsyncCount' donghu_tieba_frame.systrace → 2
//	grep -cE '^\s*VSyncGenerator-[0-9]+' donghu.ftrace → 531
//	grep -c 'GenerateVsyncCount' donghu.ftrace → 39

const (
	vsyncCensusTiebaTrace  = "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace"
	vsyncCensusDonghuTrace = "../../eval/fixtures/real_traces/donghu.ftrace"
)

func vsyncCensusIndex(t *testing.T, path string) *Index {
	t.Helper()
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatalf("BuildIndex(%s): %v", path, err)
	}
	return idx
}

func TestVsyncGeneratorWindowCensus_TiebaRealTrace(t *testing.T) {
	idx := vsyncCensusIndex(t, vsyncCensusTiebaTrace)
	census := ComputeVsyncGeneratorWindowCensus(idx, Query{})
	if census == nil {
		t.Fatalf("expected a generator census on the tieba capture, got nil")
	}
	if census.Caliber != VsyncGeneratorCensusCaliberWindow {
		t.Fatalf("caliber = %q, want %q", census.Caliber, VsyncGeneratorCensusCaliberWindow)
	}
	if len(census.Threads) != 1 {
		t.Fatalf("threads = %d, want exactly 1 (VSyncGenerator-1682); got %+v", len(census.Threads), census.Threads)
	}
	g := census.Threads[0]
	if g.Thread.Comm != "VSyncGenerator" || g.Thread.PID != 1682 || g.Thread.TGID != 1252 {
		t.Fatalf("generator identity = %+v, want VSyncGenerator-1682 (tgid 1252)", g.Thread)
	}
	if g.EventCount != 32 {
		t.Errorf("EventCount = %d, want 32 (raw survey: 32 emitted rows)", g.EventCount)
	}
	if g.TraceMarkCount != 23 {
		t.Errorf("TraceMarkCount = %d, want 23", g.TraceMarkCount)
	}
	if g.WokenCount != 2 {
		t.Errorf("WokenCount = %d, want 2 (raw survey: 2 sched_wakeup rows target it)", g.WokenCount)
	}
	if g.PeriodPrintRows != 2 {
		t.Errorf("PeriodPrintRows = %d, want 2", g.PeriodPrintRows)
	}
	if g.IdentifiedBy != "thread_name+period_print" {
		t.Errorf("IdentifiedBy = %q, want thread_name+period_print", g.IdentifiedBy)
	}
	if len(g.Periods) != 1 {
		t.Fatalf("Periods = %+v, want exactly one distinct value", g.Periods)
	}
	p := g.Periods[0]
	// The witness's authoritative period: verbatim ns from the generator's
	// own print (`GenerateVsyncCount:1, period:16552213, currRefreshRate_:0`)
	// ≈ 16.552ms ≈ 60Hz — NOT the 124.14ms consumer-callback spacing.
	if p.PeriodNs != 16552213 || p.Rows != 2 {
		t.Fatalf("period = %+v, want period_ns=16552213 rows=2", p)
	}
	if p.RefreshRate != 0 {
		t.Errorf("RefreshRate = %d, want 0 (the tieba print carries currRefreshRate_:0 — absence, never a guess)", p.RefreshRate)
	}
	if ms := p.PeriodMs(); ms < 16.55 || ms > 16.56 {
		t.Errorf("PeriodMs = %.6f, want ≈16.552", ms)
	}
}

func TestVsyncGeneratorWindowCensus_DonghuRealTrace(t *testing.T) {
	idx := vsyncCensusIndex(t, vsyncCensusDonghuTrace)
	census := ComputeVsyncGeneratorWindowCensus(idx, Query{})
	if census == nil {
		t.Fatalf("expected a generator census on the donghu capture, got nil")
	}
	if len(census.Threads) != 2 {
		t.Fatalf("threads = %d, want 2 (VSyncGenerator-2179 + hwc_vsync_threa-9443); got %+v", len(census.Threads), census.Threads)
	}
	gen := census.Threads[0]
	if gen.Thread.Comm != "VSyncGenerator" || gen.Thread.PID != 2179 || gen.Thread.TGID != 1864 {
		t.Fatalf("threads[0] = %+v, want VSyncGenerator-2179 (tgid 1864) first (event-count order)", gen.Thread)
	}
	if gen.EventCount != 531 || gen.WokenCount != 46 || gen.PeriodPrintRows != 39 {
		t.Errorf("VSyncGenerator account = events %d woken %d period_prints %d, want 531/46/39",
			gen.EventCount, gen.WokenCount, gen.PeriodPrintRows)
	}
	if len(gen.Periods) != 1 || gen.Periods[0].PeriodNs != 11091679 || gen.Periods[0].Rows != 39 {
		t.Fatalf("VSyncGenerator periods = %+v, want one value period_ns=11091679 rows=39", gen.Periods)
	}
	if gen.Periods[0].RefreshRate != 90 {
		t.Errorf("RefreshRate = %d, want 90 (donghu print carries currRefreshRate:90)", gen.Periods[0].RefreshRate)
	}
	hwc := census.Threads[1]
	if hwc.Thread.Comm != "hwc_vsync_threa" || hwc.Thread.PID != 9443 {
		t.Fatalf("threads[1] = %+v, want hwc_vsync_threa-9443", hwc.Thread)
	}
	if hwc.EventCount != 88 || hwc.WokenCount != 22 {
		t.Errorf("hwc account = events %d woken %d, want 88/22", hwc.EventCount, hwc.WokenCount)
	}
	if len(hwc.Periods) != 0 || hwc.PeriodPrintRows != 0 {
		t.Errorf("hwc periods = %+v prints=%d, want none (the HW vsync thread emits no period print)", hwc.Periods, hwc.PeriodPrintRows)
	}
	if hwc.IdentifiedBy != "thread_name" {
		t.Errorf("hwc IdentifiedBy = %q, want thread_name", hwc.IdentifiedBy)
	}
}

func TestVsyncGeneratorWindowCensus_WindowScoped(t *testing.T) {
	idx := vsyncCensusIndex(t, vsyncCensusTiebaTrace)
	// The jank window (34579.472865..34579.475857) contains NO generator
	// activity — the generator's last sighting is at 34579.468449. Absence
	// stays absence: nil census, byte-identical result face.
	census := ComputeVsyncGeneratorWindowCensus(idx, Query{TimeStart: 34579.472865, TimeEnd: 34579.475857})
	if census != nil {
		t.Fatalf("expected nil census in the generator-free jank window, got %+v", census)
	}
	// A window covering the first generator burst holds exactly one period
	// print (the second lives at ~34579.468383).
	census = ComputeVsyncGeneratorWindowCensus(idx, Query{TimeStart: 34579.450627, TimeEnd: 34579.460000})
	if census == nil || len(census.Threads) != 1 {
		t.Fatalf("expected the generator in the first-burst window, got %+v", census)
	}
	if census.Threads[0].PeriodPrintRows != 1 {
		t.Errorf("first-burst period prints = %d, want 1", census.Threads[0].PeriodPrintRows)
	}
}

func TestVsyncGeneratorSearchCensus_MatchedRowsCaliber(t *testing.T) {
	idx := vsyncCensusIndex(t, vsyncCensusTiebaTrace)
	// The witness dispatch shape: event_search pattern=vsync. The matched
	// set includes the generator's own rows, so the census carries the
	// authoritative period even when the display rows are truncated.
	census := ComputeVsyncGeneratorSearchCensus(idx, Query{View: "event_search", Pattern: "vsync"})
	if census == nil {
		t.Fatalf("expected a matched-rows census for pattern=vsync, got nil")
	}
	if census.Caliber != VsyncGeneratorCensusCaliberMatched {
		t.Fatalf("caliber = %q, want %q", census.Caliber, VsyncGeneratorCensusCaliberMatched)
	}
	if len(census.Threads) != 1 || census.Threads[0].Thread.PID != 1682 {
		t.Fatalf("threads = %+v, want VSyncGenerator-1682", census.Threads)
	}
	if census.Threads[0].PeriodPrintRows != 2 || len(census.Threads[0].Periods) != 1 ||
		census.Threads[0].Periods[0].PeriodNs != 16552213 {
		t.Fatalf("matched-rows census must carry both period prints (16552213ns), got %+v", census.Threads[0])
	}
	// A search whose matched set touches no generator row mints nothing —
	// unrelated queries stay byte-identical.
	if c := ComputeVsyncGeneratorSearchCensus(idx, Query{View: "event_search", Pattern: "binder_transaction"}); c != nil {
		t.Fatalf("expected nil census for a non-vsync search, got %+v", c)
	}
}

// TestVsyncGeneratorSearchCensus_TargetFilterImmune pins the run-2 witness
// fix (2026-07-14): the tool auto-injects the request's runtime target into
// target-less event_search calls, and the tieba target (the CONSUMER thread
// com.baidu.tieba-59566) filtered out every generator-emitted row — the
// census degenerated to events=0/woken=1 with no period. The census admission
// deliberately drops the pid/thread arms on BOTH twins.
func TestVsyncGeneratorSearchCensus_TargetFilterImmune(t *testing.T) {
	idx := vsyncCensusIndex(t, vsyncCensusTiebaTrace)
	assertFull := func(label string, census *VsyncGeneratorCensus) {
		t.Helper()
		if census == nil || len(census.Threads) != 1 {
			t.Fatalf("%s: census = %+v, want the full VSyncGenerator-1682 account", label, census)
		}
		g := census.Threads[0]
		if g.Thread.PID != 1682 || g.EventCount != 32 || g.PeriodPrintRows != 2 ||
			len(g.Periods) != 1 || g.Periods[0].PeriodNs != 16552213 {
			t.Fatalf("%s: census degraded under the thread filter: %+v", label, g)
		}
	}
	assertFull("indexed pid-filtered",
		ComputeVsyncGeneratorSearchCensus(idx, Query{View: "event_search", Pattern: "vsync", PID: 59566}))
	assertFull("indexed thread-filtered",
		ComputeVsyncGeneratorSearchCensus(idx, Query{View: "event_search", Pattern: "vsync", Thread: "com.baidu.tieba-59566"}))
	streamRes, err := StreamEventSearch(context.Background(), vsyncCensusTiebaTrace,
		Query{View: "event_search", Pattern: "vsync", Thread: "com.baidu.tieba-59566"})
	if err != nil {
		t.Fatalf("StreamEventSearch: %v", err)
	}
	assertFull("stream thread-filtered", streamRes.VsyncGeneratorCensus)
	// The display rows keep honoring the thread filter — only the census is
	// target-free (two calibers, both honest).
	for _, ev := range streamRes.Events {
		if ev.Comm == "VSyncGenerator" {
			t.Fatalf("display rows must keep the thread filter; got generator row at line %d", ev.Line)
		}
	}
}

func TestVsyncGeneratorCensus_StreamAndIndexedTwinsAgree(t *testing.T) {
	idx := vsyncCensusIndex(t, vsyncCensusTiebaTrace)
	q := Query{View: "event_search", Pattern: "vsync"}
	indexed := ComputeVsyncGeneratorSearchCensus(idx, q)
	streamRes, err := StreamEventSearch(context.Background(), vsyncCensusTiebaTrace, q)
	if err != nil {
		t.Fatalf("StreamEventSearch: %v", err)
	}
	if streamRes.VsyncGeneratorCensus == nil {
		t.Fatalf("stream result carries no generator census")
	}
	a, _ := json.Marshal(indexed)
	b, _ := json.Marshal(streamRes.VsyncGeneratorCensus)
	if string(a) != string(b) {
		t.Fatalf("stream/indexed census twins drifted:\nindexed=%s\nstream =%s", a, b)
	}
	// 修复轮 件1 (P2-1, 2026-07-14): the indexed Run() event_search branch is
	// its own wiring line (query.go default branch) and the mutation audit
	// dropped it without a single red — pin the three-way identity:
	// Run == ComputeVsyncGeneratorSearchCensus == stream.
	runRes := Run(idx, q)
	if runRes.VsyncGeneratorCensus == nil {
		t.Fatalf("Run(event_search) result carries no generator census")
	}
	c, _ := json.Marshal(runRes.VsyncGeneratorCensus)
	if string(c) != string(a) {
		t.Fatalf("Run/Compute census twins drifted:\nrun    =%s\ncompute=%s", c, a)
	}
}

// TestVsyncGeneratorCensus_RefreshRateFirstSeen (修复轮 件3, P3-3,
// 2026-07-14): a mid-window refresh-rate switch makes two period prints of
// the SAME period value carry different currRefreshRate fields; the census
// row keeps the FIRST-SEEN rate instead of silently last-wins overwriting
// (never last-wins — F1/SUPP window-derivation doctrine). Fixture rows are
// verbatim donghu print shapes with only the rate field varied.
func TestVsyncGeneratorCensus_RefreshRateFirstSeen(t *testing.T) {
	dir := t.TempDir()
	trace := `  VSyncGenerator-2179  ( 1864) [003] .... 13762.796022: print: B|1864|H:GenerateVsyncCount:1, period:11091679, currRefreshRate:90, vsyncMode:1, now:13762796008906
  VSyncGenerator-2179  ( 1864) [003] .... 13762.807100: print: B|1864|H:GenerateVsyncCount:1, period:11091679, currRefreshRate:120, vsyncMode:1, now:13762807054883
`
	path := filepath.Join(dir, "refresh_switch.systrace")
	if err := os.WriteFile(path, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	census := ComputeVsyncGeneratorWindowCensus(idx, Query{})
	if census == nil || len(census.Threads) != 1 || len(census.Threads[0].Periods) != 1 {
		t.Fatalf("census = %+v, want one thread with one period value", census)
	}
	p := census.Threads[0].Periods[0]
	if p.Rows != 2 {
		t.Fatalf("period rows = %d, want 2", p.Rows)
	}
	if p.RefreshRate != 90 {
		t.Fatalf("RefreshRate = %d, want the FIRST-SEEN 90 (last-wins would report 120)", p.RefreshRate)
	}
}

// TestVsyncGeneratorCensus_Deterministic pins the DET discipline: two
// independent computes over the same capture serialize byte-identically
// (typed constant tie keys, never map iteration order).
func TestVsyncGeneratorCensus_Deterministic(t *testing.T) {
	idx := vsyncCensusIndex(t, vsyncCensusDonghuTrace)
	a, _ := json.Marshal(ComputeVsyncGeneratorWindowCensus(idx, Query{}))
	b, _ := json.Marshal(ComputeVsyncGeneratorWindowCensus(idx, Query{}))
	if string(a) != string(b) {
		t.Fatalf("census not deterministic:\nrun1=%s\nrun2=%s", a, b)
	}
}

// TestVsyncGeneratorCensus_WindowStatsWiring pins the window_stats face: the
// census rides WindowStats whenever a generator is sighted in the window,
// population-wide (the query's pid names the CONSUMER thread — the tieba main
// thread — and the generator must still be counted).
func TestVsyncGeneratorCensus_WindowStatsWiring(t *testing.T) {
	idx := vsyncCensusIndex(t, vsyncCensusTiebaTrace)
	stats := ComputeWindowStats(idx, Query{View: "window_stats", PID: 59566, TimeStart: 34579.450627, TimeEnd: 34579.475857})
	if stats.VsyncGeneratorCensus == nil {
		t.Fatalf("window_stats carries no generator census despite in-window generator activity")
	}
	if len(stats.VsyncGeneratorCensus.Threads) != 1 || stats.VsyncGeneratorCensus.Threads[0].Thread.PID != 1682 {
		t.Fatalf("window_stats census = %+v, want VSyncGenerator-1682", stats.VsyncGeneratorCensus.Threads)
	}
	if stats.VsyncGeneratorCensus.Threads[0].Periods[0].PeriodNs != 16552213 {
		t.Fatalf("window_stats census period = %+v, want 16552213ns", stats.VsyncGeneratorCensus.Threads[0].Periods)
	}
}
