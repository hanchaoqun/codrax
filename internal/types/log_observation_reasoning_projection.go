package types

import "strings"

// ProjectLogObservationForReasoning keeps the existing peer-error authority
// boundary consistent across prompt, ledger, binding, profile and seed readers.
// LogObservation has no typed link to one of several top-level error occurrences:
// its free-form Subject and Summary cannot supply an identity or causal claim.
// The original bundle remains the lossless audit carrier. This value-copy
// projection preserves the excerpt and its coordinates, never edits an answer,
// and does not inspect the wording of either the interpretation or the excerpt.
// Single-error/operational logs retain their existing advisory interpretation.
func ProjectLogObservationForReasoning(bundle *LogBundle, obs LogObservation) (LogObservation, bool) {
	if bundle == nil || len(bundle.Errors) <= 1 {
		return obs, true
	}
	obs.Subject = ""
	obs.Summary = obs.Evidence
	return obs, strings.TrimSpace(obs.Evidence) != ""
}
