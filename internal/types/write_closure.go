package types

import (
	"strings"
	"sync"
	"time"
)

// WriteClosure is the per-Run cross-stage tracker for write-phase
// invariants — the write-mode peer to EvidenceClosure. Lives on
// MutableState, reset per-task alongside EvidenceClosure so
// plan/apply/verify share a single view without cross-task
// contamination.
//
//	W1: every file:line in an applied patch must be declared up-front
//	    in the ChangePlan's target_paths. Enforced at apply_patch.Execute
//	    — patches reaching outside TargetPaths are rejected.
//
//	W1b: every ChangeUnit.DependsOn entry must already be in
//	     appliedSet before that unit is applied. Enforced at
//	     apply_patch.Execute alongside W1.
//
//	W3: apply and verify run inside a git worktree; BusContext.RepoRoot
//	    swaps to WorktreePath during those stages, swaps back to
//	    MainRepoRoot at finalize. Main repo HEAD bytes never change.
//	    Enforced via stage_hooks.go provisioning.
//
//	W4: between two verify retries at least one fingerprint field
//	    (AppliedCount / VerifyPassed / VerifyFailed / FailureSummaryHash)
//	    must change — otherwise the planner is structurally unable to
//	    fix from the signal it has and further retry just burns LLM
//	    budget. Enforced via Fingerprints() comparison in controller repair
//	    paths.
//
// (The original B0 design also tracked W2 — verify-set coverage of
// AcceptanceTests — via a verifySet map and a stats counter struct
// the B2/B3 enforcers were going to populate. Those enforcers never
// landed; verify outcome flows through ChangeReport.TestResults and
// the criterion evaluators at internal/analysis/criterion/eval.go
// read that directly. The map and stats struct were removed once
// they had no producers.)
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

	// fingerprints is the rolling history of ApplyVerifyFingerprint
	// values, one entry per apply-verify round. The W4 convergence
	// detector compares the latest two entries to spot stalls.
	fingerprints []ApplyVerifyFingerprint

	// rejectionCounts tracks how many CONSECUTIVE times apply_patch
	// rejected a given path on W1 (path not in TargetPaths) or W1b
	// (DependsOn unsatisfied). Cleared per-path by MarkApplied
	// (forward progress wipes the slate). When any path's count
	// reaches the threshold, SaturatedRejectionPath() returns it
	// and the coder ShouldStop predicate fast-fails with a plan-
	// defect signal — repeating the same illegal apply 3+ times in
	// a row is structurally a planner mistake (TargetPaths is too
	// narrow / wrong), not a coder typo, and burning more coder
	// iterations on it just delays the right escalation.
	rejectionCounts map[string]int
}

// NewWriteClosure constructs a zero-state WriteClosure. The mutex
// ships already usable (zero-value sync.RWMutex is a valid
// unlocked mutex).
func NewWriteClosure() *WriteClosure {
	return &WriteClosure{
		appliedSet: make(map[string]bool),
	}
}

// Reset clears every field back to zero-value. Called by
// MutableState.ResetWriteClosure at the start of each task so the
// previous task's applied patches cannot leak into a fresh
// plan→apply→verify cycle.
func (c *WriteClosure) Reset() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.appliedSet = make(map[string]bool)
	c.pendingApplies = nil
	c.fingerprints = nil
	c.rejectionCounts = nil
}

