package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSleepStatisticsPublicBoundedFinalContext(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_sleep_summary/events.systrace")
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Mutable: types.NewMutableState("sleep"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Language: "zh", PerfTrace: &types.PerfBundle{},
		RuntimeTargets:         []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, Thread: "target-41 (tid=41)", Source: "user_explicit", Confidence: .95}},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetWaitOccurrences, types.RuntimeQuestionFactCountOrDuration, types.RuntimeQuestionFactDirectWaker}},
	}}}
	params := json.RawMessage(`{"source":"path","path":` + strconvQuoteForSleepTest(path) + `,"view":"wakeup_chain","pid":41,"time_start":10.001,"time_end":10.012}`)
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !r.Success {
		t.Fatalf("query: %v %+v", err, r)
	}
	ctx.Mutable.AppendDispatchToolResult(r)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"2 intervals; clipped sum=4ms, mean=2ms, max=3ms", "1 intervals; clipped sum=3ms, mean=3ms, max=3ms", "not proof of cause or completion"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("final context lost full-population statistic %q", want)
		}
	}
}

func strconvQuoteForSleepTest(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
