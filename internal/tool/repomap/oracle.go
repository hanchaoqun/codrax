package repomap

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// graphOracle adapts a built repomap.Graph (which already has
// SymbolExists as a method since commit 53) to the
// types.SymbolOracle interface. Stateless beyond the graph
// reference; safe to share across goroutines.
type graphOracle struct {
	graph *rmtypes.Graph
}

// NewSymbolOracle returns an oracle backed by the given graph.
// nil graph yields an oracle that always returns (false, 0);
// this preserves the back-compat contract that callers can pass
// nil to disable validation.
//
// The oracle is the single source of truth for downstream
// LLM-claim validators (commit 52 unified read-mode hardening):
//   - logtriage entity merge
//   - emit_answer_document mermaid bare-identifier check
//   - finalizer answer-coherence (future use)
func NewSymbolOracle(g *rmtypes.Graph) types.SymbolOracle {
	return &graphOracle{graph: g}
}

// SymbolExists implements types.SymbolOracle by trimming
// whitespace + delegating to the underlying graph.
func (o *graphOracle) SymbolExists(name string) (bool, int) {
	if o == nil || o.graph == nil {
		return false, 0
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false, 0
	}
	return o.graph.SymbolExists(name)
}
