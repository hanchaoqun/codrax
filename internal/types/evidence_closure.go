package types

import (
	"fmt"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/canonpath"
)

// EvidenceClosure is the per-Run cross-stage tracker for the four
// CGEC (Citation-Grounded Evidence Closure) invariants:
//
//	I1: every file:line surfaced in a downstream prompt must be in
//	    ReadSet (chain promotion enforces this — chains anchored
//	    outside ReadSet get demoted to PendingReads).
//
//	I2: every emit_*-accepted citation must be in ReadSet
//	    (emit_answer_document grounder writes RepairDirectives
//	    describing the gap when a citation is rejected).
//
//	I3: emit_investigation_complete may not flip the flag unless the
//	    contract checker would pass on the current EvidenceItems +
//	    ReadSet snapshot (pre-complete simulation lives in
//	    emit_investigation_complete.Execute).
//
//	I4: between two retries at least one of (ReadSet, EvidenceCount,
//	    ChainTermSet) must change — otherwise the orchestrator is
//	    in a stall and triggers Lazy Auto-Read.
//
// EvidenceClosure is the single source of truth that all four
// enforcers read from / write to. Lives on MutableState so every
// stage shares the same view. The orchestrator runTaskGraph calls
// MutableState.ResetEvidenceClosure at task entry so cross-task
// contamination is impossible (mirror of ResetTurnAArtifacts /
// ResetAnswerDocument lifecycle).
//
// Concurrency: EvidenceClosure has its own mutex because individual
// fields are written from independent code paths (chain producer,
// grounder, scheduler) and a single MutableState mutex would force
// them to serialize. Internal locking; callers go through methods.
type EvidenceClosure struct {
	mu sync.RWMutex

	// repoRoot is the absolute (or `.`-relative) repository root the
	// orchestrator passed down. When non-empty the closure's internal
	// canonicaliser strips an absolute-path prefix that matches this
	// root so LLM-supplied paths like
	// `/mnt/d/opt/codrax-main/internal/foo.go` collapse onto the
	// same map keys as repo-relative forms (`internal/foo.go`).
	// Empty repoRoot degrades to the historical
	// canonicalizeRepoPath behaviour — strip `./` and normalise
	// separators — so tests constructing with NewEvidenceClosure("")
	// keep their pre-session-22 semantics.
	repoRoot string

	// readSet is the canonical set of repo-relative file paths Turn A
	// successfully fetched via read_file (or the framework forced-read
	// via Lazy Auto-Read). Mirror of TurnAArtifacts.ReadFiles but
	// kept current per-explore-window so the chain producer and the
	// pre-complete check can read it without round-tripping through
	// extractFileCoverage on the full tool history.
	readSet map[string]bool

	// readRanges is the line-level companion to readSet: file →
	// merged sorted [Start, End] intervals the LLM actually saw.
	// Populated from read_file banner ranges (extractFileCoverage)
	// and preRead slices. Empty entry for a file still in readSet
	// means "touched but we did not record a specific range", in
	// which case HasReadLine grants the file wholesale (backward
	// compatible with tests that only set readSet). The chain
	// promotion enforcer opts into HasReadLine when the anchor
	// carries a line number so partial-read scenarios (paginated
	// read_file, symbol-guided preRead) can correctly demote chains
	// whose anchor falls outside any fetched range.
	readRanges map[string][]LineRange

	// fileTotalLines records the total line count of each file the
	// LLM has read, parsed from the read_file banner ("[path:
	// showing lines X-Y of Z total]" or "[path: showing all N lines
	// ...]"). The line-merging gate uses this to compute per-file
	// CoverageRatio = sum(end-start+1) / total, so a 113-line file
	// fully read shows 1.0 even when a 683-line sibling is at 0.6.
	// Pre-2026-04-29 the predecessor multi-path parity gate
	// compared absolute line counts against max(read_lines), letting
	// a small file at 100% coverage fail because it had fewer
	// absolute lines than a partially-read large sibling — the very
	// pathology this field originally exists to fix. The ratio gate
	// itself was later retired in favour of symbol-region verification
	// (see internal/tool/multipath/), but per-file totals remain the
	// authoritative denominator for HasFullyRead and any other future
	// per-file coverage signal.
	//
	// Zero entry (file in readRanges but absent from this map) means
	// "total unknown" — typically because the read came from a path
	// that bypassed the banner (e.g. forced-read injection) or the
	// banner was malformed. Callers that need a denominator must
	// treat zero as "unknown" and either skip the ratio check or
	// fall back to file-level booleans.
	fileTotalLines map[string]int

	// scannedSet is the broader set of files the deterministic
	// concrete-value scanner consulted from disk (keyword-scored
	// files + symbol-definition files mentioned in investigation
	// notes). Used by the chain promotion enforcer to distinguish
	// "scanned but unread" (PendingRead candidate) from "indexed
	// only" (no-op).
	scannedSet map[string]bool

	// citedRefs records every (file, lineNumber) that has appeared
	// in an emit_*-accepted citation pool so far. Lets the
	// convergence detector spot "the LLM keeps citing the same line"
	// signatures. Map: file → sorted unique line numbers.
	citedRefs map[string][]int

	// acceptedEvidence stores bounded typed identities for evidence already
	// accepted in this run. RepairDirective snapshots copy this carrier so a
	// later extractor/finalizer/report can consume precise evidence refs
	// without scraping retry prose or losing prior-stage context.
	acceptedEvidence []AcceptedEvidenceRef

	// pendingReads is the queue of files the framework has decided
	// MUST be read before emit_investigation_complete can succeed.
	// Populated by chain promotion (anchor outside ReadSet) and by
	// emit_answer_document grounder (citation in unread file). The
	// orchestrator surfaces these to the next explore round's prompt
	// as a "Forced Read List" and falls back to Lazy Auto-Read when
	// the LLM still skips them after a configurable budget.
	pendingReads []PendingRead

	// unverifiedFinds is the list of analyzer-emitted file paths /
	// symbol identifiers that the findings validator could not match
	// against the repo. Surfaced to downstream stages as a hygiene
	// warning (rendered with strikethrough + ·) so the extractor
	// and finalizer do not bake hallucinated artefacts into the
	// answer.
	unverifiedFinds []UnverifiedFinding

	// subjectMatches caches the subject.Score result per chain
	// summary so the chain ranker (called multiple times across
	// dedup + cap + render) does not re-compute the heuristic. Key
	// is the chain summary string; value is the score in [0, 1].
	subjectMatches map[string]float64

	// fingerprints is the rolling history of ClosureFingerprint
	// values, one entry per explore round. The convergence detector
	// compares the latest two entries to spot stalls (no progress
	// across a retry) and three-in-a-row to force-finalize.
	fingerprints []ClosureFingerprint

	// downgradeFingerprints is the rolling history of pre-complete
	// downgrade fingerprints, one entry per emit_investigation_complete
	// attempt that produced a downgrade. The low-delta convergence
	// boundary force-completes after N consecutive equal entries (same
	// lane + same typed blocker), breaking the runaway re-issue loop.
	// Bounded to keep memory flat under a runaway.
	downgradeFingerprints []DowngradeFingerprint

	// latestProgressDecision is the typed, prose-free result of the most recent
	// low-delta progress check. Optional analyzer-authored refine nodes consume
	// this through criterion.Env; scheduler code must not infer it from retry
	// hints or final-answer narrative.
	latestProgressDecision ProgressDecision

	// completionCaveats records lanes the convergence boundary
	// force-completed without resolving the blocker, for downstream
	// answer-caveat / telemetry consumers. Deduped by lane.
	completionCaveats []CompletionCaveat

	// repairs is the queue of structured RepairDirective values that
	// downstream enforcers (grounder, pre-complete check, stall
	// detector) emit when a contract violation needs to be surfaced
	// to the next explore round. Drained by ConsumeRepairs at retry-
	// hint render time so each directive fires exactly once.
	repairs []RepairDirective

	// nodeExecStatus is the M1b shadow execution ledger for IR TaskNode
	// status. During the transition graphState remains the scheduler
	// authority, but every mutation is mirrored here through typed accessors
	// so read/write loopkernel consumers can later stop depending on
	// orchestrator-private maps.
	nodeExecStatus map[string]NodeExecStatus

	// sourceInventoryObservation mirrors the typed source-class / candidate
	// universe produced by source_inventory navigation. It is a durable
	// closure-side projection so absence gates, loopkernel snapshots, and final
	// report/audit consumers can read one typed authority instead of each
	// scraping MutableState, TurnAArtifacts, or rendered repo_map text.
	sourceInventoryObservation SourceInventoryObservation

	// nodeExecAttempts is the run-local dispatch-attempt ledger for IR
	// TaskNodes. It deliberately lives next to nodeExecStatus so scheduler,
	// resume, replay, and audit consumers share one typed execution authority
	// instead of scattering retry counters through prompt prose or stage-local
	// helper maps.
	nodeExecAttempts map[string]int

	// stats accumulates CGEC enforcer fire counters across the
	// current task. Each enforcer increments its own field
	// (chain promotion → ChainsDemoted, findings_validator →
	// UnverifiedFinds, grounder → RepairsRaised, pre-complete →
	// PreCompleteDowngrades, runForcedReads → ForcedReads,
	// detectStallAndAct → StallSoftHits / StallHardHits).
	// Read by the orchestrator at task-end to emit a one-line
	// summary; reset to zero by Reset() on per-task entry.
	stats ClosureStats

	// symbolEmitRejections counts the per-Run number of
	// emit_answer_symbol items rejected by the line-anchor
	// verification (Pattern 2 line-hallucination guard inside
	// internal/tool/emit_answer_symbol.go::compileItem). P1 #3
	// (2026-05-03) consumes this as a precise structural signal
	// for runSymbolAnchorTrackOracle which emits
	// ViolSymbolAnchorMismatch when accumulation crosses
	// symbolAnchorMismatchThreshold AND the rendered AnswerDocument
	// is missing principal symbols — diagnosing a re-explore
	// situation rather than a finalize-retry one.
	//
	// Cleared by Reset() at per-task entry.
	symbolEmitRejections int

	// violations is the Session 11 F1 ViolationLedger — structured
	// per-run record of every enforcer reject that carries a
	// SuspectedRoot self-diagnosis. The F2 RootCauseAggregator
	// groups these by SuspectedRoot.IRField and the F3 IRPatchEngine
	// decides whether to reconcile the upstream IR. Distinct from
	// the existing `repairs` queue (which represents "what to do
	// next"); violations are the immutable "what happened"
	// corresponding trail. Both exist concurrently — a single
	// enforcer reject typically writes one Violation (fact) and
	// optionally one RepairDirective (remediation).
	//
	// Violations carrying only zero-value SuspectedRoot are still
	// recorded (backward compat with legacy three-field Violation
	// writers) but filtered out by the F2 aggregator's confidence
	// floor (default 0.5).
	violations []Violation

	// phase1UnreadFired latches the session-12 phase1_unread gate so
	// it produces its PendingReads + RepairExpandSearch at most once
	// per pipeline. Observed pathology: gate fires in dispatch N,
	// orchestrator's runForcedReads reads the queued files, but the
	// LLM's next emit_investigation_complete in dispatch N+1 still
	// triggers the gate because some top-K file wasn't canonicalised
	// into ReadSet (or runForcedReads failed). Re-firing doesn't add
	// information the first fire didn't already surface — it just
	// amplifies explorer redispatches. Stall soft/hard still guards
	// the "explorer keeps spinning" case.
	phase1UnreadFired bool

	// mergeBase* fields are stamped only on forked explorer closures.
	// Forks need the parent ledger in order to make in-dispatch
	// decisions, but MergeFrom must fold back only the events created
	// by the fork, not re-append the parent baseline once per worker.
	mergeBaseFingerprintLen       int
	mergeBaseViolationLen         int
	mergeBaseSymbolEmitRejections int
}

