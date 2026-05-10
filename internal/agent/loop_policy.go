package agent

import (
	"fmt"
	"hash/fnv"
	"regexp"

	"github.com/hanchaoqun/codrax/internal/llm"
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

	// IdenticalToolCallAfterSuccessStreak caps byte-identical
	// tool_use calls (same Name + same Params) at iter N+1 when the
	// iter N tool call SUCCEEDED. 0 disables the check. Default 1:
	// stop at the first identical repeat — a successful tool
	// result already delivered what the LLM needed, a verbatim
	// re-emit means the LLM is confused about state (e.g. the
	// AnswerDocument was already written).
	IdenticalToolCallAfterSuccessStreak int

	// IdenticalToolCallAfterFailureStreak caps byte-identical
	// tool_use calls when the previous tool call FAILED. 0 disables
	// the check. Default 3: allow 2 retries (the LLM may need a
	// turn or two to digest Fix α's aggregated error feedback), but
	// stop on the 3rd identical repeat — non-learning LLMs start
	// burning quota at that point. trace 1776450670620195562 showed
	// the exact pattern: iter=0 rejected → iter=1 identical
	// rejected → the 3rd attempt would have been the force-stop.
	IdenticalToolCallAfterFailureStreak int

	// IdenticalErrorStreak caps consecutive rejections that share
	// the same error class (first line of the failed tool Summary)
	// regardless of payload bytes. 0 disables the check. Default 3:
	// trace 1776453454793969437 exposed a payload-different-but-
	// error-identical failure mode — the LLM varied Chinese
	// rationale/summary text each iteration but kept filling the
	// same stray boolean{} on a step_list call, so every rejection
	// carried "shape=step_list forbids boolean{}" as the first line
	// but byte-level hash varied. The byte-identical detector above
	// missed it; this complementary gate catches it. Fires on
	// PhaseMidLoop after a failed LastToolResult; ignores successes.
	IdenticalErrorStreak int

	// MaxPerKeyInjects caps the number of mid-loop hint injections
	// that share the same HintKey within a single dispatch. 0
	// disables the cap. Default 5: legitimate progress signals
	// rarely need to fire more than 5 times for the same key (e.g.
	// "read these unread files" + 4 follow-ups as the LLM batches
	// reads). Beyond that the LLM has either already addressed the
	// underlying issue OR is structurally unable to address it; in
	// either case continuing to inject the same hint is spam.
	//
	// Sweep digest 2026-05-10: s1b hit midloop_inject=29 in a
	// single dispatch with 35 explorer iters. The same hint key
	// repeated dozens of times because BypassBudget=true bypassed
	// the global MaxMidLoopInjects gate. This per-key cap fires
	// REGARDLESS of BypassBudget so even "important" hints can't
	// flood the prompt.
	//
	// 2026-05-10 P2.
	MaxPerKeyInjects int
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
		MinInjectInterval:                   3,
		MaxContinuations:                    5,
		MaxMidLoopInjects:                   6,
		IdleStopThreshold:                   2,
		IdenticalToolCallAfterSuccessStreak: 1,
		IdenticalToolCallAfterFailureStreak: 3,
		IdenticalErrorStreak:                3,
		// MaxPerKeyInjects=5 — even BypassBudget hints stop after 5
		// repeats of the same HintKey. See LoopPolicy.MaxPerKeyInjects
		// for forensic anchor.
		MaxPerKeyInjects: 5,
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

	// lastMidLoopInjectIter / lastSoftStopInjectIter record the
	// most recently accepted hint injection per phase. Keeping the
	// throttle buckets separate prevents a PhaseMidLoop hint from
	// accidentally suppressing a later PhaseSoftStop repair hint
	// (or vice versa) when both happen within the global interval.
	// -1 means "no hint injected yet so the MinInjectInterval
	// check is a no-op" for that phase.
	lastMidLoopInjectIter  int
	lastSoftStopInjectIter int

	// continuations counts soft-stop hints already accepted.
	continuations int

	// midLoopInjects counts mid-loop hints already accepted.
	midLoopInjects int

	// perKeyMidLoopInjects counts mid-loop hints accepted PER
	// HintKey within this dispatch. Used to enforce MaxPerKeyInjects.
	// Distinct from midLoopInjects which counts the cross-key total.
	// Empty map until the first inject; lazy-initialised to keep
	// the zero-value loopPolicyState working.
	//
	// 2026-05-10 P2.
	perKeyMidLoopInjects map[string]int

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

	// lastToolCallHash is the fnv-64 hash of (Name+Params) across
	// the previous iteration's tool_use blocks. Zero before the
	// first observation. Used by the identical-call detector to
	// force-stop quota-burn loops where a non-learning LLM resends
	// byte-identical payloads.
	lastToolCallHash uint64

	// identicalToolCallStreak counts consecutive iterations whose
	// tool_use hash matches lastToolCallHash. Reset to 0 on any
	// different payload. Compared against the shape-dependent
	// threshold:
	//   - previous call succeeded → IdenticalToolCallAfterSuccessStreak
	//   - previous call failed    → IdenticalToolCallAfterFailureStreak
	identicalToolCallStreak int

	// lastToolCallSucceeded carries the Success bit of the previous
	// iteration's LastToolResult so the identical-call detector can
	// pick the success-vs-failure threshold. false when no previous
	// call or previous failed.
	lastToolCallSucceeded bool

	// lastErrorHash is the fnv-64 hash of the FIRST LINE of the
	// previous iteration's failed-tool Summary. Used by the
	// identical-error streak detector to catch payload-different
	// but structurally-identical rejection loops (e.g. the LLM
	// varies Chinese prose each iteration but keeps the same
	// "shape=X forbids Y" structural mistake). Zero when no prior
	// failure observed.
	lastErrorHash uint64

	// identicalErrorStreak counts consecutive iterations whose
	// previous-iter rejection error matched lastErrorHash. Reset
	// on success or on a different error class.
	identicalErrorStreak int
}

