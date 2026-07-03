package tracequery

import (
	"math"
	"strings"
	"testing"
)

// cmpSyntheticSupplyTrace is the CMP-8/CMP-9/CMP-10 numeric fixture: a
// 100ms window (1.000..1.100) over two CPUs with a fully known frequency and
// switch sequence, so delivered compute, low-frequency loss, idle mismatch,
// and the normalized densities each have ONE exact expected value.
//
//	cpu0: fmax=2000000kHz (sample 1000000@0.99 governs the head, 2000000@1.06
//	      after); hi-101 (ohos_rt prio=90) runs 1.00..1.06 at 1000000kHz
//	      → running 60ms, delivered 30ms, low-freq loss 30ms.
//	cpu1: NO cpu_frequency samples (weight 1.0, 无频点数据 caveat);
//	      worker-201 runs 1.00..1.05 (50ms), worker2-202 runs 1.08..1.09
//	      (10ms) → delivered 60ms, loss 0. worker2-202 is woken at 1.06 while
//	      cpu1 idles → runnable backlog 20ms = idle mismatch 20ms and the
//	      only runnable wait (pressure density 20/100 = 0.2).
//	Balance: nominal 200 cpu·ms, delivered 90, ratio 0.45, loss 30,
//	idle mismatch 20 (wall), core-limited remainder 60.
const cmpSyntheticSupplyTrace = `          <idle>-0 (-----) [000] .... 0.990000: cpu_frequency: state=1000000 cpu_id=0
          <idle>-0 (-----) [000] .... 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hi next_pid=101 next_prio=90
          <idle>-0 (-----) [001] .... 1.000000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=201 next_prio=20
       worker-201 (  200) [001] .... 1.050000: sched_switch: prev_comm=worker prev_pid=201 prev_prio=20 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120
           hi-101 (  101) [000] .... 1.060000: cpu_frequency: state=2000000 cpu_id=0
           hi-101 (  101) [000] .... 1.060000: sched_wakeup: comm=worker2 pid=202 prio=20 target_cpu=001
           hi-101 (  101) [000] .... 1.060000: sched_switch: prev_comm=hi prev_pid=101 prev_prio=90 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
          <idle>-0 (-----) [001] .... 1.080000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker2 next_pid=202 next_prio=20
      worker2-202 (  200) [001] .... 1.090000: sched_switch: prev_comm=worker2 prev_pid=202 prev_prio=20 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120
`

func cmpSupplyWindowQuery() Query {
	return Query{TimeStart: 1.0, TimeEnd: 1.1, TraceFlavorHint: TraceFlavorHarmonyHitrace}
}

func approxEq(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("%s = %.6f, want %.6f", name, got, want)
	}
}

