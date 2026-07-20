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
//     witness). (b) The co-movement criterion (freqTimelinesCoMove) is
//     boundary-cut robust: a carve/cap/gate boundary that straddles ONE
//     multi-row emission burst (padded head ts-gate, padded tail ts-gate,
//     MaxEvents budget cut, window-face TimeEnd cut) must not fragment a
//     cluster into same-fmax twins — that fragmentation was the huadong
//     "簇结构不可判,按纯频率比折算" degrade's mechanical source (engine-real
//     reproduction pinned in core_capability_cap3_test.go). Real mid-stream
//     state divergence (a member row genuinely missing/skewed while the
//     stream continues) still SPLITS — absence never guesses.
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

// deriveClusterFreqDomains implements the R6 four-rule cluster derivation
// (§29.88.9 用户裁定 2026-07-14, docs/design/real_trace_campaign_20260705.md),
// evolving the CFR-2 (#80) trace-native derivation:
//
//	规则2 同簇判据 = 全域频点变化一致 (donghu emission shape, see
//	clusterFreqDeriveMaxSkewSec): two sampled cores belong to one cluster iff
//	their cpu_frequency change curves agree over the whole sample stream —
//	the SAME emission (equal length, per-index equal kHz, per-index
//	timestamps within the skew bound), accepted MODULO the stream's carve
//	boundaries (freqTimelinesCoMove, CAP-3 §29.11): an unwitnessed head
//	region with carry-in state agreement and a single tail change straddled
//	by the stream tail do not split; mid-stream divergence always splits.
//	A canonical zero is a typed offline/unknown transition, not a capacity
//	sample. It remains in the state curve so carry cannot bridge across it;
//	any transition mismatch splits (fail-open — a false split only loses
//	reuse), while fmax/CAP consumers independently require positive values.
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
	// CAP-3: the stream-tail boundary is a GLOBAL property of the sample
	// stream (max sample ts over every sampled CPU) — the tail-straddle
	// exemption in freqTimelinesCoMove keys on it, never on a per-pair value
	// a foreign cluster's silence could fake.
	globalTail := 0.0
	for _, cpu := range sampled {
		tl := timelines[cpu]
		if last := tl[len(tl)-1].ts; last > globalTail {
			globalTail = last
		}
	}
	// Group sampled cores by timeline identity. Representatives are the
	// lowest member of each group (ascending scan keeps this deterministic);
	// identity against the representative suffices because same-cluster
	// timelines are copies of one emission. (CAP-3: with the boundary-trimmed
	// form, co-movement is anchored on the representative — a member joins
	// the FIRST representative it co-moves with; deterministic by the
	// ascending scan.)
	var reps []int
	for _, cpu := range sampled {
		joined := false
		for gi, rep := range reps {
			if freqTimelinesCoMove(timelines[rep], timelines[cpu], globalTail) {
				label := derivedDomainLabel(gi)
				out.byCPU[cpu] = label
				out.members[label] = append(out.members[label], cpu)
				joined = true
				break
			}
		}
		if !joined {
			label := derivedDomainLabel(len(reps))
			reps = append(reps, cpu)
			out.byCPU[cpu] = label
			out.members[label] = append(out.members[label], cpu)
		}
	}
	out.groupCount = len(reps)
	// R6 规则1 (首簇) + 规则3 (区间闭包): sample-less cores become MEMBERS —
	// leading cores join the first sampled core's cluster; enclosed cores join
	// the enclosing cluster. Ascending group order (labels ascend by lowest
	// member) makes any overlap claim deterministic; sampled cores are never
	// reassigned. Trailing cores above the highest sampled core stay out
	// (向上不外推, unchanged).
	for gi := 0; gi < len(reps); gi++ {
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
		integrity := frequencyOrderIntegrityForGlobalDerivation(idx)
		limitsDropped := func(limitTLs map[int][]freqSample, seed []int, limitAll bool, limitUnsafe map[int]bool) []int {
			dropped := append([]int(nil), seed...)
			for cpu := range limitTLs {
				if integrity.limitUnsafe(cpu) || limitAll || limitUnsafe[cpu] {
					if !containsInt(dropped, cpu) {
						dropped = append(dropped, cpu)
					}
				}
			}
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
		var dropped, limitDropped []int
		for _, ev := range idx.Events {
			if cpu, _, _, ok := perCPULimitSampleValues(ev); ok && integrity.limitUnsafe(cpu) {
				// 裁定⑨ window-carve arm: mirror of the buildFreqLimitIndex
				// events-fallback filter (roster only — that filter's drop
				// judgment is untouched).
				if !containsInt(limitDropped, cpu) {
					limitDropped = append(limitDropped, cpu)
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

// freqTimelinesCoMove is the CAP-3 (§29.11) co-movement criterion: the exact
// same-emission identity above (fast path, historical semantics byte-for-byte
// — including its constant-equal-value merge form), OR the boundary-trimmed
// STATE alignment (freqStateCoMoveTrimmed). globalTailTs is the sample
// stream's tail boundary (max sample ts across ALL sampled CPUs of this
// derivation) — the tail-straddle exemption's anchor.
func freqTimelinesCoMove(a, b []freqSample, globalTailTs float64) bool {
	if freqTimelinesSameEmission(a, b) {
		return true
	}
	return freqStateCoMoveTrimmed(a, b, globalTailTs)
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

// freqStateCoMoveTrimmed is the CAP-3 boundary-trimmed state-alignment form
// of the 同域判据. It exists because the fold face's Index is a padded CARVE
// of the raw trace under an event budget: the padded-head ts gate, the
// padded-tail ts gate, the MaxEvents budget cut and the window-face TimeEnd
// cut each land at a µs-arbitrary point of the event stream, and any of them
// straddling ONE multi-row emission burst made same-cluster timelines differ
// by exactly one boundary row — the whole-array identity then fragmented the
// cluster into same-fmax twins and the §26 judgment died on the fmax-tie /
// >4-cluster fail-loud arms (huadong_792 mechanical shape).
//
// Rules — all precise, evaluated on DEDUPED change points:
//
//	HEAD (unwitnessed region): the earlier-starting side's changes BEFORE the
//	later side's first witness (minus skew) are trimmed. The trace carries no
//	claim about the later CPU there (its rows were gate-cut, or its buffer
//	started later), so the region is one-sided evidence, not disagreement.
//	Junction guard: the trimmed side's carried state at the junction must
//	equal the later side's first value when that first change cannot pair
//	directly — a state DISAGREEMENT at first witness splits.
//
//	MIDDLE (both witnessed): strict 1:1 alignment — equal kHz and timestamps
//	within clusterFreqDeriveMaxSkewSec at every change. Any unpaired or
//	skew-violating mid-stream change splits: the state functions genuinely
//	diverge there (a missing member row means the trace CLAIMS that CPU kept
//	the old frequency — merging over it would guess hardware state).
//
//	TAIL (stream-tail straddle): at most ONE unmatched trailing change, on
//	one side only, and only when it sits within the skew bound of the GLOBAL
//	sample-stream tail (globalTailTs) — i.e. it is the stream's very last
//	emission burst, exactly where a budget/gate cut takes member rows. A
//	trailing change deeper inside the stream is real divergence (state
//	carries forward) and splits regardless.
//
//	EVIDENCE FLOOR (CAP-3 复核 P1, 2026-07-10): every trimmed-form merge
//	requires at least clusterFreqTrimmedMinAligned (=2) aligned change
//	pairs. The FIRST aligned pair may be nothing more than both sides' entry
//	ANNOUNCEMENT of a standing value (one all-policy sweep emits first rows
//	within the skew bound) — it witnesses a shared STATE, not a shared
//	TRANSITION. Deduped change points guarantee the SECOND aligned pair is a
//	real co-witnessed value transition, so the floor demands at least one.
//	The original aligned≥1 floor let two foreign clusters PARKED at one
//	value (equal first announcements, different re-announce cadence) fuse
//	and corrupt the §26 cluster count — 复核 REPRO: a 3-cluster trace
//	{active small, middle parked @1430000, big parked @1430000} collapsed
//	to 2 groups and crowned the ACTIVE SMALL cluster as big (cap 2.53)
//	while shipping default_table silently. With the floor at 2 the trimmed
//	form is genuinely no weaker than the historical constant-value merge
//	(which demanded per-index ts agreement over the WHOLE arrays); the
//	residual cost shape — a true same-cluster pair with exactly one
//	co-witnessed transition plus differing cadence — is emission-
//	structurally unreachable (one notifier loop emits all member rows, so
//	same-cluster cadence is identical by construction).
//
// Direction (§29.11 保守方向, per side):
//
//	False SPLIT (criterion too strict) → the fold degrades to the pure
//	frequency ratio with the freq_only disclosure — prices are conservative
//	but the §26 class gap is lost on EVERY row of the query (the huadong
//	regression this batch removes).
//	False MERGE (criterion too loose) → fabricated donor frequencies and a
//	corrupted cluster count. The exemptions are therefore bounded to regions
//	where the trace makes NO claim (head) or where the stream tail provably
//	cut the emission (tail, global anchor + single change + skew bound);
//	mid-stream tolerance stays ZERO.
func freqStateCoMoveTrimmed(a, b []freqSample, globalTailTs float64) bool {
	ok, _ := freqStateCoMoveTrimmedDiag(a, b, globalTailTs)
	return ok
}

// freqCoMoveSplit localizes the comparison that split a pair (CAP-3 复核 P2,
// 2026-07-10): the judging arm token plus the timestamp at the failing
// change. DISCLOSURE/AUDIT INPUT ONLY — no gate may ever read it (the split
// decision itself is the gate; this struct merely says WHERE it fired, so a
// customer replay can distinguish a boundary form from real mid-stream
// divergence, §28.11 four-layer acceptance).
type freqCoMoveSplit struct {
	arm string
	ts  float64
}

// Split-arm tokens (typed vocabulary for the audit face; see
// freqCoMoveSplitArmZH for the display labels).
const (
	freqCoMoveSplitArmEmpty = "no_samples"
	freqCoMoveSplitArmHead  = "head_junction_state_mismatch"
	freqCoMoveSplitArmMid   = "mid_alignment_mismatch"
	freqCoMoveSplitArmFloor = "co_witness_floor"
	freqCoMoveSplitArmTail  = "tail_exemption_unmet"
)

// freqCoMoveSplitArmZH maps a split-arm token to its zh display label (the
// audit wording keeps token+label side by side so the line stays greppable
// AND readable).
func freqCoMoveSplitArmZH(arm string) string {
	switch arm {
	case freqCoMoveSplitArmEmpty:
		return "一侧无采样"
	case freqCoMoveSplitArmHead:
		return "头部接续态不一致"
	case freqCoMoveSplitArmMid:
		return "中段1:1对齐违例"
	case freqCoMoveSplitArmFloor:
		return "共见证变迁不足"
	case freqCoMoveSplitArmTail:
		return "尾部豁免不满足"
	default:
		return ""
	}
}

// freqStateCoMoveTrimmedDiag is the single implementation of the trimmed
// criterion (see the rule/direction documentation above). The split result is
// meaningful only when ok=false.
func freqStateCoMoveTrimmedDiag(a, b []freqSample, globalTailTs float64) (bool, freqCoMoveSplit) {
	if len(a) == 0 || len(b) == 0 {
		return false, freqCoMoveSplit{arm: freqCoMoveSplitArmEmpty}
	}
	x, y := freqStateChangePoints(a), freqStateChangePoints(b)
	// y = earlier-or-equal starting side, x = later side.
	if x[0].ts < y[0].ts {
		x, y = y, x
	}
	// HEAD trim on the earlier side; carry the trimmed state for the junction
	// guard.
	carryKnown := false
	var carry int64
	for len(y) > 0 && y[0].ts < x[0].ts-clusterFreqDeriveMaxSkewSec {
		carryKnown, carry = true, y[0].khz
		y = y[1:]
	}
	aligned := 0
	pairs := func(p, q freqSample) bool {
		if p.khz != q.khz {
			return false
		}
		skew := p.ts - q.ts
		if skew < 0 {
			skew = -skew
		}
		return skew <= clusterFreqDeriveMaxSkewSec
	}
	// Junction: x's first change pairs normally when it can; otherwise it may
	// be x's trace-entry announcement of the standing state — consumable iff
	// the trimmed side's carried state agrees (and ONLY for this first
	// change: mid-stream carry consumption would merge over real divergence).
	if len(y) == 0 || !pairs(x[0], y[0]) {
		if !carryKnown || x[0].khz != carry {
			return false, freqCoMoveSplit{arm: freqCoMoveSplitArmHead, ts: x[0].ts}
		}
		x = x[1:]
	}
	// MIDDLE: strict 1:1 alignment.
	n := len(x)
	if len(y) < n {
		n = len(y)
	}
	for i := 0; i < n; i++ {
		if !pairs(x[i], y[i]) {
			return false, freqCoMoveSplit{arm: freqCoMoveSplitArmMid, ts: x[i].ts}
		}
	}
	aligned = n
	// 复核 P1 floor: ≥2 aligned pairs ⇒ at least one true co-witnessed
	// TRANSITION beyond the entry-announcement pair (see EVIDENCE FLOOR).
	if aligned < clusterFreqTrimmedMinAligned {
		ts := 0.0
		if len(x) > 0 {
			ts = x[0].ts
		} else if len(y) > 0 {
			ts = y[0].ts
		}
		return false, freqCoMoveSplit{arm: freqCoMoveSplitArmFloor, ts: ts}
	}
	// TAIL: at most one unmatched trailing change, on one side, at the global
	// stream tail.
	extra := x[n:]
	if len(y[n:]) > 0 {
		extra = y[n:]
	}
	if len(x[n:]) > 0 && len(y[n:]) > 0 {
		// Both sides having leftovers is impossible after the min() cut above
		// unless n==0 — defensive split.
		return false, freqCoMoveSplit{arm: freqCoMoveSplitArmTail, ts: extra[0].ts}
	}
	if len(extra) == 0 {
		return true, freqCoMoveSplit{}
	}
	if len(extra) > 1 || extra[0].ts < globalTailTs-clusterFreqDeriveMaxSkewSec {
		return false, freqCoMoveSplit{arm: freqCoMoveSplitArmTail, ts: extra[0].ts}
	}
	return true, freqCoMoveSplit{}
}

// clusterFreqTrimmedMinAligned is the trimmed-form merge's co-movement
// evidence floor (see freqStateCoMoveTrimmed EVIDENCE FLOOR): at least 2
// aligned change pairs, i.e. at least one TRUE co-witnessed transition beyond
// a possible shared entry announcement. Same rationale as
// clusterFreqComoveMinSamples — a single coincident value is not co-movement.
// EVOLUTION RECORD (复核 P1, 2026-07-10): supersedes both the aligned≥1
// general floor (parked-constant foreign clusters fused → §26 class
// inversion) and the former tail-arm-only clusterFreqTailCutMinAligned check
// (now subsumed: the general floor runs before the tail exemption).
const clusterFreqTrimmedMinAligned = 2

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
