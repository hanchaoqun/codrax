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
	"github.com/hanchaoqun/codrax/internal/analysis/subject"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
	"gopkg.in/yaml.v3"
)

type explorerEvaluator struct {
	heuristics                types.ExploreHeuristics
	tools                     *tool.Registry
	phase                     int                  // 0 = breadth scan, 1 = depth read
	broadenAttempts           int                  // times we pushed for broader grep in Phase 0
	preScannedFiles           []string             // top files from keyword search, for coverage tracking
	allScoredFiles            []string             // ALL files from keyword search (not just top 8), for supplementary evidence
	fileSymbols               map[string][]string  // path → symbol summaries from repo_map
	searchResult              *keywordSearchResult // full search result for cross-reference lookups
	investigationNotes        []string             // assistant analysis messages from ReAct loop
	userQuestion              string               // original user question, for focus alignment
	repoRoot                  string               // repository root path, cached from BuildInitialInstruction
	preScannedPushCount       int                  // times we pushed for unread pre-scanned files without progress
	lastPreScannedUnreadCount int                  // count of unread pre-scanned files at last push
	grepRedirectedFiles       map[string]bool      // files that already received a large-file grep redirect
	isEnumerationQuery        bool                 // true if user question asks to list/enumerate all items
	phase0ExtraRound          bool                 // whether we already gave one extra Phase 0 round for quality gate
	hasPrescanRepoMap         bool                 // keywordSearch (run at BuildInitialInstruction) produced a ranked file list via repo_map; the Phase 0 quality gate treats this as satisfying the structural-discovery half of its requirement, so the LLM isn't penalized for not re-running repo_map at iter=0
	structuredEvidence        []types.EvidenceItem
	flowFindings              []types.FlowFindingDigest
	ermRequirements           []EvidenceRequirement // evidence requirement model
	cachedConcreteValues      *concreteValuesResult // T1.1: built once per Execute, reused by gate + synthesis
	midLoopLastResultsLen     int                   // #34: allResults length at prev observeMidLoop call (used to infer current batch size)
	midLoopSerialStreak       int                   // #34: consecutive iters observed as single-call rounds
	midLoopParallelInjected   bool                  // #34: parallel-batching hint already pushed this dispatch
	midLoopSymbolRefInjected  bool                  // T3b: cross-file-symbol-reference hint already pushed this dispatch
	primaryReadIter           int                   // df3-drift: iter at which a primary-entity file first entered readSet (0 = never)
	notesLenAtPrimaryRead     int                   // df3-drift: snapshot of len(investigationNotes) at primaryReadIter
	investigationComplete     bool                  // set when emit_investigation_complete tool was observed in MidLoop

	// answerSubject is the AnswerSubject classification copied from
	// the analyzer's IR at BuildInitialInstruction time. The chain
	// ranker consults it to score chain terminals against the
	// expected subject kind so chains whose terminal is a different
	// kind of token (e.g. an agent name when the question asks for a
	// skill name) are demoted below subject-matching chains. Zero
	// value (Kind=SubjectUnknown) preserves historical insertion
	// ordering for tests / sub-agents that bypass the analyzer.
	answerSubject types.AnswerSubject

	// predicateAxis is the question's action-verb axis copied from
	// the analyzer's IR at BuildInitialInstruction time. The
	// evidence ranker uses it (via internal/analysis/axis.Affinity)
	// to bias items whose AnchorKind matches the axis — AxisCall
	// boosts AnchorCall and demotes AnchorDefinition, etc. Zero
	// value (AxisUnknown) disables the axis boost and preserves
	// historical ranking for tests / sub-agents that bypass the
	// analyzer.
	predicateAxis types.PredicateAxis

	// complexity is a cached copy of the analyzer-classified
	// Complexity (via irComplexity) captured at
	// BuildInitialInstruction time. T1c: drives ERM threshold
	// scaling via thresholdForKind so complex cross-component
	// questions need more evidence to satisfy a requirement than
	// simple lookups. Cached here because ShouldStop / observeSoftStop
	// do not have direct access to AgentContext; BuildInitialInstruction
	// and ParseOutput do. Zero value ("" = unknown) maps to the
	// historical moderate thresholds so tests that skip
	// BuildInitialInstruction see no behavior change.
	complexity types.Complexity

	// Loop-control state that USED to live here has been lifted into
	// LoopPolicy:
	//   - idleStreakInDepth → obs.IdleStreak (policy-owned counter)
	//   - lastToolResultCount → implicit via policy's tool-result
	//     growth detection in loopPolicyState.Apply
	//   - midLoopLastInjectIter → obs throttling via
	//     LoopPolicy.MinInjectInterval
	// The remaining fields above are DETECTION state (phase
	// transitions, serial-streak detection, primary-read gating)
	// that the explorer's own ReAct logic needs — not duplicated
	// counter state.
}

// ensureHeuristics resolves zero-valued heuristic fields to code
// defaults. Called at the top of observeMidLoop/observeSoftStop so
// tests that construct a bare explorerEvaluator{} still get the
// expected default behavior without having to set every field.
func (e *explorerEvaluator) ensureHeuristics() {
	// Cheap sentinel: if the first field is already resolved, skip.
	if e.heuristics.MidLoopMinIteration != 0 {
		return
	}
	e.heuristics = types.ResolvedExploreHeuristics(e.heuristics)
}


