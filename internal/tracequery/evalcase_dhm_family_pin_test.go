package tracequery

// evalcase_dhm_family_pin_test.go — DHMINE-1 batch (2026-07-20, user ruling
// 「可以尝试切换不同的线程,窗口等,可以用来构造一些场景代替对用户回访的采集吗」):
// disease-family engine pins on UNUSED (target, window) combos of the two
// committed real traces, substituting the roster-A 修后复放 items' FAMILY
// FORMS locally (docs/design/customer_revisit_roster_20260717.md; the
// original-frame replays themselves stay external where the artifact is
// customer-only — 边界诚实 per the DHMINE-1 spec).
//
// Mining ledger: scratchpad ledger §29.172 收账节与 roster DHMINE-1 盘点块。
// board_*.json dumps). Every number below is a measured pin re-collected at
// HEAD ≥bdb2fa4bc and cross-checked against a second source in-case
// (subtotal reproduction, raw-event tallies, µs identities) — dual-source
// discipline (§29.137 EVALCASE-DH precedent). Engine untouched (零引擎改动).
//
// Cases:
//
//	DHM-A1a  roster A1 (XLANE 族) two-ruler accounting on a NEW target —
//	         tieba 59843 full window.
//	DHM-A1b  roster A1 board-identity params component — donghu 17267 full
//	         window, two knob sets → two ordinal domains; conservation
//	         detector quiet on both.
//	DHM-A2a  roster A2 (ELIM 同帧多席形) non-target seat published at top
//	         ordinal — tieba 61839 full window.
//	DHM-A2b  roster A2 capacity-displacement per-channel disclosure with
//	         top-2 naming — donghu 24711 full window.
//	DHM-A3a  roster A3/A5 inversion-seat pair (both registry tokens, one
//	         thread, displacement ≤ envelope) — tieba 59566 EARLY window.
//	DHM-A5a  roster A5 M18 composite-score wire fork live witness + inversion
//	         census — donghu 2179 full window.
//	DHM-A7a  roster A7 (对比形) cross-trace window-length normalization
//	         ordering flip on a NEW pair — donghu 24711 × tieba 60555.
//	DHM-A7b  roster A7 same-trace dual-window: window component separates
//	         board identity; values never travel across windows — tieba
//	         59566 early/late.
//
// Fixture red line: the traces are REAL captures — every number below is a
// measured pin, never an edit target.

import (
	"encoding/json"
	"strings"
	"testing"
)

func evalcaseDHMFindItem(items []RootCauseRankItem, typ string, pid int) *RootCauseRankItem {
	for i := range items {
		if items[i].Type == typ && items[i].Thread.PID == pid {
			return &items[i]
		}
	}
	return nil
}

