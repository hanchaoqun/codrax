package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/config"
	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Orchestrator is the Layer 1 component that drives the pipeline state machine.
// It reads configuration from YAML, manages BusContext, dispatches agents,
// and evaluates transitions between stages.
type Orchestrator struct {
	config      *config.ResolvedConfig
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
func New(cfg *config.ResolvedConfig, agents *agent.Registry, skills *skill.Registry, subAgents *agent.SubAgentRegistry) *Orchestrator {
	return &Orchestrator{
		config:     cfg,
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
		Policy: types.PolicyContext{
			RequireReview:      o.config.PipelineSettings.RequireReview,
			MaxRetriesPerStage: o.config.PipelineSettings.MaxRetriesPerStage,
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
	stageConfig, err := o.config.GetStageConfig(types.StageAnalyze)
	if err != nil {
		return 0, fmt.Errorf("get analyze stage: %w", err)
	}

	o.busCtx.PipelineStage = types.StageAnalyze
	o.busCtx.TaskState.Stage = types.StageAnalyze
	o.busCtx.TaskState.Missing = types.MissingUnderstanding

	if _, err := o.dispatchStage(stageConfig); err != nil {
		return 1, err
	}
	return 1, nil
}

// runTaskPhase iterates over pending tasks and runs each through its
// own mini stage pipeline. Failed tasks do not abort the phase — the
// loop moves on to the next pending task. The total step budget
// (o.maxSteps) is enforced across all tasks; when it is exhausted
// any remaining pending tasks are marked failed.
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

		used := o.runTaskPipeline(next.ID, o.maxSteps-*stepsUsed)
		*stepsUsed += used
	}
}

// runTaskPipeline runs the per-task stage loop for a single task.
// It resets per-task state (Signals, MissingPiece, PipelineStage,
// stage visit counter) on entry, runs explore → … → finalize, and
// always marks the task as Done or Failed before returning so the
// outer loop's nextPendingTask cannot pick it up again.
func (o *Orchestrator) runTaskPipeline(taskID string, stepBudget int) int {
	// Reset per-task state.
	o.busCtx.Signals = types.ExecutionSignals{}
	o.busCtx.TaskState.Missing = types.MissingFacts
	o.busCtx.PipelineStage = types.StageExplore
	o.busCtx.TaskState.Stage = types.StageExplore
	o.stageVisits = make(map[types.PipelineStage]int)

	visitCap := o.config.PipelineSettings.MaxStageVisits
	if visitCap <= 0 {
		visitCap = types.DefaultMaxStageVisits
	}

	stepsUsed := 0
	reachedFinalize := false

	for stepsUsed < stepBudget {
		stage := o.busCtx.PipelineStage

		logging.Debug("[orchestrator] task=%s step=%d stage=%s missing=%s",
			taskID, stepsUsed, stage, o.busCtx.TaskState.Missing)

		// Oscillation guard, scoped per task.
		o.stageVisits[stage]++
		if o.stageVisits[stage] > visitCap {
			logging.Warning("[orchestrator] task %s stage %s visited %d times (cap %d); oscillation guard tripped",
				taskID, stage, o.stageVisits[stage], visitCap)
			o.busCtx.TaskState.LastError = fmt.Sprintf(
				"task %s stuck at stage %s after %d visits", taskID, stage, o.stageVisits[stage])
			break
		}

		stageConfig, err := o.config.GetStageConfig(stage)
		if err != nil {
			o.busCtx.TaskState.LastError = fmt.Sprintf("unknown stage %s: %v", stage, err)
			break
		}

		stepsUsed++

		out, err := o.dispatchStage(stageConfig)
		if err != nil {
			logging.Error("[orchestrator] task %s stage %s failed: %v", taskID, stage, err)
			o.busCtx.Signals.LastStageFailed = true
			o.busCtx.Signals.LastFailureReason = err.Error()
			o.busCtx.Signals.RetryCount++
			if o.busCtx.Signals.RetryCount > o.busCtx.Policy.MaxRetriesPerStage {
				logging.Error("[orchestrator] task %s max retries exceeded at %s, jumping to finalize",
					taskID, stage)
				o.busCtx.PipelineStage = types.StageFinalize
				o.busCtx.TaskState.Stage = types.StageFinalize
				continue
			}
		}

		if stageConfig.Terminal {
			logging.Info("[orchestrator] task %s reached terminal stage %s", taskID, stage)
			o.recordTaskFinalize(taskID, out)
			reachedFinalize = true
			break
		}

		nextStage := o.decideNextStage()
		logging.Info("[orchestrator] task %s transition: %s -> %s", taskID, stage, nextStage)

		o.emit(render.Event{
			Kind:      render.EventTransition,
			Timestamp: time.Now(),
			FromStage: stage,
			ToStage:   nextStage,
		})
		o.busCtx.LastTransitionReason = fmt.Sprintf("%s -> %s (task %s, missing %s)",
			stage, nextStage, taskID, o.busCtx.TaskState.Missing)
		o.busCtx.PipelineStage = nextStage
		o.busCtx.TaskState.Stage = nextStage

		if nextStage != stage {
			o.busCtx.Signals.RetryCount = 0
			o.busCtx.Signals.LastStageFailed = false
			// A retry hint only makes sense within the same stage's
			// self-loop. Forward transitions discard it so the next
			// agent does not inherit a stale "do this differently"
			// directive aimed at the previous stage.
			o.busCtx.TaskState.RetryHint = ""
		}
	}

	// If we exited the loop without naturally reaching finalize, force
	// one finalize call so the task gets a Result (or at least a clear
	// failure marker).
	if !reachedFinalize {
		if o.busCtx.TaskState.LastError == "" {
			o.busCtx.TaskState.LastError = fmt.Sprintf(
				"task %s did not reach terminal within budget", taskID)
		}
		logging.Warning("[orchestrator] task %s forcing finalize: %s", taskID, o.busCtx.TaskState.LastError)
		o.busCtx.PipelineStage = types.StageFinalize
		o.busCtx.TaskState.Stage = types.StageFinalize

		finalConfig, err := o.config.GetStageConfig(types.StageFinalize)
		if err != nil {
			logging.Error("[orchestrator] task %s force finalize lookup failed: %v", taskID, err)
			o.busCtx.Mutable.UpdateTaskResult(taskID, "", types.TaskFailed)
			return stepsUsed
		}
		stepsUsed++
		out, err := o.dispatchStage(finalConfig)
		if err != nil {
			logging.Error("[orchestrator] task %s forced finalize failed: %v", taskID, err)
			o.busCtx.Mutable.UpdateTaskResult(taskID, "", types.TaskFailed)
			return stepsUsed
		}
		o.recordTaskFinalize(taskID, out)
	}

	return stepsUsed
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
// (runTaskPipeline uses this to write the finalizer's answer onto
// the task's Result).
func (o *Orchestrator) dispatchStage(stageConfig *types.StageConfig) (*agent.StageOutput, error) {
	agentName := stageConfig.DefaultAgent
	skillName := stageConfig.DefaultSkill

	// Per-policy skill override at the finalize stage. The default
	// final-answer-skill is shaped for implementation tasks (its
	// workflow is "summarize all changes, compile patch information,
	// write usage instructions, list action steps, mark tasks
	// complete"), and forcing an analysis answer through that template
	// produces verbose templated prose with invented Action Steps and
	// dilutes precise quantitative answers into mush. For analysis
	// pipelines we route to a Q&A-shaped skill instead. Only the
	// finalize stage gets this routing today; other stages have a
	// single shape that fits both modes.
	if stageConfig.Name == types.StageFinalize && o.determineActivePolicy() == "analysis" {
		if _, err := o.skills.Get("analysis-final-answer-skill"); err == nil {
			skillName = "analysis-final-answer-skill"
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
	agentCtx := ctxbuilder.BuildAgentContext(o.busCtx, agentName, stageConfig.Name)

	logging.Info("[orchestrator] dispatching agent=%s skill=%s", agentName, skillName)

	stageStart := time.Now()
	o.emit(render.Event{
		Kind:      render.EventStageStart,
		Timestamp: stageStart,
		Stage:     stageConfig.Name,
		Agent:     agentName,
		Skill:     skillName,
	})
	o.emit(render.Event{
		Kind:      render.EventSkillBound,
		Timestamp: stageStart,
		Stage:     stageConfig.Name,
		Agent:     agentName,
		Skill:     skillName,
	})

	output, err := ag.Execute(agentCtx, sk)
	if err != nil {
		o.emit(render.Event{
			Kind:      render.EventStageEnd,
			Timestamp: time.Now(),
			Stage:     stageConfig.Name,
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
			Stage:        stageConfig.Name,
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
			Stage:         stageConfig.Name,
			SubAgentName:  string(agentName),
			SubAgentID:    o.busCtx.TraceID + "-subagent",
			ToolCallCount: subTools,
			FactCount:     subFacts,
			Error:         subErr,
		})
	}

	o.applyStageOutput(output)
	o.busCtx.TaskState.Completed = append(o.busCtx.TaskState.Completed, string(stageConfig.Name))

	// Emit task list update if analyzer populated it.
	if stageConfig.Name == types.StageAnalyze && o.busCtx.Mutable != nil {
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
		Stage:     stageConfig.Name,
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

	// Append structured evidence, dataflow findings, and answer chains.
	o.busCtx.EvidenceItems = append(o.busCtx.EvidenceItems, output.EvidenceItems...)
	o.busCtx.FlowFindings = append(o.busCtx.FlowFindings, output.FlowFindings...)
	o.busCtx.AnswerChains = append(o.busCtx.AnswerChains, output.AnswerChains...)
	o.busCtx.AnswerSymbols = append(o.busCtx.AnswerSymbols, output.AnswerSymbols...)

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
	// dispatch. runTaskPipeline clears this on any forward transition
	// so a hint from explore never leaks into plan.
	o.busCtx.TaskState.RetryHint = output.RetryHint

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
	// runTaskPipeline reads it directly from the StageOutput returned
	// by dispatchStage and writes it onto the task's Result field
	// via Mutable.UpdateTaskResult.

	// Update missing piece
	o.busCtx.TaskState.Missing = output.MissingPiece

	// Record error if any
	if output.Error != "" {
		o.busCtx.TaskState.LastError = output.Error
	}
}

// decideNextStage evaluates transitions and selects the highest-priority valid next stage.
// This implements the core decision function described in the architecture doc.
func (o *Orchestrator) decideNextStage() types.PipelineStage {
	current := o.busCtx.PipelineStage

	// Get all transitions from current stage (already sorted by priority desc)
	transitions := o.config.GetTransitions(current)

	// Determine active task policy
	policyName := o.determineActivePolicy()

	// Filter by policy
	transitions = o.filterByPolicy(transitions, policyName)

	// Filter by feature flags
	transitions = o.filterByPipelineSettings(transitions)

	// Filter by runtime signals
	transitions = o.filterBySignals(transitions)

	// Select highest priority (first after filtering, since they're sorted)
	if len(transitions) > 0 {
		o.busCtx.TaskState.LastDecision = fmt.Sprintf(
			"selected %s (priority %d, policy=%s)",
			transitions[0].To, transitions[0].Priority, policyName,
		)
		return transitions[0].To
	}

	// Fallback: finalize
	o.busCtx.TaskState.LastDecision = "fallback to finalize (no valid transitions)"
	return types.StageFinalize
}

// determineActivePolicy determines which task policy applies based on the task type.
//
// When the task type is unknown (analyzer hasn't run, parse failed, or
// TaskList is empty) the fallback direction is "analysis" — fail-safe
// means "answer the user" rather than "start mutating code". A read-only
// pipeline that produces nothing is a much smaller failure mode than a
// write pipeline that mutates the wrong thing.
func (o *Orchestrator) determineActivePolicy() string {
	tl := o.busCtx.Mutable.TaskList()
	task := tl.CurrentTask()
	if task == nil {
		// Fail-safe default: no task → answer the user, do not mutate.
		return "analysis"
	}
	if !task.Writing {
		return "analysis"
	}
	// Writing task. Escalate to high-risk if the task itself says so
	// OR if the Policy flag forces review globally.
	if task.HighRisk || o.busCtx.Policy.RequireReview {
		return "high_risk_implementation"
	}
	return "implementation"
}

// filterByPolicy removes transitions to stages not allowed by the active policy.
func (o *Orchestrator) filterByPolicy(transitions []types.Transition, policyName string) []types.Transition {
	policy, err := o.config.GetTaskPolicy(policyName)
	if err != nil {
		return transitions // no policy = all stages allowed
	}

	allowed := make(map[types.PipelineStage]bool)
	for _, s := range policy.AllowedStages {
		allowed[s] = true
	}

	var filtered []types.Transition
	for _, t := range transitions {
		if allowed[t.To] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// filterByPipelineSettings removes transitions to stages disabled by pipeline settings.
func (o *Orchestrator) filterByPipelineSettings(transitions []types.Transition) []types.Transition {
	flags := o.config.PipelineSettings

	var filtered []types.Transition
	for _, t := range transitions {
		switch t.To {
		case types.StageVerify:
			if !flags.EnableVerify {
				continue
			}
		}
		filtered = append(filtered, t)
	}
	return filtered
}

// filterBySignals applies runtime signal-based filtering to transitions.
func (o *Orchestrator) filterBySignals(transitions []types.Transition) []types.Transition {
	signals := o.busCtx.Signals
	missing := o.busCtx.TaskState.Missing

	var filtered []types.Transition
	for _, t := range transitions {
		if o.isTransitionValidBySignals(t, signals, missing) {
			filtered = append(filtered, t)
		}
	}

	// If filtering removed everything, return the original list
	// to avoid getting stuck (the priority ordering will handle selection)
	if len(filtered) == 0 {
		return transitions
	}
	return filtered
}

// isTransitionValidBySignals checks if a transition makes sense given the current signals.
func (o *Orchestrator) isTransitionValidBySignals(t types.Transition, signals types.ExecutionSignals, missing types.MissingPiece) bool {
	switch t.To {
	case types.StageExplore:
		// Go to explore if we need facts
		return missing == types.MissingFacts || missing == types.MissingUnderstanding || !signals.HasEnoughFacts
	case types.StagePlan:
		// Go to plan if we have facts but need a plan
		return signals.HasEnoughFacts || missing == types.MissingPlan
	case types.StageImplement:
		// Go to implement if we have a plan
		return signals.HasPlan || missing == types.MissingCode
	case types.StageDesignReview:
		// Go to design review if there's a plan to review
		return signals.HasPlan
	case types.StageCodeReview:
		// Go to code review if there's a patch to review
		return signals.HasPatch
	case types.StageVerify:
		// Go to verify if we have a patch
		return signals.HasPatch || missing == types.MissingVerification
	case types.StageFinalize:
		// Finalizing directly out of explore is only valid if the
		// explorer reported enough facts. Otherwise we'd let a thin
		// single-tool exploration short-circuit into a final answer.
		// Blocking this edge here lets the lower-priority explore →
		// explore self-loop fire instead, with the oscillation guard
		// as the ultimate bound.
		if o.busCtx.PipelineStage == types.StageExplore {
			return signals.HasEnoughFacts
		}
		return true
	case types.StageAnalyze:
		// Backtrack to analyze only if fundamentally confused
		return missing == types.MissingUnderstanding
	}
	return true
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
