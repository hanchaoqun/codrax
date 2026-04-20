package logparse

import (
	"path/filepath"
	"strings"
	"sync"
)

// Package-level user-provided source-prefix override. Set once at
// startup from cmd/root.go's --log-source-prefix flag. Consumed by
// StripBuildPathPrefix when the caller does not supply an explicit
// extraPrefixes list. Kept here (not on a config struct) so the
// analyzer doesn't need to plumb a new param through buildAnalysisIR
// for a value that is set exactly once per process and read at every
// log-triage dispatch.
var (
	sourcePrefixMu sync.RWMutex
	sourcePrefix   string
)

// SetSourcePrefix installs a user-provided build-path prefix override.
// The value is consulted by StripBuildPathPrefix when its extraPrefixes
// argument is empty. Empty string clears the override.
func SetSourcePrefix(p string) {
	sourcePrefixMu.Lock()
	defer sourcePrefixMu.Unlock()
	sourcePrefix = strings.TrimSpace(p)
}

// SourcePrefix returns the currently-installed override, or empty when
// none has been set.
func SourcePrefix() string {
	sourcePrefixMu.RLock()
	defer sourcePrefixMu.RUnlock()
	return sourcePrefix
}

// StripBuildPathPrefix heuristically strips C/C++ build-machine path
// prefixes from a log frame so the remainder has a chance of matching
// a user-repo layout.
//
// Common prefixes (checked in declared order):
//
//   - `/build/*/src/`, `/build/*/source/` — CI build roots
//   - `/builddir/`, `/rpmbuild/BUILD/`     — distro build trees
//   - `/workspace/`, `/tmp/build/`          — Jenkins / GitLab CI
//   - `/home/*/*/src/`, `/home/*/src/`      — dev-machine home dirs
//   - `<repoBaseName>/` when repoBaseName is non-empty and appears
//     as a path component (supplied via the caller's knowledge of
//     the target repo's basename)
//
// The stripped suffix is returned; callers should `os.Stat` it inside
// the repo to confirm it resolves. Empty input returns empty output.
// If no prefix matches, the input is returned unchanged — the caller
// decides whether to fall back to the Func-only degrade path.
//
// extraPrefixes lets the CLI `--log-source-prefix` override supply
// project-specific roots (users know their build path better than we
// can guess). Order: extraPrefixes first, then the built-ins, then
// the repoBaseName component scan.
func StripBuildPathPrefix(path, repoBaseName string, extraPrefixes []string) string {
	if path == "" {
		return ""
	}
	// Normalise separators so Windows-shape absolute paths like
	// `C:\build\src\foo.cpp` run through the same logic.
	p := filepath.ToSlash(path)

	// Fall back to the package-level override when no explicit extras
	// were supplied. This lets cmd/root.go's --log-source-prefix flag
	// reach every StripBuildPathPrefix caller without plumbing through.
	if len(extraPrefixes) == 0 {
		if global := SourcePrefix(); global != "" {
			extraPrefixes = []string{global}
		}
	}

	// Strip in cascading passes: extra-prefix → built-ins → home scan
	// → repo-basename. Each pass operates on the result of the
	// previous so a build path like `/rpmbuild/BUILD/pkg-1.0/src/foo.cpp`
	// with repoBaseName="pkg-1.0" reduces all the way to `src/foo.cpp`.
	p = stripExtraPrefixes(p, extraPrefixes)
	p = stripBuiltinPrefixes(p)
	p = stripHomeSourceScan(p)
	p = stripRepoBasename(p, repoBaseName)
	return p
}

func stripExtraPrefixes(p string, extra []string) string {
	for _, pre := range extra {
		pre = strings.TrimSpace(pre)
		if pre == "" {
			continue
		}
		pre = filepath.ToSlash(pre)
		if !strings.HasSuffix(pre, "/") {
			pre = pre + "/"
		}
		if strings.HasPrefix(p, pre) {
			return strings.TrimPrefix(p, pre)
		}
		if idx := strings.Index(p, "/"+pre); idx >= 0 {
			return p[idx+len("/"+pre):]
		}
	}
	return p
}

func stripBuiltinPrefixes(p string) string {
	builtins := []string{
		"/build/src/",
		"/build/source/",
		"/builddir/build/BUILD/",
		"/rpmbuild/BUILD/",
		"/workspace/source/",
		"/workspace/",
		"/tmp/build/",
		"/src/build/",
	}
	for _, pre := range builtins {
		if strings.HasPrefix(p, pre) {
			return strings.TrimPrefix(p, pre)
		}
	}
	return p
}

