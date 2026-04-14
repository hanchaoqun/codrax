package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/config"
	"github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// ---------------------------------------------------------------------------
// Mock agent
// ---------------------------------------------------------------------------

type mockAgent struct {
	name   types.AgentName
	execFn func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error)
}

func (m *mockAgent) Name() types.AgentName { return m.name }

func (m *mockAgent) Execute(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
	if m.execFn != nil {
		return m.execFn(ctx, sk)
	}
	return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
}

func newMockAgent(name types.AgentName, fn func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error)) *mockAgent {
	return &mockAgent{name: name, execFn: fn}
}

// ---------------------------------------------------------------------------
// Test helpers — build a ResolvedConfig programmatically
// ---------------------------------------------------------------------------

// defaultResolvedConfig builds a full pipeline config suitable for most tests.
// Stages: analyze -> explore -> plan -> design_review -> implement -> code_review -> verify -> finalize
func defaultResolvedConfig() *config.ResolvedConfig {
	return &config.ResolvedConfig{
		Stages: map[types.PipelineStage]*types.StageConfig{
			types.StageAnalyze: {
				Name: types.StageAnalyze, DefaultAgent: types.AgentAnalyzer,
				DefaultSkill: "analyze-skill",
			},
			types.StageExplore: {
				Name: types.StageExplore, DefaultAgent: types.AgentExplorer,
				DefaultSkill: "explore-skill",
			},
			types.StagePlan: {
				Name: types.StagePlan, DefaultAgent: types.AgentPlanner,
				DefaultSkill: "plan-skill",
			},
			types.StageDesignReview: {
				Name: types.StageDesignReview, DefaultAgent: types.AgentDesignReviewer,
				DefaultSkill: "design-review-skill",
			},
			types.StageImplement: {
				Name: types.StageImplement, DefaultAgent: types.AgentImplementer,
				DefaultSkill: "implement-skill", RequiresWrite: true,
			},
			types.StageCodeReview: {
				Name: types.StageCodeReview, DefaultAgent: types.AgentCodeReviewer,
				DefaultSkill: "code-review-skill",
			},
			types.StageVerify: {
				Name: types.StageVerify, DefaultAgent: types.AgentVerifier,
				DefaultSkill: "verify-skill",
			},
			types.StageFinalize: {
				Name: types.StageFinalize, DefaultAgent: types.AgentFinalizer,
				DefaultSkill: "finalize-skill", Terminal: true,
			},
			// P2.1: extract stage between explore and finalize.
			// Routing is still controlled by the two_turn_explorer_mode
			// flag; this config entry is the lookup target for
			// runTaskGraph's extractStageEnabled() + GetStageConfig hop.
			types.StageExtract: {
				Name: types.StageExtract, DefaultAgent: types.AgentExtractor,
				DefaultSkill: "extract-skill",
			},
		},
		Transitions: map[types.PipelineStage][]types.Transition{
			types.StageAnalyze: {
				{From: types.StageAnalyze, To: types.StageExplore, Priority: 100},
				{From: types.StageAnalyze, To: types.StageFinalize, Priority: 10},
			},
			types.StageExplore: {
				{From: types.StageExplore, To: types.StagePlan, Priority: 100},
				{From: types.StageExplore, To: types.StageExplore, Priority: 80},
				{From: types.StageExplore, To: types.StageFinalize, Priority: 10},
			},
			types.StagePlan: {
				{From: types.StagePlan, To: types.StageDesignReview, Priority: 100},
				{From: types.StagePlan, To: types.StageImplement, Priority: 80},
				{From: types.StagePlan, To: types.StageFinalize, Priority: 10},
			},
			types.StageDesignReview: {
				{From: types.StageDesignReview, To: types.StageImplement, Priority: 100},
				{From: types.StageDesignReview, To: types.StageFinalize, Priority: 10},
			},
			types.StageImplement: {
				{From: types.StageImplement, To: types.StageCodeReview, Priority: 100},
				{From: types.StageImplement, To: types.StageVerify, Priority: 90},
				{From: types.StageImplement, To: types.StageFinalize, Priority: 10},
			},
			types.StageCodeReview: {
				{From: types.StageCodeReview, To: types.StageVerify, Priority: 100},
				{From: types.StageCodeReview, To: types.StageFinalize, Priority: 10},
			},
			types.StageVerify: {
				{From: types.StageVerify, To: types.StageFinalize, Priority: 100},
			},
		},
		TaskPolicies: map[string]*types.TaskPolicyConfig{
			"implementation": {
				Name: "implementation",
				AllowedStages: []types.PipelineStage{
					types.StageAnalyze, types.StageExplore, types.StagePlan,
					types.StageImplement, types.StageVerify,
					types.StageFinalize,
				},
			},
			"high_risk_implementation": {
				Name: "high_risk_implementation",
				AllowedStages: []types.PipelineStage{
					types.StageAnalyze, types.StageExplore, types.StagePlan,
					types.StageDesignReview, types.StageImplement, types.StageCodeReview,
					types.StageVerify, types.StageFinalize,
				},
			},
			"analysis": {
				Name: "analysis",
				AllowedStages: []types.PipelineStage{
					types.StageAnalyze, types.StageExplore, types.StageFinalize,
				},
			},
		},
		PipelineSettings: types.PipelineSettings{
			EnableVerify: true,
		},
		Agents: map[types.AgentName]*types.AgentConfig{},
		Skills: map[string]*types.SkillConfigYAML{},
	}
}

// implementationTaskList builds a TaskList for an implementation
// task. Post-B5b-β, the implementation policy is selected from
// AnalysisIR.RunPolicy, not TaskItem — so any test that wants this
// task list to route through the implementation policy must also
// attach implementationIR() to BusContext.AnalysisIR.
func implementationTaskList() *types.TaskList {
	return &types.TaskList{
		Objective: "implement a widget",
		Tasks: []types.TaskItem{{
			ID:     "t1",
			Title:  "implement",
			Status: types.TaskPending,
		}},
		CurrentTaskID: "t1",
	}
}

