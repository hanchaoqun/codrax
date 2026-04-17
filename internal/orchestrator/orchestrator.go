package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/analysis/stopcond"
	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Orchestrator is the Layer 1 component that drives the pipeline state
// machine. It walks the hardcoded 4-stage topology (see topology.go),
// manages BusContext, and dispatches agents.
type Orchestrator struct {
	settings       types.PipelineSettings
	agents         *agent.Registry
	skills         *skill.Registry
	busCtx         *types.BusContext
	maxSteps       int
	subRuntime     *agent.SubAgentRuntime
	language       string
	emit           render.EventEmitter
	thinkAloudMap  map[types.AgentName]bool // per-agent think-aloud override
}

// New creates a new Orchestrator.
func New(settings types.PipelineSettings, agents *agent.Registry, skills *skill.Registry, subAgents *agent.SubAgentRegistry) *Orchestrator {
	return &Orchestrator{
		settings:   settings,
		agents:     agents,
		skills:     skills,
		maxSteps:   50,
		subRuntime: agent.NewSubAgentRuntime(subAgents),
		emit:       render.NopEmitter,
	}
}

// SetEmitter attaches an event emitter for real-time CLI rendering.
// Must be called before Run(). Passing nil restores the no-op default.
func (o *Orchestrator) SetEmitter(emit render.EventEmitter) {
	if emit == nil {
		emit = render.NopEmitter
	}
	o.emit = emit
}

// SetMaxSteps overrides the maximum number of pipeline steps (default 50).
func (o *Orchestrator) SetMaxSteps(n int) {
	o.maxSteps = n
}

// SetLanguage configures the default response language injected into
// every agent's system prompt via BusContext.Preferences. The empty
// string, "off", and "none" disable the injection so the pipeline
// behaves exactly as before. Any other value is passed through to
// languageDirective which maps well-known codes to explicit wording.
func (o *Orchestrator) SetLanguage(lang string) {
	o.language = lang
}

// SetThinkAloudMap installs the per-agent think-aloud overrides
// resolved from providers.yaml. Keys are agent names; values are
// the resolved boolean. Agents not in the map inherit the default.
func (o *Orchestrator) SetThinkAloudMap(m map[types.AgentName]bool) {
	o.thinkAloudMap = m
}

// Run executes the full pipeline for a user request.
//
// The pipeline runs in two phases:
//
//   - Phase 1 — analyze: dispatch StageAnalyze once. The analyzer
//     emits an AnalysisIR via emit_analysis and the post-processing
//     pipeline deterministically builds TaskGraph / EvidencePlan /
//     AnswerContract / HypothesisSet from it.
//
//   - Phase 2 — per-task: iterate over pending tasks (typically one),
//     running a mini-pipeline (explore → extract → finalize) for each
//     via runTaskGraph. Per-task state (Signals, MissingPiece,
//     PipelineStage, oscillation counter) resets between tasks;
//     shared state accumulates.
//
// The maxSteps budget is enforced globally across both phases.
func (o *Orchestrator) Run(request string, repoRoot string, branch string) (*types.BusContext, error) {
	// Initialize BusContext
	o.busCtx = &types.BusContext{
		PipelineStage: types.StageAnalyze,
		RepoRoot:      repoRoot,
		Branch:        branch,
		TraceID:       fmt.Sprintf("trace-%d", time.Now().UnixNano()),
		Mutable: types.NewMutableState(request),
		TaskState: types.TaskState{
			Stage:   types.StageAnalyze,
			Missing: types.MissingUnderstanding,
		},
	}

	o.busCtx.Language = o.language

	logging.Info("[orchestrator] starting pipeline: trace=%s", o.busCtx.TraceID)

	o.emit(render.Event{
		Kind:      render.EventPipelineStart,
		Timestamp: time.Now(),
		TraceID:   o.busCtx.TraceID,
	})

	// Per-trace working directory for tool blob storage. Tools that
	// produce large outputs offload to this dir and return a path in
	// ToolResult.RawRef so the LLM can re-read slices on demand instead
	// of carrying full content through the message history. Cleanup is
	// best-effort; failures are logged but do not abort the pipeline.
	if workDir, err := os.MkdirTemp("", "codrax-"+o.busCtx.TraceID+"-"); err != nil {
		logging.Warning("[orchestrator] could not create work dir: %v (blob storage disabled)", err)
	} else {
		o.busCtx.WorkDir = workDir
		logging.Info("[orchestrator] work dir: %s", workDir)
		defer func() {
			if rmErr := os.RemoveAll(workDir); rmErr != nil {
				logging.Warning("[orchestrator] work dir cleanup failed: %v", rmErr)
			}
		}()
	}

	stepsUsed := 0

	// Phase 1: analyze. Fail-loud: when analyze exhausts its retry
	// budget the whole Run terminates without entering phase 2.
	// On success, emit EventAnalysisReady so the renderer can switch
	// from stage-dispatch rows to the analyzer's actual task / sub-
	// task breakdown.
	if used, err := o.runAnalyzePhase(); err != nil {
		logging.Error("[orchestrator] analyze phase failed: %v", err)
		o.busCtx.TaskState.LastError = fmt.Sprintf("analyze: %v", err)
		o.busCtx.TaskState.IsTerminal = true
		o.busCtx.Mutable.SetResult("")
		o.emit(render.Event{
			Kind:      render.EventPipelineEnd,
			Timestamp: time.Now(),
			TraceID:   o.busCtx.TraceID,
			Error:     o.busCtx.TaskState.LastError,
		})
		return o.busCtx, nil
	} else {
		stepsUsed += used
		o.emitAnalysisReady()
	}

	// Phase 2: per-task execution.
	if err := o.runTaskPhase(&stepsUsed); err != nil {
		logging.Error("[orchestrator] task phase error: %v", err)
		if o.busCtx.TaskState.LastError == "" {
			o.busCtx.TaskState.LastError = err.Error()
		}
	}

	o.busCtx.TaskState.IsTerminal = true

	errMsg := ""
	if o.busCtx.TaskState.LastError != "" {
		errMsg = o.busCtx.TaskState.LastError
	}
	o.emit(render.Event{
		Kind:          render.EventPipelineEnd,
		Timestamp:     time.Now(),
		TraceID:       o.busCtx.TraceID,
		ToolCallCount: len(o.busCtx.ToolResults),
		MCPCallCount:  len(o.busCtx.MCPResponses),
		FactCount:     len(o.busCtx.RepoFacts),
		Error:         errMsg,
	})

	return o.busCtx, nil
}