func (e *explorerEvaluator) BuildInitialInstruction(ctx *types.AgentContext, sk *skill.Config) string {
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
	// Detection: compare the incoming Objective to the cached one.
	// Within a single Run, Objective is constant across all explore
	// dispatches (same task.Title). Across Run()s it's different (new
	// REPL turn → new analyzer output → new task title). When they
	// differ, reset every cross-Run field so the fresh-start branch
	// below fires cleanly.
	if ctx.Objective != "" && ctx.Objective != e.userQuestion {
		logging.Debug("[explorer] cross-run reset: current=%q != cached=%q", ctx.Objective, e.userQuestion)
		e.investigationNotes = nil
		e.preScannedFiles = nil
		e.allScoredFiles = nil
		e.searchResult = nil
		e.ermRequirements = nil
		e.fileSymbols = nil
		e.phase0ExtraRound = false
		e.hasPrescanRepoMap = false
		e.grepRedirectedFiles = nil
		e.preScannedPushCount = 0
		e.lastPreScannedUnreadCount = 0
		e.broadenAttempts = 0
		e.midLoopLastResultsLen = 0
		e.midLoopSerialStreak = 0
		e.midLoopParallelInjected = false
		e.midLoopSymbolRefInjected = false
		e.primaryReadIter = 0
		e.notesLenAtPrimaryRead = 0
		e.complexity = ""
		// Loop-policy counters (idleStreakInDepth, lastToolResultCount,
		// midLoopLastInjectIter) are no longer fields on this struct —
		// LoopPolicy constructs a fresh loopPolicyState per dispatch,
		// so there is nothing to reset here.
	}

	e.userQuestion = ctx.Objective
	e.repoRoot = ctx.RepoRoot
	// T1c: capture the analyzer-classified complexity so ShouldStop /
	// observeSoftStop / ensureStructuredEvidence can pass it to
	// checkRequirementSatisfaction without re-reading ctx (those call
	// sites don't all receive ctx). Zero value ("") is preserved when
	// the analyzer stage hasn't run yet (unit tests, sub-agent paths).
	e.complexity = irComplexity(ctx)
	// CGEC: capture the analyzer's AnswerSubject classification so
	// the chain ranker can score chain terminals against the
	// expected subject kind. Empty when AnalysisIR not yet available
	// (analyzer dispatch did not run, or sub-agent context).
	if ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); rm != nil {
			e.answerSubject = rm.AnswerSubject
			e.predicateAxis = rm.PredicateAxis
		}
	}
	// Strip the REPL-assembled Prior Conversation prefix before
	// enumeration detection so that prior-turn keywords (哪些 / how
	// many / list all / ...) do NOT falsely trip `isEnumerationQuery`
	// on the current request. trace 1776448040358685830 exposed the
	// cascade: Prior had "通过哪些机制" → detectEnumerationIntent →
	// mid-loop pushed LLM to read 5 extra files → Tier-1 floor + E's
	// whitelist both inflated to the point of bypass. Single
	// assignment point so every downstream consumer of the flag
	// (mid-loop enumeration hint, soft-stop coverage gate, stage
	// report tag, synthesis prompt) sees the clean signal.
	e.isEnumerationQuery = detectEnumerationIntent(types.StripConversationPrefix(ctx.Objective))
	e.structuredEvidence = nil
	e.flowFindings = nil
	e.cachedConcreteValues = nil
	e.midLoopLastResultsLen = 0
	e.midLoopSerialStreak = 0
	e.midLoopParallelInjected = false
	e.primaryReadIter = 0
	e.notesLenAtPrimaryRead = 0
	// Per-dispatch reset of the completion flag. Without this, a
	// Window 2 explore dispatch inherits Window 1's emit_investigation_complete
	// state and observeSoftStop immediately accepts the stop even if the
	// current window's LLM has done no tool work — defeating the "completion
	// is model-triggered" contract that emit_investigation_complete
	// enforces. Each dispatch must observe its OWN tool call before the
	// flag is true.
	e.investigationComplete = false

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
		// (Loop-policy counters — idleStreakInDepth, lastToolResultCount —
		// are no longer fields here; LoopPolicy rebuilds its state
		// per dispatch.)
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
		b.WriteString("**Tools:** use `grep` (efficient for locating patterns and scanning large files), `read_file` (for reading content), or both together. Pick the most efficient approach for each situation.\n\n")
		b.WriteString("Evidence format (examples — adapt to what you find):\n")
		b.WriteString("- `[DIRECT] functionName line N: <what this code establishes>`\n")
		b.WriteString("- `[CONDITIONAL] functionName line N: <what happens> IF <condition>`\n")
		b.WriteString("- `[REGISTRATION] functionName line N: <what is registered, EXACT values>`\n\n")
		b.WriteString("**User question:** " + e.userQuestion)
		return b.String()
	}

	e.phase = 0 // start in breadth-scan phase

	var b strings.Builder
	b.WriteString("## Phase 1: Breadth Scan\n\n")
	b.WriteString("Your goal is to MAP the relevant territory — find ALL files related to the question. ")
	b.WriteString("Do NOT read files in full yet. Use lightweight tools:\n")
	b.WriteString("- repo_map (task_map view) to get an overview of relevant files\n")
	b.WriteString("- grep with files_only=true to find WHICH FILES contain key terms (just filenames, not lines). Use `file_type` when the language is obvious; do not use --include so you discover all relevant file types\n")
	b.WriteString("- list_files to understand directory structure\n\n")
	b.WriteString("**Non-English questions:** When the user's question is not in English, search with BOTH the original terms AND their English programming equivalents. Most codebases use English identifiers, so always include the translated English terms alongside the original. Batch both versions as parallel grep calls.\n\n")
	b.WriteString("**Keyword variants:** For each search term, generate multiple variants and search them all:\n")
	b.WriteString("- Word roots and inflections (e.g. send/sending/sent)\n")
	b.WriteString("- Synonyms (e.g. send → emit, dispatch, publish, write)\n")
	b.WriteString("- Antonyms when relevant (e.g. lock → unlock, enable → disable)\n")
	b.WriteString("- Abbreviations and full forms (e.g. config → configuration, ctx → context)\n")
	b.WriteString("- CamelCase and snake_case variants (e.g. getUser, get_user, GetUser)\n")
	b.WriteString("Do not rely solely on the suggested terms below — use your domain knowledge to generate additional relevant identifiers.\n\n")

	analyzerKeywords := irKeywords(ctx)
	analyzerEntities := irEntities(ctx)
	analyzerKind := irQuestionKind(ctx)

	if len(analyzerKeywords) > 0 {
		display := analyzerKeywords
		if len(display) > 15 {
			display = display[:15]
		}
		b.WriteString("### Suggested Search Terms\n\n")
		b.WriteString("Use these for grep (from the analyzer's classification):\n`")
		b.WriteString(strings.Join(display, "`, `"))
		b.WriteString("`\n\n")
	}

	if len(analyzerKeywords) > 0 {
		// Run graduated keyword search before Phase 1 starts.
		// This gives the LLM a pre-ranked file list instead of
		// making it guess which grep patterns to use.
		//
		// T1a + T1b: pass analyzerEntities so files matching
		// analyzer-emitted identifiers get a multiplicative boost
		// (entities are the highest-confidence tokens — verbatim
		// from the user question); scale MaxFiles by the
		// complexity classification so complex cross-component
		// questions get a 30-file candidate pool instead of the
		// historical 20 that drops dispatch-path files below the
		// LLM's eyeline.
		sr := keywordSearchWithOptions(analyzerKeywords, ctx.RepoRoot, keywordSearchOptions{
			Entities: analyzerEntities,
			MaxFiles: MaxFilesForComplexity(irComplexity(ctx)),
		})
		e.searchResult = sr
		// Publish the graph to MutableState so tools running later in
		// this dispatch (notably emit_evidence's synchronous grounder)
		// can consult it without re-invoking BuildOrLoadGraph. The
		// handle is `any` on the types side to keep internal/types
		// decoupled from repomap; tool-side consumers type-assert.
		if ctx != nil && ctx.Mutable != nil && sr != nil && sr.Graph != nil {
			ctx.Mutable.SetSearchGraph(sr.Graph)
		}
		// Publish the keyword-search ranking to MutableState so the
		// CGEC pre-complete phase1-unread gate can cross-reference
		// top-ranked files against ReadSet. Canonicalise each path the
		// same way ground.CanonicalRepoRelative does so the gate's
		// readSet lookup is apples-to-apples.
		//
		// T3a: also merge the analyzer's EvidencePlan.RequiredFiles
		// into the ranking. The analyzer pre-computes a top-N list
		// from its repo_map query over the entity set; adding those
		// files here ensures the CGEC phase1-unread gate treats them
		// as first-class coverage targets even when the explorer's
		// own keyword search under-ranks them (the 2026-04-18
		// "explorer→subagent" debug symptom — orchestrator.go
		// matched weakly on pure-grep IDF). We preserve any score
		// the explorer's search already produced; newly-surfaced
		// RequiredFiles get a sentinel score that places them just
		// above the median so they're prioritized without
		// completely displacing high-grep-IDF matches.
		if ctx != nil && ctx.Mutable != nil && sr != nil && len(sr.Files) > 0 {
			seen := make(map[string]bool, len(sr.Files))
			ranked := make([]types.Phase1RankedFile, 0, len(sr.Files))
			for _, f := range sr.Files {
				canon := ground.CanonicalRepoRelative(f.Path, ctx.RepoRoot)
				ranked = append(ranked, types.Phase1RankedFile{
					Path:  canon,
					Score: f.Score,
				})
				seen[canon] = true
			}
			// Compute a sentinel score for analyzer-supplied files
			// that didn't show up in the explorer's keyword search.
			// Using the median of existing scores positions them
			// mid-pack so strong grep-IDF matches still rank higher
			// but the analyzer's picks can't be pushed out of the
			// top-N by noise.
			medianScore := phase1MedianScore(ranked)
			for _, p := range analyzerRequiredFilesFromIR(ctx) {
				canon := ground.CanonicalRepoRelative(p, ctx.RepoRoot)
				if seen[canon] {
					continue
				}
				ranked = append(ranked, types.Phase1RankedFile{
					Path:  canon,
					Score: medianScore,
				})
				seen[canon] = true
			}
			ctx.Mutable.SetPhase1Ranking(ranked)
		}
		results := sr.Files
		// Record that pre-scan produced a repo_map-derived ranked file
		// list the LLM can see. Read by the Phase 0 quality gate.
		e.hasPrescanRepoMap = len(results) > 0
		// T3a: surface the analyzer's EvidencePlan.RequiredFiles as
		// a dedicated prompt section. Files appear first in the
		// instruction text so the LLM sees "start here" guidance
		// before scanning the longer keyword_search ranking below.
		// Only render when non-empty AND complexity is not simple
		// (simple questions don't need the multi-file push).
		if reqFiles := analyzerRequiredFilesFromIR(ctx); len(reqFiles) > 0 &&
			irComplexity(ctx) != types.ComplexitySimple {
			b.WriteString("### Analyzer's Required Files\n\n")
			b.WriteString("The analyzer's repo_map query over the question's entities " +
				"identified these files as structurally relevant. Start your investigation " +
				"here and trace cross-file references outward:\n\n")
			for _, p := range reqFiles {
				b.WriteString("- `" + p + "`\n")
			}
			b.WriteString("\n")
		}
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
			//    survive. Falls back to `ctx.Objective` only when
			//    Objective is empty (e.g. analyze stage stub state).
			//  - Keyword detection runs over the union `Objective | Objective`,
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
			// `types.StripConversationPrefix` returns only the raw current
			// request portion (unchanged in single-shot mode where no
			// marker is present).
			cleanObjective := types.StripConversationPrefix(ctx.Objective)
			if !trustAnalyzer {
				// Entity extraction runs on cleanObjective ONLY. The prior
				// implementation fell back to ctx.Objective when cleanObjective
				// yielded zero entities — in REPL mode that blob contains
				// prior-conversation tokens (file paths, symbol names from old
				// turns, the "Prior/Current/Recent/memory" markers themselves),
				// which then became ERM requirements that polluted file ranking
				// and pulled the explorer toward unrelated files. When
				// cleanObjective yields zero entities, honestly return zero:
				// the analyzer's declared entity (if any) is already in
				// ermEntities, and ermAutoSatisfyUnresolvable below handles
				// the low-entity case so the pipeline does not deadlock.
				regexEntities := extractRankingEntitiesWithGraph(cleanObjective, sr.Graph)
				for _, ent := range regexEntities {
					if !seen[ent] {
						ermEntities = append(ermEntities, ent)
						seen[ent] = true
					}
				}
			}
			logging.Debug("[explorer] erm entities: %d (trustAnalyzer=%v declaredKind=%q analyzer=%d)",
				len(ermEntities), trustAnalyzer, declaredKind, len(analyzerEntities))
			// Keyword trigger source uses the clean current request only.
			// The prior implementation concatenated ctx.Objective as a
			// fallback source — that leaked prior-conversation verbs
			// ("如何 / how does") into the keyword-based mechanism classifier
			// and mis-routed the investigation. cleanObjective is the
			// current REPL turn's question (or the single-shot raw request
			// when no marker is present), always the right source for
			// classifying THIS question.
			ermKeywordSource := cleanObjective
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
			// CGEC D1: publish the full scored-file list to the closure
			// as ScannedSet. Downstream consumers (applyChainPromotion
			// D2, preCompleteContractCheck D3, runForcedReads D4) use
			// this to tell "the explorer knows this file exists and is
			// relevant" apart from "this file is a path the LLM / a
			// chain anchor just made up". Without ScannedSet, a ghost
			// path that sneaks into a chain anchor would otherwise be
			// force-read by the framework (wasting a read and
            // polluting ReadSet). Also include symbol-definition files
			// from repo_map so any file the graph could have surfaced
			// counts as scanned — keeps the scope aligned with the
			// ScannedSet design in architecture.md §8.
			if ctx != nil && ctx.Mutable != nil && len(e.allScoredFiles) > 0 {
				scanned := make(map[string]bool, len(e.allScoredFiles))
				for _, f := range e.allScoredFiles {
					scanned[f] = true
				}
				ctx.Mutable.EvidenceClosure().SetScannedSet(scanned)
				logging.Info("[CGEC] D1 scanned_set: origin=keyword_search files=%d", len(scanned))
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
	forEachMatchingDef(entities, graph, func(_, _, symName string, d *repomap.Symbol) bool {
		switch strings.ToLower(d.Kind) {
		case "struct", "class", "interface", "type", "enum":
			receiverHint[symName] = true
		}
		return true
	})

	seen := make(map[string]bool)
	var files []string
	forEachMatchingDef(entities, graph, func(_, _, _ string, d *repomap.Symbol) bool {
		if d.File == "" {
			return true
		}
		// Receiver-aware disambiguation for methods.
		if strings.ToLower(d.Kind) == "method" && len(receiverHint) > 0 {
			if !receiverHint[d.Receiver] {
				return true
			}
		}
		if !seen[d.File] {
			seen[d.File] = true
			files = append(files, d.File)
		}
		return true
	})
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
	forEachMatchingDef(entities, graph, func(entLower, entOrig, _ string, d *repomap.Symbol) bool {
		if strings.ToLower(d.Kind) == "method" {
			methodNames[entLower] = entOrig
		}
		return true
	})
	if len(methodNames) == 0 {
		return ""
	}

	// Collect sibling files: files OTHER than targetFile that define a
	// method with any of these names. De-duplicate by file.
	siblingSet := make(map[string]bool)
	forEachMatchingDef(methodNames, graph, func(_, _, _ string, d *repomap.Symbol) bool {
		if d.File == "" || d.File == targetFile {
			return true
		}
		if strings.ToLower(d.Kind) != "method" {
			return true
		}
		siblingSet[d.File] = true
		return true
	})
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
	// Primary path: the LLM explicitly called emit_investigation_complete.
	// This is the clean, aligned completion signal — no heuristic ambiguity.
	if e.investigationComplete {
		logging.Debug("[explorer] ShouldStop iter=%d: investigation complete (explicit tool call)", iteration)
		return true
	}

	// Fallback S1: when the LLM soft-stops (no tool calls) in Phase 1
	// with ERM fully satisfied + terminal evidence, accept the stop even
	// though the LLM forgot to call emit_investigation_complete. This
	// preserves backward-compatible behavior for LLMs that don't use the
	// new tool yet, and catches the case where the soft-stop prompt
	// injection was ignored.
	if len(resp.ToolCalls) > 0 {
		return false
	}
	if e.phase != 1 || len(e.ermRequirements) == 0 {
		return false
	}
	var notesForCheck []string
	if resp.Content != "" {
		notesForCheck = append(notesForCheck, e.investigationNotes...)
		notesForCheck = append(notesForCheck, resp.Content)
	} else {
		notesForCheck = e.investigationNotes
	}
	e.ermRequirements = checkRequirementSatisfaction(e.ermRequirements, notesForCheck, e.structuredEvidence, e.complexity)
	if !ermAllSatisfied(e.ermRequirements) {
		return false
	}
	if primary := e.primaryEntityFiles(); len(primary) > 0 {
		if e.primaryReadIter == 0 || len(e.investigationNotes) <= e.notesLenAtPrimaryRead {
			return false
		}
	}
	if len(e.investigationNotes) == 0 {
		return false
	}
	noteEvidence := parseEvidenceItems(e.investigationNotes, "explorer.s1check")
	if !hasTerminalEvidence(noteEvidence) {
		return false
	}
	logging.Info("[explorer] S1 fallback early-stop iter=%d: ERM satisfied + terminal evidence but LLM did not call emit_investigation_complete",
		iteration)
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
// observeMidLoop is the mid-loop half of LoopController.Observe. It
// inspects the tool-result stream AFTER BaseAgent has executed a
// batch of tool calls for the current iteration and returns a
// LoopSignal the policy may accept as a corrective hint. Called from
// Observe(PhaseMidLoop); the throttle (at most every 3 iters; not
// before iter 3) is enforced by LoopPolicy.MinInjectInterval, so
// this function is called unconditionally and returns silently
// until conditions warrant a hint.
//
// Detection branches (in priority order, first match wins the HintKey):
//
//  1. Function-boundary coverage: LLM started reading a symbol but
//     hasn't finished the function body. "partial-read".
//  2. Enumeration completeness: question asks to list ALL X and
//     coverage is under 60% with ≥2 files unread. "enumeration".
//  3. Parallel tool-call cue: LLM has been in a serial-ish (≤2
//     calls/round) rhythm for ≥2 iters AND ≥2 files unread AND the
//     parallel hint hasn't fired yet this dispatch. "parallelize".
//
// Detection-only state (midLoopSerialStreak, midLoopLastResultsLen,
// midLoopParallelInjected) stays on the evaluator because it drives
// phase-specific behavior the policy can't express.
func (e *explorerEvaluator) observeMidLoop(obs LoopObservation) LoopSignal {
	e.ensureHeuristics()
	iteration := obs.Iteration
	allResults := obs.AllToolResults

	// Detect explicit completion signal from the LLM.
	// emit_investigation_complete is the explorer's terminal action —
	// stop immediately instead of burning one extra LLM round that
	// ShouldStop would catch anyway.
	if obs.LastToolResult != nil && obs.LastToolResult.ToolName == "emit_investigation_complete" && obs.LastToolResult.Success {
		e.investigationComplete = true
		logging.Info("[explorer] emit_investigation_complete observed at iter=%d", iteration)
		return LoopSignal{StopRequested: true, StopReason: "emit_investigation_complete called"}
	}

	// Track primary-entity file reads for S1's df3-drift gate. Runs
	// unconditionally so even "quiet" observeMidLoop calls still
	// update the read tracking — ShouldStop's gate depends on this
	// state being current. Idempotent: once primaryReadIter is set,
	// later calls are no-ops.
	e.observePrimaryRead(iteration, allResults)

	// Infer the current batch size from the allResults growth delta.
	// observeMidLoop is called once per ReAct iteration, after all
	// tool calls in the current batch have executed and their
	// results have been appended. Update the serial-streak counter
	// every call so the parallel-batching cue (Check 3) has
	// accurate degradation history when it fires.
	//
	// Threshold: batch <= 2 counts as "serial-ish" because the LLM's
	// most common low-parallelism pattern is a 1-grep + 1-read pair
	// per round — technically 2 calls, but sequential in intent (grep
	// to locate, then read what was found). Real parallelism means 3+
	// independent calls in one batch.
	currentBatch := len(allResults) - e.midLoopLastResultsLen
	serialThresh := e.heuristics.SerialBatchThreshold
	if e.midLoopLastResultsLen > 0 && currentBatch <= serialThresh {
		e.midLoopSerialStreak++
	} else if currentBatch > serialThresh {
		e.midLoopSerialStreak = 0
	}
	e.midLoopLastResultsLen = len(allResults)

	// The old "fire at most every 3 iters, not before iter N" throttle
	// now lives in LoopPolicy.MinInjectInterval. We still want the
	// "not before iter N" behavior — no mid-loop hint makes sense in
	// the very first iteration(s) because the LLM hasn't had a chance
	// to demonstrate a pattern yet — so we short-circuit here.
	if iteration < e.heuristics.MidLoopMinIteration {
		return LoopSignal{}
	}
	if e.searchResult == nil || e.searchResult.Graph == nil {
		return LoopSignal{}
	}

	var b strings.Builder
	// hintKey tracks which detection branch fired so the LoopPolicy
	// dedup window blocks only back-to-back repeats of the SAME
	// category (partial-read → partial-read) and lets different
	// categories through unimpeded (partial-read → enumeration).
	// Higher-priority checks overwrite lower-priority keys since
	// the final hint body reflects every fired check and the last
	// branch to fire names the most salient problem.
	var hintKey string

	// Check 1: function-boundary coverage.
	if hints := detectPartiallyReadSymbols(allResults, e.searchResult.Graph); len(hints) > 0 {
		h := hints[0] // worst-coverage offender
		unreadLines := h.symEnd - h.readEnd
		if unreadLines <= e.heuristics.PartialReadLineThreshold {
			// Small remainder: direct read is cheaper than grep+read.
			fmt.Fprintf(&b, "MID-LOOP CHECK: you read `%s` in `%s` up to line %d but the function spans lines %d-%d (%.0f%% covered, %d lines remaining). "+
				"If this function is relevant to the question, call read_file with path=%q offset=%d limit=%d to see the rest.\n",
				h.symbolName, h.file, h.readEnd, h.symStart, h.symEnd, h.coverage*100, unreadLines,
				h.file, h.readEnd+1, unreadLines)
		} else {
			// Large remainder: grep-then-read is the Phase 1 strategy.
			fmt.Fprintf(&b, "MID-LOOP CHECK: you read `%s` in `%s` up to line %d but the function spans lines %d-%d (%.0f%% covered, %d lines remaining). "+
				"If this function is relevant to the question, grep for key identifiers within `%s` (lines %d-%d) to find the important sections, then read those specific ranges.\n",
				h.symbolName, h.file, h.readEnd, h.symStart, h.symEnd, h.coverage*100, unreadLines,
				h.file, h.readEnd+1, h.symEnd)
		}
		hintKey = "explorer.mid-loop.partial-read"
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
			if coverage < e.heuristics.MidLoopEnumCoverage && len(discovered)-len(readSet) >= e.heuristics.EnumMidLoopUnreadFloor {
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
					if hintKey == "" {
						hintKey = "explorer.mid-loop.enumeration"
					}
				}
			}
		}
	}

	// Check 4: cross-file symbol reference push (T3b).
	//
	// When the LLM has read a file and written analysis notes that
	// mention exported symbol names, those symbols often refer to
	// types/methods defined in OTHER files. For a cross-package
	// mechanism question like "explorer是怎么调用subagent的？" the
	// LLM reads agent.go, sees `deps.SubAgents.Get` and writes a
	// note naming `SubAgent`; the type is defined in
	// subagent_runtime.go which is the key dispatch file. Without
	// this check, the LLM has no structural push to follow the
	// reference — the answer gets half-complete because it never
	// reads the downstream file.
	//
	// This check:
	//   1. Walks graph.SymbolDefs for exported symbols (len >= 6 to
	//      match the existing synthesis-prep scanner threshold;
	//      shorter names are too generic and cause false positives).
	//   2. Skips symbols that are already defined in a file the LLM
	//      read — no need to push "go read what you already read".
	//   3. Collects the top-N candidate files (by symbol count
	//      referenced in notes → more references = stronger signal)
	//      whose definitions are NOT in readSet.
	//   4. Emits a mid-loop hint pointing at those files.
	//
	// Fires at most once per dispatch (midLoopSymbolRefInjected flag)
	// so the hint doesn't recur on every subsequent iteration.
	if b.Len() == 0 && !e.midLoopSymbolRefInjected &&
		len(e.investigationNotes) >= e.heuristics.MidLoopMinIteration {
		_, readSet := extractFileCoverage(allResults)
		gaps := detectCrossFileSymbolGaps(
			e.investigationNotes, e.searchResult.Graph, readSet, 3)
		if len(gaps) > 0 {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString("MID-LOOP CHECK: your notes reference exported symbols whose " +
				"definitions live in files you have NOT read yet. Reading the defining " +
				"file is often the next hop of the call chain. Consider reading:\n")
			for _, g := range gaps {
				fmt.Fprintf(&b, "  - `%s` (defines `%s`)\n", g.File, g.Symbol)
			}
			b.WriteString("\n")
			e.midLoopSymbolRefInjected = true
			if hintKey == "" {
				hintKey = "explorer.mid-loop.cross-file-ref"
			}
		}
	}

	// Check 3: parallel tool-call cue.
	//
	// iter=0 reliably batches 3-8 tool calls because the initial
	// prompt sets up a seed-file scan. Mid-loop iterations degrade to
	// 1-2 tool calls per round because the LLM falls into a
	// serial-ish ReAct rhythm ("grep A → read A → grep B → read B"
	// or "read A, observe, think, read B, observe, ..."). Each
	// low-parallelism round pays full LLM round-trip latency, and
	// the 2026-04-13 latency audit measured ~3s per round. On a 15-
	// iter explorer this is where most of the remaining ReAct
	// latency lives AFTER the self-dispatch fix.
	//
	// Fire only when:
	//   - the LLM has been in a serial-ish (≤2 calls/round) rhythm
	//     for at least 2 iters — one low-parallelism round is noise,
	//     two is a pattern. The ≤2 threshold catches the common
	//     1-grep + 1-read pair which looks parallel but is
	//     sequential in intent.
	//   - at least 2 discovered files remain unread — otherwise there
	//     is nothing to parallelize
	//   - no partial-read hint was emitted above — those have higher
	//     priority and the LLM should finish that function first
	//   - the hint has not already been injected this dispatch — one
	//     nudge is enough; repeated nudges become noise and would
	//     starve other mid-loop checks
	//
	// The cue stays structural: it says "parallelize independent
	// operations, serialize when output of one determines the next"
	// and names no files. The LLM is the only party that sees the
	// notes and history, so it is the only party that can judge
	// independence; we just break the implicit low-parallelism
	// rhythm that the prior iterations established.
	if b.Len() == 0 && !e.midLoopParallelInjected && e.midLoopSerialStreak >= e.heuristics.SerialStreakThreshold {
		discovered, readSet := extractFileCoverage(allResults)
		unreadCount := 0
		for _, f := range discovered {
			if !readSet[f] && !isNoisePath(f) {
				unreadCount++
			}
		}
		if unreadCount >= e.heuristics.ParallelUnreadFloor {
			b.WriteString("MID-LOOP CHECK: you have been issuing only 1-2 tool calls per round for several iterations. " +
				"Batch independent operations together in a single assistant message (multiple tool_use blocks): " +
				"multiple `read_file` calls for unrelated files, multiple `grep` calls for different patterns/scopes, " +
				"or mixed `grep` + `read_file` calls that don't depend on each other. " +
				"For example, if you plan to read files A, B, and C whose contents are independent, call all three `read_file`s at once; " +
				"if you need to grep for patterns X and Y in different directories, batch both `grep` calls together. " +
				"Serialize only when one call's output determines the next call's parameters " +
				"(e.g. grep to find a line number, then read_file at that offset).\n")
			e.midLoopParallelInjected = true
			hintKey = "explorer.mid-loop.parallelize"
		}
	}

	if b.Len() == 0 {
		return LoopSignal{}
	}
	return LoopSignal{
		HintRequested: true,
		Hint:          b.String(),
		HintKey:       hintKey,
	}
}

