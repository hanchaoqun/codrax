package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
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

func TestSeedWriteWorkflowRunPersistsWriteAnalysisContextForDirectPlan(t *testing.T) {
	mu := types.NewMutableState("fix direct plan")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task: types.WriteTask{
				Summary: "fix direct plan",
				Scope:   types.ScopeMicro,
			},
			ScopeAnchors:     []string{"pkg/bug.py"},
			ExpectedOutcomes: []string{"bug path works"},
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}

	run := o.seedWriteWorkflowRun()
	if len(run.ContextPacks) == 0 {
		t.Fatalf("expected write analysis context pack on direct-plan seed: %+v", run)
	}
	analysisPack := run.ContextPacks[0]
	view := analysisPack.View(types.WriteConsumerPlanner, 10)
	if !writeContextViewContains(view, "scope_anchor", "pkg/bug.py") {
		t.Fatalf("seeded write-analysis pack missing scope anchor: %+v", view.Items)
	}
	if active := run.Batches[0]; active.Status != types.WriteWorkflowBatchReadyToPlan {
		t.Fatalf("micro-scope direct plan should still start ready_to_plan, got %s", active.Status)
	}

	plan := &types.ChangePlan{
		ID:          "plan-direct",
		Summary:     "fix direct plan",
		TargetPaths: []string{"pkg/bug.py"},
		Changes:     []types.FileChange{{Path: "pkg/bug.py", Kind: "modify"}},
	}
	run = attachPlanContextPackToWorkflowRun(run, plan)
	merged := types.MergeWriteContextPacks("batch-1", "fix direct plan", run.ContextPacks...)
	coverage := merged.View(types.WriteConsumerPlanner, 30)
	if !writeContextViewContains(coverage, "plan_context_coverage", "covered=1/1") {
		t.Fatalf("plan context coverage should consume seeded write-analysis scope anchors: %+v", coverage.Items)
	}
	if writeContextViewContains(coverage, "plan_context_missing_source_path", "pkg/bug.py") {
		t.Fatalf("seeded scope anchor should not be reported as missing prior context: %+v", coverage.Items)
	}
}

func TestRunControllerPlanBatch_ReplansMissingSourceLocalization(t *testing.T) {
	mu := types.NewMutableState("fix owner boundary")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task:         types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopeMicro, Summary: "fix owner boundary"},
			Risk:         types.WriteRiskProfile{Overall: types.RiskBandLow},
			ScopeAnchors: []string{"pkg/owner.py"},
		},
	})
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	run := o.seedWriteWorkflowRun()
	mu.SetWriteWorkflowRun(&run)
	planCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		if stage != types.StagePlan {
			t.Fatalf("unexpected stage %s", stage)
		}
		planCalls++
		if planCalls == 1 {
			mu.SetChangePlan(&types.ChangePlan{
				ID:          "plan-caller",
				Status:      types.PlanStatusPending,
				Summary:     "patch caller symptom",
				TargetPaths: []string{"pkg/caller.py"},
				Changes:     []types.FileChange{{Path: "pkg/caller.py", Kind: "patch"}},
			})
		} else {
			hint := mu.PlanningHint()
			if !strings.Contains(hint, "Typed localization coverage critique") ||
				!strings.Contains(hint, "pkg/caller.py") ||
				!strings.Contains(hint, "pkg/owner.py") {
				t.Fatalf("localization retry hint missing context: %q", hint)
			}
			mu.SetChangePlan(&types.ChangePlan{
				ID:          "plan-owner",
				Status:      types.PlanStatusPending,
				Summary:     "patch owner boundary",
				TargetPaths: []string{"pkg/owner.py"},
				Changes:     []types.FileChange{{Path: "pkg/owner.py", Kind: "patch"}},
			})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}

	steps := 0
	if err := o.runControllerPlanBatch(&writeflow.WriteBatchPlan{ID: "batch-1", Goal: "fix owner boundary"}, &steps); err != nil {
		t.Fatalf("runControllerPlanBatch: %v", err)
	}
	if planCalls != 2 {
		t.Fatalf("plan calls = %d, want 2", planCalls)
	}
	if plan := mu.ChangePlan(); plan == nil || plan.ID != "plan-owner" {
		t.Fatalf("expected re-localized plan, got %+v", plan)
	}
}

func TestRunControllerPlanBatch_AcceptsCoveredSourceLocalization(t *testing.T) {
	mu := types.NewMutableState("fix owner boundary")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task:         types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopeMicro, Summary: "fix owner boundary"},
			Risk:         types.WriteRiskProfile{Overall: types.RiskBandLow},
			ScopeAnchors: []string{"pkg/owner.py"},
		},
	})
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	run := o.seedWriteWorkflowRun()
	mu.SetWriteWorkflowRun(&run)
	planCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		planCalls++
		mu.SetChangePlan(&types.ChangePlan{
			ID:          "plan-owner",
			Status:      types.PlanStatusPending,
			Summary:     "patch owner boundary",
			TargetPaths: []string{"pkg/owner.py"},
			Changes:     []types.FileChange{{Path: "pkg/owner.py", Kind: "patch"}},
		})
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}

	steps := 0
	if err := o.runControllerPlanBatch(&writeflow.WriteBatchPlan{ID: "batch-1", Goal: "fix owner boundary"}, &steps); err != nil {
		t.Fatalf("runControllerPlanBatch: %v", err)
	}
	if planCalls != 1 {
		t.Fatalf("covered source localization should not retry, plan calls = %d", planCalls)
	}
}

func TestRunControllerPlanBatch_LocalizationRetryNeedsPriorContext(t *testing.T) {
	mu := types.NewMutableState("fix unknown path")
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	planCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		planCalls++
		mu.SetChangePlan(&types.ChangePlan{
			ID:          "plan-new",
			Status:      types.PlanStatusPending,
			Summary:     "patch newly found source",
			TargetPaths: []string{"pkg/new_owner.py"},
			Changes:     []types.FileChange{{Path: "pkg/new_owner.py", Kind: "patch"}},
		})
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}

	steps := 0
	if err := o.runControllerPlanBatch(&writeflow.WriteBatchPlan{ID: "batch-1", Goal: "fix unknown path"}, &steps); err != nil {
		t.Fatalf("runControllerPlanBatch: %v", err)
	}
	if planCalls != 1 {
		t.Fatalf("no prior localization context should not retry, plan calls = %d", planCalls)
	}
}

func TestRunControllerPlanBatch_LocalizationRetryExcludesTestOnlyPlan(t *testing.T) {
	mu := types.NewMutableState("add regression")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task:         types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopeMicro, Summary: "add regression"},
			Risk:         types.WriteRiskProfile{Overall: types.RiskBandLow},
			ScopeAnchors: []string{"pkg/owner.py"},
		},
	})
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	run := o.seedWriteWorkflowRun()
	mu.SetWriteWorkflowRun(&run)
	planCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		planCalls++
		mu.SetChangePlan(&types.ChangePlan{
			ID:          "plan-test-only",
			Status:      types.PlanStatusPending,
			Summary:     "add regression",
			TargetPaths: []string{"tests/test_owner.py"},
			Changes:     []types.FileChange{{Path: "tests/test_owner.py", Kind: "patch"}},
		})
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}

	steps := 0
	if err := o.runControllerPlanBatch(&writeflow.WriteBatchPlan{ID: "batch-1", Goal: "add regression"}, &steps); err != nil {
		t.Fatalf("runControllerPlanBatch: %v", err)
	}
	if planCalls != 1 {
		t.Fatalf("test-only plan should not trigger localization retry, plan calls = %d", planCalls)
	}
}

func TestUpdateWorkflowRunBatchPlanStampsImpactObligations(t *testing.T) {
	run := &types.WriteWorkflowRun{
		RunID:         "run-impact",
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID: "batch-1",
		}},
	}
	plan := &types.ChangePlan{
		ID:          "plan-impact",
		Summary:     "repair caller helper contract",
		TargetPaths: []string{"caller.py", "helper.py"},
		Changes: []types.FileChange{{
			Path:      "caller.py",
			Kind:      "modify",
			DependsOn: []string{"helper.py"},
		}},
	}

	updateWorkflowRunBatchPlan(run, "batch-1", plan)

	if plan.ImpactObligations == nil {
		t.Fatalf("plan should be stamped with typed impact obligations")
	}
	if plan.ImpactAnalysis == nil || plan.ImpactAnalysis.ObligationSet == nil {
		t.Fatalf("plan should be stamped with durable impact analysis: %+v", plan.ImpactAnalysis)
	}
	for _, ob := range plan.ImpactObligations.Obligations {
		if ob.Kind == "dependency" &&
			ob.Relation == "depends_on" &&
			ob.SubjectPath == "caller.py" &&
			ob.RelatedPath == "helper.py" {
			return
		}
	}
	t.Fatalf("missing dependency impact obligation: %+v", plan.ImpactObligations.Obligations)
}

type readExplorationRunnerFunc func(*Orchestrator) (int, error)

func (f readExplorationRunnerFunc) Run(o *Orchestrator) (int, error) {
	return f(o)
}

type capturePlanSaver struct {
	dir   string
	saved []*types.ChangePlan
}

func (s *capturePlanSaver) Save(plan *types.ChangePlan) (string, error) {
	if s == nil {
		return "", fmt.Errorf("nil capturePlanSaver")
	}
	if plan == nil {
		return "", fmt.Errorf("nil plan")
	}
	path := filepath.Join(s.dir, plan.ID+".json")
	if err := types.WritePlanToFile(plan, path); err != nil {
		return "", err
	}
	cp := *plan
	s.saved = append(s.saved, &cp)
	return path, nil
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
		!workflowBatchHasAttempt(got, "explore", "complete", "", "") ||
		!workflowBatchHasAttemptArtifact(got, "apply", "applied", "apply_succeeded", "refs/codrax/applied/plan-controller-1") ||
		!workflowBatchHasAttemptArtifact(got, "verify", "passed", "tests_passed", "plan-controller-1.report.json") {
		t.Fatalf("batch attempts should track plan/apply/verify artifact refs: %+v", got.Attempts)
	}
	if plan := mu.ChangePlan(); plan == nil || plan.Status != types.PlanStatusApplied || plan.AppliedAt == nil {
		t.Fatalf("verified workflow should synchronize mutable plan status to applied, got %+v", plan)
	}
}

func TestRunWriteControllerWorkflow_DegradedExplorationHandoffContinuesToPlan(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("plan after degraded exploration")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task: types.WriteTask{
				Kind:    types.WriteTaskBugfix,
				Scope:   types.ScopePackage,
				Summary: "repair symptom after localized exploration",
			},
			ScopeAnchors: []string{"sphinx/ext/autodoc/typehints.py"},
		},
	})
	decisions := []writeflow.WriteWorkflowDecision{
		{
			Action:     writeflow.ActionExploreCode,
			ReasonCode: "need_context",
			ExplorationRequest: &types.WriteExplorationRequest{
				BatchID:        "batch-1",
				Goal:           "locate autodoc typehint rendering",
				CandidatePaths: []string{"sphinx/ext/autodoc/typehints.py"},
			},
		},
		{
			Action:     writeflow.ActionPlanBatch,
			ReasonCode: "context_ready",
			Batch: &writeflow.WriteBatchPlan{
				ID:            "batch-1",
				Goal:          "repair autodoc typehint rendering",
				ExpectedPaths: []string{"sphinx/ext/autodoc/typehints.py"},
			},
		},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModePlan, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	o.SetReadExplorationRunner(readExplorationRunnerFunc(func(o *Orchestrator) (int, error) {
		o.busCtx.Mutable.EvidenceClosure().SetReadSet(map[string]bool{
			"sphinx/ext/autodoc/typehints.py": true,
		})
		o.busCtx.Mutable.AppendEvidence([]types.EvidenceItem{{
			ID:              "ev-typehints",
			Kind:            types.EvidenceMechanism,
			Subject:         "record_typehints",
			Source:          "sphinx/ext/autodoc/typehints.py",
			LineStart:       127,
			AnchorSymbol:    "record_typehints",
			Summary:         "autodoc records annotations before rendering descriptions",
			GroundingStatus: types.GroundingRecovered,
		}})
		o.projectWriteExplorationHandoffFromTurnA()
		return 1, context.Canceled
	}))
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		if stage != types.StagePlan {
			t.Fatalf("unexpected write stage %s", stage)
		}
		handoff := mu.WriteExplorationHandoff()
		if handoff == nil || len(handoff.EvidenceRefs) != 1 {
			t.Fatalf("planner should receive degraded exploration handoff, got %+v", handoff)
		}
		mu.SetChangePlan(&types.ChangePlan{
			ID:          "plan-degraded-explore",
			Status:      types.PlanStatusPending,
			Summary:     "repair autodoc typehint rendering",
			TargetPaths: []string{"sphinx/ext/autodoc/typehints.py"},
			Changes: []types.FileChange{{
				Path:       "sphinx/ext/autodoc/typehints.py",
				Kind:       "modify",
				NewContent: "# patched\n",
			}},
		})
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}

	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("degraded exploration with typed handoff should continue to plan: %v", err)
	}
	if controllerCalls != 2 {
		t.Fatalf("controller calls = %d, want 2", controllerCalls)
	}
	if store.last == nil || store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("plan-mode workflow should complete after planning, got %+v", store.last)
	}
	batch := store.last.Batches[0]
	if !workflowBatchHasAttempt(batch, "explore", "degraded", "context canceled", "") {
		t.Fatalf("batch should record degraded exploration attempt: %+v", batch.Attempts)
	}
	if batch.Status != types.WriteWorkflowBatchPlanned || batch.PlanID != "plan-degraded-explore" {
		t.Fatalf("degraded handoff should let batch reach planned state: %+v", batch)
	}
	if !workflowProgressContains(store.last.ProgressLedger, "exploration_degraded_handoff_ready") {
		t.Fatalf("progress ledger should record degraded handoff continuation: %+v", store.last.ProgressLedger)
	}
}

func TestSeedControllerBatchPlanningHint_CompletedExplorationHandoffConverges(t *testing.T) {
	mu := types.NewMutableState("explored repair")
	run := &types.WriteWorkflowRun{
		RunID:         "run-1",
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
			Attempts: []types.WriteWorkflowAttempt{{
				Kind:   "explore",
				Status: "complete",
			}},
		}},
	}
	mu.SetWriteWorkflowRun(run)
	mu.SetWriteExplorationHandoff(&types.WriteExplorationHandoff{
		BatchID:     "batch-1",
		TargetFiles: []string{"pkg/fix.py"},
		EvidenceRefs: []types.WriteExplorationEvidenceRef{{
			ID:      "ev-1",
			Source:  "pkg/fix.py",
			Summary: "boundary check evidence",
		}},
	})
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply}
	o.seedControllerBatchPlanningHint(writeflow.WriteBatchPlan{
		ID:            "batch-1",
		Goal:          "repair boundary bug",
		ExpectedPaths: []string{"pkg/fix.py"},
	})
	hint := mu.PlanningHint()
	if !strings.Contains(hint, "Exploration convergence") ||
		!strings.Contains(hint, "bounded ChangePlan synthesis step") {
		t.Fatalf("completed exploration handoff should seed convergence hint, got:\n%s", hint)
	}
	if !activeBatchHasCompletedExplorationHandoff(mu) {
		t.Fatal("typed helper should detect completed exploration handoff")
	}
}

func TestSeedControllerBatchPlanningHint_DegradedExplorationHandoffConverges(t *testing.T) {
	mu := types.NewMutableState("degraded explored repair")
	run := &types.WriteWorkflowRun{
		RunID:         "run-1",
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
			Attempts: []types.WriteWorkflowAttempt{{
				Kind:   "explore",
				Status: "degraded",
			}},
		}},
	}
	mu.SetWriteWorkflowRun(run)
	mu.SetWriteExplorationHandoff(&types.WriteExplorationHandoff{
		BatchID:     "batch-1",
		TargetFiles: []string{"pkg/fix.py"},
		EvidenceRefs: []types.WriteExplorationEvidenceRef{{
			ID:      "ev-1",
			Source:  "pkg/fix.py",
			Summary: "localized failure evidence",
		}},
	})
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply}
	o.seedControllerBatchPlanningHint(writeflow.WriteBatchPlan{
		ID:            "batch-1",
		Goal:          "repair localized bug",
		ExpectedPaths: []string{"pkg/fix.py"},
	})
	hint := mu.PlanningHint()
	if !strings.Contains(hint, "Exploration convergence") ||
		!strings.Contains(hint, "bounded ChangePlan synthesis step") {
		t.Fatalf("degraded exploration handoff should still seed convergence hint, got:\n%s", hint)
	}
	if !activeBatchHasUsableExplorationHandoff(mu) {
		t.Fatal("typed helper should detect usable degraded exploration handoff")
	}
	if activeBatchHasCompletedExplorationHandoff(mu) {
		t.Fatal("degraded handoff must not be mislabeled as completed")
	}
}

func TestPlannerSoftCapForCompletedExplorationAppliesSynthesisFloor(t *testing.T) {
	cases := []struct {
		name             string
		base             int
		current          int
		effectiveCurrent int
		want             int
	}{
		{name: "scaled cap is preserved", base: 6, current: 13, effectiveCurrent: 13, want: 13},
		{name: "base gets synthesis floor", base: 6, current: 0, effectiveCurrent: 6, want: 12},
		{name: "low current gets synthesis floor", base: 6, current: 9, effectiveCurrent: 0, want: 12},
		{name: "final fallback gets synthesis floor", base: 6, current: 0, effectiveCurrent: 0, want: 12},
		{name: "higher configured base raises floor", base: 10, current: 10, effectiveCurrent: 10, want: 16},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := plannerSoftCapForCompletedExploration(tt.base, tt.current, tt.effectiveCurrent)
			if got != tt.want {
				t.Fatalf("plannerSoftCapForCompletedExploration = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRunWriteControllerWorkflow_AppliesReadyPlanBeforeRepeatedPlanningDecision(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("apply ready plan before repeating plan")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "apply ready plan"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "produce repair plan"}},
		{Action: writeflow.ActionReplanBatch, ReasonCode: "mistaken_repeat", Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "repeat planning"}},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "after_apply"},
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
	planCalls := 0
	applyCalls := 0
	verifyCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			planCalls++
			if planCalls > 1 {
				t.Fatalf("ready plan should apply before repeated planning, planCalls=%d", planCalls)
			}
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-ready-1", Status: types.PlanStatusPending, Summary: "ready", TargetPaths: []string{"fix.py"}})
		case types.StageApply:
			applyCalls++
		case types.StageVerify:
			verifyCalls++
			mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-ready-1", Passed: true})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if planCalls != 1 || applyCalls != 1 || verifyCalls != 1 {
		t.Fatalf("stage calls plan/apply/verify = %d/%d/%d, want 1/1/1", planCalls, applyCalls, verifyCalls)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "ready_plan_action_overridden") {
		t.Fatalf("ready plan override progress missing: %+v", store.last.ProgressLedger)
	}
}

func TestRunWriteControllerWorkflow_VerifiesAppliedPlanBeforeRepeatedPlanningDecision(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("verify applied plan before repeating plan")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "verify applied plan"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "produce repair plan"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "plan_ready"},
		{Action: writeflow.ActionReplanBatch, ReasonCode: "mistaken_repeat", Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "repeat planning"}},
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
	planCalls := 0
	applyCalls := 0
	verifyCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			planCalls++
			if planCalls > 1 {
				t.Fatalf("applied plan should verify before repeated planning, planCalls=%d", planCalls)
			}
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-applied-1", Status: types.PlanStatusPending, Summary: "ready", TargetPaths: []string{"fix.py"}})
		case types.StageApply:
			applyCalls++
		case types.StageVerify:
			verifyCalls++
			plan := mu.ChangePlan()
			if plan == nil || plan.Status != types.PlanStatusAppliedPendingVerify {
				t.Fatalf("post-apply verify should see applied_pending_verify plan status, got %+v", plan)
			}
			mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-applied-1", Passed: true})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if planCalls != 1 || applyCalls != 1 || verifyCalls != 1 {
		t.Fatalf("stage calls plan/apply/verify = %d/%d/%d, want 1/1/1", planCalls, applyCalls, verifyCalls)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "post_apply_verify_action_overridden") {
		t.Fatalf("post-apply verify override progress missing: %+v", store.last.ProgressLedger)
	}
}

func TestRunWriteControllerWorkflow_SynthesizesApplyAfterControllerDispatchError(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("apply ready plan after controller dispatch error")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "apply ready plan"}}})
	controllerSteps := []scriptedControllerStep{
		{decision: writeflow.WriteWorkflowDecision{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "produce repair plan"}}},
		{err: fmt.Errorf("controller transport unavailable")},
		{decision: writeflow.WriteWorkflowDecision{Action: writeflow.ActionVerifyBatch, ReasonCode: "after_apply"}},
		{decision: writeflow.WriteWorkflowDecision{Action: writeflow.ActionFinish, ReasonCode: "done"}},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedControllerWithErrors(t, controllerSteps, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	planCalls := 0
	applyCalls := 0
	verifyCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			planCalls++
			mu.SetChangePlan(&types.ChangePlan{
				ID:          "plan-ready-after-error",
				Status:      types.PlanStatusPending,
				Summary:     "repair",
				TargetPaths: []string{"src/fix.py"},
				Changes: []types.FileChange{{
					Path:       "src/fix.py",
					Kind:       "modify",
					NewContent: "def fixed():\n    return True\n",
				}},
			})
		case types.StageApply:
			applyCalls++
		case types.StageVerify:
			verifyCalls++
			mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-ready-after-error", Passed: true})
		default:
			t.Fatalf("unexpected stage %s", stage)
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if planCalls != 1 || applyCalls != 1 || verifyCalls != 1 {
		t.Fatalf("stage calls plan/apply/verify = %d/%d/%d, want 1/1/1", planCalls, applyCalls, verifyCalls)
	}
	if controllerCalls != len(controllerSteps) {
		t.Fatalf("controller calls = %d, want %d", controllerCalls, len(controllerSteps))
	}
	if store.last == nil || store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("workflow should complete after typed-state recovery, got %+v", store.last)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "controller_dispatch_recovered_ready_plan") {
		t.Fatalf("ready-plan dispatch recovery progress missing: %+v", store.last.ProgressLedger)
	}
}

