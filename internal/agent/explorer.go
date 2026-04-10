package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type explorerEvaluator struct {
	tools      *tool.Registry
	complexity string // set from AgentContext.CurrentTaskComplexity at dispatch time
	phase      int    // 0 = breadth scan, 1 = depth read
}

func (e *explorerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	// Capture complexity for use in ContinuationPrompt and ParseOutput,
	// which don't receive the AgentContext.
	e.complexity = ctx.CurrentTaskComplexity
	e.phase = 0 // start in breadth-scan phase

	return "## Phase 1: Breadth Scan\n\n" +
		"Your goal in this phase is to MAP the relevant territory — find ALL files related to the question. " +
		"Do NOT read files in full yet. Use lightweight tools:\n" +
		"- repo_map (task_map view) to get an overview of relevant files\n" +
		"- grep with files_only=true to find WHICH FILES contain key terms (just filenames, not lines). Do not use --include so you discover all file types\n" +
		"- list_files to understand directory structure\n\n" +
		"At the end of this phase, produce a FILE LIST of 3-6 files to read in depth. " +
		"For each file, note its ROLE and what you expect to learn from it.\n\n" +
		"Strategy:\n" +
		"- Search broadly: grep the core keyword without filtering by file type\n" +
		"- Classify each discovered file by role: (a) defines types/structures, (b) implements core logic, (c) declares configuration/topology/rules, (d) loads/parses configuration, (e) entry point. Prioritize roles a-d over e\n" +
		"- Exclude: test files, utility/infrastructure files (logging, tool wrappers), generated code\n" +
		"- Files that DECLARE rules or topology are as important as files that IMPLEMENT logic — include both in your list"
}

func (e *explorerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	// Never hard-stop. Voluntary stops (no tool calls + content) are
	// routed through ContinuationPrompt below so the evaluator can
	// override the BaseAgent default and push for actual investigation
	// instead of accepting a thinking-only summary.
	return false
}

// ContinuationPrompt implements ContinuingEvaluator with a two-phase
// exploration model:
//
// Phase 0 — Breadth Scan: lightweight tools only (grep, repo_map,
// list_files). The LLM maps the territory and identifies key files.
// When the LLM first tries to soft-stop in this phase, the prompt
// transitions to Phase 1 with a "now read these files" instruction.
//
// Phase 1 — Depth Read: the LLM reads the identified files in full,
// extracts detailed information, and cross-references. Continuation
// pushes in this phase focus on gap analysis and verification.
//
// This separation prevents the common failure mode where the LLM
// reads one file, concludes prematurely, then gets pushed into
// reading test files because "it hasn't read them yet."
func (e *explorerEvaluator) ContinuationPrompt(resp llm.Response, iteration int, continuationCount int, history []types.ToolResult) (string, bool) {
	if e.phase == 0 {
		// Phase 0 → Phase 1 transition: the LLM produced a breadth
		// scan summary. Now switch to depth reading.
		e.phase = 1
		return "## Phase 2: Depth Read\n\n" +
			"Good — you have mapped the relevant territory. Now switch to deep investigation.\n\n" +
			"Read the key source files you identified IN FULL (no offset/limit for files <500 lines). " +
			"For each file, extract:\n" +
			"- Key data structures, their fields, and what they control\n" +
			"- Control flow: conditionals, branches, state transitions\n" +
			"- Configuration-driven behavior: what changes based on config/policy\n" +
			"- Cross-references: when this file uses types/functions from another file\n\n" +
			"Do NOT read test files. Do NOT read utility/infrastructure files (logging, tool registration). " +
			"Focus on the source files that contain the actual logic you need to answer the question.\n\n" +
			"Start by reading the most important file now.", true
	}

	// Phase 1: depth read — evidence-gated continuation.
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
		var reason string
		if len(evidenceSources) < minTypes {
			reason = fmt.Sprintf("only %d distinct evidence tool type(s) (need %d)", len(evidenceSources), minTypes)
		} else {
			reason = fmt.Sprintf("only %d evidence tool call(s) (need %d)", evidenceCalls, minCalls)
		}
		return fmt.Sprintf(
			"You have %s. "+
				"Continue reading the key files from your scan. "+
				"Read source files in full, extract the details listed in the Phase 2 instructions.",
			reason), true
	}

	// Evidence gate met — graduated depth-phase budget.
	switch continuationCount {
	case 1: // continuationCount 0 was the phase transition
		return "Before concluding, check: have you read ALL the key files from your breadth scan? " +
			"If any important source file is still unread, read it now. " +
			"If you read a file but missed a critical section (e.g. a function you saw referenced but didn't read), go read that section.", true
	case 2:
		return "Final verification. If your investigation is complete, you may stop. " +
			"The synthesis step will produce the final answer from your evidence.", true
	default:
		return "", false
	}
}

