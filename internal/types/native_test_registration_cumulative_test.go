package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNativeRegistrationProofCumulativeCurrentAuthority(t *testing.T) {
	for _, historical := range []string{"ordinary", "native_registration", "without_plan"} {
		for _, fresh := range []bool{false, true} {
			t.Run(historical+map[bool]string{false: "/old_only", true: "/fresh"}[fresh], func(t *testing.T) {
				p, r := nativeRegistrationProofFixture(t)
				hp, hr := nativeRegistrationProofFixture(t)
				hp.ID, hr.PlanID = "historical", "historical"
				hr.ExistingTestExecutions[0].PlanID = "historical"
				if historical == "native_registration" {
					hp.NativeTestRegistration.PlanID = hp.ID
					hp.NativeTestRegistration.AuthorizationID = "old-authorization"
					sealNativeTestRegistration(hp)
					hr.ExistingTestExecutions[0].NativeTestRegistrationDigest = NativeTestRegistrationDigest(hp)
					if !BehaviorContractRefHasVerificationWitness(hp, hr, "value") {
						t.Fatal("historical native positive fixture is invalid")
					}
				} else {
					hp.NativeTestRegistration, hp.PersistenceKind = nil, ""
					hr.ExistingTestExecutions = nil
					if historical == "without_plan" {
						hp = nil
					}
				}
				if !fresh {
					r.ExistingTestExecutions = nil
				}
				artifacts := []VerificationProofArtifact{{Plan: hp, Report: hr}}
				before, _ := json.Marshal([]any{p, r, artifacts})
				profile := BuildCumulativeVerificationProofProfile(p, r, artifacts)
				ledger := BuildVerificationProofLedger(p, r, artifacts)
				if (ledger.State == VerificationProofLedgerVerified) != fresh {
					t.Fatalf("old/fresh authority mismatch: fresh=%v profile=%+v ledger=%+v", fresh, profile, ledger)
				}
				missing := false
				for _, code := range profile.ReasonCodes {
					missing = missing || code == "behavior_contract_observation_missing"
				}
				if missing == fresh {
					t.Fatalf("profile borrowed or lost proof: %+v", profile)
				}
				// A second projection must not resurrect the historical receipt.
				if !reflect.DeepEqual(ledger, BuildVerificationProofLedger(p, r, artifacts)) {
					t.Fatal("cumulative projection is unstable")
				}
				after, _ := json.Marshal([]any{p, r, artifacts})
				if string(before) != string(after) || hr.verificationExcludedContractRefs != nil {
					t.Fatal("cumulative projection mutated stored history")
				}
			})
		}
	}
}

func TestNativeRegistrationProofCumulativePreservesIndependentRefsAndFailures(t *testing.T) {
	p, r := nativeRegistrationProofFixture(t)
	hp, hr := b1575PairConflictFixture("history")
	hp.BehaviorContracts = append([]WriteBehaviorContract(nil), p.BehaviorContracts...)
	independent := hp.BehaviorContracts[0]
	independent.ID = "independent"
	hp.BehaviorContracts = append(hp.BehaviorContracts, independent)
	hr.VerificationConfidence[0].ContractRefs = []string{"value", "independent"}
	artifacts := []VerificationProofArtifact{{Plan: hp, Report: hr}}
	for _, fresh := range []bool{true, false} {
		if !fresh {
			r.ExistingTestExecutions = nil
		}
		ledger := BuildVerificationProofLedger(p, r, artifacts)
		independentCovered := false
		for _, item := range ledger.Obligations {
			independentCovered = independentCovered || item.ContractRef == "independent" && item.Status == VerificationProofLedgerItemCovered
		}
		if !independentCovered || (ledger.State == VerificationProofLedgerVerified) != fresh {
			t.Fatalf("unrelated contract proof lost, or stale proof borrowed: %+v", ledger)
		}
	}
	// Historical native failures remain failures, not advisory successes.
	hp, hr = nativeRegistrationProofFixture(t)
	hp.ID, hr.PlanID = "failed-history", "failed-history"
	nativeRegistrationProofFail(hr)
	ledger := BuildVerificationProofLedger(p, r, []VerificationProofArtifact{{Plan: hp, Report: hr}})
	if ledger.FailedCount+ledger.CapabilityFailedCount == 0 {
		t.Fatalf("historical execution failure erased: %+v", ledger)
	}
}

func TestNativeRegistrationProofCumulativeDoesNotInferPrimaryFromOrder(t *testing.T) {
	p, r := b1575PairConflictFixture("ordinary-current")
	hp, hr := nativeRegistrationProofFixture(t)
	before := BuildVerificationProofLedger(p, r, nil)
	for _, artifacts := range [][]VerificationProofArtifact{{{Plan: hp, Report: hr}}, {{Plan: hp, Report: hr}, {Plan: p, Report: r}}} {
		ledger := BuildVerificationProofLedger(p, r, artifacts)
		if ledger.State != before.State {
			t.Fatalf("historical registration changed explicit primary authority: before=%+v after=%+v", before, ledger)
		}
	}
}

func TestNativeRegistrationProofHistoricalFailureNeedsFreshRerunReceipt(t *testing.T) {
	for _, state := range []string{"missing", "wrong", "valid"} {
		t.Run(state, func(t *testing.T) {
			p, r := nativeRegistrationProofFixture(t)
			hp, hr := nativeRegistrationProofFixture(t)
			hp.ID, hr.PlanID, hr.ExistingTestExecutions[0].PlanID = "old-failure", "old-failure", "old-failure"
			hp.NativeTestRegistration.PlanID = hp.ID
			hp.NativeTestRegistration.AuthorizationID = "old"
			sealNativeTestRegistration(hp)
			hr.ExistingTestExecutions[0].NativeTestRegistrationDigest = NativeTestRegistrationDigest(hp)
			nativeRegistrationProofFail(hr)
			if len(BuildVerifyFailureContractRelevance(hr, hp, nativeRegistrationProofBinding(hp)).Hits) != 1 {
				t.Fatal("historical failed receipt is not valid")
			}
			switch state {
			case "missing":
				r.ExistingTestExecutions = nil
			case "wrong":
				r.ExistingTestExecutions[0].NativeTestRegistrationDigest = "stale"
			}
			before, _ := json.Marshal([]any{p, r, hp, hr})
			ledger := BuildVerificationProofLedger(p, r, []VerificationProofArtifact{{Plan: hp, Report: hr}})
			if (ledger.CapabilityFailedCount > 0) != (state != "valid") || (ledger.State == VerificationProofLedgerVerified) != (state == "valid") {
				t.Fatalf("historical failure was incorrectly resolved or retained: %+v", ledger)
			}
			after, _ := json.Marshal([]any{p, r, hp, hr})
			if string(before) != string(after) || hr.Passed || hr.ExecutedCommands[0].ExitCode == 0 {
				t.Fatal("historical failure facts rewritten")
			}
		})
	}
}
