package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func b1678DiagnosticReport(planID string, records ...VerificationConfidenceRecord) *ChangeReport {
	return &ChangeReport{
		PlanID: planID, Passed: true, VerificationStatus: VerificationStatusPassed,
		TestResults: []TestResult{{
			AssertionID: "test_apply", Suite: "test_dates.DateTest",
			ObservationScope: TestObservationScopeAssertion, Passed: true,
		}},
		ExecutedCommands: []ExecutedCommand{{
			Runner: "python", Framework: "unittest", Suite: "test_dates.py",
			Outcome: ExecutedCommandOutcomeExecuted, ExitCode: 0,
		}},
		ChangedPathCoverage: []ChangedPathVerificationCoverage{{
			Path: "dates.py", Status: ChangedPathVerificationCovered,
			Caliber: ChangedPathVerificationProjectRunner, Capability: VerificationCapabilityTargetBehavior,
		}},
		VerificationConfidence: records,
	}
}

func b1678DiagnosticItem(t *testing.T, ledger VerificationProofLedger, source, ref string) VerificationProofLedgerItem {
	t.Helper()
	var found []VerificationProofLedgerItem
	for _, item := range ledger.Obligations {
		if item.Source == source && (item.ContractRef == ref || item.Symbol == ref) {
			found = append(found, item)
		}
	}
	if len(found) != 1 {
		t.Fatalf("source=%q ref=%q: got %d rows in %+v", source, ref, len(found), ledger.Obligations)
	}
	return found[0]
}

func b1678DiagnosticJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestB1678ProofLedgerResolvedContractDiagnostic(t *testing.T) {
	for _, tc := range []struct {
		name        string
		observedRef string
		second      bool
		wantCovered bool
	}{
		{name: "missing_stays_missing"},
		{name: "exact_native_observation", observedRef: "date-result", wantCovered: true},
		{name: "other_contract_does_not_cover", observedRef: "other-result"},
		{name: "exact_does_not_cover_second_required", observedRef: "date-result", second: true, wantCovered: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := &ChangePlan{ID: "plan-diagnostic", BehaviorContracts: []WriteBehaviorContract{{
				ID: "date-result", Kind: WriteBehaviorInvariant, Polarity: WriteBehaviorPolarityExpected,
				Operator: WriteBehaviorOpSatisfies, Expected: "date operation remains type safe",
				EvidenceRef: "dates.py:12", Required: true,
			}}}
			if tc.second {
				second := plan.BehaviorContracts[0]
				second.ID = "separate-result"
				plan.BehaviorContracts = append(plan.BehaviorContracts, second)
			}
			report := b1678DiagnosticReport(plan.ID)
			if tc.observedRef != "" {
				report.VerificationConfidence = []VerificationConfidenceRecord{{
					Source: "project_test_observation", Category: "project_test_contract_refs",
					Status: "satisfied", ReasonCode: "project_test_contract_ref_observed",
					ContractRefs: []string{tc.observedRef}, WitnessKind: WriteBehaviorWitnessProjectTest,
					Detail: "original native observation detail stays unchanged",
				}}
			}
			planBefore, reportBefore := b1678DiagnosticJSON(t, plan), b1678DiagnosticJSON(t, report)
			var first VerificationProofLedger
			for iteration := 0; iteration < 3; iteration++ {
				ledger := BuildVerificationProofLedger(plan, report, nil)
				profile := BuildVerificationProofProfile(plan, report)
				item := b1678DiagnosticItem(t, ledger, "change_plan_behavior_contract", "date-result")
				if tc.wantCovered {
					if item.Status != VerificationProofLedgerItemCovered || item.ReasonCode != "resolved_by_cumulative_proof" ||
						item.Detail != "behavior contract is covered by cumulative proof carrying the exact contract_ref" {
						t.Errorf("resolved placeholder retained contradictory diagnostic: %+v", item)
					}
					observed := b1678DiagnosticItem(t, ledger, "project_test_observation", "date-result")
					if observed.Status != VerificationProofLedgerItemCovered || observed.ReasonCode != "project_test_contract_ref_observed" ||
						observed.Detail != report.VerificationConfidence[0].Detail {
						t.Errorf("native observation was rewritten: %+v", observed)
					}
				} else if item.Status != VerificationProofLedgerItemMissing || item.ReasonCode != "behavior_contract_observation_missing" ||
					item.Detail != "required behavior contract has no typed observation carrying this exact contract_ref" {
					t.Errorf("unresolved placeholder changed: %+v", item)
				}
				if tc.wantCovered && !tc.second {
					if ledger.State != VerificationProofLedgerVerified || ledger.UncoveredCount != 0 || profile.Status != VerificationProofStrong {
						t.Errorf("exact native proof authority changed: ledger=%+v profile=%+v", ledger, profile)
					}
				} else if ledger.State != VerificationProofLedgerLowConfidence || ledger.UncoveredCount != 1 || profile.Status != VerificationProofWeak {
					t.Errorf("missing obligation was authorized: ledger=%+v profile=%+v", ledger, profile)
				}
				if tc.second {
					other := b1678DiagnosticItem(t, ledger, "change_plan_behavior_contract", "separate-result")
					if other.Status != VerificationProofLedgerItemMissing || other.ReasonCode != "behavior_contract_observation_missing" {
						t.Errorf("exact witness covered unrelated contract: %+v", other)
					}
				}
				if iteration == 0 {
					first = ledger
				} else if !reflect.DeepEqual(first, ledger) {
					t.Errorf("repeated public build changed ledger: first=%+v next=%+v", first, ledger)
				}
				var restored VerificationProofLedger
				if err := json.Unmarshal(b1678DiagnosticJSON(t, ledger), &restored); err != nil {
					t.Fatal(err)
				}
				if got := NormalizeVerificationProofLedger(restored); !reflect.DeepEqual(ledger, got) {
					t.Errorf("JSON/normalization changed resolved ledger: before=%+v after=%+v", ledger, got)
				}
				final := BuildWriteFinalReport(WriteFinalReportInput{Plan: plan, Report: report})
				if finalItem := b1678DiagnosticItem(t, final.ProofLedger, "change_plan_behavior_contract", "date-result"); !reflect.DeepEqual(item, finalItem) {
					t.Errorf("final report diagnostic diverged: ledger=%+v final=%+v", item, finalItem)
				}
			}
			if string(planBefore) != string(b1678DiagnosticJSON(t, plan)) || string(reportBefore) != string(b1678DiagnosticJSON(t, report)) {
				t.Fatal("public proof projections mutated original plan/report")
			}
		})
	}
}

