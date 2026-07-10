package tracequery

// cluster_rail_evidence.go — CAP-2 (§28.4 用户裁定 2026-07-09 + §28.5 两级证据
// 梯修订, docs/design/real_trace_campaign_20260705.md): Tier-2 keyed cluster-
// rail evidence. Vendor traces (donghu-class platform, cust_trace_cpu.txt /
// cust_trace_texture_upload.txt witnesses) carry per-cluster clock_set_rate
// rails whose payload cpu_id is a CLUSTER ANCHOR key, not the emitting CPU:
//
//	clock_set_rate: m3_c0_freq state=417000  cpu_id=0
//	clock_set_rate: m3_c1_freq state=417000  cpu_id=2
//	clock_set_rate: m3_c2_freq state=1200000 cpu_id=10
//	clock_set_rate: m3_c3_freq state=2350000 cpu_id=12
//
// §7.10 VS-2c 终局裁定 stands UNCHANGED for names: a clock lane NAME alone is
// vendor free vocabulary and may never bind a cluster or feed an fmax basis
// (isCPUFrequencyClockName stays soft-guidance-only; the structural pin keeps
// its original arm). CAP-2 is the ruled EXCEPTION ARM (§28.4 演化非推翻):
// binding semantics come ENTIRELY from the cpu_id KEYED FIELD plus six
// deterministic structural gates — ALL must pass or the whole family falls
// back (不部分猜测, the fold then keeps its previous behavior):
//
//	① 族形门   ≥2 rails sharing a one-digit-run name mask (prefix+index+
//	           suffix). The NAME is used ONLY to group the family — it
//	           carries no binding semantics (§7.10 preserved).
//	② 异锚门   every family member has cpu_id-KEYED samples (a keyless
//	           positional shape — hmtrace "cpu-cluster.N 2200000.0" — fails
//	           right here: the emitting-CPU fallback is NOT a key) and the
//	           member anchors are pairwise DISTINCT. Witness for why bare
//	           anchors are insufficient alone: thermal_inte1 anchors on
//	           cpu_id=2, colliding with m3_c1_freq's anchor 2 across
//	           families — only the composite gate set disambiguates.
//	③ 不变式门 each rail's cpu_id is CONSTANT over the full trace. A
//	           wandering anchor proves the field is telemetry/emitting-CPU
//	           attribution, not a key (witnesses: heca_info sweeps every
//	           CPU; thermal_inte1 wanders 2/3/4/5/7 — §28.5-T6 the PRIMARY
//	           thermal-rail discriminator). Violation rejects the WHOLE
//	           family (fail-loud, 整体回退).
//	④ 量纲门   every sample value sits in the plausible CPU-frequency band
//	           [clusterRailFreqMinKHz, clusterRailFreqMaxKHz], identity
//	           unit (kHz). Tier-2 values feed a fold BASIS, so no unit
//	           hypothesis is tolerated here — a Hz/MHz-shaped family falls
//	           back to the default table rather than guess a divisor
//	           (witnesses killed: heca_info state=7/23303, m3_vote_delay
//	           state=305, gpufreq 167100000).
//	⑤ 相容门   the anchor set ⊆ the scheduler-observed CPU set (an anchor
//	           naming a CPU the trace never scheduled is not a CPU key).
//	⑥ 负向词汇筛 (§28.5-T6, exclusion-only): rail names containing
//	           thermal/ddr/gpu/vote/delay/info/load are excluded from
//	           candidacy BEFORE family formation. Worst case of this noisy
//	           signal is a fallback to the default table — it can exclude,
//	           never admit, so it may read the name (§7.10-compatible).
//	           Witness: a short window seeing only thermal_inte2 (constant
//	           anchor 10) + thermal_inte3 (constant anchor 12) passes ①-⑤
//	           and is killed here.
//
// Residual assumption, disclosed not guessed (§28.4): cluster MEMBERSHIP is
// the anchor-contiguity PRESUMPTION — anchors sorted ascending, cluster k =
// [anchor_k, anchor_{k+1}-1], the last runs to the highest scheduler-observed
// CPU. The display wording carries it verbatim (成员按锚点连续推定), separate
// from the measured-rail claim. The theoretical residue (a benign-named,
// constant-anchor pseudo family) is accepted with the audit-note rail-family
// disclosure (fold_rail_basis carries the family name for traceback).
//
// Flavor neutrality (§28.4 补充裁定): the gates are structural judgments over
// parsed fields — ftrace and hitrace flavors ride ONE wiring; no flavor gate
// exists. Cross-trace topology transplant is FORBIDDEN (absence never
// guesses): a trace without keyed rails falls back honestly even if a sibling
// trace of the same platform carried them.
//
// Evidence-ladder priority (§28.5): 显式拓扑 > Tier-1 (cpu_frequency
// co-movement, cluster_freq_share.go) > Tier-2 (this file) > 默认表. When both
// tiers are present the rail values are CROSS-VALIDATED against the anchor
// clusters' own cpu_frequency samples (any positive proximate contradiction
// discards Tier-2, keeping Tier-1 — fail-loud); a validated rail may then
// SUBDIVIDE Tier-1 clusters that co-movement could not distinguish
// (constant-equal-value clusters, e.g. the witness {10..13} → {10,11} +
// {12,13}); membership of the subdivided parts stays disclosed as the anchor
// presumption. The combination itself lives in core_capability.go
// (resolveCoreCapabilityEvidence) — this file only produces/validates the
// evidence, so cluster_freq_share.go stays the single membership authority
// for the donor-reuse lane.

