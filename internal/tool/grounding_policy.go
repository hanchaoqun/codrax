package tool

import "sync"

// GroundingPolicy holds runtime-tunable knobs for the evidence
// grounder. Currently carries a single threshold — grounded_ratio
// floor used by emit_investigation_complete — but kept as a struct
// so future knobs (recovery aggressiveness, per-tier enable flags)
// can be added without touching every call site.
type GroundingPolicy struct {
	// GroundingFloor is the minimum (grounded + recovered) / total
	// ratio required by emit_investigation_complete. Range [0, 1].
	// 0 disables the gate entirely; 1 requires every item grounded.
	GroundingFloor float64
}

// DefaultGroundingPolicy returns the shipped defaults. Design
// decision (2026-04-17 redesign, user pick #3): floor = 0.5. High
// enough to keep a mostly-speculative investigation from declaring
// complete; low enough that a handful of ungrounded leads in a
// rich-evidence investigation do not deadlock the pipeline.
func DefaultGroundingPolicy() GroundingPolicy {
	return GroundingPolicy{GroundingFloor: 0.5}
}

var (
	groundingPolicyMu sync.RWMutex
	groundingPolicy   = DefaultGroundingPolicy()
)

// SetGroundingPolicy installs a new grounding policy. cmd/root.go
// calls this after loading codrax.yaml; tests call it via t.Cleanup
// to restore defaults between runs.
func SetGroundingPolicy(p GroundingPolicy) {
	groundingPolicyMu.Lock()
	defer groundingPolicyMu.Unlock()
	groundingPolicy = p
}

// CurrentGroundingPolicy returns a snapshot of the active policy.
func CurrentGroundingPolicy() GroundingPolicy {
	groundingPolicyMu.RLock()
	defer groundingPolicyMu.RUnlock()
	return groundingPolicy
}
