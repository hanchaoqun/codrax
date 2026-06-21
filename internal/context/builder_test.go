package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBuildAgentContext(t *testing.T) {
	bus := &types.BusContext{
		PipelineStage:         types.StageExplore,
		ActiveAgent:           types.AgentExplorer,
		ExploreToolSurface:    types.ExploreToolSurfaceSourceInventoryLens,
		RepoRoot:              "/tmp/repo",
		Branch:                "main",
		Commit:                "abc123",
		EvalDisableGitHistory: true,
		Mutable:               types.NewMutableState("Fix the bug"),
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
		if !ac.EvalDisableGitHistory {
			t.Errorf("EvalDisableGitHistory was not propagated")
		}
		if ac.ExploreToolSurface != types.ExploreToolSurfaceSourceInventoryLens {
			t.Errorf("ExploreToolSurface was not propagated")
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

// TestBuildAgentContext_AttachedLogMirrored pins the log-triage
// plumbing contract: BusContext.AttachedLog MUST propagate into
// AgentContext.AttachedLog via BuildAgentContext. Regressions here
// silently disable the whole log-triage feature because the log_triager
// agent would see no payload and degrade to a nil bundle.
func TestBuildAgentContext_AttachedLogMirrored(t *testing.T) {
	payload := "panic: oops\n\ngoroutine 1 [running]:\nmain.x()\n\t/src/app.go:7 +0x1\n"
	bus := &types.BusContext{
		PipelineStage: types.StageAnalyze,
		RepoRoot:      "/tmp/repo",
		Mutable:       types.NewMutableState("crash question"),
		AttachedLog:   payload,
		TaskState:     types.TaskState{Stage: types.StageAnalyze},
	}
	ac := BuildAgentContext(bus, types.AgentAnalyzer, types.StageAnalyze)
	if ac.AttachedLog != payload {
		t.Fatalf("AttachedLog not mirrored: got %q, want %q",
			ac.AttachedLog, payload)
	}
}

func TestBuildAgentContext_TurnRouteHintMirrored(t *testing.T) {
	hint := types.TurnRouteHint{
		Route:             "repo",
		Source:            "external_tool",
		Operation:         "external_skill_workflow",
		NeedsRepoAccess:   true,
		ConcreteOperation: false,
		Confidence:        0.85,
	}
	bus := &types.BusContext{
		PipelineStage: types.StageAnalyze,
		RepoRoot:      "/tmp/repo",
		Mutable:       types.NewMutableState("external observation question"),
		TurnRouteHint: hint,
		TaskState:     types.TaskState{Stage: types.StageAnalyze},
	}
	ac := BuildAgentContext(bus, types.AgentAnalyzer, types.StageAnalyze)
	if ac.TurnRouteHint != hint {
		t.Fatalf("TurnRouteHint not mirrored: got %+v, want %+v", ac.TurnRouteHint, hint)
	}
}

func TestRelationDossierSourceInventoryMergesTurnAHandoff(t *testing.T) {
	mut := types.NewMutableState("source inventory")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		SourceInventoryObservation: types.SourceInventoryObservation{
			Active: true,
			Sets: []types.SourceInventoryObservationSet{{
				Role:  types.AnswerCandidateRoleFunction,
				Count: 1,
				Members: []types.SourceInventoryObservationMember{{
					Name:       "loadW3Token",
					Key:        "api.ts:10",
					SupportRef: "api.ts:10",
					File:       "api.ts",
					Line:       10,
				}},
			}},
		},
	})

	got := relationDossierSourceInventory(&types.AgentContext{Mutable: mut})
	if !got.IsActive() || len(got.Sets) != 1 || got.Sets[0].Members[0].Name != "loadW3Token" {
		t.Fatalf("relation dossier should consume TurnA source-inventory handoff: %+v", got)
	}
}

// TestBuildAgentContext_AttachedLogEmpty_SafeDefault verifies the
// negative case — when no log is attached, AgentContext.AttachedLog
// stays empty and nothing else breaks. The log_triage pre-stage
// Guard short-circuits on empty AttachedLog, so this is the
// "feature disabled by absence" path.
func TestBuildAgentContext_AttachedLogEmpty_SafeDefault(t *testing.T) {
	bus := &types.BusContext{
		PipelineStage: types.StageAnalyze,
		RepoRoot:      "/tmp/repo",
		Mutable:       types.NewMutableState("plain question"),
		TaskState:     types.TaskState{Stage: types.StageAnalyze},
	}
	ac := BuildAgentContext(bus, types.AgentAnalyzer, types.StageAnalyze)
	if ac.AttachedLog != "" {
		t.Errorf("empty BusContext.AttachedLog must mirror empty, got %q",
			ac.AttachedLog)
	}
}

func TestBuildAgentContext_RetryHintScopedToOwningStage(t *testing.T) {
	bus := &types.BusContext{
		Mutable: types.NewMutableState("q"),
		TaskState: types.TaskState{
			Stage:     types.StageExplore,
			RetryHint: "DAG-scheduled investigation window",
		},
	}
	analyzer := BuildAgentContext(bus, types.AgentAnalyzer, types.StageAnalyze)
	if analyzer.RetryHint != "" {
		t.Fatalf("analyzer must not inherit explore retry hint, got %q", analyzer.RetryHint)
	}
	explorer := BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	if explorer.RetryHint != "DAG-scheduled investigation window" {
		t.Fatalf("explorer should receive its own retry hint, got %q", explorer.RetryHint)
	}

	bus.TaskState.Stage = types.StageExtract
	bus.TaskState.RetryHintStage = types.StageExplore
	bus.TaskState.RetryHint = "Forced Read List: read internal/agent/agent.go before completing"
	extractor := BuildAgentContext(bus, types.AgentExtractor, types.StageExtract)
	if extractor.RetryHint != "" {
		t.Fatalf("extractor must not inherit explore-owned retry hint, got %q", extractor.RetryHint)
	}

	bus.TaskState.RetryHintStage = types.StageExtract
	bus.TaskState.RetryHint = "Re-emit the hypothesis verdict from existing Turn A evidence"
	extractor = BuildAgentContext(bus, types.AgentExtractor, types.StageExtract)
	if extractor.RetryHint != "Re-emit the hypothesis verdict from existing Turn A evidence" {
		t.Fatalf("extractor should receive extractor-owned retry hint, got %q", extractor.RetryHint)
	}
}

func TestBuildPromptContext_RetryHintProjectsUnavailableToolsForExtractor(t *testing.T) {
	ac := &types.AgentContext{
		Stage:     types.StageExtract,
		RetryHint: "Resolution Chain anchor line is outside the fetched slices; call repo_map, grep(files_only=true), and read_file on internal/agent/agent.go.",
	}
	pc := BuildPromptContext(ac, &skill.Config{
		Name:            "extract-skill",
		ToolSuggestions: []string{"emit_evidence", "emit_answer_symbol", "emit_hypothesis_verdict"},
	})
	sec := findSectionTitle(pc, SectionRetryDirective)
	if sec == nil {
		t.Fatal("missing retry directive section")
	}
	for _, forbidden := range []string{"repo_map", "read_file", "grep("} {
		if strings.Contains(sec.Content, forbidden) {
			t.Fatalf("extractor retry directive leaked unavailable tool %q:\n%s", forbidden, sec.Content)
		}
	}
	for _, want := range []string{"unavailable in this extraction stage", "`emit_answer_symbol`", "accepted investigation snapshot"} {
		if !strings.Contains(sec.Content, want) {
			t.Fatalf("projected retry directive missing %q:\n%s", want, sec.Content)
		}
	}
}

func TestBuildPromptContext_RetryHintKeepsAvailableExplorerTools(t *testing.T) {
	const hint = "Previous attempt used fewer than 2 distinct evidence tool types. Use both grep and read_file."
	ac := &types.AgentContext{
		Stage:     types.StageExplore,
		RetryHint: hint,
	}
	pc := BuildPromptContext(ac, &skill.Config{
		Name:            "explore-skill",
		ToolSuggestions: []string{"grep", "read_file", "repo_map", "emit_evidence", "emit_investigation_complete"},
	})
	sec := findSectionTitle(pc, SectionRetryDirective)
	if sec == nil {
		t.Fatal("missing retry directive section")
	}
	if sec.Content != hint {
		t.Fatalf("available explorer tool hint should stay unchanged:\n got: %q\nwant: %q", sec.Content, hint)
	}
}

// TestBuildPromptContext_AttachedLogSection_Rendered verifies the
// session-19 eval-FAIL fix: when AttachedLog is non-empty, the user
// prompt MUST include an "Attached Runtime Log" section containing the
// log body so every LLM (explorer / extractor / finalizer) sees the
// actual panic text — not just the Entities / RequiredFiles anchors
// the analyzer extracted. Without this section the explorer LLM
// interpreted "这个 panic 来自哪里" as asking about panics in general
// and burned 20 iterations searching broadly instead of opening the
// specific frame files.
func TestBuildPromptContext_AttachedLogSection_Rendered(t *testing.T) {
	mu := types.NewMutableState("这个 panic 来自哪里")
	ac := &types.AgentContext{
		AgentName:   types.AgentExplorer,
		Stage:       types.StageExplore,
		Objective:   "这个 panic 来自哪里",
		Mutable:     mu,
		AttachedLog: "panic: runtime error\n\ngoroutine 1 [running]:\nmain.crashy()\n\t/src/main.go:42 +0x1e\n",
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "test"})
	var found *types.PromptSection
	for i := range pc.UserSections {
		if pc.UserSections[i].Title == SectionAttachedRuntimeLog {
			found = &pc.UserSections[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no 'Attached Runtime Log' section; sections = %v", SectionTitles(pc.UserSections))
	}
	if !strings.Contains(found.Content, "main.crashy") {
		t.Errorf("section content missing frame func; got %q", found.Content)
	}
	if !strings.Contains(found.Content, "/src/main.go:42") {
		t.Errorf("section content missing frame file:line; got %q", found.Content)
	}
	if !strings.Contains(found.Content, "     1│ panic: runtime error") {
		t.Errorf("section content missing artifact-local line gutter; got %q", found.Content)
	}
	if !strings.Contains(found.Content, "not as a repository source citation") {
		t.Errorf("section content missing artifact/source boundary note; got %q", found.Content)
	}
}

// TestBuildPromptContext_AttachedLogSection_OmittedWhenEmpty confirms
// the section is skipped when no log is attached — zero noise for the
// 99% of questions that are not log-triage queries.
func TestBuildPromptContext_AttachedLogSection_OmittedWhenEmpty(t *testing.T) {
	mu := types.NewMutableState("plain question")
	ac := &types.AgentContext{
		AgentName: types.AgentExplorer,
		Stage:     types.StageExplore,
		Objective: "plain question",
		Mutable:   mu,
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "test"})
	for _, s := range pc.UserSections {
		if s.Title == SectionAttachedRuntimeLog {
			t.Fatalf("unexpected Attached Runtime Log section when AttachedLog empty")
		}
	}
}

// TestFormatAttachedLog_InlineSmall verifies the ≤ 4 KB fast path —
// whole body rendered with artifact-local line gutters, no elision,
// no blob.
func TestFormatAttachedLog_InlineSmall(t *testing.T) {
	payload := "panic: boom\n\ngoroutine 1 [running]:\nmain.x()\n\t/src/a.go:1 +0x1\n"
	got := formatAttachedLog(payload, "", attachedTriageStructured, "")
	if !strings.Contains(got, "main.x()") || !strings.Contains(got, "/src/a.go:1") {
		t.Errorf("small payload not rendered: %s", got)
	}
	if !strings.Contains(got, "     1│ panic: boom") || !strings.Contains(got, "     5│ \t/src/a.go:1 +0x1") {
		t.Errorf("small payload missing stable artifact line gutters: %s", got)
	}
	if strings.Contains(got, "```go") || !strings.Contains(got, "```text") {
		t.Errorf("attached artifact should render as text fence: %s", got)
	}
	if strings.Contains(got, "bytes elided") {
		t.Errorf("small payload should not have elision marker")
	}
}

func TestFormatAttachedLog_PreambleTracksTriageState(t *testing.T) {
	payload := "panic: boom\nmain.x()\n\t/src/a.go:1\n"

	missing := formatAttachedLog(payload, "", attachedTriageUnavailable, "")
	if !strings.Contains(missing, "No structured log summary is available") {
		t.Fatalf("missing-bundle preamble should say raw log is unparsed: %s", missing)
	}
	if strings.Contains(missing, "already parsed stack frames") {
		t.Fatalf("missing-bundle preamble must not claim triage success: %s", missing)
	}

	structured := formatAttachedLog(payload, "", attachedTriageStructured, "")
	if !strings.Contains(structured, "structured log summary is already available") {
		t.Fatalf("structured-bundle preamble missing preferred structured-source cue: %s", structured)
	}

	producer := formatAttachedLog(payload, "", attachedTriageProducer, "")
	if !strings.Contains(producer, "Prepare a structured summary") {
		t.Fatalf("producer preamble should instruct the triager to parse: %s", producer)
	}
}

// TestFormatAttachedLog_BlobOffload verifies the > 4 KB path: full body
// written to WorkDir/attached_log.txt, preview shows head + tail + blob
// path so the LLM can read_file to paginate through the elided middle.
func TestFormatAttachedLog_BlobOffload(t *testing.T) {
	dir := t.TempDir()
	// 8 KB payload above the 4 KB inline cap.
	const N = 8 * 1024
	payload := strings.Repeat("A", N)
	got := formatAttachedLog(payload, dir, attachedTriageStructured, "")
	if !strings.Contains(got, "bytes elided") {
		t.Errorf("large payload missing elision marker")
	}
	if !strings.Contains(got, AttachedLogBlobName) {
		t.Errorf("blob path not referenced: %s", got)
	}
	// Blob file was written with the full payload.
	written, err := os.ReadFile(filepath.Join(dir, AttachedLogBlobName))
	if err != nil {
		t.Fatalf("read blob: %v", err)
	}
	if len(written) != N {
		t.Errorf("blob size = %d, want %d", len(written), N)
	}
}

func TestFormatAttachedLog_BlobPreviewKeepsArtifactLineGutters(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 600)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%03d %s", i+1, strings.Repeat("x", 12))
	}
	payload := strings.Join(lines, "\n")
	got := formatAttachedLog(payload, dir, attachedTriageStructured, "")
	if !strings.Contains(got, "     1│ line-001") {
		t.Fatalf("head preview missing artifact line gutter:\n%s", got)
	}
	if !strings.Contains(got, "exact middle artifact-line anchors") {
		t.Fatalf("blob preview should tell the model how to get exact middle artifact lines:\n%s", got)
	}
	if !strings.Contains(got, "│ line-600") {
		t.Fatalf("tail preview should keep original artifact line number:\n%s", got)
	}
}

// TestFormatAttachedLog_NoWorkDirFallsBack confirms that when WorkDir
// is empty (unit-test context / degraded runtime), the function still
// returns a bounded head+tail rendering instead of dumping the full
// payload or panicking.
func TestFormatAttachedLog_NoWorkDirFallsBack(t *testing.T) {
	const N = 10 * 1024
	payload := strings.Repeat("B", N)
	got := formatAttachedLog(payload, "", attachedTriageUnavailable, "")
	if strings.Contains(got, AttachedLogBlobName) {
		t.Errorf("no-workdir fallback should not reference blob path")
	}
	if !strings.Contains(got, "bytes elided") {
		t.Errorf("no-workdir fallback missing elision marker")
	}
	if len(got) >= N {
		t.Errorf("fallback length %d >= raw %d; cap did not fire", len(got), N)
	}
}

func TestFormatAttachedTrace_BlobOffload_UsesDistinctBlobName(t *testing.T) {
	dir := t.TempDir()
	const N = 8 * 1024
	payload := strings.Repeat("T", N)
	got := formatAttachedTrace(payload, dir, attachedTriageStructured, "")
	if !strings.Contains(got, AttachedTraceBlobName) {
		t.Fatalf("trace blob path not referenced: %s", got)
	}
	if strings.Contains(got, AttachedLogBlobName) {
		t.Fatalf("trace rendering should not reuse runtime-log blob name: %s", got)
	}
	if !strings.Contains(strings.ToLower(got), "performance trace") {
		t.Fatalf("trace preamble should mention performance trace: %s", got)
	}
	written, err := os.ReadFile(filepath.Join(dir, AttachedTraceBlobName))
	if err != nil {
		t.Fatalf("read trace blob: %v", err)
	}
	if len(written) != N {
		t.Fatalf("trace blob size = %d, want %d", len(written), N)
	}
}

func TestFormatAttachedTrace_InlineSmallHasArtifactLineGutters(t *testing.T) {
	payload := "sched_switch: prev_comm=main next_comm=RenderThread\ntracing_mark_write: B|1|H:GC\n"
	got := formatAttachedTrace(payload, "", attachedTriageStructured, "")
	if !strings.Contains(got, "     1│ sched_switch") || !strings.Contains(got, "     2│ tracing_mark_write") {
		t.Fatalf("trace prompt should expose artifact-local event line gutters:\n%s", got)
	}
	if !strings.Contains(got, "not as a repository source citation") {
		t.Fatalf("trace prompt should separate artifact lines from repo citations:\n%s", got)
	}
}