import (
	"sort"
	"strconv"
	"strings"
)

// Plausible CPU-frequency band, identity unit kHz (gate ④ and the shared
// soft-lane plausibility filter's identity hypothesis). Bounds chosen to admit
// every real cpufreq operating point observed across the fleet (idle floors
// ~100-400MHz, mobile ceilings ≤3.5GHz, desktop ≤6GHz) while rejecting the
// witnessed non-frequency encodings by an order of magnitude on each side
// (heca_info 7/23303 kHz ≪ 50MHz; pid_freq 10240923 kHz = 10.24GHz ≫ 6GHz;
// gpufreq 167100000 Hz-shaped ≫ 6GHz as kHz).
const (
	clusterRailFreqMinKHz = 50000   // 50 MHz
	clusterRailFreqMaxKHz = 6000000 // 6 GHz
)

// clusterRailExcludedNameTokens is the ⑥ negative vocabulary filter
// (exclusion-only — it can only shrink the candidate set, worst case is the
// default-table fallback; it never admits anything, so reading the vendor
// name here stays inside the §7.10 name-role ruling).
// EVOLUTION RECORD (CAP-2 复核收尾 P3-2, 2026-07-09): "temp"/"tsens" joined
// the ruled seven-token set. The adversarial review demonstrated the §28.5-T6
// accepted residual hole is REACHABLE: a constant-anchor soc_temp1/soc_temp2
// pseudo family whose milli-°C values (e.g. 65000 = 65.000°C) land inside the
// kHz plausibility band clears gates ①-⑤. Exclusion-only growth is the
// §28.4-sanctioned evolution direction (worst case = default-table fallback,
// never a fabrication). "limit"/"cap"/"budget"-class words stay OUT for now
// (their value shapes rarely land in-band; under observation).
var clusterRailExcludedNameTokens = []string{"thermal", "ddr", "gpu", "vote", "delay", "info", "load", "temp", "tsens"}

// Cross-validation caliber (两级并存, §28.5): a rail sample convicts Tier-2
// only against PROXIMATE actual samples — within
// clusterRailCrossValProximitySec of the rail sample on any member CPU that
// has its own cpu_frequency data — and only when NO proximate sample agrees
// within clusterRailCrossValRelTol. DVFS propagation is sub-millisecond, so
// 10ms bounds governance-vs-actual lag generously; 10% mirrors the VS-2c
// corroboration tolerance. Absence never convicts: a rail sample with no
// proximate actual samples is vacuous (the witness fragment's 28ms gaps make
// no claim either way).
const (
	clusterRailCrossValProximitySec = 0.010
	clusterRailCrossValRelTol       = 0.10
)

