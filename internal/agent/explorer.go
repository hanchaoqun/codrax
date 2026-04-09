package agent

import (
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

type explorerEvaluator struct{}

func (e *explorerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	return "Explore the codebase to build a trusted factual foundation. Use the available tools to find entry points, understand module structure, and identify relevant files.\n\n" +
		"Strategy hints:\n" +
		"- Prefer targeted tools (grep with --include, read_file with offset/limit) over broad listings.\n" +
		"- Use repo_map with a small max_depth for structural overview; avoid recursive list_files on the repository root.\n" +
		"- If a tool result was truncated, the message will name a path (the raw_ref) where the full output is stored. Re-read slices of it with read_file (offset/limit) or grep it for specific patterns instead of re-running the original command."
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
