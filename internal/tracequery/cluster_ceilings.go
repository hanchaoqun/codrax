package tracequery

// cluster_ceilings.go — CFC batch (§7.10 VS-2c 设计): the single shared home
// for (a) the per-CPU sample admission predicates (cpu_frequency AND
// cpu_frequency_limits) and (b) the per-cluster frequency-ceiling core, so
// the historical collection lanes (window face freqByCPU /
// limitTimelineByCPU, chain/fold face chainQueryCache.freqByCPU /
// freqLimitByCPU, corroboration clock lane) stop drifting.
//
// (a) isPerCPUFrequencySample is THE membership test for any per-CPU
//     frequency basis (window residency/weighting AND fold governance). It
//     excludes clock_set_rate lanes reclassified to Type=EventCPUFrequency by
//     the isCPUFrequencyClock NAME HEURISTIC (parse.go): those lanes carry
//     vendor-free-vocabulary names, vendor-free units, and emitting-CPU
//     attribution (hmtrace hardcodes cpu 0), so letting them into a per-CPU
//     timeline fabricated cpu0 residency, flipped the inferred core topology,
//     minted false low_frequency verdicts and forged compute-supply
//     low-frequency loss. Their max still surfaces — corroboration caveat
//     only — through buildClockLaneIndex (§7.10 终局裁定, untouched).
//
// (b) ClusterFrequencyCeiling is the deduped "what was the fastest this
//     cluster could go in this window" answer. The VALUE is the max() over
//     the per-cluster evidence lanes (R5c 终判 §29.96.1, 2026-07-15 — 最大
//     可能频点: window-governing cpu_frequency_limits Max vs the highest
//     window-governing observed cpu_frequency sample, whichever is larger;
//     the source token names the winning lane, limit on ties). The two
//     faces (fold: supplyFoldGlobalMaxBasis — which superseded the retired
//     window-governed supplyFoldBigClusterFmax under R5; window: WindowStats.
//     ClusterFrequencyCeilings) share the CORE FUNCTION ONLY (ladder +
//     clustering, computeClusterFrequencyCeilings): each face feeds its own
//     input caliber — the window snapshot folds the query window's
//     FrequencyResidency timeline, the fold reads full-trace governance
//     samples over its own governance window — so the two faces'
//     numbers can legitimately diverge; one face's snapshot must
//     never substitute for the other (F2 复核措辞, 2026-07-05). What can
//     never fork again is the ladder and the clustering, not the values.

import (
	"math"
	"sort"
)

// isPerCPUFrequencySample reports whether ev may enter a per-CPU frequency
// timeline (residency, busy-segment weighting, fold governance, completeness
// caveats). PRECISE signals only: typed EventType, positive payload, verbatim
// event-name exclusion of the reclassified clock lane (the reclassification
// keeps Name=="clock_set_rate" while flipping Type — parse.go).
//
// Known residual (recorded, deliberately NOT covered): parse.go also carries a
// generalized reclassification (rawType containing cpu+freq →
// EventCPUFrequency) whose events keep Name==rawType != "clock_set_rate".
// Those shapes normally carry a cpu_id key (CPUForFieldValid=true, correct
// attribution), so they are legitimate per-CPU samples; widening this
// predicate to a name heuristic would violate the precise-signals red line.
func isPerCPUFrequencySample(ev Event) bool {
	_, _, ok := perCPUFrequencySampleValues(ev)
	return ok
}

// IsPerCPUFrequencySample exposes the same positive hard-evidence verdict to
// diagnostic/report packages. It prevents those faces from counting
// canonical zero barriers, malformed rows, or reclassified clock lanes as
// cpu_frequency samples through a parallel Type-only test.
func IsPerCPUFrequencySample(ev Event) bool {
	return isPerCPUFrequencySample(ev)
}

// perCPUFrequencySampleValues is the fixed-width authority shared by every
// hard frequency consumer. It deliberately returns int64: the text wire is
// uint32 and must retain the same meaning on 32- and 64-bit hosts before any
// public/display projection.
func perCPUFrequencySampleValues(ev Event) (cpu int, khz int64, ok bool) {
	cpu, khz, ok = perCPUFrequencyTransitionValues(ev)
	return cpu, khz, ok && khz > 0
}

// perCPUFrequencyTransitionValues includes the canonical zero transition.
// Zero is inventory, not a frequency sample, but it must remain in state
// timelines so an offline/unknown transition cuts off an older positive
// carry-in instead of silently bridging across it.
func perCPUFrequencyTransitionValues(ev Event) (cpu int, khz int64, ok bool) {
	if ev.Type != EventCPUFrequency || ev.Name == "clock_set_rate" || ev.CPUInputInvalid || !eventCPUScalarKnown(ev) ||
		!ev.CPUForFieldValid || !validTraceCPUIndex(ev.CPUForField) {
		return 0, 0, false
	}
	khz, ok = cpuScalarSemanticKHz(ev.Frequency, true)
	if !ok {
		return 0, 0, false
	}
	return ev.CPUForField, khz, true
}