func TestRunWriteControllerWorkflow_SynthesizesVerifyAfterControllerDispatchError(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("verify applied plan after controller dispatch error")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "verify applied plan"}}})
	controllerSteps := []scriptedControllerStep{
		{decision: writeflow.WriteWorkflowDecision{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "produce repair plan"}}},
		{decision: writeflow.WriteWorkflowDecision{Action: writeflow.ActionApplyPlan, ReasonCode: "plan_ready"}},
		{err: fmt.Errorf("controller transport unavailable")},
		{decision: writeflow.WriteWorkflowDecision{Action: writeflow.ActionFinish, ReasonCode: "done"}},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedControllerWithErrors(t, controllerSteps, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	planCalls := 0
	applyCalls := 0
	verifyCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			planCalls++
			mu.SetChangePlan(&types.ChangePlan{
				ID:          "plan-applied-after-error",
				Status:      types.PlanStatusPending,
				Summary:     "repair",
				TargetPaths: []string{"src/fix.py"},
				Changes: []types.FileChange{{
					Path:       "src/fix.py",
					Kind:       "modify",
					NewContent: "def fixed():\n    return True\n",
				}},
			})
		case types.StageApply:
			applyCalls++
		case types.StageVerify:
			verifyCalls++
			mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-applied-after-error", Passed: true})
		default:
			t.Fatalf("unexpected stage %s", stage)
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if planCalls != 1 || applyCalls != 1 || verifyCalls != 1 {
		t.Fatalf("stage calls plan/apply/verify = %d/%d/%d, want 1/1/1", planCalls, applyCalls, verifyCalls)
	}
	if controllerCalls != len(controllerSteps) {
		t.Fatalf("controller calls = %d, want %d", controllerCalls, len(controllerSteps))
	}
	if store.last == nil || store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("workflow should complete after typed-state recovery, got %+v", store.last)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "controller_dispatch_recovered_post_apply_verify") {
		t.Fatalf("post-apply verify dispatch recovery progress missing: %+v", store.last.ProgressLedger)
	}
}

func TestRunWriteControllerWorkflow_DoesNotRecoverCanceledControllerDispatch(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("do not recover canceled controller dispatch")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "cancel after plan"}}})
	controllerSteps := []scriptedControllerStep{
		{decision: writeflow.WriteWorkflowDecision{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "produce repair plan"}}},
		{err: context.Canceled},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedControllerWithErrors(t, controllerSteps, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	applyCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			mu.SetChangePlan(&types.ChangePlan{
				ID:          "plan-ready-canceled",
				Status:      types.PlanStatusPending,
				Summary:     "repair",
				TargetPaths: []string{"src/fix.py"},
				Changes: []types.FileChange{{
					Path:       "src/fix.py",
					Kind:       "modify",
					NewContent: "def fixed():\n    return True\n",
				}},
			})
		case types.StageApply:
			applyCalls++
		default:
			t.Fatalf("unexpected stage %s", stage)
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	err := o.runWriteControllerWorkflow(&steps)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled controller dispatch must not be recovered, got %v", err)
	}
	if applyCalls != 0 {
		t.Fatalf("canceled dispatch should not continue to apply, applyCalls=%d", applyCalls)
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

func TestRunWriteControllerWorkflow_ReplansOffScopeHighRiskBuildManifest(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("avoid off-scope build drift")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task: types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopePackage, Summary: "fix source and tests"},
			Risk: types.WriteRiskProfile{Overall: types.RiskBandLow, ChangesBuildSystem: false},
			ScopeAnchors: []string{
				"src/main/java/org/apache/commons/lang3/RandomStringUtils.java",
				"src/test/java/org/apache/commons/lang3/RandomStringUtilsTest.java",
			},
		},
	})
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, []writeflow.WriteWorkflowDecision{
			{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "fix source and tests"}},
			{Action: writeflow.ActionApplyPlan, ReasonCode: "plan_ready"},
			{Action: writeflow.ActionVerifyBatch, ReasonCode: "applied"},
			{Action: writeflow.ActionFinish, ReasonCode: "done"},
		}, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	planCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			planCalls++
			if planCalls == 1 {
				mu.SetChangePlan(&types.ChangePlan{
					ID:      "plan-with-build-drift",
					Status:  types.PlanStatusPending,
					Summary: "source fix plus unnecessary build edit",
					Changes: []types.FileChange{
						{Path: "src/main/java/org/apache/commons/lang3/RandomStringUtils.java", Kind: "patch"},
						{Path: "src/test/java/org/apache/commons/lang3/RandomStringUtilsTest.java", Kind: "patch"},
						{Path: "Makefile", Kind: "modify", NewContent: "check:\n\t@echo ok\n"},
					},
					TargetPaths: []string{
						"src/main/java/org/apache/commons/lang3/RandomStringUtils.java",
						"src/test/java/org/apache/commons/lang3/RandomStringUtilsTest.java",
						"Makefile",
					},
				})
			} else {
				if hint := mu.PlanningHint(); !strings.Contains(hint, "Makefile") || !strings.Contains(hint, "Typed plan-risk critique") {
					t.Fatalf("replan hint should carry typed off-scope high-risk path, got:\n%s", hint)
				}
				mu.SetChangePlan(&types.ChangePlan{
					ID:      "plan-source-tests-only",
					Status:  types.PlanStatusPending,
					Summary: "source and test fix only",
					Changes: []types.FileChange{
						{Path: "src/main/java/org/apache/commons/lang3/RandomStringUtils.java", Kind: "patch"},
						{Path: "src/test/java/org/apache/commons/lang3/RandomStringUtilsTest.java", Kind: "patch"},
					},
					TargetPaths: []string{
						"src/main/java/org/apache/commons/lang3/RandomStringUtils.java",
						"src/test/java/org/apache/commons/lang3/RandomStringUtilsTest.java",
					},
				})
			}
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
	if planCalls != 2 {
		t.Fatalf("plan calls = %d, want 2", planCalls)
	}
	if plan := mu.ChangePlan(); plan == nil || plan.ID != "plan-source-tests-only" || containsString(plan.TargetPaths, "Makefile") {
		t.Fatalf("workflow should finish with re-scoped source/test plan, got %+v", plan)
	}
	if store.last == nil || store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("workflow should complete after replan, got %+v", store.last)
	}
}

func TestRunControllerPlanBatch_KeepsTypedBuildSystemChangeForApproval(t *testing.T) {
	mu := types.NewMutableState("real build change")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task:         types.WriteTask{Kind: types.WriteTaskConfig, Scope: types.ScopeProject, Summary: "update build"},
			Risk:         types.WriteRiskProfile{Overall: types.RiskBandHigh, ChangesBuildSystem: true},
			ScopeAnchors: []string{"src/main/java/App.java"},
		},
	})
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	planCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		if stage != types.StagePlan {
			t.Fatalf("unexpected stage %s", stage)
		}
		planCalls++
		mu.SetChangePlan(&types.ChangePlan{
			ID:          "plan-build-system",
			Status:      types.PlanStatusPending,
			Summary:     "intentional build update",
			Changes:     []types.FileChange{{Path: "Makefile", Kind: "modify", NewContent: "check:\n\t@echo ok\n"}},
			TargetPaths: []string{"Makefile"},
		})
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runControllerPlanBatch(&writeflow.WriteBatchPlan{ID: "batch-1", Goal: "build update"}, &steps); err != nil {
		t.Fatalf("runControllerPlanBatch: %v", err)
	}
	if planCalls != 1 {
		t.Fatalf("typed build-system change should not be internally replanned, plan calls = %d", planCalls)
	}
	if plan := mu.ChangePlan(); plan == nil || plan.ID != "plan-build-system" {
		t.Fatalf("expected original build-system plan, got %+v", plan)
	}
}

func TestRunWriteControllerWorkflow_ReplansProtectedRegressionTestWeakening(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	root := t.TempDir()
	writeTestFile(t, root, "tests/test_tokenizer.py", "def test_newline():\n    assert tok.tokenize(\"#include <set>\\n\\n\\n\\n\\n\") == [300]\n")
	mu := types.NewMutableState("preserve odd newline regression")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task: types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopeMicro, Summary: "fix tokenizer fallback"},
			Risk: types.WriteRiskProfile{Overall: types.RiskBandLow},
			ScopeAnchors: []string{
				"fastlex/tokenizer.py",
				"tests/test_tokenizer.py",
			},
			Constraints: []types.WriteConstraint{{
				Kind:   "preserve_regression_test",
				Target: "tests/test_tokenizer.py",
				Note:   "five-newline odd-run regression input is intentional",
			}},
		},
	})
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, []writeflow.WriteWorkflowDecision{
			{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "fix tokenizer fallback"}},
			{Action: writeflow.ActionApplyPlan, ReasonCode: "plan_ready"},
			{Action: writeflow.ActionVerifyBatch, ReasonCode: "applied"},
			{Action: writeflow.ActionFinish, ReasonCode: "done"},
		}, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, RepoRoot: root, MainRepoRoot: root, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	planCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			planCalls++
			if planCalls == 1 {
				mu.SetChangePlan(&types.ChangePlan{
					ID:      "plan-weakens-test",
					Status:  types.PlanStatusPending,
					Summary: "fix source but narrows regression input",
					Changes: []types.FileChange{
						{Path: "fastlex/tokenizer.py", Kind: "patch", Patch: "--- a/fastlex/tokenizer.py\n+++ b/fastlex/tokenizer.py\n@@ -1 +1 @@\n-old\n+new\n"},
						{Path: "tests/test_tokenizer.py", Kind: "patch", Patch: "--- a/tests/test_tokenizer.py\n+++ b/tests/test_tokenizer.py\n@@ -1,2 +1,2 @@\n def test_newline():\n-    assert tok.tokenize(\"#include <set>\\n\\n\\n\\n\\n\") == [300]\n+    assert tok.tokenize(\"#include <set>\\n\\n\\n\\n\") == [300]\n"},
					},
					TargetPaths: []string{"fastlex/tokenizer.py", "tests/test_tokenizer.py"},
				})
			} else {
				hint := mu.PlanningHint()
				if !strings.Contains(hint, "Typed test-contract critique") || !strings.Contains(hint, "tests/test_tokenizer.py") || !strings.Contains(hint, "#include <set>") {
					t.Fatalf("replan hint should carry protected test deletion evidence, got:\n%s", hint)
				}
				mu.SetChangePlan(&types.ChangePlan{
					ID:          "plan-preserves-test",
					Status:      types.PlanStatusPending,
					Summary:     "source-only fix preserving regression test",
					Changes:     []types.FileChange{{Path: "fastlex/tokenizer.py", Kind: "patch", Patch: "--- a/fastlex/tokenizer.py\n+++ b/fastlex/tokenizer.py\n@@ -1 +1 @@\n-old\n+new\n"}},
					TargetPaths: []string{"fastlex/tokenizer.py"},
				})
			}
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
	if planCalls != 2 {
		t.Fatalf("plan calls = %d, want 2", planCalls)
	}
	if plan := mu.ChangePlan(); plan == nil || plan.ID != "plan-preserves-test" || containsString(plan.TargetPaths, "tests/test_tokenizer.py") {
		t.Fatalf("workflow should finish with source-only replan, got %+v", plan)
	}
}

func TestTestContractReplanHintDoesNotParseConstraintNote(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "tests/test_tokenizer.py", "def test_newline():\n    assert tok.tokenize(\"#include <set>\\n\\n\\n\\n\\n\") == [300]\n")
	mu := types.NewMutableState("note only")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Constraints: []types.WriteConstraint{{
				Kind:   "preserve_user_request",
				Target: "tests/test_tokenizer.py",
				Note:   "the five-newline odd-run regression input is intentional; do not reduce it",
			}},
		},
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, nil, nil, nil)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, RepoRoot: root, MainRepoRoot: root}
	plan := &types.ChangePlan{
		ID: "plan-note-only",
		Changes: []types.FileChange{{
			Path:  "tests/test_tokenizer.py",
			Kind:  "patch",
			Patch: "--- a/tests/test_tokenizer.py\n+++ b/tests/test_tokenizer.py\n@@ -1,2 +1,2 @@\n def test_newline():\n-    assert tok.tokenize(\"#include <set>\\n\\n\\n\\n\\n\") == [300]\n+    assert tok.tokenize(\"#include <set>\\n\\n\\n\\n\") == [300]\n",
		}},
		TargetPaths: []string{"tests/test_tokenizer.py"},
	}
	if hint, paths := o.testContractReplanHint(plan); hint != "" || len(paths) != 0 {
		t.Fatalf("note-only natural-language constraint must not drive hard critic, hint=%q paths=%v", hint, paths)
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

func TestRunWriteControllerWorkflow_VerifyInfraFailureRetriesVerify(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("retry missing verify report")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "retry missing verify report"}}})
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
	o.SetWriteRetryBudget(1)
	verifyCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-infra", Status: types.PlanStatusPending, Summary: "attempt", TargetPaths: []string{"fix.go"}})
		case types.StageVerify:
			verifyCalls++
			if verifyCalls == 1 {
				*stepsUsed++
				return nil, fmt.Errorf("verifier executor did not install a report")
			}
			mu.SetChangeReport(&types.ChangeReport{
				PlanID:  "plan-infra",
				Channel: types.ChangeReportChannelPostApplyVerify,
				Passed:  true,
			})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if verifyCalls != 2 {
		t.Fatalf("verify calls = %d, want one retry", verifyCalls)
	}
	if store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("workflow should complete after verify retry: %+v", store.last)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "verify_infra_retry") {
		t.Fatalf("infra retry progress missing: %+v", store.last.ProgressLedger)
	}
	if workflowProgressHasReason(store.last.ProgressLedger, "verify_failed") {
		t.Fatalf("infra retry should not be rendered as verify_failed: %+v", store.last.ProgressLedger)
	}
	batch := store.last.Batches[0]
	if !workflowBatchHasAttempt(batch, "verify", "infra_error", "verify_tool_not_called", "") ||
		!workflowBatchHasAttempt(batch, "verify", "passed", "tests_passed", "plan-infra.report.json") {
		t.Fatalf("verify attempts should record infra retry then pass: %+v", batch.Attempts)
	}
}

func TestRunWriteControllerWorkflow_VerifyInfraBudgetBlocksWithoutReplan(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("missing verify report budget exhausted")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "missing verify report"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "single change"}},
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
	o.SetWriteRetryBudget(0)
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		if stage == types.StagePlan {
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-infra-block", Status: types.PlanStatusPending, Summary: "attempt", TargetPaths: []string{"fix.go"}})
		}
		*stepsUsed++
		if stage == types.StageVerify {
			return nil, fmt.Errorf("verifier executor did not install a report")
		}
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err == nil {
		t.Fatal("workflow should block after verify infra budget is exhausted")
	}
	if store.last == nil || store.last.Status != types.WriteWorkflowRunBlocked {
		t.Fatalf("workflow should be blocked, got %+v", store.last)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "verify_infra_retry_budget_exhausted") {
		t.Fatalf("infra budget exhaustion progress missing: %+v", store.last.ProgressLedger)
	}
	if workflowProgressHasReason(store.last.ProgressLedger, "verify_failed") {
		t.Fatalf("missing report should not be converted to code verify_failed: %+v", store.last.ProgressLedger)
	}
	if !workflowBatchHasAttempt(store.last.Batches[0], "verify", "infra_error", "verify_tool_not_called", "") {
		t.Fatalf("infra error attempt missing: %+v", store.last.Batches[0].Attempts)
	}
	if plan := mu.ChangePlan(); plan == nil || plan.Status != types.PlanStatusUnverified {
		t.Fatalf("infra budget exhaustion should leave active plan unverified, got %+v", plan)
	}
}

func TestRunWriteControllerWorkflow_RunnerMissingCompletesUnverifiedWithoutReplan(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("runner missing")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "runner missing"}}})
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
	o.SetWriteRetryBudget(3)
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-runner-missing", Status: types.PlanStatusPending, Summary: "attempt", TargetPaths: []string{"fix.go"}})
		case types.StageVerify:
			mu.SetChangeReport(&types.ChangeReport{
				PlanID:         "plan-runner-missing",
				Channel:        types.ChangeReportChannelPostApplyVerify,
				Passed:         false,
				FailureKind:    types.FailureKindRunnerMissing,
				FailureSummary: "pytest: command not found",
			})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("workflow should complete unverified when typed report says runner_missing: %v", err)
	}
	if store.last == nil || store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("workflow should complete, got %+v", store.last)
	}
	if store.last.Batches[0].Completion == nil ||
		store.last.Batches[0].Completion.Verdict != types.WriteWorkflowCompletionUnverified ||
		store.last.Batches[0].Completion.ReasonCode != "runner_missing" {
		t.Fatalf("runner_missing completion verdict missing: %+v", store.last.Batches[0].Completion)
	}
	if store.last.Completion == nil ||
		store.last.Completion.Verdict != types.WriteWorkflowCompletionUnverified ||
		store.last.Completion.ReasonCode != "runner_missing" {
		t.Fatalf("runner_missing run completion verdict missing: %+v", store.last.Completion)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "batch_unverified") {
		t.Fatalf("runner_missing should complete as batch_unverified: %+v", store.last.ProgressLedger)
	}
	if workflowProgressHasReason(store.last.ProgressLedger, "verify_failed") {
		t.Fatalf("runner_missing should not enter replan verify_failed path: %+v", store.last.ProgressLedger)
	}
	if workflowProgressHasReason(store.last.ProgressLedger, "runner_missing") {
		t.Fatalf("runner_missing should not block the workflow: %+v", store.last.ProgressLedger)
	}
	if !workflowBatchHasAttempt(store.last.Batches[0], "verify", "unverified", "runner_missing", "plan-runner-missing.report.json") {
		t.Fatalf("runner_missing verify attempt missing: %+v", store.last.Batches[0].Attempts)
	}
	if plan := mu.ChangePlan(); plan == nil || plan.Status != types.PlanStatusUnverified {
		t.Fatalf("runner_missing should leave plan unverified, got %+v", plan)
	}
	if result := mu.Result(); !strings.Contains(result, "local verification unavailable for: batch-1 (runner_missing)") {
		t.Fatalf("result should carry runner_missing unverified caveat, got %q", result)
	}
}

func TestRunWriteControllerWorkflow_ParserErrorCompletesUnverifiedWithoutReplan(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("parser error")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "parser error"}}})
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
	o.SetWriteRetryBudget(3)
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-parser-error", Status: types.PlanStatusPending, Summary: "attempt", TargetPaths: []string{"fix.go"}})
		case types.StageVerify:
			mu.SetChangeReport(&types.ChangeReport{
				PlanID:            "plan-parser-error",
				Channel:           types.ChangeReportChannelPostApplyVerify,
				Passed:            false,
				FailureKind:       types.FailureKindParserError,
				FailureReasonCode: "pytest_import_startup_error",
				FailureSummary:    "pytest report missing after collection import error",
			})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("workflow should complete unverified when typed report says parser_error: %v", err)
	}
	if store.last == nil || store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("workflow should complete, got %+v", store.last)
	}
	if store.last.Batches[0].Completion == nil ||
		store.last.Batches[0].Completion.Verdict != types.WriteWorkflowCompletionUnverified ||
		store.last.Batches[0].Completion.ReasonCode != "parser_error" {
		t.Fatalf("parser_error completion verdict missing: %+v", store.last.Batches[0].Completion)
	}
	if store.last.Completion == nil ||
		store.last.Completion.Verdict != types.WriteWorkflowCompletionUnverified ||
		store.last.Completion.ReasonCode != "parser_error" {
		t.Fatalf("parser_error run completion verdict missing: %+v", store.last.Completion)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "batch_unverified") {
		t.Fatalf("parser_error should complete as batch_unverified: %+v", store.last.ProgressLedger)
	}
	if workflowProgressHasReason(store.last.ProgressLedger, "verify_failed") {
		t.Fatalf("parser_error should not enter replan verify_failed path: %+v", store.last.ProgressLedger)
	}
	if !workflowBatchHasAttempt(store.last.Batches[0], "verify", "unverified", "parser_error", "plan-parser-error.report.json") {
		t.Fatalf("parser_error verify attempt missing: %+v", store.last.Batches[0].Attempts)
	}
	verifyAttempt := latestWorkflowBatchAttempt(store.last.Batches[0], "verify")
	if verifyAttempt == nil || verifyAttempt.FailureReasonCode != "pytest_import_startup_error" {
		t.Fatalf("parser_error verify attempt should retain failure subreason: %+v", verifyAttempt)
	}
	if plan := mu.ChangePlan(); plan == nil || plan.Status != types.PlanStatusUnverified {
		t.Fatalf("parser_error should leave plan unverified, got %+v", plan)
	}
	if result := mu.Result(); !strings.Contains(result, "local verification unavailable for: batch-1 (parser_error)") {
		t.Fatalf("result should carry parser_error unverified caveat, got %q", result)
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

func TestRunWriteControllerWorkflow_ApplyTransportErrorWithAllChangesAppliedContinuesToVerify(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("recover completed apply after transport error")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "recover completed apply"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "make change"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "plan_ready"},
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
	applyCalls := 0
	verifyCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			mu.SetChangePlan(&types.ChangePlan{
				ID:          "plan-apply-transport",
				Status:      types.PlanStatusPending,
				Summary:     "transport recovery",
				TargetPaths: []string{"repository.c"},
				Changes: []types.FileChange{{
					Path:      "repository.c",
					Kind:      "patch",
					Rationale: "preserve error code",
				}},
			})
		case types.StageApply:
			applyCalls++
			plan := mu.ChangePlan()
			if plan == nil {
				t.Fatal("apply stage requires active plan")
			}
			plan.Status = types.PlanStatusApplyFailed
			plan.Changes[0].Apply = &types.FileChangeApplyRecord{Status: "applied"}
			mu.SetChangePlan(plan)
			*stepsUsed++
			return &agent.StageOutput{}, fmt.Errorf("coder stream ended after applying all changes")
		case types.StageVerify:
			verifyCalls++
			mu.SetChangeReport(&types.ChangeReport{
				PlanID:  "plan-apply-transport",
				Channel: types.ChangeReportChannelPostApplyVerify,
				Passed:  true,
			})
		default:
			t.Fatalf("unexpected stage %s", stage)
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if applyCalls != 1 || verifyCalls != 1 {
		t.Fatalf("stage calls apply/verify = %d/%d, want 1/1", applyCalls, verifyCalls)
	}
	if store.last == nil || store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("workflow should complete after typed apply recovery, got %+v", store.last)
	}
	batch := store.last.Batches[0]
	if !workflowBatchHasAttemptArtifact(batch, "apply", "applied", "apply_transport_recovered_all_changes", "refs/codrax/applied/plan-apply-transport") {
		t.Fatalf("recovered apply attempt missing: %+v", batch.Attempts)
	}
	if !workflowBatchHasAttempt(batch, "verify", "passed", "tests_passed", "plan-apply-transport.report.json") {
		t.Fatalf("verify attempt missing after recovered apply: %+v", batch.Attempts)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "apply_transport_recovered_all_changes") ||
		!workflowProgressHasReason(store.last.ProgressLedger, "batch_applied") {
		t.Fatalf("recovery progress missing: %+v", store.last.ProgressLedger)
	}
	if workflowProgressHasReason(store.last.ProgressLedger, "apply_failed") {
		t.Fatalf("completed typed apply must not be recorded as apply_failed: %+v", store.last.ProgressLedger)
	}
	if plan := mu.ChangePlan(); plan == nil || plan.Status != types.PlanStatusApplied {
		t.Fatalf("verified recovered plan status = %+v, want applied", plan)
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

func TestRunWriteControllerWorkflow_AutoExecutableAskUserAppliesPlan(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("safe doc change")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "safe doc change"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "safe doc change"}},
		{Action: writeflow.ActionAskUser, ReasonCode: "model_pause", QuestionsForUser: []string{"Do you approve applying this plan?"}, MissingFacts: []writeflow.MissingUserFact{{
			Kind:        "approval_boundary",
			Description: "controller requested operator approval despite auto-executable plan",
			EvidenceRef: "batch-1",
			Consumer:    "controller",
		}}},
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
	applyCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			mu.SetChangePlan(&types.ChangePlan{
				ID:          "plan-auto-doc",
				Status:      types.PlanStatusPending,
				Summary:     "safe doc change",
				TargetPaths: []string{"docs/guide.md"},
				Changes: []types.FileChange{{
					Path:       "docs/guide.md",
					Kind:       "modify",
					NewContent: "updated docs\n",
					Rationale:  "document the safe behavior",
				}},
			})
		case types.StageApply:
			applyCalls++
		case types.StageVerify:
			mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-auto-doc", Passed: true})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if controllerCalls != 4 {
		t.Fatalf("controller calls = %d, want 4", controllerCalls)
	}
	if applyCalls != 1 {
		t.Fatalf("auto-executable ask_user should dispatch exactly one apply, got %d", applyCalls)
	}
	if store.last == nil || store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("workflow should complete after normalized ask_user, got %+v", store.last)
	}
	if workflowProgressHasReason(store.last.ProgressLedger, string(types.WriteWorkflowBatchPendingApproval)) {
		t.Fatalf("auto-executable plan should not enter pending approval: %+v", store.last.ProgressLedger)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "ask_user_auto_executable_overridden") {
		t.Fatalf("normalization progress should be recorded: %+v", store.last.ProgressLedger)
	}
}

