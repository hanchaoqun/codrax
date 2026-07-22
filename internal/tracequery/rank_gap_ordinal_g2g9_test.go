package tracequery

// rank_gap_ordinal_g2g9_test.go — Wave-3.1 GAP-A engine pins (ledger
// real_trace_campaign_20260705.md §27.2 G2 / §27.3 G9 / §27.2 G3 / §27.4 G10,
// user ruling §28.1, 2026-07-09):
//
//	G2a trace_gap 降道跳臂 — a data blind spot wears RootCauseTierDataGap,
//	    takes no election slot, shifts nothing below it and carries no rank
//	    ordinal (数据盲区非成因; pre-G2 the opendir_79/huadong_79 boards seated
//	    blind spots at #6-#12).
//	G2b 判据 typed 化 — the two nil-interesting shapes are typed apart at the
//	    single mint site: no_sched_data (timeline truly empty) vs
//	    no_eligible_wait (intervals exist but ALL sit below the MinDurationMs
//	    floor — 复核 P3-5 precise fact; the shape the legacy "窗内无调度数据"
//	    claim over-asserted against; §27.2 witness: the same (thread, window)
//	    legally carries a depth-0 running rank row beside the blind spot).
//	G9  序数引擎侧重编号 — rank ordinals go ONLY to rows carrying a rank-board
//	    display identity; demoted rows (target_self_state 等待症状 / data_gap;
//	    复核 P1-2 removed the initial PeriodicSource arm — discounted periodic
//	    rows compete in full) carry Rank=0 and the sequence stays contiguous
//	    over the visible rows (三面同源: the engine ordinal is the only
//	    ordinal; the display badge gate Rank>0 follows by construction).
//	G3  count_sum 家族恒等式 — a Count-additivity family publishes ONE value
//	    on every value channel (ImpactMs == CumulativeImpactMs ==
//	    EffectiveImpactMs == the §7.5-capped published value); the raw
//	    count-equivalent Σ survives on the member_sum disclosure and every
//	    printed count magnitude wears the 计数当量 marker instead of a bare
//	    wall-clock "ms" (arm-level per-CLASS: page_cache_churn and
//	    file_io_hot_inode share it).
//	G10 撤回披露 witness 本地化 — the same-lock self-contradiction witness is
//	    minted in Chinese (§22.2.1 词条尺子; payload/tid untranslated, number
//	    and line formats preserved), so the zh 明细 face never ships an EN
//	    sentence verbatim.
//
// MUTATION self-checks (recorded in the batch report):
//   - reverting the trace_gap arm (blind spots re-enter the ladder) reds
//     TestTraceGapDemotesToDataGapTierG2 and the end-to-end kind pin;
//   - restoring the unconditional `Rank = i+1` pre-assignment reds
//     TestRankOrdinalsOnlyForBoardVisibleRowsG9;
//   - reverting the count-arm published-value override reds
//     TestCountFamilyFoldPublishedValueIdentityG3;
//   - reverting the witness to the EN sentence reds
//     TestLockHolderSelfContradictionWitnessIsChineseG10.

import (
	"strings"
	"testing"
)

// --- G2b: typed criterion split -------------------------------------------------

func TestTraceGapKindForTimelineG2(t *testing.T) {
	if got := traceGapKindForTimeline(nil); got != TraceGapKindNoSchedData {
		t.Fatalf("an empty timeline is the only true no-scheduler-data shape, got %q", got)
	}
	if got := traceGapKindForTimeline([]Interval{{State: StateRunning, DurationMs: 0.04}}); got != TraceGapKindNoEligibleWait {
		t.Fatalf("a timeline WITH intervals must never claim no-scheduler-data, got %q", got)
	}
}

// gapKindTraceNoSchedData: the woken target's waker (ghost-500) emits only the
// wakeup row — its own timeline inside the aligned child window is EMPTY.
const gapKindTraceNoSchedData = `
        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
      ghost-500 (500) [003] .... 1.030000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 1.035000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 1.039000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`

// gapKindTraceNoEligibleWait: the waker HAS scheduler data — a 0.040ms running
// interval below the 0.05ms floor — the §27.2 OS_FFRT shape where the legacy
// "窗内无调度数据" wording contradicted the depth-0 running rank row minted
// from the very same (thread, window).
const gapKindTraceNoEligibleWait = `
        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
      ghost-500 (500) [003] .... 1.029960: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=ghost next_pid=500 next_prio=30
      ghost-500 (500) [003] .... 1.030000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 1.035000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 1.039000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`