// DHM-A1a — roster A1 (XLANE 全族修后), acceptance sentence (verbatim):
// 「链上 runnable 族 Σ ≤ 该线程全窗 runnable(修前 E11 23.471+E26 17.635=41.106
// > 全窗 26.725 的 1.5×;修后恰一全额席,其余席互指降道)。」
// Family form on a NEW combo (tieba CookieMonsterCl-59843, full window
// 34579.450627..34579.595184): the target's own runnable seats split across
// the TWO closed rulers with per-ruler subtotals that reproduce µs-exactly
// from the published members, NO cross-ruler total field exists (M3 禁混尺),
// and the wall-ruler Σ stays ≤ the thread's whole-window runnable account —
// the double-count disease shape (41.106 > 26.725×1.5) is structurally
// impossible on this wire.
func TestEvalcaseDHMA1aTwoRulerRunnableConservation(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseTiebaFixture)
	q := Query{PID: 59843, TimeStart: 34579.450627, TimeEnd: 34579.595184,
		TraceFlavorHint: TraceFlavorHarmonyHitrace}
	rank := BuildRootCauseRank(idx, q)
	tr := rank.SelfRunnableTwoRuler
	if tr == nil {
		t.Fatal("DHM-A1a: SelfRunnableTwoRuler record missing (mined live form)")
	}
	if tr.Thread.PID != 59843 {
		t.Fatalf("DHM-A1a: record thread drifted: %+v", tr.Thread)
	}
	// Measured pins (probe board_tb_full_59843.json): wall seats rank4=8.307 +
	// rank12=0.399, subtotal 8.706; edge seat rank7=4.067, subtotal 4.067.
	if len(tr.WallSeats) != 2 || len(tr.EdgeSeats) != 1 {
		t.Fatalf("DHM-A1a: ruler membership drifted: wall=%d edge=%d", len(tr.WallSeats), len(tr.EdgeSeats))
	}
	if tr.WallSeats[0].Rank != 4 || !near(tr.WallSeats[0].EffMs, 8.307, 0.001) ||
		tr.WallSeats[1].Rank != 12 || !near(tr.WallSeats[1].EffMs, 0.399, 0.001) {
		t.Fatalf("DHM-A1a: wall seats drifted: %+v", tr.WallSeats)
	}
	if tr.EdgeSeats[0].Rank != 7 || !near(tr.EdgeSeats[0].EffMs, 4.067, 0.001) {
		t.Fatalf("DHM-A1a: edge seat drifted: %+v", tr.EdgeSeats)
	}
	// Dual-source arm: subtotals reproduce from the published members (µs).
	if !near(tr.WallSubtotalMs, tr.WallSeats[0].EffMs+tr.WallSeats[1].EffMs, 0.0011) {
		t.Fatalf("DHM-A1a: wall subtotal %.6f does not reproduce from members", tr.WallSubtotalMs)
	}
	if !near(tr.EdgeSubtotalMs, tr.EdgeSeats[0].EffMs, 0.0011) {
		t.Fatalf("DHM-A1a: edge subtotal %.6f does not reproduce from members", tr.EdgeSubtotalMs)
	}
	// Conservation arm (the acceptance sentence's Σ ≤ 全窗 runnable): the
	// wall-clock ruler's Σ must sit inside the thread's own four-state
	// runnable account for the same window (production Result-level account).
	res := Run(idx, q)
	if res.TargetWindowStates == nil {
		t.Fatal("DHM-A1a: target window state account missing")
	}
	if tr.WallSubtotalMs > res.TargetWindowStates.RunnableMs+0.001 {
		t.Fatalf("DHM-A1a: wall-ruler Σ %.3f exceeds whole-window runnable %.3f (the disease shape)",
			tr.WallSubtotalMs, res.TargetWindowStates.RunnableMs)
	}
}

// DHM-A1b — roster A1, acceptance sentences (verbatim):
// 「榜位撞号=0:两板 #1 各佩各「·板锚」chip,不再出现裸 chip「根因排序#1」×2。」
// 「跨板 Σ 病句消失:不再出现「各根因席位有效归因合计 355.562ms 超过窗长
// 233.190ms」类两板混加;各板 Σ 各<窗。」
// Family form (engine half of the XLANE-3 板身份三元组): the SAME window +
// SAME target queried under two knob sets mints two boards whose typed
// BoardParamsFingerprint differ — two genuine ordinal domains (the display
// 板锚 chip consumes exactly this typed input) — while the AXIOM-V2
// per-(thread,direction) conservation detector stays quiet on BOTH boards
// (no seat carries DirectionConservationExcess: the 两板混加 Σ disease has no
// typed foothold).
func TestEvalcaseDHMA1bBoardParamsFingerprintSeparation(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseDonghuFixture)
	base := Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		TraceFlavorHint: TraceFlavorHarmonyHitrace}
	variant := base
	variant.MaxDepth = 4
	variant.MinDurationMs = 0.5
	variant.Limit = 12
	boardA := BuildRootCauseRank(idx, base)
	boardB := BuildRootCauseRank(idx, variant)
	if boardA.BoardParamsFingerprint == boardB.BoardParamsFingerprint {
		t.Fatalf("DHM-A1b: two knob sets collapsed to one ordinal domain: %q", boardA.BoardParamsFingerprint)
	}
	// Measured pins (probe): default knobs → d60173d1, variant → 8351d844.
	if boardA.BoardParamsFingerprint != "d60173d1" || boardB.BoardParamsFingerprint != "8351d844" {
		t.Fatalf("DHM-A1b: fingerprints drifted: %q / %q",
			boardA.BoardParamsFingerprint, boardB.BoardParamsFingerprint)
	}
	for name, board := range map[string]RootCauseRankResult{"default": boardA, "variant": boardB} {
		for _, it := range board.Items {
			if it.DirectionConservationExcess != nil {
				t.Fatalf("DHM-A1b: %s board seat %s/%d carries a conservation violation: %+v",
					name, it.Type, it.Thread.PID, it.DirectionConservationExcess)
			}
		}
	}
}

