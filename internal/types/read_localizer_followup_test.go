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

func containsRepoMapRouteForLocalizer(routes []RepoMapNavigationRoute, want RepoMapNavigationRoute) bool {
	for _, route := range routes {
		if route == want {
			return true
		}
	}
	return false
}
