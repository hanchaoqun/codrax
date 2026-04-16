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
func Compute(rm types.RequestModel, sig BudgetSignals) types.EvidenceBudget {
	base := baseFor(sig.Complexity)
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

func baseFor(c types.Complexity) baseNums {
	switch c {
	case types.ComplexitySimple:
		return baseNums{files: 18, iters: 10}
	case types.ComplexityComplex:
		return baseNums{files: 48, iters: 26}
	}
	return baseNums{files: 30, iters: 16}
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