// Typed rejection reason tokens (pin surface; never user-facing wording).
const (
	clusterRailRejectAnchorKeyMissing  = "anchor_key_missing"  // ② keyless member (hmtrace positional form)
	clusterRailRejectAnchorCollision   = "anchor_collision"    // ② duplicate anchors inside the family
	clusterRailRejectAnchorUnstable    = "anchor_unstable"     // ③ wandering cpu_id (heca_info / thermal_inte1 form)
	clusterRailRejectDimension         = "dimension"           // ④ value outside the CPU-frequency band
	clusterRailRejectAnchorOutsideCPUs = "anchor_outside_cpus" // ⑤ anchor not scheduler-observed
	clusterRailRejectAmbiguousFamilies = "ambiguous_families"  // >1 family passed all gates (不部分猜测)
	clusterRailRejectCrossValidation   = "cross_validation_mismatch"
	clusterRailRejectStructureConflict = "structure_conflict"
)

// clusterRailCluster is one adopted rail: the verbatim rail name, its constant
// anchor, the anchor-contiguity member presumption, the full-trace value
// timeline (the cluster's frequency GOVERNANCE timeline, §28.5-T5) and its
// full-trace maximum.
type clusterRailCluster struct {
	rail    string
	anchor  int
	members []int
	samples []freqSample
	fmaxKHz int
}

// clusterRailAdoption is one family that passed all six gates, clusters
// sorted by ascending anchor.
type clusterRailAdoption struct {
	family   string
	clusters []clusterRailCluster
}

// clusterRailScan is the memoized whole-trace scan result: at most one
// adopted family (ambiguity falls back — 不部分猜测), plus the per-family
// typed rejection reasons for the pin/audit surface.
type clusterRailScan struct {
	adoption *clusterRailAdoption
	rejected map[string]string
}

// railCandidate accumulates one verbatim rail name's evidence.
type railCandidate struct {
	name     string
	samples  []freqSample
	anchors  []int // per keyed sample, parallel to samples' key-valid subset
	keyless  int   // samples without a cpu_id key (② witness: positional form)
	firstKey int
	keyed    int
	wander   bool
}

// isClusterRailEvent is the rail-sample admission: a clock_set_rate row with
// a named lane and a positive value. PRECISE signals: the verbatim event name
// (reclassified cpu-freq-named lanes keep Name=="clock_set_rate" with
// Type=EventCPUFrequency; unreclassified lanes keep Type=EventClockSetRate).
func isClusterRailEvent(ev Event) bool {
	if ev.ClockName == "" || ev.Frequency <= 0 {
		return false
	}
	if ev.Type == EventClockSetRate {
		return true
	}
	return ev.Type == EventCPUFrequency && ev.Name == "clock_set_rate"
}