func TestWritePlanCanProceedWithoutApprovalPauseRequiresCurrentApprovalAuthority(t *testing.T) {
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: types.NewMutableState("approval authority")}}
	plan := &types.ChangePlan{
		ID:          "plan-authority",
		Status:      types.PlanStatusPending,
		Summary:     "safe edit",
		TargetPaths: []string{"pkg/fix.py"},
		Changes: []types.FileChange{{
			Path:       "pkg/fix.py",
			Kind:       "modify",
			NewContent: "value = 1\n",
		}},
		Approval: &types.WriteApprovalRecord{
			Action:       string(writeflow.ApprovalActionAutoExecute),
			UserDecision: "auto",
		},
	}
	plan.Approval.PlanFingerprint = types.PlanFingerprint(plan)
	if !o.writePlanCanProceedWithoutApprovalPause(plan) {
		t.Fatal("current auto approval should allow no-pause apply")
	}
	plan.Summary = "changed after approval"
	if o.writePlanCanProceedWithoutApprovalPause(plan) {
		t.Fatal("stale approval fingerprint must not allow no-pause apply")
	}
	plan.Approval.PlanFingerprint = types.PlanFingerprint(plan)
	plan.Approval.RecordFingerprint = "tampered-record"
	if o.writePlanCanProceedWithoutApprovalPause(plan) {
		t.Fatal("tampered approval record must not allow no-pause apply")
	}
	plan.Approval.RecordFingerprint = ""
	plan.Approval.Action = string(writeflow.ApprovalActionManual)
	plan.Approval.UserDecision = "required"
	plan.Approval.PlanFingerprint = types.PlanFingerprint(plan)
	if o.writePlanCanProceedWithoutApprovalPause(plan) {
		t.Fatal("manual approval without user decision must not allow no-pause apply")
	}
	plan.Approval.UserDecision = "approved"
	plan.Approval.PlanFingerprint = types.PlanFingerprint(plan)
	if !o.writePlanCanProceedWithoutApprovalPause(plan) {
		t.Fatal("current approved manual approval should allow apply")
	}
}

func TestEnforceControllerWorkflowTransitionBlocksStaleApprovalApply(t *testing.T) {
	mu := types.NewMutableState("stale approval transition")
	plan := &types.ChangePlan{
		ID:          "plan-stale",
		Status:      types.PlanStatusPending,
		Summary:     "safe edit",
		TargetPaths: []string{"pkg/fix.py"},
		Changes: []types.FileChange{{
			Path:       "pkg/fix.py",
			Kind:       "modify",
			NewContent: "value = 1\n",
		}},
		Approval: &types.WriteApprovalRecord{
			Action:          string(writeflow.ApprovalActionAutoExecute),
			UserDecision:    "auto",
			PlanFingerprint: "stale-fingerprint",
		},
	}
	mu.SetChangePlan(plan)
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-stale-approval",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchPlanned,
			PlanID: "plan-stale",
		}},
	}
	got := o.enforceControllerWorkflowTransition(writeflow.WriteWorkflowDecision{
		Action:     writeflow.ActionApplyPlan,
		ReasonCode: "controller_requested_apply",
	}, run)
	if got.Action != writeflow.ActionAskUser || got.ReasonCode != "approval_authority_invalid" {
		t.Fatalf("stale approval apply should become approval ask_user, got %+v", got)
	}
	if len(got.MissingFacts) != 1 || got.MissingFacts[0].Kind != "approval_boundary" {
		t.Fatalf("ask_user should carry typed approval_boundary fact, got %+v", got.MissingFacts)
	}
	if !workflowProgressHasReason(run.ProgressLedger, "workflow_transition_rejected") {
		t.Fatalf("transition rejection progress missing: %+v", run.ProgressLedger)
	}
	if !workflowProgressHasReason(run.ProgressLedger, "workflow_transition_overridden") {
		t.Fatalf("transition override progress missing: %+v", run.ProgressLedger)
	}
}

func TestEnforceControllerWorkflowTransitionBlocksModePlanApply(t *testing.T) {
	mu := types.NewMutableState("plan mode transition")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModePlan}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-plan-mode",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchPlanned,
		}},
	}
	got := o.enforceControllerWorkflowTransition(writeflow.WriteWorkflowDecision{
		Action:     writeflow.ActionApplyPlan,
		ReasonCode: "bad_plan_mode_apply",
	}, run)
	if got.Action != writeflow.ActionBlock || got.ReasonCode != "action_not_allowed_in_mode" {
		t.Fatalf("mode-plan apply should become typed block, got %+v", got)
	}
	if !workflowProgressHasReason(run.ProgressLedger, "workflow_transition_rejected") {
		t.Fatalf("transition rejection progress missing: %+v", run.ProgressLedger)
	}
}

func TestReviewActiveAppliedPatchScopePersistsHardBlock(t *testing.T) {
	mu := types.NewMutableState("patch review")
	plan := &types.ChangePlan{
		ID:           "plan-review",
		Status:       types.PlanStatusAppliedPendingVerify,
		TargetPaths:  []string{"a.py", "b.py"},
		AppliedPaths: []string{"b.py"},
		Changes: []types.FileChange{
			{Path: "a.py", Kind: "modify", NewContent: "a = 1\n"},
			{Path: "b.py", Kind: "modify", NewContent: "b = 1\n"},
		},
		PatchEffect: &types.PatchEffectRecord{
			RecordID: "patch-effect:plan-review:slice-001:abcdef123456",
			Files: []types.PatchEffectFile{{
				Path:   "b.py",
				Status: "modified",
				Events: []types.PatchEffectEvent{{
					Code:     "control_flow_guard_touched",
					Severity: "warning",
					Path:     "b.py",
				}},
			}},
		},
		Slices: []types.ChangePlanSlice{{
			ID:            "slice-001",
			Status:        types.ChangePlanSliceObserving,
			ChangeIndexes: []int{0},
			Paths:         []string{"a.py"},
		}},
	}
	mu.SetChangePlan(plan)
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-review",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			Status:        types.WriteWorkflowBatchVerifying,
			PlanID:        "plan-review",
			ActiveSliceID: "slice-001",
			Slices: []types.WriteWorkflowSlice{{
				ID:            "slice-001",
				Status:        types.ChangePlanSliceObserving,
				PlanID:        "plan-review",
				Paths:         []string{"a.py"},
				ChangeIndexes: []int{0},
			}},
		}},
	}
	review := o.reviewActiveAppliedPatchScope(run, plan)
	if !review.HardBlock {
		t.Fatalf("outside active slice should hard block, got %+v", review)
	}
	got := mu.ChangePlan()
	if got == nil || got.PatchReview == nil || !got.PatchReview.HardBlock {
		t.Fatalf("patch review should be persisted on active plan, got %+v", got)
	}
	if reason := patchReviewHardBlockReason(review); reason != "applied_path_outside_active_slice:b.py" {
		t.Fatalf("hard-block reason = %q", reason)
	}
	markWorkflowRunActiveSlicePatchReviewBlocked(run, "batch-1", got, patchReviewHardBlockReason(review))
	if run.Batches[0].Slices[0].Status != types.ChangePlanSliceBlocked {
		t.Fatalf("slice should be blocked after patch review hard block, got %+v", run.Batches[0].Slices[0])
	}
	if run.Batches[0].Slices[0].Completion == nil || run.Batches[0].Slices[0].Completion.Source != "patch_review" {
		t.Fatalf("slice completion should carry patch_review source, got %+v", run.Batches[0].Slices[0].Completion)
	}
	if !workflowRunContextContains(run, "patch_review_finding", "control_flow_guard_touched") {
		t.Fatalf("workflow context should carry patch review findings: %+v", run.ContextPacks)
	}
}

func TestReviewActiveAppliedPatchScopeLearnsPatchConvention(t *testing.T) {
	mu := types.NewMutableState("patch convention learner")
	plan := &types.ChangePlan{
		ID:           "plan-convention",
		Status:       types.PlanStatusAppliedPendingVerify,
		TargetPaths:  []string{"pkg/a.py"},
		AppliedPaths: []string{"pkg/a.py"},
		PatchEffect: &types.PatchEffectRecord{
			RecordID: "patch-effect:plan-convention:slice-001:abcdef123456",
			Files: []types.PatchEffectFile{{
				Path:     "pkg/a.py",
				Status:   "modified",
				Language: "py",
				PathRole: types.SourcePathRoleProduction,
			}},
		},
		Slices: []types.ChangePlanSlice{{
			ID:            "slice-001",
			Status:        types.ChangePlanSliceObserving,
			ChangeIndexes: []int{0},
			Paths:         []string{"pkg/a.py"},
		}},
	}
	mu.SetChangePlan(plan)
	reportDir := t.TempDir()
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}, reportDir: reportDir}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-convention-review",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			Status:        types.WriteWorkflowBatchVerifying,
			PlanID:        "plan-convention",
			ActiveSliceID: "slice-001",
			Slices: []types.WriteWorkflowSlice{{
				ID:     "slice-001",
				Status: types.ChangePlanSliceObserving,
				PlanID: "plan-convention",
				Paths:  []string{"pkg/a.py"},
			}},
		}},
	}
	review := o.reviewActiveAppliedPatchScope(run, plan)
	if review.HardBlock {
		t.Fatalf("convention learner finding must not hard block: %+v", review)
	}
	if !patchReviewRecordHasFinding(review, "convention_surface_available") {
		t.Fatalf("learned convention finding missing: %+v", review.Findings)
	}
	if _, err := os.Stat(filepath.Join(reportDir, "workflows", "knowledge", "wf-convention-review", "conventions.json")); err != nil {
		t.Fatalf("learned convention graph should be persisted: %v", err)
	}
	if !workflowRunContextContains(run, "patch_review_finding", "category=convention") {
		t.Fatalf("workflow context should carry convention advisory finding: %+v", run.ContextPacks)
	}
}

func TestNormalizeControllerTypedStateDecisionSuppressesRepeatedAskUserFact(t *testing.T) {
	mu := types.NewMutableState("repeat ask")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-repeat",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
		ProgressLedger: []types.WriteWorkflowProgress{{
			BatchID:    "batch-1",
			Stage:      "controller",
			Status:     string(writeflow.ActionAskUser),
			ReasonCode: "missing_owner",
			FactKeys:   []string{"user_choice|planner|batch-1"},
		}},
	}
	decision := writeflow.WriteWorkflowDecision{
		Action:           writeflow.ActionAskUser,
		ReasonCode:       "missing_owner_again",
		QuestionsForUser: []string{"Different wording should not matter?"},
		MissingFacts: []writeflow.MissingUserFact{{
			Kind:        "user_choice",
			Description: "different prose for the same user-owned fact",
			EvidenceRef: "batch-1",
			Consumer:    "planner",
		}},
	}
	got := o.normalizeControllerTypedStateDecision(decision, run)
	if got.Action != writeflow.ActionBlock || got.ReasonCode != "repeated_user_fact_request" {
		t.Fatalf("repeated ask_user fact should become typed block, got %+v", got)
	}
	if !workflowProgressHasReason(run.ProgressLedger, "ask_user_repeated_suppressed") {
		t.Fatalf("suppression progress missing: %+v", run.ProgressLedger)
	}
}

func TestNormalizeControllerTypedStateDecisionAskUserBudgetBlocksDistinctFacts(t *testing.T) {
	mu := types.NewMutableState("ask budget")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-ask-budget",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
		ProgressLedger: []types.WriteWorkflowProgress{{
			BatchID:    "batch-1",
			Stage:      "controller",
			Status:     string(writeflow.ActionAskUser),
			ReasonCode: "first_missing_fact",
			FactKeys:   []string{"scope_boundary|planner|docs/a.md:1"},
		}},
	}
	decision := writeflow.WriteWorkflowDecision{
		Action:           writeflow.ActionAskUser,
		ReasonCode:       "second_missing_fact",
		QuestionsForUser: []string{"Which external service owns this callback?"},
		MissingFacts: []writeflow.MissingUserFact{{
			Kind:        "external_fact",
			Description: "service owner for callback",
			EvidenceRef: "docs/b.md:2",
			Consumer:    "planner",
		}},
	}
	got := o.normalizeControllerTypedStateDecision(decision, run)
	if got.Action != writeflow.ActionBlock || got.ReasonCode != "ask_user_budget_exhausted" {
		t.Fatalf("second distinct ask_user should exhaust budget, got %+v", got)
	}
	if !workflowProgressHasReason(run.ProgressLedger, "ask_user_budget_exhausted") {
		t.Fatalf("budget progress missing: %+v", run.ProgressLedger)
	}
}

func TestNormalizeControllerTypedStateDecisionReplanAfterNoTestsFinishesUnverified(t *testing.T) {
	mu := types.NewMutableState("no tests replan guard")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-no-tests",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: "plan-1"},
				{Kind: "verify", Status: "unverified", ReasonCode: "no_tests", PlanID: "plan-1"},
			},
		}},
	}
	decision := writeflow.WriteWorkflowDecision{
		Action: writeflow.ActionReplanBatch,
		Batch:  &writeflow.WriteBatchPlan{ID: "batch-2", Goal: "try again"},
	}
	got := o.normalizeControllerTypedStateDecision(decision, run)
	if got.Action != writeflow.ActionFinish || got.FinishDisposition != writeflow.FinishDispositionAcceptUnverified {
		t.Fatalf("no-tests replan should finish with unverified caveat, got %+v", got)
	}
	if !workflowProgressHasReason(run.ProgressLedger, "replan_without_failure_evidence_overridden") {
		t.Fatalf("override progress missing: %+v", run.ProgressLedger)
	}
}

func TestNormalizeControllerTypedStateDecisionReplanAfterRunnerMissingFinishesUnverified(t *testing.T) {
	mu := types.NewMutableState("runner missing replan guard")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-runner-missing",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: "plan-1"},
				{Kind: "verify", Status: "unverified", ReasonCode: "runner_missing", PlanID: "plan-1"},
			},
		}},
	}
	decision := writeflow.WriteWorkflowDecision{
		Action: writeflow.ActionReplanBatch,
		Batch:  &writeflow.WriteBatchPlan{ID: "batch-2", Goal: "try again"},
	}
	got := o.normalizeControllerTypedStateDecision(decision, run)
	if got.Action != writeflow.ActionFinish || got.FinishDisposition != writeflow.FinishDispositionAcceptUnverified {
		t.Fatalf("runner-missing replan should finish with unverified caveat, got %+v", got)
	}
	if !workflowProgressHasReason(run.ProgressLedger, "replan_without_failure_evidence_overridden") {
		t.Fatalf("override progress missing: %+v", run.ProgressLedger)
	}
}

func TestNormalizeControllerTypedStateDecisionRunnerMissingSuppressesInterruptions(t *testing.T) {
	for _, reasonCode := range []string{"runner_missing", "parser_error"} {
		for _, action := range []writeflow.WorkflowAction{
			writeflow.ActionExploreCode,
			writeflow.ActionPlanBatch,
			writeflow.ActionApplyPlan,
			writeflow.ActionVerifyBatch,
			writeflow.ActionAskUser,
			writeflow.ActionBlock,
			writeflow.ActionReplanBatch,
		} {
			t.Run(reasonCode+"/"+string(action), func(t *testing.T) {
				mu := types.NewMutableState("runner missing interruption guard")
				o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
				run := &types.WriteWorkflowRun{
					RunID:         "wf-unverified",
					Status:        types.WriteWorkflowRunInProgress,
					ActiveBatchID: "batch-1",
					Batches: []types.WriteWorkflowBatch{{
						ID:     "batch-1",
						Status: types.WriteWorkflowBatchComplete,
						Attempts: []types.WriteWorkflowAttempt{
							{Kind: "apply", Status: "applied", PlanID: "plan-1"},
							{Kind: "verify", Status: "unverified", ReasonCode: reasonCode, PlanID: "plan-1"},
						},
					}},
				}
				got := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{
					Action:     action,
					ReasonCode: reasonCode,
				}, run)
				if got.Action != writeflow.ActionFinish || got.FinishDisposition != writeflow.FinishDispositionAcceptUnverified {
					t.Fatalf("%s should finish with unverified caveat, got %+v", action, got)
				}
				if action == writeflow.ActionReplanBatch {
					if !workflowProgressHasReason(run.ProgressLedger, "replan_without_failure_evidence_overridden") {
						t.Fatalf("replan override progress missing: %+v", run.ProgressLedger)
					}
				} else if !workflowProgressHasReason(run.ProgressLedger, "unverified_action_overridden") {
					t.Fatalf("override progress missing: %+v", run.ProgressLedger)
				}
			})
		}
	}
}

func TestNormalizeControllerTypedStateDecisionSemanticPatchReviewAppendsFollowup(t *testing.T) {
	mu := types.NewMutableState("semantic patch review followup")
	plan := &types.ChangePlan{
		ID:     "plan-semantic",
		Status: types.PlanStatusUnverified,
		PatchReview: &types.PatchReviewRecord{
			Findings: []types.PatchReviewFinding{{
				Code:           "changed_symbol_without_probe_coverage",
				Severity:       types.PatchReviewSeverityWarning,
				Category:       types.PatchReviewCategorySemanticCoverage,
				Path:           "pkg/axis.py",
				SubjectSymbol:  "Axis.convert",
				CoverageStatus: types.PatchReviewCoverageUnverified,
			}},
		},
	}
	mu.SetChangePlan(plan)
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-semantic",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: "plan-semantic"},
				{Kind: "verify", Status: "unverified", ReasonCode: "parser_error", PlanID: "plan-semantic"},
			},
		}},
	}
	got := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{
		Action:            writeflow.ActionFinish,
		ReasonCode:        "done",
		FinishDisposition: writeflow.FinishDispositionAcceptUnverified,
	}, run)
	if got.Action != writeflow.ActionAppendBatch || got.Batch == nil || got.Batch.ID != "batch-1-impact-repair" {
		t.Fatalf("semantic patch review should append impact repair batch, got %+v", got)
	}
	if !strings.Contains(got.Batch.Goal, "pkg/axis.py") {
		t.Fatalf("follow-up goal should carry typed path scope, got %q", got.Batch.Goal)
	}
	if !containsString(got.Batch.ExpectedPaths, "pkg/axis.py") {
		t.Fatalf("follow-up batch should carry typed expected path, got %+v", got.Batch.ExpectedPaths)
	}
	if !workflowProgressHasReason(run.ProgressLedger, "impact_obligation_followup_requested") {
		t.Fatalf("impact repair follow-up progress missing: %+v", run.ProgressLedger)
	}
}

func TestNormalizeControllerTypedStateDecisionVerifiedButUndercoveredAppendsImpactRepair(t *testing.T) {
	mu := types.NewMutableState("verified but undercovered")
	plan := coverageProjectionPlanForTest()
	plan.Status = types.PlanStatusApplied
	mu.SetChangePlan(plan)
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-verified-undercovered",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: "plan-coverage"},
				{Kind: "verify", Status: "passed", ReasonCode: "tests_passed", PlanID: "plan-coverage"},
			},
		}},
	}

	got := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{
		Action:     writeflow.ActionFinish,
		ReasonCode: "done",
	}, run)

	if got.Action != writeflow.ActionAppendBatch || got.Batch == nil || got.Batch.ID != "batch-1-impact-repair" {
		t.Fatalf("verified-but-undercovered patch should append impact repair batch, got %+v", got)
	}
	if !containsString(got.Batch.ExpectedPaths, "pkg/axis.py") {
		t.Fatalf("impact repair should be scoped to the typed changed path, got %+v", got.Batch.ExpectedPaths)
	}
	if !strings.Contains(strings.Join(got.Batch.SuccessCriteria, "\n"), "changed_symbol_without_probe_coverage") {
		t.Fatalf("impact repair criteria should preserve typed finding code, got %+v", got.Batch.SuccessCriteria)
	}
	if !workflowProgressHasReason(run.ProgressLedger, "impact_obligation_followup_requested") {
		t.Fatalf("impact repair progress missing: %+v", run.ProgressLedger)
	}
}

