package types

import (
	"testing"
)


func TestCollectDriftBoundedSurfaceItems_PrefersInnerCompanionDetail(t *testing.T) {
	observed := []LogSourceDriftAnchor{
		{File: "internal/agent/analyzer.go", Func: "buildAnalysisIR"},
		{File: "internal/agent/analyzer.go", Func: "ParseOutput"},
	}
	items := []EvidenceItem{
		{
			Source:       "internal/agent/analyzer.go",
			LineStart:    674,
			AnchorKind:   AnchorCall,
			AnchorSymbol: "buildAnalysisIR",
			Subject:      "ParseOutput",
			Object:       "buildAnalysisIR",
		},
		{
			Source:       "internal/agent/analyzer.go",
			LineStart:    883,
			AnchorKind:   AnchorDefinition,
			AnchorSymbol: "buildAnalysisIR",
		},
		{
			Source:       "internal/agent/analyzer.go",
			LineStart:    884,
			AnchorKind:   AnchorCondition,
			AnchorSymbol: "buildAnalysisIR",
			Condition:    "ctx == nil || ctx.Mutable == nil",
		},
		{
			Source:       "internal/agent/analyzer.go",
			LineStart:    887,
			AnchorKind:   AnchorAssignment,
			AnchorSymbol: "buildAnalysisIR",
			Subject:      "raw",
			Object:       "ctx.Mutable.RequestModel()",
		},
	}

	got := CollectDriftBoundedSurfaceItems(observed, nil, items)
	if len(got) < 3 {
		t.Fatalf("CollectDriftBoundedSurfaceItems() len=%d, want >= 3 items with companion detail", len(got))
	}
	if got[0].AnchorKind != AnchorCall {
		t.Fatalf("first drift-bounded item = %s, want call edge", got[0].AnchorKind)
	}
	if got[1].AnchorKind != AnchorCondition {
		t.Fatalf("second drift-bounded item = %s, want nearest inner condition", got[1].AnchorKind)
	}
	if got[2].AnchorKind != AnchorAssignment {
		t.Fatalf("third drift-bounded item = %s, want companion assignment detail", got[2].AnchorKind)
	}
	if got[2].LineStart != 887 {
		t.Fatalf("companion detail line=%d, want 887", got[2].LineStart)
	}
}
