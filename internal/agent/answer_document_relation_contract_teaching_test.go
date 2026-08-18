package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAnswerDocCallChainSubmissionChecklistUsesSharedRelationOwnershipContract(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{}}
	view := &types.AnswerSemanticView{
		Family: types.QFCallChain,
		RequiredBlocks: []types.BlockRequirement{{
			Kind:            types.BlockOrderedList,
			Required:        true,
			SurfaceRoleHint: types.SurfacePrincipal,
		}},
	}

	got := renderAnswerDocSubmissionChecklist(ctx, view, false)
	if !strings.Contains(got, types.GroundedStandaloneCallChainRelationOwnershipContract) {
		t.Fatalf("call-chain checklist drifted from the shared relation contract:\n%s", got)
	}
	for _, want := range []string{
		"When the answer also contains a diagram",
		"exact from_node/to_node identifiers",
		"when no diagram exists",
		"reader-facing endpoint labels",
	} {
		if !strings.Contains(types.GroundedStandaloneCallChainRelationOwnershipContract, want) {
			t.Fatalf("shared relation contract missing carrier distinction %q:\n%s", want, types.GroundedStandaloneCallChainRelationOwnershipContract)
		}
	}
	if strings.Contains(types.GroundedStandaloneCallChainRelationOwnershipContract, "Every standalone row") {
		t.Fatalf("shared relation contract retained unconditional standalone-label wording:\n%s", types.GroundedStandaloneCallChainRelationOwnershipContract)
	}
}
