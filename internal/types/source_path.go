package types

import (
	"path/filepath"
	"strings"
)

// LooksLikePromptSupportPath reports whether a repo-relative path
// points at prompt / hint / skill support material. These files may be
// real implementation in the repo, but they are not runtime behavior
// sources and should not be promoted into answer-grade config
// precedence anchors or exact-absence proof for config questions.
//
// This helper is intentionally narrower than
// LooksLikeAuxiliaryEvidencePath: prompt-support code is still a real
// file the user may ask about directly, so we do NOT globally treat it
// as auxiliary evidence. Callers opt in only where runtime/config
// provenance matters more than repository membership.
func LooksLikePromptSupportPath(relPath string) bool {
	normalized := strings.ReplaceAll(strings.TrimSpace(relPath), `\`, `/`)
	lower := strings.ToLower(normalized)
	if lower == "" {
		return false
	}
	switch {
	case strings.Contains(lower, "/internal/skill/"),
		strings.HasPrefix(lower, "internal/skill/"),
		strings.Contains(lower, "/internal/analysis/hint/"),
		strings.HasPrefix(lower, "internal/analysis/hint/"),
		strings.Contains(lower, "/prompts/"),
		strings.HasPrefix(lower, "prompts/"),
		strings.Contains(lower, "/skills/"),
		strings.HasPrefix(lower, "skills/"):
		return true
	}
	base := strings.ToLower(filepath.Base(lower))
	switch base {
	case "glossary.go", "glossary.md":
		return true
	default:
		return false
	}
}

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
	if (strings.Contains(lower, "/internal/skill/") || strings.HasPrefix(lower, "internal/skill/")) && strings.Contains(base, "contract") {
		return true
	}
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
