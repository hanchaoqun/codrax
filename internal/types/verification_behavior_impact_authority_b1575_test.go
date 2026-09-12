package types

import (
	"bytes"
	"encoding/json"
	"testing"
)

func b1575ImpactContractPlan() *ChangePlan {
	return &ChangePlan{
		ID: "current", BehaviorContracts: []WriteBehaviorContract{{
			ID: "value", Kind: WriteBehaviorObservable, Required: true,
			Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpEquals, Expected: "42", Source: "write_analyzer",
		}},
		ImpactAnalysis: &ImpactAnalysisResult{PlanID: "current", VerificationTargets: []ImpactVerificationTarget{{
			Kind: "behavior_contract", ContractRef: "value", EvidenceRef: "value", CoverageStatus: "verified", Source: "verification_probe",
		}}},
		PatchReview: &PatchReviewRecord{PlanID: "current", Findings: []PatchReviewFinding{{
			Code: "behavior_contract_without_verify_coverage", Category: PatchReviewCategorySemanticCoverage,
			ImpactKind: PatchReviewImpactKindBehaviorContract, EvidenceRef: "value", Severity: PatchReviewSeverityWarning,
			CoverageStatus: PatchReviewCoverageVerified,
		}}},
	}
}

func TestB1575PersistedBehaviorImpactCannotMintContractProof(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ChangePlan, *ChangeReport)
		want VerificationProofLedgerItemStatus
	}{
		{name: "legacy_verified_without_witness", want: VerificationProofLedgerItemUnverified},
		{name: "native_exact_ref", edit: func(_ *ChangePlan, r *ChangeReport) {
			r.VerificationConfidence = []VerificationConfidenceRecord{{Category: "project_test_contract_refs", WitnessKind: WriteBehaviorWitnessProjectTest, Status: "satisfied", ContractRefs: []string{"value"}}}
		}, want: VerificationProofLedgerItemCovered},
		{name: "native_exact_legacy_evidence_ref", edit: func(p *ChangePlan, r *ChangeReport) {
			p.ImpactAnalysis.VerificationTargets[0].ContractRef = ""
			r.VerificationConfidence = []VerificationConfidenceRecord{{Category: "project_test_contract_refs", WitnessKind: WriteBehaviorWitnessProjectTest, Status: "satisfied", ContractRefs: []string{"value"}}}
		}, want: VerificationProofLedgerItemCovered},
		{name: "native_other_ref", edit: func(_ *ChangePlan, r *ChangeReport) {
			r.VerificationConfidence = []VerificationConfidenceRecord{{Category: "project_test_contract_refs", WitnessKind: WriteBehaviorWitnessProjectTest, Status: "satisfied", ContractRefs: []string{"other"}}}
		}, want: VerificationProofLedgerItemUnverified},
		{name: "source_text_runtime_not_witness", edit: func(_ *ChangePlan, r *ChangeReport) {
			r.VerificationConfidence = []VerificationConfidenceRecord{{Category: "source_contract_refs", WitnessKind: WriteBehaviorWitnessSourceText, Status: "satisfied", ContractRefs: []string{"value"}}}
		}, want: VerificationProofLedgerItemUnverified},
		{name: "source_text_layout_exact", edit: func(p *ChangePlan, r *ChangeReport) {
			p.BehaviorContracts[0].Kind = WriteBehaviorFileLayout
			r.VerificationConfidence = []VerificationConfidenceRecord{{Category: "source_contract_refs", WitnessKind: WriteBehaviorWitnessSourceText, Status: "satisfied", ContractRefs: []string{"value"}}}
		}, want: VerificationProofLedgerItemCovered},
		{name: "unknown_contract_kind", edit: func(p *ChangePlan, r *ChangeReport) {
			p.BehaviorContracts = nil
			r.VerificationConfidence = []VerificationConfidenceRecord{{Category: "project_test_contract_refs", WitnessKind: WriteBehaviorWitnessProjectTest, Status: "satisfied", ContractRefs: []string{"value"}}}
		}, want: VerificationProofLedgerItemUnverified},
		{name: "other_report_generation", edit: func(_ *ChangePlan, r *ChangeReport) {
			r.PlanID = "different"
			r.VerificationConfidence = []VerificationConfidenceRecord{{Category: "project_test_contract_refs", WitnessKind: WriteBehaviorWitnessProjectTest, Status: "satisfied", ContractRefs: []string{"value"}}}
		}, want: VerificationProofLedgerItemUnverified},
		{name: "persisted_failed_plan_passed_report_conflict", edit: func(p *ChangePlan, r *ChangeReport) {
			p.Status = PlanStatusVerifyFailed
			r.VerificationConfidence = []VerificationConfidenceRecord{{Category: "project_test_contract_refs", Status: "satisfied", ContractRefs: []string{"value"}}}
		}, want: VerificationProofLedgerItemUnverified},
		{name: "planning_only_historical_verified", edit: func(p *ChangePlan, _ *ChangeReport) {
			p.BehaviorContracts[0].Source += ";" + WriteBehaviorContractSourcePlanningOnlyUngrounded
		}, want: VerificationProofLedgerItemAdvisory},
		{name: "planning_only_soft_probe_cannot_upgrade", edit: func(p *ChangePlan, r *ChangeReport) {
			p.BehaviorContracts[0].Source += ";" + WriteBehaviorContractSourcePlanningOnlyUngrounded
			r.VerificationConfidence = []VerificationConfidenceRecord{{Category: "probe_soft_contract_refs", Status: "satisfied", ContractRefs: []string{"value"}}}
		}, want: VerificationProofLedgerItemAdvisory},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := b1575ImpactContractPlan()
			report := &ChangeReport{PlanID: plan.ID, Passed: true, VerificationStatus: VerificationStatusPassed,
				TestResults:      []TestResult{{AssertionID: "native-test", Passed: true}},
				ExecutedCommands: []ExecutedCommand{{Runner: "go", Outcome: ExecutedCommandOutcomeExecuted, ExitCode: 0}},
			}
			if tc.edit != nil {
				tc.edit(plan, report)
			}
			before, err := json.Marshal([]any{plan, report})
			if err != nil {
				t.Fatal(err)
			}
			// JSON reload is the same public proof path used by historical artifacts.
			var loaded ChangePlan
			planJSON, _ := json.Marshal(plan)
			if err := json.Unmarshal(planJSON, &loaded); err != nil {
				t.Fatal(err)
			}
			loadedBefore, _ := json.Marshal(&loaded)
			ledger := BuildVerificationProofLedger(&loaded, report, nil)
			seen := 0
			for _, item := range ledger.Obligations {
				if item.Kind != "behavior_contract" || (item.Source != "verification_probe" && item.Source != "patch_review") {
					continue
				}
				seen++
				if item.Status != tc.want {
					t.Errorf("persisted derived row gained/lost authority: %+v; want %s", item, tc.want)
				}
			}
			if seen != 2 {
				t.Fatalf("expected both persisted carriers, got %d: %+v", seen, ledger.Obligations)
			}
			profile := BuildVerificationProofProfile(&loaded, report)
			if tc.want == VerificationProofLedgerItemAdvisory && (profile.ImpactVerifiedCount != 0 || profile.ImpactUnverifiedCount != 0 || profile.PatchReviewVerdict != PatchReviewCoverageVerdictAdvisory) {
				t.Errorf("planning-only became measured coverage or mandatory debt: %+v", profile)
			}
			if tc.want == VerificationProofLedgerItemUnverified && profile.ImpactVerifiedCount != 0 {
				t.Errorf("profile revived old verified label: %+v", profile)
			}
			if tc.want == VerificationProofLedgerItemAdvisory {
				for _, item := range ledger.Obligations {
					if item.Kind == "behavior_contract" && item.Status == VerificationProofLedgerItemCovered {
						t.Errorf("another persisted confidence lane upgraded planning-only: %+v", item)
					}
				}
			}
			loadedAfter, _ := json.Marshal(&loaded)
			if !bytes.Equal(loadedBefore, loadedAfter) {
				t.Fatal("proof mutated reloaded input")
			}
			after, err := json.Marshal([]any{plan, report})
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("projection changed raw plan/report")
			}
		})
	}
}

