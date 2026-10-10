package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTypedErrorGranularityRequestOwnershipContractParity(t *testing.T) {
	rm := types.RequestModel{
		Intent:                    types.IntentExplain,
		ErrorGranularityProfile:   &types.ErrorGranularityProfile{IsGranularityQuestion: true, SourceQuotes: []string{"show recorded events"}, RequestedVerdictOptions: []types.ErrorGranularityVerdict{types.ErrorGranularityPerItemRejection}},
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Label: "events", Role: types.RequestedAnswerDimensionMemberSet, SourceQuote: "show recorded events", Required: true}}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{Kind: types.BlockSummary, Text: "Recorded events and their provenance."}}}
	if got := runTypedErrorGranularityProfileCheck(doc, &rm); len(got) != 0 {
		t.Fatalf("post-contract resurrected the softened failure-scope duty: %+v", got)
	}
	rm.ErrorGranularityProfile.SourceQuotes = []string{"does one invalid record reject the batch"}
	if got := runTypedErrorGranularityProfileCheck(doc, &rm); len(got) != 1 {
		t.Fatalf("independent scope question lost its required verdict: %+v", got)
	}
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{Kind: types.BlockDecision, SurfaceRole: types.SurfacePrincipal, ErrorGranularityVerdict: types.ErrorGranularityWholeBatch})
	if got := runTypedErrorGranularityProfileCheck(doc, &rm); len(got) != 0 {
		t.Fatalf("single proposition incorrectly rejected evidence-supported negative: %+v", got)
	}
	rm.ErrorGranularityProfile.RequestedVerdictOptions = []types.ErrorGranularityVerdict{types.ErrorGranularityPerItemRejection, types.ErrorGranularityFailFast}
	if got := runTypedErrorGranularityProfileCheck(doc, &rm); len(got) != 1 {
		t.Fatalf("explicit contrast no longer enforces its existing candidate set: %+v", got)
	}
}
