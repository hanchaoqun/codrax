package agent

import (
	"fmt"
)

// loop_policy.go is the throttling / dedup / budget layer that sits
// between a LoopController's raw detection signal and BaseAgent's
// loop actions. Before this file existed, every evaluator duplicated
// the same three concerns inline:
//
//   - idle-streak counting (explorer.idleStreakInDepth,
//     subExplorer.idleStreak, subExplorer.lastToolCount)
//   - hint throttling (explorer.midLoopLastInjectIter + the "fire
//     at most every 3 iters" check inside MidLoopCheck)
//   - retry budgets (explorer's continuationCount passed through,
//     answer_document_evaluator.retriesUsed / maxFinalizerCorrectionRetries)
//
// Every instance drifted slightly from the others — different reset
// points, different threshold values, different "duplicate hint"
// notions. Extracting them into a single policy that BaseAgent runs
// over a controller's raw signal gives us one grep-able contract
// plus room for evaluator-specific detection state to stay where
// it actually belongs (phase-specific bookkeeping stays on the
// evaluator; generic counters move here).
//
// Layering:
//
//     LoopController.Observe(ctx, obs) → LoopSignal    // raw
//                            ▼
//     loopPolicyState.Apply(phase, sig, iter)          // policy
//                            ▼
//                     LoopResult                        // action

// LoopPolicy is the runtime-tunable configuration for the loop-
// control layer. The zero value is NOT a safe default; use
// DefaultLoopPolicy() for the historical behavior.
type LoopPolicy struct {
	// MinInjectInterval is the minimum number of iterations between
	// two accepted hint injections. 0 disables the throttle. When
	// two hints arrive at adjacent iterations, the second is
	// dropped (Apply returns OutcomeContinue) even if every other
	// check would pass.
	MinInjectInterval int

	// MaxContinuations caps the number of PhaseSoftStop hint
	// injections accepted per dispatch. Replaces the old
	// continuationCount argument to ContinuationPrompt. 0 disables
	// the cap, allowing unbounded continuations (only the
	// evaluator's ShouldStop + MaxIterations stop the loop).
	MaxContinuations int

	// MaxMidLoopInjects caps the number of PhaseMidLoop hint
	// injections accepted per dispatch. 0 disables the cap.
	MaxMidLoopInjects int

	// IdleStopThreshold forces a stop after this many consecutive
	// iterations returned a signal with Progress=false AND
	// HintRequested=false AND StopRequested=false — i.e. the
	// controller had nothing to say. 0 disables the force-stop.
	// Replaces explorer.idleStreakInDepth≥2 and
	// subExplorer.idleStreak≥2 checks.
	IdleStopThreshold int
}

// DefaultLoopPolicy returns the policy BaseAgent installs when the
// caller leaves Dependencies.LoopPolicy as a zero value. The
// numbers are calibrated to match the pre-refactor behavior:
//
//   - MinInjectInterval=3  matches explorer.MidLoopCheck's throttle
//     cadence (mid-loop checks fire from iter≥2 onward).
//   - MaxContinuations=5   matches the worst-case observed number
//     of soft-stop continuations the explorer ever accepted before
//     forcing a stop, and sits above
//     maxFinalizerCorrectionRetries (2) so the finalizer's stricter
//     internal retries continue to win.
//   - MaxMidLoopInjects=6  matches the rough upper bound on
//     legitimate mid-loop interventions in a single explorer
//     dispatch (1-2 partial-read + 1-2 erm-gap + 1 parallelize).
//   - IdleStopThreshold=2  matches explorer.idleStreakInDepth>=2
//     and subExplorer.idleStreak>=2 exactly.
func DefaultLoopPolicy() LoopPolicy {
	return LoopPolicy{
		MinInjectInterval: 3,
		MaxContinuations:  5,
		MaxMidLoopInjects: 6,
		IdleStopThreshold: 2,
	}
}

