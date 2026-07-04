package tracequery

// CMP-8 / CMP-10 (customer_dead_session_audit_20260703 §7.1/§7.4): the
// occupancy-side decomposition ("who ate the CPU") and the true supply-side
// ledger (frequency-weighted delivered compute + supply-gap decomposition)
// for one selected window. Both reuse the running segmentation that
// ComputeWindowStats already produced — no second timing pass — and both
// refuse to publish window-normalized ratios when the query window is
// unbounded (CMP-3/CMP-9: no window, no estimate).

import (
	"fmt"
	"sort"
	"strings"
)

// queryWindowWallMs is the precise wall-clock window length in ms: both
// bounds must be explicit and ordered, otherwise 0 (unbounded — callers must
// not estimate a denominator).
func queryWindowWallMs(q Query) float64 {
	if q.TimeStart > 0 && q.TimeEnd > q.TimeStart {
		return (q.TimeEnd - q.TimeStart) * 1000
	}
	return 0
}

// schedNextIsIdle is THE idle predicate for a sched_switch next-task — the
// same test ComputeWindowStats' busy/idle split uses, factored so the CMP-10
// idle-mismatch pass cannot drift from the busy accounting.
func schedNextIsIdle(ev Event) bool {
	return ev.NextPID == 0 || strings.Contains(strings.ToLower(ev.NextComm), "idle")
}

// applyCPUPressureDensity fills the CMP-9 per-CPU runnable-wait density
// (RunnableWaitMs / wall window). windowMs<=0 leaves every density at 0.
func applyCPUPressureDensity(in []CPUPressureStats, windowMs float64) []CPUPressureStats {
	if windowMs <= 0 {
		return in
	}
	for i := range in {
		if in[i].RunnableWaitMs > 0 {
			in[i].RunnableWaitDensity = in[i].RunnableWaitMs / windowMs
		}
	}
	return in
}

