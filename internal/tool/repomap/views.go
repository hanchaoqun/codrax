package repomap

import (
	"fmt"
	"sort"
	"strings"
)

// GenerateView produces a markdown view of the graph.
func GenerateView(g *Graph, viewType string, params ViewParams) string {
	switch viewType {
	case "overview":
		return viewOverview(g, params)
	case "file_map":
		return viewFileMap(g, params)
	case "task_map":
		return viewTaskMap(g, params)
	case "call_path":
		return viewCallPath(g, params)
	case "edit_impact":
		return viewEditImpact(g, params)
	default:
		return viewOverview(g, params)
	}
}

// --- Overview: module-level summary ---

func viewOverview(g *Graph, params ViewParams) string {
	var b strings.Builder
	b.WriteString("# Repository Overview\n\n")

	// Language distribution
	b.WriteString("## Languages\n\n")
	type langCount struct {
		lang  string
		count int
	}
	var langs []langCount
	for l, c := range g.Metadata.Languages {
		langs = append(langs, langCount{l, c})
	}
	sort.Slice(langs, func(i, j int) bool { return langs[i].count > langs[j].count })
	for _, lc := range langs {
		b.WriteString(fmt.Sprintf("- **%s**: %d files\n", lc.lang, lc.count))
	}

	// Special files
	if len(g.Metadata.SpecialFiles) > 0 {
		b.WriteString("\n## Project Files\n\n")
		for _, f := range g.Metadata.SpecialFiles {
			b.WriteString(fmt.Sprintf("- `%s`\n", f))
		}
	}

	// Module/package structure
	b.WriteString("\n## Packages/Modules\n\n")
	pkgs := make(map[string][]string) // package → files
	for _, fi := range g.Files {
		if fi.Package != "" {
			pkgs[fi.Package] = append(pkgs[fi.Package], fi.RelPath)
		}
	}
	// sort packages by file count
	type pkgInfo struct {
		name  string
		files []string
	}
	var pkgList []pkgInfo
	for name, files := range pkgs {
		pkgList = append(pkgList, pkgInfo{name, files})
	}
	sort.Slice(pkgList, func(i, j int) bool { return len(pkgList[i].files) > len(pkgList[j].files) })
	for _, p := range pkgList {
		symCount := 0
		for _, f := range p.files {
			if fi, ok := g.FileIndex[f]; ok {
				symCount += len(fi.Symbols)
			}
		}
		b.WriteString(fmt.Sprintf("- **%s** — %d files, %d symbols\n", p.name, len(p.files), symCount))
	}

	// Top files by importance
	topN := params.TopN
	if topN <= 0 {
		topN = 15
	}
	top := TopFiles(g, topN)
	b.WriteString(fmt.Sprintf("\n## Top %d Files (by importance)\n\n", topN))
	for i, fi := range top {
		score := g.Scores[fi.RelPath]
		exportedCount := 0
		for _, sym := range fi.Symbols {
			if sym.Exported {
				exportedCount++
			}
		}
		b.WriteString(fmt.Sprintf("%d. `%s` — %d symbols (%d exported), score %.1f\n",
			i+1, fi.RelPath, len(fi.Symbols), exportedCount, score))
	}

	b.WriteString(fmt.Sprintf("\n---\n*%d files, %d symbols, %d relations*\n",
		g.Metadata.FileCount, g.Metadata.SymbolCount, g.Metadata.RelationCount))
	return b.String()
}

// --- File Map: symbols per file ---

