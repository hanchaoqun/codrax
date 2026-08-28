package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderRequestedDimensionEvidenceOwnershipUsesTypedIndices(t *testing.T) {
	mut := types.NewMutableState("dimension evidence ownership")
	mut.AppendEvidence([]types.EvidenceItem{
		{ID: "E1", Kind: types.EvidenceMechanism, Source: "config/load.go", LineStart: 12, AnchorKind: types.AnchorCall, Subject: "Load", Object: "Decode", GroundingStatus: types.GroundingGrounded, RequestedDimensionIndices: []int{1}},
		{ID: "E2", Kind: types.EvidenceConditional, Source: "cmd/root.go", LineStart: 20, AnchorKind: types.AnchorCondition, Subject: "resolve", GroundingStatus: types.GroundingGrounded, RequestedDimensionIndices: []int{3}},
	})
	ctx := &types.AgentContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{
			{Index: 1, Label: "parser", Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
			{Index: 3, Label: "override", Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		}},
	}}}
	prompt := renderAnswerDocRequestedDimensionEvidenceOwnership(ctx)
	for _, want := range []string{"Dimension 1", "config/load.go:12", "Dimension 3", "cmd/root.go:20", "does not cross-satisfy", "does not author the conclusion"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("ownership prompt missing %q:\n%s", want, prompt)
		}
	}
}
