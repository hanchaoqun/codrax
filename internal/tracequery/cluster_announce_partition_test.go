package tracequery

// cluster_announce_partition_test.go — CLUSTERTIE-1 件A/件B (§29.200,
// 2026-07-21) pin family: the announcement snapshot partition lane.
//
// Fleet witness (grep_result.txt / cluster_report.txt, record_trace_
// 20260526170707@880): the customer collector re-announces every core's
// STANDING frequency every ~1ms (cpu0-3=1600000 / cpu4-9=2151000 /
// cpu10-11=2500000, constant across 200+ sweeps, one on-duty thread per
// sweep). Zero transitions ⇒ witness lane pro=0 ⇒ the honest co_witness_floor
// arm — the exact split_audit line the customer's N1 replay returned
// ("cpu0↔cpu1 @925.310393 判定臂=co_witness_floor(共见证变迁不足:共见证=0
// (<2))"). The partition lane reads the sweeps themselves as structure
// evidence and judges the three clusters.
//
// 件B verdict pinned here: freqTimelinesSameEmission is NOT buggy — it merges
// the equal-length constant form exactly as designed; its all-or-nothing
// per-index identity breaks on ONE lost row anywhere (per-CPU ring buffers
// make that the fleet norm), and pro=0 means the witness lane cannot rescue
// it. The snapshot partition is the cure, not a fast-path repair.
// 冷读 P3-1 (2026-07-21): the lost-row arm is the pinned CANDIDATE mechanism,
// unproven on the customer FULL file (the local grep excerpt was head-400
// truncated; §29.200 疑点并查 lists three candidate arms — 首值差/长度差/0 值
// 离线记号 — and the cure holds for all three); discrimination rides revisit
// N1 (customer_revisit_guide_20260720.md, per-CPU 行数/首值/0 值统计回传).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// announceSweepTimelines synthesizes the customer sweep shape: groups of
// constant values re-announced every cadence seconds with realistic
// intra-sweep member offsets (µs), optionally dropping (sweep, cpu) rows.
func announceSweepTimelines(groups map[int64][]int, offsetsUs map[int]float64, base, cadence float64, sweeps int, drop map[int]map[int]bool) map[int][]freqSample {
	out := map[int][]freqSample{}
	for khz, cpus := range groups {
		for _, cpu := range cpus {
			for k := 0; k < sweeps; k++ {
				if drop[k][cpu] {
					continue
				}
				out[cpu] = append(out[cpu], freqSample{ts: base + float64(k)*cadence + offsetsUs[cpu]*1e-6, khz: khz})
			}
		}
	}
	return out
}

var announceCustomerGroups = map[int64][]int{
	1600000: {0, 1, 2, 3},
	2151000: {4, 5, 6, 7, 8, 9},
	2500000: {10, 11},
}

// announceCustomerOffsets mirrors the grep_result sweep geometry: the whole
// 12-core sweep spans ~23µs, every consecutive gap ≤10µs.
var announceCustomerOffsets = map[int]float64{
	0: 0, 1: 2, 2: 2.5, 3: 3, 4: 13, 5: 14, 6: 14.5, 7: 15, 8: 16, 9: 17, 10: 22, 11: 23,
}

