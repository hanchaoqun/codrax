package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

type fakeWorkflowRunStore struct {
	saveCount int
	last      *types.WriteWorkflowRun
	active    *types.WriteWorkflowRun
}

func (s *fakeWorkflowRunStore) Save(run *types.WriteWorkflowRun) (string, error) {
	s.saveCount++
	if run != nil {
		cp := types.CloneWriteWorkflowRun(*run)
		s.last = &cp
	}
	return "/tmp/" + run.RunID + ".json", nil
}

func (s *fakeWorkflowRunStore) FindActiveRun() (*types.WriteWorkflowRun, error) {
	if s.active == nil {
		return nil, nil
	}
	cp := types.CloneWriteWorkflowRun(*s.active)
	return &cp, nil
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
			Action:     writeflow.ActionPlanBatch,
			ReasonCode: "context_ready",
			Batch: &writeflow.WriteBatchPlan{
				ID:            "batch-1",
				Goal:          "patch planner prompt",
				ExpectedPaths: []string{"internal/agent/planner.go"},
			},
		},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "plan_ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "applied"},
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
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			if handoff := mu.WriteExplorationHandoff(); handoff == nil || len(handoff.EvidenceRefs) != 1 {
				t.Fatalf("plan batch should see exploration handoff, got %+v", handoff)
			}
			mu.SetChangePlan(&types.ChangePlan{
				ID:          "plan-controller-1",
				Status:      types.PlanStatusPending,
				Summary:     "patch planner",
				TargetPaths: []string{"internal/agent/planner.go"},
			})
		case types.StageVerify:
			mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-controller-1", Passed: true})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if controllerCalls != 5 {
		t.Fatalf("controller calls = %d, want 5", controllerCalls)
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
	got := store.last.Batches[0]
	if got.ApplyRef != "refs/codrax/applied/plan-controller-1" || got.VerifyRef != "plan-controller-1.report.json" {
		t.Fatalf("batch refs should track apply and verify artifacts: %+v", got)
	}
	if !workflowBatchHasAttemptArtifact(got, "plan", "complete", "", "plan-controller-1") ||
		!workflowBatchHasAttemptArtifact(got, "apply", "applied", "apply_succeeded", "refs/codrax/applied/plan-controller-1") ||
		!workflowBatchHasAttemptArtifact(got, "verify", "passed", "tests_passed", "plan-controller-1.report.json") {
		t.Fatalf("batch attempts should track plan/apply/verify artifact refs: %+v", got.Attempts)
	}
	if plan := mu.ChangePlan(); plan == nil || plan.Status != types.PlanStatusApplied || plan.AppliedAt == nil {
		t.Fatalf("verified workflow should synchronize mutable plan status to applied, got %+v", plan)
	}
}

func TestRunWriteControllerWorkflow_AppendsFollowupBatch(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("two batch change")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "two batch change"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "first bounded change"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "first_ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "first_applied"},
		{Action: writeflow.ActionAppendBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-2", Goal: "second bounded change", DependsOn: []string{"batch-1"}}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "second_ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "second_applied"},
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
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			planN++
			planID := fmt.Sprintf("plan-%d", planN)
			mu.SetChangePlan(&types.ChangePlan{ID: planID, Status: types.PlanStatusPending, Summary: planID, TargetPaths: []string{fmt.Sprintf("f%d.go", planN)}})
		case types.StageVerify:
			if plan := mu.ChangePlan(); plan != nil {
				mu.SetChangeReport(&types.ChangeReport{PlanID: plan.ID, Passed: true})
			}
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
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

func TestRunWriteControllerWorkflow_ModePlanStopsAfterPlan(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("plan only")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "plan only"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "produce reviewable plan"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "must_not_be_reached"},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModePlan, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		if stage != types.StagePlan {
			t.Fatalf("ModePlan should not dispatch %s after plan_batch", stage)
		}
		mu.SetChangePlan(&types.ChangePlan{
			ID:          "plan-mode-only",
			Status:      types.PlanStatusPending,
			Summary:     "review me",
			TargetPaths: []string{"fix.go"},
		})
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if controllerCalls != 1 {
		t.Fatalf("ModePlan should stop after the first plan decision, controller calls=%d", controllerCalls)
	}
	if store.last == nil || store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("ModePlan workflow should complete after plan, got %+v", store.last)
	}
	if len(store.last.Batches) != 1 || store.last.Batches[0].Status != types.WriteWorkflowBatchPlanned {
		t.Fatalf("batch should remain reviewable/planned, got %+v", store.last.Batches)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "plan_mode_complete") {
		t.Fatalf("plan_mode_complete progress should be recorded: %+v", store.last.ProgressLedger)
	}
	if workflowProgressHasReason(store.last.ProgressLedger, "apply_not_allowed_in_plan_mode") {
		t.Fatalf("ModePlan should not reach apply blocking path: %+v", store.last.ProgressLedger)
	}
}

