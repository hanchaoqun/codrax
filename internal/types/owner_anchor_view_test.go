package types

import "testing"

func TestOwnerAnchorViewFromSourceLocalizationReviewRanksOwnerAnchors(t *testing.T) {
	review := SourceLocalizationReview{
		Source: "read_turn_a",
		Anchors: []SourceLocalizationAnchor{{
			Path:     "pkg/observed.py",
			Kind:     SourceLocalizationAnchorReadFile,
			Strength: SourceLocalizationAnchorObserved,
		}, {
			Path:     "pkg/support.py",
			Kind:     SourceLocalizationAnchorEvidence,
			Strength: SourceLocalizationAnchorSupporting,
		}, {
			Path:         "pkg/owner.py",
			Kind:         SourceLocalizationAnchorGroundedEvidence,
			Strength:     SourceLocalizationAnchorOwner,
			OwnerSymbol:  "Owner",
			AnchorSymbol: "if",
			EvidenceRef: &WriteExplorationEvidenceRef{
				ID:          "ev-owner",
				Source:      "pkg/owner.py",
				LineStart:   12,
				OwnerSymbol: "Owner",
			},
		}, {
			Path:     "tests/test_owner.py",
			Kind:     SourceLocalizationAnchorGroundedEvidence,
			Strength: SourceLocalizationAnchorOwner,
		}},
	}

	view := OwnerAnchorViewFromSourceLocalizationReview(review, 0)
	if len(view.Items) != 4 {
		t.Fatalf("items = %d, want 4: %+v", len(view.Items), view.Items)
	}
	if view.Items[0].Path != "pkg/owner.py" || view.Items[0].Strength != SourceLocalizationAnchorOwner {
		t.Fatalf("first item = %+v, want owner anchor", view.Items[0])
	}
	if view.Items[0].OwnerSymbol != "Owner" || view.Items[0].AnchorSymbol != "if" {
		t.Fatalf("owner/member symbols not carried into view: %+v", view.Items[0])
	}
	if !view.HasOwner || !view.HasStrong {
		t.Fatalf("owner/strong flags missing: %+v", view)
	}
	if len(view.StrongPaths) != 2 || view.StrongPaths[0] != "pkg/owner.py" || view.StrongPaths[1] != "pkg/support.py" {
		t.Fatalf("strong paths = %+v, want production owner/support only", view.StrongPaths)
	}
}

func TestOwnerAnchorViewFromWriteContextPackUsesScopedConsumerView(t *testing.T) {
	pack := WriteContextPack{
		BatchID: "batch-1",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "localization_anchor",
			Text:        "path=pkg/owner.py strength=owner",
			SourceStage: "explore",
			BatchID:     "batch-1",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
			LocalizationAnchor: &SourceLocalizationAnchor{
				Path:         "pkg/owner.py",
				Kind:         SourceLocalizationAnchorGroundedEvidence,
				Strength:     SourceLocalizationAnchorOwner,
				OwnerSymbol:  "Owner",
				AnchorSymbol: "Owner.handle",
			},
		}, {
			Priority:    WriteContextP1,
			Kind:        "localization_anchor",
			Text:        "path=pkg/verifier.py strength=owner",
			SourceStage: "explore",
			BatchID:     "batch-1",
			Consumers:   []WriteContextConsumer{WriteConsumerVerifier},
			LocalizationAnchor: &SourceLocalizationAnchor{
				Path:     "pkg/verifier.py",
				Kind:     SourceLocalizationAnchorGroundedEvidence,
				Strength: SourceLocalizationAnchorOwner,
			},
		}, {
			Priority:    WriteContextP1,
			Kind:        "scope_anchor",
			Text:        "pkg/scope",
			SourceStage: "write_analysis",
			BatchID:     "batch-2",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}},
	}

	view := OwnerAnchorViewFromWriteContextPack(pack, WriteConsumerPlanner, "batch-1", "", 0)
	if len(view.Items) != 1 {
		t.Fatalf("items = %+v, want only planner-visible batch-1 anchor", view.Items)
	}
	if view.Items[0].Path != "pkg/owner.py" || view.Items[0].AnchorSymbol != "Owner.handle" {
		t.Fatalf("planner anchor = %+v", view.Items[0])
	}
	if view.Items[0].OwnerSymbol != "Owner" {
		t.Fatalf("planner owner symbol missing: %+v", view.Items[0])
	}
}