// newLoopPolicyState constructs a fresh counter set under the given
// policy. Uses per-phase inject indices = -1 so the
// MinInjectInterval check works correctly at iteration 0 (no
// previous inject yet).
func newLoopPolicyState(p LoopPolicy) *loopPolicyState {
	return &loopPolicyState{
		policy:                 p,
		lastMidLoopInjectIter:  -1,
		lastSoftStopInjectIter: -1,
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

func (s *loopPolicyState) lastInjectIterForPhase(phase LoopPhase) int {
	switch phase {
	case PhaseSoftStop:
		return s.lastSoftStopInjectIter
	default:
		return s.lastMidLoopInjectIter
	}
}

func (s *loopPolicyState) setLastInjectIterForPhase(phase LoopPhase, iter int) {
	switch phase {
	case PhaseSoftStop:
		s.lastSoftStopInjectIter = iter
	default:
		s.lastMidLoopInjectIter = iter
	}
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

	// Step 1c — identical-tool-call streak. Trace 1776450670620195562
	// exposed the quota-burn pattern: emit_answer_document rejected
	// on iter=0, LLM resent byte-identical payload on iter=1 (same
	// forbidden field + same citations + same prose), got the same
	// rejection, and would have repeated indefinitely if the OpenAI
	// 429 quota hadn't intervened.
	//
	// The threshold branches on the previous iteration's tool
	// result: after SUCCESS, any identical repeat means the LLM is
	// confused about state and should stop immediately (default
	// threshold 1); after FAILURE, a couple of retries are legit
	// (the LLM may need a turn to digest Fix α's aggregated error
	// feedback) so the default threshold is 3 (allow 2 retries,
	// stop on the 3rd).
	//
	// Only fires at PhaseMidLoop with non-empty tool_use blocks;
	// PhaseSoftStop carries no tool calls by contract.
	if phase == PhaseMidLoop && len(obs.Response.ToolCalls) > 0 {
		h := hashToolCalls(obs.Response.ToolCalls)
		if s.lastToolCallHash != 0 && h == s.lastToolCallHash {
			s.identicalToolCallStreak++
		} else {
			s.identicalToolCallStreak = 0
		}
		s.lastToolCallHash = h
		var threshold int
		var mode string
		if s.lastToolCallSucceeded {
			threshold = s.policy.IdenticalToolCallAfterSuccessStreak
			mode = "after success"
		} else {
			threshold = s.policy.IdenticalToolCallAfterFailureStreak
			mode = "after failure"
		}
		if threshold > 0 && s.identicalToolCallStreak >= threshold {
			return LoopResult{
				Outcome: OutcomeStop,
				Reason: fmt.Sprintf("identical tool call repeated %d time(s) %s; stopping to avoid quota burn",
					s.identicalToolCallStreak, mode),
			}
		}
	}
	// Record whether THIS iteration's tool call succeeded so the
	// NEXT Apply call can pick the correct threshold. Obs.LastToolResult
	// is the result of the call that just ran; nil on the first
	// iteration before any tool executed.
	if obs.LastToolResult != nil {
		s.lastToolCallSucceeded = obs.LastToolResult.Success
	}

	// Step 1d — identical-error streak. Complements step 1c: the
	// byte-identical tool-call gate misses payload-different but
	// structurally-identical rejection loops (trace
	// 1776453454793969437: LLM varied Chinese rationale / summary
	// each iteration but kept filling the same stray boolean{}).
	// Hashing the FIRST LINE of a failed Summary captures the
	// error class ("shape=X forbids Y") without false-negatives
	// from prose decoration around it. Skips success cases and
	// no-tool-result cases.
	if phase == PhaseMidLoop && obs.LastToolResult != nil && !obs.LastToolResult.Success {
		// Session-22 follow-up: normalize the hashed error class so
		// per-item numeric indices (items[4]:, steps[12]:, (got 3),
		// …) do not make two structurally-identical rejection
		// messages look like different error classes. Without this,
		// the LLM can batch-retry with 5 items instead of 4 (or
		// rearrange order) and the streak counter resets every time
		// — defeating the entire identical-error-streak safety net.
		// logtri_custom 2026-04-21 exercise showed this pattern:
		// 10+ iters of identical "line_start is required" errors
		// across different items, none detected as a streak.
		firstLine := firstLineOfString(obs.LastToolResult.Summary)
		eh := hashString(normalizeErrorClassForHash(firstLine))
		if s.lastErrorHash != 0 && eh == s.lastErrorHash {
			s.identicalErrorStreak++
		} else {
			s.identicalErrorStreak = 0
		}
		s.lastErrorHash = eh
		if s.policy.IdenticalErrorStreak > 0 && s.identicalErrorStreak >= s.policy.IdenticalErrorStreak {
			return LoopResult{
				Outcome: OutcomeStop,
				Reason: fmt.Sprintf("same error class repeated %d time(s): %q; stopping to avoid quota burn",
					s.identicalErrorStreak, truncForReason(firstLine, 120)),
			}
		}
	} else if obs.LastToolResult != nil && obs.LastToolResult.Success {
		// Success breaks the streak — the LLM converged.
		s.identicalErrorStreak = 0
		s.lastErrorHash = 0
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
		lastInjectIter := s.lastInjectIterForPhase(phase)
		if !sig.BypassThrottle && s.policy.MinInjectInterval > 0 && lastInjectIter >= 0 &&
			obs.Iteration-lastInjectIter < s.policy.MinInjectInterval {
			return LoopResult{
				Outcome: OutcomeContinue,
				Reason: fmt.Sprintf("hint throttled (last at iter %d, min interval %d)",
					lastInjectIter, s.policy.MinInjectInterval),
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
			if !sig.BypassBudget && s.policy.MaxMidLoopInjects > 0 && s.midLoopInjects >= s.policy.MaxMidLoopInjects {
				return LoopResult{
					Outcome: OutcomeContinue,
					Reason: fmt.Sprintf("mid-loop inject budget exhausted (%d/%d)",
						s.midLoopInjects, s.policy.MaxMidLoopInjects),
				}
			}
			// Per-key cap (2026-05-10 P2). Fires REGARDLESS of
			// BypassBudget so even "important" hints can't flood
			// the prompt with the same nag. Empty HintKey is
			// treated as a generic bucket — a flood of unkeyed
			// hints from the same evaluator branch still trips
			// the cap because the key collides under "".
			if s.policy.MaxPerKeyInjects > 0 && s.perKeyMidLoopInjects[sig.HintKey] >= s.policy.MaxPerKeyInjects {
				return LoopResult{
					Outcome: OutcomeContinue,
					Reason: fmt.Sprintf("per-key mid-loop inject cap reached (key=%q, count=%d/%d)",
						sig.HintKey, s.perKeyMidLoopInjects[sig.HintKey], s.policy.MaxPerKeyInjects),
				}
			}
			if !sig.BypassBudget {
				s.midLoopInjects++
			}
			// Per-key counter is unconditional (BypassBudget can
			// only bypass the global, not the per-key spam guard).
			if s.perKeyMidLoopInjects == nil {
				s.perKeyMidLoopInjects = make(map[string]int, 4)
			}
			s.perKeyMidLoopInjects[sig.HintKey]++
		}
		s.setLastInjectIterForPhase(phase, obs.Iteration)
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

// hashToolCalls builds an fnv-64 hash over (Name, Params) of every
// tool_use block in a response. Used by the identical-call streak
// detector to compare consecutive iterations without retaining the
// full payloads (they can be large). ID is deliberately excluded:
// the OpenAI / Anthropic SDKs assign fresh IDs per request, so two
// semantically-identical calls get different IDs and the hash must
// normalize past them. Order matters — a reordering of parallel
// tool_use blocks between iterations is a real behaviour change
// and should NOT hash-equal.
func hashToolCalls(calls []llm.ToolCall) uint64 {
	if len(calls) == 0 {
		return 0
	}
	h := fnv.New64a()
	for _, c := range calls {
		_, _ = h.Write([]byte(c.Name))
		_, _ = h.Write([]byte{0x1f})
		_, _ = h.Write(c.Params)
		_, _ = h.Write([]byte{0x1e})
	}
	return h.Sum64()
}

// hashString is the fnv-64 hash of a string. Used by the identical-
// error streak detector to compare error classes cheaply across
// iterations without retaining the full text.
func hashString(s string) uint64 {
	if s == "" {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// firstLineOfString returns the content up to the first newline
// (exclusive). Used to extract the error class from a failed tool
// Summary for streak detection — the first line carries
// "shape=X forbids Y" or similar structural messages without the
// prose decoration that follows.
func firstLineOfString(s string) string {
	if i := indexByteZero(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// errorClassNumericFieldRe strips per-item numeric noise that makes
// two structurally-identical rejection messages look different
// across retries. The patterns are chosen from the actual emit-tool
// error surface: items[N] / steps[N] / symbols[N] / citations[N]
// indices, plus "(got N)" and "line X" reporters that quote the
// offending input. Replacing them with a `[N]` / `#` placeholder
// collapses the error class without losing the structural key phrase
// (e.g. "line_start is required" stays intact).
var errorClassNumericFieldRe = regexp.MustCompile(`(?:items|steps|symbols|citations)\[\d+\]|\(got \d+\)|line_?(?:start|end)?=\d+|line \d+|\(\d+\)|:\d+-\d+|:\d+`)

// normalizeErrorClassForHash rewrites an error line so two failures
// that share the same structural problem (e.g. "line_start required"
// on different item indices) collapse to the same hash. Enables the
// identical-error-streak detector at step 1d to actually trigger on
// per-item reject loops like logtri_custom's emit_evidence pattern.
//
// Conservative rewrite rules:
//
//   - items[N] / steps[N] / symbols[N] / citations[N] → items[#] etc.
//     The KIND of container matters for error class (items[#] vs.
//     steps[#] are different problems); the INDEX does not.
//   - (got N) → (got #). Preserves "value was wrong" sense.
//   - line N / line_start=N / line_end=N → line #. Same logic.
//   - :N or :N-M line-suffixes at end of path references → :# / :#-#.
//
// Rules intentionally leave error KEY PHRASES untouched — "is
// required", "must be > 0", "unknown kind", "forbidden", etc. The
// goal is to collapse numeric noise, not to merge semantically
// different errors.
func normalizeErrorClassForHash(s string) string {
	return errorClassNumericFieldRe.ReplaceAllStringFunc(s, func(match string) string {
		// Keep the container-kind prefix (items / steps / symbols /
		// citations) so different-class errors stay distinct; only
		// the index becomes a placeholder.
		switch {
		case len(match) > 0 && match[0] == '(':
			// Covers "(got N)" and bare "(N)" — both collapse to
			// "(got #)" since both represent a numeric value the
			// error is quoting back to the caller. Using the same
			// placeholder keeps the hash-collapse symmetric with
			// the common emit-tool error phrasing.
			return "(got #)"
		case len(match) > 0 && match[0] == ':':
			if i := indexByteZero(match[1:], '-'); i >= 0 {
				return ":#-#"
			}
			return ":#"
		case len(match) >= 4 && match[:4] == "line":
			// line / line_start=N / line_end=N / line N.
			// Collapse the whole token to "line #".
			return "line #"
		default:
			// items[N] / steps[N] / symbols[N] / citations[N] form.
			if br := indexByteZero(match, '['); br >= 0 {
				return match[:br] + "[#]"
			}
			return match
		}
	})
}

// indexByteZero avoids the strings import here — loop_policy.go
// stays close to the hot path. Equivalent to strings.IndexByte(s, b).
func indexByteZero(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// truncForReason trims a string to max runes followed by "…" if
// needed. Used to keep LoopResult.Reason messages readable when
// the underlying error text is long.
func truncForReason(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
