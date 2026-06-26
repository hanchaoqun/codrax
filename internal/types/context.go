package types

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/canonpath"
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

	// RetryHintStage records which pipeline stage owns RetryHint. It lets
	// prompt construction enforce capability-scoped handoff: an exploration
	// forced-read hint must not be rendered as an extractor instruction after
	// the pipeline has moved forward. Empty preserves legacy Stage-based
	// scoping for older tests and serialized state.
	RetryHintStage PipelineStage `json:"retry_hint_stage,omitempty"`

	// SoftAnalyzerError carries the analyzer-stage exhaustion error
	// when the orchestrator falls back to buildDegradedSemanticIR.
	// Distinct from LastError because LastError is consumed by the
	// outer guard at runTaskPhase entry — setting LastError here
	// would skip explorer/extractor/finalizer dispatch entirely,
	// turning the degraded path into a no-op empty output.
	// SoftAnalyzerError is informational only: rendered into the
	// user-panel caveat after the degraded answer materialises, and
	// surfaced to operators via TaskState diagnostics. Empty in the
	// happy path.
	//
	// 2026-05-10 P-audit follow-up. Without this split, the
	// Fix-A degraded recovery IR could never actually run because
	// LastError already short-circuited Phase 2.
	SoftAnalyzerError string `json:"soft_analyzer_error,omitempty"`
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
	result string
	// resultIsPlain marks the current result as plain-text so
	// Renderer.RenderResult skips glamour markdown rendering.
	// Stage hooks (planPreHook / planPostHook / applyPreHook / etc.)
	// set this via SetResultPlain when they surface fail-loud
	// diagnostics that contain identifier-like tokens chroma would
	// otherwise split with ANSI codes. See SetResultPlain doc for
	// the production-trace context.
	resultIsPlain bool
	// finalAnswerMarkdownPath is presentation metadata for the current
	// Result(): the path to the raw markdown transcript written under
	// .codrax/output. It is deliberately separate from result so prompt
	// builders and memory do not treat the file hint as answer content.
	finalAnswerMarkdownPath string
	// finalAnswerHTMLPath is the system-rendered self-contained HTML sibling
	// of finalAnswerMarkdownPath. It is presentation metadata only; model
	// prompts and memory continue to use Result().
	finalAnswerHTMLPath string
	// outputTranscriptRequest is the current Run's exact user request for
	// final-answer output artifacts. REPL uses it to preserve paste-expanded
	// text in .codrax/output markdown/html while keeping Mutable.objective free
	// to carry prior-conversation scaffolding. This is run-scoped state: do not
	// read a reused Orchestrator's cross-turn setter directly when writing
	// artifacts.
	outputTranscriptRequest string
	requestModel            *RequestModel
	multiRepoFocusDecision  *MultiRepoFocusDecision
	emittedEvidence         []EvidenceItem
	// answerSurfaceRevision is bumped by mutators that can affect
	// BuildAnswerSurfacePlan. BusContext-level answer-plan caches use
	// it as a cheap freshness boundary so finalizer validators can
	// reuse compiled projections without observing stale Mutable data.
	answerSurfaceRevision uint64
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
	// emittedAnswerSymbolDeclaredCount mirrors the LLM's
	// self-declared count from the most recent emit_answer_symbol
	// call. Zero = no claim made (back-compat). Finalize-stage
	// validators (e.g. ViolDeclaredCountDrift producer in
	// orchestrator/contract_check.go) compare this against the
	// rendered doc count to catch "claimed N but rendered only M"
	// drift.
	emittedAnswerSymbolDeclaredCount int
	emittedHypothesisVerdicts        []HypothesisVerdict
	sourceInventoryAdvisory          SourceInventoryAdvisory
	sourceInventoryObservation       SourceInventoryObservation
	sourceInventoryAdvisoryHinted    bool
	sourceInventoryDiscoveryHinted   map[string]bool
	turnAArtifacts                   *TurnAArtifacts
	exploreForkTurnABaseNotesLen     int
	exploreForkTurnABaseBoundaryLen  int
	exploreForkTurnABaseToolLen      int
	exploreForkTurnABaseMCPLen       int
	exploreForkTurnABaseFlowLen      int
	exploreForkTurnABaseHandoffLen   int
	// cachedLabelSupport memoises the dot-qualified selector / anchor /
	// subject / object support pool drawn from turnAArtifacts.EvidenceItems.
	// Built lazily on first call to CachedLabelSupportTokens; cleared
	// whenever turnAArtifacts changes (Set / Reset). The cache key is
	// the internal turnAArtifacts pointer (NOT the value TurnAArtifacts()
	// returns — that helper produces a fresh defensive copy each call).
	cachedLabelSupport       map[string]struct{}
	cachedLabelSupportSource *TurnAArtifacts
	// searchGraph is an opaque handle to the repomap.Graph produced by
	// explorer.keywordSearch. Carried as `any` so internal/types stays
	// decoupled from internal/tool/repomap — consumers (emit_evidence,
	// grounding) type-assert on their own side. Set once per Run by
	// the explorer's BuildInitialInstruction so downstream tools can
	// share the same graph instance with zero I/O.
	searchGraph any

	// scopedSearchGraphs stores run-local projections derived from
	// searchGraph for repo_map subdirectory calls. Values are opaque for
	// the same import-cycle reason as searchGraph. SetSearchGraph clears
	// this cache so a projection cannot outlive the base graph.
	scopedSearchGraphs map[string]any

	// symbolOracle is the cached SymbolOracle the orchestrator
	// builds once per Run from the same graph as searchGraph. Tools
	// in internal/tool can't import internal/tool/repomap to
	// construct the oracle directly (cycle: repomap → tool →
	// repomap), so the orchestrator wires this field via
	// SetSymbolOracle and the tools read via SymbolOracle().
	// Mirrors the SetSearchGraph pattern. Carried as the typed
	// SymbolOracle interface so internal/types stays decoupled
	// from internal/tool/repomap.
	//
	// 2026-05-10 P1.
	symbolOracle SymbolOracle

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
	dispatchToolResults         []ToolResult
	turnAArtifactsRevision      uint64
	dispatchToolResultsRevision uint64
	searchGraphRevision         uint64
	// traceQueryRuntimeObservationCount persists the precise signal that a
	// successful trace_query call published hard-grounded runtime-artifact
	// observations during the current explore/extract cycle. The per-dispatch
	// buffer above is intentionally cleared between ReAct turns; this counter
	// lets pre-complete gates remember deterministic trace facts without asking
	// the model to re-materialise external trace rows as repo evidence.
	traceQueryRuntimeObservationCount int
	// exploreForkTraceQueryRuntimeObservationBase is the parent's count at fork
	// time so MergeExploreFork can fold back only the fork's new observations.
	exploreForkTraceQueryRuntimeObservationBase int
	// answerDocumentV2 is the block-only carrier (
	// docs/migration/block_only_carrier.md) — the structured final-
	// answer payload written by the emit_answer_document tool (one
	// atomic set per dispatch) and read by the finalizer's
	// ParseOutput to render user-visible prose. Set semantics mirror
	// SetEmittedAnswerSymbols: a later call REPLACES any previous
	// document so a correction retry from the ReAct loop cleanly
	// wins. Cross-task reset (runTaskGraph) calls
	// ResetAnswerDocumentV2 at per-task entry so stale state cannot
	// leak between tasks in a multi-task run.
	answerDocumentV2 *AnswerDocumentV2

	// answerDisplayAttachments are user-visible fallback fragments
	// recovered from malformed final-answer emits. They are rendered
	// after AnswerDocumentV2 but are not part of the structured answer
	// contract, citation pool, or validator surface.
	answerDisplayAttachments []AnswerDisplayAttachment

	// finalizerNoToolAnswerDrafts preserves answer-document-shaped
	// assistant text that the finalizer produced without using the
	// tool channel. It is a same-task recovery aid only: normal
	// emit_answer_document output still owns answerDocumentV2, and
	// recovered drafts must be parsed/validated by downstream callers
	// before they become user-visible answer state.
	finalizerNoToolAnswerDrafts []FinalizerNoToolAnswerDraft

	// lastRejectedAnswerDocumentV2 caches the most recent structurally
	// decoded answer_document draft that failed a validator gate before it
	// could be persisted as the accepted AnswerDocumentV2. Local models often
	// answer the next retry with a patch against that draft even though no
	// accepted base exists yet; tool-level compatibility may use this clone as
	// a patch base, but the merged document must still pass the normal
	// validation pipeline before it can be accepted.
	lastRejectedAnswerDocumentV2 *AnswerDocumentV2

	// lastEmitFromPatch flags whether the most recent answerDocumentV2
	// write came from emit_answer_document_patch (true) or full
	// emit_answer_document (false). Phase 2-B4 (V2 runtime
	// consolidation, 2026-05-04) — surfaces inheritance lineage in
	// retry summary for observability. v3 B4 (2026-05-04): set by
	// SetAnswerDocumentV2WithMutation based on MutationKind ==
	// MutationPartial.
	lastEmitFromPatch bool

	// lastAnswerDocAttemptShape caches the size profile of the
	// PREVIOUS emit_answer_document attempt (success or failure)
	// for the catastrophic-regression detector. See
	// AnswerDocAttemptShape doc for the load-bearing failure mode
	// it guards against.
	lastAnswerDocAttemptShape *AnswerDocAttemptShape

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

	// bestPlan / bestReport hold the highest-passing (ChangePlan,
	// ChangeReport) pair observed across verify→plan retry iterations
	// in ModeApply. clearForReplan checks every completed iteration's
	// (passed, total) score against this slot; on regression (next
	// iteration scored fewer passes than best), clearForReplan
	// restores the best pair instead of carrying the worse plan
	// forward. Reset at per-task entry.
	//
	// Bug provenance: eval Batch K forth-py — a 3-iteration retry
	// loop went 49/54 → 46/54 → 46/54, locking in the regression
	// because each iteration unconditionally overwrote the prior plan.
	// Without a "best score latch" the LLM's noisy retry trajectory
	// can permanently lose ground that earlier iterations had won.
	bestPlan   *ChangePlan
	bestReport *ChangeReport

	// analyzerRetryHint carries IR-field-level coherence detail from
	// the prior buildAnalysisIR rejection. The next analyze dispatch's
	// prependEmitRetryDirective consumes + clears it before the LLM
	// sees its prompt, so the model gets concrete contradiction
	// signals rather than a generic "gate rejected" message.
	analyzerRetryHint string

	// planningHint carries structured feedback from a failed verify
	// run back to the planner on B2.3 retry dispatches. Non-empty
	// only during retry iterations; cleared on first retry entry.
	// The planner's BuildInitialInstruction checks this slot and
	// prepends a "Previous attempt failed — revise" section.
	planningHint string

	// answerRetryEvents accumulates one AnswerRetryEvent per
	// per-stage retry that fired during a read-mode Run. Append-
	// only inside a Run; cleared by ResetAnswerRetryEvents when a
	// fresh Run begins (orchestrator.Run defensive reset). Read at
	// end-of-Run by the answer_reviewer dispatch (commit 51 Gap 3)
	// to decide whether to emit a per-repo answer pitfall pattern.
	// Empty = the Run was trivial (no retries); the reviewer skips.
	answerRetryEvents []AnswerRetryEvent

	// learningFailures accumulates one entry per
	// reflector / answer_reviewer / taxonomy-store failure during
	// the Run (commit 59 Batch E.1, audit HIGH #12). Pre-fix these
	// were silently logging.Warning'd and discarded; users had no
	// way to know cross-Run learning was broken. Now tallied and
	// surfaced at Run end so an operator running a long REPL session
	// notices "10 Runs, 8 learning failures" and investigates.
	learningFailures []LearningFailure

	// midLoopHintExhaustions records each (HintKey, Reason) pair the
	// loop-policy per-key cap saturated during the current Run. The
	// loop-policy returns OutcomeContinue on cap-saturate to avoid
	// force-stopping a dispatch whose other hint keys may still be
	// productive, but the saturated key itself is a "model is
	// structurally unable to satisfy this gate" signal — surfacing
	// it here lets the success-criteria layer and downstream Runs
	// pivot to fallback instead of paying for the dispatch's
	// remaining iteration quota in silence.
	// docs/design/post_phase2a_forensic_followups.md §2.1.G #4 + #7.
	midLoopHintExhaustions []MidLoopHintExhaustion

	// iterationLedger accumulates one IterationRecord per completed
	// verify→plan retry attempt. The orchestrator's clearForReplan
	// appends to it BEFORE resetting ChangePlan / ChangeReport, so
	// the planner on the next retry sees the FULL history (not just
	// the most recent attempt or the best one) and can recognise
	// patterns the system has no business pre-classifying — which
	// approach was tried 3 times in a row, what the regression
	// trajectory looks like, etc.
	//
	// Append-only inside a Run; cleared by ResetIterationLedger
	// when a fresh write Run begins. Reflexion-style episodic
	// memory pattern (Shinn et al. 2023).
	iterationLedger []IterationRecord

	// planStageProbeReports records every dry-run probe the planner
	// fires during plan stage via run_tests(dry_run=true, verification_probe={...}). Distinct
	// from ChangeReport (which is the verify stage's authoritative
	// outcome) so a plan-stage probe NEVER pollutes the verify
	// channel and the verify→plan retry loop continues to read only
	// real verify results. Append-only inside a Run; reset at Run
	// boundary alongside the iteration ledger.
	planStageProbeReports []*ChangeReport

	// verifyFailureHandoff carries the latest failed post-apply
	// verification to the next planning round as typed evidence. It
	// deliberately SURVIVES the controller's per-round planning-state
	// reset (which wipes ChangeReport and the iteration ledger) so the
	// replan prompt can open with failure-local targets; the scheduler
	// replaces it on each new failure and clears it when the batch
	// verifies green or the run finishes.
	verifyFailureHandoff *VerifyFailureHandoff

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
	absenceJustification        string
	investigationResultKind     string
	investigationAggregateFacts []AnswerAggregateFact
	// exactContextRequiredFiles stores repo-relative production files
	// that the explorer structurally ranked as same-scope related-
	// context anchors for an exact-resolution task. When an
	// exact-absence answer wants to close with contextual guidance, the
	// completion validator requires at least one grounded evidence item
	// from this file set before allowing result_kind=absence. Reset per
	// explore window together with the other investigation latches.
	exactContextRequiredFiles []string
	// retainedAbsenceJustification / retainedInvestigationResultKind /
	// retainedInvestigationCompleteReason preserve the most recently
	// accepted terminal investigation state across explore-window
	// retries. ResetInvestigationComplete clears the per-window latch so
	// a new explorer dispatch starts fresh, but downstream stages still
	// need the accepted absence / resolved disposition and the closure
	// rationale from the last successful emit_investigation_complete
	// call. These retained fields survive window resets until the task
	// itself ends.
	retainedAbsenceJustification        string
	retainedInvestigationResultKind     string
	retainedInvestigationCompleteReason string
	retainedInvestigationAggregateFacts []AnswerAggregateFact

	// evidenceFloorWaiver (2026-05-10) is the model-declared typed
	// escape lane consumed by emit_investigation_complete's
	// forced-read and citation-floor gates. Set via
	// emit_investigation_complete's `evidence_floor_waiver` field;
	// empty / nil means "no waiver claimed, ordinary repo grounding
	// applies". Survives window resets for retry-local gate use
	// (resetting on retry would erase the model's confident judgment
	// and force re-declaration).
	evidenceFloorWaiver *EvidenceFloorWaiver

	// retainedEvidenceFloorWaiver is promoted only after a successful
	// emit_investigation_complete. Downstream answer-surface builders
	// read this stable copy so a waiver accepted syntactically but later
	// rejected by another completion gate cannot leak into finalization.
	retainedEvidenceFloorWaiver *EvidenceFloorWaiver

	// principalSpanWaiver carries the model-declared escape for the
	// callChainPrincipalSpanDowngrade gate. Set by
	// emit_investigation_complete's `principal_span_waiver` field; the
	// gate consumer reads it as "should I relax the source→sink
	// intermediate-evidence requirement?". Same retention semantics as
	// evidenceFloorWaiver — survives window resets so the model's
	// confident judgment does not have to be re-declared every retry.
	principalSpanWaiver *PrincipalSpanWaiver

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
	prescanRoundCount  int
	prescanRoundLimit  int
	prescanReady       bool

	// evidenceClosure is the cross-stage CGEC tracker (Citation-
	// Grounded Evidence Closure). Records ReadSet, PendingReads,
	// UnverifiedFinds, RepairDirectives, and per-round Fingerprints
	// so the four CGEC enforcers (chain promotion, grounder, pre-
	// complete check, convergence detector) share a single source of
	// truth. Lazily initialized by EvidenceClosure(); reset at task
	// entry by ResetEvidenceClosure (mirror of ResetTurnAArtifacts).
	// All accessor methods live in evidence_closure.go.
	evidenceClosure *EvidenceClosure

	// repairAttempts is the cross-scope attempt counter introduced
	// in W2.5 (docs/design/iteration_inflation_remediation.md §3
	// source #3). Tracks how many times each unique
	// (ViolKind, Fingerprint) has been dispatched for repair across
	// mid-loop / fallback / contract retry scopes combined. Once a
	// key exceeds MaxRepairAttemptsPerRoot, the orchestrator
	// transitions that violation to "materialise as caveat" mode
	// instead of dispatching another retry. Lazily initialised by
	// RepairAttempts(); reset at task entry by
	// ResetRepairAttempts.
	repairAttempts *RepairAttemptHistory

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

	// richnessTelemetry is the silent-softening / family-fit channel
	// added by B5-F2/F3 (post-shape-retirement consolidated audit
	// 2026-05-03). Pure observability: signals are appended at the
	// site of the silent rule (CompileFacetCoverage hard→soft
	// downgrade, ResolveQuestionFamily comparison-fallback) and
	// drained by operator-facing summary tooling. The channel never
	// drives a violation or retry per §9.5 ("system NEVER reacts to
	// a richness signal by silently inventing a new family").
	richnessTelemetry []RichnessTelemetrySignal

	// analyzerDecisions records every silent automatic decision
	// the analyzer / extractor makes (R8 — scenario reconcile /
	// completeness downgrade / pre-scan budget reject / subtopic
	// coherence retry). Pure observability — never drives a
	// violation or retry. Drained by operator-facing summary
	// tooling at end-of-Run.
	analyzerDecisions []AnalyzerDecisionSignal

	// analyzerBlockedToolIntents records analyze-stage tool calls that
	// were blocked by the classification-only boundary but still carry
	// useful routing metadata (for example read_file(path=...)). The
	// content read is never executed in analyze; downstream stages can
	// consume the non-content path/pattern intent as a candidate lead.
	analyzerBlockedToolIntents []AnalyzerBlockedToolIntent

	// retryState is the R14 typed retry-state contract surface
	// (post_shape_residual_audit.md, 2026-05-04). Populated by the
	// orchestrator + contract_check at retry-decision time;
	// consumed by the agent layer's renderRetryState helper to
	// produce the "Previous Emit / Active Violations / Required
	// Changes / Hard Rule" prompt sections on retry attempts.
	//
	// Lifecycle: nil on fresh dispatches (no rendering); populated
	// only when scheduler decides to retry the finalizer; reset
	// to nil after the retry dispatch consumes it (so a successful
	// retry doesn't leak prev state into a subsequent fresh Run).
	retryState *RetryState

	// repairExecutionPlan is the dispatch-ready owner queue stashed by
	// the retry-decision site (G1 post_v2_runtime_gap_remediation,
	// 2026-05-04). Carried as `any` so internal/types stays decoupled
	// from internal/orchestrator (mirrors the searchGraph pattern at
	// line ~108). Consumers (orchestrator retry-decision site) type-
	// assert on their own side.
	//
	// Lifetime mirrors retryState: overwritten on every retry-
	// decision pass, cleared by ResetForFallback (Finalizer / Extract /
	// Explore targets) and ResetRetryState. Nil on fresh dispatches /
	// no-retry runs.
	repairExecutionPlan any

	// logTriage is the validated output of the log_triage pre-stage.
	// Written once by the log_triager agent via SetLogTriage; read by
	// the analyzer (entity merge, intent override, RequiredFiles seed)
	// and by future consumers. Nil means "no log attached, stage
	// skipped, or stage degraded" — every reader MUST nil-check.
	logTriage *LogBundle

	// terminationProfile records which scheduler path carried the read
	// run into finalize and whether the grounding floor was waived.
	// Soft consumption only (telemetry, degradation caveats).
	terminationProfile *TerminationProfile

	// preStageDegradations records attached-artifact pre-stages that
	// failed to produce their bundle (the run proceeds artifact-blind).
	preStageDegradations []PreStageDegradation

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

	// writeAnalysisIR is the write-mode analyzer's structured output,
	// produced by the write_analyzer agent via emit_write_analysis.
	// Nil in read-mode Runs (the read analyzer populates AnalysisIR
	// instead). Coexists with AnalysisIR in write mode because the
	// existing read analyzer still runs as a classifier providing
	// keywords for ranking, while WriteAnalysisIR carries the
	// task-shape facts (kind / scope / risk / constraints / outcomes)
	// the write agents read directly. Readers MUST nil-check.
	writeAnalysisIR *WriteAnalysisIR

	// writeExplorationRequest / writeExplorationHandoff are the typed,
	// read-only bridge from a write workflow batch into source exploration
	// and back into planning. They are planning context only: they never
	// mutate files, never bypass ChangePlan validation, and never become
	// final-answer citation state.
	writeExplorationRequest *WriteExplorationRequest
	writeExplorationHandoff *WriteExplorationHandoff
	writeContextPack        *WriteContextPack
	writeWorkflowRun        *WriteWorkflowRun
	writeWorkflowDecision   []byte

	// planCritique is the optional pre-apply review text produced by
	// the plan_critic agent (commit 4 P1-F). Empty when the critic
	// is disabled (default), or when the critic ran but found no
	// risks. The critique is INFORMATIONAL only — it never
	// auto-rejects a plan; downstream surfaces (/plan show) render
	// it for the operator's eyes.
	planCritique string

	// tier2CompletenessHint is the LLM-natural FixHint surfaced by
	// the Phase 2 Tier 2 ERM completeness validators (count
	// question lacks deterministic-tool output, call-chain answer
	// has insufficient function mentions, config-precedence
	// missing a layer, ...). Set by the orchestrator at finalize
	// stage entry when a CompletenessValidator detects a gap;
	// drained into the finalize prompt so the LLM sees what to
	// route around. R6-clean: contains no internal pipeline
	// terms. Empty when no Tier 2 gap was detected.
	tier2CompletenessHint string

	// unvalidatedReasons collects per-language static-check stages
	// that were skipped because their toolchain was unavailable
	// (e.g. "rust:cargo not in PATH"). emit_change_plan's dry-
	// build helpers append entries when they bail out on toolchain
	// absence; emit_change_plan.Execute drains these into the
	// finalised ChangePlan.UnvalidatedReasons so /plan show can
	// render them. Cleared when SetChangePlan installs a fresh
	// plan so retries don't accumulate stale reasons.
	unvalidatedReasons []string

	// patchStyleNudgeFired is legacy bookkeeping for older callers that
	// probed a once-per-run line-structure hint. emit_change_plan now
	// treats patch style as a success-only advisory; noisy style signals
	// must never consume planner retry budget.
	patchStyleNudgeFired bool
}

