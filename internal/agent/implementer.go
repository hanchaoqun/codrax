package agent

import (
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

type implementerEvaluator struct{}

func (e *implementerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	return "Implement the changes according to the plan. Write code, modify files, and generate patches."
}

func (e *implementerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	return len(resp.ToolCalls) == 0
}

func (e *implementerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	var lastContent string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			lastContent = messages[i].Content
			break
		}
	}

	// HasPatch tracks whether the implementer actually mutated the
	// workspace. apply_patch is the single canonical write tool — the
	// stale "write_file" name from before commit a52ae5a is gone, and
	// exec_command is dual-use so it does not count as patch evidence.
	hasPatch := false
	for _, r := range toolResults {
		if r.Success && r.ToolName == "apply_patch" {
			hasPatch = true
			break
		}
	}

	signals := &types.ExecutionSignals{HasPatch: hasPatch}

	return &StageOutput{
		Data:          json.RawMessage(fmt.Sprintf(`{"result": %q}`, lastContent)),
		SignalUpdates: signals,
	}, nil
}

func (e *implementerEvaluator) DetermineMissingPiece(ctx *types.AgentContext, output *StageOutput) types.MissingPiece {
	if output.SignalUpdates != nil && output.SignalUpdates.HasPatch {
		return types.MissingVerification
	}
	return types.MissingCode
}

// NewImplementerAgent creates the implementer agent (used in implement stage).
func NewImplementerAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentImplementer, deps, &implementerEvaluator{})
}
