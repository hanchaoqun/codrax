package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// Real nested unittest assertions and the root's empty discovery are both
// retained. A secondary empty invocation must not erase the native verdict or
// supply the missing identity of a required behavior-contract witness.
func TestRunTestsMixedNoTestsPublicAuthority(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git unavailable")
	}
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("Python unavailable")
	}
	for _, tc := range []struct {
		name                       string
		wrongResult, wrongIdentity bool
	}{
		{name: "native_pass_and_root_zero_tests"},
		{name: "native_failure_and_root_zero_tests", wrongResult: true},
		{name: "native_pass_keeps_required_identity_gap", wrongIdentity: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			files := map[string]string{
				".gitignore":                           "__pycache__/\n*.pyc\n",
				"packages/widget/setup.py":             "from setuptools import setup\nsetup(name='widget', version='0.0.0')\n",
				"packages/widget/widget.py":            "def increment(value):\n    return value\n",
				"packages/widget/tests/__init__.py":    "",
				"packages/widget/tests/test_widget.py": "import unittest\nfrom widget import increment\nclass IncrementTest(unittest.TestCase):\n    def test_increment(self):\n        self.assertEqual(increment(-1), 0)\n        self.assertEqual(increment(0), 1)\n        self.assertEqual(increment(2), 3)\n",
			}
			for path, body := range files {
				full := filepath.Join(root, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			b1575FixtureGit(t, root, "init", "-q")
			b1575FixtureGit(t, root, "add", ".")
			b1575FixtureGit(t, root, "-c", "user.name=Codrax Test", "-c", "user.email=codrax-test@example.invalid", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "-qm", "baseline")
			ctx := newTestBusCtx()
			ctx.RepoRoot, ctx.MainRepoRoot = root, root
			ctx.Mode, ctx.PipelineStage = types.ModeApply, types.StagePlan
			ctx.Mutable.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{
				Task:              types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopePackage, Summary: "increment input"},
				BehaviorContracts: []types.WriteBehaviorContract{{ID: "increment", Kind: types.WriteBehaviorObservable, Operator: types.WriteBehaviorOpEquals, Expected: "increment returns value plus one", Required: true}},
			}})
			for _, path := range []string{"packages/widget/widget.py", "packages/widget/tests/test_widget.py"} {
				res, err := (&ReadFile{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"path": path}))
				if err != nil || !res.Success {
					t.Fatalf("read: %v %+v", err, res)
				}
			}
			after := "def increment(value):\n    return value + 1\n"
			if tc.wrongResult {
				after = "def increment(value):\n    return value + 2\n"
			}
			assertion := "python/unittest@packages/widget::test_increment"
			if tc.wrongIdentity {
				assertion = "python/unittest@packages/widget::test_other"
			}
			res, err := (&EmitChangePlan{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
				"request": "fix increment", "summary": "repair source and use existing tests",
				"changes":                   []map[string]any{{"path": "packages/widget/widget.py", "kind": "modify", "new_content": after, "rationale": "increment the input"}},
				"project_test_observations": []map[string]any{{"id": "existing", "test_path": "packages/widget/tests/test_widget.py", "assertion_suite": "python/unittest@packages/widget::tests.test_widget.IncrementTest", "assertion_id": assertion, "contract_refs": []string{"increment"}}},
			}))
			if err != nil || !res.Success {
				t.Fatalf("emit: %v %+v", err, res)
			}
			plan := ctx.Mutable.ChangePlan()
			ctx.PipelineStage = types.StageApply
			res, err = (&ApplyPatch{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"path": "packages/widget/widget.py", "kind": "modify"}))
			if err != nil || !res.Success {
				t.Fatalf("apply: %v %+v", err, res)
			}
			ctx.PipelineStage = types.StageVerify
			res, err = (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{}))
			if err != nil {
				t.Fatal(err)
			}
			report := ctx.Mutable.ChangeReport()
			if report == nil {
				t.Fatalf("missing report: %+v", res)
			}
			wire, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			var restored types.ChangeReport
			if err := json.Unmarshal(wire, &restored); err != nil {
				t.Fatal(err)
			}
			t.Logf("ACTUAL_REPORT=%s", wire)
			zero, native := false, false
			for _, cmd := range restored.ExecutedCommands {
				zero = zero || (cmd.WorkingDir == "." && cmd.Outcome == types.ExecutedCommandOutcomeZeroTests)
			}
			for _, row := range restored.TestResults {
				native = native || (row.ObservationScope == types.TestObservationScopeAssertion && row.Passed != tc.wrongResult)
			}
			if !zero || !native || len(restored.NoTestsRunners) == 0 {
				t.Fatalf("mixed real producer prerequisites: zero=%t native=%t report=%+v", zero, native, restored)
			}
			if !strings.Contains(res.Summary, "python/unittest@.") || !strings.Contains(res.Summary, "Other invocations' assertion results are retained independently") {
				t.Errorf("public summary lost local zero-test scope: %s", res.Summary)
			}
			wantStatus := types.VerificationStatusPassed
			if tc.wrongResult {
				wantStatus = types.VerificationStatusFailed
			}
			if restored.NormalizeVerificationStatus() != wantStatus {
				t.Errorf("normalized status=%s want=%s", restored.NormalizeVerificationStatus(), wantStatus)
			}
			restored.EnsureVerificationStatus()
			if restored.VerificationStatus != wantStatus {
				t.Errorf("ensured status=%s want=%s", restored.VerificationStatus, wantStatus)
			}
			want := writeflow.ObservationAuthorityVerified
			if tc.wrongResult {
				want = writeflow.ObservationAuthorityFailed
			}
			authority := writeflow.DeriveObservationAuthorityFromReport(&restored, nil)
			if authority.State != want {
				t.Errorf("mixed authority=%+v want=%s", authority, want)
			}
			missing := verificationConfidenceContains(restored.VerificationConfidence, "project_test_contract_refs", "missing", "project_test_assertion_not_observed")
			if tc.wrongIdentity && (!missing || types.BehaviorContractRefHasVerificationWitness(plan, &restored, "increment")) {
				t.Fatal("native suite success erased required assertion identity gap")
			}
			if tc.wrongIdentity && types.BuildVerificationProofProfile(plan, &restored).Status != types.VerificationProofWeak {
				t.Fatal("unbound required identity must retain weak proof despite native suite success")
			}
		})
	}
}