func viewFileMap(g *Graph, params ViewParams) string {
	var b strings.Builder
	b.WriteString("# File Map\n\n")

	topN := params.TopN
	if topN <= 0 {
		topN = 50
	}
	files := TopFiles(g, topN)

	for _, fi := range files {
		if len(fi.Symbols) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("## %s", fi.RelPath))
		if fi.Package != "" {
			b.WriteString(fmt.Sprintf(" [%s]", fi.Package))
		}
		b.WriteString("\n\n")

		// group by kind
		groups := make(map[string][]Symbol)
		for _, sym := range fi.Symbols {
			groups[sym.Kind] = append(groups[sym.Kind], sym)
		}

		kindOrder := []string{"interface", "trait", "class", "struct", "enum", "type", "function", "method", "const", "var", "field"}
		for _, kind := range kindOrder {
			syms, ok := groups[kind]
			if !ok {
				continue
			}
			for _, sym := range syms {
				marker := " "
				if sym.Exported {
					marker = "+"
				}
				line := fmt.Sprintf("- %s`%s`", marker, sym.Name)
				if sym.Receiver != "" {
					line = fmt.Sprintf("- %s`(%s) %s`", marker, sym.Receiver, sym.Name)
				} else if sym.Parent != "" {
					line = fmt.Sprintf("- %s`%s.%s`", marker, sym.Parent, sym.Name)
				}
				line += fmt.Sprintf(" %s :%d", sym.Kind, sym.Line)
				if sym.Signature != "" {
					line += " `" + sym.Signature + "`"
				}
				if sym.Doc != "" {
					line += " — " + sym.Doc
				}
				b.WriteString(line + "\n")
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// --- Task Map: local subgraph around a query ---

func viewTaskMap(g *Graph, params ViewParams) string {
	if params.Query == "" {
		return viewOverview(g, params)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Task Map: %s\n\n", params.Query))

	// Re-rank with query
	RankGraph(g, params.Query)

	topN := params.TopN
	if topN <= 0 {
		topN = 20
	}
	relevant := TopFiles(g, topN)

	b.WriteString("## Relevant Files\n\n")
	for _, fi := range relevant {
		score := g.Scores[fi.RelPath]
		if score <= 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("### %s (score: %.1f)\n\n", fi.RelPath, score))

		// show matching symbols
		query := strings.ToLower(params.Query)
		terms := strings.Fields(query)
		for _, sym := range fi.Symbols {
			matched := false
			nameLower := strings.ToLower(sym.Name)
			for _, term := range terms {
				if strings.Contains(nameLower, term) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			line := fmt.Sprintf("- `%s` %s :%d", sym.Name, sym.Kind, sym.Line)
			if sym.Signature != "" {
				line += " `" + sym.Signature + "`"
			}
			if sym.Doc != "" {
				line += " — " + sym.Doc
			}
			b.WriteString(line + "\n")
		}

		// show imports/dependents
		deps := g.FilesImportedBy(fi.RelPath)
		if len(deps) > 0 {
			b.WriteString(fmt.Sprintf("  - imports: %s\n", strings.Join(abbreviate(deps, 5), ", ")))
		}
		importers := g.FilesImporting(fi.RelPath)
		if len(importers) > 0 {
			b.WriteString(fmt.Sprintf("  - imported by: %s\n", strings.Join(abbreviate(importers, 5), ", ")))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// --- Call Path: from entrypoint to deep call chains ---

func viewCallPath(g *Graph, params ViewParams) string {
	var b strings.Builder

	entry := params.EntryPoint
	if entry == "" {
		// auto-detect: find main or highest-scored file
		top := TopFiles(g, 1)
		if len(top) > 0 {
			entry = top[0].RelPath
		}
	}

	b.WriteString(fmt.Sprintf("# Call Path from %s\n\n", entry))

	// BFS through import graph
	visited := make(map[string]bool)
	type queueItem struct {
		file  string
		depth int
	}
	queue := []queueItem{{entry, 0}}
	maxDepth := 5

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if visited[item.file] || item.depth > maxDepth {
			continue
		}
		visited[item.file] = true

		indent := strings.Repeat("  ", item.depth)
		fi := g.FileIndex[item.file]
		if fi == nil {
			b.WriteString(fmt.Sprintf("%s- `%s` (not in index)\n", indent, item.file))
			continue
		}

		// show key symbols
		var keySyms []string
		for _, sym := range fi.Symbols {
			if sym.Exported && (sym.Kind == "function" || sym.Kind == "method") {
				keySyms = append(keySyms, sym.Name)
			}
		}
		symStr := ""
		if len(keySyms) > 0 {
			symStr = " → " + strings.Join(abbreviate(keySyms, 5), ", ")
		}
		b.WriteString(fmt.Sprintf("%s- `%s`%s\n", indent, item.file, symStr))

		// enqueue dependencies
		for _, dep := range g.ImportGraph[item.file] {
			queue = append(queue, queueItem{dep, item.depth + 1})
		}
	}

	return b.String()
}

// --- Edit Impact: what would be affected by editing a file ---

func viewEditImpact(g *Graph, params ViewParams) string {
	target := params.TargetFile
	if target == "" {
		return "# Edit Impact\n\nNo target file specified.\n"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Edit Impact: %s\n\n", target))

	fi := g.FileIndex[target]
	if fi == nil {
		b.WriteString("File not found in index.\n")
		return b.String()
	}

	// Direct dependents
	directDeps := g.FilesImporting(target)
	b.WriteString("## Direct Dependents\n\n")
	if len(directDeps) == 0 {
		b.WriteString("No files directly import this file.\n")
	}
	for _, dep := range directDeps {
		b.WriteString(fmt.Sprintf("- `%s`\n", dep))
	}

	// Transitive dependents
	transitiveDeps := g.TransitiveReverseDeps(target, 5)
	if len(transitiveDeps) > len(directDeps) {
		b.WriteString("\n## Transitive Dependents (up to 5 levels)\n\n")
		for _, dep := range transitiveDeps {
			isDirectDep := false
			for _, d := range directDeps {
				if d == dep {
					isDirectDep = true
					break
				}
			}
			if !isDirectDep {
				b.WriteString(fmt.Sprintf("- `%s`\n", dep))
			}
		}
	}

	// Exported symbols that others reference
	b.WriteString("\n## Exported Symbols\n\n")
	for _, sym := range fi.Symbols {
		if !sym.Exported {
			continue
		}
		callers := g.CallersOf(sym.Name)
		refs := len(callers)
		marker := ""
		if refs > 0 {
			marker = fmt.Sprintf(" (referenced from %d files)", refs)
		}
		b.WriteString(fmt.Sprintf("- `%s` %s%s\n", sym.Name, sym.Kind, marker))
	}

	// Files this file depends on (editing might break if APIs change)
	deps := g.FilesImportedBy(target)
	if len(deps) > 0 {
		b.WriteString("\n## Dependencies (this file imports)\n\n")
		for _, dep := range deps {
			b.WriteString(fmt.Sprintf("- `%s`\n", dep))
		}
	}

	return b.String()
}

func abbreviate(items []string, maxN int) []string {
	if len(items) <= maxN {
		return items
	}
	out := make([]string, maxN)
	copy(out, items[:maxN])
	out = append(out, fmt.Sprintf("(+%d more)", len(items)-maxN))
	return out
}