func TestRunWriteControllerWorkflow_VerifyFailureCanReplanSameBatch(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("repair failing verify")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "repair failing verify"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "first attempt"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "first_ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "first_applied"},
		{Action: writeflow.ActionReplanBatch, ReasonCode: "repair_after_verify", Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "repair attempt"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "repair_ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "repair_applied"},
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
	o.SetWriteRetryBudget(1)
	attempt := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			attempt++
			planID := fmt.Sprintf("plan-attempt-%d", attempt)
			mu.SetChangePlan(&types.ChangePlan{ID: planID, Status: types.PlanStatusPending, Summary: planID, TargetPaths: []string{"fix.go"}})
		case types.StageVerify:
			planID := ""
			if plan := mu.ChangePlan(); plan != nil {
				planID = plan.ID
			}
			if attempt == 1 {
				mu.SetChangeReport(&types.ChangeReport{
					PlanID:                planID,
					Passed:                false,
					BuildFailed:           true,
					FailureSummary:        "compile failed",
					FailureSummaryBlobRef: "/tmp/codrax/blob/compile-failed.txt",
					TestResults: []types.TestResult{{
						Kind:          types.TestResultKindBuildError,
						AssertionID:   "fix.go:12",
						Suite:         "build",
						FailureDetail: "undefined: helper",
						BuildErrors: []types.BuildError{{
							File:    "fix.go",
							Line:    12,
							Message: "undefined: helper",
						}},
					}},
				})
				*stepsUsed++
				return &agent.StageOutput{Error: "verify failed"}, nil
			}
			mu.SetChangeReport(&types.ChangeReport{PlanID: planID, Passed: true})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
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
	if len(store.last.ProgressLedger) == 0 || !workflowProgressHasReason(store.last.ProgressLedger, "verify_failed") {
		t.Fatalf("verify failure should be recorded in progress ledger: %+v", store.last.ProgressLedger)
	}
	if len(store.last.Batches) != 1 {
		t.Fatalf("expected one batch, got %+v", store.last.Batches)
	}
	batch := store.last.Batches[0]
	if batch.VerifyRef != "plan-attempt-2.report.json" {
		t.Fatalf("latest verify ref = %q, want final report ref", batch.VerifyRef)
	}
	if !workflowBatchHasAttempt(batch, "verify", "failed", "build_failed", "plan-attempt-1.report.json") ||
		!workflowBatchHasAttempt(batch, "verify", "passed", "tests_passed", "plan-attempt-2.report.json") {
		t.Fatalf("verify attempts should retain failed and passed report refs: %+v", batch.Attempts)
	}
	if !workflowRunContextContains(store.last, "build_error", "fix.go:12 undefined: helper") ||
		!workflowRunContextContains(store.last, "failure_summary_blob_ref", "/tmp/codrax/blob/compile-failed.txt") {
		t.Fatalf("verify failure context should retain typed build evidence: %+v", store.last.ContextPacks)
	}
}

func TestRunWriteControllerWorkflow_ApplyErrorDoesNotBecomePendingApprovalWithoutRecord(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("apply fails structurally")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "apply fails structurally"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "make change"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "plan_ready"},
		{Action: writeflow.ActionBlock, ReasonCode: "apply_failed"},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			mu.SetChangePlan(&types.ChangePlan{
				ID:          "plan-apply-error",
				Status:      types.PlanStatusPending,
				Summary:     "ordinary apply error",
				TargetPaths: []string{"fix.go"},
			})
		case types.StageApply:
			*stepsUsed++
			return nil, fmt.Errorf("patch did not apply")
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err == nil {
		t.Fatal("workflow should return the controller block after apply failure")
	}
	if controllerCalls != 3 {
		t.Fatalf("controller should receive a turn after ordinary apply failure, calls=%d", controllerCalls)
	}
	if store.last == nil || len(store.last.Batches) != 1 {
		t.Fatalf("workflow should persist failed apply batch: %+v", store.last)
	}
	batch := store.last.Batches[0]
	if batch.Status == types.WriteWorkflowBatchPendingApproval {
		t.Fatalf("ordinary apply error must not become pending approval: %+v", batch)
	}
	if !workflowBatchHasAttemptArtifact(batch, "apply", "failed", "apply_error", "") {
		t.Fatalf("ordinary apply error should be recorded as apply_error attempt: %+v", batch.Attempts)
	}
	if workflowProgressHasReason(store.last.ProgressLedger, string(types.WriteWorkflowBatchPendingApproval)) {
		t.Fatalf("ordinary apply error should not record pending approval progress: %+v", store.last.ProgressLedger)
	}
}

