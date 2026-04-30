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
	"github.com/hanchaoqun/codrax/internal/worktree"
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
			// Transient-dispatch retry. Restricted to stream-level
			// errors (EOF / stream stalled / first-byte timeout /
			// network blip) because HTTP 429 / 5xx are L1's domain
			// — by the time we see one here, L1 already exhausted
			// its 6-attempt × 62-second budget. Retrying at the
			// scheduler layer on a persistent 429 just multiplies
			// wait time for no recovery benefit. Stall plateau
			// detection still short-circuits two-consecutive-
			// identical-signature stalls.
			if llm.IsStreamLevelRetryable(dispatchErr) {
				sig := computeStallSignature(o.busCtx.Mutable.DispatchToolResults(), out, stage)
				// Maintain the consecutive-no-emit streak alongside
				// the per-node signature memo. A non-empty sig means
				// the agent did not reach a terminal emit in this
				// dispatch (computeStallSignature returns "" only on
				// successful emit). The streak is the signature-
				// AGNOSTIC plateau signal that catches the LLM
				// changing tool tactics between rounds while still
				// failing to emit — the trace pattern from
				// /home/chatpp/pytest 2026-04-29 06:03.
				if sig == "" {
					state.resetTransientNoEmitStreak(n.ID)
				} else {
					state.recordTransientNoEmitStall(n.ID)
				}
				// Plateau via two complementary signals:
				//   1. Identical tool-call signature twice in a row
				//      (LLM repeated the EXACT same dead path).
				//   2. Two consecutive no-emit stalls regardless of
				//      tool sequence (LLM rotated tactics but never
				//      reached a terminal emit).
				if state.transientStallPlateau(n.ID, sig) || state.transientNoEmitPlateau(n.ID) {
					friendly := stallPlateauMessage(o.busCtx, stage, friendlyDispatchErr(dispatchErr), o.autoInitRepo, o.scaffoldEnabled)
					logging.Warning("[orchestrator] %s transient stall plateau (sig=%q sigPlateau=%v noEmitPlateau=%v) — suppressing further retry",
						stage, sig,
						state.transientStallPlateau(n.ID, sig),
						state.transientNoEmitPlateau(n.ID))
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
					// Plan-stage stall recovery: when the planner stalled
					// before reaching a terminal emit (sig != "") AND we
					// are about to retry, seed PlanningHint so the next
					// dispatch's BuildInitialInstruction prepends a
					// directive to use the streaming-safe multi-round
					// emission path (emit_plan_skeleton + per-file
					// emit_plan_change). The skeleton path's payload is
					// small enough that mid-stream truncation cannot hit
					// the watchdog; per-file emits stay under any single
					// LLM response budget. Existing PlanningHint set by a
					// previous SC-retry's clearForReplan is preserved by
					// prepending — we add to the front so the stall hint
					// reads first while the prior failure context still
					// reaches the LLM.
					if stage == types.StagePlan && sig != "" && o.busCtx != nil && o.busCtx.Mutable != nil {
						existing := o.busCtx.Mutable.PlanningHint()
						stallHint := plannerStallRecoveryHint()
						if existing != "" {
							stallHint = stallHint + "\n\n" + existing
						}
						o.busCtx.Mutable.SetPlanningHint(stallHint)
					}
					logging.Warning("[orchestrator] %s transient dispatch error; requeued for retry %d/%d (transient budget): %v",
						stage, state.transientRetryUsed, o.transientRetryBudget, reason)
					o.emit(render.Event{
						Kind:      render.EventAgentReasoning,
						Timestamp: time.Now(),
						Agent:     "orchestrator",
						Reasoning: softRetryHintForStage(o.busCtx.Language, stage),
					})
					// IMPORTANT: do NOT emit EventTaskNodeEnd here.
					// The node is going to be re-dispatched, so the
					// renderer must keep treating it as "running" — an
					// emit at this point would set row.endTime and the
					// stage label would flip to the "done" phrase
					// ("已设计改动方案") even though no plan landed yet.
					// The next EventTaskNodeStart on the retry will
					// re-establish the running state cleanly. This
					// mirrors the read scheduler's verify→plan retry
					// path (orchestrator.go:2331).
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
				// Wipe transient-stall bookkeeping so the new plan
				// generation cycle starts with a clean slate. Without
				// this, a stall observed in iteration N (transient
				// retry path) would still count toward iteration N+1's
				// plateau detector — leaking a memory across two
				// distinct retry concepts (one is "blip in the LLM
				// connection", the other is "the previous plan was
				// wrong"). The two clears below cover both bookkeeping
				// maps for every node in the graph; cheap (a few map
				// deletes) and the only safe thing to do.
				for _, gn := range g.Nodes {
					state.resetTransientNoEmitStreak(gn.ID)
					if state.transientStallSignatures != nil {
						delete(state.transientStallSignatures, gn.ID)
					}
				}
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
						Reasoning: softRetryHintForStage(o.busCtx.Language, stage),
					})
					// Same retry-render contract as the transient-retry
					// branch above: do NOT emit EventTaskNodeEnd because
					// the node will be re-dispatched (along with its
					// upstream plan node via EdgeValidationFeedback).
					// Emitting end here would flip the stage label to
					// the "done" phrase even though verify is going to
					// run again.
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
// the pipeline mode, repo state, and which authorization tiers the
// operator has already granted.
//
// On the empty-repo path the remediation hint is auth-tier specific
// (the planPreHook gate already refused un-authorized cases, so by
// the time we reach plateau the auth tiers tell us WHICH layer to
// blame): if scaffolding is missing, point at --allow-scaffold; if
// init is missing point at --auto-init-repo; if both are present
// the stall is a model-routing issue, not an authorization issue.
func stallPlateauMessage(busCtx *types.BusContext, stage types.PipelineStage, transientReason string, autoInitRepo, scaffoldEnabled bool) string {
	if busCtx == nil {
		return fmt.Sprintf("%s repeatedly stalled (%s); aborting", stage, transientReason)
	}
	mode := busCtx.Mode
	// Ground-truth probe: re-detect git state at message-render time
	// rather than trusting the cached EnvFacts snapshot. EnvFacts is
	// captured ONCE at orchestrator.Run() entry; if applyPreHook
	// later ran `git init` to satisfy autoInitRepo authorization, the
	// cached "not_initialized" is stale — surfacing "目录是空仓"
	// advice for a dir that IS now a real repo. The DetectRepoState
	// call is read-only (a couple of stat / git rev-parse invocations)
	// and only fires on the terminal-failure rendering path, so its
	// cost is negligible. Falls back to the cached EnvFacts only if
	// the probe itself errored (e.g. repo path moved out from under us).
	emptyRepo := false
	if busCtx.MainRepoRoot != "" {
		if state, err := worktree.DetectRepoState(busCtx.MainRepoRoot); err == nil {
			emptyRepo = state.NeedsInit()
		} else if busCtx.EnvFacts != nil {
			switch busCtx.EnvFacts.GitRepoState {
			case "not_initialized", "no_commits":
				emptyRepo = true
			}
		}
	} else if busCtx.EnvFacts != nil {
		switch busCtx.EnvFacts.GitRepoState {
		case "not_initialized", "no_commits":
			emptyRepo = true
		}
	}
	zh := strings.HasPrefix(strings.ToLower(busCtx.Language), "zh") || busCtx.Language == ""
	writeMode := mode == types.ModePlan || mode == types.ModeApply || mode == types.ModeVerify
	if zh {
		stageLabel := writeStageZhLabel(stage)
		base := fmt.Sprintf("%s连续多次没产出可用结果,已中止重试", stageLabel)
		switch {
		case writeMode && emptyRepo && !autoInitRepo:
			return base + "。当前目录还不是 git 仓库;先加 --auto-init-repo 授权初始化,再决定是否需要 --allow-scaffold。"
		case writeMode && emptyRepo && !scaffoldEnabled:
			return base + "。目录是空的,模型没有源代码可以参考;从零创建新项目需要加 --allow-scaffold (或在配置里设 write_scaffold_enabled: true)。"
		case writeMode && emptyRepo:
			return base + "。空目录已开启 scaffold,但模型仍然给不出可用的方案。在配置文件里换更强的模型再试。"
		case writeMode:
			return base + "。模型重复给不出可用的方案。在配置文件里换更强的模型再试。"
		default:
			return base + "。两次结果完全相同,继续重试也不会有变化。"
		}
	}
	stageLabel := writeStageEnLabel(stage)
	base := fmt.Sprintf("%s repeatedly produced no usable result; retry aborted", stageLabel)
	switch {
	case writeMode && emptyRepo && !autoInitRepo:
		return base + ". The target directory is not yet a git repo; first authorize initialization via --auto-init-repo, then decide whether --allow-scaffold is also needed."
	case writeMode && emptyRepo && !scaffoldEnabled:
		return base + ". The directory is empty so the model has no existing source to read; creating a new project from scratch needs --allow-scaffold (or write_scaffold_enabled: true in the config file)."
	case writeMode && emptyRepo:
		return base + ". The empty directory has scaffold authorized, but the model keeps producing nothing usable. Switch to a stronger model in the config file and retry."
	case writeMode:
		return base + ". The model keeps producing nothing usable. Switch to a stronger model in the config file and retry."
	default:
		return base + ". Two outcomes were identical; continued retry will not change anything."
	}
}

