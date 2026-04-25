package types

import (
	"strings"
	"sync"
	"time"
)

// TaskState captures the current pipeline execution state.
type TaskState struct {
	Stage        PipelineStage `json:"stage"`
	Missing      MissingPiece  `json:"missing"`
	Completed    []string      `json:"completed"`
	Remaining    []string      `json:"remaining"`
	LastDecision string        `json:"last_decision"`
	LastError    string        `json:"last_error,omitempty"`
	IsTerminal   bool          `json:"is_terminal"`

	// RetryHint is set when a stage self-loops (orchestrator picks
	// the same stage as next). It carries the previous dispatch's own
	// diagnosis of why it could not progress, so the next dispatch
	// sees concrete "do this differently" guidance instead of being
	// re-run with an unchanged prompt. The orchestrator clears it on
	// any forward transition. The prompt builder renders it as the
	// most prominent user section.
	RetryHint string `json:"retry_hint,omitempty"`
}

// MutableState is the tool-mutable region of pipeline state. Tools
// invoked during the ReAct loop receive a *BusContext whose Mutable
// pointer aliases the orchestrator's, so updates to the contained
// state are visible to subsequent tool calls and to the next
// stage's prompt rebuild without going through applyStageOutput.
//
// Everything outside MutableState in BusContext remains agent-output
// only — mutations are funneled through StageOutput → applyStageOutput
// as before. The internal RWMutex protects against data races for
// top-level agents that may run concurrent tool dispatches in
// future refactors; today's single-agent loop does not exercise it.
//
// SubAgents do NOT share this region. SubAgentRuntime spawns
// isolated workers whose AgentContext is built by
// BuildSubAgentContext, which deliberately leaves Mutable nil. Any
// tool that requires Mutable (the emit_* channels) will reject
// calls from a sub-agent with a clear error. Sub-agents return
// their findings via SubAgentResult and the reducer merges them
// back at the orchestrator boundary.
//
// Callers go through Objective() / SetObjective() / Result() /
// SetResult() instead of touching fields directly, so locking
// stays correct.
type MutableState struct {
	mu sync.RWMutex
	// objective is the raw user question / task description seeded
	// at orchestrator Run time. Replaces the old one-task TaskList
	// wrapper — every stage reads this field as the load-bearing
	// "what is the user asking" surface.
	objective string
	// repoRoot is the orchestrator's -repo flag value. Stored on
	// MutableState so lazy-init of evidenceClosure can thread the
	// same repo root through to the closure's path canonicaliser
	// (session 22). SetRepoRoot propagates a later root to an
	// already-existing closure if one was created before the
	// orchestrator plumbed the root through.
	repoRoot string
	// result is the finalizer's final answer for the run, written
	// by recordTaskFinalize after the per-task pipeline completes.
	// Replaces the old TaskItem.Result field.
	result          string
	requestModel    *RequestModel
	emittedEvidence []EvidenceItem
	// emittedAnswerSymbols + emittedAnswerSymbolCompleteness are
	// written as a set via SetEmittedAnswerSymbols and read via
	// EmittedAnswerSymbolSet (P2.1 Phase 9). The two fields are always
	// written together because the completeness claim is a set-level
	// property of the slate — an append-then-tag API would allow a
	// retry call to partially overwrite items without also overwriting
	// the claim, which is the exact foot-gun we want to rule out.
	// Retry semantics: a subsequent Set call REPLACES both fields
	// atomically under the write lock.
	emittedAnswerSymbols            []AnswerSymbol
	emittedAnswerSymbolCompleteness CompletenessClaim
	emittedHypothesisVerdicts       []HypothesisVerdict
	turnAArtifacts                  *TurnAArtifacts
	// searchGraph is an opaque handle to the repomap.Graph produced by
	// explorer.keywordSearch. Carried as `any` so internal/types stays
	// decoupled from internal/tool/repomap — consumers (emit_evidence,
	// grounding) type-assert on their own side. Set once per Run by
	// the explorer's BuildInitialInstruction so downstream tools can
	// share the same graph instance with zero I/O.
	searchGraph any

	// phase1Ranking is the explorer's keyword-search file ranking
	// (top-scored files by the pre-scan), captured once at dispatch
	// start so downstream tools can cross-reference against ReadSet
	// without re-running keyword_search. Used by the CGEC phase1-unread
	// pre-complete gate: when the explorer marks investigation done
	// while high-ranked files are still unread, the gate raises a
	// RepairExpandSearch directive pointing at those files.
	phase1Ranking []Phase1RankedFile

	// dispatchToolResults is the per-dispatch running buffer of tool
	// results seen so far in the current BaseAgent.Execute loop.
	// BusContext.ToolResults is only populated after ParseOutput via
	// StageOutput/applyStageOutput, which is too late for tools that
	// fire from inside the ReAct loop itself (notably emit_evidence's
	// synchronous grounder needs the read_file gutter reconstructed
	// from the same dispatch's earlier calls). BaseAgent.executeTool
	// appends to this buffer on every successful result, and
	// ResetDispatchToolResults clears it at loop entry so cross-
	// dispatch leakage is impossible.
	dispatchToolResults []ToolResult
	// answerDocument is the structured final-answer payload. It is
	// written by the emit_answer_document tool (one atomic set per
	// dispatch) and read by the finalizer's ParseOutput to render the
	// user-visible prose. Set semantics mirror SetEmittedAnswerSymbols:
	// a later call REPLACES any previous document, so a correction
	// retry from the ReAct loop cleanly wins over the prior attempt.
	// Cross-task reset (runTaskGraph) calls ResetAnswerDocument at
	// per-task entry so stale state cannot leak between tasks in a
	// multi-task run.
	answerDocument *AnswerDocument

	// changePlan is the B0 write-mode structured artifact produced
	// by the planner agent (emit_change_plan tool writes it). Read
	// by the plan stage hook after dispatchStage returns to render a
	// human-visible summary into Result and by cmd/root.go to
	// serialize the plan JSON to disk. Nil in read-mode and
	// apply-mode (apply consumes a plan file, not this field).
	// Write-once-read-many: emit_change_plan sets it; no later
	// stage mutates.
	changePlan *ChangePlan

	// partialChangePlan is the in-progress write-mode plan being
	// assembled across multiple LLM rounds via the structural
	// emit_plan_skeleton + emit_plan_change pattern. The skeleton
	// tool installs it with placeholder NewContent/Patch; each
	// emit_plan_change fills one path. When the last placeholder
	// is filled, emit_plan_change runs the full validator pipeline
	// and (on success) promotes Partial → ChangePlan, clearing
	// Partial. This is the structural safety net for plans where a
	// single-shot emit_change_plan would exceed the model's output
	// ceiling — see CLAUDE.md for the design.
	//
	// Distinct from ChangePlan: a non-nil PartialChangePlan means
	// "skeleton accepted, awaiting per-file content", while a
	// non-nil ChangePlan means "complete plan, all validators
	// passed." The two are never both non-nil simultaneously.
	partialChangePlan *ChangePlan

	// changeReport is the B1.3 verify-stage structured artifact.
	// run_tests populates it directly (tool-level deterministic
	// parse of the language-specific test runner output);
	// emit_test_results optionally decorates it with an LLM-
	// authored FailureSummary narrative. Consumed by
	// the verify stage hook (writes to Mutable.Result + disk-side JSON
	// under .codrax/plans/<plan-id>.report.json).
	//
	// Lifecycle mirrors changePlan: write-once-read-many, reset
	// at per-task entry.
	changeReport *ChangeReport

	// baselineReport is the B2 pre-apply test snapshot captured by
	// the apply stage hook before dispatching the coder agent. Consumed by
	// CritNoRegression to detect tests that passed before apply but
	// fail after. Separate from changeReport (post-apply) so the
	// diff is explicit and never confused. Nil when baseline
	// capture is disabled via codrax.yaml or when the apply stage hook
	// skipped it (e.g. no test runner detected).
	baselineReport *ChangeReport

	// planningHint carries structured feedback from a failed verify
	// run back to the planner on B2.3 retry dispatches. Non-empty
	// only during retry iterations; cleared on first retry entry.
	// The planner's BuildInitialInstruction checks this slot and
	// prepends a "Previous attempt failed — revise" section.
	planningHint string

	// investigationComplete is set by the emit_investigation_complete
	// tool when the LLM explicitly declares that it has collected
	// enough evidence to answer the user's question. The explorer's
	// ShouldStop reads this flag to terminate the ReAct loop, and
	// ParseOutput reads it to set HasEnoughFacts. Reset at the start
	// of each explore window by ResetInvestigationComplete.
	investigationComplete       bool
	investigationCompleteReason string

	// absenceJustification is the LLM's declarative claim that the
	// answer is an honest "zero" / "no X" and therefore has no
	// file:line to cite. Set via emit_investigation_complete's
	// optional absence_justification parameter. Stored on Mutable so
	// the orchestrator's isJustifiedAbsenceAnswer can treat the
	// answer as absence for citation-waiver purposes even when the
	// finalizer chose an explanation shape instead of a literal 0 /
	// false / [] shape. Declarative, not command: the audit
	// (hasInvestigationEvidence ≥1 investigation tool) still runs.
	absenceJustification string

	// exploreBudget is the ExploreBudget the orchestrator installs
	// at the top of runTaskGraph. The explorer's ReAct loop reads
	// it before every tool dispatch (BudgetRemaining / RecordToolCall)
	// so NodeBudgetHints becomes a real runtime throttle rather
	// than a log-only metric. Zero-value-safe: a nil budget means
	// "no caps installed" and BudgetRemaining returns a very large
	// number so non-DAG call paths (tests, one-shot dispatches)
	// stay unaffected.
	exploreBudget *ExploreBudget

	// prescanSummaryBlob is the lowercased concatenation of every
	// successful pre-scan ToolResult.Summary the analyzer observed
	// during the current analyze dispatch (repo_map / grep
	// files_only=true / list_files). Populated by
	// `analyzerEvaluator.Observe` via AppendPrescanSummary, read by
	// `emit_analysis.Execute` to feed the runtime quality probe:
	//   - As the verified-entity whitelist for
	//     validateAnalysisInput (entities that match the generic
	//     blocklist but appear in the blob are kept instead of
	//     dropped — see filterGenericEntitiesWithWhitelist).
	//   - As the substring corpus for the keyword/entity hit-ratio
	//     probe (ComputeAnalysisQualityProbe).
	// This is analyzer-specific state that lives on MutableState
	// only because the emit_analysis tool needs a path to read it;
	// other agents never touch it, and ResetPrescanSummary is
	// called at the start of each analyze dispatch by
	// analyzerEvaluator.BuildInitialInstruction so cross-dispatch
	// state never leaks.
	prescanSummaryBlob strings.Builder

	// evidenceClosure is the cross-stage CGEC tracker (Citation-
	// Grounded Evidence Closure). Records ReadSet, PendingReads,
	// UnverifiedFinds, RepairDirectives, and per-round Fingerprints
	// so the four CGEC enforcers (chain promotion, grounder, pre-
	// complete check, convergence detector) share a single source of
	// truth. Lazily initialized by EvidenceClosure(); reset at task
	// entry by ResetEvidenceClosure (mirror of ResetTurnAArtifacts).
	// All accessor methods live in evidence_closure.go.
	evidenceClosure *EvidenceClosure

	// writeClosure is the write-phase mirror of evidenceClosure — the
	// per-Run CGEC-W tracker for the four write-mode invariants W1-W4.
	// Populated by the apply (W1, W3), verify (W2, W4), and plan
	// (PendingApplies queue) stages; zero-valued in pure read-mode
	// Runs so the memory footprint cost for legacy callers is one
	// nil pointer. Lazily initialized by WriteClosure(); reset at
	// task entry by ResetWriteClosure (mirror of ResetEvidenceClosure).
	// Accessor methods live in write_closure.go.
	writeClosure *WriteClosure

	// Session 11 C0' ClassificationGrep state — reset per analyze
	// dispatch via ResetClassificationGrep. classificationGrepTriggered
	// is flipped to true in Round 2 when the trigger conditions fire
	// (LLM subject confidence < floor, CGEC E1 cue override disagrees,
	// etc). The validator in agent.validateAnalyzerPrescanToolCall
	// reads this flag to decide whether a line-level grep call is
	// allowed. classificationObservations accumulates the line-level
	// grep results for the reconcile step in buildAnalysisIR; the
	// observations stay on MutableState (not TurnAArtifacts) so no
	// downstream stage ever treats them as evidence.
	classificationGrepTriggered bool
	classificationGrepCalls     int // count of line-level calls made in Round 2
	classificationGrepBytes     int // cumulative match bytes returned
	classificationObservations  []ClassificationObs

	// reconcileObservations records every reconcile / inference event
	// the analyzer pipeline made during this dispatch. Consumed by
	// EmitReconcileSummary at run end so operators can grep
	// "[reconcile-shadow]" for the observability snapshot. Schema-v4
	// observability layer — separate channel from CGEC enforcers
	// because reconcile is upstream of evidence closure and runs even
	// when no enforcer fires.
	reconcileObservations []ReconcileObservation

	// logTriage is the validated output of the log_triage pre-stage.
	// Written once by the log_triager agent via SetLogTriage; read by
	// the analyzer (entity merge, intent override, RequiredFiles seed)
	// and by future consumers. Nil means "no log attached, stage
	// skipped, or stage degraded" — every reader MUST nil-check.
	logTriage *LogBundle

	// logSegments is the opaque payload produced by the two-step
	// log-triage controller's Step A (segmentation). It carries a
	// JSON-marshalled []tool.LogSegment slice so the types package
	// can hold it without importing internal/tool. The two-step
	// controller unmarshals on read. Nil means either two-step was
	// not invoked or the segmenter failed.
	logSegments []byte

	// perfTrace is the validated PerfBundle produced by the
	// perf_triage pre-stage (the HarmonyOS-HiTrace / Android-
	// systrace companion to log_triage). Written once by the
	// perf_triager agent via SetPerfTrace. Nil means either no
	// --htrace attachment was provided, the stage was skipped, or
	// emit_perf_trace failed — every reader MUST nil-check. Lives
	// alongside logTriage so both channels can seed analyzer hints
	// in the same Run.
	perfTrace *PerfBundle

	// perfSegments is the perf-channel companion to logSegments.
	// Holds the JSON-marshalled []tool.PerfSegment slice produced by
	// emit_perf_segmentation in Step A of the perf two-step fallback.
	// The two-step controller unmarshals on read. Nil means either
	// two-step was not invoked or the segmenter failed.
	perfSegments []byte
}

