package tracequery

// supply_fold.go — VS-2 (§7.10, docs/design/customer_dead_session_audit_
// 20260703.md): supply-fold accounting for on-chain RUNNING-dominant wakeup-
// chain nodes. Each running slice is folded from its own CPU's governed
// EQUIVALENT CAPACITY (frequency × core-class capability, CAP §26
// docs/design/real_trace_campaign_20260705.md) to the big cluster's:
//
//	ideal = Σ slice_ms × min(1, (f_slice × cap(slice class)) /
//	                            (f_bigmax × cap(big-cluster class)))
//	SupplyFoldDeficit = RunningMs − ideal   (clamped ≥ 0)
//
// so the deficit answers "how much of this thread's running wall clock is
// running-SLOW (weaker core / governed-down frequency), not running-MUCH".
// CAP (§26 user ruling 2026-07-08). EVOLUTION RECORD: the pre-CAP fold was the
// pure frequency ratio min(1, f_slice/f_bigmax) — a small core at its own full
// frequency read as "no deficit". Capability coefficients (core_capability.go,
// default table 小1.0/中2.3/大2.53/超大3.036) now price the class gap; the
// judged-cluster requirement fails loud back to the pure ratio with the typed
// freq_only disclosure. The wording contract downstream evolved with it: the
// deficit's 下界 residue is the missing-frequency slices counted 0 — the class
// advantage itself is now IN the fold (default or, once wired, evidence).
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

	// CFR (#75 簇共频, 客户硬件域裁定): provenance of every slice-CPU whose
	// fold frequency was REUSED from a same-cluster sampled sibling
	// (cluster_freq_share.go is the single resolution authority). Typed
	// disclosure only — the reused slices book as KNOWN basis because the
	// cluster shares one hardware frequency point, and this roster is what
	// makes that claim auditable. Empty when no reuse happened
	// (unresolvable membership, cross-cluster, or own samples present).
	// ClusterFreqReuseSource (CFR-2 #80) says where the membership came
	// from: ClusterFreqSourceExplicit (explicit core_topology) or
	// ClusterFreqSourceDerived (frequency change-point derivation). Set iff
	// the roster is non-empty.
	ClusterFreqReuse       []SupplyFoldClusterReuse `json:"cluster_freq_reuse,omitempty"`
	ClusterFreqReuseSource string                   `json:"cluster_freq_reuse_source,omitempty"`

	// CapabilitySource (CAP §26 C3): typed three-state disclosure of how the
	// fold priced core-class capability — CoreCapabilitySourceDefault (§26
	// default ratio table, display renders "按默认算力比粗算"),
	// CoreCapabilitySourceEvidence (reserved: vendor capacity evidence), or
	// CoreCapabilitySourceFreqOnly (cluster structure unjudgeable — fail-loud
	// pure-frequency fallback, display renders "簇结构不可判,按纯频率比折算").
	// Set whenever the fold runs; wording input only, no gate reads it.
	CapabilitySource string `json:"capability_source,omitempty"`

	// ReferenceClass (复核 F1, 2026-07-08): the capability class of the fold's
	// reference cluster — the cluster BOTH FmaxKHz and the reference cap were
	// taken from (同簇同源, supplyFoldCapabilityReference). Normally the
	// §26-nominated big class; a demotion (nominated cluster without
	// window-governed fmax) records the actually chosen class (small/middle/
	// prime) so the display stops claiming 按大核满频 for a non-big basis.
	// Empty on the freq_only/legacy lane and on all-unknown folds. Wording
	// input only, no gate reads it.
	ReferenceClass string `json:"reference_class,omitempty"`

	// ClusterTopologySource (CAP-2 §28.4/§28.5 三级披露词): WHERE the fold's
	// cluster STRUCTURE came from — CoreCapabilityTopologyComovement (Tier-1,
	// display 按实测频点共动分簇折算) or CoreCapabilityTopologyKeyedRail
	// (Tier-2 structure used, display 按簇轨实测折算(成员按锚点连续推定)).
	// Empty for explicit-topology / legacy / freq_only records so every
	// pre-CAP-2 wire byte stays identical. Wording input only.
	ClusterTopologySource string `json:"cluster_topology_source,omitempty"`
	// RailFamily / RailGoverned (CAP-2 audit disclosure, §28.5-T6 残洞兜底):
	// the adopted keyed-rail family mask and the roster of slice CPUs whose
	// governance came from a rail timeline — the traceback that keeps the
	// anchor-presumption fold auditable. Empty when no rail slice folded.
	RailFamily   string                  `json:"rail_family,omitempty"`
	RailGoverned []SupplyFoldRailGoverned `json:"rail_governed,omitempty"`

	// ThermalCapKHz / ThermalCapClusterClass (THERM, §28.5-T7, disclosure-only
	// zero-weight edit): the dominant running cluster of this fold was pressed
	// below its fmax inside the governance window — by a governing
	// cpu_frequency_limits Max and/or a thermal-named per-cluster rail — down
	// to ThermalCapKHz. No fold number changes; the display appends the
	// 窗内该簇受热限压至 X sentence. Zero when no press or when the cluster
	// attribution is unavailable (absence never guesses).
	ThermalCapKHz          int    `json:"thermal_cap_khz,omitempty"`
	ThermalCapClusterClass string `json:"thermal_cap_cluster_class,omitempty"`
}

