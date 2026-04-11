package context

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
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
				{ID: "t1", Title: "Investigate root cause", Writing: false, Status: types.TaskInProgress},
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

func TestRelevantFilesUseLogicalSourceOnly(t *testing.T) {
	facts := []types.RepoFact{
		{
			Key:         "read_file",
			Value:       "x",
			Source:      "internal/agent/explorer.go",
			EvidenceRef: "blob://trace/raw-main.txt",
			Confidence:  0.8,
		},
		{
			Key:         "read_file",
			Value:       "y",
			Source:      "internal/agent/explorer.go",
			EvidenceRef: "blob://trace/raw-sub.txt",
			Confidence:  0.8,
		},
		{
			Key:         "grep",
			Value:       "z",
			Source:      "internal/context/builder.go",
			EvidenceRef: "blob://trace/raw-grep.txt",
			Confidence:  0.8,
		},
	}

	files := extractRelevantFiles(facts)
	if len(files) != 2 {
		t.Fatalf("extractRelevantFiles count = %d, want 2 (dedup by logical source)", len(files))
	}
	if files[0] != "internal/agent/explorer.go" || files[1] != "internal/context/builder.go" {
		t.Fatalf("extractRelevantFiles = %v", files)
	}
}

// TestE2E_PromptRelevantFilesContainsPaths verifies the full chain:
// RepoFacts with logical Source → BuildAgentContext → BuildPromptContext →
// ToMessages. The "Relevant Files" section in the final user message must
// contain real file paths, not tool names like "read_file" or "grep".
func TestE2E_PromptRelevantFilesContainsPaths(t *testing.T) {
	// Simulate facts as produced by a real explorer run after the fix:
	// - read_file facts: Source = logical file path, EvidenceRef = blob ref
	// - grep facts: Source = "tool:grep" (no single file path to extract)
	// - repo_map fact: Source = "tool:repo_map"
	facts := []types.RepoFact{
		{Key: "grep", Value: "[grep: 5 matching files]\n./a.go\n./b.go", Source: "tool:grep", EvidenceRef: "blob://t/1.txt", Confidence: 0.8},
		{Key: "repo_map", Value: "# Task Map\n\n## Relevant Files\n\n### a.go (score: 100)", Source: "tool:repo_map", Confidence: 0.3},
		{Key: "read_file", Value: "[internal/agent/explorer.go: showing lines 1-60 of 800]", Source: "internal/agent/explorer.go", EvidenceRef: "blob://t/2.txt", Confidence: 0.8},
		{Key: "grep", Value: "[grep: 1 matching lines]", Source: "tool:grep", Confidence: 0.8},
		{Key: "read_file", Value: "[internal/agent/fact_source.go: showing all 30 lines]", Source: "internal/agent/fact_source.go", EvidenceRef: "blob://t/3.txt", Confidence: 0.8},
		{Key: "read_file", Value: "[internal/agent/explorer.go: showing lines 600-700 of 800]", Source: "internal/agent/explorer.go", EvidenceRef: "blob://t/4.txt", Confidence: 0.8},
	}

	bus := &types.BusContext{
		PipelineStage: types.StageFinalize,
		RepoRoot:      "/tmp/repo",
		RepoFacts:     facts,
		Mutable: types.NewMutableState(types.TaskList{
			Objective:     "test",
			CurrentTaskID: "t1",
			Tasks:         []types.TaskItem{{ID: "t1", Title: "test task", Status: types.TaskInProgress}},
		}),
	}

	ac := BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)

	// RelevantFiles should be 4 unique sources, no tool names like "read_file"
	for _, f := range ac.RelevantFiles {
		if f == "read_file" || f == "grep" || f == "repo_map" {
			t.Errorf("RelevantFiles contains bare tool name %q — fix regression", f)
		}
	}
	// Should contain real paths (deduped)
	pathCount := 0
	for _, f := range ac.RelevantFiles {
		if strings.Contains(f, "/") {
			pathCount++
		}
	}
	if pathCount < 2 {
		t.Errorf("expected at least 2 real file paths in RelevantFiles, got %d: %v", pathCount, ac.RelevantFiles)
	}

	// Build the prompt and verify the user message
	sk := &skill.Config{Name: "finalize-skill", Goal: "test", Workflow: []string{"step"}, OutputFormat: "text"}
	pc := BuildPromptContext(ac, sk)

	msgs := ToMessages(pc)
	if len(msgs) < 2 {
		t.Fatal("expected at least 2 messages (system + user)")
	}

	userMsg := msgs[1].Content

	// The BuildPromptContext-generated "## Relevant Files" section must exist
	// and must contain actual paths, not tool names.
	if !strings.Contains(userMsg, "## Relevant Files") {
		t.Fatal("user message missing '## Relevant Files' section")
	}
	if !strings.Contains(userMsg, "internal/agent/explorer.go") {
		t.Error("user message Relevant Files missing 'internal/agent/explorer.go'")
	}
	if !strings.Contains(userMsg, "internal/agent/fact_source.go") {
		t.Error("user message Relevant Files missing 'internal/agent/fact_source.go'")
	}

	// Negative: the old broken behavior would have "read_file" and "grep"
	// as standalone lines in the Relevant Files section.
	// Extract text between the last "## Relevant Files" and the next "##" or end.
	idx := strings.LastIndex(userMsg, "## Relevant Files")
	if idx < 0 {
		t.Fatal("cannot find ## Relevant Files")
	}
	rfSection := userMsg[idx:]
	if nextSec := strings.Index(rfSection[3:], "\n## "); nextSec > 0 {
		rfSection = rfSection[:nextSec+3]
	}
	// In this section, "read_file" should NOT appear as a path entry
	for _, line := range strings.Split(rfSection, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "read_file" || trimmed == "grep" || trimmed == "repo_map" {
			t.Errorf("Relevant Files section contains bare tool name line: %q", trimmed)
		}
	}
}

func TestBuildSubAgentContextFilterScopeUsesLogicalSource(t *testing.T) {
	bus := &types.BusContext{
		PipelineStage: types.StageExplore,
		RepoRoot:      "/tmp/repo",
		RepoFacts: []types.RepoFact{
			{
				Key:         "read_file",
				Value:       "main",
				Source:      "internal/agent/explorer.go",
				EvidenceRef: "blob://trace/main.txt",
				Confidence:  0.8,
			},
			{
				Key:         "read_file",
				Value:       "sub",
				Source:      "internal/context/builder.go",
				EvidenceRef: "blob://trace/sub.txt",
				Confidence:  0.8,
			},
		},
	}
	req := &types.SubAgentRequest{
		ID:        "sa-1",
		SubAgent:  "sub_explorer",
		Objective: "focus on context package",
		Scope:     []string{"internal/context"},
	}

	ac := BuildSubAgentContext(bus, req)
	if len(ac.RelevantFiles) != 1 {
		t.Fatalf("relevant files = %d, want 1", len(ac.RelevantFiles))
	}
	if ac.RelevantFiles[0] != "internal/context/builder.go" {
		t.Fatalf("relevant files = %v", ac.RelevantFiles)
	}
}