// implementationIR builds an AnalysisIR that routes the pipeline into
// the implementation policy. Pair with implementationTaskList on
// BusContext when a test needs both the task list and the policy.
func implementationIR() *types.AnalysisIR {
	return &types.AnalysisIR{
		RunPolicy: types.RunPolicy{Writing: true},
	}
}

// buildRegistries creates agent and skill registries with mock entries matching
// the default config. agentFns lets callers override the Execute function per agent.
func buildRegistries(agentFns map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error)) (*agent.Registry, *skill.Registry, *agent.SubAgentRegistry) {
	ar := agent.NewRegistry()
	names := []types.AgentName{
		types.AgentAnalyzer, types.AgentPlanner, types.AgentExplorer, types.AgentImplementer,
		types.AgentDesignReviewer, types.AgentCodeReviewer,
		types.AgentVerifier, types.AgentFinalizer,
		types.AgentExtractor, // P2.1: Turn B agent
	}
	for _, n := range names {
		var fn func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error)
		if agentFns != nil {
			fn = agentFns[n]
		}
		ar.Register(newMockAgent(n, fn))
	}

	sr := skill.NewRegistry()
	skillNames := []string{
		"analyze-skill", "explore-skill", "plan-skill",
		"implement-skill", "design-review-skill", "code-review-skill",
		"verify-skill", "finalize-skill",
		// analysis-final-answer-skill is registered so the dispatchStage
		// finalize-routing path can find it under analysis policy.
		// dispatchStage looks it up by name and falls back to the
		// stage's DefaultSkill if missing — see TestRun_FinalizeSkillRoutedByPolicy.
		"analysis-final-answer-skill",
		"extract-skill",          // P2.1: Turn B skill
		"answer-document-skill",  // P2.2: structured finalizer skill
	}
	for _, s := range skillNames {
		sr.Register(&skill.Config{Name: s, Goal: s + " goal"})
	}

	sar := agent.NewSubAgentRegistry()

	return ar, sr, sar
}

// ---------------------------------------------------------------------------
// decideNextStage tests
// ---------------------------------------------------------------------------

func TestDecideNextStage_AnalyzeToExplore(t *testing.T) {
	t.Run("after analyze with MissingFacts should transition to explore", func(t *testing.T) {
		cfg := defaultResolvedConfig()
		ar, sr, sar := buildRegistries(nil)
		o := New(cfg, ar, sr, sar)

		o.busCtx = &types.BusContext{
			PipelineStage: types.StageAnalyze,
			TaskState: types.TaskState{
				Stage:   types.StageAnalyze,
				Missing: types.MissingFacts,
			},
			Policy: types.PolicyContext{},
		}

		next := o.decideNextStage()
		if next != types.StageExplore {
			t.Errorf("expected explore, got %s", next)
		}
	})
}

func TestDecideNextStage_ExploreToPlain(t *testing.T) {
	t.Run("after explore with HasEnoughFacts should transition to plan", func(t *testing.T) {
		cfg := defaultResolvedConfig()
		ar, sr, sar := buildRegistries(nil)
		o := New(cfg, ar, sr, sar)

		o.busCtx = &types.BusContext{
			PipelineStage: types.StageExplore,
			Signals: types.ExecutionSignals{
				HasEnoughFacts: true,
			},
			TaskState: types.TaskState{
				Stage:   types.StageExplore,
				Missing: types.MissingPlan,
			},
			Policy:     types.PolicyContext{},
			Mutable:    types.NewMutableState(*implementationTaskList()),
			AnalysisIR: implementationIR(),
		}

		next := o.decideNextStage()
		if next != types.StagePlan {
			t.Errorf("expected plan, got %s", next)
		}
	})
}

func TestDecideNextStage_PlanToDesignReview(t *testing.T) {
	t.Run("after plan with HasPlan and RequireReview should transition to design_review", func(t *testing.T) {
		cfg := defaultResolvedConfig()
		ar, sr, sar := buildRegistries(nil)
		o := New(cfg, ar, sr, sar)

		o.busCtx = &types.BusContext{
			PipelineStage: types.StagePlan,
			Signals: types.ExecutionSignals{
				HasEnoughFacts: true,
				HasPlan:        true,
			},
			TaskState: types.TaskState{
				Stage:   types.StagePlan,
				Missing: types.MissingCode,
			},
			Policy:     types.PolicyContext{RequireReview: true},
			Mutable:    types.NewMutableState(*implementationTaskList()),
			AnalysisIR: implementationIR(),
		}

		next := o.decideNextStage()
		if next != types.StageDesignReview {
			t.Errorf("expected design_review, got %s", next)
		}
	})
}

func TestDecideNextStage_ImplementToCodeReview(t *testing.T) {
	t.Run("after implement with HasPatch and RequireReview should transition to code_review", func(t *testing.T) {
		cfg := defaultResolvedConfig()
		ar, sr, sar := buildRegistries(nil)
		o := New(cfg, ar, sr, sar)

		o.busCtx = &types.BusContext{
			PipelineStage: types.StageImplement,
			Signals: types.ExecutionSignals{
				HasPlan:  true,
				HasPatch: true,
			},
			TaskState: types.TaskState{
				Stage:   types.StageImplement,
				Missing: types.MissingVerification,
			},
			Policy:     types.PolicyContext{RequireReview: true},
			Mutable:    types.NewMutableState(*implementationTaskList()),
			AnalysisIR: implementationIR(),
		}

		next := o.decideNextStage()
		if next != types.StageCodeReview {
			t.Errorf("expected code_review, got %s", next)
		}
	})
}

