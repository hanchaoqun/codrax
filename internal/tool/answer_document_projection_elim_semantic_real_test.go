package tool

// answer_document_projection_elim_semantic_real_test.go — RNB-2 件4 acceptance
// windows on ENGINE-REAL records (§29.53 产线实铸形 red line; §29.88.8 SCAN-3
// 语义/R3 锚点; zero LLM, zero dispatch variance):
//
//	W-A tieba 61839 窗 34579.470..34579.520 — the seated VerifyClass member
//	    sits exactly AT the TOP5 boundary (#5) → exactly ONE semantic member
//	    line, no fallback double-render;
//	W-B tieba 61839 旗舰窗界 34579.450627..34579.522905 — same boundary form
//	    on the flagship bounds;
//	W-C donghu 17284 窗 13762.845..13762.900 — SELF-SEM jit 族折叠席 rank#2
//	    (§29.88.8 ③, 2.388=1.781+0.607) → high-seat negative control.
//
// 如实注 (anchor drift, 2026-07-15): the §29.88.8 SCAN-3 anchor predicted the
// VerifyClass seat CUT at rank#8 in W-A (切点#5=1.338) — that scan predates
// RNB-1, whose R4/R2 lane demotions moved the outranking seats to ◇ and the
// semantic seat then landed exactly AT #5 in every window containing the span
// (probed 34579.428..34579.530 / .470...520 / flagship bounds — chain block
// stable at 5).
//
// EVOLUTION RECORD (ELIM-SELF-FIX 件1 §29.93.1, 2026-07-15): the target's own
// running supply-fold deficit seat now mints on both tieba windows (0.288ms
// boundary / 9.148ms flagship bounds, freq_only basis) and takes a TOP5 slot,
// pushing the VerifyClass member past the cut — the natural fallback TRIGGER
// form finally has a LIVE in-repo witness: both windows must render the
// semantic member exactly ONCE via the chain-tail fallback append (no TOP5
// seat, no double render). The synthetic pins (answer_document_projection_
// elim_semantic_test.go a/c) keep the unit forms.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// elimSemanticRealMarkdown renders the FULL zh answer markdown over the
// engine-real window (◎ fence + tree + detail faces — the ELIM-V2 修补轮 件1
// pin reads both the ◎ section attribution and the tree-face 修向 word).
func elimSemanticRealMarkdown(t *testing.T, trace string, pid int, start, end float64) string {
	t.Helper()
	idx, err := tracequery.BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	query := tracequery.Query{PID: pid, TimeStart: start, TimeEnd: end,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 12}
	at := time.Unix(1751600000, 0).UTC()
	var obs []types.ObservationRecord
	for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
		q := query
		q.View = view
		result := tracequery.Run(idx, q)
		obs = append(obs, traceQueryTypedObservations(result, "fixture", "p-"+view, "r", "", at)...)
	}
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck,
	}}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "RNB-2 件4 witness。"}}}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("apply: %v %s", err, res.Summary)
	}
	return render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh")
}

func elimSemanticRealFence(t *testing.T, trace string, pid int, start, end float64) string {
	t.Helper()
	md := elimSemanticRealMarkdown(t, trace, pid, start, end)
	elimAt := strings.Index(md, tracefence.ElimOpener)
	if elimAt < 0 {
		t.Fatalf("the ◎ overview must render:\n%s", md)
	}
	elim := md[elimAt:]
	if end := strings.Index(elim[len(tracefence.ElimOpener):], "```"); end >= 0 {
		elim = elim[:len(tracefence.ElimOpener)+end+3]
	}
	return elim
}

const elimSemanticTiebaTrace = "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace"
const elimSemanticDonghuTrace = "../../eval/fixtures/real_traces/donghu.ftrace"

