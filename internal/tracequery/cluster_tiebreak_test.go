package tracequery

// cluster_tiebreak_test.go — CLUSTERTIE-1 (§29.197①, 2026-07-21) pin family:
// the fmax tie-break chain. The customer verdict form (cust_report_xx.txt
// witness): streaming co-movement JUDGES the clusters, then two clusters'
// three-lane fmax values tie and the whole judgment used to die on the 禁掷币
// arm (capRatio=1 + 13 rows of 「核类排序不可判」). The chain orders the tied
// pair from precise signals — limits_rank / depressed_observed_rank /
// value_set_rank, tried in the ruled order, all broken = honest tie retained.
//
// Pins here: one positive arm per chain (order + classes + audit format), the
// all-broken honest-tie arm, the 禁计数比较 negative arm (value-set RICHNESS
// never orders), the ≥3-run honest restraint, the swap arm, and the
// progressive-one-way guarantee (untied judged verdicts and every freq_only
// arm carry an EMPTY tie-break audit — the chain only ever runs inside the
// former fail branch).

import (
	"strings"
	"testing"
)

// tieBreakResolve is the shared harness: derive domains from the observed
// timelines (singleton clusters — the pairwise witness criterion keeps the
// deliberately-disjoint lanes apart) and resolve with the given limits lanes.
func tieBreakResolve(t *testing.T, timelines, limits map[int][]freqSample) coreCapabilityMap {
	t.Helper()
	domains := deriveClusterFreqDomains(timelines)
	if domains.groupCount < 2 {
		t.Fatalf("fixture must derive ≥2 clusters, got %+v", domains.members)
	}
	return resolveCoreCapabilityEvidence(domains, timelines, limits, nil)
}

// Chain (a) limits_rank: both observed peaks sit at one value ABOVE two
// differing limits ceilings (整文件被压/stale-limits shape) — the rated
// ceilings order the classes.
func TestClusterTieBreakLimitsRank(t *testing.T) {
	timelines := map[int][]freqSample{
		0: {{ts: 1.0, khz: 2400000}},
		4: {{ts: 2.0, khz: 2400000}},
	}
	limits := map[int][]freqSample{
		0: {{ts: 0.5, khz: 2200000}},
		4: {{ts: 0.5, khz: 2295000}},
	}
	capability := tieBreakResolve(t, timelines, limits)
	if capability.source != CoreCapabilitySourceDefault || capability.freqOnlyReason != "" {
		t.Fatalf("limits_rank must break the tie into a judged verdict, got %q/%q", capability.source, capability.freqOnlyReason)
	}
	if got := capability.classByCluster["derived_c0"]; got != coreCapabilityClassSmall {
		t.Fatalf("the lower-limits cluster must take the lower class, got %q", got)
	}
	if got := capability.classByCluster["derived_c1"]; got != coreCapabilityClassBig {
		t.Fatalf("the higher-limits cluster must take the higher class, got %q", got)
	}
	// The tied fmax VALUE is untouched — only the order was judged.
	if capability.fmaxByCluster["derived_c0"] != 2400000 || capability.fmaxByCluster["derived_c1"] != 2400000 {
		t.Fatalf("tie-break must not rewrite the fmax values, got %+v", capability.fmaxByCluster)
	}
	for _, needle := range []string{"derived_c0↔derived_c1", "fmax=2400000kHz",
		"破局链=" + coreCapabilityTieBreakChainLimits, "限频上界分簇序", "2200000kHz vs 2295000kHz"} {
		if !strings.Contains(capability.fmaxTieBreakAudit, needle) {
			t.Fatalf("tie-break audit missing %q: %q", needle, capability.fmaxTieBreakAudit)
		}
	}
	// capRatio upgrade (件4 value channel): the weak cluster prices at
	// small/big instead of the freq_only flat 1.
	if cap0 := capability.capByCluster["derived_c0"]; cap0 != coreCapabilityDefaultSmall {
		t.Fatalf("weak cluster cap must price small=1.0, got %v", cap0)
	}
	if cap1 := capability.capByCluster["derived_c1"]; cap1 != coreCapabilityDefaultBig {
		t.Fatalf("top cluster cap must price big=2.53, got %v", cap1)
	}
}

// Chain (a) swap arm: the SAME shape with the limits ceilings exchanged must
// order the pair the other way (the sorted label order is not the verdict).
func TestClusterTieBreakLimitsRankSwap(t *testing.T) {
	timelines := map[int][]freqSample{
		0: {{ts: 1.0, khz: 2400000}},
		4: {{ts: 2.0, khz: 2400000}},
	}
	limits := map[int][]freqSample{
		0: {{ts: 0.5, khz: 2295000}},
		4: {{ts: 0.5, khz: 2200000}},
	}
	capability := tieBreakResolve(t, timelines, limits)
	if capability.source != CoreCapabilitySourceDefault {
		t.Fatalf("swap arm must still judge, got %q", capability.source)
	}
	if capability.classByCluster["derived_c0"] != coreCapabilityClassBig ||
		capability.classByCluster["derived_c1"] != coreCapabilityClassSmall {
		t.Fatalf("classes must follow the limits order, not the label order: %+v", capability.classByCluster)
	}
	// Audit names the low side first regardless of label order.
	if !strings.Contains(capability.fmaxTieBreakAudit, "derived_c1↔derived_c0") ||
		!strings.Contains(capability.fmaxTieBreakAudit, "2200000kHz vs 2295000kHz") {
		t.Fatalf("swap audit must present low↔high, got %q", capability.fmaxTieBreakAudit)
	}
}