func TestFormatAttachedTrace_PreambleTracksTriageState(t *testing.T) {
	payload := "sched_switch: prev_comm=main next_comm=RenderThread\n"

	missing := formatAttachedTrace(payload, "", attachedTriageUnavailable, "")
	if !strings.Contains(missing, "No structured performance summary is available") {
		t.Fatalf("missing-bundle preamble should say raw trace is unparsed: %s", missing)
	}
	if strings.Contains(missing, "already extracted structured hotspots") {
		t.Fatalf("missing-bundle preamble must not claim perf triage success: %s", missing)
	}

	structured := formatAttachedTrace(payload, "", attachedTriageStructured, "")
	if !strings.Contains(structured, "structured performance summary is already available") {
		t.Fatalf("structured-bundle preamble missing preferred structured-source cue: %s", structured)
	}

	producer := formatAttachedTrace(payload, "", attachedTriageProducer, "")
	if !strings.Contains(producer, "Prepare a structured summary") {
		t.Fatalf("producer preamble should instruct the triager to parse: %s", producer)
	}
}

func SectionTitles(sections []types.PromptSection) []string {
	out := make([]string, 0, len(sections))
	for _, s := range sections {
		out = append(out, s.Title)
	}
	return out
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
			if s.Title == SectionWorkflow {
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
			if s.Title == SectionProhibitions {
				found = true
				break
			}
		}
		if !found {
			t.Error("system sections missing Prohibitions")
		}
	})

	t.Run("missing piece moves to pipeline state", func(t *testing.T) {
		sec := findSystemSection(pc, SectionPipelineState)
		if sec == nil {
			t.Fatal("system sections missing Pipeline State")
		}
		if !strings.Contains(sec.Content, string(types.MissingUnderstanding)) {
			t.Errorf("Pipeline State missing missing-piece detail, got %q", sec.Content)
		}
		if !strings.Contains(sec.Content, "NOT as part of the user's request") {
			t.Errorf("Pipeline State should explain the control/data boundary, got %q", sec.Content)
		}
		if findSectionTitle(pc, "Missing Piece") != nil {
			t.Error("Missing Piece must not render as a user section")
		}
	})

	t.Run("user sections include user request first", func(t *testing.T) {
		if len(pc.UserSections) == 0 {
			t.Fatal("expected user sections to be populated")
		}
		first := pc.UserSections[0]
		if first.Title != SectionUserRequest {
			t.Errorf("first user section = %q, want %q", first.Title, SectionUserRequest)
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
		Mutable:       types.NewMutableState("test"),
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
		AgentName: types.AgentExplorer,
		Stage:     types.StageExplore,
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
	if !strings.Contains(userMsg, "## Knowledge & Evidence Pool") {
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

// TestFormatEvidenceItemsExcludesUngrounded pins the 2026-04-17
// redesign contract: items with GroundingStatus=Ungrounded, plus
// diagnostic-only dataflow items, are intentionally EXCLUDED from the
// Structured Evidence section and instead flow through
// formatUnverifiedLeads into a dedicated "Unverified Leads (not for
// citation)" section. This prevents the finalizer from citing claims
// the grounder could not validate or paths dataflow could not prove.
// Recovered items remain in Structured Evidence with a [recovered]
// tag so the LLM can tell "my line was right" from "the grounder
// adjusted my line".
func TestFormatEvidenceItemsExcludesUngrounded(t *testing.T) {
	items := []types.EvidenceItem{
		{
			Kind:            types.EvidenceConcrete,
			Subject:         "Handler.Name",
			Predicate:       "returns",
			Object:          "\"explorer\"",
			Source:          "internal/agent/explorer.go",
			LineStart:       18,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceConcrete,
			Subject:         "Register",
			Predicate:       "binds",
			Object:          "NewExplorerAgent",
			Source:          "internal/agent/registry.go",
			LineStart:       77,
			GroundingStatus: types.GroundingUngrounded,
		},
		{
			Kind:      types.EvidenceUnresolved,
			Subject:   "DynamicDispatch",
			Predicate: "unknown_effect",
			Object:    "target unresolved",
			Source:    "internal/agent/explorer.go",
			LineStart: 99,
		},
		{
			Kind:      types.EvidenceTruncated,
			Subject:   "candidate_files",
			Predicate: "truncated_to_budget",
			Object:    "20",
		},
	}
	out := formatEvidenceItems(items, 0, false)
	if !strings.Contains(out, "Handler.Name") {
		t.Fatalf("grounded item missing from Structured Evidence:\n%s", out)
	}
	if strings.Contains(out, "Register") {
		t.Fatalf("ungrounded item leaked into Structured Evidence; must go to Unverified Leads:\n%s", out)
	}
	if strings.Contains(out, "DynamicDispatch") || strings.Contains(out, "candidate_files") {
		t.Fatalf("diagnostic dataflow item leaked into Structured Evidence; must go to Unverified Leads:\n%s", out)
	}
	// Leads render is covered by formatUnverifiedLeads directly:
	leads := formatUnverifiedLeads(items, 0, false)
	if !strings.Contains(leads, "Register") {
		t.Fatalf("ungrounded item missing from Unverified Leads:\n%s", leads)
	}
	if !strings.Contains(leads, "DynamicDispatch") || !strings.Contains(leads, "[unresolved]") {
		t.Fatalf("unresolved diagnostic item missing from Unverified Leads:\n%s", leads)
	}
	if !strings.Contains(leads, "candidate_files") || !strings.Contains(leads, "[analysis_truncated]") {
		t.Fatalf("analysis_truncated diagnostic item missing from Unverified Leads:\n%s", leads)
	}
	if !strings.Contains(leads, "not for citation") && !strings.Contains(leads, "do NOT emit") {
		t.Fatalf("Unverified Leads section missing citation guard directive:\n%s", leads)
	}
}

func TestSelectEvidenceItemsForRenderPreservesProducerRankOrder(t *testing.T) {
	items := []types.EvidenceItem{
		{ID: "data-1", Producer: "dataflow.trace", Subject: "data-1"},
		{ID: "generic-1", Producer: "concrete_values", Subject: "generic-1"},
		{ID: "emit-1", Producer: "explorer.emit_evidence", Subject: "emit-1"},
		{ID: "generic-2", Producer: "bridge_literal", Subject: "generic-2"},
		{ID: "emit-2", Producer: "explorer.emit_evidence", Subject: "emit-2"},
	}
	got := selectEvidenceItemsForRender(items, 4)
	var ids []string
	for _, item := range got {
		ids = append(ids, item.ID)
	}
	want := []string{"emit-1", "emit-2", "generic-1", "generic-2"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("render selection order = %v, want %v", ids, want)
	}
	if items[0].ID != "data-1" || items[2].ID != "emit-1" {
		t.Fatalf("selection mutated input order: %+v", items)
	}
}

func TestBuildPromptContext_FinalizerStructuredEvidenceNeutralizesExactResolutionNotes(t *testing.T) {
	target := "explore_mid_loop_hint_budget"
	ac := &types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		Objective: "q",
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:         "config_mapping",
					ExactTargets: []string{target},
				},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{target},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceMechanism,
				Subject:         "DefaultExploreHeuristics",
				Predicate:       "explains",
				Object:          "nearby precedence baseline",
				Summary:         "This item names explore_mid_loop_hint_budget only in explanatory context; do NOT repair this item.",
				Source:          "internal/types/config.go",
				LineStart:       707,
				AnchorKind:      types.AnchorDefinition,
				AnchorSymbol:    "DefaultExploreHeuristics",
				ContextRole:     types.EvidenceContextRoleRelatedContext,
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	pc := BuildPromptContext(ac, &skill.Config{Name: "finalize-answer"})
	sec := findSectionTitle(pc, SectionEvidencePool)
	if sec == nil {
		t.Fatalf("missing Knowledge & Evidence Pool section")
	}
	if strings.Contains(strings.ToLower(sec.Content), "do not repair this item") {
		t.Fatalf("finalizer Structured Evidence should not carry operational repair notes:\n%s", sec.Content)
	}
	if strings.Contains(sec.Content, target) {
		t.Fatalf("finalizer Structured Evidence should neutralize repeated exact-target prose for nearby-context items:\n%s", sec.Content)
	}
	if !strings.Contains(sec.Content, "DefaultExploreHeuristics explains nearby precedence baseline") {
		t.Fatalf("finalizer Structured Evidence should keep the structural evidence claim:\n%s", sec.Content)
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

func TestBuildPromptContext_ConfigKeyAbsenceSection(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	ac := &types.AgentContext{
		AgentName: types.AgentExplorer,
		Stage:     types.StageExplore,
		Objective: "q",
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:            "config_mapping",
					PrimaryEntities: []string{missingKey},
					Entities:        []string{missingKey},
				},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:              types.SubjectConfigKey,
					TargetLabel:             "config key",
					Targets:                 []string{missingKey},
					AllowAbsence:            true,
					RequireTargetMention:    true,
					AliasRequiresProof:      true,
					RelatedContextPolicy:    types.ExactContextSameFamilyGrounded,
					RelatedContextScopeHint: "same namespace / prefix family",
				},
			},
		},
		UnverifiedAnalyzerFindings: []types.UnverifiedFinding{{
			Token:  missingKey,
			Kind:   "symbol",
			Reason: "symbol not found in graph",
		}},
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "explore-skill"})

	sec := findSectionTitle(pc, SectionExactResolution)
	if sec == nil {
		t.Fatalf("missing Exact Resolution section")
	}
	for _, want := range []string{missingKey, "Do not rename", "same namespace / prefix family", "absence_justification"} {
		if !strings.Contains(sec.Content, want) {
			t.Fatalf("section missing %q:\n%s", want, sec.Content)
		}
	}
}

func TestBuildAgentContext_MergesUnverifiedFindingsFromPriorStageReport(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:            "config_mapping",
					PrimaryEntities: []string{missingKey},
					Entities:        []string{missingKey},
				},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:              types.SubjectConfigKey,
					TargetLabel:             "config key",
					Targets:                 []string{missingKey},
					AllowAbsence:            true,
					RequireTargetMention:    true,
					AliasRequiresProof:      true,
					RelatedContextPolicy:    types.ExactContextSameFamilyGrounded,
					RelatedContextScopeHint: "same namespace / prefix family",
				},
			},
		},
		StageReports: []types.StageReport{{
			Stage:    types.StageAnalyze,
			Agent:    types.AgentAnalyzer,
			Findings: "The key ~~`" + missingKey + "`~~ [unverified: symbol not in repo graph] was not found exactly.",
		}},
	}
	ac := BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	pc := BuildPromptContext(ac, &skill.Config{Name: "explore-skill"})

	if len(ac.UnverifiedAnalyzerFindings) != 1 {
		t.Fatalf("unverified findings = %d, want 1", len(ac.UnverifiedAnalyzerFindings))
	}
	sec := findSectionTitle(pc, SectionExactResolution)
	if sec == nil {
		t.Fatalf("prior stage unverified annotation should render exact-resolution section")
	}
	if !strings.Contains(sec.Content, missingKey) {
		t.Fatalf("absence section missing key:\n%s", sec.Content)
	}
}

func TestBuildPromptContext_AnswerSymbols_Complete_RendersTranslationMode(t *testing.T) {
	syms := []types.AnswerSymbol{
		{Name: "Foo", File: "a.go", Line: 10, Kind: "registration"},
		{Name: "Bar", File: "b.go", Line: 20, Kind: "registration"},
	}
	ac := buildAgentCtxWithSymbols(syms, types.CompletenessComplete)
	pc := BuildPromptContext(ac, &skill.Config{Name: "s"})

	sec := findSectionTitle(pc, SectionAnswerSymbolsAuth)
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
	if findSectionTitle(pc, SectionAnswerSymbolsFloor) != nil {
		t.Error("Complete branch must not emit the lower-bound section title")
	}
}

func TestBuildPromptContext_AnswerSymbols_LowerBound_RendersSoftenedFloor(t *testing.T) {
	syms := []types.AnswerSymbol{
		{Name: "Foo", File: "a.go", Line: 10, Kind: "registration"},
	}
	ac := buildAgentCtxWithSymbols(syms, types.CompletenessLowerBound)
	pc := BuildPromptContext(ac, &skill.Config{Name: "s"})

	sec := findSectionTitle(pc, SectionAnswerSymbolsFloor)
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
	if findSectionTitle(pc, SectionAnswerSymbolsAuth) != nil {
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

	if findSectionTitle(pc, SectionAnswerSymbolsAuth) != nil {
		t.Error("Unknown claim must not render the Translation-mode section")
	}
	if findSectionTitle(pc, SectionAnswerSymbolsFloor) != nil {
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
		if findSectionTitle(pc, SectionAnswerSymbolsAuth) != nil {
			t.Errorf("claim=%q with empty slate must drop Translation section", claim)
		}
		if findSectionTitle(pc, SectionAnswerSymbolsFloor) != nil {
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
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		Objective: "q",
		Mutable:   mu,
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "final-answer-skill"})

	sec := findSectionTitle(pc, SectionHypothesisVerdicts)
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
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		Objective: "q",
		Mutable:   mu,
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "s"})
	if findSectionTitle(pc, SectionHypothesisVerdicts) != nil {
		t.Error("empty verdict buffer must not produce the section")
	}
}

