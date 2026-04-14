package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
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
	settings    types.PipelineSettings
	agents      *agent.Registry
	skills      *skill.Registry
	busCtx      *types.BusContext
	maxSteps    int
	subRuntime  *agent.SubAgentRuntime
	stageVisits map[types.PipelineStage]int
	language    string
	emit        render.EventEmitter
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

// languageDirective returns the preference sentence to insert into
// BusContext.Preferences, or "" to disable the feature. The directive
// is phrased so the model keeps the user's language if it is clearly
// different from the default — this preserves the "ask in English,
// get English" behavior without forcing a single locale.
func languageDirective(lang string) string {
	switch lang {
	case "", "off", "none":
		return ""
	case "zh", "zh-CN", "zh-cn", "cn", "chinese":
		return "Respond to the user in Simplified Chinese (简体中文) by default. " +
			"Keep technical terms, proper nouns, project-specific names, code identifiers, " +
			"file paths, and command names in their original form — do not translate them. " +
			"If the user's most recent request is clearly written in another language " +
			"(for example English or Japanese), match that language instead so the reply " +
			"is in the same language as the question."
	case "en", "en-US", "english":
		return "Respond to the user in English by default. " +
			"Keep technical terms, proper nouns, and project-specific names in their original form. " +
			"If the user's most recent request is clearly written in another language, " +
			"match that language instead."
	default:
		return fmt.Sprintf(
			"Respond to the user in %s by default. "+
				"Keep technical terms, proper nouns, project-specific names, code identifiers, "+
				"file paths, and command names in their original form — do not translate them. "+
				"If the user's most recent request is clearly written in another language, "+
				"match that language instead.", lang)
	}
}

// diagramDirective is a global preference injected into every agent's
// prompt. It teaches agents to use Mermaid fenced code blocks for
// visual representations when the answer benefits from them.
const diagramDirective = `When a visual representation would clarify your answer, use Mermaid diagrams inside fenced code blocks (` + "```mermaid" + ` ... ` + "```" + `). Choose the diagram type that best fits the content:
- Flowchart (graph TD/LR) for control flow, decision trees, pipeline stages
- Sequence diagram (sequenceDiagram) for call chains, request/response flows, multi-component interactions
- Class diagram (classDiagram) for type hierarchies, struct relationships, interface implementations
- State diagram (stateDiagram-v2) for state machines, lifecycle transitions
- Standard Markdown tables for structured comparisons, field listings, configuration summaries
Use diagrams only when they add clarity — not every answer needs one. Keep diagrams concise: collapse trivial nodes, omit boilerplate, and label edges.`