// ReconcileObservation is one decision the analyzer pipeline made
// while reconciling LLM-emitted classification against deterministic
// rules and predicates. Recorded centrally so the per-Run summary can
// surface confidence distribution + rule firing rate without each
// reconcile site reaching into observability state directly.
type ReconcileObservation struct {
	Field      string             `json:"field"`      // intent / complexity / shape / subject / axis
	Before     string             `json:"before"`     // LLM-emitted value (canonical form)
	After      string             `json:"after"`      // post-reconcile value
	Confidence float64            `json:"confidence"` // LLM-emitted confidence on this dimension
	RuleFired  string             `json:"rule_fired"` // human-readable rule name (or "none" when no override)
	Predicates SemanticPredicates `json:"predicates"` // snapshot of LLM predicates at decision time
}

// AnalyzerDecisionSignal is a single observation emitted whenever
// the analyzer / extractor performs a silent automatic decision
// — quality-gate retry, scenario reconcile, completeness downgrade,
// pre-scan budget reject, etc. Pure observability: signals are
// appended at the decision site and drained by operator-facing
// summary tooling. The channel never drives a violation or retry
// (decisions already affect downstream state by their direct
// effect; telemetry is what surfaces the otherwise-silent choice
// to operators).
//
// Replaces ad-hoc logging.Warning lines that were the only signal
// operators had for these decisions. Each line still logs (so
// existing log parsers stay byte-stable), and additionally an
// AnalyzerDecisionSignal lands on Mutable for the run summary +
// future telemetry consumers.
//
// Kind values (open enum — adding a new decision site appends here):
//   - "scenario_reconciled"          — reconcileScenario flipped Scenario
//   - "completeness_downgraded"      — extractor downgraded
//     completeness=complete → lower_bound
//   - "prescan_rejected"             — analyzer prescan tool call rejected
//     (budget exhausted / terminal-emit mode)
//   - "analyzer_tool_rejected"       — analyze-stage tool boundary rejected a
//     content-reading or otherwise non-allowlisted tool before execution
//   - "subtopic_coherence_failed"    — gate.Run emitted subtopic_coherence
//     hard fail; analyzer is about to retry
type AnalyzerDecisionSignal struct {
	Kind   string `json:"kind"`
	Stage  string `json:"stage,omitempty"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Reason string `json:"reason,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// AnalyzerBlockedToolIntent captures non-content metadata from a tool call
// that analyze rejected before execution. It deliberately stores only
// routing-safe fields; no file contents, command output, or model prose enters
// this channel.
type AnalyzerBlockedToolIntent struct {
	ToolName string    `json:"tool_name"`
	Path     string    `json:"path,omitempty"`
	Pattern  string    `json:"pattern,omitempty"`
	Query    string    `json:"query,omitempty"`
	TS       time.Time `json:"ts,omitempty"`
}

// RichnessTelemetrySignal is a single observation emitted by the
// FacetCoverageContract / FamilyResolution machinery when the
// system silently softened a hard contract OR resolved a question
// to a family that does not capture its expressive structure.
//
// The channel is informational (never raises a violation). B5-F2 +
// B5-F3 add the writers; readers are TBD — the signal will surface
// in operator-facing logs (B6+) and inform the catalogue of when to
// add a new QFComparison family vs. when to extend an existing
// family. Per the consolidated audit §9.5, the system NEVER reacts
// to a richness signal by silently inventing a new family.
//
// Kind values:
//   - "facet_softened" — a HARD facet requirement was downgraded to
//     SOFT because no surface evidence matched any AcceptableForm.
//   - "family_underrepresented" — ResolveQuestionFamily returned a
//     family that doesn't model the question's structural axes
//     (e.g. comparison-class question with QuestionStructure.Buckets
//     ≥ 2 fell to QFCallChain / QFGeneric).
type RichnessTelemetrySignal struct {
	Kind        string `json:"kind"`
	FacetID     string `json:"facet_id,omitempty"`
	FacetKind   string `json:"facet_kind,omitempty"`
	Family      string `json:"family,omitempty"`
	BucketCount int    `json:"bucket_count,omitempty"`
	Reason      string `json:"reason"`
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

	// ValidationBoundaryNotes are system-authored, criterion-derived
	// notes recorded when an accepted investigation closure is allowed
	// to proceed even though one or more DAG validation criteria remain
	// unsatisfied. They are not model prose, not citations, and not a
	// reason to invent facts; downstream stages use them to converge the
	// answer surface or disclose an evidence/scope boundary instead of
	// reopening exploration for inherently under-constrained questions.
	ValidationBoundaryNotes []string

	// ReadFiles is the de-duplicated list of repository-relative file
	// paths Turn A fetched via read_file. Used by Turn B to constrain
	// its emit_evidence / emit_answer_symbol Source citations to
	// files that were actually read (a structural defense against the
	// LLM citing a file it never saw).
	ReadFiles []string

	// SourceLocalization is the typed read-side localization summary derived
	// from ReadFiles and grounded evidence sources. It is a handoff/audit
	// artifact only; read-mode hard gates still consume their existing precise
	// evidence and answer contracts.
	SourceLocalization *SourceLocalizationReview

	// ToolResults is the raw tool result history from Turn A, in
	// chronological order. Carries grep / read_file / repo_map
	// outputs so Turn B can re-scan them without burning iterations.
	// Bounded at snapshot time: the capture site and the cross-window
	// merge both apply a per-window and merged count + byte cap
	// (oldest dropped first, chronological order kept, at least one
	// successful investigation-class result always retained) so retry
	// windows cannot grow the slice without limit. pruneToolHistory
	// does NOT bound this slice — it only stubs LLM message history.
	ToolResults []ToolResult

	// HandoffCarriers is the bounded typed projection of tool repair,
	// supported JSON retry surfaces, accepted evidence identities, and typed
	// observation refs. It rides beside raw ToolResults so later stages can
	// consume structured repair/evidence handoff without parsing summaries.
	HandoffCarriers []ToolHandoffCarrier

	// MCPResponses is the raw MCP response history from Turn A. These
	// are external resource observations, never current-source citations.
	// Keeping them beside ToolResults lets downstream stages reuse typed
	// line/row coordinates without pushing the model back to grep/read_file.
	// The fork merge point bounds count + bytes with oldest-first dropping and
	// payload-bearing successful response preservation.
	MCPResponses []MCPResponse

	// AcceptedClosureReason is the model-authored rationale from the
	// successful emit_investigation_complete call. It is carried as
	// structured exploration context so downstream stages can preserve
	// the investigator's resolved count / set / boundary / verdict
	// instead of reconstructing from stale early grep notes. It is not
	// a citation and not system-synthesised answer text: finalization
	// must reconcile it with the typed evidence and tool outputs.
	AcceptedClosureReason string

	// AcceptedResultKind mirrors emit_investigation_complete.result_kind
	// for the successful closure ("resolved" or "absence"). Kept with
	// AcceptedClosureReason so extractor/finalizer can distinguish a
	// positive resolved set from a bounded no-hit result without
	// parsing prose.
	AcceptedResultKind string

	// AcceptedAggregateFacts are model-authored structured aggregates
	// emitted on the successful emit_investigation_complete call. They
	// preserve derived totals, unique-set counts, per-bucket counts,
	// and excluded-candidate counts without asking downstream stages to
	// re-parse prose closure text.
	AcceptedAggregateFacts []AnswerAggregateFact

	// SourceInventoryAdvisory is the system-compiled, graph-backed
	// candidate set for typed source inventory or attribute-bearing
	// enumeration shapes. It is advisory context, not answer text and not
	// a citation by itself. Downstream stages may use it to avoid repeated
	// broad searches, while preserving the model's final answer decision.
	SourceInventoryAdvisory SourceInventoryAdvisory

	// SourceInventoryObservation is the auditable repo-lens companion to
	// SourceInventoryAdvisory. It preserves count/list/support-ref invariants
	// for mechanical source-inventory facts without turning navigation rows
	// into model-authored answer text.
	SourceInventoryObservation SourceInventoryObservation

	// RuntimeObservationOnlyCompletion marks the narrow case where Turn
	// A legitimately completed from a structured external log / trace
	// artifact without required current-repo read/search evidence. It is
	// set only after the typed current-source lane is not required and a
	// successful emit_investigation_complete result_kind exists, so
	// downstream empty-investigation gates can accept the artifact
	// handoff without forcing fixture reads or repo citations.
	RuntimeObservationOnlyCompletion bool

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
// Citation is a single current-repo file:line pointer (the same shape
// downstream renderers expect for any cite) that the renderer can use
// to anchor the verdict in the final answer. Empty when the verdict is
// purely inferential, or when the extractor cited an external-only
// runtime artifact frame that was preserved in Rationale instead of
// being exposed as a repo citation.
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

// ForkForExploreDispatch creates an isolated MutableState for one
// concurrently running explorer dispatch. The fork inherits run-level
// read-only state and accumulated evidence context, but per-dispatch
// buffers, completion latches, tool-result history, and closure updates are
// written only to the fork until MergeExploreFork folds them back.
func (m *MutableState) ForkForExploreDispatch() *MutableState {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	out := &MutableState{
		objective:                           m.objective,
		repoRoot:                            m.repoRoot,
		result:                              m.result,
		resultIsPlain:                       m.resultIsPlain,
		finalAnswerMarkdownPath:             m.finalAnswerMarkdownPath,
		finalAnswerHTMLPath:                 m.finalAnswerHTMLPath,
		outputTranscriptRequest:             m.outputTranscriptRequest,
		searchGraph:                         m.searchGraph,
		symbolOracle:                        m.symbolOracle,
		logTriage:                           m.logTriage,
		perfTrace:                           m.perfTrace,
		multiRepoFocusDecision:              cloneMultiRepoFocusDecision(m.multiRepoFocusDecision),
		writeAnalysisIR:                     m.writeAnalysisIR,
		investigationComplete:               m.investigationComplete,
		investigationCompleteReason:         m.investigationCompleteReason,
		absenceJustification:                m.absenceJustification,
		investigationResultKind:             m.investigationResultKind,
		retainedAbsenceJustification:        m.retainedAbsenceJustification,
		retainedInvestigationResultKind:     m.retainedInvestigationResultKind,
		retainedInvestigationCompleteReason: m.retainedInvestigationCompleteReason,
		answerSurfaceRevision:               m.answerSurfaceRevision,
		traceQueryRuntimeObservationCount:   m.traceQueryRuntimeObservationCount,
		exploreForkTraceQueryRuntimeObservationBase: m.traceQueryRuntimeObservationCount,
	}
	if m.requestModel != nil {
		cp := *m.requestModel
		out.requestModel = &cp
	}
	out.emittedEvidence = append([]EvidenceItem(nil), m.emittedEvidence...)
	out.emittedAnswerSymbols = append([]AnswerSymbol(nil), m.emittedAnswerSymbols...)
	out.emittedAnswerSymbolCompleteness = m.emittedAnswerSymbolCompleteness
	out.emittedAnswerSymbolDeclaredCount = m.emittedAnswerSymbolDeclaredCount
	out.emittedHypothesisVerdicts = append([]HypothesisVerdict(nil), m.emittedHypothesisVerdicts...)
	out.sourceInventoryAdvisory = CloneSourceInventoryAdvisory(m.sourceInventoryAdvisory)
	out.sourceInventoryObservation = CloneSourceInventoryObservation(m.sourceInventoryObservation)
	out.sourceInventoryDiscoveryHinted = cloneStringBoolMap(m.sourceInventoryDiscoveryHinted)
	if m.writeContextPack != nil {
		pack := cloneWriteContextPack(*m.writeContextPack)
		out.writeContextPack = &pack
	}
	if m.writeWorkflowRun != nil {
		run := CloneWriteWorkflowRun(*m.writeWorkflowRun)
		out.writeWorkflowRun = &run
	}
	out.writeWorkflowDecision = append([]byte(nil), m.writeWorkflowDecision...)
	if m.turnAArtifacts != nil {
		out.turnAArtifacts = cloneTurnAArtifactsPtr(m.turnAArtifacts)
		out.exploreForkTurnABaseNotesLen = len(out.turnAArtifacts.InvestigationNotes)
		out.exploreForkTurnABaseBoundaryLen = len(out.turnAArtifacts.ValidationBoundaryNotes)
		out.exploreForkTurnABaseToolLen = len(out.turnAArtifacts.ToolResults)
		out.exploreForkTurnABaseMCPLen = len(out.turnAArtifacts.MCPResponses)
		out.exploreForkTurnABaseFlowLen = len(out.turnAArtifacts.FlowFindings)
		out.exploreForkTurnABaseHandoffLen = len(out.turnAArtifacts.HandoffCarriers)
	}
	out.phase1Ranking = append([]Phase1RankedFile(nil), m.phase1Ranking...)
	out.investigationAggregateFacts = cloneAnswerAggregateFacts(m.investigationAggregateFacts)
	out.retainedInvestigationAggregateFacts = cloneAnswerAggregateFacts(m.retainedInvestigationAggregateFacts)
	out.exactContextRequiredFiles = append([]string(nil), m.exactContextRequiredFiles...)
	if m.exploreBudget != nil {
		out.exploreBudget = m.exploreBudget.Clone()
	}
	closure := m.evidenceClosure
	m.mu.RUnlock()
	if closure != nil {
		out.evidenceClosure = closure.CloneForExploreDispatch()
	}
	return out
}

// MergeExploreFork folds the mutable side effects from a forked explorer
// dispatch back into the parent Run state. The scheduler calls this
// serially in DAG declaration order after parallel workers finish.
func (m *MutableState) MergeExploreFork(fork *MutableState) {
	if m == nil || fork == nil {
		return
	}
	fork.mu.RLock()
	emitted := append([]EvidenceItem(nil), fork.emittedEvidence...)
	turnA := cloneTurnAArtifactsPtr(fork.turnAArtifacts)
	turnABase := turnAArtifactsMergeBase{
		NotesLen:              fork.exploreForkTurnABaseNotesLen,
		ValidationBoundaryLen: fork.exploreForkTurnABaseBoundaryLen,
		ToolLen:               fork.exploreForkTurnABaseToolLen,
		MCPLen:                fork.exploreForkTurnABaseMCPLen,
		FlowLen:               fork.exploreForkTurnABaseFlowLen,
		HandoffLen:            fork.exploreForkTurnABaseHandoffLen,
	}
	phase1 := append([]Phase1RankedFile(nil), fork.phase1Ranking...)
	searchGraph := fork.searchGraph
	investigationComplete := fork.investigationComplete
	investigationCompleteReason := fork.investigationCompleteReason
	absenceJustification := fork.absenceJustification
	investigationResultKind := fork.investigationResultKind
	investigationAggregateFacts := cloneAnswerAggregateFacts(fork.investigationAggregateFacts)
	sourceInventoryAdvisory := CloneSourceInventoryAdvisory(fork.sourceInventoryAdvisory)
	sourceInventoryObservation := CloneSourceInventoryObservation(fork.sourceInventoryObservation)
	sourceInventoryDiscoveryHinted := cloneStringBoolMap(fork.sourceInventoryDiscoveryHinted)
	retainedAbsenceJustification := fork.retainedAbsenceJustification
	retainedInvestigationResultKind := fork.retainedInvestigationResultKind
	retainedInvestigationCompleteReason := fork.retainedInvestigationCompleteReason
	retainedInvestigationAggregateFacts := cloneAnswerAggregateFacts(fork.retainedInvestigationAggregateFacts)
	exactContextRequiredFiles := append([]string(nil), fork.exactContextRequiredFiles...)
	traceQueryRuntimeObservationDelta := fork.traceQueryRuntimeObservationCount - fork.exploreForkTraceQueryRuntimeObservationBase
	closure := fork.evidenceClosure
	fork.mu.RUnlock()

	m.mu.Lock()
	m.emittedEvidence = mergeEvidenceByStableID(m.emittedEvidence, emitted)
	if turnA != nil {
		merged := mergeTurnAArtifactsForMutable(m.turnAArtifacts, *turnA, turnABase)
		m.turnAArtifacts = cloneTurnAArtifactsPtr(&merged)
		m.cachedLabelSupport = nil
		m.cachedLabelSupportSource = nil
	}
	if len(phase1) > 0 {
		m.phase1Ranking = append([]Phase1RankedFile(nil), phase1...)
	}
	if m.searchGraph == nil && searchGraph != nil {
		m.searchGraph = searchGraph
	}
	if investigationComplete {
		mergedAggregateFacts := MergeAnswerAggregateFacts(m.investigationAggregateFacts, investigationAggregateFacts)
		m.investigationComplete = true
		m.investigationCompleteReason = investigationCompleteReason
		m.absenceJustification = absenceJustification
		m.investigationResultKind = investigationResultKind
		m.investigationAggregateFacts = mergedAggregateFacts
		if investigationCompleteReason != "" {
			m.retainedInvestigationCompleteReason = investigationCompleteReason
		}
		if investigationResultKind != "" {
			m.retainedInvestigationResultKind = investigationResultKind
		}
		if absenceJustification != "" {
			m.retainedAbsenceJustification = absenceJustification
		}
		if len(mergedAggregateFacts) > 0 {
			m.retainedInvestigationAggregateFacts = cloneAnswerAggregateFacts(mergedAggregateFacts)
		}
	}
	if retainedInvestigationCompleteReason != "" {
		m.retainedInvestigationCompleteReason = retainedInvestigationCompleteReason
	}
	if retainedInvestigationResultKind != "" {
		m.retainedInvestigationResultKind = retainedInvestigationResultKind
	}
	if retainedAbsenceJustification != "" {
		m.retainedAbsenceJustification = retainedAbsenceJustification
	}
	if len(retainedInvestigationAggregateFacts) > 0 {
		m.retainedInvestigationAggregateFacts = MergeAnswerAggregateFacts(m.retainedInvestigationAggregateFacts, retainedInvestigationAggregateFacts)
	}
	if sourceInventoryAdvisory.IsActive() {
		m.sourceInventoryAdvisory = MergeSourceInventoryAdvisory(m.sourceInventoryAdvisory, sourceInventoryAdvisory)
	}
	if sourceInventoryObservation.IsActive() {
		m.sourceInventoryObservation = MergeSourceInventoryObservation(m.sourceInventoryObservation, sourceInventoryObservation)
	}
	if len(sourceInventoryDiscoveryHinted) > 0 {
		if m.sourceInventoryDiscoveryHinted == nil {
			m.sourceInventoryDiscoveryHinted = map[string]bool{}
		}
		for key := range sourceInventoryDiscoveryHinted {
			m.sourceInventoryDiscoveryHinted[key] = true
		}
	}
	if len(exactContextRequiredFiles) > 0 {
		m.exactContextRequiredFiles = exactContextRequiredFiles
	}
	if traceQueryRuntimeObservationDelta > 0 {
		m.traceQueryRuntimeObservationCount += traceQueryRuntimeObservationDelta
	}
	m.bumpAnswerSurfaceRevisionLocked()
	m.mu.Unlock()

	if closure != nil {
		m.EvidenceClosure().IngestEvidenceReducerInput(EvidenceReducerInput{
			Class:       EvidenceReducerInputForkClosureDelta,
			ForkClosure: closure,
		}, m.repoRoot)
	}
	if investigationComplete {
		m.EvidenceClosure().ClearPendingReads()
		m.EvidenceClosure().ClearRepairs()
	}
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

// OutputTranscriptRequest returns the exact current-turn request that should
// be written into user-facing output artifacts. Empty means callers should
// fall back to Objective()/StripConversationPrefix. This value is presentation
// metadata and must not be fed into prompts or memory.
func (m *MutableState) OutputTranscriptRequest() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.outputTranscriptRequest
}

// SetOutputTranscriptRequest stores the exact current-turn request for
// markdown/html output artifacts. The orchestrator seeds this once at Run
// entry from the REPL setter so a reused REPL runner cannot leak an earlier
// turn's request into a later output file.
func (m *MutableState) SetOutputTranscriptRequest(s string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outputTranscriptRequest = strings.TrimSpace(s)
}

// MultiRepoFocusDecision returns the latest typed multi-repo focus
// recommendation/decision recorded for this Run. It is scope metadata,
// not evidence.
func (m *MutableState) MultiRepoFocusDecision() *MultiRepoFocusDecision {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneMultiRepoFocusDecision(m.multiRepoFocusDecision)
}

// SetMultiRepoFocusDecision records run-scoped active-set selection
// metadata. Tools/agents write it before the orchestrator routes the
// active set.
func (m *MutableState) SetMultiRepoFocusDecision(decision *MultiRepoFocusDecision) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.multiRepoFocusDecision = cloneMultiRepoFocusDecision(decision)
}

func cloneMultiRepoFocusDecision(in *MultiRepoFocusDecision) *MultiRepoFocusDecision {
	if in == nil {
		return nil
	}
	out := *in
	out.Candidates = append([]MultiRepoFocusCandidate(nil), in.Candidates...)
	return &out
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
	m.scopedSearchGraphs = nil
	m.searchGraphRevision++
}

// ScopedSearchGraph returns a run-local graph projection cached under key.
// The value is intentionally opaque so internal/types stays decoupled from
// internal/tool/repomap; callers must type-assert and clone in their own
// package before mutation.
func (m *MutableState) ScopedSearchGraph(key string) any {
	if m == nil || key == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.scopedSearchGraphs == nil {
		return nil
	}
	return m.scopedSearchGraphs[key]
}

// SetScopedSearchGraph stores a run-local graph projection. Passing nil clears
// the specific key. The cache is also cleared whenever SetSearchGraph installs
// a new base graph, which prevents scoped projections from outliving the graph
// they were derived from.
func (m *MutableState) SetScopedSearchGraph(key string, g any) {
	if m == nil || key == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if g == nil {
		delete(m.scopedSearchGraphs, key)
		return
	}
	if m.scopedSearchGraphs == nil {
		m.scopedSearchGraphs = map[string]any{}
	}
	m.scopedSearchGraphs[key] = g
}

// SymbolOracle returns the orchestrator-wired SymbolOracle for the
// current Run. nil when not yet set (pre-analyze, unit tests, or
// runs without a graph). Consumers in internal/tool use this to
// gate pre-emit hallucination checks without circular imports.
//
// 2026-05-10 P1.
func (m *MutableState) SymbolOracle() SymbolOracle {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.symbolOracle
}

// SetSymbolOracle stashes a SymbolOracle for downstream tool reads.
// Called once by the orchestrator after the AnalysisIR + search
// graph are wired. Pass nil to clear.
//
// 2026-05-10 P1.
func (m *MutableState) SetSymbolOracle(o SymbolOracle) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.symbolOracle = o
}