// The customer form with ONE lost row (cpu1 missing from sweep 100 — the
// per-CPU ring-buffer norm): sameEmission is defeated, the witness lane holds
// zero transitions, and the partition lane judges the three clusters — the
// former co_witness_floor / fragment-tie verdicts die (§29.200④: the earlier
// 「簇最高频并列」 read was fragment same-value parity, gone once membership
// judges).
func TestAnnouncePartitionJudgesCustomerConstantSweepForm(t *testing.T) {
	drop := map[int]map[int]bool{100: {1: true}}
	tls := announceSweepTimelines(announceCustomerGroups, announceCustomerOffsets, 925.310391, 0.001, 200, drop)
	// 件B diagnosis anchors: the lost row breaks the same-emission identity
	// (length mismatch) and the constant stream carries zero witnesses.
	if freqTimelinesSameEmission(tls[0], tls[1]) {
		t.Fatalf("fixture: one lost row must defeat the same-emission identity")
	}
	if w := freqWitnessScanPair(tls[0], tls[1]); w.pro != 0 || w.con != 0 {
		t.Fatalf("fixture: a constant announcement stream mints zero witnesses, got %+v", w)
	}
	d := deriveClusterFreqDomains(tls)
	if d.groupCount != 3 {
		t.Fatalf("the partition lane must judge three clusters, got %+v", d.members)
	}
	for _, pair := range [][2]int{{0, 1}, {0, 3}, {4, 9}, {10, 11}} {
		if d.byCPU[pair[0]] != d.byCPU[pair[1]] {
			t.Fatalf("cpus %v must share a cluster, got %+v", pair, d.members)
		}
	}
	for _, pair := range [][2]int{{0, 4}, {9, 10}} {
		if d.byCPU[pair[0]] == d.byCPU[pair[1]] {
			t.Fatalf("cpus %v must stay in distinct clusters, got %+v", pair, d.members)
		}
	}
	capability := resolveCoreCapability(d, tls)
	if capability.source != CoreCapabilitySourceDefault || capability.freqOnlyReason != "" {
		t.Fatalf("the judged verdict must leave freq_only entirely, got %q/%q", capability.source, capability.freqOnlyReason)
	}
	wantClass := map[int]string{0: coreCapabilityClassSmall, 4: coreCapabilityClassMiddle, 10: coreCapabilityClassBig}
	for cpu, want := range wantClass {
		if got := capability.classByCluster[capability.clusterLabelFor(cpu)]; got != want {
			t.Fatalf("cpu%d class = %q, want %q (%+v)", cpu, got, want, capability.classByCluster)
		}
	}
	// Three distinct fmax (1600/2151/2500) — no tie, so no tie-break audit.
	if capability.fmaxTieBreakAudit != "" {
		t.Fatalf("distinct group fmax must not mint a tie-break audit, got %q", capability.fmaxTieBreakAudit)
	}
}

// The intact form (no lost row): the same-emission fast path merges each
// value group exactly as designed — 件B: the fast path is not the bug.
func TestAnnouncePartitionSameEmissionFastPathIntactForm(t *testing.T) {
	tls := announceSweepTimelines(announceCustomerGroups, announceCustomerOffsets, 925.310391, 0.001, 50, nil)
	if !freqTimelinesSameEmission(tls[0], tls[1]) {
		t.Fatalf("equal-length constant same-value lanes must satisfy the same-emission identity")
	}
	d := deriveClusterFreqDomains(tls)
	if d.groupCount != 3 {
		t.Fatalf("intact form must judge three clusters, got %+v", d.members)
	}
}

// Partition drift kills the whole signal (fail-open): one sweep where cpu4
// announces the small-group value changes the grouping — no partition merge,
// the honest floor split stands.
func TestAnnouncePartitionDriftFailsOpen(t *testing.T) {
	drop := map[int]map[int]bool{}
	tls := announceSweepTimelines(announceCustomerGroups, announceCustomerOffsets, 925.310391, 0.001, 20, drop)
	// Rewrite cpu4's sweep-10 row to the small-group value (grouping drift).
	for i := range tls[4] {
		if i == 10 {
			tls[4][i].khz = 1600000
		}
	}
	d := deriveClusterFreqDomains(tls)
	if d.byCPU[0] == d.byCPU[1] {
		// cpu0/cpu1 still merge via same-emission (equal constant lanes) —
		// the assertion below is the partition-specific pair.
		t.Logf("same-emission still merges the intact twins (expected)")
	}
	// The drifted stream must NOT mint partition merges: cpu4 now splits
	// from its group (its lane differs → same-emission false, witnesses:
	// its lone transition finds no same-value partner → floor).
	if d.byCPU[4] == d.byCPU[5] {
		t.Fatalf("a drifted partition must not merge cpu4 back (fail-open), got %+v", d.members)
	}
}

