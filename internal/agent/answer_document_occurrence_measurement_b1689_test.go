package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This is the production path, not a replacement ObservationRecord: query a
// fixed trace, commit a model document through the public tool, then render the
// last-mile supplement. Its occurrence ruler must remain distinct from total.
func TestB1689PublicQueryAnswerOccurrenceMeasurements(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Fatal(err)
	}
	queryBus := &types.BusContext{RepoRoot: filepath.Dir(path), WorkDir: t.TempDir(), Mutable: types.NewMutableState("b1689-query")}
	params, err := json.Marshal(map[string]any{
		"source": "path", "path": path, "view": "frame_root_cause_bundle", "pid": 59566,
		"time_start": 34579.472865, "time_end": 34579.587805,
		"trace_flavor": "harmony_hitrace", "platform": "donghu",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&tool.TraceQuery{}).Execute(queryBus, params)
	if err != nil || !result.Success {
		t.Fatalf("real trace query prerequisite failed: %v %s", err, result.Summary)
	}
	var payloadRef string
	for _, observation := range result.Observations {
		if observation.SourceRef.PayloadRef != "" {
			payloadRef = observation.SourceRef.PayloadRef
			break
		}
	}
	payload, err := os.ReadFile(payloadRef)
	if err != nil {
		t.Fatalf("read producer payload %q: %v", payloadRef, err)
	}
	var produced tracequery.Result
	if err := json.Unmarshal(payload, &produced); err != nil {
		t.Fatal(err)
	}
	if produced.FrameRootCauseBundle == nil || produced.FrameRootCauseBundle.RootCauseRank == nil {
		t.Fatal("real producer must publish a root-cause board")
	}
	var ranked *tracequery.RootCauseRankItem
	for i := range produced.FrameRootCauseBundle.RootCauseRank.Items {
		item := &produced.FrameRootCauseBundle.RootCauseRank.Items[i]
		if item.Rank == 1 {
			ranked = item
			break
		}
	}
	if ranked == nil || len(ranked.OccurrenceWindows) != 5 {
		t.Fatalf("expected the fixed trace's five distinct occurrences, got %+v", ranked)
	}
	first := ranked.OccurrenceWindows[0]
	ms := func(v float64) string { return fmt.Sprintf("%.3fms", v) }
	window := fmt.Sprintf("%.6f..%.6f", first.Window.StartTs, first.Window.EndTs)
	if ms(first.TotalMs) != "2.978ms" || ms(first.RunnableMs) != "2.377ms" || ms(first.RunningMs) != "0.483ms" || ms(first.SleepMs) != "0.118ms" || first.DominantState != "runnable" {
		t.Fatalf("producer prerequisite: distinct measured state/total rulers missing: %+v", first)
	}
	var compact string
	for _, observation := range result.Observations {
		if observation.Predicate != "root_cause_primary" {
			continue
		}
		for _, note := range observation.RichNotes {
			if strings.HasPrefix(note, "occurrence_windows="+window+",") {
				compact = strings.TrimPrefix(note, "occurrence_windows=")
				break
			}
		}
	}
	if !strings.Contains(compact, "runnable=2.377ms") || !strings.Contains(compact, "running=0.483ms") || !strings.Contains(compact, "sleep=0.118ms") || len(strings.Split(compact, ";")) != 4 {
		t.Fatalf("producer prerequisite: compact must already publish state values and retain its four-window cap: %q", compact)
	}
	t.Logf("actual producer: occurrence=%s total=%s runnable=%s running=%s; published=4 of 5", window, ms(first.TotalMs), ms(first.RunnableMs), ms(first.RunningMs))
	resultBefore := b1689Snapshot(t, result)
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			ctx := &types.AgentContext{RepoRoot: queryBus.RepoRoot, WorkDir: t.TempDir(), Language: lang,
				Mutable:    types.NewMutableState("inspect measured causal contributions"),
				AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: lang, Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}},
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
			modelText := "The reported intervals are preserved for independent inspection."
			if lang == "zh" {
				modelText = "已保留查询区间，供独立核对。"
			}
			answer, _ := json.Marshal(map[string]any{"blocks": []map[string]any{{
				"id": "model-summary", "kind": "summary", "surface_role": "principal",
				"trace_causal_claim_caliber": "no_causal_conclusion", "text": modelText,
			}}})
			answerBus := types.ToolBusContext(ctx, types.AgentFinalizer)
			answerBus.ToolResults = []types.ToolResult{result}
			emitted, err := (&tool.EmitAnswerDocument{}).Execute(answerBus, answer)
			if err != nil || !emitted.Success || ctx.Mutable.AnswerDocumentV2() == nil {
				t.Fatalf("public emit prerequisite failed: %v %s", err, emitted.Summary)
			}
			evaluator := &answerDocumentEvaluator{language: lang}
			promptBefore := evaluator.BuildInitialInstruction(ctx, nil)
			docBefore := b1689Snapshot(t, ctx.Mutable.AnswerDocumentV2())
			ledgerBefore := b1689Snapshot(t, answerDocObservationLedger(ctx))
			artifactsBefore := b1689Snapshot(t, ctx.Mutable.TurnAArtifacts())
			projectionBefore := b1689Snapshot(t, types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx)))
			selectionBefore := b1689Snapshot(t, ctx.Mutable.TraceRootCauseReport())
			out, err := evaluator.ParseOutput(ctx, nil, nil, nil)
			if err != nil || out == nil {
				t.Fatalf("public final parse failed: %v", err)
			}
			if !strings.Contains(out.FinalAnswer, modelText) {
				t.Fatal("model-owned summary was lost")
			}
			if docBefore != b1689Snapshot(t, ctx.Mutable.AnswerDocumentV2()) || ledgerBefore != b1689Snapshot(t, answerDocObservationLedger(ctx)) ||
				artifactsBefore != b1689Snapshot(t, ctx.Mutable.TurnAArtifacts()) || projectionBefore != b1689Snapshot(t, types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx))) ||
				selectionBefore != b1689Snapshot(t, ctx.Mutable.TraceRootCauseReport()) || resultBefore != b1689Snapshot(t, result) {
				t.Fatal("display mutated the accepted model document, observations, projection, selection or tool result")
			}
			if promptAfter := (&answerDocumentEvaluator{language: lang}).BuildInitialInstruction(ctx, nil); promptAfter != promptBefore {
				t.Fatal("last-mile display changed the model's initial input")
			}
			// Inspect the one occurrence in the final supplement, not a coincident
			// number from another root, a cumulative account or the model body.
			var occurrence, occurrenceLine string
			for _, line := range strings.Split(out.FinalAnswer, "\n") {
				if !strings.Contains(line, "CookieMonsterCl-59843") || !strings.Contains(line, window) {
					continue
				}
				if !(strings.Contains(line, "发生窗口：") || strings.Contains(line, "occurrence windows: ")) {
					continue
				}
				occurrenceLine = line
				occurrence = line[strings.Index(line, window):]
				closing := ")"
				if lang == "zh" {
					closing = "）"
				}
				if end := strings.Index(occurrence, closing); end >= 0 {
					occurrence = occurrence[:end+len(closing)]
				}
				break
			}
			if occurrence == "" {
				t.Fatal("real occurrence did not reach its final supplement")
			}
			wants := []string{"dominant state: runnable wait", "dominant-state time: " + ms(first.DominantImpactMs), "occurrence total: " + ms(first.TotalMs), "running: " + ms(first.RunningMs), "scheduling wait: " + ms(first.RunnableMs), "query-window impact: " + ms(first.ProjectedImpactMs), "query-window total: " + ms(first.ProjectedTotalMs), "original-window impact: " + ms(first.ActualImpactMs), "original-window total: " + ms(first.ActualTotalMs), "original state window: " + fmt.Sprintf("%.6f..%.6f", first.ActualWindow.StartTs, first.ActualWindow.EndTs), "target blocked time: " + ms(first.TargetBlockedMs)}
			if lang == "zh" {
				wants = []string{"主导状态：可运行等待", "主导状态耗时：" + ms(first.DominantImpactMs), "本段总占时：" + ms(first.TotalMs), "运行：" + ms(first.RunningMs), "调度等待：" + ms(first.RunnableMs), "查询窗内影响时长：" + ms(first.ProjectedImpactMs), "查询窗内总占时：" + ms(first.ProjectedTotalMs), "原始窗影响时长：" + ms(first.ActualImpactMs), "原始窗总占时：" + ms(first.ActualTotalMs), "原始状态窗：" + fmt.Sprintf("%.6f..%.6f", first.ActualWindow.StartTs, first.ActualWindow.EndTs), "目标阻塞时长：" + ms(first.TargetBlockedMs)}
			}
			for _, want := range wants {
				if !strings.Contains(occurrence, want) {
					t.Errorf("the already-published per-occurrence ruler %q was lost: %s", want, occurrence)
				}
			}
			sleep := "sleep wait: " + ms(first.SleepMs)
			if lang == "zh" {
				sleep = "睡眠等待：" + ms(first.SleepMs)
			}
			if !strings.Contains(occurrence, sleep) {
				t.Errorf("the already-published sleep measurement was lost: %s", occurrence)
			}
			fifth := ranked.OccurrenceWindows[4]
			if strings.Contains(occurrenceLine, fmt.Sprintf("%.6f..%.6f", fifth.Window.StartTs, fifth.Window.EndTs)) {
				t.Fatal("last-mile display expanded the producer's occurrence cap")
			}
			lastIndex := -1
			for _, published := range ranked.OccurrenceWindows[:4] {
				at := strings.Index(occurrenceLine, fmt.Sprintf("%.6f..%.6f", published.Window.StartTs, published.Window.EndTs))
				if at <= lastIndex {
					t.Fatalf("public supplement lost or reordered a published occurrence: %s", occurrenceLine)
				}
				lastIndex = at
			}
		})
	}
}