// runAnalyzePhase dispatches the analyze stage with hard fail-loud
// retry semantics. Each attempt is counted; the loop exits early
// on a clean StageOutput (no Error, non-nil AnalysisIR). After the
// retry budget is exhausted the phase returns an error so Run
// terminates without entering the per-task phase.
func (o *Orchestrator) runAnalyzePhase() (int, error) {
	o.busCtx.PipelineStage = types.StageAnalyze
	o.busCtx.TaskState.Stage = types.StageAnalyze
	o.busCtx.TaskState.Missing = types.MissingUnderstanding

	max := o.settings.MaxRetriesPerStage
	if max < 1 {
		max = 1
	}
	var lastErr string
	used := 0
	for attempt := 0; attempt < max; attempt++ {
		used++
		out, err := o.dispatchStage(types.StageAnalyze)
		if err == nil && (out == nil || out.Error == "") && o.busCtx.AnalysisIR != nil {
			return used, nil
		}
		if out != nil {
			lastErr = out.Error
		}
		if err != nil {
			lastErr = err.Error()
		}
		logging.Warning("[orchestrator] analyze attempt %d/%d failed: %s", attempt+1, max, lastErr)
	}
	return used, fmt.Errorf("analyze stage exhausted after %d attempt(s): %s", max, lastErr)
}

// runTaskPhase dispatches the single task graph for the run. After
// the v3 analyzer simplification every request maps to exactly one
// task, so this is a direct call into runTaskGraph — no loop, no
// pending-queue bookkeeping. The budget check still runs so a
// pathologically expensive analyze phase cannot silently starve the
// per-task path.
func (o *Orchestrator) runTaskPhase(stepsUsed *int) error {
	if *stepsUsed >= o.maxSteps {
		logging.Error("[orchestrator] global max-steps (%d) exhausted before task phase", o.maxSteps)
		o.busCtx.Mutable.SetResult("")
		return nil
	}

	objective := o.busCtx.Mutable.Objective()
	o.emit(render.Event{
		Kind:      render.EventObjectiveStarted,
		Timestamp: time.Now(),
		Objective: objective,
	})

	used := o.runTaskGraph(o.maxSteps - *stepsUsed)
	*stepsUsed += used
	return nil
}

