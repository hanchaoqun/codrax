package types

// SourceInventoryRequiresRepoWideLens reports whether the typed request lacks
// an explicit local source boundary, so the first source-inventory lens must
// cover the repository source universe instead of RequiredFiles hints.
func SourceInventoryRequiresRepoWideLens(rm RequestModel) bool {
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return false
	}
	if SourceInventoryHasExactRequestedFileBoundary(rm) {
		return false
	}
	if rm.SourceScopeProfile == nil {
		return true
	}
	switch rm.SourceScopeProfile.RequestedScope {
	case SourceScopeAll, SourceScopeUnknown:
		return true
	case SourceScopeProduction:
		return !rm.AnswerExclusionPolicy.ExcludesAuxiliarySourceClasses()
	default:
		return false
	}
}

// SourceInventoryHasExactRequestedFileBoundary reports whether the typed
// analyzer output carries one precise file universe twice: once as
// high-confidence RequiredFileHints and once as source-scope quotes. The two
// canonical sets must be identical. This intentionally reads no raw request,
// rationale, model completion, or answer prose.
//
// Strict equality prevents a sampled file hint from narrowing an explicit
// production/test/all source-class request. Directory and class phrases fail
// closed and retain source-class semantics.
func SourceInventoryHasExactRequestedFileBoundary(rm RequestModel) bool {
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() ||
		rm.SourceScopeProfile == nil ||
		rm.SourceScopeProfile.RequestedScope != SourceScopeProduction ||
		len(rm.SourceScopeProfile.SourceQuotes) == 0 {
		return false
	}
	required := make(map[string]bool)
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if hint.Confidence < 0.8 {
			continue
		}
		file := CanonicalRequiredFileHintPath(hint.Path, "")
		if file == "" || !HasCodeOrConfigPathSuffix(file) {
			return false
		}
		required[file] = true
	}
	if len(required) == 0 {
		return false
	}
	quoted := make(map[string]bool)
	for _, raw := range rm.SourceScopeProfile.SourceQuotes {
		file := CanonicalRequiredFileHintPath(raw, "")
		if file == "" || !HasCodeOrConfigPathSuffix(file) || !required[file] {
			return false
		}
		quoted[file] = true
	}
	if len(quoted) != len(required) {
		return false
	}
	for file := range required {
		if !quoted[file] {
			return false
		}
	}
	return true
}
