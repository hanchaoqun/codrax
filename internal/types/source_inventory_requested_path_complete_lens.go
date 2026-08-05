package types

// SourceInventoryRequestedPathCompleteLensesCoverRoles reports whether actual
// executable source-inventory queries completely covered every principal role
// at every request-bound path. It ignores globally merged observation flags:
// old root truncation must not leak into a bounded request, while a bounded
// lens must never erase real repository-wide debt.
func SourceInventoryRequestedPathCompleteLensesCoverRoles(
	observation SourceInventoryObservation,
	rm RequestModel,
	roles []AnswerCandidateRole,
) bool {
	requested := SourceInventoryRequestedPathScopes(rm)
	roles = normalizeSourceInventoryFollowupRoles(roles)
	if len(requested) == 0 || len(roles) == 0 || len(observation.CompleteLenses) == 0 {
		return false
	}
	for _, role := range roles {
		covered := make(map[string]bool, len(requested))
		for _, rawLens := range observation.CompleteLenses {
			lens := normalizeSourceInventoryCompleteLens(rawLens)
			if lens.Role != role || lens.Count != lens.Total || len(lens.QueryPathScopes) == 0 {
				continue
			}
			executable, hasToolQuery, _, _ := sourceInventoryExecutionProvenance(lens.Provenance)
			if !executable || !hasToolQuery {
				continue
			}
			for _, requestedScope := range requested {
				for _, lensScope := range lens.QueryPathScopes {
					if sourceInventoryScopeCovers(lensScope, requestedScope) {
						covered[requestedScope] = true
						break
					}
				}
			}
		}
		if len(covered) != len(requested) {
			return false
		}
	}
	return true
}
