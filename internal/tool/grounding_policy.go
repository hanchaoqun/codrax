package tool

import "sync"

// GroundingPolicy holds runtime-tunable knobs for the evidence
// grounder. Used by emit_investigation_complete to decide whether
// the LLM's claim of "investigation complete" is credible enough to
// transition the pipeline forward.
//
// Two independent floors run in AND: both must pass. See
// DefaultGroundingPolicy for the rationale behind each number.
type GroundingPolicy struct {
	// GroundingFloor is the minimum (grounded + recovered) / total
	// ratio required by emit_investigation_complete. Range [0, 1].
	// 0 disables the gate entirely; 1 requires every item grounded.
	GroundingFloor float64

	// Tier1Floor is the minimum ratio of Tier-1-proven items
	// (GroundingStatus=Grounded AND GroundingTier=TierLineText,
	// i.e. items the LLM produced from a file it actually read)
	// against the total evidence count. Range [0, 1]. 0 disables
	// the Tier-1 gate (session 7 backward-compat behaviour); a
	// positive value blocks "pure recovery" investigations where
	// the LLM never actually read_file'd any of the sources it
	// cited — the recovery tiers filled LineStart from the repomap
	// graph, but at citation time the finalizer grounder's strict
	// Tier 2 rejects those same anchors, leaving the pipeline in
	// an unrecoverable "explorer satisfied / finalizer stuck"
	// loop. The Tier-1 floor moves the intercept point upstream.
	Tier1Floor float64
}

// DefaultGroundingPolicy returns the shipped defaults.
//
// GroundingFloor=0.5 (session 5 design): high enough that a mostly-
// speculative investigation cannot declare complete; low enough that
// a handful of ungrounded leads in a rich-evidence investigation do
// not deadlock.
//
// Tier1Floor=0.3 (session 8): requires at least 30% of the emitted
// evidence to be Tier-1-proven (the LLM actually read the file).
// Picked to catch the session-7 trace failure mode (explorer
// satisfied via pure recovery; finalizer can't cite the same
// anchors) without rejecting legitimate investigations that rely on
// a mix of read_file and graph-only lookups. Set via
// analysis_evidence_tier1_floor in codrax.yaml.
func DefaultGroundingPolicy() GroundingPolicy {
	return GroundingPolicy{
		GroundingFloor: 0.5,
		Tier1Floor:     0.3,
	}
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
