package tracequery

// rank_seat_ord_test.go — ORD 引擎席位批 pins (ledger
// real_trace_campaign_20260705.md §29.8 P2②/P2③ + §29.11 补充 cap2 两观察,
// 2026-07-10). One participant, one seat:
//
//	ORD-A aggregate/成员席位排他 (cap2 观察②: #4=occurrences=2 aggregate
//	      12.401 + #5/#6 its two member occurrences 6.236/6.165 — three seats
//	      for ONE occurrence set; cmp_792 E7: the display ×N merge then summed
//	      the aggregate row (=Σ member gateds) with its own member rows and
//	      published 窗口投影 11.804 = EXACTLY 2× 有效归因 5.902 — the P2②
//	      "恰=2×/occurrence 集不可调和" accounting form IS this double mint).
//	      A wakeup-chain occurrence whose (pid, dominant-state) group seats an
//	      aggregate rank row must not mint its own per-occurrence rank row;
//	      the view lane (CausalImpacts) stays lossless.
//	ORD-B runnable_wait per-CPU 拆行漏折 (cap2 观察①: seats #1/#2/#3 all
//	      OS_FFRT_3_45387-46792 runnable_wait, cpu=1/3/2 — the off-CPU stats
//	      buckets are keyed (pid, comm, CPU), so ONE thread splits its own
//	      vote across CPU rows; §24.7.1 通用规则: 同一(线程,类型)多实例合并量
//	      参赛, cpu 是区分键 roster 保留). The four off-CPU top families join
//	      the §24.7.1 same-(thread,type) fold lane; segments of one thread are
//	      producer-guaranteed disjoint (single open-segment state machine in
//	      computeOffCPUStats), so the family sums (合计, 同线程) even though
//	      the per-CPU bucket ENVELOPES interleave.
//	ORD-C aggregate top-8 修剪吞席 (P2③ "aggregate top-8 折叠吞携榜席成员"):
//	      the AggregatedImpacts top-8 trim is a VIEW capacity measure (PTS:
//	      derived view) — seat allocation reads the FULL aggregate census, so
//	      a family beyond the view trim still competes for its seat.
//	ORD-D 周期源榜位 (P2③ "周期源榜位缺席", huadong_792 E12 VSyncGenerator
//	      ×11 eff 0.358 with NO seat): the intermediate-sleep skip swallowed
//	      periodic-source aggregates entirely; per §7.8 VS-1 + §28.7 G9 复核
//	      纠偏 ("周期源保留席位,恢复 VS-1 参赛形") a typed PeriodicSource
//	      aggregate bypasses the skip and competes with its DISCOUNTED value.
//	      Non-periodic intermediate sleeps stay seatless (chain plumbing).
//	ORD-E 序数洞 (P2③, acceptance pack criterion 2 "每窗榜位序数连续无洞"):
//	      with A/B/C/D the duplicate/split seats disappear BEFORE ordinal
//	      assignment, so the published ordinals are contiguous per window.
//
// MUTATION self-checks (recorded in the batch report):
//   - M1 revert the member-suppression arm (occurrences re-mint beside their
//     seated aggregate) → TestAggregateMemberSeatExclusivityORD red;
//   - M2 remove runnable_wait from the family-fold lane →
//     TestRunnablePerCPUFamilySingleSeatORD red;
//   - M3 remove the producer-disjoint caliber arm →
//     TestOffCPUProducerDisjointCaliberORD red (falls to MAX);
//   - M4 seat from the trimmed view list instead of the full census →
//     TestAggregateFullCensusSeatsBeyondTopEightORD red;
//   - M5 remove the PeriodicSource bypass on the intermediate-sleep skip →
//     TestPeriodicIntermediateSleepKeepsSeatORD red;
//   - M6 drop the ChainDepth>0 guard from the suppression predicate →
//     TestDepthZeroRunningRowSurvivesMemberSuppressionORD red;
//   - M7 drop the source identity from the family-fold key →
//     TestOffCPUFamilyNeverMergesAcrossSourcesORD red.
//
// 复核收尾 pins (SHIP-WITH-FIXES, 2026-07-10):
//   - P2-1 the lane admission is TYPE-UNIVERSE wide: wakeup_chain-lane rows
//     of one (thread,type,source) fold too — they carry NO typed ts, so the
//     family honestly publishes the member MAX with the raw Σ preserved on
//     the member_sum disclosure channel (cmp_792:396 E11 witness: main-6565
//     sleep ×6 窗口投影 29.298=Σ → engine single row @14.561 MAX; fixes the
//     §29.8 P3 "树头单项最大29.298实为和" account) →
//     TestChainLaneSleepFamilyFoldsToMaxE11FormORD;
//   - P3-1 the producer-disjointness proof premise is a globally ORDERED
//     event stream — the proof mints only when idx.ClockRegressions == 0
//     (M8: dropping the gate reds TestProducerProofRequiresOrderedStreamORD);
//   - P3-2 per-mint-site proof tripwires — flipping ANY of the four off-CPU
//     mint sites' proof bit reds TestOffCPUProofCensusAllFourMintSitesORD
//     (M9a-M9d recorded per site).

