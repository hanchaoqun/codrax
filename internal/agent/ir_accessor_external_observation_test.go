package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestOriginSpecificObservationWithoutRequiredSourceForExplorer(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentExplain},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, MinCitations: 2},
			},
		},
	}
	facts := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "MCP rows",
		Value: "2",
		Dimensions: []types.AnswerAggregateDimension{{
			Name:  "origin",
			Value: string(types.AnswerEvidenceOriginMCPResource),
		}},
		Members: []string{"line 7", "line 12"},
	}}

	if !originSpecificObservationWithoutRequiredSourceForExplorer(ctx, facts) {
		t.Fatal("MCP aggregate facts should satisfy external-observation completion when current source is optional")
	}

	ctx.AnalysisIR.RequestModel.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{
		IsCurrentSourceExplanationRequested: true,
		Modes: []types.CurrentSourceExplanationMode{
			types.CurrentSourceExplanationExplainCurrentMechanism,
		},
		Confidence:   0.9,
		SourceQuotes: []string{"结合源码"},
	}
	if !originSpecificObservationWithoutRequiredSourceForExplorer(ctx, facts) {
		t.Fatal("soft current-source profile should allow origin-specific completion with caveat")
	}

	ctx.AnalysisIR.RequestModel.CurrentSourceExplanationProfile.SourceQuotes = []string{"internal/tracequery/query.go:42"}
	if originSpecificObservationWithoutRequiredSourceForExplorer(ctx, facts) {
		t.Fatal("precise current-source profile must keep source completion required")
	}
}

func TestRuntimeArtifactWithoutRequiredSourceForExplorer_UsesPreflightContext(t *testing.T) {
	ctx := &types.AgentContext{
		Stage: types.StageExplore,
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			SourceNavigationOptional: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind:    "trace",
				Source:  "capture.systrace",
				Carrier: "request_path",
			}},
		}),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Scenario: types.ScenarioRootCause,
			Predicates: types.SemanticPredicates{
				IsDiagnosticQuestion: true,
			},
		}},
	}
	if !runtimeArtifactWithoutRequiredSourceForExplorer(ctx) {
		t.Fatal("typed runtime preflight should make current-source proof optional for explorer")
	}

	ctx.AnalysisIR.RequestModel.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{
		IsCurrentSourceExplanationRequested: true,
		Modes: []types.CurrentSourceExplanationMode{
			types.CurrentSourceExplanationExplainCurrentMechanism,
		},
		SourceQuotes: []string{"internal/tracequery/query.go:42"},
		Confidence:   0.9,
	}
	if runtimeArtifactWithoutRequiredSourceForExplorer(ctx) {
		t.Fatal("precise current-source anchor must keep explorer source proof load-bearing")
	}
}
