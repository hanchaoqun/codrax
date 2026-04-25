package logtriage

import (
	"path/filepath"
	"strings"
)

// ResolveSwiftFile ranks candidate repo-relative paths against a Swift
// frame's optional module/package and basename. Tier order:
//  1. Module-root layout under Sources/<Module>/ or Tests/<Module>/
//  2. Directory suffix matches the package-as-path form
//  3. Basename-only fallback
func ResolveSwiftFile(pkg, baseName string, candidates []string) []string {
	return rankByPackageAndRoots(pkg, baseName, candidates,
		[]string{"Sources/", "Tests/"},
		nil)
}

// ResolveLuaFile ranks candidate repo-relative paths for Lua stack
// frames. It prefers canonical source roots (`lua/`, `lib/`, `src/`)
// and package suffix matches when the frame carries a dotted module
// namespace.
func ResolveLuaFile(pkg, baseName string, candidates []string) []string {
	return rankByPackageAndRoots(pkg, baseName, candidates,
		[]string{"lua/", "lib/", "src/"},
		nil)
}

// ResolveProtoFile ranks candidate repo-relative paths for protobuf /
// RPC stack traces and codegen diagnostics. Package suffix matches win,
// then common proto roots, then basename fallback.
func ResolveProtoFile(pkg, baseName string, candidates []string) []string {
	return rankByPackageAndRoots(pkg, baseName, candidates,
		[]string{"proto/", "protos/", "api/", "idl/", "schemas/"},
		nil)
}

func isSwiftBasename(path string) bool {
	if path == "" || strings.ContainsAny(path, "/\\") {
		return false
	}
	return strings.HasSuffix(path, ".swift")
}

func isLuaBasename(path string) bool {
	if path == "" || strings.ContainsAny(path, "/\\") {
		return false
	}
	return strings.HasSuffix(path, ".lua")
}

func isProtoBasename(path string) bool {
	if path == "" || strings.ContainsAny(path, "/\\") {
		return false
	}
	return strings.HasSuffix(path, ".proto")
}

func rankByPackageAndRoots(pkg, baseName string, candidates, roots []string, extra func(norm, dir, pkgPath string) int) []string {
	if baseName == "" || len(candidates) == 0 {
		return nil
	}
	pkgPath := strings.ReplaceAll(strings.ReplaceAll(pkg, ".", "/"), "::", "/")

	type scored struct {
		path string
		tier int
		root int
		sort int
	}
	var hits []scored
	seen := make(map[string]bool, len(candidates))

	for _, cand := range candidates {
		if seen[cand] {
			continue
		}
		seen[cand] = true
		if filepath.Base(cand) != baseName {
			continue
		}
		norm := filepath.ToSlash(cand)
		dir := filepath.ToSlash(filepath.Dir(norm))
		rootRank := len(roots) + 1
		for idx, root := range roots {
			if strings.Contains(norm, root) {
				rootRank = idx
				break
			}
		}

		tier := 3
		if pkgPath != "" {
			if strings.HasSuffix(dir, "/"+pkgPath) || dir == pkgPath {
				tier = 1
			} else {
				for _, root := range roots {
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
		if tier == 3 {
			for _, root := range roots {
				if strings.Contains(norm, root) {
					tier = 2
					break
				}
			}
		}
		if extra != nil {
			if extTier := extra(norm, dir, pkgPath); extTier > 0 && extTier < tier {
				tier = extTier
			}
		}
		hits = append(hits, scored{path: cand, tier: tier, root: rootRank, sort: len(norm)})
	}
	if len(hits) == 0 {
		return nil
	}

	for i := 0; i < len(hits); i++ {
		for j := i + 1; j < len(hits); j++ {
			if hits[j].tier < hits[i].tier ||
				(hits[j].tier == hits[i].tier && hits[j].root < hits[i].root) ||
				(hits[j].tier == hits[i].tier && hits[j].root == hits[i].root && hits[j].sort < hits[i].sort) ||
				(hits[j].tier == hits[i].tier && hits[j].root == hits[i].root && hits[j].sort == hits[i].sort && hits[j].path < hits[i].path) {
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
