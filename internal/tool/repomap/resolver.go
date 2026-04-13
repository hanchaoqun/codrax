package repomap

import (
	"path/filepath"
	"strings"
)

// ImportResolver maps an Import statement to one or more target file
// RelPaths within a scanned Graph. One instance per language, held in
// a registry and dispatched by resolveImportGraph. Phase 2a replaces
// the flat language switch in resolveImport with dedicated resolvers
// per language; the scaffolding commit wires every language to a
// legacyResolver that delegates to the existing switch so behavior is
// unchanged while the new contract becomes the only code path.
type ImportResolver interface {
	// Language returns the language tag this resolver handles.
	Language() string

	// Prepare is called once after the Graph is built and before any
	// Resolve call. Implementations may parse build configuration
	// (go.mod, tsconfig.json, Cargo.toml) from ctx.SpecialFiles and
	// populate their own precomputed indices. Returning an error
	// surfaces a debug log but does not abort the scan — the
	// resolver falls through to best-effort resolution.
	Prepare(g *Graph, ctx *ResolverContext) error

	// Resolve returns target RelPaths for a single import in a
	// single file. An empty slice means the import is unresolved;
	// the dispatcher records an UnresolvedImport entry.
	Resolve(g *Graph, fi *FileInfo, imp Import, ctx *ResolverContext) []string
}

// ResolverContext is the shared precomputed state the graph builder
// hands to every resolver. Resolvers read all fields and may append
// to Unresolved, but must not mutate shared maps.
type ResolverContext struct {
	// PkgToFiles maps both declared package names and directory paths
	// to files. Kept for resolvers that still need the legacy lookup
	// shape during the per-language migration.
	PkgToFiles map[string][]string

	// BasenameIndex maps a file's basename (without extension) to
	// all files that share that basename. Used by the C/C++ resolver
	// and as a last-resort fallback.
	BasenameIndex map[string][]string

	// FileIndex is an alias of Graph.FileIndex, kept on the context
	// so resolvers can probe for a specific RelPath without touching
	// the Graph directly.
	FileIndex map[string]*FileInfo

	// SpecialFiles is the list of build-config file RelPaths
	// (`go.mod`, `tsconfig.json`, `Cargo.toml`, ...) the scanner
	// flagged on the way in. Resolvers use this for configuration
	// discovery.
	SpecialFiles []string

	// Unresolved accumulates imports that no resolver could map.
	// Populated by the dispatcher, not by resolvers directly —
	// a resolver only needs to return nil.
	Unresolved []UnresolvedImport
}

// newResolverContext builds the shared ResolverContext from the
// current Graph. The precomputed indices mirror what resolveImport's
// closures used to build inline.
func newResolverContext(g *Graph) *ResolverContext {
	pkgToFiles := make(map[string][]string)
	for _, fi := range g.Files {
		if fi.Package != "" {
			pkgToFiles[fi.Package] = append(pkgToFiles[fi.Package], fi.RelPath)
		}
		dir := filepath.Dir(fi.RelPath)
		pkgToFiles[dir] = append(pkgToFiles[dir], fi.RelPath)
	}
	basenameIndex := make(map[string][]string)
	for _, fi := range g.Files {
		base := strings.TrimSuffix(filepath.Base(fi.RelPath), filepath.Ext(fi.RelPath))
		basenameIndex[base] = append(basenameIndex[base], fi.RelPath)
	}
	return &ResolverContext{
		PkgToFiles:    pkgToFiles,
		BasenameIndex: basenameIndex,
		FileIndex:     g.FileIndex,
		SpecialFiles:  g.Metadata.SpecialFiles,
	}
}

// legacyResolver wraps the pre-Phase-2a resolveImport switch under
// the ImportResolver interface. The scaffolding commit registers one
// instance per language; Phase 2a commits 2–6 replace each
// registration with a dedicated resolver_*.go implementation.
type legacyResolver struct{ lang string }

func (r *legacyResolver) Language() string                       { return r.lang }
func (r *legacyResolver) Prepare(*Graph, *ResolverContext) error { return nil }

func (r *legacyResolver) Resolve(g *Graph, fi *FileInfo, imp Import, ctx *ResolverContext) []string {
	return resolveImport(g, fi, imp, ctx.PkgToFiles, ctx.BasenameIndex)
}

// defaultResolvers returns the Phase 2a dispatcher map. Languages
// that have a dedicated resolver are wired here; the rest still fall
// back to legacyResolver until their per-language commit lands.
//
// JS and TS share a single jsImportResolver instance so tsconfig
// parsing only runs once per BuildGraph.
func defaultResolvers() map[string]ImportResolver {
	js := &jsImportResolver{}
	return map[string]ImportResolver{
		LangGo:         &goImportResolver{},
		LangJava:       &javaImportResolver{},
		LangPython:     &pythonImportResolver{},
		LangJavaScript: js,
		LangTypeScript: js,
		LangRust:       &legacyResolver{lang: LangRust},
		LangC:          &legacyResolver{lang: LangC},
		LangCpp:        &legacyResolver{lang: LangCpp},
	}
}
