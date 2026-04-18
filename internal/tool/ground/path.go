package ground

import (
	"path/filepath"
	"strings"
)

// CanonicalRepoRelative reduces a file path to its repo-relative
// canonical form so comparisons across independent code sites
// (read_file banner, evidence Source, citation File, repomap RelPath,
// whitelist readFiles set) agree via string equality.
//
// Rules:
//   - empty path → empty
//   - cleaned via filepath.Clean first (./a/../b → b; // → /)
//   - if path is absolute AND inside repoRoot → strip the repoRoot
//     prefix and return the remainder ( /mnt/d/repo/foo/bar.go with
//     repoRoot=/mnt/d/repo → foo/bar.go )
//   - if path is absolute but outside repoRoot, or repoRoot is empty,
//     or the Rel calculation escapes via `..` → return the cleaned
//     absolute form unchanged (preserves legitimate out-of-repo
//     paths and avoids silently aliasing them onto repo-relative
//     names)
//   - relative paths → cleaned as-is (filepath.Clean already handles
//     `./foo/bar` → `foo/bar`)
//
// The bug this exists to fix: the LLM may read a file via
// `read_file path=README.md` (relative) but then cite
// `/mnt/d/repo/README.md:7` (absolute) — the whitelist in
// emit_answer_document.go and the LineIndex / FileIndex lookups in
// GroundCitation compared the two strings directly and rejected every
// such citation as "file not in Turn A's ReadFiles list". Funneling
// every path through this helper produces one canonical form and the
// comparison succeeds regardless of LLM choice.
func CanonicalRepoRelative(path, repoRoot string) string {
	if path == "" {
		return ""
	}
	cleaned := filepath.Clean(path)
	if repoRoot == "" || !filepath.IsAbs(cleaned) {
		return cleaned
	}
	cleanedRoot := filepath.Clean(repoRoot)
	rel, err := filepath.Rel(cleanedRoot, cleaned)
	if err != nil || strings.HasPrefix(rel, "..") {
		return cleaned
	}
	return rel
}