func b1689Snapshot(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestB1689OccurrenceDistinctWindowsAndStates(t *testing.T) {
	compact := "3.000000..3.020000,state=d_sleep,impact=4.250ms,projected_impact=6.750ms,total=9.125ms,projected_total=8.125ms,actual_impact=10.250ms,actual_total=12.500ms,actual_window=2.990000..3.030000,target=6.500ms,running=0.500ms,runnable=0.250ms,sleep=1.000ms,d_state=4.250ms,io_wait=2.500ms,lines=123456789-123456999;2.000000..2.002000,state=running,total=1.500ms"
	for _, zh := range []bool{true, false} {
		got := traceQueryObservationSupplementOccurrenceWindows(compact, zh)
		wants := []string{"dominant state: uninterruptible wait", "dominant-state time: 4.250ms", "query-window impact: 6.750ms", "occurrence total: 9.125ms", "query-window total: 8.125ms", "original-window impact: 10.250ms", "original-window total: 12.500ms", "original state window: 2.990000..3.030000", "target blocked time: 6.500ms", "running: 0.500ms", "scheduling wait: 0.250ms", "sleep wait: 1.000ms", "uninterruptible wait: 4.250ms", "IO wait: 2.500ms"}
		if zh {
			wants = []string{"主导状态：不可中断等待", "主导状态耗时：4.250ms", "查询窗内影响时长：6.750ms", "本段总占时：9.125ms", "查询窗内总占时：8.125ms", "原始窗影响时长：10.250ms", "原始窗总占时：12.500ms", "原始状态窗：2.990000..3.030000", "目标阻塞时长：6.500ms", "运行：0.500ms", "调度等待：0.250ms", "睡眠等待：1.000ms", "不可中断等待：4.250ms", "IO等待：2.500ms"}
		}
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Errorf("zh=%t lost the independent ruler %q: %s", zh, want, got)
			}
		}
		for _, raw := range []string{"state=", "projected_", "actual_", "d_sleep", "d_state", "io_wait", "lines=", "123456789", "123456999"} {
			if strings.Contains(got, raw) {
				t.Errorf("zh=%t leaked internal field/state or unqualified line %q: %s", zh, raw, got)
			}
		}
		if !strings.HasPrefix(got, "3.000000..3.020000") || strings.Index(got, "2.000000..2.002000") < strings.Index(got, "3.000000..3.020000") {
			t.Fatal("display must preserve the producer's occurrence order")
		}
	}
}

