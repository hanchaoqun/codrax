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
	// Never hard-stop. Voluntary stops (no tool calls + content) are
	// routed through ContinuationPrompt below so the evaluator can
	// override the BaseAgent default and push for actual investigation
	// instead of accepting a thinking-only summary.
	return false
}

// ContinuationPrompt implements ContinuingEvaluator. The explorer's
// failure mode that motivated this hook: the LLM articulates the
// next investigative step ("I'll check function X next") and then
// stops without doing it, because the BaseAgent's default soft-stop
// rule treats any content-only turn as completion.
//
// Budget = 2 with differentiated prompts:
//
//   - First push: "do it now". This catches the common case where
//     the LLM thought aloud about the next step but did not act.
//
//   - Second push: "if your last action did not answer it, try a
//     *different* approach — or honestly say you don't know". This
//     catches the case where the recovery action from the first
//     push was itself wrong (e.g. read_file at the wrong offset)
//     and the LLM is about to settle for a confident-but-wrong
//     summary. The "honest weakness > dishonest confidence" exit
//     is explicit in the prompt.
//
// The third soft-stop is accepted unconditionally — at that point
// the loop has had two corrective opportunities and another push
// would just be churn. The text deliberately does NOT prescribe
// which tool to use or what to look at; that depends on the
// question and is the LLM's job.
func (e *explorerEvaluator) ContinuationPrompt(resp llm.Response, iteration int, continuationCount int, history []types.ToolResult) (string, bool) {
	switch continuationCount {
	case 0:
		return "Your previous turn produced a summary without calling any tool. If you genuinely have a defensible answer, restate it in one sentence and stop. Otherwise, take the next concrete investigative step now — do not describe what you would do next, do it.", true
	case 1:
		return "Your previous turn still did not produce a defensible answer. If your last action did not get you closer, try a *different* approach — a different tool, a different file, a different query. If after this turn you still do not have a real answer, say so honestly in one sentence (an honest 'I do not know which X' is better than a confident wrong answer). Do not loop on the same approach.", true
	default:
		return "", false
	}
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
	sources := make(map[string]struct{})
	for _, r := range toolResults {
		if r.Success {
			facts = append(facts, types.RepoFact{
				Key:        r.ToolName,
				Value:      r.Summary,
				Source:     r.ToolName,
				Confidence: 0.8,
			})
			sources[r.ToolName] = struct{}{}
		}
	}

	// HasEnoughFacts is a necessary-condition floor: at least two
	// distinct tool sources must have been used. This blocks the
	// trivial "ran one command, declared done" failure mode without
	// taking a position on which tools are appropriate for which
	// kind of question — that judgment is left to the LLM, since a
	// hard-coded preference (e.g. "must read_file") would be wrong
	// for tasks that legitimately need only navigation or only
	// command execution.
	signals := &types.ExecutionSignals{HasEnoughFacts: len(sources) >= 2}

	out := &StageOutput{
		Data:          json.RawMessage(fmt.Sprintf(`{"result": %q}`, lastContent)),
		NewFacts:      facts,
		SignalUpdates: signals,
	}

	// Retry hint states the failed condition only. It deliberately
	// does not prescribe which tool to use next or what to call it
	// on; that depends on the question and is the LLM's job. The
	// hint exists so a self-looped attempt knows what changed
	// between dispatches, not so it gets a recipe.
	if !signals.HasEnoughFacts {
		out.RetryHint = "Previous attempt used fewer than 2 distinct tool types. The next attempt must use at least 2 different tools — choose them based on what the question actually needs."
	}

	return out, nil
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