// Snapshot floor: one full sweep is not structure (§28.5 P1 rationale, same
// constant as the witness floor) — two singleton announcements 1s apart plus
// one co-covering sweep stay split.
func TestAnnouncePartitionSnapshotFloor(t *testing.T) {
	tls := map[int][]freqSample{
		0: {{ts: 10.0, khz: 1600000}},
		4: {{ts: 10.000005, khz: 2151000}},
	}
	if d := deriveClusterFreqDomains(tls); d.groupCount != 2 {
		t.Fatalf("fixture: distinct values stay split regardless, got %+v", d.members)
	}
	// Same-value pair covered by ONE sweep only: below the snapshot floor.
	one := map[int][]freqSample{
		0: {{ts: 10.0, khz: 1600000}},
		1: {{ts: 10.000002, khz: 1600000}},
		4: {{ts: 10.000010, khz: 2151000}},
	}
	// (cpu0,cpu1) would merge by same-emission? Lengths equal, values equal,
	// skew 2µs — yes; defeat it with a second cpu0-only announcement so the
	// partition lane is the only candidate, then assert it refuses.
	one[0] = append(one[0], freqSample{ts: 11.0, khz: 1600000})
	if freqTimelinesSameEmission(one[0], one[1]) {
		t.Fatalf("fixture: lengths must differ")
	}
	if d := deriveClusterFreqDomains(one); d.byCPU[0] == d.byCPU[1] {
		t.Fatalf("one full snapshot sits below the snapshot floor — no merge, got %+v", d.members)
	}
}

// §28.5 毒形 disposal (§29.200 verbatim): every core parked at ONE value —
// the single value group merges honestly (all members held one frequency the
// whole trace: frequency-ratio pricing is value-identical either way,
// capRatio 恒 1 无损) and the capability layer words the single-cluster form.
func TestAnnouncePartitionSingleValueGroupHonestMerge(t *testing.T) {
	groups := map[int64][]int{1600000: {0, 1, 2, 3}}
	offsets := map[int]float64{0: 0, 1: 2, 2: 4, 3: 6}
	drop := map[int]map[int]bool{3: {2: true}} // defeat same-emission on one lane
	tls := announceSweepTimelines(groups, offsets, 10.0, 0.001, 10, drop)
	d := deriveClusterFreqDomains(tls)
	if d.groupCount != 1 {
		t.Fatalf("the parked single-value form merges honestly, got %+v", d.members)
	}
	capability := resolveCoreCapability(d, tls)
	if capability.source != CoreCapabilitySourceFreqOnly ||
		capability.freqOnlyReason != CoreCapabilityFreqOnlyReasonSingleCluster {
		t.Fatalf("the merged blob must word the single-cluster form, got %q/%q", capability.source, capability.freqOnlyReason)
	}
}