func TestB1689OccurrenceMissingAndExplicitZero(t *testing.T) {
	for _, zh := range []bool{true, false} {
		got := traceQueryObservationSupplementOccurrenceWindows("1.000000..1.002000,state=unrecognized_internal_state,total=2.000ms,impact=,running=,unknown=99.000ms,lines=123456789", zh)
		want := "1.000000..1.002000 (dominant state: unrecognized state, occurrence total: 2.000ms)"
		if zh {
			want = "1.000000..1.002000（主导状态：未识别状态，本段总占时：2.000ms）"
		}
		if got != want {
			t.Errorf("missing fields must stay missing, and unknown internal states must not leak: got %q want %q", got, want)
		}
		zero := traceQueryObservationSupplementOccurrenceWindows("1..2,impact=0ms,total=0ms,projected_impact=0ms,projected_total=0ms,actual_impact=0ms,actual_total=0ms,target=0ms,running=0ms,runnable=0ms,sleep=0ms,d_state=0ms,io_wait=0ms", zh)
		if strings.Count(zero, "0ms") != 12 {
			t.Errorf("explicit zero measurements must be preserved exactly, not dropped or inferred: %s", zero)
		}
		if blank := traceQueryObservationSupplementOccurrenceWindows("1..2,state=,impact=,total=,actual_window=", zh); blank != "1..2" {
			t.Errorf("empty fields do not license invented state, window or zero: %q", blank)
		}
	}
}

func TestB1689OccurrenceStateNames(t *testing.T) {
	for _, tc := range []struct{ raw, zh, en string }{
		{"running", "运行", "running"}, {"runnable", "可运行等待", "runnable wait"},
		{"s_sleep", "睡眠等待", "sleep wait"}, {"d_sleep", "不可中断等待", "uninterruptible wait"}, {"io_wait", "IO等待", "IO wait"},
		{"future_internal_state", "未识别状态", "unrecognized state"},
	} {
		for _, zh := range []bool{true, false} {
			want := "1..2 (dominant state: " + tc.en + ")"
			if zh {
				want = "1..2（主导状态：" + tc.zh + "）"
			}
			if got := traceQueryObservationSupplementOccurrenceWindows("1..2,state="+tc.raw, zh); got != want {
				t.Errorf("known state must describe the observed lane rather than imply a root cause: got %q want %q", got, want)
			}
		}
	}
}
