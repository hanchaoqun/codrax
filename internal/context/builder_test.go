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
		Mutable: types.NewMutableState("Fix the bug"),
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

	t.Run("objective preserved", func(t *testing.T) {
		if ac.Objective != "Fix the bug" {
			t.Errorf("got objective %q, want %q", ac.Objective, "Fix the bug")
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
		AgentName:     types.AgentAnalyzer,
		Stage:         types.StageAnalyze,
		Objective:     "Analyze requirements",
		MissingPiece:  types.MissingUnderstanding,
		Constraints:   []string{"must be backwards compatible"},
		RelevantFacts: []string{"uses logrus"},
	}

	sk := &skill.Config{
		Name:            "analysis-skill",
		Goal:            "Structurize and classify the user task",
		Workflow:        []string{"read input", "identify intent", "classify type"},
		ToolSuggestions: []string{"grep", "read_file"},
		OutputFormat:    "JSON with task_type, objective",
		Prohibitions:    []string{"do not implement"},
	}

	pc := BuildPromptContext(ac, sk)

	t.Run("metadata", func(t *testing.T) {
		if pc.AgentName != types.AgentAnalyzer {
			t.Errorf("got agent %s, want %s", pc.AgentName, types.AgentAnalyzer)
		}
		if pc.SkillName != "analysis-skill" {
			t.Errorf("got skill %q, want %q", pc.SkillName, "analysis-skill")
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

	t.Run("user sections include user request first", func(t *testing.T) {
		if len(pc.UserSections) == 0 {
			t.Fatal("expected user sections to be populated")
		}
		first := pc.UserSections[0]
		if first.Title != "User Request" {
			t.Errorf("first user section = %q, want User Request", first.Title)
		}
		if first.Content != "Analyze requirements" {
			t.Errorf("user request content = %q, want Analyze requirements", first.Content)
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
		Mutable: types.NewMutableState("test"),
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

func TestBuildPromptContextIncludesStructuredEvidenceAndDataflow(t *testing.T) {
	ac := &types.AgentContext{
		AgentName:     types.AgentExplorer,
		Stage:         types.StageExplore,
		Objective: "Investigate registration flow",
		EvidenceItems: []types.EvidenceItem{
			{
				ID:        "ev-1",
				Kind:      types.EvidenceConcrete,
				Subject:   "Handler.Name",
				Predicate: "returns",
				Object:    "\"explorer\"",
				Summary:   "`Handler.Name` line 18 returns \"explorer\"",
				Source:    "internal/agent/explorer.go",
				LineStart: 18,
				LineEnd:   18,
			},
			{
				ID:        "ev-2",
				Kind:      types.EvidenceConditional,
				Subject:   "Register",
				Predicate: "guards",
				Object:    "dispatch",
				Summary:   "`Register` line 40 dispatches IF handler is enabled",
				Source:    "internal/agent/explorer.go",
				LineStart: 40,
				LineEnd:   40,
			},
		},
		FlowFindings: []types.FlowFindingDigest{
			{
				ID:         "flow-1",
				Path:       []string{"config.handlers.explorer", "NewExplorer", "Register"},
				Conditions: []string{"handler is enabled"},
				Sources:    []string{"config/orchestrator.yaml"},
				Sinks:      []string{"Register"},
				Hops:       []string{"reads_config", "calls"},
				Confidence: 0.84,
			},
		},
	}

	sk := &skill.Config{Name: "explore", Goal: "investigate", Workflow: []string{"read", "analyze"}, OutputFormat: "text"}
	pc := BuildPromptContext(ac, sk)
	msgs := ToMessages(pc)
	if len(msgs) < 2 {
		t.Fatalf("expected system + user messages, got %d", len(msgs))
	}

	userMsg := msgs[1].Content
	if !strings.Contains(userMsg, "## Structured Evidence") {
		t.Fatalf("user prompt missing structured evidence section:\n%s", userMsg)
	}
	if !strings.Contains(userMsg, "Handler.Name") {
		t.Fatalf("user prompt missing evidence content:\n%s", userMsg)
	}
	if !strings.Contains(userMsg, "## Dataflow Findings") {
		t.Fatalf("user prompt missing dataflow findings section:\n%s", userMsg)
	}
	if !strings.Contains(userMsg, "config.handlers.explorer -> NewExplorer -> Register") {
		t.Fatalf("user prompt missing flow path:\n%s", userMsg)
	}
}

func TestBuildSubAgentContextFiltersEvidenceItemsByScope(t *testing.T) {
	bus := &types.BusContext{
		PipelineStage: types.StageExplore,
		RepoRoot:      "/tmp/repo",
		EvidenceItems: []types.EvidenceItem{
			{
				ID:        "ev-1",
				Kind:      types.EvidenceConcrete,
				Subject:   "Builder.Name",
				Predicate: "returns",
				Object:    "\"builder\"",
				Source:    "internal/context/builder.go",
				LineStart: 12,
				LineEnd:   12,
			},
			{
				ID:        "ev-2",
				Kind:      types.EvidenceConcrete,
				Subject:   "Explorer.Name",
				Predicate: "returns",
				Object:    "\"explorer\"",
				Source:    "internal/agent/explorer.go",
				LineStart: 20,
				LineEnd:   20,
			},
		},
		FlowFindings: []types.FlowFindingDigest{
			{
				ID:          "flow-1",
				Path:        []string{"internal/context/builder.go", "BuildPromptContext"},
				Sources:     []string{"internal/context/builder.go"},
				Sinks:       []string{"BuildPromptContext"},
				EvidenceIDs: []string{"ev-1"},
				Confidence:  0.8,
			},
			{
				ID:          "flow-2",
				Path:        []string{"internal/agent/explorer.go", "Register"},
				Sources:     []string{"internal/agent/explorer.go"},
				Sinks:       []string{"Register"},
				EvidenceIDs: []string{"ev-2"},
				Confidence:  0.8,
			},
		},
	}

	req := &types.SubAgentRequest{
		ID:        "sa-2",
		SubAgent:  "sub_explorer",
		Objective: "focus on context",
		Scope:     []string{"internal/context"},
	}

	ac := BuildSubAgentContext(bus, req)
	if len(ac.EvidenceItems) != 1 {
		t.Fatalf("filtered evidence count = %d, want 1", len(ac.EvidenceItems))
	}
	if ac.EvidenceItems[0].Source != "internal/context/builder.go" {
		t.Fatalf("filtered evidence = %+v", ac.EvidenceItems)
	}
	if len(ac.FlowFindings) != 1 {
		t.Fatalf("filtered flow findings count = %d, want 1", len(ac.FlowFindings))
	}
	if ac.FlowFindings[0].ID != "flow-1" {
		t.Fatalf("filtered flow findings = %+v", ac.FlowFindings)
	}
}

// TestFormatEvidenceItemsRendersUngroundedTag pins the P0.1 contract:
// evidence items whose Producer carries the "/ungrounded" suffix
// (assigned by agent.groundEvidenceItems when every validation tier
// rejected the LLM-cited line number) must appear in the finalizer
// prompt with a visible [UNGROUNDED: cite without line number] tag.
// This upgrades the /ungrounded contract from prompt-level soft
// constraint (set in finalizer.go) to a renderer-level visual marker
// the LLM cannot miss. Mirrors roadmap item P0.1.
func TestFormatEvidenceItemsRendersUngroundedTag(t *testing.T) {
	items := []types.EvidenceItem{
		{
			Kind:      types.EvidenceConcrete,
			Subject:   "Handler.Name",
			Predicate: "returns",
			Object:    "\"explorer\"",
			Source:    "internal/agent/explorer.go",
			LineStart: 18,
			Producer:  "explorer",
		},
		{
			Kind:      types.EvidenceConcrete,
			Subject:   "Register",
			Predicate: "binds",
			Object:    "NewExplorerAgent",
			Source:    "internal/agent/registry.go",
			// LineStart cleared by groundEvidenceItems when
			// tier1+tier2 failed; Producer tagged.
			Producer: "explorer/ungrounded",
		},
	}
	out := formatEvidenceItems(items, 0)
	if !strings.Contains(out, "[UNGROUNDED: cite without line number]") {
		t.Fatalf("ungrounded evidence item missing visible tag:\n%s", out)
	}
	// Grounded items must NOT carry the tag — otherwise the LLM
	// would strip line numbers from every cite.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Handler.Name") && strings.Contains(line, "[UNGROUNDED") {
			t.Fatalf("grounded item wrongly tagged ungrounded:\n%s", line)
		}
	}
	// Sanity: the ungrounded line must still render (demote, not
	// drop — renderer stays purely presentational).
	if !strings.Contains(out, "Register") {
		t.Fatalf("ungrounded item dropped instead of demoted:\n%s", out)
	}
}

// -----------------------------------------------------------------------------
// P2.1 Phase 11 — AnswerSymbolCompleteness three-way rendering
// -----------------------------------------------------------------------------
//
// The Extracted Answer Symbols section in BuildPromptContext must
// branch on ac.AnswerSymbolCompleteness:
//
//   - CompletenessComplete → Translation mode ("MUST NOT add or remove")
//   - CompletenessLowerBound → softened floor mode ("MUST include at
//     least these, MAY add more")
//   - CompletenessUnknown / zero → drop the section entirely
//
// These tests pin each branch so the Phase 9 wiring and the legacy
// flag=off path cannot silently converge.

func buildAgentCtxWithSymbols(syms []types.AnswerSymbol, claim types.CompletenessClaim) *types.AgentContext {
	return &types.AgentContext{
		AgentName:                types.AgentFinalizer,
		Stage:                    types.StageFinalize,
		Objective:                "q",
		AnswerSymbols:            syms,
		AnswerSymbolCompleteness: claim,
	}
}

func findSectionTitle(pc *types.PromptContext, title string) *types.PromptSection {
	for i := range pc.UserSections {
		if pc.UserSections[i].Title == title {
			return &pc.UserSections[i]
		}
	}
	return nil
}

func TestBuildPromptContext_AnswerSymbols_Complete_RendersTranslationMode(t *testing.T) {
	syms := []types.AnswerSymbol{
		{Name: "Foo", File: "a.go", Line: 10, Kind: "registration"},
		{Name: "Bar", File: "b.go", Line: 20, Kind: "registration"},
	}
	ac := buildAgentCtxWithSymbols(syms, types.CompletenessComplete)
	pc := BuildPromptContext(ac, &skill.Config{Name: "s"})

	sec := findSectionTitle(pc, "Extracted Answer Symbols (deterministic, authoritative)")
	if sec == nil {
		t.Fatal("Translation-mode section missing for CompletenessComplete")
	}
	if !strings.Contains(sec.Content, "MUST NOT add or remove") {
		t.Errorf("Complete branch must include MUST-NOT directive, got:\n%s", sec.Content)
	}
	if !strings.Contains(sec.Content, "Foo") || !strings.Contains(sec.Content, "Bar") {
		t.Errorf("Complete branch must list all symbols, got:\n%s", sec.Content)
	}
	// Guard against the lower-bound title leaking in
	if findSectionTitle(pc, "Answer Symbols (deterministic floor, may extend with cited evidence)") != nil {
		t.Error("Complete branch must not emit the lower-bound section title")
	}
}

func TestBuildPromptContext_AnswerSymbols_LowerBound_RendersSoftenedFloor(t *testing.T) {
	syms := []types.AnswerSymbol{
		{Name: "Foo", File: "a.go", Line: 10, Kind: "registration"},
	}
	ac := buildAgentCtxWithSymbols(syms, types.CompletenessLowerBound)
	pc := BuildPromptContext(ac, &skill.Config{Name: "s"})

	sec := findSectionTitle(pc, "Answer Symbols (deterministic floor, may extend with cited evidence)")
	if sec == nil {
		t.Fatal("LowerBound section missing for CompletenessLowerBound")
	}
	if !strings.Contains(sec.Content, "LOWER BOUND") {
		t.Errorf("LowerBound branch must state the lower-bound contract, got:\n%s", sec.Content)
	}
	if !strings.Contains(sec.Content, "MAY add additional symbols") {
		t.Errorf("LowerBound branch must permit additional symbols, got:\n%s", sec.Content)
	}
	if strings.Contains(sec.Content, "MUST NOT add or remove") {
		t.Errorf("LowerBound branch must NOT carry the Translation MUST-NOT directive, got:\n%s", sec.Content)
	}
	if !strings.Contains(sec.Content, "Foo") {
		t.Errorf("LowerBound branch must list floor symbols, got:\n%s", sec.Content)
	}
	// Guard against Translation-mode title leaking
	if findSectionTitle(pc, "Extracted Answer Symbols (deterministic, authoritative)") != nil {
		t.Error("LowerBound branch must not emit the Translation-mode section title")
	}
}

func TestBuildPromptContext_AnswerSymbols_Unknown_DropsSection(t *testing.T) {
	// CompletenessUnknown (zero value) with non-empty symbol slate =
	// producer bug per the Phase 11 contract. Rendering layer fails
	// closed by dropping the section so a partial slate cannot reach
	// the finalizer with unchecked authority.
	syms := []types.AnswerSymbol{
		{Name: "Foo", File: "a.go", Line: 10, Kind: "registration"},
	}
	ac := buildAgentCtxWithSymbols(syms, types.CompletenessUnknown)
	pc := BuildPromptContext(ac, &skill.Config{Name: "s"})

	if findSectionTitle(pc, "Extracted Answer Symbols (deterministic, authoritative)") != nil {
		t.Error("Unknown claim must not render the Translation-mode section")
	}
	if findSectionTitle(pc, "Answer Symbols (deterministic floor, may extend with cited evidence)") != nil {
		t.Error("Unknown claim must not render the LowerBound section")
	}
}

func TestBuildPromptContext_AnswerSymbols_EmptyAlwaysDrops(t *testing.T) {
	// Empty slate + any claim must drop the section. This guards
	// against a Phase 9 producer that sets CompletenessComplete with
	// zero items (which would be a vacuous "complete" claim).
	for _, claim := range []types.CompletenessClaim{
		types.CompletenessUnknown, types.CompletenessLowerBound, types.CompletenessComplete,
	} {
		ac := buildAgentCtxWithSymbols(nil, claim)
		pc := BuildPromptContext(ac, &skill.Config{Name: "s"})
		if findSectionTitle(pc, "Extracted Answer Symbols (deterministic, authoritative)") != nil {
			t.Errorf("claim=%q with empty slate must drop Translation section", claim)
		}
		if findSectionTitle(pc, "Answer Symbols (deterministic floor, may extend with cited evidence)") != nil {
			t.Errorf("claim=%q with empty slate must drop LowerBound section", claim)
		}
	}
}

func TestBuildPromptContext_RendersHypothesisVerdictsSection(t *testing.T) {
	mu := types.NewMutableState("")
	mu.AppendEmittedHypothesisVerdicts([]types.HypothesisVerdict{
		{HypothesisID: "H1", Status: types.HypConfirmed, Rationale: "direct binding", Citation: "reg.go:12"},
		{HypothesisID: "H2", Status: types.HypInconclusive, Rationale: "no conclusive cite"},
	})
	ac := &types.AgentContext{
		AgentName:   types.AgentFinalizer,
		Stage:       types.StageFinalize,
		Objective: "q",
		Mutable:     mu,
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "final-answer-skill"})

	sec := findSectionTitle(pc, "Hypothesis Verdicts")
	if sec == nil {
		t.Fatal("Hypothesis Verdicts section missing when buffer populated")
	}
	for _, want := range []string{"H1", "confirmed", "direct binding", "reg.go:12", "H2", "inconclusive", "no conclusive cite"} {
		if !strings.Contains(sec.Content, want) {
			t.Errorf("section missing %q:\n%s", want, sec.Content)
		}
	}
}

func TestBuildPromptContext_SkipsHypothesisVerdictsWhenEmpty(t *testing.T) {
	mu := types.NewMutableState("")
	ac := &types.AgentContext{
		AgentName:   types.AgentFinalizer,
		Stage:       types.StageFinalize,
		Objective: "q",
		Mutable:     mu,
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "s"})
	if findSectionTitle(pc, "Hypothesis Verdicts") != nil {
		t.Error("empty verdict buffer must not produce the section")
	}
}

func TestBuildPromptContext_SkipsHypothesisVerdictsWhenMutableNil(t *testing.T) {
	ac := &types.AgentContext{
		AgentName:   types.AgentFinalizer,
		Stage:       types.StageFinalize,
		Objective: "q",
		Mutable:     nil,
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "s"})
	if findSectionTitle(pc, "Hypothesis Verdicts") != nil {
		t.Error("nil Mutable must not produce the section")
	}
}

func TestBuildAgentContext_CarriesCompletenessFromBus(t *testing.T) {
	bus := &types.BusContext{
		PipelineStage:            types.StageFinalize,
		ActiveAgent:              types.AgentFinalizer,
		AnswerSymbols:            []types.AnswerSymbol{{Name: "Foo", File: "a.go", Line: 1}},
		AnswerSymbolCompleteness: types.CompletenessLowerBound,
		Mutable:                  types.NewMutableState(""),
	}
	ac := BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	if ac.AnswerSymbolCompleteness != types.CompletenessLowerBound {
		t.Errorf("completeness not plumbed through builder: got %q, want lower_bound", ac.AnswerSymbolCompleteness)
	}
}
