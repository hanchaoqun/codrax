package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Cross the real runner, serialization, and actual message boundaries. Display
// must neither repair a deliberately wrong PTO nor hide a mixed native failure.
func TestNativeIdentityPublicRunnerToActualAgentMessages(t *testing.T) {
	for _, tc := range []struct {
		name, assertion string
		mixedFailure    bool
	}{
		{name: "bound_pass", assertion: "test_increment"},
		{name: "unbound_pass", assertion: "test_other"},
		{name: "mixed_failure", assertion: "test_increment", mixedFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, result := nativeIdentityPublicRun(t, tc.assertion, tc.mixedFailure)
			plan, report := ctx.Mutable.ChangePlan(), ctx.Mutable.ChangeReport()
			wire := b1122JSON(t, report)
			var restored types.ChangeReport
			if err := json.Unmarshal(wire, &restored); err != nil {
				t.Fatal(err)
			}
			ctx.Mutable.SetChangeReport(&restored)
			zero, passed := false, false
			const suite = "python/unittest@packages/widget::tests.test_widget.IncrementTest"
			const id = "python/unittest@packages/widget::test_increment"
			for _, command := range restored.ExecutedCommands {
				zero = zero || (command.WorkingDir == "." && command.Outcome == types.ExecutedCommandOutcomeZeroTests)
			}
			for _, row := range restored.TestResults {
				passed = passed || (row.ObservationScope == types.TestObservationScopeAssertion && row.Suite == suite && row.AssertionID == id && row.Passed)
			}
			if !zero || !passed || restored.Passed == tc.mixedFailure {
				t.Fatalf("real producer prerequisites: root zero=%t native PASS=%t report=%s", zero, passed, wire)
			}
			if tc.assertion == "test_other" && (types.BehaviorContractRefHasVerificationWitness(plan, &restored, "increment") || types.BuildVerificationProofProfile(plan, &restored).Status != types.VerificationProofWeak) {
				t.Fatal("deliberately unbound identity unexpectedly became behavioral proof")
			}
			before := b1122JSON(t, []any{plan, &restored, types.BuildVerificationProofLedger(plan, &restored, nil)})
			agentCtx := &types.AgentContext{Mutable: ctx.Mutable, Mode: types.ModeApply, RepoRoot: ctx.RepoRoot}
			for outlet, message := range map[string]string{
				"run_tests":  result.Summary,
				"planner":    (&plannerEvaluator{}).BuildInitialInstruction(agentCtx, nil),
				"controller": (&writeControllerEvaluator{}).BuildInitialInstruction(agentCtx, nil),
			} {
				for _, want := range []string{`"assertion_suite":"` + suite + `"`, `"assertion_id":"` + id + `"`} {
					if !strings.Contains(message, want) {
						t.Errorf("%s lost exact producer identity %s", outlet, want)
					}
				}
			}
			if !bytes.Equal(before, b1122JSON(t, []any{plan, &restored, types.BuildVerificationProofLedger(plan, &restored, nil)})) {
				t.Fatal("identity display mutated plan, report, or behavioral proof")
			}
		})
	}
}

func TestNativeIdentityPublicGoSubtestPreservesImportPath(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("Go unavailable")
	}
	root := t.TempDir()
	files := map[string]string{
		"go.mod":          "module example.invalid/customer/mathbox\n\ngo 1.22\n",
		"mathbox.go":      "package mathbox\nfunc Increment(value int) int { return value + 1 }\n",
		"mathbox_test.go": "package mathbox\nimport \"testing\"\nfunc TestIncrement(t *testing.T) { t.Run(\"negative boundary\", func(t *testing.T) { if Increment(-1) != 0 { t.Fatal(\"increment\") } }) }\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mu := types.NewMutableState("verify increment")
	mu.SetChangePlan(&types.ChangePlan{ID: "native-go-plan", Status: types.PlanStatusApplied, TargetPaths: []string{"mathbox.go"}, AppliedPaths: []string{"mathbox.go"}, Changes: []types.FileChange{{Path: "mathbox.go", Kind: "modify", NewContent: files["mathbox.go"]}}})
	ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root, WorkDir: t.TempDir()}
	result, err := (&tool.RunTests{}).Execute(ctx, json.RawMessage(`{"runner":"go"}`))
	if err != nil || mu.ChangeReport() == nil || !mu.ChangeReport().Passed {
		t.Fatalf("real Go run failed: %v %+v report=%+v", err, result, mu.ChangeReport())
	}
	before := b1122JSON(t, []any{mu.ChangePlan(), mu.ChangeReport()})
	agentCtx := &types.AgentContext{Mutable: mu, Mode: types.ModeApply, RepoRoot: root}
	for outlet, message := range map[string]string{
		"run_tests":  result.Summary,
		"planner":    (&plannerEvaluator{}).BuildInitialInstruction(agentCtx, nil),
		"controller": (&writeControllerEvaluator{}).BuildInitialInstruction(agentCtx, nil),
	} {
		for _, want := range []string{`"assertion_suite":"example.invalid/customer/mathbox"`, `"assertion_id":"TestIncrement/negative_boundary"`} {
			if !strings.Contains(message, want) {
				t.Errorf("%s lost observed Go import path/subtest identity %s", outlet, want)
			}
		}
	}
	if !bytes.Equal(before, b1122JSON(t, []any{mu.ChangePlan(), mu.ChangeReport()})) || len(mu.ChangePlan().ProjectTestObservations) != 0 {
		t.Fatal("display fabricated a PTO or mutated the actual native receipt")
	}
}

