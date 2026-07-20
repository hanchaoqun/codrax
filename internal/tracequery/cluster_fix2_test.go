package tracequery

// cluster_fix2_test.go — CLUSTER-FIX-2 batch pins (audit 底稿 docs/design/
// cluster_audit_code_20260718.md S1/S3/S4-limits + cluster_audit_refs_
// 20260718.md C1/C2/C4; §29.150 用户裁定⑨; §29.129 移交清单):
//
//	件1 S1  — typed freq_only cause enum (closed set), minted on every
//	          freq_only arm; real-fixture pins (tieba single_cluster,
//	          donghu reason-less judged verdict).
//	件2 S3  — rail gate-⑤ universe widened to the typed CPU attribution set
//	          (any-event header CPU + cpu_idle payload); the window-idle
//	          anchor no longer kills the family; the nonexistent-CPU anchor
//	          still does.
//	件3 C1  — the single-co-emission-burst witness behind a comove-floor
//	          trip is DETECTED and disclosed (reason refinement token +
//	          caveat); the floor judgment itself is byte-unchanged (§28.5
//	          复核 P1 ruling stands; hard admission = delegated ruling
//	          point; §29.129 既裁③: the skew constant is never widened).
//	件4 C2  — limits anchors sitting strictly inside a derived cluster are
//	          disclosed (basis roster + caveat); membership consumption
//	          stays a ruling candidate (S9).
//	件5 C4  — the next_info affinity-mask WIDTH joins the attribution
//	          universe as an nr_cpus lower-bound witness; mask VALUES never
//	          infer cluster boundaries.
//	件6 ⑨   — poisoned cpu_frequency_limits lanes are rostered
//	          (droppedLimitCPUs) and disclosed with the isomorphic caveat
//	          (fmax 阶梯可能低估); the drop judgment is byte-unchanged.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- 件1 (S1): typed freq_only cause enum --------------------------------------

// Every resolver arm mints exactly its token; a judged verdict mints none.
func TestClusterFix2FreqOnlyReasonClosedSet(t *testing.T) {
	cases := []struct {
		name       string
		tl         map[int][]freqSample
		topology   string
		wantSource string
		wantReason string
	}{
		{name: "no_domains", tl: map[int][]freqSample{},
			wantSource: CoreCapabilitySourceFreqOnly, wantReason: CoreCapabilityFreqOnlyReasonNoDomains},
		{name: "single_cluster", tl: map[int][]freqSample{0: capTL(2000000)},
			wantSource: CoreCapabilitySourceFreqOnly, wantReason: CoreCapabilityFreqOnlyReasonSingleCluster},
		{name: "cluster_overflow", tl: map[int][]freqSample{
			0: capTL(1000000), 1: capTL(1200000), 2: capTL(1400000),
			3: capTL(1600000), 4: capTL(1800000),
		}, wantSource: CoreCapabilitySourceFreqOnly, wantReason: CoreCapabilityFreqOnlyReasonClusterOverflow},
		{name: "fmax_tie", tl: map[int][]freqSample{
			0: {{ts: 1.0, khz: 2000000}, {ts: 3.0, khz: 1000000}},
			4: {{ts: 2.0, khz: 2000000}, {ts: 4.0, khz: 1500000}},
		}, wantSource: CoreCapabilitySourceFreqOnly, wantReason: CoreCapabilityFreqOnlyReasonFmaxTie},
		// comove floor, NON-burst form: the two 1-sample lanes merge by
		// same-emission identity, but the sampled member ids are NOT one
		// contiguous ascending run (cpu0/cpu2) — the burst refinement must
		// not fire.
		{name: "comove_floor", tl: map[int][]freqSample{
			0: {{ts: 1.0, khz: 1500000}},
			2: {{ts: 1.000005, khz: 1500000}},
		}, wantSource: CoreCapabilitySourceFreqOnly, wantReason: CoreCapabilityFreqOnlyReasonComoveFloor},
		// comove floor, single-burst form (件3/C1): one co-emission burst —
		// exactly one sample per member, equal value, spread inside the FIXED
		// skew bound, contiguous ascending member ids.
		{name: "comove_floor_single_burst", tl: map[int][]freqSample{
			0: {{ts: 1.0, khz: 1500000}},
			1: {{ts: 1.000005, khz: 1500000}},
		}, wantSource: CoreCapabilitySourceFreqOnly, wantReason: CoreCapabilityFreqOnlyReasonComoveFloorSingleBurst},
		// no_sampled_cluster: explicit membership, zero fmax evidence.
		{name: "no_sampled_cluster", tl: map[int][]freqSample{}, topology: "small=0-3;big=4-7",
			wantSource: CoreCapabilitySourceFreqOnly, wantReason: CoreCapabilityFreqOnlyReasonNoSampledCluster},
		// Judged verdict: no reason token.
		{name: "judged_no_reason", tl: map[int][]freqSample{0: capTL(1000000), 4: capTL(2000000)},
			wantSource: CoreCapabilitySourceDefault, wantReason: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			domains := resolveClusterFreqDomains(tc.topology, func() map[int][]freqSample { return tc.tl })
			capability := resolveCoreCapability(domains, tc.tl)
			if capability.source != tc.wantSource {
				t.Fatalf("source = %q, want %q", capability.source, tc.wantSource)
			}
			if capability.freqOnlyReason != tc.wantReason {
				t.Fatalf("freqOnlyReason = %q, want %q", capability.freqOnlyReason, tc.wantReason)
			}
		})
	}
}

