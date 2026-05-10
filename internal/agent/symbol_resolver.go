// Package agent — symbol_resolver.go
//
// repomapSymbolResolver bridges normalizer.SymbolResolver to an
// already-loaded *repomap.Graph so normalize can consult the repo
// symbol table during analyzer's buildAnalysisIR pass. This is the
// production implementation for Options.Resolver; tests can keep
// using normalizer's in-package fakeResolver.
//
// Lookup strategy:
//
//  1. Try graph.SymbolDefs[surface] directly (O(1), case-sensitive).
//  2. Fall back to iterating SymbolDefs with NormalizeCodeKey on both
//     sides — this collapses case and underscore/hyphen differences so
//     the caller's "blob" surface lights up on a repo-defined "Blob"
//     type, "build_agent_context" on "BuildAgentContext", etc.
//
// Domain is derived from Symbol.File via a prefix table; the table is
// intentionally broad (internal/agent → "agent", internal/tool →
// "tool") rather than hand-curated per project so the same adapter
// works on foreign repos. Unknown prefixes return the first path
// segment as a best-effort domain tag.
//
// Multi-repo addendum (B2, 2026-05-10): in multi-repo workspaces the
// single-graph view is too narrow — analyzerGraphForNormalize collapses
// the multigraph to one primary sub-repo's *Graph, so a sub-repo
// outside the primary remains invisible to the resolver. The
// `multiRepoSymbolResolver` adapter at the bottom of this file
// delegates to the existing multigraph fan-out APIs (LookupSymbol /
// IterateSymbolDefs) and uses the owning sub-repo's RootRel as a
// fallback Domain when the per-graph FileInfo.Package lookup is empty.
// Single-repo posture continues to use repomapSymbolResolver — the
// adapter constructor returns nil on IsSingle() so callers fall
// through to the legacy path.
package agent