func gapKindQuery() Query {
	return Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.04, MinDurationMs: 0.05, Limit: 12}
}

func findRankItem(items []RootCauseRankItem, typ string, pid int) *RootCauseRankItem {
	for i := range items {
		if items[i].Type == typ && items[i].Thread.PID == pid {
			return &items[i]
		}
	}
	return nil
}

func TestTraceGapMintCarriesTypedKindEndToEndG2(t *testing.T) {
	// Shape 1: truly empty waker timeline → no_sched_data, and the honest
	// summary states exactly that form.
	idx := buildTraceIndex(t, "gap_no_sched_data.systrace", gapKindTraceNoSchedData)
	chain := BuildWakeupChain(idx, gapKindQuery())
	var gap *RootEvidence
	for i := range chain.RootEvidence {
		if chain.RootEvidence[i].Type == "trace_gap" {
			gap = &chain.RootEvidence[i]
		}
	}
	if gap == nil {
		t.Fatalf("fixture drifted: expected a trace_gap root evidence, got %+v", chain.RootEvidence)
	}
	if gap.GapKind != TraceGapKindNoSchedData {
		t.Fatalf("empty waker timeline must mint kind=no_sched_data, got %+v", gap)
	}
	if !strings.Contains(gap.Summary, "no scheduler intervals for this thread") {
		t.Fatalf("the no_sched_data summary must state the empty-timeline form, got %q", gap.Summary)
	}
	rank := BuildRootCauseRank(idx, gapKindQuery())
	item := findRankItem(rank.Items, "trace_gap", 500)
	if item == nil {
		t.Fatalf("the blind-spot observation must keep publishing as a rank row, got %+v", rank.Items)
	}
	if item.TraceGapKind != TraceGapKindNoSchedData || item.Tier != RootCauseTierDataGap || item.Rank != 0 {
		t.Fatalf("rank row must carry the typed kind + data_gap tier + no ordinal, got %+v", item)
	}

	// Shape 2: the waker HAS a (sub-floor, running-only) interval → the typed
	// criterion says no_eligible_wait, never the over-strong no-data claim —
	// and the SAME (thread, window) legitimately carries a depth-0 running
	// rank row beside the blind spot (§27.2 self-contradiction witness,
	// now typed apart instead of contradictory).
	idx2 := buildTraceIndex(t, "gap_no_eligible_wait.systrace", gapKindTraceNoEligibleWait)
	rank2 := BuildRootCauseRank(idx2, gapKindQuery())
	item2 := findRankItem(rank2.Items, "trace_gap", 500)
	if item2 == nil {
		t.Fatalf("expected the ghost blind-spot rank row, got %+v", rank2.Items)
	}
	if item2.TraceGapKind != TraceGapKindNoEligibleWait || item2.Tier != RootCauseTierDataGap || item2.Rank != 0 {
		t.Fatalf("sub-floor running-only shape must mint kind=no_eligible_wait + data_gap + no ordinal, got %+v", item2)
	}
	running := findRankItem(rank2.Items, "running", 500)
	if running == nil {
		t.Fatalf("the depth-0 running row of the same thread must keep competing, got %+v", rank2.Items)
	}
	// The retired over-claim must not survive anywhere on the result.
	for _, it := range rank2.Items {
		if strings.Contains(it.Summary, "no decisive scheduler interval") {
			t.Fatalf("the retired blanket claim resurfaced: %q", it.Summary)
		}
	}
}

// --- G2a + G9: demotion arm + ordinal renumbering --------------------------------

