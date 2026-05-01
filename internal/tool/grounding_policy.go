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
// Commit 61 Batch F.2 (audit CRITICAL #2, red line "no system
// hard-cap"): both floors default to 0 (gates DISABLED). Pre-
// commit-61 the defaults were 0.5 / 0.3 — auto-enabled hard caps
// that rejected emit_investigation_complete when the LLM judged
// "investigation done" with low groundedness. Per the no-hard-cap
// principle, the system cannot reliably distinguish "LLM under-
// invested" from "the question genuinely doesn't need more than
// recovery-tier evidence" — both shapes look identical at the
// gate. Trust the LLM's "complete" decision; the finalizer's
// citation-grounding layer still validates that emitted citations
// resolve to real lines, so a hallucinated answer still gets
// caught downstream by an OBJECTIVE check (does this file:line
// have this token), not a heuristic threshold.
//
// Operators who want the prior strict behaviour set
// analysis_grounding_floor / analysis_evidence_tier1_floor in
// codrax.yaml. The LegacyDefaultGroundingPolicy returns the
// pre-commit-61 numbers for tests that need to pin the strict
// behaviour explicitly.
func DefaultGroundingPolicy() GroundingPolicy {
	return GroundingPolicy{
		GroundingFloor: 0,
		Tier1Floor:     0,
	}
}

// LegacyDefaultGroundingPolicy returns the pre-commit-61 defaults
// (GroundingFloor=0.5, Tier1Floor=0.3) for tests that pin the
// strict-mode behaviour. Production cmd does NOT call this — the
// shipped defaults are gate-disabled (commit 61); operators who
// want strict mode set the yaml fields explicitly.
func LegacyDefaultGroundingPolicy() GroundingPolicy {
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
