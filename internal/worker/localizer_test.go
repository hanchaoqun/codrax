package worker

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRunLocalizerPromotesTypedOwnerAnchor(t *testing.T) {
	ref := types.WriteExplorationEvidenceRef{
		ID:           "ev-1",
		Kind:         "definition",
		Source:       "pkg/handler.py",
		LineStart:    12,
		OwnerSymbol:  "Handler",
		AnchorSymbol: "Handler",
	}
	out := RunLocalizer(LocalizerInput{
		ID:             "loc-1",
		Goal:           "localize handler owner",
		CandidatePaths: []string{"pkg/handler.py"},
		Anchors: []types.SourceLocalizationAnchor{{
			Path:         "pkg/handler.py",
			Role:         types.SourcePathRoleProduction,
			SourceStage:  "read_turn_a",
			Kind:         types.SourceLocalizationAnchorGroundedEvidence,
			Strength:     types.SourceLocalizationAnchorOwner,
			EvidenceRef:  &ref,
			OwnerSymbol:  "Handler",
			AnchorSymbol: "Handler",
			ReasonCode:   "grounded_evidence_owner",
		}},
	})
	if out.Authority.State != loopkernel.LocalizationAuthorityOwnerSupported {
		t.Fatalf("authority = %+v, want owner_supported", out.Authority)
	}
	if out.Review.Status != types.SourceLocalizationSupported {
		t.Fatalf("review = %+v, want supported", out.Review)
	}
	if len(out.Review.OwnerSupportedPaths) != 1 || out.Review.OwnerSupportedPaths[0] != "pkg/handler.py" {
		t.Fatalf("owner paths = %+v", out.Review.OwnerSupportedPaths)
	}
	if errs := ValidateRequest(out.Request); len(errs) != 0 {
		t.Fatalf("localizer request should validate: %+v", errs)
	}
	if errs := ValidateResult(out.Result); len(errs) != 0 {
		t.Fatalf("localizer result should validate: %+v", errs)
	}
	if len(out.Result.OutputRefs) == 0 || out.Result.OutputRefs[0].Kind == "" {
		t.Fatalf("result should carry typed output refs: %+v", out.Result.OutputRefs)
	}
}

func TestRunLocalizerKeepsObservedOnlyGapOpen(t *testing.T) {
	out := RunLocalizer(LocalizerInput{
		ID:             "loc-2",
		CandidatePaths: []string{"pkg/maybe.py"},
	})
	if out.Authority.State != loopkernel.LocalizationAuthorityObservedOnly {
		t.Fatalf("authority = %+v, want observed_only", out.Authority)
	}
	if len(out.Review.OwnerMissingPaths) != 1 || out.Review.OwnerMissingPaths[0] != "pkg/maybe.py" {
		t.Fatalf("owner missing paths = %+v", out.Review.OwnerMissingPaths)
	}
	if out.Requirements.OpenItems != 1 || len(out.Requirements.MissingOwnerAnchorPaths) != 1 {
		t.Fatalf("requirements should keep owner gap open: %+v", out.Requirements)
	}
	if out.Result.Status != StatusComplete || out.Result.ReasonCode != "localization_observed_without_owner" {
		t.Fatalf("result should complete with typed weak-localization reason: %+v", out.Result)
	}
}

func TestRunLocalizerPreservesPartialOwnerCoverage(t *testing.T) {
	ref := types.WriteExplorationEvidenceRef{ID: "ev-2", Source: "pkg/a.py", OwnerSymbol: "A"}
	out := RunLocalizer(LocalizerInput{
		CandidatePaths: []string{"pkg/a.py", "pkg/b.py"},
		Anchors: []types.SourceLocalizationAnchor{{
			Path:        "pkg/a.py",
			Role:        types.SourcePathRoleProduction,
			Kind:        types.SourceLocalizationAnchorGroundedEvidence,
			Strength:    types.SourceLocalizationAnchorOwner,
			EvidenceRef: &ref,
			ReasonCode:  "grounded_evidence_owner",
		}},
	})
	if out.Authority.State != loopkernel.LocalizationAuthorityConflicted {
		t.Fatalf("partial owner coverage should be conflicted, got %+v", out.Authority)
	}
	if len(out.Review.OwnerSupportedPaths) != 1 || out.Review.OwnerSupportedPaths[0] != "pkg/a.py" {
		t.Fatalf("supported paths = %+v", out.Review.OwnerSupportedPaths)
	}
	if len(out.Review.OwnerMissingPaths) != 1 || out.Review.OwnerMissingPaths[0] != "pkg/b.py" {
		t.Fatalf("missing paths = %+v", out.Review.OwnerMissingPaths)
	}
}

