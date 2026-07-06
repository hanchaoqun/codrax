package tracequery

import (
	"context"
	"strings"
	"testing"
)

// cluster_freq_share_test.go — CFR (#75, 客户硬件域裁定) + CFR-2 (#80,
// 变化点推导) pins for cluster-shared frequency reuse. The mutation surface
// this file keeps red:
//
//   - 无据复用 (reuse without ANY membership evidence — no explicit topology
//     and nothing derivable) — TestSupplyFoldNoFrequencyDataAllUnknown plus
//     the above-max fail-open arms of the derived tests go red; the CFR-1
//     "explicit-only gate" pins evolved into the derived-lane pins under the
//     #80 user ruling (see the CFR-2 section below);
//   - 簇判定绕过 (normalized-class collapse: big/large/prime folded into one
//     frequency domain) — TestSupplyFoldClusterReusePrimeVsBigDistinct and
//     TestParseClusterFreqDomainsVerbatimLabels go red;
//   - 禁反向 (donor overriding a core's OWN samples) —
//     TestClusterFreqDonorForDiscipline and
//     TestSupplyFoldClusterReuseOwnSamplesNeverOverridden go red;
//   - 跨簇复用 — TestSupplyFoldClusterReuseCrossClusterNever goes red;
//   - VS-2 identity (ideal+deficit == RunningMs) must survive the reuse —
//     asserted inside the b3 pin.

// --- helper-level pins (single authority: cluster_freq_share.go) -----------

func TestParseClusterFreqDomainsVerbatimLabels(t *testing.T) {
	d := parseClusterFreqDomains("big=4-6;prime=7;small=0-3")
	if !d.known() {
		t.Fatalf("explicit topology must parse: %+v", d)
	}
	// Verbatim label identity: big and prime are DISTINCT frequency domains
	// even though normalizeCoreClass folds both into the "big" display class
	// (超大核簇 counterexample — cross-domain reuse would fabricate hardware
	// state).
	if d.byCPU[4] != "big" || d.byCPU[7] != "prime" {
		t.Fatalf("labels must stay verbatim, got %+v", d.byCPU)
	}
	if got := d.members["big"]; len(got) != 3 || got[0] != 4 || got[2] != 6 {
		t.Fatalf("big domain roster wrong: %v", got)
	}
	if got := d.members["prime"]; len(got) != 1 || got[0] != 7 {
		t.Fatalf("prime domain roster wrong: %v", got)
	}
	// Admission parity with parseCoreTopology: unrecognized labels never arm
	// the reuse gate.
	if d := parseClusterFreqDomains("cluster0=0-3;cluster1=4-7"); d.known() {
		t.Fatalf("unrecognized labels must not arm the gate: %+v", d)
	}
	if d := parseClusterFreqDomains(""); d.known() {
		t.Fatalf("empty topology must fail open: %+v", d)
	}
}

func TestClusterFreqDonorForDiscipline(t *testing.T) {
	d := parseClusterFreqDomains("middle=2-4;big=7")
	sampled := map[int]bool{2: true, 4: true, 7: true}
	has := func(cpu int) bool { return sampled[cpu] }
	// Lowest-numbered sampled sibling wins (deterministic disclosure).
	if donor, ok := d.donorFor(3, has); !ok || donor != 2 {
		t.Fatalf("cpu3 must take lowest sampled sibling cpu2, got %d/%v", donor, ok)
	}
	// 禁反向 (structural): a sampled core NEVER takes a donor, even with a
	// sampled sibling present.
	if donor, ok := d.donorFor(2, has); ok {
		t.Fatalf("sampled core must never take a donor, got %d", donor)
	}
	// Membership unknown → fail open.
	if _, ok := d.donorFor(9, has); ok {
		t.Fatalf("unclassified cpu must fail open")
	}
	// Cross-domain: big cpu7 sampled, but cpu3's domain is middle — a
	// middle-only outage must not borrow from big.
	if donor, ok := parseClusterFreqDomains("middle=3;big=7").donorFor(3, has); ok {
		t.Fatalf("cross-cluster reuse is forbidden, got donor %d", donor)
	}
	// No sampled sibling in the domain → fail open.
	if _, ok := d.donorFor(3, func(cpu int) bool { return cpu == 7 }); ok {
		t.Fatalf("domain without sampled sibling must fail open")
	}
	// Zero-value domains (unknown topology) → fail open.
	if _, ok := (clusterFreqDomains{}).donorFor(3, has); ok {
		t.Fatalf("unknown topology must fail open")
	}
}

// --- VS-2 fold-face pins (b3 shape and its mutations) -----------------------

// clusterReuseB3Fixture is the customer's b3 shape: the dependency runs 10ms
// on cpu3, which carries NO cpu_frequency samples of its own; same-cluster
// cpu2 is sampled at 1GHz; the big core cpu7 is sampled at 2GHz (fold fmax).
const clusterReuseB3Fixture = `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2000000 cpu_id=7
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [003] .... 5.000000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [003] .... 5.009900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [003] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`

