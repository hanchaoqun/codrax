package types

import "testing"

func TestBuildVerificationProofProfileStrongProjectRunner(t *testing.T) {
	plan := &ChangePlan{
		ID: "plan-strong",
		PatchReview: &PatchReviewRecord{Findings: []PatchReviewFinding{{
			Code:           "changed_symbol_without_probe_coverage",
			Category:       PatchReviewCategorySemanticCoverage,
			CoverageStatus: PatchReviewCoverageVerified,
		}}},
		ImpactAnalysis: &ImpactAnalysisResult{VerificationTargets: []ImpactVerificationTarget{{
			Kind:           "changed_symbol",
			Path:           "src/app.go",
			CoverageStatus: "verified",
		}}},
		LocalizationReview: &SourceLocalizationReview{
			Status:         SourceLocalizationSupported,
			SourcePaths:    []string{"src/app.go"},
			SupportedPaths: []string{"src/app.go"},
		},
	}
	report := &ChangeReport{
		PlanID:             "plan-strong",
		Passed:             true,
		VerificationStatus: VerificationStatusPassed,
		TestResults: []TestResult{{
			AssertionID: "TestApp",
			Suite:       "pkg/app",
			Passed:      true,
		}},
		ExecutedCommands: []ExecutedCommand{{
			Runner:  "go",
			Suite:   "./...",
			Command: "go test ./...",
			Outcome: "executed",
			Source:  "auto_detect",
		}},
	}

	got := BuildVerificationProofProfile(plan, report)
	if got.Status != VerificationProofStrong || got.RunnerEvidence != VerificationProofRunnerProject {
		t.Fatalf("profile=%+v, want strong project runner", got)
	}
	if got.TestCount != 1 || got.ProjectRunnerCommands != 1 || got.ImpactVerifiedCount != 1 {
		t.Fatalf("profile counts not projected: %+v", got)
	}
	if len(got.ReasonCodes) != 0 {
		t.Fatalf("strong proof should not carry risk reasons: %+v", got.ReasonCodes)
	}
}

func TestBuildVerificationProofProfileDoesNotPromoteSourceStaticToBehaviorProof(t *testing.T) {
	plan := &ChangePlan{
		ID: "plan-static-only",
		BehaviorContracts: NormalizeWriteBehaviorContracts([]WriteBehaviorContract{{
			ID:       "ascii-boundary",
			Kind:     WriteBehaviorInvariant,
			Polarity: WriteBehaviorPolarityExpected,
			Subject:  "ascii boundary",
			Operator: WriteBehaviorOpEquals,
			Expected: "non-ASCII letters remain available",
		}}, nil),
	}
	report := &ChangeReport{
		PlanID:             plan.ID,
		Passed:             true,
		VerificationStatus: VerificationStatusPassed,
		ExecutedCommands: []ExecutedCommand{{
			Runner: "make", Suite: "check", Command: "make check", Outcome: "executed",
		}},
		ChangedPathCoverage: []ChangedPathVerificationCoverage{{
			Path:       "src/RandomStringUtils.java",
			Status:     ChangedPathVerificationCovered,
			Caliber:    ChangedPathVerificationDeclaredProjectCheck,
			Capability: VerificationCapabilitySourceStatic,
		}},
	}

	got := BuildVerificationProofProfile(plan, report)
	if got.Status != VerificationProofWeak || !verificationProofHasReason(got, "target_behavior_verification_missing") {
		t.Fatalf("source-static check must not prove hard behavior contract: %+v", got)
	}
	if got.SourceStaticPaths != 1 || got.TargetBehaviorPaths != 0 {
		t.Fatalf("capability counts wrong: %+v", got)
	}
	ledger := BuildVerificationProofLedger(plan, report, nil)
	if ledger.State != VerificationProofLedgerLowConfidence {
		t.Fatalf("ledger state=%q, want low_confidence: %+v", ledger.State, ledger)
	}
	found := false
	for _, item := range ledger.Capabilities {
		if item.Kind == "changed_path_verification" && item.Capability == VerificationCapabilitySourceStatic {
			found = true
		}
	}
	if !found {
		t.Fatalf("ledger omitted source-static capability: %+v", ledger.Capabilities)
	}
	foundMissingContract := false
	for _, item := range ledger.Obligations {
		if item.Kind == "behavior_contract" &&
			item.Status == VerificationProofLedgerItemMissing &&
			item.ContractRef == "ascii-boundary" &&
			item.ReasonCode == "behavior_contract_observation_missing" &&
			item.Source == "change_plan_behavior_contract" {
			foundMissingContract = true
		}
	}
	if !foundMissingContract {
		t.Fatalf("ledger omitted schedulable hard behavior-contract obligation: %+v", ledger.Obligations)
	}
}