// ReconcileObservation is one decision the analyzer pipeline made
// while reconciling LLM-emitted classification against deterministic
// rules and predicates. Recorded centrally so the per-Run summary can
// surface confidence distribution + rule firing rate without each
// reconcile site reaching into observability state directly.
type ReconcileObservation struct {
	Field      string             `json:"field"`           // intent / complexity / shape / subject / axis
	Before     string             `json:"before"`          // LLM-emitted value (canonical form)
	After      string             `json:"after"`           // post-reconcile value
	Confidence float64            `json:"confidence"`      // LLM-emitted confidence on this dimension
	RuleFired  string             `json:"rule_fired"`      // human-readable rule name (or "none" when no override)
	Predicates SemanticPredicates `json:"predicates"`      // snapshot of LLM predicates at decision time
}

// TurnAArtifacts is the P2.1 handoff payload from Turn A (explorer)
// to Turn B (extractor). It is a snapshot of everything Turn B needs
// to derive structured emit_evidence / emit_answer_symbol /
// emit_hypothesis_verdict items WITHOUT calling read_file or grep
// itself.
//
// Why a struct instead of letting Turn B re-read the BusContext
// directly: the extractor is forbidden from running tools, so it
// cannot re-derive any state that was lost during the handoff. Any
// fact that must reach the answer slate has to be in this struct
// when Turn A's ParseOutput closes. The struct is therefore the
// contract surface the two evaluators both depend on, and the
// session-1 ship pins its shape so session-2 wiring on either side
// cannot silently drop a field.
//
// Lifecycle:
//
//  1. Turn A's ParseOutput populates the struct via
//     MutableState.SetTurnAArtifacts. This happens at end-of-stage
//     after ensureStructuredEvidence + grounding + ranking, so the
//     evidence slice already reflects every deterministic
//     (concrete-value / mechanism / flow) item the explorer has.
//
//  2. The orchestrator dispatches StageExtract; the extractor's
//     BuildInitialInstruction reads the snapshot via TurnAArtifacts() and
//     bakes the relevant pieces into Turn B's prompt.
//
//  3. After Turn B's ParseOutput finishes, ResetTurnAArtifacts()
//     clears the buffer so the next per-task explore→extract cycle
//     starts clean (intra-Run self-loops + REPL turn boundary).
//
// The fields are intentionally minimal — Session 2 will iterate on
// what Turn B actually needs. Anything we omit here can be added
// incrementally as a backwards-compatible struct field; anything we
// include and stop using costs a 5-line removal commit.
type TurnAArtifacts struct {
	// UserQuestion is the original task question, plumbed through so
	// Turn B can quote it back in its prompt without re-deriving from
	// AnalysisIR.RequestModel (which is normalized and may have lost
	// the user's exact phrasing).
	UserQuestion string

	// InvestigationNotes is the sequence of per-iteration assistant
	// content blocks the explorer accumulated. Each entry is one ReAct
	// loop iteration's worth of LLM narrative. Turn B's prompt may
	// include a digest of these to ground its extraction in the same
	// language Turn A used.
	InvestigationNotes []string

	// ReadFiles is the de-duplicated list of repository-relative file
	// paths Turn A fetched via read_file. Used by Turn B to constrain
	// its emit_evidence / emit_answer_symbol Source citations to
	// files that were actually read (a structural defense against the
	// LLM citing a file it never saw).
	ReadFiles []string

	// ToolResults is the raw tool result history from Turn A, in
	// chronological order. Carries grep / read_file / repo_map
	// outputs so Turn B can re-scan them without burning iterations.
	// Subject to pruneToolHistory so the slice is bounded.
	ToolResults []ToolResult

	// EvidenceItems is the deterministic evidence the explorer's
	// ParseOutput already produced (concrete values, flow findings,
	// mechanism scan, grounded markdown items if the legacy channel
	// is still on). Turn B uses these as a starting point and may
	// emit additional items via emit_evidence; the merge happens at
	// drain time via mergeEvidenceItems.
	EvidenceItems []EvidenceItem

	// FlowFindings is the dataflow analysis output from Turn A.
	// Carries pre-extracted source→sink chains that are useful for
	// Turn B's chain rendering.
	FlowFindings []FlowFindingDigest

	// TerminalEvidenceCount is the count of EvidenceItems that Turn A's
	// deterministic extraction pipeline identified as terminal-literal
	// answer candidates (i.e. the items that hasTerminalEvidence /
	// identifyAnswerChains' strictAnswerItems filter would admit). It
	// is the β baseline for Phase 9's cardinality validator: when Turn
	// B's emit_answer_symbol claims CompletenessComplete, the
	// validator checks len(emit items) ≥ max(TerminalEvidenceCount,
	// len(AnalysisIR.AnswerContract.MustInclude)) and downgrades /
	// retries on mismatch.
	//
	// This count is NOT len(EvidenceItems) — most evidence items are
	// not terminal-literal answer candidates (they are [DIRECT] facts,
	// [MECHANISM] steps, etc.). It must be computed by Turn A at
	// handoff time using the same predicate identifyAnswerChains uses,
	// so the two numbers are directly comparable.
	//
	// Zero-value is safe: it means "Turn A produced no terminal-literal
	// candidates" which Phase 9 treats as "no β constraint — the
	// baseline collapses to len(AnswerContract.MustInclude) alone".
	TerminalEvidenceCount int
}

