package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRunReviewerWithDeadline_NoDeadlineAndNoEnvelopePassesThrough(t *testing.T) {
	called := false
	out := runReviewerWithDeadline(context.Background(), reviewerSlotSelfConsistency, 0.4, 0,
		func(ctx context.Context) []types.Violation {
			called = true
			if _, ok := ctx.Deadline(); ok {
				t.Fatalf("expected no deadline when parent has none")
			}
			return nil
		})
	if !called {
		t.Fatalf("fn must be invoked when parent has no deadline")
	}
	if out != nil {
		t.Fatalf("expected nil, got %+v", out)
	}
}

func TestRunReviewerWithDeadline_NoDeadlineUsesFallbackEnvelope(t *testing.T) {
	out := runReviewerWithDeadline(context.Background(), reviewerSlotSelfConsistency, 0.5, 30*time.Second,
		func(ctx context.Context) []types.Violation {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatalf("expected fallback envelope to install a deadline")
			}
			remaining := time.Until(deadline)
			if remaining <= 0 || remaining > 16*time.Second {
				t.Fatalf("deadline remaining=%s, want about 15s", remaining)
			}
			return []types.Violation{{Kind: types.ViolSelfContradiction}}
		})
	if len(out) != 1 || out[0].Kind != types.ViolSelfContradiction {
		t.Fatalf("expected reviewer output to pass through, got %+v", out)
	}
}

func TestRunReviewerWithDeadline_ZeroFractionDisablesGuard(t *testing.T) {
	parent, cancel := context.WithDeadline(context.Background(), time.Now().Add(100*time.Millisecond))
	defer cancel()
	called := false
	runReviewerWithDeadline(parent, reviewerSlotSelfConsistency, 0, 30*time.Second,
		func(ctx context.Context) []types.Violation {
			called = true
			if d, _ := ctx.Deadline(); !d.Equal(time.Time{}) {
				if d != (func() time.Time { dd, _ := parent.Deadline(); return dd }()) {
					t.Fatalf("ctx deadline must equal parent deadline when fraction=0")
				}
			}
			return []types.Violation{{Kind: types.ViolSelfContradiction}}
		})
	if !called {
		t.Fatalf("fn must run when fraction=0 (legacy path)")
	}
}

func TestRunReviewerWithDeadline_TimeoutDowngradesToAdvisory(t *testing.T) {
	parent, cancel := context.WithDeadline(context.Background(), time.Now().Add(200*time.Millisecond))
	defer cancel()
	out := runReviewerWithDeadline(parent, reviewerSlotSelfConsistency, 0.5, 0,
		func(ctx context.Context) []types.Violation {
			// Simulate a slow reviewer that misses the deadline.
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
				t.Fatalf("test guard: reviewer should have been cancelled")
			}
			return []types.Violation{{Kind: types.ViolSelfContradiction}}
		})
	if out != nil {
		t.Fatalf("deadline-exceeded reviewer output must be dropped, got %+v", out)
	}
}

func TestRunReviewerWithDeadline_WithinBudgetReturnsViolations(t *testing.T) {
	// Budget = 60s * 0.4 = 24s > reviewerMinBudget (10s), so dispatch proceeds.
	parent, cancel := context.WithDeadline(context.Background(), time.Now().Add(60*time.Second))
	defer cancel()
	out := runReviewerWithDeadline(parent, reviewerSlotSemanticQuality, 0.4, 0,
		func(ctx context.Context) []types.Violation {
			return []types.Violation{
				{Kind: types.ViolAnswerSemanticUnderfilled},
			}
		})
	if len(out) != 1 || out[0].Kind != types.ViolAnswerSemanticUnderfilled {
		t.Fatalf("expected reviewer output to pass through, got %+v", out)
	}
}

func TestRunReviewerWithDeadline_BudgetBelowFloorSkips(t *testing.T) {
	parent, cancel := context.WithDeadline(context.Background(), time.Now().Add(500*time.Millisecond))
	defer cancel()
	called := false
	out := runReviewerWithDeadline(parent, reviewerSlotSelfConsistency, 0.4, 0,
		func(ctx context.Context) []types.Violation {
			called = true
			return nil
		})
	if called {
		t.Fatalf("fn must NOT run when computed budget is below reviewerMinBudget")
	}
	if out != nil {
		t.Fatalf("expected nil when guard skipped dispatch, got %+v", out)
	}
}
