package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCollectExactResolutionSymbolCandidatesFromGraph_IncludesStrongSameFamilyDefaultSymbol(t *testing.T) {
	contract := &types.ExactResolutionContract{
		TargetKind:           types.SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	fileSymbols := map[string][]string{
		"internal/config/runtime.go": {
			"ExploreMidLoopMinIteration internal/config/runtime.go:231",
			"ExploreMidLoopEnumCoverage internal/config/runtime.go:235",
		},
		"internal/types/config.go": {
			"DefaultExploreHeuristics internal/types/config.go:707",
			"ExploreSettings internal/types/config.go:618",
		},
	}
	evidence := []types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/config/runtime.go",
		LineStart:       231,
		AnchorKind:      types.AnchorAssignment,
		AnchorSymbol:    "ExploreMidLoopMinIteration",
		ContextRole:     types.EvidenceContextRoleRelatedContext,
		GroundingStatus: types.GroundingGrounded,
	}}

	cands := collectExactResolutionSymbolCandidatesFromGraph(nil, contract, nil, fileSymbols, evidence)
	if len(cands) == 0 {
		t.Fatalf("expected same-family candidates, got none")
	}
	var sawDefault bool
	for _, cand := range cands {
		if cand.Symbol == "DefaultExploreHeuristics" {
			sawDefault = true
			break
		}
	}
	if !sawDefault {
		t.Fatalf("expected DefaultExploreHeuristics candidate, got %+v", cands)
	}
}
