package config

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hanchaoqun/design/internal/types"
)

// minimalYAML is a focused config for unit tests that exercises all parsed
// fields without depending on the real orchestrator.yaml.
const minimalYAML = `
stages:
  analyze:
    default_agent: planner
    default_skill: task-analysis-skill
    terminal: false
  implement:
    default_agent: implementer
    default_skill: code-implement-skill
    terminal: false
    requires_write: true
  finalize:
    default_agent: finalizer
    default_skill: final-answer-skill
    terminal: true

transitions:
  analyze:
    - to: implement
      priority: 100
    - to: finalize
      priority: 20
  implement:
    - to: finalize
      priority: 100

task_policies:
  analysis:
    allowed_stages:
      - analyze
      - finalize
  implementation:
    allowed_stages:
      - analyze
      - implement
      - finalize

feature_flags:
  enable_review: false
  enable_verify: true
  allow_skip_plan_for_small_change: true

agents:
  - name: planner
    stages: [analyze]
  - name: implementer
    stages: [implement]
    requires_write: true
  - name: finalizer
    stages: [finalize]

skills:
  - name: task-analysis-skill
    description: Analyze the task
  - name: code-implement-skill
    description: Write code
  - name: final-answer-skill
    description: Produce final answer
`

func TestParse(t *testing.T) {
	cfg, err := Parse([]byte(minimalYAML))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	t.Run("stages count", func(t *testing.T) {
		if got := len(cfg.Stages); got != 3 {
			t.Fatalf("expected 3 stages, got %d", got)
		}
	})

	t.Run("analyze stage fields", func(t *testing.T) {
		s, ok := cfg.Stages["analyze"]
		if !ok {
			t.Fatal("stage 'analyze' not found")
		}
		if s.DefaultAgent != "planner" {
			t.Errorf("analyze.default_agent = %q, want %q", s.DefaultAgent, "planner")
		}
		if s.DefaultSkill != "task-analysis-skill" {
			t.Errorf("analyze.default_skill = %q, want %q", s.DefaultSkill, "task-analysis-skill")
		}
		if s.Terminal {
			t.Error("analyze.terminal should be false")
		}
		if s.RequiresWrite {
			t.Error("analyze.requires_write should be false")
		}
	})

	t.Run("implement requires_write", func(t *testing.T) {
		s := cfg.Stages["implement"]
		if !s.RequiresWrite {
			t.Error("implement.requires_write should be true")
		}
	})

	t.Run("finalize is terminal", func(t *testing.T) {
		s := cfg.Stages["finalize"]
		if !s.Terminal {
			t.Error("finalize.terminal should be true")
		}
	})

	t.Run("transitions count", func(t *testing.T) {
		if got := len(cfg.Transitions["analyze"]); got != 2 {
			t.Fatalf("expected 2 transitions from analyze, got %d", got)
		}
		if got := len(cfg.Transitions["implement"]); got != 1 {
			t.Fatalf("expected 1 transition from implement, got %d", got)
		}
	})

	t.Run("transition fields", func(t *testing.T) {
		tr := cfg.Transitions["analyze"][0]
		if tr.To != "implement" {
			t.Errorf("first analyze transition.to = %q, want %q", tr.To, "implement")
		}
		if tr.Priority != 100 {
			t.Errorf("first analyze transition.priority = %d, want 100", tr.Priority)
		}
	})

	t.Run("task policies", func(t *testing.T) {
		if got := len(cfg.TaskPolicies); got != 2 {
			t.Fatalf("expected 2 task policies, got %d", got)
		}
		p := cfg.TaskPolicies["analysis"]
		if got := len(p.AllowedStages); got != 2 {
			t.Fatalf("analysis policy: expected 2 allowed stages, got %d", got)
		}
	})

	t.Run("feature flags", func(t *testing.T) {
		ff := cfg.FeatureFlags
		if ff.EnableReview {
			t.Error("enable_review should be false")
		}
		if !ff.EnableVerify {
			t.Error("enable_verify should be true")
		}
		if !ff.AllowSkipPlanForSmallChange {
			t.Error("allow_skip_plan_for_small_change should be true")
		}
	})

	t.Run("agents", func(t *testing.T) {
		if got := len(cfg.Agents); got != 3 {
			t.Fatalf("expected 3 agents, got %d", got)
		}
		if cfg.Agents[1].Name != "implementer" {
			t.Errorf("agents[1].name = %q, want %q", cfg.Agents[1].Name, "implementer")
		}
		if !cfg.Agents[1].RequiresWrite {
			t.Error("implementer agent should require write")
		}
	})

	t.Run("skills", func(t *testing.T) {
		if got := len(cfg.Skills); got != 3 {
			t.Fatalf("expected 3 skills, got %d", got)
		}
		if cfg.Skills[0].Name != "task-analysis-skill" {
			t.Errorf("skills[0].name = %q, want %q", cfg.Skills[0].Name, "task-analysis-skill")
		}
	})

	t.Run("invalid YAML returns error", func(t *testing.T) {
		_, err := Parse([]byte("not: [valid: yaml"))
		if err == nil {
			t.Fatal("expected error for invalid YAML, got nil")
		}
	})
}