// DHM-A2a — roster A2 (ELIM-GAP+GATED-CAL 修后), acceptance sentence
// (verbatim): 「根因排序#2=[GT]ColdPool#9-48667 runnable 8.211ms 出现在 ◎ 板
// (应列第二 bar;修前完全缺席零披露)。」
// Family form on a NEW combo (tieba T7@ZeusThreadPo-61839, full window): a
// NON-target thread's runnable seat is PUBLISHED with a top candidate ordinal
// (Rank #1, Binder:43397_19-23088, 13.898ms) instead of silently vanishing —
// the exact population arm (Node.Rank>0) the ◎ 总览 board consumes — while
// the target's own running seat rides the self lane beside it (basis
// self_wall_clock_interval, capability-folded eff).
func TestEvalcaseDHMA2aNonTargetSeatPublishedTopOrdinal(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseTiebaFixture)
	rank := BuildRootCauseRank(idx, Query{PID: 61839, TimeStart: 34579.450627, TimeEnd: 34579.595184,
		TraceFlavorHint: TraceFlavorHarmonyHitrace})
	seat := evalcaseDHMFindItem(rank.Items, "runnable_wait", 23088)
	if seat == nil {
		t.Fatal("DHM-A2a: non-target runnable seat 23088 missing (修前病形=完全缺席零披露)")
	}
	if seat.Rank != 1 || seat.Thread.Comm != "Binder:43397_19" {
		t.Fatalf("DHM-A2a: seat identity drifted: rank=%d comm=%q", seat.Rank, seat.Thread.Comm)
	}
	if !near(seat.ImpactMs, 13.898, 0.001) || !near(seat.EffectiveImpactMs, 13.898, 0.001) {
		t.Fatalf("DHM-A2a: seat value drifted: impact=%.3f eff=%.3f", seat.ImpactMs, seat.EffectiveImpactMs)
	}
	self := evalcaseDHMFindItem(rank.Items, "running", 61839)
	if self == nil || self.Rank != 2 || self.OnChainBasis != RootCauseOnChainBasisSelfWallClockInterval {
		t.Fatalf("DHM-A2a: target self running seat drifted: %+v", self)
	}
	if !near(self.ImpactMs, 30.287, 0.001) || !near(self.EffectiveImpactMs, 9.148, 0.001) {
		t.Fatalf("DHM-A2a: self seat values drifted: impact=%.3f eff=%.3f", self.ImpactMs, self.EffectiveImpactMs)
	}
}

// DHM-A2b — roster A2, acceptance sentence (verbatim): 「尾注不再只数语义行:
// 非语义持榜席值切有「另有 N 行未入榜(TOP5 值切),见明细」逐通道披露。」
// Family form on a NEW combo (donghu HiPlayer_79_DEM-24711, full window): the
// capacity face discloses the non-entering valued rows PER CHANNEL with the
// POOL2 top-2 naming (帽亡每道 top-2, §29.162④) instead of silently counting
// only semantic rows — the typed caveat carries the per-channel census
// (链上/邻近/背景) and names the largest + second-largest displaced rows.
func TestEvalcaseDHMA2bCapacityDisplacementPerChannelDisclosure(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseDonghuFixture)
	rank := BuildRootCauseRank(idx, Query{PID: 24711, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		TraceFlavorHint: TraceFlavorHarmonyHitrace})
	var hit string
	for _, cv := range rank.Caveats {
		if strings.Contains(cv, "未入发布面(链上 ") && strings.Contains(cv, "/邻近 ") && strings.Contains(cv, "/背景 ") {
			hit = cv
			break
		}
	}
	if hit == "" {
		t.Fatalf("DHM-A2b: per-channel displacement disclosure missing from caveats: %d caveat(s)", len(rank.Caveats))
	}
	for _, frag := range []string{
		"最大 page_cache_churn .ugc.aweme.lite-17267 81.616ms(背景)",
		"次大 page_cache_churn sysmgr-reclaim0-9 81.616ms",
	} {
		if !strings.Contains(hit, frag) {
			t.Fatalf("DHM-A2b: top-2 naming fragment %q missing from disclosure:\n%s", frag, hit)
		}
	}
}