func TestDetermineActivePolicy_IR(t *testing.T) {
	cfg := defaultResolvedConfig()
	ar, sr, sar := buildRegistries(nil)
	o := New(cfg, ar, sr, sar)

	// high_risk_implementation: writing + either review flag.
	o.busCtx = &types.BusContext{
		Policy: types.PolicyContext{},
		AnalysisIR: &types.AnalysisIR{
			RunPolicy: types.RunPolicy{
				Writing:             true,
				RequireDesignReview: true,
				RequireCodeReview:   true,
			},
		},
	}
	if got := o.determineActivePolicy(); got != "high_risk_implementation" {
		t.Errorf("high-risk: got %q", got)
	}

	// implementation: writing with no review flags.
	o.busCtx.AnalysisIR.RunPolicy = types.RunPolicy{Writing: true}
	if got := o.determineActivePolicy(); got != "implementation" {
		t.Errorf("writing: got %q", got)
	}

	// analysis: writing=false.
	o.busCtx.AnalysisIR.RunPolicy = types.RunPolicy{Writing: false}
	if got := o.determineActivePolicy(); got != "analysis" {
		t.Errorf("writing=false: got %q", got)
	}

	// Policy.RequireReview escalates a writing IR task even when the
	// IR itself didn't require a review.
	o.busCtx.AnalysisIR.RunPolicy = types.RunPolicy{Writing: true}
	o.busCtx.Policy = types.PolicyContext{RequireReview: true}
	if got := o.determineActivePolicy(); got != "high_risk_implementation" {
		t.Errorf("RequireReview escalation: got %q", got)
	}
}

func TestDetermineActivePolicy_NoIRFailsSafe(t *testing.T) {
	cfg := defaultResolvedConfig()
	ar, sr, sar := buildRegistries(nil)
	o := New(cfg, ar, sr, sar)

	// No AnalysisIR (analyze failed, pre-IR unit tests) → fail-safe
	// "analysis" regardless of what else is on BusContext.
	o.busCtx = &types.BusContext{
		Mutable: types.NewMutableState(types.TaskList{Objective: "nothing"}),
		Policy:  types.PolicyContext{RequireReview: true},
	}
	if got := o.determineActivePolicy(); got != "analysis" {
		t.Errorf("no IR: got %q", got)
	}
}

func TestDecideNextStage_PolicyFiltering(t *testing.T) {
	t.Run("analysis policy filters out plan and implement so explore goes to finalize", func(t *testing.T) {
		cfg := defaultResolvedConfig()
		ar, sr, sar := buildRegistries(nil)
		o := New(cfg, ar, sr, sar)

		// Set up an analysis task so the "analysis" policy is active.
		o.busCtx = &types.BusContext{
			PipelineStage: types.StageExplore,
			Mutable: types.NewMutableState(types.TaskList{
				Objective:     "analyze something",
				CurrentTaskID: "t1",
				Tasks: []types.TaskItem{
					{ID: "t1", Title: "analysis task", Status: types.TaskInProgress},
				},
			}),
			Signals: types.ExecutionSignals{
				HasEnoughFacts: true,
			},
			TaskState: types.TaskState{
				Stage:   types.StageExplore,
				Missing: types.MissingNone,
			},
			Policy: types.PolicyContext{},
		}

		next := o.decideNextStage()
		if next != types.StageFinalize {
			t.Errorf("expected finalize (policy should filter plan/implement), got %s", next)
		}
	})
}

func TestDecideNextStage_FallbackToFinalize(t *testing.T) {
	t.Run("when no valid transitions remain, should fall back to finalize", func(t *testing.T) {
		cfg := defaultResolvedConfig()
		// Remove all transitions from verify stage
		cfg.Transitions[types.StageVerify] = nil
		ar, sr, sar := buildRegistries(nil)
		o := New(cfg, ar, sr, sar)

		o.busCtx = &types.BusContext{
			PipelineStage: types.StageVerify,
			Signals: types.ExecutionSignals{
				HasPatch:           true,
				VerificationPassed: true,
			},
			TaskState: types.TaskState{
				Stage:   types.StageVerify,
				Missing: types.MissingNone,
			},
			Policy: types.PolicyContext{},
		}

		next := o.decideNextStage()
		if next != types.StageFinalize {
			t.Errorf("expected finalize (no transitions), got %s", next)
		}
	})
}

// ---------------------------------------------------------------------------
// Full pipeline test
// ---------------------------------------------------------------------------

func TestRun_SimplePipeline(t *testing.T) {
	t.Run("pipeline progresses through analyze->explore->plan->implement->verify->finalize", func(t *testing.T) {
		cfg := defaultResolvedConfig()
		// RequireReview defaults to false, so implementation policy is used
		// and review stages are not in allowed_stages — pipeline skips reviews.

		agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
			// analyze: classifies the request as implementation by writing
			// directly to the shared mutable state, mirroring how a real
			// analyzer would mutate it via the todo_write tool.
			types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				ctx.Mutable.SetTaskList(*implementationTaskList())
				return &agent.StageOutput{
					MissingPiece: types.MissingFacts,
					AnalysisIR:   implementationIR(),
				}, nil
			},
			// plan: reports HasPlan and MissingCode
			types.AgentPlanner: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				return &agent.StageOutput{
					MissingPiece: types.MissingCode,
					SignalUpdates: &types.ExecutionSignals{
						HasPlan: true,
					},
				}, nil
			},
			// explore: reports HasEnoughFacts and MissingPlan
			types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				return &agent.StageOutput{
					MissingPiece: types.MissingPlan,
					SignalUpdates: &types.ExecutionSignals{
						HasEnoughFacts: true,
					},
				}, nil
			},
			// implement: reports HasPatch and MissingVerification
			types.AgentImplementer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				return &agent.StageOutput{
					MissingPiece: types.MissingVerification,
					SignalUpdates: &types.ExecutionSignals{
						HasPatch: true,
					},
				}, nil
			},
			// design reviewer: not used in this test (review disabled)
			types.AgentDesignReviewer: nil,
			// code reviewer: not used in this test (review disabled)
			types.AgentCodeReviewer: nil,
			// verify: reports VerificationPassed and MissingNone
			types.AgentVerifier: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				return &agent.StageOutput{
					MissingPiece: types.MissingNone,
					SignalUpdates: &types.ExecutionSignals{
						VerificationPassed: true,
					},
				}, nil
			},
			// finalize: done
			types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				return &agent.StageOutput{
					MissingPiece: types.MissingNone,
				}, nil
			},
		}

		ar, sr, sar := buildRegistries(agentFns)
		o := New(cfg, ar, sr, sar)
		o.SetMaxSteps(20)

		busCtx, err := o.Run("implement a widget", "/tmp/repo", "main")
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}

		// Verify terminal state
		if !busCtx.TaskState.IsTerminal {
			t.Error("expected pipeline to reach terminal state")
		}

		// Verify completed stages
		completed := busCtx.TaskState.Completed
		expected := []types.PipelineStage{
			types.StageAnalyze,
			types.StageExplore,
			types.StagePlan,
			types.StageImplement,
			types.StageVerify,
			types.StageFinalize,
		}

		if len(completed) != len(expected) {
			t.Fatalf("expected %d completed stages, got %d: %v", len(expected), len(completed), completed)
		}

		for i, want := range expected {
			got := types.PipelineStage(completed[i])
			if got != want {
				t.Errorf("completed[%d] = %s, want %s", i, got, want)
			}
		}
	})
}