// isPerCPULimitSample is THE admission predicate for any per-CPU
// cpu_frequency_limits collection basis — the VS-2b ladder's step-2
// policy-ceiling source (window face: ComputeWindowStats limitTimelineByCPU;
// fold face: chainQueryCache.buildFreqLimitIndex). PRECISE signals only:
// typed EventType, positive policy Max, and a proven payload cpu_id. The
// emitting header CPU is never an ownership fallback. The returned cpu is
// meaningful only when ok. Window
// filtering is deliberately NOT admission: each caller applies its own
// documented window convention at the call site. Cross-face member identity
// over one event set is pinned by
// TestPerCPULimitSampleAdmissionCrossFaceIdentity.
//
// NOT this caliber: stats.CPUFrequencyLimits — the strict in-window DISPLAY
// accumulation (accumulateCPUFrequencyLimit) — also counts zero-Max rows and
// stays an independent, untouched caliber (fork pinned by
// TestComputeWindowStats_ClusterFrequencyCeilingsSnapshot).
func isPerCPULimitSample(ev Event) (cpu int, ok bool) {
	cpu, _, maxKHz, ok := perCPULimitSampleValues(ev)
	if !ok || maxKHz <= 0 {
		return 0, false
	}
	return cpu, true
}

// perCPULimitSampleValues is the atomic limits twin of
// perCPUFrequencySampleValues. A zero max is valid typed inventory but is not
// a governing ceiling (isPerCPULimitSample applies that final role gate).
func perCPULimitSampleValues(ev Event) (cpu int, minKHz, maxKHz int64, ok bool) {
	if ev.Type != EventCPUFrequencyLimit || ev.CPUInputInvalid || !eventCPUScalarKnown(ev) || ev.FrequencyMin > ev.FrequencyMax ||
		!ev.CPUForFieldValid || !validTraceCPUIndex(ev.CPUForField) {
		return 0, 0, 0, false
	}
	minKHz, minOK := cpuScalarSemanticKHz(ev.FrequencyMin, true)
	maxKHz, maxOK := cpuScalarSemanticKHz(ev.FrequencyMax, true)
	if !minOK || !maxOK {
		return 0, 0, 0, false
	}
	return ev.CPUForField, minKHz, maxKHz, true
}

func cpuScalarSemanticKHz(value int64, allowZero bool) (int64, bool) {
	if value < 0 || value > math.MaxUint32 || !allowZero && value == 0 {
		return 0, false
	}
	return value, true
}

func roundedCPUFrequencyKHz(value float64) int64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > math.MaxUint32 {
		return 0
	}
	return int64(math.Round(value))
}

// ClusterFrequencyCeiling is one core-class cluster's frequency ceiling for a
// governance window. CoreClass is "" for CPUs the topology could not classify
// (they form their own pseudo-cluster; when NO CPU classifies, every governed
// CPU folds into the single "" entry — the same freq_only pooling semantics
// the fold basis lane keeps today in supplyFoldGlobalMaxBasis, inherited from
// the retired supplyFoldBigClusterFmax). Source is one of SupplyFoldFmaxSourceLimit /
// SupplyFoldFmaxSourceObserved (VS-2b ladder steps 2/3 — no new token).
// Internal computation snapshot, NOT an observation face: WindowStats carries
// it with json:"-" so no JSON surface and no registry token appears. Its one
// display consumer is the soft window_stats stanza line (F2(a):
// writeTraceClusterFrequencyCeilings, internal/tool/trace_query.go) — display
// text only, no gate reads it.
type ClusterFrequencyCeiling struct {
	CoreClass string
	CPUs      []int
	FmaxKHz   int64
	Source    string

	// Unexported companions for behavior-equivalent consumers: the observed
	// rung value even when Source==limit, and the limits rung when present.
	observedFmaxKHz int64
	limitFmaxKHz    int64
}

