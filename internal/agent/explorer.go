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
	"gopkg.in/yaml.v3"
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
	repoRoot           string              // repository root path, cached from BuildInitialPrompt
	preScannedPushCount int  // times we pushed for unread pre-scanned files without progress
	lastPreScannedUnreadCount int // count of unread pre-scanned files at last push
}

func (e *explorerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	e.phase = 0 // start in breadth-scan phase
	e.userQuestion = ctx.CurrentTask
	e.repoRoot = ctx.RepoRoot

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
				"- Use shorter or partial keywords (prefixes, stems) — e.g. instead of 'UserAuthenticationService' try 'UserAuth' or 'authentication'\n" +
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
			"- **NEVER skip simple methods/functions.** Short ones like `getName() { return \"x\" }` or `isEnabled() { return true }` are CRITICAL " +
			"because they establish concrete values that resolve conditions. Always record them as [REGISTRATION] with the exact return value\n" +
			"- **Negative evidence matters.** If you expected to find a pattern/method/registration but it is ABSENT, record:\n" +
			"  `- [ABSENT] Expected <what> in <where> but NOT found`\n" +
			"  This is critical for exclusion reasoning (e.g., \"class X does NOT implement method Y because it is absent from the source\")\n" +
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

	// When the LLM is slowing down (idle ≥ 1), inject a preview of
	// programmatically extracted concrete values. This breaks the
	// information asymmetry between collection and synthesis phases:
	// the LLM can see what the programmatic layer already knows and
	// focus its remaining reads on gaps only it can fill (semantic
	// relationships, complex conditions, cross-file reasoning).
	var cvPreview string
	if e.idleStreakInDepth >= 1 && len(e.investigationNotes) >= 2 && e.searchResult != nil {
		cvPreview = e.buildConcreteValuesSection(e.repoRoot, readSet)
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

	// Inject concrete values preview so the LLM knows what the
	// programmatic layer can already extract and focuses its remaining
	// investigation on gaps: semantic relationships, complex conditions,
	// multi-hop reasoning that only the LLM can do.
	if cvPreview != "" {
		hint.WriteString("\n\n---\n## Programmatic Evidence Preview\n\n")
		hint.WriteString("The system has ALREADY extracted the following concrete values from source code. " +
			"You do NOT need to re-investigate these — they will be provided as ground truth in synthesis.\n\n")
		// Truncate to keep the continuation prompt from bloating.
		if len(cvPreview) > 1500 {
			cvPreview = cvPreview[:1500] + "\n... [preview truncated]\n"
		}
		hint.WriteString(cvPreview)
		hint.WriteString("\n**Focus your remaining investigation on:**\n")
		hint.WriteString("- Cross-file relationships that the table above does NOT show\n")
		hint.WriteString("- Conditions whose resolution requires reading function bodies\n")
		hint.WriteString("- Semantic intent behind registrations (WHY something is registered, not just WHAT)\n")
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

	// HasEnoughFacts: multi-dimensional quality check.
	// 1. Tool diversity: at least 2 distinct evidence tools (grep + read_file).
	// 2. File coverage: ≥50% of discovered files read, or ≥3 files.
	// 3. Evidence quality: count structured evidence tags in notes.
	//    Require at least 2 [DIRECT]/[REGISTRATION] entries (ground-truth facts).
	// 4. File relevance: weight read files by their keyword search rank.
	discovered, readSet := extractFileCoverage(toolResults)
	coverage := 0.0
	if len(discovered) > 0 {
		coverage = float64(len(readSet)) / float64(len(discovered))
	}

	// Count evidence tags in investigation notes.
	directCount := 0
	conditionalCount := 0
	for _, note := range e.investigationNotes {
		for _, line := range strings.Split(note, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- [DIRECT]") || strings.HasPrefix(trimmed, "- [REGISTRATION]") {
				directCount++
			} else if strings.HasPrefix(trimmed, "- [CONDITIONAL]") {
				conditionalCount++
			}
		}
	}

	// Compute relevance-weighted coverage: files in allScoredFiles
	// (top keyword-search hits) count more than random grep results.
	relevantRead := 0
	if len(e.allScoredFiles) > 0 {
		scoredSet := make(map[string]bool, len(e.allScoredFiles))
		for _, f := range e.allScoredFiles {
			scoredSet[f] = true
		}
		for f := range readSet {
			if scoredSet[f] {
				relevantRead++
			}
		}
	}

	toolDiversity := len(sources) >= 2
	fileCoverage := coverage >= 0.5 || len(readSet) >= 3
	evidenceQuality := directCount >= 2
	hasEnough := toolDiversity && fileCoverage && evidenceQuality

	signals := &types.ExecutionSignals{HasEnoughFacts: hasEnough}

	out := &StageOutput{
		Data:          json.RawMessage(fmt.Sprintf(`{"result": %q}`, lastContent)),
		NewFacts:      facts,
		SignalUpdates: signals,
	}

	if !signals.HasEnoughFacts {
		if !toolDiversity {
			out.RetryHint = "Previous attempt used fewer than 2 distinct evidence tool types. Use both grep and read_file."
		} else if !evidenceQuality {
			out.RetryHint = fmt.Sprintf("Previous attempt collected only %d [DIRECT]/[REGISTRATION] evidence entries (need ≥2). Read more files and extract structured evidence with [DIRECT], [REGISTRATION], [CONDITIONAL] tags.", directCount)
		} else {
			out.RetryHint = fmt.Sprintf("Previous attempt read only %d of %d discovered relevant files (%.0f%% coverage, %d relevant). Read more of the discovered files.", len(readSet), len(discovered), coverage*100, relevantRead)
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
	// Track what programmatic layers fired for adaptive instructions.
	hasConcreteValues := false
	cv := e.buildConcreteValuesSection(ctx.RepoRoot, readSet)
	if cv != "" {
		hasConcreteValues = true
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

	// Detect unresolved conditions: evidence entries tagged [CONDITIONAL]
	// whose IF clauses cannot be structurally matched against any method
	// in the Concrete Values table or Resolution Chains.
	if len(e.investigationNotes) > 0 && hasConcreteValues {
		unresolvedConditions := resolveConditions(e.investigationNotes, cv)
		if len(unresolvedConditions) > 0 {
			digest.WriteString("## Unresolved Conditions\n\n")
			digest.WriteString("These conditions from your evidence could NOT be resolved by the Concrete Values table. " +
				"Your answer should acknowledge this uncertainty:\n\n")
			for _, c := range unresolvedConditions {
				digest.WriteString(c + "\n")
			}
			digest.WriteString("\n")
		}
	}

	// Surface negative evidence: [ABSENT] entries that the LLM recorded
	// during investigation. These are critical for exclusion reasoning.
	if len(e.investigationNotes) > 0 {
		var absentEntries []string
		for _, note := range e.investigationNotes {
			for _, line := range strings.Split(note, "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "- [ABSENT]") {
					absentEntries = append(absentEntries, trimmed)
				}
			}
		}
		if len(absentEntries) > 0 {
			digest.WriteString("## Negative Evidence (Absent Patterns)\n\n")
			digest.WriteString("These patterns were EXPECTED but NOT FOUND during investigation. " +
				"Use them for exclusion reasoning:\n\n")
			for _, e := range absentEntries {
				digest.WriteString(e + "\n")
			}
			digest.WriteString("\n")
		}
	}

	// Adaptive reasoning instructions: guide the LLM based on which
	// programmatic layers actually produced output.
	digest.WriteString("## Reasoning Instructions\n\n")
	digest.WriteString("Answer the user's question by following these steps:\n\n")
	if hasConcreteValues {
		digest.WriteString("**Step 1 — Use programmatic evidence.** The Concrete Values table, " +
			"Resolution Chains, and Embedding Chains above are extracted directly from source code " +
			"and are ground truth. Start your reasoning from these.\n\n")
	}
	digest.WriteString("**Step 2 — Resolve conditions.** When your evidence says 'X happens if Y', " +
		"find the concrete value of Y. Do not stop at 'any component that satisfies the condition' — " +
		"trace through to identify WHICH specific components satisfy it right now.\n\n")
	digest.WriteString("**Step 3 — Answer the question.** Requirements:\n")
	digest.WriteString("- Name SPECIFIC components, not categories\n")
	digest.WriteString("- Ground every key claim in a file:line citation\n")
	digest.WriteString("- If a condition cannot be resolved (no concrete value found), say so explicitly " +
		"rather than guessing\n")

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
	graph := e.searchResult.Graph

	// For each symbol in the graph, check which notes mention it.
	type symbolRef struct {
		name      string
		noteIdxs  []int // 0-based indices into investigationNotes
		relKinds  []string // relation kinds connecting this symbol across files
	}
	bridgeMap := make(map[string]*symbolRef)

	for symName := range graph.SymbolDefs {
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
			bridgeMap[symName] = &symbolRef{name: symName, noteIdxs: mentioned}
		}
	}

	// Augment with relation graph: when a call/reference/type_usage
	// relation links a symbol mentioned in one note to a symbol
	// mentioned in a different note, that pair is a cross-reference
	// even if neither symbol individually spans 2+ notes.
	noteSymbolIndex := make(map[string]int) // symbol → note index (first mention)
	for i, note := range e.investigationNotes {
		for symName := range graph.SymbolDefs {
			if len(symName) < 6 {
				continue
			}
			if strings.Contains(note, symName) {
				if _, exists := noteSymbolIndex[symName]; !exists {
					noteSymbolIndex[symName] = i
				}
			}
		}
	}

	for _, fi := range graph.Files {
		for _, rel := range fi.Relations {
			if rel.Kind != "call" && rel.Kind != "reference" && rel.Kind != "type_usage" {
				continue
			}
			// Extract symbol names from relation endpoints (format: "file:Symbol" or "Symbol").
			fromSym := rel.From
			if idx := strings.LastIndex(fromSym, ":"); idx >= 0 {
				fromSym = fromSym[idx+1:]
			}
			toSym := rel.To
			if idx := strings.LastIndex(toSym, ":"); idx >= 0 {
				toSym = toSym[idx+1:]
			}
			if len(fromSym) < 6 || len(toSym) < 6 || fromSym == toSym {
				continue
			}
			fromNote, fromOK := noteSymbolIndex[fromSym]
			toNote, toOK := noteSymbolIndex[toSym]
			if !fromOK || !toOK || fromNote == toNote {
				continue
			}
			// Create a bridge named "FromSym → ToSym" with the relation kind.
			key := fromSym + "→" + toSym
			if br, ok := bridgeMap[key]; ok {
				// Add relation kind if not already present.
				hasKind := false
				for _, k := range br.relKinds {
					if k == rel.Kind {
						hasKind = true
						break
					}
				}
				if !hasKind {
					br.relKinds = append(br.relKinds, rel.Kind)
				}
			} else {
				noteIdxs := []int{fromNote, toNote}
				sort.Ints(noteIdxs)
				bridgeMap[key] = &symbolRef{
					name:     fromSym + " → " + toSym,
					noteIdxs: noteIdxs,
					relKinds: []string{rel.Kind},
				}
			}
		}
	}

	if len(bridgeMap) == 0 {
		return ""
	}

	var bridges []symbolRef
	for _, br := range bridgeMap {
		bridges = append(bridges, *br)
	}

	// Sort bridges by number of notes they span (most connected first),
	// then alphabetically for stability.
	sort.Slice(bridges, func(i, j int) bool {
		if len(bridges[i].noteIdxs) != len(bridges[j].noteIdxs) {
			return len(bridges[i].noteIdxs) > len(bridges[j].noteIdxs)
		}
		return bridges[i].name < bridges[j].name
	})

	// Adaptive cap: scale with investigation complexity.
	cap := 15
	if len(e.allScoredFiles) > 10 {
		cap = 20
	}
	if len(bridges) > cap {
		bridges = bridges[:cap]
	}

	var b strings.Builder
	b.WriteString("## Cross-References Between Evidence Sets\n\n")
	b.WriteString("These symbols appear in MULTIPLE evidence sets — they are the links in your evidence chain. ")
	b.WriteString("Trace each chain to connect facts across files:\n\n")
	for _, br := range bridges {
		// Deduplicate note indices.
		seen := make(map[int]bool)
		var unique []int
		for _, idx := range br.noteIdxs {
			if !seen[idx] {
				seen[idx] = true
				unique = append(unique, idx)
			}
		}
		refs := make([]string, len(unique))
		for i, idx := range unique {
			refs[i] = fmt.Sprintf("Evidence Set %d", idx+1)
		}
		entry := fmt.Sprintf("- **%s** → %s", br.name, strings.Join(refs, ", "))
		if len(br.relKinds) > 0 {
			entry += " (" + strings.Join(br.relKinds, ", ") + ")"
		}
		b.WriteString(entry + "\n")
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
// made explicit (e.g., RegisterX binds NewFoo → Foo.Name returns "bar").
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
		kind     string // "returns", "binds ONLY", etc.
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

	// Cache file contents to avoid re-opening the same file for each symbol.
	fileLines := make(map[string][]string)
	loadFileLines := func(absPath string) []string {
		if lines, ok := fileLines[absPath]; ok {
			return lines
		}
		f, err := os.Open(absPath)
		if err != nil {
			fileLines[absPath] = nil
			return nil
		}
		defer f.Close()
		var lines []string
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		fileLines[absPath] = lines
		return lines
	}
	getLinesRange := func(absPath string, startLine, endLine int) string {
		lines := loadFileLines(absPath)
		if lines == nil || startLine < 1 || endLine > len(lines) {
			return ""
		}
		return strings.Join(lines[startLine-1:endLine], "\n")
	}

	// Extract concrete values from source code functions. Three tiers:
	//
	// 1. Short methods (≤3 lines): full extraction of all patterns.
	// 2. Registration functions (≤30 lines, name contains Register/Config/...):
	//    full extraction but only bindings/maps kept.
	// 3. Medium functions (4-100 lines): local line scan — only lines
	//    containing return/map/register patterns are extracted with ±1
	//    line of context. This recovers concrete values from longer
	//    functions without reading them entirely.
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
			// registration, mapping, or configuration. Uses cross-language
			// verb patterns (case-insensitive where needed).
			nameLower := strings.ToLower(sym.Name)
			isRegistrationFunc := !isShort &&
				bodyLines <= 30 &&
				(strings.Contains(nameLower, "register") ||
					strings.Contains(nameLower, "defaults") ||
					strings.Contains(nameLower, "route") ||
					strings.Contains(nameLower, "handler") ||
					strings.Contains(nameLower, "config") ||
					strings.Contains(nameLower, "setup") ||
					strings.Contains(nameLower, "init") ||
					strings.Contains(nameLower, "bind") ||
					strings.Contains(nameLower, "subscribe") ||
					strings.Contains(nameLower, "provide") ||
					strings.Contains(nameLower, "module") ||
					strings.Contains(sym.Name, "Map"))
			// Medium functions: not short, not registration-named, but
			// ≤100 lines — scan specific lines for return/binding patterns.
			isMediumFunc := !isShort && !isRegistrationFunc && bodyLines <= 100

			if !isShort && !isRegistrationFunc && !isMediumFunc {
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

			if isMediumFunc {
				// Local line scan: extract only lines matching evidence
				// patterns (return, register, map entries) with ±1 context.
				absPath := filepath.Join(repoRoot, sym.File)
				allLines := loadFileLines(absPath)
				if allLines == nil {
					continue
				}
				start := sym.Line - 1
				end := sym.EndLine
				if start < 0 {
					start = 0
				}
				if end > len(allLines) {
					end = len(allLines)
				}
				for li := start; li < end; li++ {
					trimmed := strings.TrimSpace(allLines[li])
					if !isEvidenceLine(trimmed) {
						continue
					}
					// Grab ±1 line of context for the extractor.
					ctxStart := li
					if ctxStart > start {
						ctxStart--
					}
					ctxEnd := li + 2
					if ctxEnd > end {
						ctxEnd = end
					}
					snippet := strings.Join(allLines[ctxStart:ctxEnd], "\n")
					for _, cv := range extractConcreteValues(snippet) {
						allValues = append(allValues, concreteValue{
							file:     file,
							receiver: owner,
							method:   qualName,
							kind:     cv.kind,
							value:    cv.value,
							line:     li + 1,
						})
					}
				}
				continue
			}

			src := getLinesRange(filepath.Join(repoRoot, sym.File), sym.Line, sym.EndLine)
			if src == "" {
				continue
			}
			for _, cv := range extractConcreteValues(src) {
				// For longer functions, only keep binding/registration values.
				if !isShort && !strings.Contains(cv.kind, "binds") && cv.kind != "maps" {
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

	// Also scan config files (YAML/JSON) for key-value mappings.
	// These establish config-driven behavior: stage→agent, route→handler, etc.
	// Only scan config files that are in the filesToScan set (relevant
	// to the investigation).
	for file := range filesToScan {
		fi, ok := graph.FileIndex[file]
		if !ok {
			continue
		}
		isConfig := fi.IsSpecial ||
			strings.HasSuffix(file, ".yaml") || strings.HasSuffix(file, ".yml") ||
			strings.HasSuffix(file, ".json") || strings.HasSuffix(file, ".toml")
		if !isConfig {
			continue
		}
		absPath := filepath.Join(repoRoot, file)
		entries := extractConfigValues(absPath, notesJoined)
		if len(entries) > 0 {
			if len(entries) > 10 {
				entries = entries[:10]
			}
			allValues = append(allValues, concreteValue{
				file:   file,
				method: filepath.Base(file),
				kind:   "config",
				value:  strings.Join(entries, "; "),
				line:   1,
			})
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
		if strings.Contains(v.kind, "binds") || v.kind == "maps" || v.kind == "config" || v.kind == "decorates" {
			relevant = append(relevant, v)
			continue
		}
		if v.kind == "returns" {
			isStringLit := len(v.value) >= 2 && (v.value[0] == '"' || v.value[0] == '\'')
			isBoolOrNil := v.value == "true" || v.value == "false" || v.value == "nil" || v.value == "null"
			// Skip long description strings (> 80 chars).
			if isStringLit && len(v.value) > 80 {
				continue
			}
			// Always keep short string/bool returns from pre-scanned or
			// read files — these are the most relevant files.
			if (isStringLit || isBoolOrNil) && (readSet[v.file] || preScannedSet[v.file]) {
				relevant = append(relevant, v)
				continue
			}
			// For other files, require receiver/method in notes.
			if (isStringLit || isBoolOrNil) &&
				(strings.Contains(notesJoined, v.receiver) ||
					strings.Contains(notesJoined, v.method)) {
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

	// Multi-pass reference tracing: follow type references in values
	// to discover more concrete values. Repeats until no new values
	// are found, supporting chains of arbitrary depth:
	//   RegisterX binds NewFoo → Foo returns NewBar → Bar.Name returns "baz"
	// Capped at 5 iterations to prevent runaway in circular references.
	seen := make(map[string]bool)
	for _, v := range relevant {
		seen[v.method] = true
	}
	for pass := 0; pass < 5; pass++ {
		added := 0
		for _, v := range relevant {
			for _, av := range allValues {
				if seen[av.method] {
					continue
				}
				if av.receiver != "" && len(av.receiver) >= 4 &&
					containsIdentifier(v.value, av.receiver) {
					relevant = append(relevant, av)
					seen[av.method] = true
					added++
				}
			}
		}
		if added == 0 {
			break
		}
	}

	if len(relevant) == 0 {
		return ""
	}

	// Sort by usefulness: bindings first (they anchor chains), then
	// short string returns (Name/Type), then booleans, then longer values.
	sort.Slice(relevant, func(i, j int) bool {
		scoreVal := func(v concreteValue) int {
			if strings.Contains(v.kind, "binds") || v.kind == "maps" || v.kind == "config" || v.kind == "decorates" {
				return 100
			}
			if v.kind == "returns" && len(v.value) <= 20 {
				return 80 // short Name/Type returns
			}
			if v.kind == "returns" && (v.value == "true" || v.value == "false") {
				return 60
			}
			return 10
		}
		return scoreVal(relevant[i]) > scoreVal(relevant[j])
	})
	// Adaptive cap: scale with investigation complexity.
	// More scored files = more complex investigation = need more values.
	valueCap := 15
	if len(e.allScoredFiles) > 10 {
		valueCap = 25
	}
	if valueCap > 40 {
		valueCap = 40
	}
	if len(relevant) > valueCap {
		relevant = relevant[:valueCap]
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
		if v.kind != "returns" && !strings.Contains(v.kind, "binds") && v.kind != "maps" && v.kind != "config" && v.kind != "decorates" {
			continue
		}
		for _, rv := range relevant {
			if rv.receiver == "" || rv.receiver == v.receiver {
				continue
			}
			if containsIdentifier(v.value, rv.receiver) {
				chains = append(chains, fmt.Sprintf(
					"`%s()` %s %s → `%s()` %s %s",
					v.method, v.kind, v.value,
					rv.method, rv.kind, rv.value))
			}
		}
	}
	// Adaptive cap for resolution chains.
	chainCap := 10
	if len(e.allScoredFiles) > 10 {
		chainCap = 18
	}
	if len(chains) > chainCap {
		chains = chains[:chainCap]
	}
	if len(chains) > 0 {
		b.WriteString("### Resolution Chains\n\n")
		b.WriteString("These chains trace through the concrete values to resolve conditions:\n\n")
		for _, c := range chains {
			b.WriteString("- " + c + "\n")
		}
		b.WriteString("\n")
	}

	// Build type hierarchy chains: when type A embeds/extends type B,
	// and B has a concrete value (e.g., ReadOnly.IsWrite() returns false),
	// then A inherits that value. Uses the graph's embedding and
	// inheritance relations extracted by tree-sitter.
	//
	// Covers:
	//   Go:     struct embedding (ReadOnly in ExecCommand)
	//   Go:     interface embedding (Reader in ReadCloser)
	//   Java:   extends, implements
	//   Python: class inheritance (superclasses)
	//   JS/TS:  extends
	//   Rust:   trait implementations
	var hierarchyChains []string
	// Collect all concrete values indexed by receiver for fast lookup.
	valuesByReceiver := make(map[string][]concreteValue)
	for _, v := range relevant {
		if v.receiver != "" {
			valuesByReceiver[v.receiver] = append(valuesByReceiver[v.receiver], v)
		}
	}

	// Build a parent→children map and collect all embedding/inheritance
	// relations across scanned files.
	type hierRelation struct {
		childType  string
		parentType string
		verb       string // "embeds" or "extends"
	}
	var allRelations []hierRelation
	for file := range filesToScan {
		fi, ok := graph.FileIndex[file]
		if !ok {
			continue
		}
		for _, rel := range fi.Relations {
			if rel.Kind != "embedding" && rel.Kind != "inheritance" {
				continue
			}
			childType := rel.From
			if idx := strings.LastIndex(childType, ":"); idx >= 0 {
				childType = childType[idx+1:]
			}
			verb := "embeds"
			if rel.Kind == "inheritance" {
				verb = "extends"
			}
			allRelations = append(allRelations, hierRelation{
				childType: childType, parentType: rel.To, verb: verb,
			})
		}
	}

	// Multi-pass: propagate concrete values through inheritance chains.
	// Pass 1: direct parent values. Pass 2+: grandparent values etc.
	// A embeds B, B embeds C → A inherits C's concrete values.
	// Cap at 3 passes to prevent runaway in deep hierarchies.
	chainSet := make(map[string]bool) // deduplicate chains
	for pass := 0; pass < 3; pass++ {
		added := 0
		for _, rel := range allRelations {
			vals, ok := valuesByReceiver[rel.parentType]
			if !ok {
				continue
			}
			for _, v := range vals {
				chain := fmt.Sprintf(
					"`%s` %s `%s` → `%s()` %s %s applies to `%s`",
					rel.childType, rel.verb, rel.parentType,
					v.method, v.kind, v.value, rel.childType)
				if !chainSet[chain] {
					chainSet[chain] = true
					hierarchyChains = append(hierarchyChains, chain)
					added++
				}
			}
			// Propagate: child now inherits parent's values for next pass.
			// Copy the slice to avoid shared backing array mutations.
			if _, ok := valuesByReceiver[rel.childType]; !ok {
				cp := make([]concreteValue, len(vals))
				copy(cp, vals)
				valuesByReceiver[rel.childType] = cp
			} else {
				// Merge, avoiding duplicates.
				existing := make(map[string]bool)
				for _, ev := range valuesByReceiver[rel.childType] {
					existing[ev.method] = true
				}
				for _, v := range vals {
					if !existing[v.method] {
						valuesByReceiver[rel.childType] = append(valuesByReceiver[rel.childType], v)
					}
				}
			}
		}
		if added == 0 {
			break
		}
	}
	if len(hierarchyChains) > 0 {
		hierCap := 20
		if len(e.allScoredFiles) > 10 {
			hierCap = 30
		}
		if len(hierarchyChains) > hierCap {
			hierarchyChains = hierarchyChains[:hierCap]
		}
		b.WriteString("### Type Hierarchy Chains\n\n")
		b.WriteString("These types inherit behavior via embedding (Go) or inheritance (Java/Python/JS/Rust):\n\n")
		for _, e := range hierarchyChains {
			b.WriteString("- " + e + "\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

// concreteValueEntry holds a single extracted concrete value from source code.
type concreteValueEntry struct {
	kind  string // "returns", "binds ONLY", "binds"
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
//   - x.Verb(NewFoo(...))       → binds ONLY NewFoo (any method passing a constructor)
//   - return TypeName{...}      → returns TypeName{...}
func extractConcreteValues(source string) []concreteValueEntry {
	var results []concreteValueEntry
	lines := strings.Split(source, "\n")

	// Count non-blank, non-brace-only lines to detect "single-statement" bodies.
	var registerCalls []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Extract the "value expression" from return statements, arrow
		// functions, or implicit returns (Rust: last expression in block).
		var rest string
		hasValue := false

		if strings.HasPrefix(trimmed, "return ") {
			rest = strings.TrimPrefix(trimmed, "return ")
			hasValue = true
		} else if idx := strings.Index(trimmed, " return "); idx >= 0 {
			// Inline return: func() { return X }
			rest = trimmed[idx+len(" return "):]
			hasValue = true
		} else if strings.Contains(trimmed, "=>") {
			// Arrow function: () => "value" or () => value
			if idx := strings.Index(trimmed, "=>"); idx >= 0 {
				rest = strings.TrimSpace(trimmed[idx+2:])
				hasValue = rest != "" && rest != "{"
			}
		} else if !strings.HasPrefix(trimmed, "func ") &&
			!strings.HasPrefix(trimmed, "fn ") &&
			!strings.HasPrefix(trimmed, "def ") &&
			!strings.HasPrefix(trimmed, "//") &&
			!strings.HasPrefix(trimmed, "#") &&
			!strings.HasPrefix(trimmed, "}") &&
			!strings.HasPrefix(trimmed, "{") &&
			!strings.HasPrefix(trimmed, "type ") &&
			!strings.HasPrefix(trimmed, "pub ") &&
			!strings.HasPrefix(trimmed, "if ") &&
			len(trimmed) > 0 {
			// Rust/Ruby implicit return: last line of block is the value.
			// Only treat as implicit return if it looks like a simple
			// expression (quoted string or bare identifier), not a statement.
			candidate := strings.TrimRight(trimmed, " \t};")
			if len(candidate) >= 2 &&
				((candidate[0] == '"' && candidate[len(candidate)-1] == '"') ||
					(candidate[0] == '\'' && candidate[len(candidate)-1] == '\'')) {
				rest = candidate
				hasValue = true
			}
		}

		if hasValue {
			// Strip trailing "}" and whitespace for inline functions
			rest = strings.TrimRight(rest, " \t}")
			rest = strings.TrimSpace(rest)
			rest = strings.TrimRight(rest, ";") // for non-Go/Java/JS
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

		// Pattern: method call passing a constructor or instance as argument.
		// Matches Register(), Handle(), Subscribe(), Add(), etc. — any
		// call whose argument is NewXxx(...) or &Xxx{...}.
		// Skip common non-binding functions that frequently contain
		// "New" or "&" inside string literals or as non-binding args.
		if parenIdx := strings.Index(trimmed, "("); parenIdx > 0 {
			funcName := trimmed[:parenIdx]
			// Skip formatting/logging/utility calls — these never bind.
			isUtility := strings.HasSuffix(funcName, "rintf") || // Printf, Sprintf, Fprintf, Errorf
				strings.HasSuffix(funcName, "Println") ||
				strings.HasPrefix(funcName, "log.") ||
				strings.HasPrefix(funcName, "fmt.") ||
				funcName == "append" || funcName == "make" ||
				funcName == "len" || funcName == "cap" ||
				strings.HasPrefix(funcName, "logging.")
			if !isUtility {
				arg := trimmed[parenIdx+1:]
				// Find matching close paren.
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
					inner := strings.TrimSpace(arg[:end])
					// Require an actual constructor or type reference:
				//   Go:     NewXxx(...) or &Xxx{...}
				//   Java:   new Xxx(...)
				//   Python: Xxx() where Xxx is capitalized (class instantiation)
					hasConstructor := false
					for _, token := range strings.Fields(inner) {
						clean := strings.Trim(token, ",()")
						// Go: NewXxx or newXxx factory
						if strings.HasPrefix(clean, "New") && len(clean) > 3 {
							hasConstructor = true
							break
						}
						// Go: &Xxx{...} pointer to struct literal
						if strings.HasPrefix(clean, "&") && len(clean) > 1 && clean[1] >= 'A' && clean[1] <= 'Z' {
							hasConstructor = true
							break
						}
						// Java: new Xxx(...)
						if clean == "new" {
							hasConstructor = true
							break
						}
						// Python/JS: CapitalizedClass() — bare class instantiation
						// Only if the token is a standalone capitalized identifier
						if len(clean) > 1 && clean[0] >= 'A' && clean[0] <= 'Z' &&
							!strings.ContainsAny(clean, "\"'`=") {
							hasConstructor = true
							break
						}
					}
					if hasConstructor {
						registerCalls = append(registerCalls, inner)
					}
				}
			}
		}
	}

	// Pattern: decorators / annotations.
	//   Python: @app.route("/path"), @app.get("/api"), @login_required
	//   Java:   @GetMapping("/path"), @RequestMapping(value="/path")
	// Detect @decorator(args) lines and pair with the next function/class.
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "@") {
			continue
		}
		// Extract decorator name and arguments.
		decorator := trimmed[1:] // strip @
		var decoratorArgs string
		if parenIdx := strings.Index(decorator, "("); parenIdx > 0 {
			rest := decorator[parenIdx+1:]
			decorator = decorator[:parenIdx]
			if endIdx := strings.LastIndex(rest, ")"); endIdx >= 0 {
				decoratorArgs = rest[:endIdx]
			}
		}
		// Find the decorated function/class on the next non-decorator line.
		target := ""
		for j := i + 1; j < len(lines); j++ {
			nextTrimmed := strings.TrimSpace(lines[j])
			if strings.HasPrefix(nextTrimmed, "@") {
				continue // skip stacked decorators
			}
			// Extract function/class name.
			for _, prefix := range []string{"def ", "class ", "public ", "private ", "protected ", "func ", "async def ", "async "} {
				if strings.HasPrefix(nextTrimmed, prefix) {
					rest := strings.TrimPrefix(nextTrimmed, prefix)
					// Take identifier up to ( or : or {
					endIdx := strings.IndexAny(rest, "({: ")
					if endIdx > 0 {
						target = strings.TrimSpace(rest[:endIdx])
					}
					break
				}
			}
			break
		}
		if target != "" && decoratorArgs != "" {
			results = append(results, concreteValueEntry{
				kind:  "decorates",
				value: fmt.Sprintf("@%s(%s) → %s", decorator, decoratorArgs, target),
			})
		}
	}

	// If there are constructor-passing calls, summarize them.
	if len(registerCalls) > 0 {
		qualifier := "binds ONLY"
		if len(registerCalls) > 1 {
			qualifier = "binds"
		}
		results = append(results, concreteValueEntry{
			kind:  qualifier,
			value: strings.Join(registerCalls, ", "),
		})
	}

	// Pattern: map/dict literal entries — "key: value," lines.
	// Extracts key→value mappings from map literals, routing tables,
	// dispatch tables, etc. Works across Go (map[K]V{...}),
	// Python (dict), JS/TS (object literals), Java (Map.of(...)).
	//
	//   types.AgentExplorer: func(d) { return NewExplorerAgent(d) },
	//   "/api/users": NewUserHandler(),
	//   "explore": "explorer",
	var mapEntries []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Look for "key: value" pattern with trailing comma.
		// Skip lines that are struct field declarations (have type names
		// after the colon, not values).
		colonIdx := strings.Index(trimmed, ":")
		if colonIdx < 1 {
			continue
		}
		key := strings.TrimSpace(trimmed[:colonIdx])
		val := strings.TrimSpace(trimmed[colonIdx+1:])
		val = strings.TrimRight(val, ",")
		val = strings.TrimSpace(val)
		if val == "" || val == "{" || val == "}" {
			continue
		}
		// Key must be a string literal, identifier, or enum constant.
		isMapKey := false
		if len(key) >= 2 && (key[0] == '"' || key[0] == '\'') {
			isMapKey = true // string literal key
		} else if strings.Contains(key, ".") && !strings.HasPrefix(key, "//") {
			isMapKey = true // qualified name like types.AgentExplorer
		}
		// Value must contain a constructor, function, or string literal.
		hasMapping := false
		if isMapKey {
			if strings.Contains(val, "New") || strings.Contains(val, "new ") ||
				(len(val) >= 2 && (val[0] == '"' || val[0] == '\'')) {
				hasMapping = true
			}
			// Lambda/closure: func(...) { ... } or () => ...
			if strings.Contains(val, "func") || strings.Contains(val, "=>") {
				hasMapping = true
			}
		}
		if hasMapping {
			mapEntries = append(mapEntries, key+" → "+val)
		}
	}
	if len(mapEntries) > 0 {
		results = append(results, concreteValueEntry{
			kind:  "maps",
			value: strings.Join(mapEntries, "; "),
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
// extractConfigValues reads a YAML/JSON config file and returns
// key=value entries where the key or value references symbols from
// the investigation notes. For YAML, uses yaml.v3 to properly
// handle nested structures with dotted key paths (e.g.,
// "stages.explore.default_agent = explorer"). For JSON, uses the
// encoding/json decoder. For TOML, falls back to text matching.
func extractConfigValues(path string, notesJoined string) []string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil
	}

	var entries []string

	if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
		// Parse YAML and flatten to dotted key paths.
		var root interface{}
		if err := yaml.Unmarshal(data, &root); err != nil {
			return nil
		}
		flattenYAML("", root, notesJoined, &entries)
	} else if strings.HasSuffix(path, ".json") {
		// Parse JSON and flatten.
		var root interface{}
		if err := json.Unmarshal(data, &root); err != nil {
			return nil
		}
		flattenYAML("", root, notesJoined, &entries) // same flattening logic
	} else {
		// TOML or unknown: text-based fallback.
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
				continue
			}
			if colonIdx := strings.Index(trimmed, " = "); colonIdx > 0 {
				key := trimmed[:colonIdx]
				val := trimmed[colonIdx+3:]
				if strings.Contains(notesJoined, key) || strings.Contains(notesJoined, val) {
					entries = append(entries, key+" = "+val)
				}
			}
		}
	}
	return entries
}

// flattenYAML recursively flattens a parsed YAML/JSON tree into
// "dotted.key.path = value" entries, keeping only leaf scalars whose
// key or value appears in the investigation notes.
func flattenYAML(prefix string, node interface{}, notesJoined string, entries *[]string) {
	switch v := node.(type) {
	case map[string]interface{}:
		for key, val := range v {
			childPrefix := key
			if prefix != "" {
				childPrefix = prefix + "." + key
			}
			flattenYAML(childPrefix, val, notesJoined, entries)
		}
	case map[interface{}]interface{}:
		// yaml.v3 sometimes produces this type for map keys
		for key, val := range v {
			keyStr := fmt.Sprintf("%v", key)
			childPrefix := keyStr
			if prefix != "" {
				childPrefix = prefix + "." + keyStr
			}
			flattenYAML(childPrefix, val, notesJoined, entries)
		}
	case []interface{}:
		for i, item := range v {
			childPrefix := fmt.Sprintf("%s[%d]", prefix, i)
			flattenYAML(childPrefix, item, notesJoined, entries)
		}
	default:
		// Leaf scalar: string, number, bool
		valStr := fmt.Sprintf("%v", v)
		if valStr == "<nil>" || valStr == "" {
			return
		}
		// Only keep if key path or value references investigation symbols.
		// Split the dotted prefix into parts and check each.
		relevant := false
		for _, part := range strings.Split(prefix, ".") {
			if len(part) >= 3 && strings.Contains(notesJoined, part) {
				relevant = true
				break
			}
		}
		if !relevant && len(valStr) >= 3 {
			relevant = strings.Contains(notesJoined, valStr)
		}
		if relevant {
			*entries = append(*entries, prefix+" = "+valStr)
		}
	}
}

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
	// Directories from the shared exclude list (tool.ExcludeDirs).
	for _, dir := range tool.ExcludeDirs {
		dirSlash := dir + "/"
		if strings.HasPrefix(path, dirSlash) || strings.Contains(path, "/"+dirSlash) {
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

// isEvidenceLine returns true if a trimmed source line is likely to
// contain a concrete value pattern (return statement, map entry,
// registration call, or constructor binding). Used by the medium-function
// local line scanner to skip irrelevant lines cheaply.
//
// Patterns are cross-language:
//
//	return/yield   — Go, Python, Java, JS, Rust, Ruby
//	=>             — JS/TS arrow functions, Ruby hash rockets
//	key: value,    — Go maps, Python dicts, JS/TS objects, YAML
//	registration   — Register/Add/Handle/Route/Subscribe/Bind (English naming)
//	constructors   — new Foo (Java/JS), &Foo{ (Go), NewFoo/CreateFoo (Go/Python)
func isEvidenceLine(trimmed string) bool {
	// Return/yield statements (all languages).
	if strings.HasPrefix(trimmed, "return ") || strings.HasPrefix(trimmed, "return\t") ||
		strings.HasPrefix(trimmed, "yield ") {
		return true
	}
	if strings.Contains(trimmed, " return ") {
		return true // inline return: func() { return X }
	}
	// Arrow functions (JS/TS/Rust closures).
	if strings.Contains(trimmed, "=>") {
		return true
	}
	// Map/dict entries: "key": value, or key => value,
	if (strings.Contains(trimmed, ":") || strings.Contains(trimmed, "=>")) &&
		strings.Contains(trimmed, ",") {
		return true
	}
	// Registration/binding calls — common cross-language verb patterns.
	for _, kw := range []string{
		"Register", "register", "Subscribe", "subscribe",
		"Bind", "bind", "Handle", "handle",
		"Route", "route", "Map(", "map(",
		"Add(", "add(", "Set(", "set(",
		"append(", "push(", "insert(",
		"provide(", "Provide(",
	} {
		if strings.Contains(trimmed, kw) {
			return true
		}
	}
	// Constructor patterns — cross-language, tightened to avoid false positives.
	//   Java/JS/TS:  new Foo(...)
	//   Go:          &Foo{...}  (address-of struct literal)
	//   Multi-lang:  FactoryPrefix + UpperCase (NewFoo, CreateFoo, MakeFoo, BuildFoo)
	if strings.Contains(trimmed, "new ") || strings.Contains(trimmed, "new\t") {
		return true
	}
	// &UpperCase{ — Go struct literal, also Rust &Type
	for i := 0; i < len(trimmed)-2; i++ {
		if trimmed[i] == '&' && trimmed[i+1] >= 'A' && trimmed[i+1] <= 'Z' {
			return true
		}
	}
	// FactoryPrefix + UpperCase — matches NewFoo, CreateFoo, MakeFoo, BuildFoo, GetFoo.
	for _, prefix := range []string{"New", "Create", "Make", "Build", "Get"} {
		plen := len(prefix)
		for i := 0; i <= len(trimmed)-plen-1; i++ {
			if trimmed[i:i+plen] == prefix && trimmed[i+plen] >= 'A' && trimmed[i+plen] <= 'Z' {
				// Ensure prefix starts at word boundary.
				if i == 0 || !isIdentChar(trimmed[i-1]) {
					return true
				}
			}
		}
	}
	return false
}

// containsIdentifier checks whether text contains name as a whole
// identifier — not just a substring. A match requires that the character
// immediately before and after the name (if any) is NOT a letter, digit,
// or underscore. This prevents "Handler" from matching "ErrorHandler" or
// "HandlerFunc", while still matching "&Handler{}", "Handler.Name", etc.
//
// Factory prefix allowlist: common cross-language factory/constructor
// prefixes are accepted before the name. For example, "NewFoo" and
// "createFoo" both match "Foo". Supported prefixes:
//
//	Go/Java/C#:  New (NewHandler)
//	Java/JS:     new (new Handler — but typically space-separated)
//	Python/Ruby: create, make, build (create_handler, make_handler)
//	General:     get (getFoo — factory accessor pattern)
var factoryPrefixes = []string{"New", "new", "create", "Create", "make", "Make", "build", "Build", "get", "Get"}

func containsIdentifier(text, name string) bool {
	if name == "" {
		return false
	}
	start := 0
	for {
		idx := strings.Index(text[start:], name)
		if idx < 0 {
			return false
		}
		pos := start + idx
		// Check character after the match — must be a boundary.
		end := pos + len(name)
		if end < len(text) && isIdentChar(text[end]) {
			start = pos + 1
			continue
		}
		// Check character before the match.
		if pos == 0 {
			return true // start of string
		}
		before := text[pos-1]
		if !isIdentChar(before) {
			return true // clean boundary
		}
		// Allow factory prefixes: "NewFoo", "createFoo", etc. match "Foo".
		for _, prefix := range factoryPrefixes {
			plen := len(prefix)
			if pos >= plen && text[pos-plen:pos] == prefix &&
				!isIdentChar(safeCharAt(text, pos-plen-1)) {
				return true
			}
		}
		start = pos + 1
	}
}

// safeCharAt returns the byte at position i, or 0 if out of bounds.
func safeCharAt(s string, i int) byte {
	if i < 0 || i >= len(s) {
		return 0
	}
	return s[i]
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// resolveConditions checks [CONDITIONAL] evidence entries against the
// Concrete Values section. Instead of a shallow word-presence check, it
// parses the IF clause to extract the variable/method being tested and
// the expected value, then matches structurally against concrete values.
//
// Returns the list of conditions that could NOT be resolved.
func resolveConditions(notes []string, concreteValuesSection string) []string {
	if concreteValuesSection == "" {
		return nil
	}

	// Parse the concrete values table into a lookup: method → fact.
	// Table format: | file:line | `Method()` | kind value |
	cvMethods := make(map[string]string) // lowercase method → fact line
	for _, line := range strings.Split(concreteValuesSection, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "| File") || strings.HasPrefix(line, "|---") {
			continue
		}
		cols := strings.SplitN(line, "|", 5)
		if len(cols) < 4 {
			continue
		}
		method := strings.TrimSpace(cols[2])
		method = strings.Trim(method, "`()")
		fact := strings.TrimSpace(cols[3])
		if method != "" {
			cvMethods[strings.ToLower(method)] = fact
		}
	}

	// Also extract resolution chains: "A() kind val → B() kind val"
	var chainTargets []string // lowercase method names from chain right-hand sides
	for _, line := range strings.Split(concreteValuesSection, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- `") {
			continue
		}
		if idx := strings.Index(line, "→"); idx >= 0 {
			rhs := line[idx:]
			// Extract method name between backticks: `Method()`
			if s := strings.Index(rhs, "`"); s >= 0 {
				if e := strings.Index(rhs[s+1:], "`"); e >= 0 {
					m := strings.Trim(rhs[s+1:s+1+e], "()")
					chainTargets = append(chainTargets, strings.ToLower(m))
				}
			}
		}
	}

	var unresolved []string
	for _, note := range notes {
		for _, line := range strings.Split(note, "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "- [CONDITIONAL]") {
				continue
			}

			// Extract the IF clause: everything after " IF " or " if "
			ifIdx := strings.Index(trimmed, " IF ")
			if ifIdx < 0 {
				ifIdx = strings.Index(trimmed, " if ")
			}
			if ifIdx < 0 {
				// No parseable IF clause — mark as unresolved.
				unresolved = append(unresolved, trimmed)
				continue
			}
			condition := trimmed[ifIdx+4:]

			// Strategy: extract identifiers from the condition and check
			// if any of them appear as a method in the concrete values
			// table or resolution chain targets. This is structural: we
			// check that the *tested variable/method* has a concrete value,
			// not just any random word overlap.
			condTokens := extractIdentifiers(condition)
			resolved := false
			for _, tok := range condTokens {
				tokLower := strings.ToLower(tok)
				if _, ok := cvMethods[tokLower]; ok {
					resolved = true
					break
				}
				// Check if token is a type.Method pattern
				for method := range cvMethods {
					if strings.HasSuffix(method, "."+tokLower) || strings.HasPrefix(method, tokLower+".") {
						resolved = true
						break
					}
				}
				if resolved {
					break
				}
				// Check resolution chain targets
				for _, ct := range chainTargets {
					if ct == tokLower || strings.HasSuffix(ct, "."+tokLower) {
						resolved = true
						break
					}
				}
				if resolved {
					break
				}
			}
			if !resolved {
				unresolved = append(unresolved, trimmed)
			}
		}
	}
	return unresolved
}

// extractIdentifiers pulls identifier-like tokens from a condition string.
// Recognizes dotted names (foo.Bar), plain identifiers, and backtick-quoted
// symbols. Filters out common noise words and very short tokens.
func extractIdentifiers(s string) []string {
	var result []string
	seen := make(map[string]bool)

	// First extract backtick-quoted symbols: `symbolName`
	for {
		start := strings.Index(s, "`")
		if start < 0 {
			break
		}
		end := strings.Index(s[start+1:], "`")
		if end < 0 {
			break
		}
		sym := s[start+1 : start+1+end]
		sym = strings.Trim(sym, "()")
		if len(sym) >= 3 && !seen[sym] {
			seen[sym] = true
			result = append(result, sym)
		}
		s = s[:start] + " " + s[start+1+end+1:]
	}

	// Then extract bare identifiers (alphanumeric + underscore + dot).
	token := ""
	for i := 0; i <= len(s); i++ {
		var c byte
		if i < len(s) {
			c = s[i]
		}
		if isIdentChar(c) || c == '.' {
			token += string(c)
		} else {
			token = strings.Trim(token, ".")
			if len(token) >= 3 && !seen[token] && !isConditionNoise(token) {
				seen[token] = true
				result = append(result, token)
			}
			token = ""
		}
	}
	return result
}

// isConditionNoise filters out common English words that appear in
// conditions but are not code identifiers.
func isConditionNoise(s string) bool {
	switch strings.ToLower(s) {
	case "the", "and", "for", "not", "this", "that", "when", "then",
		"true", "false", "nil", "null", "none", "with", "from",
		"has", "was", "are", "were", "been", "does", "any", "all":
		return true
	}
	return false
}

// NewExplorerAgent creates the explorer agent (used in explore stage).
func NewExplorerAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentExplorer, deps, &explorerEvaluator{tools: deps.Tools})
}
