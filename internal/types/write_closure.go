package types

import (
	"sync"
	"time"
)

// WriteClosure is the per-Run cross-stage tracker for the four
// CGEC-W (Citation-Grounded Write Closure) invariants — the
// write-phase mirror of EvidenceClosure's I1-I4. Lives on
// MutableState, reset per-task alongside EvidenceClosure so
// plan/apply/verify share a single view without cross-task
// contamination.
//
//   W1: every file:line in an applied patch must be declared up-front
//       in the ChangePlan's target_paths (apply-stage pre-flight
//       compares patch scope to plan.target_paths and refuses to run
//       when the patch reaches outside).
//
//   W2: the verify stage's executed test set must cover (be a
//       superset of) the plan's AcceptanceTests; scheduler keeps
//       requeuing until the superset relation holds.
//
//   W3: apply and verify run inside a git worktree; BusContext.RepoRoot
//       swaps to WorktreePath during those stages, swaps back to
//       MainRepoRoot at finalize. Main repo HEAD bytes never change
//       inside B0.
//
//   W4: between two verify retries at least one of (appliedSet,
//       verifySet, failedAssertions) must change — otherwise apply
//       is in a repairable failure loop and the scheduler force-
//       finalizes via the the verify stage hook retry budget (mirror of I4
//       stall detection).
//
// B0 scope: WriteClosure carries the shape (fields, mutex, getters)
// so the evaluator functions in internal/analysis/criterion/eval.go
// can consult it, but the enforcer functions that actually populate
// the fields live in B2 (apply tool) and B3 (verify tool). In B0 all
// fields default to zero-value and the four criteria evaluators
// return Satisfied=true unconditionally — the structural wiring is
// in place for B2/B3 to fill without a data-model migration.
//
// Concurrency: own mutex (mirror of EvidenceClosure pattern) so the
// write stages do not serialize through MutableState.mu when the
// scheduler dispatches multiple write-phase nodes in sequence.
type WriteClosure struct {
	mu sync.RWMutex

	// appliedSet is the canonical set of repo-relative file paths
	// whose content apply_patch successfully wrote into the worktree.
	// Mirror of EvidenceClosure.readSet. Populated by the apply tool
	// on success; drained to zero by Reset() on per-task entry.
	appliedSet map[string]bool

	// pendingApplies is the queue of ChangeUnits planner emitted that
	// the apply stage has not yet executed. Mirror of
	// EvidenceClosure.pendingReads. Populated by emit_change_plan
	// (plan stage output) and drained by apply_patch.Execute as each
	// unit lands.
	pendingApplies []PendingApply

	// writeRepairs is the queue of structured WriteRepairDirective
	// values that the apply / verify enforcers emit when a contract
	// violation needs to be surfaced to the next apply or verify
	// round. Mirror of EvidenceClosure.repairs. Drained by the
	// scheduler's retry-hint builder so each directive fires exactly
	// once.
	writeRepairs []WriteRepairDirective

	// verifySet records the test assertions the verify stage
	// successfully evaluated (pass OR fail — both count as "the
	// verifier ran this test"). Keyed by assertion-id from the
	// plan's AcceptanceTests or discovered test-suite output. Used
	// by CritTestsPass and CritNoRegression evaluators and by the
	// W2 coverage-superset check.
	verifySet map[string]VerifyResult

	// fingerprints is the rolling history of ApplyVerifyFingerprint
	// values, one entry per apply-verify round. The W4 convergence
	// detector compares the latest two entries to spot stalls.
	fingerprints []ApplyVerifyFingerprint

	// stats accumulates WriteClosure enforcer fire counters across
	// the current task. Populated by the B2/B3 enforcers; read by
	// the orchestrator at task-end for the one-line summary.
	stats WriteClosureStats
}

// NewWriteClosure constructs a zero-state WriteClosure. The mutex
// ships already usable (zero-value sync.RWMutex is a valid
// unlocked mutex).
func NewWriteClosure() *WriteClosure {
	return &WriteClosure{
		appliedSet: make(map[string]bool),
		verifySet:  make(map[string]VerifyResult),
	}
}