// TestComputeSupplyBalanceNumericPin pins the CMP-10 (§7.4) ledger values on
// the synthetic frequency/switch sequence: delivered compute Σ(running×f/fmax),
// low-frequency loss Σ(running×(1−f/fmax)), idle-vs-runnable mismatch, and
// the core-limited remainder.
func TestComputeSupplyBalanceNumericPin(t *testing.T) {
	idx := buildTraceIndex(t, "cmp_supply.systrace", cmpSyntheticSupplyTrace)
	stats := ComputeWindowStats(idx, cmpSupplyWindowQuery())
	bal := stats.ComputeSupplyBalance
	if bal == nil {
		t.Fatal("ComputeSupplyBalance must be published for a bounded window")
	}
	approxEq(t, "WindowMs", bal.WindowMs, 100)
	if bal.CPUCount != 2 {
		t.Fatalf("CPUCount = %d, want 2", bal.CPUCount)
	}
	approxEq(t, "NominalCapacityMs", bal.NominalCapacityMs, 200)
	approxEq(t, "DeliveredComputeMs", bal.DeliveredComputeMs, 90)
	approxEq(t, "LowFrequencyLossMs", bal.LowFrequencyLossMs, 30)
	approxEq(t, "IdleMismatchMs", bal.IdleMismatchMs, 20)
	approxEq(t, "CoreLimitedMs", bal.CoreLimitedMs, 60)
	approxEq(t, "SupplyRatio", bal.SupplyRatio, 0.45)
	if len(bal.PerCPU) != 2 {
		t.Fatalf("PerCPU rows = %d, want 2", len(bal.PerCPU))
	}
	cpu0, cpu1 := bal.PerCPU[0], bal.PerCPU[1]
	if cpu0.CPU != 0 || cpu1.CPU != 1 {
		t.Fatalf("PerCPU order = %d,%d, want 0,1", cpu0.CPU, cpu1.CPU)
	}
	approxEq(t, "cpu0.RunningMs", cpu0.RunningMs, 60)
	approxEq(t, "cpu0.DeliveredComputeMs", cpu0.DeliveredComputeMs, 30)
	approxEq(t, "cpu0.LowFrequencyLossMs", cpu0.LowFrequencyLossMs, 30)
	if cpu0.MaxFrequencyKHz != 2000000 || !cpu0.FrequencyKnown {
		t.Fatalf("cpu0 fmax=%d known=%t, want 2000000 true", cpu0.MaxFrequencyKHz, cpu0.FrequencyKnown)
	}
	approxEq(t, "cpu1.RunningMs", cpu1.RunningMs, 60)
	approxEq(t, "cpu1.DeliveredComputeMs", cpu1.DeliveredComputeMs, 60)
	approxEq(t, "cpu1.LowFrequencyLossMs", cpu1.LowFrequencyLossMs, 0)
	if cpu1.FrequencyKnown {
		t.Fatal("cpu1 has no frequency samples and must report FrequencyKnown=false")
	}
	foundNoFreqCaveat := false
	for _, caveat := range bal.Caveats {
		if strings.Contains(caveat, "cpu=1") && strings.Contains(caveat, "无频点数据") {
			foundNoFreqCaveat = true
		}
	}
	if !foundNoFreqCaveat {
		t.Fatalf("missing 无频点数据 caveat for cpu=1: %v", bal.Caveats)
	}
	for _, want := range []string{"delivered=90.000cpu·ms", "supply_ratio=0.450", "low_freq_loss=30.000cpu·ms", "idle_mismatch=20.000ms(wall)", "core_limited≈60.000cpu·ms"} {
		if !strings.Contains(bal.Summary, want) {
			t.Fatalf("balance summary missing %q: %s", want, bal.Summary)
		}
	}
}