func TestTraceGapDemotesToDataGapTierG2(t *testing.T) {
	items := []RootCauseRankItem{
		{Type: "trace_gap", Thread: ThreadRef{Comm: "ghost", PID: 500}, TraceGapKind: TraceGapKindNoSchedData, Confidence: 0.6},
		{Type: "workqueue_activity", ImpactMs: 9, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "dma_fence_activity", ImpactMs: 5, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
	}
	assignRootCauseRankOrdinalsAndTiers(items)
	if items[0].Tier != RootCauseTierDataGap || items[0].Rank != 0 {
		t.Fatalf("a blind-spot row must wear data_gap with no ordinal: %+v", items[0])
	}
	// Election transparency: the blind spot consumes no slot and shifts
	// nothing — the first competing row is the positional primary at #1.
	if items[1].Tier != "primary" || items[1].Rank != 1 {
		t.Fatalf("the ladder head must fall to the first competing row: %+v", items[1])
	}
	if items[2].Tier != "secondary" || items[2].Rank != 2 {
		t.Fatalf("the ladder below the transparent blind spot must be unshifted: %+v", items[2])
	}
}

func TestRankOrdinalsOnlyForBoardVisibleRowsG9(t *testing.T) {
	// The §27.3 board-hole shape in one window: demoted rows interleaved with
	// competing rows. Pre-G9 the visible board read holes at the demoted
	// seats; the engine now numbers ONLY the board-visible rows, contiguously.
	//
	// EVOLUTION RECORD (复核 P1-2, 2026-07-09): the pin's original form
	// expected the PeriodicSource row at Rank=0 — that arm's premise ("the
	// display already suppresses periodic board rows") was FALSIFIED by the
	// adversarial review: the shared board (runtimeTraceProjRankBoard) has no
	// PeriodicSource filter arm and every board/lead/➊/成因-grammar gate keys
	// on Rank>0, so Rank=0 stripped the discounted row's §24 裁定① competition
	// identity and killed the VS-1 late-period crowning form. The periodic
	// no-ordinal arm was deleted: discounted periodic rows take ordinals like
	// every competing row; only the wait-symptom / data_gap arms skip.
	items := []RootCauseRankItem{
		{Type: "binder_wait", SubjectIsAnalysisTarget: true, ImpactMs: 90, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "sleep_wait", PeriodicSource: true, EffectiveImpactMs: 0.1, ImpactMs: 36, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "workqueue_activity", ImpactMs: 9, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "trace_gap", Thread: ThreadRef{Comm: "ghost", PID: 500}, TraceGapKind: TraceGapKindNoEligibleWait},
		{Type: "dma_fence_activity", ImpactMs: 5, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "jit_compile", ImpactMs: 4, EffectiveImpactMs: 4},
	}
	assignRootCauseRankOrdinalsAndTiers(items)
	// Demoted rows: no ordinal, tier semantics per their own arms.
	if items[0].Rank != 0 || items[0].Tier != RootCauseTierTargetSelfState {
		t.Fatalf("wait-symptom self row: no seat: %+v", items[0])
	}
	// The periodic row COMPETES in full — election slot (here the head slot)
	// AND board ordinal #1 (复核 P1-2 restored VS-1 form).
	if items[1].Rank != 1 || items[1].Tier != "primary" {
		t.Fatalf("periodic discounted row keeps election slot AND ordinal: %+v", items[1])
	}
	if items[3].Rank != 0 || items[3].Tier != RootCauseTierDataGap {
		t.Fatalf("blind-spot row: no seat: %+v", items[3])
	}
	// Board-visible rows: contiguous 1..4 (non-chain semantic row included —
	// its background board shows the seat).
	if items[2].Rank != 2 || items[2].Tier != "secondary" {
		t.Fatalf("the next competing row follows contiguously at #2: %+v", items[2])
	}
	if items[4].Rank != 3 || items[4].Tier != "tertiary" {
		t.Fatalf("third visible row must be #3: %+v", items[4])
	}
	// BackgroundRank POSITION counts every published non-on-chain row — the
	// non-chain blind-spot row included (F-2 semantics untouched by G9), so
	// the semantic row sits at background position 2.
	if items[5].Rank != 4 || items[5].Tier != "tertiary" || items[5].BackgroundRank != 2 {
		t.Fatalf("the non-chain semantic row keeps a seat (#4) and its background board position: %+v", items[5])
	}
	// Idempotency: the enrich pass re-runs the assignment over the same slice.
	assignRootCauseRankOrdinalsAndTiers(items)
	got := []int{items[0].Rank, items[1].Rank, items[2].Rank, items[3].Rank, items[4].Rank, items[5].Rank}
	want := []int{0, 1, 2, 0, 3, 4}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("re-assignment must be idempotent: got %v want %v", got, want)
		}
	}
}

// --- G3: count-family published-value identity -----------------------------------

