package tool

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the actual reason producer and report installation. Deliberately
// declaring an assertion identity that the aggregate Make runner cannot mint
// must remain visible in the report. Plain Python refs cannot discharge that
// debt, regardless of whether the declared list is complete.
func TestRunTestsProjectReasonRetainedForPlainPythonProbeDeclarations(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python on PATH")
	}
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make unavailable")
	}
	for _, complete := range []bool{true, false} {
		t.Run(map[bool]string{true: "complete refs", false: "partial refs"}[complete], func(t *testing.T) {
			root := t.TempDir()
			files := map[string]string{
				"widget.py":             "def increment(value):\n    return value + 1\n",
				"tests/check_widget.py": "import sys\nsys.path.insert(0, '.')\nimport widget\nassert widget.increment(2) == 3\nassert widget.increment(-1) == 0\n",
				"Makefile":              "check: widget.py tests/check_widget.py\n\t@python3 tests/check_widget.py\n",
			}
			for path, body := range files {
				path = filepath.Join(root, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			probeRefs := []string{"value"}
			probeCode := "import widget\nassert widget.increment(2) == 3\n"
			if complete {
				probeRefs = append(probeRefs, "boundary")
				probeCode += "assert widget.increment(-1) == 0\n"
			}
			plan := &types.ChangePlan{
				ID: "plan-proof-reason", Status: types.PlanStatusApplied,
				TargetPaths: []string{"widget.py"}, Changes: []types.FileChange{{Path: "widget.py", Kind: "patch"}},
				BehaviorContracts: []types.WriteBehaviorContract{
					{ID: "value", Kind: types.WriteBehaviorObservable, Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpSatisfies, Expected: "increments positive values", Required: true},
					{ID: "boundary", Kind: types.WriteBehaviorObservable, Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpSatisfies, Expected: "increments negative boundary", Required: true},
				},
				ProjectTestObservations: []types.ProjectTestObservation{{
					ID: "declared-check", TestPath: "tests/check_widget.py", AssertionSuite: "WidgetTest", AssertionID: "test_increment", ContractRefs: []string{"value", "boundary"},
				}},
				VerificationProbes: []types.VerificationProbe{{
					ID: "increment-contracts", Language: "python", Code: probeCode, ContractRefs: probeRefs, ChangedSymbolRefs: []string{"path:widget.py"},
				}},
			}
			mu := types.NewMutableState("project observation reason resolution")
			mu.SetChangePlan(plan)
			ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root}
			b1575BindAppliedPythonLines(t, ctx, plan, "widget.py", []int{2})
			result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"runner": "make"}))
			if err != nil {
				t.Fatalf("local execution failed: result=%+v err=%v report=%+v", result, err, mu.ChangeReport())
			}
			report := mu.ChangeReport()
			if report == nil {
				t.Fatal("execution report missing")
			}
			if !report.HasTargetExecutionCoverage() {
				t.Fatalf("real changed-function execution was not retained: %s", b1575TargetReceiptsJSON(report))
			}
			const reason = "project_test_assertion_not_observed"
			if !verificationConfidenceContains(report.VerificationConfidence, "project_test_contract_refs", "missing", reason) {
				t.Fatalf("producer must keep the unobserved project assertion as history: %+v", report.VerificationConfidence)
			}
			covered := types.CoveredWriteBehaviorContractIDs(types.ChangePlanVerificationBehaviorContracts(mu.ChangePlan()), report.VerificationConfidence)
			if len(covered) != 0 {
				t.Fatalf("plain probe declaration became an assertion receipt: %+v", report.VerificationConfidence)
			}
			profile := types.BuildVerificationProofProfile(mu.ChangePlan(), report)
			ledger := types.BuildVerificationProofLedger(mu.ChangePlan(), report, nil)
			retained := false
			for _, code := range profile.ReasonCodes {
				retained = retained || code == reason
			}
			if !retained || profile.Status != types.VerificationProofWeak || ledger.UncoveredCount == 0 || ledger.State == types.VerificationProofLedgerVerified {
				t.Fatalf("plain probe erased project assertion debt: profile=%+v ledger=%+v", profile, ledger)
			}
			projectExecuted, probePassed := false, false
			for _, command := range report.ExecutedCommands {
				projectExecuted = projectExecuted || (command.Runner == "make" && command.Outcome == types.ExecutedCommandOutcomeExecuted && command.ExitCode == 0)
			}
			for _, row := range report.TestResults {
				probePassed = probePassed || (row.Suite == "verification_probe/python" && row.AssertionID == "increment-contracts" && row.Passed)
			}
			if !projectExecuted || !probePassed {
				t.Fatalf("plain probe must not waive project tests or rewrite process success: %+v", report)
			}
		})
	}
}
