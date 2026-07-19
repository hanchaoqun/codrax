package tracequery

// evalcase_dh_frame_pin_test.go — EVALCASE-DH batch, 掉帧/长帧 family engine
// pins on the committed donghu.ftrace (mining ledger evalcase_donghu_mining.md
// §A; expectations re-collected at HEAD 1ada2c49f and hand-recomputed from
// raw sched_switch arithmetic — dual-source discipline).
//
// Cases:
//
//	DH-J1  长帧 CPU-bound 旗舰窗 (doFrame 641508, 36ms > 3× vsync 11.09ms):
//	       the target's own running seat dominates — 32.739ms on cpu12 (big,
//	       full 2750000 frequency, prio 53 ohos_rt), no frequency excuse and
//	       no runnable excuse; the supply signal carries the low-frequency
//	       word for OTHER (middle-cluster) cpus; the window-level
//	       io_pressure d_state belongs to ANOTHER thread (CompThread_0) —
//	       the 口径 pin: d_state here is a window-level cross-thread figure,
//	       NEVER the target's D wall clock (target has zero D in the whole
//	       trace).
//	DH-J2  帧间静默 124ms 窗 (641501→641508, vsync 掉 7 拍): the main thread
//	       is busy with its OWN work across clusters — per-(thread,cpu)
//	       running rows 57.828ms@cpu12(big) + 28.477ms@cpu4(middle) +
//	       11.030ms@cpu7(middle) (the 4.479ms@cpu8 fragment sits below the
//	       top-8 display cap) — the XCPU migration shape whose cross-cluster
//	       folding must stay per-cluster.
//
// Fixture red line: real capture — every number is a measured pin.

import "testing"

// DH-J1 (窗 13762.937400..13762.973600, target 17267).
func TestEvalcaseDHJ1LongFrameComputeBound(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseDonghuFixture)
	q := normalizeQuery(idx, Query{PID: 17267, TimeStart: 13762.9374, TimeEnd: 13762.9736})
	stats := ComputeWindowStats(idx, q)
	if len(stats.TopRunning) == 0 {
		t.Fatal("DH-J1: empty top_running")
	}
	top := stats.TopRunning[0]
	if top.Thread.PID != 17267 || top.Thread.Comm != ".ugc.aweme.lite" {
		t.Fatalf("DH-J1: top running seat must be the target main thread, got %+v", top.Thread)
	}
	// Hand cross-check: raw sched_switch pairing over the same window gives
	// 32.739ms, all on cpu12 (single-cluster residency — no migration excuse).
	if !near(top.DurationMs, 32.739, 0.001) || top.CPU != 12 || top.CoreClass != "big" {
		t.Fatalf("DH-J1: running seat drifted: %.3fms cpu=%d class=%s (want 32.739/12/big)", top.DurationMs, top.CPU, top.CoreClass)
	}
	if top.Priority != 53 || top.PriorityClass != "ohos_rt" {
		t.Fatalf("DH-J1: priority face drifted: %d/%s (want 53/ohos_rt)", top.Priority, top.PriorityClass)
	}
	// No frequency excuse: the target's compute_supply row rides the big
	// cluster at full frequency (observed max == 2750000).
	var cs *ComputeSupplySummary
	for i := range stats.ComputeSupply {
		if stats.ComputeSupply[i].Thread.PID == 17267 && stats.ComputeSupply[i].State == "running" {
			cs = &stats.ComputeSupply[i]
		}
	}
	if cs == nil {
		t.Fatalf("DH-J1: target running compute_supply row missing")
	}
	if cs.CPU != 12 || cs.CoreClass != "big" || cs.Frequency != 2750000 || cs.ObservedMaxFrequency != 2750000 {
		t.Fatalf("DH-J1: compute_supply seat drifted: cpu=%d class=%s freq=%d obsMax=%d", cs.CPU, cs.CoreClass, cs.Frequency, cs.ObservedMaxFrequency)
	}
	// Supply signal: pressure exists with the low-frequency word — but the
	// low-frequency set is exactly the MIDDLE cluster [4..11] (never the
	// target's cpu12 seat): the long frame has no supply excuse.
	s := stats.SupplyPressureSummary
	if s == nil || s.Signal != "cpu_pressure_with_low_frequency" {
		t.Fatalf("DH-J1: supply signal drifted: %+v", s)
	}
	if len(s.LowFrequencyCPUs) != 8 || s.LowFrequencyCPUs[0] != 4 || s.LowFrequencyCPUs[7] != 11 {
		t.Fatalf("DH-J1: low_frequency_cpus must be the middle cluster [4..11], got %v", s.LowFrequencyCPUs)
	}
	// 口径 pin (mining 对账注1): io_pressure_summary.d_state is the
	// WINDOW-LEVEL cross-thread aggregate — here it belongs to
	// CompThread_0-2955 (3.280ms), while the target contributes ZERO D
	// (pid 17267 has no prev_state=D in the whole trace). Any consumer
	// reading this field as the target's D wall clock is wrong by contract.
	io := stats.IOPressureSummary
	if io == nil || !near(io.DStateMs, 3.280, 0.001) {
		t.Fatalf("DH-J1: window-level d_state drifted: %+v", io)
	}
	for _, d := range stats.DStateTop {
		if d.Thread.PID == 17267 {
			t.Fatalf("DH-J1 口径: target must have NO d_state seat (zero-D lane), got %+v", d)
		}
	}
	foundOther := false
	for _, d := range stats.DStateTop {
		if d.Thread.Comm == "CompThread_0" && near(d.DurationMs, 3.280, 0.001) {
			foundOther = true
		}
	}
	if !foundOther {
		t.Fatalf("DH-J1 口径: the window d_state owner (CompThread_0 3.280ms) is missing: %+v", stats.DStateTop)
	}
}

