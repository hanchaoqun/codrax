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
	midLoopLastResultsLen     int                   // #34: allResults length at prev MidLoopCheck (used to infer current batch size)
	midLoopSerialStreak       int                   // #34: consecutive iters observed as single-call rounds
	midLoopParallelInjected   bool                  // #34: parallel-batching hint already pushed this dispatch
	primaryReadIter           int                   // df3-drift: iter at which a primary-entity file first entered readSet (0 = never)
	notesLenAtPrimaryRead     int                   // df3-drift: snapshot of len(investigationNotes) at primaryReadIter
}

// stripConversationPrefix returns only the "current request" portion
// of a REPL-assembled Objective string, stripping the conversation-
// memory prefix injected by `internal/repl/repl.go:dispatch`. When no
// marker is found (single-shot mode, or an empty-conversation REPL
// turn), returns the input unchanged.
//
// Added 2026-04-12 as part of the REPL-mode equivalence audit. The
// main explorer's entity regex (`extractRankingEntities(ctx.Objective)`)
// previously ran over the whole blob, pulling every CamelCase /
// snake_case / file-path token from the memory section as a bogus
// ERM entity. See memory/project_repl_equivalence_audit.md for the
// full diagnostic.
func stripConversationPrefix(s string) string {
	const marker = "## Current request\n"
	if idx := strings.Index(s, marker); idx >= 0 {
		return strings.TrimSpace(s[idx+len(marker):])
	}
	return s
}