// observeSoftStop is the soft-stop half of LoopController.Observe:
// called when the LLM produced content with no tool calls, and the
// policy needs to decide whether to accept the termination or inject
// a corrective continuation hint. Every branch in this method is a
// DETECTION branch — the throttle, dedup, budget, and idle-streak
// force-stop rules live in LoopPolicy and run over the returned
// LoopSignal.
//
// Where the old ContinuationPrompt read e.idleStreakInDepth, the
// new code reads obs.IdleStreak (snapshot of the policy's counter).
// Where the old code set e.idleStreakInDepth = 0 to "reset the
// streak because we have directed work", the new code sets the
// local `progress` flag and returns it as LoopSignal.Progress; the
// policy reacts by resetting its counter on the active signal.
//
// The old "if e.idleStreakInDepth >= 2 { return '', false }"
// terminal check is GONE: LoopPolicy.IdleStopThreshold enforces it
// at the policy layer so every evaluator gets the same termination
// semantics without re-implementing the count.
//
// The old `e.lastToolResultCount`-based tool-growth detector is also
// GONE: loopPolicyState.Apply tracks the tool-result count growth
// itself and resets the idle streak on growth.
func (e *explorerEvaluator) observeSoftStop(obs LoopObservation) LoopSignal {
	e.ensureHeuristics()
	resp := obs.Response
	iteration := obs.Iteration
	history := obs.AllToolResults
	// continuationCount / iteration are not currently referenced by
	// every branch — the policy owns the continuation budget so the
	// evaluator no longer needs to gate on it. Touching `iteration`
	// here keeps the name visible for a future detection branch
	// without triggering "declared and not used" diagnostics.
	_ = iteration

	// firstSoftStop encodes the "trust the LLM's first voluntary
	// stop unless hard evidence contradicts it" contract (added
	// 2026-04-15 to fix the structural "always intercept" bug this
	// function used to have):
	//
	//   - ContinuationsUsed == 0 means this dispatch has NOT yet
	//     had any soft-stop hint injected. The LLM produced content
	//     with no tool calls for the first time, which is the
	//     strongest "I'm done" signal the system gets. On this
	//     stop, only HARD-evidence branches (partial-read /
	//     enumeration / grep-redirect) are allowed to override the
	//     termination; SOFT heuristics (erm-gap / prescanned /
	//     unanalyzed) and the structural `coverage` fallthrough
	//     return an empty signal so the policy layer accepts the
	//     stop.
	//   - ContinuationsUsed >= 1 means the LLM already ignored at
	//     least one previous hint and stopped again. At that point
	//     the soft heuristics earn their seat — the LLM is clearly
	//     stalling, not finishing — so the full detector suite
	//     runs as before.
	//
	// Before this contract, every return path from this function
	// asserted `HintRequested: true`. Combined with LoopPolicy's
	// initial-state gates (dedup / throttle / budget) all being
	// no-ops on a first soft-stop, that meant the LLM's first
	// voluntary stop after any amount of tool work was
	// deterministically overridden — the iter=5 false positive
	// documented in the plan file.
	firstSoftStop := obs.ContinuationsUsed == 0
	logging.Debug("[explorer] observeSoftStop: phase=%d firstSoftStop=%v iter=%d contentLen=%d",
		e.phase, firstSoftStop, obs.Iteration, len(resp.Content))
	// progress is set to true by any branch that DETECTS forward
	// progress and wants the policy to reset the idle streak even
	// when no hint fires. The final return copies it into
	// LoopSignal.Progress.
	progress := false
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

	// If the LLM already called emit_investigation_complete, accept
	// the soft-stop immediately — no continuation prompts needed.
	if e.investigationComplete {
		return LoopSignal{Progress: progress}
	}

	if e.phase == 0 {
		// Completion is model-triggered via emit_investigation_complete
		// (see internal/tool/emit_investigation_complete.go — that tool's
		// docstring says it "replaces the implicit completion detection
		// that relied on ShouldStop heuristics and soft-stop interception").
		// The former Phase 0 early-exit heuristic accepted any soft-stop
		// that had a successful evidence-bearing tool in history; it
		// conflicted with this contract and, in REPL mode, accepted stops
		// where the LLM only listed a directory and said "next I'll read
		// the files" without actually reading any. The LLM must call
		// emit_investigation_complete (handled by the e.investigationComplete
		// guard above) to signal completion in Phase 0. Otherwise fall
		// through to the breadth-scan quality gate / Phase 1 transition.

		// Before transitioning to Phase 2, check if Phase 1 actually
		// discovered any files. If all greps returned zero results,
		// push the LLM to retry with broader patterns before moving on.
		discovered, _ := extractFileCoverage(history)
		if len(discovered) == 0 && e.broadenAttempts < e.heuristics.Phase0MaxBroadenAttempts {
			e.broadenAttempts++
			return LoopSignal{
				HintRequested: true,
				HintKey:       "explorer.phase0.broaden",
				Hint: "Your grep searches returned no file matches. Before moving to depth reading, " +
					"try broader search strategies:\n" +
					"- Drop any --include filter (search ALL file types)\n" +
					"- Use shorter or partial keywords (prefixes, stems) — e.g. instead of 'UserAuthenticationService' try 'UserAuth' or 'authentication'\n" +
					"- Use single common terms rather than compound phrases\n" +
					"- Try conceptual synonyms for the same idea\n\n" +
					"Run at least 2-3 new grep calls with files_only=true before producing your file list. If the repo is polyglot, use grep `file_type` to narrow by language.",
				Progress: progress,
			}
		}

		// Quality gate: before transitioning to Phase 1, verify the
		// breadth scan used enough discovery tools and found enough files.
		// At most 1 extra round (phase0ExtraRound prevents infinite loop).
		//
		// Pre-scan awareness: when keywordSearch ran at dispatch start it
		// already used repo_map to produce a ranked file list the LLM
		// can see (e.hasPrescanRepoMap + e.preScannedFiles). Without
		// counting those, the gate would demand a redundant runtime
		// repo_map call at the first soft-stop, which injects an extra
		// hint and consumes the LoopPolicy throttle window — so the
		// following iter's phase-transition hint gets dropped (iter
		// 3-1=2 < MinInjectInterval=3) and the explorer exits Phase 0
		// without ever delivering Phase 2 instructions to the LLM.
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
			structuralDone := usedOtherDiscovery || e.hasPrescanRepoMap
			// Merge pre-scanned files into the discovery count. Use a
			// set to dedupe overlap with runtime grep hits.
			uniq := make(map[string]struct{}, len(discovered)+len(e.preScannedFiles))
			for _, f := range discovered {
				uniq[f] = struct{}{}
			}
			for _, f := range e.preScannedFiles {
				uniq[f] = struct{}{}
			}
			totalDiscovered := len(uniq)
			minDisc := e.heuristics.Phase0MinDiscoveredFiles
			if !usedGrep || !structuralDone || totalDiscovered < minDisc {
				e.phase0ExtraRound = true
				var gate strings.Builder
				gate.WriteString("Before moving to depth reading, broaden your search:\n")
				if !usedGrep {
					gate.WriteString("- You haven't used grep yet. Search for key terms from the question with files_only=true.\n")
				}
				if !structuralDone {
					gate.WriteString("- Use repo_map (task_map view) to see structurally relevant files.\n")
				}
				if totalDiscovered < minDisc {
					fmt.Fprintf(&gate, "- You only discovered %d files. Use broader search patterns to find at least %d.\n", totalDiscovered, minDisc)
				}
				return LoopSignal{
					HintRequested: true,
					HintKey:       "explorer.phase0.quality-gate",
					Hint:          gate.String(),
					Progress:      progress,
				}
			}
		}

		// Phase 0 → Phase 1 transition: the LLM produced a breadth
		// scan summary. Now switch to depth reading with evidence
		// catalog mode: collect ALL facts, defer reasoning to synthesis.
		logging.Info("[explorer] Phase 0 → Phase 1 transition: breadth scan complete, entering evidence collection")
		e.phase = 1
		phaseTransitionHint := "## Now entering PHASE 2: Evidence Collection\n\n" +
			"Good — you have mapped the relevant territory. Now investigate the source files and collect evidence. " +
			"**Your job is to collect evidence, NOT to answer the question.** Reasoning happens later.\n\n" +
			"**Tools you should use** (pick the most efficient for each situation):\n" +
			"- `grep` — locate specific patterns, find line numbers, scan large files efficiently. Prefer grep over full-file reads when you only need specific sections\n" +
			"- `read_file` — read file contents (use offset/limit for targeted ranges)\n" +
			"- `grep` + `read_file` combo — for large files (>500 lines), grep to find relevant line numbers first, then read only those ranges\n" +
			"- `emit_evidence(items=[...])` — after gathering facts from a file, emit ALL evidence in ONE batch. Line numbers MUST come from the `read_file` gutter exactly\n\n" +
			"**Evidence format** (examples — adapt to what you find):\n" +
			"- `[DIRECT] functionName line N: <what this code establishes>` — e.g. a return value, a constant definition\n" +
			"- `[REGISTRATION] functionName line N: <what is registered, EXACT values>` — e.g. a handler binding, a route mapping\n" +
			"- `[CONDITIONAL] functionName line N: <what happens> IF <condition>` — e.g. a branch, a config-dependent path\n" +
			"- `[ABSENT] <what was expected but NOT found>` — e.g. a pattern you searched for that doesn't exist\n\n" +
			"**Key rules:**\n" +
			"- NEVER skip simple methods (`getName() { return \"x\" }`) — record them as evidence with exact return values\n" +
			"- For [REGISTRATION]: note EXACT concrete values, not just 'including X'\n" +
			"- For [CONDITIONAL]: note the exact condition, not 'when configured'\n" +
			"- Read function BODIES, not just signatures\n\n" +
			"Start investigating now."
		// Phase-transition is a HARD progress signal — the LLM has
		// completed Phase 0 and now moves into Phase 1.
		return LoopSignal{
			HintRequested: true,
			HintKey:       "explorer.phase-transition",
			Hint:          phaseTransitionHint,
			Progress:      true,
		}
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
			progress = true // keep the loop alive via LoopSignal.Progress
			var hint strings.Builder
			hint.WriteString("**Incomplete function reads detected.** If these functions are relevant to the question, finish reading them:\n\n")
			for _, ph := range partialHints {
				unreadLines := ph.symEnd - ph.readEnd
				if unreadLines <= e.heuristics.PartialReadLineThreshold {
					fmt.Fprintf(&hint, "- `%s` in %s (lines %d-%d): you read up to line %d (%.0f%%, %d lines remaining). "+
						"Call `read_file` with path=%q offset=%d limit=%d to see the rest\n",
						ph.symbolName, ph.file, ph.symStart, ph.symEnd,
						ph.readEnd, ph.coverage*100, unreadLines,
						ph.file, ph.readEnd+1, unreadLines)
				} else {
					fmt.Fprintf(&hint, "- `%s` in %s (lines %d-%d): you read up to line %d (%.0f%%, %d lines remaining). "+
						"Grep for key identifiers within `%s` (lines %d-%d) to find the important sections, then read those ranges\n",
						ph.symbolName, ph.file, ph.symStart, ph.symEnd,
						ph.readEnd, ph.coverage*100, unreadLines,
						ph.file, ph.readEnd+1, ph.symEnd)
				}
			}
			hint.WriteString("\nSkip any function that is not relevant to the user's question.")
			return LoopSignal{
				HintRequested: true,
				HintKey:       "explorer.phase1.partial-read",
				Hint:          hint.String(),
				Progress:      progress,
			}
		}
	}

	// Enumeration completeness: when the question asks to "list all X",
	// verify that the LLM has analyzed enough of the discovered files.
	// A coverage gap here means the enumeration will be incomplete.
	if e.isEnumerationQuery && len(discovered) > 0 {
		enumCoverage := float64(len(readSet)) / float64(len(discovered))
		if enumCoverage < e.heuristics.SoftStopEnumCoverage && len(unread) > 0 {
			var hint strings.Builder
			fmt.Fprintf(&hint, "**Enumeration completeness check:** This question asks to list ALL items. "+
				"You found %d matching files but only read %d (%.0f%% coverage). "+
				"For enumeration queries you must achieve ≥%.0f%% coverage.\n\n"+
				"Unread files:\n", len(discovered), len(readSet), enumCoverage*100, e.heuristics.SoftStopEnumCoverage*100)
			for _, f := range unread {
				hint.WriteString("- " + f + "\n")
			}
			hint.WriteString("\nRead these files to ensure your enumeration is complete. " +
				"Skip only files that are clearly unrelated (test helpers, documentation).")
			progress = true
			return LoopSignal{
				HintRequested: true,
				HintKey:       "explorer.phase1.enumeration",
				Hint:          hint.String(),
				Progress:      progress,
			}
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
		return LoopSignal{
			HintRequested: true,
			HintKey:       "explorer.phase1.grep-redirect",
			Hint:          hint.String(),
			Progress:      progress,
		}
	}

	// (The old "track consecutive no-tool-call rounds" block that
	// maintained e.lastToolResultCount / e.idleStreakInDepth is gone.
	// LoopPolicy owns both counters now: tool-result growth resets
	// the idle streak automatically in loopPolicyState.Apply, and
	// obs.IdleStreak below is the snapshot the policy already
	// updated before calling this Observe.)

	// --- ERM gap-directed file suggestions (SOFT heuristic) ---
	// Check which evidence requirements are still unsatisfied and suggest
	// specific files to read. Gated on !firstSoftStop because ERM's
	// checkRequirementSatisfaction is a string-match over investigation
	// notes + evidence entries — paraphrased or translated prose tags
	// the requirement as "unsatisfied" even when the LLM answered the
	// question correctly. On the first voluntary stop we trust the
	// LLM's self-assessment; on subsequent stops, ERM gets to vote.
	if !firstSoftStop && len(e.ermRequirements) > 0 && e.searchResult != nil && e.searchResult.Graph != nil {
		e.ermRequirements = checkRequirementSatisfaction(e.ermRequirements, e.investigationNotes, e.structuredEvidence, e.complexity)
		logERM(e.ermRequirements)
		if !ermAllSatisfied(e.ermRequirements) {
			suggestions := ermSuggestFiles(e.searchResult.Graph, e.ermRequirements, readSet, e.heuristics.ErmSuggestLimit)
			if len(suggestions) > 0 {
				var hint strings.Builder
				hint.WriteString(ermUnsatisfiedGaps(e.ermRequirements))
				hint.WriteString("**Suggested files to fill these gaps** (read them NOW):\n\n")
				for _, s := range suggestions {
					hint.WriteString(fmt.Sprintf("- `%s` (score=%.1f) — %s\n", s.Path, s.Score, s.Reason))
				}
				hint.WriteString("\nCall `read_file` on the top suggestion immediately. ")
				hint.WriteString("Extract structured evidence with [DIRECT], [REGISTRATION], [CONDITIONAL] tags.\n")
				// Directed ERM work resets the idle counter.
				progress = true
				return LoopSignal{
					HintRequested: true,
					HintKey:       "explorer.phase1.erm-gap",
					Hint:          hint.String(),
					Progress:      progress,
				}
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
			// Methods and functions: short names (catches Name, Run, Get).
			// Other symbols: longer names (avoids noise from generic names).
			minLen := e.heuristics.SymbolMinLenOther
			if kind == "method" || kind == "function" {
				minLen = e.heuristics.SymbolMinLenMethod
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
	// (SOFT heuristic, gated on !firstSoftStop.) The preScannedFiles
	// list is a top-N keyword-search recall result, NOT a precision
	// list — half its members are expected to be noise, and "unread"
	// does not imply "should have been read". On the first voluntary
	// stop we trust the LLM to have picked the relevant members; on
	// subsequent stops the push counter fires as before.
	//
	// Track push attempts: if the LLM ignores file-reading requests
	// repeatedly (same unread count), escalate then give up.
	if !firstSoftStop && len(preScannedUnread) > 0 {
		// Check whether the LLM made progress since the last push.
		if len(preScannedUnread) >= e.lastPreScannedUnreadCount && e.lastPreScannedUnreadCount > 0 {
			e.preScannedPushCount++
		} else {
			e.preScannedPushCount = 1
		}
		e.lastPreScannedUnreadCount = len(preScannedUnread)

		// After 3 failed pushes, stop resetting idle streak so the
		// loop terminates naturally. The LLM is clearly not going to
		// read these files — wasting more rounds won't help. Setting
		// progress=true here tells LoopPolicy to reset the idle
		// streak for this iteration only; after the third push we
		// leave progress=false so the policy's IdleStopThreshold
		// catches the drift.
		if e.preScannedPushCount <= e.heuristics.MaxPreScannedPushes {
			progress = true
		}

		var hint strings.Builder
		fmt.Fprintf(&hint,
			"File coverage: %d read out of %d discovered.\n", len(readSet), len(discovered))
		fmt.Fprintf(&hint, "\nReminder — user question: %s\n\n", e.userQuestion)

		if e.preScannedPushCount >= e.heuristics.MaxPreScannedPushes {
			// Final forceful push: name the single most important file.
			fmt.Fprintf(&hint, "STOP ANALYZING. You have NOT read %d critical files. Call read_file on this file RIGHT NOW:\n", len(preScannedUnread))
			hint.WriteString("  " + preScannedUnread[0])
			if syms := e.fileSymbols[preScannedUnread[0]]; len(syms) > 0 {
				hint.WriteString(" — defines: " + strings.Join(syms, "; "))
			}
			hint.WriteString("\n")
			if guidance := e.formatReadFileOffsetGuidance(preScannedUnread[0]); guidance != "" {
				hint.WriteString(guidance)
			}
			hint.WriteString("\nDo NOT write any analysis. Your ONLY action should be a read_file tool call.")
		} else if e.preScannedPushCount >= e.heuristics.MaxPreScannedPushes-1 {
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
			if guidance := e.formatReadFileOffsetGuidance(preScannedUnread[0]); guidance != "" {
				hint.WriteString(guidance)
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
			if guidance := e.formatReadFileOffsetGuidance(preScannedUnread[0]); guidance != "" {
				hint.WriteString(guidance)
			}
			hint.WriteString("\nRead the most important unread file and extract ALL evidence entries from it. " +
				"Remember: collect facts, do not answer the question yet.")
		}
		return LoopSignal{
			HintRequested: true,
			HintKey:       "explorer.phase1.prescanned",
			Hint:          hint.String(),
			Progress:      progress,
		}
	}

	// If files were read but have symbols the LLM didn't analyze,
	// push for analysis of those specific symbols. (SOFT heuristic,
	// gated on !firstSoftStop.) The matching logic is a literal
	// substring search of each symbol name against the joined
	// investigation notes — "Run", "Get", "Set", "New" (3-char
	// methods) match just about any note, and any paraphrase or
	// translation of the LLM's understanding slips past the match.
	// On the first voluntary stop we defer to the LLM's
	// self-assessment; on subsequent stops this branch still runs.
	if !firstSoftStop && len(unanalyzed) > 0 {
		progress = true
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
		return LoopSignal{
			HintRequested: true,
			HintKey:       "explorer.phase1.unanalyzed",
			Hint:          hint.String(),
			Progress:      progress,
		}
	}

	// The old terminal `if e.idleStreakInDepth >= 2 { return "", false }`
	// check is GONE: LoopPolicy.IdleStopThreshold=2 force-stops at
	// the policy layer, so every evaluator gets identical soft-stop
	// termination semantics without re-implementing the count.

	// STRUCTURAL fallthrough: when none of the hard-evidence
	// detectors fired and none of the soft heuristics were eligible,
	// this branch used to unconditionally return HintRequested=true
	// with a "File coverage: X read out of Y discovered" hint — the
	// structural "always intercept" bug that made `observeSoftStop`
	// incapable of ever accepting a soft-stop. On the first
	// voluntary stop we now return an empty signal (preserving
	// `progress` if any earlier detector set it, so the policy's
	// idle counter still resets appropriately). On subsequent
	// voluntary stops we fall through to the historical coverage
	// hint so long-running dispatches where the LLM repeatedly
	// stops without making progress still get a nudge toward any
	// unread files.
	if firstSoftStop {
		// Completion is model-triggered. If e.investigationComplete were
		// already true, the early guard at the top of this function would
		// have returned before reaching this point — so here we know the
		// LLM has NOT signalled completion. Nudge it to call
		// emit_investigation_complete (or resume investigating). The
		// previous branch accepted the stop whenever any evidence-bearing
		// tool had run, which bypassed the contract that emit_investigation_complete
		// is the sole completion trigger.
		return LoopSignal{
			HintRequested: true,
			HintKey:       "explorer.completion-tool-reminder",
			Hint: "You stopped without calling emit_investigation_complete. " +
				"If you have collected enough evidence to answer the user's question, " +
				"call emit_investigation_complete(reason, confidence) now. " +
				"If you still need more evidence, continue reading files and running greps.",
			Progress: progress,
		}
	}

	// When the LLM is slowing down (idle ≥ 1), inject a preview of
	// programmatically extracted concrete values. This breaks the
	// information asymmetry between collection and synthesis phases:
	// the LLM can see what the programmatic layer already knows and
	// focus its remaining reads on gaps only it can fill (semantic
	// relationships, complex conditions, cross-file reasoning).
	var cvPreview string
	if obs.IdleStreak >= 1 && len(e.investigationNotes) >= 2 && e.searchResult != nil {
		// CGEC: closure=nil for the soft-stop preview path. Preview is
		// shown as a hint to the LLM about what the deterministic
		// scanner already knows; it is NOT a citation source. The
		// canonical evidence channels at the ParseOutput call sites
		// below pass closure so promotion fires there.
		cvPreview = e.getConcreteValuesCached(e.repoRoot, readSet, nil).markdown
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
		if len(cvPreview) > e.heuristics.CVPreviewMaxLen {
			cvPreview = cvPreview[:e.heuristics.CVPreviewMaxLen] + "\n... [preview truncated]\n"
		}
		hint.WriteString(cvPreview)
		hint.WriteString("\n**Focus your remaining investigation on:**\n")
		hint.WriteString("- Cross-file relationships that the table above does NOT show\n")
		hint.WriteString("- Conditions whose resolution requires reading function bodies\n")
		hint.WriteString("- Semantic intent behind registrations (WHY something is registered, not just WHAT)\n")
	}

	return LoopSignal{
		HintRequested: true,
		HintKey:       "explorer.phase1.coverage",
		Hint:          hint.String(),
		Progress:      progress,
	}
}

// formatReadFileOffsetGuidance renders a concise multi-line hint
// fragment reminding the LLM that read_file supports offset+limit
// and showing, for the given file, the top 3 symbols with their
// line ranges plus ONE concrete read_file invocation example
// covering the union. Returns "" when no symbol info is available
// (graph miss / empty file).
//
// Motivation (log trace 1776454589211679465): every single
// read_file call in a 16-iteration explore window used offset=0,
// even when repo_map had told the LLM the relevant function lived
// at lines 100-200 of a 5000-line file. The LLM was reading the
// first 1000 lines of irrelevant code each time, which (a) wasted
// context budget, (b) often missed the actual symbol (past line
// 1000), (c) slowed the pipeline enough to hit rate limits. The
// schema description says "use offset/limit for targeted ranges"
// but the LLM doesn't parse schema descriptions, only hints it
// receives inline.
//
// Output shape:
//
//	  → Tip: read_file supports offset+limit for targeted reads.
//	    Symbols in <path>:
//	      - NewSubExplorer (lines 25-34)
//	      - SubExplorer.Run (lines 35-65)
//	      - subExplorerEvaluator (lines 67-107)
//	    Example: read_file path=<path> offset=25 limit=83 to cover lines 25-107.
func (e *explorerEvaluator) formatReadFileOffsetGuidance(path string) string {
	if e.searchResult == nil || e.searchResult.Graph == nil {
		return ""
	}
	fi, ok := e.searchResult.Graph.FileIndex[path]
	if !ok || fi == nil || len(fi.Symbols) == 0 {
		return ""
	}
	type ranged struct {
		name  string
		start int
		end   int
	}
	var syms []ranged
	for i := range fi.Symbols {
		s := &fi.Symbols[i]
		if s.Line <= 0 {
			continue
		}
		end := s.EndLine
		if end < s.Line {
			end = s.Line
		}
		syms = append(syms, ranged{name: s.Name, start: s.Line, end: end})
	}
	if len(syms) == 0 {
		return ""
	}
	sort.SliceStable(syms, func(i, j int) bool { return syms[i].start < syms[j].start })
	topN := 3
	if len(syms) < topN {
		topN = len(syms)
	}
	var b strings.Builder
	b.WriteString("  → Tip: read_file supports offset+limit for targeted reads (avoid re-reading from offset=0).\n")
	fmt.Fprintf(&b, "    Symbols in %s:\n", path)
	for i := 0; i < topN; i++ {
		fmt.Fprintf(&b, "      - %s (lines %d-%d)\n", syms[i].name, syms[i].start, syms[i].end)
	}
	// Concrete example spanning the top-N union.
	offset := syms[0].start
	coverEnd := syms[topN-1].end
	limit := coverEnd - offset + 1
	fmt.Fprintf(&b, "    Example: read_file path=%s offset=%d limit=%d  (covers lines %d-%d).\n",
		path, offset, limit, offset, coverEnd)
	return b.String()
}

// Observe implements LoopController. Dispatches to observeMidLoop or
// observeSoftStop based on the LoopPhase in the observation. See the
// two helper methods for the detection logic; this function is pure
// phase routing.
func (e *explorerEvaluator) Observe(_ *types.AgentContext, obs LoopObservation) LoopSignal {
	switch obs.Phase {
	case PhaseMidLoop:
		return e.observeMidLoop(obs)
	case PhaseSoftStop:
		return e.observeSoftStop(obs)
	}
	return LoopSignal{}
}

func (e *explorerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, mcpResponses []types.MCPResponse) (*StageOutput, error) {
	// P1.2 → 2026-04-16: the explorer no longer implements
	// SynthesizingEvaluator. The synthesis LLM call was deleted
	// because its prose output was never consumed — StageReport is
	// produced by the deterministic renderExplorerStageReport, and
	// out.Data is `{}`. The structured side effects that used to
	// live in SynthesisPrompt (concrete-value extraction, evidence
	// merge) now run directly in ParseOutput above.
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

	// Merge concrete values into structured evidence. Before the
	// synthesis-LLM removal (2026-04-16) this merge lived inside
	// SynthesisPrompt; now ParseOutput is the sole owner.
	_, cvReadSet := extractFileCoverage(toolResults)
	// CGEC: pass the per-Run EvidenceClosure so the chain promotion
	// enforcer can demote chains anchored outside ReadSet and queue
	// the missing files as PendingReads on the closure. Update the
	// closure's ReadSet snapshot first so any other CGEC consumer
	// (pre-complete check, stall detector) sees the latest view.
	var cvClosure *types.EvidenceClosure
	if ctx.Mutable != nil {
		cvClosure = ctx.Mutable.EvidenceClosure()
		cvClosure.SetReadSet(cvReadSet)
	}
	// CGEC B1a: Phase 0 broaden attempts exhausted with 0 file
	// matches → emit RepairExpandSearch so the next retry's prompt
	// tells the LLM to try keyword stems / morphological variants
	// / synonyms instead of repeating the same terms. The Keywords
	// field carries the analyzer-declared keywords the LLM has
	// already been trying (so the retry hint can surface them as
	// "these didn't work — try different forms"). De-dup by
	// AddRepair chokepoint.
	discoveredFiles, _ := extractFileCoverage(toolResults)
	if cvClosure != nil &&
		e.phase == 0 &&
		e.broadenAttempts >= e.heuristics.Phase0MaxBroadenAttempts &&
		len(discoveredFiles) == 0 {
		var kws []string
		if ctx.AnalysisIR != nil {
			kws = append(kws, ctx.AnalysisIR.RequestModel.AnalyzerHints.Keywords...)
		}
		cvClosure.AddRepair(types.RepairDirective{
			Kind:      types.RepairExpandSearch,
			Keywords:  kws,
			Rationale: fmt.Sprintf("Phase 0 exhausted %d broaden attempt(s) with 0 file matches — try stems, morphological variants, or conceptual synonyms of these keywords", e.broadenAttempts),
			Origin:    "explorer.phase0.broaden_exhausted",
		})
		logging.Info("[CGEC] B1a expand_search: origin=phase0.broaden_exhausted attempts=%d keywords=%d", e.broadenAttempts, len(kws))
	}
	cvResult := e.getConcreteValuesCached(ctx.RepoRoot, cvReadSet, cvClosure)
	if len(cvResult.evidence) > 0 {
		e.structuredEvidence = mergeEvidenceItems(e.structuredEvidence, cvResult.evidence)
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
	// Single-source investigation bypass: when exactly one type of
	// high-confidence evidence tool was used, the multi-dimensional
	// quality floor is inapplicable. The LLM answered using a single
	// tool type — file counts via exec_command, version checks via
	// read_file, existence checks via grep, directory listings via
	// list_files. If the answer is insufficient, the AnswerContract
	// checker will backtrack via the retry budget.
	if len(sources) == 1 {
		toolDiversity = true
		fileCoverage = true
		evidenceQuality = true
	}
	hasEnough := toolDiversity && fileCoverage && evidenceQuality

	// Primary signal: the LLM explicitly called emit_investigation_complete.
	// When set, it overrides ALL heuristic calculations — the LLM
	// declared it has enough evidence, and we trust it.
	if ctx != nil && ctx.Mutable != nil && ctx.Mutable.IsInvestigationComplete() {
		if !hasEnough {
			logging.Debug("[explorer] HasEnoughFacts promoted by emit_investigation_complete (heuristic was: toolDiv=%v fileCov=%v evQual=%v)",
				toolDiversity, fileCoverage, evidenceQuality)
		}
		hasEnough = true
	} else {
		// Fallback: ERM quality gate (only when no explicit signal).
		if len(e.ermRequirements) > 0 {
			e.ermRequirements = checkRequirementSatisfaction(e.ermRequirements, e.investigationNotes, e.structuredEvidence, e.complexity)
			allSat := ermAllSatisfied(e.ermRequirements)
			if hasEnough && !allSat {
				unsatCount := 0
				for _, r := range e.ermRequirements {
					if r.Status == "unsatisfied" {
						unsatCount++
					}
				}
				if unsatCount > 0 {
					logging.Debug("[explorer] ERM gate: %d unsatisfied requirements, demoting hasEnough", unsatCount)
					hasEnough = false
				}
			} else if !hasEnough && allSat {
				logging.Debug("[explorer] ERM all-satisfied promote: hasEnough=true (quantitative floors: toolDiv=%v fileCov=%v evQual=%v)",
					toolDiversity, fileCoverage, evidenceQuality)
				hasEnough = true
			}
		}
	}

	signals := &types.ExecutionSignals{HasEnoughFacts: hasEnough}

	// Rank evidence and findings by relevance to the user's question
	// so downstream consumers (finalizer) get the most useful items first.
	// Subject-aware variant boosts items whose Object / Summary tail
	// token matches the expected AnswerSubject kind — so the chain of
	// "Config assigns 'explore-skill'" out-ranks the generic
	// "NewExplorerAgent returns ..." when subject=skill_name.
	var rankGraph *repomap.Graph
	if e.searchResult != nil {
		rankGraph = e.searchResult.Graph
	}
	rankedEvidence := rankEvidenceByRelevanceWithSubject(e.userQuestion, e.structuredEvidence, readSet, e.answerSubject, rankGraph, e.predicateAxis)
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
	// Session 11 G7 — install the R3 axis-aware ledger hook so
	// identifyAnswerChains can record ViolChainDemoted entries via
	// the ambient callback instead of carrying a closure pointer
	// through the ranking helpers. Clear the hook immediately
	// after to keep package state clean between dispatches.
	if ctx != nil && ctx.Mutable != nil {
		closure := ctx.Mutable.EvidenceClosure()
		SetLedgerHook(func(v types.Violation) { closure.AppendViolation(v) })
		defer SetLedgerHook(nil)
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
	// β counts DISTINCT answer terminals, not raw evidence items.
	// When multiple strict chains converge on the same terminal
	// (e.g. two chains both ending in `Name() returns "explorer"`),
	// they describe ONE answer — inflating β with per-chain counts
	// pushes the extractor's cardinality validator to demand a slate
	// that over-populates with mechanism nodes just to clear floor.
	// Chains whose terminal is unparseable (key == "") count
	// independently so we never under-count by collapsing legitimately
	// distinct answers into one.
	terminalEvidenceCount := 0
	seenTerminals := make(map[string]bool)
	for _, c := range answerChains {
		if !c.StrictOK {
			continue
		}
		if !hasTerminalEvidence([]types.EvidenceItem{c.Item}) {
			continue
		}
		key := normalizedChainTerminal(c.Item.Summary)
		if key == "" {
			terminalEvidenceCount++
			continue
		}
		if seenTerminals[key] {
			continue
		}
		seenTerminals[key] = true
		terminalEvidenceCount++
	}
	logging.Debug("[explorer] terminalEvidenceCount=%d (slate deferred to Turn B; distinct terminals=%d)", terminalEvidenceCount, len(seenTerminals))

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
	// BuildInitialInstruction sees the complete payload.
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
		snapshot := types.TurnAArtifacts{
			UserQuestion:          e.userQuestion,
			InvestigationNotes:    e.investigationNotes,
			ReadFiles:             readFilesList,
			ToolResults:           toolResults,
			EvidenceItems:         strictEvidence,
			FlowFindings:          rankedFindings,
			TerminalEvidenceCount: terminalEvidenceCount,
		}
		// Cross-window accumulation. When the DAG scheduler requeues
		// the explore node (e.g. SuccessCriteria failed → retry window),
		// BaseAgent re-dispatches the explorer with a fresh ReAct
		// history, so `toolResults` and `readFilesList` only reflect
		// the current window. Overwriting would erase Window 1's
		// ReadFiles / ToolResults / strict evidence, which then
		// triggers extractorInvestigationEmpty's R4 fail-loud even
		// though investigation was rich. e.investigationNotes and
		// ctx.Mutable.EmittedEvidence() already accumulate across
		// windows via the cross-run reset gate — these four fields
		// must match that semantics via merge.
		snapshot = mergeTurnAArtifactsWithPrior(ctx.Mutable.TurnAArtifacts(), snapshot)
		ctx.Mutable.SetTurnAArtifacts(snapshot)
		logging.Debug("[explorer] turn A → turn B handoff: wrote TurnAArtifacts (%d notes, %d readFiles, %d toolResults, %d evidence, %d flow, termCount=%d)",
			len(snapshot.InvestigationNotes), len(snapshot.ReadFiles), len(snapshot.ToolResults), len(snapshot.EvidenceItems), len(snapshot.FlowFindings), snapshot.TerminalEvidenceCount)
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
	// Grounding moved upstream. emit_evidence.Execute now grounds every
	// LLM-emitted item synchronously (Tier 1 line_text + Tier 2
	// symbol_table via repomap, plus recovery tiers) and attaches
	// GroundingStatus to each item before AppendEvidence. Items parsed
	// from raw investigationNotes (rare — the LLM is prompted to go
	// through emit_evidence) still land here ungrounded, but their
	// GroundingStatus is left empty and the downstream renderer treats
	// empty as "not submitted via the structured channel" — equivalent
	// to ungrounded for citation-pool purposes.
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
		var ermClosure *types.EvidenceClosure
		if ctx.Mutable != nil {
			ermClosure = ctx.Mutable.EvidenceClosure()
		}
		cv := e.getConcreteValuesCached(ctx.RepoRoot, readSet, ermClosure)
		// T2.2: produce structured EvidenceMechanism items for ERM
		// mechanism requirements. No-op for non-mechanism questions.
		mechEvidence = scanMechanismEvidence(e.ermRequirements, e.searchResult.Graph, ctx.RepoRoot)
		trial := mergeEvidenceItems(parsed, cv.evidence)
		if len(mechEvidence) > 0 {
			trial = mergeEvidenceItems(trial, mechEvidence)
		}
		reqsCopy := make([]EvidenceRequirement, len(e.ermRequirements))
		copy(reqsCopy, e.ermRequirements)
		reqsCopy = checkRequirementSatisfaction(reqsCopy, e.investigationNotes, trial, e.complexity)
		if ermAllSatisfied(reqsCopy) {
			logging.Debug("[explorer] T1.1 gate: ERM all satisfied by parsed(%d)+concreteValues(%d)+mechanism(%d) — skipping dataflow.Analyze",
				len(parsed), len(cv.evidence), len(mechEvidence))
			// Merge mechanism evidence into structuredEvidence so it
			// reaches the finalizer regardless of whether dataflow runs.
			// Concrete Values are merged in ParseOutput via
			// getConcreteValuesCached + mergeEvidenceItems.
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
//
// chainAnchors is the CGEC parallel array — one entry per chain in
// the order chains were built, listing the source-file anchors each
// chain depends on. The chain promotion helper consults this when
// closure is non-nil to demote chains anchored outside Turn A's
// ReadSet, so they cannot leak into the prompt as suggestions the
// LLM would have to cite at unread file:line coordinates.
type concreteValuesResult struct {
	markdown     string
	evidence     []types.EvidenceItem
	chainAnchors []chainAnchorInfo
}

// chainAnchorInfo records which source files a Resolution Chain or
// dataflow_path EvidenceItem rests on. Used by the chain promotion
// helper to enforce CGEC invariant I1 (every file:line surfaced in a
// downstream prompt must be in ReadSet). When promotion fires, the
// file paths in Files are appended to MutableState.EvidenceClosure
// .pendingReads so the next explore round's retry hint surfaces them
// as a "Forced Read List".
//
// Summary mirrors the chain markdown line / EvidenceItem.Summary so
// callers can match an anchor entry back to the chain it describes
// without re-parsing the rendered text.
type chainAnchorInfo struct {
	Summary string
	Files   []string
	Origin  string // "concrete_values_tracer", "bridge_literal", "hierarchy"
}

// getConcreteValuesCached builds concrete values once per Execute and
// caches the result for reuse by both the dataflow-skip gate (T1.1) and
// the SynthesisPrompt section. Subsequent calls return the cached value
// regardless of readSet drift; this is safe because both call sites run
// near the end of the loop with effectively the same toolResults.
//
// closure (CGEC) is optional. When non-nil the cached unfiltered
// result is run through applyChainPromotion: chains anchored outside
// ReadSet are stripped from the markdown and from the dataflow_path
// evidence items, and the missing anchor files are appended to
// closure.PendingReads. The cache itself stores the unfiltered
// version because closure state mutates over the explore loop and
// freezing a closure-aware snapshot would serve stale filters to a
// later caller.
//
// subject (CGEC C2) is the answer-subject classification produced by
// the analyzer. When non-Unknown the chain ranker scores each chain
// terminal against the expected subject kind and re-orders chains so
// the most subject-relevant ones win the chainCap slot. Zero value
// (SubjectUnknown) preserves the historical insertion-order ranking.
// Subject is constant for the Run, so caching on (repoRoot, readSet)
// alone is sound.
func (e *explorerEvaluator) getConcreteValuesCached(repoRoot string, readSet map[string]bool, closure *types.EvidenceClosure) concreteValuesResult {
	if e.cachedConcreteValues == nil {
		r := e.buildConcreteValuesSection(repoRoot, readSet, closure)
		e.cachedConcreteValues = &r
	}
	if closure == nil {
		return *e.cachedConcreteValues
	}
	return applyChainPromotion(*e.cachedConcreteValues, readSet, closure, repoRoot)
}

// applyChainPromotion is the CGEC chain-promotion enforcer. Given an
// unfiltered concreteValuesResult and a snapshot of Turn A's
// ReadSet, it returns a new result whose markdown's "### Resolution
// Chains" section drops any chain anchored outside ReadSet, whose
// evidence slice drops dataflow_path EvidenceItems with Source
// outside ReadSet, and which records every missing anchor file as a
// PendingRead on the closure (so the next explore round's retry
// hint surfaces them via the structured RepairReadFile directive).
//
// Pure of side effects on the input — the input result is shared
// from the cache and must remain unfiltered.
//
// Why filter both markdown and evidence: the markdown chains land
// in the explorer's synthesis prompt section, so an unread-anchor
// chain can mislead the explorer LLM directly. The dataflow_path
// EvidenceItems land in TurnAArtifacts and the extractor / finalizer
// prompt, so an unread-anchor item can mislead Turn B. Both render
// paths must be cleaned for the invariant to hold.
func applyChainPromotion(in concreteValuesResult, readSet map[string]bool, closure *types.EvidenceClosure, repoRoot string) concreteValuesResult {
	if closure == nil || len(in.chainAnchors) == 0 {
		return in
	}
	keptSummaries := make(map[string]bool, len(in.chainAnchors))
	demotedSummaries := make(map[string]bool, len(in.chainAnchors))
	for _, anchor := range in.chainAnchors {
		allRead := true
		for _, f := range anchor.Files {
			if !readSet[f] {
				allRead = false
			}
		}
		if allRead {
			keptSummaries[anchor.Summary] = true
			continue
		}
		demotedSummaries[anchor.Summary] = true
		// Append PendingRead for every anchor file the closure has
		// not seen. Origin tags the source so the operator can grep
		// the trace for which enforcer raised the read.
		//
		// CGEC D2: filter by ScannedSet membership. A chain anchor
		// pointing at a file the explorer's pre-scan never saw is
		// almost certainly a ghost path (the concrete-values tracer
		// built it from an identifier token that happens to match a
		// path string elsewhere, or the LLM emitted a bad bridge).
		// Force-reading a ghost path wastes a read slot and
		// pollutes ReadSet with irrelevant content. If IsScanned
		// returns true (including the empty-ScannedSet pass-through
		// for old tests / analyzer-only dispatches), the PendingRead
		// is enqueued as before; if false, we skip and log so the
		// operator can see the filter fired.
		for _, f := range anchor.Files {
			if readSet[f] {
				continue
			}
			if !closure.IsScanned(f) {
				logging.Debug("[CGEC] D2 chain_promotion: skipping ghost anchor file=%s origin=%s (not in ScannedSet)", f, anchor.Origin)
				// Session 11 F1: record structured ledger entry so F2
				// aggregator can promote repeated ghost anchors on the
				// same file to a RetrievalGap signal (→ R5 expand_search).
				closure.AppendViolation(types.Violation{
					Kind:   types.ViolGhostAnchor,
					Detail: fmt.Sprintf("chain anchor file=%s origin=%s not in ScannedSet", f, anchor.Origin),
					Stage:  string(types.StageExplore),
					EvidenceRefs: []string{f, "origin:" + anchor.Origin},
					SuspectedRoot: types.SuspectedRoot{
						IRField:    "ScannedSet",
						Reason:     "chain anchored outside explorer pre-scan; ranker likely missed file",
						Confidence: 0.70,
					},
				})
				// Session 11 R5 — reactive feedback: when the ghost
				// anchor count for this specific file crosses a
				// threshold AND the file actually exists on disk, the
				// ranker is almost certainly missing a real answer
				// source. Add the file to ScannedSet so the next D2
				// pass accepts the chain, and raise a RepairExpandSearch
				// directive for retry-hint visibility. The promotion
				// is per-file one-shot (subsequent ghost anchors on
				// the same file no-op because IsScanned now returns
				// true).
				r5Threshold := tool.CurrentAnalysisLimits().GhostAnchorExpandSearchThreshold
				if r5Threshold > 0 &&
					countGhostAnchorsForFile(closure, f) >= r5Threshold &&
					fileExistsInRepo(repoRoot, f) {
					updated := closure.ScannedSet()
					if updated == nil {
						updated = make(map[string]bool)
					}
					updated[f] = true
					closure.SetScannedSet(updated)
					closure.AddRepair(types.RepairDirective{
						Kind:      types.RepairExpandSearch,
						Files:     []string{f},
						Rationale: fmt.Sprintf("R5 promote: %d ghost-anchor hits on %s — ranker missed a real file", r5Threshold, f),
						Origin:    "chain_promotion.r5_expand",
					})
					logging.Info("[CGEC] R5 expand_search: promoted %s to ScannedSet after %d ghost anchor hits", f, r5Threshold)
				}
				continue
			}
			closure.AddPendingRead(types.PendingRead{
				File:      f,
				Rationale: "Resolution Chain anchors here but file is outside Turn A ReadSet — read it before next emit_investigation_complete",
				Origin:    "chain_promotion." + anchor.Origin,
			})
		}
	}
	if len(demotedSummaries) == 0 {
		// All chains kept — fast path returns the unmodified input.
		return in
	}
	closure.BumpChainsDemoted(len(demotedSummaries))

	// Filter the markdown's "### Resolution Chains" section. Chains
	// in that section are bullet lines `- summary`; we drop the line
	// when the trailing summary is in demotedSummaries.
	out := in
	out.markdown = filterResolutionChainSection(in.markdown, demotedSummaries)

	// Filter dataflow_path evidence items. Match on Subject (the
	// chain summary) so per-file tracer items (no Source) are also
	// caught. Items whose summary is neither in keep nor demote
	// (e.g. items added by other producers we did not track) are
	// kept conservatively.
	if len(in.evidence) > 0 {
		filtered := make([]types.EvidenceItem, 0, len(in.evidence))
		dropped := 0
		for _, it := range in.evidence {
			if it.Kind == types.EvidenceDataflowPath && it.Predicate == "resolution_chain" {
				if demotedSummaries[it.Subject] || demotedSummaries[it.Summary] {
					dropped++
					continue
				}
			}
			filtered = append(filtered, it)
		}
		if dropped > 0 {
			logging.Debug("[explorer] chain promotion: dropped %d dataflow_path items anchored outside ReadSet", dropped)
		}
		out.evidence = filtered
	}
	logging.Debug("[explorer] chain promotion: kept %d / demoted %d chains; pending reads queued",
		len(keptSummaries), len(demotedSummaries))
	return out
}

// filterResolutionChainSection rewrites the "### Resolution Chains"
// section of a concrete-values markdown blob, dropping any bullet
// line whose body equals one of the demoted chain summaries. Other
// sections are left untouched.
//
// The chain bullets have shape `- <summary>\n` where summary is the
// raw chain text the producer appended at line 3148. We match on the
// trailing-newline + leading "- " pair so partial substring matches
// inside other sections cannot accidentally be filtered.
func filterResolutionChainSection(md string, demoted map[string]bool) string {
	const header = "### Resolution Chains\n"
	headerIdx := strings.Index(md, header)
	if headerIdx < 0 {
		return md // section not present, nothing to filter
	}
	before := md[:headerIdx+len(header)]
	rest := md[headerIdx+len(header):]
	// Section ends at the next "### " header or end-of-string.
	endIdx := strings.Index(rest, "\n### ")
	var sectionBody, after string
	if endIdx < 0 {
		sectionBody = rest
		after = ""
	} else {
		sectionBody = rest[:endIdx]
		after = rest[endIdx:]
	}
	var b strings.Builder
	b.WriteString(before)
	for _, line := range strings.Split(sectionBody, "\n") {
		if strings.HasPrefix(line, "- ") {
			summary := strings.TrimPrefix(line, "- ")
			if demoted[summary] {
				continue
			}
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	// strings.Split + Join with "\n" + manual final WriteString
	// double-counts the trailing newline; trim one back to keep the
	// section spacing identical to the unfiltered path.
	out := b.String()
	if strings.HasSuffix(out, "\n\n") {
		out = out[:len(out)-1]
	}
	out += after
	return out
}

func (e *explorerEvaluator) buildConcreteValuesSection(repoRoot string, readSet map[string]bool, closure *types.EvidenceClosure) concreteValuesResult {
	if e.searchResult == nil || e.searchResult.Graph == nil {
		return concreteValuesResult{}
	}
	graph := e.searchResult.Graph
	notesJoined := strings.Join(e.investigationNotes, "\n")

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
				// For longer functions, only keep binding/registration/map
				// values and cross-component call targets. Bulk "returns"
				// / "assigns" entries would flood the synthesis prompt,
				// but "calls" carries control-flow signal that the
				// evidence-chain resolver uses for cross-package
				// dispatch chains.
				if !isShort && !strings.Contains(cv.kind, "binds") &&
					cv.kind != "maps" && cv.kind != "calls" {
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
	b.WriteString("These are EXACT values from source code — ground truth, not summaries. " +
		"Rows whose Fact column starts with `calls →` surface cross-component " +
		"dispatch: the method on the left transfers control to the target on the " +
		"right. Follow the arrow with a `read_file` on the target's file to trace " +
		"the next hop in the chain.\n\n")
	b.WriteString("| File:Line | Method | Fact |\n")
	b.WriteString("|-----------|--------|------|\n")
	for _, v := range relevant {
		// T2c: render "calls" with a right-arrow prefix so the LLM
		// can distinguish control-flow rows from value-flow rows at
		// a glance. Example: "calls → SubAgentRuntime.Run" vs.
		// "returns \"explorer\"". All other kinds keep the historical
		// "<kind> <value>" format.
		fact := v.kind + " " + v.value
		if v.kind == "calls" {
			fact = "calls → " + v.value
		}
		fmt.Fprintf(&b, "| %s:%d | `%s()` | %s |\n",
			v.file, v.line, v.method, fact)
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
	// chainAnchors is the CGEC parallel array — for each chain
	// summary appended to chains we append the (v.file, rv.file)
	// pair so the promotion pass can drop chains anchored outside
	// Turn A's ReadSet without re-deriving the source attribution.
	var chainAnchors []chainAnchorInfo
	for _, v := range allRelevantForEvidence {
		// Skip values that don't reference other types.
		// T2b: "calls" joins the allowlist so cross-package dispatch
		// chains like `dispatchStage calls SubAgentRuntime.Run →
		// SubAgentRuntime.Run calls SubExplorer.Run` can form. The
		// call target renders as `ReceiverType.Method` which
		// containsIdentifier tests just like any other type-
		// referencing value (the LastIndex dot-split in
		// scanCallTargetsInLine keeps the receiver identifier
		// prominent so the identifier match fires).
		if v.kind != "returns" && !strings.Contains(v.kind, "binds") &&
			v.kind != "maps" && v.kind != "config" &&
			v.kind != "decorates" && v.kind != "calls" {
			continue
		}
		// Session-8 Fix γ (trace 1776450670620195562): skip long /
		// multi-line string literals. `of.WriteString("<prompt>")`
		// inside BuildAnalysisSkill captures the entire prompt text
		// as v.value; `containsIdentifier` then matches every type
		// name MENTIONED inside the prose and generates a phantom
		// chain per mention. The resulting Primary Evidence section
		// showed 6-8 duplicate "chains" whose body was hundreds of
		// chars of unrelated prompt prose. Concrete-value chains are
		// meant to be code-level facts ("returns X", "binds
		// NewFoo"); a 500-char prose literal is neither.
		if isProseLikeConcreteValue(v.value) {
			continue
		}
		for _, rv := range allRelevantForEvidence {
			if rv.receiver == "" || rv.receiver == v.receiver {
				continue
			}
			if isProseLikeConcreteValue(rv.value) {
				continue
			}
			if containsIdentifier(v.value, rv.receiver) {
				summary := fmt.Sprintf(
					"`%s()` %s %s → `%s()` %s %s",
					v.method, v.kind, v.value,
					rv.method, rv.kind, rv.value)
				chains = append(chains, summary)
				chainAnchors = append(chainAnchors, chainAnchorInfo{
					Summary: summary,
					Files:   dedupeAnchorFiles(v.file, rv.file),
					Origin:  "concrete_values_tracer",
				})
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
	// CGEC C2: subject-aware chain ranking. Reorder `chains` (and
	// the parallel `chainAnchors`) by adjusted score:
	//
	//     adjusted = baseRank * (1 + α * subject.Score(terminal, expectedKind, graph))
	//     α = 2.0
	//
	// `baseRank` is the chain's reverse-insertion-order rank (later
	// chains have larger raw rank), so a chain that the producer
	// emitted later but whose terminal matches the expected subject
	// kind can leapfrog earlier subject-mismatch chains. When
	// e.answerSubject.Kind is SubjectUnknown the score is zero for
	// every chain → adjusted ordering equals insertion order, and
	// the cap below behaves identically to the pre-CGEC behavior.
	chains, chainAnchors = rankChainsBySubject(chains, chainAnchors, e.answerSubject, graph, closure)
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
	//
	// CGEC F1: skip values whose source file is NOT in ReadSet.
	// Each concreteValue has a .file field pointing at where the
	// underlying method body lives; emitting a chain that depends
	// on a method body the LLM never read surfaces an invisible
	// dependency (the "applies to Foo" claim cannot be verified by
	// the LLM without reading v.file). The existing applyChainPromotion
	// handles Resolution Chains but historically did not touch this
	// parallel Type Hierarchy producer — chains here could leak
	// unread anchors into the synthesis prompt. The ScannedSet
	// escape hatch at IsScanned keeps back-compat for tests.
	var skippedHierarchyCount int
	chainSet := make(map[string]bool) // deduplicate chains
	for pass := 0; pass < 3; pass++ {
		added := 0
		for _, rel := range allRelations {
			vals, ok := valuesByReceiver[rel.parentType]
			if !ok {
				continue
			}
			for _, v := range vals {
				if v.file != "" && !readSet[v.file] {
					skippedHierarchyCount++
					continue
				}
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
	if skippedHierarchyCount > 0 {
		logging.Info("[CGEC] F1 hierarchy_promotion: skipped=%d (source files outside ReadSet)", skippedHierarchyCount)
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
	bridgeItems := extractBridgeLiteralChains(graph, repoRoot, allRelevantForEvidence)
	if len(bridgeItems) > 0 {
		logging.Debug("[explorer] bridge literal chains: %d items", len(bridgeItems))
		// CGEC: record per-item anchor file so the promotion helper
		// can demote bridge-literal chains anchored outside ReadSet
		// the same way it handles per-file tracer chains.
		for _, it := range bridgeItems {
			if it.Source == "" {
				continue
			}
			chainAnchors = append(chainAnchors, chainAnchorInfo{
				Summary: it.Summary,
				Files:   []string{it.Source},
				Origin:  "bridge_literal",
			})
		}
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

	return concreteValuesResult{markdown: b.String(), evidence: cvEvidence, chainAnchors: chainAnchors}
}

// rankChainsBySubject is the CGEC C2 chain ranker. Given the raw
// chain list (insertion order) and its parallel chainAnchors slice,
// it returns both reordered so chains whose terminal token matches
// the expected AnswerSubject.Kind win the chainCap slots in the
// markdown table.
//
// Scoring formula:
//
//	subjectMatch = subject.Score(ChainTerminalToken(chain), kind, graph)  // [0, 1]
//	baseRank     = N - i                                                  // 1..N, later → smaller
//	adjusted     = baseRank * (1 + α * subjectMatch)                      // α = 2.0
//
// Chains are sorted by `adjusted` descending; ties broken by
// original insertion order so the function is stable. When
// expected.Kind is SubjectUnknown, subjectMatch is zero for every
// chain and the relative order is preserved.
//
// Side effects:
//   - When closure is non-nil, writes the per-chain score into
//     closure.SetSubjectMatch so downstream consumers (the G5
//     RebindSubject producer in particular) can spot "every chain
//     scored low for the expected subject" without re-computing.
//   - When the expected subject is high-confidence (>= 0.5) AND
//     no chain achieved the rebind floor (0.4), raises a
//     RepairRebindSubject directive on the closure so the next
//     explore round's retry hint surfaces the constraint.
func rankChainsBySubject(chains []string, anchors []chainAnchorInfo, expected types.AnswerSubject, graph *repomap.Graph, closure *types.EvidenceClosure) ([]string, []chainAnchorInfo) {
	if len(chains) <= 1 || expected.Kind == types.SubjectUnknown {
		return chains, anchors
	}
	const (
		alpha         = 2.0
		rebindFloor   = 0.4
		highConfFloor = 0.5
	)
	type ranked struct {
		summary  string
		anchor   chainAnchorInfo
		match    float64
		adjusted float64
		origIdx  int
	}
	rs := make([]ranked, len(chains))
	bestMatch := 0.0
	for i, c := range chains {
		var anchor chainAnchorInfo
		if i < len(anchors) {
			anchor = anchors[i]
		}
		terminal := subject.ChainTerminalToken(c)
		match := subject.Score(terminal, expected.Kind, graph)
		if closure != nil {
			closure.SetSubjectMatch(c, match)
		}
		if match > bestMatch {
			bestMatch = match
		}
		baseRank := float64(len(chains) - i)
		rs[i] = ranked{
			summary:  c,
			anchor:   anchor,
			match:    match,
			adjusted: baseRank * (1.0 + alpha*match),
			origIdx:  i,
		}
	}
	// G5: if no chain scored above the rebind floor AND the
	// analyzer was confident in the expected subject, the chain
	// producer is fundamentally talking past the question. Raise a
	// RepairRebindSubject directive so the next retry's prompt
	// explicitly tells the LLM what kind of token it should be
	// looking for.
	if closure != nil && expected.Confidence >= highConfFloor && bestMatch < rebindFloor {
		closure.AddRepair(types.RepairDirective{
			Kind:      types.RepairRebindSubject,
			Subject:   string(expected.Kind),
			Rationale: fmt.Sprintf("ranked %d candidate chain(s); none scored above %.1f for the expected subject (best=%.2f)", len(chains), rebindFloor, bestMatch),
			Origin:    "chain_ranker",
		})
	}
	// Stable sort by adjusted desc; insertion order on ties.
	sort.SliceStable(rs, func(a, b int) bool {
		if rs[a].adjusted != rs[b].adjusted {
			return rs[a].adjusted > rs[b].adjusted
		}
		return rs[a].origIdx < rs[b].origIdx
	})
	outChains := make([]string, len(rs))
	outAnchors := make([]chainAnchorInfo, len(rs))
	for i, r := range rs {
		outChains[i] = r.summary
		outAnchors[i] = r.anchor
	}
	logging.Debug("[explorer] subject-aware ranking: kind=%s reordered %d chains (top: %q)",
		expected.Kind, len(outChains), firstChainPreview(outChains))
	return outChains, outAnchors
}

// firstChainPreview returns a short prefix of the first chain (or
// empty string), used in debug logs without dumping the full text.
func firstChainPreview(chains []string) string {
	if len(chains) == 0 {
		return ""
	}
	c := chains[0]
	if len(c) > 80 {
		return c[:77] + "..."
	}
	return c
}

// dedupeAnchorFiles returns the unique non-empty entries in the
// supplied file paths, preserving first-seen order. Used by
// chainAnchorInfo construction so a chain that mentions the same
// file twice (when v.file == rv.file) doesn't double-count for the
// chain promotion check.
func dedupeAnchorFiles(files ...string) []string {
	seen := make(map[string]bool, len(files))
	out := make([]string, 0, len(files))
	for _, f := range files {
		f = strings.TrimSpace(f)
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
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
// concreteValue is a single programmatically-extracted fact from a
// source file. Promoted to package scope 2026-04-17 so
// extractBridgeLiteralChains can accept a []concreteValue argument
// for Pass D consumer-gate join. Previously declared inside
// buildConcreteValuesSection.
type concreteValue struct {
	file     string
	receiver string
	method   string // qualified: Receiver.Name or Name
	kind     string // "returns", "binds ONLY", "assigns", etc.
	value    string
	line     int
}

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
// Pass D — Consumer-gate join: for each consumer concrete_value
// (assignment whose Object contains <Field>.Get(<key>) against a
// registry-like identifier), locate a producer binding whose fnQual
// shares a stem with <Field>, and emit a joined chain that names
// the gate, the registry population, and the terminal identity
// literal. This closes the cross-file loop for questions that
// require joining a caller-side gate with a registry population
// chain (e.g. "how many agents can call subagent" = which agents
// pass buildToolSchemas's SubAgents.Get gate, which is satisfied
// only by agents whose Name matches a registered SubAgent name;
// only NewSubExplorer is registered, Name()="explorer" — therefore
// only the explorer agent). Producer="consumer_gate".
//
// Cost is bounded by graph symbol count + body size per matching
// function. On codrax-scale repos this is a few hundred short body
// reads, sub-millisecond total.
func extractBridgeLiteralChains(graph *repomap.Graph, repoRoot string, consumerValues []concreteValue) []types.EvidenceItem {
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

	idByClass := make(map[string][]identity, len(identities))
	for _, id := range identities {
		idByClass[id.class] = append(idByClass[id.class], id)
	}
	var items []types.EvidenceItem
	seen := make(map[string]bool)

	// Pass C — Join on class identifier (existing bridge_literal chains).
	// Requires BOTH bindings AND identities; skips silently if either
	// is empty so Pass D can still fire on bindings alone.
	if len(bindings) > 0 && len(identities) > 0 {
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
	}

	// Pass D — Consumer-gate join. Requires at least one binding;
	// identities are best-effort (the chain tail naming
	// `Target.Identity()` only appears when an identity was found).
	if len(bindings) > 0 && len(consumerValues) > 0 {
		for _, cv := range consumerValues {
			// Only assignment-kind concrete values carry the <Field>.Get(
			// <key>) pattern we care about. Returns/binds/maps do not.
			if cv.kind != "assigns" {
				continue
			}
			consumerField := parseConsumerGateField(cv.value)
			if consumerField == "" {
				continue
			}
			stem := singularize(consumerField)
			// Generic names like `Tool`, `User`, `Role`, `Api` match too
			// many unrelated bindings. Require stem length ≥5 to keep
			// the heuristic tight; the typical question-relevant field
			// (SubAgent, Plugin, Handler, Resource, Listener) clears
			// this bar. Lower thresholds were tried and produced false
			// matches in dev.
			if len(stem) < 5 {
				continue
			}
			lowerStem := strings.ToLower(stem)
			for _, b := range bindings {
				if !strings.Contains(strings.ToLower(b.fnQual), lowerStem) {
					continue
				}
				ids := idByClass[b.targetClass]
				var chainTail string
				if len(ids) > 0 {
					// Pick the first identity — `Name()` is the
					// canonical choice when multiple identity methods
					// coexist; Pass B ordering puts Name first.
					id := ids[0]
					chainTail = fmt.Sprintf(" → `%s.%s()` returns %q",
						b.targetClass, id.method, id.literal)
				}
				summary := fmt.Sprintf(
					"`%s()` gates on %s.Get(...) — registry populated by `%s()` binding New%s%s",
					cv.method, consumerField, b.fnQual, b.targetClass, chainTail)
				if seen[summary] {
					continue
				}
				seen[summary] = true
				items = append(items, types.EvidenceItem{
					ID: types.StableEvidenceID(
						types.EvidenceDataflowPath, summary, "resolution_chain",
						"", "", cv.file, cv.line, cv.line),
					Kind:       types.EvidenceDataflowPath,
					Subject:    summary,
					Predicate:  "resolution_chain",
					Summary:    summary,
					Source:     cv.file,
					LineStart:  cv.line,
					LineEnd:    cv.line,
					Confidence: 0.85,
					Producer:   "consumer_gate",
				})
			}
		}
	}

	return items
}

// parseConsumerGateField extracts the last capitalised identifier
// immediately before `.Get(` in a concrete_value's Object text. For
// `err := b.deps.SubAgents.Get(string(b.name)); err == nil {` this
// returns "SubAgents". Returns empty string when no such pattern
// exists.
//
// The pattern is deliberately narrow (requires capital first letter)
// so common Go code-noise like `list.get(i)` on a local slice is
// excluded. Registry / store / repository fields are overwhelmingly
// CamelCase in Go and TypeScript.
func parseConsumerGateField(value string) string {
	idx := strings.Index(value, ".Get(")
	if idx < 0 {
		return ""
	}
	// Walk backwards from idx to find the start of the identifier.
	end := idx
	start := end
	for start > 0 {
		c := value[start-1]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '_' {
			start--
			continue
		}
		break
	}
	if start == end {
		return ""
	}
	ident := value[start:end]
	// Must start with a capital letter — this is the registry-field
	// convention. Skip locals/args that happen to have a Get method.
	if ident[0] < 'A' || ident[0] > 'Z' {
		return ""
	}
	return ident
}

// singularize strips a trailing 's' from a CamelCase plural to match
// a singular stem. Handles the common English plural form used by
// Go field names (SubAgents→SubAgent, Handlers→Handler). Does not
// attempt irregular plurals — those are rare in Go identifiers.
func singularize(s string) string {
	if strings.HasSuffix(s, "s") && len(s) > 1 {
		return s[:len(s)-1]
	}
	return s
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

	// T2a: Pattern — cross-component / cross-package method calls.
	//
	// Detects method-call expressions that transfer control to a
	// named identifiable target: `o.subRuntime.Run(proposal)`,
	// `b.deps.Tools.Execute(ctx, name, params)`, `Reducer.Reduce(...)`.
	// Unlike the constructor-passing detector above (which emits
	// "binds" when a factory value is passed as an argument), this
	// detector emits "calls" whenever control transfers to an
	// exported method on a non-stdlib receiver.
	//
	// Historical gap: the evidence-chain resolver couldn't trace
	// cross-package dispatch chains like
	//   explorer LLM → propose_sub_agents tool
	//     → orchestrator.extractSubAgentProposal
	//     → SubAgentRuntime.Run
	//     → SubExplorer.Run
	// because none of these hops are return/bind/map patterns. Every
	// hop is a method call on a field. This detector closes that gap
	// by surfacing the call target so the synthesis prompt can render
	// "calls → SubAgentRuntime.Run" rows in the Concrete Values table.
	//
	// Filtering rules (noise suppression):
	//   - Receiver chain must have at least one dot (method call, not
	//     bare function). Bare function calls match the existing
	//     constructor-passing detector when they matter.
	//   - Method name must start uppercase (exported). Unexported
	//     calls are implementation detail; exported calls cross
	//     abstraction boundaries.
	//   - Receiver's head segment must not be a known stdlib /
	//     utility package (fmt, strings, log, logging, errors, …).
	//   - Per-snippet cap: at most 6 calls surface so the concrete-
	//     values table doesn't flood on long function bodies.
	if calls := extractCallTargets(lines, 6); len(calls) > 0 {
		for _, c := range calls {
			results = append(results, concreteValueEntry{
				kind:  "calls",
				value: c,
			})
		}
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

// callTargetNoiseReceivers is the set of head-receiver identifiers
// that should NEVER produce a "calls" entry. These are Go stdlib
// packages and codrax-internal utility packages whose methods are
// implementation plumbing, not cross-component dispatch.
//
// Missing entries just means the corresponding receiver will produce
// a "calls" entry — a false positive — so this list prefers over-
// inclusion. The method-name blocklist below catches leftover noise.
var callTargetNoiseReceivers = map[string]bool{
	// Go stdlib — called everywhere, never cross-component signal.
	"fmt": true, "strings": true, "strconv": true, "errors": true,
	"sort": true, "bytes": true, "bufio": true, "io": true,
	"os": true, "filepath": true, "path": true, "time": true,
	"sync": true, "context": true, "regexp": true, "unicode": true,
	"math": true, "rand": true, "json": true, "xml": true,
	"url": true, "http": true, "net": true, "net/http": true,
	"reflect": true, "runtime": true,
	// Codrax internal utility packages.
	"log": true, "logging": true,
	// Note: single-letter locals like `b` / `s` / `err` / `w` are NOT
	// in this list because their method call chains carry real
	// signal when the chain has multiple dots (e.g. `b.deps.Tools.Execute`
	// is a legitimate cross-package dispatch that the 2026-04-18
	// debug revealed was being falsely filtered). The
	// callTargetNoiseMethods list below (String/Error/Write*) catches
	// the common noise shapes on these receivers.
}

// callTargetNoiseMethods is the set of method names that should
// NEVER produce a "calls" entry regardless of receiver. These are
// value-extraction, formatting, and comparison helpers.
var callTargetNoiseMethods = map[string]bool{
	// stringers / value extractors
	"String": true, "Error": true, "Bytes": true, "Len": true, "Cap": true,
	// strings / bytes package methods
	"Contains": true, "HasPrefix": true, "HasSuffix": true, "Index": true,
	"LastIndex": true, "TrimSpace": true, "TrimPrefix": true, "TrimSuffix": true,
	"ToLower": true, "ToUpper": true, "EqualFold": true, "Split": true,
	"Join": true, "Replace": true, "ReplaceAll": true, "Repeat": true,
	"Count": true, "Fields": true, "Trim": true,
	// fmt package methods
	"Sprintf": true, "Printf": true, "Println": true, "Fprintf": true,
	"Sprint": true, "Fprintln": true, "Errorf": true,
	// json / encoding package methods
	"Marshal": true, "Unmarshal": true, "Encode": true, "Decode": true,
	// time / misc
	"Now": true, "Sleep": true, "Since": true,
	// sync
	"Lock": true, "Unlock": true, "RLock": true, "RUnlock": true, "Wait": true,
	// log / logging (codrax's `logging` package has lowercase methods,
	// but external loggers use these capitalized forms)
	"Debug": true, "Info": true, "Warning": true, "Warn": true,
	"Log": true, "Printf2": true,
	// builder-style helpers
	"WriteString": true, "WriteByte": true, "Write": true,
	// slices / maps
	"Append": true,
}

// extractCallTargets scans `lines` for method-call expressions that
// transfer control to an identifiable cross-component target, and
// returns a deduplicated list of targets in the form "Receiver.Method".
// The result is capped at `maxCalls` entries so long function bodies
// don't flood the concrete-values table.
//
// Recognition (conservative on purpose):
//   - Line contains `<receiver>.<Method>(` where Method starts with
//     an uppercase letter.
//   - Receiver chain has >=1 dot. Bare function calls (`Foo(...)`)
//     are handled by the constructor-passing detector when they are
//     interesting; a bare call here would flood the output.
//   - Head receiver identifier is NOT in callTargetNoiseReceivers.
//   - Method name is NOT in callTargetNoiseMethods.
//   - Dedup: the same "Receiver.Method" target is emitted at most
//     once per snippet.
//
// The returned string format is `<tail-receiver>.<Method>` where
// `<tail-receiver>` is the last identifier of the receiver chain
// (e.g. `o.subRuntime.Run` → `subRuntime.Run`, `Reducer.Reduce` →
// `Reducer.Reduce`). This is the shape the synthesis prompt renders
// in the Concrete Values table's "Fact" column.
func extractCallTargets(lines []string, maxCalls int) []string {
	if maxCalls <= 0 {
		maxCalls = 6
	}
	seen := make(map[string]bool)
	out := make([]string, 0, maxCalls)
	for _, line := range lines {
		if len(out) >= maxCalls {
			break
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") ||
			strings.HasPrefix(trimmed, "#") {
			continue
		}
		for _, target := range scanCallTargetsInLine(trimmed) {
			if seen[target] {
				continue
			}
			seen[target] = true
			out = append(out, target)
			if len(out) >= maxCalls {
				break
			}
		}
	}
	return out
}

// scanCallTargetsInLine finds all `<chain>.<Method>(` call-open
// positions on a single line and returns their normalized form.
// Multiple calls on one line (chained calls like
// `x.Foo().Bar().Baz()`) are all captured so the detector doesn't
// miss fluent-API dispatches.
//
// Implementation detail: scan for `(` then walk backwards to read
// the receiver chain + method name, using identifier-character
// classification. A proper lexer would be more robust but this is
// a pattern-sniffer over short snippets, not a compiler — the
// filtering lists above are the real precision gate.
func scanCallTargetsInLine(line string) []string {
	var targets []string
	for i := 0; i < len(line); i++ {
		if line[i] != '(' {
			continue
		}
		// Walk back over the identifier chain `<head>[.<seg>]*`.
		end := i
		start := i
		for start > 0 {
			c := line[start-1]
			if isIdentChar(c) || c == '.' {
				start--
				continue
			}
			break
		}
		chain := line[start:end]
		if chain == "" || !strings.Contains(chain, ".") {
			continue
		}
		// Split into receiver + method; method is the LAST segment.
		dotIdx := strings.LastIndex(chain, ".")
		receiver := chain[:dotIdx]
		method := chain[dotIdx+1:]
		if method == "" || !isExportedIdent(method) {
			continue
		}
		if callTargetNoiseMethods[method] {
			continue
		}
		// Head receiver identifier for stdlib exclusion.
		head := receiver
		if dot := strings.Index(head, "."); dot >= 0 {
			head = head[:dot]
		}
		if callTargetNoiseReceivers[head] {
			continue
		}
		// Normalize receiver to the LAST identifier in the chain
		// (closest to the method). `o.subRuntime.Run(...)` → receiver
		// `subRuntime`; `b.deps.Tools.Execute(...)` → `Tools`. This
		// keeps the rendered target at "Type.Method" granularity
		// without losing the call semantics.
		tailRecv := receiver
		if ldot := strings.LastIndex(receiver, "."); ldot >= 0 {
			tailRecv = receiver[ldot+1:]
		}
		if tailRecv == "" {
			continue
		}
		targets = append(targets, tailRecv+"."+method)
	}
	return targets
}

// (isIdentChar is defined below at its pre-existing site near the
// cross-validation helpers; reused by scanCallTargetsInLine to walk
// backwards over the receiver chain.)

// isExportedIdent reports whether s is a non-empty identifier
// starting with an uppercase letter — the Go convention for
// exported names. Used to filter the call-target detector to
// cross-abstraction-boundary calls only.
func isExportedIdent(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	return c >= 'A' && c <= 'Z'
}

// phase1MedianScore returns the median `Score` across the given
// Phase1RankedFile slice, or 1.0 when the slice is empty. Used by
// the T3a merger that folds analyzer-supplied RequiredFiles into
// the explorer's own keyword-search ranking: analyzer files that
// don't already appear in the ranking get this median score so
// they rank mid-pack, prioritized over low-match noise but not
// displacing high-match grep-IDF winners.
//
// Median rather than mean because the grep IDF distribution is
// heavy-tailed: a handful of high-IDF files (rare keywords that
// match exactly 1-2 files) pull the mean far above the rank-10
// value, which would force analyzer-supplied files unfairly high.
// The median tracks the central mass, producing a more stable
// injection point.
func phase1MedianScore(ranked []types.Phase1RankedFile) float64 {
	if len(ranked) == 0 {
		return 1.0
	}
	scores := make([]float64, len(ranked))
	for i, r := range ranked {
		scores[i] = r.Score
	}
	// Partial sort via sort.Float64s. Median indexing handles both
	// even and odd lengths without a branch — the (n-1)/2 position
	// is always a legitimate middle choice.
	sortFloats(scores)
	return scores[(len(scores)-1)/2]
}

// sortFloats is an inline helper to avoid pulling in the sort
// package just for a single call. Uses insertion sort because
// the ranked list's len(sr.Files) is capped at 30 (T1b) and the
// asymptotic constants favor simple O(n²) for small n.
func sortFloats(xs []float64) {
	for i := 1; i < len(xs); i++ {
		x := xs[i]
		j := i
		for j > 0 && xs[j-1] > x {
			xs[j] = xs[j-1]
			j--
		}
		xs[j] = x
	}
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
		case "exec_command":
			// Best-effort file path extraction from exec_command
			// output. Handles find/ls/tree output where each line
			// is a file path. Non-path lines (counts, errors,
			// binary output) are filtered by isValidFilePath.
			for _, line := range strings.Split(r.Summary, "\n") {
				path := strings.TrimSpace(line)
				if path == "" || path[0] == '[' {
					continue
				}
				path = strings.TrimPrefix(path, "./")
				if !isValidFilePath(path) || isNoisePath(path) {
					continue
				}
				if !discoveredSet[path] {
					discoveredSet[path] = true
					discovered = append(discovered, path)
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

	scanQuestionTokens(question, func(tok string, src tokenSource) {
		switch src {
		case tokenBacktick:
			// Backtick-delimited tokens are explicit identifiers —
			// admit subject only to the trim/length/dedup gate.
			add(tok)
		case tokenRun:
			// Unquoted run — shape-gate it. Admit when CamelCase
			// (2+ upper-initial segments AND len ≥ 6) OR dotted
			// (contains '.' AND len ≥ 5). This is the split against
			// extractRankingEntitiesWithGraph, which admits all runs
			// subject to its own entityQualifies filter.
			segments := 0
			for j := 0; j < len(tok); j++ {
				if tok[j] >= 'A' && tok[j] <= 'Z' {
					if j == 0 || (tok[j-1] >= 'a' && tok[j-1] <= 'z') {
						segments++
					}
				}
			}
			if segments >= 2 && len(tok) >= 6 {
				add(tok)
			}
			if strings.Contains(tok, ".") && len(tok) >= 5 {
				add(tok)
			}
		}
	})
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
// crossFileSymbolGap is one hit returned by detectCrossFileSymbolGaps:
// an exported symbol `Symbol` referenced in the LLM's investigation
// notes, defined in `File`, which the LLM has not yet read.
type crossFileSymbolGap struct {
	File   string
	Symbol string
}

// detectCrossFileSymbolGaps scans investigation notes for exported
// symbol references and returns up to `max` files that define
// referenced symbols but haven't been read yet (T3b).
//
// Scoring:
//   - A symbol name must be ≥ 6 characters to count. Shorter names
//     (`Run`, `Get`, `Set`, `New`, `Dir`) are too generic and cause
//     false positives; the existing post-hoc synthesis scanner at
//     explorer.go:2986 uses the same threshold.
//   - Only unread files (path not in readSet) produce a gap entry.
//     Symbols defined in already-read files are no-ops — reading
//     them again buys nothing.
//   - When a symbol has multiple definitions (interface + impls,
//     overloads), pick the first one whose defining file is NOT
//     in readSet. This biases toward "find me an unread impl" when
//     the LLM has seen the interface declaration but not the
//     implementation, which is exactly the cross-package dispatch
//     chain situation.
//   - The returned slice is capped at `max` (caller passes 3 to
//     keep the mid-loop hint compact).
//   - Results are sorted by symbol-name length descending — longer
//     identifiers tend to be higher-specificity (e.g.
//     `SubAgentRuntime` outranks `Runtime`) so the most
//     discriminating suggestions come first.
//
// Returns nil when graph is nil or no gaps are found.
func detectCrossFileSymbolGaps(notes []string, graph *repomap.Graph, readSet map[string]bool, max int) []crossFileSymbolGap {
	if graph == nil || len(notes) == 0 || max <= 0 {
		return nil
	}
	joined := strings.Join(notes, "\n")
	if joined == "" {
		return nil
	}
	// Dedup by (file, symbol) — the same symbol may have multiple
	// defining sites; we only emit the first unread one.
	seen := make(map[string]bool)
	var gaps []crossFileSymbolGap
	for symName, defs := range graph.SymbolDefs {
		if len(symName) < 6 {
			continue
		}
		if !strings.Contains(joined, symName) {
			continue
		}
		for _, def := range defs {
			if def == nil {
				continue
			}
			if readSet[def.File] {
				// Canonical match — the file IS read.
				continue
			}
			// Also check common canonicalisation variants (readSet
			// keys sometimes carry a leading "./"). This mirrors the
			// logic at explorer.go:3415 where readSet lookups
			// normalize "./path" → "path".
			if readSet["./"+def.File] || readSet[strings.TrimPrefix(def.File, "./")] {
				continue
			}
			key := def.File + "\x00" + symName
			if seen[key] {
				continue
			}
			seen[key] = true
			gaps = append(gaps, crossFileSymbolGap{File: def.File, Symbol: symName})
			break // one entry per symbol — no need to enumerate further defs
		}
	}
	// Sort by symbol-name length descending (longer = more specific).
	// Insertion sort is cheap for the small slice sizes we expect.
	for i := 1; i < len(gaps); i++ {
		cur := gaps[i]
		j := i
		for j > 0 && len(gaps[j-1].Symbol) < len(cur.Symbol) {
			gaps[j] = gaps[j-1]
			j--
		}
		gaps[j] = cur
	}
	if len(gaps) > max {
		gaps = gaps[:max]
	}
	return gaps
}

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

	// Build per-file read intervals: keep every individual read_file
	// range instead of collapsing to a single min/max. The old
	// aggregation caused false positives: a targeted grep-directed
	// read (e.g. lines 440-470) would be merged with an earlier
	// import scan (lines 1-50) into {minStart:1, maxEnd:470}, making
	// every large function starting before line 470 look "partially
	// read" even though the LLM never tried to read it.
	type readInterval struct {
		start, end int
	}
	fileReads := make(map[string][]readInterval)

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
		fileReads[path] = append(fileReads[path], readInterval{startLine, endLine})
	}

	if len(fileReads) == 0 {
		return nil
	}

	var hints []partialReadHint
	for path, intervals := range fileReads {
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

			// A function counts as "partially read" only when the LLM
			// started reading FROM the function's beginning — i.e. at
			// least one read_file interval started within the first 20%
			// of the function body (or before it). A read that starts
			// deep inside (grep-directed targeted read) is NOT a partial
			// function read — the LLM was checking a specific spot, not
			// trying to read the whole function.
			bodyLines := sym.EndLine - sym.Line + 1
			entryZone := sym.Line + bodyLines/5 // first 20% of the function
			startedFromTop := false
			maxEnd := 0
			for _, iv := range intervals {
				if iv.start <= entryZone && iv.end >= sym.Line {
					// This read entered the function near its start.
					startedFromTop = true
				}
				// Track the furthest line read within this function.
				if iv.end > maxEnd && iv.start <= sym.EndLine && iv.end >= sym.Line {
					if iv.end > sym.EndLine {
						maxEnd = sym.EndLine
					} else {
						maxEnd = iv.end
					}
				}
			}
			if !startedFromTop || maxEnd == 0 {
				continue
			}
			// Fully covered?
			if maxEnd >= sym.EndLine {
				continue
			}

			coveredLines := maxEnd - sym.Line + 1
			if coveredLines < 0 {
				coveredLines = 0
			}
			cov := float64(coveredLines) / float64(bodyLines)

			// Only report if coverage < 80% AND missing > 20 lines.
			unreadLines := sym.EndLine - maxEnd
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
					readEnd:    maxEnd,
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

// proseConcreteValueMaxLen is the char ceiling for a "concrete value"
// string. Anything longer is treated as prose (doc comment, prompt
// body, markdown block) rather than a code-level fact. 120 fits
// typical return literals ("foo", 42, nil, NewFoo(deps)) and
// single-line config values while cleanly excluding multi-paragraph
// prompt strings that can run hundreds of characters.
const proseConcreteValueMaxLen = 120

// isProseLikeConcreteValue reports whether a concrete_value's value
// field is likely prose rather than a code fact. Used by the chain-
// construction loop to skip `of.WriteString("<prompt>")` style
// bindings that would otherwise generate phantom chains — the prose
// text in v.value mentions type names that containsIdentifier
// matches, fabricating cross-class edges that don't exist in code.
//
// Two heuristics:
//
//  1. Length: > proseConcreteValueMaxLen chars is almost always a
//     multi-line prose literal. Real return values / bindings fit
//     under this threshold.
//  2. Newlines: any \n inside v.value means the source captured a
//     multi-line string literal — prose, not a code expression.
//
// Both conditions are OR-ed; either one is enough to reject.
func isProseLikeConcreteValue(v string) bool {
	if len(v) > proseConcreteValueMaxLen {
		return true
	}
	if strings.ContainsRune(v, '\n') {
		return true
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
	return NewBaseAgent(types.AgentExplorer, deps, &explorerEvaluator{
		heuristics: deps.ExploreHeuristics,
		tools:      deps.Tools,
	})
}

// countGhostAnchorsForFile walks the ledger and counts how many
// ViolGhostAnchor entries reference the given repo-relative file.
// Session 11 R5 uses this to decide when to promote the file to
// ScannedSet. The ledger scan is O(N) on total violations which
// is fine in practice — the ledger is bounded by per-dispatch
// enforcer fire counts, not by LLM output size.
func countGhostAnchorsForFile(closure *types.EvidenceClosure, file string) int {
	if closure == nil || file == "" {
		return 0
	}
	n := 0
	for _, v := range closure.ViolationsByKind(types.ViolGhostAnchor) {
		for _, ref := range v.EvidenceRefs {
			if ref == file {
				n++
				break
			}
		}
	}
	return n
}

// fileExistsInRepo returns true when the repo-relative path
// resolves to an existing regular file under repoRoot. The check
// is intentionally conservative (reject directories, symlinks to
// directories, non-existent paths) so R5 never promotes something
// bogus into ScannedSet.
func fileExistsInRepo(repoRoot, rel string) bool {
	if repoRoot == "" || rel == "" {
		return false
	}
	full := filepath.Join(repoRoot, rel)
	info, err := os.Stat(full)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}
