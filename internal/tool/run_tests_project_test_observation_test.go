package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRunTestsExecutesDeclaredProjectTestObservationPath(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python on PATH; skip")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatalf("mkdir tests: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "value.py"), []byte("VALUE = 1\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	testBody := "import unittest\n\nclass ValueTest(unittest.TestCase):\n    def test_value(self):\n        self.assertEqual(1, 1)\n"
	if err := os.WriteFile(filepath.Join(root, "tests", "test_value.py"), []byte(testBody), 0o644); err != nil {
		t.Fatalf("write test: %v", err)
	}
	mu := types.NewMutableState("project observation execution")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-project-observation-execution",
		Status:      types.PlanStatusApplied,
		TargetPaths: []string{"pkg/value.py"},
		Changes:     []types.FileChange{{Path: "pkg/value.py", Kind: "patch"}},
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID: "value-contract", Required: true,
		}},
		ProjectTestObservations: []types.ProjectTestObservation{{
			ID: "value-observation", TestPath: "tests/test_value.py",
			AssertionSuite: "ValueTest", AssertionID: "test_value",
			ContractRefs: []string{"value-contract"},
		}},
	})
	ctx := &types.BusContext{
		Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify,
		RepoRoot: root, MainRepoRoot: root,
	}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("exact observation run should pass: %+v", result)
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("missing change report")
	}
	foundExact := false
	for _, cmd := range report.ExecutedCommands {
		if cmd.Runner == "python" && cmd.Framework == pythonFrameworkUnittest &&
			cmd.Outcome == "executed" && cmd.ExitCode == 0 && cmd.Suite == "tests/test_value.py" {
			foundExact = true
			if strings.Contains(cmd.Command, "discover -s") {
				t.Fatalf("observation file was widened to directory discovery: %q", cmd.Command)
			}
		}
	}
	if !foundExact {
		t.Fatalf("missing exact observation command: %+v", report.ExecutedCommands)
	}
	records := verificationConfidenceRecordsFromReport(mu.ChangePlan(), report)
	if !verificationConfidenceContains(records, "project_test_contract_refs", "satisfied", "project_test_contract_ref_observed") ||
		verificationConfidenceContains(records, "project_test_contract_refs", "missing", "project_test_assertion_not_observed") {
		t.Fatalf("executed exact assertion did not close observation debt: %+v", records)
	}
}

func TestVerificationConfidenceRequiresExactProjectTestCandidateExecution(t *testing.T) {
	plan := &types.ChangePlan{
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID: "ordinary-number-format", Required: true,
		}},
		ProjectTestObservations: []types.ProjectTestObservation{{
			ID: "format-regression", TestPath: "tests/long_double_format.cpp",
			AssertionSuite: "tests/long_double_format.cpp", AssertionID: "long_double_value",
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
		TestResults: []types.TestResult{{
			Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAssertion,
			Suite: "tests/long_double_format.cpp", AssertionID: "long_double_value", Passed: true,
		}},
	}

	records := verificationConfidenceRecordsFromReport(plan, report)
	if !verificationConfidenceContains(records, "project_test_contract_refs", "satisfied", "project_test_contract_ref_observed") {
		t.Fatalf("exact successful project-test candidate did not mint a contract receipt: %+v", records)
	}
	if verificationConfidenceContains(records, "project_test_contract_refs", "missing", "project_test_assertion_not_observed") {
		t.Fatalf("successful exact observation retained false missing debt: %+v", records)
	}
}

