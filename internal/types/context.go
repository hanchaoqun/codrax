package types

import (
	"context"
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

	// SoftAnalyzerError carries the analyzer-stage exhaustion error
	// when the orchestrator falls back to buildDegradedSemanticIR.
	// Distinct from LastError because LastError is consumed by the
	// outer guard at runTaskPhase entry — setting LastError here
	// would skip explorer/extractor/finalizer dispatch entirely,
	// turning the degraded path into a no-op "(no result)" output.
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
	requestModel            *RequestModel
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
	turnAArtifacts                   *TurnAArtifacts
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
	dispatchToolResults []ToolResult
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
	// fires during plan stage via run_tests(dry_run=true). Distinct
	// from ChangeReport (which is the verify stage's authoritative
	// outcome) so a plan-stage probe NEVER pollutes the verify
	// channel and the verify→plan retry loop continues to read only
	// real verify results. Append-only inside a Run; reset at Run
	// boundary alongside the iteration ledger.
	planStageProbeReports []*ChangeReport

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

	// RuntimeObservationOnlyCompletion marks the narrow case where Turn
	// A legitimately completed from a structured external log / trace
	// artifact without any current-repo read/search evidence. It is set
	// only after a typed observation-only RequestModel and a successful
	// emit_investigation_complete result_kind, so downstream empty-
	// investigation gates can accept the artifact handoff without
	// forcing fixture reads or repo citations.
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
	if proj := loadEvidenceProjector(); proj != nil {
		items = proj(items, m)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emittedEvidence = append(m.emittedEvidence, items...)
	m.bumpAnswerSurfaceRevisionLocked()
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

// EmittedEvidence returns a copy of the LLM-emitted evidence buffer.
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

// EmittedEvidenceSince returns a snapshot of evidence appended at or after
// start plus the current total length. It lets loop observers merge only the
// new tail without copying the full evidence pool on every pass.
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
	m.lastRejectedAnswerDocumentV2 = nil
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
				SurfaceRole:             b.SurfaceRole,
			}
			if len(b.Items) > 0 {
				cloned.Items = make([]AnswerBlockItem, len(b.Items))
				for j, it := range b.Items {
					cloned.Items[j] = AnswerBlockItem{
						ID:            it.ID,
						Label:         it.Label,
						Text:          it.Text,
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
	if a.AcceptedAggregateFacts != nil {
		snap.AcceptedAggregateFacts = cloneAnswerAggregateFacts(a.AcceptedAggregateFacts)
	}
	m.turnAArtifacts = &snap
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
	if m.turnAArtifacts.AcceptedAggregateFacts != nil {
		out.AcceptedAggregateFacts = cloneAnswerAggregateFacts(m.turnAArtifacts.AcceptedAggregateFacts)
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
	m.cachedLabelSupport = nil
	m.cachedLabelSupportSource = nil
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
	m.prescanRoundCount++
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

type ToolResult struct {
	ToolName  string      `json:"tool_name"`
	Summary   string      `json:"summary"`
	Repair    *ToolRepair `json:"repair,omitempty"`
	RawRef    string      `json:"raw_ref,omitempty"`
	Success   bool        `json:"success"`
	Timestamp time.Time   `json:"timestamp"`
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
	// Append-only during a Run; the orchestrator ZERO-initialises at
	// Run entry and consumers append via Add. nil-safe: empty value
	// behaves as "no denials" everywhere.
	TypedDenials TypedDenialSet `json:"typed_denials,omitempty"`

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
}

// AgentContext provides the narrowed view of BusContext for a single agent.
type AgentContext struct {
	AgentName AgentName     `json:"agent_name"`
	Stage     PipelineStage `json:"stage"`

	// Mode mirrors BusContext.Mode for the agent's narrowed view.
	// The analyzer reads it to gate read-mode-only quality checks
	// (hypothesis_coverage / contract_complete) so write-mode
	// classifier dispatches don't reject "create from scratch"
	// requests that have nothing to investigate. Zero-value ("" or
	// ModeRead) preserves pre-write-mode behaviour byte-identically.
	Mode PipelineMode `json:"mode,omitempty"`

	Objective string `json:"objective"`

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

	// MainRepoRoot mirrors BusContext.MainRepoRoot — the original
	// user-supplied target repo path BEFORE any worktree swap that
	// write-mode stage hooks may perform. Tools (env_recommend bare-dir
	// authorization, recall_memory diagnostics) read this when they
	// need to talk about "the user's repo" rather than the sandbox.
	// Empty in non-write paths is fine — RepoRoot is then the same dir.
	MainRepoRoot string `json:"main_repo_root,omitempty"`

	// Multi-repo mirrors of BusContext fields — so agents that hold
	// only an AgentContext (most do, post-AgentContextBuilder) can
	// read the multi-repo carrier without taking a *BusContext
	// reference. Stored as `any` for the same import-cycle reason
	// described on BusContext.MultiGraph.
	MultiGraph                    any               `json:"-"`
	SubRepos                      []SubRepoSnapshot `json:"sub_repos,omitempty"`
	ActiveSubRepo                 *SubRepoSnapshot  `json:"active_sub_repo,omitempty"`
	PendingSubRepos               []string          `json:"pending_sub_repos,omitempty"`
	MultiRepoInactivePreviewCount int               `json:"multi_repo_inactive_preview_count,omitempty"`

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
