package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/config"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// Explicit operator opt-in only. This is a real planner-stage evaluation of
// the restored proof-followup workflow, not an ordinary apply run and not a
// mock PASS. Public emit/apply + real Git/native execution prepare the source;
// only controller dispatch can mint registration or execution authority.
func TestNativeRegistrationLiveRestoredFollowup(t *testing.T) {
	out := os.Getenv("CODRAX_LIVE_REGISTRATION_EVAL_DIR")
	if out == "" {
		t.Skip("explicit live evaluation not requested")
	}
	if !filepath.IsAbs(out) {
		t.Fatal("live output must be a new absolute directory")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	save := func(name string, value any) {
		t.Helper()
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	logger, err := logging.NewFromFlags(filepath.Join(out, "logs"), "debug", false)
	if err != nil {
		t.Fatal(err)
	}
	previous := logging.Default
	logging.SetDefault(logger)
	defer func() { logging.SetDefault(previous); _ = logger.Close() }()
	providers := os.Getenv("CODRAX_LIVE_PROVIDERS")
	if providers == "" {
		providers = "../../providers.yaml"
	}
	cfg, err := config.LoadProviders(providers)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := llm.NewFromConfig(config.ResolveProvider(cfg, string(types.AgentPlanner)))
	if err != nil {
		t.Fatal(err)
	}
	const question = "实现已修好，请运行保留的现有测试，确认返回值正确。不要再修改源码、测试、配置或依赖。"
	save("evaluation.json", map[string]any{"kind": "restored_native_registration_planner", "model": adapter.ModelID(), "question": question, "full_cli_run": false, "fresh_process_restore": true, "controller_decisions": "scripted_verify_then_finish"})
	f := newControllerRegistrationFixture(t)
	o, mu := f.o, f.o.busCtx.Mutable
	mu.SetObjective(question)
	// Rehydrate the old *actual* execution from the exact source-plan path.
	path := filepath.Join(o.ensureChangeReportDir(), f.source.ID+".report.json")
	if err := types.WriteChangeReportToFile(mu.ChangeReport(), path); err != nil {
		t.Fatal(err)
	}
	save("source-plan.json", f.source)
	save("old-report.json", mu.ChangeReport())
	mu.ResetChangeReport()
	reg := tool.NewRegistry()
	reg.Register(&tool.ReadFile{})
	reg.Register(&tool.EmitChangePlan{})
	reg.Register(&tool.EmitPlanSkeleton{})
	skills := skill.NewRegistry()
	skill.RegisterDefaults(skills)
	planningSkill, err := skills.Get("change-plan-skill")
	if err != nil {
		t.Fatal(err)
	}
	planner := agent.NewPlannerAgent(&agent.Dependencies{Tools: reg, LLM: adapter, MaxIterations: 12, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
	o.controllerWriteStageFn = func(stage types.PipelineStage, steps *int) (*agent.StageOutput, error) {
		if stage != types.StagePlan || mu.NativeTestRegistrationAuthorization() == nil || mu.ChangeReport() != nil {
			t.Fatal("controller did not authorize fresh read-only planning")
		}
		*steps++
		ctx := &types.AgentContext{RepoRoot: o.busCtx.RepoRoot, WorkDir: o.busCtx.WorkDir, MainRepoRoot: o.busCtx.MainRepoRoot, Mode: types.ModeApply, Stage: stage, AgentName: types.AgentPlanner, Mutable: mu}
		result, err := planner.Execute(ctx, planningSkill)
		if err == nil {
			err = planPostHook(o, result)
		}
		return result, err
	}
	defer func() {
		save("final-plan.json", o.busCtx.Mutable.ChangePlan())
		save("final-report.json", o.busCtx.Mutable.ChangeReport())
		save("workflow.json", o.busCtx.Mutable.WriteWorkflowRun())
		for _, name := range []string{"value.py", "test_value.py"} {
			data, err := os.ReadFile(filepath.Join(o.busCtx.RepoRoot, name))
			if err == nil {
				_ = os.WriteFile(filepath.Join(out, name), data, 0600)
			}
		}
	}()
	steps := 0
	if err := o.runControllerPlanBatch(&writeflow.WriteBatchPlan{ID: "proof", Purpose: "verification_proof_followup", ExpectedPaths: []string{"value.py"}}, &steps); err != nil {
		t.Fatal(err)
	}
	plan := mu.ChangePlan()
	if steps != 1 || !types.IsPersistedNativeTestRegistrationPlan(plan) || len(plan.Changes) != 0 || mu.NativeTestRegistrationAuthorization() != nil || mu.ChangeReport() != nil {
		t.Fatalf("planner did not return an independent read-only registration: plan=%+v", plan)
	}
	run := mu.WriteWorkflowRun()
	o.stampWorkflowPlanForActiveBatch(plan, run)
	updateWorkflowRunBatchPlan(run, run.ActiveBatchID, plan, f.source)
	o.stampCumulativeVerificationScope(plan, run, f.source)
	updateWorkflowRunBatchStatus(run, run.ActiveBatchID, types.WriteWorkflowBatchPlanned)
	promoteActiveProofProbeOnlyBatchToVerifyOnly(run)
	mu.SetWriteWorkflowRun(run)
	save("before-restart-plan.json", plan)
	save("before-restart-workflow.json", run)
	report := f.restart(t)
	plan = o.busCtx.Mutable.ChangePlan()
	if report == nil || len(report.ExistingTestExecutions) != 1 || !types.BehaviorContractRefHasVerificationWitness(plan, report, "increment-result") {
		t.Fatal("fresh execution did not prove the original required behavior")
	}
	receipt := report.ExistingTestExecutions[0]
	invocation := report.ExecutedCommands[receipt.CommandIndex].InvocationID
	if invocation == "" || invocation == f.oldInvocation || receipt.SourcePlanID != f.source.ID || receipt.AppliedCommitSHA != f.head || o.busCtx.Mutable.NativeTestRegistrationExecutionAuthorized(plan, o.busCtx.RepoRoot) {
		t.Fatal("old execution reused or fresh grant leaked")
	}
	root := o.busCtx.RepoRoot
	body, _ := os.ReadFile(filepath.Join(root, "test_value.py"))
	if string(body) != controllerRegistrationTestSource || strings.TrimSpace(runGitForWorkflowRestoreTest(t, root, "rev-parse", "HEAD")) != f.head || runGitForWorkflowRestoreTest(t, root, "diff", "HEAD", "--") != "" {
		t.Fatal("registration changed retained source/tests")
	}
	completedRun := o.busCtx.Mutable.WriteWorkflowRun()
	save("receipt.json", map[string]any{"passed": true, "source_plan_id": f.source.ID, "registration_plan_id": plan.ID, "old_invocation": f.oldInvocation, "fresh_invocation": invocation, "behavior_contract": "increment-result", "readonly": true, "fresh_process_restore": true, "durable_run_complete": completedRun.Status == types.WriteWorkflowRunComplete, "workflow_completion": completedRun.Completion, "proof_batch_verified": true})
}