// The b3 pin: with explicit topology naming cpu2/cpu3 one cluster, the
// unsampled-cpu3 slice folds with cpu2's 1GHz samples against the 2GHz big
// fmax — the fold SUCCEEDS on a fully-known basis (the display's honest
// "频点数据不全,无法折算" branch structurally cannot render: it keys on
// UnknownMs>0), the deficit is the true 5ms, and the reuse is disclosed.
func TestSupplyFoldClusterSharedReuseB3Shape(t *testing.T) {
	idx := buildTraceIndex(t, "cfr_b3_reuse.systrace", clusterReuseB3Fixture)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05,
		TraceFlavorHint: TraceFlavorHarmonyHitrace, CoreTopology: "middle=2-3;big=7"})
	dep := supplyFoldDepImpact(t, chain)
	if dep.SupplyFoldBasis == nil {
		t.Fatalf("fold must run: %+v", dep)
	}
	basis := dep.SupplyFoldBasis
	if !basis.AllKnown() || basis.UnknownMs != 0 {
		t.Fatalf("cluster-shared reuse must yield a fully-known basis (b3 caveat gone), got %+v", basis)
	}
	// ~10ms @1GHz (reused from cpu2) against big fmax 2GHz → ~5ms deficit.
	if dep.SupplyFoldDeficitMs < 4.7 || dep.SupplyFoldDeficitMs > 5.3 {
		t.Fatalf("reused slice must fold at the sibling's 1GHz (deficit ~5ms), got %.3f", dep.SupplyFoldDeficitMs)
	}
	// §7.10 red line: the identity survives the reuse.
	if got, want := dep.SupplyFoldIdealMs+dep.SupplyFoldDeficitMs, dep.RunningMs; !floatNear(got, want) {
		t.Fatalf("ideal+deficit must reconstruct RunningMs: %.6f != %.6f", got, want)
	}
	if got, want := basis.KnownMs+basis.UnknownMs, dep.RunningMs; !floatNear(got, want) {
		t.Fatalf("basis split must cover RunningMs: %.6f != %.6f", got, want)
	}
	// Typed disclosure: exactly the cpu3←cpu2 pair.
	if len(basis.ClusterFreqReuse) != 1 || basis.ClusterFreqReuse[0] != (SupplyFoldClusterReuse{CPU: 3, DonorCPU: 2}) {
		t.Fatalf("reuse must disclose cpu3←cpu2, got %+v", basis.ClusterFreqReuse)
	}
	if basis.FmaxKHz != 2000000 || basis.FmaxSource != SupplyFoldFmaxSourceObserved {
		t.Fatalf("fmax ladder untouched by the reuse: %+v", basis)
	}
}

// CFR-2 (#80, 用户裁定 2026-07-06) evolution of the former
// TestSupplyFoldClusterReuseFailOpenWithoutExplicitTopology: without explicit
// topology the change-point derivation now ARMS the gate on this fixture —
// sampled {2} and {7} are two singleton domains (single samples, different
// values), and unsampled cpu3 inherits toward the higher core number
// (向高核号就近继承): donor cpu7 @2GHz, NOT cpu2 @1GHz. Contrast with the b3
// explicit pin above, where "middle=2-3" makes cpu2 the donor — the SAME
// fixture resolving different donors is the explicit-vs-derived priority
// distinction, and the derived source is disclosed. The fail-open lanes this
// test USED to witness now live in TestSupplyFoldNoFrequencyDataAllUnknown
// (no samples at all) and TestSupplyFoldUnknownFrequencySliceZeroDeficit
// (above the highest sampled core — 向上不外推).
func TestSupplyFoldClusterReuseDerivedWithoutExplicitTopology(t *testing.T) {
	idx := buildTraceIndex(t, "cfr_b3_derived.systrace", clusterReuseB3Fixture)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05,
		TraceFlavorHint: TraceFlavorHarmonyHitrace})
	dep := supplyFoldDepImpact(t, chain)
	if dep.SupplyFoldBasis == nil {
		t.Fatalf("fold must run: %+v", dep)
	}
	basis := dep.SupplyFoldBasis
	if !basis.AllKnown() {
		t.Fatalf("derived reuse must yield a fully-known basis, got %+v", basis)
	}
	// Donor is cpu7 @2GHz == big fmax → ratio 1, zero deficit (the derived
	// membership puts cpu3 in cpu7's domain, unlike the explicit middle=2-3).
	if dep.SupplyFoldDeficitMs != 0 {
		t.Fatalf("cpu3 folds at the cpu7 donor frequency (== fmax): %.3f", dep.SupplyFoldDeficitMs)
	}
	if len(basis.ClusterFreqReuse) != 1 || basis.ClusterFreqReuse[0] != (SupplyFoldClusterReuse{CPU: 3, DonorCPU: 7}) {
		t.Fatalf("derived reuse must disclose cpu3←cpu7, got %+v", basis.ClusterFreqReuse)
	}
	if basis.ClusterFreqReuseSource != ClusterFreqSourceDerived {
		t.Fatalf("derived reuse must disclose its source, got %q", basis.ClusterFreqReuseSource)
	}
	if got, want := dep.SupplyFoldIdealMs+dep.SupplyFoldDeficitMs, dep.RunningMs; !floatNear(got, want) {
		t.Fatalf("ideal+deficit must reconstruct RunningMs: %.6f != %.6f", got, want)
	}
}

// 跨簇 mutation pin: cpu3 sits alone in its declared cluster; cpu2 (small)
// and cpu7 (big) are sampled but belong to OTHER clusters — no reuse.
func TestSupplyFoldClusterReuseCrossClusterNever(t *testing.T) {
	idx := buildTraceIndex(t, "cfr_b3_crosscluster.systrace", clusterReuseB3Fixture)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05,
		TraceFlavorHint: TraceFlavorHarmonyHitrace, CoreTopology: "small=2;middle=3;big=7"})
	dep := supplyFoldDepImpact(t, chain)
	basis := dep.SupplyFoldBasis
	if basis == nil {
		t.Fatalf("fold must run: %+v", dep)
	}
	if basis.KnownMs != 0 || !floatNear(basis.UnknownMs, dep.RunningMs) || len(basis.ClusterFreqReuse) != 0 {
		t.Fatalf("cross-cluster samples must not be reused: %+v", basis)
	}
}

// 簇判定绕过 mutation pin: "big" and "prime" both normalize to the big
// DISPLAY class, but they are distinct frequency domains — a prime core must
// not borrow the big core's samples. A normalized-class domain identity flips
// this basis to known and goes red.
func TestSupplyFoldClusterReusePrimeVsBigDistinct(t *testing.T) {
	idx := buildTraceIndex(t, "cfr_prime_vs_big.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000 cpu_id=2
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [003] .... 5.000000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [003] .... 5.009900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [003] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05,
		TraceFlavorHint: TraceFlavorHarmonyHitrace, CoreTopology: "big=2;prime=3"})
	dep := supplyFoldDepImpact(t, chain)
	basis := dep.SupplyFoldBasis
	if basis == nil {
		t.Fatalf("fold must run: %+v", dep)
	}
	if basis.KnownMs != 0 || len(basis.ClusterFreqReuse) != 0 {
		t.Fatalf("prime must not borrow big's samples (distinct frequency domains): %+v", basis)
	}
}