// ClosureStats is the structured per-task counter snapshot the
// orchestrator emits after each task so operators / eval harnesses
// can grep one line for invariant fire counts. All fields default
// to zero; counters increment via the corresponding Bump method
// below, which threads through the closure mutex so concurrent
// emit_*-tool calls cannot race.
//
// ExpandSearchRaised and ViewSwapRaised are separate from the
// generic RepairsRaised counter because Session 10 Group B wires
// them as first-class producers with their own semantics — operator
// observability treats "the framework asked the LLM to grep wider"
// and "the framework asked the LLM to swap answer shape" as
// distinct signals worth their own column in the task summary.
type ClosureStats struct {
	ChainsDemoted              int // I1: chains stripped from prompt by chain promotion
	UnverifiedFinds            int // I1: hallucinated paths/symbols flagged by findings_validator
	RepairsRaised              int // I2: structured RepairDirectives written to closure (any kind)
	ExpandSearchRaised         int // B1: RepairExpandSearch directives raised (Phase0 / stall / preComplete)
	ViewSwapRaised             int // B2: RepairSwapView directives raised (grounder / preComplete / retry)
	PreCompleteDowngrades      int // I3: emit_investigation_complete downgrade events
	ForcedReads                int // I4: files read on the LLM's behalf (Lazy Auto-Read)
	StallSoftHits              int // I4: convergence soft threshold hits
	StallHardHits              int // I4: convergence hard threshold hits (force-complete)
	ViolationsLogged           int // Session 11 F1: ledger entries recorded (any kind)
	AutoPairedRoleDescriptions int // Plan 2 v2 (2026-05-05): role-description mechanism evidence auto-paired by emit_evidence from leading doc-comment

	// PerStage carries stage-wise breakdown of the same activity the
	// scalar counters above record at Run scope. Block 1 (architecture
	// overhaul 2026-05-02) introduces this so downstream consumers
	// (yield delta in selective fallback, end-of-Run summary, audit
	// methodology §2.x dimensions) have a single place to ask "what
	// happened in stage X". Keys are types.PipelineStage string values
	// — "analyze" / "explore" / "extract" / "finalize" / "plan" /
	// "apply" / "verify". Empty / nil map equals "no per-stage signal
	// recorded yet"; consumers MUST treat nil == zero StageStats.
	//
	// The scalar counters above stay authoritative for whole-Run
	// aggregates so existing readers (tests pinning HasActivity, log
	// parsers grepping the legacy summary line) continue to work
	// byte-identical. PerStage is the additive layer.
	PerStage map[string]StageStats
}

// StageStats is the per-stage subset of ClosureStats. Each PipelineStage
// gets its own copy when reviewers / enforcers fire from that stage.
// Block 3's selective upstream fallback reads this map to compute
// stage-targeted yield deltas (so a finalizer-only retry's yield is
// measured against finalizer-recorded events alone, not the entire
// Run's accumulated counters).
type StageStats struct {
	Retries        int // re-dispatches of this stage during the Run
	Violations     int // closure.violations entries with Stage == this stage
	Repairs        int // closure.repairs entries originating from this stage
	Contradictions int // self-consistency-style contradictions (subset of Violations)
	PendingReads   int // PendingRead entries raised by this stage
	ForcedReads    int // forced reads attributed to this stage
}

// IsEmpty reports whether the StageStats has zero activity. Used by
// the summary renderer to skip empty rows.
func (s StageStats) IsEmpty() bool {
	return s.Retries+s.Violations+s.Repairs+s.Contradictions+s.PendingReads+s.ForcedReads == 0
}

// HasActivity returns true when at least one enforcer fired this
// task. Used by the orchestrator's emit gate so a no-op task does
// not pollute the trace with an empty summary.
func (s ClosureStats) HasActivity() bool {
	if s.ChainsDemoted+s.UnverifiedFinds+s.RepairsRaised+
		s.ExpandSearchRaised+s.ViewSwapRaised+
		s.PreCompleteDowngrades+s.ForcedReads+
		s.StallSoftHits+s.StallHardHits+
		s.ViolationsLogged+s.AutoPairedRoleDescriptions > 0 {
		return true
	}
	for _, st := range s.PerStage {
		if !st.IsEmpty() {
			return true
		}
	}
	return false
}

// PendingRead is one entry in EvidenceClosure.pendingReads. Anchor is
// the file we want to bring into ReadSet; Rationale is the prose the
// retry hint will render so the LLM understands WHY (e.g. "chain X
// anchors here but file unread"); Origin is a short tag of which
// enforcer raised it ("chain_promotion", "grounder_reject",
// "subject_constraint") so we can attribute / debug.
//
// LineRanges (optional) carries surgical 1-based [Start, End] line
// ranges the demanding gate computed. When non-empty, runForcedReads
// (CGEC E2 / Lazy Auto-Read) issues one read_file PER range with
// surgical offset/limit instead of paginating the whole file. When
// empty / nil, the historical full-file fallback is preserved so
// older PendingRead emitters (chain promotion, grounder reject,
// phase1_unread, primary_anchor) keep working byte-identically.
//
// Set by the multi-path symbol-anchored gate (multipath.EvaluateAnchor
// → applyMultiPathAnchorChecks) to translate Decision.Demand into
// surgical recovery: when stall detection forces a Lazy Auto-Read,
// only the named slivers are pulled into context, never the entire
// 4 000-line file the predecessor 30%-floor gate would have demanded.
type PendingRead struct {
	File       string
	Rationale  string
	Origin     string
	LineRanges []LineRange

	// Stage (A.1+E.1, 2026-05-02) is the explicit stage attribution for
	// this pending read. When non-empty, AddPendingRead bumps
	// stats.PerStage[Stage].PendingReads directly so StageHealthSnapshot
	// no longer needs to derive from Origin. Empty Stage preserves the
	// legacy pendingReadDerivedStage(Origin) fallback so callers that
	// haven't been migrated keep working byte-identically.
	Stage string
}

// UnverifiedFinding is one entry in EvidenceClosure.unverifiedFinds.
// Token is the literal text from the analyzer's report (a path or
// `backtick-quoted` symbol); Kind is "path" or "symbol"; Reason is
// the validator's diagnosis ("file does not exist in repo", "symbol
// not found in graph"). The findings_validator emits these and the
// renderer in context/builder.go decorates them at prompt-build time.
type UnverifiedFinding struct {
	Token  string
	Kind   string // "path" | "symbol"
	Reason string
}

// ClosureFingerprint is the snapshot used by the convergence detector
// (I4 enforcer). Two adjacent fingerprints with all four fields
// equal means an explore round did not change ReadSet, did not
// produce new evidence, did not surface new chain terminals, AND
// did not pull a new (file, line) into the citation pool —
// retrying produces the same output and the orchestrator must
// intervene (Lazy Auto-Read, force-finalize). The hashes use FNV-32
// because the values are short keys and we only compare equality.
//
// CitedRefsHash is the fourth dimension added in G7. Without it the
// convergence detector cannot spot the "LLM keeps citing the same
// (file, line) tuples across retries" pattern that emit_answer_document
// surfaces. ReadSet/Evidence/ChainTerm collectively miss that signal:
// the LLM might not read new files but might re-cite existing ones
// with different quote text — that's still a stall.
type ClosureFingerprint struct {
	ReadSetHash   uint32
	EvidenceHash  uint32
	ChainTermSet  uint32
	CitedRefsHash uint32
}

// NewEvidenceClosure constructs an empty closure. Callers should pin
// the returned pointer onto MutableState via SetEvidenceClosure once
// per Run.
//
// repoRoot threads the orchestrator's repo root through so the
// closure's internal path canonicaliser can strip an absolute
// prefix the LLM may use in read_file banners and evidence source
// fields. Pass "" when the caller does not have a meaningful
// repoRoot (unit tests) — canonicalisation then degrades to the
// historical slash-normalise + strip-"./" behaviour.
func NewEvidenceClosure(repoRoot string) *EvidenceClosure {
	return &EvidenceClosure{
		repoRoot:         repoRoot,
		readSet:          make(map[string]bool),
		readRanges:       make(map[string][]LineRange),
		fileTotalLines:   make(map[string]int),
		scannedSet:       make(map[string]bool),
		citedRefs:        make(map[string][]int),
		subjectMatches:   make(map[string]float64),
		nodeExecStatus:   make(map[string]NodeExecStatus),
		nodeExecAttempts: make(map[string]int),
	}
}

// Clone returns a deep copy of the closure. It is used by parallel
// explorer workers so each dispatch can update read coverage, repairs, and
// violations without mutating the parent Run state until the scheduler
// serially merges results in DAG order.
func (c *EvidenceClosure) Clone() *EvidenceClosure {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := NewEvidenceClosure(c.repoRoot)
	out.readSet = cloneBoolMap(c.readSet)
	out.readRanges = cloneLineRangeMap(c.readRanges)
	out.fileTotalLines = cloneIntMap(c.fileTotalLines)
	out.scannedSet = cloneBoolMap(c.scannedSet)
	out.citedRefs = cloneIntSliceMap(c.citedRefs)
	out.acceptedEvidence = cloneAcceptedEvidenceRefs(c.acceptedEvidence)
	out.pendingReads = clonePendingReads(c.pendingReads)
	out.unverifiedFinds = append([]UnverifiedFinding(nil), c.unverifiedFinds...)
	out.subjectMatches = cloneFloatMap(c.subjectMatches)
	out.fingerprints = append([]ClosureFingerprint(nil), c.fingerprints...)
	out.downgradeFingerprints = append([]DowngradeFingerprint(nil), c.downgradeFingerprints...)
	out.latestProgressDecision = c.latestProgressDecision
	out.repairs = cloneRepairDirectives(c.repairs)
	out.nodeExecStatus = cloneNodeExecStatusMap(c.nodeExecStatus)
	out.nodeExecAttempts = cloneNodeExecAttemptMap(c.nodeExecAttempts)
	out.sourceInventoryObservation = CloneSourceInventoryObservation(c.sourceInventoryObservation)
	out.stats = cloneClosureStats(c.stats)
	out.symbolEmitRejections = c.symbolEmitRejections
	out.violations = cloneViolations(c.violations)
	out.phase1UnreadFired = c.phase1UnreadFired
	out.mergeBaseFingerprintLen = c.mergeBaseFingerprintLen
	out.mergeBaseViolationLen = c.mergeBaseViolationLen
	out.mergeBaseSymbolEmitRejections = c.mergeBaseSymbolEmitRejections
	return out
}

