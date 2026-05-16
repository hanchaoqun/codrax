package authority

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// pendingFilteredLocator wraps a SymbolLocator and drops any
// SymbolLocation whose File falls inside a pending sub-repo (path-
// from-parent prefix match against ctx.PendingSubRepos). The filter
// mirrors MultiRepoActiveSetGater.ResolveActiveSetPath's "topology -
// PendingSubRepos" rule so symbol-resolution results never anchor a
// citation to a sub-repo the file-system tools refuse to read.
//
// SymbolsInFile is also filtered: if the queried file itself sits in
// a pending sub-repo, the entire result is dropped.
type pendingFilteredLocator struct {
	inner   types.SymbolLocator
	pending []string
}

func newPendingFilteredLocator(inner types.SymbolLocator, pendingRootRels []string) types.SymbolLocator {
	if inner == nil || len(pendingRootRels) == 0 {
		return inner
	}
	cleaned := make([]string, 0, len(pendingRootRels))
	seen := make(map[string]bool, len(pendingRootRels))
	for _, r := range pendingRootRels {
		t := strings.TrimSpace(r)
		if t == "" || t == "." || seen[t] {
			continue
		}
		seen[t] = true
		cleaned = append(cleaned, t)
	}
	if len(cleaned) == 0 {
		return inner
	}
	return &pendingFilteredLocator{inner: inner, pending: cleaned}
}

// PathInsidePendingSubRepo reports whether file resolves into one of
// the pending sub-repos (exact RootRel match OR prefix RootRel + "/").
// Mirrors the prefix-walk in MultiGraph.ResolveActiveSetPath; callers
// queueing forced-reads / projecting symbol hits / building required-
// files should consult this before flagging a path as actionable.
// Pending is a slice of RootRels as published on BusContext.PendingSubRepos.
func PathInsidePendingSubRepo(file string, pending []string) bool {
	if file == "" {
		return false
	}
	clean := strings.ReplaceAll(strings.TrimSpace(file), "\\", "/")
	for _, rootRel := range pending {
		if clean == rootRel || strings.HasPrefix(clean, rootRel+"/") {
			return true
		}
	}
	return false
}

func (l *pendingFilteredLocator) LocateSymbol(name string) []types.SymbolLocation {
	if l == nil || l.inner == nil {
		return nil
	}
	raw := l.inner.LocateSymbol(name)
	if len(raw) == 0 {
		return raw
	}
	out := raw[:0]
	for _, loc := range raw {
		if PathInsidePendingSubRepo(loc.File, l.pending) {
			continue
		}
		out = append(out, loc)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (l *pendingFilteredLocator) SymbolsInFile(file string) []types.SymbolLocation {
	if l == nil || l.inner == nil {
		return nil
	}
	if PathInsidePendingSubRepo(file, l.pending) {
		return nil
	}
	raw := l.inner.SymbolsInFile(file)
	if len(raw) == 0 {
		return raw
	}
	out := raw[:0]
	for _, loc := range raw {
		if PathInsidePendingSubRepo(loc.File, l.pending) {
			continue
		}
		out = append(out, loc)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
