package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeArtifactScopeGuidanceContext() *types.AgentContext {
	mut := types.NewMutableState("只分析这份 trace")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			ID:              "trace_query:local#state_churn",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "state_churn",
			ClaimKey:        "state_churn:worker-200",
			Subject:         "worker-200",
		}},
	}}})
	return &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
				RequestedScope: types.RuntimeArtifactScopeFullArtifact,
				SourceQuote:    "这份 trace",
				Confidence:     1,
			},
		}},
	}
}

func TestRuntimeTraceGuidanceSeparatesFullArtifactScopeFromModelWindows(t *testing.T) {
	ctx := runtimeArtifactScopeGuidanceContext()
	withoutCensus := renderAnswerDocRuntimeTraceAnswerGuidance(ctx)
	for _, want := range []string{
		"`requested_scope=full_artifact`",
		"local witness only",
		"do not claim artifact-wide",
	} {
		if !strings.Contains(withoutCensus, want) {
			t.Fatalf("missing unavailable whole-artifact guidance %q:\n%s", want, withoutCensus)
		}
	}

	ctx.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{
		Views:                   []string{"window_stats"},
		RequestedArtifactScope:  types.RuntimeArtifactScopeFullArtifact,
		ViewValueObservations:   []int{1},
		ViewObservationFamilies: []types.TraceSupplementViewFamilyCensus{{TargetStateRows: 1}},
	}, nil)
	withCensus := renderAnswerDocRuntimeTraceAnswerGuidance(ctx)
	for _, want := range []string{
		"typed whole-artifact supplement result is present",
		"Use that whole-artifact account",
		"model-authored narrower trace_query windows are local witnesses",
	} {
		if !strings.Contains(withCensus, want) {
			t.Fatalf("missing available whole-artifact guidance %q:\n%s", want, withCensus)
		}
	}
	if strings.Contains(withCensus, "whole-artifact account is unavailable") {
		t.Fatalf("available whole-artifact census retained the unavailable warning:\n%s", withCensus)
	}
}
