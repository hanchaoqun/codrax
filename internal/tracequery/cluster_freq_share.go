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
//   - Reuse activates ONLY under an EXPLICIT Query.CoreTopology declaration.
//     The frequency-tier INFERENCE (inferCoreTopologyFromFrequency) is a noisy
//     signal and can never classify an unsampled core anyway — inferred or
//     unknown topology = fail-open, behavior unchanged (no reuse, the honest
//     无频点数据 caveats stay).
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

// clusterFreqDomains is the parsed explicit frequency-domain map: cpu →
// verbatim (lowercased) topology label, plus the ascending member roster per
// label. Zero value = topology unknown = every donorFor call fails open.
type clusterFreqDomains struct {
	byCPU   map[int]string
	members map[string][]int
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
	return out
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
//	cpu not in the map    → no donor (membership unknown),
//	no sampled sibling    → no donor.
func (d clusterFreqDomains) donorFor(cpu int, hasSamples func(int) bool) (int, bool) {
	if hasSamples == nil || hasSamples(cpu) {
		return 0, false
	}
	label, ok := d.byCPU[cpu]
	if !ok {
		return 0, false
	}
	for _, sibling := range d.members[label] {
		if sibling == cpu {
			continue
		}
		if hasSamples(sibling) {
			return sibling, true
		}
	}
	return 0, false
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
}

func newClusterFreqDonorResolver(rawTopology string, hasSamples func(int) bool) *clusterFreqDonorResolver {
	return &clusterFreqDonorResolver{
		domains:    parseClusterFreqDomains(rawTopology),
		hasSamples: hasSamples,
		memo:       map[int]int{},
		missMemo:   map[int]bool{},
	}
}

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
		return 0, false
	}
	r.memo[cpu] = donor
	return donor, true
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
// scheduler-latency faces cannot drift on wording. "" when nothing was reused.
func clusterFreqReuseCaveat(pairs [][2]int) string {
	if len(pairs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		parts = append(parts, "cpu"+strconv.Itoa(pair[0])+"←cpu"+strconv.Itoa(pair[1]))
	}
	return "cluster-shared frequency reuse under explicit core_topology: " + strings.Join(parts, ",") +
		" — cores without own cpu_frequency samples take a same-cluster sampled core's timeline on frequency-weighted faces only; raw sampling facts (frequency residency / census / ceilings membership) are not rewritten (簇共频复用:无自身频点采样的核按同簇采样核频点折算,仅影响频率加权面,原始采样事实不改写)"
}