// Phase1RankedFile is the minimum projection of the explorer's
// keyword_search result that downstream CGEC checks need. Carrying a
// trimmed struct (not the explorer's internal keywordFileScore) keeps
// internal/types decoupled from internal/agent while giving the
// pre-complete phase1-unread gate enough signal to sort and surface
// the right files.
type Phase1RankedFile struct {
	// Path is the repo-relative file path, canonicalised the same way
	// ReadSet entries are canonicalised (see tool/ground/path.go
	// CanonicalRepoRelative) so a direct map lookup correctly reports
	// "was this file read?".
	Path string

	// Score is the merged keyword-search score produced by
	// explorer.keywordSearch. Higher = more keyword / repomap evidence.
	// Carried so retry hints can cite a concrete ranking position.
	Score float64

	// ExactEntityRank is >0 when keyword_search found a unique exact
	// entity anchor for this file (symbol_exact / path_exact /
	// qualified_symbol_exact). Carried through MutableState so
	// pre-complete gates can distinguish a true user-named anchor from
	// broader structural hints such as RequiredFiles.
	ExactEntityRank int
}

// HypothesisVerdict is the structured verdict the extractor (Turn B)
// emits for a single hypothesis from AnalysisIR.HypothesisSet. It is
// the on-the-wire shape of the emit_hypothesis_verdict tool's items
// and the input shape of MutableState.MarkHypothesis (the D7 carve-out
// API landing in P6).
//
// HypothesisID is matched against AnalysisIR.HypothesisSet[*].ID at
// drain time; unknown IDs are diagnosed but not silently dropped, so
// a typo in the LLM's emission cannot disappear a real hypothesis.
//
// Citation is a single file:line pointer (the same shape downstream
// renderers expect for any cite) that the renderer can use to anchor
// the verdict in the final answer. Empty when the verdict is purely
// inferential — but inferential verdicts must be 'inconclusive', not
// 'confirmed' / 'rejected'.
type HypothesisVerdict struct {
	HypothesisID string           `json:"hypothesis_id"`
	Status       HypothesisStatus `json:"status"`
	Rationale    string           `json:"rationale,omitempty"`
	Citation     string           `json:"citation,omitempty"`
}

// NewMutableState constructs a MutableState seeded with the given
// raw objective (user question). Use this instead of zero-value
// literals so the internal mutex is paired correctly with its data.
func NewMutableState(objective string) *MutableState {
	return &MutableState{objective: objective}
}

// SetRepoRoot caches the orchestrator's -repo root so lazy-init of
// the evidenceClosure (via EvidenceClosure()) can pass the same
// root into NewEvidenceClosure. Call once per Run right after
// NewMutableState — multiple Runs of the same orchestrator
// overwrite the previous value. Idempotent on equal roots.
//
// If the closure has already been lazy-init'd with an older root,
// this propagates the new value in place so subsequent
// canonicalisations use it. Safe to call with "" — canonicalisation
// then degrades to the historical strip-"./" behaviour, matching
// the pre-session-22 semantics that tests rely on.
// RepoRoot returns the cached -repo root. Useful for agent code that
// has only the Mutable handle but needs to know the active worktree
// path (write-mode apply/verify dispatches swap RepoRoot to the
// worktree via SetRepoRoot).
func (m *MutableState) RepoRoot() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.repoRoot
}

func (m *MutableState) SetRepoRoot(root string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.repoRoot = root
	if m.evidenceClosure != nil {
		m.evidenceClosure.SetRepoRoot(root)
	}
}

// Objective returns the raw user question / task description.
func (m *MutableState) Objective() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.objective
}

// SetObjective atomically replaces the objective string.
func (m *MutableState) SetObjective(s string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objective = s
}

// SearchGraph returns the opaque handle previously stored by
// SetSearchGraph. Consumers that need a typed pointer must do a type
// assertion in their own package so internal/types does not depend on
// internal/tool/repomap.
func (m *MutableState) SearchGraph() any {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.searchGraph
}

// SetSearchGraph stores the repomap graph so downstream tools (notably
// emit_evidence's grounder) can reuse it without re-invoking
// BuildOrLoadGraph. Pass nil to clear.
func (m *MutableState) SetSearchGraph(g any) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.searchGraph = g
}

// LogTriage returns the validated LogBundle produced by the log_triage
// pre-stage, or nil when no log was attached, the stage was skipped,
// or the stage degraded. Readers MUST nil-check. Returns the stored
// pointer directly (no defensive copy): the bundle is treated as
// immutable after SetLogTriage, and both fields that readers mutate
// (none exist today) would be a contract violation. Keeping the
// pointer lets downstream consumers type-assert and cache without
// an allocation.
func (m *MutableState) LogTriage() *LogBundle {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.logTriage
}

// SetLogTriage stores the validated LogBundle produced by the
// log_triager agent. Called at most once per Run — the log_triage
// pre-stage runs exactly once before analyze. The two-step controller
// also calls SetLogTriage(nil) between partial-segment dispatches to
// clear the slot before the next partial emit. Explicit per-turn
// cleanup is not needed — each orchestrator.Run constructs a fresh
// MutableState.
func (m *MutableState) SetLogTriage(b *LogBundle) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logTriage = b
}

// PerfTrace returns the validated PerfBundle produced by the
// perf_triage pre-stage, or nil when no HiTrace/atrace was attached
// or the stage degraded. Mirrors LogTriage for the performance
// channel. Readers MUST nil-check.
func (m *MutableState) PerfTrace() *PerfBundle {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.perfTrace
}

// SetPerfTrace stores the validated PerfBundle produced by the
// perf_triager agent. Called at most once per Run from the
// perf_triage pre-stage. The orchestrator calls SetPerfTrace(nil)
// at Run start to clear any stale state so tests that reuse a
// MutableState do not see yesterday's trace.
func (m *MutableState) SetPerfTrace(b *PerfBundle) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.perfTrace = b
}

// SetLogSegments stores the opaque JSON-marshalled segment payload
// produced by the two-step segmentation tool. The bytes are copied
// so the caller can reuse its buffer after the call. Pass nil to
// clear.
func (m *MutableState) SetLogSegments(raw []byte) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(raw) == 0 {
		m.logSegments = nil
		return
	}
	cp := make([]byte, len(raw))
	copy(cp, raw)
	m.logSegments = cp
}

// LogSegments returns the stored segmentation payload, or nil when
// none exists. Caller owns unmarshalling.
func (m *MutableState) LogSegments() []byte {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.logSegments) == 0 {
		return nil
	}
	out := make([]byte, len(m.logSegments))
	copy(out, m.logSegments)
	return out
}

// SetPerfSegments stores the opaque JSON-marshalled
// []tool.PerfSegment payload from Step A of the perf two-step
// fallback. Mirrors SetLogSegments exactly so the two-step
// controllers (one per channel) share a single mental model.
// Pass nil to clear.
func (m *MutableState) SetPerfSegments(raw []byte) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(raw) == 0 {
		m.perfSegments = nil
		return
	}
	cp := make([]byte, len(raw))
	copy(cp, raw)
	m.perfSegments = cp
}

// PerfSegments returns the stored perf-segmentation payload, or nil
// when none exists.
func (m *MutableState) PerfSegments() []byte {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.perfSegments) == 0 {
		return nil
	}
	out := make([]byte, len(m.perfSegments))
	copy(out, m.perfSegments)
	return out
}

