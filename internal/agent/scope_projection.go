// Package agent — scope_projection.go
//
// projectPrimaryScopes / projectSubTopicScopes lift sub-repo names
// out of the untyped Entities lane into the typed PrimaryScopes /
// SubTopic.Scopes lane. The motivating defect (forensic anchor:
// 2026-05-10 chatpp-7d46dee4 log L2710) is that
// internal/analysis/gate/coherence.go's R1.3 / R1.5 invariants treat
// every emitted Entity as a symbol that must round-trip through
// resolver.LookupSymbol. In multi-repo workspaces sub-repo NAMES
// (codrax / opencode / dotfiles-claude / ...) are structural scopes,
// NOT symbols — round-tripping them through the resolver produces
// asymmetric resolution noise that fails the coherence gate on
// structurally legitimate cross-sub-repo emit shapes.
//
// Same shape as the existing typed-anchor promotions
// expandEntitiesWithImplementers (paths a, b, b1, b2) and
// promoteSubTopicFileAnchorToPrimary (path c) over in analyzer.go —
// pure read on rm + ctx, returns a typed slice the caller stores
// onto AnalyzerHints.PrimaryScopes / SubTopic.Scopes.
//
// COPY semantics (not move): the matched entity stays in
// PrimaryEntities for downstream consumers (search_keywords,
// exact-anchor ranking, evidence_closure tier seeds) that read the
// untyped lane. The typed lane only adds new typed information; it
// does not subtract from existing flows. This makes the change
// byte-additive and lets downstream code that needs scope/symbol
// disambiguation opt in lane-by-lane.
//
// Single-repo posture (mg == nil || mg.IsSingle() || ActiveSlugSnapshot
// empty) returns nil — preserving PrimaryScopes==nil in legacy paths
// and keeping the omitempty JSON serialisation byte-identical.
//
// Match policy: NormalizeCodeKey(entity) == NormalizeCodeKey(RootRel).
// Collapses case + underscore + hyphen so questions using either
// "codrax" / "Codrax" / "co-drax" all light up. RootRel "."
// (degenerate single-repo SubRepo) is excluded from the candidate
// set so a single-repo workspace never produces scope projections
// even if the IsSingle short-circuit ever weakens.
package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// projectPrimaryScopes returns the subset of `entities` whose
// canonical (NormalizeCodeKey) form matches an active sub-repo
// RootRel. The output is a fresh slice in the original RootRel
// spelling (not the entity's original casing) so downstream
// consumers see the SAME spelling the rest of the multi-repo UI
// uses (`/repos focus <RootRel>` etc.). Caller stores the result
// onto AnalyzerHints.PrimaryScopes — this helper is pure.
func projectPrimaryScopes(ctx *types.AgentContext, entities []string) []string {
	if len(entities) == 0 {
		return nil
	}
	canonByRootRel := activeScopeCanonMap(ctx)
	if len(canonByRootRel) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(entities))
	var out []string
	for _, e := range entities {
		canon := normalizer.NormalizeCodeKey(strings.TrimSpace(e))
		if canon == "" {
			continue
		}
		rel, ok := canonByRootRel[canon]
		if !ok || seen[rel] {
			continue
		}
		seen[rel] = true
		out = append(out, rel)
	}
	return out
}

// projectSubTopicScopes applies projectPrimaryScopes per-sub-topic
// and writes the result back onto each SubTopic.Scopes (in place).
// Sub-topics whose entities yield zero scope matches keep
// SubTopic.Scopes == nil so omitempty JSON serialisation stays
// byte-identical for the no-match case.
//
// Mutates `subs` in place; returns nothing. Caller passes
// rm.SubTopics directly — slice header is unchanged.
func projectSubTopicScopes(ctx *types.AgentContext, subs []types.SubTopic) {
	if len(subs) == 0 {
		return
	}
	for i := range subs {
		scoped := projectPrimaryScopes(ctx, subs[i].Entities)
		if len(scoped) > 0 {
			subs[i].Scopes = scoped
		}
	}
}

// activeScopeCanonMap returns canonical(RootRel) -> RootRel for
// every currently active (LRU-resident) sub-repo whose RootRel is
// not "." (degenerate single-repo case). Empty map when not in
// multi-repo posture.
//
// Hoisted out so projectPrimaryScopes and pre-emit-time
// detectScopesFromQuestion (B4, analyzer.go) share one source of
// truth for what counts as a scope token.
func activeScopeCanonMap(ctx *types.AgentContext) map[string]string {
	mg := repomap.MultiGraphFromAgentContext(ctx)
	if mg == nil || mg.IsSingle() {
		return nil
	}
	active := mg.ActiveSlugSnapshot()
	if len(active) == 0 {
		return nil
	}
	topo := mg.Topology()
	if topo == nil {
		return nil
	}
	out := make(map[string]string, len(active))
	for slug := range active {
		sr := topo.SubRepoBySlug(slug)
		if sr == nil || sr.RootRel == "" || sr.RootRel == "." {
			continue
		}
		canon := normalizer.NormalizeCodeKey(sr.RootRel)
		if canon == "" {
			continue
		}
		out[canon] = sr.RootRel
	}
	return out
}
