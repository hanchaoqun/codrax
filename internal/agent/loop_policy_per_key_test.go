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

	"github.com/hanchaoqun/codrax/internal/types"
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

// breakDedupApply fires a one-off hint with a fresh HintKey so the
// next target call won't trip the loop-policy's lastAcceptedKey
// dedup gate (loop_policy.go:516). Required for the cap-saturation
// tests, where the target hits its per-key cap BEFORE the
// lastAcceptedKey gate naturally rolls over; a fresh breaker key
// guarantees the cap-rejection branch executes (otherwise the
// dedup gate intercepts first and CapSaturatedHintKey is never
// populated).
func breakDedupApply(state *loopPolicyState, iter int, freshKey string) {
	state.Apply(PhaseMidLoop, LoopObservation{Iteration: iter}, LoopSignal{HintRequested: true, Hint: "z", HintKey: freshKey})
}

// TestLoopPolicy_PerKeyCap_ReportsCapSaturatedKeyOnceThenDedups pins the
// docs/design/post_phase2a_forensic_followups.md §2.1.G #4 + #7
// escalation: the FIRST cap-reached rejection for a given key
// populates LoopResult.CapSaturatedHintKey so the BaseAgent caller
// can emit a one-shot notice + record a LearningFailure + surface a
// typed marker on Mut. Subsequent cap rejections for the same key
// still continue the loop but leave the field empty so the dock and
// Mut are not spammed with repeat events.
func TestLoopPolicy_PerKeyCap_ReportsCapSaturatedKeyOnceThenDedups(t *testing.T) {
	p := DefaultLoopPolicy()
	// Use MaxPerKeyInjects=10 for the breaker family so a series of
	// fresh breaker_N keys never hit their own cap (only the target
	// is at risk of saturating). cap=2 for the target keeps the
	// trace short.
	p.MaxPerKeyInjects = 10
	p.MinInjectInterval = 0
	p.MaxMidLoopInjects = 0
	p.IdleStopThreshold = 0
	p.IdenticalToolCallAfterSuccessStreak = 0
	p.IdenticalToolCallAfterFailureStreak = 0
	p.IdenticalErrorStreak = 0
	state := newLoopPolicyState(p)

	target := "explorer.mid-loop.closure-repair"

	// Two accepted target injects bring perKey[target] to its
	// per-target cap of 2 (overridden inline below by hard-capping
	// the policy after the warm-up).
	state.Apply(PhaseMidLoop, LoopObservation{Iteration: 0}, LoopSignal{HintRequested: true, Hint: "x", HintKey: target})
	breakDedupApply(state, 1, "breaker_1")
	state.Apply(PhaseMidLoop, LoopObservation{Iteration: 2}, LoopSignal{HintRequested: true, Hint: "x", HintKey: target})
	breakDedupApply(state, 3, "breaker_2")

	// Lower the policy cap to the warm-up count so the next target
	// call is the FIRST cap-reached rejection.
	state.policy.MaxPerKeyInjects = 2

	// First cap reject — CapSaturatedHintKey MUST carry target.
	res1 := state.Apply(PhaseMidLoop, LoopObservation{Iteration: 4}, LoopSignal{HintRequested: true, Hint: "x", HintKey: target})
	if res1.Outcome != OutcomeContinue {
		t.Fatalf("first cap reject: outcome = %v, want OutcomeContinue", res1.Outcome)
	}
	if res1.CapSaturatedHintKey != target {
		t.Fatalf("first cap reject: CapSaturatedHintKey = %q, want %q", res1.CapSaturatedHintKey, target)
	}

	// Re-attempt target after a fresh breaker — should still cap
	// reject, but CapSaturatedHintKey MUST be empty (already reported).
	breakDedupApply(state, 5, "breaker_3")
	res2 := state.Apply(PhaseMidLoop, LoopObservation{Iteration: 6}, LoopSignal{HintRequested: true, Hint: "x", HintKey: target})
	if res2.Outcome != OutcomeContinue {
		t.Fatalf("second cap reject: outcome = %v, want OutcomeContinue", res2.Outcome)
	}
	if res2.CapSaturatedHintKey != "" {
		t.Fatalf("second cap reject: CapSaturatedHintKey should be empty after dedup, got %q", res2.CapSaturatedHintKey)
	}

	// Third re-attempt — same dedup.
	breakDedupApply(state, 7, "breaker_4")
	res3 := state.Apply(PhaseMidLoop, LoopObservation{Iteration: 8}, LoopSignal{HintRequested: true, Hint: "x", HintKey: target})
	if res3.CapSaturatedHintKey != "" {
		t.Fatalf("third cap reject: CapSaturatedHintKey should stay empty, got %q", res3.CapSaturatedHintKey)
	}
}

