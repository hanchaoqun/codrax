package types

import "strings"

// ObservedTypeLiteral proves a spelling on the bound historical source line,
// not an exception category, logger role, protocol, or error semantics. JSON
// recovery and edits to either model field cannot revive private authority.
func (e LogError) ObservedTypeLiteral() string {
	if !e.SourceBinding.supportsErrorTypeLiteral(e.Type, e.Message) {
		return ""
	}
	return strings.TrimSpace(e.Type)
}

// LogBundleObservedTypeLiterals is only for original-spelling preservation.
// LogBundleErrorTypes remains available for soft diagnostic/search hints.
func LogBundleObservedTypeLiterals(bundle *LogBundle) []string {
	var values []string
	seen := map[string]bool{}
	WalkLogErrors(bundle, func(e *LogError) {
		if value := e.ObservedTypeLiteral(); value != "" && !seen[value] {
			values, seen[value] = append(values, value), true
		}
	})
	return values
}

// LogErrorTypeSourceNote separates the classification from literal spelling.
func LogErrorTypeSourceNote(e LogError) string {
	if e.ObservedTypeLiteral() != "" {
		return "source literal"
	}
	return "diagnostic label (unverified)"
}