import (
	"fmt"
	"strings"
	"testing"
)

// assertRankOrdinalsContiguous pins the acceptance criterion (验收判据 2,
// docs/design/revisit_acceptance_pack_20260709.md): over the published items,
// the Rank>0 ordinals are exactly 1..K with no holes and no duplicates.
func assertRankOrdinalsContiguous(t *testing.T, items []RootCauseRankItem) {
	t.Helper()
	seen := map[int]bool{}
	max := 0
	for _, item := range items {
		if item.Rank <= 0 {
			continue
		}
		if seen[item.Rank] {
			t.Fatalf("duplicate rank ordinal #%d: %+v", item.Rank, items)
		}
		seen[item.Rank] = true
		if item.Rank > max {
			max = item.Rank
		}
	}
	for i := 1; i <= max; i++ {
		if !seen[i] {
			t.Fatalf("rank ordinal hole at #%d (max #%d): %+v", i, max, items)
		}
	}
}

func rankItemsOfTypeAndPID(items []RootCauseRankItem, typ string, pid int) []RootCauseRankItem {
	var out []RootCauseRankItem
	for _, item := range items {
		if item.Type == typ && item.Thread.PID == pid {
			out = append(out, item)
		}
	}
	return out
}

// --- ORD-B: runnable per-CPU family (cap2 观察①) ------------------------------

// ordRunnableSplitTrace: comp-300 accumulates two runnable segments opened on
// DIFFERENT CPUs (preemption on cpu2, wakeup targeting cpu3) — the
// computeOffCPUStats bucket key (pid/comm/cpu) splits them into two
// RunnableTop rows, the pre-ORD engine seated both.
const ordRunnableSplitTrace = `
        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       comp-300 (300) [002] .... 1.000500: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=comp next_pid=300 next_prio=90
       comp-300 (300) [002] .... 1.001000: sched_switch: prev_comm=comp prev_pid=300 prev_prio=90 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120
       comp-300 (300) [002] .... 1.003000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=comp next_pid=300 next_prio=90
       comp-300 (300) [002] .... 1.003500: sched_switch: prev_comm=comp prev_pid=300 prev_prio=90 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      other-400 (400) [003] .... 1.004000: sched_wakeup: comm=comp pid=300 prio=90 target_cpu=003
       comp-300 (300) [003] .... 1.007000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=comp next_pid=300 next_prio=90
       comp-300 (300) [003] .... 1.007500: sched_switch: prev_comm=comp prev_pid=300 prev_prio=90 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.008000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 1.009000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`

func TestRunnablePerCPUFamilySingleSeatORD(t *testing.T) {
	idx := buildTraceIndex(t, "ord_runnable_split.systrace", ordRunnableSplitTrace)
	q := Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.01, MinDurationMs: 0.5, Limit: 12}
	stats := ComputeWindowStats(idx, q)
	perCPU := 0
	for _, td := range stats.RunnableTop {
		if td.Thread.PID == 300 {
			perCPU++
		}
	}
	if perCPU != 2 {
		t.Fatalf("fixture drifted: want comp-300 split into 2 per-CPU RunnableTop rows, got %d: %+v", perCPU, stats.RunnableTop)
	}
	rank := BuildRootCauseRank(idx, q)
	rows := rankItemsOfTypeAndPID(rank.Items, "runnable_wait", 300)
	if len(rows) != 1 {
		t.Fatalf("ORD-B: one thread = one runnable_wait contender (§24.7.1), got %d rows: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.MemberCount != 2 {
		t.Fatalf("family row must count its 2 per-CPU members, got %+v", row)
	}
	// Producer-disjoint segments of one thread sum (合计, 同线程) — the
	// per-CPU envelopes interleave, but computeOffCPUStats keeps at most one
	// open segment per PID, so the Σ is a genuine same-thread wall figure.
	if row.MemberFoldCaliber != RootCauseMemberFoldCaliberSumDisjoint {
		t.Fatalf("producer-disjoint off-CPU family must publish the Σ caliber, got %q (%+v)", row.MemberFoldCaliber, row)
	}
	if !near(row.CumulativeImpactMs, 5.0, 0.01) {
		t.Fatalf("family value must be the same-thread Σ (2.0+3.0), got %.3f", row.CumulativeImpactMs)
	}
	// 区分键 roster (§24.7.1①): the per-CPU identity must not be lost.
	roster := strings.Join(row.MemberRoster, " | ")
	if !strings.Contains(roster, "cpu=2") || !strings.Contains(roster, "cpu=3") {
		t.Fatalf("family roster must keep the cpu distinguishing keys, got %q", roster)
	}
	if row.Rank <= 0 {
		t.Fatalf("the family contender must hold a board seat, got %+v", row)
	}
	assertRankOrdinalsContiguous(t, rank.Items)
}

