package orchestrator

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// runWriteSchedulerLoop walks the linear plan→apply→verify TaskGraph
// emitted by BuildWriteTaskGraph. Mirrors the read scheduler's
// graphState + criterion.Env machinery but skips the read-mode
// chunks (Tier1Floor, contract retry, extract dispatch, finalize
// backtrack) — write mode has none of those.
//
// Each round:
//
//  1. Build criterion.Env from current Mutable state.
//  2. readyWriteWindow returns the single ready write node (write
//     graphs are linear, so at most one is ready at a time).
//  3. Dispatch via stageMapping → stage. Pre-hook fires before
//     dispatch; post-hook after. A pre-hook error terminates the
//     Run with that error in TaskState.LastError.
//  4. SuccessCriteria check on the dispatched node. Pass: markDone.
//     Fail on verify (TestsPass / NoRegression): the only retryable
//     case — clearForReplan + requeueValidationTargets to re-run
//     plan→apply→verify with PlanningHint context. Other SC failures
//     are terminal (apply SC fail = patches didn't land, no retry
//     useful inside the same Run).
//  5. Loop until allDone or step budget exhausted.
//
// Return value is the steps consumed (counted toward o.maxSteps).
func (o *Orchestrator) runWriteSchedulerLoop(stepBudget int) int {
	g := o.busCtx.AnalysisIR.TaskGraph
	if len(g.Nodes) == 0 {
		return 0
	}
	state := newGraphState(g)
	stepsUsed := 0
	retryAttempt := 0

	buildEnv := func() criterion.Env {
		env := criterion.Env{
			IR:             o.busCtx.AnalysisIR,
			Evidence:       o.busCtx.EvidenceItems,
			AnswerSymbols:  o.busCtx.AnswerSymbols,
			AnswerChains:   o.busCtx.AnswerChains,
			ToolResults:    o.busCtx.ToolResults,
			Signals:        o.busCtx.Signals,
			ReactItersUsed: stepsUsed,
		}
		if o.busCtx.Mutable != nil {
			env.ChangePlan = o.busCtx.Mutable.ChangePlan()
			env.ChangeReport = o.busCtx.Mutable.ChangeReport()
			env.BaselineReport = o.busCtx.Mutable.BaselineReport()
			env.WriteClosure = o.busCtx.Mutable.WriteClosure()
		}
		return env
	}

	for stepsUsed < stepBudget && !state.allDone() {
		// Phase 2 cancel checkpoint between write-window dispatches
		// (plan / apply / verify). A Ctrl+C during the long apply
		// stage would otherwise wait until the agent's next ReAct
		// iter; this returns immediately with CanceledError.
		if cerr := o.checkCanceled("write_scheduler", stepsUsed); cerr != nil {
			o.busCtx.TaskState.LastError = cerr.Error()
			return stepsUsed
		}
		env := buildEnv()
		ready, blocked := state.readyWriteWindow(env)
		if len(ready) == 0 {
			if len(blocked) > 0 {
				// Every remaining node is blocked on EntryConditions
				// that won't resolve without a prior dispatch (which
				// would have produced one). This is a terminal stall —
				// surface it and bail.
				logging.Warning("[orchestrator] write scheduler stalled: %d node(s) blocked on entry conditions", len(blocked))
			}
			break
		}
		// Linear graph: only one node ready at a time. Defensive — if
		// the graph ever grew parallel branches, dispatch the first
		// and revisit the rest next round.
		n := ready[0]
		stage, err := stageMapping(g, n, true)
		if err != nil {
			logging.Error("[orchestrator] write scheduler: %s has no stage mapping: %v", n.ID, err)
			state.markFailed(n.ID)
			continue
		}

		o.busCtx.PipelineStage = stage
		o.busCtx.TaskState.Stage = stage
		state.markRunning(n.ID)
		o.emitNodeStart(n.ID)

		// SkipOnFirstVisit short-circuit: when the node opted into this
		// flag (write-mode plan node with --plan-file is the canonical
		// case), the FIRST entry is treated as success without
		// dispatching the agent. This preserves R8a (don't regenerate
		// the user-reviewed plan) while keeping the node in the graph
		// so EdgeValidationFeedback retry can re-dispatch it on a
		// later iteration. visits() returns >=1 inside markRunning, so
		// "first visit" means visits()==1.
		if n.SkipOnFirstVisit && state.visits(n.ID) == 1 {
			logging.Info("[orchestrator] skipping first visit of %s (SkipOnFirstVisit; plan loaded from disk)", n.ID)
			state.markDone(n.ID)
			o.emitNodeEnd(n.ID, true, "skipped on first visit (plan from disk)")
			continue
		}

		// /approve --skip-verify short-circuit. When the operator
		// asked codrax to apply without testing (typically because
		// integration tests need infra unavailable on this host),
		// the verify node is marked done without dispatching the
		// verifier. apply has already landed bytes in the worktree;
		// status flips to applied via the verify post-hook below
		// is bypassed too — we set it directly here so /plan list
		// reflects the truth.
		if n.Type == types.NodeVerify && o.skipVerify {
			now := time.Now()
			o.persistPlanStatus(types.PlanStatusApplied, &now)
			logging.Info("[orchestrator] verify skipped (--skip-verify); plan marked applied without test execution")
			state.markDone(n.ID)
			o.emitNodeEnd(n.ID, true, "skipped (--skip-verify)")
			continue
		}

		// Pre-hook: worktree provision, plan load, baseline capture.
		// A failure terminates the Run.
		if err := runStagePreHook(o, stage); err != nil {
			logging.Error("[orchestrator] %s pre-hook: %v", stage, err)
			o.busCtx.TaskState.LastError = err.Error()
			state.markFailed(n.ID)
			o.emitNodeEnd(n.ID, false, err.Error())
			break
		}

		stepsUsed++
		out, dispatchErr := o.dispatchStage(stage)

		if dispatchErr != nil {
			// Dispatch error path. Two distinct sub-cases:
			//   (1) Hard runtime crash with out == nil — agent panicked
			//       before producing a StageOutput. Skip the post-hook
			//       (it expects a meaningful StageOutput).
			//   (2) Structured failure with out != nil — the agent's
			//       evaluator returned an error AND a StageOutput
			//       describing it (canonical example: verifier's
			//       ParseOutput emits `out.Error = "verify failed: …"`
			//       AND a non-nil error). Mutable.ChangeReport is
			//       installed in this case, so the post-hook MUST run
			//       (saveChangeReport → report.json on disk; failure
			//       diagnosis was previously lost because we skipped
			//       it). Then take the verify→plan retry branch below
			//       instead of breaking out of the loop, so the
			//       configured retry budget actually fires.
			logging.Error("[orchestrator] %s dispatch: %v", stage, dispatchErr)
			// Transient-dispatch retry. When the underlying error is one
			// IsRetryableDispatchError accepts (HTTP 429/5xx, network
			// timeouts, stream watchdog kills, EOF, etc.), the failure
			// has nothing to do with plan / apply correctness — the LLM
			// connection blipped. Requeue the SAME node within a SEPARATE
			// transientRetryBudget so brief blips don't drain the verify→
			// plan SC retry budget. Stall plateau detection short-circuits
			// when two consecutive transient failures produce identical
			// tool-call signatures with no terminal emit between them —
			// that means the LLM is structurally stuck (e.g. empty repo
			// in plan mode), and burning more retries cannot help.
			if llm.IsRetryableDispatchError(dispatchErr) {
				sig := computeStallSignature(o.busCtx.Mutable.DispatchToolResults(), out, stage)
				if state.transientStallPlateau(n.ID, sig) {
					friendly := stallPlateauMessage(o.busCtx, stage, friendlyDispatchErr(dispatchErr))
					logging.Warning("[orchestrator] %s transient stall plateau (sig=%q) — suppressing further retry",
						stage, sig)
					o.busCtx.TaskState.LastError = friendly
					state.markFailed(n.ID)
					o.emitNodeEnd(n.ID, false, friendly)
					break
				}
				if state.transientRetryUsed < o.transientRetryBudget {
					state.recordTransientRetry()
					state.rememberTransientSignature(n.ID, sig)
					state.requeue(n.ID)
					o.busCtx.TaskState.LastError = ""
					reason := friendlyDispatchErr(dispatchErr)
					logging.Warning("[orchestrator] %s transient dispatch error; requeued for retry %d/%d (transient budget): %v",
						stage, state.transientRetryUsed, o.transientRetryBudget, reason)
					o.emit(render.Event{
						Kind:      render.EventAgentReasoning,
						Timestamp: time.Now(),
						Agent:     "orchestrator",
						Reasoning: softRetryHintMessage(o.busCtx.Language),
					})
					o.emitNodeEnd(n.ID, false, fmt.Sprintf("%s transient: retrying", stage))
					continue
				}
				logging.Warning("[orchestrator] %s transient dispatch error but transient retry budget exhausted (%d/%d); going terminal",
					stage, state.transientRetryUsed, o.transientRetryBudget)
			}
			if out == nil {
				switch stage {
				case types.StageApply:
					o.persistPlanStatus(types.PlanStatusApplyFailed, nil)
				case types.StageVerify:
					o.persistPlanStatus(types.PlanStatusVerifyFailed, nil)
				}
				// Translate typed dispatch errors to a friendlier surface
				// so the user-facing LastError names the upstream
				// condition (e.g. "upstream LLM stream stalled") instead
				// of the raw "context canceled" the watchdog leaves
				// behind. Non-typed errors pass through unchanged.
				friendly := friendlyDispatchErr(dispatchErr)
				o.busCtx.TaskState.LastError = friendly
				state.markFailed(n.ID)
				o.emitNodeEnd(n.ID, false, friendly)
				break
			}
			// Structured failure: post-hook runs (persists report +
			// status) and we fall through into the retry-decision
			// block below by leaving out non-nil and stage-Error
			// populated. The post-hook itself sets PlanStatusVerifyFailed
			// on the verify path, so we don't double-persist here.
			if strings.TrimSpace(out.Error) == "" {
				out.Error = friendlyDispatchErr(dispatchErr)
			}
		}
		// Soft (StageOutput-level) failures still get the post-hook
		// because StageOutput is meaningful — the agent declared its
		// failure mode in a structured form the post-hook can read.
		runStagePostHook(o, stage, out)
		// StageOutput.Error is the agent's structured failure
		// (e.g. coder's "missing path X", verifier's "verify failed:
		// 3 of 5 tests failed"). Treat it like a SuccessCriteria
		// failure for routing purposes.
		envAfter := buildEnv()
		ok, failed := state.markSuccessCriteriaFailed(n, envAfter)
		stageErrText := ""
		if out != nil {
			stageErrText = out.Error
		}
		logging.Debug("[orchestrator] %s post-dispatch eval: ok=%v stageErrText=%q failed=%d retryUsed=%d budget=%d nType=%s",
			stage, ok, stageErrText, len(failed), state.retryUsed, g.ExecutionPolicy.RetryBudget, n.Type)
		if ok && stageErrText == "" {
			state.markDone(n.ID)
			o.emitNodeEnd(n.ID, true, "")
			continue
		}

		// SC failed (or stage returned an Error). Routing:
		//   - verify SC fail with FailureKind=runner_missing → terminal
		//     (re-running the planner cannot install missing software;
		//     retry just burns LLM tokens on a problem the LLM can't
		//     fix). User-facing error names the tool + install hint.
		//   - verify SC fail with budget remaining → retry cycle
		//   - any other failure → terminal
		if n.Type == types.NodeVerify {
			if shouldSuppressVerifyRetry(o.busCtx.Mutable.ChangeReport()) {
				logging.Warning("[orchestrator] verify failed with FailureKind=runner_missing; suppressing retry — env issue, not a plan defect")
				// Fall through to terminal failure path below.
			} else if reason := verifyStallReason(o.busCtx.Mutable.WriteClosure()); reason != "" {
				// Two adjacent verify rounds produced the EXACT same
				// signal (applied count + pass/fail count + failure
				// summary hash all match). The planner just regenerated
				// what was already tried; further retry is plateau
				// burn. Fall through to terminal so the user sees the
				// best-known-good plan + a clear "stuck on same
				// failure" diagnostic rather than waiting for the
				// retry budget to drain on identical outcomes.
				logging.Warning("[orchestrator] suppressing verify retry — %s", reason)
				// Fall through to terminal failure path below.
			} else if state.retryUsed < g.ExecutionPolicy.RetryBudget {
				retryAttempt++
				clearForReplan(o, retryAttempt)
				targets := state.requeueValidationTargets(n.ID)
				if len(targets) > 0 {
					state.recordRetry()
					// Clear LastError before the retry — applyStageOutput
					// set it from the failed verify's StageOutput.Error
					// (its hot path) and never resets it on subsequent
					// success. Without this clear, a successful retry
					// would still leak the previous failure as LastError
					// to Run()'s caller. The task state Missing is also
					// reset so the next dispatch sees a clean retry.
					o.busCtx.TaskState.LastError = ""
					o.busCtx.TaskState.Missing = types.MissingFacts
					logging.Info("[orchestrator] verify SC failed; requeued %v for retry %d/%d",
						targets, state.retryUsed, g.ExecutionPolicy.RetryBudget)
					o.emit(render.Event{
						Kind:      render.EventAgentReasoning,
						Timestamp: time.Now(),
						Agent:     "orchestrator",
						Reasoning: softRetryHintMessage(o.busCtx.Language),
					})
					o.emitNodeEnd(n.ID, false, "verify failed; retrying")
					continue
				}
				logging.Warning("[orchestrator] verify SC failed but no validation_feedback targets; giving up")
			}
		}
		// Terminal failure path. Before surfacing the failure, restore
		// the best-known-good (plan, report) pair if the current
		// iteration regressed against an earlier one — without this
		// guard the user sees the LAST iteration's worse plan instead
		// of the highest-scoring one the retry loop ever produced.
		// No-op on the happy path (current is best or no retry latched).
		if n.Type == types.NodeVerify {
			restoreBestIfRegressed(o)
		}
		errSummary := stageErrText
		if errSummary == "" && len(failed) > 0 {
			errSummary = failed[0].Detail
		}
		if errSummary == "" {
			errSummary = string(n.Type) + " stage failed"
		}
		if o.busCtx.TaskState.LastError == "" {
			o.busCtx.TaskState.LastError = errSummary
		}
		state.markFailed(n.ID)
		o.emitNodeEnd(n.ID, false, errSummary)
		break
	}
	return stepsUsed
}

