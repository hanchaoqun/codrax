package agent

import (
	"errors"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Capture the actual analyzer-to-adapter boundary. These are contrasting user
// requests, not a deterministic classifier or a scripted model answer: the
// same existing decision table must reach the model without minting intent.
func TestAnalyzerRuntimeCompositionTeachingReachesActualModelMessages(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request string
	}{
		{
			name: "measurements plus causal and work relation questions",
			request: "Only analyze the attached trace. Find the full LoadReport response interval, " +
				"separate request latency, actual thread wait, and post-wakeup scheduling time, " +
				"then determine which dependency really delayed the response. Preserve the measured " +
				"worker operations and their relation to the response; do not assume the longest background transfer caused it.",
		},
		{
			name: "finite measurements without diagnosis",
			request: "Only report the attached trace's request latency, actual thread wait, " +
				"and post-wakeup scheduling time in milliseconds. Do not diagnose a cause.",
		},
		{
			name: "finite condition to target verdict",
			request: "From the attached trace, report the worker's measured frequency and determine " +
				"whether the recorded frequency ceiling constrained this worker. Do not search for other causes.",
		},
		{
			name: "observed dependency path without cause discovery",
			request: "From the attached trace, list the recorded wakeup path between the main thread " +
				"and its worker, including peer identities. No bottleneck or root-cause diagnosis is requested.",
		},
		{
			name:    "one short operation still asks for cause discovery",
			request: "仅分析附加trace中2.010到2.040秒这一次保存操作：先定位业务范围，再找出究竟是哪条依赖拖慢了响应。",
		},
		{
			name:    "same short window asks only for measurements",
			request: "仅统计附加trace中2.010到2.040秒这一次保存操作的运行、可运行和等待时长，不分析根因。",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &types.AgentContext{
				Stage: types.StageAnalyze, Objective: tc.request, Language: "en",
				Mutable:         types.NewMutableState(tc.request),
				AttachedHitrace: "worker-42 (42) [000] .... 1.000000: tracing_mark_write: B|42|LoadReport\n",
			}
			registry := tool.NewRegistry()
			registry.Register(&tool.EmitAnalysis{})
			capture := &traceTeachingCaptureLLM{stop: errors.New("captured analyzer composition teaching")}
			analyzer := NewAnalyzerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1})
			_, err := analyzer.Execute(ctx, skill.BuildAnalysisSkill())
			if !errors.Is(err, capture.stop) || capture.calls != 1 {
				t.Fatalf("expected exactly one initial adapter request: calls=%d err=%v", capture.calls, err)
			}
			var system, user strings.Builder
			for _, message := range capture.messages {
				switch message.Role {
				case "system":
					system.WriteString(message.Content)
				case "user":
					user.WriteString(message.Content)
				}
			}
			for _, teaching := range []string{
				skill.AnalysisRuntimeCausalAttributionTeaching,
				skill.AnalysisRuntimeScopeFromDimensionTeaching,
			} {
				if strings.Count(system.String(), teaching) != 1 {
					t.Fatal("complete shared runtime composition teaching must reach the model exactly once")
				}
			}
			for _, want := range []string{
				"Combine the work-relation decision with `causal_attribution` or `causal_contributor_set`",
				"keep finite measurements beside verdict/work/causal dimensions as their own `observed_value` dimensions",
				"Use `relation_path` only for a separately requested topology, endpoint, or hop sequence",
				"Use `bounded_fact_set` when every requested dimension is a finite observed fact",
				"exactly one required `target_effect_verdict` and no required causal role ALWAYS selects `bounded_effect_verdict`",
				"when the request asks why the target was blocked or asks for the principal blocking mechanism/root cause",
				"Scope never pre-decides the finding",
				"Runtime scope describes the requested conclusion, not the duration or number of windows or operations",
				"Locating one business interval is navigation, not a bounded-fact decision",
			} {
				if !strings.Contains(system.String(), want) {
					t.Errorf("actual model message lost composition/scope boundary %q", want)
				}
			}
			if !strings.Contains(user.String(), tc.request) {
				t.Fatal("the complete current request must survive beside its classification teaching")
			}
			if len(capture.tools) != 1 || capture.tools[0].Name != "emit_analysis" {
				t.Fatalf("expected the actual analyzer classification tool: %+v", capture.tools)
			}
			parameters := string(capture.tools[0].Parameters)
			for _, want := range []string{
				"Runtime scope describes the requested conclusion, not the duration or number of windows or operations",
				"Locating one business interval is navigation, not a bounded-fact decision",
			} {
				if !strings.Contains(parameters, want) {
					t.Errorf("actual schema lost the range/breadth distinction %q", want)
				}
			}
			for _, teaching := range []string{skill.AnalysisRuntimeScopeSchemaTeaching, skill.AnalysisRuntimeDimensionSchemaTeaching} {
				if !strings.Contains(parameters, teaching) {
					t.Fatal("actual emit_analysis schema lost its compact runtime shape teaching")
				}
			}
			if ctx.Mutable.RequestModel() != nil || ctx.AnalysisIR != nil {
				t.Fatal("request wording or attachment presence must not automatically mint classification")
			}
		})
	}
}
