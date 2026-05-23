package types

import "testing"

func TestNormalizeRequestedAnswerDimensionProfile_PreservesAnchoredDimensions(t *testing.T) {
	raw := "请说明每次提交的 diff 线索、当前关键代码、作用和影响，不要只给 commit id。"
	profile, warnings := NormalizeRequestedAnswerDimensionProfile(raw, &RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Confidence:          0.8,
		Dimensions: []RequestedAnswerDimension{
			{Label: "当前关键代码", Role: RequestedAnswerDimensionCurrentKeyCode, Required: true, Index: 2},
			{Label: "diff 线索", Role: RequestedAnswerDimensionDiffClue, Required: true, Index: 1},
			{Label: "diff 线索", Role: RequestedAnswerDimensionDiffClue, Required: true, Index: 9},
			{Label: "invented axis", Role: RequestedAnswerDimensionImpact, Required: true, Index: 3},
			{Label: "影响", Role: RequestedAnswerDimensionImpact, Required: true, Index: 4},
		},
	})
	if profile == nil || !profile.Active() {
		t.Fatalf("profile should survive: profile=%+v warnings=%v", profile, warnings)
	}
	if len(profile.Dimensions) != 3 {
		t.Fatalf("dimensions=%d want 3: %+v warnings=%v", len(profile.Dimensions), profile.Dimensions, warnings)
	}
	if profile.Dimensions[0].Label != "diff 线索" || profile.Dimensions[1].Label != "当前关键代码" || profile.Dimensions[2].Label != "影响" {
		t.Fatalf("order/labels not preserved: %+v", profile.Dimensions)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warning for unanchored dimension")
	}
}

func TestCompileAnswerPresentationContract_CarriesRequestedDimensions(t *testing.T) {
	profile := &RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Dimensions: []RequestedAnswerDimension{
			{Label: "diff 线索", Role: RequestedAnswerDimensionDiffClue, Index: 1},
			{Label: "当前关键代码", Role: RequestedAnswerDimensionCurrentKeyCode, Index: 2},
		},
	}
	ir := &AnalysisIR{RequestModel: RequestModel{RequestedAnswerDimensions: profile}}
	view := BuildAnswerSemanticView(ir, nil)
	if view == nil {
		t.Fatal("view is nil")
	}
	if len(view.Presentation.RequestedDimensions) != 2 {
		t.Fatalf("presentation dimensions=%d want 2: %+v", len(view.Presentation.RequestedDimensions), view.Presentation.RequestedDimensions)
	}
}
