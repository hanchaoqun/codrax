package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the public source-plan path with unchanged existing tests. The
// executor, not this fixture, creates Applied state and assertion receipts.
// Nested assertion identities must survive producer qualification without
// borrowing a sibling project's otherwise identical assertion.
func TestProjectTestObservationNestedPythonScopePublic(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git unavailable")
	}
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("Python unavailable")
	}
	for _, tc := range []struct {
		name, prefix  string
		sibling       bool
		assertionFail bool
		wantProof     bool
		wantFailure   bool
	}{
		{name: "root_positive", wantProof: true},
		{name: "nested_qualified_positive", prefix: "packages/widget", wantProof: true},
		{name: "nested_qualified_failure_relevance", prefix: "packages/widget", assertionFail: true, wantFailure: true},
		{name: "sibling_identity_must_not_cross", prefix: "packages/widget", sibling: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			const contractID = "increment-contract"
			const before = "def increment(value):\n    return value\n"
			after := "def increment(value):\n    return value + 1\n"
			if tc.assertionFail {
				after = "def increment(value):\n    return value + 2\n"
			}
			const testBody = "import unittest\nfrom widget import increment\n\nclass WidgetTest(unittest.TestCase):\n    def test_increment(self):\n        self.assertEqual(increment(2), 3)\n        self.assertEqual(increment(-1), 0)\n"
			prefixes := []string{tc.prefix}
			if tc.sibling {
				prefixes = append(prefixes, "packages/other")
			}
			write := func(path, body string) {
				t.Helper()
				full := filepath.Join(root, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			write(".gitignore", "__pycache__/\n*.pyc\n")
			var changes []map[string]any
			for _, prefix := range prefixes {
				sourcePath := filepath.ToSlash(filepath.Join(prefix, "widget.py"))
				write(sourcePath, before)
				write(filepath.Join(prefix, "tests/test_widget.py"), testBody)
				write(filepath.Join(prefix, "tests/__init__.py"), "")
				// setup.py exposes a nested project without declaring a pytest
				// configuration; unittest is selected from actual test source.
				write(filepath.Join(prefix, "setup.py"), "from setuptools import setup\nsetup(name='widget-fixture', version='0.0.0')\n")
				changes = append(changes, map[string]any{"path": sourcePath, "kind": "modify", "new_content": after, "rationale": "return the requested increment"})
			}
			b1575FixtureGit(t, root, "init", "-q")
			b1575FixtureGit(t, root, "add", ".")
			b1575FixtureGit(t, root, "-c", "user.name=Codrax Test", "-c", "user.email=codrax-test@example.invalid", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "-qm", "baseline")
			ctx := newTestBusCtx()
			ctx.RepoRoot, ctx.MainRepoRoot = root, root
			ctx.Mode, ctx.PipelineStage = types.ModeApply, types.StagePlan
			ctx.Mutable.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{
				Task:              types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopePackage, Summary: "increment returns input plus one"},
				BehaviorContracts: []types.WriteBehaviorContract{{ID: contractID, Kind: types.WriteBehaviorObservable, Operator: types.WriteBehaviorOpEquals, Expected: "increment(2) == 3 and increment(-1) == 0", Required: true}},
			}})
			testPath := filepath.ToSlash(filepath.Join(tc.prefix, "tests/test_widget.py"))
			for _, path := range []string{filepath.ToSlash(filepath.Join(tc.prefix, "widget.py")), testPath} {
				read, err := (&ReadFile{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"path": path}))
				if err != nil || !read.Success {
					t.Fatalf("actual file read prerequisite: %v %+v", err, read)
				}
			}
			suite, assertion := "tests.test_widget.WidgetTest", "test_increment"
			if tc.prefix != "" {
				owner := tc.prefix
				if tc.sibling {
					owner = "packages/other"
				}
				label := "python/unittest@" + owner + "::"
				suite, assertion = label+suite, label+assertion
			}
			params := runTestsJSONParams(t, map[string]any{
				"request": "fix increment", "summary": "Reuse the unchanged existing assertion with its complete runner identity.", "changes": changes,
				"project_test_observations": []map[string]any{{"id": "existing-increment", "test_path": testPath, "assertion_suite": suite, "assertion_id": assertion, "contract_refs": []string{contractID}}},
			})
			emitted, err := (&EmitChangePlan{}).Execute(ctx, params)
			if err != nil || !emitted.Success {
				t.Fatalf("actual emit prerequisite: %v %+v", err, emitted)
			}
			plan := ctx.Mutable.ChangePlan()
			if plan == nil || len(plan.Changes) != len(changes) || len(plan.ProjectTestObservations) != 1 || len(types.RequiredWriteBehaviorContractIDs(plan.BehaviorContracts, true)) != 1 {
				t.Fatalf("required source plan/PTO not retained: %+v", plan)
			}
			ctx.PipelineStage = types.StageApply
			for _, change := range changes {
				path := change["path"].(string)
				applied, err := (&ApplyPatch{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"path": path, "kind": "modify"}))
				if err != nil || !applied.Success || !ctx.Mutable.WriteClosure().HasApplied(path) {
					t.Fatalf("actual apply prerequisite: %v %+v", err, applied)
				}
			}
			ctx.PipelineStage = types.StageVerify
			result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{}))
			if err != nil {
				t.Fatalf("run_tests error: %v", err)
			}
			report := ctx.Mutable.ChangeReport()
			if report == nil || report.PlanID != plan.ID {
				t.Fatalf("actual report missing: %+v", result)
			}
			wire, _ := json.MarshalIndent(report, "", "  ")
			t.Logf("ACTUAL_PLAN_PTO=%+v\nACTUAL_REPORT=%s", plan.ProjectTestObservations[0], wire)
			commandObserved, assertionObserved := false, false
			workingDir := tc.prefix
			if workingDir == "" {
				workingDir = "."
			}
			for _, command := range report.ExecutedCommands {
				if command.Runner == "python" && command.Framework == pythonFrameworkUnittest &&
					command.WorkingDir == workingDir && command.Suite == "tests/test_widget.py" &&
					command.Outcome == types.ExecutedCommandOutcomeExecuted && (command.ExitCode != 0) == tc.assertionFail {
					commandObserved = true
				}
			}
			for _, row := range report.TestResults {
				if row.Kind == types.TestResultKindUnit && row.ObservationScope == types.TestObservationScopeAssertion && row.Passed != tc.assertionFail && row.AssertionID == assertion && row.Suite == suite {
					assertionObserved = true
				}
			}
			if !commandObserved || !assertionObserved {
				t.Fatalf("actual exact command/assertion prerequisite: command=%t assertion=%t", commandObserved, assertionObserved)
			}
			for _, prefix := range prefixes {
				body, err := os.ReadFile(filepath.Join(root, prefix, "tests/test_widget.py"))
				if err != nil || string(body) != testBody {
					t.Fatalf("existing test changed: %v", err)
				}
			}
			changed := strings.Fields(b1575FixtureGit(t, root, "diff", "--name-only"))
			if len(changed) != len(changes) {
				t.Fatalf("unexpected changed paths: %v", changed)
			}
			for _, path := range changed {
				if filepath.Base(path) != "widget.py" {
					t.Fatalf("verification altered non-source path: %s", path)
				}
			}
			satisfied := verificationConfidenceContains(report.VerificationConfidence, "project_test_contract_refs", "satisfied", "project_test_contract_ref_observed")
			missing := verificationConfidenceContains(report.VerificationConfidence, "project_test_contract_refs", "missing", "project_test_assertion_not_observed")
			_, covered := types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, report.VerificationConfidence)[contractID]
			relevance := BuildVerifyFailureContractRelevance(report, plan)
			failureBound := len(relevance.Hits) == 1 && relevance.Hits[0].ContractID == contractID && relevance.Hits[0].Reason == types.WriteBehaviorContractRetiredFailedProjectTestAssertion
			t.Logf("ACTUAL_PROOF satisfied=%t missing=%t covered=%t failureBound=%t resultSuccess=%t reportPassed=%t relevance=%+v", satisfied, missing, covered, failureBound, result.Success, report.Passed, relevance)
			if satisfied != tc.wantProof || covered != tc.wantProof || missing == tc.wantProof {
				t.Fatalf("exact passed assertion must discharge only its own source binding: wantProof=%t satisfied=%t missing=%t covered=%t", tc.wantProof, satisfied, missing, covered)
			}
			if failureBound != tc.wantFailure || (!tc.wantFailure && len(relevance.Hits) != 0) {
				t.Fatalf("failure relevance must share the exact execution/source binding: wantFailure=%t got=%+v", tc.wantFailure, relevance)
			}
			if report.Passed == tc.assertionFail {
				t.Fatalf("actual project-test outcome lost: assertionFail=%t report=%+v", tc.assertionFail, report)
			}
		})
	}
}
