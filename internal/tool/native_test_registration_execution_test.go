package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNativeTestRegistrationPublicArbitraryFilenameThroughAlias(t *testing.T) {
	const target = "checks/arbitrary_cases.py"
	ctx, source, delivery := nativeRegistrationPublicFixtureForTestPath(t, nativeRegistrationTestBody, target)
	alias := filepath.Join(t.TempDir(), "repo-alias")
	if err := os.Symlink(ctx.RepoRoot, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	ctx.RepoRoot = alias
	nativeRegistrationPublicReadPath(t, ctx, target, true, 100)
	payload := nativeRegistrationPublicPayload()
	row := &payload["project_test_observations"].([]types.ProjectTestObservation)[0]
	row.TestPath, row.AssertionSuite = target, "checks.arbitrary_cases.Tests"
	if result := nativeRegistrationPublicEmit(t, ctx, "full", payload); !result.Success {
		t.Fatal(result.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	nativeRegistrationPublicAuthorizeExecution(t, ctx, delivery, source.BehaviorContracts)
	// Context seam negatives: the persisted root may not redirect an unrelated
	// current repository, and ordinary plans retain their path behavior.
	foreign := ctx.ShallowClone()
	foreign.RepoRoot = t.TempDir()
	if nativeRegistrationPhysicalExecutionContext(foreign) != foreign {
		t.Fatal("stored registration redirected a different current repository")
	}
	ordinary := ctx.ShallowClone()
	ordinary.Mutable = types.NewMutableState("ordinary plan")
	ordinary.Mutable.SetChangePlan(&types.ChangePlan{ID: "ordinary"})
	if nativeRegistrationPhysicalExecutionContext(ordinary) != ordinary {
		t.Fatal("ordinary plan acquired new path normalization")
	}
	report := existingTestDeliveryPublicRun(t, ctx)
	if ctx.RepoRoot != alias {
		t.Fatal("execution mutated caller's repository path")
	}
	if len(report.ExistingTestExecutions) != 1 || report.ExistingTestExecutions[0].TestPath != target || len(types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, report.VerificationConfidence)) != 1 {
		t.Fatalf("exact registered path or physical root lost: %+v", report)
	}
}

func TestNativeTestRegistrationPublicNoHistoricalOrUnlicensedProof(t *testing.T) {
	ctx, source, delivery := nativeRegistrationPublicFixture(t, nativeRegistrationTestBody)
	ctx.PipelineStage = types.StageVerify
	old := existingTestDeliveryPublicRun(t, ctx)
	if !old.Passed {
		t.Fatal("original test should actually pass")
	}
	oldBytes, _ := json.Marshal(old)
	ctx.PipelineStage = types.StagePlan
	nativeRegistrationPublicRead(t, ctx, true, 100)
	if result := nativeRegistrationPublicEmit(t, ctx, "full", nativeRegistrationPublicPayload()); !result.Success {
		t.Fatal(result.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if refs := types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, types.EffectiveVerificationConfidence(plan, old)); len(refs) != 0 {
		t.Fatalf("historical PASS proved a new registration: %v", refs)
	}
	ctx.PipelineStage = types.StageVerify
	unlicensed := existingTestDeliveryPublicRun(t, ctx)
	if len(unlicensed.ExistingTestExecutions) != 0 || len(types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, unlicensed.VerificationConfidence)) != 0 {
		t.Fatal("no runtime controller permission still minted registration proof")
	}
	nativeRegistrationPublicAuthorizeExecution(t, ctx, delivery, source.BehaviorContracts)
	fresh := existingTestDeliveryPublicRun(t, ctx)
	if len(fresh.ExistingTestExecutions) != 1 || len(types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, fresh.VerificationConfidence)) != 1 {
		t.Fatalf("new authorized execution unavailable: %+v", fresh.VerificationConfidence)
	}
	after, _ := json.Marshal(old)
	if string(oldBytes) != string(after) {
		t.Fatal("verification rewrote the old report")
	}
}

func TestNativeTestRegistrationPublicNativeVerdicts(t *testing.T) {
	for _, condition := range []string{"wrong_assertion", "wrong_suite", "failing_assertion", "skipped_assertion", "zero_tests", "test_changes_itself"} {
		t.Run(condition, func(t *testing.T) {
			body := nativeRegistrationTestBody
			switch condition {
			case "failing_assertion":
				body = strings.ReplaceAll(body, "increment(2), 3", "increment(2), 4")
			case "skipped_assertion":
				body = strings.ReplaceAll(body, "    def test_value", "    @unittest.skip('deliberately not observed')\n    def test_value")
			case "zero_tests":
				body = "import unittest\nclass Tests(unittest.TestCase):\n    pass\n"
			case "test_changes_itself":
				body = strings.ReplaceAll(body, "self.assertEqual(increment(2), 3)", "self.assertEqual(increment(2), 3); open(__file__, 'a').write('# changed during execution\\n')")
			}
			ctx, source, delivery := nativeRegistrationPublicFixture(t, body)
			nativeRegistrationPublicRead(t, ctx, true, 100)
			payload := nativeRegistrationPublicPayload()
			if condition == "wrong_assertion" {
				payload["project_test_observations"].([]types.ProjectTestObservation)[0].AssertionID = "test_missing"
			}
			if condition == "wrong_suite" {
				payload["project_test_observations"].([]types.ProjectTestObservation)[0].AssertionSuite = "another.Tests"
			}
			if result := nativeRegistrationPublicEmit(t, ctx, "full", payload); !result.Success {
				t.Fatalf("declaration was not admitted: %+v", result)
			}
			plan := ctx.Mutable.ChangePlan()
			nativeRegistrationPublicAuthorizeExecution(t, ctx, delivery, source.BehaviorContracts)
			report := existingTestDeliveryPublicRun(t, ctx)
			if len(types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, report.VerificationConfidence)) != 0 {
				t.Fatalf("unobserved assertion manufactured proof: %+v", report.VerificationConfidence)
			}
			if condition == "failing_assertion" {
				matches := projectTestObservationExecutionMatches(plan.ProjectTestObservations[0], report, false)
				strong := false
				for _, m := range matches {
					strong = strong || types.NativeTestRegistrationAssertionMatches(plan, report, plan.ProjectTestObservations[0], m.CommandIndex, m.ResultIndex)
				}
				if report.Passed || !strong {
					t.Fatalf("fresh actual failure lost its current identity: %+v", report)
				}
				if got := BuildVerifyFailureContractRelevance(report, plan); len(got.Hits) == 0 {
					t.Fatalf("fresh failure lost contract relevance: %+v", got)
				}
				for _, mutation := range []string{"missing_receipt", "wrong_digest"} {
					copy := *report
					copy.ExistingTestExecutions = append([]types.ExistingTestExecutionReceipt(nil), report.ExistingTestExecutions...)
					if mutation == "missing_receipt" {
						copy.ExistingTestExecutions = nil
					} else {
						copy.ExistingTestExecutions[0].NativeTestRegistrationDigest = strings.Repeat("0", 64)
					}
					if got := BuildVerifyFailureContractRelevance(&copy, plan); len(got.Hits) != 0 || copy.Passed {
						t.Fatalf("unbound failure signed contract relevance (%s): %+v", mutation, got)
					}
				}
			}
			if (condition == "zero_tests" || condition == "skipped_assertion" || condition == "test_changes_itself") && len(report.ExistingTestExecutions) != 0 {
				t.Fatalf("nonasserting or changed test minted receipt: %+v", report.ExistingTestExecutions)
			}
		})
	}
}

