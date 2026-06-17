package impact

import (
	"path/filepath"
	"sort"
	"strings"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	codraxtypes "github.com/hanchaoqun/codrax/internal/types"
)

func GraphProviderFromSearchGraph(searchGraph any) GraphProvider {
	graph, ok := searchGraph.(*rmtypes.Graph)
	if !ok || graph == nil {
		return nil
	}
	return repomapGraphProvider{graph: graph}
}

type repomapGraphProvider struct {
	graph *rmtypes.Graph
}

func (p repomapGraphProvider) Imports(path string) []string {
	if p.graph == nil {
		return nil
	}
	return sortedStrings(p.graph.FilesImportedBy(normalizeImpactPath(path)))
}

func (p repomapGraphProvider) ReverseImports(path string) []string {
	if p.graph == nil {
		return nil
	}
	return sortedStrings(p.graph.FilesImporting(normalizeImpactPath(path)))
}

func (p repomapGraphProvider) RelatedTests(path string) []string {
	if p.graph == nil {
		return nil
	}
	path = normalizeImpactPath(path)
	if path == "" {
		return nil
	}
	base := sourceStem(path)
	dir := filepath.ToSlash(filepath.Dir(path))
	var out []string
	for rel := range p.graph.FileIndex {
		rel = normalizeImpactPath(rel)
		if rel == "" || !codraxtypes.LooksLikeTestFilePath(rel) {
			continue
		}
		if filepath.ToSlash(filepath.Dir(rel)) == dir || strings.Contains(sourceStem(rel), base) {
			out = append(out, rel)
		}
	}
	return sortedStrings(out)
}

func (p repomapGraphProvider) SymbolsInFile(path string) []SymbolRef {
	if p.graph == nil {
		return nil
	}
	symbols := p.graph.SymbolsInFile(normalizeImpactPath(path))
	out := make([]SymbolRef, 0, len(symbols))
	for _, sym := range symbols {
		out = append(out, SymbolRef{
			ID:      string(sym.ID),
			Name:    sym.Name,
			File:    sym.File,
			Line:    sym.Line,
			EndLine: sym.EndLine,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func sourceStem(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

func sortedStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, raw := range in {
		s := normalizeImpactPath(raw)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