func TestNormalizeControllerTypedStateDecisionMissingSoftProofAppendsProofFollowup(t *testing.T) {
	mu := types.NewMutableState("verified but soft proof missing")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-soft-proof",
		Status:      types.PlanStatusApplied,
		TargetPaths: []string{"pkg/axis.py", "tests/test_axis.py"},
	})
	mu.SetChangeReport(&types.ChangeReport{
		PlanID:             "plan-soft-proof",
		Passed:             true,
		VerificationStatus: types.VerificationStatusPassed,
		VerificationConfidence: []types.VerificationConfidenceRecord{{
			Source:       "verification_probe",
			Category:     "probe_soft_contract_refs",
			Status:       "missing",
			Severity:     "warning",
			ReasonCode:   "verification_probe_missing_soft_contract_ref",
			ContractRefs: []string{"soft-outcome"},
		}},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-soft-proof",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: "plan-soft-proof"},
				{Kind: "verify", Status: "passed", ReasonCode: "tests_passed", PlanID: "plan-soft-proof"},
			},
		}},
	}

	got := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{
		Action:     writeflow.ActionFinish,
		ReasonCode: "done",
	}, run)

	if got.Action != writeflow.ActionAppendBatch || got.Batch == nil || got.Batch.ID != "batch-1-proof-repair" {
		t.Fatalf("missing soft proof should append proof repair batch, got %+v", got)
	}
	if got.Batch.Purpose != "verification_proof_followup" {
		t.Fatalf("proof follow-up purpose not preserved, got %q", got.Batch.Purpose)
	}
	if !containsString(got.Batch.ExpectedPaths, "pkg/axis.py") || containsString(got.Batch.ExpectedPaths, "tests/test_axis.py") {
		t.Fatalf("proof follow-up should scope to production plan paths only, got %+v", got.Batch.ExpectedPaths)
	}
	criteria := strings.Join(got.Batch.SuccessCriteria, "\n")
	for _, want := range []string{
		"code=verification_probe_missing_soft_contract_ref",
		"contract_ref=soft-outcome",
		"source=verification_confidence",
	} {
		if !strings.Contains(criteria, want) {
			t.Fatalf("proof follow-up criteria missing %q: %+v", want, got.Batch.SuccessCriteria)
		}
	}
	if !workflowProgressHasReason(run.ProgressLedger, "verification_proof_followup_requested") {
		t.Fatalf("proof follow-up progress missing: %+v", run.ProgressLedger)
	}
}

func TestNormalizeControllerTypedStateDecisionMissingProofWithoutRefsDoesNotAppendBlindBatch(t *testing.T) {
	mu := types.NewMutableState("verified but proof record has no refs")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-no-proof-ref",
		Status:      types.PlanStatusApplied,
		TargetPaths: []string{"pkg/axis.py"},
	})
	mu.SetChangeReport(&types.ChangeReport{
		PlanID:             "plan-no-proof-ref",
		Passed:             true,
		VerificationStatus: types.VerificationStatusPassed,
		VerificationConfidence: []types.VerificationConfidenceRecord{{
			Source:     "verification_probe",
			Category:   "probe_soft_contract_refs",
			Status:     "missing",
			Severity:   "warning",
			ReasonCode: "verification_probe_missing_soft_contract_ref",
		}},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-no-proof-ref",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: "plan-no-proof-ref"},
				{Kind: "verify", Status: "passed", ReasonCode: "tests_passed", PlanID: "plan-no-proof-ref"},
			},
		}},
	}

	got := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{
		Action:     writeflow.ActionFinish,
		ReasonCode: "done",
	}, run)

	if got.Action != writeflow.ActionFinish {
		t.Fatalf("proof records without typed refs should remain telemetry, got %+v", got)
	}
}

func TestNormalizeControllerTypedStateDecisionProofFollowupDoesNotRecurse(t *testing.T) {
	mu := types.NewMutableState("proof followup recursion")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-proof-recursion",
		Status:      types.PlanStatusApplied,
		TargetPaths: []string{"pkg/axis.py"},
	})
	mu.SetChangeReport(&types.ChangeReport{
		PlanID:             "plan-proof-recursion",
		Passed:             true,
		VerificationStatus: types.VerificationStatusPassed,
		VerificationConfidence: []types.VerificationConfidenceRecord{{
			Source:       "verification_probe",
			Category:     "probe_contract_refs",
			Status:       "missing",
			Severity:     "warning",
			ReasonCode:   "verification_probe_missing_required_contract_ref",
			ContractRefs: []string{"hard-outcome"},
		}},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-proof-recursion",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1-proof-repair",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1-proof-repair",
			Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: "plan-proof-recursion"},
				{Kind: "verify", Status: "passed", ReasonCode: "tests_passed", PlanID: "plan-proof-recursion"},
			},
		}},
		ProgressLedger: []types.WriteWorkflowProgress{{
			BatchID:    "batch-1",
			ReasonCode: "verification_proof_followup_requested",
		}},
	}

	got := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{
		Action:     writeflow.ActionFinish,
		ReasonCode: "done",
	}, run)

	if got.Action != writeflow.ActionFinish {
		t.Fatalf("proof follow-up should not recursively append another batch, got %+v", got)
	}
}

func TestEnrichProofFollowupPlanProbeRefsBindsDurableContractRefs(t *testing.T) {
	mu := types.NewMutableState("proof followup probe refs")
	run := &types.WriteWorkflowRun{
		RunID:         "wf-proof-bind",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1-proof-repair",
		Batches: []types.WriteWorkflowBatch{{
			ID:      "batch-1-proof-repair",
			Purpose: "verification_proof_followup",
			SuccessCriteria: []string{
				"impact_obligation=soft-1 kind=behavior_contract code=verification_probe_missing_soft_contract_ref paths=pkg/widget.py contract_ref=outcome-1 evidence_ref=verification_probe:probe_soft_contract_refs source=verification_confidence",
				"impact_obligation=soft-2 kind=behavior_contract contract_ref=missing-contract",
				"impact_obligation=soft-3 kind=behavior_contract contract_ref=outcome-2",
			},
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
		ProgressLedger: []types.WriteWorkflowProgress{{
			BatchID:    "batch-1",
			ReasonCode: "verification_proof_followup_requested",
		}},
	}
	mu.SetWriteWorkflowRun(run)
	plan := &types.ChangePlan{
		ID:          "plan-proof-bind",
		TargetPaths: []string{"pkg/widget.py", "tests/test_widget.py"},
		Changes: []types.FileChange{{
			Path: "pkg/widget.py",
			Kind: "patch",
		}},
		BehaviorContracts: []types.WriteBehaviorContract{
			{ID: "outcome-1", Kind: types.WriteBehaviorObservable},
			{ID: "outcome-2", Kind: types.WriteBehaviorObservable},
		},
		VerificationProbes: []types.VerificationProbe{{
			ID:       "probe-1",
			Language: "python",
			Code:     "pass",
		}},
	}
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}

	if !o.enrichProofFollowupPlanProbeRefs(nil, plan) {
		t.Fatal("proof follow-up plan should receive deterministic probe refs")
	}
	if !containsString(plan.VerificationProbes[0].ContractRefs, "outcome-1") ||
		!containsString(plan.VerificationProbes[0].ContractRefs, "outcome-2") ||
		containsString(plan.VerificationProbes[0].ContractRefs, "missing-contract") {
		t.Fatalf("contract refs should bind only criteria refs known to behavior contracts, got %+v", plan.VerificationProbes[0].ContractRefs)
	}
	if got := plan.VerificationProbes[0].ChangedSymbolRefs; len(got) != 1 || got[0] != "path:pkg/widget.py" {
		t.Fatalf("single production target should seed changed-symbol fallback, got %+v", got)
	}
}

func TestEnrichProofFollowupPlanProbeRefsRefreshesAutoApprovalFingerprint(t *testing.T) {
	mu := types.NewMutableState("proof followup approval refresh")
	run := &types.WriteWorkflowRun{
		RunID:         "wf-proof-refresh",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1-proof-repair",
		Batches: []types.WriteWorkflowBatch{{
			ID:      "batch-1-proof-repair",
			Purpose: "verification_proof_followup",
			SuccessCriteria: []string{
				"impact_obligation=soft-1 kind=behavior_contract code=verification_probe_missing_soft_contract_ref paths=pkg/widget.py contract_ref=outcome-1 evidence_ref=verification_probe:probe_soft_contract_refs source=verification_confidence",
			},
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
		ProgressLedger: []types.WriteWorkflowProgress{{
			BatchID:    "batch-1",
			ReasonCode: "verification_proof_followup_requested",
		}},
	}
	mu.SetWriteWorkflowRun(run)
	plan := &types.ChangePlan{
		ID:          "plan-proof-refresh",
		Status:      types.PlanStatusPending,
		Summary:     "refresh auto approval after deterministic proof binding",
		TargetPaths: []string{"pkg/widget.py"},
		Changes: []types.FileChange{{
			Path:       "pkg/widget.py",
			Kind:       "modify",
			NewContent: "VALUE = 1\n",
		}},
		BehaviorContracts: []types.WriteBehaviorContract{
			{ID: "outcome-1", Kind: types.WriteBehaviorObservable},
		},
		VerificationProbes: []types.VerificationProbe{{
			ID:       "probe-1",
			Language: "python",
			Code:     "assert True",
		}},
	}
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	assessment := writeflow.AssessWriteRisk(o.writeRiskAssessmentInput(plan))
	decision := writeflow.DecideWriteApproval(writeflow.ApprovalPolicyAutoSafe, assessment)
	if decision.Action != writeflow.ApprovalActionAutoExecute {
		t.Fatalf("test fixture should be auto executable, got %+v", decision)
	}
	plan.Approval = writeflow.NewApprovalRecord(assessment, decision, "plan_post_hook", "auto", types.PlanFingerprint(plan), "")
	priorFingerprint := plan.Approval.PlanFingerprint

	if !o.enrichProofFollowupPlanProbeRefs(nil, plan) {
		t.Fatal("proof follow-up plan should receive deterministic probe refs")
	}
	currentFingerprint := types.PlanFingerprint(plan)
	if currentFingerprint == priorFingerprint {
		t.Fatal("test fixture should mutate the apply-relevant plan fingerprint")
	}
	if plan.Approval == nil || plan.Approval.PlanFingerprint != currentFingerprint {
		t.Fatalf("auto approval fingerprint was not refreshed: approval=%+v current=%s", plan.Approval, currentFingerprint)
	}
	if plan.Approval.Source != "proof_followup_probe_ref_enrichment" {
		t.Fatalf("approval refresh source = %q", plan.Approval.Source)
	}
	view := writeflow.DeriveApprovalExecutionView(plan)
	if view.State != writeflow.ApprovalExecutionAutoAllowed {
		t.Fatalf("refreshed approval should be auto allowed, got %+v", view)
	}
}

func TestEnrichProofFollowupPlanProbeRefsRequiresAuthorizedDurableBatch(t *testing.T) {
	mu := types.NewMutableState("proof followup unauthorized")
	mu.SetWriteWorkflowRun(&types.WriteWorkflowRun{
		RunID:         "wf-proof-unauthorized",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:              "batch-1",
			Purpose:         "verification_proof_followup",
			SuccessCriteria: []string{"contract_ref=outcome-1"},
			Status:          types.WriteWorkflowBatchReadyToPlan,
		}},
	})
	plan := &types.ChangePlan{
		ID:                "plan-proof-unauthorized",
		TargetPaths:       []string{"pkg/widget.py"},
		BehaviorContracts: []types.WriteBehaviorContract{{ID: "outcome-1", Kind: types.WriteBehaviorObservable}},
		VerificationProbes: []types.VerificationProbe{{
			ID:       "probe-1",
			Language: "python",
			Code:     "pass",
		}},
	}
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}

	if o.enrichProofFollowupPlanProbeRefs(nil, plan) {
		t.Fatal("proof refs must not bind without the controller proof-follow-up ledger reason")
	}
	if len(plan.VerificationProbes[0].ContractRefs) != 0 || len(plan.VerificationProbes[0].ChangedSymbolRefs) != 0 {
		t.Fatalf("unauthorized plan should remain unchanged, got %+v", plan.VerificationProbes[0])
	}
}

func TestEnrichProofFollowupPlanProbeRefsDoesNotGuessAcrossMultipleProbes(t *testing.T) {
	mu := types.NewMutableState("proof followup multiple probes")
	mu.SetWriteWorkflowRun(&types.WriteWorkflowRun{
		RunID:         "wf-proof-multi-probe",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1-proof-repair",
		Batches: []types.WriteWorkflowBatch{{
			ID:              "batch-1-proof-repair",
			Purpose:         "verification_proof_followup",
			SuccessCriteria: []string{"contract_ref=outcome-1"},
			Status:          types.WriteWorkflowBatchReadyToPlan,
		}},
		ProgressLedger: []types.WriteWorkflowProgress{{
			BatchID:    "batch-1",
			ReasonCode: "verification_proof_followup_requested",
		}},
	})
	plan := &types.ChangePlan{
		ID:                "plan-proof-multi-probe",
		TargetPaths:       []string{"pkg/widget.py"},
		BehaviorContracts: []types.WriteBehaviorContract{{ID: "outcome-1", Kind: types.WriteBehaviorObservable}},
		VerificationProbes: []types.VerificationProbe{
			{ID: "probe-1", Language: "python", Code: "pass"},
			{ID: "probe-2", Language: "python", Code: "pass"},
		},
	}
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}

	if o.enrichProofFollowupPlanProbeRefs(nil, plan) {
		t.Fatal("multiple probes should not receive guessed proof refs")
	}
	if len(plan.VerificationProbes[0].ContractRefs) != 0 || len(plan.VerificationProbes[1].ContractRefs) != 0 {
		t.Fatalf("multiple probes should remain unchanged, got %+v", plan.VerificationProbes)
	}
}

func TestCompleteDispatchInterruptedRunIfAllBatchesCompleteFinishesVerifiedRun(t *testing.T) {
	mu := types.NewMutableState("complete after interrupted dispatch")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-dispatch-complete",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{{
				Kind:       "verify",
				Status:     "passed",
				ReasonCode: "tests_passed",
				PlanID:     "plan-1",
			}},
		}},
	}

	if !o.completeDispatchInterruptedRunIfAllBatchesComplete(run, "controller_dispatch_interrupted_after_complete") {
		t.Fatal("complete run should terminalize when controller dispatch is interrupted")
	}
	if run.Status != types.WriteWorkflowRunComplete || run.Completion == nil ||
		run.Completion.Verdict != types.WriteWorkflowCompletionVerified {
		t.Fatalf("run should be complete and verified, got status=%s completion=%+v", run.Status, run.Completion)
	}
	if !workflowProgressHasReason(run.ProgressLedger, "controller_dispatch_interrupted_after_complete") {
		t.Fatalf("terminalization progress missing: %+v", run.ProgressLedger)
	}
	if got := mu.WriteWorkflowRun(); got == nil || got.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("mutable workflow run should be complete, got %+v", got)
	}
}

func TestCompleteDispatchInterruptedRunIfAllBatchesCompleteDoesNotSwallowProofFollowup(t *testing.T) {
	mu := types.NewMutableState("interrupted dispatch with proof followup")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-proof-needed",
		Status:      types.PlanStatusApplied,
		TargetPaths: []string{"pkg/widget.py"},
	})
	mu.SetChangeReport(&types.ChangeReport{
		PlanID:             "plan-proof-needed",
		Passed:             true,
		VerificationStatus: types.VerificationStatusPassed,
		VerificationConfidence: []types.VerificationConfidenceRecord{{
			Source:       "verification_probe",
			Category:     "probe_contract_refs",
			Status:       "missing",
			Severity:     "error",
			ReasonCode:   "verification_probe_missing_required_contract_ref",
			ContractRefs: []string{"outcome-1"},
		}},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-proof-needed",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{{
				Kind:       "verify",
				Status:     "passed",
				ReasonCode: "tests_passed",
				PlanID:     "plan-proof-needed",
			}},
		}},
	}

	if o.completeDispatchInterruptedRunIfAllBatchesComplete(run, "controller_dispatch_interrupted_after_complete") {
		t.Fatal("pending typed proof follow-up should not be swallowed by interrupted-dispatch terminalization")
	}
	if run.Status != types.WriteWorkflowRunInProgress {
		t.Fatalf("run should remain in progress when proof follow-up is pending, got %s", run.Status)
	}
	if workflowProgressHasReason(run.ProgressLedger, "verification_proof_followup_requested") {
		t.Fatalf("follow-up probe should run on a cloned decision state, not mutate original run: %+v", run.ProgressLedger)
	}
}

func TestNormalizeControllerTypedStateDecisionVerifiedSoftTelemetryOnlyFinishes(t *testing.T) {
	mu := types.NewMutableState("verified soft telemetry only")
	mu.SetChangePlan(&types.ChangePlan{
		ID:     "plan-soft-telemetry",
		Status: types.PlanStatusApplied,
		PatchReview: &types.PatchReviewRecord{
			Findings: []types.PatchReviewFinding{{
				Code:           "dependent_surface_without_verify_coverage",
				Severity:       types.PatchReviewSeverityWarning,
				Category:       types.PatchReviewCategorySemanticCoverage,
				Path:           "pkg/axis.py",
				RelatedPath:    "pkg/caller.py",
				CoverageStatus: types.PatchReviewCoverageUnverified,
				EvidenceRef:    "pkg/caller.py->pkg/axis.py",
			}, {
				Code:           "related_test_surface_unverified",
				Severity:       types.PatchReviewSeverityWarning,
				Category:       types.PatchReviewCategorySemanticCoverage,
				Path:           "pkg/axis.py",
				RelatedPath:    "tests/test_axis.py",
				CoverageStatus: types.PatchReviewCoverageUnverified,
				EvidenceRef:    "tests/test_axis.py",
			}},
		},
		ImpactAnalysis: &types.ImpactAnalysisResult{
			VerificationTargets: []types.ImpactVerificationTarget{{
				Kind:           "dependent",
				Path:           "pkg/axis.py",
				RelatedPath:    "pkg/caller.py",
				Priority:       40,
				Source:         "impact_engine",
				CoverageStatus: "unverified",
				EvidenceRef:    "pkg/caller.py->pkg/axis.py",
			}, {
				Kind:           "test_surface",
				Path:           "pkg/axis.py",
				RelatedPath:    "tests/test_axis.py",
				Priority:       50,
				Source:         "impact_engine",
				CoverageStatus: "unverified",
				EvidenceRef:    "tests/test_axis.py",
			}, {
				Kind:           "dependency",
				Path:           "pkg/axis.py",
				RelatedPath:    "pkg/base.py",
				Priority:       60,
				Source:         "impact_engine",
				CoverageStatus: "unverified",
				EvidenceRef:    "pkg/axis.py->pkg/base.py",
			}, {
				Kind:           "effect_followup",
				Path:           "pkg/axis.py",
				Priority:       70,
				Source:         "patch_effect",
				CoverageStatus: "unverified",
				EvidenceRef:    "patch-effect:pkg/axis.py",
			}},
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-soft-telemetry",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: "plan-soft-telemetry"},
				{Kind: "verify", Status: "passed", ReasonCode: "tests_passed", PlanID: "plan-soft-telemetry"},
			},
		}},
	}

	got := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{
		Action:     writeflow.ActionFinish,
		ReasonCode: "done",
	}, run)

	if got.Action != writeflow.ActionFinish {
		t.Fatalf("soft telemetry should not append a code follow-up after passed verify, got %+v", got)
	}
	if workflowProgressHasReason(run.ProgressLedger, "impact_obligation_followup_requested") {
		t.Fatalf("soft telemetry should remain handoff evidence, not a repair batch: %+v", run.ProgressLedger)
	}
}

func TestNormalizeControllerTypedStateDecisionVerifiedActualDiffBoundaryAppendsFollowup(t *testing.T) {
	mu := types.NewMutableState("verified actual-diff boundary")
	mu.SetChangePlan(&types.ChangePlan{
		ID:     "plan-boundary",
		Status: types.PlanStatusApplied,
		PatchReview: &types.PatchReviewRecord{
			Findings: []types.PatchReviewFinding{{
				Code:           "typescript_nested_string_key_direct_access_added",
				Severity:       types.PatchReviewSeverityWarning,
				Category:       types.PatchReviewCategorySemanticCoverage,
				Path:           "src/widget.ts",
				CoverageStatus: types.PatchReviewCoverageUnknown,
				EvidenceRef:    "src/widget.ts:42",
			}},
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-boundary",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: "plan-boundary"},
				{Kind: "verify", Status: "passed", ReasonCode: "tests_passed", PlanID: "plan-boundary"},
			},
		}},
	}

	got := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{
		Action:     writeflow.ActionFinish,
		ReasonCode: "done",
	}, run)

	if got.Action != writeflow.ActionAppendBatch || got.Batch == nil {
		t.Fatalf("actual-diff boundary finding should append impact repair batch, got %+v", got)
	}
	if !containsString(got.Batch.ExpectedPaths, "src/widget.ts") {
		t.Fatalf("boundary follow-up should stay scoped to typed path, got %+v", got.Batch.ExpectedPaths)
	}
	if !strings.Contains(strings.Join(got.Batch.SuccessCriteria, "\n"), "typescript_nested_string_key_direct_access_added") {
		t.Fatalf("boundary follow-up should preserve typed event code, got %+v", got.Batch.SuccessCriteria)
	}
}