// readyWriteWindow returns ready write-mode nodes. Mirrors
// readyExplorerWindow's contract but tolerates write node types and
// honors the OneShot flag — a OneShot node already done doesn't get
// re-yielded even if its EntryConditions still pass.
//
// Counterfactual nodes are skipped (write graphs don't carry them
// today, but the shape stays consistent). Hard-dependency edges are
// honored: a node is only ready if every From in its incoming
// EdgeHardDependency edges is nodeDone.
func (s *graphState) readyWriteWindow(env criterion.Env) (ready []*types.TaskNode, blocked []nodeBlock) {
	if s == nil || len(s.graph.Nodes) == 0 {
		return nil, nil
	}
	for i := range s.graph.Nodes {
		n := &s.graph.Nodes[i]
		if n.IsCounterfactual {
			continue
		}
		st := s.status[n.ID]
		// OneShot nodes already done: never re-yield.
		if n.OneShot && st == nodeDone {
			continue
		}
		if st != nodePending && st != nodeRequeued {
			continue
		}
		// Hard-dependency check.
		depsOK := true
		for _, e := range s.graph.Edges {
			if e.EdgeType != types.EdgeHardDependency {
				continue
			}
			if e.To != n.ID {
				continue
			}
			if s.status[e.From] != nodeDone {
				depsOK = false
				break
			}
		}
		if !depsOK {
			continue
		}
		if len(n.EntryConditions) > 0 {
			ok, failed := criterion.EvalAll(n.EntryConditions, env)
			if !ok {
				blocked = append(blocked, nodeBlock{NodeID: n.ID, FailedCriteria: failed})
				continue
			}
		}
		ready = append(ready, n)
	}
	return ready, blocked
}


