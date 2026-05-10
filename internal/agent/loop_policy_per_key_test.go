// Package agent — loop_policy_per_key_test.go (2026-05-10).
//
// P2 of the post-sweep optimization: MaxPerKeyInjects caps the
// number of mid-loop hint injections sharing the same HintKey
// within a single dispatch. Fires REGARDLESS of BypassBudget so
// "important" hints can't flood the prompt by repeating dozens of
// times with the same key.
//
// Sweep digest 2026-05-10: s1b hit midloop_inject=29 in a single
// dispatch with 35 explorer iters because BypassBudget=true
// bypassed the global MaxMidLoopInjects gate. This test pins the
// per-key cap behaviour.
package agent

import (
	"strings"
	"testing"
)

// hammer fires the same target HintKey repeatedly, alternating with
// `breakerKey` between calls so the policy's same-key dedup
// (lastAcceptedKey check at loop_policy.go:494) doesn't intercept
// before the per-key cap check. Returns the per-iteration outcomes
// for the target key only.
func hammer(state *loopPolicyState, target, breaker string, n int) []LoopOutcome {
	out := make([]LoopOutcome, 0, n)
	iter := 0
	for i := 0; i < n; i++ {
		// Inject breaker first so target won't hit lastAcceptedKey
		// dedup. We discard breaker outcome.
		breakerSig := LoopSignal{HintRequested: true, Hint: "x", HintKey: breaker}
		state.Apply(PhaseMidLoop, LoopObservation{Iteration: iter}, breakerSig)
		iter++
		// Now target hint.
		targetSig := LoopSignal{HintRequested: true, Hint: "y", HintKey: target}
		res := state.Apply(PhaseMidLoop, LoopObservation{Iteration: iter}, targetSig)
		iter++
		out = append(out, res.Outcome)
	}
	return out
}

func TestLoopPolicy_PerKeyCap_RejectsBeyondLimit(t *testing.T) {
	p := DefaultLoopPolicy()
	p.MaxPerKeyInjects = 3
	// Disable other gates so we exercise the per-key cap in
	// isolation.
	p.MinInjectInterval = 0
	p.MaxMidLoopInjects = 0
	p.IdleStopThreshold = 0
	p.IdenticalToolCallAfterSuccessStreak = 0
	p.IdenticalToolCallAfterFailureStreak = 0
	p.IdenticalErrorStreak = 0
	state := newLoopPolicyState(p)

	out := hammer(state, "explorer.spam", "explorer.alt", 5)
	for i, outcome := range out {
		if i < 3 {
			if outcome != OutcomeInjectHint {
				t.Errorf("target iter %d: expected OutcomeInjectHint; got %v", i, outcome)
			}
		} else {
			if outcome != OutcomeContinue {
				t.Errorf("target iter %d: expected per-key cap to reject; got %v", i, outcome)
			}
		}
	}
	_ = strings.Contains // keep import alive
}

// hammerBypass fires alternating HintKeys with BypassBudget=true
// on the target, so we test that BypassBudget doesn't skip the
// per-key cap.
func hammerBypass(state *loopPolicyState, target, breaker string, n int) []LoopOutcome {
	out := make([]LoopOutcome, 0, n)
	iter := 0
	for i := 0; i < n; i++ {
		breakerSig := LoopSignal{HintRequested: true, Hint: "x", HintKey: breaker, BypassBudget: true}
		state.Apply(PhaseMidLoop, LoopObservation{Iteration: iter}, breakerSig)
		iter++
		targetSig := LoopSignal{HintRequested: true, Hint: "y", HintKey: target, BypassBudget: true}
		res := state.Apply(PhaseMidLoop, LoopObservation{Iteration: iter}, targetSig)
		iter++
		out = append(out, res.Outcome)
	}
	return out
}