// 禁反向 mutation pin: the slice CPU has its OWN 1.5GHz sample; a same-cluster
// 1GHz donor exists. The fold must use 1.5GHz (deficit 10×(1−0.75)=2.5ms) —
// a donor override would read 1GHz and inflate the deficit to 5ms.
func TestSupplyFoldClusterReuseOwnSamplesNeverOverridden(t *testing.T) {
	idx := buildTraceIndex(t, "cfr_own_samples.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000 cpu_id=2
      <idle>-0 (-----) [003] .... 4.900000: cpu_frequency: state=1500000 cpu_id=3
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2000000 cpu_id=7
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [003] .... 5.000000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [003] .... 5.009900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [003] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05,
		TraceFlavorHint: TraceFlavorHarmonyHitrace, CoreTopology: "middle=2-3;big=7"})
	dep := supplyFoldDepImpact(t, chain)
	basis := dep.SupplyFoldBasis
	if basis == nil {
		t.Fatalf("fold must run: %+v", dep)
	}
	if dep.SupplyFoldDeficitMs < 2.2 || dep.SupplyFoldDeficitMs > 2.8 {
		t.Fatalf("own 1.5GHz samples must win over the 1GHz donor (deficit ~2.5ms), got %.3f", dep.SupplyFoldDeficitMs)
	}
	if len(basis.ClusterFreqReuse) != 0 {
		t.Fatalf("own samples → no reuse disclosure: %+v", basis.ClusterFreqReuse)
	}
}

// Aggregate mirror: the reuse disclosure is a set union over folded members.
func TestSupplyFoldClusterReuseAggregateUnion(t *testing.T) {
	member := func(startTs float64, pairs []SupplyFoldClusterReuse) WakeupCausalImpact {
		return WakeupCausalImpact{
			Thread:              ThreadRef{Comm: "dep", PID: 200},
			Window:              TimeWindow{StartTs: startTs, EndTs: startTs + 0.02},
			ChainDepth:          1,
			OnChain:             true,
			DominantState:       string(StateRunning),
			DominantImpactMs:    20,
			TotalMs:             20,
			RunningMs:           20,
			SupplyFoldDeficitMs: 5,
			SupplyFoldIdealMs:   15,
			SupplyFoldBasis:     &SupplyFoldBasis{KnownMs: 20, ClusterFreqReuse: pairs},
		}
	}
	chain := ChainResult{CausalImpacts: []WakeupCausalImpact{
		member(5.0, []SupplyFoldClusterReuse{{CPU: 6, DonorCPU: 4}}),
		member(6.0, []SupplyFoldClusterReuse{{CPU: 3, DonorCPU: 2}, {CPU: 6, DonorCPU: 4}}),
	}}
	aggregates := aggregateWakeupCausalImpacts(&chain)
	if len(aggregates) != 1 {
		t.Fatalf("expected one aggregate, got %+v", aggregates)
	}
	got := aggregates[0].SupplyFoldBasis.ClusterFreqReuse
	if len(got) != 2 || got[0] != (SupplyFoldClusterReuse{CPU: 3, DonorCPU: 2}) || got[1] != (SupplyFoldClusterReuse{CPU: 6, DonorCPU: 4}) {
		t.Fatalf("aggregate must union+dedupe+sort the pairs, got %+v", got)
	}
}

// --- window-face pins --------------------------------------------------------

// clusterReuseWindowFixture: three busy CPUs; cpu2 sampled 1GHz, cpu7 sampled
// 2GHz, cpu3 unsampled.
const clusterReuseWindowFixture = `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2000000 cpu_id=7
        w2-200 (200) [002] .... 5.000000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=w2 next_pid=200 next_prio=120
        w3-300 (300) [003] .... 5.000000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=w3 next_pid=300 next_prio=120
        w7-700 (700) [007] .... 5.000000: sched_switch: prev_comm=idle/7 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=w7 next_pid=700 next_prio=120
        w2-200 (200) [002] .... 5.010000: sched_switch: prev_comm=w2 prev_pid=200 prev_prio=120 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        w3-300 (300) [003] .... 5.010000: sched_switch: prev_comm=w3 prev_pid=300 prev_prio=120 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
        w7-700 (700) [007] .... 5.010000: sched_switch: prev_comm=w7 prev_pid=700 prev_prio=120 prev_state=S ==> next_comm=idle/7 next_pid=0 next_prio=120
	`

func windowFixtureSupplyRow(t *testing.T, stats WindowStats, cpu int) ComputeSupplyCPUBalance {
	t.Helper()
	if stats.ComputeSupplyBalance == nil {
		t.Fatalf("compute_supply must build on a bounded window")
	}
	for _, per := range stats.ComputeSupplyBalance.PerCPU {
		if per.CPU == cpu {
			return per
		}
	}
	t.Fatalf("cpu=%d row missing: %+v", cpu, stats.ComputeSupplyBalance.PerCPU)
	return ComputeSupplyCPUBalance{}
}

func windowFixtureTopRunningFreq(t *testing.T, stats WindowStats, pid int) int {
	t.Helper()
	for _, td := range stats.TopRunning {
		if td.Thread.PID == pid {
			return td.Frequency
		}
	}
	t.Fatalf("pid=%d missing from TopRunning: %+v", pid, stats.TopRunning)
	return 0
}

