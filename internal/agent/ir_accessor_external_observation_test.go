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
	if originSpecificObservationWithoutRequiredSourceForExplorer(ctx, facts) {
		t.Fatal("typed current-source request must keep source completion required")
	}
}