func TestBuildVerificationProofLedgerResolvesExactProjectTestContractReceipt(t *testing.T) {
	plan := &ChangePlan{
		ID: "plan-project-observation",
		BehaviorContracts: NormalizeWriteBehaviorContracts([]WriteBehaviorContract{{
			ID: "ordinary-number-format", Kind: WriteBehaviorInvariant,
			Polarity: WriteBehaviorPolarityExpected, Subject: "ordinary numbers",
			Operator: WriteBehaviorOpSatisfies, Expected: "remain unchanged",
		}}, nil),
	}
	report := &ChangeReport{
		PlanID: plan.ID, Passed: true, VerificationStatus: VerificationStatusPassed,
		ExecutedCommands: []ExecutedCommand{{Runner: "make", Suite: "check", Outcome: "executed"}},
		ChangedPathCoverage: []ChangedPathVerificationCoverage{{
			Path: "include/json.hpp", Status: ChangedPathVerificationCovered,
			Caliber: ChangedPathVerificationProjectRunner, Capability: VerificationCapabilityTargetBehavior,
		}},
		VerificationConfidence: []VerificationConfidenceRecord{{
			Source: "project_test_observation", Category: "project_test_contract_refs",
			Status: "satisfied", ReasonCode: "project_test_contract_ref_observed",
			ContractRefs: []string{"ordinary-number-format"},
		}},
	}

	ledger := BuildVerificationProofLedger(plan, report, nil)
	if ledger.State != VerificationProofLedgerVerified || ledger.UncoveredCount != 0 {
		t.Fatalf("exact project-test receipt did not resolve behavior contract: %+v", ledger)
	}
	if !verificationProofLedgerHasItem(ledger, "behavior_contract", VerificationProofLedgerItemCovered, "project_test_contract_ref_observed") {
		t.Fatalf("project-test contract receipt missing from ledger: %+v", ledger.Obligations)
	}
}

func TestBuildVerificationProofLedgerResolvesExactSourceContractReceipt(t *testing.T) {
	plan := &ChangePlan{
		ID: "plan-source-observation",
		BehaviorContracts: []WriteBehaviorContract{{
			ID: "return-line", Kind: WriteBehaviorObservable,
			Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpEquals,
			Expected: "return buf;", EvidenceRef: "main.c:19", Required: true,
		}},
	}
	report := &ChangeReport{
		PlanID: plan.ID, Passed: true, VerificationStatus: VerificationStatusPassed,
		ExecutedCommands: []ExecutedCommand{{Runner: "make", Suite: "test", Outcome: "executed", ExitCode: 0}},
		ChangedPathCoverage: []ChangedPathVerificationCoverage{{
			Path: "main.c", Status: ChangedPathVerificationCovered,
			Caliber: ChangedPathVerificationProjectRunner, Capability: VerificationCapabilityTargetBehavior,
		}},
		VerificationConfidence: []VerificationConfidenceRecord{{
			Source: "post_apply_source_observation", Category: "source_contract_refs",
			Status: "satisfied", ReasonCode: "post_apply_source_contract_observed",
			ContractRefs: []string{"return-line"},
		}},
	}

	profile := BuildVerificationProofProfile(plan, report)
	if profile.Status != VerificationProofStrong || verificationProofHasReason(profile, "behavior_contract_observation_missing") {
		t.Fatalf("exact source receipt did not close the source-value contract: %+v", profile)
	}
	ledger := BuildVerificationProofLedger(plan, report, nil)
	if ledger.State != VerificationProofLedgerVerified || ledger.UncoveredCount != 0 {
		t.Fatalf("exact source receipt did not close proof ledger: %+v", ledger)
	}
	if !verificationProofLedgerHasItem(ledger, "behavior_contract", VerificationProofLedgerItemCovered, "post_apply_source_contract_observed") {
		t.Fatalf("source contract receipt missing from ledger: %+v", ledger.Obligations)
	}
}

func TestBuildVerificationProofLedgerIgnoresPersistedPlanningOnlyProjectDebt(t *testing.T) {
	plan := &ChangePlan{
		ID: "plan-planning-only-project-observation",
		BehaviorContracts: []WriteBehaviorContract{{
			ID: "planning-sync", Kind: WriteBehaviorInvariant,
			Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpSatisfies,
			Expected: "generated headers remain synchronized", Required: false,
			Source: "write_analyzer;" + WriteBehaviorContractSourcePlanningOnlyUngrounded,
		}},
	}
	report := &ChangeReport{
		PlanID: plan.ID, Passed: true, VerificationStatus: VerificationStatusPassed,
		ExecutedCommands: []ExecutedCommand{{Runner: "make", Suite: "check", Outcome: "executed"}},
		ChangedPathCoverage: []ChangedPathVerificationCoverage{{
			Path: "include/json.hpp", Status: ChangedPathVerificationCovered,
			Caliber: ChangedPathVerificationProjectRunner, Capability: VerificationCapabilityTargetBehavior,
		}},
		// Resume compatibility: reports written before the authority fix may
		// contain this record. It must not recreate a required obligation.
		VerificationConfidence: []VerificationConfidenceRecord{{
			Source: "project_test_observation", Category: "project_test_contract_refs",
			Status: "missing", ReasonCode: "project_test_assertion_not_observed",
			ContractRefs: []string{"planning-sync"},
		}},
	}

	profile := BuildVerificationProofProfile(plan, report)
	if profile.Status != VerificationProofStrong || verificationProofHasReason(profile, "project_test_assertion_not_observed") {
		t.Fatalf("planning-only project observation weakened proof profile: %+v", profile)
	}
	ledger := BuildVerificationProofLedger(plan, report, nil)
	if ledger.State != VerificationProofLedgerVerified || ledger.UncoveredCount != 0 {
		t.Fatalf("planning-only project observation entered proof ledger: %+v", ledger)
	}
}