// runTaskGraph walks AnalysisIR.TaskGraph with criterion-aware
// scheduling. Each round:
//
//  1. Build a criterion.Env from current BusContext state.
//  2. Check stopcond.ShouldStop; if true, forceCloseExploreWindow
//     and jump directly to finalize.
//  3. readyExplorerWindow returns nodes whose hard deps are done
//     AND whose EntryConditions all pass. Dispatch them as one
//     explore window. After the explore dispatch, evaluate each
//     window node's SuccessCriteria: successful ones are marked
//     done; failed ones are requeued.
//  4. A failed validate node's SuccessCriteria triggers
//     requeueValidationTargets — only the specific upstream
//     evidence nodes named by EdgeValidationFeedback get requeued,
//     not the whole window.
//  5. Finalize dispatch + contract check on the same contract-
//     checker retry semantics as before.
func (o *Orchestrator) runTaskGraph(stepBudget int) int {
	ir := o.busCtx.AnalysisIR
	if ir == nil || len(ir.TaskGraph.Nodes) == 0 {
		// Defensive: analyzer should always produce a non-empty
		// TaskGraph, but if something upstream failed we cannot
		// execute the task.
		logging.Error("[orchestrator] task: no AnalysisIR.TaskGraph — analyzer failed to produce a valid IR")
		o.busCtx.Mutable.SetResult("")
		o.busCtx.TaskState.LastError = "analyzer failed to produce TaskGraph"
		return 0
	}

	// Per-task state reset so a multi-task run does not drag signals
	// across the task boundary.
	o.busCtx.Signals = types.ExecutionSignals{}
	o.busCtx.TaskState.Missing = types.MissingFacts

	// Cross-task reset of the Turn A/B handoff surface. Multi-task runs (REPL turns, batched analysis, task
	// list with >1 entry) otherwise drag stale state from task N
	// into task N+1: the previous task's TurnAArtifacts would still
	// be visible to this task's extractor, the previous task's
	// answer-symbol slate would still be drained into this task's
	// StageOutput, and the previous task's hypothesis verdicts would
	// still populate the finalizer prompt. Each Reset is a no-op
	// when the buffer is already empty, so it is safe to call
	// unconditionally at the top of every per-task dispatch.
	if o.busCtx.Mutable != nil {
		o.busCtx.Mutable.ResetTurnAArtifacts()
		o.busCtx.Mutable.ResetEmittedAnswerSymbols()
		o.busCtx.Mutable.ResetEmittedHypothesisVerdicts()
		o.busCtx.Mutable.ResetEmittedEvidence()
		// AnswerDocument is the finalizer's structured output buffer;
		// reset it alongside the extractor buffers so a multi-task run
		// cannot drag a stale document from task N into task N+1.
		o.busCtx.Mutable.ResetAnswerDocument()
	}
	// AnswerSymbolCompleteness is a BusContext field, not a
	// MutableState field — reset it here too so the applyStageOutput
	// "last non-empty writer wins" merge rule does not accidentally
	// keep the previous task's claim alive when the current task's
	// extractor emits CompletenessUnknown.
	o.busCtx.AnswerSymbolCompleteness = types.CompletenessUnknown

	state := newGraphState(ir.TaskGraph)
	resolveSurface := termSurfaceLookup(ir)

	// Install the ExploreBudget derived from the analyzer's
	// NodeBudgetHints. Explorer's ReAct loop reads this through
	// ctx.Mutable.ExploreBudget() to throttle per-tool calls.
	hints := ir.EvidencePlan.NodeBudgetHints
	o.busCtx.Mutable.SetExploreBudget(&types.ExploreBudget{
		PerToolCap:  hints.PerToolCap,
		PerToolUsed: map[string]int{},
		OverallCap:  hints.OverallCap,
	})

	var pendingViolation string
	var pendingValidationTargets []string

	if b := ir.EvidencePlan.Budget.MaxReactIters; b > 0 && b < stepBudget {
		stepBudget = b
	}

	// Adaptive budget scaling for multi-topic questions. When the
	// analyzer detected >1 SubTopics, the pipeline needs more steps
	// to investigate each sub-topic thoroughly.
	if nSub := len(ir.RequestModel.SubTopics); nSub > 1 {
		agentCfg := o.settings.Agent
		extraSteps := nSub * agentCfg.SubTopicPipelineStepsExtra
		adjusted := stepBudget + extraSteps
		if adjusted > 100 {
			adjusted = 100
		}
		if adjusted > stepBudget {
			logging.Info("[orchestrator] multi-topic scaling: %d sub-topics, step budget %d → %d",
				nSub, stepBudget, adjusted)
			stepBudget = adjusted
		}
	}

	stepsUsed := 0
	var lastFinalize *agent.StageOutput

	buildEnv := func(draft string, draftCitations int) criterion.Env {
		return criterion.Env{
			IR:             ir,
			Evidence:       o.busCtx.EvidenceItems,
			AnswerSymbols:  o.busCtx.AnswerSymbols,
			AnswerChains:   o.busCtx.AnswerChains,
			ToolResults:    o.busCtx.ToolResults,
			PrescanBlob:    o.busCtx.Mutable.PrescanSummaryBlob(),
			Signals:        o.busCtx.Signals,
			DraftAnswer:    draft,
			DraftCitations: draftCitations,
			ReactItersUsed: stepsUsed,
		}
	}

	for stepsUsed < stepBudget && !state.allDone() {
		env := buildEnv("", 0)

		if stop, reason := stopcond.ShouldStop(ir.EvidencePlan, env); stop {
			logging.Info("[orchestrator] stop condition fired: %s", reason)
			state.forceCloseExploreWindow()
			continue
		}

		window, blocked := state.readyExplorerWindow(env)
		fin := state.firstFinalizeReadyMerged()

		if len(window) > 0 {
			hint := renderWindowHint(window, blocked, pendingValidationTargets, resolveSurface, pendingViolation)
			pendingViolation = ""
			pendingValidationTargets = nil
			o.applyWindowHint(hint)
			for _, n := range window {
				state.markRunning(n.ID)
				o.emitNodeStart(n.ID)
			}

			o.busCtx.PipelineStage = types.StageExplore
			o.busCtx.TaskState.Stage = types.StageExplore
			// Reset per-tool usage counters and investigation-complete
			// flag so a retry window (validation_feedback requeue or
			// contract backtrack) starts fresh.
			o.busCtx.Mutable.ResetInvestigationComplete()
			if eb := o.busCtx.Mutable.ExploreBudget(); eb != nil {
				o.busCtx.Mutable.SetExploreBudget(&types.ExploreBudget{
					PerToolCap:  eb.PerToolCap,
					PerToolUsed: map[string]int{},
					OverallCap:  eb.OverallCap,
				})
			}
			stepsUsed++
			if _, err := o.dispatchStage(types.StageExplore); err != nil {
				logging.Error("[orchestrator] DAG explore window failed: %v", err)
				for _, n := range window {
					state.markFailed(n.ID)
					o.emitNodeEnd(n.ID, false, err.Error())
				}
			} else {
				// Post-dispatch criterion evaluation. Separate
				// validate-node failure from non-validate failure:
				// validate failures trigger fine-grained
				// requeueValidationTargets, others just mark the
				// node requeued.
				icComplete := o.busCtx.Mutable != nil && o.busCtx.Mutable.IsInvestigationComplete()
				icPolicy := o.settings.Agent.InvestigationCompletePolicy

				// "override" policy: when the LLM called
				// emit_investigation_complete, skip all criteria and
				// mark every explore-type node done immediately. The
				// AnswerContract checker at finalize is the sole quality
				// gate in this mode.
				if icComplete && icPolicy == types.ICPolicyOverride {
					for _, n := range window {
						state.markDone(n.ID)
						o.emitNodeEnd(n.ID, true, "")
					}
					o.emit(render.Event{
						Kind:      render.EventAgentReasoning,
						Timestamp: time.Now(),
						Agent:     "orchestrator",
						Reasoning: "investigation_complete override: all explore nodes marked done (policy=override).",
					})
					o.runAutoVerdicts()
					o.drainHypothesisVerdicts()
					continue
				}

				envAfter := buildEnv("", 0)
				// "soft" policy: inject the completion signal into the
				// criterion env so evidence_count lowers to >=1.
				if icComplete && icPolicy == types.ICPolicySoft {
					envAfter.InvestigationComplete = true
				}
				var valFailed *types.TaskNode
				for _, n := range window {
					ok, failed := state.markSuccessCriteriaFailed(n, envAfter)
					if ok {
						state.markDone(n.ID)
						o.emitNodeEnd(n.ID, true, "")
						continue
					}
					logging.Info("[orchestrator] node %s success criteria failed: %+v", n.ID, failed)
					// Surface the criterion failure to the user.
					var details []string
					for _, f := range failed {
						details = append(details, fmt.Sprintf("%s %s: %s", f.Kind, f.Expr, f.Detail))
					}
					o.emit(render.Event{
						Kind:      render.EventAgentReasoning,
						Timestamp: time.Now(),
						Agent:     "orchestrator",
						Reasoning: fmt.Sprintf("⟳ Node %s success criteria not met (%s) — requeuing.", n.ID, strings.Join(details, "; ")),
					})
					// No EventTaskNodeEnd on requeue — the renderer treats
					// the node as still "running" until the next
					// EventTaskNodeStart flips it back in.
					if n.Type == types.NodeValidate {
						valFailed = n
					} else {
						state.requeue(n.ID)
					}
				}
				if valFailed != nil {
					targets := state.requeueValidationTargets(valFailed.ID)
					if len(targets) == 0 {
						// No upstream evidence edges found — fall
						// back to requeueing the validate node only.
						state.requeue(valFailed.ID)
					} else {
						pendingValidationTargets = targets
						state.recordRetry()
					}
				}

				// Lightweight auto-verdict after each explore window:
				// evaluate criterion-based hypothesis verdicts without
				// an LLM call. The full extract dispatch (with LLM)
				// runs once just before finalize.
				o.runAutoVerdicts()
				o.drainHypothesisVerdicts()
			}
			continue
		}

		if fin == nil {
			// No ready window (or every node blocked) AND no ready
			// finalize. If blocked nodes exist we can make progress
			// only by waiting for a future env change; since env is
			// pure-read we would loop forever. Break to forced
			// finalize.
			if len(blocked) > 0 {
				logging.Warning("[orchestrator] %d node(s) blocked on entry conditions; forcing finalize", len(blocked))
			} else {
				logging.Warning("[orchestrator] DAG scheduler stalled; forcing finalize")
			}
			break
		}

		// Full Turn B extract dispatch — runs once, just before
		// finalize, with complete accumulated evidence from all
		// explore windows. Answer-symbol selection + LLM hypothesis
		// verdicts happen here.
		o.busCtx.PipelineStage = types.StageExtract
		o.busCtx.TaskState.Stage = types.StageExtract
		stepsUsed++
		if _, exErr := o.dispatchStage(types.StageExtract); exErr != nil {
			logging.Warning("[orchestrator] pre-finalize extract dispatch failed (continuing): %v", exErr)
		} else {
			o.drainHypothesisVerdicts()
		}

		state.markRunning(fin.ID)
		o.emitNodeStart(fin.ID)
		o.busCtx.PipelineStage = types.StageFinalize
		o.busCtx.TaskState.Stage = types.StageFinalize
		stepsUsed++
		out, err := o.dispatchStage(types.StageFinalize)
		if err != nil {
			logging.Error("[orchestrator] DAG finalize failed: %v", err)
			state.markFailed(fin.ID)
			o.emitNodeEnd(fin.ID, false, err.Error())
			break
		}
		lastFinalize = out

		// Evaluate finalize node's SuccessCriteria alongside
		// the AnswerContract check. SuccessCriteria on finalize
		// nodes carry citation / symbol constraints the compiler
		// declared; failing them is treated like a contract
		// violation for backtrack purposes.
		envFin := buildEnv(out.FinalAnswer, len(extractCitationsFromAnswer(out.FinalAnswer)))
		if scOK, scFailed := state.markSuccessCriteriaFailed(fin, envFin); !scOK {
			for _, f := range scFailed {
				logging.Info("[orchestrator] finalize success criteria failed: %s %s — %s", f.Kind, f.Expr, f.Detail)
			}
		}

		// Contract check.
		res := runContractCheck(out, ir.AnswerContract)
		if res.Passed {
			state.markDone(fin.ID)
			o.emitNodeEnd(fin.ID, true, "")
			break
		}

		logging.Info("[orchestrator] contract check failed (%d violation(s)); retryUsed=%d/%d",
			len(res.Violations), state.retryUsed, ir.TaskGraph.ExecutionPolicy.RetryBudget)

		if state.retryBudgetExhausted() {
			// Fail-loud — preserve the original answer beneath an
			// honest warning so the user sees the gap.
			out.FinalAnswer = appendViolationsToAnswer(out.FinalAnswer, res)
			lastFinalize = out
			state.markDone(fin.ID)
			o.emitNodeEnd(fin.ID, true, "")
			break
		}

		// Backtrack: requeue the finalize node and every explorer-
		// window node that sits behind it, so the next round
		// re-runs the merged investigation with the violation
		// diagnostic in front. No EventTaskNodeEnd here — the next
		// scheduler round will fire EventTaskNodeStart for each
		// requeued node, and the renderer treats that as the row's
		// transition back to running.
		state.requeue(fin.ID)
		for _, n := range ir.TaskGraph.Nodes {
			if n.Type == types.NodeFinalize {
				continue
			}
			if state.status[n.ID] == nodeDone {
				state.requeue(n.ID)
			}
		}
		state.recordRetry()
		pendingViolation = renderViolations(res)

		// Surface the backtrack to the user so they know
		// the pipeline is re-investigating, not stalled.
		o.emit(render.Event{
			Kind:      render.EventAgentReasoning,
			Timestamp: time.Now(),
			Agent:     "orchestrator",
			Reasoning: "⟳ Answer contract check failed: " + pendingViolation + " — re-investigating.",
		})
	}

	if lastFinalize == nil {
		// Force one finalize dispatch so the task always terminates
		// with a Result.
		logging.Warning("[orchestrator] DAG run produced no finalize output; forcing finalize")

		// Extract before forced finalize.
		o.busCtx.PipelineStage = types.StageExtract
		o.busCtx.TaskState.Stage = types.StageExtract
		stepsUsed++
		if _, exErr := o.dispatchStage(types.StageExtract); exErr != nil {
			logging.Warning("[orchestrator] pre-forced-finalize extract dispatch failed (continuing): %v", exErr)
		} else {
			o.drainHypothesisVerdicts()
		}

		o.busCtx.PipelineStage = types.StageFinalize
		o.busCtx.TaskState.Stage = types.StageFinalize
		stepsUsed++
		out, err := o.dispatchStage(types.StageFinalize)
		if err != nil {
			logging.Error("[orchestrator] forced finalize failed: %v", err)
			o.busCtx.Mutable.SetResult("")
			o.busCtx.TaskState.LastError = fmt.Sprintf("forced finalize: %v", err)
			return stepsUsed
		}
		lastFinalize = out
	}

	o.recordTaskFinalize(lastFinalize)
	return stepsUsed
}

