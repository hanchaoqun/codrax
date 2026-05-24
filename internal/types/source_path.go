package types

import (
	"path/filepath"
	"strings"
)

// SourcePathRole is the language-neutral role of a repo-relative path in a
// code investigation. It is intentionally derived from path structure, not from
// a user's prose, so callers can use it as a precise typed signal for candidate
// partitioning while still letting analyzer intent decide which roles are
// principal for a given question.
type SourcePathRole string

const (
	SourcePathRoleUnknown       SourcePathRole = ""
	SourcePathRoleProduction    SourcePathRole = "production"
	SourcePathRoleTest          SourcePathRole = "test"
	SourcePathRoleFixture       SourcePathRole = "fixture"
	SourcePathRoleExample       SourcePathRole = "example"
	SourcePathRoleDocumentation SourcePathRole = "documentation"
	SourcePathRolePromptSupport SourcePathRole = "prompt_support"
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

// ClassifySourcePathRole classifies a repo-relative path into a stable evidence
// role. The role tells downstream code how the path may be used; it does not
// decide whether a user intended to ask about that role. For example, test
// files are auxiliary for ordinary production implementation questions, but can
// become principal when the analyzer's typed request scope asks for tests.
func ClassifySourcePathRole(relPath string) SourcePathRole {
	normalized := strings.ReplaceAll(strings.TrimSpace(relPath), `\`, `/`)
	lower := strings.ToLower(normalized)
	if lower == "" {
		return SourcePathRoleUnknown
	}
	lower = strings.TrimPrefix(lower, "./")
	if LooksLikeTestFilePath(normalized) {
		return SourcePathRoleTest
	}
	switch {
	case strings.Contains(lower, "/testdata/"),
		strings.HasPrefix(lower, "testdata/"),
		strings.Contains(lower, "/tests/"),
		strings.HasPrefix(lower, "tests/"),
		strings.HasPrefix(lower, "test/"),
		strings.Contains(lower, "/__tests__/"),
		strings.HasPrefix(lower, "__tests__/"),
		strings.Contains(lower, "/fixtures/"),
		strings.HasPrefix(lower, "fixtures/"):
		return SourcePathRoleFixture
	case strings.Contains(lower, "/examples/"),
		strings.HasPrefix(lower, "examples/"):
		return SourcePathRoleExample
	case strings.Contains(lower, "/docs/"),
		strings.HasPrefix(lower, "docs/"):
		return SourcePathRoleDocumentation
	}
	base := strings.ToLower(filepath.Base(lower))
	if (strings.Contains(lower, "/internal/skill/") || strings.HasPrefix(lower, "internal/skill/")) && strings.Contains(base, "contract") {
		return SourcePathRolePromptSupport
	}
	if LooksLikeConfigFilePath(normalized) {
		return SourcePathRoleProduction
	}
	switch {
	case strings.HasPrefix(base, "readme."),
		strings.HasPrefix(base, "changelog."),
		strings.HasPrefix(base, "contributing."),
		strings.HasPrefix(base, "security."),
		strings.HasPrefix(base, "design."):
		return SourcePathRoleDocumentation
	}
	switch filepath.Ext(base) {
	case ".md", ".markdown", ".rst", ".adoc", ".txt":
		return SourcePathRoleDocumentation
	default:
		return SourcePathRoleProduction
	}
}

// SourcePathRoleIsAuxiliary reports whether a path role is normally supporting
// context rather than primary production proof. It is separate from
// ClassifySourcePathRole so intent-aware callers can opt tests/docs back into
// principal scope without re-parsing paths.
func SourcePathRoleIsAuxiliary(role SourcePathRole) bool {
	switch role {
	case SourcePathRoleTest,
		SourcePathRoleFixture,
		SourcePathRoleExample,
		SourcePathRoleDocumentation,
		SourcePathRolePromptSupport:
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
	return SourcePathRoleIsAuxiliary(ClassifySourcePathRole(relPath))
}
