package repomap

import (
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/index"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/retrieve"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func projectedGraphFromBusContext(ctx *types.BusContext, parentRoot, scopeRoot, query string) (*Graph, bool, error) {
	if ctx == nil {
		return nil, false, nil
	}
	if ctx.Mutable != nil {
		if g, ok, err := projectedGraphFromMutable(ctx.Mutable, parentRoot, scopeRoot, query); ok || err != nil {
			return g, ok, err
		}
	}
	if g, ok := ctx.SearchGraph.(*Graph); ok {
		for _, parent := range projectionParentCandidates(g.Root, parentRoot, scopeRoot) {
			projected, err := projectGraphToRoot(g, parent, scopeRoot, query)
			if err != nil {
				return nil, false, err
			}
			if projected != nil {
				logging.Info("repo_map: projected scoped graph from bus in-memory graph (%d files)", projected.Metadata.FileCount)
				return cloneGraphForRanking(projected), true, nil
			}
		}
	}
	return nil, false, nil
}

func projectedGraphFromAgentContext(ctx *types.AgentContext, parentRoot, scopeRoot, query string) (*Graph, bool, error) {
	if ctx == nil {
		return nil, false, nil
	}
	if ctx.Mutable != nil {
		if g, ok, err := projectedGraphFromMutable(ctx.Mutable, parentRoot, scopeRoot, query); ok || err != nil {
			return g, ok, err
		}
	}
	if g, ok := ctx.SearchGraph.(*Graph); ok {
		for _, parent := range projectionParentCandidates(g.Root, parentRoot, scopeRoot) {
			projected, err := projectGraphToRoot(g, parent, scopeRoot, query)
			if err != nil {
				return nil, false, err
			}
			if projected != nil {
				logging.Info("repo_map: projected scoped graph from agent in-memory graph (%d files)", projected.Metadata.FileCount)
				return projected, true, nil
			}
		}
	}
	return nil, false, nil
}

func projectedGraphFromMutable(mut *types.MutableState, parentRoot, scopeRoot, query string) (*Graph, bool, error) {
	if mut == nil {
		return nil, false, nil
	}
	base, ok := mut.SearchGraph().(*Graph)
	if !ok || base == nil {
		return nil, false, nil
	}
	parents := projectionParentCandidates(base.Root, parentRoot, scopeRoot)
	for _, parent := range parents {
		key := scopedGraphProjectionCacheKey(parent, scopeRoot, query)
		if key == "" {
			continue
		}
		if cached, ok := mut.ScopedSearchGraph(key).(*Graph); ok && cached != nil {
			clone := cloneGraphForRanking(cached)
			logging.Info("repo_map: reused scoped in-memory graph (%d files)", clone.Metadata.FileCount)
			return clone, true, nil
		}
	}
	for _, parent := range parents {
		projected, err := projectGraphToRoot(base, parent, scopeRoot, query)
		if err != nil {
			return nil, false, err
		}
		if projected == nil {
			continue
		}
		if key := scopedGraphProjectionCacheKey(parent, scopeRoot, query); key != "" {
			mut.SetScopedSearchGraph(key, projected)
		}
		logging.Info("repo_map: projected scoped graph from in-memory graph (%d files)", projected.Metadata.FileCount)
		return cloneGraphForRanking(projected), true, nil
	}
	return nil, false, nil
}

func projectionParentCandidates(baseRoot, parentRoot, scopeRoot string) []string {
	var out []string
	add := func(root string) {
		root = strings.TrimSpace(root)
		if root == "" {
			return
		}
		for _, existing := range out {
			if sameRepoMapRoot(existing, root) {
				return
			}
		}
		out = append(out, root)
	}
	add(parentRoot)
	if baseRoot != "" {
		if _, ok := repoMapRelPathWithinRoot(baseRoot, scopeRoot); ok {
			add(baseRoot)
		}
	}
	return out
}

func scopedGraphProjectionCacheKey(parentRoot, scopeRoot, query string) string {
	parent, errParent := canonicalRepoMapPath(parentRoot)
	scope, errScope := canonicalRepoMapPath(scopeRoot)
	if errParent != nil || errScope != nil || parent == "" || scope == "" {
		return ""
	}
	return parent + "\x00" + scope + "\x00" + query
}

func projectGraphToRoot(base *Graph, parentRoot, scopeRoot, query string) (*Graph, error) {
	if base == nil || strings.TrimSpace(base.Root) == "" {
		return nil, nil
	}
	if parentRoot == "" {
		parentRoot = base.Root
	}
	if !sameRepoMapRoot(base.Root, parentRoot) {
		return nil, nil
	}
	scopeRel, ok := repoMapRelPathWithinRoot(parentRoot, scopeRoot)
	if !ok {
		return nil, nil
	}
	scopeRel = strings.Trim(strings.ReplaceAll(scopeRel, `\`, `/`), "/")
	if scopeRel == "" || scopeRel == "." {
		return cloneGraphForRanking(base), nil
	}
	scopeRootAbs, err := canonicalRepoMapPath(scopeRoot)
	if err != nil {
		return nil, err
	}
	projectedFiles := make([]*FileInfo, 0)
	for _, fi := range base.Files {
		if fi == nil || !projectGraphFileInScope(fi.RelPath, scopeRel) {
			continue
		}
		cloned, ok := cloneFileInfoForScope(fi, scopeRel)
		if !ok {
			continue
		}
		projectedFiles = append(projectedFiles, cloned)
	}
	if len(projectedFiles) == 0 {
		return nil, nil
	}
	graph := index.BuildGraph(scopeRootAbs, projectedFiles)
	retrieveRankGraph(graph, query)
	// BuildGraph rebuilt Metadata wholesale; stamp the projection
	// observation after it. Cached projections keep this stamp on
	// later reuse — which is accurate: they are in-memory state.
	graph.Metadata.IndexStatus.Source = IndexSourceScopedProjection
	graph.Metadata.IndexStatus.Freshness = IndexFreshnessReused
	return graph, nil
}

func retrieveRankGraph(graph *Graph, query string) {
	if graph == nil {
		return
	}
	// Keep the retrieve dependency isolated in multigraph_facade.go for most
	// callers; scoped projections still need the same ranking behavior.
	retrieve.RankGraph(graph, query)
}

func cloneFileInfoForScope(fi *FileInfo, scopeRel string) (*FileInfo, bool) {
	rel, ok := projectGraphRebaseRelPath(fi.RelPath, scopeRel)
	if !ok {
		return nil, false
	}
	cloned := *fi
	cloned.RelPath = rel
	cloned.Symbols = cloneSymbolsForScope(fi.Symbols, scopeRel)
	cloned.Imports = cloneImportsForScope(fi.Imports, scopeRel)
	cloned.Relations = cloneRelationsForScope(fi.Relations, scopeRel)
	if fi.LineFeatures != nil {
		cloned.LineFeatures = make(map[int][]rmtypes.LineFeature, len(fi.LineFeatures))
		for line, features := range fi.LineFeatures {
			cloned.LineFeatures[line] = append([]rmtypes.LineFeature(nil), features...)
		}
	}
	return &cloned, true
}

func cloneSymbolsForScope(symbols []Symbol, scopeRel string) []Symbol {
	if len(symbols) == 0 {
		return nil
	}
	out := make([]Symbol, 0, len(symbols))
	for _, sym := range symbols {
		if rel, ok := projectGraphRebaseRelPath(sym.File, scopeRel); ok {
			sym.File = rel
		}
		sym.RequiredMethods = append([]string(nil), sym.RequiredMethods...)
		sym.Implements = append([]rmtypes.SymbolID(nil), sym.Implements...)
		out = append(out, sym)
	}
	return out
}

func cloneImportsForScope(imports []Import, scopeRel string) []Import {
	if len(imports) == 0 {
		return nil
	}
	out := make([]Import, 0, len(imports))
	for _, imp := range imports {
		if rel, ok := projectGraphRebaseRelPath(imp.File, scopeRel); ok {
			imp.File = rel
		}
		out = append(out, imp)
	}
	return out
}

func cloneRelationsForScope(relations []Relation, scopeRel string) []Relation {
	if len(relations) == 0 {
		return nil
	}
	out := make([]Relation, 0, len(relations))
	for _, rel := range relations {
		if !projectGraphFileInScope(rel.File, scopeRel) {
			continue
		}
		if rebased, ok := projectGraphRebaseRelPath(rel.File, scopeRel); ok {
			rel.File = rebased
		}
		if rel.FromEP.File != "" {
			if rebased, ok := projectGraphRebaseRelPath(rel.FromEP.File, scopeRel); ok {
				rel.FromEP.File = rebased
			}
		}
		if rel.ToEP.File != "" {
			if rebased, ok := projectGraphRebaseRelPath(rel.ToEP.File, scopeRel); ok {
				rel.ToEP.File = rebased
			} else {
				continue
			}
		}
		out = append(out, rel)
	}
	return out
}

func projectGraphFileInScope(file, scopeRel string) bool {
	file = strings.Trim(strings.ReplaceAll(strings.TrimSpace(file), `\`, `/`), "/")
	scopeRel = strings.Trim(strings.ReplaceAll(strings.TrimSpace(scopeRel), `\`, `/`), "/")
	if file == "" || scopeRel == "" || scopeRel == "." {
		return file != ""
	}
	return file == scopeRel || strings.HasPrefix(file, scopeRel+"/")
}

func projectGraphRebaseRelPath(file, scopeRel string) (string, bool) {
	file = strings.Trim(strings.ReplaceAll(strings.TrimSpace(file), `\`, `/`), "/")
	scopeRel = strings.Trim(strings.ReplaceAll(strings.TrimSpace(scopeRel), `\`, `/`), "/")
	if file == "" {
		return "", false
	}
	if scopeRel == "" || scopeRel == "." {
		return file, true
	}
	if file == scopeRel {
		return filepath.Base(file), true
	}
	prefix := scopeRel + "/"
	if !strings.HasPrefix(file, prefix) {
		return "", false
	}
	rel := strings.TrimPrefix(file, prefix)
	return strings.Trim(rel, "/"), rel != ""
}