func TestRun_PipelineWithBothReviews(t *testing.T) {
	t.Run("pipeline with reviews: analyze->explore->plan->design_review->implement->code_review->verify->finalize", func(t *testing.T) {
		cfg := defaultResolvedConfig()

		agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
			types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				ctx.Mutable.SetTaskList(*implementationTaskList())
				return &agent.StageOutput{
					MissingPiece: types.MissingFacts,
					AnalysisIR:   implementationIR(),
				}, nil
			},
			types.AgentPlanner: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				return &agent.StageOutput{
					MissingPiece:  types.MissingReview,
					SignalUpdates: &types.ExecutionSignals{HasPlan: true},
				}, nil
			},
			types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				return &agent.StageOutput{
					MissingPiece:  types.MissingPlan,
					SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				}, nil
			},
			types.AgentDesignReviewer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				return &agent.StageOutput{
					MissingPiece:  types.MissingCode,
					SignalUpdates: &types.ExecutionSignals{DesignReviewPassed: true},
				}, nil
			},
			types.AgentImplementer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				return &agent.StageOutput{
					MissingPiece:  types.MissingReview,
					SignalUpdates: &types.ExecutionSignals{HasPatch: true},
				}, nil
			},
			types.AgentCodeReviewer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				return &agent.StageOutput{
					MissingPiece:  types.MissingVerification,
					SignalUpdates: &types.ExecutionSignals{CodeReviewPassed: true},
				}, nil
			},
			types.AgentVerifier: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				return &agent.StageOutput{
					MissingPiece:  types.MissingNone,
					SignalUpdates: &types.ExecutionSignals{VerificationPassed: true},
				}, nil
			},
			types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
			},
		}

		ar, sr, sar := buildRegistries(agentFns)
		o := New(cfg, ar, sr, sar)

		// Manually construct busCtx with RequireReview to activate high_risk_implementation policy,
		// instead of adding test-only API surface to Orchestrator.
		o.busCtx = &types.BusContext{
			PipelineStage: types.StageAnalyze,
			RepoRoot:      "/tmp/repo",
			Branch:        "main",
			TraceID:       "trace-test-reviews",
			Mutable:       types.NewMutableState(types.TaskList{Objective: "implement a risky feature"}),
			TaskState: types.TaskState{
				Stage:   types.StageAnalyze,
				Missing: types.MissingUnderstanding,
			},
			Policy: types.PolicyContext{
				RequireReview:      true,
				MaxRetriesPerStage: 3,
			},
		}

		// Drive the pipeline loop manually
		for step := 0; step < 20; step++ {
			stageConfig, err := cfg.GetStageConfig(o.busCtx.PipelineStage)
			if err != nil {
				t.Fatalf("step %d: unknown stage %s: %v", step, o.busCtx.PipelineStage, err)
			}

			if _, err := o.dispatchStage(stageConfig); err != nil {
				t.Fatalf("step %d: stage %s failed: %v", step, o.busCtx.PipelineStage, err)
			}

			if stageConfig.Terminal {
				o.busCtx.TaskState.IsTerminal = true
				break
			}

			nextStage := o.decideNextStage()
			o.busCtx.PipelineStage = nextStage
			o.busCtx.TaskState.Stage = nextStage
		}

		if !o.busCtx.TaskState.IsTerminal {
			t.Error("expected pipeline to reach terminal state")
		}

		completed := o.busCtx.TaskState.Completed
		expected := []types.PipelineStage{
			types.StageAnalyze,
			types.StageExplore,
			types.StagePlan,
			types.StageDesignReview,
			types.StageImplement,
			types.StageCodeReview,
			types.StageVerify,
			types.StageFinalize,
		}

		if len(completed) != len(expected) {
			t.Fatalf("expected %d completed stages, got %d: %v", len(expected), len(completed), completed)
		}

		for i, want := range expected {
			got := types.PipelineStage(completed[i])
			if got != want {
				t.Errorf("completed[%d] = %s, want %s", i, got, want)
			}
		}

		// Verify both review signals were set
		if !o.busCtx.Signals.DesignReviewPassed {
			t.Error("expected DesignReviewPassed to be true")
		}
		if !o.busCtx.Signals.CodeReviewPassed {
			t.Error("expected CodeReviewPassed to be true")
		}
	})
}