// limits 副证 sub-veto (§29.200 处置): a value group whose members carry two
// DISTINCT positive limits ceilings is policy-boundary contradicted — the
// group mints no partition merges (fail-open, no guessed sub-membership);
// the split pair then falls to the fmax ladder (here: chain (a) orders the
// two pressed-together policies by their ceilings — a judged verdict with
// the tie-break audit, not a guessed merge).
func TestAnnouncePartitionLimitsSubVeto(t *testing.T) {
	groups := map[int64][]int{1800000: {0, 1}}
	offsets := map[int]float64{0: 0, 1: 2}
	drop := map[int]map[int]bool{5: {1: true}} // defeat same-emission
	tls := announceSweepTimelines(groups, offsets, 10.0, 0.001, 20, drop)
	limits := map[int][]freqSample{
		0: {{ts: 9.0, khz: 2200000}},
		1: {{ts: 9.0, khz: 2295000}},
	}
	d := deriveClusterFreqDomainsLimits(tls, limits)
	if d.byCPU[0] == d.byCPU[1] {
		t.Fatalf("two distinct limits ceilings inside one value group must veto the merge, got %+v", d.members)
	}
	// The split pair ties on announce value 1800000 < both ceilings? No —
	// fmax = max(limits, observed): 2200000 vs 2295000, distinct → judged
	// without any tie machinery. Assert the honest downstream shape.
	capability := resolveCoreCapabilityEvidence(d, tls, limits, nil)
	if capability.source != CoreCapabilitySourceDefault {
		t.Fatalf("distinct ceilings order the split policies, got %q/%q", capability.source, capability.freqOnlyReason)
	}
	if capability.fmaxTieBreakAudit != "" {
		t.Fatalf("distinct fmax needs no tie-break, got %q", capability.fmaxTieBreakAudit)
	}
	// The same shape with EQUAL announce-above-ceiling observed values —
	// the true chain-(a) form: both fmax read the observed 2400000 tie and
	// the limits ceilings break it.
	for cpu := range tls {
		tls[cpu] = append(tls[cpu], freqSample{ts: 30.0 + float64(cpu)*1e-3, khz: 2400000})
	}
	d = deriveClusterFreqDomainsLimits(tls, limits)
	if d.byCPU[0] == d.byCPU[1] {
		t.Fatalf("the sub-veto must hold with the observed tail too, got %+v", d.members)
	}
	capability = resolveCoreCapabilityEvidence(d, tls, limits, nil)
	if capability.source != CoreCapabilitySourceDefault ||
		!strings.Contains(capability.fmaxTieBreakAudit, "破局链="+coreCapabilityTieBreakChainLimits) {
		t.Fatalf("the observed tie must fall to chain (a), got %q audit %q", capability.source, capability.fmaxTieBreakAudit)
	}
}

// End-to-end flagship witness: the synthesized 925-window customer form as a
// REAL ftrace file through BuildIndex → the full capability resolution
// (limits ladder + rail scan included) judges three clusters — the report
// chain that used to print 13 rows of 「核类排序不可判」 now walks the judged
// fork end to end.
func TestAnnouncePartitionCustomerFormEndToEnd(t *testing.T) {
	var b strings.Builder
	b.WriteString("# tracer: nop\n")
	cpus := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	valueOf := func(cpu int) int64 {
		switch {
		case cpu <= 3:
			return 1600000
		case cpu <= 9:
			return 2151000
		default:
			return 2500000
		}
	}
	for k := 0; k < 200; k++ {
		base := 925.310391 + float64(k)*0.001
		for _, cpu := range cpus {
			if k == 100 && cpu == 1 {
				continue // the lost ring-buffer row
			}
			ts := base + announceCustomerOffsets[cpu]*1e-6
			fmt.Fprintf(&b, " wk:1/1/0/12-4699  (    2) [001] .... %.6f: cpu_frequency: state=%d cpu_id=%d\n", ts, valueOf(cpu), cpu)
		}
	}
	path := filepath.Join(t.TempDir(), "announce_sweeps.systrace")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	cache := &chainQueryCache{idx: idx}
	capability := cache.coreCapability("")
	if capability.source != CoreCapabilitySourceDefault || capability.freqOnlyReason != "" {
		t.Fatalf("end-to-end customer form must judge, got %q/%q (split audit %q)",
			capability.source, capability.freqOnlyReason, capability.freqOnlySplitAudit)
	}
	if got := len(capability.classByCluster); got != 3 {
		t.Fatalf("want three judged clusters, got %d (%+v)", got, capability.classByCluster)
	}
	wantFmax := map[int]int64{0: 1600000, 4: 2151000, 10: 2500000}
	for cpu, want := range wantFmax {
		if got := capability.fmaxByCluster[capability.clusterLabelFor(cpu)]; got != want {
			t.Fatalf("cpu%d cluster fmax = %d, want %d", cpu, got, want)
		}
	}
	fm, refCap, refClass := cache.supplyFoldGlobalMaxBasis(capability)
	if fm.khz != 2500000 || refClass != coreCapabilityClassBig || refCap != coreCapabilityDefaultBig {
		t.Fatalf("fold basis must be the judged big cluster (2500000/big/2.53), got %d/%s/%v", fm.khz, refClass, refCap)
	}
}

