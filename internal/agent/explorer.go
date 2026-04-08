package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/design/internal/llm"
	"github.com/hanchaoqun/design/internal/skill"
	"github.com/hanchaoqun/design/internal/types"
)

type explorerEvaluator struct{}

func (e *explorerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Objective: %s\n", ctx.Objective)
	fmt.Fprintf(&b, "Repository: %s (branch: %s)\n", ctx.RepoRoot, ctx.Branch)
	if ctx.CurrentTask != "" {
		fmt.Fprintf(&b, "Current Task: %s\n", ctx.CurrentTask)
	}
	if len(ctx.RelevantFacts) > 0 {
		b.WriteString("\nAlready known:\n")
		for _, f := range ctx.RelevantFacts {
			fmt.Fprintf(&b, "- %s\n", f)
		}
	}
	b.WriteString("\nExplore the codebase to build a trusted factual foundation. ")
	b.WriteString("Use the available tools to find entry points, understand module structure, and identify relevant files.\n")
	return b.String()
}

func (e *explorerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	return len(resp.ToolCalls) == 0
}

func (e *explorerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, mcpResponses []types.MCPResponse) (*StageOutput, error) {
	var lastContent string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			lastContent = messages[i].Content
			break
		}
	}

	// Extract facts from tool results
	var facts []types.RepoFact
	for _, r := range toolResults {
		if r.Success {
			facts = append(facts, types.RepoFact{
				Key:        r.ToolName,
				Value:      r.Summary,
				Source:     r.ToolName,
				Confidence: 0.8,
			})
		}
	}

	signals := &types.ExecutionSignals{HasEnoughFacts: len(facts) > 0}

	return &StageOutput{
		Data:          json.RawMessage(fmt.Sprintf(`{"result": %q}`, lastContent)),
		NewFacts:      facts,
		SignalUpdates: signals,
	}, nil
}

func (e *explorerEvaluator) DetermineMissingPiece(ctx *types.AgentContext, output *StageOutput) types.MissingPiece {
	if output.SignalUpdates != nil && output.SignalUpdates.HasEnoughFacts {
		return types.MissingPlan
	}
	return types.MissingFacts
}

// NewExplorerAgent creates the explorer agent (used in explore stage).
func NewExplorerAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentExplorer, deps, &explorerEvaluator{})
}