// computeCPUOccupancyStats builds the CMP-8 occupancy decomposition from the
// window's existing running buckets: running is the full per-(pid/comm/cpu)
// running map (pre-truncation), pressure the per-CPU accumulators, catalog
// the pid→process catalog already built for the process rollup.
func computeCPUOccupancyStats(q Query, windowMs float64, running map[string]ThreadDuration, pressure map[int]*cpuPressureAcc, coreByCPU map[int]string, catalog map[int]ThreadRef, cpus []CPUStats, max int) *CPUOccupancyStats {
	if len(running) == 0 {
		return nil
	}
	if max <= 0 {
		max = 8
	}
	occ := &CPUOccupancyStats{WindowMs: windowMs}

	// (a) top threads: aggregate the running buckets across CPUs per thread.
	type threadAcc struct {
		item       CPUOccupancyThread
		cpuSet     map[int]bool
		coreSet    map[string]bool
		dominantMs float64
	}
	threads := map[string]*threadAcc{}
	for _, td := range running {
		if td.DurationMs <= 0 || (td.Thread.PID <= 0 && td.Thread.Comm == "") {
			continue
		}
		key := fmt.Sprintf("%d/%s", td.Thread.PID, td.Thread.Comm)
		acc := threads[key]
		if acc == nil {
			acc = &threadAcc{item: CPUOccupancyThread{Thread: td.Thread}, cpuSet: map[int]bool{}, coreSet: map[string]bool{}}
			threads[key] = acc
		}
		acc.item.RunningMs += td.DurationMs
		if !acc.cpuSet[td.CPU] {
			acc.cpuSet[td.CPU] = true
			acc.item.CPUs = append(acc.item.CPUs, td.CPU)
		}
		if class := coreByCPU[td.CPU]; class != "" && !acc.coreSet[class] {
			acc.coreSet[class] = true
			acc.item.CoreClasses = append(acc.item.CoreClasses, class)
		}
		if td.DurationMs > acc.dominantMs {
			acc.dominantMs = td.DurationMs
			acc.item.Priority = td.Priority
			acc.item.PriorityClass = td.PriorityClass
		}
		if acc.item.LineStart == 0 || (td.LineStart > 0 && td.LineStart < acc.item.LineStart) {
			acc.item.LineStart = td.LineStart
		}
		if td.LineEnd > acc.item.LineEnd {
			acc.item.LineEnd = td.LineEnd
		}
	}
	for _, acc := range threads {
		sort.Ints(acc.item.CPUs)
		sort.SliceStable(acc.item.CoreClasses, func(i, j int) bool {
			return coreClassRank(acc.item.CoreClasses[i]) < coreClassRank(acc.item.CoreClasses[j])
		})
		occ.TopThreads = append(occ.TopThreads, acc.item)
	}
	sort.SliceStable(occ.TopThreads, func(i, j int) bool {
		if occ.TopThreads[i].RunningMs != occ.TopThreads[j].RunningMs {
			return occ.TopThreads[i].RunningMs > occ.TopThreads[j].RunningMs
		}
		return occ.TopThreads[i].LineStart < occ.TopThreads[j].LineStart
	})
	if len(occ.TopThreads) > max {
		occ.TopThreads = occ.TopThreads[:max]
	}

	// (b) per-process (tgid) running rollup over the SAME buckets — the full
	// map, not the display-truncated Top-8, so many-small-thread processes
	// keep their true share.
	type procAcc struct {
		item      ProcessCPULoadSummary
		threadSet map[int]bool
		cpuSet    map[int]bool
		coreSet   map[string]bool
	}
	procs := map[string]*procAcc{}
	for _, td := range running {
		if td.DurationMs <= 0 || (td.Thread.PID <= 0 && td.Thread.Comm == "") {
			continue
		}
		proc := processRefForThread(td.Thread, catalog)
		key := processKey(proc)
		acc := procs[key]
		if acc == nil {
			acc = &procAcc{item: ProcessCPULoadSummary{Process: proc}, threadSet: map[int]bool{}, cpuSet: map[int]bool{}, coreSet: map[string]bool{}}
			procs[key] = acc
		}
		if proc.Comm != "" && acc.item.Process.Comm == "" {
			acc.item.Process.Comm = proc.Comm
		}
		if td.Thread.PID > 0 && !acc.threadSet[td.Thread.PID] {
			acc.threadSet[td.Thread.PID] = true
			acc.item.ThreadCount++
		}
		acc.item.RunningMs += td.DurationMs
		if td.DurationMs > acc.item.TopThreadMs {
			acc.item.TopThread = td.Thread
			acc.item.TopThreadMs = td.DurationMs
		}
		if td.CPU >= 0 && !acc.cpuSet[td.CPU] {
			acc.cpuSet[td.CPU] = true
			acc.item.CPUs = append(acc.item.CPUs, td.CPU)
		}
		if class := coreByCPU[td.CPU]; class != "" && !acc.coreSet[class] {
			acc.coreSet[class] = true
			acc.item.CoreClasses = append(acc.item.CoreClasses, class)
		}
		if acc.item.LineStart == 0 || (td.LineStart > 0 && td.LineStart < acc.item.LineStart) {
			acc.item.LineStart = td.LineStart
		}
		if td.LineEnd > acc.item.LineEnd {
			acc.item.LineEnd = td.LineEnd
		}
	}
	for _, acc := range procs {
		sort.Ints(acc.item.CPUs)
		sort.SliceStable(acc.item.CoreClasses, func(i, j int) bool {
			return coreClassRank(acc.item.CoreClasses[i]) < coreClassRank(acc.item.CoreClasses[j])
		})
		acc.item.Summary = fmt.Sprintf("%s occupancy running=%.3fms(cpu·ms) threads=%d top_thread=%s %.3fms cpus=%s",
			threadLabel(acc.item.Process), acc.item.RunningMs, acc.item.ThreadCount,
			threadLabel(acc.item.TopThread), acc.item.TopThreadMs, intListString(acc.item.CPUs))
		occ.TopProcesses = append(occ.TopProcesses, acc.item)
	}
	sort.SliceStable(occ.TopProcesses, func(i, j int) bool {
		if occ.TopProcesses[i].RunningMs != occ.TopProcesses[j].RunningMs {
			return occ.TopProcesses[i].RunningMs > occ.TopProcesses[j].RunningMs
		}
		return occ.TopProcesses[i].LineStart < occ.TopProcesses[j].LineStart
	})
	if len(occ.TopProcesses) > max {
		occ.TopProcesses = occ.TopProcesses[:max]
	}

	// (c) per-CPU top occupiers (top 2), straight from the per-CPU running
	// buckets the pressure accounting already accumulated.
	cpuStats := map[int]CPUStats{}
	for _, cpu := range cpus {
		cpuStats[cpu.CPU] = cpu
	}
	for cpu, acc := range pressure {
		if acc == nil || len(acc.running) == 0 {
			continue
		}
		entry := CPUOccupancyPerCPU{
			CPU:       cpu,
			CoreClass: coreByCPU[cpu],
			BusyMs:    cpuStats[cpu].BusyMs,
			IdleMs:    cpuStats[cpu].IdleMs,
			Top:       topThreadDurations(acc.running, 2),
		}
		occ.PerCPUTop = append(occ.PerCPUTop, entry)
	}
	sort.SliceStable(occ.PerCPUTop, func(i, j int) bool { return occ.PerCPUTop[i].CPU < occ.PerCPUTop[j].CPU })

	// (d) priority-band running split: typed platform classes only.
	type bandAcc struct {
		item      CPUOccupancyPriorityBand
		threadSet map[string]bool
	}
	bands := map[string]*bandAcc{}
	for _, td := range running {
		if td.DurationMs <= 0 {
			continue
		}
		band := td.PriorityClass
		if band == "" {
			band = "unclassified"
		}
		acc := bands[band]
		if acc == nil {
			acc = &bandAcc{
				item: CPUOccupancyPriorityBand{
					Band:         band,
					HighPriority: isHighPriorityForPressure(q.TraceFlavor, td.Priority, td.PriorityClass),
				},
				threadSet: map[string]bool{},
			}
			bands[band] = acc
		}
		acc.item.RunningMs += td.DurationMs
		tk := fmt.Sprintf("%d/%s", td.Thread.PID, td.Thread.Comm)
		if !acc.threadSet[tk] {
			acc.threadSet[tk] = true
			acc.item.ThreadCount++
		}
	}
	for _, acc := range bands {
		occ.PriorityBands = append(occ.PriorityBands, acc.item)
	}
	sort.SliceStable(occ.PriorityBands, func(i, j int) bool {
		if occ.PriorityBands[i].RunningMs != occ.PriorityBands[j].RunningMs {
			return occ.PriorityBands[i].RunningMs > occ.PriorityBands[j].RunningMs
		}
		return occ.PriorityBands[i].Band < occ.PriorityBands[j].Band
	})

	occ.Caveats = append(occ.Caveats, "occupancy durations are cpu-time (cpu·ms) clipped to the selected window; cross-CPU sums may exceed the wall-clock window and must not be read as elapsed time")
	if windowMs <= 0 {
		occ.Caveats = append(occ.Caveats, "query window unbounded: window_ms/densities unavailable (no estimate)")
	}
	return occ
}

