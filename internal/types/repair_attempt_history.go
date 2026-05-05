package types

import (
	"sync"
)

// repair_attempt_history.go — W2.5 (2026-05-05).
//
// RepairAttemptHistory is a per-Run accumulator that tracks how
// many times each unique (ViolKind, Fingerprint) pair has been
// dispatched for repair across ANY retry scope (mid-loop window
// hint, selective explore fallback, contract retry). Without this
// cross-scope view, the same root cause can survive indefinitely:
// it gets one retry budget per scope, and m1a's explorer_iters=33
// (4× the configured pipeline-max-steps=15) is the consequence.
//
// Once an entry exceeds MaxRepairAttemptsPerRoot, the orchestrator
// stops retrying and materialises the underlying violation as a
// user caveat (W1) — the system has demonstrably failed to fix the
// issue and further retries are wasted tokens.
//
// See docs/design/iteration_inflation_remediation.md §3 source #3.

// FpKey identifies a unique repair target. Mirrors the closure-side
// clusterIdent shape; intentionally NOT identical so the History
// can sit in the types package without depending on orchestrator.
type FpKey struct {
	Kind        ViolationKind
	Fingerprint string
}

// MaxRepairAttemptsPerRoot caps the per-(kind, fp) attempt count.
// Reaching this threshold transitions the violation to "materialise
// as caveat" mode at the next retry decision point. Set to 3 to
// give 1 first emit + 2 retries; pre-W2 the system effectively
// allowed 3+3+3 across the three retry scopes. Operators may
// tune via PipelineSettings in a follow-up commit if needed.
const MaxRepairAttemptsPerRoot = 3

// RepairAttemptHistory is the per-Run map of repair attempts keyed
// by (kind, fp). All accessors are concurrent-safe.
type RepairAttemptHistory struct {
	mu       sync.RWMutex
	attempts map[FpKey]int
}

// NewRepairAttemptHistory returns an empty, ready-to-use history.
func NewRepairAttemptHistory() *RepairAttemptHistory {
	return &RepairAttemptHistory{attempts: make(map[FpKey]int)}
}

// Increment bumps the counter for k by 1 and returns the new
// post-increment value. Safe to call from any goroutine.
func (h *RepairAttemptHistory) Increment(k FpKey) int {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.attempts == nil {
		h.attempts = make(map[FpKey]int)
	}
	h.attempts[k]++
	return h.attempts[k]
}

// Get returns the current attempt count for k (0 when never seen).
func (h *RepairAttemptHistory) Get(k FpKey) int {
	if h == nil {
		return 0
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.attempts[k]
}

// Exhausted reports whether the attempt count for k has reached
// or exceeded MaxRepairAttemptsPerRoot. Convenience helper for
// retry decision sites.
func (h *RepairAttemptHistory) Exhausted(k FpKey) bool {
	return h.Get(k) >= MaxRepairAttemptsPerRoot
}

// Reset clears the entire history. Used by the orchestrator's
// per-task reset path so each new Run starts with a fresh
// accumulator.
func (h *RepairAttemptHistory) Reset() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.attempts = make(map[FpKey]int)
}

// Snapshot returns a copy of the current attempt counts. Used by
// telemetry / debug summaries; never mutated by the caller.
func (h *RepairAttemptHistory) Snapshot() map[FpKey]int {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[FpKey]int, len(h.attempts))
	for k, v := range h.attempts {
		out[k] = v
	}
	return out
}

// ── MutableState accessors ──

// RepairAttempts lazily initialises and returns the per-Run
// RepairAttemptHistory. Lazy init mirrors EvidenceClosure() — the
// first reader allocates, subsequent readers reuse the same
// instance. Concurrent-safe.
func (m *MutableState) RepairAttempts() *RepairAttemptHistory {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	if m.repairAttempts != nil {
		h := m.repairAttempts
		m.mu.RUnlock()
		return h
	}
	m.mu.RUnlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.repairAttempts == nil {
		m.repairAttempts = NewRepairAttemptHistory()
	}
	return m.repairAttempts
}

// ResetRepairAttempts clears the per-Run history. Called at task
// entry to mirror ResetEvidenceClosure semantics.
func (m *MutableState) ResetRepairAttempts() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.repairAttempts != nil {
		m.repairAttempts.Reset()
	}
}