// Phase1Ranking returns a defensive copy of the stored ranked file
// list. Returns nil when no ranking has been set (no explore dispatch
// has run in this Run, or keyword_search produced no hits).
func (m *MutableState) Phase1Ranking() []Phase1RankedFile {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.phase1Ranking) == 0 {
		return nil
	}
	out := make([]Phase1RankedFile, len(m.phase1Ranking))
	copy(out, m.phase1Ranking)
	return out
}

// SetPhase1Ranking atomically replaces the stored ranking. The explorer
// calls this once per dispatch right after keyword_search runs so the
// pre-complete gate and retry-hint renderer see a consistent snapshot.
// Pass nil / empty to clear.
func (m *MutableState) SetPhase1Ranking(files []Phase1RankedFile) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(files) == 0 {
		m.phase1Ranking = nil
		return
	}
	m.phase1Ranking = make([]Phase1RankedFile, len(files))
	copy(m.phase1Ranking, files)
}

// AppendDispatchToolResult pushes a tool result onto the per-dispatch
// running buffer. BaseAgent.executeTool calls this after every
// successful tool execution so tools that run later in the same
// ReAct loop (emit_evidence's grounder is the motivating consumer)
// can see the earlier results.
func (m *MutableState) AppendDispatchToolResult(r ToolResult) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dispatchToolResults = append(m.dispatchToolResults, r)
}

// DispatchToolResults returns a snapshot of the per-dispatch running
// buffer. Snapshot (not alias) so callers that iterate while a
// sibling appends stay on a stable view.
func (m *MutableState) DispatchToolResults() []ToolResult {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.dispatchToolResults) == 0 {
		return nil
	}
	out := make([]ToolResult, len(m.dispatchToolResults))
	copy(out, m.dispatchToolResults)
	return out
}

// ResetDispatchToolResults clears the per-dispatch running buffer.
// BaseAgent.Execute calls this at loop entry so a fresh dispatch
// never inherits results from the previous one.
func (m *MutableState) ResetDispatchToolResults() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dispatchToolResults = nil
}

// Result returns the finalizer's final answer recorded for this run.
// Empty until recordTaskFinalize has fired.
func (m *MutableState) Result() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.result
}

// SetResult atomically replaces the recorded final answer.
func (m *MutableState) SetResult(s string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.result = s
}

// RequestModel returns a pointer to the analyzer-emitted RequestModel,
// or nil when no emit_analysis call has landed yet. Read by
// analyzer.ParseOutput after the ReAct loop exits to build AnalysisIR.
// The returned pointer is a snapshot copy — callers must not mutate
// the TermGraph slices in place.
func (m *MutableState) RequestModel() *RequestModel {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.requestModel == nil {
		return nil
	}
	cp := *m.requestModel
	return &cp
}

// SetRequestModel stores the analyzer's v3 RequestModel emitted via
// the emit_analysis tool. Callers pass a fully-populated RequestModel;
// the stored value is a deep-enough copy that the analyzer can read it
// out once the ReAct loop closes.
func (m *MutableState) SetRequestModel(rm RequestModel) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := rm
	m.requestModel = &cp
}

// AppendEvidence appends one or more LLM-emitted evidence items to the
// per-run buffer. Written by the emit_evidence tool; read by the
// explorer's ensureStructuredEvidence after the ReAct loop exits.
//
// P1.1: this is the structured replacement for the markdown-parsed
// evidence channel (parseEvidenceItems).
// Tools fill this buffer instead of asking the LLM to write a markdown
// header that a regex then walks. The two channels are merged in
// ensureStructuredEvidence so the structured and markdown channels run
// simultaneously and dedup on StableEvidenceID.
func (m *MutableState) AppendEvidence(items []EvidenceItem) {
	if m == nil || len(items) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emittedEvidence = append(m.emittedEvidence, items...)
}

// EmittedEvidence returns a snapshot of the LLM-emitted evidence buffer.
// The returned slice shares its backing array with the internal state —
// callers must not mutate it in place.
func (m *MutableState) EmittedEvidence() []EvidenceItem {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.emittedEvidence) == 0 {
		return nil
	}
	out := make([]EvidenceItem, len(m.emittedEvidence))
	copy(out, m.emittedEvidence)
	return out
}

// ResetEmittedEvidence clears the buffer. Called by the explorer's
// cross-Run reset path so a stage re-dispatch starts from an empty
// emitted-evidence state, matching how investigationNotes is reset.
func (m *MutableState) ResetEmittedEvidence() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emittedEvidence = nil
}

// SetEmittedAnswerSymbols atomically replaces the answer-symbol
// buffer and the accompanying completeness claim (P2.1 Phase 9
// set-level semantics). The emit_answer_symbol tool calls this on
// every invocation; subsequent calls REPLACE the prior slate,
// matching the "last writer wins" retry contract: on a mismatch
// retry the LLM either raises the list or downgrades the claim, and
// either way the new batch wins.
//
// The claim is validated for IsValid() and coerced to
// CompletenessUnknown on invalid input, so a buggy producer cannot
// corrupt the buffer with an unknown enum value. The items slice is
// defensively copied so a later mutation on the caller's side cannot
// race with reader goroutines.
func (m *MutableState) SetEmittedAnswerSymbols(items []AnswerSymbol, claim CompletenessClaim) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(items) == 0 {
		m.emittedAnswerSymbols = nil
	} else {
		m.emittedAnswerSymbols = append([]AnswerSymbol(nil), items...)
	}
	if !claim.IsValid() {
		claim = CompletenessUnknown
	}
	m.emittedAnswerSymbolCompleteness = claim
}

// EmittedAnswerSymbols returns a snapshot of the LLM-emitted answer
// symbol buffer paired with the set-level completeness claim. Both
// travel together so the reader cannot accidentally use one without
// checking the other. The slice is a defensive copy; the claim is a
// string enum so it is copied by value.
func (m *MutableState) EmittedAnswerSymbols() ([]AnswerSymbol, CompletenessClaim) {
	if m == nil {
		return nil, CompletenessUnknown
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.emittedAnswerSymbols) == 0 {
		return nil, m.emittedAnswerSymbolCompleteness
	}
	out := make([]AnswerSymbol, len(m.emittedAnswerSymbols))
	copy(out, m.emittedAnswerSymbols)
	return out, m.emittedAnswerSymbolCompleteness
}

// ResetEmittedAnswerSymbols clears both the buffer and the claim at
// the start of a new extractor dispatch. Mirror of ResetEmittedEvidence.
func (m *MutableState) ResetEmittedAnswerSymbols() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emittedAnswerSymbols = nil
	m.emittedAnswerSymbolCompleteness = CompletenessUnknown
}

// AppendEmittedHypothesisVerdicts merges LLM-emitted hypothesis
// verdicts (P2.1 Turn B emit_hypothesis_verdict channel) into the
// per-run buffer. Non-empty HypothesisID entries use last-wins
// semantics so repeated auto-verdict / override cycles do not spam
// duplicate verdict lines into downstream prompts; malformed empty-ID
// entries still append verbatim so diagnostics are not hidden. The
// extractor's ParseOutput drains this buffer at end-of-stage and
// routes the verdicts through MutableState.MarkHypothesis (the D7
// carve-out API on AnalysisIR). Sister API to AppendEvidence and
// AppendEmittedAnswerSymbols.
func (m *MutableState) AppendEmittedHypothesisVerdicts(items []HypothesisVerdict) {
	if m == nil || len(items) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range items {
		if strings.TrimSpace(item.HypothesisID) == "" {
			m.emittedHypothesisVerdicts = append(m.emittedHypothesisVerdicts, item)
			continue
		}
		replaced := false
		for i := range m.emittedHypothesisVerdicts {
			if m.emittedHypothesisVerdicts[i].HypothesisID == item.HypothesisID {
				m.emittedHypothesisVerdicts[i] = item
				replaced = true
				break
			}
		}
		if !replaced {
			m.emittedHypothesisVerdicts = append(m.emittedHypothesisVerdicts, item)
		}
	}
}

// EmittedHypothesisVerdicts returns a snapshot of the verdict buffer.
// Returned slice is a copy; safe to retain across subsequent appends.
func (m *MutableState) EmittedHypothesisVerdicts() []HypothesisVerdict {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.emittedHypothesisVerdicts) == 0 {
		return nil
	}
	out := make([]HypothesisVerdict, len(m.emittedHypothesisVerdicts))
	copy(out, m.emittedHypothesisVerdicts)
	return out
}

// ResetEmittedHypothesisVerdicts clears the buffer at the start of a
// new extractor dispatch. Mirror of ResetEmittedEvidence.
func (m *MutableState) ResetEmittedHypothesisVerdicts() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emittedHypothesisVerdicts = nil
}

// SetAnswerDocument atomically replaces the P2.2 structured answer
// payload. Called by the emit_answer_document tool after the schema
// validator has accepted the LLM's emission. Set-replace semantics:
// a later call fully overwrites any previous document, so a
// correction retry from the finalizer's ReAct loop always wins. A nil
// argument clears the buffer (the same effect as ResetAnswerDocument).
//
// The input is defensively deep-copied so a later mutation on the
// caller's side cannot race with reader goroutines through the
// AnswerDocument accessor.
func (m *MutableState) SetAnswerDocument(doc *AnswerDocument) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.answerDocument = CloneAnswerDocument(doc)
}

