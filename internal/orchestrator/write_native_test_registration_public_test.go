package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

const controllerRegistrationTestSource = "import unittest\nfrom value import increment\nclass ValueTest(unittest.TestCase):\n    def test_increment(self): self.assertEqual(increment(4), 5)\n"

type controllerRegistrationFixture struct {
	o               *Orchestrator
	source          *types.ChangePlan
	ir              *types.WriteAnalysisIR
	head, sourceRef string
	oldInvocation   string
}

func controllerRegistrationTool(t *testing.T, ctx *types.BusContext, run func(*types.BusContext, json.RawMessage) (types.ToolResult, error), params any) types.ToolResult {
	t.Helper()
	body, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	result, err := run(ctx, body)
	if err != nil || !result.Success {
		t.Fatalf("public %s: %v %s", result.ToolName, err, result.Summary)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	return result
}

// Public emit/apply tools and a real Git commit create source delivery. No
// model-read receipt, registration, execution report or passing flag is forged.
func newControllerRegistrationFixture(t *testing.T) controllerRegistrationFixture {
	t.Helper()
	for _, executable := range []string{"git", "python3"} {
		if _, err := exec.LookPath(executable); err != nil {
			t.Skip(executable + " unavailable")
		}
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string]string{"value.py": "def increment(value):\n    return value\n", "test_value.py": controllerRegistrationTestSource, ".gitignore": "__pycache__/\n*.pyc\n.codrax/\n"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) string { return runGitForWorkflowRestoreTest(t, root, args...) }
	git("init", "-q")
	git("add", ".")
	git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "-qm", "before")
	mu := types.NewMutableState("correct increment and retain existing tests")
	ir := &types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopePackage, Summary: "increment adds one"}, BehaviorContracts: []types.WriteBehaviorContract{{ID: "increment-result", Kind: types.WriteBehaviorObservable, Operator: types.WriteBehaviorOpEquals, Expected: "increment(4) == 5", Required: true}}}}
	mu.SetWriteAnalysisIR(ir)
	ctx := &types.BusContext{Mutable: mu, RepoRoot: root, MainRepoRoot: root, WorkDir: t.TempDir(), Mode: types.ModeApply, PipelineStage: types.StagePlan}
	controllerRegistrationTool(t, ctx, (&tool.EmitChangePlan{}).Execute, map[string]any{"request": "correct increment", "summary": "Correct the source while retaining existing tests.", "changes": []map[string]any{{"path": "value.py", "kind": "modify", "new_content": "def increment(value):\n    return value + 1\n", "rationale": "return the next integer"}}})
	source := mu.ChangePlan()
	ctx.PipelineStage = types.StageApply
	controllerRegistrationTool(t, ctx, (&tool.ApplyPatch{}).Execute, map[string]any{"path": "value.py", "kind": "modify"})
	git("add", "value.py")
	git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "-qm", "applied")
	head := strings.TrimSpace(git("rev-parse", "HEAD"))
	diff, err := worktree.CaptureCommitPatch(root, head)
	if err != nil {
		t.Fatal(err)
	}
	effect := writeflow.PatchEffectRecordFromUnifiedDiff(source.ID, "", "applied_commit", head+"^", head, diff)
	source.PatchEffect, source.AppliedCommitSHA, source.WorktreePath, source.Status = &effect, head, root, types.PlanStatusApplied
	mu.SetChangePlan(source)
	ctx.PipelineStage = types.StageVerify
	// The old real execution lacks the new declaration and may not be reused.
	_, err = (&tool.RunTests{}).Execute(ctx, json.RawMessage(`{}`))
	if err != nil || mu.ChangeReport() == nil {
		t.Fatalf("initial real native run: %v", err)
	}
	if types.BehaviorContractRefHasVerificationWitness(source, mu.ChangeReport(), "increment-result") {
		t.Fatal("undeclared old test unexpectedly supplies registered proof")
	}
	oldInvocation := ""
	for _, row := range mu.ChangeReport().TestResults {
		if row.ObservationScope == types.TestObservationScopeAssertion && row.AssertionID == "test_increment" && row.Passed {
			oldInvocation = row.InvocationID
		}
	}
	if oldInvocation == "" {
		t.Fatal("fixture lacks an actual old passing native invocation to reject as new proof")
	}
	sourceRef := filepath.Join(ctx.WorkDir, "plans", source.ID+".json")
	if err := types.WritePlanToFile(source, sourceRef); err != nil {
		t.Fatal(err)
	}
	mu.SetWriteWorkflowRun(&types.WriteWorkflowRun{RunID: "native-registration-run", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "proof", ProgressLedger: []types.WriteWorkflowProgress{{BatchID: "source", ReasonCode: "verification_proof_followup_requested"}}, Batches: []types.WriteWorkflowBatch{
		{ID: "source", PlanID: source.ID, Status: types.WriteWorkflowBatchComplete, Attempts: []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: source.ID, FinishedAt: time.Now().Add(-time.Minute)}}},
		{ID: "proof", Purpose: "verification_proof_followup", ExpectedPaths: []string{"value.py"}, Status: types.WriteWorkflowBatchReadyToPlan},
	}})
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = ctx
	o.cancelTokenPtr.Store(NewCancelToken())
	return controllerRegistrationFixture{o: o, source: source, ir: ir, head: head, sourceRef: sourceRef, oldInvocation: oldInvocation}
}

