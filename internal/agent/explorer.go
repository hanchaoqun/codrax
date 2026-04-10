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
	tools      *tool.Registry
	complexity string // set from AgentContext.CurrentTaskComplexity at dispatch time
}

func (e *explorerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	// Capture complexity for use in ContinuationPrompt and ParseOutput,
	// which don't receive the AgentContext.
	e.complexity = ctx.CurrentTaskComplexity
	minTypes, minCalls := evidenceThresholds(e.complexity)
	return fmt.Sprintf(
		"Explore the codebase to build a trusted factual foundation. Use the available tools to find entry points, understand module structure, and identify relevant files.\n\n"+
			"Investigation depth for this task: %s (minimum %d distinct evidence tool types, %d evidence tool calls).\n\n"+
			"Strategy hints:\n"+
			"- Prefer [high-confidence evidence] tools (grep, read_file, exec_command, …) — they read the real codebase and their results count as citable evidence.\n"+
			"- Navigation tools (repo_map) are useful for orientation — they show which files and symbols exist and rank them by relevance. Use repo_map to get a starting map, then verify with evidence tools. repo_map does NOT count as evidence.\n"+
			"- For source files (<500 lines), call read_file WITHOUT offset/limit to get the full file — small-file reads are cheap and you see the complete picture. Only use offset/limit for large files (>500 lines) or when a tool result tells you the output was truncated.\n"+
			"- Avoid recursive list_files on the repository root.\n"+
			"- If a tool result was truncated, the message will name a path (the raw_ref) where the full output is stored. Re-read slices of it with read_file (offset/limit) or grep it for specific patterns instead of re-running the original command.\n"+
			"- Prioritize reading source files over test files — tests show usage examples but the actual logic lives in the source. Read config files (YAML, etc.) when the question involves system topology or behavior settings.",
		complexityLabel(ctx.CurrentTaskComplexity), minTypes, minCalls)
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
// Evidence gate: complexity-adaptive. The thresholds scale with the
// task's declared complexity (simple/moderate/complex). Until both
// the type floor and call-count floor are met, every soft-stop is
// rejected unconditionally.
//
// Once the evidence gate is passed, a budget of 3 differentiated
// pushes applies:
//
//   - Push 0 — gap analysis: list what aspects of the question are
//     NOT yet covered, then investigate the biggest gap.
//
//   - Push 1 — verify: check the most important unverified claim.
//
//   - Push 2 — final escape: state the answer or honestly say what
//     is missing.
//
// The fourth soft-stop is accepted unconditionally.
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

	minTypes, minCalls := evidenceThresholds(e.complexity)
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
		// Structured gap analysis: force the LLM to audit coverage
		// against concrete dimensions, not just vague "all aspects".
		return "Before concluding, perform a structured gap analysis. " +
			"For the question asked, check your answer against these dimensions:\n" +
			"- WHAT: did you name every key component/concept involved?\n" +
			"- HOW: did you explain the mechanism (not just list names)?\n" +
			"- WHEN/WHY: did you explain conditions, triggers, or branching logic?\n" +
			"- WHERE: did you cite specific file:line for each claim?\n" +
			"List which dimensions are still weak, then investigate the weakest one now — " +
			"read the specific file that would fill that gap. Do NOT re-read files you already saw; " +
			"find the file you haven't read yet that contains the missing piece.", true
	case 1:
		return "Pick the most important claim in your answer that you have NOT yet verified with a tool call. Verify it now — read the relevant file or grep for the relevant symbol. If it holds, good; if not, correct your answer.", true
	case 2:
		return "This is your final turn. Write your COMPLETE answer now — it must incorporate ALL findings from your entire investigation, not just the last thing you checked. " +
			"Restate the full answer from scratch, integrating everything you learned across all tool calls. " +
			"This final message will be passed to downstream stages as your stage report, so it must be self-contained and comprehensive.", true
	default:
		return "", false
	}
}

func (e *explorerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, mcpResponses []types.MCPResponse) (*StageOutput, error) {
	// Find both the last assistant message and the longest one.
	// The continuation mechanism can push the explorer to investigate
	// further after an initial comprehensive answer. When that happens,
	// the last message is often a brief note about the final check,
	// while the best answer is an earlier, longer message. If the last
	// message is less than half the length of the longest, use the
	// longest — it's almost certainly the comprehensive synthesis.
	var lastContent, longestContent string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			if lastContent == "" {
				lastContent = messages[i].Content
			}
			if len(messages[i].Content) > len(longestContent) {
				longestContent = messages[i].Content
			}
		}
	}
	// Prefer the longest message if the last one is a fragment.
	// Also set StageReport here (instead of relying on BaseAgent's
	// auto-capture which always picks the last message) so that the
	// best comprehensive answer flows to downstream stages.
	bestReport := lastContent
	if len(longestContent) > 0 && len(lastContent) < len(longestContent)/2 {
		lastContent = longestContent
		bestReport = longestContent
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

	// HasEnoughFacts is complexity-adaptive: thresholds scale with
	// the task's declared complexity (simple/moderate/complex).
	evidenceCalls := 0
	for _, r := range toolResults {
		if r.Success && e.toolConfidence(r.ToolName) > 0.5 {
			evidenceCalls++
		}
	}
	minTypes, minCalls := evidenceThresholds(e.complexity)
	signals := &types.ExecutionSignals{HasEnoughFacts: len(sources) >= minTypes && evidenceCalls >= minCalls}

	out := &StageOutput{
		Data:          json.RawMessage(fmt.Sprintf(`{"result": %q}`, lastContent)),
		NewFacts:      facts,
		SignalUpdates: signals,
		StageReport:   bestReport, // pre-set so BaseAgent.Execute skips auto-capture
	}

	// Retry hint states the failed condition only.
	if !signals.HasEnoughFacts {
		if len(sources) < minTypes {
			out.RetryHint = fmt.Sprintf("Previous attempt used only %d distinct evidence tool type(s) (need %d). The next attempt must use more different tools.", len(sources), minTypes)
		} else {
			out.RetryHint = fmt.Sprintf("Previous attempt made only %d evidence tool call(s) (need %d for %s complexity). The next attempt must investigate more thoroughly — read more files, grep for more patterns.", evidenceCalls, minCalls, complexityLabel(e.complexity))
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

// evidenceThresholds returns the minimum distinct evidence tool types
// and minimum total evidence tool calls for a given complexity level.
// These thresholds gate both ContinuationPrompt (ReAct-loop level)
// and HasEnoughFacts (stage-level retry signal).
func evidenceThresholds(complexity string) (minTypes, minCalls int) {
	switch complexity {
	case "simple":
		return 2, 2
	case "complex":
		return 2, 8
	default: // "moderate" or empty
		return 2, 4
	}
}

// complexityLabel returns a human-readable label for logging and
// prompt messages. Empty string defaults to "moderate".
func complexityLabel(complexity string) string {
	switch complexity {
	case "simple":
		return "simple"
	case "complex":
		return "complex"
	default:
		return "moderate"
	}
}

// NewExplorerAgent creates the explorer agent (used in explore stage).
func NewExplorerAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentExplorer, deps, &explorerEvaluator{tools: deps.Tools})
}