// applyWindowHint writes the rendered DAG-window hint into the
// shared TaskState.RetryHint slot so BuildAgentContext picks it up
// and BuildPromptContext renders it as the "Retry Directive (READ
// FIRST)" section. This is the only state field the DAG scheduler
// modifies on BusContext outside the standard PipelineStage / Stage
// fields. Empty hint clears the slot.
func (o *Orchestrator) applyWindowHint(hint string) {
	o.busCtx.TaskState.RetryHint = hint
}

// emitAnalysisReady projects AnalysisIR.TaskGraph into the renderer-
// facing TaskNodeInfo list and fires EventAnalysisReady. Hidden
// nodes (counterfactual, probe) are filtered here so the renderer
// can show one row per user-visible task without re-implementing
// the filtering rules.
//
// Probe nodes are pre-scan placeholders the analyzer uses internally;
// they do not correspond to a piece of the user's question. Counter-
// factual nodes are speculative branches that may never actually run
// — surfacing them as task rows would mislead the user about what
// the pipeline is committed to investigating.
//
// Finalize is intentionally kept in the projection so the user sees
// the "synthesise answer" step at the bottom of the list and the
// row turns green when the answer is ready.
func (o *Orchestrator) emitAnalysisReady() {
	if o.busCtx == nil || o.busCtx.AnalysisIR == nil {
		return
	}
	nodes := o.busCtx.AnalysisIR.TaskGraph.Nodes
	out := make([]render.TaskNodeInfo, 0, len(nodes))
	for _, n := range nodes {
		if n.IsCounterfactual {
			continue
		}
		if n.Type == types.NodeProbe {
			continue
		}
		out = append(out, render.TaskNodeInfo{
			ID:        n.ID,
			Type:      string(n.Type),
			Objective: n.Objective,
		})
	}
	if len(out) == 0 {
		return
	}
	o.emit(render.Event{
		Kind:      render.EventAnalysisReady,
		Timestamp: time.Now(),
		TraceID:   o.busCtx.TraceID,
		TaskNodes: out,
	})
}