// LogTriage returns the validated LogBundle produced by the log_triage
// pre-stage, or nil when no log was attached, the stage was skipped,
// or the stage degraded. Readers MUST nil-check. Returns the stored
// pointer directly (no defensive copy): the bundle is treated as
// immutable after SetLogTriage, and both fields that readers mutate
// (none exist today) would be a contract violation. Keeping the
// pointer lets downstream consumers type-assert and cache without
// an allocation.
// AttachedArtifacts returns both artifact-lane bundles under ONE lock
// acquisition. Consumers building a joint log+trace view must use this
// instead of sequential LogTriage()/PerfTrace() calls: the two-step
// read has a window where a pre-stage retry or the perf two-step
// escalation rewrites one slot between the reads.
func (m *MutableState) AttachedArtifacts() (*LogBundle, *PerfBundle) {
	if m == nil {
		return nil, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.logTriage, m.perfTrace
}

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
	m.bumpAnswerSurfaceRevisionLocked()
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
	m.bumpAnswerSurfaceRevisionLocked()
}

// WriteAnalysisIR returns the validated WriteAnalysisIR produced by
// the write_analyzer pre-stage in write mode. Returns nil for
// read-mode Runs (the stage never fires) or when the LLM failed to
// emit and the validator could not synthesize a fallback. Readers
// MUST nil-check.
func (m *MutableState) WriteAnalysisIR() *WriteAnalysisIR {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.writeAnalysisIR
}

// SetWriteAnalysisIR stores the validated WriteAnalysisIR. Called at
// most once per write-mode Run from the write_analyze stage. The
// orchestrator clears it via SetWriteAnalysisIR(nil) at Run start so
// stale state from a prior task does not leak in.
func (m *MutableState) SetWriteAnalysisIR(ir *WriteAnalysisIR) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeAnalysisIR = ir
}

// WriteExplorationRequest returns the current read-only exploration request for
// a write workflow batch, if one has been queued. The returned value is a
// defensive copy so planner/explorer code cannot mutate shared state by
// accident.
func (m *MutableState) WriteExplorationRequest() *WriteExplorationRequest {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.writeExplorationRequest == nil {
		return nil
	}
	out := cloneWriteExplorationRequest(*m.writeExplorationRequest)
	return &out
}

// SetWriteExplorationRequest stores a normalized read-only exploration request
// for the next write workflow batch. Passing nil clears the request.
func (m *MutableState) SetWriteExplorationRequest(req *WriteExplorationRequest) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if req == nil {
		m.writeExplorationRequest = nil
		return
	}
	snap := NormalizeWriteExplorationRequest(cloneWriteExplorationRequest(*req))
	m.writeExplorationRequest = &snap
}

// ResetWriteExplorationRequest clears the queued write exploration request.
func (m *MutableState) ResetWriteExplorationRequest() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeExplorationRequest = nil
}

// WriteExplorationHandoff returns the compact planner-facing handoff produced
// by read-only exploration for a write workflow batch. The returned value is a
// defensive copy and is planning context only, not final-answer evidence.
func (m *MutableState) WriteExplorationHandoff() *WriteExplorationHandoff {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.writeExplorationHandoff == nil {
		return nil
	}
	out := cloneWriteExplorationHandoff(*m.writeExplorationHandoff)
	return &out
}

// SetWriteExplorationHandoff stores the normalized compact handoff from
// read-only exploration. Passing nil clears the handoff.
func (m *MutableState) SetWriteExplorationHandoff(handoff *WriteExplorationHandoff) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if handoff == nil {
		m.writeExplorationHandoff = nil
		return
	}
	snap := NormalizeWriteExplorationHandoff(cloneWriteExplorationHandoff(*handoff))
	m.writeExplorationHandoff = &snap
}

// ResetWriteExplorationHandoff clears the planner-facing exploration handoff.
func (m *MutableState) ResetWriteExplorationHandoff() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeExplorationHandoff = nil
}

// WriteContextPack returns the prioritized cross-stage handoff pack for the
// current write workflow batch. The returned value is a defensive copy.
func (m *MutableState) WriteContextPack() *WriteContextPack {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.writeContextPack == nil {
		return nil
	}
	out := cloneWriteContextPack(*m.writeContextPack)
	return &out
}

// SetWriteContextPack stores the normalized prioritized context pack. Passing
// nil clears the pack.
func (m *MutableState) SetWriteContextPack(pack *WriteContextPack) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if pack == nil {
		m.writeContextPack = nil
		return
	}
	snap := NormalizeWriteContextPack(cloneWriteContextPack(*pack))
	m.writeContextPack = &snap
}

// MergeWriteContextPack merges a new pack into the current batch pack.
func (m *MutableState) MergeWriteContextPack(pack WriteContextPack) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeContextPack == nil {
		snap := NormalizeWriteContextPack(cloneWriteContextPack(pack))
		m.writeContextPack = &snap
		return
	}
	merged := MergeWriteContextPacks(m.writeContextPack.BatchID, m.writeContextPack.Goal, *m.writeContextPack, pack)
	m.writeContextPack = &merged
}

// ResetWriteContextPack clears the prioritized cross-stage write handoff.
func (m *MutableState) ResetWriteContextPack() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeContextPack = nil
}

// WriteWorkflowRun returns the current outer workflow run snapshot, when the
// controller engine is active. The returned value is a defensive copy.
func (m *MutableState) WriteWorkflowRun() *WriteWorkflowRun {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.writeWorkflowRun == nil {
		return nil
	}
	out := CloneWriteWorkflowRun(*m.writeWorkflowRun)
	return &out
}

// SetWriteWorkflowRun stores the normalized outer workflow run snapshot.
// Passing nil clears the snapshot.
func (m *MutableState) SetWriteWorkflowRun(run *WriteWorkflowRun) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if run == nil {
		m.writeWorkflowRun = nil
		return
	}
	snap := CloneWriteWorkflowRun(*run)
	m.writeWorkflowRun = &snap
}

func (m *MutableState) ResetWriteWorkflowRun() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeWorkflowRun = nil
}

// WriteWorkflowDecisionJSON returns the last schema-normalized controller
// decision payload emitted by emit_write_workflow_decision. Consumers must
// unmarshal it into the typed writeflow.WriteWorkflowDecision before routing;
// model prose is intentionally not part of this channel.
func (m *MutableState) WriteWorkflowDecisionJSON() []byte {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]byte(nil), m.writeWorkflowDecision...)
}

// SetWriteWorkflowDecisionJSON stores a controller decision payload. Passing
// nil or an empty slice clears the decision. The setter does not validate the
// schema because the tool owns validation; it only preserves an immutable
// snapshot for downstream typed consumers.
func (m *MutableState) SetWriteWorkflowDecisionJSON(raw []byte) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(raw) == 0 {
		m.writeWorkflowDecision = nil
		return
	}
	m.writeWorkflowDecision = append([]byte(nil), raw...)
}

// ResetWriteWorkflowDecisionJSON clears the last controller decision.
func (m *MutableState) ResetWriteWorkflowDecisionJSON() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeWorkflowDecision = nil
}

// RecordUnvalidatedReason appends a reason describing a
// per-language static-check stage that was skipped because its
// toolchain was unavailable. Called by emit_change_plan dry-build
// helpers (Rust / Swift / Java / Kotlin etc.) when the relevant
// binary is missing. Drained into ChangePlan.UnvalidatedReasons by
// emit_change_plan.Execute after successful validation.
func (m *MutableState) RecordUnvalidatedReason(reason string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unvalidatedReasons = append(m.unvalidatedReasons, reason)
}

// DrainUnvalidatedReasons returns and clears the collected reasons.
// emit_change_plan.Execute calls this after validation succeeds
// and pipes the result into the finalised ChangePlan struct.
func (m *MutableState) DrainUnvalidatedReasons() []string {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := append([]string(nil), m.unvalidatedReasons...)
	m.unvalidatedReasons = nil
	return out
}

// TestAndSetPatchStyleNudge reports whether the legacy once-per-run patch
// line-structure marker may fire now (true exactly once), marking it consumed.
// emit_change_plan no longer rejects on this marker; patch style is rendered
// as an advisory on an otherwise successful tool result.
func (m *MutableState) TestAndSetPatchStyleNudge() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.patchStyleNudgeFired {
		return false
	}
	m.patchStyleNudgeFired = true
	return true
}

// PlanCritique returns the pre-apply review text produced by the
// plan_critic agent. Empty when the critic was disabled or
// produced no risks. /plan show renders this verbatim.
func (m *MutableState) PlanCritique() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.planCritique
}

// SetPlanCritique stores the critique text. Called at most once per
// Run from the plan stage hook after a successful plan_critic
// dispatch. Empty string is legal (the critic emitted zero risks).
func (m *MutableState) SetPlanCritique(text string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.planCritique = text
}

// SetTier2CompletenessHint stores an LLM-natural FixHint produced
// by a Phase 2 Tier 2 ERM completeness validator. Read by the
// finalize-stage prompt builder so the LLM sees the structural-
// coverage gap and can route around it (e.g., "use exec_command
// for the count" / "list every load-bearing intermediate
// function"). Empty string clears any prior hint.
func (m *MutableState) SetTier2CompletenessHint(hint string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tier2CompletenessHint = hint
}

// Tier2CompletenessHint returns the most-recently-stored Tier 2
// FixHint, or empty when none was set. Read by the finalize prompt
// builder.
func (m *MutableState) Tier2CompletenessHint() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tier2CompletenessHint
}

// Phase 2.B Tier 2 retry budgeting is delegated to the existing
// per-ViolationKind retry machinery (state.retryUsedForKind) — no
// dedicated counter on MutableState is needed. The post-finalize
// hard gate emits a typed Violation (ViolScalarCountUnsourced /
// ViolPathDepthInsufficient / ViolCardinalityShort /
// ViolEntityParityImbalanced) and the existing retry budget bounds
// the loop just like any other contract violation.

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
	r = AttachToolHandoffCarrier(r)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dispatchToolResults = append(m.dispatchToolResults, r)
	m.dispatchToolResultsRevision++
	traceRuntimeObservations := traceQueryRuntimeObservationToolResultCount(r)
	m.traceQueryRuntimeObservationCount += traceRuntimeObservations
	if traceRuntimeObservations > 0 {
		m.bumpAnswerSurfaceRevisionLocked()
	}
}

// TraceQueryRuntimeObservationCount returns how many hard-grounded runtime
// observations trace_query has published in the current explore/extract cycle.
// Unlike DispatchToolResults, this survives per-dispatch resets so synchronous
// pre-complete gates can consume the same deterministic trace-query signal that
// later handoff/finalizer stages receive through ToolResults/TurnAArtifacts.
func (m *MutableState) TraceQueryRuntimeObservationCount() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.traceQueryRuntimeObservationCount
}

func traceQueryRuntimeObservationToolResultCount(r ToolResult) int {
	if !r.Success || CanonicalToolName(r.ToolName) != "trace_query" {
		return 0
	}
	count := 0
	for _, observation := range r.Observations {
		if observation.Origin != AnswerEvidenceOriginRuntimeArtifact ||
			!runtimeObservationProducerIsDeterministicQuery(observation.Producer) ||
			observation.SourceRef.Kind != ObservationSourceRuntimeArtifact ||
			observation.GroundingPolicy != ClaimGroundingHard {
			continue
		}
		count++
	}
	return count
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
	m.dispatchToolResultsRevision++
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
// Clears the plain-text flag on the assumption a fresh write is the
// regular markdown answer; callers that store a plain-text fail-
// loud message must call SetResultPlain instead.
func (m *MutableState) SetResult(s string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.result = s
	m.resultIsPlain = false
	m.finalAnswerMarkdownPath = ""
	m.finalAnswerHTMLPath = ""
}

// SetResultPlain stores a result string AND marks it as plain text
// so the renderer skips glamour markdown rendering. Used by stage
// hooks (planPreHook / planPostHook / applyPreHook / etc.) when
// they surface a fail-loud diagnostic message that contains
// identifier-like tokens (e.g. "emit_change_plan") that chroma
// would otherwise split into ANSI-colored fragments. The user-
// visible message must read as a single uncolored line.
//
// Pre-2026-04-29 these messages went through SetResult and got
// glamour'd alongside real LLM answers. Production trace
// /home/chatpp/pytest 2026-04-29 06:03 showed the broken render:
// "emit_change_plan" became
// "[ANSI]emit_[/ANSI][ANSI]change_[/ANSI][ANSI]plan[/ANSI]" —
// chroma's tokenizer split on underscores. This separator is the
// fix.
func (m *MutableState) SetResultPlain(s string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.result = s
	m.resultIsPlain = true
	m.finalAnswerMarkdownPath = ""
	m.finalAnswerHTMLPath = ""
}

// ResultIsPlain reports whether the current result was stored via
// SetResultPlain (true) or SetResult (false). Renderer.RenderResult
// consults this to decide whether to glamour-render the result
// content; plain-text results pass through unchanged.
func (m *MutableState) ResultIsPlain() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.resultIsPlain
}

// SetFinalAnswerMarkdownPath stores the markdown transcript path for the
// current final answer. The path is presentation metadata, not answer content;
// renderers may surface it to users, but prompt builders should keep using
// Result() for the answer body.
func (m *MutableState) SetFinalAnswerMarkdownPath(path string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finalAnswerMarkdownPath = strings.TrimSpace(path)
	m.finalAnswerHTMLPath = ""
}

// SetFinalAnswerOutputPaths stores all presentation artifacts for the current
// answer in one lock acquisition. These paths are not answer content and must
// not be fed back into prompts.
func (m *MutableState) SetFinalAnswerOutputPaths(markdownPath, htmlPath string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finalAnswerMarkdownPath = strings.TrimSpace(markdownPath)
	m.finalAnswerHTMLPath = strings.TrimSpace(htmlPath)
}

// FinalAnswerMarkdownPath returns the markdown transcript path for the current
// final answer, or empty when output dumping was disabled or failed.
func (m *MutableState) FinalAnswerMarkdownPath() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.finalAnswerMarkdownPath
}

// FinalAnswerHTMLPath returns the self-contained HTML transcript path for the
// current final answer, or empty when output dumping was disabled or failed.
func (m *MutableState) FinalAnswerHTMLPath() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.finalAnswerHTMLPath
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
//
// AuthorityCeiling axis: items lacking Origin/Authority (the
// "bypass" paths — concrete_value extractor, mechanism_scan,
// bridge_literal merge — that build EvidenceItem literals directly
// instead of going through emit_evidence) are passed through the
// registered projector so they too participate in drift detection.
// Without this backfill, bypass items would be Authority="" which
// HighestAuthorityFor treats as factual-equivalent — letting a
// drift-bounded answer dodge hedging because a sibling deterministic
// value happened to be at the same anchor. The projector lives in
// internal/authority and is registered once at startup.
func (m *MutableState) AppendEvidence(items []EvidenceItem) {
	if m == nil || len(items) == 0 {
		return
	}
	items = normalizeEvidenceItemsForMutableStorage(items, m.RepoRoot())
	if proj := loadEvidenceProjector(); proj != nil {
		items = proj(items, m)
		items = normalizeEvidenceItemsForMutableStorage(items, m.RepoRoot())
	}
	m.mu.Lock()
	m.emittedEvidence = append(m.emittedEvidence, items...)
	if m.evidenceClosure == nil {
		m.evidenceClosure = NewEvidenceClosure(m.repoRoot)
	}
	closure := m.evidenceClosure
	m.bumpAnswerSurfaceRevisionLocked()
	m.mu.Unlock()
	if closure != nil {
		closure.IngestEvidenceReducerInput(EvidenceReducerInput{
			Class:         EvidenceReducerInputStageEvidenceSnapshot,
			EvidenceItems: items,
		}, m.repoRoot)
	}
}

func normalizeEvidenceItemsForMutableStorage(items []EvidenceItem, repoRoot string) []EvidenceItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]EvidenceItem, len(items))
	for i := range items {
		out[i] = cloneEvidenceItemForMutableStorage(items[i])
		if normalizeEvidenceItemPathsForMutableStorage(&out[i], repoRoot) {
			id := strings.TrimSpace(out[i].ID)
			if id == "" || strings.HasPrefix(id, "ev-") {
				out[i].ID = StableEvidenceID(out[i])
			}
		}
	}
	return out
}

func cloneEvidenceItemForMutableStorage(in EvidenceItem) EvidenceItem {
	out := in
	if in.CrossfileQuery != nil {
		clone := *in.CrossfileQuery
		clone.Files = append([]string(nil), in.CrossfileQuery.Files...)
		out.CrossfileQuery = &clone
	}
	if in.CrossfileAssertion != nil {
		clone := *in.CrossfileAssertion
		out.CrossfileAssertion = &clone
	}
	if in.NegativeQuery != nil {
		clone := *in.NegativeQuery
		out.NegativeQuery = &clone
	}
	out.DerivedFrom = append([]string(nil), in.DerivedFrom...)
	out.SurfaceTerms = append([]string(nil), in.SurfaceTerms...)
	return out
}

func normalizeEvidenceItemPathsForMutableStorage(item *EvidenceItem, repoRoot string) bool {
	if item == nil {
		return false
	}
	changed := false
	if source, ok := canonicalEvidenceStoragePath(item.Source, repoRoot); ok {
		item.Source = source
		changed = true
	}
	if item.CrossfileQuery != nil {
		for i, file := range item.CrossfileQuery.Files {
			if canon, ok := canonicalEvidenceStoragePath(file, repoRoot); ok {
				item.CrossfileQuery.Files[i] = canon
				changed = true
			}
		}
	}
	if item.NegativeQuery != nil {
		if file, ok := canonicalEvidenceStoragePath(item.NegativeQuery.File, repoRoot); ok {
			item.NegativeQuery.File = file
			changed = true
		}
	}
	return changed
}