// --- ORD-A: aggregate/member seat exclusivity (cap2 观察② / cmp_792 E7) -------

// ordAggregateMemberTrace: app-100 sleeps twice, wk-200 runs then wakes it each
// time — two wakeup-chain occurrences of ONE (thread, running) group, so the
// engine mints an occurrences=2 aggregate. Pre-ORD, the aggregate AND both
// member occurrences seated (the cap2 #4/#5/#6 triple-seat form).
const ordAggregateMemberTrace = `
        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
         wk-200 (200) [002] .... 1.000500: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=wk next_pid=200 next_prio=97
         wk-200 (200) [002] .... 1.004000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
         wk-200 (200) [002] .... 1.004500: sched_switch: prev_comm=wk prev_pid=200 prev_prio=97 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.004200: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 1.005000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
         wk-200 (200) [002] .... 1.005500: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=wk next_pid=200 next_prio=97
         wk-200 (200) [002] .... 1.009000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
         wk-200 (200) [002] .... 1.009500: sched_switch: prev_comm=wk prev_pid=200 prev_prio=97 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.009200: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 1.010000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`

func TestAggregateMemberSeatExclusivityORD(t *testing.T) {
	idx := buildTraceIndex(t, "ord_aggregate_member.systrace", ordAggregateMemberTrace)
	q := Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.01, MinDurationMs: 1.0, Limit: 12}
	chain := BuildWakeupChain(idx, q)
	occurrences := 0
	for _, impact := range chain.CausalImpacts {
		if impact.Thread.PID == 200 && impact.ChainDepth > 0 && impact.DominantState == string(StateRunning) {
			occurrences++
		}
	}
	if occurrences < 2 {
		t.Fatalf("fixture drifted: want ≥2 wakeup-chain running occurrences for wk-200, got %d: %+v", occurrences, chain.CausalImpacts)
	}
	var aggregate *WakeupCausalAggregate
	for i := range chain.AggregatedImpacts {
		if chain.AggregatedImpacts[i].Thread.PID == 200 && chain.AggregatedImpacts[i].DominantState == string(StateRunning) {
			aggregate = &chain.AggregatedImpacts[i]
		}
	}
	if aggregate == nil || aggregate.OccurrenceCount < 2 {
		t.Fatalf("fixture drifted: want the occurrences>=2 aggregate for wk-200, got %+v", chain.AggregatedImpacts)
	}
	rank := BuildRootCauseRank(idx, q)
	rows := rankItemsOfTypeAndPID(rank.Items, "running", 200)
	if len(rows) != 1 {
		t.Fatalf("ORD-A: one occurrence set = one seat — want exactly the aggregate rank row, got %d rows: %+v", len(rows), rows)
	}
	if rows[0].Source != "wakeup_chain.aggregated_impacts" {
		t.Fatalf("the family's one seat is the AGGREGATE row (合并量参赛 §24.7.1), got source %q: %+v", rows[0].Source, rows[0])
	}
	// The view lane stays lossless: both per-occurrence causal-impact records
	// keep publishing (they are the drill-down/detail evidence, not seats).
	if occurrences != 2 {
		t.Fatalf("view lane must stay lossless, got %d occurrence records", occurrences)
	}
	assertRankOrdinalsContiguous(t, rank.Items)
}

// --- ORD-C: full-census seats beyond the view top-8 trim ----------------------