// Real-fixture pins: tieba (the customer container single-policy shape, the
// S1 主形) mints single_cluster; donghu's judged 3-cluster verdict mints no
// reason. Both verdicts themselves are byte-unchanged (the ground-truth pins
// in cluster_derive_rnb4_test.go stay the judgment witnesses).
func TestClusterFix2FreqOnlyReasonRealFixtures(t *testing.T) {
	tieba, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Fatalf("BuildIndex(tieba): %v", err)
	}
	capability := newChainQueryCache(tieba, nil).coreCapability("")
	if capability.source != CoreCapabilitySourceFreqOnly || capability.freqOnlyReason != CoreCapabilityFreqOnlyReasonSingleCluster {
		t.Fatalf("tieba must stay freq_only WITH the single_cluster reason, got %q/%q", capability.source, capability.freqOnlyReason)
	}
	if len(capability.limitsAnchorMismatch) != 0 {
		t.Fatalf("tieba has no limits lanes — the C2 roster must stay empty, got %v", capability.limitsAnchorMismatch)
	}
	donghu, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatalf("BuildIndex(donghu): %v", err)
	}
	capabilityD := newChainQueryCache(donghu, nil).coreCapability("")
	if capabilityD.source != CoreCapabilitySourceDefault || capabilityD.freqOnlyReason != "" {
		t.Fatalf("donghu must stay judged with NO reason token, got %q/%q", capabilityD.source, capabilityD.freqOnlyReason)
	}
	// C2 negative witness (件4): donghu's limits anchors {0,4} are exactly
	// the first members of the first two derived clusters — no mismatch.
	if len(capabilityD.limitsAnchorMismatch) != 0 {
		t.Fatalf("donghu anchors {0,4} are cluster starts — the C2 roster must be empty, got %v", capabilityD.limitsAnchorMismatch)
	}
}