func canonicalEvidenceStoragePath(raw, repoRoot string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "://") || strings.ContainsAny(raw, "\n\r\t") {
		return raw, false
	}
	if !strings.Contains(raw, "/") && !strings.Contains(raw, `\`) && !strings.Contains(raw, ".") {
		return raw, false
	}
	canon := strings.TrimSpace(canonpath.CanonicalRepoRelative(raw, repoRoot))
	if canon == "" || canon == raw {
		return raw, false
	}
	return canon, true
}

func (m *MutableState) answerSurfaceRevisionValue() uint64 {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.answerSurfaceRevision
}

func (m *MutableState) bumpAnswerSurfaceRevisionLocked() {
	if m == nil {
		return
	}
	m.answerSurfaceRevision++
}

// EvidenceProjector adapts an opaque (item-list, mutable-state) pair
// into a possibly-mutated item-list. Used by the AuthorityCeiling
// axis to backfill Origin/Authority on items that bypassed
// emit_evidence's hook.
type EvidenceProjector func(items []EvidenceItem, m *MutableState) []EvidenceItem

var (
	evidenceProjectorMu sync.RWMutex
	evidenceProjector   EvidenceProjector
)

// RegisterEvidenceProjector installs (or clears with nil) the
// projector. Callers MUST register before any goroutine calls
// AppendEvidence; in production cmd/root.go does this in initApp
// before any Run() dispatch fires.
func RegisterEvidenceProjector(p EvidenceProjector) {
	evidenceProjectorMu.Lock()
	defer evidenceProjectorMu.Unlock()
	evidenceProjector = p
}

func loadEvidenceProjector() EvidenceProjector {
	evidenceProjectorMu.RLock()
	defer evidenceProjectorMu.RUnlock()
	return evidenceProjector
}

// EmittedEvidence returns a compacted copy of the LLM-emitted evidence buffer.
// The raw buffer remains append-tailed so EmittedEvidenceSince can report
// mid-loop correction events, but full snapshots merge same-ID amendments before
// downstream stages see them.
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
	return mergeEvidenceByStableID(nil, out)
}

// EmittedEvidenceSince returns the raw append-tail of evidence events at or
// after start plus the current raw total length. It intentionally does not
// compact same-ID amendments: explorer mid-loop observers must see correction
// events even when the answer-grade EmittedEvidence snapshot collapses them.
func (m *MutableState) EmittedEvidenceSince(start int) ([]EvidenceItem, int) {
	if m == nil {
		return nil, 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	total := len(m.emittedEvidence)
	if total == 0 {
		return nil, 0
	}
	if start < 0 || start > total {
		start = 0
	}
	if start == total {
		return nil, total
	}
	out := make([]EvidenceItem, total-start)
	copy(out, m.emittedEvidence[start:])
	return out, total
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
	m.bumpAnswerSurfaceRevisionLocked()
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
	m.bumpAnswerSurfaceRevisionLocked()
}

// SetEmittedAnswerSymbolDeclaredCount stores the LLM's self-
// declared item count from emit_answer_symbol. Zero resets to "no
// claim". Called immediately after SetEmittedAnswerSymbols on the
// same emit invocation. Read by finalize-stage drift validators
// (orchestrator/contract_check.go ViolDeclaredCountDrift producer).
func (m *MutableState) SetEmittedAnswerSymbolDeclaredCount(n int) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if n < 0 {
		n = 0
	}
	m.emittedAnswerSymbolDeclaredCount = n
}

// EmittedAnswerSymbolDeclaredCount returns the buffered claim.
// Zero = no claim was made on the most recent emit (back-compat
// with pre-commit-49 callers and with current LLMs that omit
// the count field).
func (m *MutableState) EmittedAnswerSymbolDeclaredCount() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.emittedAnswerSymbolDeclaredCount
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
	m.bumpAnswerSurfaceRevisionLocked()
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

// AnswerDocAttemptShape is a numeric snapshot of an emit_answer_document
// payload's "size profile" — captured on EVERY emit attempt (success
// AND failure), and read by the catastrophic-regression detector to
// decide whether the next emit dropped substantial grounded content
// vs the previous attempt.
//
// The detector exists because pre-2026-04-30 traces showed retry
// rounds where the LLM's iter=2 reject named two fields ("Fix ONLY
// steps[2].description / .citation_ref") and iter=3 emit had
// summary="", steps missing entirely, citations missing — the
// model interpreted "Fix ONLY" as "emit only those fields" and
// dropped everything else. This struct lets the tool flag the
// regression on the next rejection so the retry hint can prepend a
// "PASTE THE FULL PRIOR PAYLOAD BACK byte-identical" override
// before the field-specific correction.
type AnswerDocAttemptShape struct {
	CitationsCount     int
	StepsCount         int
	SymbolsCount       int
	SummaryRunes       int
	HasValue           bool
	HasBoolean         bool
	HasExactResolution bool
}

// FinalizerNoToolAnswerDraft is a raw assistant response captured when the
// finalizer wrote either an answer_document-shaped payload or a rich visible
// table/diagram/list draft in text instead of using the tool channel. The
// content is intentionally raw so recovery can reuse the same parser/normalizer
// as the ordinary text-recovery path, with visible-surface fallback as a
// display-only backup.
type FinalizerNoToolAnswerDraft struct {
	Content    string
	CapturedAt time.Time
}

const maxFinalizerNoToolAnswerDrafts = 4

// SetLastAnswerDocAttemptShape stores the size profile of the most
// recent emit ATTEMPT (success or failure) so the next attempt can
// be compared against it. Nil clears the field.
func (m *MutableState) SetLastAnswerDocAttemptShape(shape *AnswerDocAttemptShape) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if shape == nil {
		m.lastAnswerDocAttemptShape = nil
		return
	}
	clone := *shape
	m.lastAnswerDocAttemptShape = &clone
}

// LastAnswerDocAttemptShape returns a defensive copy of the most
// recent emit attempt's size profile, or nil when no attempt has
// run yet on this MutableState (first emit).
func (m *MutableState) LastAnswerDocAttemptShape() *AnswerDocAttemptShape {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.lastAnswerDocAttemptShape == nil {
		return nil
	}
	clone := *m.lastAnswerDocAttemptShape
	return &clone
}

// AppendFinalizerNoToolAnswerDraft records one finalizer assistant draft for
// same-task recovery. Callers decide whether the draft is structurally worth
// preserving. This method deduplicates exact content and keeps a bounded window
// that preserves the first draft plus the most recent drafts, because the first
// rich no-tool answer is often the only complete one after later fallback turns.
func (m *MutableState) AppendFinalizerNoToolAnswerDraft(content string) bool {
	if m == nil {
		return false
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, draft := range m.finalizerNoToolAnswerDrafts {
		if draft.Content == content {
			return false
		}
	}
	m.finalizerNoToolAnswerDrafts = append(m.finalizerNoToolAnswerDrafts, FinalizerNoToolAnswerDraft{
		Content:    content,
		CapturedAt: time.Now(),
	})
	if len(m.finalizerNoToolAnswerDrafts) > maxFinalizerNoToolAnswerDrafts {
		first := m.finalizerNoToolAnswerDrafts[0]
		tail := append([]FinalizerNoToolAnswerDraft(nil), m.finalizerNoToolAnswerDrafts[len(m.finalizerNoToolAnswerDrafts)-(maxFinalizerNoToolAnswerDrafts-1):]...)
		m.finalizerNoToolAnswerDrafts = append([]FinalizerNoToolAnswerDraft{first}, tail...)
	}
	return true
}

// FinalizerNoToolAnswerDrafts returns defensive copies of the same-task
// no-tool drafts captured for answer_document text recovery.
func (m *MutableState) FinalizerNoToolAnswerDrafts() []FinalizerNoToolAnswerDraft {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.finalizerNoToolAnswerDrafts) == 0 {
		return nil
	}
	out := make([]FinalizerNoToolAnswerDraft, len(m.finalizerNoToolAnswerDrafts))
	copy(out, m.finalizerNoToolAnswerDrafts)
	return out
}

// ResetFinalizerNoToolAnswerDrafts clears same-task no-tool finalizer drafts.
func (m *MutableState) ResetFinalizerNoToolAnswerDrafts() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finalizerNoToolAnswerDrafts = nil
}

// SetAnswerDocumentV2WithMutation atomically replaces the V2 block-
// only carrier and flags the patch-lineage according to the typed
// MutationKind. v3 B4 (2026-05-04) — single canonical setter,
// replacing the pre-v3 split SetAnswerDocumentV2 (full emit) /
// SetAnswerDocumentV2FromPatch (patch emit) pair. The input is
// cloned through cloneAnswerDocumentV2 so later caller-side
// mutations cannot race readers.
//
// MutationPartial sets LastEmitFromPatch=true so retry summary can
// render "Previous Emit (inherited from patch)" lineage; any other
// kind (including MutationReplaceAll) clears the flag.
//
// Both emit_answer_document and emit_answer_document_patch route
// through ApplyAndPersistMutation in internal/tool, which calls
// this setter. Direct callers should NOT bypass that helper —
// merged-doc validation lives there.
func (m *MutableState) SetAnswerDocumentV2WithMutation(kind MutationKind, doc *AnswerDocumentV2) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.answerDocumentV2 = cloneAnswerDocumentV2(doc)
	m.lastEmitFromPatch = (kind == MutationPartial)
	m.lastRejectedAnswerDocumentV2 = nil
}

// SetAnswerDisplayAttachments replaces the current final-answer
// fallback attachment list. Attachments are deliberately separate
// from AnswerDocumentV2 so they can preserve model-authored visible
// content without weakening structured answer validation.
func (m *MutableState) SetAnswerDisplayAttachments(in []AnswerDisplayAttachment) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.answerDisplayAttachments = cloneAnswerDisplayAttachments(in)
}

// AnswerDisplayAttachments returns a defensive copy of recovered
// user-visible fallback fragments associated with the current answer.
func (m *MutableState) AnswerDisplayAttachments() []AnswerDisplayAttachment {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneAnswerDisplayAttachments(m.answerDisplayAttachments)
}

// SetLastRejectedAnswerDocumentV2 stores a defensive copy of a decoded but
// rejected V2 answer draft. The draft is not user-visible contract state; it is
// a local-model retry aid consumed only by compatibility paths that re-run the
// full validator before accepting anything.
func (m *MutableState) SetLastRejectedAnswerDocumentV2(doc *AnswerDocumentV2) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastRejectedAnswerDocumentV2 = cloneAnswerDocumentV2(doc)
}

// LastRejectedAnswerDocumentV2 returns the most recent decoded-but-rejected
// V2 answer draft, or nil when no such draft exists.
func (m *MutableState) LastRejectedAnswerDocumentV2() *AnswerDocumentV2 {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneAnswerDocumentV2(m.lastRejectedAnswerDocumentV2)
}

// ResetAnswerDisplayAttachments clears recovered fallback fragments.
func (m *MutableState) ResetAnswerDisplayAttachments() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.answerDisplayAttachments = nil
}

// LastEmitFromPatch reports whether the most recent V2 doc on this
// MutableState was sourced via emit_answer_document_patch (true) or
// fresh full emit (false). Returns false on nil receiver or when no
// emit has happened yet.
func (m *MutableState) LastEmitFromPatch() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastEmitFromPatch
}

// AnswerDocumentV2 returns a defensive deep copy of the buffered
// V2 carrier, or nil when no V2 document has been set on this
// MutableState. The returned pointer is independent of internal
// state.
func (m *MutableState) AnswerDocumentV2() *AnswerDocumentV2 {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneAnswerDocumentV2(m.answerDocumentV2)
}

// ResetAnswerDocumentV2 clears the V2 buffer. Used by tests + the
// orchestrator's per-task reset path. The general task-entry reset
// is handled by ResetAnswerDocument which clears BOTH carriers.
func (m *MutableState) ResetAnswerDocumentV2() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.answerDocumentV2 = nil
	m.answerDisplayAttachments = nil
	m.finalizerNoToolAnswerDrafts = nil
	m.lastRejectedAnswerDocumentV2 = nil
}

// ResetActiveAnswerDocumentV2ForFinalizeDispatch clears only the active
// accepted answer-document buffer before a fresh finalizer dispatch. It
// deliberately preserves the most recent rejected draft and recovered display
// attachments: those are same-turn recovery aids for transient stream failures,
// not accepted answer state. Per-task resets and fallback resets should keep
// using ResetAnswerDocumentV2 so stale rejected drafts cannot leak across
// unrelated tasks or deeper retry scopes.
func (m *MutableState) ResetActiveAnswerDocumentV2ForFinalizeDispatch() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.answerDocumentV2 = nil
	m.lastEmitFromPatch = false
}

// cloneAnswerDocumentV2 makes a defensive deep copy of an
// AnswerDocumentV2. Returns nil when the input is nil. Mirror of
// CloneAnswerDocument; the per-field cloning ensures slices /
// pointers are independent so readers cannot race writers.
func cloneAnswerDocumentV2(in *AnswerDocumentV2) *AnswerDocumentV2 {
	if in == nil {
		return nil
	}
	out := &AnswerDocumentV2{
		DocumentModel: in.DocumentModel,
	}
	if len(in.Blocks) > 0 {
		out.Blocks = make([]AnswerBlock, len(in.Blocks))
		for i, b := range in.Blocks {
			cloned := AnswerBlock{
				ID:                      b.ID,
				Kind:                    b.Kind,
				Title:                   b.Title,
				Text:                    b.Text,
				ErrorGranularityVerdict: b.ErrorGranularityVerdict,
				CurrentStatusVerdict:    b.CurrentStatusVerdict,
				ScopeDisclosure:         b.ScopeDisclosure,
				SurfaceRole:             b.SurfaceRole,
			}
			if len(b.Columns) > 0 {
				cloned.Columns = append([]string(nil), b.Columns...)
			}
			if len(b.Items) > 0 {
				cloned.Items = make([]AnswerBlockItem, len(b.Items))
				for j, it := range b.Items {
					cloned.Items[j] = AnswerBlockItem{
						ID:            it.ID,
						Label:         it.Label,
						Text:          it.Text,
						Cells:         append([]string(nil), it.Cells...),
						CandidateRole: it.CandidateRole,
						CitationRef:   it.CitationRef,
					}
				}
			}
			if b.Diagram != nil {
				diag := *b.Diagram
				cloned.Diagram = &diag
			}
			if len(b.ClaimUses) > 0 {
				cloned.ClaimUses = append([]RenderedClaimUse(nil), b.ClaimUses...)
			}
			if len(b.FacetIDs) > 0 {
				cloned.FacetIDs = append([]string(nil), b.FacetIDs...)
			}
			// G2-4 (post_v2_runtime_gap_remediation, 2026-05-04):
			// EdgeAnchors was previously omitted from this clone,
			// silently dropping the typed annotation field on every
			// SetAnswerDocumentV2 / AnswerDocumentV2() round-trip.
			// The G2 cross-path equivalence audit caught this.
			if len(b.EdgeAnchors) > 0 {
				cloned.EdgeAnchors = append([]DiagramEdgeAnchor(nil), b.EdgeAnchors...)
			}
			out.Blocks[i] = cloned
		}
	}
	if len(in.Citations) > 0 {
		out.Citations = append([]Citation(nil), in.Citations...)
	}
	if in.ExactResolution != nil {
		er := *in.ExactResolution
		out.ExactResolution = &er
	}
	if len(in.MissingRequestedRoles) > 0 {
		out.MissingRequestedRoles = append([]AnswerMissingRequestedRole(nil), in.MissingRequestedRoles...)
	}
	if len(in.Caveats) > 0 {
		out.Caveats = append([]string(nil), in.Caveats...)
	}
	if len(in.Snippets) > 0 {
		out.Snippets = append([]CodeSnippet(nil), in.Snippets...)
	}
	if len(in.ReadOwnerAnchors) > 0 {
		out.ReadOwnerAnchors = NormalizeOwnerAnchorView(OwnerAnchorView{Items: in.ReadOwnerAnchors}, 0).Items
	}
	out.ReadSourceLocalization = CloneSourceLocalizationReviewPtr(in.ReadSourceLocalization)
	if in.ReadNavigationCoverage != nil {
		coverage := NormalizeRepoMapNavigationCoverage(*in.ReadNavigationCoverage)
		out.ReadNavigationCoverage = &coverage
	}
	out.ReadLocalizerFollowup = CloneReadLocalizerFollowupPtr(in.ReadLocalizerFollowup)
	out.ReadReasoningGraph = CloneAnswerReasoningGraphSummaryPtr(in.ReadReasoningGraph)
	return out
}

func cloneAnswerDisplayAttachments(in []AnswerDisplayAttachment) []AnswerDisplayAttachment {
	if len(in) == 0 {
		return nil
	}
	out := make([]AnswerDisplayAttachment, len(in))
	copy(out, in)
	return out
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

// FallbackResetTarget enumerates the partial-reset depths the
// Block 3 selective upstream fallback uses. Each value names the
// shallowest stage state that must be cleared so the next pipeline
// dispatch from that stage starts clean. Deeper resets imply
// shallower resets (extract → also reset finalize; explore → also
// reset extract+finalize; etc).
//
// FallbackResetTargetFinalizer  — clear AnswerDocument; keep
//
//	EmittedAnswerSymbol + Evidence
//
// FallbackResetTargetExtract    — clear AnswerDocument +
//
//	EmittedAnswerSymbol; keep Evidence
//
// FallbackResetTargetExplore    — clear AnswerDocument +
//
//	EmittedAnswerSymbol; keep Evidence
//	+ ScannedSet + ReadSet (sunk cost),
//	caller is expected to repopulate
//	PendingReads if it wants the
//	explorer to read more
//
// FallbackResetTargetAnalyze    — currently unimplemented (analyzer
//
//	reset is fail-loud per the red
//	line); reserved for future expansion.
type FallbackResetTarget string

const (
	FallbackResetTargetFinalizer FallbackResetTarget = "finalizer"
	FallbackResetTargetExtract   FallbackResetTarget = "extract"
	FallbackResetTargetExplore   FallbackResetTarget = "explore"
	FallbackResetTargetAnalyze   FallbackResetTarget = "analyze"
)

// ResetForFallback (Block 3 architecture overhaul 2026-05-02)
// performs a partial Mutable reset for selective upstream fallback.
// The semantics:
//
//   - Finalizer-only fallback: clear AnswerDocument so the next
//     finalize re-emit starts from a clean slate. EmittedAnswerSymbol
//
//   - Evidence preserved (the next finalizer dispatch reads them).
//
//   - Extract fallback: also clear EmittedAnswerSymbol so the
//     extractor can re-build the slate from current evidence. Sub-
//     state survives.
//
//   - Explore fallback: also clear ChangePlan / PartialChangePlan
//     (write-mode) so the next explorer pass is not anchored to
//     prior plan state. Evidence + ScannedSet + ReadSet PRESERVE
//     because they are sunk costs; the LLM's previous reads are
//     real and re-reading them adds nothing. PendingReads cleared
//     so the next explorer dispatch is not coupled to prior repair
//     queue items.
//
//   - Analyze fallback: NO-OP. Re-classifying the user's request
//     is a fail-loud event handled by the orchestrator above this
//     layer; a partial Mutable reset cannot legitimately
//     re-derive the IR.
//
// Returns the names of fields cleared so the caller can log a
// human-readable trace of what was sacrificed. Safe with nil
// receiver.
func (m *MutableState) ResetForFallback(target FallbackResetTarget) []string {
	if m == nil {
		return nil
	}
	cleared := []string{}
	switch target {
	case FallbackResetTargetFinalizer:
		m.ResetAnswerDocumentV2()
		m.ResetRepairExecutionPlan()
		cleared = append(cleared, "AnswerDocumentV2", "RepairExecutionPlan")
	case FallbackResetTargetExtract:
		m.ResetAnswerDocumentV2()
		m.ResetEmittedAnswerSymbols()
		m.ResetRepairExecutionPlan()
		cleared = append(cleared, "AnswerDocumentV2", "EmittedAnswerSymbols", "RepairExecutionPlan")
	case FallbackResetTargetExplore:
		m.ResetAnswerDocumentV2()
		m.ResetEmittedAnswerSymbols()
		// ChangePlan only meaningful in write mode; ResetChangePlan
		// is nil-safe via guards above.
		m.ResetChangePlan()
		m.ResetRepairExecutionPlan()
		// PendingReads — clear via closure so the next explorer
		// dispatch's repair-queue starts empty. Evidence /
		// ScannedSet / ReadSet preserved (sunk cost).
		if closure := m.EvidenceClosure(); closure != nil {
			closure.ClearPendingReads()
		}
		cleared = append(cleared, "AnswerDocumentV2", "EmittedAnswerSymbols", "ChangePlan", "RepairExecutionPlan", "PendingReads")
	case FallbackResetTargetAnalyze:
		// Deliberate no-op — see red line. Caller is expected to
		// fail-loud at this depth.
		return nil
	}
	return cleared
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

// BestPlanReport returns the highest-passing (plan, report) pair
// observed across verify→plan retry iterations, or (nil, nil) when no
// iteration has finished yet. Read by clearForReplan to detect
// regression: when the latest iteration's score is lower than the
// best, clearForReplan restores the best pair instead of carrying
// the worse plan into the next retry.
func (m *MutableState) BestPlanReport() (*ChangePlan, *ChangeReport) {
	if m == nil {
		return nil, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bestPlan, m.bestReport
}

// SetBestPlanReport installs a (plan, report) pair as the new
// best-known-good across retry iterations. Caller is responsible for
// the score comparison (see ChangeReportScore); this slot is dumb
// storage with no internal score check, so a future retry policy
// could plug in a different selection rule (e.g., prefer plans with
// no regressions over plans with the highest raw pass count) without
// touching the storage layer.
func (m *MutableState) SetBestPlanReport(plan *ChangePlan, report *ChangeReport) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bestPlan = plan
	m.bestReport = report
}

// ResetBestPlanReport clears the best-known-good slot at per-task
// entry. Called by Run() before each plan→apply→verify cycle so a
// previous task's high-water mark cannot leak into a new task.
func (m *MutableState) ResetBestPlanReport() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bestPlan = nil
	m.bestReport = nil
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

// SetAnalyzerRetryHint installs the IR-field-level coherence detail
// the analyzer's next emit_analysis dispatch should see. Called by
// buildAnalysisIR when the QualityGate's coherence checks fire so
// the LLM gets concrete feedback ("R1.1 domain_divergence: TermGraph
// spans 3 domains [agent, orchestrator, finalizer] but only 1
// sub-topic emitted") rather than a generic "the gate rejected
// you" prompt. Mirror of the planner's PlanningHint channel; the
// analyzer reads + clears it in prependEmitRetryDirective on the
// next dispatch.
func (m *MutableState) SetAnalyzerRetryHint(hint string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.analyzerRetryHint = hint
}

// AnalyzerRetryHint returns the coherence retry detail, or "" when
// no hint is pending.
func (m *MutableState) AnalyzerRetryHint() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.analyzerRetryHint
}

// ResetAnalyzerRetryHint clears the coherence retry detail; called
// once the analyzer has consumed it.
func (m *MutableState) ResetAnalyzerRetryHint() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.analyzerRetryHint = ""
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

// AppendIteration records one row in the per-Run iteration ledger
// (Module C). Called by the orchestrator's clearForReplan after a
// verify→plan retry decision is made and BEFORE Mutable.ChangePlan /
// Mutable.ChangeReport are reset, so every record carries the verbatim
// data from a completed attempt — no system pre-classification.
//
// Defensive copy of ChangedFiles so a later mutation on the caller
// side cannot rewrite history. The slice is short (target paths,
// usually < 20 entries) so the copy is negligible.
func (m *MutableState) AppendIteration(rec IterationRecord) {
	if m == nil {
		return
	}
	if len(rec.ChangedFiles) > 0 {
		copied := make([]string, len(rec.ChangedFiles))
		copy(copied, rec.ChangedFiles)
		rec.ChangedFiles = copied
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.iterationLedger = append(m.iterationLedger, rec)
}

// IterationLedger returns a copy of the per-Run iteration ledger
// in append order (oldest attempt first). The copy guarantees the
// caller cannot mutate the orchestrator's stored state by mutating
// the returned slice. Empty slice when no retry has fired yet.
func (m *MutableState) IterationLedger() []IterationRecord {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.iterationLedger) == 0 {
		return nil
	}
	out := make([]IterationRecord, len(m.iterationLedger))
	copy(out, m.iterationLedger)
	return out
}

// ResetIterationLedger clears the ledger. Called at the start of a
// fresh write Run so prior-Run history doesn't leak. NOT called by
// clearForReplan — that path APPENDS to the ledger between attempts;
// only a Run boundary resets it.
func (m *MutableState) ResetIterationLedger() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.iterationLedger = nil
}

// AnswerRetryEvent is one entry in the per-Run read-mode retry log.
// Stage names the pipeline stage that fired the retry ("analyze" /
// "explore" / "extract" / "finalize"); Reason is a verbatim short
// note (the retry hint or contract-rejection summary). Read by the
// answer_reviewer at end-of-Run to optionally distill an answer
// pitfall.
type AnswerRetryEvent struct {
	Stage  string `json:"stage"`
	Reason string `json:"reason"`
}

// AppendAnswerRetryEvent records one read-mode retry event. Called
// by the orchestrator's per-stage retry sites (runAnalyzePhase,
// runReadSchedulerLoop on contract failure, etc) so the end-of-Run
// answer_reviewer dispatch sees the full pattern of what the
// pipeline had to work through. Reason is trimmed; an empty Reason
// is recorded as "" so the count of events is preserved as a
// "this Run had retries" signal even when the reason text is
// missing.
func (m *MutableState) AppendAnswerRetryEvent(stage, reason string) {
	if m == nil {
		return
	}
	stage = strings.TrimSpace(stage)
	m.mu.Lock()
	m.answerRetryEvents = append(m.answerRetryEvents, AnswerRetryEvent{
		Stage:  stage,
		Reason: strings.TrimSpace(reason),
	})
	m.mu.Unlock()
	// Block 1 (architecture overhaul 2026-05-02) — mirror every
	// retry event into the closure's stage-wise stats so the
	// StageHealthSnapshot exposes the retry count without callers
	// having to re-walk answerRetryEvents themselves. Use
	// EvidenceClosure() (lazy-init aware) instead of reading the
	// raw field, so a test path that calls AppendAnswerRetryEvent
	// before any closure access still ticks the per-stage retry.
	if stage != "" {
		if closure := m.EvidenceClosure(); closure != nil {
			closure.IncrementStageRetry(stage)
		}
	}
}

// AnswerRetryEvents returns a copy of the per-Run read-mode retry
// log (oldest first). Empty slice when no retries fired.
func (m *MutableState) AnswerRetryEvents() []AnswerRetryEvent {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.answerRetryEvents) == 0 {
		return nil
	}
	out := make([]AnswerRetryEvent, len(m.answerRetryEvents))
	copy(out, m.answerRetryEvents)
	return out
}

// ResetAnswerRetryEvents clears the read-mode retry log. Called at
// the start of a fresh Run (orchestrator defensive reset) so prior-
// Run history doesn't leak. Not called between retries within the
// same Run.
func (m *MutableState) ResetAnswerRetryEvents() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.answerRetryEvents = nil
}

// LearningFailure records a single non-fatal failure in the cross-
// Run learning chain (reflector / answer_reviewer / taxonomy
// append). Stage names the LLM role; Reason is the err.Error()
// text. Read at Run end by the orchestrator to log a summary so
// operators notice if 80% of Runs have broken learning.
type LearningFailure struct {
	Stage  string `json:"stage"`
	Reason string `json:"reason"`
}

// AppendLearningFailure records one failure. Called from
// runAnswerReviewerOnSuccess and clearForReplan reflector dispatch
// when the LLM call errors / no tool call returned / append fails.
// Concurrency-safe; the per-Run slice grows append-only.
func (m *MutableState) AppendLearningFailure(stage, reason string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.learningFailures = append(m.learningFailures, LearningFailure{
		Stage:  strings.TrimSpace(stage),
		Reason: strings.TrimSpace(reason),
	})
}

// LearningFailures returns a snapshot of the per-Run failures.
// Empty = clean Run.
func (m *MutableState) LearningFailures() []LearningFailure {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.learningFailures) == 0 {
		return nil
	}
	out := make([]LearningFailure, len(m.learningFailures))
	copy(out, m.learningFailures)
	return out
}

// ResetLearningFailures clears the slice at Run start (defensive
// cross-Run reset).
func (m *MutableState) ResetLearningFailures() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.learningFailures = nil
}

// MidLoopHintExhaustion records one per-key mid-loop hint cap
// saturation event. HintKey is the loop-policy bucket the cap fired
// on (internal-vocab — for trace + cross-Run learning, not
// user-facing); Reason carries the cap counter for context.
//
// docs/design/post_phase2a_forensic_followups.md §2.1.G #4 + #7.
type MidLoopHintExhaustion struct {
	HintKey string `json:"hint_key"`
	Reason  string `json:"reason"`
}

// NoteMidLoopHintExhausted records that a per-key mid-loop hint cap
// saturated for the given key during this Run. Idempotent per key —
// the loop-policy already de-duplicates the cap-saturation signal,
// but defending here keeps the record set stable if a future caller
// fires twice. Concurrency-safe; appends to the per-Run slice.
//
// docs/design/post_phase2a_forensic_followups.md §2.1.G #4 + #7.
func (m *MutableState) NoteMidLoopHintExhausted(hintKey, reason string) {
	if m == nil {
		return
	}
	hintKey = strings.TrimSpace(hintKey)
	if hintKey == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.midLoopHintExhaustions {
		if e.HintKey == hintKey {
			return
		}
	}
	m.midLoopHintExhaustions = append(m.midLoopHintExhaustions, MidLoopHintExhaustion{
		HintKey: hintKey,
		Reason:  strings.TrimSpace(reason),
	})
}

// MidLoopHintExhaustions returns a snapshot of the per-Run mid-loop
// hint cap saturations. Empty = no cap fired during this Run.
// Success-criteria expressions / the orchestrator's fallback gate
// can read this to detect "model is structurally unable to satisfy
// some gate" and pivot earlier instead of waiting for the
// dispatch's max-step quota.
//
// docs/design/post_phase2a_forensic_followups.md §2.1.G #4 + #7.
func (m *MutableState) MidLoopHintExhaustions() []MidLoopHintExhaustion {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.midLoopHintExhaustions) == 0 {
		return nil
	}
	out := make([]MidLoopHintExhaustion, len(m.midLoopHintExhaustions))
	copy(out, m.midLoopHintExhaustions)
	return out
}

// AppendPlanStageProbeReport records one plan-stage dry-run probe
// (Module E). Called by run_tests when invoked with dry_run=true in
// plan stage. Stored separately from Mutable.changeReport so the
// verify→plan retry channel sees ONLY authoritative verify outcomes
// (preventing a plan-stage probe from being mistaken for a verify
// result that drives a retry decision).
func (m *MutableState) AppendPlanStageProbeReport(r *ChangeReport) {
	if m == nil || r == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.planStageProbeReports = append(m.planStageProbeReports, r)
}

// PlanStageProbeReports returns a copy of the per-Run probe slice
// (oldest-first). Empty when no probe has fired. The planner can
// read these from its dispatch context to inform its plan emission.
func (m *MutableState) PlanStageProbeReports() []*ChangeReport {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.planStageProbeReports) == 0 {
		return nil
	}
	out := make([]*ChangeReport, len(m.planStageProbeReports))
	copy(out, m.planStageProbeReports)
	return out
}

// SetVerifyFailureHandoff installs the typed replan carrier for the latest
// failed post-apply verification. The scheduler owns the lifecycle: replace
// on each failure, clear on green verify or run completion.
func (m *MutableState) SetVerifyFailureHandoff(h *VerifyFailureHandoff) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.verifyFailureHandoff = h
}

// VerifyFailureHandoff returns the current typed replan carrier, or nil.
func (m *MutableState) VerifyFailureHandoff() *VerifyFailureHandoff {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.verifyFailureHandoff
}

// ResetVerifyFailureHandoff clears the replan carrier.
func (m *MutableState) ResetVerifyFailureHandoff() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.verifyFailureHandoff = nil
}

// ResetPlanStageProbeReports clears the probe slice. Called at Run
// boundary alongside ResetIterationLedger.
func (m *MutableState) ResetPlanStageProbeReports() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.planStageProbeReports = nil
}

// SetSourceInventoryAdvisory stores the current graph-backed source inventory
// candidate artifact. The artifact is advisory context: it is not answer text
// and downstream validators still require normal grounded evidence, aggregate
// facts, or citations for visible claims.
func (m *MutableState) SetSourceInventoryAdvisory(a SourceInventoryAdvisory) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	priorActive := m.sourceInventoryAdvisory.IsActive()
	priorObservation := CloneSourceInventoryObservation(m.sourceInventoryObservation)
	m.sourceInventoryAdvisory = CloneSourceInventoryAdvisory(a)
	m.sourceInventoryObservation = SourceInventoryObservationFromAdvisory(m.sourceInventoryAdvisory)
	if priorObservation.IsActive() {
		m.sourceInventoryObservation = MergeSourceInventoryObservation(priorObservation, m.sourceInventoryObservation)
	}
	if m.sourceInventoryObservation.IsActive() {
		if m.evidenceClosure == nil {
			m.evidenceClosure = NewEvidenceClosure(m.repoRoot)
		}
		m.evidenceClosure.IngestEvidenceReducerInput(EvidenceReducerInput{
			Class:                      EvidenceReducerInputSourceInventoryObservation,
			SourceInventoryObservation: m.sourceInventoryObservation,
		}, m.repoRoot)
	}
	if !m.sourceInventoryAdvisory.IsActive() || !priorActive {
		m.sourceInventoryAdvisoryHinted = false
	}
	m.bumpAnswerSurfaceRevisionLocked()
}

// SourceInventoryAdvisory returns a defensive copy of the current advisory
// candidate artifact. A zero-value artifact means no typed, bounded source
// inventory carrier is available for this run.
func (m *MutableState) SourceInventoryAdvisory() SourceInventoryAdvisory {
	if m == nil {
		return SourceInventoryAdvisory{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return CloneSourceInventoryAdvisory(m.sourceInventoryAdvisory)
}

// SetSourceInventoryObservation stores the auditable source-inventory lens
// companion. Callers that already have a SourceInventoryAdvisory should usually
// use SetSourceInventoryAdvisory so both carriers stay in lockstep.
func (m *MutableState) SetSourceInventoryObservation(o SourceInventoryObservation) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sourceInventoryObservation = CloneSourceInventoryObservation(o)
	if m.sourceInventoryObservation.IsActive() {
		if m.evidenceClosure == nil {
			m.evidenceClosure = NewEvidenceClosure(m.repoRoot)
		}
		m.evidenceClosure.IngestEvidenceReducerInput(EvidenceReducerInput{
			Class:                      EvidenceReducerInputSourceInventoryObservation,
			SourceInventoryObservation: m.sourceInventoryObservation,
		}, m.repoRoot)
	}
	m.bumpAnswerSurfaceRevisionLocked()
}

// SourceInventoryObservation returns a defensive copy of the current auditable
// source-inventory lens artifact.
func (m *MutableState) SourceInventoryObservation() SourceInventoryObservation {
	if m == nil {
		return SourceInventoryObservation{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return CloneSourceInventoryObservation(m.sourceInventoryObservation)
}

// ClaimSourceInventoryAdvisoryHint records that the compact advisory checklist
// has already been attached to a model-visible tool result. It returns true
// exactly once per active advisory lifecycle so pre-explore advisories can still
// surface near the first discovery tool without spamming every repo_map/list_files
// observation.
func (m *MutableState) ClaimSourceInventoryAdvisoryHint() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.sourceInventoryAdvisory.IsActive() || m.sourceInventoryAdvisoryHinted {
		return false
	}
	m.sourceInventoryAdvisoryHinted = true
	return true
}

// ClaimSourceInventoryDiscoveryHint records that a broad-navigation discovery
// hint has already been surfaced for a structured tool/scope key. It is
// intentionally independent from ClaimSourceInventoryAdvisoryHint: discovery
// hints help the model find the repo-lens entrypoint, while advisory hints
// render an already-active source-inventory artifact.
func (m *MutableState) ClaimSourceInventoryDiscoveryHint(key string) bool {
	if m == nil {
		return false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sourceInventoryDiscoveryHinted == nil {
		m.sourceInventoryDiscoveryHinted = map[string]bool{}
	}
	if m.sourceInventoryDiscoveryHinted[key] {
		return false
	}
	m.sourceInventoryDiscoveryHinted[key] = true
	return true
}

// ClearSourceInventoryAdvisory clears the current advisory artifact so stale
// inventory candidates cannot leak across a fresh explore/extract cycle.
func (m *MutableState) ClearSourceInventoryAdvisory() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sourceInventoryAdvisory = SourceInventoryAdvisory{}
	m.sourceInventoryObservation = SourceInventoryObservation{}
	m.sourceInventoryAdvisoryHinted = false
	m.bumpAnswerSurfaceRevisionLocked()
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
	if a.ValidationBoundaryNotes != nil {
		snap.ValidationBoundaryNotes = append([]string(nil), a.ValidationBoundaryNotes...)
	}
	if a.ReadFiles != nil {
		snap.ReadFiles = append([]string(nil), a.ReadFiles...)
	}
	snap.SourceLocalization = CloneSourceLocalizationReviewPtr(a.SourceLocalization)
	if a.ToolResults != nil {
		snap.ToolResults = append([]ToolResult(nil), a.ToolResults...)
	}
	if a.HandoffCarriers != nil {
		snap.HandoffCarriers = append([]ToolHandoffCarrier(nil), a.HandoffCarriers...)
	}
	if a.MCPResponses != nil {
		snap.MCPResponses = append([]MCPResponse(nil), a.MCPResponses...)
	}
	if a.EvidenceItems != nil {
		snap.EvidenceItems = append([]EvidenceItem(nil), a.EvidenceItems...)
	}
	if a.FlowFindings != nil {
		snap.FlowFindings = append([]FlowFindingDigest(nil), a.FlowFindings...)
	}
	if a.AcceptedAggregateFacts != nil {
		snap.AcceptedAggregateFacts = cloneAnswerAggregateFacts(a.AcceptedAggregateFacts)
	}
	snap.SourceInventoryAdvisory = CloneSourceInventoryAdvisory(a.SourceInventoryAdvisory)
	snap.SourceInventoryObservation = CloneSourceInventoryObservation(a.SourceInventoryObservation)
	if !snap.SourceInventoryObservation.IsActive() && snap.SourceInventoryAdvisory.IsActive() {
		snap.SourceInventoryObservation = SourceInventoryObservationFromAdvisory(snap.SourceInventoryAdvisory)
	}
	snap.HandoffCarriers = ToolHandoffCarriersFromTurnAInputs(snap.ToolResults, snap.EvidenceItems, snap.HandoffCarriers)
	m.turnAArtifacts = &snap
	m.turnAArtifactsRevision++
	if snap.SourceInventoryObservation.IsActive() || len(snap.HandoffCarriers) > 0 || len(snap.EvidenceItems) > 0 {
		if m.evidenceClosure == nil {
			m.evidenceClosure = NewEvidenceClosure(m.repoRoot)
		}
		m.evidenceClosure.IngestEvidenceReducerInput(EvidenceReducerInput{
			Class:                      EvidenceReducerInputTurnAHandoffSnapshot,
			EvidenceItems:              snap.EvidenceItems,
			HandoffCarriers:            snap.HandoffCarriers,
			SourceInventoryObservation: snap.SourceInventoryObservation,
		}, m.repoRoot)
	}
	// Snapshot changed → invalidate the memoised label-support pool.
	m.cachedLabelSupport = nil
	m.cachedLabelSupportSource = nil
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
	if m.turnAArtifacts.ValidationBoundaryNotes != nil {
		out.ValidationBoundaryNotes = append([]string(nil), m.turnAArtifacts.ValidationBoundaryNotes...)
	}
	if m.turnAArtifacts.ReadFiles != nil {
		out.ReadFiles = append([]string(nil), m.turnAArtifacts.ReadFiles...)
	}
	out.SourceLocalization = CloneSourceLocalizationReviewPtr(m.turnAArtifacts.SourceLocalization)
	if m.turnAArtifacts.ToolResults != nil {
		out.ToolResults = append([]ToolResult(nil), m.turnAArtifacts.ToolResults...)
	}
	if m.turnAArtifacts.HandoffCarriers != nil {
		out.HandoffCarriers = append([]ToolHandoffCarrier(nil), m.turnAArtifacts.HandoffCarriers...)
	}
	if m.turnAArtifacts.MCPResponses != nil {
		out.MCPResponses = append([]MCPResponse(nil), m.turnAArtifacts.MCPResponses...)
	}
	if m.turnAArtifacts.EvidenceItems != nil {
		out.EvidenceItems = append([]EvidenceItem(nil), m.turnAArtifacts.EvidenceItems...)
	}
	if m.turnAArtifacts.FlowFindings != nil {
		out.FlowFindings = append([]FlowFindingDigest(nil), m.turnAArtifacts.FlowFindings...)
	}
	if m.turnAArtifacts.AcceptedAggregateFacts != nil {
		out.AcceptedAggregateFacts = cloneAnswerAggregateFacts(m.turnAArtifacts.AcceptedAggregateFacts)
	}
	out.SourceInventoryAdvisory = CloneSourceInventoryAdvisory(m.turnAArtifacts.SourceInventoryAdvisory)
	out.SourceInventoryObservation = CloneSourceInventoryObservation(m.turnAArtifacts.SourceInventoryObservation)
	if !out.SourceInventoryObservation.IsActive() && out.SourceInventoryAdvisory.IsActive() {
		out.SourceInventoryObservation = SourceInventoryObservationFromAdvisory(out.SourceInventoryAdvisory)
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
	m.turnAArtifactsRevision++
	m.exploreForkTurnABaseNotesLen = 0
	m.exploreForkTurnABaseBoundaryLen = 0
	m.exploreForkTurnABaseToolLen = 0
	m.exploreForkTurnABaseMCPLen = 0
	m.exploreForkTurnABaseFlowLen = 0
	m.exploreForkTurnABaseHandoffLen = 0
	m.traceQueryRuntimeObservationCount = 0
	m.exploreForkTraceQueryRuntimeObservationBase = 0
	m.sourceInventoryAdvisory = SourceInventoryAdvisory{}
	m.sourceInventoryObservation = SourceInventoryObservation{}
	if m.evidenceClosure != nil {
		m.evidenceClosure.ClearSourceInventoryObservation()
	}
	m.cachedLabelSupport = nil
	m.cachedLabelSupportSource = nil
}

func (m *MutableState) GroundingContextRevisions() (turnARevision, dispatchRevision, searchGraphRevision uint64) {
	if m == nil {
		return 0, 0, 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.turnAArtifactsRevision, m.dispatchToolResultsRevision, m.searchGraphRevision
}

func (m *MutableState) GroundingContextSnapshot() (turnA *TurnAArtifacts, dispatch []ToolResult, searchGraph any, turnARevision, dispatchRevision, searchGraphRevision uint64) {
	if m == nil {
		return nil, nil, nil, 0, 0, 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	turnA = cloneTurnAArtifactsPtr(m.turnAArtifacts)
	if len(m.dispatchToolResults) > 0 {
		dispatch = append([]ToolResult(nil), m.dispatchToolResults...)
	}
	return turnA, dispatch, m.searchGraph, m.turnAArtifactsRevision, m.dispatchToolResultsRevision, m.searchGraphRevision
}

func cloneTurnAArtifactsPtr(in *TurnAArtifacts) *TurnAArtifacts {
	if in == nil {
		return nil
	}
	out := *in
	out.InvestigationNotes = append([]string(nil), in.InvestigationNotes...)
	out.ValidationBoundaryNotes = append([]string(nil), in.ValidationBoundaryNotes...)
	out.ReadFiles = append([]string(nil), in.ReadFiles...)
	out.SourceLocalization = CloneSourceLocalizationReviewPtr(in.SourceLocalization)
	out.ToolResults = append([]ToolResult(nil), in.ToolResults...)
	out.HandoffCarriers = append([]ToolHandoffCarrier(nil), in.HandoffCarriers...)
	out.MCPResponses = append([]MCPResponse(nil), in.MCPResponses...)
	out.EvidenceItems = append([]EvidenceItem(nil), in.EvidenceItems...)
	out.FlowFindings = cloneFlowFindingDigests(in.FlowFindings)
	out.AcceptedAggregateFacts = cloneAnswerAggregateFacts(in.AcceptedAggregateFacts)
	out.SourceInventoryAdvisory = CloneSourceInventoryAdvisory(in.SourceInventoryAdvisory)
	out.SourceInventoryObservation = CloneSourceInventoryObservation(in.SourceInventoryObservation)
	if !out.SourceInventoryObservation.IsActive() && out.SourceInventoryAdvisory.IsActive() {
		out.SourceInventoryObservation = SourceInventoryObservationFromAdvisory(out.SourceInventoryAdvisory)
	}
	return &out
}

type turnAArtifactsMergeBase struct {
	NotesLen              int
	ValidationBoundaryLen int
	ToolLen               int
	MCPLen                int
	FlowLen               int
	HandoffLen            int
}

const (
	turnAArtifactsMutableToolResultsCountCap  = 320
	turnAArtifactsMutableToolResultsByteCap   = 2 << 20
	turnAArtifactsMutableMCPResponsesCountCap = 320
	turnAArtifactsMutableMCPResponsesByteCap  = 2 << 20
)

func mergeTurnAArtifactsForMutable(prior *TurnAArtifacts, current TurnAArtifacts, base turnAArtifactsMergeBase) TurnAArtifacts {
	if prior == nil {
		if cloned := cloneTurnAArtifactsPtr(&current); cloned != nil {
			return *cloned
		}
		return current
	}
	merged := *cloneTurnAArtifactsPtr(prior)
	if current.UserQuestion != "" {
		merged.UserQuestion = current.UserQuestion
	}
	merged.InvestigationNotes = append(
		append([]string(nil), prior.InvestigationNotes...),
		current.InvestigationNotes[clampMergeSliceBase(base.NotesLen, len(current.InvestigationNotes)):]...,
	)
	merged.ValidationBoundaryNotes = append(
		append([]string(nil), prior.ValidationBoundaryNotes...),
		current.ValidationBoundaryNotes[clampMergeSliceBase(base.ValidationBoundaryLen, len(current.ValidationBoundaryNotes)):]...,
	)
	merged.InvestigationNotes = PreserveSupersededClosureReasonNote(merged.InvestigationNotes, prior.AcceptedClosureReason, current.AcceptedClosureReason)
	merged.ReadFiles = mergeStringsForMutable(prior.ReadFiles, current.ReadFiles)
	merged.SourceLocalization = MergeSourceLocalizationReviews(prior.SourceLocalization, current.SourceLocalization)
	merged.ToolResults = append(
		append([]ToolResult(nil), prior.ToolResults...),
		current.ToolResults[clampMergeSliceBase(base.ToolLen, len(current.ToolResults)):]...,
	)
	merged.ToolResults = BoundTurnAToolResults(
		merged.ToolResults,
		turnAArtifactsMutableToolResultsCountCap,
		turnAArtifactsMutableToolResultsByteCap,
		PreserveSuccessfulToolResultWithPayload,
	)
	merged.HandoffCarriers = NormalizeToolHandoffCarriers(append(
		append([]ToolHandoffCarrier(nil), prior.HandoffCarriers...),
		current.HandoffCarriers[clampMergeSliceBase(base.HandoffLen, len(current.HandoffCarriers)):]...,
	))
	merged.MCPResponses = append(
		append([]MCPResponse(nil), prior.MCPResponses...),
		current.MCPResponses[clampMergeSliceBase(base.MCPLen, len(current.MCPResponses)):]...,
	)
	merged.MCPResponses = BoundTurnAMCPResponses(
		merged.MCPResponses,
		turnAArtifactsMutableMCPResponsesCountCap,
		turnAArtifactsMutableMCPResponsesByteCap,
		PreserveSuccessfulMCPResponseWithPayload,
	)
	if current.AcceptedClosureReason != "" {
		merged.AcceptedClosureReason = current.AcceptedClosureReason
	}
	if current.AcceptedResultKind != "" {
		merged.AcceptedResultKind = current.AcceptedResultKind
	}
	if len(current.AcceptedAggregateFacts) > 0 {
		merged.AcceptedAggregateFacts = cloneAnswerAggregateFacts(current.AcceptedAggregateFacts)
	}
	merged.SourceInventoryAdvisory = MergeSourceInventoryAdvisory(prior.SourceInventoryAdvisory, current.SourceInventoryAdvisory)
	merged.SourceInventoryObservation = MergeSourceInventoryObservation(prior.SourceInventoryObservation, current.SourceInventoryObservation)
	if !merged.SourceInventoryObservation.IsActive() && merged.SourceInventoryAdvisory.IsActive() {
		merged.SourceInventoryObservation = SourceInventoryObservationFromAdvisory(merged.SourceInventoryAdvisory)
	}
	merged.RuntimeObservationOnlyCompletion = prior.RuntimeObservationOnlyCompletion || current.RuntimeObservationOnlyCompletion
	merged.EvidenceItems = mergeEvidenceByStableID(prior.EvidenceItems, current.EvidenceItems)
	merged.HandoffCarriers = ToolHandoffCarriersFromTurnAInputs(merged.ToolResults, merged.EvidenceItems, merged.HandoffCarriers)
	flowBase := clampMergeSliceBase(base.FlowLen, len(current.FlowFindings))
	merged.FlowFindings = mergeFlowFindingsForMutable(prior.FlowFindings, current.FlowFindings[flowBase:])
	if prior.TerminalEvidenceCount > merged.TerminalEvidenceCount {
		merged.TerminalEvidenceCount = prior.TerminalEvidenceCount
	}
	if current.TerminalEvidenceCount > merged.TerminalEvidenceCount {
		merged.TerminalEvidenceCount = current.TerminalEvidenceCount
	}
	return merged
}

// PreserveSupersededClosureReasonNote keeps rich model-authored closure prose
// visible as advisory context when a later accepted closure replaces the
// authoritative closure reason. The later reason remains authoritative; the
// prior prose is retained only as an investigation note so downstream agents can
// reuse useful explanation without treating it as a citation or validator fact.
func PreserveSupersededClosureReasonNote(notes []string, priorReason, currentReason string) []string {
	priorReason = strings.TrimSpace(priorReason)
	currentReason = strings.TrimSpace(currentReason)
	if priorReason == "" || currentReason == "" || strings.EqualFold(priorReason, currentReason) {
		return notes
	}
	for _, note := range notes {
		if strings.Contains(note, priorReason) {
			return notes
		}
	}
	const prefix = "Previous accepted closure reason (preserved advisory, not a citation): "
	return append(notes, prefix+priorReason)
}

func clampMergeSliceBase(base, n int) int {
	if base < 0 {
		return 0
	}
	if base > n {
		return n
	}
	return base
}

func mergeEvidenceByStableID(existing, incoming []EvidenceItem) []EvidenceItem {
	out := append([]EvidenceItem(nil), existing...)
	seen := make(map[string]int, len(out)+len(incoming))
	seenRevision := make(map[string]int, len(out)+len(incoming))
	for i := range out {
		id := out[i].ID
		if id == "" {
			id = StableEvidenceID(out[i])
			out[i].ID = id
		}
		seen[EvidenceStableMergeKey(out[i])] = i
		if key := EvidenceRevisionKey(out[i]); key != "" {
			seenRevision[key] = i
		}
	}
	for _, item := range incoming {
		id := item.ID
		if id == "" {
			id = StableEvidenceID(item)
			item.ID = id
		}
		mergeKey := EvidenceStableMergeKey(item)
		if idx, ok := seen[mergeKey]; ok {
			out[idx] = mergeEvidenceByStableIDItem(out[idx], item)
			continue
		}
		if key := EvidenceRevisionKey(item); key != "" {
			if idx, ok := seenRevision[key]; ok {
				item.ID = out[idx].ID
				out[idx] = mergeEvidenceByStableIDItem(out[idx], item)
				continue
			}
		}
		seen[mergeKey] = len(out)
		if key := EvidenceRevisionKey(item); key != "" {
			seenRevision[key] = len(out)
		}
		out = append(out, item)
	}
	return out
}

func mergeEvidenceByStableIDItem(dst, src EvidenceItem) EvidenceItem {
	return MergeEvidenceItemByStableID(dst, src)
}

func mergeStringSetForMutable(dst, src []string) []string {
	return MergeEvidenceStringSet(dst, src)
}

func mergeFlowFindingsForMutable(existing, incoming []FlowFindingDigest) []FlowFindingDigest {
	if len(existing) == 0 {
		return cloneFlowFindingDigests(incoming)
	}
	out := cloneFlowFindingDigests(existing)
	seen := make(map[string]bool, len(out)+len(incoming))
	for _, f := range out {
		if f.ID != "" {
			seen[f.ID] = true
		}
	}
	for _, f := range incoming {
		if f.ID != "" && seen[f.ID] {
			continue
		}
		if f.ID != "" {
			seen[f.ID] = true
		}
		out = append(out, cloneFlowFindingDigest(f))
	}
	return out
}

func cloneFlowFindingDigests(in []FlowFindingDigest) []FlowFindingDigest {
	if len(in) == 0 {
		return nil
	}
	out := make([]FlowFindingDigest, len(in))
	for i, f := range in {
		out[i] = cloneFlowFindingDigest(f)
	}
	return out
}

func cloneFlowFindingDigest(in FlowFindingDigest) FlowFindingDigest {
	out := in
	out.Path = append([]string(nil), in.Path...)
	out.Conditions = append([]string(nil), in.Conditions...)
	out.Sources = append([]string(nil), in.Sources...)
	out.Sinks = append([]string(nil), in.Sinks...)
	out.Hops = append([]string(nil), in.Hops...)
	out.EvidenceIDs = append([]string(nil), in.EvidenceIDs...)
	return out
}

func mergeStringsForMutable(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range append(append([]string(nil), a...), b...) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func cloneStringBoolMap(in map[string]bool) map[string]bool {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]bool, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

// CachedLabelSupportTokens returns the dot-qualified selector / anchor /
// subject / object token pool drawn from the buffered Turn A
// EvidenceItems, memoised across calls until a Set / Reset of
// turnAArtifacts invalidates it. The `build` function is supplied by
// the caller so MutableState stays decoupled from
// internal/orchestrator (where the actual token-extraction logic
// lives).
//
// Returns nil when no Turn A snapshot is buffered or when build is nil.
// The returned map is shared — callers MUST treat it as read-only.
func (m *MutableState) CachedLabelSupportTokens(build func([]EvidenceItem) map[string]struct{}) map[string]struct{} {
	if m == nil || build == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.turnAArtifacts == nil {
		return nil
	}
	if m.cachedLabelSupportSource == m.turnAArtifacts && m.cachedLabelSupport != nil {
		return m.cachedLabelSupport
	}
	out := build(m.turnAArtifacts.EvidenceItems)
	m.cachedLabelSupport = out
	m.cachedLabelSupportSource = m.turnAArtifacts
	return out
}

// AppendPrescanSummary appends `summary` to the per-dispatch
// pre-scan summary blob, lowercased, followed by a newline
// separator. Called by `analyzerEvaluator.Observe` once per
// successful pre-scan tool result (repo_map / grep files_only=true
// / list_files). The blob is bounded in practice by the analyzer's
// pre-scan budget (`AnalysisLimits.MaxPrescanRounds`, default 3) ×
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
	m.prescanRoundCount++
}

// AppendPrescanIntentSummary adds non-content analyzer intent metadata to the
// same lowercased corpus used by emit_analysis validation, without spending a
// pre-scan round. This lets blocked deep-read paths help entity/path retention
// while preserving the evidence-lite boundary.
func (m *MutableState) AppendPrescanIntentSummary(summary string) {
	if m == nil || strings.TrimSpace(summary) == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prescanSummaryBlob.WriteString(strings.ToLower(summary))
	m.prescanSummaryBlob.WriteByte('\n')
}

// PrescanRoundCount returns the number of analyzer pre-scan rounds
// that have completed in the current analyze dispatch.
func (m *MutableState) PrescanRoundCount() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.prescanRoundCount
}

// SetPrescanRoundLimit records the active analyze-dispatch pre-scan
// budget so runtime validators can enforce must-emit hard gates
// against the same effective limit the analyzer prompt used.
func (m *MutableState) SetPrescanRoundLimit(limit int) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prescanRoundLimit = limit
}

// PrescanRoundLimit returns the active analyze-dispatch pre-scan
// budget recorded for the current dispatch.
func (m *MutableState) PrescanRoundLimit() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.prescanRoundLimit
}

// MarkPrescanReady records that the analyzer has gathered enough lightweight
// location/existence signal for classification and should emit the analysis
// result instead of widening pre-scan. It is per-dispatch analyzer state and is
// reset with ResetPrescanSummary.
func (m *MutableState) MarkPrescanReady() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prescanReady = true
}

// PrescanReady reports whether the current analyze dispatch should expose only
// the terminal emit tool because a bounded pre-scan already established enough
// routing signal.
func (m *MutableState) PrescanReady() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.prescanReady
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
	m.prescanRoundCount = 0
	m.prescanRoundLimit = 0
	m.prescanReady = false
	m.analyzerBlockedToolIntents = nil
}

// ── Session 11 C0' ClassificationGrep accessors ────────────────────

// ClassificationObs is one line-level grep match observation
// captured in analyzer Round 2. Pre-PR2 the buildAnalysisIR
// pipeline ran a reconcileFromObservations helper against this
// stream to refine AnalysisIR fields (answer_subject.kind,
// question_kind, entity_axes — and historically AnswerShape,
// retired with the V2 carrier). The helper itself is gone; the
// observation channel still flows so future axis-specific
// reconcilers can pick it up. The classification signal never
// leaks into TurnAArtifacts or EvidenceClosure.
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

// AppendRichnessTelemetry records one richness-telemetry signal
// (facet softening or family underrepresentation). Safe to call
// concurrently. Empty Kind is silently dropped. Identical signals
// are deduplicated by composite key (Kind + FacetID + FacetKind +
// Family + BucketCount + Reason) — the same softening rule firing
// from multiple call sites within the same Run records once.
func (m *MutableState) AppendRichnessTelemetry(sig RichnessTelemetrySignal) {
	if m == nil || sig.Kind == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.richnessTelemetry {
		if existing == sig {
			return
		}
	}
	m.richnessTelemetry = append(m.richnessTelemetry, sig)
}

// RichnessTelemetry returns a defensive copy of the recorded
// richness-telemetry signals. Empty slice means no silent softening
// or family-fit fallback has fired so far in this Run.
func (m *MutableState) RichnessTelemetry() []RichnessTelemetrySignal {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.richnessTelemetry) == 0 {
		return nil
	}
	out := make([]RichnessTelemetrySignal, len(m.richnessTelemetry))
	copy(out, m.richnessTelemetry)
	return out
}

// ResetRichnessTelemetry clears the channel — typically called at
// the start of a fresh Run by NewMutableState callers; not part of
// any per-dispatch reset cycle (the channel is per-Run, not per-
// dispatch).
func (m *MutableState) ResetRichnessTelemetry() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.richnessTelemetry = nil
}

// AppendAnalyzerDecision records one analyzer/extractor automatic-
// decision signal. Empty Kind silently drops. Identical signals
// are deduplicated by composite key so the same decision firing
// from a retry loop records once.
func (m *MutableState) AppendAnalyzerDecision(sig AnalyzerDecisionSignal) {
	if m == nil || sig.Kind == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.analyzerDecisions {
		if existing == sig {
			return
		}
	}
	m.analyzerDecisions = append(m.analyzerDecisions, sig)
}

// AnalyzerDecisions returns a defensive copy of recorded
// analyzer-decision signals.
func (m *MutableState) AnalyzerDecisions() []AnalyzerDecisionSignal {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.analyzerDecisions) == 0 {
		return nil
	}
	out := make([]AnalyzerDecisionSignal, len(m.analyzerDecisions))
	copy(out, m.analyzerDecisions)
	return out
}

// RecordAnalyzerBlockedToolIntent stores routing-safe metadata from a tool call
// rejected by the analyze-stage boundary. It deduplicates by tool/path/pattern/
// query so a model that repeats the same blocked read does not bloat state.
// Returns the number of distinct blocked intents now recorded.
func (m *MutableState) RecordAnalyzerBlockedToolIntent(intent AnalyzerBlockedToolIntent) int {
	if m == nil || strings.TrimSpace(intent.ToolName) == "" {
		return 0
	}
	intent.ToolName = strings.TrimSpace(intent.ToolName)
	intent.Path = strings.TrimSpace(intent.Path)
	intent.Pattern = strings.TrimSpace(intent.Pattern)
	intent.Query = strings.TrimSpace(intent.Query)
	if intent.TS.IsZero() {
		intent.TS = time.Now()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.analyzerBlockedToolIntents {
		if existing.ToolName == intent.ToolName &&
			existing.Path == intent.Path &&
			existing.Pattern == intent.Pattern &&
			existing.Query == intent.Query {
			return len(m.analyzerBlockedToolIntents)
		}
	}
	m.analyzerBlockedToolIntents = append(m.analyzerBlockedToolIntents, intent)
	return len(m.analyzerBlockedToolIntents)
}

// AnalyzerBlockedToolIntentCount reports whether the analyzer has tried to
// cross the classification-only boundary during this dispatch.
func (m *MutableState) AnalyzerBlockedToolIntentCount() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.analyzerBlockedToolIntents)
}

// AnalyzerBlockedToolIntents returns a defensive copy of blocked analyze-stage
// tool intents.
func (m *MutableState) AnalyzerBlockedToolIntents() []AnalyzerBlockedToolIntent {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.analyzerBlockedToolIntents) == 0 {
		return nil
	}
	out := make([]AnalyzerBlockedToolIntent, len(m.analyzerBlockedToolIntents))
	copy(out, m.analyzerBlockedToolIntents)
	return out
}

// SetRetryState installs the R14 typed retry-state surface. Called
// by the orchestrator after contract.Check failure when the
// scheduler decides to retry the finalizer. nil rs clears the
// state so a fresh dispatch starts with no retry signal.
func (m *MutableState) SetRetryState(rs *RetryState) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retryState = rs
}

// RetryState returns the current retry-state pointer (or nil when
// no retry is in progress). Read by the agent layer's render path.
// Returned pointer is the LIVE value — callers MUST NOT mutate
// without going through SetRetryState (the field is mutex-guarded).
// Read-only consumption (rendering) is safe under the RLock.
func (m *MutableState) RetryState() *RetryState {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.retryState
}

// ResetRetryState clears the retry-state surface. Called at fresh
// dispatch entry so a finalizer dispatch always starts with a clean
// slate even if a previous run left state behind.
//
// G1 (post_v2_runtime_gap_remediation, 2026-05-04): also clears the
// stashed RepairExecutionPlan — the two surfaces are paired
// (retry-state populator updates both together).
func (m *MutableState) ResetRetryState() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retryState = nil
	m.repairExecutionPlan = nil
}

// SetRepairExecutionPlan stashes the dispatch-ready owner queue
// produced by the retry-decision site. Carried as `any` so
// internal/types stays decoupled from internal/orchestrator;
// orchestrator-side callers type-assert on the receive side. Passing
// nil clears the slot.
//
// G1 step 2 (post_v2_runtime_gap_remediation, 2026-05-04). The
// retry-decision site MUST call this BEFORE state.requeue so the
// next dispatch's retry-decision pass observes the queue.
func (m *MutableState) SetRepairExecutionPlan(plan any) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.repairExecutionPlan = plan
}

// RepairExecutionPlan returns the most recently stashed plan (or nil
// when no retry-decision pass has run). Read-only consumption is
// safe under the RLock; mutation MUST go through SetRepairExecutionPlan.
func (m *MutableState) RepairExecutionPlan() any {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.repairExecutionPlan
}

// ResetRepairExecutionPlan clears the stashed plan in isolation.
// Called by ResetForFallback at every reset target — the queue is
// scoped to the current retry chain; once a fallback re-runs an
// upstream stage, the queue from the prior chain is no longer
// authoritative.
func (m *MutableState) ResetRepairExecutionPlan() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.repairExecutionPlan = nil
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
	reason = strings.TrimSpace(reason)
	m.investigationComplete = true
	m.investigationCompleteReason = reason
	if reason != "" {
		m.retainedInvestigationCompleteReason = reason
	}
	m.bumpAnswerSurfaceRevisionLocked()
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

// SetEvidenceFloorWaiver records a model-declared waiver from
// emit_investigation_complete. Called only by the tool layer after
// strict-decoding the typed payload (Reason validated against
// EvidenceFloorWaiverReasonValues, Rationale required non-empty).
// Idempotent — repeated calls overwrite. nil clears.
func (m *MutableState) SetEvidenceFloorWaiver(w *EvidenceFloorWaiver) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if w == nil {
		m.evidenceFloorWaiver = nil
		m.retainedEvidenceFloorWaiver = nil
		m.bumpAnswerSurfaceRevisionLocked()
		return
	}
	// Defensive copy so the caller's pointer cannot mutate stored state.
	clone := *w
	m.evidenceFloorWaiver = &clone
	m.bumpAnswerSurfaceRevisionLocked()
}

// EvidenceFloorWaiver returns the model-declared waiver, or nil
// when none has been set. The returned pointer is a fresh copy;
// mutating it does not affect stored state.
func (m *MutableState) EvidenceFloorWaiver() *EvidenceFloorWaiver {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.evidenceFloorWaiver == nil {
		return nil
	}
	clone := *m.evidenceFloorWaiver
	return &clone
}

// ClearEvidenceFloorWaiver clears both the current retry-local waiver
// and any retained successful waiver. Use this for an explicit model
// retraction; ResetInvestigationComplete intentionally does not call it.
func (m *MutableState) ClearEvidenceFloorWaiver() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.evidenceFloorWaiver = nil
	m.retainedEvidenceFloorWaiver = nil
	m.bumpAnswerSurfaceRevisionLocked()
}

// RetainEvidenceFloorWaiver promotes the current waiver into the stable
// answer-surface slot after emit_investigation_complete has passed every
// completion gate. If no current active waiver exists, the stable slot is
// cleared so a later normal completion can retract a prior waiver.
func (m *MutableState) RetainEvidenceFloorWaiver() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.evidenceFloorWaiver.IsActive() {
		m.retainedEvidenceFloorWaiver = nil
		m.bumpAnswerSurfaceRevisionLocked()
		return
	}
	clone := *m.evidenceFloorWaiver
	m.retainedEvidenceFloorWaiver = &clone
	m.bumpAnswerSurfaceRevisionLocked()
}

// StableEvidenceFloorWaiver returns the waiver from the most recently
// successful emit_investigation_complete, or nil when no successful
// completion retained one. The returned pointer is a defensive copy.
func (m *MutableState) StableEvidenceFloorWaiver() *EvidenceFloorWaiver {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.retainedEvidenceFloorWaiver.IsActive() {
		return nil
	}
	clone := *m.retainedEvidenceFloorWaiver
	return &clone
}

// SetPrincipalSpanWaiver records a model-declared escape for the
// callChainPrincipalSpanDowngrade gate. Called only by the tool
// layer after strict-decoding the typed payload (Reason validated
// against PrincipalSpanWaiverReasonValues, Rationale required
// non-empty). Idempotent — repeated calls overwrite. nil clears.
func (m *MutableState) SetPrincipalSpanWaiver(w *PrincipalSpanWaiver) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if w == nil {
		m.principalSpanWaiver = nil
		return
	}
	clone := *w
	m.principalSpanWaiver = &clone
}

// PrincipalSpanWaiver returns the model-declared escape, or nil when
// none has been set. The returned pointer is a defensive copy.
func (m *MutableState) PrincipalSpanWaiver() *PrincipalSpanWaiver {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.principalSpanWaiver == nil {
		return nil
	}
	clone := *m.principalSpanWaiver
	return &clone
}

// ClearPrincipalSpanWaiver retracts a previously declared escape. Use
// this when later investigation shows the principal span gate does
// apply (e.g. the model now believes intermediate evidence exists).
func (m *MutableState) ClearPrincipalSpanWaiver() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.principalSpanWaiver = nil
}

// StableInvestigationCompleteReason returns the best available
// accepted completion rationale across explore-window resets.
func (m *MutableState) StableInvestigationCompleteReason() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if reason := strings.TrimSpace(m.investigationCompleteReason); reason != "" {
		return reason
	}
	return strings.TrimSpace(m.retainedInvestigationCompleteReason)
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
	m.investigationResultKind = ""
	m.investigationAggregateFacts = nil
	m.exactContextRequiredFiles = nil
	m.bumpAnswerSurfaceRevisionLocked()
}

// SetExactContextRequiredFiles stores the repo-relative file set that
// must contribute at least one grounded production related-context
// anchor before an exact-absence answer may close with contextual
// guidance. The explorer refreshes this at the start of every explore
// dispatch from structurally-ranked candidates; downstream validators
// consume it deterministically instead of re-deriving the scope from
// free-form prose.
func (m *MutableState) SetExactContextRequiredFiles(files []string) {
	if m == nil {
		return
	}
	seen := make(map[string]bool)
	var norm []string
	for _, file := range files {
		file = strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`))
		file = strings.TrimPrefix(file, "./")
		if file == "" || seen[file] {
			continue
		}
		seen[file] = true
		norm = append(norm, file)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exactContextRequiredFiles = norm
	m.bumpAnswerSurfaceRevisionLocked()
}

