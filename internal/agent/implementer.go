package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/design/internal/llm"
	"github.com/hanchaoqun/design/internal/skill"
	"github.com/hanchaoqun/design/internal/types"
)

type implementerEvaluator struct{}

func (e *implementerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Objective: %s\n", ctx.Objective)
	if ctx.PlanSummary != "" {
		fmt.Fprintf(&b, "\nImplementation Plan:\n%s\n", ctx.PlanSummary)
	}
	if len(ctx.RelevantFiles) > 0 {
		fmt.Fprintf(&b, "\nRelevant Files:\n")
		for _, f := range ctx.RelevantFiles {
			fmt.Fprintf(&b, "- %s\n", f)
		}
	}
	if len(ctx.Constraints) > 0 {
		fmt.Fprintf(&b, "\nConstraints:\n")
		for _, c := range ctx.Constraints {
			fmt.Fprintf(&b, "- %s\n", c)
		}
	}
	b.WriteString("\nImplement the changes according to the plan. Write code, modify files, and generate patches.\n")
	return b.String()
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

	// Check if any write operations succeeded
	hasPatch := false
	for _, r := range toolResults {
		if r.Success && (r.ToolName == "write_file" || r.ToolName == "exec_command") {
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
