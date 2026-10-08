package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCPUStateFrequencyPublicFinalContextPreservesUnknownAndWindow(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_cpu_state_frequency/events.systrace")
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Mutable: types.NewMutableState("CPU空闲与频率"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", PerfTrace: &types.PerfBundle{}}}}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "cpu_state_frequency", "time_start": 1, "time_end": 1.04})
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !r.Success {
		t.Fatalf("query: %v %+v", err, r)
	}
	ctx.Mutable.AppendDispatchToolResult(r)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	requestedStart, requestedEnd := 1.0, 1.04
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &requestedStart, TimeEnd: &requestedEnd, SourceQuote: "1..1.04"}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"### 已观测CPU状态与频率联合区间", "全部核时间=120", "联合未知=60", "CPU2", "idle状态2", "每核占比分母=40", "[1.000000000,1.010000000)", "不分配根因排名"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("lost %q", want)
		}
	}
	if strings.Contains(prompt, "actual query window is unknown") || strings.Contains(prompt, "实际查询范围未明确") {
		t.Fatal("valid CPU query receipt contradicted by unknown-window teaching")
	}
	var record types.ObservationRecord
	for _, candidate := range r.Observations {
		if candidate.Predicate == tool.TraceCPUStateFrequencyPredicate {
			record = candidate
		}
	}
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{record, record}}
	if got := renderAnswerDocCPUStateFrequency(ctx, ledger); strings.Count(got, "### 已观测CPU状态与频率联合区间") != 1 {
		t.Fatal("identical copy lost")
	}
	p, _ := tool.DecodeTraceCPUStateFrequency(record)
	p.CPUs[0].CPU = 7
	body, _ := json.Marshal(p)
	conflict := record
	conflict.RichNotes = []string{types.TraceNoteKeyCPUStateFrequency + "=" + string(body)}
	if _, ok := tool.DecodeTraceCPUStateFrequency(conflict); !ok {
		t.Fatal("conflict fixture must be individually valid")
	}
	ledger.Records[1] = conflict
	if got := renderAnswerDocCPUStateFrequency(ctx, ledger); got != "" {
		t.Fatalf("conflicting same credential picked by order: %s", got)
	}
	start, end := 1.005, 1.035
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.005..1.035"}
	if strings.Contains(renderAnswerDocCPUStateFrequency(ctx, answerDocObservationLedger(ctx)), "### 已观测CPU状态与频率联合区间") {
		t.Fatal("borrowed wider account")
	}
}

func TestNativeCPUIntervalsReachActualFinalizerContext(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_cpu_native_intervals/capture.data")
	dir := t.TempDir()
	start, end := 0.0, 0.04
	profile := &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "前40毫秒"}
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("CPU组合分布"), TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")}),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", PerfTrace: &types.PerfBundle{}, RuntimeArtifactScopeProfile: profile}}}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "cpu_state_frequency", "time_start": start, "time_end": end})
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !r.Success {
		t.Fatalf("query: %v %+v", err, r)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"### 已观测CPU状态与频率联合区间", "全部核时间=80", "联合未知=45", "源CPU状态码0（含义未核实）", "源CPU状态码2（含义未核实）", "1000000", "不分配根因排名"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("native handoff lost %q", want)
		}
	}
	if strings.Contains(prompt, "actual query window is unknown") || strings.Contains(prompt, "实际查询范围未明确") {
		t.Fatal("native window lost at finalizer")
	}
}
