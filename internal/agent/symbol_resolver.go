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
	"sync"

	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	coretypes "github.com/hanchaoqun/codrax/internal/types"
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

	flatOnce sync.Once
	flatDefs map[string][]*repomap.Symbol
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
		defs = r.flatSymbolDefs()[target]
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

func (r *repomapSymbolResolver) flatSymbolDefs() map[string][]*repomap.Symbol {
	if r == nil || r.graph == nil {
		return nil
	}
	r.flatOnce.Do(func() {
		out := make(map[string][]*repomap.Symbol, len(r.graph.SymbolDefs))
		for name, defs := range r.graph.SymbolDefs {
			key := normalizer.NormalizeCodeKey(name)
			if key == "" || len(defs) == 0 {
				continue
			}
			out[key] = append(out[key], defs...)
		}
		r.flatDefs = out
	})
	return r.flatDefs
}

// LookupFileSurface resolves analyzer-emitted file surfaces against
// repomap's FileIndex. It is intentionally separate from LookupSymbol:
// a repo file path / basename is a structural anchor for architecture,
// trace, config, and cross-language questions, but it must not be
// smuggled into the symbol table. Exact repo-relative paths win; short
// basenames and suffix paths are accepted only when unique.
func (r *repomapSymbolResolver) LookupFileSurface(surface string) []normalizer.SymbolHit {
	if r == nil || r.graph == nil || len(r.graph.FileIndex) == 0 {
		return nil
	}
	return lookupFileSurfaceInGraph(surface, r.graph)
}