func TestB1575BehaviorImpactProjectionPreservesFailureAndNonBehaviorAxes(t *testing.T) {
	for _, status := range []string{"failed", "error"} {
		plan := b1575ImpactContractPlan()
		plan.BehaviorContracts[0].Source += ";" + WriteBehaviorContractSourcePlanningOnlyUngrounded
		plan.ImpactAnalysis.VerificationTargets[0].CoverageStatus = status
		plan.ImpactAnalysis.VerificationTargets = append(plan.ImpactAnalysis.VerificationTargets, ImpactVerificationTarget{
			Kind: "changed_file", Path: "client.py", CoverageStatus: "verified", EvidenceRef: "physical-source",
		})
		plan.PatchReview.Status, plan.PatchReview.HardBlock = "failed", true
		plan.PatchReview.Findings = append(plan.PatchReview.Findings, PatchReviewFinding{
			Category: PatchReviewCategoryStructural, Severity: PatchReviewSeverityError,
			Code: "structured_file_parse_error", CoverageStatus: PatchReviewCoverageUnverified,
		})
		report := &ChangeReport{PlanID: plan.ID, Passed: false, VerificationStatus: VerificationStatusFailed,
			VerificationConfidence: []VerificationConfidenceRecord{{Category: "project_test_contract_refs", Status: "satisfied", ContractRefs: []string{"value"}}},
		}
		before, _ := json.Marshal([]any{plan, report})
		projected := EffectiveBehaviorContractVerificationPlan(plan, report)
		if projected.ImpactAnalysis.VerificationTargets[0].CoverageStatus != status || !projected.PatchReview.HardBlock || projected.PatchReview.Status != "failed" {
			t.Fatalf("independent failure was demoted: %+v %+v", projected.ImpactAnalysis, projected.PatchReview)
		}
		originalOther, _ := json.Marshal([]any{plan.ImpactAnalysis.VerificationTargets[1], plan.PatchReview.Findings[1]})
		projectedOther, _ := json.Marshal([]any{projected.ImpactAnalysis.VerificationTargets[1], projected.PatchReview.Findings[1]})
		if !bytes.Equal(originalOther, projectedOther) {
			t.Fatal("unrelated kind or hard finding changed")
		}
		if BehaviorContractRefHasVerificationWitness(plan, report, "value") {
			t.Fatal("planning-only native mention was granted authority")
		}
		after, _ := json.Marshal([]any{plan, report})
		if !bytes.Equal(before, after) {
			t.Fatal("source plan/report changed")
		}
	}
}

