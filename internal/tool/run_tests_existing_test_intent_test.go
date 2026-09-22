package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestExistingTestIntentRequiresCurrentExactRead(t *testing.T) {
	for _, tc := range []string{"current", "missing", "prior_dispatch", "other_repository", "too_many", "unsupported_file"} {
		t.Run(tc, func(t *testing.T) {
			path := "checks/arbitrary_cases.py"
			if tc == "unsupported_file" {
				path = "fixtures/input.txt"
			}
			ctx := protectedReadBus(t, path)
			if tc != "missing" {
				readCtx := ctx
				if tc == "other_repository" {
					readCtx = protectedReadBus(t, path)
				}
				result := protectedReadResult(t, readCtx, path)
				if tc != "prior_dispatch" {
					ctx.Mutable.AppendDispatchToolResult(result)
				}
			}
			constraints := []map[string]string{{"kind": types.WriteConstraintRunExistingTest, "target": path}}
			if tc == "too_many" {
				for len(constraints) <= types.MaxRequiredExistingTests {
					constraints = append(constraints, constraints[0])
				}
			}
			params, _ := json.Marshal(map[string]any{"task": map[string]string{"kind": "bugfix", "scope": "micro", "summary": "execute an existing test"}, "risk": map[string]string{"overall": "low"}, "constraints": constraints})
			result, err := (&EmitWriteAnalysis{}).Execute(ctx, params)
			want := tc == "current" || tc == "unsupported_file"
			if err != nil || result.Success != want {
				t.Fatalf("admission %s: result=%+v err=%v", tc, result, err)
			}
			if want && ctx.Mutable.WriteAnalysisIR().Request.Constraints[0].Target != path {
				t.Fatal("exact intent changed")
			}
			if ctx.Mutable.ChangeReport() != nil {
				t.Fatal("read/admission minted execution")
			}
		})
	}
}

func TestExistingTestCurrentDeliveryAndFreshNativeInvocation(t *testing.T) {
	for _, tc := range []string{"current_twice", "wrong_applied_sha", "wrong_patch_fingerprint", "wrong_head_ref", "tracked_source_dirty", "test_file_dirty", "different_head"} {
		t.Run(tc, func(t *testing.T) {
			root := t.TempDir()
			for file, body := range map[string]string{"widget.py": "def increment(x):\n    return x + 1\n", "test_widget.py": "import unittest\nfrom widget import increment\nclass Tests(unittest.TestCase):\n    def test_value(self): self.assertEqual(increment(1), 2)\n"} {
				if err := os.WriteFile(filepath.Join(root, file), []byte(body), 0644); err != nil {
					t.Fatal(err)
				}
			}
			ctx := &types.BusContext{RepoRoot: root, MainRepoRoot: root, Mode: types.ModeApply, PipelineStage: types.StageVerify, Mutable: types.NewMutableState("execute test")}
			plan := &types.ChangePlan{ID: "required-existing-test", WriteAnalysisIR: &types.WriteAnalysisIR{Request: types.WriteRequestModel{Constraints: []types.WriteConstraint{{Kind: types.WriteConstraintRunExistingTest, Target: "test_widget.py"}}}}}
			b1575BindAppliedPythonLines(t, ctx, plan, "widget.py", []int{2})
			switch tc {
			case "wrong_applied_sha":
				plan.AppliedCommitSHA = strings.Repeat("1", 40)
			case "wrong_patch_fingerprint":
				plan.PatchEffect.DiffFingerprint = strings.Repeat("1", 64)
			case "wrong_head_ref":
				plan.PatchEffect.HeadRef = "HEAD^"
			case "tracked_source_dirty":
				_ = os.WriteFile(filepath.Join(root, "widget.py"), []byte("def increment(x): return x\n"), 0644)
			case "test_file_dirty":
				_ = os.WriteFile(filepath.Join(root, "test_widget.py"), []byte("import unittest\n"), 0644)
			case "different_head":
				b1575FixtureGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-qm", "different delivery")
			}
			ctx.Mutable.SetChangePlan(plan)
			invocation := runnerPlan{Runner: "python", Framework: "unittest", Root: root, Suite: "test_widget.py"}
			run, command := prepareExistingTestUnittestInvocation(ctx, invocation)
			if tc != "current_twice" {
				if run != nil {
					run.cleanup()
					t.Fatal("stale or modified delivery admitted")
				}
				return
			}
			if run == nil {
				t.Fatal("actual applied delivery rejected")
			}
			defer run.cleanup()
			firstCommand := command
			for i := 0; i < 2; i++ {
				if i == 1 {
					run, command = prepareExistingTestUnittestInvocation(ctx, invocation)
					if run == nil {
						t.Fatal("second invocation missing")
					}
					defer run.cleanup()
					if command == firstCommand {
						t.Fatal("native invocation reused prior artifact path")
					}
				}
				cmd := NewShellCommandContext(ctx.Context(), command)
				cmd.Dir = root
				out, runErr := cmd.CombinedOutput()
				if runErr != nil {
					t.Fatalf("native unittest failed: %v %s", runErr, out)
				}
				report, err := run.readReport(ctx, 0, string(out), runErr)
				if err != nil || report == nil || len(report.TestResults) != 1 {
					t.Fatalf("fresh native report: %+v %v", report, err)
				}
				commands := []types.ExecutedCommand{{Runner: "python", Framework: "unittest", WorkingDir: ".", Suite: "test_widget.py", Command: command, Outcome: types.ExecutedCommandOutcomeExecuted}}
				surface := BuildTestSurface(root, "")
				receipts := existingTestExecutionReceipts(ctx, invocation, report, surface, commands, run)
				if len(receipts) != 1 || receipts[0].AssertionCount != 1 {
					t.Fatalf("actual native receipt missing: %+v", receipts)
				}
			}
		})
	}
}
