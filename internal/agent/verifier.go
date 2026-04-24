package agent

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// verifierEvaluator is the B0 verify-stage agent's stub evaluator.
// The real verifier — which runs the project test suite inside the
// applied worktree via run_tests, parses per-language test output,
// and emits a ChangeReport via emit_test_results — lands in B3. The
// stub follows the same pattern as coderEvaluator: exit immediately,
// return a fail-loud error, let runVerifyPhase route it to
// TaskState.LastError.
//
// Paired stubs: run_tests and emit_test_results both return "not yet
// implemented" errors, so any LLM iteration on the verify stage in
// B0 would be wasted. Forcing ShouldStop=true at iter=0 saves an
// otherwise-certain-to-fail round-trip.
type verifierEvaluator struct{}

// BuildInitialInstruction returns an empty supplement. The skill's
// Workflow already reads as a B0 stub.
func (e *verifierEvaluator) BuildInitialInstruction(_ *types.AgentContext, _ *skill.Config) string {
	return ""
}

// ShouldStop exits the loop immediately at iter=0 because the stub
// tools cannot produce useful output.
func (e *verifierEvaluator) ShouldStop(_ llm.Response, _ int) bool {
	return true
}

// ParseOutput returns a clean stub error surfaced as TaskState.
// LastError upstream.
func (e *verifierEvaluator) ParseOutput(
	_ *types.AgentContext,
	_ []llm.Message,
	_ []types.ToolResult,
	_ []types.MCPResponse,
) (*StageOutput, error) {
	const msg = "[B0 skeleton] verify stage: verifier agent is a structural stub; real implementation ships in B3 with per-language test parsers."
	logging.Info("[verifier] stub invoked — returning fail-loud error")
	return &StageOutput{
		MissingPiece: types.MissingFacts,
		Error:        msg,
	}, fmt.Errorf("verifier: B0 skeleton stub; implementation deferred to B3")
}

// DetermineMissingPiece returns MissingFacts so the stub's exit is
// not mistaken for successful completion by retry budgets.
func (e *verifierEvaluator) DetermineMissingPiece(_ *types.AgentContext, output *StageOutput) types.MissingPiece {
	if output == nil {
		return types.MissingFacts
	}
	return output.MissingPiece
}

// NewVerifierAgent constructs the B0 verify-stage agent.
func NewVerifierAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentVerifier, deps, &verifierEvaluator{})
}
