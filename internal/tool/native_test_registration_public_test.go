package tool

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

const nativeRegistrationTestBody = "import unittest\nfrom widget import increment\nclass Tests(unittest.TestCase):\n    def test_value(self): self.assertEqual(increment(2), 3)\n"

// Real emit/apply/read/native execution. Controller authorization and the
// successful model-request delivery commit are explicit protocol seams here;
// the orchestrator/agent suites cover those callers, not this tool fixture.
func nativeRegistrationPublicFixture(t *testing.T, body string) (*types.BusContext, *types.ChangePlan, types.VerificationDeliverySnapshot) {
	t.Helper()
	return nativeRegistrationPublicFixtureForTestPath(t, body, "test_widget.py")
}

func nativeRegistrationPublicFixtureForTestPath(t *testing.T, body, testPath string, requiredTests ...map[string]string) (*types.BusContext, *types.ChangePlan, types.VerificationDeliverySnapshot) {
	t.Helper()
	if !GitAvailable() {
		t.Skip("git unavailable")
	}
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("Python unavailable")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"widget.py": "def increment(value):\n    return value\n", testPath: body, ".gitignore": "__pycache__/\n*.pyc\n"}
	var constraints []types.WriteConstraint
	for _, tests := range requiredTests {
		for path, data := range tests {
			files[path] = data
			constraints = append(constraints, types.WriteConstraint{Kind: types.WriteConstraintRunExistingTest, Target: path})
		}
	}
	if dir := filepath.Dir(testPath); dir != "." {
		files[filepath.Join(dir, "__init__.py")] = ""
		// Keep a real stdlib discovery signal independent of the explicitly
		// selected nonconvention filename.
		files["test_anchor.py"] = "import unittest\nclass Anchor(unittest.TestCase):\n    def test_anchor(self): self.assertTrue(True)\n"
	}
	for path, data := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, path), []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	b1575FixtureGit(t, root, "init", "-q")
	b1575FixtureGit(t, root, "add", ".")
	b1575FixtureGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "-qm", "before")
	ctx := newTestBusCtx()
	ctx.RepoRoot, ctx.MainRepoRoot, ctx.Mode, ctx.PipelineStage = root, root, types.ModeApply, types.StagePlan
	ctx.WorkDir = t.TempDir()
	ctx.Mutable.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{
		Task:              types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopePackage, Summary: "increment adds one"},
		Constraints:       constraints,
		BehaviorContracts: []types.WriteBehaviorContract{{ID: "increment-result", Kind: types.WriteBehaviorObservable, Operator: types.WriteBehaviorOpEquals, Expected: "increment(2) == 3", Required: true}},
	}})
	payload := runTestsJSONParams(t, map[string]any{"request": "correct increment", "summary": "Correct the source while keeping existing tests unchanged.", "changes": []map[string]any{{"path": "widget.py", "kind": "modify", "new_content": "def increment(value):\n    return value + 1\n", "rationale": "return the correct next integer"}}})
	result, err := (&EmitChangePlan{}).Execute(ctx, payload)
	if err != nil || !result.Success {
		t.Fatalf("source emit: %+v %v", result, err)
	}
	source := ctx.Mutable.ChangePlan()
	ctx.PipelineStage = types.StageApply
	result, err = (&ApplyPatch{}).Execute(ctx, json.RawMessage(`{"path":"widget.py","kind":"modify"}`))
	if err != nil || !result.Success {
		t.Fatalf("source apply: %+v %v", result, err)
	}
	b1575FixtureGit(t, root, "add", "widget.py")
	b1575FixtureGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "-qm", "applied")
	head := strings.TrimSpace(b1575FixtureGit(t, root, "rev-parse", "HEAD"))
	diff, err := worktree.CaptureCommitPatch(root, head)
	if err != nil {
		t.Fatal(err)
	}
	effect := writeflow.PatchEffectRecordFromUnifiedDiff(source.ID, "", "applied_commit", head+"^", head, diff)
	source.PatchEffect, source.AppliedCommitSHA, source.WorktreePath, source.Status = &effect, head, root, types.PlanStatusApplied
	ctx.Mutable.SetChangePlan(source)
	delivery, ok := types.VerificationDeliverySnapshotFromAppliedPlan(source)
	if !ok {
		t.Fatal("real applied delivery unavailable")
	}
	ctx.PipelineStage = types.StagePlan
	ctx.Mutable.SetWriteWorkflowRun(&types.WriteWorkflowRun{RunID: "registration-run", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "proof-batch", Batches: []types.WriteWorkflowBatch{{ID: "proof-batch", Purpose: "verification_proof_followup", ExecutionMode: types.WriteWorkflowBatchExecutionVerifyOnly}}})
	if err := ctx.Mutable.AuthorizeNativeTestRegistration(root, delivery, source.BehaviorContracts, source.TargetPaths); err != nil {
		t.Fatal(err)
	}
	return ctx, source, delivery
}

