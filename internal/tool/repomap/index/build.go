package index

import (
	"fmt"
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
	populateImplementers(g)
	return g
}

// populateImplementers (Phase 6 P0 batch, 2026-05-03) walks every
// concrete type in the graph and matches its method set against
// every interface's RequiredMethods. When the concrete type's
// method set is a superset of the interface's required set, the
// interface SymbolID is appended to the concrete type's
// Implements slice.
//
// This is the typed replacement for byte-grep "implementers of
// interface X" searches. Solves Go's structural-typing case: a
// type T implementing interface I via method-set agreement
// without textually mentioning I. Other languages benefit too —
// Java/Kotlin's `class T implements I` already gives an explicit
// signal but the typed lookup remains uniform across languages.
//
// Performance: O(N_interfaces × N_concrete_types) per file pair,
// bounded in practice by the number of interfaces — codrax has
// ~50 interfaces and ~3K types, so the full pass is ~150K
// comparisons (microseconds).
//
// Per-package matching: a concrete type T at package P matches
// interface I at any package — Go's structural typing is
// package-agnostic. Cross-language matching is intentionally
// disabled (a Java interface should not match a Python class).
func populateImplementers(g *types.Graph) {
	if g == nil {
		return
	}
	type ifaceEntry struct {
		id  types.SymbolID
		req map[string]bool
		fi  *types.FileInfo
	}
	// Phase 6 P0b (2026-05-03) — back-fill RequiredMethods from
	// method-symbol Parent links for languages whose per-extractor
	// path doesn't populate it inline. Java / Kotlin / TS / ArkTS /
	// Swift / Python (ABC) / Cangjie all emit interface methods as
	// Symbol{Kind=method, Parent=interfaceName}; iterating once
	// over fi.Symbols and grouping by Parent gives the contract
	// signature without touching each extractor.
	//
	// Pre-populated RequiredMethods (Go's inline path via
	// goExtractInterfaceMethods) is preserved — back-fill only
	// when empty.
	for _, fi := range g.Files {
		if fi == nil {
			continue
		}
		var ifaceSymbols []*types.Symbol
		methodsByOwner := make(map[string][]string)
		for i := range fi.Symbols {
			sym := &fi.Symbols[i]
			switch sym.Kind {
			case "interface", "trait", "protocol":
				ifaceSymbols = append(ifaceSymbols, sym)
			case "method", "function":
				owner := sym.Parent
				if owner == "" {
					owner = sym.Receiver
				}
				if owner == "" {
					continue
				}
				key := fmt.Sprintf("%s(%d)", sym.Name, sym.Arity)
				methodsByOwner[owner] = append(methodsByOwner[owner], key)
			}
		}
		for _, iface := range ifaceSymbols {
			if len(iface.RequiredMethods) > 0 {
				continue
			}
			methods, ok := methodsByOwner[iface.Name]
			if !ok || len(methods) == 0 {
				continue
			}
			seen := make(map[string]bool, len(methods))
			for _, m := range methods {
				if !seen[m] {
					seen[m] = true
					iface.RequiredMethods = append(iface.RequiredMethods, m)
				}
			}
		}
	}

	// Group interfaces by language so cross-language matches are
	// excluded structurally.
	byLang := make(map[string][]ifaceEntry)
	for _, fi := range g.Files {
		if fi == nil {
			continue
		}
		for i := range fi.Symbols {
			sym := &fi.Symbols[i]
			if sym.Kind != "interface" && sym.Kind != "trait" && sym.Kind != "protocol" {
				continue
			}
			if len(sym.RequiredMethods) == 0 {
				continue
			}
			req := make(map[string]bool, len(sym.RequiredMethods))
			for _, m := range sym.RequiredMethods {
				req[m] = true
			}
			byLang[fi.Language] = append(byLang[fi.Language], ifaceEntry{
				id:  sym.ID,
				req: req,
				fi:  fi,
			})
		}
	}
	if len(byLang) == 0 {
		return
	}
	// For each file, group its method symbols by Receiver/Parent
	// to derive per-type method sets. Then match against same-
	// language interfaces.
	for _, fi := range g.Files {
		if fi == nil {
			continue
		}
		ifaces := byLang[fi.Language]
		if len(ifaces) == 0 {
			continue
		}
		methodsByOwner := buildMethodSetsForFile(fi)
		if len(methodsByOwner) == 0 {
			continue
		}
		for i := range fi.Symbols {
			sym := &fi.Symbols[i]
			if sym.Kind == "interface" || sym.Kind == "trait" {
				continue
			}
			ownerMethods, ok := methodsByOwner[sym.Name]
			if !ok || len(ownerMethods) == 0 {
				continue
			}
			for _, iface := range ifaces {
				if !ownerMethods.Implements(iface.req) {
					continue
				}
				sym.Implements = appendUniqueSymbolID(sym.Implements, iface.id)
			}
		}
	}
}

// methodSet is the per-owner-type bag of method signatures.
type methodSet map[string]bool

// Implements reports whether `s` is a superset of `required`.
func (s methodSet) Implements(required map[string]bool) bool {
	if len(required) == 0 {
		return false
	}
	for k := range required {
		if !s[k] {
			return false
		}
	}
	return true
}

// buildMethodSetsForFile groups method symbols by their owner
// (Receiver for Go, Parent for Java/Python/etc.). Returns
// owner-name → set-of-"name(arity)" strings.
func buildMethodSetsForFile(fi *types.FileInfo) map[string]methodSet {
	out := make(map[string]methodSet)
	for i := range fi.Symbols {
		sym := &fi.Symbols[i]
		if sym.Kind != "method" && sym.Kind != "function" {
			// Java/Kotlin/Python interface contracts may surface
			// methods under Kind="method" with Parent set; only
			// these participate in implements matching.
			if sym.Kind != "" {
				continue
			}
		}
		owner := sym.Receiver
		if owner == "" {
			owner = sym.Parent
		}
		if owner == "" {
			continue
		}
		key := fmt.Sprintf("%s(%d)", sym.Name, sym.Arity)
		set, ok := out[owner]
		if !ok {
			set = make(methodSet)
			out[owner] = set
		}
		set[key] = true
	}
	return out
}

// appendUniqueSymbolID appends id to ids if not already present.
func appendUniqueSymbolID(ids []types.SymbolID, id types.SymbolID) []types.SymbolID {
	if id == "" {
		return ids
	}
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
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
		// try with extensions. `.ets` / `.cj` / `.kt` added so
		// HarmonyOS ArkTS + Cangjie and Android Kotlin relative
		// imports resolve — the legacy fallback path is the
		// simplest way to make a `import X from './UserCard'` in
		// an ArkTS file find UserCard.ets alongside it.
		for _, ext := range []string{"", ".go", ".py", ".js", ".ts", ".jsx", ".tsx", ".java", ".rs", ".ets", ".cj", ".kt", ".kts"} {
			candidate := resolved + ext
			if _, ok := g.FileIndex[candidate]; ok {
				return []string{candidate}
			}
		}
		// try as directory index — HarmonyOS Stage Model convention:
		// pages/<PageName>/Index.ets is common.
		for _, idx := range []string{"/index.js", "/index.ts", "/index.tsx", "/__init__.py", "/Index.ets", "/index.ets"} {
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
