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
			}, {
				Priority:    WriteContextP1,
				Kind:        "localization_anchor",
				SourceStage: "explore",
				Text:        "path=src/owner.py owner=Owner strength=owner",
				LocalizationAnchor: &SourceLocalizationAnchor{
					Path:         "src/owner.py",
					Kind:         SourceLocalizationAnchorGroundedEvidence,
					Strength:     SourceLocalizationAnchorOwner,
					OwnerSymbol:  "Owner",
					AnchorSymbol: "Owner.handle",
				},
			}},
		}},
	}
	plan := &ChangePlan{
		ID:               "plan-1",
		Status:           PlanStatusUnverified,
		TargetPaths:      []string{"src/app.py", "tests/test_app.py"},
		AppliedPaths:     []string{"src/app.py"},
		AppliedCommitSHA: "abc123",
		LocalizationReview: &SourceLocalizationReview{
			Status:            SourceLocalizationWeak,
			Source:            "write_plan_context",
			PlanID:            "plan-1",
			SourcePaths:       []string{"src/app.py"},
			PriorContextPaths: []string{"src/owner.py"},
			MissingPaths:      []string{"src/app.py"},
			ReasonCodes:       []string{"plan_source_paths_partially_outside_prior_context"},
			Anchors: []SourceLocalizationAnchor{{
				Path:         "src/owner.py",
				Kind:         SourceLocalizationAnchorGroundedEvidence,
				Strength:     SourceLocalizationAnchorOwner,
				OwnerSymbol:  "Owner",
				AnchorSymbol: "Owner.handle",
			}},
		},
		OwnerAnchors: []OwnerAnchorViewItem{{
			Path:         "src/owner.py",
			Kind:         SourceLocalizationAnchorGroundedEvidence,
			Strength:     SourceLocalizationAnchorOwner,
			OwnerSymbol:  "Owner",
			AnchorSymbol: "Owner.handle",
		}},
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
	if got.Plan.Localization == nil ||
		got.Plan.Localization.Status != SourceLocalizationWeak ||
		len(got.Plan.Localization.MissingPaths) != 1 ||
		got.Plan.Localization.MissingPaths[0] != "src/app.py" {
		t.Fatalf("Plan.Localization=%+v, want weak missing source path", got.Plan.Localization)
	}
	if len(got.Plan.OwnerAnchors) != 1 || got.Plan.OwnerAnchors[0].OwnerSymbol != "Owner" {
		t.Fatalf("Plan.OwnerAnchors=%+v, want selected owner anchor", got.Plan.OwnerAnchors)
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
	if got.Proof.Status != VerificationProofUnavailable ||
		got.Proof.RunnerEvidence != VerificationProofRunnerUnavailable ||
		!writeFinalProofHasReason(got.Proof, "runner_missing") {
		t.Fatalf("Proof=%+v, want unavailable runner_missing profile", got.Proof)
	}
	if got.Impact.CoverageCounts["unverified"] != 1 || len(got.Impact.UncoveredTargets) != 1 {
		t.Fatalf("Impact=%+v, want one unverified target", got.Impact)
	}
	if len(got.Handoff.TopItems) == 0 || got.Handoff.TopItems[0].Kind != "constraint" {
		t.Fatalf("Handoff=%+v, want typed context item", got.Handoff)
	}
	if len(got.Handoff.OwnerAnchors) != 1 || got.Handoff.OwnerAnchors[0].OwnerSymbol != "Owner" {
		t.Fatalf("Handoff.OwnerAnchors=%+v, want typed owner anchor", got.Handoff.OwnerAnchors)
	}
	if len(got.ResidualRisks) == 0 {
		t.Fatalf("ResidualRisks empty, want unverified risks")
	}
	if !writeFinalReportHasRisk(got, "source_localization_weak") {
		t.Fatalf("ResidualRisks=%+v missing source localization risk", got.ResidualRisks)
	}
	if !writeFinalReportHasRisk(got, "verification_proof_unavailable") {
		t.Fatalf("ResidualRisks=%+v missing verification proof risk", got.ResidualRisks)
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

func TestBuildWriteFinalReportProjectsOwnerAnchorGaps(t *testing.T) {
	report := BuildWriteFinalReport(WriteFinalReportInput{
		Run: &WriteWorkflowRun{
			RunID:  "run-owner-gap",
			Status: WriteWorkflowRunComplete,
			ContextPacks: []WriteContextPack{{
				PackID:      "write-analysis",
				BatchID:     "batch-1",
				SourceStage: "write_analysis",
				Items: []WriteContextItem{{
					Priority:    WriteContextP1,
					Kind:        "scope_anchor",
					Text:        "pkg/bug.py",
					SourceStage: "write_analysis",
					Consumers:   []WriteContextConsumer{WriteConsumerPlanner, WriteConsumerController},
				}},
			}},
		},
		Plan: &ChangePlan{
			ID:          "plan-owner-gap",
			TargetPaths: []string{"pkg/bug.py", "tests/test_bug.py"},
			Changes: []FileChange{{
				Path: "pkg/bug.py",
				Kind: "modify",
			}},
		},
	})

	if len(report.Plan.OwnerAnchorGaps) != 1 {
		t.Fatalf("OwnerAnchorGaps=%+v, want one source owner gap", report.Plan.OwnerAnchorGaps)
	}
	gap := report.Plan.OwnerAnchorGaps[0]
	if gap.Path != "pkg/bug.py" ||
		gap.ReasonCode != "plan_source_path_without_owner_anchor" ||
		gap.RequiredEvidence != "typed_owner_localization_anchor" ||
		gap.Source != "prior_context" {
		t.Fatalf("wrong owner gap row: %+v", gap)
	}
	if !writeFinalReportHasRisk(report, "source_owner_anchor_missing") {
		t.Fatalf("ResidualRisks=%+v missing owner-anchor gap risk", report.ResidualRisks)
	}
}

func TestBuildWriteFinalReportDoesNotProjectOwnerGapWhenEvidenceAnchorExists(t *testing.T) {
	ref := WriteExplorationEvidenceRef{ID: "ev-owner", Source: "pkg/bug.py", LineStart: 12, OwnerSymbol: "Owner.fix"}
	report := BuildWriteFinalReport(WriteFinalReportInput{
		Run: &WriteWorkflowRun{
			RunID:  "run-owner-covered",
			Status: WriteWorkflowRunComplete,
			ContextPacks: []WriteContextPack{{
				PackID:      "exploration-handoff",
				BatchID:     "batch-1",
				SourceStage: "explore",
				Items: []WriteContextItem{{
					Priority:    WriteContextP1,
					Kind:        "localization_anchor",
					Text:        "path=pkg/bug.py owner=Owner.fix",
					SourceStage: "explore",
					Consumers:   []WriteContextConsumer{WriteConsumerPlanner, WriteConsumerController},
					EvidenceRef: &ref,
					LocalizationAnchor: &SourceLocalizationAnchor{
						Path:        "pkg/bug.py",
						Role:        SourcePathRoleProduction,
						Kind:        SourceLocalizationAnchorGroundedEvidence,
						Strength:    SourceLocalizationAnchorOwner,
						EvidenceRef: &ref,
						OwnerSymbol: "Owner.fix",
					},
				}},
			}},
		},
		Plan: &ChangePlan{
			ID:          "plan-owner-covered",
			TargetPaths: []string{"pkg/bug.py"},
			Changes: []FileChange{{
				Path: "pkg/bug.py",
				Kind: "modify",
			}},
		},
	})

	if len(report.Plan.OwnerAnchorGaps) != 0 {
		t.Fatalf("OwnerAnchorGaps=%+v, want no gap with typed owner evidence", report.Plan.OwnerAnchorGaps)
	}
	if writeFinalReportHasRisk(report, "source_owner_anchor_missing") {
		t.Fatalf("ResidualRisks=%+v should not include owner-anchor gap", report.ResidualRisks)
	}
}

func TestBuildWriteFinalReportUsesCumulativeProofArtifacts(t *testing.T) {
	primaryReport := &ChangeReport{
		PlanID:             "plan-proof",
		Passed:             true,
		VerificationStatus: VerificationStatusPassed,
		ExecutedCommands: []ExecutedCommand{{
			Runner:  "verification_probe",
			Suite:   "verification_probe/python",
			Outcome: "executed",
			Source:  "python_verification_probe",
		}},
		VerificationConfidence: []VerificationConfidenceRecord{{
			Source:       "verification_probe",
			Category:     "probe_soft_contract_refs",
			Status:       "missing",
			ReasonCode:   "verification_probe_missing_soft_contract_ref",
			ContractRefs: []string{"outcome-1"},
		}},
	}
	sourceReport := &ChangeReport{
		PlanID:             "plan-source",
		Passed:             true,
		VerificationStatus: VerificationStatusPassed,
		ExecutedCommands: []ExecutedCommand{{
			Runner:  "verification_probe",
			Suite:   "verification_probe/python",
			Outcome: "executed",
			Source:  "python_verification_probe",
		}},
		VerificationConfidence: []VerificationConfidenceRecord{{
			Source:       "verification_probe",
			Category:     "probe_soft_contract_refs",
			Status:       "satisfied",
			ReasonCode:   "verification_probe_soft_contract_ref_covered",
			ContractRefs: []string{"outcome-1"},
		}},
	}

	got := BuildWriteFinalReport(WriteFinalReportInput{
		Run: &WriteWorkflowRun{
			RunID:  "run-proof",
			Status: WriteWorkflowRunComplete,
			Completion: &WriteWorkflowCompletion{
				Verdict: WriteWorkflowCompletionVerified,
			},
		},
		Report: primaryReport,
		ProofArtifacts: []VerificationProofArtifact{{
			Report: sourceReport,
		}},
	})

	if got.Proof.Status != VerificationProofAdequate || !got.Proof.Cumulative {
		t.Fatalf("Proof=%+v, want cumulative adequate profile", got.Proof)
	}
	if writeFinalProofHasReason(got.Proof, "verification_probe_missing_soft_contract_ref") {
		t.Fatalf("Proof reasons=%+v should not retain resolved soft-contract gap", got.Proof.ReasonCodes)
	}
}

func TestWriteOutputKindIncludesFinalReport(t *testing.T) {
	if !IsValidWriteOutputKind(WriteOutputFinalReport) {
		t.Fatal("WriteOutputFinalReport must be declared")
	}
}

func writeFinalReportHasRisk(report WriteFinalReport, code string) bool {
	for _, risk := range report.ResidualRisks {
		if risk.Code == code {
			return true
		}
	}
	return false
}

func writeFinalProofHasReason(profile VerificationProofProfile, code string) bool {
	for _, reason := range profile.ReasonCodes {
		if reason == code {
			return true
		}
	}
	return false
}
