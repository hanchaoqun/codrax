package types

import "strings"

// SourceInventoryLensExecuted reports whether the observation came from an
// executable source-inventory lens. Two shapes qualify: an active observation
// with row/source-class content, and the typed executed-empty carrier
// (§29.122 LENSBURN 病B — a successfully executed lens whose result is empty
// is still an executed lens). Both shapes share the same provenance
// discipline: analyzer-stage lenses never count, and a bare stage-scoped
// provenance without an executable marker never counts, so neither a failed
// lens nor a never-executed observation can impersonate execution.
func SourceInventoryLensExecuted(o SourceInventoryObservation) bool {
	if !o.IsActive() && !o.LensExecutedEmpty() {
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

// sourceInventoryLensExecutionCredentialProvenance extracts the execution-
// semantic provenance strings from an executed lens observation: the lens
// tool-query marker and the executable stage markers. Merge paths use this to
// carry a lost execution credential onto a merged view without importing any
// row-level provenance the donor never had.
func sourceInventoryLensExecutionCredentialProvenance(provenance []string) []string {
	var out []string
	for _, raw := range provenance {
		switch trimmed := strings.TrimSpace(raw); trimmed {
		case SourceInventoryProvenanceRepoLensToolQuery,
			SourceInventoryProvenancePreExplore,
			SourceInventoryProvenanceStageExplore:
			out = append(out, trimmed)
		}
	}
	return out
}

// ensureSourceInventoryLensExecutionCredential preserves an executed-lens
// credential across merge paths that would otherwise drop the donor side
// (§29.122 LENSBURN 病B: the executed-empty carrier is not IsActive, so the
// historic early-return merge arms silently discarded it). When the surviving
// view already carries an execution credential, or the donor never had one,
// this is the identity function.
func ensureSourceInventoryLensExecutionCredential(out, donor SourceInventoryObservation) SourceInventoryObservation {
	if !SourceInventoryLensExecuted(donor) || SourceInventoryLensExecuted(out) {
		return out
	}
	if !out.IsActive() {
		// Neither side carries rows: keep the typed executed-empty carrier
		// itself as the durable view.
		return CloneSourceInventoryObservation(donor)
	}
	out.Provenance = mergeSourceInventoryAdvisoryStrings(out.Provenance,
		sourceInventoryLensExecutionCredentialProvenance(donor.Provenance))
	return out
}