func TestNormalizeControllerTypedStateDecisionImpactRepairUsesTypedImpactKind(t *testing.T) {
	mu := types.NewMutableState("verified typed impact kind")
	mu.SetChangePlan(&types.ChangePlan{
		ID:     "plan-impact-kind",
		Status: types.PlanStatusApplied,
		PatchReview: &types.PatchReviewRecord{
			Findings: []types.PatchReviewFinding{{
				Code:           "custom_owner_boundary_gap",
				Severity:       types.PatchReviewSeverityWarning,
				Category:       types.PatchReviewCategorySemanticCoverage,
				ImpactKind:     types.PatchReviewImpactKindChangedSymbol,
				Path:           "pkg/axis.py",
				SubjectSymbol:  "Axis.convert",
				CoverageStatus: types.PatchReviewCoverageUnverified,
				EvidenceRef:    "pkg/axis.py:12",
			}},
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-impact-kind",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: "plan-impact-kind"},
				{Kind: "verify", Status: "passed", ReasonCode: "tests_passed", PlanID: "plan-impact-kind"},
			},
		}},
	}

	got := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{
		Action:     writeflow.ActionFinish,
		ReasonCode: "done",
	}, run)

	if got.Action != writeflow.ActionAppendBatch || got.Batch == nil {
		t.Fatalf("typed impact kind should append impact repair batch, got %+v", got)
	}
	if !strings.Contains(strings.Join(got.Batch.SuccessCriteria, "\n"), "kind=changed_symbol") ||
		!strings.Contains(strings.Join(got.Batch.SuccessCriteria, "\n"), "symbol=Axis.convert") {
		t.Fatalf("impact repair should preserve typed impact kind and symbol, got %+v", got.Batch.SuccessCriteria)
	}
}

func TestNormalizeControllerTypedStateDecisionImpactRepairWithoutPathExploresFirst(t *testing.T) {
	mu := types.NewMutableState("impact repair needs localization")
	mu.SetChangePlan(&types.ChangePlan{
		ID:     "plan-contract",
		Status: types.PlanStatusApplied,
		PatchReview: &types.PatchReviewRecord{
			Findings: []types.PatchReviewFinding{{
				Code:           "behavior_contract_without_verify_coverage",
				Severity:       types.PatchReviewSeverityWarning,
				Category:       types.PatchReviewCategorySemanticCoverage,
				CoverageStatus: types.PatchReviewCoverageUnverified,
				EvidenceRef:    "contract-1",
			}},
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-contract-undercovered",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: "plan-contract"},
				{Kind: "verify", Status: "passed", ReasonCode: "tests_passed", PlanID: "plan-contract"},
			},
		}},
	}

	got := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{
		Action:     writeflow.ActionFinish,
		ReasonCode: "done",
	}, run)

	if got.Action != writeflow.ActionExploreCode || got.ExplorationRequest == nil {
		t.Fatalf("pathless impact repair should explore before planning, got %+v", got)
	}
	if got.ExplorationRequest.BatchID != "batch-1-impact-repair" {
		t.Fatalf("exploration should target the impact repair batch, got %+v", got.ExplorationRequest)
	}
	if !strings.Contains(strings.Join(got.ExplorationRequest.EvidenceRequirements, "\n"), "behavior_contract_without_verify_coverage") {
		t.Fatalf("exploration requirements should preserve typed finding code, got %+v", got.ExplorationRequest.EvidenceRequirements)
	}
}

func TestNormalizeControllerTypedStateDecisionSemanticPatchReviewFollowupRunsOnce(t *testing.T) {
	mu := types.NewMutableState("semantic patch review followup once")
	mu.SetChangePlan(&types.ChangePlan{
		ID:     "plan-semantic",
		Status: types.PlanStatusUnverified,
		PatchReview: &types.PatchReviewRecord{
			Findings: []types.PatchReviewFinding{{
				Code:           "dependent_surface_without_verify_coverage",
				Severity:       types.PatchReviewSeverityWarning,
				Category:       types.PatchReviewCategorySemanticCoverage,
				Path:           "pkg/axis.py",
				RelatedPath:    "pkg/caller.py",
				CoverageStatus: types.PatchReviewCoverageUnavailable,
			}},
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-semantic-once",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: "plan-semantic"},
				{Kind: "verify", Status: "unverified", ReasonCode: "parser_error", PlanID: "plan-semantic"},
			},
		}, {
			ID: "batch-1-semantic-review",
		}},
		ProgressLedger: []types.WriteWorkflowProgress{{
			BatchID:    "batch-1",
			ReasonCode: "impact_obligation_followup_requested",
		}},
	}
	got := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{
		Action:            writeflow.ActionFinish,
		ReasonCode:        "done",
		FinishDisposition: writeflow.FinishDispositionAcceptUnverified,
	}, run)
	if got.Action != writeflow.ActionFinish || got.FinishDisposition != writeflow.FinishDispositionAcceptUnverified {
		t.Fatalf("semantic follow-up should run once, then allow unverified finish, got %+v", got)
	}
}

func TestNormalizeControllerTypedStateDecisionLegacySemanticFollowupReasonPreventsImpactRepairRecursion(t *testing.T) {
	mu := types.NewMutableState("legacy semantic reason prevents impact recursion")
	mu.SetChangePlan(&types.ChangePlan{
		ID:     "plan-semantic",
		Status: types.PlanStatusUnverified,
		PatchReview: &types.PatchReviewRecord{
			Findings: []types.PatchReviewFinding{{
				Code:           "dependent_surface_without_verify_coverage",
				Severity:       types.PatchReviewSeverityWarning,
				Category:       types.PatchReviewCategorySemanticCoverage,
				Path:           "pkg/axis.py",
				RelatedPath:    "pkg/caller.py",
				CoverageStatus: types.PatchReviewCoverageUnavailable,
			}},
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-semantic-legacy-once",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: "plan-semantic"},
				{Kind: "verify", Status: "unverified", ReasonCode: "parser_error", PlanID: "plan-semantic"},
			},
		}},
		ProgressLedger: []types.WriteWorkflowProgress{{
			BatchID:    "batch-1",
			ReasonCode: "semantic_patch_review_followup_requested",
		}},
	}
	got := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{
		Action:            writeflow.ActionFinish,
		ReasonCode:        "done",
		FinishDisposition: writeflow.FinishDispositionAcceptUnverified,
	}, run)
	if got.Action != writeflow.ActionFinish || got.FinishDisposition != writeflow.FinishDispositionAcceptUnverified {
		t.Fatalf("legacy semantic follow-up reason should suppress impact repair recursion, got %+v", got)
	}
}

func TestNormalizeControllerTypedStateDecisionSemanticPatchReviewIgnoresCoveredAdvisory(t *testing.T) {
	mu := types.NewMutableState("semantic patch review covered")
	mu.SetChangePlan(&types.ChangePlan{
		ID:     "plan-semantic-covered",
		Status: types.PlanStatusUnverified,
		PatchReview: &types.PatchReviewRecord{
			Findings: []types.PatchReviewFinding{{
				Code:           "changed_symbol_without_probe_coverage",
				Severity:       types.PatchReviewSeverityWarning,
				Category:       types.PatchReviewCategorySemanticCoverage,
				Path:           "pkg/axis.py",
				SubjectSymbol:  "Axis.convert",
				CoverageStatus: types.PatchReviewCoverageVerified,
			}, {
				Code:           "convention_surface_available",
				Severity:       types.PatchReviewSeverityInfo,
				Category:       types.PatchReviewCategoryConvention,
				Path:           "pkg/axis.py",
				CoverageStatus: types.PatchReviewCoverageAdvisory,
			}},
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-semantic-covered",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: "plan-semantic-covered"},
				{Kind: "verify", Status: "unverified", ReasonCode: "parser_error", PlanID: "plan-semantic-covered"},
			},
		}},
	}
	got := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{
		Action:            writeflow.ActionFinish,
		ReasonCode:        "done",
		FinishDisposition: writeflow.FinishDispositionAcceptUnverified,
	}, run)
	if got.Action != writeflow.ActionFinish || got.FinishDisposition != writeflow.FinishDispositionAcceptUnverified {
		t.Fatalf("covered/advisory semantic review should not append follow-up, got %+v", got)
	}
}

func TestSyncMutablePlanStatusAfterVerifyMarksCoverageVerifiedAndRefreshesContext(t *testing.T) {
	mu := types.NewMutableState("coverage projector")
	plan := coverageProjectionPlanForTest()
	scoped := types.WriteContextPackFromChangePlan(plan).WithScope("batch-1", "slice-1")
	mu.SetChangePlan(plan)
	mu.SetWriteContextPack(&scoped)
	mu.SetWriteWorkflowRun(&types.WriteWorkflowRun{
		RunID:         "wf-coverage",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			ActiveSliceID: "slice-1",
		}},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorktreePath: "/tmp/codrax-worktree"}}

	o.syncMutablePlanStatusAfterVerify(&types.ChangeReport{
		PlanID:             "plan-coverage",
		Passed:             true,
		VerificationStatus: types.VerificationStatusPassed,
	}, nil)

	got := mu.ChangePlan()
	if got == nil || got.Status != types.PlanStatusApplied {
		t.Fatalf("plan status = %+v, want applied", got)
	}
	if got.ImpactAnalysis == nil || len(got.ImpactAnalysis.VerificationTargets) == 0 ||
		got.ImpactAnalysis.VerificationTargets[0].CoverageStatus != "verified" {
		t.Fatalf("impact target coverage not verified: %+v", got.ImpactAnalysis)
	}
	if got.PatchReview == nil || len(got.PatchReview.Findings) == 0 ||
		got.PatchReview.Findings[0].CoverageStatus != types.PatchReviewCoverageVerified {
		t.Fatalf("patch review coverage not verified: %+v", got.PatchReview)
	}
	pack := mu.WriteContextPack()
	if pack == nil {
		t.Fatal("expected refreshed mutable context pack")
	}
	view := pack.ViewForScope(types.WriteConsumerVerifier, 40, "batch-1", "slice-1")
	if !writeContextViewHasItem(view, "patch_review_finding", "coverage_status=verified") ||
		!writeContextViewHasItem(view, "impact_verification_target", "coverage_status=verified") {
		t.Fatalf("refreshed context pack missing verified coverage: %+v", view.Items)
	}
	if writeContextViewHasItem(view, "patch_review_finding", "coverage_status=unverified") ||
		writeContextViewHasItem(view, "impact_verification_target", "coverage_status=unverified") {
		t.Fatalf("stale unverified coverage leaked into context pack: %+v", view.Items)
	}
}

func TestSyncMutablePlanStatusAfterVerifyPersistsCoverageSnapshot(t *testing.T) {
	mu := types.NewMutableState("coverage projection persistence")
	plan := coverageProjectionPlanForTest()
	plan.TargetPaths = []string{"pkg/axis.py"}
	plan.Changes = []types.FileChange{{
		Path:      "pkg/axis.py",
		Kind:      "patch",
		Patch:     "@@ -1 +1 @@\n-old\n+new\n",
		Rationale: "test persistence",
	}}
	mu.SetChangePlan(plan)
	workDir := t.TempDir()
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable:      mu,
		WorkDir:      workDir,
		WorktreePath: "/tmp/codrax-worktree",
	}}

	o.syncMutablePlanStatusAfterVerify(&types.ChangeReport{
		PlanID:             "plan-coverage",
		Passed:             true,
		VerificationStatus: types.VerificationStatusPassed,
	}, nil)

	path := filepath.Join(workDir, "plans", "plan-coverage.json")
	loaded, err := types.LoadChangePlanFromFile(path)
	if err != nil {
		t.Fatalf("LoadChangePlanFromFile(%s): %v", path, err)
	}
	if loaded.PatchReview == nil || len(loaded.PatchReview.Findings) == 0 ||
		loaded.PatchReview.Findings[0].CoverageStatus != types.PatchReviewCoverageVerified {
		t.Fatalf("persisted patch-review coverage not verified: %+v", loaded.PatchReview)
	}
	if loaded.ImpactAnalysis == nil || len(loaded.ImpactAnalysis.VerificationTargets) == 0 ||
		loaded.ImpactAnalysis.VerificationTargets[0].CoverageStatus != "verified" {
		t.Fatalf("persisted impact coverage not verified: %+v", loaded.ImpactAnalysis)
	}
}

func TestApplyVerifyCoverageToChangePlanPreservesMissingProbeSymbolGap(t *testing.T) {
	plan := coverageProjectionPlanForTest()
	applyVerifyCoverageToChangePlan(plan, &types.ChangeReport{
		PlanID:             "plan-coverage",
		Passed:             true,
		VerificationStatus: types.VerificationStatusPassed,
		VerificationConfidence: []types.VerificationConfidenceRecord{{
			Source:     "verification_probe",
			Category:   "probe_changed_symbol",
			Status:     "missing",
			Severity:   "warning",
			ReasonCode: "verification_probe_missing_changed_symbol_ref",
		}},
	}, nil)

	if plan.ImpactAnalysis == nil || len(plan.ImpactAnalysis.VerificationTargets) == 0 ||
		plan.ImpactAnalysis.VerificationTargets[0].CoverageStatus != "unverified" {
		t.Fatalf("missing symbol confidence should keep impact target unverified: %+v", plan.ImpactAnalysis)
	}
	if plan.PatchReview == nil || len(plan.PatchReview.Findings) == 0 ||
		plan.PatchReview.Findings[0].CoverageStatus != types.PatchReviewCoverageUnverified {
		t.Fatalf("missing symbol confidence should keep patch review unverified: %+v", plan.PatchReview)
	}
}

func TestApplyVerifyCoverageToChangePlanPreservesActualDiffUnknownCoverage(t *testing.T) {
	plan := &types.ChangePlan{
		ID:     "plan-boundary",
		Status: types.PlanStatusUnverified,
		PatchReview: &types.PatchReviewRecord{
			Findings: []types.PatchReviewFinding{
				{
					Code:           "python_nested_string_key_direct_access_added",
					Severity:       types.PatchReviewSeverityWarning,
					Category:       types.PatchReviewCategorySemanticCoverage,
					Path:           "pkg/widget.py",
					CoverageStatus: types.PatchReviewCoverageUnknown,
					EvidenceRef:    "pkg/widget.py:12",
				},
				{
					Code:           "javascript_nested_string_key_direct_access_added",
					Severity:       types.PatchReviewSeverityWarning,
					Category:       types.PatchReviewCategorySemanticCoverage,
					Path:           "src/widget.js",
					CoverageStatus: types.PatchReviewCoverageUnknown,
					EvidenceRef:    "src/widget.js:8",
				},
			},
		},
		ImpactAnalysis: &types.ImpactAnalysisResult{
			VerificationTargets: []types.ImpactVerificationTarget{{
				Kind:           "effect_followup",
				Path:           "pkg/widget.py",
				Priority:       70,
				Source:         "patch_effect",
				CoverageStatus: "unverified",
				EvidenceRef:    "patch-effect:pkg/widget.py",
			}},
		},
	}

	applyVerifyCoverageToChangePlan(plan, &types.ChangeReport{
		PlanID:             "plan-boundary",
		Passed:             true,
		VerificationStatus: types.VerificationStatusPassed,
		TestResults: []types.TestResult{{
			AssertionID: "tests/test_unrelated.py::test_ok",
			Suite:       "tests/test_unrelated.py",
			Passed:      true,
		}},
	}, nil)

	if plan.ImpactAnalysis == nil || len(plan.ImpactAnalysis.VerificationTargets) != 1 ||
		plan.ImpactAnalysis.VerificationTargets[0].CoverageStatus != "unverified" {
		t.Fatalf("actual-diff effect follow-up should require explicit coverage: %+v", plan.ImpactAnalysis)
	}
	if plan.PatchReview == nil || len(plan.PatchReview.Findings) != 2 {
		t.Fatalf("patch review missing: %+v", plan.PatchReview)
	}
	for _, finding := range plan.PatchReview.Findings {
		if finding.CoverageStatus != types.PatchReviewCoverageUnknown {
			t.Fatalf("actual-diff unknown coverage event %s should stay unknown after unrelated pass: %+v", finding.Code, finding)
		}
	}
	summary := types.SummarizePatchReviewCoverage(*plan.PatchReview)
	if summary.Verdict != types.PatchReviewCoverageVerdictUnverified || !summary.HasUncoveredSemantic {
		t.Fatalf("coverage summary should remain uncovered: %+v", summary)
	}
}

func TestApplyVerifyCoverageToChangePlanVerifiesScopedTestSurfaceCoverage(t *testing.T) {
	plan := &types.ChangePlan{
		ID:     "plan-test-surface",
		Status: types.PlanStatusUnverified,
		PatchReview: &types.PatchReviewRecord{
			Findings: []types.PatchReviewFinding{{
				Code:           "related_test_surface_unverified",
				Severity:       types.PatchReviewSeverityWarning,
				Category:       types.PatchReviewCategorySemanticCoverage,
				Path:           "pkg/widget.py",
				RelatedPath:    "tests/test_widget.py",
				CoverageStatus: types.PatchReviewCoverageUnverified,
				EvidenceRef:    "tests/test_widget.py",
			}},
		},
		ImpactAnalysis: &types.ImpactAnalysisResult{
			VerificationTargets: []types.ImpactVerificationTarget{{
				Kind:           "test_surface",
				Path:           "pkg/widget.py",
				RelatedPath:    "tests/test_widget.py",
				Priority:       50,
				Source:         "impact_engine",
				CoverageStatus: "unverified",
				EvidenceRef:    "tests/test_widget.py",
			}},
		},
	}

	applyVerifyCoverageToChangePlan(plan, &types.ChangeReport{
		PlanID:             "plan-test-surface",
		Passed:             true,
		VerificationStatus: types.VerificationStatusPassed,
		TestResults: []types.TestResult{{
			AssertionID: "tests/test_widget.py::test_label",
			Suite:       "tests/test_widget.py",
			Passed:      true,
		}},
	}, nil)

	if plan.ImpactAnalysis == nil || len(plan.ImpactAnalysis.VerificationTargets) != 1 ||
		plan.ImpactAnalysis.VerificationTargets[0].CoverageStatus != "verified" {
		t.Fatalf("scoped passing suite should verify matching impact target: %+v", plan.ImpactAnalysis)
	}
	if plan.PatchReview == nil || len(plan.PatchReview.Findings) != 1 ||
		plan.PatchReview.Findings[0].CoverageStatus != types.PatchReviewCoverageVerified {
		t.Fatalf("scoped passing suite should verify matching patch-review finding: %+v", plan.PatchReview)
	}
}

func TestNormalizeControllerTypedStateDecisionSemanticPatchReviewDoesNotRecurse(t *testing.T) {
	mu := types.NewMutableState("semantic patch review followup recursion")
	mu.SetChangePlan(&types.ChangePlan{
		ID:     "plan-semantic-followup",
		Status: types.PlanStatusUnverified,
		PatchReview: &types.PatchReviewRecord{
			Findings: []types.PatchReviewFinding{{
				Code:           "related_test_surface_unverified",
				Severity:       types.PatchReviewSeverityWarning,
				Category:       types.PatchReviewCategorySemanticCoverage,
				Path:           "pkg/axis.py",
				RelatedPath:    "tests/test_axis.py",
				CoverageStatus: types.PatchReviewCoverageUnverified,
			}},
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-semantic-no-recursion",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1-semantic-review",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1-semantic-review",
			Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: "plan-semantic-followup"},
				{Kind: "verify", Status: "unverified", ReasonCode: "parser_error", PlanID: "plan-semantic-followup"},
			},
		}},
		ProgressLedger: []types.WriteWorkflowProgress{{
			BatchID:    "batch-1",
			ReasonCode: "impact_obligation_followup_requested",
		}},
	}
	got := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{
		Action:            writeflow.ActionFinish,
		ReasonCode:        "done",
		FinishDisposition: writeflow.FinishDispositionAcceptUnverified,
	}, run)
	if got.Action != writeflow.ActionFinish || got.FinishDisposition != writeflow.FinishDispositionAcceptUnverified {
		t.Fatalf("semantic follow-up should not recursively append another follow-up, got %+v", got)
	}
}

func TestNormalizeControllerTypedStateDecisionReplanWithFailureHandoffAllowed(t *testing.T) {
	mu := types.NewMutableState("failed verify replan allowed")
	mu.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{BatchID: "batch-1", Attempt: 1})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-failed",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: "plan-1"},
				{Kind: "verify", Status: "failed", ReasonCode: "tests_failed", PlanID: "plan-1"},
			},
		}},
	}
	decision := writeflow.WriteWorkflowDecision{
		Action: writeflow.ActionReplanBatch,
		Batch:  &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "fix failed verify"},
	}
	got := o.normalizeControllerTypedStateDecision(decision, run)
	if got.Action != writeflow.ActionReplanBatch {
		t.Fatalf("typed failure handoff must allow replan, got %+v", got)
	}
}

func TestNormalizeControllerTypedStateDecisionNeedsReplanRejectsStalePlanActions(t *testing.T) {
	for _, action := range []writeflow.WorkflowAction{
		writeflow.ActionApplyPlan,
		writeflow.ActionVerifyBatch,
		writeflow.ActionPlanBatch,
	} {
		t.Run(string(action), func(t *testing.T) {
			mu := types.NewMutableState("failed verify stale action")
			o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
			run := &types.WriteWorkflowRun{
				RunID:         "wf-needs-replan",
				Status:        types.WriteWorkflowRunInProgress,
				ActiveBatchID: "batch-1",
				Batches: []types.WriteWorkflowBatch{{
					ID:     "batch-1",
					Goal:   "repair failing behavior",
					Status: types.WriteWorkflowBatchReadyToPlan,
					PlanID: "plan-1",
					Attempts: []types.WriteWorkflowAttempt{
						{Kind: "plan", Status: "planned", PlanID: "plan-1"},
						{Kind: "apply", Status: "applied", PlanID: "plan-1"},
						{Kind: "verify", Status: "failed", ReasonCode: "tests_failed", PlanID: "plan-1"},
					},
				}},
			}
			decision := writeflow.WriteWorkflowDecision{
				Action: action,
				Batch:  &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "repair failing behavior"},
			}
			got := o.normalizeControllerTypedStateDecision(decision, run)
			if got.Action != writeflow.ActionReplanBatch {
				t.Fatalf("needs_replan must normalize %s to replan_batch, got %+v", action, got)
			}
			if got.Batch == nil || got.Batch.ID != "batch-1" || got.Batch.Goal != "repair failing behavior" {
				t.Fatalf("replan decision should preserve active batch identity, got %+v", got.Batch)
			}
			if !workflowProgressHasReason(run.ProgressLedger, "needs_replan_stale_action_overridden") {
				t.Fatalf("override progress missing: %+v", run.ProgressLedger)
			}
		})
	}
}

