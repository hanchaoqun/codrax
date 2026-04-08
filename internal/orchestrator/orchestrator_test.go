package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/design/internal/agent"
	"github.com/hanchaoqun/design/internal/config"
	"github.com/hanchaoqun/design/internal/skill"
	"github.com/hanchaoqun/design/internal/types"
)

// ---------------------------------------------------------------------------
// Mock agent
// ---------------------------------------------------------------------------

type mockAgent struct {
	name    types.AgentName
	execFn  func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error)
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
// Stages: analyze -> explore -> plan -> implement -> review -> verify -> finalize
func defaultResolvedConfig() *config.ResolvedConfig {
	return &config.ResolvedConfig{
		Stages: map[types.PipelineStage]*types.StageConfig{
			types.StageAnalyze: {
				Name: types.StageAnalyze, DefaultAgent: types.AgentPlanner,
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
			types.StageImplement: {
				Name: types.StageImplement, DefaultAgent: types.AgentImplementer,
				DefaultSkill: "implement-skill", RequiresWrite: true,
			},
			types.StageReview: {
				Name: types.StageReview, DefaultAgent: types.AgentReviewer,
				DefaultSkill: "design-review-skill",
			},
			types.StageVerify: {
				Name: types.StageVerify, DefaultAgent: types.AgentVerifier,
				DefaultSkill: "verify-skill",
			},
			types.StageFinalize: {
				Name: types.StageFinalize, DefaultAgent: types.AgentFinalizer,
				DefaultSkill: "finalize-skill", Terminal: true,
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
				{From: types.StagePlan, To: types.StageImplement, Priority: 100},
				{From: types.StagePlan, To: types.StageReview, Priority: 90},
				{From: types.StagePlan, To: types.StageFinalize, Priority: 10},
			},
			types.StageImplement: {
				{From: types.StageImplement, To: types.StageReview, Priority: 100},
				{From: types.StageImplement, To: types.StageVerify, Priority: 90},
				{From: types.StageImplement, To: types.StageFinalize, Priority: 10},
			},
			types.StageReview: {
				{From: types.StageReview, To: types.StageVerify, Priority: 100},
				{From: types.StageReview, To: types.StageFinalize, Priority: 10},
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
					types.StageImplement, types.StageReview, types.StageVerify,
					types.StageFinalize,
				},
			},
			"analysis": {
				Name: "analysis",
				AllowedStages: []types.PipelineStage{
					types.StageAnalyze, types.StageExplore, types.StageFinalize,
				},
			},
		},
		FeatureFlags: types.FeatureFlags{
			EnableReview: true,
			EnableVerify: true,
		},
		Agents: map[types.AgentName]*types.AgentConfig{},
		Skills: map[string]*types.SkillConfigYAML{},
	}
}

// buildRegistries creates agent and skill registries with mock entries matching
// the default config. agentFns lets callers override the Execute function per agent.
func buildRegistries(agentFns map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error)) (*agent.Registry, *skill.Registry) {
	ar := agent.NewRegistry()
	names := []types.AgentName{
		types.AgentPlanner, types.AgentExplorer, types.AgentImplementer,
		types.AgentReviewer, types.AgentVerifier, types.AgentFinalizer,
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
	}
	for _, s := range skillNames {
		sr.Register(&skill.Config{Name: s, Goal: s + " goal"})
	}

	return ar, sr
}

// ---------------------------------------------------------------------------
// decideNextStage tests
// ---------------------------------------------------------------------------

func TestDecideNextStage_AnalyzeToExplore(t *testing.T) {
	t.Run("after analyze with MissingFacts should transition to explore", func(t *testing.T) {
		cfg := defaultResolvedConfig()
		ar, sr := buildRegistries(nil)
		o := New(cfg, ar, sr)

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
		ar, sr := buildRegistries(nil)
		o := New(cfg, ar, sr)

		o.busCtx = &types.BusContext{
			PipelineStage: types.StageExplore,
			Signals: types.ExecutionSignals{
				HasEnoughFacts: true,
			},
			TaskState: types.TaskState{
				Stage:   types.StageExplore,
				Missing: types.MissingPlan,
			},
			Policy: types.PolicyContext{},
		}

		next := o.decideNextStage()
		if next != types.StagePlan {
			t.Errorf("expected plan, got %s", next)
		}
	})
}