// clusterRailNameExcluded is gate ⑥ (exclusion-only).
func clusterRailNameExcluded(name string) bool {
	lower := strings.ToLower(name)
	for _, token := range clusterRailExcludedNameTokens {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

// railFamilyMasks returns the ① family keys of one rail name: for every
// maximal digit run, the name with that run replaced by "#". A name with no
// digit run has no mask and can never join a family (heca_info, pid_freq,
// cpu_freq are structurally family-less).
func railFamilyMasks(name string) []string {
	var masks []string
	for i := 0; i < len(name); {
		if name[i] >= '0' && name[i] <= '9' {
			j := i
			for j < len(name) && name[j] >= '0' && name[j] <= '9' {
				j++
			}
			masks = append(masks, name[:i]+"#"+name[j:])
			i = j
		} else {
			i++
		}
	}
	return masks
}

// clusterRailInvariantGateOK is gate ③ over one candidate: every sample must
// be cpu_id-keyed and every key must equal the first (constant anchor). Kept
// as a separate function so the heca_info witness pins the gate directly.
func clusterRailInvariantGateOK(cand *railCandidate) bool {
	return cand != nil && cand.keyed > 0 && cand.keyless == 0 && !cand.wander
}

// clusterRailDimensionGateOK is gate ④ over one candidate: identity-unit kHz
// band on EVERY sample (one out-of-band value rejects the family — 不部分猜测).
func clusterRailDimensionGateOK(cand *railCandidate) bool {
	if cand == nil || len(cand.samples) == 0 {
		return false
	}
	for _, s := range cand.samples {
		if s.khz < clusterRailFreqMinKHz || s.khz > clusterRailFreqMaxKHz {
			return false
		}
	}
	return true
}

// collectClusterRailCandidates gathers per-rail evidence in one event pass.
func collectClusterRailCandidates(events []Event) map[string]*railCandidate {
	out := map[string]*railCandidate{}
	for _, ev := range events {
		if !isClusterRailEvent(ev) {
			continue
		}
		cand := out[ev.ClockName]
		if cand == nil {
			cand = &railCandidate{name: ev.ClockName}
			out[ev.ClockName] = cand
		}
		cand.samples = append(cand.samples, freqSample{ts: ev.Ts, khz: ev.Frequency})
		if !ev.CPUForFieldValid || !validTraceCPUIndex(ev.CPUForField) {
			cand.keyless++
			continue
		}
		if cand.keyed == 0 {
			cand.firstKey = ev.CPUForField
		} else if ev.CPUForField != cand.firstKey {
			cand.wander = true
		}
		cand.keyed++
		cand.anchors = append(cand.anchors, ev.CPUForField)
	}
	return out
}

// scanClusterRailEvidence runs gates ①-⑥ over the trace. schedCPUs is the
// scheduler-observed CPU set (gate ⑤ + the membership presumption's upper
// bound). Exactly ONE family may be adopted; two adoptable families are an
// ambiguity and both fall back (不部分猜测).
func scanClusterRailEvidence(events []Event, schedCPUs map[int]bool) clusterRailScan {
	scan := clusterRailScan{rejected: map[string]string{}}
	if len(schedCPUs) == 0 {
		return scan
	}
	maxCPU := 0
	for cpu := range schedCPUs {
		if cpu > maxCPU {
			maxCPU = cpu
		}
	}
	candidates := collectClusterRailCandidates(events)
	// ⑥ exclusion before family formation.
	families := map[string][]string{}
	for name := range candidates {
		if clusterRailNameExcluded(name) {
			continue
		}
		for _, mask := range railFamilyMasks(name) {
			families[mask] = append(families[mask], name)
		}
	}
	var adopted []*clusterRailAdoption
	maskKeys := make([]string, 0, len(families))
	for mask := range families {
		maskKeys = append(maskKeys, mask)
	}
	sort.Strings(maskKeys)
	for _, mask := range maskKeys {
		names := families[mask]
		if len(names) < 2 {
			continue // ① family gate: <2 members is not a family (no reason entry — never a candidate)
		}
		sort.Strings(names)
		reason := ""
		anchorSeen := map[int]string{}
		for _, name := range names {
			cand := candidates[name]
			// ③ before ② so a wandering anchor reports its own reason (the
			// per-member anchor is only defined once constancy holds).
			if !clusterRailInvariantGateOK(cand) {
				if cand.keyed == 0 || cand.keyless > 0 {
					reason = clusterRailRejectAnchorKeyMissing // ② keyed-field requirement
				}
				if cand.wander {
					reason = clusterRailRejectAnchorUnstable // ③
				}
				break
			}
			if prev, dup := anchorSeen[cand.firstKey]; dup && prev != name {
				reason = clusterRailRejectAnchorCollision // ②
				break
			}
			anchorSeen[cand.firstKey] = name
			if !clusterRailDimensionGateOK(cand) {
				reason = clusterRailRejectDimension // ④
				break
			}
			if !schedCPUs[cand.firstKey] {
				reason = clusterRailRejectAnchorOutsideCPUs // ⑤
				break
			}
		}
		if reason != "" {
			scan.rejected[mask] = reason
			continue
		}
		adoption := &clusterRailAdoption{family: mask}
		for _, name := range names {
			cand := candidates[name]
			sort.SliceStable(cand.samples, func(i, j int) bool { return cand.samples[i].ts < cand.samples[j].ts })
			fmax := 0
			for _, s := range cand.samples {
				if s.khz > fmax {
					fmax = s.khz
				}
			}
			adoption.clusters = append(adoption.clusters, clusterRailCluster{
				rail:    name,
				anchor:  cand.firstKey,
				samples: cand.samples,
				fmaxKHz: fmax,
			})
		}
		sort.SliceStable(adoption.clusters, func(i, j int) bool {
			return adoption.clusters[i].anchor < adoption.clusters[j].anchor
		})
		// Anchor-contiguity membership presumption (§28.4 残余假设): cluster k
		// runs from its anchor to the next anchor − 1; the last runs to the
		// highest scheduler-observed CPU. CPUs below the first anchor are NOT
		// claimed (the presumption only extends upward from an anchor).
		for i := range adoption.clusters {
			start := adoption.clusters[i].anchor
			end := maxCPU
			if i+1 < len(adoption.clusters) {
				end = adoption.clusters[i+1].anchor - 1
			}
			for cpu := start; cpu <= end; cpu++ {
				adoption.clusters[i].members = append(adoption.clusters[i].members, cpu)
			}
		}
		adopted = append(adopted, adoption)
	}
	switch len(adopted) {
	case 0:
	case 1:
		scan.adoption = adopted[0]
	default:
		// Two structurally-valid families with no way to choose: 不部分猜测.
		for _, a := range adopted {
			scan.rejected[a.family] = clusterRailRejectAmbiguousFamilies
		}
	}
	return scan
}

// clusterRailCrossValidate is the 两级并存 cross-validation (§28.5): every
// rail sample is compared against the PROXIMATE cpu_frequency samples of its
// cluster's own members. A rail sample with at least one proximate actual
// sample and NO agreeing one (within the relative tolerance) is a positive
// contradiction — Tier-2 is discarded wholesale (fail-loud, Tier-1 kept). No
// proximate samples anywhere = vacuous pass (absence never convicts; the
// membership presumption disclosure still applies).
func clusterRailCrossValidate(adoption *clusterRailAdoption, freqByCPU map[int][]freqSample) bool {
	if adoption == nil {
		return true
	}
	for _, cluster := range adoption.clusters {
		var memberTLs [][]freqSample
		for _, cpu := range cluster.members {
			if tl := freqByCPU[cpu]; len(tl) > 0 {
				memberTLs = append(memberTLs, tl)
			}
		}
		if len(memberTLs) == 0 {
			continue
		}
		for _, rs := range cluster.samples {
			had, matched := false, false
			for _, tl := range memberTLs {
				lo := sort.Search(len(tl), func(i int) bool { return tl[i].ts >= rs.ts-clusterRailCrossValProximitySec })
				for i := lo; i < len(tl) && tl[i].ts <= rs.ts+clusterRailCrossValProximitySec; i++ {
					had = true
					diff := float64(tl[i].khz - rs.khz)
					if diff < 0 {
						diff = -diff
					}
					if diff <= clusterRailCrossValRelTol*float64(rs.khz) {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if had && !matched {
				return false
			}
		}
	}
	return true
}

// railDomainLabel names the pure-Tier-2 synthetic frequency domains.
func railDomainLabel(i int) string { return "rail_c" + strconv.Itoa(i) }

// railOnlyDomains builds the pure-Tier-2 (no cpu_frequency at all) frequency
// domains: membership is the full anchor-contiguity presumption, source is
// the keyed-rail token. donorFor over these domains is structurally inert
// (no member ever has cpu_frequency samples), so the CFR donor-reuse lane is
// untouched by construction.
func railOnlyDomains(adoption *clusterRailAdoption) (clusterFreqDomains, map[string][]freqSample, map[string]string) {
	domains := clusterFreqDomains{
		byCPU:   map[int]string{},
		members: map[string][]int{},
		source:  ClusterFreqSourceKeyedRail,
	}
	railTL := map[string][]freqSample{}
	railName := map[string]string{}
	for i, cluster := range adoption.clusters {
		label := railDomainLabel(i)
		for _, cpu := range cluster.members {
			domains.byCPU[cpu] = label
			domains.members[label] = append(domains.members[label], cpu)
		}
		railTL[label] = cluster.samples
		railName[label] = cluster.rail
	}
	return domains, railTL, railName
}

// railRefinement is the Tier-1 × Tier-2 combination outcome (§28.5 细分).
type railRefinement struct {
	domains         clusterFreqDomains
	railTLByLabel   map[string][]freqSample
	railNameByLabel map[string]string
	structureUsed   bool // ≥1 Tier-1 cluster was subdivided by anchors
	ok              bool
	reason          string
}

// refineDomainsWithRails subdivides Tier-1 (derived) domains with the
// validated rail anchors. Rules (§28.5, priority Tier-1 > Tier-2):
//
//   - a rail range containing sampled members of TWO distinct Tier-1 domains
//     contradicts the measured co-movement split — the anchor presumption is
//     refuted, Tier-2 is discarded wholesale (structure_conflict);
//   - a Tier-1 domain whose sampled members span ≥2 rail ranges is SPLIT
//     along them ({10..13} → {10,11}+{12,13} — the constant-equal-value form
//     co-movement cannot distinguish); the split parts inherit the anchor
//     presumption disclosure;
//   - a Tier-1 domain with members below the first anchor AND members inside
//     ranges is mixed evidence — conflict (conservative); entirely-below
//     domains stay untouched (the presumption claims nothing there);
//   - rail ranges without any sampled member contribute no structure (their
//     CPUs keep the Tier-1 inheritance rules — membership presumption never
//     overrides measured-lane conventions when Tier-1 is primary).
//
// Unsampled CPUs keep the derived-lane 向下继承/向上不外推 rules: byCPU is
// only rewritten for sampled members, sampledAsc is preserved, so
// domainMembersFor/derivedDomainLabelFor keep working over the refined map.
func refineDomainsWithRails(domains clusterFreqDomains, adoption *clusterRailAdoption) railRefinement {
	out := railRefinement{domains: domains}
	if adoption == nil {
		out.ok = true
		return out
	}
	anchors := make([]int, len(adoption.clusters))
	for i, cluster := range adoption.clusters {
		anchors[i] = cluster.anchor
	}
	rangeOf := func(cpu int) int {
		idx := sort.SearchInts(anchors, cpu+1) - 1 // last anchor ≤ cpu
		return idx                                 // -1 when cpu < first anchor
	}
	// Conflict detection over sampled members.
	labelsByRange := map[int]map[string]bool{}
	rangesByLabel := map[string]map[int]bool{}
	for cpu, label := range domains.byCPU {
		r := rangeOf(cpu)
		if labelsByRange[r] == nil {
			labelsByRange[r] = map[string]bool{}
		}
		labelsByRange[r][label] = true
		if rangesByLabel[label] == nil {
			rangesByLabel[label] = map[int]bool{}
		}
		rangesByLabel[label][r] = true
	}
	for r, labels := range labelsByRange {
		if r >= 0 && len(labels) > 1 {
			out.reason = clusterRailRejectStructureConflict
			return out
		}
	}
	for _, ranges := range rangesByLabel {
		if ranges[-1] && len(ranges) > 1 {
			out.reason = clusterRailRejectStructureConflict
			return out
		}
	}
	// Rebuild membership with per-range sub-labels where a domain splits.
	refined := clusterFreqDomains{
		byCPU:                map[int]string{},
		members:              map[string][]int{},
		source:               domains.source,
		sampledAsc:           domains.sampledAsc,
		explicitInputIgnored: domains.explicitInputIgnored,
	}
	railTL := map[string][]freqSample{}
	railName := map[string]string{}
	labelKeys := make([]string, 0, len(domains.members))
	for label := range domains.members {
		labelKeys = append(labelKeys, label)
	}
	sort.Strings(labelKeys)
	for _, label := range labelKeys {
		ranges := rangesByLabel[label]
		split := len(ranges) > 1
		if split {
			out.structureUsed = true
		}
		for _, cpu := range domains.members[label] {
			r := rangeOf(cpu)
			sub := label
			if split {
				sub = label + "_r" + strconv.Itoa(r)
			}
			refined.byCPU[cpu] = sub
			refined.members[sub] = append(refined.members[sub], cpu)
			if r >= 0 {
				railTL[sub] = adoption.clusters[r].samples
				railName[sub] = adoption.clusters[r].rail
			}
		}
	}
	for label := range refined.members {
		sort.Ints(refined.members[label])
	}
	refined.groupCount = len(refined.members)
	out.domains = refined
	out.railTLByLabel = railTL
	out.railNameByLabel = railName
	out.ok = true
	return out
}

// --- chainQueryCache lanes (memoized once per query cache) ------------------

// schedObservedCPUs is the gate-⑤ universe: CPUs on which the scheduler
// demonstrably ran (sched_switch emitting CPU). Memoized.
func (c *chainQueryCache) schedObservedCPUs() map[int]bool {
	if c.schedCPUOnce {
		return c.schedCPUs
	}
	c.schedCPUOnce = true
	c.schedCPUs = map[int]bool{}
	if c.idx == nil {
		return c.schedCPUs
	}
	for _, ev := range c.idx.Events {
		if ev.Type == EventSchedSwitch {
			c.schedCPUs[ev.CPU] = true
		}
	}
	return c.schedCPUs
}

// clusterRailScanResult memoizes the six-gate scan (independent of the query
// topology parameter; cross-validation and combination happen per capability
// resolution in core_capability.go).
func (c *chainQueryCache) clusterRailScanResult() clusterRailScan {
	if c.railScanOnce {
		return c.railScan
	}
	c.railScanOnce = true
	if c.idx == nil {
		c.railScan = clusterRailScan{rejected: map[string]string{}}
		return c.railScan
	}
	c.railScan = scanClusterRailEvidence(c.idx.Events, c.schedObservedCPUs())
	return c.railScan
}

// --- THERM (§28.5-T7, disclosure-only) ---------------------------------------

// thermalRailTimeline is one thermal-family rail: name-excluded from the
// Tier-2 candidate set by gate ⑥, but itself a per-cluster THERMAL-CEILING
// governance timeline when its anchors stay inside one resolved cluster
// (witness: thermal_inte1 wanders 2/3/4/5/7 — all inside {2..9};
// thermal_inte2 anchors 10 ⊆ {10,11}; thermal_inte3 anchors 12 ⊆ {12,13}).
type thermalRailTimeline struct {
	name    string
	anchors []int
	samples []freqSample
}

// thermalRailTimelines collects thermal-named rails whose samples are keyed
// and dimension-plausible (identity kHz band). Memoized. Anchors may wander
// (that is the thermal family's shape) — cluster association happens at
// consumption time against the resolved membership.
func (c *chainQueryCache) thermalRailTimelines() []thermalRailTimeline {
	if c.thermalRailOnce {
		return c.thermalRails
	}
	c.thermalRailOnce = true
	if c.idx == nil {
		return nil
	}
	byName := map[string]*thermalRailTimeline{}
	valid := map[string]bool{}
	for _, ev := range c.idx.Events {
		if !isClusterRailEvent(ev) {
			continue
		}
		if !strings.Contains(strings.ToLower(ev.ClockName), "thermal") {
			continue
		}
		rail := byName[ev.ClockName]
		if rail == nil {
			rail = &thermalRailTimeline{name: ev.ClockName}
			byName[ev.ClockName] = rail
			valid[ev.ClockName] = true
		}
		if !ev.CPUForFieldValid || !validTraceCPUIndex(ev.CPUForField) || ev.Frequency < clusterRailFreqMinKHz || ev.Frequency > clusterRailFreqMaxKHz {
			// Unkeyed or non-frequency-shaped thermal telemetry: the whole
			// rail is dropped (absence never guesses a ceiling).
			valid[ev.ClockName] = false
			continue
		}
		rail.samples = append(rail.samples, freqSample{ts: ev.Ts, khz: ev.Frequency})
		rail.anchors = append(rail.anchors, ev.CPUForField)
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		if valid[name] && len(byName[name].samples) > 0 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		rail := byName[name]
		sort.SliceStable(rail.samples, func(i, j int) bool { return rail.samples[i].ts < rail.samples[j].ts })
		c.thermalRails = append(c.thermalRails, *rail)
	}
	return c.thermalRails
}

// thermalRailClusterLabel resolves which capability cluster a thermal rail
// speaks for: every anchor must land in ONE cluster's membership ("" when the
// anchors span clusters or hit unresolved CPUs — 归属不可得时不发).
func thermalRailClusterLabel(rail thermalRailTimeline, capability coreCapabilityMap) string {
	label := ""
	for _, anchor := range rail.anchors {
		l := capability.clusterLabelFor(anchor)
		if l == "" {
			return ""
		}
		if label == "" {
			label = l
		} else if l != label {
			return ""
		}
	}
	return label
}
