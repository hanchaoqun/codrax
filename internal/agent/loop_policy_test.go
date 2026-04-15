package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// loop_policy_test.go pins the throttle / dedup / budget / idle-reset
// semantics of loopPolicyState.Apply. Every test exercises Apply in
// isolation against synthetic LoopSignal + LoopObservation inputs so
// each rule stays verifiable without any evaluator wiring.
//
// The task explicitly called out three scenarios that must pass:
//
//   - consecutive duplicate hints must not be injected twice;
//   - exceeding the budget must force a stop (soft-stop) or drop
//     the hint (mid-loop);
//   - a new high-confidence tool result must reset the idle streak
//     automatically, even when the evaluator returns a bare
//     Progress=false signal.
//
// All three are covered below, plus enough adjacent cases to make the
// policy rules grep-visible from either side.

// newTestPolicyState constructs a loopPolicyState with explicit
// policy values. Exported as a helper so every test can dial in the
// exact threshold it wants to exercise.
func newTestPolicyState(p LoopPolicy) *loopPolicyState {
	return newLoopPolicyState(p)
}

// midLoopObs / softStopObs build minimal LoopObservation fixtures.
// The iteration is required (the policy's throttle math uses it)
// but every other field is zero so the signal shape drives the
// test branch.
func midLoopObs(iter int, toolResults []types.ToolResult) LoopObservation {
	return LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      iter,
		AllToolResults: toolResults,
	}
}

func softStopObsFixture(iter int, toolResults []types.ToolResult) LoopObservation {
	return LoopObservation{
		Phase:          PhaseSoftStop,
		Iteration:      iter,
		AllToolResults: toolResults,
	}
}

// -----------------------------------------------------------------------------
// Dedup: consecutive duplicate hints must drop the second
// -----------------------------------------------------------------------------

// TestLoopPolicy_DedupDropsConsecutiveDuplicates is the first scenario
// the task named: "consecutive duplicate hints must not be injected
// twice in a row".
func TestLoopPolicy_DedupDropsConsecutiveDuplicates(t *testing.T) {
	s := newTestPolicyState(LoopPolicy{
		MinInjectInterval: 0, // disable throttle
		MaxMidLoopInjects: 0, // disable budget
	})

	sig := LoopSignal{
		HintRequested: true,
		Hint:          "the exact same message twice",
		HintKey:       "foo.duplicate-key",
	}

	first := s.Apply(PhaseMidLoop, midLoopObs(5, nil), sig)
	if first.Outcome != OutcomeInjectHint {
		t.Fatalf("first hint should inject, got %s (%s)", first.Outcome, first.Reason)
	}
	if first.Hint != sig.Hint {
		t.Errorf("first hint body lost on wire: got %q", first.Hint)
	}

	second := s.Apply(PhaseMidLoop, midLoopObs(6, nil), sig)
	if second.Outcome != OutcomeContinue {
		t.Errorf("duplicate hint should be dropped, got %s", second.Outcome)
	}
	if !strings.Contains(second.Reason, "deduped") {
		t.Errorf("drop reason should mention dedup, got %q", second.Reason)
	}
}

// TestLoopPolicy_DedupDoesNotBlockDifferentKeys verifies a different
// HintKey is NOT caught by dedup. Two hints with distinct keys can
// fire back-to-back as long as throttle and budget allow.
func TestLoopPolicy_DedupDoesNotBlockDifferentKeys(t *testing.T) {
	s := newTestPolicyState(LoopPolicy{MinInjectInterval: 0})

	first := s.Apply(PhaseMidLoop, midLoopObs(5, nil), LoopSignal{
		HintRequested: true, Hint: "A", HintKey: "key.A",
	})
	second := s.Apply(PhaseMidLoop, midLoopObs(6, nil), LoopSignal{
		HintRequested: true, Hint: "B", HintKey: "key.B",
	})

	if first.Outcome != OutcomeInjectHint || second.Outcome != OutcomeInjectHint {
		t.Errorf("distinct keys should both inject: first=%s second=%s",
			first.Outcome, second.Outcome)
	}
}

