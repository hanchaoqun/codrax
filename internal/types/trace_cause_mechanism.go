package types

import (
	"fmt"
	"math"
	"strings"
)

// A mechanism qualifier is independent of frame causality and numerical
// attribution. Absence makes no positive claim about a mechanism.
const TraceMechanismLowerPriorityDependencyCandidate = "lower_priority_dependency_candidate"

// The schema and selector context share this explanation; output facts do
// not add model-authored fields or an extra completion obligation.
func TraceRootCauseMechanismTeaching() string {
	return "A mechanism_qualifier of " + TraceMechanismLowerPriorityDependencyCandidate + " describes a measured low-priority dependency candidate, not proof that inversion occurred or that a lock blocked the target; it is independent of frame causality. Its value_description separates ready-to-run waiting counted in full from the discounted running supply deficit. An absent mechanism_qualifier is not confirmation. Do not submit mechanism_qualifier or impact_breakdown: both are populated from the selected evidence."
}

// TraceNodeIsPriorityInversionCandidate is shared by the model's evidence
// handoff and the public sidecar compiler. Only producer-owned fields decide;
// an observed low-priority dependency does not prove inversion or a lock.
func TraceNodeIsPriorityInversionCandidate(node TraceCausalProjectionNode) bool {
	return node.PriorityInversionCandidate || TraceRootCauseTypeIsPriorityInversion(strings.TrimSpace(node.TypeToken))
}

// TraceRootCauseTypeIsPriorityInversion is the single row-family membership
// table for engine, projection and display consumers. Keep exact token
// equality here; presentation adapters may trim their input explicitly.
func TraceRootCauseTypeIsPriorityInversion(token string) bool {
	switch token {
	case "priority_inversion_candidate", "priority_inversion_runnable_wait":
		return true
	default:
		return false
	}
}

// TraceRootCauseImpactBreakdown is the existing producer's composition of
// the published impact, not another elapsed interval or a new estimate.
// Explicit zero components remain visible. It is output-only; the model
// continues to submit candidate_id and an optional description.
type TraceRootCauseImpactBreakdown struct {
	RunnableSeconds       float64 `json:"runnable_seconds"`
	RunningDeficitSeconds float64 `json:"running_deficit_seconds"`
	CapabilitySource      string  `json:"capability_source,omitempty"`
}

// NormalizeTraceRootCauseImpactBreakdown checks a frozen producer composition.
// It is shared by the binder and public report validator.
func NormalizeTraceRootCauseImpactBreakdown(in *TraceRootCauseImpactBreakdown, caliber string, impact float64) (*TraceRootCauseImpactBreakdown, error) {
	if in == nil {
		return nil, nil
	}
	if math.IsNaN(impact) || math.IsInf(impact, 0) || impact <= 0 {
		return nil, fmt.Errorf("impact_breakdown requires a finite positive published impact")
	}
	out := *in
	out.CapabilitySource = strings.TrimSpace(out.CapabilitySource)
	for _, value := range []float64{out.RunnableSeconds, out.RunningDeficitSeconds} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return nil, fmt.Errorf("impact_breakdown components must be finite and non-negative")
		}
	}
	// Observation amounts are independently published at microsecond precision.
	// One microsecond is the maximum rounding discrepancy, not a new estimate.
	if caliber != TraceImpactCaliberEffectiveAttribution || math.Abs(out.RunnableSeconds+out.RunningDeficitSeconds-impact) > 1.001e-6 {
		return nil, fmt.Errorf("impact_breakdown must compose the published effective attribution")
	}
	switch out.CapabilitySource {
	case "", "default_table", "evidence_table", "freq_only":
	default:
		return nil, fmt.Errorf("impact_breakdown capability_source=%q is unsupported", out.CapabilitySource)
	}
	return &out, nil
}