// computeStallSignature derives a stable summary of what the just-
// failed dispatch attempt did, used by transientStallPlateau to
// decide whether the next transient retry would be productive.
//
// Empty signature ("") encodes "the attempt made meaningful progress"
// — either it produced a StageOutput with a terminal emit (out != nil
// AND out.Error == ""), or it landed structured artefacts on Mutable
// (a ChangePlan for plan stage, a ChangeReport for verify, etc.).
// In those cases the next retry sees a different starting state, so
// plateau detection should not fire.
//
// Non-empty signature is the "|"-joined ordered list of tool names
// the agent called this dispatch. When two consecutive transient
// failures produce identical signatures, the LLM is hitting the same
// wall and further retry burns wall-time without recovery.
func computeStallSignature(toolResults []types.ToolResult, out *agent.StageOutput, stage types.PipelineStage) string {
	// Per-stage progress signal. Any of these means "we got further
	// than last time" — empty signature short-circuits the plateau
	// detector and lets the next retry through.
	if out != nil && out.Error == "" {
		return ""
	}
	if len(toolResults) == 0 {
		// No tools called at all — e.g. LLM stream stalled before its
		// first tool_use block. Record an explicit marker so the
		// plateau detector can compare two consecutive "stalled
		// before any tool" failures.
		return "<no-tools>"
	}
	names := make([]string, 0, len(toolResults))
	for _, r := range toolResults {
		names = append(names, r.ToolName)
	}
	return strings.Join(names, "|")
}

