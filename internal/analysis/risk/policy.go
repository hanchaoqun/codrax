package risk

import "github.com/hanchaoqun/codrax/internal/types"

// DerivePolicy maps a risk matrix and the writing flag into a
// read-only RunPolicy the orchestrator will use for the rest of the
// run. The mapping table comes straight from the refactor plan
// (analyzer-v3/b0, §4.4):
//
//	writing=false                     → all require_* = false
//	security ≥ 3 OR compliance ≥ 3    → design_review + code_review
//	data_integrity ≥ 3                → verify
//	compatibility ≥ 3                 → forbid skipping design_review
//	performance   ≥ 4                 → verify
//
// Every rule is strictly additive: once a stage is required, no
// subsequent rule can drop it. This is the invariant the "RunPolicy
// is read-only for the rest of the run" design rests on.
func DerivePolicy(rm types.RiskMatrix, writing bool) types.RunPolicy {
	p := types.RunPolicy{Writing: writing}
	if !writing {
		return p
	}
	if rm.Security.Level >= 3 || rm.Compliance.Level >= 3 {
		p.RequireDesignReview = true
		p.RequireCodeReview = true
	}
	if rm.DataIntegrity.Level >= 3 {
		p.RequireVerify = true
	}
	if rm.Compatibility.Level >= 3 {
		p.ForbidSkipStages = appendUnique(p.ForbidSkipStages, "design_review")
	}
	if rm.Performance.Level >= 4 {
		p.RequireVerify = true
	}
	return p
}

func appendUnique(xs []string, v string) []string {
	for _, x := range xs {
		if x == v {
			return xs
		}
	}
	return append(xs, v)
}
