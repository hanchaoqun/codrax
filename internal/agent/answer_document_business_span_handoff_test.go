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

func TestBusinessSpanActualFinalContextKeepsFactsWithoutRelationRequest(t *testing.T) {
	ctx := hmosBusinessIOFinalizerContext(t)
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.RuntimeWorkRelationRequested = false
	before, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []struct{ name, subject, duration string }{{"OpenDocument", "app-main-100", "50.000"}, {"LoadDocumentIndex", "document-worker-200", "40.000"}} {
		found := false
		for _, line := range strings.Split(prompt, "\n") {
			if strings.Contains(line, "业务 "+fmt.Sprintf("%q", want.name)) && strings.Contains(line, want.subject) && strings.Contains(line, "耗时 "+want.duration+" 毫秒") && strings.Contains(line, "source=") && strings.Contains(line, "observation_id=") && strings.Contains(line, "query_scope=") {
				found = true
				if strings.Contains(line, "不是窗口内耗时") {
					t.Errorf("an entirely in-window span was falsely taught as outside the window ruler: %s", line)
				}
			}
		}
		if !found {
			t.Errorf("actual compact final-context lost one bound business fact: %+v", want)
		}
	}
	if view := types.BuildAnswerSemanticViewForAgentContext(ctx); view.RuntimeWorkRelationContract.Active() {
		t.Fatal("factual handoff activated an unrequested receipt")
	}
	after, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
	if string(before) != string(after) {
		t.Fatal("business handoff changed accepted tool facts")
	}
}

func TestBusinessSpanExplicitWindowKeepsChoicesAndFactsOnSameScope(t *testing.T) {
	ctx := hmosBusinessIOFinalizerContext(t)
	start, end := 1.01, 1.02
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.01..1.02"}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, "### 已观测业务区间") || types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeWorkRelationContract.Active() {
		t.Fatal("broad historical query created a hidden narrow-window work choice")
	}
	path, err := filepath.Abs(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_business_io_chain", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": start, "time_end": end})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success {
		t.Fatalf("narrow public query failed: %v %+v", err, result)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	prompt = (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	var workLine string
	for _, line := range strings.Split(prompt, "\n") {
		if strings.Contains(line, "业务 \"OpenDocument\"") {
			workLine = line
			break
		}
	}
	for _, want := range []string{"所选窗口 1.010000–1.020000", "窗口内区间 1.010000–1.020000", "耗时 10.000 毫秒", "原始配对范围 1.000000–1.050000", "完整耗时 50.000 毫秒（不是窗口内耗时）"} {
		if !strings.Contains(workLine, want) {
			t.Errorf("clipped business detail lost %q: %s", want, workLine)
		}
	}
	contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeWorkRelationContract
	if !contract.Active() {
		t.Fatal("narrow returned work did not restore visible choices")
	}
	for _, row := range contract.Rows {
		if row.MeasuredDurationMS != 10 {
			t.Errorf("work receipt borrowed the physical/full duration: %+v", row)
		}
	}
}

func TestBusinessSpanHandoffCapDisclosesAndKeepsChoiceIDsVisible(t *testing.T) {
	ctx := hmosBusinessIOFinalizerContext(t)
	ledger := answerDocObservationLedger(ctx)
	facts := types.TraceBusinessSpanFacts(ledger, &ctx.AnalysisIR.RequestModel)
	if len(facts) < 1 {
		t.Fatal("missing actual published facts")
	}
	var rows []types.ObservationRecord
	for i := 0; i < types.TraceBusinessSpanFactLimit+3; i++ {
		r := facts[0]
		r.ID, r.Object = fmt.Sprintf("span-%02d", i), fmt.Sprintf("Work-%02d", i)
		rows = append(rows, r)
	}
	ledger.Records = rows
	text := renderAnswerDocBusinessSpanFacts(ctx, ledger)
	if !strings.Contains(text, "19 条业务区间中的前 16 条") || !strings.Contains(text, "3 条未展示") {
		t.Fatalf("capped context claimed complete visibility: %s", text)
	}
	contract := types.BuildRuntimeWorkRelationContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: rows}}}, true)
	for _, row := range contract.Rows {
		if !strings.Contains(text, fmt.Sprintf("observation_id=%q", row.ObservationID)) {
			t.Errorf("schema choice has no visible factual row: %+v", row)
		}
	}
	if strings.Contains(text, "Work-16") {
		t.Fatal("prompt cap and selection cap disagree")
	}
}

