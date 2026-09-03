package orchestrator

import (
	"errors"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// V4-4 (colleague_merge_audit §40.22): the degraded-recovery IR rebuild
// copies RequiredFileHints; the unresolved-owner marker that describes those
// hints must ride with them, or degraded recovery silently hides the
// ownership gap from the explorer guide.
func TestBuildDegradedSemanticIR_PreservesDimensionOwnerUnresolved(t *testing.T) {
	partial := &types.AnalysisIR{}
	partial.RequestModel.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{
		{Path: "config/load.go", Confidence: 0.95, RequestedDimensionIndices: []int{1}},
	}
	partial.RequestModel.AnalyzerHints.DimensionOwnerUnresolved = &types.DimensionOwnerUnresolved{
		DimensionIndices: []int{2}, UnclassifiedFiles: []string{"cmd/root.go"},
	}
	got := buildDegradedSemanticIR("q", partial, errors.New("test"))
	marker := got.RequestModel.AnalyzerHints.DimensionOwnerUnresolved
	if !reflect.DeepEqual(marker, partial.RequestModel.AnalyzerHints.DimensionOwnerUnresolved) {
		t.Fatalf("marker not preserved: %+v", marker)
	}
	if marker == partial.RequestModel.AnalyzerHints.DimensionOwnerUnresolved {
		t.Fatal("marker must be copied, not aliased to the partial IR")
	}
	if nilCase := buildDegradedSemanticIR("q", &types.AnalysisIR{}, errors.New("test")); nilCase.RequestModel.AnalyzerHints.DimensionOwnerUnresolved != nil {
		t.Fatalf("absent marker must stay nil: %+v", nilCase.RequestModel.AnalyzerHints.DimensionOwnerUnresolved)
	}
}