// cpuSupplyAcc accumulates, per CPU, the running cpu·ms and its
// frequency-weighted integral over the SAME segments ComputeWindowStats'
// busy loop judged (CMP-10 §7.4).
type cpuSupplyAcc struct {
	runningMs       float64
	freqWeightKHzMs float64 // Σ segment_ms × duration-weighted kHz (known segments)
	freqKnownMs     float64 // Σ segment_ms with any frequency coverage
}

// computeComputeSupplyBalance builds the CMP-10 supply ledger. Returns nil
// when the window is unbounded (nominal capacity would be an estimate) or no
// CPU was observed via sched_switch. schedCPUs is the precise
// sched-observation signal (≥1 in-window sched_switch judged by the busy
// loop); headRunnable seeds the idle-mismatch pass with threads already
// runnable at the window head (adversarial review 2026-07-04 F3).
func computeComputeSupplyBalance(idx *Index, q Query, windowMs float64, supply map[int]*cpuSupplyAcc, schedCPUs map[int]bool, headRunnable map[int]bool, cpus []CPUStats, coreByCPU map[int]string, observedFmaxByCPU map[int]int) *ComputeSupplyBalance {
	if windowMs <= 0 || len(cpus) == 0 {
		return nil
	}
	// Adversarial review 2026-07-04 F2: nominal capacity and the per-CPU
	// ledger count ONLY CPUs with in-window sched_switch activity. CPUs that
	// surface via cpu_frequency samples alone (ghost CPUs: no busy/idle
	// accounting, no switch event) are excluded wholesale and disclosed by
	// count, so the "observed via sched_switch" caveat below stays truthful
	// (counterexample pinned: 4 real + 4 freq-only cores → nominal = 4×window).
	observed := make([]CPUStats, 0, len(cpus))
	freqOnly := 0
	for _, cpu := range cpus {
		if schedCPUs[cpu.CPU] {
			observed = append(observed, cpu)
		} else {
			freqOnly++
		}
	}
	if len(observed) == 0 {
		return nil
	}
	bal := &ComputeSupplyBalance{
		WindowMs:          windowMs,
		CPUCount:          len(observed),
		NominalCapacityMs: windowMs * float64(len(observed)),
	}
	if freqOnly > 0 {
		bal.Caveats = append(bal.Caveats, fmt.Sprintf("%d CPU(s) had only cpu_frequency samples and no sched_switch activity in this window; excluded from nominal capacity and the per-CPU ledger (另 %d 核仅有频点样本无调度事件,未计入)", freqOnly, freqOnly))
	}
	for _, cpu := range observed {
		acc := supply[cpu.CPU]
		var runningMs, weightMs, knownMs float64
		if acc != nil {
			runningMs, weightMs, knownMs = acc.runningMs, acc.freqWeightKHzMs, acc.freqKnownMs
		}
		// Adversarial review 2026-07-04 F1: fmax is the max over the samples
		// that GOVERN this window — the head-governing sample (nearest
		// preceding the window start) plus the in-window samples — which is
		// exactly the window residency timeline computeCPUFrequencyResidency
		// already built (same governance set as the busy-loop residency/
		// weighting, no second collection pass). Raw pre-window history MUST
		// NOT participate: a 3.0GHz burst sample long before a window
		// governed entirely at 1.8GHz would otherwise fabricate low-frequency
		// loss for a window that never had 3.0GHz available.
		// CFC (§7.10 VS-2c 设计): the residency re-scan moved to the shared
		// window observed source (windowObservedFmaxByCPU) — same caliber,
		// computed once beside the ClusterFrequencyCeilings snapshot.
		fmax := observedFmaxByCPU[cpu.CPU]
		per := ComputeSupplyCPUBalance{
			CPU:             cpu.CPU,
			CoreClass:       coreByCPU[cpu.CPU],
			RunningMs:       runningMs,
			MaxFrequencyKHz: fmax,
			FrequencyKnown:  fmax > 0,
		}
		switch {
		case fmax > 0:
			delivered := weightMs / float64(fmax)
			if delivered > knownMs {
				delivered = knownMs // guard: weighted kHz can never exceed fmax
			}
			per.DeliveredComputeMs = delivered + (runningMs - knownMs)
			per.LowFrequencyLossMs = knownMs - delivered
		default:
			// No frequency sample at all on this CPU: weight 1.0 (§7.4
			// 无频点数据) and say so.
			per.DeliveredComputeMs = runningMs
			bal.Caveats = append(bal.Caveats, fmt.Sprintf("cpu=%d has no cpu_frequency samples in the window; its running time is weighted 1.0 (无频点数据)", cpu.CPU))
		}
		bal.DeliveredComputeMs += per.DeliveredComputeMs
		bal.LowFrequencyLossMs += per.LowFrequencyLossMs
		bal.PerCPU = append(bal.PerCPU, per)
	}
	sort.SliceStable(bal.PerCPU, func(i, j int) bool { return bal.PerCPU[i].CPU < bal.PerCPU[j].CPU })
	if bal.NominalCapacityMs > 0 {
		bal.SupplyRatio = bal.DeliveredComputeMs / bal.NominalCapacityMs
	}
	bal.IdleMismatchMs = computeIdleRunnableMismatchMs(idx, q, headRunnable)
	if rest := bal.NominalCapacityMs - bal.DeliveredComputeMs - bal.LowFrequencyLossMs - bal.IdleMismatchMs; rest > 0 {
		bal.CoreLimitedMs = rest
	}
	bal.Caveats = append(bal.Caveats,
		"delivered/nominal/low_freq_loss/core_limited are cpu·ms; idle_mismatch is wall-clock ms (∃CPU idle ∧ runnable backlog>0 = scheduling/affinity mismatch, not missing capacity); core_limited is the residual approximation",
		fmt.Sprintf("nominal capacity counts the %d CPU(s) observed via sched_switch in this window", len(observed)),
		// Adversarial review 2026-07-04 F3: the window-head runnable set is
		// seeded from pre-window prev_state=R switch segments the off-CPU
		// pass already carries; threads made runnable ONLY by a pre-window
		// wakeup with no in-window events remain invisible, so the figure is
		// a lower bound and says so.
		"idle_mismatch seeds window-head runnable threads from pre-window prev_state=R switch segments; threads woken only before the window with no in-window events stay uncounted (窗前仅被唤醒且窗内无事件的线程不计入闲置错配,该项为下界)")
	bal.Summary = fmt.Sprintf("compute_supply_balance window_ms=%.3f cpus=%d nominal=%.3fcpu·ms delivered=%.3fcpu·ms supply_ratio=%.3f low_freq_loss=%.3fcpu·ms idle_mismatch=%.3fms(wall) core_limited≈%.3fcpu·ms",
		bal.WindowMs, bal.CPUCount, bal.NominalCapacityMs, bal.DeliveredComputeMs, bal.SupplyRatio, bal.LowFrequencyLossMs, bal.IdleMismatchMs, bal.CoreLimitedMs)
	return bal
}