// W-A: the TOP5-boundary window — the seated semantic member holds #5; the
// fallback must not double-render it (boundary sentinel).
func TestElimSemanticFallbackTiebaBoundaryWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace witness")
	}
	if _, err := os.Stat(elimSemanticTiebaTrace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	elim := elimSemanticRealFence(t, elimSemanticTiebaTrace, 61839, 34579.470, 34579.520)
	members := elimOverviewMemberLines(elim)
	semLines := 0
	chainMembers := len(elimOverviewChainMemberLines(elim))
	for _, line := range members {
		if strings.Contains(line, "类校验") {
			semLines++
		}
	}
	// EVOLUTION RECORD (ELIM-SELF-FIX 件1, 2026-07-15): the self running
	// fold seat (0.288ms) takes the boundary slot — the semantic member now
	// rides the chain-tail FALLBACK exactly once (the live trigger form).
	// ELIM-V2 EVOLUTION RECORD (2026-07-18): the render-position TOP5 proxy
	// retired with the flat layout (sections regroup the rendered order); the
	// fallback provenance now pins as the member COUNT — TOP5 chain slots
	// plus exactly the one appended fallback member.
	if semLines != 1 || chainMembers != runtimeTraceProjElimTopN+1 {
		t.Fatalf("boundary form: the semantic member renders exactly once via the fallback append (sem=%d chain=%d):\n%s", semLines, chainMembers, elim)
	}
	// EVOLUTION RECORD (XCPU §29.104.5, 2026-07-15): the wakeup-delay
	// segments' switch-in CPU stamp unlocked the CPU-specific lanes on this
	// window — the generic 0.288ms 「running ·折算」 self seat refined into
	// the frequency-evidenced 低频运行 seat (runnable account conserved).
	// EVOLUTION RECORD (ELIM-GAP 件A/件B, §29.104.15, 2026-07-16): the fourth
	// 种群臂 carriage (bare Node.Rank — the R1 absorb backfill) covers the
	// large merged rank carriers this window ALWAYS held on the tree face
	// (Binder 13.898 [E#(+2)] / SharedPreferenc 8.049 [E#(+2)]).
	// 勘正 (§29.112 P3①, DISPLAY-HYG 二轮 2026-07-17): the original note
	// over-claimed BOTH as rescued — Binder 13.898 was already board-seated
	// pre-件A through an earlier carriage; the carrier the fourth arm truly
	// rescued on this window is SharedPreferenc 8.049 (the customer E15
	// disease form). The pin below checks both SLOTS (presence), not rescue
	// provenance.
	// The tiny 低频运行 self seat is now legitimately value-cut from TOP5 and
	// COUNTED in the per-channel cut footnote (静默消失=0); the self family
	// still holds board slots (目标自身·墙钟席 seats below the two carriers).
	if !strings.Contains(elim, "13.898ms") || !strings.Contains(elim, "8.049ms") {
		t.Fatalf("the formerly-vanished merged rank carriers must hold their ◎ slots:\n%s", elim)
	}
	// OMGCLEAN-1 件8 (§29.175.1): the ◎ chip shrank to the bare family word
	// (the wall-clock caliber is the board default; the tree 行2 keeps the
	// full qualifier). §29.187① rename: 自身 → 目标自身.
	if !strings.Contains(elim, "·目标自身") {
		t.Fatalf("the self wall-clock family must still hold board slots on the boundary window:\n%s", elim)
	}
	if !strings.Contains(elim, "· 未入榜") {
		t.Fatalf("the value-cut self seats must be counted, never silent (件B):\n%s", elim)
	}
}

// W-B: the boundary control window (semantic seat inside TOP5 — no fallback).
func TestElimSemanticFallbackTiebaControlWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace witness")
	}
	if _, err := os.Stat(elimSemanticTiebaTrace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	elim := elimSemanticRealFence(t, elimSemanticTiebaTrace, 61839, 34579.450627, 34579.522905)
	members := elimOverviewMemberLines(elim)
	semLines := 0
	chainMembers := len(elimOverviewChainMemberLines(elim))
	for _, line := range members {
		if strings.Contains(line, "类校验") {
			semLines++
		}
	}
	// EVOLUTION RECORD (ELIM-SELF-FIX 件1, 2026-07-15): the flagship-bounds
	// window mints the self running fold seat at 9.148ms (board #2) — the
	// semantic member moves to the chain-tail fallback append, exactly once.
	// ELIM-V2 EVOLUTION RECORD (2026-07-18): position proxy → member-count
	// proxy (see the boundary window above).
	if semLines != 1 || chainMembers != runtimeTraceProjElimTopN+1 {
		t.Fatalf("the flagship-bounds window renders the semantic member exactly once via the fallback append (sem=%d chain=%d):\n%s", semLines, chainMembers, elim)
	}
	if !strings.Contains(elim, "9.148ms") {
		t.Fatalf("the self running fold seat (9.148ms) must hold its board slot:\n%s", elim)
	}
}

