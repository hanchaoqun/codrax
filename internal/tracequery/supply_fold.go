package tracequery

// supply_fold.go — VS-2 (§7.10, docs/design/customer_dead_session_audit_
// 20260703.md): supply-fold accounting for on-chain RUNNING-dominant wakeup-
// chain nodes. Each running slice is folded from its own CPU's governed
// frequency to the big-cluster fmax:
//
//	ideal = Σ slice_ms × min(1, f_slice / f_bigmax)
//	SupplyFoldDeficit = RunningMs − ideal   (clamped ≥ 0)
//
// so the deficit answers "how much of this thread's running wall clock is
// running-SLOW (weaker core / governed-down frequency), not running-MUCH".
// The wording contract downstream is pinned to "按频点折算,不含微架构差异,
// 缺口为下界" — frequency ratio only, never an IPC/microarchitecture claim.
//
// Governance caliber (§7.10 (4), reusing the CMP-10 F1 ruling): a slice's
// frequency and the big-cluster fmax read ONLY the samples that GOVERN the
// analysis window — the head-governing sample (nearest at or before the
// window start) plus the in-window samples. Raw pre-window history MUST NOT
// participate: a stale 3.0GHz burst long before a window governed at 2.0GHz
// would otherwise fabricate deficit for capacity that never existed in the
// window.
//
// Cluster caliber (CMP-C reuse): CPUs classify through the SAME
// resolveCoreTopology entry the window-stats face uses (explicit
// Query.CoreTopology first, then the frequency-tier inference over the
// governed per-CPU fmax). The big cluster is the highest core class among
// CPUs that actually have governed samples; its fmax is that cluster's
// highest governed sample. No classification at all (single-tier or unknown
// topology) folds every governed CPU as one cluster.
//
// Missing data is NEVER a fabricated deficit (§7.10 无频点数据 rule): a slice
// with unknown CPU or no governed frequency folds at ratio 1 (ideal = wall)
// and is booked as UNKNOWN basis; SupplyFoldBasis keeps the known/unknown
// wall split so the display layer can refuse the affirmative "ran at full
// frequency" claim unless the basis is fully known.

import "sort"

// SupplyFoldBasis records how much of a folded node's running wall clock had
// governed frequency coverage (known) vs none (unknown). KnownMs+UnknownMs
// equals the folded RunningMs; ratios derive from the two fields. Typed
// display input only — no gate reads it.
type SupplyFoldBasis struct {
	KnownMs   float64 `json:"known_ms"`
	UnknownMs float64 `json:"unknown_ms"`

	// VS-2b (§7.10) fmax ladder provenance: FmaxKHz is the big-cluster fmax
	// the fold divided by; FmaxSource says which ladder step supplied it —
	// "limit" (in-trace cpu_frequency_limits, the most authoritative offline
	// source: the cpufreq POLICY ceiling, which includes thermal caps and is
	// NOT the hardware rated maximum, so the deficit's "下界" wording stands)
	// or "observed" (highest window-governing cpu_frequency sample, the
	// fallback). Empty/zero when no governed sample existed anywhere.
	FmaxKHz    int    `json:"fmax_khz,omitempty"`
	FmaxSource string `json:"fmax_source,omitempty"`

	// VS-2b companion finding (typed comparison, display renders the soft
	// clause): the big cluster's governing limits.Max sat BELOW a higher
	// cpu_frequency sample observed on the same cluster elsewhere in the
	// loaded trace — part of the deficit is policy/thermal throttling, not
	// scheduling. TraceObservedMaxKHz is that full-trace observed maximum.
	LimitThrottled      bool `json:"limit_throttled,omitempty"`
	TraceObservedMaxKHz int  `json:"trace_observed_max_khz,omitempty"`

	// VS-2c(a) cluster-lane corroboration (§7.10 终局裁定): the highest
	// in-window sample of any cpu-freq-NAMED clock_set_rate lane. Lane names
	// are vendor free vocabulary — corroboration ONLY, never the fold basis.
	// ClusterLaneDivergent is the precise disagreement flag: the raw lane
	// sample matched the fold fmax within ±10% under NO unit hypothesis
	// (clusterLaneUnitDivisors — vendors are free vocabulary about units
	// too). When a hypothesis matches, ClusterLaneMaxKHz holds the resolved
	// kHz value; when divergent (or no fmax exists) it holds the RAW lane
	// value with the unit unresolved. The display caveat renders only when
	// the flag is set and must say the unit is unknown (单位不明).
	ClusterLaneName      string `json:"cluster_lane_name,omitempty"`
	ClusterLaneMaxKHz    int    `json:"cluster_lane_max_khz,omitempty"`
	ClusterLaneDivergent bool   `json:"cluster_lane_divergent,omitempty"`
}

