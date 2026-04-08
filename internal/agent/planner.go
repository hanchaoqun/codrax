package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/design/internal/llm"
	"github.com/hanchaoqun/design/internal/skill"
	"github.com/hanchaoqun/design/internal/types"
)

// plannerEvaluator customizes the ReAct loop for the planner agent.
type plannerEvaluator struct{}

func (e *plannerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Task: %s\n", ctx.CurrentTask)
	if len(ctx.RelevantFacts) > 0 {
		b.WriteString("\nKnown Facts:\n")
		for _, f := range ctx.RelevantFacts {
			fmt.Fprintf(&b, "- %s\n", f)
		}
	}
	if ctx.MissingPiece != types.MissingNone {
		fmt.Fprintf(&b, "\nCurrently missing: %s\n", ctx.MissingPiece)
	}
	fmt.Fprintf(&b, "\nPlease follow the workflow steps and produce output in the specified format.\n")
	return b.String()
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
	switch ctx.Stage {
	case types.StageAnalyze:
		return types.MissingFacts
	case types.StagePlan:
		return types.MissingCode
	default:
		return types.MissingNone
	}
}

// NewPlannerAgent creates the planner agent (used in analyze and plan stages).
func NewPlannerAgent(deps *Dependencies) Agent {
	eval := &plannerEvaluator{}
	return NewBaseAgent(types.AgentPlanner, deps, eval)
}