// DHM-A3a — roster A3/A5 (RANKDIS 词族四批修后), acceptance sentence
// (verbatim, A5 ④): 「transcript 不再出现「rank 1-8 全是与目标无关的 s_sleep
// 线程」类误读(状态手递面已改 drill_rank);无双 Rank#1 reconcile 循环;正文无
// 「排名不一致」自我调和段。」
// Deterministic family half on a NEW combo (tieba 59566, UNUSED early window
// 34579.450627..34579.520000): the inversion-rich board publishes BOTH
// registry inversion tokens for ONE thread (CookieMonsterCl-59843) with the
// displacement discipline eff ≤ impact (位移量测,永不冒充包络), and both
// tokens resolve the SAME registry fix direction (反转席三面同词 feeds from
// this single declaration). The transcript-facing drill_rank fork itself is
// LLM-surface and stays with the A5 replay.
func TestEvalcaseDHMA3aInversionSeatPairEarlyWindow(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseTiebaFixture)
	rank := BuildRootCauseRank(idx, Query{PID: 59566, TimeStart: 34579.450627, TimeEnd: 34579.520000,
		TraceFlavorHint: TraceFlavorHarmonyHitrace})
	wait := evalcaseDHMFindItem(rank.Items, "priority_inversion_runnable_wait", 59843)
	cand := evalcaseDHMFindItem(rank.Items, "priority_inversion_candidate", 59843)
	if wait == nil || cand == nil {
		t.Fatalf("DHM-A3a: inversion pair missing: wait=%v cand=%v", wait != nil, cand != nil)
	}
	if !near(wait.ImpactMs, 19.372, 0.001) || !near(wait.EffectiveImpactMs, 1.847, 0.001) {
		t.Fatalf("DHM-A3a: runnable-wait inversion values drifted: %.3f/%.3f", wait.ImpactMs, wait.EffectiveImpactMs)
	}
	if !near(cand.ImpactMs, 12.128, 0.001) {
		t.Fatalf("DHM-A3a: candidate inversion value drifted: %.3f", cand.ImpactMs)
	}
	if wait.EffectiveImpactMs > wait.ImpactMs {
		t.Fatalf("DHM-A3a: displacement eff %.3f exceeds envelope %.3f", wait.EffectiveImpactMs, wait.ImpactMs)
	}
	for _, tok := range []string{"priority_inversion_runnable_wait", "priority_inversion_candidate"} {
		if dir := CausalTokenFixDirectionFor(tok); dir != CausalFixDirectionLockPriority {
			t.Fatalf("DHM-A3a: %s fix direction drifted: %q", tok, dir)
		}
	}
}

