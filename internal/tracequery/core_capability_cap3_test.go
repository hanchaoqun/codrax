package tracequery

// core_capability_cap3_test.go — CAP-3 batch (§29.11 + 补充, docs/design/
// real_trace_campaign_20260705.md, user rulings 2026-07-09) pins:
//
//	R1  cap2 四窗对照 engine-real reproduction (the batch's acceptance basis,
//	    /Users/han/opt/customlogs/cap2_report.txt): one berlin-shaped trace,
//	    four windowed carves — two MaxEvents-cap cuts (the huadong window
//	    a/b form, event budget tripping in the padding tail), one padded-head
//	    burst straddle, one clean control (the g12 form) — ALL must judge the
//	    SAME default-table capability with the SAME co-movement topology and
//	    the SAME per-cluster fmax ladder. Pre-CAP-3 the three boundary-cut
//	    carves fragmented the cluster grouping and died on the fail-loud
//	    freq_only arms while the control judged — the adjudicated
//	    "同 trace 异窗一判一不判" lane fork.
//	R2  cpu_frequency 状态携入 (§29.11 ①): a fold governance window with ZERO
//	    in-window frequency events prices its slices from the carried-in
//	    (窗前最近变化点) state — "窗内无变频事件" ≠ 数据缺失. Guards the
//	    "携入臂偷偷退回窗内取事件" mutation: dropping the head-governing
//	    sample books everything UNKNOWN and reds here.
//	R3  真缺失保留 (§29.11 诚实边界): a CPU with no change point anywhere up
//	    to the governance window end keeps the missing-counts-0 lower-bound
//	    arm (UnknownMs) — carry-in never fabricates a value backwards.
//	R4  window-face 拓扑全局基 (§29.11 ②): the window faces derive donor
//	    membership from the INDEX-global sample stream. Guards the
//	    "全局基偷偷窗裁剪" mutation: re-feeding the ≤TimeEnd window
//	    collection fragments a straddled cluster into ≥3 pseudo-domains and
//	    resurrects the false 超大核 declaration this pin asserts absent.
//	R5  同 Index 折算词一致 (§29.11 补充 ③ 验收): every fold row of one Index
//	    carries the same capability-source / topology-source token pair —
//	    across governance windows and across the VS-2/R5d lanes.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// cap3Burst is one cluster-wide DVFS emission: member rows 1µs apart, the
// donghu-measured shape (one notifier loop writes one row per member).
type cap3Burst struct {
	ts   float64
	khz  int
	cpus []int
}

// cap3TraceLines renders bursts + filler scheduler traffic as a raw ftrace
// text, ts-sorted. Filler density controls where an event budget lands.
func cap3TraceLines(bursts []cap3Burst, fillerEveryUs float64, spanStart, spanEnd float64, tail []string) string {
	type row struct {
		ts   float64
		line string
	}
	var rows []row
	for _, b := range bursts {
		for i, cpu := range b.cpus {
			ts := b.ts + float64(i)*1e-6
			rows = append(rows, row{ts, fmt.Sprintf("  tppmgr-sched-in-5850  (    2) [001] .... %.6f: cpu_frequency: state=%d cpu_id=%d", ts, b.khz, cpu)})
		}
	}
	for ts := spanStart; ts <= spanEnd; ts += fillerEveryUs * 1e-6 {
		rows = append(rows, row{ts, fmt.Sprintf("  app-20  (   20) [001] .... %.6f: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001", ts)})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ts < rows[j].ts })
	lines := make([]string, 0, len(rows)+len(tail)+1)
	for _, r := range rows {
		lines = append(lines, r.line)
	}
	lines = append(lines, tail...)
	return strings.Join(lines, "\n") + "\n"
}

// cap3SweepBursts is the berlin-shaped three-cluster DVFS sweep with the cap2
// witness frequency vocabulary: small {0,1,2,3} 1430000↔1530000 (window-a
// residency), middle {4,5,6,7} 1652000/1930000/2150000 and big {8,9}
// 1850000/2288000 (control-window residency). Bursts every everyMs; the three
// clusters emit 200µs apart so cross-cluster rows never sit inside one
// another's skew bound.
func cap3SweepBursts(start, end, everyMs float64) []cap3Burst {
	smallVals := []int{1430000, 1530000}
	middleVals := []int{1652000, 1930000, 2150000}
	bigVals := []int{1850000, 2288000}
	var bursts []cap3Burst
	for k := 0; ; k++ {
		ts := start + float64(k)*everyMs*1e-3
		if ts > end {
			break
		}
		bursts = append(bursts,
			cap3Burst{ts, smallVals[k%2], []int{0, 1, 2, 3}},
			cap3Burst{ts + 0.0002, middleVals[k%3], []int{4, 5, 6, 7}},
			cap3Burst{ts + 0.0004, bigVals[k%2], []int{8, 9}})
	}
	return bursts
}