// PARTDISC-1 NP1: a partition lane that observed one full snapshot and then
// rejected a changed grouping must disclose that refusal through the real
// Run → SupplyFoldBasis → Result.Caveats path. The capability verdict and
// every fold value remain owned by the existing witness/fmax lanes.
func TestRunLiftsPartitionDriftRefusalCaveat(t *testing.T) {
	var b strings.Builder
	row := func(ts float64, khz, cpu int) {
		fmt.Fprintf(&b, "  tppmgr-sched-in-5850  (    2) [001] .... %.6f: cpu_frequency: state=%d cpu_id=%d\n", ts, khz, cpu)
	}
	// Full snapshot 1: all four CPUs share one value group.
	row(4.900000, 1430000, 0)
	row(4.900001, 1430000, 1)
	row(4.900002, 1430000, 4)
	row(4.900003, 1430000, 8)
	// Defeat same-emission for the parked twins without creating witnesses.
	row(4.930000, 1430000, 4)
	row(4.940000, 1430000, 8)
	// Full snapshot 2: {0,1}|{4,8}; the partition grouping drifted.
	row(4.950000, 1530000, 0)
	row(4.950001, 1530000, 1)
	row(4.950002, 1430000, 4)
	row(4.950003, 1430000, 8)
	row(4.970000, 1430000, 8)

	b.WriteString("        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n")
	b.WriteString("        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\n")
	b.WriteString("        dep-200 (100) [000] .... 5.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20\n")
	b.WriteString("        dep-200 (100) [000] .... 5.009900: sched_wakeup: comm=app pid=100 prio=53 target_cpu=001\n")
	b.WriteString("        dep-200 (100) [000] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120\n")
	b.WriteString("        app-100 (100) [001] .... 5.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n")

	idx := buildTraceIndex(t, "partdisc_drift_run.systrace", b.String())
	res := Run(idx, Query{
		View: "wakeup_chain", PID: 100, TimeStart: 5.0, TimeEnd: 5.010,
		MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace,
	})
	joined := strings.Join(res.Caveats, "\n")
	for _, needle := range []string{
		"capability_freq_only_split_audit=",
		"分区车道=partition_drift",
		"此前完整公告快照1次",
		"@4.950000",
		"不参与任何判定",
	} {
		if !strings.Contains(joined, needle) {
			t.Fatalf("partition-drift caveat missing %q:\n%s", needle, joined)
		}
	}
}