func stripHomeSourceScan(p string) string {
	if !strings.HasPrefix(p, "/home/") {
		return p
	}
	for _, anchor := range []string{"/src/", "/source/", "/workspace/", "/project/"} {
		if idx := strings.Index(p, anchor); idx >= 0 {
			return p[idx+len(anchor):]
		}
	}
	return p
}

func stripRepoBasename(p, repoBaseName string) string {
	if repoBaseName == "" {
		return p
	}
	marker := "/" + repoBaseName + "/"
	if idx := strings.Index(p, marker); idx >= 0 {
		return p[idx+len(marker):]
	}
	if strings.HasPrefix(p, repoBaseName+"/") {
		return strings.TrimPrefix(p, repoBaseName+"/")
	}
	return p
}

// ResolveJavaFile ranks candidate repo-relative paths against a Java
// frame's (package, baseName) pair and returns matches in descending
// priority. The caller supplies the candidate list (typically the
// result of a repomap Glob or keyword_search for the baseName so the
// set is bounded).
//
// Ranking tiers:
//
//  1. Exact package-path match: the file's directory ends with the
//     package-as-path form (`com.example.foo` → "com/example/foo").
//     Most specific — always preferred when present.
//  2. Source-layout prefix match: the file lies under a recognised
//     Java source root (`src/main/java/`, `src/test/java/`) and the
//     post-root portion ends with the package-as-path form.
//  3. Baseline match: the candidate's basename matches and neither
//     of the package checks fired. Ordered by path length (shorter
//     wins) so deeply-nested files do not outrank sibling modules.
//
// Duplicates across tiers are resolved by keeping the higher tier.
// The function does NOT call os.Stat — callers decide whether to
// confirm existence or trust the repomap-sourced candidate list.
func ResolveJavaFile(pkg, baseName string, candidates []string) []string {
	if baseName == "" || len(candidates) == 0 {
		return nil
	}
	pkgPath := strings.ReplaceAll(pkg, ".", "/")

	type scored struct {
		path string
		tier int
		sort int // shorter path wins at equal tier
	}
	var hits []scored
	seen := make(map[string]bool, len(candidates))

	for _, cand := range candidates {
		if seen[cand] {
			continue
		}
		seen[cand] = true
		base := filepath.Base(cand)
		if base != baseName {
			continue
		}
		norm := filepath.ToSlash(cand)
		dir := filepath.ToSlash(filepath.Dir(norm))

		tier := 3
		if pkgPath != "" {
			if strings.HasSuffix(dir, "/"+pkgPath) || dir == pkgPath {
				tier = 1
			} else {
				for _, root := range []string{"src/main/java/", "src/test/java/", "src/"} {
					if idx := strings.Index(norm, root); idx >= 0 {
						after := norm[idx+len(root):]
						afterDir := filepath.ToSlash(filepath.Dir(after))
						if afterDir == pkgPath || strings.HasSuffix(afterDir, pkgPath) {
							tier = 2
							break
						}
					}
				}
			}
		}
		hits = append(hits, scored{path: cand, tier: tier, sort: len(norm)})
	}
	if len(hits) == 0 {
		return nil
	}

	// Sort tier asc (1 beats 3), then path length asc, then
	// lexicographic for determinism.
	for i := 0; i < len(hits); i++ {
		for j := i + 1; j < len(hits); j++ {
			if hits[j].tier < hits[i].tier ||
				(hits[j].tier == hits[i].tier && hits[j].sort < hits[i].sort) ||
				(hits[j].tier == hits[i].tier && hits[j].sort == hits[i].sort && hits[j].path < hits[i].path) {
				hits[i], hits[j] = hits[j], hits[i]
			}
		}
	}
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.path)
	}
	return out
}

// IsRuntimeInternalFile reports whether path references a language-
// runtime source file rather than a user-repo file. Used to filter
// `runtime/asm_amd64.s`, `node:internal/...`, `java.lang.*`,
// `<python>/threading.py` out of the RequiredFiles seed so the
// explorer does not burn budget reading stdlib internals.
func IsRuntimeInternalFile(path string) bool {
	if path == "" {
		return true
	}
	p := filepath.ToSlash(path)
	// Go runtime.
	if strings.Contains(p, "/go/src/runtime/") ||
		strings.HasPrefix(p, "runtime/") ||
		strings.HasSuffix(p, "asm_amd64.s") ||
		strings.HasSuffix(p, "asm_arm64.s") {
		return true
	}
	// Node internals.
	if strings.HasPrefix(p, "node:") {
		return true
	}
	// JDK internals — no standard path (they're inside rt.jar), but
	// frames like `java.base/java.lang.Thread.java` appear in some
	// traces. Reject anything under `java.base/` or `java.lang.` file.
	if strings.HasPrefix(p, "java.base/") {
		return true
	}
	return false
}
