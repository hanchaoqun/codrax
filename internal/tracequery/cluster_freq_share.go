package tracequery

// cluster_freq_share.go — CFR (#75, 客户硬件域裁定 2026-07-04): cluster-shared
// frequency reuse. On the customer's devices every CPU CLUSTER shares one
// frequency point (小/中/大/超大核簇): when ANY core of a cluster carries
// cpu_frequency samples, the samples are hardware truth for every sibling of
// that cluster (donghu real-device witness: cpu_frequency lanes exist only for
// cpu3-5, yet the whole cluster runs at that frequency). This file is the
// SINGLE AUTHORITY for resolving "cpu → cluster → a sampled sibling whose
// frequency timeline may be reused". Every consumption face that wants the
// reuse MUST go through clusterFreqDomains.donorFor — no second cluster
// judgment may grow beside it.
//
// Gate discipline (precise signals only):
//
//   - Reuse activates under an EXPLICIT Query.CoreTopology declaration, or —
//     CFR-2 (#80, 用户裁定 2026-07-06) — under TRACE-NATIVE cluster
//     derivation when no explicit topology exists (deriveClusterFreqDomains
//     below: identical change-point timelines merge sampled cores into one
//     domain; unsampled cores inherit the domain of the nearest sampled core
//     with a HIGHER-OR-EQUAL core number; cores above the highest sampled
//     core are NEVER extrapolated into a sampled domain). The 3-class
//     frequency-tier INFERENCE (inferCoreTopologyFromFrequency) remains a
//     noisy signal and never arms the gate; the derivation reads exact value
//     sequences and a documented timestamp bound — precise signals.
//   - CAP-3 (§29.11, docs/design/real_trace_campaign_20260705.md, user ruling
//     2026-07-09): cpu_frequency is a STATE lane and the cluster topology is
//     a TRACE-GLOBAL attribute. (a) Every face derives over the SAME
//     Index-global sample stream (indexFreqSampleTimelines) — never a
//     query-window crop: cropping the derivation input moved the carve
//     boundary per window and made the SAME trace judge in one window and
//     fail in the next (huadong_792 两窗不可判 vs g12 窗判出, cap2_report
//     witness). (b) CLUSTERSTREAM-1 (§29.193/§29.193.1, 2026-07-21): the
//     co-movement criterion (freqTimelinesCoMove) is the streaming
//     incremental WITNESS accumulation — paired same-value transitions
//     within the fixed skew bound mint pro witnesses, paired different-value
//     transitions mint con witnesses (one-vote veto), entry announcements
//     mint neither — so a boundary/lost row costs one witness instead of the
//     whole merge, while real persistent divergence keeps minting con and
//     SPLITS (absence never guesses; see the deriveClusterFreqDomains
//     EVOLUTION RECORD for the retired trimmed whole-sequence family).
//   - Cluster identity is the VERBATIM (lowercased) topology label, NOT the
//     normalized 3-class display taxonomy. normalizeCoreClass folds
//     big/large/prime into one "big" class — correct for core-class display,
//     WRONG for frequency domains: "big=4-6;prime=7" declares two distinct
//     frequency domains (大核簇 vs 超大核簇) and cross-domain reuse would
//     fabricate hardware state. Only entries whose label the class taxonomy
//     recognizes participate, so the reuse gate can never be MORE permissive
//     than the explicit-topology gate itself.
//   - 禁反向 (structural): a core that has its own samples NEVER takes a
//     donor — donorFor refuses when hasSamples(cpu) is true, so the sampled
//     cores' behavior is byte-identical by construction, not by call-site
//     discipline.
//   - Donor choice is deterministic: the lowest-numbered same-domain sibling
//     that satisfies hasSamples. All sampled siblings observe the SAME
//     hardware signal, so any is truthful; lowest-id keeps the disclosure
//     stable and auditable.
//   - No per-SoC table anywhere (C-7 semantic ruling pin: 簇判定无硬编码 —
//     the hiview 12-core hardcode is the pinned anti-example).
//
// Consumers (audited 2026-07-06, CFR batch):
//   - VS-2 supply fold per-slice governance (supply_fold.go) — the b3
//     "频点数据不全,无法折算" shape; disclosure via
//     SupplyFoldBasis.ClusterFreqReuse + the fold_cluster_freq_reuse note.
//   - Window-face frequency-weighted lookups (ComputeWindowStats busy loop +
//     computeOffCPUStats) and the compute_supply per-CPU fmax
//     (computeComputeSupplyBalance) — disclosure via the per-CPU
//     FrequencyClusterDonorCPU field and window/ledger caveats.
//
// Deliberately NOT consumers (facts are never rewritten): cpu_frequency
// census cpu sets, CPUStats.FrequencyResidency / CPUStats.Frequency,
// ClusterFrequencyCeilings membership lists, topology inference input,
// supply_pressure LowFrequencyCPUs, R5d-2 weakCoreDeficitMs (its unknown
// slices contribute zero by documented lower-bound contract).

import (
	"sort"
	"strconv"
	"strings"
)

// Cluster-domain source tokens (typed disclosure lane — the wire/JSON faces
// carry them so every reuse says WHERE the membership came from).
const (
	// ClusterFreqSourceExplicit: membership from the explicit core_topology
	// parameter (CFR #75 lane).
	ClusterFreqSourceExplicit = "explicit_topology"
	// ClusterFreqSourceDerived: membership derived from identical
	// cpu_frequency change-point timelines + downward core-number
	// inheritance (CFR-2 #80 lane). CAP-2 (§28.5): this derivation IS the
	// evidence ladder's Tier-1 arm (实测频点共动分簇) — the capability lane
	// consumes it behind the clusterFreqComoveMinSamples floor below.
	ClusterFreqSourceDerived = "freq_change_point_derived"
	// ClusterFreqSourceKeyedRail (CAP-2 §28.4, 2026-07-09): membership from a
	// six-gate-validated cpu_id-KEYED cluster-rail family
	// (cluster_rail_evidence.go), anchors + contiguity presumption. Pure
	// Tier-2 form only — no member ever has cpu_frequency samples, so the
	// donor-reuse lane is structurally inert over these domains.
	ClusterFreqSourceKeyedRail = "keyed_rail"
)

// clusterFreqComoveMinSamples (CAP-2 Tier-1 样本数下限门, §28.5): a derived
// MULTI-CPU merge is admissible CAPABILITY-CLASS evidence only when the shared
// timeline carries at least this many samples. A single coincident sample is
// not co-movement — two distinct clusters idling at one equal value would
// merge on a coin flip and corrupt the cluster COUNT the §26 class mapping
// keys on; with ≥2 samples the identity criterion has witnessed the value
// vector at two instants (constant-equal-value merges remain the HONEST-merge
// form the ruling accepts). Scope: the capability lane ONLY — singleton
// domains make no co-movement claim, and the CFR-2 donor-reuse lane keeps its
// adjudicated #80 behavior untouched (reuse among identical timelines is
// truthful at any sample count).
const clusterFreqComoveMinSamples = 2

// clusterFreqDerivedPrimeLabel marks cores ABOVE the highest sampled core
// when the derivation already produced ≥3 domains (小/中/大 present → higher
// cores are the 超大核 cluster by the user ruling). The label is
// EXCLUSIONARY: the domain has no sampled member, so donorFor can never mint
// a donor for it — it pins "big-core samples must not leak upward". CFR-2
// verify P3-3: consulted prime cores are DISCLOSED on the derived caveat
// face (clusterFreqReuseCaveat prime clause) so the ruling's 超大核
// declaration reaches the answer instead of staying an internal label.
const clusterFreqDerivedPrimeLabel = "derived_prime"

// clusterFreqDeriveMaxSkewSec is the timestamp-consistency bound for merging
// two sampled cores' timelines into one derived domain. Emission semantics,
// MEASURED over the full donghu real capture
// (eval/fixtures/real_traces/donghu_tieba_frame.systrace, 30 transitions ×
// 3 member rows — CFR-2 verify round re-measurement): one DVFS transition
// emits one cpu_frequency row per cluster member from a single notifier
// loop, and the member rows within one burst spread at most 5µs; the two
// tightest DISTINCT transitions observed sit 46µs and 61µs apart. 15µs is
// ≈3× above the worst member spread and ≈3× below the tightest transition
// gap. The bound is only the SECOND factor of the merge criterion: even if
// two foreign transitions ever landed inside it, the merge additionally
// requires the full kHz sequences equal at EVERY index (值相等判据兜底) —
// both factors must fail together to false-merge. A violation SPLITS
// (fail-open, no reuse): a false split only loses reuse; a false merge
// would fabricate hardware state. Boundary pinned: 10µs same-value merges,
// 20µs splits (TestDeriveClusterFreqDomainsSkewBoundary).
const clusterFreqDeriveMaxSkewSec = 15e-6

