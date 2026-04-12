package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/dataflow"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
	"gopkg.in/yaml.v3"
)

type explorerEvaluator struct {
	tools                     *tool.Registry
	phase                     int                  // 0 = breadth scan, 1 = depth read
	broadenAttempts           int                  // times we pushed for broader grep in Phase 0
	idleStreakInDepth         int                  // consecutive no-tool-call rounds in Phase 2
	lastToolResultCount       int                  // tool result count at last continuation check
	preScannedFiles           []string             // top files from keyword search, for coverage tracking
	allScoredFiles            []string             // ALL files from keyword search (not just top 8), for supplementary evidence
	fileSymbols               map[string][]string  // path → symbol summaries from repo_map
	searchResult              *keywordSearchResult // full search result for cross-reference lookups
	investigationNotes        []string             // assistant analysis messages from ReAct loop
	userQuestion              string               // original user question, for focus alignment
	repoRoot                  string               // repository root path, cached from BuildInitialPrompt
	preScannedPushCount       int                  // times we pushed for unread pre-scanned files without progress
	lastPreScannedUnreadCount int                  // count of unread pre-scanned files at last push
	grepRedirectedFiles       map[string]bool      // files that already received a large-file grep redirect
	isEnumerationQuery        bool                 // true if user question asks to list/enumerate all items
	phase0ExtraRound          bool                 // whether we already gave one extra Phase 0 round for quality gate
	structuredEvidence        []types.EvidenceItem
	flowFindings              []types.FlowFindingDigest
	ermRequirements           []EvidenceRequirement // evidence requirement model
	cachedConcreteValues      *concreteValuesResult // T1.1: built once per Execute, reused by gate + synthesis
	midLoopLastInjectIter     int                   // #34: throttle MidLoopCheck cadence
}