// Typed FmaxSource values (VS-2b ladder steps 2 and 3; step 1 — sysfs — is
// unreachable offline by definition).
const (
	SupplyFoldFmaxSourceLimit    = "limit"
	SupplyFoldFmaxSourceObserved = "observed"
)

// AllKnown reports a fully-covered fold: every running slice had a governed
// frequency sample. Only then may the display layer make the affirmative
// "已满频满核,running 属真实工作量" claim (§7.10 fourth branch).
func (b SupplyFoldBasis) AllKnown() bool {
	return b.UnknownMs == 0 && b.KnownMs > 0
}

// supplyFoldGovernanceWindow picks the window whose governance set the fold
// reads: the bounded query window when the caller supplied one, else the
// node's own aligned window (chain expansion always has both ends).
func supplyFoldGovernanceWindow(q Query, nodeStart, nodeEnd float64) (float64, float64) {
	if q.TimeStart > 0 && q.TimeEnd > q.TimeStart {
		return q.TimeStart, q.TimeEnd
	}
	return nodeStart, nodeEnd
}

// governedFreqSamples returns cpu's samples that GOVERN [gStart, gEnd]: the
// head-governing sample (nearest at or before gStart, re-timestamped to the
// window head) plus every in-window sample. Empty when the CPU has no
// governing sample at all. The pre-window tail beyond the head sample is
// deliberately absent (CMP-10 F1 — see the file header).
func (c *chainQueryCache) governedFreqSamples(cpu int, gStart, gEnd float64) []freqSample {
	c.buildFreqIndex()
	return governedWindowSamples(c.freqByCPU[cpu], gStart, gEnd)
}

// governedWindowSamples is the shared governance selection over any
// ts-ordered sample timeline: head-governing sample (re-timestamped to the
// window head) + in-window samples. Used by both the cpu_frequency and the
// cpu_frequency_limits timelines so the two ladder steps read the SAME
// window-governance caliber.
func governedWindowSamples(samples []freqSample, gStart, gEnd float64) []freqSample {
	if len(samples) == 0 {
		return nil
	}
	// First sample strictly after gStart.
	pos := sort.Search(len(samples), func(i int) bool { return samples[i].ts > gStart })
	var out []freqSample
	if pos > 0 {
		head := samples[pos-1]
		head.ts = gStart
		out = append(out, head)
	}
	for i := pos; i < len(samples); i++ {
		if samples[i].ts > gEnd {
			break
		}
		out = append(out, samples[i])
	}
	return out
}

// buildFreqLimitIndex lazily scans the index for cpu_frequency_limits events
// (VS-2b ladder step 2). Mirrors buildFreqIndex: per-CPU, ts-ordered, kHz
// (FrequencyMax — the policy ceiling).
func (c *chainQueryCache) buildFreqLimitIndex() {
	if c.freqLimitOnce {
		return
	}
	c.freqLimitOnce = true
	c.freqLimitByCPU = map[int][]freqSample{}
	if c.idx == nil {
		return
	}
	for _, ev := range c.idx.Events {
		// CFC F1: admission + CPU attribution via the shared limits predicate
		// (isPerCPULimitSample, cluster_ceilings.go). No window filter here —
		// THIS face's convention is a full-trace timeline with window
		// governance applied at query time (governedLimitMaxKHz).
		cpu, ok := isPerCPULimitSample(ev)
		if !ok {
			continue
		}
		c.freqLimitByCPU[cpu] = append(c.freqLimitByCPU[cpu], freqSample{ts: ev.Ts, khz: ev.FrequencyMax})
	}
}