func cap3WriteTrace(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cap3_berlin_shape.systrace")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// cap3EventsThrough counts full-parse events inside [carveStart, cut] — the
// MaxEvents value that makes the windowed build's budget trip exactly at cut.
func cap3EventsThrough(t *testing.T, path string, carveStart, cut float64) int {
	t.Helper()
	full, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, ev := range full.Events {
		if ev.Ts >= carveStart && ev.Ts <= cut {
			n++
		}
	}
	if n == 0 {
		t.Fatalf("no events in [%.6f, %.6f] — fixture broken", carveStart, cut)
	}
	return n
}

// --- R1: cap2 四窗对照 -------------------------------------------------------

func TestCoreCapabilityCap2FourWindowParity(t *testing.T) {
	content := cap3TraceLines(cap3SweepBursts(1.0, 4.0, 2.0), 100, 1.0, 4.0, nil)
	path := cap3WriteTrace(t, content)

	wantFmax := map[string]int64{"small": 1530000, "middle": 2150000, "big": 2288000}

	type carve struct {
		name string
		opts BuildOptions
	}
	carves := []carve{}
	// huadong window a/b form: MaxEvents trips just past a small-cluster
	// burst's second member row inside the padding tail (bursts land on the
	// 2ms grid; +1.5µs admits cpu0/cpu1 and cuts cpu2/cpu3 of that burst).
	for _, w := range []struct {
		name             string
		start, end, trip float64
	}{
		{"huadong_window_a_cap_cut", 2.0, 2.2, 2.302 + 1.5e-6},
		{"huadong_window_b_cap_cut", 2.6, 2.8, 2.902 + 1.5e-6},
	} {
		carves = append(carves, carve{
			name: w.name,
			opts: BuildOptions{
				TimeStart: w.start, TimeEnd: w.end, TimeStartSet: true, TimeEndSet: true,
				TimePaddingBefore: 0.5, TimePaddingAfter: 0.5, AllowWindowedParse: true,
				MaxEvents: cap3EventsThrough(t, path, w.start-0.5, w.trip),
			},
		})
	}
	// padded-head burst straddle: the ts gate lands between member rows of
	// the burst on the 1.5s grid (padded start = TimeStart−0.5 = 1.5000015).
	carves = append(carves, carve{
		name: "padded_head_straddle",
		opts: BuildOptions{
			TimeStart: 2.0000015, TimeEnd: 2.2, TimeStartSet: true, TimeEndSet: true,
			TimePaddingBefore: 0.5, TimePaddingAfter: 0.5, AllowWindowedParse: true,
		},
	})
	// g12 control: clean carve — no budget, and both padded boundaries land
	// 100µs OFF the burst grid, so the pre-CAP-3 whole-array identity judged
	// this window too (the campaign witness: g12 判出 while huadong 不可判).
	carves = append(carves, carve{
		name: "g12_control_clean",
		opts: BuildOptions{
			TimeStart: 3.0001, TimeEnd: 3.1001, TimeStartSet: true, TimeEndSet: true,
			TimePaddingBefore: 0.5, TimePaddingAfter: 0.5, AllowWindowedParse: true,
		},
	})

	for _, c := range carves {
		t.Run(c.name, func(t *testing.T) {
			idx, err := BuildIndexWithOptions(context.Background(), path, c.opts)
			if err != nil {
				t.Fatalf("carve build must succeed: %v", err)
			}
			capability := newChainQueryCache(idx, nil).coreCapability("")
			if capability.source != CoreCapabilitySourceDefault {
				t.Fatalf("§29.11: this carve must judge the default capability table (同 trace 全窗同判), got %q (floorTripped=%v, groups=%d)",
					capability.source, capability.comoveFloorTripped, capability.domains.groupCount)
			}
			if capability.topologySource != CoreCapabilityTopologyComovement {
				t.Fatalf("judged structure must carry the co-movement topology token, got %q", capability.topologySource)
			}
			if capability.domains.groupCount != 3 {
				t.Fatalf("berlin shape has three clusters, got %d", capability.domains.groupCount)
			}
			for label, class := range capability.classByCluster {
				if capability.fmaxByCluster[label] != wantFmax[class] {
					t.Fatalf("class %s fmax = %d, want %d (label %s)", class, capability.fmaxByCluster[label], wantFmax[class], label)
				}
			}
			// The fail-loud arms stay live for NON-boundary shapes; here they
			// must all be silent.
			if capability.comoveFloorTripped {
				t.Fatalf("comove floor must not trip on a dense sweep")
			}
		})
	}
}

// --- R2/R3: 状态携入 + 真缺失 -----------------------------------------------

func TestSupplyFoldCarryInStateSemantics(t *testing.T) {
	// Sweep ends at 4.0; the fold's governance window [5.0, 5.1] contains
	// ZERO cpu_frequency events. Small-core slice: carried-in state = the
	// last small change ≤ 5.0 (the k=1500 burst at 4.0 → 1430000). The index
	// is a windowed CARVE whose padded head straddles the burst on the 1.5s
	// grid (padded start 1.5000015 admits cpu2/cpu3 rows of that burst and
	// gate-cuts cpu0/cpu1) — pre-CAP-3 that single boundary row fragmented
	// the small cluster and this fold degraded to freq_only.
	content := cap3TraceLines(cap3SweepBursts(1.0, 4.0, 2.0), 400, 1.0, 5.2, nil)
	path := cap3WriteTrace(t, content)
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 2.0000015, TimeEnd: 5.15, TimeStartSet: true, TimeEndSet: true,
		TimePaddingBefore: 0.5, TimePaddingAfter: 0.5, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	cache := newChainQueryCache(idx, nil)
	q := Query{TimeStart: 5.0, TimeEnd: 5.1}
	intervals := []Interval{{State: StateRunning, StartTs: 5.01, EndTs: 5.05, DurationMs: 40, CPU: 2, CPUKnown: true}}
	ideal, basis := cache.supplyFoldRunningIntervals(q, 5.0, 5.1, intervals)
	if basis.CapabilitySource != CoreCapabilitySourceDefault {
		t.Fatalf("topology judged from the global stream even with an event-free window, got %q", basis.CapabilitySource)
	}
	if basis.CapabilitySplitAudit != "" {
		t.Fatalf("复核 P2 absence discipline: a judged basis carries no split audit, got %q", basis.CapabilitySplitAudit)
	}
	if basis.UnknownMs != 0 || basis.KnownMs != 40 {
		t.Fatalf("携入语义: an event-free window is NOT missing data — want known=40 unknown=0, got known=%v unknown=%v", basis.KnownMs, basis.UnknownMs)
	}
	// SLICE side: carried-in state (CMP-10 F1 governance + §29.11 状态语义) —
	// the slice's small frequency is the window-head carried value (last
	// small change at the sweep tail, k=1500 even: 1430000). BASIS side (R5
	// §29.88.3 + R6 规则4, 2026-07-15 evolution): the 全域最大核最高频点 over
	// the FULL-FILE curves — the big lane's sweep maximum 2288000 (bigVals
	// alternate 1850000/2288000), NOT the carried window value 1850000.
	if basis.FmaxKHz != 2288000 || basis.FmaxSource != SupplyFoldFmaxSourceObserved {
		t.Fatalf("reference fmax must be the full-file big maximum (R5), got %d/%s", basis.FmaxKHz, basis.FmaxSource)
	}
	want := 40 * (1430000.0 * coreCapabilityDefaultSmall) / (2288000.0 * coreCapabilityDefaultBig)
	if diff := ideal - want; diff < -1e-9 || diff > 1e-9 {
		t.Fatalf("carried-in fold ideal = %v, want %v", ideal, want)
	}
	if deficit := 40 - ideal; deficit <= 0 {
		t.Fatalf("small-core carried slice must mint a deficit, got %v", deficit)
	}

	// R3 真缺失: cpu11 has no change point anywhere up to the governance end
	// — the missing-counts-0 lower-bound arm stands (absence never guesses).
	missing := []Interval{{State: StateRunning, StartTs: 5.01, EndTs: 5.05, DurationMs: 40, CPU: 11, CPUKnown: true}}
	idealMissing, basisMissing := cache.supplyFoldRunningIntervals(q, 5.0, 5.1, missing)
	if basisMissing.UnknownMs != 40 || basisMissing.KnownMs != 0 {
		t.Fatalf("真缺失 must stay UNKNOWN basis, got known=%v unknown=%v", basisMissing.KnownMs, basisMissing.UnknownMs)
	}
	if idealMissing != 40 {
		t.Fatalf("unknown slices fold at ratio 1 (never fabricate deficit), got %v", idealMissing)
	}
}

// --- R4: window-face 拓扑全局基 ----------------------------------------------

func TestComputeWindowStatsDonorDerivationIndexGlobal(t *testing.T) {
	// Two real clusters: A={1,2} (small vocab), B={3,4} (big vocab). The
	// window's TimeEnd (2.0) straddles B's burst at 1.999999: cpu3's row is
	// admitted into the ≤TimeEnd window collection, cpu4's row (2.000001) is
	// not. Window-cropped derivation therefore fragments B into {3},{4} —
	// three derived pseudo-domains — and consulting busy cpu5 (above the
	// highest sampled core) minted the false 超大核 declaration into the
	// user-facing caveat. Index-global derivation (CAP-3) keeps two domains:
	// no prime declaration, donor reuse for cpu0 unchanged.
	var b strings.Builder
	writeBurst := func(ts float64, khz int, cpus ...int) {
		for i, cpu := range cpus {
			fmt.Fprintf(&b, "  tppmgr-sched-in-5850  (    2) [001] .... %.6f: cpu_frequency: state=%d cpu_id=%d\n", ts+float64(i)*1e-6, khz, cpu)
		}
	}
	writeBurst(1.100000, 1430000, 1, 2)
	writeBurst(1.300000, 1530000, 1, 2)
	writeBurst(1.200000, 1850000, 3, 4)
	// The straddled burst: cpu3's member row at 1.999999 ≤ TimeEnd, cpu4's at
	// 2.000001 > TimeEnd (2µs member spread, one emission within the skew
	// bound) — the ≤TimeEnd window collection sees cpu3 only, and B's
	// remaining co-movement evidence lives AFTER TimeEnd, so a window-cropped
	// derivation is stuck below the tail evidence floor and fragments B; the
	// Index-global stream carries the full identical timelines.
	fmt.Fprintf(&b, "  tppmgr-sched-in-5850  (    2) [001] .... 1.999999: cpu_frequency: state=2288000 cpu_id=3\n")
	fmt.Fprintf(&b, "  tppmgr-sched-in-5850  (    2) [001] .... 2.000001: cpu_frequency: state=2288000 cpu_id=4\n")
	writeBurst(2.100000, 1850000, 3, 4)
	writeBurst(2.200000, 2288000, 3, 4)
	// Busy segments: cpu0 (donor consumer, unsampled) and cpu5 (prime consult).
	b.WriteString("        app-100 (100) [000] .... 1.500000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n")
	b.WriteString("        app-100 (100) [000] .... 1.600000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120\n")
	b.WriteString("        dep-200 (200) [005] .... 1.500000: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20\n")
	b.WriteString("        dep-200 (200) [005] .... 1.600000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/5 next_pid=0 next_prio=120\n")
	idx := buildTraceIndexFromContent(t, b.String())

	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 2.0})
	joined := strings.Join(stats.Caveats, "\n")
	if strings.Contains(joined, "超大核") || strings.Contains(joined, "prime domain") {
		t.Fatalf("window-cropped fragmentation resurrected the false prime declaration (全局基偷偷窗裁剪):\n%s", joined)
	}
	if !strings.Contains(joined, "cpu0←cpu1") {
		t.Fatalf("cpu0 donor reuse from the derived small cluster must survive, caveats:\n%s", joined)
	}
	if !strings.Contains(joined, "freq-change-point derived clusters") {
		t.Fatalf("derived-membership disclosure missing:\n%s", joined)
	}
}

