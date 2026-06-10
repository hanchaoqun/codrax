package orchestrator

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

type fakeWorkflowRunStore struct {
	saveCount int
	last      *types.WriteWorkflowRun
}

func (s *fakeWorkflowRunStore) Save(run *types.WriteWorkflowRun) (string, error) {
	s.saveCount++
	if run != nil {
		cp := types.CloneWriteWorkflowRun(*run)
		s.last = &cp
	}
	return "/tmp/" + run.RunID + ".json", nil
}

func TestRunWriteControllerWorkflow_ExplorePlanFinish(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("patch planner from exploration")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task:             types.WriteTask{Kind: types.WriteTaskFeature, Scope: types.ScopePackage, Summary: "patch planner"},
			Risk:             types.WriteRiskProfile{Overall: types.RiskBandMedium},
			ExpectedOutcomes: []string{"planner consumes exploration"},
		},
	})
	decisions := []writeflow.WriteWorkflowDecision{
		{
			Action:     writeflow.ActionExploreCode,
			ReasonCode: "need_context",
			ExplorationRequest: &types.WriteExplorationRequest{
				BatchID:              "batch-1",
				Goal:                 "inspect planner prompt",
				ExplorationQuestions: []string{"where is planner context rendered?"},
				CandidatePaths:       []string{"internal/agent/planner.go"},
			},
		},
		{
			Action:     writeflow.ActionPlanChangeBatch,
			ReasonCode: "context_ready",
			Batch: &writeflow.WriteBatchPlan{
				ID:            "batch-1",
				Goal:          "patch planner prompt",
				ExpectedPaths: []string{"internal/agent/planner.go"},
			},
		},
		{Action: writeflow.ActionFinish, ReasonCode: "done"},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
				ReadFiles: []string{"internal/agent/planner.go"},
				EvidenceItems: []types.EvidenceItem{{
					ID:              "ev-planner",
					Kind:            types.EvidenceMechanism,
					Subject:         "planner context",
					Source:          "internal/agent/planner.go",
					LineStart:       140,
					AnchorSymbol:    "BuildInitialInstruction",
					Summary:         "planner renders write handoff",
					GroundingStatus: types.GroundingRecovered,
				}},
			})
			return &agent.StageOutput{}, nil
		},
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	o.runTaskPhaseFn = func(stepsUsed *int) error {
		if handoff := mu.WriteExplorationHandoff(); handoff == nil || len(handoff.EvidenceRefs) != 1 {
			t.Fatalf("inner batch should see exploration handoff, got %+v", handoff)
		}
		mu.SetChangePlan(&types.ChangePlan{
			ID:          "plan-controller-1",
			Status:      types.PlanStatusApplied,
			Summary:     "patch planner",
			TargetPaths: []string{"internal/agent/planner.go"},
		})
		mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-controller-1", Passed: true})
		*stepsUsed++
		return nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if controllerCalls != 3 {
		t.Fatalf("controller calls = %d, want 3", controllerCalls)
	}
	if store.last == nil || store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("workflow should complete, got %+v", store.last)
	}
	if len(store.last.ContextPacks) == 0 {
		t.Fatalf("workflow should persist context packs: %+v", store.last)
	}
	if got := store.last.Batches[0]; got.PlanID != "plan-controller-1" || got.Status != types.WriteWorkflowBatchComplete {
		t.Fatalf("batch not marked complete with plan id: %+v", got)
	}
}

func TestRunWriteControllerWorkflow_AppendsFollowupBatch(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("two batch change")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "two batch change"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanChangeBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "first bounded change"}},
		{Action: writeflow.ActionPlanChangeBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-2", Goal: "second bounded change", DependsOn: []string{"batch-1"}}},
		{Action: writeflow.ActionFinish, ReasonCode: "done"},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	planN := 0
	o.runTaskPhaseFn = func(stepsUsed *int) error {
		planN++
		planID := fmt.Sprintf("plan-%d", planN)
		mu.SetChangePlan(&types.ChangePlan{ID: planID, Status: types.PlanStatusApplied, Summary: planID, TargetPaths: []string{fmt.Sprintf("f%d.go", planN)}})
		mu.SetChangeReport(&types.ChangeReport{PlanID: planID, Passed: true})
		*stepsUsed++
		return nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if len(store.last.Batches) != 2 {
		t.Fatalf("expected two batches, got %+v", store.last.Batches)
	}
	for i, batch := range store.last.Batches {
		if batch.Status != types.WriteWorkflowBatchComplete || batch.PlanID == "" {
			t.Fatalf("batch %d not complete with plan: %+v", i, batch)
		}
	}
}

func TestRunWriteControllerWorkflow_VerifyFailureCanReplanSameBatch(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("repair failing verify")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "repair failing verify"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanChangeBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "first attempt"}},
		{Action: writeflow.ActionPlanChangeBatch, ReasonCode: "repair_after_verify", Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "repair attempt"}},
		{Action: writeflow.ActionFinish, ReasonCode: "done"},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	attempt := 0
	o.runTaskPhaseFn = func(stepsUsed *int) error {
		attempt++
		planID := fmt.Sprintf("plan-attempt-%d", attempt)
		mu.SetChangePlan(&types.ChangePlan{ID: planID, Status: types.PlanStatusApplied, Summary: planID, TargetPaths: []string{"fix.go"}})
		if attempt == 1 {
			mu.SetChangeReport(&types.ChangeReport{PlanID: planID, Passed: false, FailureSummary: "unit failed"})
			*stepsUsed++
			return fmt.Errorf("verify failed")
		}
		mu.SetChangeReport(&types.ChangeReport{PlanID: planID, Passed: true})
		*stepsUsed++
		return nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if attempt != 2 {
		t.Fatalf("expected two inner attempts, got %d", attempt)
	}
	if store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("workflow should complete after repair: %+v", store.last)
	}
	if len(store.last.ProgressLedger) == 0 || !workflowProgressHasReason(store.last.ProgressLedger, "inner_batch_failed") {
		t.Fatalf("verify failure should be recorded in progress ledger: %+v", store.last.ProgressLedger)
	}
}

func scriptedController(t *testing.T, decisions []writeflow.WriteWorkflowDecision, calls *int) func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
	t.Helper()
	return func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
		if *calls >= len(decisions) {
			t.Fatalf("unexpected extra controller call %d", *calls+1)
		}
		decision := writeflow.NormalizeWriteWorkflowDecision(decisions[*calls])
		raw, err := json.Marshal(decision)
		if err != nil {
			t.Fatalf("marshal decision: %v", err)
		}
		ctx.Mutable.SetWriteWorkflowDecisionJSON(raw)
		(*calls)++
		return &agent.StageOutput{Data: raw}, nil
	}
}

func workflowProgressHasReason(items []types.WriteWorkflowProgress, reason string) bool {
	for _, item := range items {
		if item.ReasonCode == reason {
			return true
		}
	}
	return false
}