func (e *explorerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, mcpResponses []types.MCPResponse) (*StageOutput, error) {
	// The last assistant message is always the synthesis response
	// (produced by the SynthesizingEvaluator step in BaseAgent.Execute
	// after the ReAct loop). If synthesis didn't run, fall back to the
	// last assistant message from the investigation loop.
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

// SynthesisPrompt implements SynthesizingEvaluator. After the ReAct
// investigation loop ends, BaseAgent calls this to get a synthesis
// prompt. The prompt includes a structured digest of all tool results
// so the LLM can produce a comprehensive answer in clean context,
// without the noise of intermediate summaries and continuation pushes.
func (e *explorerEvaluator) SynthesisPrompt(ctx *types.AgentContext, toolResults []types.ToolResult) (string, bool) {
	// Only synthesize if we have evidence-bearing results.
	var evidenceResults []types.ToolResult
	for _, r := range toolResults {
		if r.Success && e.toolConfidence(r.ToolName) > 0.5 {
			evidenceResults = append(evidenceResults, r)
		}
	}
	if len(evidenceResults) == 0 {
		return "", false
	}

	// Build a structured digest of all evidence tool results.
	var digest strings.Builder
	digest.WriteString("You have completed your investigation. Below is a structured digest of ALL evidence you collected.\n\n")
	digest.WriteString("## User Question\n")
	digest.WriteString(ctx.CurrentTask)
	digest.WriteString("\n\n## Evidence Collected\n\n")

	for i, r := range evidenceResults {
		// Truncate very long results to keep the synthesis prompt focused.
		summary := r.Summary
		if len(summary) > 3000 {
			summary = summary[:3000] + "\n... [truncated]"
		}
		fmt.Fprintf(&digest, "### [%d] %s\n```\n%s\n```\n\n", i+1, r.ToolName, summary)
	}

	digest.WriteString("## Instructions\n\n")
	digest.WriteString("Based on ALL the evidence above, write a comprehensive answer to the user's question. Requirements:\n")
	digest.WriteString("- Synthesize from the evidence, do not just list files\n")
	digest.WriteString("- For architecture/flow questions: explain the mechanism, conditions, and branching logic — not just component names\n")
	digest.WriteString("- Include a Mermaid diagram if the question asks for a flowchart or if a visual would clarify the answer\n")
	digest.WriteString("- Ground every key claim in a file:line citation from the evidence\n")
	digest.WriteString("- Match the answer depth to the question depth\n")

	return digest.String(), true
}

// evidenceThresholds returns the minimum distinct evidence tool types
// and minimum total evidence tool calls for a given complexity level.
// These thresholds gate both ContinuationPrompt (ReAct-loop level)
// and HasEnoughFacts (stage-level retry signal).
func evidenceThresholds(complexity string) (minTypes, minCalls int) {
	// Now that synthesis is separated from investigation (via
	// SynthesizingEvaluator), tool calls are pure investigation —
	// none are wasted on producing intermediate answers. This means
	// we can set higher floors without impacting response time
	// disproportionately.
	//
	// The gap between moderate and complex is kept small (1 call)
	// so that instability in the analyzer's complexity judgment
	// has minimal impact on investigation depth.
	switch complexity {
	case "simple":
		return 2, 3
	case "complex":
		return 2, 7
	default: // "moderate" or empty
		return 2, 6
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
