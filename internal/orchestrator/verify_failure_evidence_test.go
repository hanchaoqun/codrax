package orchestrator

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// A failed verify attempt must leave durable evidence on disk before any
// cleanup can run: the typed report JSON plus the applied-commit patch, with
// the diff artifact ref attached to the batch's verify attempt record.
func TestPersistVerifyFailureEvidence_WritesReportAndDiff(t *testing.T) {
	mainRoot := filepath.Join(t.TempDir(), "main")
	if err := os.MkdirAll(mainRoot, 0o755); err != nil {
		t.Fatalf("mkdir main: %v", err)
	}
	git := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = mainRoot
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("config", "user.email", "test@local")
	git("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(mainRoot, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-q", "-m", "seed")

	sess, err := worktree.Create(filepath.Join(t.TempDir(), "wt"), mainRoot, "trace-evidence-test")
	if err != nil {
		t.Fatalf("worktree.Create: %v", err)
	}
	t.Cleanup(func() { _ = sess.Discard() })
	if err := os.WriteFile(filepath.Join(sess.Path(), "seed.txt"), []byte("changed by plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sha, err := worktree.CommitChanges(sess.Path(), "codrax apply iter (plan=plan-evidence)")
	if err != nil {
		t.Fatalf("CommitChanges: %v", err)
	}

	planDir := t.TempDir()
	mu := types.NewMutableState("evidence")
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, WorktreePath: sess.Path(), MainRepoRoot: mainRoot}
	o.reportDir = planDir
	o.currentIterCommitSHA = sha

	run := types.WriteWorkflowRun{
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID: "batch-1",
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "verify", Status: "failed", ReasonCode: "tests_failed", PlanID: "plan-evidence", ReportID: "plan-evidence.report.json"},
			},
		}},
	}
	report := &types.ChangeReport{PlanID: "plan-evidence", Passed: false, FailureSummary: "red"}
	o.persistVerifyFailureEvidence(&run, report, 1)

	reportPath := filepath.Join(planDir, "plan-evidence.report.json")
	loaded := readChangeReportFile(t, reportPath)
	if loaded.GeneratedAt.IsZero() {
		t.Fatal("persisted report must carry a non-zero generated_at")
	}
	diffPath := filepath.Join(planDir, "plan-evidence.attempt-1.diff")
	diffBytes, err := os.ReadFile(diffPath)
	if err != nil {
		t.Fatalf("attempt diff not persisted: %v", err)
	}
	if !strings.Contains(string(diffBytes), "changed by plan") {
		t.Fatalf("attempt diff should contain the applied change, got:\n%s", string(diffBytes))
	}
	attempt := run.Batches[0].Attempts[0]
	if attempt.ArtifactRef != "plan-evidence.attempt-1.diff" {
		t.Fatalf("verify attempt ArtifactRef should point at the diff, got %q", attempt.ArtifactRef)
	}
}

// The controller workflow's verify-failure branch must persist the report
// even when stage hooks are bypassed — the durable artifact chain cannot
// depend on a single save site (the missing-report eval class).
func TestRunWriteControllerWorkflow_VerifyFailurePersistsReportArtifact(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	planDir := t.TempDir()
	mu := types.NewMutableState("persist failure report")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "persist failure report"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "attempt"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "applied"},
		{Action: writeflow.ActionFinish, ReasonCode: "accept", FinishDisposition: writeflow.FinishDispositionAcceptUnverified},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	o.reportDir = planDir
	o.SetWriteRetryBudget(3)
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-durable", Status: types.PlanStatusPending, Summary: "attempt", TargetPaths: []string{"fix.go"}})
		case types.StageVerify:
			mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-durable", Passed: false, FailureSummary: "still red"})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	reportPath := filepath.Join(planDir, "plan-durable.report.json")
	loaded := readChangeReportFile(t, reportPath)
	if loaded.Passed {
		t.Fatalf("persisted report should record the failure: %+v", loaded)
	}
	if loaded.GeneratedAt.IsZero() {
		t.Fatal("persisted report must carry generated_at")
	}
}

func readChangeReportFile(t *testing.T, path string) *types.ChangeReport {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("durable report JSON missing at %s: %v", path, err)
	}
	var report types.ChangeReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("report JSON did not parse: %v", err)
	}
	return &report
}
