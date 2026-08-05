package types

// normalizeSourceInventoryObservationCompleteLenses mints per-query lenses
// only before lineage exists. Scopes/Sets/Provenance on a merged observation
// are global unions and cannot truthfully mint a boundary no query executed.
func normalizeSourceInventoryObservationCompleteLenses(in SourceInventoryObservation) []SourceInventoryCompleteLens {
	if len(in.CompleteLenses) == 0 {
		return sourceInventoryCompleteLensesFromObservation(in)
	}
	return mergeSourceInventoryCompleteLenses(nil, in.CompleteLenses)
}