// Under explicit topology, the window face's frequency-WEIGHTED consumers see
// the donor timeline for cpu3 (compute_supply fmax + thread frequency), each
// reuse is disclosed (per-CPU donor field + ledger caveat + window caveat),
// and the raw sampling FACTS — residency, latest frequency — stay unwritten.
func TestComputeWindowStatsClusterSharedFrequencyReuse(t *testing.T) {
	idx := buildTraceIndex(t, "cfr_window_reuse.systrace", clusterReuseWindowFixture)
	stats := ComputeWindowStats(idx, Query{TimeStart: 5.0, TimeEnd: 5.020, CoreTopology: "middle=2-3;big=7"})
	row := windowFixtureSupplyRow(t, stats, 3)
	if !row.FrequencyKnown || row.MaxFrequencyKHz != 1000000 {
		t.Fatalf("cpu3 must take the same-cluster cpu2 fmax (1GHz), got %+v", row)
	}
	if row.FrequencyClusterDonorCPU == nil || *row.FrequencyClusterDonorCPU != 2 {
		t.Fatalf("cpu3 row must disclose the donor (cpu2), got %+v", row)
	}
	if row.LowFrequencyLossMs != 0 {
		t.Fatalf("weighting and fmax read the SAME donor timeline — no phantom loss: %+v", row)
	}
	ledger := strings.Join(stats.ComputeSupplyBalance.Caveats, "\n")
	if !strings.Contains(ledger, "cpu=3 has no own cpu_frequency samples; frequency taken from same-cluster cpu=2") ||
		!strings.Contains(ledger, "cpu3 频点=同簇 cpu2,簇共频复用") {
		t.Fatalf("ledger caveat must disclose the reuse, got:\n%s", ledger)
	}
	if strings.Contains(ledger, "cpu=3 has no cpu_frequency samples in the window") {
		t.Fatalf("the old 无频点数据 weight-1.0 caveat must be replaced by the reuse disclosure:\n%s", ledger)
	}
	if got := windowFixtureTopRunningFreq(t, stats, 300); got != 1000000 {
		t.Fatalf("w3's TopRunning frequency must read the donor timeline, got %d", got)
	}
	window := strings.Join(stats.Caveats, "\n")
	if !strings.Contains(window, "cluster-shared frequency reuse under explicit core_topology: cpu3←cpu2") {
		t.Fatalf("window-level disclosure missing:\n%s", window)
	}
	// Sampling FACTS stay facts: cpu3 has no residency and no latest sample.
	for _, cpu := range stats.CPU {
		if cpu.CPU == 3 && (len(cpu.FrequencyResidency) != 0 || cpu.Frequency != 0) {
			t.Fatalf("raw sampling facts must not be rewritten: %+v", cpu)
		}
	}
	// The sampled cores are byte-untouched: own samples, no donor fields.
	for _, cpu := range []int{2, 7} {
		row := windowFixtureSupplyRow(t, stats, cpu)
		if row.FrequencyClusterDonorCPU != nil {
			t.Fatalf("sampled cpu%d must not carry a donor: %+v", cpu, row)
		}
	}
}

// CFR-2 (#80) evolution of the former window-face fail-open pin: without
// explicit topology the change-point derivation now resolves cpu3 into
// cpu7's singleton domain (向高核号就近继承 — sampled {2} and {7} never merge:
// different values), so the frequency-weighted faces read cpu7's 2GHz with
// the DERIVED disclosure variants everywhere. A busy core ABOVE the highest
// sampled core (cpu9) stays fail-open on this very face: honest weight-1.0
// caveat, no donor — the window-face 向上不外推 witness.
func TestComputeWindowStatsClusterReuseDerivedWithoutTopology(t *testing.T) {
	idx := buildTraceIndex(t, "cfr_window_derived.systrace", clusterReuseWindowFixture+`        w9-900 (900) [009] .... 5.000000: sched_switch: prev_comm=idle/9 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=w9 next_pid=900 next_prio=120
        w9-900 (900) [009] .... 5.010000: sched_switch: prev_comm=w9 prev_pid=900 prev_prio=120 prev_state=S ==> next_comm=idle/9 next_pid=0 next_prio=120
	`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 5.0, TimeEnd: 5.020})
	row := windowFixtureSupplyRow(t, stats, 3)
	if !row.FrequencyKnown || row.MaxFrequencyKHz != 2000000 {
		t.Fatalf("derived membership must give cpu3 the cpu7 fmax (2GHz), got %+v", row)
	}
	if row.FrequencyClusterDonorCPU == nil || *row.FrequencyClusterDonorCPU != 7 ||
		row.FrequencyClusterDonorSource != ClusterFreqSourceDerived {
		t.Fatalf("cpu3 row must disclose donor cpu7 with the derived source, got %+v", row)
	}
	ledger := strings.Join(stats.ComputeSupplyBalance.Caveats, "\n")
	if !strings.Contains(ledger, "cpu=3 has no own cpu_frequency samples; frequency taken from same-cluster cpu=7, clusters derived from frequency change points (cpu3 频点=同簇 cpu7,簇共频复用,频点变化点推导)") {
		t.Fatalf("derived ledger caveat missing:\n%s", ledger)
	}
	if got := windowFixtureTopRunningFreq(t, stats, 300); got != 2000000 {
		t.Fatalf("w3's TopRunning frequency must read the derived donor timeline, got %d", got)
	}
	window := strings.Join(stats.Caveats, "\n")
	if !strings.Contains(window, "cluster-shared frequency reuse from freq-change-point derived clusters (no explicit core_topology): cpu3←cpu7") {
		t.Fatalf("derived window disclosure missing:\n%s", window)
	}
	// 向上不外推 (window face): cpu9 sits above the highest sampled core —
	// no donor, honest weight-1.0 caveat, thread frequency stays absent.
	row9 := windowFixtureSupplyRow(t, stats, 9)
	if row9.FrequencyKnown || row9.MaxFrequencyKHz != 0 || row9.FrequencyClusterDonorCPU != nil {
		t.Fatalf("cpu9 above the highest sampled core must never be extrapolated: %+v", row9)
	}
	if !strings.Contains(ledger, "cpu=9 has no cpu_frequency samples in the window; its running time is weighted 1.0 (无频点数据)") {
		t.Fatalf("cpu9 must keep the honest weight-1.0 caveat:\n%s", ledger)
	}
	if got := windowFixtureTopRunningFreq(t, stats, 900); got != 0 {
		t.Fatalf("w9 must stay frequency-less above the sampled range, got %d", got)
	}
}

