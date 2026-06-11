package types

import "path/filepath"

// DeriveSymbolID rebuilds a Symbol's canonical SymbolID from the
// containing FileInfo plus the symbol's own fields. Called from
// index.BuildGraph so cache-loaded symbols (which have no persisted
// ID) get identical IDs on every rebuild.
//
// Rules:
//   - lang: fi.Language, unless empty in which case the ID is empty
//     (we cannot uniquely identify symbols in un-parsed files).
//   - pkg: fi.Package for Go/Java; for languages without a declared
//     package concept (JS, Python file-scoped, C), we substitute
//     filepath.Dir(fi.RelPath) so two functions with the same name
//     in different directories still get distinct IDs.
//   - receiver: Symbol.Receiver when set (Go methods), falling back
//     to Symbol.Parent (Java/Python class members).
//   - name: Symbol.Name.
//   - arity: Symbol.Arity (0 for types/consts/vars/fields).
//
// Returns "" if lang is unknown so callers can skip indexing.
func DeriveSymbolID(fi *FileInfo, s *Symbol) SymbolID {
	if fi == nil || s == nil || fi.Language == "" || s.Name == "" {
		return ""
	}
	pkg := fi.Package
	if pkg == "" {
		pkg = filepath.Dir(fi.RelPath)
	}
	receiver := s.Receiver
	if receiver == "" {
		receiver = s.Parent
	}
	return MakeSymbolID(fi.Language, pkg, receiver, s.Name, s.Arity)
}

// AppendUnique appends item to slice only when it is not already
// present, preserving insertion order. Linear in slice length, so
// callers with large slices should prefer a set and a conversion
// pass; repomap's graph-edge use case is small enough that this
// simple form is faster in practice.
func AppendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

// SymbolKey returns a unique key for a symbol: "file:Name" or "file:Receiver.Name"
func SymbolKey(s *Symbol) string {
	if s.Receiver != "" {
		return s.File + ":" + s.Receiver + "." + s.Name
	}
	if s.Parent != "" {
		return s.File + ":" + s.Parent + "." + s.Name
	}
	return s.File + ":" + s.Name
}

// FilesImporting returns all files that directly import the given file.
func (g *Graph) FilesImporting(file string) []string {
	return g.ReverseImports[file]
}

// FilesImportedBy returns all files directly imported by the given file.
func (g *Graph) FilesImportedBy(file string) []string {
	return g.ImportGraph[file]
}

// SymbolsInFile returns all symbols defined in a file.
func (g *Graph) SymbolsInFile(file string) []Symbol {
	fi, ok := g.FileIndex[file]
	if !ok {
		return nil
	}
	return fi.Symbols
}