// TestRun_AnalysisPolicyFromMutable verifies that an analyzer that
// writes an Analysis-typed task into the shared mutable state drives
// the orchestrator into the analysis policy, so the pipeline avoids
// the implement / verify stages.
func TestRun_AnalysisPolicyFromMutable(t *testing.T) {
	cfg := defaultResolvedConfig()

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			ctx.Mutable.SetTaskList(types.TaskList{
				Objective: "explain the project",
				Tasks: []types.TaskItem{{
					ID: "t1", Title: "explain", Status: types.TaskPending,
				}},
				CurrentTaskID: "t1",
			})
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
			}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece:  types.MissingNone,
				SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
			}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "this project is a 5-layer agent system",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(cfg, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("explain what this project does", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// Pipeline must reach a terminal state.
	if !busCtx.TaskState.IsTerminal {
		t.Error("expected pipeline to reach terminal state")
	}

	// busCtx.Mutable.TaskList must reflect the analyzer's update.
	tl := busCtx.Mutable.TaskList()
	current := tl.CurrentTask()
	if current == nil {
		t.Fatal("expected BusContext.Mutable.TaskList to be populated from analyzer")
	}
	// Post-B5b-β, "read-only task" is asserted on the IR, not TaskItem.
	if busCtx.AnalysisIR != nil && busCtx.AnalysisIR.RunPolicy.Writing {
		t.Errorf("analysis task should produce a non-writing RunPolicy")
	}

	// Pipeline must NOT visit implement / verify under analysis policy.
	for _, s := range busCtx.TaskState.Completed {
		stage := types.PipelineStage(s)
		if stage == types.StageImplement || stage == types.StageVerify {
			t.Errorf("analysis pipeline should not reach %s, completed = %v", stage, busCtx.TaskState.Completed)
		}
	}

	// The finalizer's answer must land on the task's Result field.
	if current.Result != "this project is a 5-layer agent system" {
		t.Errorf("task.Result = %q, want propagated finalizer answer", current.Result)
	}
}

// TestRun_FinalizeSkillRoutedByPolicy locks in the dispatchStage
// override that picks analysis-final-answer-skill instead of the
// configured DefaultSkill ("finalize-skill") when the active policy
// is analysis. This routing exists because final-answer-skill's
// workflow is shaped for implementation tasks ("summarize all
// changes, compile patch information, write usage instructions, list
// action steps, mark tasks complete") and forcing a Q&A answer
// through that template diluted precise quantitative answers ("1
// SubExplorer") into mushy ones ("several components facilitate
// subagent management"). The fix is at the orchestrator dispatch
// layer rather than inside finalizer because the finalizer mock here
// — and the real finalizer — both consume whatever skill they are
// given; routing is the orchestrator's job.
//
// Implementation pipelines must keep the original DefaultSkill, so
// the test asserts both directions.
func TestRun_FinalizeSkillRoutedByPolicy(t *testing.T) {
	t.Run("analysis policy → analysis-final-answer-skill", func(t *testing.T) {
		var observedFinalizeSkill string

		cfg := defaultResolvedConfig()
		agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
			types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				ctx.Mutable.SetTaskList(types.TaskList{
					Objective: "explain something",
					Tasks: []types.TaskItem{{
						ID: "t1", Title: "explain", Status: types.TaskPending,
					}},
					CurrentTaskID: "t1",
				})
				return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
			},
			types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				return &agent.StageOutput{
					MissingPiece:  types.MissingNone,
					SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				}, nil
			},
			types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				observedFinalizeSkill = sk.Name
				return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "ok"}, nil
			},
		}

		ar, sr, sar := buildRegistries(agentFns)
		o := New(cfg, ar, sr, sar)
		o.SetMaxSteps(20)

		if _, err := o.Run("explain", "/tmp/repo", "main"); err != nil {
			t.Fatalf("Run: %v", err)
		}

		if observedFinalizeSkill != "analysis-final-answer-skill" {
			t.Errorf("finalize skill = %q, want analysis-final-answer-skill", observedFinalizeSkill)
		}
	})

	t.Run("implementation policy → default finalize-skill", func(t *testing.T) {
		var observedFinalizeSkill string

		cfg := defaultResolvedConfig()
		agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
			types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				ctx.Mutable.SetTaskList(*implementationTaskList())
				return &agent.StageOutput{
					MissingPiece: types.MissingFacts,
					AnalysisIR:   implementationIR(),
				}, nil
			},
			types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				return &agent.StageOutput{
					MissingPiece:  types.MissingPlan,
					SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				}, nil
			},
			types.AgentPlanner: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				return &agent.StageOutput{
					MissingPiece:  types.MissingCode,
					SignalUpdates: &types.ExecutionSignals{HasPlan: true},
				}, nil
			},
			types.AgentImplementer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				return &agent.StageOutput{
					MissingPiece:  types.MissingVerification,
					SignalUpdates: &types.ExecutionSignals{HasPatch: true},
				}, nil
			},
			types.AgentVerifier: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				return &agent.StageOutput{
					MissingPiece:  types.MissingNone,
					SignalUpdates: &types.ExecutionSignals{VerificationPassed: true},
				}, nil
			},
			types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				observedFinalizeSkill = sk.Name
				return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "done"}, nil
			},
		}

		ar, sr, sar := buildRegistries(agentFns)
		o := New(cfg, ar, sr, sar)
		o.SetMaxSteps(30)

		if _, err := o.Run("implement a widget", "/tmp/repo", "main"); err != nil {
			t.Fatalf("Run: %v", err)
		}

		// Implementation tasks must keep the implementation-shaped finalizer
		// skill — never get rerouted to the analysis variant.
		if observedFinalizeSkill != "finalize-skill" {
			t.Errorf("finalize skill = %q, want finalize-skill (implementation pipeline must not be rerouted)", observedFinalizeSkill)
		}
	})
}

