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
		"typed whole-artifact query coverage is present",
		"Scope coverage alone does not prove",
		"independent typed enumeration authority",
	} {
		if !strings.Contains(withCensus, want) {
			t.Fatalf("missing available whole-artifact guidance %q:\n%s", want, withCensus)
		}
	}
	if strings.Contains(withCensus, "whole-artifact account is unavailable") {
		t.Fatalf("available whole-artifact census retained the unavailable warning:\n%s", withCensus)
	}
}

func TestRuntimeTraceGuidanceAcceptsTypedUnboundedModelQueryCoverage(t *testing.T) {
	ctx := runtimeArtifactScopeGuidanceContext()
	artifacts := ctx.Mutable.TurnAArtifacts()
	artifacts.ToolResults[0].Observations = append(artifacts.ToolResults[0].Observations, types.ObservationRecord{
		ID:              "trace_query:full#runtime_artifact_scope_coverage",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: types.ObservationSourceRef{
			Kind:         types.ObservationSourceRuntimeArtifact,
			ArtifactID:   "runtime_artifact:full",
			ArtifactKind: "trace",
		},
		Predicate: types.RuntimeArtifactScopeCoveragePredicate,
		Object:    string(types.RuntimeArtifactScopeFullArtifact),
		Value:     "thread_timeline",
		Scope:     string(types.RuntimeArtifactScopeFullArtifact),
	})
	ctx.Mutable.SetTurnAArtifacts(*artifacts)

	got := renderAnswerDocRuntimeTraceAnswerGuidance(ctx)
	for _, want := range []string{
		"typed whole-artifact query coverage is present",
		"Scope coverage alone does not prove",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing unified model-query coverage guidance %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "whole-artifact account is unavailable") {
		t.Fatalf("unbounded typed model query retained unavailable warning:\n%s", got)
	}
}
