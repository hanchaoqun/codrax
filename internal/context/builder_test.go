package context

import (
	"testing"

	"github.com/hanchaoqun/design/internal/skill"
	"github.com/hanchaoqun/design/internal/types"
)

func TestBuildAgentContext(t *testing.T) {
	bus := &types.BusContext{
		PipelineStage: types.StageExplore,
		ActiveAgent:   types.AgentExplorer,
		RepoRoot:      "/tmp/repo",
		Branch:        "main",
		Commit:        "abc123",
		Mutable: types.NewMutableState(types.TaskList{
			Objective:     "Fix the bug",
			CurrentTaskID: "t1",
			Tasks: []types.TaskItem{
				{ID: "t1", Title: "Investigate root cause", Type: types.TaskTypeAnalysis, Status: types.TaskInProgress},
			},
		}),
		TaskState: types.TaskState{
			Stage:   types.StageExplore,
			Missing: types.MissingFacts,
		},
		RepoFacts: []types.RepoFact{
			{Key: "entrypoint", Value: "cmd/main.go", Source: "repo_map", Confidence: 0.9},
		},
		ToolResults: []types.ToolResult{
			{ToolName: "grep", Summary: "Found 3 matches", Success: true},
		},
		Constraints: []string{"read-only"},
	}

	ac := BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)

	t.Run("basic fields", func(t *testing.T) {
		if ac.AgentName != types.AgentExplorer {
			t.Errorf("got agent %s, want %s", ac.AgentName, types.AgentExplorer)
		}
		if ac.Stage != types.StageExplore {
			t.Errorf("got stage %s, want %s", ac.Stage, types.StageExplore)
		}
		if ac.Objective != "Fix the bug" {
			t.Errorf("got objective %q, want %q", ac.Objective, "Fix the bug")
		}
		if ac.RepoRoot != "/tmp/repo" {
			t.Errorf("got repo %q, want %q", ac.RepoRoot, "/tmp/repo")
		}
	})

	t.Run("current task", func(t *testing.T) {
		if ac.CurrentTaskID != "t1" {
			t.Errorf("got task ID %q, want %q", ac.CurrentTaskID, "t1")
		}
		if ac.CurrentTask != "Investigate root cause" {
			t.Errorf("got task %q, want %q", ac.CurrentTask, "Investigate root cause")
		}
	})

	t.Run("facts extracted", func(t *testing.T) {
		if len(ac.RelevantFacts) != 1 {
			t.Fatalf("got %d facts, want 1", len(ac.RelevantFacts))
		}
	})

	t.Run("tool summaries extracted", func(t *testing.T) {
		if len(ac.RelevantToolSummaries) != 1 {
			t.Fatalf("got %d tool summaries, want 1", len(ac.RelevantToolSummaries))
		}
	})

	t.Run("missing piece", func(t *testing.T) {
		if ac.MissingPiece != types.MissingFacts {
			t.Errorf("got missing %s, want %s", ac.MissingPiece, types.MissingFacts)
		}
	})
}

func TestBuildPromptContext(t *testing.T) {
	ac := &types.AgentContext{
		AgentName:     types.AgentPlanner,
		Stage:         types.StageAnalyze,
		Objective:     "Add logging",
		CurrentTaskID: "t1",
		CurrentTask:   "Analyze requirements",
		MissingPiece:  types.MissingUnderstanding,
		Constraints:   []string{"must be backwards compatible"},
		RelevantFacts: []string{"uses logrus"},
	}

	sk := &skill.Config{
		Name:            "task-analysis-skill",
		Goal:            "Structurize and classify the user task",
		Workflow:        []string{"read input", "identify intent", "classify type"},
		ToolSuggestions: []string{"grep", "read_file"},
		OutputFormat:    "JSON with task_type, objective",
		Prohibitions:    []string{"do not implement"},
	}

	pc := BuildPromptContext(ac, sk)

	t.Run("metadata", func(t *testing.T) {
		if pc.AgentName != types.AgentPlanner {
			t.Errorf("got agent %s, want %s", pc.AgentName, types.AgentPlanner)
		}
		if pc.SkillName != "task-analysis-skill" {
			t.Errorf("got skill %q, want %q", pc.SkillName, "task-analysis-skill")
		}
	})

	t.Run("system sections", func(t *testing.T) {
		if len(pc.SystemSections) < 2 {
			t.Fatalf("got %d system sections, want >= 2", len(pc.SystemSections))
		}
	})

	t.Run("system sections include workflow", func(t *testing.T) {
		found := false
		for _, s := range pc.SystemSections {
			if s.Title == "Workflow" {
				found = true
				break
			}
		}
		if !found {
			t.Error("system sections missing Workflow")
		}
	})

	t.Run("system sections include prohibitions", func(t *testing.T) {
		found := false
		for _, s := range pc.SystemSections {
			if s.Title == "Prohibitions" {
				found = true
				break
			}
		}
		if !found {
			t.Error("system sections missing Prohibitions")
		}
	})

	t.Run("user sections include current task", func(t *testing.T) {
		found := false
		for _, s := range pc.UserSections {
			if s.Title == "Current Task" {
				found = true
				break
			}
		}
		if !found {
			t.Error("user sections missing Current Task")
		}
	})

	t.Run("user sections include user request first", func(t *testing.T) {
		if len(pc.UserSections) == 0 {
			t.Fatal("expected user sections to be populated")
		}
		first := pc.UserSections[0]
		if first.Title != "User Request" {
			t.Errorf("first user section = %q, want User Request", first.Title)
		}
		if first.Content != "Add logging" {
			t.Errorf("user request content = %q, want Add logging", first.Content)
		}
	})

	t.Run("system sections do not duplicate objective", func(t *testing.T) {
		for _, s := range pc.SystemSections {
			if s.Title == "Objective" {
				t.Error("Objective should live in user sections, not system sections")
			}
		}
	})

	t.Run("enabled tools from skill", func(t *testing.T) {
		if len(pc.EnabledTools) != 2 {
			t.Errorf("got %d enabled tools, want 2", len(pc.EnabledTools))
		}
	})
}

func TestToMessages(t *testing.T) {
	pc := &types.PromptContext{
		SystemSections: []types.PromptSection{
			{Title: "Identity", Content: "You are planner"},
			{Title: "Goal", Content: "Analyze task"},
		},
		UserSections: []types.PromptSection{
			{Title: "Task", Content: "Fix the bug"},
		},
	}

	msgs := ToMessages(pc)

	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("msg[0] role = %q, want system", msgs[0].Role)
	}
	if msgs[1].Role != "user" {
		t.Errorf("msg[1] role = %q, want user", msgs[1].Role)
	}
}
