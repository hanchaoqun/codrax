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
	tools              *tool.Registry
	phase              int // 0 = breadth scan, 1 = depth read
	broadenAttempts    int // times we pushed for broader grep in Phase 0
	idleStreakInDepth   int // consecutive no-tool-call rounds in Phase 2
	lastToolResultCount int // tool result count at last continuation check
	preScannedFiles    []string // top files from keyword search, for coverage tracking
	investigationNotes []string // assistant analysis messages from ReAct loop
	userQuestion       string   // original user question, for focus alignment
}

func (e *explorerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	e.phase = 0 // start in breadth-scan phase
	e.userQuestion = ctx.CurrentTask

	var b strings.Builder
	b.WriteString("## Phase 1: Breadth Scan\n\n")
	b.WriteString("Your goal in this phase is to MAP the relevant territory — find ALL files related to the question. ")
	b.WriteString("Do NOT read files in full yet. Use lightweight tools:\n")
	b.WriteString("- repo_map (task_map view) to get an overview of relevant files\n")
	b.WriteString("- grep with files_only=true to find WHICH FILES contain key terms (just filenames, not lines). Do not use --include so you discover all file types\n")
	b.WriteString("- list_files to understand directory structure\n\n")

	if len(ctx.CurrentTaskKeywords) > 0 {
		// Run graduated keyword search before Phase 1 starts.
		// This gives the LLM a pre-ranked file list instead of
		// making it guess which grep patterns to use.
		results := keywordSearch(ctx.CurrentTaskKeywords, ctx.RepoRoot)
		if len(results) > 0 {
			b.WriteString(formatKeywordResults(results))
			// Save top files for coverage tracking in Phase 2.
			// The continuation prompt will flag these as unread if
			// the LLM skips them.
			top := 10
			if top > len(results) {
				top = len(results)
			}
			for _, r := range results[:top] {
				e.preScannedFiles = append(e.preScannedFiles, r.Path)
			}
		} else {
			// No hits at any level — list the keywords so the LLM
			// can try its own grep strategies.
			b.WriteString("### Search Keywords (no pre-scan hits)\n\n")
			b.WriteString("The analyzer provided these keywords but none matched. Try broader patterns:\n")
			for _, kw := range ctx.CurrentTaskKeywords {
				fmt.Fprintf(&b, "- `%s`\n", kw)
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("At the end of this phase, produce a FILE LIST of 3-6 files to read in depth. ")
	b.WriteString("For each file, note its ROLE and what you expect to learn from it.\n\n")
	b.WriteString("Strategy:\n")
	b.WriteString("- Search broadly: grep the core keyword without filtering by file type\n")
	b.WriteString("- Classify each discovered file by role: (a) defines types/structures, (b) implements core logic, (c) declares configuration/topology/rules, (d) loads/parses configuration, (e) entry point. Prioritize roles a-d over e\n")
	b.WriteString("- Exclude: test files, utility/infrastructure files (logging, tool wrappers), generated code\n")
	b.WriteString("- Files that DECLARE rules or topology are as important as files that IMPLEMENT logic — include both in your list")

	return b.String()
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
	// Capture assistant analysis messages from the ReAct loop.
	// These contain the LLM's processed understanding of the files
	// it read — essential for synthesis, where raw files get truncated.
	if resp.Content != "" && e.phase == 1 {
		e.investigationNotes = append(e.investigationNotes, resp.Content)
	}

	if e.phase == 0 {
		// Before transitioning to Phase 2, check if Phase 1 actually
		// discovered any files. If all greps returned zero results,
		// push the LLM to retry with broader patterns before moving on.
		discovered, _ := extractFileCoverage(history)
		if len(discovered) == 0 && e.broadenAttempts < 2 {
			e.broadenAttempts++
			return "Your grep searches returned no file matches. Before moving to depth reading, " +
				"try broader search strategies:\n" +
				"- Drop any --include filter (search ALL file types)\n" +
				"- Use shorter or partial keywords (prefixes, stems) — e.g. instead of 'SubAgentRuntime' try 'SubAgent' or 'subagent'\n" +
				"- Use single common terms rather than compound phrases\n" +
				"- Try conceptual synonyms for the same idea\n\n" +
				"Run at least 2-3 new grep calls with files_only=true before producing your file list.", true
		}

		// Phase 0 → Phase 1 transition: the LLM produced a breadth
		// scan summary. Now switch to depth reading.
		e.phase = 1
		return "## Phase 2: Depth Read\n\n" +
			"Good — you have mapped the relevant territory. Now switch to deep investigation.\n\n" +
			"**User question: " + e.userQuestion + "**\n\n" +
			"Read the key source files you identified. For EACH file you read, write a brief analysis:\n" +
			"1. What does this file reveal about the user's question?\n" +
			"2. What specific code (function name, line) is the evidence?\n" +
			"3. Does this file reference another file that you need to read?\n\n" +
			"**Important:** Read ONE file at a time, then analyze it before reading the next. " +
			"Do not read multiple large files in one batch — you will lose detail. " +
			"Small files (<100 lines) can be read in the same batch.\n\n" +
			"Trace through conditionals: when you find 'only if X matches Y', identify what X and Y are concretely. " +
			"Do NOT just catalog structures — answer the question.\n\n" +
			"Start by reading the most important file now.", true
	}

	// Phase 1 (depth read): use runtime file coverage as guidance.
	// Merge grep-discovered files with pre-scanned top files so the
	// coverage check catches high-scoring files the LLM didn't grep.
	discovered, readSet := extractFileCoverage(history)
	// Inject pre-scanned files that aren't already in discovered.
	seen := make(map[string]bool, len(discovered))
	for _, f := range discovered {
		seen[f] = true
	}
	for _, f := range e.preScannedFiles {
		if !seen[f] && !isNoisePath(f) {
			discovered = append(discovered, f)
			seen[f] = true
		}
	}
	var unread []string
	for _, f := range discovered {
		if !readSet[f] {
			unread = append(unread, f)
		}
	}

	// Track consecutive no-tool-call rounds in Phase 2.
	if len(history) > e.lastToolResultCount {
		e.idleStreakInDepth = 0
	}
	e.lastToolResultCount = len(history)
	e.idleStreakInDepth++

	// Check which pre-scanned high-priority files are still unread.
	var preScannedUnread []string
	for _, f := range e.preScannedFiles {
		if !readSet[f] && !isNoisePath(f) {
			preScannedUnread = append(preScannedUnread, f)
		}
	}

	// Check which read files were NOT analyzed in investigation notes.
	var unanalyzed []string
	notesJoined := strings.Join(e.investigationNotes, "\n")
	for f := range readSet {
		// Check if the file was mentioned in any analysis note.
		base := f
		if idx := strings.LastIndex(f, "/"); idx >= 0 {
			base = f[idx+1:]
		}
		if !strings.Contains(notesJoined, base) {
			unanalyzed = append(unanalyzed, f)
		}
	}

	// If there are unread high-priority files, force reading.
	if len(preScannedUnread) > 0 {
		e.idleStreakInDepth = 0
		var hint strings.Builder
		fmt.Fprintf(&hint,
			"File coverage: %d read out of %d discovered.\n", len(readSet), len(discovered))
		fmt.Fprintf(&hint, "\nReminder — user question: %s\n\n", e.userQuestion)
		hint.WriteString("The following HIGH-PRIORITY files from the pre-scan ranking have NOT been read yet. ")
		hint.WriteString("Read them now — they scored highly for keyword + structural relevance:\n")
		for _, f := range preScannedUnread {
			hint.WriteString("- " + f + "\n")
		}
		hint.WriteString("\nRead the most important unread file now.")
		return hint.String(), true
	}

	// If files were read but not analyzed, push for analysis.
	if len(unanalyzed) > 0 {
		e.idleStreakInDepth = 0
		var hint strings.Builder
		fmt.Fprintf(&hint, "Reminder — user question: %s\n\n", e.userQuestion)
		hint.WriteString("You read these files but did NOT analyze them in relation to the question:\n")
		for _, f := range unanalyzed {
			hint.WriteString("- " + f + "\n")
		}
		hint.WriteString("\nFor each file above, write what it reveals about the user's question. " +
			"Do not skip files — the answer may depend on connecting evidence across multiple files.")
		return hint.String(), true
	}

	// No unread high-priority files. Apply idle streak detection.
	if e.idleStreakInDepth >= 2 {
		return "", false
	}

	// Show remaining coverage for grep-discovered files.
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
	hasEvidence := false
	for _, r := range toolResults {
		if r.Success && e.toolConfidence(r.ToolName) > 0.5 {
			hasEvidence = true
			break
		}
	}
	if !hasEvidence {
		return "", false
	}

	var digest strings.Builder
	digest.WriteString("You have completed your investigation. Below is your analysis from the investigation phase.\n\n")
	digest.WriteString("## User Question\n")
	digest.WriteString(ctx.CurrentTask)
	digest.WriteString("\n\n")

	// Include the LLM's own intermediate analysis from the ReAct loop.
	// These are the assistant messages where it summarized each file's
	// key findings — much more valuable than truncated raw file contents,
	// because the LLM already processed the full files and extracted
	// the relevant details.
	if len(e.investigationNotes) > 0 {
		digest.WriteString("## Your Investigation Notes\n\n")
		digest.WriteString("These are YOUR OWN analyses from reading the source files during investigation:\n\n")
		for i, note := range e.investigationNotes {
			if len(note) > 2000 {
				note = note[:2000] + "\n... [truncated]"
			}
			fmt.Fprintf(&digest, "### Analysis %d\n%s\n\n", i+1, note)
		}
	}

	// Include a compact file list (not full contents) so the LLM
	// knows which files it read.
	digest.WriteString("## Files Read\n\n")
	for _, r := range toolResults {
		if r.Success && r.ToolName == "read_file" {
			first := strings.SplitN(r.Summary, "\n", 2)[0]
			digest.WriteString("- " + first + "\n")
		}
	}
	digest.WriteString("\n")

	digest.WriteString("## Instructions\n\n")
	digest.WriteString("Based on your investigation notes above, write a comprehensive answer to the user's question. Requirements:\n")
	digest.WriteString("- Synthesize from your analysis, do not just list files or component names\n")
	digest.WriteString("- For architecture/flow questions: explain the mechanism, conditions, and branching logic\n")
	digest.WriteString("- Trace through the code to identify SPECIFIC answers (e.g. which exact agent, not 'any agent that meets conditions')\n")
	digest.WriteString("- Ground every key claim in a file:line citation\n")
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
	for _, dir := range []string{
		"node_modules/", "vendor/", "__pycache__/", ".tox/",
		"target/debug/", "target/release/",
	} {
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