type controllerRegistrationLLM struct {
	t     *testing.T
	entry string
	round int
}

func (l *controllerRegistrationLLM) Chat(_ context.Context, messages []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.round++
	available := map[string]bool{}
	for _, schema := range schemas {
		available[schema.Name] = true
	}
	if !available[l.entry] || l.round == 1 && !available["read_file"] {
		l.t.Fatalf("real model tool surface omits the registration route: %v", available)
	}
	if l.round == 1 {
		var deliveredTeaching strings.Builder
		for _, message := range messages {
			deliveredTeaching.WriteString(message.Content)
		}
		for _, rule := range []string{
			"This dispatch also permits read-only existing-test registration as an alternative to probes:",
			"first read each entire existing Python unittest file and receive its contents in a model turn",
			"It is not a test pass: verification must execute these exact tests again.",
			"No additional metadata fields are needed.",
		} {
			if strings.Count(deliveredTeaching.String(), rule) != 1 {
				l.t.Fatalf("actual planning request must teach current registration exactly once: %q", rule)
			}
		}
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "read-native", Name: "read_file", Params: json.RawMessage(`{"path":"test_value.py"}`)}}}, nil
	}
	if l.round != 2 {
		return llm.Response{}, fmt.Errorf("unexpected extra planner request %d", l.round)
	}
	delivered := false
	for _, message := range messages {
		if message.Role == "tool" && message.ToolCallID == "read-native" && strings.Contains(message.Content, "def test_increment(self): self.assertEqual(increment(4), 5)") {
			delivered = true
		}
	}
	if !delivered {
		for _, message := range messages {
			l.t.Logf("actual %s %s: %.1200s", message.Role, message.ToolCallID, message.Content)
		}
		l.t.Fatal("complete native read was not in the actual model request")
	}
	params := json.RawMessage(`{"request":"Verify retained source without another edit","summary":"Bind the existing native assertion, then execute it again.","changes":[],"project_test_observations":[{"id":"existing-increment","test_path":"test_value.py","assertion_suite":"test_value.ValueTest","assertion_id":"test_increment","contract_refs":["increment-result"]}]}`)
	return llm.Response{ToolCalls: []llm.ToolCall{{ID: "register-native", Name: l.entry, Params: params}}}, nil
}
func (*controllerRegistrationLLM) ModelID() string               { return "controller-registration-public" }
func (*controllerRegistrationLLM) MaxContextTokens() int         { return 128000 }
func (*controllerRegistrationLLM) MaxOutputTokens() int          { return 4096 }
func (*controllerRegistrationLLM) RequestTimeout() time.Duration { return 0 }
func (*controllerRegistrationLLM) RetryMaxAttempts() int         { return 0 }

