package agent

// FRCAP (§29.12, docs/design/real_trace_campaign_20260705.md,
// 2026-07-10): the F7 empty-blocks reject breaker threshold migrated
// from a hardcoded const to a config-driven package var. Pins: the
// shipped default stays 3 byte-for-byte, the setter honors overrides,
// and non-positive values clamp back (the breaker can never be
// disabled — an unbounded identical-reject streak is exactly what it
// exists to stop).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSetFinalizerEmptyBlocksBreakerMaxStreak(t *testing.T) {
	t.Cleanup(func() { SetFinalizerEmptyBlocksBreakerMaxStreak(emptyBlocksRejectBreakerMaxStreakDefault) })

	if emptyBlocksRejectBreakerMaxStreak != 3 || emptyBlocksRejectBreakerMaxStreakDefault != 3 {
		t.Fatalf("shipped F7 threshold must stay 3, got %d (default %d)",
			emptyBlocksRejectBreakerMaxStreak, emptyBlocksRejectBreakerMaxStreakDefault)
	}
	SetFinalizerEmptyBlocksBreakerMaxStreak(5)
	if emptyBlocksRejectBreakerMaxStreak != 5 {
		t.Fatalf("setter(5) → %d", emptyBlocksRejectBreakerMaxStreak)
	}
	SetFinalizerEmptyBlocksBreakerMaxStreak(0)
	if emptyBlocksRejectBreakerMaxStreak != emptyBlocksRejectBreakerMaxStreakDefault {
		t.Fatalf("0 must clamp to default (breaker never disabled), got %d", emptyBlocksRejectBreakerMaxStreak)
	}
	SetFinalizerEmptyBlocksBreakerMaxStreak(-1)
	if emptyBlocksRejectBreakerMaxStreak != emptyBlocksRejectBreakerMaxStreakDefault {
		t.Fatalf("negative must clamp to default, got %d", emptyBlocksRejectBreakerMaxStreak)
	}
}

// TestFinalizerEmptyBlocksBreakerHonorsKnob — behaviour half: with the
// threshold raised to 4, the 4th identical reject still hints and the
// 5th trips the breaker.
func TestFinalizerEmptyBlocksBreakerHonorsKnob(t *testing.T) {
	t.Cleanup(func() { SetFinalizerEmptyBlocksBreakerMaxStreak(emptyBlocksRejectBreakerMaxStreakDefault) })
	SetFinalizerEmptyBlocksBreakerMaxStreak(4)

	mut := types.NewMutableState("q")
	ctx := &types.AgentContext{Stage: types.StageFinalize, Mutable: mut}
	e := &answerDocumentEvaluator{}
	_ = e.BuildInitialInstruction(ctx, nil)

	emptyReject := func() LoopObservation {
		return LoopObservation{
			Phase: PhaseMidLoop,
			LastToolResult: &types.ToolResult{
				ToolName: "emit_answer_document",
				Success:  false,
				Repair:   &types.ToolRepair{Code: types.ToolRepairCodeAnswerDocBlocksRequired},
			},
		}
	}
	for i := 1; i <= 4; i++ {
		if sig := e.Observe(ctx, emptyReject()); sig.StopRequested {
			t.Fatalf("reject #%d must not trip a threshold of 4: %+v", i, sig)
		}
	}
	sig := e.Observe(ctx, emptyReject())
	if !sig.StopRequested || !strings.Contains(sig.StopReason, "empty-blocks reject breaker") {
		t.Fatalf("5th identical reject must trip the raised threshold, got %+v", sig)
	}
}
