package types

import (
	"path"
	"strings"
)

// BoundedSourceEnumerationMinFiles is the conservative floor for treating a
// category-enumeration IR as a bounded source inventory scope when the analyzer
// omitted the optional source_inventory_profile. The signal is advisory and
// scheduling-only at callers; it must not become answer authority.
const BoundedSourceEnumerationMinFiles = 6

// BoundedSourceEnumerationScopeFiles returns the source/config files that make
// a typed category enumeration look like one bounded source inventory scope.
// The helper is intentionally schema-only: it consumes RequestModel typed
// fields, EvidencePlan.RequiredFiles, AnalyzerHints.RequiredFileHints, and
// deterministic SourcePathRole classification. It never scans raw user text or
// model prose.
func BoundedSourceEnumerationScopeFiles(rm RequestModel, requiredFiles []string, repoRoot string) []string {
	if !IsTypedSourceEnumerationShape(rm) {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		canon := CanonicalRequiredFileHintPath(raw, repoRoot)
		if canon == "" || seen[canon] || !boundedSourceEnumerationPathAllowed(rm, canon) {
			return
		}
		seen[canon] = true
		out = append(out, canon)
	}
	for _, file := range requiredFiles {
		add(file)
	}
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if hint.Confidence < 0.5 {
			continue
		}
		add(hint.Path)
	}
	return out
}

// HasBoundedSourceEnumerationScope reports whether the typed IR carries a
// sizeable same-scope source inventory shape even without source_inventory_profile.
func HasBoundedSourceEnumerationScope(rm RequestModel, requiredFiles []string, repoRoot string) bool {
	files := BoundedSourceEnumerationScopeFiles(rm, requiredFiles, repoRoot)
	if len(files) < BoundedSourceEnumerationMinFiles {
		return false
	}
	return BoundedSourceEnumerationCommonScope(files) != ""
}

// IsTypedSourceEnumerationShape reports whether the request model describes a
// source-inventory-like enumeration shape without relying on raw request text.
// Callers must still prove a bounded scope separately, either through
// RequiredFiles or graph-resolved source entities.
func IsTypedSourceEnumerationShape(rm RequestModel) bool {
	if rm.Intent != IntentEnumerate || !rm.Predicates.IsCategoryEnumeration {
		return false
	}
	if rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsRoleLocateLookup ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	return true
}

func boundedSourceEnumerationPathAllowed(rm RequestModel, rel string) bool {
	rel = strings.TrimSpace(strings.ReplaceAll(rel, `\`, `/`))
	if rel == "" || rel == "." || strings.HasSuffix(rel, "/") {
		return false
	}
	if !HasCodeOrConfigPathSuffix(rel) {
		return false
	}
	scope := SourceScopeProduction
	if rm.SourceScopeProfile != nil && rm.SourceScopeProfile.RequestedScope != "" {
		scope = rm.SourceScopeProfile.RequestedScope
	}
	return SourceScopeAllowsPathRole(scope, ClassifySourcePathRole(rel))
}

// BoundedSourceEnumerationCommonScope returns the common non-root directory for
// a filtered set of source inventory files. Empty means the set is too broad or
// root-level to use as a bounded inventory scope.
func BoundedSourceEnumerationCommonScope(files []string) string {
	var common []string
	for _, file := range files {
		dir := strings.Trim(path.Dir(strings.Trim(strings.ReplaceAll(file, `\`, `/`), "/")), "/")
		if dir == "" || dir == "." {
			return ""
		}
		parts := strings.Split(dir, "/")
		if len(common) == 0 {
			common = append([]string(nil), parts...)
			continue
		}
		n := 0
		for n < len(common) && n < len(parts) && common[n] == parts[n] {
			n++
		}
		common = common[:n]
		if len(common) == 0 {
			return ""
		}
	}
	if len(common) == 0 {
		return ""
	}
	return strings.Join(common, "/")
}