// loopPolicyState is the per-dispatch counter set a BaseAgent holds
// while running a single Execute call. Unexported because every
// counter's lifetime is tied to a single ReAct loop; nothing
// outside the Execute method needs to read or write the running
// state, and keeping it unexported means tests access the policy
// via Apply exactly the way BaseAgent does.
type loopPolicyState struct {
	policy LoopPolicy

	// idleStreak counts consecutive Apply calls where the signal
	// had no Progress, HintRequested, or StopRequested flag set
	// AND no new tool results arrived since the last Apply call.
	// Reset to 0 on any of those signals.
	idleStreak int

	// lastInjectIter is the iteration index of the most recently
	// accepted hint injection. -1 means "no hint injected yet so
	// the MinInjectInterval check is a no-op".
	lastInjectIter int

	// continuations counts soft-stop hints already accepted.
	continuations int

	// midLoopInjects counts mid-loop hints already accepted.
	midLoopInjects int

	// lastAcceptedKey is the HintKey of the most recently accepted
	// injection. Used for dedup: when the next signal carries the
	// same non-empty HintKey, the hint is dropped regardless of
	// throttle / budget — the evaluator is re-firing the exact
	// same condition that was already addressed.
	lastAcceptedKey string

	// lastToolResultCount is the length of obs.AllToolResults at
	// the previous Apply call. Growth between observations is an
	// implicit progress signal — even if the evaluator returns a
	// bare Progress=false, a larger tool-result slice means the
	// ReAct loop executed tool calls since we last checked, and
	// the idle streak must reset. Owned by the policy so evaluators
	// no longer need their own `lastToolCount` counter (the task
	// explicitly called this out as "duplicate counter state").
	lastToolResultCount int
}

// newLoopPolicyState constructs a fresh counter set under the given
// policy. Uses lastInjectIter=-1 so the MinInjectInterval check
// works correctly at iteration 0 (no previous inject yet).
func newLoopPolicyState(p LoopPolicy) *loopPolicyState {
	return &loopPolicyState{
		policy:         p,
		lastInjectIter: -1,
	}
}

// snapshot returns the (idleStreak, continuations, midLoopInjects)
// triple in one call. BaseAgent uses this to populate the
// LoopObservation fields the evaluator sees BEFORE Apply runs, so a
// controller branching on "is this the N-th idle round?" sees the
// count that led INTO the current observation, not the count after
// this iteration's update.
func (s *loopPolicyState) snapshot() (idle, conts, midLoop int) {
	return s.idleStreak, s.continuations, s.midLoopInjects
}

// LoopOutcome is the final decision Apply returns to BaseAgent. Four
// cases per (phase, outcome) matrix exist in practice:
//
//   - PhaseMidLoop  + OutcomeContinue    → no-op, next iteration
//   - PhaseMidLoop  + OutcomeInjectHint  → append hint, next iteration
//   - PhaseMidLoop  + OutcomeStop        → force terminate the loop
//   - PhaseSoftStop + OutcomeContinue    → accept soft-stop (end loop)
//   - PhaseSoftStop + OutcomeInjectHint  → forced continuation
//   - PhaseSoftStop + OutcomeStop        → accept soft-stop (end loop)
//
// The soft-stop rows look identical on the surface because either
// OutcomeContinue or OutcomeStop at PhaseSoftStop ends the loop;
// they differ only in the log message Reason carries (one reads
// "no continuation requested", the other reads e.g. "evaluator stop").
type LoopOutcome int

const (
	// OutcomeContinue means BaseAgent should proceed to the next
	// iteration without modifying the message stream. At
	// PhaseSoftStop this value is reinterpreted as "end the loop
	// naturally" — the BaseAgent never forces a continuation on
	// its own.
	OutcomeContinue LoopOutcome = iota
	// OutcomeStop means BaseAgent should terminate the ReAct loop
	// immediately. The Reason field carries the trace string.
	OutcomeStop
	// OutcomeInjectHint means BaseAgent should append LoopResult.Hint
	// as a user-role message and proceed to the next iteration.
	OutcomeInjectHint
)

// String makes the outcome print readably in debug logs.
func (o LoopOutcome) String() string {
	switch o {
	case OutcomeContinue:
		return "continue"
	case OutcomeStop:
		return "stop"
	case OutcomeInjectHint:
		return "inject_hint"
	}
	return "unknown"
}

// LoopResult is the structured output of loopPolicyState.Apply. It
// carries the final outcome BaseAgent should act on plus optional
// metadata: Hint (only when Outcome == OutcomeInjectHint) and
// Reason (a short tag the BaseAgent.Execute debug trace surfaces).
type LoopResult struct {
	Outcome LoopOutcome
	Hint    string
	Reason  string
}

