package index

import (
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// cangjieImportResolver handles Cangjie imports. Cangjie import
// syntax: `import <package.path>` or `import <package.path> as <alias>`
// or `import <package.path>.{a, b, c}`.
//
// Resolution rules:
//
//  1. Black-hole the standard library and core packages (`std.*`,
//     `core.*`). These ship with the compiler, never map to repo
//     files.
//  2. Look up the dotted package path in PkgToFiles. Cangjie's
//     FileInfo.Package comes from the mandatory package_clause
//     (red line L-Cangjie-2), so PkgToFiles already contains a
//     canonical mapping from every in-repo package to its files.
//  3. If step 2 produces nothing, try the file-system fallback:
//     some Cangjie modules live under `src/<last_path_component>/`
//     so a plain basename glob on the last component can find
//     them.
//
// cjpm.toml parsing is intentionally not done at Tier 1 today.
// cjpm.toml declares external dependencies which by definition
// live outside the repo tree; mapping those to anything useful
// requires a populated build cache that codrax does not own.
// A future optimisation could parse cjpm.toml to decide which
// `std.*`-looking imports are actually local package overrides,
// but this is a narrow case.
type cangjieImportResolver struct{}

func (r *cangjieImportResolver) Language() string { return types.LangCangjie }

// Prepare is a no-op: cjpm.toml parsing is deferred (see package
// comment). We still satisfy the interface so the registration is
// symmetric with other resolvers.
func (r *cangjieImportResolver) Prepare(*types.Graph, *ResolverContext) error { return nil }

// Resolve walks PkgToFiles for the dotted package path, then falls
// back to basename globs on the final component.
func (r *cangjieImportResolver) Resolve(_ *types.Graph, _ *types.FileInfo, imp types.Import, ctx *ResolverContext) []string {
	path := strings.TrimSpace(imp.Path)
	if path == "" {
		return nil
	}
	if isCangjieBuiltin(path) {
		return nil
	}

	// 1) Exact package lookup.
	if files, ok := ctx.PkgToFiles[path]; ok && len(files) > 0 {
		return files
	}

	// 2) Partial-prefix match: `std.collection.ArrayList` → look
	//    up `std.collection` if nothing matched the full path.
	//    Walk shrinking prefixes to support nested subpackage
	//    references.
	parts := strings.Split(path, ".")
	for i := len(parts) - 1; i > 0; i-- {
		prefix := strings.Join(parts[:i], ".")
		if files, ok := ctx.PkgToFiles[prefix]; ok && len(files) > 0 {
			return files
		}
	}

	// 3) Basename glob on the last component. `demo.cart.Cart` →
	//    look for a `Cart` basename in repo. Useful when the
	//    package_clause is missing or truncated (Tier 2 salvage).
	last := parts[len(parts)-1]
	if files, ok := ctx.BasenameIndex[last]; ok && len(files) > 0 {
		out := make([]string, 0, len(files))
		for _, f := range files {
			if filepath.Ext(f) == ".cj" {
				out = append(out, f)
			}
		}
		if len(out) > 0 {
			return out
		}
	}

	return nil
}

// isCangjieBuiltin reports whether `path` is a Cangjie standard
// library or core package. Grep-able invariant: single producer.
//
// Known prefixes on the 1.0.0 LTS release surface:
//
//	std.*       — standard library
//	core.*      — core runtime
//	runtime.*   — runtime support
//	ohos.*      — HarmonyOS integration (Cangjie-side, distinct
//	              from ArkTS `@ohos.*` imports)
func isCangjieBuiltin(path string) bool {
	switch {
	case strings.HasPrefix(path, "std."):
		return true
	case strings.HasPrefix(path, "core."):
		return true
	case strings.HasPrefix(path, "runtime."):
		return true
	case path == "std" || path == "core" || path == "runtime":
		return true
	case strings.HasPrefix(path, "ohos."):
		return true
	}
	return false
}
