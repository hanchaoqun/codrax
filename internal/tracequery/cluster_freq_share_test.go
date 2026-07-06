package tracequery

import (
	"strings"
	"testing"
)

// cluster_freq_share_test.go — CFR (#75, 客户硬件域裁定) pins for
// cluster-shared frequency reuse. The mutation surface this file keeps red:
//
//   - 复用门摘除 (reuse without explicit topology / under inference only) —
//     TestSupplyFoldClusterReuseFailOpenWithoutExplicitTopology and the
//     window-face fail-open twin go red;
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

// 复用门摘除 mutation pin: the SAME fixture without explicit topology (the
// frequency-tier inference still classifies the sampled cpus) must keep the
// pre-CFR behavior byte-for-byte — unknown basis, zero fabricated deficit, no
// disclosure. Any change that reuses "some sampled CPU" without the explicit
// gate turns KnownMs positive here and goes red.
func TestSupplyFoldClusterReuseFailOpenWithoutExplicitTopology(t *testing.T) {
	idx := buildTraceIndex(t, "cfr_b3_noreuse.systrace", clusterReuseB3Fixture)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05,
		TraceFlavorHint: TraceFlavorHarmonyHitrace})
	dep := supplyFoldDepImpact(t, chain)
	if dep.SupplyFoldBasis == nil {
		t.Fatalf("fold must run: %+v", dep)
	}
	basis := dep.SupplyFoldBasis
	if basis.KnownMs != 0 || !floatNear(basis.UnknownMs, dep.RunningMs) {
		t.Fatalf("no explicit topology → no reuse, everything unknown: %+v vs running %.3f", basis, dep.RunningMs)
	}
	if dep.SupplyFoldDeficitMs != 0 {
		t.Fatalf("fail-open must never fabricate a deficit: %.3f", dep.SupplyFoldDeficitMs)
	}
	if len(basis.ClusterFreqReuse) != 0 {
		t.Fatalf("no reuse → no disclosure: %+v", basis.ClusterFreqReuse)
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

// 复用门摘除 mutation pin (window face): without explicit topology the
// pre-CFR behavior stands verbatim — weight 1.0 + the honest 无频点数据
// caveat, no donor fields, no disclosure caveat, no thread frequency.
func TestComputeWindowStatsClusterReuseFailOpenWithoutTopology(t *testing.T) {
	idx := buildTraceIndex(t, "cfr_window_noreuse.systrace", clusterReuseWindowFixture)
	stats := ComputeWindowStats(idx, Query{TimeStart: 5.0, TimeEnd: 5.020})
	row := windowFixtureSupplyRow(t, stats, 3)
	if row.FrequencyKnown || row.MaxFrequencyKHz != 0 || row.FrequencyClusterDonorCPU != nil {
		t.Fatalf("no explicit topology → no reuse on cpu3: %+v", row)
	}
	ledger := strings.Join(stats.ComputeSupplyBalance.Caveats, "\n")
	if !strings.Contains(ledger, "cpu=3 has no cpu_frequency samples in the window; its running time is weighted 1.0 (无频点数据)") {
		t.Fatalf("the honest weight-1.0 caveat must survive fail-open:\n%s", ledger)
	}
	if got := windowFixtureTopRunningFreq(t, stats, 300); got != 0 {
		t.Fatalf("w3 must stay frequency-less without the reuse gate, got %d", got)
	}
	if strings.Contains(strings.Join(stats.Caveats, "\n"), "cluster-shared frequency reuse") {
		t.Fatalf("no reuse → no window disclosure:\n%s", strings.Join(stats.Caveats, "\n"))
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
