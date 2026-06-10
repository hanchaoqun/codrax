package types

import "strings"

// TerminationKind classifies WHICH scheduler path is carrying the read
// run into finalize. It is written by the orchestrator at each
// termination point and consumed softly (telemetry, degradation
// caveats, gate diagnostics) — never as a hard routing signal.
type TerminationKind string

const (
	// TerminationNormal: every DAG node completed and finalize became
	// ready through the regular window flow.
	TerminationNormal TerminationKind = "normal"
	// TerminationStopCondition: a typed EvidencePlan stop condition
	// fired and force-closed the explore window.
	TerminationStopCondition TerminationKind = "stop_condition"
	// TerminationHardStall: the CGEC hard-stall detector saw the
	// evidence fingerprint pinned across consecutive rounds and
	// force-closed exploration; re-running exploration is futile by
	// construction (the fingerprint is static).
	TerminationHardStall TerminationKind = "hard_stall"
	// TerminationBlockedDAG: no window and no ready finalize while
	// blocked nodes existed — the scheduler broke out to forced
	// finalize because a pure-read environment can never unblock.
	TerminationBlockedDAG TerminationKind = "blocked_dag"
	// TerminationSchedulerStalled: no window, no ready finalize, and
	// no blocked nodes — a defensive break out of the loop.
	TerminationSchedulerStalled TerminationKind = "scheduler_stalled"
)

// TerminationProfile is the typed record of how the run reached
// finalize plus whether the pre-finalize grounding floor had to be
// waived. FloorDegraded=true means the Tier-1 floor failed AND no
// remediation lane remained (budget exhausted, forced finalize, or a
// hard stall where requeueing cannot change the evidence) — the
// answer ships, but the degradation must be visible to the user.
type TerminationProfile struct {
	Kind TerminationKind `json:"kind"`

	// FloorDegraded marks that the grounding floor failed without a
	// remediation lane. Detail keeps the bounded diagnostic for logs
	// and telemetry; it is NOT user-facing text.
	FloorDegraded bool   `json:"floor_degraded,omitempty"`
	Detail        string `json:"detail,omitempty"`
}

// NormalizeTerminationKind bounds arbitrary input to the enum,
// defaulting to normal.
func NormalizeTerminationKind(k TerminationKind) TerminationKind {
	switch k {
	case TerminationStopCondition, TerminationHardStall, TerminationBlockedDAG, TerminationSchedulerStalled:
		return k
	default:
		return TerminationNormal
	}
}

// SetTerminationProfile records the termination path. Later writers
// may upgrade the profile (e.g. mark FloorDegraded after the kind was
// recorded); kind writes never downgrade an existing degradation flag.
func (m *MutableState) SetTerminationProfile(p TerminationProfile) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p.Kind = NormalizeTerminationKind(p.Kind)
	p.Detail = strings.TrimSpace(p.Detail)
	if m.terminationProfile != nil && m.terminationProfile.FloorDegraded {
		p.FloorDegraded = true
		if p.Detail == "" {
			p.Detail = m.terminationProfile.Detail
		}
	}
	m.terminationProfile = &p
}

// MarkTerminationFloorDegraded upgrades the current profile (creating
// a normal-kind one when absent) with the floor-degradation flag.
func (m *MutableState) MarkTerminationFloorDegraded(detail string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.terminationProfile
	if p == nil {
		p = &TerminationProfile{Kind: TerminationNormal}
	}
	p.FloorDegraded = true
	if d := strings.TrimSpace(detail); d != "" {
		p.Detail = d
	}
	m.terminationProfile = p
}

// TerminationProfile returns the recorded profile, or nil.
func (m *MutableState) TerminationProfile() *TerminationProfile {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.terminationProfile == nil {
		return nil
	}
	cp := *m.terminationProfile
	return &cp
}