// --- grammar parity pin (F2, CFR verify round) ------------------------------

// TestParseClusterFreqDomainsGrammarParityWithCoreTopology is the differential
// pin for the "grammar and entry admission are IDENTICAL to parseCoreTopology"
// claim in cluster_freq_share.go. The two parsers copy the separator/Cut/trim
// steps rather than sharing them, so a grammar edit to either one must turn
// this red instead of silently letting the reuse gate drift wider or narrower
// than the explicit-topology gate. Invariant per corpus entry: identical cpu
// key sets, and per cpu the verbatim domain label normalizes to exactly the
// class parseCoreTopology admitted.
func TestParseClusterFreqDomainsGrammarParityWithCoreTopology(t *testing.T) {
	corpus := []string{
		"small=0-3,middle=4-6;big=7",
		"little:0-1;m:2,3;prime:7",
		"  big = 4-6 ; prime = 7 ; small = 0-3  ",
		"big=4-6;big=5;small=0-3",
		"small=0-2;middle=2-4",
		"cluster0=0-3;big=4-7",
		"garbage;=;:;big=",
		"BIG=4-6;Prime=7",
		"b=6;l=0;m=3",
		"",
		"   ",
		"middle=2-4;big=7",
	}
	for _, raw := range corpus {
		topo := parseCoreTopology(raw)
		domains := parseClusterFreqDomains(raw)
		if len(topo) != len(domains.byCPU) {
			t.Fatalf("%q: admission drifted: parseCoreTopology=%d cpus %v, parseClusterFreqDomains=%d cpus %v",
				raw, len(topo), topo, len(domains.byCPU), domains.byCPU)
		}
		for cpu, class := range topo {
			label, ok := domains.byCPU[cpu]
			if !ok {
				t.Fatalf("%q: cpu%d admitted by parseCoreTopology (class %q) but missing from freq domains %v",
					raw, cpu, class, domains.byCPU)
			}
			if normalizeCoreClass(label) != class {
				t.Fatalf("%q: cpu%d domain label %q normalizes to %q, parseCoreTopology admitted class %q",
					raw, cpu, label, normalizeCoreClass(label), class)
			}
		}
		// Internal coherence: every roster member maps back to its label and
		// rosters carry no cpu that byCPU assigned elsewhere (later-wins).
		for label, members := range domains.members {
			for _, cpu := range members {
				if domains.byCPU[cpu] != label {
					t.Fatalf("%q: roster/byCPU divergence: cpu%d in roster %q but byCPU says %q",
						raw, cpu, label, domains.byCPU[cpu])
				}
			}
		}
	}
}

// --- CFR-2 (#80, 用户裁定 2026-07-06) change-point derivation pins ----------
//
// Mutation surface this section keeps red:
//   - 向上外推 (folding cores above the highest sampled core into a sampled
//     domain) — TestDeriveClusterFreqDomainsUpwardExtrapolationForbidden and
//     the cpu9 arm of the window-face derived test;
//   - 时间线不一致合并 (merging sampled cores whose timelines differ in
//     values, length, or beyond the skew bound) —
//     TestDeriveClusterFreqDomainsMismatchNeverMerges;
//   - explicit 被推导覆盖 (derivation overriding an explicit topology) —
//     TestResolveClusterFreqDomainsExplicitPriority;
//   - real-device shape drift — TestDeriveClusterFreqDomainsRealDonghuTrace
//     runs the derivation over the committed donghu capture.

// donghuBurstTimelines builds the donghu emission shape: per DVFS transition
// one row per cluster member, values identical, member rows µs apart.
func donghuBurstTimelines(khz []int, baseTs float64, cpus ...int) map[int][]freqSample {
	out := map[int][]freqSample{}
	for i, cpu := range cpus {
		tl := make([]freqSample, 0, len(khz))
		for k, v := range khz {
			ts := baseTs + float64(k)*0.010 + float64(i)*1e-6
			tl = append(tl, freqSample{ts: ts, khz: v})
		}
		out[cpu] = tl
	}
	return out
}

func TestDeriveClusterFreqDomainsDonghuEmissionShape(t *testing.T) {
	d := deriveClusterFreqDomains(donghuBurstTimelines([]int{1090000, 2189000, 1224000}, 100.0, 3, 4, 5))
	if d.source != ClusterFreqSourceDerived || d.groupCount != 1 {
		t.Fatalf("identical burst timelines must form one derived domain: %+v", d)
	}
	has := func(cpu int) bool { return cpu >= 3 && cpu <= 5 }
	// 向下继承: cpus 0-2 inherit the {3,4,5} domain, lowest sampled donor.
	for cpu := 0; cpu <= 2; cpu++ {
		if donor, ok := d.donorFor(cpu, has); !ok || donor != 3 {
			t.Fatalf("cpu%d must inherit the sampled cluster (donor 3), got %d/%v", cpu, donor, ok)
		}
	}
	// 禁反向: sampled members never take a donor.
	if _, ok := d.donorFor(4, has); ok {
		t.Fatalf("sampled cpu4 must never take a donor")
	}
	// 向上不外推: one derived domain (<3) — cpus above 5 stay unassigned.
	for cpu := 6; cpu <= 7; cpu++ {
		if _, ok := d.donorFor(cpu, has); ok {
			t.Fatalf("cpu%d above the highest sampled core must not be extrapolated", cpu)
		}
		if label := d.derivedDomainLabelFor(cpu); label != "" {
			t.Fatalf("with <3 domains cpu%d must stay unassigned, got %q", cpu, label)
		}
	}
}

