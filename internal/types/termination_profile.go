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

// TerminationFloorArm names WHICH pre-finalize floor detection degraded the
// termination (件1, 2026-07-13). The user-facing disclosure must speak each
// arm's own truth — the ratio arm measures a proven-evidence ratio, the
// follow-up arm records unexecuted localization/drill-down suggestions —
// sharing one wording produced a false statement on the follow-up arm.
type TerminationFloorArm string

const (
	// TerminationFloorArmTier1Ratio: the Tier-1 proven-evidence ratio fell
	// below the configured floor.
	TerminationFloorArmTier1Ratio TerminationFloorArm = "tier1_ratio"
	// TerminationFloorArmFollowupCoverage: the read-localizer follow-up
	// (source localization / trace drill-down suggestions) was still open.
	TerminationFloorArmFollowupCoverage TerminationFloorArm = "followup_coverage"
)

// NormalizeTerminationFloorArm bounds arbitrary input to the enum; empty
// (legacy/unknown) is preserved and renders the ratio wording downstream.
func NormalizeTerminationFloorArm(a TerminationFloorArm) TerminationFloorArm {
	switch a {
	case TerminationFloorArmTier1Ratio, TerminationFloorArmFollowupCoverage:
		return a
	default:
		return ""
	}
}

// TerminationProfile is the typed record of how the run reached
// finalize plus whether the pre-finalize grounding floor had to be
// waived. FloorDegraded=true means a floor detection failed without a
// remediation lane — the answer ships, but the degradation must be
// visible to the user (arm-specific wording via FloorArm).
type TerminationProfile struct {
	Kind TerminationKind `json:"kind"`

	// FloorDegraded marks that the grounding floor failed without a
	// remediation lane. Detail keeps the bounded diagnostic for logs
	// and telemetry; it is NOT user-facing text. FloorArm names the
	// detection that fired so the disclosure can speak its truth;
	// empty means legacy/unknown and renders the ratio wording.
	FloorDegraded bool                `json:"floor_degraded,omitempty"`
	FloorArm      TerminationFloorArm `json:"floor_arm,omitempty"`
	Detail        string              `json:"detail,omitempty"`
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
		if p.FloorArm == "" {
			p.FloorArm = m.terminationProfile.FloorArm
		}
	}
	m.terminationProfile = &p
}

// MarkTerminationFloorDegraded upgrades the current profile (creating
// a normal-kind one when absent) with the floor-degradation flag.
// Legacy entry — arm-less; the disclosure renders the ratio wording.
func (m *MutableState) MarkTerminationFloorDegraded(detail string) {
	m.MarkTerminationFloorDegradedArm("", detail)
}

// MarkTerminationFloorDegradedArm is the arm-aware form (件1): the caller
// names which floor detection fired so the user-facing disclosure can speak
// that arm's truth instead of asserting a ratio the arm never measured.
func (m *MutableState) MarkTerminationFloorDegradedArm(arm TerminationFloorArm, detail string) {
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
	if a := NormalizeTerminationFloorArm(arm); a != "" {
		p.FloorArm = a
	}
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