func TestBuildPromptContext_SkipsHypothesisVerdictsWhenMutableNil(t *testing.T) {
	ac := &types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		Objective: "q",
		Mutable:   nil,
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "s"})
	if findSectionTitle(pc, SectionHypothesisVerdicts) != nil {
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

// -----------------------------------------------------------------------------
// Analyzer scenario — regression fences for the Skill/Evaluator boundary
// -----------------------------------------------------------------------------
//
// These tests assert the builder side of the analyzer prompt contract
// that the agent package asserts from its side in
// internal/agent/analyzer_prompt_test.go. Together they pin a bright
// line between the two layers:
//
//   - This file (context package) verifies BuildPromptContext renders
//     the complete analysis-skill system block (Skill Goal / Workflow
//     / Output Format / Prohibitions) plus every builder-owned
//     dynamic section (User Request / User Preferences / Retry
//     Directive) for a populated AgentContext, and renders each
//     section exactly once.
//   - analyzer_prompt_test.go verifies analyzerEvaluator.BuildInitialInstruction
//     returns no content from the removed hardcoded prompt and does
//     not re-emit any canonical section header.
//
// If either side fails, the boundary rule documented at
// docs/architecture.md §3.3 has been violated.

// TestBuildPromptContext_AnalysisSkill_RendersAllSections assembles
// the full analyze-stage prompt from the real analysis-skill and a
// populated AgentContext, then asserts every section the boundary
// rule allocates to the builder is present and carries the expected
// content. It catches the "nobody noticed the system block lost a
// section" regression from both sides:
//
//   - a stripped analysis-skill (missing Goal / Workflow / ...) fails
//     the system-section checks.
//   - a stripped BuildPromptContext (forgot to render RetryHint)
//     fails the user-section checks.
func TestBuildPromptContext_AnalysisSkill_RendersAllSections(t *testing.T) {
	const (
		objectivePin  = "SENTINEL_ANALYZER_OBJECTIVE: trace the analyze dispatch end-to-end"
		preferencePin = "SENTINEL_ANALYZER_PREFERENCE: respond to the user in English"
		retryPin      = "SENTINEL_ANALYZER_RETRY: previous dispatch emitted 0 times; retry"
	)

	ac := &types.AgentContext{
		AgentName:   types.AgentAnalyzer,
		Stage:       types.StageAnalyze,
		Objective:   objectivePin,
		Preferences: []string{preferencePin},
		RetryHint:   retryPin,
	}
	sk := skill.BuildAnalysisSkill()

	pc := BuildPromptContext(ac, sk)

	// --- static skill sections (system side) ---
	requiredSystemSections := []string{
		SectionAgentIdentity,
		SectionSkillGoal,
		SectionWorkflow,
		SectionOutputFormat,
		SectionProhibitions,
		SectionUserPreferences,
	}
	for _, title := range requiredSystemSections {
		if findSystemSection(pc, title) == nil {
			t.Errorf("system missing section %q; builder dropped a skill-owned or runtime section", title)
		}
	}

	// --- dynamic builder sections (user side) ---
	if sec := findSectionTitle(pc, SectionUserRequest); sec == nil {
		t.Error("user sections missing User Request")
	} else if !strings.Contains(sec.Content, objectivePin) {
		t.Errorf("User Request missing objective sentinel %q, got:\n%s", objectivePin, sec.Content)
	}
	if sec := findSectionTitle(pc, SectionRetryDirective); sec == nil {
		t.Error("user sections missing Retry Directive")
	} else if !strings.Contains(sec.Content, retryPin) {
		t.Errorf("Retry Directive missing retry sentinel %q, got:\n%s", retryPin, sec.Content)
	}
	if sec := findSystemSection(pc, SectionUserPreferences); sec == nil {
		t.Error("system sections missing User Preferences")
	} else if !strings.Contains(sec.Content, preferencePin) {
		t.Errorf("User Preferences missing preference sentinel %q, got:\n%s", preferencePin, sec.Content)
	}

	// --- SSOT integrity: the Output Format block must carry every
	// enum-table header renderEnumTable emits. If this fails, the
	// analysis-skill lost its classification content and the LLM
	// sees neither side. ---
	of := findSystemSection(pc, SectionOutputFormat)
	if of == nil {
		t.Fatal("Output Format section missing")
	}
	for _, field := range []string{
		"intent",
		"scenario",
		"complexity",
		"question_kind",
	} {
		header := field + " — pick one:"
		if !strings.Contains(of.Content, header) {
			t.Errorf("Output Format missing enum-table header %q — SSOT is incomplete", header)
		}
	}

	// Tool suggestions scope the analyzer's LLM to the evidence-lite
	// pre-scan surface: emit_analysis (exit channel) plus three
	// read-only navigation tools (repo_map, grep, list_files). A
	// heavier tool appearing here — most importantly read_file or
	// exec_command — means the skill's ToolSuggestions leaked past
	// the evidence-lite boundary.
	wantEnabled := map[string]bool{
		"emit_analysis": true,
		"repo_map":      true,
		"grep":          true,
		"list_files":    true,
	}
	if len(pc.EnabledTools) != len(wantEnabled) {
		t.Errorf("EnabledTools = %v, want exactly %d entries", pc.EnabledTools, len(wantEnabled))
	}
	for _, name := range pc.EnabledTools {
		if !wantEnabled[name] {
			t.Errorf("EnabledTools contains unexpected %q", name)
		}
	}
	for name := range wantEnabled {
		found := false
		for _, got := range pc.EnabledTools {
			if got == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("EnabledTools missing %q", name)
		}
	}
}

// TestBuildPromptContext_AnalysisSkill_NoDuplicateSections asserts
// each skill-owned section renders exactly once in the system block.
// A second copy of e.g. Workflow or Output Format would be a
// builder-side bug that nobody would catch from the analyzer tests
// alone — the only way to see it is to count titles on this side of
// the boundary.
func TestBuildPromptContext_AnalysisSkill_NoDuplicateSections(t *testing.T) {
	ac := &types.AgentContext{
		AgentName:   types.AgentAnalyzer,
		Stage:       types.StageAnalyze,
		Objective:   "pin no-duplication",
		Preferences: []string{"Respond in English."},
	}
	sk := skill.BuildAnalysisSkill()
	pc := BuildPromptContext(ac, sk)

	systemCounts := make(map[string]int, len(pc.SystemSections))
	for _, s := range pc.SystemSections {
		systemCounts[s.Title]++
	}
	for _, title := range []string{
		SectionAgentIdentity,
		SectionReasoningHygiene,
		SectionUserPreferences,
		SectionSkillGoal,
		SectionWorkflow,
		SectionOutputFormat,
		SectionProhibitions,
	} {
		if c := systemCounts[title]; c != 1 {
			t.Errorf("system section %q rendered %d times, want 1", title, c)
		}
	}

	userCounts := make(map[string]int, len(pc.UserSections))
	for _, s := range pc.UserSections {
		userCounts[s.Title]++
	}
	for _, title := range []string{SectionUserRequest} {
		if c := userCounts[title]; c != 1 {
			t.Errorf("user section %q rendered %d times, want 1", title, c)
		}
	}
}

// TestBuildPromptContext_AnalysisSkill_NoStaticContractPhrases mirrors
// analyzer_prompt_test.go's banned-phrase list from the BUILDER side:
// the old hardcoded BuildInitialInstruction text should be nowhere in the
// rendered system/user message. If a future commit puts e.g. "ERM
// predicate selector" into the analysis-skill's Goal or a new
// builder section, this test will fail here even if the analyzer
// package-level test does not — the boundary has two faces and each
// side owns its own guard.
//
// IMPORTANT: this guard asserts the phrases are absent from BOTH the
// SKILL fields (Goal / Workflow / OutputFormat / Prohibitions) AND
// the BUILDER-added sections (Agent Identity, Reasoning Hygiene, ...).
// The skill's renderEnumTable emits lowercase `"<field> — pick one:"`
// (different from the old uppercase "— the task intent. Pick one:")
// so the banned phrases do not collide with the legitimate
// SSOT content.
func TestBuildPromptContext_AnalysisSkill_NoStaticContractPhrases(t *testing.T) {
	ac := &types.AgentContext{
		AgentName: types.AgentAnalyzer,
		Stage:     types.StageAnalyze,
		Objective: "assert distinctive phrases are absent",
	}
	sk := skill.BuildAnalysisSkill()
	pc := BuildPromptContext(ac, sk)
	msgs := ToMessages(pc)

	banned := []string{
		"Required emit_analysis fields",
		"ERM predicate selector",
		"anti-hallucination selector",
		"task intent. Pick one",
		"predicate whitelist",
	}

	for _, m := range msgs {
		for _, phrase := range banned {
			if strings.Contains(m.Content, phrase) {
				t.Errorf("rendered %s message contains banned phrase %q — "+
					"static contract text crept back via the skill or a new builder section",
					m.Role, phrase)
			}
		}
	}
}

// findSystemSection is the system-side analogue of findSectionTitle
// (which only scans UserSections). Both helpers return nil when the
// requested title is absent so the caller can distinguish "section
// missing" from "section present but wrong content".
func findSystemSection(pc *types.PromptContext, title string) *types.PromptSection {
	for i := range pc.SystemSections {
		if pc.SystemSections[i].Title == title {
			return &pc.SystemSections[i]
		}
	}
	return nil
}

// -----------------------------------------------------------------------------
// Capability-aware reasoning hygiene tests
// -----------------------------------------------------------------------------
//
// reasoningHygieneFor used to be a constant that mentioned exec_command
// and shell pipelines unconditionally — which told the analyzer, the
// extractor, and the finalizer to "run `find | wc -l`" even though
// their skill allowlists physically block exec_command. The three
// stages would occasionally attempt the tool call, the schema set
// would reject it, and the dispatch wasted a round. Making the hint
// capability-aware means each stage only ever sees advice that
// names a tool it can actually invoke.
//
// These tests pin the capability matrix:
//
//     explore-skill   → exec_command present      → shell variant
//     analysis-skill       → grep/list_files/repo_map  → read-only variant
//     extract-skill        → emit_* only               → no-tool variant
//     answer-document-skill → emit_* only              → no-tool variant
//
// A future skill-level refactor that changes the allowlist should
// fail here if the new allowlist mismatches the variant.

// reasoningHygieneSectionOf reaches into BuildPromptContext's output
// and returns the Reasoning Hygiene section body, or the empty
// string when the section is missing. Helper so every test below
// can stay one line.
func reasoningHygieneSectionOf(t *testing.T, sk *skill.Config) string {
	t.Helper()
	ac := &types.AgentContext{
		AgentName: types.AgentName("test"),
		Stage:     types.StageExplore,
		Objective: "capability matrix probe",
	}
	pc := BuildPromptContext(ac, sk)
	sec := findSystemSection(pc, SectionReasoningHygiene)
	if sec == nil {
		t.Fatal("Reasoning Hygiene section missing")
	}
	return sec.Content
}

// TestReasoningHygiene_ExplorerGetsShellVariant pins the historical
// shell-pipeline advice for the explorer. The explorer owns
// exec_command, so the distinctive phrase `find ... | wc -l`
// (plus the word exec_command) should appear in its hygiene block.
func TestReasoningHygiene_ExplorerGetsShellVariant(t *testing.T) {
	sk := &skill.Config{
		Name: "explore-skill",
		ToolSuggestions: []string{
			"repo_map", "grep", "read_file", "list_files", "exec_command",
		},
	}
	body := reasoningHygieneSectionOf(t, sk)

	if !strings.Contains(body, "exec_command") {
		t.Errorf("explorer hygiene should mention exec_command, got:\n%s", body)
	}
	if !strings.Contains(body, "`find ... | wc -l`") {
		t.Errorf("explorer hygiene should suggest a shell pipeline, got:\n%s", body)
	}
}

// TestReasoningHygiene_AnalyzerGetsReadOnlyVariant pins the analyzer
// capability profile. analysis-skill has grep / list_files /
// repo_map but NOT exec_command, so the hint must:
//
//   - list only the tools the analyzer actually has
//   - NOT name exec_command anywhere (would be an unavailable-tool
//     suggestion and waste a tool-call turn)
func TestReasoningHygiene_AnalyzerGetsReadOnlyVariant(t *testing.T) {
	sk := skill.BuildAnalysisSkill()
	body := reasoningHygieneSectionOf(t, sk)

	if strings.Contains(body, "exec_command") {
		t.Errorf("analyzer hygiene must NOT mention exec_command (not in its allowlist), got:\n%s", body)
	}
	if strings.Contains(body, "`find ... | wc -l`") {
		t.Errorf("analyzer hygiene must NOT suggest shell pipelines, got:\n%s", body)
	}
	// Positive: the three navigation tools the analyzer DOES have
	// should be named so the hint is actionable.
	for _, name := range []string{"grep", "list_files", "repo_map"} {
		if !strings.Contains(body, name) {
			t.Errorf("analyzer hygiene should mention %q, got:\n%s", name, body)
		}
	}
	// read_file is NOT in the analyzer's allowlist (the evidence-
	// lite boundary) and must NOT be named.
	if strings.Contains(body, "read_file") {
		t.Errorf("analyzer hygiene must NOT mention read_file (not in its allowlist), got:\n%s", body)
	}
}

// TestReasoningHygiene_ExtractorGetsNoToolVariant pins the
// extract-skill (Turn B) capability profile. The extractor has
// zero read/compute tools — only emit_answer_symbol +
// emit_hypothesis_verdict — so the hint must instruct the LLM to
// consume upstream evidence without naming any read-tool or shell
// pipeline.
func TestReasoningHygiene_ExtractorGetsNoToolVariant(t *testing.T) {
	sk := &skill.Config{
		Name: "extract-skill",
		ToolSuggestions: []string{
			"emit_answer_symbol",
			"emit_hypothesis_verdict",
		},
	}
	body := reasoningHygieneSectionOf(t, sk)

	// Negative assertions: none of the heavy / read tools should
	// appear in the hint because the extractor cannot call any of
	// them. "do NOT have permission" is fine prose; bare tool names
	// are the risk.
	for _, forbidden := range []string{
		"exec_command",
		"`find",
		"grep ", // trailing space avoids matching "grep match count" in another variant
		"list_files",
		"read_file",
		"repo_map",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("extractor hygiene must NOT name unavailable tool %q, got:\n%s", forbidden, body)
		}
	}
	// Positive: the hint should frame the stage as "consume
	// upstream evidence" rather than "run a tool".
	if !strings.Contains(body, "upstream evidence") {
		t.Errorf("extractor hygiene should reframe around upstream evidence, got:\n%s", body)
	}
}

// TestReasoningHygiene_FinalizerGetsNoToolVariant mirrors the
// extractor check for answer-document-skill. The finalizer has
// only emit_answer_document in its allowlist.
func TestReasoningHygiene_FinalizerGetsNoToolVariant(t *testing.T) {
	sk := &skill.Config{
		Name:            "answer-document-skill",
		ToolSuggestions: []string{"emit_answer_document"},
	}
	body := reasoningHygieneSectionOf(t, sk)

	for _, forbidden := range []string{
		"exec_command",
		"`find",
		"list_files",
		"read_file",
		"repo_map",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("finalizer hygiene must NOT name unavailable tool %q, got:\n%s", forbidden, body)
		}
	}
	if !strings.Contains(body, "upstream evidence") {
		t.Errorf("finalizer hygiene should reframe around upstream evidence, got:\n%s", body)
	}
}

// TestReasoningHygiene_NilSkillFallsBackSafely exercises the
// defensive path for a nil skill.Config. The builder is invoked
// directly from a handful of fixture-heavy tests with partially
// zeroed inputs, and the function must not panic; it should
// return the no-tool variant since no allowlist means no
// capability.
func TestReasoningHygiene_NilSkillFallsBackSafely(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("reasoningHygieneFor(nil) panicked: %v", r)
		}
	}()
	body := reasoningHygieneFor(nil)
	if strings.Contains(body, "exec_command") {
		t.Errorf("nil skill must NOT produce shell variant, got:\n%s", body)
	}
	if body == "" {
		t.Error("nil skill should still produce a non-empty hygiene body")
	}
}

// -----------------------------------------------------------------------------
// Extract-skill prompt trim: Known Facts + Structured Evidence dropped
// -----------------------------------------------------------------------------
//
// Rationale: the extractor has no investigation tools (grep /
// read_file / repo_map / list_files / exec_command are all outside
// its ToolSuggestions), so raw tool dumps and the full evidence list
// are inert. The extractor's BuildInitialInstruction appends the
// accepted Turn A digest; StageReports are suppressed when that digest
// is present so parallel exploration reports cannot compete with the
// structured handoff. Three tests below pin the gate on the skill-name
// axis so a rename of either skill breaks them loudly.

func extractorSkill() *skill.Config {
	return &skill.Config{
		Name:            "extract-skill",
		ToolSuggestions: []string{"emit_answer_symbol", "emit_hypothesis_verdict"},
	}
}

func finalizerSkill() *skill.Config {
	return &skill.Config{
		Name:            "answer-document-skill",
		ToolSuggestions: []string{"emit_answer_document"},
	}
}

func explorerSkill() *skill.Config {
	return &skill.Config{
		Name: "explore-skill",
		ToolSuggestions: []string{
			"repo_map", "grep", "read_file", "list_files", "exec_command",
		},
	}
}

func acWithFactsAndEvidence() *types.AgentContext {
	return &types.AgentContext{
		AgentName: types.AgentExtractor,
		Stage:     types.StageExtract,
		Objective: "q",
		RelevantFacts: []string{
			"[grep] grep = [grep: 42 matching files]",
			"[read_file] read_file = [internal/agent/agent.go: showing lines 1-100 of 1127 total]",
		},
		EvidenceItems: []types.EvidenceItem{
			{ID: "e1", Kind: types.EvidenceDirect, Subject: "A", Predicate: "binds", Object: "B", Source: "a.go", LineStart: 1},
			{ID: "e2", Kind: types.EvidenceRegistration, Subject: "C", Predicate: "registers", Object: "D", Source: "b.go", LineStart: 2},
		},
	}
}

func TestBuildPromptContext_ExtractSkill_SkipsKnownFactsAndStructuredEvidence(t *testing.T) {
	ac := acWithFactsAndEvidence()
	pc := BuildPromptContext(ac, extractorSkill())
	if findSectionTitle(pc, SectionKnownFacts) != nil {
		t.Error("extract-skill must drop Known Facts (tool dumps the extractor cannot act on)")
	}
	if findSectionTitle(pc, SectionEvidencePool) != nil {
		t.Error("extract-skill must drop Structured Evidence (duplicates Primary Evidence + Turn A digest)")
	}
}

func TestStageReportsForExtractorSuppressedWhenTurnADigestPresent(t *testing.T) {
	mu := types.NewMutableState("q")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		AcceptedClosureReason: "resolved from structured handoff",
		EvidenceItems: []types.EvidenceItem{{
			Kind:       types.EvidenceDirect,
			Source:     "a.go",
			LineStart:  1,
			AnchorKind: types.AnchorDefinition,
		}},
	})
	ac := &types.AgentContext{
		AgentName: types.AgentExtractor,
		Stage:     types.StageExtract,
		Mutable:   mu,
	}
	reports := []types.StageReport{
		{Stage: types.StageExplore, Agent: types.AgentExplorer, Findings: "## Primary Evidence\n- stale branch"},
		{Stage: types.StageExplore, Agent: types.AgentExplorer, Findings: "## Primary Evidence\n- sibling branch"},
	}
	if got := stageReportsForAgent(reports, ac); len(got) != 0 {
		t.Fatalf("extractor should trust Turn A digest instead of duplicate StageReports, got %+v", got)
	}
}

func TestStageReportsForAgent_DropsCurrentStageAndDedupesPriorReports(t *testing.T) {
	ac := &types.AgentContext{
		AgentName: types.AgentExplorer,
		Stage:     types.StageExplore,
	}
	reports := []types.StageReport{
		{Stage: types.StageAnalyze, Agent: types.AgentAnalyzer, Findings: "analysis result"},
		{Stage: types.StageAnalyze, Agent: types.AgentAnalyzer, Findings: "analysis result"},
		{Stage: types.StageAnalyze, Agent: types.AgentAnalyzer, Findings: " \n\t "},
		{Stage: types.StageExplore, Agent: types.AgentExplorer, Findings: "same-stage explore report"},
		{Stage: types.StageLogTriage, Agent: types.AgentLogTriager, Findings: "log pre-stage report"},
	}
	got := stageReportsForAgent(reports, ac)
	if len(got) != 2 {
		t.Fatalf("stageReportsForAgent len=%d, want 2; got %+v", len(got), got)
	}
	if got[0].Agent != types.AgentAnalyzer || got[0].Findings != "analysis result" {
		t.Fatalf("first retained report = %+v, want one analyzer analysis result", got[0])
	}
	if got[1].Agent != types.AgentLogTriager || got[1].Findings != "log pre-stage report" {
		t.Fatalf("second retained report = %+v, want log triage pre-stage", got[1])
	}
	out := formatStageReports(got)
	if strings.Count(out, "### Prior request analysis result") != 1 {
		t.Fatalf("deduped prompt should contain one analyzer heading, got:\n%s", out)
	}
	if strings.Contains(out, "same-stage explore report") {
		t.Fatalf("same-stage explorer report leaked into explorer prompt:\n%s", out)
	}
}

