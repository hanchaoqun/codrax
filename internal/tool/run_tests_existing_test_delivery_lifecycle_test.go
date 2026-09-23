package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Execute the actual native observer, then change one authority input at the
// read or mint seam. These are invocation tests, not entire orchestrator runs.
func TestExistingTestDeliveryInvocationLifecycle(t *testing.T) {
	for _, borrowed := range []bool{false, true} {
		for _, seam := range []string{"before_read", "before_mint"} {
			for _, change := range []string{"none", "current_plan", "effect_record", "nested_effect", "head", "source_bytes", "test_bytes"} {
				name := "own/"
				if borrowed {
					name = "borrowed/"
				}
				t.Run(name+seam+"/"+change, func(t *testing.T) {
					ctx, source, plan := existingTestDeliveryPublicFixture(t)
					if !borrowed {
						source.WriteAnalysisIR = plan.WriteAnalysisIR
						plan = source
						ctx.Mutable.SetChangePlan(plan)
					}
					invocation := runnerPlan{Runner: "python", Framework: "unittest", Root: ctx.RepoRoot, Suite: "test_widget.py"}
					run, command := prepareExistingTestUnittestInvocation(ctx, invocation)
					if run == nil {
						t.Fatal("valid real delivery did not prepare")
					}
					defer run.cleanup()
					cmd := NewShellCommandContext(ctx.Context(), command)
					cmd.Dir = ctx.RepoRoot
					output, runErr := cmd.CombinedOutput()
					if runErr != nil {
						t.Fatalf("original native run failed: %v %s", runErr, output)
					}
					mutate := func() {
						effect := plan.PatchEffect
						if borrowed {
							effect = plan.CumulativeVerificationScope.AppliedSources[0].PatchEffect
						}
						switch change {
						case "current_plan":
							plan.ID = "different-current-plan"
							if !borrowed {
								effect.PlanID = plan.ID
							}
						case "effect_record":
							effect.RecordID += "-changed"
						case "nested_effect":
							effect.Files[0].Hunks[0].AddedLineTexts[0].Text += " # changed metadata"
						case "head":
							b1575FixtureGit(t, ctx.RepoRoot, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-qm", "later head")
						case "source_bytes", "test_bytes":
							path := "widget.py"
							if change == "test_bytes" {
								path = "test_widget.py"
							}
							data, _ := os.ReadFile(filepath.Join(ctx.RepoRoot, path))
							if err := os.WriteFile(filepath.Join(ctx.RepoRoot, path), append(data, []byte("# later bytes\n")...), 0644); err != nil {
								t.Fatal(err)
							}
						}
						ctx.Mutable.SetChangePlan(plan)
					}
					if seam == "before_read" {
						mutate()
					}
					report, err := run.readReport(ctx, 0, string(output), runErr)
					if seam == "before_read" && change != "none" {
						if err == nil || report != nil {
							t.Fatal("changed execution identity accepted at read")
						}
						return
					}
					if err != nil || report == nil {
						t.Fatalf("fresh native read failed: %v", err)
					}
					if seam == "before_mint" {
						mutate()
					}
					commands := []types.ExecutedCommand{{Runner: "python", Framework: "unittest", WorkingDir: ".", Suite: "test_widget.py", Command: command, Outcome: types.ExecutedCommandOutcomeExecuted}}
					receipts := existingTestExecutionReceipts(ctx, invocation, report, BuildTestSurface(ctx.RepoRoot, ""), commands, run)
					if (len(receipts) == 1) != (change == "none") {
						t.Fatalf("mint changed=%s receipts=%+v", change, receipts)
					}
				})
			}
		}
	}
}

func TestExistingTestDeliveryPublicProbeChangesHead(t *testing.T) {
	ctx, _, plan := existingTestDeliveryPublicFixture(t)
	// Moving only HEAD after actually executing the target leaves its current
	// source bytes unchanged. The final physical check must still reject it.
	base := b1575FixtureGit(t, ctx.RepoRoot, "rev-parse", "HEAD^")
	literal, _ := json.Marshal(base)
	plan.VerificationProbes[0].Code += "from pathlib import Path\nPath('.git/HEAD').write_text(" + string(literal) + ")\n"
	ctx.Mutable.SetChangePlan(plan)
	report := existingTestDeliveryPublicRun(t, ctx)
	passed := false
	for _, row := range report.TestResults {
		if row.Suite == "verification_probe/python" && row.AssertionID == "target-probe" {
			passed = row.Passed
		}
	}
	if !passed {
		t.Fatal("identity observation changed original successful probe verdict")
	}
	if got := types.ResolveVerificationProbeTargetExecution(plan, plan.VerificationProbes[0], report); len(got.Paths) != 0 {
		t.Fatalf("changed HEAD granted execution: %+v", got)
	}
	if len(report.ExistingTestExecutions) != 0 {
		t.Fatal("changed HEAD granted subsequent native receipt")
	}
	for _, command := range report.ExecutedCommands {
		if command.ProbeExecution != nil && command.ProbeExecution.TargetExecution != nil && command.ProbeExecution.TargetExecution.ReasonCode != "applied_source_changed" {
			t.Errorf("wrong physical freshness diagnostic: %+v", command.ProbeExecution.TargetExecution)
		}
	}
}

func TestExistingTestDeliveryPublicOwnProbeLegacy(t *testing.T) {
	for _, sha := range []string{"", "legacy-unrelated-apply-field"} {
		t.Run("applied_sha="+sha, func(t *testing.T) {
			ctx, source, followup := existingTestDeliveryPublicFixture(t)
			source.AppliedCommitSHA = sha
			source.VerificationProbes = followup.VerificationProbes
			ctx.Mutable.SetChangePlan(source)
			report := existingTestDeliveryPublicRun(t, ctx)
			if got := types.ResolveVerificationProbeTargetExecution(source, source.VerificationProbes[0], report); len(got.Paths) != 1 {
				t.Fatalf("own effect-based probe protocol regressed: %+v; receipts=%s", got, b1575TargetReceiptsJSON(report))
			}
		})
	}
}

func TestExistingTestDeliveryPublicNativeVerdicts(t *testing.T) {
	for _, skipped := range []bool{false, true} {
		name := "failed"
		body := "import unittest\nclass Tests(unittest.TestCase):\n    def test_value(self): self.fail('original-native-failure')\n"
		if skipped {
			name = "skipped"
			body = "import unittest\n@unittest.skip('original-native-skip')\nclass Tests(unittest.TestCase):\n    def test_value(self): self.fail('not run')\n"
		}
		t.Run(name, func(t *testing.T) {
			ctx, _, plan := existingTestDeliveryPublicFixtureWithTest(t, body)
			report := existingTestDeliveryPublicRun(t, ctx)
			confidence := types.ExistingTestExecutionConfidence(plan, report)
			if len(confidence) != 1 {
				t.Fatalf("missing independent native intent: %+v", confidence)
			}
			if skipped {
				if len(report.ExistingTestExecutions) != 0 || confidence[0].Status == "satisfied" || report.Passed {
					t.Fatalf("all-skip signed execution: %+v", report)
				}
			} else {
				if len(report.ExistingTestExecutions) != 1 || report.ExistingTestExecutions[0].FailedAssertionCount != 1 || confidence[0].Status != "failed" || report.Passed || report.FailureKind != types.FailureKindTestsFailed {
					t.Fatalf("new binding concealed actual native failure: %+v", report)
				}
				found := false
				for _, row := range report.TestResults {
					found = found || strings.Contains(row.FailureDetail, "original-native-failure")
				}
				if !found {
					t.Fatal("native failure detail lost")
				}
			}
		})
	}
}

func TestExistingTestDeliveryPublicModelCannotAuthorSource(t *testing.T) {
	for _, field := range []string{"cumulative_verification_scope", "applied_sources"} {
		t.Run(field, func(t *testing.T) {
			ctx, _, plan := existingTestDeliveryPublicFixture(t)
			before, _ := json.Marshal(plan)
			params, _ := json.Marshal(map[string]any{"request": "verify existing code", "summary": "readonly verification of retained delivery", "changes": []any{}, field: plan.CumulativeVerificationScope})
			result, err := (&EmitChangePlan{}).Execute(ctx, params)
			if err == nil || result.Success || !strings.Contains(result.Summary, "unknown field") {
				t.Fatalf("model authored controller delivery: %+v %v", result, err)
			}
			after, _ := json.Marshal(ctx.Mutable.ChangePlan())
			if string(before) != string(after) {
				t.Fatal("rejected model field changed active plan")
			}
		})
	}
}