// Run executes the full pipeline for a user request.
//
// The pipeline runs in two phases:
//
//   - Phase 1 — analyze: dispatch StageAnalyze once. The analyzer
//     populates BusContext.Mutable.TaskList (typically by calling
//     todo_write) so the orchestrator knows what work to do.
//
//   - Phase 2 — per-task: iterate over pending tasks, running a
//     mini-pipeline (explore → … → finalize) for each one. Per-task
//     state (Signals, MissingPiece, PipelineStage, oscillation
//     counter) resets between tasks; shared state (RepoFacts,
//     ToolResults, MCPResponses, Mutable) accumulates across tasks.
//     Each task's finalize call writes its result onto the task
//     itself via Mutable.UpdateTaskResult.
//
// The maxSteps budget is enforced globally across both phases.
func (o *Orchestrator) Run(request string, repoRoot string, branch string) (*types.BusContext, error) {
	// Initialize BusContext
	o.busCtx = &types.BusContext{
		PipelineStage: types.StageAnalyze,
		RepoRoot:      repoRoot,
		Branch:        branch,
		TraceID:       fmt.Sprintf("trace-%d", time.Now().UnixNano()),
		Mutable: types.NewMutableState(types.TaskList{
			Objective: request,
		}),
		TaskState: types.TaskState{
			Stage:   types.StageAnalyze,
			Missing: types.MissingUnderstanding,
		},
	}

	if pref := languageDirective(o.language); pref != "" {
		o.busCtx.Preferences = append(o.busCtx.Preferences, pref)
	}
	o.busCtx.Preferences = append(o.busCtx.Preferences, diagramDirective)

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

	// Phase 1: analyze.
	if used, err := o.runAnalyzePhase(); err != nil {
		logging.Error("[orchestrator] analyze phase failed: %v", err)
		o.busCtx.TaskState.LastError = fmt.Sprintf("analyze: %v", err)
	} else {
		stepsUsed += used
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

// runAnalyzePhase dispatches the analyze stage once and applies its
// output. It does not iterate; the orchestrator does not evaluate
// transitions out of analyze. The analyzer is expected to populate
// the task list via todo_write (or its fail-safe path) before
// returning so the per-task phase has work to do.
func (o *Orchestrator) runAnalyzePhase() (int, error) {
	o.busCtx.PipelineStage = types.StageAnalyze
	o.busCtx.TaskState.Stage = types.StageAnalyze
	o.busCtx.TaskState.Missing = types.MissingUnderstanding

	if _, err := o.dispatchStage(types.StageAnalyze); err != nil {
		return 1, err
	}
	return 1, nil
}

// runTaskPhase iterates over pending tasks and runs each through
// the DAG scheduler. Failed tasks do not abort the phase — the loop
// moves on to the next pending task. The total step budget
// (o.maxSteps) is enforced across all tasks; when it is exhausted
// any remaining pending tasks are marked failed.
//
// After the 2026-04-14 simplification the codrax pipeline is
// read-only: implementation stages (plan / implement / design_review
// / code_review / verify) are gone, every task routes through
// runTaskGraph with the analysis-only DAG scheduler.
func (o *Orchestrator) runTaskPhase(stepsUsed *int) error {
	for {
		next := o.nextPendingTask()
		if next == nil {
			return nil
		}
		if *stepsUsed >= o.maxSteps {
			logging.Error("[orchestrator] global max-steps (%d) exhausted; marking remaining tasks failed",
				o.maxSteps)
			o.busCtx.Mutable.UpdateTaskResult(next.ID, "", types.TaskFailed)
			continue
		}

		o.busCtx.Mutable.SetCurrentTask(next.ID)
		o.busCtx.Mutable.UpdateTaskStatus(next.ID, types.TaskInProgress)

		o.emit(render.Event{
			Kind:       render.EventTaskStatusChanged,
			Timestamp:  time.Now(),
			TaskID:     next.ID,
			TaskTitle:  next.Title,
			TaskStatus: types.TaskInProgress,
		})

		used := o.runTaskGraph(next.ID, o.maxSteps-*stepsUsed)
		*stepsUsed += used
	}
}

// runTaskGraph is the DAG-driven execution path and (after the
// 2026-04-14 simplification) the single per-task execution path.
// It walks AnalysisIR.TaskGraph.Nodes via graphState.
//
// P1.3 conservative-schedule (P1.3-MERGED-SCHEDULE):
//
// All ready non-finalize TaskNodes in the current readyNodes() batch
// are dispatched as ONE explorer execution per round. The merge
// trades node-level dispatch granularity for a 35-cell baseline that
// stays close to the legacy-pipeline LLM-call count. The deferred
// breakdown lives in memory/project_p1_3_deferred_items.md (D1, D2,
// D5, D7). When P2.1 lands two-turn explorer, this body is the
// place to relax the merge.
//
// Round structure:
//
//  1. Collect the current explorer window (every ready non-finalize
//     node). If non-empty, render its objectives + search hint
//     surfaces into a Retry Directive and dispatch StageExplore once.
//     Mark every window node done on success.
//
//  2. If a finalize node is now ready, dispatch StageFinalize, then
//     run the AnswerContract checker over its FinalAnswer.
//
//     - Pass: mark the finalize node done, record the answer, return.
//
//     - Fail with retry budget remaining: requeue the finalize node,
//       requeue every explorer-window node so the next round sees a
//       fresh batch, record one cross-window retry, and inject the
//       contract violation diagnostic into the next window's
//       RetryHint via pendingViolation.
//
//     - Fail with budget exhausted: prepend a fail-loud
//       "answer-contract validation exhausted" warning to the answer
//       and return the original answer body beneath it (P0.2 pattern).
//
//  3. Loop until allDone or stepsUsed hits the per-task cap.
//
// On any unexpected scheduler stall (no ready window, no ready
// finalize, but not all done) the function falls back to a forced
// finalize dispatch so the task always terminates with a Result.
func (o *Orchestrator) runTaskGraph(taskID string, stepBudget int) int {
	ir := o.busCtx.AnalysisIR
	if ir == nil || len(ir.TaskGraph.Nodes) == 0 {
		// Defensive: analyzer should always produce a non-empty
		// TaskGraph, but if something upstream failed we cannot
		// execute the task.
		logging.Error("[orchestrator] task %s: no AnalysisIR.TaskGraph — analyzer failed to produce a valid IR", taskID)
		o.busCtx.Mutable.UpdateTaskResult(taskID, "", types.TaskFailed)
		return 0
	}

	// Per-task state reset, mirroring legacy semantics so a multi-task
	// run that mixes IR + legacy never drags signals or stage visit
	// counters across the boundary.
	o.busCtx.Signals = types.ExecutionSignals{}
	o.busCtx.TaskState.Missing = types.MissingFacts
	o.stageVisits = make(map[types.PipelineStage]int)

	// P2.1 Phase 14 — cross-task reset of the Turn A/B handoff
	// surface. Multi-task runs (REPL turns, batched analysis, task
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
		// P2.2: the AnswerDocument buffer is the finalizer's output
		// channel under answer_document_mode=on. Reset it at per-task
		// entry alongside the P2.1 extractor buffers so a multi-task
		// run cannot drag a stale document from task N into task N+1.
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

	// pendingViolation carries the contract-checker diagnosis from the
	// previous failed finalize into the next explorer window so the
	// LLM sees the gap as a Retry Directive header.
	var pendingViolation string

	// Cap per-task budget at the EvidencePlan-declared max iterations
	// when present. The plan's MaxReactIters is "react iterations
	// expected for this scenario"; treating it as the per-task step
	// cap is the natural mapping under the merged schedule (each
	// dispatch consumes ~1 react loop's worth of LLM calls). Empty /
	// zero falls through to the orchestrator-passed stepBudget.
	if budget := ir.EvidencePlan.Budget.MaxReactIters; budget > 0 && budget < stepBudget {
		stepBudget = budget
	}

	stepsUsed := 0
	var lastFinalize *agent.StageOutput

	for stepsUsed < stepBudget && !state.allDone() {
		window := state.pendingExplorerWindow()
		fin := state.firstFinalizeReadyMerged()

		// The driver always tries the explorer window first, then
		// finalize. This deterministic order matches the templates'
		// hard_dependency edges (probe → evidence → … → finalize).
		if len(window) > 0 {
			hint := renderWindowHint(window, resolveSurface, pendingViolation)
			pendingViolation = ""
			o.applyWindowHint(hint)
			for _, n := range window {
				state.markRunning(n.ID)
			}

			o.busCtx.PipelineStage = types.StageExplore
			o.busCtx.TaskState.Stage = types.StageExplore
			stepsUsed++
			if _, err := o.dispatchStage(types.StageExplore); err != nil {
				logging.Error("[orchestrator] task %s DAG explore window failed: %v", taskID, err)
				for _, n := range window {
					state.markFailed(n.ID)
				}
				// Continue to the finalize attempt anyway — the
				// finalizer can still produce a partial answer that
				// the contract checker will judge.
			} else {
				for _, n := range window {
					state.markDone(n.ID)
				}
				// Turn B: immediately after the merged explorer
				// window completes successfully, dispatch the
				// extractor to drain Turn A's transcript into
				// structured emit_answer_symbol / emit_hypothesis_verdict
				// items.
				o.busCtx.PipelineStage = types.StageExtract
				o.busCtx.TaskState.Stage = types.StageExtract
				stepsUsed++
				if _, exDispatchErr := o.dispatchStage(types.StageExtract); exDispatchErr != nil {
					logging.Warning("[orchestrator] task %s DAG extract dispatch failed (continuing to finalize): %v", taskID, exDispatchErr)
				} else {
					// Turn B's hypothesis verdict drain hook. After the
					// extractor successfully emits emit_hypothesis_verdict
					// batches, Turn B's ParseOutput leaves the verdicts
					// in MutableState.EmittedHypothesisVerdicts instead
					// of copying them into StageOutput (MarkHypothesis
					// writes into AnalysisIR, which the extractor does
					// not own — the v3 contract makes the analyzer the
					// sole writer of the IR with MarkHypothesis as the
					// dedicated carve-out API). The orchestrator reads
					// the buffer here, applies MarkHypothesis for each
					// verdict, and LEAVES the buffer populated so the
					// finalizer's prompt builder can render the
					// rationale and citation back to the user.
					o.drainHypothesisVerdicts(taskID)
				}
			}
			continue
		}

		if fin == nil {
			// No ready window AND no ready finalize but not allDone:
			// scheduler stall. Force finalize so the task terminates.
			logging.Warning("[orchestrator] task %s DAG scheduler stalled; forcing finalize", taskID)
			break
		}

		state.markRunning(fin.ID)
		o.busCtx.PipelineStage = types.StageFinalize
		o.busCtx.TaskState.Stage = types.StageFinalize
		stepsUsed++
		out, err := o.dispatchStage(types.StageFinalize)
		if err != nil {
			logging.Error("[orchestrator] task %s DAG finalize failed: %v", taskID, err)
			state.markFailed(fin.ID)
			break
		}
		lastFinalize = out

		// Contract check.
		res := runContractCheck(out, ir.AnswerContract)
		if res.Passed {
			state.markDone(fin.ID)
			break
		}

		logging.Info("[orchestrator] task %s contract check failed (%d violation(s)); retryUsed=%d/%d",
			taskID, len(res.Violations), state.retryUsed, ir.TaskGraph.ExecutionPolicy.RetryBudget)

		if state.retryBudgetExhausted() {
			// Fail-loud — preserve the original answer beneath an
			// honest warning so the user sees the gap.
			out.FinalAnswer = appendViolationsToAnswer(out.FinalAnswer, res)
			lastFinalize = out
			state.markDone(fin.ID)
			break
		}

		// Backtrack: requeue the finalize node and every explorer-
		// window node that sits behind it, so the next round
		// re-runs the merged investigation with the violation
		// diagnostic in front. P1.3-MERGED-SCHEDULE: D2 in the
		// deferred memo will switch this to selective evidence
		// re-entry once node-level dispatch lands.
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
	}

	if lastFinalize == nil {
		// Force one finalize dispatch so the task always terminates
		// with a Result.
		logging.Warning("[orchestrator] task %s DAG run produced no finalize output; forcing finalize", taskID)
		o.busCtx.PipelineStage = types.StageFinalize
		o.busCtx.TaskState.Stage = types.StageFinalize
		stepsUsed++
		out, err := o.dispatchStage(types.StageFinalize)
		if err != nil {
			logging.Error("[orchestrator] task %s forced finalize failed: %v", taskID, err)
			o.busCtx.Mutable.UpdateTaskResult(taskID, "", types.TaskFailed)
			return stepsUsed
		}
		lastFinalize = out
	}

	o.recordTaskFinalize(taskID, lastFinalize)
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
func (o *Orchestrator) drainHypothesisVerdicts(taskID string) {
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
			logging.Warning("[orchestrator] task %s hypothesis verdict drain: %v (rationale=%q citation=%q)",
				taskID, err, v.Rationale, v.Citation)
			continue
		}
		applied++
	}
	logging.Debug("[orchestrator] task %s applied %d/%d hypothesis verdicts to IR; buffer retained for finalizer rendering",
		taskID, applied, len(verdicts))
}

// recordTaskFinalize copies the finalizer's FinalAnswer into the
// task's Result field via Mutable, and marks the task done. Empty
// answers still mark Done — the per-task loop's contract is that
// every task it runs ends with a definitive status.
func (o *Orchestrator) recordTaskFinalize(taskID string, out *agent.StageOutput) {
	answer := ""
	if out != nil {
		answer = out.FinalAnswer
	}
	actual := o.busCtx.Mutable.UpdateTaskResult(taskID, answer, types.TaskDone)
	if actual == "" {
		logging.Warning("[orchestrator] recordTaskFinalize: task list was empty; finalizer answer (%d bytes) dropped",
			len(answer))
	} else if actual != taskID {
		logging.Warning("[orchestrator] recordTaskFinalize: task ID %q not found, fell back to %q (likely a mid-pipeline todo_write replacement)",
			taskID, actual)
	}

	// Find the task title for the event.
	taskTitle := taskID
	tl := o.busCtx.Mutable.TaskList()
	for _, t := range tl.Tasks {
		if t.ID == taskID {
			taskTitle = t.Title
			break
		}
	}
	o.emit(render.Event{
		Kind:       render.EventTaskStatusChanged,
		Timestamp:  time.Now(),
		TaskID:     taskID,
		TaskTitle:  taskTitle,
		TaskStatus: types.TaskDone,
	})
}

// nextPendingTask returns a pointer to a copy of the next pending
// task, or nil if no pending tasks remain. The pointer is for
// convenience; do not mutate it directly — go through Mutable.
func (o *Orchestrator) nextPendingTask() *types.TaskItem {
	tl := o.busCtx.Mutable.TaskList()
	for i := range tl.Tasks {
		if tl.Tasks[i].Status == types.TaskPending {
			t := tl.Tasks[i]
			return &t
		}
	}
	return nil
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

	// The finalize stage always uses the structured AnswerDocument
	// channel (answer-document-skill) when it is registered. The skill
	// is shape-agnostic — the evaluator resolves the target shape from
	// AnalysisIR at BuildInitialPrompt time. Unit tests with a minimal
	// skill registry fall back to the default skill cleanly.
	if stage == types.StageFinalize {
		if _, err := o.skills.Get("answer-document-skill"); err == nil {
			skillName = "answer-document-skill"
		}
	}

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

	// Emit task list update if analyzer populated it.
	if stage == types.StageAnalyze && o.busCtx.Mutable != nil {
		tl := o.busCtx.Mutable.TaskList()
		if len(tl.Tasks) > 0 {
			o.emit(render.Event{
				Kind:      render.EventTaskListUpdated,
				Timestamp: time.Now(),
				TaskList:  &tl,
			})
		}
	}

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
	// dispatch. runTaskPipelineLegacy clears this on any forward transition
	// so a hint from explore never leaks into plan.
	o.busCtx.TaskState.RetryHint = output.RetryHint

	// Store the Analyzer v3 structured output on the first non-nil
	// value and never overwrite it. Subsequent re-dispatches of
	// analyze (rare but possible under retry budget) do not mutate
	// the IR in place.
	if output.AnalysisIR != nil && o.busCtx.AnalysisIR == nil {
		o.busCtx.AnalysisIR = output.AnalysisIR
	}

	// Update signals
	if output.SignalUpdates != nil {
		s := output.SignalUpdates
		if s.HasEnoughFacts {
			o.busCtx.Signals.HasEnoughFacts = true
		}
		if s.HasPlan {
			o.busCtx.Signals.HasPlan = true
		}
		if s.HasPatch {
			o.busCtx.Signals.HasPatch = true
		}
		if s.DesignReviewPassed {
			o.busCtx.Signals.DesignReviewPassed = true
		}
		if s.CodeReviewPassed {
			o.busCtx.Signals.CodeReviewPassed = true
		}
		if s.VerificationPassed {
			o.busCtx.Signals.VerificationPassed = true
		}
	}

	// FinalAnswer is no longer captured here. The per-task loop in
	// runTaskPipelineLegacy reads it directly from the StageOutput returned
	// by dispatchStage and writes it onto the task's Result field
	// via Mutable.UpdateTaskResult.

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
