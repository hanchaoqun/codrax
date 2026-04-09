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
// request and produce the initial task list.
//
// The analyzer drives task list creation through the todo_write tool
// (see internal/tool/todo_write.go) — it does not parse the LLM's
// free-form output anymore. The tool mutates BusContext.Mutable
// directly, so by the time ParseOutput runs the working task list is
// already in place. ParseOutput's only remaining job is to capture
// the assistant's natural-language summary of what it decided.
type analyzerEvaluator struct{}

func (e *analyzerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	return "Classify the user request and call todo_write with the decomposed task list. " +
		"Then briefly explain your classification."
}

func (e *analyzerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	return len(resp.ToolCalls) == 0
}

func (e *analyzerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, _ []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	// Extract the last assistant message as the human-readable summary.
	var lastContent string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			lastContent = messages[i].Content
			break
		}
	}

	// If the LLM never called todo_write, fall back to a single-item
	// analysis-typed task so the orchestrator still has something to
	// route on. The fallback uses the user's original request as the
	// task title and writes through Mutable so we go through the same
	// channel a tool call would.
	if ctx.Mutable != nil {
		if tl := ctx.Mutable.TaskList(); len(tl.Tasks) == 0 {
			// Fail-safe default: the user request as a single
			// non-writing, non-risky task. Picks the analysis policy
			// so the pipeline answers the user instead of guessing
			// code changes.
			tl.Tasks = []types.TaskItem{{
				ID:       "task-1",
				Title:    tl.Objective,
				Writing:  false,
				HighRisk: false,
				Status:   types.TaskPending,
			}}
			tl.CurrentTaskID = "task-1"
			ctx.Mutable.SetTaskList(tl)
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