func ordChainWithNAggregateGroups(groups int, depth0RunningPID int) ChainResult {
	chain := ChainResult{Target: ThreadRef{PID: 42, Comm: "target"}}
	base := 100.0
	for g := 0; g < groups; g++ {
		pid := 1001 + g
		// Descending dominant impact so the view trim keeps the FIRST eight
		// groups and folds the rest — the census keeps all of them.
		impact := float64(groups-g) * 2.0
		for occ := 0; occ < 2; occ++ {
			start := base + float64(g)*1.0 + float64(occ)*0.4
			end := start + impact/1000
			chain.CausalImpacts = append(chain.CausalImpacts, WakeupCausalImpact{
				Thread:           ThreadRef{PID: pid, Comm: fmt.Sprintf("wk-%d", pid)},
				ChainDepth:       1,
				DominantState:    string(StateRunning),
				DominantImpactMs: impact,
				TotalMs:          impact,
				RunningMs:        impact,
				TargetBlockedMs:  impact,
				FragmentCount:    1,
				Window:           TimeWindow{StartTs: start, EndTs: end},
				ActualWindow:     TimeWindow{StartTs: start, EndTs: end},
				LineStart:        1000 + g*100 + occ*10,
				LineEnd:          1005 + g*100 + occ*10,
			})
		}
	}
	if depth0RunningPID > 0 {
		chain.CausalImpacts = append(chain.CausalImpacts, WakeupCausalImpact{
			Thread:           ThreadRef{PID: depth0RunningPID, Comm: fmt.Sprintf("wk-%d", depth0RunningPID)},
			ChainDepth:       0,
			DominantState:    string(StateRunning),
			DominantImpactMs: 1.0,
			TotalMs:          1.0,
			RunningMs:        1.0,
			Window:           TimeWindow{StartTs: base + 50, EndTs: base + 50.001},
			ActualWindow:     TimeWindow{StartTs: base + 50, EndTs: base + 50.001},
			LineStart:        9000,
			LineEnd:          9001,
		})
		// Same thread, DIFFERENT dominant state, single occurrence: the
		// aggregation key is (pid, dominant state) — a D-state single of a
		// thread whose RUNNING group seated an aggregate is NOT a member and
		// must keep its own seat (guards the suppression key against a
		// pid-only over-reach).
		chain.CausalImpacts = append(chain.CausalImpacts, WakeupCausalImpact{
			Thread:           ThreadRef{PID: depth0RunningPID, Comm: fmt.Sprintf("wk-%d", depth0RunningPID)},
			ChainDepth:       1,
			DominantState:    string(StateDSleep),
			DominantImpactMs: 0.8,
			TotalMs:          0.8,
			DStateMs:         0.8,
			TargetBlockedMs:  0.8,
			Window:           TimeWindow{StartTs: base + 60, EndTs: base + 60.0008},
			ActualWindow:     TimeWindow{StartTs: base + 60, EndTs: base + 60.0008},
			LineStart:        9100,
			LineEnd:          9101,
		})
	}
	return chain
}

func TestAggregateFullCensusSeatsBeyondTopEightORD(t *testing.T) {
	chain := ordChainWithNAggregateGroups(9, 0)
	// This is a seat-census test, not a CAP test. Convert its synthetic groups
	// to the runnable caliber, whose full typed duration is independently
	// rankable; a raw running wall clock without a supply deficit is correctly
	// rank-0 under the closed effective matrix.
	for i := range chain.CausalImpacts {
		impact := &chain.CausalImpacts[i]
		impact.DominantState = string(StateRunnable)
		impact.RunnableMs = impact.RunningMs
		impact.RunningMs = 0
	}
	chain.AggregatedImpacts = aggregateWakeupCausalImpacts(&chain)
	if len(chain.AggregatedImpacts) != 8 || chain.AggregatedImpactsFold == nil {
		t.Fatalf("fixture drifted: want the top-8 view trim to fire (8 kept + fold), got %d fold=%v", len(chain.AggregatedImpacts), chain.AggregatedImpactsFold)
	}
	q := Query{PID: 42, TimeStart: 100.0, TimeEnd: 115.0, Limit: 12}
	rank := buildRootCauseRankFrom(nil, q, chain, WindowStats{})
	seated := 0
	for _, item := range rank.Items {
		if item.Source == "wakeup_chain.aggregated_impacts" {
			seated++
		}
	}
	if seated != 9 {
		t.Fatalf("ORD-C: seat allocation reads the FULL aggregate census (view trim is display capacity, not a seat gate) — want 9 aggregate seats, got %d: %+v", seated, rank.Items)
	}
	// And none of the 18 member occurrences re-mint beside their aggregates.
	for pid := 1001; pid <= 1009; pid++ {
		rows := rankItemsOfTypeAndPID(rank.Items, "runnable_wait", pid)
		if len(rows) != 1 || rows[0].Source != "wakeup_chain.aggregated_impacts" {
			t.Fatalf("pid %d: want exactly the aggregate seat, got %+v", pid, rows)
		}
	}
	assertRankOrdinalsContiguous(t, rank.Items)
}