// emitNodeStart / emitNodeEnd are thin wrappers around o.emit that
// also look up the node's Type and Objective so the renderer never
// needs to cross-reference the AnalysisIR. Called from runTaskGraph
// at every state.markRunning / markDone / markFailed / requeue site
// so the renderer's row state stays in lockstep with the scheduler.
func (o *Orchestrator) emitNodeStart(id string) {
	if id == "" || o.busCtx == nil || o.busCtx.AnalysisIR == nil {
		return
	}
	n := findNode(o.busCtx.AnalysisIR.TaskGraph, id)
	if n == nil {
		return
	}
	o.emit(render.Event{
		Kind:          render.EventTaskNodeStart,
		Timestamp:     time.Now(),
		NodeID:        id,
		NodeKind:      string(n.Type),
		NodeObjective: n.Objective,
	})
}

func (o *Orchestrator) emitNodeEnd(id string, ok bool, errMsg string) {
	if id == "" || o.busCtx == nil || o.busCtx.AnalysisIR == nil {
		return
	}
	n := findNode(o.busCtx.AnalysisIR.TaskGraph, id)
	if n == nil {
		return
	}
	ev := render.Event{
		Kind:          render.EventTaskNodeEnd,
		Timestamp:     time.Now(),
		NodeID:        id,
		NodeKind:      string(n.Type),
		NodeObjective: n.Objective,
	}
	if !ok {
		ev.Error = errMsg
		if ev.Error == "" {
			ev.Error = "criteria not met"
		}
	}
	o.emit(ev)
}