// TestLoopPolicy_PerKeyCap_CapSaturatedKeyDedupsIndependentlyPerKey
// pins that the dedup is keyed PER hint key, so a second key
// saturating later still gets its own one-shot CapSaturatedHintKey
// (otherwise an early-saturated key would mask later, semantically
// distinct cap events).
func TestLoopPolicy_PerKeyCap_CapSaturatedKeyDedupsIndependentlyPerKey(t *testing.T) {
	p := DefaultLoopPolicy()
	p.MaxPerKeyInjects = 10
	p.MinInjectInterval = 0
	p.MaxMidLoopInjects = 0
	p.IdleStopThreshold = 0
	p.IdenticalToolCallAfterSuccessStreak = 0
	p.IdenticalToolCallAfterFailureStreak = 0
	p.IdenticalErrorStreak = 0
	state := newLoopPolicyState(p)

	// Accept one of each key under the spacious cap.
	state.Apply(PhaseMidLoop, LoopObservation{Iteration: 0}, LoopSignal{HintRequested: true, Hint: "a", HintKey: "keyA"})
	state.Apply(PhaseMidLoop, LoopObservation{Iteration: 1}, LoopSignal{HintRequested: true, Hint: "b", HintKey: "keyB"})

	// Tighten policy so the existing counts (=1) are AT cap, and the
	// next call for each key is a cap rejection.
	state.policy.MaxPerKeyInjects = 1

	// Saturate keyA — first rejection MUST carry keyA. Break dedup
	// first because keyB was the last accepted (lastAcceptedKey=keyB
	// so a keyA call would not hit the dedup gate, but be explicit).
	breakDedupApply(state, 2, "breaker_1")
	resA1 := state.Apply(PhaseMidLoop, LoopObservation{Iteration: 3}, LoopSignal{HintRequested: true, Hint: "a", HintKey: "keyA"})
	if resA1.CapSaturatedHintKey != "keyA" {
		t.Fatalf("keyA first cap reject: CapSaturatedHintKey = %q, want keyA", resA1.CapSaturatedHintKey)
	}

	// Saturate keyB after — first rejection MUST carry keyB
	// independently (not masked by the earlier keyA saturation).
	breakDedupApply(state, 4, "breaker_2")
	resB1 := state.Apply(PhaseMidLoop, LoopObservation{Iteration: 5}, LoopSignal{HintRequested: true, Hint: "b", HintKey: "keyB"})
	if resB1.CapSaturatedHintKey != "keyB" {
		t.Fatalf("keyB first cap reject: CapSaturatedHintKey = %q, want keyB", resB1.CapSaturatedHintKey)
	}

	// Re-saturating keyA AGAIN MUST stay empty (already reported).
	breakDedupApply(state, 6, "breaker_3")
	resA2 := state.Apply(PhaseMidLoop, LoopObservation{Iteration: 7}, LoopSignal{HintRequested: true, Hint: "a", HintKey: "keyA"})
	if resA2.CapSaturatedHintKey != "" {
		t.Fatalf("keyA second cap reject: CapSaturatedHintKey should be empty after dedup, got %q", resA2.CapSaturatedHintKey)
	}
}

// TestMutableState_MidLoopHintExhaustions_RecordsTypedMarker pins the
// docs/design/post_phase2a_forensic_followups.md §2.1.G #4 + #7
// success-criteria channel: when the loop-policy escalates a cap
// saturation, NoteMidLoopHintExhausted records a typed (HintKey,
// Reason) marker the success-criteria layer can read to pivot to
// fallback earlier than the dispatch's max-step quota. Empty
// receiver / empty key are no-ops (matches the LearningFailure
// helper's defensive shape).
func TestMutableState_MidLoopHintExhaustions_RecordsTypedMarker(t *testing.T) {
	mu := types.NewMutableState("test")
	if got := mu.MidLoopHintExhaustions(); got != nil {
		t.Errorf("fresh MutableState should have no midloop hint exhaustions; got %v", got)
	}

	mu.NoteMidLoopHintExhausted("explorer.mid-loop.closure-repair", "per-key mid-loop inject cap reached (count=5/5)")
	mu.NoteMidLoopHintExhausted("explorer.partial-read", "per-key mid-loop inject cap reached (count=5/5)")
	got := mu.MidLoopHintExhaustions()
	if len(got) != 2 {
		t.Fatalf("expected 2 exhaustions, got %d: %+v", len(got), got)
	}
	if got[0].HintKey != "explorer.mid-loop.closure-repair" {
		t.Errorf("first record HintKey drift: %q", got[0].HintKey)
	}
	if !strings.Contains(got[0].Reason, "count=5/5") {
		t.Errorf("first record Reason should carry cap counter: %q", got[0].Reason)
	}
	if got[1].HintKey != "explorer.partial-read" {
		t.Errorf("second record HintKey drift: %q", got[1].HintKey)
	}

	// Idempotent: repeating the same key MUST NOT grow the slice.
	mu.NoteMidLoopHintExhausted("explorer.mid-loop.closure-repair", "any reason")
	if again := mu.MidLoopHintExhaustions(); len(again) != 2 {
		t.Errorf("repeated key MUST be idempotent, got len=%d: %+v", len(again), again)
	}

	// Defensive: empty key is a no-op; nil receiver does not panic.
	mu.NoteMidLoopHintExhausted("", "ignored")
	if final := mu.MidLoopHintExhaustions(); len(final) != 2 {
		t.Errorf("empty key MUST be no-op, got len=%d", len(final))
	}
	var nilMu *types.MutableState
	nilMu.NoteMidLoopHintExhausted("key", "reason") // must not panic
	if got := nilMu.MidLoopHintExhaustions(); got != nil {
		t.Errorf("nil receiver must return nil snapshot, got %v", got)
	}
}
