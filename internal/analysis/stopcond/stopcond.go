// Package stopcond evaluates EvidencePlan.StopConditions at each
// scheduler round. Returning stop=true tells the orchestrator to
// short-circuit the explorer window and dispatch finalize.
//
// Each StopCondition is a {Kind, Expr} pair treated as a Criterion.
// The whole list is OR-ed: the first satisfied condition wins and
// its human-readable detail surfaces as the stop reason.
package stopcond

import (
	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/types"
)

// ShouldStop reports whether the current Env satisfies any of the
// plan's stop conditions.
func ShouldStop(plan types.EvidencePlan, env criterion.Env) (stop bool, reason string) {
	for _, sc := range plan.StopConditions {
		c := types.Criterion{Kind: sc.Kind, Expr: sc.Expr}
		r := criterion.Eval(c, env)
		if r.UnknownKind {
			// Gate should have caught this; surface loudly.
			return true, "unknown stop condition kind " + sc.Kind + " (gate bug)"
		}
		if r.Satisfied {
			return true, sc.Kind + ": " + r.Detail
		}
	}
	return false, ""
}
