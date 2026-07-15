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
// semantic seat now lands exactly AT #5 in every window containing the span
// (probed 34579.428..34579.530 / .470...520 / flagship bounds — chain block
// stable at 5). The natural TRIGGER form therefore has no live witness in the
// two in-repo fixtures (same disposition as SCAN-3 ⑤ 负结果); the fallback
// arm is carried by the synthetic pins (answer_document_projection_elim_
// semantic_test.go a/c) and the concrete customer-case replay awaits the
// customer ftrace (§29.88 立案注).

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
	if semLines != 1 || !semInTop5 {
		t.Fatalf("boundary form: the seated semantic member holds a TOP5 seat exactly once (got %d, inTop5=%v):\n%s", semLines, semInTop5, elim)
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
	if semLines != 1 || !semInTop5 {
		t.Fatalf("the control window seats the semantic member INSIDE TOP5 exactly once (got %d, inTop5=%v):\n%s", semLines, semInTop5, elim)
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