// TestLoopPolicy_DedupEmptyKeyNeverMatches verifies empty HintKey
// short-circuits dedup entirely. This matches the production
// contract: an evaluator that doesn't care about dedup can leave
// HintKey blank and the policy stays out of the way.
func TestLoopPolicy_DedupEmptyKeyNeverMatches(t *testing.T) {
	s := newTestPolicyState(LoopPolicy{MinInjectInterval: 0})

	for i := 0; i < 3; i++ {
		r := s.Apply(PhaseMidLoop, midLoopObs(10+i, nil), LoopSignal{
			HintRequested: true, Hint: "same body, empty key",
			// HintKey deliberately empty.
		})
		if r.Outcome != OutcomeInjectHint {
			t.Errorf("empty-key hint %d should inject, got %s", i, r.Outcome)
		}
	}
}

// -----------------------------------------------------------------------------
// Throttle: MinInjectInterval gaps consecutive injections
// -----------------------------------------------------------------------------

// TestLoopPolicy_ThrottleGapsConsecutiveHints exercises the
// MinInjectInterval check. Two hints fired too close together
// must have the second dropped.
func TestLoopPolicy_ThrottleGapsConsecutiveHints(t *testing.T) {
	s := newTestPolicyState(LoopPolicy{MinInjectInterval: 3})

	sig := LoopSignal{HintRequested: true, Hint: "hint", HintKey: "k1"}
	sig2 := LoopSignal{HintRequested: true, Hint: "hint2", HintKey: "k2"}

	if r := s.Apply(PhaseMidLoop, midLoopObs(5, nil), sig); r.Outcome != OutcomeInjectHint {
		t.Fatalf("first inject at iter 5 should pass, got %s", r.Outcome)
	}
	// Second hint at iter 6 — only 1 iter later, below the 3-iter floor.
	if r := s.Apply(PhaseMidLoop, midLoopObs(6, nil), sig2); r.Outcome != OutcomeContinue {
		t.Errorf("second inject at iter 6 should be throttled, got %s", r.Outcome)
	}
	// Third hint at iter 8 — now 3 iters later, should pass.
	if r := s.Apply(PhaseMidLoop, midLoopObs(8, nil), sig2); r.Outcome != OutcomeInjectHint {
		t.Errorf("inject at iter 8 (3 iters later) should pass, got %s (%s)",
			r.Outcome, r.Reason)
	}
}

// -----------------------------------------------------------------------------
// Budget: PhaseMidLoop drops, PhaseSoftStop force-stops
// -----------------------------------------------------------------------------

// TestLoopPolicy_BudgetDropsExcessMidLoopHints is the second scenario
// the task named: "budget exceeded must force-stop (soft-stop) or
// drop (mid-loop)". This half covers the mid-loop case.
func TestLoopPolicy_BudgetDropsExcessMidLoopHints(t *testing.T) {
	s := newTestPolicyState(LoopPolicy{
		MinInjectInterval: 0, // isolate the budget rule
		MaxMidLoopInjects: 2,
	})

	for i := 0; i < 2; i++ {
		sig := LoopSignal{
			HintRequested: true,
			Hint:          "ok",
			HintKey:       "k" + string(rune('A'+i)),
		}
		if r := s.Apply(PhaseMidLoop, midLoopObs(i, nil), sig); r.Outcome != OutcomeInjectHint {
			t.Fatalf("inject %d should pass, got %s", i, r.Outcome)
		}
	}

	// Third inject — budget exhausted. Mid-loop drops the hint but
	// keeps the loop alive.
	r := s.Apply(PhaseMidLoop, midLoopObs(2, nil), LoopSignal{
		HintRequested: true, Hint: "over budget", HintKey: "kC",
	})
	if r.Outcome != OutcomeContinue {
		t.Errorf("mid-loop over-budget should continue with drop, got %s", r.Outcome)
	}
	if !strings.Contains(r.Reason, "budget exhausted") {
		t.Errorf("drop reason should mention budget, got %q", r.Reason)
	}
}

