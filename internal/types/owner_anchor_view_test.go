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
			AnchorSymbol: "Owner.handle",
			EvidenceRef: &WriteExplorationEvidenceRef{
				ID:        "ev-owner",
				Source:    "pkg/owner.py",
				LineStart: 12,
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
}