func countFamilyFixtureQuery() Query {
	// 100ms window → §7.5 background cap = 35ms; the raw count-equivalent Σ
	// (133.2 + 65.1 = 198.3) sits far above it — the opendir_79 页缓存抖动
	// shape scaled onto a fixture window.
	return Query{TimeStart: 10.0, TimeEnd: 10.1}
}

func countFamilyMember(inode string, churn int, startTs, endTs float64, lineStart, lineEnd int) RootCauseRankItem {
	q := countFamilyFixtureQuery()
	raw := float64(churn) * 0.3
	item := rootCauseItem("page_cache_churn", ThreadRef{Comm: "pc", PID: 91}, backgroundImpactMs(q, raw, true, false), 0.66, lineStart, lineEnd, "window_stats.page_cache_by_inode", "page cache churn inode="+inode)
	item.CumulativeImpactMs = raw
	item.Causality = "background"
	item.ChainRelevance = "background"
	item.StartTs = startTs
	item.EndTs = endTs
	item.Inode = inode
	item.Dev = "253,0"
	item.MemberKey = "inode=" + inode + " dev=253,0"
	return item
}

func TestCountFamilyFoldPublishedValueIdentityG3(t *testing.T) {
	q := countFamilyFixtureQuery()
	items := []RootCauseRankItem{
		countFamilyMember("0x1", 444, 10.010, 10.020, 100, 110), // raw 133.200
		countFamilyMember("0x2", 217, 10.030, 10.040, 200, 210), // raw 65.100
	}
	out := foldSameThreadTypeRankFamilies(q, true, items)
	if len(out) != 1 {
		t.Fatalf("same-(thread,type) count rows must merge into one contender, got %d: %+v", len(out), out)
	}
	merged := out[0]
	if merged.MemberFoldCaliber != RootCauseMemberFoldCaliberCountSum {
		t.Fatalf("count additivity must ride the count_sum caliber, got %q", merged.MemberFoldCaliber)
	}
	// 恒等式 (Σ计入 == V == 引擎发布值): every value channel publishes the ONE
	// capped value — 35.000 (100ms window × 0.35 cap).
	if !near(merged.ImpactMs, 35.0, 0.001) {
		t.Fatalf("published value must be the §7.5-capped magnitude, got %.3f", merged.ImpactMs)
	}
	if merged.CumulativeImpactMs != merged.ImpactMs || merged.EffectiveImpactMs != merged.ImpactMs {
		t.Fatalf("G3 identity: cumulative/effective must equal the published value, got cum=%.3f eff=%.3f pub=%.3f",
			merged.CumulativeImpactMs, merged.EffectiveImpactMs, merged.ImpactMs)
	}
	// The raw count-equivalent Σ survives losslessly on the disclosure channel.
	if !near(merged.MemberSumMs, 198.3, 0.001) {
		t.Fatalf("the raw count-equivalent Σ must stay disclosed on member_sum, got %.3f", merged.MemberSumMs)
	}
	// normalize must not resurrect the raw Σ onto the effective face.
	normalizeRootCauseEffectiveImpact(out)
	if out[0].EffectiveImpactMs != out[0].ImpactMs {
		t.Fatalf("normalize resurrected the raw Σ: eff=%.3f pub=%.3f", out[0].EffectiveImpactMs, out[0].ImpactMs)
	}
	// Both prose faces wear the count-equivalent marker — never a bare
	// wall-clock "ms" on a count-derived scalar (两面同源: one helper).
	// §29.55 观察③ 两形一裁 (2026-07-14): the marker form is suffix-free —
	// 计数当量 X(非墙钟), never 计数当量Xms (双复核 件13 定稿空格).
	if !strings.Contains(merged.Summary, "combined=计数当量 35.000(非墙钟) (count_sum)") ||
		!strings.Contains(merged.Summary, "member_sum=计数当量 198.300(非墙钟)") {
		t.Fatalf("summary must print the published value + raw Σ through the count-equivalent marker, got %q", merged.Summary)
	}
	for _, entry := range merged.MemberRoster {
		if !strings.Contains(entry, "计数当量") {
			t.Fatalf("roster entries must wear the count-equivalent marker, got %v", merged.MemberRoster)
		}
	}
}

