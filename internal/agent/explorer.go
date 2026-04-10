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
		"- Prefer [high-confidence evidence] tools (grep, read_file, exec_command, …) — they read the real codebase and their results count as citable evidence.\n" +
		"- You must use at least 2 distinct evidence tool types AND make at least 4 evidence tool calls before concluding. Investigate thoroughly — read multiple files, cross-reference findings.\n" +
		"- Navigation tools (repo_map) are useful for orientation but do NOT count as evidence — always follow up with an evidence tool to verify.\n" +
		"- Prefer targeted tools (grep with --include, read_file with offset/limit) over broad listings.\n" +
		"- Avoid recursive list_files on the repository root.\n" +
		"- If a tool result was truncated, the message will name a path (the raw_ref) where the full output is stored. Re-read slices of it with read_file (offset/limit) or grep it for specific patterns instead of re-running the original command.\n" +
		"- For architecture or explanation questions, make sure to read all the key files involved — a shallow peek at one file is not enough."
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
// Evidence gate: the explorer must cross two floors before it may
// stop — at least 2 distinct evidence-tool types AND at least 4
// total evidence-tool calls. Until both are met, every soft-stop is
// rejected unconditionally. The type floor prevents trivial
// satisfaction (one grep and done); the call-count floor prevents
// shallow sweeps (one grep + one read_file on the same spot).
//
// Once the evidence gate is passed, a budget of 3 differentiated
// pushes applies:
//
//   - Push 0 — self-audit: "review your own answer against the
//     original question — are all aspects covered?" This catches
//     premature conclusions where the LLM answered part of the
//     question and forgot the rest.
//
//   - Push 1 — verify: "check at least one claim you haven't
//     verified with a tool yet." This catches confident-but-wrong
//     assertions that were inferred rather than observed.
//
//   - Push 2 — final escape: "if after all this you still believe
//     your answer is complete, state it. Otherwise try a different
//     approach or honestly say you don't know."
//
// The fourth soft-stop is accepted unconditionally. The prompts
// deliberately do NOT prescribe which tool to use; that depends on
// the question and is the LLM's job.
func (e *explorerEvaluator) ContinuationPrompt(resp llm.Response, iteration int, continuationCount int, history []types.ToolResult) (string, bool) {
	// Count evidence-bearing tool calls and distinct source types.
	evidenceSources := make(map[string]struct{})
	evidenceCalls := 0
	for _, r := range history {
		if r.Success && e.toolConfidence(r.ToolName) > 0.5 {
			evidenceSources[r.ToolName] = struct{}{}
			evidenceCalls++
		}
	}

	minTypes := 2
	minCalls := 4
	hasEnoughEvidence := len(evidenceSources) >= minTypes && evidenceCalls >= minCalls

	if !hasEnoughEvidence {
		// Evidence gate not met — push unconditionally. No escape.
		var reason string
		if len(evidenceSources) < minTypes {
			reason = fmt.Sprintf("only %d distinct evidence tool type(s) (need %d)", len(evidenceSources), minTypes)
		} else {
			reason = fmt.Sprintf("only %d evidence tool call(s) (need %d)", evidenceCalls, minCalls)
		}
		return fmt.Sprintf(
			"You have %s. "+
				"Take the next investigative step now — do not summarize, do not conclude, call a tool. "+
				"Read more files, grep for more patterns, or verify claims you have not checked yet.",
			reason), true
	}

	// Evidence gate met — graduated continuation budget.
	switch continuationCount {
	case 0:
		return "Before concluding, self-audit: re-read the original question and check whether your answer covers ALL aspects it asked about. If any part is unaddressed or only superficially covered, investigate that part now. Do not stop until every aspect of the question has evidence behind it.", true
	case 1:
		return "Pick the most important claim in your answer that you have NOT yet verified with a tool call. Verify it now — read the relevant file or grep for the relevant symbol. If it holds, good; if not, correct your answer.", true
	case 2:
		return "Final check. If your answer is genuinely complete and every key claim is backed by evidence, state your final answer now. If your last action revealed something that changes your answer, incorporate it. If you still cannot answer part of the question, say so honestly — an honest gap is better than a confident wrong answer.", true
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

	// HasEnoughFacts requires two conditions: at least 2 distinct
	// evidence-bearing tool types AND at least 4 total evidence
	// tool calls. The type floor ensures breadth (grep + read_file,
	// not just one tool repeated); the call-count floor ensures
	// depth (actually reading multiple locations, not one peek).
	evidenceCalls := 0
	for _, r := range toolResults {
		if r.Success && e.toolConfidence(r.ToolName) > 0.5 {
			evidenceCalls++
		}
	}
	signals := &types.ExecutionSignals{HasEnoughFacts: len(sources) >= 2 && evidenceCalls >= 4}

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
		if len(sources) < 2 {
			out.RetryHint = "Previous attempt used fewer than 2 distinct evidence tool types. The next attempt must use at least 2 different tools — choose them based on what the question actually needs."
		} else {
			out.RetryHint = fmt.Sprintf("Previous attempt made only %d evidence tool call(s) (need at least 4). The next attempt must investigate more thoroughly — read more files, grep for more patterns.", evidenceCalls)
		}
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