// CloneForExploreDispatch returns a fork-ready closure snapshot. Set-like
// state is preserved so the worker can make the same decisions a serial
// dispatch would make; event counters start at zero and baseline lengths are
// stamped so MergeFrom folds back only worker-created events.
func (c *EvidenceClosure) CloneForExploreDispatch() *EvidenceClosure {
	out := c.Clone()
	if out == nil {
		return nil
	}
	out.mu.Lock()
	out.stats = ClosureStats{}
	out.mergeBaseFingerprintLen = len(out.fingerprints)
	out.mergeBaseViolationLen = len(out.violations)
	out.mergeBaseSymbolEmitRejections = out.symbolEmitRejections
	out.mu.Unlock()
	return out
}

// MergeFrom folds a forked closure back into the parent. Coverage fields
// union, telemetry fields append/add, and repair directives keep their
// existing AddRepair de-dup semantics.
func (c *EvidenceClosure) MergeFrom(other *EvidenceClosure) {
	if c == nil || other == nil {
		return
	}
	snap := other.Clone()
	if snap == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.readSet == nil {
		c.readSet = make(map[string]bool)
	}
	for f, v := range snap.readSet {
		if v {
			c.readSet[f] = true
		}
	}
	if c.readRanges == nil {
		c.readRanges = make(map[string][]LineRange)
	}
	for file, ranges := range snap.readRanges {
		combined := append(cloneLineRanges(c.readRanges[file]), ranges...)
		c.readRanges[file] = mergeLineRanges(combined)
	}
	if c.fileTotalLines == nil {
		c.fileTotalLines = make(map[string]int)
	}
	for file, total := range snap.fileTotalLines {
		if total > c.fileTotalLines[file] {
			c.fileTotalLines[file] = total
		}
	}
	if c.scannedSet == nil {
		c.scannedSet = make(map[string]bool)
	}
	for f, v := range snap.scannedSet {
		if v {
			c.scannedSet[f] = true
		}
	}
	if c.citedRefs == nil {
		c.citedRefs = make(map[string][]int)
	}
	for file, lines := range snap.citedRefs {
		c.citedRefs[file] = mergeSortedUniqueInts(c.citedRefs[file], lines)
	}
	c.acceptedEvidence = mergeAcceptedEvidenceRefs(c.acceptedEvidence, snap.acceptedEvidence)
	for _, p := range snap.pendingReads {
		c.mergePendingReadLocked(p)
	}
	c.unverifiedFinds = mergeUnverifiedFindings(c.unverifiedFinds, snap.unverifiedFinds)
	if c.subjectMatches == nil {
		c.subjectMatches = make(map[string]float64)
	}
	for k, v := range snap.subjectMatches {
		if v > c.subjectMatches[k] {
			c.subjectMatches[k] = v
		}
	}
	if base := clampMergeBaseLen(snap.mergeBaseFingerprintLen, len(snap.fingerprints)); base > 0 {
		c.fingerprints = append(c.fingerprints, snap.fingerprints[base:]...)
	} else {
		c.fingerprints = append(c.fingerprints, snap.fingerprints...)
	}
	for _, r := range snap.repairs {
		r = NormalizeRepairDirective(r)
		duplicate := false
		for i, existing := range c.repairs {
			if existing.Kind == r.Kind && existing.Subject == r.Subject && existing.Advisory == r.Advisory && sameFileSet(existing.Files, r.Files) {
				c.repairs[i].AcceptedEvidence = mergeAcceptedEvidenceRefs(existing.AcceptedEvidence, r.AcceptedEvidence)
				duplicate = true
				break
			}
		}
		if !duplicate {
			c.repairs = append(c.repairs, r)
		}
	}
	if c.nodeExecStatus == nil {
		c.nodeExecStatus = make(map[string]NodeExecStatus)
	}
	for id, status := range snap.nodeExecStatus {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		c.nodeExecStatus[id] = NormalizeNodeExecStatus(status)
	}
	if c.nodeExecAttempts == nil {
		c.nodeExecAttempts = make(map[string]int)
	}
	for id, attempts := range snap.nodeExecAttempts {
		id = strings.TrimSpace(id)
		if id == "" || attempts <= 0 {
			continue
		}
		if attempts > c.nodeExecAttempts[id] {
			c.nodeExecAttempts[id] = attempts
		}
	}
	if snap.sourceInventoryObservation.IsActive() {
		c.sourceInventoryObservation = MergeSourceInventoryObservation(c.sourceInventoryObservation, snap.sourceInventoryObservation)
	}
	c.stats = addClosureStats(c.stats, snap.stats)
	if base := clampMergeBaseLen(snap.mergeBaseSymbolEmitRejections, snap.symbolEmitRejections); base > 0 {
		c.symbolEmitRejections += snap.symbolEmitRejections - base
	} else {
		c.symbolEmitRejections += snap.symbolEmitRejections
	}
	if base := clampMergeBaseLen(snap.mergeBaseViolationLen, len(snap.violations)); base > 0 {
		c.violations = append(c.violations, cloneViolations(snap.violations[base:])...)
	} else {
		c.violations = append(c.violations, cloneViolations(snap.violations)...)
	}
	c.phase1UnreadFired = c.phase1UnreadFired || snap.phase1UnreadFired
}

// SetRepoRoot updates the closure's cached repo root. Called by
// MutableState.SetRepoRoot when the orchestrator plumbs the root
// through after construction, so any EvidenceClosure that was
// lazy-init'd with an empty root can pick up the real root
// retroactively. Idempotent.
func (c *EvidenceClosure) SetRepoRoot(root string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.repoRoot = root
	c.mu.Unlock()
}

// canonicalize reduces a file path to the closure's repo-relative
// canonical form. When the closure was built with a repoRoot, the
// platform-aware canonpath.CanonicalRepoRelative is used (handles
// Windows volumes case-insensitively, keeps POSIX slashes, strips
// an absolute prefix that matches repoRoot). Otherwise falls back
// to canonicalizeRepoPath for the historical strip-"./" +
// normalise-separators behaviour. Empty input returns empty.
func (c *EvidenceClosure) canonicalize(p string) string {
	if c == nil || p == "" {
		return canonicalizeRepoPath(p)
	}
	if c.repoRoot != "" {
		return canonpath.CanonicalRepoRelative(p, c.repoRoot)
	}
	return canonicalizeRepoPath(p)
}

// SetReadSet atomically replaces the current ReadSet snapshot. Called
// by the explorer once per round after extractFileCoverage runs.
func (c *EvidenceClosure) SetReadSet(files map[string]bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readSet = make(map[string]bool, len(files))
	for f, v := range files {
		if !v {
			continue
		}
		if canon := c.canonicalize(f); canon != "" {
			c.readSet[canon] = true
		}
	}
}

// AddReadSet merges files into the current ReadSet snapshot.
func (c *EvidenceClosure) AddReadSet(files map[string]bool) {
	if c == nil || len(files) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.readSet == nil {
		c.readSet = make(map[string]bool, len(files))
	}
	for f, v := range files {
		if !v {
			continue
		}
		if canon := c.canonicalize(f); canon != "" {
			c.readSet[canon] = true
		}
	}
}

// ReadSet returns a defensive copy of the current ReadSet so callers
// can iterate without holding the closure lock.
func (c *EvidenceClosure) ReadSet() map[string]bool {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]bool, len(c.readSet))
	for f := range c.readSet {
		out[f] = true
	}
	return out
}

// HasRead returns true when the named file is in ReadSet. Hot-path
// lookup used by the chain promotion enforcer for every chain.
func (c *EvidenceClosure) HasRead(file string) bool {
	if c == nil || file == "" {
		return false
	}
	file = c.canonicalize(file)
	if file == "" {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.readAliasLocked(file)
	return ok
}

func (c *EvidenceClosure) readAliasLocked(file string) (string, bool) {
	if c == nil || file == "" {
		return "", false
	}
	if c.readSet[file] {
		return file, true
	}
	matches := 0
	alias := ""
	for readFile := range c.readSet {
		if readFile == "" {
			continue
		}
		if evidenceClosureRelativeAliasMatch(readFile, file) {
			matches++
			alias = readFile
			if matches > 1 {
				return "", false
			}
		}
	}
	return alias, matches == 1
}

// AddReadRanges merges the supplied ranges into the existing per-file
// records. Caller semantics: "the LLM just fetched these line slices".
// Multiple calls accumulate (read_file pagination, preRead slices, a
// later forced-read all merge into one canonical sorted slice per
// file). Empty or malformed ranges are silently dropped by the merge
// pass — callers do not need to pre-validate.
func (c *EvidenceClosure) AddReadRanges(ranges map[string][]LineRange) {
	if c == nil || len(ranges) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.readRanges == nil {
		c.readRanges = make(map[string][]LineRange, len(ranges))
	}
	for file, rngs := range ranges {
		file = c.canonicalize(file)
		if file == "" || len(rngs) == 0 {
			continue
		}
		combined := append([]LineRange{}, c.readRanges[file]...)
		combined = append(combined, rngs...)
		c.readRanges[file] = mergeLineRanges(combined)
	}
}

// SetReadRanges atomically replaces the entire range map. Used by
// explorer.ParseOutput once per round, paralleling SetReadSet, so a
// fresh extractFileCoverage pass fully refreshes the snapshot without
// inheriting stale per-file ranges from a prior dispatch.
func (c *EvidenceClosure) SetReadRanges(ranges map[string][]LineRange) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readRanges = make(map[string][]LineRange, len(ranges))
	for file, rngs := range ranges {
		file = c.canonicalize(file)
		if file == "" || len(rngs) == 0 {
			continue
		}
		c.readRanges[file] = mergeLineRanges(append([]LineRange{}, rngs...))
	}
}

// RecordFileTotalLines stores the total line count for a file as
// observed in a read_file banner. Subsequent observations win when
// they are larger (defensive against banners that report a partial
// total mid-pagination — every healthy banner reports the same Z).
// Zero / negative inputs are dropped silently so callers do not need
// to validate.
func (c *EvidenceClosure) RecordFileTotalLines(file string, total int) {
	if c == nil || total <= 0 {
		return
	}
	file = c.canonicalize(file)
	if file == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fileTotalLines == nil {
		c.fileTotalLines = make(map[string]int)
	}
	if cur := c.fileTotalLines[file]; cur >= total {
		return
	}
	c.fileTotalLines[file] = total
}

// SetFileTotalLines atomically replaces the entire totals map.
// Mirrors SetReadRanges: explorer.ParseOutput refreshes the snapshot
// per dispatch from the latest extractFileCoverage walk.
func (c *EvidenceClosure) SetFileTotalLines(totals map[string]int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fileTotalLines = make(map[string]int, len(totals))
	for file, total := range totals {
		if total <= 0 {
			continue
		}
		file = c.canonicalize(file)
		if file == "" {
			continue
		}
		// Keep largest value when canonicalisation collapses two keys.
		if cur := c.fileTotalLines[file]; cur >= total {
			continue
		}
		c.fileTotalLines[file] = total
	}
}