func TestDeriveClusterFreqDomainsMismatchNeverMerges(t *testing.T) {
	// (a) same values, timestamps beyond the skew bound → split.
	tl := donghuBurstTimelines([]int{1090000, 1618000}, 100.0, 3)
	skewed := donghuBurstTimelines([]int{1090000, 1618000}, 100.0+1e-3, 4) // 1ms off
	d := deriveClusterFreqDomains(map[int][]freqSample{3: tl[3], 4: skewed[4]})
	if d.groupCount != 2 {
		t.Fatalf("1ms timestamp skew must split (bound %.0fµs), got %+v", clusterFreqDeriveMaxSkewSec*1e6, d)
	}
	// (b) one differing value → split.
	a := donghuBurstTimelines([]int{1090000, 1618000}, 100.0, 3, 4)
	a[4][1].khz = 1617000
	if d := deriveClusterFreqDomains(a); d.groupCount != 2 {
		t.Fatalf("value mismatch must split, got %+v", d)
	}
	// (c) different lengths → split.
	b := donghuBurstTimelines([]int{1090000, 1618000}, 100.0, 3, 4)
	b[4] = b[4][:1]
	if d := deriveClusterFreqDomains(b); d.groupCount != 2 {
		t.Fatalf("length mismatch must split, got %+v", d)
	}
	// Within-bound µs jitter (donghu shape) still merges.
	if d := deriveClusterFreqDomains(donghuBurstTimelines([]int{1090000, 1618000}, 100.0, 3, 4)); d.groupCount != 1 {
		t.Fatalf("µs jitter within the bound must merge, got %+v", d)
	}
}

func TestDeriveClusterFreqDomainsUpwardExtrapolationForbidden(t *testing.T) {
	// Three distinct singleton domains (小/中/大 derived) on cpus 1/3/5.
	timelines := map[int][]freqSample{
		1: {{ts: 100.0, khz: 800000}},
		3: {{ts: 100.0, khz: 1500000}},
		5: {{ts: 100.0, khz: 2000000}},
	}
	d := deriveClusterFreqDomains(timelines)
	if d.groupCount != 3 {
		t.Fatalf("three distinct timelines must form three domains: %+v", d)
	}
	has := func(cpu int) bool { _, ok := timelines[cpu]; return ok }
	// 依次类推 downward/between: cpu0→1, cpu2→3, cpu4→5.
	for cpu, want := range map[int]int{0: 1, 2: 3, 4: 5} {
		if donor, ok := d.donorFor(cpu, has); !ok || donor != want {
			t.Fatalf("cpu%d must inherit toward the higher core number (donor %d), got %d/%v", cpu, want, donor, ok)
		}
	}
	// ≥3 domains → cores above the highest sampled core are DECLARED the
	// prime pseudo-domain and stay donor-less (排除性规则: big-core samples
	// must not leak upward into the 超大核).
	for cpu := 6; cpu <= 7; cpu++ {
		if donor, ok := d.donorFor(cpu, has); ok {
			t.Fatalf("cpu%d must never borrow from below (got donor %d) — 向上不外推", cpu, donor)
		}
		if label := d.derivedDomainLabelFor(cpu); label != clusterFreqDerivedPrimeLabel {
			t.Fatalf("cpu%d must be declared %q with ≥3 domains, got %q", cpu, clusterFreqDerivedPrimeLabel, label)
		}
	}
	// Sampled member label stays a real group, never prime.
	if label := d.derivedDomainLabelFor(5); label == clusterFreqDerivedPrimeLabel || label == "" {
		t.Fatalf("sampled cpu5 must keep its own domain label, got %q", label)
	}
}

func TestResolveClusterFreqDomainsExplicitPriority(t *testing.T) {
	// Derivation over these timelines would put cpu3 into cpu7's domain
	// (向高核号就近继承); the explicit map says middle=2-3 — explicit wins.
	timelines := map[int][]freqSample{
		2: {{ts: 100.0, khz: 1000000}},
		7: {{ts: 100.0, khz: 2000000}},
	}
	source := func() map[int][]freqSample { return timelines }
	d := resolveClusterFreqDomains("middle=2-3;big=7", source)
	if d.source != ClusterFreqSourceExplicit {
		t.Fatalf("explicit topology must win outright, got source %q", d.source)
	}
	has := func(cpu int) bool { _, ok := timelines[cpu]; return ok }
	if donor, ok := d.donorFor(3, has); !ok || donor != 2 {
		t.Fatalf("explicit middle=2-3 must resolve donor cpu2, got %d/%v", donor, ok)
	}
	// Absence of the explicit map falls back to derivation (source token +
	// the different donor prove the lane switch).
	d = resolveClusterFreqDomains("", source)
	if d.source != ClusterFreqSourceDerived {
		t.Fatalf("no explicit topology must derive, got source %q", d.source)
	}
	if donor, ok := d.donorFor(3, has); !ok || donor != 7 {
		t.Fatalf("derived membership resolves donor cpu7, got %d/%v", donor, ok)
	}
	// Nothing to derive from → zero value, every lookup fails open.
	d = resolveClusterFreqDomains("", func() map[int][]freqSample { return nil })
	if d.known() || d.source != "" {
		t.Fatalf("no topology and no samples must fail open: %+v", d)
	}
}

