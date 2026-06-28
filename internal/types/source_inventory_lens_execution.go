package types

import "strings"

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
	if len(o.Sets) > 0 && !hasStage {
		return true
	}
	if hasToolQuery && !hasStage {
		return true
	}
	for _, lens := range o.Lens {
		switch strings.TrimSpace(lens) {
		case "", "source_class_universe", "count", "repo_languages":
			continue
		default:
			return true
		}
	}
	return false
}