// DHM-A5a — roster A5, acceptance sentence (verbatim): 「复合分数值佩
// composite score 词,不再以 ms 假墙钟出现在正文。」
// Family form on a NEW combo (donghu VSyncGenerator-2179, full window — the
// richest local inversion window, 14 inversion seats): the composite
// block_io_by_inode row's wire payload publishes its magnitude under the
// *_score keys and SUPPRESSES the ms-semantic keys (M18 wire fork,
// CausalTokenCompositeValueWire — the 146ms 窗发布 impact=635.077ms 假墙钟
// disease family has no wire foothold), while the wall-clock inversion seats
// keep their ms keys untouched.
func TestEvalcaseDHMA5aCompositeScoreWireForkLive(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseDonghuFixture)
	rank := BuildRootCauseRank(idx, Query{PID: 2179, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		TraceFlavorHint: TraceFlavorHarmonyHitrace})
	var candidates, waits int
	for _, it := range rank.Items {
		switch it.Type {
		case "priority_inversion_candidate":
			candidates++
		case "priority_inversion_runnable_wait":
			waits++
		}
	}
	// NIAUTH-04 (2026-07-26): the full CPU-constraint census admits one
	// precisely proven constraint contender that used to die behind the
	// display Top-8. The fixed board capacity therefore retains 7 candidate
	// + 6 wait inversion rows instead of the former 7+7 publication. This is
	// a publication pin; the test's composite-score subject stays present
	// and is checked below.
	if candidates != 7 || waits != 6 {
		t.Fatalf("DHM-A5a: published inversion roster drifted: candidates=%d waits=%d (want 7/6)", candidates, waits)
	}
	for _, item := range rank.Items {
		if item.Type == "block_io_by_inode" && item.Thread.PID == 17267 && rootCauseItemIsOnChain(item) {
			t.Fatalf("DHM-A5a: target identity must not authorize inode/block relation: %+v", item)
		}
	}
	if !CausalTokenCompositeValueWire("block_io_by_inode") {
		t.Fatal("DHM-A5a: registry lost the composite wire arm for block_io_by_inode")
	}
	// Keep the generic wire contract independently pinned. The live trace is
	// no longer used to fixture a relation that its producer cannot prove.
	comp := RootCauseRankItem{Type: "block_io_by_inode", Thread: ThreadRef{PID: 17267}, ImpactMs: 2.694, CumulativeImpactMs: 2.694}
	blob, err := json.Marshal(comp)
	if err != nil {
		t.Fatal(err)
	}
	wire := string(blob)
	if !strings.Contains(wire, `"impact_score":`) {
		t.Fatalf("DHM-A5a: composite wire lost the impact_score key:\n%s", wire)
	}
	if strings.Contains(wire, `"impact_ms":`) || strings.Contains(wire, `"effective_impact_ms":`) {
		t.Fatalf("DHM-A5a: composite wire leaked an ms-semantic key (假墙钟形):\n%s", wire)
	}
	// Control arm: a wall-clock inversion seat keeps its ms keys.
	inv := evalcaseDHMFindItem(rank.Items, "priority_inversion_runnable_wait", 19050)
	if inv == nil {
		t.Fatal("DHM-A5a: control inversion seat 19050 missing")
	}
	invBlob, err := json.Marshal(*inv)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(invBlob), `"impact_ms":`) {
		t.Fatalf("DHM-A5a: wall-clock seat lost its ms key:\n%s", string(invBlob))
	}
}

// DHM-A7a — roster A7 (对比/runnable/data 三场景伞项), acceptance sentence
// (verbatim): 「对比报告 per-工件双投影+对比总览表恒在场(≥2 已编译投影即出,
// 不再吃 LLM 分类方差);回探指令不再点名「Missing repo_map lenses」类结构性
// 不可满足项;data 场景 15/15 route=data 级稳定、无 46min 死等。」
// Deterministic family half on a NEW cross-trace pair (donghu
// HiPlayer_79_DEM-24711 × tieba ThreadPoolForeg-60555): the per-side typed
// truths a cross-trace narrative must consume — each side's OWN window wall
// as the density denominator. The raw wakeup census ORDERS ONE WAY (donghu
// 34 > tieba 31) and INVERTS under window-length normalization (0.146/ms <
// 0.214/ms) — the CMP-A 窗长差假象 family (客户 2.18× lesson) live on a new
// pair. The 对比总览表 surface itself is LLM-side and stays with the A7
// replay.
func TestEvalcaseDHMA7aCrossTraceWakeupDensityInversion(t *testing.T) {
	donghu := evalcaseIndex(t, evalcaseDonghuFixture)
	tieba := evalcaseIndex(t, evalcaseTiebaFixture)
	countWakeups := func(idx *Index, pid int) int {
		n := 0
		for _, ev := range idx.Events {
			if (ev.Type == EventSchedWakeup || ev.Type == EventSchedWaking) && ev.WakeePID == pid {
				n++
			}
		}
		return n
	}
	donghuWakeups := countWakeups(donghu, 24711)
	tiebaWakeups := countWakeups(tieba, 60555)
	if donghuWakeups != 34 || tiebaWakeups != 31 {
		t.Fatalf("DHM-A7a: raw wakeup census drifted: donghu=%d tieba=%d (want 34/31)", donghuWakeups, tiebaWakeups)
	}
	donghuSpanMs := (donghu.LastTs - donghu.FirstTs) * 1000
	tiebaSpanMs := (tieba.LastTs - tieba.FirstTs) * 1000
	if !near(donghuSpanMs, 233.190, 0.001) || !near(tiebaSpanMs, 144.557, 0.001) {
		t.Fatalf("DHM-A7a: window walls drifted: %.3f / %.3f", donghuSpanMs, tiebaSpanMs)
	}
	// Raw ordering: donghu side larger.
	if !(donghuWakeups > tiebaWakeups) {
		t.Fatalf("DHM-A7a: raw ordering lost: %d vs %d", donghuWakeups, tiebaWakeups)
	}
	// Density ordering INVERTS once each side divides by its own window wall.
	donghuDensity := float64(donghuWakeups) / donghuSpanMs
	tiebaDensity := float64(tiebaWakeups) / tiebaSpanMs
	if !(donghuDensity < tiebaDensity) {
		t.Fatalf("DHM-A7a: density inversion lost: %.4f vs %.4f", donghuDensity, tiebaDensity)
	}
	if !near(donghuDensity, 0.1458, 0.0005) || !near(tiebaDensity, 0.2144, 0.0005) {
		t.Fatalf("DHM-A7a: densities drifted: %.4f / %.4f", donghuDensity, tiebaDensity)
	}
}