func TestRunLocalizerCarriesNavigationCoverageRefs(t *testing.T) {
	coverage := types.RepoMapNavigationCoverage{
		State:          types.RepoMapNavigationCoveragePartial,
		ReasonCode:     "repo_map_navigation_partial",
		RequiredRoutes: []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteFileMap, types.RepoMapNavigationRouteRelationMap},
		CoveredRoutes:  []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteFileMap},
		MissingRoutes:  []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteRelationMap},
		EvidenceRefs:   []string{"repo-map-1"},
	}
	out := RunLocalizer(LocalizerInput{
		ID:             "loc-3",
		CandidatePaths: []string{"pkg/target.py"},
		Navigation:     &coverage,
	})
	if out.Navigation.State != types.RepoMapNavigationCoveragePartial {
		t.Fatalf("navigation not preserved: %+v", out.Navigation)
	}
	if !stringSliceContains(out.Result.EvidenceRefs, "repo-map-1") {
		t.Fatalf("navigation evidence ref not carried: %+v", out.Result.EvidenceRefs)
	}
	if !artifactKindsContain(out.Result.OutputRefs, "repo_map_navigation_coverage") {
		t.Fatalf("navigation output ref missing: %+v", out.Result.OutputRefs)
	}
}

func TestLocalizerInputFromReadArtifactsUsesTypedTurnAState(t *testing.T) {
	turnA := &types.TurnAArtifacts{
		ReadFiles: []string{"pkg/read_owner.py"},
		SourceLocalization: &types.SourceLocalizationReview{
			Status:              types.SourceLocalizationSupported,
			Source:              "read_turn_a",
			SourcePaths:         []string{"pkg/read_owner.py"},
			SupportedPaths:      []string{"pkg/read_owner.py"},
			OwnerSupportedPaths: []string{"pkg/read_owner.py"},
			Anchors: []types.SourceLocalizationAnchor{{
				Path:       "pkg/read_owner.py",
				Role:       types.SourcePathRoleProduction,
				Kind:       types.SourceLocalizationAnchorGroundedEvidence,
				Strength:   types.SourceLocalizationAnchorOwner,
				ReasonCode: "grounded_evidence_owner",
			}},
		},
	}
	input := LocalizerInputFromReadArtifacts("read-loc", &types.AnalysisIR{}, types.ExploreLanePlan{}, turnA)
	if input.ID != "read-loc" || len(input.InputRefs) == 0 {
		t.Fatalf("read adapter did not preserve id/input refs: %+v", input)
	}
	out := RunLocalizer(input)
	if out.Authority.State != loopkernel.LocalizationAuthorityOwnerSupported {
		t.Fatalf("read adapter output authority = %+v", out.Authority)
	}
}

func TestLocalizerInputFromWriteWorkflowRunUsesContextPacks(t *testing.T) {
	ref := types.WriteExplorationEvidenceRef{ID: "ev-write", Source: "pkg/write_owner.py", OwnerSymbol: "WriteOwner"}
	run := types.WriteWorkflowRun{
		RunID:         "run-1",
		Goal:          "repair write owner",
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			ExpectedPaths: []string{"pkg/write_owner.py"},
		}},
		ContextPacks: []types.WriteContextPack{{
			PackID:  "pack-1",
			BatchID: "batch-1",
			Items: []types.WriteContextItem{{
				Kind:        "owner_anchor",
				Priority:    types.WriteContextP1,
				SourceStage: "explore",
				LocalizationAnchor: &types.SourceLocalizationAnchor{
					Path:        "pkg/write_owner.py",
					Role:        types.SourcePathRoleProduction,
					Kind:        types.SourceLocalizationAnchorGroundedEvidence,
					Strength:    types.SourceLocalizationAnchorOwner,
					EvidenceRef: &ref,
					ReasonCode:  "grounded_evidence_owner",
				},
			}},
		}},
	}
	input := LocalizerInputFromWriteWorkflowRun("write-loc", run, "batch-1")
	if input.RunID != "run-1" || input.BatchID != "batch-1" || len(input.InputRefs) == 0 {
		t.Fatalf("write adapter did not preserve run/batch/input refs: %+v", input)
	}
	out := RunLocalizer(input)
	if out.Authority.State != loopkernel.LocalizationAuthorityOwnerSupported {
		t.Fatalf("write adapter output authority = %+v", out.Authority)
	}
	if !stringSliceContains(out.Result.EvidenceRefs, "ev-write") {
		t.Fatalf("write evidence ref not carried: %+v", out.Result.EvidenceRefs)
	}
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func artifactKindsContain(refs []loopkernel.ArtifactRef, want string) bool {
	for _, ref := range refs {
		if ref.Kind == want {
			return true
		}
	}
	return false
}