// Apply runs the throttling / dedup / budget rules against one raw
// LoopSignal, reading the observation for implicit signals
// (tool-result growth) and updating the policy state. Returns the
// final LoopResult BaseAgent should act on. Call exactly once per
// Observe — Apply mutates the policy state (idle counters, inject
// bookkeeping) and calling it twice for the same iteration would
// double-count.
//
// The obs argument is used for two things:
//   - implicit progress detection: if len(obs.AllToolResults) grew
//     since the last Apply call, reset the idle streak automatically.
//     This removes the need for evaluators to track their own
//     `lastToolCount` counter (the task explicitly called this out).
//   - iteration number: obs.Iteration drives the throttle window.
//
// Precedence (in execution order):
//
//  1. Update the idle streak. A signal with Progress=true OR
//     HintRequested=true OR StopRequested=true is "non-idle" and
//     resets the counter. Implicit: new tool results since the last
//     Apply call ALSO reset the counter, even when the explicit
//     signal is empty. Otherwise the counter increments.
//  2. Honor an explicit StopRequested immediately. The evaluator has
//     the final word on termination — we do not second-guess it
//     with throttle or budget checks.
//  3. Hint path: dedup (matching HintKey → drop), throttle
//     (MinInjectInterval not yet elapsed → drop), budget (phase cap
//     exceeded → drop for mid-loop, force-stop for soft-stop). A
//     surviving hint ticks up the phase-specific counter and
//     records the inject metadata for the next iteration's dedup
//     and throttle checks.
//  4. No hint, no stop: at PhaseSoftStop this becomes OutcomeStop
//     ("accept soft-stop"); at PhaseMidLoop this becomes
//     OutcomeContinue.
//  5. AFTER (1)-(4) have produced an outcome, the idle-streak
//     force-stop gate fires at PhaseMidLoop when the running
//     idleStreak reaches IdleStopThreshold AND the outcome was
//     OutcomeContinue. Ordering this last lets the final idle
//     round still request one last hint before we force a stop.
func (s *loopPolicyState) Apply(phase LoopPhase, obs LoopObservation, sig LoopSignal) LoopResult {
	// Step 1a — implicit progress signal from tool-result growth.
	toolGrowth := len(obs.AllToolResults) - s.lastToolResultCount
	s.lastToolResultCount = len(obs.AllToolResults)

	// Step 1b — idle streak update.
	active := sig.Progress || sig.HintRequested || sig.StopRequested || toolGrowth > 0
	if active {
		s.idleStreak = 0
	} else {
		s.idleStreak++
	}

	// Step 2 — explicit stop.
	if sig.StopRequested {
		reason := "evaluator stop"
		if sig.StopReason != "" {
			reason = "evaluator stop: " + sig.StopReason
		}
		return LoopResult{Outcome: OutcomeStop, Reason: reason}
	}

	// Step 3 — hint path (dedup → throttle → budget).
	if sig.HintRequested && sig.Hint != "" {
		if sig.HintKey != "" && sig.HintKey == s.lastAcceptedKey {
			return LoopResult{
				Outcome: OutcomeContinue,
				Reason:  "hint deduped: " + sig.HintKey,
			}
		}
		if s.policy.MinInjectInterval > 0 && s.lastInjectIter >= 0 &&
			obs.Iteration-s.lastInjectIter < s.policy.MinInjectInterval {
			return LoopResult{
				Outcome: OutcomeContinue,
				Reason: fmt.Sprintf("hint throttled (last at iter %d, min interval %d)",
					s.lastInjectIter, s.policy.MinInjectInterval),
			}
		}
		switch phase {
		case PhaseSoftStop:
			if s.policy.MaxContinuations > 0 && s.continuations >= s.policy.MaxContinuations {
				return LoopResult{
					Outcome: OutcomeStop,
					Reason: fmt.Sprintf("continuation budget exhausted (%d/%d)",
						s.continuations, s.policy.MaxContinuations),
				}
			}
			s.continuations++
		case PhaseMidLoop:
			if s.policy.MaxMidLoopInjects > 0 && s.midLoopInjects >= s.policy.MaxMidLoopInjects {
				return LoopResult{
					Outcome: OutcomeContinue,
					Reason: fmt.Sprintf("mid-loop inject budget exhausted (%d/%d)",
						s.midLoopInjects, s.policy.MaxMidLoopInjects),
				}
			}
			s.midLoopInjects++
		}
		s.lastInjectIter = obs.Iteration
		s.lastAcceptedKey = sig.HintKey
		return LoopResult{Outcome: OutcomeInjectHint, Hint: sig.Hint}
	}

	// Step 4 — no hint, no stop.
	if phase == PhaseSoftStop {
		return LoopResult{Outcome: OutcomeStop, Reason: "no continuation requested"}
	}

	// Step 5 — idle-streak force-stop (mid-loop only; the soft-stop
	// path above already terminates the loop, so the force-stop
	// would be redundant there).
	if s.policy.IdleStopThreshold > 0 && s.idleStreak >= s.policy.IdleStopThreshold {
		return LoopResult{
			Outcome: OutcomeStop,
			Reason: fmt.Sprintf("idle streak %d ≥ threshold %d",
				s.idleStreak, s.policy.IdleStopThreshold),
		}
	}
	return LoopResult{Outcome: OutcomeContinue}
}
