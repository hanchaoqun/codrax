package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// This adapter uses the real controller agent, default skill and emit tool.
// max_batches is deliberately an ordinary model proposal, not host policy.
type hmc292BudgetController struct {
	*controllerRegistrationLLM
	calls   int
	prompts []string
}

func TestHMC292BudgetExplicitOneRejectsBeforePersistingFollowup(t *testing.T) {
	f := newControllerRegistrationFixture(t)
	o, mu := f.o, f.o.busCtx.Mutable
	mu.ResetChangeReport()
	o.reportDir = filepath.Dir(f.sourceRef)
	o.planPath, o.busCtx.PlanPath = "", ""
	run := &types.WriteWorkflowRun{RunID: "capped-source", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "source", Budget: types.WriteWorkflowBudget{MaxBatches: 1}, Batches: []types.WriteWorkflowBatch{{ID: "source", PlanID: f.source.ID, Status: types.WriteWorkflowBatchVerifying, ApplyRef: f.head, Attempts: []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: f.source.ID, ArtifactRef: f.head}}}}}
	identity := o.currentWriteWorkflowRepoIdentity(o.currentWriteWorkflowGoalHashSource())
	run.Identity = &identity
	store := repl.NewWriteWorkflowRunStore(o.ensureChangeReportDir())
	if _, err := store.Save(run); err != nil {
		t.Fatal(err)
	}
	mu.SetWriteWorkflowRun(run)
	o.writeWorkflowRunStore = store
	calls, verifies := 0, 0
	var beforeReject *types.WriteWorkflowRun
	var beforeDiskBatches []byte
	var reportBytes []byte
	ar, sr, _ := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){types.AgentWriteController: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
		calls++
		if calls > 2 {
			t.Fatal("capped run continued after rejection")
		}
		action := writeflow.ActionVerifyBatch
		if calls == 2 {
			action = writeflow.ActionFinish
			beforeReject = mu.WriteWorkflowRun()
			durable, err := store.Load(run.RunID)
			if err != nil {
				t.Fatal(err)
			}
			beforeDiskBatches, _ = json.Marshal(durable.Batches)
			reportBytes, _ = json.Marshal(mu.ChangeReport())
		}
		result := controllerRegistrationTool(t, o.busCtx, (&tool.EmitWriteWorkflowDecision{}).Execute, map[string]any{"action": action, "workflow": map[string]any{"max_batches": 99}})
		return &agent.StageOutput{ToolResults: []types.ToolResult{result}}, nil
	}})
	o.agents, o.skills = ar, sr
	o.controllerWriteStageFn = func(stage types.PipelineStage, steps *int) (*agent.StageOutput, error) {
		*steps++
		if stage != types.StageVerify {
			t.Fatalf("capped run admitted stage %s", stage)
		}
		verifies++
		result := controllerRegistrationTool(t, o.busCtx, (&tool.RunTests{}).Execute, map[string]any{})
		out := &agent.StageOutput{ToolResults: []types.ToolResult{result}}
		return out, verifyPostHook(o, out)
	}
	steps := 0
	err := o.runWriteControllerWorkflow(&steps)
	var budgetErr *writeflow.BatchBudgetExceededError
	if !errors.As(err, &budgetErr) || budgetErr.Used != 1 || budgetErr.Limit != 1 || verifies != 1 {
		t.Fatalf("explicit budget must reject with typed admission error after real source verify: %v verifies=%d", err, verifies)
	}
	got := mu.WriteWorkflowRun()
	assertNativeRegistrationSourceTerminal(t, got, f.source.ID)
	if got.Status != types.WriteWorkflowRunBlocked || got.ActiveBatchID != "source" || len(got.Batches) != 1 || got.Budget.MaxBatches != 1 || got.Budget.BatchesUsed != 1 {
		t.Fatalf("unstarted batch persisted: %+v", got)
	}
	if beforeReject == nil || !reflect.DeepEqual(got.Batches, beforeReject.Batches) || !reflect.DeepEqual(got.Edges, beforeReject.Edges) || len(got.ProgressLedger) != len(beforeReject.ProgressLedger)+1 || !reflect.DeepEqual(got.ProgressLedger[:len(beforeReject.ProgressLedger)], beforeReject.ProgressLedger) {
		t.Fatalf("rejection changed prior batch/attempt/progress: before=%+v after=%+v", beforeReject, got)
	}
	if got.ProgressLedger[len(got.ProgressLedger)-1].ReasonCode != "max_batches_reached" {
		t.Fatal("missing honest rejection")
	}
	report := mu.ChangeReport()
	afterReport, _ := json.Marshal(report)
	if !reflect.DeepEqual(reportBytes, afterReport) || report == nil || !report.Passed || types.BehaviorContractRefHasVerificationWitness(f.source, report, "increment-result") || mu.NativeTestRegistrationAuthorization() != nil {
		t.Fatal("rejection changed source report or invented missing contract authority")
	}
	retained, loadErr := store.Load(got.RunID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	// The store adds content-addressed context refs to its durable copy. Check
	// the complete persisted batch before/after rejection separately from the
	// exact in-memory batch comparison above; neither representation may drift.
	diskBatches, _ := json.Marshal(retained.Batches)
	if string(diskBatches) != string(beforeDiskBatches) || retained.ActiveBatchID != "source" || retained.Status != types.WriteWorkflowRunBlocked {
		t.Fatalf("persisted roster changed on rejection: before=%s after=%s active=%s status=%s", beforeDiskBatches, diskBatches, retained.ActiveBatchID, retained.Status)
	}
}