// ExactContextRequiredFiles returns the structurally-ranked
// same-scope related-context files that an exact-absence closure may
// need to ground before completing. Empty means no additional
// related-context anchor is currently required.
func (m *MutableState) ExactContextRequiredFiles() []string {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.exactContextRequiredFiles) == 0 {
		return nil
	}
	out := make([]string, len(m.exactContextRequiredFiles))
	copy(out, m.exactContextRequiredFiles)
	return out
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
	just = strings.TrimSpace(just)
	m.absenceJustification = just
	if just != "" {
		m.retainedAbsenceJustification = just
	}
	m.bumpAnswerSurfaceRevisionLocked()
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

// SetInvestigationResultKind stores the structured terminal
// disposition emitted by emit_investigation_complete.
func (m *MutableState) SetInvestigationResultKind(kind string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	kind = strings.TrimSpace(kind)
	m.investigationResultKind = kind
	m.retainedInvestigationResultKind = kind
	if !strings.EqualFold(kind, "absence") {
		m.retainedAbsenceJustification = ""
	}
	m.bumpAnswerSurfaceRevisionLocked()
}

// SetInvestigationAggregateFacts stores the current model-emitted
// aggregate facts from emit_investigation_complete. The facts become
// stable answer-surface data only after RetainInvestigationAggregateFacts
// runs on a successful completion.
func (m *MutableState) SetInvestigationAggregateFacts(facts []AnswerAggregateFact) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.investigationAggregateFacts = cloneAnswerAggregateFacts(facts)
	m.bumpAnswerSurfaceRevisionLocked()
}

