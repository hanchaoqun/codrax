package orchestrator

import (
	"encoding/json"
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

type nativeRegistrationRestartBoundary struct{}

// Unlike the original restart negative control, the source batch starts before
// verification. Only the real controller may append its verification attempt
// and completion, and only typed proof routing may create the registration.
func TestNativeRegistrationSourceTerminalPublic(t *testing.T) {
	f := newControllerRegistrationFixture(t)
	o, mu := f.o, f.o.busCtx.Mutable
	mu.ResetChangeReport()
	o.reportDir = filepath.Dir(f.sourceRef)
	o.planPath, o.busCtx.PlanPath = "", ""
	run := &types.WriteWorkflowRun{RunID: "native-source-terminal", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "source", Batches: []types.WriteWorkflowBatch{{
		ID: "source", PlanID: f.source.ID, Status: types.WriteWorkflowBatchVerifying, ApplyRef: f.head,
		Attempts: []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: f.source.ID, ArtifactRef: f.head}},
	}}}
	identity := o.currentWriteWorkflowRepoIdentity(o.currentWriteWorkflowGoalHashSource())
	run.Identity = &identity
	store := repl.NewWriteWorkflowRunStore(o.ensureChangeReportDir())
	if _, err := store.Save(run); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(run.RunID); err != nil {
		t.Fatal(err)
	}
	mu.SetWriteWorkflowRun(run)
	o.writeWorkflowRunStore = store
	calls := 0
	ar, sr, _ := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			if types.IsPersistedNativeTestRegistrationPlan(ctx.Mutable.ChangePlan()) {
				// Stop after the scheduler persisted the accepted plan and typed
				// transition. No production error/terminal shortcut is injected.
				panic(nativeRegistrationRestartBoundary{})
			}
			calls++
			if calls > 12 {
				t.Fatal("controller did not converge to a registration")
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
	adapter := &controllerRegistrationLLM{t: t, entry: "emit_change_plan"}
	registry := tool.NewRegistry()
	registry.Register(&tool.ReadFile{})
	registry.Register(&tool.EmitChangePlan{})
	planner := agent.NewPlannerAgent(&agent.Dependencies{Tools: registry, LLM: adapter, MaxIterations: 2, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
	planned, verified := 0, 0
	o.readExplorationRunner = readExplorationRunnerFunc(func(o *Orchestrator) (int, error) {
		controllerRegistrationTool(t, o.busCtx, (&tool.ReadFile{}).Execute, map[string]any{"path": "value.py"})
		return 1, nil
	})
	o.controllerWriteStageFn = func(stage types.PipelineStage, steps *int) (*agent.StageOutput, error) {
		*steps++
		switch stage {
		case types.StageVerify:
			verified++
			result := controllerRegistrationTool(t, o.busCtx, (&tool.RunTests{}).Execute, map[string]any{})
			report := mu.ChangeReport()
			if report == nil || !report.Passed || report.PlanID != f.source.ID || len(report.ExecutedCommands) == 0 || types.BehaviorContractRefHasVerificationWitness(mu.ChangePlan(), report, "increment-result") {
				t.Fatal("source verification must be a real fresh pass without the later native declaration")
			}
			out := &agent.StageOutput{ToolResults: []types.ToolResult{result}}
			return out, verifyPostHook(o, out)
		case types.StagePlan:
			planned++
			assertNativeRegistrationSourceTerminal(t, mu.WriteWorkflowRun(), f.source.ID)
			if mu.NativeTestRegistrationAuthorization() == nil || mu.ChangeReport() != nil {
				t.Fatal("planner lacks fresh isolated registration authority")
			}
			ctx := &types.AgentContext{RepoRoot: o.busCtx.RepoRoot, MainRepoRoot: o.busCtx.MainRepoRoot, WorkDir: o.busCtx.WorkDir, Mode: types.ModeApply, Stage: stage, AgentName: types.AgentPlanner, Mutable: mu}
			out, err := planner.Execute(ctx, &skill.Config{ToolSuggestions: []string{"read_file", "emit_change_plan"}})
			if err == nil {
				err = planPostHook(o, out)
			}
			return out, err
		default:
			t.Fatalf("unexpected source mutation stage %s", stage)
		}
		return nil, nil
	}
	steps := 0
	var err error
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
		err = o.runWriteControllerWorkflow(&steps)
	}()
	current := mu.WriteWorkflowRun()
	assertNativeRegistrationSourceTerminal(t, current, f.source.ID)
	body, _ := json.MarshalIndent(current, "", "  ")
	if err != nil || planned != 1 || !interrupted {
		t.Fatalf("real source verification must route to native registration before finishing: plans=%d verifies=%d err=%v\n%s", planned, verified, err, body)
	}
	retained, err := types.LoadChangePlanFromFile(filepath.Join(o.ensureChangeReportDir(), f.source.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := verificationSourcePlanDelivery(retained); !ok || retained.ID != f.source.ID {
		t.Fatalf("source artifact was not retained across planning: %+v", retained)
	}
	f.restartExpectingSourceTerminal(t, true)
	// The successful registration only closes its own predecessor's proof
	// debt. An unrelated batch with no terminal verify verdict stays open.
	completed := mu.WriteWorkflowRun()
	completed.Batches = append(completed.Batches, types.WriteWorkflowBatch{ID: "other-delivery", Status: types.WriteWorkflowBatchComplete,
		Attempts: []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: "other-delivery"}},
	})
	writeflow.MarkWorkflowRunCompletionFromBatches(completed)
	if completed.Completion == nil || completed.Completion.Verdict != types.WriteWorkflowCompletionUnverified || completed.Completion.ReasonCode != "missing_terminal_verify_verdict" {
		t.Fatalf("registration broadcast a terminal verdict to unrelated source: %+v", completed.Completion)
	}
}

func assertNativeRegistrationSourceTerminal(t *testing.T, run *types.WriteWorkflowRun, sourceID string) {
	t.Helper()
	for _, batch := range run.Batches {
		if batch.PlanID != sourceID || proofFollowupPurpose(batch.Purpose) {
			continue
		}
		if batch.Completion == nil || batch.Completion.Verdict != types.WriteWorkflowCompletionVerified || batch.Completion.Source != "verify_attempt" || batch.VerifyRef == "" {
			t.Fatalf("source did not obtain its own real terminal verification: %+v", batch)
		}
		for _, attempt := range batch.Attempts {
			if attempt.Kind == "verify" && attempt.Status == "passed" && attempt.PlanID == sourceID && attempt.ReportID == batch.VerifyRef {
				return
			}
		}
		t.Fatalf("source completion lacks its own matching verify attempt: %+v", batch)
	}
	t.Fatal("source batch disappeared")
}