func TestHMC292BudgetCumulativeFollowupRejectsBeforePublishingContext(t *testing.T) {
	testCumulativePatchReviewFollowupCoversSourcePatch(t, 1)
}

func (a *hmc292BudgetController) Chat(_ context.Context, messages []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	a.calls++
	var prompt strings.Builder
	for _, m := range messages {
		prompt.WriteString(m.Content)
	}
	a.prompts = append(a.prompts, prompt.String())
	action := "finish"
	if a.calls == 1 {
		action = "verify_batch"
	}
	if a.calls > 12 {
		return llm.Response{}, fmt.Errorf("controller did not converge")
	}
	params, _ := json.Marshal(map[string]any{"action": action, "workflow": map[string]any{"max_batches": 1}})
	return llm.Response{ToolCalls: []llm.ToolCall{{ID: fmt.Sprintf("budget-%d", a.calls), Name: "emit_write_workflow_decision", Params: params}}}, nil
}

func TestHMC292BudgetRestoredControllerUsesHostPolicyBeforeModel(t *testing.T) {
	f := newControllerRegistrationFixture(t)
	registry := tool.NewRegistry()
	registry.Register(&tool.ReadFile{})
	registry.Register(&tool.EmitChangePlan{})
	plannerAdapter := &controllerRegistrationLLM{t: t, entry: "emit_change_plan"}
	planner := agent.NewPlannerAgent(&agent.Dependencies{Tools: registry, LLM: plannerAdapter, MaxIterations: 12, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
	skills := skill.NewRegistry()
	skill.RegisterDefaults(skills)
	planning, err := skills.Get("change-plan-skill")
	if err != nil {
		t.Fatal(err)
	}
	adapter := &hmc292BudgetController{controllerRegistrationLLM: &controllerRegistrationLLM{t: t}}
	runNativeRegistrationTerminalPlanning(t, f, planner, planning, nil, nativeRegistrationControllerAgent(adapter))
	run := f.o.busCtx.Mutable.WriteWorkflowRun()
	if len(adapter.prompts) < 2 || !strings.Contains(adapter.prompts[0], "budget: batches 1/5") || run.Budget.MaxBatches != defaultWriteWorkflowMaxBatches {
		t.Fatalf("host default/roster not present before model decision: calls=%d budget=%+v", adapter.calls, run.Budget)
	}
	if plannerAdapter.round != 2 || !types.IsPersistedNativeTestRegistrationPlan(f.o.busCtx.Mutable.ChangePlan()) {
		t.Fatal("source verification did not reach read-only registration through real controller/planner")
	}
}