func TestDepthZeroRunningRowSurvivesMemberSuppressionORD(t *testing.T) {
	// M6 anti-overreach pin: pid 1001 has a SEATED (pid, running) aggregate
	// AND a depth-0 running impact — the aggregate groups only ChainDepth>0
	// members, so the depth-0 row is NOT a member and must keep minting
	// (§20 A-fix(3) depth-0 running exception).
	chain := ordChainWithNAggregateGroups(1, 1001)
	chain.AggregatedImpacts = aggregateWakeupCausalImpacts(&chain)
	q := Query{PID: 42, TimeStart: 100.0, TimeEnd: 160.0, Limit: 12}
	rank := buildRootCauseRankFrom(nil, q, chain, WindowStats{})
	rows := rankItemsOfTypeAndPID(rank.Items, "running", 1001)
	if len(rows) != 2 {
		t.Fatalf("want the aggregate seat AND the depth-0 running row, got %+v", rows)
	}
	sources := map[string]bool{}
	for _, row := range rows {
		sources[row.Source] = true
	}
	if !sources["wakeup_chain.aggregated_impacts"] || !sources["wakeup_chain.causal_impacts"] {
		t.Fatalf("want one aggregate seat + one depth-0 causal-impact row, got %+v", rows)
	}
	// Cross-state guard: the same thread's single D-state occurrence is not
	// a member of the seated RUNNING aggregate (key = pid AND dominant
	// state) and keeps its own seat.
	if rows := rankItemsOfTypeAndPID(rank.Items, "d_state_or_io_wait", 1001); len(rows) != 1 {
		t.Fatalf("the same thread's single D-state occurrence must keep minting, got %+v", rank.Items)
	}
}

// --- ORD-D: periodic source keeps its seat ------------------------------------

func TestPeriodicIntermediateSleepKeepsSeatORD(t *testing.T) {
	chain := buildPeriodicVSyncChain(berlinVSyncIntervalsMs, berlinVSyncSleepMs, berlinVSyncRunnableMs)
	// The periodic thread is an intermediate chain node: something wakes IT —
	// the huadong VSyncGenerator shape (…→VSyncGenerator→target).
	chain.Edges = append(chain.Edges, WakeupEdge{
		Waker: ThreadRef{PID: 777, Comm: "upstream"},
		Wakee: ThreadRef{PID: 610, Comm: "VSyncGenerator"},
	})
	chain.AggregatedImpacts = aggregateWakeupCausalImpacts(&chain)
	if len(chain.AggregatedImpacts) != 1 || !chain.AggregatedImpacts[0].PeriodicSource {
		t.Fatalf("fixture drifted: want the periodic aggregate, got %+v", chain.AggregatedImpacts)
	}
	q := Query{PID: 4144, TimeStart: 4520.0, TimeEnd: 4520.2, Limit: 12}
	rank := buildRootCauseRankFrom(nil, q, chain, WindowStats{})
	rows := rankItemsOfTypeAndPID(rank.Items, "sleep_wait", 610)
	if len(rows) != 1 {
		t.Fatalf("ORD-D: a typed periodic source bypasses the intermediate-sleep skip and holds its seat (§7.8 VS-1 + §28.7 G9 复核纠偏), got %+v", rank.Items)
	}
	row := rows[0]
	if !row.PeriodicSource || row.Source != "wakeup_chain.aggregated_impacts" {
		t.Fatalf("the periodic seat is the aggregate row with its typed cadence identity, got %+v", row)
	}
	if !near(row.EffectiveImpactMs, 0.105, 0.001) {
		t.Fatalf("the periodic seat competes with its DISCOUNTED attribution (runnable+lateness), got %+v", row)
	}
	if row.Rank <= 0 {
		t.Fatalf("the periodic row must carry a board ordinal, got %+v", row)
	}
	assertRankOrdinalsContiguous(t, rank.Items)

	// Negative arm: the SAME shape without the periodic stamp stays seatless —
	// a non-periodic intermediate sleep is chain plumbing, its wait is
	// explained by ITS upstream (skip unchanged).
	aperiodic := buildPeriodicVSyncChain([]float64{3.0, 9.5, 2.2, 14.0, 5.1}, berlinVSyncSleepMs, berlinVSyncRunnableMs)
	aperiodic.Edges = append(aperiodic.Edges, WakeupEdge{
		Waker: ThreadRef{PID: 777, Comm: "upstream"},
		Wakee: ThreadRef{PID: 610, Comm: "VSyncGenerator"},
	})
	aperiodic.AggregatedImpacts = aggregateWakeupCausalImpacts(&aperiodic)
	if len(aperiodic.AggregatedImpacts) != 1 || aperiodic.AggregatedImpacts[0].PeriodicSource {
		t.Fatalf("fixture drifted: want ONE non-periodic aggregate, got %+v", aperiodic.AggregatedImpacts)
	}
	rank2 := buildRootCauseRankFrom(nil, q, aperiodic, WindowStats{})
	if rows := rankItemsOfTypeAndPID(rank2.Items, "sleep_wait", 610); len(rows) != 0 {
		t.Fatalf("a NON-periodic intermediate sleep must stay seatless (plumbing), got %+v", rows)
	}
}

// --- fold-key/caliber unit pins ------------------------------------------------