func TestStageReportsForAgent_AnalyzerRetryDoesNotReadOwnReport(t *testing.T) {
	ac := &types.AgentContext{
		AgentName: types.AgentAnalyzer,
		Stage:     types.StageAnalyze,
	}
	reports := []types.StageReport{
		{Stage: types.StageAnalyze, Agent: types.AgentAnalyzer, Findings: "previous failed analyzer summary"},
		{Stage: types.StageLogTriage, Agent: types.AgentLogTriager, Findings: "log pre-stage report"},
	}
	got := stageReportsForAgent(reports, ac)
	if len(got) != 1 || got[0].Agent != types.AgentLogTriager {
		t.Fatalf("analyzer retry should only keep true pre-stage reports, got %+v", got)
	}
}

func TestBuildPromptContext_FinalizerSkill_KeepsKnownFactsAndStructuredEvidence(t *testing.T) {
	ac := acWithFactsAndEvidence()
	ac.AgentName = types.AgentFinalizer
	ac.Stage = types.StageFinalize
	pc := BuildPromptContext(ac, finalizerSkill())
	if findSectionTitle(pc, SectionKnownFacts) == nil {
		t.Error("finalizer still needs Known Facts (the trim targets extract-skill only)")
	}
	if findSectionTitle(pc, SectionEvidencePool) == nil {
		t.Error("finalizer needs Structured Evidence for citation pool coverage")
	}
}

func TestBuildPromptContext_FinalizerSkill_SkipsDuplicateDataflowFindings(t *testing.T) {
	ac := acWithFactsAndEvidence()
	ac.AgentName = types.AgentFinalizer
	ac.Stage = types.StageFinalize
	ac.PriorReports = []types.StageReport{
		{
			Stage:    types.StageExplore,
			Agent:    types.AgentExplorer,
			Findings: "## Dataflow Findings\n- prior path",
		},
	}
	ac.FlowFindings = []types.FlowFindingDigest{
		{
			Path:       []string{"A", "B"},
			Sources:    []string{"A"},
			Sinks:      []string{"B"},
			Confidence: 0.8,
		},
	}

	pc := BuildPromptContext(ac, finalizerSkill())
	if findSectionTitle(pc, SectionPriorStageFindings) == nil {
		t.Fatal("test setup should render prior reports")
	}
	if findSectionTitle(pc, SectionDataflowFindings) != nil {
		t.Fatal("finalizer should not render a second standalone Dataflow Findings section when prior reports already contain one")
	}
}

func TestBuildPromptContext_ExplorerSkill_KeepsBothSections(t *testing.T) {
	ac := acWithFactsAndEvidence()
	ac.AgentName = types.AgentExplorer
	ac.Stage = types.StageExplore
	pc := BuildPromptContext(ac, explorerSkill())
	if findSectionTitle(pc, SectionKnownFacts) == nil {
		t.Error("explore-skill must keep Known Facts (explorer is the producer but may iterate)")
	}
	if findSectionTitle(pc, SectionEvidencePool) == nil {
		t.Error("explore-skill must keep Structured Evidence")
	}
}

func TestBuildPromptContext_ExplorerSkill_UsesExplorationContractTitle(t *testing.T) {
	pc := BuildPromptContext(&types.AgentContext{
		AgentName: types.AgentExplorer,
		Stage:     types.StageExplore,
		Objective: "q",
	}, explorerSkill())

	if findSystemSection(pc, SectionExplorationContract) == nil {
		t.Fatal("explore-skill must render Exploration Contract")
	}
	if findSystemSection(pc, SectionOutputFormat) != nil {
		t.Fatal("explore-skill must not render the generic Output Format title")
	}
}

func TestIsExtractorSkill_Table(t *testing.T) {
	cases := []struct {
		name string
		sk   *skill.Config
		want bool
	}{
		{"nil skill", nil, false},
		{"extract-skill", &skill.Config{Name: "extract-skill"}, true},
		{"answer-document-skill", &skill.Config{Name: "answer-document-skill"}, false},
		{"explore-skill", &skill.Config{Name: "explore-skill"}, false},
		{"analysis-skill", &skill.Config{Name: "analysis-skill"}, false},
		{"unknown skill name", &skill.Config{Name: "mystery"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isExtractorSkill(c.sk); got != c.want {
				t.Errorf("isExtractorSkill(%q) = %v, want %v", skillName(c.sk), got, c.want)
			}
		})
	}
}

func skillName(sk *skill.Config) string {
	if sk == nil {
		return "<nil>"
	}
	return sk.Name
}

// TestReasoningHygiene_EveryStageProducesNonEmpty is a sanity guard
// against a future variant picker that returns "" for an unhandled
// combination. Every skill configuration in the codebase must yield
// non-empty hygiene text.
func TestReasoningHygiene_EveryStageProducesNonEmpty(t *testing.T) {
	cases := []struct {
		name string
		sk   *skill.Config
	}{
		{"explorer", &skill.Config{Name: "explore-skill", ToolSuggestions: []string{
			"repo_map", "grep", "read_file", "list_files", "exec_command",
		}}},
		{"analyzer", skill.BuildAnalysisSkill()},
		{"extractor", &skill.Config{Name: "extract-skill", ToolSuggestions: []string{
			"emit_answer_symbol", "emit_hypothesis_verdict",
		}}},
		{"finalizer", &skill.Config{Name: "answer-document-skill", ToolSuggestions: []string{
			"emit_answer_document",
		}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := reasoningHygieneFor(c.sk)
			if strings.TrimSpace(body) == "" {
				t.Errorf("%s produced empty hygiene", c.name)
			}
		})
	}
}

// measurementScalarIR builds a minimal AnalysisIR with the exact
// AnswerContract state that analyzer.buildAnalysisIR's citation-free
// value carve-out sets: ShapeValue + CitationReq disabled. Used by
// the Raw Tool Outputs tests to mirror upstream state without pulling
// in the full analyzer build pipeline.
func measurementScalarIR() *types.AnalysisIR {
	return &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Predicates: types.SemanticPredicates{IsScalarAnswer: true},
		},
		AnswerContract: types.AnswerContract{
			CitationReq: types.CitationReq{Required: false, MinCitations: 0},
		},
	}
}

// explanationIR is the negative-control IR: ShapeExplanation + a
// normal citation floor. Raw Tool Outputs must NEVER render for this
// shape — the finalizer must rely on structured evidence, not on raw
// tool dumps that would pollute explain-class answers.
func explanationIR() *types.AnalysisIR {
	return &types.AnalysisIR{
		AnswerContract: types.AnswerContract{
			CitationReq: types.CitationReq{Required: true, MinCitations: 3, Granularity: "file_line"},
		},
	}
}

// TestShouldRenderRawToolOutputs_Gate covers all discriminator paths
// so a future change to the measurement-scalar carve-out (or an
// accidental widening of CitationReq semantics) is caught as a
// regression rather than a silent behaviour drift.
func TestShouldRenderRawToolOutputs_Gate(t *testing.T) {
	mu := types.NewMutableState("q")
	cases := []struct {
		name string
		ctx  *types.AgentContext
		want bool
	}{
		{"extract + measurement-scalar IR renders",
			&types.AgentContext{Stage: types.StageExtract, Mutable: mu, AnalysisIR: measurementScalarIR()}, true},
		{"finalize + measurement-scalar IR renders",
			&types.AgentContext{Stage: types.StageFinalize, Mutable: mu, AnalysisIR: measurementScalarIR()}, true},
		{"explore stage never renders",
			&types.AgentContext{Stage: types.StageExplore, Mutable: mu, AnalysisIR: measurementScalarIR()}, false},
		{"analyze stage never renders",
			&types.AgentContext{Stage: types.StageAnalyze, Mutable: mu, AnalysisIR: measurementScalarIR()}, false},
		{"explanation IR never renders (deep-investigation path)",
			&types.AgentContext{Stage: types.StageFinalize, Mutable: mu, AnalysisIR: explanationIR()}, false},
		{"value shape with citation still required does not render",
			&types.AgentContext{Stage: types.StageFinalize, Mutable: mu, AnalysisIR: &types.AnalysisIR{
				AnswerContract: types.AnswerContract{
					CitationReq: types.CitationReq{Required: true, MinCitations: 1},
				},
			}}, false},
		{"nil Mutable never renders",
			&types.AgentContext{Stage: types.StageFinalize, Mutable: nil, AnalysisIR: measurementScalarIR()}, false},
		{"nil AnalysisIR never renders",
			&types.AgentContext{Stage: types.StageFinalize, Mutable: mu, AnalysisIR: nil}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shouldRenderRawToolOutputs(c.ctx); got != c.want {
				t.Errorf("shouldRenderRawToolOutputs = %v, want %v", got, c.want)
			}
		})
	}
}

// TestFormatRawToolOutputs_PreservesTailForScalar is the load-bearing
// assertion: when `wc -l` ends with a "NNNN total" line at the BOTTOM
// of a long output, the preserved tail must include it. Regressing
// this loses the entire point of the Raw Tool Outputs section.
func TestFormatRawToolOutputs_PreservesTailForScalar(t *testing.T) {
	// 5000 bytes of per-file counts + the total marker at the end.
	var long strings.Builder
	for i := 0; i < 180; i++ {
		fmt.Fprintf(&long, "    %3d ./internal/pkg%03d/file.go\n", i*20, i)
	}
	long.WriteString("  73396 total\n")

	got := formatRawToolOutputs([]types.ToolResult{
		{ToolName: "exec_command", Success: true, Summary: long.String()},
	})
	if got == "" {
		t.Fatal("rendered empty section")
	}
	if !strings.Contains(got, "73396 total") {
		t.Errorf("rendered section missing the tail 'total' marker — tail trim broke:\n%s", got)
	}
	if !strings.Contains(got, "exec_command") {
		t.Errorf("rendered section missing tool name header:\n%s", got)
	}
	if !strings.Contains(got, "trimmed") {
		t.Errorf("long output should carry a trimmed marker:\n%s", got)
	}
}

// TestFormatRawToolOutputs_SkipsEmitTools verifies that emit_* tool
// results are never rendered — they duplicate signal already present
// in structured channels (Evidence / Answer Symbols / Hypothesis
// Verdicts / Answer Document).
func TestFormatRawToolOutputs_SkipsEmitTools(t *testing.T) {
	results := []types.ToolResult{
		{ToolName: "emit_evidence", Success: true, Summary: "emit_evidence accepted 3 item(s)"},
		{ToolName: "emit_answer_symbol", Success: true, Summary: "emit_answer_symbol accepted 5 item(s)"},
		{ToolName: "emit_hypothesis_verdict", Success: true, Summary: "verdicts applied"},
		{ToolName: "emit_answer_document", Success: true, Summary: "answer document stored"},
		{ToolName: "emit_investigation_complete", Success: true, Summary: "complete"},
		{ToolName: "emit_analysis", Success: true, Summary: "analysis emitted"},
		{ToolName: "repo_map", Success: true, Summary: "repo_map: 230 files indexed"},
	}
	got := formatRawToolOutputs(results)
	if got != "" {
		t.Errorf("section should be empty when only emit_* + repo_map results exist, got:\n%s", got)
	}
}

// TestFormatRawToolOutputs_SkipsFailedResults prevents error noise
// from flooding the section. Failed tool calls are usually retried
// with a different invocation; their error Summary is not a scalar
// the finalizer should quote.
func TestFormatRawToolOutputs_SkipsFailedResults(t *testing.T) {
	got := formatRawToolOutputs([]types.ToolResult{
		{ToolName: "exec_command", Success: false, Summary: "command failed: exit 1"},
	})
	if got != "" {
		t.Errorf("failed tool results should be skipped, got:\n%s", got)
	}
}

// TestFormatRawToolOutputs_TotalCapWithMarker verifies the total-bytes
// cap engages and the trailing "... more omitted" marker appears when
// the section would otherwise balloon the prompt.
func TestFormatRawToolOutputs_TotalCapWithMarker(t *testing.T) {
	// Build many successful tool results each large enough that the
	// total cap trips before all render.
	results := make([]types.ToolResult, 0, 20)
	for i := 0; i < 20; i++ {
		results = append(results, types.ToolResult{
			ToolName: "exec_command",
			Success:  true,
			// 1500 bytes: above per-call head (800) + tail (400) + the
			// fmt envelope ("- **exec_command** ..."); so each rendered
			// chunk is ~1300 bytes.
			Summary: strings.Repeat("output line\n", 200),
		})
	}
	got := formatRawToolOutputs(results)
	if !strings.Contains(got, "more tool call(s) omitted") {
		t.Errorf("total cap marker missing when many huge results present:\n%s", got)
	}
	if len(got) > 2*rawToolOutputTotalCapBytes {
		t.Errorf("rendered section %d bytes, should be bounded near cap %d", len(got), rawToolOutputTotalCapBytes)
	}
}

func TestFormatRawToolOutputs_PreservesGitLogHistoryHead(t *testing.T) {
	summary := "[git_log: count=10 evidence_origin=vcs_metadata]\n" +
		"commit ae1dd6b256fab219104c09447b6ffe3697239b7a\n" +
		strings.Repeat("stat line for commit history\n", 120) +
		"commit 333707a65de3751cfd099cacd694550a29c7acd0\n"
	got := formatRawToolOutputs([]types.ToolResult{{
		ToolName: "git_log",
		Success:  true,
		Summary:  summary,
	}})
	if !strings.Contains(got, "commit ae1dd6b256fab219104c09447b6ffe3697239b7a") ||
		!strings.Contains(got, "commit 333707a65de3751cfd099cacd694550a29c7acd0") {
		t.Fatalf("git_log history output should preserve recent-N commit rows for finalizer enrichment:\n%s", got)
	}
	if strings.Contains(got, "...[trimmed") {
		t.Fatalf("moderate git_log history output should not be trimmed:\n%s", got)
	}
}

// TestBuildPromptContext_RawToolOutputs_WiredForMeasurementScalar is
// the end-to-end gate: for a count question IR, the Raw Tool Outputs
// section appears in the finalizer's prompt with the scalar tail
// visible.
func TestBuildPromptContext_RawToolOutputs_WiredForMeasurementScalar(t *testing.T) {
	mu := types.NewMutableState("how many .go files")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{
			{ToolName: "exec_command", Success: true, Summary: "   891 ./cmd/root.go\n   491 ./main.go\n  73396 total\n"},
		},
	})
	ac := &types.AgentContext{
		AgentName:  types.AgentFinalizer,
		Stage:      types.StageFinalize,
		Objective:  "how many .go files",
		Mutable:    mu,
		AnalysisIR: measurementScalarIR(),
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "answer-document-skill"})
	sec := findSectionTitle(pc, SectionRawToolOutputs)
	if sec == nil {
		t.Fatal("expected Raw Tool Outputs section to be present for measurement-scalar IR")
	}
	if !strings.Contains(sec.Content, "73396 total") {
		t.Errorf("section missing the scalar tail:\n%s", sec.Content)
	}
	if strings.Contains(sec.Content, "value{literal") {
		t.Errorf("raw tool output guidance must not teach retired V1 scalar payloads:\n%s", sec.Content)
	}
}

func TestBuildPromptContext_ToolSourcedValueGuidance_FinalizeAvoidsUnsupportedGroupCounts(t *testing.T) {
	ac := &types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		Objective: "最近 10 次提交都做了哪些事情，作用和影响分别是什么",
		Mutable:   types.NewMutableState("最近 10 次提交都做了哪些事情，作用和影响分别是什么"),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqHistory)},
			},
			AnswerContract: types.AnswerContract{CitationReq: types.CitationReq{Required: false}},
		},
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "answer-document-skill"})
	sec := findSectionTitle(pc, SectionToolSourcedValue)
	if sec == nil {
		t.Fatal("expected Tool-Sourced Value section to be present for VCS finalizer prompt")
	}
	if !strings.Contains(sec.Content, "Do not introduce module/component/category counts") ||
		!strings.Contains(sec.Content, "prefer qualitative grouping") ||
		!strings.Contains(sec.Content, "both the observed change and the effect/impact") {
		t.Fatalf("finalizer guidance should prevent unsupported VCS group counts, got:\n%s", sec.Content)
	}
}

func TestBuildPromptContext_ToolSourcedValueGuidance_RenderedForExplore(t *testing.T) {
	mu := types.NewMutableState("which commit introduced EvidenceClosure")
	ac := &types.AgentContext{
		AgentName:  types.AgentExplorer,
		Stage:      types.StageExplore,
		Objective:  "which commit introduced EvidenceClosure",
		Mutable:    mu,
		AnalysisIR: measurementScalarIR(),
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "explorer-skill"})
	sec := findSectionTitle(pc, SectionToolSourcedValue)
	if sec == nil {
		t.Fatal("expected Tool-Sourced Value section to be present for citation-free value IR")
	}
	if !strings.Contains(sec.Content, "tool-only investigation") {
		t.Fatalf("explore guidance should mention tool-only investigation, got:\n%s", sec.Content)
	}
}

func TestBuildPromptContext_ToolSourcedValueGuidance_PureHistoryDoesNotAskForReadFile(t *testing.T) {
	mu := types.NewMutableState("最近 10 次提交都做了什么")
	ac := &types.AgentContext{
		AgentName: types.AgentExplorer,
		Stage:     types.StageExplore,
		Objective: "最近 10 次提交都做了什么",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsHistoryLookup:       true,
					IsCategoryEnumeration: true,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqHistory)},
			},
			AnswerContract: types.AnswerContract{CitationReq: types.CitationReq{Required: false}},
		},
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "explorer-skill"})
	sec := findSectionTitle(pc, SectionToolSourcedValue)
	if sec == nil {
		t.Fatal("expected Tool-Sourced Value section for pure VCS history")
	}
	if !strings.Contains(sec.Content, "pure VCS-history handoff") ||
		!strings.Contains(sec.Content, "do not call `read_file` or `emit_evidence`") {
		t.Fatalf("pure history guidance should prevent current-source habit reads, got:\n%s", sec.Content)
	}
}

