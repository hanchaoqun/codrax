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

	// CapabilitySplitAudit (CAP-3 复核 P2, §29.11): when CapabilitySource is
	// freq_only via a derived-lane FRAGMENTATION arm (>4 clusters / fmax
	// tie), this localizes the first co-movement split behind the offending
	// cluster pair — "cpuA↔cpuB @ts 判定臂=token(zh)". DISCLOSURE/AUDIT ONLY,
	// never a gate: it lets a customer replay distinguish a carve-boundary
	// form (healed by CAP-3) from real mid-stream divergence (the honest
	// freq_only residual). Also lifted once per result into the engine
	// caveat lane (capabilitySplitAuditCaveat, query.go) so tracediag
	// reports carry it verbatim.
	CapabilitySplitAudit string `json:"capability_split_audit,omitempty"`

	// ReferenceClass (复核 F1, 2026-07-08): the capability class of the fold's
	// reference cluster — the cluster BOTH FmaxKHz and the reference cap were
	// taken from (同簇同源; live resolver supplyFoldGlobalMaxBasis — the top
	// judged cluster, prime included). R5 (§29.88.3/§29.88.12) retired the
	// demoting resolver (supplyFoldCapabilityReference) and its word fork —
	// the basis is trace-global and never demoted; the field stays as the
	// audit record of the chosen class. Empty on the freq_only/legacy lane
	// and on all-unknown folds. Wording input only, no gate reads it.
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
	RailFamily   string                   `json:"rail_family,omitempty"`
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
	// ThermalCapWitnessed (CR-3 件⑥ F-10, 2026-07-12; CR-2 冷读 D5 witness:
	// 「受热限压至 1.53GHz」 with zero in-window thermal/limits event — the
	// cap was a pre-window carry-in governance value): true iff at least one
	// cpu_frequency_limits sample or thermal-rail sample with an IN-WINDOW
	// timestamp pressed this cluster. False = the press is real governance
	// but its cause is unwitnessed inside the window — the display words it
	// 运行于 X(限压原因未见证) instead of 受热限压至 X. Wording input only.
	ThermalCapWitnessed bool `json:"thermal_cap_witnessed,omitempty"`
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
//
// CAP-3 (§29.11 状态携入语义, per-side direction): the head-governing sample
// IS the carry-in arm — cpu_frequency is a state lane, so a window/slice with
// zero in-window change events reads the last change point BEFORE the window,
// never "missing data" (窗内无变频事件 ≠ 数据缺失; pinned by
// TestSupplyFoldCarryInStateSemantics against the arm silently reverting to
// in-window events only).
//
//	Carry-in kept   → the folded ideal uses the frequency the hardware
//	                  actually held (state truth); dropping it would book the
//	                  slice UNKNOWN and hide a real deficit behind the 计0
//	                  lower bound — conservative-looking but LOSSY (typed
//	                  zero-value = 有损信号, forbidden as a silent default).
//	True absence    → a CPU with NO change point anywhere up to the window
//	                  end stays UNKNOWN (empty return): carrying a value
//	                  backwards from a LATER sample would fabricate pre-first-
//	                  witness state — the missing-counts-0 lower-bound arm +
//	                  disclosure stand (absence never guesses).
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
	// R6 rule 4 (full_freq_curves.go): the limits rung of the fmax ladder is
	// a frequency-derived quantity — full-file basis when the single-pass
	// collection covered the whole file (order poisoning audited there over
	// the same superset; window governance still applies at query time via
	// governedWindowSamples). READ-ONLY shared map.
	if tls, ok := c.idx.fullFrequencyLimitTimelines(); ok {
		c.freqLimitByCPU = tls
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
		if c.frequencyLimitLaneUnsafe(cpu) {
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

// supplyFoldGlobalMaxBasis resolves the R5 conversion basis: the 全域最大核
// 最高频点 (§29.88.3 + §29.88.12 用户裁定 2026-07-14/15) — ONE basis for
// every running conversion (the former gated 「按下游消费核」 lane and the
// supply-fold 「按大核满频」 lane both fold against THIS pair now).
//
//	basis capacity = fmax(全域最大核, FULL-FILE) × cap(其类)
//
// Under a usable capability map the 最大核 is the TOP judged cluster
// (presentClassesByRankDesc — prime included: R5's 「最大核」 supersedes the
// §26 复核-F1 big-class nomination), and BOTH the fmax and the cap come from
// that ONE cluster (同簇同源 preserved — the Probe-A cross-cluster mixing
// stays impossible by construction). Under freq_only (structure unjudgeable)
// the basis is the global maximum frequency point over every real core's
// full-trace curve with cap 1 (pure frequency ratio, typed disclosure
// unchanged).
//
// The fmax VALUE walks the fold-lane rung order (limit > observed > rail),
// FULL-TRACE caliber per R6 规则4 (curves from full_freq_curves.go when the
// scan covered the file): a window-local basis systematically under-states
// fmax when the frequency history's head/tail falls outside the window
// (缺口系统性偏小 — the adjudicated disease).
//
// EVOLUTION RECORD (R5, 2026-07-15): supplyFoldCapabilityReference
// (window-governed basis + demotion walk) and supplyFoldBigClusterFmax
// (window-governed pickBigClusterCeiling arm) are RETIRED — the demotion
// forms (按小核/中核/超大核满频 wording family) have no producer anymore:
// the basis is trace-global and always resolvable when any judged cluster
// exists. CMP-10 discipline stands: only real cores with governance-timeline
// evidence participate (ghost topology entries carry no curves and no fmax).
// The throttling companion keeps its shape: basis from a limits rung sitting
// below the same cluster's full-trace observed maximum ⇒ policy/thermal
// capping disclosed.
func (c *chainQueryCache) supplyFoldGlobalMaxBasis(capability coreCapabilityMap) (supplyFoldFmax, float64, string) {
	c.buildFreqIndex()
	c.buildFreqLimitIndex()
	ladder := func(members []int, railTL []freqSample) supplyFoldFmax {
		observed, limit := 0, 0
		for _, cpu := range members {
			for _, sample := range c.freqByCPU[cpu] {
				if sample.khz > observed {
					observed = sample.khz
				}
			}
			for _, sample := range c.freqLimitByCPU[cpu] {
				if sample.khz > limit {
					limit = sample.khz
				}
			}
		}
		railMax := 0
		for _, sample := range railTL {
			if sample.khz > railMax {
				railMax = sample.khz
			}
		}
		out := supplyFoldFmax{khz: railMax, source: SupplyFoldFmaxSourceRail, traceObservedMaxKHz: observed}
		if observed > 0 {
			out.khz, out.source = observed, SupplyFoldFmaxSourceObserved
		}
		if limit > 0 {
			out.khz, out.source = limit, SupplyFoldFmaxSourceLimit
			out.throttled = observed > limit
		}
		return out
	}
	if capability.usable() {
		for _, class := range capability.presentClassesByRankDesc() {
			label, ok := capability.classClusterLabel(class)
			if !ok {
				continue
			}
			fm := ladder(capability.domains.members[label], capability.railByCluster[label])
			if fm.khz <= 0 {
				// Structurally unreachable (a judged cluster carries a class-
				// ordering fmax by construction) — defensive walk-down.
				continue
			}
			return fm, coreCapabilityDefaultByClass[class], class
		}
		return supplyFoldFmax{}, 0, ""
	}
	// freq_only: no class identity — the 全域最高频点 over every real core.
	cpus := map[int]bool{}
	for cpu := range c.freqByCPU {
		cpus[cpu] = true
	}
	for cpu := range c.freqLimitByCPU {
		cpus[cpu] = true
	}
	members := make([]int, 0, len(cpus))
	for cpu := range cpus {
		members = append(members, cpu)
	}
	sort.Ints(members)
	fm := ladder(members, nil)
	if fm.khz <= 0 {
		return supplyFoldFmax{}, 0, ""
	}
	return fm, 1, ""
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
	// R5 (§29.88.3/§29.88.12 单基准单算法): the basis is the 全域最大核最高
	// 频点 pair — trace-global, never window-governed, never demoted; 同簇同源
	// holds by construction (both fmax and cap from the one top cluster).
	fm, refCap, refClass := c.supplyFoldGlobalMaxBasis(capability)
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
	// CAP-3 复核 P2: the fragmentation split localization rides the freq_only
	// basis (disclosure only — see the field doc; empty on every other form).
	if capability.source == CoreCapabilitySourceFreqOnly {
		basis.CapabilitySplitAudit = capability.freqOnlySplitAudit
	}
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
		if c.frequencyLaneUnsafe(it.CPU) {
			// A broken physical sample lane is UNKNOWN for this CPU. Do not let
			// cluster donor reuse or the lower rail fallback turn the fail-close
			// into a synthetic known-frequency slice.
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
				if c.frequencyLaneUnsafe(sibling) {
					return false
				}
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
	witnessed := false
	press := func(sample freqSample) {
		if sample.khz <= 0 {
			return
		}
		if cap == 0 || sample.khz < cap {
			cap = sample.khz
		}
		// CR-3 件⑥ F-10 (2026-07-12): governedWindowSamples re-timestamps
		// the pre-window carry-in head to exactly gStart; a strictly-later
		// timestamp is therefore a REAL in-window limits/thermal event — the
		// typed witness the 受热限压 wording requires (冷读 D5: a carry-in
		// cap wore the thermal word with zero in-window event).
		// 修复轮 P4 (2026-07-12): the witness must itself PRESS — an
		// in-window sample at (or above) the cluster fmax is a release/
		// no-op event and never earns the thermal word for a carry-in cap.
		if sample.ts > gStart && sample.khz < fmax {
			witnessed = true
		}
	}
	c.buildFreqLimitIndex()
	for _, cpu := range capability.domains.members[label] {
		for _, sample := range governedWindowSamples(c.freqLimitByCPU[cpu], gStart, gEnd) {
			press(sample)
		}
	}
	for _, rail := range c.thermalRailTimelines() {
		if thermalRailClusterLabel(rail, capability) != label {
			continue
		}
		for _, sample := range governedWindowSamples(rail.samples, gStart, gEnd) {
			press(sample)
		}
	}
	if cap > 0 && cap < fmax {
		basis.ThermalCapKHz = cap
		basis.ThermalCapClusterClass = capability.classByCluster[label]
		basis.ThermalCapWitnessed = witnessed
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