// AnswerDocument returns a defensive deep copy of the buffered
// structured answer payload, or nil when no document has been set on
// this MutableState. The returned pointer is independent of the
// internal state — callers cannot mutate the buffered document in
// place.
func (m *MutableState) AnswerDocument() *AnswerDocument {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return CloneAnswerDocument(m.answerDocument)
}

// ResetAnswerDocument clears the answer payload at the start of a
// fresh per-task dispatch. Mirror of ResetTurnAArtifacts /
// ResetEmittedAnswerSymbols. Called from runTaskGraph so multi-task
// runs do not drag a stale document from task N into task N+1.
func (m *MutableState) ResetAnswerDocument() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.answerDocument = nil
}

// SetChangePlan atomically installs the B0 write-mode ChangePlan
// produced by the planner's emit_change_plan tool. Set-replace
// semantics mirror SetAnswerDocument: a later call overwrites any
// previous plan from a correction retry; a nil argument clears.
//
// Unlike SetAnswerDocument, the input is stored by pointer without
// a deep copy — ChangePlan is conceptually write-once-read-many and
// no caller ever mutates a ChangePlan in place. Downstream readers
// (the plan stage hook, cmd/root.go's plan-out writer) treat the pointer
// as immutable.
func (m *MutableState) SetChangePlan(plan *ChangePlan) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.changePlan = plan
}

// ChangePlan returns the buffered write-mode plan, or nil when no
// plan has been emitted. Read by the plan stage hook after the planner
// agent completes; cmd/root.go reads it post-Run to serialize
// JSON to disk via --plan-out.
func (m *MutableState) ChangePlan() *ChangePlan {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.changePlan
}

// ResetChangePlan clears the plan buffer at the start of a fresh
// per-task dispatch. Mirror of ResetAnswerDocument so a multi-task
// Run cannot leak a plan from a prior task into a later one. Safe
// to call with nil receiver and when no plan has been set.
//
// Also clears partialChangePlan: a stale skeleton from a prior task
// must not persist into the next planner dispatch.
func (m *MutableState) ResetChangePlan() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.changePlan = nil
	m.partialChangePlan = nil
}

// SetPartialChangePlan installs the in-progress skeleton produced by
// emit_plan_skeleton. Set-replace semantics mirror SetChangePlan;
// callers using emit_plan_skeleton + emit_plan_change must clear
// any stale plan via ResetChangePlan first to avoid the "two
// concurrent plans" pathology.
func (m *MutableState) SetPartialChangePlan(plan *ChangePlan) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.partialChangePlan = plan
}

// PartialChangePlan returns the buffered in-progress plan, or nil
// when no skeleton has been emitted (or the last emit_plan_change
// has already promoted it to ChangePlan). Read by emit_plan_change
// to find the slot to fill and by tests to assert intermediate
// state.
func (m *MutableState) PartialChangePlan() *ChangePlan {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.partialChangePlan
}

// PromotePartialToChangePlan atomically moves the partial plan into
// the final ChangePlan slot and clears Partial. Called by
// emit_plan_change once all placeholders are filled and all
// validators pass. Caller is responsible for running validators
// FIRST — this method is purely a slot transition.
func (m *MutableState) PromotePartialToChangePlan() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.changePlan = m.partialChangePlan
	m.partialChangePlan = nil
}

// SetChangeReport atomically installs the B1.3 verify-stage
// ChangeReport. run_tests calls this after parsing the test
// runner output; emit_test_results (optional LLM narrative)
// may replace it with a decorated version. Pointer storage
// mirrors SetChangePlan — no deep copy because ChangeReport
// is conceptually write-once-read-many.
func (m *MutableState) SetChangeReport(report *ChangeReport) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.changeReport = report
}

// ChangeReport returns the buffered verify-stage report, or nil
// when verify has not run (or when it ran but produced no report
// — the latter is a bug worth logging). Read by the verify stage hook
// to render Result + call WriteChangeReportToFile.
func (m *MutableState) ChangeReport() *ChangeReport {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.changeReport
}

// ResetChangeReport clears the report buffer at per-task entry.
// Mirror of ResetChangePlan.
func (m *MutableState) ResetChangeReport() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.changeReport = nil
}

// SetBaselineReport installs the pre-apply test snapshot. Called by
// the apply stage hook before coder dispatch when baseline capture is
// enabled. Pointer storage mirrors SetChangeReport.
func (m *MutableState) SetBaselineReport(report *ChangeReport) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.baselineReport = report
}

// BaselineReport returns the pre-apply test snapshot, or nil when
// baseline capture was skipped. Consumed by CritNoRegression.
func (m *MutableState) BaselineReport() *ChangeReport {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.baselineReport
}

// ResetBaselineReport clears the baseline slot at per-task entry.
// Mirror of ResetChangeReport.
func (m *MutableState) ResetBaselineReport() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.baselineReport = nil
}

// SetPlanningHint installs the retry feedback text the planner's
// next dispatch should fold into its instruction. Called by the
// verify→plan retry loop (B2.3) after a verify failure with
// remaining retry budget.
func (m *MutableState) SetPlanningHint(hint string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.planningHint = hint
}

// PlanningHint returns the retry feedback text, or "" when no
// retry hint is pending. Read by the planner's
// BuildInitialInstruction on retry dispatches.
func (m *MutableState) PlanningHint() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.planningHint
}

// ResetPlanningHint clears the retry hint; called after the
// planner has consumed it.
func (m *MutableState) ResetPlanningHint() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.planningHint = ""
}

// SetTurnAArtifacts stores the P2.1 handoff snapshot from the
// explorer (Turn A) for the extractor (Turn B) to consume. Called
// from the explorer's ParseOutput at end-of-stage when
// agent.TwoTurnExplorerEnabled() is true. The setter takes a value
// (not a pointer) so the explorer cannot accidentally mutate the
// snapshot after handoff — Turn B always sees a frozen view.
func (m *MutableState) SetTurnAArtifacts(a TurnAArtifacts) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Defensive copy of the slice headers so a later append on the
	// caller side cannot mutate the buffered snapshot in place.
	snap := a
	if a.InvestigationNotes != nil {
		snap.InvestigationNotes = append([]string(nil), a.InvestigationNotes...)
	}
	if a.ReadFiles != nil {
		snap.ReadFiles = append([]string(nil), a.ReadFiles...)
	}
	if a.ToolResults != nil {
		snap.ToolResults = append([]ToolResult(nil), a.ToolResults...)
	}
	if a.EvidenceItems != nil {
		snap.EvidenceItems = append([]EvidenceItem(nil), a.EvidenceItems...)
	}
	if a.FlowFindings != nil {
		snap.FlowFindings = append([]FlowFindingDigest(nil), a.FlowFindings...)
	}
	m.turnAArtifacts = &snap
}

// TurnAArtifacts returns a snapshot of the buffered handoff payload,
// or nil when no Turn A has run yet on this MutableState. The
// returned pointer is to a fresh copy — callers cannot mutate the
// buffered state in place.
func (m *MutableState) TurnAArtifacts() *TurnAArtifacts {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.turnAArtifacts == nil {
		return nil
	}
	out := *m.turnAArtifacts
	if m.turnAArtifacts.InvestigationNotes != nil {
		out.InvestigationNotes = append([]string(nil), m.turnAArtifacts.InvestigationNotes...)
	}
	if m.turnAArtifacts.ReadFiles != nil {
		out.ReadFiles = append([]string(nil), m.turnAArtifacts.ReadFiles...)
	}
	if m.turnAArtifacts.ToolResults != nil {
		out.ToolResults = append([]ToolResult(nil), m.turnAArtifacts.ToolResults...)
	}
	if m.turnAArtifacts.EvidenceItems != nil {
		out.EvidenceItems = append([]EvidenceItem(nil), m.turnAArtifacts.EvidenceItems...)
	}
	if m.turnAArtifacts.FlowFindings != nil {
		out.FlowFindings = append([]FlowFindingDigest(nil), m.turnAArtifacts.FlowFindings...)
	}
	return &out
}

// ResetTurnAArtifacts clears the buffered handoff snapshot. Called
// at the start of a fresh per-task explore→extract cycle (intra-Run
// self-loops + REPL turn boundary) so a stale Turn A from the
// previous task cannot leak into the next extractor dispatch.
func (m *MutableState) ResetTurnAArtifacts() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.turnAArtifacts = nil
}

