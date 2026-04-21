package logtriage

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// sourcePrefix is the package-level user-provided build-path prefix
// override. Installed once at process start by cmd/root.go from the
// --log-source-prefix flag (or codrax.yaml's log_triage_source_prefix).
// Consulted by StripBuildPathPrefix when the caller does not supply an
// explicit extraPrefixes list. Kept here (not on a config struct) so
// every call site picks it up without plumbing through.
var (
	sourcePrefixMu sync.RWMutex
	sourcePrefix   string
)

// SetSourcePrefix installs the user-provided build-path prefix
// override. The value is consulted by StripBuildPathPrefix when the
// caller's extraPrefixes is empty. Empty string clears the override.
func SetSourcePrefix(p string) {
	sourcePrefixMu.Lock()
	defer sourcePrefixMu.Unlock()
	sourcePrefix = strings.TrimSpace(p)
}

// SourcePrefix returns the currently-installed override, or "" when
// none has been set.
func SourcePrefix() string {
	sourcePrefixMu.RLock()
	defer sourcePrefixMu.RUnlock()
	return sourcePrefix
}

// StripBuildPathPrefix heuristically strips CI / build-machine path
// prefixes so the remainder has a chance of matching a user-repo
// layout. Common prefixes checked in declared order:
//
//   - `/build/*/src/`, `/build/*/source/` — CI build roots
//   - `/builddir/`, `/rpmbuild/BUILD/`     — distro build trees
//   - `/workspace/`, `/tmp/build/`          — Jenkins / GitLab CI
//   - `/home/*/*/src/`, `/home/*/src/`      — dev-machine home dirs
//   - `<repoBaseName>/` when repoBaseName is non-empty and appears
//     as a path component
//
// The stripped suffix is returned; callers should os.Stat it inside
// the repo to confirm it resolves. Empty input returns empty output.
// If no prefix matches, the input is returned unchanged.
//
// extraPrefixes lets the CLI --log-source-prefix override supply
// project-specific roots. Order: extraPrefixes first, then built-ins,
// then home scan, then repo-basename.
func StripBuildPathPrefix(path, repoBaseName string, extraPrefixes []string) string {
	if path == "" {
		return ""
	}
	p := filepath.ToSlash(path)

	if len(extraPrefixes) == 0 {
		if global := SourcePrefix(); global != "" {
			extraPrefixes = []string{global}
		}
	}

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
// result of a filesystem walk for the baseName so the set is bounded).
//
// Ranking tiers:
//
//  1. Exact package-path match: the file's directory ends with the
//     package-as-path form.
//  2. Source-layout prefix match: the file lies under `src/main/java/`
//     or `src/test/java/` and the post-root portion ends with the
//     package path.
//  3. Baseline match: basename matches but no package signal fired.
//
// Duplicates across tiers collapse to the higher tier. The function
// does NOT call os.Stat — callers own verification.
func ResolveJavaFile(pkg, baseName string, candidates []string) []string {
	if baseName == "" || len(candidates) == 0 {
		return nil
	}
	pkgPath := strings.ReplaceAll(pkg, ".", "/")

	type scored struct {
		path string
		tier int
		sort int
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
// `runtime/asm_amd64.s`, `node:internal/...`, `java.base/*`, and
// similar stdlib/builtin entries out of the ResolvedFiles seed so
// the explorer does not burn budget on stdlib internals.
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
	// JDK internals.
	if strings.HasPrefix(p, "java.base/") {
		return true
	}
	return false
}

// ResolveFrameFile returns the repo-relative form of cand when it
// resolves to a file inside repoRoot. Handles two shapes:
//
//   - repo-relative ("internal/agent/foo.go") → os.Stat joined with
//     repoRoot; accept when it is a regular file
//   - absolute path that lives under repoRoot → filepath.Rel yields
//     the repo-relative form (rejects "../" escape)
//
// repoRoot is resolved to an absolute path internally before Rel is
// called — this handles the "-repo ." default where repoRoot is the
// literal string "." and a naive Rel would fail. Returns "" when the
// candidate cannot be verified.
func ResolveFrameFile(repoRoot, cand string) string {
	if cand == "" || repoRoot == "" {
		return ""
	}
	cand = filepath.ToSlash(cand)
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return ""
	}
	if filepath.IsAbs(cand) {
		rel, err := filepath.Rel(absRoot, filepath.FromSlash(cand))
		if err != nil || strings.HasPrefix(rel, "..") {
			return ""
		}
		if info, err := os.Stat(filepath.Join(absRoot, rel)); err == nil && !info.IsDir() {
			return filepath.ToSlash(rel)
		}
		return ""
	}
	if info, err := os.Stat(filepath.Join(absRoot, cand)); err == nil && !info.IsDir() {
		return cand
	}
	return ""
}

// GlobByBasename walks repoRoot and returns every file whose basename
// equals baseName. Capped at 64 matches; skips .git, node_modules,
// vendor, and hidden directories. Used by the Java frame resolver when
// the frame carries only a basename.
func GlobByBasename(repoRoot, baseName string) ([]string, error) {
	const maxHits = 64
	var out []string
	rootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, err
	}
	err = filepath.Walk(rootAbs, func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			if info != nil && info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" ||
				(strings.HasPrefix(name, ".") && len(name) > 1 && path != rootAbs) {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Name() != baseName {
			return nil
		}
		rel, rerr := filepath.Rel(rootAbs, path)
		if rerr != nil {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		if len(out) >= maxHits {
			return filepath.SkipAll
		}
		return nil
	})
	return out, err
}

// isJavaBasename reports whether path looks like a bare Java
// basename (no directory separator, ends with ".java"). Java stack
// traces carry file names without paths, so this check tells the
// validator to route the frame through the basename resolver instead
// of direct os.Stat.
func isJavaBasename(path string) bool {
	if path == "" {
		return false
	}
	if strings.ContainsAny(path, "/\\") {
		return false
	}
	return strings.HasSuffix(path, ".java")
}
