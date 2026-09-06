package tool

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func projectFailureScopeFixture() (*types.ChangePlan, *types.ChangeReport) {
	plan := &types.ChangePlan{ID: "plan-scope", ProjectTestObservations: []types.ProjectTestObservation{{
		ID: "a-boundary", TestPath: "tests/a_test.py", AssertionSuite: "BoundaryTests", AssertionID: "test_edge",
		ContractRefs: []string{"a-contract"},
	}}}
	report := &types.ChangeReport{
		PlanID: plan.ID,
		TestSurface: &types.TestSurface{Candidates: []types.TestSurfaceCandidate{{
			Runner: "python", Framework: pythonFrameworkPytest, WorkingDir: ".", HasTestSignal: true,
		}}},
		ExecutedCommands: []types.ExecutedCommand{{
			Runner: "python", Framework: pythonFrameworkPytest, WorkingDir: ".", Suite: "tests/a_test.py",
			Outcome: types.ExecutedCommandOutcomeExecuted, ExitCode: 1,
		}},
		TestResults: []types.TestResult{{
			Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAssertion,
			Suite: "tests/a_test.py::BoundaryTests", AssertionID: "test_edge", Passed: false,
		}},
	}
	return plan, report
}

func TestProjectFailureContractRelevanceRequiresExactExecutionAndFile(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*types.ChangePlan, *types.ChangeReport)
		want   bool
	}{
		{"same assertion and exact execution", func(*types.ChangePlan, *types.ChangeReport) {}, true},
		{"other file same assertion", func(_ *types.ChangePlan, r *types.ChangeReport) {
			r.TestResults[0].Suite = "tests/b_test.py::BoundaryTests"
		}, false},
		{"other runner", func(_ *types.ChangePlan, r *types.ChangeReport) { r.ExecutedCommands[0].Runner = "ruby" }, false},
		{"other framework", func(_ *types.ChangePlan, r *types.ChangeReport) {
			r.ExecutedCommands[0].Framework = pythonFrameworkUnittest
		}, false},
		{"other workdir", func(_ *types.ChangePlan, r *types.ChangeReport) { r.ExecutedCommands[0].WorkingDir = "other" }, false},
		{"other command scope", func(_ *types.ChangePlan, r *types.ChangeReport) { r.ExecutedCommands[0].Suite = "tests/b_test.py" }, false},
		{"missing test surface", func(_ *types.ChangePlan, r *types.ChangeReport) { r.TestSurface = nil }, false},
		{"missing commands", func(_ *types.ChangePlan, r *types.ChangeReport) { r.ExecutedCommands = nil }, false},
		{"runner unavailable", func(_ *types.ChangePlan, r *types.ChangeReport) {
			r.ExecutedCommands[0].Outcome = types.ExecutedCommandOutcomeRunnerMissing
		}, false},
		{"passed command cannot own failed row", func(_ *types.ChangePlan, r *types.ChangeReport) { r.ExecutedCommands[0].ExitCode = 0 }, false},
		{"passed assertion", func(_ *types.ChangePlan, r *types.ChangeReport) { r.TestResults[0].Passed = true }, false},
		{"aggregate row", func(_ *types.ChangePlan, r *types.ChangeReport) {
			r.TestResults[0].ObservationScope = types.TestObservationScopeAggregate
		}, false},
		{"multiple failed command sources", func(_ *types.ChangePlan, r *types.ChangeReport) {
			r.ExecutedCommands = append(r.ExecutedCommands, types.ExecutedCommand{
				Runner: "ruby", WorkingDir: ".", Suite: "spec/a_spec.rb", Outcome: types.ExecutedCommandOutcomeExecuted, ExitCode: 1,
			})
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, report := projectFailureScopeFixture()
			tc.mutate(plan, report)
			got := BuildVerifyFailureContractRelevance(report, plan)
			wantCount := 0
			if tc.want {
				wantCount = 1
			}
			if len(got.Hits) != wantCount {
				t.Fatalf("bound=%v want %v: %+v", len(got.Hits) == 1, tc.want, got)
			}
		})
	}
}

func TestProjectAssertionSuccessAndFailureUseTheSamePathResolver(t *testing.T) {
	for _, wrongFile := range []bool{false, true} {
		plan, report := projectFailureScopeFixture()
		if wrongFile {
			report.TestResults[0].Suite = "tests/b_test.py::BoundaryTests"
		}
		failed := len(BuildVerifyFailureContractRelevance(report, plan).Hits) > 0
		report.TestResults[0].Passed = true
		report.ExecutedCommands[0].ExitCode = 0
		passed := projectTestObservationExecuted(plan.ProjectTestObservations[0], report)
		if failed != passed || passed == wrongFile {
			t.Fatalf("same file authority required for both verdicts: wrongFile=%v failed=%v passed=%v", wrongFile, failed, passed)
		}
	}
}

