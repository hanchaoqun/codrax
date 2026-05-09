// Package budget computes the EvidencePlan.Budget as a function of
// Complexity × term cardinality × hypothesis count × prescan hit
// ratio. It replaces the pre-refactor compiler.defaultBudget which
// only scaled by Complexity.
//
// Scaling is multiplicative so a "complex question with lots of
// terms but confirmed-grounded keywords" gets a large budget while a
// "simple question with zero prescan hits" gets a small but
// probe-friendly one.
package budget

import (
	"math"

	"github.com/hanchaoqun/codrax/internal/types"
)

// BudgetSignals are the runtime inputs to Compute. All fields are
// read by the analyzer from BusContext + quality probe.
type BudgetSignals struct {
	Complexity      types.Complexity
	TermCount       int
	HypothesisCount int
	PrescanHitRatio float64 // 0..1; 1 = prescan confirmed every keyword
}

// Compute returns the scaled EvidenceBudget for this signal set. The
// formula is pinned by budget_test.go — adjust numbers only when the
// test is updated alongside.
//
// Project-orientation carve-out (2026-04-29): when the analyzer
// classifies the request as a project-orientation ask
// (types.IsProjectOrientationQuestion: intent=explain, simple
// complexity, no entities, clean predicates), the base shrinks
// further to {files: 8, iters: 6}. The reasoning is empirical —
// "what does this project do?" type answers come from README +
// manifest + entry point + a couple of top-level package summaries;
// the moderate / complex bases pull the explorer toward 30+
// file reads that simply aren't necessary for the answer shape.
// Production trace at /home/chatpp/pytest 2026-04-29 was the case
// study: 31 explorer rounds + 70 evidence items for a question that
// needed ~5. The carve-out makes MaxFiles / MaxReactIters tight
// enough that the explorer's existing budget gate enforces the
// shape — no new yaml field, no new IR field, no per-level magic
// numbers; reuses the SAME structural-signal helper the gate at
// emit_investigation_complete already uses.
//
// Phase 2.C uplift (2026-05-09): the per-complexity bases were
// raised by 25-33% to give the explorer enough budget for
// exhaustive-enumeration questions over a multi-package directory.
// Pre-uplift the complex base (48 files / 26 iters / 104 tool
// calls @ mult=1) ran out before covering a 5-10 sub-package
// module's exports — u8a/u8b regressed with "lower-bound"
// answers because explorer hit budget after partial coverage.
// Project-orientation tier stays at 8/6 (the answer shape is
// intrinsically small); the multiplier path (termFactor ×
// hypFactor × probeFactor ∈ [0.42, 6.0]) is unchanged so the
// uplift is a base bump, not a ceiling change. Simple-question
// budgets in practice still scale DOWN to ~10 files / 5 iters
// when termFactor/hypFactor collapse, so the uplift doesn't
// inflate token cost on already-cheap requests.
func Compute(rm types.RequestModel, sig BudgetSignals) types.EvidenceBudget {
	base := baseFor(rm, sig.Complexity)
	termFactor := clamp(0.6+0.05*float64(sig.TermCount), 0.6, 2.0)
	hypFactor := clamp(0.7+0.10*float64(sig.HypothesisCount), 0.7, 2.0)
	probeFactor := clamp(1.0+(1.0-clamp(sig.PrescanHitRatio, 0, 1))*0.5, 1.0, 1.5)
	mult := termFactor * hypFactor * probeFactor
	return types.EvidenceBudget{
		MaxFiles:      intMax(1, int(math.Round(float64(base.files)*mult))),
		MaxBytes:      200_000,
		MaxReactIters: intMax(1, int(math.Round(float64(base.iters)*mult))),
		MaxToolCalls:  intMax(1, int(math.Round(float64(base.iters*4)*mult))),
	}
}

type baseNums struct{ files, iters int }

func baseFor(rm types.RequestModel, c types.Complexity) baseNums {
	// Project-orientation short-circuit: tighter than ComplexitySimple
	// because the answer shape (README + manifest + entry point) is
	// fundamentally smaller than even a "simple" code-investigation
	// answer. types.IsProjectOrientationQuestion uses the same
	// structured-signal predicate the multi-path symbol-anchored gate
	// (applyMultiPathAnchorChecks) uses, so both stages agree on what
	// "orientation question" means.
	if types.IsProjectOrientationQuestion(rm) {
		return baseNums{files: 8, iters: 6}
	}
	switch c {
	case types.ComplexitySimple:
		// Phase 2.C uplift: 18/10 → 24/12 (+33% files, +20% iters).
		// Tool-call budget @ mult=1: 40 → 48 (+20%).
		return baseNums{files: 24, iters: 12}
	case types.ComplexityComplex:
		// Phase 2.C uplift: 48/26 → 64/32 (+33% files, +23% iters).
		// Tool-call budget @ mult=1: 104 → 128 (+23%). Sized to
		// cover exhaustive-enumeration questions over a 5-10
		// sub-package directory (e.g. u8a/u8b's "list all exports
		// of internal/analysis/criterion" / "list all enum types
		// in internal/types") without hitting MaxFiles or
		// MaxReactIters mid-investigation.
		return baseNums{files: 64, iters: 32}
	}
	// Phase 2.C uplift: 30/16 → 40/20 (+33% files, +25% iters).
	// Tool-call budget @ mult=1: 64 → 80 (+25%).
	return baseNums{files: 40, iters: 20}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func intMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}