// --- R5: 同 Index 折算词一致 --------------------------------------------------

func TestSupplyFoldCapabilityTokensConsistentAcrossWindows(t *testing.T) {
	content := cap3TraceLines(cap3SweepBursts(1.0, 4.0, 2.0), 100, 1.0, 4.0, nil)
	path := cap3WriteTrace(t, content)
	// The huadong window-a carve (budget cut mid-burst) — the historically
	// forked shape; every fold row of this ONE Index must carry one token
	// pair no matter which governance window prices it.
	opts := BuildOptions{
		TimeStart: 2.0, TimeEnd: 2.2, TimeStartSet: true, TimeEndSet: true,
		TimePaddingBefore: 0.5, TimePaddingAfter: 0.5, AllowWindowedParse: true,
		MaxEvents: cap3EventsThrough(t, path, 1.5, 2.302+1.5e-6),
	}
	idx, err := BuildIndexWithOptions(context.Background(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	cache := newChainQueryCache(idx, nil)
	type tokens struct{ source, topo string }
	var seen []tokens
	for _, w := range [][2]float64{{2.0, 2.1}, {2.1, 2.2}, {2.05, 2.15}} {
		q := Query{TimeStart: w[0], TimeEnd: w[1]}
		intervals := []Interval{{State: StateRunning, StartTs: w[0] + 0.01, EndTs: w[0] + 0.03, DurationMs: 20, CPU: 1, CPUKnown: true}}
		_, basis := cache.supplyFoldRunningIntervals(q, w[0], w[1], intervals)
		seen = append(seen, tokens{basis.CapabilitySource, basis.ClusterTopologySource})
	}
	// R5d lane rides the same memoized judgment.
	capability := cache.coreCapability("")
	seen = append(seen, tokens{capability.source, capability.topologySource})
	for i, tok := range seen {
		if tok.source != CoreCapabilitySourceDefault || tok.topo != CoreCapabilityTopologyComovement {
			t.Fatalf("row %d token pair (%q, %q) — 同 Index 全部折算行必须同词 (default_table, freq_comovement); all=%v", i, tok.source, tok.topo, seen)
		}
	}
}

// buildTraceIndexFromContent is buildTraceIndex with pre-rendered content
// (the CAP-3 generators render their own line sets).
func buildTraceIndexFromContent(t *testing.T, content string) *Index {
	t.Helper()
	return buildTraceIndex(t, "cap3_inline.systrace", content)
}

// --- 复核 P1 domain witness: parked-twin fusion must not invert classes ------

// a1' domain form (复核 REPRO, 2026-07-10): three REAL clusters — an ACTIVE
// small cluster {0,1} plus middle {4} and big {8} both PARKED at one value
// with first announcements inside the skew bound and differing re-announce
// cadence. Under the aligned≥1 floor the parked twins fused → groupCount=2 →
// the §26 two-cluster mapping crowned the ACTIVE SMALL cluster as big
// (cap 2.53) and shipped default_table silently — a class INVERSION. With
// clusterFreqTrimmedMinAligned=2 the twins split; the two parked singletons
// then tie on fmax and the judgment fails LOUD to freq_only (禁掷币 arm —
// honest: nothing distinguishes the parked twins), never the inverted table.
func TestCoreCapabilityParkedTwinClustersNeverInvertClasses(t *testing.T) {
	timelines := map[int][]freqSample{
		// active small cluster: real transitions, members identical.
		0: {{ts: 10, khz: 1430000}, {ts: 20, khz: 1530000}, {ts: 30, khz: 1430000}},
		1: {{ts: 10.000001, khz: 1430000}, {ts: 20.000001, khz: 1530000}, {ts: 30.000001, khz: 1430000}},
		// middle cluster parked @1430000, re-announce cadence A.
		4: {{ts: 10.000002, khz: 1430000}, {ts: 15, khz: 1430000}},
		// big cluster parked @1430000, re-announce cadence B.
		8: {{ts: 10.000003, khz: 1430000}, {ts: 18, khz: 1430000}, {ts: 25, khz: 1430000}},
	}
	d := deriveClusterFreqDomains(timelines)
	if d.byCPU[4] == d.byCPU[8] {
		t.Fatalf("parked twin clusters fused (a1' false merge): %+v", d)
	}
	if d.groupCount != 3 {
		t.Fatalf("want 3 derived domains ({0,1}/{4}/{8}), got %+v", d)
	}
	capability := resolveCoreCapability(d, timelines)
	if capability.source != CoreCapabilitySourceFreqOnly {
		t.Fatalf("parked-twin fmax tie must fail loud to freq_only (禁掷币), got %q", capability.source)
	}
	// The inversion witness stays dead: under NO judged table may the active
	// small cluster price above 1 while real middle/big clusters exist.
	if cap := capability.capabilityFor(0); cap != 1 {
		t.Fatalf("freq_only must price every CPU at 1 (no class claim), cpu0 got %v", cap)
	}
}

// --- 复核 P2: freq_only fragmentation split-audit (disclosure lane) ----------

// The audit must localize the FIRST co-movement split behind a fragmentation
// freq_only verdict with all three elements — cpu pair, timestamp, judging
// arm — so a customer replay can tell a healed boundary form from the honest
// mid-stream residual. Disclosure only: judged (default_table) bases must
// stay audit-free byte for byte.
func TestCoreCapabilityFreqOnlySplitAuditLocalizesFirstSplit(t *testing.T) {
	// Tie-arm form (the parked-twin shape): the tied pair is {4} vs {8},
	// split by the co-witness evidence floor.
	twins := map[int][]freqSample{
		0: {{ts: 10, khz: 1430000}, {ts: 20, khz: 1530000}, {ts: 30, khz: 1430000}},
		1: {{ts: 10.000001, khz: 1430000}, {ts: 20.000001, khz: 1530000}, {ts: 30.000001, khz: 1430000}},
		4: {{ts: 10.000002, khz: 1430000}, {ts: 15, khz: 1430000}},
		8: {{ts: 10.000003, khz: 1430000}, {ts: 18, khz: 1430000}, {ts: 25, khz: 1430000}},
	}
	capability := resolveCoreCapability(deriveClusterFreqDomains(twins), twins)
	if capability.source != CoreCapabilitySourceFreqOnly {
		t.Fatalf("fixture must land freq_only, got %q", capability.source)
	}
	audit := capability.freqOnlySplitAudit
	for _, needle := range []string{"cpu4↔cpu8", "@10.000003", "判定臂=" + freqCoMoveSplitArmFloor, freqCoMoveSplitArmZH(freqCoMoveSplitArmFloor)} {
		if !strings.Contains(audit, needle) {
			t.Fatalf("tie-arm audit missing %q: %q", needle, audit)
		}
	}

	// Mid-arm form (the honest residual ①): cpu1 genuinely misses a member
	// row mid-stream while the stream continues — four groups, the twin pair
	// ties on fmax, and the audit names the mid-alignment violation at the
	// missing change's timestamp.
	midMiss := map[int][]freqSample{
		0: {{ts: 10, khz: 1430000}, {ts: 20, khz: 1530000}, {ts: 30, khz: 1430000}, {ts: 40, khz: 1530000}},
		1: {{ts: 10.000001, khz: 1430000}, {ts: 30.000001, khz: 1430000}, {ts: 40.000001, khz: 1530000}},
		4: {{ts: 10.000002, khz: 1652000}, {ts: 20.000002, khz: 2150000}, {ts: 40.000002, khz: 1652000}},
		8: {{ts: 10.000003, khz: 1850000}, {ts: 20.000003, khz: 2288000}, {ts: 40.000003, khz: 1850000}},
	}
	capability = resolveCoreCapability(deriveClusterFreqDomains(midMiss), midMiss)
	if capability.source != CoreCapabilitySourceFreqOnly {
		t.Fatalf("mid-miss fixture must land freq_only (tie between the fragmented twins), got %q", capability.source)
	}
	audit = capability.freqOnlySplitAudit
	for _, needle := range []string{"cpu0↔cpu1", "@40.000001", "判定臂=" + freqCoMoveSplitArmMid, freqCoMoveSplitArmZH(freqCoMoveSplitArmMid)} {
		if !strings.Contains(audit, needle) {
			t.Fatalf("mid-arm audit missing %q: %q", needle, audit)
		}
	}
}

// Fold-basis carriage + result-caveat lift + judged-basis absence.
func TestSupplyFoldCapabilitySplitAuditDisclosure(t *testing.T) {
	var b strings.Builder
	row := func(ts float64, khz, cpu int) {
		fmt.Fprintf(&b, "  tppmgr-sched-in-5850  (    2) [001] .... %.6f: cpu_frequency: state=%d cpu_id=%d\n", ts, khz, cpu)
	}
	// Active small cluster {0,1}; parked twins {4} / {8} (a1' domain form).
	row(1.100000, 1430000, 0)
	row(1.100001, 1430000, 1)
	row(1.100002, 1430000, 4)
	row(1.100003, 1430000, 8)
	row(1.200000, 1530000, 0)
	row(1.200001, 1530000, 1)
	row(1.150000, 1430000, 4) // parked re-announce cadence A
	row(1.180000, 1430000, 8) // parked re-announce cadence B
	row(1.250000, 1430000, 8)
	b.WriteString("        app-100 (100) [000] .... 1.400000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n")
	b.WriteString("        app-100 (100) [000] .... 1.500000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120\n")
	idx := buildTraceIndex(t, "cap3_split_audit.systrace", b.String())
	cache := newChainQueryCache(idx, nil)
	q := Query{TimeStart: 1.0, TimeEnd: 1.5}
	intervals := []Interval{{State: StateRunning, StartTs: 1.4, EndTs: 1.45, DurationMs: 50, CPU: 0, CPUKnown: true}}
	_, basis := cache.supplyFoldRunningIntervals(q, 1.0, 1.5, intervals)
	if basis.CapabilitySource != CoreCapabilitySourceFreqOnly {
		t.Fatalf("parked-twin index must fold freq_only, got %q", basis.CapabilitySource)
	}
	for _, needle := range []string{"cpu4↔cpu8", "@", "判定臂="} {
		if !strings.Contains(basis.CapabilitySplitAudit, needle) {
			t.Fatalf("basis audit missing %q: %q", needle, basis.CapabilitySplitAudit)
		}
	}
	// Result-caveat lift: one caveat, three localization elements, and the
	// explicit disclosure-only sentence (never a gate).
	res := Result{WakeupChain: &ChainResult{CausalImpacts: []WakeupCausalImpact{{SupplyFoldBasis: &basis}}}}
	caveat := capabilitySplitAuditCaveat(res)
	for _, needle := range []string{"capability_freq_only_split_audit=", "cpu4↔cpu8", "判定臂=", "不参与任何判定", "never a gate"} {
		if !strings.Contains(caveat, needle) {
			t.Fatalf("lifted caveat missing %q: %q", needle, caveat)
		}
	}
	// Absence discipline: a judged basis carries NO audit and lifts nothing.
	judged := SupplyFoldBasis{CapabilitySource: CoreCapabilitySourceDefault}
	if caveat := capabilitySplitAuditCaveat(Result{WakeupChain: &ChainResult{CausalImpacts: []WakeupCausalImpact{{SupplyFoldBasis: &judged}}}}); caveat != "" {
		t.Fatalf("judged basis must lift no audit caveat, got %q", caveat)
	}
}

// End-to-end Run-path flow: the audit caveat reaches Result.Caveats — the
// verbatim engine lane tracediag replays print — for a wakeup_chain whose
// fold degraded freq_only on a fragmentation arm (the P2 acceptance surface).
func TestRunLiftsCapabilitySplitAuditCaveat(t *testing.T) {
	var b strings.Builder
	row := func(ts float64, khz, cpu int) {
		fmt.Fprintf(&b, "  tppmgr-sched-in-5850  (    2) [001] .... %.6f: cpu_frequency: state=%d cpu_id=%d\n", ts, khz, cpu)
	}
	// Active small cluster {0,1}; parked twins {4}/{8} (a1' domain form).
	row(4.900000, 1430000, 0)
	row(4.900001, 1430000, 1)
	row(4.900002, 1430000, 4)
	row(4.900003, 1430000, 8)
	row(4.950000, 1530000, 0)
	row(4.950001, 1530000, 1)
	row(4.930000, 1430000, 4)
	row(4.960000, 1430000, 8)
	row(4.980000, 1430000, 8)
	// dep runs ~10ms on cpu0 then wakes app (running-dominant on-chain node
	// → the VS-2 fold runs and lands freq_only with the audit).
	b.WriteString("        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n")
	b.WriteString("        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\n")
	b.WriteString("        dep-200 (100) [000] .... 5.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20\n")
	b.WriteString("        dep-200 (100) [000] .... 5.009900: sched_wakeup: comm=app pid=100 prio=53 target_cpu=001\n")
	b.WriteString("        dep-200 (100) [000] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120\n")
	b.WriteString("        app-100 (100) [001] .... 5.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n")
	idx := buildTraceIndex(t, "cap3_split_audit_run.systrace", b.String())
	res := Run(idx, Query{View: "wakeup_chain", PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	joined := strings.Join(res.Caveats, "\n")
	for _, needle := range []string{"capability_freq_only_split_audit=", "cpu4↔cpu8", "判定臂=", "不参与任何判定"} {
		if !strings.Contains(joined, needle) {
			t.Fatalf("Run caveats missing %q:\n%s", needle, joined)
		}
	}
}
