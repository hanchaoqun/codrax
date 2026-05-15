package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/pterm/pterm"
)

// TestRenderer_EmitterHonorsConfiguredOutput locks the renderer's
// live-output sink contract. The post-2026-04-30 liveBar refactor
// accidentally hard-coded os.Stdout throughout the event path,
// which polluted single-shot CLI stdout with progress ticks and
// agent reasoning. Supplying a bytes.Buffer here must capture the
// live output; otherwise the renderer regressed to writing outside
// its configured sink again.
func TestRenderer_EmitterHonorsConfiguredOutput(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)
	r.StartSpinner()
	defer r.StopSpinner()

	now := time.Now()
	emit := r.Emitter()
	emit(Event{
		Kind:      EventObjectiveStarted,
		Objective: "find the entry point",
		Timestamp: now,
	})
	emit(Event{
		Kind:      EventStageStart,
		Stage:     types.StageAnalyze,
		Agent:     types.AgentAnalyzer,
		Timestamp: now,
	})
	emit(Event{
		Kind:      EventAgentReasoning,
		Agent:     types.AgentAnalyzer,
		Iteration: 0,
		Reasoning: "Reasoning should surface through the configured writer.",
		Timestamp: now,
	})

	out := buf.String()
	if out == "" {
		t.Fatal("expected renderer to write live output to configured writer")
	}
	if !strings.Contains(out, "configured writer") {
		t.Fatalf("expected reasoning text in configured writer output, got %q", out)
	}
}

func TestRenderer_ReasoningDefaultsToFullText(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)

	longReasoning := strings.Repeat("detail ", reasoningMaxChars/len("detail ")+10) + "TAILMARKER"
	emit := r.Emitter()
	emit(Event{
		Kind:      EventAgentReasoning,
		Agent:     types.AgentAnalyzer,
		Iteration: 0,
		Reasoning: longReasoning,
		Timestamp: time.Now(),
	})

	out := buf.String()
	if !strings.Contains(out, "TAILMARKER") {
		t.Fatalf("default reasoning output should be untruncated, got %q", out)
	}
}

func TestRenderer_ReasoningTruncateOptIn(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)
	r.SetThinkingTruncate(true)

	longReasoning := strings.Repeat("detail ", reasoningMaxChars/len("detail ")+10) + "TAILMARKER"
	emit := r.Emitter()
	emit(Event{
		Kind:      EventAgentReasoning,
		Agent:     types.AgentAnalyzer,
		Iteration: 0,
		Reasoning: longReasoning,
		Timestamp: time.Now(),
	})

	out := buf.String()
	if strings.Contains(out, "TAILMARKER") {
		t.Fatalf("opt-in reasoning truncation should hide tail marker, got %q", out)
	}
	if !strings.Contains(out, "...") {
		t.Fatalf("opt-in reasoning truncation should add ellipsis, got %q", out)
	}
}

func TestFormatReasoningStylesTagAndBodySeparately(t *testing.T) {
	pterm.EnableColor()
	defer pterm.DisableColor()

	got := formatReasoning("analyzer", types.StageAnalyze, 0, "Readable detail.", false, "zh")
	expected := "  " + statusReasoningGlyph.Sprint("⋯") + " " + statusMeta.Sprint("分析 · 第 1 轮") + " " + statusReasoningBody.Sprint("Readable detail.")
	if got != expected {
		t.Fatalf("reasoning style should keep tag muted and body readable\nwant %q\ngot  %q", expected, got)
	}
	if wantGlyph := pterm.NewStyle(pterm.FgLightCyan).Sprint("⋯"); !strings.Contains(got, wantGlyph) {
		t.Fatalf("reasoning glyph should be bright but isolated\nwant glyph %q\ngot        %q", wantGlyph, got)
	}
	if wantBody := pterm.NewStyle(pterm.FgWhite, pterm.Fuzzy).Sprint("Readable detail."); !strings.Contains(got, wantBody) {
		t.Fatalf("reasoning body should use dim white to stay below answer prose while remaining readable\nwant body %q\ngot       %q", wantBody, got)
	}
	if plain := stripAnsiEscapes(got); plain != "  ⋯ 分析 · 第 1 轮 Readable detail." {
		t.Fatalf("styled reasoning changed visible text: %q", plain)
	}
}

func TestFormatReasoningFallsBackWithoutExposingUnknownAgent(t *testing.T) {
	got := formatReasoning("experimental_agent", "", 1, "Readable detail.", false, "zh")
	plain := stripAnsiEscapes(got)
	if want := "  ⋯ 模型 · 第 2 轮 Readable detail."; plain != want {
		t.Fatalf("unknown agent fallback changed\nwant %q\ngot  %q", want, plain)
	}
	if strings.Contains(plain, "experimental_agent") {
		t.Fatalf("unknown internal agent id leaked into user-facing scrollback: %q", plain)
	}
}

func TestFormatToolCallBatchSummarizesPureToolResponse(t *testing.T) {
	pterm.EnableColor()
	defer pterm.DisableColor()

	got := formatToolCallBatch("explorer", types.StageExplore, 19,
		[]string{"read_file"},
		1,
		"read_file",
		"internal/tool/apply_patch.go",
		"zh",
	)
	plain := stripAnsiEscapes(got)
	want := "  ⇢ 探索 · 第 20 轮 调用工具 read_file internal/tool/apply_patch.go"
	if plain != want {
		t.Fatalf("tool-only response line changed\nwant %q\ngot  %q", want, plain)
	}
	if wantGlyph := pterm.NewStyle(pterm.FgLightGreen).Sprint("⇢"); !strings.Contains(got, wantGlyph) {
		t.Fatalf("tool-call glyph should be bright but isolated\nwant glyph %q\ngot        %q", wantGlyph, got)
	}
}

func TestFormatToolCallBatchCompactsRepeatedTools(t *testing.T) {
	got := formatToolCallBatch("explorer", types.StageExplore, 2,
		[]string{"read_file", "read_file", "grep", "emit_evidence", "repo_map", "list_files"},
		6,
		"read_file",
		"",
		"en",
	)
	plain := stripAnsiEscapes(got)
	for _, want := range []string{"⇢ Explore · Round 3", "calling 6 tools", "read_file x2", "grep", "emit_evidence", "repo_map", "+1"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("tool batch line missing %q; got %q", want, plain)
		}
	}
}

func TestRenderer_ToolCallBatchUsesConfiguredOutput(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.SetOutput(&buf)

	emit := r.Emitter()
	emit(Event{
		Kind:          EventAgentToolCallBatch,
		Agent:         types.AgentExplorer,
		Stage:         types.StageExplore,
		Iteration:     3,
		ToolName:      "emit_evidence",
		ToolNames:     []string{"emit_evidence"},
		ToolCallCount: 1,
		Timestamp:     time.Now(),
	})

	out := stripAnsiEscapes(buf.String())
	for _, want := range []string{"⇢ 探索 · 第 4 轮", "调用工具 emit_evidence"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tool batch event must leave visible output %q; got %q", want, out)
		}
	}
}