// stallPlateauMessage builds the user-facing terminal message when
// transient stall plateau fires. Names the stage, the underlying
// transient cause, and the most likely remediation depending on
// the pipeline mode and repo state.
func stallPlateauMessage(busCtx *types.BusContext, stage types.PipelineStage, transientReason string) string {
	if busCtx == nil {
		return fmt.Sprintf("%s repeatedly stalled (%s); aborting", stage, transientReason)
	}
	mode := busCtx.Mode
	emptyRepo := false
	if busCtx.EnvFacts != nil {
		switch busCtx.EnvFacts.GitRepoState {
		case "not_initialized", "no_commits":
			emptyRepo = true
		}
	}
	zh := strings.HasPrefix(strings.ToLower(busCtx.Language), "zh") || busCtx.Language == ""
	if zh {
		base := fmt.Sprintf("%s 阶段连续多次以同一形态失败(%s)，已中止重试", stage, transientReason)
		switch {
		case mode == types.ModePlan && emptyRepo, mode == types.ModeApply && emptyRepo:
			return base + "。当前目录是空仓库;创建型请求建议先用 `codrax --auto-init-repo` 让 codrax 初始化空仓,或切到 `/mode read` 让 LLM 直接给代码而不走 plan/apply 通道。"
		case mode == types.ModePlan, mode == types.ModeApply, mode == types.ModeVerify:
			return base + "。LLM 似乎卡在同一面墙,连续两次都未能产出 emit_change_plan / emit_test_results。先检查 codrax.yaml 模型路由,或换更强的模型再试。"
		default:
			return base + "。两次连续失败的工具调用形态完全一致,继续重试无意义。"
		}
	}
	base := fmt.Sprintf("%s repeatedly stalled with the same shape (%s); aborting retry", stage, transientReason)
	switch {
	case mode == types.ModePlan && emptyRepo, mode == types.ModeApply && emptyRepo:
		return base + ". The target directory is an empty repo; for from-scratch creation use `codrax --auto-init-repo` first or switch to `/mode read` so the LLM emits code directly without the plan/apply path."
	case mode == types.ModePlan, mode == types.ModeApply, mode == types.ModeVerify:
		return base + ". The LLM appears stuck on the same wall — neither attempt produced a terminal emit. Check codrax.yaml model routing or try a stronger model."
	default:
		return base + ". Two consecutive transient failures share an identical tool-call signature; further retry would not recover."
	}
}

