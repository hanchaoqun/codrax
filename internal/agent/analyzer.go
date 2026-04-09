package agent

import (
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/design/internal/llm"
	"github.com/hanchaoqun/design/internal/skill"
	"github.com/hanchaoqun/design/internal/types"
)

// analyzerEvaluator customizes the ReAct loop for the analyzer agent,
// which is responsible only for the analyze stage: classify the user
// request and produce the initial task list. It used to live inside
// plannerEvaluator behind an `if ctx.Stage == StageAnalyze` branch;
// splitting it out gives every stage a 1:1 agent mapping.
type analyzerEvaluator struct{}

func (e *analyzerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	return "Please follow the workflow steps and produce output in the specified format."
}

func (e *analyzerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	return len(resp.ToolCalls) == 0
}

func (e *analyzerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
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

func (e *analyzerEvaluator) DetermineMissingPiece(ctx *types.AgentContext, _ *StageOutput) types.MissingPiece {
	return types.MissingFacts
}

// NewAnalyzerAgent creates the analyzer agent (used in the analyze stage).
func NewAnalyzerAgent(deps *Dependencies) Agent {
	eval := &analyzerEvaluator{}
	return NewBaseAgent(types.AgentAnalyzer, deps, eval)
}
