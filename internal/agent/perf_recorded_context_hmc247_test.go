package agent

import (
	"errors"
	"strings"
	"testing"

	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the same registered skill and production Execute adapter boundary as
// a live dispatch. The capture adapter stops before any model or tool side effect.
func TestHMC247ActualExplorerSamplingTeachingPreservesTypedExecutionBoundary(t *testing.T) {
	for _, scope := range []types.RuntimeQuestionScope{"", types.RuntimeQuestionScopeCausalDiagnosis} {
		t.Run(string(scope), func(t *testing.T) {
			ctx := traceTeachingRuntimeContext(types.StageExplore)
			ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: scope}
			ctx.AttachedHitrace = "poll_report-1609 (1514) [000] .... 486489.464088: perf_sample: cpu=0 cpu_known=true sample_kind=unknown sample_weight=1 symbol=OnDataEvent_\n"
			registry := toolpkg.NewRegistry()
			registry.Register(&toolpkg.TraceQuery{})
			capture := &traceTeachingCaptureLLM{stop: errors.New("captured sampling teaching")}
			explorer := NewExplorerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1})
			_, err := explorer.Execute(ctx, traceTeachingSkill(t, "explore-skill"))
			if !errors.Is(err, capture.stop) || capture.calls != 1 {
				t.Fatalf("actual adapter boundary: calls=%d err=%v", capture.calls, err)
			}
			var system string
			for _, message := range capture.messages {
				if message.Role == "system" {
					system += message.Content + "\n"
				}
			}
			for _, want := range []string{
				"recorded code-location and stack context",
				"sample_kind=unknown leaves execution state unestablished even when a CPU is recorded",
				"sample_kind=off_cpu records an off-CPU sample, not a measured blocking interval",
				"sample_cpu_scope=unknown or cpu_known=false leaves the CPU/core unavailable",
				"sample_kind=on_cpu supports only the recorded on-CPU sample points",
				"For independently established running or compute-supply causes",
				"positive CAP/compute-supply deficit and otherwise remains context_only",
				"business_span_mention families (verbatim span + trio)",
			} {
				if strings.Count(system, want) != 1 {
					t.Errorf("actual prompt must publish exactly one %q", want)
				}
			}
			for _, stale := range []string{"Treat perf sample output as code-execution context", "what code consumed sampled CPU time", "what sampled threads were doing"} {
				if strings.Contains(system, stale) {
					t.Errorf("actual prompt still upgrades sample meaning with %q", stale)
				}
			}
		})
	}
}
