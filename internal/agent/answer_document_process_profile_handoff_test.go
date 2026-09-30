package agent

import (
	"encoding/json"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessProfilePublicFinalContextPreservesUnknownAndWindow(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_process_profile/events.systrace")
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Mutable: types.NewMutableState("进程概览"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", PerfTrace: &types.PerfBundle{}}}}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "process_profile", "pid": 10, "time_start": 1, "time_end": 1.02})
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !r.Success {
		t.Fatalf("query: %v %+v", err, r)
	}
	ctx.Mutable.AppendDispatchToolResult(r)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"### 已观测进程概览", `"pid":12`, `"coverage":"unavailable"`, `"unknown_ms":20`, "已观测成员=3", "Load", "submit_bio", "不画成唤醒因果链"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("lost %q", want)
		}
	}
	start, end := 1.005, 1.015
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.005..1.015"}
	if strings.Contains(renderAnswerDocProcessProfiles(ctx, answerDocObservationLedger(ctx)), "### 已观测进程概览") {
		t.Fatal("borrowed wider account")
	}
}