func TestRunWriteControllerWorkflow_VerifyFailureCanReexploreThenReplan(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("repair with fresh exploration")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "repair with fresh exploration"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "first attempt"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "first_ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "first_applied"},
		{
			Action:     writeflow.ActionExploreCode,
			ReasonCode: "need_failure_context",
			ExplorationRequest: &types.WriteExplorationRequest{
				BatchID:              "batch-1",
				Goal:                 "inspect failing test surface",
				ExplorationQuestions: []string{"why did verify fail?"},
				CandidatePaths:       []string{"fix.go"},
			},
		},
		{Action: writeflow.ActionReplanBatch, ReasonCode: "fresh_context_ready", Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "repair after exploration"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "repair_ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "repair_applied"},
		{Action: writeflow.ActionFinish, ReasonCode: "done"},
	}
	controllerCalls := 0
	explorerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
				ReadFiles: []string{"fix.go"},
				EvidenceItems: []types.EvidenceItem{{
					ID:              "ev-after-verify-failure",
					Kind:            types.EvidenceMechanism,
					Subject:         "failing assertion",
					Source:          "fix.go",
					LineStart:       42,
					Summary:         "fresh exploration found the failed branch",
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
	o.SetWriteRetryBudget(1)
	attempt := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			attempt++
			planID := fmt.Sprintf("plan-reexplore-%d", attempt)
			mu.SetChangePlan(&types.ChangePlan{ID: planID, Status: types.PlanStatusPending, Summary: planID, TargetPaths: []string{"fix.go"}})
			if attempt == 2 {
				handoff := mu.WriteExplorationHandoff()
				if handoff == nil || len(handoff.EvidenceRefs) != 1 || handoff.EvidenceRefs[0].Source != "fix.go" {
					t.Fatalf("second plan should consume fresh exploration handoff, got %+v", handoff)
				}
			}
		case types.StageVerify:
			planID := ""
			if plan := mu.ChangePlan(); plan != nil {
				planID = plan.ID
			}
			if attempt == 1 {
				mu.SetChangeReport(&types.ChangeReport{PlanID: planID, Passed: false, FailureSummary: "assertion failed before exploration"})
				*stepsUsed++
				return &agent.StageOutput{Error: "verify failed"}, nil
			}
			mu.SetChangeReport(&types.ChangeReport{PlanID: planID, Passed: true})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if explorerCalls != 1 {
		t.Fatalf("expected one re-exploration call, got %d", explorerCalls)
	}
	if attempt != 2 {
		t.Fatalf("expected two inner attempts, got %d", attempt)
	}
	if store.last == nil || store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("workflow should complete after re-explore/replan: %+v", store.last)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "verify_failed") ||
		!workflowProgressHasReason(store.last.ProgressLedger, "exploration_complete") {
		t.Fatalf("failure and re-exploration progress should both persist: %+v", store.last.ProgressLedger)
	}
	if len(store.last.ContextPacks) == 0 {
		t.Fatalf("workflow should retain handoff context packs: %+v", store.last)
	}
}

func TestRunWriteControllerWorkflow_PendingApprovalKeepsRunActive(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("requires approval")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "requires approval"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "high risk batch"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "must_not_be_reached"},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		if stage == types.StagePlan {
			plan := &types.ChangePlan{
				ID:          "plan-needs-approval",
				Status:      types.PlanStatusPending,
				Summary:     "manual gate",
				TargetPaths: []string{"go.mod"},
			}
			plan.Approval = &types.WriteApprovalRecord{
				Policy:          string(writeflow.ApprovalPolicyAutoSafe),
				RiskLevel:       string(writeflow.RiskHigh),
				Action:          string(writeflow.ApprovalActionManual),
				UserDecision:    "required",
				ReasonCode:      "high_write_risk",
				PlanFingerprint: types.PlanFingerprint(plan),
			}
			mu.SetChangePlan(plan)
			*stepsUsed++
			return &agent.StageOutput{}, nil
		}
		if stage == types.StageApply {
			t.Fatalf("pending approval should stop before apply stage")
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err == nil {
		t.Fatal("pending approval should return the apply-pre gate error to the caller")
	}
	if store.last == nil {
		t.Fatal("workflow run should be persisted")
	}
	if controllerCalls != 1 {
		t.Fatalf("pending approval should be decided from the typed plan approval record before another controller turn; calls=%d", controllerCalls)
	}
	if store.last.Status != types.WriteWorkflowRunInProgress {
		t.Fatalf("pending approval must keep workflow active, got %+v", store.last)
	}
	if len(store.last.Batches) != 1 || store.last.Batches[0].Status != types.WriteWorkflowBatchPendingApproval {
		t.Fatalf("active batch should await approval: %+v", store.last.Batches)
	}
	if store.last.Batches[0].PlanID != "plan-needs-approval" {
		t.Fatalf("pending batch should keep plan id: %+v", store.last.Batches[0])
	}
}

func TestRunWriteControllerWorkflow_ResumesActiveRun(t *testing.T) {
	store := &fakeWorkflowRunStore{active: &types.WriteWorkflowRun{
		RunID:         "wf-active",
		Goal:          "resume me",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-9",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-9",
			Goal:   "existing batch",
			Status: types.WriteWorkflowBatchPendingApproval,
		}},
	}}
	mu := types.NewMutableState("new request should resume active run")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "new seed"}}})
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, []writeflow.WriteWorkflowDecision{
			{Action: writeflow.ActionFinish, ReasonCode: "done"},
		}, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if store.last == nil || store.last.RunID != "wf-active" {
		t.Fatalf("workflow should resume active run, got %+v", store.last)
	}
	if store.last.ActiveBatchID != "batch-9" || len(store.last.ProgressLedger) == 0 ||
		!workflowProgressHasReason(store.last.ProgressLedger, "workflow_resumed") {
		t.Fatalf("resume progress should be persisted: %+v", store.last)
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

func workflowBatchHasAttempt(batch types.WriteWorkflowBatch, kind, status, reasonCode, reportID string) bool {
	for _, attempt := range batch.Attempts {
		if attempt.Kind == kind && attempt.Status == status && attempt.ReasonCode == reasonCode && attempt.ReportID == reportID {
			return true
		}
	}
	return false
}

func workflowBatchHasAttemptArtifact(batch types.WriteWorkflowBatch, kind, status, reasonCode, artifactRef string) bool {
	for _, attempt := range batch.Attempts {
		if attempt.Kind == kind && attempt.Status == status && attempt.ReasonCode == reasonCode && attempt.ArtifactRef == artifactRef {
			return true
		}
	}
	return false
}

func workflowRunContextContains(run *types.WriteWorkflowRun, kind, substring string) bool {
	if run == nil {
		return false
	}
	for _, pack := range run.ContextPacks {
		for _, item := range pack.Items {
			if item.Kind == kind && strings.Contains(item.Text, substring) {
				return true
			}
		}
	}
	return false
}

func TestRunWriteControllerWorkflow_FinishGateRequiresTypedDisposition(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("finish after failed verify")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "finish gate"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "one attempt"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "applied"},
		// Bare finish after a failed verify must be rejected by the typed gate…
		{Action: writeflow.ActionFinish, ReasonCode: "premature"},
		// …and the schema-validated escape lane completes the run with a caveat.
		{Action: writeflow.ActionFinish, ReasonCode: "accepted", FinishDisposition: writeflow.FinishDispositionAcceptUnverified},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	o.SetWriteRetryBudget(3)
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-finish-gate", Status: types.PlanStatusPending, Summary: "attempt", TargetPaths: []string{"fix.go"}})
		case types.StageVerify:
			mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-finish-gate", Passed: false, FailureSummary: "red tests"})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if controllerCalls != 5 {
		t.Fatalf("controller calls = %d, want 5 (finish rejected once, accepted once)", controllerCalls)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "finish_rejected_failed_verify") {
		t.Fatalf("typed finish rejection should be recorded: %+v", store.last.ProgressLedger)
	}
	if store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("accept_unverified finish should complete the run, got %+v", store.last.Status)
	}
	result := mu.Result()
	if !strings.Contains(result, "accepted with failed verification") || !strings.Contains(result, "batch-1") {
		t.Fatalf("result should carry the typed unverified caveat, got %q", result)
	}
}