func findNode(g types.TaskGraph, id string) *types.TaskNode {
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			return &g.Nodes[i]
		}
	}
	return nil
}

// drainHypothesisVerdicts is the P2.1 Phase 10 hook invoked after a
// successful StageExtract dispatch. It reads the Turn B verdict
// buffer, applies MarkHypothesis for each entry, and LEAVES the
// buffer populated so the finalizer's prompt builder can render the
// rationale / citation text back to the user.
//
// Error handling policy:
//
//   - Unknown hypothesis id: log a warning and skip. The v3
//     schema-level emit_hypothesis_verdict tool already rejects
//     malformed calls at decode time, so reaching this path means
//     the LLM emitted a verdict for an id not in the hypothesis set
//     (hallucinated id or a typo). We never let a hallucinated id
//     corrupt the IR.
//
//   - Unknown status: same as above. MarkHypothesis validates the
//     enum and returns an error. Skip + warn.
//
//   - Nil AnalysisIR: the extractor dispatched without an analyzer
//     run (REPL bootstrap, unit tests). Skip the drain entirely;
//     the verdicts stay in the buffer but have no IR to write
//     through. This is the same fail-closed policy as Phase 11's
//     nil-Mutable check in the explorer.
//
// The function is a no-op when the buffer is empty, so it is always
// safe to call after any extract dispatch regardless of whether the
// LLM actually used emit_hypothesis_verdict.
func (o *Orchestrator) drainHypothesisVerdicts() {
	if o.busCtx == nil || o.busCtx.Mutable == nil || o.busCtx.AnalysisIR == nil {
		return
	}
	verdicts := o.busCtx.Mutable.EmittedHypothesisVerdicts()
	if len(verdicts) == 0 {
		return
	}
	applied := 0
	for _, v := range verdicts {
		if err := o.busCtx.AnalysisIR.MarkHypothesis(v.HypothesisID, v.Status); err != nil {
			logging.Warning("[orchestrator] hypothesis verdict drain: %v (rationale=%q citation=%q)",
				err, v.Rationale, v.Citation)
			continue
		}
		applied++
	}
	logging.Debug("[orchestrator] applied %d/%d hypothesis verdicts to IR; buffer retained for finalizer rendering",
		applied, len(verdicts))
}

// runAutoVerdicts evaluates criterion-based hypothesis auto-verdicts
// without dispatching the extractor LLM. Falsification conditions
// that are satisfied inject a "rejected" verdict; hypotheses whose
// RequiredEvidence is fully satisfied (but no LLM verdict exists)
// get an "inconclusive" verdict. This is the lightweight post-
// explore-window hook that replaced the per-window extract dispatch
// — the full LLM-backed extract runs once just before finalize.
func (o *Orchestrator) runAutoVerdicts() {
	if o.busCtx == nil || o.busCtx.AnalysisIR == nil || len(o.busCtx.AnalysisIR.HypothesisSet) == 0 {
		return
	}
	mu := o.busCtx.Mutable
	if mu == nil {
		return
	}
	var taToolResults []types.ToolResult
	if ta := mu.TurnAArtifacts(); ta != nil {
		taToolResults = ta.ToolResults
	}
	env := criterion.Env{
		IR:            o.busCtx.AnalysisIR,
		Evidence:      o.busCtx.EvidenceItems,
		ToolResults:   taToolResults,
		AnswerSymbols: o.busCtx.AnswerSymbols,
		PrescanBlob:   mu.PrescanSummaryBlob(),
	}
	existing := mu.EmittedHypothesisVerdicts()
	byID := make(map[string]bool, len(existing))
	for _, v := range existing {
		byID[v.HypothesisID] = true
	}
	var injected []types.HypothesisVerdict
	for _, h := range o.busCtx.AnalysisIR.HypothesisSet {
		fals := criterion.Eval(h.FalsificationCondition, env)
		if fals.Satisfied {
			if byID[h.ID] {
				logging.Warning("[orchestrator] auto-verdict: falsification satisfied for %s: forcing rejected", h.ID)
			}
			injected = append(injected, types.HypothesisVerdict{
				HypothesisID: h.ID,
				Status:       types.HypRejected,
				Rationale:    "falsification condition satisfied: " + fals.Detail,
			})
			continue
		}
		if byID[h.ID] {
			continue
		}
		okReq, _ := criterion.EvalAll(h.RequiredEvidence, env)
		if okReq && len(h.RequiredEvidence) > 0 {
			injected = append(injected, types.HypothesisVerdict{
				HypothesisID: h.ID,
				Status:       types.HypInconclusive,
				Rationale:    "required evidence satisfied but no LLM verdict emitted",
			})
		}
	}
	if len(injected) > 0 {
		mu.AppendEmittedHypothesisVerdicts(injected)
		logging.Info("[orchestrator] injected %d auto-verdict(s) from criterion evaluation", len(injected))
	}
}

// recordTaskFinalize copies the finalizer's FinalAnswer into
// Mutable.result and emits the objective-done event. Empty answers
// are still recorded — callers downstream (render layer) treat an
// empty result as "no answer" and display the fail state instead.
func (o *Orchestrator) recordTaskFinalize(out *agent.StageOutput) {
	answer := ""
	if out != nil {
		answer = out.FinalAnswer
	}
	o.busCtx.Mutable.SetResult(answer)
	logging.Debug("[orchestrator] final answer (len=%d):\n%s\n---", len(answer), answer)

	o.emit(render.Event{
		Kind:      render.EventObjectiveDone,
		Timestamp: time.Now(),
		Objective: o.busCtx.Mutable.Objective(),
	})
}