func TestB1678ProofLedgerResolvedSymbolDiagnostic(t *testing.T) {
	for _, tc := range []struct {
		name       string
		observed   string
		wantStatus VerificationProofLedgerItemStatus
	}{
		{name: "missing_stays_missing", wantStatus: VerificationProofLedgerItemMissing},
		{name: "exact_native_observation", observed: "Date.Apply", wantStatus: VerificationProofLedgerItemCovered},
		{name: "unrelated_symbol_stays_missing", observed: "Date.Parse", wantStatus: VerificationProofLedgerItemMissing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := b1678DiagnosticReport("plan-symbol", VerificationConfidenceRecord{
				Source: "verification_probe", Category: "probe_changed_symbol", Status: "missing",
				ReasonCode: "verification_probe_missing_changed_symbol_ref", ChangedSymbolRefs: []string{"Date.Apply"},
				Detail: "no changed-symbol observation was available",
			}, VerificationConfidenceRecord{
				Source: "preserved_observation", Category: "probe_changed_symbol", Status: "unverified",
				ReasonCode: "unverified_symbol_observation", ChangedSymbolRefs: []string{"Date.Apply"},
				Detail: "original unverified observation must not become positive proof",
			})
			if tc.observed != "" {
				report.VerificationConfidence = append(report.VerificationConfidence, VerificationConfidenceRecord{
					Source: "native_symbol_observation", Category: "probe_changed_symbol", Status: "satisfied",
					ReasonCode: "project_test_symbol_observed", ChangedSymbolRefs: []string{tc.observed},
					WitnessKind: WriteBehaviorWitnessProjectTest, Detail: "original native symbol detail",
				})
			}
			before := b1678DiagnosticJSON(t, report)
			ledger := BuildVerificationProofLedger(nil, report, nil)
			item := b1678DiagnosticItem(t, ledger, "verification_probe", "Date.Apply")
			if item.Status != tc.wantStatus {
				t.Fatalf("exact-symbol coverage changed: %+v", item)
			}
			if tc.wantStatus == VerificationProofLedgerItemCovered {
				if item.ReasonCode != "resolved_by_cumulative_proof" || item.Detail != "changed symbol is covered by cumulative proof carrying the exact symbol ref" {
					t.Errorf("resolved symbol retained missing diagnostic: %+v", item)
				}
			} else if item.ReasonCode != "verification_probe_missing_changed_symbol_ref" || item.Detail != report.VerificationConfidence[0].Detail {
				t.Errorf("unresolved symbol diagnostic changed: %+v", item)
			}
			unverified := b1678DiagnosticItem(t, ledger, "preserved_observation", "Date.Apply")
			if unverified.Status != VerificationProofLedgerItemUnverified || unverified.ReasonCode != "unverified_symbol_observation" ||
				unverified.Detail != report.VerificationConfidence[1].Detail {
				t.Errorf("original unverified observation was rewritten: %+v", unverified)
			}
			if tc.observed != "" {
				observed := b1678DiagnosticItem(t, ledger, "native_symbol_observation", tc.observed)
				if observed.Status != VerificationProofLedgerItemCovered || observed.ReasonCode != "project_test_symbol_observed" || observed.Detail != "original native symbol detail" {
					t.Errorf("original covered observation was rewritten: %+v", observed)
				}
			}
			if again := BuildVerificationProofLedger(nil, report, nil); !reflect.DeepEqual(ledger, again) {
				t.Errorf("repeated symbol compilation changed ledger: %+v", again)
			}
			if string(before) != string(b1678DiagnosticJSON(t, report)) {
				t.Fatal("symbol proof projection mutated original report")
			}
		})
	}
}

