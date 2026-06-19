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
			OwnerSymbol:     "Owner",
			AnchorSymbol:    "if",
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
	ownerAnchor := sourceLocalizationTestAnchor(review, "src/owner.py", SourceLocalizationAnchorGroundedEvidence, SourceLocalizationAnchorOwner)
	if ownerAnchor == nil || ownerAnchor.OwnerSymbol != "Owner" || ownerAnchor.AnchorSymbol != "if" {
		t.Fatalf("owner/member symbols not preserved: %+v", ownerAnchor)
	}
	if len(review.EvidenceRefs) == 0 || review.EvidenceRefs[0].OwnerSymbol != "Owner" {
		t.Fatalf("owner symbol not preserved on evidence refs: %+v", review.EvidenceRefs)
	}
	if sourceLocalizationTestHasAnchor(review, "tests/test_owner.py", SourceLocalizationAnchorGroundedEvidence, SourceLocalizationAnchorOwner) {
		t.Fatalf("auxiliary test evidence must not become owner anchor: %+v", review.Anchors)
	}
	if !strings.Contains(strings.Join(review.ReasonCodes, ","), "read_turn_a_owner_anchor_observed") {
		t.Fatalf("owner-anchor reason missing: %+v", review.ReasonCodes)
	}
}

func TestSourceLocalizationReviewFromTurnAInfersOwnerFromSourcePrefixedSubject(t *testing.T) {
	review := SourceLocalizationReviewFromTurnA(
		nil,
		[]EvidenceItem{{
			ID:              "ev-owner-subject",
			Kind:            EvidenceDirect,
			Source:          "pkg/owner.py",
			LineStart:       42,
			Subject:         "pkg/owner.py:Owner.handle",
			AnchorSymbol:    "if",
			GroundingStatus: GroundingGrounded,
			Scope:           ScopeLine,
		}},
	)

	ownerAnchor := sourceLocalizationTestAnchor(review, "pkg/owner.py", SourceLocalizationAnchorGroundedEvidence, SourceLocalizationAnchorOwner)
	if ownerAnchor == nil {
		t.Fatalf("owner anchor missing: %+v", review.Anchors)
	}
	if ownerAnchor.OwnerSymbol != "Owner.handle" || ownerAnchor.AnchorSymbol != "if" {
		t.Fatalf("source-prefixed subject owner not inferred: %+v", ownerAnchor)
	}
	if len(review.EvidenceRefs) != 1 || review.EvidenceRefs[0].OwnerSymbol != "Owner.handle" {
		t.Fatalf("evidence ref owner not inferred: %+v", review.EvidenceRefs)
	}
}

