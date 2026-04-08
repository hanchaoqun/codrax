package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/design/internal/llm"
	"github.com/hanchaoqun/design/internal/skill"
	"github.com/hanchaoqun/design/internal/types"
)

type finalizerEvaluator struct{}

func (e *finalizerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Objective: %s\n", ctx.Objective)
	if ctx.PlanSummary != "" {
		fmt.Fprintf(&b, "\nPlan:\n%s\n", ctx.PlanSummary)
	}
	if ctx.PatchSummary != "" {
		fmt.Fprintf(&b, "\nChanges Made:\n%s\n", ctx.PatchSummary)
	}
	if ctx.ReviewSummary != "" {
		fmt.Fprintf(&b, "\nReview Result:\n%s\n", ctx.ReviewSummary)
	}
	if ctx.VerificationSummary != "" {
		fmt.Fprintf(&b, "\nVerification Result:\n%s\n", ctx.VerificationSummary)
	}
	if len(ctx.RelevantToolSummaries) > 0 {
		b.WriteString("\nTool Results:\n")
		for _, s := range ctx.RelevantToolSummaries {
			fmt.Fprintf(&b, "- %s\n", s)
		}
	}
	b.WriteString("\nCompile all results into a clear, actionable final answer for the user.\n")
	return b.String()
}

func (e *finalizerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	// Finalizer typically doesn't need tools — stop after first response
	return true
}

func (e *finalizerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, _ []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	var lastContent string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			lastContent = messages[i].Content
			break
		}
	}
	return &StageOutput{
		Data: json.RawMessage(fmt.Sprintf(`{"final_answer": %q}`, lastContent)),
	}, nil
}

func (e *finalizerEvaluator) DetermineMissingPiece(_ *types.AgentContext, _ *StageOutput) types.MissingPiece {
	return types.MissingNone
}

// NewFinalizerAgent creates the finalizer agent (used in finalize stage).
func NewFinalizerAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentFinalizer, deps, &finalizerEvaluator{})
}