func (e *explorerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	e.userQuestion = ctx.CurrentTask
	e.repoRoot = ctx.RepoRoot
	e.isEnumerationQuery = detectEnumerationIntent(ctx.CurrentTask)
	e.structuredEvidence = nil
	e.flowFindings = nil
	e.cachedConcreteValues = nil
	e.midLoopLastInjectIter = -10

	// Self-loop detection: if we already have investigation notes from
	// a prior run, this is a retry (explore → explore self-loop). Skip
	// Phase 0 breadth scan and go directly to Phase 1 depth read with
	// a retry-specific prompt. The agent is a singleton so evaluator
	// state (investigationNotes, searchResult, preScannedFiles) survives
	// across dispatches.
	if len(e.investigationNotes) > 0 {
		e.phase = 1
		// Reset per-run counters but preserve accumulated evidence.
		e.idleStreakInDepth = 0
		e.lastToolResultCount = 0
		e.preScannedPushCount = 0
		e.lastPreScannedUnreadCount = 0
		e.broadenAttempts = 0
		e.grepRedirectedFiles = nil // re-detect large files in retry

		var b strings.Builder
		b.WriteString("## Retry: Depth Investigation (continued)\n\n")
		b.WriteString("Your previous investigation of this question was insufficient.\n\n")
		if ctx.RetryHint != "" {
			fmt.Fprintf(&b, "**Retry directive:** %s\n\n", ctx.RetryHint)
		}

		// Inject the previous synthesis conclusion so the retry builds on
		// it rather than starting from scratch. Without this, the second
		// explore round drifts — producing a different (often worse)
		// answer instead of improving the first one.
		if len(ctx.PriorReports) > 0 {
			for i := len(ctx.PriorReports) - 1; i >= 0; i-- {
				if ctx.PriorReports[i].Stage == types.StageExplore {
					findings := ctx.PriorReports[i].Findings
					if len(findings) > 3000 {
						findings = findings[:3000] + "\n... [truncated]"
					}
					b.WriteString("## Previous Synthesis (baseline — improve, don't restart)\n\n")
					b.WriteString(findings)
					b.WriteString("\n\n")
					b.WriteString("The answer above was judged insufficient. Identify its specific gaps " +
						"and fill them — do NOT discard it and start over.\n\n")
					break
				}
			}
		}

		fmt.Fprintf(&b, "You already collected %d evidence sets. ",
			len(e.investigationNotes))
		b.WriteString("Focus on the gaps identified above. Do NOT re-read files you already analyzed.\n\n")
		b.WriteString("Continue using the evidence collection format:\n")
		b.WriteString("- [DIRECT] `functionName` line N: <what this code establishes>\n")
		b.WriteString("- [CONDITIONAL] `functionName` line N: <what happens> IF <condition>\n")
		b.WriteString("- [REGISTRATION] `functionName` line N: <what is registered, EXACT values>\n\n")
		b.WriteString("**Large file strategy:** grep for key identifiers first, prefer `context_lines=3`, then read only matched line ranges when you need the full body.\n")
		b.WriteString("**User question:** " + e.userQuestion)
		return b.String()
	}

	e.phase = 0 // start in breadth-scan phase

	var b strings.Builder
	b.WriteString("## Phase 1: Breadth Scan\n\n")
	b.WriteString("Your goal in this phase is to MAP the relevant territory — find ALL files related to the question. ")
	b.WriteString("Do NOT read files in full yet. Use lightweight tools:\n")
	b.WriteString("- repo_map (task_map view) to get an overview of relevant files\n")
	b.WriteString("- grep with files_only=true to find WHICH FILES contain key terms (just filenames, not lines). Use `file_type` when the language is obvious; do not use --include so you discover all relevant file types\n")
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
			// Extract ERM requirements with separate entity and keyword
			// sources:
			//
			//  - Entities come from `ctx.Objective` ONLY (the original
			//    user request), so the precise CamelCase identifiers
			//    survive. Falls back to `ctx.CurrentTask` only when
			//    Objective is empty (e.g. analyze stage stub state).
			//  - Keyword detection runs over the union `Objective | CurrentTask`,
			//    so Chinese trigger words ("怎么"/"多少") AND the analyzer's
			//    English idioms ("Determine the number of...") both fire.
			//
			// Earlier (commit c04298f) ran both extractions over the
			// joined string. The integration test (df1 5x, 063536) caught
			// a regression: the analyzer's rewrite contributed generic
			// English nouns ("count","agents","that","call") to the
			// entity set, inflating registration req count from 2 to 8
			// and flipping answer_chain[0] from the canonical
			// `RegisterDefaultSubAgents → SubExplorer` chain to the
			// spurious `RegisterDefaults → GrepTool.Description` chain
			// (the tool registry matched MORE polluted entities than the
			// correct answer). Splitting the sources isolates the noise.
			// Entity source strategy: UNION of the analyzer's declared
			// entities (verbatim from user wording per analyzer contract)
			// and the regex extraction over ctx.Objective. Deduplicated,
			// analyzer entries first so they dominate any ordering-based
			// downstream logic.
			//
			// Union, not preference, because:
			//  - Analyzer alone is not sufficient: df1 revealed the analyzer
			//    can legitimately produce only 1 entity ("subagent") when
			//    the user's phrasing has a single CamelCase-looking token.
			//    ERM's call_chain requirement demands 2+ entities to reach
			//    "satisfied", so the T1.1 gate never skipped and dataflow
			//    ran on every run (eval/results/df1-20260412-081619).
			//  - Regex alone is the pre-analyzer-contract behaviour: it
			//    works but misses user-intent signal (the analyzer saw the
			//    raw request and chose symbols deliberately).
			//
			// The c04298f regression this change must NOT re-introduce was
			// joining the original Chinese question and the analyzer's
			// English rewrite into a single STRING and then running regex
			// extraction over the noise. That is a different failure mode:
			// here we keep the extraction sources SEPARATE (analyzer field
			// + regex over Objective only) and merge two clean lists.
			var ermEntities []string
			seen := make(map[string]bool)
			for _, ent := range ctx.CurrentTaskEntities {
				if ent = strings.TrimSpace(ent); ent != "" && !seen[ent] {
					ermEntities = append(ermEntities, ent)
					seen[ent] = true
				}
			}
			regexEntities := extractRankingEntities(ctx.Objective)
			if len(regexEntities) == 0 {
				regexEntities = extractRankingEntities(ctx.CurrentTask)
			}
			for _, ent := range regexEntities {
				if !seen[ent] {
					ermEntities = append(ermEntities, ent)
					seen[ent] = true
				}
			}
			ermKeywordSource := ctx.CurrentTask
			if ctx.Objective != "" && ctx.Objective != ctx.CurrentTask {
				ermKeywordSource = ctx.Objective + " | " + ctx.CurrentTask
			}
			// Pass the analyzer's declared question_kind (may be empty or
			// "unknown"; the hint-aware path handles both by falling
			// back to pure keyword inference).
			e.ermRequirements = extractEvidenceRequirementsWithHint(
				ermKeywordSource, ermEntities, ctx.CurrentTaskQuestionKind,
			)
			// Auto-satisfy requirements whose entities don't match any
			// symbol in the codebase — prevents generic English words from
			// creating unsatisfiable requirements that block the pipeline.
			if sr.Graph != nil {
				e.ermRequirements = ermAutoSatisfyUnresolvable(e.ermRequirements, sr.Graph)
			}
			logERM(e.ermRequirements)

			sort.Slice(candidates, func(i, j int) bool {
				// Primary: ERM score (question-relevant files first)
				// Secondary: repo_map structural importance
				var ermI, ermJ float64
				if sr.Graph != nil {
					ermI = ermFileScore(sr.Graph.FileIndex[candidates[i].path], e.ermRequirements)
					ermJ = ermFileScore(sr.Graph.FileIndex[candidates[j].path], e.ermRequirements)
				}
				scoreI := candidates[i].repoMapScore + ermI*200 // ERM boost
				scoreJ := candidates[j].repoMapScore + ermJ*200
				return scoreI > scoreJ
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
// MidLoopCheck (#34) fires after every tool batch. Unlike
// ContinuationPrompt — which only runs on soft-stop — this is the only
// channel that can redirect the LLM while it is still actively calling
// tools but in the wrong direction. The check is throttled to fire at
// most once every 3 iterations and only after iter ≥ 3 so the LLM has
// at least one productive cycle of tool reads to evaluate against.
//
// Two invariants are checked, both reusing helpers that already exist
// for ContinuationPrompt but were blind to the active-tool-calling
// case:
//
//  1. Function-boundary coverage — `detectPartiallyReadSymbols` finds
//     read_file slices that left a long function partially read. The
//     LLM gets a one-line nudge with exact offset/limit.
//  2. Enumeration completeness — when the question asks to list all X
//     and the LLM has read fewer files than the discovered set, push
//     the unread tail into the conversation.
//
// Hints are kept short on purpose — this runs every iteration and
// would otherwise blow up the message budget.
func (e *explorerEvaluator) MidLoopCheck(iteration int, lastResult *types.ToolResult, allResults []types.ToolResult) (string, bool) {
	// Throttle: fire at most every 3 iters, and not before iter 3.
	if iteration < 3 || iteration-e.midLoopLastInjectIter < 3 {
		return "", false
	}
	if e.searchResult == nil || e.searchResult.Graph == nil {
		return "", false
	}

	var b strings.Builder

	// Check 1: function-boundary coverage.
	if hints := detectPartiallyReadSymbols(allResults, e.searchResult.Graph); len(hints) > 0 {
		h := hints[0] // worst-coverage offender
		fmt.Fprintf(&b, "MID-LOOP CHECK: you started reading `%s` at lines %d-%d but the function spans %d-%d (%.0f%% covered). "+
			"Finish reading this function before moving on — call read_file with offset=%d limit=%d.\n",
			h.symbolName, h.symStart, h.readEnd, h.symStart, h.symEnd, h.coverage*100,
			h.readEnd+1, h.symEnd-h.readEnd)
	}

	// Check 2: enumeration completeness.
	//
	// Two coverage tiers work together to push enumeration questions
	// toward "list ALL" correctness without overshooting on the easy
	// cases:
	//
	//   0.6 (here, mid-loop)  — early warning: fire when the LLM has
	//      read less than 60% AND there are at least 2 files still
	//      unread. This is a "you're falling behind" nudge, not a
	//      hard gate; if coverage is already above 60% we trust the
	//      LLM to finish on its own.
	//
	//   0.8 (line ~536, pre-stop) — hard gate: fire on the LLM's
	//      soft-stop attempt when coverage is below 80%. Blocks
	//      finalization of any enumeration that hasn't cleared the
	//      "read almost all discovered files" bar.
	//
	// The two-tier split matters because a single 0.8 gate would
	// only push at soft-stop time, burning iterations; a single 0.6
	// gate would let questions with 75%-80% coverage slip through.
	// Both numbers encode "list ALL" semantics, not case-specific
	// tuning — they are tied to the enumeration question class, not
	// df1.
	if e.isEnumerationQuery {
		discovered, readSet := extractFileCoverage(allResults)
		if len(discovered) > 0 {
			coverage := float64(len(readSet)) / float64(len(discovered))
			if coverage < 0.6 && len(discovered)-len(readSet) >= 2 {
				var unread []string
				for _, f := range discovered {
					if !readSet[f] && !isNoisePath(f) {
						unread = append(unread, f)
					}
					if len(unread) >= 5 {
						break
					}
				}
				if len(unread) > 0 {
					if b.Len() > 0 {
						b.WriteString("\n")
					}
					fmt.Fprintf(&b, "MID-LOOP CHECK: the question asks for an enumeration but you have read only %d of %d discovered files (%.0f%%). "+
						"Read these next: %s\n",
						len(readSet), len(discovered), coverage*100, strings.Join(unread, ", "))
				}
			}
		}
	}

	if b.Len() == 0 {
		return "", false
	}
	e.midLoopLastInjectIter = iteration
	return b.String(), true
}

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
				"Run at least 2-3 new grep calls with files_only=true before producing your file list. If the repo is polyglot, use grep `file_type` to narrow by language.", true
		}

		// Quality gate: before transitioning to Phase 1, verify the
		// breadth scan used enough discovery tools and found enough files.
		// At most 1 extra round (phase0ExtraRound prevents infinite loop).
		if !e.phase0ExtraRound {
			discovered, _ = extractFileCoverage(history)
			usedGrep := false
			usedOtherDiscovery := false
			for _, r := range history {
				if r.Success {
					switch r.ToolName {
					case "grep":
						usedGrep = true
					case "repo_map", "list_files":
						usedOtherDiscovery = true
					}
				}
			}
			if (!usedGrep || !usedOtherDiscovery) || len(discovered) < 3 {
				e.phase0ExtraRound = true
				var gate strings.Builder
				gate.WriteString("Before moving to depth reading, broaden your search:\n")
				if !usedGrep {
					gate.WriteString("- You haven't used grep yet. Search for key terms from the question with files_only=true.\n")
				}
				if !usedOtherDiscovery {
					gate.WriteString("- Use repo_map (task_map view) to see structurally relevant files.\n")
				}
				if len(discovered) < 3 {
					fmt.Fprintf(&gate, "- You only discovered %d files. Use broader search patterns to find at least 3.\n", len(discovered))
				}
				return gate.String(), true
			}
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
			"- Read ONE file at a time\n" +
			"- **Large file strategy (MANDATORY for files >500 lines):** Do NOT read large files with offset=0. " +
			"Instead, FIRST grep the file for the key identifiers from the user's question (field names, function names, string literals), using `context_lines=3` where helpful, " +
			"THEN read only the specific line ranges where grep found matches. " +
			"Sequential paging through a 2000-line file wastes steps and misses content — targeted grep + read is both faster and more thorough\n\n" +
			"Start investigating now. For each file: grep first, then read the matched sections.", true
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

	// Function-boundary read guidance (HIGHEST PRIORITY): when the LLM
	// reads part of a function but stops before the end, inject exact
	// read ranges. This is the #1 cause of answer quality failures —
	// the LLM reads 40 lines of a 300-line function and misses critical
	// logic at the end. Must fire before all other checks.
	if e.searchResult != nil && e.searchResult.Graph != nil {
		partialHints := detectPartiallyReadSymbols(history, e.searchResult.Graph)
		if len(partialHints) > 0 {
			e.idleStreakInDepth = 0 // reset to keep loop alive
			var hint strings.Builder
			hint.WriteString("**CRITICAL: Incomplete function reads.** You MUST finish reading these functions before doing anything else:\n\n")
			for _, ph := range partialHints {
				unreadLines := ph.symEnd - ph.readEnd
				if ph.coverage < 0.3 {
					fmt.Fprintf(&hint, "- `%s` in %s (lines %d-%d, %d lines): you read only %.0f%% of this function. "+
						"Call `read_file` with offset=%d limit=%d to see the FULL implementation\n",
						ph.symbolName, ph.file, ph.symStart, ph.symEnd, ph.symEnd-ph.symStart+1,
						ph.coverage*100, ph.symStart, ph.symEnd-ph.symStart+1)
				} else {
					fmt.Fprintf(&hint, "- `%s` in %s (lines %d-%d): you read up to line %d (%.0f%%). "+
						"Call `read_file` with offset=%d limit=%d to see the remaining %d lines\n",
						ph.symbolName, ph.file, ph.symStart, ph.symEnd,
						ph.readEnd, ph.coverage*100,
						ph.readEnd, unreadLines+1, unreadLines)
				}
			}
			hint.WriteString("\nDo NOT read other files or draw conclusions until you have read the complete function bodies listed above. " +
				"The unread portions often contain the answer to the question.")
			return hint.String(), true
		}
	}

	// Enumeration completeness: when the question asks to "list all X",
	// verify that the LLM has analyzed enough of the discovered files.
	// A coverage gap here means the enumeration will be incomplete.
	if e.isEnumerationQuery && len(discovered) > 0 {
		enumCoverage := float64(len(readSet)) / float64(len(discovered))
		if enumCoverage < 0.8 && len(unread) > 0 {
			var hint strings.Builder
			fmt.Fprintf(&hint, "**Enumeration completeness check:** This question asks to list ALL items. "+
				"You found %d matching files but only read %d (%.0f%% coverage). "+
				"For enumeration queries you must achieve ≥80%% coverage.\n\n"+
				"Unread files:\n", len(discovered), len(readSet), enumCoverage*100)
			for _, f := range unread {
				hint.WriteString("- " + f + "\n")
			}
			hint.WriteString("\nRead these files to ensure your enumeration is complete. " +
				"Skip only files that are clearly unrelated (test helpers, documentation).")
			e.idleStreakInDepth = 0
			return hint.String(), true
		}
	}

	// Large-file grep redirect: when the LLM reads a large file but
	// only sees a truncated portion, it tends to page through blindly,
	// producing shallow evidence. Detect truncated read_file results
	// where the LLM has NOT already grepped that file (with line-level
	// results), and redirect to a grep-then-read strategy.
	// Tracked per-file so each new large file gets its own redirect.
	if e.grepRedirectedFiles == nil {
		e.grepRedirectedFiles = make(map[string]bool)
	}
	truncated, grepped := detectTruncatedUngrepped(history)
	var newTruncated []truncatedFileInfo
	for _, tf := range truncated {
		if !e.grepRedirectedFiles[tf.path] {
			newTruncated = append(newTruncated, tf)
		}
	}
	if len(newTruncated) > 0 {
		for _, tf := range newTruncated {
			e.grepRedirectedFiles[tf.path] = true
		}
		var hint strings.Builder
		hint.WriteString("**Strategy redirect — large files detected.**\n\n")
		hint.WriteString("You are reading large files that don't fit in a single read_file result. ")
		hint.WriteString("Paging through them sequentially will miss details and waste steps.\n\n")
		hint.WriteString("**For each truncated file below, grep for the specific pattern from the user's question WITHIN that file**, ")
		hint.WriteString("then read only the matched line ranges:\n\n")
		for _, tf := range newTruncated {
			fmt.Fprintf(&hint, "- `%s` (read %d of %d lines) — ",
				tf.path, tf.linesRead, tf.totalLines)
			if grepped[tf.path] {
				hint.WriteString("already grepped with files_only, but **re-grep with files_only=false** to get LINE NUMBERS\n")
			} else {
				hint.WriteString("not yet grepped — **grep for the key pattern now**\n")
			}
		}
		hint.WriteString("\nThe user question is: " + e.userQuestion + "\n")
		hint.WriteString("Identify the key identifier (field name, constant, function) and grep for it within these files.")
		return hint.String(), true
	}

	// Track consecutive no-tool-call rounds in Phase 2.
	if len(history) > e.lastToolResultCount {
		e.idleStreakInDepth = 0
	}
	e.lastToolResultCount = len(history)
	e.idleStreakInDepth++

	// --- ERM gap-directed file suggestions ---
	// Check which evidence requirements are still unsatisfied and suggest
	// specific files to read. This is higher priority than generic coverage
	// pushes because it's semantically directed by the question.
	if len(e.ermRequirements) > 0 && e.searchResult != nil && e.searchResult.Graph != nil {
		e.ermRequirements = checkRequirementSatisfaction(e.ermRequirements, e.investigationNotes, e.structuredEvidence)
		logERM(e.ermRequirements)
		if !ermAllSatisfied(e.ermRequirements) {
			suggestions := ermSuggestFiles(e.searchResult.Graph, e.ermRequirements, readSet, 3)
			if len(suggestions) > 0 {
				var hint strings.Builder
				hint.WriteString(ermUnsatisfiedGaps(e.ermRequirements))
				hint.WriteString("**Suggested files to fill these gaps** (read them NOW):\n\n")
				for _, s := range suggestions {
					hint.WriteString(fmt.Sprintf("- `%s` (score=%.1f) — %s\n", s.Path, s.Score, s.Reason))
				}
				hint.WriteString("\nCall `read_file` on the top suggestion immediately. ")
				hint.WriteString("Extract structured evidence with [DIRECT], [REGISTRATION], [CONDITIONAL] tags.\n")
				if e.idleStreakInDepth >= 1 {
					e.idleStreakInDepth = 0 // reset: we have directed work to do
				}
				return hint.String(), true
			}
		}
	}

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
		path          string
		missedSymbols []string
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
		cvPreview = e.getConcreteValuesCached(e.repoRoot, readSet).markdown
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
				Key:         r.ToolName,
				Value:       r.Summary,
				Source:      logicalFactSource(r.Summary, r.ToolName),
				EvidenceRef: r.RawRef,
				Confidence:  confidence,
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

	e.ensureStructuredEvidence(ctx, toolResults)

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
	// Enumeration queries need stricter thresholds: higher file coverage
	// and more evidence entries to ensure exhaustive listing.
	if e.isEnumerationQuery {
		fileCoverage = coverage >= 0.8 || len(readSet) >= len(discovered)
		minDirect := len(discovered) / 3
		if minDirect < 2 {
			minDirect = 2
		}
		evidenceQuality = directCount >= minDirect
	}
	hasEnough := toolDiversity && fileCoverage && evidenceQuality

	// ERM quality gate: if we have evidence requirements that are still
	// unsatisfied, demote hasEnough to trigger a retry that fills gaps.
	if hasEnough && len(e.ermRequirements) > 0 {
		e.ermRequirements = checkRequirementSatisfaction(e.ermRequirements, e.investigationNotes, e.structuredEvidence)
		if !ermAllSatisfied(e.ermRequirements) {
			unsatCount := 0
			for _, r := range e.ermRequirements {
				if r.Status == "unsatisfied" {
					unsatCount++
				}
			}
			// Only block if there are fully unsatisfied requirements.
			// Partial requirements are tolerable.
			if unsatCount > 0 {
				logging.Debug("[explorer] ERM gate: %d unsatisfied requirements, demoting hasEnough", unsatCount)
				hasEnough = false
			}
		}
	}

	signals := &types.ExecutionSignals{HasEnoughFacts: hasEnough}

	// Rank evidence and findings by relevance to the user's question
	// so downstream consumers (finalizer) get the most useful items first.
	rankedEvidence := rankEvidenceByRelevance(e.userQuestion, e.structuredEvidence, readSet)
	rankedFindings := rankFindingsByRelevance(e.userQuestion, e.flowFindings)

	// Identify answer chains: deterministic resolution chains that
	// directly answer the user's question. These get a dedicated
	// section in the finalizer prompt with higher priority than
	// generic evidence items.
	var ermGraph *repomap.Graph
	if e.searchResult != nil {
		ermGraph = e.searchResult.Graph
	}
	answerChains := identifyAnswerChains(e.userQuestion, e.structuredEvidence, 5,
		buildAnswerWhitelist(e.ermRequirements), e.ermRequirements, ermGraph)
	if len(answerChains) > 0 {
		logging.Debug("[explorer] identified %d answer chains", len(answerChains))
		for i, ac := range answerChains {
			logging.Debug("[explorer]   answer_chain[%d]: %s", i, ac)
		}
	}

	// L0-2: structured translation. For registration / call_chain /
	// return_value kinds, extract canonical terminal symbols from the
	// chains so the finalizer can be constrained to prose over this
	// exact list. For other kinds, returns nil and the finalizer
	// falls back to the legacy prose path.
	answerSymbols := extractAnswerSymbols(answerChains, ctx.CurrentTaskQuestionKind, ermGraph)
	if len(answerSymbols) > 0 {
		logging.Debug("[explorer] L0-2 extracted %d answer symbols", len(answerSymbols))
		for i, s := range answerSymbols {
			if s.File != "" {
				logging.Debug("[explorer]   answer_symbol[%d]: %s (%s:%d)", i, s.Name, s.File, s.Line)
			} else {
				logging.Debug("[explorer]   answer_symbol[%d]: %s", i, s.Name)
			}
		}
	}

	out := &StageOutput{
		Data:          json.RawMessage(fmt.Sprintf(`{"result": %q}`, lastContent)),
		NewFacts:      facts,
		EvidenceItems: rankedEvidence,
		FlowFindings:  rankedFindings,
		AnswerChains:  answerChains,
		AnswerSymbols: answerSymbols,
		SignalUpdates: signals,
	}

	if !signals.HasEnoughFacts {
		if !toolDiversity {
			out.RetryHint = "Previous attempt used fewer than 2 distinct evidence tool types. Use both grep and read_file."
		} else if !evidenceQuality {
			out.RetryHint = fmt.Sprintf("Previous attempt collected only %d [DIRECT]/[REGISTRATION] evidence entries (need ≥2). Read more files and extract structured evidence with [DIRECT], [REGISTRATION], [CONDITIONAL] tags.", directCount)
		} else if len(e.ermRequirements) > 0 && !ermAllSatisfied(e.ermRequirements) {
			out.RetryHint = "Previous attempt left evidence requirements unsatisfied. " + ermUnsatisfiedGaps(e.ermRequirements)
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

func (e *explorerEvaluator) ensureStructuredEvidence(ctx *types.AgentContext, toolResults []types.ToolResult) {
	if len(e.structuredEvidence) > 0 || len(e.flowFindings) > 0 {
		return
	}

	parsed := parseEvidenceItems(e.investigationNotes, "explorer.llm")
	intent := dataflowIntent(e.userQuestion, parsed)
	hasGraph := e.searchResult != nil && e.searchResult.Graph != nil
	logging.Debug("[explorer] ensureStructuredEvidence: parsed=%d dataflowIntent=%s hasGraph=%v", len(parsed), intent, hasGraph)
	if !hasGraph || intent == IntentNone {
		e.structuredEvidence = parsed
		return
	}

	_, readSet := extractFileCoverage(toolResults)

	// T1.1: two-phase dataflow decision. Build concrete values + chains
	// early, merge with parsed LLM evidence, run ERM satisfaction check on
	// a *copy* of requirements (so the live state updated later is
	// unaffected). When all ERM requirements are satisfied by the
	// deterministic layers, skip the heavy dataflow.Analyze pass — the
	// question is already answered by Concrete Values + Chains.
	//
	// mechEvidence is declared at the outer scope so the dataflow path
	// (when the gate falls through) can also merge it into the final
	// structuredEvidence.
	var mechEvidence []types.EvidenceItem
	if len(e.ermRequirements) > 0 {
		cv := e.getConcreteValuesCached(ctx.RepoRoot, readSet)
		// T2.2: produce structured EvidenceMechanism items for ERM
		// mechanism requirements. No-op for non-mechanism questions.
		mechEvidence = scanMechanismEvidence(e.ermRequirements, e.searchResult.Graph, ctx.RepoRoot)
		trial := mergeEvidenceItems(parsed, cv.evidence)
		if len(mechEvidence) > 0 {
			trial = mergeEvidenceItems(trial, mechEvidence)
		}
		reqsCopy := make([]EvidenceRequirement, len(e.ermRequirements))
		copy(reqsCopy, e.ermRequirements)
		reqsCopy = checkRequirementSatisfaction(reqsCopy, e.investigationNotes, trial)
		if ermAllSatisfied(reqsCopy) {
			logging.Debug("[explorer] T1.1 gate: ERM all satisfied by parsed(%d)+concreteValues(%d)+mechanism(%d) — skipping dataflow.Analyze",
				len(parsed), len(cv.evidence), len(mechEvidence))
			// Merge mechanism evidence into structuredEvidence so it
			// reaches the finalizer regardless of whether dataflow runs.
			// Concrete Values are merged later in SynthesisPrompt via
			// the existing path (line ~1192).
			if len(mechEvidence) > 0 {
				e.structuredEvidence = mergeEvidenceItems(parsed, mechEvidence)
			} else {
				e.structuredEvidence = parsed
			}
			return
		}
		var unsat []string
		for _, r := range reqsCopy {
			if r.Status != "satisfied" {
				unsat = append(unsat, fmt.Sprintf("%s/%s", r.Kind, r.Status))
			}
		}
		logging.Debug("[explorer] T1.1 gate: ERM unsatisfied (%d/%d) — running dataflow.Analyze: %s",
			len(unsat), len(reqsCopy), strings.Join(unsat, ","))
	}
	candidateSet := make(map[string]bool)
	for file := range readSet {
		candidateSet[file] = true
	}
	for _, file := range e.preScannedFiles {
		candidateSet[file] = true
	}
	for _, file := range e.allScoredFiles {
		candidateSet[file] = true
	}
	// Expand candidates with ERM-directed files that may have been
	// missed by keyword search ranking but contain gap-filling evidence.
	if len(e.ermRequirements) > 0 {
		for _, s := range ermSuggestFiles(e.searchResult.Graph, e.ermRequirements, readSet, 5) {
			candidateSet[s.Path] = true
		}
	}
	var candidates []string
	for file := range candidateSet {
		candidates = append(candidates, file)
	}
	sort.Strings(candidates)

	// T2.3: thread ERM entities into dataflow as a re-ranking bias so
	// the engine focuses on question-relevant files when truncating to
	// MaxFiles.
	var entityBias []string
	for _, r := range e.ermRequirements {
		entityBias = append(entityBias, r.Entities...)
	}
	result := dataflow.Analyze(e.searchResult.Graph, dataflow.Options{
		RepoRoot:        ctx.RepoRoot,
		Question:        e.userQuestion,
		CandidateFiles:  candidates,
		WorkDir:         ctx.WorkDir,
		MaxFiles:        40,
		MaxIterations:   6,
		MaxNodesPerFunc: 400,
		SkipFindings:    intent == IntentLookup,
		EntityBias:      entityBias,
	})
	logging.Debug("[explorer] dataflow.Analyze(intent=%s): %d evidence, %d findings from %d candidates",
		intent, len(result.Evidence), len(result.Findings), len(candidates))
	e.structuredEvidence = mergeEvidenceItems(parsed, result.Evidence)
	if len(mechEvidence) > 0 {
		e.structuredEvidence = mergeEvidenceItems(e.structuredEvidence, mechEvidence)
	}
	e.flowFindings = mergeFlowFindings(result.Findings)
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

	e.ensureStructuredEvidence(ctx, toolResults)
	// Pre-rank evidence so structured sections show the most relevant items.
	e.structuredEvidence = rankEvidenceByRelevance(e.userQuestion, e.structuredEvidence, nil)
	e.flowFindings = rankFindingsByRelevance(e.userQuestion, e.flowFindings)

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
			// Adaptive truncation: scale limit with investigation complexity.
			// More notes = more complex investigation = each note needs more space.
			truncLimit := 1200
			if noteCount := len(e.investigationNotes); noteCount > 3 {
				truncLimit = 1200 + 400*(noteCount-3)
			}
			if truncLimit > 3000 {
				truncLimit = 3000
			}
			if len(note) > truncLimit {
				note = note[:truncLimit] + "\n... [truncated]"
			}
			fmt.Fprintf(&digest, "### Evidence Set %d\n%s\n\n", i+1, note)
		}
	}

	if section := formatEvidenceSection(e.structuredEvidence, types.EvidenceConcrete, "Structured Concrete Evidence", 18); section != "" {
		digest.WriteString(section)
	}
	if section := formatFlowFindingsSection(e.flowFindings, "Resolved Dataflow Paths", 10, false); section != "" {
		digest.WriteString(section)
	}
	if section := formatEvidenceSection(e.structuredEvidence, types.EvidenceConditional, "Condition Guards", 12); section != "" {
		digest.WriteString(section)
	}
	if section := formatFlowFindingsSection(e.flowFindings, "Conflicts / Unknowns", 8, true); section != "" {
		digest.WriteString(section)
	}
	if section := formatEvidenceSection(e.structuredEvidence, types.EvidenceAbsent, "Negative Evidence", 10); section != "" {
		digest.WriteString(section)
	}

	// Focus alignment: detect if the LLM's evidence primarily discusses
	// a different entity than what the question asks about.
	questionEntities := extractQuestionEntities(e.userQuestion)
	if len(questionEntities) > 0 && len(e.investigationNotes) > 0 {
		notesJoined := strings.Join(e.investigationNotes, "\n")
		targetEntity := questionEntities[0]
		targetCount := strings.Count(notesJoined, targetEntity)

		// Find the most-mentioned entity across all entities we know about.
		bestEntity := targetEntity
		bestCount := targetCount
		// Also check entities from the graph that appear in notes.
		if e.searchResult != nil && e.searchResult.Graph != nil {
			for symName := range e.searchResult.Graph.SymbolDefs {
				if len(symName) < 6 {
					continue
				}
				// Skip if it's already one of the question entities.
				isQuestionEntity := false
				for _, qe := range questionEntities {
					if symName == qe || strings.Contains(qe, symName) || strings.Contains(symName, qe) {
						isQuestionEntity = true
						break
					}
				}
				if isQuestionEntity {
					continue
				}
				cnt := strings.Count(notesJoined, symName)
				if cnt > bestCount {
					bestEntity = symName
					bestCount = cnt
				}
			}
		}

		if bestEntity != targetEntity && bestCount > targetCount*2 && bestCount >= 3 {
			digest.WriteString("## WARNING: Potential Focus Misalignment\n\n")
			fmt.Fprintf(&digest, "Your question asks about **`%s`** but your evidence primarily discusses "+
				"**`%s`** (%d mentions vs %d mentions for the target). "+
				"Ensure your answer focuses on `%s`, not `%s`.\n\n",
				targetEntity, bestEntity, bestCount, targetCount,
				targetEntity, bestEntity)
		}
	}

	// Build cross-reference map: identify symbols that appear in 2+
	// evidence sets. These are the links in the evidence chain.
	if crossRefs := e.buildCrossReferenceMap(); crossRefs != "" {
		digest.WriteString(crossRefs)
	}

	// Enumeration completeness: show the LLM how many files were found
	// vs analyzed, so it can assess whether its list is exhaustive.
	if e.isEnumerationQuery {
		allDiscovered, allReadSet := extractFileCoverage(toolResults)
		enumCov := 0.0
		if len(allDiscovered) > 0 {
			enumCov = float64(len(allReadSet)) / float64(len(allDiscovered)) * 100
		}
		digest.WriteString("## Enumeration Completeness\n\n")
		fmt.Fprintf(&digest, "This is an enumeration query. Files found: %d, analyzed: %d (%.0f%% coverage).\n",
			len(allDiscovered), len(allReadSet), enumCov)
		var enumUnread []string
		for _, f := range allDiscovered {
			if !allReadSet[f] {
				enumUnread = append(enumUnread, f)
			}
		}
		if len(enumUnread) > 0 {
			digest.WriteString("**UNREAD files (your answer may be incomplete):**\n")
			for _, f := range enumUnread {
				digest.WriteString("- " + f + "\n")
			}
		} else {
			digest.WriteString("All discovered files were analyzed — good coverage.\n")
		}
		digest.WriteString("\nEnsure your answer explicitly states the total count of items found.\n\n")
	}

	// Include a compact file list so the LLM knows what was read.
	_, readSet := extractFileCoverage(toolResults)

	// Inject concrete values extracted from short methods/functions
	// across all relevant files (pre-scanned, read, and scored).
	// Also builds resolution chains that trace through symbol references.
	// Track what programmatic layers fired for adaptive instructions.
	hasConcreteValues := false
	cvResult := e.getConcreteValuesCached(ctx.RepoRoot, readSet)
	cv := cvResult.markdown
	if cv != "" {
		hasConcreteValues = true
		digest.WriteString(cv)
	}
	// Merge concrete values evidence into structured evidence so it
	// flows to finalizer regardless of synthesis success.
	if len(cvResult.evidence) > 0 {
		e.structuredEvidence = mergeEvidenceItems(e.structuredEvidence, cvResult.evidence)
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

	// Cross-validate LLM evidence against programmatic concrete values.
	// Surface conflicts where the LLM's claims contradict ground truth
	// extracted directly from source code.
	if len(e.investigationNotes) > 0 && hasConcreteValues {
		if conflicts := crossValidateEvidence(e.investigationNotes, cv); conflicts != "" {
			digest.WriteString(conflicts)
		}
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

	// When the question asks for an itemized listing ("哪几种", "具体有哪些",
	// "what strategies", "what are the"), add instruction to list each item
	// individually with its exact trigger condition. This prevents the LLM
	// from over-summarizing multiple distinct items into vague categories.
	if detectDetailListingIntent(e.userQuestion) {
		digest.WriteString("\n**Step 4 — Itemize your answer.** The question asks for specific items. " +
			"List EACH distinct item as a numbered entry with:\n")
		digest.WriteString("- Its exact name or identifier\n")
		digest.WriteString("- Its trigger condition or defining characteristic\n")
		digest.WriteString("- A file:line citation\n")
		digest.WriteString("Do NOT merge multiple distinct items into one broad category. " +
			"If there are 6 strategies, list all 6 separately — not 3 categories of 2.\n")
	}

	// Global budget: cap the synthesis prompt to avoid exceeding LLM
	// context windows. gpt-4o ≈ 128K tokens ≈ 400KB; allow ~120KB for
	// the synthesis user message (the system prompt and overhead take the
	// rest). If over budget, progressively truncate lower-priority
	// sections from the bottom of the digest.
	const synthBudgetBytes = 120_000
	result := digest.String()
	if len(result) > synthBudgetBytes {
		logging.Debug("[explorer] synthesis prompt %d bytes exceeds budget %d, truncating", len(result), synthBudgetBytes)
		result = truncateSynthesisPrompt(result, synthBudgetBytes)
	}
	return result, true
}

// truncateSynthesisPrompt progressively removes lower-priority sections
// from the synthesis prompt to fit within the byte budget. Sections are
// removed from lowest to highest priority.
func truncateSynthesisPrompt(prompt string, budget int) string {
	if len(prompt) <= budget {
		return prompt
	}
	// Sections to remove, in order of lowest to highest priority.
	// Each entry is the markdown heading that starts the section.
	lowPrioritySections := []string{
		"## WARNING: Unread Important Files",
		"## Unresolved Conditions",
		"## Negative Evidence (Absent Patterns)",
		"## WARNING: Potential Focus Misalignment",
		"## Enumeration Completeness",
		"## Cross-References Between Evidence Sets",
		"### Type Hierarchy Chains",
		"### Resolution Chains",
	}
	for _, heading := range lowPrioritySections {
		if len(prompt) <= budget {
			break
		}
		prompt = removeMarkdownSection(prompt, heading)
	}
	// If still over budget, hard-truncate the evidence catalog notes.
	if len(prompt) > budget {
		prompt = prompt[:budget] + "\n\n... [synthesis prompt truncated to fit context window]\n"
	}
	return prompt
}

// removeMarkdownSection removes a markdown section (heading + content up to
// the next heading of equal or higher level) from text.
func removeMarkdownSection(text, heading string) string {
	idx := strings.Index(text, heading)
	if idx < 0 {
		return text
	}
	// Determine the heading level (count leading #).
	level := 0
	for _, ch := range heading {
		if ch == '#' {
			level++
		} else {
			break
		}
	}
	// Find the end of this section: next heading of same or higher level.
	rest := text[idx+len(heading):]
	endMarker := strings.Repeat("#", level) + " "
	// Also stop at headings with fewer # (higher level).
	endIdx := -1
	for i := 0; i < len(rest); i++ {
		if i == 0 || (i > 0 && rest[i-1] == '\n') {
			remaining := rest[i:]
			for l := 1; l <= level; l++ {
				prefix := strings.Repeat("#", l) + " "
				if strings.HasPrefix(remaining, prefix) {
					endIdx = i
					break
				}
			}
			if endIdx >= 0 {
				break
			}
			_ = endMarker // suppress unused
		}
	}
	if endIdx < 0 {
		// Section runs to end of text.
		return text[:idx]
	}
	return text[:idx] + rest[endIdx:]
}

// buildCrossReferenceMap scans investigation notes for symbol names
// from the repo_map graph and identifies symbols that appear in 2+
// different notes. These "bridge entities" are the connective tissue
// for multi-hop reasoning — they tell the LLM which analyses to
// chain together.
//
// Each bridge carries directional information: where the symbol is
// defined, which evidence sets define vs. use it, and for relation-
// based bridges, the exact relationship verb (calls, references,
// uses_type). This lets synthesis trace chains in the right direction
// instead of guessing which end is the source.
func (e *explorerEvaluator) buildCrossReferenceMap() string {
	if crossRefs := buildCrossReferenceMapFromEvidence(e.structuredEvidence, e.flowFindings); crossRefs != "" {
		return crossRefs
	}
	if e.searchResult == nil || e.searchResult.Graph == nil || len(e.investigationNotes) < 2 {
		return ""
	}
	graph := e.searchResult.Graph

	// For each symbol in the graph, check which notes mention it.
	type symbolRef struct {
		name     string
		noteIdxs []int    // 0-based indices into investigationNotes
		relKinds []string // relation kinds connecting this symbol across files
		defFile  string   // file where the symbol is defined (for single-symbol bridges)
		directed bool     // true for relation-based bridges (From→To)
	}
	bridgeMap := make(map[string]*symbolRef)

	// Build symbol → definition file index for directionality annotation.
	symDefFile := make(map[string]string) // symbol name → defining file
	for symName, defs := range graph.SymbolDefs {
		if len(defs) > 0 {
			symDefFile[symName] = defs[0].File
		}
	}

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
			bridgeMap[symName] = &symbolRef{
				name:     symName,
				noteIdxs: mentioned,
				defFile:  symDefFile[symName],
			}
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

	// Relation kind → human-readable directional verb.
	relVerb := map[string]string{
		"call":       "calls",
		"reference":  "references",
		"type_usage": "uses type",
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
			// Create a directed bridge with the relationship verb.
			verb := relVerb[rel.Kind]
			if verb == "" {
				verb = rel.Kind
			}
			key := fromSym + "→" + toSym
			if br, ok := bridgeMap[key]; ok {
				// Add relation kind if not already present.
				hasKind := false
				for _, k := range br.relKinds {
					if k == verb {
						hasKind = true
						break
					}
				}
				if !hasKind {
					br.relKinds = append(br.relKinds, verb)
				}
			} else {
				noteIdxs := []int{fromNote, toNote}
				sort.Ints(noteIdxs)
				bridgeMap[key] = &symbolRef{
					name:     fromSym + " → " + toSym,
					noteIdxs: noteIdxs,
					relKinds: []string{verb},
					directed: true,
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

	// Sort bridges: directed relations first (more actionable), then by
	// number of notes they span (most connected first), then alphabetically.
	sort.Slice(bridges, func(i, j int) bool {
		// Directed bridges before single-symbol bridges.
		if bridges[i].directed != bridges[j].directed {
			return bridges[i].directed
		}
		if len(bridges[i].noteIdxs) != len(bridges[j].noteIdxs) {
			return len(bridges[i].noteIdxs) > len(bridges[j].noteIdxs)
		}
		return bridges[i].name < bridges[j].name
	})

	// Adaptive cap: scale with investigation complexity.
	bridgeCap := 15
	if len(e.allScoredFiles) > 10 {
		bridgeCap = 20
	}
	if len(bridges) > bridgeCap {
		bridges = bridges[:bridgeCap]
	}

	var b strings.Builder
	b.WriteString("## Cross-References Between Evidence Sets\n\n")
	b.WriteString("These symbols link your evidence sets. Directed entries (A —[verb]→ B) show ")
	b.WriteString("the code-level relationship; trace them to connect facts across files:\n\n")
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

		var entry string
		if br.directed && len(br.relKinds) > 0 {
			// Directed bridge: "SymA —[calls]→ SymB"
			entry = fmt.Sprintf("- **%s** —[%s]→ %s",
				br.name, strings.Join(br.relKinds, ", "),
				strings.Join(refs, ", "))
		} else {
			// Single-symbol bridge: "SymName" with definition site.
			entry = fmt.Sprintf("- **%s** — %s", br.name, strings.Join(refs, ", "))
			if br.defFile != "" {
				entry += fmt.Sprintf(" (defined in %s)", br.defFile)
			}
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
// concreteValuesResult holds both the markdown for synthesis prompt and
// structured evidence items for downstream stages.
type concreteValuesResult struct {
	markdown string
	evidence []types.EvidenceItem
}

// getConcreteValuesCached builds concrete values once per Execute and
// caches the result for reuse by both the dataflow-skip gate (T1.1) and
// the SynthesisPrompt section. Subsequent calls return the cached value
// regardless of readSet drift; this is safe because both call sites run
// near the end of the loop with effectively the same toolResults.
func (e *explorerEvaluator) getConcreteValuesCached(repoRoot string, readSet map[string]bool) concreteValuesResult {
	if e.cachedConcreteValues != nil {
		return *e.cachedConcreteValues
	}
	r := e.buildConcreteValuesSection(repoRoot, readSet)
	e.cachedConcreteValues = &r
	return r
}

func (e *explorerEvaluator) buildConcreteValuesSection(repoRoot string, readSet map[string]bool) concreteValuesResult {
	if e.searchResult == nil || e.searchResult.Graph == nil {
		return concreteValuesResult{}
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
	logging.Debug("[explorer] concrete values: scanning %d files (preScanned=%d, scored=%d, readSet=%d)",
		len(filesToScan), len(e.preScannedFiles), len(e.allScoredFiles), len(readSet))
	// Log which high-value files are in the scan set
	for _, key := range []string{"sub_explorer", "subagent.go"} {
		for f := range filesToScan {
			if strings.Contains(f, key) {
				logging.Debug("[explorer] concrete values: %s in filesToScan", f)
			}
		}
	}
	// Per-file diagnostic counters to distinguish extraction misses
	// (graph lookup failure, no symbols, all skipped by filter) from
	// filter drops later in the pipeline.
	type scanStats struct {
		graphMiss    bool
		symTotal     int
		symWrongKind int
		symNoEndLine int
		symOversize  int
		symScanned   int
	}
	fileStats := make(map[string]*scanStats)
	for file := range filesToScan {
		st := &scanStats{}
		fileStats[file] = st
		fi, ok := graph.FileIndex[file]
		if !ok {
			st.graphMiss = true
			continue
		}
		st.symTotal = len(fi.Symbols)
		for _, sym := range fi.Symbols {
			if sym.Kind != "method" && sym.Kind != "function" {
				st.symWrongKind++
				continue
			}
			if sym.EndLine == 0 {
				st.symNoEndLine++
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
				st.symOversize++
				continue
			}
			st.symScanned++

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
	// Dump per-file scan stats for the top-scored files. This surfaces
	// Bug A: when a file is in filesToScan but the graph either doesn't
	// index it (graphMiss), reports zero symbols, or all symbols are
	// filtered out as wrong-kind / no-endline / oversize, no concrete
	// values will flow downstream regardless of the filter.
	for i, f := range e.allScoredFiles {
		if i >= 15 {
			break
		}
		st := fileStats[f]
		if st == nil {
			logging.Debug("[explorer]   scan-stats[%02d] %s → NOT in filesToScan", i, f)
			continue
		}
		logging.Debug("[explorer]   scan-stats[%02d] %s → graphMiss=%v symTotal=%d wrongKind=%d noEndLine=%d oversize=%d scanned=%d",
			i, f, st.graphMiss, st.symTotal, st.symWrongKind, st.symNoEndLine, st.symOversize, st.symScanned)
	}
	// Per-file count of ALL extracted values (pre-filter). Pair this
	// with the post-filter per-file count below to distinguish
	// extraction misses from filter drops.
	{
		perFileAll := make(map[string]int, len(allValues))
		for _, v := range allValues {
			perFileAll[v.file]++
		}
		type fc2 struct {
			file  string
			count int
		}
		var fcAll []fc2
		for f, c := range perFileAll {
			fcAll = append(fcAll, fc2{f, c})
		}
		sort.Slice(fcAll, func(i, j int) bool { return fcAll[i].count > fcAll[j].count })
		for i, x := range fcAll {
			if i >= 15 {
				break
			}
			logging.Debug("[explorer]   allValues-by-file[%02d] %s → %d values", i, x.file, x.count)
		}
	}
	if len(allValues) == 0 {
		return concreteValuesResult{}
	}

	// Build pre-scanned set for filtering.
	preScannedSet := make(map[string]bool, len(e.preScannedFiles))
	for _, f := range e.preScannedFiles {
		preScannedSet[f] = true
	}
	// All keyword-search-scored files are question-relevant by
	// construction; their short returns are deterministic facts and
	// must not be gated on whether the LLM happened to mention the
	// receiver in its investigation notes.
	allScoredSet := make(map[string]bool, len(e.allScoredFiles))
	for _, f := range e.allScoredFiles {
		allScoredSet[f] = true
	}

	// Filter to keep only values relevant to the investigation:
	// 1. Registrations — always kept (rule A)
	// 2. Short string returns from pre-scanned/read/scored files — always kept (rule B1)
	// 3. Short string returns from other files — only if receiver is in notes (rule B2)
	// 4. Values referencing symbols from the investigation notes (rule C)
	var relevant []concreteValue
	// Per-rule counters for observability: split B1 by which file-set
	// triggered retention so P1 (allScoredSet path) impact is visible.
	var cntA, cntB1Read, cntB1PreScan, cntB1Scored, cntB2, cntC, cntLongSkip int
	for _, v := range allValues {
		if strings.Contains(v.kind, "binds") || v.kind == "maps" || v.kind == "config" || v.kind == "decorates" || v.kind == "assigns" {
			relevant = append(relevant, v)
			cntA++
			continue
		}
		if v.kind == "returns" {
			isStringLit := len(v.value) >= 2 && (v.value[0] == '"' || v.value[0] == '\'')
			isBoolOrNil := v.value == "true" || v.value == "false" || v.value == "nil" || v.value == "null"
			// Skip long description strings (> 80 chars).
			if isStringLit && len(v.value) > 80 {
				cntLongSkip++
				continue
			}
			// Always keep short string/bool returns from any
			// question-relevant file (read, pre-scanned, or
			// keyword-search-scored). These are deterministic facts
			// and must not depend on LLM notes content.
			if isStringLit || isBoolOrNil {
				if readSet[v.file] {
					relevant = append(relevant, v)
					cntB1Read++
					continue
				}
				if preScannedSet[v.file] {
					relevant = append(relevant, v)
					cntB1PreScan++
					continue
				}
				if allScoredSet[v.file] {
					relevant = append(relevant, v)
					cntB1Scored++
					continue
				}
				// For other files, require receiver/method in notes.
				if strings.Contains(notesJoined, v.receiver) ||
					strings.Contains(notesJoined, v.method) {
					relevant = append(relevant, v)
					cntB2++
					continue
				}
			}
		}
		// Keep values referencing noted symbols
		for _, word := range strings.Fields(v.value) {
			cleaned := strings.Trim(word, "(){}[]&*,;")
			if len(cleaned) >= 6 && strings.Contains(notesJoined, cleaned) {
				relevant = append(relevant, v)
				cntC++
				break
			}
		}
	}

	logging.Debug("[explorer] concrete values filter: total=%d relevant=%d (A/reg=%d, B1/read=%d, B1/preScan=%d, B1/scored=%d, B2/notes-recv=%d, C/notes-word=%d, longSkip=%d)",
		len(allValues), len(relevant), cntA, cntB1Read, cntB1PreScan, cntB1Scored, cntB2, cntC, cntLongSkip)

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
		logging.Debug("[explorer] concrete values tracing pass %d: +%d values (total=%d)", pass+1, added, len(relevant))
		if added == 0 {
			break
		}
	}

	logging.Debug("[explorer] concrete values: %d relevant after multi-pass tracing", len(relevant))

	if len(relevant) == 0 {
		return concreteValuesResult{}
	}

	// Sort by usefulness: bindings first (they anchor chains), then
	// short string returns (Name/Type), then booleans, then longer values.
	sort.Slice(relevant, func(i, j int) bool {
		scoreVal := func(v concreteValue) int {
			if strings.Contains(v.kind, "binds") || v.kind == "maps" || v.kind == "config" || v.kind == "decorates" || v.kind == "assigns" {
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

	// Dump a sample of the sorted relevant set so that we can verify
	// which concrete values made it through the filter (independent of
	// the markdown cap, which truncates the synthesis table but not this
	// log).
	for i, v := range relevant {
		if i >= 40 {
			break
		}
		logging.Debug("[explorer]   relevant[%02d] %s:%d %s %s %s", i, v.file, v.line, v.method, v.kind, v.value)
	}
	// Per-file count of relevant values — helps diagnose cases where a
	// file is in filesToScan but its concrete values never make it into
	// the relevant set (extraction miss vs. filter drop).
	perFile := make(map[string]int, len(relevant))
	for _, v := range relevant {
		perFile[v.file]++
	}
	type fc struct {
		file  string
		count int
	}
	var fcList []fc
	for f, c := range perFile {
		fcList = append(fcList, fc{f, c})
	}
	sort.Slice(fcList, func(i, j int) bool { return fcList[i].count > fcList[j].count })
	for i, x := range fcList {
		if i >= 15 {
			break
		}
		logging.Debug("[explorer]   relevant-by-file[%02d] %s → %d values", i, x.file, x.count)
	}

	// Save the full relevant set for evidence generation BEFORE capping.
	// The cap controls synthesis markdown size, but evidence items flow
	// through a separate pipeline (StageOutput → finalizer) with its own
	// ranking and limit, and must not be truncated by the markdown budget.
	allRelevantForEvidence := relevant

	// Adaptive cap: controls markdown table size in synthesis prompt.
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

	// Decision block extraction: for long functions the LLM has read,
	// detect independent logic blocks (comment-header + return-terminated).
	// This tells synthesis exactly how many distinct strategies/cases/steps
	// a function contains, preventing the LLM from merging N items into
	// fewer categories. Only fires for functions with ≥3 blocks — below
	// that threshold the structure is simple enough for the LLM to handle.
	var blockSections []string
	for file := range readSet {
		// Normalize path: readSet may contain "./path" while graph uses "path".
		normalizedFile := strings.TrimPrefix(file, "./")
		fi, ok := graph.FileIndex[normalizedFile]
		if !ok {
			continue
		}
		absPath := filepath.Join(repoRoot, normalizedFile)
		allLines := loadFileLines(absPath)
		if allLines == nil {
			continue
		}
		for _, sym := range fi.Symbols {
			if sym.Kind != "method" && sym.Kind != "function" {
				continue
			}
			bodyLines := sym.EndLine - sym.Line
			if bodyLines < 50 || sym.EndLine == 0 {
				continue
			}
			blocks := extractDecisionBlocks(allLines, sym.Line, sym.EndLine)
			if blocks == nil {
				if bodyLines >= 100 {
					logging.Debug("[explorer] decision blocks: %s.%s (%d lines, L%d-%d) → nil blocks",
						sym.Receiver, sym.Name, bodyLines, sym.Line, sym.EndLine)
				}
				continue
			}
			logging.Debug("[explorer] decision blocks: %s.%s → %d blocks detected",
				sym.Receiver, sym.Name, len(blocks))
			owner := sym.Receiver
			if owner == "" {
				owner = sym.Parent
			}
			qualName := sym.Name
			if owner != "" {
				qualName = owner + "." + sym.Name
			}
			var entry strings.Builder
			fmt.Fprintf(&entry, "**`%s`** (%s:%d-%d) — %d independent blocks:\n\n",
				qualName, normalizedFile, sym.Line, sym.EndLine, len(blocks))
			entry.WriteString("| # | Lines | Label |\n")
			entry.WriteString("|---|-------|-------|\n")
			for i, blk := range blocks {
				fmt.Fprintf(&entry, "| %d | %d-%d | %s |\n",
					i+1, blk.startLine, blk.endLine, blk.label)
			}
			blockSections = append(blockSections, entry.String())
		}
	}
	if len(blockSections) > 0 {
		logging.Debug("[explorer] decision blocks: emitting %d function entries to synthesis", len(blockSections))
		b.WriteString("### Decision Blocks (programmatically detected)\n\n")
		b.WriteString("These functions contain multiple INDEPENDENT logic blocks. " +
			"Each block is a separate strategy/case/step — do NOT merge them.\n\n")
		for _, sec := range blockSections {
			b.WriteString(sec)
			b.WriteString("\n")
		}
	}

	// Build resolution chains: when value A mentions type T, and
	// there's a value from T.SomeMethod, chain them. This covers:
	//   - Register(NewFoo) → Foo.Name() returns "bar"
	//   - returns NewFoo() → Foo.Name() returns "bar"
	//   - returns &Foo{} → Foo.Name() returns "bar"
	// Build resolution chains from the FULL relevant set (pre-cap)
	// so that chains like "RegisterDefaultSubAgents binds NewSubExplorer
	// → SubExplorer.Name returns explorer" are discovered even when
	// SubExplorer.Name is outside the top-25 markdown cap.
	var chains []string
	for _, v := range allRelevantForEvidence {
		// Skip values that don't reference other types.
		if v.kind != "returns" && !strings.Contains(v.kind, "binds") && v.kind != "maps" && v.kind != "config" && v.kind != "decorates" {
			continue
		}
		for _, rv := range allRelevantForEvidence {
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
	logging.Debug("[explorer] concrete values: built %d resolution chains (before cap)", len(chains))
	for i, c := range chains {
		if i >= 10 {
			break
		}
		logging.Debug("[explorer]   chain[%02d] %s", i, c)
	}
	// Save the full chain list for evidence generation BEFORE capping.
	// The cap controls the synthesis markdown table size; the evidence
	// pipeline (StageOutput → finalizer) has its own ranking/top-K and
	// must see every chain so that cross-boundary chains like
	// `RegisterDefaultSubAgents → SubExplorer.Name returns "explorer"`
	// can reach the answer identification layer even when the markdown
	// table is dominated by higher-scoring noise.
	allChainsForEvidence := chains
	// Adaptive cap for resolution chains (markdown table only).
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
	for _, v := range allRelevantForEvidence {
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

	// Build structured evidence items from the FULL relevant set (pre-cap).
	// These flow to StageOutput → BusContext → finalizer, independent
	// of whether synthesis succeeds. The downstream rankEvidenceByRelevance
	// + formatEvidenceItems(limit=18) handles its own selection.
	var cvEvidence []types.EvidenceItem
	for _, v := range allRelevantForEvidence {
		kind := types.EvidenceConcrete
		predicate := v.kind
		cvEvidence = append(cvEvidence, types.EvidenceItem{
			ID: types.StableEvidenceID(kind, v.method, predicate, v.value, "", v.file, v.line, v.line),
			Kind:      kind,
			Subject:   v.method,
			Predicate: predicate,
			Object:    v.value,
			Source:    v.file,
			LineStart: v.line,
			LineEnd:   v.line,
			Confidence: 0.95,
			Producer:  "concrete_values",
			Summary:   fmt.Sprintf("`%s()` %s %s", v.method, predicate, v.value),
		})
	}
	for _, c := range allChainsForEvidence {
		cvEvidence = append(cvEvidence, types.EvidenceItem{
			ID:         types.StableEvidenceID(types.EvidenceDataflowPath, c, "resolution_chain", "", "", "", 0, 0),
			Kind:       types.EvidenceDataflowPath,
			Subject:    c,
			Predicate:  "resolution_chain",
			Confidence: 0.9,
			Producer:   "concrete_values",
			Summary:    c,
		})
	}
	logging.Debug("[explorer] concrete values: %d chain evidence items (from %d uncapped chains)", len(allChainsForEvidence), len(allChainsForEvidence))

	return concreteValuesResult{markdown: b.String(), evidence: cvEvidence}
}

// decisionBlock represents one independent logic block inside a long function,
// delimited by a section-header comment and terminated by a return/break or
// the next section header. Used by the synthesis prompt to show the LLM how
// many distinct blocks a function contains, preventing over-summarization.
type decisionBlock struct {
	label     string // cleaned text from the section-header comment
	startLine int    // 1-based, line of the section-header comment
	endLine   int    // 1-based, line of the terminating return/break (or next header - 1)
}

// extractDecisionBlocks scans a function body for independent decision blocks.
// A block starts at a section-header comment (a comment line whose text begins
// with an uppercase letter, signaling a new logical section) and ends at the
// next early return/break at the same or shallower indent, or at the next
// section header.
//
// This is a cross-language heuristic: section-header comments are universal
// developer practice (Go //, Python #, Java //, Rust //, shell #, SQL --),
// and early return is the universal block terminator.
//
// Parameters:
//   - lines: the raw source lines of the file (0-indexed)
//   - funcStart, funcEnd: 1-based inclusive line range of the function body
//   - baseIndent: the indentation level of the function body's top-level
//     statements (auto-detected from the first non-blank line after funcStart)
//
// Returns nil if fewer than 3 blocks are found (not worth surfacing).
func extractDecisionBlocks(lines []string, funcStart, funcEnd int) []decisionBlock {
	if funcStart < 1 || funcEnd > len(lines) || funcEnd-funcStart < 10 {
		return nil
	}

	// Auto-detect base indent from the first non-blank body line.
	// Accept both tabs and spaces; use raw character count as indent depth.
	baseIndentLen := -1
	for i := funcStart; i < funcEnd && i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || trimmed == "{" || trimmed == "}" || trimmed == "BEGIN" || trimmed == "END;" {
			continue
		}
		raw := lines[i]
		baseIndentLen = len(raw) - len(strings.TrimLeft(raw, " \t"))
		break
	}
	if baseIndentLen < 0 {
		return nil
	}

	// Cross-language comment prefixes.
	commentPrefixes := []string{"//", "#", "--", "/*", "*"}

	lineIndentLen := func(line string) int {
		return len(line) - len(strings.TrimLeft(line, " \t"))
	}

	extractHeaderLabel := func(line string) (string, bool) {
		trimmed := strings.TrimSpace(line)
		for _, pfx := range commentPrefixes {
			if !strings.HasPrefix(trimmed, pfx) {
				continue
			}
			text := strings.TrimSpace(trimmed[len(pfx):])
			if len(text) > 0 && text[0] >= 'A' && text[0] <= 'Z' {
				label := text
				for _, sep := range []string{". ", ": ", " — ", " - "} {
					if idx := strings.Index(label, sep); idx > 0 && idx < 80 {
						label = label[:idx]
						break
					}
				}
				if len(label) > 80 {
					label = label[:80]
				}
				return label, true
			}
		}
		return "", false
	}

	// Detect section-header comments: a comment line starting with an
	// uppercase letter, preceded by a blank line (or closing brace or
	// function start). This is the strongest cross-language signal for
	// "new logical section" — developers universally leave a blank line
	// before a new section header but NOT before continuation comments.
	isSectionHeader := func(idx int) (string, bool) {
		line := lines[idx]
		indent := lineIndentLen(line)
		if indent < baseIndentLen || indent > baseIndentLen+4 {
			return "", false
		}
		label, ok := extractHeaderLabel(line)
		if !ok {
			return "", false
		}
		// Must be preceded by a blank line, closing brace, or function start.
		if idx > funcStart-1 {
			prevTrimmed := strings.TrimSpace(lines[idx-1])
			prevIndent := lineIndentLen(lines[idx-1])
			isStructuralBoundary := prevTrimmed == "" ||
				prevTrimmed == "}" || prevTrimmed == "};" ||
				prevTrimmed == "{" ||
				prevTrimmed == "BEGIN" || prevTrimmed == "END;" || prevTrimmed == "end" ||
				// Function opening line (at indent 0 or less than base): `func ... {`
				(strings.HasSuffix(prevTrimmed, "{") && prevIndent < baseIndentLen)
			if !isStructuralBoundary {
				return "", false
			}
		}
		return label, true
	}

	// An early-return line at the function body's indent level (or one deeper).
	isBlockTerminator := func(line string) bool {
		trimmed := strings.TrimSpace(line)
		indent := lineIndentLen(line)
		if indent < baseIndentLen || indent > baseIndentLen+4 {
			return false
		}
		for _, kw := range []string{"return ", "return\t", "break", "raise ", "throw ", "RAISE ", "THROW "} {
			if strings.HasPrefix(trimmed, kw) {
				return true
			}
		}
		if trimmed == "return" || trimmed == "return;" {
			return true
		}
		return false
	}

	var blocks []decisionBlock
	var current *decisionBlock

	for i := funcStart - 1; i < funcEnd && i < len(lines); i++ {
		lineNo := i + 1

		if label, ok := isSectionHeader(i); ok {
			if current != nil {
				current.endLine = lineNo - 1
				blocks = append(blocks, *current)
			}
			current = &decisionBlock{label: label, startLine: lineNo}
			continue
		}

		if current != nil && isBlockTerminator(lines[i]) {
			current.endLine = lineNo
			blocks = append(blocks, *current)
			current = nil
		}
	}
	if current != nil {
		current.endLine = funcEnd
		blocks = append(blocks, *current)
	}

	// Filter: keep only blocks that contain a return/break/throw/raise
	// terminator within their line range. Blocks without a terminator
	// are setup/bookkeeping code, not independent decision paths.
	var filtered []decisionBlock
	for _, blk := range blocks {
		hasTerminator := false
		for li := blk.startLine - 1; li < blk.endLine && li < len(lines); li++ {
			if isBlockTerminator(lines[li]) {
				hasTerminator = true
				break
			}
		}
		if hasTerminator {
			filtered = append(filtered, blk)
		}
	}

	if len(filtered) < 3 {
		return nil
	}
	return filtered
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

		// Pattern: variable assignment creating a new composite value.
		// Captures "varName := []Type{elem, ...}" and "varName := Type{...}"
		// which establish what a variable IS (important for control flow
		// reasoning — e.g., synthMessages is a NEW slice, not accumulated).
		if strings.Contains(trimmed, ":=") {
			if idx := strings.Index(trimmed, ":="); idx > 0 {
				lhs := strings.TrimSpace(trimmed[:idx])
				rhs := strings.TrimSpace(trimmed[idx+2:])
				// Only capture composite literals (struct/slice/map/array).
				if len(rhs) > 0 && (strings.Contains(rhs, "{") || strings.HasPrefix(rhs, "[]")) {
					// Extract the variable name (last identifier on LHS).
					parts := strings.Fields(lhs)
					varName := parts[len(parts)-1]
					if len(varName) >= 2 && len(rhs) < 80 {
						results = append(results, concreteValueEntry{
							kind:  "assigns",
							value: varName + " := " + rhs,
						})
					}
				}
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
			// grep results come in two formats:
			//   files_only=true:  one path per line ("internal/agent/explorer.go")
			//   files_only=false: "path:linenum: content" per line
			// Both formats are parsed to extract the file path. The
			// first line may be a summary header like "[grep: N matching ...]".
			for _, line := range strings.Split(r.Summary, "\n") {
				path := strings.TrimSpace(line)
				if path == "" || path[0] == '[' {
					continue
				}
				// Normalize: strip leading ./
				path = strings.TrimPrefix(path, "./")
				// Detect "path:linenum: content" format from files_only=false.
				// A line like "internal/agent/explorer.go:157: func ..." should
				// extract just "internal/agent/explorer.go". We look for ":digits:"
				// pattern which distinguishes line-level output from plain paths.
				if colonIdx := strings.Index(path, ":"); colonIdx > 0 {
					afterColon := path[colonIdx+1:]
					// Check if what follows the first colon starts with digits
					// (line number), indicating files_only=false format.
					isLineLevel := false
					for j := 0; j < len(afterColon); j++ {
						if afterColon[j] >= '0' && afterColon[j] <= '9' {
							isLineLevel = true
						} else {
							break
						}
					}
					if isLineLevel {
						path = path[:colonIdx]
					}
				}
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

// truncatedFileInfo describes a file whose read_file result was truncated.
type truncatedFileInfo struct {
	path       string
	linesRead  int // lines actually shown
	totalLines int // total lines in file
}

// detectTruncatedUngrepped scans tool history for read_file results
// that were truncated (showing only a portion of a large file) and
// checks whether the LLM has already grepped those files with line-level
// output (files_only=false). Returns truncated files and a set of
// files that have been line-grepped.
func detectTruncatedUngrepped(history []types.ToolResult) ([]truncatedFileInfo, map[string]bool) {
	// Track the max lines read and total lines for each file.
	type fileRead struct {
		maxLineRead int
		totalLines  int
	}
	reads := make(map[string]*fileRead)

	// Track files grepped with line-level output.
	grepped := make(map[string]bool)

	for _, r := range history {
		if !r.Success {
			continue
		}
		switch r.ToolName {
		case "read_file":
			first := strings.SplitN(r.Summary, "\n", 2)[0]
			// Parse "[path: showing lines X-Y of Z total]"
			if !strings.HasPrefix(first, "[") {
				continue
			}
			colonIdx := strings.Index(first, ": showing lines ")
			if colonIdx < 1 {
				continue
			}
			path := first[1:colonIdx]
			rest := first[colonIdx+len(": showing lines "):]
			dashIdx := strings.Index(rest, "-")
			ofIdx := strings.Index(rest, " of ")
			if dashIdx < 0 || ofIdx < 0 {
				continue
			}
			endLine, err1 := strconv.Atoi(rest[dashIdx+1 : ofIdx])
			totalStr := strings.TrimSuffix(strings.TrimSuffix(rest[ofIdx+4:], "]"), " total")
			total, err2 := strconv.Atoi(strings.TrimSpace(totalStr))
			if err1 != nil || err2 != nil {
				continue
			}
			fr, ok := reads[path]
			if !ok {
				fr = &fileRead{}
				reads[path] = fr
			}
			if endLine > fr.maxLineRead {
				fr.maxLineRead = endLine
			}
			if total > fr.totalLines {
				fr.totalLines = total
			}

		case "grep":
			// Check if this grep targeted a specific file (path param)
			// and returned line-level results (not files_only).
			if !strings.HasPrefix(r.Summary, "[grep:") {
				continue
			}
			// Line-level grep results contain "matching lines" not "matching files".
			if strings.Contains(r.Summary, "matching lines") {
				// Extract the file path from the grep result lines.
				// When grep targets a single file, lines look like "NNN: content".
				// When grep targets a directory, lines look like "path:NNN: content".
				for _, line := range strings.Split(r.Summary, "\n") {
					line = strings.TrimSpace(line)
					if len(line) == 0 || line[0] == '[' {
						continue
					}
					if colonIdx := strings.Index(line, ":"); colonIdx > 0 {
						maybePath := line[:colonIdx]
						if strings.Contains(maybePath, "/") || strings.Contains(maybePath, ".") {
							grepped[maybePath] = true
						}
					}
				}
			}
		}
	}

	var result []truncatedFileInfo
	for path, fr := range reads {
		// File was truncated if the LLM didn't read to the end.
		if fr.totalLines > 500 && fr.maxLineRead < fr.totalLines {
			result = append(result, truncatedFileInfo{
				path:       path,
				linesRead:  fr.maxLineRead,
				totalLines: fr.totalLines,
			})
		}
	}
	return result, grepped
}

// extractQuestionEntities pulls code identifiers from a user question.
// Returns backtick-quoted identifiers first (most explicit), then
// CamelCase identifiers, then dotted identifiers. Used by focus
// alignment to detect when the LLM's evidence discusses a different
// entity than what the question asks about.
func extractQuestionEntities(question string) []string {
	seen := make(map[string]bool)
	var entities []string
	add := func(s string) {
		s = strings.Trim(s, "(){}[]")
		if len(s) < 3 || seen[s] {
			return
		}
		seen[s] = true
		entities = append(entities, s)
	}

	// 1. Backtick-quoted identifiers: `MyClass.doThing`
	rest := question
	for {
		start := strings.Index(rest, "`")
		if start < 0 {
			break
		}
		end := strings.Index(rest[start+1:], "`")
		if end < 0 {
			break
		}
		sym := rest[start+1 : start+1+end]
		add(sym)
		rest = rest[start+1+end+1:]
	}

	// 2. CamelCase identifiers (2+ uppercase-initial segments): SubAgent, BaseAgent
	inIdent := false
	identStart := 0
	for i := 0; i <= len(question); i++ {
		var c byte
		if i < len(question) {
			c = question[i]
		}
		isIdent := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '.'
		if isIdent && !inIdent {
			identStart = i
			inIdent = true
		} else if !isIdent && inIdent {
			token := question[identStart:i]
			// Count uppercase-initial segments (CamelCase detection).
			segments := 0
			for j := 0; j < len(token); j++ {
				if token[j] >= 'A' && token[j] <= 'Z' {
					if j == 0 || (token[j-1] >= 'a' && token[j-1] <= 'z') {
						segments++
					}
				}
			}
			if segments >= 2 && len(token) >= 6 {
				add(token)
			}
			// Dotted identifiers: Foo.Bar
			if strings.Contains(token, ".") && len(token) >= 5 {
				add(token)
			}
			inIdent = false
		}
	}
	return entities
}

// detectDetailListingIntent checks if a question asks for an itemized
// listing where each item should be described individually. This is
// broader than enumeration — it also covers "what strategies", "what
// are the steps", "哪几种" etc. where the answer should be a numbered
// list, not a prose summary.
func detectDetailListingIntent(question string) bool {
	lower := strings.ToLower(question)
	// Chinese patterns requesting itemized detail.
	for _, kw := range []string{"哪几种", "具体有哪些", "分别", "逐个", "排列", "每种"} {
		if strings.Contains(question, kw) {
			return true
		}
	}
	// English patterns.
	for _, kw := range []string{
		"what strategies", "what are the", "what steps",
		"how many different", "list each", "describe each",
		"what types of", "what kinds of",
	} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	// Enumeration queries are always detail-listing queries.
	return detectEnumerationIntent(question)
}

// detectEnumerationIntent checks if a question asks to list or enumerate
// all items of a certain type. This triggers stricter file coverage
// thresholds and enumeration completeness verification in synthesis.
//
// Supports Chinese and English enumeration patterns. The heuristic
// requires the enumeration keyword to appear in a context that suggests
// exhaustive listing (not just incidental use of "all").
func detectEnumerationIntent(question string) bool {
	lower := strings.ToLower(question)

	// Chinese enumeration keywords (high confidence).
	for _, kw := range []string{"所有", "每个", "全部", "哪些", "有哪些", "列出", "列举"} {
		if strings.Contains(question, kw) {
			return true
		}
	}

	// English enumeration patterns.
	// Direct enumeration verbs.
	for _, kw := range []string{"list all", "find all", "enumerate", "how many"} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	// "all X" where X is a noun — "all implementations", "all types"
	// Require "all" followed by a word, not "all errors are handled" pattern.
	for _, prefix := range []string{"all the ", "every ", "each "} {
		if strings.Contains(lower, prefix) {
			return true
		}
	}
	// "what are the Xs" — plural noun query
	if strings.Contains(lower, "what are") || strings.Contains(lower, "which ") {
		return true
	}
	return false
}

// partialReadHint describes a function/method that was partially read.
type partialReadHint struct {
	file       string
	symbolName string
	symbolKind string
	symStart   int     // symbol.Line (1-based)
	symEnd     int     // symbol.EndLine (1-based)
	readEnd    int     // max line the LLM read in this file
	coverage   float64 // fraction of function body covered (0.0-1.0)
}

// detectPartiallyReadSymbols checks whether any function/method in the
// read files was only partially covered by the LLM's read_file calls.
// It cross-references the read ranges (from banner parsing) against the
// symbol boundaries from the repo_map graph. Returns hints for functions
// where the LLM missed >20 lines and covered <80% of the body.
//
// This catches the common failure mode where the LLM reads a 40-line
// slice of a 300-line function and misses critical logic at the end.
func detectPartiallyReadSymbols(history []types.ToolResult, graph *repomap.Graph) []partialReadHint {
	if graph == nil {
		return nil
	}

	// Build per-file read ranges: track the max end line across all reads.
	type readRange struct {
		minStart int
		maxEnd   int
	}
	fileReads := make(map[string]*readRange)

	for _, r := range history {
		if !r.Success || r.ToolName != "read_file" {
			continue
		}
		first := strings.SplitN(r.Summary, "\n", 2)[0]
		if !strings.HasPrefix(first, "[") {
			continue
		}
		colonIdx := strings.Index(first, ": showing lines ")
		if colonIdx < 1 {
			continue
		}
		path := first[1:colonIdx]
		rest := first[colonIdx+len(": showing lines "):]
		dashIdx := strings.Index(rest, "-")
		ofIdx := strings.Index(rest, " of ")
		if dashIdx < 0 || ofIdx < 0 {
			continue
		}
		startLine, err1 := strconv.Atoi(rest[:dashIdx])
		endLine, err2 := strconv.Atoi(rest[dashIdx+1 : ofIdx])
		if err1 != nil || err2 != nil {
			continue
		}
		if rr, ok := fileReads[path]; ok {
			if startLine < rr.minStart {
				rr.minStart = startLine
			}
			if endLine > rr.maxEnd {
				rr.maxEnd = endLine
			}
		} else {
			fileReads[path] = &readRange{minStart: startLine, maxEnd: endLine}
		}
	}

	if len(fileReads) == 0 {
		return nil
	}

	var hints []partialReadHint
	for path, rr := range fileReads {
		fi, ok := graph.FileIndex[path]
		if !ok {
			continue
		}
		for _, sym := range fi.Symbols {
			if sym.Kind != "function" && sym.Kind != "method" {
				continue
			}
			if sym.EndLine == 0 || sym.EndLine-sym.Line < 10 {
				continue // skip trivial functions
			}
			// Check if this symbol overlaps with the read range but was
			// not fully covered.
			if sym.Line > rr.maxEnd || sym.EndLine <= rr.maxEnd {
				continue // entirely outside read range, or fully covered
			}
			// Symbol was partially read: sym.Line <= rr.maxEnd < sym.EndLine
			bodyLines := sym.EndLine - sym.Line + 1
			overlapStart := sym.Line
			if rr.minStart > overlapStart {
				overlapStart = rr.minStart
			}
			overlapEnd := rr.maxEnd
			if overlapEnd > sym.EndLine {
				overlapEnd = sym.EndLine
			}
			overlapLines := overlapEnd - overlapStart + 1
			if overlapLines < 0 {
				overlapLines = 0
			}
			cov := float64(overlapLines) / float64(bodyLines)

			// Only report if coverage < 80% AND missing > 20 lines.
			unreadLines := sym.EndLine - rr.maxEnd
			if cov < 0.8 && unreadLines > 20 {
				qualName := sym.Name
				if sym.Receiver != "" {
					qualName = sym.Receiver + "." + sym.Name
				} else if sym.Parent != "" {
					qualName = sym.Parent + "." + sym.Name
				}
				hints = append(hints, partialReadHint{
					file:       path,
					symbolName: qualName,
					symbolKind: sym.Kind,
					symStart:   sym.Line,
					symEnd:     sym.EndLine,
					readEnd:    rr.maxEnd,
					coverage:   cov,
				})
			}
		}
	}

	// Sort by coverage ascending (worst coverage first).
	sort.Slice(hints, func(i, j int) bool {
		return hints[i].coverage < hints[j].coverage
	})
	// Cap at 5 hints to avoid overwhelming the LLM.
	if len(hints) > 5 {
		hints = hints[:5]
	}
	return hints
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
	// Variable assignment creating new composite values:
	//   Go:     varName := Type{...} or varName := []Type{...}
	//   JS/TS:  const x = { ... } or const x = [ ... ]
	//   Python: x = ClassName(...)
	// These are evidence because they establish what a variable IS
	// (e.g., synthMessages is a NEW slice, not accumulated messages).
	if strings.Contains(trimmed, ":=") || strings.Contains(trimmed, " = ") {
		rhs := trimmed
		if idx := strings.Index(trimmed, ":="); idx >= 0 {
			rhs = strings.TrimSpace(trimmed[idx+2:])
		} else if idx := strings.Index(trimmed, " = "); idx >= 0 {
			rhs = strings.TrimSpace(trimmed[idx+3:])
		}
		// RHS creates a new composite value (struct/slice/map/array literal).
		if strings.Contains(rhs, "{") || strings.HasPrefix(rhs, "[") ||
			strings.HasPrefix(rhs, "[]") {
			return true
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

// crossValidateEvidence compares LLM-generated [DIRECT] and [REGISTRATION]
// evidence against the programmatically extracted Concrete Values table.
// When the same method appears in both with contradictory facts, a conflict
// is surfaced so the synthesis LLM can resolve it using source code as
// ground truth.
//
// This addresses a systemic weakness: the LLM can misread code (e.g.,
// reporting "returns true" when the code says "returns false"), and without
// cross-validation these errors propagate silently into the final answer.
//
// The comparison is language-agnostic: it extracts method names and core
// value assertions from both sources and compares them structurally.
func crossValidateEvidence(notes []string, concreteValuesSection string) string {
	if concreteValuesSection == "" {
		return ""
	}

	// Parse concrete values table: method → fact.
	// Table format: | file:line | `Method()` | kind value |
	type cvEntry struct {
		method string // lowercase, without parens
		fact   string // the full "kind value" column
	}
	var cvEntries []cvEntry
	cvByMethod := make(map[string]string) // lowercase method → fact
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
		if method != "" && fact != "" {
			key := strings.ToLower(method)
			cvByMethod[key] = fact
			cvEntries = append(cvEntries, cvEntry{method: key, fact: fact})
		}
	}
	if len(cvEntries) == 0 {
		return ""
	}

	// Parse LLM claims from [DIRECT] and [REGISTRATION] lines.
	// Format: - [DIRECT] `methodName` line N: <fact>
	// Format: - [REGISTRATION] `methodName` line N: <fact>
	type llmClaim struct {
		tag        string // "DIRECT" or "REGISTRATION"
		method     string // extracted method name, lowercase (for matching)
		methodOrig string // original case method name (for display)
		fact       string // the claim after "line N:"
		original   string // full original line for display
	}
	var claims []llmClaim
	for _, note := range notes {
		for _, line := range strings.Split(note, "\n") {
			trimmed := strings.TrimSpace(line)
			var tag string
			if strings.HasPrefix(trimmed, "- [DIRECT]") {
				tag = "DIRECT"
			} else if strings.HasPrefix(trimmed, "- [REGISTRATION]") {
				tag = "REGISTRATION"
			} else {
				continue
			}

			// Extract method name between backticks.
			btStart := strings.Index(trimmed, "`")
			if btStart < 0 {
				continue
			}
			btEnd := strings.Index(trimmed[btStart+1:], "`")
			if btEnd < 0 {
				continue
			}
			method := trimmed[btStart+1 : btStart+1+btEnd]
			method = strings.Trim(method, "()")

			// Extract fact: everything after "line N:" or the colon
			// following the method name.
			fact := ""
			afterMethod := trimmed[btStart+1+btEnd+1:]
			// Try "line N:" pattern first.
			if idx := strings.Index(afterMethod, ":"); idx >= 0 {
				fact = strings.TrimSpace(afterMethod[idx+1:])
			}
			if fact == "" {
				continue
			}

			claims = append(claims, llmClaim{
				tag:        tag,
				method:     strings.ToLower(method),
				methodOrig: method,
				fact:       fact,
				original:   trimmed,
			})
		}
	}
	if len(claims) == 0 {
		return ""
	}

	// Cross-validate: find claims where the same method has a concrete
	// value and check whether the facts agree or conflict.
	var conflicts []string
	seen := make(map[string]bool) // deduplicate by method
	for _, claim := range claims {
		if seen[claim.method] {
			continue
		}
		// Try exact match, then Type.Method partial matches.
		cvFact := ""
		if f, ok := cvByMethod[claim.method]; ok {
			cvFact = f
		} else {
			// Try matching just the method name part (e.g., claim has
			// "Name" and CV has "Foo.Name").
			for cvMethod, f := range cvByMethod {
				if strings.HasSuffix(cvMethod, "."+claim.method) ||
					claim.method == cvMethod {
					cvFact = f
					break
				}
			}
			if cvFact == "" {
				// Try the reverse: claim has "Foo.Name", CV has "Name".
				parts := strings.SplitN(claim.method, ".", 2)
				if len(parts) == 2 {
					if f, ok := cvByMethod[parts[1]]; ok {
						cvFact = f
					}
				}
			}
		}
		if cvFact == "" {
			continue // no matching concrete value to compare
		}
		seen[claim.method] = true

		// Compare the core assertions. Extract the value part from both.
		claimCore := normalizeValueAssertion(claim.fact)
		cvCore := normalizeValueAssertion(cvFact)
		if claimCore == "" || cvCore == "" {
			continue
		}
		if !valueAssertionsAgree(claimCore, cvCore) {
			conflicts = append(conflicts, fmt.Sprintf(
				"- **`%s`**: LLM claims \"%s\" but source code shows **%s**",
				claim.methodOrig, claim.fact, cvFact))
		}
	}

	if len(conflicts) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Evidence Conflicts (LLM vs. Source Code)\n\n")
	b.WriteString("The following claims from your investigation CONTRADICT the programmatic ")
	b.WriteString("evidence extracted directly from source code. The Concrete Values table is ")
	b.WriteString("ground truth — adjust your reasoning accordingly:\n\n")
	for _, c := range conflicts {
		b.WriteString(c + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

// normalizeValueAssertion extracts the core value from a fact string.
// Handles patterns like "returns true", "returns \"explorer\"",
// "binds NewFoo", "registers NewFoo and NewBar".
// Returns the normalized value for comparison, or "" if unparseable.
func normalizeValueAssertion(fact string) string {
	fact = strings.TrimSpace(fact)
	lower := strings.ToLower(fact)

	// Strip common prefixes to get to the value.
	for _, prefix := range []string{
		"returns ", "return ", "binds only ", "binds ",
		"maps ", "registers only ", "registers ",
		"decorates ", "config ",
	} {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(fact[len(prefix):])
		}
	}
	return fact
}

// valueAssertionsAgree checks if two normalized value assertions refer
// to the same thing. Handles quote style differences, whitespace, and
// simple boolean/nil equivalences.
func valueAssertionsAgree(a, b string) bool {
	// Normalize for comparison: lowercase, strip quotes, trim.
	normalize := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.Trim(s, "\"'`")
		s = strings.TrimSpace(s)
		return s
	}
	na, nb := normalize(a), normalize(b)
	if na == nb {
		return true
	}
	// One contains the other (handles "true" vs "true (always)")
	if strings.Contains(na, nb) || strings.Contains(nb, na) {
		return true
	}
	return false
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