func TestBuildVerificationProofProfileSingleReportResolvesProbeMissingRefWithExactProjectReceipt(t *testing.T) {
	plan := &ChangePlan{
		ID: "plan-mixed-contract-proof",
		BehaviorContracts: []WriteBehaviorContract{
			{ID: "outcome-project", Kind: WriteBehaviorObservable, Polarity: WriteBehaviorPolarityExpected,
				Operator: WriteBehaviorOpSatisfies, Expected: "project test observes behavior", Required: true},
			{ID: "outcome-probe", Kind: WriteBehaviorObservable, Polarity: WriteBehaviorPolarityExpected,
				Operator: WriteBehaviorOpSatisfies, Expected: "probe observes fallback", Required: true},
		},
	}
	report := &ChangeReport{
		PlanID: plan.ID, Passed: true, VerificationStatus: VerificationStatusPassed,
		TestResults: []TestResult{
			{AssertionID: "project-case", Suite: "pkg/project", Passed: true},
			{AssertionID: "probe-case", Suite: "verification_probe/python", Passed: true},
		},
		ExecutedCommands: []ExecutedCommand{
			{Runner: "python", Suite: "pkg/project", Outcome: "executed", Source: "declared_coverage_test_surface"},
			{Runner: "verification_probe", Suite: "verification_probe/python", Outcome: "executed", Source: "pre_suite_verification_probe"},
		},
		VerificationConfidence: []VerificationConfidenceRecord{
			{Source: "project_test_observation", Category: "project_test_contract_refs", Status: "satisfied",
				ReasonCode: "project_test_contract_ref_observed", ContractRefs: []string{"outcome-project"}},
			{Source: "verification_probe", Category: "probe_soft_contract_refs", Status: "missing",
				ReasonCode: "verification_probe_missing_soft_contract_ref", ContractRefs: []string{"outcome-project"}},
			{Source: "verification_probe", Category: "probe_soft_contract_refs", Status: "satisfied",
				ReasonCode: "verification_probe_soft_contract_ref_covered", ContractRefs: []string{"outcome-probe"}},
		},
	}

	profile := BuildVerificationProofProfile(plan, report)
	if profile.Status != VerificationProofStrong ||
		verificationProofHasReason(profile, "verification_probe_missing_soft_contract_ref") {
		t.Fatalf("exact report-local project receipt did not resolve the probe-only missing ref: %+v", profile)
	}
	ledger := BuildVerificationProofLedger(plan, report, nil)
	if ledger.State != VerificationProofLedgerVerified || ledger.UncoveredCount != 0 ||
		ledger.UnavailableCount != 0 || ledger.FailedCount != 0 {
		t.Fatalf("closed report-local mixed proof did not produce a verified ledger: %+v", ledger)
	}

	report.VerificationConfidence[0].ContractRefs = []string{"different-outcome"}
	profile = BuildVerificationProofProfile(plan, report)
	if profile.Status != VerificationProofWeak ||
		!verificationProofHasReason(profile, "verification_probe_missing_soft_contract_ref") {
		t.Fatalf("unrelated project receipt must not resolve the exact missing ref: %+v", profile)
	}
}

func TestBuildVerificationProofProfileTargetBehaviorDoesNotBlanketSignContracts(t *testing.T) {
	plan := &ChangePlan{
		ID: "plan-behavior",
		BehaviorContracts: NormalizeWriteBehaviorContracts([]WriteBehaviorContract{{
			ID: "contract", Kind: WriteBehaviorInvariant, Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpEquals,
			Subject: "value", Expected: "preserved",
		}}, nil),
	}
	report := &ChangeReport{
		PlanID: plan.ID, Passed: true, VerificationStatus: VerificationStatusPassed,
		ExecutedCommands: []ExecutedCommand{{Runner: "java", Outcome: "executed"}},
		ChangedPathCoverage: []ChangedPathVerificationCoverage{{
			Path: "src/Widget.java", Status: ChangedPathVerificationCovered,
			Caliber: ChangedPathVerificationProjectRunner, Capability: VerificationCapabilityTargetBehavior,
		}},
	}

	got := BuildVerificationProofProfile(plan, report)
	if got.Status != VerificationProofWeak || got.TargetBehaviorPaths != 1 ||
		!verificationProofHasReason(got, "behavior_contract_observation_missing") {
		t.Fatalf("path behavior capability must not blanket-sign an independent contract: %+v", got)
	}
	ledger := BuildVerificationProofLedger(plan, report, nil)
	foundMissing := false
	for _, item := range ledger.Obligations {
		if item.Source == "change_plan_behavior_contract" && item.Status == VerificationProofLedgerItemMissing {
			foundMissing = true
		}
	}
	if !foundMissing {
		t.Fatalf("exact contract observation debt was not retained: %+v", ledger.Obligations)
	}

	report.VerificationConfidence = []VerificationConfidenceRecord{{
		Source:       "verification_probe",
		Category:     "probe_contract_refs",
		Status:       "satisfied",
		ReasonCode:   "verification_probe_contract_ref_covered",
		ContractRefs: []string{"contract"},
	}}
	got = BuildVerificationProofProfile(plan, report)
	if got.Status != VerificationProofStrong || verificationProofHasReason(got, "behavior_contract_observation_missing") {
		t.Fatalf("exact executed contract receipt should restore strong proof: %+v", got)
	}
	ledger = BuildVerificationProofLedger(plan, report, nil)
	if ledger.State != VerificationProofLedgerVerified || ledger.UncoveredCount != 0 {
		t.Fatalf("exact contract receipt did not close ledger: %+v", ledger)
	}
}

