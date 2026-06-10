package orchestrator

import (
	"encoding/json"
	"fmt"
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