// TestSupplyPressureDensityPin pins CMP-9 (§7.3): pressure_density is the
// exact value/window division on both the summary aggregate and the per-CPU
// runnable-wait rows, and an unbounded window publishes NO density and NO
// supply balance (never an estimate).
func TestSupplyPressureDensityPin(t *testing.T) {
	idx := buildTraceIndex(t, "cmp_density.systrace", cmpSyntheticSupplyTrace)
	stats := ComputeWindowStats(idx, cmpSupplyWindowQuery())
	supply := stats.SupplyPressureSummary
	if supply == nil {
		t.Fatal("expected supply pressure summary")
	}
	approxEq(t, "WindowMs", supply.WindowMs, 100)
	approxEq(t, "PressureDensity", supply.PressureDensity, supply.CPUPressureMs/100)
	// CPUPressureMs = runnable backlog (20ms on cpu1) + high-priority running
	// (60ms ohos_rt on cpu0) = 80 cpu·ms → density 80/100 = 0.8 exactly.
	approxEq(t, "CPUPressureMs", supply.CPUPressureMs, 80)
	approxEq(t, "PressureDensity(exact)", supply.PressureDensity, 0.8)
	if !strings.Contains(supply.Summary, "window_ms=100.000") || !strings.Contains(supply.Summary, "pressure_density=0.80") {
		t.Fatalf("supply summary missing CMP-9 typed tokens: %s", supply.Summary)
	}
	foundCPU1 := false
	for _, pressure := range stats.CPUPressure {
		if pressure.CPU != 1 {
			continue
		}
		foundCPU1 = true
		approxEq(t, "cpu1.RunnableWaitMs", pressure.RunnableWaitMs, 20)
		approxEq(t, "cpu1.RunnableWaitDensity", pressure.RunnableWaitDensity, 0.2)
	}
	if !foundCPU1 {
		t.Fatal("expected cpu=1 pressure row")
	}

	// Unbounded window: no density, no estimate, no balance.
	unbounded := ComputeWindowStats(idx, Query{TraceFlavorHint: TraceFlavorHarmonyHitrace})
	if unbounded.ComputeSupplyBalance != nil {
		t.Fatal("unbounded window must not publish a compute-supply balance (nominal capacity would be an estimate)")
	}
	if s := unbounded.SupplyPressureSummary; s != nil && (s.WindowMs != 0 || s.PressureDensity != 0) {
		t.Fatalf("unbounded window must not publish window/density: window=%.3f density=%.3f", s.WindowMs, s.PressureDensity)
	}
	for _, pressure := range unbounded.CPUPressure {
		if pressure.RunnableWaitDensity != 0 {
			t.Fatalf("unbounded window must not publish per-CPU density: %+v", pressure)
		}
	}
	if occ := unbounded.CPUOccupancy; occ != nil {
		if occ.WindowMs != 0 {
			t.Fatalf("unbounded occupancy WindowMs = %.3f, want 0", occ.WindowMs)
		}
		foundCaveat := false
		for _, caveat := range occ.Caveats {
			if strings.Contains(caveat, "window unbounded") {
				foundCaveat = true
			}
		}
		if !foundCaveat {
			t.Fatalf("unbounded occupancy must carry the no-window caveat: %v", occ.Caveats)
		}
	}
}