func TestBuildVerificationProofLedgerExpectedOutcomeFallbackIsPlanningOnly(t *testing.T) {
	plan := &ChangePlan{
		ID: "plan-fallback-observation",
		BehaviorContracts: []WriteBehaviorContract{{
			ID:       "outcome-1",
			Kind:     WriteBehaviorObservable,
			Polarity: WriteBehaviorPolarityExpected,
			Operator: WriteBehaviorOpSatisfies,
			Expected: "ordinary floating point remains unchanged",
			Required: true,
			Source:   WriteBehaviorContractSourceExpectedOutcomeFallback,
		}},
	}
	report := &ChangeReport{
		PlanID: plan.ID, Passed: true, VerificationStatus: VerificationStatusPassed,
		ExecutedCommands: []ExecutedCommand{{Runner: "make", Suite: "check", Outcome: "executed"}},
		ChangedPathCoverage: []ChangedPathVerificationCoverage{{
			Path: "serializer.hpp", Status: ChangedPathVerificationCovered,
			Caliber: ChangedPathVerificationProjectRunner, Capability: VerificationCapabilityTargetBehavior,
		}},
	}

	ledger := BuildVerificationProofLedger(plan, report, nil)
	if ledger.State != VerificationProofLedgerVerified || ledger.UncoveredCount != 0 || ledger.ObligationCount != 0 {
		t.Fatalf("planning-only expected-outcome fallback entered proof authority: %+v", ledger)
	}
}

func TestVerificationProofLedgerSupersedesHistoricalExactCommandFailure(t *testing.T) {
	primary := &ChangeReport{
		PlanID: "plan-new", Passed: true, VerificationStatus: VerificationStatusPassed,
		ExecutedCommands: []ExecutedCommand{{
			Runner: "make", WorkingDir: ".", Suite: "check", Command: "make check", Outcome: "executed", ExitCode: 0,
		}},
	}
	historical := &ChangeReport{
		PlanID: "plan-old", Passed: false, VerificationStatus: VerificationStatusFailed,
		FailureKind: FailureKindTestsFailed,
		ExecutedCommands: []ExecutedCommand{{
			Runner: "make", WorkingDir: ".", Suite: "check", Command: "make check", Outcome: "executed", ExitCode: 1,
		}},
	}

	ledger := BuildVerificationProofLedger(nil, primary, []VerificationProofArtifact{{Report: historical}})
	if ledger.State == VerificationProofLedgerFailed || ledger.CapabilityFailedCount != 0 {
		t.Fatalf("terminal exact rerun should supersede historical command failure: %+v", ledger)
	}
	foundAdvisory := false
	for _, item := range ledger.Capabilities {
		if item.ReportPlanID == "plan-old" && item.Status == VerificationProofLedgerItemAdvisory &&
			item.ReasonCode == "superseded_by_terminal_exact_command_pass" {
			foundAdvisory = true
		}
	}
	if !foundAdvisory {
		t.Fatalf("historical failure was not preserved as superseded advisory: %+v", ledger.Capabilities)
	}
}

func TestBuildVerificationProofProfileWeakProbeOnly(t *testing.T) {
	plan := &ChangePlan{
		ID: "plan-weak",
		PatchReview: &PatchReviewRecord{Findings: []PatchReviewFinding{{
			Code:           "behavior_contract_without_verify_coverage",
			Category:       PatchReviewCategorySemanticCoverage,
			CoverageStatus: PatchReviewCoverageUnverified,
			ImpactKind:     PatchReviewImpactKindBehaviorContract,
		}}},
		ImpactAnalysis: &ImpactAnalysisResult{VerificationTargets: []ImpactVerificationTarget{{
			Kind:           "behavior_contract",
			ContractRef:    "contract-1",
			CoverageStatus: "unverified",
		}}},
		LocalizationReview: &SourceLocalizationReview{
			Status:       SourceLocalizationWeak,
			SourcePaths:  []string{"src/app.py"},
			MissingPaths: []string{"src/app.py"},
			ReasonCodes:  []string{"plan_source_paths_without_prior_context"},
		},
	}
	report := &ChangeReport{
		PlanID:             "plan-weak",
		Passed:             true,
		VerificationStatus: VerificationStatusPassed,
		TestResults: []TestResult{{
			AssertionID: "probe/python",
			Suite:       "verification_probe/python",
			Passed:      true,
		}},
		ExecutedCommands: []ExecutedCommand{{
			Runner:  "verification_probe",
			Suite:   "verification_probe/python",
			Outcome: "executed",
			Source:  "python_verification_probe",
		}},
		VerificationConfidence: []VerificationConfidenceRecord{{
			Source:     "verification_probe",
			Category:   "probe_contract_refs",
			Status:     "missing",
			ReasonCode: "verification_probe_missing_required_contract_ref",
		}},
	}

	got := BuildVerificationProofProfile(plan, report)
	if got.Status != VerificationProofWeak || got.RunnerEvidence != VerificationProofRunnerVerificationProbe {
		t.Fatalf("profile=%+v, want weak verification probe", got)
	}
	for _, want := range []string{
		"verification_probe_missing_required_contract_ref",
		"patch_review_semantic_unverified",
		"impact_targets_unverified",
		"source_localization_weak",
	} {
		if !verificationProofHasReason(got, want) {
			t.Fatalf("profile reasons %+v missing %q", got.ReasonCodes, want)
		}
	}
}