func TestVerificationConfidenceRejectsProjectTestTargetOrPathMismatch(t *testing.T) {
	plan := &types.ChangePlan{ProjectTestObservations: []types.ProjectTestObservation{{
		ID: "format-regression", TestPath: "tests/long_double_format.cpp",
		AssertionSuite: "tests/long_double_format.cpp", AssertionID: "long_double_value",
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
		TestResults: []types.TestResult{{
			Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAssertion,
			Suite: "tests/long_double_format.cpp", AssertionID: "long_double_value", Passed: true,
		}},
	}

	records := verificationConfidenceRecordsFromReport(plan, base)
	if verificationConfidenceContains(records, "project_test_contract_refs", "satisfied", "project_test_contract_ref_observed") ||
		!verificationConfidenceContains(records, "project_test_contract_refs", "missing", "project_test_assertion_not_observed") {
		t.Fatalf("path mismatch must fail closed: %+v", records)
	}

	base.TestSurface.Candidates[0].DeclaredCoveragePaths = []string{"tests/long_double_format.cpp"}
	base.ExecutedCommands[0].Suite = "install"
	records = verificationConfidenceRecordsFromReport(plan, base)
	if verificationConfidenceContains(records, "project_test_contract_refs", "satisfied", "project_test_contract_ref_observed") ||
		!verificationConfidenceContains(records, "project_test_contract_refs", "missing", "project_test_assertion_not_observed") {
		t.Fatalf("target mismatch must fail closed: %+v", records)
	}
}

func TestVerificationConfidenceAcceptsExactNonMakeProjectTestObservation(t *testing.T) {
	plan := &types.ChangePlan{ProjectTestObservations: []types.ProjectTestObservation{{
		ID: "newline-regression", TestPath: "tests/test_tokenizer.py",
		AssertionSuite: "TokenizerTest", AssertionID: "test_consecutive_newline_run_uses_pair_merge_token",
		ContractRefs: []string{"newline-fold"},
	}}}
	report := &types.ChangeReport{
		Passed: true,
		TestSurface: &types.TestSurface{Candidates: []types.TestSurfaceCandidate{{
			ID: "python/unittest@.", Runner: "python", Framework: pythonFrameworkUnittest,
			WorkingDir: ".", HasTestSignal: true,
		}}},
		ExecutedCommands: []types.ExecutedCommand{{
			Runner: "python", Framework: pythonFrameworkUnittest, WorkingDir: ".",
			Suite: "tests/test_tokenizer.py", Outcome: "executed", ExitCode: 0,
		}},
		TestResults: []types.TestResult{{
			Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAssertion,
			Suite:       "tests.test_tokenizer.TokenizerTest",
			AssertionID: "test_consecutive_newline_run_uses_pair_merge_token", Passed: true,
		}},
	}

	records := verificationConfidenceRecordsFromReport(plan, report)
	if !verificationConfidenceContains(records, "project_test_contract_refs", "satisfied", "project_test_contract_ref_observed") ||
		verificationConfidenceContains(records, "project_test_contract_refs", "missing", "project_test_assertion_not_observed") {
		t.Fatalf("exact non-Make observation did not mint its contract receipt: %+v", records)
	}
}

func TestVerificationConfidenceRejectsBroadOrMismatchedNonMakeProjectTestObservation(t *testing.T) {
	plan := &types.ChangePlan{ProjectTestObservations: []types.ProjectTestObservation{{
		ID: "newline-regression", TestPath: "tests/test_tokenizer.py",
		AssertionSuite: "TokenizerTest", AssertionID: "test_newline",
		ContractRefs: []string{"newline-fold"},
	}}}
	base := &types.ChangeReport{
		Passed: true,
		TestSurface: &types.TestSurface{Candidates: []types.TestSurfaceCandidate{{
			ID: "python/unittest@.", Runner: "python", Framework: pythonFrameworkUnittest,
			WorkingDir: ".", HasTestSignal: true,
		}}},
		ExecutedCommands: []types.ExecutedCommand{{
			Runner: "python", Framework: pythonFrameworkUnittest, WorkingDir: ".",
			Suite: "tests", Outcome: "executed", ExitCode: 0,
		}},
		TestResults: []types.TestResult{{
			Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAssertion,
			Suite: "tests.test_tokenizer.TokenizerTest", AssertionID: "test_newline", Passed: true,
		}},
	}

	assertMissing := func(label string) {
		t.Helper()
		records := verificationConfidenceRecordsFromReport(plan, base)
		if verificationConfidenceContains(records, "project_test_contract_refs", "satisfied", "project_test_contract_ref_observed") ||
			!verificationConfidenceContains(records, "project_test_contract_refs", "missing", "project_test_assertion_not_observed") {
			t.Fatalf("%s must fail closed: %+v", label, records)
		}
	}
	assertMissing("directory-wide command")

	base.ExecutedCommands[0].Suite = "tests/test_tokenizer.py"
	base.TestResults[0].Suite = "tests.test_other.OtherTokenizerTest"
	assertMissing("non-boundary suite mismatch")

	base.TestResults[0].Suite = "tests.test_tokenizer.TokenizerTest"
	base.TestResults[0].ObservationScope = types.TestObservationScopeAggregate
	assertMissing("aggregate result")
}

func TestAggregateProjectResultCannotMintAssertionContractReceipt(t *testing.T) {
	plan := &types.ChangePlan{ProjectTestObservations: []types.ProjectTestObservation{{
		ID: "format-regression", TestPath: "tests/long_double_format.cpp",
		AssertionSuite: "check", AssertionID: "make-test",
		ContractRefs: []string{"ordinary-number-format"},
	}}}
	report := &types.ChangeReport{
		Passed: true,
		TestSurface: &types.TestSurface{Candidates: []types.TestSurfaceCandidate{{
			ID: "make@.", Runner: "make", WorkingDir: ".", MakeTarget: "check",
			HasTestSignal: true, DeclaredCoveragePaths: []string{"tests/long_double_format.cpp"},
		}}},
		ExecutedCommands: []types.ExecutedCommand{{
			Runner: "make", WorkingDir: ".", Suite: "check", Outcome: "executed", ExitCode: 0,
		}},
		TestResults: []types.TestResult{{
			Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAggregate,
			Suite: "check", AssertionID: "make-test", Passed: true,
		}},
	}
	records := verificationConfidenceRecordsFromReport(plan, report)
	if verificationConfidenceContains(records, "project_test_contract_refs", "satisfied", "project_test_contract_ref_observed") ||
		!verificationConfidenceContains(records, "project_test_contract_refs", "missing", "project_test_assertion_not_observed") {
		t.Fatalf("aggregate Make result borrowed assertion authority: %+v", records)
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
