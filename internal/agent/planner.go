package agent

import (
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/design/internal/llm"
	"github.com/hanchaoqun/design/internal/skill"
	"github.com/hanchaoqun/design/internal/types"
)

// plannerEvaluator customizes the ReAct loop for the planner agent,
// which serves only the plan stage. The analyze stage was previously
// handled here as well; it now lives in analyzerEvaluator so each stage
// maps to a single agent.
type plannerEvaluator struct{}

func (e *plannerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	return "Please follow the workflow steps and produce output in the specified format."
}

func (e *plannerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	return len(resp.ToolCalls) == 0
}

func (e *plannerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	// Extract the last assistant message as output
	var lastContent string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			lastContent = messages[i].Content
			break
		}
	}
	return &StageOutput{
		Data: json.RawMessage(fmt.Sprintf(`{"result": %q}`, lastContent)),
	}, nil
}

func (e *plannerEvaluator) DetermineMissingPiece(ctx *types.AgentContext, _ *StageOutput) types.MissingPiece {
	return types.MissingCode
}

// NewPlannerAgent creates the planner agent (used in the plan stage).
func NewPlannerAgent(deps *Dependencies) Agent {
	eval := &plannerEvaluator{}
	return NewBaseAgent(types.AgentPlanner, deps, eval)
}