// clusterFreqConVeto is one union-refusing contradiction edge recorded at
// derivation time (CLUSTERSTREAM-1 复核 F1, 2026-07-21): when the union-find
// refuses a pro merge because a CROSS-COMPONENT pair carries a con edge, the
// first such edge's endpoint cpus and its transition_conflict factors (both
// sides' transition targets + the pair's skew) are recorded on the derived
// result. Without the record the split audit could render EMPTY on a real
// veto split: the audit re-diagnoses only the two fragments' representative
// pair, and the vetoing con edge may sit between NON-representative members
// ({0,1}|{2,3} split by con(1,2) while the representative pair (0,2) itself
// carries pro≥floor ∧ con==0). DISCLOSURE ONLY — no gate may ever read it
// (the veto itself already fired inside deriveClusterFreqDomains).
type clusterFreqConVeto struct {
	cpuA, cpuB int
	ts         float64
	khzA, khzB int64
	skewSec    float64
}

// clusterFreqDomains is the frequency-domain map: cpu → domain label, plus
// the ascending member roster per label. source says where the membership
// came from (explicit topology vs change-point derivation); the derived form
// additionally keeps the ascending sampled-cpu list and the domain count for
// the inheritance / no-upward-extrapolation rules. Zero value = membership
// unknown = every donorFor call fails open.
type clusterFreqDomains struct {
	byCPU   map[int]string
	members map[string][]int
	// source is ClusterFreqSourceExplicit / ClusterFreqSourceDerived / ""
	// (unknown).
	source string
	// sampledAsc / groupCount are populated by the derived form only.
	sampledAsc []int
	groupCount int
	// explicitInputIgnored (CFR-2 verify P3-4): PRECISE flag — the caller
	// supplied a non-empty core_topology string but parsing admitted ZERO
	// entries (no recognizable cluster labels), so resolution fell through to
	// the derived lane. The caveat face must say so instead of claiming
	// "no explicit core_topology".
	explicitInputIgnored bool
	// conVetoes (复核 F1) — the union-refusing con edges recorded by the
	// derived form, in ascending union-scan order (deduped per edge).
	// Disclosure input for capabilityFreqOnlySplitAudit only.
	conVetoes []clusterFreqConVeto
	// partitionAudit (PARTDISC-1, 2026-07-24) carries only the exact facts
	// observed when the announcement-snapshot lane declined a merge. It is a
	// trace-global, label-independent disclosure input; sameGroup deliberately
	// reads only announceSnapshotPartition.fired/groupByCPU, so no gate can
	// consume this side record.
	partitionAudit announcePartitionAudit
}

// conVetoBetween returns the first recorded union-refusing con edge whose
// endpoints landed in the two given FINAL domains (order-insensitive). A con
// edge's endpoints always finish in different components (the veto keeps
// them apart by construction), so the final byCPU lookup is well-defined.
func (d clusterFreqDomains) conVetoBetween(labelA, labelB string) (clusterFreqConVeto, bool) {
	for _, v := range d.conVetoes {
		la, lb := d.byCPU[v.cpuA], d.byCPU[v.cpuB]
		if (la == labelA && lb == labelB) || (la == labelB && lb == labelA) {
			return v, true
		}
	}
	return clusterFreqConVeto{}, false
}

// parseClusterFreqDomains parses the explicit Query.CoreTopology string into
// frequency domains. Grammar and entry admission are IDENTICAL to
// parseCoreTopology (same separators, same recognized class labels via
// normalizeCoreClass) — the ONLY difference is the domain identity: the
// verbatim lowercased label, so "big" and "prime" stay two domains (see the
// file header). Empty/unparseable input yields the zero value (fail-open).
func parseClusterFreqDomains(raw string) clusterFreqDomains {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return clusterFreqDomains{}
	}
	var out clusterFreqDomains
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' }) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			k, v, ok = strings.Cut(part, ":")
		}
		if !ok {
			continue
		}
		// Admission parity with the explicit-topology gate: only labels the
		// class taxonomy recognizes participate (an entry parseCoreTopology
		// drops must not silently arm the reuse gate).
		if normalizeCoreClass(k) == "" {
			continue
		}
		label := strings.ToLower(strings.TrimSpace(k))
		for _, cpu := range parseCPURangeList(v) {
			if out.byCPU == nil {
				out.byCPU = map[int]string{}
				out.members = map[string][]int{}
			}
			if prev, dup := out.byCPU[cpu]; dup {
				// Later entry wins (map-overwrite semantics, same as
				// parseCoreTopology); drop the cpu from the previous roster.
				out.members[prev] = removeInt(out.members[prev], cpu)
			}
			out.byCPU[cpu] = label
			out.members[label] = append(out.members[label], cpu)
		}
	}
	for label := range out.members {
		sort.Ints(out.members[label])
	}
	if len(out.byCPU) > 0 {
		out.source = ClusterFreqSourceExplicit
	}
	return out
}

// resolveClusterFreqDomains is THE single entry every consumption face uses:
// an explicit core_topology wins outright; only in its ABSENCE does the
// CFR-2 trace-native derivation run over the face's own per-CPU sample
// timelines (lazy — the callback is only invoked when derivation is needed).
// Each face passes its own collection caliber (fold face: full-trace
// chainQueryCache timelines; window faces: the window collection), mirroring
// the CFC precedent: one shared ALGORITHM, per-face inputs.
func resolveClusterFreqDomains(rawTopology string, timelines func() map[int][]freqSample) clusterFreqDomains {
	if d := parseClusterFreqDomains(rawTopology); d.known() {
		return d
	}
	// CFR-2 verify P3-4: non-empty input that parsed to nothing is a PRECISE
	// signal the derived lane must disclose (静默换道禁止) — the caveat then
	// says "input had no recognizable cluster labels" instead of "no explicit
	// core_topology".
	ignored := strings.TrimSpace(rawTopology) != ""
	if timelines == nil {
		return clusterFreqDomains{explicitInputIgnored: ignored}
	}
	d := deriveClusterFreqDomains(timelines())
	d.explicitInputIgnored = ignored
	return d
}

// indexDerivedClusterFreqDomains is the Index-level lazy single derivation
// (CLUSTERSTREAM-1 件1 复用纪律, §29.193.1): the pairwise witness derivation
// runs ONCE per Index over THE Index-global sample basis
// (indexFreqSampleTimelines — full-file curves / side-scan / carve, poison
// filtering included) and every query of the same trace shares the memo.
// The returned struct copy shares its maps READ-ONLY BY CONTRACT (the
// clusterFreqDomains house rule). This memo stores a derivation of the
// Index's OWN basis only — never a per-query view — so it does not breach
// the side-scan cache's raw-content boundary (that boundary forbids derived
// conclusions in the CROSS-Index artifact cache; the Index memo is the same
// scope the derivedClassOnce capability memo already occupies).
func indexDerivedClusterFreqDomains(idx *Index) clusterFreqDomains {
	if idx == nil {
		return clusterFreqDomains{}
	}
	idx.clusterDomainsOnce.Do(func() {
		// CLUSTERTIE-1 件A (§29.200): the limits view of the SAME basis
		// decision joins the derivation as the announcement snapshot
		// partition's extra-evidence sub-veto input.
		idx.clusterDomains = deriveClusterFreqDomainsLimits(indexFreqSampleTimelines(idx), indexFreqLimitSampleTimelines(idx))
	})
	return idx.clusterDomains
}

// resolveClusterFreqDomainsIndexed is resolveClusterFreqDomains for callers
// whose derivation input IS the Index-global basis (the production faces:
// fold capability, window stats, scheduler latency): explicit topology wins
// outright; the derived arm reads the per-Index memo above instead of
// re-deriving per query. The P3-4 ignored-input flag stays per-call (it
// depends on the query's rawTopology, not on the trace).
func resolveClusterFreqDomainsIndexed(rawTopology string, idx *Index) clusterFreqDomains {
	if d := parseClusterFreqDomains(rawTopology); d.known() {
		return d
	}
	d := indexDerivedClusterFreqDomains(idx)
	d.explicitInputIgnored = strings.TrimSpace(rawTopology) != ""
	return d
}

