package multigraph

import (
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// multiGraphLocator implements types.SymbolLocator by fanning out
// across active sub-repos. Returned SymbolLocation.File values are
// rewritten to the path-from-parent form (sub-repo prefix prepended)
// so callers comparing positions across sub-repos do not collide on
// "main.go" or "lib.rs" entries that exist in two sibling sub-repos.
//
// Behaviour contract:
//   - LocateSymbol concatenates per-sub-repo results in iteration
//     order of MultiGraph.AllGraphs (no specific order — callers
//     must NOT rely on it).
//   - SymbolsInFile is owner-aware: file path-from-parent → owning
//     sub-repo's *Graph.SymbolsInFile (after stripping the prefix).
//     Cross-sub-repo file paths return nil (independent namespaces).
type multiGraphLocator struct {
	mg *MultiGraph

	mu     sync.Mutex
	cached map[string]types.SymbolLocator
}

type fallbackLocator struct {
	graph *rmtypes.Graph
}

func (l *fallbackLocator) LocateSymbol(name string) []types.SymbolLocation {
	if l == nil || l.graph == nil {
		return nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	defs := l.graph.SymbolDefs[name]
	if len(defs) == 0 {
		return nil
	}
	out := make([]types.SymbolLocation, 0, len(defs))
	for _, sym := range defs {
		if sym == nil {
			continue
		}
		out = append(out, types.SymbolLocation{
			File:    sym.File,
			Line:    sym.Line,
			EndLine: sym.EndLine,
		})
	}
	return out
}

func (l *fallbackLocator) SymbolsInFile(file string) []types.SymbolLocation {
	if l == nil || l.graph == nil {
		return nil
	}
	syms := l.graph.SymbolsInFile(file)
	if len(syms) == 0 {
		return nil
	}
	out := make([]types.SymbolLocation, 0, len(syms))
	for _, sym := range syms {
		out = append(out, types.SymbolLocation{
			File:    sym.File,
			Line:    sym.Line,
			EndLine: sym.EndLine,
		})
	}
	return out
}

func (l *multiGraphLocator) perGraphLocator(slug string, g *rmtypes.Graph) types.SymbolLocator {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cached == nil {
		l.cached = make(map[string]types.SymbolLocator)
	}
	if loc, ok := l.cached[slug]; ok {
		return loc
	}
	var loc types.SymbolLocator
	if l.mg.locatorFactory != nil {
		loc = l.mg.locatorFactory(g)
	} else {
		loc = &fallbackLocator{graph: g}
	}
	l.cached[slug] = loc
	return loc
}

func (l *multiGraphLocator) LocateSymbol(name string) []types.SymbolLocation {
	if l == nil || l.mg == nil {
		return nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	graphs := l.mg.AllGraphs()
	if len(graphs) == 0 {
		return nil
	}
	subMap := l.subRepoMap()
	var out []types.SymbolLocation
	for slug, g := range graphs {
		if g == nil {
			continue
		}
		sub := l.perGraphLocator(slug, g)
		if sub == nil {
			continue
		}
		locs := sub.LocateSymbol(name)
		if len(locs) == 0 {
			continue
		}
		sr := subMap[slug]
		for _, loc := range locs {
			out = append(out, types.SymbolLocation{
				File:    SubRepoRelPath(sr, loc.File),
				Line:    loc.Line,
				EndLine: loc.EndLine,
			})
		}
	}
	return out
}

func (l *multiGraphLocator) SymbolsInFile(file string) []types.SymbolLocation {
	if l == nil || l.mg == nil {
		return nil
	}
	g, sr, hit := l.mg.GraphFor(file)
	if !hit || g == nil {
		return nil
	}
	internal := stripSubRepoPrefix(sr, file)
	sub := l.perGraphLocator(sr.Slug, g)
	if sub == nil {
		return nil
	}
	locs := sub.SymbolsInFile(internal)
	if len(locs) == 0 {
		return nil
	}
	out := make([]types.SymbolLocation, len(locs))
	for i, loc := range locs {
		out[i] = types.SymbolLocation{
			File:    SubRepoRelPath(sr, loc.File),
			Line:    loc.Line,
			EndLine: loc.EndLine,
		}
	}
	return out
}

func (l *multiGraphLocator) subRepoMap() map[string]*topology.SubRepo {
	if l.mg == nil || l.mg.topo == nil {
		return nil
	}
	out := make(map[string]*topology.SubRepo, len(l.mg.topo.Repos))
	for i := range l.mg.topo.Repos {
		out[l.mg.topo.Repos[i].Slug] = &l.mg.topo.Repos[i]
	}
	return out
}