// TestLoopPolicy_BudgetForceStopsExcessSoftStop is the other half:
// PhaseSoftStop over-budget force-stops the loop. The difference
// matters because continuing past the budget is an option for
// mid-loop ("just don't inject") but meaningless for soft-stop
// (the LLM is already stopped, so "drop the hint" is the same as
// "accept the stop").
func TestLoopPolicy_BudgetForceStopsExcessSoftStop(t *testing.T) {
	s := newTestPolicyState(LoopPolicy{
		MinInjectInterval: 0,
		MaxContinuations:  2,
	})

	for i := 0; i < 2; i++ {
		sig := LoopSignal{
			HintRequested: true,
			Hint:          "keep going",
			HintKey:       "cont" + string(rune('A'+i)),
		}
		if r := s.Apply(PhaseSoftStop, softStopObsFixture(i, nil), sig); r.Outcome != OutcomeInjectHint {
			t.Fatalf("continuation %d should pass, got %s", i, r.Outcome)
		}
	}

	// Third — budget exhausted → force-stop.
	r := s.Apply(PhaseSoftStop, softStopObsFixture(2, nil), LoopSignal{
		HintRequested: true, Hint: "over", HintKey: "contC",
	})
	if r.Outcome != OutcomeStop {
		t.Errorf("soft-stop over-budget should stop, got %s", r.Outcome)
	}
	if !strings.Contains(r.Reason, "continuation budget") {
		t.Errorf("stop reason should mention continuation budget, got %q", r.Reason)
	}
}

// -----------------------------------------------------------------------------
// Idle-streak + tool-result growth: the "reset on new tool result"
// scenario the task named by example
// -----------------------------------------------------------------------------

// TestLoopPolicy_IdleStreakCountsSilentRounds walks the counter up
// across three idle rounds and verifies the force-stop fires exactly
// when the threshold is reached.
func TestLoopPolicy_IdleStreakCountsSilentRounds(t *testing.T) {
	s := newTestPolicyState(LoopPolicy{IdleStopThreshold: 3})

	// Three consecutive bare signals — no tools, no progress, no hint.
	var last LoopResult
	for i := 0; i < 3; i++ {
		last = s.Apply(PhaseMidLoop, midLoopObs(i, nil), LoopSignal{})
	}
	if last.Outcome != OutcomeStop {
		t.Errorf("3rd idle round should force stop, got %s (%s)", last.Outcome, last.Reason)
	}
	if !strings.Contains(last.Reason, "idle streak") {
		t.Errorf("stop reason should mention idle streak, got %q", last.Reason)
	}
}

// TestLoopPolicy_IdleStreakResetsOnNewToolResults is the third
// scenario the task named: "when a new high-confidence tool result
// arrives the idle count must reset". The policy auto-detects this
// by comparing obs.AllToolResults length to the running count,
// so the evaluator's signal can stay bare Progress=false and the
// policy still does the right thing.
func TestLoopPolicy_IdleStreakResetsOnNewToolResults(t *testing.T) {
	s := newTestPolicyState(LoopPolicy{IdleStopThreshold: 3})

	// Two idle rounds, no tool results.
	s.Apply(PhaseMidLoop, midLoopObs(0, nil), LoopSignal{})
	s.Apply(PhaseMidLoop, midLoopObs(1, nil), LoopSignal{})
	if s.idleStreak != 2 {
		t.Fatalf("idle streak should be 2 after two silent rounds, got %d", s.idleStreak)
	}

	// Third round: a new tool result arrives. Evaluator still
	// returns a bare signal — the policy must auto-reset the
	// counter on the tool-result growth.
	newTools := []types.ToolResult{{ToolName: "grep", Success: true}}
	r := s.Apply(PhaseMidLoop, midLoopObs(2, newTools), LoopSignal{})
	if r.Outcome == OutcomeStop {
		t.Errorf("tool-result growth should reset idle streak; got force-stop: %s", r.Reason)
	}
	if s.idleStreak != 0 {
		t.Errorf("idle streak should reset to 0 after tool growth, got %d", s.idleStreak)
	}
}