func TestProjectFailureContractRelevanceBindsRealNativeTestResult(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable Python runtime")
	}
	root := t.TempDir()
	for _, dir := range []string{"pkg", "tests"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string]string{
		"pkg/value.py":        "VALUE = 1\n",
		"tests/test_value.py": "import unittest\nclass ValueTest(unittest.TestCase):\n    def test_value(self):\n        self.assertEqual(1, 2)\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mu := types.NewMutableState("failed project assertion source")
	mu.SetChangePlan(&types.ChangePlan{
		ID: "native-failed-observation", Status: types.PlanStatusApplied,
		TargetPaths: []string{"pkg/value.py"}, Changes: []types.FileChange{{Path: "pkg/value.py", Kind: "patch"}},
		BehaviorContracts: []types.WriteBehaviorContract{{ID: "value-contract", Required: true}},
		ProjectTestObservations: []types.ProjectTestObservation{{
			ID: "value-observation", TestPath: "tests/test_value.py", AssertionSuite: "ValueTest", AssertionID: "test_value",
			ContractRefs: []string{"value-contract"},
		}},
	})
	ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root}
	_, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"runner": "python", "framework": "unittest"}))
	if err != nil {
		t.Fatal(err)
	}
	report := mu.ChangeReport()
	got := BuildVerifyFailureContractRelevance(report, mu.ChangePlan())
	if len(got.Hits) != 1 || got.Hits[0].ContractID != "value-contract" {
		t.Fatalf("real exact project execution lost failure relevance: relevance=%+v report=%+v", got, report)
	}
	if got.Hits[0].EvidenceRefs[0] != "assertion:tests/test_value.py::ValueTest::test_value" {
		t.Fatalf("failure witness lost its source file: %+v", got.Hits)
	}
	// The same parsed result may not be reattached to an identically named
	// declaration in another file, even though its suite suffix still matches.
	other := *mu.ChangePlan()
	other.ProjectTestObservations = append([]types.ProjectTestObservation(nil), other.ProjectTestObservations...)
	other.ProjectTestObservations[0].TestPath = "tests/other_test.py"
	if rebound := BuildVerifyFailureContractRelevance(report, &other); len(rebound.Hits) != 0 {
		t.Fatalf("real result was stolen by sibling file: %+v", rebound)
	}
}

func TestProjectFailureContractRelevanceRejectsAmbiguousMakeRoster(t *testing.T) {
	plan, report := projectFailureScopeFixture()
	plan.ProjectTestObservations[0].TestPath = "tests/a_test.cpp"
	report.TestSurface.Candidates = []types.TestSurfaceCandidate{{
		Runner: "make", WorkingDir: ".", MakeTarget: "check", HasTestSignal: true,
		DeclaredCoveragePaths: []string{"tests/a_test.cpp"},
	}}
	report.ExecutedCommands = []types.ExecutedCommand{{Runner: "make", WorkingDir: ".", Suite: "check", Outcome: types.ExecutedCommandOutcomeExecuted, ExitCode: 1}}
	report.TestResults[0].Suite = "BoundaryTests"
	if got := BuildVerifyFailureContractRelevance(report, plan); len(got.Hits) != 1 {
		t.Fatalf("single-file bounded execution should retain its binding: %+v", got)
	}
	report.TestSurface.Candidates[0].DeclaredCoveragePaths = append(report.TestSurface.Candidates[0].DeclaredCoveragePaths, "tests/b_test.cpp")
	if got := BuildVerifyFailureContractRelevance(report, plan); len(got.Hits) != 0 {
		t.Fatalf("multi-file Make roster cannot bind a file-less assertion row: %+v", got)
	}
}

func TestProjectFailureRetiresOnlyTheExecutedSiblingExpectation(t *testing.T) {
	plan, report := projectFailureScopeFixture()
	plan.BehaviorContracts = []types.WriteBehaviorContract{
		{ID: "a-contract", Kind: types.WriteBehaviorInvariant, Operator: types.WriteBehaviorOpSatisfies, Required: true, Expected: "a boundary keeps its shape"},
		{ID: "b-contract", Kind: types.WriteBehaviorInvariant, Operator: types.WriteBehaviorOpSatisfies, Required: true, Expected: "b boundary keeps its shape"},
	}
	bObservation := plan.ProjectTestObservations[0]
	bObservation.ID, bObservation.TestPath, bObservation.ContractRefs = "b-boundary", "tests/b_test.py", []string{"b-contract"}
	plan.ProjectTestObservations = append(plan.ProjectTestObservations, bObservation)
	report.ExecutedCommands[0].Suite = "tests/b_test.py"
	report.TestResults[0].Suite = "tests/b_test.py::BoundaryTests"
	relevance := BuildVerifyFailureContractRelevance(report, plan)
	retained, tombstones := types.RebaseVerifyFailureWriteBehaviorContracts(plan.BehaviorContracts, nil, types.WriteBehaviorContractRetirementDecision{
		Lane: types.FailureKindContractRetireRelevanceSubset, Relevance: relevance,
		PlanID: plan.ID, FailureKind: types.FailureKindTestsFailed, Attempt: 1,
	})
	if len(retained) != 1 || retained[0].ID != "a-contract" || len(tombstones) != 1 || tombstones[0].ID != "b-contract" {
		t.Fatalf("sibling failure retired another expectation: relevance=%+v retained=%+v tombstones=%+v", relevance, retained, tombstones)
	}
}