// governedLimitMaxKHz returns the highest cpu_frequency_limits Max governing
// [gStart, gEnd] on cpu (head-governing sample + in-window samples — the same
// caliber as governedFreqSamples). 0 = no governing limits row for this CPU.
func (c *chainQueryCache) governedLimitMaxKHz(cpu int, gStart, gEnd float64) int {
	c.buildFreqLimitIndex()
	return governedMaxKHz(c.freqLimitByCPU[cpu], gStart, gEnd)
}

// clockLaneSample is one cpu-freq-NAMED clock_set_rate lane sample (VS-2c
// corroboration only — see buildClockLaneIndex).
type clockLaneSample struct {
	ts   float64
	khz  int
	name string
}

// buildClockLaneIndex lazily collects clock_set_rate samples whose lane name
// hits the isCPUFrequencyClockName heuristic (NOISY name signal — §7.10
// VS-2c 终局裁定: vendor free vocabulary, corroboration caveat only, NEVER
// the fold basis). Note the reclassified events carry Type=EventCPUFrequency
// but keep Name="clock_set_rate" — the verbatim name is the precise
// membership signal here, matching the buildFreqIndex exclusion.
func (c *chainQueryCache) buildClockLaneIndex() {
	if c.clockLaneOnce {
		return
	}
	c.clockLaneOnce = true
	if c.idx == nil {
		return
	}
	for _, ev := range c.idx.Events {
		if ev.Name != "clock_set_rate" || ev.Frequency <= 0 {
			continue
		}
		if !isCPUFrequencyClockName(ev.ClockName) {
			continue
		}
		// Vendor lanes are free-form about UNITS too (kHz, Hz, MHz all exist
		// in the wild), so the sample is kept RAW here. The old single-sided
		// "≥1e8 must be Hz" threshold produced REVERSED caveats for the
		// sub-100MHz-Hz shape (96MHz emitted as 96000000 stayed "kHz") and
		// for MHz lanes; unit resolution now happens at comparison time via
		// magnitude-tolerant hypothesis matching against the fold fmax
		// (applyClusterLaneCorroboration, 2026-07-04 review).
		c.clockLaneSamples = append(c.clockLaneSamples, clockLaneSample{ts: ev.Ts, khz: ev.Frequency, name: ev.ClockName})
	}
}

// clusterLaneUnitDivisors are the unit hypotheses tried when comparing a raw
// clock_set_rate lane sample against the fold fmax (kHz): the lane value AS
// kHz (÷1), as Hz (÷1e3), and as a ×1e6-scaled shape (÷1e6). Vendor lanes are
// free vocabulary about units, so this is a SOFT magnitude match feeding a
// corroboration caveat only — never the fold basis (§7.10 终局裁定).
var clusterLaneUnitDivisors = [...]float64{1, 1e3, 1e6}

// applyClusterLaneCorroboration (VS-2c(a), §7.10) records the in-window
// cluster-lane maximum beside the fold fmax and raises the precise >10%
// divergence flag. Consistent lanes leave the flag unset (一致时不加注);
// the lane value never replaces the fmax (pinned).
//
// 2026-07-04 review: the comparison is magnitude-tolerant. The raw lane
// sample is normalized under each clusterLaneUnitDivisors hypothesis; ANY
// hypothesis landing within ±10% of the fmax declares agreement and stores
// the resolved kHz value. Only when EVERY hypothesis misses does the
// divergence flag set — the stored value then stays RAW (unit unresolved)
// and the display caveat says the lane unit is unknown (单位不明) instead of
// asserting a false direction, which the old single-sided ≥1e8 Hz threshold
// did for sub-100MHz-Hz and MHz lanes.
func (c *chainQueryCache) applyClusterLaneCorroboration(basis *SupplyFoldBasis, gStart, gEnd float64) {
	c.buildClockLaneIndex()
	maxRaw, name := 0, ""
	for _, sample := range c.clockLaneSamples {
		if sample.ts < gStart || sample.ts > gEnd {
			continue
		}
		if sample.khz > maxRaw {
			maxRaw, name = sample.khz, sample.name
		}
	}
	if maxRaw <= 0 {
		return
	}
	basis.ClusterLaneName = name
	basis.ClusterLaneMaxKHz = maxRaw
	if basis.FmaxKHz <= 0 {
		return
	}
	fmax := float64(basis.FmaxKHz)
	for _, div := range clusterLaneUnitDivisors {
		norm := float64(maxRaw) / div
		diff := norm - fmax
		if diff < 0 {
			diff = -diff
		}
		if diff <= 0.10*fmax {
			basis.ClusterLaneMaxKHz = int(norm)
			basis.ClusterLaneDivergent = false
			return
		}
	}
	basis.ClusterLaneDivergent = true
}

