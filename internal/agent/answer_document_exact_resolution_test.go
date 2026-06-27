package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestScoreExactResolutionEvidence_IgnoresSummaryOnlyTargetMention(t *testing.T) {
	contract := &types.ExactResolutionContract{
		TargetKind:           types.SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	summaryOnly := types.EvidenceItem{
		Kind:            types.EvidenceDirect,
		Source:          "internal/skill/glossary.go",
		LineStart:       32,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "ProjectSpecificIdentifierBlocklist",
		ContextRole:     types.EvidenceContextRoleRelatedContext,
		GroundingStatus: types.GroundingGrounded,
		Summary:         "Prompt docs mention explore_mid_loop_hint_budget as an example identifier.",
	}
	structural := summaryOnly
	structural.Snippet = `const key = "explore_mid_loop_hint_budget"`

	summaryScore := scoreExactResolutionEvidence(summaryOnly, contract, types.ExactResolutionContextTerms(contract), true)
	structuralScore := scoreExactResolutionEvidence(structural, contract, types.ExactResolutionContextTerms(contract), true)
	if summaryScore > 0 {
		t.Fatalf("summary-only exact-target mention should not earn a positive score, got %d", summaryScore)
	}
	if structuralScore <= summaryScore {
		t.Fatalf("structural exact-target mention should outrank summary-only mention, structural=%d summary=%d", structuralScore, summaryScore)
	}
}