// deriveClusterFreqDomains implements the R6 four-rule cluster derivation
// (§29.88.9 用户裁定 2026-07-14, docs/design/real_trace_campaign_20260705.md),
// evolving the CFR-2 (#80) trace-native derivation:
//
//	规则2 同簇判据 = 流式增量共移见证 (CLUSTERSTREAM-1, §29.193/§29.193.1 用户
//	授权 2026-07-21, CLUSTERDIAG dossier §5): two sampled cores belong to one
//	cluster iff their cpu_frequency streams accumulate co-movement WITNESSES
//	— paired state TRANSITIONS to the SAME new kHz value within the fixed
//	skew bound (clusterFreqDeriveMaxSkewSec) — reaching the fixed evidence
//	floor (clusterFreqCoWitnessFloor) with ZERO contradiction witnesses
//	(paired transitions to DIFFERENT values inside one skew window = con,
//	one-vote veto). 公告不铸见证: a core's first-seen value (its entry
//	announcement — no prior state) is not a transition and mints neither pro
//	nor con (structural exclusion of the §28.5 复核 P1 parked all-policy
//	announcement false merge). Only PAIRED transitions are compared — never
//	instantaneous carried state, so a multi-value co-movement burst
//	(DHMINE §29.172: cpu12+13 {1675000→1200000} inside one burst) counts as
//	pro witnesses, never as con. The historical whole-array SAME-EMISSION
//	identity survives as the SECOND merge lane (freqTimelinesSameEmission —
//	two parked cores with identical re-announce cadence carry zero
//	transitions and merge on emission identity exactly as CFR-2 pinned).
//	A canonical zero is a typed offline/unknown transition, not a capacity
//	sample. It remains in the state curve so carry cannot bridge across it;
//	fmax/CAP consumers independently require positive values.
//
//	EVOLUTION RECORD (CLUSTERSTREAM-1, §29.193.1, 2026-07-21): the CAP-3
//	(§29.11) boundary-TRIMMED whole-sequence identity family — HEAD junction
//	guard, MIDDLE strict 1:1 alignment, TAIL straddle exemption
//	(freqStateCoMoveTrimmed and the head_junction_state_mismatch /
//	mid_alignment_mismatch / tail_exemption_unmet split arms) — is RETIRED.
//	Its single-veto structure sentenced whole clusters on ONE unpaired
//	mid-stream change point (fleet witness case1: 2517 频点行 killed by one
//	mid_alignment_mismatch @17729.521567) while hundreds of co-witnessed
//	transitions stood; the witness criterion demotes one unpaired point to
//	one LOST witness (若真分歧持续 the pair keeps minting con and honestly
//	splits). Direction (§29.11 保守方向, unchanged): a false split only
//	loses reuse; a false merge would fabricate hardware state — hence the
//	con one-vote veto and the fixed pro floor (宁漏勿假).
//
//	规则1 首簇: cores 0 through the FIRST sampled core belong to that core's
//	cluster — the leading sample-less cores are MEMBERS of the first cluster
//	(donor resolution and class pricing reach them through ordinary
//	membership, no inheritance rule needed).
//
//	规则3 区间闭包: sample-less cores whose core number lies strictly inside
//	one cluster's member interval [min(member), max(member)] join that
//	cluster. Sampled cores always keep their rule-2 identity; on the
//	(hardware-unreachable) shape of overlapping intervals both claiming one
//	core, the lowest-labelled cluster wins deterministically.
//
//	向上不外推 (#80, unchanged by R6): cores above the highest sampled core
//	are never folded into a sampled domain. With ≥3 derived domains they are
//	declared the 超大核 pseudo-domain (clusterFreqDerivedPrimeLabel, no
//	sampled members — structurally donor-less); with fewer domains they stay
//	unassigned. Both forms fail open (honest 无频点数据 accounting).
//
//	EVOLUTION RECORD (R6, 2026-07-14): the former 向下继承 arm ("an unsampled
//	core inherits the nearest higher-numbered sampled core's domain") is
//	RETIRED. For leading cores rule 1 gives the identical result as direct
//	membership; for enclosed cores rule 3 does; the remaining shape — a
//	sample-less core BETWEEN two different clusters — used to be silently
//	claimed by the higher cluster (exactly the donghu 9-11→big misassignment
//	direction the R6 ruling adjudicates) and is now honestly UNASSIGNED
//	(fail-open: no donor, no class, 无频点数据 caveats stand).
//
//	规则4 全文件扫描: the timelines handed in here are the FULL-FILE curves
//	whenever the build's single pass covered the file (full_freq_curves.go via
//	indexFreqSampleTimelines) — the derivation input is never a window/budget
//	carve of the frequency history.
//
// Domain labels are synthetic ("derived_c0", "derived_c1", … ascending by
// lowest member) — deliberately NOT the small/middle/big display taxonomy:
// this is frequency-domain identity, not core-class classification.
func deriveClusterFreqDomains(timelines map[int][]freqSample) clusterFreqDomains {
	return deriveClusterFreqDomainsLimits(timelines, nil)
}

// deriveClusterFreqDomainsLimits is deriveClusterFreqDomains plus the
// cpu_frequency_limits view of the SAME collection basis (CLUSTERTIE-1 件A,
// §29.200): the limits lanes feed ONLY the announcement snapshot partition's
// extra-evidence sub-veto (a value-group whose members carry two distinct
// positive limits ceilings mints no partition merges). nil limits = the
// sub-veto is vacuous; every other criterion is byte-identical.
func deriveClusterFreqDomainsLimits(timelines, limits map[int][]freqSample) clusterFreqDomains {
	sampled := make([]int, 0, len(timelines))
	for cpu, tl := range timelines {
		if len(tl) > 0 {
			sampled = append(sampled, cpu)
		}
	}
	if len(sampled) == 0 {
		return clusterFreqDomains{}
	}
	sort.Ints(sampled)
	out := clusterFreqDomains{
		byCPU:      map[int]string{},
		members:    map[string][]int{},
		source:     ClusterFreqSourceDerived,
		sampledAsc: sampled,
	}
	// CLUSTERSTREAM-1 (§29.193.1) clustering: PAIRWISE witness verdicts over
	// every sampled pair (not representative-anchored — a con edge between ANY
	// two members must be able to veto a transitive union), then connected
	// components over the merge edges under the con one-vote veto.
	//
	//	merge edge (i,j) = sameEmission(i,j)  [second lane, parked-core form]
	//	                 ∨ (pro(i,j) ≥ clusterFreqCoWitnessFloor ∧ con(i,j)==0)
	//	veto            = a union of two components is refused when ANY cross
	//	                  pair between them carries a con edge (矛盾守卫传递
	//	                  安全 — the veto is checked against the CURRENT
	//	                  component rosters, so pro chains can never smuggle
	//	                  two contradicting cores into one cluster).
	//
	// Determinism: pairs are scanned in ascending (i,j) order and every union
	// keeps the smaller-min-member root, so component labels ascend by lowest
	// member exactly as the pre-batch representative scan did. Cost bound:
	// one witness scan per pair — O(P² · N/P) = O(P·N) with P sampled CPUs
	// and N total collected samples (N is already bounded by the side-scan /
	// in-pass sample caps); the derivation itself runs once per Index
	// (indexDerivedClusterFreqDomains memo). 稠密窗假设 (复核 F5, 2026-07-21):
	// the per-pair scan's inner window walk is linear only while few
	// transitions crowd one ±15µs skew window (true of DVFS streams — one
	// transition per governor decision, the donghu-measured ≥46µs transition
	// gap); a poisoned/adversarial timestamp pile-up (many distinct-value
	// transitions inside one window) degrades the pair scan toward O(k²) over
	// the crowded window and is bounded only by the collection sample caps
	// above, not structurally excluded (clock-regression poisoning covers the
	// rollback shape only). Deliberately NOT capped harder: a step cap would
	// change the merge/split verdict on a noisy signal (§29.129 既裁③ fixed
	// bounds; 宁漏勿假 keeps the criterion, this note records the assumption).
	n := len(sampled)
	proEdge := make([][]bool, n)
	conEdge := make([][]bool, n)
	for i := range proEdge {
		proEdge[i] = make([]bool, n)
		conEdge[i] = make([]bool, n)
	}
	// CLUSTERTIE-1 件A (§29.200, 2026-07-21): the announcement snapshot
	// partition — the third merge lane, computed once over the whole stream.
	// The customer fleet's collector re-announces EVERY core's standing value
	// every ~1ms (恒值周期全量公告形): zero transitions, so the witness lane
	// honestly floors (公告不铸见证 by design) and the same-emission lane dies
	// on its all-or-nothing preconditions (one lost row anywhere breaks the
	// per-index identity). The partition lane reads the announcements
	// themselves as structure evidence: value groups constant across every
	// full snapshot = the cluster structure, proven per burst by value
	// disjointness. See deriveAnnounceSnapshotPartition for the criteria.
	snap := deriveAnnounceSnapshotPartition(sampled, timelines, limits)
	out.partitionAudit = snap.audit
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			a, b := timelines[sampled[i]], timelines[sampled[j]]
			if freqTimelinesSameEmission(a, b) {
				proEdge[i][j] = true
				continue
			}
			w := freqWitnessScanPair(a, b)
			if w.con > 0 {
				// Real contradiction witnesses trump parked announcements —
				// the con veto outranks every merge lane, partition included.
				conEdge[i][j] = true
				continue
			}
			if w.pro >= clusterFreqCoWitnessFloor {
				proEdge[i][j] = true
				continue
			}
			if snap.sameGroup(sampled[i], sampled[j]) {
				// 件A: same constant announcement-partition group — the merge
				// is backed by every full snapshot (组内合并由全部 burst 一致
				// 背书); cross-group pairs stay on the witness verdict (组间
				// 分离每 burst 由值互异直接证明 — no edge is minted there,
				// and a witness-lane merge of a cross-group pair keeps its
				// own authority: real transitions outrank announcements).
				//
				// 追审记档 冷读 P3-3 (CLUSTERTIE-1 dual review 2026-07-21,
				// P3): group separation is a PROOF face, not a veto — a
				// cross-group pair accumulating pro witnesses in partial
				// bursts merges on the witness lane (deliberate, §29.200②
				// evidence order: 公告弱于真变迁); and the con>0 ∧ sameGroup
				// overlap arm (partial-burst divergent transitions + full-
				// snapshot partition coexisting) has no dedicated pin — code
				// order takes the con continue first, covered generically by
				// the CLUSTERSTREAM con-veto family. 若要闭死可加 con+
				// sameGroup 专项负臂 pin(§29.193 追审面).
				proEdge[i][j] = true
			}
		}
	}
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(v int) int {
		for parent[v] != v {
			parent[v] = parent[parent[v]]
			v = parent[v]
		}
		return v
	}
	// conBetween locates the FIRST (ascending scan order) cross-component con
	// edge between the two live rosters; (0,0,false) when none.
	conBetween := func(ra, rb int) (int, int, bool) {
		for i := 0; i < n; i++ {
			if find(i) != ra {
				continue
			}
			for j := 0; j < n; j++ {
				if find(j) != rb {
					continue
				}
				lo, hi := i, j
				if lo > hi {
					lo, hi = hi, lo
				}
				if conEdge[lo][hi] {
					return lo, hi, true
				}
			}
		}
		return 0, 0, false
	}
	// recordConVeto (复核 F1): remember the union-refusing con edge with its
	// transition_conflict factors so the split audit can disclose the actual
	// veto even when the audited fragments' representative pair co-moves.
	// The factor extraction re-runs the SAME witness scan that minted the con
	// edge (no second judgment copy); deduped per edge because several pro
	// edges may retry the same component union.
	recordConVeto := func(lo, hi int) {
		a, b := sampled[lo], sampled[hi]
		for _, v := range out.conVetoes {
			if v.cpuA == a && v.cpuB == b {
				return
			}
		}
		w := freqWitnessScanPair(timelines[a], timelines[b])
		if !w.conSet {
			return // unreachable: the con edge was minted from this very scan
		}
		out.conVetoes = append(out.conVetoes, clusterFreqConVeto{cpuA: a, cpuB: b,
			ts: w.conTs, khzA: w.conKhzA, khzB: w.conKhzB, skewSec: w.conSkewSec})
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if !proEdge[i][j] {
				continue
			}
			ri, rj := find(i), find(j)
			if ri == rj {
				continue
			}
			if lo, hi, vetoed := conBetween(ri, rj); vetoed {
				recordConVeto(lo, hi)
				continue // 一票否决: cross-component contradiction witness
			}
			if ri < rj {
				parent[rj] = ri
			} else {
				parent[ri] = rj
			}
		}
	}
	// Component labels ascend by lowest member (root == the component's
	// minimum sampled index by the union rule above).
	rootGroup := map[int]int{}
	groups := 0
	for i := 0; i < n; i++ {
		r := find(i)
		gi, ok := rootGroup[r]
		if !ok {
			gi = groups
			rootGroup[r] = gi
			groups++
		}
		label := derivedDomainLabel(gi)
		out.byCPU[sampled[i]] = label
		out.members[label] = append(out.members[label], sampled[i])
	}
	out.groupCount = groups
	// R6 规则1 (首簇) + 规则3 (区间闭包): sample-less cores become MEMBERS —
	// leading cores join the first sampled core's cluster; enclosed cores join
	// the enclosing cluster. Ascending group order (labels ascend by lowest
	// member) makes any overlap claim deterministic; sampled cores are never
	// reassigned. Trailing cores above the highest sampled core stay out
	// (向上不外推, unchanged).
	for gi := 0; gi < groups; gi++ {
		label := derivedDomainLabel(gi)
		members := out.members[label]
		lo, hi := members[0], members[len(members)-1]
		if gi == 0 && sampled[0] == lo {
			// 规则1: the group holding the FIRST sampled core absorbs cores
			// 0..firstSampled-1. (lo == sampled[0] is structurally true for
			// derived_c0 — kept as a precise guard.)
			lo = 0
		}
		for cpu := lo; cpu < hi; cpu++ {
			if _, taken := out.byCPU[cpu]; taken {
				continue
			}
			out.byCPU[cpu] = label
			out.members[label] = append(out.members[label], cpu)
		}
		sort.Ints(out.members[label])
	}
	return out
}

