package tool

import "github.com/hanchaoqun/codrax/internal/types"

// A mixed answer's scalar block need not count its explanation members. Keep
// explicit label/member checks, but do not infer scalar ownership from the
// overall count intent when another typed scalar value has been retained.
// This narrows an advisory consistency hint; it changes no evidence gate.
func preEmitImplicitMemberCountBindingUnambiguous(facts []types.AnswerAggregateFact) bool {
	for _, fact := range facts {
		if fact.Kind == types.AnswerAggregateScalar {
			return false
		}
	}
	return true
}