func TestRunWriteControllerWorkflow_FinishAfterPassedVerifyNeedsNoDisposition(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("clean finish")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "clean finish"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "single change"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "applied"},
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
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-clean", Status: types.PlanStatusPending, Summary: "attempt", TargetPaths: []string{"fix.go"}})
		case types.StageVerify:
			mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-clean", Passed: true})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if workflowProgressHasReason(store.last.ProgressLedger, "finish_rejected_failed_verify") {
		t.Fatalf("clean finish must not trip the gate: %+v", store.last.ProgressLedger)
	}
	if store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("run should complete, got %+v", store.last.Status)
	}
}

func TestRunWriteControllerWorkflow_VerifyFailureSetsHandoffAndGreenClears(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("handoff lifecycle")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "handoff lifecycle"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "first attempt"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "applied"},
		{Action: writeflow.ActionReplanBatch, ReasonCode: "repair", Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "repair"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "repair_ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "repair_applied"},
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
	o.SetWriteRetryBudget(2)
	attempt := 0
	var handoffSeenAtReplan *types.VerifyFailureHandoff
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			attempt++
			if attempt == 2 {
				handoffSeenAtReplan = mu.VerifyFailureHandoff()
			}
			planID := fmt.Sprintf("plan-%d", attempt)
			mu.SetChangePlan(&types.ChangePlan{ID: planID, Status: types.PlanStatusPending, Summary: planID, TargetPaths: []string{"fix.go"}})
		case types.StageVerify:
			plan := mu.ChangePlan()
			if attempt == 1 {
				mu.SetChangeReport(&types.ChangeReport{
					PlanID:         plan.ID,
					Passed:         false,
					FailureKind:    types.FailureKindTestsFailed,
					FailureSummary: "TestA failed",
					TestResults:    []types.TestResult{{AssertionID: "TestA", Passed: false, FailureDetail: "want 2 got 3"}},
				})
			} else {
				mu.SetChangeReport(&types.ChangeReport{PlanID: plan.ID, Passed: true})
			}
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if handoffSeenAtReplan == nil {
		t.Fatal("replan planning round must see the typed verify-failure handoff (it survives the planning-state reset)")
	}
	if handoffSeenAtReplan.PlanID != "plan-1" || len(handoffSeenAtReplan.FailingTests) != 1 {
		t.Fatalf("handoff should carry the failing plan's typed rows: %+v", handoffSeenAtReplan)
	}
	if got := mu.VerifyFailureHandoff(); got != nil {
		t.Fatalf("green verify must clear the handoff, got %+v", got)
	}
}

