package repomap

import (
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/types"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// graphOracle adapts a built repomap.Graph (which already has
// SymbolExists as a method since commit 53) to the
// types.SymbolOracle interface. Stateless beyond the graph
// reference; safe to share across goroutines.
//
// flatIndex is built lazily on first SymbolExistsFlat call (Fix E,
// 2026-05-07). It maps each symbol's flat-form name (case-folded,
// `_`/`-` stripped — same canonicalisation as
// contract.FlattenIdentifier) to the lowest tier across matching
// definitions. The mutex guards the lazy build; once populated the
// map is read-only so concurrent reads are safe without further
// locking.
type graphOracle struct {
	graph *rmtypes.Graph

	flatOnce  sync.Once
	flatIndex map[string]int
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
//   - finalizer answer-coherence (Fix C / Fix D hallucination gates)
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

// SymbolExistsFlat (Fix E, 2026-05-07) implements the case-form-
// aware existence check. The graph's SymbolDefs index is
// case-sensitive exact, so a query like "pipeline_max_steps"
// (yaml-tag form) misses the Go field "PipelineMaxSteps" even
// though they name the same logical entity. SymbolExistsFlat
// builds (lazily, once per oracle) a flatname → minTier index by
// applying contract.FlattenIdentifier to every symbol name in the
// graph; queries are flattened the same way before lookup, so all
// snake / camel / Pascal / kebab / SCREAMING_SNAKE / flatcase
// renderings of the same logical name resolve to the same entry.
//
// The lazy build is protected by sync.Once so the index is built
// at most once per oracle instance, regardless of caller
// concurrency. After the build, the map is read-only and reads are
// lock-free.
func (o *graphOracle) SymbolExistsFlat(name string) (bool, int) {
	if o == nil || o.graph == nil {
		return false, 0
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false, 0
	}
	flatQuery := contract.FlattenIdentifier(name)
	if flatQuery == "" {
		return false, 0
	}
	o.flatOnce.Do(o.buildFlatIndex)
	if tier, ok := o.flatIndex[flatQuery]; ok {
		return true, tier
	}
	return false, 0
}

// buildFlatIndex walks the graph's SymbolDefs once and computes
// the flatname → minTier map. Symbols with the same flat form
// (e.g. snake-case Go field + Pascal-case Go method that happen to
// differ only by separator/case) collapse into a single entry whose
// tier is the lowest (most reliable) across the group. Tier 0
// fallback for stale-index rows mirrors graph.SymbolExists.
func (o *graphOracle) buildFlatIndex() {
	o.flatIndex = make(map[string]int, len(o.graph.SymbolDefs))
	for name, defs := range o.graph.SymbolDefs {
		if name == "" || len(defs) == 0 {
			continue
		}
		flat := contract.FlattenIdentifier(name)
		if flat == "" {
			continue
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
				tier = 1
			}
			if minTier == 0 || tier < minTier {
				minTier = tier
			}
		}
		if minTier == 0 {
			// Stale index — fall back to lowest-confidence tier so
			// callers behave conservatively (mirrors
			// Graph.SymbolExists).
			minTier = 4
		}
		if existing, ok := o.flatIndex[flat]; !ok || minTier < existing {
			o.flatIndex[flat] = minTier
		}
	}
}

// NewDeclSpanSource returns a types.DeclSpanSource backed by the graph's
// per-file symbol index. Only declaration LINES of exported symbols are
// reported (Symbol.Line — the signature line), never body spans: a hunk
// inside an exported function's body must not read as API impact. Files
// parsed at low-confidence fallback tiers (regex-only / path-only) report
// ok=false so risk consumers degrade to softer signals instead of trusting
// unreliable spans. nil graph yields a source that always reports ok=false.
func NewDeclSpanSource(g *rmtypes.Graph) types.DeclSpanSource {
	return &graphDeclSpans{graph: g}
}

type graphDeclSpans struct{ graph *rmtypes.Graph }

func (s *graphDeclSpans) ExportedDeclLines(relPath string) ([]int, bool) {
	if s == nil || s.graph == nil || s.graph.FileIndex == nil {
		return nil, false
	}
	relPath = strings.TrimSpace(relPath)
	if relPath == "" {
		return nil, false
	}
	fi, ok := s.graph.FileIndex[relPath]
	if !ok || fi == nil {
		return nil, false
	}
	// Tier 0 (single-grammar languages) and tiers 1-2 (primary/secondary
	// grammar) carry trustworthy line positions; tier 3 (regex) and tier 4
	// (path-only) do not.
	if fi.ParseTier > 2 {
		return nil, false
	}
	var lines []int
	for i := range fi.Symbols {
		sym := &fi.Symbols[i]
		if !sym.Exported || sym.Line <= 0 {
			continue
		}
		lines = append(lines, sym.Line)
	}
	return lines, true
}
