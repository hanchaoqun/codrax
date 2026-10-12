package types

// Keep every query/chain intact; only the requested ruler is source-owned.
// In particular, a wide query and its valid local selected_window fragments
// are not clipped or discarded when another input requests a narrower range.
func traceSourceRequestedMemberProjections(records []ObservationRecord, entities []traceCausalProjectionAnchorEntity, requested *RuntimeArtifactScopeProfile, indices ...*RuntimeObservationDisplayScopes) []TraceCausalProjection {
	if requested == nil || requested.Comparison == nil {
		return traceRequestedMemberProjections(records, entities, requested)
	}
	var index *RuntimeObservationDisplayScopes
	if len(indices) == 1 {
		index = indices[0]
	}
	var out []TraceCausalProjection
	for _, group := range traceRequestedMemberResults(records, entities, nil) {
		local, qualified := index.Scope(group.records[0], requested)
		projections := traceRequestedMemberProjections(group.records, entities, local)
		for i := range projections {
			if !qualified {
				// Unknown ownership stays inspectable, never a requested input.
				projections[i].WindowScope.Role = TraceQueryWindowScopeSupportingExploration
			}
		}
		out = append(out, projections...)
	}
	return out
}