func TestSourceLocalizationReviewFromTurnADeterministicConcreteValueNeedsDefiningRoleForAnchor(t *testing.T) {
	review := SourceLocalizationReviewFromTurnA(
		[]string{"src/owner.py"},
		[]EvidenceItem{{
			ID:        "ev-concrete",
			Kind:      EvidenceConcrete,
			Source:    "src/other.py",
			LineStart: 40,
			Subject:   "Other.value",
			Scope:     ScopeLine,
		}, {
			ID:          "ev-defining",
			Kind:        EvidenceConcrete,
			Source:      "src/defined.py",
			LineStart:   50,
			Subject:     "Defined.value",
			Scope:       ScopeLine,
			ContextRole: EvidenceContextRoleDefining,
		}},
	)

	if sourceLocalizationTestHasAnchor(review, "src/other.py", SourceLocalizationAnchorEvidence, SourceLocalizationAnchorSupporting) {
		t.Fatalf("broad deterministic concrete value must not become localization anchor: %+v", review.Anchors)
	}
	if !sourceLocalizationTestHasAnchor(review, "src/defined.py", SourceLocalizationAnchorEvidence, SourceLocalizationAnchorSupporting) {
		t.Fatalf("typed defining deterministic evidence should remain eligible: %+v", review.Anchors)
	}
	if len(review.EvidenceRefs) != 2 {
		t.Fatalf("deterministic rows should remain evidence refs, got %+v", review.EvidenceRefs)
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
	if strings.Join(review.SupportedPaths, ",") != "" {
		t.Fatalf("supported paths = %+v", review.SupportedPaths)
	}
	if strings.Join(review.MissingPaths, ",") != "pkg/caller.py,pkg/covered/helper.py,pkg/owner.py" {
		t.Fatalf("missing paths = %+v", review.MissingPaths)
	}
	if strings.Join(review.OwnerMissingPaths, ",") != "pkg/caller.py,pkg/covered/helper.py,pkg/owner.py" {
		t.Fatalf("owner missing paths = %+v", review.OwnerMissingPaths)
	}
	if review.SupportRatio != 0 {
		t.Fatalf("support ratio = %v", review.SupportRatio)
	}
}

func TestSourceLocalizationReviewFromWritePlanContextCarriesMatchedPriorAnchors(t *testing.T) {
	ref := WriteExplorationEvidenceRef{ID: "ev-owner", Source: "pkg/owner.py", LineStart: 12, Subject: "Owner", AnchorSymbol: "Owner.handle"}
	prior := []WriteContextPack{{
		PackID:      "exploration-handoff",
		SourceStage: "explore",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "localization_anchor",
			Text:        "path=pkg/owner.py strength=owner",
			SourceStage: "explore",
			LocalizationAnchor: &SourceLocalizationAnchor{
				Path:         "pkg/owner.py",
				Role:         SourcePathRoleProduction,
				SourceStage:  "read_turn_a",
				Kind:         SourceLocalizationAnchorGroundedEvidence,
				Strength:     SourceLocalizationAnchorOwner,
				EvidenceRef:  &ref,
				Subject:      "Owner",
				AnchorSymbol: "Owner.handle",
			},
		}, {
			Priority:    WriteContextP1,
			Kind:        "scope_anchor",
			Text:        "pkg/scope",
			SourceStage: "write_analysis",
		}, {
			Priority:    WriteContextP2,
			Kind:        "localization_anchor",
			Text:        "path=pkg/observed.py strength=observed",
			SourceStage: "explore",
			LocalizationAnchor: &SourceLocalizationAnchor{
				Path:     "pkg/observed.py",
				Role:     SourcePathRoleProduction,
				Kind:     SourceLocalizationAnchorReadFile,
				Strength: SourceLocalizationAnchorObserved,
			},
		}},
	}}
	plan := &ChangePlan{
		ID:          "plan-1",
		TargetPaths: []string{"pkg/owner.py", "pkg/scope/helper.py", "pkg/observed.py"},
		Changes: []FileChange{
			{Path: "pkg/owner.py", Kind: "modify"},
			{Path: "pkg/scope/helper.py", Kind: "modify"},
			{Path: "pkg/observed.py", Kind: "modify"},
		},
	}

	review := SourceLocalizationReviewFromWritePlanContext("batch-1", "repair", prior, plan)
	if review.Status != SourceLocalizationWeak {
		t.Fatalf("status = %s, want weak because observed.py is not owner-supported: %+v", review.Status, review)
	}
	if strings.Join(review.SupportedPaths, ",") != "pkg/owner.py" {
		t.Fatalf("supported paths = %+v", review.SupportedPaths)
	}
	if strings.Join(review.OwnerSupportedPaths, ",") != "pkg/owner.py" {
		t.Fatalf("owner supported paths = %+v", review.OwnerSupportedPaths)
	}
	if strings.Join(review.MissingPaths, ",") != "pkg/observed.py,pkg/scope/helper.py" {
		t.Fatalf("missing paths = %+v", review.MissingPaths)
	}
	if strings.Join(review.OwnerMissingPaths, ",") != "pkg/observed.py,pkg/scope/helper.py" {
		t.Fatalf("owner missing paths = %+v", review.OwnerMissingPaths)
	}
	if !sourceLocalizationTestHasAnchor(review, "pkg/owner.py", SourceLocalizationAnchorGroundedEvidence, SourceLocalizationAnchorOwner) {
		t.Fatalf("owner anchor not carried into plan review: %+v", review.Anchors)
	}
	if !sourceLocalizationTestHasAnchor(review, "pkg/scope", SourceLocalizationAnchorScope, SourceLocalizationAnchorSupporting) {
		t.Fatalf("scope anchor not carried into plan review: %+v", review.Anchors)
	}
	if sourceLocalizationTestHasAnchor(review, "pkg/observed.py", SourceLocalizationAnchorReadFile, SourceLocalizationAnchorObserved) {
		t.Fatalf("observed read-file anchors must not satisfy plan review: %+v", review.Anchors)
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
	if strings.Join(review.OwnerMissingPaths, ",") != "pkg/new_owner.py" {
		t.Fatalf("owner missing paths = %+v", review.OwnerMissingPaths)
	}
	if got := WritePlanSourcePathsOutsidePriorContext(nil, plan); len(got) != 0 {
		t.Fatalf("no-prior review is audit signal only; routing helper returned %+v", got)
	}
}

func TestNormalizeSourceLocalizationReviewDowngradesSupportedWithOwnerGaps(t *testing.T) {
	review := NormalizeSourceLocalizationReview(SourceLocalizationReview{
		Status:            SourceLocalizationSupported,
		SourcePaths:       []string{"pkg/bug.py"},
		SupportedPaths:    []string{"pkg/bug.py"},
		OwnerMissingPaths: []string{"pkg/bug.py"},
		ReasonCodes:       []string{"plan_source_path_without_owner_anchor"},
	})

	if review.Status != SourceLocalizationWeak {
		t.Fatalf("status = %s, want weak when owner gaps remain: %+v", review.Status, review)
	}
	if strings.Join(review.OwnerMissingPaths, ",") != "pkg/bug.py" {
		t.Fatalf("owner missing paths = %+v", review.OwnerMissingPaths)
	}
}

func sourceLocalizationTestHasAnchor(review SourceLocalizationReview, path string, kind SourceLocalizationAnchorKind, strength SourceLocalizationAnchorStrength) bool {
	return sourceLocalizationTestAnchor(review, path, kind, strength) != nil
}

func sourceLocalizationTestAnchor(review SourceLocalizationReview, path string, kind SourceLocalizationAnchorKind, strength SourceLocalizationAnchorStrength) *SourceLocalizationAnchor {
	for _, anchor := range review.Anchors {
		if anchor.Path == path && anchor.Kind == kind && anchor.Strength == strength {
			anchorCopy := anchor
			return &anchorCopy
		}
	}
	return nil
}
