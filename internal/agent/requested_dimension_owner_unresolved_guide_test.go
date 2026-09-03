package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// V4-4 (colleague_merge_audit §40.22): the explorer guide discloses the
// unresolved-owner marker as typed index/role/path lines; a nil marker adds
// nothing. No internal type names reach the prompt (glossary lint).
func TestRenderExplorerRequestedDimensionEvidenceOwnershipGuideListsUnresolvedOwnership(t *testing.T) {
	ctx := requestedDimensionEvidenceOwnershipContext()
	ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{
		{Path: "config/load.go", Confidence: 0.95, RequestedDimensionIndices: []int{1}},
		{Path: "cmd/root.go", Confidence: 0.95},
	}
	ctx.AnalysisIR.RequestModel.AnalyzerHints.DimensionOwnerUnresolved = &types.DimensionOwnerUnresolved{
		DimensionIndices: []int{3}, UnclassifiedFiles: []string{"cmd/root.go"},
	}
	guide := renderExplorerRequestedDimensionEvidenceOwnershipGuide(ctx)
	for _, want := range []string{
		"Ownership not settled during analysis: index=3 (branch_behavior)",
		"Read the operation that implements it and set `requested_dimension_indices`",
		"Files listed without a declared role: cmd/root.go",
		"not as a settled owner",
	} {
		if !strings.Contains(guide, want) {
			t.Fatalf("explorer guide missing unresolved-owner disclosure %q:\n%s", want, guide)
		}
	}
	if strings.Contains(guide, "index=1 (") {
		t.Fatalf("owned dimension must not be reported as unresolved:\n%s", guide)
	}

	ctx.AnalysisIR.RequestModel.AnalyzerHints.DimensionOwnerUnresolved = nil
	if got := renderExplorerRequestedDimensionEvidenceOwnershipGuide(ctx); strings.Contains(got, "Ownership not settled") || strings.Contains(got, "without a declared role") {
		t.Fatalf("nil marker must add no disclosure:\n%s", got)
	}
}