// TestRun_ForcesFinalizeWhenMaxStepsExhausted verifies that when the
// pipeline runs out of steps before reaching a terminal stage, the
// orchestrator forces one finalizer call so the caller always sees a
// terminal state and the FinalAnswer plumbing still gets exercised.
func TestRun_ForcesFinalizeWhenMaxStepsExhausted(t *testing.T) {
	cfg := defaultResolvedConfig()

	// Build a pipeline where every non-finalize agent reports
	// "make no progress" so transitions oscillate and the loop runs
	// out of steps without naturally hitting finalize.
	noProgress := func(missing types.MissingPiece) func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
		return func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: missing}, nil
		}
	}

	finalizerCalled := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			ctx.Mutable.SetTaskList(*implementationTaskList())
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				AnalysisIR:   implementationIR(),
			}, nil
		},
		types.AgentExplorer:    noProgress(types.MissingPlan),
		types.AgentPlanner:     noProgress(types.MissingCode),
		types.AgentImplementer: noProgress(types.MissingCode), // never reports HasPatch
		types.AgentVerifier:    noProgress(types.MissingVerification),
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizerCalled++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "forced summary after max-steps",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(cfg, ar, sr, sar)
	o.SetMaxSteps(3) // intentionally small so we can't possibly reach finalize naturally

	busCtx, err := o.Run("implement a thing", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if !busCtx.TaskState.IsTerminal {
		t.Error("expected pipeline to be marked terminal after forced finalize")
	}
	if busCtx.PipelineStage != types.StageFinalize {
		t.Errorf("PipelineStage = %s, want finalize", busCtx.PipelineStage)
	}
	if finalizerCalled != 1 {
		t.Errorf("finalizer call count = %d, want exactly 1", finalizerCalled)
	}
	tl := busCtx.Mutable.TaskList()
	if len(tl.Tasks) == 0 {
		t.Fatal("expected at least one task on the list")
	}
	if got := tl.Tasks[0].Result; got != "forced summary after max-steps" {
		t.Errorf("task.Result = %q, want forced summary", got)
	}
	if tl.Tasks[0].Status != types.TaskDone {
		t.Errorf("task.Status = %s, want done after forced finalize", tl.Tasks[0].Status)
	}
	if busCtx.TaskState.LastError == "" {
		t.Error("expected LastError to record max-steps exhaustion")
	}
}

// TestRun_MultiTaskExecution verifies the multi-task execution model:
// when the analyzer produces N tasks, each task runs through its own
// per-task pipeline, ends with its own finalize call, and writes its
// own Result onto the task. The orchestrator iterates over pending
// tasks until none remain.
//
// Post-B5b-β, the RunPolicy is frozen on BusContext.AnalysisIR at the
// analyze stage and governs every task in the run uniformly — the
// per-task mix of "some analysis, some implementation" the pre-β
// version of this test asserted is no longer supported (the memo
// called this out as "Orchestrator HighRisk mapping is lossy — defer
// to β"). The test now exercises 3 writing tasks under one shared
// implementation policy.
func TestRun_MultiTaskExecution(t *testing.T) {
	cfg := defaultResolvedConfig()

	exploreCount := 0
	planCount := 0
	implementCount := 0
	verifyCount := 0
	finalizerCount := 0

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		// Analyzer plants three writing tasks under one implementation
		// RunPolicy.
		types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			ctx.Mutable.SetTaskList(types.TaskList{
				Objective: "three-part request",
				Tasks: []types.TaskItem{
					{ID: "ta", Title: "implement X", Status: types.TaskPending},
					{ID: "tb", Title: "implement Y", Status: types.TaskPending},
					{ID: "tc", Title: "implement Z", Status: types.TaskPending},
				},
				CurrentTaskID: "ta",
			})
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				AnalysisIR:   implementationIR(),
			}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			exploreCount++
			return &agent.StageOutput{
				MissingPiece:  types.MissingPlan,
				SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
			}, nil
		},
		types.AgentPlanner: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			planCount++
			return &agent.StageOutput{
				MissingPiece:  types.MissingCode,
				SignalUpdates: &types.ExecutionSignals{HasPlan: true},
			}, nil
		},
		types.AgentImplementer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			implementCount++
			return &agent.StageOutput{
				MissingPiece:  types.MissingVerification,
				SignalUpdates: &types.ExecutionSignals{HasPatch: true},
			}, nil
		},
		types.AgentVerifier: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			verifyCount++
			return &agent.StageOutput{
				MissingPiece:  types.MissingNone,
				SignalUpdates: &types.ExecutionSignals{VerificationPassed: true},
			}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizerCount++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "answer for " + ctx.CurrentTaskID,
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(cfg, ar, sr, sar)
	o.SetMaxSteps(60)

	busCtx, err := o.Run("two-part request", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if !busCtx.TaskState.IsTerminal {
		t.Error("expected pipeline to terminate")
	}

	tl := busCtx.Mutable.TaskList()
	if len(tl.Tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tl.Tasks))
	}

	// Each task should have its own answer and be marked done.
	expectedAnswers := map[string]string{
		"ta": "answer for ta",
		"tb": "answer for tb",
		"tc": "answer for tc",
	}
	for _, task := range tl.Tasks {
		if task.Status != types.TaskDone {
			t.Errorf("task %s status = %s, want done", task.ID, task.Status)
		}
		if want, ok := expectedAnswers[task.ID]; ok && task.Result != want {
			t.Errorf("task %s result = %q, want %q", task.ID, task.Result, want)
		}
	}

	// Finalizer must have run once per task.
	if finalizerCount != 3 {
		t.Errorf("finalizer call count = %d, want 3 (once per task)", finalizerCount)
	}

	// All 3 tasks share the implementation policy so every stage runs
	// once per task.
	if exploreCount != 3 {
		t.Errorf("explore count = %d, want 3 (once per task)", exploreCount)
	}
	if planCount != 3 {
		t.Errorf("plan count = %d, want 3 (once per task)", planCount)
	}
	if implementCount != 3 {
		t.Errorf("implement count = %d, want 3 (once per task)", implementCount)
	}
	if verifyCount != 3 {
		t.Errorf("verify count = %d, want 3 (once per task)", verifyCount)
	}
}