// TestDeriveClusterFreqDomainsRealDonghuTrace runs the derivation over the
// committed donghu real-device capture (PROFILE.md: cpu_frequency lanes exist
// ONLY for cpu3/4/5, 30 rows each, per-transition member bursts µs apart —
// the emission shape the skew bound is anchored on).
func TestDeriveClusterFreqDomainsRealDonghuTrace(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Fatalf("BuildIndex(donghu_tieba_frame): %v", err)
	}
	c := &chainQueryCache{idx: idx}
	c.buildFreqIndex()
	d := deriveClusterFreqDomains(c.freqByCPU)
	if d.source != ClusterFreqSourceDerived {
		t.Fatalf("donghu capture must derive domains, got %+v", d)
	}
	if len(d.sampledAsc) != 3 || d.sampledAsc[0] != 3 || d.sampledAsc[2] != 5 {
		t.Fatalf("donghu samples live on cpu3/4/5 only, got %v", d.sampledAsc)
	}
	if d.groupCount != 1 {
		t.Fatalf("cpu3/4/5 emit one shared frequency point — one derived domain, got %d (%+v)", d.groupCount, d.members)
	}
	has := func(cpu int) bool { return len(c.freqByCPU[cpu]) > 0 }
	// The customer b3 shape: cpus 0-2 (no lanes in the capture) inherit the
	// sampled cluster — donor cpu3.
	for cpu := 0; cpu <= 2; cpu++ {
		if donor, ok := d.donorFor(cpu, has); !ok || donor != 3 {
			t.Fatalf("cpu%d must resolve donor cpu3 on the real capture, got %d/%v", cpu, donor, ok)
		}
	}
	// Above cpu5: single derived domain (<3) — never extrapolated upward.
	for cpu := 6; cpu <= 11; cpu++ {
		if _, ok := d.donorFor(cpu, has); ok {
			t.Fatalf("cpu%d above the sampled cluster must fail open on the real capture", cpu)
		}
	}
	// 禁反向 on real data: the sampled cores answer for themselves.
	for cpu := 3; cpu <= 5; cpu++ {
		if _, ok := d.donorFor(cpu, has); ok {
			t.Fatalf("sampled cpu%d must never take a donor", cpu)
		}
	}
}

