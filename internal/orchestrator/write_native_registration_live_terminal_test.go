package orchestrator

import (
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// The terminal live case must start from a source batch verified by the real
// controller, not merely from an applied source and a historical passing test.
func TestNativeRegistrationLiveTerminalPublic(t *testing.T) {
	f := newControllerRegistrationFixture(t)
	registry := tool.NewRegistry()
	registry.Register(&tool.ReadFile{})
	registry.Register(&tool.EmitChangePlan{})
	registry.Register(&tool.EmitPlanSkeleton{})
	adapter := &controllerRegistrationLLM{t: t, entry: "emit_change_plan"}
	planner := agent.NewPlannerAgent(&agent.Dependencies{Tools: registry, LLM: adapter, MaxIterations: 12, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
	skills := skill.NewRegistry()
	skill.RegisterDefaults(skills)
	planningSkill, err := skills.Get("change-plan-skill")
	if err != nil {
		t.Fatal(err)
	}
	sourceInvocation := ""
	runNativeRegistrationTerminalPlanning(t, f, planner, planningSkill, func(report *types.ChangeReport, run *types.WriteWorkflowRun) {
		assertNativeRegistrationSourceTerminal(t, run, f.source.ID)
		if report == nil || !report.Passed || report.PlanID != f.source.ID || types.BehaviorContractRefHasVerificationWitness(f.source, report, "increment-result") {
			t.Fatal("source receipt must be an actual pass without the later declaration")
		}
		for _, command := range report.ExecutedCommands {
			if command.InvocationID != "" {
				sourceInvocation = command.InvocationID
			}
		}
	})
	if sourceInvocation == "" || sourceInvocation == f.oldInvocation || adapter.round != 2 {
		t.Fatal("source controller did not perform a new verification before the planner")
	}
	f.restartExpectingSourceTerminal(t, true)
}

// Keep the absent-source-terminal control on its original preparation path.
// A new verified proof batch cannot retroactively sign that source batch.
func TestNativeRegistrationLiveTerminalMissingSourceRemainsUnverified(t *testing.T) {
	f := newControllerRegistrationFixture(t)
	f.register(t, "emit_change_plan")
	f.restart(t)
}

// Both the deterministic test above and the opt-in live evaluation use this
// exact public preparation path. Only the planner adapter differs. The source
// completion, proof-followup routing and persisted registration all come from
// the production controller; no terminal verdict or private grant is injected.
func runNativeRegistrationTerminalPlanning(t *testing.T, f controllerRegistrationFixture, planner agent.Agent, planningSkill *skill.Config, sourceVerified func(*types.ChangeReport, *types.WriteWorkflowRun)) {
	t.Helper()
	o, mu := f.o, f.o.busCtx.Mutable
	mu.ResetChangeReport()
	o.reportDir = filepath.Dir(f.sourceRef)
	o.planPath, o.busCtx.PlanPath = "", ""
	run := &types.WriteWorkflowRun{RunID: "native-live-source-terminal", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "source", Batches: []types.WriteWorkflowBatch{{
		ID: "source", PlanID: f.source.ID, Status: types.WriteWorkflowBatchVerifying, ApplyRef: f.head,
		Attempts: []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: f.source.ID, ArtifactRef: f.head}},
	}}}
	identity := o.currentWriteWorkflowRepoIdentity(o.currentWriteWorkflowGoalHashSource())
	run.Identity = &identity
	store := repl.NewWriteWorkflowRunStore(o.ensureChangeReportDir())
	if _, err := store.Save(run); err != nil {
		t.Fatal(err)
	}
	mu.SetWriteWorkflowRun(run)
	o.writeWorkflowRunStore = store
	calls := 0
	ar, sr, _ := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			if types.IsPersistedNativeTestRegistrationPlan(ctx.Mutable.ChangePlan()) {
				// The scheduler has already persisted the plan and typed transition.
				// Stop here without introducing a production failure or terminal.
				panic(nativeRegistrationRestartBoundary{})
			}
			calls++
			if calls > 12 {
				t.Fatal("source controller did not converge to registration")
			}
			action := writeflow.ActionFinish
			if calls == 1 {
				action = writeflow.ActionVerifyBatch
			}
			result := controllerRegistrationTool(t, o.busCtx, (&tool.EmitWriteWorkflowDecision{}).Execute, map[string]any{"action": action})
			return &agent.StageOutput{ToolResults: []types.ToolResult{result}}, nil
		},
	})
	o.agents, o.skills = ar, sr
	o.readExplorationRunner = readExplorationRunnerFunc(func(o *Orchestrator) (int, error) {
		controllerRegistrationTool(t, o.busCtx, (&tool.ReadFile{}).Execute, map[string]any{"path": "value.py"})
		return 1, nil
	})
	planned, verified := 0, 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, steps *int) (*agent.StageOutput, error) {
		*steps++
		switch stage {
		case types.StageVerify:
			verified++
			result := controllerRegistrationTool(t, o.busCtx, (&tool.RunTests{}).Execute, map[string]any{})
			report := mu.ChangeReport()
			if report == nil || !report.Passed || report.PlanID != f.source.ID || len(report.ExecutedCommands) == 0 || types.BehaviorContractRefHasVerificationWitness(mu.ChangePlan(), report, "increment-result") {
				t.Fatal("source verification must pass freshly without a registered contract witness")
			}
			out := &agent.StageOutput{ToolResults: []types.ToolResult{result}}
			return out, verifyPostHook(o, out)
		case types.StagePlan:
			planned++
			assertNativeRegistrationSourceTerminal(t, mu.WriteWorkflowRun(), f.source.ID)
			if mu.NativeTestRegistrationAuthorization() == nil || mu.ChangeReport() != nil {
				t.Fatal("source controller did not authorize isolated read-only planning")
			}
			if sourceVerified != nil {
				sourceVerified(o.loadWriteFinalReportChangeReport(f.source.ID), mu.WriteWorkflowRun())
			}
			ctx := &types.AgentContext{RepoRoot: o.busCtx.RepoRoot, MainRepoRoot: o.busCtx.MainRepoRoot, WorkDir: o.busCtx.WorkDir, Mode: types.ModeApply, Stage: stage, AgentName: types.AgentPlanner, Mutable: mu}
			out, err := planner.Execute(ctx, planningSkill)
			if err == nil {
				err = planPostHook(o, out)
			}
			return out, err
		default:
			t.Fatalf("unexpected mutation stage in terminal registration fixture: %s", stage)
		}
		return nil, nil
	}
	steps := 0
	var runErr error
	interrupted := false
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				if _, ok := recovered.(nativeRegistrationRestartBoundary); !ok {
					panic(recovered)
				}
				interrupted = true
			}
		}()
		runErr = o.runWriteControllerWorkflow(&steps)
	}()
	assertNativeRegistrationSourceTerminal(t, mu.WriteWorkflowRun(), f.source.ID)
	plan := mu.ChangePlan()
	// The controller may reverify the source before its cumulative-proof check.
	// Require actual verification, not a harness-imposed number of executions.
	if runErr != nil || planned != 1 || verified == 0 || !interrupted || !types.IsPersistedNativeTestRegistrationPlan(plan) || len(plan.Changes) != 0 || mu.NativeTestRegistrationAuthorization() != nil || mu.ChangeReport() != nil {
		t.Fatalf("source verify→registration did not stop at the persisted restart boundary: plans=%d verifies=%d interrupted=%t err=%v", planned, verified, interrupted, runErr)
	}
	retained, err := types.LoadChangePlanFromFile(filepath.Join(o.ensureChangeReportDir(), f.source.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := verificationSourcePlanDelivery(retained); !ok || retained.ID != f.source.ID {
		t.Fatal("planning replaced the retained source delivery")
	}
}