// Reset clears every field back to zero-value. Called by
// MutableState.ResetWriteClosure at the start of each task so the
// previous task's applied patches / verify results cannot leak into
// a fresh plan→apply→verify cycle.
func (c *WriteClosure) Reset() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.appliedSet = make(map[string]bool)
	c.pendingApplies = nil
	c.writeRepairs = nil
	c.verifySet = make(map[string]VerifyResult)
	c.fingerprints = nil
	c.stats = WriteClosureStats{}
}

// AppliedSet returns a shallow copy of the applied file set so
// callers can iterate without holding the closure lock. Mirrors
// EvidenceClosure.ReadSet.
func (c *WriteClosure) AppliedSet() map[string]bool {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]bool, len(c.appliedSet))
	for k, v := range c.appliedSet {
		out[k] = v
	}
	return out
}

// HasApplied reports whether the apply stage committed a change to
// the given file in the current task's worktree.
func (c *WriteClosure) HasApplied(file string) bool {
	if c == nil || file == "" {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.appliedSet[file]
}

// MarkApplied records that apply_patch successfully wrote content
// to file. Idempotent. Called from apply_patch.Execute on success.
func (c *WriteClosure) MarkApplied(file string) {
	if c == nil || file == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.appliedSet == nil {
		c.appliedSet = make(map[string]bool)
	}
	c.appliedSet[file] = true
}

// PendingApplies returns the current queue (copy).
func (c *WriteClosure) PendingApplies() []PendingApply {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]PendingApply, len(c.pendingApplies))
	copy(out, c.pendingApplies)
	return out
}

// EnqueuePendingApply appends a ChangeUnit to the apply queue. Called
// by emit_change_plan when the planner finalizes its output.
func (c *WriteClosure) EnqueuePendingApply(p PendingApply) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pendingApplies = append(c.pendingApplies, p)
}

// VerifyResults returns a shallow copy of the verify-set map.
func (c *WriteClosure) VerifyResults() map[string]VerifyResult {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]VerifyResult, len(c.verifySet))
	for k, v := range c.verifySet {
		out[k] = v
	}
	return out
}

// RecordVerify stores one assertion-id → result mapping. Called by
// emit_test_results (B3). Overwrites prior result for the same
// assertion (verify-stage retry semantics).
func (c *WriteClosure) RecordVerify(assertionID string, r VerifyResult) {
	if c == nil || assertionID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.verifySet == nil {
		c.verifySet = make(map[string]VerifyResult)
	}
	c.verifySet[assertionID] = r
}

// ConsumeRepairs atomically drains the current repair queue,
// returning the slice and resetting the internal slice to nil.
// Mirror of EvidenceClosure.ConsumeRepairs — each directive fires
// exactly once when the retry-hint renderer picks it up.
func (c *WriteClosure) ConsumeRepairs() []WriteRepairDirective {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.writeRepairs
	c.writeRepairs = nil
	return out
}

// RaiseRepair appends a directive to the repair queue. Called by
// apply / verify enforcers when they detect a violation that needs
// LLM-visible remediation.
func (c *WriteClosure) RaiseRepair(d WriteRepairDirective) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writeRepairs = append(c.writeRepairs, d)
}

// Stats returns a copy of the current counter snapshot.
func (c *WriteClosure) Stats() WriteClosureStats {
	if c == nil {
		return WriteClosureStats{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// AppendFingerprint records a new convergence-detection snapshot.
// The W4 enforcer compares the last two entries to detect apply-
// verify stall.
func (c *WriteClosure) AppendFingerprint(fp ApplyVerifyFingerprint) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fingerprints = append(c.fingerprints, fp)
}

// Fingerprints returns a copy of the full convergence history.
func (c *WriteClosure) Fingerprints() []ApplyVerifyFingerprint {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]ApplyVerifyFingerprint, len(c.fingerprints))
	copy(out, c.fingerprints)
	return out
}