// SupplyFoldRailGoverned is one disclosed rail-governed slice CPU: slices on
// CPU folded with the named keyed rail's governance timeline (CAP-2 Tier-2).
type SupplyFoldRailGoverned struct {
	CPU  int    `json:"cpu"`
	Rail string `json:"rail"`
}

// SupplyFoldClusterReuse is one disclosed reuse pair: slices on CPU folded
// with DonorCPU's governed frequency timeline (same explicit cluster).
type SupplyFoldClusterReuse struct {
	CPU      int `json:"cpu"`
	DonorCPU int `json:"donor_cpu"`
}

// Typed FmaxSource values (VS-2b ladder steps 2 and 3; step 1 — sysfs — is
// unreachable offline by definition). CAP-2 (§28.4) appends the keyed-rail
// rung: a cluster with neither a governing limits row nor an observed sample
// may anchor the basis with its validated rail timeline's governed maximum —
// the most inferential rung, so it ranks LAST (limit > observed > rail).
const (
	SupplyFoldFmaxSourceLimit    = "limit"
	SupplyFoldFmaxSourceObserved = "observed"
	SupplyFoldFmaxSourceRail     = "rail"
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
		// CAP-2 任务6 (§28.5-T8 pid_freq 量纲陷阱, 噪音源头消除): a sample
		// implausible as a CPU frequency under EVERY unit hypothesis is
		// telemetry noise, not a frequency — drop it here so the soft
		// corroboration lane stops carrying it (witness: pid_freq
		// state=10240923 — 10.24GHz/10.24MHz/10.24kHz under ÷1/÷1e3/÷1e6,
		// 10.24THz under the ×1e3 MHz hypothesis — all outside the band).
		// A filter INSIDE the soft lane only: the §7.10 role of this lane
		// (corroboration caveat, never the fold basis) is untouched, and
		// unit-ambiguous-but-plausible shapes (the pinned MHz hedge) survive.
		if !clockLanePlausibleCPUFrequency(ev.Frequency) {
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

// clockLanePlausibilityFactors are the unit hypotheses of the soft-lane
// plausibility filter: raw-as-kHz (÷1), Hz (÷1e3), ×1e6-scaled (÷1e6), and
// the MHz shape (×1e3 — the corroboration comparison cannot divide its way to
// it, but the pinned unit-unknown hedge for e.g. raw 2200 vs fmax 2200000
// requires the sample to SURVIVE this filter). A value plausible under ANY
// hypothesis is kept raw for the comparison-time matching.
var clockLanePlausibilityFactors = [...]float64{1e-3, 1, 1e3, 1e6}

// clockLanePlausibleCPUFrequency reports whether raw can denote a CPU
// frequency inside [clusterRailFreqMinKHz, clusterRailFreqMaxKHz] under at
// least one unit hypothesis.
func clockLanePlausibleCPUFrequency(raw int) bool {
	for _, div := range clockLanePlausibilityFactors {
		norm := float64(raw) / div
		if norm >= clusterRailFreqMinKHz && norm <= clusterRailFreqMaxKHz {
			return true
		}
	}
	return false
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

// supplyFoldCapabilityReference (复核 F1, 2026-07-08) resolves the fold's
// SAME-CLUSTER basis pair under a usable capability map: the reference
// cluster is nominated by class (§26 letter: coreCapabilityReferenceClass =
// 大核; single-constant switch point), and BOTH the fmax and the cap come
// from that ONE cluster — mixing the legacy pickBigClusterCeiling cluster's
// fmax with the capability ladder's top cap fabricated deficits (Probe A:
// prime cluster ungoverned in window → big-at-fmax slice minted 1.650ms;
// Probe B: small-only governance window → 5.987ms plus a false 按大核满频
// verdict).
//
// Demotion: when the nominated cluster has NO window-governed observed fmax,
// the reference moves to the highest-capability-class cluster that HAS one —
// fmax and cap move TOGETHER (宁 freq_only 勿拼积; the demoted class is
// disclosed via SupplyFoldBasis.ReferenceClass so the display wording follows
// the actual basis). No cluster governed at all → zero (the fold books
// everything unknown, exactly the legacy path).
//
// The fmax VALUE walks the same VS-2b ladder as the legacy resolver, scoped
// to the chosen cluster's members: window-governing cpu_frequency_limits Max
// (policy authority) beats the highest window-governing observed sample; the
// throttling finding compares against the SAME cluster's full-trace observed
// maximum. Membership rule unchanged: only observed cpu_frequency samples
// nominate (a limits-only cluster never mints a basis).
func (c *chainQueryCache) supplyFoldCapabilityReference(capability coreCapabilityMap, gStart, gEnd float64) (supplyFoldFmax, float64, string) {
	c.buildFreqIndex()
	classes := []string{coreCapabilityReferenceClass}
	for _, class := range capability.presentClassesByRankDesc() {
		if class != coreCapabilityReferenceClass {
			classes = append(classes, class)
		}
	}
	for _, class := range classes {
		label, ok := capability.classClusterLabel(class)
		if !ok {
			continue
		}
		members := capability.domains.members[label]
		if len(members) == 0 {
			continue
		}
		observed, limit, traceMax := 0, 0, 0
		for _, cpu := range members {
			if fmax := governedMaxKHz(c.freqByCPU[cpu], gStart, gEnd); fmax > observed {
				observed = fmax
			}
			if l := c.governedLimitMaxKHz(cpu, gStart, gEnd); l > limit {
				limit = l
			}
			for _, sample := range c.freqByCPU[cpu] {
				if sample.khz > traceMax {
					traceMax = sample.khz
				}
			}
		}
		// CAP-2 (§28.4): a validated keyed-rail governance timeline is the
		// LAST fmax rung and a nomination source of its own — the pure Tier-2
		// form has no observed samples at all, yet the cluster genuinely
		// carries a governed ceiling. Nomination stays "governance evidence
		// required": limits-only clusters still never mint a basis (pinned
		// membership rule).
		railGoverned := 0
		if tl := capability.railByCluster[label]; len(tl) > 0 {
			railGoverned = governedMaxKHz(tl, gStart, gEnd)
		}
		if observed <= 0 && railGoverned <= 0 {
			continue // this cluster cannot anchor the window's basis — demote
		}
		out := supplyFoldFmax{khz: railGoverned, source: SupplyFoldFmaxSourceRail, traceObservedMaxKHz: traceMax}
		if observed > 0 {
			out.khz, out.source = observed, SupplyFoldFmaxSourceObserved
		}
		if limit > 0 {
			out.khz, out.source = limit, SupplyFoldFmaxSourceLimit
			out.throttled = traceMax > limit
		}
		return out, coreCapabilityDefaultByClass[class], class
	}
	return supplyFoldFmax{}, 0, ""
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
	// CFR (#75 簇共频) + CFR-2 (#80 变化点推导): frequency domains for
	// same-cluster sample reuse (single authority: cluster_freq_share.go).
	// Explicit topology wins outright; in its ABSENCE the domains derive from
	// the FULL-trace per-CPU timelines (identity over complete sequences —
	// robust against governance-window truncation). No resolvable membership
	// at all fails open and the fold behaves as before.
	//
	// CAP (§26): the SAME resolution also carries the core-class capability
	// judgment (memoized on the cache); the fold discloses its pricing
	// caliber on the typed basis flag.
	capability := c.coreCapability(q.CoreTopology)
	domains := capability.domains
	// 复核 F1 (同簇同源): under a usable capability map the basis pair
	// (fmax, cap) resolves from ONE cluster — nominated big class, honest
	// demotion, class disclosed. The freq_only/legacy lane keeps the
	// pre-CAP pickBigClusterCeiling basis byte-identically with cap 1.
	var fm supplyFoldFmax
	refCap := 1.0
	refClass := ""
	if capability.usable() {
		fm, refCap, refClass = c.supplyFoldCapabilityReference(capability, gStart, gEnd)
	} else {
		fm = c.supplyFoldBigClusterFmax(q, gStart, gEnd)
	}
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
	basis.CapabilitySource = capability.source
	// CAP-2 (§28.4/§28.5): the typed topology-source token rides the basis
	// only on the two evidence forms (explicit/legacy stay token-less —
	// byte-identical wire). THERM (§28.5-T7) appends the disclosure-only
	// in-window thermal/policy press on the dominant running cluster.
	if capability.usable() {
		basis.ClusterTopologySource = capability.topologySource
	}
	c.applyThermalCapDisclosure(&basis, capability, gStart, gEnd, intervals)
	if bigFmax > 0 {
		basis.ReferenceClass = refClass
	}
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
		if len(samples) == 0 && domains.known() {
			// CFR (#75): the slice CPU has no governing samples of its own —
			// reuse a same-cluster sampled sibling's governed timeline (the
			// cluster shares one hardware frequency point, so this is truth,
			// not estimation). donorFor structurally refuses when the CPU has
			// its own samples (禁反向) and fails open on unknown membership.
			hasGoverned := func(sibling int) bool {
				return len(c.governedFreqSamples(sibling, gStart, gEnd)) > 0
			}
			if donor, ok := domains.donorFor(it.CPU, hasGoverned); ok {
				samples = c.governedFreqSamples(donor, gStart, gEnd)
				recordSupplyFoldClusterReuse(&basis, it.CPU, donor)
				// CFR-2 披露区分: say WHERE the membership came from.
				basis.ClusterFreqReuseSource = domains.source
			}
		}
		if len(samples) == 0 {
			// CAP-2 Tier-2 (§28.5 轨值时间线=该簇频率治理时间线): no own
			// samples and no donor — the slice CPU's validated keyed-rail
			// timeline governs it. Booked as KNOWN basis with the typed
			// rail-governed roster + family disclosure (the anchor
			// presumption wording carries the residual assumption).
			if label := capability.clusterLabelFor(it.CPU); label != "" {
				if tl := capability.railByCluster[label]; len(tl) > 0 {
					if railSamples := governedWindowSamples(tl, gStart, gEnd); len(railSamples) > 0 {
						samples = railSamples
						recordSupplyFoldRailGoverned(&basis, it.CPU, capability.railNameByCluster[label])
						basis.RailFamily = capability.railFamily
					}
				}
			}
		}
		// CAP (§26) + 复核 F1: the interval's CPU prices at its cluster class
		// relative to the CHOSEN reference cluster's cap (same-cluster pair —
		// never a second cluster judgment; 1 under freq_only / unknown
		// membership, the pre-CAP pure-frequency value — see sliceCapRatio).
		capRatio := capability.sliceCapRatio(it.CPU, refCap)
		wall := it.EndTs - it.StartTs
		if wall <= 0 {
			// Degenerate interval: single lookup at its start.
			freq := governedFrequencyAt(samples, it.StartTs)
			idealMs += supplyFoldSliceIdeal(it.DurationMs, freq, bigFmax, capRatio, &basis)
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
			idealMs += supplyFoldSliceIdeal(sliceMs, freq, bigFmax, capRatio, &basis)
		}
	}
	return idealMs, basis
}

// supplyFoldSliceIdeal books one slice into the basis split and returns its
// folded ideal contribution: known slices fold at
// min(1, (f/fmax) × capRatio) — capRatio is the CAP §26 class multiplier
// cap(实际核类)/cap(大核簇) (1 under freq_only / unknown membership), and the
// clamp keeps a slice governed ABOVE the big cluster's equivalent capacity
// (possible under explicit topology) from minting a negative deficit —
// unknown slices fold at 1.
// recordSupplyFoldClusterReuse appends one disclosed reuse pair, deduped and
// kept sorted by CPU (deterministic disclosure roster; a node's slices on the
// same CPU always resolve the same donor — lowest sampled sibling).
func recordSupplyFoldClusterReuse(basis *SupplyFoldBasis, cpu, donor int) {
	for _, pair := range basis.ClusterFreqReuse {
		if pair.CPU == cpu && pair.DonorCPU == donor {
			return
		}
	}
	basis.ClusterFreqReuse = append(basis.ClusterFreqReuse, SupplyFoldClusterReuse{CPU: cpu, DonorCPU: donor})
	sort.Slice(basis.ClusterFreqReuse, func(i, j int) bool {
		if basis.ClusterFreqReuse[i].CPU != basis.ClusterFreqReuse[j].CPU {
			return basis.ClusterFreqReuse[i].CPU < basis.ClusterFreqReuse[j].CPU
		}
		return basis.ClusterFreqReuse[i].DonorCPU < basis.ClusterFreqReuse[j].DonorCPU
	})
}

// recordSupplyFoldRailGoverned appends one disclosed rail-governed slice CPU,
// deduped and sorted (deterministic audit roster, mirror of the CFR recorder).
func recordSupplyFoldRailGoverned(basis *SupplyFoldBasis, cpu int, rail string) {
	for _, entry := range basis.RailGoverned {
		if entry.CPU == cpu && entry.Rail == rail {
			return
		}
	}
	basis.RailGoverned = append(basis.RailGoverned, SupplyFoldRailGoverned{CPU: cpu, Rail: rail})
	sort.Slice(basis.RailGoverned, func(i, j int) bool {
		if basis.RailGoverned[i].CPU != basis.RailGoverned[j].CPU {
			return basis.RailGoverned[i].CPU < basis.RailGoverned[j].CPU
		}
		return basis.RailGoverned[i].Rail < basis.RailGoverned[j].Rail
	})
}

// applyThermalCapDisclosure (THERM, §28.5-T7) computes the disclosure-only
// in-window thermal/policy press for the fold's dominant running cluster:
//
//	cap = min over the governance window of (a) the members' governing
//	      cpu_frequency_limits Max samples and (b) the values of thermal-named
//	      per-cluster rails whose anchors all sit inside this cluster;
//	press exists ⇔ cap < the cluster's ladder fmax (precise int comparison).
//
// Zero-weight edit: nothing numeric changes — the display appends the
// 窗内该簇受热限压至 X sentence off the typed field. The dominant cluster is
// the wall-clock argmax over the node's own RUNNING slices (deterministic
// label tie-break); an unattributable cluster emits nothing (归属不可得时
// 不发, absence never guesses).
func (c *chainQueryCache) applyThermalCapDisclosure(basis *SupplyFoldBasis, capability coreCapabilityMap, gStart, gEnd float64, intervals []Interval) {
	if !capability.usable() {
		return
	}
	wallByLabel := map[string]float64{}
	for _, it := range intervals {
		if it.State != StateRunning || !it.CPUKnown || it.DurationMs <= 0 {
			continue
		}
		if label := capability.clusterLabelFor(it.CPU); label != "" {
			wallByLabel[label] += it.DurationMs
		}
	}
	label := ""
	best := 0.0
	for l, wall := range wallByLabel {
		if wall > best || (wall == best && (label == "" || l < label)) {
			label, best = l, wall
		}
	}
	if label == "" {
		return
	}
	fmax := capability.fmaxByCluster[label]
	if fmax <= 0 {
		return
	}
	cap := 0
	press := func(khz int) {
		if khz > 0 && (cap == 0 || khz < cap) {
			cap = khz
		}
	}
	c.buildFreqLimitIndex()
	for _, cpu := range capability.domains.members[label] {
		for _, sample := range governedWindowSamples(c.freqLimitByCPU[cpu], gStart, gEnd) {
			press(sample.khz)
		}
	}
	for _, rail := range c.thermalRailTimelines() {
		if thermalRailClusterLabel(rail, capability) != label {
			continue
		}
		for _, sample := range governedWindowSamples(rail.samples, gStart, gEnd) {
			press(sample.khz)
		}
	}
	if cap > 0 && cap < fmax {
		basis.ThermalCapKHz = cap
		basis.ThermalCapClusterClass = capability.classByCluster[label]
	}
}

func supplyFoldSliceIdeal(sliceMs float64, freqKHz, bigFmaxKHz int, capRatio float64, basis *SupplyFoldBasis) float64 {
	if freqKHz <= 0 {
		basis.UnknownMs += sliceMs
		return sliceMs
	}
	basis.KnownMs += sliceMs
	ratio := float64(freqKHz) / float64(bigFmaxKHz)
	if capRatio > 0 {
		ratio *= capRatio
	}
	if ratio > 1 {
		ratio = 1
	}
	return sliceMs * ratio
}
