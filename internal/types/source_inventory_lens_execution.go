package types

import "strings"

// SourceInventoryLensExecuted reports whether the observation came from an
// executable source-inventory lens, as opposed to a class-universe-only seed
// that exists solely to make absence gates repo-truth-aware.
func SourceInventoryLensExecuted(o SourceInventoryObservation) bool {
	if !o.IsActive() {
		return false
	}
	if len(o.Sets) > 0 {
		return true
	}
	for _, provenance := range o.Provenance {
		switch strings.TrimSpace(provenance) {
		case "repo_lens:tool_query", "pre_explore_typed_request":
			return true
		}
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
