package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestLogTriagerInitialInstructionCarriesProducerOwnedCounterSemantics(t *testing.T) {
	e := &logTriagerEvaluator{}
	ctx := &types.AgentContext{AttachedLog: strings.Join([]string{
		"WARN [orchestrator] finalizer attempt 1/3 failed: timeout",
		"INFO [render] ⟳ 4/4 模型响应出错,正在重新撰写答案",
	}, "\n")}
	got := e.BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"counter_domain=agent_dispatch_attempt value=1/3",
		"counter_domain=pipeline_stage_progress value=4/4",
		"does_not_mean=model_count",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("initial instruction missing %q:\n%s", want, got)
		}
	}
}

func TestStampLogOperationalSemanticsUsesHeldFullLog(t *testing.T) {
	mu := types.NewMutableState("log-operational-semantics")
	mu.SetLogTriage(&types.LogBundle{Meta: types.LogMeta{Lang: "unknown"}})
	ctx := &types.AgentContext{Mutable: mu}
	raw := "INFO [render] ⟳ 4/4 模型响应出错,正在重新撰写答案"

	stampLogOperationalSemantics(ctx, raw)

	bundle := mu.LogTriage()
	if bundle == nil || len(bundle.OperationalSemantics) != 1 ||
		bundle.OperationalSemantics[0].CounterDomain != types.LogOperationalCounterPipelineStageProgress {
		t.Fatalf("system semantics not stamped: %+v", bundle)
	}
}