func derivedDomainLabel(group int) string {
	return "derived_c" + strconv.Itoa(group)
}

// --- CLUSTERTIE-1 件A (§29.200, 2026-07-21): announcement snapshot partition -
//
// 公告快照分区一致性 — the third merge lane's judgment. Fleet witness
// (grep_result.txt, record_trace_20260526170707@880): the collector re-emits
// every core's STANDING frequency every ~1ms from whatever thread is on duty
// (cpu0-3=1600000 / cpu4-9=2151000 / cpu10-11=2500000, constant across 200+
// sweeps). Zero transitions ⇒ the witness lane floors honestly, and one lost
// row anywhere ⇒ the same-emission lane's per-index identity breaks. But the
// three-cluster structure IS in the data — proven once per millisecond by the
// value grouping of each full announcement sweep.
//
// Criteria (all fixed, zero adaptivity — §29.129 既裁③; the only tolerance is
// the existing clusterFreqDeriveMaxSkewSec as the burst chain gap, and the
// only floor is the existing clusterFreqCoWitnessFloor as the snapshot count):
//
//	burst      — maximal run of the globally ts-sorted positive samples where
//	             each consecutive gap ≤ clusterFreqDeriveMaxSkewSec (chain
//	             linking: the customer sweep spans ~23µs end to end but every
//	             consecutive gap is ≤10µs; a real DVFS transition sits ≥46µs
//	             from its neighbours on the measured platform and never chains
//	             into a sweep).
//	full 快照   — a burst carrying EXACTLY one sample per sampled CPU (a lost
//	             row or a straddling transition makes the burst partial and it
//	             is SKIPPED — robustness the same-emission identity lacks; a
//	             skipped burst neither proves nor vetoes).
//	分区恒定    — the value-grouping of the sampled CPUs is IDENTICAL across
//	             every full snapshot (values may move between snapshots; the
//	             grouping may not). Any drift kills the whole signal
//	             (fail-open — hotplug/policy re-org shapes stay honest).
//	快照地板    — ≥ clusterFreqCoWitnessFloor (2) full snapshots (one
//	             coincident sweep is not structure — the §28.5 P1 rationale,
//	             same constant lineage as the witness floor).
//	limits 副证 — §29.200 处置: a value-group whose members carry ≥2 DISTINCT
//	             positive cpu_frequency_limits lane maxima is policy-boundary
//	             contradicted (two pressed-together policies) and mints NO
//	             partition merges (fail-open, no guessed sub-membership);
//	             groups with ≤1 distinct ceiling merge.
//
// §28.5 毒形 (two true clusters parked at ONE value the whole trace): they
// share a value group in every snapshot and — absent the limits 副证 — merge
// honestly: every member held the SAME frequency for the entire trace, so the
// frequency-ratio pricing of the merged blob is value-identical to the split
// truth (capRatio 恒 1 无损, §29.200 处置 verbatim); the fmax ladder above
// still refuses to coin-flip an order between equal-fmax groups.
//
// DISCLOSURE: the partition merges ride the existing derived-lane disclosure
// faces (freq_comovement topology token + the C2 limits-anchor roster);
// no new wire token (the membership evidence is still measured cpu_frequency
// rows of the trace itself).
//
// PARTDISC-1 refusal tokens are a closed disclosure vocabulary. The reserved
// value-set token belongs to the separate F2 live-specimen repair and is not
// minted by this batch.
const (
	announcePartitionRefusalBelowFloor = "partition_below_floor"
	announcePartitionRefusalDrift      = "partition_drift"
	announcePartitionRefusalLimitsVeto = "partition_limits_veto"
	announcePartitionRefusalValueSet   = "partition_value_set_veto"
)

// announcePartitionAudit records facts the partition derivation already
// observed before declining a merge. It is disclosure-only: refusal never
// enters sameGroup, and limits-veto groups merely explain members already
// withheld from groupByCPU.
type announcePartitionAudit struct {
	refusal          string
	snapshots        int
	driftTs          float64
	limitsVetoGroups []announcePartitionLimitsVetoGroup
}

type announcePartitionLimitsVetoGroup struct {
	members  []int
	ceilings []int64
}

// 备案 复核 F2 (CLUSTERTIE-1 dual review 2026-07-21, P3, 记档不修): the merge
// is blind to 快照间亚周期异值漂移 — two true clusters constant-equal in every
// FULL snapshot but each briefly jumping to DIFFERENT values BETWEEN snapshots
// (mutually >15µs apart ⇒ con=0, pro<floor) still merge, and the §29.200
// lossless argument (capRatio 恒 1) fails there: the merged fmax takes the
// strong side's excursion value and overstates the weak side. judgeBurst only
// reads full-burst values; the members' whole-trace value-set divergence is
// not a veto input. Reachability is harsh (EVERY full sweep must miss EVERY
// excursion) and the customer constant-value fleet shape (zero transitions)
// is untouched, but the direction is 假并 (违宁漏勿假) — hence filed.
// fix_direction (报告 verbatim): 分区组内 merge 前加值集同质性副证:同组成员
// 全 trace 正值集合(或 max)不等→该组不铸 merge(fail-open 与 limits 副证
// 同构);或在账本落『接受残洞』裁定注明该形。
type announceSnapshotPartition struct {
	fired      bool
	snapshots  int
	groupByCPU map[int]int
	audit      announcePartitionAudit
}