// Chain (b) depressed_observed_rank: equal limits ceilings, one cluster's
// observed peak flattened by an in-force press window — excluding the pressed
// samples separates the true peaks (去热限窗观测法).
func TestClusterTieBreakDepressedObservedRank(t *testing.T) {
	timelines := map[int][]freqSample{
		// c0: unpressed peak 2200000 @5.5, pressed sample 1720000 @7.0.
		0: {{ts: 5.5, khz: 2200000}, {ts: 7.0, khz: 1720000}},
		// c1: unpressed peak 2270000 (== both clusters' limits ceiling).
		4: {{ts: 5.5, khz: 2270000}},
	}
	limits := map[int][]freqSample{
		0: {{ts: 5.0, khz: 2270000}, {ts: 6.0, khz: 1720000}},
		4: {{ts: 5.0, khz: 2270000}},
	}
	capability := tieBreakResolve(t, timelines, limits)
	if capability.source != CoreCapabilitySourceDefault {
		t.Fatalf("depressed-observed rank must break the tie, got %q/%q", capability.source, capability.freqOnlyReason)
	}
	if capability.classByCluster["derived_c0"] != coreCapabilityClassSmall ||
		capability.classByCluster["derived_c1"] != coreCapabilityClassBig {
		t.Fatalf("de-pressed order must be c0<c1: %+v", capability.classByCluster)
	}
	for _, needle := range []string{"破局链=" + coreCapabilityTieBreakChainDepressed,
		"去热限窗实测序", "2200000kHz vs 2270000kHz"} {
		if !strings.Contains(capability.fmaxTieBreakAudit, needle) {
			t.Fatalf("depressed audit missing %q: %q", needle, capability.fmaxTieBreakAudit)
		}
	}
}

// Chain (c) value_set_rank: the tie value came from the limits lane on both
// sides while the observed value sets stratify (§29.193① second signal —
// one side holds a value above the other's ENTIRE set). Chain (b) is broken
// first because every observed sample of the weak side is pressed.
func TestClusterTieBreakValueSetRank(t *testing.T) {
	timelines := map[int][]freqSample{
		// c0: only observed sample sits inside its press window → de-pressed
		// max 0 (chain (b) inapplicable), observed set {1500000}.
		0: {{ts: 7.0, khz: 1500000}},
		// c1: observed set {2200000} — above c0's entire set.
		4: {{ts: 5.5, khz: 2200000}},
	}
	limits := map[int][]freqSample{
		0: {{ts: 5.0, khz: 2270000}, {ts: 6.0, khz: 1500000}},
		4: {{ts: 5.0, khz: 2270000}},
	}
	// Fixture sanity: chains (a)/(b) really are broken.
	if la, lb := clusterLimitsLaneMax([]int{0}, limits), clusterLimitsLaneMax([]int{4}, limits); la != lb {
		t.Fatalf("fixture: limits ceilings must tie, got %d vs %d", la, lb)
	}
	if da := clusterDepressedObservedMax([]int{0}, timelines, limits); da != 0 {
		t.Fatalf("fixture: c0's de-pressed max must be 0 (all samples pressed), got %d", da)
	}
	capability := tieBreakResolve(t, timelines, limits)
	if capability.source != CoreCapabilitySourceDefault {
		t.Fatalf("value-set rank must break the tie, got %q/%q", capability.source, capability.freqOnlyReason)
	}
	if capability.classByCluster["derived_c0"] != coreCapabilityClassSmall ||
		capability.classByCluster["derived_c1"] != coreCapabilityClassBig {
		t.Fatalf("value-set order must be c0<c1: %+v", capability.classByCluster)
	}
	for _, needle := range []string{"破局链=" + coreCapabilityTieBreakChainValueSet,
		"频点值集分层序", "1500000kHz vs 2200000kHz"} {
		if !strings.Contains(capability.fmaxTieBreakAudit, needle) {
			t.Fatalf("value-set audit missing %q: %q", needle, capability.fmaxTieBreakAudit)
		}
	}
}

// All chains broken = the honest tie stands byte-for-byte: freq_only,
// fmax_tie reason, split-audit localization, EMPTY tie-break audit (真同
// fmax 异算力 SoC and same-signal fragment twins keep the fail-loud arm).
func TestClusterTieBreakAllBrokenHonestTie(t *testing.T) {
	timelines := map[int][]freqSample{
		0: {{ts: 1.0, khz: 2000000}},
		4: {{ts: 2.0, khz: 2000000}},
	}
	capability := tieBreakResolve(t, timelines, nil)
	if capability.source != CoreCapabilitySourceFreqOnly ||
		capability.freqOnlyReason != CoreCapabilityFreqOnlyReasonFmaxTie {
		t.Fatalf("all-broken must keep the honest tie, got %q/%q", capability.source, capability.freqOnlyReason)
	}
	if capability.fmaxTieBreakAudit != "" {
		t.Fatalf("honest tie must carry no tie-break audit, got %q", capability.fmaxTieBreakAudit)
	}
}