func TestRunControllerPlanBatch_NoPlanReplanRoundGetsOneRetry(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("no plan retry")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "no plan retry"}}})
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	mu.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{PlanID: "plan-prev", BatchID: "batch-1", Attempt: 1})
	planCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		if stage != types.StagePlan {
			t.Fatalf("unexpected stage %s", stage)
		}
		planCalls++
		*stepsUsed++
		if planCalls == 1 {
			// First round: no plan emitted — the planner-style terminal error.
			return &agent.StageOutput{Error: "no change plan was produced this round"}, nil
		}
		mu.SetChangePlan(&types.ChangePlan{ID: "plan-retry", Status: types.PlanStatusPending, Summary: "repair", TargetPaths: []string{"fix.go"}})
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runControllerPlanBatch(&writeflow.WriteBatchPlan{ID: "batch-1", Goal: "repair"}, &steps); err != nil {
		t.Fatalf("one empty round with handoff active must be retried, got %v", err)
	}
	if planCalls != 2 {
		t.Fatalf("plan stage calls = %d, want 2 (one empty + one retry)", planCalls)
	}
	if !strings.Contains(mu.PlanningHint(), "previous planning round ended without a stored change plan") {
		t.Fatalf("retry hint must cite the empty round, got %q", mu.PlanningHint())
	}
	if mu.ChangePlan() == nil {
		t.Fatal("retry round should have installed the plan")
	}
}

func TestRunControllerPlanBatch_NoPlanWithoutHandoffStaysTerminal(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("no plan terminal")
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	planCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		planCalls++
		*stepsUsed++
		return &agent.StageOutput{Error: "no change plan was produced this round"}, nil
	}
	steps := 0
	if err := o.runControllerPlanBatch(&writeflow.WriteBatchPlan{ID: "batch-1", Goal: "first"}, &steps); err == nil {
		t.Fatal("first-attempt empty round without handoff must stay terminal")
	}
	if planCalls != 1 {
		t.Fatalf("plan stage calls = %d, want 1 (no retry without typed failure context)", planCalls)
	}
}

func TestSeedWriteWorkflowRun_MicroScopeStartsReadyToPlan(t *testing.T) {
	mu := types.NewMutableState("micro short path")
	ir := &types.WriteAnalysisIR{}
	ir.Request.Task = types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopeMicro, Summary: "fix literal"}
	ir.Request.ScopeAnchors = []string{"src/util.c"}
	mu.SetWriteAnalysisIR(ir)
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	run := o.seedWriteWorkflowRun()
	if len(run.Batches) != 1 {
		t.Fatalf("expected one seeded batch, got %+v", run.Batches)
	}
	if run.Batches[0].Status != types.WriteWorkflowBatchReadyToPlan {
		t.Fatalf("micro-scope seed should start ready_to_plan, got %s", run.Batches[0].Status)
	}
}

func TestSeedWriteWorkflowRun_NonMicroStartsNeedsExploration(t *testing.T) {
	mu := types.NewMutableState("regular seed")
	ir := &types.WriteAnalysisIR{}
	ir.Request.Task = types.WriteTask{Kind: types.WriteTaskFeature, Scope: types.ScopePackage, Summary: "wider"}
	ir.Request.ScopeAnchors = []string{"pkg/"}
	mu.SetWriteAnalysisIR(ir)
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	run := o.seedWriteWorkflowRun()
	if len(run.Batches) != 1 || run.Batches[0].Status != types.WriteWorkflowBatchNeedsExploration {
		t.Fatalf("non-micro seed must keep needs_exploration, got %+v", run.Batches)
	}
}

