package types

import (
	"strings"
	"testing"
)

func TestSourceLocalizationReviewFromTurnAClassifiesSourceAndAuxiliaryPaths(t *testing.T) {
	review := SourceLocalizationReviewFromTurnA(
		[]string{"src/app.py", "tests/test_app.py", "README.md"},
		[]EvidenceItem{{
			ID:           "ev-1",
			Kind:         EvidenceDirect,
			Source:       "src/owner.py",
			LineStart:    12,
			LineEnd:      18,
			Subject:      "Owner",
			AnchorSymbol: "Owner.handle",
		}},
	)

	if review.Status != SourceLocalizationObserved {
		t.Fatalf("status = %s, want observed: %+v", review.Status, review)
	}
	if strings.Join(review.SourcePaths, ",") != "src/app.py,src/owner.py" {
		t.Fatalf("source paths = %+v", review.SourcePaths)
	}
	if strings.Join(review.AuxiliaryPaths, ",") != "README.md,tests/test_app.py" {
		t.Fatalf("auxiliary paths = %+v", review.AuxiliaryPaths)
	}
	if len(review.EvidenceRefs) != 1 || review.EvidenceRefs[0].Source != "src/owner.py" {
		t.Fatalf("evidence refs = %+v", review.EvidenceRefs)
	}
	if !sourceLocalizationTestHasAnchor(review, "src/owner.py", SourceLocalizationAnchorEvidence, SourceLocalizationAnchorSupporting) {
		t.Fatalf("supporting evidence anchor missing: %+v", review.Anchors)
	}
	if !sourceLocalizationTestHasAnchor(review, "src/app.py", SourceLocalizationAnchorReadFile, SourceLocalizationAnchorObserved) {
		t.Fatalf("read-file observed anchor missing: %+v", review.Anchors)
	}
}

func TestSourceLocalizationReviewFromTurnAGroundedEvidenceCreatesOwnerAnchor(t *testing.T) {
	review := SourceLocalizationReviewFromTurnA(
		[]string{"src/candidate.py", "tests/test_candidate.py"},
		[]EvidenceItem{{
			ID:              "ev-owner",
			Kind:            EvidenceMechanism,
			Source:          "src/owner.py",
			LineStart:       24,
			LineEnd:         28,
			Subject:         "Owner.handle",
			AnchorSymbol:    "Owner.handle",
			GroundingStatus: GroundingGrounded,
			Scope:           ScopeLine,
		}, {
			ID:              "ev-test",
			Kind:            EvidenceDirect,
			Source:          "tests/test_owner.py",
			LineStart:       9,
			GroundingStatus: GroundingGrounded,
			Scope:           ScopeLine,
		}},
	)

	if !sourceLocalizationTestHasAnchor(review, "src/owner.py", SourceLocalizationAnchorGroundedEvidence, SourceLocalizationAnchorOwner) {
		t.Fatalf("owner evidence anchor missing: %+v", review.Anchors)
	}
	if sourceLocalizationTestHasAnchor(review, "tests/test_owner.py", SourceLocalizationAnchorGroundedEvidence, SourceLocalizationAnchorOwner) {
		t.Fatalf("auxiliary test evidence must not become owner anchor: %+v", review.Anchors)
	}
	if !strings.Contains(strings.Join(review.ReasonCodes, ","), "read_turn_a_owner_anchor_observed") {
		t.Fatalf("owner-anchor reason missing: %+v", review.ReasonCodes)
	}
}

func TestSourceLocalizationReviewFromWritePlanContextSupportedAndMissing(t *testing.T) {
	prior := []WriteContextPack{{
		PackID:      "write-analysis",
		SourceStage: "write_analysis",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "scope_anchor",
			Text:        "pkg/owner.py",
			SourceStage: "write_analysis",
		}, {
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "pkg/covered",
			SourceStage: "explore",
		}},
	}}
	plan := &ChangePlan{
		ID:          "plan-1",
		TargetPaths: []string{"pkg/owner.py", "pkg/caller.py", "pkg/covered/helper.py", "tests/test_owner.py"},
		Changes: []FileChange{
			{Path: "pkg/owner.py", Kind: "modify"},
			{Path: "pkg/caller.py", Kind: "modify"},
			{Path: "pkg/covered/helper.py", Kind: "modify"},
			{Path: "tests/test_owner.py", Kind: "modify"},
		},
	}

	review := SourceLocalizationReviewFromWritePlanContext("batch-1", "repair", prior, plan)
	if review.Status != SourceLocalizationWeak {
		t.Fatalf("status = %s, want weak: %+v", review.Status, review)
	}
	if strings.Join(review.SupportedPaths, ",") != "pkg/covered/helper.py,pkg/owner.py" {
		t.Fatalf("supported paths = %+v", review.SupportedPaths)
	}
	if strings.Join(review.MissingPaths, ",") != "pkg/caller.py" {
		t.Fatalf("missing paths = %+v", review.MissingPaths)
	}
	if review.SupportRatio != 2.0/3.0 {
		t.Fatalf("support ratio = %v", review.SupportRatio)
	}
}

func TestSourceLocalizationReviewFromWritePlanContextNoPriorIsWeakNotRoutingGrade(t *testing.T) {
	plan := &ChangePlan{
		ID:          "plan-1",
		TargetPaths: []string{"pkg/new_owner.py", "tests/test_new_owner.py"},
		Changes: []FileChange{
			{Path: "pkg/new_owner.py", Kind: "modify"},
			{Path: "tests/test_new_owner.py", Kind: "modify"},
		},
	}

	review := SourceLocalizationReviewFromWritePlanContext("batch-1", "repair", nil, plan)
	if review.Status != SourceLocalizationWeak {
		t.Fatalf("status = %s, want weak: %+v", review.Status, review)
	}
	if strings.Join(review.MissingPaths, ",") != "pkg/new_owner.py" {
		t.Fatalf("missing paths = %+v", review.MissingPaths)
	}
	if got := WritePlanSourcePathsOutsidePriorContext(nil, plan); len(got) != 0 {
		t.Fatalf("no-prior review is audit signal only; routing helper returned %+v", got)
	}
}

func sourceLocalizationTestHasAnchor(review SourceLocalizationReview, path string, kind SourceLocalizationAnchorKind, strength SourceLocalizationAnchorStrength) bool {
	for _, anchor := range review.Anchors {
		if anchor.Path == path && anchor.Kind == kind && anchor.Strength == strength {
			return true
		}
	}
	return false
}
