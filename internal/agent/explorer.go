package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/analysis/dataflow"
	"github.com/hanchaoqun/codrax/internal/analysis/declarative"
	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
	"github.com/hanchaoqun/codrax/internal/analysis/subject"
	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
	"gopkg.in/yaml.v3"
)

const (
	explorerParseSlowAfter = 5 * time.Second
	explorerParseSlowEvery = 10 * time.Second
)

type explorerSearchCache struct {
	mu      sync.RWMutex
	entries map[string]*keywordSearchResult
}

func (c *explorerSearchCache) Get(fp string) (*keywordSearchResult, bool) {
	if c == nil || fp == "" {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.entries == nil {
		return nil, false
	}
	sr, ok := c.entries[fp]
	return sr, ok && sr != nil
}

func (c *explorerSearchCache) Put(fp string, sr *keywordSearchResult) {
	if c == nil || fp == "" || sr == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]*keywordSearchResult)
	}
	c.entries[fp] = sr
}

type explorerEvaluator struct {
	heuristics                types.ExploreHeuristics
	tools                     *tool.Registry
	sharedSearchCache         *explorerSearchCache
	phase                     int                  // 0 = breadth scan, 1 = depth read
	broadenAttempts           int                  // times we pushed for broader grep in Phase 0
	preScannedFiles           []string             // top files from keyword search, for coverage tracking
	allScoredFiles            []string             // ALL files from keyword search (not just top 8), for supplementary evidence
	fileSymbols               map[string][]string  // path → symbol summaries from repo_map
	searchResult              *keywordSearchResult // full search result for cross-reference lookups
	searchFingerprint         string               // T1.2: fingerprint of keyword_search inputs; reuses searchResult across explorer redispatches within one Run when inputs are unchanged
	multiGraphHandle          any                  // P4-cross-sub-repo (Sc 1, 2026-05-08): cached *multigraph.MultiGraph for fan-out at midLoop hooks where ctx is unavailable
	pendingSubRepos           []string             // BusContext.PendingSubRepos cached at dispatch entry so chain_promotion (free function, no ctx) can refuse PendingRead injection on inactive sub-repo paths — matches MultiRepoActiveSetGater rule
	analyzerKeywords          []string             // analyzer-provided keywords cached from BuildInitialInstruction for exact-resolution scope hints
	exactAnchorFiles          []string             // exact-entity anchor files from keyword search, in rank order
	declarativeAnchorFiles    []string             // declarative registry/defaults/routes anchors for enumeration questions
	declarativeCandidateFiles []string             // analyzer-ranked structural candidates when no canonical declarative anchors were derived automatically
	// primaryEntitiesRegistrationShape (2026-05-10 L1) — cached
	// result of primaryEntitiesLookLikeRegistration computed once
	// during BuildInitialInstruction. Gates declarativeFocusRelevant's
	// "enumeration && isEnumeration" branch so function-body
	// enumerations (e.g. "list 9 internal checks of gate.Run") don't
	// hijack the registry-shape declarative path.
	primaryEntitiesRegistrationShape bool
	// requiredFileHints (2026-05-10 L3) — analyzer-emitted per-file
	// recommendations with confidence + rationale. Cached on dispatch
	// entry from ctx.AnalysisIR so the threshold-band consumers
	// (effectivePrimaryFiles for ≥0.8 hints, preReadRequiredFiles for
	// ≥0.5 hints) can read without re-traversing the IR.
	requiredFileHints []types.RequiredFileHint
	// irrelevantFilesSet (2026-05-10 L4) — analyzer-declared
	// irrelevant files, canonicalised + indexed for O(1) lookup.
	// Honored as a hard exclusion across pre-read pools, mid-loop
	// "Read these next:" hints, and primary-file selection.
	irrelevantFilesSet        map[string]bool
	investigationNotes        []string        // assistant analysis messages from ReAct loop
	userQuestion              string          // original user question, for focus alignment
	repoRoot                  string          // repository root path, cached from BuildInitialInstruction
	preScannedPushCount       int             // times we pushed for unread pre-scanned files without progress
	lastPreScannedUnreadCount int             // count of unread pre-scanned files at last push
	grepRedirectedFiles       map[string]bool // files that already received a large-file grep redirect
	isEnumerationQuery        bool            // true if user question asks to list/enumerate all items
	// isOrientationQuery mirrors types.IsProjectOrientationQuestion
	// (intent=explain + simple complexity + no entities + clean
	// predicates). Cached at dispatch entry so observeMidLoop can
	// emit a "you have enough; finalize" nudge without re-running
	// the predicate every iteration. Same semantic as the
	// multi-path symbol-anchored skip (applyMultiPathAnchorChecks
	// → multipath.EvaluateAnchor) — both stages must agree on what
	// an orientation question is.
	isOrientationQuery         bool
	phase0ExtraRound           bool // whether we already gave one extra Phase 0 round for quality gate
	hasPrescanRepoMap          bool // keywordSearch (run at BuildInitialInstruction) produced a ranked file list via repo_map; the Phase 0 quality gate treats this as satisfying the structural-discovery half of its requirement, so the LLM isn't penalized for not re-running repo_map at iter=0
	structuredEvidence         []types.EvidenceItem
	flowFindings               []types.FlowFindingDigest
	ermRequirements            []EvidenceRequirement // evidence requirement model
	cachedConcreteValues       *concreteValuesResult // T1.1: built once per Execute, reused by gate + synthesis
	midLoopLastResultsLen      int                   // #34: allResults length at prev observeMidLoop call (used to infer current batch size)
	midLoopSerialStreak        int                   // #34: consecutive iters observed as single-call rounds
	midLoopParallelInjected    bool                  // #34: parallel-batching hint already pushed this dispatch
	midLoopSymbolRefInjected   bool                  // T3b: cross-file-symbol-reference hint already pushed this dispatch
	midLoopPostPrimaryInjected bool                  // one-shot: immediate "keep using tools after the first anchor read" hint already pushed this dispatch
	// midLoopBudgetExhaustedSent (2026-05-10 Fix B) tracks the
	// per-tool one-shot budget-exhausted nudge. The 2026-05-10 sweep
	// digest forensic on s5b iter=2 exposed the waste: a 6-call
	// parallel batch of read_file all hard-rejected with "explore
	// budget exhausted for tool read_file" — 6 LLM-emitted tool
	// calls, 0 bytes of evidence, the LLM had to plan another iter
	// before realising the tool was hard-banned. The nudge surfaces
	// the budget status as a structural directive so subsequent
	// iters route to other tools or to emit. Per-tool keying lets
	// independent budgets (read_file, grep, repo_map, list_files)
	// each fire their own one-shot.
	midLoopBudgetExhaustedSent      map[string]bool
	midLoopEvidenceRepairSent       bool // one-shot: recovered/ungrounded emit_evidence repair hint already pushed this dispatch
	midLoopEvidenceRepairResultsLen int  // allResults length when the current emit_evidence repair hint fired
	midLoopSurfaceTermReviewSent    bool // one-shot: model-authored surface_terms review hint already pushed this dispatch
	midLoopClosureRepairSent        bool // one-shot: structured closure repair from a downgraded completion already pushed this dispatch
	midLoopClosureRepairResultsLen  int  // allResults length when the current closure repair hint fired
	midLoopIntentWindowSent         bool // session-22: structural-intent-vs-narrow-window hint already pushed this dispatch
	midLoopRankerCoverageSent       bool // session-22: ranker-coverage-too-low hint already pushed this dispatch
	midLoopAbsentRedirectSent       bool // session-22: emit_evidence kind=absent deprecation redirect already pushed this dispatch
	midLoopExternalArtifactSent     bool // one-shot: external-source runtime artifact redirected this dispatch
	midLoopExactAbsenceContextSent  bool // one-shot: exact absence still needs one grounded same-family production anchor before closure
	midLoopExactAbsenceSent         bool // one-shot: exact-resolution absence already looks closure-ready this dispatch
	midLoopSchemaLevelHintSent      bool // one-shot: schema-level evidence nudge already pushed this Run (config-trace + exact-absent only)
	midLoopAuthoritativeTier1Sent   bool // one-shot: authoritative log path is semantically enough but would fail Tier-1 floor before completion
	midLoopEnumInjected             bool // session-22: enumeration-coverage hint already pushed this dispatch (was missing → 68 fires / run observed on goroutine_dump)
	// midLoopOrientationFinalizeSent latches the once-per-dispatch
	// orientation finalize nudge. Without this latch the nudge would
	// re-render every iteration after the threshold fires, drowning
	// the LLM in repeated "you have enough" reminders.
	midLoopOrientationFinalizeSent  bool
	midLoopNoEmitPushSent           bool // one-shot: current evidence-materialization backlog window already received its read-without-emit nudge
	midLoopNoEmitEscalated          bool // one-shot: stronger "emit evidence now" escalation after the current backlog window's nudge was ignored
	midLoopExecRedirectSent         bool // one-shot: redirected shell-style browsing back to built-in grep/read_file before recording the current backlog window
	midLoopExplanationAnchorSent    bool // one-shot: multi-topic explanation still lacks one grounded anchor per sub-topic
	midLoopCompletionReadySent      bool // one-shot: generic "you already have enough grounded evidence; close now" hint already pushed this dispatch
	midLoopCompletionReadyEscalated bool // one-shot: stronger close-now escalation after the completion-ready hint was ignored
	midLoopCompletionReadyIter      int  // iteration where completion-ready first fired
	midLoopNoEmitPushIter           int  // iteration where the current backlog window's read-without-emit nudge fired
	midLoopNoEmitPushResultsLen     int  // allResults length when the current backlog window's read-without-emit nudge fired
	midLoopEmitBacklogBaseLen       int  // allResults length immediately after the last successful emit_evidence that closed the prior backlog window
	primaryReadSeen                 bool // df3-drift: whether any primary-entity file has entered readSet this dispatch
	primaryReadIter                 int  // df3-drift: iter at which a primary-entity file first entered readSet
	notesLenAtPrimaryRead           int  // df3-drift: snapshot of len(investigationNotes) at primaryReadIter
	investigationComplete           bool // set when emit_investigation_complete tool was observed in MidLoop
	mergedEmittedEvidenceLen        int  // number of Mutable.EmittedEvidence rows already folded into structuredEvidence this dispatch

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

	// requiredFiles is a cached copy of EvidencePlan.RequiredFiles
	// from the analyzer IR, set at BuildInitialInstruction time.
	// Used by observeSoftStop's RequiredFiles coverage gate (T3a
	// follow-up) to push the LLM toward reading analyzer-identified
	// files before stopping.
	requiredFiles []string

	// logTriage is a cached pointer to the log-triage bundle from
	// BusContext.Mutable.LogTriage(), captured at BuildInitialInstruction
	// time. Nil when no log was attached or the pre-stage degraded.
	// Session-22 fix F2.1: Check 6 (ranker-coverage) reads
	// bundle.Meta.Signals + bundle.ResolvedFiles to skip the nudge when
	// the attached failure trace has resolved frames AND the LLM has
	// already covered every resolved frame's file — in that state the
	// ranker's remaining top-K are noise siblings sharing method names.
	logTriage *types.LogBundle

	// perfTrace is a cached pointer to the perf-triage bundle from
	// BusContext.Mutable.PerfTrace(), captured at BuildInitialInstruction
	// time. Nil when no HiTrace/atrace/systrace/perfetto payload was
	// attached or perf_triage degraded. Treated as a peer to logTriage
	// by the authoritative-frame plumbing: perf bundle stalls with
	// File+Line are equally authoritative locators, and the
	// backbone-first emit principle ("record one anchor on the failure
	// site before expanding context") applies to jank / main-thread
	// stall / cold-start-slow questions just like it applies to
	// panic / crash. Decoupling the gate from the panic/crash signal
	// whitelist is what makes that work.
	perfTrace *types.PerfBundle

	// mutable caches the current dispatch's MutableState so mid-loop
	// readiness checks can consume the same compiled AnswerSurfacePlan
	// authority as pre-complete / extractor / finalizer instead of
	// re-deriving explanation-anchor policy from local evidence slices.
	mutable *types.MutableState

	// exactResolution caches the analyzer's exact-target contract and
	// the subset of still-pending targets so mid-loop closure nudges
	// can recognize "exact absence already proven; stop expanding
	// nearby context" without re-plumbing AgentContext into Observe.
	exactResolution     *types.ExactResolutionContract
	exactPendingTargets []string
	exactContextFiles   []string

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

	// scenario is the analyzer-classified request scenario cached from
	// BuildInitialInstruction so exact-resolution closure checks can use
	// the same scenario-aware contract as finalizer / validator stages.
	scenario types.Scenario

	// analysisIR caches the analyzer output so mid-loop readiness checks
	// can reuse the same sub-topic / shape contract without re-deriving
	// it from prose.
	analysisIR *types.AnalysisIR

	// kindConfidence caches RequestModel.KindConfidence at
	// BuildInitialInstruction. Schema-v4 downstream guard: gates
	// aggressive narrowing such as tightenDeclarativeFrontier so a
	// low-confidence question_kind cannot over-narrow the read set
	// to wrong files. Zero (LLM declined to rate) is treated as low
	// confidence — the guard refuses to narrow.
	kindConfidence float64

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
		e.searchFingerprint = ""
		e.multiGraphHandle = nil
		e.pendingSubRepos = nil
		e.analyzerKeywords = nil
		e.ermRequirements = nil
		e.fileSymbols = nil
		e.primaryEntitiesRegistrationShape = false
		e.requiredFileHints = nil
		e.irrelevantFilesSet = nil
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
		e.midLoopPostPrimaryInjected = false
		e.midLoopBudgetExhaustedSent = nil
		e.midLoopEvidenceRepairSent = false
		e.midLoopEvidenceRepairResultsLen = 0
		e.midLoopSurfaceTermReviewSent = false
		e.midLoopClosureRepairSent = false
		e.midLoopClosureRepairResultsLen = 0
		e.midLoopIntentWindowSent = false
		e.midLoopRankerCoverageSent = false
		e.midLoopAbsentRedirectSent = false
		e.midLoopExternalArtifactSent = false
		e.midLoopExactAbsenceContextSent = false
		e.midLoopExactAbsenceSent = false
		e.midLoopSchemaLevelHintSent = false
		e.midLoopAuthoritativeTier1Sent = false
		e.midLoopEnumInjected = false
		e.midLoopOrientationFinalizeSent = false
		e.midLoopNoEmitPushSent = false
		e.midLoopNoEmitEscalated = false
		e.midLoopExecRedirectSent = false
		e.midLoopExplanationAnchorSent = false
		e.midLoopCompletionReadySent = false
		e.midLoopCompletionReadyEscalated = false
		e.midLoopCompletionReadyIter = 0
		e.midLoopNoEmitPushIter = 0
		e.midLoopNoEmitPushResultsLen = 0
		e.midLoopEmitBacklogBaseLen = 0
		e.primaryReadSeen = false
		e.primaryReadIter = 0
		e.notesLenAtPrimaryRead = 0
		e.complexity = ""
		e.scenario = ""
		e.answerSubject = types.AnswerSubject{}
		e.predicateAxis = types.AxisUnknown
		e.kindConfidence = 0
		e.requiredFiles = nil
		e.exactAnchorFiles = nil
		e.declarativeAnchorFiles = nil
		e.declarativeCandidateFiles = nil
		e.exactResolution = nil
		e.exactPendingTargets = nil
		e.exactContextFiles = nil
		e.analysisIR = nil
		e.logTriage = nil
		e.perfTrace = nil
		e.mutable = nil
		e.investigationComplete = false
		e.mergedEmittedEvidenceLen = 0
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
	if ctx != nil && ctx.AnalysisIR != nil {
		e.scenario = ctx.AnalysisIR.RequestModel.Scenario
	} else {
		e.scenario = ""
	}
	e.analysisIR = ctx.AnalysisIR
	e.mutable = ctx.Mutable
	e.multiGraphHandle = ctx.MultiGraph
	e.pendingSubRepos = append(e.pendingSubRepos[:0], ctx.PendingSubRepos...)
	e.requiredFiles = analyzerRequiredFilesFromIR(ctx)
	if explorerHistoryPrefersVCSNarrativePrincipal(ctx) {
		e.requiredFiles = nil
	}
	// Session-22 fix F2.1: cache the log-triage bundle so Check 6's
	// mid-loop gate can consult bundle.Meta.Signals + ResolvedFiles
	// without re-reading ctx (observeMidLoop takes only a
	// LoopObservation, which does not carry the bus context).
	e.logTriage = nil
	e.perfTrace = nil
	if ctx.Mutable != nil {
		e.logTriage = ctx.Mutable.LogTriage()
		e.perfTrace = ctx.Mutable.PerfTrace()
	}
	if ctx.AnalysisIR != nil {
		e.exactResolution = ctx.AnalysisIR.AnswerContract.ExactResolution
		e.exactPendingTargets = types.ExactResolutionPendingTargets(e.exactResolution, ctx.UnverifiedAnalyzerFindings)
	} else {
		e.exactResolution = nil
		e.exactPendingTargets = nil
	}
	e.exactContextFiles = nil
	// CGEC: capture the analyzer's AnswerSubject classification so
	// the chain ranker can score chain terminals against the
	// expected subject kind. Empty when AnalysisIR not yet available
	// (analyzer dispatch did not run, or sub-agent context).
	e.answerSubject = types.AnswerSubject{}
	e.predicateAxis = types.AxisUnknown
	e.kindConfidence = 0
	if ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); rm != nil {
			e.answerSubject = rm.AnswerSubject
			e.predicateAxis = rm.PredicateAxis
			e.kindConfidence = rm.KindConfidence
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
	e.isEnumerationQuery = enumerationIntentForContext(ctx)
	// Cache the orientation predicate alongside the enumeration flag
	// so mid-loop hints can branch without re-running the predicate.
	e.isOrientationQuery = false
	if ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); rm != nil {
			e.isOrientationQuery = types.IsProjectOrientationQuestion(*rm)
		}
	}
	e.structuredEvidence = nil
	e.flowFindings = nil
	e.cachedConcreteValues = nil
	e.primaryEntitiesRegistrationShape = false
	e.requiredFileHints = nil
	e.irrelevantFilesSet = nil
	e.midLoopLastResultsLen = 0
	e.midLoopSerialStreak = 0
	e.midLoopParallelInjected = false
	e.midLoopSymbolRefInjected = false
	e.midLoopPostPrimaryInjected = false
	e.midLoopBudgetExhaustedSent = nil
	e.midLoopEvidenceRepairSent = false
	e.midLoopEvidenceRepairResultsLen = 0
	e.midLoopSurfaceTermReviewSent = false
	e.midLoopClosureRepairSent = false
	e.midLoopClosureRepairResultsLen = 0
	e.midLoopIntentWindowSent = false
	e.midLoopRankerCoverageSent = false
	e.midLoopAbsentRedirectSent = false
	e.midLoopExternalArtifactSent = false
	e.midLoopExactAbsenceContextSent = false
	e.midLoopExactAbsenceSent = false
	e.midLoopSchemaLevelHintSent = false
	e.midLoopAuthoritativeTier1Sent = false
	e.midLoopEnumInjected = false
	e.midLoopOrientationFinalizeSent = false
	e.midLoopNoEmitPushSent = false
	e.midLoopNoEmitEscalated = false
	e.midLoopExecRedirectSent = false
	e.midLoopExplanationAnchorSent = false
	e.midLoopCompletionReadySent = false
	e.midLoopCompletionReadyEscalated = false
	e.midLoopCompletionReadyIter = 0
	e.midLoopNoEmitPushIter = 0
	e.midLoopNoEmitPushResultsLen = 0
	e.midLoopEmitBacklogBaseLen = 0
	e.primaryReadSeen = false
	e.primaryReadIter = 0
	e.notesLenAtPrimaryRead = 0
	e.mergedEmittedEvidenceLen = 0
	e.declarativeAnchorFiles = nil
	e.declarativeCandidateFiles = nil
	// Per-dispatch reset of the completion flag. Without this, a
	// Window 2 explore dispatch inherits Window 1's emit_investigation_complete
	// state and observeSoftStop immediately accepts the stop even if the
	// current window's LLM has done no tool work — defeating the "completion
	// is model-triggered" contract that emit_investigation_complete
	// enforces. Each dispatch must observe its OWN tool call before the
	// flag is true.
	e.investigationComplete = false

	if observationOnlyRuntimeArtifactForExplorer(ctx) {
		e.phase = 1
		return e.buildRuntimeObservationOnlyStartInstruction(ctx)
	}

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
		// Retry hint is rendered by builder.go as the top-level
		// "Retry Directive (READ FIRST)" user section; repeating it
		// here just duplicated the same content in the prompt.

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
		b.WriteString("Prefer the built-in `grep` / `read_file` tools for repository browsing. Reserve `exec_command` for deterministic computations or checks that the structured tools cannot perform directly.\n\n")
		b.WriteString("Evidence format (examples — adapt to what you find):\n")
		b.WriteString("- `[DIRECT] functionName line N: <what this code establishes>`\n")
		b.WriteString("- `[CONDITIONAL] functionName line N: <what happens> IF <condition>`\n")
		b.WriteString("- `[REGISTRATION] functionName line N: <what is registered, EXACT values>`\n\n")
		if guide := schemaLevelScopeGuide(e); guide != "" {
			b.WriteString(guide)
		}
		b.WriteString("**User question:** " + e.userQuestion)
		return b.String()
	}

	e.phase = 0 // start in breadth-scan phase

	var b strings.Builder
	b.WriteString("## Breadth Scan\n\n")
	b.WriteString("Your goal is to map a bounded candidate set for the user's question, not every broadly related file. ")
	b.WriteString("Do NOT read files in full yet. Use lightweight tools:\n")
	b.WriteString("- repo_map (task_map view) to get an overview of relevant files\n")
	b.WriteString("- grep with files_only=true to find WHICH FILES contain key terms (just filenames, not lines). Use `file_type` when the language is obvious; do not use --include so you discover all relevant file types\n")
	b.WriteString("- list_files to understand directory structure\n\n")
	b.WriteString("Prefer the built-in repository tools above for discovery. Reserve `exec_command` for deterministic computations or checks that the structured tools cannot perform directly.\n\n")
	b.WriteString("**Non-English questions:** When the user's question is not in English, search with BOTH the original terms AND their English programming equivalents. Most codebases use English identifiers, so always include the translated English terms alongside the original. Batch both versions as parallel grep calls.\n\n")
	b.WriteString("**Keyword variants:** Start with exact identifiers and high-confidence translations. Broaden only when those searches return zero or too few useful files:\n")
	b.WriteString("- Word roots and inflections (e.g. send/sending/sent)\n")
	b.WriteString("- Synonyms (e.g. send → emit, dispatch, publish, write)\n")
	b.WriteString("- Antonyms only when the user asks about absence, negation, disabling, or opposite behavior (e.g. lock → unlock, enable → disable)\n")
	b.WriteString("- Abbreviations and full forms (e.g. config → configuration, ctx → context)\n")
	b.WriteString("- CamelCase and snake_case variants (e.g. getUser, get_user, GetUser)\n")
	b.WriteString("Keep variants tied to the user's concrete nouns, symbols, config keys, or error frames; do not let generic domain words expand the search frontier by themselves.\n\n")
	b.WriteString("### Completion Handoff\n\n")
	b.WriteString("When you call `emit_investigation_complete`, make `reason` the concise investigation conclusion you want later answer writing to preserve, not just a generic \"done\" statement.\n")
	b.WriteString("- Include the terminal judgment, important scope boundaries, cross-repository or cross-component distinctions, no-hit/exclusion findings, and caveats that should not disappear.\n")
	b.WriteString("- Do not leave those conclusions only in free-form text before the tool call; put them in `reason` so the next stages receive them even when prose is absent or parallel investigations are merged.\n")
	b.WriteString("- Keep counts, complete member sets, and per-bucket facts in `aggregate_facts`; use `absence_justification` for a genuine zero or not-found result.\n")
	b.WriteString("- For a verified zero-result search inside a repository or sub-repo, use `aggregate_facts` with `kind=\"negative_search\"`, `value=\"0\"`, and dimensions for `repo`, `query` or `pattern`, `scope`, and `searched_at`. Do not emit fake file:line evidence such as `repo:0` for a not-found result.\n")
	b.WriteString("- For verified absence in non-repo evidence such as git history/diff output, an attached log, a trace, command output, or repo-map/index output, use `aggregate_facts` with `kind=\"negative_observation\"`, `value=\"0\"`, and dimensions for `origin`, `target` or `query`/`pattern`/`predicate`, `scope`, and `searched_at`. Do not invent repo dimensions for these cases.\n")
	b.WriteString("- Ground the conclusion in tool output and emitted evidence. The `reason` is preserved as context, not as a citation.\n\n")
	if ctx != nil && strings.TrimSpace(ctx.RepoRoot) != "" {
		b.WriteString("### Repository Command Scope\n\n")
		fmt.Fprintf(&b, "`exec_command` runs from the active repository root: `%s`. Use repo-relative paths; do not guess absolute checkout directories or run `cd` / `git -C` / `--git-dir` outside this root. For commit history and diff questions, prefer `git_log`, `git_show`, `git_diff`, and `git_history_search` before free-form shell; use `git_show` for a specific commit/ref's metadata/patch/stat/name-only output, and use `git_history_search.order=recent` for latest/last-N windows or `order=oldest` for first-introduced / earliest-occurrence windows.\n\n",
			ctx.RepoRoot)
	}
	if ctx != nil && ctx.AnalysisIR != nil && ctx.AnalysisIR.RequestModel.Predicates.IsHistoryLookup {
		if explorerHistoryPrefersVCSNarrativePrincipal(ctx) {
			b.WriteString("### VCS History Narrative Handoff\n\n")
			b.WriteString("This question's principal evidence is repository history: commits, refs, subjects, authorship, diff/stat/name-only output, and verified history-search results. Use the VCS tools first and do not turn commit metadata into fake source `emit_evidence` rows.\n")
			b.WriteString("- Put the concise history conclusion in `emit_investigation_complete.reason`, including the relevant commits, what each changed, how the changes relate, and any scope boundary.\n")
			b.WriteString("- Use `aggregate_facts` only as supporting VCS metadata unless the user asked for a scalar/count/list; the narrative answer should remain richer than one commit hash or one raw value.\n")
			b.WriteString("- Current-source files are optional support in this lane. Read them only when the VCS diff points to a file whose current state is needed for a caveat; do not keep searching source files solely to satisfy a source-code coverage habit.\n")
			b.WriteString("- A grounded VCS conclusion may close the investigation without reading preselected current-source files.\n\n")
		} else {
			b.WriteString("### Mixed History / Current-source Handoff\n\n")
			b.WriteString("This history question also needs current-source, diagram, diagnostic, comparison, or change-impact evidence. Keep the lanes separate: VCS tools establish what changed and why; `read_file` evidence establishes current implementation, current risk, or code-flow claims.\n")
			b.WriteString("- Do not let a commit hash or diff stat replace the user's requested code explanation.\n")
			b.WriteString("- Do not let current source evidence erase the VCS conclusion. Preserve both in `emit_investigation_complete.reason`, and emit source `emit_evidence` only for current-code claims that the final answer must cite.\n\n")
		}
	}

	analyzerKeywords := irKeywords(ctx)
	analyzerEntities := irEntities(ctx)
	analyzerKind := irQuestionKind(ctx)
	e.analyzerKeywords = append(e.analyzerKeywords[:0], analyzerKeywords...)

	if len(analyzerKeywords) > 0 {
		display := analyzerKeywords
		if len(display) > 15 {
			display = display[:15]
		}
		b.WriteString("### Suggested Search Terms\n\n")
		b.WriteString("Use these for grep (from the classification above):\n`")
		b.WriteString(strings.Join(display, "`, `"))
		b.WriteString("`\n\n")
	}
	if e.exactResolution != nil && e.exactResolution.TargetKind == types.SubjectConfigKey {
		b.WriteString("### Exact-Key Search Discipline\n\n")
		b.WriteString("- For exact config-key questions, test assertions, documentation examples, and prompt strings that merely mention the token are context only, not defining proof.\n")
		b.WriteString("- Prefer the built-in `grep` tool for repo-wide token sweeps. It keeps breadth scans path-stable on every OS and avoids shell-specific path drift.\n")
		b.WriteString("- Spend read_file budget on production config/default/loader/flag-binding surfaces before repairing doc/test mentions.\n\n")
	}
	if ctx != nil && ctx.AnalysisIR != nil && ctx.AnalysisIR.RequestModel.EnumerationBoundary != nil {
		boundary := ctx.AnalysisIR.RequestModel.EnumerationBoundary
		if boundary.DeclaredCount > 0 && strings.TrimSpace(boundary.SourceQuote) != "" {
			b.WriteString("### Requested Principal Set Boundary\n\n")
			fmt.Fprintf(&b, "The user explicitly declared a bounded principal set: `%s` (%d item(s)). Preserve that boundary while you investigate.\n\n",
				boundary.SourceQuote, boundary.DeclaredCount)
			b.WriteString("- If the same owner/file also contains adjacent guards, coherence checks, repair hooks, compatibility shims, or later-added caveats beyond that boundary, do not promote them into the principal set just because they are nearby in code.\n")
			b.WriteString("- Emit the principal bounded set as defining evidence first. Emit extra adjacent items only as `related_context` or `illustrative_only` when they matter as caveats.\n")
			b.WriteString("- When the main set is ordered in one owner file, prefer the owner's primary append/registration sequence itself over drifting into sibling helper definitions unless the boundary cannot be grounded otherwise.\n\n")
		}
	}
	// Plan E (2026-05-02) — surface CompletenessObligation + Buckets
	// to the explorer so the investigation knows it must be exhaustive
	// (G1 will refuse premature emit_investigation_complete) and which
	// user-named groups the answer must reproduce.
	if ctx != nil && ctx.AnalysisIR != nil {
		rm := ctx.AnalysisIR.RequestModel
		if rm.CompletenessObligation.IsActive() {
			b.WriteString("### Exhaustive-coverage Obligation\n\n")
			fmt.Fprintf(&b, "The user demands every match (`%s` in the question). Every grep / repo_map / list_files candidate file MUST be either read_file'd OR explicitly excluded by a narrower follow-up grep before you call emit_investigation_complete with result_kind='resolved'. The framework refuses premature completion when scanned candidates remain unread under this obligation. The honest fallback when the investigation legitimately cannot enumerate the full set is result_kind='absence' with absence_justification, OR an emit_investigation_complete that explicitly notes the un-read scope.\n\n",
				rm.CompletenessObligation.SourceQuote)
		}
		if rm.Predicates.IsCountQuestion || len(rm.Buckets) >= 2 || rm.EnumerationBoundary != nil || (rm.CompletenessObligation != nil && rm.CompletenessObligation.Required) || types.RequiresExhaustiveEnumerationMemberSetHandoff(rm) {
			b.WriteString("### Structured Aggregate Handoff\n\n")
			b.WriteString("When the answer depends on derived totals, unique-set sizes, per-dimension counts, user-bucket counts, excluded-candidate counts, or an exhaustive principal member list, include `aggregate_facts` in your successful `emit_investigation_complete` call. Do this even when the same numbers or member names also appear in your reason prose.\n")
			b.WriteString("- Use `member_set` for an exact exhaustive list of principal answer members, `total_count` for the principal hit count, `unique_count` for distinct file/package/module sets, `grouped_count` for syntax/category/language dimensions, `bucket_count` for user-named partitions, and `excluded_count` for comments/tests/docs/unrelated candidates that were deliberately not counted.\n")
			b.WriteString("- Put concrete members in `members` when they are part of the user's requested answer, such as enum/type names, file:line labels, or distinct file paths. A `member_set` or count fact with `members` is treated as an exact set, so the number of members must match `value`; omit count members rather than provide samples.\n")
			b.WriteString("- When a `member_set` member is shaped \"<code identifier> (<qualifier>)\" — e.g. `Orchestrator (4-stage pipeline)` — you MUST attach `support_refs` mapping each decorated member to a grounded file:line. The decorator changes the surface text so the framework cannot auto-resolve the member against an evidence anchor named just `Orchestrator`. Two accepted forms: labeled `[\"Orchestrator: codrax/internal/orchestrator/orchestrator.go:42\", \"Gate.Run: codrax/internal/analysis/gate/gate.go:128\"]` (label = the bare leading identifier of each member, no decorator), or positional `[\"codrax/internal/orchestrator/orchestrator.go:42\", \"codrax/internal/analysis/gate/gate.go:128\"]` with one entry per `members[]` in the same order. Bare code-identity members (no decorator) and pure display-prose members keep auto-resolution; only decorated code-shape members trip emit-time rejection when support_refs is empty.\n")
			b.WriteString("- When a count fact's members are source locations spanning multiple files, also emit a companion `unique_count` fact for the distinct file set. Put rejected candidates in `excluded` rather than mixing them into principal members.\n")
			b.WriteString("- Values must come from your verified command output, grounded evidence, or explicit candidate classification. Do not leave later answer writing to recompute aggregates from prose.\n\n")
		}
		if types.HasAttributeBearingEnumeration(rm) {
			b.WriteString("### Attribute-bearing Enumeration Discipline\n\n")
			b.WriteString("This request has two axes: an exhaustive principal member set plus a per-member attribute. Keep those axes separate while investigating. Coverage completeness applies to the principal members; attribute certainty is recorded per member.\n")
			b.WriteString("- Support all language surfaces the repo map can expose: packages, modules, namespaces, crates, directories, files, classes, functions, methods, routes, config keys, and registry entries. Do not assume Go-only package/function naming.\n")
			b.WriteString("- Once a principal member has a grounded attribute candidate, emit that candidate as evidence. If the attribute is ambiguous or not grounded for that member after the relevant file/list/grep scope has been checked, record the member with an explicit unresolved-attribute note instead of widening indefinitely.\n")
			b.WriteString("- When the bounded / exhaustive principal member set is covered, call emit_investigation_complete. Do not keep searching solely to prove a unique attribute when the final answer can carry a caveat for that member.\n\n")
		}
		if types.ResolveQuestionFamily(rm) == types.QFArchitecture {
			b.WriteString("### Architecture Role / Output Handoff\n\n")
			b.WriteString("Architecture answers need both component boundaries and role/output detail. For each stage, layer, module, agent, subsystem, or pipeline component you intend downstream synthesis to describe, emit structured evidence for:\n")
			b.WriteString("- the component anchor itself (`definition`, `registration`, `import`, or call/dispatch relation when that is the real boundary);\n")
			b.WriteString("- the role or responsibility it performs, preferably as `evidence_kind=\"mechanism\"` with grounded subject/object fields and a concise model-authored summary;\n")
			b.WriteString("- any typed output artifact, plan, IR, report, verdict, state transition, or handoff product it produces/consumes when the source line or doc comment names one.\n")
			b.WriteString("Use `surface_terms` for exact artifact labels visible in the read lines, and set `load_bearing_summary=true` only when the role/output wording cannot be reconstructed from subject/object/anchor/snippet alone. These role/output facts enrich sections; they do not create extra architectural layers unless the boundary evidence also makes them principal.\n\n")
		}
		if rm.FieldValueProfile != nil && rm.FieldValueProfile.Active() {
			b.WriteString("### Field/Value Count Discipline\n\n")
			fmt.Fprintf(&b, "This request asks for a scalar/count about `%s = %s`. Search every syntax family that can express the same assignment before closing:\n",
				rm.FieldValueProfile.Target, rm.FieldValueProfile.Literal)
			b.WriteString("- Full selector/member writes, such as `Owner.Field = value` or `object.Owner.Field = value`.\n")
			b.WriteString("- Aggregate/object/named-argument literals, such as `Owner{Field: value}`, `Owner: { Field: value }`, `Field = value`, and C/C++ designated initializers like `.field = value` when the nearby initializer owner is the requested type/member.\n")
			b.WriteString("- Keep the owner context and the leaf field/value surface together when filtering matches. A bare leaf field in an unrelated owner is context, not a principal hit.\n")
			b.WriteString("- Classify every candidate as production assignment, comment/doc/test, or unrelated. Do not close on a direct selector grep if aggregate/object/designated-initializer candidates remain unread.\n\n")
		}
		if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() && rm.ChangeImpactProfile.RequestedBroadAffectedSites() {
			target := strings.TrimSpace(rm.ChangeImpactProfile.Target)
			if target == "" {
				target = "the changed target"
			}
			b.WriteString("### Change-impact Principal Evidence Handoff\n\n")
			fmt.Fprintf(&b, "The active change-impact target is `%s`. For every affected production site you intend downstream synthesis to list as a principal file/site/symbol, emit grounded evidence whose structured fields carry the target or owner-qualified member path. Summary prose alone is not a handoff.\n\n", target)
			b.WriteString("- Put the actual cited source line in `snippet` when the line contains the affected selector/member/config path.\n")
			b.WriteString("- Use `anchor_kind=\"initializer\"` for struct/object/named-argument/designated/config member initializer lines; use `anchor_kind=\"assignment\"` for direct writes such as `:=` or `=`.\n")
			b.WriteString("- Preserve the target in `anchor_symbol`, `subject`, `object`, `condition`, or `surface_terms` when that surface is visible on the already-read line. For owner-qualified fields, keep owner + member together rather than emitting only the leaf field name.\n")
			b.WriteString("- If a line is merely a comment, documentation note, adjacent helper, or different owner with the same leaf name, emit it as context (`related_context` / `illustrative_only`) instead of a principal affected site.\n")
			b.WriteString("- This applies across languages: selectors, namespace/member paths, object literals, designated initializers, property accesses, imports, config keys, generated/build declarations, and bridge adapters should all use the same structured evidence fields rather than relying on final-answer reconstruction.\n\n")
		}
		if len(rm.Buckets) >= 2 {
			labels := make([]string, 0, len(rm.Buckets))
			for _, bk := range rm.Buckets {
				labels = append(labels, fmt.Sprintf("`%s`", bk.Label))
			}
			b.WriteString("### User-named Partition\n\n")
			fmt.Fprintf(&b, "The user split the answer into %d named groups: %s. Investigate each bucket with comparable depth — equal read_file calls per bucket, comparable evidence emission. The downstream answer renderer enforces verbatim use of each label, so investigate to the point you can ground each bucket's items independently.\n\n",
				len(rm.Buckets), strings.Join(labels, ", "))
		}
	}

	// Conditional-enumeration hint: when the question asks "how many X
	// can Y" / "有几个X可以Y", the answer requires enumerating ALL
	// candidates, verifying each against a condition, and counting.
	// The LLM tends to short-circuit (reading the total registration
	// count instead of filtering). This directive makes the required
	// reasoning strategy explicit. Fires when question_kind is
	// enumeration and the question contains a relational verb.
	if e.isEnumerationQuery && analyzerKind == "enumeration" {
		// Phase 6 stage 25 (2026-05-03) — typed relational-lookup
		// detection. The retired 12-keyword EN+ZH table ({"调用",
		// "invoke", "call", "register", "实现", "implement", "use",
		// "使用", "access", "触发", "trigger", "可以"}) scanned
		// user-question prose; replaced by typed
		// Predicates.IsRelationalLookup boolean + PredicateAxis enum
		// equality on the analyzer's structured emit. Per the
		// no-keyword-classification red line.
		hasRelationalVerb := false
		if rm := requestModelFromContext(ctx); rm != nil {
			if rm.Predicates.IsRelationalLookup {
				hasRelationalVerb = true
			}
			switch rm.PredicateAxis {
			case types.AxisCall, types.AxisRegister, types.AxisImplement:
				hasRelationalVerb = true
			}
		}
		if hasRelationalVerb {
			b.WriteString("### ⚠ Counting Strategy (CRITICAL)\n\n")
			b.WriteString("This question asks HOW MANY items satisfy a CONDITION. You MUST:\n")
			b.WriteString("1. **Enumerate ALL candidates** — find every instance of the subject type in the repository\n")
			b.WriteString("2. **Verify each candidate** — check whether it satisfies the predicate stated in the question\n")
			b.WriteString("3. **Count only the qualifying ones** — do NOT use a total registration/declaration count as the answer\n\n")
			b.WriteString("Common mistake: treating the total number of declared items as the answer without filtering by the condition. The qualifying count is often strictly smaller — possibly 0 or 1.\n\n")
		}
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
		//
		// T1.2: cache result across explorer redispatches within a
		// Run. keyword_search is deterministic in its four inputs
		// (keywords/entities/domain hints/maxFiles) and analyzer
		// only runs once per pipeline, so the second+ explorer
		// dispatch repeats identical work. Cross-run reset clears
		// searchFingerprint so a fresh REPL turn still recomputes.
		domainHints := irDomainHints(ctx)
		entityProvenance := irEntityProvenance(ctx)
		searchEntities := filterEntitiesByProvenance(analyzerEntities, entityProvenance, entityProvenanceRoleSearch)
		maxFiles := MaxFilesForComplexity(irComplexity(ctx))
		exactContract := irExactResolutionContract(ctx)
		sourceScope := irSourceScopeProfile(ctx)
		var exactTargets []string
		exactPolicy := ""
		if exactContract != nil {
			exactTargets = exactContract.Targets
			exactPolicy = string(exactContract.RelatedContextPolicy)
		}
		fp := keywordSearchScopedFingerprint(ctx, keywordSearchFingerprint(analyzerKeywords, searchEntities, irMentionedEntities(ctx), irPrimaryEntities(ctx), domainHints, exactTargets, exactPolicy, maxFiles, false, sourceScope.Fingerprint()))
		sharedFP := fp
		var sr *keywordSearchResult
		if e.searchResult != nil && e.searchFingerprint != "" && e.searchFingerprint == fp {
			logging.Debug("[keyword_search] cache hit fp=%s (%d files, %d keywords)",
				fp, len(e.searchResult.Files), len(analyzerKeywords))
			sr = e.searchResult
		} else if cached, ok := e.sharedSearchCache.Get(sharedFP); ok {
			logging.Debug("[keyword_search] shared cache hit fp=%s (%d files, %d keywords)",
				fp, len(cached.Files), len(analyzerKeywords))
			sr = cached
			e.searchResult = sr
			e.searchFingerprint = fp
			e.multiGraphHandle = ctx.MultiGraph
		} else {
			sr = keywordSearchWithOptions(analyzerKeywords, ctx.RepoRoot, keywordSearchOptions{
				Entities:          analyzerEntities,
				EntityProvenance:  entityProvenance,
				MentionedEntities: irMentionedEntities(ctx),
				PrimaryEntities:   irPrimaryEntities(ctx),
				DomainHints:       domainHints,
				MaxFiles:          maxFiles,
				ExactResolution:   exactContract,
				SourceScope:       sourceScope,
				MultiGraph:        ctx.MultiGraph,
				PendingSubRepos:   ctx.PendingSubRepos,
			})
			e.searchResult = sr
			e.searchFingerprint = fp
			// Cache the multigraph carrier for mid-loop hooks
			// (observeMidLoop) that don't get ctx — Sc 1
			// implementer fan-out at line 6406 reads e.multiGraphHandle.
			e.multiGraphHandle = ctx.MultiGraph
			e.sharedSearchCache.Put(sharedFP, sr)
		}
		if sr != nil {
			e.exactAnchorFiles = exactAnchorFilesFromScores(sr.Files)
			if len(e.exactPendingTargets) > 0 {
				// An exact target that findings_validator still marks
				// pending / unverified must NOT hard-focus the
				// investigation onto a single "exact anchor" file. That
				// path is exactly how absence-candidate questions drift
				// into nearby-family files and start treating context as
				// the answer. Keep the exact-resolution contract active,
				// but suppress the unique-anchor fast path until
				// exploration proves the target concretely.
				e.exactAnchorFiles = nil
			}
			e.requiredFiles = e.filterRequiredFiles(e.requiredFiles, ctx)
			// L1 (2026-05-10) — cache the structural registration-shape
			// check once per dispatch. The result gates
			// declarativeFocusRelevant's enumeration branch on whether
			// PrimaryEntities actually land in registry-shaped files.
			// Computing it here (after sr.Graph is available + before
			// any declarative-file builder runs) ensures every
			// downstream call sees a consistent answer.
			declAllowed := declarativeAllowedKinds(analyzerKind, e.predicateAxis)
			if ctx != nil {
				e.primaryEntitiesRegistrationShape = primaryEntitiesLookLikeRegistration(ctx.AnalysisIR, sr.Graph, declAllowed)
				// L3 cache — analyzer-emitted per-file hints.
				// Validate against the repomap graph: hints whose
				// `path` is not in graph.FileIndex are LLM
				// hallucinations (typo, fabricated file, wrong
				// directory) and dropped here so they never reach
				// downstream consumers. graph membership is the
				// authoritative "exists in this repo" signal —
				// stronger than os.Stat (the graph already filtered
				// out .gitignore'd, binary, and language-irrelevant
				// files) and free (no extra I/O).
				if ctx.AnalysisIR != nil {
					rawHints := ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints
					e.requiredFileHints = filterValidRequiredFileHints(rawHints, sr.Graph, ctx)
					// L3 observability — log the high/soft/dropped
					// distribution per dispatch so operators can
					// audit how often the analyzer emits useful
					// per-file confidence and how often the graph
					// validator drops hallucinated entries. Only
					// emits when the analyzer actually populated
					// the field (avoid log noise on every dispatch).
					if len(rawHints) > 0 {
						high, soft, low := 0, 0, 0
						for _, h := range e.requiredFileHints {
							switch {
							case h.Confidence >= requiredFileHintHighConfidence:
								high++
							case h.Confidence >= requiredFileHintSoftConfidence:
								soft++
							default:
								low++
							}
						}
						logging.Debug("[explorer] required_file_hints: emitted=%d kept=%d (high=%d soft=%d low=%d) dropped_by_graph=%d",
							len(rawHints), len(e.requiredFileHints), high, soft, low, len(rawHints)-len(e.requiredFileHints))
					}
					// L4 — analyzer-declared irrelevant files.
					// Index by canonical path for O(1) lookup at
					// every consumer site.
					//
					// 2026-05-10 P3 audit follow-up: irrelevant_files
					// is a HARD exclusion (drops the path from
					// primary set + pre-read pool + mid-loop hints).
					// If the analyzer LLM mistakenly declares the
					// real answer file irrelevant, the explorer
					// would never surface it. Two-layer validation:
					//
					//   (i) Graph membership — a path the LLM
					//       hallucinated isn't in the repo. Drop it
					//       silently (no useful exclusion possible
					//       on a non-existent file).
					//
					//   (ii) Defer to evidence — when the explorer
					//        has ALREADY emit_evidence'd from a
					//        path, its judgement has more grounding
					//        than the analyzer's pre-investigation
					//        guess. Drop those exclusions too so
					//        the explorer's own citations win.
					//        (The structuredEvidence slice is
					//        empty on first dispatch, so this layer
					//        only fires on subsequent dispatches
					//        within the same Run.)
					raw := ctx.AnalysisIR.RequestModel.AnalyzerHints.IrrelevantFiles
					if len(raw) > 0 {
						citedSources := make(map[string]bool, len(e.structuredEvidence))
						for _, ev := range e.structuredEvidence {
							if ev.Producer != tool.EmitEvidenceProducer {
								continue
							}
							if cs := canonicalEvidenceSourcePath(ev.Source); cs != "" {
								citedSources[cs] = true
							}
						}
						set := make(map[string]bool, len(raw))
						droppedNoGraph := 0
						droppedCited := 0
						for _, p := range raw {
							canon := canonicalExplorerAgentPath(ctx, p)
							if canon == "" {
								continue
							}
							// Layer (i): graph membership.
							if sr.Graph != nil && sr.Graph.FileIndex != nil {
								if _, ok := sr.Graph.FileIndex[canon]; !ok {
									if !explorerAgentPathExists(ctx, canon) {
										droppedNoGraph++
										continue
									}
								}
							}
							// Layer (ii): defer to explorer's own citations.
							if citedSources[canon] {
								droppedCited++
								continue
							}
							set[canon] = true
						}
						e.irrelevantFilesSet = set
						logging.Debug("[explorer] irrelevant_files: declared=%d kept=%d (dropped_no_graph=%d, dropped_cited=%d)",
							len(raw), len(set), droppedNoGraph, droppedCited)
					} else {
						e.irrelevantFilesSet = nil
					}
				} else {
					e.requiredFileHints = nil
					e.irrelevantFilesSet = nil
				}
			} else {
				// No ctx → fail-open (preserve legacy behaviour).
				e.primaryEntitiesRegistrationShape = true
				e.requiredFileHints = nil
			}
			e.declarativeAnchorFiles = declarativeAnchorFilesFromScores(sr.Files, analyzerKind, e.predicateAxis, e.isEnumerationQuery, e.primaryEntitiesRegistrationShape)
			e.declarativeCandidateFiles = nil
			if len(e.declarativeAnchorFiles) == 0 && len(e.requiredFiles) > 0 {
				e.declarativeAnchorFiles = declarativeAnchorFilesFromPaths(e.requiredFiles, analyzerKind, e.predicateAxis, e.isEnumerationQuery, e.primaryEntitiesRegistrationShape)
				if len(e.declarativeAnchorFiles) == 0 {
					e.declarativeCandidateFiles = structuralCandidateFilesFromPaths(e.requiredFiles, analyzerKind, e.predicateAxis, e.isEnumerationQuery, e.primaryEntitiesRegistrationShape)
				}
			}
		}
		results := e.filterKeywordResults(sr.Files)
		// Publish the graph to MutableState so tools running later in
		// this dispatch (notably emit_evidence's synchronous grounder)
		// can consult it without re-invoking BuildOrLoadGraph. The
		// handle is `any` on the types side to keep internal/types
		// decoupled from repomap; tool-side consumers type-assert.
		if ctx != nil && ctx.Mutable != nil && sr != nil && sr.Graph != nil {
			ctx.Mutable.SetSearchGraph(sr.Graph)
			// 2026-05-10 P1: stash the SymbolOracle so the pre-emit
			// chokepoint in internal/tool can run the enumeration-
			// label grounding check without importing
			// internal/tool/repomap (cycle: repomap → tool →
			// repomap). The oracle is stateless / read-only so
			// passing the same instance to every tool call is safe.
			ctx.Mutable.SetSymbolOracle(repomap.NewSymbolOracle(sr.Graph))
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
		if ctx != nil && ctx.Mutable != nil && (len(results) > 0 || len(e.requiredFiles) > 0) {
			seen := make(map[string]bool, len(results)+len(e.requiredFiles))
			ranked := make([]types.Phase1RankedFile, 0, len(results)+len(e.requiredFiles))
			for _, f := range results {
				canon := ground.CanonicalRepoRelative(f.Path, ctx.RepoRoot)
				ranked = append(ranked, types.Phase1RankedFile{
					Path:            canon,
					Score:           f.Score,
					ExactEntityRank: f.ExactEntityRank,
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
			for _, p := range e.requiredFiles {
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
		// Record that pre-scan produced a repo_map-derived ranked file
		// list the LLM can see. Read by the Phase 0 quality gate.
		e.hasPrescanRepoMap = len(results) > 0
		// T3a: surface the analyzer's EvidencePlan.RequiredFiles as
		// a dedicated prompt section. Files appear first in the
		// instruction text so the LLM sees "start here" guidance
		// before scanning the longer keyword_search ranking below.
		// Only render when non-empty AND complexity is not simple
		// (simple questions don't need the multi-file push).
		if reqFiles := e.requiredFiles; len(reqFiles) > 0 &&
			irComplexity(ctx) != types.ComplexitySimple {
			// Session-22 F2.2: split log-frame files from ranker files
			// so the LLM treats panic/exception anchors as authoritative
			// (must-read) and ranker candidates as opt-in cross-refs.
			logFiles, rankerFiles := e.partitionRequiredFilesByLogTriage(reqFiles)
			if len(logFiles) > 0 {
				b.WriteString("### Frames from the attached log\n\n")
				b.WriteString(fmt.Sprintf(
					"The attached runtime log's stack frames resolved to these repo files. "+
						"They are the authoritative anchors for the failure — read them first and base "+
						"any call-chain / sequence diagram in the answer on the Call chain block in the "+
						"%s section, not on the Auxiliary candidates below.\n\n",
					promptctx.SectionLogTriageExtraction))
				for _, p := range logFiles {
					b.WriteString("- `" + p + "`\n")
				}
				b.WriteString("\n")
				if guidance := e.renderAuthoritativeFrameStartSection(logFiles); guidance != "" {
					b.WriteString(guidance)
				}
			}
			if len(rankerFiles) > 0 {
				if len(logFiles) > 0 {
					b.WriteString("### Auxiliary candidates (opt-in cross-references)\n\n")
					b.WriteString("The keyword ranker flagged these files as entity matches. " +
						"Open them ONLY when the evidence chain visibly crosses file boundaries " +
						"beyond the log-frame anchors above. Do NOT cite these in the answer's " +
						"call-chain diagram unless you observed a direct call to or from them.\n\n")
				} else {
					b.WriteString("### Analyzer's Required Files\n\n")
					b.WriteString("The analyzer's repo_map query over the question's entities " +
						"identified these files as structurally relevant. Start your investigation " +
						"here and trace cross-file references outward:\n\n")
				}
				for _, p := range rankerFiles {
					b.WriteString("- `" + p + "`\n")
				}
				b.WriteString("\n")
			}
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
			var ermPreds types.SemanticPredicates
			var ermRM *types.RequestModel
			if ctx != nil && ctx.AnalysisIR != nil {
				ermPreds = ctx.AnalysisIR.RequestModel.Predicates
				ermRM = &ctx.AnalysisIR.RequestModel
			}
			e.ermRequirements = extractEvidenceRequirementsWithModel(
				ermKeywordSource, ermEntities, analyzerKind, ermPreds, ermRM,
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
			if banner := e.buildExactResolutionScopeBanner(ctx, analyzerKeywords); banner != "" {
				b.WriteString(banner)
			}
			if capabilityQuery := detectStageToolCapabilityQueryFromContext(ctx); capabilityQuery != nil {
				e.phase = 1
				e.requiredFiles = capabilityFocusedFiles(capabilityQuery, e.requiredFiles)
				return e.buildCapabilityFocusedStartInstruction(ctx, analyzerKeywords, capabilityQuery)
			}
			if e.shouldStartFocusedDepth(analyzerKind) {
				e.phase = 1
				e.tightenFocusedFrontier()
				return e.buildFocusedDepthStartInstruction(ctx, analyzerKeywords)
			}
			if e.shouldStartDeclarativeDepth(analyzerKind) {
				e.phase = 1
				e.tightenDeclarativeFrontier()
				return e.buildDeclarativeFocusedStartInstruction(ctx, analyzerKeywords)
			}
			if e.shouldStartDeclarativeCandidateDepth(analyzerKind) {
				e.phase = 1
				e.tightenDeclarativeCandidateFrontier()
				return e.buildDeclarativeCandidateStartInstruction(ctx, analyzerKeywords)
			}
			if e.shouldStartPrimaryEntityDepth(analyzerKind) {
				e.phase = 1
				e.requiredFiles = e.filterRequiredFiles(e.requiredFiles, ctx)
				e.tightenPrimaryEntityFrontier()
				return e.buildPrimaryEntityDepthStartInstruction(ctx, analyzerKeywords)
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
	b.WriteString("- Files that DECLARE rules or topology are as important as files that IMPLEMENT logic — include both in your list\n\n")

	if guide := schemaLevelScopeGuide(e); guide != "" {
		b.WriteString(guide)
	}

	return b.String()
}

// schemaLevelScopeGuide returns the dispatch-time instruction block
// that nudges the LLM toward emitting schema-level scopes (file /
// crossfile / negative) when the analyzer's classification suggests
// the question's facts are layer-shaped, contract-shaped, or
// absence-shaped rather than per-line.
//
// Triggers when (a) the analyzer set Scenario=config_trace AND
// AnswerSubject.Kind=config_key — the strongest signal that schema-
// level evidence is going to be needed — OR (b) any ExactResolutionContract
// has AllowAbsence set, which signals the analyzer expects the
// target may be absent.
//
// Returns "" when the conditions don't apply, so the caller can
// guard with `if guide := schemaLevelScopeGuide(e); guide != ""`.
func schemaLevelScopeGuide(e *explorerEvaluator) string {
	if e == nil {
		return ""
	}
	configTrace := false
	if e.analysisIR != nil {
		rm := e.analysisIR.RequestModel
		if rm.Scenario == types.ScenarioConfigTrace &&
			rm.AnswerSubject.Kind == types.SubjectConfigKey {
			configTrace = true
		}
	}
	allowAbsence := e.exactResolution != nil && e.exactResolution.AllowAbsence
	if !configTrace && !allowAbsence {
		return ""
	}

	var b strings.Builder
	b.WriteString("**Anchor scope (when emitting evidence):** this question's classification suggests the facts are likely layer-shaped, contract-shaped, or absence-shaped — not all per-line. Prefer the matching scope on emit_evidence:\n")
	b.WriteString("- `scope=file` for each layer-canonical file you identify (set source to the file path; set file_role_label to `config_canonical` / `cli_registration` / `default_struct` / `manifest` / `schema`). This anchors the file's identity AS a layer regardless of whether the target appears at any specific line in it.\n")
	b.WriteString("- `scope=negative` to make a confirmed absence a citable fact (kind=`absent`, negative_query={file, pattern: <target>}, negative_scope=`file`/`section`/`struct_fields`).\n")
	b.WriteString("- `scope=crossfile` for cross-file contracts you can verify by grep (crossfile_query={files, pattern} + crossfile_assertion={kind: `exists`/`forbidden`/`count_eq`}). The system re-runs the query so emit only what you can back.\n")
	b.WriteString("Use `scope=line` for evidence anchored at one specific code location; do NOT use it as a fallback for layer / contract / absence facts. The answer surface is stronger when the evidence shape matches the fact shape.\n\n")
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

func (e *explorerEvaluator) uniquePrimaryEntityFile() (string, bool) {
	primary := e.primaryEntityFiles()
	if len(primary) != 1 {
		return "", false
	}
	path := canonicalExplorerPath(primary[0])
	if path == "" {
		return "", false
	}
	return path, true
}

// effectivePrimaryFiles returns primaryEntityFiles unioned with the
// repo-relative files the explorer's own emit_evidence has already
// cited. Two motivations (2026-05-10):
//
//  1. The LLM has read the file and judged it relevant enough to
//     emit grounded evidence. That is a stronger primary-file
//     signal than the analyzer's pre-investigation entity→file
//     resolver, which can fail in two ways: package-qualified
//     names ("gate.Run", "mod::Type::method") that don't match
//     graph keys, and over-broad analyzer Required Files that
//     pull in irrelevant files like internal/types/analysis_ir.go.
//
//  2. The single-primary evidence filter (filterEvidenceByPrimaryFiles
//     branch where len(primary) == 1) is destructive: it drops
//     every item not from the single primary file, including
//     LLM-emitted evidence pointing at the *real* primary file the
//     analyzer missed. Augmenting the primary set with emit-cited
//     files prevents the filter from inverting the LLM's own
//     citations.
//
// Order: analyzer-derived primary files come first (preserved order),
// then emit-cited files in iteration order. Deduplicated by canonical
// repo-relative path. Returns nil when both lanes are empty.
func (e *explorerEvaluator) effectivePrimaryFiles() []string {
	primary := e.primaryEntityFiles()
	cited := make(map[string]bool)
	for _, ev := range e.structuredEvidence {
		if ev.Producer != tool.EmitEvidenceProducer {
			continue
		}
		canon := canonicalEvidenceSourcePath(ev.Source)
		if canon == "" {
			continue
		}
		cited[canon] = true
	}
	// L3 (2026-05-10): high-confidence analyzer hints (≥0.8) join the
	// primary-files set. The LLM has full context (user question +
	// repo summary + pre-scan results) and explicitly judged these
	// files needed; honoring the recommendation supersedes the
	// resolver's heuristic when the LLM is confident.
	highConfHints := make(map[string]bool)
	for _, h := range e.requiredFileHints {
		if h.Confidence < requiredFileHintHighConfidence {
			continue
		}
		canon := canonicalExplorerPath(h.Path)
		if canon == "" {
			continue
		}
		highConfHints[canon] = true
	}
	if len(primary) == 0 && len(cited) == 0 && len(highConfHints) == 0 {
		return nil
	}
	// L4 (2026-05-10): drop analyzer-declared irrelevant files at
	// every accumulator. This honors the LLM's negative judgment
	// across analyzer-derived primaries (where the resolver might
	// land on a file the LLM already inspected and rejected),
	// emit-cited paths (defensive), AND high-confidence hints
	// (defensive — LLM should not contradict itself, but rule wins).
	dropIrrelevant := func(canon string) bool {
		if len(e.irrelevantFilesSet) == 0 {
			return false
		}
		return e.irrelevantFilesSet[canon]
	}
	seen := make(map[string]bool, len(primary)+len(cited)+len(highConfHints))
	out := make([]string, 0, len(primary)+len(cited)+len(highConfHints))
	for _, p := range primary {
		canon := canonicalExplorerPath(p)
		if canon == "" || seen[canon] || dropIrrelevant(canon) {
			continue
		}
		seen[canon] = true
		out = append(out, p)
	}
	for canon := range cited {
		if seen[canon] || dropIrrelevant(canon) {
			continue
		}
		seen[canon] = true
		out = append(out, canon)
	}
	for canon := range highConfHints {
		if seen[canon] || dropIrrelevant(canon) {
			continue
		}
		seen[canon] = true
		out = append(out, canon)
	}
	return out
}

// L3 threshold bands. Mirrors the established
// kindConfidenceFloorForNarrowing (0.7) pattern but slightly higher
// since per-file judgements are easier for an LLM than meta-
// classification. Values are tuned to give ≥0.8 a strong "primary"
// signal while ≥0.5 catches softer "this might help" hints.
const (
	requiredFileHintHighConfidence = 0.8
	requiredFileHintSoftConfidence = 0.5
)

// filterValidRequiredFileHints drops hints whose `Path` does not
// resolve to a real file in the repomap graph. The graph's
// FileIndex is the canonical "exists in this repo" set — stronger
// than os.Stat (the graph already filtered out .gitignore'd, binary,
// and language-irrelevant files) and free (no extra I/O).
//
// Hallucinated paths from the analyzer (typo, wrong directory,
// fabricated file) are silently dropped here so they never reach
// downstream consumers and can't waste a primary-files slot or
// inflate the pre-read pool. A debug log lists each dropped path
// so operators can audit LLM-side path quality.
//
// Fail-open: when graph is nil OR FileIndex is empty (e.g. test
// harness without a real repo), all hints pass through unchanged.
//
// Cross-language: works for every language codrax's repomap
// supports — graph.FileIndex is populated identically across all
// scanners (Go / Python / Java / Rust / C++ / Ruby / Swift / etc.).
func filterValidRequiredFileHints(hints []types.RequiredFileHint, graph *repomap.Graph, ctxOpt ...*types.AgentContext) []types.RequiredFileHint {
	if len(hints) == 0 {
		return nil
	}
	var ctx *types.AgentContext
	if len(ctxOpt) > 0 {
		ctx = ctxOpt[0]
	}
	if graph == nil || len(graph.FileIndex) == 0 {
		// Fail-open — graph not available, can't validate.
		return hints
	}
	out := make([]types.RequiredFileHint, 0, len(hints))
	var dropped []string
	for _, h := range hints {
		canon := canonicalExplorerAgentPath(ctx, h.Path)
		if canon == "" {
			dropped = append(dropped, h.Path)
			continue
		}
		if _, ok := graph.FileIndex[canon]; !ok {
			if !explorerAgentPathExists(ctx, canon) {
				dropped = append(dropped, canon)
				continue
			}
		}
		h.Path = canon
		out = append(out, h)
	}
	if len(dropped) > 0 {
		logging.Debug("[explorer] required_file_hints: dropped %d path(s) not in graph: %v", len(dropped), dropped)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func canonicalExplorerAgentPath(ctx *types.AgentContext, path string) string {
	if ctx != nil {
		if canon := ground.CanonicalAgentPath(ctx, path); canon != "" {
			return canonicalExplorerPath(canon)
		}
	}
	return canonicalExplorerPath(path)
}

func explorerAgentPathExists(ctx *types.AgentContext, path string) bool {
	if ctx == nil || ctx.RepoRoot == "" || path == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(ctx.RepoRoot, path))
	return err == nil && !info.IsDir()
}

// preReadEligibleHintFiles returns the canonical paths from
// e.requiredFileHints whose confidence ≥ requiredFileHintSoftConfidence
// (default 0.5). Result is in declaration order from the analyzer
// emit. Used by callers of preReadRequiredFiles to fold the
// analyzer's recommended set into the pre-read pool. High-confidence
// hints (≥0.8) appear first, then soft hints, then the caller's
// existing files (so the analyzer's strongest recommendations get
// the limited maxFiles slots).
func (e *explorerEvaluator) preReadEligibleHintFiles() []string {
	if len(e.requiredFileHints) == 0 {
		return nil
	}
	type tier struct {
		path string
		high bool
	}
	tiers := make([]tier, 0, len(e.requiredFileHints))
	for _, h := range e.requiredFileHints {
		if h.Confidence < requiredFileHintSoftConfidence {
			continue
		}
		canon := canonicalExplorerPath(h.Path)
		if canon == "" {
			continue
		}
		tiers = append(tiers, tier{path: canon, high: h.Confidence >= requiredFileHintHighConfidence})
	}
	if len(tiers) == 0 {
		return nil
	}
	out := make([]string, 0, len(tiers))
	seen := make(map[string]bool, len(tiers))
	// High-confidence first.
	for _, t := range tiers {
		if !t.high || seen[t.path] {
			continue
		}
		seen[t.path] = true
		out = append(out, t.path)
	}
	// Soft tier next.
	for _, t := range tiers {
		if t.high || seen[t.path] {
			continue
		}
		seen[t.path] = true
		out = append(out, t.path)
	}
	return out
}

// mergeHintFilesIntoPreRead returns the caller's preReadFiles list
// with high+soft confidence analyzer hints prepended, deduplicated.
// Used by buildCapabilityFocusedStartInstruction /
// buildFocusedDepthStartInstruction /
// buildPrimaryEntityDepthStartInstruction /
// buildDeclarativeFocusedStartInstruction /
// buildDeclarativeCandidateStartInstruction so each path's pre-read
// pool benefits from the L3 typed recommendations.
func (e *explorerEvaluator) mergeHintFilesIntoPreRead(callerFiles []string) []string {
	hintFiles := e.preReadEligibleHintFiles()
	if len(hintFiles) == 0 {
		return callerFiles
	}
	seen := make(map[string]bool, len(callerFiles)+len(hintFiles))
	out := make([]string, 0, len(callerFiles)+len(hintFiles))
	for _, f := range hintFiles {
		canon := canonicalExplorerPath(f)
		if canon == "" || seen[canon] {
			continue
		}
		seen[canon] = true
		out = append(out, f)
	}
	for _, f := range callerFiles {
		canon := canonicalExplorerPath(f)
		if canon == "" || seen[canon] {
			continue
		}
		seen[canon] = true
		out = append(out, f)
	}
	return out
}

func exactAnchorFilesFromScores(results []keywordFileScore) []string {
	if len(results) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var files []string
	for _, r := range results {
		if r.ExactEntityRank <= 0 || r.Path == "" || seen[r.Path] {
			continue
		}
		seen[r.Path] = true
		files = append(files, r.Path)
	}
	return files
}

func (e *explorerEvaluator) uniqueExactAnchorFile() (string, bool) {
	if len(e.exactPendingTargets) > 0 {
		return "", false
	}
	if len(e.exactAnchorFiles) != 1 {
		return "", false
	}
	return e.exactAnchorFiles[0], true
}

func (e *explorerEvaluator) primaryEntityFocusRelevant() bool {
	if _, ok := e.uniqueExactAnchorFile(); ok {
		return false
	}
	if len(e.declarativeAnchorFiles) > 0 || len(e.declarativeCandidateFiles) > 0 || e.isEnumerationQuery {
		return false
	}
	_, ok := e.uniquePrimaryEntityFile()
	return ok
}

// declarativeFocusRelevant decides whether the explorer should
// route through the declarative-registration depth path. The path
// pre-reads up to 2 declarative-shaped files and tightens the
// frontier to registry / route / manifest neighbors.
//
// 2026-05-10 L1 tightening: previously, ANY enumeration question
// with isEnumeration=true triggered this path. The s1a forensic
// showed function-body enumerations (e.g. "list the 9 internal
// checks of gate.Run") were mis-routed onto registry files
// (bug_class_registry.go / violation_registry.go / analysis_ir.go),
// drowning the real subject file in declarative-shape noise.
//
// The new `primaryEntitiesRegistrationShape` parameter — computed
// by primaryEntitiesLookLikeRegistration and cached on the
// evaluator — gates the enumeration branch on whether at least one
// AnalyzerHints.PrimaryEntity structurally resolves to a file
// classified by `declarative.Classifier.ClassifyPath` as a
// registration-shaped surface (Registry / Routes / Manifest /
// Defaults / Wire / Topology / Schema).
//
// Fail-open: when the IR has no PrimaryEntities OR the graph is
// unavailable, primaryEntitiesLookLikeRegistration returns true,
// preserving the legacy trigger so cases without analyzer hints
// don't lose the path. AxisRegister is a strong explicit signal
// from the analyzer; that branch bypasses the structural gate.
//
// Cross-language: the helper relies on forEachMatchingDef (the
// multi-language qualified-name resolver from B fix), so all 12+
// languages codrax's repomap supports inherit this gate
// automatically.
func declarativeFocusRelevant(
	questionKind string,
	isEnumeration bool,
	axis types.PredicateAxis,
	primaryEntitiesRegistrationShape bool,
) bool {
	switch strings.ToLower(strings.TrimSpace(questionKind)) {
	case "registration", "config_mapping":
		return true
	case "enumeration":
		if !isEnumeration && axis != types.AxisRegister {
			return false
		}
		if axis == types.AxisRegister {
			// Explicit register axis from analyzer — strong signal,
			// keep the legacy bypass.
			return true
		}
		return primaryEntitiesRegistrationShape
	}
	return isEnumeration && axis == types.AxisRegister
}

// primaryEntitiesLookLikeRegistration returns true when at least
// one entity in ir.RequestModel.AnalyzerHints.PrimaryEntities structurally
// resolves (via the multi-language resolver in forEachMatchingDef)
// to a file the declarative classifier marks as registration-shaped.
//
// "Registration-shaped" = any kind in `allowedKinds`, which is
// computed by declarativeAllowedKinds(questionKind, axis) — a
// function-of-the-question whitelist that already drives the
// downstream anchor-file selection. We reuse that whitelist so
// L1's gate fires on the same shape set the rest of the path
// consumes (no shape drift).
//
// Fail-open returns true when:
//   - ir or graph is nil (no signal to gate on)
//   - PrimaryEntities is empty (legacy code didn't have this signal)
//
// Returns false when PrimaryEntities is populated but every
// resolved file's ClassifyPath kind falls outside allowedKinds.
//
// The check short-circuits on first match for performance.
func primaryEntitiesLookLikeRegistration(
	ir *types.AnalysisIR,
	graph *repomap.Graph,
	allowedKinds map[declarative.Kind]bool,
) bool {
	if ir == nil || graph == nil {
		return true
	}
	if len(ir.RequestModel.AnalyzerHints.PrimaryEntities) == 0 {
		return true
	}
	entities := make(map[string]string, len(ir.RequestModel.AnalyzerHints.PrimaryEntities))
	for _, e := range ir.RequestModel.AnalyzerHints.PrimaryEntities {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		entities[strings.ToLower(e)] = e
	}
	if len(entities) == 0 {
		return true
	}
	found := false
	forEachMatchingDef(entities, graph, func(_, _, _ string, d *repomap.Symbol) bool {
		if d == nil || d.File == "" {
			return true
		}
		kind, _ := declarativeClassifier.ClassifyPath(d.File)
		if kind != declarative.KindNone && allowedKinds[kind] {
			found = true
			return false
		}
		return true
	})
	return found
}

func declarativeAllowedKinds(questionKind string, axis types.PredicateAxis) map[declarative.Kind]bool {
	allowed := map[declarative.Kind]bool{
		declarative.KindRegistry: true,
		declarative.KindDefaults: true,
		declarative.KindWire:     true,
		declarative.KindTopology: true,
	}
	switch strings.ToLower(strings.TrimSpace(questionKind)) {
	case "config_mapping":
		allowed[declarative.KindSchema] = true
		allowed[declarative.KindManifest] = true
	case "registration", "enumeration":
		allowed[declarative.KindRoutes] = true
		if axis == types.AxisConfigure {
			allowed[declarative.KindSchema] = true
		}
	}
	return allowed
}

func declarativeAnchorFilesFromScores(results []keywordFileScore, questionKind string, axis types.PredicateAxis, isEnumeration bool, primaryRegistrationShape bool) []string {
	if len(results) == 0 || !declarativeFocusRelevant(questionKind, isEnumeration, axis, primaryRegistrationShape) {
		return nil
	}
	paths := make([]string, 0, len(results))
	for _, result := range results {
		paths = append(paths, result.Path)
	}
	return declarativeAnchorFilesFromPaths(paths, questionKind, axis, isEnumeration, primaryRegistrationShape)
}

func declarativeAnchorFilesFromPaths(paths []string, questionKind string, axis types.PredicateAxis, isEnumeration bool, primaryRegistrationShape bool) []string {
	if len(paths) == 0 || !declarativeFocusRelevant(questionKind, isEnumeration, axis, primaryRegistrationShape) {
		return nil
	}
	allowed := declarativeAllowedKinds(questionKind, axis)
	collect := func(anyDeclarative, allowSecondary bool) []string {
		seen := make(map[string]bool)
		var out []string
		for _, rawPath := range paths {
			path := canonicalExplorerPath(rawPath)
			if path == "" || isNoisePath(path) || seen[path] {
				continue
			}
			if !allowSecondary && declarativeSecondarySurfacePath(path) {
				continue
			}
			kind, _ := declarativeClassifier.ClassifyPath(path)
			if kind == declarative.KindNone {
				continue
			}
			if !anyDeclarative && !allowed[kind] {
				continue
			}
			seen[path] = true
			out = append(out, path)
			if len(out) >= 3 {
				break
			}
		}
		return out
	}
	if anchors := collect(false, false); len(anchors) > 0 {
		return anchors
	}
	if anchors := collect(true, false); len(anchors) > 0 {
		return anchors
	}
	if anchors := collect(false, true); len(anchors) > 0 {
		return anchors
	}
	if anchors := collect(true, true); len(anchors) > 0 {
		return anchors
	}
	return nil
}

func (e *explorerEvaluator) answerSurfacePlan() *types.AnswerSurfacePlan {
	if e == nil || e.analysisIR == nil {
		return nil
	}
	return types.BuildAnswerSurfacePlan(
		e.analysisIR,
		e.mutable,
		e.logTriage,
		e.flowFindings,
		nil,
		e.structuredEvidence,
	)
}

func structuralCandidateFilesFromPaths(paths []string, questionKind string, axis types.PredicateAxis, isEnumeration bool, primaryRegistrationShape bool) []string {
	if len(paths) == 0 || !declarativeFocusRelevant(questionKind, isEnumeration, axis, primaryRegistrationShape) {
		return nil
	}
	collect := func(allowSecondary bool) []string {
		seen := make(map[string]bool)
		out := make([]string, 0, min(4, len(paths)))
		for _, rawPath := range paths {
			path := canonicalExplorerPath(rawPath)
			if path == "" || isNoisePath(path) || seen[path] {
				continue
			}
			if !allowSecondary && declarativeSecondarySurfacePath(path) {
				continue
			}
			seen[path] = true
			out = append(out, path)
			if len(out) >= 4 {
				break
			}
		}
		return out
	}
	if out := collect(false); len(out) > 0 {
		return out
	}
	return collect(true)
}

func declarativeSecondarySurfacePath(path string) bool {
	return types.LooksLikeAuxiliaryEvidencePath(path) || types.LooksLikeWrappedConfigFilePath(path)
}

func canonicalExplorerPath(path string) string {
	// Unconditional backslash → slash normalization: filepath.ToSlash
	// is a no-op on Linux (separator is already '/'), so a Windows-
	// shaped banner like `[.\internal\foo.go: showing lines …]` would
	// otherwise carry through with backslashes intact and miss every
	// downstream slash-keyed map. Keep ToSlash too for Windows so the
	// platform-native conversion still applies in addition.
	path = strings.ReplaceAll(filepath.ToSlash(strings.TrimSpace(path)), "\\", "/")
	return strings.TrimPrefix(path, "./")
}

func readSetContains(readSet map[string]bool, path string) bool {
	if len(readSet) == 0 {
		return false
	}
	path = canonicalExplorerPath(path)
	if path == "" {
		return false
	}
	if readSet[path] {
		return true
	}
	matches := 0
	for readPath := range readSet {
		readPath = canonicalExplorerPath(readPath)
		if readPath == "" {
			continue
		}
		if explorerRelativeAliasMatch(readPath, path) {
			matches++
			if matches > 1 {
				return false
			}
		}
	}
	return matches == 1
}

func explorerRelativeAliasMatch(a, b string) bool {
	a = canonicalExplorerPath(a)
	b = canonicalExplorerPath(b)
	if a == "" || b == "" || explorerLooksAbsolutePath(a) || explorerLooksAbsolutePath(b) {
		return false
	}
	return strings.HasSuffix(a, "/"+b) || strings.HasSuffix(b, "/"+a)
}

func explorerLooksAbsolutePath(path string) bool {
	if strings.HasPrefix(path, "/") {
		return true
	}
	return len(path) >= 3 && path[1] == ':' && path[2] == '/'
}

func sameRepoDir(a, b string) bool {
	a = canonicalExplorerPath(a)
	b = canonicalExplorerPath(b)
	if a == "" || b == "" {
		return false
	}
	return filepath.Dir(a) == filepath.Dir(b)
}

func sameRepoDirAny(anchors []string, candidate string) bool {
	candidate = canonicalExplorerPath(candidate)
	if candidate == "" {
		return false
	}
	for _, anchor := range anchors {
		if sameRepoDir(anchor, candidate) {
			return true
		}
	}
	return false
}

func (e *explorerEvaluator) focusedAnchorNeighborhood() map[string]bool {
	anchor, ok := e.uniqueExactAnchorFile()
	if !ok {
		return nil
	}
	files := make(map[string]bool)
	add := func(path string) {
		path = canonicalExplorerPath(path)
		if path == "" || isNoisePath(path) {
			return
		}
		files[path] = true
	}
	add(anchor)
	if e.searchResult == nil || e.searchResult.Graph == nil {
		return files
	}
	graph := e.searchResult.Graph
	if fi := graph.FileIndex[anchor]; fi != nil {
		for _, rel := range fi.Relations {
			if rel.Kind != "call" {
				continue
			}
			if rel.ToEP.File != "" {
				add(rel.ToEP.File)
			}
			if target := graph.ResolveCallTarget(fi, rel); target != nil {
				add(target.File)
			}
		}
	}
	return files
}

func (e *explorerEvaluator) primaryEntityNeighborhood() map[string]bool {
	anchor, ok := e.uniquePrimaryEntityFile()
	if !ok {
		return nil
	}
	files := make(map[string]bool)
	add := func(path string) {
		path = canonicalExplorerPath(path)
		if path == "" || isNoisePath(path) {
			return
		}
		files[path] = true
	}
	add(anchor)
	if e.searchResult == nil || e.searchResult.Graph == nil {
		return files
	}
	graph := e.searchResult.Graph
	if fi := graph.FileIndex[anchor]; fi != nil {
		for _, rel := range fi.Relations {
			if rel.Kind != "call" {
				continue
			}
			if rel.ToEP.File != "" {
				add(rel.ToEP.File)
			}
			if target := graph.ResolveCallTarget(fi, rel); target != nil {
				add(target.File)
			}
		}
	}
	return files
}

func (e *explorerEvaluator) focusedAnchorAllowsFile(path string) bool {
	path = canonicalExplorerPath(path)
	if path == "" || isNoisePath(path) {
		return false
	}
	focused := e.focusedAnchorNeighborhood()
	if len(focused) == 0 {
		return true
	}
	if focused[path] {
		return true
	}
	anchor, ok := e.uniqueExactAnchorFile()
	return ok && sameRepoDir(anchor, path)
}

func (e *explorerEvaluator) primaryEntityAllowsFile(path string) bool {
	path = canonicalExplorerPath(path)
	if path == "" || isNoisePath(path) {
		return false
	}
	focused := e.primaryEntityNeighborhood()
	if len(focused) == 0 {
		return false
	}
	if focused[path] {
		return true
	}
	anchor, ok := e.uniquePrimaryEntityFile()
	return ok && sameRepoDir(anchor, path)
}

func (e *explorerEvaluator) declarativeFocusNeighborhood() map[string]bool {
	if _, ok := e.uniqueExactAnchorFile(); ok || len(e.declarativeAnchorFiles) == 0 {
		return nil
	}
	files := make(map[string]bool)
	add := func(path string) {
		path = canonicalExplorerPath(path)
		if path == "" || isNoisePath(path) {
			return
		}
		files[path] = true
	}
	for _, anchor := range e.declarativeAnchorFiles {
		add(anchor)
	}
	if e.searchResult == nil || e.searchResult.Graph == nil {
		return files
	}
	graph := e.searchResult.Graph
	for _, anchor := range e.declarativeAnchorFiles {
		fi := graph.FileIndex[anchor]
		if fi == nil {
			continue
		}
		for _, rel := range fi.Relations {
			if rel.Kind != "call" {
				continue
			}
			if rel.ToEP.File != "" {
				add(rel.ToEP.File)
			}
			if target := graph.ResolveCallTarget(fi, rel); target != nil {
				add(target.File)
			}
		}
	}
	return files
}

func (e *explorerEvaluator) declarativeAnchorAllowsFile(path string) bool {
	path = canonicalExplorerPath(path)
	if path == "" || isNoisePath(path) {
		return false
	}
	anchors := e.declarativeAnchorFiles
	if len(anchors) == 0 {
		return true
	}
	neighborhood := e.declarativeFocusNeighborhood()
	if neighborhood[path] {
		return true
	}
	return sameRepoDirAny(anchors, path)
}

func (e *explorerEvaluator) declarativeCandidateAllowsFile(path string) bool {
	path = canonicalExplorerPath(path)
	if path == "" || isNoisePath(path) {
		return false
	}
	cands := e.declarativeCandidateFiles
	if len(cands) == 0 {
		return true
	}
	for _, cand := range cands {
		if path == canonicalExplorerPath(cand) {
			return true
		}
	}
	return sameRepoDirAny(cands, path)
}

func (e *explorerEvaluator) activeFocusAllowsFile(path string) bool {
	if _, ok := e.uniqueExactAnchorFile(); ok {
		return e.focusedAnchorAllowsFile(path)
	}
	if len(e.declarativeAnchorFiles) > 0 {
		return e.declarativeAnchorAllowsFile(path)
	}
	if len(e.declarativeCandidateFiles) > 0 {
		return e.declarativeCandidateAllowsFile(path)
	}
	if e.primaryEntityFocusRelevant() {
		return e.primaryEntityAllowsFile(path)
	}
	return true
}

// partitionRequiredFilesByLogTriage splits files into two ordered
// slices:
//
//   - logFiles: entries that came from the log-triage bundle's
//     ResolvedFiles. These are authoritative: panic/exception frames
//     the validator already os.Stat-verified against the repo.
//   - rankerFiles: the remaining entries, contributed by
//     rankAnalyzerRequiredFiles (exact-anchor tier + QueryScore
//     fallthrough).
//
// Used by the three "Analyzer's Required Files" prompt blocks (F2.2)
// to render the two groups under distinct headers with distinct
// framing, so the LLM treats log-frame files as must-read anchors
// and ranker files as opt-in cross-references. Shared logic prevents
// the three blocks from drifting apart. When no log-triage bundle
// exists OR it has no ResolvedFiles, logFiles is nil and rankerFiles
// is a shallow copy of files (historical rendering preserved).
func (e *explorerEvaluator) partitionRequiredFilesByLogTriage(files []string) (logFiles, rankerFiles []string) {
	if e.logTriage == nil || len(e.logTriage.ResolvedFiles) == 0 {
		return nil, append([]string(nil), files...)
	}
	set := make(map[string]bool, len(e.logTriage.ResolvedFiles))
	for _, f := range e.logTriage.ResolvedFiles {
		set[f] = true
	}
	for _, f := range files {
		if set[f] {
			logFiles = append(logFiles, f)
		} else {
			rankerFiles = append(rankerFiles, f)
		}
	}
	return
}

// authoritativeFailureCovered reports whether the attached failure
// trace (log_triage and/or perf_triage) carries authoritative frames
// AND every one of the union of its ResolvedFiles has entered readSet.
// "Authoritative" here means frame resolution succeeded — File+Line are
// repo-grounded locators — independent of which signal name the LLM
// labelled the trace with. A panic, an OOM, a timeout, a main-thread
// stall, a jank span, or an assertion violation all carry the same
// kind of authoritative locator once their frames pass validation.
//
// When this returns true the Check 6 ranker-coverage pushback is
// skipped: the authoritative file set is already covered, so pushing
// for more top-K reads would only drag the LLM into ranker-noise
// siblings (method names like ParseOutput that match many unrelated
// files). Mirrors the bundle-authoritative check in
// analyzerRequiredFiles so both sites speak the same contract.
func (e *explorerEvaluator) authoritativeFailureCovered(allResults []types.ToolResult) bool {
	files := e.authoritativeFailureResolvedFiles()
	if len(files) == 0 {
		return false
	}
	if !e.hasAuthoritativeFailureFrames() {
		return false
	}
	_, readSet, _ := extractFileCoverage(allResults, e.repoRoot)
	for _, f := range files {
		canon := canonicalExplorerPath(f)
		if canon == "" {
			canon = f
		}
		if !readSetContains(readSet, canon) {
			return false
		}
	}
	return true
}

// authoritativeFailureResolvedFiles returns the union of LogBundle and
// PerfBundle ResolvedFiles. Both pre-stages run their own validators
// so each ResolvedFiles list is already repo-grounded. Order is
// log-first then perf-only additions, deduplicated by canonical path.
func (e *explorerEvaluator) authoritativeFailureResolvedFiles() []string {
	if e == nil {
		return nil
	}
	seen := make(map[string]bool)
	out := make([]string, 0, 8)
	add := func(raw string) {
		canon := canonicalExplorerPath(raw)
		if canon == "" {
			canon = raw
		}
		if canon == "" || seen[canon] {
			return
		}
		seen[canon] = true
		out = append(out, canon)
	}
	if e.logTriage != nil {
		for _, f := range e.logTriage.ResolvedFiles {
			add(f)
		}
	}
	if e.perfTrace != nil {
		for _, f := range e.perfTrace.ResolvedFiles {
			add(f)
		}
	}
	return out
}

func (e *explorerEvaluator) filterRequiredFiles(files []string, ctxOpt ...*types.AgentContext) []string {
	if len(files) == 0 {
		return nil
	}
	var ctx *types.AgentContext
	if len(ctxOpt) > 0 {
		ctx = ctxOpt[0]
	}
	seen := make(map[string]bool, len(files))
	out := make([]string, 0, len(files))
	for _, path := range files {
		canon := canonicalExplorerAgentPath(ctx, path)
		if canon == "" || isNoisePath(canon) || seen[canon] {
			continue
		}
		if !e.activeFocusAllowsFile(canon) {
			continue
		}
		seen[canon] = true
		out = append(out, canon)
	}
	return out
}

func (e *explorerEvaluator) filterKeywordResults(results []keywordFileScore) []keywordFileScore {
	if len(results) == 0 {
		return nil
	}
	out := make([]keywordFileScore, 0, len(results))
	for _, result := range results {
		result.Path = canonicalExplorerPath(result.Path)
		if result.Path == "" || isNoisePath(result.Path) {
			continue
		}
		if !e.activeFocusAllowsFile(result.Path) {
			continue
		}
		out = append(out, result)
	}
	return out
}

func (e *explorerEvaluator) preScannedUnreadCandidates(readSet map[string]bool) []string {
	if len(e.preScannedFiles) == 0 {
		return nil
	}
	readFiles := make([]string, 0, len(readSet))
	for f := range readSet {
		if canon := e.repoRelativeExplorerPath(f); canon != "" {
			readFiles = append(readFiles, canon)
		}
	}
	sort.Strings(readFiles)

	required := make(map[string]bool, len(e.requiredFiles))
	for _, f := range e.requiredFiles {
		if canon := e.repoRelativeExplorerPath(f); canon != "" {
			required[canon] = true
		}
	}
	exact := make(map[string]bool, len(e.exactAnchorFiles))
	for _, f := range e.exactAnchorFiles {
		if canon := e.repoRelativeExplorerPath(f); canon != "" {
			exact[canon] = true
		}
	}

	seen := make(map[string]bool, len(e.preScannedFiles))
	var unread []string
	for _, f := range e.preScannedFiles {
		canon := e.repoRelativeExplorerPath(f)
		if canon == "" || seen[canon] || isNoisePath(canon) || readSetContains(readSet, canon) {
			continue
		}
		if !e.activeFocusAllowsFile(canon) {
			continue
		}
		if len(readFiles) > 0 && !e.preScannedFileHasDepthSignal(canon, readFiles, required, exact) {
			logging.Debug("[explorer] prescan unread: skip distant keyword-only file=%s", canon)
			continue
		}
		seen[canon] = true
		unread = append(unread, canon)
	}
	return unread
}

func (e *explorerEvaluator) repoRelativeExplorerPath(path string) string {
	path = canonicalExplorerPath(path)
	if path == "" {
		return ""
	}
	if e.repoRoot != "" {
		path = ground.CanonicalRepoRelative(path, e.repoRoot)
	}
	return canonicalExplorerPath(path)
}

func (e *explorerEvaluator) preScannedFileHasDepthSignal(file string, readFiles []string, required, exact map[string]bool) bool {
	return e.fileHasDepthSignal(file, readFiles, required, exact, true)
}

func (e *explorerEvaluator) rankerFileHasDepthSignal(file string, readFiles []string, required, exact map[string]bool) bool {
	return e.fileHasDepthSignal(file, readFiles, required, exact, false)
}

func (e *explorerEvaluator) fileHasDepthSignal(file string, readFiles []string, required, exact map[string]bool, allowSiblingDir bool) bool {
	if required[file] || exact[file] {
		return true
	}
	if e.preScannedFileMatchesEntities(file) {
		return true
	}
	for _, read := range readFiles {
		if read == "" || read == file {
			continue
		}
		if (allowSiblingDir && sameRepoDir(read, file)) || e.graphConnectsFiles(read, file) {
			return true
		}
	}
	return false
}

func (e *explorerEvaluator) preScannedFileMatchesEntities(file string) bool {
	entities := e.ermEntityList()
	if len(entities) == 0 {
		return false
	}
	if entityBoostFactor(file, e.searchGraph(), entities) > 1.0 {
		return true
	}
	base := normalizeEntityHaystack(strings.ToLower(filepath.Base(file)))
	for _, ent := range entities {
		if entityHits(base, ent) {
			return true
		}
	}
	for _, sym := range e.fileSymbols[file] {
		name := sym
		if idx := strings.IndexByte(name, ' '); idx > 0 {
			name = name[:idx]
		}
		name = normalizeEntityHaystack(strings.ToLower(name))
		for _, ent := range entities {
			if entityHits(name, ent) {
				return true
			}
		}
	}
	return false
}

func (e *explorerEvaluator) ermEntityList() []string {
	seen := make(map[string]bool)
	var entities []string
	for _, req := range e.ermRequirements {
		for _, ent := range req.Entities {
			ent = strings.TrimSpace(ent)
			key := strings.ToLower(ent)
			if ent == "" || seen[key] {
				continue
			}
			seen[key] = true
			entities = append(entities, ent)
		}
	}
	return entities
}

func (e *explorerEvaluator) searchGraph() *repomap.Graph {
	if e.searchResult == nil {
		return nil
	}
	return e.searchResult.Graph
}

func (e *explorerEvaluator) graphConnectsFiles(a, b string) bool {
	graph := e.searchGraph()
	if graph == nil {
		return false
	}
	a = e.repoRelativeExplorerPath(a)
	b = e.repoRelativeExplorerPath(b)
	if a == "" || b == "" {
		return false
	}
	for _, dep := range graph.FilesImportedBy(a) {
		if e.repoRelativeExplorerPath(dep) == b {
			return true
		}
	}
	for _, importer := range graph.FilesImporting(a) {
		if e.repoRelativeExplorerPath(importer) == b {
			return true
		}
	}
	return fileInfoReferencesFile(graph.FileIndex[a], b) || fileInfoReferencesFile(graph.FileIndex[b], a)
}

func fileInfoReferencesFile(fi *repomap.FileInfo, target string) bool {
	if fi == nil || target == "" {
		return false
	}
	for _, rel := range fi.Relations {
		if relationEndpointFile(rel.ToEP.File) == target ||
			relationEndpointFile(rel.File) == target ||
			relationEndpointFile(rel.To) == target {
			return true
		}
	}
	return false
}

func relationEndpointFile(raw string) string {
	raw = canonicalExplorerPath(raw)
	if raw == "" {
		return ""
	}
	if idx := strings.IndexByte(raw, ':'); idx > 0 {
		raw = raw[:idx]
	}
	return raw
}

func (e *explorerEvaluator) shouldStartFocusedDepth(questionKind string) bool {
	if _, ok := e.uniqueExactAnchorFile(); !ok {
		return false
	}
	if e.isEnumerationQuery {
		return false
	}
	return !strings.EqualFold(strings.TrimSpace(questionKind), "enumeration")
}

func (e *explorerEvaluator) shouldStartPrimaryEntityDepth(questionKind string) bool {
	if _, ok := e.uniqueExactAnchorFile(); ok {
		return false
	}
	if len(e.declarativeAnchorFiles) > 0 || len(e.declarativeCandidateFiles) > 0 {
		return false
	}
	if _, ok := e.uniquePrimaryEntityFile(); !ok {
		return false
	}
	if e.isEnumerationQuery {
		return false
	}
	return !strings.EqualFold(strings.TrimSpace(questionKind), "enumeration")
}

func (e *explorerEvaluator) shouldStartDeclarativeDepth(questionKind string) bool {
	if _, ok := e.uniqueExactAnchorFile(); ok {
		return false
	}
	if len(e.declarativeAnchorFiles) == 0 {
		return false
	}
	// Schema-v4 confidence guard: tightenDeclarativeFrontier
	// aggressively narrows the read set to declarative-anchor files
	// keyed off the LLM's question_kind. When the LLM is unsure about
	// the kind (KindConfidence < kindConfidenceFloorForNarrowing),
	// declining to narrow keeps the broader read set so a
	// misclassification cannot drop the answer-bearing files. Zero
	// confidence (LLM declined to rate) is treated as low — guard
	// refuses to narrow.
	if e.kindConfidence > 0 && e.kindConfidence < kindConfidenceFloorForNarrowing {
		return false
	}
	return declarativeFocusRelevant(questionKind, e.isEnumerationQuery, e.predicateAxis, e.primaryEntitiesRegistrationShape)
}

func (e *explorerEvaluator) shouldStartDeclarativeCandidateDepth(questionKind string) bool {
	if _, ok := e.uniqueExactAnchorFile(); ok {
		return false
	}
	if len(e.declarativeAnchorFiles) > 0 || len(e.declarativeCandidateFiles) == 0 {
		return false
	}
	if e.kindConfidence > 0 && e.kindConfidence < kindConfidenceFloorForNarrowing {
		return false
	}
	return declarativeFocusRelevant(questionKind, e.isEnumerationQuery, e.predicateAxis, e.primaryEntitiesRegistrationShape)
}

// kindConfidenceFloorForNarrowing is the minimum LLM-emitted
// KindConfidence at which the explorer is allowed to apply
// aggressive narrowing rules (tightenDeclarativeFrontier in
// particular). Below this floor, the explorer keeps the broader read
// set so a hesitant classification cannot drop answer-bearing files.
//
// 0.7 is chosen so the LLM has to be more than "leaning" — it has to
// be confident — before downstream behaviour collapses the search
// surface. The pre-v4 code had no guard at all; this floor is the
// safety net for the prose-cue tables we deleted.
const kindConfidenceFloorForNarrowing = 0.7

func (e *explorerEvaluator) tightenFocusedFrontier() {
	anchor, ok := e.uniqueExactAnchorFile()
	if !ok {
		return
	}
	limit := tool.CurrentAnalysisLimits().Phase1UnreadTopK
	if limit <= 0 {
		limit = 3
	}
	if limit > 3 {
		limit = 3
	}
	seen := make(map[string]bool)
	var narrowed []string
	add := func(path string) {
		path = canonicalExplorerPath(path)
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		narrowed = append(narrowed, path)
	}
	add(anchor)
	var focusedNeighbors []string
	for file := range e.focusedAnchorNeighborhood() {
		file = canonicalExplorerPath(file)
		if file == "" || file == anchor {
			continue
		}
		focusedNeighbors = append(focusedNeighbors, file)
	}
	sort.Strings(focusedNeighbors)
	for _, file := range focusedNeighbors {
		if len(narrowed) >= limit {
			break
		}
		add(file)
	}
	for _, f := range e.requiredFiles {
		if len(narrowed) >= limit {
			break
		}
		add(f)
	}
	for _, f := range e.preScannedFiles {
		if len(narrowed) >= limit {
			break
		}
		add(f)
	}
	if len(narrowed) > 0 {
		e.preScannedFiles = narrowed
	}
}

func (e *explorerEvaluator) tightenPrimaryEntityFrontier() {
	anchor, ok := e.uniquePrimaryEntityFile()
	if !ok {
		return
	}
	limit := tool.CurrentAnalysisLimits().Phase1UnreadTopK
	if limit <= 0 {
		limit = 3
	}
	if limit > 3 {
		limit = 3
	}
	seen := make(map[string]bool)
	var narrowed []string
	add := func(path string) {
		path = canonicalExplorerPath(path)
		if path == "" || seen[path] || !e.primaryEntityAllowsFile(path) {
			return
		}
		seen[path] = true
		narrowed = append(narrowed, path)
	}
	add(anchor)
	var neighbors []string
	for file := range e.primaryEntityNeighborhood() {
		file = canonicalExplorerPath(file)
		if file == "" || file == anchor {
			continue
		}
		neighbors = append(neighbors, file)
	}
	sort.Strings(neighbors)
	for _, file := range neighbors {
		if len(narrowed) >= limit {
			break
		}
		add(file)
	}
	for _, f := range e.requiredFiles {
		if len(narrowed) >= limit {
			break
		}
		add(f)
	}
	for _, f := range e.preScannedFiles {
		if len(narrowed) >= limit {
			break
		}
		add(f)
	}
	if len(narrowed) > 0 {
		e.preScannedFiles = narrowed
	}
}

func (e *explorerEvaluator) tightenDeclarativeFrontier() {
	if len(e.declarativeAnchorFiles) == 0 {
		return
	}
	limit := tool.CurrentAnalysisLimits().Phase1UnreadTopK
	if limit <= 0 {
		limit = 3
	}
	if limit < len(e.declarativeAnchorFiles) {
		limit = len(e.declarativeAnchorFiles)
	}
	if limit > 4 {
		limit = 4
	}
	seen := make(map[string]bool)
	var narrowed []string
	add := func(path string) {
		path = canonicalExplorerPath(path)
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		narrowed = append(narrowed, path)
	}
	for _, anchor := range e.declarativeAnchorFiles {
		add(anchor)
	}
	var neighbors []string
	for file := range e.declarativeFocusNeighborhood() {
		file = canonicalExplorerPath(file)
		if file == "" || seen[file] {
			continue
		}
		neighbors = append(neighbors, file)
	}
	sort.Strings(neighbors)
	for _, file := range neighbors {
		if len(narrowed) >= limit {
			break
		}
		add(file)
	}
	for _, f := range e.requiredFiles {
		if len(narrowed) >= limit {
			break
		}
		add(f)
	}
	for _, f := range e.preScannedFiles {
		if len(narrowed) >= limit {
			break
		}
		add(f)
	}
	if len(narrowed) > 0 {
		e.preScannedFiles = narrowed
	}
}

func (e *explorerEvaluator) tightenDeclarativeCandidateFrontier() {
	if len(e.declarativeCandidateFiles) == 0 {
		return
	}
	limit := tool.CurrentAnalysisLimits().Phase1UnreadTopK
	if limit <= 0 {
		limit = 3
	}
	if limit < len(e.declarativeCandidateFiles) {
		limit = len(e.declarativeCandidateFiles)
	}
	if limit > 4 {
		limit = 4
	}
	seen := make(map[string]bool)
	var narrowed []string
	add := func(path string) {
		path = canonicalExplorerPath(path)
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		narrowed = append(narrowed, path)
	}
	for _, file := range e.declarativeCandidateFiles {
		add(file)
	}
	for _, file := range e.requiredFiles {
		if len(narrowed) >= limit {
			break
		}
		add(file)
	}
	for _, file := range e.preScannedFiles {
		if len(narrowed) >= limit {
			break
		}
		add(file)
	}
	if len(narrowed) > 0 {
		e.preScannedFiles = narrowed
	}
}

func (e *explorerEvaluator) buildCapabilityFocusedStartInstruction(ctx *types.AgentContext, analyzerKeywords []string, q *stageToolCapabilityQuery) string {
	if q == nil {
		return ""
	}
	files := capabilityFocusedFiles(q, e.requiredFiles)
	if len(files) == 0 {
		return ""
	}
	e.requiredFiles = files
	var b strings.Builder
	b.WriteString("## Capability Surface Start\n\n")
	b.WriteString("This question is about the stage/tool capability surface. Skip broad repo discovery and inspect the canonical authority files first.\n\n")
	b.WriteString(renderCapabilityAuthoritySection(q, "Capability Authority"))
	b.WriteString("Read these files first, in order:\n")
	for _, file := range files {
		b.WriteString("- `" + file + "`\n")
	}
	b.WriteString("\nWorkflow:\n")
	b.WriteString("- Establish the stage -> agent -> skill binding first.\n")
	b.WriteString("- Confirm whether the named skill's `ToolSuggestions` includes the named tool.\n")
	b.WriteString("- Confirm that `buildToolSchemas` exposes only the skill allowlist to the LLM.\n")
	b.WriteString("- Treat helper subsets and validator functions as supporting detail only after the capability surface is settled.\n")
	b.WriteString("- If implementation detail matters, expand only to the specific helper file already named in these authority files.\n\n")
	if ctx != nil && ctx.RepoRoot != "" {
		if preReadInjected := preReadRequiredFilesTracked(ctx, ctx.RepoRoot, e.mergeHintFilesIntoPreRead(files), 3, 220, e.excludeReadAndIrrelevantFromCtx(ctx)); preReadInjected != "" {
			b.WriteString("### Pre-read File Content (saves you a read_file call)\n\n")
			b.WriteString(preReadInjected)
		}
	}
	if len(analyzerKeywords) > 0 {
		display := analyzerKeywords
		if len(display) > 12 {
			display = display[:12]
		}
		b.WriteString("### Search Terms\n\n")
		b.WriteString("Use these only within the authority files or the helper they directly cite before widening scope:\n`")
		b.WriteString(strings.Join(display, "`, `"))
		b.WriteString("`\n\n")
	}
	b.WriteString("Evidence format:\n")
	b.WriteString("- `[DIRECT] symbol line N: <what this authority file establishes>`\n")
	b.WriteString("- `[CONDITIONAL] symbol line N: <what narrower helper subset or validator does>`\n")
	b.WriteString("- `[ABSENT] <what is not exposed on the stage capability surface>`\n\n")
	b.WriteString("Read the authority files now and emit evidence before widening.\n")
	return b.String()
}

func (e *explorerEvaluator) buildRuntimeObservationOnlyStartInstruction(ctx *types.AgentContext) string {
	var b strings.Builder
	b.WriteString("## Runtime Artifact Only Start\n\n")
	b.WriteString("The attached runtime artifact has no current-repo intersection, and this request does not ask for current-version verification. Treat the Log / Trace Triage section and the attached artifact bytes as the evidence pool.\n\n")
	b.WriteString("Workflow:\n")
	b.WriteString("- Do not run repo breadth search (`repo_map`, `grep`, `list_files`) and do not read current-repo files just because artifact labels resemble repo symbols.\n")
	b.WriteString("- Explain the artifact's own observed frames, spans, messages, or cause chain. If those facts are already present in the Log / Trace Triage section, proceed to completion instead of looking for same-named tests or helpers.\n")
	b.WriteString("- Do not call `emit_evidence` for unresolved artifact frames: that tool is for current-checkout source anchors. Preserve artifact facts in `emit_investigation_complete.reason` and, when useful, `aggregate_facts`.\n")
	b.WriteString("- If the artifact is sufficient, call `emit_investigation_complete` with a resolved result and, when needed, `evidence_floor_waiver.reason=\"external_only_log\"` or `\"external_only_trace\"` so the final answer preserves the observation-only boundary.\n\n")
	if ctx != nil && ctx.LogTriage != nil {
		if len(ctx.LogTriage.Errors) > 0 {
			b.WriteString("The structured log triage already extracted runtime error facts; prefer those over repository lookups.\n\n")
		}
	}
	return b.String()
}

func (e *explorerEvaluator) buildFocusedDepthStartInstruction(ctx *types.AgentContext, analyzerKeywords []string) string {
	anchor, ok := e.uniqueExactAnchorFile()
	if !ok {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Focused Depth Start\n\n")
	b.WriteString("A unique exact entity anchor was found in the repo. ")
	b.WriteString("Skip repo-wide breadth search and start with direct depth investigation.\n\n")
	fmt.Fprintf(&b, "**Read `%s` first.** This file is the exact anchor for the user-named entity.\n\n", anchor)
	b.WriteString("Workflow:\n")
	b.WriteString("- Use `read_file` on the anchor file immediately. If it is large, first use `grep` WITHIN that file (`files_only=false`) to locate the exact symbol body.\n")
	b.WriteString("- Extract direct evidence about the named entity before widening the search.\n")
	b.WriteString("- Expand outward only to files directly referenced by the anchor, the required-file list above, or files named by unresolved symbols from your notes.\n")
	b.WriteString("- Do NOT run broad repo-wide synonym/variant grep until after you have read the anchor file.\n\n")
	if banner := e.buildPrimaryTargetBanner(); banner != "" {
		b.WriteString(banner)
	}
	if banner := e.buildExactResolutionScopeBanner(ctx, analyzerKeywords); banner != "" {
		b.WriteString(banner)
	}
	if guidance := e.renderAuthoritativeFrameStartSection(append([]string{anchor}, e.requiredFiles...)); guidance != "" {
		b.WriteString(guidance)
	}
	requiredForPrompt := e.requiredFiles
	if len(requiredForPrompt) > 0 {
		focusedRequired := make([]string, 0, len(requiredForPrompt))
		for _, f := range requiredForPrompt {
			canon := canonicalExplorerPath(f)
			if canon == "" {
				continue
			}
			keep := sameRepoDirAny(e.declarativeAnchorFiles, canon)
			if !keep {
				for _, anchor := range e.declarativeAnchorFiles {
					if canon == canonicalExplorerPath(anchor) {
						keep = true
						break
					}
				}
			}
			if keep {
				focusedRequired = append(focusedRequired, f)
			}
		}
		requiredForPrompt = focusedRequired
	}
	if len(requiredForPrompt) > 0 {
		// Session-22 F2.2: distinguish log-frame anchors from ranker
		// candidates so the LLM does not conflate "ranker scored this
		// high" with "this is part of the failure call chain".
		logFiles, rankerFiles := e.partitionRequiredFilesByLogTriage(requiredForPrompt)
		writeGroup := func(title, framing string, group []string, cap int) int {
			count := 0
			for _, f := range group {
				f = strings.TrimPrefix(f, "./")
				if f == "" || f == anchor {
					continue
				}
				if count == 0 {
					b.WriteString(title)
					b.WriteString(framing)
				}
				b.WriteString("- `" + f + "`\n")
				count++
				if count >= cap {
					break
				}
			}
			if count > 0 {
				b.WriteString("\n")
			}
			return count
		}
		logWritten := writeGroup(
			"### Frames from the attached log\n\n",
			"These repo files resolved from the attached log's stack frames. Read them before any ranker candidates, and base the answer's call-chain diagram on the Log Triage Call chain block, not on the Auxiliary candidates:\n\n",
			logFiles, 4,
		)
		auxTitle := "### Analyzer's Required Files\n\n"
		auxFraming := "Trace outward from the anchor through these structurally relevant files only as needed:\n\n"
		if logWritten > 0 {
			auxTitle = "### Auxiliary candidates (opt-in cross-references)\n\n"
			auxFraming = "Open these ONLY if the evidence chain visibly crosses file boundaries beyond the log-frame anchors above. Do NOT cite them in the answer's call-chain diagram unless you observed a direct call to or from them:\n\n"
		}
		writeGroup(auxTitle, auxFraming, rankerFiles, 4)
	}
	if len(analyzerKeywords) > 0 {
		display := analyzerKeywords
		if len(display) > 12 {
			display = display[:12]
		}
		b.WriteString("### Search Terms\n\n")
		b.WriteString("Use these terms inside the anchor file and its direct neighbors before widening scope:\n`")
		b.WriteString(strings.Join(display, "`, `"))
		b.WriteString("`\n\n")
	}
	if ctx != nil && ctx.RepoRoot != "" {
		preReadFiles := []string{anchor}
		for _, f := range e.requiredFiles {
			f = strings.TrimPrefix(f, "./")
			if f == "" || f == anchor {
				continue
			}
			preReadFiles = append(preReadFiles, f)
		}
		if preReadInjected := preReadRequiredFilesTracked(ctx, ctx.RepoRoot, e.mergeHintFilesIntoPreRead(preReadFiles), 2, 200, e.excludeReadAndIrrelevantFromCtx(ctx)); preReadInjected != "" {
			b.WriteString("### Pre-read File Content (saves you a read_file call)\n\n")
			b.WriteString(preReadInjected)
		}
	}
	b.WriteString("Evidence format:\n")
	b.WriteString("- `[DIRECT] functionName line N: <what this code establishes>`\n")
	b.WriteString("- `[REGISTRATION] functionName line N: <what is registered, EXACT values>`\n")
	b.WriteString("- `[CONDITIONAL] functionName line N: <what happens> IF <condition>`\n")
	b.WriteString("- `[ABSENT] <what was expected but NOT found>`\n\n")
	b.WriteString("Read the anchor now and collect evidence before expanding.\n")
	return b.String()
}

func (e *explorerEvaluator) buildPrimaryEntityDepthStartInstruction(ctx *types.AgentContext, analyzerKeywords []string) string {
	anchor, ok := e.uniquePrimaryEntityFile()
	if !ok {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Primary Entity Depth Start\n\n")
	b.WriteString("The question's entities resolve to a single primary implementation file in the repo graph. ")
	b.WriteString("Start with that file directly instead of doing a broad second-round sweep.\n\n")
	fmt.Fprintf(&b, "**Read `%s` first.** This is the receiver-aware primary target for the user question.\n\n", anchor)
	b.WriteString("Workflow:\n")
	b.WriteString("- Use `read_file` on the primary file immediately. If it is large, first use `grep` WITHIN that file (`files_only=false`) to locate the exact symbol body.\n")
	b.WriteString("- Extract direct evidence about the named entity before widening the search.\n")
	b.WriteString("- Expand only to direct neighbors of the primary file, analyzer-required files that stay in the same focus area, or files named by unresolved symbols from your notes.\n")
	b.WriteString("- Do NOT fall back to the full keyword-search tail until the primary file and its direct neighbors are exhausted.\n\n")
	if banner := e.buildPrimaryTargetBanner(); banner != "" {
		b.WriteString(banner)
	}
	if banner := e.buildExactResolutionScopeBanner(ctx, analyzerKeywords); banner != "" {
		b.WriteString(banner)
	}
	if guidance := e.renderAuthoritativeFrameStartSection(append([]string{anchor}, e.requiredFiles...)); guidance != "" {
		b.WriteString(guidance)
	}
	requiredForPrompt := e.requiredFiles
	if len(requiredForPrompt) > 0 {
		focusedRequired := make([]string, 0, len(requiredForPrompt))
		for _, f := range requiredForPrompt {
			canon := canonicalExplorerPath(f)
			if canon == "" {
				continue
			}
			keep := sameRepoDirAny(e.declarativeAnchorFiles, canon)
			if !keep {
				for _, anchor := range e.declarativeAnchorFiles {
					if canon == canonicalExplorerPath(anchor) {
						keep = true
						break
					}
				}
			}
			if keep {
				focusedRequired = append(focusedRequired, f)
			}
		}
		requiredForPrompt = focusedRequired
	}
	if len(requiredForPrompt) > 0 {
		logFiles, rankerFiles := e.partitionRequiredFilesByLogTriage(requiredForPrompt)
		writeGroup := func(title, framing string, group []string, cap int) int {
			count := 0
			for _, f := range group {
				f = strings.TrimPrefix(f, "./")
				if f == "" || f == anchor || !e.primaryEntityAllowsFile(f) {
					continue
				}
				if count == 0 {
					b.WriteString(title)
					b.WriteString(framing)
				}
				b.WriteString("- `" + f + "`\n")
				count++
				if count >= cap {
					break
				}
			}
			if count > 0 {
				b.WriteString("\n")
			}
			return count
		}
		logWritten := writeGroup(
			"### Frames from the attached log\n\n",
			"These repo files resolved from the attached log's stack frames. Read them before any ranker candidates, and base the answer's call-chain diagram on the Log Triage Call chain block, not on the Auxiliary candidates:\n\n",
			logFiles, 4,
		)
		auxTitle := "### Analyzer's Required Files\n\n"
		auxFraming := "Trace outward from the primary file through these structurally relevant files only as needed:\n\n"
		if logWritten > 0 {
			auxTitle = "### Auxiliary candidates (opt-in cross-references)\n\n"
			auxFraming = "Open these ONLY if the evidence chain visibly crosses file boundaries beyond the log-frame anchors above. Do NOT cite them in the answer's call-chain diagram unless you observed a direct call to or from them:\n\n"
		}
		writeGroup(auxTitle, auxFraming, rankerFiles, 4)
	}
	if len(analyzerKeywords) > 0 {
		display := analyzerKeywords
		if len(display) > 12 {
			display = display[:12]
		}
		b.WriteString("### Search Terms\n\n")
		b.WriteString("Use these terms inside the primary file and its direct neighbors before widening scope:\n`")
		b.WriteString(strings.Join(display, "`, `"))
		b.WriteString("`\n\n")
	}
	if ctx != nil && ctx.RepoRoot != "" {
		preReadFiles := []string{anchor}
		for _, f := range e.requiredFiles {
			f = strings.TrimPrefix(f, "./")
			if f == "" || f == anchor || !e.primaryEntityAllowsFile(f) {
				continue
			}
			preReadFiles = append(preReadFiles, f)
		}
		if preReadInjected := preReadRequiredFilesTracked(ctx, ctx.RepoRoot, e.mergeHintFilesIntoPreRead(preReadFiles), 2, 200, e.excludeReadAndIrrelevantFromCtx(ctx)); preReadInjected != "" {
			b.WriteString("### Pre-read File Content (saves you a read_file call)\n\n")
			b.WriteString(preReadInjected)
		}
	}
	b.WriteString("Evidence format:\n")
	b.WriteString("- `[DIRECT] functionName line N: <what this code establishes>`\n")
	b.WriteString("- `[REGISTRATION] functionName line N: <what is registered, EXACT values>`\n")
	b.WriteString("- `[CONDITIONAL] functionName line N: <what happens> IF <condition>`\n")
	b.WriteString("- `[ABSENT] <what was expected but NOT found>`\n\n")
	b.WriteString("Read the primary file now and collect evidence before expanding.\n")
	return b.String()
}

func (e *explorerEvaluator) buildDeclarativeFocusedStartInstruction(ctx *types.AgentContext, analyzerKeywords []string) string {
	if len(e.declarativeAnchorFiles) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Declarative Registration Start\n\n")
	b.WriteString("This question likely resolves to declarative registration / defaults / routing surfaces. ")
	b.WriteString("Start by reading those surfaces directly instead of widening to generic implementation files.\n\n")
	b.WriteString("Read these files first:\n")
	for _, anchor := range e.declarativeAnchorFiles {
		b.WriteString("- `" + anchor + "`\n")
	}
	b.WriteString("\nWorkflow:\n")
	b.WriteString("- Enumerate every registration / binding / mapping entry in these files before widening scope.\n")
	b.WriteString("- If an entry registers a builder or factory call, resolve the builder's stable terminal identity (for example Name / Key / Type / Route) before concluding that the item is known.\n")
	b.WriteString("- Expand only to builders, factories, or direct neighbors referenced by these surfaces.\n")
	b.WriteString("- Do NOT broaden to the full keyword-search tail until the declarative surfaces and their direct builder references are exhausted.\n\n")
	if banner := e.buildExactResolutionScopeBanner(ctx, analyzerKeywords); banner != "" {
		b.WriteString(banner)
	}
	if guidance := e.renderAuthoritativeFrameStartSection(append(append([]string(nil), e.declarativeAnchorFiles...), e.requiredFiles...)); guidance != "" {
		b.WriteString(guidance)
	}
	requiredForPrompt := e.requiredFiles
	if len(requiredForPrompt) > 0 {
		focusedRequired := make([]string, 0, len(requiredForPrompt))
		for _, f := range requiredForPrompt {
			canon := canonicalExplorerPath(f)
			if canon == "" {
				continue
			}
			keep := sameRepoDirAny(e.declarativeAnchorFiles, canon)
			if !keep {
				for _, anchor := range e.declarativeAnchorFiles {
					if canon == canonicalExplorerPath(anchor) {
						keep = true
						break
					}
				}
			}
			if keep {
				focusedRequired = append(focusedRequired, f)
			}
		}
		requiredForPrompt = focusedRequired
	}
	if len(requiredForPrompt) > 0 {
		// Session-22 F2.2: same split as buildFocusedDepthStartInstruction
		// — log-frame files get strong framing, ranker files get opt-in
		// framing. Declarative path has no anchor-dedupe so we pass "".
		logFiles, rankerFiles := e.partitionRequiredFilesByLogTriage(requiredForPrompt)
		writeGroup := func(title, framing string, group []string, cap int) int {
			count := 0
			for _, f := range group {
				f = strings.TrimPrefix(f, "./")
				if f == "" {
					continue
				}
				if count == 0 {
					b.WriteString(title)
					b.WriteString(framing)
				}
				b.WriteString("- `" + f + "`\n")
				count++
				if count >= cap {
					break
				}
			}
			if count > 0 {
				b.WriteString("\n")
			}
			return count
		}
		logWritten := writeGroup(
			"### Frames from the attached log\n\n",
			"These repo files resolved from the attached log's stack frames. Read them before any ranker candidates, and base the answer's call-chain diagram on the Log Triage Call chain block, not on the Auxiliary candidates:\n\n",
			logFiles, 4,
		)
		auxTitle := "### Analyzer's Required Files\n\n"
		auxFraming := "Trace outward through these structurally relevant neighbors only as needed:\n\n"
		if logWritten > 0 {
			auxTitle = "### Auxiliary candidates (opt-in cross-references)\n\n"
			auxFraming = "Open these ONLY if the evidence chain visibly crosses file boundaries beyond the log-frame anchors above. Do NOT cite them in the answer's call-chain diagram unless you observed a direct call to or from them:\n\n"
		}
		writeGroup(auxTitle, auxFraming, rankerFiles, 4)
	}
	if len(analyzerKeywords) > 0 {
		display := analyzerKeywords
		if len(display) > 12 {
			display = display[:12]
		}
		b.WriteString("### Search Terms\n\n")
		b.WriteString("Use these inside the declarative surfaces and their direct builder neighbors before widening scope:\n`")
		b.WriteString(strings.Join(display, "`, `"))
		b.WriteString("`\n\n")
	}
	if ctx != nil && ctx.RepoRoot != "" {
		preReadFiles := append([]string(nil), e.declarativeAnchorFiles...)
		for _, f := range e.requiredFiles {
			f = strings.TrimPrefix(f, "./")
			if f == "" {
				continue
			}
			preReadFiles = append(preReadFiles, f)
		}
		if preReadInjected := preReadRequiredFilesTracked(ctx, ctx.RepoRoot, e.mergeHintFilesIntoPreRead(preReadFiles), 2, 220, e.excludeReadAndIrrelevantFromCtx(ctx)); preReadInjected != "" {
			b.WriteString("### Pre-read File Content (saves you a read_file call)\n\n")
			b.WriteString(preReadInjected)
		}
	}
	b.WriteString("Evidence format:\n")
	b.WriteString("- `[DIRECT] symbol line N: <what this declaration establishes>`\n")
	b.WriteString("- `[REGISTRATION] symbol line N: <what is registered or bound, EXACT values>`\n")
	b.WriteString("- `[CONDITIONAL] symbol line N: <what happens> IF <condition>`\n")
	b.WriteString("- `[ABSENT] <what was expected but NOT found>`\n\n")
	b.WriteString("Read the declarative surfaces now and collect evidence before expanding.\n")
	return b.String()
}

func (e *explorerEvaluator) buildDeclarativeCandidateStartInstruction(ctx *types.AgentContext, analyzerKeywords []string) string {
	if len(e.declarativeCandidateFiles) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Structural Candidate Start\n\n")
	b.WriteString("This question likely resolves to declarative registration / defaults / routing / binding surfaces, but no canonical declarative file was derived automatically.\n\n")
	b.WriteString("Choose the most structurally declarative file from these analyzer-ranked candidates first, read it, and emit evidence before widening:\n")
	for _, file := range e.declarativeCandidateFiles {
		b.WriteString("- `" + file + "`\n")
	}
	b.WriteString("\nWorkflow:\n")
	b.WriteString("- Use the file contents, not the path spelling, to decide whether a candidate is the right surface. Prefer files that define schemas, manifests, defaults, registry tables, route tables, or direct binding structs/maps.\n")
	b.WriteString("- Read ONE candidate first and emit evidence from it before opening generic implementation files.\n")
	b.WriteString("- Expand only to direct neighbors that the candidate explicitly references or that are already present in the required-file list above.\n")
	b.WriteString("- If the first candidate turns out not to be a declarative surface, move to the next candidate instead of broadening repo-wide.\n\n")
	if banner := e.buildExactResolutionScopeBanner(ctx, analyzerKeywords); banner != "" {
		b.WriteString(banner)
	}
	if len(analyzerKeywords) > 0 {
		display := analyzerKeywords
		if len(display) > 12 {
			display = display[:12]
		}
		b.WriteString("### Search Terms\n\n")
		b.WriteString("Use these terms inside the chosen candidate and its direct neighbors before widening scope:\n`")
		b.WriteString(strings.Join(display, "`, `"))
		b.WriteString("`\n\n")
	}
	if ctx != nil && ctx.RepoRoot != "" {
		if preReadInjected := preReadRequiredFilesTracked(ctx, ctx.RepoRoot, e.mergeHintFilesIntoPreRead(e.declarativeCandidateFiles), 2, 220, e.excludeReadAndIrrelevantFromCtx(ctx)); preReadInjected != "" {
			b.WriteString("### Pre-read File Content (saves you a read_file call)\n\n")
			b.WriteString(preReadInjected)
		}
	}
	b.WriteString("Evidence format:\n")
	b.WriteString("- `[DIRECT] symbol line N: <what this declaration establishes>`\n")
	b.WriteString("- `[REGISTRATION] symbol line N: <what is registered or bound, EXACT values>`\n")
	b.WriteString("- `[CONDITIONAL] symbol line N: <what happens> IF <condition>`\n")
	b.WriteString("- `[ABSENT] <what was expected but NOT found>`\n\n")
	b.WriteString("Read the first structural candidate now and collect evidence before expanding.\n")
	return b.String()
}

func (e *explorerEvaluator) activeFrontierFileSet(readSet map[string]bool, notesJoined string) map[string]bool {
	files := make(map[string]bool)
	focused := e.focusedAnchorNeighborhood()
	declarativeFocused := e.declarativeFocusNeighborhood()
	primaryFocused := e.primaryEntityNeighborhood()
	add := func(path string) {
		path = canonicalExplorerPath(path)
		if path == "" || isNoisePath(path) {
			return
		}
		files[path] = true
	}
	for file := range readSet {
		if !e.activeFocusAllowsFile(file) {
			continue
		}
		add(file)
	}
	for _, file := range e.exactAnchorFiles {
		add(file)
	}
	for _, file := range e.declarativeAnchorFiles {
		add(file)
	}
	for _, file := range e.declarativeCandidateFiles {
		add(file)
	}
	for file := range focused {
		add(file)
	}
	for file := range declarativeFocused {
		add(file)
	}
	for file := range primaryFocused {
		add(file)
	}
	for _, file := range e.primaryEntityFiles() {
		add(file)
	}
	for _, file := range e.requiredFiles {
		if !e.activeFocusAllowsFile(file) {
			continue
		}
		add(file)
	}
	limit := tool.CurrentAnalysisLimits().Phase1UnreadTopK
	if limit <= 0 {
		limit = 3
	}
	if len(e.exactAnchorFiles) == 1 && limit > 2 {
		limit = 2
	} else if len(e.declarativeAnchorFiles) > 0 && limit > 3 {
		limit = 3
	}
	for i, file := range e.preScannedFiles {
		if i >= limit {
			break
		}
		if !e.activeFocusAllowsFile(file) {
			continue
		}
		add(file)
	}
	if e.searchResult != nil && e.searchResult.Graph != nil && notesJoined != "" {
		for symName, defs := range e.searchResult.Graph.SymbolDefs {
			if len(symName) < 6 || !strings.Contains(notesJoined, symName) {
				continue
			}
			for _, def := range defs {
				if def != nil {
					if !e.activeFocusAllowsFile(def.File) {
						continue
					}
					add(def.File)
				}
			}
		}
	}
	if len(files) == 0 {
		for i, file := range e.preScannedFiles {
			if i >= 2 {
				break
			}
			if !e.activeFocusAllowsFile(file) {
				continue
			}
			add(file)
		}
	}
	if len(files) == 0 {
		for i, file := range e.allScoredFiles {
			if i >= 2 {
				break
			}
			if !e.activeFocusAllowsFile(file) {
				continue
			}
			add(file)
		}
	}
	return files
}

func (e *explorerEvaluator) activeFrontierFiles(readSet map[string]bool, notesJoined string) []string {
	files := e.activeFrontierFileSet(readSet, notesJoined)
	if len(files) == 0 {
		return nil
	}
	out := make([]string, 0, len(files))
	for file := range files {
		out = append(out, file)
	}
	sort.Strings(out)
	return out
}

func (e *explorerEvaluator) coverageScopeFiles(discovered []string, readSet map[string]bool, notesJoined string) []string {
	if _, ok := e.uniqueExactAnchorFile(); ok || len(e.declarativeAnchorFiles) > 0 || e.primaryEntityFocusRelevant() {
		if scope := e.activeFrontierFiles(readSet, notesJoined); len(scope) > 0 {
			return scope
		}
	}
	if scope := e.requiredFilePackageScopeForEnumeration(discovered, readSet); len(scope) > 0 {
		return scope
	}
	if len(discovered) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(discovered))
	out := make([]string, 0, len(discovered))
	for _, file := range discovered {
		file = canonicalExplorerPath(file)
		if file == "" || isNoisePath(file) || seen[file] {
			continue
		}
		seen[file] = true
		out = append(out, file)
	}
	return out
}

func (e *explorerEvaluator) requiredFilePackageScopeForEnumeration(discovered []string, readSet map[string]bool) []string {
	if e == nil || e.analysisIR == nil || len(e.requiredFiles) == 0 {
		return nil
	}
	rm := e.analysisIR.RequestModel
	if !rm.Predicates.IsCategoryEnumeration || rm.Predicates.IsRelationalLookup {
		return nil
	}
	if !types.RequiresExhaustiveEnumerationMemberSetHandoff(rm) && !rm.QuestionStructure().HasAnyObligation() {
		return nil
	}
	required := make([]string, 0, len(e.requiredFiles))
	seenRequired := make(map[string]bool, len(e.requiredFiles))
	for _, file := range e.requiredFiles {
		file = e.repoRelativeExplorerPath(file)
		if file == "" || isNoisePath(file) || !e.activeFocusAllowsFile(file) || seenRequired[file] {
			continue
		}
		seenRequired[file] = true
		required = append(required, file)
	}
	if len(required) == 0 {
		return nil
	}
	requiredDir := ""
	for _, file := range required {
		dir := filepath.Dir(file)
		if dir == "." || dir == "" {
			return nil
		}
		if requiredDir == "" {
			requiredDir = dir
			continue
		}
		if dir != requiredDir {
			return nil
		}
	}
	scopeSet := make(map[string]bool, len(required)+len(discovered)+len(readSet))
	addIfSameDir := func(file string) {
		file = e.repoRelativeExplorerPath(file)
		if file == "" || isNoisePath(file) || !e.activeFocusAllowsFile(file) {
			return
		}
		if filepath.Dir(file) != requiredDir {
			return
		}
		scopeSet[file] = true
	}
	for _, file := range required {
		addIfSameDir(file)
	}
	for _, file := range discovered {
		addIfSameDir(file)
	}
	for file := range readSet {
		addIfSameDir(file)
	}
	if len(required) == 1 && len(scopeSet) <= 1 {
		return nil
	}
	out := make([]string, 0, len(scopeSet))
	for file := range scopeSet {
		out = append(out, file)
	}
	sort.Strings(out)
	return out
}

// ImplementerExpansion is the typed result of expanding interface /
// trait / protocol entity names into their concrete implementers via
// the repo graph. Both consumers of the implementers-of-interface
// graph walk fill from the SAME single traversal:
//
//   - Files: canonical repo-relative paths of every FileInfo that
//     contains an implementing concrete-type Symbol. Used by the
//     explorer's mid-loop scope hint (P2 #4) to replace the keyword
//     ranker's noisy file list for category-enumeration questions.
//
//   - Names: bare Symbol.Name of each concrete implementer
//     (insertion-order preserved, deduped case-insensitively). Used
//     by the analyzer's L0-B-pre entity expansion (2026-05-06) so the
//     enumeration cardinality gate sees the implementer values rather
//     than the wrapper-interface name alone.
//
// Both fields are populated in one graph walk so a future third
// consumer (extract / finalize / etc.) can consume either or both
// without re-traversing.
type ImplementerExpansion struct {
	Files []string
	Names []string
}

// expandImplementersFromGraph is the unified typed-graph primitive
// read for "find concrete implementers of these interface entities".
// Pre-2026-05-06 the explorer (`implementerFilesFromGraph`) and the
// analyzer (`expandEntitiesWithImplementers`) each walked the graph
// independently — same `g.SymbolDefs[entity]` lookup + same
// `fi.Symbols[i].Implements` traversal — diverging only at the
// final field they kept (file path vs symbol name). The unified
// helper does the walk once, returns both, and lets the two
// consumers each take what they need.
//
// Returns a zero-value ImplementerExpansion when graph is nil, no
// entity resolves to an interface declaration, or no implementers
// exist. Both fields preserve insertion order for deterministic
// downstream rendering, and dedupe case-insensitively (Files dedup
// by canonical path; Names dedup by lowercased trimmed token).
//
// Per the precise-signals-for-hard-gates red line: this signal IS
// precise enough to drive a structural decision. Reads only
// graph.SymbolDefs (declaration site) and Symbol.Implements
// (concrete-side membership populated by populateImplementers).
// Zero substring matching, zero ranker scoring; cross-language by
// construction (per-language extractors fill Implements uniformly).
func expandImplementersFromGraph(graph any, entities []string) ImplementerExpansion {
	if graph == nil || len(entities) == 0 {
		return ImplementerExpansion{}
	}
	seenFile := make(map[string]bool)
	seenName := make(map[string]bool)
	var files []string
	var names []string

	// P4-cross-sub-repo (Sc 1, 2026-05-08): when caller passes a
	// *multigraph.MultiGraph, fan out across every active sub-repo.
	// Returned files are path-from-parent (sub-repo prefix already
	// applied by SubRepoRelPath inside the carrier helpers) so cross-
	// sub-repo implementers don't collide on names like "Service".
	// Single-repo IsSingle() posture is byte-identical to the legacy
	// per-graph branch below — the fan-out enumerates one entry.
	if mg, ok := graph.(*multigraph.MultiGraph); ok && mg != nil {
		for _, entity := range entities {
			entity = strings.TrimSpace(entity)
			if entity == "" {
				continue
			}
			hits := mg.ImplementersOf(entity)
			if len(hits) == 0 {
				continue
			}
			for _, hit := range hits {
				sym, _, found := mg.LookupSymbolByID(hit.ID)
				if !found || sym == nil {
					continue
				}
				file := canonicalExplorerPath(multigraph.SubRepoRelPath(hit.Sub, sym.File))
				if file != "" && !seenFile[file] {
					seenFile[file] = true
					files = append(files, file)
				}
				if name := strings.TrimSpace(sym.Name); name != "" {
					key := strings.ToLower(name)
					if !seenName[key] {
						seenName[key] = true
						names = append(names, name)
					}
				}
			}
		}
		return ImplementerExpansion{Files: files, Names: names}
	}

	g, ok := graph.(*repotypes.Graph)
	if !ok || g == nil {
		return ImplementerExpansion{}
	}
	for _, entity := range entities {
		entity = strings.TrimSpace(entity)
		if entity == "" {
			continue
		}
		ids := g.ImplementersOf(entity)
		if len(ids) == 0 {
			continue
		}
		for _, id := range ids {
			sym, ok := g.SymbolByID[id]
			if !ok || sym == nil {
				continue
			}
			if file := canonicalExplorerPath(sym.File); file != "" && !seenFile[file] {
				seenFile[file] = true
				files = append(files, file)
			}
			if name := strings.TrimSpace(sym.Name); name != "" {
				key := strings.ToLower(name)
				if !seenName[key] {
					seenName[key] = true
					names = append(names, name)
				}
			}
		}
	}
	return ImplementerExpansion{Files: files, Names: names}
}

// implementerFilesFromGraph is the explorer-side thin wrapper around
// expandImplementersFromGraph that returns just the canonical file
// paths. Kept as its own entry point so the existing call site at
// explorer.go:6318 (P2 #4 mid-loop scope hint) stays a one-line
// invocation. Behaviour is byte-identical to the pre-2026-05-06
// stand-alone implementation.
func implementerFilesFromGraph(graph any, entities []string) []string {
	return expandImplementersFromGraph(graph, entities).Files
}

func (e *explorerEvaluator) rankerCoverageFiles() []string {
	if len(e.allScoredFiles) == 0 {
		return nil
	}
	if _, ok := e.uniqueExactAnchorFile(); !ok && len(e.declarativeAnchorFiles) == 0 && !e.primaryEntityFocusRelevant() {
		return append([]string(nil), e.allScoredFiles...)
	}
	out := make([]string, 0, len(e.allScoredFiles))
	for _, f := range e.allScoredFiles {
		if e.activeFocusAllowsFile(f) {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return append([]string(nil), e.allScoredFiles...)
	}
	return out
}

func (e *explorerEvaluator) rankerCoverageFilesForReadSet(readSet map[string]bool) []string {
	ranked := e.rankerCoverageFiles()
	if len(ranked) == 0 || len(readSet) == 0 {
		return ranked
	}
	readFiles := make([]string, 0, len(readSet))
	for f := range readSet {
		if canon := e.repoRelativeExplorerPath(f); canon != "" {
			readFiles = append(readFiles, canon)
		}
	}
	sort.Strings(readFiles)
	required := make(map[string]bool, len(e.requiredFiles))
	for _, f := range e.requiredFiles {
		if canon := e.repoRelativeExplorerPath(f); canon != "" {
			required[canon] = true
		}
	}
	exact := make(map[string]bool, len(e.exactAnchorFiles))
	for _, f := range e.exactAnchorFiles {
		if canon := e.repoRelativeExplorerPath(f); canon != "" {
			exact[canon] = true
		}
	}
	if !e.hasDepthSignalBasis(required, exact) || (e.logTriage != nil && len(e.logTriage.ResolvedFiles) > 0) {
		return ranked
	}

	seen := make(map[string]bool, len(ranked))
	out := make([]string, 0, len(ranked))
	for _, f := range ranked {
		canon := e.repoRelativeExplorerPath(f)
		if canon == "" || seen[canon] || isNoisePath(canon) {
			continue
		}
		if readSetContains(readSet, canon) || e.rankerFileHasDepthSignal(canon, readFiles, required, exact) {
			seen[canon] = true
			out = append(out, canon)
		} else {
			logging.Debug("[explorer] ranker coverage: skip keyword-only file=%s after read focus", canon)
		}
	}
	return out
}

func (e *explorerEvaluator) hasDepthSignalBasis(required, exact map[string]bool) bool {
	if len(required) > 0 || len(exact) > 0 || len(e.ermEntityList()) > 0 {
		return true
	}
	graph := e.searchGraph()
	return graph != nil && len(graph.FileIndex) > 0
}

func coverageSnapshot(scope []string, readSet map[string]bool) (readCount int, coverage float64, unread []string) {
	if len(scope) == 0 {
		return 0, 0, nil
	}
	for _, file := range scope {
		if readSetContains(readSet, file) {
			readCount++
			continue
		}
		unread = append(unread, file)
	}
	coverage = float64(readCount) / float64(len(scope))
	return readCount, coverage, unread
}

func filterEvidenceItemsByFileSet(items []types.EvidenceItem, allowed map[string]bool) []types.EvidenceItem {
	if len(items) == 0 || len(allowed) == 0 {
		return items
	}
	out := make([]types.EvidenceItem, 0, len(items))
	for _, item := range items {
		source := canonicalExplorerPath(item.Source)
		if source != "" && !allowed[source] {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (e *explorerEvaluator) concreteValueFocusSymbols(graph *repomap.Graph, filesToScan map[string]bool) map[string]map[string]bool {
	if graph == nil || len(filesToScan) == 0 || len(e.ermRequirements) == 0 {
		return nil
	}
	entities := make(map[string]string)
	for _, req := range e.ermRequirements {
		for _, ent := range req.Entities {
			ent = strings.TrimSpace(ent)
			if ent == "" {
				continue
			}
			entities[strings.ToLower(ent)] = ent
		}
	}
	if len(entities) == 0 {
		return nil
	}
	out := make(map[string]map[string]bool)
	forEachMatchingDef(entities, graph, func(_, _, _ string, d *repomap.Symbol) bool {
		if d == nil {
			return true
		}
		file := strings.TrimPrefix(d.File, "./")
		if file == "" || !filesToScan[file] {
			return true
		}
		kind := strings.ToLower(d.Kind)
		if kind != "function" && kind != "method" {
			return true
		}
		if out[file] == nil {
			out[file] = make(map[string]bool)
		}
		out[file][strings.ToLower(d.Name)] = true
		owner := d.Receiver
		if owner == "" {
			owner = d.Parent
		}
		if owner != "" {
			out[file][strings.ToLower(owner+"."+d.Name)] = true
		}
		return true
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func concreteValueMatchesFocus(sym *repomap.Symbol, focus map[string]bool) bool {
	if len(focus) == 0 || sym == nil {
		return true
	}
	if focus[strings.ToLower(sym.Name)] {
		return true
	}
	owner := sym.Receiver
	if owner == "" {
		owner = sym.Parent
	}
	if owner == "" {
		return false
	}
	if focus[strings.ToLower(owner)] {
		return true
	}
	return focus[strings.ToLower(owner+"."+sym.Name)]
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

func (e *explorerEvaluator) buildExactResolutionScopeBanner(ctx *types.AgentContext, analyzerKeywords []string) string {
	if ctx != nil && ctx.Mutable != nil {
		ctx.Mutable.SetExactContextRequiredFiles(nil)
	}
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	contract := ctx.AnalysisIR.AnswerContract.ExactResolution
	if contract == nil {
		return ""
	}
	if len(contract.Targets) == 0 {
		return ""
	}
	cands := e.collectExactResolutionSymbolCandidates(contract, analyzerKeywords)
	cands = exactResolutionFilterCandidatesToPreferredFiles(cands, e.requiredFiles)
	if ctx.Mutable != nil &&
		contract.AllowAbsence &&
		contract.RelatedContextPolicy != types.ExactContextGroundedOnly {
		var graph *repomap.Graph
		if e.searchResult != nil {
			graph = e.searchResult.Graph
		}
		e.mergeEmittedEvidenceDelta(ctx)
		e.exactContextFiles = refreshedExactResolutionContextFiles(
			contract,
			ctx.AnalysisIR.RequestModel.Scenario,
			graph,
			e.structuredEvidence,
			cands,
			e.requiredFiles,
			e.exactContextFiles,
		)
		ctx.Mutable.SetExactContextRequiredFiles(e.exactContextFiles)
	}
	if contract.RelatedContextPolicy != types.ExactContextSameFamilyGrounded {
		return ""
	}
	if len(cands) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Same-Family Repo Symbols\n\n")
	b.WriteString("If the exact target stays absent, read these same-family symbols before jumping to a different config family. They were ranked from repo_map using the question's current keywords plus the exact-target family terms:\n")
	for _, cand := range cands {
		if cand.Line > 0 {
			fmt.Fprintf(&b, "- `%s` in `%s:%d`\n", cand.Symbol, cand.File, cand.Line)
			continue
		}
		fmt.Fprintf(&b, "- `%s` in `%s`\n", cand.Symbol, cand.File)
	}
	if contract != nil &&
		contract.TargetKind == types.SubjectConfigKey &&
		contract.RelatedContextPolicy == types.ExactContextSameFamilyGrounded {
		b.WriteString("\nFor exact config-key traces, do not let a single broad family root decide the next hop by itself. Prefer config-file layers, config structs/tags, binding/merge code, and override surfaces before generic implementation files that merely mention the same root.\n")
	}
	b.WriteString("\nTreat any result here as related context unless you find explicit alias / parser-mapping proof for the exact target.\n\n")
	return b.String()
}

func (e *explorerEvaluator) collectExactResolutionSymbolCandidates(contract *types.ExactResolutionContract, analyzerKeywords []string) []exactResolutionSymbolCandidate {
	var graph *repomap.Graph
	if e.searchResult != nil {
		graph = e.searchResult.Graph
	}
	return collectExactResolutionSymbolCandidatesFromGraph(graph, contract, analyzerKeywords, e.fileSymbols, e.structuredEvidence)
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
		p = canonicalExplorerPath(p)
		if p != "" {
			primarySet[p] = true
		}
	}
	out := items[:0:0] // new slice, preserve original order
	for _, ev := range items {
		if ev.Source == "" || primarySet[canonicalEvidenceSourcePath(ev.Source)] {
			out = append(out, ev)
		}
	}
	return out
}

func canonicalEvidenceSourcePath(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	if strings.HasPrefix(source, "mechanism_scan:") {
		source = strings.TrimSpace(strings.TrimPrefix(source, "mechanism_scan:"))
	}
	return canonicalExplorerPath(source)
}

// balanceEvidenceAcrossPrimaryFiles rearranges the leading entries of
// a ranked evidence slice so each primary file contributes in
// round-robin order until every primary file is exhausted. Entries
// whose source is not any primary file are appended at the end while
// keeping their relative order.
//
// The motivation is multi-path comparison questions (u3a style:
// "compare A.go vs B.go"): a count-heavy cluster from one file
// (explorer.go at 87 items) would otherwise dominate the first 12
// Primary Evidence slots and starve the other primary file
// (extractor.go at 18 items), producing finalizer answers biased
// toward whichever cluster fell first. Round-robin forces "some X,
// some Y" ordering so every primary file appears near the top of the
// finalizer's view.
//
// No-op when there is < 2 primary files — single-subject questions
// keep their score-based ordering since there is no balance to
// enforce. Stable within each file's group (relative ordering of a
// single file's items preserved). Non-primary items retain their
// relative order after the primary block.
func balanceEvidenceAcrossPrimaryFiles(items []types.EvidenceItem, primary []string) []types.EvidenceItem {
	if len(primary) < 2 || len(items) == 0 {
		return items
	}
	primaryKey := make(map[string]int, len(primary))
	primaryOrder := make([]string, 0, len(primary))
	for _, p := range primary {
		key := canonicalExplorerPath(p)
		if key == "" {
			continue
		}
		if _, seen := primaryKey[key]; seen {
			continue
		}
		primaryKey[key] = len(primaryOrder)
		primaryOrder = append(primaryOrder, key)
	}
	if len(primaryOrder) < 2 {
		return items
	}

	groups := make([][]types.EvidenceItem, len(primaryOrder))
	nonPrimary := make([]types.EvidenceItem, 0, len(items))
	for _, ev := range items {
		key := canonicalEvidenceSourcePath(ev.Source)
		if idx, ok := primaryKey[key]; ok {
			groups[idx] = append(groups[idx], ev)
			continue
		}
		nonPrimary = append(nonPrimary, ev)
	}

	result := make([]types.EvidenceItem, 0, len(items))
	cursors := make([]int, len(primaryOrder))
	for {
		progress := false
		for gi, group := range groups {
			if cursors[gi] < len(group) {
				result = append(result, group[cursors[gi]])
				cursors[gi]++
				progress = true
			}
		}
		if !progress {
			break
		}
	}
	result = append(result, nonPrimary...)
	return result
}

// observePrimaryRead detects whether any primary-entity file has
// just entered the readSet derived from the given tool history. On
// first detection, it snapshots the current iteration and the
// length of investigationNotes so ShouldStop's S1 anchor can
// enforce: "primary file was read AND LLM subsequently wrote fresh
// evidence notes from that read."
//
// Idempotent: once primaryReadSeen is true, later calls are no-ops.
// Called from MidLoopCheck (runs after every tool batch).
func (e *explorerEvaluator) observePrimaryRead(iteration int, history []types.ToolResult) {
	if e.primaryReadSeen {
		return
	}
	primary := e.primaryEntityFiles()
	if len(primary) == 0 {
		return
	}
	_, readSet, _ := extractFileCoverage(history, e.repoRoot)
	for _, pf := range primary {
		if readSetContains(readSet, pf) {
			e.primaryReadSeen = true
			e.primaryReadIter = iteration
			e.notesLenAtPrimaryRead = len(e.investigationNotes)
			logging.Debug("[explorer] primary-entity file read at iter=%d: %s (notesAtRead=%d)",
				iteration, pf, e.notesLenAtPrimaryRead)
			return
		}
	}
}

func (e *explorerEvaluator) unreadActiveFrontierFiles(readSet map[string]bool) []string {
	frontier := e.activeFrontierFiles(readSet, "")
	if len(frontier) == 0 {
		return nil
	}
	var unread []string
	for _, file := range frontier {
		if !readSetContains(readSet, file) {
			unread = append(unread, file)
		}
	}
	return unread
}

func renderPartialReadHint(h partialReadHint, smallRemainderThreshold int) string {
	unreadLines := h.symEnd - h.readEnd
	if unreadLines <= 0 {
		return ""
	}
	if smallRemainderThreshold <= 0 {
		smallRemainderThreshold = types.ResolvedExploreHeuristics(types.ExploreHeuristics{}).PartialReadLineThreshold
	}
	if unreadLines <= smallRemainderThreshold {
		// read_file offset is 0-based (see internal/tool/builtin.go);
		// h.readEnd is the 1-based last line read. The next unread
		// 1-based line is h.readEnd+1 → 0-based offset h.readEnd.
		return fmt.Sprintf("MID-LOOP CHECK: you read `%s` in `%s` up to line %d but the function spans lines %d-%d (%.0f%% covered, %d lines remaining). "+
			"If this function is relevant to the question, call read_file with path=%q offset=%d limit=%d to see the rest.\n",
			h.symbolName, h.file, h.readEnd, h.symStart, h.symEnd, h.coverage*100, unreadLines,
			h.file, types.LineToReadFileOffset(h.readEnd+1), unreadLines)
	}
	return fmt.Sprintf("MID-LOOP CHECK: you read `%s` in `%s` up to line %d but the function spans lines %d-%d (%.0f%% covered, %d lines remaining). "+
		"If this function is relevant to the question, grep for key identifiers within `%s` (lines %d-%d) to find the important sections, then read those specific ranges.\n",
		h.symbolName, h.file, h.readEnd, h.symStart, h.symEnd, h.coverage*100, unreadLines,
		h.file, h.readEnd+1, h.symEnd)
}

type anchorNextHop struct {
	symbol     string
	line       int
	targetFile string
}

type symbolSpan struct {
	start int
	end   int
}

type evidenceRepairTarget struct {
	file  string
	lines []int
}

type readInterval struct {
	start int
	end   int
}

func lineWithinAnySpan(line int, spans []symbolSpan) bool {
	if line <= 0 || len(spans) == 0 {
		return false
	}
	for _, span := range spans {
		if line >= span.start && line <= span.end {
			return true
		}
	}
	return false
}

func (e *explorerEvaluator) primaryAnchorNextHops() (local []anchorNextHop, external []anchorNextHop) {
	anchor, ok := e.uniqueExactAnchorFile()
	if !ok || e.searchResult == nil || e.searchResult.Graph == nil || len(e.ermRequirements) == 0 {
		return nil, nil
	}
	graph := e.searchResult.Graph
	fi := graph.FileIndex[anchor]
	if fi == nil {
		return nil, nil
	}

	entities := make(map[string]string)
	exactEntities := make(map[string]bool)
	for _, req := range e.ermRequirements {
		for _, ent := range req.Entities {
			ent = strings.TrimSpace(ent)
			if ent == "" {
				continue
			}
			entities[strings.ToLower(ent)] = ent
			exactEntities[ent] = true
		}
	}
	if len(entities) == 0 {
		return nil, nil
	}

	var exactSpans []symbolSpan
	var fallbackSpans []symbolSpan
	forEachMatchingDef(entities, graph, func(_, _, _ string, d *repomap.Symbol) bool {
		if d == nil || canonicalExplorerPath(d.File) != anchor {
			return true
		}
		kind := strings.ToLower(d.Kind)
		if kind != "function" && kind != "method" {
			return true
		}
		start := d.Line
		end := d.EndLine
		if end < start {
			end = start
		}
		if start > 0 {
			span := symbolSpan{start: start, end: end}
			fallbackSpans = append(fallbackSpans, span)
			if exactEntities[d.Name] {
				exactSpans = append(exactSpans, span)
			}
		}
		return true
	})
	spans := fallbackSpans
	if len(exactSpans) > 0 {
		spans = exactSpans
	}
	if len(spans) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool)
	add := func(dst *[]anchorNextHop, hop anchorNextHop) {
		if hop.symbol == "" || hop.line <= 0 {
			return
		}
		key := fmt.Sprintf("%d\x00%s\x00%s", hop.line, hop.symbol, hop.targetFile)
		if seen[key] {
			return
		}
		seen[key] = true
		*dst = append(*dst, hop)
	}

	for _, rel := range fi.Relations {
		if rel.Kind != "call" || !lineWithinAnySpan(rel.Line, spans) {
			continue
		}
		symbol := strings.TrimSpace(rel.ToEP.Name)
		if symbol == "" {
			symbol = strings.TrimSpace(rel.To)
		}
		if symbol == "" {
			continue
		}
		var targetFile string
		if target := graph.ResolveCallTarget(fi, rel); target != nil {
			if symbol == "" {
				symbol = strings.TrimSpace(target.Name)
			}
			targetFile = canonicalExplorerPath(target.File)
		}
		if targetFile == "" {
			if files := exactSymbolFiles(graph, symbol); len(files) == 1 {
				resolved := canonicalExplorerPath(files[0])
				targetFile = resolved
			}
		}
		hop := anchorNextHop{
			symbol:     symbol,
			line:       rel.Line,
			targetFile: targetFile,
		}
		if targetFile == anchor {
			add(&local, hop)
			continue
		}
		if isNoisePath(targetFile) {
			continue
		}
		add(&external, hop)
	}

	sort.SliceStable(local, func(i, j int) bool {
		if local[i].line != local[j].line {
			return local[i].line < local[j].line
		}
		return local[i].symbol < local[j].symbol
	})
	sort.SliceStable(external, func(i, j int) bool {
		if external[i].line != external[j].line {
			return external[i].line < external[j].line
		}
		if external[i].targetFile != external[j].targetFile {
			return external[i].targetFile < external[j].targetFile
		}
		return external[i].symbol < external[j].symbol
	})
	return local, external
}

func renderAnchorLocalGroundingHint(anchor string, hops []anchorNextHop) string {
	if len(hops) == 0 {
		return ""
	}
	if anchor == "" {
		anchor = hops[0].targetFile
	}
	maxList := 4
	if len(hops) < maxList {
		maxList = len(hops)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Before widening to other files, stay in `%s` and ground these branch/call sites from the anchor itself:\n", anchor)
	for _, hop := range hops[:maxList] {
		fmt.Fprintf(&b, "  - line %d: `%s`\n", hop.line, hop.symbol)
	}
	b.WriteString("\nFor call evidence from these lines, keep the current containing function as `subject` and the callee on that line as `object` (caller -> callee). ")
	b.WriteString("Use grep/read_file within the anchor around these exact lines if you still need precise gutters or snippets. Expand to other files only after these anchor-local branches are grounded.")
	return b.String()
}

func unreadAnchorExternalTargetFiles(readSet map[string]bool, hops []anchorNextHop) []string {
	if len(hops) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	files := make([]string, 0, len(hops))
	for _, hop := range hops {
		file := canonicalExplorerPath(hop.targetFile)
		if file == "" || seen[file] || readSetContains(readSet, file) {
			continue
		}
		seen[file] = true
		files = append(files, file)
	}
	return files
}

func parseEmitEvidenceRepairTargets(summary string) []evidenceRepairTarget {
	if !strings.Contains(summary, "emit_evidence accepted") {
		return nil
	}
	targets := make(map[string]map[int]bool)
	currentFile := ""
	currentLine := 0
	currentNeedsRepair := false
	currentDrop := false
	flushCurrent := func() {
		if currentFile == "" || currentLine <= 0 || !currentNeedsRepair || currentDrop {
			return
		}
		if targets[currentFile] == nil {
			targets[currentFile] = make(map[int]bool)
		}
		targets[currentFile][currentLine] = true
	}
	for _, raw := range strings.Split(summary, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			flushCurrent()
			at := strings.Index(line, " @ ")
			if at < 0 {
				currentFile = ""
				currentLine = 0
				currentNeedsRepair = false
				currentDrop = false
				continue
			}
			loc := strings.TrimSpace(line[at+3:])
			if dash := strings.Index(loc, " — "); dash >= 0 {
				loc = loc[:dash]
			} else if dash := strings.Index(loc, " - "); dash >= 0 {
				loc = loc[:dash]
			}
			colon := strings.LastIndex(loc, ":")
			if colon < 0 {
				currentFile = ""
				currentLine = 0
				currentNeedsRepair = false
				currentDrop = false
				continue
			}
			n, err := strconv.Atoi(strings.TrimSpace(loc[colon+1:]))
			if err != nil || n <= 0 {
				currentFile = ""
				currentLine = 0
				currentNeedsRepair = false
				currentDrop = false
				continue
			}
			currentFile = canonicalExplorerPath(strings.TrimSpace(loc[:colon]))
			currentLine = n
			currentNeedsRepair = false
			currentDrop = false
			continue
		}
		if currentFile == "" || currentLine <= 0 {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "do not spend read_file budget") || strings.Contains(lower, "do not repair") {
			currentDrop = true
			continue
		}
		if !isEmitEvidenceStatusLine(line) {
			continue
		}
		if !strings.Contains(line, "recovered") && !strings.Contains(line, "ungrounded") {
			continue
		}
		currentNeedsRepair = true
	}
	flushCurrent()
	if len(targets) == 0 {
		return nil
	}
	files := make([]string, 0, len(targets))
	for file := range targets {
		files = append(files, file)
	}
	sort.Strings(files)
	out := make([]evidenceRepairTarget, 0, len(files))
	for _, file := range files {
		var lines []int
		for line := range targets[file] {
			lines = append(lines, line)
		}
		sort.Ints(lines)
		out = append(out, evidenceRepairTarget{file: file, lines: lines})
	}
	return out
}

func repairTargetsFromToolRepair(repair *types.ToolRepair) ([]evidenceRepairTarget, bool) {
	if repair == nil || repair.Code != "evidence_line_text_repair" {
		return nil, false
	}
	if len(repair.Targets) == 0 {
		return nil, true
	}
	out := make([]evidenceRepairTarget, 0, len(repair.Targets))
	for _, target := range repair.Targets {
		if !strings.EqualFold(strings.TrimSpace(target.Action), string(types.RepairReadFile)) {
			continue
		}
		file := canonicalExplorerPath(target.File)
		if file == "" || len(target.Lines) == 0 {
			continue
		}
		lines := append([]int(nil), target.Lines...)
		sort.Ints(lines)
		out = append(out, evidenceRepairTarget{file: file, lines: lines})
	}
	if len(out) == 0 {
		return nil, true
	}
	sort.Slice(out, func(i, j int) bool { return out[i].file < out[j].file })
	return out, true
}

func (e *explorerEvaluator) evidenceRepairTargetsForToolResult(result *types.ToolResult) []evidenceRepairTarget {
	if result == nil {
		return nil
	}
	targets, structured := repairTargetsFromToolRepair(result.Repair)
	if !structured {
		targets = parseEmitEvidenceRepairTargets(result.Summary)
	}
	if len(targets) == 0 {
		return nil
	}
	if anchor, ok := e.uniqueExactAnchorFile(); ok {
		targets = filterEvidenceRepairTargetsByFiles(targets, []string{anchor})
	} else if primary := e.primaryEntityFiles(); len(primary) > 0 {
		targets = filterEvidenceRepairTargetsByFiles(targets, primary)
	}
	return targets
}

func (e *explorerEvaluator) pendingEvidenceRepairTargets(results []types.ToolResult) []evidenceRepairTarget {
	if e == nil || !e.midLoopEvidenceRepairSent || e.midLoopEvidenceRepairResultsLen <= 0 || e.midLoopEvidenceRepairResultsLen > len(results) {
		return nil
	}
	result := results[e.midLoopEvidenceRepairResultsLen-1]
	if result.ToolName != "emit_evidence" || !result.Success {
		return nil
	}
	return e.evidenceRepairTargetsForToolResult(&result)
}

func isEmitEvidenceStatusLine(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "→") || strings.HasPrefix(line, "->")
}

func filterEvidenceRepairTargetsByFiles(targets []evidenceRepairTarget, preferred []string) []evidenceRepairTarget {
	if len(targets) == 0 || len(preferred) == 0 {
		return targets
	}
	allowed := make(map[string]bool, len(preferred))
	for _, file := range preferred {
		file = canonicalExplorerPath(file)
		if file != "" {
			allowed[file] = true
		}
	}
	var kept []evidenceRepairTarget
	for _, target := range targets {
		if allowed[canonicalExplorerPath(target.file)] {
			kept = append(kept, target)
		}
	}
	if len(kept) > 0 {
		return kept
	}
	return targets
}

func renderRepairLineList(lines []int, max int) string {
	if len(lines) == 0 {
		return ""
	}
	if max <= 0 || max > len(lines) {
		max = len(lines)
	}
	parts := make([]string, 0, max+1)
	for _, line := range lines[:max] {
		parts = append(parts, strconv.Itoa(line))
	}
	if len(lines) > max {
		parts = append(parts, "...")
	}
	return strings.Join(parts, ", ")
}

func renderEmitEvidenceRepairHint(targets []evidenceRepairTarget) string {
	if len(targets) == 0 {
		return ""
	}
	maxFiles := 2
	if len(targets) < maxFiles {
		maxFiles = len(targets)
	}
	var b strings.Builder
	b.WriteString("MID-LOOP CHECK: some evidence you just emitted is only recovered or ungrounded, not line-text grounded yet.\n")
	b.WriteString("Before reading other files, re-read these exact source locations and re-emit grounded evidence:\n")
	for _, target := range targets[:maxFiles] {
		lines := renderRepairLineList(target.lines, 4)
		if lines == "" {
			fmt.Fprintf(&b, "  - `%s`\n", target.file)
			continue
		}
		fmt.Fprintf(&b, "  - `%s` near lines %s\n", target.file, lines)
	}
	b.WriteString("\nDo the repair in the existing anchor file first; only widen scope after those items ground cleanly.")
	return b.String()
}

func renderEmitEvidenceRepairClosureOnlyHint(targets []evidenceRepairTarget) string {
	if len(targets) == 0 {
		return ""
	}
	maxFiles := 2
	if len(targets) < maxFiles {
		maxFiles = len(targets)
	}
	var b strings.Builder
	b.WriteString("MID-LOOP CHECK: the previous recovered/ungrounded `emit_evidence` rows are still not line-text grounded. Auto-recovered line numbers are audit feedback, not a completed repair for strict citations.\n")
	b.WriteString("Re-emit `emit_evidence` now for these already-read source locations, using the exact gutter line numbers you just saw:\n")
	for _, target := range targets[:maxFiles] {
		lines := renderRepairLineList(target.lines, 4)
		if lines == "" {
			fmt.Fprintf(&b, "  - `%s`\n", target.file)
			continue
		}
		fmt.Fprintf(&b, "  - `%s` near lines %s\n", target.file, lines)
	}
	b.WriteString("\nDo not open more files or complete the investigation until the repaired `emit_evidence(items=[...])` call succeeds.")
	return b.String()
}

// budgetExhaustedHintMarker is the substring the budget gate writes
// into a refused tool result Summary. Source of truth:
// internal/agent/agent.go::executeTool — keep both in sync if either
// is reworded.
const budgetExhaustedHintMarker = "explore budget exhausted for tool"

// extractBudgetExhaustedToolName parses the canonical budget-rejection
// Summary shape (`explore budget exhausted for tool "<name>": …`) and
// returns the bare tool name. Returns empty string when the marker
// isn't present or the surrounding shape changes.
func extractBudgetExhaustedToolName(summary string) string {
	idx := strings.Index(summary, budgetExhaustedHintMarker)
	if idx < 0 {
		return ""
	}
	rest := summary[idx+len(budgetExhaustedHintMarker):]
	q1 := strings.Index(rest, `"`)
	if q1 < 0 {
		return ""
	}
	q2 := strings.Index(rest[q1+1:], `"`)
	if q2 < 0 {
		return ""
	}
	return rest[q1+1 : q1+1+q2]
}

// postBudgetExhaustedSignal fires a per-tool one-shot nudge when the
// explore budget for a tool refuses an LLM call. Without this nudge
// the LLM keeps batching parallel calls against the same exhausted
// tool — every call returns "budget exhausted" but the LLM only
// learns this AFTER the iter completes, then plans another iter
// that may include more of the same.
//
// BypassThrottle + BypassBudget so the nudge lands on the very next
// iter regardless of recent injection cadence — the budget condition
// is structural (next call to <tool> WILL refuse), not a soft hint
// that politeness rules should defer.
//
// Per-tool MaxPerKeyInjects=5 (P2) caps total fires of this key per
// dispatch, which prevents log spam if the LLM stubbornly keeps
// trying the exhausted tool.
func (e *explorerEvaluator) postBudgetExhaustedSignal(obs LoopObservation) LoopSignal {
	if obs.LastToolResult == nil || obs.LastToolResult.Success {
		return LoopSignal{}
	}
	tool := extractBudgetExhaustedToolName(obs.LastToolResult.Summary)
	if tool == "" {
		return LoopSignal{}
	}
	if e.midLoopBudgetExhaustedSent == nil {
		e.midLoopBudgetExhaustedSent = make(map[string]bool)
	}
	if e.midLoopBudgetExhaustedSent[tool] {
		return LoopSignal{}
	}
	e.midLoopBudgetExhaustedSent[tool] = true
	// Tool-specific alternative hints. The list is conservative —
	// other read-class tools the explorer can still call. We don't
	// recommend emit_* directly because the LLM also needs to make
	// progress; emit before further reads would be premature.
	alternatives := map[string]string{
		"read_file":  "`grep` (with `files_only=false` to fetch a small line window) or `repo_map`",
		"grep":       "`read_file` (focused at a specific path:line) or `repo_map`",
		"repo_map":   "`grep` (for token-anchored discovery) or `list_files`",
		"list_files": "`grep` or `repo_map`",
	}
	alt, ok := alternatives[tool]
	if !ok {
		alt = "a different read tool"
	}
	hint := fmt.Sprintf(
		"The `%s` budget is exhausted — every further `%s` call this dispatch will refuse with the same error. Stop calling `%s`. Use %s for the next investigative step, or call `emit_evidence` / `emit_investigation_complete` to advance the investigation. Do not retry the same `%s` call with different arguments — the cap is per-tool, not per-argument.",
		tool, tool, tool, alt, tool)
	return LoopSignal{
		HintRequested:  true,
		HintKey:        fmt.Sprintf("explorer.budget_exhausted.%s", tool),
		Hint:           hint,
		Progress:       false,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

func (e *explorerEvaluator) postEmitEvidenceRepairSignal(obs LoopObservation) LoopSignal {
	if e.midLoopEvidenceRepairSent || obs.LastToolResult == nil || obs.LastToolResult.ToolName != "emit_evidence" || !obs.LastToolResult.Success {
		return LoopSignal{}
	}
	targets := e.evidenceRepairTargetsForToolResult(obs.LastToolResult)
	if len(targets) == 0 {
		return LoopSignal{}
	}
	e.midLoopEvidenceRepairSent = true
	e.midLoopEvidenceRepairResultsLen = len(obs.AllToolResults)
	return LoopSignal{
		HintRequested:  true,
		HintKey:        "explorer.mid-loop.evidence-repair",
		Hint:           renderEmitEvidenceRepairHint(targets),
		Progress:       true,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

func (e *explorerEvaluator) postEmitEvidenceSurfaceTermReviewSignal(obs LoopObservation) LoopSignal {
	if e.midLoopSurfaceTermReviewSent || obs.LastToolResult == nil || obs.LastToolResult.ToolName != "emit_evidence" || !obs.LastToolResult.Success {
		return LoopSignal{}
	}
	repair := obs.LastToolResult.Repair
	if repair == nil || repair.Code != tool.EmitEvidenceSurfaceTermReviewCode || strings.TrimSpace(repair.Hint) == "" {
		return LoopSignal{}
	}
	e.midLoopSurfaceTermReviewSent = true
	return LoopSignal{
		HintRequested:  true,
		HintKey:        "explorer.mid-loop.surface-terms-review",
		Hint:           repair.Hint,
		Progress:       true,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

func (e *explorerEvaluator) postEmitEvidenceRepairClosureOnlySignal(obs LoopObservation) LoopSignal {
	if !e.awaitingEvidenceRepair(obs.AllToolResults) || e.investigationComplete {
		return LoopSignal{}
	}
	navCount := successfulToolCountSince(obs.AllToolResults, e.midLoopLastResultsLen, navigationToolNames)
	if navCount == 0 {
		return LoopSignal{}
	}
	if successfulToolCountSince(obs.AllToolResults, e.midLoopLastResultsLen, completionProgressToolNames) > 0 {
		return LoopSignal{}
	}
	targets := e.pendingEvidenceRepairTargets(obs.AllToolResults)
	hint := "MID-LOOP CHECK: the current dispatch already has a concrete `emit_evidence` repair to do on previously-read anchors. Finish that repair and re-emit grounded evidence before widening scope or opening more files. Do not keep navigating until the repaired `emit_evidence(items=[...])` batch succeeds."
	if len(targets) > 0 {
		hint = renderEmitEvidenceRepairClosureOnlyHint(targets)
	}
	return LoopSignal{
		HintRequested:  true,
		HintKey:        fmt.Sprintf("explorer.mid-loop.evidence-repair-closure-only.%d", obs.Iteration),
		Hint:           hint,
		Progress:       true,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

// postReadWithoutEmitSignal fires a one-shot mid-loop nudge when the
// LLM has read multiple files but has not called emit_evidence yet.
//
// This closes the gap where the LLM jumps from a read_file batch
// straight to emit_investigation_complete, skipping the evidence-
// emission step. When that happens, Structured Evidence is empty,
// Turn B has nothing to quote, and the finalizer falls back to prose
// synthesis of the investigation notes — exactly the "explorer didn't
// emit_evidence" pathology observed on the 10:04 run.
//
// The tool-side pre-complete simulator also downgrades completion in
// that state (zero cite-eligible items < MinCitations floor), but
// that fires AFTER the LLM has already tried to complete; this hint
// fires BEFORE, letting the LLM emit first and complete cleanly. A
// one-shot guard prevents repeated firing within a single evidence
// backlog window; the key includes the post-emit window boundary so
// LoopPolicy dedup does not suppress a later, independent backlog.
//
// Thresholds are intentionally "early but not first-hop": 2+
// iterations and 2+ successful read_file calls, with the CURRENT
// batch containing a successful read_file. This catches the common
// drift where the model keeps reading files without ever
// materializing the current evidence backlog, even when the batch
// ends in grep / a failed emit_evidence retry rather than the
// read_file result itself.
func (e *explorerEvaluator) postReadWithoutEmitSignal(obs LoopObservation) LoopSignal {
	if e.closureReadyLatched() {
		return LoopSignal{}
	}
	if e.midLoopNoEmitPushSent {
		return LoopSignal{}
	}
	if obs.Iteration < 2 || !currentBatchHasSuccessfulRead(obs.AllToolResults, e.midLoopLastResultsLen) {
		return LoopSignal{}
	}
	reads := successfulToolCountSince(obs.AllToolResults, e.midLoopEmitBacklogBaseLen, map[string]bool{"read_file": true})
	if reads < 2 || successfulToolCountSince(obs.AllToolResults, e.midLoopEmitBacklogBaseLen, map[string]bool{"emit_evidence": true}) > 0 {
		return LoopSignal{}
	}
	e.midLoopNoEmitPushSent = true
	e.midLoopNoEmitPushIter = obs.Iteration
	e.midLoopNoEmitPushResultsLen = len(obs.AllToolResults)
	scope := "this round"
	recording := "have not called `emit_evidence` yet"
	if e.midLoopEmitBacklogBaseLen > 0 {
		scope = "since your last successful `emit_evidence`"
		recording = "have not recorded any new structured evidence yet"
	}
	return LoopSignal{
		HintRequested: true,
		HintKey:       e.readWithoutEmitHintKey(),
		Hint: fmt.Sprintf(
			"MID-LOOP CHECK: you have read %d file(s) %s but %s. "+
				"Facts left only in your prose notes are NOT recorded — anything that is not passed through `emit_evidence(items=[...])` is invisible to the rest of the pipeline (concrete value, definition, call-site, or condition). "+
				"Pick the strongest anchors you have identified in the files you just read and emit them in ONE batch now. Line numbers MUST come verbatim from the `read_file` gutter (copy the leading `N| ` prefix). "+
				"After the batch succeeds, continue investigating or call `emit_investigation_complete(reason, confidence, result_kind)`.%s%s",
			reads, scope, recording, e.authoritativeLogDriftReminder(obs.AllToolResults), e.authoritativeLogBackboneFirstEmitReminder(obs.AllToolResults)),
		Progress:       true,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

func (e *explorerEvaluator) postReadWithoutEmitSoftStopSignal(obs LoopObservation) LoopSignal {
	if e.closureReadyLatched() || e.investigationComplete {
		return LoopSignal{}
	}
	reads := successfulToolCountSince(obs.AllToolResults, e.midLoopEmitBacklogBaseLen, map[string]bool{"read_file": true})
	if reads < 2 {
		return LoopSignal{}
	}
	if successfulToolCountSince(obs.AllToolResults, e.midLoopEmitBacklogBaseLen, map[string]bool{"emit_evidence": true}) > 0 {
		return LoopSignal{}
	}
	if !e.midLoopNoEmitPushSent {
		e.midLoopNoEmitPushSent = true
		e.midLoopNoEmitPushIter = obs.Iteration
		e.midLoopNoEmitPushResultsLen = len(obs.AllToolResults)
	}
	scope := "this dispatch"
	recording := "no successful `emit_evidence` call"
	if e.midLoopEmitBacklogBaseLen > 0 {
		scope = "since the last successful `emit_evidence`"
		recording = "no new successful `emit_evidence` call"
	}
	return LoopSignal{
		HintRequested: true,
		HintKey:       fmt.Sprintf("%s.%d", e.emitBacklogWindowHintKey("explorer.soft-stop.read-without-emit"), obs.Iteration),
		Hint: fmt.Sprintf(
			"You have read %d file(s) %s but there is still %s. "+
				"Do not answer in prose and do not broaden the search yet. "+
				"Convert the strongest facts from the lines already read into ONE `emit_evidence(items=[...])` tool call now, using exact line numbers from the `read_file` gutter. "+
				"After that call succeeds, either continue with a genuinely unresolved branch or call `emit_investigation_complete(reason, confidence, result_kind)`.",
			reads, scope, recording),
		Progress:       true,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

func (e *explorerEvaluator) readWithoutEmitHintKey() string {
	return e.emitBacklogWindowHintKey("explorer.mid-loop.read-without-emit")
}

func (e *explorerEvaluator) emitBacklogWindowHintKey(base string) string {
	if e == nil || e.midLoopEmitBacklogBaseLen <= 0 {
		return base
	}
	return fmt.Sprintf("%s.%d", base, e.midLoopEmitBacklogBaseLen)
}

func (e *explorerEvaluator) postExecRedirectBeforeEmitSignal(obs LoopObservation) LoopSignal {
	if e.closureReadyLatched() {
		return LoopSignal{}
	}
	if e.midLoopExecRedirectSent || !e.midLoopNoEmitPushSent || e.midLoopNoEmitPushResultsLen == 0 {
		return LoopSignal{}
	}
	if !e.awaitingStructuredEvidenceMaterialization(obs.AllToolResults) {
		return LoopSignal{}
	}
	if obs.Iteration <= e.midLoopNoEmitPushIter {
		return LoopSignal{}
	}
	if successfulToolCountSince(obs.AllToolResults, e.midLoopNoEmitPushResultsLen, map[string]bool{"exec_command": true}) == 0 {
		return LoopSignal{}
	}
	e.midLoopExecRedirectSent = true
	return LoopSignal{
		HintRequested: true,
		HintKey:       e.emitBacklogWindowHintKey("explorer.mid-loop.exec-redirect-before-emit"),
		Hint: "MID-LOOP CHECK: you are still browsing with `exec_command` before recording the current structured-evidence backlog. " +
			"For repository investigation, switch back to the built-in `grep` / `read_file` tools so paths stay stable across OSes and line gutters remain machine-readable. " +
			"Use the lines you already read to call `emit_evidence(items=[...])` now; reserve `exec_command` for deterministic computations or checks that the structured tools cannot perform directly.",
		Progress:       true,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

func (e *explorerEvaluator) postReadWithoutEmitEscalationSignal(obs LoopObservation) LoopSignal {
	if e.closureReadyLatched() {
		return LoopSignal{}
	}
	if e.midLoopNoEmitEscalated || !e.midLoopNoEmitPushSent || e.midLoopNoEmitPushResultsLen == 0 {
		return LoopSignal{}
	}
	if !e.awaitingStructuredEvidenceMaterialization(obs.AllToolResults) {
		return LoopSignal{}
	}
	if obs.Iteration < e.midLoopNoEmitPushIter+2 {
		return LoopSignal{}
	}
	navigationCalls := successfulToolCountSince(obs.AllToolResults, e.midLoopNoEmitPushResultsLen, navigationToolNames)
	if navigationCalls < 2 {
		return LoopSignal{}
	}
	e.midLoopNoEmitEscalated = true
	backlogScope := "after additional tool rounds"
	if e.midLoopEmitBacklogBaseLen > 0 {
		backlogScope = "after opening additional files since the last successful `emit_evidence`"
	}
	return LoopSignal{
		HintRequested: true,
		HintKey:       e.emitBacklogWindowHintKey("explorer.mid-loop.read-without-emit-escalated"),
		Hint: "MID-LOOP CHECK: the earlier `emit_evidence` nudge was ignored and you still have an unrecorded evidence backlog " + backlogScope + ". " +
			"Stop expanding with more navigation for the moment. Use the grounded lines you have already read to emit ONE batch of `emit_evidence(items=[...])` now. " +
			"After that batch succeeds, either continue on any truly unresolved branch or call `emit_investigation_complete(reason, confidence, result_kind)` if the evidence already answers the question." +
			e.authoritativeLogDriftReminder(obs.AllToolResults) +
			e.authoritativeLogBackboneFirstEmitReminder(obs.AllToolResults),
		Progress:       true,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

func (e *explorerEvaluator) postReadWithoutEmitClosureOnlySignal(obs LoopObservation) LoopSignal {
	if !e.midLoopNoEmitEscalated || e.investigationComplete || !e.awaitingStructuredEvidenceMaterialization(obs.AllToolResults) {
		return LoopSignal{}
	}
	navCount := successfulToolCountSince(obs.AllToolResults, e.midLoopLastResultsLen, navigationToolNames)
	if navCount == 0 {
		return LoopSignal{}
	}
	if successfulToolCountSince(obs.AllToolResults, e.midLoopLastResultsLen, completionProgressToolNames) > 0 {
		return LoopSignal{}
	}
	return LoopSignal{
		HintRequested:  true,
		HintKey:        fmt.Sprintf("explorer.mid-loop.read-without-emit-closure-only.%d", obs.Iteration),
		Hint:           "MID-LOOP CHECK: an earlier hint already established that your next useful step is to materialize structured evidence. The current batch still spent effort on navigation tools without recording the current backlog. Do NOT keep expanding with `read_file`, `grep`, `repo_map`, `list_files`, or `exec_command` until you first emit ONE grounded `emit_evidence(items=[...])` batch from the lines you already have." + e.authoritativeLogDriftReminder(obs.AllToolResults),
		Progress:       true,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

func (e *explorerEvaluator) authoritativeLogDriftReminder(allResults []types.ToolResult) string {
	if !e.authoritativeFailureCovered(allResults) {
		return ""
	}
	if len(e.authoritativeFrameSymbolTailsByFile()) == 0 {
		return ""
	}
	noun := e.failureTraceNounPhrase()
	return " For " + noun + ", treat raw stack/sample line numbers as stale locators once the named file/function has been grounded. Emit evidence from the current grounded functions/calls you already read instead of chasing historical numeric offsets."
}

func (e *explorerEvaluator) authoritativeLogBackboneFirstEmitReminder(allResults []types.ToolResult) string {
	if !e.authoritativeFailureCovered(allResults) {
		return ""
	}
	if hasSuccessfulTool(allResults, "emit_evidence") {
		return ""
	}
	if len(e.authoritativeResolvedFrames()) == 0 {
		return ""
	}
	noun := e.failureTraceNounPhrase()
	return " For " + noun + ", keep the FIRST `emit_evidence` batch on the failure path itself: record one caller→callee edge from the grounded frames plus one mechanism/guard/definition anchor from those same frame files. Defer related_context, setup helpers, and broader same-file background until that initial batch succeeds."
}

// failureTraceNounPhrase renders the LLM-facing noun phrase for the
// attached failure trace, prioritised by the user's question framing
// over the bundle's signal labels:
//
//  1. RequestModel.Intent (LLM-classified): IntentPerformance →
//     "the attached performance trace"; IntentRootCause /
//     IntentTrace → defer to signals (the user did ask about a
//     failure, but the bundle's signals say what kind).
//  2. LogBundle.Meta.Signals + PerfBundle.Meta.Signals union:
//     deduplicate, cap at 3, render as "panic/oom/timeout traces" or
//     "jank/main-thread-stall traces".
//  3. Empty union: fall back to "the attached failure trace".
//
// All inputs are typed enums emitted by the LLM through validated
// schemas — no substring matching against RawRequest. Decoupling the
// text from the panic/crash whitelist is what lets the same reminder
// fire for OOM, timeout, jank, and assertion-violation questions.
func (e *explorerEvaluator) failureTraceNounPhrase() string {
	if e == nil {
		return "the attached failure trace"
	}
	intent := e.requestIntent()
	hasLog := e.logTriage != nil && len(e.logTriage.Meta.Signals) > 0
	hasPerf := e.perfTrace != nil && len(e.perfTrace.Meta.Signals) > 0
	// Performance framing: prefer the user's intent or the perf
	// bundle's mere presence over enumerating signals.
	if e.perfTrace != nil && (intent == "performance" || !hasLog) {
		labels := uniqLowerLabels(e.perfTrace.Meta.Signals, 3)
		if len(labels) == 0 {
			return "the attached performance trace"
		}
		return strings.Join(labels, "/") + " traces"
	}
	signals := make([]string, 0, 6)
	if hasLog {
		for _, s := range e.logTriage.Meta.Signals {
			signals = append(signals, string(s))
		}
	}
	if hasPerf {
		signals = append(signals, e.perfTrace.Meta.Signals...)
	}
	labels := uniqLowerLabels(signals, 3)
	if len(labels) == 0 {
		return "the attached failure trace"
	}
	return strings.Join(labels, "/") + " traces"
}

func (e *explorerEvaluator) requestIntent() string {
	if e == nil || e.analysisIR == nil {
		return ""
	}
	return string(e.analysisIR.RequestModel.Intent)
}

func uniqLowerLabels[S ~string](in []S, cap int) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		v := strings.ToLower(strings.TrimSpace(string(s)))
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
		if cap > 0 && len(out) >= cap {
			break
		}
	}
	return out
}

func (e *explorerEvaluator) closureReadyLatched() bool {
	return e.midLoopCompletionReadySent || e.midLoopExactAbsenceSent
}

func (e *explorerEvaluator) postClosureReadyBacklogSignal(obs LoopObservation) LoopSignal {
	if !e.closureReadyLatched() || e.investigationComplete || !e.awaitingStructuredEvidenceMaterialization(obs.AllToolResults) {
		return LoopSignal{}
	}
	if successfulToolCountSince(obs.AllToolResults, e.midLoopLastResultsLen, completionProgressToolNames) > 0 {
		return LoopSignal{}
	}
	navCount := successfulToolCountSince(obs.AllToolResults, e.midLoopLastResultsLen, navigationToolNames)
	if navCount == 0 {
		return LoopSignal{}
	}
	state := "the current branch is already closure-ready"
	if e.midLoopExactAbsenceSent && !e.midLoopCompletionReadySent {
		state = "the exact-absence closure is already established"
	}
	return LoopSignal{
		HintRequested: true,
		HintKey:       fmt.Sprintf("explorer.mid-loop.closure-ready-backlog.%d", obs.Iteration),
		Hint: "MID-LOOP CHECK: " + state + ", but this batch reopened navigation before finishing the answer. " +
			"If the new lines you opened truly change the answer, emit exactly ONE grounded `emit_evidence(items=[...])` repair batch from those lines now. " +
			"Otherwise stop and call `emit_investigation_complete(reason, confidence, result_kind)` immediately. " +
			"Do NOT keep widening scope or opening more neighboring files from here.",
		Progress:       true,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

func (e *explorerEvaluator) postExternalLogRedirectSignal(obs LoopObservation) LoopSignal {
	if e.midLoopExternalArtifactSent {
		return LoopSignal{}
	}
	artifactLabel := ""
	if e.logTriage != nil && e.logTriage.IsExternalSource() {
		artifactLabel = "log"
	} else if e.perfTrace != nil && e.perfTrace.IsExternalSource() {
		artifactLabel = "trace"
	}
	if artifactLabel == "" {
		return LoopSignal{}
	}
	for _, r := range obs.AllToolResults {
		if r.Success || r.ToolName != "emit_evidence" {
			continue
		}
		summary := strings.ToLower(r.Summary)
		if !strings.Contains(summary, "repo-relative file path") &&
			!strings.Contains(summary, "runtime log (unresolved)") &&
			!strings.Contains(summary, "external_perf_stall_unresolved") {
			continue
		}
		e.midLoopExternalArtifactSent = true
		hintKey := "explorer.mid-loop.external-runtime-no-anchor"
		if artifactLabel == "log" {
			hintKey = "explorer.mid-loop.external-log-no-anchor"
		}
		return LoopSignal{
			HintRequested: true,
			HintKey:       hintKey,
			Hint: fmt.Sprintf("MID-LOOP CHECK: the attached %s is an external-source runtime artifact (resolved_files=0). ", artifactLabel) +
				"Runtime frames / spans that do not resolve to repo files cannot go through `emit_evidence`, and reading unrelated repo files just to manufacture citations is wasted work. " +
				"If the structured runtime artifact already answers the question, call `emit_investigation_complete` now — the answer can be composed from the log / trace semantics alone. " +
				"Only continue repo reads if you have identified a real repository file that explains how this repo handles the observed runtime behavior.",
			Progress:       true,
			BypassThrottle: true,
			BypassBudget:   true,
		}
	}
	return LoopSignal{}
}

func (e *explorerEvaluator) postExactAbsenceClosureSignal(obs LoopObservation) LoopSignal {
	if e.midLoopExactAbsenceSent || e.exactResolution == nil || !e.exactResolution.AllowAbsence {
		return LoopSignal{}
	}
	targets := e.exactPendingTargets
	if len(targets) == 0 {
		targets = e.exactResolution.Targets
	}
	if len(targets) == 0 || obs.Iteration < e.heuristics.MidLoopMinIteration {
		return LoopSignal{}
	}
	reads, emits := 0, 0
	for _, r := range obs.AllToolResults {
		if !r.Success {
			continue
		}
		switch r.ToolName {
		case "read_file":
			reads++
		case "emit_evidence":
			emits++
		}
	}
	if reads < 2 || emits == 0 {
		return LoopSignal{}
	}
	_, readSet, _ := extractFileCoverage(obs.AllToolResults, e.repoRoot)
	if !e.midLoopExactAbsenceContextSent {
		if hops := e.pendingConfigTraceCoverageHops(readSet); len(hops) > 0 {
			e.rememberExactContextHopFiles(hops)
			e.midLoopExactAbsenceContextSent = true
			var b strings.Builder
			if roles := types.ConfigTraceMissingRequestedDiagramRoles(e.exactResolution, e.exactContextFiles, e.structuredEvidence); len(roles) > 0 {
				fmt.Fprintf(&b, "MID-LOOP CHECK: the exact target already looks absent, but the current config-trace answer is still missing grounded precedence coverage for the user-requested role(s) `%s`. Before closing, follow the next structural consumer / merge hop from the precedence layer you already grounded.\n", types.JoinEvidenceDiagramRoles(roles))
			} else {
				b.WriteString("MID-LOOP CHECK: the exact target already looks absent, but the current config-trace answer is still missing one or more grounded precedence hops. Before closing, follow the next structural consumer / merge hop from the precedence layer you already grounded.\n")
			}
			b.WriteString("The next step is file-local grounding, not another repo-wide search. Use `read_file` directly on one or two of these structurally connected files next, then emit evidence and only after that close with `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`:\n")
			limit := len(hops)
			if limit > 2 {
				limit = 2
			}
			for i := 0; i < limit; i++ {
				hop := hops[i]
				if hop.Via != "" {
					fmt.Fprintf(&b, "- `%s` (%s)\n", hop.File, hop.Via)
				} else {
					fmt.Fprintf(&b, "- `%s`\n", hop.File)
				}
			}
			b.WriteString("Do NOT widen scope with another repo-wide `grep` / `search_repo_map` first unless one of these listed files itself fans out into multiple same-scope anchors. Keep every such item labeled as related context only, never as an equivalent or substitute for the exact missing target.")
			return LoopSignal{
				HintRequested:  true,
				HintKey:        "explorer.mid-loop.exact-absence-precedence-next-hop",
				Hint:           b.String(),
				Progress:       true,
				BypassThrottle: true,
				BypassBudget:   true,
			}
		}
		cands := e.collectExactResolutionSymbolCandidates(e.exactResolution, e.analyzerKeywords)
		if pending := pendingExactResolutionContextCandidates(e.exactResolution, e.structuredEvidence, cands); len(pending) > 0 {
			e.midLoopExactAbsenceContextSent = true
			var b strings.Builder
			if e.scenario == types.ScenarioConfigTrace && e.exactResolution != nil && e.exactResolution.TargetKind == types.SubjectConfigKey {
				if roles := types.ConfigTraceMissingRequestedDiagramRoles(e.exactResolution, e.exactContextFiles, e.structuredEvidence); len(roles) > 0 {
					fmt.Fprintf(&b, "MID-LOOP CHECK: the exact target already looks absent, but before closing this config-trace answer still needs grounded precedence coverage for the user-requested roles `%s`. Keep reading within the current same-scope lineage until each requested role has its own grounded anchor.\n", types.JoinEvidenceDiagramRoles(roles))
				} else {
					b.WriteString("MID-LOOP CHECK: the exact target already looks absent, but before closing this config-trace answer still needs enough grounded precedence coverage to explain the nearby lineage honestly. When the same-scope search already spans multiple files, aim for at least two validated precedence roles (for example `default` plus `config` / `runtime` / `override`) before you close.\n")
				}
			} else {
				b.WriteString("MID-LOOP CHECK: the exact target already looks absent, but before closing you still need one grounded production same-family anchor so the related context stays focused.\n")
			}
			b.WriteString("Read one or two of these repo_map-ranked symbols next, then emit evidence and only after that close with `emit_investigation_complete(..., result_kind=\"absence\", absence_justification=...)`:\n")
			limit := len(pending)
			if limit > 2 {
				limit = 2
			}
			for i := 0; i < limit; i++ {
				cand := pending[i]
				if cand.Line > 0 {
					fmt.Fprintf(&b, "- `%s` in `%s:%d`\n", cand.Symbol, cand.File, cand.Line)
					continue
				}
				fmt.Fprintf(&b, "- `%s` in `%s`\n", cand.Symbol, cand.File)
			}
			b.WriteString("Keep every such item labeled as related context only, never as an equivalent or substitute for the exact missing target.")
			return LoopSignal{
				HintRequested:  true,
				HintKey:        "explorer.mid-loop.exact-absence-read-same-family",
				Hint:           b.String(),
				Progress:       true,
				BypassThrottle: true,
				BypassBudget:   true,
			}
		}
	}
	if !exactAbsenceClosureReady(e.exactResolution, e.scenario, targets, e.structuredEvidence, e.exactContextFiles) {
		return LoopSignal{}
	}
	e.midLoopExactAbsenceSent = true
	label := strings.TrimSpace(e.exactResolution.TargetLabel)
	if label == "" {
		label = "target"
	}
	return LoopSignal{
		HintRequested: true,
		HintKey:       "explorer.mid-loop.exact-absence-close",
		Hint: fmt.Sprintf(
			"MID-LOOP CHECK: the requested exact %s already appears resolved as absent / not found, and you already have grounded same-scope context. "+
				"Do NOT keep expanding nearby knobs / symbols as substitutes. If you do not have explicit alias / parser-mapping proof that names the exact target, close now with `emit_investigation_complete(reason, confidence, result_kind=\"absence\", absence_justification=...)`. "+
				"Any remaining nearby items must stay labeled as related context only, not equivalents.",
			label),
		Progress:       true,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

func (e *explorerEvaluator) rememberExactContextHopFiles(hops []configTraceCoverageHop) {
	if e == nil || len(hops) == 0 {
		return
	}
	files := make([]string, 0, len(hops))
	for _, hop := range hops {
		if hop.File != "" {
			files = append(files, hop.File)
		}
	}
	e.exactContextFiles = mergeContextScopeFiles(e.exactContextFiles, files)
}

type configTraceCoverageHop struct {
	File  string
	Via   string
	Score int
}

func (e *explorerEvaluator) pendingConfigTraceCoverageHops(readSet map[string]bool) []configTraceCoverageHop {
	if e == nil || e.exactResolution == nil || e.searchResult == nil || e.searchResult.Graph == nil {
		return nil
	}
	if e.scenario != types.ScenarioConfigTrace || e.exactResolution.TargetKind != types.SubjectConfigKey {
		return nil
	}
	bestByFile := make(map[string]configTraceCoverageHop)
	add := func(file string, score int, via string) {
		file = canonicalExplorerPath(file)
		if file == "" || readSetContains(readSet, file) || types.LooksLikeAuxiliaryEvidencePath(file) || !e.activeFocusAllowsFile(file) {
			return
		}
		if score <= 0 {
			return
		}
		for _, required := range e.requiredFiles {
			if canonicalExplorerPath(required) == file {
				score += 6
				break
			}
		}
		if e.searchResult.Graph.QueryScores != nil {
			if q := e.searchResult.Graph.QueryScores[file]; q > 0 {
				score += int(q * 10)
			}
		}
		hop := configTraceCoverageHop{File: file, Via: via, Score: score}
		if cur, ok := bestByFile[file]; !ok || hop.Score > cur.Score {
			bestByFile[file] = hop
		}
	}
	for _, file := range e.exactContextFiles {
		add(file, 40, "already in the current same-scope precedence search")
	}
	for _, item := range e.structuredEvidence {
		role := scopeShapingDiagramRole(e.exactResolution, item, e.exactContextFiles)
		if role == types.EvidenceDiagramRoleUnknown {
			continue
		}
		source := canonicalExplorerPath(item.Source)
		if source == "" || !readSetContains(readSet, source) {
			continue
		}
		for _, consumer := range e.searchResult.Graph.FilesImporting(source) {
			via := fmt.Sprintf("imports / consumes the current `%s` precedence layer from `%s`", role, source)
			score := 18
			switch role {
			case types.EvidenceDiagramRoleRuntime, types.EvidenceDiagramRoleOverride:
				score = 30
			case types.EvidenceDiagramRoleConfig:
				score = 22
			case types.EvidenceDiagramRoleDefault:
				score = 16
			}
			add(consumer, score, via)
		}
	}
	if len(bestByFile) == 0 {
		return nil
	}
	hops := make([]configTraceCoverageHop, 0, len(bestByFile))
	for _, hop := range bestByFile {
		hops = append(hops, hop)
	}
	sort.SliceStable(hops, func(i, j int) bool {
		if hops[i].Score != hops[j].Score {
			return hops[i].Score > hops[j].Score
		}
		return hops[i].File < hops[j].File
	})
	return hops
}

func currentBatchHasSuccessfulRead(results []types.ToolResult, prevLen int) bool {
	return currentBatchHasSuccessfulTool(results, prevLen, "read_file")
}

func currentBatchHasSuccessfulTool(results []types.ToolResult, prevLen int, toolName string) bool {
	if prevLen < 0 || prevLen > len(results) {
		prevLen = 0
	}
	for _, r := range results[prevLen:] {
		if r.Success && r.ToolName == toolName {
			return true
		}
	}
	return false
}

func lastSuccessfulToolIndex(results []types.ToolResult, toolName string) int {
	names := map[string]bool{toolName: true}
	for i := len(results) - 1; i >= 0; i-- {
		if toolResultCountsAsProgress(results[i], names) {
			return i
		}
	}
	return -1
}

var navigationToolNames = map[string]bool{
	"read_file":    true,
	"grep":         true,
	"list_files":   true,
	"repo_map":     true,
	"exec_command": true,
}

var completionProgressToolNames = map[string]bool{
	"emit_evidence":               true,
	"emit_investigation_complete": true,
}

func hasSuccessfulTool(results []types.ToolResult, toolName string) bool {
	for _, r := range results {
		if r.Success && r.ToolName == toolName {
			return true
		}
	}
	return false
}

func successfulToolCountSince(results []types.ToolResult, prevLen int, names map[string]bool) int {
	if prevLen < 0 || prevLen > len(results) {
		prevLen = 0
	}
	count := 0
	for _, r := range results[prevLen:] {
		if toolResultCountsAsProgress(r, names) {
			count++
		}
	}
	return count
}

func toolResultCountsAsProgress(r types.ToolResult, names map[string]bool) bool {
	if !r.Success || !names[r.ToolName] {
		return false
	}
	if r.Repair != nil {
		if r.Repair.Code == tool.EmitEvidenceDuplicateNoopCode {
			return false
		}
		if strings.EqualFold(strings.TrimSpace(r.Repair.Metadata["progress"]), "none") {
			return false
		}
	}
	return true
}

func (e *explorerEvaluator) syncEmitBacklogWindow(results []types.ToolResult) {
	baseLen := 0
	if idx := lastSuccessfulToolIndex(results, "emit_evidence"); idx >= 0 {
		baseLen = idx + 1
	}
	if baseLen == e.midLoopEmitBacklogBaseLen {
		return
	}
	e.midLoopEmitBacklogBaseLen = baseLen
	e.midLoopNoEmitPushSent = false
	e.midLoopNoEmitEscalated = false
	e.midLoopExecRedirectSent = false
	e.midLoopNoEmitPushIter = 0
	e.midLoopNoEmitPushResultsLen = 0
}

func (e *explorerEvaluator) awaitingStructuredEvidenceMaterialization(results []types.ToolResult) bool {
	if !e.midLoopNoEmitPushSent {
		return false
	}
	return successfulToolCountSince(results, e.midLoopEmitBacklogBaseLen, completionProgressToolNames) == 0
}

func (e *explorerEvaluator) awaitingEvidenceRepair(results []types.ToolResult) bool {
	if !e.midLoopEvidenceRepairSent || e.midLoopEvidenceRepairResultsLen == 0 {
		return false
	}
	return successfulToolCountSince(results, e.midLoopEvidenceRepairResultsLen, map[string]bool{"emit_evidence": true}) == 0
}

func (e *explorerEvaluator) syncEvidenceRepairState(results []types.ToolResult) {
	if !e.midLoopEvidenceRepairSent || e.midLoopEvidenceRepairResultsLen == 0 {
		return
	}
	if successfulToolCountSince(results, e.midLoopEvidenceRepairResultsLen, map[string]bool{"emit_evidence": true}) == 0 {
		return
	}
	e.midLoopEvidenceRepairSent = false
	e.midLoopEvidenceRepairResultsLen = 0
}

func closureRepairDirectives(mutable *types.MutableState) []types.RepairDirective {
	if mutable == nil {
		return nil
	}
	closure := mutable.EvidenceClosure()
	if closure == nil {
		return nil
	}
	repairs := closure.ActiveRepairs()
	if len(repairs) == 0 {
		return nil
	}
	out := make([]types.RepairDirective, 0, len(repairs))
	for _, repair := range repairs {
		switch repair.Kind {
		case types.RepairReadFile, types.RepairEmitEvidence, types.RepairExpandSearch, types.RepairRebindSubject, types.RepairForceCompleteDowngrade:
			out = append(out, repair)
		}
	}
	return types.MergeRepairs(out)
}

func renderCompactClosureRepairSection(repair types.RepairDirective) string {
	var b strings.Builder
	switch repair.Kind {
	case types.RepairReadFile:
		files := repair.Files
		if len(files) == 0 {
			return ""
		}
		limit := len(files)
		if limit > 2 {
			limit = 2
		}
		b.WriteString("## Forced Read List\n")
		b.WriteString("Read these blocking source file(s) before retrying completion:\n")
		for _, file := range files[:limit] {
			if strings.TrimSpace(file) == "" {
				continue
			}
			fmt.Fprintf(&b, "- `%s`\n", file)
		}
		if len(files) > limit {
			fmt.Fprintf(&b, "- ... and %d more blocking source(s)\n", len(files)-limit)
		}
	case types.RepairEmitEvidence:
		b.WriteString("## Evidence Materialization\n")
		if len(repair.Files) > 0 {
			limit := len(repair.Files)
			if limit > 2 {
				limit = 2
			}
			b.WriteString("Stay on these already-read anchor file(s) and re-emit grounded evidence:\n")
			for _, file := range repair.Files[:limit] {
				if strings.TrimSpace(file) == "" {
					continue
				}
				fmt.Fprintf(&b, "- `%s`\n", file)
			}
			if len(repair.Files) > limit {
				fmt.Fprintf(&b, "- ... and %d more already-read anchor file(s)\n", len(repair.Files)-limit)
			}
		} else {
			b.WriteString("Re-emit grounded evidence from the already-read anchor lines before retrying completion.\n")
		}
	case types.RepairExpandSearch:
		b.WriteString("## Search Coverage Gap\n")
		if len(repair.Keywords) > 0 {
			b.WriteString("Broaden the search with these keyword stems: ")
			b.WriteString(strings.Join(repair.Keywords, ", "))
			b.WriteString("\n")
		}
	case types.RepairSwapView:
		b.WriteString("## View Reconcile\n")
		if strings.TrimSpace(repair.Subject) != "" {
			fmt.Fprintf(&b, "Use the corrected QuestionFamily routing: %s.\n", repair.Subject)
		}
	case types.RepairRebindSubject:
		b.WriteString("## Subject Constraint\n")
		if strings.TrimSpace(repair.Subject) != "" {
			fmt.Fprintf(&b, "Keep the answer on `%s`-kind terminals only.\n", repair.Subject)
		}
	case types.RepairForceCompleteDowngrade:
		b.WriteString("## Force-Complete Downgrade\n")
		b.WriteString("No new navigation branch is changing the answer; close out with the current grounded evidence.\n")
	}
	return strings.TrimSpace(b.String())
}

func renderClosureRepairHint(repairs []types.RepairDirective) string {
	if len(repairs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("MID-LOOP CHECK: the last completion attempt already queued structured closure repairs. Finish the blocking repair first instead of returning to generic navigation.\n\n")
	limit := len(repairs)
	if limit > 2 {
		limit = 2
	}
	for i := 0; i < limit; i++ {
		rendered := renderCompactClosureRepairSection(repairs[i])
		if rendered == "" {
			continue
		}
		b.WriteString(rendered)
		b.WriteString("\n\n")
	}
	if len(repairs) > limit {
		fmt.Fprintf(&b, "... and %d more queued repair(s).\n\n", len(repairs)-limit)
	}
	b.WriteString("After one repair succeeds, re-emit grounded evidence if needed, then retry `emit_investigation_complete(reason, confidence, result_kind)`.")
	return strings.TrimSpace(b.String())
}

func (e *explorerEvaluator) awaitingClosureRepair(results []types.ToolResult) bool {
	if !e.midLoopClosureRepairSent || e.midLoopClosureRepairResultsLen == 0 {
		return false
	}
	return successfulToolCountSince(results, e.midLoopClosureRepairResultsLen, completionProgressToolNames) == 0
}

func (e *explorerEvaluator) syncClosureRepairState(results []types.ToolResult) {
	if !e.midLoopClosureRepairSent || e.midLoopClosureRepairResultsLen == 0 {
		return
	}
	if successfulToolCountSince(results, e.midLoopClosureRepairResultsLen, completionProgressToolNames) == 0 {
		return
	}
	e.midLoopClosureRepairSent = false
	e.midLoopClosureRepairResultsLen = 0
}

func (e *explorerEvaluator) postClosureRepairSignal(obs LoopObservation) LoopSignal {
	if e.midLoopClosureRepairSent || obs.LastToolResult == nil || obs.LastToolResult.ToolName != "emit_investigation_complete" {
		return LoopSignal{}
	}
	repairs := closureRepairDirectives(e.mutable)
	if len(repairs) == 0 {
		return LoopSignal{}
	}
	triggeredByDowngrade := obs.LastToolResult.Success && strings.HasPrefix(obs.LastToolResult.Summary, tool.EmitInvestigationCompleteDowngradePrefix)
	triggeredByRepair := !obs.LastToolResult.Success && obs.LastToolResult.Repair != nil
	triggeredByQueuedRepairs := !obs.LastToolResult.Success
	if !triggeredByDowngrade && !triggeredByRepair && !triggeredByQueuedRepairs {
		return LoopSignal{}
	}
	e.midLoopClosureRepairSent = true
	e.midLoopClosureRepairResultsLen = len(obs.AllToolResults)
	return LoopSignal{
		HintRequested:  true,
		HintKey:        "explorer.mid-loop.closure-repair",
		Hint:           renderClosureRepairHint(repairs),
		Progress:       true,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

func (e *explorerEvaluator) postClosureRepairClosureOnlySignal(obs LoopObservation) LoopSignal {
	if !e.awaitingClosureRepair(obs.AllToolResults) || e.investigationComplete {
		return LoopSignal{}
	}
	navCount := successfulToolCountSince(obs.AllToolResults, e.midLoopLastResultsLen, navigationToolNames)
	if navCount == 0 {
		return LoopSignal{}
	}
	if successfulToolCountSince(obs.AllToolResults, e.midLoopLastResultsLen, completionProgressToolNames) > 0 {
		return LoopSignal{}
	}
	repairs := closureRepairDirectives(e.mutable)
	message := "MID-LOOP CHECK: the last completion attempt already queued structured closure repairs, and the current batch still spent effort on generic navigation. Do not keep widening scope yet. Finish the queued repair first, then re-emit grounded evidence or retry `emit_investigation_complete(...)`."
	for _, repair := range repairs {
		if repair.Kind == types.RepairEmitEvidence {
			message = "MID-LOOP CHECK: the last completion attempt already identified an evidence-materialization repair on files you have already read. Do NOT read neighboring files yet. Stay on the queued repair target, emit a corrected grounded evidence batch from that existing anchor, then retry `emit_investigation_complete(...)`."
			break
		}
	}
	return LoopSignal{
		HintRequested:  true,
		HintKey:        fmt.Sprintf("explorer.mid-loop.closure-repair-closure-only.%d", obs.Iteration),
		Hint:           message,
		Progress:       true,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

type explorerCompletionReadiness struct {
	HasEnough                bool
	ERMSatisfied             bool
	ToolDiversity            bool
	FileCoverage             bool
	EvidenceQuality          bool
	NarrativeCarrier         bool
	AuthoritativeCoverage    bool
	AuthoritativeClosure     bool
	ExplanationAnchorReady   bool
	ExplanationAnchorCovered int
	ExplanationAnchorTotal   int
	ToolSources              int
	ReadCount                int
	DirectCount              int
	MinDirectCount           int
	ScopeReadCount           int
	ScopeTotalCount          int
	DiscoveredCount          int
	RelevantRead             int
	Coverage                 float64
	ReadyFaces               []string
	MissingFaces             []string
}

func (e *explorerEvaluator) strictEnumerationReadinessFloor() bool {
	if e == nil || !e.isEnumerationQuery {
		return false
	}
	if e.analysisIR == nil {
		// Preserve the legacy unit-test harness behavior when no
		// structured analyzer result is available.
		return true
	}
	rm := e.analysisIR.RequestModel
	if types.IsArchitectureNarrativeExplanation(rm) {
		return false
	}
	if types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) == types.ReqEnumeration {
		return true
	}
	if rm.Intent == types.IntentEnumerate || rm.Predicates.IsCategoryEnumeration {
		return true
	}
	return types.ResolveQuestionFamily(rm) == types.QFEnumeration
}

func (e *explorerEvaluator) groundedRequirementCarrierCount() int {
	if e == nil || len(e.structuredEvidence) == 0 {
		return 0
	}
	reqs := e.ermRequirements
	if len(reqs) == 0 && e.analysisIR != nil {
		if kind := types.NormalizeRequirementKind(e.analysisIR.RequestModel.AnalyzerHints.Kind); kind != types.ReqUnknown {
			reqs = []EvidenceRequirement{{Kind: kind}}
		}
	}
	if len(reqs) == 0 {
		return 0
	}
	count := 0
	for _, ev := range e.structuredEvidence {
		switch ev.GroundingStatus {
		case types.GroundingGrounded, types.GroundingRecovered, "":
		default:
			continue
		}
		for _, req := range reqs {
			reqEntities := e.filterRequirementEntitiesForShape(req.Entities)
			if types.EvidenceStructurallyMatchesRequirement(ev, req.Kind) &&
				evidenceMatchesRequirementEntities(ev, reqEntities) {
				count++
				break
			}
		}
	}
	return count
}

func (e *explorerEvaluator) filterRequirementEntitiesForShape(entities []string) []string {
	if e == nil || e.analysisIR == nil {
		return entities
	}
	return filterEntitiesByProvenance(
		entities,
		e.analysisIR.RequestModel.AnalyzerHints.EntityProvenance,
		entityProvenanceRoleShape,
	)
}

func cloneEvidenceRequirements(reqs []EvidenceRequirement) []EvidenceRequirement {
	if len(reqs) == 0 {
		return nil
	}
	out := make([]EvidenceRequirement, len(reqs))
	copy(out, reqs)
	return out
}

func (e *explorerEvaluator) completionReadiness(toolResults []types.ToolResult, sourceCount int, exactAbsenceSalvaged, mutateERM bool) explorerCompletionReadiness {
	discovered, readSet, _ := extractFileCoverage(toolResults, e.repoRoot)
	return e.completionReadinessWithCoverage(toolResults, sourceCount, exactAbsenceSalvaged, mutateERM, discovered, readSet)
}

func (e *explorerEvaluator) completionReadinessWithCoverage(toolResults []types.ToolResult, sourceCount int, exactAbsenceSalvaged, mutateERM bool, discovered []string, readSet map[string]bool) explorerCompletionReadiness {
	if readSet == nil {
		readSet = map[string]bool{}
	}
	scope := e.coverageScopeFiles(discovered, readSet, strings.Join(e.investigationNotes, "\n"))
	scopeReadCount, scopeCoverage, _ := coverageSnapshot(scope, readSet)
	coverage := 0.0
	if len(scope) > 0 {
		coverage = scopeCoverage
	} else if len(discovered) > 0 {
		coverage = float64(len(readSet)) / float64(len(discovered))
	}

	if sourceCount < 0 {
		sources := make(map[string]struct{})
		for _, r := range toolResults {
			if !r.Success {
				continue
			}
			if e.toolConfidence(r.ToolName) > 0.5 {
				sources[r.ToolName] = struct{}{}
			}
		}
		sourceCount = len(sources)
	}

	directCount := 0
	for _, item := range e.structuredEvidence {
		switch item.Kind {
		case types.EvidenceDirect, types.EvidenceRegistration:
			directCount++
		}
	}

	relevantRead := 0
	rankedFiles := e.rankerCoverageFilesForReadSet(readSet)
	if len(rankedFiles) > 0 {
		scoredSet := make(map[string]bool, len(rankedFiles))
		for _, f := range rankedFiles {
			scoredSet[f] = true
		}
		for f := range readSet {
			if scoredSet[f] {
				relevantRead++
			}
		}
	}

	toolDiversity := sourceCount >= 2
	fileCoverage := coverage >= 0.5 || len(readSet) >= 3
	authoritativeCoverage := e.authoritativeFailureCovered(toolResults)
	if authoritativeCoverage {
		fileCoverage = true
	}
	minDirect := 2
	evidenceQuality := directCount >= minDirect
	narrativeCarrier := e.narrativeClosureCarrierReady()
	if !e.strictEnumerationReadinessFloor() &&
		(hasGroundedTerminalEvidence(e.structuredEvidence) ||
			e.groundedRequirementCarrierCount() >= minDirect ||
			narrativeCarrier ||
			len(e.flowFindings) > 0) {
		evidenceQuality = true
	}
	authoritativeClosure := false
	if authoritativeCoverage {
		authoritativeClosure = e.authoritativeLogClosureCarrierReady()
		if authoritativeClosure {
			evidenceQuality = true
		}
	}
	if e.strictEnumerationReadinessFloor() {
		fileCoverage = coverage >= 0.8 || (len(scope) > 0 && scopeReadCount >= len(scope))
		minDirect = len(scope) / 3
		if minDirect == 0 {
			minDirect = len(discovered) / 3
		}
		if minDirect < 2 {
			minDirect = 2
		}
		evidenceQuality = directCount >= minDirect
	}
	if sourceCount == 1 {
		toolDiversity = true
		fileCoverage = true
		evidenceQuality = true
	}

	hasEnough := toolDiversity && fileCoverage && evidenceQuality
	ermSatisfied := len(e.ermRequirements) == 0
	if len(e.ermRequirements) > 0 {
		reqs := e.ermRequirements
		if !mutateERM {
			reqs = cloneEvidenceRequirements(e.ermRequirements)
		}
		reqs = checkRequirementSatisfaction(reqs, e.investigationNotes, e.structuredEvidence, e.complexity)
		ermSatisfied = ermAllSatisfied(reqs)
		if mutateERM {
			e.ermRequirements = reqs
		}
		if hasEnough && !ermSatisfied {
			hasEnough = false
		} else if !hasEnough && ermSatisfied && len(e.structuredEvidence) > 0 {
			hasEnough = true
		}
	}
	if exactAbsenceSalvaged && !hasEnough {
		hasEnough = true
	}
	explanationAnchorReady := true
	explanationAnchorCovered := 0
	explanationAnchorTotal := 0
	if e.analysisIR != nil && types.IRRequiresAnchorSkeleton(e.analysisIR) {
		if plan := e.answerSurfacePlan(); plan != nil {
			explanationAnchorCovered = len(plan.ExplanationAnchorBackbone)
			explanationAnchorTotal = explanationAnchorCovered + len(plan.ExplanationAnchorMissingTopics)
			if explanationAnchorTotal == 0 {
				explanationAnchorTotal = len(e.analysisIR.RequestModel.SubTopics)
			}
			explanationAnchorReady = len(plan.ExplanationAnchorMissingTopics) == 0 && explanationAnchorTotal > 0
		} else {
			explanationAnchorTotal = len(e.analysisIR.RequestModel.SubTopics)
			explanationAnchorReady = explanationAnchorTotal == 0
		}
		if !explanationAnchorReady {
			hasEnough = false
		}
	}
	if authoritativeClosure && e.driftBoundedCompletionReadyMode() {
		hasEnough = true
	}
	readyFaces, missingFaces := explorerReadinessFaces(
		toolDiversity,
		fileCoverage,
		evidenceQuality,
		len(e.ermRequirements) > 0,
		ermSatisfied,
		explanationAnchorTotal > 0,
		explanationAnchorReady,
		authoritativeClosure,
	)

	return explorerCompletionReadiness{
		HasEnough:                hasEnough,
		ERMSatisfied:             ermSatisfied,
		ToolDiversity:            toolDiversity,
		FileCoverage:             fileCoverage,
		EvidenceQuality:          evidenceQuality,
		NarrativeCarrier:         narrativeCarrier,
		AuthoritativeCoverage:    authoritativeCoverage,
		AuthoritativeClosure:     authoritativeClosure,
		ExplanationAnchorReady:   explanationAnchorReady,
		ExplanationAnchorCovered: explanationAnchorCovered,
		ExplanationAnchorTotal:   explanationAnchorTotal,
		ToolSources:              sourceCount,
		ReadCount:                len(readSet),
		DirectCount:              directCount,
		MinDirectCount:           minDirect,
		ScopeReadCount:           scopeReadCount,
		ScopeTotalCount:          len(scope),
		DiscoveredCount:          len(discovered),
		RelevantRead:             relevantRead,
		Coverage:                 coverage,
		ReadyFaces:               readyFaces,
		MissingFaces:             missingFaces,
	}
}

func (e *explorerEvaluator) needsStructuredMemberSetHandoff(ctx *types.AgentContext) bool {
	var ir *types.AnalysisIR
	if ctx != nil && ctx.AnalysisIR != nil {
		ir = ctx.AnalysisIR
	} else if e != nil {
		ir = e.analysisIR
	}
	if ir == nil {
		return false
	}
	rm := ir.RequestModel
	if !types.RequiresExhaustiveEnumerationMemberSetHandoff(rm) {
		return false
	}
	var view *types.AnswerSemanticView
	if ctx != nil {
		view = types.BuildAnswerSemanticViewForAgentContext(ctx)
	} else if e != nil && e.mutable != nil {
		view = types.BuildAnswerSemanticViewForBusContext(&types.BusContext{
			AnalysisIR: ir,
			Mutable:    e.mutable,
		})
	} else {
		view = types.BuildAnswerSemanticView(ir, nil)
	}
	return view != nil && view.Family == types.QFEnumeration && view.NeedsEnumerationSlate()
}

func acceptedStructuredMemberSetHandoff(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	for _, fact := range ctx.Mutable.StableInvestigationAggregateFacts() {
		if fact.Kind == types.AnswerAggregateMemberSet && len(fact.Members) > 0 {
			return true
		}
	}
	return false
}

func explorerReadinessFaces(
	toolDiversity bool,
	fileCoverage bool,
	evidenceQuality bool,
	ermApplicable bool,
	ermSatisfied bool,
	explanationAnchorApplicable bool,
	explanationAnchorReady bool,
	authoritativeClosure bool,
) (ready []string, missing []string) {
	add := func(ok bool, label string) {
		if ok {
			ready = append(ready, label)
			return
		}
		missing = append(missing, label)
	}
	add(toolDiversity, "tool sources")
	add(fileCoverage, "file coverage")
	add(evidenceQuality, "answer evidence")
	if ermApplicable {
		add(ermSatisfied, "current evidence requirements")
	}
	if explanationAnchorApplicable {
		add(explanationAnchorReady, "topic anchors")
	}
	if authoritativeClosure {
		ready = append(ready, "current-branch failure path")
	}
	return ready, missing
}

func (e *explorerEvaluator) postCompletionReadySignal(obs LoopObservation) LoopSignal {
	if e.midLoopCompletionReadySent || e.phase != 1 || e.investigationComplete {
		return LoopSignal{}
	}
	if obs.Iteration < e.heuristics.MidLoopMinIteration {
		return LoopSignal{}
	}
	if !hasSuccessfulTool(obs.AllToolResults, "emit_evidence") {
		return LoopSignal{}
	}
	if e.exactResolution != nil && e.exactResolution.AllowAbsence {
		targets := e.exactPendingTargets
		if len(targets) == 0 {
			targets = e.exactResolution.Targets
		}
		if exactAbsenceClosureReady(e.exactResolution, e.scenario, targets, e.structuredEvidence, e.exactContextFiles) {
			return LoopSignal{}
		}
	}
	readiness := e.completionReadiness(obs.AllToolResults, -1, false, false)
	if !readiness.HasEnough {
		return LoopSignal{}
	}
	scalarLocateReady := e.scalarRoleLocateClosureReady()
	if !hasTerminalEvidence(e.structuredEvidence) &&
		len(e.flowFindings) == 0 &&
		!hasGroundedRequirementCarrier(e.structuredEvidence, e.ermRequirements) &&
		!readiness.NarrativeCarrier &&
		!readiness.AuthoritativeClosure &&
		!scalarLocateReady {
		return LoopSignal{}
	}
	e.midLoopCompletionReadySent = true
	e.midLoopCompletionReadyIter = obs.Iteration
	var b strings.Builder
	b.WriteString("MID-LOOP CHECK: you already have enough evidence to answer this question on the current branch. ")
	b.WriteString("Stop widening scope and close the investigation now with `emit_investigation_complete(reason, confidence, result_kind)` instead of reading more neighboring files. Put the concise conclusion and any important boundary in `reason`. ")
	b.WriteString("Use `result_kind=\"resolved\"` unless this is a genuine honest-zero / not-found answer.\n")
	if len(e.ermRequirements) > 0 && readiness.ERMSatisfied {
		b.WriteString("- all current evidence requirements are satisfied\n")
	}
	if readiness.AuthoritativeClosure {
		b.WriteString("- authoritative log frames already carry grounded call/mechanism anchors on the current branch\n")
	} else {
		fmt.Fprintf(&b, "- evidence-bearing tool sources: %d\n", readiness.ToolSources)
		fmt.Fprintf(&b, "- direct/registration evidence items: %d\n", readiness.DirectCount)
		if readiness.ScopeTotalCount > 0 {
			fmt.Fprintf(&b, "- scoped file coverage: %d / %d\n", readiness.ScopeReadCount, readiness.ScopeTotalCount)
		} else {
			fmt.Fprintf(&b, "- files read: %d\n", readiness.ReadCount)
		}
	}
	if e.driftBoundedCompletionReadyMode() && readiness.AuthoritativeClosure {
		b.WriteString("- the current checkout already bounds the answer surface; do not reopen older-build-only or upstream-provenance branches unless a new contradiction appears on this same grounded path\n")
		if reason := e.driftBoundedCompletionHintReason(); reason != "" {
			fmt.Fprintf(&b, "- if you close now, keep `reason` no stronger than: %s\n", reason)
		}
	}
	if scalarLocateReady {
		b.WriteString("- a grounded owner / definition anchor already identifies the requested literal and its source location\n")
	}
	if readiness.NarrativeCarrier {
		b.WriteString("- architecture/mechanism explanation has enough grounded defining/mechanism carriers for the requested narrative shape\n")
	}
	if readiness.ExplanationAnchorTotal > 0 {
		fmt.Fprintf(&b, "- topic anchors ready: %d / %d\n",
			readiness.ExplanationAnchorCovered, readiness.ExplanationAnchorTotal)
	}
	if len(readiness.ReadyFaces) > 0 {
		fmt.Fprintf(&b, "- answer-ready faces: %s\n", strings.Join(readiness.ReadyFaces, ", "))
	}
	if e.needsStructuredMemberSetHandoff(nil) {
		b.WriteString("- this is an exhaustive principal-member enumeration; your successful close must include `aggregate_facts` with kind=`member_set`, numeric `value`, and every principal member in `members`\n")
	}
	b.WriteString("Only continue reading if one specific unresolved branch would still change the final answer.")
	return LoopSignal{
		HintRequested:  true,
		HintKey:        "explorer.mid-loop.completion-ready",
		Hint:           b.String(),
		Progress:       true,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

func (e *explorerEvaluator) authoritativeTier1Readiness() explorerTier1Readiness {
	status := explorerTier1Readiness{Ready: true}
	if e == nil || len(e.structuredEvidence) == 0 {
		return status
	}
	policy := tool.CurrentGroundingPolicy()
	status.Floor = policy.Tier1Floor
	if policy.Tier1Floor <= 0 {
		return status
	}
	stableAbsent := false
	requiredFiles := e.exactContextFiles
	if e.exactResolution != nil && e.exactResolution.AllowAbsence {
		targets := e.exactPendingTargets
		if len(targets) == 0 {
			targets = e.exactResolution.Targets
		}
		stableAbsent = exactAbsenceClosureReady(e.exactResolution, e.scenario, targets, e.structuredEvidence, requiredFiles)
	}
	rm := types.RequestModel{}
	if e.analysisIR != nil {
		rm = e.analysisIR.RequestModel
	}
	var repairItems []types.EvidenceItem
	for _, ev := range e.structuredEvidence {
		if !types.EvidenceCountsTowardTier1FloorInContext(ev, e.exactResolution, e.scenario, stableAbsent, requiredFiles, rm) {
			continue
		}
		status.Total++
		switch ev.GroundingStatus {
		case types.GroundingGrounded:
			if ev.GroundingTier == types.TierLineText || ev.GroundingTier == "" {
				status.Tier1++
			} else {
				repairItems = append(repairItems, ev)
			}
		case types.GroundingRecovered:
			repairItems = append(repairItems, ev)
		case "":
			status.Tier1++
		}
	}
	if status.Total == 0 {
		return status
	}
	status.Ready = float64(status.Tier1)/float64(status.Total) >= policy.Tier1Floor
	status.Targets = tool.BuildTier1RepairTargets(e.repoRoot, repairItems)
	return status
}

func (e *explorerEvaluator) postAuthoritativeTier1CompletionSignal(obs LoopObservation) LoopSignal {
	if e == nil || e.phase != 1 || e.investigationComplete || e.midLoopAuthoritativeTier1Sent {
		return LoopSignal{}
	}
	if obs.Iteration < e.heuristics.MidLoopMinIteration {
		return LoopSignal{}
	}
	if !hasSuccessfulTool(obs.AllToolResults, "emit_evidence") {
		return LoopSignal{}
	}
	readiness := e.completionReadiness(obs.AllToolResults, -1, false, false)
	if !readiness.HasEnough || !readiness.AuthoritativeCoverage {
		return LoopSignal{}
	}
	tier1 := e.authoritativeTier1Readiness()
	if tier1.Ready || tier1.Total == 0 {
		return LoopSignal{}
	}
	e.midLoopAuthoritativeTier1Sent = true
	var b strings.Builder
	fmt.Fprintf(&b,
		"MID-LOOP CHECK: the current authoritative log path is already semantically enough to answer, but `emit_investigation_complete` would still be rejected by the line-text grounding floor (currently %d/%d = %.0f%%, need ≥ %.0f%%). Before closing, convert the load-bearing failure anchors to `read_file`-grounded line_text evidence on the CURRENT branch.\n",
		tier1.Tier1, tier1.Total, float64(tier1.Tier1)*100/float64(tier1.Total), tier1.Floor*100)
	b.WriteString("Do NOT widen into more neighboring files yet. Re-read the current authoritative sources near the cited lines, then re-emit ONE tighter `emit_evidence(items=[...])` batch that keeps the failure path grounded first. Related context and setup/background anchors can wait until after the principal anchors pass the line-text grounding floor.\n")
	if len(tier1.Targets) > 0 {
		maxList := 4
		if maxList > len(tier1.Targets) {
			maxList = len(tier1.Targets)
		}
		b.WriteString("Suggested repair targets:\n")
		for i := 0; i < maxList; i++ {
			target := tier1.Targets[i]
			lines := tool.Tier1LineList(target.Lines, 4)
			if lines == "" {
				fmt.Fprintf(&b, "  - `%s`\n", target.File)
				continue
			}
			fmt.Fprintf(&b, "  - `%s` near lines %s\n", target.File, lines)
		}
		if len(tier1.Targets) > maxList {
			fmt.Fprintf(&b, "  - ... and %d more current-branch source(s)\n", len(tier1.Targets)-maxList)
		}
	}
	return LoopSignal{
		HintRequested:  true,
		HintKey:        "explorer.mid-loop.authoritative-tier1-before-complete",
		Hint:           b.String(),
		Progress:       true,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

func (e *explorerEvaluator) narrativeClosureCarrierReady() bool {
	if e == nil || e.analysisIR == nil || len(e.structuredEvidence) == 0 {
		return false
	}
	rm := e.analysisIR.RequestModel
	architectureNarrative := types.IsArchitectureNarrativeExplanation(rm)
	singleMechanism := types.IsSingleTopicMechanismExplanation(rm)
	if !architectureNarrative && !singleMechanism {
		return false
	}
	if types.RequiresExhaustiveEnumerationMemberSetHandoff(rm) {
		return false
	}
	minCarriers := 2
	if architectureNarrative {
		minCarriers = 3
	}
	carriers := 0
	sources := make(map[string]bool)
	for _, ev := range e.structuredEvidence {
		if !narrativeClosureEvidenceCarrier(ev) {
			continue
		}
		carriers++
		if source := canonicalEvidenceSourcePath(ev.Source); source != "" {
			sources[source] = true
		}
	}
	if carriers < minCarriers {
		return false
	}
	if architectureNarrative && len(sources) < 2 && carriers < minCarriers+1 {
		return false
	}
	return true
}

func narrativeClosureEvidenceCarrier(ev types.EvidenceItem) bool {
	switch ev.GroundingStatus {
	case types.GroundingGrounded, types.GroundingRecovered, "":
	default:
		return false
	}
	switch ev.ContextRole {
	case types.EvidenceContextRoleIllustrativeOnly, types.EvidenceContextRoleAbsenceSupport:
		return false
	}
	switch ev.Kind {
	case types.EvidenceMechanism, types.EvidenceRelationship, types.EvidenceRegistration:
		return true
	case types.EvidenceDirect:
		switch ev.AnchorKind {
		case types.AnchorDefinition, types.AnchorCall, types.AnchorCondition,
			types.AnchorInitializer, types.AnchorAssignment, types.AnchorReturn:
			return true
		}
	}
	return false
}

func (e *explorerEvaluator) scalarRoleLocateClosureReady() bool {
	if e == nil || e.analysisIR == nil {
		return false
	}
	plan := e.answerSurfacePlan()
	if (plan == nil || plan.SummarySurfaceMode != types.AnswerSummarySurfaceMinimalScalarRoleLocate) &&
		!types.IsScalarSourceLiteralLookup(e.analysisIR.RequestModel) {
		return false
	}
	for _, ev := range e.structuredEvidence {
		switch ev.GroundingStatus {
		case types.GroundingGrounded, types.GroundingRecovered:
		default:
			continue
		}
		switch ev.ContextRole {
		case types.EvidenceContextRoleIllustrativeOnly, types.EvidenceContextRoleAbsenceSupport:
			continue
		}
		if scalarRoleLocateEvidenceReady(ev) {
			return true
		}
	}
	return false
}

func scalarRoleLocateEvidenceReady(ev types.EvidenceItem) bool {
	if strings.TrimSpace(ev.Source) == "" || ev.LineStart <= 0 {
		return false
	}
	if ev.Kind == types.EvidenceRegistration || isRegistrationShape(ev) {
		return true
	}
	if ev.Kind == types.EvidenceConcrete {
		switch ev.Predicate {
		case "returns", "maps":
			return true
		}
	}
	if ev.Kind == types.EvidenceDirect && ev.AnchorKind == types.AnchorDefinition {
		return true
	}
	return false
}

type authoritativeFrameRef struct {
	File string
	Tail string
	Func string
}

type explorerTier1Readiness struct {
	Ready   bool
	Floor   float64
	Tier1   int
	Total   int
	Targets []tool.Tier1RepairTarget
}

func (e *explorerEvaluator) authoritativeLogClosureCarrierReady() bool {
	if e == nil || e.logTriage == nil || len(e.logTriage.ResolvedFiles) == 0 || len(e.structuredEvidence) == 0 {
		return false
	}
	frames := e.authoritativeResolvedFrames()
	if len(frames) == 0 {
		return false
	}
	allowed := make(map[string]bool, len(e.logTriage.ResolvedFiles))
	for _, f := range e.logTriage.ResolvedFiles {
		canon := canonicalExplorerPath(f)
		if canon == "" {
			canon = f
		}
		if canon != "" {
			allowed[canon] = true
		}
	}
	if len(allowed) == 0 {
		return false
	}

	mechanismByFailure := make(map[string]bool, len(frames))
	callByFailure := make(map[string]bool, len(frames))
	for _, ev := range e.structuredEvidence {
		switch ev.GroundingStatus {
		case types.GroundingGrounded, types.GroundingRecovered:
		default:
			continue
		}
		switch ev.ContextRole {
		case types.EvidenceContextRoleIllustrativeOnly, types.EvidenceContextRoleAbsenceSupport:
			continue
		}
		source := canonicalExplorerPath(ev.Source)
		if source == "" {
			source = ev.Source
		}
		if source == "" || !allowed[source] {
			continue
		}

		tails := evidenceSurfaceTailSet(ev)
		if len(tails) == 0 {
			continue
		}

		for i, failure := range frames {
			failureKey := failure.File + "\x00" + failure.Tail
			if types.EvidenceStructurallyMatchesRequirement(ev, types.ReqCallChain) &&
				evidenceBindsAuthoritativeFailureCall(ev, tails, frames, i, source) {
				callByFailure[failureKey] = true
			}
			if evidenceBindsAuthoritativeFailureMechanism(ev, tails, frames, i, source) {
				mechanismByFailure[failureKey] = true
			}
			if callByFailure[failureKey] && mechanismByFailure[failureKey] {
				return true
			}
		}
	}
	return false
}

func evidenceSurfaceTailSet(ev types.EvidenceItem) map[string]bool {
	tails := types.EvidenceSurfaceSymbolTails(ev)
	if len(tails) == 0 {
		return nil
	}
	out := make(map[string]bool, len(tails))
	for _, tail := range tails {
		if tail == "" {
			continue
		}
		out[tail] = true
	}
	return out
}

func evidenceBindsAuthoritativeFailureCall(
	ev types.EvidenceItem,
	tails map[string]bool,
	frames []authoritativeFrameRef,
	idx int,
	source string,
) bool {
	if idx < 0 || idx >= len(frames) {
		return false
	}
	failure := frames[idx]
	if !tails[failure.Tail] {
		return false
	}
	if len(frames) == 1 {
		return source == failure.File
	}
	if idx+1 >= len(frames) {
		return false
	}
	caller := frames[idx+1]
	return source == caller.File && tails[caller.Tail]
}

func evidenceBindsAuthoritativeFailureMechanism(
	ev types.EvidenceItem,
	tails map[string]bool,
	frames []authoritativeFrameRef,
	idx int,
	source string,
) bool {
	if idx < 0 || idx >= len(frames) {
		return false
	}
	if !(types.EvidenceStructurallyMatchesRequirement(ev, types.ReqConditional) ||
		ev.AnchorKind == types.AnchorDefinition ||
		ev.AnchorKind == types.AnchorReturn ||
		ev.AnchorKind == types.AnchorAssignment ||
		ev.AnchorKind == types.AnchorInitializer) {
		return false
	}
	failure := frames[idx]
	if source == failure.File && tails[failure.Tail] {
		return true
	}
	if idx+1 >= len(frames) {
		return false
	}
	caller := frames[idx+1]
	return source == caller.File && tails[caller.Tail] && tails[failure.Tail]
}

func (e *explorerEvaluator) postExplanationAnchorSignal(obs LoopObservation) LoopSignal {
	if e.midLoopExplanationAnchorSent || e.phase != 1 || e.investigationComplete || e.analysisIR == nil {
		return LoopSignal{}
	}
	if !types.IRRequiresAnchorSkeleton(e.analysisIR) {
		return LoopSignal{}
	}
	if obs.Iteration < e.heuristics.MidLoopMinIteration {
		return LoopSignal{}
	}
	if !hasSuccessfulTool(obs.AllToolResults, "emit_evidence") {
		return LoopSignal{}
	}
	plan := e.answerSurfacePlan()
	if plan == nil {
		return LoopSignal{}
	}
	anchors := plan.ExplanationAnchorBackbone
	missing := plan.ExplanationAnchorMissingTopics
	claim := plan.ExplanationAnchorCompleteness
	if len(anchors) == 0 || len(missing) == 0 {
		return LoopSignal{}
	}
	e.midLoopExplanationAnchorSent = true
	var b strings.Builder
	b.WriteString("MID-LOOP CHECK: this is a multi-topic explanation answer, and the current evidence still lacks one grounded anchor line per sub-topic. Before closing, make sure each sub-topic has one load-bearing symbol/field/owner line that the next pass can turn into the Key Anchors skeleton.\n")
	total := len(anchors) + len(missing)
	if total == 0 {
		total = len(e.analysisIR.RequestModel.SubTopics)
	}
	fmt.Fprintf(&b, "- grounded anchor coverage: %d / %d sub-topics\n", len(anchors), total)
	switch claim {
	case types.CompletenessLowerBound:
		b.WriteString("- current anchor coverage is only a lower bound\n")
	case types.CompletenessUnknown:
		b.WriteString("- current anchor coverage is still incomplete\n")
	}
	b.WriteString("- missing sub-topics:\n")
	for _, topic := range missing {
		fmt.Fprintf(&b, "  - %s\n", topic)
	}
	b.WriteString("Read the exact definition/owner line for the missing sub-topic(s), emit grounded evidence from those lines, then call `emit_investigation_complete(...)`.")
	return LoopSignal{
		HintRequested:  true,
		HintKey:        "explorer.mid-loop.explanation-anchor-skeleton",
		Hint:           b.String(),
		Progress:       true,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

func (e *explorerEvaluator) postCompletionReadyEscalationSignal(obs LoopObservation) LoopSignal {
	if !e.midLoopCompletionReadySent || e.midLoopCompletionReadyEscalated || e.investigationComplete {
		return LoopSignal{}
	}
	if e.midLoopCompletionReadyIter <= 0 || obs.Iteration < e.midLoopCompletionReadyIter+2 {
		return LoopSignal{}
	}
	e.midLoopCompletionReadyEscalated = true
	hint := "MID-LOOP CHECK: an earlier hint already established that the current evidence is sufficient. Do NOT reopen adjacent files or widen scope by default. Either call `emit_investigation_complete(reason, confidence, result_kind)` now, or verify exactly one concrete unresolved branch only if that branch could still change the final answer."
	if e.driftBoundedCompletionReadyMode() {
		hint = "MID-LOOP CHECK: an earlier hint already established that the current checkout already explains the grounded failure path. Do NOT reopen upstream-caller or older-build-only branches by default. Either call `emit_investigation_complete(reason, confidence, result_kind)` now, or verify exactly one contradiction only if it would change the grounded current-branch answer."
	}
	return LoopSignal{
		HintRequested:  true,
		HintKey:        "explorer.mid-loop.completion-ready-escalated",
		Hint:           hint,
		Progress:       true,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

func (e *explorerEvaluator) postCompletionReadyClosureOnlySignal(obs LoopObservation) LoopSignal {
	if !e.midLoopCompletionReadySent || e.investigationComplete {
		return LoopSignal{}
	}
	navCount := successfulToolCountSince(obs.AllToolResults, e.midLoopLastResultsLen, navigationToolNames)
	if navCount == 0 {
		return LoopSignal{}
	}
	if successfulToolCountSince(obs.AllToolResults, e.midLoopLastResultsLen, completionProgressToolNames) > 0 {
		return LoopSignal{}
	}
	fastTrack := !e.midLoopCompletionReadyEscalated && e.driftBoundedCompletionReadyMode()
	if !e.midLoopCompletionReadyEscalated && !fastTrack {
		return LoopSignal{}
	}
	if fastTrack {
		e.midLoopCompletionReadyEscalated = true
	}
	hint := "MID-LOOP CHECK: completion-ready has already been established and escalated. The current batch still spent effort on navigation tools. Do NOT keep calling `read_file`, `grep`, `repo_map`, `list_files`, or `exec_command` unless this batch surfaced one concrete contradiction that would change the final answer. From here, either emit exactly one repair batch for that contradiction or call `emit_investigation_complete(reason, confidence, result_kind)` now."
	if fastTrack {
		hint = "MID-LOOP CHECK: completion-ready is already established for the grounded current branch, and this batch reopened navigation anyway. Do NOT keep tracing upstream-provenance or older-build-only branches from here. Either emit exactly one repair batch for a concrete contradiction from the lines you already opened, or call `emit_investigation_complete(reason, confidence, result_kind)` now."
		if reason := e.driftBoundedCompletionHintReason(); reason != "" {
			hint += " Reuse this bounded `reason` surface (or a weaker one): " + reason
		}
	}
	return LoopSignal{
		HintRequested:  true,
		HintKey:        fmt.Sprintf("explorer.mid-loop.completion-ready-closure-only.%d", obs.Iteration),
		Hint:           hint,
		Progress:       true,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

func (e *explorerEvaluator) driftBoundedCompletionReadyMode() bool {
	if e == nil {
		return false
	}
	if !e.authoritativeLogClosureCarrierReady() || e.analysisIR == nil ||
		e.analysisIR.RequestModel.Scenario != types.ScenarioRootCause {
		return false
	}
	view := types.BuildAnswerSemanticView(e.analysisIR, e.answerSurfacePlan())
	if view == nil {
		return false
	}
	// Drift-bounded long-form rendering applies to families that emit
	// either a principal ordered hop list (call_chain / root_cause_trace)
	// or a long-form prose narrative (architecture / generic). The
	// scalar / role-lookup / config-precedence / enumeration families
	// are excluded because their principal payload is not a narrative.
	if !view.NeedsOrderedPrincipalList() &&
		view.Family != types.QFArchitecture && view.Family != types.QFGeneric {
		return false
	}
	if plan := e.answerSurfacePlan(); plan != nil && len(plan.DriftBoundedSurfaceItems) > 0 {
		return true
	}
	_, _, items := e.currentDriftBoundedSurface()
	return len(items) > 0
}

func (e *explorerEvaluator) driftBoundedCompletionReason() string {
	if e == nil {
		return ""
	}
	plan := e.answerSurfacePlan()
	lang := ""
	if e.analysisIR != nil {
		lang = strings.TrimSpace(e.analysisIR.AnswerContract.Language)
		if lang == "" {
			lang = strings.TrimSpace(e.analysisIR.RequestModel.Language)
		}
	}
	if plan == nil || len(plan.DriftBoundedSurfaceItems) == 0 {
		observed, drift, items := e.currentDriftBoundedSurface()
		if len(items) == 0 {
			return ""
		}
		plan = &types.AnswerSurfacePlan{
			SummarySurfaceMode:       types.AnswerSummarySurfaceDriftBoundedRootCause,
			LogObservedAnchors:       observed,
			LogSourceDriftAnchors:    drift,
			DriftBoundedSurfaceItems: items,
		}
	}
	if reason := strings.TrimSpace(tool.RenderDriftBoundedCurrentRootCauseSummary(plan, lang)); reason != "" {
		return reason
	}
	return renderFallbackDriftBoundedCompletionReason(plan)
}

func (e *explorerEvaluator) driftBoundedCompletionHintReason() string {
	if e == nil {
		return ""
	}
	plan := e.answerSurfacePlan()
	if plan == nil || len(plan.DriftBoundedSurfaceItems) == 0 {
		observed, drift, items := e.currentDriftBoundedSurface()
		if len(items) == 0 {
			return ""
		}
		plan = &types.AnswerSurfacePlan{
			SummarySurfaceMode:       types.AnswerSummarySurfaceDriftBoundedRootCause,
			LogObservedAnchors:       observed,
			LogSourceDriftAnchors:    drift,
			DriftBoundedSurfaceItems: items,
		}
	}
	if reason := renderFallbackDriftBoundedCompletionReason(plan); reason != "" {
		return reason
	}
	return e.driftBoundedCompletionReason()
}

func (e *explorerEvaluator) currentDriftBoundedSurface() ([]types.LogSourceDriftAnchor, []types.LogSourceDriftAnchor, []types.EvidenceItem) {
	if e == nil || e.analysisIR == nil || (e.logTriage == nil && e.perfTrace == nil) {
		return nil, nil, nil
	}
	observed := types.CollectArtifactObservedAnchors(e.analysisIR.RequestModel, e.logTriage, e.perfTrace, e.structuredEvidence)
	drift := types.CollectArtifactSourceDriftAnchors(e.analysisIR.RequestModel, e.logTriage, e.perfTrace, e.structuredEvidence)
	items := types.CollectDriftBoundedSurfaceItems(observed, drift, e.structuredEvidence)
	return observed, drift, items
}

func renderFallbackDriftBoundedCompletionReason(plan *types.AnswerSurfacePlan) string {
	if plan == nil || len(plan.DriftBoundedSurfaceItems) == 0 {
		return ""
	}
	clauses := make([]string, 0, 2)
	for _, item := range plan.DriftBoundedSurfaceItems {
		clause := strings.TrimSpace(types.EvidenceStructuredSemanticLine(item, false))
		if clause == "" {
			for _, candidate := range []string{item.AnchorSymbol, item.Subject, item.Object} {
				if candidate = strings.TrimSpace(candidate); candidate != "" {
					clause = candidate
					break
				}
			}
		}
		if clause == "" {
			continue
		}
		if item.Source != "" && item.LineStart > 0 {
			clause = fmt.Sprintf("%s (%s:%d)", clause, strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`)), item.LineStart)
		}
		clauses = append(clauses, clause)
		if len(clauses) >= 2 {
			break
		}
	}
	switch len(clauses) {
	case 0:
		return ""
	case 1:
		return "Stay within the grounded current-branch anchor: " + clauses[0] + "."
	default:
		return "Stay within the grounded current-branch anchors: " + clauses[0] + "; " + clauses[1] + "."
	}
}

func (e *explorerEvaluator) updateMidLoopSerialStreak(prevResultsLen, currentResultsLen int) {
	if currentResultsLen < 0 {
		currentResultsLen = 0
	}
	if prevResultsLen < 0 {
		prevResultsLen = 0
	}
	if prevResultsLen > currentResultsLen {
		prevResultsLen = currentResultsLen
	}
	currentBatch := currentResultsLen - prevResultsLen
	serialThresh := e.heuristics.SerialBatchThreshold
	if prevResultsLen > 0 && currentBatch <= serialThresh {
		e.midLoopSerialStreak++
	} else if currentBatch > serialThresh {
		e.midLoopSerialStreak = 0
	}
}

func (e *explorerEvaluator) finalizeMidLoopBatch(prevResultsLen, currentResultsLen int) {
	if currentResultsLen < 0 {
		currentResultsLen = 0
	}
	if prevResultsLen < 0 {
		prevResultsLen = 0
	}
	if prevResultsLen > currentResultsLen {
		prevResultsLen = currentResultsLen
	}
	// Consume the current tool-result batch even when observeMidLoop exits
	// early through a high-priority hint. Otherwise the next iteration
	// still compares against a stale baseline, which can hide the exact
	// “you only navigated after a repair/completion cue” follow-up we want
	// to emit.
	e.midLoopLastResultsLen = currentResultsLen
}

func exactAbsenceClosureReady(contract *types.ExactResolutionContract, scenario types.Scenario, targets []string, evidence []types.EvidenceItem, requiredFiles []string) bool {
	return types.ExactResolutionAbsenceClosureReady(contract, scenario, targets, evidence, requiredFiles)
}

func (e *explorerEvaluator) salvageExactAbsenceCompletion(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil {
		return false
	}
	if ctx.Mutable.IsInvestigationComplete() || strings.TrimSpace(ctx.Mutable.StableInvestigationResultKind()) != "" {
		return false
	}
	contract := ctx.AnalysisIR.AnswerContract.ExactResolution
	if contract == nil || !contract.AllowAbsence || len(contract.Targets) == 0 {
		return false
	}
	requiredFiles := ctx.Mutable.ExactContextRequiredFiles()
	if len(requiredFiles) == 0 {
		requiredFiles = e.exactContextFiles
	}
	if !exactAbsenceClosureReady(contract, e.scenario, contract.Targets, e.structuredEvidence, requiredFiles) {
		return false
	}
	just := types.ExactResolutionAutoAbsenceJustification(contract)
	if just == "" {
		return false
	}
	ctx.Mutable.SetInvestigationResultKind("absence")
	ctx.Mutable.SetAbsenceJustification(just)
	logging.Info("[explorer] salvaged exact-absence completion from structured evidence (scenario=%s targets=%v)", e.scenario, contract.Targets)
	return true
}

func (e *explorerEvaluator) postPrimaryReadMidLoopSignal(obs LoopObservation) LoopSignal {
	if e.phase != 1 || e.midLoopPostPrimaryInjected || !e.primaryReadSeen {
		return LoopSignal{}
	}
	if len(e.investigationNotes) > e.notesLenAtPrimaryRead {
		return LoopSignal{}
	}
	// Only fire immediately after the first anchor-file read, before
	// the model has had a chance to drift into a text-only soft-stop.
	if obs.Iteration > e.primaryReadIter+1 {
		return LoopSignal{}
	}
	if e.searchResult == nil || e.searchResult.Graph == nil {
		return LoopSignal{}
	}
	_, readSet, _ := extractFileCoverage(obs.AllToolResults, e.repoRoot)
	if len(readSet) == 0 {
		return LoopSignal{}
	}
	if hint := e.authoritativeFrameRealignmentHint(obs.AllToolResults); hint != "" {
		e.midLoopPostPrimaryInjected = true
		return LoopSignal{
			HintRequested: true,
			HintKey:       "explorer.mid-loop.post-primary-read",
			Hint:          hint,
			Progress:      true,
		}
	}
	if e.scalarSourceLiteralPrimaryReadMode() {
		e.midLoopPostPrimaryInjected = true
		return LoopSignal{
			HintRequested: true,
			HintKey:       "explorer.mid-loop.post-primary-read",
			Hint: "MID-LOOP CHECK: this is a scalar source-literal lookup and you just reached the primary anchor file. " +
				"Do NOT switch into full function walkthrough mode by default. First emit grounded evidence for the owner / definition line that identifies the requested literal and its source location. " +
				"Only keep reading if that line still does not determine the answer after the evidence batch.",
			Progress: true,
		}
	}
	if partials := detectPartiallyReadSymbols(obs.AllToolResults, e.searchResult.Graph); len(partials) > 0 {
		partials = e.filterPartialReadsForPostPrimary(partials)
		if len(partials) == 0 {
			return LoopSignal{}
		}
		allowed := e.activeFrontierFileSet(readSet, "")
		var chosen partialReadHint
		found := false
		for _, partial := range partials {
			if len(allowed) > 0 && !allowed[canonicalExplorerPath(partial.file)] {
				continue
			}
			chosen = partial
			found = true
			break
		}
		if !found {
			chosen = partials[0]
		}
		e.midLoopPostPrimaryInjected = true
		traceSupplement := e.orderedSameFileTracePartialReadHint()
		return LoopSignal{
			HintRequested: true,
			HintKey:       "explorer.mid-loop.post-primary-read",
			Hint: "MID-LOOP CHECK: you just reached the primary anchor file. Do NOT stop with a prose summary yet. " +
				"Keep using tools and finish the most relevant unread code first.\n" +
				renderPartialReadHint(chosen, e.heuristics.PartialReadLineThreshold) +
				traceSupplement,
			Progress: true,
		}
	}

	var b strings.Builder
	b.WriteString("MID-LOOP CHECK: you just reached the primary anchor file. Do NOT stop with a prose summary yet. Continue with tools.\n")
	if obs.LastToolResult != nil && obs.LastToolResult.ToolName == "read_file" {
		b.WriteString("First, emit grounded evidence from what you just read if you have not done so yet. ")
	} else {
		b.WriteString("You already have first-pass evidence from the anchor. ")
	}

	anchor, _ := e.uniqueExactAnchorFile()
	localHops, externalHops := e.primaryAnchorNextHops()
	if len(localHops) > 0 {
		b.WriteString(renderAnchorLocalGroundingHint(anchor, localHops))
		e.midLoopPostPrimaryInjected = true
		return LoopSignal{
			HintRequested: true,
			HintKey:       "explorer.mid-loop.post-primary-read",
			Hint:          b.String(),
			Progress:      true,
		}
	}

	unread := unreadAnchorExternalTargetFiles(readSet, externalHops)
	if len(unread) == 0 {
		unread = e.unreadActiveFrontierFiles(readSet)
	}
	if len(unread) == 0 {
		return LoopSignal{}
	}
	maxList := 3
	if len(unread) < maxList {
		maxList = len(unread)
	}
	b.WriteString("Keep tracing the mechanism through one of these next-hop files:\n")
	for _, file := range unread[:maxList] {
		fmt.Fprintf(&b, "  - `%s`\n", file)
	}
	b.WriteString("\nRead one of these files now, or grep within the anchor's unresolved callees / conditions before stopping.")
	e.midLoopPostPrimaryInjected = true
	return LoopSignal{
		HintRequested: true,
		HintKey:       "explorer.mid-loop.post-primary-read",
		Hint:          b.String(),
		Progress:      true,
	}
}

func (e *explorerEvaluator) scalarSourceLiteralPrimaryReadMode() bool {
	if e == nil || e.analysisIR == nil {
		return false
	}
	if types.IsScalarSourceLiteralLookup(e.analysisIR.RequestModel) {
		return true
	}
	plan := e.answerSurfacePlan()
	return plan != nil && plan.SummarySurfaceMode == types.AnswerSummarySurfaceMinimalScalarRoleLocate
}

func (e *explorerEvaluator) orderedSameFileTracePartialReadHint() string {
	if e == nil || e.analysisIR == nil {
		return ""
	}
	rm := e.analysisIR.RequestModel
	if !types.IsSingleTopicStructuralTrace(rm) {
		view := types.BuildAnswerSemanticView(e.analysisIR, e.answerSurfacePlan())
		if view == nil || !view.NeedsOrderedPrincipalList() ||
			(view.Family != types.QFCallChain && view.Family != types.QFRootCauseTrace) {
			return ""
		}
	}
	return "Because this dispatch wants an ordered in-file trace, first materialize the call / assignment / guard anchors that are ALREADY visible in the current read span in source order. Then keep paging this same function until the requested source-to-sink interval is covered before widening to sibling helpers or nearby files.\n"
}

func (e *explorerEvaluator) filterPartialReadsByAuthoritativeFrames(hints []partialReadHint) []partialReadHint {
	if len(hints) == 0 {
		return nil
	}
	preferred := e.authoritativeFrameSymbolTailsByFile()
	if len(preferred) == 0 {
		return hints
	}
	out := make([]partialReadHint, 0, len(hints))
	for _, hint := range hints {
		tails := preferred[canonicalExplorerPath(hint.file)]
		if len(tails) == 0 || tails[types.NormalizedSurfaceSymbolTail(hint.symbolName)] {
			out = append(out, hint)
		}
	}
	return out
}

func (e *explorerEvaluator) filterPartialReadsForPostPrimary(hints []partialReadHint) []partialReadHint {
	if len(hints) == 0 {
		return nil
	}
	hints = e.filterPartialReadsByAuthoritativeFrames(hints)
	hints = e.filterPartialReadsBySurfaceIntent(hints)
	hints = e.filterPartialReadsByTypedRelevance(hints)
	return hints
}

func (e *explorerEvaluator) filterPartialReadsForCurrentContext(hints []partialReadHint) []partialReadHint {
	if len(hints) == 0 {
		return nil
	}
	hints = e.filterPartialReadsByAuthoritativeFrames(hints)
	hints = e.filterPartialReadsBySurfaceIntent(hints)
	hints = e.filterPartialReadsByTypedRelevance(hints)
	hints = e.filterPartialReadsByGroundedEvidence(hints)
	return hints
}

func (e *explorerEvaluator) filterPartialReadsBySurfaceIntent(hints []partialReadHint) []partialReadHint {
	if len(hints) == 0 || e == nil || e.analysisIR == nil || !e.scalarSourceLiteralPrimaryReadMode() {
		return hints
	}
	allowed := e.scalarSourceLiteralPartialReadTails()
	if len(allowed) == 0 {
		return hints
	}
	out := make([]partialReadHint, 0, len(hints))
	for _, hint := range hints {
		tail := types.NormalizedSurfaceSymbolTail(hint.symbolName)
		if tail == "" || allowed[tail] {
			out = append(out, hint)
		}
	}
	if len(out) == 0 {
		return hints
	}
	return out
}

func (e *explorerEvaluator) scalarSourceLiteralPartialReadTails() map[string]bool {
	if e == nil || e.analysisIR == nil || !e.scalarSourceLiteralPrimaryReadMode() {
		return nil
	}
	allowed := make(map[string]bool)
	add := func(raw string) {
		tail := types.NormalizedSurfaceSymbolTail(raw)
		if tail != "" {
			allowed[tail] = true
		}
	}
	if plan := e.answerSurfacePlan(); plan != nil && plan.ExactResolution != nil {
		for _, target := range plan.ExactResolution.Targets {
			add(target)
		}
	}
	for _, item := range e.structuredEvidence {
		switch item.GroundingStatus {
		case types.GroundingGrounded, types.GroundingRecovered:
		default:
			continue
		}
		switch item.ContextRole {
		case types.EvidenceContextRoleIllustrativeOnly, types.EvidenceContextRoleAbsenceSupport:
			continue
		}
		add(item.AnchorSymbol)
		add(item.Subject)
		add(item.Object)
	}
	if len(allowed) == 0 {
		return nil
	}
	return allowed
}

func (e *explorerEvaluator) filterPartialReadsByTypedRelevance(hints []partialReadHint) []partialReadHint {
	if len(hints) == 0 || e == nil || e.analysisIR == nil {
		return hints
	}
	allowed := e.partialReadTypedRelevanceTails()
	if len(allowed) == 0 {
		return hints
	}
	out := make([]partialReadHint, 0, len(hints))
	for _, hint := range hints {
		if partialReadHintMatchesTypedTails(hint, allowed) {
			out = append(out, hint)
		}
	}
	return out
}

func (e *explorerEvaluator) partialReadTypedRelevanceTails() map[string]bool {
	if e == nil || e.analysisIR == nil {
		return nil
	}
	allowed := make(map[string]bool)
	add := func(raw string) {
		for _, tail := range partialReadSurfaceTails(raw) {
			allowed[tail] = true
		}
	}
	rm := e.analysisIR.RequestModel
	for _, s := range rm.AnalyzerHints.PrimaryEntities {
		add(s)
	}
	for _, s := range rm.AnalyzerHints.MentionedEntities {
		add(s)
	}
	for _, s := range rm.AnalyzerHints.ExactTargets {
		add(s)
	}
	for _, topic := range rm.SubTopics {
		for _, s := range topic.Entities {
			add(s)
		}
	}
	for _, item := range e.structuredEvidence {
		switch item.GroundingStatus {
		case types.GroundingGrounded, types.GroundingRecovered:
		default:
			continue
		}
		add(item.AnchorSymbol)
		add(item.Subject)
		add(item.Object)
		add(item.OwnerSymbol)
	}
	if len(allowed) == 0 {
		return nil
	}
	return allowed
}

func partialReadHintMatchesTypedTails(hint partialReadHint, allowed map[string]bool) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, tail := range partialReadSurfaceTails(hint.symbolName) {
		if allowed[tail] {
			return true
		}
	}
	return false
}

func partialReadSurfaceTails(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	seen := make(map[string]bool)
	out := make([]string, 0, 3)
	add := func(s string) {
		tail := types.NormalizedSurfaceSymbolTail(s)
		if tail == "" || seen[tail] {
			return
		}
		seen[tail] = true
		out = append(out, tail)
	}
	add(raw)
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '.' || r == ':' || r == '/' || r == '\\'
	})
	for _, part := range parts {
		add(part)
	}
	return out
}

func (e *explorerEvaluator) filterPartialReadsByGroundedEvidence(hints []partialReadHint) []partialReadHint {
	if len(hints) == 0 || len(e.structuredEvidence) == 0 {
		return hints
	}
	counts := make(map[string]int)
	add := func(file, symbol string) {
		file = canonicalExplorerPath(file)
		tail := types.NormalizedSurfaceSymbolTail(symbol)
		if file == "" || tail == "" {
			return
		}
		counts[file+"\x00"+tail]++
	}
	for _, item := range e.structuredEvidence {
		if item.GroundingStatus != types.GroundingGrounded {
			continue
		}
		add(item.Source, item.AnchorSymbol)
		add(item.Source, item.Subject)
		add(item.Source, item.Object)
	}
	out := make([]partialReadHint, 0, len(hints))
	for _, hint := range hints {
		key := canonicalExplorerPath(hint.file) + "\x00" + types.NormalizedSurfaceSymbolTail(hint.symbolName)
		if counts[key] >= 2 && hint.coverage >= 0.15 {
			continue
		}
		out = append(out, hint)
	}
	return out
}

func (e *explorerEvaluator) authoritativeFrameRealignmentHint(history []types.ToolResult) string {
	if e.searchResult == nil || e.searchResult.Graph == nil {
		return ""
	}
	preferred := e.authoritativeFrameSymbolTailsByFile()
	if len(preferred) == 0 {
		return ""
	}
	intervalsByFile := readFileIntervalsFromHistory(history)
	if len(intervalsByFile) == 0 {
		return ""
	}
	anchorFile := ""
	for _, file := range e.primaryEntityFiles() {
		canon := canonicalExplorerPath(file)
		if canon == "" || len(preferred[canon]) == 0 {
			continue
		}
		if len(intervalsByFile[canon]) == 0 {
			continue
		}
		anchorFile = canon
		break
	}
	if anchorFile == "" {
		return ""
	}
	fi := e.searchResult.Graph.FileIndex[anchorFile]
	if fi == nil {
		return ""
	}
	preferredTails := preferred[anchorFile]
	intervals := intervalsByFile[anchorFile]
	if len(preferredTails) == 0 || len(intervals) == 0 {
		return ""
	}

	var partialPreferred *partialReadHint
	var unreadPreferred []string
	for _, sym := range fi.Symbols {
		if sym.Kind != "function" && sym.Kind != "method" {
			continue
		}
		tail := types.NormalizedSurfaceSymbolTail(sym.Name)
		if !preferredTails[tail] {
			continue
		}
		startedFromTop, maxEnd := symbolReadProgress(sym, intervals)
		if startedFromTop && maxEnd > 0 && maxEnd < sym.EndLine {
			qualName := sym.Name
			if sym.Receiver != "" {
				qualName = sym.Receiver + "." + sym.Name
			} else if sym.Parent != "" {
				qualName = sym.Parent + "." + sym.Name
			}
			partial := partialReadHint{
				file:       anchorFile,
				symbolName: qualName,
				symbolKind: sym.Kind,
				symStart:   sym.Line,
				symEnd:     sym.EndLine,
				readEnd:    maxEnd,
				coverage:   float64(maxEnd-sym.Line+1) / float64(sym.EndLine-sym.Line+1),
			}
			partialPreferred = &partial
			break
		}
		if !startedFromTop {
			unreadPreferred = append(unreadPreferred, sym.Name)
		}
	}
	if partialPreferred != nil {
		return "MID-LOOP CHECK: the attached runtime log already points to this file, and the authoritative function body is only partially read. " +
			"Do NOT widen to nearby helpers yet.\n" +
			renderPartialReadHint(*partialPreferred, e.heuristics.PartialReadLineThreshold)
	}
	if len(unreadPreferred) == 0 {
		return ""
	}
	if len(unreadPreferred) > 3 {
		unreadPreferred = unreadPreferred[:3]
	}
	var b strings.Builder
	b.WriteString("MID-LOOP CHECK: the attached runtime log already names concrete function(s) in this file, but the lines you opened do not cover them yet. ")
	b.WriteString("Before following nearby helpers, grep/read these authoritative function names in the same file and ground them first:\n")
	for _, name := range unreadPreferred {
		fmt.Fprintf(&b, "  - `%s` in `%s`\n", name, anchorFile)
	}
	b.WriteString("\nUse the function name from the log frame as the next hop; treat the current line number as a stale locator, not as proof that the surrounding helper is the failure site.")
	return b.String()
}

func (e *explorerEvaluator) authoritativeFrameSymbolTailsByFile() map[string]map[string]bool {
	frames := e.authoritativeResolvedFrames()
	if len(frames) == 0 {
		return nil
	}
	out := make(map[string]map[string]bool)
	for _, frame := range frames {
		if out[frame.File] == nil {
			out[frame.File] = make(map[string]bool)
		}
		out[frame.File][frame.Tail] = true
	}
	return out
}

func (e *explorerEvaluator) renderAuthoritativeFrameStartSection(files []string) string {
	if e == nil {
		return ""
	}
	byFile := e.authoritativeFrameSymbolTailsByFile()
	if len(byFile) == 0 {
		return ""
	}
	seen := make(map[string]bool)
	orderedFiles := make([]string, 0, len(files))
	for _, file := range files {
		canon := canonicalExplorerPath(file)
		if canon == "" || seen[canon] || len(byFile[canon]) == 0 {
			continue
		}
		seen[canon] = true
		orderedFiles = append(orderedFiles, canon)
	}
	if len(orderedFiles) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Current function anchors from the log\n\n")
	b.WriteString("Before following nearby helpers or historical numeric offsets, grep/read the current definitions of these log-named functions in their resolved files. Use the function name as the next hop and treat raw stack line numbers as stale locators from an older build snapshot:\n\n")
	for _, file := range orderedFiles {
		targets := e.authoritativeFrameReadTargetsForFile(file)
		if len(targets) == 0 {
			continue
		}
		b.WriteString("- `" + file + "`: `")
		b.WriteString(strings.Join(targets, "`, `"))
		b.WriteString("`\n")
	}
	b.WriteString("\n")
	return b.String()
}

func (e *explorerEvaluator) authoritativeFrameReadTargetsForFile(file string) []string {
	if e == nil {
		return nil
	}
	canon := canonicalExplorerPath(file)
	if canon == "" {
		return nil
	}
	frames := e.authoritativeResolvedFrames()
	if len(frames) == 0 {
		return nil
	}
	lineByTail := make(map[string]int)
	nameByTail := make(map[string]string)
	if e.searchResult != nil && e.searchResult.Graph != nil {
		if fi := e.searchResult.Graph.FileIndex[canon]; fi != nil {
			for _, sym := range fi.Symbols {
				if sym.Kind != "function" && sym.Kind != "method" {
					continue
				}
				tail := types.NormalizedSurfaceSymbolTail(sym.Name)
				if tail == "" || lineByTail[tail] != 0 {
					continue
				}
				lineByTail[tail] = sym.Line
				nameByTail[tail] = strings.TrimSpace(sym.Name)
			}
		}
	}
	seen := make(map[string]bool)
	out := make([]string, 0, 4)
	for _, frame := range frames {
		if frame.File != canon {
			continue
		}
		name := authoritativeFrameDisplayName(frame.Func)
		if name == "" {
			name = frame.Tail
		}
		if pretty := strings.TrimSpace(nameByTail[frame.Tail]); pretty != "" {
			name = pretty
		}
		if name == "" || seen[name] {
			continue
		}
		if line := lineByTail[frame.Tail]; line > 0 {
			name = fmt.Sprintf("%s (line %d)", name, line)
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}

func authoritativeFrameDisplayName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.ReplaceAll(raw, `\`, `/`)
	if idx := strings.LastIndex(raw, "/"); idx >= 0 && idx+1 < len(raw) {
		raw = raw[idx+1:]
	}
	if idx := strings.LastIndex(raw, "."); idx >= 0 && idx+1 < len(raw) {
		raw = raw[idx+1:]
	}
	return strings.TrimSpace(raw)
}

// authoritativeResolvedFrames unions LogBundle.Errors[].Frames and
// PerfBundle.Stalls into a single (file, function-tail) anchor list.
// A frame counts as authoritative when its File resolves to a
// repo-relative path AND its symbol name has a normalisable tail.
// The validator on each pre-stage already enforces Confidence and
// repo-stat checks before promoting to ResolvedFiles, so we trust the
// frames whose file/func survived that filter.
//
// Decoupled from the panic/crash signal whitelist: an OOM trace, a
// timeout deadline-exceeded stack, a main-thread stall, and a jank
// span all carry the same kind of grounded locator once frames pass
// validation. The "authority" comes from frame resolution, not from
// the signal label.
func (e *explorerEvaluator) authoritativeResolvedFrames() []authoritativeFrameRef {
	if e == nil {
		return nil
	}
	out := make([]authoritativeFrameRef, 0, 8)
	seen := make(map[string]bool)
	add := func(rawFile, rawFunc string) {
		file := canonicalExplorerPath(rawFile)
		tail := types.NormalizedSurfaceSymbolTail(rawFunc)
		if file == "" || tail == "" {
			return
		}
		key := file + "\x00" + tail
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, authoritativeFrameRef{File: file, Tail: tail, Func: strings.TrimSpace(rawFunc)})
	}
	if e.logTriage != nil {
		for _, err := range e.logTriage.Errors {
			for _, frame := range err.Frames {
				add(frame.File, frame.Func)
			}
		}
	}
	if e.perfTrace != nil {
		for _, stall := range e.perfTrace.Stalls {
			if stall.File == "" || stall.Line == 0 {
				continue
			}
			add(stall.File, stall.Symbol)
		}
	}
	return out
}

// hasAuthoritativeFailureFrames reports whether at least one
// authoritative locator (resolved log frame OR resolved perf stall
// with file+line) is present. This is the signal-neutral replacement
// for the historical hasAuthoritativeCrashLog whitelist.
func (e *explorerEvaluator) hasAuthoritativeFailureFrames() bool {
	return len(e.authoritativeResolvedFrames()) > 0
}

func symbolReadProgress(sym repomap.Symbol, intervals []readInterval) (startedFromTop bool, maxEnd int) {
	if sym.EndLine == 0 || sym.EndLine < sym.Line || len(intervals) == 0 {
		return false, 0
	}
	bodyLines := sym.EndLine - sym.Line + 1
	entryZone := sym.Line + bodyLines/5
	for _, iv := range intervals {
		if iv.start <= entryZone && iv.end >= sym.Line {
			startedFromTop = true
		}
		if iv.end > maxEnd && iv.start <= sym.EndLine && iv.end >= sym.Line {
			if iv.end > sym.EndLine {
				maxEnd = sym.EndLine
			} else {
				maxEnd = iv.end
			}
		}
	}
	return startedFromTop, maxEnd
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
	if e.analysisIR != nil && types.RequiresExhaustiveEnumerationMemberSetHandoff(e.analysisIR.RequestModel) {
		logging.Debug("[explorer] ShouldStop iter=%d: suppressing S1 fallback because exhaustive enumeration requires structured member_set completion", iteration)
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
		if !e.primaryReadSeen || len(e.investigationNotes) <= e.notesLenAtPrimaryRead {
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
	e.syncEmitBacklogWindow(obs.AllToolResults)
	e.syncEvidenceRepairState(obs.AllToolResults)
	e.syncClosureRepairState(obs.AllToolResults)
	iteration := obs.Iteration
	allResults := obs.AllToolResults
	prevResultsLen := e.midLoopLastResultsLen
	currentResultsLen := len(allResults)
	e.updateMidLoopSerialStreak(prevResultsLen, currentResultsLen)
	defer e.finalizeMidLoopBatch(prevResultsLen, currentResultsLen)

	// Detect explicit completion signal from the LLM.
	// emit_investigation_complete is the explorer's terminal action —
	// stop immediately instead of burning one extra LLM round that
	// ShouldStop would catch anyway.
	//
	// The tool soft-downgrades failed completions (pending forced
	// reads / cite-floor miss / unverified anchor paths) with
	// Success=true and a DOWNGRADED-prefixed Summary so the LLM sees
	// the explanation in its tool-result history. MutableState's
	// InvestigationComplete flag stays FALSE in that case. Observers
	// must skip the terminal branch on downgrade — otherwise a soft
	// keep-alive is mistaken for completion and the loop exits before
	// the LLM can re-invest (and, critically, before it has a chance
	// to call emit_evidence if it skipped that step altogether).
	if obs.LastToolResult != nil && obs.LastToolResult.ToolName == "emit_investigation_complete" && obs.LastToolResult.Success {
		if strings.HasPrefix(obs.LastToolResult.Summary, tool.EmitInvestigationCompleteDowngradePrefix) {
			logging.Info("[explorer] emit_investigation_complete DOWNGRADED at iter=%d — loop continues", iteration)
		} else {
			e.investigationComplete = true
			logging.Info("[explorer] emit_investigation_complete observed at iter=%d", iteration)
			return LoopSignal{StopRequested: true, StopReason: "emit_investigation_complete called"}
		}
	}

	// Track primary-entity file reads for S1's df3-drift gate. Runs
	// unconditionally so even "quiet" observeMidLoop calls still
	// update the read tracking — ShouldStop's gate depends on this
	// state being current. Idempotent: once primaryReadSeen is set,
	// later calls are no-ops.
	e.observePrimaryRead(iteration, allResults)

	// Budget-exhausted nudge (Fix B 2026-05-10). Fires before any
	// other signal because the budget condition is structural — it
	// changes which tools the LLM is allowed to call from this iter
	// onward. Letting other signals fire first would inject a
	// repair / closure hint while the LLM is still planning to
	// re-call the exhausted tool; the budget hint corrects the
	// misplan in the same iter.
	if sig := e.postBudgetExhaustedSignal(obs); sig.HintRequested {
		return sig
	}

	if sig := e.postEmitEvidenceRepairSignal(obs); sig.HintRequested {
		return sig
	}
	if sig := e.postEmitEvidenceSurfaceTermReviewSignal(obs); sig.HintRequested {
		return sig
	}
	if sig := e.postEmitEvidenceRepairClosureOnlySignal(obs); sig.HintRequested {
		return sig
	}
	if e.awaitingEvidenceRepair(obs.AllToolResults) {
		return LoopSignal{}
	}
	if sig := e.postClosureRepairSignal(obs); sig.HintRequested {
		return sig
	}
	if sig := e.postClosureRepairClosureOnlySignal(obs); sig.HintRequested {
		return sig
	}
	if e.awaitingClosureRepair(obs.AllToolResults) {
		return LoopSignal{}
	}
	if sig := e.postExactAbsenceClosureSignal(obs); sig.HintRequested {
		return sig
	}
	if sig := e.postClosureReadyBacklogSignal(obs); sig.HintRequested {
		return sig
	}
	if sig := e.postExplanationAnchorSignal(obs); sig.HintRequested {
		return sig
	}
	if sig := e.postAuthoritativeTier1CompletionSignal(obs); sig.HintRequested {
		return sig
	}
	// Completion-ready is a typed close signal. It must beat generic
	// "read more / materialize backlog" nudges once repair-specific
	// blockers above have had their chance; otherwise a run can keep
	// widening scope even after the answer faces are already covered.
	// The post-anchor and read-without-emit nudges below still fire
	// when completion readiness is not established.
	if sig := e.postCompletionReadySignal(obs); sig.HintRequested {
		return sig
	}
	if sig := e.postCompletionReadyEscalationSignal(obs); sig.HintRequested {
		return sig
	}
	if sig := e.postCompletionReadyClosureOnlySignal(obs); sig.HintRequested {
		return sig
	}
	// Immediate post-anchor push: the most common drift after the
	// first primary-file read is a text-only recap ("I now understand
	// the function...") before the LLM has followed any next hop.
	// Fire a one-shot tool-first hint as soon as the anchor enters the
	// readSet, even before the generic mid-loop min-iteration gate.
	if sig := e.postPrimaryReadMidLoopSignal(obs); sig.HintRequested {
		return sig
	}
	if sig := e.postReadWithoutEmitSignal(obs); sig.HintRequested {
		return sig
	}
	if sig := e.postExecRedirectBeforeEmitSignal(obs); sig.HintRequested {
		return sig
	}
	if sig := e.postReadWithoutEmitEscalationSignal(obs); sig.HintRequested {
		return sig
	}
	if sig := e.postExternalLogRedirectSignal(obs); sig.HintRequested {
		return sig
	}
	if sig := e.postReadWithoutEmitClosureOnlySignal(obs); sig.HintRequested {
		return sig
	}

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
	if e.midLoopCompletionReadySent {
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

	// Once the dispatch has already been told to materialize the
	// current structured-evidence backlog, generic expansion nudges like
	// partial-read / enumeration / cross-file would only compete with
	// that higher-priority action and recreate the tail-spin we are
	// trying to avoid. Higher-priority closure signals above still run.
	if e.awaitingStructuredEvidenceMaterialization(obs.AllToolResults) {
		return LoopSignal{}
	}

	// Check 1: function-boundary coverage.
	if hints := e.filterPartialReadsForCurrentContext(detectPartiallyReadSymbols(allResults, e.searchResult.Graph)); len(hints) > 0 {
		h := hints[0] // worst-coverage offender
		unreadLines := h.symEnd - h.readEnd
		if unreadLines <= e.heuristics.PartialReadLineThreshold {
			// Small remainder: direct read is cheaper than grep+read.
			// read_file offset is 0-based; next unread 1-based line is
			// h.readEnd+1 → 0-based offset h.readEnd.
			fmt.Fprintf(&b, "MID-LOOP CHECK: you read `%s` in `%s` up to line %d but the function spans lines %d-%d (%.0f%% covered, %d lines remaining). "+
				"If this function is relevant to the question, call read_file with path=%q offset=%d limit=%d to see the rest.\n",
				h.symbolName, h.file, h.readEnd, h.symStart, h.symEnd, h.coverage*100, unreadLines,
				h.file, types.LineToReadFileOffset(h.readEnd+1), unreadLines)
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
	// Project-orientation finalize nudge. When the analyzer
	// classifies the request as orientation (intent=explain + simple
	// + no entities + clean predicates) and the LLM has already
	// gathered ≥ 3 grounded evidence items, push a one-shot hint to
	// call emit_investigation_complete. Without this, an LLM that's
	// satisfied with its evidence can still walk one or two extra
	// rounds reading siblings before emitting completion. The
	// once-per-dispatch latch keeps the hint from drowning the loop.
	//
	// The threshold (3) is intentionally low — orientation answers
	// rest on README + manifest + entry-point and rarely need more.
	// budget.Compute already caps MaxFiles=8 for the same reason, so
	// this hint is the soft companion to that hard ceiling.
	if e.isOrientationQuery && !e.midLoopOrientationFinalizeSent &&
		len(e.structuredEvidence) >= 3 && !e.investigationComplete {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b,
			"MID-LOOP CHECK: project-orientation question (\"what does this repo do?\"). You have %d grounded evidence item(s) — enough to answer. README + manifest + top-level entry-point cover this answer shape; reading additional sibling files will not improve the answer.\n"+
				"NEXT ACTION: call `emit_investigation_complete(reason=\"orientation answer ready: project purpose + key modules grounded from README/manifest/entry-point\", confidence=\"high\", result_kind=\"resolved\")` now. Do NOT call any more `read_file` / `grep` / `repo_map` first.\n",
			len(e.structuredEvidence),
		)
		e.midLoopOrientationFinalizeSent = true
		if hintKey == "" {
			hintKey = "explorer.mid-loop.orientation-finalize"
		}
	}

	if e.isEnumerationQuery && !e.midLoopEnumInjected {
		discovered, readSet, _ := extractFileCoverage(allResults, e.repoRoot)
		// P2 #4 (2026-05-03) — typed override for category-enumeration
		// questions that name a known interface / trait / protocol.
		// implementerFilesFromGraph reads typed Symbol.Implements
		// relations and bypasses the keyword ranker entirely; on
		// `s5a` ("list all LoopController implementations") this
		// returned the 8 actual implementer files instead of the
		// ranker's 12+ unrelated ones.
		var typedScope []string
		if e.analysisIR != nil &&
			e.analysisIR.RequestModel.Predicates.IsCategoryEnumeration {
			rmHints := e.analysisIR.RequestModel.AnalyzerHints
			candidateEntities := append([]string(nil), rmHints.PrimaryEntities...)
			candidateEntities = append(candidateEntities, rmHints.Entities...)
			// P4-cross-sub-repo (Sc 1): prefer cached multigraph
			// carrier so implementer expansion fans out across active
			// sub-repos. Falls through to e.searchResult.Graph (single-
			// graph view) when multigraph isn't wired (single-shot
			// tests / multi_repo_enabled=false).
			implTarget := any(e.searchResult.Graph)
			if e.multiGraphHandle != nil {
				implTarget = e.multiGraphHandle
			}
			if files := implementerFilesFromGraph(implTarget, candidateEntities); len(files) > 0 {
				typedScope = files
			}
		}
		scope := typedScope
		if len(scope) == 0 {
			scope = e.coverageScopeFiles(discovered, readSet, strings.Join(e.investigationNotes, "\n"))
		}
		if len(scope) > 0 {
			readCount, coverage, unread := coverageSnapshot(scope, readSet)
			// L2 (2026-05-10) — drop unread candidates the LLM has
			// already read in a prior iteration without emitting any
			// emit_evidence on them. Re-pushing those files
			// contradicts the LLM's own judgment that they are off-
			// topic. The structuredEvidence slice IS per-dispatch but
			// readSet is cumulative — the difference (read but not
			// cited this dispatch OR any prior) is the LLM's "saw and
			// declined" signal.
			cited := make(map[string]bool, len(e.structuredEvidence))
			for _, ev := range e.structuredEvidence {
				if ev.Producer != tool.EmitEvidenceProducer {
					continue
				}
				if cf := canonicalEvidenceSourcePath(ev.Source); cf != "" {
					cited[cf] = true
				}
			}
			filteredUnread := make([]string, 0, len(unread))
			for _, f := range unread {
				cf := canonicalExplorerPath(f)
				if readSetContains(readSet, cf) && !cited[cf] {
					continue
				}
				// L4 (2026-05-10) — analyzer-declared irrelevant
				// files never appear in mid-loop hints. Honors the
				// LLM's negative judgment across iterations.
				if e.irrelevantFilesSet != nil && e.irrelevantFilesSet[cf] {
					continue
				}
				filteredUnread = append(filteredUnread, f)
			}
			unread = filteredUnread
			if coverage < e.heuristics.MidLoopEnumCoverage && len(unread) >= e.heuristics.EnumMidLoopUnreadFloor {
				if len(unread) > 5 {
					unread = unread[:5]
				}
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				headLabel := "discovered"
				if len(typedScope) > 0 {
					headLabel = "typed-relation"
				}
				fmt.Fprintf(&b, "MID-LOOP CHECK: the question asks for an enumeration but you have read only %d of %d %s files (%.0f%%). "+
					"Read these next: %s\n",
					readCount, len(scope), headLabel, coverage*100, strings.Join(unread, ", "))
				// Session-22 one-shot lock. Without this gate, the
				// enumeration check re-fired every post-throttle window
				// while coverage sat below MidLoopEnumCoverage — 68
				// firings observed in a single goroutine_dump run. The
				// message payload is structurally stable (same "read
				// these next" list as LLM inches coverage upward), so
				// repeat fires are noise, not signal. Mirrors the
				// pattern on Check 3 (parallelize), Check 4
				// (cross-file-ref), Check 5 (intent-window) and Check 6
				// (ranker-coverage). Reset cross-dispatch.
				e.midLoopEnumInjected = true
				if hintKey == "" {
					hintKey = "explorer.mid-loop.enumeration"
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
	// #2 fix: cross-file-ref no longer gated on b.Len()==0 so it can
	// co-exist with partial-read hints in the same signal. This
	// prevents the throttle from eating the hint when partial-read
	// took an earlier injection slot.
	if !e.midLoopSymbolRefInjected &&
		len(e.investigationNotes) >= e.heuristics.MidLoopMinIteration {
		_, readSet, _ := extractFileCoverage(allResults, e.repoRoot)
		gaps := detectCrossFileSymbolGapsWithFileFilter(
			e.investigationNotes, e.searchResult.Graph, readSet, 3, e.activeFocusAllowsFile)
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
			// Use a unique key so cross-file-ref dedup is independent
			// of partial-read dedup.
			hintKey = "explorer.mid-loop.cross-file-ref"
		}
	}

	// Check 5: structural-intent vs narrow-window mismatch (session 22).
	//
	// Symptom from a customer trace: the LLM said in its iter content
	// "最后确认 agent.go 中 ReAct 循环的大致结构" — a declared intent to
	// grasp overall STRUCTURE — while the accompanying read_file call
	// covered lines 527-586 of a 1549-line file (3.9%). A 60-line
	// window cannot support a structural-overview conclusion; the LLM
	// subsequently exited with a "未完整取证" caveat because its own
	// coverage was demonstrably shallow.
	//
	// detectPartiallyReadSymbols only detects SYMBOL-RANGE gaps (LLM
	// started reading a function at line N but did not reach its
	// EndLine). A narrow targeted read that does NOT enter any
	// function's entry zone — common when the LLM grep-directs a
	// read_file to a specific offset — slips through that check.
	//
	// This branch triggers on the *semantic* mismatch: structural
	// intent tokens in the assistant content AND a read_file window
	// covering ≤ 15% of a file ≥ 300 lines. The hint directs the LLM
	// to either list_files (to see the module layout) or a wider
	// read_file window starting from the top.
	//
	// Fires at most once per dispatch (midLoopIntentWindowSent flag)
	// so the hint does not recur on every subsequent read.
	if b.Len() == 0 && !e.midLoopIntentWindowSent &&
		obs.LastToolResult != nil && obs.LastToolResult.ToolName == "read_file" &&
		obs.LastToolResult.Success && isStructuralIntent(obs.Response.Content) {
		if path, rng, total, ok := parseReadFileBanner(obs.LastToolResult.Summary); ok && total >= 300 {
			windowSize := rng.End - rng.Start + 1
			if windowSize > 0 && float64(windowSize)/float64(total) < 0.15 {
				windowPct := float64(windowSize) * 100 / float64(total)
				fmt.Fprintf(&b,
					"MID-LOOP CHECK: your note signals a structural / overview intent, "+
						"but the read_file window covers lines %d-%d of %d total in `%s` "+
						"(%.1f%% of the file). A narrow window cannot support a conclusion "+
						"about the file's OVERALL structure. Before ending the investigation, "+
						"either (a) call `list_files` on the containing directory plus a single "+
						"read_file with offset=1 limit=200 to see the top-level imports + type "+
						"declarations, or (b) issue a wider read_file (limit>=300) starting from "+
						"offset=1 so the structural claim is grounded in actual coverage.\n",
					rng.Start, rng.End, total, path, windowPct)
				e.midLoopIntentWindowSent = true
				hintKey = "explorer.mid-loop.intent-window-mismatch"
			}
		}
	}

	// Check 6: low ranker-coverage pushback (session 22).
	//
	// Symptom: the LLM closed explorer with readSet=4 while the
	// keyword ranker had scored 36 files as relevant — 11% coverage.
	// Non-enumeration queries had no mid-loop gate for this; the
	// enumeration path has MidLoopEnumCoverage (0.6) but it keys off
	// isEnumerationQuery, so mechanism / explain / root_cause questions
	// slip through when the LLM opens only a handful of the top-
	// ranked files.
	//
	// Criteria (all must hold):
	//   1. Past early-loop phase (iter >= 2 * MidLoopMinIteration).
	//   2. Ranker scored at least 6 files (small scopes don't need
	//      the nudge — legitimate "only these files matter" cases).
	//   3. Of the top-5 ranked files, at least 3 are NOT in readSet
	//      (the LLM is demonstrably skipping the most relevant work).
	//   4. Session-22 F2.1 (generalised): when the attached failure
	//      trace (log_triage and/or perf_triage) has resolved frames
	//      AND the LLM has read every ResolvedFiles entry from the
	//      union, SKIP the nudge — the grounded frames define the
	//      file-set ceiling; the ranker's remaining top-K are noise
	//      siblings sharing common method names. Pushing to read more
	//      of them caused the logtri_go hallucination chain. Holds for
	//      panic/crash, OOM, timeout, jank, and main-thread-stall traces
	//      alike — the gate is frame-resolution, not a signal whitelist.
	//
	// One-shot per dispatch so the hint does not re-fire every round.
	// Complements Check 5: Check 5 catches a single narrow-window call;
	// Check 6 catches aggregate coverage drift.
	if b.Len() == 0 && !e.midLoopRankerCoverageSent &&
		iteration >= e.heuristics.MidLoopMinIteration*2 &&
		len(e.allScoredFiles) >= 6 &&
		!e.authoritativeFailureCovered(allResults) {
		_, readSet, _ := extractFileCoverage(allResults, e.repoRoot)
		rankedFiles := e.rankerCoverageFilesForReadSet(readSet)
		if len(rankedFiles) >= 6 {
			topK := 5
			if topK > len(rankedFiles) {
				topK = len(rankedFiles)
			}
			var missing []string
			for _, f := range rankedFiles[:topK] {
				// allScoredFiles entries are always repo-relative (keyword
				// search emits relative paths). readSet is now also always
				// repo-relative because extractFileCoverage canonicalises
				// via ground.CanonicalRepoRelative with e.repoRoot. Both
				// map keys speak the same form, so a direct lookup works
				// without an absolute-vs-relative reconciliation layer.
				canon := canonicalExplorerPath(f)
				if canon == "" {
					canon = f
				}
				if isNoisePath(canon) {
					continue
				}
				if readSetContains(readSet, canon) {
					continue
				}
				missing = append(missing, f)
			}
			if len(missing) >= 3 {
				fmt.Fprintf(&b,
					"MID-LOOP CHECK: the keyword ranker scored %d relevant files but %d of "+
						"the top-%d remain unread: %s. Before declaring the investigation "+
						"complete, open at least 2-3 of these if they correspond to the "+
						"question's subject or mechanism — the ranker's top-K is where "+
						"grounded evidence typically lives.\n",
					len(rankedFiles), len(missing), topK, strings.Join(missing, ", "))
				e.midLoopRankerCoverageSent = true
				hintKey = "explorer.mid-loop.ranker-coverage"
			}
		}
	}

	// Check 7: emit_evidence kind=absent deprecation redirect
	// (session-22 follow-up).
	//
	// The default analyze/explore prompts intentionally do not warn
	// against kind=absent — it is a rare antipattern (maybe 1% of
	// queries, where the LLM tries to emit per-fact "not found"
	// claims through the evidence channel) and pre-declaring every
	// deprecated kind in the skill would bloat the prompt. Instead,
	// we detect the bug in situ: the tool's reject message already
	// includes the full redirect to absence_justification, but
	// some LLMs ignore the tool result and re-emit the same batch
	// with a slight rewording.
	//
	// After we see the structured redirect string ("kind=absent is
	// not emittable via emit_evidence") appear in 2 OR MORE failed
	// tool results, inject a stronger orchestrator-level hint. The
	// orchestrator channel is visually distinct from tool results —
	// the LLM treats it as a system directive rather than
	// tool-validation feedback, which increases the chance of
	// course-correction.
	//
	// One-shot per dispatch; reset cross-dispatch so a later window
	// can fire again if the LLM slips back.
	if b.Len() == 0 && !e.midLoopAbsentRedirectSent {
		const marker = "kind=absent is not emittable via emit_evidence"
		hits := 0
		for i := range allResults {
			r := &allResults[i]
			if r.Success || r.ToolName != "emit_evidence" {
				continue
			}
			if strings.Contains(r.Summary, marker) {
				hits++
				if hits >= 2 {
					break
				}
			}
		}
		if hits >= 2 {
			b.WriteString(
				"MID-LOOP CHECK: your emit_evidence batch has been rejected multiple times for including kind=absent items. " +
					"This kind was deprecated from the emit_evidence channel because the tool's validator requires line_start > 0 + anchor_kind + anchor_symbol for every item — a 'searched and found nothing' claim cannot satisfy these. " +
					"ACTION: re-emit the batch with kind=absent items REMOVED. " +
					"If the overall answer is 'zero / no X' (whole-answer absence), declare it on emit_investigation_complete with `result_kind=\"absence\"` and `absence_justification` describing what was searched and not found — that field waives the citation floor by contract. " +
					"If the absence is per-fact inside a larger investigation, just omit the item from emit_evidence and describe the absence in your <think> notes (the answer summary can pick it up from there). " +
					"Do not retry kind=absent — the channel will keep rejecting.\n")
			e.midLoopAbsentRedirectSent = true
			hintKey = "explorer.mid-loop.absent-redirect"
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
		discovered, readSet, _ := extractFileCoverage(allResults, e.repoRoot)
		unreadCount := 0
		for _, f := range discovered {
			if !readSetContains(readSet, f) && !isNoisePath(f) {
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
	hint := b.String()
	logging.Debug("[explorer] mid-loop hint built key=%q len=%d body=%q",
		hintKey, len(hint), logging.Truncate(hint, logging.HintBodyMax))
	return LoopSignal{
		HintRequested: true,
		Hint:          hint,
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
// The old "if e.idleStreakInDepth >= 2 { return ”, false }"
// terminal check is GONE: LoopPolicy.IdleStopThreshold enforces it
// at the policy layer so every evaluator gets the same termination
// semantics without re-implementing the count.
//
// The old `e.lastToolResultCount`-based tool-growth detector is also
// GONE: loopPolicyState.Apply tracks the tool-result count growth
// itself and resets the idle streak on growth.
func (e *explorerEvaluator) observeSoftStop(obs LoopObservation) LoopSignal {
	e.ensureHeuristics()
	e.syncEmitBacklogWindow(obs.AllToolResults)
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

	// If the LLM already called emit_investigation_complete, accept
	// the soft-stop immediately — no continuation prompts needed.
	if e.investigationComplete {
		return LoopSignal{Progress: progress}
	}

	note := normalizeExplorationNote(resp.Content)
	if meta := softStopMetaDialogue(resp.Content); meta {
		return LoopSignal{
			HintRequested: true,
			HintKey:       "explorer.phase1.no-meta-dialogue",
			Hint: "Do not ask the user whether to continue investigating or which area to inspect next. " +
				"Continue the investigation yourself: follow the latest repair / retry directive, use tools, and either emit grounded evidence or call emit_investigation_complete if the evidence is already sufficient. " +
				"Keep the response in the user's language and avoid `Answer:` / `Evidence:` style headings.",
			Progress: progress,
		}
	}

	// Capture assistant analysis messages from the ReAct loop.
	// These contain the LLM's processed understanding of the files
	// it read — essential for synthesis, where raw files get truncated.
	if note != "" && e.phase == 1 {
		e.investigationNotes = append(e.investigationNotes, note)
		// Cross-reference tracking: scan the note for symbol names
		// that are defined in other files. If the LLM mentions
		// "NewSubExplorer" and repo_map knows it's defined in
		// sub_explorer.go, add that file to coverage tracking.
		e.trackCrossReferences(note)
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

		if sig := e.postReadWithoutEmitSoftStopSignal(obs); sig.HintRequested {
			return sig
		}

		// Before transitioning to Phase 2, check if Phase 1 actually
		// discovered any files. If all greps returned zero results,
		// push the LLM to retry with broader patterns before moving on.
		discovered, _, _ := extractFileCoverage(history, e.repoRoot)
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
			discovered, _, _ = extractFileCoverage(history, e.repoRoot)
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
			"- For call-like evidence (`calls` / `invokes` / `dispatches`): `subject` = caller, `object` = callee\n" +
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

	if evidenceLikeSoftStop(resp.Content) {
		return LoopSignal{
			HintRequested: true,
			HintKey:       "explorer.phase1.emit-evidence",
			Hint: "You wrote evidence-like `[DIRECT]` / `[REGISTRATION]` / `[CONDITIONAL]` lines in text, " +
				"but those facts are NOT recorded until you call `emit_evidence(items=[...])`. " +
				"Re-emit those facts now with `source`, exact `line_start`, `anchor_kind`, and `anchor_symbol`. " +
				"After `emit_evidence` succeeds, either continue investigating or call `emit_investigation_complete(reason, confidence, result_kind)`. " +
				"Do NOT use `result_kind=\"absence\"` / `absence_justification` unless the answer is genuinely zero / no-such-symbol / not found.",
			Progress: true,
		}
	}

	// Phase 1 (depth read): use runtime file coverage as guidance.
	// Merge grep-discovered files with pre-scanned top files so the
	// coverage check catches high-scoring files the LLM didn't grep.
	discovered, readSet, _ := extractFileCoverage(history, e.repoRoot)
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
	notesJoined := strings.Join(e.investigationNotes, "\n")
	scope := e.coverageScopeFiles(discovered, readSet, notesJoined)
	scopeReadCount, scopeCoverage, scopeUnread := coverageSnapshot(scope, readSet)
	unread := append([]string(nil), scopeUnread...)

	// Function-boundary read guidance: when the LLM reads part of a
	// function but stops before the end, inject exact read ranges.
	//
	// #1 relevance filter: skip partial-read hints for functions
	// whose name is NOT related to the question entities / keywords.
	// The 2026-04-18 "explorer是如何调用subagent的？" failure showed
	// partial-read fixation on BuildInitialInstruction (600 lines,
	// prompt-builder, irrelevant to "calling subagent") consuming 5
	// iterations while orchestrator.go / propose_sub_agents.go were
	// never read. The filter keeps partial-read hints for functions
	// that contain any entity/keyword token in their name; non-
	// matching functions get their hint suppressed so the LLM's
	// attention budget is spent on reading NEW relevant files
	// instead of finishing irrelevant large functions.
	if e.searchResult != nil && e.searchResult.Graph != nil {
		partialHints := e.filterPartialReadsForCurrentContext(detectPartiallyReadSymbols(history, e.searchResult.Graph))
		if len(partialHints) > 0 {
			progress = true // keep the loop alive via LoopSignal.Progress
			var hint strings.Builder
			hint.WriteString("**Incomplete function reads detected.** If these functions are relevant to the question, finish reading them:\n\n")
			for _, ph := range partialHints {
				unreadLines := ph.symEnd - ph.readEnd
				if unreadLines <= e.heuristics.PartialReadLineThreshold {
					// read_file offset is 0-based; next unread 1-based
					// line is ph.readEnd+1 → 0-based offset ph.readEnd.
					fmt.Fprintf(&hint, "- `%s` in %s (lines %d-%d): you read up to line %d (%.0f%%, %d lines remaining). "+
						"Call `read_file` with path=%q offset=%d limit=%d to see the rest\n",
						ph.symbolName, ph.file, ph.symStart, ph.symEnd,
						ph.readEnd, ph.coverage*100, unreadLines,
						ph.file, types.LineToReadFileOffset(ph.readEnd+1), unreadLines)
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

	// Retry-triggered refinement: bump RetryCount on unsatisfied
	// requirements each time the soft-stop fires. When RetryCount
	// exceeds replanRetryThreshold, split multi-entity requirements
	// into per-entity sub-requirements via ermMaybeRefine.
	if len(e.ermRequirements) > 0 && !ermAllSatisfied(e.ermRequirements) {
		for i := range e.ermRequirements {
			if e.ermRequirements[i].Status != "satisfied" {
				e.ermRequirements[i].RetryCount++
			}
		}
		if refined, changed := ermMaybeRefine(e.ermRequirements); changed {
			e.ermRequirements = refined
			e.ermRequirements = checkRequirementSatisfaction(
				e.ermRequirements, e.investigationNotes, e.structuredEvidence, e.complexity)
		}
	}

	// RequiredFiles coverage (HARD evidence, T3a follow-up): when the
	// analyzer pre-computed a RequiredFiles list (from repo_map entity
	// query) and < 50% of those files have been read, push the LLM
	// to read the unread ones. This is a hard-evidence branch — the
	// RequiredFiles list is structurally derived from entity matches,
	// not a heuristic guess — so it fires even on firstSoftStop.
	// Closes the gap from the 2026-04-18 log where propose_sub_agents.go
	// was in RequiredFiles and visible in the prompt but never read.
	if reqFiles := e.requiredFiles; len(reqFiles) > 0 {
		var unreadReq []string
		for _, rf := range reqFiles {
			if !readSetContains(readSet, rf) {
				unreadReq = append(unreadReq, rf)
			}
		}
		if len(unreadReq) > 0 && float64(len(unreadReq))/float64(len(reqFiles)) > 0.3 {
			progress = true
			var hint strings.Builder
			hint.WriteString("**Analyzer Required Files not yet read.** The analyzer identified these files as structurally relevant to the question, but you haven't read them yet:\n\n")
			for _, f := range unreadReq {
				if len(hint.String()) > 600 {
					break
				}
				hint.WriteString("- `" + f + "`\n")
			}
			hint.WriteString("\nRead the most important unread file and extract evidence from it.\n")
			return LoopSignal{
				HintRequested: true,
				HintKey:       "explorer.phase1.required-files",
				Hint:          hint.String(),
				Progress:      progress,
			}
		}
	}

	// Enumeration completeness: when the question asks to "list all X",
	// verify that the LLM has analyzed enough of the discovered files.
	// A coverage gap here means the enumeration will be incomplete.
	if e.isEnumerationQuery && len(scope) > 0 {
		enumCoverage := scopeCoverage
		if enumCoverage < e.heuristics.SoftStopEnumCoverage && len(unread) > 0 {
			var hint strings.Builder
			fmt.Fprintf(&hint, "**Enumeration completeness check:** This question asks to list ALL items. "+
				"You found %d matching files but only read %d (%.0f%% coverage). "+
				"For enumeration queries you must achieve ≥%.0f%% coverage.\n\n"+
				"Unread files:\n", len(scope), scopeReadCount, enumCoverage*100, e.heuristics.SoftStopEnumCoverage*100)
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
	preScannedUnread := e.preScannedUnreadCandidates(readSet)

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
			"File coverage: %d read out of %d discovered.\n", scopeReadCount, len(scope))
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
				"Continue the investigation yourself — do not ask the user what to do next. " +
				"If you have collected enough evidence to answer the user's question, call emit_investigation_complete(reason, confidence, result_kind) now, with the concise conclusion and boundary in reason. " +
				"If you still need more evidence, keep reading files / running greps, and avoid `Answer:` / `Evidence:` style headings in your notes.",
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
		cvPreview = e.getConcreteValuesCached(context.TODO(), e.repoRoot, readSet, nil).markdown
	}

	// Show remaining coverage for grep-discovered files.
	var hint strings.Builder
	fmt.Fprintf(&hint,
		"File coverage: %d read out of %d discovered.\n", scopeReadCount, len(scope))
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

func normalizeExplorationNote(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	var kept []string
	for _, rawLine := range strings.Split(content, "\n") {
		line := stripExplorationNarrativeLabel(rawLine)
		if line != "" {
			kept = append(kept, line)
		}
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func stripExplorationNarrativeLabel(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	stripped := strings.TrimLeft(trimmed, "* ")
	lower := strings.ToLower(stripped)
	for _, label := range []string{"answer", "evidence", "summary", "caveat", "question", "workflow"} {
		if !strings.HasPrefix(lower, label) {
			continue
		}
		rest := stripped[len(label):]
		rest = strings.TrimLeft(rest, "* ")
		if !strings.HasPrefix(rest, ":") {
			continue
		}
		return strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(rest, ":"), "* "))
	}
	return trimmed
}

func softStopMetaDialogue(content string) bool {
	text := strings.ToLower(strings.TrimSpace(content))
	if text == "" {
		return false
	}
	if containsAnySubstr(text,
		"would you like me to", "do you want me to", "should i continue",
		"let me know if you want me to", "continue investigating or address a different area",
		"address a different area",
		"是否继续", "要我继续", "继续调查还是", "换个方向", "其他方向") {
		return true
	}
	return containsAnySubstr(text,
		"i encountered an error", "i ran into an error", "there was an error") &&
		containsAnySubstr(text,
			"continue investigating", "different area", "let me know", "would you like")
}

func containsAnySubstr(text string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func evidenceLikeSoftStop(content string) bool {
	if strings.TrimSpace(content) == "" {
		return false
	}
	for _, rawLine := range strings.Split(content, "\n") {
		line, ok := normalizeEvidenceLine(rawLine)
		if !ok {
			continue
		}
		close := strings.Index(line, "]")
		if close < 2 {
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(line[1:close])) {
		case "DIRECT", "REGISTRATION", "CONDITIONAL", "MECHANISM", "RELATIONSHIP", "ABSENT":
			return true
		}
	}
	return false
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
//	→ Tip: read_file supports offset+limit for targeted reads.
//	  Symbols in <path>:
//	    - NewSubExplorer (lines 25-34)
//	    - SubExplorer.Run (lines 35-65)
//	    - subExplorerEvaluator (lines 67-107)
//	  Example: read_file path=<path> offset=25 limit=83 to cover lines 25-107.
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
	// Concrete example spanning the top-N union. read_file offset is
	// 0-based; the symbol's 1-based start line maps to offset
	// start-1. The "(covers lines …)" gloss stays in 1-based form so
	// it matches the symbol-line listing above.
	startLine := syms[0].start
	coverEnd := syms[topN-1].end
	offset := types.LineToReadFileOffset(startLine)
	limit := coverEnd - startLine + 1
	fmt.Fprintf(&b, "    Example: read_file path=%s offset=%d limit=%d  (covers lines %d-%d).\n",
		path, offset, limit, startLine, coverEnd)
	return b.String()
}

// Observe implements LoopController. Dispatches to observeMidLoop or
// observeSoftStop based on the LoopPhase in the observation. See the
// two helper methods for the detection logic; this function is pure
// phase routing.
func (e *explorerEvaluator) Observe(ctx *types.AgentContext, obs LoopObservation) LoopSignal {
	switch obs.Phase {
	case PhaseMidLoop:
		e.refreshMidLoopStructuredEvidence(ctx)
		return e.observeMidLoop(obs)
	case PhaseSoftStop:
		return e.observeSoftStop(obs)
	}
	return LoopSignal{}
}

func (e *explorerEvaluator) refreshMidLoopStructuredEvidence(ctx *types.AgentContext) {
	e.mergeEmittedEvidenceDelta(ctx)
	e.refreshExactContextFiles(ctx)
}

func (e *explorerEvaluator) mergeEmittedEvidenceDelta(ctx *types.AgentContext) {
	if e == nil || ctx == nil || ctx.Mutable == nil {
		return
	}
	delta, total := ctx.Mutable.EmittedEvidenceSince(e.mergedEmittedEvidenceLen)
	if total == 0 {
		e.mergedEmittedEvidenceLen = 0
		return
	}
	if len(delta) == 0 {
		e.mergedEmittedEvidenceLen = total
		return
	}
	e.structuredEvidence = mergeEvidenceItems(e.structuredEvidence, delta)
	e.mergedEmittedEvidenceLen = total
}

func (e *explorerEvaluator) refreshExactContextFiles(ctx *types.AgentContext) {
	if e == nil || ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil {
		return
	}
	contract := ctx.AnalysisIR.AnswerContract.ExactResolution
	if contract == nil || !contract.AllowAbsence || len(contract.Targets) == 0 ||
		contract.RelatedContextPolicy == types.ExactContextGroundedOnly {
		e.exactContextFiles = nil
		ctx.Mutable.SetExactContextRequiredFiles(nil)
		return
	}
	cands := e.collectExactResolutionSymbolCandidates(contract, e.analyzerKeywords)
	cands = exactResolutionFilterCandidatesToPreferredFiles(cands, e.requiredFiles)
	e.exactContextFiles = refreshedExactResolutionContextFiles(
		contract,
		ctx.AnalysisIR.RequestModel.Scenario,
		e.searchGraph(),
		e.structuredEvidence,
		cands,
		e.requiredFiles,
		e.exactContextFiles,
	)
	ctx.Mutable.SetExactContextRequiredFiles(e.exactContextFiles)
}

func startExplorerParseSectionWatchdog(ctx *types.AgentContext, section string) func() {
	stage := ""
	if ctx != nil {
		stage = string(ctx.Stage)
	}
	start := time.Now()
	logging.Debug("[diag explorer] phase=parse_output section=%s stage=%s start", section, stage)

	done := make(chan struct{})
	var once sync.Once
	go func() {
		timer := time.NewTimer(explorerParseSlowAfter)
		defer timer.Stop()
		for {
			select {
			case <-done:
				return
			case <-timer.C:
				logging.Warning("[diag explorer] phase=parse_output section=%s stage=%s still running elapsed=%s",
					section, stage, time.Since(start).Round(time.Second))
				timer.Reset(explorerParseSlowEvery)
			}
		}
	}()

	return func() {
		once.Do(func() {
			close(done)
			logging.Debug("[diag explorer] phase=parse_output section=%s stage=%s done elapsed=%s",
				section, stage, time.Since(start).Round(time.Millisecond))
		})
	}
}

func explorerParseContextErr(ctx *types.AgentContext, section string) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Context().Err(); err != nil {
		logging.Warning("[diag explorer] phase=parse_output section=%s canceled: %v", section, err)
		return err
	}
	return nil
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
	stopParseSection := startExplorerParseSectionWatchdog(ctx, "repo_facts")
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
	stopParseSection()
	if err := explorerParseContextErr(ctx, "repo_facts"); err != nil {
		return nil, err
	}

	stopParseSection = startExplorerParseSectionWatchdog(ctx, "structured_evidence")
	e.ensureStructuredEvidence(ctx, toolResults)
	stopParseSection()
	if err := explorerParseContextErr(ctx, "structured_evidence"); err != nil {
		return nil, err
	}

	// Merge concrete values into structured evidence. Before the
	// synthesis-LLM removal (2026-04-16) this merge lived inside
	// SynthesisPrompt; now ParseOutput is the sole owner.
	// cvReadRanges tracks the per-file [Start, End] slices the LLM
	// actually fetched via read_file (see parseReadFileBanner): the
	// range-aware chain promotion enforcer consults this via
	// EvidenceClosure.HasReadLine so a paginated read that covered
	// only lines 1-200 cannot grant coverage to a chain anchored at
	// line 500.
	stopParseSection = startExplorerParseSectionWatchdog(ctx, "file_coverage")
	cvReadSet, cvReadRanges, cvTotals, discoveredFiles := extractFileCoverageWithTotals(toolResults, e.repoRoot)
	stopParseSection()
	if err := explorerParseContextErr(ctx, "file_coverage"); err != nil {
		return nil, err
	}
	// CGEC: pass the per-Run EvidenceClosure so the chain promotion
	// enforcer can demote chains anchored outside ReadSet and queue
	// the missing files as PendingReads on the closure. Update the
	// closure's ReadSet snapshot first so any other CGEC consumer
	// (pre-complete check, stall detector) sees the latest view.
	// SetFileTotalLines propagates per-file totals from the read_file
	// banner so HasFullyRead / CoverageRatio have a real denominator.
	var cvClosure *types.EvidenceClosure
	if ctx.Mutable != nil {
		cvClosure = ctx.Mutable.EvidenceClosure()
		cvClosure.SetReadSet(cvReadSet)
		cvClosure.SetReadRanges(cvReadRanges)
		cvClosure.SetFileTotalLines(cvTotals)
	}
	// CGEC B1a: Phase 0 broaden attempts exhausted with 0 file
	// matches → emit RepairExpandSearch so the next retry's prompt
	// tells the LLM to try keyword stems / morphological variants
	// / synonyms instead of repeating the same terms. The Keywords
	// field carries the analyzer-declared keywords the LLM has
	// already been trying (so the retry hint can surface them as
	// "these didn't work — try different forms"). De-dup by
	// AddRepair chokepoint.
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
			Rationale: fmt.Sprintf("breadth scan exhausted %d broaden attempt(s) with 0 file matches — try stems, morphological variants, or conceptual synonyms of these keywords", e.broadenAttempts),
			Origin:    "explorer.breadth_scan.broaden_exhausted",
		})
		logging.Info("[CGEC] B1a expand_search: origin=phase0.broaden_exhausted attempts=%d keywords=%d", e.broadenAttempts, len(kws))
	}
	stopParseSection = startExplorerParseSectionWatchdog(ctx, "concrete_values")
	cvResult := e.getConcreteValuesCached(ctx.Context(), ctx.RepoRoot, cvReadSet, cvClosure)
	if len(cvResult.evidence) > 0 {
		e.structuredEvidence = mergeEvidenceItems(e.structuredEvidence, cvResult.evidence)
	}
	stopParseSection()
	if err := explorerParseContextErr(ctx, "concrete_values"); err != nil {
		return nil, err
	}

	stopParseSection = startExplorerParseSectionWatchdog(ctx, "exact_context")
	e.refreshExactContextFiles(ctx)
	exactAbsenceSalvaged := e.salvageExactAbsenceCompletion(ctx)
	stopParseSection()
	if err := explorerParseContextErr(ctx, "exact_context"); err != nil {
		return nil, err
	}

	// HasEnoughFacts: multi-dimensional quality check.
	// 1. Tool diversity: at least 2 distinct evidence tools (grep + read_file).
	// 2. File coverage: ≥50% of discovered files read, or ≥3 files.
	// 3. Evidence quality: count structured evidence tags in notes.
	//    Require at least 2 [DIRECT]/[REGISTRATION] entries (ground-truth facts).
	// 4. File relevance: weight read files by their keyword search rank.
	stopParseSection = startExplorerParseSectionWatchdog(ctx, "completion_readiness")
	readiness := e.completionReadinessWithCoverage(toolResults, len(sources), exactAbsenceSalvaged, true, discoveredFiles, cvReadSet)
	stopParseSection()
	if err := explorerParseContextErr(ctx, "completion_readiness"); err != nil {
		return nil, err
	}

	// Primary signal: the LLM explicitly called emit_investigation_complete.
	// When set, it overrides ALL heuristic calculations — the LLM
	// declared it has enough evidence, and we trust it.
	missingStructuredMemberSet := e.needsStructuredMemberSetHandoff(ctx) && !acceptedStructuredMemberSetHandoff(ctx)
	if missingStructuredMemberSet {
		readiness.HasEnough = false
	}
	if ctx != nil && ctx.Mutable != nil && ctx.Mutable.IsInvestigationComplete() && !readiness.HasEnough && !missingStructuredMemberSet {
		logging.Debug("[explorer] HasEnoughFacts promoted by emit_investigation_complete (heuristic was: toolDiv=%v fileCov=%v evQual=%v)",
			readiness.ToolDiversity, readiness.FileCoverage, readiness.EvidenceQuality)
		readiness.HasEnough = true
	}
	signals := &types.ExecutionSignals{HasEnoughFacts: readiness.HasEnough}
	readSet := cvReadSet

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
	stopParseSection = startExplorerParseSectionWatchdog(ctx, "rank_evidence")
	rankedEvidence := rankEvidenceByRelevanceWithSubject(e.userQuestion, e.structuredEvidence, readSet, e.answerSubject, rankGraph, e.predicateAxis)
	rankedFindings := rankFindingsByRelevance(e.userQuestion, e.flowFindings)
	stopParseSection()
	if err := explorerParseContextErr(ctx, "rank_evidence"); err != nil {
		return nil, err
	}

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
	// don't block the finalizer on an empty set.
	//
	// Extended to single-subject enumeration (2026-04-22 s1a audit):
	// questions like "gate.Run 的 7 项检查" resolve to exactly one
	// primary file (gate.go). Consumers and orthogonal-concept files
	// (classifyGateFailure / HypRejected / extractor.go) leaked into
	// Structured Evidence via the LLM's second-half emit_evidence burst
	// and drowned the enumeration items for the finalizer. When
	// primary file set size is 1 the answer is structurally confined
	// to that file, so the filter is safe to apply; when primary > 1
	// (cross-package enumeration like "list all agents") the filter
	// would incorrectly narrow scope — registration/call_chain/
	// multi-primary enumeration stay unaffected.
	stopParseSection = startExplorerParseSectionWatchdog(ctx, "shape_filter")
	questionKind := strings.ToLower(strings.TrimSpace(irQuestionKind(ctx)))
	var primaryFiles []string
	if questionKind == "mechanism" || questionKind == "enumeration" {
		// effectivePrimaryFiles unions the analyzer's entity→file
		// resolution with files the explorer has already cited via
		// emit_evidence. Without this union the dotted-form
		// resolution gap (gate.Run → graph stores "Run", not
		// "gate.Run") would let the single-primary filter mis-fire
		// onto a side-entity file and drop every grounded LLM
		// citation. See effectivePrimaryFiles + qualified_name.go.
		if primary := e.effectivePrimaryFiles(); len(primary) > 0 {
			primaryFiles = primary
			applyFilter := questionKind == "mechanism" || len(primary) == 1
			if applyFilter {
				filtered := filterEvidenceByPrimaryFiles(rankedEvidence, primary)
				if len(filtered) > 0 {
					logging.Debug("[explorer] %s-kind evidence filter: %d → %d items (primary files: %v)",
						questionKind, len(rankedEvidence), len(filtered), primary)
					rankedEvidence = filtered
				} else {
					logging.Debug("[explorer] %s-kind evidence filter: 0 items match primary files %v, keeping full set (%d)",
						questionKind, primary, len(rankedEvidence))
				}
			}
		}
	}
	// Multi-path balance: for questions naming ≥ 2 primary files
	// (comparison / cross-file explain), round-robin the ranked
	// evidence so each primary file surfaces in the leading entries.
	// Without this, score-based ordering lets a count-heavy cluster
	// from one file dominate Primary Evidence and bias the finalizer
	// toward that side. Single-primary questions skip this — there is
	// no imbalance to correct.
	if len(primaryFiles) >= 2 {
		balanced := balanceEvidenceAcrossPrimaryFiles(rankedEvidence, primaryFiles)
		if len(balanced) == len(rankedEvidence) {
			rankedEvidence = balanced
		}
	}
	stopParseSection()
	if err := explorerParseContextErr(ctx, "shape_filter"); err != nil {
		return nil, err
	}

	// Identify answer chains: deterministic resolution chains that
	// directly answer the user's question. These get a dedicated
	// section in the finalizer prompt with higher priority than
	// generic evidence items.
	var ermGraph *repomap.Graph
	if e.searchResult != nil {
		ermGraph = e.searchResult.Graph
	}
	// df3 drift fix: mechanism questions do not benefit from the
	// chain-ranked Ground Truth section. identifyAnswerChains tends
	// to surface whatever bind/return chains rank high, which for
	// multi-type polymorphic methods (e.g. ContinuationPrompt on
	// both explorerEvaluator and subExplorerEvaluator) pulls
	// sibling evaluators into the Ground Truth and poisons the
	// final answer. Evidence Items (filtered above) carry the
	// [MECHANISM]/[CONDITIONAL] tags with file:line citations which
	// is the right anchoring for a mechanism step_list answer.
	var answerChains []types.AnswerChain
	if strings.EqualFold(questionKind, "mechanism") {
		logging.Debug("[explorer] mechanism-kind: skipping answer-chain identification (step_list shape uses Evidence Items)")
	} else {
		stopParseSection = startExplorerParseSectionWatchdog(ctx, "answer_chains")
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
		answerChains = identifyAnswerChains(e.userQuestion, e.structuredEvidence, 5,
			buildAnswerWhitelist(e.ermRequirements), e.ermRequirements, ermGraph)
		stopParseSection()
		if err := explorerParseContextErr(ctx, "answer_chains"); err != nil {
			return nil, err
		}
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
	stopParseSection = startExplorerParseSectionWatchdog(ctx, "stage_report")
	canonicalReport := renderExplorerStageReport(
		irQuestionKind(ctx),
		irQuestionFamily(ctx),
		irExactResolutionContract(ctx),
		rankedEvidence,
		answerChains,
		nil, // symbols: deferred to Turn B
		rankedFindings,
		readFilesList,
		e.isEnumerationQuery,
	)
	stopParseSection()
	if err := explorerParseContextErr(ctx, "stage_report"); err != nil {
		return nil, err
	}

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
		stopParseSection = startExplorerParseSectionWatchdog(ctx, "turn_a_handoff")
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
		if len(strictEvidence) == 0 && shouldSeedTurnAStrictEvidenceFromRanked(ctx, questionKind) && len(rankedEvidence) > 0 {
			// Mechanism answers intentionally drop answer chains above:
			// step-list finalization should read the grounded mechanism
			// evidence directly, not a terminal-symbol slate. Preserve
			// the same concise top-N digest Turn B renders, so it does
			// not see an empty handoff while avoiding a noisy hundreds-
			// item transcript snapshot from deterministic scanners.
			limit := len(rankedEvidence)
			if limit > extractorMaxEvidence {
				limit = extractorMaxEvidence
			}
			strictEvidence = append(strictEvidence, rankedEvidence[:limit]...)
		}
		snapshot := types.TurnAArtifacts{
			UserQuestion:           e.userQuestion,
			InvestigationNotes:     e.investigationNotes,
			ReadFiles:              readFilesList,
			ToolResults:            toolResults,
			AcceptedClosureReason:  strings.TrimSpace(ctx.Mutable.StableInvestigationCompleteReason()),
			AcceptedResultKind:     strings.TrimSpace(ctx.Mutable.StableInvestigationResultKind()),
			AcceptedAggregateFacts: ctx.Mutable.StableInvestigationAggregateFacts(),
			RuntimeObservationOnlyCompletion: observationOnlyRuntimeArtifactForExplorer(ctx) &&
				strings.TrimSpace(ctx.Mutable.StableInvestigationCompleteReason()) != "" &&
				strings.TrimSpace(ctx.Mutable.StableInvestigationResultKind()) != "",
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
		stopParseSection()
		if err := explorerParseContextErr(ctx, "turn_a_handoff"); err != nil {
			return nil, err
		}
	}

	if !signals.HasEnoughFacts {
		var hintKey string
		if missingStructuredMemberSet {
			hintKey = "explorer.retry.structured-member-set"
			out.RetryHint = "Previous attempt gathered an exhaustive principal-member enumeration but did not close through a model-authored aggregate_facts.member_set. Reuse the already-read evidence, call emit_investigation_complete(result_kind=\"resolved\"), and include aggregate_facts with kind=\"member_set\", value=len(members), and every principal answer member in members[]. Do not leave the complete set only in thinking, read_file output, or closure prose."
		} else if !readiness.ToolDiversity {
			hintKey = "explorer.retry.tool-diversity"
			out.RetryHint = "Previous attempt used fewer than 2 distinct evidence tool types. Use both grep and read_file."
		} else if !readiness.EvidenceQuality {
			hintKey = "explorer.retry.evidence-quality"
			out.RetryHint = fmt.Sprintf("Previous attempt collected %d [DIRECT]/[REGISTRATION] evidence entries, but this answer shape needs ≥%d. Read more files and extract structured evidence with [DIRECT], [REGISTRATION], [CONDITIONAL] tags.", readiness.DirectCount, readiness.MinDirectCount)
		} else if !readiness.ExplanationAnchorReady {
			hintKey = "explorer.retry.explanation-anchor"
			out.RetryHint = fmt.Sprintf("Previous attempt covered %d of %d required explanation anchors. Read the missing topic anchors and emit grounded evidence for each before completing.", readiness.ExplanationAnchorCovered, readiness.ExplanationAnchorTotal)
		} else if len(e.ermRequirements) > 0 && !ermAllSatisfied(e.ermRequirements) {
			hintKey = "explorer.retry.erm-unsatisfied"
			out.RetryHint = "Previous attempt left evidence requirements unsatisfied. " + ermUnsatisfiedGaps(e.ermRequirements)
		} else {
			hintKey = "explorer.retry.file-coverage"
			out.RetryHint = fmt.Sprintf("Previous attempt read only %d of %d discovered relevant files (%.0f%% coverage, %d relevant). Read more of the discovered files.", readiness.ScopeReadCount, max(readiness.ScopeTotalCount, readiness.DiscoveredCount), readiness.Coverage*100, readiness.RelevantRead)
		}
		logging.Debug("[explorer] retry hint built key=%q len=%d body=%q",
			hintKey, len(out.RetryHint), logging.Truncate(out.RetryHint, logging.HintBodyMax))
	}

	return out, nil
}

func shouldSeedTurnAStrictEvidenceFromRanked(ctx *types.AgentContext, questionKind string) bool {
	if strings.EqualFold(strings.TrimSpace(questionKind), string(types.ReqMechanism)) {
		return true
	}
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	if !needsAnswerSymbols(ctx) {
		return true
	}
	return types.HasCapabilitySurfaceHint(ctx.AnalysisIR.RequestModel)
}

func (e *explorerEvaluator) DetermineMissingPiece(ctx *types.AgentContext, output *StageOutput) types.MissingPiece {
	if output.SignalUpdates != nil && output.SignalUpdates.HasEnoughFacts {
		return types.MissingNone
	}
	return types.MissingFacts
}

func (e *explorerEvaluator) ensureStructuredEvidence(ctx *types.AgentContext, toolResults []types.ToolResult) {
	if len(e.structuredEvidence) > 0 || len(e.flowFindings) > 0 {
		e.mergeEmittedEvidenceDelta(ctx)
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
			e.mergedEmittedEvidenceLen = len(emitted)
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
	intent := dataflowIntent(requestModelFromContext(ctx), parsed)
	hasGraph := e.searchResult != nil && e.searchResult.Graph != nil
	logging.Debug("[explorer] ensureStructuredEvidence: parsed=%d dataflowIntent=%s hasGraph=%v", len(parsed), intent, hasGraph)
	if !hasGraph || intent == IntentNone {
		e.structuredEvidence = parsed
		return
	}

	_, readSet, _ := extractFileCoverage(toolResults, e.repoRoot)

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
		cv := e.getConcreteValuesCached(ctx.Context(), ctx.RepoRoot, readSet, ermClosure)
		// T2.2: produce structured EvidenceMechanism items for ERM
		// mechanism requirements. No-op for non-mechanism questions.
		mechEvidence = scanMechanismEvidence(e.ermRequirements, e.searchResult.Graph, ctx.RepoRoot)
		trial := mergeEvidenceItems(parsed, cv.evidence, mechEvidence)
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
	candidateSet := e.activeFrontierFileSet(readSet, strings.Join(e.investigationNotes, "\n"))
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
	if e.analysisIR != nil {
		entityBias = filterEntitiesByProvenance(
			entityBias,
			e.analysisIR.RequestModel.AnalyzerHints.EntityProvenance,
			entityProvenanceRoleSearch,
		)
	}
	result := dataflow.Analyze(e.searchResult.Graph, dataflow.Options{
		Context:         ctx.Context(),
		RepoRoot:        ctx.RepoRoot,
		Question:        e.userQuestion,
		CandidateFiles:  candidates,
		WorkDir:         ctx.WorkDir,
		MaxFiles:        40,
		MaxIterations:   6,
		MaxNodesPerFunc: 400,
		MaxItemsPerFile: 50,
		SkipFindings:    intent == IntentLookup,
		EntityBias:      entityBias,
	})
	logging.Debug("[explorer] dataflow.Analyze(intent=%s): %d evidence, %d findings from %d candidates",
		intent, len(result.Evidence), len(result.Findings), len(candidates))
	e.structuredEvidence = mergeEvidenceItems(parsed, result.Evidence, mechEvidence)
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
	// FileLines is a parallel slice aligned 1:1 with Files: entry i
	// holds the 1-based source line the chain terminal (or producer)
	// actually sits on, or 0 when the producer has no line metadata
	// (bridge-literal chains built from graph relations have no
	// point-of-use). The chain promotion enforcer calls
	// closure.HasReadLine(Files[i], FileLines[i]) so a partial read
	// — e.g. read_file pagination that fetched lines 1-200 while
	// the terminal lives at line 350 — correctly demotes the chain.
	// A zero line degrades to the file-level HasRead grant.
	FileLines []int
	// IsDefFile is a parallel slice aligned 1:1 with Files: entry i
	// is true when the file at Files[i] contains the chain terminal
	// symbol's DEFINITION (graph.FileIndex symbol with matching
	// Name + Receiver), false when it is only a USAGE site.
	// applyChainPromotion's X2 filter drops promotions whose anchor
	// is USAGE-only — forcing the LLM to read a file that just
	// references a symbol (vs the file that defines it) wastes a
	// read budget on a tangential location.
	//
	// Empty / nil slice => fail-open (every file treated as DefFile).
	// This preserves behaviour for paths that have not been wired up
	// (older test fixtures, bridge-literal anchors before this audit
	// is rolled out across all producers).
	IsDefFile []bool
	Origin    string // "concrete_values_tracer", "bridge_literal", "hierarchy"
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
// evidence items. Missing anchor files are only appended to
// closure.PendingReads when the anchor source is strong enough to
// justify another read; noisy concrete-value chains are demoted
// instead of expanding the investigation to a brand-new file. The
// cache itself stores the unfiltered version because closure state
// mutates over the explore loop and freezing a closure-aware snapshot
// would serve stale filters to a later caller.
//
// subject (CGEC C2) is the answer-subject classification produced by
// the analyzer. When non-Unknown the chain ranker scores each chain
// terminal against the expected subject kind and re-orders chains so
// the most subject-relevant ones win the chainCap slot. Zero value
// (SubjectUnknown) preserves the historical insertion-order ranking.
// Subject is constant for the Run, so caching on (repoRoot, readSet)
// alone is sound.
func (e *explorerEvaluator) getConcreteValuesCached(ctx context.Context, repoRoot string, readSet map[string]bool, closure *types.EvidenceClosure) concreteValuesResult {
	if ctx == nil {
		ctx = context.TODO()
	}
	if e.cachedConcreteValues == nil {
		r := e.buildConcreteValuesSection(ctx, repoRoot, readSet, closure)
		if ctx.Err() != nil {
			return r
		}
		e.cachedConcreteValues = &r
	}
	if closure == nil {
		return *e.cachedConcreteValues
	}
	return applyChainPromotion(*e.cachedConcreteValues, readSet, closure, repoRoot, e.answerSubject.Confidence, e.pendingSubRepos)
}

// applyChainPromotion is the CGEC chain-promotion enforcer. Given an
// unfiltered concreteValuesResult and a snapshot of Turn A's
// ReadSet, it returns a new result whose markdown's "### Resolution
// Chains" section drops any chain anchored outside ReadSet, whose
// evidence slice drops dataflow_path EvidenceItems with Source
// outside ReadSet, and which records only high-confidence missing
// anchors as PendingReads on the closure (so the next explore round's
// retry hint surfaces them via the structured RepairReadFile
// directive).
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
func applyChainPromotion(in concreteValuesResult, readSet map[string]bool, closure *types.EvidenceClosure, repoRoot string, subjectConfidence float64, pendingSubRepos []string) concreteValuesResult {
	if closure == nil || len(in.chainAnchors) == 0 {
		return in
	}
	keptSummaries := make(map[string]bool, len(in.chainAnchors))
	demotedSummaries := make(map[string]bool, len(in.chainAnchors))
	// Pass 1: classify each anchor as kept (all files in ReadSet) or
	// demoted (at least one file unread). Side-effect-free: closure
	// is only mutated in pass 2's enqueue loop. Splitting these two
	// concerns lets the X1 short-circuit decide BEFORE we touch the
	// PendingRead queue.
	type pendingDemote struct{ anchor chainAnchorInfo }
	var demoteList []pendingDemote
	for _, anchor := range in.chainAnchors {
		// Row-level check: for each anchor file, require either
		//   (a) file-level readSet covers it (pure fallback for
		//       callers / tests that do not populate ranges), OR
		//   (b) closure.HasReadLine(f, line) confirms the specific
		//       terminal line sits inside a fetched slice.
		// (b) subsumes (a) when the closure is non-nil and range
		// info is tracked — HasReadLine grants file-level coverage
		// for any file in readSet without range records. This keeps
		// the semantics backward-compatible for chain_promotion_test.go
		// suites that never touched ranges, while letting paginated
		// read_file calls catch partial reads.
		allRead := true
		for i, f := range anchor.Files {
			line := 0
			if i < len(anchor.FileLines) {
				line = anchor.FileLines[i]
			}
			if closure.HasReadLine(f, line) {
				continue
			}
			if readSetContains(readSet, f) && line == 0 {
				continue
			}
			allRead = false
			break
		}
		if allRead {
			keptSummaries[anchor.Summary] = true
			continue
		}
		demotedSummaries[anchor.Summary] = true
		demoteList = append(demoteList, pendingDemote{anchor})
	}
	// X1 short-circuit (PER-CHAIN): when a demoted chain's anchor
	// files are ENTIRELY outside the investigator's read-set at the
	// file level, the chain producer's connections do not intersect
	// with the investigator's reading scope for that chain at all.
	// Promoting any anchor of that chain burns an explore re-dispatch
	// reading a file the investigator already chose to skip.
	//
	// Crucially, this is per-chain — a chain whose anchors are
	// [a.go, b.go] with a.go inside readSet and b.go outside is the
	// "on-target but incomplete" shape. Such chains may still promote
	// PendingReads when the anchor source is high-confidence; noisy
	// concrete-value tracer chains are only demoted, while already
	// touched files with a missing line slice promote surgical reads.
	//
	// The markdown / cvEvidence demote of every demoted chain still
	// fires so the LLM doesn't see misleading chains; only the
	// per-chain PendingRead enqueue is skipped when the entire
	// anchor set is off-target.
	//
	// Pre-2026-05-06 attempts gated this on subjectConfidence first
	// (>= 0.7 / >= 0.5) — unreachable since AnswerSubject.Confidence
	// in this codebase tops out at 0.4. Then on aggregate kept=0
	// only — too narrow, missed the "kept=4 / demoted=3 with all
	// demoted anchors outside readSet" case (qf_arch architecture
	// questions where 4 chains land on read files and 3 chains land
	// on a downstream-consumer file the LLM never opened).
	// subjectConfidence is retained as a parameter for future
	// per-confidence telemetry but no longer gates the skip.
	x1Skipped := 0

	// Pass 2: enqueue PendingReads for demoted anchors (X1 per-chain
	// + X2 per-file filtered).
	for _, pd := range demoteList {
		anchor := pd.anchor

		// X1 per-chain: skip the entire PendingRead enqueue when no
		// anchor file of THIS chain is in readSet at the file level.
		anyAnchorFileInReadSet := false
		for _, f := range anchor.Files {
			if readSetContains(readSet, f) {
				anyAnchorFileInReadSet = true
				break
			}
		}
		if !anyAnchorFileInReadSet {
			x1Skipped++
			logging.Debug("[CGEC] X1 chain_promotion: chain %q anchored entirely outside ReadSet (origin=%s, subjectConfidence=%.2f) — skipping PendingRead",
				anchor.Summary, anchor.Origin, subjectConfidence)
			continue
		}
		// Append PendingRead for every anchor file the closure has
		// not seen AND whose origin is strong enough to justify a
		// system-initiated read. Origin tags the source so the operator
		// can grep the trace for which enforcer raised the read.
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
		for i, f := range anchor.Files {
			line := 0
			if i < len(anchor.FileLines) {
				line = anchor.FileLines[i]
			}
			if closure.HasReadLine(f, line) {
				continue
			}
			if readSetContains(readSet, f) && line == 0 {
				continue
			}
			if readSetContains(readSet, f) && line > 0 && len(closure.ReadRanges(f)) == 0 {
				continue
			}
			// X2 def-vs-usage filter: skip when the anchor file does
			// NOT contain the chain terminal symbol's definition —
			// the file just USES the symbol, so forcing the LLM to
			// read it pollutes ReadSet with a tangential location.
			// Fail-open default (IsDefFile slice empty / true) keeps
			// legacy paths working; only triggers when computeIsDefFile
			// confidently determined the anchor file is USAGE-only.
			if i < len(anchor.IsDefFile) && !anchor.IsDefFile[i] {
				logging.Debug("[CGEC] X2 chain_promotion: skipping usage-only anchor file=%s origin=%s — terminal not defined here",
					f, anchor.Origin)
				continue
			}
			if !closure.IsScanned(f) {
				logging.Debug("[CGEC] D2 chain_promotion: skipping ghost anchor file=%s origin=%s (not in ScannedSet)", f, anchor.Origin)
				// Session 11 F1: record structured ledger entry so F2
				// aggregator can promote repeated ghost anchors on the
				// same file to a RetrievalGap signal (→ R5 expand_search).
				closure.AppendViolation(types.Violation{
					Kind:         types.ViolGhostAnchor,
					Detail:       fmt.Sprintf("chain anchor file=%s origin=%s not in ScannedSet", f, anchor.Origin),
					ClusterKey:   types.IdentityClusterKey("file:"+f, "ScannedSet"),
					Stage:        string(types.StageExplore),
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
			// C belt-and-suspenders for the 2026-05-16 finalize loop:
			// chain anchors resolved via SymbolLocator can still point
			// at a pending sub-repo file even after the locator-side
			// filter (residual A). Refuse to queue forced-reads on
			// paths the FS tools would refuse to read, otherwise we
			// re-create the lockup pattern via a different door.
			if types.PathInsidePendingSubRepo(f, pendingSubRepos) {
				logging.Debug("[CGEC] chain_promotion: skipping pending-sub-repo anchor file=%s origin=%s (active-set refused)",
					f, anchor.Origin)
				continue
			}
			// Concrete-value resolution chains are useful synthesis
			// hints, but they are derived from broad deterministic
			// scans rather than a model-authored citation or a user-
			// requested file. If such a chain points at a brand-new
			// file, demoting the chain is safer than pulling the whole
			// pipeline back into exploration. When the file was already
			// touched and only the specific anchor line is outside the
			// fetched slice, keep the corrective read, but make it
			// surgical via LineRanges below.
			if anchor.Origin == "concrete_values_tracer" && !readSetContains(readSet, f) {
				logging.Debug("[CGEC] chain_promotion: demoted concrete-values chain %q without forced-read; missing new file=%s",
					anchor.Summary, f)
				continue
			}
			lineRanges := chainPromotionPendingLineRanges(closure, f, line)
			rationale := "Resolution Chain anchors here but file is outside the investigation's read-files list — read it before next emit_investigation_complete"
			if len(lineRanges) > 0 {
				rationale = "Resolution Chain anchor line is outside the fetched slices — read the surgical range before next emit_investigation_complete"
			}
			closure.AddPendingRead(types.PendingRead{
				File:       f,
				Rationale:  rationale,
				Origin:     "chain_promotion." + anchor.Origin,
				LineRanges: lineRanges,
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
	logging.Debug("[explorer] chain promotion: kept %d / demoted %d chains; eligible pending reads queued",
		len(keptSummaries), len(demotedSummaries))
	return out
}

// chainPromotionPendingLineRanges turns a precise chain anchor line
// into the smallest read_file demand needed to cover it. Nil preserves
// the legacy full-file fallback for older/no-line anchors; non-nil
// lets runForcedReads issue surgical reads instead of paginating the
// whole file.
func chainPromotionPendingLineRanges(closure *types.EvidenceClosure, file string, line int) []types.LineRange {
	if line <= 0 {
		return nil
	}
	window := tool.CurrentAnalysisLimits().MultiPathSymbolContextLines
	if window <= 0 {
		window = 15
	}
	if closure != nil {
		if missing := closure.MissingContextRegions(file, []int{line, line}, window); len(missing) > 0 {
			return missing
		}
	}
	start := line - window
	if start < 1 {
		start = 1
	}
	return []types.LineRange{{Start: start, End: line + window}}
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

func (e *explorerEvaluator) filterConcreteValueScanFiles(files map[string]bool) map[string]bool {
	if len(files) == 0 {
		return files
	}
	if e.exactResolution == nil || !types.ExactResolutionRequiresDefiningPrimaryProof(e.exactResolution) {
		return files
	}
	filtered := make(map[string]bool, len(files))
	for file := range files {
		if types.LooksLikeAuxiliaryEvidencePath(file) {
			continue
		}
		filtered[file] = true
	}
	if len(filtered) == 0 {
		return files
	}
	return filtered
}

func (e *explorerEvaluator) buildConcreteValuesSection(ctx context.Context, repoRoot string, readSet map[string]bool, closure *types.EvidenceClosure) concreteValuesResult {
	if ctx == nil {
		ctx = context.TODO()
	}
	if e.searchResult == nil || e.searchResult.Graph == nil {
		return concreteValuesResult{}
	}
	graph := e.searchResult.Graph
	notesJoined := strings.Join(e.investigationNotes, "\n")

	var allValues []concreteValue

	// Build the active frontier: files we have read, the exact/primary
	// anchors, analyzer-required files, and direct note-referenced
	// neighbors. Deliberately excludes the whole allScoredFiles tail so
	// first-round ranking noise does not become second-round scan scope.
	filesToScan := e.activeFrontierFileSet(readSet, notesJoined)
	filesToScan = e.filterConcreteValueScanFiles(filesToScan)
	focusSymbols := e.concreteValueFocusSymbols(graph, filesToScan)

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
	getDeclarationSnippet := func(absPath string, startLine, endLine int) string {
		lines := loadFileLines(absPath)
		if lines == nil || startLine < 1 || startLine > len(lines) {
			return ""
		}
		start := startLine - 1
		end := len(lines)
		if endLine >= startLine && endLine <= len(lines) {
			end = endLine
		}
		if maxEnd := start + 12; end > maxEnd {
			end = maxEnd
		}
		if end <= start {
			return ""
		}
		return strings.Join(lines[start:end], "\n")
	}

	// Extract concrete values from executable symbols. Three tiers:
	//
	// 1. Short bodies (≤3 lines): full extraction of all patterns.
	// 2. Registration-like bodies (≤30 lines, name contains Register/Config/...):
	//    full extraction but only bindings/maps kept.
	// 3. Medium bodies (4-100 lines): local line scan — only lines
	//    containing return/map/register patterns are extracted with ±1
	//    line of context. This recovers concrete values from longer
	//    bodies without reading them entirely.
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
		if ctx.Err() != nil {
			return concreteValuesResult{}
		}
		st := &scanStats{}
		fileStats[file] = st
		fi, ok := graph.FileIndex[file]
		if !ok {
			st.graphMiss = true
			continue
		}
		st.symTotal = len(fi.Symbols)
		for _, sym := range fi.Symbols {
			if ctx.Err() != nil {
				return concreteValuesResult{}
			}
			if focus := focusSymbols[strings.TrimPrefix(file, "./")]; len(focus) > 0 && !concreteValueMatchesFocus(&sym, focus) {
				continue
			}
			bodyKind := isConcreteValueBodySymbolKind(sym.Kind)
			declKind := isConcreteValueDeclarationSymbolKind(sym.Kind)
			if !bodyKind && !declKind {
				st.symWrongKind++
				continue
			}
			if !bodyKind {
				continue
			}
			if sym.EndLine == 0 {
				st.symNoEndLine++
				continue
			}
			bodyLines := sym.EndLine - sym.Line
			isShort := bodyLines <= 3
			// Phase 6 stage 15 (2026-05-03) — the registration-name
			// substring list is yaml-tunable via
			// codrax.yaml :: explore.registration_function_name_tokens.
			// Default list (12 English tokens) is unchanged so
			// pre-stage-15 production behaviour is byte-identical;
			// non-English / project-specific naming patterns can
			// extend the list without recompiling. The retired
			// hardcoded inline keyword table directly violated the
			// no-keyword-tables red line by being unconfigurable.
			isRegistrationFunc := !isShort &&
				bodyLines <= 30 &&
				isRegistrationLikeName(sym.Name, e.heuristics.RegistrationFunctionNameTokens)
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
					// Phase 6 stage 18 (2026-05-03) — typed AST query
					// replaces the retired isEvidenceLine token-table
					// scanner. fi.LineFeatures[li+1] is the per-line
					// feature set populated by repomap's tree-sitter
					// extractor; empty ⇒ skip (no AST signal).
					if !isEvidenceLineByFeatures(fi.LineFeatures[li+1]) {
						continue
					}
					_ = strings.TrimSpace(allLines[li])
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
					for _, cv := range extractConcreteValues(snippet, fi.Language) {
						allValues = append(allValues, concreteValue{
							file:     file,
							receiver: owner,
							method:   qualName,
							kind:     cv.kind,
							value:    cv.value,
							line:     concreteValueAbsoluteLine(ctxStart+1, cv.lineOffset),
						})
					}
				}
				continue
			}

			src := getLinesRange(filepath.Join(repoRoot, sym.File), sym.Line, sym.EndLine)
			if src == "" {
				continue
			}
			for _, cv := range extractConcreteValues(src, fi.Language) {
				// For longer functions, only keep binding/registration/map
				// values and cross-component call targets. Bulk "returns"
				// / "assigns" entries would flood the synthesis prompt,
				// but "calls" carries control-flow signal that the
				// evidence-chain resolver uses for cross-package
				// dispatch chains.
				if !isShort && !isBindsKind(cv.kind) &&
					cv.kind != "maps" && cv.kind != "calls" &&
					cv.kind != "embeds" && cv.kind != "implements" &&
					cv.kind != "conditional" && cv.kind != "errors" {
					continue
				}
				allValues = append(allValues, concreteValue{
					file:     file,
					receiver: owner,
					method:   qualName,
					kind:     cv.kind,
					value:    cv.value,
					line:     concreteValueAbsoluteLine(sym.Line, cv.lineOffset),
				})
			}
		}
	}

	// Declaration-level pass: executable-body scanners miss facts that
	// live on type / protocol / RPC headers. Keep this pass narrow to
	// declaration-shaped concrete values so we don't duplicate return /
	// call rows from method bodies.
	for file := range filesToScan {
		if ctx.Err() != nil {
			return concreteValuesResult{}
		}
		fi, ok := graph.FileIndex[file]
		if !ok {
			continue
		}
		absPath := filepath.Join(repoRoot, fi.RelPath)
		for _, sym := range fi.Symbols {
			if ctx.Err() != nil {
				return concreteValuesResult{}
			}
			if focus := focusSymbols[strings.TrimPrefix(file, "./")]; len(focus) > 0 && !concreteValueMatchesFocus(&sym, focus) {
				continue
			}
			if !isConcreteValueDeclarationSymbolKind(sym.Kind) {
				continue
			}
			src := getDeclarationSnippet(absPath, sym.Line, sym.EndLine)
			if src == "" {
				continue
			}
			receiver, qualName := declarationConcreteValueContext(sym)
			for _, cv := range extractDeclarationConcreteValues(src, fi.Language) {
				allValues = append(allValues, concreteValue{
					file:     file,
					receiver: receiver,
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
		if ctx.Err() != nil {
			return concreteValuesResult{}
		}
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

	// Pre-filter: strip prose-like values at the source so they
	// never enter the relevance filter, multi-pass tracer, chain
	// builder, or evidence pipeline. This prevents 500+ char prompt
	// texts from `of.WriteString("...")` calls inside
	// BuildAnalysisSkill (and similar prompt-builder functions) from
	// polluting the entire downstream pipeline with phantom matches.
	{
		clean := allValues[:0]
		for i, v := range allValues {
			if i%1024 == 0 && ctx.Err() != nil {
				return concreteValuesResult{}
			}
			if !isProseLikeConcreteValue(v.value) {
				clean = append(clean, v)
			}
		}
		allValues = clean
	}

	// Build pre-scanned/frontier sets for filtering.
	preScannedSet := make(map[string]bool, len(e.preScannedFiles))
	for _, f := range e.preScannedFiles {
		preScannedSet[f] = true
	}
	// Frontier files are the converged scan scope for this run. Short
	// deterministic returns from these files are safe to keep even when
	// the LLM has not yet mentioned the receiver explicitly in notes.
	frontierSet := make(map[string]bool, len(filesToScan))
	for f := range filesToScan {
		frontierSet[f] = true
	}

	// Filter to keep only values relevant to the investigation:
	// 1. Registrations — always kept (rule A)
	// 2. Short string returns from pre-scanned/read/frontier files — always kept (rule B1)
	// 3. Short string returns from other files — only if receiver is in notes (rule B2)
	// 4. Values referencing symbols from the investigation notes (rule C)
	var relevant []concreteValue
	// Per-rule counters for observability: split B1 by which file-set
	// triggered retention so the active-frontier path stays visible.
	var cntA, cntB1Read, cntB1PreScan, cntB1Scored, cntB2, cntC, cntLongSkip int
	for i, v := range allValues {
		if i%1024 == 0 && ctx.Err() != nil {
			return concreteValuesResult{}
		}
		if isBindingShapeKind(v.kind) {
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
				if readSetContains(readSet, v.file) {
					relevant = append(relevant, v)
					cntB1Read++
					continue
				}
				if preScannedSet[v.file] {
					relevant = append(relevant, v)
					cntB1PreScan++
					continue
				}
				if frontierSet[v.file] {
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

	logging.Debug("[explorer] concrete values filter: total=%d relevant=%d (A/reg=%d, B1/read=%d, B1/preScan=%d, B1/frontier=%d, B2/notes-recv=%d, C/notes-word=%d, longSkip=%d)",
		len(allValues), len(relevant), cntA, cntB1Read, cntB1PreScan, cntB1Scored, cntB2, cntC, cntLongSkip)

	valueIndex := newConcreteValueReceiverIndex(allValues, graph)

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
		passLen := len(relevant)
		for i := 0; i < passLen; i++ {
			if i%512 == 0 && ctx.Err() != nil {
				return concreteValuesResult{}
			}
			v := relevant[i]
			candidates := valueIndex.valuesForReceivers(valueIndex.referencedReceivers(v))
			for _, av := range candidates {
				if seen[av.method] {
					continue
				}
				linked := false
				if av.receiver != "" {
					if len(av.receiver) >= 4 {
						linked = concreteValuesLinked(v, av, graph)
					} else if v.kind == concreteValueKindCalls {
						// Preserve the historical call-resolution
						// exception: graph-assisted call links can match
						// short receiver names safely because the match is
						// equality over typed SymbolDefs receivers.
						linked = callValueMatchesReceiver(v.value, av.receiver, graph)
					}
				}
				if linked {
					relevant = append(relevant, av)
					seen[av.method] = true
					added++
					continue
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
			if isBindingShapeKind(v.kind) {
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

	// S5: Adaptive cap scaled by complexity. Simple questions get a
	// tighter cap to reduce prompt token count (fewer tokens → faster
	// LLM first-token latency). Complex questions keep the wider cap
	// because they genuinely need more concrete values for multi-hop
	// chain building.
	valueCap := 15
	switch e.complexity {
	case types.ComplexitySimple:
		valueCap = 10
	case types.ComplexityComplex:
		if len(e.allScoredFiles) > 10 {
			valueCap = 25
		}
	default: // moderate
		if len(e.allScoredFiles) > 10 {
			valueCap = 20
		}
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
		// Render each kind with a distinctive prefix so the LLM can
		// distinguish control-flow, inheritance, and error rows at a
		// glance without reading the kind column.
		fact := v.kind + " " + v.value
		switch v.kind {
		case "calls":
			fact = "calls → " + v.value
		case "embeds":
			fact = "⊂ " + v.value
		case "implements":
			fact = "⊳ " + v.value
		case "conditional":
			fact = "guard: " + v.value
		case "errors":
			fact = "⚠ " + v.value
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
			blocks := extractDecisionBlocks(allLines, sym.Line, sym.EndLine, fi.LineFeatures)
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
	chainIndex := newConcreteValueReceiverIndex(allRelevantForEvidence, graph)
	reverseLinkedValues := make(map[string][]concreteValue)
	for _, rv := range allRelevantForEvidence {
		if rv.kind != concreteValueKindCalls &&
			rv.kind != concreteValueKindEmbeds &&
			rv.kind != concreteValueKindImplements {
			continue
		}
		for _, receiver := range chainIndex.referencedReceivers(rv) {
			reverseLinkedValues[receiver] = append(reverseLinkedValues[receiver], rv)
		}
	}
	for rootIdx, v := range allRelevantForEvidence {
		if rootIdx%512 == 0 && ctx.Err() != nil {
			return concreteValuesResult{}
		}
		// Skip values that don't reference other types.
		// T2b: "calls" joins the allowlist so cross-package dispatch
		// chains like `dispatchStage calls SubAgentRuntime.Run →
		// SubAgentRuntime.Run calls SubExplorer.Run` can form. The
		// call target renders as `ReceiverType.Method` which
		// containsIdentifier tests just like any other type-
		// referencing value (the LastIndex dot-split in
		// scanCallTargetsInLine keeps the receiver identifier
		// prominent so the identifier match fires).
		if v.kind != concreteValueKindReturns && !isBindsKind(v.kind) &&
			v.kind != "maps" && v.kind != "config" &&
			v.kind != "decorates" && v.kind != "calls" &&
			v.kind != "embeds" && v.kind != "implements" {
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
		candidates := chainIndex.valuesForReceivers(chainIndex.referencedReceivers(v))
		if v.receiver != "" {
			candidates = append(candidates, reverseLinkedValues[v.receiver]...)
		}
		seenCandidates := make(map[string]bool, len(candidates))
		for _, rv := range candidates {
			candidateKey := concreteValueKey(rv)
			if seenCandidates[candidateKey] {
				continue
			}
			seenCandidates[candidateKey] = true
			if rv.receiver == "" || rv.receiver == v.receiver {
				continue
			}
			if isProseLikeConcreteValue(rv.value) {
				continue
			}
			if concreteValuesLinked(v, rv, graph) {
				summary := fmt.Sprintf(
					"`%s()` %s %s → `%s()` %s %s",
					v.method, v.kind, v.value,
					rv.method, rv.kind, rv.value)
				chains = append(chains, summary)
				files, lines := dedupeAnchorFilesWithLines(
					anchorFileLine{File: v.file, Line: v.line},
					anchorFileLine{File: rv.file, Line: rv.line},
				)
				isDefFile := computeIsDefFile(graph, files, summary)
				chainAnchors = append(chainAnchors, chainAnchorInfo{
					Summary:   summary,
					Files:     files,
					FileLines: lines,
					IsDefFile: isDefFile,
					Origin:    "concrete_values_tracer",
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
	// S5: Adaptive chain cap scaled by complexity.
	chainCap := 8
	switch e.complexity {
	case types.ComplexitySimple:
		chainCap = 5
	case types.ComplexityComplex:
		chainCap = 18
	default:
		if len(e.allScoredFiles) > 10 {
			chainCap = 12
		}
	}
	if len(chains) > chainCap {
		chains = chains[:chainCap]
		// Cap chainAnchors in lockstep so applyChainPromotion only
		// enqueues PendingReads for chains the LLM actually sees in
		// the synthesis markdown. Without this, low-ranked chains
		// (subject score below the cap, never shown to the LLM) still
		// flow through chainAnchors and trigger forced-read explore
		// retries for anchor files that have nothing to do with the
		// answer the LLM is forming. Observed cost on architecture
		// questions: 1 wasted explore round per Run reading whichever
		// large file the chain pair anchored on.
		//
		// allChainsForEvidence (uncapped) was captured above so the
		// dataflow_path EvidenceItems pool stays fully populated —
		// that surface has its own top-K and benefits from the
		// long tail. PendingRead enforcement is a different surface
		// (forced reads) and must align with what the LLM saw.
		//
		// Bridge-literal anchors append to chainAnchors AFTER this
		// cap (line ~9798) and are intentionally never capped: those
		// are typed graph-wide JOINs, deliberately small in number,
		// and worth promoting end-to-end.
		chainAnchors = chainAnchors[:chainCap]
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
				if v.file != "" && !readSetContains(readSet, v.file) {
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
	for i, v := range allRelevantForEvidence {
		if i%1024 == 0 && ctx.Err() != nil {
			return concreteValuesResult{}
		}
		// Skip prose-like values from becoming evidence items. The
		// chain builder already filters these (lines 3731/3738) but
		// the evidence pipeline didn't — so 500+ char prompt-text
		// "binds" entries (e.g. BuildAnalysisSkill's WriteString
		// calls) leaked into the extractor/finalizer evidence set
		// and drowned the real signal. This was the #4 root cause
		// in the "有几个agent可以调用subagent" failure log.
		if isProseLikeConcreteValue(v.value) {
			continue
		}
		kind := types.EvidenceConcrete
		predicate := v.kind
		cvItem := types.EvidenceItem{
			Kind:       kind,
			Subject:    v.method,
			Predicate:  predicate,
			Object:     v.value,
			Source:     v.file,
			LineStart:  v.line,
			LineEnd:    v.line,
			Confidence: 0.95,
			Producer:   "concrete_values",
			Summary:    fmt.Sprintf("`%s()` %s %s", v.method, predicate, v.value),
			Scope:      types.ScopeLine,
			// 2026-05-02 L1: project the syntactic predicate into the
			// typed AnchorKind axis so Phase 0's ClaimFormOf can
			// classify the evidence (was 100% ClaimUnknown
			// pre-projection). See concreteValueKindToAnchorKind.
			AnchorKind: concreteValueKindToAnchorKind(predicate),
			// 2026-05-02 L2/L3: project file-ext + method-prefix
			// context into DiagramRole so config-layer / runtime-
			// binding-layer evidence reaches ClaimFormOf Rule 3
			// (DiagramRole != Default → ClaimPrecedenceRole) instead
			// of falling through to AnchorKind dispatch alone. Both
			// signal lists are operator-tunable (codrax.yaml ::
			// concrete_values_config_layer_extensions /
			// concrete_values_runtime_method_prefixes /
			// concrete_values_default_method_prefixes).
			DiagramRole: concreteValueDiagramRole(v.file, v.method),
			// Origin is intentionally left as ClaimOriginUnknown
			// here so the BackfillEvidenceProjector
			// (internal/authority/authority.go) decides:
			//   - log/perf frame match → ClaimOriginLog / Perf
			//     (ClaimFormOf Rule 1 → ClaimExternalObservation)
			//   - schema-level scope (File/Crossfile/Negative) →
			//     ClaimOriginCurrentRepo + AuthorityFactual
			//   - fallback (no log/perf match, line-shaped scope,
			//     grounded) → ClaimOriginCurrentRepo
			// Hardcoding Origin here would suppress the projector's
			// idempotent guard (Origin != Unknown skips backfill),
			// breaking log-frame matching on concrete_values items
			// that anchor on attached-log file:line locations.
		}
		cvItem.ID = types.StableEvidenceID(cvItem)
		cvEvidence = append(cvEvidence, cvItem)
	}
	for i, c := range allChainsForEvidence {
		if i%1024 == 0 && ctx.Err() != nil {
			return concreteValuesResult{}
		}
		// Dataflow chains have no source/line — they describe a
		// resolution chain across the repo. Treat as ScopeLine with
		// empty source so the line-shaped invariant still holds for
		// downstream consumers (the chain producer fills LineStart=0
		// intentionally; existing logic accepts that).
		chainItem := types.EvidenceItem{
			Kind:       types.EvidenceDataflowPath,
			Subject:    c,
			Predicate:  "resolution_chain",
			Confidence: 0.9,
			Producer:   "concrete_values",
			Summary:    c,
			Scope:      types.ScopeLine,
		}
		chainItem.ID = types.StableEvidenceID(chainItem)
		cvEvidence = append(cvEvidence, chainItem)
	}
	logging.Debug("[explorer] concrete values: %d chain evidence items (from %d uncapped chains)", len(allChainsForEvidence), len(allChainsForEvidence))

	// Bridge-literal extraction pass — deterministic cross-file JOIN
	// producing `A() binds ONLY NewB(...) → B.Name() returns "lit"`
	// chains even when the LLM didn't read the target file. Orthogonal
	// to the per-file extractConcreteValues + multi-pass tracer above,
	// this pass is graph-wide and bounded by symbol-name matching.
	// See memory/project_baseline_2026_04_13_post_phase4.md.
	bridgeEvidence := extractBridgeLiteralEvidence(graph, repoRoot, allRelevantForEvidence)
	bridgeTerminalItems := filterEvidenceItemsByFileSet(bridgeEvidence.terminalReturns, filesToScan)
	if len(bridgeTerminalItems) > 0 {
		logging.Debug("[explorer] bridge literal terminal returns: %d items", len(bridgeTerminalItems))
		cvEvidence = append(cvEvidence, bridgeTerminalItems...)
	}
	bridgeItems := filterEvidenceItemsByFileSet(bridgeEvidence.chains, filesToScan)
	if len(bridgeItems) > 0 {
		logging.Debug("[explorer] bridge literal chains: %d items", len(bridgeItems))
		// CGEC: record per-item anchor file so the promotion helper
		// can demote bridge-literal chains anchored outside ReadSet
		// the same way it handles per-file tracer chains.
		for _, it := range bridgeItems {
			if it.Source == "" {
				continue
			}
			// Bridge-literal evidence carries a LineStart so the row-
			// level enforcer can demote a chain whose definition sits
			// in a paginated-unread slice. LineStart==0 degrades to
			// the file-level grant via HasReadLine.
			files := []string{it.Source}
			isDefFile := computeIsDefFile(graph, files, it.Summary)
			chainAnchors = append(chainAnchors, chainAnchorInfo{
				Summary:   it.Summary,
				Files:     files,
				FileLines: []int{it.LineStart},
				IsDefFile: isDefFile,
				Origin:    "bridge_literal",
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
		beta          = 3.0
		rebindFloor   = 0.4
		highConfFloor = 0.5
	)
	// Topical tokens from analyzer-emitted EntityAxes. When the
	// expected subject kind is SubjectGeneric (Score returns 0.5 for
	// every token), subject match alone cannot differentiate chains —
	// a chain terminating at an unrelated symbol like
	// `checkRequirementSatisfaction` ties with a chain terminating at
	// `RegisterDefaultSubAgents` for a "how does explorer call
	// subagent" question. Topical relevance (token overlap between
	// chain text and question entities) breaks those ties and penalises
	// off-topic chains that otherwise leak into the citation pool.
	topicTokens := extractChainTopicTokens(expected.EntityAxes)
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
		relevance := chainTopicRelevance(c, anchor, topicTokens)
		rs[i] = ranked{
			summary:  c,
			anchor:   anchor,
			match:    match,
			adjusted: baseRank * (1.0 + alpha*match) * (1.0 + beta*relevance),
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

// extractChainTopicTokens parses the analyzer-emitted EntityAxes
// (strings shaped "A → B" or "A -> B" or bare "A") into a lowercase
// token set suitable for substring matching against chain text. Short
// tokens (< 3 chars) are dropped so noise like "A" / "of" doesn't
// pollute the match.
func extractChainTopicTokens(axes []string) map[string]bool {
	tokens := make(map[string]bool, len(axes)*2)
	for _, axis := range axes {
		for _, field := range strings.FieldsFunc(axis, func(r rune) bool {
			return r == '→' || r == ' ' || r == '\t' || r == ',' || r == ';'
		}) {
			t := strings.ToLower(strings.TrimSpace(field))
			t = strings.ReplaceAll(t, "->", "")
			t = strings.TrimFunc(t, func(r rune) bool {
				return r == '"' || r == '\'' || r == '`' || r == '.' || r == ':'
			})
			if len(t) >= 3 {
				tokens[t] = true
			}
		}
	}
	return tokens
}

// chainTopicRelevance returns the fraction of topic tokens that
// appear (case-insensitive substring) in the chain summary text.
// Range [0, 1]. Empty token set returns 0 (neutral — no topical
// signal means no re-ranking, leave subject match in charge).
//
// Anchor file paths are deliberately EXCLUDED: a file named
// `explorer_erm.go` matches the token "explorer" but its chain
// content is unrelated to an explorer-dispatch question. The
// summary is what the finalizer will render and must be the sole
// source of topical signal.
//
// Used as an α/β multiplier in rankChainsBySubject to penalise
// chains that mention none of the question entities.
func chainTopicRelevance(chain string, _ chainAnchorInfo, topicTokens map[string]bool) float64 {
	if len(topicTokens) == 0 {
		return 0
	}
	hay := strings.ToLower(chain)
	hit := 0
	for tok := range topicTokens {
		if strings.Contains(hay, tok) {
			hit++
		}
	}
	return float64(hit) / float64(len(topicTokens))
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

// anchorFileLine binds a per-file line number so chainAnchorInfo can
// carry Files + FileLines in lock-step. Used by
// dedupeAnchorFilesWithLines to dedupe on path while preferring the
// first non-zero line a given file appears with.
type anchorFileLine struct {
	File string
	Line int
}

// computeIsDefFile returns a parallel-to-files slice marking whether
// each file contains the chain terminal symbol's DEFINITION (true) or
// is only a USAGE site (false). Used by applyChainPromotion's X2
// filter so a forced-read budget is not spent on a file that just
// references a symbol — the LLM should be reading the file that
// DEFINES the symbol when it needs ground truth.
//
// Language-agnostic: matches against fields the repomap extractor
// already populates per-language (Symbol.Name + optional
// Symbol.Receiver), accepting any of the common qualified-form
// surface syntaxes — bare name, "Receiver.Member" (Go / Java /
// Python / Kotlin / Swift / TS / Ruby), "Receiver::Member" (Rust /
// C++ / Cangjie), or just "Receiver" alone (when the terminal is
// the type itself, not a member). The filter does NOT parse the
// terminal token according to any one language's grammar; instead it
// constructs candidate qualified forms FROM the symbol record and
// equality-checks each against the terminal string.
//
// Fail-open semantics: when graph is nil, file is missing from
// FileIndex, or terminal is empty / a non-symbol-shaped string
// (literal like "true" / "{" / "false" / a numeric), every entry is
// true so promotion continues to fire just like before this audit.
// The filter only triggers when we have HIGH confidence the anchor
// file is USAGE-only.
func computeIsDefFile(graph *repomap.Graph, files []string, chainSummary string) []bool {
	out := make([]bool, len(files))
	for i := range out {
		out[i] = true // fail-open default
	}
	if graph == nil || len(files) == 0 || chainSummary == "" {
		return out
	}
	terminal := strings.TrimSpace(subject.ChainTerminalToken(chainSummary))
	if terminal == "" {
		// Extraction failed — no signal to base the def/usage split
		// on. Preserve historical promotion behaviour (fail-open).
		return out
	}
	if !looksLikeSymbolName(terminal) {
		// Terminal IS something but is a literal / punctuation
		// (`true` / `false` / `{` / numeric / etc). The chain has
		// no concrete subject identifier — no symbol to ground TO,
		// so a forced-read on its anchor file is grounding nothing.
		// Fail-closed: mark every file as USAGE-only so
		// applyChainPromotion's X2 filter skips the per-file
		// PendingRead. Pre-2026-05-06 this branch fell through to
		// fail-open, which let SubjectGeneric architecture
		// questions enqueue forced-reads for chains whose terminals
		// were `{` literals from struct returns.
		for i := range out {
			out[i] = false
		}
		return out
	}
	for i, f := range files {
		fi, ok := graph.FileIndex[f]
		if !ok || fi == nil {
			out[i] = true // unknown file → fail open
			continue
		}
		out[i] = false
		for _, sym := range fi.Symbols {
			if symbolMatchesTerminal(sym, terminal) {
				out[i] = true
				break
			}
		}
	}
	return out
}

// symbolMatchesTerminal checks whether a graph symbol could be the
// chain terminal. Constructs candidate keys from the symbol's own
// fields and compares each against terminal — language-agnostic
// because the symbol fields are populated by per-language extractors
// in repomap and already encode whatever qualifier conventions the
// source language uses.
func symbolMatchesTerminal(sym repotypes.Symbol, terminal string) bool {
	if terminal == "" || sym.Name == "" {
		return false
	}
	if sym.Name == terminal {
		return true
	}
	if sym.Receiver != "" {
		// Common qualified surface syntaxes used by codrax-supported
		// languages. The compare order doesn't matter (each is an
		// equality check); we list them explicitly so a future
		// language that introduces a new qualifier (e.g. "<<") needs
		// only one extra case, not a parser rewrite.
		if sym.Receiver+"."+sym.Name == terminal {
			return true
		}
		if sym.Receiver+"::"+sym.Name == terminal {
			return true
		}
		if sym.Receiver == terminal {
			return true
		}
	}
	return false
}

// looksLikeSymbolName is a language-agnostic guard for "terminal is
// shaped like an identifier we can usefully look up", screening out
// literal terminals (`true`, `{`, numeric, punctuation-only) that
// would never appear in graph.FileIndex.Symbols anyway. Requires at
// least one alphabetic character + no whitespace / brackets / common
// keyword-literals in any of the codrax-supported languages.
func looksLikeSymbolName(s string) bool {
	if s == "" {
		return false
	}
	switch s {
	case "true", "false", "True", "False", "TRUE", "FALSE",
		"nil", "null", "None", "NULL", "Nothing", "undefined":
		return false
	}
	hasAlpha := false
	for _, r := range s {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			return false
		case r == '{' || r == '}' || r == '(' || r == ')' ||
			r == '[' || r == ']' || r == ',' || r == ';' || r == '\'':
			return false
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasAlpha = true
		}
	}
	return hasAlpha
}

// dedupeAnchorFilesWithLines is the range-aware counterpart of
// dedupeAnchorFiles. It dedupes by path (same semantics as the
// original) and records a representative line per retained file.
// When the same file is contributed twice with different lines, the
// first non-zero line wins — a later zero-line entry cannot overwrite
// a real line, and a later non-zero entry cannot overwrite an earlier
// non-zero one (deterministic, avoids insertion-order flip-flop).
// Returns parallel slices aligned by index: files[i] ↔ lines[i].
func dedupeAnchorFilesWithLines(in ...anchorFileLine) (files []string, lines []int) {
	seenIdx := make(map[string]int, len(in))
	for _, af := range in {
		f := strings.TrimSpace(af.File)
		if f == "" {
			continue
		}
		if idx, ok := seenIdx[f]; ok {
			if lines[idx] == 0 && af.Line > 0 {
				lines[idx] = af.Line
			}
			continue
		}
		seenIdx[f] = len(files)
		files = append(files, f)
		lines = append(lines, af.Line)
	}
	return files, lines
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
// Phase 6 stage 18 (2026-05-03): block-terminator detection is
// driven by the typed `lineFeatures` map populated by repomap's
// AST extractors (LineFeatureReturnStmt / BreakStmt / RaiseStmt /
// ThrowStmt). The retired path used a hardcoded prefix-token
// table ("return ", "break", "raise ", "throw ") on the source
// line text — a red-line violation when read uniformly with the
// rest of Phase 6's intent-classification cleanup. When
// lineFeatures is nil/empty (Tier 3+ regex-only fallback or AST
// not extracted for this language), the function returns nil:
// no signal ⇒ no decision-block surfacing rather than guessing
// from byte tokens.
//
// Parameters:
//   - lines: the raw source lines of the file (0-indexed)
//   - funcStart, funcEnd: 1-based inclusive line range of the function body
//   - lineFeatures: the typed AST features map (FileInfo.LineFeatures),
//     keyed by 1-based line. nil/empty disables this function.
//
// Returns nil if fewer than 3 blocks are found (not worth surfacing).
func extractDecisionBlocks(lines []string, funcStart, funcEnd int, lineFeatures map[int][]repotypes.LineFeature) []decisionBlock {
	if len(lineFeatures) == 0 {
		// Phase 6 stage 18 contract: no AST features ⇒ no signal ⇒
		// skip rather than fall back to byte-token scanning.
		return nil
	}
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

	// Phase 6 stage 18 (2026-05-03) — typed AST replacement for
	// the retired hardcoded prefix-token table ("return ",
	// "return\t", "break", "raise ", "throw ", "RAISE ", "THROW ").
	// Reads the per-line LineFeature set populated by repomap's
	// extractor and consults LineFeature.IsBlockTerminator() —
	// closed enum that includes ReturnStmt / BreakStmt /
	// RaiseStmt / ThrowStmt without scanning byte tokens.
	//
	// The indent gate is preserved: the AST feature flags any
	// return/break/raise/throw, but only those at the function
	// body's top-level indent (or one deeper) are treated as
	// block terminators. Inner-loop break / nested-helper return
	// inside the same function don't terminate the outer block.
	isBlockTerminator := func(line string, lineNo int) bool {
		indent := lineIndentLen(line)
		if indent < baseIndentLen || indent > baseIndentLen+4 {
			return false
		}
		for _, f := range lineFeatures[lineNo] {
			if f.IsBlockTerminator() {
				return true
			}
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

		if current != nil && isBlockTerminator(lines[i], i+1) {
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
			if isBlockTerminator(lines[li], li+1) {
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

type concreteValueReceiverIndex struct {
	byReceiver         map[string][]concreteValue
	receiverOrder      []string
	receiverByLower    map[string][]string
	receiverByFolded   map[string][]string
	factoryReturnsByFn map[string]map[string][]string
	refCache           map[string][]string
	graph              *repomap.Graph
}

func newConcreteValueReceiverIndex(values []concreteValue, graph *repomap.Graph) *concreteValueReceiverIndex {
	idx := &concreteValueReceiverIndex{
		byReceiver:         make(map[string][]concreteValue),
		receiverByLower:    make(map[string][]string),
		receiverByFolded:   make(map[string][]string),
		factoryReturnsByFn: make(map[string]map[string][]string),
		refCache:           make(map[string][]string),
		graph:              graph,
	}
	files := make(map[string]bool)
	for _, v := range values {
		files[v.file] = true
		if v.receiver == "" {
			continue
		}
		if _, seen := idx.byReceiver[v.receiver]; !seen {
			idx.receiverByLower[strings.ToLower(v.receiver)] = append(idx.receiverByLower[strings.ToLower(v.receiver)], v.receiver)
			idx.receiverByFolded[concreteValueFoldName(v.receiver)] = append(idx.receiverByFolded[concreteValueFoldName(v.receiver)], v.receiver)
		}
		idx.byReceiver[v.receiver] = append(idx.byReceiver[v.receiver], v)
	}
	for receiver := range idx.byReceiver {
		idx.receiverOrder = append(idx.receiverOrder, receiver)
	}
	sort.Strings(idx.receiverOrder)
	if graph != nil {
		for file := range files {
			fi := graph.FileIndex[file]
			if fi == nil {
				continue
			}
			fnReturns := make(map[string][]string)
			for _, sym := range fi.Symbols {
				if sym.Name == "" || len(sym.ReturnTypeNames) == 0 {
					continue
				}
				for _, rt := range sym.ReturnTypeNames {
					if _, ok := idx.byReceiver[rt]; ok {
						fnReturns[sym.Name] = appendUniqueConcreteString(fnReturns[sym.Name], rt)
					}
				}
			}
			if len(fnReturns) > 0 {
				idx.factoryReturnsByFn[file] = fnReturns
			}
		}
	}
	return idx
}

func appendUniqueConcreteString(in []string, v string) []string {
	if v == "" {
		return in
	}
	for _, existing := range in {
		if existing == v {
			return in
		}
	}
	return append(in, v)
}

func concreteValueFoldName(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), "_", "")
}

func concreteValueKey(v concreteValue) string {
	return v.file + "\x00" + v.receiver + "\x00" + v.method + "\x00" + v.kind + "\x00" + v.value + "\x00" + strconv.Itoa(v.line)
}

func (idx *concreteValueReceiverIndex) addReceiver(out map[string]bool, receiver string) {
	if idx == nil || receiver == "" {
		return
	}
	if _, ok := idx.byReceiver[receiver]; ok {
		out[receiver] = true
	}
}

func (idx *concreteValueReceiverIndex) addReceiverAliases(out map[string]bool, surface string) {
	if idx == nil || surface == "" {
		return
	}
	idx.addReceiver(out, surface)
	for _, receiver := range idx.receiverByLower[strings.ToLower(surface)] {
		out[receiver] = true
	}
	for _, receiver := range idx.receiverByFolded[concreteValueFoldName(surface)] {
		out[receiver] = true
	}
}

func (idx *concreteValueReceiverIndex) referencedReceivers(v concreteValue) []string {
	if idx == nil || v.value == "" {
		return nil
	}
	key := concreteValueKey(v)
	if cached, ok := idx.refCache[key]; ok {
		return cached
	}
	seen := make(map[string]bool)
	for _, tok := range concreteValueIdentifierTokens(v.value) {
		idx.addReceiverAliases(seen, tok)
		if byFn := idx.factoryReturnsByFn[v.file]; len(byFn) > 0 {
			for _, receiver := range byFn[tok] {
				idx.addReceiver(seen, receiver)
			}
		}
	}
	if v.kind == concreteValueKindCalls {
		for _, receiver := range resolveCallTargetReceivers(v.value, idx.graph) {
			idx.addReceiver(seen, receiver)
		}
		if recvVar := callValueReceiverSurface(v.value); recvVar != "" {
			idx.addReceiverAliases(seen, recvVar)
		}
	}
	out := make([]string, 0, len(seen))
	for _, receiver := range idx.receiverOrder {
		if seen[receiver] {
			out = append(out, receiver)
		}
	}
	idx.refCache[key] = out
	return out
}

func (idx *concreteValueReceiverIndex) valuesForReceivers(receivers []string) []concreteValue {
	if idx == nil || len(receivers) == 0 {
		return nil
	}
	var out []concreteValue
	for _, receiver := range receivers {
		out = append(out, idx.byReceiver[receiver]...)
	}
	return out
}

func concreteValueIdentifierTokens(s string) []string {
	seen := make(map[string]bool)
	var out []string
	for i := 0; i < len(s); {
		if !isIdentChar(s[i]) {
			i++
			continue
		}
		start := i
		for i < len(s) && isIdentChar(s[i]) {
			i++
		}
		tok := s[start:i]
		if tok == "" || seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, tok)
	}
	return out
}

func callValueReceiverSurface(callValue string) string {
	dot := strings.LastIndex(callValue, ".")
	if dot <= 0 {
		return ""
	}
	varName := callValue[:dot]
	if ldot := strings.LastIndex(varName, "."); ldot >= 0 {
		varName = varName[ldot+1:]
	}
	return strings.TrimSpace(varName)
}

func concreteValuesLinked(v, rv concreteValue, graph *repomap.Graph) bool {
	linked := containsIdentifier(v.value, rv.receiver) ||
		containsFactoryReference(v.value, rv.receiver, graphFileInfo(graph, v.file))
	if !linked && v.kind == concreteValueKindCalls {
		linked = callValueMatchesReceiver(v.value, rv.receiver, graph)
	}
	if !linked && rv.kind == concreteValueKindCalls {
		linked = callValueMatchesReceiver(rv.value, v.receiver, graph)
	}
	if !linked && (v.kind == concreteValueKindEmbeds || v.kind == concreteValueKindImplements) {
		linked = containsIdentifier(v.value, rv.receiver) ||
			containsFactoryReference(v.value, rv.receiver, graphFileInfo(graph, v.file))
	}
	if !linked && (rv.kind == concreteValueKindEmbeds || rv.kind == concreteValueKindImplements) {
		linked = containsIdentifier(rv.value, v.receiver) ||
			containsFactoryReference(rv.value, v.receiver, graphFileInfo(graph, rv.file))
	}
	return linked
}

func graphFileInfo(graph *repomap.Graph, file string) *repotypes.FileInfo {
	if graph == nil {
		return nil
	}
	return graph.FileIndex[file]
}

type concreteValueEntry struct {
	kind       string // "returns", "binds ONLY", "binds"
	value      string // the concrete value
	lineOffset int    // zero-based line offset within the scanned snippet
}

func concreteValueAbsoluteLine(snippetStartLine, lineOffset int) int {
	if snippetStartLine <= 0 {
		return 0
	}
	if lineOffset < 0 {
		lineOffset = 0
	}
	return snippetStartLine + lineOffset
}

// Phase 6 stage 23 (2026-05-03) — typed family predicates that
// replace the `strings.Contains(kind, "binds")` substring checks
// scattered across the buildConcreteValuesSection scoring path.
// The concrete-values producer emits `kind` strings from a
// closed enum (the canonical list below); reading that enum
// structurally removes the substring family-membership pattern.
//
// The "binds" family is the only one that needs prefix-style
// membership: the producer emits both bare "binds" and
// "binds ONLY" / "binds default" / similar qualifiers in the
// same architectural role. The typed predicate captures the
// family without scanning arbitrary text.
const (
	concreteValueKindReturns    = "returns"
	concreteValueKindBinds      = "binds"
	concreteValueKindBindsOnly  = "binds ONLY"
	concreteValueKindMaps       = "maps"
	concreteValueKindConfig     = "config"
	concreteValueKindDecorates  = "decorates"
	concreteValueKindAssigns    = "assigns"
	concreteValueKindCalls      = "calls"
	concreteValueKindEmbeds     = "embeds"
	concreteValueKindImplements = "implements"
)

// isBindsKind reports whether `kind` is the bare "binds" or any
// "binds <qualifier>" variant. The producer emits qualifiers
// like "binds ONLY" / "binds default" that all share the same
// downstream semantic (factory-shape registration). Replaces
// the retired `strings.Contains(kind, "binds")` substring check
// with a structural prefix-equality test on the closed family.
func isBindsKind(kind string) bool {
	if kind == concreteValueKindBinds {
		return true
	}
	return strings.HasPrefix(kind, concreteValueKindBinds+" ")
}

// isBindingShapeKind reports whether `kind` belongs to the
// binding/registration family (binds variants + maps + config +
// decorates + assigns). All five share the architectural role
// of "this code line establishes a runtime binding from one
// identifier to another". Replaces the retired Contains-OR
// chain (`Contains(kind, "binds") || kind=="maps" || ...`).
func isBindingShapeKind(kind string) bool {
	if isBindsKind(kind) {
		return true
	}
	switch kind {
	case concreteValueKindMaps, concreteValueKindConfig,
		concreteValueKindDecorates, concreteValueKindAssigns:
		return true
	}
	return false
}

func isConcreteValueBodySymbolKind(kind string) bool {
	switch kind {
	case "function", "method", "ctor", "operator", "foreign-func", "builder",
		"styles", "ui-entry", "suspend-function", "extension-function", "extend":
		return true
	default:
		return false
	}
}

func isConcreteValueDeclarationSymbolKind(kind string) bool {
	switch kind {
	case "class", "module", "interface", "struct", "enum", "trait",
		"protocol", "object", "data-class", "sealed", "sealed-class",
		"type", "rpc", "service", "component", "actor",
		"companion-object", "annotation":
		return true
	default:
		return false
	}
}

func extractDeclarationConcreteValues(source, lang string) []concreteValueEntry {
	if lang == "" {
		return nil
	}
	lines := stripCommentLines(strings.Split(source, "\n"))
	if len(lines) == 0 {
		return nil
	}
	var out []concreteValueEntry
	out = append(out, scanContractPatterns(lines, lang)...)
	out = append(out, scanEmbedsPatterns(lines, lang)...)
	out = append(out, scanImplementsPatterns(lines, lang)...)
	return out
}

func declarationConcreteValueContext(sym repomap.Symbol) (receiver, method string) {
	if sym.Kind == "rpc" {
		receiver = sym.Parent
		if receiver == "" {
			receiver = sym.Name
		}
		method = sym.Name
		if receiver != "" && receiver != sym.Name {
			method = receiver + "." + sym.Name
		}
		return receiver, method
	}
	receiver = sym.Name
	method = sym.Name
	if sym.Parent != "" {
		method = sym.Parent + "." + sym.Name
	}
	if receiver == "" {
		receiver = method
	}
	if method == "" {
		method = receiver
	}
	return receiver, method
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
type bridgeLiteralEvidence struct {
	chains          []types.EvidenceItem
	terminalReturns []types.EvidenceItem
}

func extractBridgeLiteralChains(graph *repomap.Graph, repoRoot string, consumerValues []concreteValue) []types.EvidenceItem {
	return extractBridgeLiteralEvidence(graph, repoRoot, consumerValues).chains
}

func extractBridgeLiteralEvidence(graph *repomap.Graph, repoRoot string, consumerValues []concreteValue) bridgeLiteralEvidence {
	if graph == nil {
		return bridgeLiteralEvidence{}
	}
	// isRegName matches function names from the "registration family"
	// across the currently indexed imperative languages. It is a
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
		fnQual        string
		file          string
		line          int
		verb          string
		targetClass   string
		targetFactory string
	}
	type identity struct {
		class   string
		method  string
		literal string
		file    string
		line    int
	}
	type builderIdentity struct {
		factory string
		field   string
		literal string
		file    string
		line    int
	}
	var bindings []binding
	var identities []identity
	var builderIdentities []builderIdentity

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
					for _, cv := range extractConcreteValues(body, fi.Language) {
						if !isBindsKind(cv.kind) {
							continue
						}
						qual := sym.Name
						if sym.Receiver != "" {
							qual = sym.Receiver + "." + sym.Name
						}
						for _, part := range strings.Split(cv.value, ",") {
							part = strings.TrimSpace(part)
							tgt := parseTargetClassFromBinding(part)
							factory := parseBindingFactoryName(part)
							if tgt == "" && factory == "" {
								continue
							}
							bindings = append(bindings, binding{
								fnQual:        qual,
								file:          fi.RelPath,
								line:          concreteValueAbsoluteLine(sym.Line, cv.lineOffset),
								verb:          cv.kind,
								targetClass:   tgt,
								targetFactory: factory,
							})
						}
					}
				}
			}
		}
	}

	neededClasses := make(map[string]bool, len(bindings))
	neededFactories := make(map[string]bool, len(bindings))
	for _, b := range bindings {
		if b.targetClass != "" {
			neededClasses[b.targetClass] = true
		}
		if b.targetFactory != "" {
			neededFactories[b.targetFactory] = true
		}
	}
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
			owner := sym.Receiver
			if owner == "" {
				owner = sym.Parent
			}
			if sym.Kind == "method" && owner != "" &&
				neededClasses[owner] &&
				isIdentityMethod(sym.Name) && bodyLen <= 10 {
				body := loadBody(fi.RelPath, sym.Line, sym.EndLine)
				if body != "" {
					for _, cv := range extractConcreteValues(body, fi.Language) {
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
							file:    fi.RelPath,
							line:    concreteValueAbsoluteLine(sym.Line, cv.lineOffset),
						})
						break // first literal wins per method
					}
				}
			}
			if sym.Kind == "function" && neededFactories[sym.Name] && bodyLen <= 160 {
				body := loadBody(fi.RelPath, sym.Line, sym.EndLine)
				field, lit, line := extractStableBuilderIdentity(body, sym.Line)
				if field == "" || lit == "" {
					continue
				}
				builderIdentities = append(builderIdentities, builderIdentity{
					factory: sym.Name,
					field:   field,
					literal: lit,
					file:    fi.RelPath,
					line:    line,
				})
			}
		}
	}

	idByClass := make(map[string][]identity, len(identities))
	for _, id := range identities {
		idByClass[id.class] = append(idByClass[id.class], id)
	}
	builderByFactory := make(map[string][]builderIdentity, len(builderIdentities))
	for _, id := range builderIdentities {
		builderByFactory[id.factory] = append(builderByFactory[id.factory], id)
	}
	var items []types.EvidenceItem
	var terminalReturns []types.EvidenceItem
	seen := make(map[string]bool)
	seenTerminalReturns := make(map[string]bool)
	addTerminalReturn := func(id identity) {
		if id.class == "" || id.method == "" || id.literal == "" || id.file == "" || id.line <= 0 {
			return
		}
		subject := id.class + "." + id.method
		object := strconv.Quote(id.literal)
		key := strings.ToLower(id.file) + "\x00" + strconv.Itoa(id.line) + "\x00" +
			strings.ToLower(subject) + "\x00" + object
		if seenTerminalReturns[key] {
			return
		}
		seenTerminalReturns[key] = true
		item := types.EvidenceItem{
			Kind:         types.EvidenceConcrete,
			Subject:      subject,
			Predicate:    "returns",
			Object:       object,
			Summary:      fmt.Sprintf("`%s()` returns %s", subject, object),
			Source:       id.file,
			LineStart:    id.line,
			LineEnd:      id.line,
			Confidence:   0.95,
			Producer:     "bridge_literal_terminal",
			Scope:        types.ScopeLine,
			AnchorKind:   types.AnchorReturn,
			AnchorSymbol: id.method,
			OwnerSymbol:  id.class,
		}
		item.ID = types.StableEvidenceID(item)
		terminalReturns = append(terminalReturns, item)
	}

	// Pass C — Join on class identifier (existing bridge_literal chains).
	// Requires BOTH bindings AND identities; skips silently if either
	// is empty so Pass D can still fire on bindings alone.
	if len(bindings) > 0 {
		for _, b := range bindings {
			ids := idByClass[b.targetClass]
			if len(ids) > 0 {
				for _, id := range ids {
					addTerminalReturn(id)
					summary := fmt.Sprintf(
						"`%s()` %s New%s(...) → `%s.%s()` returns %q",
						b.fnQual, b.verb, b.targetClass, b.targetClass, id.method, id.literal)
					if seen[summary] {
						continue
					}
					seen[summary] = true
					bridgeItem := types.EvidenceItem{
						Kind:       types.EvidenceDataflowPath,
						Subject:    summary,
						Predicate:  "resolution_chain",
						Summary:    summary,
						Source:     b.file,
						LineStart:  b.line,
						LineEnd:    b.line,
						Confidence: 0.9,
						Producer:   "bridge_literal",
						Scope:      types.ScopeLine,
					}
					bridgeItem.ID = types.StableEvidenceID(bridgeItem)
					items = append(items, bridgeItem)
				}
				continue
			}
			for _, id := range builderByFactory[b.targetFactory] {
				summary := fmt.Sprintf(
					"`%s()` %s `%s()` → `%s()` returns %s=%q",
					b.fnQual, b.verb, b.targetFactory, b.targetFactory, id.field, id.literal)
				if seen[summary] {
					continue
				}
				seen[summary] = true
				bridgeFactoryItem := types.EvidenceItem{
					Kind:       types.EvidenceDataflowPath,
					Subject:    summary,
					Predicate:  "resolution_chain",
					Summary:    summary,
					Source:     b.file,
					LineStart:  b.line,
					LineEnd:    b.line,
					Confidence: 0.88,
					Producer:   "bridge_literal",
					Scope:      types.ScopeLine,
				}
				bridgeFactoryItem.ID = types.StableEvidenceID(bridgeFactoryItem)
				items = append(items, bridgeFactoryItem)
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
					addTerminalReturn(id)
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
				consumerItem := types.EvidenceItem{
					Kind:       types.EvidenceDataflowPath,
					Subject:    summary,
					Predicate:  "resolution_chain",
					Summary:    summary,
					Source:     cv.file,
					LineStart:  cv.line,
					LineEnd:    cv.line,
					Confidence: 0.85,
					Producer:   "consumer_gate",
					Scope:      types.ScopeLine,
				}
				consumerItem.ID = types.StableEvidenceID(consumerItem)
				items = append(items, consumerItem)
			}
		}
	}

	return bridgeLiteralEvidence{chains: items, terminalReturns: terminalReturns}
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

var stableIdentityFieldPriority = []string{
	"name", "id", "key", "type", "kind", "label", "slug", "path", "route",
}

var stableIdentityFieldRank = func() map[string]int {
	out := make(map[string]int, len(stableIdentityFieldPriority))
	for i, field := range stableIdentityFieldPriority {
		out[field] = i
	}
	return out
}()

func extractStableBuilderIdentity(body string, startLine int) (field, literal string, line int) {
	if body == "" {
		return "", "", 0
	}
	lines := stripCommentLines(strings.Split(body, "\n"))
	bestRank := len(stableIdentityFieldPriority) + 1
	bestLine := 0
	bestField := ""
	bestLiteral := ""
	for i, raw := range lines {
		f, lit, ok := parseStableIdentityLiteral(raw)
		if !ok {
			continue
		}
		rank, ok := stableIdentityFieldRank[f]
		if !ok || rank >= bestRank {
			continue
		}
		bestRank = rank
		bestField = f
		bestLiteral = lit
		bestLine = startLine + i
		if rank == 0 {
			break
		}
	}
	if bestField == "" || bestLiteral == "" {
		return "", "", 0
	}
	return bestField, bestLiteral, bestLine
}

func parseStableIdentityLiteral(line string) (field, literal string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", "", false
	}
	for _, sep := range []string{":", "="} {
		idx := strings.Index(trimmed, sep)
		if idx <= 0 {
			continue
		}
		lhs := strings.TrimSpace(trimmed[:idx])
		rhs := strings.TrimSpace(trimmed[idx+1:])
		if sep == ":" && strings.HasPrefix(strings.ToLower(lhs), "case ") {
			continue
		}
		field = stableIdentityFieldName(lhs)
		if field == "" {
			continue
		}
		lits := extractQuotedLiterals(rhs)
		if len(lits) == 0 || strings.TrimSpace(lits[0]) == "" {
			continue
		}
		return field, lits[0], true
	}
	return "", "", false
}

func stableIdentityFieldName(lhs string) string {
	lhs = strings.TrimSpace(lhs)
	if lhs == "" {
		return ""
	}
	if idx := strings.LastIndex(lhs, "."); idx >= 0 {
		lhs = lhs[idx+1:]
	}
	if strings.HasPrefix(lhs, "[") && strings.Contains(lhs, "]") {
		lhs = lhs[1:strings.Index(lhs, "]")]
	}
	lhs = strings.TrimSpace(strings.Trim(lhs, "`\"' "))
	lhs = strings.TrimLeft(lhs, "&*")
	lower := strings.ToLower(lhs)
	if _, ok := stableIdentityFieldRank[lower]; ok {
		return lower
	}
	return ""
}

func parseBindingFactoryName(token string) string {
	t := strings.TrimSpace(token)
	t = strings.TrimLeft(t, "&,()")
	t = strings.TrimSpace(t)
	if t == "" {
		return ""
	}
	if strings.HasPrefix(t, "new ") {
		t = strings.TrimSpace(t[4:])
	}
	if paren := strings.IndexByte(t, '('); paren >= 0 {
		t = t[:paren]
	}
	if brace := strings.IndexByte(t, '{'); brace >= 0 {
		t = t[:brace]
	}
	if strings.ContainsAny(t, ".:") {
		splits := strings.FieldsFunc(t, func(r rune) bool {
			return r == '.' || r == ':'
		})
		picked := ""
		for i := len(splits) - 1; i >= 0; i-- {
			part := strings.TrimSpace(splits[i])
			if len(part) > 0 && part[0] >= 'A' && part[0] <= 'Z' {
				picked = part
				break
			}
		}
		if picked == "" {
			for i := len(splits) - 1; i >= 0; i-- {
				part := strings.TrimSpace(splits[i])
				if len(part) > 0 {
					picked = part
					break
				}
			}
		}
		if picked != "" {
			t = picked
		}
	}
	end := 0
	for end < len(t) {
		c := t[end]
		if c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			end++
			continue
		}
		break
	}
	t = strings.TrimSpace(t[:end])
	if t == "" || t[0] < 'A' || t[0] > 'Z' {
		return ""
	}
	return t
}

// parseTargetClassFromBinding extracts the class identifier from a
// binding value token like "NewSubExplorer(deps)", "new Handler()",
// "UserHandler", or "&Config{}". Returns the class name with the
// "New"/"new " constructor prefixes stripped and parenthesized args
// removed. Empty string if the shape is not a constructor reference.
func parseTargetClassFromBinding(token string) string {
	t := parseBindingFactoryName(token)
	if t == "" {
		return ""
	}
	// Factory prefix: "NewXxx" → "Xxx" (common Go/C# idiom). Only
	// strips if the remaining name starts with an uppercase letter,
	// keeping words like "News" / "Newer" intact.
	if strings.HasPrefix(t, "New") && len(t) > 3 && t[3] >= 'A' && t[3] <= 'Z' {
		t = t[3:]
	}
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
//   - Python docstrings:      `"""` / `”'` multi-line string blocks
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
// that establish concrete values. The universal patterns are language-
// agnostic (return literals, constructor-passing calls, map entries,
// decorators, cross-component calls). Language-specific patterns layer
// on top via the `lang` parameter — pass `""` for language-agnostic
// mode (tests / fallback), or a concrete repomap language name for the
// full executable/declarative pattern set available in that language.
//
// Universal kinds emitted (any language):
//   - "returns"   — return literal / bool / nil / number
//   - "assigns"   — composite-literal variable assignment
//   - "binds"     — constructor-passing call argument
//   - "decorates" — @annotation paired with next decl
//   - "maps"      — key:value map/dict literal entries
//   - "calls"     — cross-component exported method call (T2a)
//
// Language-aware kinds emitted (when lang != ""):
//   - "conditional" — control-flow guards (if / switch / match / when / guard)
//   - "embeds"      — type embedding / inheritance / mixin ancestry
//   - "implements"  — interface / protocol / mixin conformance
//   - "errors"      — throw / raise / panic / fatal error construction
//   - proto contract rows reuse "maps" + "returns" so RPC request /
//     response types flow through the existing evidence pipeline.
func extractConcreteValues(source, lang string) []concreteValueEntry {
	var results []concreteValueEntry
	appendEntry := func(kind, value string, lineOffset int) {
		results = append(results, concreteValueEntry{
			kind:       kind,
			value:      value,
			lineOffset: lineOffset,
		})
	}
	// Pre-strip comment-only lines so none of the pattern scanners
	// below can parse comment text as code. The constructor-passing
	// call scanner in particular is vulnerable: a line like
	//   // (NewSubExplorer called once from RegisterDefaults). Each
	// would otherwise be emitted as a phantom `binds ONLY` entry,
	// which pollutes resolution-chain synthesis. See
	// memory/project_baseline_2026_04_13_post_phase4.md.
	lines := stripCommentLines(strings.Split(source, "\n"))

	// Count non-blank, non-brace-only lines to detect "single-statement" bodies.
	type registerCall struct {
		value      string
		lineOffset int
	}
	var registerCalls []registerCall

	for i, line := range lines {
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
				appendEntry("returns", rest, i)
				continue
			}
			// Boolean / nil / null / none
			lower := strings.ToLower(rest)
			if lower == "true" || lower == "false" || lower == "nil" ||
				lower == "null" || lower == "none" {
				appendEntry("returns", rest, i)
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
				appendEntry("returns", rest, i)
				continue
			}
			// Type literal: return Type{...} or return &Type{...}
			if strings.Contains(rest, "{") {
				appendEntry("returns", rest, i)
				continue
			}
			// Simple expression: return string(x), return x
			if !strings.Contains(rest, "\n") && len(rest) < 40 {
				appendEntry("returns", rest, i)
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
						appendEntry("assigns", varName+" := "+rhs, i)
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
						registerCalls = append(registerCalls, registerCall{value: inner, lineOffset: i})
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
			appendEntry("decorates", fmt.Sprintf("@%s(%s) → %s", decorator, decoratorArgs, target), i)
		}
	}

	// If there are constructor-passing calls, summarize them.
	if len(registerCalls) > 0 {
		qualifier := "binds ONLY"
		if len(registerCalls) > 1 {
			qualifier = "binds"
		}
		values := make([]string, 0, len(registerCalls))
		firstOffset := registerCalls[0].lineOffset
		for _, call := range registerCalls {
			values = append(values, call.value)
			if call.lineOffset < firstOffset {
				firstOffset = call.lineOffset
			}
		}
		appendEntry(qualifier, strings.Join(values, ", "), firstOffset)
	}

	// Language-aware structural facts: executable snippets can emit
	// conditionals / embeds / implements / errors, and declarative
	// snippets (today: proto RPC headers) can also project request /
	// response contracts via the existing maps / returns kinds.
	//
	// Ordering matters: emit language-specific kinds BEFORE the
	// universal "calls" detector so the final slice reads
	// naturally (declarations first, references last) when
	// rendered into the Concrete Values table.
	if lang != "" {
		results = append(results, scanContractPatterns(lines, lang)...)
		results = append(results, scanConditionalPatterns(lines, lang)...)
		results = append(results, scanEmbedsPatterns(lines, lang)...)
		results = append(results, scanImplementsPatterns(lines, lang)...)
		results = append(results, scanErrorsPatterns(lines, lang)...)
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
	if calls := extractCallTargetsWithLang(lines, lang, 6); len(calls) > 0 {
		for _, c := range calls {
			appendEntry("calls", c, 0)
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
	mapFirstOffset := -1
	for i, line := range lines {
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
			if mapFirstOffset < 0 || i < mapFirstOffset {
				mapFirstOffset = i
			}
		}
	}
	if len(mapEntries) > 0 {
		appendEntry("maps", strings.Join(mapEntries, "; "), mapFirstOffset)
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

// callTargetNoiseReceiversByLang keeps language-local library heads out
// of the cross-component call detector without globally blocking the
// same identifier in unrelated languages.
var callTargetNoiseReceiversByLang = map[string]map[string]bool{
	"lua": {
		"table": true, "string": true, "math": true, "coroutine": true,
		"utf8": true, "os": true, "io": true, "package": true, "debug": true,
	},
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

// extractCallTargets is the language-agnostic wrapper kept for tests
// and older call sites. Languages with extra member-call separators
// (currently Lua's `:`) should use extractCallTargetsWithLang so the
// detector can normalize them without broadening every language's
// false-positive surface.
func extractCallTargets(lines []string, maxCalls int) []string {
	return extractCallTargetsWithLang(lines, "", maxCalls)
}

// extractCallTargetsWithLang scans `lines` for method-call expressions
// that transfer control to an identifiable cross-component target, and
// returns a deduplicated list of targets in the form "Receiver.Method".
// The result is capped at `maxCalls` entries so long function bodies
// don't flood the concrete-values table.
//
// Recognition (conservative on purpose):
//   - Line contains `<receiver>.<Method>(` where Method starts with
//     an uppercase letter.
//   - Lua may also use `<receiver>:<method>(`; when lang=="lua" the
//     `:` form is normalized to the same target shape.
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
func extractCallTargetsWithLang(lines []string, lang string, maxCalls int) []string {
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
		for _, target := range scanCallTargetsInLineWithLang(trimmed, lang) {
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

func scanCallTargetsInLine(line string) []string {
	return scanCallTargetsInLineWithLang(line, "")
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
func scanCallTargetsInLineWithLang(line, lang string) []string {
	var targets []string
	for i := 0; i < len(line); i++ {
		if line[i] != '(' {
			continue
		}
		// Walk back over the identifier chain. Accepts three
		// separator styles for multi-language support:
		//   .   — Go / Java / Python / JS / TS
		//   ::  — Rust / C++ (path separator)
		//   ->  — C++ (pointer member access)
		end := i
		start := i
		for start > 0 {
			c := line[start-1]
			if isIdentChar(c) || c == '.' || c == ':' || c == '>' || c == '-' {
				start--
				continue
			}
			break
		}
		chain := line[start:end]
		if chain == "" {
			continue
		}
		if lang == "lua" {
			chain = strings.ReplaceAll(chain, ":", ".")
		}
		// Normalize separators: :: and -> become . so the rest of
		// the logic works uniformly.
		chain = strings.ReplaceAll(chain, "::", ".")
		chain = strings.ReplaceAll(chain, "->", ".")
		if !strings.Contains(chain, ".") {
			continue
		}
		// Split into receiver + method; method is the LAST segment.
		dotIdx := strings.LastIndex(chain, ".")
		receiver := chain[:dotIdx]
		method := chain[dotIdx+1:]
		// #4 multi-language: accept non-uppercase methods for
		// Python/Java/JS/TS/Rust where exported = no leading
		// underscore, not uppercase. Keep the uppercase filter as a
		// PREFERENCE: try uppercase first; if no methods pass,
		// accept lowercase methods ≥ 4 chars that aren't in the
		// noise list. This prevents flooding from short utility
		// methods (get, set, run) while accepting dispatch-relevant
		// methods like process, execute, dispatch.
		if method == "" {
			continue
		}
		if callTargetNoiseMethods[method] {
			continue
		}
		if !isExportedIdent(method) {
			// Lowercase fallback: accept long-enough non-noise methods
			if len(method) < 4 || strings.HasPrefix(method, "_") {
				continue
			}
		}
		// Head receiver identifier for stdlib exclusion.
		head := receiver
		if dot := strings.Index(head, "."); dot >= 0 {
			head = head[:dot]
		}
		if callTargetNoiseReceivers[head] {
			continue
		}
		if extra := callTargetNoiseReceiversByLang[lang]; extra != nil && extra[head] {
			continue
		}
		// Normalize receiver to the LAST identifier in the chain.
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

// resolveCallTargetReceiver resolves a "calls" kind value like
// "subRuntime.Run" to the actual TYPE name of the receiver by
// looking up the method in graph.SymbolDefs. Returns the resolved
// type name (e.g. "SubAgentRuntime") or "" if not resolvable.
//
// This bridges the fundamental gap in the chain linker: the "calls"
// detector emits VARIABLE names (last-segment normalisation), but
// the chain linker needs TYPE names for containsIdentifier matching.
// Without this resolver, `containsIdentifier("subRuntime.Run",
// "SubAgentRuntime")` returns false and the chain breaks.
//
// Strategy:
//  1. Extract the method name from the value (after the last dot).
//  2. Look up graph.SymbolDefs[method] — all definitions of that
//     method across the codebase.
//  3. For each definition that is a method (has a non-empty Receiver),
//     return the Receiver as a candidate type name.
//  4. Caller matches the returned type against rv.receiver.
//
// When the method name is common (e.g. "Run" defined on 5 types),
// returns ALL candidate receivers so the caller can match any of
// them. This is O(method-defs) per call but the SymbolDefs map is
// bounded by codebase size and the "calls" entries are capped at 6
// per snippet, so the total work is negligible.
func resolveCallTargetReceivers(callValue string, graph *repomap.Graph) []string {
	if graph == nil || callValue == "" {
		return nil
	}
	// Extract method name: "subRuntime.Run" → "Run"
	dot := strings.LastIndex(callValue, ".")
	if dot < 0 || dot >= len(callValue)-1 {
		return nil
	}
	method := callValue[dot+1:]
	if method == "" {
		return nil
	}
	defs, ok := graph.SymbolDefs[method]
	if !ok || len(defs) == 0 {
		return nil
	}
	var receivers []string
	for _, def := range defs {
		if def != nil && def.Receiver != "" {
			receivers = append(receivers, def.Receiver)
		}
	}
	return receivers
}

// callValueMatchesReceiver checks whether a "calls" kind value
// links to a target whose receiver is `targetReceiver`, using
// graph-assisted type resolution. Returns true when ANY of the
// resolved receiver types for the call target equals targetReceiver.
//
// Example: value="subRuntime.Run", targetReceiver="SubAgentRuntime"
//
//	→ resolves "Run" → SymbolDefs → [SubAgentRuntime, SomeOtherRunnable]
//	→ matches SubAgentRuntime → true
func callValueMatchesReceiver(callValue, targetReceiver string, graph *repomap.Graph) bool {
	// Path 1: graph-assisted exact type resolution (Go-optimal).
	for _, recv := range resolveCallTargetReceivers(callValue, graph) {
		if recv == targetReceiver {
			return true
		}
	}
	// Path 2 (#5): case-insensitive variable→type heuristic for
	// non-Go languages where variable naming conventions don't align
	// with type names (Java: subAgentRuntime → SubAgentRuntime,
	// Python: sub_agent_runtime → SubAgentRuntime). Extract the
	// receiver portion of the call value (before the last dot) and
	// check if it case-insensitively equals the targetReceiver.
	dot := strings.LastIndex(callValue, ".")
	if dot > 0 {
		varName := callValue[:dot]
		// Also handle chain receivers: take the last segment only
		if ldot := strings.LastIndex(varName, "."); ldot >= 0 {
			varName = varName[ldot+1:]
		}
		if strings.EqualFold(varName, targetReceiver) {
			return true
		}
		// Strip underscores for snake_case → CamelCase matching:
		// sub_agent_runtime → subagentruntime == SubAgentRuntime
		stripped := strings.ReplaceAll(strings.ToLower(varName), "_", "")
		if stripped == strings.ToLower(targetReceiver) {
			return true
		}
	}
	return false
}

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
	sort.Float64s(scores)
	return scores[(len(scores)-1)/2]
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

// parseReadFileBanner is the agent-package alias for the canonical
// parser in ground.ParseReadFileBanner. Lifted into the ground package
// so per-tool gates that fire mid-dispatch from internal/tool/ can
// share it without reaching into the agent layer; this thin wrapper
// stays so the existing in-agent callers (extractFileCoverage,
// detectTruncatedUngrepped, detectPartiallyReadSymbols, extractor) do
// not need to churn at every call site.
func parseReadFileBanner(summary string) (path string, rng types.LineRange, totalLines int, ok bool) {
	return ground.ParseReadFileBanner(summary)
}

// extractFileCoverage walks a tool-result history and extracts the
// discovered/read file sets the explorer uses for coverage gates.
//
// repoRoot upgrade (session 22): an LLM-supplied read_file path
// may be absolute (`/mnt/d/opt/codrax-main/internal/...`), a
// leading-./ relative form (`./internal/...`), or already
// repo-relative. Without a repo-root-aware canonicaliser these
// forms land on DIFFERENT map keys — readSet ends up with
// `/mnt/d/opt/codrax-main/internal/agent/agent.go` while the
// explorer's allScoredFiles / preScannedFiles / ERM closure all
// speak in `internal/agent/agent.go`. Every coverage check keyed
// off readSet then silently misses legitimate reads, and the
// explorer exits with demonstrably low coverage despite the LLM
// having actually opened the file.
//
// ground.CanonicalRepoRelative is the platform-aware canonicaliser
// (Windows volumes case-insensitive, POSIX slash-form preserved,
// strips an absolute-prefix that matches repoRoot). An empty
// repoRoot is a safe default — the function degrades to slash-form
// cleanup, matching the pre-session-22 behaviour, so unit tests
// that don't care about repo roots keep passing with
// repoRoot="".
func extractFileCoverage(history []types.ToolResult, repoRoot string) (
	discovered []string,
	readSet map[string]bool,
	readRanges map[string][]types.LineRange,
) {
	readSet, readRanges, _, discovered = extractFileCoverageWithTotals(history, repoRoot)
	return
}

// extractFileCoverageWithTotals is the canonical walker that ALSO
// returns per-file total line counts harvested from the read_file
// banner. The original extractFileCoverage shape is preserved for
// every caller that does not need totals; the multi-path symbol-
// anchored gate (applyMultiPathAnchorChecks → multipath.EvaluateAnchor)
// reads HasFullyRead and per-file totals via the closure that this
// walker populates, so the FullyRead bypass and the surgical
// MissingContextRegions calculation both have a real denominator.
func extractFileCoverageWithTotals(history []types.ToolResult, repoRoot string) (
	readSet map[string]bool,
	readRanges map[string][]types.LineRange,
	totals map[string]int,
	discovered []string,
) {
	readSet = make(map[string]bool)
	readRanges = make(map[string][]types.LineRange)
	totals = make(map[string]int)
	discoveredSet := make(map[string]bool)
	canon := func(p string) string {
		if p == "" {
			return ""
		}
		if repoRoot != "" {
			return ground.CanonicalRepoRelative(p, repoRoot)
		}
		return canonicalExplorerPath(p)
	}

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
				// Detect "path:linenum:content" (match line) or
				// "path-linenum-content" (context line). For both
				// separators we look for the first occurrence, verify
				// the next token is a run of digits (the lineno), and
				// slice off everything after that.
				if idx := firstSeparatorBeforeLineno(path); idx > 0 {
					path = path[:idx]
				}
				path = canon(path)
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
			// The banner carries both the path and the line range the
			// LLM actually saw. Record the path into readSet (file-
			// level, backward-compatible) AND the range into
			// readRanges so CGEC I1 chain promotion can distinguish a
			// partial paginated read from a full read. The total line
			// count goes into `totals` so per-file CoverageRatio /
			// HasFullyRead can compute against the real denominator
			// instead of comparing absolute lines to max-file lines.
			if path, rng, total, ok := parseReadFileBanner(r.Summary); ok {
				path = canon(path)
				if path != "" {
					readSet[path] = true
					readRanges[path] = append(readRanges[path], rng)
					if total > 0 && total > totals[path] {
						totals[path] = total
					}
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
				path = canon(path)
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
	return readSet, readRanges, totals, discovered
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
	// Shared directory filter: excludes structural noise at any depth,
	// but keeps root-only names (`memory`, `logs`, `eval`) legal when
	// they appear nested inside real project packages.
	if tool.IsExcludedRelativePath(path) {
		return true
	}
	// Test files (cross-language naming conventions). Keep this in sync with
	// the repomap-backed classifier instead of hand-maintaining a Go/JS-only
	// suffix list here; grep/repo_map discoveries should not create production
	// closure obligations from ArkTS, Cangjie, C/C++, Swift, Ruby, Lua, etc.
	base := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		base = path[idx+1:]
	}
	if types.LooksLikeTestFilePath(path) {
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
			// Use the shared canonical parser (which already strips
			// `[forced_read]` / `[forced_read surgical]` trace prefixes
			// before parsing the banner). Inline parsing here would
			// silently skip every forced-read result because the
			// trace prefix's own `[` collides with the banner's
			// `[path: ...]` shape — same pre-existing bug the
			// finalizer's grounder hit before commit 3238f9c.
			path, rng, total, ok := ground.ParseReadFileBanner(r.Summary)
			if !ok {
				continue
			}
			endLine := rng.End
			fr, fok := reads[path]
			if !fok {
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

// extractQuestionEntitiesFallback (C.1 audit followup 2026-05-02) is
// the fallback path for purely-CJK or all-lowercase-short-word
// questions where extractQuestionEntities returns empty (the strict
// CamelCase / dotted shape gates reject them). Without this
// fallback, buildAnalyzerRepoOverview (analyzer.go:316) degrades to
// the un-ranked general overview view, which means the analyzer LLM
// gets no question-relevant symbol candidates and is forced to emit
// concept-word entities ("explorer", "retry") that then trigger
// downstream R1.5 entity_unresolvable rejection.
//
// Strategy: tokenize the question (whitespace + CJK boundary
// aware), normalize each token via NormalizeCodeKey
// (case+underscore+hyphen insensitive), and look up against
// graph.SymbolDefs. Token-shape gating is REPLACED by
// repomap-existence-gating: any token that maps to a real Tier-1/2
// symbol is admitted; anything else is dropped. This is structural
// (no keyword tables, red line §5.2) — the signal source is the
// repo's own symbol surface as observed by tree-sitter / extractors.
//
// Token noise floor: NormalizeCodeKey result must be ≥3 runes to
// admit. This drops 1-2-character token noise ("a", "is", "如" etc).
//
// Returns nil when graph is nil OR no token resolves to a known
// symbol. Caller should treat nil as "no fallback signal" and
// degrade to general overview as before.
func extractQuestionEntitiesFallback(question string, graph *repomap.Graph) []string {
	if graph == nil || strings.TrimSpace(question) == "" {
		return nil
	}
	tokens := tokenizeQuestionCJKAware(question)
	seen := make(map[string]bool, len(tokens))
	var out []string
	for _, tok := range tokens {
		canon := normalizer.NormalizeCodeKey(tok)
		if len([]rune(canon)) < 3 {
			continue
		}
		// Direct hit on the repo symbol surface (case-sensitive).
		if defs, ok := graph.SymbolDefs[tok]; ok && len(defs) > 0 {
			if !seen[tok] {
				seen[tok] = true
				out = append(out, tok)
			}
			continue
		}
		// Case/underscore-insensitive secondary lookup. Walks
		// SymbolDefs once per miss; bounded by the dedup map so
		// the same fallback hit doesn't re-walk.
		for name, defs := range graph.SymbolDefs {
			if len(defs) == 0 {
				continue
			}
			if normalizer.NormalizeCodeKey(name) == canon {
				if !seen[name] {
					seen[name] = true
					out = append(out, name)
				}
				break
			}
		}
	}
	return out
}

// tokenizeQuestionCJKAware splits a question string on:
//   - ASCII whitespace
//   - ASCII punctuation such as dots, brackets, quotes, slash, and backslash
//   - CJK character boundaries (each CJK rune is its own token —
//     they are word boundaries in Chinese / Japanese / Korean since
//     those languages don't separate words by spaces)
//
// CJK runes are dropped from the output (they cannot match any
// repo symbol; they're effectively just word boundaries here).
// ASCII tokens of any length are emitted; the caller's NormalizeCodeKey
// length floor handles noise.
func tokenizeQuestionCJKAware(s string) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		case r == '.' || r == ',' || r == ';' || r == ':' || r == '!' || r == '?' ||
			r == '(' || r == ')' || r == '{' || r == '}' || r == '[' || r == ']' ||
			r == '<' || r == '>' || r == '"' || r == '\'' || r == '`' || r == '\\' ||
			r == '/':
			flush()
		case r >= 0x4E00 && r <= 0x9FFF, // CJK Unified
			r >= 0x3400 && r <= 0x4DBF, // CJK Extension A
			r >= 0x3000 && r <= 0x303F, // CJK Symbols
			r >= 0x3040 && r <= 0x309F, // Hiragana
			r >= 0x30A0 && r <= 0x30FF, // Katakana
			r >= 0xAC00 && r <= 0xD7AF, // Hangul
			r == '。' || r == '，' || r == '、' || r == '；' || r == '：' ||
				r == '！' || r == '？' ||
				r == '“' || r == '”' || // " "
				r == '‘' || r == '’': // ' '
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// detectDetailListingIntent was retired 2026-05-03 (Phase 6
// stage 17). The function classified the user's question via
// hardcoded EN+ZH keyword tables ("哪几种", "what strategies",
// "what kinds of", etc.) — the same red-line violation as the
// stage 16 retired path in explorer_erm.go. Intent classification
// is the analyzer's single responsibility; explore consumes the
// typed result via isEnumerationRequestModel and other typed
// accessors. Function had zero production callers (only the
// retired test reached it).

// enumerationIntentForContext (Phase 6 stage 17, 2026-05-03)
// returns the typed `isEnumerationRequestModel` verdict when a
// structured RequestModel is available; otherwise false. The
// retired path fell back to detectEnumerationIntent which
// tokenized the question's free-form text via hardcoded EN+ZH
// keyword tables ("所有", "list all", etc.) — that violated the
// "意图分类是分析器的工作" red line. When the analyzer hasn't
// run, ERM cannot know whether the question is enumeration; the
// safe default is false (the explorer's other coverage gates do
// not depend on this signal except as an enrichment).
func enumerationIntentForContext(ctx *types.AgentContext) bool {
	rm := requestModelFromContext(ctx)
	if rm != nil && hasStructuredRequestModel(ctx, rm) {
		return isEnumerationRequestModel(*rm)
	}
	return false
}

func requestModelFromContext(ctx *types.AgentContext) *types.RequestModel {
	if ctx == nil {
		return nil
	}
	if ctx.AnalysisIR != nil {
		return &ctx.AnalysisIR.RequestModel
	}
	if ctx.Mutable != nil {
		return ctx.Mutable.RequestModel()
	}
	return nil
}

func explorerHistoryPrefersVCSNarrativePrincipal(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	return types.HistoryLookupPrefersVCSNarrativePrincipal(
		ctx.AnalysisIR.RequestModel,
		&ctx.AnalysisIR.AnswerContract,
	)
}

func hasStructuredRequestModel(ctx *types.AgentContext, rm *types.RequestModel) bool {
	if ctx != nil && ctx.AnalysisIR != nil {
		return true
	}
	if rm == nil {
		return false
	}
	if rm.Intent != "" || rm.PredicateAxis != types.AxisUnknown || rm.AnswerSubject.Kind != "" {
		return true
	}
	if rm.Language != "" || rm.AnalyzerHints.Kind != "" {
		return true
	}
	if rm.Predicates.IsCategoryEnumeration || rm.Predicates.IsRelationalLookup || rm.Predicates.IsScalarAnswer {
		return true
	}
	if len(rm.SubTopics) > 0 ||
		len(rm.AnalyzerHints.Entities) > 0 ||
		len(rm.AnalyzerHints.PrimaryEntities) > 0 ||
		len(rm.AnalyzerHints.ExactTargets) > 0 {
		return true
	}
	return false
}

// detectEnumerationIntent was retired 2026-05-03 (Phase 6
// stage 17). The function tokenized the user's question via
// hardcoded EN+ZH keyword tables ("所有", "每个", "list all",
// "find all", "enumerate", "how many", "every", "each", "what
// are", "which") to fabricate an enumeration intent — a direct
// red-line violation per the stage 16/17 architectural directive
// "意图分类是分析器的工作 / 不允许出现这些关键字去更改意图分类的".
// The typed analyzer signal `isEnumerationRequestModel` (reads
// RequestModel.Predicates.IsCategoryEnumeration / Intent /
// PredicateAxis enum values) is the canonical replacement.

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

// preReadRequiredFiles reads the first `maxFiles` files (each capped
// at `maxLines` lines) and formats their content as a prompt section.
// Narrowly used by buildFocusedDepthStartInstruction: only fires when
// uniqueExactAnchorFile() returns a high-confidence anchor, so the
// "amplify wrong file" risk that prompted b9438d6's broader preRead
// removal is gated out upstream — the caller pre-proves the target
// file is the user's named entity before we inject any content.
//
// Returns an empty string when no files can be read (missing paths,
// empty repoRoot, etc.). Each file header shows the repo-relative
// path so the LLM can cite the right location without guessing.
//
// L2 (2026-05-10): the optional `excludeRead` set lets callers skip
// files the LLM has already read in a prior dispatch within the same
// Run. Re-injecting those files wastes prompt tokens and contradicts
// the LLM's own judgment when it had already chosen not to evidence
// them. Pass nil for legacy unconditional injection.
func preReadRequiredFiles(repoRoot string, files []string, maxFiles, maxLines int, excludeRead map[string]bool) string {
	return preReadRequiredFilesWithObserver(repoRoot, files, maxFiles, maxLines, excludeRead, nil)
}

func preReadRequiredFilesTracked(ctx *types.AgentContext, repoRoot string, files []string, maxFiles, maxLines int, excludeRead map[string]bool) string {
	var closure *types.EvidenceClosure
	if ctx != nil && ctx.Mutable != nil {
		closure = ctx.Mutable.EvidenceClosure()
	}
	return preReadRequiredFilesWithObserver(repoRoot, files, maxFiles, maxLines, excludeRead, func(file string, totalLines, readLines int) {
		if closure == nil || file == "" || totalLines <= 0 || readLines <= 0 {
			return
		}
		readSet := closure.ReadSet()
		if readSet == nil {
			readSet = make(map[string]bool, 1)
		}
		readSet[canonicalExplorerPath(file)] = true
		closure.SetReadSet(readSet)
		closure.AddReadRanges(map[string][]types.LineRange{
			file: {{Start: 1, End: readLines}},
		})
		closure.RecordFileTotalLines(file, totalLines)
	})
}

func preReadRequiredFilesWithObserver(repoRoot string, files []string, maxFiles, maxLines int, excludeRead map[string]bool, observe func(file string, totalLines, readLines int)) string {
	if repoRoot == "" || len(files) == 0 || maxFiles <= 0 {
		return ""
	}
	var b strings.Builder
	injected := 0
	for _, f := range files {
		if injected >= maxFiles {
			break
		}
		if excludeRead != nil {
			if excludeRead[canonicalExplorerPath(f)] {
				continue
			}
		}
		absPath := filepath.Join(repoRoot, f)
		content, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		lines := strings.Split(string(content), "\n")
		totalLines := len(lines)
		truncated := false
		if totalLines > maxLines {
			lines = lines[:maxLines]
			truncated = true
		}
		fmt.Fprintf(&b, "#### `%s`", f)
		if truncated {
			// Disclose the total line count so the model does not
			// mistake a head slice for the whole file and skip a
			// read_file with offset>0 that it genuinely needs.
			fmt.Fprintf(&b, " (first %d of %d total lines — issue read_file with offset to see the rest)", maxLines, totalLines)
		} else {
			fmt.Fprintf(&b, " (full file, %d lines)", totalLines)
		}
		b.WriteString("\n```\n")
		for i, line := range lines {
			fmt.Fprintf(&b, "%d\t%s\n", i+1, line)
		}
		b.WriteString("```\n\n")
		if observe != nil {
			observe(f, totalLines, len(lines))
		}
		injected++
	}
	return b.String()
}

// excludeReadFromCtx pulls the canonical-path set from
// EvidenceClosure.ReadSet() — files the LLM has already read in any
// prior dispatch within this Run. Returns nil when the closure is
// unavailable, signalling preReadRequiredFiles to skip the dedup
// (legacy unconditional injection).
//
// The closure's ReadSet IS already cumulative across dispatches (set
// from extractFileCoverage on each Observe), so no extra tracking
// state is needed on the evaluator.
//
// L4 (2026-05-10): when called via excludeReadAndIrrelevantFromCtx,
// the analyzer-declared irrelevant_files are also unioned into the
// exclusion set so pre-read injection respects the LLM's negative
// declaration.
func excludeReadFromCtx(ctx *types.AgentContext) map[string]bool {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	closure := ctx.Mutable.EvidenceClosure()
	if closure == nil {
		return nil
	}
	rs := closure.ReadSet()
	if len(rs) == 0 {
		return nil
	}
	out := make(map[string]bool, len(rs))
	for f := range rs {
		canon := canonicalExplorerPath(f)
		if canon == "" {
			continue
		}
		out[canon] = true
	}
	return out
}

// excludeReadAndIrrelevantFromCtx returns the union of
// EvidenceClosure.ReadSet() (L2 dedup) and the explorer's cached
// irrelevant_files set (L4 negative channel). Both are honored
// uniformly by preReadRequiredFiles — files the LLM read in a prior
// dispatch AND files it explicitly declared off-topic are skipped.
//
// L4 (2026-05-10).
func (e *explorerEvaluator) excludeReadAndIrrelevantFromCtx(ctx *types.AgentContext) map[string]bool {
	rs := excludeReadFromCtx(ctx)
	if len(e.irrelevantFilesSet) == 0 {
		return rs
	}
	if rs == nil {
		out := make(map[string]bool, len(e.irrelevantFilesSet))
		for k, v := range e.irrelevantFilesSet {
			out[k] = v
		}
		return out
	}
	for k := range e.irrelevantFilesSet {
		rs[k] = true
	}
	return rs
}

// crossFileSymbolGap is one hit returned by detectCrossFileSymbolGaps:
// an exported symbol `Symbol` referenced in the LLM's investigation
// notes, defined in `File`, which the LLM has not yet read.
type crossFileSymbolGap struct {
	File   string
	Symbol string
}

// detectCrossFileSymbolGaps scans investigation notes for exported
// symbol references and returns up to `max` files that define
// referenced symbols but haven't been read yet (T3b). The explorer's
// mid-loop call uses detectCrossFileSymbolGapsWithFileFilter so this
// signal cannot widen beyond the active primary-entity frontier.
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
	return detectCrossFileSymbolGapsWithFileFilter(notes, graph, readSet, max, nil)
}

func detectCrossFileSymbolGapsWithFileFilter(notes []string, graph *repomap.Graph, readSet map[string]bool, max int, allowFile func(string) bool) []crossFileSymbolGap {
	if graph == nil || len(notes) == 0 || max <= 0 {
		return nil
	}
	normalized := make([]string, 0, len(notes))
	for _, note := range notes {
		if cleaned := normalizeExplorationNote(note); cleaned != "" {
			normalized = append(normalized, cleaned)
		}
	}
	joined := strings.Join(normalized, "\n")
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
			file := canonicalExplorerPath(def.File)
			if file == "" || isNoisePath(file) {
				continue
			}
			if readSetContains(readSet, file) {
				continue
			}
			if allowFile != nil && !allowFile(file) {
				continue
			}
			key := file + "\x00" + symName
			if seen[key] {
				continue
			}
			seen[key] = true
			gaps = append(gaps, crossFileSymbolGap{File: file, Symbol: symName})
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
	fileReads := readFileIntervalsFromHistory(history)
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
			startedFromTop, maxEnd := symbolReadProgress(sym, intervals)
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

func readFileIntervalsFromHistory(history []types.ToolResult) map[string][]readInterval {
	fileReads := make(map[string][]readInterval)
	for _, r := range history {
		if !r.Success || r.ToolName != "read_file" {
			continue
		}
		// Use the shared canonical parser so this inline reader
		// participates in the same forced-read-prefix-strip the
		// finalizer's grounder + closure coverage walker already
		// share. Without delegation, every `[forced_read surgical] `
		// or `[forced_read] ` prefixed read would be silently skipped
		// here — partial-read mid-loop hints would then under-count
		// what the LLM has been shown via Lazy Auto-Read recovery
		// and emit redundant "you read up to line N" hints for
		// ranges already covered.
		path, rng, _, ok := ground.ParseReadFileBanner(r.Summary)
		if !ok {
			continue
		}
		fileReads[path] = append(fileReads[path], readInterval{start: rng.Start, end: rng.End})
	}
	return fileReads
}

// structuralIntentTokens are the surface forms the LLM uses when it
// declares a "give me the big picture / overall structure" intent —
// i.e. a goal that a 60-line targeted window over a 1500-line file
// will answer poorly. zh-first because customer reports were
// zh-dominant; the en forms cover parallel English phrasings.
var structuralIntentTokens = []string{
	// zh
	"整体结构", "大致结构", "整个结构", "整体流程",
	"整个流程", "大致流程", "整体架构", "大致架构",
	"整体布局", "整体梳理", "全貌", "概览",
	// en
	"overall structure", "overall flow", "overall layout",
	"overall architecture", "big picture", "high level",
	"high-level overview", "full structure", "whole structure",
	"whole flow", "whole file",
}

// isStructuralIntent reports whether an assistant message content
// expresses a structural / overview intent — a goal that requires
// broad file coverage rather than a narrow targeted read. Returns
// false on empty content. Case-insensitive on English tokens; zh
// tokens are matched verbatim (case does not apply to CJK).
func isStructuralIntent(content string) bool {
	if content == "" {
		return false
	}
	lower := strings.ToLower(content)
	for _, tok := range structuralIntentTokens {
		if strings.Contains(lower, strings.ToLower(tok)) {
			return true
		}
	}
	return false
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

// isEvidenceLineByFeatures (Phase 6 stage 18, 2026-05-03) is the
// new-world replacement for the retired isEvidenceLine token-
// table scanner. It reads the per-line typed AST features
// populated by repomap's tree-sitter extractors
// (FileInfo.LineFeatures) and consults the typed
// LineFeature.IsEvidenceShape() predicate — closed enum that
// includes ReturnStmt / CallExpression / NewExpression /
// CompositeLiteral / ArrowFunction.
//
// The retired isEvidenceLine had EIGHT independent byte-level
// detectors (return prefix, =>, key:value, registration verb
// table, `new ` keyword, &UpperCase, factory-prefix table,
// composite literal RHS) — a direct red-line violation under
// stage 17/18's no-keyword-tables doctrine.
//
// Empty lineFeatures slice ⇒ no signal ⇒ returns false. Tier 3+
// regex-only fallback files / languages without AST extraction
// silently skip the deep concrete-values scan rather than
// guessing via byte tokens. This is the typed-only contract.
func isEvidenceLineByFeatures(lineFeatures []repotypes.LineFeature) bool {
	for _, f := range lineFeatures {
		if f.IsEvidenceShape() {
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
// IdentifierFactoryPrefixes (the yaml field + setter +
// package-global cache) was retired 2026-05-03 (Phase 6 stage 20).
// The naming-convention heuristic (`NewFoo` matches a search for
// `Foo` because `New` is a known factory verb) is replaced by a
// structurally correct typed signal: every function/method's
// `Symbol.ReturnTypeNames` is populated by the AST extractor;
// `containsFactoryReference` walks the file's symbols and matches
// any symbol whose return type includes the target. No naming
// table — return type is the source of truth.

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
		// Phase 6 stage 20 (2026-05-03) — the prior factory-prefix
		// generosity branch was retired here. Factory-call evidence
		// (`NewFoo` referencing `Foo`) is now derived structurally
		// via containsFactoryReference, which reads the file's
		// Symbol.ReturnTypeNames typed slot populated by the AST
		// extractor. Word-boundary matching survives unchanged —
		// it is itself a typed signal (left/right neighbour byte
		// is not in [A-Za-z0-9_]).
		start = pos + 1
	}
}

// containsFactoryReference reports whether `text` references the
// target type `name` via a factory call structurally captured in
// `fi.Symbols`. Phase 6 stage 20 (2026-05-03) typed replacement
// for the retired IdentifierFactoryPrefixes naming-convention
// heuristic.
//
// For each Symbol in `fi` whose typed ReturnTypeNames includes
// `name`, check whether `text` contains the symbol's Name as a
// word-boundary match (reuses containsIdentifier's existing
// boundary scanner — no new walker). A match means the text
// actually invokes a function whose declared return type is the
// target — strictly correct, naming-style-agnostic.
//
// Empty `fi` / no matching ReturnTypeNames ⇒ returns false. Tier 3+
// regex-only files have empty ReturnTypeNames so this signal
// degrades to "no factory match" rather than guessing via
// prefix tables — same typed-only contract as stage 18.
func containsFactoryReference(text, name string, fi *repotypes.FileInfo) bool {
	if text == "" || name == "" || fi == nil {
		return false
	}
	for _, sym := range fi.Symbols {
		if sym.Name == "" || len(sym.ReturnTypeNames) == 0 {
			continue
		}
		matchesTarget := false
		for _, rt := range sym.ReturnTypeNames {
			if rt == name {
				matchesTarget = true
				break
			}
		}
		if !matchesTarget {
			continue
		}
		if containsIdentifier(text, sym.Name) {
			return true
		}
	}
	return false
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
//
// Phase 6 stage 29 (2026-05-03) — the prefix list now derives
// from the typed concreteValueKind enum constants (stage 23)
// rather than a parallel inline literal table. The producer
// emits facts in the form "<kind> <value>" where <kind> is one
// of concreteValueKindReturns / concreteValueKindBinds (and
// "binds <qualifier>" variants) / concreteValueKindMaps /
// concreteValueKindDecorates / concreteValueKindConfig (and
// "registers" / "registers only" historical variants for
// backward compat). Reading from the typed enum keeps this
// helper in lockstep with the producer's vocabulary — adding a
// new concreteValueKindFoo constant automatically extends the
// strip set here.
func normalizeValueAssertion(fact string) string {
	fact = strings.TrimSpace(fact)
	lower := strings.ToLower(fact)
	for _, prefix := range valueAssertionPrefixes() {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(fact[len(prefix):])
		}
	}
	return fact
}

// valueAssertionPrefixes returns the lowercase, space-suffixed
// prefix forms derived from the typed concreteValueKind enum
// (stage 23). Order matters: longer prefixes ("binds only ",
// "registers only ") must come before their shorter siblings so
// the HasPrefix loop doesn't strip "binds " when the actual
// prefix is "binds only ".
//
// All values are lowercased to match the normalizeValueAssertion
// `lower` comparison; the producer's `concreteValueKindBindsOnly`
// constant is "binds ONLY" (mixed case) so we transform it.
//
// "registers" / "registers only" are kept as backward-compat
// historical synonyms of binds-shape registration that some
// older fixtures still emit. They have no concreteValueKind
// constant of their own (the producer canonicalised on "binds"
// long ago), so they live here as the only inline literals.
func valueAssertionPrefixes() []string {
	return []string{
		strings.ToLower(concreteValueKindBindsOnly) + " ",
		concreteValueKindBinds + " ",
		"registers only ", "registers ",
		concreteValueKindReturns + " ", "return ",
		concreteValueKindMaps + " ",
		concreteValueKindDecorates + " ",
		concreteValueKindConfig + " ",
	}
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

type explorerEvalSlot struct {
	mu   sync.Mutex
	eval *explorerEvaluator
}

// ExplorerAgent owns keyed explorer evaluator instances. A top-level
// explorer dispatch needs evaluator-local ReAct state, but a re-dispatch of
// the same DAG node must still see its previous notes and search state. The
// key is supplied by the scheduler through AgentContext.ExploreDispatchKey.
type ExplorerAgent struct {
	deps        *Dependencies
	searchCache *explorerSearchCache
	mu          sync.Mutex
	slots       map[string]*explorerEvalSlot
}

// NewExplorerAgent creates the explorer agent (used in explore stage).
func NewExplorerAgent(deps *Dependencies) Agent {
	if deps == nil {
		deps = &Dependencies{}
	}
	return &ExplorerAgent{
		deps:        deps,
		searchCache: &explorerSearchCache{},
		slots:       make(map[string]*explorerEvalSlot),
	}
}

func (a *ExplorerAgent) Name() types.AgentName {
	return types.AgentExplorer
}

func (a *ExplorerAgent) Execute(ctx *types.AgentContext, sk *skill.Config) (*StageOutput, error) {
	slot := a.slotFor(ctx)
	slot.mu.Lock()
	defer slot.mu.Unlock()
	base := NewBaseAgent(types.AgentExplorer, a.deps, slot.eval)
	return base.Execute(ctx, sk)
}

func (a *ExplorerAgent) slotFor(ctx *types.AgentContext) *explorerEvalSlot {
	key := explorerDispatchEvaluatorKey(ctx)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.slots == nil {
		a.slots = make(map[string]*explorerEvalSlot)
	}
	if slot := a.slots[key]; slot != nil {
		return slot
	}
	if len(a.slots) > 64 {
		prefix := explorerDispatchTracePrefix(ctx)
		for k := range a.slots {
			if prefix == "" || !strings.HasPrefix(k, prefix) {
				delete(a.slots, k)
			}
		}
	}
	slot := &explorerEvalSlot{eval: &explorerEvaluator{
		heuristics:        a.deps.ExploreHeuristics,
		tools:             a.deps.Tools,
		sharedSearchCache: a.searchCache,
	}}
	a.slots[key] = slot
	return slot
}

func explorerDispatchEvaluatorKey(ctx *types.AgentContext) string {
	prefix := explorerDispatchTracePrefix(ctx)
	focus := ""
	if ctx != nil {
		focus = strings.TrimSpace(ctx.ExploreDispatchKey)
	}
	if focus == "" {
		focus = "default"
	}
	if prefix == "" {
		return focus
	}
	return prefix + focus
}

func explorerDispatchTracePrefix(ctx *types.AgentContext) string {
	if ctx == nil {
		return ""
	}
	trace := strings.TrimSpace(ctx.TraceID)
	if trace == "" {
		trace = strings.TrimSpace(ctx.Objective)
	}
	if trace == "" {
		return ""
	}
	return trace + "|"
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
