package repomap

import (
	"path/filepath"
	"strings"
	"time"
)

// BuildGraph constructs the full repository graph from parsed files.
func BuildGraph(repoRoot string, files []*FileInfo) *Graph {
	g := &Graph{
		Root:           repoRoot,
		Files:          files,
		FileIndex:      make(map[string]*FileInfo, len(files)),
		SymbolDefs:     make(map[string][]*Symbol),
		SymbolByID:     make(map[SymbolID]*Symbol),
		MethodIndex:    make(map[MethodKey]*Symbol),
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

		// index symbols by name AND by canonical SymbolID. The ID is
		// re-derived on every BuildGraph call from the containing
		// FileInfo's language/package + the symbol's Receiver/Parent/
		// Arity, so cached JSON that predates the ID field still
		// produces correct indices on load without a cache version
		// bump.
		for idx := range fi.Symbols {
			s := &fi.Symbols[idx]
			g.SymbolDefs[s.Name] = append(g.SymbolDefs[s.Name], s)
			s.ID = deriveSymbolID(fi, s)
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
			key := MethodKey{Pkg: pkg, Receiver: s.Receiver, Name: s.Name}
			if s.Receiver == "" {
				key.Receiver = s.Parent
			}
			if _, exists := g.MethodIndex[key]; !exists {
				g.MethodIndex[key] = s
			}
		}
	}

	g.Metadata = Metadata{
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

// resolveImportGraph builds file→file import edges by matching import
// paths to actual files in the repo.
func resolveImportGraph(g *Graph) {
	// Build a lookup: package/module → files
	pkgToFiles := make(map[string][]string)
	for _, fi := range g.Files {
		if fi.Package != "" {
			pkgToFiles[fi.Package] = append(pkgToFiles[fi.Package], fi.RelPath)
		}
		// also index by directory
		dir := filepath.Dir(fi.RelPath)
		pkgToFiles[dir] = append(pkgToFiles[dir], fi.RelPath)
	}

	// basename index for header/source matching (C/C++)
	basenameIndex := make(map[string][]string)
	for _, fi := range g.Files {
		base := strings.TrimSuffix(filepath.Base(fi.RelPath), filepath.Ext(fi.RelPath))
		basenameIndex[base] = append(basenameIndex[base], fi.RelPath)
	}

	for _, fi := range g.Files {
		for _, imp := range fi.Imports {
			targets := resolveImport(g, fi, imp, pkgToFiles, basenameIndex)
			for _, t := range targets {
				g.ImportGraph[fi.RelPath] = appendUnique(g.ImportGraph[fi.RelPath], t)
				g.ReverseImports[t] = appendUnique(g.ReverseImports[t], fi.RelPath)
			}
		}
	}
}

func resolveImport(g *Graph, fi *FileInfo, imp Import, pkgToFiles map[string][]string, basenameIndex map[string][]string) []string {
	path := imp.Path

	// Direct file match (relative imports in JS/Python)
	if strings.HasPrefix(path, ".") {
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

	// Go-style: match import path suffix to directory structure
	if fi.Language == LangGo {
		// extract last path segment as package name
		parts := strings.Split(path, "/")
		pkgName := parts[len(parts)-1]
		if files, ok := pkgToFiles[pkgName]; ok {
			return files
		}
		// try matching suffix of import path to directory paths
		for dir, files := range pkgToFiles {
			if strings.HasSuffix(path, dir) || strings.HasSuffix(dir, path) {
				return files
			}
		}
		return nil
	}

	// Java: match package path
	if fi.Language == LangJava {
		javaPath := strings.ReplaceAll(path, ".", "/")
		for relPath := range g.FileIndex {
			if strings.Contains(relPath, javaPath) {
				return []string{relPath}
			}
		}
		return nil
	}

	// C/C++: header include → match by basename
	if fi.Language == LangC || fi.Language == LangCpp {
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if files, ok := basenameIndex[base]; ok {
			return files
		}
		return nil
	}

	// Python: dotted module path
	if fi.Language == LangPython {
		pyPath := strings.ReplaceAll(path, ".", "/")
		for _, ext := range []string{".py", "/__init__.py"} {
			candidate := pyPath + ext
			if _, ok := g.FileIndex[candidate]; ok {
				return []string{candidate}
			}
		}
		return nil
	}

	// Rust: match mod name
	if fi.Language == LangRust {
		dir := filepath.Dir(fi.RelPath)
		for _, candidate := range []string{
			filepath.Join(dir, path+".rs"),
			filepath.Join(dir, path, "mod.rs"),
		} {
			if _, ok := g.FileIndex[candidate]; ok {
				return []string{candidate}
			}
		}
		return nil
	}

	// JS/TS non-relative: node_modules, skip
	return nil
}

// deriveSymbolID rebuilds a Symbol's canonical SymbolID from the
// containing FileInfo plus the symbol's own fields. Called from
// BuildGraph so cache-loaded symbols (which have no persisted ID)
// get identical IDs on every rebuild.
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
func deriveSymbolID(fi *FileInfo, s *Symbol) SymbolID {
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

func appendUnique(slice []string, item string) []string {
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

// resolveCallTarget maps a call Relation to its canonical target
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
func (g *Graph) resolveCallTarget(fi *FileInfo, rel Relation) *Symbol {
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
	// Cross-package: scan every package for (pkg, receiver, name).
	// Deliberately linear because MethodIndex has one entry per
	// method and the typical repo has a few hundred packages; this
	// is still cheap compared to the full scan the extractor runs.
	if recv != "" {
		for k, s := range g.MethodIndex {
			if k.Receiver == recv && k.Name == name {
				return s
			}
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