func TestBuildVerificationProofProfileUnavailable(t *testing.T) {
	report := &ChangeReport{
		PlanID:             "plan-unavailable",
		Passed:             true,
		VerificationStatus: VerificationStatusUnavailable,
		FailureKind:        FailureKindRunnerMissing,
		FailureReasonCode:  "pytest_missing",
	}

	got := BuildVerificationProofProfile(nil, report)
	if got.Status != VerificationProofUnavailable ||
		got.RunnerEvidence != VerificationProofRunnerUnavailable ||
		!verificationProofHasReason(got, "pytest_missing") {
		t.Fatalf("profile=%+v, want unavailable pytest_missing", got)
	}
}

func TestBuildCumulativeVerificationProofProfileResolvesCoveredProbeContracts(t *testing.T) {
	primaryReport := &ChangeReport{
		PlanID:             "plan-proof",
		Passed:             true,
		VerificationStatus: VerificationStatusPassed,
		TestResults: []TestResult{{
			AssertionID: "probe/proof",
			Suite:       "verification_probe/python",
			Passed:      true,
		}},
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
			ContractRefs: []string{"outcome-3"},
		}, {
			Source:       "verification_probe",
			Category:     "probe_soft_contract_refs",
			Status:       "satisfied",
			ReasonCode:   "verification_probe_soft_contract_ref_covered",
			ContractRefs: []string{"outcome-4"},
		}},
	}
	sourceReport := &ChangeReport{
		PlanID:             "plan-source",
		Passed:             true,
		VerificationStatus: VerificationStatusPassed,
		TestResults: []TestResult{{
			AssertionID: "probe/source",
			Suite:       "verification_probe/python",
			Passed:      true,
		}},
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
			ContractRefs: []string{"outcome-3"},
		}, {
			Source:       "verification_probe",
			Category:     "probe_soft_contract_refs",
			Status:       "missing",
			ReasonCode:   "verification_probe_missing_soft_contract_ref",
			ContractRefs: []string{"outcome-4"},
		}},
	}

	got := BuildCumulativeVerificationProofProfile(nil, primaryReport, []VerificationProofArtifact{{
		Report: sourceReport,
	}})

	if got.Status != VerificationProofAdequate {
		t.Fatalf("profile=%+v, want adequate after cumulative soft-contract coverage", got)
	}
	if !got.Cumulative || got.ContributingReports != 2 {
		t.Fatalf("profile cumulative metadata=%+v, want 2 reports", got)
	}
	if verificationProofHasReason(got, "verification_probe_missing_soft_contract_ref") {
		t.Fatalf("resolved missing soft-contract reason should be removed: %+v", got.ReasonCodes)
	}
	if got.ProbeCommands != 2 || got.TestCount != 2 {
		t.Fatalf("profile counts=%+v, want cumulative probe/test counts", got)
	}
}

func TestBuildCumulativeVerificationProofProfileKeepsUnresolvedProbeContract(t *testing.T) {
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
			Category:     "probe_contract_refs",
			Status:       "missing",
			ReasonCode:   "verification_probe_missing_required_contract_ref",
			ContractRefs: []string{"hard-outcome"},
		}},
	}
	relatedReport := &ChangeReport{
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
			Category:     "probe_contract_refs",
			Status:       "satisfied",
			ReasonCode:   "verification_probe_contract_ref_covered",
			ContractRefs: []string{"other-outcome"},
		}},
	}

	got := BuildCumulativeVerificationProofProfile(nil, primaryReport, []VerificationProofArtifact{{Report: relatedReport}})

	if got.Status != VerificationProofWeak {
		t.Fatalf("profile=%+v, want weak with unresolved hard contract", got)
	}
	if !verificationProofHasReason(got, "verification_probe_missing_required_contract_ref") {
		t.Fatalf("profile reasons %+v missing unresolved required-contract reason", got.ReasonCodes)
	}
}