func ordOffCPUItem(source string, cpu int, raw, start, end float64) RootCauseRankItem {
	item := rootCauseItem("runnable_wait", ThreadRef{PID: 300, Comm: "comp"}, raw, 0.76, 100, 110, source, "comp runnable")
	item.CumulativeImpactMs = raw
	item.StartTs = start
	item.EndTs = end
	item.DominantState = string(StateRunnable)
	item.RunnableMs = raw
	item.MemberKey = fmt.Sprintf("cpu=%d", cpu)
	item.memberSegmentsProducerDisjoint = true
	return item
}

func TestOffCPUProducerDisjointCaliberORD(t *testing.T) {
	q := Query{TimeStart: 1.0, TimeEnd: 2.0}
	// Interleaved per-CPU bucket ENVELOPES (the cap2 shape: lines/ts ranges of
	// the cpu=1/3/2 rows all overlap) — envelope disjointness cannot prove the
	// sum, the producer guarantee does.
	a := ordOffCPUItem("window_stats", 2, 2.0, 1.10, 1.50)
	b := ordOffCPUItem("window_stats", 3, 3.0, 1.20, 1.60)
	out := foldSameThreadTypeRankFamilies(q, false, []RootCauseRankItem{a, b})
	if len(out) != 1 {
		t.Fatalf("want one family row, got %+v", out)
	}
	if out[0].MemberFoldCaliber != RootCauseMemberFoldCaliberSumDisjoint || !near(out[0].CumulativeImpactMs, 5.0, 0.001) {
		t.Fatalf("producer-disjoint members must Σ (合计,同线程), got %+v", out[0])
	}
	// Without the producer guarantee the SAME overlap honestly degrades to
	// the member MAX (a lower bound) — never a naive Σ.
	c := ordOffCPUItem("window_stats", 2, 2.0, 1.10, 1.50)
	d := ordOffCPUItem("window_stats", 3, 3.0, 1.20, 1.60)
	c.memberSegmentsProducerDisjoint = false
	d.memberSegmentsProducerDisjoint = false
	out2 := foldSameThreadTypeRankFamilies(q, false, []RootCauseRankItem{c, d})
	if len(out2) != 1 || out2[0].MemberFoldCaliber != RootCauseMemberFoldCaliberMaxOverlapFallback || !near(out2[0].CumulativeImpactMs, 3.0, 0.001) {
		t.Fatalf("unproven overlap must fall back to the member MAX, got %+v", out2)
	}
}

func TestOffCPUFamilyNeverMergesAcrossSourcesORD(t *testing.T) {
	// M7 pin: the wakeup-chain lane and the window-stats lane measure the SAME
	// physical time through different rulers — same (thread,type) rows from
	// different producers must never Σ into one family (双计防线; the fold key
	// carries the mint-source identity).
	q := Query{TimeStart: 1.0, TimeEnd: 2.0}
	a := ordOffCPUItem("window_stats", 2, 2.0, 1.10, 1.50)
	b := ordOffCPUItem("wakeup_chain", 3, 3.0, 1.20, 1.60)
	out := foldSameThreadTypeRankFamilies(q, false, []RootCauseRankItem{a, b})
	if len(out) != 2 {
		t.Fatalf("cross-source rows must stay separate, got %+v", out)
	}
}

// --- 复核 P2-1: chain-lane (E11 form) family fold behavior change -------------