// W-C: donghu SELF-SEM high seat (negative control for the fallback).
func TestElimSemanticSelfSemDonghuHighSeat(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace witness")
	}
	if _, err := os.Stat(elimSemanticDonghuTrace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	elim := elimSemanticRealFence(t, elimSemanticDonghuTrace, 17284, 13762.845, 13762.900)
	members := elimOverviewMemberLines(elim)
	semLines := 0
	semInTop5 := false
	for i, line := range members {
		if strings.Contains(line, "JIT编译") {
			semLines++
			if i < runtimeTraceProjElimTopN {
				semInTop5 = true
			}
		}
	}
	if semLines != 1 || !semInTop5 {
		t.Fatalf("SELF-SEM jit fold seat must sit inside TOP5 exactly once (got %d, inTop5=%v):\n%s", semLines, semInTop5, elim)
	}
}

// ELIM-SELF-FIX 件3 display half (§29.93.3 全族在榜恒等, 2026-07-15) — the
// DEGENERATE tieba head window (no cross-thread chain; the pre-fix board
// rendered the honest-empty line while the target's own seats died at the
// candidate cap): after Form-1 (self running fold seat) + Form-2 (selfSide
// cap protection) the ◎ board carries the target's WHOLE seat family —
// running (fold deficit, hand-verified 9.365), the runnable family and the
// iowait family — every line on the ⛓ chain channel.
func TestElimSelfDegenerateWindowBoardCarriesSelfFamily(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace witness")
	}
	if _, err := os.Stat(elimSemanticTiebaTrace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	elim := elimSemanticRealFence(t, elimSemanticTiebaTrace, 59566, 34579.450627, 34579.472865)
	if strings.Contains(elim, "无同尺持值行") {
		t.Fatalf("the degenerate window must no longer render the empty board:\n%s", elim)
	}
	members := elimOverviewMemberLines(elim)
	// OMGCLEAN-1 件11 (§29.175.17 判词文法): the ◎ board face speaks the
	// verdict words — running 折算席 → 低频运行 (·折算 on the caliber slot),
	// io_wait → IO阻塞; the raw state words stay on the tree/state faces.
	wantSubstrings := map[string]bool{
		"9.365ms": false, // self running fold deficit (hand-verified)
		"低频运行":    false,
		"IO阻塞":    false, // self io family
	}
	for _, line := range members {
		if !strings.Contains(line, "com.baidu.tieba-59566") {
			continue
		}
		if !(strings.Contains(line, "█") || strings.Contains(line, "░")) {
			t.Fatalf("R8: a self board line must render on the bar grid:\n%s", line)
		}
		for want := range wantSubstrings {
			if strings.Contains(line, want) {
				wantSubstrings[want] = true
			}
		}
	}
	for want, seen := range wantSubstrings {
		if !seen {
			t.Fatalf("全族在榜: the degenerate-window board must carry the self %q face:\n%s", want, elim)
		}
	}
	// The runnable family holds seats too (the exact split may evolve with
	// the inversion typing; the family PRESENCE is the invariant). 件11: the
	// board face word for the runnable family is 调度延迟 (or the inversion
	// compound, which 维持s its own words).
	runnableSeen := false
	for _, line := range members {
		if strings.Contains(line, "com.baidu.tieba-59566") &&
			(strings.Contains(line, "调度延迟") || strings.Contains(line, "可运行等待")) {
			runnableSeen = true
		}
	}
	if !runnableSeen {
		t.Fatalf("全族在榜: the self runnable family must hold a board seat:\n%s", elim)
	}
	// The self running fold seat wears the fold + self wording.
	for _, line := range members {
		if strings.Contains(line, "9.365ms") {
			if !strings.Contains(line, "折算") || !strings.Contains(line, "·目标自身") {
				t.Fatalf("the self running fold seat must wear 折算 + the 目标自身 chip (件8 短形; §29.187① rename):\n%s", line)
			}
		}
	}
}