func TestBuildCumulativeVerificationProofProfileResolvesRenderedTextPlacement(t *testing.T) {
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
			Category:     "probe_placement_refs",
			Status:       "missing",
			ReasonCode:   "verification_probe_missing_required_placement_ref",
			ContractRefs: []string{"repr-unit-placement"},
		}},
	}
	goStringerReport := &ChangeReport{
		PlanID:             "plan-source",
		Passed:             true,
		VerificationStatus: VerificationStatusPassed,
		ExecutedCommands: []ExecutedCommand{{
			Runner:  "verification_probe",
			Suite:   "verification_probe/go",
			Outcome: "executed",
			Source:  "go_verification_probe",
		}},
		VerificationConfidence: []VerificationConfidenceRecord{{
			Source:       "verification_probe",
			Category:     "probe_placement_refs",
			Status:       "satisfied",
			ReasonCode:   "verification_probe_placement_ref_covered",
			ContractRefs: []string{"repr-unit-placement"},
		}},
	}

	got := BuildCumulativeVerificationProofProfile(nil, primaryReport, []VerificationProofArtifact{{Report: goStringerReport}})

	if got.Status != VerificationProofAdequate {
		t.Fatalf("profile=%+v, want adequate after cumulative placement coverage", got)
	}
	if verificationProofHasReason(got, "verification_probe_missing_required_placement_ref") {
		t.Fatalf("resolved placement gap should be removed: %+v", got.ReasonCodes)
	}
}

func TestBuildCumulativeVerificationProofProfilePreservesUnavailablePrimary(t *testing.T) {
	primaryReport := &ChangeReport{
		PlanID:             "plan-proof",
		VerificationStatus: VerificationStatusUnavailable,
		FailureKind:        FailureKindParserError,
		FailureReasonCode:  "parser_error",
	}
	relatedReport := &ChangeReport{
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
			Category:     "probe_contract_refs",
			Status:       "satisfied",
			ReasonCode:   "verification_probe_contract_ref_covered",
			ContractRefs: []string{"hard-outcome"},
		}},
	}

	got := BuildCumulativeVerificationProofProfile(nil, primaryReport, []VerificationProofArtifact{{Report: relatedReport}})

	if got.Status != VerificationProofUnavailable || !verificationProofHasReason(got, "parser_error") {
		t.Fatalf("profile=%+v, want primary unavailable authority preserved", got)
	}
}

func TestBuildVerificationProofLedgerProjectsCoverageObligations(t *testing.T) {
	plan := &ChangePlan{
		ID: "plan-ledger",
		PatchReview: &PatchReviewRecord{Findings: []PatchReviewFinding{{
			Code:           "changed_symbol_without_probe_coverage",
			Category:       PatchReviewCategorySemanticCoverage,
			ImpactKind:     PatchReviewImpactKindChangedSymbol,
			CoverageStatus: PatchReviewCoverageUnverified,
			Path:           "pkg/widget.py",
			SubjectSymbol:  "Widget.render",
		}}},
		ImpactAnalysis: &ImpactAnalysisResult{VerificationTargets: []ImpactVerificationTarget{{
			ID:             "target-contract",
			Kind:           "behavior_contract",
			Path:           "pkg/widget.py",
			ContractRef:    "contract-render",
			CoverageStatus: "unavailable",
		}}},
	}
	report := &ChangeReport{
		PlanID:             "plan-ledger",
		VerificationStatus: VerificationStatusPassed,
		Passed:             true,
		ExecutedCommands: []ExecutedCommand{{
			Runner:  "verification_probe",
			Suite:   "verification_probe/python",
			Outcome: "executed",
			Source:  "python_verification_probe",
		}},
		VerificationConfidence: []VerificationConfidenceRecord{{
			Source:            "verification_probe",
			Category:          "probe_changed_symbol",
			Status:            "missing",
			ReasonCode:        "verification_probe_missing_changed_symbol_ref",
			ChangedSymbolRefs: []string{"Widget.render"},
		}, {
			Source:       "verification_probe",
			Category:     "probe_contract_refs",
			Status:       "satisfied",
			ReasonCode:   "verification_probe_contract_ref_covered",
			ContractRefs: []string{"contract-render"},
		}},
	}

	got := BuildVerificationProofLedger(plan, report, nil)

	if got.State != VerificationProofLedgerLowConfidence {
		t.Fatalf("ledger state=%q, want low_confidence: %+v", got.State, got)
	}
	if got.ObligationCount == 0 || got.UncoveredCount == 0 || got.CoveredCount == 0 || got.UnavailableCount == 0 {
		t.Fatalf("ledger counts not projected: %+v", got)
	}
	if !verificationProofLedgerHasItem(got, "changed_symbol", VerificationProofLedgerItemMissing, "verification_probe_missing_changed_symbol_ref") {
		t.Fatalf("ledger obligations=%+v missing changed-symbol gap", got.Obligations)
	}
	if !verificationProofLedgerHasItem(got, "behavior_contract", VerificationProofLedgerItemCovered, "verification_probe_contract_ref_covered") {
		t.Fatalf("ledger obligations=%+v missing covered contract", got.Obligations)
	}
	if !verificationProofLedgerHasItem(got, "behavior_contract", VerificationProofLedgerItemUnavailable, "unavailable") {
		t.Fatalf("ledger obligations=%+v missing unavailable impact target", got.Obligations)
	}
}

