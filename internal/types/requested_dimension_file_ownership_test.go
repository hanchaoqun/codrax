package types

import "testing"

func TestRequestedExplanationOperationNeedsUsesOnlyExplicitHighConfidenceFileBindings(t *testing.T) {
	profile := &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []RequestedAnswerDimension{
		{Index: 1, Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		{Index: 2, Role: RequestedAnswerDimensionObservedValue, Required: true},
		{Index: 3, Role: RequestedAnswerDimensionBranchBehavior, Required: true},
	}}
	hints := []RequiredFileHint{
		{Path: "./config/load.go", Confidence: 0.95, RequestedDimensionIndices: []int{1}},
		{Path: "cmd\\root.go", Confidence: 0.9, RequestedDimensionIndices: []int{3}},
		{Path: "noise.go", Confidence: 0.7, RequestedDimensionIndices: []int{1}},
	}
	needs := RequestedExplanationOperationNeeds(profile, hints)
	if len(needs) != 2 || needs[0].Dimension.Index != 1 || needs[0].Source != "config/load.go" ||
		needs[1].Dimension.Index != 3 || needs[1].Source != "cmd/root.go" {
		t.Fatalf("needs=%+v", needs)
	}

	wrongFile := EvidenceItem{Kind: EvidenceMechanism, Source: "cmd/root.go", GroundingStatus: GroundingGrounded, RequestedDimensionIndices: []int{1}}
	if RequestedExplanationOperationNeedCovered(needs[0], wrongFile) {
		t.Fatal("sibling-file operation must not close the config/load.go seat")
	}
	rightFile := wrongFile
	rightFile.Source = "./config/load.go"
	if !RequestedExplanationOperationNeedCovered(needs[0], rightFile) {
		t.Fatal("exact file-scoped grounded operation should close its seat")
	}
}

func TestRequestedExplanationOperationNeedsPreservesLegacyUnscopedBehavior(t *testing.T) {
	profile := &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []RequestedAnswerDimension{
		{Index: 1, Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		{Index: 3, Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true},
	}}
	needs := RequestedExplanationOperationNeeds(profile, []RequiredFileHint{{Path: "nav.go", Confidence: 1}})
	if len(needs) != 2 || needs[0].Source != "" || needs[1].Source != "" {
		t.Fatalf("navigation-only required files must preserve unscoped operation seats: %+v", needs)
	}
}