// DH-J2 (窗 13762.813789..13762.937367, target 17267) — cross-cluster
// migration running account, per-(thread,cpu) census rows.
func TestEvalcaseDHJ2InterFrameGapMigration(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseDonghuFixture)
	q := normalizeQuery(idx, Query{PID: 17267, TimeStart: 13762.813789, TimeEnd: 13762.937367})
	stats := ComputeWindowStats(idx, q)
	got := map[int]ThreadDuration{}
	for _, r := range stats.TopRunning {
		if r.Thread.PID == 17267 {
			got[r.CPU] = r
		}
	}
	// Hand cross-check (raw sched_switch arithmetic): {4:28.477, 7:11.030,
	// 8:4.479, 12:57.828} = 101.814ms total; the top-8 display face carries
	// the three largest fragments.
	want := map[int]struct {
		ms    float64
		class string
	}{
		12: {57.828, "big"},
		4:  {28.477, "middle"},
		7:  {11.030, "middle"},
	}
	for cpu, w := range want {
		r, ok := got[cpu]
		if !ok {
			t.Fatalf("DH-J2: missing 17267 running row on cpu%d (rows=%v)", cpu, got)
		}
		if !near(r.DurationMs, w.ms, 0.001) || r.CoreClass != w.class {
			t.Fatalf("DH-J2: cpu%d row drifted: %.3fms class=%s (want %.3f/%s)", cpu, r.DurationMs, r.CoreClass, w.ms, w.class)
		}
	}
	// The migration shape spans TWO capability classes — cross-cluster
	// folding must price each fragment on its own cluster (cap 2.3 vs 2.53),
	// so the per-(thread,cpu) rows must never collapse into one class.
	classes := map[string]bool{}
	for _, r := range got {
		classes[r.CoreClass] = true
	}
	if !classes["middle"] || !classes["big"] {
		t.Fatalf("DH-J2: migration shape must span middle AND big classes, got %v", classes)
	}
	// fscache witness lane is present in the gap window (the blocked_reason
	// census carries the caller for the target).
	foundFscache := false
	for _, c := range stats.BlockedReasonCensus {
		if c.Thread.PID != 17267 {
			continue
		}
		for _, caller := range c.Callers {
			if caller.Caller == "fscache_page_get_an" {
				foundFscache = true
			}
		}
	}
	if !foundFscache {
		t.Fatalf("DH-J2: fscache_page_get_an blocked_reason census witness missing for the target: %+v", stats.BlockedReasonCensus)
	}
}