func nativeRegistrationPublicRead(t *testing.T, ctx *types.BusContext, delivered bool, limit int) {
	t.Helper()
	nativeRegistrationPublicReadPath(t, ctx, "test_widget.py", delivered, limit)
}

func nativeRegistrationPublicReadPath(t *testing.T, ctx *types.BusContext, path string, delivered bool, limit int) {
	t.Helper()
	offset := 0
	if limit == 1 {
		offset = 1 // explicit partial page; inline-sized reads otherwise expand
	}
	result := dispatchReadVersionPublish(t, ctx, path, offset, limit)
	if !delivered {
		return
	}
	generation := ctx.Mutable.BeginDispatchRepositoryFileRead()
	callID := "actual-test-read-" + path
	ticket := ctx.Mutable.BindDispatchRepositoryReadMessage(generation, callID, result)
	if !ticket.MatchesMessage(callID, result.Summary) || !ctx.Mutable.RecordDispatchRepositoryReadDelivery(generation, []types.DispatchRepositoryReadMessageReceipt{ticket}) {
		t.Fatalf("actual read message delivery failed: result=%+v coverage=%+v", result, result.ReadCoverage)
	}
}

func nativeRegistrationPublicPayload() map[string]any {
	return map[string]any{"request": "verify the retained source without editing files", "summary": "Register the existing native assertion against the existing result contract, then rerun verification.", "changes": []any{}, "project_test_observations": []types.ProjectTestObservation{{ID: "existing-value", TestPath: "test_widget.py", AssertionSuite: "test_widget.Tests", AssertionID: "test_value", ContractRefs: []string{"increment-result"}}}}
}

func nativeRegistrationPublicEmit(t *testing.T, ctx *types.BusContext, entry string, payload map[string]any) types.ToolResult {
	t.Helper()
	data := runTestsJSONParams(t, payload)
	var result types.ToolResult
	var err error
	if entry == "skeleton" {
		result, err = (&EmitPlanSkeleton{}).Execute(ctx, data)
	} else {
		result, err = (&EmitChangePlan{}).Execute(ctx, data)
	}
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func nativeRegistrationPublicAuthorizeExecution(t *testing.T, ctx *types.BusContext, delivery types.VerificationDeliverySnapshot, contracts []types.WriteBehaviorContract) {
	t.Helper()
	plan := ctx.Mutable.ChangePlan()
	run := ctx.Mutable.WriteWorkflowRun()
	for i := range run.Batches {
		if run.Batches[i].ID == run.ActiveBatchID {
			run.Batches[i].PlanID = plan.ID
			run.Batches[i].Status = types.WriteWorkflowBatchVerifying
		}
	}
	ctx.Mutable.SetWriteWorkflowRun(run)
	if err := ctx.Mutable.AuthorizeNativeTestRegistrationExecution(plan, repositoryReadPhysicalIdentity(ctx.RepoRoot), delivery, contracts); err != nil {
		t.Fatal(err)
	}
	ctx.PipelineStage = types.StageVerify
}

func TestNativeTestRegistrationPublicFreshExecution(t *testing.T) {
	for _, entry := range []string{"full", "skeleton"} {
		t.Run(entry, func(t *testing.T) {
			ctx, source, delivery := nativeRegistrationPublicFixture(t, nativeRegistrationTestBody)
			sourceBytes, _ := json.Marshal(source)
			nativeRegistrationPublicRead(t, ctx, true, 100)
			if result := nativeRegistrationPublicEmit(t, ctx, entry, nativeRegistrationPublicPayload()); !result.Success {
				t.Fatalf("registration: %+v", result)
			}
			plan := ctx.Mutable.ChangePlan()
			if !types.IsPersistedNativeTestRegistrationPlan(plan) || len(plan.Changes) != 0 || len(types.RequiredExistingTestPaths(plan)) != 0 || ctx.Mutable.PartialChangePlan() != nil {
				t.Fatalf("registration shape: %+v", plan)
			}
			if ctx.Mutable.ChangeReport() != nil {
				t.Fatal("registration manufactured PASS")
			}
			nativeRegistrationPublicAuthorizeExecution(t, ctx, delivery, source.BehaviorContracts)
			var prior string
			for i := 0; i < 2; i++ {
				report := existingTestDeliveryPublicRun(t, ctx)
				if len(report.ExistingTestExecutions) != 1 {
					t.Fatalf("fresh receipt missing: %+v", report)
				}
				receipt := report.ExistingTestExecutions[0]
				invocation := report.ExecutedCommands[receipt.CommandIndex].InvocationID
				if receipt.NativeTestRegistrationDigest != types.NativeTestRegistrationDigest(plan) || receipt.SourcePlanID != source.ID || receipt.PlanID != plan.ID || invocation == "" || invocation == prior {
					t.Fatalf("receipt identity: %+v", receipt)
				}
				prior = invocation
				if len(types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, report.VerificationConfidence)) != 1 {
					t.Fatalf("new native assertion not accepted: %+v", report)
				}
			}
			afterSource, _ := json.Marshal(source)
			testBytes, _ := os.ReadFile(filepath.Join(ctx.RepoRoot, "test_widget.py"))
			if !bytes.Equal(sourceBytes, afterSource) || string(testBytes) != nativeRegistrationTestBody || strings.TrimSpace(b1575FixtureGit(t, ctx.RepoRoot, "diff", "HEAD", "--")) != "" {
				t.Fatal("registration/verification modified source delivery or tests")
			}
		})
	}
}