func TestResolve(t *testing.T) {
	cfg, err := Parse([]byte(minimalYAML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rc, err := Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	t.Run("stages map populated", func(t *testing.T) {
		if got := len(rc.Stages); got != 3 {
			t.Fatalf("expected 3 stages, got %d", got)
		}
	})

	t.Run("stage has typed PipelineStage name", func(t *testing.T) {
		sc := rc.Stages[types.StageAnalyze]
		if sc == nil {
			t.Fatal("stage analyze not found in resolved config")
		}
		if sc.Name != types.StageAnalyze {
			t.Errorf("stage name = %q, want %q", sc.Name, types.StageAnalyze)
		}
	})

	t.Run("stage has typed AgentName", func(t *testing.T) {
		sc := rc.Stages[types.StageAnalyze]
		if sc.DefaultAgent != types.AgentPlanner {
			t.Errorf("default_agent = %q, want %q", sc.DefaultAgent, types.AgentPlanner)
		}
	})

	t.Run("stage default_skill preserved", func(t *testing.T) {
		sc := rc.Stages[types.StageAnalyze]
		if sc.DefaultSkill != "task-analysis-skill" {
			t.Errorf("default_skill = %q, want %q", sc.DefaultSkill, "task-analysis-skill")
		}
	})

	t.Run("transitions From field populated", func(t *testing.T) {
		ts := rc.Transitions[types.StageAnalyze]
		if got := len(ts); got != 2 {
			t.Fatalf("expected 2 transitions from analyze, got %d", got)
		}
		for i, tr := range ts {
			if tr.From != types.StageAnalyze {
				t.Errorf("transition[%d].From = %q, want %q", i, tr.From, types.StageAnalyze)
			}
		}
	})

	t.Run("transitions To field is typed", func(t *testing.T) {
		ts := rc.Transitions[types.StageAnalyze]
		if ts[0].To != types.StageImplement {
			t.Errorf("transition[0].To = %q, want %q", ts[0].To, types.StageImplement)
		}
	})

	t.Run("task policies resolved with Name", func(t *testing.T) {
		if got := len(rc.TaskPolicies); got != 2 {
			t.Fatalf("expected 2 task policies, got %d", got)
		}
		p := rc.TaskPolicies["implementation"]
		if p == nil {
			t.Fatal("implementation policy not found")
		}
		if p.Name != "implementation" {
			t.Errorf("policy.Name = %q, want %q", p.Name, "implementation")
		}
		if got := len(p.AllowedStages); got != 3 {
			t.Fatalf("expected 3 allowed stages, got %d", got)
		}
	})

	t.Run("feature flags preserved", func(t *testing.T) {
		if rc.FeatureFlags.EnableReview {
			t.Error("enable_review should be false")
		}
		if !rc.FeatureFlags.EnableVerify {
			t.Error("enable_verify should be true")
		}
		if !rc.FeatureFlags.AllowSkipPlanForSmallChange {
			t.Error("allow_skip_plan_for_small_change should be true")
		}
	})

	t.Run("agents resolved by name key", func(t *testing.T) {
		if got := len(rc.Agents); got != 3 {
			t.Fatalf("expected 3 agents, got %d", got)
		}
		impl := rc.Agents[types.AgentImplementer]
		if impl == nil {
			t.Fatal("implementer agent not found")
		}
		if !impl.RequiresWrite {
			t.Error("implementer agent should require write")
		}
	})

	t.Run("skills resolved by name key", func(t *testing.T) {
		if got := len(rc.Skills); got != 3 {
			t.Fatalf("expected 3 skills, got %d", got)
		}
		s := rc.Skills["task-analysis-skill"]
		if s == nil {
			t.Fatal("task-analysis-skill not found")
		}
		if s.Description != "Analyze the task" {
			t.Errorf("description = %q, want %q", s.Description, "Analyze the task")
		}
	})
}

func TestGetTransitions(t *testing.T) {
	cfg, err := Parse([]byte(minimalYAML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rc, err := Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	t.Run("sorted by priority descending", func(t *testing.T) {
		ts := rc.GetTransitions(types.StageAnalyze)
		if got := len(ts); got != 2 {
			t.Fatalf("expected 2 transitions, got %d", got)
		}
		if ts[0].Priority < ts[1].Priority {
			t.Errorf("transitions not sorted descending: priorities %d, %d", ts[0].Priority, ts[1].Priority)
		}
		if ts[0].Priority != 100 {
			t.Errorf("first transition priority = %d, want 100", ts[0].Priority)
		}
		if ts[1].Priority != 20 {
			t.Errorf("second transition priority = %d, want 20", ts[1].Priority)
		}
	})

	t.Run("correct destinations after sort", func(t *testing.T) {
		ts := rc.GetTransitions(types.StageAnalyze)
		if ts[0].To != types.StageImplement {
			t.Errorf("first transition.To = %q, want %q", ts[0].To, types.StageImplement)
		}
		if ts[1].To != types.StageFinalize {
			t.Errorf("second transition.To = %q, want %q", ts[1].To, types.StageFinalize)
		}
	})

	t.Run("From field populated in returned transitions", func(t *testing.T) {
		ts := rc.GetTransitions(types.StageAnalyze)
		for i, tr := range ts {
			if tr.From != types.StageAnalyze {
				t.Errorf("transition[%d].From = %q, want %q", i, tr.From, types.StageAnalyze)
			}
		}
	})

	t.Run("unknown stage returns empty slice", func(t *testing.T) {
		ts := rc.GetTransitions(types.PipelineStage("nonexistent"))
		if got := len(ts); got != 0 {
			t.Errorf("expected 0 transitions for unknown stage, got %d", got)
		}
	})

	t.Run("does not mutate original order", func(t *testing.T) {
		original := rc.Transitions[types.StageAnalyze]
		origTo := make([]types.PipelineStage, len(original))
		for i, tr := range original {
			origTo[i] = tr.To
		}

		_ = rc.GetTransitions(types.StageAnalyze)

		for i, tr := range rc.Transitions[types.StageAnalyze] {
			if tr.To != origTo[i] {
				t.Fatal("GetTransitions mutated the original transitions slice")
			}
		}
	})
}

func TestGetStageConfig(t *testing.T) {
	cfg, err := Parse([]byte(minimalYAML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rc, err := Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	t.Run("known stage returns config", func(t *testing.T) {
		sc, err := rc.GetStageConfig(types.StageImplement)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sc.Name != types.StageImplement {
			t.Errorf("name = %q, want %q", sc.Name, types.StageImplement)
		}
		if !sc.RequiresWrite {
			t.Error("implement should require write")
		}
		if sc.Terminal {
			t.Error("implement should not be terminal")
		}
	})

	t.Run("terminal stage", func(t *testing.T) {
		sc, err := rc.GetStageConfig(types.StageFinalize)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !sc.Terminal {
			t.Error("finalize should be terminal")
		}
	})

	t.Run("unknown stage returns error", func(t *testing.T) {
		_, err := rc.GetStageConfig(types.PipelineStage("bogus"))
		if err == nil {
			t.Fatal("expected error for unknown stage, got nil")
		}
	})
}

func TestGetTaskPolicy(t *testing.T) {
	cfg, err := Parse([]byte(minimalYAML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rc, err := Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	t.Run("known policy", func(t *testing.T) {
		p, err := rc.GetTaskPolicy("analysis")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Name != "analysis" {
			t.Errorf("name = %q, want %q", p.Name, "analysis")
		}
		if got := len(p.AllowedStages); got != 2 {
			t.Fatalf("expected 2 allowed stages, got %d", got)
		}
		found := map[types.PipelineStage]bool{}
		for _, s := range p.AllowedStages {
			found[s] = true
		}
		if !found[types.StageAnalyze] {
			t.Error("analysis policy should include analyze stage")
		}
		if !found[types.StageFinalize] {
			t.Error("analysis policy should include finalize stage")
		}
	})

	t.Run("implementation policy stages", func(t *testing.T) {
		p, err := rc.GetTaskPolicy("implementation")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := len(p.AllowedStages); got != 3 {
			t.Fatalf("expected 3 allowed stages, got %d", got)
		}
	})

	t.Run("unknown policy returns error", func(t *testing.T) {
		_, err := rc.GetTaskPolicy("nonexistent")
		if err == nil {
			t.Fatal("expected error for unknown policy, got nil")
		}
	})
}

// projectRoot returns the absolute path to the project root by walking up from
// this test file's directory (internal/config/ -> project root).
func projectRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to determine test file path")
	}
	return filepath.Join(filepath.Dir(filename), "..", "..")
}

func TestLoadAndResolve(t *testing.T) {
	root := projectRoot(t)
	configPath := filepath.Join(root, "config", "orchestrator.yaml")

	rc, err := LoadAndResolve(configPath)
	if err != nil {
		t.Fatalf("LoadAndResolve(%q): %v", configPath, err)
	}

	t.Run("7 stages loaded", func(t *testing.T) {
		if got := len(rc.Stages); got != 7 {
			t.Fatalf("expected 7 stages, got %d", got)
		}
		for _, s := range types.AllStages() {
			if _, ok := rc.Stages[s]; !ok {
				t.Errorf("missing stage: %s", s)
			}
		}
	})

	t.Run("finalize is terminal", func(t *testing.T) {
		sc, err := rc.GetStageConfig(types.StageFinalize)
		if err != nil {
			t.Fatalf("GetStageConfig(finalize): %v", err)
		}
		if !sc.Terminal {
			t.Error("finalize should be terminal")
		}
	})

	t.Run("non-terminal stages are not terminal", func(t *testing.T) {
		for _, s := range []types.PipelineStage{
			types.StageAnalyze, types.StageExplore, types.StagePlan,
			types.StageReview, types.StageImplement, types.StageVerify,
		} {
			sc, err := rc.GetStageConfig(s)
			if err != nil {
				t.Fatalf("GetStageConfig(%s): %v", s, err)
			}
			if sc.Terminal {
				t.Errorf("stage %s should not be terminal", s)
			}
		}
	})

	t.Run("implement requires_write", func(t *testing.T) {
		sc, err := rc.GetStageConfig(types.StageImplement)
		if err != nil {
			t.Fatalf("GetStageConfig(implement): %v", err)
		}
		if !sc.RequiresWrite {
			t.Error("implement should require write")
		}
	})

	t.Run("analyze has 3 transitions sorted by priority", func(t *testing.T) {
		ts := rc.GetTransitions(types.StageAnalyze)
		if got := len(ts); got != 3 {
			t.Fatalf("expected 3 transitions from analyze, got %d", got)
		}
		expected := []struct {
			to       types.PipelineStage
			priority int
		}{
			{types.StageExplore, 100},
			{types.StagePlan, 80},
			{types.StageFinalize, 20},
		}
		for i, e := range expected {
			if ts[i].To != e.to {
				t.Errorf("transition[%d].To = %q, want %q", i, ts[i].To, e.to)
			}
			if ts[i].Priority != e.priority {
				t.Errorf("transition[%d].Priority = %d, want %d", i, ts[i].Priority, e.priority)
			}
			if ts[i].From != types.StageAnalyze {
				t.Errorf("transition[%d].From = %q, want %q", i, ts[i].From, types.StageAnalyze)
			}
		}
	})

	t.Run("3 task policies", func(t *testing.T) {
		if got := len(rc.TaskPolicies); got != 3 {
			t.Fatalf("expected 3 task policies, got %d", got)
		}
		for _, name := range []string{"analysis", "implementation", "high_risk_implementation"} {
			p, err := rc.GetTaskPolicy(name)
			if err != nil {
				t.Errorf("GetTaskPolicy(%q): %v", name, err)
				continue
			}
			if p.Name != name {
				t.Errorf("policy.Name = %q, want %q", p.Name, name)
			}
		}
	})

	t.Run("analysis policy has 3 allowed stages", func(t *testing.T) {
		p, _ := rc.GetTaskPolicy("analysis")
		if got := len(p.AllowedStages); got != 3 {
			t.Errorf("analysis policy: expected 3 allowed stages, got %d", got)
		}
	})

	t.Run("feature flags have correct defaults", func(t *testing.T) {
		ff := rc.FeatureFlags
		if !ff.EnableReview {
			t.Error("enable_review should be true")
		}
		if !ff.EnableVerify {
			t.Error("enable_verify should be true")
		}
		if ff.AllowSkipPlanForSmallChange {
			t.Error("allow_skip_plan_for_small_change should be false")
		}
	})

	t.Run("6 agents loaded", func(t *testing.T) {
		if got := len(rc.Agents); got != 6 {
			t.Fatalf("expected 6 agents, got %d", got)
		}
	})

	t.Run("9 skills loaded", func(t *testing.T) {
		if got := len(rc.Skills); got != 9 {
			t.Fatalf("expected 9 skills, got %d", got)
		}
	})
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/to/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