// governedFrequencyAt returns the governed frequency in effect at ts: the
// last governed sample at or before ts, falling back to the nearest LATER
// governed sample when none precedes it (R5e head rule, restricted to the
// governance set). 0 = no governed sample on this CPU.
func governedFrequencyAt(samples []freqSample, ts float64) int {
	best := 0
	for _, sample := range samples {
		if sample.ts <= ts {
			best = sample.khz
			continue
		}
		break
	}
	if best == 0 && len(samples) > 0 {
		best = samples[0].khz
	}
	return best
}

// supplyFoldFmax is the resolved big-cluster fmax plus its VS-2b ladder
// provenance (see SupplyFoldBasis for the field semantics).
type supplyFoldFmax struct {
	khz                 int
	source              string
	throttled           bool
	traceObservedMaxKHz int
}

// supplyFoldBigClusterFmax resolves the big-cluster fmax for the governance
// window. Cluster membership: classify the CPUs that have governed
// cpu_frequency samples via the CMP-C entry (explicit topology first, then
// frequency-tier inference over the governed per-CPU fmax) and take the
// highest class present. The fmax VALUE then walks the VS-2b ladder (§7.10):
//
//	(2) any cluster CPU with a window-governing cpu_frequency_limits row →
//	    fmax = the cluster's highest governing limits.Max (policy authority);
//	(3) otherwise fmax = the cluster's highest window-governing cpu_frequency
//	    sample (observed fallback — the pre-VS-2b behavior).
//
// Ladder step (1) — sysfs cpuinfo_max_freq/scaling_max_freq — is unreachable
// in an offline trace by definition. Zero khz = no governed sample anywhere
// (the fold then books everything as unknown basis). The companion
// throttling finding compares the limits fmax against the same cluster's
// highest cpu_frequency sample over the FULL loaded trace (zero new
// parsing): observed > limit ⇒ part of the gap is policy/thermal capping.
//
// CFC (§7.10 VS-2c 设计): the ladder + clustering now route through the
// shared computeClusterFrequencyCeilings core (cluster_ceilings.go); this
// adapter supplies the fold face's governance-resolved inputs and picks the
// big cluster. Behavior-equivalent to the pre-CFC inline implementation
// (pinned by the VS-2 golden fold tests + the clock-lane ruling pins).
func (c *chainQueryCache) supplyFoldBigClusterFmax(q Query, gStart, gEnd float64) supplyFoldFmax {
	c.buildFreqIndex()
	governedFmax := map[int]int{}
	for cpu, samples := range c.freqByCPU {
		if fmax := governedMaxKHz(samples, gStart, gEnd); fmax > 0 {
			governedFmax[cpu] = fmax
		}
	}
	if len(governedFmax) == 0 {
		return supplyFoldFmax{}
	}
	cpus := make([]CPUStats, 0, len(governedFmax))
	for cpu, fmax := range governedFmax {
		cpus = append(cpus, CPUStats{CPU: cpu, Frequency: fmax})
	}
	sort.SliceStable(cpus, func(i, j int) bool { return cpus[i].CPU < cpus[j].CPU })
	coreByCPU, _ := resolveCoreTopology(cpus, q.CoreTopology)
	ceilings := computeClusterFrequencyCeilings(governedFmax, coreByCPU, func(cpu int) int {
		return c.governedLimitMaxKHz(cpu, gStart, gEnd)
	})
	top := pickBigClusterCeiling(ceilings)
	if top == nil {
		return supplyFoldFmax{}
	}
	traceMax := 0
	for _, cpu := range top.CPUs {
		for _, sample := range c.freqByCPU[cpu] {
			if sample.khz > traceMax {
				traceMax = sample.khz
			}
		}
	}
	out := supplyFoldFmax{khz: top.FmaxKHz, source: top.Source, traceObservedMaxKHz: traceMax}
	if top.Source == SupplyFoldFmaxSourceLimit {
		// Typed throttling comparison (int vs int, precise); the clause it
		// feeds is display-side soft wording only.
		out.throttled = traceMax > top.FmaxKHz
	}
	return out
}