func TestMixedNoTestsMergeKeepsLeafScopeAndFailure(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, tc := range []struct {
			name                                        string
			falseNoTests, failNative, unavailableNative bool
			want                                        types.VerificationStatus
		}{
			{name: "pass", want: types.VerificationStatusPassed},
			{name: "failure", failNative: true, want: types.VerificationStatusFailed},
			{name: "false_explicit_no_tests", falseNoTests: true, want: types.VerificationStatusUnavailable},
			{name: "missing_runner", unavailableNative: true, want: types.VerificationStatusUnavailable},
		} {
			t.Run(tc.name+map[bool]string{false: "/forward", true: "/reverse"}[reverse], func(t *testing.T) {
				zero := &types.ChangeReport{Passed: true, NoTestsRunners: []string{"python"}}
				zero.EnsureVerificationStatus()
				if zero.FailureKind != types.FailureKindNoTests {
					t.Fatal("leaf prerequisite")
				}
				if tc.falseNoTests {
					zero.Passed = false
				}
				native := &types.ChangeReport{Passed: !tc.failNative, TestResults: []types.TestResult{{Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAssertion, AssertionID: "python/unittest@pkg::test_value", Suite: "python/unittest@pkg::tests.Widget", Passed: !tc.failNative}}}
				if tc.failNative {
					native.FailureKind = types.FailureKindTestsFailed
				}
				if tc.unavailableNative {
					native.Passed = false
					native.FailureKind = types.FailureKindRunnerMissing
				}
				reports := []*types.ChangeReport{zero, native}
				if reverse {
					reports[0], reports[1] = reports[1], reports[0]
				}
				merged := mergeChangeReports(reports)
				if merged.NormalizeVerificationStatus() != tc.want || len(merged.NoTestsRunners) != 1 || len(merged.TestResults) != 1 {
					t.Fatalf("merged=%+v want=%s", merged, tc.want)
				}
				if zero.FailureKind != types.FailureKindNoTests {
					t.Fatal("merge mutated local evidence")
				}
			})
		}
	}
	failed := makeZeroTestsFailureReport(runnerPlan{Runner: "python"}, "")
	failed = mergeChangeReports([]*types.ChangeReport{failed, {Passed: true, NoTestsRunners: []string{"ruby"}}})
	if failed.NormalizeVerificationStatus() != types.VerificationStatusFailed || failed.TestResults[0].Passed {
		t.Fatal("typed runnable-surface zero-test failure was weakened")
	}
}
