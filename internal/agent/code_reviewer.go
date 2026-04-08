package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/design/internal/llm"
	"github.com/hanchaoqun/design/internal/skill"
	"github.com/hanchaoqun/design/internal/types"
)

type codeReviewerEvaluator struct{}

func (e *codeReviewerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Objective: %s\n", ctx.Objective)

	if ctx.PatchSummary != "" {
		fmt.Fprintf(&b, "\nCode Changes to Review:\n%s\n", ctx.PatchSummary)
	}
	if ctx.PlanSummary != "" {
		fmt.Fprintf(&b, "\nOriginal Plan:\n%s\n", ctx.PlanSummary)
	}
	if len(ctx.RelevantFacts) > 0 {
		b.WriteString("\nKnown Facts:\n")
		for _, f := range ctx.RelevantFacts {
			fmt.Fprintf(&b, "- %s\n", f)
		}
	}
	if len(ctx.Constraints) > 0 {
		fmt.Fprintf(&b, "\nConstraints:\n")
		for _, c := range ctx.Constraints {
			fmt.Fprintf(&b, "- %s\n", c)
		}
	}
	fmt.Fprintf(&b, "\nReview the code changes according to the skill workflow. Produce a pass/fail result with detailed findings.\n")
	return b.String()
}

func (e *codeReviewerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	return len(resp.ToolCalls) == 0
}

func (e *codeReviewerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	var lastContent string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			lastContent = messages[i].Content
			break
		}
	}

	contentLower := strings.ToLower(lastContent)
	passed := strings.Contains(contentLower, "pass") && !strings.Contains(contentLower, "fail")

	signals := &types.ExecutionSignals{CodeReviewPassed: passed}

	return &StageOutput{
		Data:          json.RawMessage(fmt.Sprintf(`{"result": %q, "code_review_passed": %v}`, lastContent, passed)),
		SignalUpdates: signals,
	}, nil
}

func (e *codeReviewerEvaluator) DetermineMissingPiece(ctx *types.AgentContext, output *StageOutput) types.MissingPiece {
	if output.SignalUpdates != nil && output.SignalUpdates.CodeReviewPassed {
		return types.MissingNone
	}
	return types.MissingReview
}

// NewCodeReviewerAgent creates the code reviewer agent (used in code_review stage).
func NewCodeReviewerAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentCodeReviewer, deps, &codeReviewerEvaluator{})
}