func TestNormalizeControllerTypedStateDecisionPlanBatchNeedsExplorationRoutesExplore(t *testing.T) {
	mu := types.NewMutableState("needs exploration before plan")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-needs-explore",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
	}
	decision := writeflow.WriteWorkflowDecision{
		Action: writeflow.ActionPlanBatch,
		Batch:  &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "repair exception rendering"},
		Workflow: writeflow.WriteWorkflowPlan{
			PlannedBatches: []writeflow.WriteBatchPlan{{
				ID:                   "batch-1",
				Goal:                 "repair exception rendering",
				NeedsCodeExploration: true,
				ExpectedPaths:        []string{"src/_pytest/_code/code.py"},
			}},
		},
	}
	got := o.normalizeControllerTypedStateDecision(decision, run)
	if got.Action != writeflow.ActionExploreCode {
		t.Fatalf("needs_code_exploration plan_batch should route to explore_code first, got %+v", got)
	}
	if got.ExplorationRequest == nil {
		t.Fatalf("explore_code decision missing exploration_request: %+v", got)
	}
	if got.ExplorationRequest.BatchID != "batch-1" {
		t.Fatalf("BatchID = %q, want batch-1", got.ExplorationRequest.BatchID)
	}
	if !containsString(got.ExplorationRequest.CandidatePaths, "src/_pytest/_code/code.py") {
		t.Fatalf("candidate paths should preserve typed expected paths, got %+v", got.ExplorationRequest.CandidatePaths)
	}
	if !workflowProgressHasReason(run.ProgressLedger, "plan_batch_needs_exploration_overridden") {
		t.Fatalf("override progress missing: %+v", run.ProgressLedger)
	}
}

func TestNormalizeControllerTypedStateDecisionPlanBatchAfterExplorationAttemptAllowed(t *testing.T) {
	mu := types.NewMutableState("needs exploration already attempted")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-needs-explore",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID: "batch-1",
			Attempts: []types.WriteWorkflowAttempt{{
				Kind:   "explore",
				Status: "degraded",
			}},
		}},
	}
	decision := writeflow.WriteWorkflowDecision{
		Action: writeflow.ActionPlanBatch,
		Batch: &writeflow.WriteBatchPlan{
			ID:                   "batch-1",
			Goal:                 "repair exception rendering",
			NeedsCodeExploration: true,
			ExpectedPaths:        []string{"src/_pytest/_code/code.py"},
		},
	}
	got := o.normalizeControllerTypedStateDecision(decision, run)
	if got.Action != writeflow.ActionPlanBatch {
		t.Fatalf("existing exploration attempt should allow planning, got %+v", got)
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
			Status: types.WriteWorkflowBatchReadyToPlan,
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

func TestRunWriteControllerWorkflow_ResumePendingApprovalPausesBeforeController(t *testing.T) {
	store := &fakeWorkflowRunStore{active: &types.WriteWorkflowRun{
		RunID:         "wf-approval",
		Goal:          "resume approval",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-approval",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-approval",
			Goal:   "existing approval batch",
			Status: types.WriteWorkflowBatchPendingApproval,
			PlanID: "plan-needs-human",
		}},
	}}
	mu := types.NewMutableState("new request must not bypass approval")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "new seed"}}})
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
			controllerCalls++
			t.Fatalf("pending approval resume must pause before controller dispatch")
			return &agent.StageOutput{}, nil
		},
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	steps := 0
	err := o.runWriteControllerWorkflow(&steps)
	if err == nil || !strings.Contains(err.Error(), "write workflow pending approval for plan plan-needs-human") {
		t.Fatalf("pending approval resume should fail-loud before controller, got %v", err)
	}
	if controllerCalls != 0 {
		t.Fatalf("pending approval resume must not call controller, calls=%d", controllerCalls)
	}
	if store.last == nil || store.last.RunID != "wf-approval" {
		t.Fatalf("workflow should remain active and persisted, got %+v", store.last)
	}
	if store.last.Status != types.WriteWorkflowRunInProgress {
		t.Fatalf("pending approval resume must not terminalize run: %+v", store.last)
	}
	if len(store.last.Batches) != 1 || store.last.Batches[0].Status != types.WriteWorkflowBatchPendingApproval {
		t.Fatalf("pending approval batch should stay paused: %+v", store.last.Batches)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "workflow_resumed") ||
		!workflowProgressHasReason(store.last.ProgressLedger, "pending_approval_resume_paused") {
		t.Fatalf("resume pause progress missing: %+v", store.last.ProgressLedger)
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

type scriptedControllerStep struct {
	decision writeflow.WriteWorkflowDecision
	err      error
}

func scriptedControllerWithErrors(t *testing.T, steps []scriptedControllerStep, calls *int) func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
	t.Helper()
	return func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
		if *calls >= len(steps) {
			t.Fatalf("unexpected extra controller call %d", *calls+1)
		}
		step := steps[*calls]
		(*calls)++
		if step.err != nil {
			return nil, step.err
		}
		decision := writeflow.NormalizeWriteWorkflowDecision(step.decision)
		raw, err := json.Marshal(decision)
		if err != nil {
			t.Fatalf("marshal decision: %v", err)
		}
		ctx.Mutable.SetWriteWorkflowDecisionJSON(raw)
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

func latestWorkflowBatchAttempt(batch types.WriteWorkflowBatch, kind string) *types.WriteWorkflowAttempt {
	for i := len(batch.Attempts) - 1; i >= 0; i-- {
		if batch.Attempts[i].Kind == kind {
			return &batch.Attempts[i]
		}
	}
	return nil
}

func workflowBatchHasAttemptArtifact(batch types.WriteWorkflowBatch, kind, status, reasonCode, artifactRef string) bool {
	for _, attempt := range batch.Attempts {
		if attempt.Kind == kind && attempt.Status == status && attempt.ReasonCode == reasonCode && attempt.ArtifactRef == artifactRef {
			return true
		}
	}
	return false
}

func workflowBatchHasAttemptSurface(batch types.WriteWorkflowBatch, kind, status, reasonCode, surfaceRef string) bool {
	for _, attempt := range batch.Attempts {
		if attempt.Kind == kind && attempt.Status == status && attempt.ReasonCode == reasonCode && attempt.SurfaceRef == surfaceRef {
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

func writeContextViewHasItem(view types.WriteContextView, kind, substring string) bool {
	for _, item := range view.Items {
		if item.Kind == kind && strings.Contains(item.Text, substring) {
			return true
		}
	}
	return false
}

func coverageProjectionPlanForTest() *types.ChangePlan {
	return &types.ChangePlan{
		ID:      "plan-coverage",
		Summary: "repair covered symbol",
		Status:  types.PlanStatusUnverified,
		PatchReview: &types.PatchReviewRecord{
			ReviewID: "patch-review:plan-coverage:slice-1",
			Findings: []types.PatchReviewFinding{{
				Code:           "changed_symbol_without_probe_coverage",
				Severity:       types.PatchReviewSeverityWarning,
				Category:       types.PatchReviewCategorySemanticCoverage,
				Path:           "pkg/axis.py",
				SubjectSymbol:  "Axis.convert",
				CoverageStatus: types.PatchReviewCoverageUnverified,
				EvidenceRef:    "pkg/axis.py:12",
			}},
		},
		ImpactAnalysis: &types.ImpactAnalysisResult{
			ResultID: "impact-analysis:plan-coverage:patch-effect",
			PlanID:   "plan-coverage",
			VerificationTargets: []types.ImpactVerificationTarget{{
				Kind:           "changed_symbol",
				Path:           "pkg/axis.py",
				Symbol:         "Axis.convert",
				Priority:       20,
				Source:         "impact_engine",
				CoverageStatus: "unverified",
				EvidenceRef:    "pkg/axis.py:12",
			}},
		},
	}
}

func patchReviewRecordHasFinding(review types.PatchReviewRecord, code string) bool {
	for _, finding := range review.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func workflowProgressContains(progress []types.WriteWorkflowProgress, reasonCode string) bool {
	for _, item := range progress {
		if item.ReasonCode == reasonCode {
			return true
		}
	}
	return false
}

func TestAttachPlanContextPackToWorkflowRunPersistsPriorContextCoverage(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "run-1",
		ActiveBatchID: "batch-1",
		ContextPacks: []types.WriteContextPack{{
			PackID:      "exploration-handoff",
			BatchID:     "batch-1",
			SourceStage: "explore",
			Items: []types.WriteContextItem{{
				Priority:    types.WriteContextP1,
				Kind:        "target_file",
				Text:        "bug.py",
				SourceStage: "explore",
				Consumers:   []types.WriteContextConsumer{types.WriteConsumerPlanner},
			}, {
				Priority:    types.WriteContextP1,
				Kind:        "target_file",
				Text:        "helper.py",
				SourceStage: "explore",
				Consumers:   []types.WriteContextConsumer{types.WriteConsumerPlanner},
			}},
		}, {
			PackID:      "change-plan",
			BatchID:     "batch-1",
			SourceStage: "plan",
			Items: []types.WriteContextItem{{
				Priority:    types.WriteContextP1,
				Kind:        "target_file",
				Text:        "self.py",
				SourceStage: "plan",
				Consumers:   []types.WriteContextConsumer{types.WriteConsumerPlanner},
			}},
		}},
	}
	plan := &types.ChangePlan{
		ID:          "plan-1",
		Summary:     "repair bug",
		TargetPaths: []string{"bug.py"},
		Changes:     []types.FileChange{{Path: "bug.py", Kind: "modify"}},
	}

	got := attachPlanContextPackToWorkflowRun(run, plan)
	if !workflowRunContextContains(&got, "plan_context_coverage", "covered=1/2") ||
		!workflowRunContextContains(&got, "plan_context_uncovered_path", "helper.py") {
		t.Fatalf("plan context coverage should persist with workflow run: %+v", got.ContextPacks)
	}
	if !workflowRunContextContains(&got, "target_file", "bug.py") {
		t.Fatalf("change-plan context should remain present: %+v", got.ContextPacks)
	}
	for _, pack := range got.ContextPacks {
		if pack.BatchID == "batch-1" && pack.SourceStage == "plan" && pack.PackID != "change-plan" {
			t.Fatalf("plan context pack identity should remain stable, got %+v", pack)
		}
	}
}

func TestAttachPlanContextPackToWorkflowRunPersistsMissingPriorContext(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "run-1",
		ActiveBatchID: "batch-1",
	}
	plan := &types.ChangePlan{
		ID:          "plan-1",
		Summary:     "repair bug",
		TargetPaths: []string{"pkg/a.py"},
		Changes:     []types.FileChange{{Path: "pkg/a.py", Kind: "modify"}},
	}

	got := attachPlanContextPackToWorkflowRun(run, plan)
	if !workflowRunContextContains(&got, "plan_context_coverage", "covered=0/0") ||
		!workflowRunContextContains(&got, "plan_context_missing_source_path", "pkg/a.py") {
		t.Fatalf("missing prior context coverage should persist with workflow run: %+v", got.ContextPacks)
	}
	if !workflowRunContextContains(&got, "target_file", "pkg/a.py") {
		t.Fatalf("change-plan context should remain present: %+v", got.ContextPacks)
	}
}

func TestSyncCurrentWriteContextPackToRunKeepsStaleBatchEvidenceOutOfActiveBatch(t *testing.T) {
	mu := types.NewMutableState("sync scoped context")
	mu.SetWriteContextPack(&types.WriteContextPack{
		PackID:      "merged",
		BatchID:     "batch-1",
		SourceStage: "verify",
		Items: []types.WriteContextItem{{
			Priority:    types.WriteContextP0,
			Kind:        "constraint",
			Text:        "preserve public API",
			SourceStage: "write_analysis",
			Consumers:   []types.WriteContextConsumer{types.WriteConsumerPlanner},
		}, {
			Priority:    types.WriteContextP2,
			Kind:        "verify_failure",
			Text:        "old batch failed",
			SourceStage: "verify",
			Consumers:   []types.WriteContextConsumer{types.WriteConsumerPlanner},
		}, {
			BatchID:     "batch-2",
			Priority:    types.WriteContextP1,
			Kind:        "target_file",
			Text:        "pkg/new.py",
			SourceStage: "explore",
			Consumers:   []types.WriteContextConsumer{types.WriteConsumerPlanner},
		}},
	})
	run := types.WriteWorkflowRun{
		RunID:         "run-1",
		ActiveBatchID: "batch-2",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
		}, {
			ID:     "batch-2",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	}
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}

	o.syncCurrentWriteContextPackToRun(&run)
	merged := types.MergeWriteContextPacks("batch-2", "sync scoped context", run.ContextPacks...)
	view := merged.ViewForScope(types.WriteConsumerPlanner, 20, "batch-2", "")
	if !writeContextViewContains(view, "constraint", "preserve public API") {
		t.Fatalf("global P0 context should persist into active batch: %+v", view.Items)
	}
	if !writeContextViewContains(view, "target_file", "pkg/new.py") {
		t.Fatalf("active batch context missing after scoped sync: %+v", view.Items)
	}
	if writeContextViewContains(view, "verify_failure", "old batch failed") {
		t.Fatalf("stale batch failure should not be persisted as active context: %+v", view.Items)
	}
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
		// …and accept_unverified is still rejected for typed code-failure evidence.
		{Action: writeflow.ActionFinish, ReasonCode: "accepted", FinishDisposition: writeflow.FinishDispositionAcceptUnverified},
		{Action: writeflow.ActionBlock, ReasonCode: "post_apply_failed"},
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
	if err := o.runWriteControllerWorkflow(&steps); err == nil {
		t.Fatal("workflow should block after failed verification finish attempts are rejected")
	}
	if controllerCalls != 6 {
		t.Fatalf("controller calls = %d, want 6 (two finish rejections, then block)", controllerCalls)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "finish_rejected_failed_verify") {
		t.Fatalf("typed finish rejection should be recorded: %+v", store.last.ProgressLedger)
	}
	if store.last.Status != types.WriteWorkflowRunBlocked {
		t.Fatalf("code-failure verification should block when controller stops, got %+v", store.last.Status)
	}
	result := mu.Result()
	if !strings.Contains(result, "post_apply_failed") {
		t.Fatalf("result should carry the typed block reason, got %q", result)
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

func TestRunWriteControllerWorkflow_PlannerProbePassCannotFinishFailedPostApplyVerify(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("probe pass but post-apply fails")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "probe pass but post-apply fails"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "single change"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "applied"},
		{Action: writeflow.ActionFinish, ReasonCode: "probe_said_green"},
		{Action: writeflow.ActionBlock, ReasonCode: "post_apply_failed"},
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
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-probe-pass", Status: types.PlanStatusPending, Summary: "attempt", TargetPaths: []string{"fix.go"}})
			mu.AppendPlanStageProbeReport(&types.ChangeReport{
				PlanID:  "plan-probe-pass",
				Channel: types.ChangeReportChannelPlannerProbe,
				Passed:  true,
			})
		case types.StageVerify:
			mu.SetChangeReport(&types.ChangeReport{
				PlanID:         "plan-probe-pass",
				Channel:        types.ChangeReportChannelPostApplyVerify,
				Passed:         false,
				FailureSummary: "post-apply test failed",
			})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err == nil {
		t.Fatal("workflow should block after bare finish is rejected and controller blocks")
	}
	if store.last == nil || store.last.Status != types.WriteWorkflowRunBlocked {
		t.Fatalf("workflow should be blocked by the final controller decision, got %+v", store.last)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "finish_rejected_failed_verify") {
		t.Fatalf("planner probe pass must not unblock failed post-apply finish: %+v", store.last.ProgressLedger)
	}
	if !workflowBatchHasAttempt(store.last.Batches[0], "verify", "failed", "tests_failed", "plan-probe-pass.report.json") {
		t.Fatalf("post-apply failure should be the authoritative verify attempt: %+v", store.last.Batches[0].Attempts)
	}
}

func TestRunWriteControllerWorkflow_PlannerProbeFailureDoesNotBlockPassedPostApplyVerify(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("probe fails but post-apply passes")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "probe fails but post-apply passes"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "single change"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "applied"},
		{Action: writeflow.ActionFinish, ReasonCode: "post_apply_green"},
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
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-probe-fail", Status: types.PlanStatusPending, Summary: "attempt", TargetPaths: []string{"fix.go"}})
			mu.AppendPlanStageProbeReport(&types.ChangeReport{
				PlanID:         "plan-probe-fail",
				Channel:        types.ChangeReportChannelPlannerProbe,
				Passed:         false,
				FailureSummary: "dry-run failure before apply",
			})
		case types.StageVerify:
			mu.SetChangeReport(&types.ChangeReport{
				PlanID:  "plan-probe-fail",
				Channel: types.ChangeReportChannelPostApplyVerify,
				Passed:  true,
			})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("post-apply pass should finish despite stale probe failure: %v", err)
	}
	if store.last == nil || store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("workflow should complete from post-apply green report, got %+v", store.last)
	}
	if workflowProgressHasReason(store.last.ProgressLedger, "verify_failed") ||
		workflowProgressHasReason(store.last.ProgressLedger, "finish_rejected_failed_verify") {
		t.Fatalf("planner probe failure must not pollute post-apply state: %+v", store.last.ProgressLedger)
	}
	if !workflowBatchHasAttempt(store.last.Batches[0], "verify", "passed", "tests_passed", "plan-probe-fail.report.json") {
		t.Fatalf("post-apply success should be the authoritative verify attempt: %+v", store.last.Batches[0].Attempts)
	}
}

func TestRunWriteControllerWorkflow_RejectsNonAuthoritativeVerifyReports(t *testing.T) {
	cases := []struct {
		name       string
		report     types.ChangeReport
		wantDetail string
	}{
		{
			name: "planner probe channel",
			report: types.ChangeReport{
				PlanID:  "plan-channel",
				Channel: types.ChangeReportChannelPlannerProbe,
				Passed:  true,
			},
			wantDetail: "non-post-apply ChangeReport channel",
		},
		{
			name: "stale plan id",
			report: types.ChangeReport{
				PlanID:  "older-plan",
				Channel: types.ChangeReportChannelPostApplyVerify,
				Passed:  true,
			},
			wantDetail: "stale plan",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeWorkflowRunStore{}
			mu := types.NewMutableState(tc.name)
			mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: tc.name}}})
			decisions := []writeflow.WriteWorkflowDecision{
				{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "single change"}},
				{Action: writeflow.ActionApplyPlan, ReasonCode: "ready"},
				{Action: writeflow.ActionVerifyBatch, ReasonCode: "applied"},
				{Action: writeflow.ActionBlock, ReasonCode: "invalid_report"},
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
					mu.SetChangePlan(&types.ChangePlan{ID: "plan-channel", Status: types.PlanStatusPending, Summary: "attempt", TargetPaths: []string{"fix.go"}})
				case types.StageVerify:
					report := tc.report
					mu.SetChangeReport(&report)
				}
				*stepsUsed++
				return &agent.StageOutput{}, nil
			}
			steps := 0
			if err := o.runWriteControllerWorkflow(&steps); err == nil {
				t.Fatal("workflow should block after invalid verify report")
			}
			if store.last == nil || store.last.Status != types.WriteWorkflowRunBlocked {
				t.Fatalf("workflow should be blocked after invalid report, got %+v", store.last)
			}
			if !workflowProgressHasMessage(store.last.ProgressLedger, tc.wantDetail) {
				t.Fatalf("invalid report detail %q should be persisted in progress: %+v", tc.wantDetail, store.last.ProgressLedger)
			}
			if !workflowBatchHasAttempt(store.last.Batches[0], "verify", "failed", "tests_failed", "plan-channel.report.json") {
				t.Fatalf("invalid report should become a failed verify attempt for active plan: %+v", store.last.Batches[0].Attempts)
			}
		})
	}
}

func TestRunWriteControllerWorkflow_VerifyFailureSetsHandoffAndGreenClears(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	planDir := t.TempDir()
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
	o.reportDir = planDir
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
					TestSurface: &types.TestSurface{Candidates: []types.TestSurfaceCandidate{{
						ID:            "go@.",
						Runner:        "go",
						WorkingDir:    ".",
						HasTestSignal: true,
					}}},
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
	if handoffSeenAtReplan.SurfaceArtifactRef != "plan-1.attempt-1.surface.json" {
		t.Fatalf("handoff should carry the persisted test surface ref, got %q", handoffSeenAtReplan.SurfaceArtifactRef)
	}
	if handoffSeenAtReplan.SurfaceArtifactPath != filepath.Join(planDir, "plan-1.attempt-1.surface.json") {
		t.Fatalf("handoff should carry resolved test surface path, got %q", handoffSeenAtReplan.SurfaceArtifactPath)
	}
	if _, err := types.LoadTestSurfaceFromFile(filepath.Join(planDir, handoffSeenAtReplan.SurfaceArtifactRef)); err != nil {
		t.Fatalf("persisted test surface should be readable: %v", err)
	}
	if store.last == nil || len(store.last.Batches) == 0 ||
		!workflowBatchHasAttemptSurface(store.last.Batches[0], "verify", "failed", "tests_failed", "plan-1.attempt-1.surface.json") {
		t.Fatalf("failed verify attempt should reference persisted surface: %+v", store.last)
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

func TestRunControllerPlanBatch_NoPlanDoesNotSpendRetryAfterCancel(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("no plan retry canceled")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "no plan retry"}}})
	mu.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{PlanID: "plan-prev", BatchID: "batch-1", Attempt: 1})
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	planCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		if stage != types.StagePlan {
			t.Fatalf("unexpected stage %s", stage)
		}
		planCalls++
		*stepsUsed++
		o.Cancel("write mode wall-time exceeded (600s)")
		return &agent.StageOutput{Error: "no change plan was produced this round"}, nil
	}
	steps := 0
	err := o.runControllerPlanBatch(&writeflow.WriteBatchPlan{ID: "batch-1", Goal: "repair"}, &steps)
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("canceled no-plan round should return ErrCanceled, got %v", err)
	}
	if planCalls != 1 {
		t.Fatalf("plan stage calls = %d, want 1 (no retry after cancel)", planCalls)
	}
	if strings.Contains(mu.PlanningHint(), "previous planning round ended without a stored change plan") {
		t.Fatalf("canceled no-plan round must not spend retry hint budget, got %q", mu.PlanningHint())
	}
	if !strings.Contains(o.busCtx.TaskState.LastError, "write mode wall-time exceeded") {
		t.Fatalf("last error should preserve cancel reason, got %q", o.busCtx.TaskState.LastError)
	}
}