func TestBuildVerificationProofLedgerResolvesRetainedSamePathWithTerminalCumulativeCoverage(t *testing.T) {
	oldPlan := &ChangePlan{
		ID: "plan-old",
		ImpactAnalysis: &ImpactAnalysisResult{VerificationTargets: []ImpactVerificationTarget{{
			ID: "old-path", Kind: "changed_file", Path: "repository.c", CoverageStatus: "unverified",
		}}},
	}
	newPlan := &ChangePlan{
		ID: "plan-new",
		CumulativeVerificationScope: &CumulativeVerificationScope{
			SourcePlanIDs: []string{"plan-old"},
			TargetPaths:   []string{"repository.c"},
		},
	}
	oldReport := &ChangeReport{
		PlanID: "plan-old", VerificationStatus: VerificationStatusFailed,
		ExecutedCommands: []ExecutedCommand{{Runner: "make", Suite: "check", Command: "make check", Outcome: "failed", ExitCode: 1}},
		ChangedPathCoverage: []ChangedPathVerificationCoverage{{
			Path: "repository.c", Status: ChangedPathVerificationUncovered,
		}},
	}
	newReport := &ChangeReport{
		PlanID: "plan-new", Passed: true, VerificationStatus: VerificationStatusPassed,
		ExecutedCommands: []ExecutedCommand{{Runner: "make", Suite: "check", Command: "make check", Outcome: "executed"}},
		ChangedPathCoverage: []ChangedPathVerificationCoverage{{
			Path: "repository.c", Status: ChangedPathVerificationCovered,
			Caliber: ChangedPathVerificationProjectRunner, Capability: VerificationCapabilityTargetBehavior,
		}},
	}

	got := BuildVerificationProofLedger(newPlan, newReport, []VerificationProofArtifact{
		{Plan: oldPlan, Report: oldReport},
		{Plan: newPlan, Report: newReport},
	})
	if got.State != VerificationProofLedgerVerified || got.UncoveredCount != 0 {
		t.Fatalf("terminal cumulative same-path proof did not close retained obligations: %+v", got)
	}
	if !verificationProofLedgerHasItem(got, "changed_file", VerificationProofLedgerItemCovered, "resolved_by_terminal_cumulative_changed_path") {
		t.Fatalf("retained changed-file obligation was not audibly resolved: %+v", got.Obligations)
	}
}

func TestBuildVerificationProofLedgerDoesNotResolveUnboundHistoricalSamePath(t *testing.T) {
	oldPlan := &ChangePlan{
		ID: "plan-old",
		ImpactAnalysis: &ImpactAnalysisResult{VerificationTargets: []ImpactVerificationTarget{{
			ID: "old-path", Kind: "changed_file", Path: "repository.c", CoverageStatus: "unverified",
		}}},
	}
	newPlan := &ChangePlan{ID: "plan-new"}
	newReport := &ChangeReport{
		PlanID: "plan-new", Passed: true, VerificationStatus: VerificationStatusPassed,
		ChangedPathCoverage: []ChangedPathVerificationCoverage{{
			Path: "repository.c", Status: ChangedPathVerificationCovered,
		}},
	}

	got := BuildVerificationProofLedger(newPlan, newReport, []VerificationProofArtifact{
		{Plan: oldPlan, Report: &ChangeReport{PlanID: "plan-old", VerificationStatus: VerificationStatusFailed}},
		{Plan: newPlan, Report: newReport},
	})
	if got.State == VerificationProofLedgerVerified || got.UncoveredCount == 0 {
		t.Fatalf("unbound same-path history was incorrectly erased: %+v", got)
	}
}

func TestBuildVerificationProofLedgerProjectsRenderedTextPlacementObligations(t *testing.T) {
	report := &ChangeReport{
		PlanID:             "plan-placement-ledger",
		VerificationStatus: VerificationStatusPassed,
		Passed:             true,
		ExecutedCommands: []ExecutedCommand{{
			Runner:  "verification_probe",
			Suite:   "verification_probe/javascript",
			Outcome: "executed",
			Source:  "javascript_verification_probe",
		}},
		VerificationConfidence: []VerificationConfidenceRecord{{
			Source:       "verification_probe",
			Category:     "probe_placement_refs",
			Status:       "missing",
			ReasonCode:   "verification_probe_missing_required_placement_ref",
			ContractRefs: []string{"cli-label-placement"},
		}, {
			Source:       "verification_probe",
			Category:     "probe_placement_refs",
			Status:       "satisfied",
			ReasonCode:   "verification_probe_placement_ref_covered",
			ContractRefs: []string{"stringer-label-placement"},
		}},
	}

	got := BuildVerificationProofLedger(nil, report, nil)

	if got.State != VerificationProofLedgerLowConfidence {
		t.Fatalf("ledger state=%q, want low_confidence: %+v", got.State, got)
	}
	if !verificationProofLedgerHasItem(got, "rendered_text_placement_contract", VerificationProofLedgerItemMissing, "verification_probe_missing_required_placement_ref") {
		t.Fatalf("ledger obligations=%+v missing placement gap", got.Obligations)
	}
	if !verificationProofLedgerHasItem(got, "rendered_text_placement_contract", VerificationProofLedgerItemCovered, "verification_probe_placement_ref_covered") {
		t.Fatalf("ledger obligations=%+v missing covered placement", got.Obligations)
	}
}

