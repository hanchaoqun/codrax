// Package canonpath holds the repo-relative path canonicaliser used
// by BOTH the types data model (EvidenceClosure.ReadSet) and the
// higher layers (tool/ground, agent/explorer). The canonicalisation
// logic was born in internal/tool/ground/path.go (session 8 —
// cross-OS banner / evidence / citation path normalisation) but
// lived in a package that cannot be imported from internal/types
// without creating a dependency cycle. Session 22 split it out so
// the types package (the lowest layer) can canonicalise its own
// file-path keys the same way every upstream caller does — no
// duplication, no cycle.
//
// The package has ZERO internal imports — only stdlib. This is
// the neutral layer both `internal/types` and `internal/tool/...`
// can depend on.
package canonpath

import (
	"path"
	"path/filepath"
	"strings"
)

// CanonicalRepoRelative reduces a file path to the repo-relative
// canonical form used across read-file banners, evidence sources,
// citation files, repomap paths, and Turn A read sets.
//
// Contract:
//   - empty input returns empty
//   - output always uses forward slashes
//   - relative paths are cleaned and preserved
//   - absolute paths inside repoRoot are stripped to repo-relative form
//   - absolute paths outside repoRoot stay absolute
//
// Why this is not filepath.Rel-only: on Windows, POSIX absolute paths
// such as /mnt/d/repo/README.md are treated with host-native semantics
// and can come back as backslash paths, which then miss the slash-based
// repo-internal indices. We therefore keep comparisons in slash form.
func CanonicalRepoRelative(pathArg, repoRoot string) string {
	if pathArg == "" {
		return ""
	}
	cleaned := cleanRepoPath(pathArg)
	if repoRoot == "" || !isAbsoluteRepoPath(cleaned) {
		return cleaned
	}

	absRoot := cleanRepoPath(repoRoot)
	if !isAbsoluteRepoPath(absRoot) {
		// Relative repo roots such as "." need to be resolved against
		// the process cwd before we can compare them with absolute cites.
		resolved, err := filepath.Abs(repoRoot)
		if err != nil {
			return cleaned
		}
		absRoot = cleanRepoPath(resolved)
	}

	rel, ok := stripRepoRoot(cleaned, absRoot)
	if !ok {
		return cleaned
	}
	return rel
}

func cleanRepoPath(p string) string {
	if p == "" {
		return ""
	}
	// Unconditional backslash → slash normalisation. Without this
	// step, Windows-style absolute paths (`C:\Users\...`) emitted
	// by an LLM running on a Windows test box stay backslash-laden
	// when the canonicaliser runs on Linux/macOS — `filepath.Clean`
	// and `filepath.ToSlash` are platform-native, so on Linux they
	// don't recognise `\` as a separator and `isWindowsAbsolutePath`
	// (which keys on `<letter>:/`) misses. The result was that
	// Windows-absolute paths never got stripped to repo-relative on
	// non-Windows hosts, leaking through to evidence / citation /
	// banner surfaces. Fixing it at the source means every caller
	// (tool/ground, agent/explorer, types/EvidenceClosure) gets the
	// correct cross-platform behaviour without per-call duplication.
	p = strings.ReplaceAll(p, "\\", "/")
	// UNC shares (`//server/share/...`) need the leading double
	// slash preserved across `path.Clean`, which would otherwise
	// collapse `//` to `/`. We strip one slash before clean and
	// re-add it after so the rest of the path normalises while the
	// UNC prefix survives.
	if strings.HasPrefix(p, "//") {
		return "/" + path.Clean(p)
	}
	// POSIX-rooted paths use slash semantics directly.
	if strings.HasPrefix(p, "/") {
		return path.Clean(p)
	}
	// Relative paths + Windows volume paths (`C:/Users/...`):
	// after the ReplaceAll above no backslashes remain, so the
	// platform-native filepath.Clean is safe — on Linux it cleans
	// dot-segments without touching the slash-encoded volume,
	// on Windows it keeps producing canonical-form paths.
	return filepath.ToSlash(filepath.Clean(p))
}

func isAbsoluteRepoPath(p string) bool {
	if p == "" {
		return false
	}
	return path.IsAbs(p) || isWindowsAbsolutePath(p)
}

func isWindowsAbsolutePath(p string) bool {
	if len(p) >= 3 && isASCIIAlpha(p[0]) && p[1] == ':' && p[2] == '/' {
		return true
	}
	return strings.HasPrefix(p, "//")
}

func isASCIIAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func stripRepoRoot(target, root string) (string, bool) {
	if target == "" || root == "" {
		return "", false
	}

	targetCmp := target
	rootCmp := root
	// Windows volumes / UNC shares are case-insensitive; keep POSIX
	// paths case-sensitive so Linux behavior does not change.
	if isWindowsAbsolutePath(target) && isWindowsAbsolutePath(root) {
		targetCmp = strings.ToLower(targetCmp)
		rootCmp = strings.ToLower(rootCmp)
	}

	if targetCmp == rootCmp {
		return ".", true
	}

	prefix := rootCmp
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	if !strings.HasPrefix(targetCmp, prefix) {
		return "", false
	}
	return target[len(prefix):], true
}