func TestChainLaneSleepFamilyFoldsToMaxE11FormORD(t *testing.T) {
	// cmp_792:396 witness shape: six same-thread sleep rows on the
	// wakeup_chain lane (engine-minted RootEvidence rank rows carry NO typed
	// ts identity) used to publish six seats whose display merge summed
	// 窗口投影 to 29.298 while the largest member is 14.561. Post-ORD the
	// lane admission folds them into ONE contender: no interval identity →
	// honest member MAX (max_overlap_fallback), raw Σ preserved on the
	// member_sum disclosure channel. The RootEvidence entries are minted by
	// the REAL evidence constructor (rootEvidenceFromCausalImpact) and the
	// rank rows by the REAL RootEvidence mint loop in buildRootCauseRankFrom
	// (fixture 取引擎实铸形).
	sleeps := []float64{14.561, 4.000, 3.129, 3.000, 2.928, 1.680} // Σ = 29.298
	chain := ChainResult{Target: ThreadRef{PID: 42, Comm: "target"}}
	for i, ms := range sleeps {
		impact := WakeupCausalImpact{
			Thread:           ThreadRef{PID: 6565, Comm: "main"},
			ChainDepth:       1,
			DominantState:    string(StateSSleep),
			DominantImpactMs: ms,
			TotalMs:          ms,
			SleepMs:          ms,
			LineStart:        1000 + i*10,
			LineEnd:          1005 + i*10,
		}
		root := rootEvidenceFromCausalImpact(impact, "sleep evidence", 0.7)
		if root.Type != "sleep_wait" {
			t.Fatalf("fixture drifted: the evidence constructor must mint sleep_wait, got %+v", root)
		}
		chain.RootEvidence = append(chain.RootEvidence, root)
	}
	q := Query{PID: 42, TimeStart: 1.0, TimeEnd: 2.0, Limit: 12}
	rank := buildRootCauseRankFrom(nil, q, chain, WindowStats{})
	rows := rankItemsOfTypeAndPID(rank.Items, "sleep_wait", 6565)
	if len(rows) != 1 {
		t.Fatalf("E11 form: six chain-lane sleep rows of one thread must fold into ONE contender, got %d: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.MemberCount != 6 {
		t.Fatalf("family must count its 6 members, got %+v", row)
	}
	// No typed ts on the chain lane → no disjointness proof → the published
	// value is the member MAX (14.561), NEVER the naive Σ (29.298) —
	// 墙钟纪律: an unprovable overlap must not over-claim.
	if row.MemberFoldCaliber != RootCauseMemberFoldCaliberMaxOverlapFallback {
		t.Fatalf("chain-lane family must publish the MAX caliber, got %q (%+v)", row.MemberFoldCaliber, row)
	}
	if !near(row.CumulativeImpactMs, 14.561, 0.001) || !near(row.ImpactMs, 14.561, 0.001) {
		t.Fatalf("published value must be the member MAX 14.561, got cum=%.3f impact=%.3f", row.CumulativeImpactMs, row.ImpactMs)
	}
	// The raw Σ survives losslessly on the member_sum disclosure channel.
	if !near(row.MemberSumMs, 29.298, 0.001) {
		t.Fatalf("raw member Σ must stay disclosed via member_sum, got %+v", row)
	}
	assertRankOrdinalsContiguous(t, rank.Items)
}

// --- 复核 P3-1: ordered-stream premise gate ------------------------------------

// ordInterleavedRunnableTrace builds comp-300 with THREE runnable segments on
// alternating CPUs (cpu2 1ms, cpu3 1ms, cpu2 1.5ms) so the per-CPU bucket
// ENVELOPES interleave (cpu2 [1.001,1.0065] ⊃ cpu3 [1.003,1.004]) — the shape
// where ONLY the producer proof can admit the Σ caliber. withRegression
// inserts one out-of-order event (an unrelated wakeup whose ts moves
// backwards), the exact ClockRegressions shape the proof premise excludes.
func ordInterleavedRunnableTrace(withRegression bool) string {
	lines := []string{
		`        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`       comp-300 (300) [002] .... 1.000500: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=comp next_pid=300 next_prio=90`,
		`       comp-300 (300) [002] .... 1.001000: sched_switch: prev_comm=comp prev_pid=300 prev_prio=90 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120`,
		`       comp-300 (300) [002] .... 1.002000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=comp next_pid=300 next_prio=90`,
		`       comp-300 (300) [002] .... 1.002500: sched_switch: prev_comm=comp prev_pid=300 prev_prio=90 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120`,
		`      other-400 (400) [003] .... 1.003000: sched_wakeup: comm=comp pid=300 prio=90 target_cpu=003`,
		`       comp-300 (300) [003] .... 1.004000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=comp next_pid=300 next_prio=90`,
		`       comp-300 (300) [003] .... 1.004500: sched_switch: prev_comm=comp prev_pid=300 prev_prio=90 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120`,
		`      other-400 (400) [003] .... 1.005000: sched_wakeup: comm=comp pid=300 prio=90 target_cpu=002`,
		`       comp-300 (300) [002] .... 1.006500: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=comp next_pid=300 next_prio=90`,
		`       comp-300 (300) [002] .... 1.007000: sched_switch: prev_comm=comp prev_pid=300 prev_prio=90 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120`,
		`        app-100 (100) [001] .... 1.008000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52`,
		`        app-100 (100) [001] .... 1.009000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
	}
	if withRegression {
		lines = append(lines, `      other-400 (400) [003] .... 1.003500: sched_wakeup: comm=other pid=400 prio=90 target_cpu=003`)
	}
	return "\n" + strings.Join(lines, "\n") + "\n"
}

func TestProducerProofRequiresOrderedStreamORD(t *testing.T) {
	q := Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.01, MinDurationMs: 0.4, Limit: 12}

	// Ordered stream: the interleaved envelopes are provably disjoint via the
	// producer proof — the family sums (1.0 + 1.0 + 1.5 across cpu2/cpu3).
	idx := buildTraceIndex(t, "ord_interleaved_ordered.systrace", ordInterleavedRunnableTrace(false))
	if idx.ClockRegressions != 0 {
		t.Fatalf("fixture drifted: ordered stream must carry zero regressions, got %d", idx.ClockRegressions)
	}
	rank := BuildRootCauseRank(idx, q)
	rows := rankItemsOfTypeAndPID(rank.Items, "runnable_wait", 300)
	if len(rows) != 1 || rows[0].MemberCount != 2 {
		t.Fatalf("want one two-bucket runnable family, got %+v", rows)
	}
	if rows[0].MemberFoldCaliber != RootCauseMemberFoldCaliberSumDisjoint || !near(rows[0].CumulativeImpactMs, 3.5, 0.01) {
		t.Fatalf("ordered stream: producer proof must admit the Σ caliber (3.5), got %+v", rows[0])
	}

	// Regressed stream: the ordered-stream premise fails — the proof must not
	// mint and the SAME shape honestly degrades to the member MAX (the cpu2
	// bucket, 2.5), never an over-count Σ.
	idxReg := buildTraceIndex(t, "ord_interleaved_regressed.systrace", ordInterleavedRunnableTrace(true))
	if idxReg.ClockRegressions == 0 {
		t.Fatalf("fixture drifted: want a clock regression, got none")
	}
	rankReg := BuildRootCauseRank(idxReg, q)
	rowsReg := rankItemsOfTypeAndPID(rankReg.Items, "runnable_wait", 300)
	if len(rowsReg) != 1 || rowsReg[0].MemberCount != 2 {
		t.Fatalf("regressed stream: still one family, got %+v", rowsReg)
	}
	if rowsReg[0].MemberFoldCaliber != RootCauseMemberFoldCaliberMaxOverlapFallback || !near(rowsReg[0].CumulativeImpactMs, 2.5, 0.01) {
		t.Fatalf("regressed stream: proof must not mint — MAX fallback (2.5) expected, got %+v", rowsReg[0])
	}
}

