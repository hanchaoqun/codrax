package repomap

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// graphOracle wraps a built repomap.Graph for the
// types.SymbolOracle interface. Stateless beyond the graph
// reference; safe to share across goroutines because the
// underlying maps are read-only after BuildGraph returns.
type graphOracle struct {
	graph *rmtypes.Graph
}

// NewSymbolOracle returns an oracle backed by the given graph.
// nil graph yields an oracle that always returns (false, 0);
// this preserves the back-compat contract that callers can pass
// nil to disable validation (used by tests + single-shot CLI
// flows that don't build a repomap).
//
// The oracle is the single source of truth for downstream
// LLM-claim validators (commit 52 unified read-mode hardening):
//   - logtriage entity merge: drop LLM-extracted entities whose
//     name does not name a real symbol in this repo.
//   - emit_answer_document mermaid bare-identifier check: reject
//     diagram boxes whose body cannot be resolved.
//   - finalizer answer-coherence check (future use): same lookup.
func NewSymbolOracle(g *rmtypes.Graph) types.SymbolOracle {
	return &graphOracle{graph: g}
}

// SymbolExists implements types.SymbolOracle.
func (o *graphOracle) SymbolExists(name string) (bool, int) {
	if o == nil || o.graph == nil {
		return false, 0
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false, 0
	}
	defs, ok := o.graph.SymbolDefs[name]
	if !ok || len(defs) == 0 {
		return false, 0
	}
	minTier := 0
	for _, sym := range defs {
		if sym == nil {
			continue
		}
		fi, ok := o.graph.FileIndex[sym.File]
		if !ok || fi == nil {
			continue
		}
		tier := fi.ParseTier
		if tier <= 0 {
			tier = 1 // unset → assume primary parse
		}
		if minTier == 0 || tier < minTier {
			minTier = tier
		}
	}
	if minTier == 0 {
		// Defs entry exists but no FileInfo matched (defensive: stale
		// index). Treat as "found at unknown tier" — fall back to
		// Tier 4 so callers that filter by tier conservatively reject.
		return true, 4
	}
	return true, minTier
}