// Verify before the native process, after its report is read, and again at
// receipt publication. These tests execute Python; they do not invent rows.
func TestNativeTestRegistrationInvocationFreshness(t *testing.T) {
	for _, seam := range []string{"before_prepare", "before_read", "before_mint"} {
		for _, change := range []string{"contract_body", "source_head", "test_bytes", "authorization_revoked", "batch_changed"} {
			t.Run(seam+"/"+change, func(t *testing.T) {
				ctx, source, delivery := nativeRegistrationPublicFixture(t, nativeRegistrationTestBody)
				nativeRegistrationPublicRead(t, ctx, true, 100)
				if result := nativeRegistrationPublicEmit(t, ctx, "full", nativeRegistrationPublicPayload()); !result.Success {
					t.Fatal(result.Summary)
				}
				plan := ctx.Mutable.ChangePlan()
				nativeRegistrationPublicAuthorizeExecution(t, ctx, delivery, source.BehaviorContracts)
				mutate := func() {
					switch change {
					case "contract_body":
						plan.BehaviorContracts[0].Expected = "different expected value with the same ID"
					case "source_head":
						b1575FixtureGit(t, ctx.RepoRoot, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-qm", "different source HEAD")
					case "test_bytes":
						if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "test_widget.py"), []byte(nativeRegistrationTestBody+"# new bytes\n"), 0644); err != nil {
							t.Fatal(err)
						}
					case "authorization_revoked":
						ctx.Mutable.RevokeNativeTestRegistrationExecution()
					case "batch_changed":
						run := ctx.Mutable.WriteWorkflowRun()
						run.ActiveBatchID = "another-batch"
						ctx.Mutable.SetWriteWorkflowRun(run)
					}
				}
				invocation := runnerPlan{Runner: "python", Framework: "unittest", Root: ctx.RepoRoot, Suite: "test_widget.py"}
				if seam == "before_prepare" {
					mutate()
				}
				observed, command := prepareExistingTestUnittestInvocation(ctx, invocation)
				if seam == "before_prepare" {
					if observed != nil {
						observed.cleanup()
						t.Fatal("stale registration prepared observer")
					}
					return
				}
				if observed == nil {
					t.Fatal("fresh registration failed to prepare")
				}
				defer observed.cleanup()
				process := NewShellCommandContext(ctx.Context(), command)
				process.Dir = ctx.RepoRoot
				output, runErr := process.CombinedOutput()
				if runErr != nil {
					t.Fatalf("native process: %v %s", runErr, output)
				}
				if seam == "before_read" {
					mutate()
				}
				report, err := observed.readReport(ctx, 0, string(output), runErr)
				if seam == "before_read" {
					if report != nil || err == nil {
						t.Fatal("stale execution report accepted")
					}
					return
				}
				if err != nil || report == nil {
					t.Fatalf("fresh native report: %v", err)
				}
				mutate()
				commands := []types.ExecutedCommand{{Runner: "python", Framework: "unittest", WorkingDir: ".", Suite: "test_widget.py", Command: command, Outcome: types.ExecutedCommandOutcomeExecuted}}
				if receipts := existingTestExecutionReceipts(ctx, invocation, report, BuildTestSurface(ctx.RepoRoot, ""), commands, observed); len(receipts) != 0 {
					t.Fatalf("stale receipt minted: %+v", receipts)
				}
			})
		}
	}
}