// Budget completion lane: a batch applied with its last steps must still get
// a verification verdict; a green verdict completes the run instead of
// blocking it and discarding the applied work.
func TestRunWriteControllerWorkflow_BudgetExhaustionRunsCompletionVerify(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("budget completion")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "budget completion"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "single change"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "ready"},
		// The loop-top budget check fires before this decision is requested.
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "never_reached"},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	o.maxSteps = 4 // controller+plan+controller+apply → exhausted before verify turn
	verifyRan := false
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-budget", Status: types.PlanStatusPending, Summary: "attempt", TargetPaths: []string{"fix.go"}})
		case types.StageVerify:
			verifyRan = true
			mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-budget", Passed: true})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("completion verify should finish the run, got %v", err)
	}
	if !verifyRan {
		t.Fatal("completion lane must run the verify dispatch")
	}
	if store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("run should complete, got %s", store.last.Status)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "budget_completion_verify") {
		t.Fatalf("completion lane should be recorded: %+v", store.last.ProgressLedger)
	}
	if store.last.Batches[0].Status != types.WriteWorkflowBatchComplete {
		t.Fatalf("batch should be verified complete, got %+v", store.last.Batches[0])
	}
}

func TestRunWriteControllerWorkflow_BudgetCompletionVerifyFailureStillBlocks(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	planDir := t.TempDir()
	mu := types.NewMutableState("budget completion fail")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "budget completion fail"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "single change"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "ready"},
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
	o.maxSteps = 4
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-budget-fail", Status: types.PlanStatusPending, Summary: "attempt", TargetPaths: []string{"fix.go"}})
		case types.StageVerify:
			mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-budget-fail", Passed: false, FailureSummary: "still red"})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	err := o.runWriteControllerWorkflow(&steps)
	if err == nil || !strings.Contains(err.Error(), "budget exhausted") {
		t.Fatalf("failed completion verify must still block on budget, got %v", err)
	}
	// The failed verdict must leave durable evidence.
	if _, statErr := os.Stat(filepath.Join(planDir, "plan-budget-fail.report.json")); statErr != nil {
		t.Fatalf("completion verify failure must persist the report: %v", statErr)
	}
}

// The --plan-file artifact must follow the live plan across replan rounds.
func TestRunWriteControllerWorkflow_MirrorsActivePlanToImportFile(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mirror := filepath.Join(t.TempDir(), "imported.plan.json")
	seedPlan := &types.ChangePlan{ID: "plan-imported", Status: types.PlanStatusPending, Summary: "seed", TargetPaths: []string{"fix.go"}, Request: "fix it",
		Changes: []types.FileChange{{Path: "fix.go", Kind: "modify", NewContent: "package main\n"}}}
	if err := types.WritePlanToFile(seedPlan, mirror); err != nil {
		t.Fatalf("seed plan write: %v", err)
	}
	mu := types.NewMutableState("mirror live plan")
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionApplyPlan, ReasonCode: "imported_ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "applied"},
		{Action: writeflow.ActionReplanBatch, ReasonCode: "repair", Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "repair"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "repair_ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "repair_applied"},
		{Action: writeflow.ActionFinish, ReasonCode: "done"},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}, PlanPath: mirror}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	o.SetWriteRetryBudget(2)
	attempt := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			attempt++
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-repair", Status: types.PlanStatusPending, Summary: "repair", TargetPaths: []string{"fix.go"}})
		case types.StageVerify:
			planID := "plan-imported"
			if plan := mu.ChangePlan(); plan != nil {
				planID = plan.ID
			}
			if attempt == 0 {
				mu.SetChangeReport(&types.ChangeReport{PlanID: planID, Passed: false, FailureSummary: "imported plan red"})
			} else {
				mu.SetChangeReport(&types.ChangeReport{PlanID: planID, Passed: true})
			}
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	data, err := os.ReadFile(mirror)
	if err != nil {
		t.Fatalf("mirror file gone: %v", err)
	}
	var mirrored types.ChangePlan
	if err := json.Unmarshal(data, &mirrored); err != nil {
		t.Fatalf("mirror parse: %v", err)
	}
	if mirrored.ID != "plan-repair" {
		t.Fatalf("mirror must hold the live plan after replan, got %s", mirrored.ID)
	}
	if mirrored.Status != types.PlanStatusApplied {
		t.Fatalf("mirror must carry the final verified status, got %s", mirrored.Status)
	}
}


