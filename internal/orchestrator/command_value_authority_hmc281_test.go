package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC281SemanticReviewerKeepsCommandLabelBoundary(t *testing.T) {
	facts := []types.AnswerAggregateFact{{Kind: types.AnswerAggregateScalar,
		Role: types.AnswerAggregateRolePrincipalAnswer, Label: "derived physical relation", Value: "7",
		Dimensions: []types.AnswerAggregateDimension{{Name: "origin", Value: "command_measurement"}}}}
	bindings := types.CompileAnswerClaimBindingsFromAggregateFacts(facts, nil, nil)
	prompt := renderSemanticQualityUserMessage(SemanticQualityInput{ClaimBindings: semanticClaimBindingSummaries(bindings)})
	for _, want := range []string{"model_label=\"derived physical relation\"", "value_support=`reported_command_value`", "policy=`soft`", "authority_ceiling=`illustrative`"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("reviewer lost shared value-versus-label boundary %q:\n%s", want, prompt)
		}
	}
	ordinary := semanticClaimBindingSummaries([]types.AnswerClaimBinding{{
		Origin: types.AnswerEvidenceOriginVCSMetadata, TargetRef: "recorded change",
		GroundingPolicy: types.ClaimGroundingRepairable, AuthorityCeiling: types.AuthorityHistorical,
	}})
	unchanged := renderSemanticQualityUserMessage(SemanticQualityInput{ClaimBindings: ordinary})
	if !strings.Contains(unchanged, "target=\"recorded change\"") || strings.Contains(unchanged, "value_support=") || strings.Contains(unchanged, "model_label=") {
		t.Fatalf("non-command historical observation was relabelled:\n%s", unchanged)
	}
}
