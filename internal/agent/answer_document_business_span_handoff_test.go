package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
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
