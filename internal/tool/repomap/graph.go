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
		ImportGraph:    make(map[string][]string),
		ReverseImports: make(map[string][]string),
		Scores:         make(map[string]float64),
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

		// index symbols by name
		for idx := range fi.Symbols {
			s := &fi.Symbols[idx]
			g.SymbolDefs[s.Name] = append(g.SymbolDefs[s.Name], s)
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
