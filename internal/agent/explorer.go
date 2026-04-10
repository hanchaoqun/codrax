package agent

import (
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type explorerEvaluator struct {
	tools *tool.Registry
}

func (e *explorerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	return "Explore the codebase to build a trusted factual foundation. Use the available tools to find entry points, understand module structure, and identify relevant files.\n\n" +
		"Strategy hints:\n" +
		"- Prefer [high-confidence evidence] tools (grep, read_file, exec_command, …) — they read the real codebase and their results count as citable evidence. Use at least 2 distinct evidence tools before concluding.\n" +
		"- Navigation tools (repo_map) are useful for orientation but do NOT count as evidence — always follow up with an evidence tool to verify.\n" +
		"- Prefer targeted tools (grep with --include, read_file with offset/limit) over broad listings.\n" +
		"- Avoid recursive list_files on the repository root.\n" +
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
	// Count distinct evidence-bearing tool sources already used.
	// If the explorer has not yet crossed the minimum evidence
	// threshold, we keep pushing regardless of how many continuation
	// prompts have been sent — the "easy out" is only offered once
	// the evidence floor is met.
	evidenceSources := make(map[string]struct{})
	for _, r := range history {
		if r.Success && e.toolConfidence(r.ToolName) > 0.5 {
			evidenceSources[r.ToolName] = struct{}{}
		}
	}
	hasEnoughEvidence := len(evidenceSources) >= 2

	if !hasEnoughEvidence {
		// Evidence floor not met — push unconditionally. No escape
		// hatch: the explorer must call more tools before it may stop.
		return fmt.Sprintf(
			"You have only used %d distinct evidence tool type(s) so far. "+
				"You need at least 2 (e.g. grep + read_file) before you can conclude. "+
				"Take the next investigative step now — do not summarize, do not conclude, call a tool.",
			len(evidenceSources)), true
	}

	// Evidence floor met — normal continuation budget.
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

	// Extract facts from tool results. Each tool declares its own
	// Confidence via the Tool interface: evidence tools (grep,
	// read_file, …) return 0.8, navigation indexes (repo_map) return
	// 0.3, and orchestration tools (todo_write, propose_sub_agents)
	// return 0.0. Only tools with Confidence > 0.5 count toward the
	// evidence-source floor below.
	var facts []types.RepoFact
	sources := make(map[string]struct{})
	for _, r := range toolResults {
		if r.Success {
			confidence := e.toolConfidence(r.ToolName)
			facts = append(facts, types.RepoFact{
				Key:        r.ToolName,
				Value:      r.Summary,
				Source:     r.ToolName,
				Confidence: confidence,
			})
			// Only evidence-bearing tools (Confidence > 0.5) count
			// toward the "enough facts" floor. Navigation indexes and
			// orchestration tools are excluded so the explorer cannot
			// satisfy this by mapping the repo without actually reading
			// or grepping the code.
			if confidence > 0.5 {
				sources[r.ToolName] = struct{}{}
			}
		}
	}

	// HasEnoughFacts is a necessary-condition floor: at least two
	// distinct evidence-bearing tool sources must have been used.
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

// toolConfidence returns the Confidence declared by the named tool.
// Falls back to 0.8 (evidence-level) for unknown tools so that MCP
// tools or future additions are not silently excluded from the
// evidence-source count.
func (e *explorerEvaluator) toolConfidence(name string) float64 {
	if e.tools == nil {
		return 0.8
	}
	t, err := e.tools.Get(name)
	if err != nil {
		return 0.8
	}
	return t.Confidence()
}

// NewExplorerAgent creates the explorer agent (used in explore stage).
func NewExplorerAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentExplorer, deps, &explorerEvaluator{tools: deps.Tools})
}