func TestBuildPromptContext_EvidenceOriginBoundary_RendersUpstreamOnly(t *testing.T) {
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqHistory)},
		},
		AnswerContract: types.AnswerContract{CitationReq: types.CitationReq{Required: false}},
	}
	for _, stage := range []types.PipelineStage{types.StageExplore, types.StageExtract} {
		ac := &types.AgentContext{
			AgentName:  types.AgentExplorer,
			Stage:      stage,
			Objective:  "最近 10 次提交都做了什么",
			Mutable:    types.NewMutableState("最近 10 次提交都做了什么"),
			AnalysisIR: ir,
		}
		pc := BuildPromptContext(ac, &skill.Config{Name: "test-skill"})
		sec := findSectionTitle(pc, SectionEvidenceOrigin)
		if sec == nil {
			t.Fatalf("stage %s should render Evidence Origin Boundary", stage)
		}
		for _, want := range []string{
			"Non-current-source facts are first-class evidence",
			"Do not convert VCS history",
			"fake current-source `emit_evidence` rows",
		} {
			if !strings.Contains(sec.Content, want) {
				t.Fatalf("stage %s evidence-origin section missing %q:\n%s", stage, want, sec.Content)
			}
		}
		if strings.Contains(sec.Content, "Mixed-origin lane plan") {
			t.Fatalf("pure VCS history should not render mixed-origin lane plan:\n%s", sec.Content)
		}
	}

	ac := &types.AgentContext{
		AgentName:  types.AgentFinalizer,
		Stage:      types.StageFinalize,
		Objective:  "最近 10 次提交都做了什么",
		Mutable:    types.NewMutableState("最近 10 次提交都做了什么"),
		AnalysisIR: ir,
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "finalize-answer"})
	if sec := findSectionTitle(pc, SectionEvidenceOrigin); sec != nil {
		t.Fatalf("finalizer must not get duplicate Evidence Origin Boundary from BuildPromptContext; evaluator renders the finalizer copy:\n%s", sec.Content)
	}
}

func TestBuildPromptContext_EvidenceOriginBoundary_MixedOriginLanePlan(t *testing.T) {
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Scenario: types.ScenarioArchitectureExplain,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
		},
		AnswerContract: types.AnswerContract{CitationReq: types.CitationReq{Required: false}},
	}
	ac := &types.AgentContext{
		AgentName:       types.AgentExplorer,
		Stage:           types.StageExplore,
		Objective:       "根据最近一次合入解释当前实现",
		Mutable:         types.NewMutableState("根据最近一次合入解释当前实现"),
		AnalysisIR:      ir,
		ExploreLanePlan: types.CompileExploreLanePlan(ir.RequestModel, &ir.AnswerContract, types.AnswerPresentationContract{}),
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "explorer-skill"})
	sec := findSectionTitle(pc, SectionEvidenceOrigin)
	if sec == nil {
		t.Fatal("mixed VCS/current-source request should render Evidence Origin Boundary")
	}
	for _, want := range []string{
		"Mixed-origin lane plan",
		"preserve its typed origin",
		"present-checkout implementation claims",
		"keep both summaries",
		"Typed explore lane ownership plan",
	} {
		if !strings.Contains(sec.Content, want) {
			t.Fatalf("mixed-origin section missing %q:\n%s", want, sec.Content)
		}
	}
	if !strings.Contains(sec.Content, "current_source") || !strings.Contains(sec.Content, "vcs_metadata") {
		t.Fatalf("mixed-origin section should show both evidence lanes:\n%s", sec.Content)
	}
}

func TestBuildPromptContext_ExploreLanePlan_RendersForCurrentSourceBuckets(t *testing.T) {
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			Buckets: []types.QuestionBucket{
				{Label: "explorer", Index: 1},
				{Label: "subagent", Index: 2},
			},
		},
		AnswerContract: types.AnswerContract{CitationReq: types.CitationReq{Required: true}},
	}
	ac := &types.AgentContext{
		AgentName:       types.AgentExplorer,
		Stage:           types.StageExplore,
		Objective:       "分别解释 explorer 和 subagent",
		Mutable:         types.NewMutableState("分别解释 explorer 和 subagent"),
		AnalysisIR:      ir,
		ExploreLanePlan: types.CompileExploreLanePlan(ir.RequestModel, &ir.AnswerContract, types.AnswerPresentationContract{}),
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "explorer-skill"})
	sec := findSectionTitle(pc, SectionEvidenceOrigin)
	if sec == nil {
		t.Fatal("current-source bucketed request should render explore lane ownership guidance")
	}
	for _, want := range []string{
		"Typed explore lane ownership plan",
		"unit=explorer",
		"unit=subagent",
		"current_source",
		"soft guidance, not a validator gate",
	} {
		if !strings.Contains(sec.Content, want) {
			t.Fatalf("bucket lane section missing %q:\n%s", want, sec.Content)
		}
	}
	if strings.Contains(sec.Content, "Non-current-source facts") || strings.Contains(sec.Content, "VCS history") {
		t.Fatalf("pure current-source lane ownership must not inject mixed-origin warnings:\n%s", sec.Content)
	}
}

// TestBuildPromptContext_RawToolOutputs_SkippedForExplanationShape is
// the negative regression — the section must NOT appear for deep-
// investigation (explain-class) questions, or the finalizer will
// quote raw file dumps instead of the curated Structured Evidence.
func TestBuildPromptContext_RawToolOutputs_SkippedForExplanationShape(t *testing.T) {
	mu := types.NewMutableState("explain the analyzer pipeline")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{
			{ToolName: "exec_command", Success: true, Summary: "should never appear"},
		},
	})
	ac := &types.AgentContext{
		AgentName:  types.AgentFinalizer,
		Stage:      types.StageFinalize,
		Objective:  "explain the analyzer pipeline",
		Mutable:    mu,
		AnalysisIR: explanationIR(),
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "answer-document-skill"})
	if findSectionTitle(pc, SectionRawToolOutputs) != nil {
		t.Error("Raw Tool Outputs must NOT render for explanation-shape questions")
	}
}

// TestBuildPromptContext_PriorConvHiddenFlag pins the 2026-04-17
// Prior-Conversation intent-aware filter. When AgentContext.
// PriorConvHidden is true, the "Prior Conversation (reference only)"
// user section is omitted even if Objective carries a prior block.
// The "User Request" section is always rendered so the stage still
// sees the current question.
func TestBuildPromptContext_PriorConvHiddenFlag(t *testing.T) {
	objective := "## Prior conversation\n- You: old Q\n  Codrax: old A\n\n## Current request\nhow does Foo work"

	// Visible (zero-value) → section present (historical behaviour).
	acVis := &types.AgentContext{
		AgentName: types.AgentExplorer,
		Stage:     types.StageExplore,
		Objective: objective,
	}
	pcVis := BuildPromptContext(acVis, &skill.Config{Name: "s"})
	if findSectionTitle(pcVis, SectionPriorConversation) == nil {
		t.Fatal("zero-value PriorConvHidden must render Prior Conversation section (legacy default)")
	}
	if findSectionTitle(pcVis, SectionUserRequest) == nil {
		t.Fatal("User Request section must always render")
	}

	// Hidden → section absent, User Request still rendered.
	acHid := &types.AgentContext{
		AgentName:       types.AgentExplorer,
		Stage:           types.StageExplore,
		Objective:       objective,
		PriorConvHidden: true,
	}
	pcHid := BuildPromptContext(acHid, &skill.Config{Name: "s"})
	if findSectionTitle(pcHid, SectionPriorConversation) != nil {
		t.Error("PriorConvHidden=true must omit Prior Conversation section")
	}
	if findSectionTitle(pcHid, SectionUserRequest) == nil {
		t.Error("PriorConvHidden=true must STILL render User Request section")
	}
	urSection := findSectionTitle(pcHid, SectionUserRequest)
	if !strings.Contains(urSection.Content, "how does Foo work") {
		t.Errorf("User Request must carry the current question, got %q", urSection.Content)
	}
	if strings.Contains(urSection.Content, "old Q") {
		t.Errorf("User Request must NOT leak prior content, got %q", urSection.Content)
	}
}

func TestBuildPromptContext_PresentationDirectiveIsTypedMetadata(t *testing.T) {
	bus := &types.BusContext{
		Mutable:               types.NewMutableState("读取代码，对比 codrax 和 opencode"),
		PresentationDirective: "输出各自的逻辑视图",
	}
	ac := BuildAgentContext(bus, types.AgentAnalyzer, types.StageAnalyze)
	pc := BuildPromptContext(ac, &skill.Config{Name: "analysis-skill"})

	userReq := findSectionTitle(pc, SectionUserRequest)
	if userReq == nil {
		t.Fatal("missing User Request section")
	}
	if strings.Contains(userReq.Content, "Presentation Directive") ||
		strings.Contains(userReq.Content, "输出各自的逻辑视图") {
		t.Fatalf("presentation directive must not pollute User Request: %q", userReq.Content)
	}
	sec := findSectionTitle(pc, SectionPresentationDirective)
	if sec == nil {
		t.Fatal("missing Presentation Directive section")
	}
	if !strings.Contains(sec.Content, "输出各自的逻辑视图") ||
		!strings.Contains(sec.Content, "不要把它当作仓库代码") {
		t.Fatalf("presentation directive section missing guardrails/content: %q", sec.Content)
	}
}

// TestExtractRelevantFacts_TrimsGrepPathListBody pins session-8
// Fix δ (trace 1776450670620195562): the Known Facts section used
// to quote each grep tool result's Summary verbatim, and because
// grep's Summary is "header\n + 20-odd paths", five grep calls
// rendered 100 overlapping path lines in the prompt. trimKnownFactValue
// keeps only the "[grep: N matching files]" header; the paths still
// live in the Relevant Files section, so no information is lost.
func TestExtractRelevantFacts_TrimsGrepPathListBody(t *testing.T) {
	facts := []types.RepoFact{
		{
			Key:        "grep",
			Value:      "[grep: 94 matching files]\n./a.go\n./b.go\n./c.go\n./d.go",
			Source:     "tool:grep",
			Confidence: 0.80,
		},
		{
			Key:        "read_file",
			Value:      "package foo\n\nfunc Bar() {}\n",
			Source:     "a.go",
			Confidence: 0.80,
		},
	}
	got := extractRelevantFacts(facts)
	if len(got) != 2 {
		t.Fatalf("got %d lines, want 2", len(got))
	}
	if !strings.Contains(got[0], "[grep: 94 matching files]") {
		t.Errorf("grep header must survive: %q", got[0])
	}
	if strings.Contains(got[0], "./a.go") || strings.Contains(got[0], "./b.go") {
		t.Errorf("grep path body must be stripped from Known Facts: %q", got[0])
	}
	// read_file is NOT grep → full value preserved (up to maxFactValueLen).
	if !strings.Contains(got[1], "package foo") {
		t.Errorf("non-grep fact must keep its value: %q", got[1])
	}
}

// TestBuildPromptContext_NoDuplicateRetryDirective pins the invariant
// documented on the Retry Directive section: the hint body MUST NOT
// carry its own "Retry Directive" heading. The builder is the sole
// owner of that title. Producers (hint.Composer.Render,
// orchestrator contract-check hints) must emit structured bullets
// only. A regression here would produce the double-H2 the prompt
// audit flagged — the LLM sees
// "## Retry Directive (READ FIRST)\n## Retry Directive\n**…**" —
// which wastes tokens and visually fragments the directive.
func TestBuildPromptContext_NoDuplicateRetryDirective(t *testing.T) {
	// Case 1: hint body is a plain directive without any heading →
	// one Retry Directive title in the final prompt.
	ac := &types.AgentContext{
		AgentName: types.AgentExplorer,
		Stage:     types.StageExplore,
		Objective: "q",
		RetryHint: "your previous answer missed the registration site; re-read defaults.go",
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "explore-skill"})
	msgs := ToMessages(pc)

	userBody := ""
	for _, m := range msgs {
		if m.Role == "user" {
			userBody += m.Content + "\n"
		}
	}
	if strings.Count(userBody, "Retry Directive") != 1 {
		t.Errorf("Retry Directive heading appears %d time(s) in rendered user prompt; want exactly 1\n\n%s",
			strings.Count(userBody, "Retry Directive"), userBody)
	}

	// Case 2: verify a hint body that now comes from
	// hint.Composer.Render — which no longer emits its own H2 —
	// still produces exactly ONE Retry Directive heading. This is
	// the realistic production path after the composer change; if a
	// future commit reintroduces "## Retry Directive" into the hint
	// body this assertion fires loud.
	ac.RetryHint = "**What failed**: shape mismatch\n\n**How to fix now**: pick another shape"
	pc = BuildPromptContext(ac, &skill.Config{Name: "explore-skill"})
	msgs = ToMessages(pc)
	userBody = ""
	for _, m := range msgs {
		if m.Role == "user" {
			userBody += m.Content + "\n"
		}
	}
	if strings.Count(userBody, "Retry Directive") != 1 {
		t.Errorf("composer-style hint body produced %d Retry Directive heading(s); want exactly 1\n\n%s",
			strings.Count(userBody, "Retry Directive"), userBody)
	}
}

// TestBuildAgentContext_WriteStage_ScrubsReadModeArtifacts pins the
// session-35 red line: no read-mode stage artifact may travel into a
// planner / coder / verifier AgentContext. The bus below is seeded
// with every known leak vector (5 confirmed + 2 benign-but-
// conceptually-read-mode) per the audit. All MUST be empty on the
// built context for each write-mode stage.
//
// Drift here silently reintroduces the emit_change_plan bypass bug
// (analyzer's <think> preamble tricking planner LLM into emitting
// analysis JSON prose).
func TestBuildAgentContext_WriteStage_ScrubsReadModeArtifacts(t *testing.T) {
	seedBus := func() *types.BusContext {
		mut := types.NewMutableState("seed")
		// Populate the closure with unverified findings + subject
		// matches so the Mutable-backed side of the cut is exercised.
		closure := mut.EvidenceClosure()
		closure.AppendUnverifiedFinding(types.UnverifiedFinding{
			Token: "nonexistent.go", Kind: "path", Reason: "not in graph",
		})
		closure.SetSubjectMatch("chain-1", 0.42)
		return &types.BusContext{
			RepoRoot: "/tmp/repo",
			Mutable:  mut,
			AnalysisIR: &types.AnalysisIR{
				RequestModel: types.RequestModel{
					AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey, Confidence: 0.9},
				},
			},
			// Read-mode artifact seeds — all should be stripped for
			// write stages.
			RepoFacts: []types.RepoFact{
				{Key: "entrypoint", Value: "cmd/main.go", Source: "repo_map", Confidence: 0.9},
			},
			EvidenceItems: []types.EvidenceItem{{Source: "a.go", LineStart: 1}},
			FlowFindings:  []types.FlowFindingDigest{{ID: "ff1", Sources: []string{"a.go"}}},
			AnswerChains:  []types.AnswerChain{{Item: types.EvidenceItem{Source: "a.go", LineStart: 1}, Score: 1.0}},
			AnswerSymbols: []types.AnswerSymbol{
				{Name: "Foo", File: "a.go", Line: 10, Kind: "registration"},
			},
			AnswerSymbolCompleteness: types.CompletenessComplete,
			ToolResults: []types.ToolResult{
				{ToolName: "grep", Summary: "3 hits", Success: true},
			},
			StageReports: []types.StageReport{{
				Stage:    types.StageAnalyze,
				Agent:    types.AgentAnalyzer,
				Findings: "<think>Let me emit the analysis.</think>\n\nThe request is clear.",
			}},
		}
	}

	writeStages := []struct {
		stage types.PipelineStage
		agent types.AgentName
	}{
		{types.StagePlan, types.AgentPlanner},
		{types.StageApply, types.AgentCoder},
		{types.StageVerify, types.AgentVerifier},
	}
	for _, c := range writeStages {
		c := c
		t.Run(string(c.stage), func(t *testing.T) {
			bus := seedBus()
			ac := BuildAgentContext(bus, c.agent, c.stage)

			assertEmpty := func(name string, length int) {
				t.Helper()
				if length != 0 {
					t.Errorf("ac.%s for write stage %s: got len=%d, want 0 (read-mode artifact leak)", name, c.stage, length)
				}
			}
			assertEmpty("RelevantFacts", len(ac.RelevantFacts))
			assertEmpty("RelevantFiles", len(ac.RelevantFiles))
			assertEmpty("EvidenceItems", len(ac.EvidenceItems))
			assertEmpty("FlowFindings", len(ac.FlowFindings))
			assertEmpty("AnswerChains", len(ac.AnswerChains))
			assertEmpty("AnswerSymbols", len(ac.AnswerSymbols))
			assertEmpty("RelevantToolSummaries", len(ac.RelevantToolSummaries))
			assertEmpty("RelevantMCPNotes", len(ac.RelevantMCPNotes))
			assertEmpty("PriorReports", len(ac.PriorReports))
			assertEmpty("UnverifiedAnalyzerFindings", len(ac.UnverifiedAnalyzerFindings))
			assertEmpty("SubjectMatches", len(ac.SubjectMatches))

			if ac.AnswerSymbolCompleteness != types.CompletenessUnknown {
				t.Errorf("ac.AnswerSymbolCompleteness for write stage %s: got %v, want CompletenessUnknown", c.stage, ac.AnswerSymbolCompleteness)
			}
			if ac.ExpectedAnswerSubject.Kind != types.SubjectUnknown {
				t.Errorf("ac.ExpectedAnswerSubject.Kind for write stage %s: got %v, want SubjectUnknown", c.stage, ac.ExpectedAnswerSubject.Kind)
			}

			// Prompt-level assertion: BuildPromptContext must not
			// render any of the read-mode-only section titles.
			pc := BuildPromptContext(ac, &skill.Config{Name: "change-plan-skill"})
			leakSections := []string{
				SectionPriorStageFindings,
				SectionUnverifiedAnalyzer,
				"Structured Evidence", // historical name — guard against accidental re-introduction
				SectionDataflowFindings,
				SectionAnswerSymbolsAuth,
				"Extracted Answer Symbols (lower-bound)", // historical name — guard against accidental re-introduction
				SectionSubjectMatchSummary,
			}
			for _, title := range leakSections {
				if sec := findSectionTitle(pc, title); sec != nil {
					t.Errorf("write stage %s rendered read-mode section %q (body=%q)", c.stage, title, sec.Content)
				}
			}
		})
	}
}

