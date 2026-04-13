package index

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// pythonImportResolver resolves Python imports against a precomputed
// dotted module index built from every types.FileInfo's Python package
// attribute. The extract_python.go extractor already derives
// `fi.Package` from the file path (`internal/tool/repomap/x.py` →
// `internal.tool.repomap.x`); __init__.py files are normalized by
// stripping the trailing `.__init__` so `from foo.bar import X`
// resolves to `foo/bar/__init__.py`.
//
// Relative imports (`from .x import y`, `from ..x import y`) are
// delegated to the legacy relative-path branch in resolveImport,
// which already knows how to walk the filesystem.
//
// Non-relative imports that don't match any indexed module are
// classified `python_external`. We do not maintain a stdlib
// whitelist because from the repo's perspective stdlib, third-party
// packages, and typos are all equally "not in this repo" — honest
// classification over false precision.
type pythonImportResolver struct {
	moduleIndex map[string][]string // dotted module → []RelPath
}

func (r *pythonImportResolver) Language() string { return types.LangPython }

func (r *pythonImportResolver) Prepare(g *types.Graph, ctx *ResolverContext) error {
	r.moduleIndex = make(map[string][]string)
	for _, fi := range g.Files {
		if fi.Language != types.LangPython {
			continue
		}
		if fi.Package == "" {
			continue
		}
		mod := strings.TrimSuffix(fi.Package, ".__init__")
		r.moduleIndex[mod] = append(r.moduleIndex[mod], fi.RelPath)
	}
	return nil
}

func (r *pythonImportResolver) Resolve(g *types.Graph, fi *types.FileInfo, imp types.Import, ctx *ResolverContext) []string {
	path := imp.Path
	if path == "" {
		return nil
	}

	// Relative imports start with one or more dots. Delegate to the
	// legacy branch which resolves against filesystem layout.
	if strings.HasPrefix(path, ".") {
		return resolveImport(g, fi, imp, ctx.PkgToFiles, ctx.BasenameIndex)
	}

	// Exact match first — covers `import foo.bar` and `from foo.bar
	// import X` when `foo.bar` is itself a module.
	if files, ok := r.moduleIndex[path]; ok {
		return files
	}

	// `from foo.bar import baz` where `baz` is a submodule of
	// `foo.bar` (not a name defined in foo/bar/__init__.py). The
	// extractor records the imp.Path as the fully joined
	// `foo.bar.baz`, so an exact match above would have already
	// fired. But when `baz` is a name re-exported from elsewhere,
	// fall back to the parent package.
	if dot := strings.LastIndex(path, "."); dot > 0 {
		parent := path[:dot]
		if files, ok := r.moduleIndex[parent]; ok {
			return files
		}
	}

	ctx.Unresolved = append(ctx.Unresolved, types.UnresolvedImport{
		File: fi.RelPath, Raw: path, Reason: "python_external",
	})
	return nil
}