// RetainInvestigationAggregateFacts promotes the current aggregate
// facts after emit_investigation_complete has passed every completion
// gate. The completion tool is responsible for carrying forward any
// already-accepted aggregate handoff when a later repair/reconcile
// window closes without re-emitting it; this method only snapshots the
// current post-gate value.
func (m *MutableState) RetainInvestigationAggregateFacts() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retainedInvestigationAggregateFacts = cloneAnswerAggregateFacts(m.investigationAggregateFacts)
	m.bumpAnswerSurfaceRevisionLocked()
}

// StableInvestigationAggregateFacts returns the accepted aggregate
// handoff for downstream extraction/finalization. It never exposes a
// downgraded completion's current facts unless the investigation flag
// is actually set.
func (m *MutableState) StableInvestigationAggregateFacts() []AnswerAggregateFact {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.investigationComplete && len(m.investigationAggregateFacts) > 0 {
		return cloneAnswerAggregateFacts(m.investigationAggregateFacts)
	}
	return cloneAnswerAggregateFacts(m.retainedInvestigationAggregateFacts)
}

// InvestigationResultKind returns the structured completion
// disposition for the current investigation window.
func (m *MutableState) InvestigationResultKind() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.investigationResultKind
}

// StableAbsenceJustification returns the best available accepted
// absence justification for downstream contract checks. It prefers the
// current explore window's declarative state, then falls back to the
// most recently accepted terminal investigation result that survived
// window resets. Non-absence retained states return "" fail-closed.
func (m *MutableState) StableAbsenceJustification() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if strings.EqualFold(strings.TrimSpace(m.investigationResultKind), "absence") {
		if just := strings.TrimSpace(m.absenceJustification); just != "" {
			return just
		}
	}
	if !strings.EqualFold(strings.TrimSpace(m.retainedInvestigationResultKind), "absence") {
		return ""
	}
	return strings.TrimSpace(m.retainedAbsenceJustification)
}

