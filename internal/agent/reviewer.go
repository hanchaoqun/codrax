package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/design/internal/llm"
	"github.com/hanchaoqun/design/internal/skill"
	"github.com/hanchaoqun/design/internal/types"
)

type reviewerEvaluator struct{}

func (e *reviewerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Objective: %s\n", ctx.Objective)

	// Context depends on whether this is a design review or code review
	if ctx.PlanSummary != "" {
		fmt.Fprintf(&b, "\nPlan to Review:\n%s\n", ctx.PlanSummary)
	}
	if ctx.PatchSummary != "" {
		fmt.Fprintf(&b, "\nCode Changes to Review:\n%s\n", ctx.PatchSummary)
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
	fmt.Fprintf(&b, "\nReview according to the skill workflow. Produce a pass/fail result with detailed findings.\n")
	return b.String()
}

func (e *reviewerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	return len(resp.ToolCalls) == 0
}

func (e *reviewerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	var lastContent string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			lastContent = messages[i].Content
			break
		}
	}

	// Simple heuristic: if the response contains "pass" and not "fail", review passed
	contentLower := strings.ToLower(lastContent)
	passed := strings.Contains(contentLower, "pass") && !strings.Contains(contentLower, "fail")

	signals := &types.ExecutionSignals{ReviewPassed: passed}

	return &StageOutput{
		Data:          json.RawMessage(fmt.Sprintf(`{"result": %q, "review_passed": %v}`, lastContent, passed)),
		SignalUpdates: signals,
	}, nil
}

func (e *reviewerEvaluator) DetermineMissingPiece(ctx *types.AgentContext, output *StageOutput) types.MissingPiece {
	if output.SignalUpdates != nil && output.SignalUpdates.ReviewPassed {
		return types.MissingNone
	}
	return types.MissingReview
}

// NewReviewerAgent creates the reviewer agent (used in review stage for both design and code reviews).
func NewReviewerAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentReviewer, deps, &reviewerEvaluator{})
}