// ELIM-V2 修补轮 件1 (2026-07-18) — the rankFoldPeers FixDirection adoption's
// ONLY live payload witness, pinned on the ENGINE-REAL flagship board (donghu
// 17267 旗舰窗 13762.791708..13763.024898): the two priority-inversion seats
// (CompThread_0-2955 / JankManager-9655) reach the display as RNB rank-fold
// carriages whose surviving rows arrive direction-BARE — the engine-stamped
// direction rides the folded rank twin and the tree.go attach loop's
// empty-slot backfill is the one point that carries it across. Strip that
// backfill (mutation) and this board silently degrades three faces at once:
// both inversion seats fall into the ◎ 方向未定/复合 tail, the 锁与优先级
// subtotal (12.115ms, 区间互斥) evaporates, and the tree rows lose their
// 「·修向 锁与优先级」 word — with every synthetic pin still green. This pin
// makes the payload point mutation-visible.
func TestElimV2RankFoldDirectionAdoptionDonghuFlagship(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace witness")
	}
	if _, err := os.Stat(elimSemanticDonghuTrace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	md := elimSemanticRealMarkdown(t, elimSemanticDonghuTrace, 17267, 13762.791708, 13763.024898)
	elimAt := strings.Index(md, tracefence.ElimOpener)
	if elimAt < 0 {
		t.Fatalf("the ◎ overview must render:\n%s", md)
	}
	elim := md[elimAt:]
	if end := strings.Index(elim[len(tracefence.ElimOpener):], "```"); end >= 0 {
		elim = elim[:len(tracefence.ElimOpener)+end+3]
	}
	// ◎ face: every inversion seat renders inside the 锁与优先级 section —
	// never the 方向未定/复合 tail (section attribution = the adopted engine
	// direction, 显示侧零词面推断).
	currentHead := ""
	inversionSeats := 0
	subjects := map[string]bool{}
	for _, line := range strings.Split(elim, "\n") {
		if strings.HasPrefix(line, tracefence.ElimSectionGlyph+" ") {
			currentHead = line
			continue
		}
		if strings.HasPrefix(line, "◇ 邻近(") {
			currentHead = "" // the ◇ block is unsectioned
			continue
		}
		if !strings.Contains(line, "优先级反转候选") {
			continue
		}
		inversionSeats++
		if !strings.Contains(currentHead, "锁与优先级") {
			t.Fatalf("件1: an inversion seat must sit under the 锁与优先级 head (got %q):\n%s", currentHead, elim)
		}
		for _, subject := range []string{"CompThread_0-2955", "JankManager-9655"} {
			if strings.Contains(line, subject) {
				subjects[subject] = true
			}
		}
	}
	if inversionSeats != 2 || len(subjects) != 2 {
		t.Fatalf("件1: the flagship board holds exactly the two inversion seats (got %d, subjects %v):\n%s",
			inversionSeats, subjects, elim)
	}
	// The adopted pair publishes the flagship subtotal (7.405 + 4.710 —
	// disjoint typed envelopes; the direction-bare mutation kills this line
	// together with the section attribution).
	if !strings.Contains(elim, "▸ 锁与优先级 · 最大可消 7.405ms · 2席 · 小计 12.115ms(区间互斥)") {
		t.Fatalf("件1: the 锁与优先级 head must publish the flagship subtotal:\n%s", elim)
	}
	// Tree face: the surviving rows wear the adopted 修向 word.
	if !strings.Contains(md, "·修向 锁与优先级") {
		t.Fatalf("件1: the tree face must wear ·修向 锁与优先级:\n%s", md)
	}
}
