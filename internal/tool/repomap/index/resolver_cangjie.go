package index

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
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
type cangjieImportResolver struct {
	// externalDeps holds the union of all external dependency names
	// declared across every `cjpm.toml [dependencies]` section in
	// the repo. An import whose package-path-head matches an entry
	// here resolves to nil (external — handled at build time by the
	// cjpm build cache, not by repomap).
	externalDeps map[string]bool
	// modulePkgs maps the `name` field in each cjpm.toml [package]
	// block → the repo-relative dir of that manifest. Used when a
	// Cangjie import path matches a declared module name — we can
	// then search that dir's .cj tree for the target package.
	modulePkgs map[string]string
}

func (r *cangjieImportResolver) Language() string { return types.LangCangjie }

// Prepare parses every cjpm.toml in the repo. Errors are logged at
// Debug level and never fatal — a missing or malformed manifest just
// means the external-dep gating is less precise for that module.
func (r *cangjieImportResolver) Prepare(g *types.Graph, ctx *ResolverContext) error {
	r.externalDeps = map[string]bool{}
	r.modulePkgs = map[string]string{}
	for _, sp := range ctx.SpecialFiles {
		if filepath.Base(sp) != "cjpm.toml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(g.Root, sp))
		if err != nil {
			logging.Debug("resolver_cangjie: read %s: %v", sp, err)
			continue
		}
		sections, err := ParseTOMLSubset(data)
		if err != nil {
			logging.Debug("resolver_cangjie: toml %s: %v", sp, err)
			continue
		}
		if pkg, ok := sections["package"]; ok {
			if name, ok := pkg["name"].(string); ok && name != "" {
				r.modulePkgs[name] = filepath.ToSlash(filepath.Dir(sp))
			}
		}
		// cjpm declares deps under either `[dependencies]` or
		// `[dependencies.<name>]` (the latter uses the key `name`
		// inside the subsection or omits it, using the section
		// name suffix as the dep name). We capture both shapes.
		if deps, ok := sections["dependencies"]; ok {
			for k := range deps {
				r.externalDeps[k] = true
			}
		}
		for sect := range sections {
			const prefix = "dependencies."
			if strings.HasPrefix(sect, prefix) {
				r.externalDeps[strings.TrimPrefix(sect, prefix)] = true
			}
		}
	}
	return nil
}

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

	// External dependencies declared in any cjpm.toml. These
	// resolve at cjpm-build time against the package cache, not
	// against repo sources — return nil so the dispatcher records
	// them as cjpm_external in Unresolved (the caller can then
	// distinguish them from genuine unresolved imports).
	parts := strings.Split(path, ".")
	if len(parts) > 0 && r.externalDeps[parts[0]] {
		return nil
	}

	// 1) Exact package lookup.
	if files, ok := ctx.PkgToFiles[path]; ok && len(files) > 0 {
		return files
	}

	// 2) Module-name match: an import whose first segment matches
	//    a local module's declared cjpm.toml [package].name goes
	//    to that module's directory — the dispatcher's legacy
	//    fallback then picks a .cj file under that tree.
	if len(parts) > 0 {
		if dir, ok := r.modulePkgs[parts[0]]; ok {
			// Collect .cj files under that dir by basename
			// intersection (the BasenameIndex is already bounded).
			var hits []string
			for _, f := range ctx.FileIndex {
				if filepath.Ext(f.RelPath) == ".cj" && strings.HasPrefix(f.RelPath, dir+"/") {
					hits = append(hits, f.RelPath)
				}
			}
			if len(hits) > 0 {
				return hits
			}
		}
	}

	// 3) Partial-prefix match: `std.collection.ArrayList` → look
	//    up `std.collection` if nothing matched the full path.
	//    Walk shrinking prefixes to support nested subpackage
	//    references.
	for i := len(parts) - 1; i > 0; i-- {
		prefix := strings.Join(parts[:i], ".")
		if files, ok := ctx.PkgToFiles[prefix]; ok && len(files) > 0 {
			return files
		}
	}

	// 4) Basename glob on the last component. `demo.cart.Cart` →
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
