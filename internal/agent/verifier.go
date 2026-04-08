package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/design/internal/llm"
	"github.com/hanchaoqun/design/internal/skill"
	"github.com/hanchaoqun/design/internal/types"
)

type verifierEvaluator struct{}

func (e *verifierEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Objective: %s\n", ctx.Objective)
	if ctx.PatchSummary != "" {
		fmt.Fprintf(&b, "\nChanges to Verify:\n%s\n", ctx.PatchSummary)
	}
	fmt.Fprintf(&b, "Repository: %s (branch: %s)\n", ctx.RepoRoot, ctx.Branch)
	b.WriteString("\nVerify the implementation: compile, run tests, lint, and check for correctness.\n")
	return b.String()
}

func (e *verifierEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	return len(resp.ToolCalls) == 0
}

func (e *verifierEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	var lastContent string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			lastContent = messages[i].Content
			break
		}
	}

	// Check if all tool results succeeded
	allPassed := true
	for _, r := range toolResults {
		if !r.Success {
			allPassed = false
			break
		}
	}

	signals := &types.ExecutionSignals{VerificationPassed: allPassed}

	return &StageOutput{
		Data:          json.RawMessage(fmt.Sprintf(`{"result": %q, "verification_passed": %v}`, lastContent, allPassed)),
		SignalUpdates: signals,
	}, nil
}

func (e *verifierEvaluator) DetermineMissingPiece(ctx *types.AgentContext, output *StageOutput) types.MissingPiece {
	if output.SignalUpdates != nil && output.SignalUpdates.VerificationPassed {
		return types.MissingNone
	}
	return types.MissingVerification
}

// NewVerifierAgent creates the verifier agent (used in verify stage).
func NewVerifierAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentVerifier, deps, &verifierEvaluator{})
}