func TestRunControllerPlanBatch_NoChangeSentinelRestoresAppliedPlanForVerify(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("no-change replan")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "repair already applied"}}})
	prior := &types.ChangePlan{
		ID:               "plan-prior",
		Status:           types.PlanStatusAppliedPendingVerify,
		Summary:          "prior applied repair",
		TargetPaths:      []string{"src/flask/cli.py"},
		AppliedCommitSHA: "abc123",
		Changes: []types.FileChange{{
			Path: "src/flask/cli.py",
			Kind: "patch",
			Apply: &types.FileChangeApplyRecord{
				Status: "applied",
			},
		}},
	}
	mu.SetChangePlan(prior)
	mu.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{PlanID: "plan-prior", BatchID: "batch-1", Attempt: 1})
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		if stage != types.StagePlan {
			t.Fatalf("unexpected stage %s", stage)
		}
		mu.AppendPlanStageProbeReport(&types.ChangeReport{
			PlanID:  "plan-prior",
			Channel: types.ChangeReportChannelPlannerProbe,
			Passed:  true,
			TestResults: []types.TestResult{{
				AssertionID: "tests/test_cli.py::TestRoutes::test_route_registration",
				Kind:        types.TestResultKindUnit,
				Passed:      true,
			}},
		})
		mu.SetChangePlan(&types.ChangePlan{
			ID:        "noop-plan-prior",
			Status:    types.PlanStatusNoChangeRequired,
			Summary:   "scoped probe passed against existing worktree",
			CreatedAt: prior.CreatedAt,
		})
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	err := o.runControllerPlanBatch(&writeflow.WriteBatchPlan{ID: "batch-1", Goal: "repair"}, &steps)
	if !errors.Is(err, errPlannerProbePassedExistingWorktree) {
		t.Fatalf("runControllerPlanBatch error = %v, want errPlannerProbePassedExistingWorktree", err)
	}
	got := mu.ChangePlan()
	if got == nil || got.ID != "plan-prior" {
		t.Fatalf("controller should restore prior applied plan, got %+v", got)
	}
}

func TestRunControllerPlanBatch_NoPlanWithTypedAnchorsGetsOneRetry(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("no plan retry with typed anchors")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{
		Task:         types.WriteTask{Summary: "fix exception rendering"},
		ScopeAnchors: []string{"src/_pytest/_code/code.py"},
	}})
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	planCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		if stage != types.StagePlan {
			t.Fatalf("unexpected stage %s", stage)
		}
		planCalls++
		*stepsUsed++
		if planCalls == 1 {
			return &agent.StageOutput{Error: "no change plan was produced this round"}, nil
		}
		hint := mu.PlanningHint()
		if !strings.Contains(hint, "typed planning context") {
			t.Fatalf("retry hint must cite typed planning context, got %q", hint)
		}
		if !strings.Contains(hint, "batch expected paths") || !strings.Contains(hint, "WriteAnalysisIR scope anchors") {
			t.Fatalf("retry hint must preserve typed anchor boundary, got %q", hint)
		}
		mu.SetChangePlan(&types.ChangePlan{
			ID:          "plan-anchor-retry",
			Status:      types.PlanStatusPending,
			Summary:     "repair exception rendering",
			TargetPaths: []string{"src/_pytest/_code/code.py"},
		})
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runControllerPlanBatch(&writeflow.WriteBatchPlan{
		ID:            "batch-1",
		Goal:          "repair exception rendering",
		ExpectedPaths: []string{"src/_pytest/_code/code.py"},
	}, &steps); err != nil {
		t.Fatalf("one empty round with typed anchors must be retried, got %v", err)
	}
	if planCalls != 2 {
		t.Fatalf("plan stage calls = %d, want 2", planCalls)
	}
	if mu.ChangePlan() == nil {
		t.Fatal("retry round should have installed the plan")
	}
}

func TestRunControllerPlanBatch_NoPlanAfterExplorationHandoffGetsOneRetry(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("no plan retry after exploration handoff")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "symptom-driven repair"}}})
	mu.SetWriteExplorationHandoff(&types.WriteExplorationHandoff{
		BatchID:     "batch-1",
		Goal:        "locate boundary logic from symptom",
		TargetFiles: []string{"src/duration.rs"},
		EvidenceRefs: []types.WriteExplorationEvidenceRef{{
			ID:        "evidence-1",
			Kind:      "source",
			Source:    "src/duration.rs",
			LineStart: 12,
			LineEnd:   18,
			Summary:   "constructor lower-bound branch handles milliseconds",
		}},
		Confidence: "medium",
	})
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	planCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		if stage != types.StagePlan {
			t.Fatalf("unexpected stage %s", stage)
		}
		planCalls++
		*stepsUsed++
		if planCalls == 1 {
			return &agent.StageOutput{Error: "no change plan was produced this round"}, nil
		}
		hint := mu.PlanningHint()
		if !strings.Contains(hint, "previous planning round ended without a stored change plan") {
			t.Fatalf("retry hint must cite the empty round, got %q", hint)
		}
		if !strings.Contains(hint, "exploration findings") {
			t.Fatalf("retry hint must preserve typed exploration handoff, got %q", hint)
		}
		handoff := mu.WriteExplorationHandoff()
		if handoff == nil || len(handoff.EvidenceRefs) != 1 || handoff.TargetFiles[0] != "src/duration.rs" {
			t.Fatalf("exploration handoff should survive planning reset: %+v", handoff)
		}
		mu.SetChangePlan(&types.ChangePlan{
			ID:          "plan-exploration-retry",
			Status:      types.PlanStatusPending,
			Summary:     "repair localized boundary branch",
			TargetPaths: []string{"src/duration.rs"},
			Changes: []types.FileChange{{
				Path:       "src/duration.rs",
				Kind:       "modify",
				NewContent: "pub fn fixed() {}\n",
			}},
		})
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runControllerPlanBatch(&writeflow.WriteBatchPlan{ID: "batch-1", Goal: "repair boundary bug"}, &steps); err != nil {
		t.Fatalf("one empty round with exploration handoff active must be retried, got %v", err)
	}
	if planCalls != 2 {
		t.Fatalf("plan stage calls = %d, want 2 (one empty + one retry)", planCalls)
	}
	if mu.ChangePlan() == nil {
		t.Fatal("retry round should have installed the plan")
	}
}

func TestRunControllerPlanBatch_ReadOnlyProgressGetsSecondRetry(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("no plan retry while planner is still reading exact bytes")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "symptom-driven repair"}}})
	mu.SetWriteExplorationHandoff(&types.WriteExplorationHandoff{
		BatchID:     "batch-1",
		Goal:        "repair localized parser behavior",
		TargetFiles: []string{"src/parser.py", "tests/test_parser.py"},
		EvidenceRefs: []types.WriteExplorationEvidenceRef{{
			ID:        "evidence-parser",
			Kind:      "source",
			Source:    "src/parser.py",
			LineStart: 41,
			LineEnd:   73,
			Summary:   "typed exploration points at option normalization",
		}},
		Confidence: "medium",
	})
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	planCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		if stage != types.StagePlan {
			t.Fatalf("unexpected stage %s", stage)
		}
		planCalls++
		*stepsUsed++
		mu.ResetDispatchToolResults()
		switch planCalls {
		case 1:
			mu.AppendDispatchToolResult(types.ToolResult{
				ToolName: "read_file",
				Success:  true,
				Summary:  "[src/parser.py: showing lines 40-80 of 160 total]\ncode",
			})
			return &agent.StageOutput{Error: "no change plan was produced this round"}, nil
		case 2:
			hint := mu.PlanningHint()
			if !strings.Contains(hint, "previous planning round ended without a stored change plan") {
				t.Fatalf("first retry hint must cite the empty round, got %q", hint)
			}
			mu.AppendDispatchToolResult(types.ToolResult{
				ToolName: "grep",
				Success:  true,
				Summary:  "tests/test_parser.py:12: assert parse_option",
			})
			return &agent.StageOutput{Error: "no change plan was produced this round"}, nil
		default:
			hint := mu.PlanningHint()
			if !strings.Contains(hint, "exploration findings") {
				t.Fatalf("second retry hint should still preserve exploration handoff, got %q", hint)
			}
			mu.SetChangePlan(&types.ChangePlan{
				ID:          "plan-read-progress-retry",
				Status:      types.PlanStatusPending,
				Summary:     "repair localized parser behavior",
				TargetPaths: []string{"src/parser.py"},
				Changes: []types.FileChange{{
					Path: "src/parser.py",
					Kind: "patch",
					Edits: []types.StructuredEdit{{
						Kind:    "replace",
						OldText: "old",
						Content: "new",
					}},
				}},
			})
			return &agent.StageOutput{}, nil
		}
	}
	steps := 0
	if err := o.runControllerPlanBatch(&writeflow.WriteBatchPlan{ID: "batch-1", Goal: "repair parser behavior"}, &steps); err != nil {
		t.Fatalf("read-only planning progress should receive a bounded second retry, got %v", err)
	}
	if planCalls != 3 {
		t.Fatalf("plan stage calls = %d, want 3 (initial + two bounded retries)", planCalls)
	}
	if mu.ChangePlan() == nil {
		t.Fatal("second retry round should have installed the plan")
	}
}

func TestRunControllerPlanBatch_PlanEmitRejectionGetsSecondRetry(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("plan emit rejection retry")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "symptom-driven repair"}}})
	mu.SetWriteExplorationHandoff(&types.WriteExplorationHandoff{
		BatchID:     "batch-1",
		TargetFiles: []string{"sphinx/ext/autodoc/typehints.py"},
		EvidenceRefs: []types.WriteExplorationEvidenceRef{{
			ID:      "evidence-1",
			Kind:    "source",
			Source:  "sphinx/ext/autodoc/typehints.py",
			Summary: "typed evidence identifies field-list handling",
		}},
		Confidence: "medium",
	})
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	planCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		if stage != types.StagePlan {
			t.Fatalf("unexpected stage %s", stage)
		}
		planCalls++
		*stepsUsed++
		mu.ResetDispatchToolResults()
		switch planCalls {
		case 1:
			mu.AppendDispatchToolResult(types.ToolResult{
				ToolName: "emit_change_plan",
				Success:  false,
				Summary:  `emit_change_plan rejected: duplicate change for path "sphinx/ext/autodoc/typehints.py"`,
			})
			return &agent.StageOutput{Error: "the proposed change plan was rejected"}, nil
		case 2:
			hint := mu.PlanningHint()
			if !strings.Contains(hint, "duplicate change") {
				t.Fatalf("first retry hint must carry latest emit rejection, got %q", hint)
			}
			mu.AppendDispatchToolResult(types.ToolResult{
				ToolName: "emit_change_plan",
				Success:  false,
				Summary:  `emit_change_plan rejected: structured edit builder: old_text mismatch`,
			})
			return &agent.StageOutput{Error: "the proposed change plan was rejected"}, nil
		default:
			hint := mu.PlanningHint()
			if !strings.Contains(hint, "old_text mismatch") {
				t.Fatalf("second retry hint must carry latest emit rejection, got %q", hint)
			}
			mu.SetChangePlan(&types.ChangePlan{
				ID:          "plan-emit-retry",
				Status:      types.PlanStatusPending,
				Summary:     "repair localized typehint handling",
				TargetPaths: []string{"sphinx/ext/autodoc/typehints.py"},
				Changes: []types.FileChange{{
					Path: "sphinx/ext/autodoc/typehints.py",
					Kind: "patch",
					Edits: []types.StructuredEdit{{
						Kind:    "replace",
						OldText: "old",
						Content: "new",
					}},
				}},
			})
			return &agent.StageOutput{}, nil
		}
	}
	steps := 0
	if err := o.runControllerPlanBatch(&writeflow.WriteBatchPlan{ID: "batch-1", Goal: "repair autodoc typehints"}, &steps); err != nil {
		t.Fatalf("structured emit rejection should receive a bounded second retry, got %v", err)
	}
	if planCalls != 3 {
		t.Fatalf("plan stage calls = %d, want 3 (initial + two bounded retries)", planCalls)
	}
	if mu.ChangePlan() == nil {
		t.Fatal("second retry round should have installed the plan")
	}
}

func TestRunWriteControllerWorkflow_ReplanProbePassRestoresAppliedPlanForVerify(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("probe pass no-op replan")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "repair already applied"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, ReasonCode: "initial", Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "initial"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "apply"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "verify"},
		{Action: writeflow.ActionReplanBatch, ReasonCode: "repair", Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "repair"}},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "verify_existing"},
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

	planCalls := 0
	applyCalls := 0
	verifyCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		*stepsUsed++
		switch stage {
		case types.StagePlan:
			planCalls++
			if planCalls == 1 {
				mu.SetChangePlan(&types.ChangePlan{
					ID:          "plan-1",
					Status:      types.PlanStatusPending,
					Summary:     "first repair",
					TargetPaths: []string{"src/Fix.java"},
					Changes: []types.FileChange{{
						Path:      "src/Fix.java",
						Kind:      "modify",
						Rationale: "repair",
					}},
				})
				return &agent.StageOutput{}, nil
			}
			mu.AppendPlanStageProbeReport(&types.ChangeReport{
				PlanID:  "plan-1",
				Channel: types.ChangeReportChannelPlannerProbe,
				Passed:  true,
				TestResults: []types.TestResult{{
					AssertionID: "make-test",
					Kind:        types.TestResultKindUnit,
					Passed:      true,
				}},
			})
			return &agent.StageOutput{Error: "no change plan was produced this round"}, nil
		case types.StageApply:
			applyCalls++
			plan := mu.ChangePlan()
			if plan == nil {
				t.Fatal("apply stage needs active plan")
			}
			plan.AppliedCommitSHA = "sha-applied"
			plan.WorktreePath = t.TempDir()
			for i := range plan.Changes {
				plan.Changes[i].Apply = &types.FileChangeApplyRecord{Status: "applied"}
			}
			mu.SetChangePlan(plan)
			return &agent.StageOutput{}, nil
		case types.StageVerify:
			verifyCalls++
			plan := mu.ChangePlan()
			if plan == nil {
				t.Fatal("verify stage needs restored active plan")
			}
			if plan.ID != "plan-1" {
				t.Fatalf("verify should run against restored applied plan, got %q", plan.ID)
			}
			if verifyCalls == 1 {
				mu.SetChangeReport(&types.ChangeReport{
					PlanID:         plan.ID,
					Channel:        types.ChangeReportChannelPostApplyVerify,
					Passed:         false,
					FailureKind:    types.FailureKindTestsFailed,
					FailureSummary: "missing regression assertion",
					TestResults:    []types.TestResult{{AssertionID: "make-test", Passed: false, FailureDetail: "missing regression assertion"}},
				})
				return &agent.StageOutput{}, nil
			}
			mu.SetChangeReport(&types.ChangeReport{
				PlanID:  plan.ID,
				Channel: types.ChangeReportChannelPostApplyVerify,
				Passed:  true,
			})
			return &agent.StageOutput{}, nil
		default:
			t.Fatalf("unexpected stage %s", stage)
		}
		return &agent.StageOutput{}, nil
	}

	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if planCalls != 2 {
		t.Fatalf("plan calls = %d, want 2", planCalls)
	}
	if applyCalls != 1 {
		t.Fatalf("apply calls = %d, want 1; replan probe pass must not re-apply", applyCalls)
	}
	if verifyCalls != 2 {
		t.Fatalf("verify calls = %d, want 2", verifyCalls)
	}
	if store.last == nil || store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("workflow should complete after restored-plan verify, got %+v", store.last)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "planner_probe_passed_existing_worktree") {
		t.Fatalf("workflow progress should record typed probe-pass recovery: %+v", store.last.ProgressLedger)
	}
	if got := mu.VerifyFailureHandoff(); got != nil {
		t.Fatalf("green verify should clear failure handoff, got %+v", got)
	}
}

func TestRestoreActiveSliceCheckpointBeforeReplanResetsWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	mainRepo := t.TempDir()
	runGitForWorkflowRestoreTest(t, mainRepo, "init", "-q")
	runGitForWorkflowRestoreTest(t, mainRepo, "config", "user.email", "test@codrax")
	runGitForWorkflowRestoreTest(t, mainRepo, "config", "user.name", "test-user")
	if err := os.WriteFile(filepath.Join(mainRepo, "fix.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitForWorkflowRestoreTest(t, mainRepo, "add", "fix.txt")
	runGitForWorkflowRestoreTest(t, mainRepo, "commit", "-q", "-m", "base")

	worktreeBase := t.TempDir()
	wt := filepath.Join(worktreeBase, "wt")
	runGitForWorkflowRestoreTest(t, mainRepo, "worktree", "add", "--detach", wt, "HEAD")
	if err := os.WriteFile(filepath.Join(wt, "fix.txt"), []byte("good\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitForWorkflowRestoreTest(t, wt, "add", "fix.txt")
	runGitForWorkflowRestoreTest(t, wt, "commit", "-q", "-m", "good checkpoint")
	goodSHA := strings.TrimSpace(runGitForWorkflowRestoreTest(t, wt, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(wt, "fix.txt"), []byte("regressed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mu := types.NewMutableState("restore checkpoint")
	mu.SetChangePlan(&types.ChangePlan{ID: "plan-restore"})
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Mutable:      mu,
			WorktreePath: wt,
			RepoRoot:     wt,
		},
		worktreeBase: worktreeBase,
	}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-restore",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			ActiveSliceID: "slice-1",
			Slices: []types.WriteWorkflowSlice{{
				ID:     "slice-1",
				Status: types.ChangePlanSliceFailed,
				PlanID: "plan-restore",
				Checkpoint: &types.WriteWorkflowCheckpoint{
					Ref:          "refs/codrax/applied/plan-restore",
					CommitSHA:    goodSHA,
					WorktreePath: wt,
					Source:       "slice_apply",
				},
			}},
		}},
	}

	restored, err := o.restoreActiveSliceCheckpointBeforeReplan(run, "verify_failed_replan")
	if err != nil {
		t.Fatalf("restoreActiveSliceCheckpointBeforeReplan: %v", err)
	}
	if !restored {
		t.Fatal("expected checkpoint restore")
	}
	got, err := os.ReadFile(filepath.Join(wt, "fix.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "good\n" {
		t.Fatalf("worktree was not reset to checkpoint: %q", string(got))
	}
	slice := run.Batches[0].Slices[0]
	if slice.Restore == nil || slice.Restore.CommitSHA != goodSHA || slice.Restore.ReasonCode != "verify_failed_replan" {
		t.Fatalf("slice restore metadata missing: %+v", slice.Restore)
	}
	if len(run.Batches[0].SliceEvents) != 1 || run.Batches[0].SliceEvents[0].Event != types.WriteWorkflowSliceEventRestored {
		t.Fatalf("restore event missing: %+v", run.Batches[0].SliceEvents)
	}
	if mu.RepoRoot() != wt || o.busCtx.RepoRoot != wt {
		t.Fatalf("repo root should point at restored worktree, mutable=%q bus=%q", mu.RepoRoot(), o.busCtx.RepoRoot)
	}
}

func TestSafeWorkflowCheckpointWorktreePathRejectsExternalDirectory(t *testing.T) {
	allowedBase := t.TempDir()
	external := t.TempDir()
	o := &Orchestrator{
		busCtx:       &types.BusContext{WorktreePath: filepath.Join(allowedBase, "current")},
		worktreeBase: allowedBase,
	}
	if got, ok, reason := o.safeWorkflowCheckpointWorktreePath(external); ok || got != "" || reason != "untrusted_worktree" {
		t.Fatalf("external checkpoint path should be rejected, got path=%q ok=%v reason=%q", got, ok, reason)
	}
}

func runGitForWorkflowRestoreTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v (%s)", args, dir, err, out)
	}
	return string(out)
}

func TestRunWriteControllerWorkflow_ReplanCancelReportsAppliedPatch(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("interrupted after applied patch")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "repair already applied"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{
			Action:     writeflow.ActionExploreCode,
			ReasonCode: "need_context",
			ExplorationRequest: &types.WriteExplorationRequest{
				BatchID: "batch-1",
				Goal:    "locate failed repair",
			},
		},
		{Action: writeflow.ActionPlanBatch, ReasonCode: "initial", Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "initial"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "apply"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "verify"},
		{Action: writeflow.ActionReplanBatch, ReasonCode: "repair", Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "repair"}},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}, Language: "en"}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	o.readExplorationRunner = readExplorationRunnerFunc(func(*Orchestrator) (int, error) {
		return 1, nil
	})
	o.SetWriteRetryBudget(2)
	planCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		*stepsUsed++
		switch stage {
		case types.StagePlan:
			planCalls++
			if planCalls == 1 {
				mu.SetChangePlan(&types.ChangePlan{
					ID:          "plan-applied-then-canceled",
					Status:      types.PlanStatusPending,
					Summary:     "first repair",
					TargetPaths: []string{"src/fix.py"},
					Changes: []types.FileChange{{
						Path:       "src/fix.py",
						Kind:       "modify",
						NewContent: "def fixed():\n    return True\n",
					}},
				})
				return &agent.StageOutput{}, nil
			}
			return nil, context.Canceled
		case types.StageApply:
			plan := mu.ChangePlan()
			if plan == nil {
				t.Fatal("apply stage needs active plan")
			}
			plan.WorktreePath = t.TempDir()
			plan.AppliedCommitSHA = "sha-applied"
			for i := range plan.Changes {
				plan.Changes[i].Apply = &types.FileChangeApplyRecord{Status: "applied"}
			}
			mu.SetChangePlan(plan)
			return &agent.StageOutput{}, nil
		case types.StageVerify:
			plan := mu.ChangePlan()
			if plan == nil {
				t.Fatal("verify stage needs active plan")
			}
			mu.SetChangeReport(&types.ChangeReport{
				PlanID:         plan.ID,
				Passed:         false,
				FailureKind:    types.FailureKindTestsFailed,
				FailureSummary: "targeted regression failed",
				TestResults: []types.TestResult{{
					Kind:        types.TestResultKindUnit,
					AssertionID: "test_fix",
					Suite:       "tests/test_fix.py",
					Passed:      false,
				}},
			})
			return &agent.StageOutput{}, nil
		default:
			t.Fatalf("unexpected stage %s", stage)
		}
		return &agent.StageOutput{}, nil
	}
	steps := 0
	err := o.runWriteControllerWorkflow(&steps)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run should preserve cancellation error, got %v", err)
	}
	restored := mu.ChangePlan()
	if restored == nil || restored.ID != "plan-applied-then-canceled" {
		t.Fatalf("applied prior plan must be restored after interrupted replan, got %+v", restored)
	}
	result := mu.Result()
	for _, want := range []string{
		"already applied and preserved",
		"plan-applied-then-canceled",
		"refs/codrax/applied/plan-applied-then-canceled",
		"latest verification: failed",
	} {
		if !strings.Contains(result, want) {
			t.Fatalf("result missing %q:\n%s", want, result)
		}
	}
	if !mu.ResultIsPlain() {
		t.Fatal("interrupted applied-patch guidance must be plain text")
	}
	if !strings.Contains(o.busCtx.TaskState.LastError, "already applied and preserved") {
		t.Fatalf("LastError should surface applied-patch guidance, got %q", o.busCtx.TaskState.LastError)
	}
	if store.last == nil || !workflowProgressHasReason(store.last.ProgressLedger, "plan_batch_interrupted_after_applied_patch") {
		t.Fatalf("progress ledger should record interrupted applied patch, got %+v", store.last)
	}
	if store.last.Status == types.WriteWorkflowRunBlocked {
		t.Fatalf("canceled replan should preserve resumable workflow state, got blocked: %+v", store.last)
	}
}