// TestBuildPromptContext_PlanStage_RendersAnalyzerPrescan locks the
// architecturally-correct B fix for the planner cold-start failure
// mode (planner STOP at iter=6 (eval) on legitimate feature-add
// requests because the prompt only contained the 80-char user
// request and the LLM had to re-discover everything the analyzer
// already knew). Source data is structured AnalysisIR fields only —
// no StageReports / Mutable.EvidenceClosure access — so the
// session-35 leak vector cannot reappear through this path.
func TestBuildPromptContext_PlanStage_RendersAnalyzerPrescan(t *testing.T) {
	bus := &types.BusContext{
		RepoRoot: "/tmp/repo",
		Mutable:  types.NewMutableState("add ssh-mcp downloader"),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					PrimaryEntities: []string{"mcp", "ssh", "StdioServer"},
					Entities:        []string{"mcp", "ssh", "StdioServer", "Registry"},
					Keywords:        []string{"sftp", "scp"},
				},
				SubTopics: []types.SubTopic{
					{Summary: "Add new MCP server for SSH file download", Entities: []string{"mcp", "ssh"}},
				},
			},
			EvidencePlan: types.EvidencePlan{
				RequiredFiles: []string{"internal/mcp/mcp.go", "internal/mcp/stdio.go"},
			},
		},
	}
	bus.Mutable.SetObjective("add ssh-mcp downloader")

	ac := BuildAgentContext(bus, types.AgentPlanner, types.StagePlan)
	pc := BuildPromptContext(ac, &skill.Config{Name: "change-plan-skill"})

	sec := findSectionTitle(pc, SectionAnalyzerPrescan)
	if sec == nil {
		t.Fatal("StagePlan: expected 'Analyzer Pre-scan Findings' section, got none")
	}
	wantSubstrings := []string{
		"Sub-topics",
		"Add new MCP server for SSH file download",
		"Code entities (analyzer-extracted)",
		"- mcp",
		"- StdioServer",
		"Pre-scored relevant files",
		"internal/mcp/mcp.go",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(sec.Content, want) {
			t.Errorf("Analyzer Pre-scan Findings missing %q\n--- section ---\n%s", want, sec.Content)
		}
	}
}

// TestBuildPromptContext_NonPlanStages_SuppressAnalyzerPrescan locks
// that the section is StagePlan-only. Apply/verify consume Mutable.
// ChangePlan directly and any analyzer hints would be off-task signal;
// read stages have their own richer Prior Stage Findings channel.
func TestBuildPromptContext_NonPlanStages_SuppressAnalyzerPrescan(t *testing.T) {
	mkBus := func() *types.BusContext {
		bus := &types.BusContext{
			RepoRoot: "/tmp/repo",
			Mutable:  types.NewMutableState("seed"),
			AnalysisIR: &types.AnalysisIR{
				RequestModel: types.RequestModel{
					AnalyzerHints: types.AnalyzerHints{
						PrimaryEntities: []string{"foo"},
					},
				},
				EvidencePlan: types.EvidencePlan{RequiredFiles: []string{"a.go"}},
			},
		}
		bus.Mutable.SetObjective("seed")
		return bus
	}

	cases := []struct {
		stage types.PipelineStage
		agent types.AgentName
	}{
		{types.StageApply, types.AgentCoder},
		{types.StageVerify, types.AgentVerifier},
		{types.StageExplore, types.AgentExplorer},
		{types.StageExtract, types.AgentExtractor},
		{types.StageFinalize, types.AgentFinalizer},
		{types.StageAnalyze, types.AgentAnalyzer},
	}
	for _, c := range cases {
		c := c
		t.Run(string(c.stage), func(t *testing.T) {
			ac := BuildAgentContext(mkBus(), c.agent, c.stage)
			pc := BuildPromptContext(ac, &skill.Config{Name: "test-skill"})
			if sec := findSectionTitle(pc, SectionAnalyzerPrescan); sec != nil {
				t.Errorf("stage %s rendered StagePlan-only section (body=%q)", c.stage, sec.Content)
			}
		})
	}
}

// TestFormatAnalyzerPrescanForPlan_EmptyReturnsEmpty pins the
// degenerate path: no entities, no sub-topics, no required files →
// empty string so the caller skips section emission entirely. Without
// this an analyzer that returned only AnswerSubject (e.g. on a tiny
// rephrase request) would render an unhelpful "use the structured
// findings below" header with no body.
func TestFormatAnalyzerPrescanForPlan_EmptyReturnsEmpty(t *testing.T) {
	cases := []struct {
		name string
		ir   *types.AnalysisIR
	}{
		{"nil IR", nil},
		{"empty IR", &types.AnalysisIR{}},
		{"only AnswerSubject", &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatAnalyzerPrescanForPlan(c.ir); got != "" {
				t.Errorf("expected empty string, got: %q", got)
			}
		})
	}
}

// TestBuildAgentContext_ReadStage_PropagatesArtifacts confirms the
// gate is one-sided: read-mode stages still see the artifacts they
// were designed to consume. Paired with
// TestBuildAgentContext_WriteStage_ScrubsReadModeArtifacts — if this
// test passes and that one fails, the gate polarity is correct but
// write-side is broken; if this one fails, the gate broke read-mode
// (a regression no one wants).
func TestBuildAgentContext_ReadStage_PropagatesArtifacts(t *testing.T) {
	bus := &types.BusContext{
		RepoRoot: "/tmp/repo",
		Mutable:  types.NewMutableState("seed"),
		RepoFacts: []types.RepoFact{
			{Key: "entrypoint", Value: "cmd/main.go", Source: "repo_map", Confidence: 0.9},
		},
		AnswerChains: []types.AnswerChain{{Item: types.EvidenceItem{Source: "a.go", LineStart: 1}, Score: 1.0}},
		StageReports: []types.StageReport{{
			Stage: types.StageAnalyze, Agent: types.AgentAnalyzer, Findings: "analysis narrative",
		}},
	}
	// StageExplore is a canonical read-mode stage.
	ac := BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)

	if len(ac.RelevantFacts) == 0 {
		t.Error("RelevantFacts empty for read stage — gate leaked into read mode")
	}
	if len(ac.AnswerChains) == 0 {
		t.Error("AnswerChains empty for read stage — gate leaked into read mode")
	}
	if len(ac.PriorReports) == 0 {
		t.Error("PriorReports empty for read stage — gate leaked into read mode")
	}
}

func TestRenderAnswerChainForPrompt_PrefersDeterministicSurface(t *testing.T) {
	chain := types.AnswerChain{
		Item: types.EvidenceItem{
			Kind:         types.EvidenceConditional,
			Source:       "internal/agent/analyzer.go",
			LineStart:    861,
			AnchorKind:   types.AnchorCondition,
			AnchorSymbol: "buildAnalysisIR",
			Condition:    "ctx == nil || ctx.Mutable == nil",
			Summary:      "nil ctx definitely flowed in from the caller and skipped the guard",
		},
	}

	got := renderAnswerChainForPrompt(chain)
	if !strings.Contains(got, "buildAnalysisIR guard condition IF ctx == nil || ctx.Mutable == nil") {
		t.Fatalf("expected deterministic guard surface, got: %s", got)
	}
	if strings.Contains(got, "nil ctx definitely flowed in from the caller") {
		t.Fatalf("answer-chain prompt should not replay over-strong free summary: %s", got)
	}
}

func TestBuildAgentContext_FinalizerTypedSupportSuppressesCompetingStageReports(t *testing.T) {
	mut := types.NewMutableState("panic root cause")
	bus := &types.BusContext{
		PipelineStage: types.StageFinalize,
		Mutable:       mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioRootCause,
				Intent:   types.IntentRootCause,
				LogTriage: &types.LogBundle{
					Errors: []types.LogError{{Type: "panic"}},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
		StageReports: []types.StageReport{
			{Stage: types.StageLogTriage, Agent: types.AgentLogTriager, Findings: "structured triage narrative"},
			{Stage: types.StageAnalyze, Agent: types.AgentAnalyzer, Findings: "analysis narrative"},
			{Stage: types.StageExplore, Agent: types.AgentExplorer, Findings: "investigation summary"},
			{Stage: types.StageExtract, Agent: types.AgentExtractor, Findings: "free-form extractor conclusion"},
		},
	}
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{{
				File: "internal/agent/analyzer.go",
				Line: 250,
				Func: "buildAnalysisIR",
				Raw:  "buildAnalysisIR(0x0)",
			}},
		}},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceRelationship,
			Source:          "internal/agent/analyzer.go",
			LineStart:       743,
			AnchorKind:      types.AnchorCall,
			Subject:         "ParseOutput",
			Object:          "buildAnalysisIR",
			AnchorSymbol:    "buildAnalysisIR",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceConditional,
			Source:          "internal/agent/analyzer.go",
			LineStart:       978,
			AnchorKind:      types.AnchorCondition,
			AnchorSymbol:    "buildAnalysisIR",
			Condition:       "ctx == nil || ctx.Mutable == nil",
			GroundingStatus: types.GroundingGrounded,
		},
	})

	ac := BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	if len(ac.PriorReports) != 0 {
		t.Fatalf("typed-support finalizer should suppress plain competing prior stage reports, got %d", len(ac.PriorReports))
	}
}

func TestBuildAgentContext_FinalizerTypedSupportKeepsRichModelSurfaceDraft(t *testing.T) {
	mut := types.NewMutableState("source inventory table")
	bus := &types.BusContext{
		PipelineStage: types.StageFinalize,
		Mutable:       mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
			},
		},
		StageReports: []types.StageReport{
			{Stage: types.StageAnalyze, Agent: types.AgentAnalyzer, Findings: "| noisy | table |\n| --- | --- |\n| analyzer | should drop |"},
			{Stage: types.StageExtract, Agent: types.AgentExtractor, Findings: "| member | entry |\n| --- | --- |\n| alpha | Run |\n| beta | Execute |"},
		},
	}
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "alpha.go",
		LineStart:       1,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Run",
		GroundingStatus: types.GroundingGrounded,
	}})

	ac := BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	if len(ac.PriorReports) != 1 {
		t.Fatalf("typed-support finalizer should keep one rich extractor surface, got %+v", ac.PriorReports)
	}
	if ac.PriorReports[0].Agent != types.AgentExtractor ||
		!strings.Contains(ac.PriorReports[0].Findings, "Model-authored visible surface draft") ||
		!strings.Contains(ac.PriorReports[0].Findings, "| alpha | Run |") ||
		strings.Contains(ac.PriorReports[0].Findings, "analyzer") {
		t.Fatalf("unexpected preserved surface report: %+v", ac.PriorReports)
	}
}

func TestBuildPromptContext_AnalyzerSuppressesRawAttachedLogWhenLogBundleIsAuthoritative(t *testing.T) {
	mut := types.NewMutableState("panic root cause")
	mut.SetLogTriage(&types.LogBundle{
		Meta:          types.LogMeta{Lang: "go", Signals: []types.LogSignal{types.SignalPanic}},
		ResolvedFiles: []string{"internal/agent/analyzer.go"},
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{
				{
					File:       "internal/agent/analyzer.go",
					Line:       250,
					Func:       "buildAnalysisIR",
					Raw:        "github.com/hanchaoqun/codrax/internal/agent.buildAnalysisIR(0x0)",
					Confidence: 0.95,
				},
			},
		}},
	})
	bus := &types.BusContext{
		PipelineStage: types.StageAnalyze,
		Mutable:       mut,
		AttachedLog: "github.com/hanchaoqun/codrax/internal/agent.buildAnalysisIR(0x0)\n\tinternal/agent/analyzer.go:250 +0x1e\n" +
			"github.com/hanchaoqun/codrax/internal/agent.(*analyzerEvaluator).ParseOutput(0x0, 0x0, 0x0)\n\tinternal/agent/analyzer.go:320 +0x2a",
	}

	ac := BuildAgentContext(bus, types.AgentAnalyzer, types.StageAnalyze)
	pc := BuildPromptContext(ac, &skill.Config{Name: "analysis-skill"})
	if findSectionTitle(pc, SectionAttachedRuntimeLog) != nil {
		t.Fatal("authoritative structured log bundle should suppress raw attached runtime log for downstream analyzer prompts")
	}
	sec := findSectionTitle(pc, SectionLogTriageExtraction)
	if sec == nil {
		t.Fatal("expected structured log triage section when authoritative bundle exists")
	}
	if strings.Contains(sec.Content, "buildAnalysisIR(0x0)") {
		t.Fatalf("structured section should redact raw runtime tuple payloads:\n%s", sec.Content)
	}
	if !strings.Contains(sec.Content, "buildAnalysisIR(...)") {
		t.Fatalf("structured section should retain the frame identity after redaction:\n%s", sec.Content)
	}
}

func TestBuildAgentContext_ExtractorSuppressesTriageStageReportsWhenStructuredBundlesExist(t *testing.T) {
	mut := types.NewMutableState("triage prompt hygiene")
	mut.SetLogTriage(&types.LogBundle{
		Meta: types.LogMeta{Lang: "go", Summary: "auxiliary log summary"},
		Errors: []types.LogError{{
			Type: "panic",
		}},
	})
	bus := &types.BusContext{
		PipelineStage: types.StageExtract,
		Mutable:       mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:  types.ScenarioRootCause,
				Intent:    types.IntentRootCause,
				LogTriage: mut.LogTriage(),
			},
		},
		StageReports: []types.StageReport{
			{Stage: types.StageLogTriage, Agent: types.AgentLogTriager, Findings: "Synopsis (auxiliary shorthand): nil receiver"},
			{Stage: types.StageExplore, Agent: types.AgentExplorer, Findings: "investigation summary"},
		},
	}

	ac := BuildAgentContext(bus, types.AgentExtractor, types.StageExtract)
	if len(ac.PriorReports) != 1 {
		t.Fatalf("expected only non-triager prior reports to survive, got %d", len(ac.PriorReports))
	}
	if ac.PriorReports[0].Agent != types.AgentExplorer {
		t.Fatalf("unexpected surviving prior report agent: %s", ac.PriorReports[0].Agent)
	}
}

func TestBuildPromptContext_ExtractorOmitsStructuredLogSummaryProse(t *testing.T) {
	mut := types.NewMutableState("triage prompt hygiene")
	mut.SetLogTriage(&types.LogBundle{
		Meta: types.LogMeta{
			Lang:    "go",
			Signals: []types.LogSignal{types.SignalPanic},
			Summary: "ParseOutput 收到了 nil receiver",
		},
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{{
				File:       "internal/agent/analyzer.go",
				Line:       250,
				Func:       "buildAnalysisIR",
				Raw:        "buildAnalysisIR(0x0)",
				Confidence: 0.95,
			}},
		}},
	})
	bus := &types.BusContext{
		PipelineStage: types.StageExtract,
		Mutable:       mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:  types.ScenarioRootCause,
				Intent:    types.IntentRootCause,
				LogTriage: mut.LogTriage(),
			},
		},
	}

	ac := BuildAgentContext(bus, types.AgentExtractor, types.StageExtract)
	pc := BuildPromptContext(ac, &skill.Config{Name: "extract-skill"})
	sec := findSectionTitle(pc, SectionLogTriageExtraction)
	if sec == nil {
		t.Fatal("missing structured log triage section")
	}
	if strings.Contains(sec.Content, "Summary:") || strings.Contains(sec.Content, "nil receiver") {
		t.Fatalf("structured log triage section must not surface auxiliary summary prose:\n%s", sec.Content)
	}
}

