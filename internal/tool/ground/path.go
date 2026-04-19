package ground

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
	// POSIX-rooted paths must stay on slash semantics even on Windows.
	if strings.HasPrefix(p, "/") {
		return path.Clean(strings.ReplaceAll(p, "\\", "/"))
	}
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