func TestBuildVerificationProofLedgerTreatsProbeParserErrorCommandAsUnavailable(t *testing.T) {
	report := &ChangeReport{
		PlanID:             "plan-probe-unavailable",
		VerificationStatus: VerificationStatusUnavailable,
		Passed:             false,
		FailureKind:        FailureKindParserError,
		FailureReasonCode:  "verification_probe_module_not_found",
		ExecutedCommands: []ExecutedCommand{{
			Runner:     "verification_probe",
			Framework:  "python",
			Command:    "python -c <verification_probe:no-crash>",
			Outcome:    "parser_error",
			ReasonCode: "verification_probe_module_not_found",
			ExitCode:   1,
			Source:     "pre_suite_verification_probe",
		}, {
			Runner:    "verification_probe",
			Framework: "python",
			Command:   "python -c <verification_probe:bad-fixture>",
			Outcome:   "parser_error",
			ExitCode:  1,
			Source:    "parser_error_verification_probe",
		}},
	}

	got := BuildVerificationProofLedger(nil, report, nil)

	if got.State != VerificationProofLedgerUnavailable {
		t.Fatalf("ledger state=%q, want unavailable: %+v", got.State, got)
	}
	if got.CapabilityFailedCount != 0 {
		t.Fatalf("capability failed count=%d, want 0; capabilities=%+v", got.CapabilityFailedCount, got.Capabilities)
	}
	if got.CapabilityUnavailableCount < 3 {
		t.Fatalf("capability unavailable count=%d, want report + command rows; capabilities=%+v", got.CapabilityUnavailableCount, got.Capabilities)
	}
	if !verificationProofLedgerHasCapability(got, "executed_command", VerificationProofLedgerItemUnavailable, "verification_probe_module_not_found") {
		t.Fatalf("ledger capabilities=%+v missing typed unavailable probe command", got.Capabilities)
	}
	if !verificationProofLedgerHasCapability(got, "executed_command", VerificationProofLedgerItemUnavailable, "parser_error") {
		t.Fatalf("ledger capabilities=%+v missing generic parser_error unavailable command", got.Capabilities)
	}
}

func TestBuildVerificationProofLedgerResolvesExactRunnerMissingEscalation(t *testing.T) {
	newReport := func(escalationSuite string) *ChangeReport {
		return &ChangeReport{
			PlanID:             "plan-python-fallback",
			VerificationStatus: VerificationStatusPassed,
			Passed:             true,
			ExecutedCommands: []ExecutedCommand{{
				Runner: "python", Framework: "pytest", WorkingDir: ".", Suite: "tests/test_tokenizer.py",
				Command: "python3 -m pytest tests/test_tokenizer.py", ExitCode: 1,
				Source: "impact_test_surface", Outcome: string(FailureKindRunnerMissing),
			}, {
				Runner: "python", Framework: "unittest", WorkingDir: ".", Suite: escalationSuite,
				Command: "python3 -m unittest tests/test_tokenizer.py -v", ExitCode: 0,
				Source: "runner_missing_escalation", Outcome: "executed",
			}},
		}
	}

	got := BuildVerificationProofLedger(nil, newReport("tests/test_tokenizer.py"), nil)
	if got.State != VerificationProofLedgerVerified || got.CapabilityUnavailableCount != 0 {
		t.Fatalf("exact typed runner escalation did not resolve unavailable candidate: %+v", got)
	}
	if !verificationProofLedgerHasCapability(got, "executed_command", VerificationProofLedgerItemAdvisory, "superseded_by_exact_runner_missing_escalation") {
		t.Fatalf("resolved runner candidate lost advisory receipt: %+v", got.Capabilities)
	}

	differentTarget := BuildVerificationProofLedger(nil, newReport("tests/test_other.py"), nil)
	if differentTarget.CapabilityUnavailableCount != 1 ||
		!verificationProofLedgerHasCapability(differentTarget, "executed_command", VerificationProofLedgerItemUnavailable, string(FailureKindRunnerMissing)) {
		t.Fatalf("different-suite pass laundered unavailable runner candidate: %+v", differentTarget)
	}
}

func verificationProofHasReason(profile VerificationProofProfile, code string) bool {
	for _, reason := range profile.ReasonCodes {
		if reason == code {
			return true
		}
	}
	return false
}

func verificationProofLedgerHasCapability(ledger VerificationProofLedger, kind string, status VerificationProofLedgerItemStatus, reason string) bool {
	for _, item := range ledger.Capabilities {
		if item.Kind == kind && item.Status == status && item.ReasonCode == reason {
			return true
		}
	}
	return false
}

func verificationProofLedgerHasItem(ledger VerificationProofLedger, kind string, status VerificationProofLedgerItemStatus, reason string) bool {
	for _, item := range ledger.Obligations {
		if item.Kind == kind && item.Status == status && item.ReasonCode == reason {
			return true
		}
	}
	return false
}
