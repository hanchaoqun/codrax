package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
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
	preScannedFiles    []string            // top files from keyword search, for coverage tracking
	allScoredFiles     []string            // ALL files from keyword search (not just top 8), for supplementary evidence
	fileSymbols        map[string][]string // path → symbol summaries from repo_map
	searchResult       *keywordSearchResult // full search result for cross-reference lookups
	investigationNotes []string            // assistant analysis messages from ReAct loop
	userQuestion       string              // original user question, for focus alignment
	preScannedPushCount int  // times we pushed for unread pre-scanned files without progress
	lastPreScannedUnreadCount int // count of unread pre-scanned files at last push
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
		sr := keywordSearch(ctx.CurrentTaskKeywords, ctx.RepoRoot)
		e.searchResult = sr
		results := sr.Files
		if len(results) > 0 {
			b.WriteString(formatKeywordResults(results))
			// Save files with repo_map structural relevance for coverage
			// tracking in Phase 2, along with their symbol tables.
			// Sort by repo_map score (structural importance) rather than
			// combined score, so structurally important files like
			// subagent.go (high repo_map, low grep) aren't crowded out.
			// Cap at 8 files to stay within iteration budget.
			type coverageCandidate struct {
				path         string
				repoMapScore float64
				symbols      []string
			}
			var candidates []coverageCandidate
			for _, r := range results {
				if r.RepoMapScore > 0 {
					candidates = append(candidates, coverageCandidate{
						path:         r.Path,
						repoMapScore: r.RepoMapScore,
						symbols:      r.Symbols,
					})
				}
			}
			sort.Slice(candidates, func(i, j int) bool {
				return candidates[i].repoMapScore > candidates[j].repoMapScore
			})
			e.fileSymbols = make(map[string][]string)
			for i, c := range candidates {
				e.allScoredFiles = append(e.allScoredFiles, c.path)
				if i < 8 {
					e.preScannedFiles = append(e.preScannedFiles, c.path)
				}
				if len(c.symbols) > 0 {
					e.fileSymbols[c.path] = c.symbols
				}
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
		// Cross-reference tracking: scan the note for symbol names
		// that are defined in other files. If the LLM mentions
		// "NewSubExplorer" and repo_map knows it's defined in
		// sub_explorer.go, add that file to coverage tracking.
		e.trackCrossReferences(resp.Content)
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
		// scan summary. Now switch to depth reading with evidence
		// catalog mode: collect ALL facts, defer reasoning to synthesis.
		e.phase = 1
		return "## Phase 2: Evidence Collection\n\n" +
			"Good — you have mapped the relevant territory. Now switch to deep evidence collection.\n\n" +
			"**User question: " + e.userQuestion + "**\n\n" +
			"**Your job is to collect evidence, NOT to answer the question.** " +
			"Do not form hypotheses or draw conclusions during this phase. " +
			"Reasoning happens later in synthesis — right now, be a thorough investigator.\n\n" +
			"Read the key source files you identified. After EACH file, extract ALL relevant facts as structured evidence:\n\n" +
			"```\n" +
			"## Evidence from [filename]\n" +
			"- [DIRECT] `functionName` line N: <what this code establishes>\n" +
			"- [CONDITIONAL] `functionName` line N: <what happens> IF <condition>\n" +
			"- [REGISTRATION] `functionName` line N: <what is registered/configured, with EXACT values>\n" +
			"- [MECHANISM] `functionName` line N: <how something works>\n" +
			"- [RELATIONSHIP] `symbolA` → `symbolB`: <nature of the link>\n" +
			"```\n\n" +
			"**Rules:**\n" +
			"- Extract EVERY fact that MIGHT be relevant, even if you're unsure — err on the side of over-collecting\n" +
			"- For [REGISTRATION] entries: always note the EXACT concrete values (which specific items are registered, what strings are returned). " +
			"If a function registers exactly 1 item, say 'registers ONLY X' — 'including X' is ambiguous and insufficient\n" +
			"- For [CONDITIONAL] entries: note the exact condition — do NOT summarize conditions as 'when configured' or 'if applicable'\n" +
			"- **NEVER skip simple methods.** Short methods like `Name() string { return \"x\" }` or `Type() int { return 3 }` are CRITICAL " +
			"because they establish concrete values that resolve conditions. Always record them as [REGISTRATION] with the exact return value\n" +
			"- For interface implementations: note WHICH concrete type implements WHICH interface, and what each method returns\n" +
			"- Read function BODIES, not just signatures — the specific values, registrations, and return values inside bodies are critical evidence\n" +
			"- Read ONE file at a time\n\n" +
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

	// Check which read files have UNanalyzed symbols — symbols that
	// the LLM didn't mention in its investigation notes. This catches
	// the case where the LLM read a file but only analyzed the first
	// few type definitions, skipping key functions at the end.
	//
	// We use different thresholds: short names (3+ chars) for methods
	// and functions (which often return critical values like Name()),
	// and longer names (8+ chars) for types/constants (to avoid noise
	// from generic names like "New", "Run").
	type unanalyzedFile struct {
		path           string
		missedSymbols  []string
	}
	var unanalyzed []unanalyzedFile
	notesJoined := strings.Join(e.investigationNotes, "\n")
	for f := range readSet {
		syms := e.fileSymbols[f]
		if len(syms) == 0 {
			continue
		}
		var missed []string
		for _, sym := range syms {
			// Extract symbol name and kind from "Name kind:line" format.
			name := sym
			kind := ""
			if idx := strings.Index(sym, " "); idx > 0 {
				name = sym[:idx]
				rest := sym[idx+1:]
				if kidx := strings.Index(rest, ":"); kidx > 0 {
					kind = rest[:kidx]
				}
			}
			// Methods and functions: 3+ chars (catches Name, Run, Get).
			// Other symbols: 8+ chars (avoids noise from generic names).
			minLen := 8
			if kind == "method" || kind == "function" {
				minLen = 3
			}
			if len(name) >= minLen && !strings.Contains(notesJoined, name) {
				missed = append(missed, sym)
			}
		}
		if len(missed) > 0 {
			unanalyzed = append(unanalyzed, unanalyzedFile{path: f, missedSymbols: missed})
		}
	}

	// If there are unread high-priority files, push for reading.
	// Track push attempts: if the LLM ignores file-reading requests
	// repeatedly (same unread count), escalate then give up.
	if len(preScannedUnread) > 0 {
		// Check whether the LLM made progress since the last push.
		if len(preScannedUnread) >= e.lastPreScannedUnreadCount && e.lastPreScannedUnreadCount > 0 {
			e.preScannedPushCount++
		} else {
			e.preScannedPushCount = 1
		}
		e.lastPreScannedUnreadCount = len(preScannedUnread)

		// After 3 failed pushes, stop resetting idle streak so the
		// loop terminates naturally. The LLM is clearly not going to
		// read these files — wasting more rounds won't help.
		if e.preScannedPushCount <= 3 {
			e.idleStreakInDepth = 0
		}

		var hint strings.Builder
		fmt.Fprintf(&hint,
			"File coverage: %d read out of %d discovered.\n", len(readSet), len(discovered))
		fmt.Fprintf(&hint, "\nReminder — user question: %s\n\n", e.userQuestion)

		if e.preScannedPushCount >= 3 {
			// Final forceful push: name the single most important file.
			fmt.Fprintf(&hint, "STOP ANALYZING. You have NOT read %d critical files. Call read_file on this file RIGHT NOW:\n", len(preScannedUnread))
			hint.WriteString("  " + preScannedUnread[0])
			if syms := e.fileSymbols[preScannedUnread[0]]; len(syms) > 0 {
				hint.WriteString(" — defines: " + strings.Join(syms, "; "))
			}
			hint.WriteString("\n\nDo NOT write any analysis. Your ONLY action should be a read_file tool call.")
		} else if e.preScannedPushCount >= 2 {
			// Escalated push: more forceful language.
			hint.WriteString("You keep writing analysis without reading the critical files. STOP and call read_file.\n\n")
			hint.WriteString("Unread HIGH-PRIORITY files:\n")
			for _, f := range preScannedUnread {
				hint.WriteString("- " + f)
				if syms := e.fileSymbols[f]; len(syms) > 0 {
					hint.WriteString(" — defines: " + strings.Join(syms, "; "))
				}
				hint.WriteString("\n")
			}
			hint.WriteString("\nCall read_file on the most important one. Do NOT respond with analysis text — use the tool.")
		} else {
			// First push: gentle.
			hint.WriteString("The following HIGH-PRIORITY files have NOT been read yet:\n")
			for _, f := range preScannedUnread {
				hint.WriteString("- " + f)
				if syms := e.fileSymbols[f]; len(syms) > 0 {
					hint.WriteString(" — defines: ")
					hint.WriteString(strings.Join(syms, "; "))
				}
				hint.WriteString("\n")
			}
			hint.WriteString("\nRead the most important unread file and extract ALL evidence entries from it. " +
				"Remember: collect facts, do not answer the question yet.")
		}
		return hint.String(), true
	}

	// If files were read but have symbols the LLM didn't analyze,
	// push for analysis of those specific symbols.
	if len(unanalyzed) > 0 {
		e.idleStreakInDepth = 0
		var hint strings.Builder
		fmt.Fprintf(&hint, "Reminder — user question: %s\n\n", e.userQuestion)
		hint.WriteString("You read these files but SKIPPED some symbols that may be relevant:\n\n")
		for _, ua := range unanalyzed {
			fmt.Fprintf(&hint, "**%s** — missed symbols:\n", ua.path)
			for _, sym := range ua.missedSymbols {
				hint.WriteString("  - " + sym + "\n")
			}
		}
		hint.WriteString("\nFor each missed symbol, QUOTE its complete implementation (not just the signature). " +
			"Then extract evidence entries: what does this implementation register, configure, or establish? " +
			"Include [REGISTRATION] entries with EXACT values.")
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
	digest.WriteString("You have completed your evidence collection. Below is the evidence catalog from your investigation.\n\n")
	digest.WriteString("## User Question\n")
	digest.WriteString(ctx.CurrentTask)
	digest.WriteString("\n\n")

	// Include the LLM's evidence entries from the ReAct loop.
	// These contain structured facts extracted from each file.
	if len(e.investigationNotes) > 0 {
		digest.WriteString("## Evidence Catalog\n\n")
		digest.WriteString("These are the evidence entries YOU collected during investigation:\n\n")
		for i, note := range e.investigationNotes {
			if len(note) > 1200 {
				note = note[:1200] + "\n... [truncated]"
			}
			fmt.Fprintf(&digest, "### Evidence Set %d\n%s\n\n", i+1, note)
		}
	}

	// Build cross-reference map: identify symbols that appear in 2+
	// evidence sets. These are the links in the evidence chain.
	if crossRefs := e.buildCrossReferenceMap(); crossRefs != "" {
		digest.WriteString(crossRefs)
	}

	// Include a compact file list so the LLM knows what was read.
	_, readSet := extractFileCoverage(toolResults)

	// Inject concrete values extracted from short methods/functions
	// across all relevant files (pre-scanned, read, and scored).
	// Also builds resolution chains that trace through symbol references.
	if cv := e.buildConcreteValuesSection(ctx.RepoRoot, readSet); cv != "" {
		digest.WriteString(cv)
	}
	digest.WriteString("## Files Read\n\n")
	for _, r := range toolResults {
		if r.Success && r.ToolName == "read_file" {
			first := strings.SplitN(r.Summary, "\n", 2)[0]
			digest.WriteString("- " + first + "\n")
		}
	}
	digest.WriteString("\n")

	// Flag pre-scanned files that were never read.
	var unreadImportant []string
	for _, f := range e.preScannedFiles {
		if !readSet[f] && !isNoisePath(f) {
			unreadImportant = append(unreadImportant, f)
		}
	}
	if len(unreadImportant) > 0 {
		digest.WriteString("## WARNING: Unread Important Files\n\n")
		digest.WriteString("These structurally important files were identified but NEVER READ. ")
		digest.WriteString("Your answer may be incomplete:\n\n")
		for _, f := range unreadImportant {
			digest.WriteString("- " + f)
			if syms := e.fileSymbols[f]; len(syms) > 0 {
				digest.WriteString(" — defines: " + strings.Join(syms, "; "))
			}
			digest.WriteString("\n")
		}
		digest.WriteString("\n")
	}

	digest.WriteString("## Reasoning Instructions\n\n")
	digest.WriteString("Answer the user's question by following these steps:\n\n")
	digest.WriteString("**Step 1 — Read the Resolution Chains section above.** These chains have ALREADY traced through " +
		"the code to resolve conditional logic. Use them as given — they are programmatically verified.\n\n")
	digest.WriteString("**Step 2 — Read the Concrete Values table.** Each row is an EXACT fact from source code.\n\n")
	digest.WriteString("**Step 3 — Answer the question.** Requirements:\n")
	digest.WriteString("- Name SPECIFIC components, not categories\n")
	digest.WriteString("- Start from the resolution chains and concrete values to determine the specific answer\n")
	digest.WriteString("- Ground every key claim in a file:line citation\n")
	digest.WriteString("- If no resolution chain exists, fall back to your evidence catalog\n")

	return digest.String(), true
}

// buildCrossReferenceMap scans investigation notes for symbol names
// from the repo_map graph and identifies symbols that appear in 2+
// different notes. These "bridge entities" are the connective tissue
// for multi-hop reasoning — they tell the LLM which analyses to
// chain together.
func (e *explorerEvaluator) buildCrossReferenceMap() string {
	if e.searchResult == nil || e.searchResult.Graph == nil || len(e.investigationNotes) < 2 {
		return ""
	}

	// For each symbol in the graph, check which notes mention it.
	type symbolRef struct {
		name      string
		noteIdxs  []int // 0-based indices into investigationNotes
	}
	var bridges []symbolRef

	for symName := range e.searchResult.Graph.SymbolDefs {
		// Skip short/generic names that would produce noise.
		if len(symName) < 6 {
			continue
		}
		var mentioned []int
		for i, note := range e.investigationNotes {
			if strings.Contains(note, symName) {
				mentioned = append(mentioned, i)
			}
		}
		if len(mentioned) >= 2 {
			bridges = append(bridges, symbolRef{name: symName, noteIdxs: mentioned})
		}
	}

	if len(bridges) == 0 {
		return ""
	}

	// Sort bridges by number of notes they span (most connected first),
	// then alphabetically for stability.
	sort.Slice(bridges, func(i, j int) bool {
		if len(bridges[i].noteIdxs) != len(bridges[j].noteIdxs) {
			return len(bridges[i].noteIdxs) > len(bridges[j].noteIdxs)
		}
		return bridges[i].name < bridges[j].name
	})

	// Cap to avoid overwhelming the synthesis prompt.
	if len(bridges) > 15 {
		bridges = bridges[:15]
	}

	var b strings.Builder
	b.WriteString("## Cross-References Between Evidence Sets\n\n")
	b.WriteString("These symbols appear in MULTIPLE evidence sets — they are the links in your evidence chain. ")
	b.WriteString("Trace each chain to connect facts across files:\n\n")
	for _, br := range bridges {
		refs := make([]string, len(br.noteIdxs))
		for i, idx := range br.noteIdxs {
			refs[i] = fmt.Sprintf("Evidence Set %d", idx+1)
		}
		fmt.Fprintf(&b, "- **%s** → %s\n", br.name, strings.Join(refs, ", "))
	}
	b.WriteString("\n")
	return b.String()
}

// buildConcreteValuesSection scans all files from the keyword search
// and investigation for short methods/functions (≤3 lines), extracts
// concrete values (return values, registrations), and builds a table
// for the synthesis prompt. Unlike LLM-generated evidence, this is
// deterministic — it doesn't depend on which files the LLM chose to
// read or what it extracted.
//
// The function also builds resolution chains: when one concrete value
// references a symbol that has its own concrete value, the chain is
// made explicit (e.g., RegisterX registers NewFoo → Foo.Name returns "bar").
func (e *explorerEvaluator) buildConcreteValuesSection(repoRoot string, readSet map[string]bool) string {
	if e.searchResult == nil || e.searchResult.Graph == nil {
		return ""
	}
	graph := e.searchResult.Graph
	notesJoined := strings.Join(e.investigationNotes, "\n")

	type concreteValue struct {
		file     string
		receiver string
		method   string // qualified: Receiver.Name or Name
		kind     string // "returns", "registers ONLY", etc.
		value    string
		line     int
	}
	var allValues []concreteValue

	// Build the set of files to scan: all scored files + all read files +
	// files that define symbols mentioned in investigation notes.
	filesToScan := make(map[string]bool)
	for file := range readSet {
		filesToScan[file] = true
	}
	for _, f := range e.allScoredFiles {
		filesToScan[f] = true
	}
	for _, f := range e.preScannedFiles {
		filesToScan[f] = true
	}
	for symName, defs := range graph.SymbolDefs {
		if len(symName) >= 6 && strings.Contains(notesJoined, symName) {
			for _, def := range defs {
				filesToScan[def.File] = true
			}
		}
	}

	// Extract concrete values from short methods/functions (≤3 lines)
	// and from longer registration functions (any length, but only
	// extracting Register() calls from those).
	logging.Debug("[explorer] concrete values: scanning %d files", len(filesToScan))
	for file := range filesToScan {
		fi, ok := graph.FileIndex[file]
		if !ok {
			continue
		}
		for _, sym := range fi.Symbols {
			if sym.Kind != "method" && sym.Kind != "function" {
				continue
			}
			if sym.EndLine == 0 {
				continue
			}
			bodyLines := sym.EndLine - sym.Line
			isShort := bodyLines <= 3
			// For longer functions, only scan if the name suggests
			// registration (contains "Register", "Defaults", "Init").
			isRegistrationFunc := !isShort &&
				bodyLines <= 30 &&
				(strings.Contains(sym.Name, "Register") ||
					strings.Contains(sym.Name, "Defaults") ||
					strings.Contains(sym.Name, "register"))
			if !isShort && !isRegistrationFunc {
				continue
			}
			src := readSourceLines(filepath.Join(repoRoot, sym.File), sym.Line, sym.EndLine)
			if src == "" {
				continue
			}
			// Use Receiver (Go methods) or Parent (Java/Python/JS/Rust
			// methods inside classes) for the qualified name.
			owner := sym.Receiver
			if owner == "" {
				owner = sym.Parent
			}
			qualName := sym.Name
			if owner != "" {
				qualName = owner + "." + sym.Name
			}
			for _, cv := range extractConcreteValues(src) {
				// For longer functions, only keep registration values.
				if !isShort && !strings.Contains(cv.kind, "registers") {
					continue
				}
				allValues = append(allValues, concreteValue{
					file:     file,
					receiver: owner,
					method:   qualName,
					kind:     cv.kind,
					value:    cv.value,
					line:     sym.Line,
				})
			}
		}
	}

	logging.Debug("[explorer] concrete values: extracted %d total values", len(allValues))
	if len(allValues) == 0 {
		return ""
	}

	// Build pre-scanned set for filtering.
	preScannedSet := make(map[string]bool, len(e.preScannedFiles))
	for _, f := range e.preScannedFiles {
		preScannedSet[f] = true
	}

	// Filter to keep only values relevant to the investigation:
	// 1. Registrations — always kept
	// 2. Short string returns from pre-scanned/read files — always kept
	// 3. Short string returns from other files — only if receiver is in notes
	// 4. Values referencing symbols from the investigation notes
	var relevant []concreteValue
	for _, v := range allValues {
		if strings.Contains(v.kind, "registers") {
			relevant = append(relevant, v)
			continue
		}
		if v.kind == "returns" && len(v.value) >= 2 &&
			(v.value[0] == '"' || v.value[0] == '\'') {
			// Skip long description strings (> 80 chars).
			if len(v.value) > 80 {
				continue
			}
			// Always keep from pre-scanned or read files — these are
			// the files the system identified as most relevant.
			if readSet[v.file] || preScannedSet[v.file] {
				relevant = append(relevant, v)
				continue
			}
			// For other files, require receiver/method in notes.
			if strings.Contains(notesJoined, v.receiver) ||
				strings.Contains(notesJoined, v.method) {
				relevant = append(relevant, v)
				continue
			}
		}
		// Keep values referencing noted symbols
		for _, word := range strings.Fields(v.value) {
			cleaned := strings.Trim(word, "(){}[]&*,;")
			if len(cleaned) >= 6 && strings.Contains(notesJoined, cleaned) {
				relevant = append(relevant, v)
				break
			}
		}
	}

	logging.Debug("[explorer] concrete values: %d relevant (of %d total) after filtering", len(relevant), len(allValues))

	// Pass 2: trace references in relevant values to find more values.
	// E.g., RegisterDefaultSubAgents registers NewSubExplorer → add
	// SubExplorer.Name() which returns "explorer".
	seen := make(map[string]bool)
	for _, v := range relevant {
		seen[v.method] = true
	}
	for _, v := range relevant {
		// Scan value for symbol names that have their own concrete values.
		for _, av := range allValues {
			if seen[av.method] {
				continue
			}
			// Check if any word in v.value matches av's receiver type
			if av.receiver != "" && len(av.receiver) >= 6 &&
				strings.Contains(v.value, av.receiver) {
				logging.Debug("[explorer] concrete values pass2: %s references %s → adding %s", v.method, av.receiver, av.method)
				relevant = append(relevant, av)
				seen[av.method] = true
			}
		}
	}

	if len(relevant) == 0 {
		return ""
	}

	// Cap to prevent bloating.
	if len(relevant) > 15 {
		relevant = relevant[:15]
	}

	var b strings.Builder
	b.WriteString("## Concrete Values (programmatically extracted from source code)\n\n")
	b.WriteString("These are EXACT values from source code — ground truth, not summaries.\n\n")
	b.WriteString("| File:Line | Method | Fact |\n")
	b.WriteString("|-----------|--------|------|\n")
	for _, v := range relevant {
		fmt.Fprintf(&b, "| %s:%d | `%s()` | %s %s |\n",
			v.file, v.line, v.method, v.kind, v.value)
	}
	b.WriteString("\n")

	// Build resolution chains: when value A mentions type T, and
	// there's a value from T.SomeMethod, chain them. This covers:
	//   - Register(NewFoo) → Foo.Name() returns "bar"
	//   - returns NewFoo() → Foo.Name() returns "bar"
	//   - returns &Foo{} → Foo.Name() returns "bar"
	var chains []string
	for _, v := range relevant {
		// Skip values that don't reference other types.
		if v.kind != "returns" && !strings.Contains(v.kind, "registers") {
			continue
		}
		for _, rv := range relevant {
			if rv.receiver == "" || rv.receiver == v.receiver {
				continue
			}
			if strings.Contains(v.value, rv.receiver) {
				chains = append(chains, fmt.Sprintf(
					"`%s()` %s %s → `%s()` %s %s",
					v.method, v.kind, v.value,
					rv.method, rv.kind, rv.value))
			}
		}
	}
	// Deduplicate chains (same source can chain to multiple targets).
	if len(chains) > 10 {
		chains = chains[:10]
	}
	if len(chains) > 0 {
		b.WriteString("### Resolution Chains\n\n")
		b.WriteString("These chains trace through the concrete values to resolve conditions:\n\n")
		for _, c := range chains {
			b.WriteString("- " + c + "\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

// readSourceLines reads lines [startLine, endLine] (1-based, inclusive)
// from the given file path. Returns empty string on any error.
func readSourceLines(path string, startLine, endLine int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum >= startLine && lineNum <= endLine {
			lines = append(lines, scanner.Text())
		}
		if lineNum > endLine {
			break
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

// concreteValueEntry holds a single extracted concrete value from source code.
type concreteValueEntry struct {
	kind  string // "returns", "registers ONLY", "assigns"
	value string // the concrete value
}

// extractConcreteValues parses a short source code snippet for patterns
// that establish concrete values. These patterns are language-agnostic
// enough to work across Go, Python, Java, TypeScript, etc.
//
// Recognized patterns:
//   - return "literal" or 'literal' → returns "literal"
//   - return number             → returns number
//   - return true/false/nil     → returns true/false/nil
//   - x.Register(NewFoo(...))   → registers ONLY NewFoo
//   - return TypeName{...}      → returns TypeName{...}
func extractConcreteValues(source string) []concreteValueEntry {
	var results []concreteValueEntry
	lines := strings.Split(source, "\n")

	// Count non-blank, non-brace-only lines to detect "single-statement" bodies.
	var registerCalls []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Pattern: return "string literal"
		if strings.HasPrefix(trimmed, "return ") {
			rest := strings.TrimPrefix(trimmed, "return ")
			rest = strings.TrimRight(rest, ";") // for non-Go langs
			// String literal (double or single quotes)
			if len(rest) >= 2 &&
				((rest[0] == '"' && rest[len(rest)-1] == '"') ||
					(rest[0] == '\'' && rest[len(rest)-1] == '\'')) {
				results = append(results, concreteValueEntry{
					kind:  "returns",
					value: rest,
				})
				continue
			}
			// Boolean / nil / null / none
			lower := strings.ToLower(rest)
			if lower == "true" || lower == "false" || lower == "nil" ||
				lower == "null" || lower == "none" {
				results = append(results, concreteValueEntry{
					kind:  "returns",
					value: rest,
				})
				continue
			}
			// Number
			isNum := true
			for _, c := range rest {
				if !((c >= '0' && c <= '9') || c == '.' || c == '-') {
					isNum = false
					break
				}
			}
			if isNum && len(rest) > 0 {
				results = append(results, concreteValueEntry{
					kind:  "returns",
					value: rest,
				})
				continue
			}
			// Type literal: return Type{...} or return &Type{...}
			if strings.Contains(rest, "{") {
				results = append(results, concreteValueEntry{
					kind:  "returns",
					value: rest,
				})
				continue
			}
			// Simple expression: return string(x), return x
			if !strings.Contains(rest, "\n") && len(rest) < 40 {
				results = append(results, concreteValueEntry{
					kind:  "returns",
					value: rest,
				})
			}
		}

		// Pattern: .Register(NewFoo(...)) or Register(NewFoo(...))
		if strings.Contains(trimmed, "Register(") || strings.Contains(trimmed, "register(") {
			// Extract the argument to Register()
			idx := strings.Index(trimmed, "Register(")
			if idx < 0 {
				idx = strings.Index(trimmed, "register(")
			}
			if idx >= 0 {
				arg := trimmed[idx+len("Register("):]
				// Find matching paren
				depth := 1
				end := 0
				for i, c := range arg {
					if c == '(' {
						depth++
					} else if c == ')' {
						depth--
						if depth == 0 {
							end = i
							break
						}
					}
				}
				if end > 0 {
					registerCalls = append(registerCalls, arg[:end])
				}
			}
		}
	}

	// If there are register calls, summarize them
	if len(registerCalls) > 0 {
		qualifier := "registers ONLY"
		if len(registerCalls) > 1 {
			qualifier = "registers"
		}
		results = append(results, concreteValueEntry{
			kind:  qualifier,
			value: strings.Join(registerCalls, ", "),
		})
	}

	return results
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

// trackCrossReferences scans an investigation note for symbol names
// that are defined in files not yet in the coverage list. When the
// LLM mentions e.g. "NewSubExplorer" in its analysis, this method
// looks up where that symbol is defined (sub_explorer.go) and adds
// that file to preScannedFiles so the coverage prompt ensures it
// gets read.
func (e *explorerEvaluator) trackCrossReferences(note string) {
	if e.searchResult == nil || e.searchResult.Graph == nil {
		return
	}
	graph := e.searchResult.Graph

	// Build set of already-tracked files.
	tracked := make(map[string]bool, len(e.preScannedFiles))
	for _, f := range e.preScannedFiles {
		tracked[f] = true
	}

	// Check each symbol definition in the graph.
	// Only track specific symbols (8+ chars, not common names) to
	// avoid noise from generic names like "New", "Run", "Execute".
	for symName, defs := range graph.SymbolDefs {
		if len(symName) < 8 {
			continue
		}
		// Only exported symbols (starts with uppercase in Go).
		if len(symName) > 0 && symName[0] >= 'a' && symName[0] <= 'z' {
			continue
		}
		// Skip overly common symbol names that appear in many files.
		if len(defs) > 3 {
			continue
		}
		if !strings.Contains(note, symName) {
			continue
		}
		// The note mentions this symbol. Add its defining file(s)
		// to coverage if not already tracked.
		for _, def := range defs {
			if def.File != "" && !tracked[def.File] && !isNoisePath(def.File) {
				e.preScannedFiles = append(e.preScannedFiles, def.File)
				tracked[def.File] = true
				// Also store symbols for the continuation prompt.
				if fi, ok := graph.FileIndex[def.File]; ok && e.fileSymbols != nil {
					var syms []string
					for _, s := range fi.Symbols {
						if s.Exported || s.Kind == "function" || s.Kind == "method" {
							syms = append(syms, fmt.Sprintf("%s %s:%d", s.Name, s.Kind, s.Line))
						}
					}
					e.fileSymbols[def.File] = syms
				}
				logging.Debug("[explorer] cross-ref: note mentions %q → added %s to coverage", symName, def.File)
			}
		}
	}
}

// NewExplorerAgent creates the explorer agent (used in explore stage).
func NewExplorerAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentExplorer, deps, &explorerEvaluator{tools: deps.Tools})
}
