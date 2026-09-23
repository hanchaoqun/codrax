package orchestrator

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// Exercise the real no-edit emitter, controller handoff, durable reload and
// native execution together. Only the prior apply is arranged as a real git
// commit; neither delivery snapshots nor successful reports are test-authored.
func TestVerificationDeliveryHandoff_EmitPersistRunTests(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 unavailable")
	}
	repo, output := t.TempDir(), t.TempDir()
	testBytes := []byte("import unittest\nfrom value import increment\nclass ValueTest(unittest.TestCase):\n    def test_increment(self): self.assertEqual(increment(4), 5)\n")
	for path, data := range map[string][]byte{"value.py": []byte("def increment(value):\n    return value\n"), "test_value.py": testBytes} {
		if err := os.WriteFile(filepath.Join(repo, path), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) string { return runGitForWorkflowRestoreTest(t, repo, args...) }
	git("init", "-q")
	git("add", "--all")
	git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "before")
	if err := os.WriteFile(filepath.Join(repo, "value.py"), []byte("def increment(value):\n    return value + 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", "value.py")
	git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "applied")
	head := strings.TrimSpace(git("rev-parse", "HEAD"))
	diff, err := worktree.CaptureCommitPatch(repo, head)
	if err != nil {
		t.Fatal(err)
	}
	source := &types.ChangePlan{ID: "applied-source", Status: types.PlanStatusVerifyFailed,
		AppliedCommitSHA: head, WorktreePath: repo, TargetPaths: []string{"value.py"}, Changes: []types.FileChange{{Path: "value.py", Kind: "patch"}}}
	effect := writeflow.PatchEffectRecordFromUnifiedDiff(source.ID, "", "applied_commit", head+"^", head, diff)
	source.PatchEffect = &effect
	mu := types.NewMutableState("verify the already applied change and run test_value.py")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{
		Constraints: []types.WriteConstraint{{Kind: types.WriteConstraintRunExistingTest, Target: "test_value.py"}}}})
	run := verificationDeliveryRun()
	run.ActiveBatchID, run.Status = "proof", types.WriteWorkflowRunInProgress
	run.Batches = append(run.Batches, types.WriteWorkflowBatch{ID: "proof", Purpose: "verification_proof_followup", Status: types.WriteWorkflowBatchReadyToPlan, ExpectedPaths: []string{"value.py"}})
	run.ProgressLedger = []types.WriteWorkflowProgress{{BatchID: "source-batch", ReasonCode: "verification_proof_followup_requested"}}
	mu.SetWriteWorkflowRun(run)
	mu.SetChangePlan(source)
	ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StagePlan, RepoRoot: repo, MainRepoRoot: repo, WorkDir: output}
	o := &Orchestrator{busCtx: ctx}
	result, err := (&tool.EmitChangePlan{}).Execute(ctx, json.RawMessage(`{
		"summary":"Recheck the existing applied implementation without editing any source or tests", "changes":[],
		"verification_probes":[{"id":"increment-call", "language":"python", "code":"from value import increment\nassert increment(4) == 5\n", "changed_symbol_refs":["path:value.py"]}]
	}`))
	if err != nil || !result.Success {
		t.Fatalf("public proof-only emitter: %v %+v", err, result)
	}
	if err := planPostHook(o, nil); err != nil {
		t.Fatalf("real planner completion hook: %v", err)
	}
	plan := mu.ChangePlan()
	o.stampCumulativeVerificationScope(plan, run, source)
	if plan.AppliedCommitSHA != "" || plan.PatchEffect != nil || len(plan.ProjectTestObservations) != 0 {
		t.Fatal("controller fabricated application or native declarations on proof-only plan")
	}
	path := filepath.Join(output, "proof.json")
	if err := types.WritePlanToFile(plan, path); err != nil {
		t.Fatal(err)
	}
	loaded, err := types.LoadChangePlanFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mu.SetChangePlan(loaded)
	ctx.PipelineStage = types.StageVerify
	result, err = (&tool.RunTests{}).Execute(ctx, json.RawMessage(`{}`))
	if err != nil || mu.ChangeReport() == nil {
		t.Fatalf("public verification: %v %+v", err, result)
	}
	report := mu.ChangeReport()
	confidence := types.ExistingTestExecutionConfidence(loaded, report)
	if len(confidence) != 1 || confidence[0].Status != "satisfied" || len(report.ExistingTestExecutions) != 1 {
		t.Fatalf("fresh native execution lost after public handoff: %+v receipts=%+v", confidence, report.ExistingTestExecutions)
	}
	receipt := report.ExistingTestExecutions[0]
	if receipt.PlanID != loaded.ID || receipt.SourcePlanID != source.ID || receipt.AppliedCommitSHA != head {
		t.Fatalf("execution borrowed or relabelled source ownership: %+v", receipt)
	}
	if got := types.ResolveVerificationProbeTargetExecution(loaded, loaded.VerificationProbes[0], report); len(got.Paths) != 1 || got.Paths[0] != "value.py" {
		t.Fatalf("neighbor target execution did not use controller identity: %+v", got)
	}
	// Verification completion writes lifecycle metadata even though this plan
	// did not apply source. Exercise that real transition before the next run.
	o.syncMutablePlanStatusAfterVerify(report, nil)
	if loaded.AppliedAt == nil || loaded.AppliedCommitSHA != "" || loaded.PatchEffect != nil {
		t.Fatal("verify lifecycle did not retain timestamp without an own source application")
	}
	if err := types.WritePlanToFile(loaded, path); err != nil {
		t.Fatal(err)
	}
	loaded, err = types.LoadChangePlanFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mu.SetChangePlan(loaded)
	result, err = (&tool.RunTests{}).Execute(ctx, json.RawMessage(`{}`))
	if err != nil || mu.ChangeReport() == nil {
		t.Fatalf("public verification after lifecycle restore: %v %+v", err, result)
	}
	nextReport := mu.ChangeReport()
	if next := types.ExistingTestExecutionConfidence(loaded, nextReport); len(next) != 1 || next[0].Status != "satisfied" || len(nextReport.ExistingTestExecutions) != 1 {
		t.Fatalf("lifecycle restore lost fresh native execution: %+v receipts=%+v", next, nextReport.ExistingTestExecutions)
	}
	if nextReport.ExecutedCommands[nextReport.ExistingTestExecutions[0].CommandIndex].InvocationID == report.ExecutedCommands[receipt.CommandIndex].InvocationID {
		t.Fatal("lifecycle restore reused the old test execution")
	}
	currentTest, _ := os.ReadFile(filepath.Join(repo, "test_value.py"))
	if string(currentTest) != string(testBytes) || strings.TrimSpace(git("rev-parse", "HEAD")) != head || git("diff", "HEAD", "--") != "" {
		t.Fatal("verification changed the existing delivery or native test")
	}
}
