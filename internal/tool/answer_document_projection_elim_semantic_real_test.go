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

func elimSemanticRealFence(t *testing.T, trace string, pid int, start, end float64) string {
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
	md := render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh")
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
	semInTop5 := false
	for i, line := range members {
		if strings.Contains(line, "类校验") {
			semLines++
			if i < runtimeTraceProjElimTopN {
				semInTop5 = true
			}
		}
	}
	// EVOLUTION RECORD (ELIM-SELF-FIX 件1, 2026-07-15): the self running
	// fold seat (0.288ms) takes the boundary slot — the semantic member now
	// rides the chain-tail FALLBACK exactly once (the live trigger form).
	if semLines != 1 || semInTop5 {
		t.Fatalf("boundary form: the semantic member renders exactly once via the fallback append (got %d, inTop5=%v):\n%s", semLines, semInTop5, elim)
	}
	// EVOLUTION RECORD (XCPU §29.104.5, 2026-07-15): the wakeup-delay
	// segments' switch-in CPU stamp unlocked the CPU-specific lanes on this
	// window — the generic 0.288ms 「running ·折算」 self seat refined into
	// the frequency-evidenced 低频运行 seat (runnable account conserved:
	// 1.871+0.298 = 1.347+0.822 = 2.169ms across the re-bucketed members).
	// The compute-supply self seat itself must still hold a board slot.
	if !strings.Contains(elim, "低频运行") || !strings.Contains(elim, "自身·墙钟席") {
		t.Fatalf("the self compute-supply seat must hold its board slot on the boundary window:\n%s", elim)
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
	semInTop5 := false
	for i, line := range members {
		if strings.Contains(line, "类校验") {
			semLines++
			if i < runtimeTraceProjElimTopN {
				semInTop5 = true
			}
		}
	}
	// EVOLUTION RECORD (ELIM-SELF-FIX 件1, 2026-07-15): the flagship-bounds
	// window mints the self running fold seat at 9.148ms (board #2) — the
	// semantic member moves to the chain-tail fallback append, exactly once.
	if semLines != 1 || semInTop5 {
		t.Fatalf("the flagship-bounds window renders the semantic member exactly once via the fallback append (got %d, inTop5=%v):\n%s", semLines, semInTop5, elim)
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
	wantSubstrings := map[string]bool{
		"9.365ms": false, // self running fold deficit (hand-verified)
		"running": false,
		"iowait":  false, // self io family
	}
	for _, line := range members {
		if !strings.Contains(line, "com.baidu.tieba-59566") {
			continue
		}
		if !strings.Contains(line, "⛓ 链上") {
			t.Fatalf("R8: a self board line must wear the chain channel:\n%s", line)
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
	// the inversion typing; the family PRESENCE is the invariant).
	runnableSeen := false
	for _, line := range members {
		if strings.Contains(line, "com.baidu.tieba-59566") &&
			(strings.Contains(line, "runnable") || strings.Contains(line, "可运行等待")) {
			runnableSeen = true
		}
	}
	if !runnableSeen {
		t.Fatalf("全族在榜: the self runnable family must hold a board seat:\n%s", elim)
	}
	// The self running fold seat wears the fold + self wording.
	for _, line := range members {
		if strings.Contains(line, "9.365ms") {
			if !strings.Contains(line, "折算") || !strings.Contains(line, "自身·墙钟席") {
				t.Fatalf("the self running fold seat must wear 折算 + 自身·墙钟席:\n%s", line)
			}
		}
	}
}
