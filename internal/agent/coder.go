package agent

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// coderEvaluator is the B0 apply-stage agent's stub evaluator. The
// real coder — which consumes a ChangePlan and executes each
// ChangeUnit via apply_patch inside the active git worktree — lands
// in B2. For B0 the stub exists so:
//
//  1. runApplyPhase can dispatch the apply stage (so the Mode switch
//     in orchestrator.Run remains structurally valid even when the
//     write tools are not yet implemented).
//  2. Users running `--mode=apply` before B2 see a clean fail-loud
//     "not yet implemented" error rather than silent no-op.
//
// The stub ShouldStop immediately, ParseOutput returns a clean
// StageOutput.Error with the B0-boundary message. The apply_patch
// tool's Execute also returns "not yet implemented" — this evaluator
// just makes sure the agent-side path surfaces that cleanly without
// consuming LLM iterations.
type coderEvaluator struct{}

// BuildInitialInstruction returns an empty supplement — the skill's
// Workflow already reads "B0 stub" so nothing needs to be added
// dynamically.
func (e *coderEvaluator) BuildInitialInstruction(_ *types.AgentContext, _ *skill.Config) string {
	return ""
}

// ShouldStop exits the loop immediately at iter=0. No LLM iteration
// is useful when the underlying tool is stubbed.
func (e *coderEvaluator) ShouldStop(_ llm.Response, _ int) bool {
	return true
}

// ParseOutput returns a clean stub error so the dispatch chain
// surfaces a fail-loud "apply stage not implemented" message.
// runApplyPhase routes the Error to TaskState.LastError; the user
// sees a clear diagnostic instead of a silent run.
func (e *coderEvaluator) ParseOutput(
	_ *types.AgentContext,
	_ []llm.Message,
	_ []types.ToolResult,
	_ []types.MCPResponse,
) (*StageOutput, error) {
	const msg = "[B0 skeleton] apply stage: coder agent is a structural stub; real implementation ships in B2 alongside apply_patch and run_tests."
	logging.Info("[coder] stub invoked — returning fail-loud error")
	return &StageOutput{
		MissingPiece: types.MissingFacts,
		Error:        msg,
	}, fmt.Errorf("coder: B0 skeleton stub; implementation deferred to B2")
}

// DetermineMissingPiece returns MissingFacts so upstream retry
// budgets do not treat the stub's exit as a successful completion.
func (e *coderEvaluator) DetermineMissingPiece(_ *types.AgentContext, output *StageOutput) types.MissingPiece {
	if output == nil {
		return types.MissingFacts
	}
	return output.MissingPiece
}

// NewCoderAgent constructs the B0 apply-stage agent. Wiring mirrors
// NewPlannerAgent. When B2 lands, this constructor gains real
// ChangePlan parsing + per-unit dispatch; the evaluator struct grows
// fields to track apply progress against plan.target_paths.
func NewCoderAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentCoder, deps, &coderEvaluator{})
}
