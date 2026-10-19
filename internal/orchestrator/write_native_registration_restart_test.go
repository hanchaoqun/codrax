package orchestrator

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
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
	SourceTerminal                                                  bool
	PriorInvocations                                                []string
	NaturalController                                               bool
	ProvidersPath, AuditDir                                         string
}

func (f controllerRegistrationFixture) restart(t *testing.T, reject ...string) *types.ChangeReport {
	return f.restartExpectingSourceTerminal(t, false, reject...)
}

func (f controllerRegistrationFixture) restartExpectingSourceTerminal(t *testing.T, sourceTerminal bool, reject ...string) *types.ChangeReport {
	t.Helper()
	return f.restartWithController(t, sourceTerminal, false, "", "", reject...)
}

func (f controllerRegistrationFixture) restartWithController(t *testing.T, sourceTerminal, natural bool, providers, auditDir string, reject ...string) *types.ChangeReport {
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
	input.SourceTerminal = sourceTerminal
	input.NaturalController, input.ProvidersPath, input.AuditDir = natural, providers, auditDir
	if prior := o.loadWriteFinalReportChangeReport(f.source.ID); prior != nil {
		for _, command := range prior.ExecutedCommands {
			if command.InvocationID != "" {
				input.PriorInvocations = append(input.PriorInvocations, command.InvocationID)
			}
		}
	}
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
	if sourceTerminal {
		auditNativeRegistrationContextPacks(t, store, input.WorkDir, restored)
		assertNativeRegistrationContextPackReceipt(t, input.WorkDir)
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
	// Preserve the independent process's actual state even if an assertion
	// below fails before the success-only report/receipt publication.
	defer saveRegistrationRestartAudit(t, input, mu)
	calls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, []writeflow.WriteWorkflowDecision{
			{Action: writeflow.ActionVerifyBatch, ReasonCode: "resume_registered_tests"},
			{Action: writeflow.ActionFinish, ReasonCode: "all_verified"},
		}, &calls),
	})
	if input.NaturalController {
		installNativeRegistrationChildController(t, input, ar, sr)
	}
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
	if input.SourceTerminal {
		assertNativeRegistrationSourceTerminal(t, run, input.SourceID)
		if run.Completion == nil || run.Completion.Verdict != types.WriteWorkflowCompletionVerified {
			t.Fatalf("verified source and current registration did not complete: %+v", run.Completion)
		}
	} else if run.Completion == nil || run.Completion.Verdict != types.WriteWorkflowCompletionUnverified || run.Completion.ReasonCode != "missing_terminal_verify_verdict" {
		t.Fatalf("missing source completion was laundered: %+v", run.Completion)
	}
	validExecutionCount := verified == 1
	if input.NaturalController {
		// Real decisions may request another fresh verification; do not turn
		// the scripted helper's fixed sequence into a natural-run oracle.
		validExecutionCount = verified >= 1
	}
	if !validExecutionCount || report == nil || !report.Passed || len(report.ExistingTestExecutions) != 1 || !types.BehaviorContractRefHasVerificationWitness(plan, report, "increment-result") {
		t.Fatalf("new registered proof missing: executions=%d report=%+v", verified, report)
	}
	r := report.ExistingTestExecutions[0]
	invocation := report.ExecutedCommands[r.CommandIndex].InvocationID
	if invocation == "" || invocation == input.OldInvocation || r.SourcePlanID != input.SourceID || r.AppliedCommitSHA != input.Head || mu.NativeTestRegistrationExecutionAuthorized(plan, input.Root) {
		t.Fatal("historical proof reused or execution grant leaked")
	}
	for _, previous := range input.PriorInvocations {
		if invocation == previous {
			t.Fatal("registration reused the source controller's previous invocation")
		}
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

var registrationRestartAuditFiles = []string{"restart-child-plan.json", "restart-child-report.json", "restart-child-workflow.json", "restart-child-audit.json"}

func preserveRegistrationRestartAudit(t *testing.T, workDir, out string) {
	t.Helper()
	for _, name := range append(append([]string(nil), registrationRestartAuditFiles...), registrationContextPackAuditFile) {
		if data, err := os.ReadFile(filepath.Join(workDir, name)); err == nil {
			if err := os.WriteFile(filepath.Join(out, name), data, 0600); err != nil {
				t.Errorf("preserve child audit %s: %v", name, err)
			}
		}
	}
}

func saveRegistrationRestartAudit(t *testing.T, input registrationRestartInput, mu *types.MutableState) {
	t.Helper()
	defer preserveRegistrationChildDeliveryArchive(t, input, mu)
	gitRead := func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = input.Root
		body, err := cmd.Output()
		return strings.TrimSpace(string(body)), err
	}
	head, headErr := gitRead("rev-parse", "HEAD")
	diff, diffErr := gitRead("diff", "HEAD", "--")
	status, statusErr := gitRead("status", "--porcelain")
	testBytes, testErr := os.ReadFile(filepath.Join(input.Root, "test_value.py"))
	sourceBytes, sourceErr := os.ReadFile(filepath.Join(input.Root, "value.py"))
	checked := headErr == nil && diffErr == nil && statusErr == nil && testErr == nil && sourceErr == nil
	readonly := checked && head == input.Head && diff == "" && status == "" && string(testBytes) == controllerRegistrationTestSource && string(sourceBytes) == "def increment(value):\n    return value + 1\n"
	invocations := []string{}
	if report := mu.ChangeReport(); report != nil {
		for _, command := range report.ExecutedCommands {
			if command.InvocationID != "" {
				invocations = append(invocations, command.InvocationID)
			}
		}
	}
	audit := map[string]any{
		"pid": os.Getpid(), "parent_pid": input.ParentPID, "independent_process": os.Getpid() != input.ParentPID,
		"test_failed": t.Failed(), "readonly_checked": checked, "readonly": readonly, "head": head, "expected_head": input.Head,
		"git_status": status, "git_diff": diff,
		"current_invocations": invocations, "old_invocation": input.OldInvocation, "prior_source_invocations": input.PriorInvocations,
		"natural_controller": input.NaturalController,
	}
	if testErr == nil {
		audit["test_sha256"] = fmt.Sprintf("%x", sha256.Sum256(testBytes))
	}
	if sourceErr == nil {
		audit["source_sha256"] = fmt.Sprintf("%x", sha256.Sum256(sourceBytes))
	}
	checkErrors := map[string]string{}
	for name, err := range map[string]error{"head": headErr, "diff": diffErr, "status": statusErr, "test": testErr, "source": sourceErr} {
		if err != nil {
			checkErrors[name] = err.Error()
		}
	}
	if len(checkErrors) != 0 {
		audit["check_errors"] = checkErrors
	}
	values := []any{mu.ChangePlan(), mu.ChangeReport(), mu.WriteWorkflowRun(), audit}
	for i, name := range registrationRestartAuditFiles {
		body, err := json.MarshalIndent(values[i], "", "  ")
		if err == nil {
			err = os.WriteFile(filepath.Join(input.WorkDir, name), body, 0600)
		}
		if err != nil {
			t.Errorf("preserve independent child audit %s: %v", name, err)
		}
	}
	t.Logf("independent child audit saved: pid=%d parent=%d failed=%t readonly_checked=%t readonly=%t", os.Getpid(), input.ParentPID, t.Failed(), checked, readonly)
}