// sameGroup reports whether both cpus sit in one constant announcement
// partition group (false when the signal never fired or a cpu's group was
// withheld by the limits sub-veto).
func (p announceSnapshotPartition) sameGroup(a, b int) bool {
	if !p.fired {
		return false
	}
	ga, okA := p.groupByCPU[a]
	gb, okB := p.groupByCPU[b]
	return okA && okB && ga == gb
}

// deriveAnnounceSnapshotPartition computes the announcement snapshot
// partition over the sampled roster (ascending) — see the criteria doc above.
func deriveAnnounceSnapshotPartition(sampled []int, timelines, limits map[int][]freqSample) announceSnapshotPartition {
	if len(sampled) < 2 {
		return announceSnapshotPartition{}
	}
	type sampleEvent struct {
		ts  float64
		cpu int
		khz int64
	}
	total := 0
	for _, cpu := range sampled {
		total += len(timelines[cpu])
	}
	events := make([]sampleEvent, 0, total)
	for _, cpu := range sampled {
		for _, s := range timelines[cpu] {
			if s.khz > 0 {
				events = append(events, sampleEvent{ts: s.ts, cpu: cpu, khz: s.khz})
			}
		}
	}
	if len(events) == 0 {
		return announceSnapshotPartition{}
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].ts != events[j].ts {
			return events[i].ts < events[j].ts
		}
		return events[i].cpu < events[j].cpu
	})
	idxByCPU := make(map[int]int, len(sampled))
	for k, cpu := range sampled {
		idxByCPU[cpu] = k
	}
	var refSig []int
	snapshots := 0
	burstVal := make([]int64, len(sampled))
	burstSeen := make([]bool, len(sampled))
	start := 0
	judgeBurst := func(lo, hi int) bool { // false = partition drift, kill the signal
		if hi-lo != len(sampled) {
			return true // partial burst: skipped, never a veto
		}
		for k := range burstSeen {
			burstSeen[k] = false
		}
		for k := lo; k < hi; k++ {
			ci := idxByCPU[events[k].cpu]
			if burstSeen[ci] {
				return true // duplicate cpu (straddling transition): partial
			}
			burstSeen[ci] = true
			burstVal[ci] = events[k].khz
		}
		// Full snapshot: canonical grouping signature (group ids by first
		// appearance over the ascending roster).
		sig := make([]int, len(sampled))
		gidByVal := map[int64]int{}
		for k := range sampled {
			gid, ok := gidByVal[burstVal[k]]
			if !ok {
				gid = len(gidByVal)
				gidByVal[burstVal[k]] = gid
			}
			sig[k] = gid
		}
		if refSig == nil {
			refSig = sig
		} else {
			for k := range sig {
				if sig[k] != refSig[k] {
					return false // 分区漂移 — the whole signal dies
				}
			}
		}
		snapshots++
		return true
	}
	for k := 1; k <= len(events); k++ {
		if k < len(events) && events[k].ts-events[k-1].ts <= clusterFreqDeriveMaxSkewSec {
			continue
		}
		if !judgeBurst(start, k) {
			return announceSnapshotPartition{audit: announcePartitionAudit{
				refusal:   announcePartitionRefusalDrift,
				snapshots: snapshots,
				driftTs:   events[start].ts,
			}}
		}
		start = k
	}
	if refSig == nil || snapshots < clusterFreqCoWitnessFloor {
		if snapshots == 1 {
			return announceSnapshotPartition{audit: announcePartitionAudit{
				refusal: announcePartitionRefusalBelowFloor, snapshots: snapshots,
			}}
		}
		return announceSnapshotPartition{}
	}
	out := announceSnapshotPartition{fired: true, snapshots: snapshots, groupByCPU: map[int]int{}}
	groups := map[int][]int{}
	for k, cpu := range sampled {
		groups[refSig[k]] = append(groups[refSig[k]], cpu)
	}
	// groups is a map because the signature ids are sparse implementation
	// details. Sort by each group's first (therefore minimum) CPU before
	// producing audit rows so identical traces always disclose identical
	// limits-veto order.
	groupIDs := make([]int, 0, len(groups))
	for gid := range groups {
		groupIDs = append(groupIDs, gid)
	}
	sort.Slice(groupIDs, func(i, j int) bool {
		return groups[groupIDs[i]][0] < groups[groupIDs[j]][0]
	})
	for _, gid := range groupIDs {
		members := groups[gid]
		// limits 副证 sub-veto: ≥2 distinct positive per-member lane maxima.
		distinct := map[int64]bool{}
		for _, cpu := range members {
			var laneMax int64
			for _, s := range limits[cpu] {
				if s.khz > laneMax {
					laneMax = s.khz
				}
			}
			if laneMax > 0 {
				distinct[laneMax] = true
			}
		}
		if len(distinct) >= 2 {
			ceilings := make([]int64, 0, len(distinct))
			for khz := range distinct {
				ceilings = append(ceilings, khz)
			}
			sort.Slice(ceilings, func(i, j int) bool { return ceilings[i] < ceilings[j] })
			out.audit.limitsVetoGroups = append(out.audit.limitsVetoGroups, announcePartitionLimitsVetoGroup{
				members: append([]int(nil), members...), ceilings: ceilings,
			})
			continue // policy-boundary contradicted: no merges for this group
		}
		for _, cpu := range members {
			out.groupByCPU[cpu] = gid
		}
	}
	return out
}

// TOMBSTONE (CAP-3 §29.11, 2026-07-10): windowFreqSampleTimelines — the
// window faces' Event→freqSample derivation adapter — is RETIRED. Feeding
// each face's window-cropped collection into deriveClusterFreqDomains re-cut
// the sample stream at the query window and forked the topology judgment per
// window (the adjudicated lane fork behind huadong_792's "簇结构不可判" vs
// g12's judged form). The window faces' donor DERIVATION now reads
// indexFreqSampleTimelines (below); the donor timelines' VALUES still come
// from each face's own window collection at the call sites (governance
// caliber untouched — only MEMBERSHIP went Index-global).