// SymbolExists reports whether `name` resolves to at least one
// definition in the graph; minTier is the LOWEST (=most reliable)
// parse tier across matches. Lower tier = more reliable parse
// (Tier 1 = primary grammar; Tier 4 = path-only).
//
// Backs the types.SymbolOracle contract (commit 52); the wrapper
// in repomap/oracle.go just adapts this method to the interface
// shape.
//
// ImplementersOf returns the SymbolIDs of every concrete type
// that satisfies the named interface / trait. Phase 6 P0 batch
// (2026-05-03) — typed replacement for byte-grep searches that
// asked "which types implement interface X". Reads the
// Symbol.Implements list populated by the populateImplementers
// post-pass against every interface whose Name == name.
//
// Returns nil for unknown names, nil receiver, or files that have
// not yet been indexed. Callers can pass either a bare
// `LoopController` style name or a fully-qualified `pkg.IFace`
// — the lookup matches against both Symbol.Name (bare form, the
// declaration site) and the resolved SymbolID.
func (g *Graph) ImplementersOf(name string) []SymbolID {
	if g == nil || name == "" {
		return nil
	}
	defs, ok := g.SymbolDefs[name]
	if !ok || len(defs) == 0 {
		return nil
	}
	wanted := make(map[SymbolID]bool, len(defs))
	for _, d := range defs {
		if d == nil || d.ID == "" {
			continue
		}
		// Only interface / trait / protocol declarations qualify as
		// the "interface side" of the implements relation.
		switch d.Kind {
		case "interface", "trait", "protocol":
			wanted[d.ID] = true
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	var out []SymbolID
	seen := make(map[SymbolID]bool)
	for _, fi := range g.Files {
		if fi == nil {
			continue
		}
		for i := range fi.Symbols {
			sym := &fi.Symbols[i]
			if len(sym.Implements) == 0 {
				continue
			}
			for _, ifaceID := range sym.Implements {
				if !wanted[ifaceID] {
					continue
				}
				if !seen[sym.ID] {
					seen[sym.ID] = true
					out = append(out, sym.ID)
				}
			}
		}
	}
	return out
}

// LineFeaturesAt returns the typed AST node-shape features
// observed at `file:line` (1-based). Phase 6 stage 18 (2026-05-03)
// typed replacement for explorer-side source-line token tables.
//
// Empty slice ⇒ no features recorded (regex-only Tier 3+ fallback,
// or AST extraction not yet wired for this language). Callers
// treat the empty case as "no signal" and skip the dependent
// branch rather than guessing via byte tokens. The caller-side
// migration of explorer.go's isBlockTerminator + isEvidenceLine
// uses LineFeature.IsBlockTerminator() / IsEvidenceShape() helpers
// on the returned slice.
//
// nil receiver / unknown file / line out of range tolerated: all
// return nil so callers don't need defensive checks.
func (g *Graph) LineFeaturesAt(file string, line int) []LineFeature {
	if g == nil || line <= 0 || file == "" {
		return nil
	}
	fi, ok := g.FileIndex[file]
	if !ok || fi == nil || len(fi.LineFeatures) == 0 {
		return nil
	}
	return fi.LineFeatures[line]
}

// HasLineFeatureAt reports whether `feat` was observed at
// `file:line`. Convenience wrapper for the common "is this line
// a return statement?" / "is this line a call expression?" check.
func (g *Graph) HasLineFeatureAt(file string, line int, feat LineFeature) bool {
	for _, f := range g.LineFeaturesAt(file, line) {
		if f == feat {
			return true
		}
	}
	return false
}

// nil receiver tolerated: returns (false, 0) so callers can pass
// nil to disable validation without nil-checking each site.
func (g *Graph) SymbolExists(name string) (bool, int) {
	if g == nil {
		return false, 0
	}
	defs, ok := g.SymbolDefs[name]
	if !ok || len(defs) == 0 {
		return false, 0
	}
	minTier := 0
	for _, sym := range defs {
		if sym == nil {
			continue
		}
		fi, ok := g.FileIndex[sym.File]
		if !ok || fi == nil {
			continue
		}
		tier := fi.ParseTier
		if tier <= 0 {
			tier = 1
		}
		if minTier == 0 || tier < minTier {
			minTier = tier
		}
	}
	if minTier == 0 {
		// Defs entry exists but no FileInfo — stale index. Conservative fallback.
		return true, 4
	}
	return true, minTier
}

// CallersOf returns files that call the given symbol name.
//
// Legacy name-only resolver: when two methods in different files
// share a name (the receiver drift corpus — `Execute`, `Name`,
// `String`, etc.), a call to any of them credits every file that
// defines a same-named method. Post-Phase-1, consumers should
// prefer CallersOfID which filters by the canonical (pkg, receiver,
// name) tuple. CallersOf is kept for legacy consumers migrated in
// P1.3 and deleted in P1.4.
func (g *Graph) CallersOf(symbolName string) []string {
	var callers []string
	seen := make(map[string]bool)
	for _, fi := range g.Files {
		for _, rel := range fi.Relations {
			if rel.Kind == "call" && rel.To == symbolName && !seen[rel.File] {
				seen[rel.File] = true
				callers = append(callers, rel.File)
			}
		}
	}
	return callers
}

// CallersOfID is the receiver-aware caller resolver. Given a
// canonical SymbolID (typically a method with a non-empty receiver
// type), it walks every call relation and includes only the files
// whose call site's ToEP.Receiver matches the target's receiver.
// Calls whose receiver is unresolved (empty ToEP.Receiver) are
// conservatively included as potential callers — we would rather
// over-report than silently drop legitimate edges. When the target
// is a bare function (receiver empty), only calls with empty
// ToEP.Receiver are included.
//
// Returns nil when the SymbolID is not in SymbolByID.
func (g *Graph) CallersOfID(id SymbolID) []string {
	target, ok := g.SymbolByID[id]
	if !ok {
		return nil
	}
	wantRecv := target.Receiver
	if wantRecv == "" {
		wantRecv = target.Parent
	}
	var callers []string
	seen := make(map[string]bool)
	for _, fi := range g.Files {
		for _, rel := range fi.Relations {
			if rel.Kind != "call" || rel.To != target.Name {
				continue
			}
			gotRecv := rel.ToEP.Receiver
			// Filter by receiver:
			//   - target is a method (wantRecv != ""): accept calls
			//     whose receiver matches wantRecv OR is empty
			//     (unresolved, best-effort).
			//   - target is a bare function (wantRecv == ""): accept
			//     only calls whose receiver is also empty.
			if wantRecv == "" && gotRecv != "" {
				continue
			}
			if wantRecv != "" && gotRecv != "" && gotRecv != wantRecv {
				continue
			}
			if !seen[rel.File] {
				seen[rel.File] = true
				callers = append(callers, rel.File)
			}
		}
	}
	return callers
}

// ResolveCallTarget maps a call Relation to its canonical target
// Symbol via MethodIndex. Returns nil when the call is unresolved
// (external package, unknown receiver, or ambiguous pre-Phase-1
// selector_expression that the extractor could not attribute to a
// specific type).
//
// Resolution order:
//  1. If ToEP.Receiver matches a type/receiver in fi.Package via
//     MethodIndex, use it.
//  2. Else if ToEP.Receiver is empty and fi.Package has a bare
//     function with this name, use it.
//  3. Else try every package for (pkg, ToEP.Receiver, name) — this
//     catches cross-package method calls where the receiver was
//     scope-resolved to a type that lives in another package.
//  4. Give up; caller falls back to name-only accounting.
//
// Exported (was resolveCallTarget) so retrieve.RankGraph in a
// different package can call it.
func (g *Graph) ResolveCallTarget(fi *FileInfo, rel Relation) *Symbol {
	if rel.Kind != "call" {
		return nil
	}
	name := rel.ToEP.Name
	if name == "" {
		name = rel.To
	}
	if name == "" {
		return nil
	}
	recv := rel.ToEP.Receiver
	pkg := fi.Package
	if pkg == "" {
		pkg = filepath.Dir(fi.RelPath)
	}
	// Same-package receiver match.
	if s, ok := g.MethodIndex[MethodKey{Pkg: pkg, Receiver: recv, Name: name}]; ok {
		return s
	}
	// Same-package bare function fallback.
	if recv != "" {
		if s, ok := g.MethodIndex[MethodKey{Pkg: pkg, Receiver: "", Name: name}]; ok {
			return s
		}
	}
	// Cross-package: a method with this (receiver, name) defined in
	// any package. Resolved through a memoized (receiver, name) index
	// instead of scanning the whole MethodIndex per call — the latter
	// is O(methods) per relation and made BuildGraph quadratic on
	// large repos. The index picks the first definition in file order
	// (deterministic; the old map-iteration scan returned a random
	// match among duplicates).
	if recv != "" {
		if s := g.resolveReceiverName(recv, name); s != nil {
			return s
		}
	}
	return nil
}

// TransitiveDeps returns all files reachable from the given file via imports.
func (g *Graph) TransitiveDeps(file string, maxDepth int) []string {
	visited := make(map[string]bool)
	var result []string
	g.walkDeps(file, 0, maxDepth, visited, &result)
	return result
}

func (g *Graph) walkDeps(file string, depth, maxDepth int, visited map[string]bool, result *[]string) {
	if depth > maxDepth || visited[file] {
		return
	}
	visited[file] = true
	if depth > 0 {
		*result = append(*result, file)
	}
	for _, dep := range g.ImportGraph[file] {
		g.walkDeps(dep, depth+1, maxDepth, visited, result)
	}
}

// TransitiveReverseDeps returns all files that transitively depend on the given file.
func (g *Graph) TransitiveReverseDeps(file string, maxDepth int) []string {
	visited := make(map[string]bool)
	var result []string
	g.walkReverseDeps(file, 0, maxDepth, visited, &result)
	return result
}

func (g *Graph) walkReverseDeps(file string, depth, maxDepth int, visited map[string]bool, result *[]string) {
	if depth > maxDepth || visited[file] {
		return
	}
	visited[file] = true
	if depth > 0 {
		*result = append(*result, file)
	}
	for _, dep := range g.ReverseImports[file] {
		g.walkReverseDeps(dep, depth+1, maxDepth, visited, result)
	}
}