// The reason token rides the fold basis wire iff the verdict is freq_only
// (single-cluster end-to-end shape: one sampled lane, dep runs on its
// sibling; the fold degrades to the pure frequency ratio and now names WHY).
func TestClusterFix2FreqOnlyReasonRidesFoldBasis(t *testing.T) {
	idx := buildTraceIndex(t, "fix2_single_cluster.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1800000 cpu_id=2
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.000000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [002] .... 5.009900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [002] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	dep := supplyFoldDepImpact(t, chain)
	basis := dep.SupplyFoldBasis
	if basis == nil || basis.CapabilitySource != CoreCapabilitySourceFreqOnly {
		t.Fatalf("single-lane trace must fold freq_only, got %+v", basis)
	}
	if basis.CapabilityFreqOnlyReason != CoreCapabilityFreqOnlyReasonSingleCluster {
		t.Fatalf("the typed cause must ride the basis: got %q, want %q", basis.CapabilityFreqOnlyReason, CoreCapabilityFreqOnlyReasonSingleCluster)
	}
}

// --- 件2/件5 (S3/C4): the rail attribution universe ----------------------------

// Unit census of the universe builder: the three closed arms admit, the
// anti-circularity exclusions hold.
func TestClusterFix2RailCPUAttributionUniverse(t *testing.T) {
	events := []Event{
		// (a) any event's header CPU.
		{Type: EventSchedSwitch, CPU: 3},
		{Type: EventTraceMark, CPU: 5},
		// (b) cpu_idle payload cpu_id.
		{Type: EventCPUIdle, CPU: 0, CPUForField: 7, CPUForFieldValid: true},
		// Anti-circularity: a clock rail's payload cpu_id must NOT self-attest
		// (neither the raw clock_set_rate form nor the cpu-freq-named
		// reclassified form) — only its header CPU joins.
		{Type: EventClockSetRate, Name: "clock_set_rate", ClockName: "m3_c9_freq", CPU: 1, CPUForField: 9, CPUForFieldValid: true, Frequency: 1000000},
		{Type: EventCPUFrequency, Name: "clock_set_rate", ClockName: "m3_c8_freq", CPU: 1, CPUForField: 8, CPUForFieldValid: true, Frequency: 1000000},
	}
	universe := railCPUAttributionUniverse(events)
	for _, cpu := range []int{0, 1, 3, 5, 7} {
		if !universe[cpu] {
			t.Fatalf("cpu%d must be attributed, universe=%v", cpu, universe)
		}
	}
	for _, cpu := range []int{8, 9} {
		if universe[cpu] {
			t.Fatalf("cpu%d is only named by a rail payload key — it must NOT self-attest into the gate-⑤ universe (circularity), universe=%v", cpu, universe)
		}
	}
	// (c) C4: the affinity-mask WIDTH is an nr_cpus lower-bound witness —
	// 3fff → 14 → cpus 0..13 exist (donghu 字面见证); a restricted per-task
	// mask (0xf) only witnesses width 4.
	wide := railCPUAttributionUniverse([]Event{
		{Type: EventSchedSwitch, CPU: 0, NextInfoAllowedCPUs: parseCPUMaskHex("3fff")},
	})
	for cpu := 0; cpu <= 13; cpu++ {
		if !wide[cpu] {
			t.Fatalf("mask 3fff (width 14) must witness cpu%d, universe=%v", cpu, wide)
		}
	}
	if wide[14] {
		t.Fatalf("mask 3fff must not witness cpu14, universe=%v", wide)
	}
	narrow := railCPUAttributionUniverse([]Event{
		{Type: EventSchedSwitch, CPU: 0, NextInfoAllowedCPUs: parseCPUMaskHex("f")},
	})
	if narrow[4] || !narrow[3] {
		t.Fatalf("mask f (width 4) must witness exactly cpus 0..3, universe=%v", narrow)
	}
}

// 判定行为改动 witness (件2, before/after in one pin): a family whose top
// anchors' CPUs are window-idle (cpu_idle rows only, zero sched_switch) was
// rejected on gate ⑤ under the old sched-only universe and is ADOPTED under
// the attribution universe; an anchor naming a CPU with NO attribution at
// all still rejects (gate ⑤ stays a hard precise gate).
func TestClusterFix2RailGateIdleAnchorAdoptsNonexistentStillRejects(t *testing.T) {
	rails := `
    tppmgr-idle-0-296   (    2) [000] .... 15151.855000: clock_set_rate: m3_c0_freq state=417000 cpu_id=0
    tppmgr-idle-0-296   (    2) [000] .... 15151.855002: clock_set_rate: m3_c1_freq state=1200000 cpu_id=10
`
	idle := `
   <idle>-0 (-----) [010] .... 15151.850000: cpu_idle: state=1 cpu_id=10
`
	sched := `
   filler-800 (  900) [000] .... 15151.820000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=filler next_pid=900 next_prio=120
`
	idx := buildTraceIndex(t, "fix2_idle_anchor.systrace", sched+idle+rails)
	// BEFORE form (the retired sched-only universe, direct-arm control): the
	// anchor-10 family died on ⑤ exactly as the S3 audit measured.
	oldUniverse := map[int]bool{0: true}
	if scan := scanClusterRailEvidence(idx.Events, oldUniverse); scan.adoption != nil || scan.rejected["m3_c#_freq"] != clusterRailRejectAnchorOutsideCPUs {
		t.Fatalf("control: the sched-only universe must reject the idle anchor (window carve over-kill), got %+v/%v", scan.adoption, scan.rejected)
	}
	// AFTER form (production wiring): cpu_idle attributes cpu10 → adopted;
	// the membership bound reaches the highest attributed CPU.
	cache := newChainQueryCache(idx, nil)
	scan := scanClusterRailEvidence(idx.Events, cache.cpuAttributionUniverse())
	if scan.adoption == nil {
		t.Fatalf("the attribution universe must adopt the idle-anchor family (窗内没调度 ≠ CPU 不存在), rejected=%v", scan.rejected)
	}
	last := scan.adoption.clusters[len(scan.adoption.clusters)-1]
	if last.anchor != 10 || last.members[len(last.members)-1] != 10 {
		t.Fatalf("membership bound must reach the attributed cpu10, got %+v", last)
	}
	// Gate ⑤ stays hard: an anchor naming a CPU nothing attributes rejects.
	ghost := strings.ReplaceAll(rails, "cpu_id=10", "cpu_id=12")
	idxGhost := buildTraceIndex(t, "fix2_ghost_anchor.systrace", sched+idle+ghost)
	cacheGhost := newChainQueryCache(idxGhost, nil)
	if scan := scanClusterRailEvidence(idxGhost.Events, cacheGhost.cpuAttributionUniverse()); scan.adoption != nil || scan.rejected["m3_c#_freq"] != clusterRailRejectAnchorOutsideCPUs {
		t.Fatalf("an anchor with NO attribution must still reject on ⑤, got %+v/%v", scan.adoption, scan.rejected)
	}
}

// C4 end-to-end (件5): with no sched/idle coverage beyond cpu0, the HarmonyOS
// next_info mask width alone attests the platform's core span and lets the
// anchor-12 family through gate ⑤; membership extends to the mask width.
func TestClusterFix2RailGateMaskWidthWitness(t *testing.T) {
	body := `
   filler-800 (  900) [000] .... 15151.820000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=filler next_pid=900 next_prio=120 next_info=3fff,100,3,0,2,0
    tppmgr-idle-0-296   (    2) [000] .... 15151.855000: clock_set_rate: m3_c0_freq state=417000 cpu_id=0
    tppmgr-idle-0-296   (    2) [000] .... 15151.855002: clock_set_rate: m3_c1_freq state=2350000 cpu_id=12
`
	idx := buildTraceIndex(t, "fix2_mask_width.systrace", body)
	cache := newChainQueryCache(idx, nil)
	scan := scanClusterRailEvidence(idx.Events, cache.cpuAttributionUniverse())
	if scan.adoption == nil {
		t.Fatalf("the 3fff mask width (nr_cpus=14) must attest cpu12 for gate ⑤, rejected=%v", scan.rejected)
	}
	last := scan.adoption.clusters[len(scan.adoption.clusters)-1]
	if got := last.members[len(last.members)-1]; got != 13 {
		t.Fatalf("the membership bound must extend to the mask width (cpu13), got cpu%d (%+v)", got, last)
	}
}

// --- 件3 (C1): the single-burst witness stays disclosure-only ------------------

// The floor JUDGMENT is byte-unchanged on the single-burst form (freq_only,
// comoveFloorTripped, priced at 1) — only the reason token refines. The
// engine caveat lifts the burst disclosure with the ruling sentence.
func TestClusterFix2SingleBurstFloorJudgmentUnchangedAndDisclosed(t *testing.T) {
	tl := map[int][]freqSample{
		0: {{ts: 1.0, khz: 1500000}},
		1: {{ts: 1.000005, khz: 1500000}},
	}
	capability := resolveCoreCapability(deriveClusterFreqDomains(tl), tl)
	if capability.source != CoreCapabilitySourceFreqOnly || !capability.comoveFloorTripped {
		t.Fatalf("the §28.5 复核 P1 floor must still trip on a single burst (判定零改), got %q floor=%v", capability.source, capability.comoveFloorTripped)
	}
	if got := capability.capabilityFor(0); got != 1 {
		t.Fatalf("single-burst floor form must price at 1, got %v", got)
	}
	if capability.freqOnlyReason != CoreCapabilityFreqOnlyReasonComoveFloorSingleBurst {
		t.Fatalf("the burst witness must be disclosed via the reason token, got %q", capability.freqOnlyReason)
	}
	// Caveat lift: the disclosure names the ruled floor and its own inertness.
	res := Result{RootCauseRank: &RootCauseRankResult{Items: []RootCauseRankItem{{
		SupplyFoldBasis: &SupplyFoldBasis{
			CapabilitySource:         CoreCapabilitySourceFreqOnly,
			CapabilityFreqOnlyReason: CoreCapabilityFreqOnlyReasonComoveFloorSingleBurst,
		},
	}}}}
	caveats := clusterFixTwoDisclosureCaveats(res)
	if len(caveats) != 1 || !strings.Contains(caveats[0], "comove_floor_single_burst") ||
		!strings.Contains(caveats[0], "≥2 共见证变迁裁定门") || !strings.Contains(caveats[0], "不参与任何判定") {
		t.Fatalf("the burst caveat must carry the ruling sentence, got %v", caveats)
	}
}

// The burst detector's four literal conditions each reject independently.
func TestClusterFix2SingleBurstWitnessConditions(t *testing.T) {
	base := func() map[int][]freqSample {
		return map[int][]freqSample{
			0: {{ts: 1.0, khz: 1500000}},
			1: {{ts: 1.000005, khz: 1500000}},
		}
	}
	if !comoveFloorSingleBurstWitness([]int{0, 1}, base()) {
		t.Fatalf("the canonical single burst must witness")
	}
	twoSamples := base()
	twoSamples[0] = append(twoSamples[0], freqSample{ts: 2.0, khz: 1600000})
	if comoveFloorSingleBurstWitness([]int{0, 1}, twoSamples) {
		t.Fatalf("(1) a second sample must defeat the single-burst claim")
	}
	diffValue := base()
	diffValue[1] = []freqSample{{ts: 1.000005, khz: 1600000}}
	if comoveFloorSingleBurstWitness([]int{0, 1}, diffValue) {
		t.Fatalf("(2) unequal values must defeat the burst claim")
	}
	wideSkew := base()
	wideSkew[1] = []freqSample{{ts: 1.001, khz: 1500000}}
	if comoveFloorSingleBurstWitness([]int{0, 1}, wideSkew) {
		t.Fatalf("(3) a spread beyond the FIXED skew bound must defeat the burst claim (§29.129 既裁③: no adaptive widening)")
	}
	gap := map[int][]freqSample{
		0: {{ts: 1.0, khz: 1500000}},
		2: {{ts: 1.000005, khz: 1500000}},
	}
	if comoveFloorSingleBurstWitness([]int{0, 2}, gap) {
		t.Fatalf("(4) non-contiguous member ids must defeat the burst claim")
	}
}

// --- 件4 (C2): limits anchor consistency disclosure ----------------------------

// A limits anchor strictly inside a derived cluster is rostered; the verdict
// itself is untouched (判定零改 witness: classes/fmax identical with and
// without the interior anchor's lane, because its values sit below the
// observed maxima).
func TestClusterFix2LimitsAnchorMismatchDisclosedJudgmentUnchanged(t *testing.T) {
	// Two derived clusters {0,1} and {2,3} (identical co-moving member
	// timelines, ≥2 samples so the floor stays quiet); the limits anchor
	// cpu1 sits strictly INSIDE the first cluster.
	tl := map[int][]freqSample{
		0: {{ts: 1.0, khz: 800000}, {ts: 2.0, khz: 1000000}},
		1: {{ts: 1.000002, khz: 800000}, {ts: 2.000002, khz: 1000000}},
		2: {{ts: 1.0, khz: 1500000}, {ts: 2.0, khz: 2000000}},
		3: {{ts: 1.000002, khz: 1500000}, {ts: 2.000002, khz: 2000000}},
	}
	domains := deriveClusterFreqDomains(tl)
	limits := map[int][]freqSample{1: {{ts: 1.5, khz: 900000}}}
	with := resolveCoreCapabilityEvidence(domains, tl, limits, nil)
	without := resolveCoreCapabilityEvidence(domains, tl, nil, nil)
	if with.source != CoreCapabilitySourceDefault || without.source != CoreCapabilitySourceDefault {
		t.Fatalf("both forms must judge, got %q/%q", with.source, without.source)
	}
	for cpu := 0; cpu <= 2; cpu++ {
		if with.capabilityFor(cpu) != without.capabilityFor(cpu) {
			t.Fatalf("判定零改: the interior anchor must not change cpu%d's class pricing", cpu)
		}
	}
	if fmt.Sprint(with.limitsAnchorMismatch) != "[1]" {
		t.Fatalf("the interior anchor cpu1 must be rostered, got %v", with.limitsAnchorMismatch)
	}
	if len(without.limitsAnchorMismatch) != 0 {
		t.Fatalf("no limits → no roster, got %v", without.limitsAnchorMismatch)
	}
	// Cluster-start anchors are consistent — silent (donghu shape).
	startAnchors := map[int][]freqSample{0: {{ts: 1.5, khz: 900000}}, 2: {{ts: 1.5, khz: 1800000}}}
	if got := resolveCoreCapabilityEvidence(domains, tl, startAnchors, nil); len(got.limitsAnchorMismatch) != 0 {
		t.Fatalf("cluster-start anchors must stay silent, got %v", got.limitsAnchorMismatch)
	}
	// Explicit topology is user-authoritative — no mismatch minted.
	explicit := parseClusterFreqDomains("small=0-1;big=2-3")
	if got := resolveCoreCapabilityEvidence(explicit, tl, limits, nil); len(got.limitsAnchorMismatch) != 0 {
		t.Fatalf("the C2 roster is derived-lane only, got %v", got.limitsAnchorMismatch)
	}
	// An anchor with no membership (above the highest sampled core, <3
	// domains) makes no partition claim.
	orphan := map[int][]freqSample{9: {{ts: 1.5, khz: 900000}}}
	if got := resolveCoreCapabilityEvidence(domains, tl, orphan, nil); len(got.limitsAnchorMismatch) != 0 {
		t.Fatalf("an unassigned anchor must stay silent, got %v", got.limitsAnchorMismatch)
	}
	// Caveat lift: the disclosure names the S9 ruling-candidate boundary.
	res := Result{RootCauseRank: &RootCauseRankResult{Items: []RootCauseRankItem{{
		SupplyFoldBasis: &SupplyFoldBasis{ClusterLimitsAnchorMismatch: []int{1}},
	}}}}
	caveats := clusterFixTwoDisclosureCaveats(res)
	if len(caveats) != 1 || !strings.Contains(caveats[0], "cluster_limits_anchor_mismatch=cpu1") ||
		!strings.Contains(caveats[0], "成员判定不消费 limits 锚") {
		t.Fatalf("the C2 caveat must disclose the anchor and the non-consumption boundary, got %v", caveats)
	}
}

// --- 件6 (裁定⑨): poisoned limits lanes rostered + isomorphic caveat -----------

// A physical same-lane rollback on a cpu_frequency_limits lane still drops
// the lane (judgment unchanged — the long-standing fail-close) and is now
// rostered and disclosed with the fmax-ladder caveat, isomorphic to the
// CLUSTER-FIX-1 cpu_frequency form.
func TestClusterFix2LimitsIntegrityDropDisclosed(t *testing.T) {
	trace := `
   probe-10  (   10) [000] .... 1.000000: cpu_frequency: state=800000 cpu_id=0
   probe-10  (   10) [000] .... 1.000100: cpu_frequency_limits: min=417000 max=1750000 cpu_id=4
   probe-10  (   10) [000] .... 2.000000: cpu_frequency: state=900000 cpu_id=0
   probe-10  (   10) [000] .... 2.000100: cpu_frequency_limits: min=417000 max=1550000 cpu_id=4
   probe-10  (   10) [000] .... 1.500000: cpu_frequency_limits: min=417000 max=1650000 cpu_id=4
   probe-10  (   10) [000] .... 3.000000: sched_switch: prev_comm=probe prev_pid=10 prev_prio=100 prev_state=S ==> next_comm=app next_pid=100 next_prio=100
`
	path := filepath.Join(t.TempDir(), "limits_rollback.systrace")
	if err := os.WriteFile(path, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !idx.fullFreq.collected {
		t.Fatalf("fixture setup: the complete build must publish full curves")
	}
	if fmt.Sprint(idx.fullFreq.droppedLimitCPUs) != "[4]" {
		t.Fatalf("the poisoned limits lane must be rostered at collection, got %v", idx.fullFreq.droppedLimitCPUs)
	}
	if len(idx.fullFreq.limitByCPU[4]) != 0 {
		t.Fatalf("the drop judgment itself must stay unchanged (cpu4 limits out)")
	}
	// The freq lane is untouched by the limits poison (cpu0 keeps its curve).
	if len(idx.fullFreq.freqByCPU[0]) == 0 {
		t.Fatalf("the cpu_frequency lane must be unaffected by a limits-lane rollback")
	}
	basis, dropped := indexClusterSampleBasis(idx)
	if basis != ClusterSampleBasisFullIndex || len(dropped) != 0 {
		t.Fatalf("the cpu_frequency dropped roster must stay EMPTY on a limits-only rollback, got %q/%v", basis, dropped)
	}
	if fmt.Sprint(idx.freqLimitTimelinesDropped) != "[4]" {
		t.Fatalf("the limits dropped memo must carry cpu4, got %v", idx.freqLimitTimelinesDropped)
	}
	caveats := resultCaveats(idx, Query{}, Result{})
	found := false
	for _, caveat := range caveats {
		if strings.Contains(caveat, "cluster_freq_limits_integrity_dropped_cpus=cpu4") && strings.Contains(caveat, "fmax 阶梯可能低估") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the 裁定⑨ limits drop must be disclosed on the caveat lane, got %v", caveats)
	}
	// The buildFreqLimitIndex consumer keeps its identical fail-close drop.
	cache := newChainQueryCache(idx, nil)
	cache.buildFreqLimitIndex()
	if len(cache.freqLimitByCPU[4]) != 0 {
		t.Fatalf("the limits consumer must keep the fail-close drop, got %v", cache.freqLimitByCPU[4])
	}
	// Window-carve arm: the same disclosure on an events-basis synthetic
	// index (the FIX-1 carve-arm mirror).
	carve := &Index{Events: []Event{
		{Type: EventCPUFrequencyLimit, Name: "cpu_frequency_limits", Ts: 1.0, Frequency: 1000000, FrequencyMax: 1750000, CPUForField: 4, CPUForFieldValid: true},
		{Type: EventCPUFrequencyLimit, Name: "cpu_frequency_limits", Ts: 2.0, Frequency: 1000000, FrequencyMax: 1550000, CPUForField: 4, CPUForFieldValid: true},
		{Type: EventCPUFrequencyLimit, Name: "cpu_frequency_limits", Ts: 1.5, Frequency: 1000000, FrequencyMax: 1650000, CPUForField: 4, CPUForFieldValid: true},
	}}
	basisC, droppedC := indexClusterSampleBasis(carve)
	if basisC != ClusterSampleBasisWindowCarve || len(droppedC) != 0 {
		t.Fatalf("carve arm: freq roster must stay empty, got %q/%v", basisC, droppedC)
	}
	if fmt.Sprint(carve.freqLimitTimelinesDropped) != "[4]" {
		t.Fatalf("carve arm: the limits dropped memo must carry cpu4, got %v", carve.freqLimitTimelinesDropped)
	}
}