// TestCPUOccupancyDecompositionPin pins the CMP-8 (§7.1) occupancy surfaces:
// thread/process running rollups over the FULL running buckets, per-CPU top
// occupiers, and the platform priority-band split.
func TestCPUOccupancyDecompositionPin(t *testing.T) {
	idx := buildTraceIndex(t, "cmp_occupancy.systrace", cmpSyntheticSupplyTrace)
	stats := ComputeWindowStats(idx, cmpSupplyWindowQuery())
	occ := stats.CPUOccupancy
	if occ == nil {
		t.Fatal("expected cpu occupancy stats")
	}
	approxEq(t, "WindowMs", occ.WindowMs, 100)

	// (a) top threads by cpu·ms: hi 60, worker 50, worker2 10.
	if len(occ.TopThreads) != 3 {
		t.Fatalf("TopThreads = %d rows, want 3: %+v", len(occ.TopThreads), occ.TopThreads)
	}
	if occ.TopThreads[0].Thread.Comm != "hi" || occ.TopThreads[1].Thread.Comm != "worker" || occ.TopThreads[2].Thread.Comm != "worker2" {
		t.Fatalf("TopThreads order wrong: %+v", occ.TopThreads)
	}
	approxEq(t, "hi.RunningMs", occ.TopThreads[0].RunningMs, 60)
	approxEq(t, "worker.RunningMs", occ.TopThreads[1].RunningMs, 50)
	approxEq(t, "worker2.RunningMs", occ.TopThreads[2].RunningMs, 10)
	if occ.TopThreads[0].PriorityClass != "ohos_cfs" && occ.TopThreads[0].PriorityClass != "ohos_rt" {
		t.Fatalf("hi priority class missing: %+v", occ.TopThreads[0])
	}

	// (b) per-process rollup: worker+worker2 share tgid 200 → 60ms with 2
	// threads; hi-101 stands alone with 60ms.
	if len(occ.TopProcesses) != 2 {
		t.Fatalf("TopProcesses = %d rows, want 2: %+v", len(occ.TopProcesses), occ.TopProcesses)
	}
	byPID := map[int]ProcessCPULoadSummary{}
	for _, proc := range occ.TopProcesses {
		byPID[proc.Process.PID] = proc
	}
	shared, ok := byPID[200]
	if !ok {
		t.Fatalf("expected tgid=200 process rollup: %+v", occ.TopProcesses)
	}
	approxEq(t, "process200.RunningMs", shared.RunningMs, 60)
	if shared.ThreadCount != 2 {
		t.Fatalf("process200.ThreadCount = %d, want 2", shared.ThreadCount)
	}
	solo, ok := byPID[101]
	if !ok {
		t.Fatalf("expected pid=101 process rollup: %+v", occ.TopProcesses)
	}
	approxEq(t, "process101.RunningMs", solo.RunningMs, 60)

	// (c) per-CPU top occupiers (≤2 per CPU).
	if len(occ.PerCPUTop) != 2 {
		t.Fatalf("PerCPUTop = %d rows, want 2: %+v", len(occ.PerCPUTop), occ.PerCPUTop)
	}
	if occ.PerCPUTop[0].CPU != 0 || len(occ.PerCPUTop[0].Top) != 1 || occ.PerCPUTop[0].Top[0].Thread.Comm != "hi" {
		t.Fatalf("cpu0 top occupiers wrong: %+v", occ.PerCPUTop[0])
	}
	cpu1 := occ.PerCPUTop[1]
	if cpu1.CPU != 1 || len(cpu1.Top) != 2 || cpu1.Top[0].Thread.Comm != "worker" || cpu1.Top[1].Thread.Comm != "worker2" {
		t.Fatalf("cpu1 top occupiers wrong: %+v", cpu1)
	}

	// (d) priority bands via the platform rule: prio=90 → ohos_rt (high),
	// prio=20 → ohos_cfs (not high).
	bands := map[string]CPUOccupancyPriorityBand{}
	for _, band := range occ.PriorityBands {
		bands[band.Band] = band
	}
	rt, ok := bands["ohos_rt"]
	if !ok || !rt.HighPriority || rt.ThreadCount != 1 {
		t.Fatalf("ohos_rt band wrong: %+v", occ.PriorityBands)
	}
	approxEq(t, "rt.RunningMs", rt.RunningMs, 60)
	cfs, ok := bands["ohos_cfs"]
	if !ok || cfs.HighPriority || cfs.ThreadCount != 2 {
		t.Fatalf("ohos_cfs band wrong: %+v", occ.PriorityBands)
	}
	approxEq(t, "cfs.RunningMs", cfs.RunningMs, 60)

	// cpu·ms unit contract caveat is always present.
	foundUnitCaveat := false
	for _, caveat := range occ.Caveats {
		if strings.Contains(caveat, "cpu·ms") {
			foundUnitCaveat = true
		}
	}
	if !foundUnitCaveat {
		t.Fatalf("occupancy must pin the cpu·ms unit contract: %v", occ.Caveats)
	}
}

// TestComputeSupplyFmaxWindowGovernedPin pins adversarial-review F1
// (2026-07-04): fmax is the max over the samples GOVERNING this window (the
// head-governing sample + in-window samples = the residency timeline), never
// raw pre-window history. Reviewer counterexample: a 3.0GHz burst sample long
// before the window plus a 1.8GHz head governor covering the whole window →
// fmax MUST be 1.8GHz and the low-frequency loss MUST be 0 (the buggy
// timeline-wide max fabricated fmax=3.0GHz → 24 cpu·ms of phantom loss).
func TestComputeSupplyFmaxWindowGovernedPin(t *testing.T) {
	const trace = `          <idle>-0 (-----) [000] .... 0.500000: cpu_frequency: state=3000000 cpu_id=0
          <idle>-0 (-----) [000] .... 0.990000: cpu_frequency: state=1800000 cpu_id=0
          <idle>-0 (-----) [000] .... 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hi next_pid=101 next_prio=90
           hi-101 (  101) [000] .... 1.060000: sched_switch: prev_comm=hi prev_pid=101 prev_prio=90 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
`
	idx := buildTraceIndex(t, "cmp_fmax_governed.systrace", trace)
	stats := ComputeWindowStats(idx, cmpSupplyWindowQuery())
	bal := stats.ComputeSupplyBalance
	if bal == nil {
		t.Fatal("expected compute supply balance")
	}
	if len(bal.PerCPU) != 1 || bal.PerCPU[0].CPU != 0 {
		t.Fatalf("PerCPU rows = %+v, want exactly cpu0", bal.PerCPU)
	}
	cpu0 := bal.PerCPU[0]
	if cpu0.MaxFrequencyKHz != 1800000 || !cpu0.FrequencyKnown {
		t.Fatalf("cpu0 fmax=%d known=%t, want the 1800000 head governor (pre-window 3000000 history must not participate)", cpu0.MaxFrequencyKHz, cpu0.FrequencyKnown)
	}
	approxEq(t, "cpu0.RunningMs", cpu0.RunningMs, 60)
	approxEq(t, "cpu0.DeliveredComputeMs", cpu0.DeliveredComputeMs, 60)
	approxEq(t, "cpu0.LowFrequencyLossMs", cpu0.LowFrequencyLossMs, 0)
	approxEq(t, "LowFrequencyLossMs", bal.LowFrequencyLossMs, 0)
	approxEq(t, "DeliveredComputeMs", bal.DeliveredComputeMs, 60)
}