func TestRunWriteControllerWorkflow_ReplanFailureAfterAppliedPatchBlocksRun(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("failed replan after applied patch")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "repair already applied"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, ReasonCode: "initial", Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "initial"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "apply"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "verify"},
		{Action: writeflow.ActionReplanBatch, ReasonCode: "repair", Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "repair"}},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			var decision writeflow.WriteWorkflowDecision
			if controllerCalls < len(decisions) {
				decision = decisions[controllerCalls]
			} else {
				decision = writeflow.WriteWorkflowDecision{
					Action:     writeflow.ActionBlock,
					ReasonCode: "unexpected_extra_controller_call",
					Reason:     "test fallback should not decide terminal state",
				}
			}
			controllerCalls++
			decision = writeflow.NormalizeWriteWorkflowDecision(decision)
			raw, err := json.Marshal(decision)
			if err != nil {
				t.Fatalf("marshal decision: %v", err)
			}
			ctx.Mutable.SetWriteWorkflowDecisionJSON(raw)
			return &agent.StageOutput{Data: raw}, nil
		},
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}, Language: "en"}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	o.SetWriteRetryBudget(2)
	planCalls := 0
	var appliedPlan *types.ChangePlan
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		*stepsUsed++
		switch stage {
		case types.StagePlan:
			planCalls++
			if planCalls == 1 {
				mu.SetChangePlan(&types.ChangePlan{
					ID:          "plan-applied-then-replan-failed",
					Status:      types.PlanStatusPending,
					Summary:     "first repair",
					TargetPaths: []string{"src/fix.py"},
					Changes: []types.FileChange{{
						Path:       "src/fix.py",
						Kind:       "modify",
						NewContent: "def fixed():\n    return True\n",
					}},
				})
				return &agent.StageOutput{}, nil
			}
			if appliedPlan != nil {
				mu.SetChangePlan(appliedPlan)
			}
			return &agent.StageOutput{Error: "planner stopped before emitting a replacement ChangePlan"}, nil
		case types.StageApply:
			plan := mu.ChangePlan()
			if plan == nil {
				t.Fatal("apply stage needs active plan")
			}
			plan.WorktreePath = t.TempDir()
			plan.AppliedCommitSHA = "sha-applied"
			for i := range plan.Changes {
				plan.Changes[i].Apply = &types.FileChangeApplyRecord{Status: "applied"}
			}
			snap := *plan
			snap.TargetPaths = append([]string(nil), plan.TargetPaths...)
			snap.Changes = append([]types.FileChange(nil), plan.Changes...)
			appliedPlan = &snap
			mu.SetChangePlan(plan)
			return &agent.StageOutput{}, nil
		case types.StageVerify:
			plan := mu.ChangePlan()
			if plan == nil {
				t.Fatal("verify stage needs active plan")
			}
			mu.SetChangeReport(&types.ChangeReport{
				PlanID:         plan.ID,
				Passed:         false,
				FailureKind:    types.FailureKindTestsFailed,
				FailureSummary: "targeted regression failed",
				TestResults: []types.TestResult{{
					Kind:        types.TestResultKindUnit,
					AssertionID: "test_fix",
					Suite:       "tests/test_fix.py",
					Passed:      false,
				}},
			})
			return &agent.StageOutput{}, nil
		default:
			t.Fatalf("unexpected stage %s", stage)
		}
		return &agent.StageOutput{}, nil
	}
	steps := 0
	err := o.runWriteControllerWorkflow(&steps)
	if err == nil || !strings.Contains(err.Error(), "reused the failed ChangePlan fingerprint") {
		t.Fatalf("run should fail loud on replacement planning failure, got %v; planCalls=%d progress=%+v", err, planCalls, store.last)
	}
	if store.last == nil {
		t.Fatal("workflow state was not persisted")
	}
	if store.last.Status != types.WriteWorkflowRunBlocked {
		t.Fatalf("workflow status = %s; want blocked; run=%+v", store.last.Status, store.last)
	}
	var batchStatus types.WriteWorkflowBatchStatus
	for _, batch := range store.last.Batches {
		if batch.ID == "batch-1" {
			batchStatus = batch.Status
			break
		}
	}
	if batchStatus != types.WriteWorkflowBatchBlocked {
		t.Fatalf("batch status = %s; want blocked", batchStatus)
	}
	for _, want := range []string{
		"plan_batch_interrupted_after_applied_patch",
		"plan_batch_interrupted_after_applied_patch_blocked",
	} {
		if !workflowProgressHasReason(store.last.ProgressLedger, want) {
			t.Fatalf("progress ledger missing %s: %+v", want, store.last.ProgressLedger)
		}
	}
	if !strings.Contains(mu.Result(), "already applied and preserved") {
		t.Fatalf("result should preserve applied-patch guidance, got:\n%s", mu.Result())
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

func TestRunWriteControllerWorkflow_NoPlanBlocksDurableRun(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("no plan blocks")
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "first"}},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	planCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		if stage != types.StagePlan {
			t.Fatalf("unexpected stage %s", stage)
		}
		planCalls++
		*stepsUsed++
		return &agent.StageOutput{Error: "no change plan was produced this round"}, nil
	}
	steps := 0
	err := o.runWriteControllerWorkflow(&steps)
	if err == nil || !strings.Contains(err.Error(), "no ChangePlan") {
		t.Fatalf("workflow should fail-loud on terminal no-plan, got %v", err)
	}
	if controllerCalls != 1 || planCalls != 1 {
		t.Fatalf("calls controller=%d plan=%d, want 1/1", controllerCalls, planCalls)
	}
	if store.last == nil {
		t.Fatal("workflow run should be persisted")
	}
	if store.last.Status != types.WriteWorkflowRunBlocked {
		t.Fatalf("workflow status = %s, want blocked: %+v", store.last.Status, store.last)
	}
	if len(store.last.Batches) != 1 || store.last.Batches[0].Status != types.WriteWorkflowBatchBlocked {
		t.Fatalf("active batch should be blocked after terminal no-plan: %+v", store.last.Batches)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "plan_batch_failed_blocked") {
		t.Fatalf("progress ledger missing plan_batch_failed_blocked: %+v", store.last.ProgressLedger)
	}
}

func TestRunWriteControllerWorkflow_CanceledPlanRecordsCanceledReason(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("no plan canceled")
	mu.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{PlanID: "plan-prev", BatchID: "batch-1", Attempt: 1})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "retry"}},
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
		if stage != types.StagePlan {
			t.Fatalf("unexpected stage %s", stage)
		}
		*stepsUsed++
		o.Cancel("write mode wall-time exceeded (600s)")
		return &agent.StageOutput{Error: "no change plan was produced this round"}, nil
	}
	steps := 0
	err := o.runWriteControllerWorkflow(&steps)
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("workflow should surface cancellation, got %v", err)
	}
	if store.last == nil {
		t.Fatal("workflow run should be persisted")
	}
	if store.last.Status == types.WriteWorkflowRunBlocked {
		t.Fatalf("canceled workflow should remain resumable, got blocked: %+v", store.last)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "plan_batch_canceled") {
		t.Fatalf("progress ledger missing plan_batch_canceled: %+v", store.last.ProgressLedger)
	}
	if workflowProgressHasReason(store.last.ProgressLedger, "plan_batch_failed_blocked") {
		t.Fatalf("canceled workflow must not be recorded as terminal no-plan block: %+v", store.last.ProgressLedger)
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
func TestRunWriteControllerWorkflow_BudgetExhaustionAutoAppliesReadyPlan(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("budget ready plan")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "budget ready plan"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "single safe change"}},
		// The ready-plan completion lane should apply before a second
		// controller decision is needed.
		{Action: writeflow.ActionApplyPlan, ReasonCode: "never_reached"},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	o.maxSteps = 2 // controller+plan exhaust the budget before a controller apply turn
	applyRan := false
	verifyRan := false
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			mu.SetChangePlan(&types.ChangePlan{
				ID:          "plan-budget-ready",
				Status:      types.PlanStatusPending,
				Summary:     "attempt",
				TargetPaths: []string{"pkg/fix.py"},
				Changes:     []types.FileChange{{Path: "pkg/fix.py", Kind: "modify", NewContent: "value = 1\n"}},
			})
		case types.StageApply:
			applyRan = true
		case types.StageVerify:
			verifyRan = true
			mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-budget-ready", Passed: true})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("ready-plan completion lane should finish the run, got %v", err)
	}
	if controllerCalls != 1 {
		t.Fatalf("ready-plan completion lane should not require a second controller turn, got %d", controllerCalls)
	}
	if !applyRan {
		t.Fatal("ready-plan completion lane must run apply")
	}
	if !verifyRan {
		t.Fatal("ready-plan completion lane must feed the existing completion verify path")
	}
	if store.last == nil || store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("run should complete, got %+v", store.last)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "budget_ready_plan_auto_apply") ||
		!workflowProgressHasReason(store.last.ProgressLedger, "budget_completion_verify") {
		t.Fatalf("ready-plan and verify completion lanes should be recorded: %+v", store.last.ProgressLedger)
	}
	if store.last.Batches[0].Status != types.WriteWorkflowBatchComplete {
		t.Fatalf("batch should be verified complete, got %+v", store.last.Batches[0])
	}
}

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
	o.maxSteps = 3 // controller+plan+auto-apply → exhausted before verify turn
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
	o.maxSteps = 3
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

func TestLastPlanEmitRejectionView_PrefersTypedRepairPackMetadata(t *testing.T) {
	pack := types.PlanRepairPack{
		ToolName:          "emit_change_plan",
		ReasonCode:        "old_text_mismatch",
		Message:           "structured edit old_text did not match current file bytes",
		FailingFieldPaths: []string{"$.changes[0].edits[0].old_text"},
		FailingPaths:      []string{"src/widget.py"},
		CurrentBytes: []types.PlanRepairCurrentBytes{{
			Path:            "src/widget.py",
			StartLine:       10,
			EndLine:         10,
			ExpectedOldText: "VALUE = 1\n",
			SuppliedOldText: "VALUE = 0\n",
			SafeEditKinds:   []string{"replace"},
		}},
	}
	results := []types.ToolResult{{
		ToolName: "emit_change_plan",
		Success:  false,
		Summary:  "emit_change_plan rejected: opaque prose\nPLAN_REPAIR_PACK:{not-consumed}",
		Repair: &types.ToolRepair{
			Code: types.PlanRepairToolCode,
			Metadata: map[string]string{
				types.PlanRepairMetadataKey: types.PlanRepairPackJSON(pack),
			},
		},
	}}

	view := lastPlanEmitRejectionView(results)
	if view.RepairPack == nil {
		t.Fatalf("expected typed repair pack view, got %+v", view)
	}
	hint := view.RenderHint()
	for _, want := range []string{
		"plan_repair_pack:",
		"reason_code: old_text_mismatch",
		"failing_fields: $.changes[0].edits[0].old_text",
		"failing_paths: src/widget.py",
		"expected_old_text:",
		"VALUE = 1",
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("rendered hint missing %q:\n%s", want, hint)
		}
	}
	if strings.Contains(hint, "PLAN_REPAIR_PACK") || strings.Contains(hint, "opaque prose") {
		t.Fatalf("rendered hint should consume metadata rather than raw summary:\n%s", hint)
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
		FailureSummary: "red", TestResults: []types.TestResult{{AssertionID: "TestX", Passed: false}},
		TestSurface: &types.TestSurface{Candidates: []types.TestSurfaceCandidate{{
			ID:            "make@.",
			Runner:        "make",
			WorkingDir:    ".",
			HasTestSignal: true,
		}}}}
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
				{Kind: "verify", Status: "failed", ReasonCode: "tests_failed", PlanID: "plan-resume", ReportID: "plan-resume.report.json", ArtifactRef: "plan-resume.attempt-1.diff", SurfaceRef: "plan-resume.attempt-1.surface.json"},
			},
		}},
	}
	store := &fakeWorkflowRunStore{active: active}
	mu := types.NewMutableState("resume hydration")
	decisions := []writeflow.WriteWorkflowDecision{
		// Resumed failed-verify runs must replan with the hydrated handoff
		// before reusing the worktree. The first apply request is normalized
		// to replan_batch by typed batch state.
		{Action: writeflow.ActionApplyPlan, ReasonCode: "resume_apply"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "resume_verify"},
		{Action: writeflow.ActionFinish, ReasonCode: "done"},
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
	var handoffAtPlan *types.VerifyFailureHandoff
	var handoffAtApply *types.VerifyFailureHandoff
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			handoffAtPlan = mu.VerifyFailureHandoff()
			mu.SetChangePlan(&types.ChangePlan{
				ID:          "plan-resume-replan",
				Status:      types.PlanStatusPending,
				Summary:     "resume replan",
				TargetPaths: []string{"pkg/file.py"},
			})
		case types.StageApply:
			handoffAtApply = mu.VerifyFailureHandoff()
		case types.StageVerify:
			mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-resume-replan", Passed: true})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("resumed run should complete: %v", err)
	}
	if handoffAtPlan == nil || handoffAtPlan.PlanID != "plan-resume" || len(handoffAtPlan.FailingTests) != 1 {
		t.Fatalf("resume must rebuild the verify-failure carrier before replan, got %+v", handoffAtPlan)
	}
	if handoffAtApply == nil || handoffAtApply.PlanID != "plan-resume" || len(handoffAtApply.FailingTests) != 1 {
		t.Fatalf("resume must rebuild the verify-failure carrier from the durable report, got %+v", handoffAtApply)
	}
	if handoffAtApply.DiffArtifactRef != "plan-resume.attempt-1.diff" {
		t.Fatalf("resume carrier should reference the persisted attempt diff, got %q", handoffAtApply.DiffArtifactRef)
	}
	if handoffAtApply.DiffArtifactPath != filepath.Join(planDir, "plan-resume.attempt-1.diff") {
		t.Fatalf("resume carrier should carry resolved attempt diff path, got %q", handoffAtApply.DiffArtifactPath)
	}
	if handoffAtApply.SurfaceArtifactRef != "plan-resume.attempt-1.surface.json" {
		t.Fatalf("resume carrier should reference the persisted test surface, got %q", handoffAtApply.SurfaceArtifactRef)
	}
	if handoffAtApply.SurfaceArtifactPath != filepath.Join(planDir, "plan-resume.attempt-1.surface.json") {
		t.Fatalf("resume carrier should carry resolved test surface path, got %q", handoffAtApply.SurfaceArtifactPath)
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
		{Action: writeflow.ActionBlock, ReasonCode: "first_batch_failed"},
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
			QuestionsForUser: []string{"Which database engine should the migration target?", "Is downtime acceptable?"},
			MissingFacts: []writeflow.MissingUserFact{{
				Kind:        "user_choice",
				Description: "database engine and downtime policy are user-owned deployment choices",
				EvidenceRef: "batch-1",
				Consumer:    "planner",
			}}},
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
	if !strings.Contains(result, "Missing facts:") ||
		!strings.Contains(result, "user_choice for planner") {
		t.Fatalf("typed missing facts must surface with ask_user result, got %q", result)
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
	if !workflowProgressHasReason(store.last.ProgressLedger, "batch_unverified") {
		t.Fatalf("no-tests completion must be recorded as unverified, got %+v", store.last.ProgressLedger)
	}
	if workflowProgressHasReason(store.last.ProgressLedger, "batch_verified") {
		t.Fatalf("no-tests completion must not be recorded as verified, got %+v", store.last.ProgressLedger)
	}
}

func TestRunWriteControllerWorkflow_NoTestsDoesNotReplan(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("no tests should not replan")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "script only"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "script"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "applied"},
		{Action: writeflow.ActionReplanBatch, ReasonCode: "mistaken_no_tests_replan", Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "repeat script"}},
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
	planCalls := 0
	verifyCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			planCalls++
			if planCalls > 1 {
				t.Fatalf("NoTestsRunners should complete unverified instead of replanning, planCalls=%d", planCalls)
			}
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-no-tests", Status: types.PlanStatusPending, Summary: "script", TargetPaths: []string{"tool.py"}})
		case types.StageVerify:
			verifyCalls++
			mu.SetChangeReport(&types.ChangeReport{
				PlanID:         "plan-no-tests",
				Passed:         false,
				FailureSummary: "fallback unittest loader errors after zero pytest collection",
				FailureKind:    types.FailureKindTestsFailed,
				NoTestsRunners: []string{"python"},
			})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if planCalls != 1 || verifyCalls != 1 {
		t.Fatalf("stage calls plan/verify = %d/%d, want 1/1", planCalls, verifyCalls)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "batch_unverified") {
		t.Fatalf("no-tests completion must be recorded as unverified, got %+v", store.last.ProgressLedger)
	}
	if workflowProgressHasReason(store.last.ProgressLedger, "mistaken_no_tests_replan") {
		t.Fatalf("workflow should not consume the mistaken replan decision after no-tests: %+v", store.last.ProgressLedger)
	}
}

func TestRunWriteControllerWorkflow_UnverifiedSuppressesFollowupExploreBatch(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	mu := types.NewMutableState("parser error should not start followup exploration")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "fix redirect method"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "fix redirect"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "applied"},
		{
			Action: writeflow.ActionExploreCode,
			ExplorationRequest: &types.WriteExplorationRequest{
				BatchID:              "batch-2",
				Goal:                 "inspect already applied fix",
				ExplorationQuestions: []string{"is the code change present?"},
				CandidatePaths:       []string{"requests/sessions.py"},
			},
			ReasonCode: "verify_failed_loader_error",
		},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	explorationCalls := 0
	o.SetReadExplorationRunner(readExplorationRunnerFunc(func(o *Orchestrator) (int, error) {
		explorationCalls++
		return 0, nil
	}))
	planCalls := 0
	verifyCalls := 0
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			planCalls++
			if planCalls > 1 {
				t.Fatalf("parser_error unverified should not replan, planCalls=%d", planCalls)
			}
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-parser-error", Status: types.PlanStatusPending, Summary: "fix redirect", TargetPaths: []string{"requests/sessions.py"}})
		case types.StageVerify:
			verifyCalls++
			mu.SetChangeReport(&types.ChangeReport{
				PlanID:         "plan-parser-error",
				Passed:         false,
				FailureSummary: "unittest collection/import failed before executing real test cases",
				FailureKind:    types.FailureKindParserError,
				NoTestsRunners: []string{"python"},
			})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("runWriteControllerWorkflow: %v", err)
	}
	if explorationCalls != 0 {
		t.Fatalf("follow-up exploration after parser_error unverified should be suppressed, got %d call(s)", explorationCalls)
	}
	if planCalls != 1 || verifyCalls != 1 {
		t.Fatalf("stage calls plan/verify = %d/%d, want 1/1", planCalls, verifyCalls)
	}
	if store.last == nil || store.last.Status != types.WriteWorkflowRunComplete {
		t.Fatalf("workflow should finish after accepting unverified verdict, got %+v", store.last)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "batch_unverified") {
		t.Fatalf("parser_error completion must be recorded as unverified, got %+v", store.last.ProgressLedger)
	}
	if !workflowProgressHasReason(store.last.ProgressLedger, "unverified_action_overridden") {
		t.Fatalf("follow-up explore must be recorded as overridden, got %+v", store.last.ProgressLedger)
	}
	if workflowProgressHasReason(store.last.ProgressLedger, "exploration_complete") {
		t.Fatalf("suppressed follow-up explore must not run exploration: %+v", store.last.ProgressLedger)
	}
}

func TestRunWriteControllerWorkflow_PendingApprovalPersistsWorkflowPlan(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	saver := &capturePlanSaver{dir: t.TempDir()}
	mu := types.NewMutableState("approval plan must persist")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "risky update"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "risky update"}},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	o.SetPlanSaver(saver)
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		if stage != types.StagePlan {
			t.Fatalf("unexpected stage %s", stage)
		}
		mu.SetChangePlan(&types.ChangePlan{
			ID:          "plan-needs-approval",
			Status:      types.PlanStatusPending,
			Summary:     "risky update",
			TargetPaths: []string{".github/workflows/ci.yml"},
			Approval: &types.WriteApprovalRecord{
				Action:     string(writeflow.ApprovalActionManual),
				ReasonCode: "manual_review_required",
			},
		})
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	err := o.runWriteControllerWorkflow(&steps)
	if err == nil || !strings.Contains(err.Error(), "pending approval") {
		t.Fatalf("workflow should pause for manual approval, got %v", err)
	}
	if len(saver.saved) == 0 {
		t.Fatal("pending approval plan was not persisted")
	}
	saved := saver.saved[len(saver.saved)-1]
	if saved.ID != "plan-needs-approval" {
		t.Fatalf("saved wrong plan: %+v", saved)
	}
	if store.last == nil || saved.PhaseGroupID != store.last.RunID || saved.PhaseIndex != 0 {
		t.Fatalf("saved plan missing workflow coordinates: plan=%+v run=%+v", saved, store.last)
	}
	if _, err := os.Stat(filepath.Join(saver.dir, "plan-needs-approval.json")); err != nil {
		t.Fatalf("persisted plan file missing: %v", err)
	}
	if store.last.Batches[0].PlanID != "plan-needs-approval" ||
		store.last.Batches[0].Status != types.WriteWorkflowBatchPendingApproval {
		t.Fatalf("workflow did not retain pending plan ref/status: %+v", store.last.Batches[0])
	}
}
