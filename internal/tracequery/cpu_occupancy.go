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
// derivedAttribution is the B-3 (§7.11) window flag: catalog TGIDs were
// soft-derived from the trace_mark span-pid vote, so derived process rows
// carry the marker and the stanza discloses the derivation.
func computeCPUOccupancyStats(q Query, windowMs float64, running map[string]ThreadDuration, pressure map[int]*cpuPressureAcc, coreByCPU map[int]string, catalog map[int]ThreadRef, cpus []CPUStats, max int, derivedAttribution bool) *CPUOccupancyStats {
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
		key := threadKey(td.Thread)
		acc := threads[key]
		if acc == nil {
			acc = &threadAcc{item: CPUOccupancyThread{Thread: td.Thread}, cpuSet: map[int]bool{}, coreSet: map[string]bool{}}
			threads[key] = acc
		} else if td.LineEnd > acc.item.LineEnd || (td.LineEnd == acc.item.LineEnd && threadDisplayLess(td.Thread, acc.item.Thread)) {
			acc.item.Thread = td.Thread
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
		derived   bool
	}
	procs := map[string]*procAcc{}
	anyDerivedProc := false
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
		if derivedAttribution && catalog[td.Thread.PID].TGID > 0 {
			acc.derived = true
			anyDerivedProc = true
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
		if acc.derived {
			acc.item.Summary += tidTgidDerivedRowMarker
		}
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
		tk := threadKey(td.Thread)
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
	if anyDerivedProc {
		occ.Caveats = append(occ.Caveats, tidTgidDerivedCaveat)
	}
	return occ
}

// computeProcessDomainCensus builds the WSR §8 b3 pid-scoped process-domain
// rollup lane (real_trace_campaign_20260705 §8.1). Attribution finding: a
// window_stats(pid=…) query never fed its pid into the rollup domain — event
// admission has no pid predicate, running is bucketed per (pid,comm,cpu) so
// cross-CPU fragmented runners get diluted below the global top-8 cut, and
// process_cpu_load then rolled up ONLY those display survivors while
// rendering "threads=N" as if it were a process census. This lane answers
// the pid question from the FULL pre-truncation running buckets (the same
// map CMP-8 occupancy consumes) plus the window thread catalog:
//
//   - ThreadCount is the true census: every distinct thread of this process
//     observed INSIDE the line+time window (same admission predicate as the
//     running side; catalog attribution via the single shared wiring point
//     processRefForThread), not the surviving-roster count.
//   - TopThreads merges each thread's running across CPUs. Wall-clock
//     soundness of that merge: sched_switch gives a thread at most one CPU
//     at any instant, so one thread's running segments on different CPUs are
//     pairwise non-overlapping and their sum is a genuine ms figure for that
//     thread. No such exclusivity exists ACROSS threads — TotalRunningMs /
//     FoldedRunningMs are cross-thread cpu·ms (CMP-3) and say so.
//   - Roster cap: the shared up-to-8 roster bound (mirror of the PTV5
//     wire-cap fold roster); overflow follows the PTS fold discipline —
//     count + aggregate, never silent truncation.
//
// The global faces (TopRunning / ThreadCPULoad / ProcessCPULoad) are not
// touched: this lane is additive and nil when the query names no target or
// the target's process identity cannot be resolved (a censusless bare pid
// must not fabricate "threads=1").
func computeProcessDomainCensus(idx *Index, q Query, running map[string]ThreadDuration, catalog map[int]ThreadRef, coreByCPU map[int]string, rosterCap int, derivedAttribution bool) *ProcessDomainCensus {
	if q.PID <= 0 && strings.TrimSpace(q.Thread) == "" && strings.TrimSpace(q.ThreadInput) == "" {
		return nil
	}
	target := resolveThread(idx, q)
	if target.PID <= 0 {
		return nil
	}
	if rosterCap <= 0 {
		rosterCap = 8
	}
	// Process identity through the shared catalog wiring point. A domain is
	// only claimable when a tgid linkage exists: either the target itself
	// carries one (native TGID column or B-3 derived vote), or the target IS
	// the tgid some catalogued member points at.
	targetRef := catalogThreadRef(catalog, target.PID, target.Comm)
	if targetRef.TGID == 0 && target.TGID > 0 {
		targetRef.TGID = target.TGID
	}
	if targetRef.TGID == 0 {
		for _, ref := range catalog {
			if ref.TGID == target.PID {
				targetRef.TGID = target.PID
				break
			}
		}
	}
	if targetRef.TGID == 0 {
		return nil
	}
	proc := processRefForThread(targetRef, catalog)
	if proc.PID <= 0 {
		return nil
	}
	if proc.Comm == "" && proc.PID == target.PID {
		proc.Comm = target.Comm
	}
	domainKey := processKey(proc)

	census := &ProcessDomainCensus{Process: proc, Target: targetRef}
	memberOf := func(thread ThreadRef) bool {
		return processKey(processRefForThread(thread, catalog)) == domainKey
	}

	// WSR 核验 F1 (2026-07-06): the census claims a WINDOW census, so member
	// OBSERVATION must pass the same line+time admission predicate as the
	// running side (ComputeWindowStats' event loop). The catalog alone is
	// line-window scoped — counting straight off it admits threads whose
	// only events sit before/after the time window (Efw-802 shape: all
	// events 0.1s pre-window still produced threads=3). The catalog stays
	// the attribution wiring point (tgid linkage/comm may legitimately be
	// recorded anywhere in scope); the time predicate gates observation.
	observed := map[int]bool{}
	if idx != nil {
		note := func(pid int) {
			if pid > 0 {
				observed[pid] = true
			}
		}
		for _, ev := range idx.Events {
			if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
				continue
			}
			note(ev.PID)
			note(ev.PrevPID)
			note(ev.NextPID)
			note(ev.WakeePID)
			if cf := ev.ConstraintFields; cf != nil {
				note(cf.PID)
			}
		}
	}

	// (1) True thread census: domain members that were actually observed
	// inside the line+time window. On B-3 derived-vote windows the tgid
	// label entry can be a display backfill rather than an observed thread;
	// the observation gate excludes event-less backfills, and those windows
	// already carry the derived caveat/marker anyway.
	derived := false
	for pid, ref := range catalog {
		if !observed[pid] {
			continue
		}
		member := catalogThreadRef(catalog, pid, ref.Comm)
		if !memberOf(member) {
			continue
		}
		census.ThreadCount++
		if derivedAttribution && member.TGID > 0 {
			derived = true
		}
	}
	if census.ThreadCount == 0 {
		return nil
	}

	// (2) Running rollup over the FULL pre-truncation buckets, merged per
	// thread across CPUs (see soundness note in the function comment).
	type memberAcc struct {
		item       ProcessDomainThread
		cpuSet     map[int]bool
		coreSet    map[string]bool
		dominantMs float64
	}
	members := map[string]*memberAcc{}
	runningPIDs := map[int]bool{}
	cpuSet := map[int]bool{}
	coreSet := map[string]bool{}
	for _, td := range running {
		if td.DurationMs <= 0 || td.Thread.PID <= 0 || !memberOf(td.Thread) {
			continue
		}
		key := threadKey(td.Thread)
		acc := members[key]
		if acc == nil {
			acc = &memberAcc{item: ProcessDomainThread{Thread: td.Thread}, cpuSet: map[int]bool{}, coreSet: map[string]bool{}}
			members[key] = acc
		} else if td.LineEnd > acc.item.LineEnd || (td.LineEnd == acc.item.LineEnd && threadDisplayLess(td.Thread, acc.item.Thread)) {
			acc.item.Thread = td.Thread
		}
		acc.item.RunningMs += td.DurationMs
		census.TotalRunningMs += td.DurationMs
		runningPIDs[td.Thread.PID] = true
		if !acc.cpuSet[td.CPU] {
			acc.cpuSet[td.CPU] = true
			acc.item.CPUs = append(acc.item.CPUs, td.CPU)
		}
		if !cpuSet[td.CPU] {
			cpuSet[td.CPU] = true
			census.CPUs = append(census.CPUs, td.CPU)
		}
		if class := coreByCPU[td.CPU]; class != "" {
			if !acc.coreSet[class] {
				acc.coreSet[class] = true
				acc.item.CoreClasses = append(acc.item.CoreClasses, class)
			}
			if !coreSet[class] {
				coreSet[class] = true
				census.CoreClasses = append(census.CoreClasses, class)
			}
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
		if census.LineStart == 0 || (td.LineStart > 0 && td.LineStart < census.LineStart) {
			census.LineStart = td.LineStart
		}
		if td.LineEnd > census.LineEnd {
			census.LineEnd = td.LineEnd
		}
	}
	census.RunningThreadCount = len(runningPIDs)
	roster := make([]ProcessDomainThread, 0, len(members))
	for _, acc := range members {
		sort.Ints(acc.item.CPUs)
		sort.SliceStable(acc.item.CoreClasses, func(i, j int) bool {
			return coreClassRank(acc.item.CoreClasses[i]) < coreClassRank(acc.item.CoreClasses[j])
		})
		roster = append(roster, acc.item)
	}
	sort.SliceStable(roster, func(i, j int) bool {
		if roster[i].RunningMs != roster[j].RunningMs {
			return roster[i].RunningMs > roster[j].RunningMs
		}
		return roster[i].LineStart < roster[j].LineStart
	})
	if len(roster) > rosterCap {
		census.TopThreads = roster[:rosterCap]
		for _, folded := range roster[rosterCap:] {
			census.FoldedThreadCount++
			census.FoldedRunningMs += folded.RunningMs
		}
	} else {
		census.TopThreads = roster
	}
	sort.Ints(census.CPUs)
	sort.SliceStable(census.CoreClasses, func(i, j int) bool {
		return coreClassRank(census.CoreClasses[i]) < coreClassRank(census.CoreClasses[j])
	})

	census.Summary = fmt.Sprintf("%s process-domain census threads=%d running_threads=%d running_total=%.3fcpu·ms",
		threadLabel(census.Process), census.ThreadCount, census.RunningThreadCount, census.TotalRunningMs)
	if len(census.TopThreads) > 0 {
		census.Summary += fmt.Sprintf(" top_thread=%s %.3fms", threadLabel(census.TopThreads[0].Thread), census.TopThreads[0].RunningMs)
	}
	if derived {
		// WSR 核验 F2 (2026-07-06): marker at the HEAD of the summary — the
		// rendered header passes through the 200-byte banner value cut and
		// a tail-appended marker could be silently truncated away.
		census.Summary = strings.TrimSpace(tidTgidDerivedRowMarker) + " " + census.Summary
	}
	// WSR 核验 F2 (2026-07-06): one clause per caveat, zh/en as separate
	// entries, each comfortably under the 200-byte banner value cap
	// (sanitizeForBanner) — the load-bearing clauses ("counts survivors,
	// not the process" / "不可当作墙钟耗时") must survive on the rendered
	// face, never live past the truncation horizon of a packed mega-caveat.
	census.Caveats = append(census.Caveats,
		fmt.Sprintf("threads=%d is the true in-window census of this process, aggregated from the full pre-truncation running buckets and in-window thread observations", census.ThreadCount),
		"process_cpu_load rows roll up only the surviving display roster; their threads= counts survivors, not the process",
		"threads= 为该进程时窗内全部可见线程普查,非展示名册幸存者数",
		"per-thread running merges the same thread's non-overlapping cross-CPU segments (wall-additive for one thread)",
		"running_total is a cross-thread cpu·ms sum and may exceed the wall window (跨线程合计为 cpu·ms,不可当作墙钟耗时)")
	return census
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
// clusterFreqDonorFor (CFR #75 簇共频) is the same-cluster donor resolver
// (cluster_freq_share.go single authority; nil or fail-open = pre-CFR
// behavior). clusterFreqDonorSource (CFR-2 #80) is the resolver's membership
// source token (explicit topology vs change-point derivation) — pure
// disclosure input for the per-CPU field and caveat wording. When a CPU has
// no own samples but a donor exists, its fmax reads the donor's
// window-observed fmax and the row disclosures say so — the busy-loop
// weighting already integrated the same donor timeline, so the ledger stays
// one caliber end to end.
func computeComputeSupplyBalance(idx *Index, q Query, windowMs float64, supply map[int]*cpuSupplyAcc, schedCPUs map[int]bool, headRunnable map[int]bool, cpus []CPUStats, coreByCPU map[int]string, observedFmaxByCPU map[int]int64, clusterFreqDonorFor func(cpu int) (int, bool), clusterFreqDonorSource string) *ComputeSupplyBalance {
	if windowMs <= 0 || len(cpus) == 0 {
		return nil
	}
	// Adversarial review 2026-07-04 F2: nominal capacity and the per-CPU
	// ledger count ONLY CPUs with an in-window switch or a proven governing
	// window-head checkpoint. CPUs that
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
		donorCPU := -1
		if fmax <= 0 && clusterFreqDonorFor != nil {
			// CFR (#75 簇共频): no own samples — the same-cluster sampled
			// sibling's window-observed fmax IS this CPU's fmax (one shared
			// hardware frequency point); membership from explicit topology or
			// the CFR-2 change-point derivation, per the resolver. Fail-open
			// keeps the honest 无频点数据 branch below.
			if donor, ok := clusterFreqDonorFor(cpu.CPU); ok {
				if donorFmax := observedFmaxByCPU[donor]; donorFmax > 0 {
					fmax, donorCPU = donorFmax, donor
				}
			}
		}
		per := ComputeSupplyCPUBalance{
			CPU:             cpu.CPU,
			CoreClass:       coreByCPU[cpu.CPU],
			RunningMs:       runningMs,
			MaxFrequencyKHz: fmax,
			FrequencyKnown:  fmax > 0,
		}
		if donorCPU >= 0 {
			donor := donorCPU
			per.FrequencyClusterDonorCPU = &donor
			// CFR-2 (#80) 披露区分: the row and the caveat both say where the
			// cluster membership came from. Explicit wording is byte-identical
			// to the CFR #75 original (pinned).
			per.FrequencyClusterDonorSource = clusterFreqDonorSource
			if clusterFreqDonorSource == ClusterFreqSourceDerived {
				bal.Caveats = append(bal.Caveats, fmt.Sprintf("cpu=%d has no own cpu_frequency samples; frequency taken from same-cluster cpu=%d, clusters derived from frequency change points (cpu%d 频点=同簇 cpu%d,簇共频复用,频点变化点推导)", cpu.CPU, donor, cpu.CPU, donor))
			} else {
				bal.Caveats = append(bal.Caveats, fmt.Sprintf("cpu=%d has no own cpu_frequency samples; frequency taken from same-cluster cpu=%d under explicit core_topology (cpu%d 频点=同簇 cpu%d,簇共频复用)", cpu.CPU, donor, cpu.CPU, donor))
			}
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
		fmt.Sprintf("nominal capacity counts the %d CPU(s) observed by an in-window sched_switch or a proven window-head scheduler checkpoint", len(observed)),
		"idle_mismatch seeds window-head runnable threads from the scheduler head snapshot (including pre-window wakeups and prev_state=R); any subject missing a checkpoint is disclosed by scheduler_head_coverage and its unknown head segment remains omitted (该项为下界)")
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
	if schedulerStateIntegrityFailureForQuery(idx, q, 0) != nil {
		return 0
	}
	if threadIncarnationConflictForQuery(idx, q, 0) != nil {
		return 0
	}
	var evs []Event
	for _, ev := range idx.Events {
		// SUPP-HYG P3-C (§29.81 立案, 2026-07-14): cooperative sampling point —
		// this collection scan sat between two ComputeWindowStats boundary
		// samples with zero interior points. On fire the mismatch value is
		// abandoned (0); the caller's whole stats face is discarded by the
		// attach gate anyway (禁半账).
		if q.runCancel.tick() {
			return 0
		}
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
			continue
		}
		switch ev.Type {
		case EventSchedSwitch, EventSchedWakeup, EventSchedWaking:
			evs = append(evs, ev)
		}
	}
	if q.TimeStart > 0 && q.LineStart == 0 && q.LineEnd == 0 {
		if head := schedulerHeadForQuery(idx, q); head != nil && head.Complete {
			boundarySwitchCPUs := map[int]bool{}
			for _, ev := range evs {
				if ev.Type == EventSchedSwitch && ev.Ts == q.TimeStart {
					boundarySwitchCPUs[ev.CPU] = true
				}
			}
			for _, state := range schedulerHeadSortedCPUs(head) {
				if !boundarySwitchCPUs[state.CPU] {
					evs = append(evs, Event{Type: EventSchedSwitch, Ts: q.TimeStart, CPU: state.CPU, NextPID: state.Thread.PID, NextComm: state.Thread.Comm, Line: state.Line})
				}
			}
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
		// SUPP-HYG P3-C: sweep-loop sampling point (same discard contract as
		// the collection loop above).
		if q.runCancel.tick() {
			return 0
		}
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
