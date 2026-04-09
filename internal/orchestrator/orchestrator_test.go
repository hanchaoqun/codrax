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

// implementationTaskList builds a TaskList that classifies the user
// request as an implementation task. Used by tests whose analyzer mock
// needs to drive the orchestrator into the implementation policy now
// that the default fallback is analysis.
func implementationTaskList() *types.TaskList {
	return &types.TaskList{
		Objective: "implement a widget",
		Tasks: []types.TaskItem{{
			ID:     "t1",
			Title:  "implement",
			Type:   types.TaskTypeImplementation,
			Status: types.TaskPending,
		}},
		CurrentTaskID: "t1",
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
			Policy:   types.PolicyContext{},
			TaskList: *implementationTaskList(),
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
			Policy:   types.PolicyContext{RequireReview: true},
			TaskList: *implementationTaskList(),
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
			Policy:   types.PolicyContext{RequireReview: true},
			TaskList: *implementationTaskList(),
		}

		next := o.decideNextStage()
		if next != types.StageCodeReview {
			t.Errorf("expected code_review, got %s", next)
		}
	})
}

func TestDecideNextStage_PolicyFiltering(t *testing.T) {
	t.Run("analysis policy filters out plan and implement so explore goes to finalize", func(t *testing.T) {
		cfg := defaultResolvedConfig()
		ar, sr, sar := buildRegistries(nil)
		o := New(cfg, ar, sr, sar)

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
			// analyze: classifies the request as implementation and reports
			// MissingFacts so orchestrator transitions to explore.
			types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
				return &agent.StageOutput{
					MissingPiece:   types.MissingFacts,
					TaskListUpdate: implementationTaskList(),
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
				return &agent.StageOutput{
					MissingPiece:   types.MissingFacts,
					TaskListUpdate: implementationTaskList(),
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
			TaskList:      types.TaskList{Objective: "implement a risky feature"},
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

			if err := o.executeStage(stageConfig); err != nil {
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

// TestRun_AnalysisPolicyFromTaskListUpdate verifies the wiring added in
// commit 2: an analyzer that returns StageOutput.TaskListUpdate with a
// task of type Analysis must drive the orchestrator into the analysis
// policy, so the pipeline avoids the implement / verify stages.
func TestRun_AnalysisPolicyFromTaskListUpdate(t *testing.T) {
	cfg := defaultResolvedConfig()

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				TaskListUpdate: &types.TaskList{
					Objective: "explain the project",
					Tasks: []types.TaskItem{{
						ID: "t1", Title: "explain", Type: types.TaskTypeAnalysis, Status: types.TaskPending,
					}},
					CurrentTaskID: "t1",
				},
			}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece:  types.MissingNone,
				SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
			}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
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

	// busCtx.TaskList must reflect the analyzer's update.
	current := busCtx.TaskList.CurrentTask()
	if current == nil {
		t.Fatal("expected BusContext.TaskList to be populated from TaskListUpdate")
	}
	if current.Type != types.TaskTypeAnalysis {
		t.Errorf("current task type = %s, want analysis", current.Type)
	}

	// Pipeline must NOT visit implement / verify under analysis policy.
	for _, s := range busCtx.TaskState.Completed {
		stage := types.PipelineStage(s)
		if stage == types.StageImplement || stage == types.StageVerify {
			t.Errorf("analysis pipeline should not reach %s, completed = %v", stage, busCtx.TaskState.Completed)
		}
	}
}