// LookupRepoSurface resolves non-symbol repo surfaces such as package names,
// directory names, and unique file stems. These surfaces are valid analyzer
// anchors for cross-language architecture questions, but they deliberately stay
// outside LookupSymbol so downstream code does not mistake them for declared
// identifiers.
func (r *repomapSymbolResolver) LookupRepoSurface(surface string) []normalizer.SymbolHit {
	if r == nil || r.graph == nil || len(r.graph.FileIndex) == 0 {
		return nil
	}
	return lookupRepoSurfaceInGraph(surface, r.graph)
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

// LookupSymbolActionAlias resolves analyzer concept surfaces that
// dropped a repo symbol's leading action verb while preserving the
// rest of the identifier exactly after case/separator normalization:
// `read_scheduler_loop` ↔ `runReadSchedulerLoop`. This is stricter
// than LookupSymbolStem: it only strips a bounded action-prefix enum
// from repo symbols and requires the remaining flat form to equal the
// analyzer surface. Callers that use this for hard gates should still
// require a unique hit.
func (r *repomapSymbolResolver) LookupSymbolActionAlias(surface string) []normalizer.SymbolHit {
	if r == nil || r.graph == nil {
		return nil
	}
	target := normalizedActionAliasSurface(surface)
	if target == "" {
		return nil
	}
	hits := make([]normalizer.SymbolHit, 0, maxSymbolResolverHits)
	seen := make(map[string]bool, maxSymbolResolverHits)
	for name, defs := range r.graph.SymbolDefs {
		stem := stripActionPrefix(name)
		if stem == "" || normalizer.NormalizeCodeKey(stem) != target {
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

var actionPrefixCanonicalLowers = []string{
	"run", "build", "make", "new", "create", "load", "parse", "handle",
}

const actionAliasFloor = 8

func normalizedActionAliasSurface(surface string) string {
	trimmed := strings.TrimSpace(surface)
	if trimmed == "" {
		return ""
	}
	target := normalizer.NormalizeCodeKey(trimmed)
	if len(target) < actionAliasFloor {
		return ""
	}
	return target
}

func stripActionPrefix(name string) string {
	if name == "" {
		return ""
	}
	lower := strings.ToLower(name)
	for _, prefix := range actionPrefixCanonicalLowers {
		if !strings.HasPrefix(lower, prefix) || len(name) <= len(prefix) {
			continue
		}
		boundary := name[len(prefix)]
		if boundary != '_' && boundary != '-' && (boundary < 'A' || boundary > 'Z') {
			continue
		}
		stem := strings.TrimLeft(name[len(prefix):], "_-")
		if stem != "" {
			return stem
		}
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

func lookupFileSurfaceInGraph(surface string, g *repomap.Graph) []normalizer.SymbolHit {
	if g == nil || len(g.FileIndex) == 0 {
		return nil
	}
	query := normalizeFileSurfaceForResolver(surface)
	if query == "" || !coretypes.HasCodeOrConfigPathSuffix(query) {
		return nil
	}
	if fi := g.FileIndex[query]; fi != nil {
		return []normalizer.SymbolHit{{Canonical: query, Domain: fileInfoDomain(query, fi)}}
	}
	var (
		matchPath string
		matchInfo *rmtypes.FileInfo
		matches   int
	)
	for rel, fi := range g.FileIndex {
		if fi == nil {
			continue
		}
		rel = strings.TrimSpace(strings.ReplaceAll(rel, "\\", "/"))
		if rel == "" {
			continue
		}
		if fileSurfaceMatches(query, rel) {
			matchPath = rel
			matchInfo = fi
			matches++
			if matches > 1 {
				return nil
			}
		}
	}
	if matches != 1 {
		return nil
	}
	return []normalizer.SymbolHit{{Canonical: matchPath, Domain: fileInfoDomain(matchPath, matchInfo)}}
}

func lookupRepoSurfaceInGraph(surface string, g *repomap.Graph) []normalizer.SymbolHit {
	if g == nil || len(g.FileIndex) == 0 {
		return nil
	}
	query := normalizeFileSurfaceForResolver(strings.Trim(surface, "`'\" "))
	if query == "" || query == "." || strings.Contains(query, "..") {
		return nil
	}
	if coretypes.HasCodeOrConfigPathSuffix(query) {
		return lookupFileSurfaceInGraph(query, g)
	}
	target := normalizer.NormalizeCodeKey(repoSurfaceLeaf(query))
	if len(target) < stemSuffixFloor && !repoSurfaceSpecificEnough(query) {
		return nil
	}
	hits := make(map[string]normalizer.SymbolHit)
	for rel, fi := range g.FileIndex {
		for _, cand := range repoSurfaceCandidatesForFile(query, target, rel, fi) {
			canon := cand.Canonical
			if canon == "" {
				continue
			}
			hits[canon] = normalizer.SymbolHit{
				Canonical: canon,
				Domain:    fileInfoDomain(cand.DomainRel, fi),
			}
			if len(hits) > 1 {
				return nil
			}
		}
	}
	for _, hit := range hits {
		return []normalizer.SymbolHit{hit}
	}
	return nil
}

func normalizeFileSurfaceForResolver(surface string) string {
	s := strings.TrimSpace(strings.ReplaceAll(surface, "\\", "/"))
	for strings.HasPrefix(s, "./") {
		s = strings.TrimPrefix(s, "./")
	}
	s = strings.TrimPrefix(s, "/")
	return path.Clean(s)
}

type repoSurfaceCandidate struct {
	Canonical string
	DomainRel string
}

func repoSurfaceCandidatesForFile(query, target, rel string, fi *rmtypes.FileInfo) []repoSurfaceCandidate {
	rel = normalizeFileSurfaceForResolver(rel)
	if rel == "" || rel == "." {
		return nil
	}
	if fi != nil {
		if canon, ok := repoSurfacePackageCanonical(query, target, fi.Package); ok {
			return []repoSurfaceCandidate{{Canonical: canon, DomainRel: rel}}
		}
		if canon, ok := repoSurfaceSpecialCanonical(query, target, rel, fi); ok {
			return []repoSurfaceCandidate{{Canonical: canon, DomainRel: rel}}
		}
	}
	var out []repoSurfaceCandidate
	add := func(canon string) {
		canon = strings.TrimSpace(canon)
		if canon == "" || canon == "." {
			return
		}
		out = append(out, repoSurfaceCandidate{Canonical: canon, DomainRel: rel})
	}
	if canon, ok := repoSurfaceDirCanonical(query, rel); ok {
		add(canon)
	}
	if !strings.Contains(query, "/") && !strings.Contains(query, ".") {
		stem := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
		if normalizer.NormalizeCodeKey(stem) == target {
			add(rel)
		}
	}
	if fi != nil {
		for _, imp := range fi.Imports {
			if canon, ok := repoSurfaceImportCanonical(query, target, imp); ok {
				add(canon)
			}
		}
		for _, relation := range fi.Relations {
			for _, surface := range repoSurfaceRelationSurfaces(relation) {
				if canon, ok := repoSurfaceRelationCanonical(query, target, surface); ok {
					add(canon)
				}
			}
		}
	}
	return dedupeRepoSurfaceCandidates(out)
}

func repoSurfaceLeaf(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	if strings.Contains(query, "/") {
		return path.Base(query)
	}
	if idx := strings.LastIndex(query, "."); idx >= 0 && idx+1 < len(query) {
		return query[idx+1:]
	}
	return query
}

func repoSurfaceDirCanonical(query, rel string) (string, bool) {
	dir := path.Dir(rel)
	if dir == "" || dir == "." {
		return "", false
	}
	if strings.Contains(query, "/") {
		if dir == query || strings.HasSuffix(dir, "/"+query) {
			return dir, true
		}
		return "", false
	}
	if strings.Contains(query, ".") {
		return "", false
	}
	target := normalizer.NormalizeCodeKey(query)
	parts := strings.Split(dir, "/")
	for i, part := range parts {
		if normalizer.NormalizeCodeKey(part) != target {
			continue
		}
		return strings.Join(parts[:i+1], "/"), true
	}
	return "", false
}

func repoSurfacePackageCanonical(query, target, pkg string) (string, bool) {
	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		return "", false
	}
	pkgNorm := normalizer.NormalizeCodeKey(pkg)
	queryNorm := normalizer.NormalizeCodeKey(query)
	if strings.Contains(query, ".") {
		if pkgNorm == queryNorm || strings.HasSuffix(pkgNorm, queryNorm) {
			return pkg, true
		}
		return "", false
	}
	for _, part := range strings.Split(pkg, ".") {
		if normalizer.NormalizeCodeKey(part) == target {
			return pkg, true
		}
	}
	return "", false
}

func repoSurfaceSpecialCanonical(query, target, rel string, fi *rmtypes.FileInfo) (string, bool) {
	if fi == nil || !fi.IsSpecial {
		return "", false
	}
	specialType := strings.TrimSpace(fi.SpecialType)
	for _, surface := range []string{specialType, path.Base(rel), strings.TrimSuffix(path.Base(rel), path.Ext(rel))} {
		if repoSurfaceNormalizedEqual(query, target, surface) {
			if specialType != "" {
				return specialType, true
			}
			return rel, true
		}
	}
	return "", false
}

func repoSurfaceImportCanonical(query, target string, imp rmtypes.Import) (string, bool) {
	canonical := strings.TrimSpace(imp.Path)
	if canonical == "" {
		canonical = strings.Trim(strings.TrimSpace(imp.Raw), "`'\" ")
	}
	if canonical == "" {
		canonical = strings.TrimSpace(imp.Alias)
	}
	if canonical == "" {
		return "", false
	}
	for _, surface := range []string{imp.Path, imp.Raw, imp.Alias} {
		surface = strings.Trim(strings.TrimSpace(surface), "`'\" ")
		if surface == "" {
			continue
		}
		if repoSurfaceNormalizedEqual(query, target, surface) {
			return canonical, true
		}
		if repoSurfaceSpecificEnough(surface) &&
			!strings.Contains(query, "/") && !strings.Contains(query, ".") &&
			target == normalizer.NormalizeCodeKey(repoSurfaceLeaf(surface)) {
			return canonical, true
		}
	}
	return "", false
}

func repoSurfaceRelationSurfaces(relation rmtypes.Relation) []string {
	var out []string
	add := func(surface string) {
		surface = strings.TrimSpace(surface)
		if surface != "" {
			out = append(out, surface)
		}
	}
	add(relation.ToEP.Name)
	add(relation.ToEP.Receiver)
	add(relation.ToEP.File)
	add(relation.FromEP.Name)
	return out
}

func repoSurfaceRelationCanonical(query, target, surface string) (string, bool) {
	surface = strings.Trim(strings.TrimSpace(surface), "`'\" ")
	if surface == "" || !repoSurfaceSpecificEnough(surface) {
		return "", false
	}
	if repoSurfaceNormalizedEqual(query, target, surface) {
		return surface, true
	}
	if strings.Contains(query, "/") || strings.Contains(query, ".") {
		return "", false
	}
	for _, candidate := range []string{repoSurfaceLeaf(surface), repoSurfaceRelationTail(surface)} {
		if candidate == "" || !repoSurfaceSpecificEnough(candidate) {
			continue
		}
		if normalizer.NormalizeCodeKey(candidate) == target {
			return surface, true
		}
	}
	return "", false
}

func repoSurfaceRelationTail(surface string) string {
	surface = strings.TrimSpace(surface)
	if idx := strings.LastIndex(surface, ":"); idx >= 0 && idx+1 < len(surface) {
		return surface[idx+1:]
	}
	return surface
}

func repoSurfaceNormalizedEqual(query, target, surface string) bool {
	surface = strings.Trim(strings.TrimSpace(surface), "`'\" ")
	if surface == "" {
		return false
	}
	if strings.EqualFold(query, surface) {
		return true
	}
	return target != "" && normalizer.NormalizeCodeKey(surface) == target
}

func repoSurfaceSpecificEnough(surface string) bool {
	surface = strings.TrimSpace(surface)
	key := normalizer.NormalizeCodeKey(surface)
	if len(key) < stemSuffixFloor {
		return false
	}
	if len(key) >= 8 {
		return true
	}
	for _, r := range surface {
		if r >= 'A' && r <= 'Z' {
			return true
		}
		if r >= '0' && r <= '9' {
			return true
		}
		switch r {
		case '_', '.', '/', '\\', ':', '@', '#', '$', '-':
			return true
		}
	}
	return false
}

func dedupeRepoSurfaceCandidates(in []repoSurfaceCandidate) []repoSurfaceCandidate {
	if len(in) <= 1 {
		return in
	}
	seen := make(map[string]bool, len(in))
	out := make([]repoSurfaceCandidate, 0, len(in))
	for _, c := range in {
		key := c.Canonical
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	return out
}

func fileSurfaceMatches(query, rel string) bool {
	if query == "" || rel == "" {
		return false
	}
	if rel == query {
		return true
	}
	if strings.Contains(query, "/") {
		return strings.HasSuffix(rel, "/"+query)
	}
	return path.Base(rel) == query
}

func fileInfoDomain(rel string, fi *rmtypes.FileInfo) string {
	if fi != nil && strings.TrimSpace(fi.Package) != "" {
		return strings.TrimSpace(fi.Package)
	}
	parent := path.Dir(strings.TrimSpace(rel))
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
	// pendingSubRepos mirrors BusContext.PendingSubRepos so adaptHits
	// drops symbol hits whose sub-repo the routing fold left inactive
	// for this question. Without this, hits from LRU-resident-but-
	// pending sub-repos leak into normalizer.TermGraph and downstream
	// stages can mistake them for current-repo-grounded targets — the
	// same family of leak that L1 plugged at the pre-scan ranker
	// level, here at the analyzer's symbol normalization surface.
	pendingSubRepos []string

	flatOnce sync.Once
	flatHits map[string][]multigraph.SymbolHit
}

// newMultiRepoSymbolResolver returns a multi-graph-backed resolver
// when mg has ≥2 sub-repos. Returns nil for single-repo / nil
// receivers so the caller (analyzerSymbolResolver) can fall back to
// the legacy single-graph adapter without nil-check dance — same
// contract as newRepomapSymbolResolver.
//
// pendingRootRels is the BusContext.PendingSubRepos snapshot for the
// current question; hits from those sub-repos are filtered before
// reaching normalizer consumers.
func newMultiRepoSymbolResolver(mg *multigraph.MultiGraph, pendingRootRels []string) normalizer.SymbolResolver {
	if mg == nil || mg.IsSingle() {
		return nil
	}
	r := &multiRepoSymbolResolver{mg: mg}
	if len(pendingRootRels) > 0 {
		r.pendingSubRepos = append([]string(nil), pendingRootRels...)
	}
	return r
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
		// canonical surface. The flat index is built once per resolver
		// instance so analyzer parse_output does not rescan every
		// active sub-repo's SymbolDefs for each entity/surface.
		target := normalizer.NormalizeCodeKey(trimmed)
		if target == "" {
			return nil
		}
		fb := r.flatSymbolHits()[target]
		if len(fb) == 0 {
			return nil
		}
		if len(fb) > maxSymbolResolverHits {
			fb = fb[:maxSymbolResolverHits]
		}
		hits = fb
	}
	return r.adaptHits(hits)
}

func (r *multiRepoSymbolResolver) flatSymbolHits() map[string][]multigraph.SymbolHit {
	if r == nil || r.mg == nil {
		return nil
	}
	r.flatOnce.Do(func() {
		out := make(map[string][]multigraph.SymbolHit)
		r.mg.IterateSymbolDefs(func(name string, defs []*rmtypes.Symbol, sub *topology.SubRepo) bool {
			key := normalizer.NormalizeCodeKey(name)
			if key == "" || len(defs) == 0 {
				return true
			}
			for _, sym := range defs {
				if sym == nil {
					continue
				}
				out[key] = append(out[key], multigraph.SymbolHit{Symbol: sym, Sub: sub})
			}
			return true
		})
		r.flatHits = out
	})
	return r.flatHits
}

// LookupFileSurface is the multi-repo counterpart to the single-graph
// file-surface resolver. It accepts exact path-from-parent, exact
// sub-repo-internal path, and unique basename/suffix matches across
// the active set. Ambiguous basenames return nil so R1.5 remains a
// precise hard gate.
func (r *multiRepoSymbolResolver) LookupFileSurface(surface string) []normalizer.SymbolHit {
	if r == nil || r.mg == nil {
		return nil
	}
	query := normalizeFileSurfaceForResolver(surface)
	if query == "" || !coretypes.HasCodeOrConfigPathSuffix(query) {
		return nil
	}
	if fi, sub, ok := r.mg.FileInfoFor(query); ok {
		canon := multigraph.SubRepoRelPath(sub, fi.RelPath)
		return []normalizer.SymbolHit{{Canonical: canon, Domain: fileSurfaceDomain(canon, fi, sub)}}
	}
	var (
		matchCanon string
		matchInfo  *rmtypes.FileInfo
		matchSub   *topology.SubRepo
		matches    int
	)
	r.mg.IterateFileIndex(func(internalRel string, fi *rmtypes.FileInfo, sub *topology.SubRepo) bool {
		if fi == nil {
			return true
		}
		internalRel = normalizeFileSurfaceForResolver(internalRel)
		canon := multigraph.SubRepoRelPath(sub, internalRel)
		if fileSurfaceMatches(query, internalRel) || fileSurfaceMatches(query, canon) {
			matchCanon = canon
			matchInfo = fi
			matchSub = sub
			matches++
			return matches <= 1
		}
		return true
	})
	if matches != 1 {
		return nil
	}
	return []normalizer.SymbolHit{{Canonical: matchCanon, Domain: fileSurfaceDomain(matchCanon, matchInfo, matchSub)}}
}

// LookupRepoSurface is the multi-repo counterpart to
// repomapSymbolResolver.LookupRepoSurface. It accepts only unique repo
// package/directory/file-stem surfaces across the active set; ambiguous
// surfaces return nil so analyzer hard gates stay precise.
func (r *multiRepoSymbolResolver) LookupRepoSurface(surface string) []normalizer.SymbolHit {
	if r == nil || r.mg == nil {
		return nil
	}
	query := normalizeFileSurfaceForResolver(strings.Trim(surface, "`'\" "))
	if query == "" || query == "." || strings.Contains(query, "..") {
		return nil
	}
	if coretypes.HasCodeOrConfigPathSuffix(query) {
		return r.LookupFileSurface(query)
	}
	target := normalizer.NormalizeCodeKey(repoSurfaceLeaf(query))
	if len(target) < stemSuffixFloor && !repoSurfaceSpecificEnough(query) {
		return nil
	}
	hits := make(map[string]normalizer.SymbolHit)
	r.mg.IterateFileIndex(func(internalRel string, fi *rmtypes.FileInfo, sub *topology.SubRepo) bool {
		for _, cand := range repoSurfaceCandidatesForFile(query, target, internalRel, fi) {
			canon := cand.Canonical
			if canon == "" {
				continue
			}
			if !strings.Contains(canon, ".") || strings.Contains(canon, "/") {
				canon = multigraph.SubRepoRelPath(sub, canon)
			} else if sub != nil && strings.TrimSpace(sub.RootRel) != "" && sub.RootRel != "." {
				canon = strings.TrimSpace(sub.RootRel) + ":" + canon
			}
			hits[canon] = normalizer.SymbolHit{
				Canonical: canon,
				Domain:    fileSurfaceDomain(multigraph.SubRepoRelPath(sub, cand.DomainRel), fi, sub),
			}
			if len(hits) > 1 {
				return false
			}
		}
		return true
	})
	for _, hit := range hits {
		return []normalizer.SymbolHit{hit}
	}
	return nil
}

func fileSurfaceDomain(rel string, fi *rmtypes.FileInfo, sub *topology.SubRepo) string {
	if fi != nil && strings.TrimSpace(fi.Package) != "" {
		return strings.TrimSpace(fi.Package)
	}
	if sub != nil && strings.TrimSpace(sub.RootRel) != "" && sub.RootRel != "." {
		return strings.TrimSpace(sub.RootRel)
	}
	return fileInfoDomain(rel, fi)
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

// LookupSymbolActionAlias is the multi-repo counterpart of
// repomapSymbolResolver.LookupSymbolActionAlias. Same bounded prefix
// stripping; fan-out via IterateSymbolDefs.
func (r *multiRepoSymbolResolver) LookupSymbolActionAlias(surface string) []normalizer.SymbolHit {
	if r == nil || r.mg == nil {
		return nil
	}
	target := normalizedActionAliasSurface(surface)
	if target == "" {
		return nil
	}
	var hits []multigraph.SymbolHit
	r.mg.IterateSymbolDefs(func(name string, defs []*rmtypes.Symbol, sub *topology.SubRepo) bool {
		stem := stripActionPrefix(name)
		if stem == "" || normalizer.NormalizeCodeKey(stem) != target {
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
//  1. per-graph symbolDomain (FileInfo.Package or parent-dir basename)
//  2. owning sub-repo's RootRel (cross-sub-repo "code area" tag)
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
		// B: drop hits whose sub-repo is pending for this question.
		// Sub.RootRel is checked against r.pendingSubRepos via the
		// shared types.PathInsidePendingSubRepo predicate so this
		// matches the rule the FS-tool gate, the pre-scan ranker,
		// and chain_promotion all enforce.
		if h.Sub != nil && coretypes.PathInsidePendingSubRepo(h.Sub.RootRel, r.pendingSubRepos) {
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