// DHM-A7b — roster A7, same acceptance sentence as DHM-A7a (同 trace 双窗对比
// 形 per the DHMINE-1 spec A7 family: donghu×tieba 跨 trace + 同 trace 双窗).
// Family form (tieba 59566, early 34579.450627..34579.520000 vs late
// 34579.520000..34579.595184): the SAME target under the SAME knobs mints two
// boards whose identity separates on the WINDOW component of the 板身份三元组
// (fingerprints identical — params half equal by design), and a thread's
// published account is WINDOW-SCOPED: CookieMonsterCl-59843 publishes a
// 19.372ms inversion envelope in the early window and a 0.689ms runnable seat
// in the late window — no value travels across the window boundary.
func TestEvalcaseDHMA7bSameTraceDualWindowScoping(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseTiebaFixture)
	early := BuildRootCauseRank(idx, Query{PID: 59566, TimeStart: 34579.450627, TimeEnd: 34579.520000,
		TraceFlavorHint: TraceFlavorHarmonyHitrace})
	late := BuildRootCauseRank(idx, Query{PID: 59566, TimeStart: 34579.520000, TimeEnd: 34579.595184,
		TraceFlavorHint: TraceFlavorHarmonyHitrace})
	if early.BoardParamsFingerprint != "d60173d1" || late.BoardParamsFingerprint != "d60173d1" {
		t.Fatalf("DHM-A7b: params fingerprints drifted: %q / %q (window component must carry the separation)",
			early.BoardParamsFingerprint, late.BoardParamsFingerprint)
	}
	if early.Window == late.Window {
		t.Fatal("DHM-A7b: the two boards lost their window separation")
	}
	earlyInv := evalcaseDHMFindItem(early.Items, "priority_inversion_runnable_wait", 59843)
	if earlyInv == nil || !near(earlyInv.ImpactMs, 19.372, 0.001) {
		t.Fatalf("DHM-A7b: early-window inversion seat drifted: %+v", earlyInv)
	}
	lateRun := evalcaseDHMFindItem(late.Items, "runnable_wait", 59843)
	if lateRun == nil || lateRun.Rank != 1 || !near(lateRun.ImpactMs, 0.689, 0.001) {
		t.Fatalf("DHM-A7b: late-window runnable seat drifted: %+v", lateRun)
	}
	// The early-window envelope value appears NOWHERE on the late board for
	// this thread (window-scoped accounting, no cross-window travel).
	for _, it := range late.Items {
		if it.Thread.PID == 59843 && near(it.ImpactMs, 19.372, 0.001) {
			t.Fatalf("DHM-A7b: early-window value travelled onto the late board: %s %.3f", it.Type, it.ImpactMs)
		}
	}
}
