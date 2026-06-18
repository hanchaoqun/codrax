package types

import (
	"path/filepath"
	"testing"
	"time"
)

func TestBuildWriteFinalReportProjectsTypedArtifacts(t *testing.T) {
	now := time.Unix(123, 0)
	run := &WriteWorkflowRun{
		RunID:         "run-1",
		Goal:          "fix bug",
		Status:        WriteWorkflowRunComplete,
		ActiveBatchID: "batch-1",
		Completion: &WriteWorkflowCompletion{
			Verdict:    WriteWorkflowCompletionUnverified,
			ReasonCode: "runner_missing",
			Source:     "verify_attempt",
			At:         now,
		},
		Batches: []WriteWorkflowBatch{{
			ID:            "batch-1",
			Status:        WriteWorkflowBatchComplete,
			PlanID:        "plan-1",
			VerifyRef:     "plan-1.report.json",
			ActiveSliceID: "slice-1",
			Slices: []WriteWorkflowSlice{{
				ID:     "slice-1",
				Status: ChangePlanSliceUnverified,
			}},
			Completion: &WriteWorkflowCompletion{
				Verdict:    WriteWorkflowCompletionUnverified,
				ReasonCode: "runner_missing",
				Source:     "verify_attempt",
				At:         now,
			},
			Attempts: []WriteWorkflowAttempt{{
				Kind:       "verify",
				Status:     "unverified",
				ReasonCode: "runner_missing",
			}},
		}},
		ContextPacks: []WriteContextPack{{
			Items: []WriteContextItem{{
				Priority:    WriteContextP0,
				Kind:        "constraint",
				SourceStage: "write_analysis",
				Text:        "must stay scoped",
			}},
		}},
	}
	plan := &ChangePlan{
		ID:               "plan-1",
		Status:           PlanStatusUnverified,
		TargetPaths:      []string{"src/app.py", "tests/test_app.py"},
		AppliedPaths:     []string{"src/app.py"},
		AppliedCommitSHA: "abc123",
		PatchEffect: &PatchEffectRecord{
			RecordID:        "patch-effect:plan-1:slice-1:fp",
			DiffFingerprint: "fp",
			DiffBytes:       42,
			Files: []PatchEffectFile{{
				Path:       "src/app.py",
				AddedLines: 2,
				Events: []PatchEffectEvent{{
					Code:     "caller_return_shape_adapter_added",
					Severity: "warning",
				}},
			}},
		},
		PatchReview: &PatchReviewRecord{
			Status: "passed",
			Findings: []PatchReviewFinding{{
				Code:           "changed_symbol_without_probe_coverage",
				Severity:       PatchReviewSeverityWarning,
				Category:       PatchReviewCategorySemanticCoverage,
				ImpactKind:     PatchReviewImpactKindChangedSymbol,
				CoverageStatus: PatchReviewCoverageUnverified,
			}},
		},
		ImpactAnalysis: &ImpactAnalysisResult{
			ResultID: "impact-1",
			VerificationTargets: []ImpactVerificationTarget{{
				ID:             "target-1",
				Kind:           "changed_symbol",
				Path:           "src/app.py",
				Priority:       20,
				CoverageStatus: "unverified",
			}},
		},
	}
	report := &ChangeReport{
		PlanID:             "plan-1",
		VerificationStatus: VerificationStatusUnavailable,
		FailureKind:        FailureKindRunnerMissing,
		FailureReasonCode:  "runner_missing",
		NoTestsRunners:     []string{"pytest"},
		GeneratedAt:        now,
	}

	got := BuildWriteFinalReport(WriteFinalReportInput{
		Run:          run,
		Plan:         plan,
		Report:       report,
		PlanPath:     "/tmp/plan-1.json",
		ReportPath:   "/tmp/plan-1.report.json",
		WorkflowPath: "/tmp/workflows/run-1.json",
		GeneratedAt:  now,
	})

	if got.Kind != WriteOutputFinalReport || got.SchemaVersion != WriteFinalReportSchemaVersion {
		t.Fatalf("unexpected final report identity: %+v", got)
	}
	if got.Completion == nil || got.Completion.Verdict != WriteWorkflowCompletionUnverified {
		t.Fatalf("Completion=%+v, want unverified", got.Completion)
	}
	if got.Plan.ID != "plan-1" || len(got.Plan.SourcePaths) != 1 || got.Plan.SourcePaths[0] != "src/app.py" {
		t.Fatalf("Plan=%+v, want source path projection", got.Plan)
	}
	if got.Patch.PatchEffectID == "" || len(got.Patch.EffectEventCodes) != 1 {
		t.Fatalf("Patch=%+v, want patch effect projection", got.Patch)
	}
	if got.Verification.Status != VerificationStatusUnavailable || got.Verification.FailureKind != FailureKindRunnerMissing {
		t.Fatalf("Verification=%+v, want unavailable runner_missing", got.Verification)
	}
	if got.PatchReview.Verdict != PatchReviewCoverageVerdictUnverified ||
		len(got.PatchReview.UnverifiedKinds) != 1 ||
		got.PatchReview.UnverifiedKinds[0] != PatchReviewImpactKindChangedSymbol {
		t.Fatalf("PatchReview=%+v, want unverified changed_symbol", got.PatchReview)
	}
	if got.Impact.CoverageCounts["unverified"] != 1 || len(got.Impact.UncoveredTargets) != 1 {
		t.Fatalf("Impact=%+v, want one unverified target", got.Impact)
	}
	if len(got.Handoff.TopItems) != 1 || got.Handoff.TopItems[0].Kind != "constraint" {
		t.Fatalf("Handoff=%+v, want typed context item", got.Handoff)
	}
	if len(got.ResidualRisks) == 0 {
		t.Fatalf("ResidualRisks empty, want unverified risks")
	}
}

func TestWriteFinalReportToFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan-1.final.json")
	report := BuildWriteFinalReport(WriteFinalReportInput{
		Run: &WriteWorkflowRun{
			RunID:  "run-1",
			Status: WriteWorkflowRunComplete,
			Completion: &WriteWorkflowCompletion{
				Verdict: WriteWorkflowCompletionVerified,
			},
		},
		Plan: &ChangePlan{ID: "plan-1", Status: PlanStatusApplied},
		Report: &ChangeReport{
			PlanID: "plan-1",
			Passed: true,
		},
	})
	if err := WriteFinalReportToFile(&report, path); err != nil {
		t.Fatalf("WriteFinalReportToFile: %v", err)
	}
	got, err := LoadWriteFinalReportFromFile(path)
	if err != nil {
		t.Fatalf("LoadWriteFinalReportFromFile: %v", err)
	}
	if got.RunID != "run-1" || got.Plan.ID != "plan-1" || got.Verification.Status != VerificationStatusPassed {
		t.Fatalf("round trip=%+v", got)
	}
}

func TestWriteOutputKindIncludesFinalReport(t *testing.T) {
	if !IsValidWriteOutputKind(WriteOutputFinalReport) {
		t.Fatal("WriteOutputFinalReport must be declared")
	}
}