// supplyFoldRunningIntervals folds every RUNNING interval of one causal-impact
// node (see the file header for the caliber). Slice boundaries follow the
// governed frequency change points of the interval's own CPU (R5e: in-window
// frequency changes are honored segment by segment); each sub-slice's wall
// share is taken as a fraction of the interval's DurationMs so the identity
//
//	idealMs + deficit == RunningMs   and   KnownMs + UnknownMs == RunningMs
//
// holds exactly. Returns the folded ideal and the known/unknown basis; the
// caller derives the deficit (clamped ≥ 0).
func (c *chainQueryCache) supplyFoldRunningIntervals(q Query, nodeStart, nodeEnd float64, intervals []Interval) (float64, SupplyFoldBasis) {
	gStart, gEnd := supplyFoldGovernanceWindow(q, nodeStart, nodeEnd)
	fm := c.supplyFoldBigClusterFmax(q, gStart, gEnd)
	bigFmax := fm.khz
	var idealMs float64
	var basis SupplyFoldBasis
	basis.FmaxKHz = fm.khz
	basis.FmaxSource = fm.source
	basis.LimitThrottled = fm.throttled
	if fm.throttled {
		basis.TraceObservedMaxKHz = fm.traceObservedMaxKHz
	}
	c.applyClusterLaneCorroboration(&basis, gStart, gEnd)
	for _, it := range intervals {
		if it.State != StateRunning || it.DurationMs <= 0 {
			continue
		}
		if bigFmax <= 0 || !it.CPUKnown {
			// No fold reference or no slice CPU: never fabricate a deficit.
			basis.UnknownMs += it.DurationMs
			idealMs += it.DurationMs
			continue
		}
		samples := c.governedFreqSamples(it.CPU, gStart, gEnd)
		wall := it.EndTs - it.StartTs
		if wall <= 0 {
			// Degenerate interval: single lookup at its start.
			freq := governedFrequencyAt(samples, it.StartTs)
			idealMs += supplyFoldSliceIdeal(it.DurationMs, freq, bigFmax, &basis)
			continue
		}
		boundaries := []float64{it.StartTs}
		for _, sample := range samples {
			if sample.ts > it.StartTs && sample.ts < it.EndTs {
				boundaries = append(boundaries, sample.ts)
			}
		}
		boundaries = append(boundaries, it.EndTs)
		for i := 0; i+1 < len(boundaries); i++ {
			s0, s1 := boundaries[i], boundaries[i+1]
			if s1 <= s0 {
				continue
			}
			sliceMs := it.DurationMs * (s1 - s0) / wall
			freq := governedFrequencyAt(samples, (s0+s1)/2)
			idealMs += supplyFoldSliceIdeal(sliceMs, freq, bigFmax, &basis)
		}
	}
	return idealMs, basis
}

// supplyFoldSliceIdeal books one slice into the basis split and returns its
// folded ideal contribution: known slices fold at min(1, f/fmax) — the clamp
// keeps a slice governed ABOVE the big-cluster fmax (possible under explicit
// topology) from minting a negative deficit — unknown slices fold at 1.
func supplyFoldSliceIdeal(sliceMs float64, freqKHz, bigFmaxKHz int, basis *SupplyFoldBasis) float64 {
	if freqKHz <= 0 {
		basis.UnknownMs += sliceMs
		return sliceMs
	}
	basis.KnownMs += sliceMs
	ratio := float64(freqKHz) / float64(bigFmaxKHz)
	if ratio > 1 {
		ratio = 1
	}
	return sliceMs * ratio
}