func TestBusinessSpanSchedulerActualFinalizerMessageHasMarkerLocalRulers(t *testing.T) {
	ctx := hmosBusinessIOFinalizerContext(t)
	before, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []struct{ name, running, runnable, sleep, total string }{
		{"OpenDocument", "5.000", "1.000", "44.000", "50.000"},
		{"LoadDocumentIndex", "8.000", "1.000", "31.000", "40.000"},
	} {
		found := false
		for _, line := range strings.Split(prompt, "\n") {
			if strings.Contains(line, "业务 "+fmt.Sprintf("%q", want.name)) {
				found = strings.Contains(line, "本业务区间的线程状态") &&
					strings.Contains(line, "运行 "+want.running+" 毫秒") &&
					strings.Contains(line, "等待调度 "+want.runnable+" 毫秒") &&
					strings.Contains(line, "睡眠 "+want.sleep+" 毫秒") &&
					strings.Contains(line, "已计量 "+want.total+" 毫秒")
			}
		}
		if !found {
			t.Errorf("actual finalizer message has no marker-local state breakdown for %s", want.name)
		}
	}
	after, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
	if string(before) != string(after) {
		t.Fatal("marker state handoff mutated accepted facts")
	}
	ctx.Language, ctx.AnalysisIR.RequestModel.Language = "en", "en"
	english := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(english, "marker-local scheduler states: running 5.000 ms, runnable 1.000 ms, sleep 44.000 ms") ||
		!strings.Contains(english, "must not be replaced by wider-query totals") {
		t.Fatal("English finalizer lost the marker-local measurement or scope boundary")
	}
}

func TestBusinessSpanSchedulerFinalizerMissingCoverageIsNotZero(t *testing.T) {
	for _, partial := range []bool{false, true} {
		ctx := hmosBusinessIOFinalizerContext(t)
		trace := "# tracer: nop\ntask-700 (600) [003] .... 9.000000: tracing_mark_write: B|600|OtherWork\n"
		if partial {
			trace += "irq-80 (2) [003] .... 9.005000: sched_wakeup: comm=task pid=700 prio=120 target_cpu=003\n" +
				"task-700 (600) [003] .... 9.006000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=700 next_prio=120\n"
		}
		trace += "task-700 (600) [003] .... 9.010000: tracing_mark_write: E|600\n"
		path := filepath.Join(t.TempDir(), "unknown.systrace")
		if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
			t.Fatal(err)
		}
		params, _ := json.Marshal(map[string]any{"path": path, "view": "window_stats", "time_start": 9, "time_end": 9.01})
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
		if err != nil || !result.Success {
			t.Fatalf("public query failed: %v %+v", err, result)
		}
		ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
		prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
		var line string
		for _, candidate := range strings.Split(prompt, "\n") {
			if strings.Contains(candidate, "业务 \"OtherWork\"") {
				line = candidate
				break
			}
		}
		if partial {
			if !strings.Contains(line, "运行 4.000 毫秒") || !strings.Contains(line, "已计量 5.000 毫秒") || !strings.Contains(line, "仅部分覆盖") || strings.Contains(line, "覆盖完整") {
				t.Fatalf("partial marker measurement was lost or completed: %s", line)
			}
		} else if !strings.Contains(line, "本业务区间的线程状态未能计量，不能按零处理") || strings.Contains(line, "运行 0.000") {
			t.Fatalf("missing scheduler data became measured zero: %s", line)
		}
	}
}

func TestBusinessSpanSchedulerHandoffRejectsMismatchedCarrier(t *testing.T) {
	ctx := hmosBusinessIOFinalizerContext(t)
	facts := types.TraceBusinessSpanFacts(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)
	if len(facts) == 0 {
		t.Fatal("public business facts missing")
	}
	for _, mutate := range []func(*tracequery.TraceSpanSchedulerStates){
		func(s *tracequery.TraceSpanSchedulerStates) { s.Window.StartTs -= .001 },
		func(s *tracequery.TraceSpanSchedulerStates) { s.Thread.PID++ },
		func(s *tracequery.TraceSpanSchedulerStates) { s.SourcePath += ".other" },
		func(s *tracequery.TraceSpanSchedulerStates) { s.RunningMs += 2 },
		func(s *tracequery.TraceSpanSchedulerStates) { s.MeasurementDomain.TargetTID++ },
	} {
		record := facts[0]
		record.RichNotes = append([]string(nil), record.RichNotes...)
		var states tracequery.TraceSpanSchedulerStates
		if err := json.Unmarshal([]byte(traceQueryObservationSupplementNoteValue(record, "business_span_scheduler_states")), &states); err != nil {
			t.Fatal(err)
		}
		mutate(&states)
		data, _ := json.Marshal(states)
		for i, note := range record.RichNotes {
			if strings.HasPrefix(note, "business_span_scheduler_states=") {
				record.RichNotes[i] = "business_span_scheduler_states=" + string(data)
			}
		}
		if got := answerDocBusinessSpanSchedulerMeaning(record, true); got != "" {
			t.Fatalf("mismatched producer carrier reached reader facts: %s", got)
		}
	}
}