func TestNativeTestRegistrationSelectsItsNativeProtocol(t *testing.T) {
	ctx, source, delivery := nativeRegistrationPublicFixture(t, nativeRegistrationTestBody)
	nativeRegistrationPublicRead(t, ctx, true, 100)
	if result := nativeRegistrationPublicEmit(t, ctx, "full", nativeRegistrationPublicPayload()); !result.Success {
		t.Fatal(result.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	nativeRegistrationPublicAuthorizeExecution(t, ctx, delivery, source.BehaviorContracts)
	// Inventory boundary: a supported stdlib candidate coexists with a more
	// highly ranked pytest candidate. No claim this fixture installs pytest.
	surface := BuildTestSurface(ctx.RepoRoot, "")
	surface.Candidates = append(surface.Candidates, types.TestSurfaceCandidate{ID: "pytest-sibling", Runner: "python", Framework: "pytest", WorkingDir: ".", HasTestSignal: true, Priority: 100})
	plans := impactRunnerPlansFromChangePlan(ctx.RepoRoot, surface, plan)
	found := false
	for _, selected := range plans {
		if selected.Suite == "test_widget.py" {
			found = true
			if selected.Runner != "python" || selected.Framework != "unittest" {
				t.Fatalf("registered execution protocol silently replaced: %+v", selected)
			}
		}
	}
	if !found {
		t.Fatal("registered exact file was not scheduled")
	}
}