// FileTotalLines returns the recorded total line count for a file,
// or 0 when no banner-derived total has been observed.
func (c *EvidenceClosure) FileTotalLines(file string) int {
	if c == nil || file == "" {
		return 0
	}
	file = c.canonicalize(file)
	if file == "" {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.fileTotalLines[file]
}

// MergedReadLines returns the sum of (end - start + 1) over the
// merged read ranges for a file. Zero when the file is unread or
// has no recorded ranges.
func (c *EvidenceClosure) MergedReadLines(file string) int {
	if c == nil || file == "" {
		return 0
	}
	file = c.canonicalize(file)
	if file == "" {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	sum := 0
	for _, r := range c.readRanges[file] {
		if r.End >= r.Start {
			sum += r.End - r.Start + 1
		}
	}
	return sum
}

// CoverageRatio returns merged_read_lines / total_lines for a file,
// in [0, 1]. Returns -1 when the total is unknown so callers can
// distinguish "ratio too low" from "ratio cannot be computed". A
// ratio >= 1 (e.g. ranges merged to cover all lines) is clamped to
// 1.0 — the gate semantics are "fully covered or not", not "over
// covered".
func (c *EvidenceClosure) CoverageRatio(file string) float64 {
	total := c.FileTotalLines(file)
	if total <= 0 {
		return -1
	}
	read := c.MergedReadLines(file)
	if read <= 0 {
		return 0
	}
	r := float64(read) / float64(total)
	if r > 1.0 {
		return 1.0
	}
	return r
}

// HasFullyRead reports whether merged ranges cover the entire file.
// Two-armed contract:
//
//  1. total known (FileTotalLines > 0): true iff merged_read_lines
//     >= total_lines (the banner's own ranges already merge to that).
//  2. total unknown: false. Without a denominator we cannot prove
//     "fully read"; conservative answer prevents the parity gate from
//     short-circuiting on an undersampled file.
//
// Used by the multi-path symbol-anchored verification check (and any
// future per-file coverage gate) to bypass the comparison entirely
// when a file is already fully covered — historically also the fix
// for the 113-line file being asked to cover 205 lines bug, before
// the per-file-ratio floor itself was retired in favour of symbol-
// region verification.
func (c *EvidenceClosure) HasFullyRead(file string) bool {
	total := c.FileTotalLines(file)
	if total <= 0 {
		return false
	}
	return c.MergedReadLines(file) >= total
}

// ReadRanges returns a defensive copy of the merged ranges recorded
// for `file`, or nil if none are tracked.
func (c *EvidenceClosure) ReadRanges(file string) []LineRange {
	if c == nil || file == "" {
		return nil
	}
	file = c.canonicalize(file)
	if file == "" {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneLineRanges(c.readRanges[file])
}

// HasContextAroundRegion reports whether every line in the closed
// interval [start-window, end+window] is covered by some merged read
// range for `file`. Returns false when start <= 0 or end < start
// (malformed input). Window<0 is treated as 0.
//
// Used by the multi-path symbol-anchored verification check so a
// "primary anchor symbol Y is in this file at lines L..M" hypothesis
// can be cleared by reading L-window..M+window — without demanding
// that the file as a whole be paginated.
func (c *EvidenceClosure) HasContextAroundRegion(file string, start, end, window int) bool {
	if c == nil || file == "" || start <= 0 || end < start {
		return false
	}
	if window < 0 {
		window = 0
	}
	file = c.canonicalize(file)
	if file == "" {
		return false
	}
	want := LineRange{Start: start - window, End: end + window}
	if want.Start < 1 {
		want.Start = 1
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	// EOF clamp: when the file's banner-derived total line count is
	// known, clamp want.End to the total. Without this, a window that
	// padded past EOF (e.g. symbol at line 8 + window 15 in a 10-line
	// file → want.End = 23) is uncoverable by any read because the
	// banner stops at line 10, leaving the [11, 23] tail permanently
	// uncovered. The next drain → re-raise cycle would loop until the
	// stall detector force-completes. Unknown total (== 0) preserves
	// the legacy "no clamp" behaviour so callers without banner totals
	// keep working byte-identically.
	if total := c.fileTotalLines[file]; total > 0 && want.End > total {
		want.End = total
	}
	if want.End < want.Start {
		// Region is entirely past EOF after clamp — defensively treat
		// as not coverable so the caller does not silently grant
		// satisfaction.
		return false
	}
	ranges := c.readRanges[file]
	if len(ranges) == 0 {
		return false
	}
	covered := want.Start
	for _, r := range ranges {
		if r.End < want.Start {
			continue
		}
		if r.Start > covered {
			return false
		}
		if r.End >= covered {
			covered = r.End + 1
		}
		if covered > want.End {
			return true
		}
	}
	return covered > want.End
}

// MissingContextRegions returns the minimum set of LineRange entries
// covering, for every (regionStart, regionEnd) in `regions`, the
// intersection [regionStart-window, regionEnd+window] that is NOT yet
// covered by `file`'s merged read ranges. Adjacent / overlapping
// uncovered slivers are merged so the returned slice is the smallest
// surgical-read demand that would clear HasContextAroundRegion for
// every input region.
//
// Each region is a pair of consecutive ints in `regions` —
// regions[2i]=start, regions[2i+1]=end. Malformed pairs (start<=0,
// end<start) are skipped silently. Returns an empty slice when every
// region's expanded window is already covered (caller can use this
// to detect "no demand needed").
//
// Window<0 is treated as 0. The minimum start is clamped to line 1.
//
// Used by the multi-path symbol-anchored check to build a precise
// PendingRead Rationale that names the missing slivers instead of a
// blanket "read N% of the file" demand.
func (c *EvidenceClosure) MissingContextRegions(file string, regions []int, window int) []LineRange {
	if c == nil || file == "" || len(regions) < 2 {
		return nil
	}
	if window < 0 {
		window = 0
	}
	file = c.canonicalize(file)
	if file == "" {
		return nil
	}
	c.mu.RLock()
	covered := cloneLineRanges(c.readRanges[file])
	totalLines := c.fileTotalLines[file]
	c.mu.RUnlock()

	demand := make([]LineRange, 0)
	for i := 0; i+1 < len(regions); i += 2 {
		start := regions[i]
		end := regions[i+1]
		if start <= 0 || end < start {
			continue
		}
		want := LineRange{Start: start - window, End: end + window}
		if want.Start < 1 {
			want.Start = 1
		}
		// EOF clamp: when the file's banner-derived total line count
		// is known, clamp want.End to the total so the demand never
		// names lines past EOF. Without this, a window like [80, 115]
		// against a 100-line file would never be satisfied — read_file
		// returns lines 80-100 only, the [101, 115] tail stays
		// uncovered, and the next drain → re-raise cycle loops until
		// the stall detector force-completes. Unknown total (== 0)
		// preserves the legacy "no clamp" behaviour for callers
		// without banner totals.
		if totalLines > 0 && want.End > totalLines {
			want.End = totalLines
		}
		if want.End < want.Start {
			continue
		}
		demand = append(demand, subtractCoveredRanges(want, covered)...)
	}
	if len(demand) == 0 {
		return nil
	}
	return mergeLineRanges(demand)
}

// subtractCoveredRanges returns the slivers of `want` not covered by
// any range in `covered`. Pure function; assumes `covered` is sorted
// and merged (which it is when read out of EvidenceClosure under the
// mu lock since SetReadRanges runs mergeLineRanges on insert).
func subtractCoveredRanges(want LineRange, covered []LineRange) []LineRange {
	if want.End < want.Start {
		return nil
	}
	cursor := want.Start
	out := make([]LineRange, 0)
	for _, r := range covered {
		if r.End < cursor {
			continue
		}
		if r.Start > want.End {
			break
		}
		if r.Start > cursor {
			gapEnd := r.Start - 1
			if gapEnd > want.End {
				gapEnd = want.End
			}
			out = append(out, LineRange{Start: cursor, End: gapEnd})
		}
		if r.End+1 > cursor {
			cursor = r.End + 1
		}
		if cursor > want.End {
			return out
		}
	}
	if cursor <= want.End {
		out = append(out, LineRange{Start: cursor, End: want.End})
	}
	return out
}

// HasReadLine is the row-level complement to HasRead. Semantics:
//
//  1. file not in readSet                      → false (never saw it)
//  2. file in readSet, no range records kept   → true  (backward
//     compatible: old callers that only populate readSet still grant
//     file-level coverage)
//  3. file in readSet, line within a range     → true
//  4. file in readSet, line outside all ranges → false (partial read,
//     anchor in unread section)
//
// The chain promotion enforcer opts into this API when the chain
// anchor carries a Line > 0; other ReadSet consumers (pre-complete
// gate, stall detector, F1 hierarchy promotion) keep calling HasRead
// because their semantics are "the file was touched at all", not
// "this specific line was seen".
func (c *EvidenceClosure) HasReadLine(file string, line int) bool {
	if c == nil || file == "" {
		return false
	}
	file = c.canonicalize(file)
	if file == "" {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	file, ok := c.readAliasLocked(file)
	if !ok {
		return false
	}
	if line <= 0 {
		return true
	}
	rngs := c.readRanges[file]
	if len(rngs) == 0 {
		return true
	}
	return rangesContain(rngs, line)
}

// SetScannedSet atomically replaces the ScannedSet snapshot.
func (c *EvidenceClosure) SetScannedSet(files map[string]bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scannedSet = make(map[string]bool, len(files))
	for f, v := range files {
		if !v {
			continue
		}
		// Mirror SetReadSet's canonicalization (line 425): without
		// this, a caller storing "./internal/foo.go" would not match
		// an IsScanned("internal/foo.go") lookup, treating the file
		// as a ghost path and false-warning in runForcedReads.
		// 2026-05-10 path-canonicalization audit.
		if canon := c.canonicalize(f); canon != "" {
			c.scannedSet[canon] = true
		}
	}
}

// IsScanned reports whether the explorer's keyword_search / repo_map
// pre-scan actually saw this file. Callers use this to distinguish
// "file relevant to the question (producer / chain anchor can
// legitimately reference it)" from "ghost path the LLM fabricated".
// Empty ScannedSet means the pre-scan never ran (old tests, analyzer-
// only dispatch) — in that case every query returns true so the
// historical behavior is preserved.
func (c *EvidenceClosure) IsScanned(file string) bool {
	if c == nil || file == "" {
		return false
	}
	// Mirror HasRead's canonicalization (line 452): without this,
	// IsScanned("./internal/foo.go") misses scannedSet["internal/foo.go"]
	// and the cgec D4 advisory falsely flags a real file as a ghost
	// path. 2026-05-10 path-canonicalization audit.
	canon := c.canonicalize(file)
	if canon == "" {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.scannedSet) == 0 {
		return true
	}
	return c.scannedSet[canon]
}

// ScannedSet returns a defensive copy of the scanned-file set.
func (c *EvidenceClosure) ScannedSet() map[string]bool {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.scannedSet) == 0 {
		return nil
	}
	out := make(map[string]bool, len(c.scannedSet))
	for f := range c.scannedSet {
		out[f] = true
	}
	return out
}

// AddPendingRead enqueues a forced-read directive. De-duplicates by
// File + Origin so two enforcers raising the same file do not
// double-render in the retry hint.
func (c *EvidenceClosure) AddPendingRead(p PendingRead) {
	if c == nil || p.File == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.addPendingReadLocked(p)
}

// addPendingReadLocked is the lock-held helper shared by AddPendingRead
// and the A1 bridge inside AddRepair. Caller MUST hold c.mu.
//
// Dedup-by-(File, Origin) with surgical-aware refresh: when an
// existing entry matches and EITHER side has non-empty LineRanges,
// the existing entry's LineRanges and Rationale are REPLACED by
// the incoming ones. Latest gate computation is by definition the
// most up-to-date demand — drain semantics handle stale entries
// elsewhere; the dedup must NOT silently drop a more recent
// surgical demand just because a stale entry has the same key
// (the previous bug masked the multi-path gate's iterative refinement
// of demand ranges across consecutive emit_investigation_complete
// calls within a single dispatch).
//
// When BOTH old and new have nil/empty LineRanges, the legacy
// "first-write wins" semantic is preserved (same byte-identical
// behaviour as pre-2026-05-01 callers).
func (c *EvidenceClosure) addPendingReadLocked(p PendingRead) {
	if p.File == "" {
		return
	}
	// Canonicalize at INSERT time so the dedup compare and every
	// downstream lookup (ClearPendingReadFor / runForcedReads
	// readSet match / DrainSatisfiedPendingReads) see a single
	// canonical form. Without this, two callers raising the same
	// file with different prefix forms ("./foo.go" vs "foo.go" vs
	// `foo\go`) bypass the (File, Origin) dedup and produce
	// duplicate forced-read demands. 2026-05-10
	// path-canonicalization audit.
	if canon := c.canonicalize(p.File); canon != "" {
		p.File = canon
	}
	for i, existing := range c.pendingReads {
		if existing.File != p.File || existing.Origin != p.Origin {
			continue
		}
		// Match: refresh in place when surgical info is involved.
		if len(p.LineRanges) > 0 || len(existing.LineRanges) > 0 {
			c.pendingReads[i].LineRanges = append([]LineRange(nil), p.LineRanges...)
			if p.Rationale != "" {
				c.pendingReads[i].Rationale = p.Rationale
			}
		}
		// A.1+E.1 stage attribution refresh: if the incoming carries
		// an explicit Stage and the existing entry didn't, fill it in.
		// Bumping PerStage stays one-shot (no double-count on refresh).
		if p.Stage != "" && existing.Stage == "" {
			c.pendingReads[i].Stage = p.Stage
			c.bumpStagePendingReadsLocked(p.Stage)
		}
		return
	}
	c.pendingReads = append(c.pendingReads, p)
	if p.Stage != "" {
		c.bumpStagePendingReadsLocked(p.Stage)
	}
}

// mergePendingReadLocked folds a forked PendingRead into the parent without
// changing counters. The fork's own stats already include any accounting for
// newly-created or refreshed pending reads; merge must preserve the state
// transition without charging the same event a second time.
func (c *EvidenceClosure) mergePendingReadLocked(p PendingRead) {
	if p.File == "" {
		return
	}
	if canon := c.canonicalize(p.File); canon != "" {
		p.File = canon
	}
	for i, existing := range c.pendingReads {
		if existing.File != p.File || existing.Origin != p.Origin {
			continue
		}
		if len(p.LineRanges) > 0 || len(existing.LineRanges) > 0 {
			c.pendingReads[i].LineRanges = append([]LineRange(nil), p.LineRanges...)
			if p.Rationale != "" {
				c.pendingReads[i].Rationale = p.Rationale
			}
		}
		if p.Stage != "" && existing.Stage == "" {
			c.pendingReads[i].Stage = p.Stage
		}
		return
	}
	c.pendingReads = append(c.pendingReads, p)
}

// bumpStagePendingReadsLocked is the lock-held PerStage[Stage].PendingReads
// bumper called from addPendingReadLocked. Kept separate so the
// dedup-vs-fresh-insert paths share the same accounting and a future
// reader can grep one place for the write.
func (c *EvidenceClosure) bumpStagePendingReadsLocked(stage string) {
	if stage == "" {
		return
	}
	if c.stats.PerStage == nil {
		c.stats.PerStage = make(map[string]StageStats, 4)
	}
	s := c.stats.PerStage[stage]
	s.PendingReads++
	c.stats.PerStage[stage] = s
}

// PendingReads returns a defensive copy of the queue.
func (c *EvidenceClosure) PendingReads() []PendingRead {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.pendingReads) == 0 {
		return nil
	}
	out := make([]PendingRead, len(c.pendingReads))
	copy(out, c.pendingReads)
	return out
}

// activeReadRepairsLocked compiles the current live read-file repair
// view from pendingReads. This is the authoritative runtime source
// for "which files still block closure" because pendingReads is
// drained as soon as a file enters ReadSet, while queued
// RepairReadFile directives are historical provenance that can go
// stale after the repair target moves. Caller MUST hold either the
// read or write lock.
func (c *EvidenceClosure) activeReadRepairsLocked() []RepairDirective {
	if len(c.pendingReads) == 0 {
		return nil
	}
	type groupKey struct {
		rationale string
		origin    string
		ranges    string
	}
	var order []groupKey
	filesByGroup := make(map[groupKey][]string)
	rangesByGroup := make(map[groupKey][]LineRange)
	seenFile := make(map[string]bool, len(c.pendingReads))
	for _, pending := range c.pendingReads {
		file := c.canonicalize(pending.File)
		if file == "" || seenFile[file] {
			continue
		}
		seenFile[file] = true
		ranges := cloneLineRanges(pending.LineRanges)
		key := groupKey{rationale: pending.Rationale, origin: pending.Origin, ranges: lineRangesStableKey(ranges)}
		if _, ok := filesByGroup[key]; !ok {
			order = append(order, key)
		}
		filesByGroup[key] = append(filesByGroup[key], file)
		if len(ranges) > 0 {
			rangesByGroup[key] = ranges
		}
	}
	if len(order) == 0 {
		return nil
	}
	out := make([]RepairDirective, 0, len(order))
	acceptedEvidence := cloneAcceptedEvidenceRefs(c.acceptedEvidence)
	for _, key := range order {
		files := filesByGroup[key]
		if len(files) == 0 {
			continue
		}
		out = append(out, RepairDirective{
			Kind:             RepairReadFile,
			Files:            append([]string(nil), files...),
			Rationale:        key.rationale,
			Origin:           key.origin,
			LineRanges:       cloneLineRanges(rangesByGroup[key]),
			AcceptedEvidence: cloneAcceptedEvidenceRefs(acceptedEvidence),
		})
	}
	return out
}

func lineRangesStableKey(ranges []LineRange) string {
	if len(ranges) == 0 {
		return ""
	}
	var b strings.Builder
	for i, r := range ranges {
		if i > 0 {
			b.WriteByte(';')
		}
		fmt.Fprintf(&b, "%d-%d", r.Start, r.End)
	}
	return b.String()
}

// ActiveRepairs returns the live repair surface the next retry
// should follow. Read-file repairs are compiled from pendingReads
// so the rendered hint always reflects the current closure state;
// queued RepairReadFile directives remain in the internal repairs
// ledger for provenance/debugging but are intentionally not treated
// as authoritative repair targets once pendingReads has evolved.
func (c *EvidenceClosure) ActiveRepairs() []RepairDirective {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []RepairDirective
	out = append(out, c.activeReadRepairsLocked()...)
	for _, repair := range c.repairs {
		if repair.Kind == RepairReadFile || repair.Advisory {
			continue
		}
		out = append(out, repair)
	}
	return MergeRepairs(out)
}

// ClearPendingReads (Block 3 architecture overhaul 2026-05-02)
// drops every queued PendingRead. Called by
// MutableState.ResetForFallback when a selective fallback reaches
// the explore stage so the next explorer dispatch's repair queue
// starts empty rather than re-firing prior pending reads, and by
// accepted investigation-completion boundaries where a typed closure
// has superseded support-tier forced-read debt.
//
// Distinct from ClearPendingReadFor (per-file removal) and from
// DrainSatisfiedPendingReads (selective removal of items the
// closure can prove are satisfied). This API is the all-or-
// nothing reset.
func (c *EvidenceClosure) ClearPendingReads() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	n := len(c.pendingReads)
	c.pendingReads = nil
	return n
}

// ClearPendingReadsByDebtClass removes pending reads whose structured repair
// debt class matches one of the supplied classes. It is used at accepted
// closure boundaries to drop support/advisory read debt without weakening
// principal-blocking anchors.
func (c *EvidenceClosure) ClearPendingReadsByDebtClass(classes ...RepairDebtClass) int {
	if c == nil || len(classes) == 0 {
		return 0
	}
	wanted := make(map[RepairDebtClass]bool, len(classes))
	for _, class := range classes {
		if class != "" {
			wanted[class] = true
		}
	}
	if len(wanted) == 0 {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.pendingReads) == 0 {
		return 0
	}
	kept := c.pendingReads[:0]
	cleared := 0
	for _, pending := range c.pendingReads {
		if wanted[ClassifyPendingReadRepair(pending)] {
			cleared++
			continue
		}
		kept = append(kept, pending)
	}
	c.pendingReads = kept
	return cleared
}

// ClearRepairs drops queued repair directives without touching read coverage,
// pending forced reads, violations, or statistics. It is used at accepted
// investigation-completion boundaries: a later accepted closure supersedes
// stale repair directives raised by earlier downgraded completion attempts,
// especially from sibling parallel explorer forks.
func (c *EvidenceClosure) ClearRepairs() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	n := len(c.repairs)
	c.repairs = nil
	return n
}

// ClearPendingReadFor removes every PendingRead whose File equals the
// argument. Called by the Lazy Auto-Read path after a forced read
// succeeds and by the explorer once it sees an LLM-driven read_file
// for the same path.
func (c *EvidenceClosure) ClearPendingReadFor(file string) {
	if c == nil || file == "" {
		return
	}
	// Canonicalize input so the comparison sees the same form
	// addPendingReadLocked stored. Without this, runForcedReads's
	// post-success ClearPendingReadFor with the LLM-supplied path
	// could fail to clear the pending entry (entry stored canonical,
	// argument raw with "./" prefix) and the forced-read directive
	// re-fires on the next round. 2026-05-10 audit.
	canon := c.canonicalize(file)
	if canon == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	kept := c.pendingReads[:0]
	for _, p := range c.pendingReads {
		// p.File is already canonical because addPendingReadLocked
		// canonicalizes at insert. Accept a unique active-sub-repo alias
		// too, so a user-driven read_file using the tool-resolved
		// prefixed path can clear a previously queued bare demand.
		if p.File == canon || evidenceClosureRelativeAliasMatch(p.File, canon) {
			continue
		}
		kept = append(kept, p)
	}
	c.pendingReads = kept
}

// DrainSatisfiedPendingReads removes every PendingRead the closure
// can prove is now satisfied. Two semantics, decided per entry:
//
//   - Empty LineRanges (legacy file-level demand): drained when the
//     File is in readSet — the historical "the LLM touched this file
//     at all" behaviour, used by chain promotion / grounder reject /
//     phase1_unread / primary_anchor.
//
//   - Non-empty LineRanges (surgical demand from the multi-path
//     anchor gate): drained ONLY when EVERY demanded [Start, End]
//     range is covered by merged read ranges (HasContextAroundRegion
//     with window=0). A surgical demand for lines 100-130 must NOT
//     be drained just because the LLM read lines 1-50 of the file —
//     that would let a stale, larger-than-necessary demand re-raise
//     on the next gate pass when addPendingReadLocked's
//     (File, Origin) dedup silently drops the smaller demand.
//
// Returns the count of drained entries. Used by
// preCompleteContractCheck after syncing the read snapshot from
// ground-context LineIndex (and by the multi-path anchor gate via
// the same call site): without this drain, a PendingRead enqueued
// once by an enforcer lingers across every iteration of a single
// dispatch even after the LLM reads the file on its own, because
// the only other drain site (runForcedReads) runs at window
// boundaries, not within a ReAct loop.
func (c *EvidenceClosure) DrainSatisfiedPendingReads() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.pendingReads) == 0 {
		return 0
	}
	kept := c.pendingReads[:0]
	drained := 0
	for _, p := range c.pendingReads {
		canon := c.canonicalize(p.File)
		readCanon, ok := c.readAliasLocked(canon)
		if !ok {
			kept = append(kept, p)
			continue
		}
		// File is in readSet. Decide per surgical / legacy semantic.
		if len(p.LineRanges) == 0 {
			drained++
			continue
		}
		// Surgical: every demanded range must be fully covered. Run
		// the same range-walk logic HasContextAroundRegion exposes,
		// inline here so we hold the closure mutex once for the whole
		// drain pass instead of re-acquiring per check.
		if c.everyRangeCoveredLocked(readCanon, p.LineRanges) {
			drained++
			continue
		}
		kept = append(kept, p)
	}
	c.pendingReads = kept
	return drained
}

