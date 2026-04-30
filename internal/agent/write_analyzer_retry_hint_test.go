package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestWriteAnalyzer_BuildInitialInstruction_RendersHint pins the
// commit 13 #1 fix: SetAnalyzerRetryHint set by runWriteAnalyzePhase
// must surface in the next dispatch's prompt as "## Previous
// attempt rejected".
func TestWriteAnalyzer_BuildInitialInstruction_RendersHint(t *testing.T) {
	mu := types.NewMutableState("test")
	mu.SetAnalyzerRetryHint("missing required field task.summary")
	ctx := &types.AgentContext{Mutable: mu}
	eval := &writeAnalyzerEvaluator{}
	got := eval.BuildInitialInstruction(ctx, nil)
	if !strings.Contains(got, "Previous attempt rejected") {
		t.Errorf("expected '## Previous attempt rejected' header; got %q", got)
	}
	if !strings.Contains(got, "missing required field task.summary") {
		t.Errorf("expected hint text in prompt; got %q", got)
	}
	// Consume-once: a second call should NOT re-render the hint
	// (Mutable.AnalyzerRetryHint was reset by the first call).
	got2 := eval.BuildInitialInstruction(ctx, nil)
	if got2 != "" {
		t.Errorf("second call should produce empty prompt (hint consumed); got %q", got2)
	}
}

// TestWriteAnalyzer_BuildInitialInstruction_NoHintReturnsEmpty
// pins the no-retry path: a fresh dispatch with no prior failure
// produces an empty supplement (skill owns the full prompt).
func TestWriteAnalyzer_BuildInitialInstruction_NoHintReturnsEmpty(t *testing.T) {
	mu := types.NewMutableState("test")
	ctx := &types.AgentContext{Mutable: mu}
	eval := &writeAnalyzerEvaluator{}
	if got := eval.BuildInitialInstruction(ctx, nil); got != "" {
		t.Errorf("no hint should yield empty supplement; got %q", got)
	}
}

// TestWriteAnalyzer_BuildInitialInstruction_NilSafe pins
// defensive paths.
func TestWriteAnalyzer_BuildInitialInstruction_NilSafe(t *testing.T) {
	eval := &writeAnalyzerEvaluator{}
	if got := eval.BuildInitialInstruction(nil, nil); got != "" {
		t.Errorf("nil ctx should yield empty supplement; got %q", got)
	}
	if got := eval.BuildInitialInstruction(&types.AgentContext{}, nil); got != "" {
		t.Errorf("nil mutable should yield empty supplement; got %q", got)
	}
}