func TestAnnouncePartitionRefusalAuditFactorsDoNotChangeVerdicts(t *testing.T) {
	t.Run("drift", func(t *testing.T) {
		groups := map[int64][]int{1430000: {0, 1}, 2000000: {4, 5}}
		offsets := map[int]float64{0: 0, 1: 1, 4: 2, 5: 3}
		tls := announceSweepTimelines(groups, offsets, 10.0, 0.001, 3, nil)
		tls[4][1].khz = 1430000
		d := deriveClusterFreqDomains(tls)
		if d.byCPU[4] == d.byCPU[5] {
			t.Fatalf("drift must retain the pre-PARTDISC fail-open split: %+v", d.members)
		}
		audit := d.partitionAudit
		if audit.refusal != announcePartitionRefusalDrift || audit.snapshots != 1 ||
			audit.driftTs != 10.001 {
			t.Fatalf("drift audit factors = %+v, want refusal/1/10.001", audit)
		}
		clause := capabilityPartitionRefusalClause(d)
		for _, want := range []string{
			"分区车道=partition_drift",
			"此前完整公告快照1次",
			"@10.001000",
		} {
			if !strings.Contains(clause, want) {
				t.Fatalf("drift clause missing %q: %q", want, clause)
			}
		}
	})

	t.Run("below_floor", func(t *testing.T) {
		tls := map[int][]freqSample{
			0: {{ts: 10.0, khz: 1600000}, {ts: 11.0, khz: 1600000}},
			1: {{ts: 10.000002, khz: 1600000}},
			4: {{ts: 10.000010, khz: 2151000}},
		}
		d := deriveClusterFreqDomains(tls)
		if d.byCPU[0] == d.byCPU[1] {
			t.Fatalf("one snapshot must retain the pre-PARTDISC floor split: %+v", d.members)
		}
		audit := d.partitionAudit
		if audit.refusal != announcePartitionRefusalBelowFloor || audit.snapshots != 1 {
			t.Fatalf("below-floor audit factors = %+v, want refusal/1", audit)
		}
		clause := capabilityPartitionRefusalClause(d)
		for _, want := range []string{
			"分区车道=partition_below_floor",
			"完整公告快照仅1次(<2)",
		} {
			if !strings.Contains(clause, want) {
				t.Fatalf("below-floor clause missing %q: %q", want, clause)
			}
		}
	})

	t.Run("limits_veto_sorted", func(t *testing.T) {
		groups := map[int64][]int{1800000: {0, 1}, 2100000: {4, 5}}
		offsets := map[int]float64{0: 0, 1: 1, 4: 2, 5: 3}
		drop := map[int]map[int]bool{3: {1: true, 5: true}}
		tls := announceSweepTimelines(groups, offsets, 10.0, 0.001, 4, drop)
		limits := map[int][]freqSample{
			0: {{ts: 9, khz: 2200000}},
			1: {{ts: 9, khz: 2300000}},
			4: {{ts: 9, khz: 2400000}},
			5: {{ts: 9, khz: 2500000}},
		}
		d := deriveClusterFreqDomainsLimits(tls, limits)
		if d.byCPU[0] == d.byCPU[1] || d.byCPU[4] == d.byCPU[5] {
			t.Fatalf("limits sub-veto must retain both pre-PARTDISC splits: %+v", d.members)
		}
		audits := d.partitionAudit.limitsVetoGroups
		if len(audits) != 2 || audits[0].members[0] != 0 || audits[1].members[0] != 4 {
			t.Fatalf("limits audit groups must be deterministic by minimum CPU: %+v", audits)
		}
		clause := capabilityPartitionRefusalClause(d)
		first := strings.Index(clause, "值组[cpu0,1]")
		second := strings.Index(clause, "值组[cpu4,5]")
		if first < 0 || second <= first {
			t.Fatalf("limits clauses must follow deterministic CPU order: %q", clause)
		}
		for _, want := range []string{
			"分区车道=partition_limits_veto",
			"2档不同限频上界(2200000/2300000kHz)",
			"2档不同限频上界(2400000/2500000kHz)",
		} {
			if !strings.Contains(clause, want) {
				t.Fatalf("limits clause missing %q: %q", want, clause)
			}
		}
	})
}

