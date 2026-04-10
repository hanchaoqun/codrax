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
	tools *tool.Registry
	phase int // 0 = breadth scan, 1 = depth read
}

func (e *explorerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
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

	// Phase 1 (depth read): use runtime file coverage as guidance.
	// Extract discovered files (grep files_only) and read files
	// (read_file) to show the LLM what it hasn't read yet.
	discovered, readSet := extractFileCoverage(history)
	var unread []string
	for _, f := range discovered {
		if !readSet[f] {
			unread = append(unread, f)
		}
	}

	// Hard budget: phase transition used count 0, depth starts at 1.
	// Cap at 3 pushes to prevent empty loops.
	depthBudget := continuationCount - 1

	if depthBudget < 3 {
		// Show coverage status and unread files. Let the LLM decide
		// which (if any) are worth reading — not all discovered files
		// are equally important.
		var hint strings.Builder
		fmt.Fprintf(&hint,
			"File coverage: %d read out of %d discovered.\n", len(readSet), len(discovered))
		if len(unread) > 0 {
			hint.WriteString("Unread files that matched the query (may or may not be relevant):\n")
			for _, f := range unread {
				hint.WriteString("- " + f + "\n")
			}
			hint.WriteString("\nIf any of these files are likely to contain key information for the question, read them now. ")
			hint.WriteString("Skip files that are clearly secondary (utilities, documentation, tangential modules). ")
			hint.WriteString("Do NOT re-read files you have already seen.")
		} else {
			hint.WriteString("All discovered files have been read. You may stop if your investigation is complete.")
		}
		return hint.String(), true
	}

	// Budget exhausted.
	if depthBudget == 3 {
		return "Final check. If your investigation is complete, you may stop. " +
			"The synthesis step will produce the final answer from your evidence.", true
	}
	return "", false
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

	// HasEnoughFacts: check file coverage — have we read a reasonable
	// fraction of the files discovered in Phase 1? Also require at
	// least 2 distinct evidence tool types (grep + read_file).
	discovered, readSet := extractFileCoverage(toolResults)
	coverage := 0.0
	if len(discovered) > 0 {
		coverage = float64(len(readSet)) / float64(len(discovered))
	}
	hasEnough := len(sources) >= 2 && (coverage >= 0.5 || len(readSet) >= 3)

	signals := &types.ExecutionSignals{HasEnoughFacts: hasEnough}

	out := &StageOutput{
		Data:          json.RawMessage(fmt.Sprintf(`{"result": %q}`, lastContent)),
		NewFacts:      facts,
		SignalUpdates: signals,
	}

	if !signals.HasEnoughFacts {
		if len(sources) < 2 {
			out.RetryHint = "Previous attempt used fewer than 2 distinct evidence tool types. Use both grep and read_file."
		} else {
			out.RetryHint = fmt.Sprintf("Previous attempt read only %d of %d discovered relevant files (%.0f%% coverage). Read more of the discovered files.", len(readSet), len(discovered), coverage*100)
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

// extractFileCoverage analyzes tool history to determine which files
// were discovered (via grep files_only) and which were actually read
// (via read_file). Returns:
//   - discovered: relevant source files from grep results (filtered to
//     exclude noise like logs, binary, .git, test files)
//   - readSet: set of file paths that were read via read_file
//
// File path extraction is format-agnostic: it parses grep's one-path-
// per-line output and read_file's "[path: ...]" summary banner. No
// assumptions about language or project structure.
func extractFileCoverage(history []types.ToolResult) (discovered []string, readSet map[string]bool) {
	readSet = make(map[string]bool)
	discoveredSet := make(map[string]bool)

	for _, r := range history {
		if !r.Success {
			continue
		}
		switch r.ToolName {
		case "grep":
			// grep files_only returns one path per line.
			for _, line := range strings.Split(r.Summary, "\n") {
				path := strings.TrimSpace(line)
				if path == "" {
					continue
				}
				// Normalize: strip leading ./
				path = strings.TrimPrefix(path, "./")
				// Filter noise: skip non-source files.
				if isNoisePath(path) {
					continue
				}
				if !discoveredSet[path] {
					discoveredSet[path] = true
					discovered = append(discovered, path)
				}
			}
		case "read_file":
			// read_file summary starts with "[path: ...]" banner.
			first := strings.SplitN(r.Summary, "\n", 2)[0]
			if strings.HasPrefix(first, "[") {
				// Extract path from "[path: showing ...]"
				if idx := strings.Index(first, ":"); idx > 1 {
					path := strings.TrimSpace(first[1:idx])
					readSet[path] = true
				}
			}
		}
	}
	return
}

// isNoisePath returns true for paths that should be excluded from the
// discovered-files list: binary outputs, logs, VCS metadata, test
// files, and documentation investigation notes. The checks are based
// on path patterns, not file extensions, so they work across languages.
func isNoisePath(path string) bool {
	// No extension + no directory = likely a binary output
	if !strings.Contains(path, ".") && !strings.Contains(path, "/") {
		return true
	}
	// Dot-prefixed paths: VCS (.git/), hidden dirs (.cache/), dotfiles
	if strings.HasPrefix(path, ".") {
		return true
	}
	// Common dependency/vendor directories (cross-ecosystem)
	for _, dir := range []string{"node_modules/", "vendor/", "__pycache__/", ".tox/", "target/debug/", "target/release/"} {
		if strings.HasPrefix(path, dir) || strings.Contains(path, "/"+dir) {
			return true
		}
	}
	// Test files (cross-language naming conventions)
	base := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		base = path[idx+1:]
	}
	if strings.HasSuffix(base, "_test.go") || strings.HasPrefix(base, "test_") ||
		strings.HasSuffix(base, ".test.js") || strings.HasSuffix(base, ".test.ts") ||
		strings.HasSuffix(base, "_test.py") || strings.HasSuffix(base, ".spec.js") ||
		strings.HasSuffix(base, ".spec.ts") || strings.HasSuffix(base, "_spec.rb") {
		return true
	}
	// Log files
	if strings.HasSuffix(base, ".log") {
		return true
	}
	return false
}

// NewExplorerAgent creates the explorer agent (used in explore stage).
func NewExplorerAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentExplorer, deps, &explorerEvaluator{tools: deps.Tools})
}