// AppendPrescanSummary appends `summary` to the per-dispatch
// pre-scan summary blob, lowercased, followed by a newline
// separator. Called by `analyzerEvaluator.Observe` once per
// successful pre-scan tool result (repo_map / grep files_only=true
// / list_files). The blob is bounded in practice by the analyzer's
// pre-scan budget (`AnalysisLimits.MaxPrescanRounds`, default 2) ×
// the per-result blob size, so no explicit size cap is enforced
// here — callers rely on the runtime gate to stop the dispatch
// before this grows unbounded.
//
// Lowercase-at-write means `PrescanSummaryBlob()` is a hot-path
// zero-allocation read the emit_analysis tool and the validator
// can call without touching strings.ToLower. Callers MUST NOT
// assume the blob preserves the verbatim Summary — it is a
// case-folded corpus for substring probing, not a trace record.
func (m *MutableState) AppendPrescanSummary(summary string) {
	if m == nil || summary == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prescanSummaryBlob.WriteString(strings.ToLower(summary))
	m.prescanSummaryBlob.WriteByte('\n')
}

// PrescanSummaryBlob returns the lowercased concatenation of every
// pre-scan summary appended so far during the current analyze
// dispatch. Returns an empty string when no summary has been
// appended (e.g. the dispatch had no pre-scan rounds, or the
// caller is running outside the analyze stage).
func (m *MutableState) PrescanSummaryBlob() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.prescanSummaryBlob.String()
}

// ResetPrescanSummary zeroes the pre-scan summary blob so a new
// analyze dispatch starts with a clean buffer. Called by
// `analyzerEvaluator.BuildInitialInstruction` at dispatch entry,
// symmetrical with the prescanRounds reset. Cross-dispatch
// retries from the orchestrator each get an empty blob.
func (m *MutableState) ResetPrescanSummary() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prescanSummaryBlob.Reset()
}

// ── Session 11 C0' ClassificationGrep accessors ────────────────────

// ClassificationObs is one line-level grep match observation
// captured in analyzer Round 2. The reconcileFromObservations step
// in buildAnalysisIR consumes these to refine AnalysisIR fields
// (answer_subject.kind, answer_shape, question_kind, entity_axes)
// without the classification signal ever leaking into TurnAArtifacts
// or EvidenceClosure.
type ClassificationObs struct {
	Pattern string    // grep pattern
	Path    string    // file the match came from (repo-relative)
	Line    int       // 1-indexed line number
	Text    string    // matched line content (trimmed)
	Kind    string    // declarative.Kind string label (informational)
	TS      time.Time // capture time (for diagnostics)
}

// SetClassificationGrepTriggered flips the gate flag. Called by
// analyzerEvaluator after it decides Round 2 should open the
// line-level grep capability. The validator
// (validateAnalyzerPrescanToolCall) consults the flag before
// admitting a files_only=false grep call.
func (m *MutableState) SetClassificationGrepTriggered(v bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.classificationGrepTriggered = v
}

// ClassificationGrepTriggered reports whether the gate is open.
// Zero-cost when unset.
func (m *MutableState) ClassificationGrepTriggered() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.classificationGrepTriggered
}

// BumpClassificationGrepCall records one line-level call and the
// byte cost of its returned match block. Returns the new counters
// so the caller can compare against caps without an extra lock
// acquire.
func (m *MutableState) BumpClassificationGrepCall(bytes int) (calls, totalBytes int) {
	if m == nil {
		return 0, 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.classificationGrepCalls++
	if bytes > 0 {
		m.classificationGrepBytes += bytes
	}
	return m.classificationGrepCalls, m.classificationGrepBytes
}

// ClassificationGrepCalls returns the number of line-level calls
// admitted so far (for budget check in the validator).
func (m *MutableState) ClassificationGrepCalls() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.classificationGrepCalls
}

// ClassificationGrepBytes returns the cumulative match bytes
// admitted so far.
func (m *MutableState) ClassificationGrepBytes() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.classificationGrepBytes
}

// AppendClassificationObs records one line-level match into the
// sidecar observation channel. Safe to call concurrently.
func (m *MutableState) AppendClassificationObs(obs ClassificationObs) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.classificationObservations = append(m.classificationObservations, obs)
}

// ClassificationObservations returns a defensive copy of the
// sidecar channel. Consumed by reconcileFromObservations in
// buildAnalysisIR. Empty slice means the C0' path did not fire
// (either Round 2 was skipped or classification did not need
// verification).
func (m *MutableState) ClassificationObservations() []ClassificationObs {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.classificationObservations) == 0 {
		return nil
	}
	out := make([]ClassificationObs, len(m.classificationObservations))
	copy(out, m.classificationObservations)
	return out
}

// AppendReconcileObservation records one reconcile event into the
// observability channel. Safe to call concurrently. Empty Field is
// silently dropped (the field name is the index key in the per-Run
// summary).
func (m *MutableState) AppendReconcileObservation(obs ReconcileObservation) {
	if m == nil || obs.Field == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reconcileObservations = append(m.reconcileObservations, obs)
}

// ReconcileObservations returns a defensive copy of the recorded
// reconcile events. Consumed by analyzer EmitReconcileSummary at
// run end. Empty slice means no reconcile rule fired AND no
// observation was recorded (a clean LLM classification).
func (m *MutableState) ReconcileObservations() []ReconcileObservation {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.reconcileObservations) == 0 {
		return nil
	}
	out := make([]ReconcileObservation, len(m.reconcileObservations))
	copy(out, m.reconcileObservations)
	return out
}

// ResetReconcileObservations clears the channel at the start of a
// new analyze dispatch.
func (m *MutableState) ResetReconcileObservations() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reconcileObservations = nil
}

// ResetClassificationGrep clears every C0' gate/budget/observation
// at the start of a new analyze dispatch. Called from
// analyzerEvaluator.BuildInitialInstruction symmetrically with
// ResetPrescanSummary.
func (m *MutableState) ResetClassificationGrep() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.classificationGrepTriggered = false
	m.classificationGrepCalls = 0
	m.classificationGrepBytes = 0
	m.classificationObservations = nil
}

// SetInvestigationComplete marks the investigation as complete with
// the given reason. Called by emit_investigation_complete tool.
func (m *MutableState) SetInvestigationComplete(reason string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.investigationComplete = true
	m.investigationCompleteReason = reason
}

// IsInvestigationComplete reports whether the LLM has called
// emit_investigation_complete during this explore window.
func (m *MutableState) IsInvestigationComplete() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.investigationComplete
}

// InvestigationCompleteReason returns the LLM's stated reason for
// completing investigation. Empty when not yet called.
func (m *MutableState) InvestigationCompleteReason() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.investigationCompleteReason
}

// ResetInvestigationComplete clears the completion flag so a retried
// explore window starts fresh. Called by the orchestrator before
// each explore dispatch (alongside ExploreBudget reset).
func (m *MutableState) ResetInvestigationComplete() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.investigationComplete = false
	m.investigationCompleteReason = ""
	m.absenceJustification = ""
}

// SetAbsenceJustification stores the LLM's declarative claim that
// the answer is a justified zero. See the absenceJustification field
// doc for the contract.
func (m *MutableState) SetAbsenceJustification(just string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.absenceJustification = just
}

// AbsenceJustification returns the LLM-declared zero rationale.
// Empty when not set.
func (m *MutableState) AbsenceJustification() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.absenceJustification
}

// SetExploreBudget installs a fresh ExploreBudget for the current
// per-task explore window. The orchestrator calls this once at the
// start of runTaskGraph with per-tool caps derived from the
// analyzer's NodeBudgetHints. A nil argument clears the budget so
// non-DAG call paths degrade to "no throttling".
func (m *MutableState) SetExploreBudget(b *ExploreBudget) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exploreBudget = b
}

// ExploreBudget returns a defensive clone of the currently
// installed budget. Callers that just need counters (RecordToolCall /
// BudgetRemaining) should use those methods directly; this getter
// is for the explorer's ReAct loop to pass the clone into
// sourcemix.BudgetForTool.
func (m *MutableState) ExploreBudget() *ExploreBudget {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.exploreBudget == nil {
		return nil
	}
	return m.exploreBudget.Clone()
}

// RecordToolCall bumps the per-tool and overall used counters for
// the given canonical tool name. No-op when no budget is installed.
func (m *MutableState) RecordToolCall(tool string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.exploreBudget == nil {
		return
	}
	if m.exploreBudget.PerToolUsed == nil {
		m.exploreBudget.PerToolUsed = make(map[string]int)
	}
	m.exploreBudget.PerToolUsed[tool]++
	m.exploreBudget.OverallUsed++
}

// RefundToolCall decrements the per-tool and overall counters for
// `tool` when a prior RecordToolCall should not have spent budget —
// e.g. read_file that failed with a path-not-found error while the
// LLM was still triangulating the repo layout. Clamps at zero so a
// spurious refund never underflows into negative usage. No-op when
// no budget is installed.
func (m *MutableState) RefundToolCall(tool string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.exploreBudget == nil {
		return
	}
	if m.exploreBudget.PerToolUsed != nil {
		if m.exploreBudget.PerToolUsed[tool] > 0 {
			m.exploreBudget.PerToolUsed[tool]--
		}
	}
	if m.exploreBudget.OverallUsed > 0 {
		m.exploreBudget.OverallUsed--
	}
}