func TestCountFamilyWallClockArmUnchangedG3(t *testing.T) {
	// Negative control (arm-level scope guard): a wall-clock disjoint family
	// keeps its legacy faces — Σ published uncapped-consistent, bare ms
	// wording, no member_sum disclosure, no count marker.
	q := countFamilyFixtureQuery()
	mk := func(startTs, endTs float64, ms float64, line int) RootCauseRankItem {
		item := rootCauseItem("io_latency", ThreadRef{Comm: "io", PID: 90}, ms, 0.86, line, line+1, "window_stats", "block IO")
		item.CumulativeImpactMs = ms
		item.Causality = "background"
		item.ChainRelevance = "background"
		item.StartTs = startTs
		item.EndTs = endTs
		item.MemberKey = "dev=253,0"
		return item
	}
	out := foldSameThreadTypeRankFamilies(q, true, []RootCauseRankItem{
		mk(10.010, 10.011, 1.0, 100),
		mk(10.020, 10.0205, 0.5, 200),
	})
	if len(out) != 1 || out[0].MemberFoldCaliber != RootCauseMemberFoldCaliberSumDisjoint {
		t.Fatalf("disjoint wall-clock family must keep sum_disjoint, got %+v", out)
	}
	if strings.Contains(out[0].Summary, "计数当量") {
		t.Fatalf("wall-clock families must never wear the count marker, got %q", out[0].Summary)
	}
	if out[0].MemberSumMs != 0 || !near(out[0].CumulativeImpactMs, 1.5, 0.001) {
		t.Fatalf("legacy disjoint faces must stay byte-stable, got %+v", out[0])
	}
}

func TestCountSingleRowNormalizeEffectiveUsesPublishedG3(t *testing.T) {
	// The single-row half of the same identity: an UNFOLDED count row's
	// effective face is the published (capped) value, never the raw
	// count-equivalent cumulative the generic fallback would pick up.
	q := countFamilyFixtureQuery()
	items := []RootCauseRankItem{countFamilyMember("0x1", 444, 10.010, 10.020, 100, 110)}
	if !near(items[0].ImpactMs, 35.0, 0.001) || !near(items[0].CumulativeImpactMs, 133.2, 0.001) {
		t.Fatalf("fixture drifted: want capped 35.0 / raw 133.2, got %+v", items[0])
	}
	_ = q
	normalizeRootCauseEffectiveImpact(items)
	if items[0].EffectiveImpactMs != items[0].ImpactMs {
		t.Fatalf("single count row effective must be the published value, got eff=%.3f pub=%.3f",
			items[0].EffectiveImpactMs, items[0].ImpactMs)
	}
	// The raw disclosure channel is untouched (lossless).
	if !near(items[0].CumulativeImpactMs, 133.2, 0.001) {
		t.Fatalf("the raw count-equivalent cumulative must survive as disclosure, got %+v", items[0])
	}
}

// --- G10: the withdrawal witness is Chinese ---------------------------------------

func TestLockHolderSelfContradictionWitnessIsChineseG10(t *testing.T) {
	rows := collectBlockingSpanRows(opendirChimeraIndex(t), Query{}, opendirChimeraStats())
	var lego *blockingSpanRow
	for i := range rows {
		if rows[i].cand.Thread.PID == 16865 {
			lego = &rows[i]
		}
	}
	if lego == nil || lego.cand.HolderSelfContradiction == "" {
		t.Fatalf("fixture drifted: expected the withdrawn LegoHandler row, got %+v", rows)
	}
	witness := lego.cand.HolderSelfContradiction
	// §22.2.1 词条尺子: the witness is Chinese; payload/tid stay untranslated
	// and the number / line formats are byte-preserved (%.3fms, 行 %d-%d).
	if !strings.HasPrefix(witness, "推断持有者 ugc.aweme.lite-16547 自身在同一 payload 持有者 tid 42067 上排队 ") {
		t.Fatalf("witness must open with the zh inferred-holder form, got %q", witness)
	}
	if !strings.Contains(witness, "ms(本段共 115.944ms;行 45696-79136)") {
		t.Fatalf("witness must keep the span/line disclosure formats, got %q", witness)
	}
	if strings.Contains(witness, "inferred holder") || strings.Contains(witness, "itself waited") {
		t.Fatalf("the EN sentence must not survive on the zh-bound witness, got %q", witness)
	}
}
