package types

// SourceInventoryLensExecuted reports whether the observation came from an executable source-inventory lens.
func SourceInventoryLensExecuted(o SourceInventoryObservation) bool {
	if !o.IsActive() {
		return false
	}
	executable, hasToolQuery, hasStage, hasAnalyzeStage := sourceInventoryExecutionProvenance(o.Provenance)
	if executable {
		return true
	}
	if hasAnalyzeStage {
		return false
	}
	if hasToolQuery && !hasStage {
		return true
	}
	return false
}