// PendingApply is one ChangeUnit in WriteClosure.pendingApplies.
// The planner emits a list of these via emit_change_plan; the apply
// stage drains them one-by-one as apply_patch.Execute runs.
//
// Mirror of EvidenceClosure.PendingRead. Path is the repo-relative
// file to modify; Rationale is the planner's prose explanation (used
// in the retry-hint render if the unit fails); Origin tags which
// planner pass emitted it.
type PendingApply struct {
	Path      string
	Rationale string
	Origin    string
}

// WriteRepairDirective is one entry in WriteClosure.writeRepairs.
// Mirror of EvidenceClosure.RepairDirective. Kind names the
// repair class ("rollback_file", "retry_apply",
// "reconsider_plan"); Path is the affected file when applicable;
// Detail is the prose payload.
type WriteRepairDirective struct {
	Kind   string
	Path   string
	Detail string
}

// VerifyResult is the pass/fail verdict the verify stage records
// against a single AcceptanceTest or observed test-suite assertion.
// Stored in WriteClosure.verifySet keyed by assertion-id.
type VerifyResult struct {
	Passed   bool
	Reason   string        // human-readable failure detail; empty on pass
	Duration time.Duration // observed execution time
}

// ApplyVerifyFingerprint is one snapshot used by the W4 convergence
// detector. Apply and verify enforcers append a fingerprint per
// round; the stall detector compares adjacent entries to decide
// whether the system made progress.
type ApplyVerifyFingerprint struct {
	AppliedCount  int
	VerifyPassed  int
	VerifyFailed  int
	Timestamp     time.Time
}

// WriteClosureStats accumulates the enforcer fire counters for one
// task. Mirror of ClosureStats. Every field defaults to zero and is
// incremented by the B2/B3 enforcers via dedicated Bump methods
// (added when enforcers land).
type WriteClosureStats struct {
	PlansGenerated     int // plan stage: ChangePlan artifacts emitted
	AppliesCommitted   int // apply stage: ChangeUnits successfully applied
	VerifiesPassed     int // verify stage: assertions that passed
	VerifiesFailed     int // verify stage: assertions that failed
	RollbacksTriggered int // apply stage: per-unit rollbacks
	WriteRepairsRaised int // any enforcer: repairs written to queue
}

// HasActivity returns true when at least one write-phase enforcer
// fired this task. Mirror of ClosureStats.HasActivity; gates the
// one-line orchestrator summary emission.
func (s WriteClosureStats) HasActivity() bool {
	return s.PlansGenerated+s.AppliesCommitted+
		s.VerifiesPassed+s.VerifiesFailed+
		s.RollbacksTriggered+s.WriteRepairsRaised > 0
}

// MutableState accessors below — co-located with the rest of
// WriteClosure-aware code (same pattern EvidenceClosure uses at
// the end of evidence_closure.go). Go allows methods on a type
// defined in any file of the same package.

// WriteClosure returns the per-Run write-phase closure tracker.
// Lazily initialized: the first reader installs a fresh instance
// so callers never have to nil-check. Mirror of
// MutableState.EvidenceClosure(). Read-mode Runs never touch this
// method and the nil pointer stays — tools that call it in read
// mode (which they shouldn't, per the L3 red line) will still get
// a working zero-state closure.
func (m *MutableState) WriteClosure() *WriteClosure {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	if m.writeClosure != nil {
		c := m.writeClosure
		m.mu.RUnlock()
		return c
	}
	m.mu.RUnlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeClosure == nil {
		m.writeClosure = NewWriteClosure()
	}
	return m.writeClosure
}

// SetWriteClosure atomically replaces the closure pointer. Pass nil
// to clear (the next WriteClosure() call will lazy-init). Primarily
// used by tests that want to inject a pre-seeded closure state.
func (m *MutableState) SetWriteClosure(c *WriteClosure) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeClosure = c
}

// ResetWriteClosure clears the closure at task entry. Mirror of
// ResetEvidenceClosure: the per-task scheduler loop calls this so
// write-phase state from a prior task cannot bleed into a fresh
// plan→apply→verify cycle. Safe to call when the closure is nil
// (no-op).
func (m *MutableState) ResetWriteClosure() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeClosure != nil {
		m.writeClosure.Reset()
	}
}
