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

// intentMatch (Phase 6 stage 22, 2026-05-03) — typed mapping
// from analyzer Intent enum to the FalsificationCondition.Kind
// values that align with that intent. Replaces the retired
// keyword-table scoring on Hypothesis.Statement prose ("cause",
// "config", "architect", etc.) which violated the no-keyword-
// matching red line by classifying typed Hypothesis records via
// substring on free-form text.
//
// Per-intent score table:
//
//	IntentRootCause           → 1.0 if Falsification.Kind matches
//	                             a root-cause-aligned criterion
//	IntentConfigQuery         → 1.0 if config-aligned
//	IntentEnumerate /
//	IntentReturnValue         → 1.0 if enumeration-aligned
//	IntentExplain /
//	IntentTrace               → 0.8 if explanation/trace-aligned
//	(fallback)                → 0.4
//
// The closed Criterion enum is the typed truth — HDP planner
// emits Falsification.Kind structurally; no prose scan needed.
func intentMatch(h types.Hypothesis, rm types.RequestModel) float64 {
	kind := h.FalsificationCondition.Kind
	if kind == "" {
		return 0.4
	}
	switch rm.Intent {
	case types.IntentRootCause:
		if intentRootCauseAlignedCriterion(kind) {
			return 1.0
		}
	case types.IntentConfigQuery:
		if intentConfigAlignedCriterion(kind) {
			return 1.0
		}
	case types.IntentEnumerate, types.IntentReturnValue:
		if intentEnumerationAlignedCriterion(kind) {
			return 1.0
		}
	case types.IntentExplain, types.IntentTrace:
		if intentExplainAlignedCriterion(kind) {
			return 0.8
		}
	}
	return 0.4
}

// intentRootCauseAlignedCriterion reports whether `kind` is a
// FalsificationCondition.Kind that HDP planner emits for root-
// cause hypotheses. The set is: untrusted-input → sink, broken
// invariant, missing-evidence-on-symbol — all structural proxies
// for "the bug originates here".
func intentRootCauseAlignedCriterion(kind string) bool {
	switch kind {
	case types.CritUntrustedReachesSink,
		types.CritInvariantBroken,
		types.CritNoCallSites,
		types.CritNoRelevantEvidence:
		return true
	}
	return false
}

// intentConfigAlignedCriterion — config-query hypotheses test
// "the configured value resolves through this chain" and
// fail-loudly when multiple chains exist (ambiguous resolution).
func intentConfigAlignedCriterion(kind string) bool {
	switch kind {
	case types.CritMultipleResolutionChains,
		types.CritUserClauseUnresolved:
		return true
	}
	return false
}

// intentEnumerationAlignedCriterion — enumeration / return-value
// hypotheses fail when the answer set is unbounded or when no
// concrete evidence supports a finite set.
func intentEnumerationAlignedCriterion(kind string) bool {
	switch kind {
	case types.CritAnswerSetUnbounded,
		types.CritNoRelevantEvidence,
		types.CritEvidenceCount,
		types.CritSymbolPresent:
		return true
	}
	return false
}

// intentExplainAlignedCriterion — explain / trace hypotheses
// rely on relationship/evidence-anchoring criteria.
func intentExplainAlignedCriterion(kind string) bool {
	switch kind {
	case types.CritNoRelevantEvidence,
		types.CritEvidenceCount,
		types.CritSymbolPresent,
		types.CritAnswerSetBounded:
		return true
	}
	return false
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

// ambiguityResolution (Phase 6 stage 22, 2026-05-03) — typed
// match between Hypothesis.FalsificationCondition.Expr and the
// Ambiguity.Clause that drove the hypothesis. HDP planner emits
// every ambiguity-derived hypothesis with FalsificationCondition
// .Kind == CritUserClauseUnresolved and .Expr == amb.Clause
// verbatim (see internal/analysis/hdp/planner.go:117). The
// retired path tokenized Hypothesis.Statement and substring-
// matched against Ambiguity.Clause word fragments — heuristic
// on free-form prose. The new path is structural equality on
// typed Criterion fields.
func ambiguityResolution(h types.Hypothesis, rm types.RequestModel) float64 {
	if len(rm.Ambiguities) == 0 {
		return 0
	}
	if h.FalsificationCondition.Kind != types.CritUserClauseUnresolved {
		return 0
	}
	expr := strings.TrimSpace(h.FalsificationCondition.Expr)
	if expr == "" {
		return 0
	}
	for _, a := range rm.Ambiguities {
		if strings.TrimSpace(a.Clause) == expr {
			return 1.0
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