func TestB1575PlanningOnlyProbeAndSliceRefsDoNotMintImpactObligations(t *testing.T) {
	plan := b1575ImpactContractPlan()
	plan.BehaviorContracts[0].Source += ";" + WriteBehaviorContractSourcePlanningOnlyUngrounded
	plan.VerificationProbes = []VerificationProbe{{ID: "plain", ContractRefs: []string{"value", "unknown"}, ChangedSymbolRefs: []string{"path:client.py"}}}
	plan.Slices = []ChangePlanSlice{{ID: "slice", ContractRefs: []string{"value", "unknown"}}}
	before, _ := json.Marshal(plan)
	set := ImpactObligationSetFromChangePlan(plan)
	unknown, path := 0, false
	for _, ob := range set.Obligations {
		if ob.ContractRef == "value" {
			t.Errorf("planning-only declaration acquired verification obligation: %+v", ob)
		}
		if ob.ContractRef == "unknown" {
			unknown++
		}
		path = path || ob.Kind == "changed_file"
	}
	if unknown != 2 || !path {
		t.Fatalf("unrelated declaration/path inventory changed: %+v", set)
	}
	after, _ := json.Marshal(plan)
	if !bytes.Equal(before, after) {
		t.Fatal("obligation projection rewrote model declarations")
	}
}

func TestB1575HistoricalImpactWithoutReportDoesNotResolveCurrentContract(t *testing.T) {
	current := b1575ImpactContractPlan()
	current.ImpactAnalysis, current.PatchReview = nil, nil
	history := b1575ImpactContractPlan()
	history.ID = "historical"
	report := &ChangeReport{PlanID: current.ID, Passed: true, VerificationStatus: VerificationStatusPassed}
	ledger := BuildVerificationProofLedger(current, report, []VerificationProofArtifact{{Plan: history}})
	for _, item := range ledger.Obligations {
		if item.Kind == "behavior_contract" && item.Status == VerificationProofLedgerItemCovered {
			t.Errorf("reportless stored status resolved a live obligation: %+v", item)
		}
	}
}