// TestLoopPolicy_IdleStreakResetsOnExplicitProgress verifies the
// complementary path: when the evaluator explicitly returns
// Progress=true without any new tool results, the policy still
// resets the idle counter. This is the "I found a useful piece of
// content in the LLM's reply even though it called no tools" path.
func TestLoopPolicy_IdleStreakResetsOnExplicitProgress(t *testing.T) {
	s := newTestPolicyState(LoopPolicy{IdleStopThreshold: 3})

	s.Apply(PhaseMidLoop, midLoopObs(0, nil), LoopSignal{})
	s.Apply(PhaseMidLoop, midLoopObs(1, nil), LoopSignal{})
	if s.idleStreak != 2 {
		t.Fatalf("setup: idle streak should be 2, got %d", s.idleStreak)
	}

	// Explicit Progress=true → reset, even with no tool-result
	// growth and no hint.
	s.Apply(PhaseMidLoop, midLoopObs(2, nil), LoopSignal{Progress: true})
	if s.idleStreak != 0 {
		t.Errorf("explicit Progress=true should reset idle streak, got %d", s.idleStreak)
	}
}

// -----------------------------------------------------------------------------
// Explicit stop: evaluator has the final word on termination
// -----------------------------------------------------------------------------

// TestLoopPolicy_ExplicitStopHonoredImmediately verifies the
// evaluator's StopRequested is always honored, bypassing throttle
// and budget. Used by evaluators that detect terminal conditions
// (answer complete, hard budget exhausted) and need to halt the
// loop without waiting for the idle streak to catch up.
func TestLoopPolicy_ExplicitStopHonoredImmediately(t *testing.T) {
	s := newTestPolicyState(LoopPolicy{
		MinInjectInterval: 3,
		MaxContinuations:  10,
		IdleStopThreshold: 5,
	})

	r := s.Apply(PhaseMidLoop, midLoopObs(0, nil), LoopSignal{
		StopRequested: true,
		StopReason:    "terminal evidence collected",
	})
	if r.Outcome != OutcomeStop {
		t.Errorf("explicit stop should always terminate, got %s", r.Outcome)
	}
	if !strings.Contains(r.Reason, "terminal evidence collected") {
		t.Errorf("stop reason should surface StopReason, got %q", r.Reason)
	}
}

// -----------------------------------------------------------------------------
// Soft-stop default: no hint, no stop → accept soft-stop
// -----------------------------------------------------------------------------

// TestLoopPolicy_SoftStopEmptySignalTerminates verifies that a
// bare-signal soft-stop observation terminates the loop naturally.
// This is the happy-path case: LLM finished its work and the
// evaluator has nothing to add.
func TestLoopPolicy_SoftStopEmptySignalTerminates(t *testing.T) {
	s := newTestPolicyState(LoopPolicy{})

	r := s.Apply(PhaseSoftStop, softStopObsFixture(5, nil), LoopSignal{})
	if r.Outcome != OutcomeStop {
		t.Errorf("empty soft-stop signal should accept termination, got %s", r.Outcome)
	}
	if !strings.Contains(r.Reason, "no continuation") {
		t.Errorf("stop reason should say 'no continuation', got %q", r.Reason)
	}
}

// -----------------------------------------------------------------------------
// Default policy sanity
// -----------------------------------------------------------------------------

// TestDefaultLoopPolicy_HasHistoricalValues pins the DefaultLoopPolicy
// numbers so a future tweak to the constants surfaces as a clearly
// intentional change rather than a silent drift from the historical
// "fire every 3 iters, force-stop after 2 idle rounds" behavior.
func TestDefaultLoopPolicy_HasHistoricalValues(t *testing.T) {
	p := DefaultLoopPolicy()
	if p.MinInjectInterval != 3 {
		t.Errorf("MinInjectInterval = %d, want 3", p.MinInjectInterval)
	}
	if p.MaxContinuations != 5 {
		t.Errorf("MaxContinuations = %d, want 5", p.MaxContinuations)
	}
	if p.MaxMidLoopInjects != 6 {
		t.Errorf("MaxMidLoopInjects = %d, want 6", p.MaxMidLoopInjects)
	}
	if p.IdleStopThreshold != 2 {
		t.Errorf("IdleStopThreshold = %d, want 2", p.IdleStopThreshold)
	}
}
