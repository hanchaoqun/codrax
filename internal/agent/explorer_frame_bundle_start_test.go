package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestExplorerKnownFrameBundleFirst(t *testing.T) {
	ctx, rm := runtimeMeasurementPlanningContext(t)
	rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
		Scope: types.RuntimeQuestionScopeCausalDiagnosis, FrameCausalityRequested: true,
	}
	rm.RuntimeTargetProfile = &types.RuntimeTargetProfile{
		Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "tid 62116",
	}
	rm.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 62116, Source: "user_explicit"}}
	rm.PerfTrace = ctx.Mutable.PerfTrace()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
	ctx.Stage = types.StageExplore
	material := ctx.AttachedTraceMaterial
	request := &ctx.AnalysisIR.RequestModel
	explorer := &explorerEvaluator{}
	for _, tc := range []struct {
		name   string
		change func()
	}{
		{name: "known frame"},
		{name: "no physical attachment", change: func() { ctx.AttachedTraceMaterial = nil }},
		{name: "unknown window", change: func() { request.RuntimeArtifactScopeProfile = nil }},
		{name: "multiple windows", change: func() {
			window := types.RuntimeArtifactTimeWindow{
				TimeStart: rm.RuntimeArtifactScopeProfile.TimeStart, TimeEnd: rm.RuntimeArtifactScopeProfile.TimeEnd, SourceQuote: "2 to 2.25 seconds",
			}
			start, end := 3.0, 3.25
			request.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
				RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
				TimeWindows:    []types.RuntimeArtifactTimeWindow{window, {TimeStart: &start, TimeEnd: &end, SourceQuote: "3 to 3.25 seconds"}},
			}
		}},
		{name: "process only", change: func() {
			request.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindProcess, PID: 62116, Source: "user_explicit"}}
		}},
		{name: "ordinary causal question", change: func() {
			request.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			*request = rm
			ctx.AttachedTraceMaterial = material
			if tc.change != nil {
				tc.change()
			}
			prompt := explorer.BuildInitialInstruction(ctx, nil)
			firstCall := "Use `trace_query(view=\"frame_root_cause_bundle\", pid=62116, time_start=2, time_end=2.25)` as the first trace query after any required attachment reads"
			legacy := "first establish the target thread's state priority"
			if tc.change == nil {
				if !strings.Contains(prompt, firstCall) || strings.Contains(prompt, legacy) ||
					!strings.Contains(prompt, "coverage line takes precedence") ||
					!strings.Contains(strings.ToLower(prompt), "if the bundle is unavailable or incomplete") ||
					!strings.Contains(prompt, "Complete with `emit_investigation_complete`") {
					t.Fatal("known frame lost the bundle first call, focused fallback, or completion guidance")
				}
			} else if strings.Contains(prompt, "as the first trace query after any required attachment reads") || !strings.Contains(prompt, legacy) {
				t.Fatal("out-of-scope request did not retain the original first-call guidance")
			}
		})
	}
}
