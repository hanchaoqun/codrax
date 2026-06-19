package types

import (
	"path/filepath"
	"strings"
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
	if !verificationLanguageFamilyContains(got.Patch.LanguageFamilies, VerificationLanguagePython) {
		t.Fatalf("Patch.LanguageFamilies=%+v, want python", got.Patch.LanguageFamilies)
	}
	if got.Verification.Status != VerificationStatusUnavailable || got.Verification.FailureKind != FailureKindRunnerMissing {
		t.Fatalf("Verification=%+v, want unavailable runner_missing", got.Verification)
	}
	if !verificationLanguageFamilyContains(got.Verification.RunnerFamilies, VerificationLanguagePython) {
		t.Fatalf("Verification.RunnerFamilies=%+v, want python", got.Verification.RunnerFamilies)
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
	if got.ProofLedger.State != VerificationProofLedgerUnavailable ||
		got.ProofLedger.ProfileStatus != VerificationProofUnavailable ||
		!writeFinalProofLedgerHasCapability(got.ProofLedger, "local_verification", VerificationProofLedgerItemUnavailable) {
		t.Fatalf("ProofLedger=%+v, want unavailable ledger with uncovered obligations", got.ProofLedger)
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
	report.Loop = &WriteFinalLoopSummary{
		RunID:          "run-1",
		Status:         "complete",
		ActiveUnitID:   "slice-1",
		EventCount:     3,
		LastEventKind:  "run_completed",
		LastReasonCode: "tests_passed",
		EventRefs:      []string{" event-1 ", "event-1", "event-2"},
		RuntimeUnits: []WriteFinalRuntimeUnitSummary{{
			UnitID: "slice-1",
			Status: "complete",
			Truth: TruthLedger{
				State:      TruthLedgerCovered,
				ReasonCode: "tests_passed",
			},
		}},
		Truth: TruthLedger{
			State:      TruthLedgerCovered,
			ReasonCode: "tests_passed",
		},
	}
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
	if got.Loop == nil || got.Loop.EventCount != 3 || strings.Join(got.Loop.EventRefs, ",") != "event-1,event-2" {
		t.Fatalf("Loop summary round trip=%+v", got.Loop)
	}
	if len(got.Loop.RuntimeUnits) != 1 || got.Loop.RuntimeUnits[0].Truth.State != TruthLedgerCovered {
		t.Fatalf("Loop runtime units=%+v, want covered unit truth", got.Loop.RuntimeUnits)
	}
}

func TestBuildWriteFinalReportProjectsPrimarySourceAuthoritySeparatelyFromTerminalPlan(t *testing.T) {
	terminalPlan := &ChangePlan{
		ID:           "plan-proof",
		Status:       PlanStatusApplied,
		TargetPaths:  []string{"tests/test_bug.py"},
		AppliedPaths: []string{"tests/test_bug.py"},
		Changes: []FileChange{{
			Path: "tests/test_bug.py",
			Kind: "patch",
		}},
	}
	sourcePlan := &ChangePlan{
		ID:           "plan-source",
		Status:       PlanStatusVerifyFailed,
		TargetPaths:  []string{"pkg/bug.py", "tests/test_bug.py"},
		AppliedPaths: []string{"pkg/bug.py", "tests/test_bug.py"},
		Changes: []FileChange{{
			Path: "pkg/bug.py",
			Kind: "patch",
		}, {
			Path: "tests/test_bug.py",
			Kind: "patch",
		}},
		LocalizationReview: &SourceLocalizationReview{
			Status:              SourceLocalizationSupported,
			Source:              "write_plan_context",
			PlanID:              "plan-source",
			SourcePaths:         []string{"pkg/bug.py"},
			SupportedPaths:      []string{"pkg/bug.py"},
			OwnerSupportedPaths: []string{"pkg/bug.py"},
			Anchors: []SourceLocalizationAnchor{{
				Path:         "pkg/bug.py",
				Role:         SourcePathRoleProduction,
				Kind:         SourceLocalizationAnchorGroundedEvidence,
				Strength:     SourceLocalizationAnchorOwner,
				OwnerSymbol:  "BugOwner",
				AnchorSymbol: "BugOwner.fix",
			}},
		},
	}

	got := BuildWriteFinalReport(WriteFinalReportInput{
		Run: &WriteWorkflowRun{
			RunID:  "run-primary-source",
			Status: WriteWorkflowRunComplete,
		},
		Plan:              terminalPlan,
		PrimarySourcePlan: sourcePlan,
		Report: &ChangeReport{
			PlanID:             "plan-proof",
			Passed:             true,
			VerificationStatus: VerificationStatusPassed,
		},
		Delivery: WriteFinalDeliverySummary{
			Status:              "coherent",
			Relation:            "source_plan_with_later_test_followup",
			FinalPlanID:         "plan-proof",
			ReportPlanID:        "plan-proof",
			PrimarySourcePlanID: "plan-source",
			SourceOwnerPlanIDs:  []string{"plan-source"},
			SourcePaths:         []string{"pkg/bug.py"},
			TestPaths:           []string{"tests/test_bug.py"},
			FinalPlanTestOnly:   true,
		},
	})

	if got.Plan.ID != "plan-proof" || len(got.Plan.SourcePaths) != 0 {
		t.Fatalf("terminal plan summary = %+v, want test-only proof plan", got.Plan)
	}
	if got.SourceAuthority.PlanID != "plan-source" ||
		strings.Join(got.SourceAuthority.SourcePaths, ",") != "pkg/bug.py" {
		t.Fatalf("SourceAuthority=%+v, want primary source plan path", got.SourceAuthority)
	}
	if got.SourceAuthority.Localization == nil ||
		got.SourceAuthority.Localization.Status != SourceLocalizationSupported ||
		strings.Join(got.SourceAuthority.Localization.OwnerSupportedPaths, ",") != "pkg/bug.py" {
		t.Fatalf("SourceAuthority.Localization=%+v, want supported owner localization", got.SourceAuthority.Localization)
	}
	if len(got.SourceAuthority.OwnerAnchors) == 0 || got.SourceAuthority.OwnerAnchors[0].OwnerSymbol != "BugOwner" {
		t.Fatalf("SourceAuthority.OwnerAnchors=%+v, want source owner anchor", got.SourceAuthority.OwnerAnchors)
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
	if report.Plan.Localization == nil ||
		report.Plan.Localization.Status != SourceLocalizationWeak ||
		strings.Join(report.Plan.Localization.OwnerMissingPaths, ",") != "pkg/bug.py" {
		t.Fatalf("Plan.Localization=%+v, want weak owner-authority gap", report.Plan.Localization)
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

func TestNormalizeWriteFinalReportDowngradesSupportedLocalizationWithOwnerGap(t *testing.T) {
	report := NormalizeWriteFinalReport(WriteFinalReport{
		Plan: WriteFinalPlanSummary{
			Localization: &SourceLocalizationReview{
				Status:         SourceLocalizationSupported,
				SourcePaths:    []string{"pkg/bug.py"},
				SupportedPaths: []string{"pkg/bug.py"},
			},
			OwnerAnchorGaps: []WriteFinalOwnerGap{{
				Path:             "pkg/bug.py",
				ReasonCode:       "plan_source_path_without_owner_anchor",
				RequiredEvidence: "typed_owner_localization_anchor",
				Source:           "prior_context",
			}},
		},
	})

	if report.Plan.Localization == nil || report.Plan.Localization.Status != SourceLocalizationWeak {
		t.Fatalf("Plan.Localization=%+v, want weak after owner-gap normalization", report.Plan.Localization)
	}
	if strings.Join(report.Plan.Localization.OwnerMissingPaths, ",") != "pkg/bug.py" {
		t.Fatalf("owner missing paths = %+v", report.Plan.Localization.OwnerMissingPaths)
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
	if got.ProofLedger.State != VerificationProofLedgerVerified || !got.ProofLedger.Cumulative {
		t.Fatalf("ProofLedger=%+v, want cumulative verified ledger", got.ProofLedger)
	}
	if writeFinalProofHasReason(got.Proof, "verification_probe_missing_soft_contract_ref") {
		t.Fatalf("Proof reasons=%+v should not retain resolved soft-contract gap", got.Proof.ReasonCodes)
	}
}

func TestWriteOutputKindIncludesFinalReport(t *testing.T) {
	if !IsValidWriteOutputKind(WriteOutputFinalReport) {
		t.Fatal("WriteOutputFinalReport must be declared")
	}
	if !IsValidWriteOutputKind(WriteOutputFinalAudit) {
		t.Fatal("WriteOutputFinalAudit must be declared")
	}
}

func TestBuildWriteFinalAuditSummaryProjectsTypedReplayFields(t *testing.T) {
	report := NormalizeWriteFinalReport(WriteFinalReport{
		Kind:      WriteOutputFinalReport,
		RunID:     "run-audit",
		RunStatus: WriteWorkflowRunComplete,
		Completion: &WriteWorkflowCompletion{
			Verdict:    WriteWorkflowCompletionVerified,
			ReasonCode: "tests_passed",
		},
		ActiveBatchID: "batch-1",
		Batches: []WriteFinalBatchSummary{{
			ID:     "batch-1",
			Status: WriteWorkflowBatchComplete,
			PlanID: "plan-audit",
		}},
		Artifacts: WriteFinalArtifactRefs{
			PlanPath:     "/tmp/plan-audit.json",
			WorkflowPath: "/tmp/workflows/run-audit.json",
		},
		Patch: WriteFinalPatchSummary{
			ChangedFiles:     []string{"pkg/fix.py"},
			LanguageFamilies: []VerificationLanguageFamily{VerificationLanguagePython},
		},
		Verification: WriteFinalVerificationSummary{
			Status:         VerificationStatusPassed,
			Passed:         true,
			RunnerFamilies: []VerificationLanguageFamily{VerificationLanguagePython},
		},
		Handoff: WriteFinalHandoffSummary{
			TopItems: []WriteFinalHandoffItem{{
				Priority:    WriteContextP1,
				Kind:        "owner_anchor",
				EvidenceRef: "file.py:12",
			}},
		},
		Loop: &WriteFinalLoopSummary{
			RunID:          "run-audit",
			Status:         "complete",
			ActiveUnitID:   "slice-1",
			EventCount:     2,
			LastEventKind:  "truth_projected",
			LastReasonCode: "tests_passed",
			EventRefs:      []string{"event-1", "event-2"},
			Truth: TruthLedger{
				State:      TruthLedgerCovered,
				ReasonCode: "tests_passed",
			},
			Proof: WriteFinalAuthoritySummary{
				State:      "covered",
				ReasonCode: "proof_covered",
			},
			Localization: WriteFinalAuthoritySummary{
				State:       "supported",
				ReasonCode:  "owner_anchor_supported",
				SourcePaths: []string{"pkg/fix.py"},
			},
			Permission: WriteFinalAuthoritySummary{
				State:      "allow",
				ReasonCode: "low_risk",
			},
		},
		ResidualRisks: []WriteFinalResidualRisk{{
			Code:     "local_verify_only",
			Severity: "info",
		}},
	})

	got := BuildWriteFinalAuditSummary(report, "/tmp/plan-audit.final.json")
	if got.Kind != WriteOutputFinalAudit || got.SchemaVersion != WriteFinalAuditSchemaVersion {
		t.Fatalf("audit identity = kind %q schema %d", got.Kind, got.SchemaVersion)
	}
	if got.RunID != "run-audit" || got.BatchCount != 1 || got.Completion == nil || got.Completion.Verdict != WriteWorkflowCompletionVerified {
		t.Fatalf("audit run summary = %+v", got)
	}
	if got.Artifacts.ReportPath != "/tmp/plan-audit.final.json" || got.ReportPath != "/tmp/plan-audit.final.json" {
		t.Fatalf("audit report path = %+v artifacts=%+v", got.ReportPath, got.Artifacts)
	}
	if got.Loop == nil || got.Loop.Truth.State != TruthLedgerCovered || strings.Join(got.Loop.EventRefs, ",") != "event-1,event-2" {
		t.Fatalf("audit loop = %+v", got.Loop)
	}
	if len(got.Patch.LanguageFamilies) != 1 || got.Patch.LanguageFamilies[0] != VerificationLanguagePython {
		t.Fatalf("audit patch languages = %+v", got.Patch.LanguageFamilies)
	}
	if len(got.Verification.RunnerFamilies) != 1 || got.Verification.RunnerFamilies[0] != VerificationLanguagePython {
		t.Fatalf("audit runner languages = %+v", got.Verification.RunnerFamilies)
	}
	if len(got.Handoff.TopItems) != 1 || got.Handoff.TopItems[0].EvidenceRef != "file.py:12" {
		t.Fatalf("audit handoff = %+v", got.Handoff)
	}
	if len(got.ResidualRisks) != 1 || got.ResidualRisks[0].Code != "local_verify_only" {
		t.Fatalf("audit risks = %+v", got.ResidualRisks)
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

func writeFinalProofLedgerHasCapability(ledger VerificationProofLedger, kind string, status VerificationProofLedgerItemStatus) bool {
	for _, item := range ledger.Capabilities {
		if item.Kind == kind && item.Status == status {
			return true
		}
	}
	return false
}