// indexFreqTransitionTimelines is THE Index-global per-CPU cpu_frequency
// STATE timeline collector. Canonical zero transitions remain here solely as
// carry barriers; chainQueryCache frequency governance consumes this map.
// Cluster/CAP/rail evidence must instead consume indexFreqSampleTimelines,
// whose positive-only projection cannot count a zero barrier as a capacity
// sample or co-movement point.
func indexFreqTransitionTimelines(idx *Index) map[int][]freqSample {
	if idx == nil {
		return map[int][]freqSample{}
	}
	idx.freqTransitionTimelinesOnce.Do(func() {
		// Shared arm discipline (54ce112ed defensive gate × CLUSTER-FIX-1
		// §29.129): every basis arm applies the current index's integrity
		// ledger as a defensive second gate (a composite or warm derived
		// index must never let a sibling/full-map/cached fast path resurrect
		// an unsafe CPU lane), records the typed ClusterSampleBasis* token,
		// and rosters every lane the filter removed (S4 收披露 — judgment
		// unchanged, the caveat lane discloses).
		filterCurves := func(tls map[int][]freqSample, integrity frequencyOrderIntegrity, unsafeAll bool, unsafeByCPU map[int]bool, seed []int) (map[int][]freqSample, []int) {
			dropped := append([]int(nil), seed...)
			clean := true
			for cpu := range tls {
				if integrity.frequencyUnsafe(cpu) || unsafeAll || unsafeByCPU[cpu] {
					clean = false
					break
				}
			}
			if clean {
				return tls, dropped
			}
			out := make(map[int][]freqSample, len(tls))
			for cpu, samples := range tls {
				if integrity.frequencyUnsafe(cpu) || unsafeAll || unsafeByCPU[cpu] {
					if !containsInt(dropped, cpu) {
						dropped = append(dropped, cpu)
					}
					continue
				}
				out[cpu] = samples
			}
			return out, dropped
		}
		// limitsDropped (CLUSTER-FIX-2 件6 = 裁定⑨): the limits-lane dropped
		// roster for the SAME basis decision — full-file lanes carry their
		// poison-application roster plus the defensive integrity filter; the
		// window-carve arm mirrors the events-fallback filter below. Pure
		// disclosure memo (idx.freqLimitTimelinesDropped), judgment untouched.
		// CLUSTERTIE-1 件A (§29.200): the KEPT complement is memoized beside
		// the roster (idx.freqLimitTimelines) as the announcement snapshot
		// partition's limits sub-veto input — same basis, same Once.
		integrity := frequencyOrderIntegrityForGlobalDerivation(idx)
		limitsDropped := func(limitTLs map[int][]freqSample, seed []int, limitAll bool, limitUnsafe map[int]bool) []int {
			dropped := append([]int(nil), seed...)
			kept := map[int][]freqSample{}
			for cpu, samples := range limitTLs {
				if integrity.limitUnsafe(cpu) || limitAll || limitUnsafe[cpu] {
					if !containsInt(dropped, cpu) {
						dropped = append(dropped, cpu)
					}
					continue
				}
				kept[cpu] = samples
			}
			idx.freqLimitTimelines = kept
			sort.Ints(dropped)
			return dropped
		}
		// R6 rule 4 (§29.88.9, full_freq_curves.go): when the build's single
		// forward pass covered the whole file, the FULL-FILE state curves are
		// the basis. Order poisoning was audited over that same superset.
		if tls, ok := idx.fullFrequencyTimelines(); ok {
			out, dropped := filterCurves(tls, integrity, idx.fullFreq.freqAll, idx.fullFreq.freqUnsafe, idx.fullFreq.droppedFreqCPUs)
			sort.Ints(dropped)
			idx.freqTransitionTimelines = out
			idx.freqTimelinesBasis = ClusterSampleBasisFullIndex
			idx.freqTimelinesDropped = dropped
			idx.freqLimitTimelinesDropped = limitsDropped(idx.fullFreq.limitByCPU, idx.fullFreq.droppedLimitCPUs, idx.fullFreq.limitAll, idx.fullFreq.limitUnsafe)
			return
		}
		// CLUSTER-FIX-1 (user ruling 2026-07-18, freq_side_scan.go): the
		// in-pass collection could not cover the file (line-window stop /
		// padding truncation / unstamped anchor-seek / composite skip) — the
		// bounded streaming side-scan recovers the SAME Index-global
		// full-file basis before any window carve is consulted.
		// Precise-signal chain: collected flags only, never a heuristic. The
		// scan ran the same collector (own poison audit over the full file);
		// the defensive filter above still applies the current index's
		// integrity verdicts on top (cached cross-Index reuse path).
		if curves, degrade := idx.sideScanFreqTimelines(); degrade == "" && curves.collected {
			out, dropped := filterCurves(curves.freqByCPU, integrity, curves.freqAll, curves.freqUnsafe, curves.droppedFreqCPUs)
			sort.Ints(dropped)
			idx.freqTransitionTimelines = out
			idx.freqTimelinesBasis = ClusterSampleBasisSideScan
			idx.freqTimelinesDropped = dropped
			idx.freqLimitTimelinesDropped = limitsDropped(curves.limitByCPU, curves.droppedLimitCPUs, curves.limitAll, curves.limitUnsafe)
			return
		}
		out := map[int][]freqSample{}
		limitOut := map[int][]freqSample{}
		var dropped, limitDropped []int
		for _, ev := range idx.Events {
			if cpu, _, maxKHz, ok := perCPULimitSampleValues(ev); ok {
				if integrity.limitUnsafe(cpu) {
					// 裁定⑨ window-carve arm: mirror of the buildFreqLimitIndex
					// events-fallback filter (roster only — that filter's drop
					// judgment is untouched).
					if !containsInt(limitDropped, cpu) {
						limitDropped = append(limitDropped, cpu)
					}
				} else {
					// CLUSTERTIE-1 件A: the kept limits complement (same
					// admission as the buildFreqLimitIndex events fallback).
					limitOut[cpu] = append(limitOut[cpu], freqSample{ts: ev.Ts, khz: maxKHz})
				}
			}
			cpu, khz, ok := perCPUFrequencyTransitionValues(ev)
			if !ok {
				continue
			}
			if integrity.frequencyUnsafe(cpu) {
				if !containsInt(dropped, cpu) {
					dropped = append(dropped, cpu)
				}
				continue
			}
			out[cpu] = append(out[cpu], freqSample{ts: ev.Ts, khz: khz})
		}
		sort.Ints(dropped)
		sort.Ints(limitDropped)
		idx.freqTransitionTimelines = out
		idx.freqTimelinesBasis = ClusterSampleBasisWindowCarve
		idx.freqTimelinesDropped = dropped
		idx.freqLimitTimelinesDropped = limitDropped
		idx.freqLimitTimelines = limitOut
	})
	return idx.freqTransitionTimelines
}

// indexFreqSampleTimelines is the Index-global POSITIVE hard-evidence view
// used by cluster derivation, CAP sample floors, rail cross-validation and
// fmax evidence. It is a memoized projection of the single transition
// authority above, so admission/order/CPU ownership cannot drift. When no
// zero transition exists it reuses the state map by identity; otherwise it
// allocates only the positive entries.
//
// 复核 P3: memoized once per Index (sync.Once, concurrent-read safe — the
// Index lazy-memo house pattern, see Index.freqTimelinesOnce). The returned
// map and its slices are READ-ONLY BY CONTRACT for every consumer.
func indexFreqSampleTimelines(idx *Index) map[int][]freqSample {
	if idx == nil {
		return map[int][]freqSample{}
	}
	idx.freqTimelinesOnce.Do(func() {
		transitions := indexFreqTransitionTimelines(idx)
		hasZero := false
		for _, samples := range transitions {
			for _, sample := range samples {
				if sample.khz == 0 {
					hasZero = true
					break
				}
			}
			if hasZero {
				break
			}
		}
		if !hasZero {
			idx.freqTimelines = transitions
			return
		}
		out := make(map[int][]freqSample, len(transitions))
		for cpu, samples := range transitions {
			for _, sample := range samples {
				if sample.khz > 0 {
					out[cpu] = append(out[cpu], sample)
				}
			}
		}
		idx.freqTimelines = out
	})
	return idx.freqTimelines
}

// indexFreqLimitSampleTimelines (CLUSTERTIE-1 件A, §29.200) exposes the KEPT
// cpu_frequency_limits lanes of the active derivation basis (written inside
// the freqTransitionTimelinesOnce memo — see the Index field doc). Input to
// the announcement snapshot partition's limits sub-veto only; READ-ONLY BY
// CONTRACT like every sibling memo view.
func indexFreqLimitSampleTimelines(idx *Index) map[int][]freqSample {
	if idx == nil {
		return map[int][]freqSample{}
	}
	indexFreqTransitionTimelines(idx)
	return idx.freqLimitTimelines
}

// indexClusterSampleBasis resolves (building the memo when needed) the typed
// ClusterSampleBasis* token for idx's active derivation basis plus the sorted
// integrity-dropped cpu_frequency lanes (CLUSTER-FIX-1 件2/件3 disclosure
// inputs; both READ-ONLY, never gates).
func indexClusterSampleBasis(idx *Index) (basis string, droppedCPUs []int) {
	if idx == nil {
		return "", nil
	}
	indexFreqSampleTimelines(idx)
	return idx.freqTimelinesBasis, idx.freqTimelinesDropped
}

// freqTimelinesSameEmission is the precise 同域判据 (see
// deriveClusterFreqDomains): equal length, per-index equal kHz, per-index
// timestamp skew within clusterFreqDeriveMaxSkewSec.
func freqTimelinesSameEmission(a, b []freqSample) bool {
	if len(a) != len(b) || len(a) == 0 {
		return false
	}
	for i := range a {
		if a[i].khz != b[i].khz {
			return false
		}
		skew := a[i].ts - b[i].ts
		if skew < 0 {
			skew = -skew
		}
		if skew > clusterFreqDeriveMaxSkewSec {
			return false
		}
	}
	return true
}

// freqTimelinesCoMove is the CLUSTERSTREAM-1 (§29.193.1) pair-level
// co-movement criterion: the exact same-emission identity above (fast path,
// historical semantics byte-for-byte — including its constant-equal-value
// merge form), OR the accumulated-witness verdict (pro ≥ floor ∧ con == 0).
// EVOLUTION RECORD (§29.193.1): the former globalTailTs parameter is retired
// with the tail-straddle exemption — the witness criterion has no boundary
// arms to anchor.
func freqTimelinesCoMove(a, b []freqSample) bool {
	if freqTimelinesSameEmission(a, b) {
		return true
	}
	ok, _ := freqWitnessCoMoveDiag(a, b)
	return ok
}

// freqStateChangePoints dedups a raw ts-ordered sample timeline into state
// CHANGE points (cpu_frequency is a state lane — a re-announcement of the
// standing value is not a transition and must not desynchronize the pairwise
// alignment below).
func freqStateChangePoints(tl []freqSample) []freqSample {
	out := make([]freqSample, 0, len(tl))
	for _, s := range tl {
		if len(out) > 0 && out[len(out)-1].khz == s.khz {
			continue
		}
		out = append(out, s)
	}
	return out
}

// freqCoMoveSplit localizes the verdict that kept a pair apart (CAP-3 复核
// P2, 2026-07-10; CLUSTERSTREAM-1 件3 败因因子 extension, §29.193.1): the
// judging arm token, the timestamp at the deciding witness, and the failure
// FACTORS — a value conflict records both sides' transition targets plus the
// pair's skew (kHz 不等), an evidence-floor split records the accumulated pro
// count plus the tightest same-value near-miss beyond the bound (偏斜超界).
// DISCLOSURE/AUDIT INPUT ONLY — no gate may ever read it (the split decision
// itself is the gate; this struct merely says WHERE and WHY it fired, so a
// customer replay can distinguish a lost-row form from real divergence).
type freqCoMoveSplit struct {
	arm string
	ts  float64
	// transition_conflict factors (meaningful on that arm only):
	conKhzA, conKhzB int64
	conSkewSec       float64
	// co_witness_floor factors (meaningful on that arm only):
	pro         int
	nearSkewSec float64 // tightest unmatched same-value cross skew; valid iff nearSkewSet
	nearSkewSet bool
}