func evidenceClosureRelativeAliasMatch(a, b string) bool {
	a = strings.TrimSpace(strings.ReplaceAll(a, `\`, `/`))
	b = strings.TrimSpace(strings.ReplaceAll(b, `\`, `/`))
	if a == "" || b == "" || evidenceClosureLooksAbsolutePath(a) || evidenceClosureLooksAbsolutePath(b) {
		return false
	}
	return strings.HasSuffix(a, "/"+b) || strings.HasSuffix(b, "/"+a)
}

func evidenceClosureLooksAbsolutePath(path string) bool {
	if strings.HasPrefix(path, "/") {
		return true
	}
	if len(path) >= 3 && path[1] == ':' && path[2] == '/' {
		return true
	}
	return false
}

// everyRangeCoveredLocked reports whether every entry in `ranges` is
// fully inside the merged read ranges for `canon` (already-canonical
// path key). Caller MUST hold c.mu (read or write).
//
// EOF clamp: when fileTotalLines[canon] is known, each demand range's
// End is treated as min(r.End, total) before checking — a demand
// that overflows EOF (e.g. ±15 ctx around a symbol near end-of-file)
// is satisfied as soon as the in-file portion is covered, never
// stuck waiting for unreadable lines past EOF. Mirrors the same
// clamp in HasContextAroundRegion + MissingContextRegions for
// cross-method consistency.
func (c *EvidenceClosure) everyRangeCoveredLocked(canon string, ranges []LineRange) bool {
	merged := c.readRanges[canon]
	if len(merged) == 0 {
		return false
	}
	totalLines := c.fileTotalLines[canon]
	for _, r := range ranges {
		if r.End < r.Start || r.Start <= 0 {
			// Malformed entry — defensively treat as "not yet covered"
			// so the PendingRead survives and re-raises with corrected
			// data on the next gate pass.
			return false
		}
		end := r.End
		if totalLines > 0 && end > totalLines {
			end = totalLines
		}
		if end < r.Start {
			// Entire range past EOF after clamp — vacuously covered
			// (nothing to read). Skip without failing.
			continue
		}
		covered := r.Start
		ok := false
		for _, m := range merged {
			if m.End < covered {
				continue
			}
			if m.Start > covered {
				return false
			}
			if m.End >= covered {
				covered = m.End + 1
			}
			if covered > end {
				ok = true
				break
			}
		}
		if !ok && covered <= end {
			return false
		}
	}
	return true
}

// AppendUnverifiedFinding records one analyzer hallucination probe
// failure. Idempotent: same Token+Kind tuple is recorded once.
func (c *EvidenceClosure) AppendUnverifiedFinding(u UnverifiedFinding) {
	if c == nil || u.Token == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, existing := range c.unverifiedFinds {
		if existing.Token == u.Token && existing.Kind == u.Kind {
			return
		}
	}
	c.unverifiedFinds = append(c.unverifiedFinds, u)
}

// UnverifiedFindings returns a defensive copy.
func (c *EvidenceClosure) UnverifiedFindings() []UnverifiedFinding {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.unverifiedFinds) == 0 {
		return nil
	}
	out := make([]UnverifiedFinding, len(c.unverifiedFinds))
	copy(out, c.unverifiedFinds)
	return out
}

// RecordCitation records a (file, line) pair that an emit_*-accepted
// citation pool referenced. Used by the convergence detector and by
// the pre-complete simulator's "do we have any cite-eligible
// evidence" check.
func (c *EvidenceClosure) RecordCitation(file string, line int) {
	if c == nil || file == "" || line <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.citedRefs == nil {
		c.citedRefs = make(map[string][]int)
	}
	for _, existing := range c.citedRefs[file] {
		if existing == line {
			return
		}
	}
	c.citedRefs[file] = append(c.citedRefs[file], line)
}

// CitedRefs returns a copy of the file → lines map.
func (c *EvidenceClosure) CitedRefs() map[string][]int {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string][]int, len(c.citedRefs))
	for f, lines := range c.citedRefs {
		dup := make([]int, len(lines))
		copy(dup, lines)
		out[f] = dup
	}
	return out
}

// AppendAcceptedEvidenceItems records accepted evidence identities in the
// closure handoff carrier. The caller supplies already-normalized EvidenceItem
// values from the authoritative accepted evidence buffer.
func (c *EvidenceClosure) AppendAcceptedEvidenceItems(items []EvidenceItem) {
	if c == nil || len(items) == 0 {
		return
	}
	refs := acceptedEvidenceRefsFromEvidenceItems(items)
	c.AppendAcceptedEvidenceRefs(refs)
}

// AppendAcceptedEvidenceRefs merges bounded accepted-evidence identities into
// the closure. It is intentionally typed and compact; downstream repair
// handoff must not reconstruct this state from model prose.
func (c *EvidenceClosure) AppendAcceptedEvidenceRefs(refs []AcceptedEvidenceRef) {
	if c == nil || len(refs) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.acceptedEvidence = mergeAcceptedEvidenceRefs(c.acceptedEvidence, refs)
}

// AcceptedEvidenceRefs returns the current bounded evidence identity carrier.
func (c *EvidenceClosure) AcceptedEvidenceRefs() []AcceptedEvidenceRef {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneAcceptedEvidenceRefs(c.acceptedEvidence)
}

func acceptedEvidenceRefsFromEvidenceItems(items []EvidenceItem) []AcceptedEvidenceRef {
	if len(items) == 0 {
		return nil
	}
	refs := make([]AcceptedEvidenceRef, 0, len(items))
	for _, item := range items {
		if ref, ok := AcceptedEvidenceRefFromEvidenceItem(item); ok {
			refs = append(refs, ref)
		}
	}
	return normalizeAcceptedEvidenceRefs(refs)
}

// SetSubjectMatch caches a chain → subject score lookup.
func (c *EvidenceClosure) SetSubjectMatch(chain string, score float64) {
	if c == nil || chain == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.subjectMatches == nil {
		c.subjectMatches = make(map[string]float64)
	}
	c.subjectMatches[chain] = score
}

// SubjectMatch returns the cached score for chain (or 0 + false).
func (c *EvidenceClosure) SubjectMatch(chain string) (float64, bool) {
	if c == nil {
		return 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.subjectMatches[chain]
	return v, ok
}

// AllSubjectMatches returns a defensive copy of the entire
// subject-match cache. Used by the CGEC E3 pre-complete check to
// compute the aggregate "did any chain meaningfully match the
// expected AnswerSubject?" signal without re-running the chain
// ranker. Empty map returns nil.
func (c *EvidenceClosure) AllSubjectMatches() map[string]float64 {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.subjectMatches) == 0 {
		return nil
	}
	out := make(map[string]float64, len(c.subjectMatches))
	for k, v := range c.subjectMatches {
		out[k] = v
	}
	return out
}

// BestSubjectMatch returns the highest score recorded in the
// subjectMatches cache, and true when at least one score exists.
// The highest-match chain's summary is the second return value.
// Useful for the pre-complete check and finalizer citation choice.
func (c *EvidenceClosure) BestSubjectMatch() (string, float64, bool) {
	if c == nil {
		return "", 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.subjectMatches) == 0 {
		return "", 0, false
	}
	var bestChain string
	var bestScore float64
	for k, v := range c.subjectMatches {
		if v > bestScore {
			bestScore = v
			bestChain = k
		}
	}
	return bestChain, bestScore, true
}

// AppendFingerprint records a per-round closure snapshot. Returns the
// updated history length so the convergence detector can decide
// whether to compare adjacent entries this round.
func (c *EvidenceClosure) AppendFingerprint(fp ClosureFingerprint) int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fingerprints = append(c.fingerprints, fp)
	return len(c.fingerprints)
}

// downgradeFingerprintWindow bounds the rolling downgrade history so a
// runaway loop cannot grow it without limit.
const downgradeFingerprintWindow = 16

// AppendDowngradeFingerprint records a pre-complete downgrade fingerprint and
// returns how many trailing fingerprints (including this one) are equal to it —
// i.e. the count of consecutive no-progress re-attempts on the same lane+blocker.
// The convergence boundary force-completes once this reaches its hard threshold.
func (c *EvidenceClosure) AppendDowngradeFingerprint(fp DowngradeFingerprint) int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.downgradeFingerprints = append(c.downgradeFingerprints, fp)
	if len(c.downgradeFingerprints) > downgradeFingerprintWindow {
		c.downgradeFingerprints = append(c.downgradeFingerprints[:0:0], c.downgradeFingerprints[len(c.downgradeFingerprints)-downgradeFingerprintWindow:]...)
	}
	return ConsecutiveEqualTail(c.downgradeFingerprints)
}

func (c *EvidenceClosure) RecordDowngradeProgressDelta(lane DowngradeLane, blockerKey uint32, hardThreshold int) ProgressDecision {
	consecutive := c.AppendDowngradeFingerprint(DowngradeFingerprint{Lane: lane, BlockerKey: blockerKey})
	decision := ShouldReplan(ProgressDelta{
		Kind:          ProgressDeltaDowngradeBlocker,
		DowngradeLane: lane,
		BlockerKey:    blockerKey,
		Consecutive:   consecutive,
		HardThreshold: hardThreshold,
	})
	if c != nil {
		c.mu.Lock()
		c.latestProgressDecision = decision
		c.mu.Unlock()
	}
	return decision
}

func (c *EvidenceClosure) LatestProgressDecision() ProgressDecision {
	if c == nil {
		return ProgressDecision{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latestProgressDecision
}

// AppendCompletionCaveat records, deduped by lane, that the convergence boundary
// force-completed a downgrade lane without resolving its blocker.
func (c *EvidenceClosure) AppendCompletionCaveat(caveat CompletionCaveat) {
	if c == nil || caveat.Lane == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, existing := range c.completionCaveats {
		if existing.Lane == caveat.Lane {
			return
		}
	}
	c.completionCaveats = append(c.completionCaveats, caveat)
}

// CompletionCaveats returns a copy of the recorded convergence completion caveats.
func (c *EvidenceClosure) CompletionCaveats() []CompletionCaveat {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.completionCaveats) == 0 {
		return nil
	}
	return append([]CompletionCaveat(nil), c.completionCaveats...)
}

// Fingerprints returns a defensive copy of the history.
func (c *EvidenceClosure) Fingerprints() []ClosureFingerprint {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.fingerprints) == 0 {
		return nil
	}
	out := make([]ClosureFingerprint, len(c.fingerprints))
	copy(out, c.fingerprints)
	return out
}

// AddRepair enqueues a RepairDirective. De-duplicates by Kind +
// sorted-Files + Subject so repeated firings of the same enforcer do
// not double-render the retry hint. Increments stats.RepairsRaised
// on every NEW (non-deduplicated) directive so the orchestrator's
// task-end summary reports a single accurate count regardless of
// caller (emit_answer_document, pre-complete, stall detector all
// flow through this single chokepoint).
//
// A1 bridge: every RepairReadFile directive simultaneously writes
// its Files onto the PendingReads queue. Without this mirror, the
// grounder's citation-drop feedback would ONLY reach the retry hint
// prompt, where an inattentive LLM can ignore or mis-read it. The
// mirror lets runForcedReads (I4) pick the same files up from the
// PendingReads queue so the framework can READ the files on the
// LLM's behalf as a fallback. ConsumeRepairs drains the Repairs
// queue but does NOT clear the mirrored PendingReads — they stay
// until runForcedReads consumes them (ClearPendingReadFor) or the
// LLM reads the file naturally (marked done via SetReadSet +
// ClearPendingReadFor from runForcedReads).
func (c *EvidenceClosure) AddRepair(r RepairDirective) {
	if c == nil || r.Kind == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.addRepairLocked(r)
}

func (c *EvidenceClosure) addRepairLocked(r RepairDirective) {
	if c == nil || r.Kind == "" {
		return
	}
	r = NormalizeRepairDirective(r)
	if len(r.AcceptedEvidence) == 0 && len(c.acceptedEvidence) > 0 {
		r.AcceptedEvidence = cloneAcceptedEvidenceRefs(c.acceptedEvidence)
	}
	for i, existing := range c.repairs {
		if existing.Kind == r.Kind && existing.Subject == r.Subject && sameFileSet(existing.Files, r.Files) {
			c.repairs[i].AcceptedEvidence = mergeAcceptedEvidenceRefs(existing.AcceptedEvidence, r.AcceptedEvidence)
			return
		}
	}
	c.repairs = append(c.repairs, r)
	c.stats.RepairsRaised++
	// A.1+E.1 stage attribution: when the directive carries an explicit
	// Stage, eagerly bump PerStage[Stage].Repairs so StageHealthSnapshot
	// no longer needs to derive. Empty Stage preserves the legacy
	// derive-on-read behaviour for un-migrated callers.
	if r.Stage != "" {
		if c.stats.PerStage == nil {
			c.stats.PerStage = make(map[string]StageStats, 4)
		}
		s := c.stats.PerStage[r.Stage]
		s.Repairs++
		c.stats.PerStage[r.Stage] = s
	}
	// Per-kind counter bumps: each RepairKind has its own column in
	// the CGEC summary line so operators can tell at a glance which
	// enforcer family is driving retries. New kinds added to
	// RepairKind MUST add a case here so TestAllRepairKindsHaveProducer
	// does not regress.
	switch r.Kind {
	case RepairReadFile:
		origin := "auto_bridge"
		if r.Origin != "" {
			origin = "auto_bridge." + r.Origin
		}
		for _, f := range r.Files {
			// Propagate the directive's LineRanges (if any) into the
			// bridged PendingRead so the surgical-demand semantic
			// survives the AddRepair → addPendingReadLocked hop.
			// Without this, any caller wiring up surgical demand via
			// AddRepair instead of AddPendingRead would silently
			// fall back to a full-file read in runForcedReads. Stage
			// (A.1+E.1) is similarly propagated so the bridged
			// PendingRead's PerStage bump matches the originating
			// directive's stage attribution.
			c.addPendingReadLocked(PendingRead{
				File:       f,
				Rationale:  r.Rationale,
				Origin:     origin,
				LineRanges: append([]LineRange(nil), r.LineRanges...),
				Stage:      r.Stage,
			})
		}
	case RepairEmitEvidence:
		// Already-read evidence materialization repairs intentionally
		// do NOT mirror into PendingReads: the fix is to emit a
		// corrected grounded evidence batch from current anchors, not
		// to widen scope with another framework-forced read.
	case RepairExpandSearch:
		c.stats.ExpandSearchRaised++
	case RepairSwapView:
		c.stats.ViewSwapRaised++
	}
}

// AppendViolation records one Session-11 F1 ledger entry. Always
// appends (no dedup) because a single retry may surface the same
// kind/field pair multiple times and the F2 aggregator relies on
// event count to weight confidence. Bumps ClosureStats.ViolationsLogged
// so the orchestrator's task-end summary reports a single accurate
// count regardless of caller.
//
// Zero-value Violation.Kind entries are rejected (defensive: a bug
// in the caller should not poison the ledger with uncategorized
// rows).
func (c *EvidenceClosure) AppendViolation(v Violation) {
	if c == nil || v.Kind == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.violations = append(c.violations, v)
	c.stats.ViolationsLogged++
	// Block 1 (2026-05-02) per-stage tally bookkeeping. Every
	// violation carrying a non-empty Stage updates the stage-wise
	// view so downstream consumers (StageHealthSnapshot, Block 3
	// fallback policy, end-of-Run summary) get the breakdown for
	// free. Empty Stage entries (legacy callers) bypass the bump
	// so the map stays clean of "" buckets.
	if v.Stage != "" {
		if c.stats.PerStage == nil {
			c.stats.PerStage = make(map[string]StageStats, 4)
		}
		s := c.stats.PerStage[v.Stage]
		s.Violations++
		if v.Kind == ViolSelfContradiction {
			s.Contradictions++
		}
		c.stats.PerStage[v.Stage] = s
	}
}

// Violations returns a defensive copy of the ledger.
func (c *EvidenceClosure) Violations() []Violation {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.violations) == 0 {
		return nil
	}
	out := make([]Violation, len(c.violations))
	copy(out, c.violations)
	return out
}

// ViolationsByField returns the subset of ledger entries whose
// SuspectedRoot.IRField equals the argument. Used by the F2
// aggregator to compute per-field heat.
func (c *EvidenceClosure) ViolationsByField(field string) []Violation {
	if c == nil || field == "" {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []Violation
	for _, v := range c.violations {
		if v.SuspectedRoot.IRField == field {
			out = append(out, v)
		}
	}
	return out
}

// ViolationsByKind returns the subset of ledger entries whose Kind
// equals the argument. Used by the F4 HintComposer when rendering
// a kind-specific allowed/forbidden hint.
func (c *EvidenceClosure) ViolationsByKind(kind ViolationKind) []Violation {
	if c == nil || kind == "" {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []Violation
	for _, v := range c.violations {
		if v.Kind == kind {
			out = append(out, v)
		}
	}
	return out
}

// DistinctViolationKindCount (A.2, 2026-05-02) returns the number
// of unique ViolationKind values in the ledger. Block 1 audit
// established that the yield-kill gate's ViolationsLogged axis was
// effectively dead because AppendViolation auto-bumps the global
// counter once per Append. Distinct-kind cardinality recovers a
// meaningful axis: the first AppendViolation of a kind raises the
// count from N→N+1, but the second Append of the SAME kind leaves
// it at N+1. So a stuck "same kind keeps re-firing" pathology now
// yields zero on this axis, and the yield-kill gate fires correctly.
//
// The cost is one O(distinct-kinds) walk per snapshot — bounded by
// AllViolationKinds() ≈ 18 entries today. Cheaper than a full
// O(violations) walk (typical violations slice ≤ 20 entries per Run
// but can reach 100+ for self-consistency-heavy answers).
func (c *EvidenceClosure) DistinctViolationKindCount() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.violations) == 0 {
		return 0
	}
	seen := make(map[ViolationKind]struct{}, len(c.violations))
	for _, v := range c.violations {
		seen[v.Kind] = struct{}{}
	}
	return len(seen)
}

// ViolationStageTally returns a histogram (stage → event count) over
// the entire ledger, keyed by Violation.Stage. Empty Stage entries
// are bucketed under "" so callers can detect "writers that forgot
// to fill Stage" — Block 3's selective fallback policy uses the
// non-empty subset to decide which stage's yield delta to measure.
func (c *EvidenceClosure) ViolationStageTally() map[string]int {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.violations) == 0 {
		return nil
	}
	tally := make(map[string]int, 4)
	for _, v := range c.violations {
		tally[v.Stage]++
	}
	return tally
}

// TopSuspectedStage returns the Stage with the highest violation
// count plus that count and the max SuspectedRoot.Confidence observed
// among events at that Stage. Empty-Stage events are skipped so the
// answer always names a real pipeline stage. Used by Block 1's
// stage-wise CGEC summary and Block 3's fallback policy when the
// dominant violation kind itself doesn't pin a stage.
func (c *EvidenceClosure) TopSuspectedStage() (stage string, count int, maxConf float64) {
	if c == nil {
		return "", 0, 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.violations) == 0 {
		return "", 0, 0
	}
	perStage := make(map[string]int)
	perStageConf := make(map[string]float64)
	for _, v := range c.violations {
		if v.Stage == "" {
			continue
		}
		perStage[v.Stage]++
		if v.SuspectedRoot.Confidence > perStageConf[v.Stage] {
			perStageConf[v.Stage] = v.SuspectedRoot.Confidence
		}
	}
	for s, n := range perStage {
		if n > count {
			count = n
			stage = s
			maxConf = perStageConf[s]
		}
	}
	return stage, count, maxConf
}

// StageHealthSnapshot returns a copy of stats.PerStage merged with the
// stage-wise tallies derived from pendingReads[]. The Violations /
// Contradictions counters come straight from stats.PerStage (writers
// bump them inside AppendViolation, see the Block 1 contract on
// AppendViolation). PendingReads is computed at read time because
// PendingRead structs do not carry Stage today (Origin → stage
// derivation is the helper below). Block 3's yield delta calls this
// before/after every fallback dispatch.
//
// IMPORTANT: this method returns a SNAPSHOT, not a live view. Mutations
// after the call do not propagate to the returned map.
func (c *EvidenceClosure) StageHealthSnapshot() map[string]StageStats {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]StageStats, 4)
	// Trust stats.PerStage as the authoritative source for
	// Retries / Violations / Contradictions / Repairs. Writers
	// (AppendViolation / IncrementStageRetry / AddRepair) keep it
	// in sync with the underlying ledger.
	for stage, st := range c.stats.PerStage {
		out[stage] = st
	}
	// PendingReads attribution (A.1+E.1, 2026-05-02 update):
	//
	// 1) Entries with explicit p.Stage have already been counted at
	//    AddPendingRead time via bumpStagePendingReadsLocked, so they
	//    are present in c.stats.PerStage above. Skip here to avoid
	//    double-count.
	// 2) Entries with empty p.Stage (legacy callers not yet migrated)
	//    fall through to the historical pendingReadDerivedStage
	//    Origin-string mapping. This branch shrinks as callers adopt
	//    the explicit Stage field.
	for _, p := range c.pendingReads {
		if p.Stage != "" {
			continue // already accounted in PerStage
		}
		stage := pendingReadDerivedStage(p)
		s := out[stage]
		s.PendingReads++
		out[stage] = s
	}
	return out
}

// IncrementStageRetry bumps StageStats.Retries for the named stage.
// Called by the orchestrator's per-stage retry sites so the stage-wise
// snapshot always reflects "how often did stage X re-dispatch this
// Run". Empty stage is silently dropped so a typo at the call-site
// can't pollute the map with "" buckets.
func (c *EvidenceClosure) IncrementStageRetry(stage string) {
	if c == nil || stage == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stats.PerStage == nil {
		c.stats.PerStage = make(map[string]StageStats, 4)
	}
	s := c.stats.PerStage[stage]
	s.Retries++
	c.stats.PerStage[stage] = s
}

// IncrementSymbolEmitRejection bumps the per-Run counter of
// emit_answer_symbol items rejected by the line-anchor verification
// inside the tool (Pattern 2 line-hallucination guard). The counter
// feeds runSymbolAnchorTrackOracle (P1 #3) which emits
// ViolSymbolAnchorMismatch when accumulation crosses the threshold —
// signalling that the explorer never surfaced def-region lines for
// the user's principal entities and a BackToExplore fallback is
// warranted.
func (c *EvidenceClosure) IncrementSymbolEmitRejection() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.symbolEmitRejections++
}

// SymbolEmitRejections returns the current rejection counter.
func (c *EvidenceClosure) SymbolEmitRejections() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.symbolEmitRejections
}

// pendingReadDerivedStage maps a PendingRead.Origin string to the
// most likely stage name. Pure helper, no closure access — called
// inside StageHealthSnapshot under RLock so it must be lock-free.
// The mapping is a best-effort derivation; a future PendingRead
// schema upgrade could carry Stage explicitly.
func pendingReadDerivedStage(p PendingRead) string {
	switch p.Origin {
	case "chain_promotion", "subject_constraint", "phase1_unread", "primary_anchor", "multipath":
		return string(StageExplore)
	case "grounder_reject", "pre_complete.citation_floor_low", "pre_complete.explanation_anchor_skeleton":
		return string(StageFinalize)
	}
	return string(StageExplore)
}

// ViolationFieldTally returns a histogram (field → event count)
// over the entire ledger. The F2 aggregator consumes this to drive
// the patch-threshold decision; operators can also hand-inspect it
// via the extended CGEC summary line.
func (c *EvidenceClosure) ViolationFieldTally() map[string]int {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.violations) == 0 {
		return nil
	}
	tally := make(map[string]int, len(c.violations))
	for _, v := range c.violations {
		field := v.SuspectedRoot.IRField
		if field == "" {
			continue // zero-root events do not feed per-field aggregation
		}
		tally[field]++
	}
	return tally
}

// TopSuspectedField returns the IRField with the highest count in
// the ledger, along with that count and the maximum Confidence
// observed among events for that field. Used by the orchestrator's
// fail-loud warning (F5) when the retry budget is exhausted.
func (c *EvidenceClosure) TopSuspectedField() (field string, count int, maxConf float64) {
	if c == nil {
		return "", 0, 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.violations) == 0 {
		return "", 0, 0
	}
	perField := make(map[string]int)
	perFieldConf := make(map[string]float64)
	for _, v := range c.violations {
		f := v.SuspectedRoot.IRField
		if f == "" {
			continue
		}
		perField[f]++
		if v.SuspectedRoot.Confidence > perFieldConf[f] {
			perFieldConf[f] = v.SuspectedRoot.Confidence
		}
	}
	for f, n := range perField {
		if n > count {
			count = n
			field = f
			maxConf = perFieldConf[f]
		}
	}
	return field, count, maxConf
}

// BumpViolationsLogged exposes the counter increment so the F3
// IRPatchEngine's audit-trail writes (which also emit structured
// records but not through AppendViolation's path) can bump the
// unified counter.
func (c *EvidenceClosure) BumpViolationsLogged(n int) {
	c.bumpStat(func(s *ClosureStats) { s.ViolationsLogged += n })
}

// PendingRepairs returns a defensive copy of the queue WITHOUT
// draining it. Use ConsumeRepairs to drain.
func (c *EvidenceClosure) PendingRepairs() []RepairDirective {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.repairs) == 0 {
		return nil
	}
	return cloneRepairDirectives(c.repairs)
}

// ConsumeRepairs returns and clears the queue in one atomic step. The
// retry-hint renderer calls this so each directive surfaces in
// exactly one prompt.
func (c *EvidenceClosure) ConsumeRepairs() []RepairDirective {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []RepairDirective
	out = append(out, c.activeReadRepairsLocked()...)
	for _, repair := range c.repairs {
		if repair.Kind == RepairReadFile || repair.Advisory {
			continue
		}
		out = append(out, repair)
	}
	c.repairs = nil
	return MergeRepairs(out)
}

func (c *EvidenceClosure) RecordSourceInventoryObservation(observation SourceInventoryObservation) {
	if c == nil || !observation.IsActive() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sourceInventoryObservation = MergeSourceInventoryObservation(c.sourceInventoryObservation, observation)
}

func (c *EvidenceClosure) SourceInventoryObservation() SourceInventoryObservation {
	if c == nil {
		return SourceInventoryObservation{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return CloneSourceInventoryObservation(c.sourceInventoryObservation)
}

func (c *EvidenceClosure) ClearSourceInventoryObservation() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sourceInventoryObservation = SourceInventoryObservation{}
}

// Reset wipes the closure back to NewEvidenceClosure() state. Called
// by MutableState.ResetEvidenceClosure at task entry.
//
// Block 1 audit (2026-05-02) — Reset hygiene contract:
//
//	The struct re-assignment `c.stats = ClosureStats{}` clears every
//	new ClosureStats field for free, including the PerStage map added
//	by Block 1. Adding a new top-level closure FIELD (not a sub-field
//	on stats) is what requires explicit zeroing here — pre-Block-1
//	the fileTotalLines line was missing (covered now), the readRanges
//	denominator that the multipath gate relied on. Future contributors
//	adding a new field MUST add a line here too; the test suite's
//	TestEvidenceClosureReset_AllFieldsCleared (B1-T1.9) pins the rule.
func (c *EvidenceClosure) Reset() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readSet = make(map[string]bool)
	c.readRanges = make(map[string][]LineRange)
	c.fileTotalLines = make(map[string]int)
	c.scannedSet = make(map[string]bool)
	c.citedRefs = make(map[string][]int)
	c.acceptedEvidence = nil
	c.pendingReads = nil
	c.unverifiedFinds = nil
	c.subjectMatches = make(map[string]float64)
	c.fingerprints = nil
	c.downgradeFingerprints = nil
	c.latestProgressDecision = ProgressDecision{}
	c.repairs = nil
	c.nodeExecStatus = make(map[string]NodeExecStatus)
	c.nodeExecAttempts = make(map[string]int)
	c.sourceInventoryObservation = SourceInventoryObservation{}
	c.violations = nil
	c.stats = ClosureStats{}
	c.phase1UnreadFired = false
	c.symbolEmitRejections = 0
}

// Phase1UnreadFired reports whether the session-12 phase1_unread
// gate has already produced its PendingReads + RepairExpandSearch in
// the current pipeline. Callers use the latch to skip a second
// firing; see the field comment on EvidenceClosure for rationale.
func (c *EvidenceClosure) Phase1UnreadFired() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.phase1UnreadFired
}

// MarkPhase1UnreadFired sets the phase1_unread latch. Idempotent.
func (c *EvidenceClosure) MarkPhase1UnreadFired() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.phase1UnreadFired = true
}

// Stats returns a snapshot of the per-task counters. Cheap (struct
// copy under read lock) so callers can poll across enforcer
// invocations.
func (c *EvidenceClosure) Stats() ClosureStats {
	if c == nil {
		return ClosureStats{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// BumpChainsDemoted / BumpUnverifiedFinds / etc. are typed counter
// increments. Each takes a delta (always positive in practice) so
// batch operations (e.g. one applyChainPromotion call demoting N
// chains) increment in one mutex acquire.

func (c *EvidenceClosure) BumpChainsDemoted(n int) {
	c.bumpStat(func(s *ClosureStats) { s.ChainsDemoted += n })
}
func (c *EvidenceClosure) BumpUnverifiedFinds(n int) {
	c.bumpStat(func(s *ClosureStats) { s.UnverifiedFinds += n })
}
func (c *EvidenceClosure) BumpRepairsRaised(n int) {
	c.bumpStat(func(s *ClosureStats) { s.RepairsRaised += n })
}
func (c *EvidenceClosure) BumpExpandSearchRaised(n int) {
	c.bumpStat(func(s *ClosureStats) { s.ExpandSearchRaised += n })
}
func (c *EvidenceClosure) BumpViewSwapRaised(n int) {
	c.bumpStat(func(s *ClosureStats) { s.ViewSwapRaised += n })
}
func (c *EvidenceClosure) BumpPreCompleteDowngrades(n int) {
	c.bumpStat(func(s *ClosureStats) { s.PreCompleteDowngrades += n })
}
func (c *EvidenceClosure) BumpForcedReads(n int) {
	c.bumpStat(func(s *ClosureStats) { s.ForcedReads += n })
}

// BumpForcedReadsForStage (A.1+E.1, 2026-05-02) bumps the global
// ForcedReads counter AND attributes the bump to a named stage's
// PerStage[stage].ForcedReads. cgec_enforcers.go's runForcedReads
// loop calls this with each PendingRead's Stage so the stage breakdown
// is accurate without snapshot-time derivation. Empty stage degrades
// to BumpForcedReads — the global counter still moves but the
// PerStage row is left alone.
func (c *EvidenceClosure) BumpForcedReadsForStage(stage string, n int) {
	if c == nil || n == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats.ForcedReads += n
	if stage == "" {
		return
	}
	if c.stats.PerStage == nil {
		c.stats.PerStage = make(map[string]StageStats, 4)
	}
	s := c.stats.PerStage[stage]
	s.ForcedReads += n
	c.stats.PerStage[stage] = s
}
func (c *EvidenceClosure) BumpStallSoftHits(n int) {
	c.bumpStat(func(s *ClosureStats) { s.StallSoftHits += n })
}
func (c *EvidenceClosure) BumpAutoPairedRoleDescriptions(n int) {
	c.bumpStat(func(s *ClosureStats) { s.AutoPairedRoleDescriptions += n })
}
func (c *EvidenceClosure) BumpStallHardHits(n int) {
	c.bumpStat(func(s *ClosureStats) { s.StallHardHits += n })
}

func (c *EvidenceClosure) bumpStat(mut func(*ClosureStats)) {
	if c == nil || mut == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	mut(&c.stats)
}
