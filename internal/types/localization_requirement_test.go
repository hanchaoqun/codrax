package types

import (
	"strings"
	"testing"
)

func TestLocalizationRequirementsFromWritePlanContextClassifiesPriorAndOwnerGaps(t *testing.T) {
	ref := WriteExplorationEvidenceRef{ID: "ev-owner", Source: "pkg/owner.py", LineStart: 12, OwnerSymbol: "Owner.fix"}
	prior := []WriteContextPack{{
		PackID:      "mixed-localization",
		BatchID:     "batch-1",
		SourceStage: "explore",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "scope_anchor",
			Text:        "pkg/scope.py",
			SourceStage: "write_analysis",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}, {
			Priority:    WriteContextP1,
			Kind:        "localization_anchor",
			Text:        "path=pkg/owner.py owner=Owner.fix",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
			EvidenceRef: &ref,
			LocalizationAnchor: &SourceLocalizationAnchor{
				Path:        "pkg/owner.py",
				Role:        SourcePathRoleProduction,
				Kind:        SourceLocalizationAnchorGroundedEvidence,
				Strength:    SourceLocalizationAnchorOwner,
				EvidenceRef: &ref,
				OwnerSymbol: "Owner.fix",
			},
		}},
	}}
	plan := &ChangePlan{
		ID:          "plan-1",
		TargetPaths: []string{"pkg/owner.py", "pkg/scope.py", "pkg/missing.py", "pkg/tests/test_bug.py"},
		Changes: []FileChange{
			{Path: "pkg/owner.py", Kind: "patch"},
			{Path: "pkg/scope.py", Kind: "patch"},
			{Path: "pkg/missing.py", Kind: "patch"},
			{Path: "pkg/tests/test_bug.py", Kind: "patch"},
		},
	}

	got := LocalizationRequirementsFromWritePlanContext("batch-1", "slice-1", WriteConsumerPlanner, prior, plan, 0)
	if got.OpenItems != 2 {
		t.Fatalf("OpenItems=%d, want 2: %+v", got.OpenItems, got.Items)
	}
	if strings.Join(got.MissingPriorContextPaths, ",") != "pkg/missing.py" {
		t.Fatalf("MissingPriorContextPaths=%+v", got.MissingPriorContextPaths)
	}
	if strings.Join(got.MissingOwnerAnchorPaths, ",") != "pkg/scope.py" {
		t.Fatalf("MissingOwnerAnchorPaths=%+v", got.MissingOwnerAnchorPaths)
	}
	rows := strings.Join(LocalizationRequirementEvidenceRows(got, 8), "\n")
	if !strings.Contains(rows, "path=pkg/missing.py") ||
		!strings.Contains(rows, "reason=plan_source_path_missing_prior_context") ||
		!strings.Contains(rows, "path=pkg/scope.py") ||
		!strings.Contains(rows, "reason=plan_source_path_without_owner_anchor") {
		t.Fatalf("evidence rows missing typed requirements:\n%s", rows)
	}
}

func TestLocalizationRequirementsFromCandidatePathsRequestsOwnerEvidence(t *testing.T) {
	view := OwnerAnchorViewFromSourceLocalizationReview(SourceLocalizationReview{
		Source: "explore",
		Anchors: []SourceLocalizationAnchor{{
			Path:        "pkg/owner.py",
			Role:        SourcePathRoleProduction,
			Kind:        SourceLocalizationAnchorGroundedEvidence,
			Strength:    SourceLocalizationAnchorOwner,
			OwnerSymbol: "Owner.fix",
		}},
	}, 0)

	got := LocalizationRequirementsFromCandidatePaths([]string{"pkg/owner.py", "pkg/symptom.py", "pkg/tests/test_bug.py"}, view, "batch-1", "", "controller_preplan", 0)
	if got.OpenItems != 1 || strings.Join(got.MissingOwnerAnchorPaths, ",") != "pkg/symptom.py" {
		t.Fatalf("candidate owner requirements = %+v", got)
	}
	row := strings.Join(LocalizationRequirementEvidenceRows(got, 8), "\n")
	if !strings.Contains(row, "reason=candidate_path_without_owner_anchor") ||
		!strings.Contains(row, "required=typed_owner_or_supporting_localization_anchor") {
		t.Fatalf("candidate requirement row missing typed fields: %q", row)
	}
}

func TestLocalizationRequirementsFromSourceLocalizationReviewReportsReadOwnerDiscovery(t *testing.T) {
	review := SourceLocalizationReviewFromTurnA(
		[]string{"pkg/read_only.py"},
		[]EvidenceItem{{
			ID:              "ev-aux",
			Source:          "pkg/tests/test_bug.py",
			Kind:            EvidenceDirect,
			GroundingStatus: GroundingGrounded,
			ContextRole:     EvidenceContextRoleRelatedContext,
		}},
	)

	got := LocalizationRequirementsFromSourceLocalizationReview(review, 0)
	if got.OpenItems != 1 || strings.Join(got.MissingOwnerAnchorPaths, ",") != "pkg/read_only.py" {
		t.Fatalf("read localization requirements = %+v", got)
	}
	row := strings.Join(LocalizationRequirementEvidenceRows(got, 8), "\n")
	if !strings.Contains(row, "reason=read_source_path_without_owner_anchor") {
		t.Fatalf("read owner-discovery row missing: %q", row)
	}
}

func TestLocalizationRequirementsIgnorePlanNarrativeText(t *testing.T) {
	prior := []WriteContextPack{{
		PackID:      "write-analysis",
		BatchID:     "batch-1",
		SourceStage: "write_analysis",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "scope_anchor",
			Text:        "pkg/bug.py",
			SourceStage: "write_analysis",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}},
	}}
	planA := &ChangePlan{
		ID:          "plan-a",
		Summary:     "fix the customer-visible crash",
		TargetPaths: []string{"pkg/bug.py"},
		Changes:     []FileChange{{Path: "pkg/bug.py", Kind: "patch"}},
	}
	planB := &ChangePlan{
		ID:          "plan-b",
		Summary:     "unrelated narrative with tempting words about owner symbols",
		TargetPaths: append([]string(nil), planA.TargetPaths...),
		Changes:     append([]FileChange(nil), planA.Changes...),
	}

	rowsA := strings.Join(LocalizationRequirementEvidenceRows(LocalizationRequirementsFromWritePlanContext("batch-1", "", WriteConsumerPlanner, prior, planA, 0), 8), "\n")
	rowsB := strings.Join(LocalizationRequirementEvidenceRows(LocalizationRequirementsFromWritePlanContext("batch-1", "", WriteConsumerPlanner, prior, planB, 0), 8), "\n")
	if rowsA != rowsB {
		t.Fatalf("localization requirements must ignore plan narrative text:\nA=%s\nB=%s", rowsA, rowsB)
	}
}
