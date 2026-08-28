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

func TestRenderExplorerRequestedDimensionEvidenceOwnershipGuideUsesTypedRoster(t *testing.T) {
	ctx := requestedDimensionEvidenceOwnershipContext()
	ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{
		{Path: "config/load.go", Confidence: 0.95, RequestedDimensionIndices: []int{1}},
		{Path: "cmd/root.go", Confidence: 0.95, RequestedDimensionIndices: []int{3}},
	}
	guide := renderExplorerRequestedDimensionEvidenceOwnershipGuide(ctx)
	for _, want := range []string{
		"index=1 role=function_or_purpose",
		"index=3 role=branch_behavior",
		"index=1 source=config/load.go",
		"index=3 source=cmd/root.go",
		"exact source",
		"requested_dimension_indices",
		"aggregate_facts",
		"does not choose, phrase, or rewrite the answer conclusion",
	} {
		if !strings.Contains(guide, want) {
			t.Fatalf("explorer ownership guide missing %q:\n%s", want, guide)
		}
	}
	if strings.Contains(guide, "index=2") {
		t.Fatalf("non-operation impact dimension leaked into ownership guide:\n%s", guide)
	}
}

func TestRenderExplorerRequestedDimensionEvidenceOwnershipGuideIgnoresRuntimeAndSingleShapes(t *testing.T) {
	for _, dimensions := range [][]types.RequestedAnswerDimension{
		{{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true}},
		{
			{Index: 1, Role: types.RequestedAnswerDimensionCausalAttribution, Required: true},
			{Index: 2, Role: types.RequestedAnswerDimensionCausalContributorSet, Required: true},
			{Index: 3, Role: types.RequestedAnswerDimensionObservedValue, Required: true},
		},
	} {
		ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: dimensions},
		}}}
		if got := renderExplorerRequestedDimensionEvidenceOwnershipGuide(ctx); got != "" {
			t.Fatalf("unrelated shape received ownership guide:\n%s", got)
		}
	}
}

func TestExplorerBuildInitialInstructionTeachesDimensionOwnershipBeforeCompletion(t *testing.T) {
	ctx := requestedDimensionEvidenceOwnershipContext()
	ctx.Objective = "explain two independently requested config behaviors"
	prompt := (&explorerEvaluator{}).BuildInitialInstruction(ctx, nil)
	guideAt := strings.Index(prompt, "### Requested Explanation Evidence Ownership")
	completionAt := strings.Index(prompt, "### Completion Handoff")
	if guideAt < 0 || completionAt < 0 || guideAt > completionAt {
		t.Fatalf("ownership guide must precede completion handoff: guide=%d completion=%d\n%s", guideAt, completionAt, prompt)
	}
	if got := strings.Count(prompt, "### Requested Explanation Evidence Ownership"); got != 1 {
		t.Fatalf("ownership guide count=%d want 1", got)
	}
}

func requestedDimensionEvidenceOwnershipContext() *types.AgentContext {
	return &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{
			{Index: 1, Label: "parser", Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
			{Index: 2, Label: "impact", Role: types.RequestedAnswerDimensionImpact, Required: true},
			{Index: 3, Label: "override", Role: types.RequestedAnswerDimensionBranchBehavior, Required: true},
		}},
	}}}
}