// Split-arm tokens (typed vocabulary for the audit face; see
// freqCoMoveSplitArmZH for the display labels).
//
// EVOLUTION RECORD (CLUSTERSTREAM-1, §29.193.1, 2026-07-21): the trimmed
// whole-sequence arms head_junction_state_mismatch / mid_alignment_mismatch /
// tail_exemption_unmet are RETIRED with their criterion (see the
// deriveClusterFreqDomains EVOLUTION RECORD). The surviving audit vocabulary
// is the witness criterion's own: co_witness_floor evolves its semantics from
// "aligned change pairs < 2 (entry announcement counted)" to "co-witnessed
// TRANSITIONS < 2 (announcements never counted)", and transition_conflict is
// the new contradiction-veto arm.
const (
	freqCoMoveSplitArmEmpty    = "no_samples"
	freqCoMoveSplitArmFloor    = "co_witness_floor"
	freqCoMoveSplitArmConflict = "transition_conflict"
)

// freqCoMoveSplitArmZH maps a split-arm token to its zh display label (the
// audit wording keeps token+label side by side so the line stays greppable
// AND readable).
func freqCoMoveSplitArmZH(arm string) string {
	switch arm {
	case freqCoMoveSplitArmEmpty:
		return "一侧无采样"
	case freqCoMoveSplitArmFloor:
		return "共见证变迁不足"
	case freqCoMoveSplitArmConflict:
		return "同窗异值变迁"
	default:
		return ""
	}
}

// freqPairWitness is one pair's accumulated witness account (CLUSTERSTREAM-1
// 件1): pro / con counts plus the disclosure factors the split audit reports.
// The counts are computed over deduped state TRANSITIONS only — each side's
// entry announcement (first change point) is excluded (公告不铸见证).
type freqPairWitness struct {
	pro int
	con int
	// first contradiction localization (valid iff conSet):
	conSet           bool
	conTs            float64
	conKhzA, conKhzB int64
	conSkewSec       float64
	// first unpaired transition (either side, valid iff unpairedSet):
	unpairedSet bool
	unpairedTs  float64
	// tightest same-value cross pair among the UNMATCHED transitions
	// (necessarily beyond the skew bound; valid iff nearSkewSet):
	nearSkewSet bool
	nearSkewSec float64
}

// freqWitnessScanPair accumulates the pairwise pro/con witnesses for two raw
// sample timelines (single forward walk per side, CLUSTERSTREAM-1 件1):
//
//	pro — transition pairing, value-keyed: a transition of one side matches
//	      the earliest unmatched SAME-kHz transition of the other side within
//	      the fixed clusterFreqDeriveMaxSkewSec window. Greedy
//	      earliest-compatible matching over ascending timestamps is a maximum
//	      matching per value class, so the pro count is symmetric in the
//	      argument order.
//	con — after pro matching, an UNMATCHED transition of one side with an
//	      UNMATCHED transition of the other side inside the skew window is a
//	      contradiction witness (their values necessarily differ — equal
//	      values would have pro-matched). Only paired TRANSITIONS compare;
//	      carried state never does, so a multi-value co-movement burst
//	      (DHMINE §29.172) yields pro pairs per value and zero con. The
//	      con>0 verdict is symmetric; the count itself is orientation-scoped
//	      and feeds disclosure only.
//
// All bounds are the fixed ruled constants (§29.129 既裁③: no adaptive
// widening anywhere); the whole account is disclosure + criterion input —
// no other gate may read it.
func freqWitnessScanPair(a, b []freqSample) freqPairWitness {
	x, y := freqStateChangePoints(a), freqStateChangePoints(b)
	var tx, ty []freqSample
	if len(x) > 1 {
		tx = x[1:]
	}
	if len(y) > 1 {
		ty = y[1:]
	}
	var w freqPairWitness
	matchedX := make([]bool, len(tx))
	matchedY := make([]bool, len(ty))
	// Phase 1 — value-keyed pro matching.
	jStart := 0
	for i := range tx {
		for jStart < len(ty) && ty[jStart].ts < tx[i].ts-clusterFreqDeriveMaxSkewSec {
			jStart++
		}
		for j := jStart; j < len(ty) && ty[j].ts <= tx[i].ts+clusterFreqDeriveMaxSkewSec; j++ {
			if !matchedY[j] && ty[j].khz == tx[i].khz {
				matchedX[i], matchedY[j] = true, true
				w.pro++
				break
			}
		}
	}
	// Phase 2 — contradiction scan over the unmatched remainder, plus the
	// first-unpaired localization and the near-miss factor inputs.
	recordUnpaired := func(ts float64) {
		if !w.unpairedSet || ts < w.unpairedTs {
			w.unpairedSet, w.unpairedTs = true, ts
		}
	}
	unmatchedYByVal := map[int64][]float64{}
	for j := range ty {
		if !matchedY[j] {
			recordUnpaired(ty[j].ts)
			unmatchedYByVal[ty[j].khz] = append(unmatchedYByVal[ty[j].khz], ty[j].ts)
		}
	}
	jStart = 0
	for i := range tx {
		if matchedX[i] {
			continue
		}
		recordUnpaired(tx[i].ts)
		for jStart < len(ty) && ty[jStart].ts < tx[i].ts-clusterFreqDeriveMaxSkewSec {
			jStart++
		}
		for j := jStart; j < len(ty) && ty[j].ts <= tx[i].ts+clusterFreqDeriveMaxSkewSec; j++ {
			if matchedY[j] {
				continue
			}
			w.con++
			if !w.conSet {
				skew := tx[i].ts - ty[j].ts
				if skew < 0 {
					skew = -skew
				}
				ts := tx[i].ts
				if ty[j].ts < ts {
					ts = ty[j].ts
				}
				w.conSet = true
				w.conTs, w.conKhzA, w.conKhzB, w.conSkewSec = ts, tx[i].khz, ty[j].khz, skew
			}
			break
		}
		// near-miss factor: tightest same-value cross gap among unmatched
		// transitions — a per-value LINEAR rescan over the unmatched map
		// above (复核 F6: disclosure factor only, the unmatched remainder is
		// small on real DVFS streams; never a criterion input).
		for _, yts := range unmatchedYByVal[tx[i].khz] {
			gap := tx[i].ts - yts
			if gap < 0 {
				gap = -gap
			}
			if !w.nearSkewSet || gap < w.nearSkewSec {
				w.nearSkewSet, w.nearSkewSec = true, gap
			}
		}
	}
	return w
}

// freqWitnessCoMoveDiag is the pair-level witness verdict plus its split
// localization (the split result is meaningful only when ok=false):
//
//	con > 0                          → split, transition_conflict (one-vote veto)
//	pro ≥ clusterFreqCoWitnessFloor  → merge
//	otherwise                        → split, co_witness_floor
func freqWitnessCoMoveDiag(a, b []freqSample) (bool, freqCoMoveSplit) {
	if len(a) == 0 || len(b) == 0 {
		return false, freqCoMoveSplit{arm: freqCoMoveSplitArmEmpty}
	}
	w := freqWitnessScanPair(a, b)
	if w.con > 0 {
		return false, freqCoMoveSplit{arm: freqCoMoveSplitArmConflict, ts: w.conTs,
			conKhzA: w.conKhzA, conKhzB: w.conKhzB, conSkewSec: w.conSkewSec, pro: w.pro}
	}
	if w.pro >= clusterFreqCoWitnessFloor {
		return true, freqCoMoveSplit{}
	}
	split := freqCoMoveSplit{arm: freqCoMoveSplitArmFloor, pro: w.pro,
		nearSkewSec: w.nearSkewSec, nearSkewSet: w.nearSkewSet}
	if w.unpairedSet {
		split.ts = w.unpairedTs
	} else {
		// No transitions at all on at least one side — localize at the later
		// side's entry announcement (the pre-batch floor localization anchor).
		x, y := freqStateChangePoints(a), freqStateChangePoints(b)
		split.ts = x[0].ts
		if y[0].ts > split.ts {
			split.ts = y[0].ts
		}
	}
	return false, split
}

// clusterFreqCoWitnessFloor is the witness-lane merge's evidence floor
// (CLUSTERSTREAM-1 §29.193.1): at least 2 co-witnessed TRANSITIONS (pro
// witnesses; entry announcements never count — 公告不铸见证). Same constant
// and same argument lineage as the retired trimmed-form floor
// clusterFreqTrimmedMinAligned (§28.5 复核 P1: a single coincident value is
// not co-movement — two foreign clusters parked at one value must never fuse)
// and as clusterFreqComoveMinSamples. pro==1 does NOT merge (地板臂): one
// coincident same-value transition across two independent clusters is a
// plausible accident; two with zero contradictions is the ruled evidence bar.
// EVOLUTION RECORD (§29.193.1, 2026-07-21): renamed from
// clusterFreqTrimmedMinAligned — the value (2) and the P1 rationale carry
// over; the counting caliber tightens (announcement pairs no longer count
// toward the floor).
const clusterFreqCoWitnessFloor = 2