func TestBuildAgentContext_FinalizerTypedSupportFiltersRepoFactsToSupportFiles(t *testing.T) {
	mut := types.NewMutableState("panic root cause")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{{
				File: "internal/agent/analyzer.go",
				Line: 250,
				Func: "buildAnalysisIR",
				Raw:  "buildAnalysisIR(0x0)",
			}},
		}},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceRelationship,
			Source:          "internal/agent/analyzer.go",
			LineStart:       743,
			AnchorKind:      types.AnchorCall,
			Subject:         "ParseOutput",
			Object:          "buildAnalysisIR",
			AnchorSymbol:    "buildAnalysisIR",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceConditional,
			Source:          "internal/agent/analyzer.go",
			LineStart:       978,
			AnchorKind:      types.AnchorCondition,
			AnchorSymbol:    "buildAnalysisIR",
			Condition:       "ctx == nil || ctx.Mutable == nil",
			GroundingStatus: types.GroundingGrounded,
		},
	})

	bus := &types.BusContext{
		PipelineStage: types.StageFinalize,
		Mutable:       mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioRootCause,
				Intent:   types.IntentRootCause,
				LogTriage: &types.LogBundle{
					Errors: []types.LogError{{Type: "panic"}},
				},
			},
		},
		RepoFacts: []types.RepoFact{
			{Key: "read_file", Value: "[internal/agent/analyzer.go: showing lines 698-978]", Source: "internal/agent/analyzer.go", Confidence: 0.8},
			{Key: "read_file", Value: "[internal/orchestrator/orchestrator.go: showing lines 4701-4780]", Source: "internal/orchestrator/orchestrator.go", Confidence: 0.8},
		},
	}

	ac := BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	if len(ac.RelevantFiles) != 1 || ac.RelevantFiles[0] != "internal/agent/analyzer.go" {
		t.Fatalf("RelevantFiles = %v, want only internal/agent/analyzer.go", ac.RelevantFiles)
	}
	if len(ac.RelevantFacts) != 1 || !strings.Contains(ac.RelevantFacts[0], "internal/agent/analyzer.go") {
		t.Fatalf("RelevantFacts = %v, want only analyzer.go sourced facts", ac.RelevantFacts)
	}
}

func TestBuildPromptContext_FinalizerTypedSupportEvidencePoolUsesAuthoritativeSurface(t *testing.T) {
	mut := types.NewMutableState("panic root cause")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{{
				File: "internal/agent/analyzer.go",
				Line: 250,
				Func: "buildAnalysisIR",
				Raw:  "buildAnalysisIR(0x0)",
			}},
		}},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceRelationship,
			Source:          "internal/agent/analyzer.go",
			LineStart:       743,
			AnchorKind:      types.AnchorCall,
			Subject:         "ParseOutput",
			Object:          "buildAnalysisIR",
			AnchorSymbol:    "buildAnalysisIR",
			Summary:         "ParseOutput 第 743 行直接将 ctx 传递给 buildAnalysisIR，如果 ctx 为 nil 则传入 nil",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       698,
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    "ctx",
			OwnerSymbol:     "ParseOutput",
			Summary:         "ParseOutput 的第一个参数是 ctx *types.AgentContext，日志显示调用时传入了 nil (0x0)",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/orchestrator/orchestrator.go",
			LineStart:       4701,
			AnchorKind:      types.AnchorCall,
			Subject:         "dispatchStage",
			Object:          "runAnalyzePhase",
			Summary:         "dispatchStage 继续把 nil ctx 向外层传递",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceConditional,
			Source:          "internal/agent/analyzer.go",
			LineStart:       978,
			AnchorKind:      types.AnchorCondition,
			AnchorSymbol:    "buildAnalysisIR",
			Condition:       "ctx == nil || ctx.Mutable == nil",
			Summary:         "buildAnalysisIR 开头有 nil 检查",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       981,
			AnchorKind:      types.AnchorCall,
			Subject:         "buildAnalysisIR",
			Object:          "RequestModel",
			AnchorSymbol:    "RequestModel",
			Summary:         "buildAnalysisIR 第 981 行调用 ctx.Mutable.RequestModel()，这就是 panic 的实际代码行",
			GroundingStatus: types.GroundingGrounded,
		},
	})

	bus := &types.BusContext{
		PipelineStage: types.StageFinalize,
		Mutable:       mut,
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceRelationship,
				Source:          "internal/agent/analyzer.go",
				LineStart:       743,
				AnchorKind:      types.AnchorCall,
				Subject:         "ParseOutput",
				Object:          "buildAnalysisIR",
				AnchorSymbol:    "buildAnalysisIR",
				Summary:         "ParseOutput 第 743 行直接将 ctx 传递给 buildAnalysisIR，如果 ctx 为 nil 则传入 nil",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/agent/analyzer.go",
				LineStart:       698,
				AnchorKind:      types.AnchorAssignment,
				AnchorSymbol:    "ctx",
				OwnerSymbol:     "ParseOutput",
				Summary:         "ParseOutput 的第一个参数是 ctx *types.AgentContext，日志显示调用时传入了 nil (0x0)",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/orchestrator/orchestrator.go",
				LineStart:       4701,
				AnchorKind:      types.AnchorCall,
				Subject:         "dispatchStage",
				Object:          "runAnalyzePhase",
				Summary:         "dispatchStage 继续把 nil ctx 向外层传递",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceConditional,
				Source:          "internal/agent/analyzer.go",
				LineStart:       978,
				AnchorKind:      types.AnchorCondition,
				AnchorSymbol:    "buildAnalysisIR",
				Condition:       "ctx == nil || ctx.Mutable == nil",
				Summary:         "buildAnalysisIR 开头有 nil 检查",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/agent/analyzer.go",
				LineStart:       981,
				AnchorKind:      types.AnchorCall,
				Subject:         "buildAnalysisIR",
				Object:          "RequestModel",
				AnchorSymbol:    "RequestModel",
				Summary:         "buildAnalysisIR 第 981 行调用 ctx.Mutable.RequestModel()，这就是 panic 的实际代码行",
				GroundingStatus: types.GroundingGrounded,
			},
		},
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioRootCause,
				Intent:   types.IntentRootCause,
				LogTriage: &types.LogBundle{
					Errors: []types.LogError{{Type: "panic"}},
				},
			},
		},
	}

	ac := BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	pc := BuildPromptContext(ac, &skill.Config{Name: "finalize-answer"})
	sec := findSectionTitle(pc, SectionEvidencePool)
	if sec == nil {
		t.Fatalf("missing Knowledge & Evidence Pool section")
	}
	if !strings.Contains(sec.Content, "citation coverage only") {
		t.Fatalf("typed-support evidence preamble missing:\n%s", sec.Content)
	}
	if strings.Contains(sec.Content, "传入了 nil") || strings.Contains(sec.Content, "0x0") {
		t.Fatalf("typed-support evidence pool must not replay freeform nil-provenance summaries:\n%s", sec.Content)
	}
	if strings.Contains(sec.Content, "dispatchStage") || strings.Contains(sec.Content, "runAnalyzePhase") {
		t.Fatalf("typed-support evidence pool leaked out-of-ceiling outer call chain:\n%s", sec.Content)
	}
	if strings.Contains(sec.Content, "RequestModel") {
		t.Fatalf("typed-support evidence pool should not reintroduce intra-function helper calls outside the support lanes:\n%s", sec.Content)
	}
	if !strings.Contains(sec.Content, "ParseOutput calls buildAnalysisIR") {
		t.Fatalf("typed-support evidence pool should keep authoritative call anchor text:\n%s", sec.Content)
	}
	if strings.Contains(sec.Content, "ParseOutput assignment anchor for ctx") {
		t.Fatalf("typed-support evidence pool should not keep non-support-lane same-file assignments once location ceiling is active:\n%s", sec.Content)
	}
}

func TestBuildPromptContext_FinalizerTypedSupportSuppressesCompetingSemanticSections(t *testing.T) {
	mut := types.NewMutableState("panic root cause")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{{
				File: "internal/agent/analyzer.go",
				Line: 250,
				Func: "buildAnalysisIR",
				Raw:  "buildAnalysisIR(0x0)",
			}},
		}},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceRelationship,
			Source:          "internal/agent/analyzer.go",
			LineStart:       743,
			AnchorKind:      types.AnchorCall,
			Subject:         "ParseOutput",
			Object:          "buildAnalysisIR",
			AnchorSymbol:    "buildAnalysisIR",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceConditional,
			Source:          "internal/agent/analyzer.go",
			LineStart:       978,
			AnchorKind:      types.AnchorCondition,
			AnchorSymbol:    "buildAnalysisIR",
			Condition:       "ctx == nil || ctx.Mutable == nil",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       320,
			AnchorKind:      types.AnchorCall,
			AnchorSymbol:    "ParseOutput",
			Summary:         "栈帧显示第 320 行调用 ParseOutput，第 1 个参数 0x0 表示 receiver e 为 nil",
			GroundingStatus: types.GroundingUngrounded,
		},
	})
	mut.SetEmittedAnswerSymbols([]types.AnswerSymbol{{
		Name:      "dispatchStage",
		File:      "internal/orchestrator/orchestrator.go",
		Line:      4701,
		Rationale: "outer orchestration hop",
	}}, types.CompletenessLowerBound)
	mut.EvidenceClosure().SetSubjectMatch("internal/orchestrator/orchestrator.go", 0.91)

	bus := &types.BusContext{
		PipelineStage: types.StageFinalize,
		Mutable:       mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioRootCause,
				Intent:   types.IntentRootCause,
				LogTriage: &types.LogBundle{
					Errors: []types.LogError{{Type: "panic"}},
				},
			},
		},
		RepoFacts: []types.RepoFact{
			{Key: "read_file", Value: "[internal/agent/analyzer.go: showing lines 698-978]", Source: "internal/agent/analyzer.go", Confidence: 0.8},
		},
	}

	ac := BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	pc := BuildPromptContext(ac, finalizerSkill())

	if findSectionTitle(pc, SectionKnownFacts) != nil {
		t.Fatal("typed-support finalizer must suppress Known Facts to avoid competing prose facts")
	}
	if findSectionTitle(pc, SectionAnswerSymbolsAuth) != nil || findSectionTitle(pc, SectionAnswerSymbolsFloor) != nil {
		t.Fatal("typed-support finalizer must suppress Extracted Answer Symbols sections")
	}
	if findSectionTitle(pc, SectionSubjectMatchSummary) != nil {
		t.Fatal("typed-support finalizer must suppress Subject Match Summary")
	}
	if findSectionTitle(pc, SectionUnverifiedLeads) != nil {
		t.Fatal("typed-support finalizer must suppress Unverified Leads")
	}
}

// TestStripThinkBlocks covers the Category-4 defense-in-depth: reasoning
// traces leaking into downstream prompts. Even for read-mode stages
// (which keep PriorReports), the analyzer's internal deliberation
// should not travel as a plain-text preamble.
func TestStripThinkBlocks(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no tags", "plain narrative", "plain narrative"},
		{"single block", "<think>internal</think>visible", "visible"},
		{"trailing whitespace after close tag stripped",
			"<think>a</think>\n\nbody",
			"body"},
		{"multi-line block (DOTALL)",
			"before <think>line1\nline2\nline3</think> after",
			"before after"},
		{"multiple blocks",
			"<think>one</think>middle<think>two</think>end",
			"middleend"},
		{"case-insensitive close tag",
			"<think>x</Think>y",
			"y"},
		{"unclosed think block preserved (regex demands close)",
			"<think>runaway",
			"<think>runaway"},
		{"mixed with code fences",
			"```go\nfunc main() {}\n```\n<think>why?</think>\nsummary",
			"```go\nfunc main() {}\n```\nsummary"},
	}
	for _, c := range cases {
		if got := stripThinkBlocks(c.in); got != c.want {
			t.Errorf("stripThinkBlocks(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestFormatStageReports_StripsThinkBlocks integration-tests that the
// full render pipeline silently drops <think> spans. This closes the
// session-35 root cause: the analyzer's "Let me emit the analysis…"
// preamble that tricked the planner LLM into skipping
// emit_change_plan.
func TestFormatStageReports_StripsThinkBlocks(t *testing.T) {
	reports := []types.StageReport{{
		Stage:    types.StageAnalyze,
		Agent:    types.AgentAnalyzer,
		Findings: "<think>Let me emit the analysis.</think>\n\nmain.go confirmed present.",
	}}
	out := formatStageReports(reports)
	if strings.Contains(out, "<think>") || strings.Contains(out, "Let me emit the analysis") {
		t.Errorf("formatStageReports leaked think-block content:\n%s", out)
	}
	if !strings.Contains(out, "main.go confirmed present") {
		t.Errorf("formatStageReports dropped visible content:\n%s", out)
	}
	if !strings.Contains(out, "### Prior request analysis result") {
		t.Errorf("formatStageReports dropped section header:\n%s", out)
	}
	if strings.Contains(out, "[analyze / analyzer]") {
		t.Errorf("formatStageReports leaked internal stage/agent codenames:\n%s", out)
	}
}

// stubMemReader is the minimal MemoryReader fake the propagation
// tests need — Search returns a single canned IndexEntry so callers
// can detect "yes, the same instance flowed through".
type stubMemReader struct{ tag string }

func (s *stubMemReader) Search(query string, opts types.MemorySearchOpts) []types.MemoryIndexEntry {
	return []types.MemoryIndexEntry{{ID: s.tag, Topic: query}}
}

// TestBuildAgentContext_PlumbsReadOnlyToolFields pins the regression
// fix shipped in this commit: BuildAgentContext MUST propagate the
// read-only fields tools depend on (Memory / EnvFacts /
// EnvRecommendSettings / MainRepoRoot) from BusContext to
// AgentContext. Pre-fix these were silently dropped, causing
// recall_memory to return "unavailable" and the env_recommend
// integration to be DOA in agent dispatch paths.
func TestBuildAgentContext_PlumbsReadOnlyToolFields(t *testing.T) {
	mem := &stubMemReader{tag: "MEM-PROP"}
	facts := &types.EnvFacts{OS: "linux"}
	settings := types.EnvRecommendSettings{Enabled: true}
	bus := &types.BusContext{
		RepoRoot:             "/tmp/repo",
		MainRepoRoot:         "/home/user/orig-repo",
		WorktreePath:         "/tmp/codrax-worktree",
		Mutable:              types.NewMutableState("q"),
		Memory:               mem,
		EnvFacts:             facts,
		EnvRecommendSettings: settings,
		Language:             "zh",
	}
	ac := BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)

	if ac.MainRepoRoot != "/home/user/orig-repo" {
		t.Errorf("MainRepoRoot lost: got %q, want %q", ac.MainRepoRoot, "/home/user/orig-repo")
	}
	if ac.WorktreePath != "/tmp/codrax-worktree" {
		t.Errorf("WorktreePath lost: got %q, want %q", ac.WorktreePath, "/tmp/codrax-worktree")
	}
	if ac.Memory == nil {
		t.Fatalf("Memory dropped — recall_memory tool would see nil")
	}
	got := ac.Memory.Search("probe", types.MemorySearchOpts{})
	if len(got) != 1 || got[0].ID != "MEM-PROP" {
		t.Errorf("Memory adapter identity lost: %+v", got)
	}
	if ac.EnvFacts == nil || ac.EnvFacts.OS != "linux" {
		t.Errorf("EnvFacts dropped: got %+v", ac.EnvFacts)
	}
	if !ac.EnvRecommendSettings.Enabled {
		t.Errorf("EnvRecommendSettings dropped: got %+v", ac.EnvRecommendSettings)
	}
	// Language was already plumbed pre-fix; pin it here too so the
	// "5 read-only fields" set is locked as a unit.
	if ac.Language != "zh" {
		t.Errorf("Language dropped: got %q", ac.Language)
	}
}

// TestShouldSuppressAttachedRuntimeTrace_PerfBundleAuthoritative is
// the perf-trace mirror of the existing log-side suppression test.
// When a typed PerfBundle has resolved frames + the "performance"
// IntentHint, downstream non-perf-triager agents should NOT see the
// raw trace section — the structured bundle is authoritative and
// the raw text's tuple-dense payload (thread ids, hex addresses,
// μs timestamps) only bait the LLM into rationalising those numbers.
func TestShouldSuppressAttachedRuntimeTrace_PerfBundleAuthoritative(t *testing.T) {
	mu := types.NewMutableState("why is the cold start slow")
	mu.SetPerfTrace(&types.PerfBundle{
		IntentHint:    "performance",
		ResolvedFiles: []string{"app/main.ets"},
		Meta:          types.PerfMeta{Source: "hitrace", Signals: []string{"cold-start-slow"}},
	})
	ac := &types.AgentContext{
		AgentName:       types.AgentAnalyzer,
		Stage:           types.StageAnalyze,
		Objective:       "why is the cold start slow",
		Mutable:         mu,
		PerfTrace:       mu.PerfTrace(),
		AttachedHitrace: "0xabc12345 cpu 3 sched_switch prev_comm=app prev_pid=1 next_comm=ui next_pid=2\n",
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "test"})
	for _, s := range pc.UserSections {
		if s.Title == SectionAttachedPerfTrace {
			t.Fatalf("raw perf trace section should be suppressed when typed PerfBundle is authoritative; got %q", s.Content)
		}
	}
}

func TestBuildPromptContext_FinalizerSuppressesPerfResidueAfterTraceQuery(t *testing.T) {
	mu := types.NewMutableState("summarize the trace window")
	mu.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace"},
		Observations: []types.PerfObservation{{
			Kind:      "state_churn",
			Subject:   "app-20",
			Summary:   "typed perf observation remains available",
			LineStart: 12,
		}},
		Residue: []string{"pre-triage availability note should stay out of final answer prompt"},
	})
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{{
			ToolName: "trace_query",
			Success:  true,
			Observations: []types.ObservationRecord{{
				ID:       "trace-query:window-stats:0",
				Origin:   types.AnswerEvidenceOriginRuntimeArtifact,
				Producer: "trace_query",
				SourceRef: types.ObservationSourceRef{
					Kind:       types.ObservationSourceRuntimeArtifact,
					ArtifactID: "attached_trace",
				},
				Summary: "trace_query window_stats returned bounded state churn metrics",
			}},
		}},
	})
	ac := &types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		Objective: "summarize the trace window",
		Mutable:   mu,
		PerfTrace: mu.PerfTrace(),
	}

	pc := BuildPromptContext(ac, finalizerSkill())
	sec := findSectionTitle(pc, SectionPerfTriageExtraction)
	if sec == nil {
		t.Fatal("finalizer should still receive structured perf-triage observations")
	}
	if !strings.Contains(sec.Content, "typed perf observation remains available") {
		t.Fatalf("typed perf observation was dropped with residue:\n%s", sec.Content)
	}
	if strings.Contains(sec.Content, "Residue") ||
		strings.Contains(sec.Content, "pre-triage availability note should stay out") {
		t.Fatalf("finalizer prompt should suppress pre-triage residue after trace_query:\n%s", sec.Content)
	}
}

