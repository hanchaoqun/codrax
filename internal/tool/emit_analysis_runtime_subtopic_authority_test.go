package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func explicitWindowCausalRequestModelForSubtopicAuthority() types.RequestModel {
	start, end := 5.0, 5.007
	return types.RequestModel{
		SubTopics: []types.SubTopic{
			{Summary: "provisional timing claim"},
			{Summary: "provisional blocking mechanism"},
		},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
			Scope: types.RuntimeQuestionScopeCausalDiagnosis,
		},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
			RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
			TimeStart:      &start,
			TimeEnd:        &end,
			SourceQuote:    "5.000s 到 5.007s",
		},
		RuntimeTargetProfile: &types.RuntimeTargetProfile{
			Declaration: types.RuntimeTargetDeclarationNamedTarget,
			SourceQuote: "目标线程是 app-100",
		},
		RuntimeTargets: []types.RuntimeTarget{{
			Kind:   types.RuntimeTargetKindThread,
			Thread: "app-100",
		}},
	}
}

func TestNormalizeSingleTargetExplicitWindowCausalSubTopicsDropsModelPlanningClaims(t *testing.T) {
	rm := explicitWindowCausalRequestModelForSubtopicAuthority()
	warning := normalizeSingleTargetExplicitWindowCausalSubTopics(&rm)
	if warning == "" {
		t.Fatal("expected an audit warning when model-authored runtime sub-topics are removed")
	}
	if len(rm.SubTopics) != 0 {
		t.Fatalf("single-target explicit-window causal lane kept %d model sub-topics: %+v", len(rm.SubTopics), rm.SubTopics)
	}
	if len(rm.RuntimeTargets) != 1 || rm.RuntimeTargets[0].Thread != "app-100" {
		t.Fatalf("typed target changed while removing planning labels: %+v", rm.RuntimeTargets)
	}
}

func TestNormalizeSingleTargetExplicitWindowCausalSubTopicsPreservesWiderTypedShapes(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*types.RequestModel)
	}{
		{
			name: "full artifact",
			mutate: func(rm *types.RequestModel) {
				rm.RuntimeArtifactScopeProfile.RequestedScope = types.RuntimeArtifactScopeFullArtifact
				rm.RuntimeArtifactScopeProfile.TimeStart = nil
				rm.RuntimeArtifactScopeProfile.TimeEnd = nil
			},
		},
		{
			name: "relation analysis",
			mutate: func(rm *types.RequestModel) {
				rm.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeRelationAnalysis
			},
		},
		{
			name: "multiple targets",
			mutate: func(rm *types.RequestModel) {
				rm.RuntimeTargets = append(rm.RuntimeTargets, types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, Thread: "worker-200"})
			},
		},
		{
			name: "cross component",
			mutate: func(rm *types.RequestModel) {
				rm.Predicates.IsCrossComponent = true
			},
		},
		{
			name: "explicit buckets",
			mutate: func(rm *types.RequestModel) {
				rm.Buckets = []types.QuestionBucket{{Label: "A", Index: 1}, {Label: "B", Index: 2}}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm := explicitWindowCausalRequestModelForSubtopicAuthority()
			tt.mutate(&rm)
			before := len(rm.SubTopics)
			if warning := normalizeSingleTargetExplicitWindowCausalSubTopics(&rm); warning != "" {
				t.Fatalf("wider typed shape unexpectedly normalized: %s", warning)
			}
			if len(rm.SubTopics) != before {
				t.Fatalf("wider typed shape lost sub-topics: got=%d want=%d", len(rm.SubTopics), before)
			}
		})
	}
}