func TestB1678ProofLedgerCrossPlanDiagnosticIdentity(t *testing.T) {
	for _, differentContract := range []bool{false, true} {
		name := "same_contract_resolves"
		if differentContract {
			name = "different_contract_keeps_derived_veto"
		}
		t.Run(name, func(t *testing.T) {
			contract := WriteBehaviorContract{
				ID: "date-result", Kind: WriteBehaviorInvariant, Polarity: WriteBehaviorPolarityExpected,
				Operator: WriteBehaviorOpSatisfies, Expected: "date operation remains type safe",
				EvidenceRef: "dates.py:12", Required: true,
			}
			oldPlan := &ChangePlan{ID: "plan-old", BehaviorContracts: []WriteBehaviorContract{contract},
				ImpactAnalysis: &ImpactAnalysisResult{PlanID: "plan-old", VerificationTargets: []ImpactVerificationTarget{{
					ID: "date-impact", Kind: "behavior_contract", ContractRef: contract.ID, CoverageStatus: "verified",
				}}},
			}
			oldReport := b1678DiagnosticReport(oldPlan.ID, VerificationConfidenceRecord{
				Source: "verification_probe", Category: "probe_changed_symbol", Status: "missing",
				ReasonCode: "verification_probe_missing_changed_symbol_ref", ChangedSymbolRefs: []string{"Date.Apply"},
				Detail: "no changed-symbol observation was available",
			})
			newPlan := &ChangePlan{ID: "plan-new", BehaviorContracts: []WriteBehaviorContract{contract}}
			if differentContract {
				newPlan.BehaviorContracts[0].Expected = "a distinct outcome with the same short contract ID"
			}
			newReport := b1678DiagnosticReport(newPlan.ID, VerificationConfidenceRecord{
				Source: "project_test_observation", Category: "project_test_contract_refs", Status: "satisfied",
				ReasonCode: "project_test_contract_ref_observed", ContractRefs: []string{contract.ID},
				WitnessKind: WriteBehaviorWitnessProjectTest, Detail: "native contract observation",
			}, VerificationConfidenceRecord{
				Source: "native_symbol_observation", Category: "probe_changed_symbol", Status: "satisfied",
				ReasonCode: "project_test_symbol_observed", ChangedSymbolRefs: []string{"Date.Apply"},
				WitnessKind: WriteBehaviorWitnessProjectTest, Detail: "native symbol observation",
			})
			before := b1678DiagnosticJSON(t, []VerificationProofArtifact{{oldPlan, oldReport}, {newPlan, newReport}})
			oldLedger := BuildVerificationProofLedger(oldPlan, oldReport, nil)
			newLedger := BuildVerificationProofLedger(newPlan, newReport, nil)
			oldContract := b1678DiagnosticItem(t, oldLedger, "change_plan_behavior_contract", contract.ID)
			oldSymbol := b1678DiagnosticItem(t, oldLedger, "verification_probe", "Date.Apply")
			if oldContract.Status != VerificationProofLedgerItemMissing || oldSymbol.Status != VerificationProofLedgerItemMissing {
				t.Fatalf("pre-cumulative obligations must actually be missing: contract=%+v symbol=%+v", oldContract, oldSymbol)
			}
			if oldContract.ID == "" || oldContract.PlanID != oldPlan.ID || oldContract.ReportPlanID != "" ||
				oldSymbol.ID == "" || oldSymbol.PlanID != "" || oldSymbol.ReportPlanID != oldReport.PlanID {
				t.Fatalf("unexpected placeholder identities: contract=%+v symbol=%+v", oldContract, oldSymbol)
			}
			contractObservation := b1678DiagnosticItem(t, newLedger, "project_test_observation", contract.ID)
			symbolObservation := b1678DiagnosticItem(t, newLedger, "native_symbol_observation", "Date.Apply")
			for _, observation := range []VerificationProofLedgerItem{contractObservation, symbolObservation} {
				if observation.ID == "" || observation.PlanID != "" || observation.ReportPlanID != newReport.PlanID {
					t.Fatalf("unexpected native observation identity: %+v", observation)
				}
			}
			artifacts := []VerificationProofArtifact{{Plan: oldPlan, Report: oldReport}}
			ledger := BuildVerificationProofLedger(newPlan, newReport, artifacts)
			findIdentity := func(original VerificationProofLedgerItem) VerificationProofLedgerItem {
				t.Helper()
				for _, item := range ledger.Obligations {
					if item.ID == original.ID {
						if item.PlanID != original.PlanID || item.ReportPlanID != original.ReportPlanID {
							t.Fatalf("cumulative resolution changed identity: before=%+v after=%+v", original, item)
						}
						return item
					}
				}
				t.Fatalf("cumulative resolution lost original identity: %+v", original)
				return VerificationProofLedgerItem{}
			}
			resolvedContract, resolvedSymbol := findIdentity(oldContract), findIdentity(oldSymbol)
			if differentContract {
				if !reflect.DeepEqual(oldContract, resolvedContract) || ledger.UncoveredCount == 0 || ledger.State != VerificationProofLedgerLowConfidence {
					t.Errorf("different-contract derived veto changed: item=%+v ledger=%+v", resolvedContract, ledger)
				}
			} else if resolvedContract.Status != VerificationProofLedgerItemCovered || resolvedContract.ReasonCode != "resolved_by_cumulative_proof" ||
				resolvedContract.Detail != "behavior contract is covered by cumulative proof carrying the exact contract_ref" ||
				ledger.State != VerificationProofLedgerVerified || ledger.UncoveredCount != 0 {
				t.Errorf("cross-plan exact contract did not resolve coherently: %+v", ledger)
			}
			if resolvedSymbol.Status != VerificationProofLedgerItemCovered || resolvedSymbol.ReasonCode != "resolved_by_cumulative_proof" ||
				resolvedSymbol.Detail != "changed symbol is covered by cumulative proof carrying the exact symbol ref" {
				t.Errorf("cross-plan exact symbol did not resolve coherently: %+v", resolvedSymbol)
			}
			for _, observation := range []VerificationProofLedgerItem{contractObservation, symbolObservation} {
				if actual := findIdentity(observation); !reflect.DeepEqual(observation, actual) {
					t.Errorf("original observation changed in cumulative ledger: before=%+v after=%+v", observation, actual)
				}
			}
			if !ledger.Cumulative {
				t.Fatal("test did not exercise cumulative public artifacts path")
			}
			if again := BuildVerificationProofLedger(newPlan, newReport, artifacts); !reflect.DeepEqual(ledger, again) {
				t.Errorf("repeated cross-plan build changed ledger: %+v", again)
			}
			if string(before) != string(b1678DiagnosticJSON(t, []VerificationProofArtifact{{oldPlan, oldReport}, {newPlan, newReport}})) {
				t.Fatal("cumulative proof projection mutated original plan/report artifacts")
			}
		})
	}
}

func TestB1678ProofLedgerDoesNotMigratePersistedCoveredDiagnostic(t *testing.T) {
	original := VerificationProofLedgerItem{
		ID: "persisted-required-obligation", Kind: "behavior_contract", Status: VerificationProofLedgerItemCovered,
		Source: "change_plan_behavior_contract", PlanID: "historical-plan", ContractRef: "date-result",
		ReasonCode: "behavior_contract_observation_missing", Detail: "historical covered diagnostic is not a migration target",
	}
	ledger := NormalizeVerificationProofLedger(VerificationProofLedger{Obligations: []VerificationProofLedgerItem{original}})
	item := b1678DiagnosticItem(t, ledger, original.Source, original.ContractRef)
	// Public normalization gives omitted capability its existing canonical value;
	// no missing-to-covered transition occurred, so diagnostics remain verbatim.
	original.Capability = VerificationCapabilityUnknown
	if !reflect.DeepEqual(original, item) {
		t.Fatalf("already-covered historical record was migrated: before=%+v after=%+v", original, item)
	}
}
