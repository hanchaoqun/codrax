package orchestrator

import (
	"context"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// reviewerSlot names a downstream LLM reviewer for telemetry and
// deadline accounting. Each slot consumes its own share of the
// remaining wall budget so a slow reviewer cannot eat the entire
// dispatch window.
type reviewerSlot string

const (
	reviewerSlotSelfConsistency reviewerSlot = "self_consistency"
	reviewerSlotSemanticQuality reviewerSlot = "semantic_quality"
)

// reviewerMinBudget is the floor below which the deadline guard
// declines to dispatch — paying the LLM round-trip plus prompt
// assembly cost for under 10s of wall is wasted work.
const reviewerMinBudget = 10 * time.Second

type reviewerRequestTimeoutProvider interface {
	ReviewerRequestTimeout() time.Duration
}

func reviewerRequestTimeoutEnvelope(reviewer any) time.Duration {
	if reviewer == nil {
		return 0
	}
	provider, ok := reviewer.(reviewerRequestTimeoutProvider)
	if !ok {
		return 0
	}
	return provider.ReviewerRequestTimeout()
}

// runReviewerWithDeadline wraps fn so it sees a context bounded by
// fraction*remaining_wall_budget. If the parent context has no deadline,
// callers may provide fallbackEnvelope (normally the reviewer's adapter
// RequestTimeout) so read-mode Runs without an overall wall deadline still
// bound advisory reviewer cost. If fraction <= 0, or no deadline/envelope is
// available, fn runs against parent unchanged (legacy behaviour). On deadline
// timeout, the reviewer's output is dropped and a Warning is logged; the
// dispatch continues without the reviewer's violations. This preserves shipping
// the answer when reviewer slowness would otherwise burn the wall budget.
//
// Red line: this only enforces an upper bound on reviewer wall
// cost. It does not synthesize violations, does not infer answer
// content, and does not change the typed reviewer contract. If
// fn returns violations within budget they pass through unchanged.
func runReviewerWithDeadline(
	parent context.Context,
	slot reviewerSlot,
	fraction float64,
	fallbackEnvelope time.Duration,
	fn func(context.Context) []types.Violation,
) []types.Violation {
	if parent == nil {
		parent = context.Background()
	}
	if fn == nil {
		return nil
	}
	if fraction <= 0 {
		return fn(parent)
	}
	remaining := time.Duration(0)
	if deadline, ok := parent.Deadline(); ok {
		remaining = time.Until(deadline)
		if remaining <= 0 {
			logging.Warning("[reviewer-deadline] slot=%s wall budget already exhausted, skipping dispatch", slot)
			return nil
		}
	} else if fallbackEnvelope > 0 {
		remaining = fallbackEnvelope
	} else {
		return fn(parent)
	}
	budget := time.Duration(float64(remaining) * fraction)
	if budget < reviewerMinBudget {
		logging.Warning("[reviewer-deadline] slot=%s budget=%s below floor=%s, skipping dispatch", slot, budget, reviewerMinBudget)
		return nil
	}
	subCtx, cancel := context.WithDeadline(parent, time.Now().Add(budget))
	defer cancel()
	out := fn(subCtx)
	if err := subCtx.Err(); err == context.DeadlineExceeded {
		logging.Warning("[reviewer-deadline] slot=%s exceeded budget=%s, downgrading to advisory", slot, budget)
		return nil
	}
	return out
}