func TestLastPlanEmitRejectionSummary_PicksLatestPlanEmitFailure(t *testing.T) {
	results := []types.ToolResult{
		{ToolName: "read_file", Success: false, Summary: "irrelevant"},
		{ToolName: "emit_change_plan", Success: false, Summary: "emit_change_plan rejected: task.scope=micro forbids kind=modify"},
		{ToolName: "emit_change_plan", Success: true, Summary: "stored"},
		{ToolName: "grep", Success: false, Summary: "noise"},
	}
	got := lastPlanEmitRejectionSummary(results)
	if got != "" {
		t.Fatalf("latest emit succeeded; no rejection should be cited, got %q", got)
	}
	results = append(results, types.ToolResult{ToolName: "emit_plan_skeleton", Success: false, Summary: "emit_plan_skeleton rejected: bounded body missing"})
	got = lastPlanEmitRejectionSummary(results)
	if !strings.Contains(got, "bounded body missing") {
		t.Fatalf("latest plan-emit rejection should be cited, got %q", got)
	}
}

// G1: a resumed run must rebuild retry counts, the active plan, and the
// verify-failure carrier from typed records + durable artifacts.
func TestRunWriteControllerWorkflow_ResumeHydratesRetryPlanAndHandoff(t *testing.T) {
	planDir := t.TempDir()
	storedPlan := &types.ChangePlan{ID: "plan-resume", Status: types.PlanStatusVerifyFailed, Summary: "resume me",
		TargetPaths: []string{"fix.go"},
		Changes:     []types.FileChange{{Path: "fix.go", Kind: "modify", NewContent: "package main\n"}}}
	if err := types.WritePlanToFile(storedPlan, filepath.Join(planDir, "plan-resume.json")); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	failedReport := &types.ChangeReport{PlanID: "plan-resume", Passed: false, FailureKind: types.FailureKindTestsFailed,
		FailureSummary: "red", TestResults: []types.TestResult{{AssertionID: "TestX", Passed: false}}}
	if err := types.WriteChangeReportToFile(failedReport, filepath.Join(planDir, "plan-resume.report.json")); err != nil {
		t.Fatalf("seed report: %v", err)
	}
	active := &types.WriteWorkflowRun{
		RunID: "wf-resume", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID: "batch-1", Status: types.WriteWorkflowBatchReadyToPlan, PlanID: "plan-resume",
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "plan", Status: "complete", PlanID: "plan-resume"},
				{Kind: "apply", Status: "applied", PlanID: "plan-resume"},
				{Kind: "verify", Status: "failed", ReasonCode: "tests_failed", PlanID: "plan-resume", ReportID: "plan-resume.report.json", ArtifactRef: "plan-resume.attempt-1.diff"},
			},
		}},
	}
	store := &fakeWorkflowRunStore{active: active}
	mu := types.NewMutableState("resume hydration")
	decisions := []writeflow.WriteWorkflowDecision{
		// Resumed run can go straight to apply because the plan is hydrated.
		{Action: writeflow.ActionApplyPlan, ReasonCode: "resume_apply"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "resume_verify"},
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
	o.reportDir = planDir
	o.SetWriteRetryBudget(3)
	var handoffAtApply *types.VerifyFailureHandoff
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StageApply:
			handoffAtApply = mu.VerifyFailureHandoff()
		case types.StageVerify:
			mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-resume", Passed: true})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("resumed run should complete: %v", err)
	}
	if handoffAtApply == nil || handoffAtApply.PlanID != "plan-resume" || len(handoffAtApply.FailingTests) != 1 {
		t.Fatalf("resume must rebuild the verify-failure carrier from the durable report, got %+v", handoffAtApply)
	}
	if handoffAtApply.DiffArtifactRef != "plan-resume.attempt-1.diff" {
		t.Fatalf("resume carrier should reference the persisted attempt diff, got %q", handoffAtApply.DiffArtifactRef)
	}
}

// G1: rebuilt retry counts must keep the budget honest across resume.
func TestRunWriteControllerWorkflow_ResumeKeepsRetryBudgetHonest(t *testing.T) {
	planDir := t.TempDir()
	active := &types.WriteWorkflowRun{
		RunID: "wf-budget", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID: "batch-1", Status: types.WriteWorkflowBatchReadyToPlan, PlanID: "plan-b",
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "verify", Status: "failed", ReasonCode: "tests_failed", PlanID: "plan-b"},
				{Kind: "verify", Status: "failed", ReasonCode: "tests_failed", PlanID: "plan-b"},
			},
		}},
	}
	store := &fakeWorkflowRunStore{active: active}
	mu := types.NewMutableState("resume budget")
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionReplanBatch, ReasonCode: "retry", Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "retry"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "applied"},
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
	o.SetWriteRetryBudget(2) // two failures already on record → next failure exceeds
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-b2", Status: types.PlanStatusPending, Summary: "retry", TargetPaths: []string{"fix.go"}})
		case types.StageVerify:
			mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-b2", Passed: false, FailureSummary: "still red"})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	err := o.runWriteControllerWorkflow(&steps)
	if err == nil || !strings.Contains(err.Error(), "verify failed") {
		t.Fatalf("third failure must exhaust the resumed budget, got %v", err)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "verify_retry_budget_exhausted") {
		t.Fatalf("resumed retry budget must count prior failed attempts: %+v", store.last.ProgressLedger)
	}
}

