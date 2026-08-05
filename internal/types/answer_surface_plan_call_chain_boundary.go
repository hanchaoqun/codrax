package types

// projectCallChainBoundaryAnswerAuthority removes model-authored path rosters
// from the finalizer's answer-authority view after the model has declared the
// typed no_directed_path disposition. The raw accepted aggregates remain in
// MutableState for audit/resume; finalization instead consumes the exact
// endpoint boundary plus grounded call-edge triples. A member_set cannot be a
// directed path carrier in this disposition because its ordered members do not
// encode edge direction.
func projectCallChainBoundaryAnswerAuthority(
	facts []AnswerAggregateFact,
	boundary *CallChainEndpointBoundary,
) []AnswerAggregateFact {
	if boundary == nil || !boundary.Active() ||
		boundary.Disposition != CallChainEndpointNoDirectedPath {
		return cloneAnswerAggregateFacts(facts)
	}
	out := make([]AnswerAggregateFact, 0, len(facts))
	for _, fact := range facts {
		if fact.Kind == AnswerAggregateMemberSet {
			continue
		}
		out = append(out, fact)
	}
	return out
}
