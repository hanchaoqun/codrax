package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This is the ordinary source-plan lane: the existing test is verification
// input, not a same-byte change used to get it into the plan. All execution
// receipts below come from the public tools, not hand-built ChangeReports.
func TestProjectTestObservationExistingFilePublicSourcePlan(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git is unavailable")
	}
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable Python on PATH")
	}
	for _, tc := range []struct {
		name, assertionID, expression string
		skipped, wantProof, skeleton  bool
	}{
		{name: "exact_assertion", assertionID: "test_increment", expression: "value + 1", wantProof: true},
		{name: "skeleton_finalize_exact_assertion", assertionID: "test_increment", expression: "value + 1", wantProof: true, skeleton: true},
		{name: "wrong_assertion_id_despite_suite_pass", assertionID: "test_missing", expression: "value + 1"},
		{name: "real_assertion_failure", assertionID: "test_increment", expression: "value + 2"},
		{name: "skipped_assertion_despite_suite_pass", assertionID: "test_increment", expression: "value + 1", skipped: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			const sourcePath = "widget.py"
			const testPath = "tests/test_widget.py"
			const contractID = "increment-contract"
			beforeSource := "def increment(value):\n    return value\n"
			afterSource := "def increment(value):\n    return " + tc.expression + "\n"
			decorator := ""
			if tc.skipped {
				decorator = "    @unittest.skip('fixture: assertion deliberately not executed')\n"
			}
			testBody := "import unittest\nfrom widget import increment\n\nclass WidgetTest(unittest.TestCase):\n" + decorator +
				"    def test_increment(self):\n        self.assertEqual(increment(2), 3)\n        self.assertEqual(increment(-1), 0)\n"
			for path, body := range map[string]string{
				sourcePath: beforeSource, testPath: testBody, "tests/__init__.py": "", ".gitignore": "__pycache__/\n*.pyc\n",
			} {
				fullPath := filepath.Join(root, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fullPath, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			b1575FixtureGit(t, root, "init", "-q")
			b1575FixtureGit(t, root, "add", ".")
			b1575FixtureGit(t, root, "-c", "user.name=Codrax Test", "-c", "user.email=codrax-test@example.invalid",
				"-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "-qm", "baseline")

			ctx := newTestBusCtx()
			ctx.RepoRoot, ctx.MainRepoRoot = root, root
			ctx.Mode, ctx.PipelineStage = types.ModeApply, types.StagePlan
			ctx.Mutable.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{
				Task: types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopePackage, Summary: "increment returns the input plus one"},
				BehaviorContracts: []types.WriteBehaviorContract{{
					ID: contractID, Kind: types.WriteBehaviorObservable, Operator: types.WriteBehaviorOpEquals,
					Expected: "increment(2) == 3 and increment(-1) == 0", Required: true,
				}},
			}})
			change := map[string]any{
				"path": sourcePath, "kind": "modify", "new_content": afterSource, "rationale": "correct the increment result",
			}
			if tc.skeleton {
				delete(change, "new_content")
			}
			params, err := json.Marshal(map[string]any{
				"request": "fix increment to return the input plus one",
				"summary": "Change the source and reuse the existing source-dependent project assertion.",
				"changes": []map[string]any{change},
				"project_test_observations": []map[string]any{{
					"id": "existing-increment-test", "test_path": testPath,
					"assertion_suite": "WidgetTest", "assertion_id": tc.assertionID, "contract_refs": []string{contractID},
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			var emitted types.ToolResult
			if tc.skeleton {
				emitted, err = (&EmitPlanSkeleton{}).Execute(ctx, params)
				if err != nil || !emitted.Success {
					t.Fatalf("source-only skeleton must accept the existing unchanged test: err=%v result=%+v", err, emitted)
				}
				partial := ctx.Mutable.PartialChangePlan()
				if ctx.Mutable.ChangePlan() != nil || partial == nil || len(partial.Changes) != 1 ||
					partial.Changes[0].Path != sourcePath || len(partial.ProjectTestObservations) != 1 ||
					partial.ProjectTestObservations[0].TestPath != testPath {
					t.Fatalf("skeleton must retain source-only changes and the separate existing-test observation: %+v", partial)
				}
				body, marshalErr := json.Marshal(map[string]any{"path": sourcePath, "new_content": afterSource})
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				emitted, err = (&EmitPlanChange{}).Execute(ctx, body)
				if err == nil && emitted.Success && ctx.Mutable.PartialChangePlan() != nil {
					t.Fatal("final source body did not retire the partial plan")
				}
			} else {
				emitted, err = (&EmitChangePlan{}).Execute(ctx, params)
			}
			if err != nil || !emitted.Success {
				t.Fatalf("source-only plan must accept the existing unchanged test: err=%v result=%+v", err, emitted)
			}
			plan := ctx.Mutable.ChangePlan()
			if plan == nil || len(plan.Changes) != 1 || plan.Changes[0].Path != sourcePath ||
				len(plan.TargetPaths) != 1 || plan.TargetPaths[0] != sourcePath || len(plan.ProjectTestObservations) != 1 {
				t.Fatalf("emitted plan must contain only the source change plus separate PTO: %+v", plan)
			}
			observation := plan.ProjectTestObservations[0]
			if observation.TestPath != testPath || observation.AssertionSuite != "WidgetTest" || observation.AssertionID != tc.assertionID ||
				len(observation.ContractRefs) != 1 || observation.ContractRefs[0] != contractID {
				t.Fatalf("emission lost exact existing-test binding: %+v", observation)
			}
			if diff := b1575FixtureGit(t, root, "diff", "--name-only"); diff != "" {
				t.Fatalf("emission modified the fixture: %q", diff)
			}

			// Invoke the real apply tool; do not install an Applied plan or invent
			// an applied-path/commit receipt. This covers tool wiring, not the
			// orchestrator's separate approval and worktree lifecycle.
			ctx.PipelineStage = types.StageApply
			applied, err := (&ApplyPatch{}).Execute(ctx, json.RawMessage(`{"path":"widget.py","kind":"modify"}`))
			if err != nil || !applied.Success || !ctx.Mutable.WriteClosure().HasApplied(sourcePath) {
				t.Fatalf("public source apply failed: err=%v result=%+v", err, applied)
			}
			if got, err := os.ReadFile(filepath.Join(root, sourcePath)); err != nil || string(got) != afterSource {
				t.Fatalf("apply did not write the emitted source: err=%v got=%q", err, got)
			}

			ctx.PipelineStage = types.StageVerify
			result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{}))
			if err != nil {
				t.Fatalf("public RunTests: %v", err)
			}
			report := ctx.Mutable.ChangeReport()
			if report == nil || report.PlanID != plan.ID {
				t.Fatalf("missing actual plan-bound report: result=%+v report=%+v", result, report)
			}
			if tc.wantProof && (!result.Success || !report.Passed) {
				t.Fatalf("fixed source with existing exact assertion must pass: result=%+v report=%+v", result, report)
			}
			wantCommandPass := tc.name != "real_assertion_failure"
			foundCommand := false
			for _, command := range report.ExecutedCommands {
				if command.Runner != "python" || command.Framework != pythonFrameworkUnittest ||
					command.Outcome != types.ExecutedCommandOutcomeExecuted || command.Suite != testPath {
					continue
				}
				foundCommand = true
				if (command.ExitCode == 0) != wantCommandPass || strings.Contains(command.Command, "discover -s") {
					t.Fatalf("expected real exact-file command outcome (pass=%t): %+v", wantCommandPass, command)
				}
			}
			if !foundCommand {
				t.Fatalf("no real exact-file Python execution: %+v", report.ExecutedCommands)
			}
			foundAssertion := false
			for _, row := range report.TestResults {
				if row.AssertionID != "test_increment" || !strings.HasSuffix(row.Suite, "WidgetTest") {
					continue
				}
				foundAssertion = true
				wantScope := types.TestObservationScopeAssertion
				if tc.skipped {
					wantScope = types.TestObservationScopeNonAsserting
				}
				if row.Kind != types.TestResultKindUnit || row.Passed != wantCommandPass || row.ObservationScope != wantScope {
					t.Fatalf("real assertion outcome/scope differs: %+v", row)
				}
			}
			if !foundAssertion {
				t.Fatalf("real source-dependent assertion was not reported: %+v", report.TestResults)
			}

			// Consume the report installed by RunTests without filling in or
			// recomputing Passed, TestResults, or VerificationConfidence.
			satisfied := verificationConfidenceContains(report.VerificationConfidence, "project_test_contract_refs", "satisfied", "project_test_contract_ref_observed")
			missing := verificationConfidenceContains(report.VerificationConfidence, "project_test_contract_refs", "missing", "project_test_assertion_not_observed")
			_, covered := types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, report.VerificationConfidence)[contractID]
			ledger := types.BuildVerificationProofLedger(plan, report, nil)
			ledgerCovered := false
			for _, obligation := range ledger.Obligations {
				if obligation.ContractRef == contractID && obligation.Status == types.VerificationProofLedgerItemCovered {
					ledgerCovered = true
				}
			}
			if satisfied != tc.wantProof || missing == tc.wantProof || covered != tc.wantProof || ledgerCovered != tc.wantProof {
				t.Fatalf("aggregate suite outcome must not replace exact assertion proof: wantProof=%t confidence=%+v ledger=%+v", tc.wantProof, report.VerificationConfidence, ledger)
			}
			if got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(testPath))); err != nil || string(got) != testBody {
				t.Fatalf("existing test bytes changed: err=%v got=%q", err, got)
			}
			if diff := strings.TrimSpace(b1575FixtureGit(t, root, "diff", "--name-only")); diff != sourcePath {
				t.Fatalf("only source may change across emit/apply/verify; got %q", diff)
			}
			t.Logf("public emit -> apply -> real unittest: source-only changes; unchanged existing test; command_pass=%t exact_contract_proof=%t", wantCommandPass, satisfied)
		})
	}
}