import (
	"path"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// maxSymbolResolverHits caps the slice LookupSymbol returns so a
// generic word matching hundreds of symbols does not cascade into a
// giant TermGraph. The rarity confidence scaling in canonicalize.go
// already sinks such terms, but bounding the slice keeps the cost of
// a single Resolve call O(k) in k = cap.
const maxSymbolResolverHits = 20

// repomapSymbolResolver is the adapter passed as normalizer.Options.Resolver.
// It holds a read-only pointer to a graph built by repomap.BuildOrLoadGraph.
// Safe for concurrent reads; the underlying Graph is not mutated here.
type repomapSymbolResolver struct {
	graph *repomap.Graph
}

// newRepomapSymbolResolver returns nil when graph is nil so callers
// can pass the result directly to normalizer.Options.Resolver without
// a nil-check dance — normalizer already treats a nil Resolver as
// "gate closed, stay TermConcept".
func newRepomapSymbolResolver(g *repomap.Graph) normalizer.SymbolResolver {
	if g == nil {
		return nil
	}
	return &repomapSymbolResolver{graph: g}
}

// LookupSymbol returns every repomap definition whose canonical key
// matches the surface. Canonical is the repo's spelling (preserves
// exported casing); Domain is the best-effort dir-prefix tag.
func (r *repomapSymbolResolver) LookupSymbol(surface string) []normalizer.SymbolHit {
	if r == nil || r.graph == nil {
		return nil
	}
	trimmed := strings.TrimSpace(surface)
	if trimmed == "" {
		return nil
	}

	defs := r.graph.SymbolDefs[trimmed]
	if len(defs) == 0 {
		target := normalizer.NormalizeCodeKey(trimmed)
		if target == "" {
			return nil
		}
		for name, candidate := range r.graph.SymbolDefs {
			if normalizer.NormalizeCodeKey(name) == target {
				defs = candidate
				break
			}
		}
	}
	if len(defs) == 0 {
		return nil
	}

	hits := make([]normalizer.SymbolHit, 0, len(defs))
	seen := make(map[string]bool, len(defs))
	for _, sym := range defs {
		if sym == nil || sym.Name == "" {
			continue
		}
		key := sym.Name + "|" + sym.File
		if seen[key] {
			continue
		}
		seen[key] = true
		hits = append(hits, normalizer.SymbolHit{
			Canonical: sym.Name,
			Domain:    symbolDomain(r.graph, sym),
		})
		if len(hits) >= maxSymbolResolverHits {
			break
		}
	}
	return hits
}

// roleSuffixCanonicalLowers is the canonical list of role-naming
// suffixes that the LookupSymbolStem fallback strips before
// stem-substring matching. The list captures common conceptual
// role nouns the user may attach to a symbol's logical name when
// the codebase uses a different implementation suffix:
//
//	user "AnalyzerAgent" + repo `analyzerEvaluator` (Agent → Evaluator)
//	user "RequestHandler" + repo `requestProcessor` (Handler → Processor)
//	user "ConfigService" + repo `configManager`     (Service → Manager)
//
// Only role suffixes go here (not common type-ish nouns) — entries
// like "User" / "Item" would match too many false-positive cases.
// Each entry is the lowercase form; matching is case-insensitive.
var roleSuffixCanonicalLowers = []string{
	"agent", "handler", "manager", "service", "worker", "driver",
	"controller", "dispatcher", "processor", "runner", "executor",
	"module", "component", "adapter", "provider", "factory", "builder",
	"impl", "wrapper", "delegate", "facade", "client", "server",
}

// stemSuffixFloor is the minimum stem length subject to substring
// flat-match. "ClassA" → strip "class" → "a" (below floor → skip);
// "AnalyzerAgent" → strip "agent" → "analyzer" (8 chars ≥ 4 → ok).
const stemSuffixFloor = 4

// LookupSymbolStem (Fix J, 2026-05-07 m1b r1 forensic) is the
// concept-form fallback for callers that need "is this user-
// expressed entity grounded in the codebase even if the graph
// uses a different role suffix?". Use only after LookupSymbol
// returns empty. Matches the m1b case where the user said
// "analyzer agent" and the graph has `analyzerEvaluator`.
//
// Three-stage stem-aware lookup:
//
//  1. Strip a known role-naming suffix (Agent / Handler / Manager
//     / Service / etc.) from the surface, case-insensitive.
//  2. If the remaining stem is < stemSuffixFloor chars, abort
//     (trivial stems would substring-match too many symbols).
//  3. Flat-form-canonicalise the stem (NormalizeCodeKey) and scan
//     SymbolDefs for any name whose flat form CONTAINS the stem
//     flat. Cap hits at maxSymbolResolverHits. Returns nil when
//     no role suffix matches.
//
// Distinct semantic from LookupSymbol: this is a fuzzier
// "concept-existence" match used by gates (e.g. coherence
// entity-resolution) that ask "does this concept have ANY
// implementation in the repo?" — NOT "what is the canonical
// symbol for this name?".
func (r *repomapSymbolResolver) LookupSymbolStem(surface string) []normalizer.SymbolHit {
	if r == nil || r.graph == nil {
		return nil
	}
	trimmed := strings.TrimSpace(surface)
	if trimmed == "" {
		return nil
	}
	stem := stripRoleSuffix(trimmed)
	if stem == "" || len(stem) < stemSuffixFloor {
		return nil
	}
	flatStem := normalizer.NormalizeCodeKey(stem)
	if flatStem == "" || len(flatStem) < stemSuffixFloor {
		return nil
	}
	hits := make([]normalizer.SymbolHit, 0, maxSymbolResolverHits)
	seen := make(map[string]bool, maxSymbolResolverHits)
	for name, defs := range r.graph.SymbolDefs {
		if !strings.Contains(normalizer.NormalizeCodeKey(name), flatStem) {
			continue
		}
		for _, sym := range defs {
			if sym == nil || sym.Name == "" {
				continue
			}
			key := sym.Name + "|" + sym.File
			if seen[key] {
				continue
			}
			seen[key] = true
			hits = append(hits, normalizer.SymbolHit{
				Canonical: sym.Name,
				Domain:    symbolDomain(r.graph, sym),
			})
			if len(hits) >= maxSymbolResolverHits {
				return hits
			}
		}
	}
	return hits
}

// stripRoleSuffix removes a recognised role-naming suffix (case-
// insensitive) from the end of name and returns the remaining
// stem. Returns "" when no role suffix matches. Used by
// LookupSymbolStem to bridge user-concept-form
// (`AnalyzerAgent`) ↔ repo-implementation-form
// (`analyzerEvaluator`).
func stripRoleSuffix(name string) string {
	lower := strings.ToLower(name)
	for _, suffix := range roleSuffixCanonicalLowers {
		if !strings.HasSuffix(lower, suffix) {
			continue
		}
		stem := name[:len(name)-len(suffix)]
		// Trim a trailing separator that joined the role suffix to
		// the stem (`request_handler` → `request_`; trim trailing
		// `_` to get `request`).
		stem = strings.TrimRight(stem, "_-")
		return stem
	}
	return ""
}

// symbolDomain maps a Symbol to a free-form domain tag. Prefers the
// extractor-declared package name (FileInfo.Package — Go/Java/Rust
// extractors populate this) and falls back to the immediate parent
// directory of the file. The parent-dir rule is language-agnostic:
//
//	internal/agent/explorer.go          → agent
//	internal/tool/repomap/tool.go       → repomap
//	src/auth/login.py                   → auth
//	src/main/java/com/myapp/user/U.java → user
//	cmd/codrax/main.go                  → codrax
//
// Root-level files return "" (no directory context to infer from).
// There is no hardcoded repo vocabulary or project-specific prefix
// table — Domain flows from whatever the source tree shape provides.
func symbolDomain(g *repomap.Graph, sym *repomap.Symbol) string {
	if sym == nil || sym.File == "" {
		return ""
	}
	if g != nil && g.FileIndex != nil {
		if fi := g.FileIndex[sym.File]; fi != nil && fi.Package != "" {
			return fi.Package
		}
	}
	file := strings.ReplaceAll(sym.File, "\\", "/")
	parent := path.Dir(file)
	if parent == "" || parent == "." {
		return ""
	}
	return path.Base(parent)
}

// === Multi-repo SymbolResolver (B2, 2026-05-10) ===
//
// multiRepoSymbolResolver implements normalizer.SymbolResolver atop
// a *multigraph.MultiGraph. Used by analyzerSymbolResolver when the
// workspace is multi-repo (≥2 active sub-repos). For single-repo /
// nil multigraph posture, callers fall back to the legacy
// repomapSymbolResolver — byte-identical pre-multi-repo behaviour.
//
// Lookup strategy:
//
//  1. mg.LookupSymbol delegates to the existing cross-sub-repo fan-out
//     (multigraph.go:588) — returns []SymbolHit{*Symbol, *SubRepo}.
//  2. NormalizeCodeKey-equivalence fallback iterates SymbolDefs across
//     all active sub-repos (mg.IterateSymbolDefs, multigraph.go:704).
//
// Domain derivation: prefer per-graph symbolDomain (FileInfo.Package
// or parent-dir basename); fall back to the owning sub-repo's RootRel
// when the per-graph signal is empty. The RootRel fallback is the
// "code area" tag that lets coherence R1.4 axis_collapse stay
// meaningful in cross-sub-repo emit shapes — distinct sub-repos are
// distinct domains by construction.
//
// LookupSymbolStem reuses the package-local stripRoleSuffix /
// stemSuffixFloor from repomapSymbolResolver — the role-suffix
// vocabulary is a property of the user's spelling, not of which
// graph is loaded.
type multiRepoSymbolResolver struct {
	mg *multigraph.MultiGraph
}

// newMultiRepoSymbolResolver returns a multi-graph-backed resolver
// when mg has ≥2 sub-repos. Returns nil for single-repo / nil
// receivers so the caller (analyzerSymbolResolver) can fall back to
// the legacy single-graph adapter without nil-check dance — same
// contract as newRepomapSymbolResolver.
func newMultiRepoSymbolResolver(mg *multigraph.MultiGraph) normalizer.SymbolResolver {
	if mg == nil || mg.IsSingle() {
		return nil
	}
	return &multiRepoSymbolResolver{mg: mg}
}

// LookupSymbol delegates to mg.LookupSymbol then adapts each
// multigraph.SymbolHit to a normalizer.SymbolHit. Falls through to
// a NormalizeCodeKey-equivalence scan via IterateSymbolDefs when
// the verbatim lookup misses, matching the second-pass shape of
// repomapSymbolResolver.LookupSymbol.
func (r *multiRepoSymbolResolver) LookupSymbol(surface string) []normalizer.SymbolHit {
	if r == nil || r.mg == nil {
		return nil
	}
	trimmed := strings.TrimSpace(surface)
	if trimmed == "" {
		return nil
	}
	hits := r.mg.LookupSymbol(trimmed)
	if len(hits) == 0 {
		// Case/underscore-insensitive fallback. NormalizeCodeKey on
		// each name; pull defs whose canonical form matches the
		// canonical surface.
		target := normalizer.NormalizeCodeKey(trimmed)
		if target == "" {
			return nil
		}
		var fb []multigraph.SymbolHit
		r.mg.IterateSymbolDefs(func(name string, defs []*rmtypes.Symbol, sub *topology.SubRepo) bool {
			if normalizer.NormalizeCodeKey(name) != target {
				return true
			}
			for _, sym := range defs {
				if sym == nil {
					continue
				}
				fb = append(fb, multigraph.SymbolHit{Symbol: sym, Sub: sub})
				if len(fb) >= maxSymbolResolverHits {
					return false
				}
			}
			return true
		})
		if len(fb) == 0 {
			return nil
		}
		hits = fb
	}
	return r.adaptHits(hits)
}

// LookupSymbolStem is the multi-repo counterpart of
// repomapSymbolResolver.LookupSymbolStem. Same role-suffix stripping
// (stripRoleSuffix / stemSuffixFloor); fan-out via IterateSymbolDefs.
func (r *multiRepoSymbolResolver) LookupSymbolStem(surface string) []normalizer.SymbolHit {
	if r == nil || r.mg == nil {
		return nil
	}
	trimmed := strings.TrimSpace(surface)
	if trimmed == "" {
		return nil
	}
	stem := stripRoleSuffix(trimmed)
	if stem == "" || len(stem) < stemSuffixFloor {
		return nil
	}
	flatStem := normalizer.NormalizeCodeKey(stem)
	if flatStem == "" || len(flatStem) < stemSuffixFloor {
		return nil
	}
	var hits []multigraph.SymbolHit
	r.mg.IterateSymbolDefs(func(name string, defs []*rmtypes.Symbol, sub *topology.SubRepo) bool {
		if !strings.Contains(normalizer.NormalizeCodeKey(name), flatStem) {
			return true
		}
		for _, sym := range defs {
			if sym == nil {
				continue
			}
			hits = append(hits, multigraph.SymbolHit{Symbol: sym, Sub: sub})
			if len(hits) >= maxSymbolResolverHits {
				return false
			}
		}
		return true
	})
	if len(hits) == 0 {
		return nil
	}
	return r.adaptHits(hits)
}

// adaptHits maps multigraph.SymbolHit → normalizer.SymbolHit with
// per-sub-repo Domain enrichment and dedup. Domain priority:
//   1. per-graph symbolDomain (FileInfo.Package or parent-dir basename)
//   2. owning sub-repo's RootRel (cross-sub-repo "code area" tag)
//
// Dedup key includes Domain so the same Symbol resolved through
// different sub-repos (rare but possible — vendored modules etc.)
// yields distinct hits. The maxSymbolResolverHits cap keeps the
// final slice bounded.
func (r *multiRepoSymbolResolver) adaptHits(hits []multigraph.SymbolHit) []normalizer.SymbolHit {
	if len(hits) == 0 {
		return nil
	}
	out := make([]normalizer.SymbolHit, 0, len(hits))
	seen := make(map[string]bool, len(hits))
	all := r.mg.AllGraphs()
	for _, h := range hits {
		if h.Symbol == nil {
			continue
		}
		domain := ""
		if h.Sub != nil {
			if g, ok := all[h.Sub.Slug]; ok && g != nil {
				domain = symbolDomain(g, h.Symbol)
			}
			if domain == "" {
				domain = h.Sub.RootRel
			}
		}
		key := h.Symbol.Name + "|" + h.Symbol.File + "|" + domain
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, normalizer.SymbolHit{
			Canonical: h.Symbol.Name,
			Domain:    domain,
		})
		if len(out) >= maxSymbolResolverHits {
			break
		}
	}
	return out
}
