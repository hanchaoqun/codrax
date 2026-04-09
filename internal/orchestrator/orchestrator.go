package orchestrator

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/hanchaoqun/design/internal/agent"
	"github.com/hanchaoqun/design/internal/config"
	ctxbuilder "github.com/hanchaoqun/design/internal/context"
	"github.com/hanchaoqun/design/internal/skill"
	"github.com/hanchaoqun/design/internal/types"
)

// Orchestrator is the Layer 1 component that drives the pipeline state machine.
// It reads configuration from YAML, manages BusContext, dispatches agents,
// and evaluates transitions between stages.
type Orchestrator struct {
	config     *config.ResolvedConfig
	agents     *agent.Registry
	skills     *skill.Registry
	busCtx     *types.BusContext
	maxSteps   int
	subRuntime *agent.SubAgentRuntime
}

// New creates a new Orchestrator.
func New(cfg *config.ResolvedConfig, agents *agent.Registry, skills *skill.Registry, subAgents *agent.SubAgentRegistry) *Orchestrator {
	return &Orchestrator{
		config:     cfg,
		agents:     agents,
		skills:     skills,
		maxSteps:   50,
		subRuntime: agent.NewSubAgentRuntime(subAgents),
	}
}

// SetMaxSteps overrides the maximum number of pipeline steps (default 50).
func (o *Orchestrator) SetMaxSteps(n int) {
	o.maxSteps = n
}

// Run executes the full pipeline for a user request.
// It initializes the BusContext, then loops through stages until a terminal stage is reached.
func (o *Orchestrator) Run(request string, repoRoot string, branch string) (*types.BusContext, error) {
	// Initialize BusContext
	o.busCtx = &types.BusContext{
		PipelineStage: types.StageAnalyze,
		RepoRoot:      repoRoot,
		Branch:        branch,
		TraceID:       fmt.Sprintf("trace-%d", time.Now().UnixNano()),
		TaskList: types.TaskList{
			Objective: request,
		},
		TaskState: types.TaskState{
			Stage:   types.StageAnalyze,
			Missing: types.MissingUnderstanding,
		},
		Policy: types.PolicyContext{
			RequireReview:      o.config.PipelineSettings.RequireReview,
			MaxRetriesPerStage: o.config.PipelineSettings.MaxRetriesPerStage,
		},
	}

	log.Printf("[orchestrator] starting pipeline: trace=%s", o.busCtx.TraceID)

	// Pipeline loop
	for step := 0; step < o.maxSteps; step++ {
		stage := o.busCtx.PipelineStage

		log.Printf("[orchestrator] step %d: stage=%s, missing=%s",
			step, stage, o.busCtx.TaskState.Missing)

		// Check terminal condition
		stageConfig, err := o.config.GetStageConfig(stage)
		if err != nil {
			return o.busCtx, fmt.Errorf("unknown stage %s: %w", stage, err)
		}

		// Dispatch agent for current stage
		if err := o.executeStage(stageConfig); err != nil {
			log.Printf("[orchestrator] stage %s failed: %v", stage, err)
			o.busCtx.Signals.LastStageFailed = true
			o.busCtx.Signals.LastFailureReason = err.Error()
			o.busCtx.Signals.RetryCount++

			if o.busCtx.Signals.RetryCount > o.busCtx.Policy.MaxRetriesPerStage {
				log.Printf("[orchestrator] max retries exceeded for stage %s, forcing finalize", stage)
				o.busCtx.PipelineStage = types.StageFinalize
				o.busCtx.TaskState.Stage = types.StageFinalize
				continue
			}
		}

		// Check if we've reached a terminal stage after execution
		if stageConfig.Terminal {
			log.Printf("[orchestrator] reached terminal stage: %s", stage)
			o.busCtx.TaskState.IsTerminal = true
			break
		}

		// Decide next stage
		nextStage := o.decideNextStage()
		log.Printf("[orchestrator] transition: %s -> %s", stage, nextStage)

		o.busCtx.LastTransitionReason = fmt.Sprintf("%s -> %s (missing: %s)",
			stage, nextStage, o.busCtx.TaskState.Missing)
		o.busCtx.PipelineStage = nextStage
		o.busCtx.TaskState.Stage = nextStage

		// Reset retry count on successful transition to a new stage
		if nextStage != stage {
			o.busCtx.Signals.RetryCount = 0
			o.busCtx.Signals.LastStageFailed = false
		}
	}

	// Loop exited without reaching a terminal stage — max-steps was
	// exhausted. Force one finalize run so the user always gets an
	// answer (or at least a clearly-marked failure summary). The
	// forced finalizer runs outside the transition loop: its output is
	// applied via the normal applyStageOutput path, but we do not
	// re-enter decideNextStage afterwards.
	if !o.busCtx.TaskState.IsTerminal {
		log.Printf("[orchestrator] max-steps (%d) exhausted at stage %s, forcing finalize",
			o.maxSteps, o.busCtx.PipelineStage)
		o.busCtx.TaskState.LastError = fmt.Sprintf(
			"pipeline did not reach terminal stage within %d steps; forced finalize",
			o.maxSteps,
		)
		o.busCtx.LastTransitionReason = fmt.Sprintf(
			"%s -> finalize (max-steps exhausted)", o.busCtx.PipelineStage,
		)
		o.busCtx.PipelineStage = types.StageFinalize
		o.busCtx.TaskState.Stage = types.StageFinalize

		finalStageConfig, err := o.config.GetStageConfig(types.StageFinalize)
		if err != nil {
			return o.busCtx, fmt.Errorf("force finalize: %w", err)
		}
		if err := o.executeStage(finalStageConfig); err != nil {
			log.Printf("[orchestrator] forced finalize failed: %v", err)
		}
		o.busCtx.TaskState.IsTerminal = true
	}

	return o.busCtx, nil
}