func nativeIdentityPublicRun(t *testing.T, assertion string, mixedFailure bool) (*types.BusContext, types.ToolResult) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("Python unavailable")
	}
	root := t.TempDir()
	tests := "import unittest\nfrom widget import increment\nclass IncrementTest(unittest.TestCase):\n    def test_increment(self):\n        self.assertEqual(increment(-1), 0)\n        self.assertEqual(increment(0), 1)\n        self.assertEqual(increment(2), 3)\n"
	if mixedFailure {
		tests += "    def test_independent_failure(self):\n        self.fail('independent native failure must remain')\n"
	}
	files := map[string]string{
		".gitignore":                           "__pycache__/\n*.pyc\n",
		"packages/widget/setup.py":             "from setuptools import setup\nsetup(name='widget', version='0.0.0')\n",
		"packages/widget/widget.py":            "def increment(value):\n    return value\n",
		"packages/widget/tests/__init__.py":    "",
		"packages/widget/tests/test_widget.py": tests,
	}
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=Codrax Test", "-c", "user.email=codrax-test@example.invalid", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "-qm", "baseline"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixture git: %v: %s", err, out)
		}
	}
	ctx := &types.BusContext{Mutable: types.NewMutableState("fix increment"), RepoRoot: root, MainRepoRoot: root, WorkDir: t.TempDir(), Mode: types.ModeApply, PipelineStage: types.StagePlan}
	ctx.Mutable.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{
		Task:              types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopePackage, Summary: "increment input"},
		BehaviorContracts: []types.WriteBehaviorContract{{ID: "increment", Kind: types.WriteBehaviorObservable, Operator: types.WriteBehaviorOpEquals, Expected: "increment returns value plus one", Required: true}},
	}})
	for _, path := range []string{"packages/widget/widget.py", "packages/widget/tests/test_widget.py"} {
		res, err := (&tool.ReadFile{}).Execute(ctx, b1122JSON(t, map[string]any{"path": path}))
		if err != nil || !res.Success {
			t.Fatalf("read fixture: %v %+v", err, res)
		}
	}
	res, err := (&tool.EmitChangePlan{}).Execute(ctx, b1122JSON(t, map[string]any{
		"request": "fix increment", "summary": "repair source and use existing tests",
		"changes":                   []map[string]any{{"path": "packages/widget/widget.py", "kind": "modify", "new_content": "def increment(value):\n    return value + 1\n", "rationale": "increment the input"}},
		"project_test_observations": []map[string]any{{"id": "existing", "test_path": "packages/widget/tests/test_widget.py", "assertion_suite": "python/unittest@packages/widget::tests.test_widget.IncrementTest", "assertion_id": "python/unittest@packages/widget::" + assertion, "contract_refs": []string{"increment"}}},
	}))
	if err != nil || !res.Success {
		t.Fatalf("emit fixture: %v %+v", err, res)
	}
	ctx.PipelineStage = types.StageApply
	res, err = (&tool.ApplyPatch{}).Execute(ctx, b1122JSON(t, map[string]any{"path": "packages/widget/widget.py", "kind": "modify"}))
	if err != nil || !res.Success {
		t.Fatalf("apply fixture: %v %+v", err, res)
	}
	ctx.PipelineStage = types.StageVerify
	res, err = (&tool.RunTests{}).Execute(ctx, json.RawMessage(`{}`))
	if err != nil || ctx.Mutable.ChangeReport() == nil {
		t.Fatalf("run fixture: %v %+v", err, res)
	}
	return ctx, res
}