// dispatchStage runs the agent bound to the given stage and returns
// the StageOutput it produced. The output has already been routed
// through applyStageOutput by the time this function returns, so
// callers don't need to apply it again — they can just inspect
// fields like FinalAnswer that are useful for per-stage reactions
// (runTaskGraph uses this to write the finalizer's answer onto the
// task's Result).
func (o *Orchestrator) dispatchStage(stage types.PipelineStage) (*agent.StageOutput, error) {
	info, ok := pipelineTopology[stage]
	if !ok {
		return nil, fmt.Errorf("unknown pipeline stage: %s", stage)
	}
	agentName := info.Agent
	skillName := info.Skill

	ag, err := o.agents.Get(agentName)
	if err != nil {
		return nil, fmt.Errorf("get agent %s: %w", agentName, err)
	}

	sk, err := o.skills.Get(skillName)
	if err != nil {
		return nil, fmt.Errorf("get skill %s: %w", skillName, err)
	}

	o.busCtx.ActiveAgent = agentName
	agentCtx := ctxbuilder.BuildAgentContext(o.busCtx, agentName, stage)
	if ta, ok := o.thinkAloudMap[agentName]; ok {
		agentCtx.ThinkAloud = ta
	}

	// Adaptive explorer iteration scaling for multi-topic questions.
	if stage == types.StageExplore && o.busCtx.AnalysisIR != nil {
		if nSub := len(o.busCtx.AnalysisIR.RequestModel.SubTopics); nSub > 1 {
			agentCfg := o.settings.Agent
			base := agentCfg.MaxIterations
			extra := nSub * agentCfg.SubTopicExplorerBudgetExtra
			adjusted := base + extra
			if adjusted > 35 {
				adjusted = 35
			}
			if adjusted > base {
				agentCtx.MaxIterOverride = adjusted
				logging.Debug("[orchestrator] multi-topic explorer scaling: %d sub-topics, iterations %d → %d",
					nSub, base, adjusted)
			}
		}
	}

	logging.Info("[orchestrator] dispatching agent=%s skill=%s", agentName, skillName)

	stageStart := time.Now()
	o.emit(render.Event{
		Kind:      render.EventStageStart,
		Timestamp: stageStart,
		Stage:     stage,
		Agent:     agentName,
		Skill:     skillName,
	})
	o.emit(render.Event{
		Kind:      render.EventSkillBound,
		Timestamp: stageStart,
		Stage:     stage,
		Agent:     agentName,
		Skill:     skillName,
	})

	output, err := ag.Execute(agentCtx, sk)
	if err != nil {
		o.emit(render.Event{
			Kind:      render.EventStageEnd,
			Timestamp: time.Now(),
			Stage:     stage,
			Agent:     agentName,
			Error:     err.Error(),
		})
		return nil, fmt.Errorf("agent %s execution: %w", agentName, err)
	}

	// SubAgent decomposition path: replace the original output with
	// the merged sub-agent output for the rest of the pipeline.
	if proposal := extractSubAgentProposal(output, agentName); proposal != nil {
		logging.Info("[orchestrator] sub-agent proposal: %s (%d sub_tasks)", proposal.Reason, len(proposal.SubTasks))

		subTitle := ""
		if len(proposal.SubTasks) > 0 {
			subTitle = proposal.SubTasks[0].Title
		}
		o.emit(render.Event{
			Kind:         render.EventSubAgentStart,
			Timestamp:    time.Now(),
			Stage:        stage,
			SubAgentName: string(agentName),
			SubAgentID:   o.busCtx.TraceID + "-subagent",
			SubTaskTitle: subTitle,
			SubTaskCount: len(proposal.SubTasks),
		})

		merged, runErr := o.subRuntime.Run(o.busCtx, proposal)

		subErr := ""
		subTools := 0
		subFacts := 0
		if runErr != nil {
			logging.Error("[orchestrator] sub-agent run failed: %v, using original output", runErr)
			subErr = runErr.Error()
		} else {
			subTools = len(merged.ToolResults)
			subFacts = len(merged.NewFacts)
			output = merged
		}

		o.emit(render.Event{
			Kind:          render.EventSubAgentEnd,
			Timestamp:     time.Now(),
			Stage:         stage,
			SubAgentName:  string(agentName),
			SubAgentID:    o.busCtx.TraceID + "-subagent",
			ToolCallCount: subTools,
			FactCount:     subFacts,
			Error:         subErr,
		})
	}

	o.applyStageOutput(output)
	o.busCtx.TaskState.Completed = append(o.busCtx.TaskState.Completed, string(stage))


	stageErr := ""
	if output.Error != "" {
		stageErr = output.Error
	}
	o.emit(render.Event{
		Kind:      render.EventStageEnd,
		Timestamp: time.Now(),
		Stage:     stage,
		Agent:     agentName,
		Error:     stageErr,
	})

	return output, nil
}