// BudgetRemaining returns the smaller of the per-tool and overall
// remaining cap for `tool`. When no budget is installed, returns a
// very large number so callers can treat the return as "plenty".
func (m *MutableState) BudgetRemaining(tool string) int {
	if m == nil {
		return 1 << 30
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.exploreBudget == nil {
		return 1 << 30
	}
	const inf = 1 << 30
	perRem := inf
	if cap, ok := m.exploreBudget.PerToolCap[tool]; ok {
		perRem = cap - m.exploreBudget.PerToolUsed[tool]
	}
	overallRem := inf
	if m.exploreBudget.OverallCap > 0 {
		overallRem = m.exploreBudget.OverallCap - m.exploreBudget.OverallUsed
	}
	if perRem < overallRem {
		return perRem
	}
	return overallRem
}

// StageReport is the synthesized narrative an agent leaves behind
// at the end of its ReAct loop. It carries the LLM's own summary of
// what it discovered or decided so downstream stages can read prior
// reasoning instead of reverse-engineering it from raw tool dumps.
//
// Reports are append-only and accumulate across the whole pipeline
// run. Each stage dispatch produces at most one report (the last
// non-empty assistant message of that ReAct loop).
type StageReport struct {
	Stage    PipelineStage `json:"stage"`
	Agent    AgentName     `json:"agent"`
	Findings string        `json:"findings"`
}

// RepoFact is a single discovered fact about the repository.
type RepoFact struct {
	Key         string  `json:"key"`
	Value       string  `json:"value"`
	Source      string  `json:"source"`
	EvidenceRef string  `json:"evidence_ref,omitempty"`
	Confidence  float64 `json:"confidence"`
}

// ToolResult records the outcome of a tool invocation.
type ToolResult struct {
	ToolName  string    `json:"tool_name"`
	Summary   string    `json:"summary"`
	RawRef    string    `json:"raw_ref,omitempty"`
	Success   bool      `json:"success"`
	Timestamp time.Time `json:"timestamp"`
}

// MCPResponse records a response from an MCP server.
type MCPResponse struct {
	ServerName string    `json:"server_name"`
	Method     string    `json:"method"`
	Summary    string    `json:"summary"`
	RawRef     string    `json:"raw_ref,omitempty"`
	Success    bool      `json:"success"`
	Timestamp  time.Time `json:"timestamp"`
}

// ExecutionSignals tracks boolean signals produced by agents. After
// the 2026-04-14 simplification only HasEnoughFacts remains — the
// write-pipeline signals (HasPlan, HasPatch, *ReviewPassed,
// VerificationPassed) all gated stages that no longer exist.
type ExecutionSignals struct {
	HasEnoughFacts bool `json:"has_enough_facts"`
}

// BusContext is the central data structure passed through the pipeline.
//
// The Mutable region is the only part of BusContext that tools may
// write to during the ReAct loop. Everything else is mutated only
// via Orchestrator.applyStageOutput so the orchestrator stays the
// single point of stage-level state changes.
type BusContext struct {
	// Mutable holds the tool-writable region (currently the working
	// task list). Tools see this pointer through the narrowed busCtx
	// constructed in BaseAgent.executeTool, so direct mutations are
	// visible immediately to subsequent tool calls and prompt rebuilds.
	Mutable *MutableState `json:"mutable,omitempty"`

	TaskState TaskState `json:"task_state"`

	PipelineStage PipelineStage `json:"pipeline_stage"`
	ActiveAgent   AgentName     `json:"active_agent"`

	RepoRoot  string   `json:"repo_root"`
	Branch    string   `json:"branch"`
	Commit    string   `json:"commit"`
	ModuleMap []string `json:"module_map,omitempty"`

	// WorkDir is a per-trace temporary directory used by tools to
	// offload large outputs to disk (see internal/tool/blob.go). The
	// orchestrator creates and tears it down around Run(). When empty
	// (e.g. unit tests with a zero-value BusContext) tools degrade to
	// inline previews without persisting full content.
	WorkDir string `json:"work_dir,omitempty"`

	RepoFacts     []RepoFact          `json:"repo_facts,omitempty"`
	EvidenceItems []EvidenceItem      `json:"evidence_items,omitempty"`
	FlowFindings  []FlowFindingDigest `json:"flow_findings,omitempty"`
	AnswerChains  []AnswerChain       `json:"answer_chains,omitempty"`  // deterministic answer-relevance envelopes (typed)
	AnswerSymbols []AnswerSymbol      `json:"answer_symbols,omitempty"` // L0-2: structured terminal symbols extracted from AnswerChains
	// AnswerSymbolCompleteness is the P2.1 set-level authority claim
	// attached to AnswerSymbols. It is written by whichever stage
	// populated AnswerSymbols (explorer flag=off path, or extractor
	// flag=on path) and read by context/builder.go §Answer Symbols to
	// pick the correct rendering branch (Translation mode for
	// "complete", softened floor prompt for "lower_bound", drop the
	// section entirely for "unknown"/zero). Zero value is the
	// fail-closed default. See types.CompletenessClaim for the three-
	// level authority ladder.
	AnswerSymbolCompleteness CompletenessClaim `json:"answer_symbol_completeness,omitempty"`
	ToolResults              []ToolResult      `json:"tool_results,omitempty"`
	MCPResponses             []MCPResponse     `json:"mcp_responses,omitempty"`
	StageReports             []StageReport     `json:"stage_reports,omitempty"`

	Signals ExecutionSignals `json:"signals"`

	Constraints []string `json:"constraints,omitempty"`
	Preferences []string `json:"preferences,omitempty"`

	// Language is the response-language code from the -lang flag.
	// builder.go reads it to generate the language preference
	// directive. Empty / "off" / "none" disables the directive.
	Language string `json:"language,omitempty"`

	LastTransitionReason string `json:"last_transition_reason,omitempty"`
	TraceID              string `json:"trace_id"`

	// AnalysisIR is the Analyzer v3 structured output. Set once by the
	// analyze stage via StageOutput.AnalysisIR → applyStageOutput and
	// never rewritten thereafter — the analyzer is the sole writer.
	// Downstream stages may write hypothesis status or per-node
	// execution state through dedicated APIs; the top-level pointer
	// itself stays read-only.
	AnalysisIR *AnalysisIR `json:"analysis_ir,omitempty"`

	// AttachedLog carries a runtime log excerpt (panic, exception
	// stack, sanitizer diagnostic, traceback) the user attached to
	// the current request via --log / --log-text or the REPL /log
	// command. Empty when no log is attached. Populated once at
	// orchestrator.Run entry; never rewritten mid-pipeline.
	//
	// Flows into the analyzer via AgentContext.AttachedLog, where
	// internal/analysis/logtriage.ValidateBundle extracts stack frames to seed
	// AnalyzerHints.Entities (function names, error literals) and
	// EvidencePlan.RequiredFiles (frame file paths). Other stages
	// read the field for observability only; analyzer is the sole
	// consumer today.
	//
	// Kept separate from the user's question string so the normalizer
	// never sees raw log noise — a 2000-line panic pasted into
	// `request` would otherwise flood TermGraph with hundreds of
	// spurious kindLiteral surfaces.
	AttachedLog string `json:"attached_log,omitempty"`

	// AttachedHitrace carries a HarmonyOS HiTrace or Android systrace
	// excerpt (ftrace-compatible text) attached via --htrace /
	// --htrace-text. Read once by the StagePerfTriage pre-stage's
	// perf_triager agent; never mirrored into AgentContext because
	// only that one consumer needs it. Empty string = skip perf_triage.
	//
	// Kept separate from AttachedLog so log_triage and perf_triage
	// can run independently and contribute orthogonal hints — a single
	// Run may carry both a panic log and a jank trace.
	AttachedHitrace string `json:"attached_hitrace,omitempty"`

	// Mode is the pipeline's execution mode for this Run(). Zero-value
	// ("" / ModeRead) preserves pre-B0 read-only behavior byte-
	// identically — the orchestrator's Mode-dispatch switch falls
	// through to the existing runTaskPhase path when Mode is ModeRead
	// or the empty string. Set once at Run() entry from the CLI/yaml
	// layer; immutable for the rest of the Run. See
	// internal/types/pipeline_mode.go for constants.
	Mode PipelineMode `json:"mode,omitempty"`

	// MainRepoRoot is the original target repo root the user passed
	// via --repo. In read-only mode equals RepoRoot throughout the
	// Run. In write modes the apply/verify stages swap RepoRoot to
	// the worktree checkout path while the plan and finalize stages
	// see MainRepoRoot. Empty in pre-B0 callers (tests) — code that
	// needs the "original root" falls back to RepoRoot in that case.
	MainRepoRoot string `json:"main_repo_root,omitempty"`

	// WorktreePath is the git worktree directory the apply/verify
	// stages operate inside. Populated by the Plan / Apply stage
	// entry; cleared (via worktree.Discard) by the orchestrator's
	// global defer at Run end. Empty in read-only mode. Same
	// canonicalization discipline as RepoRoot (absolute path).
	WorktreePath string `json:"worktree_path,omitempty"`

	// PlanPath is the absolute path of the ChangePlan JSON the
	// apply/verify stages consume, supplied by --plan-file or the
	// REPL /plan state. Empty in plan mode (the plan stage
	// produces a plan; it does not consume one) and in read mode.
	PlanPath string `json:"plan_path,omitempty"`
}

// AgentContext provides the narrowed view of BusContext for a single agent.
type AgentContext struct {
	AgentName AgentName     `json:"agent_name"`
	Stage     PipelineStage `json:"stage"`

	Objective string `json:"objective"`

	// AnalysisIR aliases BusContext.AnalysisIR for agents that have
	// opted into the v3 pipeline. Still nil for legacy call paths —
	// consumers MUST nil-check before reading.
	AnalysisIR *AnalysisIR `json:"-"`

	RelevantFacts []string            `json:"relevant_facts,omitempty"`
	RelevantFiles []string            `json:"relevant_files,omitempty"`
	EvidenceItems []EvidenceItem      `json:"evidence_items,omitempty"`
	FlowFindings  []FlowFindingDigest `json:"flow_findings,omitempty"`
	AnswerChains  []AnswerChain       `json:"answer_chains,omitempty"`
	AnswerSymbols []AnswerSymbol      `json:"answer_symbols,omitempty"`
	// AnswerSymbolCompleteness mirrors BusContext.AnswerSymbolCompleteness
	// for the narrowed agent view. Read by finalize's prompt builder
	// to pick Translation / softened-floor / shape-based rendering.
	AnswerSymbolCompleteness CompletenessClaim `json:"answer_symbol_completeness,omitempty"`
	RelevantToolSummaries    []string          `json:"relevant_tool_summaries,omitempty"`
	RelevantMCPNotes         []string          `json:"relevant_mcp_notes,omitempty"`
	PriorReports             []StageReport     `json:"prior_reports,omitempty"`

	// UnverifiedAnalyzerFindings is populated from
	// EvidenceClosure.UnverifiedFindings() at BuildAgentContext time.
	// The CGEC findings_validator (I1) records each path/symbol the
	// analyzer mentioned that the repo graph could not confirm; the
	// prompt builder renders these as a dedicated "## Unverified
	// Analyzer Findings" section so every downstream agent sees a
	// single consolidated warning list instead of having to re-derive
	// it from the annotated StageReport text. Capped at render time
	// to keep prompt-size bounded. Empty / nil when no findings were
	// flagged.
	UnverifiedAnalyzerFindings []UnverifiedFinding `json:"unverified_analyzer_findings,omitempty"`

	// SubjectMatches is populated from EvidenceClosure.AllSubjectMatches()
	// at BuildAgentContext time. The explorer's rankChainsBySubject (C2)
	// writes a per-chain score against the expected AnswerSubject; the
	// extractor and finalizer prompt builders render the top-K entries
	// so Turn B emits answer symbols / leading citations aligned with
	// the chain the framework believes best matches the question. Empty
	// / nil when no chain was scored (analyzer-only dispatch, subject
	// unknown, or sub-agents that bypass the ranker).
	SubjectMatches map[string]float64 `json:"subject_matches,omitempty"`

	// ExpectedAnswerSubject mirrors AnalysisIR.RequestModel.AnswerSubject
	// into the agent view so the prompt builder can include the kind
	// label next to SubjectMatches without re-plumbing the whole IR.
	// Zero-value Kind means no expected subject — renderers should
	// suppress the section.
	ExpectedAnswerSubject AnswerSubject `json:"expected_answer_subject,omitempty"`

	Constraints []string `json:"constraints,omitempty"`
	Preferences []string `json:"preferences,omitempty"`
	Language    string   `json:"language,omitempty"`

	MissingPiece MissingPiece `json:"missing_piece"`

	// RetryHint is propagated from TaskState.RetryHint when the
	// previous dispatch of this same stage flagged itself as
	// insufficient. The prompt builder renders it as the most
	// prominent user section to override the agent's instinct to
	// repeat the same approach.
	RetryHint string `json:"retry_hint,omitempty"`

	RepoRoot string `json:"repo_root"`
	Branch   string `json:"branch"`
	Commit   string `json:"commit"`
	WorkDir  string `json:"work_dir,omitempty"`

	// Mutable aliases the orchestrator's BusContext.Mutable so that
	// tools dispatched from this agent (via BaseAgent.executeTool)
	// can write to the shared region. This breaks the strict
	// "AgentContext is a value-only narrow view" rule for one specific
	// pointer field by design — see MutableState's doc.
	Mutable *MutableState `json:"-"`

	// SearchGraph is the opaque read-only handle to the repomap graph
	// the main explorer seeded on Mutable.SearchGraph(). Duplicated
	// onto AgentContext so SubAgents (which deliberately run with
	// Mutable=nil) can reuse the same graph instance without a second
	// BuildOrLoadGraph round-trip. The main-agent path usually reads
	// through Mutable.SearchGraph() directly; this field is the
	// Mutable-free alternative for sub-agents.
	SearchGraph any `json:"-"`

	// ThinkAloud controls the "Think Aloud" system section. Resolved
	// per-agent from providers.yaml (default.think_aloud, overridable
	// per agents.<name>.think_aloud). When false, the section is
	// omitted from the prompt — useful for providers that natively
	// combine reasoning with tool calls without needing the directive.
	ThinkAloud bool `json:"think_aloud,omitempty"`

	// MaxIterOverride, when > 0, overrides the agent's default
	// MaxIterations for this single dispatch. Used by the orchestrator
	// to grant extra explorer iterations for multi-topic questions.
	// This is the OUTER ReAct-loop ceiling (BaseAgent.Execute's
	// for-loop bound). Distinct from the per-evaluator inner caps
	// below — agents whose ShouldStop uses a soft/hard pair (planner /
	// coder / verifier / extractor) have a separate per-dispatch
	// channel because conflating them would force the outer loop to
	// terminate at the inner soft cap, eliminating the recovery
	// window the inner pair was designed to provide.
	MaxIterOverride int `json:"-"`

	// PlannerSoftIterCapOverride, when > 0, overrides the planner
	// evaluator's default soft iteration cap for this single
	// dispatch. The hard cap is derived as soft + the agent-settings
	// recovery slack (default hard - default soft). Set by the
	// orchestrator's per-dispatch scaling block based on the
	// analyzer's complexity + sub-topic signals; the planner reads it
	// in BuildInitialInstruction. Decoupled from MaxIterOverride
	// because the planner's outer ReAct ceiling (default 20) MUST
	// remain a strict superset of the inner soft cap so the
	// soft→hard recovery window can actually run.
	PlannerSoftIterCapOverride int `json:"-"`

	// AttachedLog mirrors BusContext.AttachedLog into the narrowed
	// agent view. Consumed by the log_triager agent and rendered as
	// a prompt section for every stage (so the LLM sees the raw log
	// body for narrative context). Empty when no log was attached.
	AttachedLog string `json:"attached_log,omitempty"`

	// AttachedHitrace mirrors BusContext.AttachedHitrace for the
	// perf_triager agent. Only the perf_triage stage reads this
	// field today; other stages rely on the structured PerfTrace
	// pointer below to avoid flooding prompts with raw ftrace text.
	AttachedHitrace string `json:"attached_hitrace,omitempty"`

	// LogTriage mirrors Mutable.LogTriage() into the narrowed agent
	// view, so consumers that want the structured bundle (analyzer's
	// entity merge, reconcileIntent, RequiredFiles seeding) can read
	// it without reaching through Mutable. Nil when no log was
	// attached, the triage stage was skipped, or the stage degraded.
	// Readers MUST nil-check.
	LogTriage *LogBundle `json:"-"`

	// PerfTrace mirrors Mutable.PerfTrace() for consumers that want
	// the validated PerfBundle. Nil when no trace was attached, the
	// perf_triage stage was skipped, or emit_perf_trace failed.
	// Readers MUST nil-check.
	PerfTrace *PerfBundle `json:"-"`

	// PriorConvHidden gates whether the REPL-assembled Prior
	// Conversation block is HIDDEN from this agent's user prompt.
	// The orchestrator resolves the flag from AgentSettings.
	// PriorConvPolicy before dispatch; the prompt builder in
	// internal/context/builder.go reads it to decide whether to skip
	// the "Prior Conversation (reference only)" section.
	//
	// Inverted (Hidden instead of Visible) so the zero-value path —
	// unit tests, single-shot dispatches, legacy callers — preserves
	// the historical "Prior always visible" behaviour without
	// requiring every construction site to set a new field.
	//
	// The Objective field ALWAYS carries the full "Prior + Current"
	// string regardless of this flag — StripConversationPrefix and
	// SplitConversation continue to work unchanged. The flag only
	// gates the user-facing prompt section; analyzer's TermGraph
	// normalisation and explorer's ERM extraction keep routing
	// through StripConversationPrefix so they never ingest prior
	// text into entity/keyword ranking.
	PriorConvHidden bool `json:"-"`
}

// PromptSection is a titled block of content used in prompt construction.
type PromptSection struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// PromptContext holds the assembled prompt for an agent invocation.
type PromptContext struct {
	SystemSections []PromptSection `json:"system_sections"`
	UserSections   []PromptSection `json:"user_sections"`

	EnabledTools []string `json:"enabled_tools"`

	AgentName AgentName     `json:"agent_name"`
	Stage     PipelineStage `json:"stage"`
	SkillName string        `json:"skill_name"`
}