func TestNativeTestRegistrationPublicAdmissionGuards(t *testing.T) {
	for _, entry := range []string{"full", "skeleton"} {
		for _, condition := range []string{"not_read", "not_delivered", "partial_read", "bytes_after_read", "wrong_head", "no_grant", "foreign_root", "mixed_probes", "retirement", "unknown_contract", "unsupported_path"} {
			t.Run(entry+"/"+condition, func(t *testing.T) {
				ctx, _, _ := nativeRegistrationPublicFixture(t, nativeRegistrationTestBody)
				payload := nativeRegistrationPublicPayload()
				if condition != "not_read" {
					limit := 100
					if condition == "partial_read" {
						limit = 1
					}
					nativeRegistrationPublicRead(t, ctx, condition != "not_delivered", limit)
				}
				switch condition {
				case "bytes_after_read":
					if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "test_widget.py"), []byte(nativeRegistrationTestBody+"# changed\n"), 0644); err != nil {
						t.Fatal(err)
					}
				case "wrong_head":
					b1575FixtureGit(t, ctx.RepoRoot, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-qm", "new HEAD")
				case "no_grant":
					ctx.Mutable.RevokeNativeTestRegistrationAuthorization()
				case "foreign_root":
					ctx.RepoRoot = t.TempDir()
				case "mixed_probes":
					payload["verification_probes"] = []types.VerificationProbe{{ID: "probe", Language: "python", Code: "assert True"}}
				case "retirement":
					payload["superseded_contract_refs"] = []string{"increment-result"}
				case "unknown_contract":
					payload["project_test_observations"].([]types.ProjectTestObservation)[0].ContractRefs = []string{"new-contract"}
				case "unsupported_path":
					payload["project_test_observations"].([]types.ProjectTestObservation)[0].TestPath = "widget.py"
				}
				before, _ := json.Marshal(ctx.Mutable.ChangePlan())
				runBefore, _ := json.Marshal(ctx.Mutable.WriteWorkflowRun())
				result := nativeRegistrationPublicEmit(t, ctx, entry, payload)
				after, _ := json.Marshal(ctx.Mutable.ChangePlan())
				runAfter, _ := json.Marshal(ctx.Mutable.WriteWorkflowRun())
				if result.Success || !bytes.Equal(before, after) || !bytes.Equal(runBefore, runAfter) {
					t.Fatalf("rejected registration changed prior state: %+v", result)
				}
			})
		}
	}
}