func TestAnnouncePartitionRefusalDistinguishesRanFromNoSnapshot(t *testing.T) {
	noSnapshot := map[int][]freqSample{
		0: {{ts: 10, khz: 1430000}, {ts: 20, khz: 1530000}, {ts: 30, khz: 1430000}},
		1: {{ts: 10.000001, khz: 1430000}, {ts: 20.000001, khz: 1530000}, {ts: 30.000001, khz: 1430000}},
		4: {{ts: 10.5, khz: 1430000}, {ts: 15, khz: 1430000}},
		8: {{ts: 11, khz: 1430000}, {ts: 18, khz: 1430000}, {ts: 25, khz: 1430000}},
	}
	noSnapshotCapability := resolveCoreCapability(deriveClusterFreqDomains(noSnapshot), noSnapshot)
	if noSnapshotCapability.source != CoreCapabilitySourceFreqOnly ||
		noSnapshotCapability.freqOnlyReason != CoreCapabilityFreqOnlyReasonFmaxTie {
		t.Fatalf("no-snapshot fixture must land fmax-tie freq_only: %q/%q",
			noSnapshotCapability.source, noSnapshotCapability.freqOnlyReason)
	}
	if strings.Contains(noSnapshotCapability.freqOnlySplitAudit, "分区车道=") {
		t.Fatalf("zero complete snapshots must remain silent: %q", noSnapshotCapability.freqOnlySplitAudit)
	}

	drift := map[int][]freqSample{
		0: {{ts: 10, khz: 1430000}, {ts: 20, khz: 1530000}, {ts: 30, khz: 1430000}},
		1: {{ts: 10.000001, khz: 1430000}, {ts: 20.000001, khz: 1530000}, {ts: 30.000001, khz: 1430000}},
		4: {{ts: 10.000002, khz: 1430000}, {ts: 15, khz: 1430000}, {ts: 20.000002, khz: 1430000}},
		8: {{ts: 10.000003, khz: 1430000}, {ts: 18, khz: 1430000}, {ts: 20.000003, khz: 1430000}, {ts: 25, khz: 1430000}},
	}
	driftCapability := resolveCoreCapability(deriveClusterFreqDomains(drift), drift)
	if driftCapability.source != CoreCapabilitySourceFreqOnly ||
		driftCapability.freqOnlyReason != CoreCapabilityFreqOnlyReasonFmaxTie {
		t.Fatalf("drift fixture must retain the same fmax-tie verdict: %q/%q",
			driftCapability.source, driftCapability.freqOnlyReason)
	}
	if !strings.Contains(driftCapability.freqOnlySplitAudit, "分区车道=partition_drift") {
		t.Fatalf("the ran-and-refused fixture must disclose drift: %q", driftCapability.freqOnlySplitAudit)
	}
	if driftCapability.freqOnlySplitAudit == noSnapshotCapability.freqOnlySplitAudit {
		t.Fatalf("ran-and-refused must be byte-distinguishable from never-observed")
	}
}

func TestAnnouncePartitionSuccessAndZeroSnapshotStayAuditSilent(t *testing.T) {
	drop := map[int]map[int]bool{3: {1: true}}
	tls := announceSweepTimelines(announceCustomerGroups, announceCustomerOffsets, 10, 0.001, 10, drop)
	d := deriveClusterFreqDomains(tls)
	if d.groupCount != 3 {
		t.Fatalf("successful partition fixture drifted: %+v", d.members)
	}
	if got := capabilityPartitionRefusalClause(d); got != "" {
		t.Fatalf("successful partition must mint no refusal clause: %q", got)
	}

	zero := clusterFreqDomains{
		source: ClusterFreqSourceDerived,
		partitionAudit: announcePartitionAudit{
			refusal: "", snapshots: 0,
		},
	}
	if got := capabilityPartitionRefusalClause(zero); got != "" {
		t.Fatalf("zero complete snapshots must mint no clause: %q", got)
	}
}

func TestRailRefinementCarriesGlobalPartitionAudit(t *testing.T) {
	domains := clusterFreqDomains{
		byCPU:      map[int]string{0: "derived_c0", 1: "derived_c0"},
		members:    map[string][]int{"derived_c0": {0, 1}},
		source:     ClusterFreqSourceDerived,
		sampledAsc: []int{0, 1},
		groupCount: 1,
		partitionAudit: announcePartitionAudit{
			refusal: announcePartitionRefusalDrift, snapshots: 2, driftTs: 12.5,
		},
	}
	adoption := &clusterRailAdoption{clusters: []clusterRailCluster{
		{anchor: 0, members: []int{0}},
		{anchor: 1, members: []int{1}},
	}}
	refined := refineDomainsWithRails(domains, adoption)
	if !refined.ok || !refined.structureUsed {
		t.Fatalf("fixture must refine one measured domain into two rail ranges: %+v", refined)
	}
	audit := refined.domains.partitionAudit
	if audit.refusal != announcePartitionRefusalDrift || audit.snapshots != 2 || audit.driftTs != 12.5 {
		t.Fatalf("rail refinement lost global partition audit: %+v", audit)
	}
}