func TestDecideNextStage_PolicyFiltering(t *testing.T) {
	t.Run("analysis policy filters out plan and implement so explore goes to finalize", func(t *testing.T) {
		cfg := defaultResolvedConfig()
		ar, sr := buildRegistries(nil)
		o := New(cfg, ar, sr)

		// Set up an analysis task so the "analysis" policy is active.
		o.busCtx = &types.BusContext{
			PipelineStage: types.StageExplore,
			TaskList: types.TaskList{
				Objective:     "analyze something",
				CurrentTaskID: "t1",
				Tasks: []types.TaskItem{
					{ID: "t1", Title: "analysis task", Type: types.TaskTypeAnalysis, Status: types.TaskInProgress},
				},
			},
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

func TestDecideNextStage_FeatureFlagDisablesReview(t *testing.T) {
	t.Run("enable_review=false filters review transitions", func(t *testing.T) {
		cfg := defaultResolvedConfig()
		cfg.FeatureFlags.EnableReview = false
		ar, sr := buildRegistries(nil)
		o := New(cfg, ar, sr)

		// After implement, the highest priority transition is review (100),
		// but with review disabled, it should go to verify (90).
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
			Policy: types.PolicyContext{},
		}

		next := o.decideNextStage()
		if next != types.StageVerify {
			t.Errorf("expected verify (review disabled), got %s", next)
		}
	})
}

func TestDecideNextStage_FallbackToFinalize(t *testing.T) {
	t.Run("when no valid transitions remain, should fall back to finalize", func(t *testing.T) {
		cfg := defaultResolvedConfig()
		// Remove all transitions from verify stage
		cfg.Transitions[types.StageVerify] = nil
		ar, sr := buildRegistries(nil)
		o := New(cfg, ar, sr)

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
// resolveSkillName tests
// ---------------------------------------------------------------------------

func TestResolveSkillName_ReviewDualRole(t *testing.T) {
	cfg := defaultResolvedConfig()
	ar, sr := buildRegistries(nil)
	o := New(cfg, ar, sr)

	reviewStageConfig := cfg.Stages[types.StageReview]

	t.Run("after plan completion review uses design-review-skill", func(t *testing.T) {
		o.busCtx = &types.BusContext{
			TaskState: types.TaskState{
				Completed: []string{string(types.StageAnalyze), string(types.StageExplore), string(types.StagePlan)},
			},
		}
		got := o.resolveSkillName(reviewStageConfig)
		if got != "design-review-skill" {
			t.Errorf("expected design-review-skill, got %s", got)
		}
	})

	t.Run("after implement completion review uses code-review-skill", func(t *testing.T) {
		o.busCtx = &types.BusContext{
			TaskState: types.TaskState{
				Completed: []string{
					string(types.StageAnalyze), string(types.StageExplore),
					string(types.StagePlan), string(types.StageImplement),
				},
			},
		}
		got := o.resolveSkillName(reviewStageConfig)
		if got != "code-review-skill" {
			t.Errorf("expected code-review-skill, got %s", got)
		}
	})
}

// ---------------------------------------------------------------------------
// Full pipeline test
// ---------------------------------------------------------------------------

func TestRun_SimplePipeline(t *testing.T) {
	t.Run("pipeline progresses through analyze->explore->plan->implement->verify->finalize", func(t *testing.T) {
		cfg := defaultResolvedConfig()
		// Disable review so the pipeline goes straight implement -> verify.
		cfg.FeatureFlags.EnableReview = false

		agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
			// analyze: reports MissingFacts so orchestrator transitions to explore
			types.AgentPlanner: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				switch ctx.Stage {
				case types.StageAnalyze:
					return &agent.StageOutput{
						MissingPiece: types.MissingFacts,
					}, nil
				case types.StagePlan:
					return &agent.StageOutput{
						MissingPiece: types.MissingCode,
						SignalUpdates: &types.ExecutionSignals{
							HasPlan: true,
						},
					}, nil
				}
				return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
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
			// reviewer: not used in this test (review disabled)
			types.AgentReviewer: nil,
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

		ar, sr := buildRegistries(agentFns)
		o := New(cfg, ar, sr)
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