// G2: exhausting the exploration budget rejects the ACTION, not the run.
func TestRunWriteControllerWorkflow_ExplorationBudgetSoftRejects(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("exploration budget")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "explore a lot"}}})
	explore := func() writeflow.WriteWorkflowDecision {
		return writeflow.WriteWorkflowDecision{Action: writeflow.ActionExploreCode, ExplorationRequest: &types.WriteExplorationRequest{BatchID: "batch-1", Goal: "look around"}}
	}
	decisions := []writeflow.WriteWorkflowDecision{
		explore(), explore(),
		explore(), // third: budget exhausted → rejected, run continues
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "bounded"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "applied"},
		{Action: writeflow.ActionFinish, ReasonCode: "done"},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{}, nil
		},
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-x", Status: types.PlanStatusPending, Summary: "bounded", TargetPaths: []string{"fix.go"}})
		case types.StageVerify:
			mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-x", Passed: true})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("budget-rejected exploration must not kill the run: %v", err)
	}
	if store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("run should complete after the controller pivots to planning, got %s", store.last.Status)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "exploration_budget_exhausted") {
		t.Fatalf("rejection event must be recorded: %+v", store.last.ProgressLedger)
	}
}

// G3: switching the active batch clears the previous batch's carrier.
func TestRunWriteControllerWorkflow_BatchSwitchClearsStaleHandoff(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("batch switch handoff")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "two batches"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "first"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "applied"},
		{Action: writeflow.ActionAppendBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-2", Goal: "second"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "second_ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "second_applied"},
		{Action: writeflow.ActionFinish, ReasonCode: "done", FinishDisposition: writeflow.FinishDispositionAcceptUnverified},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	o.SetWriteRetryBudget(5)
	var handoffAtBatch2Plan *types.VerifyFailureHandoff
	planCount := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			planCount++
			if planCount == 2 {
				handoffAtBatch2Plan = mu.VerifyFailureHandoff()
			}
			mu.SetChangePlan(&types.ChangePlan{ID: fmt.Sprintf("plan-%d", planCount), Status: types.PlanStatusPending, Summary: "p", TargetPaths: []string{"fix.go"}})
		case types.StageVerify:
			plan := mu.ChangePlan()
			if planCount == 1 {
				mu.SetChangeReport(&types.ChangeReport{PlanID: plan.ID, Passed: false, FailureSummary: "batch-1 red"})
			} else {
				mu.SetChangeReport(&types.ChangeReport{PlanID: plan.ID, Passed: true})
			}
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	_ = o.runWriteControllerWorkflow(&steps)
	if handoffAtBatch2Plan != nil {
		t.Fatalf("batch-2 planning must not see batch-1's verify-failure carrier, got %+v", handoffAtBatch2Plan)
	}
}

// G4: ask_user questions surface verbatim in the blocked result.
func TestRunWriteControllerWorkflow_AskUserSurfacesQuestions(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("ask user")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "needs input"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionAskUser, ReasonCode: "missing_owner_decision",
			QuestionsForUser: []string{"Which database engine should the migration target?", "Is downtime acceptable?"}},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	steps := 0
	err := o.runWriteControllerWorkflow(&steps)
	if err == nil {
		t.Fatal("ask_user pauses the run")
	}
	result := mu.Result()
	if !strings.Contains(result, "Which database engine should the migration target?") ||
		!strings.Contains(result, "Is downtime acceptable?") {
		t.Fatalf("typed questions must surface verbatim in the result, got %q", result)
	}
}

// G10: a batch completed on a no-tests outcome carries a finish caveat.
func TestRunWriteControllerWorkflow_NoTestsCompletionCarriesCaveat(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("no tests caveat")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "script only"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "script"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "applied"},
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
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-script", Status: types.PlanStatusPending, Summary: "script", TargetPaths: []string{"tool.py"}})
		case types.StageVerify:
			mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-script", Passed: true, NoTestsRunners: []string{"python"}})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if result := mu.Result(); !strings.Contains(result, "no tests executed for: batch-1") {
		t.Fatalf("no-tests completion must carry the typed caveat, got %q", result)
	}
}