// computeIdleRunnableMismatchMs integrates the wall-clock time during which
// at least one observed CPU sat idle WHILE the global runnable queue was
// non-empty (CMP-10 §7.4 闲置错配). Single pass over the window's
// sched_switch/sched_wakeup stream maintaining per-CPU idle state and a
// runnable pid set — O(events) after one deterministic sort. Unknown CPU
// state (before a CPU's first switch) never counts as idle.
//
// headRunnable (adversarial review 2026-07-04 F3) seeds the runnable set
// with pids already runnable AT the window head — extracted from the
// off-CPU pass's window-head-open runnable segments (pre-window
// prev_state=R switch evidence), so a thread preempted before the window
// that never gets scheduled inside it still counts as backlog while CPUs
// idle. Seeded pids leave the set through the same switch-in rule as
// window-observed ones.
func computeIdleRunnableMismatchMs(idx *Index, q Query, headRunnable map[int]bool) float64 {
	if idx == nil {
		return 0
	}
	var evs []Event
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
			continue
		}
		switch ev.Type {
		case EventSchedSwitch, EventSchedWakeup, EventSchedWaking:
			evs = append(evs, ev)
		}
	}
	if len(evs) == 0 {
		return 0
	}
	sort.SliceStable(evs, func(i, j int) bool {
		if evs[i].Ts != evs[j].Ts {
			return evs[i].Ts < evs[j].Ts
		}
		return evs[i].Line < evs[j].Line
	})
	const (
		cpuUnknown = 0
		cpuIdle    = 1
		cpuBusy    = 2
	)
	cpuState := map[int]int{}
	idleCount := 0
	runnable := map[int]bool{}
	for pid := range headRunnable {
		if pid > 0 {
			runnable[pid] = true
		}
	}
	mismatch := 0.0
	cursor := q.TimeStart
	if cursor <= 0 || evs[0].Ts > cursor {
		cursor = evs[0].Ts
	}
	advance := func(ts float64) {
		if ts > cursor {
			if idleCount > 0 && len(runnable) > 0 {
				mismatch += (ts - cursor) * 1000
			}
			cursor = ts
		}
	}
	for _, ev := range evs {
		advance(ev.Ts)
		switch ev.Type {
		case EventSchedWakeup, EventSchedWaking:
			if ev.WakeePID > 0 {
				runnable[ev.WakeePID] = true
			}
		case EventSchedSwitch:
			if ev.NextPID > 0 {
				delete(runnable, ev.NextPID)
			}
			next := cpuBusy
			if schedNextIsIdle(ev) {
				next = cpuIdle
			}
			prev := cpuState[ev.CPU]
			if prev != next {
				if prev == cpuIdle {
					idleCount--
				}
				if next == cpuIdle {
					idleCount++
				}
				cpuState[ev.CPU] = next
			}
			if ev.PrevPID > 0 && stateFromPrevState(ev.PrevState) == StateRunnable {
				runnable[ev.PrevPID] = true
			}
		}
	}
	if q.TimeEnd > cursor {
		advance(q.TimeEnd)
	}
	return mismatch
}