// applyStageOutput updates BusContext with the results from an agent execution.
func (o *Orchestrator) applyStageOutput(output *agent.StageOutput) {
	if output == nil {
		return
	}

	// Append tool results
	o.busCtx.ToolResults = append(o.busCtx.ToolResults, output.ToolResults...)

	// Append MCP responses
	o.busCtx.MCPResponses = append(o.busCtx.MCPResponses, output.MCPResponses...)

	// Append new facts
	o.busCtx.RepoFacts = append(o.busCtx.RepoFacts, output.NewFacts...)

	// Merge-deduplicate structured evidence, dataflow findings, and
	// answer chains/symbols. These four slices are "truth sets" that
	// downstream prompt builders render verbatim; without dedup a
	// stage self-loop (explore → explore) would accumulate the same
	// items N times, because the explorer's ParseOutput re-emits the
	// full snapshot of its cumulative investigation on every entry.
	// See memory/project_applystage_dedup.md for the full rationale
	// and the stability lock tests in
	// internal/orchestrator/apply_stage_output_dedup_test.go.
	//
	// Tool results, MCP responses, and repo facts are LEFT appending
	// because they are per-call history logs, not dedupable truth
	// items — each entry corresponds to a distinct tool invocation
	// and the downstream consumers (e.g. ReAct history pruning,
	// debug logs) rely on that per-call granularity.
	o.busCtx.EvidenceItems = agent.MergeEvidenceItems(o.busCtx.EvidenceItems, output.EvidenceItems)
	o.busCtx.FlowFindings = agent.MergeFlowFindings(o.busCtx.FlowFindings, output.FlowFindings)
	o.busCtx.AnswerChains = types.MergeAnswerChains(o.busCtx.AnswerChains, output.AnswerChains)
	o.busCtx.AnswerSymbols = types.MergeAnswerSymbols(o.busCtx.AnswerSymbols, output.AnswerSymbols)

	// P2.1 AnswerSymbolCompleteness — last non-empty writer wins. The
	// zero value (CompletenessUnknown) means "no claim attached" and
	// must not overwrite a previously-written complete/lower_bound. On
	// an explorer→extractor hand-off the extractor's claim is always
	// more authoritative because it has seen Turn A's TerminalEvidenceCount
	// plus the emit_answer_symbol LLM claim; the "last writer wins"
	// rule reflects that ordering without encoding stage names. Invalid
	// values (should be impossible under the schema validator) are
	// silently dropped so a malformed stage output cannot corrupt the
	// BusContext field.
	if output.AnswerSymbolCompleteness != types.CompletenessUnknown && output.AnswerSymbolCompleteness.IsValid() {
		o.busCtx.AnswerSymbolCompleteness = output.AnswerSymbolCompleteness
	}

	// Append the stage's synthesized narrative so downstream stages
	// can read prior reasoning. The active agent/stage at this point
	// is whatever just executed.
	if output.StageReport != "" {
		o.busCtx.StageReports = append(o.busCtx.StageReports, types.StageReport{
			Stage:    o.busCtx.PipelineStage,
			Agent:    o.busCtx.ActiveAgent,
			Findings: output.StageReport,
		})
	}

	// Carry the agent's own retry diagnosis through to the next
	// dispatch. The forward explorer→finalize transition clears this
	// on BusContext so a hint from explore never leaks forward.
	o.busCtx.TaskState.RetryHint = output.RetryHint
	if output.RetryHint != "" {
		// Surface the retry reason to the user so they know the
		// pipeline is re-running, not stalled.
		summary := output.RetryHint
		if len(summary) > 200 {
			summary = summary[:197] + "..."
		}
		o.emit(render.Event{
			Kind:      render.EventAgentReasoning,
			Timestamp: time.Now(),
			Agent:     "orchestrator",
			Reasoning: "⟳ Evidence insufficient — retrying: " + summary,
		})
	}

	// Store the Analyzer v3 structured output on the first non-nil
	// value and never overwrite it. Subsequent re-dispatches of
	// analyze (rare but possible under retry budget) do not mutate
	// the IR in place.
	if output.AnalysisIR != nil && o.busCtx.AnalysisIR == nil {
		o.busCtx.AnalysisIR = output.AnalysisIR
	}

	// Update signals — only HasEnoughFacts survives after the
	// write-pipeline deletion.
	if output.SignalUpdates != nil && output.SignalUpdates.HasEnoughFacts {
		o.busCtx.Signals.HasEnoughFacts = true
	}

	// FinalAnswer is not captured here. runTaskGraph reads it
	// directly from the StageOutput returned by dispatchStage and
	// writes it onto the task's Result field via
	// Mutable.UpdateTaskResult.

	// Update missing piece
	o.busCtx.TaskState.Missing = output.MissingPiece

	// Record error if any
	if output.Error != "" {
		o.busCtx.TaskState.LastError = output.Error
	}
}


// BusContext returns the current bus context (for inspection/testing).
func (o *Orchestrator) BusContext() *types.BusContext {
	return o.busCtx
}

// extractSubAgentProposal scans tool results for a propose_sub_agents call
// and parses the proposal. Each sub_task is routed to a SubAgent of the same
// name as the calling Agent, so sub_agent is filled in from agentName here
// (the LLM-visible schema omits this field entirely).
func extractSubAgentProposal(output *agent.StageOutput, agentName types.AgentName) *types.SubAgentProposal {
	if output == nil {
		return nil
	}
	for _, r := range output.ToolResults {
		if r.ToolName == "propose_sub_agents" && r.Success {
			var proposal types.SubAgentProposal
			if err := json.Unmarshal([]byte(r.Summary), &proposal); err != nil {
				continue
			}
			if len(proposal.SubTasks) == 0 {
				continue
			}
			for i := range proposal.SubTasks {
				proposal.SubTasks[i].SubAgent = string(agentName)
			}
			return &proposal
		}
	}
	return nil
}