// TestShouldSuppressAttachedRuntimeTrace_PerfTriagerStillSeesIt
// confirms the perf_triager itself is exempt from suppression — it
// is the producer of the typed bundle and must read the raw trace
// to produce it.
func TestShouldSuppressAttachedRuntimeTrace_PerfTriagerStillSeesIt(t *testing.T) {
	mu := types.NewMutableState("why is the cold start slow")
	mu.SetPerfTrace(&types.PerfBundle{
		IntentHint:    "performance",
		ResolvedFiles: []string{"app/main.ets"},
		Meta:          types.PerfMeta{Source: "hitrace"},
	})
	ac := &types.AgentContext{
		AgentName:       types.AgentPerfTriager,
		Stage:           types.StagePerfTriage,
		Objective:       "why is the cold start slow",
		Mutable:         mu,
		PerfTrace:       mu.PerfTrace(),
		AttachedHitrace: "0xabc12345 cpu 3 sched_switch prev_comm=app\n",
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "test"})
	hasSection := false
	for _, s := range pc.UserSections {
		if s.Title == SectionAttachedPerfTrace {
			hasSection = true
			break
		}
	}
	if !hasSection {
		t.Fatal("perf_triager must see the raw perf trace section regardless of authoritative bundle")
	}
}

// TestShouldSuppressAttachedRuntimeTrace_NoBundle_StillRenders
// confirms the gate falls through to "render" when no typed bundle
// is on Mutable — non-authoritative state means the LLM should still
// see the raw trace as the only legible input channel.
func TestShouldSuppressAttachedRuntimeTrace_NoBundle_StillRenders(t *testing.T) {
	mu := types.NewMutableState("why is the cold start slow")
	ac := &types.AgentContext{
		AgentName:       types.AgentAnalyzer,
		Stage:           types.StageAnalyze,
		Objective:       "why is the cold start slow",
		Mutable:         mu,
		AttachedHitrace: "0xabc12345 cpu 3 sched_switch prev_comm=app\n",
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "test"})
	hasSection := false
	for _, s := range pc.UserSections {
		if s.Title == SectionAttachedPerfTrace {
			hasSection = true
			break
		}
	}
	if !hasSection {
		t.Fatal("raw perf trace must render when no typed PerfBundle is present")
	}
}

func TestFormatPerfTriageStructured_ExternalSourceDirective(t *testing.T) {
	bundle := &types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace", Signals: []string{"jank"}},
		Stalls: []types.PerfStall{{
			Symbol: "ForeignRender",
			File:   "foreign/render.cpp",
			Line:   42,
		}},
	}
	got := formatPerfTriageStructured(bundle, nil)
	for _, want := range []string{
		"External-source trace",
		"resolved_files=0",
		"attached-trace observation lane",
		"keep two lanes",
		"leave the claim uncited",
		"foreign/render.cpp:42 observed, unresolved",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("perf structured section missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "The answer must come from the trace's own timing") {
		t.Fatalf("external-source directive must not collapse mixed current-code requests into observation-only:\n%s", got)
	}
	if strings.Contains(got, "foreign/render.cpp:42 ★ resolved") {
		t.Fatalf("unresolved trace file must not be marked citation-grade:\n%s", got)
	}
}

func TestFormatPerfTriageStructured_FrameDriftWarning(t *testing.T) {
	loc := &stubLocator{
		byName: map[string][]types.SymbolLocation{},
		byFile: map[string][]types.SymbolLocation{},
	}
	bundle := &types.PerfBundle{
		Meta:          types.PerfMeta{Source: "perfetto"},
		ResolvedFiles: []string{"app/render.go"},
		Stalls: []types.PerfStall{{
			Symbol: "OldRender",
			File:   "app/render.go",
			Line:   88,
		}},
	}
	got := formatPerfTriageStructured(bundle, loc)
	for _, want := range []string{
		"Frame ↔ current-code drift warning",
		"app/render.go:88",
		"OPAQUE OBSERVATION",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("perf drift section missing %q:\n%s", want, got)
		}
	}
}

// TestSanitizeRuntimeFrameRaw_AllLines confirms #5: every line of a
// multi-line stack-frame raw payload gets scrubbed, not just the
// first. Real-world Go panics put the runtime tuple on the SECOND
// line (the body line, after the goroutine header) — only sanitising
// lines[0] would let the tuple leak intact.
func TestSanitizeRuntimeFrameRaw_AllLines(t *testing.T) {
	raw := "goroutine 1 [running]:\n" +
		"main.Foo(0xc000010040, 0x0, 0x42)\n" +
		"\t/path/to/main.go:42 +0x88"
	got := sanitizeRuntimeFrameRaw(raw)
	if strings.Contains(got, "0xc000010040") || strings.Contains(got, "0x0,") {
		t.Errorf("body-line tuple not sanitised; got:\n%s", got)
	}
	if !strings.Contains(got, "main.Foo(...)") {
		t.Errorf("body line should have been collapsed to Func(...); got:\n%s", got)
	}
	// Header line ("goroutine 1 [running]:") is not function-call
	// shaped so the per-line sanitiser leaves it as-is.
	if !strings.Contains(got, "goroutine 1 [running]:") {
		t.Errorf("non-tuple header line should not be touched; got:\n%s", got)
	}
}

// TestRuntimeFrameTupleAtomRe_ExtendedAtoms confirms #7: bool / null
// sentinels in different runtimes / decimal / scientific notation
// are now treated as atoms, so all-atom tuples in non-Go panics
// (Rust / Python / Java / sanitizer) get scrubbed too.
func TestRuntimeFrameTupleAtomRe_ExtendedAtoms(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string // expected output ("" for no change)
	}{
		{name: "go nil + hex", line: "main.Foo(nil, 0xc0000)", want: "main.Foo(...)"},
		{name: "rust null", line: "rust_panic(null)", want: "rust_panic(...)"},
		{name: "python None", line: "wrap(None, None)", want: "wrap(...)"},
		{name: "go bool", line: "Bar(true, false)", want: "Bar(...)"},
		{name: "decimal float", line: "Baz(3.14, -1.5)", want: "Baz(...)"},
		{name: "scientific", line: "Big(1.5e+09, -2.0E-3)", want: "Big(...)"},
		{name: "string preserved", line: "Foo(\"hello\", 42)", want: "Foo(\"hello\", 42)"},
		{name: "non-atom mixed", line: "Foo(0x1, someVar)", want: "Foo(0x1, someVar)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeRuntimeFrameTupleLine(tc.line)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBundleHasAuthoritativeCrashFrames_ResolutionThreshold confirms
// #6: the gate now requires ≥2 resolved files OR ≥50% resolution
// rate. Single-resolve bundles (Java basename glob hit 1 of 5
// frames) fall through so the raw log stays visible for the LLM.
func TestBundleHasAuthoritativeCrashFrames_ResolutionThreshold(t *testing.T) {
	mk := func(resolved []string, frameCount int) *types.LogBundle {
		b := &types.LogBundle{
			Meta:          types.LogMeta{Signals: []types.LogSignal{types.SignalPanic}},
			ResolvedFiles: resolved,
		}
		if frameCount > 0 {
			frames := make([]types.LogFrame, frameCount)
			b.Errors = []types.LogError{{Frames: frames}}
		}
		return b
	}
	cases := []struct {
		name   string
		bundle *types.LogBundle
		want   bool
	}{
		{name: "no signal", bundle: &types.LogBundle{ResolvedFiles: []string{"a"}}, want: false},
		{name: "1 of 5 (low rate)", bundle: mk([]string{"a"}, 5), want: false},
		{name: "1 of 1 (full rate, single)", bundle: mk([]string{"a"}, 1), want: true},
		{name: "2 of 5 (≥2 absolute)", bundle: mk([]string{"a", "b"}, 5), want: true},
		{name: "3 of 5 (≥50%)", bundle: mk([]string{"a", "b", "c"}, 5), want: true},
		{name: "header-only crash", bundle: mk([]string{"a"}, 0), want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := bundleHasAuthoritativeCrashFrames(tc.bundle); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// =====================================================================
// Phase 1.L0 (2026-05-08) — multi-repo active-set advisory tests
// =====================================================================

func TestFormatMultiRepoActiveSetAdvisory_SingleRepoSuppressed(t *testing.T) {
	ac := &types.AgentContext{
		SubRepos:                      []types.SubRepoSnapshot{{RootRel: "."}},
		MultiRepoInactivePreviewCount: 2,
	}
	if got := formatMultiRepoActiveSetAdvisory(ac); got != "" {
		t.Errorf("single-repo workspace must produce empty advisory, got %q", got)
	}
}

func TestFormatMultiRepoActiveSetAdvisory_MultiRepoNoPending(t *testing.T) {
	ac := &types.AgentContext{
		SubRepos: []types.SubRepoSnapshot{
			{RootRel: "repo-a", PrimaryLangs: []string{"Go"}},
			{RootRel: "repo-b", PrimaryLangs: []string{"Python"}},
		},
		PendingSubRepos:               nil,
		MultiRepoInactivePreviewCount: 2,
	}
	got := formatMultiRepoActiveSetAdvisory(ac)
	if got == "" {
		t.Fatalf("multi-repo advisory must render, got empty")
	}
	if !strings.Contains(got, "Active sub-repos") {
		t.Errorf("advisory missing active header: %q", got)
	}
	if !strings.Contains(got, "repo-a") || !strings.Contains(got, "repo-b") {
		t.Errorf("advisory must list both active sub-repos: %q", got)
	}
	if strings.Contains(got, "Out of active set") {
		t.Errorf("no inactive should not render the inactive header: %q", got)
	}
}

func TestFormatMultiRepoActiveSetAdvisory_RendersFocusSource(t *testing.T) {
	ac := &types.AgentContext{
		SubRepos: []types.SubRepoSnapshot{
			{RootRel: "repo-a", PrimaryLangs: []string{"Go"}},
			{RootRel: "repo-b", PrimaryLangs: []string{"Python"}},
		},
		PendingSubRepos:               []string{"repo-b"},
		MultiRepoInactivePreviewCount: 2,
		MultiRepoFocusDecision: &types.MultiRepoFocusDecision{
			Source: types.MultiRepoFocusSourceModelRecommended,
		},
	}
	got := formatMultiRepoActiveSetAdvisory(ac)
	if !strings.Contains(got, "Scope source: selected from a compact workspace-topology pre-scan") {
		t.Fatalf("advisory should explain model-recommended focus source, got:\n%s", got)
	}
}

func TestFormatMultiRepoActiveSetAdvisory_RendersRuntimeArtifactDeferredSource(t *testing.T) {
	ac := &types.AgentContext{
		SubRepos: []types.SubRepoSnapshot{
			{RootRel: "repo-a", PrimaryLangs: []string{"Go"}},
			{RootRel: "repo-b", PrimaryLangs: []string{"Python"}},
		},
		PendingSubRepos:               []string{"repo-b"},
		MultiRepoInactivePreviewCount: 2,
		MultiRepoFocusDecision: &types.MultiRepoFocusDecision{
			Source: types.MultiRepoFocusSourceRuntimeArtifactDeferred,
		},
	}
	got := formatMultiRepoActiveSetAdvisory(ac)
	if !strings.Contains(got, "source-code routing is deferred") ||
		!strings.Contains(got, "attached runtime artifact") {
		t.Fatalf("advisory should explain runtime-artifact-deferred focus source, got:\n%s", got)
	}
}

func TestFormatMultiRepoActiveSetAdvisory_TruncatesInactivePreview(t *testing.T) {
	ac := &types.AgentContext{
		SubRepos: []types.SubRepoSnapshot{
			{RootRel: "repo-a", PrimaryLangs: []string{"Go"}},
			{RootRel: "repo-b", PrimaryLangs: []string{"Python"}},
			{RootRel: "repo-c", PrimaryLangs: []string{"Rust"}},
			{RootRel: "repo-d", PrimaryLangs: []string{"TypeScript"}},
			{RootRel: "repo-e", PrimaryLangs: []string{"Kotlin"}},
		},
		// active = repo-a only; rest pending.
		PendingSubRepos:               []string{"repo-b", "repo-c", "repo-d", "repo-e"},
		MultiRepoInactivePreviewCount: 2,
	}
	got := formatMultiRepoActiveSetAdvisory(ac)
	// Active section shows repo-a.
	if !strings.Contains(got, "- repo-a (Go)") {
		t.Errorf("active list missing repo-a: %q", got)
	}
	// Preview shows repo-b + repo-c (alphabetical, count=2).
	if !strings.Contains(got, "- repo-b (Python)") || !strings.Contains(got, "- repo-c (Rust)") {
		t.Errorf("inactive preview missing first 2 alphabetical entries: %q", got)
	}
	// Truncation marker for the remaining 2.
	if !strings.Contains(got, "... and 2 more") {
		t.Errorf("inactive preview must surface truncation marker: %q", got)
	}
	// repo-d / repo-e (beyond preview) MUST NOT be listed verbatim.
	if strings.Contains(got, "repo-d") || strings.Contains(got, "repo-e") {
		t.Errorf("inactive preview must respect cap and not leak beyond it: %q", got)
	}
	// Recovery prose present (no LLM-facing slash command literal — LLM
	// surfaces the gap to the user in plain prose, the user adjusts).
	if !strings.Contains(got, "adjust the workspace scope") {
		t.Errorf("advisory must surface user-adjusts-workspace tip: %q", got)
	}
	// Auto-prefix policy hint present.
	if !strings.Contains(got, "first unique match") {
		t.Errorf("advisory must surface auto-prefix policy: %q", got)
	}
}

func TestFormatMultiRepoActiveSetAdvisory_FinalizerHidesInactiveNames(t *testing.T) {
	ac := &types.AgentContext{
		AgentName: types.AgentFinalizer,
		SubRepos: []types.SubRepoSnapshot{
			{RootRel: "repo-a", PrimaryLangs: []string{"Go"}},
			{RootRel: "repo-b", PrimaryLangs: []string{"Python"}},
			{RootRel: "repo-c", PrimaryLangs: []string{"Rust"}},
		},
		PendingSubRepos:               []string{"repo-b", "repo-c"},
		MultiRepoInactivePreviewCount: 2,
	}
	got := formatMultiRepoActiveSetAdvisory(ac)
	if !strings.Contains(got, "- repo-a (Go)") {
		t.Fatalf("finalizer advisory must still list active scope: %q", got)
	}
	if strings.Contains(got, "repo-b") || strings.Contains(got, "repo-c") ||
		strings.Contains(got, "Out of active set") ||
		strings.Contains(got, "adjust the workspace scope") {
		t.Fatalf("finalizer prompt must not leak inactive sub-repo names or scope-retry prose: %q", got)
	}
}

func TestFormatMultiRepoActiveSetAdvisory_ZeroCountFallsBackToConfigDefault(t *testing.T) {
	ac := &types.AgentContext{
		SubRepos: []types.SubRepoSnapshot{
			{RootRel: "active-a"},
			{RootRel: "inactive-x"},
			{RootRel: "inactive-y"},
			{RootRel: "inactive-z"},
		},
		PendingSubRepos:               []string{"inactive-x", "inactive-y", "inactive-z"},
		MultiRepoInactivePreviewCount: 0, // not stamped → fallback
	}
	got := formatMultiRepoActiveSetAdvisory(ac)
	// Default is 2 → first two inactive show, third is in "and 1 more".
	if !strings.Contains(got, "inactive-x") || !strings.Contains(got, "inactive-y") {
		t.Errorf("default preview should show first 2 inactive: %q", got)
	}
	if strings.Contains(got, "inactive-z") {
		t.Errorf("default preview must not exceed default count: %q", got)
	}
	if !strings.Contains(got, "... and 1 more") {
		t.Errorf("expected truncation marker for remaining 1: %q", got)
	}
}

func TestFormatMultiRepoActiveSetAdvisory_SkipsRepoScopedWrite(t *testing.T) {
	ac := &types.AgentContext{
		Mode: types.ModeApply,
		SubRepos: []types.SubRepoSnapshot{
			{RootRel: "bindings-py", PrimaryLangs: []string{"Python"}},
			{RootRel: "client-ts", PrimaryLangs: []string{"TypeScript"}},
		},
		ActiveSubRepo: &types.SubRepoSnapshot{RootRel: "bindings-py"},
	}
	if got := formatMultiRepoActiveSetAdvisory(ac); got != "" {
		t.Fatalf("repo-scoped write should not render parent multi-repo path guidance: %q", got)
	}
}