// writeStageZhLabel translates an internal pipeline stage id into a
// user-facing Chinese label (no internal jargon). Falls back to the
// raw stage string when the mapping is missing — better to surface a
// short tag than crash the message.
func writeStageZhLabel(stage types.PipelineStage) string {
	switch stage {
	case types.StagePlan:
		return "生成改动方案"
	case types.StageApply:
		return "应用改动"
	case types.StageVerify:
		return "运行验证测试"
	}
	return string(stage)
}

func writeStageEnLabel(stage types.PipelineStage) string {
	switch stage {
	case types.StagePlan:
		return "Drafting the change plan"
	case types.StageApply:
		return "Applying changes"
	case types.StageVerify:
		return "Running verification tests"
	}
	return string(stage)
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

// plannerStallRecoveryHint returns the LLM-facing directive the
// orchestrator prepends to PlanningHint when a plan-stage retry
// fires after a streaming-truncation stall. The hint nudges the
// planner toward the multi-round emission path: emit_plan_skeleton
// (small payload, structurally cannot truncate) followed by
// per-file emit_plan_change calls (each bounded). The skill
// prompt already describes both modes; on first-attempt the LLM
// picks single-shot by default. After a stall we know single-shot
// didn't fit, so we promote the multi-round path from "available"
// to "required for this retry".
//
// Always English — this string is consumed by the LLM as a
// system-level directive, not displayed to the user. Localising
// LLM hints would risk degrading the model's instruction-following
// for non-English locales while adding a translation maintenance
// burden. User-facing prose surfaces (Result, error messages) ARE
// localised; the LLM-facing PlanningHint is not.
func plannerStallRecoveryHint() string {
	return "RETRY DIRECTIVE — your previous response was cut off mid-stream because a single emit_change_plan exceeded what the streaming response can carry.\n" +
		"\n" +
		"On this retry use the multi-round path:\n" +
		"  1. Call emit_plan_skeleton ONCE: request, summary, changes[] metadata only (path + kind + rationale; no new_content, no patch).\n" +
		"  2. Call emit_plan_change once per file with kind ∈ {create, modify, patch}, with that single file's body. Skip kind=delete.\n" +
		"\n" +
		"Do NOT call emit_change_plan again on this retry — same wall. The previous response was discarded; start the skeleton fresh."
}

// plannerScaffoldHint is the proactive PlanningHint seeded by
// planPreHook when the target is a bare directory the operator
// has authorized to scaffold (autoInitRepo=true). The planner is
// about to assemble a brand-new project's plan with no existing
// source to read — the canonical streaming-truncation trigger.
// Pre-arming the multi-round directive avoids the wait-for-first-
// stall recovery cycle.
//
// Always English — same rationale as plannerStallRecoveryHint.
// Distinct lead phrase ("SCAFFOLD DIRECTIVE") so an operator
// inspecting the LLM's prompt context (debug log) can tell which
// branch armed the hint.
func plannerScaffoldHint() string {
	return "SCAFFOLD DIRECTIVE — the target directory is empty; this run scaffolds a project from scratch. A single emit_change_plan with every file's body inline is likely too large to stream.\n" +
		"\n" +
		"Use the multi-round path:\n" +
		"  1. Call emit_plan_skeleton ONCE with every file's metadata (path + kind + rationale; no new_content, no patch).\n" +
		"  2. Call emit_plan_change once per non-delete file to send its body.\n" +
		"\n" +
		"Begin the changes[] with the language manifest the user's request implies (go.mod for Go, package.json for Node, pyproject.toml for Python, Cargo.toml for Rust, etc.). For files that import other files in this same plan, set depends_on so the manifest and any imported files appear earlier in apply order."
}
