package types

import "strings"

// LooksLikeTestFilePath reports whether a repo-relative path follows a
// conventional test/spec location or filename pattern for the
// languages Codrax understands. This is a deterministic language-rule
// helper, not an intent classifier: callers use it to keep test-only
// files from masquerading as production evidence, ranking anchors, or
// classification observations.
func LooksLikeTestFilePath(relPath string) bool {
	normalized := strings.ReplaceAll(strings.TrimSpace(relPath), `\`, `/`)
	lower := strings.ToLower(normalized)
	if lower == "" {
		return false
	}
	base := lower
	if slash := strings.LastIndex(base, "/"); slash != -1 {
		base = base[slash+1:]
	}
	baseRaw := normalized
	if slash := strings.LastIndex(baseRaw, "/"); slash != -1 {
		baseRaw = baseRaw[slash+1:]
	}

	switch {
	case strings.HasSuffix(base, "_test.go"):
		return true
	case strings.HasSuffix(base, "_test.py"):
		return true
	case strings.HasSuffix(base, ".py") && strings.HasPrefix(base, "test_"):
		return true
	case strings.HasSuffix(base, ".rs") &&
		(strings.Contains(lower, "/tests/") || strings.HasPrefix(lower, "tests/")):
		return true
	case strings.HasSuffix(base, "test.java") || strings.HasSuffix(base, "tests.java"):
		return true
	case strings.HasSuffix(base, ".java") && hasJavaTestPrefix(baseRaw):
		return true
	case strings.HasSuffix(base, "test.kt") || strings.HasSuffix(base, "tests.kt") ||
		strings.HasSuffix(base, "test.kts") || strings.HasSuffix(base, "tests.kts"):
		return true
	case strings.Contains(lower, "/src/test/") || strings.Contains(lower, "/src/androidtest/"):
		return true
	case strings.HasSuffix(base, "_spec.rb") || strings.HasSuffix(base, "_test.rb"):
		return true
	case strings.HasSuffix(base, ".rb") &&
		(strings.Contains(lower, "/spec/") || strings.HasPrefix(lower, "spec/") ||
			strings.Contains(lower, "/test/") || strings.HasPrefix(lower, "test/")):
		return true
	case strings.HasSuffix(base, "test.swift") || strings.HasSuffix(base, "tests.swift"):
		return true
	case strings.HasSuffix(base, ".swift") &&
		(strings.Contains(lower, "/tests/") || strings.HasPrefix(lower, "tests/")):
		return true
	case strings.HasSuffix(base, "_spec.lua") || strings.HasSuffix(base, "_test.lua"):
		return true
	case strings.HasSuffix(base, ".lua") &&
		(strings.Contains(lower, "/tests/") || strings.Contains(lower, "/spec/") ||
			strings.HasPrefix(lower, "tests/") || strings.HasPrefix(lower, "spec/")):
		return true
	case strings.HasSuffix(base, "_test.c") || strings.HasSuffix(base, "_test.cc") ||
		strings.HasSuffix(base, "_test.cpp") || strings.HasSuffix(base, "_test.cxx") ||
		strings.HasSuffix(base, "_unittest.cc") || strings.HasSuffix(base, "_unittest.cpp"):
		return true
	}

	for _, ext := range []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx"} {
		if strings.HasSuffix(base, ".test"+ext) || strings.HasSuffix(base, ".spec"+ext) {
			return true
		}
	}
	return false
}

func hasJavaTestPrefix(base string) bool {
	base = strings.TrimSpace(base)
	switch {
	case strings.HasPrefix(base, "Test") && len(base) > len("Test"):
		r := rune(base[len("Test")])
		return r >= 'A' && r <= 'Z'
	case strings.HasPrefix(base, "IT") && len(base) > len("IT"):
		r := rune(base[len("IT")])
		return r >= 'A' && r <= 'Z'
	default:
		return false
	}
}