// friendlyDispatchErr translates typed transport/streaming errors
// into a user-readable single-sentence surface for LastError /
// out.Error / log lines. Non-typed errors pass through verbatim.
//
// Why this exists: *llm.StreamStalledError and
// *llm.StreamFirstByteTimeoutError unwrap to context.Canceled (the
// watchdog cancels the request to abort the stream). When Err()
// re-emits the wrapped form ("upstream LLM stream stalled (no bytes
// for 1m1s): read stream: context canceled") it carries a confusing
// "context canceled" tail the user reads as "I cancelled this" —
// they didn't, the watchdog did. This helper strips the inner
// context.Canceled chain so the user-visible surface names the
// actual upstream condition.
func friendlyDispatchErr(err error) string {
	if err == nil {
		return ""
	}
	var stall *llm.StreamStalledError
	if errors.As(err, &stall) && stall != nil {
		return fmt.Sprintf("upstream LLM stream stalled (no bytes for %s)", stall.IdleFor)
	}
	var firstByte *llm.StreamFirstByteTimeoutError
	if errors.As(err, &firstByte) && firstByte != nil {
		return fmt.Sprintf("upstream LLM produced no SSE bytes within %s of the request being accepted", firstByte.IdleFor)
	}
	return err.Error()
}

// shouldSuppressVerifyRetry reports whether a verify failure was
// caused by a missing test runner binary (FailureKindRunnerMissing).
// In that case the verify→plan retry loop must short-circuit: the
// LLM cannot install software and the planner would just regenerate
// equivalent test code that hits the same missing-tool wall. The
// caller falls through to the terminal failure path so the user
// sees the install hint without burning retry budget.
//
// Defensive against nil report (apply or worktree errors that never
// reached the runner) — returns false so the existing budget check
// still applies.
func shouldSuppressVerifyRetry(report *types.ChangeReport) bool {
	if report == nil {
		return false
	}
	return report.FailureKind == types.FailureKindRunnerMissing
}