func removeInt(in []int, v int) []int {
	out := in[:0]
	for _, x := range in {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

// known reports whether an explicit topology was parsed at all.
func (d clusterFreqDomains) known() bool { return len(d.byCPU) > 0 }

// donorFor is THE cluster-shared frequency resolution (single authority, see
// the file header). It returns the lowest-numbered same-domain sibling for
// which hasSamples is true. Fail-open rules, all precise:
//
//	hasSamples(cpu) true  → no donor (禁反向: own samples always win),
//	membership unknown    → no donor (explicit: not in the map; derived:
//	                        above the highest sampled core — 向上不外推),
//	no sampled sibling    → no donor.
func (d clusterFreqDomains) donorFor(cpu int, hasSamples func(int) bool) (int, bool) {
	if hasSamples == nil || hasSamples(cpu) {
		return 0, false
	}
	for _, sibling := range d.domainMembersFor(cpu) {
		if sibling == cpu {
			continue
		}
		if hasSamples(sibling) {
			return sibling, true
		}
	}
	return 0, false
}

// domainMembersFor resolves cpu's domain roster — direct map membership only.
// R6 (规则1/规则3): the derived form now closes leading and enclosed
// sample-less cores into the membership map itself, so the former 向下继承
// on-demand inheritance arm is retired; a core outside every closed interval
// (cross-cluster gap, or above the highest sampled core — 向上不外推) gets
// NOTHING and every consumer fails open.
func (d clusterFreqDomains) domainMembersFor(cpu int) []int {
	if label, ok := d.byCPU[cpu]; ok {
		return d.members[label]
	}
	return nil
}

// derivedDomainLabelFor exposes the derived-form membership declaration for
// disclosure and pins: the group label for member cores (R6 规则1/规则3
// closures included), the exclusionary prime label for cores above the
// highest sampled core when ≥3 domains were derived (超大核 rule — declared,
// never donated), "" otherwise (cross-cluster gap cores stay honestly
// unassigned — R6 retired the 向下继承 arm).
func (d clusterFreqDomains) derivedDomainLabelFor(cpu int) string {
	if d.source != ClusterFreqSourceDerived {
		return ""
	}
	if label, ok := d.byCPU[cpu]; ok {
		return label
	}
	if len(d.sampledAsc) > 0 && cpu > d.sampledAsc[len(d.sampledAsc)-1] && d.groupCount >= 3 {
		return clusterFreqDerivedPrimeLabel
	}
	return ""
}

// clusterFreqDonorResolver memoizes donorFor over one query's per-CPU sample
// availability, and records every pair actually resolved so the window face
// can disclose them in one deterministic caveat. Resolution NEVER mutates the
// underlying sample map — reuse is a read-time alias, the sampling facts stay
// as collected.
type clusterFreqDonorResolver struct {
	domains    clusterFreqDomains
	hasSamples func(int) bool
	memo       map[int]int
	missMemo   map[int]bool
	// primeMemo (CFR-2 verify P3-3) records consulted cores that hit the
	// derived prime pseudo-domain (above the highest sampled core, ≥3
	// domains) — the caveat face discloses them so the 超大核 ruling is
	// visible in the answer, not just declared internally.
	primeMemo map[int]bool
}

// newClusterFreqDonorResolver takes the ALREADY-resolved domains (single
// entry: resolveClusterFreqDomains — explicit topology first, CFR-2
// change-point derivation in its absence).
func newClusterFreqDonorResolver(domains clusterFreqDomains, hasSamples func(int) bool) *clusterFreqDonorResolver {
	return &clusterFreqDonorResolver{
		domains:    domains,
		hasSamples: hasSamples,
		memo:       map[int]int{},
		missMemo:   map[int]bool{},
		primeMemo:  map[int]bool{},
	}
}

// sourceToken exposes the membership source for the disclosure faces.
func (r *clusterFreqDonorResolver) sourceToken() string { return r.domains.source }

// explicitIgnored exposes the P3-4 unparseable-input flag for the caveat face.
func (r *clusterFreqDonorResolver) explicitIgnored() bool { return r.domains.explicitInputIgnored }

func (r *clusterFreqDonorResolver) donorFor(cpu int) (int, bool) {
	if donor, ok := r.memo[cpu]; ok {
		return donor, true
	}
	if r.missMemo[cpu] {
		return 0, false
	}
	donor, ok := r.domains.donorFor(cpu, r.hasSamples)
	if !ok {
		r.missMemo[cpu] = true
		if r.domains.derivedDomainLabelFor(cpu) == clusterFreqDerivedPrimeLabel {
			r.primeMemo[cpu] = true
		}
		return 0, false
	}
	r.memo[cpu] = donor
	return donor, true
}

// primeCPUs returns the consulted cores declared into the derived prime
// pseudo-domain, sorted — the disclosure roster for the 超大核 clause.
func (r *clusterFreqDonorResolver) primeCPUs() []int {
	if len(r.primeMemo) == 0 {
		return nil
	}
	out := make([]int, 0, len(r.primeMemo))
	for cpu := range r.primeMemo {
		out = append(out, cpu)
	}
	sort.Ints(out)
	return out
}

// usedPairs returns the resolved (cpu, donor) pairs sorted by cpu — the
// deterministic disclosure roster.
func (r *clusterFreqDonorResolver) usedPairs() [][2]int {
	if len(r.memo) == 0 {
		return nil
	}
	out := make([][2]int, 0, len(r.memo))
	for cpu, donor := range r.memo {
		out = append(out, [2]int{cpu, donor})
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// caveatListContains reports whether needle appears verbatim in haystack
// (caveat dedupe between the window-stats and scheduler-latency faces).
func caveatListContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// clusterFreqReuseCaveat renders the single disclosure caveat for a face that
// consumed donor timelines. One shared formatter so the window-stats and
// scheduler-latency faces cannot drift on wording; the source variant says
// WHERE the cluster membership came from (CFR-2 披露区分). "" when nothing
// was reused. The explicit-source wording is byte-identical to the CFR #75
// original (pinned). Derived-lane companions (CFR-2 verify round):
//
//	primeCPUs (P3-3)        — consulted cores declared into the derived
//	                          超大核 pseudo-domain get their own clause: the
//	                          user ruling's "更高的核推导为超大核" is now
//	                          answer-face visible, together with the fact
//	                          that they were NOT reused;
//	explicitIgnored (P3-4)  — a non-empty core_topology that parsed to
//	                          nothing must not be narrated as "no explicit
//	                          core_topology" (静默换道禁止).
func clusterFreqReuseCaveat(pairs [][2]int, source string, primeCPUs []int, explicitIgnored bool) string {
	if len(pairs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		parts = append(parts, "cpu"+strconv.Itoa(pair[0])+"←cpu"+strconv.Itoa(pair[1]))
	}
	if source == ClusterFreqSourceDerived {
		head := "cluster-shared frequency reuse from freq-change-point derived clusters (no explicit core_topology): "
		if explicitIgnored {
			head = "cluster-shared frequency reuse from freq-change-point derived clusters (core_topology input had no recognizable cluster labels — fell back to change-point derivation / core_topology 输入未能解析(无可识别簇标签),已按频点变化点推导): "
		}
		caveat := head + strings.Join(parts, ",") +
			" — sampled cores whose full-trace cpu_frequency change curves agree form one cluster; cores below the first sampled core join the first cluster and sample-less cores enclosed by a cluster's member interval join that cluster (R6 首簇/区间闭包); cores between two clusters or above the highest sampled core are never extrapolated; frequency-weighted faces only, raw sampling facts are not rewritten (簇共频复用:无显式拓扑输入生效,按全文件频点变化曲线推导同簇——曲线一致合并、0核至首个有频点核同属首簇、簇成员区间内无样本核并入该簇(R6 规则1/规则3)、簇间与最高采样核以上不外推;仅影响频率加权面,原始采样事实不改写)"
		if len(primeCPUs) > 0 {
			primes := make([]string, 0, len(primeCPUs))
			for _, cpu := range primeCPUs {
				primes = append(primes, "cpu"+strconv.Itoa(cpu))
			}
			caveat += ";" + strings.Join(primes, ",") + " 高于最高采样核,按裁定推导为超大核域(无采样成员,不复用,原样保留无频点口径)(cores above the highest sampled core form the derived prime domain: declared, never donated — their 无频点数据 accounting stands)"
		}
		return caveat
	}
	return "cluster-shared frequency reuse under explicit core_topology: " + strings.Join(parts, ",") +
		" — cores without own cpu_frequency samples take a same-cluster sampled core's timeline on frequency-weighted faces only; raw sampling facts (frequency residency / census / ceilings membership) are not rewritten (簇共频复用:无自身频点采样的核按同簇采样核频点折算,仅影响频率加权面,原始采样事实不改写)"
}
