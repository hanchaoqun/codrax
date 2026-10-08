package orchestrator

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// Only transport addresses/expectations across the process boundary. In
// particular no private grant, analysis IR, model-read receipt, or report is
// installed in the child by the harness.
type registrationRestartInput struct {
	Root, WorkDir, Objective, PlanID, SourceID, Head, OldInvocation string
	ParentPID                                                       int
	Reject                                                          string
}

func (f controllerRegistrationFixture) restart(t *testing.T, reject ...string) *types.ChangeReport {
	t.Helper()
	o := f.o
	plan, run := o.busCtx.Mutable.ChangePlan(), o.busCtx.Mutable.WriteWorkflowRun()
	identity := o.currentWriteWorkflowRepoIdentity(o.currentWriteWorkflowGoalHashSource())
	run.Identity = &identity
	store := repl.NewWriteWorkflowRunStore(o.ensureChangeReportDir())
	if _, err := store.Save(run); err != nil {
		t.Fatal(err)
	}
	if err := types.WritePlanToFile(plan, filepath.Join(o.ensureChangeReportDir(), plan.ID+".json")); err != nil {
		t.Fatal(err)
	}
	input := registrationRestartInput{Root: o.busCtx.RepoRoot, WorkDir: o.busCtx.WorkDir, Objective: o.busCtx.Mutable.Objective(), PlanID: plan.ID, SourceID: f.source.ID, Head: f.head, OldInvocation: f.oldInvocation, ParentPID: os.Getpid()}
	if len(reject) > 0 {
		input.Reject = reject[0]
	}
	body, _ := json.Marshal(input)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestNativeRegistrationFreshProcessHelper$", "-test.v")
	cmd.Env = append(os.Environ(), "CODRAX_REGISTRATION_RESTART_INPUT="+string(body))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fresh-process restore: %v\n%s", err, out)
	}
	t.Logf("fresh-process restore:\n%s", out)
	if input.Reject != "" {
		return nil
	}
	report, err := types.LoadChangeReportFromFile(filepath.Join(o.busCtx.WorkDir, "restart-report.json"))
	if err != nil {
		t.Fatal(err)
	}
	restored, err := store.Load(run.RunID)
	if err != nil || restored.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("default durable store did not complete: %v %+v", err, restored)
	}
	// Mirror the child's durable outputs for the optional live audit only;
	// these values were not supplied to its execution environment.
	completed, err := types.LoadChangePlanFromFile(filepath.Join(o.ensureChangeReportDir(), plan.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	o.busCtx.Mutable.SetChangePlan(completed)
	o.busCtx.Mutable.SetWriteWorkflowRun(restored)
	o.busCtx.Mutable.SetChangeReport(report)
	return report
}

func TestNativeRegistrationFreshProcess(t *testing.T) {
	for _, status := range []types.WriteWorkflowBatchStatus{types.WriteWorkflowBatchPlanned, types.WriteWorkflowBatchVerifying} {
		t.Run(string(status), func(t *testing.T) {
			f := newControllerRegistrationFixture(t)
			f.register(t, "emit_change_plan")
			run := f.o.busCtx.Mutable.WriteWorkflowRun()
			updateWorkflowRunBatchStatus(run, run.ActiveBatchID, status)
			f.o.busCtx.Mutable.SetWriteWorkflowRun(run)
			report := f.restart(t)
			if !types.BehaviorContractRefHasVerificationWitness(f.o.busCtx.Mutable.ChangePlan(), report, "increment-result") {
				t.Fatal("restored execution did not prove the original contract")
			}
		})
	}
}

func TestNativeRegistrationFreshProcessRejects(t *testing.T) {
	for _, condition := range []string{"missing_source", "changed_contract", "missing_run_receipt"} {
		t.Run(condition, func(t *testing.T) {
			f := newControllerRegistrationFixture(t)
			f.register(t, "emit_change_plan")
			if condition == "missing_source" {
				if err := os.Rename(f.sourceRef, f.sourceRef+".withheld"); err != nil {
					t.Fatal(err)
				}
			}
			if condition == "missing_run_receipt" {
				run := f.o.busCtx.Mutable.WriteWorkflowRun()
				run.NativeTestRegistrations = nil
				f.o.busCtx.Mutable.SetWriteWorkflowRun(run)
			}
			f.restart(t, condition)
		})
	}
}

func TestNativeRegistrationFreshProcessHelper(t *testing.T) {
	raw := os.Getenv("CODRAX_REGISTRATION_RESTART_INPUT")
	if raw == "" {
		t.Skip("subprocess entry only")
	}
	var input registrationRestartInput
	if err := json.Unmarshal([]byte(raw), &input); err != nil || input.ParentPID == os.Getpid() {
		t.Fatalf("not an independent process: %v", err)
	}
	mu := types.NewMutableState(input.Objective)
	calls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, []writeflow.WriteWorkflowDecision{
			{Action: writeflow.ActionVerifyBatch, ReasonCode: "resume_registered_tests"},
			{Action: writeflow.ActionFinish, ReasonCode: "all_verified"},
		}, &calls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, RepoRoot: input.Root, MainRepoRoot: input.Root, WorkDir: input.WorkDir, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelTokenPtr.Store(NewCancelToken())
	o.writeWorkflowRunStore = repl.NewWriteWorkflowRunStore(filepath.Join(input.WorkDir, "plans"))
	if mu.ChangePlan() != nil || mu.WriteAnalysisIR() != nil || mu.ChangeReport() != nil {
		t.Fatal("harness installed durable authority directly")
	}
	if input.Reject != "" {
		var changed *types.WriteAnalysisIR
		if input.Reject == "changed_contract" {
			changed = &types.WriteAnalysisIR{Request: types.WriteRequestModel{BehaviorContracts: []types.WriteBehaviorContract{{ID: "increment-result", Kind: types.WriteBehaviorObservable, Operator: types.WriteBehaviorOpEquals, Expected: "increment(4) == 99", Required: true}}}}
			mu.SetWriteAnalysisIR(changed)
		}
		run, err := o.loadOrSeedWriteWorkflowRun()
		if err != nil {
			t.Fatal(err)
		}
		if err = o.hydrateResumedWorkflowState(&run, map[string]int{}); err != nil {
			t.Fatal(err)
		}
		if changed != nil && mu.WriteAnalysisIR() != changed {
			t.Fatal("resume erased the changed requirement")
		}
		if err = o.authorizeNativeTestRegistrationVerification(mu.ChangePlan()); err == nil || mu.NativeTestRegistrationExecutionAuthorized(mu.ChangePlan(), input.Root) || mu.ChangeReport() != nil {
			t.Fatalf("invalid restore authorized execution: %s %v", input.Reject, err)
		}
		t.Logf("rejected before execution: %s: %v", input.Reject, err)
		return
	}
	verified := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, steps *int) (*agent.StageOutput, error) {
		plan := mu.ChangePlan()
		if stage != types.StageVerify || plan == nil || plan.ID != input.PlanID || mu.NativeTestRegistrationAuthorization() != nil || !mu.NativeTestRegistrationExecutionAuthorized(plan, input.Root) {
			t.Fatalf("unexpected dispatch or missing fresh authorization: %s %+v", stage, plan)
		}
		verified++
		*steps++
		result := controllerRegistrationTool(t, o.busCtx, (&tool.RunTests{}).Execute, map[string]any{})
		return &agent.StageOutput{ToolResults: []types.ToolResult{result}}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Logf("restored run=%+v plan=%+v ir=%+v", mu.WriteWorkflowRun(), mu.ChangePlan(), mu.WriteAnalysisIR())
		t.Fatalf("resume workflow: %v", err)
	}
	plan, report := mu.ChangePlan(), mu.ChangeReport()
	run := mu.WriteWorkflowRun()
	proofVerified := false
	for _, batch := range run.Batches {
		if batch.PlanID == input.PlanID {
			proofVerified = batch.Completion != nil && batch.Completion.Verdict == types.WriteWorkflowCompletionVerified
		}
	}
	if !proofVerified {
		ledger := types.BuildVerificationProofLedger(plan, report, nil)
		t.Fatalf("new proof batch not verified: completion=%+v proof=%s reasons=%v", run.Completion, ledger.State, ledger.ReasonCodes)
	}
	// This fixture has actual source delivery but no terminal verdict for the
	// source batch. A verified follow-up must not fabricate that missing verdict.
	if run.Completion == nil || run.Completion.Verdict != types.WriteWorkflowCompletionUnverified || run.Completion.ReasonCode != "missing_terminal_verify_verdict" {
		t.Fatalf("missing source completion was laundered: %+v", run.Completion)
	}
	if verified != 1 || report == nil || !report.Passed || len(report.ExistingTestExecutions) != 1 || !types.BehaviorContractRefHasVerificationWitness(plan, report, "increment-result") {
		t.Fatalf("new registered proof missing: executions=%d report=%+v", verified, report)
	}
	r := report.ExistingTestExecutions[0]
	invocation := report.ExecutedCommands[r.CommandIndex].InvocationID
	if invocation == "" || invocation == input.OldInvocation || r.SourcePlanID != input.SourceID || r.AppliedCommitSHA != input.Head || mu.NativeTestRegistrationExecutionAuthorized(plan, input.Root) {
		t.Fatal("historical proof reused or execution grant leaked")
	}
	testBytes, err := os.ReadFile(filepath.Join(input.Root, "test_value.py"))
	if err != nil || string(testBytes) != controllerRegistrationTestSource || strings.TrimSpace(runGitForWorkflowRestoreTest(t, input.Root, "rev-parse", "HEAD")) != input.Head || runGitForWorkflowRestoreTest(t, input.Root, "diff", "HEAD", "--") != "" {
		t.Fatal("restored verification changed source, tests or HEAD")
	}
	if err := types.WriteChangeReportToFile(report, filepath.Join(input.WorkDir, "restart-report.json")); err != nil {
		t.Fatal(err)
	}
	t.Logf("pid=%d parent=%d new_invocation=%s retained_head=%s", os.Getpid(), input.ParentPID, invocation, input.Head)
}
