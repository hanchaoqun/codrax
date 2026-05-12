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

	got := formatReasoning("analyzer", 0, "Readable detail.", false)
	expected := "  " + statusMeta.Sprint("💭 [analyzer-1]") + " " + statusReasoningBody.Sprint("Readable detail.")
	if got != expected {
		t.Fatalf("reasoning style should keep tag muted and body readable\nwant %q\ngot  %q", expected, got)
	}
	if plain := stripAnsiEscapes(got); plain != "  💭 [analyzer-1] Readable detail." {
		t.Fatalf("styled reasoning changed visible text: %q", plain)
	}
}