func TestLoopPolicy_PerKeyCap_BypassBudgetStillCapped(t *testing.T) {
	// BypassBudget can skip the global cap but NOT the per-key
	// cap — that's the whole point: stop key-flood spam.
	p := DefaultLoopPolicy()
	p.MaxPerKeyInjects = 2
	p.MinInjectInterval = 0
	p.MaxMidLoopInjects = 0
	state := newLoopPolicyState(p)

	out := hammerBypass(state, "evaluator.must-fire", "evaluator.alt", 4)
	for i, outcome := range out {
		if i < 2 && outcome != OutcomeInjectHint {
			t.Errorf("target iter %d: BypassBudget hint within cap should pass; got %v", i, outcome)
		}
		if i >= 2 && outcome != OutcomeContinue {
			t.Errorf("target iter %d: BypassBudget should NOT bypass per-key cap; got %v", i, outcome)
		}
	}
}

func TestLoopPolicy_PerKeyCap_DistinctKeysIndependentBuckets(t *testing.T) {
	p := DefaultLoopPolicy()
	p.MaxPerKeyInjects = 2
	p.MinInjectInterval = 0
	p.MaxMidLoopInjects = 0
	state := newLoopPolicyState(p)

	// Key A fires 3 times via hammer (against breaker key B).
	// First 2 fires accept; 3rd hits cap.
	outA := hammer(state, "key-A", "key-X", 3)
	if outA[0] != OutcomeInjectHint || outA[1] != OutcomeInjectHint || outA[2] != OutcomeContinue {
		t.Fatalf("key-A: expected inject,inject,continue; got %v", outA)
	}
	// Now key-B with a different breaker — should still have its
	// full quota since per-key buckets are independent.
	outB := hammer(state, "key-B", "key-Y", 2)
	for i, outcome := range outB {
		if outcome != OutcomeInjectHint {
			t.Errorf("key-B iter %d: independent bucket should still pass; got %v", i, outcome)
		}
	}
}

func TestLoopPolicy_PerKeyCap_DefaultIsFive(t *testing.T) {
	p := DefaultLoopPolicy()
	if p.MaxPerKeyInjects != 5 {
		t.Errorf("default MaxPerKeyInjects = %d, want 5", p.MaxPerKeyInjects)
	}
}

func TestLoopPolicy_PerKeyCap_ZeroDisablesCheck(t *testing.T) {
	// MaxPerKeyInjects=0 should disable the per-key gate, so the
	// cap is effectively infinite (legacy behaviour for callers
	// that opt out via yaml or per-test override).
	p := DefaultLoopPolicy()
	p.MaxPerKeyInjects = 0
	p.MinInjectInterval = 0
	p.MaxMidLoopInjects = 0
	state := newLoopPolicyState(p)
	out := hammer(state, "spam", "alt", 50)
	for i, outcome := range out {
		if outcome != OutcomeInjectHint {
			t.Fatalf("iter %d: cap=0 should disable; got %v", i, outcome)
		}
	}
}

func TestLoopPolicy_PerKeyCap_EmptyKeyBucketCounts(t *testing.T) {
	// HintKey="" hints don't trip same-key dedup (loop_policy.go:494
	// gates on `HintKey != ""`). All empty-key fires go through
	// the per-key counter under the "" bucket — confirms the
	// counter increments on empty keys too, so unkeyed evaluators
	// can't bypass the cap by leaving HintKey blank.
	p := DefaultLoopPolicy()
	p.MaxPerKeyInjects = 2
	p.MinInjectInterval = 0
	p.MaxMidLoopInjects = 0
	state := newLoopPolicyState(p)
	results := make([]LoopOutcome, 0, 4)
	for i := 0; i < 4; i++ {
		sig := LoopSignal{HintRequested: true, Hint: "x", HintKey: ""}
		res := state.Apply(PhaseMidLoop, LoopObservation{Iteration: i}, sig)
		results = append(results, res.Outcome)
	}
	for i, want := range []LoopOutcome{OutcomeInjectHint, OutcomeInjectHint, OutcomeContinue, OutcomeContinue} {
		if results[i] != want {
			t.Errorf("empty-key iter %d: got %v, want %v", i, results[i], want)
		}
	}
}
