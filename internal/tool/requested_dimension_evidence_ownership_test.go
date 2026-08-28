package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func dimensionOwnershipContext(dims ...types.RequestedAnswerDimension) *types.BusContext {
	return &types.BusContext{
		Mutable: types.NewMutableState("dimension ownership"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions:          dims,
			},
		}},
	}
}

func TestRequestedDimensionEvidenceOwnershipRequiresIndependentOperationRows(t *testing.T) {
	ctx := dimensionOwnershipContext(
		types.RequestedAnswerDimension{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		types.RequestedAnswerDimension{Index: 3, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
	)
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceRegistration, GroundingStatus: types.GroundingGrounded,
		RequestedDimensionIndices: []int{3},
	}}
	got := requestedDimensionEvidenceOwnershipDowngrade(ctx, evidence)
	if !strings.Contains(got, "1 (function_or_purpose)") || strings.Contains(got, "3 (function_or_purpose)") {
		t.Fatalf("downgrade=%q", got)
	}

	evidence = append(evidence, types.EvidenceItem{
		Kind: types.EvidenceMechanism, GroundingStatus: types.GroundingGrounded,
		RequestedDimensionIndices: []int{1},
	})
	if got := requestedDimensionEvidenceOwnershipDowngrade(ctx, evidence); got != "" {
		t.Fatalf("independently supported dimensions should close, got %q", got)
	}
}

func TestRequestedDimensionEvidenceOwnershipDoesNotTreatDefinitionAsMechanism(t *testing.T) {
	ctx := dimensionOwnershipContext(
		types.RequestedAnswerDimension{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		types.RequestedAnswerDimension{Index: 2, Role: types.RequestedAnswerDimensionBranchBehavior, Required: true},
	)
	evidence := []types.EvidenceItem{
		{Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, GroundingStatus: types.GroundingGrounded, RequestedDimensionIndices: []int{1}},
		{Kind: types.EvidenceConditional, AnchorKind: types.AnchorCondition, GroundingStatus: types.GroundingGrounded, RequestedDimensionIndices: []int{2}},
	}
	if got := requestedDimensionEvidenceOwnershipDowngrade(ctx, evidence); !strings.Contains(got, "1 (function_or_purpose)") {
		t.Fatalf("identity-only definition must not close a mechanism dimension: %q", got)
	}
}

func TestRequestedDimensionEvidenceOwnershipLeavesSingleExplanationOnExistingFloor(t *testing.T) {
	ctx := dimensionOwnershipContext(
		types.RequestedAnswerDimension{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
	)
	if got := requestedDimensionEvidenceOwnershipDowngrade(ctx, nil); got != "" {
		t.Fatalf("single explanation must keep existing floor contract, got %q", got)
	}
}
