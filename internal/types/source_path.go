package types

import (
	"path/filepath"
	"strings"
)

// LooksLikeAuxiliaryEvidencePath reports whether a repo-relative path
// points at test / fixture / example / documentation material that is
// usually useful only as supporting context, not as primary proof for
// implementation-presence questions.
func LooksLikeAuxiliaryEvidencePath(relPath string) bool {
	normalized := strings.ReplaceAll(strings.TrimSpace(relPath), `\`, `/`)
	lower := strings.ToLower(normalized)
	if lower == "" {
		return false
	}
	if LooksLikeTestFilePath(lower) {
		return true
	}
	switch {
	case strings.Contains(lower, "/testdata/"),
		strings.HasPrefix(lower, "testdata/"),
		strings.Contains(lower, "/fixtures/"),
		strings.HasPrefix(lower, "fixtures/"),
		strings.Contains(lower, "/examples/"),
		strings.HasPrefix(lower, "examples/"),
		strings.Contains(lower, "/docs/"),
		strings.HasPrefix(lower, "docs/"):
		return true
	}
	base := strings.ToLower(filepath.Base(lower))
	switch {
	case strings.HasPrefix(base, "readme."),
		strings.HasPrefix(base, "changelog."),
		strings.HasPrefix(base, "contributing."),
		strings.HasPrefix(base, "security."),
		strings.HasPrefix(base, "design."):
		return true
	}
	switch filepath.Ext(base) {
	case ".md", ".markdown", ".rst", ".adoc", ".txt":
		return true
	default:
		return false
	}
}