// TestComputeSupplyGhostCPUExcludedPin pins adversarial-review F2
// (2026-07-04): CPUs that surface only through cpu_frequency samples (no
// in-window sched_switch activity) MUST NOT inflate CPUCount / nominal
// capacity, and the exclusion is disclosed by count. Reviewer counterexample:
// 4 real cores + 4 freq-only ghost cores → nominal counts exactly 4 cores.
func TestComputeSupplyGhostCPUExcludedPin(t *testing.T) {
	const trace = `          <idle>-0 (-----) [000] .... 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=w0 next_pid=401 next_prio=20
          <idle>-0 (-----) [001] .... 1.000000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=w1 next_pid=402 next_prio=20
          <idle>-0 (-----) [002] .... 1.000000: sched_switch: prev_comm=swapper/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=w2 next_pid=403 next_prio=20
          <idle>-0 (-----) [003] .... 1.000000: sched_switch: prev_comm=swapper/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=w3 next_pid=404 next_prio=20
              w0-401 (  400) [004] .... 1.010000: cpu_frequency: state=1000000 cpu_id=4
              w0-401 (  400) [005] .... 1.010000: cpu_frequency: state=1000000 cpu_id=5
              w0-401 (  400) [006] .... 1.010000: cpu_frequency: state=1000000 cpu_id=6
              w0-401 (  400) [007] .... 1.010000: cpu_frequency: state=1000000 cpu_id=7
              w0-401 (  400) [000] .... 1.050000: sched_switch: prev_comm=w0 prev_pid=401 prev_prio=20 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
              w1-402 (  400) [001] .... 1.050000: sched_switch: prev_comm=w1 prev_pid=402 prev_prio=20 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120
              w2-403 (  400) [002] .... 1.050000: sched_switch: prev_comm=w2 prev_pid=403 prev_prio=20 prev_state=S ==> next_comm=swapper/2 next_pid=0 next_prio=120
              w3-404 (  400) [003] .... 1.050000: sched_switch: prev_comm=w3 prev_pid=404 prev_prio=20 prev_state=S ==> next_comm=swapper/3 next_pid=0 next_prio=120
`
	idx := buildTraceIndex(t, "cmp_ghost_cpus.systrace", trace)
	stats := ComputeWindowStats(idx, cmpSupplyWindowQuery())
	bal := stats.ComputeSupplyBalance
	if bal == nil {
		t.Fatal("expected compute supply balance")
	}
	if bal.CPUCount != 4 {
		t.Fatalf("CPUCount = %d, want 4 (ghost freq-only CPUs must not count)", bal.CPUCount)
	}
	approxEq(t, "NominalCapacityMs", bal.NominalCapacityMs, 400)
	if len(bal.PerCPU) != 4 {
		t.Fatalf("PerCPU rows = %d, want 4 sched-observed CPUs only: %+v", len(bal.PerCPU), bal.PerCPU)
	}
	for i, per := range bal.PerCPU {
		if per.CPU != i {
			t.Fatalf("PerCPU[%d].CPU = %d, want %d (ghost CPUs 4-7 excluded)", i, per.CPU, i)
		}
	}
	foundGhostCaveat := false
	for _, caveat := range bal.Caveats {
		if strings.Contains(caveat, "另 4 核仅有频点样本无调度事件,未计入") {
			foundGhostCaveat = true
		}
	}
	if !foundGhostCaveat {
		t.Fatalf("missing ghost-CPU disclosure caveat: %v", bal.Caveats)
	}
}

