// Package agent — explorer_budget_exhausted_signal_test.go (2026-05-10).
//
// Fix B of the post-sweep optimization: when the explore-budget
// gate refuses an LLM tool call with "explore budget exhausted for
// tool <name>", the explorer evaluator emits a per-tool one-shot
// nudge steering the LLM to alternative tools or to emit, instead
// of silently re-attempting the same exhausted tool.
//
// Forensic anchor: 2026-05-10 sweep digest s5b explorer iter=2 — a
// 6-call parallel batch of read_file all hard-rejected with the
// same "budget exhausted" error. The LLM only learned the budget
// state from the per-call rejections AFTER the iter completed, by
// which point 6 LLM-emitted tool calls had been wasted.
package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func budgetExhaustedResult(tool string) *types.ToolResult {
	return &types.ToolResult{
		ToolName:   tool,
		Success:    false,
		Repair:     exploreBudgetExhaustedRepair(tool),
		Refinement: exploreBudgetExhaustedRefinement(tool),
		Summary: "explore budget exhausted for tool \"" + tool + "\": per-tool or overall cap reached. " +
			"Use a different tool or stop the investigation.",
	}
}

func TestPostBudgetExhaustedSignal_NoLastResult_NoOp(t *testing.T) {
	e := &explorerEvaluator{}
	if got := e.postBudgetExhaustedSignal(LoopObservation{}); got.HintRequested {
		t.Errorf("nil LastToolResult: expected no hint; got %+v", got)
	}
}

func TestPostBudgetExhaustedSignal_SuccessfulCall_NoOp(t *testing.T) {
	e := &explorerEvaluator{}
	obs := LoopObservation{
		LastToolResult: &types.ToolResult{
			ToolName: "read_file",
			Success:  true,
			Summary:  "explore budget exhausted for tool \"read_file\": ...",
		},
	}
	if got := e.postBudgetExhaustedSignal(obs); got.HintRequested {
		t.Errorf("successful tool result must not trigger nudge even if Summary contains marker; got %+v", got)
	}
}

func TestPostBudgetExhaustedSignal_NonBudgetError_NoOp(t *testing.T) {
	e := &explorerEvaluator{}
	obs := LoopObservation{
		LastToolResult: &types.ToolResult{
			ToolName: "read_file",
			Success:  false,
			Summary:  "read failed: open /missing/file: no such file or directory",
		},
	}
	if got := e.postBudgetExhaustedSignal(obs); got.HintRequested {
		t.Errorf("non-budget error must not trigger nudge; got %+v", got)
	}
}

func TestPostBudgetExhaustedSignal_SummaryOnlyBudgetText_NoOp(t *testing.T) {
	e := &explorerEvaluator{}
	obs := LoopObservation{
		LastToolResult: &types.ToolResult{
			ToolName: "read_file",
			Success:  false,
			Summary:  "explore budget exhausted for tool \"read_file\": per-tool or overall cap reached.",
		},
	}
	if got := e.postBudgetExhaustedSignal(obs); got.HintRequested {
		t.Errorf("summary-only budget text must not trigger loop-control; got %+v", got)
	}
}

func TestPostBudgetExhaustedSignal_FiresWithCorrectShape(t *testing.T) {
	e := &explorerEvaluator{}
	obs := LoopObservation{
		LastToolResult: budgetExhaustedResult("read_file"),
	}
	got := e.postBudgetExhaustedSignal(obs)
	if !got.HintRequested {
		t.Fatalf("expected nudge; got %+v", got)
	}
	if got.HintKey != "explorer.budget_exhausted.read_file" {
		t.Errorf("HintKey = %q, want explorer.budget_exhausted.read_file", got.HintKey)
	}
	if !got.BypassThrottle {
		t.Errorf("nudge must set BypassThrottle=true so MinInjectInterval cannot delay it")
	}
	if !got.BypassBudget {
		t.Errorf("nudge must set BypassBudget=true so the structural directive isn't blocked by mid-loop budget")
	}
	for _, want := range []string{"read_file", "grep", "repo_map"} {
		if !strings.Contains(got.Hint, want) {
			t.Errorf("hint missing token %q (alternatives steering); got %q", want, got.Hint)
		}
	}
}

func TestPostBudgetExhaustedSignal_PerToolOneShot(t *testing.T) {
	// Same tool budget refused twice within the dispatch — the
	// nudge must fire ONCE so the LLM doesn't get spammed.
	e := &explorerEvaluator{}
	obs := LoopObservation{LastToolResult: budgetExhaustedResult("read_file")}
	first := e.postBudgetExhaustedSignal(obs)
	if !first.HintRequested {
		t.Fatal("first call should fire nudge")
	}
	second := e.postBudgetExhaustedSignal(obs)
	if second.HintRequested {
		t.Errorf("second call same tool: expected one-shot guard; got %+v", second)
	}
}

func TestPostBudgetExhaustedSignal_DistinctToolsFireIndependently(t *testing.T) {
	// read_file and grep have independent budgets — exhaustion
	// of one must not silence the other.
	e := &explorerEvaluator{}
	rfRes := e.postBudgetExhaustedSignal(LoopObservation{
		LastToolResult: budgetExhaustedResult("read_file"),
	})
	if !rfRes.HintRequested {
		t.Fatal("read_file: expected nudge")
	}
	grepRes := e.postBudgetExhaustedSignal(LoopObservation{
		LastToolResult: budgetExhaustedResult("grep"),
	})
	if !grepRes.HintRequested {
		t.Fatal("grep: distinct tool should fire its own nudge")
	}
	if rfRes.HintKey == grepRes.HintKey {
		t.Errorf("HintKey collision: read_file and grep nudges share %q", rfRes.HintKey)
	}
}

func TestPostBudgetExhaustedSignal_HintIsLanguageNeutral(t *testing.T) {
	// R6 audit: nudge must not leak internal pipeline terminology
	// or stage names.
	e := &explorerEvaluator{}
	got := e.postBudgetExhaustedSignal(LoopObservation{
		LastToolResult: budgetExhaustedResult("read_file"),
	})
	if !got.HintRequested {
		t.Fatal("expected nudge")
	}
	for _, banned := range []string{
		"explorer", "extractor", "finalizer", "analyzer",
		"BusContext", "MutableState", "ExploreBudget",
		"L1 gate", "L2 gate", "L3", "L4",
	} {
		if strings.Contains(got.Hint, banned) {
			t.Errorf("internal term %q leaked into LLM-facing hint: %q", banned, got.Hint)
		}
	}
}

func TestPostBudgetExhaustedSignal_UnknownToolFallsBackToGenericAlternatives(t *testing.T) {
	// A tool we don't have an alternatives entry for should still
	// fire the nudge with a sensible generic suggestion (rather
	// than crashing on a nil map lookup).
	e := &explorerEvaluator{}
	got := e.postBudgetExhaustedSignal(LoopObservation{
		LastToolResult: budgetExhaustedResult("some_future_tool"),
	})
	if !got.HintRequested {
		t.Fatal("expected nudge for unknown tool")
	}
	if !strings.Contains(got.Hint, "some_future_tool") {
		t.Errorf("hint should still name the exhausted tool; got %q", got.Hint)
	}
	if !strings.Contains(got.Hint, "different read tool") && !strings.Contains(got.Hint, "emit_") {
		t.Errorf("hint should still steer to alternatives or emit; got %q", got.Hint)
	}
}