func (e *explorerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	// CROSS-RUN STATE RESET (REPL turn boundary fix).
	//
	// The explorer evaluator is a process-lifetime singleton — state
	// fields like `investigationNotes`, `preScannedFiles`, `searchResult`,
	// `ermRequirements`, `fileSymbols`, `allScoredFiles` survive across
	// `Run()` calls. Within ONE Run() that's legitimate (intra-pipeline
	// explore → explore self-loop uses the `retry` branch below). But
	// across Run() calls — specifically REPL turn N+1 — these fields
	// carry previous-turn state into a completely unrelated question,
	// and the retry branch then treats the new question as a
	// continuation of the old one.
	//
	// Detection: compare the incoming CurrentTask to the cached one.
	// Within a single Run, CurrentTask is constant across all explore
	// dispatches (same task.Title). Across Run()s it's different (new
	// REPL turn → new analyzer output → new task title). When they
	// differ, reset every cross-Run field so the fresh-start branch
	// below fires cleanly.
	if ctx.CurrentTask != "" && ctx.CurrentTask != e.userQuestion {
		logging.Debug("[explorer] cross-run reset: current=%q != cached=%q", ctx.CurrentTask, e.userQuestion)
		e.investigationNotes = nil
		e.preScannedFiles = nil
		e.allScoredFiles = nil
		e.searchResult = nil
		e.ermRequirements = nil
		e.fileSymbols = nil
		e.phase0ExtraRound = false
		e.grepRedirectedFiles = nil
		e.idleStreakInDepth = 0
		e.lastToolResultCount = 0
		e.preScannedPushCount = 0
		e.lastPreScannedUnreadCount = 0
		e.broadenAttempts = 0
		e.midLoopLastResultsLen = 0
		e.midLoopSerialStreak = 0
		e.midLoopParallelInjected = false
		e.primaryReadIter = 0
		e.notesLenAtPrimaryRead = 0
	}

	e.userQuestion = ctx.CurrentTask
	e.repoRoot = ctx.RepoRoot
	e.isEnumerationQuery = detectEnumerationIntent(ctx.CurrentTask)
	e.structuredEvidence = nil
	e.flowFindings = nil
	e.cachedConcreteValues = nil
	e.midLoopLastInjectIter = -10
	e.midLoopLastResultsLen = 0
	e.midLoopSerialStreak = 0
	e.midLoopParallelInjected = false
	e.primaryReadIter = 0
	e.notesLenAtPrimaryRead = 0

	// Self-loop detection: if we already have investigation notes from
	// a prior run, this is a retry (explore → explore self-loop). Skip
	// Phase 0 breadth scan and go directly to Phase 1 depth read with
	// a retry-specific prompt. The agent is a singleton so evaluator
	// state (investigationNotes, searchResult, preScannedFiles) survives
	// across dispatches — the cross-run reset above ensures this only
	// triggers for legitimate intra-Run self-loops.
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

	analyzerKeywords := irKeywords(ctx)
	analyzerEntities := irEntities(ctx)
	analyzerKind := irQuestionKind(ctx)

	if len(analyzerKeywords) > 0 {
		// Run graduated keyword search before Phase 1 starts.
		// This gives the LLM a pre-ranked file list instead of
		// making it guess which grep patterns to use.
		sr := keywordSearch(analyzerKeywords, ctx.RepoRoot)
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
			// Entity source strategy: prefer the analyzer's declared
			// entities outright when it provided ≥ 2 entries alongside a
			// concrete declared kind — the analyzer sees the raw user
			// wording and its output is strictly more intentional than a
			// regex over the same string. Fall back to UNION with the
			// regex extraction only when the analyzer's set is too thin
			// to satisfy ERM's call_chain minimum of 2 entities.
			//
			// 2026-04-13 (REPL-audit follow-up #5): the previous UNION
			// policy pulled regex noise in even when the analyzer had
			// already returned a clean set. Combined with the #3 tighter
			// extractRankingEntitiesWithGraph filter, preferring the
			// analyzer set removes the last over-broad path by which
			// generic English words reach ERM. The < 2 fallback preserves
			// the reason the original UNION existed: df1 revealed that
			// the analyzer can legitimately produce only 1 entity
			// ("subagent") for questions whose phrasing has a single
			// CamelCase-looking token, and ERM's call_chain requirement
			// demands 2+ entities to reach "satisfied".
			//
			// The c04298f regression this change must NOT re-introduce
			// was joining the original Chinese question and the
			// analyzer's English rewrite into a single STRING and then
			// running regex extraction over the noise. That is a
			// different failure mode: here we keep the two sources
			// SEPARATE and either trust the analyzer outright or merge
			// two clean lists.
			var ermEntities []string
			seen := make(map[string]bool)
			for _, ent := range analyzerEntities {
				if ent = strings.TrimSpace(ent); ent != "" && !seen[ent] {
					ermEntities = append(ermEntities, ent)
					seen[ent] = true
				}
			}
			declaredKind := strings.ToLower(strings.TrimSpace(analyzerKind))
			trustAnalyzer := declaredKind != "" && declaredKind != "unknown" && len(ermEntities) >= 2
			// REPL-mode entity pollution fix.
			//
			// In REPL mode ctx.Objective is the REPL's `effective` string:
			//   "## Prior conversation\n<memory dump>\n\n## Current request\n<raw>"
			// Running regex entity extraction over this blob pulls every
			// CamelCase / snake_case / file-path token from the memory
			// section into `regexEntities` — on a typical codrax session
			// that's 20+ bogus "entities" including internal symbol
			// names, file paths, and line-number fragments. They then
			// become ERM requirements that can never be satisfied,
			// S1 semantic early-stop never fires (because ermAllSatisfied
			// stays false forever), and the answer quality degrades.
			//
			// `stripConversationPrefix` returns only the raw current
			// request portion (unchanged in single-shot mode where no
			// marker is present).
			cleanObjective := stripConversationPrefix(ctx.Objective)
			if !trustAnalyzer {
				regexEntities := extractRankingEntitiesWithGraph(cleanObjective, sr.Graph)
				if len(regexEntities) == 0 {
					regexEntities = extractRankingEntitiesWithGraph(ctx.CurrentTask, sr.Graph)
				}
				for _, ent := range regexEntities {
					if !seen[ent] {
						ermEntities = append(ermEntities, ent)
						seen[ent] = true
					}
				}
			}
			logging.Debug("[explorer] erm entities: %d (trustAnalyzer=%v declaredKind=%q analyzer=%d)",
				len(ermEntities), trustAnalyzer, declaredKind, len(analyzerEntities))
			// Keyword trigger source also uses the clean current
			// request. A memory blob containing a prior "如何 / how
			// does" question would otherwise over-trigger the
			// mechanism classifier on a fresh enumeration question.
			ermKeywordSource := ctx.CurrentTask
			if cleanObjective != "" && cleanObjective != ctx.CurrentTask {
				ermKeywordSource = cleanObjective + " | " + ctx.CurrentTask
			}
			// Pass the analyzer's declared question_kind (may be empty or
			// "unknown"; the hint-aware path handles both by falling
			// back to pure keyword inference).
			e.ermRequirements = extractEvidenceRequirementsWithHint(
				ermKeywordSource, ermEntities, analyzerKind,
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
			// Primary-target banner: when the ERM entities resolve to a
			// SINGLE primary file via receiver-aware disambiguation AND
			// sibling-receiver definitions of the same method name exist
			// in OTHER files, emit an explicit "read this, avoid those"
			// directive. Without this, the LLM sees the sibling files in
			// the keyword_search ranked list and repo_map output, then
			// self-directs "Next steps: gather evidence from sub_explorer
			// and finalizer" — poisoning the final answer with drift from
			// siblings even though f99a727's evidence filter drops their
			// items. Tracked in the df3 eval at 190611 (2/3 runs drifted).
			if banner := e.buildPrimaryTargetBanner(); banner != "" {
				b.WriteString(banner)
			}
		} else {
			// No hits at any level — list the keywords so the LLM
			// can try its own grep strategies.
			b.WriteString("### Search Keywords (no pre-scan hits)\n\n")
			b.WriteString("The analyzer provided these keywords but none matched. Try broader patterns:\n")
			for _, kw := range analyzerKeywords {
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

// primaryEntityFiles computes the set of file paths that define any
// ERM requirement entity as a symbol in the repo graph. This is the
// "primary entity" file set — the files the LLM MUST read_file (not
// merely grep) to substantively answer the question.
//
// Entity-to-file lookup uses exact-name match (case-insensitive) on
// `Graph.SymbolDefs`. Entities that have no graph symbol (concept
// words, generic English nouns) contribute nothing — for those the
// gate is skipped and the existing ERM/evidence checks govern.
//
// Receiver-aware disambiguation: when the entity set contains a
// type-shaped symbol (struct / class / interface / type kind), that
// symbol's name is treated as a "receiver hint". Method-kind entities
// in the same set are then filtered to definitions whose Receiver is
// in the hint set. This makes "explorerEvaluator 的 ContinuationPrompt"
// resolve to the SINGLE explorer.go definition instead of sibling
// methods (explorerEvaluator / subExplorerEvaluator / ...) all named
// ContinuationPrompt — the df3 drift root cause. When no receiver
// hint exists (question has only method entities with no type
// qualifier), the old behaviour is preserved:
// all method definitions contribute their file.
//
// The function is called each time MidLoopCheck and ShouldStop need
// the set. It is cheap (hash lookups per entity) and re-computing
// avoids stale-cache risk when ermRequirements evolve across iters.
func (e *explorerEvaluator) primaryEntityFiles() []string {
	if e.searchResult == nil || e.searchResult.Graph == nil || len(e.ermRequirements) == 0 {
		return nil
	}
	graph := e.searchResult.Graph

	// Flatten all ERM entities into a set.
	entities := make(map[string]string) // lower → original case
	for _, req := range e.ermRequirements {
		for _, ent := range req.Entities {
			if ent == "" {
				continue
			}
			entities[strings.ToLower(ent)] = ent
		}
	}
	if len(entities) == 0 {
		return nil
	}

	// Build receiver hint set: entities that resolve to a type-shaped
	// symbol (struct / class / interface / type / enum). Use the
	// canonical symbol name from the graph (original case) since
	// Symbol.Receiver strings also preserve case.
	receiverHint := make(map[string]bool)
	for entLower := range entities {
		for symName, defs := range graph.SymbolDefs {
			if strings.ToLower(symName) != entLower {
				continue
			}
			for _, d := range defs {
				if d == nil {
					continue
				}
				switch strings.ToLower(d.Kind) {
				case "struct", "class", "interface", "type", "enum":
					receiverHint[symName] = true
				}
			}
		}
	}

	seen := make(map[string]bool)
	var files []string
	for entLower := range entities {
		for symName, defs := range graph.SymbolDefs {
			if strings.ToLower(symName) != entLower {
				continue
			}
			for _, d := range defs {
				if d == nil || d.File == "" {
					continue
				}
				// Receiver-aware disambiguation for methods.
				if strings.ToLower(d.Kind) == "method" && len(receiverHint) > 0 {
					if !receiverHint[d.Receiver] {
						continue
					}
				}
				if !seen[d.File] {
					seen[d.File] = true
					files = append(files, d.File)
				}
			}
		}
	}
	return files
}

// buildPrimaryTargetBanner returns a prompt block that names the single
// primary target file and lists sibling-receiver files to avoid. Fires
// only when receiver-aware disambiguation resolves the ERM method
// entities to exactly one file AND at least one sibling-receiver
// definition of the same method name exists in another file.
//
// This is the second layer of the receiver drift fix (f99a727 was the
// first). The evidence filter in f99a727 drops sibling-file evidence
// items but cannot stop the LLM from READING sub_explorer.go /
// finalizer.go in the first place — both appear in the keyword_search
// ranked list and the repo_map output for any grep of a polymorphic
// method name. Once the LLM has read them, their observations leak
// into the narrative StageReport even though the structured evidence
// items are filtered out. Df3 eval at 190611 run-2 / run-3 both drifted
// this way: cited internal/agent/sub_explorer.go:154-198 with the exact
// signature line, because the LLM chose to go read it.
//
// Banner fires only when the drift is actually possible. When no
// siblings exist (single definition), no banner is emitted — the
// evidence filter and primary-file S1 gate already handle that case.
func (e *explorerEvaluator) buildPrimaryTargetBanner() string {
	if e.searchResult == nil || e.searchResult.Graph == nil || len(e.ermRequirements) == 0 {
		return ""
	}
	graph := e.searchResult.Graph
	primary := e.primaryEntityFiles()
	if len(primary) != 1 {
		return ""
	}
	targetFile := primary[0]

	// Collect the method-name entities (the ones whose sibling
	// definitions we need to warn against). These are entities that
	// resolve to a "method" kind in the graph.
	entities := make(map[string]string) // lower → original
	for _, req := range e.ermRequirements {
		for _, ent := range req.Entities {
			if ent == "" {
				continue
			}
			entities[strings.ToLower(ent)] = ent
		}
	}
	methodNames := make(map[string]string) // lower → original
	for entLower, entOrig := range entities {
		for symName, defs := range graph.SymbolDefs {
			if strings.ToLower(symName) != entLower {
				continue
			}
			for _, d := range defs {
				if d != nil && strings.ToLower(d.Kind) == "method" {
					methodNames[entLower] = entOrig
					break
				}
			}
		}
	}
	if len(methodNames) == 0 {
		return ""
	}

	// Collect sibling files: files OTHER than targetFile that define a
	// method with any of these names. De-duplicate by file.
	siblingSet := make(map[string]bool)
	for entLower := range methodNames {
		for symName, defs := range graph.SymbolDefs {
			if strings.ToLower(symName) != entLower {
				continue
			}
			for _, d := range defs {
				if d == nil || d.File == "" {
					continue
				}
				if strings.ToLower(d.Kind) != "method" {
					continue
				}
				if d.File == targetFile {
					continue
				}
				siblingSet[d.File] = true
			}
		}
	}
	if len(siblingSet) == 0 {
		return ""
	}

	siblings := make([]string, 0, len(siblingSet))
	for f := range siblingSet {
		siblings = append(siblings, f)
	}
	sort.Strings(siblings)

	// Pick the most distinctive method name for the directive. When
	// multiple polymorphic method entities are present, prefer the
	// longest name as the most specific.
	var distinct string
	for _, orig := range methodNames {
		if len(orig) > len(distinct) {
			distinct = orig
		}
	}

	var b strings.Builder
	b.WriteString("### Primary Target File\n\n")
	fmt.Fprintf(&b, "**Read `%s` to answer this question.** ", targetFile)
	fmt.Fprintf(&b, "This is the only file whose `%s` definition matches the receiver in the question.\n\n",
		distinct)
	b.WriteString("**Do NOT gather evidence from these sibling files** — they define methods with the same name but on different receiver types, and are NOT the target:\n")
	for _, f := range siblings {
		fmt.Fprintf(&b, "- `%s`\n", f)
	}
	b.WriteString("\nIgnore these siblings even if grep/repo_map/pre-scan ranking surfaces them. ")
	b.WriteString("They answer a different question about a different type.\n\n")
	return b.String()
}

// filterEvidenceByPrimaryFiles keeps evidence items whose Source is
// in the primary-file set (empty Source is kept too — items without
// a location cannot be filtered safely and usually carry general
// facts like resolved chains). Used only for mechanism questions
// where the finalizer needs tightly-scoped evidence to avoid being
// drowned by concrete-value noise from unrelated files.
//
// Filter F8 in the evidence filtering pipeline. Fail-open: returns
// the unfiltered set on zero survivors.
// Paired with F9 (scrubSiblingEvidenceBlocks) which enforces the
// same primary-file scope on the prose channel — both must run.
func filterEvidenceByPrimaryFiles(items []types.EvidenceItem, primary []string) []types.EvidenceItem {
	if len(items) == 0 || len(primary) == 0 {
		return items
	}
	primarySet := make(map[string]bool, len(primary))
	for _, p := range primary {
		primarySet[p] = true
	}
	out := items[:0:0] // new slice, preserve original order
	for _, ev := range items {
		if ev.Source == "" || primarySet[ev.Source] {
			out = append(out, ev)
		}
	}
	return out
}

// observePrimaryRead detects whether any primary-entity file has
// just entered the readSet derived from the given tool history. On
// first detection, it snapshots the current iteration and the
// length of investigationNotes so ShouldStop's S1 anchor can
// enforce: "primary file was read AND LLM subsequently wrote fresh
// evidence notes from that read."
//
// Idempotent: once primaryReadIter is set, later calls are no-ops.
// Called from MidLoopCheck (runs after every tool batch).
func (e *explorerEvaluator) observePrimaryRead(iteration int, history []types.ToolResult) {
	if e.primaryReadIter > 0 {
		return
	}
	primary := e.primaryEntityFiles()
	if len(primary) == 0 {
		return
	}
	_, readSet := extractFileCoverage(history)
	for _, pf := range primary {
		if readSet[pf] {
			e.primaryReadIter = iteration
			e.notesLenAtPrimaryRead = len(e.investigationNotes)
			logging.Debug("[explorer] primary-entity file read at iter=%d: %s (notesAtRead=%d)",
				iteration, pf, e.notesLenAtPrimaryRead)
			return
		}
	}
}

func (e *explorerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	// Semantic early stop (S1 of the early-stop audit, see
	// memory/project_explorer_early_stop_audit.md): when the LLM
	// has just produced a content-only response (no tool calls) AND
	//  1. Phase 1 (depth read) is active,
	//  2. ERM requirements are ALL satisfied,
	//  3. the structured evidence set already carries at least one
	//     answer-shaped (terminal) item,
	// the investigation has everything it needs. Keep running past
	// this point only burns iterations on cross-reference self-
	// recursion (LLM writes notes mentioning internal symbols like
	// `ContinuationPrompt`, those symbol files get added to
	// preScannedFiles, and the loop chases its own tail).
	//
	// Runtime evidence: the 2026-04-12 `有多少个agent可以调用subagent`
	// re-test reached this state at iter=13 but the loop burned 6
	// more iterations reading internal/agent/explorer.go — entirely
	// useless for the user question. See
	// /tmp/earlystop_run.log for the full per-iteration trace.
	//
	// Why stop here instead of in ContinuationPrompt: the existing
	// ContinuationPrompt branches (partial-read hints, preScanned
	// unread, unanalyzed symbols) have HIGHER priority than the
	// terminal `idleStreakInDepth >= 2` escape hatch, and they each
	// reset the idle counter so it never actually trips. ShouldStop
	// runs BEFORE ContinuationPrompt in BaseAgent.Execute, so
	// returning true here bypasses all those self-feeding branches
	// cleanly.
	if len(resp.ToolCalls) > 0 {
		return false
	}
	if e.phase != 1 {
		return false
	}
	if len(e.ermRequirements) == 0 {
		return false
	}
	// Refresh ERM satisfaction from the latest investigation
	// notes. Pre-Phase-2, ERM state was only updated inside
	// ContinuationPrompt — which runs AFTER ShouldStop in
	// BaseAgent.Execute. That meant the first soft-stop AFTER an
	// iteration that satisfied the requirements would still see
	// stale (unsatisfied) state and push through to
	// ContinuationPrompt, burning one extra iteration. Running
	// the check here is cheap (pure note parsing + tag counting;
	// no file reads, no dataflow) and makes S1 fire at the
	// earliest possible soft-stop.
	//
	// Important: the check must also parse the current soft-stop
	// content into notes before running the ERM check, because
	// ContinuationPrompt is what normally appends. Without this,
	// ShouldStop always sees stale notes (missing the iteration
	// that just produced the satisfying evidence).
	var notesForCheck []string
	if resp.Content != "" {
		notesForCheck = append(notesForCheck, e.investigationNotes...)
		notesForCheck = append(notesForCheck, resp.Content)
	} else {
		notesForCheck = e.investigationNotes
	}
	e.ermRequirements = checkRequirementSatisfaction(e.ermRequirements, notesForCheck, e.structuredEvidence)
	if !ermAllSatisfied(e.ermRequirements) {
		logging.Debug("[explorer] S1 check iter=%d notes=%d fresh=%v unsat: %s",
			iteration, len(e.investigationNotes), resp.Content != "",
			formatERMStatuses(e.ermRequirements))
		return false
	}
	// S1 primary-file-read gate (df3 file-selection drift fix).
	//
	// Observed failure mode (df3 file-selection drift, 2026-04-13
	// eval/results/df3-20260413-173231/run-3): LLM runs parallel grep
	// on 5 files, extracts [MECHANISM] / [DIRECT] / [REGISTRATION]
	// tags from the grep CONTEXT LINES (not from actual file bodies),
	// writes a `## Evidence from <file>` block per file. ERM counts
	// the tags, flips to "satisfied". S1 fires. The LLM never read
	// any of the files via read_file, so all evidence is fabricated
	// from 3-line grep context.
	//
	// Two-part gate:
	//
	//   (a) At least ONE primary-entity file must be in the readSet.
	//       Primary-entity file = file defining any ERM entity as a
	//       graph symbol (exact-name match in SymbolDefs). Concept-
	//       word entities (no symbol) contribute nothing; when no
	//       primary file exists, the gate is skipped. This forces
	//       the LLM to actually read_file the target before S1.
	//
	//   (b) investigationNotes must have GROWN since the primary
	//       file first entered the readSet. Without this, the LLM
	//       can read the file at iter=N but still soft-stop at
	//       iter=N+1 with only iter=N-2's fake notes on record —
	//       S1 would fire on the fake notes and the read is wasted.
	//       The grow-check guarantees at least one soft-stop AFTER
	//       the primary read, during which ContinuationPrompt has a
	//       chance to append fresh notes derived from the real read.
	//
	// Tracking state (primaryReadIter, notesLenAtPrimaryRead) is
	// updated from MidLoopCheck's observePrimaryRead at every tool
	// batch, so ShouldStop always sees a current snapshot.
	if primary := e.primaryEntityFiles(); len(primary) > 0 {
		if e.primaryReadIter == 0 {
			logging.Debug("[explorer] S1 check iter=%d blocked: primary-entity files %v not read yet",
				iteration, primary)
			return false
		}
		if len(e.investigationNotes) <= e.notesLenAtPrimaryRead {
			logging.Debug("[explorer] S1 check iter=%d blocked: notes have not grown since primary read at iter=%d (notes=%d, atRead=%d)",
				iteration, e.primaryReadIter, len(e.investigationNotes), e.notesLenAtPrimaryRead)
			return false
		}
	}
	// During the ReAct loop, `e.structuredEvidence` is only
	// populated at ParseOutput time (ensureStructuredEvidence is
	// the end-of-stage hook), so it is nil here and
	// hasTerminalEvidence would always return false. Parse the
	// live investigation notes on-the-fly instead — this covers
	// [REGISTRATION] and [DIRECT] tags the LLM has already
	// written, which is what hasTerminalEvidence needs to see.
	// The parse is a pure string walk (parseEvidenceItems), no
	// file reads, no dataflow — cheap enough to run every
	// soft-stop check.
	if len(e.investigationNotes) == 0 {
		return false
	}
	noteEvidence := parseEvidenceItems(e.investigationNotes, "explorer.s1check")
	if !hasTerminalEvidence(noteEvidence) {
		logging.Debug("[explorer] S1 check iter=%d notes=%d ERM satisfied but no terminal evidence (%d items parsed)",
			iteration, len(e.investigationNotes), len(noteEvidence))
		return false
	}
	logging.Debug("[explorer] S1 early-stop iter=%d notes=%d ERM satisfied + terminal evidence=%d (%s)",
		iteration, len(e.investigationNotes), len(noteEvidence),
		formatERMStatuses(e.ermRequirements))
	return true
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
	// Track primary-entity file reads for S1's df3-drift gate. Runs
	// before the throttle so even "skipped" MidLoopCheck calls still
	// update the read tracking — ShouldStop's gate depends on this
	// state being current. Idempotent: once primaryReadIter is set,
	// later calls are no-ops.
	e.observePrimaryRead(iteration, allResults)

	// Infer the current batch size from the allResults growth delta.
	// MidLoopCheck is called once per ReAct iteration, after all tool
	// calls in the current batch have executed and their results have
	// been appended. This is the cleanest signal we have without
	// changing the MidLoopEvaluator interface. Update the serial-
	// streak counter regardless of whether the throttled checks below
	// fire, so the parallel-batching cue (Check 3) has accurate
	// degradation history when it runs.
	currentBatch := len(allResults) - e.midLoopLastResultsLen
	if e.midLoopLastResultsLen > 0 && currentBatch <= 1 {
		e.midLoopSerialStreak++
	} else if currentBatch > 1 {
		e.midLoopSerialStreak = 0
	}
	e.midLoopLastResultsLen = len(allResults)

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

	// Check 3: parallel tool-call cue.
	//
	// iter=0 reliably batches 3-8 tool calls because the initial
	// prompt sets up a seed-file scan. Mid-loop iterations degrade to
	// 1-2 tool calls per round because the LLM falls into a single-
	// step ReAct rhythm ("read A, observe, think, read B, observe,
	// ..."). Each serial round pays full LLM round-trip latency, and
	// the 2026-04-13 latency audit measured ~3s per round. On a 15-
	// iter explorer this is where most of the
	// remaining ReAct latency lives AFTER the self-dispatch fix.
	//
	// Fire only when:
	//   - the LLM has been in a serial (≤1 call/round) rhythm for at
	//     least 2 iters in a row — one serial round is noise, two is
	//     a pattern
	//   - at least 2 discovered files remain unread — otherwise there
	//     is nothing to parallelize
	//   - no partial-read hint was emitted above — those have higher
	//     priority and the LLM should finish that function first
	//   - the hint has not already been injected this dispatch — one
	//     nudge is enough; repeated nudges become noise and would
	//     starve other mid-loop checks
	//
	// The cue stays structural: it says "parallelize independent
	// reads, serialize when output of one determines the next" and
	// names no files. The LLM is the only party that sees the notes
	// and history, so it is the only party that can judge
	// independence; we just remove the implicit "one-at-a-time"
	// rhythm that the prior iterations established.
	if b.Len() == 0 && !e.midLoopParallelInjected && e.midLoopSerialStreak >= 2 {
		discovered, readSet := extractFileCoverage(allResults)
		unreadCount := 0
		for _, f := range discovered {
			if !readSet[f] && !isNoisePath(f) {
				unreadCount++
			}
		}
		if unreadCount >= 2 {
			b.WriteString("MID-LOOP CHECK: you have been issuing one tool call per round for several iterations. " +
				"If you need to read multiple files whose contents do NOT depend on each other, issue all the `read_file` calls as a single parallel tool-call batch (multiple tool_use blocks in one assistant message) — this cuts LLM round-trip latency significantly. " +
				"Serialize only when the output of one read determines what to read next (e.g. you need to see a symbol in file A before you know which line range of file B to fetch). " +
				"Apply the same rule to independent `grep` calls.\n")
			e.midLoopParallelInjected = true
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
			"Read the key source files you identified. After EACH file, extract ALL relevant facts as structured evidence.\n\n" +
			"**Preferred channel: call the `emit_evidence` tool.** After reading a file, call `emit_evidence(items=[...])` with one item per fact you want the synthesis layer to see. Send the full batch in ONE call per file — do not invoke the tool per item. Each item is an object with these fields:\n" +
			"  - `kind`: one of `direct`, `conditional`, `registration`, `mechanism`, `relationship`, `absent`\n" +
			"  - `subject`: the primary symbol (function/type/key) the fact is about\n" +
			"  - `object`: the secondary symbol (REQUIRED for `relationship`)\n" +
			"  - `source`: repository-relative file path (REQUIRED, must contain `/` or `.`)\n" +
			"  - `line_start`: integer line number taken EXACTLY from the read_file gutter (omit if no specific line)\n" +
			"  - `line_end`: optional end of range, defaults to line_start\n" +
			"  - `condition`: the IF clause for `conditional` items\n" +
			"  - `summary`: free-text rationale\n" +
			"Unknown fields and unknown kinds are REJECTED with a clear error — fix and resend rather than retry blind.\n\n" +
			"**Fallback channel: markdown blocks.** If you cannot use the tool for a particular item, write the markdown shape below in your assistant message. The two channels are merged downstream and deduplicated, so it is also safe to use both — just do not duplicate the same fact verbatim across them.\n\n" +
			"Markdown shape:\n\n" +
			"```\n" +
			"## Evidence from [filename]\n" +
			"- [DIRECT] `functionName` line N: <what this code establishes>\n" +
			"- [CONDITIONAL] `functionName` line N: <what happens> IF <condition>\n" +
			"- [REGISTRATION] `functionName` line N: <what is registered/configured, with EXACT values>\n" +
			"- [MECHANISM] `functionName` line N: <how something works>\n" +
			"- [RELATIONSHIP] `symbolA` → `symbolB`: <nature of the link>\n" +
			"```\n\n" +
			"**Rules:**\n" +
			"- **Line numbers must come from the gutter.** Every `read_file` result shows each line with its absolute line number in the left gutter (format `   123│ code...`). When you write `line N` in an evidence entry, `N` MUST be the exact gutter number of the line you are describing — do not estimate, do not interpolate between two gutter numbers you saw, do not carry a number over from `grep` output. If you want to cite a range, cite the gutter numbers of the first and last lines of that range verbatim. If you are not certain which gutter number applies, leave the `line N` part off rather than guess — an entry without a line is useful, an entry with a wrong line is not.\n" +
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
	// P1.2 — the explorer no longer captures the synthesis LLM's
	// last-assistant prose into StageOutput. The prose-to-finalizer
	// channel was the R1 escape hatch; out.StageReport below is set
	// to the deterministic canonical render (renderExplorerStageReport)
	// and out.Data is reduced to an empty JSON object. The synthesis
	// LLM call still runs because SynthesisPrompt has structured side
	// effects (concrete-value extraction merged into e.structuredEvidence)
	// — but its prose output dies inside BaseAgent.Execute.
	//
	// Extract facts from tool results. Each tool declares its own
	// Confidence via the Tool interface: evidence tools (grep,
	// read_file, …) return 0.8, navigation indexes (repo_map) return
	// 0.3, and orchestration/emit tools (propose_sub_agents, emit_*)
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
	// Conversely (S1 alignment, see memory/project_explorer_early_stop_audit.md):
	// if all ERM requirements ARE satisfied, PROMOTE hasEnough to true
	// regardless of the quantitative toolDiversity / fileCoverage /
	// evidenceQuality thresholds. Those thresholds are heuristic
	// proxies for "enough evidence"; when ERM is fully satisfied we
	// know semantically that the required evidence exists, so blocking
	// on a coverage ratio would re-enter the stage uselessly and
	// eventually hit the oscillation guard (5-visit cap).
	//
	// Concrete trigger: 2026-04-12 `有多少个agent可以调用subagent` real-
	// scenario re-test. S1 stopped the ReAct loop at iter=11 with ERM
	// fully satisfied, but ParseOutput still computed hasEnough=false
	// because enumeration-mode fileCoverage needed ≥80% of discovered
	// files read (the explorer stopped earlier than that). HasEnough=false
	// → MissingFacts → orchestrator re-dispatched explore → 5 visits → oscillation error.
	if len(e.ermRequirements) > 0 {
		e.ermRequirements = checkRequirementSatisfaction(e.ermRequirements, e.investigationNotes, e.structuredEvidence)
		allSat := ermAllSatisfied(e.ermRequirements)
		if hasEnough && !allSat {
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
		} else if !hasEnough && allSat {
			// S1 alignment: ERM fully satisfied overrides quantitative
			// floors. Record which floor originally failed so the retry
			// hint (if any downstream code still builds one) is accurate.
			logging.Debug("[explorer] ERM all-satisfied promote: hasEnough=true (quantitative floors: toolDiv=%v fileCov=%v evQual=%v)",
				toolDiversity, fileCoverage, evidenceQuality)
			hasEnough = true
		}
	}

	signals := &types.ExecutionSignals{HasEnoughFacts: hasEnough}

	// Rank evidence and findings by relevance to the user's question
	// so downstream consumers (finalizer) get the most useful items first.
	rankedEvidence := rankEvidenceByRelevance(e.userQuestion, e.structuredEvidence, readSet)
	rankedFindings := rankFindingsByRelevance(e.userQuestion, e.flowFindings)

	// df3 drift fix: for mechanism questions anchored on a primary
	// entity file (e.g. "explorerEvaluator 的 ContinuationPrompt 怎
	// 么实现?" → primary file = internal/agent/explorer.go), filter
	// the evidence to items from the primary file(s) before passing
	// to the finalizer. This solves the `cmd/root.go` / `sub_explorer.go`
	// contamination of the finalizer's top-18 Structured Evidence
	// section — concrete-value extraction across the whole repo
	// would otherwise drown the actual [MECHANISM]/[CONDITIONAL]
	// tags the LLM wrote about the target function.
	//
	// Fail-open: if filtering removes everything (no primary-file
	// evidence survived — unusual, implies the investigation never
	// touched the target file), the unfiltered list is used so we
	// don't block the finalizer on an empty set. Only applied for
	// mechanism; enumeration / registration / call_chain are unaffected.
	if strings.EqualFold(strings.TrimSpace(irQuestionKind(ctx)), "mechanism") {
		if primary := e.primaryEntityFiles(); len(primary) > 0 {
			filtered := filterEvidenceByPrimaryFiles(rankedEvidence, primary)
			if len(filtered) > 0 {
				logging.Debug("[explorer] mechanism-kind evidence filter: %d → %d items (primary files: %v)",
					len(rankedEvidence), len(filtered), primary)
				rankedEvidence = filtered
			} else {
				logging.Debug("[explorer] mechanism-kind evidence filter: 0 items match primary files %v, keeping full set (%d)",
					primary, len(rankedEvidence))
			}
		}
	}

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
	// df3 drift fix: mechanism questions do not benefit from the
	// chain-ranked Ground Truth section. identifyAnswerChains tends
	// to surface whatever bind/return chains rank high, which for
	// multi-type polymorphic methods (e.g. ContinuationPrompt on
	// both explorerEvaluator and subExplorerEvaluator) pulls
	// sibling evaluators into the Ground Truth and poisons the
	// final answer. Evidence Items (filtered above) carry the
	// [MECHANISM]/[CONDITIONAL] tags with file:line citations which
	// is the right anchoring for a mechanism step_list answer.
	if strings.EqualFold(strings.TrimSpace(irQuestionKind(ctx)), "mechanism") {
		if len(answerChains) > 0 {
			logging.Debug("[explorer] mechanism-kind: dropping %d answer chains (step_list shape uses Evidence Items)",
				len(answerChains))
		}
		answerChains = nil
	}
	if len(answerChains) > 0 {
		strictCount := 0
		for _, c := range answerChains {
			if c.StrictOK {
				strictCount++
			}
		}
		logging.Debug("[explorer] identified %d answer chains (%d strict)", len(answerChains), strictCount)
		for i, c := range answerChains {
			logging.Debug("[explorer]   answer_chain[%d]: %s (score=%.3f strict=%v)",
				i, c.Item.Summary, c.Score, c.StrictOK)
		}
	}

	// Turn A computes only the terminal-evidence count (β) and hands
	// the strict subset to Turn B via TurnAArtifacts. Turn B
	// (extractor) is the sole producer of AnswerSymbols — it calls
	// emit_answer_symbol and the cardinality validator cross-checks
	// the emitted count against max(β, len(AnswerContract.MustInclude))
	// before allowing a CompletenessComplete claim to pass through to
	// the finalizer. Turn A leaves StageOutput.AnswerSymbols nil and
	// the completeness claim at CompletenessUnknown; the orchestrator's
	// per-task merge rule treats nil as "no claim yet" so Turn B's
	// subsequent output authoritatively fills the slot.
	terminalEvidenceCount := 0
	for _, c := range answerChains {
		if c.StrictOK && hasTerminalEvidence([]types.EvidenceItem{c.Item}) {
			terminalEvidenceCount++
		}
	}
	logging.Debug("[explorer] terminalEvidenceCount=%d (slate deferred to Turn B)", terminalEvidenceCount)

	// P1.2 — deterministic StageReport. Build the read-files slice
	// from the coverage set and render the canonical markdown that
	// becomes "Prior Stage Findings" downstream. This replaces the
	// LLM-prose channel that BaseAgent.Execute would otherwise
	// auto-capture into output.StageReport (P1.2 remediation).
	readFilesList := make([]string, 0, len(readSet))
	for f := range readSet {
		readFilesList = append(readFilesList, f)
	}
	canonicalReport := renderExplorerStageReport(
		irQuestionKind(ctx),
		irAnswerShape(ctx),
		rankedEvidence,
		answerChains,
		nil, // symbols: deferred to Turn B
		rankedFindings,
		readFilesList,
		e.isEnumerationQuery,
	)

	out := &StageOutput{
		Data:          json.RawMessage(`{}`),
		StageReport:   canonicalReport,
		NewFacts:      facts,
		EvidenceItems: rankedEvidence,
		FlowFindings:  rankedFindings,
		AnswerChains:  answerChains,
		SignalUpdates: signals,
		// AnswerSymbols + AnswerSymbolCompleteness left zero — Turn B
		// (extractor) is the sole producer; see comment above.
	}

	// Turn A → Turn B handoff: write TurnAArtifacts so the extractor
	// has a frozen snapshot of everything Turn A produced. Must
	// happen AFTER rankedEvidence / rankedFindings / readFilesList
	// are final and BEFORE return so the extractor's
	// BuildInitialPrompt sees the complete payload.
	if ctx != nil && ctx.Mutable != nil {
		// Turn B gets the strict subset of answer-relevant evidence —
		// the items that passed the L0-1 terminal/origin predicates.
		// Demoted items are dropped here because Turn B's cardinality
		// validator needs a predicate-passing baseline, not the loose
		// Ground Truth fallback.
		strictEvidence := make([]types.EvidenceItem, 0, len(answerChains))
		for _, c := range answerChains {
			if c.StrictOK {
				strictEvidence = append(strictEvidence, c.Item)
			}
		}
		ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
			UserQuestion:          e.userQuestion,
			InvestigationNotes:    e.investigationNotes,
			ReadFiles:             readFilesList,
			ToolResults:           toolResults,
			EvidenceItems:         strictEvidence,
			FlowFindings:          rankedFindings,
			TerminalEvidenceCount: terminalEvidenceCount,
		})
		logging.Debug("[explorer] turn A → turn B handoff: wrote TurnAArtifacts (%d notes, %d readFiles, %d toolResults, %d evidence, %d flow, termCount=%d)",
			len(e.investigationNotes), len(readFilesList), len(toolResults), len(strictEvidence), len(rankedFindings), terminalEvidenceCount)
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
		return types.MissingNone
	}
	return types.MissingFacts
}

func (e *explorerEvaluator) ensureStructuredEvidence(ctx *types.AgentContext, toolResults []types.ToolResult) {
	if len(e.structuredEvidence) > 0 || len(e.flowFindings) > 0 {
		return
	}

	parsed := parseEvidenceItems(e.investigationNotes, "explorer.llm")
	// Merge structured items emitted via the emit_evidence tool with
	// the markdown-parsed channel. The two sources are merged by
	// StableEvidenceID so a single fact reported through both
	// channels (LLM both wrote markdown AND called the tool)
	// collapses to one item. The structured tool is always
	// registered after the 2026-04-14 simplification — the markdown
	// parser remains a secondary channel for LLMs that keep writing
	// prose blocks.
	if ctx != nil && ctx.Mutable != nil {
		if emitted := ctx.Mutable.EmittedEvidence(); len(emitted) > 0 {
			logging.Debug("[explorer] ensureStructuredEvidence: merging %d emit_evidence item(s) with %d parsed", len(emitted), len(parsed))
			parsed = mergeEvidenceItems(parsed, emitted)
		}
	}
	// Deterministic line grounding: every parsed item that carries
	// a Subject / Source / LineStart triple is cross-checked against
	// (a) the gutter-reconstructed line text the LLM actually saw
	// and (b) the tree-sitter symbol table. Mismatches have their
	// LineStart cleared and their Producer suffixed "/ungrounded".
	// See groundEvidenceItems for the full contract and
	// memory/project_fake_green_audit_2026_04_14.md Pattern 2 for
	// the failure mode this closes. Items from emit_evidence go through
	// the same grounder — the channel they came from doesn't change
	// the line-number trust contract.
	var graphForGrounding *repomap.Graph
	if e.searchResult != nil {
		graphForGrounding = e.searchResult.Graph
	}
	parsed = groundEvidenceItems(parsed, graphForGrounding, toolResults)
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
	//
	// P1.2 — F9 (`scrubSiblingEvidenceBlocks`) was deleted in the
	// same commit as the deterministic StageReport renderer. Before
	// P1.2, sibling-file `## Evidence from <path>` blocks would have
	// reached the synthesis LLM here, been replayed into its prose
	// output, and then leaked to the finalizer through StageReport.
	// After P1.2 the synthesis LLM's prose no longer flows to
	// StageReport (see ParseOutput's renderExplorerStageReport call),
	// so even if the synthesis LLM repeats sibling-file content the
	// finalizer never sees it. The notes are now passed through
	// untouched (P1.2 remediation).
	if len(e.investigationNotes) > 0 {
		digest.WriteString("## Evidence Catalog\n\n")
		digest.WriteString("These are the evidence entries YOU collected during investigation:\n\n")
		for i, note := range e.investigationNotes {
			note = strings.TrimSpace(note)
			if note == "" {
				continue
			}
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
// buildUniqueDefFileIndex maps each symbol name in the graph to its
// unique defining file, skipping names whose definitions span two
// or more files. Closes the second of the two B-bucket drift sites
// documented in memory/project_repomap_refactor_plan.md: the old
// code read `defs[0].File` unconditionally, so a name like
// `Execute` (present on every *Agent type) would drift to whichever
// file the map iterator happened to visit first. Callers that show
// "(defined in X)" annotations now get a clean empty value when the
// answer is ambiguous, and the decoration is dropped instead of
// displaying the wrong file.
func buildUniqueDefFileIndex(graph *repomap.Graph) map[string]string {
	out := make(map[string]string, len(graph.SymbolDefs))
	for name, defs := range graph.SymbolDefs {
		if len(defs) == 0 {
			continue
		}
		file := defs[0].File
		unique := true
		for _, d := range defs[1:] {
			if d.File != file {
				unique = false
				break
			}
		}
		if unique {
			out[name] = file
		}
	}
	return out
}

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

	// Build symbol → definition file index for directionality
	// annotation. Drift-safe: see buildUniqueDefFileIndex.
	symDefFile := buildUniqueDefFileIndex(graph)

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

	// Bridge-literal extraction pass — deterministic cross-file JOIN
	// producing `A() binds ONLY NewB(...) → B.Name() returns "lit"`
	// chains even when the LLM didn't read the target file. Orthogonal
	// to the per-file extractConcreteValues + multi-pass tracer above,
	// this pass is graph-wide and bounded by symbol-name matching.
	// See memory/project_baseline_2026_04_13_post_phase4.md.
	bridgeItems := extractBridgeLiteralChains(graph, repoRoot)
	if len(bridgeItems) > 0 {
		logging.Debug("[explorer] bridge literal chains: %d items", len(bridgeItems))
		cvEvidence = append(cvEvidence, bridgeItems...)
	}

	// Collapse resolution_chain duplicates produced by the two
	// independent chain producers. The per-file multi-pass tracer
	// (Producer="concrete_values") and the graph-wide JOIN
	// (Producer="bridge_literal") can emit semantically-identical
	// chains that differ only in surface wording — `NewFoo(deps)`
	// vs `NewFoo(...)`, `Name()` vs `Foo.Name()`, `binds` vs
	// `binds ONLY`. Before dedup, identifyAnswerChains used to pick
	// BOTH into its top 5, wasting slots that should have held
	// genuinely distinct chains. Prefer the bridge_literal
	// representation when available because it carries explicit
	// receiver qualifiers and Source/LineStart locators.
	cvEvidence = dedupeResolutionChains(cvEvidence)

	return concreteValuesResult{markdown: b.String(), evidence: cvEvidence}
}

// normalizeChainKey extracts a semantic identity for a resolution
// chain summary so surface-level wording differences between the
// two chain producers collapse to one key. The key is composed of
//
//  1. the first backtick-quoted method (receiver preserved — the
//     chain root usually identifies which register function is
//     doing the binding and is meaningful to retain)
//  2. the last backtick-quoted method with its receiver qualifier
//     and argument list stripped (the terminal identity method is
//     often written as `Foo.Name()` by one producer and as `Name()`
//     by the other)
//  3. the sorted set of double-quoted string literals mentioned in
//     the summary (the terminal return literal is the ground truth
//     the chain answers toward)
//
// Returns an empty-anchor sentinel string ("||") when no backtick
// tokens appear at all so keyless chains never collide in dedup.
func normalizeChainKey(summary string) string {
	var tokens []string
	rest := summary
	for {
		i := strings.Index(rest, "`")
		if i < 0 {
			break
		}
		j := strings.Index(rest[i+1:], "`")
		if j < 0 {
			break
		}
		tokens = append(tokens, rest[i+1:i+1+j])
		rest = rest[i+1+j+1:]
	}
	first := ""
	last := ""
	if len(tokens) > 0 {
		first = normalizeChainMethod(tokens[0], true)
		last = normalizeChainMethod(tokens[len(tokens)-1], false)
	}
	if first == "" && last == "" {
		return "||"
	}
	var literals []string
	s := summary
	for {
		i := strings.Index(s, "\"")
		if i < 0 {
			break
		}
		j := strings.Index(s[i+1:], "\"")
		if j < 0 {
			break
		}
		literals = append(literals, s[i+1:i+1+j])
		s = s[i+1+j+1:]
	}
	sort.Strings(literals)
	return first + "|" + last + "|" + strings.Join(literals, ",")
}

// normalizeChainMethod trims the parenthesized argument list and,
// when keepReceiver is false, also drops any receiver qualifier
// (`Foo.Name` → `Name`). Used by normalizeChainKey to reconcile
// the per-producer differences on the terminal method slot.
func normalizeChainMethod(tok string, keepReceiver bool) string {
	if idx := strings.Index(tok, "("); idx >= 0 {
		tok = tok[:idx]
	}
	tok = strings.TrimSpace(tok)
	if !keepReceiver {
		if dot := strings.LastIndex(tok, "."); dot >= 0 {
			tok = tok[dot+1:]
		}
	}
	return tok
}

// dedupeResolutionChains collapses resolution_chain items whose
// normalizeChainKey matches. For each group, the winner is the item
// with the highest producer rank (bridge_literal > concrete_values >
// anything else); a non-empty Source is used as a secondary tie-break
// so the retained item carries a real file locator when possible.
// Non-chain items and chains with an empty anchor key pass through
// unchanged, preserving their position in the input slice so this
// pass can be safely inserted at the tail of the evidence pipeline
// without disturbing unrelated ordering.
func dedupeResolutionChains(items []types.EvidenceItem) []types.EvidenceItem {
	producerRank := func(p string) int {
		switch p {
		case "bridge_literal":
			return 2
		case "concrete_values":
			return 1
		}
		return 0
	}
	keyToIdx := make(map[string]int)
	kept := make([]types.EvidenceItem, 0, len(items))
	for _, it := range items {
		if it.Kind != types.EvidenceDataflowPath || it.Predicate != "resolution_chain" {
			kept = append(kept, it)
			continue
		}
		key := normalizeChainKey(it.Summary)
		if key == "||" {
			kept = append(kept, it)
			continue
		}
		if existingIdx, ok := keyToIdx[key]; ok {
			existing := kept[existingIdx]
			eRank := producerRank(existing.Producer)
			nRank := producerRank(it.Producer)
			replace := false
			switch {
			case nRank > eRank:
				replace = true
			case nRank == eRank && existing.Source == "" && it.Source != "":
				replace = true
			}
			if replace {
				kept[existingIdx] = it
			}
			continue
		}
		keyToIdx[key] = len(kept)
		kept = append(kept, it)
	}
	return kept
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

// extractBridgeLiteralChains produces deterministic
// `A() binds ONLY NewB(...) → B.Name() returns "literal"` evidence
// chains via a graph-wide cross-file join. This is the "production"
// half of the bridge-literal story (Phase 4 is the "selection" half):
// it guarantees the strict-subset contains the chain even when the
// LLM's investigation didn't read the binding file.
//
// Pass A — Binding collection: walk every function/method whose name
// matches a register-family pattern, run extractConcreteValues on its
// body (comment-stripped), and emit (bindingFn, targetClass) tuples
// for each constructor-passing-call token.
//
// Pass B — Identity-method scan: walk every method whose name is one
// of (Name|ID|Key|Type|Label|Slug|Kind)-family, run
// extractConcreteValues, and emit (class, method, literal) triples
// for string-literal returns.
//
// Pass C — Join on class identifier: for every (binding, identity)
// pair whose target class matches the identity receiver, emit a
// `resolution_chain` EvidenceItem with Producer="bridge_literal".
//
// Cost is bounded by graph symbol count + body size per matching
// function. On codrax-scale repos this is a few hundred short body
// reads, sub-millisecond total.
func extractBridgeLiteralChains(graph *repomap.Graph, repoRoot string) []types.EvidenceItem {
	if graph == nil {
		return nil
	}
	// isRegName matches function names from the "registration family"
	// across supported languages (Go/Java/Python/JS/TS/Rust/C). It is a
	// structural heuristic: any such function MAY contain binding calls.
	// False positives are filtered downstream because Pass C requires a
	// paired identity method on the target class — functions named
	// `addSlice` or `setupDB` that don't actually wire handlers produce
	// zero chains. Case-insensitive so snake_case / camelCase both match.
	regPrefixes := []string{
		"register", "bind", "mount", "wire", "provide", "install",
		"setup", "configure", "attach", "subscribe", "listen", "route",
	}
	isRegName := func(name string) bool {
		lower := strings.ToLower(name)
		for _, p := range regPrefixes {
			if strings.HasPrefix(lower, p) {
				return true
			}
		}
		if strings.HasSuffix(lower, "defaults") || strings.HasSuffix(lower, "default") {
			return true
		}
		// Go-style init() and init-family wiring functions.
		if lower == "init" {
			return true
		}
		if strings.HasPrefix(lower, "init") && len(lower) > 4 {
			return true
		}
		return false
	}
	isIdentityMethod := func(name string) bool {
		switch name {
		case "Name", "ID", "Id", "Key", "Type", "Label", "Slug", "Kind",
			"name", "id", "key", "type", "label", "slug", "kind",
			"getName", "GetName", "get_name",
			"getID", "GetID", "get_id", "getId",
			"getKey", "GetKey", "get_key",
			"getType", "GetType", "get_type":
			return true
		}
		return false
	}

	// File-content cache shared across all loadBody calls.
	fileCache := make(map[string][]string)
	loadBody := func(relPath string, start, end int) string {
		lines, ok := fileCache[relPath]
		if !ok {
			f, err := os.Open(filepath.Join(repoRoot, relPath))
			if err != nil {
				fileCache[relPath] = nil
				return ""
			}
			var ls []string
			sc := bufio.NewScanner(f)
			sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for sc.Scan() {
				ls = append(ls, sc.Text())
			}
			f.Close()
			fileCache[relPath] = ls
			lines = ls
		}
		if lines == nil || start < 1 || end > len(lines) || start > end {
			return ""
		}
		return strings.Join(lines[start-1:end], "\n")
	}

	type binding struct {
		fnQual      string
		file        string
		line        int
		targetClass string
	}
	type identity struct {
		class   string
		method  string
		literal string
	}
	var bindings []binding
	var identities []identity

	for _, fi := range graph.Files {
		if fi == nil {
			continue
		}
		for i := range fi.Symbols {
			sym := &fi.Symbols[i]
			if sym.Kind != "function" && sym.Kind != "method" {
				continue
			}
			if sym.EndLine == 0 {
				continue
			}
			bodyLen := sym.EndLine - sym.Line
			// Pass A — binding collection (register-family names).
			if isRegName(sym.Name) && bodyLen <= 60 {
				body := loadBody(fi.RelPath, sym.Line, sym.EndLine)
				if body != "" {
					for _, cv := range extractConcreteValues(body) {
						if !strings.Contains(cv.kind, "binds") {
							continue
						}
						qual := sym.Name
						if sym.Receiver != "" {
							qual = sym.Receiver + "." + sym.Name
						}
						for _, part := range strings.Split(cv.value, ",") {
							tgt := parseTargetClassFromBinding(part)
							if tgt == "" {
								continue
							}
							bindings = append(bindings, binding{
								fnQual:      qual,
								file:        fi.RelPath,
								line:        sym.Line,
								targetClass: tgt,
							})
						}
					}
				}
			}
			// Pass B — identity-method scan. Methods on any class/struct
			// (Go uses sym.Receiver, Java/Python/JS/Rust use sym.Parent;
			// mirrors the owner fallback in buildConcreteValuesSection),
			// short body (≤10 lines).
			owner := sym.Receiver
			if owner == "" {
				owner = sym.Parent
			}
			if sym.Kind == "method" && owner != "" &&
				isIdentityMethod(sym.Name) && bodyLen <= 10 {
				body := loadBody(fi.RelPath, sym.Line, sym.EndLine)
				if body != "" {
					for _, cv := range extractConcreteValues(body) {
						if cv.kind != "returns" {
							continue
						}
						if len(cv.value) < 2 {
							continue
						}
						first, last := cv.value[0], cv.value[len(cv.value)-1]
						if (first != '"' && first != '\'') || first != last {
							continue
						}
						lit := cv.value[1 : len(cv.value)-1]
						if lit == "" {
							continue
						}
						identities = append(identities, identity{
							class:   owner,
							method:  sym.Name,
							literal: lit,
						})
						break // first literal wins per method
					}
				}
			}
		}
	}

	// Pass C — Join on class identifier.
	if len(bindings) == 0 || len(identities) == 0 {
		return nil
	}
	idByClass := make(map[string][]identity, len(identities))
	for _, id := range identities {
		idByClass[id.class] = append(idByClass[id.class], id)
	}
	var items []types.EvidenceItem
	seen := make(map[string]bool)
	for _, b := range bindings {
		ids := idByClass[b.targetClass]
		if len(ids) == 0 {
			continue
		}
		for _, id := range ids {
			summary := fmt.Sprintf(
				"`%s()` binds ONLY New%s(...) → `%s.%s()` returns %q",
				b.fnQual, b.targetClass, b.targetClass, id.method, id.literal)
			if seen[summary] {
				continue
			}
			seen[summary] = true
			items = append(items, types.EvidenceItem{
				ID: types.StableEvidenceID(
					types.EvidenceDataflowPath, summary, "resolution_chain",
					"", "", b.file, b.line, b.line),
				Kind:       types.EvidenceDataflowPath,
				Subject:    summary,
				Predicate:  "resolution_chain",
				Summary:    summary,
				Source:     b.file,
				LineStart:  b.line,
				LineEnd:    b.line,
				Confidence: 0.9,
				Producer:   "bridge_literal",
			})
		}
	}
	return items
}

// parseTargetClassFromBinding extracts the class identifier from a
// binding value token like "NewSubExplorer(deps)", "new Handler()",
// "UserHandler", or "&Config{}". Returns the class name with the
// "New"/"new " constructor prefixes stripped and parenthesized args
// removed. Empty string if the shape is not a constructor reference.
func parseTargetClassFromBinding(token string) string {
	t := strings.TrimSpace(token)
	t = strings.TrimLeft(t, "&,()")
	t = strings.TrimSpace(t)
	if t == "" {
		return ""
	}
	if paren := strings.IndexByte(t, '('); paren >= 0 {
		t = t[:paren]
	}
	if brace := strings.IndexByte(t, '{'); brace >= 0 {
		t = t[:brace]
	}
	t = strings.TrimSpace(t)
	// Java: "new Xxx"
	if strings.HasPrefix(t, "new ") {
		t = strings.TrimSpace(t[4:])
	}
	// Qualified name disambiguation — walks separator-split segments
	// and returns the rightmost one that begins with an uppercase
	// letter (the type, regardless of which side it's on). Handles:
	//   pkg.Xxx         (Go/Python/JS: pkg is module, Xxx is type)
	//   a.b.UserHandler (chained module access)
	//   pkg::Xxx        (Rust/C++ module path)
	//   Handler::new    (Rust static factory: Handler is the type)
	//   Handler<T>      (generics: <T> gets trimmed by the leading-id
	//                    walk below)
	if strings.ContainsAny(t, ".:") {
		splits := strings.FieldsFunc(t, func(r rune) bool {
			return r == '.' || r == ':'
		})
		picked := ""
		for i := len(splits) - 1; i >= 0; i-- {
			s := splits[i]
			if len(s) > 0 && s[0] >= 'A' && s[0] <= 'Z' {
				picked = s
				break
			}
		}
		if picked != "" {
			t = picked
		}
	}
	// Factory prefix: "NewXxx" → "Xxx" (common Go/C# idiom). Only
	// strips if the remaining name starts with an uppercase letter,
	// keeping words like "News" / "Newer" intact.
	if strings.HasPrefix(t, "New") && len(t) > 3 && t[3] >= 'A' && t[3] <= 'Z' {
		t = t[3:]
	}
	// Take the leading identifier only: stop at the first non-ident
	// char (covers `Xxx::new`, `Xxx()`, `Xxx{}`, `Xxx<T>`, etc.).
	end := 0
	for end < len(t) {
		c := t[end]
		if c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			end++
			continue
		}
		break
	}
	t = t[:end]
	if t == "" || t[0] < 'A' || t[0] > 'Z' {
		return ""
	}
	return t
}

// stripCommentLines replaces comment-only lines (and lines inside a
// multi-line comment block) with empty strings, preserving the line
// array length. This is a pre-pass for extractConcreteValues and
// friends so that the downstream pattern scanners cannot parse comment
// text as code.
//
// Coverage (superset across languages — the scanner doesn't know the
// file's language, so it accepts any common comment shape):
//   - Go/Java/JS/TS/C/Rust:   `//` line, `/* ... */` block, `* ...`
//     continuation lines inside block comments
//   - Python/Ruby/Shell/YAML: `#` line
//   - Python docstrings:      `"""` / `'''` multi-line string blocks
//
// A real code line that happens to start with `*` (e.g. a C pointer
// deref `*ptr = 5`) is not blanked: the helper only strips the
// shapes `*` alone, `* text`, or `*/`, which are exclusively found in
// block-comment continuation lines.
func stripCommentLines(lines []string) []string {
	out := make([]string, len(lines))
	inBlock := false
	inTripleDouble := false
	inTripleSingle := false
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if inBlock {
			if strings.Contains(t, "*/") {
				inBlock = false
			}
			continue
		}
		if inTripleDouble {
			if strings.Contains(line, `"""`) {
				inTripleDouble = false
			}
			continue
		}
		if inTripleSingle {
			if strings.Contains(line, `'''`) {
				inTripleSingle = false
			}
			continue
		}
		if strings.HasPrefix(t, "/*") {
			// Block comment opener. Blank the line regardless;
			// if it doesn't also close on the same line, enter block state.
			if !strings.Contains(t[2:], "*/") {
				inBlock = true
			}
			continue
		}
		if strings.HasPrefix(t, "//") || strings.HasPrefix(t, "#") {
			continue
		}
		if t == "*" || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "*/") {
			continue
		}
		// Standalone Python-style triple-quoted strings (docstrings).
		// A real triple-quoted ASSIGNMENT like `x = """abc"""` is also
		// blanked, but such lines would not produce code-shaped
		// extractions anyway.
		if strings.HasPrefix(t, `"""`) {
			rest := strings.TrimPrefix(t, `"""`)
			if !strings.Contains(rest, `"""`) {
				inTripleDouble = true
			}
			continue
		}
		if strings.HasPrefix(t, `'''`) {
			rest := strings.TrimPrefix(t, `'''`)
			if !strings.Contains(rest, `'''`) {
				inTripleSingle = true
			}
			continue
		}
		out[i] = line
	}
	return out
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
	// Pre-strip comment-only lines so none of the pattern scanners
	// below can parse comment text as code. The constructor-passing
	// call scanner in particular is vulnerable: a line like
	//   // (NewSubExplorer called once from RegisterDefaults). Each
	// would otherwise be emitted as a phantom `binds ONLY` entry,
	// which pollutes resolution-chain synthesis. See
	// memory/project_baseline_2026_04_13_post_phase4.md.
	lines := stripCommentLines(strings.Split(source, "\n"))

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

// firstSeparatorBeforeLineno returns the index of the first `:` or `-`
// that sits immediately before a run of digits — matching ripgrep's
// "path:lineno:content" match format and "path-lineno-content" context
// format. Returns -1 if no separator-before-lineno is found.
func firstSeparatorBeforeLineno(s string) int {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != ':' && c != '-' {
			continue
		}
		// Next char must be a digit for this separator to count as
		// "start of lineno". Otherwise it's a colon/dash inside the
		// path or content, keep scanning.
		if i+1 >= len(s) || s[i+1] < '0' || s[i+1] > '9' {
			continue
		}
		return i
	}
	return -1
}

// isValidFilePath is a cheap sanity check: a real repo-relative file
// path contains either a directory separator or an extension dot after
// any base name. Rejects garbage like "158" (lineno-only), "  // blah"
// (code comment), or "--" (grep group separator) so they don't inflate
// the discovered-files list.
func isValidFilePath(p string) bool {
	if p == "" {
		return false
	}
	// A directory separator is the strongest signal.
	if strings.Contains(p, "/") {
		// But reject paths that contain whitespace or tabs — those are
		// code content, not paths.
		if strings.ContainsAny(p, " \t") {
			return false
		}
		return true
	}
	// Bare filename: must have an extension dot and no whitespace.
	if strings.ContainsAny(p, " \t") {
		return false
	}
	if dot := strings.LastIndex(p, "."); dot > 0 && dot < len(p)-1 {
		return true
	}
	return false
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
			// grep results come in these formats:
			//   files_only=true:  one path per line ("internal/agent/explorer.go")
			//   files_only=false: "path:linenum:content" per match line
			//   with context lines: "path-linenum-content" (dash separator)
			//   group separator:    "--" between context groups
			//
			// Both dash and colon separators must be handled: a context
			// line like "file.go-101-\t// blah" has no colon before the
			// lineno, and without recognizing the dash form the whole
			// line gets treated as a "discovered file", inflating the
			// coverage denominator with dozens of bogus entries per
			// grep call. (Headline fix that made this necessary: prior
			// to the GrepTool -H flag, single-file searches dropped
			// filenames entirely, producing lines like "158-content";
			// isValidFilePath below is the defense-in-depth guard.)
			//
			// The first line may be a summary header "[grep: N matching ...]".
			for _, line := range strings.Split(r.Summary, "\n") {
				path := strings.TrimSpace(line)
				if path == "" || path[0] == '[' || path == "--" {
					continue
				}
				// Normalize: strip leading ./
				path = strings.TrimPrefix(path, "./")
				// Detect "path:linenum:content" (match line) or
				// "path-linenum-content" (context line). For both
				// separators we look for the first occurrence, verify
				// the next token is a run of digits (the lineno), and
				// slice off everything after that.
				if idx := firstSeparatorBeforeLineno(path); idx > 0 {
					path = path[:idx]
				}
				// Defense-in-depth: reject anything that doesn't look
				// like a real file path. A real path has either a
				// directory separator or a file extension (a `.` after
				// the last `/`). Rejects stray lineno-only lines and
				// garbage like "some random string".
				if !isValidFilePath(path) {
					continue
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
//
// S2 (2026-04-12 early-stop audit): the symbol name must overlap
// with an ERM entity (the question's actual subjects) before the
// file is added. Pre-audit this was unfiltered, so when the LLM
// wrote meta-commentary like "handled in ContinuationPrompt" or
// "injected into ToolSchema", the enclosing files (explorer.go,
// llm.go, mcp.go) were pushed into preScannedFiles and the LLM
// was then chased to read its own source code. Evidence:
// /tmp/earlystop_run.log lines 1486-87 and 1601-02 where
// "ToolSchema" and "ContinuationPrompt" triggered self-feeding
// cross-refs that burned iters 14-19.
//
// The filter is structural: any overlap (substring, case-
// insensitive) between the cross-ref symbol and ANY ERM entity
// passes. When the question is about `subagent` / `agent`, symbols
// like `NewSubExplorer`, `SubAgentRegistry`, `AgentName` pass (all
// contain "agent" as a substring). Meta-symbols like
// `ContinuationPrompt`, `ToolSchema`, `BuildToolSchemas` do not.
// When `ermRequirements` is empty (no entities extracted) the
// filter is bypassed so we keep legacy behavior for
// non-entity-oriented questions.
func (e *explorerEvaluator) trackCrossReferences(note string) {
	if e.searchResult == nil || e.searchResult.Graph == nil {
		return
	}
	graph := e.searchResult.Graph

	// Collect ERM entities once, lowercased, for S2 filtering.
	var ermEntities []string
	for _, req := range e.ermRequirements {
		for _, ent := range req.Entities {
			if ent != "" {
				ermEntities = append(ermEntities, strings.ToLower(ent))
			}
		}
	}

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
		// S2 filter: require entity overlap before pulling the
		// symbol's file into preScannedFiles. Empty ermEntities
		// bypass the filter (legacy behavior for non-entity
		// questions).
		if len(ermEntities) > 0 {
			symLower := strings.ToLower(symName)
			match := false
			for _, ent := range ermEntities {
				if strings.Contains(symLower, ent) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
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
