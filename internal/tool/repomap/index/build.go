package index

import (
	"path/filepath"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// BuildGraph constructs the full repository graph from parsed files.
// Populates SymbolDefs (legacy name-keyed index), SymbolByID (the
// canonical drift-proof index), MethodIndex (receiver-aware method
// lookup), ImportGraph / ReverseImports (via resolveImportGraph),
// and the types.Metadata summary.
func BuildGraph(repoRoot string, files []*types.FileInfo) *types.Graph {
	g := &types.Graph{
		Root:           repoRoot,
		Files:          files,
		FileIndex:      make(map[string]*types.FileInfo, len(files)),
		SymbolDefs:     make(map[string][]*types.Symbol),
		SymbolByID:     make(map[types.SymbolID]*types.Symbol),
		MethodIndex:    make(map[types.MethodKey]*types.Symbol),
		ImportGraph:    make(map[string][]string),
		ReverseImports: make(map[string][]string),
		Scores:         make(map[string]float64),
		QueryScores:    make(map[string]float64),
	}

	langs := make(map[string]int)
	var specialFiles []string
	symCount := 0
	relCount := 0

	for _, fi := range files {
		g.FileIndex[fi.RelPath] = fi
		if fi.Language != "" {
			langs[fi.Language]++
		}
		if fi.IsSpecial {
			specialFiles = append(specialFiles, fi.RelPath)
		}
		symCount += len(fi.Symbols)
		relCount += len(fi.Relations)

		// index symbols by name AND by canonical types.SymbolID. The ID is
		// re-derived on every BuildGraph call from the containing
		// types.FileInfo's language/package + the symbol's Receiver/Parent/
		// Arity, so cached JSON that predates the ID field still
		// produces correct indices on load without a cache version
		// bump.
		for idx := range fi.Symbols {
			s := &fi.Symbols[idx]
			g.SymbolDefs[s.Name] = append(g.SymbolDefs[s.Name], s)
			s.ID = types.DeriveSymbolID(fi, s)
			if s.ID != "" {
				// Collision policy: first-wins. Two symbols with the
				// same canonical ID in the same package indicate
				// either a genuine duplicate (valid Go program would
				// not compile) or an extractor bug. Keep the first
				// and track the rest via SymbolDefs for diagnosis.
				if _, exists := g.SymbolByID[s.ID]; !exists {
					g.SymbolByID[s.ID] = s
				}
			}
			// MethodIndex: callable symbols (function, method) plus
			// types (for compat with receiver-resolution lookups
			// where a call site's "receiver" is actually a package-
			// level type name). Uses fi.Package directly so the key
			// matches what the call resolver hands it.
			pkg := fi.Package
			if pkg == "" {
				pkg = filepath.Dir(fi.RelPath)
			}
			key := types.MethodKey{Pkg: pkg, Receiver: s.Receiver, Name: s.Name}
			if s.Receiver == "" {
				key.Receiver = s.Parent
			}
			if _, exists := g.MethodIndex[key]; !exists {
				g.MethodIndex[key] = s
			}
		}
	}

	g.Metadata = types.Metadata{
		ScanTime:      time.Now(),
		FileCount:     len(files),
		SymbolCount:   symCount,
		RelationCount: relCount,
		Languages:     langs,
		SpecialFiles:  specialFiles,
	}

	resolveImportGraph(g)
	return g
}

// resolveImportGraph builds file→file import edges by dispatching
// each types.Import statement to the ImportResolver registered for its
// source language. Unresolvable imports are recorded on
// types.Graph.types.Metadata so consumers can compute per-import accuracy
// without re-walking the resolver.
func resolveImportGraph(g *types.Graph) {
	ctx := newResolverContext(g)
	resolvers := defaultResolvers()
	for _, r := range resolvers {
		if err := r.Prepare(g, ctx); err != nil {
			logging.Debug("repomap: resolver %s prepare: %v", r.Language(), err)
		}
	}
	for _, fi := range g.Files {
		r, ok := resolvers[fi.Language]
		if !ok {
			// No resolver registered for this language: treat every
			// import as unresolved so the harness can count it.
			for _, imp := range fi.Imports {
				ctx.Unresolved = append(ctx.Unresolved, types.UnresolvedImport{
					File: fi.RelPath, Raw: imp.Path, Reason: "no_resolver:" + fi.Language,
				})
			}
			continue
		}
		for _, imp := range fi.Imports {
			before := len(ctx.Unresolved)
			targets := r.Resolve(g, fi, imp, ctx)
			if len(targets) == 0 {
				// Only record a generic unresolved entry when the
				// resolver didn't already append a more specific
				// reason of its own.
				if len(ctx.Unresolved) == before {
					ctx.Unresolved = append(ctx.Unresolved, types.UnresolvedImport{
						File: fi.RelPath, Raw: imp.Path, Reason: fi.Language,
					})
				}
				continue
			}
			for _, t := range targets {
				g.ImportGraph[fi.RelPath] = types.AppendUnique(g.ImportGraph[fi.RelPath], t)
				g.ReverseImports[t] = types.AppendUnique(g.ReverseImports[t], fi.RelPath)
			}
		}
	}
	g.Metadata.UnresolvedImports = ctx.Unresolved
}

// resolveImport is the legacy per-language relative-path fallback
// used by the resolver plugins whose first tier is "try the file
// relative to the source file's directory". Kept here rather than
// in resolver.go so all the legacy branches live in one place.
func resolveImport(g *types.Graph, fi *types.FileInfo, imp types.Import, pkgToFiles map[string][]string, basenameIndex map[string][]string) []string {
	path := imp.Path

	// Direct file match (relative imports in JS/Python)
	if len(path) > 0 && path[0] == '.' {
		dir := filepath.Dir(fi.RelPath)
		resolved := filepath.Clean(filepath.Join(dir, path))
		// try with extensions
		for _, ext := range []string{"", ".go", ".py", ".js", ".ts", ".jsx", ".tsx", ".java", ".rs"} {
			candidate := resolved + ext
			if _, ok := g.FileIndex[candidate]; ok {
				return []string{candidate}
			}
		}
		// try as directory index
		for _, idx := range []string{"/index.js", "/index.ts", "/index.tsx", "/__init__.py"} {
			candidate := resolved + idx
			if _, ok := g.FileIndex[candidate]; ok {
				return []string{candidate}
			}
		}
		return nil
	}
	// Non-relative fallback is handled entirely by each per-language
	// resolver plugin in its own Resolve implementation.
	return nil
}