func TestOwnerAnchorCandidatePathsPreferOwnerEvidenceBeforeFallback(t *testing.T) {
	ref := WriteExplorationEvidenceRef{ID: "ev-owner", Source: "pkg/owner.py", LineStart: 12, OwnerSymbol: "Owner.handle"}
	view := NormalizeOwnerAnchorView(OwnerAnchorView{Items: []OwnerAnchorViewItem{{
		Path:        "pkg/scope.py",
		Kind:        SourceLocalizationAnchorScope,
		Strength:    SourceLocalizationAnchorSupporting,
		Priority:    WriteContextP1,
		SourceStage: "write_analysis",
	}, {
		Path:        "pkg/support.py",
		Kind:        SourceLocalizationAnchorEvidence,
		Strength:    SourceLocalizationAnchorSupporting,
		Priority:    WriteContextP1,
		SourceStage: "explore",
		Subject:     "Support.boundary",
	}, {
		Path:        "pkg/owner.py",
		Kind:        SourceLocalizationAnchorGroundedEvidence,
		Strength:    SourceLocalizationAnchorOwner,
		Priority:    WriteContextP1,
		SourceStage: "explore",
		OwnerSymbol: "Owner.handle",
		EvidenceRef: &ref,
	}}}, 0)

	paths := OwnerAnchorCandidatePaths(view, []string{"pkg/symptom.py", "pkg/owner.py"}, 0)
	want := []string{"pkg/owner.py", "pkg/support.py", "pkg/symptom.py"}
	if len(paths) != len(want) {
		t.Fatalf("paths = %+v, want %+v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths = %+v, want %+v", paths, want)
		}
	}
	reqs := OwnerAnchorEvidenceRequirements(view, 0)
	if len(reqs) != 2 {
		t.Fatalf("requirements = %+v, want owner/support only", reqs)
	}
	if reqs[0] != "owner_anchor path=pkg/owner.py owner_symbol=Owner.handle evidence_ref=ev-owner" {
		t.Fatalf("owner requirement = %q", reqs[0])
	}
	if reqs[1] != "owner_anchor path=pkg/support.py subject=Support.boundary" {
		t.Fatalf("support requirement = %q", reqs[1])
	}
}

func TestStampChangePlanOwnerAnchorsUsesLocalizationReview(t *testing.T) {
	plan := &ChangePlan{
		ID: "plan-owner",
		LocalizationReview: &SourceLocalizationReview{
			Source:  "write_plan_context",
			PlanID:  "plan-owner",
			BatchID: "batch-1",
			Anchors: []SourceLocalizationAnchor{{
				Path:         "pkg/owner.py",
				Kind:         SourceLocalizationAnchorGroundedEvidence,
				Strength:     SourceLocalizationAnchorOwner,
				Subject:      "Owner.handle",
				OwnerSymbol:  "Owner",
				AnchorSymbol: "if",
				EvidenceRef: &WriteExplorationEvidenceRef{
					ID:          "ev-owner",
					Source:      "pkg/owner.py",
					LineStart:   12,
					OwnerSymbol: "Owner",
				},
			}},
		},
	}

	StampChangePlanOwnerAnchors(plan, 12)
	view := OwnerAnchorViewFromChangePlan(plan, 12)
	if len(plan.OwnerAnchors) != 1 || len(view.Items) != 1 {
		t.Fatalf("owner anchors not stamped: plan=%+v view=%+v", plan.OwnerAnchors, view.Items)
	}
	if plan.OwnerAnchors[0].ID == "" || plan.OwnerAnchors[0].OwnerSymbol != "Owner" || plan.OwnerAnchors[0].AnchorSymbol != "if" {
		t.Fatalf("stamped anchor lost owner/member identity: %+v", plan.OwnerAnchors[0])
	}
}