// verifyStallReason inspects the last two verify-round fingerprints
// and returns a non-empty reason string when consecutive rounds
// produced identical signal — applied count, verify pass/fail
// counts, AND failure summary hash all match. That's a stall:
// retry would just regenerate the same outcome, so further LLM
// budget is wasted on a problem the planner is structurally unable
// to fix from the signal it has.
//
// Returns empty when:
//   - fewer than 2 fingerprints recorded (can't compare adjacency)
//   - the two latest differ in any field
//   - WriteClosure / busCtx is nil (defensive)
//
// Generic by design: the predicate compares hashes, not specific
// failure text — works the same for tests-failed plateau, build-
// failure plateau, OOM plateau, etc. New retry kinds get coverage
// for free.
func verifyStallReason(closure *types.WriteClosure) string {
	if closure == nil {
		return ""
	}
	hist := closure.Fingerprints()
	if len(hist) < 2 {
		return ""
	}
	cur := hist[len(hist)-1]
	prev := hist[len(hist)-2]
	if !cur.SameSignal(prev) {
		return ""
	}
	return fmt.Sprintf("two consecutive verify rounds produced identical signal (applied=%d passed=%d failed=%d failureHash=%s) — planner stuck on the same outcome",
		cur.AppliedCount, cur.VerifyPassed, cur.VerifyFailed, cur.FailureSummaryHash)
}
