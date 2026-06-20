package types

import (
	"strings"
	"testing"
)

func TestDeriveReadLocalizerFollowupNeedsOwnerAndNavigation(t *testing.T) {
	review := SourceLocalizationReviewFromTurnA([]string{"pkg/handler.py"}, nil)
	coverage := RepoMapNavigationCoverage{
		State:          RepoMapNavigationCoverageMissing,
		ReasonCode:     "repo_map_navigation_missing",
		RequiredRoutes: []RepoMapNavigationRoute{RepoMapNavigationRouteFileMap, RepoMapNavigationRouteRelationMap},
		MissingRoutes:  []RepoMapNavigationRoute{RepoMapNavigationRouteFileMap, RepoMapNavigationRouteRelationMap},
	}

	got := DeriveReadLocalizerFollowup(&review, &coverage)
	if got == nil {
		t.Fatal("expected follow-up")
	}
	if got.State != ReadLocalizerFollowupNeeded ||
		got.ReasonCode != "read_localizer_owner_and_navigation_missing" {
		t.Fatalf("unexpected follow-up state/reason: %+v", got)
	}
	if strings.Join(got.CandidatePaths, ",") != "pkg/handler.py" {
		t.Fatalf("candidate paths = %+v", got.CandidatePaths)
	}
	if !containsRepoMapRouteForLocalizer(got.MissingRoutes, RepoMapNavigationRouteRelationMap) {
		t.Fatalf("missing route relation_map not preserved: %+v", got.MissingRoutes)
	}
	rows := strings.Join(got.EvidenceRequirements, "\n")
	for _, want := range []string{
		"repo_map_navigation_requirement route=file_map",
		"localization_requirement path=pkg/handler.py",
		"required=typed_owner_localization_anchor",
	} {
		if !strings.Contains(rows, want) {
			t.Fatalf("evidence requirements missing %q:\n%s", want, rows)
		}
	}
}

func TestDeriveReadLocalizerFollowupNilWhenOwnerAndNavigationCovered(t *testing.T) {
	review := NormalizeSourceLocalizationReview(SourceLocalizationReview{
		Status:              SourceLocalizationSupported,
		SourcePaths:         []string{"pkg/handler.py"},
		OwnerSupportedPaths: []string{"pkg/handler.py"},
		Anchors: []SourceLocalizationAnchor{{
			Path:        "pkg/handler.py",
			Strength:    SourceLocalizationAnchorOwner,
			Kind:        SourceLocalizationAnchorGroundedEvidence,
			OwnerSymbol: "Handler",
		}},
	})
	coverage := RepoMapNavigationCoverage{
		State:          RepoMapNavigationCoverageCovered,
		RequiredRoutes: []RepoMapNavigationRoute{RepoMapNavigationRouteFileMap},
		CoveredRoutes:  []RepoMapNavigationRoute{RepoMapNavigationRouteFileMap},
	}

	if got := DeriveReadLocalizerFollowup(&review, &coverage); got != nil {
		t.Fatalf("covered owner/navigation should not produce follow-up: %+v", got)
	}
}

func TestDeriveReadLocalizerFollowupDemotesNavigationOnlyAfterOwnerCoverage(t *testing.T) {
	review := SourceLocalizationReviewFromTurnA(
		[]string{"pkg/owner.py", "pkg/nearby.py"},
		[]EvidenceItem{{
			ID:              "ev-owner",
			Kind:            EvidenceMechanism,
			Scope:           ScopeLine,
			Source:          "pkg/owner.py",
			LineStart:       42,
			Subject:         "Owner.Handle",
			GroundingStatus: GroundingGrounded,
			GroundingTier:   TierLineText,
		}},
	)
	coverage := RepoMapNavigationCoverage{
		State:          RepoMapNavigationCoverageMissing,
		ReasonCode:     "repo_map_navigation_missing",
		RequiredRoutes: []RepoMapNavigationRoute{RepoMapNavigationRouteFileMap},
		MissingRoutes:  []RepoMapNavigationRoute{RepoMapNavigationRouteFileMap},
	}

	if got := DeriveReadLocalizerFollowup(&review, &coverage); got != nil {
		t.Fatalf("navigation-only debt should be advisory after owner evidence coverage: %+v", got)
	}
}

func TestDeriveReadLocalizerFollowupKeepsExplicitMissingOwner(t *testing.T) {
	review := NormalizeSourceLocalizationReview(SourceLocalizationReview{
		Status:              SourceLocalizationWeak,
		SourcePaths:         []string{"pkg/owner.py", "pkg/missing.py"},
		OwnerSupportedPaths: []string{"pkg/owner.py"},
		OwnerMissingPaths:   []string{"pkg/missing.py"},
		Anchors: []SourceLocalizationAnchor{{
			Path:        "pkg/owner.py",
			Strength:    SourceLocalizationAnchorOwner,
			Kind:        SourceLocalizationAnchorGroundedEvidence,
			OwnerSymbol: "Owner.Handle",
		}},
	})
	coverage := RepoMapNavigationCoverage{
		State:          RepoMapNavigationCoverageMissing,
		ReasonCode:     "repo_map_navigation_missing",
		RequiredRoutes: []RepoMapNavigationRoute{RepoMapNavigationRouteFileMap},
		MissingRoutes:  []RepoMapNavigationRoute{RepoMapNavigationRouteFileMap},
	}

	got := DeriveReadLocalizerFollowup(&review, &coverage)
	if got == nil {
		t.Fatal("explicit missing owner path should remain a follow-up")
	}
	if got.ReasonCode != "read_localizer_owner_and_navigation_missing" {
		t.Fatalf("explicit owner gap should preserve blocking reason, got %+v", got)
	}
	if strings.Join(got.CandidatePaths, ",") != "pkg/missing.py,pkg/owner.py" {
		t.Fatalf("candidate paths should preserve explicit missing owner first, got %+v", got.CandidatePaths)
	}
}

func containsRepoMapRouteForLocalizer(routes []RepoMapNavigationRoute, want RepoMapNavigationRoute) bool {
	for _, route := range routes {
		if route == want {
			return true
		}
	}
	return false
}