// TestRun_ExplorerFactSourceFlowsToFinalizer is an end-to-end test that
// verifies the fix for the RepoFact.Source semantic mismatch. Before the
// fix, explorer's ParseOutput filled Source with the tool name ("read_file",
// "grep"), but downstream consumers (extractRelevantFiles, filterFilesByScope,
// BuildPromptContext) treated Source as a repository-relative file path.
// This caused the finalizer's "Relevant Files" prompt section to contain
// meaningless entries like ["read_file", "grep"] instead of actual paths.
//
// The test simulates: analyze → explore → finalize (analysis policy).
// The explorer mock returns NewFacts whose Source fields are logical file
// paths (as produced by logicalFactSource) and EvidenceRef fields carry
// the raw blob references. Assertions check:
//   1. Facts accumulate on BusContext.RepoFacts with correct Source/EvidenceRef
//   2. The finalizer's AgentContext.RelevantFiles contains real file paths
//   3. The finalizer's AgentContext.RelevantFiles is properly deduplicated
//   4. The finalizer's RelevantFacts includes source annotations with file paths
//   5. BuildPromptContext renders "Relevant Files" with paths, not tool names
func TestRun_ExplorerFactSourceFlowsToFinalizer(t *testing.T) {
	cfg := defaultResolvedConfig()

	// Capture what the finalizer actually receives.
	var finalizerRelevantFiles []string
	var finalizerRelevantFacts []string

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			ctx.Mutable.SetTaskList(types.TaskList{
				Objective: "explain how the explorer works",
				Tasks: []types.TaskItem{{
					ID: "t1", Title: "explain explorer", Status: types.TaskPending,
				}},
				CurrentTaskID: "t1",
			})
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		// Explorer returns facts that mimic what logicalFactSource produces:
		// Source = file path (not tool name), EvidenceRef = blob reference.
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				NewFacts: []types.RepoFact{
					{
						Key:         "read_file",
						Value:       "[internal/agent/explorer.go: showing lines 1-50 of 800]\npackage agent\n...",
						Source:      "internal/agent/explorer.go",
						EvidenceRef: "blob://trace/tool-result-1.txt",
						Confidence:  0.8,
					},
					{
						Key:         "grep",
						Value:       "[internal/agent/explorer.go:615] facts = append(facts, types.RepoFact{",
						Source:      "internal/agent/explorer.go",
						EvidenceRef: "blob://trace/tool-result-2.txt",
						Confidence:  0.8,
					},
					{
						Key:         "read_file",
						Value:       "[internal/context/builder.go: showing lines 371-381 of 442]\nfunc extractRelevantFiles...",
						Source:      "internal/context/builder.go",
						EvidenceRef: "blob://trace/tool-result-3.txt",
						Confidence:  0.8,
					},
					{
						Key:         "repo_map",
						Value:       "project structure overview",
						Source:      "tool:repo_map",
						EvidenceRef: "",
						Confidence:  0.3,
					},
				},
			}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			// Capture what the finalizer receives via BuildAgentContext.
			finalizerRelevantFiles = make([]string, len(ctx.RelevantFiles))
			copy(finalizerRelevantFiles, ctx.RelevantFiles)
			finalizerRelevantFacts = make([]string, len(ctx.RelevantFacts))
			copy(finalizerRelevantFacts, ctx.RelevantFacts)
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "The explorer uses a 3-phase investigation model.",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(cfg, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("explain how the explorer works", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if !busCtx.TaskState.IsTerminal {
		t.Error("expected pipeline to reach terminal state")
	}

	// --- Assertion 1: BusContext.RepoFacts accumulated correctly ---
	if len(busCtx.RepoFacts) != 4 {
		t.Fatalf("BusContext.RepoFacts count = %d, want 4", len(busCtx.RepoFacts))
	}
	// Verify Source is a file path, not a tool name.
	for _, f := range busCtx.RepoFacts {
		if f.Source == "read_file" || f.Source == "grep" || f.Source == "repo_map" {
			t.Errorf("RepoFact.Source = %q — still contains tool name instead of logical path", f.Source)
		}
	}
	// Verify EvidenceRef preserved for tool-backed facts.
	evidenceCount := 0
	for _, f := range busCtx.RepoFacts {
		if f.EvidenceRef != "" {
			evidenceCount++
		}
	}
	if evidenceCount != 3 {
		t.Errorf("facts with EvidenceRef = %d, want 3", evidenceCount)
	}

	// --- Assertion 2: Finalizer received real file paths ---
	if len(finalizerRelevantFiles) == 0 {
		t.Fatal("finalizer received empty RelevantFiles")
	}
	for _, f := range finalizerRelevantFiles {
		if f == "read_file" || f == "grep" || f == "repo_map" {
			t.Errorf("finalizer RelevantFiles contains tool name %q instead of file path", f)
		}
	}

	// --- Assertion 3: Deduplication by logical path ---
	// Two facts have Source="internal/agent/explorer.go" — should be deduped to 1.
	// Total unique: explorer.go, builder.go, tool:repo_map = 3 entries.
	if len(finalizerRelevantFiles) != 3 {
		t.Errorf("finalizer RelevantFiles count = %d, want 3 (dedup by logical source); got %v",
			len(finalizerRelevantFiles), finalizerRelevantFiles)
	}
	expectedFiles := map[string]bool{
		"internal/agent/explorer.go":   false,
		"internal/context/builder.go":  false,
		"tool:repo_map":               false,
	}
	for _, f := range finalizerRelevantFiles {
		if _, ok := expectedFiles[f]; ok {
			expectedFiles[f] = true
		} else {
			t.Errorf("unexpected file in RelevantFiles: %q", f)
		}
	}
	for path, found := range expectedFiles {
		if !found {
			t.Errorf("expected file %q not found in RelevantFiles", path)
		}
	}

	// --- Assertion 4: RelevantFacts contain source annotations with paths ---
	for _, fact := range finalizerRelevantFacts {
		// Each formatted fact looks like: [key] key = value (source: PATH, confidence: N.NN)
		if strings.Contains(fact, "source: read_file") || strings.Contains(fact, "source: grep") {
			t.Errorf("formatted fact still references tool name as source: %s", fact)
		}
	}

	// --- Assertion 5: BuildPromptContext renders paths ---
	// Reconstruct the prompt the finalizer would get.
	finalizerAC := &types.AgentContext{
		AgentName:     types.AgentFinalizer,
		Stage:         types.StageFinalize,
		Objective:     "explain how the explorer works",
		RelevantFacts: finalizerRelevantFacts,
		RelevantFiles: finalizerRelevantFiles,
	}
	sk := &skill.Config{
		Name:         "finalize-skill",
		Goal:         "Compile final answer",
		Workflow:     []string{"summarize findings"},
		OutputFormat: "plain text",
	}
	pc := context.BuildPromptContext(finalizerAC, sk)

	// Find the "Relevant Files" user section.
	var filesSection string
	for _, s := range pc.UserSections {
		if s.Title == "Relevant Files" {
			filesSection = s.Content
			break
		}
	}
	if filesSection == "" {
		t.Fatal("BuildPromptContext did not produce a 'Relevant Files' section")
	}
	if strings.Contains(filesSection, "read_file") || strings.Contains(filesSection, "\ngrep\n") {
		t.Errorf("Relevant Files section contains tool names: %s", filesSection)
	}
	if !strings.Contains(filesSection, "internal/agent/explorer.go") {
		t.Errorf("Relevant Files section missing expected path: %s", filesSection)
	}
	if !strings.Contains(filesSection, "internal/context/builder.go") {
		t.Errorf("Relevant Files section missing expected path: %s", filesSection)
	}
}

// TestRun_OldBehaviorWouldFail demonstrates that if explorer still filled
// Source with tool names (the pre-fix behavior), the downstream assertions
// would catch it. This is a regression guard: if logicalFactSource is ever
// broken or bypassed, this test exposes the damage.
func TestRun_OldBehaviorWouldFail(t *testing.T) {
	cfg := defaultResolvedConfig()

	var finalizerRelevantFiles []string

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			ctx.Mutable.SetTaskList(types.TaskList{
				Objective: "explain it",
				Tasks: []types.TaskItem{{
					ID: "t1", Title: "explain", Status: types.TaskPending,
				}},
				CurrentTaskID: "t1",
			})
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		// Simulate the OLD broken behavior: Source = tool name.
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece:  types.MissingNone,
				SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				NewFacts: []types.RepoFact{
					{
						Key:        "read_file",
						Value:      "file contents...",
						Source:     "read_file", // OLD: tool name, not path
						Confidence: 0.8,
					},
					{
						Key:        "grep",
						Value:      "match results...",
						Source:     "grep", // OLD: tool name, not path
						Confidence: 0.8,
					},
				},
			}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizerRelevantFiles = make([]string, len(ctx.RelevantFiles))
			copy(finalizerRelevantFiles, ctx.RelevantFiles)
			return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "ok"}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(cfg, ar, sr, sar)
	o.SetMaxSteps(20)

	if _, err := o.Run("explain it", "/tmp/repo", "main"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// With the old behavior, RelevantFiles would be ["read_file", "grep"] —
	// clearly wrong. Assert that these tool-name entries are indeed garbage.
	hasToolName := false
	for _, f := range finalizerRelevantFiles {
		if f == "read_file" || f == "grep" {
			hasToolName = true
			break
		}
	}
	if !hasToolName {
		t.Fatal("expected old-behavior simulation to produce tool names in RelevantFiles, but it didn't — test setup may be wrong")
	}

	// Now verify these are NOT valid file paths (they lack "/" or "." extension).
	for _, f := range finalizerRelevantFiles {
		if strings.Contains(f, "/") || strings.HasSuffix(f, ".go") {
			t.Errorf("old-behavior fact %q unexpectedly looks like a real path", f)
		}
	}
}