// StableInvestigationResultKind returns the accepted terminal
// investigation disposition that downstream stages should trust across
// explore-window retries. It prefers the current window's explicit
// result_kind and falls back to the most recently accepted retained
// result when the current window has already been reset.
func (m *MutableState) StableInvestigationResultKind() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if kind := strings.TrimSpace(m.investigationResultKind); kind != "" {
		return kind
	}
	return strings.TrimSpace(m.retainedInvestigationResultKind)
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
type ToolRepairTarget struct {
	File   string `json:"file,omitempty"`
	Lines  []int  `json:"lines,omitempty"`
	Action string `json:"action,omitempty"`
}

type ToolRepair struct {
	Code     string             `json:"code,omitempty"`
	Hint     string             `json:"hint,omitempty"`
	Fields   []string           `json:"fields,omitempty"`
	Targets  []ToolRepairTarget `json:"targets,omitempty"`
	Metadata map[string]string  `json:"metadata,omitempty"`
}

// ToolRefinementHint is the typed, cross-stage advisory surface a tool may
// attach when its result is too broad, truncated, paginated, or intentionally
// skipped by a safety cap. Consumers may render or prioritize this hint, but
// hard gates must still consume the underlying typed tool result fields.
type ToolRefinementHint struct {
	ReasonCode               string            `json:"reason_code,omitempty"`
	ResultTruncated          bool              `json:"result_truncated,omitempty"`
	CandidateBudgetTruncated bool              `json:"candidate_budget_truncated,omitempty"`
	SkippedLargeCandidates   []string          `json:"skipped_large_candidates,omitempty"`
	UniverseExcludedReason   string            `json:"universe_excluded_reason,omitempty"`
	ExcludedRoots            []string          `json:"excluded_roots,omitempty"`
	NextCursor               string            `json:"next_cursor,omitempty"`
	TopSourceClasses         []SourcePathRole  `json:"top_source_classes,omitempty"`
	PreferredNextTool        string            `json:"preferred_next_tool,omitempty"`
	PreferredParams          map[string]string `json:"preferred_params,omitempty"`
	RequiredFields           []string          `json:"required_fields,omitempty"`
}

type ToolRuntimeTiming struct {
	Phase         string `json:"phase,omitempty"`
	ElapsedMillis int64  `json:"elapsed_millis,omitempty"`
	Status        string `json:"status,omitempty"`
	Count         int    `json:"count,omitempty"`
}

const (
	ToolPathDiscoveryKindGrep      = "grep"
	ToolPathDiscoveryKindListFiles = "list_files"
)

// ToolPathDiscovery is the typed call-shape/result carrier for tools that
// discover or prove file-path families. It records only schema parameters and
// tool-computed outcomes; rendered summaries remain user-visible prose and
// must not be parsed by hard gates to recover these fields.
type ToolPathDiscovery struct {
	Kind             string `json:"kind,omitempty"`
	Path             string `json:"path,omitempty"`
	Recursive        bool   `json:"recursive,omitempty"`
	Include          string `json:"include,omitempty"`
	FileType         string `json:"file_type,omitempty"`
	FilesOnly        bool   `json:"files_only,omitempty"`
	IncludeAuxiliary bool   `json:"include_auxiliary,omitempty"`
	NoMatches        bool   `json:"no_matches,omitempty"`
	ResultCount      int    `json:"result_count,omitempty"`
}

type ToolResult struct {
	ToolName      string              `json:"tool_name"`
	Summary       string              `json:"summary"`
	Repair        *ToolRepair         `json:"repair,omitempty"`
	Refinement    *ToolRefinementHint `json:"refinement,omitempty"`
	Handoff       *ToolHandoffCarrier `json:"handoff,omitempty"`
	RawRef        string              `json:"raw_ref,omitempty"`
	PathDiscovery *ToolPathDiscovery  `json:"path_discovery,omitempty"`

	// Observations are optional producer-published typed observation rows for
	// this tool result — the ToolResult companion to MCPResponse.Observations.
	// Tools that already compute a fully typed product (e.g. trace_query's
	// ranked root causes / evidence-pack facts) attach the rows here so the
	// observation ledger can consume them directly instead of re-parsing the
	// prose Summary, whose per-section caps and blob-preview clipping silently
	// drop facts. Rows ride the existing ToolResults channel (dispatch buffer,
	// TurnAArtifacts, bus history) and keep their producer-declared origin and
	// grounding classification; the ledger compiler never accepts a
	// current-source row from this field, so producer rows can never become
	// current-source citations. Empty for tools without a typed product — the
	// ledger's summary re-parse path remains the fallback for those results.
	Observations []ObservationRecord `json:"observations,omitempty"`

	// RuntimeTimings are optional typed tool-internal phase timings. They are
	// operational telemetry only: agent/reasoninggraph projection may surface
	// them for supportability, but answer and planning hard gates must not
	// interpret them as semantic evidence about the repository.
	RuntimeTimings []ToolRuntimeTiming `json:"runtime_timings,omitempty"`

	Success   bool      `json:"success"`
	Timestamp time.Time `json:"timestamp"`
}

// MCPResponse records a response from an MCP server.
type MCPResponse struct {
	ServerName   string                `json:"server_name"`
	Method       string                `json:"method"`
	Summary      string                `json:"summary"`
	RawRef       string                `json:"raw_ref,omitempty"`
	PayloadRef   string                `json:"payload_ref,omitempty"`
	RowSetRef    string                `json:"row_set_ref,omitempty"`
	PageRef      string                `json:"page_ref,omitempty"`
	ResourceURI  string                `json:"resource_uri,omitempty"`
	MIMEType     string                `json:"mime_type,omitempty"`
	JSONPointer  string                `json:"json_pointer,omitempty"`
	Selector     string                `json:"selector,omitempty"`
	Row          int                   `json:"row,omitempty"`
	LineStart    int                   `json:"line_start,omitempty"`
	LineEnd      int                   `json:"line_end,omitempty"`
	Observations []MCPTypedObservation `json:"observations,omitempty"`
	Success      bool                  `json:"success"`
	Timestamp    time.Time             `json:"timestamp"`
}

// MCPTypedObservation is an optional structured coordinate row decoded from an
// explicit MCP observation envelope. It is always treated as external MCP
// evidence, never as a current-source citation.
type MCPTypedObservation struct {
	Summary     string `json:"summary,omitempty"`
	RawRef      string `json:"raw_ref,omitempty"`
	PayloadRef  string `json:"payload_ref,omitempty"`
	RowSetRef   string `json:"row_set_ref,omitempty"`
	PageRef     string `json:"page_ref,omitempty"`
	ResourceURI string `json:"resource_uri,omitempty"`
	MIMEType    string `json:"mime_type,omitempty"`
	JSONPointer string `json:"json_pointer,omitempty"`
	Selector    string `json:"selector,omitempty"`
	Row         int    `json:"row,omitempty"`
	LineStart   int    `json:"line_start,omitempty"`
	LineEnd     int    `json:"line_end,omitempty"`
}

// MCPServerConfig is the codrax.yaml shape for one external MCP server.
// The current implementation supports stdio only; other transports are reserved and fail loudly.
type MCPServerConfig struct {
	Name             string            `yaml:"name" json:"name"`
	Transport        TransportType     `yaml:"transport" json:"transport"`
	Command          string            `yaml:"command" json:"command"`
	Args             []string          `yaml:"args" json:"args,omitempty"`
	Env              map[string]string `yaml:"env" json:"env,omitempty"`
	InheritEnv       *bool             `yaml:"inherit_env" json:"inherit_env,omitempty"`
	StartupTimeoutMS *int              `yaml:"startup_timeout_ms" json:"startup_timeout_ms,omitempty"`
	CallTimeoutMS    *int              `yaml:"call_timeout_ms" json:"call_timeout_ms,omitempty"`
	MaxResponseBytes *int              `yaml:"max_response_bytes" json:"max_response_bytes,omitempty"`

	// Optional operation-provider capability metadata. These fields do not
	// expose the MCP server to the operation route by default; the operator must
	// opt in with operation_provider=true. They are consumed only by the REPL
	// operation planner so ordinary explorer MCP behavior stays unchanged.
	OperationProvider             *bool    `yaml:"operation_provider" json:"operation_provider,omitempty"`
	OperationKinds                []string `yaml:"operation_kinds" json:"operation_kinds,omitempty"`
	OperationSurfaces             []string `yaml:"operation_surfaces" json:"operation_surfaces,omitempty"`
	OperationSideEffects          []string `yaml:"operation_side_effects" json:"operation_side_effects,omitempty"`
	OperationTool                 string   `yaml:"operation_tool" json:"operation_tool,omitempty"`
	OperationDescription          string   `yaml:"operation_description" json:"operation_description,omitempty"`
	OperationInputSchema          string   `yaml:"operation_input_schema" json:"operation_input_schema,omitempty"`
	OperationExamples             []string `yaml:"operation_examples" json:"operation_examples,omitempty"`
	OperationLazyStart            *bool    `yaml:"operation_lazy_start" json:"operation_lazy_start,omitempty"`
	OperationRequiresConfirmation *bool    `yaml:"operation_requires_confirmation" json:"operation_requires_confirmation,omitempty"`
}

// OperationSkillConfig is the codrax.yaml shape for one local operation skill.
// It is intentionally separate from internal/skill prompt skills: this config
// describes a side-effect capable local command provider that is visible only to
// the typed operation route and is executed only after operation approval.
type OperationSkillConfig struct {
	Name                          string                         `yaml:"name" json:"name"`
	Description                   string                         `yaml:"description" json:"description,omitempty"`
	WhenToUse                     []string                       `yaml:"when_to_use" json:"when_to_use,omitempty"`
	WhenNotToUse                  []string                       `yaml:"when_not_to_use" json:"when_not_to_use,omitempty"`
	OperationKinds                []string                       `yaml:"operation_kinds" json:"operation_kinds,omitempty"`
	OperationSurfaces             []string                       `yaml:"operation_surfaces" json:"operation_surfaces,omitempty"`
	OperationSideEffects          []string                       `yaml:"operation_side_effects" json:"operation_side_effects,omitempty"`
	OperationDescription          string                         `yaml:"operation_description" json:"operation_description,omitempty"`
	InputSchema                   string                         `yaml:"input_schema" json:"input_schema,omitempty"`
	OperationInputSchema          string                         `yaml:"operation_input_schema" json:"operation_input_schema,omitempty"`
	Examples                      []string                       `yaml:"examples" json:"examples,omitempty"`
	OperationExamples             []string                       `yaml:"operation_examples" json:"operation_examples,omitempty"`
	Workflows                     []OperationSkillWorkflowConfig `yaml:"workflows" json:"workflows,omitempty"`
	OutputContract                OperationSkillOutputContract   `yaml:"output_contract" json:"output_contract,omitempty"`
	OperationLazyStart            *bool                          `yaml:"operation_lazy_start" json:"operation_lazy_start,omitempty"`
	OperationRequiresConfirmation *bool                          `yaml:"operation_requires_confirmation" json:"operation_requires_confirmation,omitempty"`
	Command                       string                         `yaml:"command" json:"command"`
	Args                          []string                       `yaml:"args" json:"args,omitempty"`
	Env                           map[string]string              `yaml:"env" json:"env,omitempty"`
	InheritEnv                    *bool                          `yaml:"inherit_env" json:"inherit_env,omitempty"`
	WorkDir                       string                         `yaml:"work_dir" json:"work_dir,omitempty"`
	InputMode                     string                         `yaml:"input_mode" json:"input_mode,omitempty"`
	TimeoutMS                     *int                           `yaml:"timeout_ms" json:"timeout_ms,omitempty"`
	MaxOutputBytes                *int                           `yaml:"max_output_bytes" json:"max_output_bytes,omitempty"`
}

// OperationSkillWorkflowConfig is prompt-safe workflow catalog metadata for a
// local operation skill. It is rendered only to the operation planner and is
// not executable authority; execution still goes through ProviderInfo matching,
// operation approval, and the configured command provider.
type OperationSkillWorkflowConfig struct {
	Name           string                             `yaml:"name" json:"name"`
	Summary        string                             `yaml:"summary" json:"summary,omitempty"`
	Description    string                             `yaml:"description" json:"description,omitempty"`
	Entry          *bool                              `yaml:"entry" json:"entry,omitempty"`
	OperationKind  string                             `yaml:"operation_kind" json:"operation_kind,omitempty"`
	TargetSurface  string                             `yaml:"target_surface" json:"target_surface,omitempty"`
	NextProviders  []string                           `yaml:"next_providers" json:"next_providers,omitempty"`
	ReturnProvider string                             `yaml:"return_provider" json:"return_provider,omitempty"`
	RequiredInputs []string                           `yaml:"required_inputs" json:"required_inputs,omitempty"`
	Steps          []OperationSkillWorkflowStepConfig `yaml:"steps" json:"steps,omitempty"`
	Examples       []string                           `yaml:"examples" json:"examples,omitempty"`
}

// OperationSkillWorkflowStepConfig is one prompt-safe step descriptor inside a
// workflow catalog. It intentionally contains no command/env/work_dir fields.
type OperationSkillWorkflowStepConfig struct {
	ID            string `yaml:"id" json:"id,omitempty"`
	Provider      string `yaml:"provider" json:"provider,omitempty"`
	OperationKind string `yaml:"operation_kind" json:"operation_kind,omitempty"`
	TargetSurface string `yaml:"target_surface" json:"target_surface,omitempty"`
	Description   string `yaml:"description" json:"description,omitempty"`
}

// OperationSkillOutputContract summarizes which structured result fields the
// local skill may return. It is planner guidance only; the parser remains
// tolerant and execution policy remains authoritative.
type OperationSkillOutputContract struct {
	ArtifactRefs  *bool `yaml:"artifact_refs" json:"artifact_refs,omitempty"`
	PayloadRef    *bool `yaml:"payload_ref" json:"payload_ref,omitempty"`
	NextActions   *bool `yaml:"next_actions" json:"next_actions,omitempty"`
	ReturnAction  *bool `yaml:"return_action" json:"return_action,omitempty"`
	WorkflowState *bool `yaml:"workflow_state" json:"workflow_state,omitempty"`
}

// ExecutionSignals tracks boolean signals produced by agents. After
// the 2026-04-14 simplification only HasEnoughFacts remains — the
// write-pipeline signals (HasPlan, HasPatch, *ReviewPassed,
// VerificationPassed) all gated stages that no longer exist.
type ExecutionSignals struct {
	HasEnoughFacts bool `json:"has_enough_facts"`
}

// ActiveSetGateResult is the return shape of MultiRepoActiveSetGater.
// Lives in internal/types (a leaf package both internal/tool and
// internal/tool/repomap/multigraph can import) so the file-system
// tools (read_file, grep, repo_map) can call into the multi-repo
// gate without taking a direct dependency on the multigraph package
// (which transitively imports back into internal/tool, producing a
// cycle).
type ActiveSetGateResult struct {
	// Allowed reports whether the tool call may proceed.
	Allowed bool

	// ResolvedPath is the path the tool should actually use. When the
	// LLM-supplied path was already absolute or sub-repo-prefixed,
	// ResolvedPath is identical (modulo leading `./` strip). When the
	// LLM passed a bare relative path that auto-prefix matched a
	// unique active sub-repo, ResolvedPath is the prefixed form.
	ResolvedPath string

	// SubRepoRootRel is the active sub-repo's RootRel that the path
	// resolved into. Empty when Allowed=false or in the single-repo
	// bypass.
	SubRepoRootRel string

	// AutoPrefixed reports whether the gate auto-prefixed an
	// unprefixed bare path (telemetry / banner display).
	AutoPrefixed bool

	// RefusalProse is the user-facing refusal text the tool layer
	// surfaces as the tool's Summary. Empty when Allowed=true.
	// Generic prose, no internal pipeline terminology (R6 audited).
	RefusalProse string
}

// MultiRepoActiveSetGater is the contract the multigraph package
// implements to expose active-set checks to file-system tools.
// fileExists may be nil; callers pass an os.Stat-style probe when
// path-existence semantics matter (read_file's auto-prefix unique-
// match), or nil when the path is a directory (grep / repo_map).
//
// ResolveActiveSetCommand covers the free-form-shell surface
// (exec_command). It scans a command string for inactive sub-repo
// path tokens and refuses with the same prose family as the path
// gate when one is found. Only ResolvedPath/SubRepoRootRel are
// unset on this path; refusal carries the same RefusalProse contract.
type MultiRepoActiveSetGater interface {
	ResolveActiveSetPath(
		ctx *BusContext,
		toolName string,
		llmPath string,
		fileExists func(absPath string) bool,
	) ActiveSetGateResult

	ResolveActiveSetCommand(
		ctx *BusContext,
		toolName string,
		command string,
	) ActiveSetGateResult
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

	// PresentationDirective is typed current-turn display metadata
	// derived by the REPL turn policy from the user's own request
	// (for example "logic flow diagram" or "markdown table"). It is
	// deliberately separate from Mutable.Objective so render status
	// lines, repo_map queries, and memory never show system-generated
	// markdown headers as if they were user text.
	PresentationDirective string `json:"presentation_directive,omitempty"`

	// TurnRouteHint is typed current-turn routing metadata produced
	// before the read pipeline starts. It is not user prose and not
	// evidence; analyzer uses it only to avoid doing the wrong kind
	// of pre-scan before emit_analysis. It must never become a hard
	// answer gate.
	TurnRouteHint TurnRouteHint `json:"turn_route_hint,omitempty"`

	// ExploreDispatchKey is a scheduler-owned key for focused explorer
	// windows. It lets the explorer agent isolate mutable evaluator state
	// per DAG evidence node even when the scheduler uses the normal serial
	// dispatchStage path instead of the parallel fork runner.
	ExploreDispatchKey string `json:"-"`

	// ExploreDispatchKind is the structured TaskNode type for the current
	// focused explorer dispatch. It is render-only metadata: the evaluator
	// still keys state by ExploreDispatchKey, while the UI can distinguish a
	// serial reconcile pass from ordinary evidence collection without parsing
	// prompt text or node ids.
	ExploreDispatchKind TaskNodeType `json:"-"`

	// ExploreToolSurface is a scheduler-owned tool capability slice for this
	// focused explorer dispatch. Unlike ExploreDispatchKind, this field is
	// explicitly allowed to drive tool schema filtering and runtime tool-call
	// boundary checks because it is a typed execution contract, not UI metadata.
	ExploreToolSurface ExploreToolSurface `json:"-"`

	// ReadDispatchPolicy is a scheduler-owned one-dispatch policy for typed
	// read-loop retry actions. It is allowed to drive tool schema filtering,
	// runtime tool-call boundaries, and bounded per-dispatch budgets. It must
	// never be derived from prompt text, model rationale, or RawRequest words.
	ReadDispatchPolicy ReadDispatchPolicy `json:"-"`

	// SourceInventoryFollowupDebt is a scheduler-owned source-inventory
	// continuation contract. It is derived from typed observation/page/source
	// class carriers and may drive repo_map(source_inventory) route validation.
	SourceInventoryFollowupDebt SourceInventoryFollowupDebt `json:"-"`

	// CompletionOnlySurface marks the one bounded completion-obligation
	// dispatch the read scheduler grants when the explore loop drained
	// its budget without an accepted emit_investigation_complete while
	// a typed completion obligation (e.g. has_per_member_table's
	// member_set handoff) is still pending. The explorer's tool-schema
	// filter narrows the dispatch to the emit-only completion surface.
	// Scheduler-owned; never set by tools or models.
	CompletionOnlySurface bool `json:"-"`

	// ExploreLanePlan is a derived typed view over evidence-origin lanes for
	// the current request. It is scheduling/render guidance only: it must not
	// decide the answer and must not replace model-authored summaries.
	ExploreLanePlan ExploreLanePlan `json:"-"`

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

	// AttachedHitraceSource preserves the user's attachment spelling or
	// source hint for the perf trace channel. Values are advisory
	// strings such as "harmony_hitrace", "android_atrace", or
	// "generic_ftrace"; deterministic trace parsing must still fall
	// back to content detection when confidence is low.
	AttachedHitraceSource string `json:"attached_hitrace_source,omitempty"`

	// Mode is the pipeline's execution mode for this Run(). Zero-value
	// ("" / ModeRead) preserves pre-B0 read-only behavior byte-
	// identically — the orchestrator's Mode-dispatch switch falls
	// through to the existing runTaskPhase path when Mode is ModeRead
	// or the empty string. Set once at Run() entry from the CLI/yaml
	// layer; immutable for the rest of the Run. See
	// internal/types/pipeline_mode.go for constants.
	Mode PipelineMode `json:"mode,omitempty"`

	// EvalDisableGitHistory is an eval-only fairness guard. Normal
	// product runs leave it false so VCS history tools keep working;
	// SWE-bench style fair runs may set it to force tools to consume
	// only the current checkout/worktree state. Because tool dispatches
	// receive narrowed BusContext views, this field is mirrored in
	// AgentContext and listed in projectionTypedSignalFields.
	EvalDisableGitHistory bool `json:"eval_disable_git_history,omitempty"`

	// MainRepoRoot is the original target repo root the user passed
	// via --repo. In read-only mode equals RepoRoot throughout the
	// Run. In write modes the apply/verify stages swap RepoRoot to
	// the worktree checkout path while the plan and finalize stages
	// see MainRepoRoot. Empty in pre-B0 callers (tests) — code that
	// needs the "original root" falls back to RepoRoot in that case.
	MainRepoRoot string `json:"main_repo_root,omitempty"`

	// MultiGraph is the multi-repo carrier for this Run (design v2).
	// Non-nil when codrax.yaml :: multi_repo_enabled is true (the
	// default) AND the orchestrator has a topology to wire (cmd/root
	// builds it from app.topology). Single-repo posture wraps a
	// single *Graph; multi-repo posture exposes per-sub-repo graphs
	// via AllGraphs / GraphFor / Oracle / Locator.
	//
	// nil when multi_repo_enabled=false OR the orchestrator was not
	// supplied a topology (test fixtures, eval harness without an
	// app context). Read-side consumers MUST tolerate nil and fall
	// back to legacy `repomap.BuildOrLoadGraph(RepoRoot, query)`
	// pre-multi-repo behaviour.
	//
	// Mutating MultiGraph mid-Run is forbidden — the orchestrator
	// installs it once at Run entry and treats it as immutable for
	// the lifetime of the BusContext. The carrier itself is
	// thread-safe (its LRU has internal locking).
	//
	// Treated as opaque by JSON serialisation (json:"-"). Stored as
	// any so types/ stays free of import cycles on the multigraph
	// package — consumers (agent / orchestrator layer) cast to
	// *multigraph.MultiGraph at the use site. nil-tolerant: the
	// helper MultiGraphFromContext below performs the cast and
	// surfaces nil when the field is unset / non-multigraph.
	MultiGraph any `json:"-"`

	// SearchGraph is an optional read-only repo_map graph handle for
	// tool dispatches that deliberately do not receive Mutable. The main
	// explorer normally shares the graph through Mutable.SearchGraph();
	// sub-agent tool calls use this field so navigation tools can reuse
	// the parent graph without gaining write access to MutableState.
	SearchGraph any `json:"-"`

	// SubRepos is a snapshot of the multi-repo topology — same
	// content as MultiGraph.Topology().Repos but copied here so
	// non-MultiGraph-aware consumers (REPL renderer, telemetry) can
	// read sub-repo metadata without owning a *MultiGraph reference.
	// nil in single-repo / pre-multi-repo callers.
	SubRepos []SubRepoSnapshot `json:"sub_repos,omitempty"`

	// ActiveSubRepo names the single sub-repo this Run is targeting.
	// Set by write-mode flows (plan / apply / verify) to the
	// converged target — design §4.5.5 fail-louds when a write
	// ChangePlan would span multiple sub-repos. Read mode leaves
	// this nil; multi-repo Routing fold treats the entire active
	// LRU as queryable, not just one sub-repo.
	ActiveSubRepo *SubRepoSnapshot `json:"active_sub_repo,omitempty"`

	// PendingSubRepos is the user-visible RootRel list of sub-repos
	// the routing fold left INACTIVE because of cap pressure. Read
	// by the LLM-facing summary so the answer can disclose
	// "we did not consult X / Y / Z" (R3 partial_typed_lane red
	// line). Slugs are NEVER surfaced here — only RootRel — to keep
	// internal pipeline identifiers out of LLM prompts (R6 red line).
	PendingSubRepos []string `json:"pending_sub_repos,omitempty"`

	// MultiRepoInactivePreviewCount caps how many out-of-active sub-
	// repos the L0 advisory surfaces to the LLM. Stamped by cmd/root
	// from codrax.yaml :: multi_repo_inactive_preview_count and clamped
	// through config.ClampMultiRepoInactivePreviewCount (default 2,
	// hard ceiling 3). 0 here is treated as "not yet stamped" by the
	// advisory builder, which falls back to the config default.
	MultiRepoInactivePreviewCount int `json:"multi_repo_inactive_preview_count,omitempty"`

	// MultiRepoFocusDecision explains how the current active set was
	// selected (user-pinned, model-recommended, or fallback preview).
	// It is LLM/UX scope metadata only; it is not source evidence.
	MultiRepoFocusDecision *MultiRepoFocusDecision `json:"multi_repo_focus_decision,omitempty"`

	// TypedDenials is the architectural negative-knowledge channel.
	// Any typed gate that downgrades a structured field (frame.File
	// cleared by frameFileCorroboratesFunc / oracle.SymbolExists fail
	// / drift detector FileMoved / evidence ground miss / future MCP
	// shape mismatch) MUST stamp the corresponding raw token here.
	//
	// Three enforcement surfaces consume it (R3 second-axis red line):
	//   - L1 tool-call: read_file / grep / repo_map registry guards
	//     call IsPathDenied → typed error to LLM
	//   - L2 prompt rendering: builder calls Sanitise on raw fields
	//     (frame.Raw, stall raw text) → LLM cannot extract verbatim
	//     denied tokens from prose
	//   - L3 answer validator: ViolDeniedTokenUndeclared fires when
	//     finalize prose names a denied token without an "unverified"
	//     caveat
	//
	// Append-only during a Run; the orchestrator allocates ONE set at
	// Run entry (NewTypedDenialSet) and every narrowing helper
	// (ToolBusContext / SubAgentContext / BuildAgentContext /
	// ShallowClone) propagates the SAME pointer, so a denial a tool
	// stamps mid-dispatch reaches all three surfaces immediately.
	// Pointer — never a value copy: the pre-2026-06-12 value field
	// meant the per-tool-call projection got a slice-header copy and
	// every tool-stamped denial died with the discarded projection.
	// The set is internally synchronized (parallel explore workers
	// share it). nil-safe: a nil set behaves as "no denials" on every
	// read and ignores Add — the channel is simply disabled for bare
	// test fixtures that never allocate it.
	TypedDenials *TypedDenialSet `json:"typed_denials,omitempty"`

	// Memory is the read-only handle into the REPL memory store. nil
	// in single-shot CLI runs / non-REPL test fixtures (no Store to
	// wire). recall_memory tool nil-checks before calling Search;
	// other tools never touch this field. Not serialised — the
	// pointer is process-local only.
	Memory MemoryReader `json:"-"`

	// Ctx is the cancellation-aware context the orchestrator
	// derives from its CancelToken at Run() entry. HTTP-level
	// callers (LLM Adapter.Chat, exec.CommandContext, worktree
	// git operations) should use BusContext.Context() — never read
	// this field directly — so nil-safe degradation works in test
	// fixtures and single-shot CLI paths. Set to nil between Runs;
	// Context() returns context.TODO() in that case.
	Ctx context.Context `json:"-"`

	// EnvFacts is the cached environment snapshot the env_recommend
	// subsystem produces. nil when env_recommend is disabled or
	// the probe failed; tools that consume it must nil-check.
	// Probed once per Run at orchestrator entry.
	EnvFacts *EnvFacts `json:"env_facts,omitempty"`

	// EnvRecommendSettings carries the resolved yaml knobs for the
	// env_recommend pipeline. Used by tools (run_tests) and the
	// orchestrator's diagnose/recommend dispatch to gate on the
	// master switch and pass the LLM-timeout / RecommendGlobalInstall
	// flags through to the recommender.
	EnvRecommendSettings EnvRecommendSettings `json:"env_recommend_settings,omitempty"`

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

	// answerSurfaceCache stores expensive, deterministic answer-surface
	// projections at the orchestrator/BusContext layer. Finalizer
	// post-emit validators repeatedly need the same surface/view/support
	// contracts after the answer document is written; rebuilding them
	// against large evidence pools can keep the REPL in local CPU work.
	// The cache key includes slice sizes plus MutableState's answer-
	// surface revision, and orchestrator.applyStageOutput explicitly
	// invalidates it when truth-set slices change.
	answerSurfaceCacheMu     sync.Mutex
	answerSurfaceCacheKey    answerPlanCacheKey
	answerSurfacePlanCached  bool
	answerSemanticViewCached bool
	answerSupportPlanCached  bool
	answerSurfacePlan        *AnswerSurfacePlan
	answerSemanticView       *AnswerSemanticView
	answerSupportPlan        *AnswerSupportPlan

	groundingContextCacheMu    sync.Mutex
	groundingContextCacheKey   string
	groundingContextCacheValue any
}

