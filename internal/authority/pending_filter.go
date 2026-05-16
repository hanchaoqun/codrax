package authority

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// pendingFilteredLocator wraps a SymbolLocator and drops any
// SymbolLocation whose File falls inside a pending sub-repo. The
// active-set predicate lives in internal/types (single source of
// truth shared with the ranker / chain_promotion / FS-tool gate);
// this file just composes it onto the SymbolLocator surface.
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
		if types.PathInsidePendingSubRepo(loc.File, l.pending) {
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
	if types.PathInsidePendingSubRepo(file, l.pending) {
		return nil
	}
	raw := l.inner.SymbolsInFile(file)
	if len(raw) == 0 {
		return raw
	}
	out := raw[:0]
	for _, loc := range raw {
		if types.PathInsidePendingSubRepo(loc.File, l.pending) {
			continue
		}
		out = append(out, loc)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