// Fold-face integration: the donghu-burst emission WITHOUT topology folds an
// unsampled-core slice with the derived donor and discloses the derived
// source (the q2/b3 "频点数据不全,无法折算" shape resolves trace-natively).
func TestSupplyFoldClusterReuseDerivedDonghuBurst(t *testing.T) {
	idx := buildTraceIndex(t, "cfr2_donghu_burst.systrace", `
      <idle>-0 (-----) [003] .... 4.900000: cpu_frequency: state=1000000 cpu_id=3
      <idle>-0 (-----) [003] .... 4.900001: cpu_frequency: state=1000000 cpu_id=4
      <idle>-0 (-----) [003] .... 4.900002: cpu_frequency: state=1000000 cpu_id=5
      <idle>-0 (-----) [003] .... 4.950000: cpu_frequency: state=1500000 cpu_id=3
      <idle>-0 (-----) [003] .... 4.950001: cpu_frequency: state=1500000 cpu_id=4
      <idle>-0 (-----) [003] .... 4.950002: cpu_frequency: state=1500000 cpu_id=5
        app-100 (100) [002] .... 4.990000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [002] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        dep-200 (100) [001] .... 5.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [001] .... 5.009900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=002
        dep-200 (100) [001] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        app-100 (100) [002] .... 5.010000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	dep := supplyFoldDepImpact(t, chain)
	if dep.SupplyFoldBasis == nil {
		t.Fatalf("fold must run: %+v", dep)
	}
	basis := dep.SupplyFoldBasis
	if !basis.AllKnown() {
		t.Fatalf("derived donghu-burst reuse must yield a fully-known basis (b3 caveat gone), got %+v", basis)
	}
	if dep.SupplyFoldDeficitMs != 0 {
		t.Fatalf("slice governed at the cluster frequency == fmax must carry zero deficit, got %.3f", dep.SupplyFoldDeficitMs)
	}
	if len(basis.ClusterFreqReuse) != 1 || basis.ClusterFreqReuse[0] != (SupplyFoldClusterReuse{CPU: 1, DonorCPU: 3}) {
		t.Fatalf("derived reuse must disclose cpu1←cpu3, got %+v", basis.ClusterFreqReuse)
	}
	if basis.ClusterFreqReuseSource != ClusterFreqSourceDerived {
		t.Fatalf("derived source must be disclosed, got %q", basis.ClusterFreqReuseSource)
	}
	if got, want := dep.SupplyFoldIdealMs+dep.SupplyFoldDeficitMs, dep.RunningMs; !floatNear(got, want) {
		t.Fatalf("ideal+deficit must reconstruct RunningMs: %.6f != %.6f", got, want)
	}
}

// --- CFR-2 verify-round pins (P2-1/P2-2, P3-3, P3-4, first-wins nit) --------

// TestDeriveClusterFreqDomainsSkewBoundary pins both sides of the measured
// 15µs bound (real-capture anchors: 5µs worst member spread, 46µs/61µs
// tightest distinct transitions): 10µs same-value timelines MUST merge, 20µs
// MUST split. Guards the constant against silent drift in either direction.
func TestDeriveClusterFreqDomainsSkewBoundary(t *testing.T) {
	build := func(skew float64) map[int][]freqSample {
		return map[int][]freqSample{
			3: {{ts: 100.0, khz: 1090000}, {ts: 100.010, khz: 1618000}},
			4: {{ts: 100.0 + skew, khz: 1090000}, {ts: 100.010 + skew, khz: 1618000}},
		}
	}
	if d := deriveClusterFreqDomains(build(10e-6)); d.groupCount != 1 {
		t.Fatalf("10µs same-value skew must merge (bound 15µs), got %d groups", d.groupCount)
	}
	if d := deriveClusterFreqDomains(build(20e-6)); d.groupCount != 2 {
		t.Fatalf("20µs skew must split (bound 15µs), got %d groups", d.groupCount)
	}
}

// P3-3 + P3-4 formatter pins: the derived caveat discloses consulted prime
// cores and an ignored unparseable core_topology input; the explicit variant
// stays byte-identical to the CFR #75 wording no matter what companions are
// passed.
func TestClusterFreqReuseCaveatPrimeAndIgnoredDisclosure(t *testing.T) {
	pairs := [][2]int{{2, 3}}
	caveat := clusterFreqReuseCaveat(pairs, ClusterFreqSourceDerived, []int{6, 7}, false)
	if !strings.Contains(caveat, "(no explicit core_topology): cpu2←cpu3") {
		t.Fatalf("plain derived head lost: %s", caveat)
	}
	if !strings.Contains(caveat, "cpu6,cpu7 高于最高采样核,按裁定推导为超大核域(无采样成员,不复用,原样保留无频点口径)") {
		t.Fatalf("prime disclosure clause missing: %s", caveat)
	}
	caveat = clusterFreqReuseCaveat(pairs, ClusterFreqSourceDerived, nil, true)
	if !strings.Contains(caveat, "core_topology input had no recognizable cluster labels") ||
		!strings.Contains(caveat, "core_topology 输入未能解析(无可识别簇标签),已按频点变化点推导") {
		t.Fatalf("ignored-input head missing: %s", caveat)
	}
	if strings.Contains(caveat, "(no explicit core_topology)") {
		t.Fatalf("ignored input must not be narrated as absent input: %s", caveat)
	}
	if strings.Contains(caveat, "超大核域") {
		t.Fatalf("no prime cores consulted → no prime clause: %s", caveat)
	}
	// Explicit variant: byte-identical to CFR #75, companions ignored.
	if got, want := clusterFreqReuseCaveat(pairs, ClusterFreqSourceExplicit, []int{6}, true),
		clusterFreqReuseCaveat(pairs, ClusterFreqSourceExplicit, nil, false); got != want {
		t.Fatalf("explicit wording must ignore derived-lane companions:\n%s\nvs\n%s", got, want)
	}
}

// P3-3/P3-4 window-face integration: three distinct sampled singletons
// (小/中/大 derived) + an unparseable core_topology input; the inheriting
// core reuses with the derived disclosure, the above-max core is declared
// prime in the window caveat AND keeps the honest weight-1.0 accounting.
func TestComputeWindowStatsClusterReusePrimeAndIgnoredInput(t *testing.T) {
	idx := buildTraceIndex(t, "cfr2_window_prime.systrace", `
      <idle>-0 (-----) [001] .... 4.900000: cpu_frequency: state=800000 cpu_id=1
      <idle>-0 (-----) [003] .... 4.900000: cpu_frequency: state=1500000 cpu_id=3
      <idle>-0 (-----) [005] .... 4.900000: cpu_frequency: state=2000000 cpu_id=5
        w2-200 (200) [002] .... 5.000000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=w2 next_pid=200 next_prio=120
        w6-600 (600) [006] .... 5.000000: sched_switch: prev_comm=idle/6 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=w6 next_pid=600 next_prio=120
        w2-200 (200) [002] .... 5.010000: sched_switch: prev_comm=w2 prev_pid=200 prev_prio=120 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        w6-600 (600) [006] .... 5.010000: sched_switch: prev_comm=w6 prev_pid=600 prev_prio=120 prev_state=S ==> next_comm=idle/6 next_pid=0 next_prio=120
	`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 5.0, TimeEnd: 5.020, CoreTopology: "cluster0=0-9"})
	row := windowFixtureSupplyRow(t, stats, 2)
	if !row.FrequencyKnown || row.MaxFrequencyKHz != 1500000 ||
		row.FrequencyClusterDonorCPU == nil || *row.FrequencyClusterDonorCPU != 3 ||
		row.FrequencyClusterDonorSource != ClusterFreqSourceDerived {
		t.Fatalf("cpu2 must inherit cpu3's domain with derived disclosure, got %+v", row)
	}
	row6 := windowFixtureSupplyRow(t, stats, 6)
	if row6.FrequencyKnown || row6.FrequencyClusterDonorCPU != nil {
		t.Fatalf("prime cpu6 must never be donated to: %+v", row6)
	}
	ledger := strings.Join(stats.ComputeSupplyBalance.Caveats, "\n")
	if !strings.Contains(ledger, "cpu=6 has no cpu_frequency samples in the window; its running time is weighted 1.0 (无频点数据)") {
		t.Fatalf("prime core keeps the honest weight-1.0 accounting:\n%s", ledger)
	}
	window := strings.Join(stats.Caveats, "\n")
	if !strings.Contains(window, "core_topology input had no recognizable cluster labels") {
		t.Fatalf("ignored-input disclosure missing (P3-4):\n%s", window)
	}
	if !strings.Contains(window, "cpu6 高于最高采样核,按裁定推导为超大核域(无采样成员,不复用,原样保留无频点口径)") {
		t.Fatalf("prime disclosure missing (P3-3):\n%s", window)
	}
}

// First-wins nit (CFR-2 verify): the aggregate's ClusterFreqReuseSource takes
// the FIRST folded member's non-empty token and never flips on later members.
func TestSupplyFoldClusterReuseAggregateSourceFirstWins(t *testing.T) {
	member := func(startTs float64, source string) WakeupCausalImpact {
		return WakeupCausalImpact{
			Thread:              ThreadRef{Comm: "dep", PID: 200},
			Window:              TimeWindow{StartTs: startTs, EndTs: startTs + 0.02},
			ChainDepth:          1,
			OnChain:             true,
			DominantState:       string(StateRunning),
			DominantImpactMs:    20,
			TotalMs:             20,
			RunningMs:           20,
			SupplyFoldDeficitMs: 5,
			SupplyFoldIdealMs:   15,
			SupplyFoldBasis: &SupplyFoldBasis{KnownMs: 20,
				ClusterFreqReuse:       []SupplyFoldClusterReuse{{CPU: 6, DonorCPU: 4}},
				ClusterFreqReuseSource: source},
		}
	}
	chain := ChainResult{CausalImpacts: []WakeupCausalImpact{
		member(5.0, ClusterFreqSourceDerived),
		member(6.0, ClusterFreqSourceExplicit),
	}}
	aggregates := aggregateWakeupCausalImpacts(&chain)
	if len(aggregates) != 1 {
		t.Fatalf("expected one aggregate, got %+v", aggregates)
	}
	if got := aggregates[0].SupplyFoldBasis.ClusterFreqReuseSource; got != ClusterFreqSourceDerived {
		t.Fatalf("aggregate source must keep the first member's token, got %q", got)
	}
}
