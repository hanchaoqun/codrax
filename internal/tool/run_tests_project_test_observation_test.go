package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestVerificationConfidenceRequiresExactProjectTestCandidateExecution(t *testing.T) {
	plan := &types.ChangePlan{
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID: "ordinary-number-format", Required: true,
		}},
		ProjectTestObservations: []types.ProjectTestObservation{{
			ID: "format-regression", TestPath: "tests/long_double_format.cpp",
			ContractRefs: []string{"ordinary-number-format"},
		}},
	}
	report := &types.ChangeReport{
		Passed: true,
		TestSurface: &types.TestSurface{Candidates: []types.TestSurfaceCandidate{{
			ID: "make@.", Runner: "make", WorkingDir: ".", MakeTarget: "check",
			HasTestSignal: true,
			DeclaredCoveragePaths: []string{
				"tests/long_double_format.cpp",
			},
		}}},
		ExecutedCommands: []types.ExecutedCommand{{
			Runner: "make", WorkingDir: ".", Suite: "check",
			Outcome: "executed", ExitCode: 0,
		}},
	}

	records := verificationConfidenceRecordsFromReport(plan, report)
	if !verificationConfidenceContains(records, "project_test_contract_refs", "satisfied", "project_test_contract_ref_observed") {
		t.Fatalf("exact successful project-test candidate did not mint a contract receipt: %+v", records)
	}
	if verificationConfidenceContains(records, "project_test_contract_refs", "missing", "project_test_observation_not_executed") {
		t.Fatalf("successful exact observation retained false missing debt: %+v", records)
	}
}

func TestVerificationConfidenceRejectsProjectTestTargetOrPathMismatch(t *testing.T) {
	plan := &types.ChangePlan{ProjectTestObservations: []types.ProjectTestObservation{{
		ID: "format-regression", TestPath: "tests/long_double_format.cpp",
		ContractRefs: []string{"ordinary-number-format"},
	}}}
	base := &types.ChangeReport{
		Passed: true,
		TestSurface: &types.TestSurface{Candidates: []types.TestSurfaceCandidate{{
			ID: "make@.", Runner: "make", WorkingDir: ".", MakeTarget: "check",
			HasTestSignal: true,
			DeclaredCoveragePaths: []string{
				"tests/another_test.cpp",
			},
		}}},
		ExecutedCommands: []types.ExecutedCommand{{
			Runner: "make", WorkingDir: ".", Suite: "check",
			Outcome: "executed", ExitCode: 0,
		}},
	}

	records := verificationConfidenceRecordsFromReport(plan, base)
	if verificationConfidenceContains(records, "project_test_contract_refs", "satisfied", "project_test_contract_ref_observed") ||
		!verificationConfidenceContains(records, "project_test_contract_refs", "missing", "project_test_observation_not_executed") {
		t.Fatalf("path mismatch must fail closed: %+v", records)
	}

	base.TestSurface.Candidates[0].DeclaredCoveragePaths = []string{"tests/long_double_format.cpp"}
	base.ExecutedCommands[0].Suite = "install"
	records = verificationConfidenceRecordsFromReport(plan, base)
	if verificationConfidenceContains(records, "project_test_contract_refs", "satisfied", "project_test_contract_ref_observed") ||
		!verificationConfidenceContains(records, "project_test_contract_refs", "missing", "project_test_observation_not_executed") {
		t.Fatalf("target mismatch must fail closed: %+v", records)
	}
}

func TestAggregateProjectPassWithoutObservationDoesNotMintContractReceipt(t *testing.T) {
	plan := &types.ChangePlan{BehaviorContracts: []types.WriteBehaviorContract{{ID: "ordinary-number-format", Required: true}}}
	report := &types.ChangeReport{Passed: true, ExecutedCommands: []types.ExecutedCommand{{
		Runner: "make", WorkingDir: ".", Suite: "check", Outcome: "executed", ExitCode: 0,
	}}}
	for _, record := range verificationConfidenceRecordsFromReport(plan, report) {
		if record.Category == "project_test_contract_refs" && record.Status == "satisfied" {
			t.Fatalf("aggregate project pass borrowed undeclared contract authority: %+v", record)
		}
	}
}
