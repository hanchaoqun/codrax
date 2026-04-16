// Package priority scores hypotheses on four dimensions and
// collapses the result into a single integer the gate and scheduler
// can compare. The replacement for the pre-refactor hardcoded
// priority constants (90/85/80/70/60) in internal/analysis/hdp.
//
// Stability: ties are broken by the hypothesis index, so callers
// sorting desc get a deterministic ordering.
package priority

import (
	"math"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Inputs is the per-hypothesis score breakdown. Each field is in
// [0, 1] and is combined linearly in Score with weights:
//
//	IntentMatch         0.35
//	RiskElevation       0.30
//	TermCardinality     0.20
//	AmbiguityResolution 0.15
type Inputs struct {
	IntentMatch         float64
	RiskElevation       float64
	TermCardinality     float64
	AmbiguityResolution float64
}

// Score computes the final priority. The return value is
// `round(raw*100) * 1000 - idx`, packed so a slice sorted desc is
// stable even when two hypotheses have equal raw scores.
func Score(h types.Hypothesis, rm types.RequestModel, idx int) int {
	in := ComputeInputs(h, rm)
	raw := 0.35*in.IntentMatch +
		0.30*in.RiskElevation +
		0.20*in.TermCardinality +
		0.15*in.AmbiguityResolution
	if raw < 0 {
		raw = 0
	}
	if raw > 1 {
		raw = 1
	}
	base := int(math.Round(raw*100)) * 1000
	return base - idx
}

// Raw returns Score divided back down to the 0-100 range human
// operators expect to see in gate diagnostics. Used by gate's
// hypothesis_coverage check — its min-priority threshold is in the
// human scale, not the packed scale.
func Raw(score int) int {
	return score / 1000
}

// ComputeInputs extracts the four scoring dimensions from a
// hypothesis + request model pair. Exported for tests.
func ComputeInputs(h types.Hypothesis, rm types.RequestModel) Inputs {
	return Inputs{
		IntentMatch:         intentMatch(h, rm),
		RiskElevation:       riskElevation(h, rm),
		TermCardinality:     termCardinality(rm),
		AmbiguityResolution: ambiguityResolution(h, rm),
	}
}

func intentMatch(h types.Hypothesis, rm types.RequestModel) float64 {
	stmt := strings.ToLower(h.Statement)
	switch rm.Intent {
	case types.IntentRootCause:
		if strings.Contains(stmt, "cause") || strings.Contains(stmt, "symptom") || strings.Contains(stmt, "failing") {
			return 1.0
		}
	case types.IntentConfigQuery:
		if strings.Contains(stmt, "config") || strings.Contains(stmt, "resolve") || strings.Contains(stmt, "override") {
			return 1.0
		}
	case types.IntentEnumerate, types.IntentReturnValue:
		if strings.Contains(stmt, "set") || strings.Contains(stmt, "finite") || strings.Contains(stmt, "list") {
			return 1.0
		}
	case types.IntentExplain, types.IntentTrace:
		if strings.Contains(stmt, "architect") || strings.Contains(stmt, "evidence") || strings.Contains(stmt, "anchored") {
			return 0.8
		}
	}
	return 0.4
}

func riskElevation(h types.Hypothesis, rm types.RequestModel) float64 {
	switch h.FalsificationCondition.Kind {
	case types.CritUntrustedReachesSink:
		return float64(rm.RiskMatrix.Security.Level) / 5.0
	case types.CritInvariantBroken:
		return float64(rm.RiskMatrix.DataIntegrity.Level) / 5.0
	case types.CritMultipleResolutionChains:
		return float64(rm.RiskMatrix.Ops.Level) / 5.0
	}
	return maxRisk(rm.RiskMatrix) / 5.0 * 0.5
}

func termCardinality(rm types.RequestModel) float64 {
	n := float64(len(rm.TermGraph.Canonical))
	// 1 - exp(-n/5) saturates near 1 by ~15 terms.
	return 1.0 - math.Exp(-n/5.0)
}

func ambiguityResolution(h types.Hypothesis, rm types.RequestModel) float64 {
	if len(rm.Ambiguities) == 0 {
		return 0
	}
	stmt := strings.ToLower(h.Statement)
	for _, a := range rm.Ambiguities {
		clause := strings.ToLower(a.Clause)
		if clause == "" {
			continue
		}
		for _, tok := range strings.Fields(clause) {
			if len(tok) > 3 && strings.Contains(stmt, tok) {
				return 1.0
			}
		}
	}
	return 0
}

func maxRisk(rm types.RiskMatrix) float64 {
	levels := []int{
		rm.Security.Level, rm.DataIntegrity.Level, rm.Compatibility.Level,
		rm.Performance.Level, rm.Ops.Level, rm.Compliance.Level,
	}
	max := 0
	for _, l := range levels {
		if l > max {
			max = l
		}
	}
	return float64(max)
}