// TestRun_OscillationGuardTripsBeforeMaxSteps verifies that when a
// stage re-enters itself repeatedly without making progress, the
// per-stage visit counter trips well before max-steps would, leading
// to a forced finalize with a stuck-error message. The default
// config has an explore → explore self-loop transition; combined
// with an analysis-typed task and an explorer that never reports
// HasEnoughFacts, the orchestrator naturally bounces inside explore.
func TestRun_OscillationGuardTripsBeforeMaxSteps(t *testing.T) {
	cfg := defaultResolvedConfig()

	finalizerCalled := 0
	exploreCalled := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			ctx.Mutable.SetTaskList(types.TaskList{
				Objective: "explain it",
				Tasks: []types.TaskItem{{
					ID: "t1", Title: "explain", Status: types.TaskPending,
				}},
				CurrentTaskID: "t1",
			})
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		// explorer never reports HasEnoughFacts, so the explore → explore
		// self-loop keeps firing until the guard trips.
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			exploreCalled++
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizerCalled++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "stuck — forced finalize",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(cfg, ar, sr, sar)
	// Generous max-steps so this test fails clearly if the guard is
	// not the thing that breaks the loop.
	o.SetMaxSteps(50)

	busCtx, err := o.Run("explain something", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if !busCtx.TaskState.IsTerminal {
		t.Error("expected pipeline to terminate via forced finalize")
	}
	if finalizerCalled != 1 {
		t.Errorf("finalizer call count = %d, want exactly 1", finalizerCalled)
	}
	tl := busCtx.Mutable.TaskList()
	if len(tl.Tasks) == 0 {
		t.Fatal("expected at least one task on the list")
	}
	if got := tl.Tasks[0].Result; got != "stuck — forced finalize" {
		t.Errorf("task.Result = %q, want forced summary", got)
	}
	if busCtx.TaskState.LastError == "" {
		t.Error("expected LastError to record oscillation")
	}
	// Explorer should have run at most maxStageVisits times before
	// the guard tripped — definitely far fewer than the 50-step cap.
	if exploreCalled > 10 {
		t.Errorf("expected guard to fire fast; explorer ran %d times", exploreCalled)
	}
}
