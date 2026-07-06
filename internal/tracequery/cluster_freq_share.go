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
	// inheritance (CFR-2 #80 lane).
	ClusterFreqSourceDerived = "freq_change_point_derived"
)

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

// deriveClusterFreqDomains implements the CFR-2 (#80) 用户裁定: derive
// frequency domains from the trace itself when no explicit topology exists.
//
//	同域判据 (donghu emission shape, see clusterFreqDeriveMaxSkewSec): two
//	sampled cores belong to one cluster iff their cpu_frequency timelines are
//	the SAME emission — equal length, per-index equal kHz values, per-index
//	timestamps within the skew bound. One DVFS transition writes one row per
//	cluster member, so same-cluster timelines are copies of each other; any
//	mismatch splits (fail-open — a false split only loses reuse).
//
//	向下继承: an unsampled core inherits the domain of the nearest sampled
//	core with a higher-or-equal core number ("≤3 的核都和 3 核一样,依次
//	类推") — resolved on demand in domainMembersFor.
//
//	向上不外推: cores above the highest sampled core are never folded into a
//	sampled domain. With ≥3 derived domains they are declared the
//	超大核 pseudo-domain (clusterFreqDerivedPrimeLabel, no sampled members —
//	structurally donor-less); with fewer domains they stay unassigned. Both
//	forms fail open, keeping the honest 无频点数据 caveats for those cores.
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
	// Group sampled cores by timeline identity. Representatives are the
	// lowest member of each group (ascending scan keeps this deterministic);
	// identity against the representative suffices because same-cluster
	// timelines are copies of one emission.
	var reps []int
	for _, cpu := range sampled {
		joined := false
		for gi, rep := range reps {
			if freqTimelinesSameEmission(timelines[rep], timelines[cpu]) {
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
	return out
}

func derivedDomainLabel(group int) string {
	return "derived_c" + strconv.Itoa(group)
}

// windowFreqSampleTimelines adapts a window-face per-CPU Event collection to
// the freqSample shape the derivation reads (ts + kHz only). Pure conversion
// — the caller's collection caliber (window bounds, admission predicate) is
// taken as-is.
func windowFreqSampleTimelines(freqByCPU map[int][]Event) map[int][]freqSample {
	out := make(map[int][]freqSample, len(freqByCPU))
	for cpu, events := range freqByCPU {
		if len(events) == 0 {
			continue
		}
		tl := make([]freqSample, 0, len(events))
		for _, ev := range events {
			tl = append(tl, freqSample{ts: ev.Ts, khz: ev.Frequency})
		}
		out[cpu] = tl
	}
	return out
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

// domainMembersFor resolves cpu's domain roster. Explicit source: direct map
// membership only. Derived source: sampled members answer from the map;
// unsampled cores inherit the domain of the nearest sampled core with a
// higher-or-equal core number (CFR-2 向下继承); cores above the highest
// sampled core get NOTHING (向上不外推 — the prime pseudo-domain has no
// sampled members by construction).
func (d clusterFreqDomains) domainMembersFor(cpu int) []int {
	if label, ok := d.byCPU[cpu]; ok {
		return d.members[label]
	}
	if d.source != ClusterFreqSourceDerived {
		return nil
	}
	idx := sort.SearchInts(d.sampledAsc, cpu)
	if idx >= len(d.sampledAsc) {
		return nil
	}
	return d.members[d.byCPU[d.sampledAsc[idx]]]
}

// derivedDomainLabelFor exposes the derived-form membership declaration for
// disclosure and pins: the group label for sampled/inherited cores, the
// exclusionary prime label for cores above the highest sampled core when ≥3
// domains were derived (超大核 rule — declared, never donated), "" otherwise.
func (d clusterFreqDomains) derivedDomainLabelFor(cpu int) string {
	if d.source != ClusterFreqSourceDerived {
		return ""
	}
	if label, ok := d.byCPU[cpu]; ok {
		return label
	}
	idx := sort.SearchInts(d.sampledAsc, cpu)
	if idx < len(d.sampledAsc) {
		return d.byCPU[d.sampledAsc[idx]]
	}
	if d.groupCount >= 3 {
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
			" — sampled cores with identical cpu_frequency change-point timelines form one cluster, unsampled cores inherit the nearest higher-numbered sampled core's cluster, cores above the highest sampled core are never extrapolated; frequency-weighted faces only, raw sampling facts are not rewritten (簇共频复用:无显式拓扑输入生效,按频点变化点推导同簇——同变化点时间线合并、未采样核向高核号就近继承、最高采样核以上不外推;仅影响频率加权面,原始采样事实不改写)"
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