// executeStage dispatches the appropriate agent for the current stage.
func (o *Orchestrator) executeStage(stageConfig *types.StageConfig) error {
	agentName := stageConfig.DefaultAgent
	skillName := stageConfig.DefaultSkill

	// Get the agent
	ag, err := o.agents.Get(agentName)
	if err != nil {
		return fmt.Errorf("get agent %s: %w", agentName, err)
	}

	// Get the skill
	sk, err := o.skills.Get(skillName)
	if err != nil {
		return fmt.Errorf("get skill %s: %w", skillName, err)
	}

	// Build agent context
	o.busCtx.ActiveAgent = agentName
	agentCtx := ctxbuilder.BuildAgentContext(o.busCtx, agentName, stageConfig.Name)

	log.Printf("[orchestrator] dispatching agent=%s skill=%s", agentName, skillName)

	// Execute the agent
	output, err := ag.Execute(agentCtx, sk)
	if err != nil {
		return fmt.Errorf("agent %s execution: %w", agentName, err)
	}

	// Check if the agent proposed SubAgent decomposition
	if proposal := extractSubAgentProposal(output, agentName); proposal != nil {
		log.Printf("[orchestrator] sub-agent proposal: %s (%d sub_tasks)", proposal.Reason, len(proposal.SubTasks))

		merged, runErr := o.subRuntime.Run(o.busCtx, proposal)
		if runErr != nil {
			log.Printf("[orchestrator] sub-agent run failed: %v, using original output", runErr)
		} else {
			o.applyStageOutput(merged)
			o.busCtx.TaskState.Completed = append(o.busCtx.TaskState.Completed, string(stageConfig.Name))
			return nil
		}
	}

	// No sub-agent decomposition — use original output
	o.applyStageOutput(output)

	// Record stage completion
	o.busCtx.TaskState.Completed = append(o.busCtx.TaskState.Completed, string(stageConfig.Name))

	return nil
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

	// Apply task list update (currently produced by analyzer)
	if output.TaskListUpdate != nil {
		o.busCtx.TaskList = *output.TaskListUpdate
	}

	// Capture final answer (currently produced by finalizer)
	if output.FinalAnswer != "" {
		o.busCtx.FinalAnswer = output.FinalAnswer
	}

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
	task := o.busCtx.TaskList.CurrentTask()
	if task != nil {
		switch task.Type {
		case types.TaskTypeAnalysis:
			return "analysis"
		case types.TaskTypeImplementation:
			if o.busCtx.Policy.RequireReview {
				return "high_risk_implementation"
			}
			return "implementation"
		}
	}

	// Fail-safe default: no classification → answer the user, do not mutate.
	return "analysis"
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
		// Can always finalize
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