// --- 复核 P3-2: per-mint-site proof census -------------------------------------

func TestOffCPUProofCensusAllFourMintSitesORD(t *testing.T) {
	// One thread per off-CPU family, TWO per-CPU bucket rows each with
	// INTERLEAVED envelopes (outer [1.10,1.60] ⊃ inner [1.20,1.40]) — only
	// the mint-site producer proof admits the Σ caliber, so flipping ANY of
	// the four sites' proof bit reds exactly its family here (silent MAX
	// degradation gets a tripwire per site). The idx is a real regression-free
	// index (the P3-1 gate reads it).
	idx := buildTraceIndex(t, "ord_proof_census.systrace", ordRunnableSplitTrace)
	if idx.ClockRegressions != 0 {
		t.Fatalf("fixture drifted: census idx must be regression-free")
	}
	mk := func(pid int, comm string) [2]ThreadDuration {
		return [2]ThreadDuration{
			{Thread: ThreadRef{PID: pid, Comm: comm}, DurationMs: 2.0, CPU: 2, StartTs: 1.10, EndTs: 1.60, LineStart: 10, LineEnd: 60},
			{Thread: ThreadRef{PID: pid, Comm: comm}, DurationMs: 3.0, CPU: 3, StartTs: 1.20, EndTs: 1.40, LineStart: 20, LineEnd: 40},
		}
	}
	runnable := mk(501, "r-thread")
	sleep := mk(502, "s-thread")
	iowait := mk(503, "i-thread")
	dstate := mk(504, "d-thread")
	stats := WindowStats{
		Window:      TimeWindow{StartTs: 1.0, EndTs: 2.0},
		RunnableTop: runnable[:],
		SleepTop:    sleep[:],
		IOWaitTop:   iowait[:],
		DStateTop:   dstate[:],
	}
	q := Query{PID: 42, TimeStart: 1.0, TimeEnd: 2.0, Limit: 12}
	rank := buildRootCauseRankFrom(idx, q, ChainResult{}, stats)
	for _, tc := range []struct {
		typ string
		pid int
	}{
		{"runnable_wait", 501},
		{"sleep_wait", 502},
		{"io_wait", 503},
		{"d_state_or_io_wait", 504},
	} {
		rows := rankItemsOfTypeAndPID(rank.Items, tc.typ, tc.pid)
		if len(rows) != 1 {
			t.Fatalf("%s: want one family row, got %+v", tc.typ, rows)
		}
		if rows[0].MemberCount != 2 {
			t.Fatalf("%s: want 2 members, got %+v", tc.typ, rows[0])
		}
		if rows[0].MemberFoldCaliber != RootCauseMemberFoldCaliberSumDisjoint {
			t.Fatalf("%s: mint-site proof must admit the Σ caliber (silent MAX degradation tripwire), got %q", tc.typ, rows[0].MemberFoldCaliber)
		}
		if !near(rows[0].CumulativeImpactMs, 5.0, 0.001) {
			t.Fatalf("%s: family value must be the same-thread Σ, got %+v", tc.typ, rows[0])
		}
	}
}
