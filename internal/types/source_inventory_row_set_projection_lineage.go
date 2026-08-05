package types

// sourceInventoryRequestBoundCompleteLensRowsCoverRoles grants principal-row
// projection authority to an executable per-query lens even when an older,
// broader observation left the merged role set incomplete. The grant is exact:
// every requested principal role needs one lens that covers the full requested
// path boundary, whose count equals the typed principal rows backed by that
// same lens. A completion credential alone never manufactures answer rows.
func sourceInventoryRequestBoundCompleteLensRowsCoverRoles(
	observation SourceInventoryObservation,
	rm RequestModel,
	roles []AnswerCandidateRole,
) bool {
	requested := SourceInventoryRequestedPathScopes(rm)
	roles = normalizeSourceInventoryFollowupRoles(roles)
	if len(requested) == 0 || len(roles) == 0 || len(observation.CompleteLenses) == 0 {
		return false
	}
	rowSet := BuildSourceInventoryPrincipalRowSet(SourceInventoryPrincipalRowSetInput{
		Observation:  observation,
		RequestModel: rm,
	})
	if !rowSet.Active || rowSet.PrincipalTotal == 0 {
		return false
	}
	rowsByRole := make(map[AnswerCandidateRole][]SourceInventoryRow, len(roles))
	for _, row := range rowSet.PrincipalRows {
		if row.Role != "" && row.Role != AnswerCandidateRoleUnknown {
			rowsByRole[row.Role] = append(rowsByRole[row.Role], row)
		}
	}
	for _, role := range roles {
		rows := rowsByRole[role]
		if len(rows) == 0 || !sourceInventoryRequestBoundLensExactlyCoversRows(observation.CompleteLenses, requested, role, rows) {
			return false
		}
	}
	return true
}

func sourceInventoryRequestBoundLensExactlyCoversRows(
	lenses []SourceInventoryCompleteLens,
	requested []string,
	role AnswerCandidateRole,
	rows []SourceInventoryRow,
) bool {
	for _, rawLens := range lenses {
		lens := normalizeSourceInventoryCompleteLens(rawLens)
		if lens.Role != role || lens.Count <= 0 || lens.Count != lens.Total || lens.Count != len(rows) ||
			!sourceInventoryCompleteLensHasExecutableToolCredential(lens) ||
			!sourceInventoryCompleteLensCoversAllRequestedPaths(lens.QueryPathScopes, requested) {
			continue
		}
		allRowsCovered := true
		for _, row := range rows {
			if !sourceInventoryCompleteLensCoversRow(lens, row) || !sourceInventoryCompleteLensCoversRowSurface(lens, row) {
				allRowsCovered = false
				break
			}
		}
		if allRowsCovered {
			return true
		}
	}
	return false
}

func sourceInventoryCompleteLensHasExecutableToolCredential(lens SourceInventoryCompleteLens) bool {
	executable, hasToolQuery, _, _ := sourceInventoryExecutionProvenance(lens.Provenance)
	return executable && hasToolQuery
}

func sourceInventoryCompleteLensCoversAllRequestedPaths(lensScopes, requested []string) bool {
	if len(lensScopes) == 0 || len(requested) == 0 {
		return false
	}
	for _, requestedScope := range requested {
		covered := false
		for _, lensScope := range lensScopes {
			if sourceInventoryScopeCovers(lensScope, requestedScope) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func sourceInventoryCompleteLensCoversRowSurface(lens SourceInventoryCompleteLens, row SourceInventoryRow) bool {
	if len(lens.SurfaceFamilies) == 0 {
		return true
	}
	want := SourceInventorySurfaceTermKey(row.SurfaceFamily)
	if want == "" {
		return false
	}
	for _, family := range lens.SurfaceFamilies {
		if SourceInventorySurfaceTermKey(family) == want {
			return true
		}
	}
	return false
}
