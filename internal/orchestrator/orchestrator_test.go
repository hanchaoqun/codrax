package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/config"
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
			ID:      "t1",
			Title:   "implement",
			Writing: true,
			Status:  types.TaskPending,
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
		// analysis-final-answer-skill is registered so the dispatchStage
		// finalize-routing path can find it under analysis policy.
		// dispatchStage looks it up by name and falls back to the
		// stage's DefaultSkill if missing — see TestRun_FinalizeSkillRoutedByPolicy.
		"analysis-final-answer-skill",
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
			Mutable: types.NewMutableState(*implementationTaskList()),
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
			Mutable: types.NewMutableState(*implementationTaskList()),
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
			Mutable: types.NewMutableState(*implementationTaskList()),
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
			Mutable: types.NewMutableState(types.TaskList{
				Objective:     "analyze something",
				CurrentTaskID: "t1",
				Tasks: []types.TaskItem{
					{ID: "t1", Title: "analysis task", Writing: false, Status: types.TaskInProgress},
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
				return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
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
					ID: "t1", Title: "explain", Writing: false, Status: types.TaskPending,
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
	if current.Writing {
		t.Errorf("current task should be read-only (Writing=false), got Writing=true")
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
						ID: "t1", Title: "explain", Writing: false, Status: types.TaskPending,
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
				return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
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
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
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
func TestRun_MultiTaskExecution(t *testing.T) {
	cfg := defaultResolvedConfig()

	exploreCount := 0
	planCount := 0
	implementCount := 0
	verifyCount := 0
	finalizerCount := 0

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		// Analyzer plants three tasks: one read-only, two writing.
		types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			ctx.Mutable.SetTaskList(types.TaskList{
				Objective: "two-part request",
				Tasks: []types.TaskItem{
					{ID: "ta", Title: "explain X", Writing: false, Status: types.TaskPending},
					{ID: "tb", Title: "implement Y", Writing: true, Status: types.TaskPending},
					{ID: "tc", Title: "implement Z", Writing: true, Status: types.TaskPending},
				},
				CurrentTaskID: "ta",
			})
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
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

	// The read-only task (ta) should have skipped plan/implement/verify
	// because the analysis policy filters them out, while the two
	// writing tasks (tb, tc) should have visited all of them.
	if exploreCount != 3 {
		t.Errorf("explore count = %d, want 3 (once per task)", exploreCount)
	}
	if planCount != 2 {
		t.Errorf("plan count = %d, want 2 (writing tasks only)", planCount)
	}
	if implementCount != 2 {
		t.Errorf("implement count = %d, want 2 (writing tasks only)", implementCount)
	}
	if verifyCount != 2 {
		t.Errorf("verify count = %d, want 2 (writing tasks only)", verifyCount)
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
					ID: "t1", Title: "explain", Writing: false, Status: types.TaskPending,
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