func (f controllerRegistrationFixture) register(t *testing.T, entry string) *types.ChangePlan {
	t.Helper()
	o, mu := f.o, f.o.busCtx.Mutable
	before, _ := json.Marshal(f.source)
	adapter := &controllerRegistrationLLM{t: t, entry: entry}
	registry := tool.NewRegistry()
	registry.Register(&tool.ReadFile{})
	registry.Register(&tool.EmitChangePlan{})
	registry.Register(&tool.EmitPlanSkeleton{})
	planner := agent.NewPlannerAgent(&agent.Dependencies{Tools: registry, LLM: adapter, MaxIterations: 2, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
	o.controllerWriteStageFn = func(stage types.PipelineStage, steps *int) (*agent.StageOutput, error) {
		if stage != types.StagePlan || mu.NativeTestRegistrationAuthorization() == nil || mu.ChangeReport() != nil {
			t.Fatal("scheduler did not prepare grant and clear old execution before planning")
		}
		*steps++
		ctx := &types.AgentContext{RepoRoot: o.busCtx.RepoRoot, WorkDir: o.busCtx.WorkDir, MainRepoRoot: o.busCtx.MainRepoRoot, Mode: types.ModeApply, Stage: stage, AgentName: types.AgentPlanner, Mutable: mu}
		out, err := planner.Execute(ctx, &skill.Config{ToolSuggestions: []string{"read_file", entry}})
		if err == nil {
			err = planPostHook(o, out)
		}
		return out, err
	}
	steps := 0
	batch := &writeflow.WriteBatchPlan{ID: "proof", Purpose: "verification_proof_followup", ExpectedPaths: []string{"value.py"}}
	if err := o.runControllerPlanBatch(batch, &steps); err != nil {
		t.Fatalf("scheduler real registration: %v", err)
	}
	plan := mu.ChangePlan()
	if adapter.round != 2 || steps != 1 || !types.IsPersistedNativeTestRegistrationPlan(plan) || mu.NativeTestRegistrationAuthorization() != nil || mu.ChangeReport() != nil {
		t.Fatalf("registration mutated proof or leaked grant: rounds=%d steps=%d plan=%+v", adapter.round, steps, plan)
	}
	run := mu.WriteWorkflowRun()
	o.stampWorkflowPlanForActiveBatch(plan, run)
	updateWorkflowRunBatchPlan(run, run.ActiveBatchID, plan, f.source)
	o.stampCumulativeVerificationScope(plan, run, f.source)
	updateWorkflowRunBatchStatus(run, run.ActiveBatchID, types.WriteWorkflowBatchPlanned)
	promoteActiveProofProbeOnlyBatchToVerifyOnly(run)
	mu.SetWriteWorkflowRun(run)
	after, _ := json.Marshal(f.source)
	if !bytes.Equal(before, after) || len(plan.Changes) != 0 || plan.PatchEffect != nil || plan.AppliedCommitSHA != "" || len(types.RequiredExistingTestPaths(plan)) != 0 {
		t.Fatal("read-only registration changed source ownership or invented an execution requirement")
	}
	return plan
}

func (f controllerRegistrationFixture) reload(t *testing.T) *types.ChangePlan {
	t.Helper()
	ctx := f.o.busCtx
	planPath, runPath := filepath.Join(ctx.WorkDir, "registration.json"), filepath.Join(ctx.WorkDir, "registration-run.json")
	if err := types.WritePlanToFile(ctx.Mutable.ChangePlan(), planPath); err != nil {
		t.Fatal(err)
	}
	if err := types.WriteWorkflowRunToFile(ctx.Mutable.WriteWorkflowRun(), runPath); err != nil {
		t.Fatal(err)
	}
	plan, err := types.LoadChangePlanFromFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	run, err := types.LoadWriteWorkflowRunFromFile(runPath)
	if err != nil {
		t.Fatal(err)
	}
	mu := types.NewMutableState("restore native registration")
	mu.SetWriteAnalysisIR(f.ir)
	mu.SetChangePlan(plan)
	mu.SetWriteWorkflowRun(run)
	ctx.Mutable = mu
	if mu.NativeTestRegistrationAuthorization() != nil || mu.NativeTestRegistrationExecutionAuthorized(plan, ctx.RepoRoot) || mu.ChangeReport() != nil {
		t.Fatal("durable declaration resurrected private authorization or historical execution")
	}
	return plan
}

func TestNativeRegistrationControllerPublicRoundTrip(t *testing.T) {
	for _, entry := range []string{"emit_change_plan", "emit_plan_skeleton"} {
		t.Run(entry, func(t *testing.T) {
			f := newControllerRegistrationFixture(t)
			f.register(t, entry)
			var priorInvocation string
			for invocation := 0; invocation < 2; invocation++ {
				plan := f.reload(t)
				if !writeFinalMaterializationStrictProofOnly(plan) || !f.o.writeFinalReportDeliverySummary(f.o.busCtx.Mutable.WriteWorkflowRun(), plan, nil).FinalPlanValidationOnly {
					t.Fatal("final delivery mistook the read-only registration for a new source change")
				}
				f.o.controllerWriteStageFn = func(stage types.PipelineStage, steps *int) (*agent.StageOutput, error) {
					if stage != types.StageVerify || !f.o.busCtx.Mutable.NativeTestRegistrationExecutionAuthorized(plan, f.o.busCtx.RepoRoot) {
						t.Fatal("controller dispatched verifier without exact fresh authorization")
					}
					*steps++
					result := controllerRegistrationTool(t, f.o.busCtx, (&tool.RunTests{}).Execute, map[string]any{})
					return &agent.StageOutput{ToolResults: []types.ToolResult{result}}, nil
				}
				steps := 0
				if err := f.o.runControllerVerifyBatch(&steps); err != nil {
					t.Fatal(err)
				}
				if steps != 1 || f.o.busCtx.Mutable.NativeTestRegistrationExecutionAuthorized(plan, f.o.busCtx.RepoRoot) {
					t.Fatal("verifier did not run once or leaked its authorization")
				}
				report := f.o.busCtx.Mutable.ChangeReport()
				if report == nil || len(report.ExistingTestExecutions) != 1 || !types.BehaviorContractRefHasVerificationWitness(plan, report, "increment-result") {
					t.Fatalf("fresh registered native proof missing: %+v", report)
				}
				receipt := report.ExistingTestExecutions[0]
				id := report.ExecutedCommands[receipt.CommandIndex].InvocationID
				if id == "" || id == priorInvocation || id == f.oldInvocation || receipt.PlanID != plan.ID || receipt.SourcePlanID != f.source.ID || receipt.AppliedCommitSHA != f.head {
					t.Fatalf("execution reused/relabelled proof: %+v invocation=%q", receipt, id)
				}
				priorInvocation = id
				f.o.syncMutablePlanStatusAfterVerify(report, nil)
			}
			root := f.o.busCtx.RepoRoot
			body, _ := os.ReadFile(filepath.Join(root, "test_value.py"))
			if string(body) != controllerRegistrationTestSource || strings.TrimSpace(runGitForWorkflowRestoreTest(t, root, "rev-parse", "HEAD")) != f.head || runGitForWorkflowRestoreTest(t, root, "diff", "HEAD", "--") != "" {
				t.Fatal("registration/verification changed the delivered source or existing test")
			}
		})
	}
}

func TestNativeRegistrationControllerPublicRestoreRejects(t *testing.T) {
	for _, condition := range []string{"skip", "rejected", "old_run", "missing_source", "contract_changed", "rollback"} {
		t.Run(condition, func(t *testing.T) {
			f := newControllerRegistrationFixture(t)
			f.register(t, "emit_change_plan")
			plan := f.reload(t)
			mu := f.o.busCtx.Mutable
			run := mu.WriteWorkflowRun()
			switch condition {
			case "skip":
				f.o.skipVerify = true
			case "rejected":
				plan.Status = types.PlanStatusRejected
			case "old_run":
				run.NativeTestRegistrations = nil
				mu.SetWriteWorkflowRun(run)
			case "missing_source":
				if err := os.Rename(f.sourceRef, f.sourceRef+".removed"); err != nil {
					t.Fatal(err)
				}
			case "contract_changed":
				changed := *f.ir
				changed.Request = f.ir.Request
				changed.Request.BehaviorContracts = append([]types.WriteBehaviorContract(nil), f.ir.Request.BehaviorContracts...)
				changed.Request.BehaviorContracts[0].Expected = "increment(4) == 9"
				mu.SetWriteAnalysisIR(&changed)
			case "rollback":
				run.ProgressLedger = append(run.ProgressLedger, types.WriteWorkflowProgress{BatchID: "source", ReasonCode: "checkpoint_restored_before_replan", At: time.Now()})
				mu.SetWriteWorkflowRun(run)
			}
			before, _ := json.Marshal([]any{plan, mu.WriteWorkflowRun()})
			f.o.controllerWriteStageFn = func(types.PipelineStage, *int) (*agent.StageOutput, error) {
				t.Fatal("rejected registration reached verifier dispatch")
				return nil, nil
			}
			steps := 0
			if err := f.o.runControllerVerifyBatch(&steps); err == nil || steps != 0 || mu.NativeTestRegistrationExecutionAuthorized(plan, f.o.busCtx.RepoRoot) {
				t.Fatalf("%s regained native execution authority", condition)
			}
			after, _ := json.Marshal([]any{plan, mu.WriteWorkflowRun()})
			if !bytes.Equal(before, after) || mu.ChangeReport() != nil {
				t.Fatal("failed authorization rewrote durable state or created proof")
			}
		})
	}
}