func (ctx *BusContext) GroundingContextCacheGet(key string) (any, bool) {
	if ctx == nil || key == "" {
		return nil, false
	}
	ctx.groundingContextCacheMu.Lock()
	defer ctx.groundingContextCacheMu.Unlock()
	if ctx.groundingContextCacheKey != key || ctx.groundingContextCacheValue == nil {
		return nil, false
	}
	return ctx.groundingContextCacheValue, true
}

func (ctx *BusContext) GroundingContextCacheSet(key string, value any) {
	if ctx == nil || key == "" || value == nil {
		return
	}
	ctx.groundingContextCacheMu.Lock()
	defer ctx.groundingContextCacheMu.Unlock()
	ctx.groundingContextCacheKey = key
	ctx.groundingContextCacheValue = value
}

// AgentContext provides the narrowed view of BusContext for a single agent.
type AgentContext struct {
	AgentName AgentName     `json:"agent_name"`
	Stage     PipelineStage `json:"stage"`

	// TraceID mirrors BusContext.TraceID so agent implementations that
	// maintain per-run caches can separate one Run() from the next
	// without reading the full BusContext.
	TraceID string `json:"trace_id,omitempty"`

	// ExploreDispatchKey is set by the read scheduler for focused
	// explorer windows. The explorer agent uses it to keep evaluator
	// state isolated per DAG evidence node while preserving legitimate
	// self-loop state for a re-dispatch of the same node.
	ExploreDispatchKey string `json:"-"`

	// ExploreDispatchKind mirrors BusContext.ExploreDispatchKind for
	// render-only activity labels. It must not drive evaluator behavior.
	ExploreDispatchKind TaskNodeType `json:"-"`

	// ExploreToolSurface mirrors BusContext.ExploreToolSurface for scheduler-
	// owned tool schema filtering and runtime boundary checks.
	ExploreToolSurface ExploreToolSurface `json:"-"`

	// ReadDispatchPolicy mirrors BusContext.ReadDispatchPolicy for scheduler-
	// owned tool schema filtering, runtime boundaries, and dispatch budgets.
	ReadDispatchPolicy ReadDispatchPolicy `json:"-"`

	// SourceInventoryFollowupDebt mirrors BusContext.SourceInventoryFollowupDebt
	// for source-inventory follow-up route validation.
	SourceInventoryFollowupDebt SourceInventoryFollowupDebt `json:"-"`

	// CompletionOnlySurface mirrors BusContext.CompletionOnlySurface
	// for the explorer's tool-schema filter (emit-only completion
	// dispatch). See the BusContext field for semantics.
	CompletionOnlySurface bool `json:"-"`

	// ExploreLanePlan mirrors BusContext.ExploreLanePlan for prompt and
	// render-only scoped exploration guidance.
	ExploreLanePlan ExploreLanePlan `json:"-"`

	// ParallelGroupID is render-only telemetry for orchestrator-owned
	// bounded fan-out. When set, BaseAgent mirrors it onto LLM/tool
	// activity events together with ExploreDispatchKey so the renderer
	// can aggregate concurrent worker state without parsing prompt text
	// or inferring from focus labels.
	ParallelGroupID string `json:"-"`

	// Mode mirrors BusContext.Mode for the agent's narrowed view.
	// The analyzer reads it to gate read-mode-only quality checks
	// (hypothesis_coverage / contract_complete) so write-mode
	// classifier dispatches don't reject "create from scratch"
	// requests that have nothing to investigate. Zero-value ("" or
	// ModeRead) preserves pre-write-mode behaviour byte-identically.
	Mode PipelineMode `json:"mode,omitempty"`

	Objective string `json:"objective"`

	// PresentationDirective mirrors BusContext.PresentationDirective
	// for prompt construction. It may affect answer presentation
	// affordances only; it is not repository evidence, not a code
	// entity, and not a search query.
	PresentationDirective string `json:"presentation_directive,omitempty"`

	// TurnRouteHint mirrors BusContext.TurnRouteHint for prompt
	// construction. It is typed current-turn routing metadata, not
	// repository evidence and not a substitute for emit_analysis.
	TurnRouteHint TurnRouteHint `json:"turn_route_hint,omitempty"`

	// AnalysisIR aliases BusContext.AnalysisIR for agents that have
	// opted into the v3 pipeline. Still nil for legacy call paths —
	// consumers MUST nil-check before reading.
	AnalysisIR *AnalysisIR `json:"-"`

	RelevantFacts []string       `json:"relevant_facts,omitempty"`
	RelevantFiles []string       `json:"relevant_files,omitempty"`
	EvidenceItems []EvidenceItem `json:"evidence_items,omitempty"`
	// TypedRelationHints is the system-derived structural-relation
	// candidate channel (P3 #6 follow-up, 2026-05-03). Populated at
	// BuildAgentContext time when the analyzer's predicates indicate
	// a structural enumeration / relational lookup AND at least one
	// analyzer entity resolves to a typed-graph relation source
	// (interface / trait / protocol / class / function / package).
	// The prompt assembler render-time-merges these into the
	// "Structured Evidence" section with a Provenance column tagging
	// each row as llm_evidence vs typed_graph. Dedup by
	// (Subject, Object, AnchorKind) keeps the LLM's mental model
	// unified — one evidence pool, two provenance lanes — rather
	// than splitting into separate sections that the LLM would have
	// to mentally union. Per
	// feedback_no_system_backfill_to_user_panel, these never reach
	// AnswerDocument fields; they are pure prompt input.
	TypedRelationHints []TypedRelationHint `json:"typed_relation_hints,omitempty"`
	FlowFindings       []FlowFindingDigest `json:"flow_findings,omitempty"`
	AnswerChains       []AnswerChain       `json:"answer_chains,omitempty"`
	AnswerSymbols      []AnswerSymbol      `json:"answer_symbols,omitempty"`
	// AnswerSymbolCompleteness mirrors BusContext.AnswerSymbolCompleteness
	// for the narrowed agent view. Read by finalize's prompt builder
	// to pick Translation / softened-floor / shape-based rendering.
	AnswerSymbolCompleteness CompletenessClaim `json:"answer_symbol_completeness,omitempty"`
	RelevantToolSummaries    []string          `json:"relevant_tool_summaries,omitempty"`
	RelevantMCPNotes         []string          `json:"relevant_mcp_notes,omitempty"`
	MCPResponses             []MCPResponse     `json:"-"`
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

	// EvalDisableGitHistory mirrors BusContext.EvalDisableGitHistory
	// through agent/tool projections. Eval-only; default false.
	EvalDisableGitHistory bool `json:"eval_disable_git_history,omitempty"`

	// MainRepoRoot mirrors BusContext.MainRepoRoot — the original
	// user-supplied target repo path BEFORE any worktree swap that
	// write-mode stage hooks may perform. Tools (env_recommend bare-dir
	// authorization, recall_memory diagnostics) read this when they
	// need to talk about "the user's repo" rather than the sandbox.
	// Empty in non-write paths is fine — RepoRoot is then the same dir.
	MainRepoRoot string `json:"main_repo_root,omitempty"`

	// WorktreePath mirrors BusContext.WorktreePath so tool dispatches can
	// resolve stale absolute main-checkout paths onto the active writable
	// checkout during write-mode apply/replan/verify loops. Empty outside
	// active write workflows.
	WorktreePath string `json:"worktree_path,omitempty"`

	// Multi-repo mirrors of BusContext fields — so agents that hold
	// only an AgentContext (most do, post-AgentContextBuilder) can
	// read the multi-repo carrier without taking a *BusContext
	// reference. Stored as `any` for the same import-cycle reason
	// described on BusContext.MultiGraph.
	MultiGraph                    any                     `json:"-"`
	SubRepos                      []SubRepoSnapshot       `json:"sub_repos,omitempty"`
	ActiveSubRepo                 *SubRepoSnapshot        `json:"active_sub_repo,omitempty"`
	PendingSubRepos               []string                `json:"pending_sub_repos,omitempty"`
	MultiRepoInactivePreviewCount int                     `json:"multi_repo_inactive_preview_count,omitempty"`
	MultiRepoFocusDecision        *MultiRepoFocusDecision `json:"multi_repo_focus_decision,omitempty"`

	// TypedDenials mirrors BusContext.TypedDenials (Phase A.2 of the
	// negative-knowledge architecture). Tools dispatched from the
	// agent layer consult it to refuse calls naming denied tokens;
	// prompt builders call .Sanitise on raw fields. Pointer (rather
	// than value) so a tool call that stamps a fresh denial mid-
	// dispatch is visible to subsequent calls in the same loop.
	TypedDenials *TypedDenialSet `json:"typed_denials,omitempty"`

	// Memory mirrors BusContext.Memory so the recall_memory tool can
	// query prior-conversation memory from the agent dispatch path.
	// Pre-this-fix, BuildAgentContext dropped Memory and the agent's
	// executeTool then reconstructed a busCtx without it — causing
	// recall_memory to return "unavailable" even in interactive REPL
	// runs where the orchestrator HAD wired the adapter.
	Memory MemoryReader `json:"-"`

	// Ctx mirrors BusContext.Ctx — the cancellation-aware context
	// the orchestrator hands down so HTTP-level operations (LLM
	// Adapter.Chat, subprocess via context.Context) cancel
	// immediately on Ctrl+C / /cancel rather than waiting for the
	// next cooperative checkpoint. Read via Context() for nil-safe
	// degradation.
	Ctx context.Context `json:"-"`

	// EnvFacts mirrors BusContext.EnvFacts (the env_recommend probe
	// snapshot). Tools that surface install hints / bare-dir guidance
	// (run_tests env_recommend integration, write-mode apply pre-hook
	// fallback) read this. nil in single-shot CLI / when env_recommend
	// is disabled.
	EnvFacts *EnvFacts `json:"env_facts,omitempty"`

	// EnvRecommendSettings mirrors BusContext.EnvRecommendSettings —
	// the resolved yaml knobs for env_recommend (master switch, LLM
	// fallback, sudo gate, cache TTL). Same consumers as EnvFacts.
	EnvRecommendSettings EnvRecommendSettings `json:"env_recommend_settings,omitempty"`

	// Mutable aliases the orchestrator's BusContext.Mutable so that
	// tools dispatched from this agent (via BaseAgent.executeTool)
	// can write to the shared region. This breaks the strict
	// "AgentContext is a value-only narrow view" rule for one specific
	// pointer field by design — see MutableState's doc.
	Mutable *MutableState `json:"-"`

	// answerSurfaceCache stores expensive, deterministic answer-surface
	// projections for this single dispatch. The finalizer prompt path asks
	// for the same surface/view/support contracts from many helper sections
	// before the first LLM request; without a per-dispatch cache, large
	// evidence pools can burn CPU while the UI is still in the "preparing
	// context" state. Callers receive defensive clones from the public
	// builders, so local prompt helpers can still adjust their copy without
	// corrupting later readers.
	answerSurfaceCacheMu sync.Mutex
	answerSurfacePlan    *AnswerSurfacePlan
	answerSemanticView   *AnswerSemanticView
	answerSupportPlan    *AnswerSupportPlan

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

	// ExtractorSoftIterCapOverride mirrors PlannerSoftIterCapOverride
	// for the extractor evaluator. Set by the orchestrator's
	// per-dispatch scaling block when nSub > 1 or complexity is
	// elevated, so the extractor has room to emit one Key-Anchor row
	// per sub-topic (5a356ec). Hard cap is derived as soft + recovery
	// slack inside the evaluator. Zero leaves the static
	// AgentSettings.ExtractorSoftIterCap in effect.
	ExtractorSoftIterCapOverride int `json:"-"`

	// VerifierSoftIterCapOverride mirrors PlannerSoftIterCapOverride
	// for the verifier evaluator. Set by the orchestrator's
	// per-dispatch scaling block from len(plan.TargetPaths), so a
	// multi-language monorepo plan that needs N runner invocations
	// gets enough iterations to dispatch run_tests N times before
	// the soft cap fires. Zero leaves the static
	// AgentSettings.VerifierSoftIterCap in effect.
	VerifierSoftIterCapOverride int `json:"-"`

	// ActivePitfalls carries the stage-3 Failure Taxonomy
	// entries the orchestrator deemed relevant to THIS plan
	// dispatch. Populated by stage_hooks.planPreHook (or
	// equivalently the multi-phase scheduler) BEFORE the
	// planner dispatches; consumed by the planner's
	// BuildInitialInstruction via buildActivePitfallsSection.
	// Empty slice = no relevant pitfalls (or the feature is
	// disabled). The planner renders this under a "## Known
	// active pitfalls in this repo" heading so the LLM has
	// see prior failure modes before emitting.
	ActivePitfalls []FailurePattern `json:"-"`

	// ActiveAnswerPitfalls carries the read-mode Answer Taxonomy
	// entries (commit 51, mirror of ActivePitfalls but for the
	// read pipeline). Populated by orchestrator.dispatchStage
	// BEFORE the analyzer dispatches, consumed by the analyzer's
	// BuildInitialInstruction via buildActiveAnswerPitfallsSection.
	// Empty = no relevant patterns OR the feature is disabled.
	// Same descriptive framing as ActivePitfalls — the analyzer
	// reads observations, not instructions.
	ActiveAnswerPitfalls []AnswerPattern `json:"-"`

	// EmitStageRetryAttempt is the 0-based retry-attempt counter for
	// stages whose terminal action is a structured `emit_*` tool call
	// (analyze / extract / finalize / log_triage / perf_triage). The
	// orchestrator's per-stage retry loop (runAnalyzePhase et al)
	// increments this on each re-dispatch after a "tool_choice=required
	// did not produce a tool call" failure. The value of 0 is the
	// happy path (first attempt); >= 1 activates terminal forcing in
	// the agent layer:
	//
	//  1. The agent's BuildInitialInstruction prepends a literal
	//     tool-call template so a model that produced text-only on
	//     attempt 0 sees the exact JSON shape it must emit.
	//
	//  2. agent.go::dispatchOnce switches ChatOptions.ToolChoice from
	//     the generic "required" string to the named-function form
	//     `{"type":"function","function":{"name":"<emit_tool>"}}`,
	//     which some providers honor more reliably than bare
	//     "required" (observed on certain GLM / MiniMax / DeepSeek
	//     deploys).
	//
	// The pattern is "escalate the protocol AND escalate the prompt"
	// on every failed retry; together they close the failure mode
	// where a model acknowledged the requirement in <think> but still
	// produced no tool call.
	EmitStageRetryAttempt int `json:"-"`

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

	// AttachedHitraceSource mirrors BusContext.AttachedHitraceSource
	// for tools that need producer/flavor hints without inspecting
	// command-line state.
	AttachedHitraceSource string `json:"attached_hitrace_source,omitempty"`

	// LogTriage mirrors Mutable.LogTriage() into the narrowed agent
	// view, so consumers that want the structured bundle (analyzer's
	// entity merge, reconcileIntent, RequiredFiles seeding) can read
	// it without reaching through Mutable. Nil when no log was
	// attached, the triage stage was skipped, or the stage degraded.
	// Readers MUST nil-check.
	LogTriage *LogBundle `json:"-"`

	// PreStageDegradations mirrors Mutable.PreStageDegradations for
	// prompt builders: when an attached-artifact pre-stage failed, the
	// prompt explains WHY the structured summary is absent instead of
	// leaving the model to guess.
	PreStageDegradations []PreStageDegradation `json:"-"`

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

// Context returns the cancellation-aware context.Context attached to
// this BusContext, or context.TODO() when the field is unset (test
// fixtures, single-shot CLI paths between Runs). Callers that want
// HTTP-level cancellation derive from this — passing nil is rejected
// by the LLM Adapter so context.TODO() is the safest fallback.
func (b *BusContext) Context() context.Context {
	if b == nil || b.Ctx == nil {
		return context.TODO()
	}
	return b.Ctx
}

// Context mirrors BusContext.Context() for the agent-scoped view.
// Same nil-safety contract: returns context.TODO() when the agent
// view never had a ctx attached.
func (a *AgentContext) Context() context.Context {
	if a == nil || a.Ctx == nil {
		return context.TODO()
	}
	return a.Ctx
}