// computeClusterFrequencyCeilings is the shared CFC core. observedFmaxByCPU
// is the caller's governance-resolved per-CPU observed fmax (window face:
// residency max over the F1-governed timeline; fold face: max over
// governedFreqSamples); coreByCPU the caller's resolveCoreTopology result;
// limitMaxKHz the governance-resolved cpu_frequency_limits Max lookup (nil =
// no limits source). Only CPUs with an observed sample participate — a CPU
// with a limits row but no cpu_frequency sample never mints a ceiling (same
// membership rule the fold face always used). Output is sorted small →
// middle → big → unclassified.
func computeClusterFrequencyCeilings(observedFmaxByCPU map[int]int64, coreByCPU map[int]string, limitMaxKHz func(cpu int) int64) []ClusterFrequencyCeiling {
	byClass := map[string]*ClusterFrequencyCeiling{}
	for cpu, observed := range observedFmaxByCPU {
		if observed <= 0 {
			continue
		}
		class := coreByCPU[cpu]
		if coreClassRank(class) >= 3 {
			// Defensive normalization: every non-{small,middle,big} label is
			// ONE unclassified pool, exactly like the fold's bestRank<0 path.
			class = ""
		}
		g := byClass[class]
		if g == nil {
			g = &ClusterFrequencyCeiling{CoreClass: class}
			byClass[class] = g
		}
		g.CPUs = append(g.CPUs, cpu)
		if observed > g.observedFmaxKHz {
			g.observedFmaxKHz = observed
		}
		if limitMaxKHz != nil {
			if limit := limitMaxKHz(cpu); limit > g.limitFmaxKHz {
				g.limitFmaxKHz = limit
			}
		}
	}
	out := make([]ClusterFrequencyCeiling, 0, len(byClass))
	for _, g := range byClass {
		sort.Ints(g.CPUs)
		// R5c 终判 (§29.96.1, 2026-07-15) — 各簇 fmax 同法: max() over the
		// evidence lanes replaces the limit-first priority (an observed peak
		// above a pressed/stale limits ceiling now wins — 整文件被压 shape);
		// the source token attributes the winning lane (limit on ties).
		if g.limitFmaxKHz > 0 && g.limitFmaxKHz >= g.observedFmaxKHz {
			g.FmaxKHz, g.Source = g.limitFmaxKHz, SupplyFoldFmaxSourceLimit
		} else {
			g.FmaxKHz, g.Source = g.observedFmaxKHz, SupplyFoldFmaxSourceObserved
		}
		out = append(out, *g)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return coreClassRank(out[i].CoreClass) < coreClassRank(out[j].CoreClass)
	})
	return out
}

// pickBigClusterCeiling selects the WINDOW face's reference cluster: the
// highest KNOWN core class present, else the single unclassified pool, else
// nil. It preserves the bestRank selection of the RETIRED window-governed
// fold basis (supplyFoldBigClusterFmax, superseded by the trace-global
// supplyFoldGlobalMaxBasis under R5 §29.88.3) byte for byte — the window
// snapshot face deliberately kept that caliber.
func pickBigClusterCeiling(ceilings []ClusterFrequencyCeiling) *ClusterFrequencyCeiling {
	var best *ClusterFrequencyCeiling
	for i := range ceilings {
		if ceilings[i].CoreClass == "" {
			if best == nil {
				best = &ceilings[i]
			}
			continue
		}
		if best == nil || best.CoreClass == "" || coreClassRank(ceilings[i].CoreClass) > coreClassRank(best.CoreClass) {
			best = &ceilings[i]
		}
	}
	return best
}

// governedMaxKHz is the max sample governing [gStart, gEnd] on one ts-ordered
// timeline (head-governing + in-window caliber — governedWindowSamples).
// Shared by the chain-face limits rung (governedLimitMaxKHz) and the window
// snapshot so the two ladder inputs read one caliber.
func governedMaxKHz(samples []freqSample, gStart, gEnd float64) int64 {
	var max int64
	for _, sample := range governedWindowSamples(samples, gStart, gEnd) {
		if sample.khz > max {
			max = sample.khz
		}
	}
	return max
}

// windowObservedFmaxByCPU extracts the window face's per-CPU observed fmax:
// the max over each CPU's FrequencyResidency segments — the F1-governed
// timeline (head-governing sample + in-window samples) ComputeWindowStats
// already built. This is the SAME caliber computeComputeSupplyBalance always
// used; it is now computed once and shared (compute_supply per-CPU fmax and
// the ceilings snapshot both read it).
func windowObservedFmaxByCPU(cpus []CPUStats) map[int]int64 {
	out := map[int]int64{}
	for _, cpu := range cpus {
		var fmax int64
		for _, res := range cpu.FrequencyResidency {
			if res.Frequency > fmax {
				fmax = res.Frequency
			}
		}
		if fmax > 0 {
			out[cpu.CPU] = fmax
		}
	}
	return out
}

// computeWindowClusterFrequencyCeilings builds the WindowStats snapshot. The
// limits rung deliberately reads the head-governing + in-window caliber
// (limits rows only fire on CHANGE, so a window with no in-window row is
// still governed by the last pre-window row) — an intentional fork from
// stats.CPUFrequencyLimits' strict in-window display caliber, which stays
// untouched. An unbounded window end governs with +Inf (every collected
// sample participates; collection already applied the upper bound).
func computeWindowClusterFrequencyCeilings(observedFmaxByCPU map[int]int64, coreByCPU map[int]string, limitTimelineByCPU map[int][]freqSample, q Query) []ClusterFrequencyCeiling {
	gStart := q.TimeStart
	gEnd := q.TimeEnd
	if gEnd <= 0 {
		gEnd = math.Inf(1)
	}
	var limitFn func(cpu int) int64
	if len(limitTimelineByCPU) > 0 {
		limitFn = func(cpu int) int64 { return governedMaxKHz(limitTimelineByCPU[cpu], gStart, gEnd) }
	}
	return computeClusterFrequencyCeilings(observedFmaxByCPU, coreByCPU, limitFn)
}