// ResetExceptApplied clears every per-cycle queue (pendingApplies,
// fingerprints, rejectionCounts) BUT preserves appliedSet. Used by
// clearForReplan when the verify→plan retry path warm-rewinds the
// worktree to the previous apply's commit SHA: the worktree disk
// state still carries the applied files, so AppliedSet must stay
// aligned with disk reality. Resetting the whole closure on warm
// rewind would leave apply_patch's idempotency check (which reads
// AppliedSet) blind to files that physically exist, so the next plan
// retry would re-emit kind=create for an existing file and apply_patch
// would reject "file already exists in worktree".
func (c *WriteClosure) ResetExceptApplied() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pendingApplies = nil
	c.fingerprints = nil
	c.rejectionCounts = nil
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
// RecordRejection bumps the per-path apply-rejection counter. Called
// by apply_patch.Execute when a W1 (path not in TargetPaths) or W1b
// (DependsOn unsatisfied) check rejects a tool call. After repeated
// rejections of the SAME path the coder ShouldStop predicate
// surfaces a plan-defect signal so the verify→plan retry hint can
// tell the planner "your TargetPaths excludes <path> the LLM keeps
// trying" — that's a planner-side fix (widen TargetPaths or rewrite
// the plan), not something more coder iterations will solve.
//
// Counts are per-path; a different path resets that path's counter
// independently. MarkApplied(path) clears path's counter (forward
// progress).
func (c *WriteClosure) RecordRejection(path string) int {
	if c == nil || strings.TrimSpace(path) == "" {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rejectionCounts == nil {
		c.rejectionCounts = make(map[string]int)
	}
	c.rejectionCounts[path]++
	return c.rejectionCounts[path]
}

// RejectionCount returns the current consecutive-rejection count for
// path. Zero when MarkApplied has cleared it or the path was never
// rejected. Used by tests to verify the counter state directly;
// production code reads SaturatedRejectionPath instead.
func (c *WriteClosure) RejectionCount(path string) int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rejectionCounts[path]
}

// SaturatedRejectionPath returns the first path whose rejection
// count has reached threshold (the "plan defect" signal). Empty
// string means no path saturated. Threshold is fixed at 3 — that's
// "LLM tried, got the rejection summary explaining valid TargetPaths,
// tried again, got it again, tried a third time" which is a clear
// signal the LLM is fixated and the plan's TargetPaths is the real
// issue.
func (c *WriteClosure) SaturatedRejectionPath() string {
	const threshold = 3
	if c == nil {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for path, n := range c.rejectionCounts {
		if n >= threshold {
			return path
		}
	}
	return ""
}

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
	// Forward progress: any successful apply clears EVERY rejection
	// counter — the LLM is moving (a different file landed even
	// when X kept rejecting), so we don't want a stale W1 reject
	// from earlier in the loop to trip SaturatedRejectionPath()
	// after a clean apply has happened.
	c.rejectionCounts = nil
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

// ReplacePendingApplies swaps the pending queue atomically. Online write-mode
// uses this when an active slice narrows the apply surface: the planner may
// have emitted a full-plan queue, but the controller only authorizes the
// current slice for this edit/run/observe cycle.
func (c *WriteClosure) ReplacePendingApplies(pending []PendingApply) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pendingApplies = append([]PendingApply(nil), pending...)
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

// ApplyVerifyFingerprint is one snapshot used by the W4 convergence
// detector. Apply and verify enforcers append a fingerprint per
// round; the stall detector compares adjacent entries to decide
// whether the system made progress.
//
// FailureSummaryHash carries a fixed-size digest of the verify
// stage's FailureSummary (FNV-32 hex). The verify→plan retry
// detector compares adjacent fingerprints across all four fields;
// if every field is identical to the previous round, no progress
// was made and further retry would just burn LLM budget on a
// problem the planner is structurally unable to fix from the
// signal it has. Hashing avoids storing arbitrary-length runner
// stderr in the fingerprint (which is supposed to be a small
// per-round snapshot) while preserving exact-match semantics.
type ApplyVerifyFingerprint struct {
	AppliedCount       int
	VerifyPassed       int
	VerifyFailed       int
	FailureSummaryHash string
	Timestamp          time.Time
}

// SameSignal reports whether two fingerprints describe the same
// outcome — used by the verify→plan retry detector to identify a
// "no progress" loop. Compares every field except Timestamp (which
// always differs across rounds and is irrelevant to convergence).
func (a ApplyVerifyFingerprint) SameSignal(b ApplyVerifyFingerprint) bool {
	return a.AppliedCount == b.AppliedCount &&
		a.VerifyPassed == b.VerifyPassed &&
		a.VerifyFailed == b.VerifyFailed &&
		a.FailureSummaryHash == b.FailureSummaryHash
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