// TestIdleMismatchHeadRunnableSeedPin pins adversarial-review F3
// (2026-07-04): a thread preempted BEFORE the window (prev_state=R) that
// never gets scheduled inside it seeds the idle-mismatch runnable set from
// the off-CPU pass's window-head-open runnable segment. Without seeding the
// event scan sees an empty runnable queue and reports 0 for a window where a
// ready thread starved 80ms against an idle CPU.
func TestIdleMismatchHeadRunnableSeedPin(t *testing.T) {
	const trace = `          <idle>-0 (-----) [000] .... 0.950000: sched_switch: prev_comm=waiter prev_pid=301 prev_prio=20 prev_state=R ==> next_comm=swapper/0 next_pid=0 next_prio=120
          <idle>-0 (-----) [000] .... 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=work next_pid=401 next_prio=20
            work-401 (  400) [000] .... 1.020000: sched_switch: prev_comm=work prev_pid=401 prev_prio=20 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
`
	idx := buildTraceIndex(t, "cmp_head_runnable.systrace", trace)
	stats := ComputeWindowStats(idx, cmpSupplyWindowQuery())
	bal := stats.ComputeSupplyBalance
	if bal == nil {
		t.Fatal("expected compute supply balance")
	}
	// waiter-301 runnable across the whole window; cpu0 idle 1.02..1.10.
	approxEq(t, "IdleMismatchMs", bal.IdleMismatchMs, 80)
	foundLowerBoundCaveat := false
	for _, caveat := range bal.Caveats {
		if strings.Contains(caveat, "该项为下界") {
			foundLowerBoundCaveat = true
		}
	}
	if !foundLowerBoundCaveat {
		t.Fatalf("missing idle-mismatch lower-bound disclosure caveat: %v", bal.Caveats)
	}
}

// TestSchedNextIsIdlePredicateSharedPin is the adversarial-review F4
// (2026-07-04) structural pin: the busy/idle split in ComputeWindowStats and
// the CMP-10 idle-mismatch pass MUST judge "switched to idle" through the
// same schedNextIsIdle predicate. Divergence probe: an idle-injection task
// (comm contains "idle", pid>0). If the busy loop re-inlines a pid-only
// check, BusyMs becomes 100; if the mismatch pass diverges, the mismatch
// drops to 0 — either fork fails this test.
func TestSchedNextIsIdlePredicateSharedPin(t *testing.T) {
	const trace = `          <idle>-0 (-----) [000] .... 1.000000: sched_wakeup: comm=waiter pid=301 prio=20 target_cpu=000
          <idle>-0 (-----) [000] .... 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=idle_inject/0 next_pid=7 next_prio=120
`
	idx := buildTraceIndex(t, "cmp_idle_predicate.systrace", trace)
	stats := ComputeWindowStats(idx, cmpSupplyWindowQuery())
	foundCPU0 := false
	for _, cpu := range stats.CPU {
		if cpu.CPU != 0 {
			continue
		}
		foundCPU0 = true
		approxEq(t, "cpu0.BusyMs", cpu.BusyMs, 0)
		approxEq(t, "cpu0.IdleMs", cpu.IdleMs, 100)
	}
	if !foundCPU0 {
		t.Fatalf("expected cpu0 stats: %+v", stats.CPU)
	}
	bal := stats.ComputeSupplyBalance
	if bal == nil {
		t.Fatal("expected compute supply balance")
	}
	approxEq(t, "IdleMismatchMs", bal.IdleMismatchMs, 100)
}