func TestB1575CumulativeExactWitnessOnlySupersedesSameContractDerivedDebt(t *testing.T) {
	for _, tc := range []struct {
		name   string
		edit   func(*ChangePlan, *ChangeReport)
		closed bool
	}{
		{name: "exact_native", closed: true},
		{name: "wrong_ref", edit: func(_ *ChangePlan, r *ChangeReport) { r.VerificationConfidence[0].ContractRefs = []string{"different"} }},
		{name: "source_text_for_runtime", edit: func(_ *ChangePlan, r *ChangeReport) {
			r.VerificationConfidence[0].Category = "source_contract_refs"
			r.VerificationConfidence[0].WitnessKind = WriteBehaviorWitnessSourceText
		}},
		{name: "changed_expected", edit: func(p *ChangePlan, _ *ChangeReport) { p.BehaviorContracts[0].Expected = "43" }},
		{name: "changed_source", edit: func(p *ChangePlan, _ *ChangeReport) { p.BehaviorContracts[0].Source = "another_typed_origin" }},
		{name: "wrong_report_plan", edit: func(_ *ChangePlan, r *ChangeReport) { r.PlanID = "unrelated" }},
		{name: "conflicting_donor_failed_plan_passed_report", edit: func(p *ChangePlan, _ *ChangeReport) { p.Status = PlanStatusVerifyFailed }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			history := b1575ImpactContractPlan()
			history.ID = "history"
			history.ImpactAnalysis.PlanID, history.PatchReview.PlanID = history.ID, history.ID
			current := b1575ImpactContractPlan()
			current.ImpactAnalysis, current.PatchReview = nil, nil
			report := &ChangeReport{PlanID: current.ID, Passed: true, VerificationStatus: VerificationStatusPassed,
				TestResults:            []TestResult{{AssertionID: "native", Passed: true}},
				ExecutedCommands:       []ExecutedCommand{{Runner: "go", Outcome: ExecutedCommandOutcomeExecuted}},
				VerificationConfidence: []VerificationConfidenceRecord{{Category: "project_test_contract_refs", WitnessKind: WriteBehaviorWitnessProjectTest, Status: "satisfied", ContractRefs: []string{"value"}}},
			}
			if tc.edit != nil {
				tc.edit(current, report)
			}
			before, _ := json.Marshal([]any{history, current, report})
			artifacts := []VerificationProofArtifact{{Plan: history}}
			ledger := BuildVerificationProofLedger(current, report, artifacts)
			profile := BuildCumulativeVerificationProofProfile(current, report, artifacts)
			if (ledger.State == VerificationProofLedgerVerified) != tc.closed || (profile.Status == VerificationProofStrong) != tc.closed {
				t.Errorf("cumulative closure=%v expected=%v; ledger=%+v profile=%+v", ledger.State, tc.closed, ledger, profile)
			}
			seen := 0
			for _, item := range ledger.Obligations {
				if item.PlanID != history.ID || item.Kind != "behavior_contract" {
					continue
				}
				if item.Source == "verification_probe" || item.Source == "patch_review" {
					seen++
					want := VerificationProofLedgerItemUnverified
					if tc.closed {
						want = VerificationProofLedgerItemAdvisory
					}
					if item.Status != want {
						t.Errorf("historical derived claim became its own receipt: %+v want=%s", item, want)
					}
				} else if !tc.closed && item.Source == "change_plan_behavior_contract" && item.Status == VerificationProofLedgerItemCovered {
					t.Errorf("same ref with unproven semantic identity closed old required contract: %+v", item)
				}
			}
			if seen != 2 {
				t.Fatalf("historical derived rows lost: %+v", ledger.Obligations)
			}
			after, _ := json.Marshal([]any{history, current, report})
			if !bytes.Equal(before, after) {
				t.Fatal("cumulative proof mutated original artifacts")
			}
		})
	}
}
