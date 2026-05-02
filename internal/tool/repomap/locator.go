package repomap

import (
	"path/filepath"
	"strings"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// graphLocator adapts a built repomap.Graph to the
// types.SymbolLocator interface. Stateless beyond the graph
// reference; safe to share across goroutines.
type graphLocator struct {
	graph *rmtypes.Graph
}

// NewSymbolLocator returns a locator backed by the given graph.
// nil graph yields a locator that returns nil for every query;
// this preserves the back-compat contract that callers can pass
// nil to disable drift detection without crashing.
//
// Implements types.SymbolLocator. Used by the AuthorityCeiling
// pipeline at evidence emit time (commit 3 hooks emit_evidence
// to call DetectDriftForFrame against this locator).
func NewSymbolLocator(g *rmtypes.Graph) types.SymbolLocator {
	return &graphLocator{graph: g}
}

// LocateSymbol implements types.SymbolLocator by trimming
// whitespace + delegating to graph.SymbolDefs. Returns nil when
// no definition matches or the graph is empty.
func (l *graphLocator) LocateSymbol(name string) []types.SymbolLocation {
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
			File:    canonicalLocatorPath(sym.File),
			Line:    sym.Line,
			EndLine: sym.EndLine,
		})
	}
	return out
}

// SymbolsInFile implements types.SymbolLocator by canonicalising
// the path lookup key + delegating to graph.SymbolsInFile. Returns
// nil when the file is not indexed or the graph is empty.
func (l *graphLocator) SymbolsInFile(file string) []types.SymbolLocation {
	if l == nil || l.graph == nil {
		return nil
	}
	canon := canonicalLocatorPath(file)
	if canon == "" {
		return nil
	}
	syms := l.graph.SymbolsInFile(canon)
	if len(syms) == 0 {
		// Try the as-given form as a fallback for callers that pass
		// already-canonicalised paths through a non-clean route
		// (Windows-style backslashes that fully matched the index).
		syms = l.graph.SymbolsInFile(file)
	}
	if len(syms) == 0 {
		return nil
	}
	out := make([]types.SymbolLocation, 0, len(syms))
	for _, sym := range syms {
		out = append(out, types.SymbolLocation{
			File:    canonicalLocatorPath(sym.File),
			Line:    sym.Line,
			EndLine: sym.EndLine,
		})
	}
	return out
}

// canonicalLocatorPath produces a forward-slashed, cleaned form of
// p. Mirrors logtriage.normalizeFrameFile so the two sides of the
// drift comparison agree on path keys.
func canonicalLocatorPath(p string) string {
	s := strings.TrimSpace(strings.ReplaceAll(p, `\`, `/`))
	if s == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(s))
}