// 禁计数比较 negative arm: value-set RICHNESS (more distinct values, more
// samples) is a noisy signal and must never order the pair — equal maxima
// with unequal richness stays the honest tie.
func TestClusterTieBreakRichnessNeverOrders(t *testing.T) {
	timelines := map[int][]freqSample{
		0: {{ts: 1.0, khz: 800000}, {ts: 2.0, khz: 900000}, {ts: 3.0, khz: 1000000},
			{ts: 4.0, khz: 1100000}, {ts: 5.0, khz: 2000000}},
		4: {{ts: 2.5, khz: 2000000}},
	}
	capability := tieBreakResolve(t, timelines, nil)
	if capability.source != CoreCapabilitySourceFreqOnly ||
		capability.freqOnlyReason != CoreCapabilityFreqOnlyReasonFmaxTie {
		t.Fatalf("the richer value set must NOT win the order, got %q/%q", capability.source, capability.freqOnlyReason)
	}
	if capability.fmaxTieBreakAudit != "" {
		t.Fatalf("richness arm must not mint a tie-break audit, got %q", capability.fmaxTieBreakAudit)
	}
}

// ≥3-way run restraint: per-pair chain verdicts need not be transitive across
// a longer run — a 3-way tie keeps the honest arm even when pairwise chains
// would fire (the limits ceilings below all differ).
func TestClusterTieBreakThreeWayRunStaysHonest(t *testing.T) {
	timelines := map[int][]freqSample{
		0: {{ts: 1.0, khz: 2000000}},
		4: {{ts: 2.0, khz: 2000000}},
		8: {{ts: 3.0, khz: 2000000}},
	}
	limits := map[int][]freqSample{
		0: {{ts: 0.5, khz: 1800000}},
		4: {{ts: 0.5, khz: 1900000}},
		8: {{ts: 0.5, khz: 1850000}},
	}
	capability := tieBreakResolve(t, timelines, limits)
	if capability.source != CoreCapabilitySourceFreqOnly ||
		capability.freqOnlyReason != CoreCapabilityFreqOnlyReasonFmaxTie {
		t.Fatalf("a 3-way tie run must stay the honest tie, got %q/%q", capability.source, capability.freqOnlyReason)
	}
	if capability.fmaxTieBreakAudit != "" {
		t.Fatalf("3-way run must not mint a tie-break audit, got %q", capability.fmaxTieBreakAudit)
	}
}

// Progressive one-way (件4 渐进单向): an untied judged verdict never carries
// the audit — the chain only ever runs inside the former fail branch, so
// every currently-judged surface is byte-identical by construction.
func TestClusterTieBreakUntiedJudgedCarriesNoAudit(t *testing.T) {
	timelines := map[int][]freqSample{
		0: {{ts: 1.0, khz: 1720000}},
		4: {{ts: 2.0, khz: 2270000}},
	}
	capability := tieBreakResolve(t, timelines, nil)
	if capability.source != CoreCapabilitySourceDefault {
		t.Fatalf("distinct fmax must judge without any tie machinery, got %q", capability.source)
	}
	if capability.fmaxTieBreakAudit != "" {
		t.Fatalf("untied judged verdict must carry no tie-break audit, got %q", capability.fmaxTieBreakAudit)
	}
}

// Wire ride + caveat lift: the audit reaches SupplyFoldBasis on judged
// verdicts and the engine caveat lane names the chain family; untied bases
// stay caveat-silent (absence preserves every existing byte).
func TestClusterTieBreakAuditCaveatLift(t *testing.T) {
	res := Result{RootCauseRank: &RootCauseRankResult{Items: []RootCauseRankItem{{
		SupplyFoldBasis: &SupplyFoldBasis{
			CapabilitySource:        CoreCapabilitySourceDefault,
			CapabilityTieBreakAudit: "derived_c0↔derived_c1 fmax=2400000kHz 破局链=limits_rank(限频上界分簇序:2200000kHz vs 2295000kHz)",
		},
	}}}}
	got := capabilityTieBreakAuditCaveat(res)
	for _, needle := range []string{"capability_fmax_tie_break=derived_c0↔derived_c1",
		"破局链=limits_rank", "仅披露/审计用", "disclosure/audit only, never a gate"} {
		if !strings.Contains(got, needle) {
			t.Fatalf("caveat missing %q: %q", needle, got)
		}
	}
	if got := capabilityTieBreakAuditCaveat(Result{RootCauseRank: &RootCauseRankResult{
		Items: []RootCauseRankItem{{SupplyFoldBasis: &SupplyFoldBasis{}}},
	}}); got != "" {
		t.Fatalf("no audit → no caveat, got %q", got)
	}
}
