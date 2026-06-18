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
	stems := relatedTestSourceStems(path)
	if len(stems) == 0 {
		return nil
	}
	dir := filepath.ToSlash(filepath.Dir(path))
	var candidates []relatedTestCandidate
	for rel := range p.graph.FileIndex {
		rel = normalizeImpactPath(rel)
		if rel == "" || !codraxtypes.LooksLikeTestFilePath(rel) {
			continue
		}
		score := 0
		if filepath.ToSlash(filepath.Dir(rel)) == dir {
			score = 100
		} else if relatedTestPackageMarkerPath(rel) {
			continue
		} else {
			score = relatedTestStemMatchScore(rel, stems)
		}
		if score > 0 {
			candidates = append(candidates, relatedTestCandidate{Path: rel, Score: score})
		}
	}
	return sortedRelatedTestCandidates(candidates, 32)
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

func (p repomapGraphProvider) LineFeaturesInRange(path string, startLine, endLine int) []string {
	if p.graph == nil || startLine <= 0 {
		return nil
	}
	if endLine < startLine {
		endLine = startLine
	}
	seen := map[string]bool{}
	var out []string
	path = normalizeImpactPath(path)
	for line := startLine; line <= endLine; line++ {
		for _, feature := range p.graph.LineFeaturesAt(path, line) {
			key := strings.TrimSpace(string(feature))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func sourceStem(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

type relatedTestCandidate struct {
	Path  string
	Score int
}

func relatedTestSourceStems(path string) []string {
	path = normalizeImpactPath(path)
	if path == "" {
		return nil
	}
	var out []string
	add := func(stem string) {
		stem = strings.TrimSpace(strings.ToLower(stem))
		if stem == "" || relatedTestGenericStem(stem) {
			return
		}
		for _, existing := range out {
			if existing == stem {
				return
			}
		}
		out = append(out, stem)
		if singular := strings.TrimSuffix(stem, "s"); singular != "" && singular != stem && len(singular) >= 3 {
			out = append(out, singular)
		}
	}
	add(sourceStem(path))
	dir := filepath.ToSlash(filepath.Dir(path))
	if dir != "." && dir != "/" && dir != "" {
		add(filepath.Base(dir))
	}
	return out
}

func relatedTestGenericStem(stem string) bool {
	switch strings.TrimSpace(strings.ToLower(stem)) {
	case "__init__", "index", "main", "mod", "lib", "package":
		return true
	default:
		return false
	}
}

func relatedTestPackageMarkerPath(path string) bool {
	switch strings.ToLower(filepath.Base(filepath.ToSlash(path))) {
	case "__init__.py", "index.js", "index.jsx", "index.ts", "index.tsx", "mod.rs", "lib.rs", "main.rs":
		return true
	default:
		return false
	}
}

func relatedTestStemMatchScore(path string, stems []string) int {
	testStem := sourceStem(path)
	testDir := filepath.Base(filepath.ToSlash(filepath.Dir(path)))
	score := 0
	for _, stem := range stems {
		if relatedTestTokenMatch(testStem, stem) {
			score = maxInt(score, 80)
		}
		if relatedTestTokenMatch(testDir, stem) {
			score = maxInt(score, 70)
		}
	}
	return score
}

func relatedTestTokenMatch(value, needle string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	needle = strings.TrimSpace(strings.ToLower(needle))
	if value == "" || needle == "" {
		return false
	}
	for _, token := range strings.FieldsFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		if token == needle {
			return true
		}
	}
	return false
}

func sortedRelatedTestCandidates(in []relatedTestCandidate, limit int) []string {
	seen := map[string]relatedTestCandidate{}
	for _, cand := range in {
		path := normalizeImpactPath(cand.Path)
		if path == "" {
			continue
		}
		cand.Path = path
		if prev, ok := seen[path]; ok && prev.Score >= cand.Score {
			continue
		}
		seen[path] = cand
	}
	out := make([]relatedTestCandidate, 0, len(seen))
	for _, cand := range seen {
		out = append(out, cand)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Path < out[j].Path
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	paths := make([]string, 0, len(out))
	for _, cand := range out {
		paths = append(paths, cand.Path)
	}
	return paths
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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
