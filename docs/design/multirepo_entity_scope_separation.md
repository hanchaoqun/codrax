# Multi-repo Entity vs Scope Separation — Design Doc

**Status**: Active (2026-05-10)
**Baseline**: `0abf6c7` HEAD at design time. `internal/analysis/gate/coherence.go`, `internal/agent/analyzer.go`, `internal/agent/symbol_resolver.go`, `internal/tool/repomap/multigraph_facade.go`, `internal/tool/repomap/multigraph/multigraph.go`, `internal/types/analysis_ir.go`, `internal/skill/analysis_contract.go`, `internal/orchestrator/orchestrator.go` line refs in §3 / §5 are pinned to this commit; if you re-land this work after rebases re-grep against the helpers' names rather than line numbers.

**Scope of work**: 7 batches (B1–B7) producing ~600–900 net LOC + tests. **Single-repo posture (`mg == nil || mg.IsSingle()`) MUST be byte-identical to pre-change behaviour.** L1 red line.

---

## §1 Forensic anchor

A real REPL session at `/home/chatpp/.codrax/logs/chatpp-7d46dee4/codrax-20260510-011154-000-3930920.log` failed both turns of `对比codrax和opencode分别是怎么保证对代码分析结果精准无幻觉的` with `analyze stage exhausted after 3 attempt(s)` despite both `codrax` and `opencode` being in the active set (`L1320 / L2714`). All 6 emit_analysis attempts hit the `subtopic_coherence` gate with no possible legal emit shape: R1.2 demands ≥2 sub-topics for cross-component, R1.3 demands sub-topic.entities ∩ primary, R1.5 demands resolver-symmetric resolution. The third leg is unsatisfiable: when primary entities are sub-repo NAMES (`codrax` / `opencode`), `resolver.LookupSymbol` resolves them asymmetrically (one matches `OpencodeClient` via NormalizeCodeKey, the other matches nothing), tripping R1.5.

Five interlocking root causes feed the failure:

| RC | Defect | Evidence |
|---|---|---|
| **A** | `subtopic_coherence` is structurally unsatisfiable for cross-sub-repo comparison | L462 (R1.2), L1316 (R1.5), L2710 (R1.5 on cleanest possible emit) |
| **B** | `pickPrimarySubRepo` collapses MultiGraph→single graph for analyzer pre-inject AND R1.5 resolver | `entities=[opencode]` repeats verbatim across all 6 retries (L43,L475,L898,L1338,L1906,L2301) — codrax never enters pre-inject |
| **C** | `composeCoherenceRetryHint` leaks pipeline-internal token names (`is_cross_component` / `emit_analysis`) into LLM hint, causing meta-topic hallucination | L1316 sub-topic 2 `entities=[emit_analysis, is_cross_component]` |
| **D** | Skill prompt fails to forbid cross-turn topic seepage; prior conversation about diagram sizing leaks as sub-topic | L885 sub-topic 4 `entities=[chart, diagram, mobile, display]` |
| **E** | No early-exit detection when N retries hit the same gate name with identical fingerprint; budget burns deterministically | Both turns burn full 3-attempt budget; second turn replays first |

The forensic write-up is in this conversation's prior turns; this doc is the implementation contract.

---

## §2 Architectural principle

Per `CLAUDE.md` red line: hard gates use **PRECISE typed signals**; noisy heuristics drive only soft guidance.

The defect is a **type confusion**: in multi-repo mode, sub-repo names function as **structural scopes** (containers, namespaces) but flow through pipelines that treat them as **symbols** (resolvable identifiers). The fix is to **promote scope to a typed lane in the IR** so every downstream consumer (resolver / oracle / gate / pre-inject / write-mode cross-repo refusal) reads the right type.

Architecture picture (post-fix):

```
RawRequest ─> analyzer LLM ─> emit_analysis
                                     │
                                     ▼
                          PrimaryEntities []string         ← LLM-emitted, untyped (legacy lane)
                                     │
            ┌────────────── post-process ──────────────┐
            ▼                                          ▼
    PrimaryScopes []SubRepoSlug         PrimaryEntities (unchanged copy)
       (typed: matches active           (legacy: still flows everywhere it
        sub-repo slug verbatim)          flowed before; redundancy is safety)
            │                                          │
            └──────────┬───────────────────────────────┘
                       ▼
                coherence gate (S2)
                resolver (S3)
                pre-inject (S4)
                retry hint sanitize (S5)
                early-exit (S6)
```

Single-repo / nil multigraph: `PrimaryScopes` always empty → all consumers short-circuit to legacy paths → byte-identical.

---

## §3 Existing code we reuse (do NOT reinvent)

This section is the discipline that justifies the PR being ~700 LOC instead of ~2000.

### 3.1 Cross-sub-repo symbol fan-out (B2 reuses)

| Already exists | Where | What we use it for |
|---|---|---|
| `multigraph.MultiGraph.LookupSymbol(name) []SymbolHit` | `multigraph.go:588-618` | New `multiRepoSymbolResolver.LookupSymbol` thin wrapper — **already does cross-sub-repo fan-out**, returns `[]SymbolHit{Symbol, Sub}` |
| `multigraph.MultiGraph.ImplementersOf(interfaceName) []ImplementerHit` | `multigraph.go:639` | Same shape; future use only — current B2 doesn't need it |
| `multigraph.MultiGraph.IterateSymbolDefs(yield)` | `multigraph.go:704` | Used by future code-fold passes; not B2 |
| `multigraph.MultiGraph.SubRepoBySlug` (via `m.topo.Repos`) | `multigraph.go:596-599 inline` | New resolver maps `SymbolHit.Sub.Slug` → `Domain` (R1.4 axis_collapse signal) |
| `topology.RepoTopology.SubRepoBySlug(slug)` | `topology/topology.go:128-138` | Topology-level lookup the gate code can use without holding mg ref |

**Note**: `mg.LookupSymbol` returns `multigraph.SymbolHit` (`{*Symbol, *SubRepo}`) which is **not the same shape** as `normalizer.SymbolHit` (`{Canonical, Domain}` strings). The B2 adapter does the field copy — small, in-package.

### 3.2 Sub-repo identity / topology

| Already exists | Where | What we use it for |
|---|---|---|
| `topology.SubRepo.Slug` (cache slug) | `topology.go:28-29` | Verbatim match against tokens for scope-projection (B1) |
| `topology.SubRepo.RootRel` | `topology.go:35-38` | User-facing slug shown in `/repos`, multi-repo overview, gate detail prose |
| `topology.RepoTopology.IsSingle()` | `topology.go:76-78` | All B1-B6 short-circuits |
| `multigraph.MultiGraph.IsSingle()` | `multigraph.go:147` | Same purpose at the mg level (more callers) |

**Decision**: scope-projection (B1) matches against `RootRel`, not `Slug`. RootRel is what the user types and what every other prompt shows (e.g. `/repos focus codrax` uses RootRel). Slug is a cache identifier (e.g. `codrax-d11dc254`) the user never sees. Matching RootRel keeps the user-mental-model invariant.

### 3.3 Existing path (a)/(b)/(c) typed-anchor expansions

| Already exists | Where | Pattern we mirror |
|---|---|---|
| `expandEntitiesWithImplementers` (paths a, b, b1, b2) | `analyzer.go:~2480-2880` | Uses graph primitives `ImplementersOf` / `FileIndex` / `SymbolDefs` to expand single-entity emit into ≥2 typed members |
| `promoteSubTopicFileAnchorToPrimary` (path c) | `analyzer.go:2903-2983` | When nSub==1 and sub-topic anchor is a file/interface, promote it to PrimaryEntities so R1.3 overlap is satisfied |
| `subTopicAnchorResolvesInGraph` | `analyzer.go:2990-3009` | Typed-graph predicate (FileIndex OR SymbolDefs[Kind ∈ {interface,trait,protocol}]) |

**Reuse strategy for B1**: scope projection is **architecturally analogous to path (c)** — it's a typed-anchor promotion using a different signal source (active sub-repo slug instead of FileIndex/SymbolDefs). Same pattern, new typed primitive. The new helper goes in `analyzer.go` next to `promoteSubTopicFileAnchorToPrimary`, sharing the same `distinctNamedEntities` / `lookupFileInfoWithSuffix` neighbours.

### 3.4 Single-graph fast paths to preserve

| File | Function | Single-repo behaviour |
|---|---|---|
| `multigraph_facade.go:156-169` | `GraphFromAgentContextOrLoad` | `IsSingle → mg.Single()` returns the byte-equivalent legacy `*Graph` |
| `analyzer.go:478-483` | `analyzerOracleFromCtx` | `mg.Oracle()` collapses to legacy oracle when `IsSingle` |
| `analyzer.go:1376` | `newRepomapSymbolResolver(...)` | nil graph → nil resolver → normalizer falls through |
| `analyzer.go:401-405` | `buildAnalyzerRepoOverview` multi-repo header | only added when `!mg.IsSingle()` |
| `coherence.go:174-233` | R1.5 `if resolver != nil && nSub >= 2` | nil resolver disables R1.5 entirely |

Every fast path is preserved. The new `multiRepoSymbolResolver` (B2) is wired to be **return-equivalent to `repomapSymbolResolver`** when `mg.IsSingle()` (delegates to `mg.Single()` and uses the legacy adapter).

### 3.5 Retry hint plumbing (B5 reuses)

| Already exists | Where | What we hook |
|---|---|---|
| `composeCoherenceRetryHint` | `analyzer.go:247-262` | We replace `plainCoherenceDetail` callsites with sanitized version |
| `plainCoherenceDetail` + `stripCoherencePrefix` | `analyzer.go:274-311` | We add a body sanitizer AFTER the prefix strip |
| `Mutable.SetAnalyzerRetryHint` / `AnalyzerRetryHint` / `ResetAnalyzerRetryHint` | `types/context.go:2006-2039` | Untouched — the hint string is sanitized BEFORE storage |

### 3.6 GateReport / orchestrator retry (B6 hooks)

| Already exists | Where | What we modify |
|---|---|---|
| `types.GateReport` | `analysis_ir.go:1070-1075` | Add `Fingerprint string` (NOT `Checks` — the fingerprint is the report-level summary) |
| `runAnalyzePhase` retry loop | `orchestrator.go:1925-1959` | Track `lastFingerprint` + repeat-counter; early-exit when N == ceil(max/2) consecutive same-fp |
| Caveat injection | existing `Mutable.AppendAnswerRetryEvent` (`orchestrator.go:1956`) | Reuse — caveat goes through the same answer_reviewer/caveat surface |

---

## §4 New code surface (B1–B6, by batch)

### 4.1 B1 — typed PrimaryScopes lane

#### 4.1.1 IR schema additions (`internal/types/analysis_ir.go`)

```go
// AnalyzerHints (lines 398-444) — append after PrimaryEntities/MentionedEntities/DerivedEntities:
//
// PrimaryScopes is the typed sub-repo-name lane parallel to
// PrimaryEntities. When the analyzer LLM emits an entity whose
// surface form matches an active sub-repo's RootRel verbatim,
// the post-emit projection (analyzer.go::projectPrimaryScopes)
// COPIES that entity here while keeping it in PrimaryEntities.
// Empty in single-repo posture; empty in multi-repo when no
// emitted entity matches an active sub-repo. Coherence gate's
// scope-aware path (R1.3 widened, R1.5 carve-out) reads this
// lane to distinguish "structural scope" from "code symbol".
PrimaryScopes []string `json:"primary_scopes,omitempty"`
```

```go
// SubTopic (lines 501-504) — append:
//
// Scopes is the per-sub-topic typed sub-repo-name lane parallel
// to Entities. Same projection rule as PrimaryScopes: any
// SubTopic.Entities surface that matches an active sub-repo's
// RootRel is copied here at post-emit time. Coherence R1.3
// considers (Entities ∪ Scopes) when checking PrimaryEntities
// overlap; R1.5 skips scopes (active-set membership is
// authoritative — no resolver lookup needed).
Scopes []string `json:"scopes,omitempty"`
```

**Why COPY not MOVE**: defense in depth. If a single-repo Go module ever has a top-level identifier named `codrax`, ripping `codrax` out of PrimaryEntities would break `analyzerHints.Entities` consumers (search_keywords, exact-anchor ranking, evidence_closure tier seed) that rely on it being there. Having it in BOTH lanes is harmless — the gate carve-out only consults Scopes for the carve-out decision; downstream ranking still gets the original Entities lane unchanged.

#### 4.1.2 Projection helper (`internal/agent/scope_projection.go` — new file)

```go
// Package agent — scope_projection.go
//
// projectPrimaryScopes mirrors `promoteSubTopicFileAnchorToPrimary`
// (analyzer.go:2903) — same shape: read rm + ctx, return a typed-
// anchor list. Only the signal source differs:
//   path (c) used graph.FileIndex / graph.SymbolDefs.Kind ∈ interface
//   path (s) (this) uses topology.RepoTopology.SubRepos.RootRel
//
// Single-repo / nil mg short-circuit to nil — preserves
// PrimaryScopes==nil in legacy path. Multi-repo with no emitted
// entity matching an active sub-repo also returns nil.
//
// Match is by NormalizeCodeKey(entity) == NormalizeCodeKey(RootRel).
// This collapses "codrax" / "Codrax" / "co-drax" so a question
// using either form lights up. RootRel "." (single-repo) is
// excluded from the candidate set.
package agent

import (
    "strings"
    "github.com/hanchaoqun/codrax/internal/analysis/normalizer"
    "github.com/hanchaoqun/codrax/internal/tool/repomap"
    "github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
    "github.com/hanchaoqun/codrax/internal/types"
)

// projectPrimaryScopes returns the subset of `entities` whose
// canonical form matches an active sub-repo RootRel. Caller is
// responsible for storing it onto AnalyzerHints.PrimaryScopes /
// SubTopic.Scopes (this helper is pure).
func projectPrimaryScopes(ctx *types.AgentContext, entities []string) []string {
    if len(entities) == 0 {
        return nil
    }
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
    canonByRootRel := make(map[string]string, len(active))
    for slug := range active {
        sr := topo.SubRepoBySlug(slug)
        if sr == nil || sr.RootRel == "" || sr.RootRel == "." {
            continue
        }
        canonByRootRel[normalizer.NormalizeCodeKey(sr.RootRel)] = sr.RootRel
    }
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
        if !ok {
            continue
        }
        if seen[rel] {
            continue
        }
        seen[rel] = true
        out = append(out, rel)
    }
    return out
}

// projectSubTopicScopes applies projectPrimaryScopes per-sub-topic
// and writes the result back onto each SubTopic.Scopes. Empty
// scopes slice on a sub-topic is preserved (i.e. SubTopic.Scopes
// stays nil), matching omitempty semantics.
func projectSubTopicScopes(ctx *types.AgentContext, subs []types.SubTopic) {
    for i := range subs {
        scoped := projectPrimaryScopes(ctx, subs[i].Entities)
        if len(scoped) > 0 {
            subs[i].Scopes = scoped
        }
    }
}
```

#### 4.1.3 Wire site (`analyzer.go::buildAnalysisIR`)

Insert after `rm.AnalyzerHints.PrimaryEntities = promoteSubTopicFileAnchorToPrimary(ctx, rm)` at line 1433:

```go
// Multi-repo scope projection — typed lane parallel to PrimaryEntities
// (see PrimaryScopes / SubTopic.Scopes godoc, scope_projection.go,
// docs/design/multirepo_entity_scope_separation.md §4.1).
//
// Empty in single-repo / nil-multigraph posture: COPY semantics
// preserve PrimaryEntities verbatim, so this is byte-additive.
rm.AnalyzerHints.PrimaryScopes = projectPrimaryScopes(ctx, rm.AnalyzerHints.PrimaryEntities)
projectSubTopicScopes(ctx, rm.SubTopics)
```

#### 4.1.4 Skill JSON schema sync (R2' compliance)

`internal/skill/analysis_contract.go` — add to `Optional fields` enumeration in two places:

1. After line 84 `Optional fields: sub_topics (array), answer_subject (object), …`, append `, primary_scopes (auto-derived; do not emit)` — but **wait**, this is auto-derived, not LLM-emitted. So we do NOT add it as an emit field. Instead we add a description below:

```go
// New section after "## Sub-topic detection":
of.WriteString("## Sub-repo scopes (system-projected)\n\n")
of.WriteString("In multi-repo workspaces the system automatically detects when an entity surface matches an active sub-repo name (e.g. `codrax` / `opencode` matching the workspace's sub-repo RootRel). These detections flow into a typed scope lane alongside your `entities` list — you do NOT need to emit a separate field. Just include the sub-repo name verbatim in `entities` (or `sub_topics[].entities`) when the user named it; the system handles the typed projection.\n\n")
```

#### 4.1.5 R2' six-spot sync inventory

Per `feedback_typed_signal_six_spot_sync.md`:

| Spot | Action |
|---|---|
| 1. struct field | `analysis_ir.go` AnalyzerHints + SubTopic |
| 2. JSON schema desc | skill prompt §"Sub-repo scopes" |
| 3. skill prompt | same as #2 |
| 4. retry hint | none (system-projected; LLM never sees the field name in retry hints) |
| 5. JSON decoder error remap | `n/a` — field is omitempty + system-set, never deserialized from LLM emit |
| 6. cooccurrence rule / RepairLocus | will add `RepairLocusAnalyzeScopeProjection` if any cooccurrence rule needs it (B3 will tell — likely no) |

**Sync verdict**: 2 of 6 spots active (struct + skill desc). The other 4 are N/A because PrimaryScopes is system-projected, not LLM-emitted. **R2' compliant.**

#### 4.1.6 Tests (`internal/agent/scope_projection_test.go` — new file)

- `TestProjectPrimaryScopes_SingleRepo_ReturnsNil`
- `TestProjectPrimaryScopes_NilMultiGraph_ReturnsNil`
- `TestProjectPrimaryScopes_MultiRepoMatch_CopiesRootRel`
- `TestProjectPrimaryScopes_NormalizeCodeKeyEquivalence` (case / underscore / hyphen)
- `TestProjectPrimaryScopes_SkipsRootRelDot`
- `TestProjectSubTopicScopes_PerTopicProjection`
- `TestProjectPrimaryScopes_NoActiveSlugMatch_ReturnsNil`
- `TestProjectPrimaryScopes_EdgeCase_SymbolNameEqualsRootRel` (the var-named-codrax safety test — verify projection writes Scopes but does NOT remove from PrimaryEntities)

### 4.2 B2 — multi-graph SymbolResolver adapter

#### 4.2.1 New adapter (`internal/agent/symbol_resolver.go` — append)

```go
// multiRepoSymbolResolver wraps multigraph.MultiGraph behind the
// normalizer.SymbolResolver interface. Used by analyzerGraphForNormalize
// when MultiGraph is multi-repo (≥ 2 active sub-repos). For single-
// repo / nil multigraph, callers continue using
// repomapSymbolResolver — byte-identical legacy behaviour.
//
// Lookup strategy:
//
//  1. Delegate to mg.LookupSymbol (multigraph/multigraph.go:588) which
//     fans out across every active sub-repo's SymbolDefs, returning
//     []SymbolHit{*Symbol, *SubRepo}.
//  2. Map each multigraph.SymbolHit → normalizer.SymbolHit by:
//     - Canonical = sym.Name verbatim
//     - Domain    = symbolDomain(g, sym) where g is the OWNING sub-repo's
//                   graph, fall back to Sub.RootRel when the prefix table
//                   does not match (multi-repo file paths don't always
//                   live under internal/agent — every sub-repo has its
//                   own root, so RootRel is the meaningful "code area"
//                   tag)
//
// LookupSymbolStem follows the same delegation pattern: fan out
// across sub-repos via mg.IterateSymbolDefs, apply the role-suffix
// stripping locally (stripRoleSuffix is package-local).
type multiRepoSymbolResolver struct {
    mg *multigraph.MultiGraph
}

func newMultiRepoSymbolResolver(mg *multigraph.MultiGraph) normalizer.SymbolResolver {
    if mg == nil || mg.IsSingle() {
        return nil
    }
    return &multiRepoSymbolResolver{mg: mg}
}

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
        // Case/underscore-insensitive fallback: iterate per-graph
        // SymbolDefs once. Same shape as repomapSymbolResolver.LookupSymbol's
        // second-pass path but fanned across sub-repos.
        target := normalizer.NormalizeCodeKey(trimmed)
        if target == "" {
            return nil
        }
        var fallback []multigraph.SymbolHit
        r.mg.IterateSymbolDefs(func(name string, defs []*rmtypes.Symbol, sub *topology.SubRepo) bool {
            if normalizer.NormalizeCodeKey(name) != target {
                return true
            }
            for _, sym := range defs {
                if sym == nil {
                    continue
                }
                fallback = append(fallback, multigraph.SymbolHit{Symbol: sym, Sub: sub})
            }
            return true
        })
        if len(fallback) == 0 {
            return nil
        }
        hits = fallback
    }
    return r.adaptHits(hits)
}

func (r *multiRepoSymbolResolver) adaptHits(hits []multigraph.SymbolHit) []normalizer.SymbolHit {
    out := make([]normalizer.SymbolHit, 0, len(hits))
    seen := make(map[string]bool, len(hits))
    for _, h := range hits {
        if h.Symbol == nil {
            continue
        }
        // Domain derivation: prefer per-graph symbolDomain() when we
        // can fetch the owning graph (mg.AllGraphs lookup); fall back
        // to the sub-repo RootRel as the "code area" tag for cross-
        // graph distinguishability.
        domain := ""
        if h.Sub != nil {
            if g, ok := r.mg.AllGraphs()[h.Sub.Slug]; ok && g != nil {
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

// LookupSymbolStem is the multi-repo counterpart of
// repomapSymbolResolver.LookupSymbolStem. Same role-suffix
// stripping policy; fan-out via IterateSymbolDefs.
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
    return r.adaptHits(hits)
}
```

#### 4.2.2 Wire site (`analyzer.go::analyzerGraphForNormalize`)

Currently (line 1376):
```go
resolver := newRepomapSymbolResolver(analyzerGraphForNormalize(ctx, rm))
```

After:
```go
resolver := analyzerSymbolResolver(ctx, rm)
```

New helper (next to `analyzerGraphForNormalize`, ~analyzer.go:1778):

```go
// analyzerSymbolResolver returns the right normalizer.SymbolResolver
// for the current Run: multi-graph fan-out resolver in multi-repo
// posture, single-graph adapter otherwise. Single-repo / nil mg
// returns the legacy repomapSymbolResolver wrapping the result of
// analyzerGraphForNormalize — byte-identical behaviour.
//
// Layered with analyzerOracleFromCtx (analyzer.go:478) which already
// switched to multi-graph fan-out for the SymbolOracle path; this
// closes the matching gap on the SymbolResolver path.
func analyzerSymbolResolver(ctx *types.AgentContext, rm types.RequestModel) normalizer.SymbolResolver {
    if mg := repomap.MultiGraphFromAgentContext(ctx); mg != nil && !mg.IsSingle() {
        return newMultiRepoSymbolResolver(mg)
    }
    return newRepomapSymbolResolver(analyzerGraphForNormalize(ctx, rm))
}
```

#### 4.2.3 Tests (`internal/agent/symbol_resolver_multirepo_test.go` — new file)

- `TestMultiRepoSymbolResolver_NilMultiGraph_ReturnsNil`
- `TestMultiRepoSymbolResolver_SingleRepo_ReturnsNil` (caller falls back to single)
- `TestMultiRepoSymbolResolver_LookupSymbol_FanOut`
- `TestMultiRepoSymbolResolver_DomainFromRootRelFallback`
- `TestMultiRepoSymbolResolver_StemFallback_CrossSubRepo`
- `TestAnalyzerSymbolResolver_SingleRepoPath_ByteIdentical` (assert same `*repomapSymbolResolver` instance returned in single-repo posture)

### 4.3 B3 — coherence gate scope-aware rewrite

#### 4.3.1 Changes to `internal/analysis/gate/coherence.go::checkSubtopicCoherence`

**R1.3 widening** (line 138-147):
```go
// R1.3 — Sub-topic entity orphan (scope-aware).
//
// Multi-repo lifting (2026-05-10): when sub-repo NAMES are part of
// PrimaryEntities (e.g. cross-sub-repo comparison "compare codrax
// vs opencode"), they also live in PrimaryScopes via the typed
// projection in analyzer.go::projectPrimaryScopes. Compare the
// UNION of (entities ∪ scopes) on both sides so a sub-topic
// anchoring a scope (e.g. SubTopic.Scopes=[codrax]) satisfies
// R1.3 against PrimaryScopes=[codrax, opencode]. Single-repo
// posture has both Scopes lanes empty → identical to pre-fix.
if nSub >= 1 && len(rm.AnalyzerHints.PrimaryEntities)+len(rm.AnalyzerHints.PrimaryScopes) >= coherenceMinPrimaryEntitiesForOrphan {
    subTokens := flattenSubTopicEntitiesAndScopes(rm.SubTopics)
    primaryTokens := unionStringLists(rm.AnalyzerHints.PrimaryEntities, rm.AnalyzerHints.PrimaryScopes)
    if len(subTokens) > 0 && len(primaryTokens) > 0 {
        if !anyOverlap(subTokens, primaryTokens) {
            details = append(details, fmt.Sprintf(
                "R1.3 entity_orphan: sub-topic entities %s share no element with primary entities %s",
                formatStringList(subTokens), formatStringList(primaryTokens)))
        }
    }
}
```

**R1.5 carve-out** (line 174-232) — when iterating sub-topic states, skip the resolver lookup on entries whose surface matches the sub-topic's Scopes lane:

```go
// R1.5 — Sub-topic entity asymmetry (scope-aware carve-out).
//
// 2026-05-10 lifting: scope tokens (sub-repo RootRels) are
// authoritatively grounded by the active-set membership check
// (anyOverlap with PrimaryScopes ⊆ active slug set). Running them
// through resolver.LookupSymbol produces an asymmetric resolution
// pattern that is signal-side noise (R3 red line: hard gates may
// not key on noisy signals). Skip scope tokens at the per-entity
// resolver iteration so a sub-topic with Scopes=[codrax] hits
// "scope path, not resolver path" and contributes to anyHit
// without symmetry distortion.
//
// Pre-fix the resolver's per-graph SymbolDefs view was also single-
// graph (B2 fixed that), so even genuine symbols in the non-primary
// sub-repo would miss. B2 + this carve-out together close the
// asymmetric-resolution failure on cross-repo comparisons.
if resolver != nil && nSub >= 2 {
    type subTopicState struct { /* unchanged */ }
    states := make([]subTopicState, 0, nSub)
    anyHit := false
    anyMiss := false
    for i, st := range rm.SubTopics {
        // Scope-only sub-topic short-circuit: if every Entities
        // surface ALSO appears in Scopes (active sub-repo membership
        // is authoritative), this sub-topic is scope-typed and skips
        // the resolver entirely. anyHit flips true so the
        // anyHit∧anyMiss gate still works for mixed populations.
        if subTopicScopeOnly(st, rm.AnalyzerHints.PrimaryScopes) {
            anyHit = true
            states = append(states, subTopicState{index: i, topic: st, hit: true})
            continue
        }
        // ... existing per-entity loop unchanged
    }
    // ... existing anyHit∧anyMiss check unchanged
}
```

**R1.8 NEW — Scope anchor distribution (SOFT)**

```go
// R1.8 — Scope anchor distribution (SOFT advisory, NOT hard gate).
//
// When the LLM emits ≥2 PrimaryScopes (e.g. cross-sub-repo
// comparison naming both `codrax` and `opencode`) AND
// IsCrossComponent=true, each PrimaryScope ideally has at least
// one sub-topic anchored to it. Failure to balance — e.g. all
// 3 sub-topics anchor to `codrax`, none to `opencode` — produces
// an asymmetric answer that under-serves the user's compare-and-
// contrast intent.
//
// This is a SOFT advisory not a HARD reject because:
//   (a) cross-cutting sub-topics (single sub-topic that compares
//       both scopes for one aspect) are a legitimate alternative
//       decomposition and would be falsely rejected by HARD gate
//   (b) the LLM's downstream stages (explorer / extractor /
//       finalizer) can recover by reading both scopes during
//       investigation; the analyzer doesn't need to enforce
//       structural symmetry at classification time
//
// R3 second-axis compliance: scope distribution is a noisy
// heuristic (the "ideal" distribution depends on the question's
// rhetorical shape, which the analyzer can't see). Soft only.
if rm.Predicates.IsCrossComponent && len(rm.AnalyzerHints.PrimaryScopes) >= 2 && nSub >= 2 {
    anchored := make(map[string]bool, len(rm.AnalyzerHints.PrimaryScopes))
    for _, st := range rm.SubTopics {
        for _, sc := range st.Scopes {
            anchored[strings.TrimSpace(sc)] = true
        }
    }
    var missing []string
    for _, scope := range rm.AnalyzerHints.PrimaryScopes {
        if !anchored[strings.TrimSpace(scope)] {
            missing = append(missing, scope)
        }
    }
    if len(missing) > 0 {
        sort.Strings(missing)
        softAdvisories = append(softAdvisories, fmt.Sprintf(
            "R1.8 scope_anchor_distribution (advisory): cross-component question with %d sub-repo scopes %s but %d sub-repo(s) %s have no sub-topic anchor — if the answer should compare each sub-repo, consider adding a sub-topic per scope; if a cross-cutting decomposition is intentional, ignore this advisory",
            len(rm.AnalyzerHints.PrimaryScopes),
            formatStringList(rm.AnalyzerHints.PrimaryScopes),
            len(missing),
            formatStringList(missing)))
    }
}
```

#### 4.3.2 New helpers (same file)

```go
// flattenSubTopicEntitiesAndScopes is R1.3's lifted equivalent of
// flattenSubTopicEntities — collects entities AND scopes into one
// dedup'd list. Both lanes are case-sensitive deduped.
func flattenSubTopicEntitiesAndScopes(subs []types.SubTopic) []string {
    if len(subs) == 0 {
        return nil
    }
    seen := make(map[string]bool)
    var out []string
    for _, st := range subs {
        for _, e := range st.Entities {
            t := strings.TrimSpace(e)
            if t != "" && !seen[t] {
                seen[t] = true
                out = append(out, t)
            }
        }
        for _, s := range st.Scopes {
            t := strings.TrimSpace(s)
            if t != "" && !seen[t] {
                seen[t] = true
                out = append(out, t)
            }
        }
    }
    return out
}

// unionStringLists returns the dedup'd union of two slices.
func unionStringLists(a, b []string) []string {
    seen := make(map[string]bool, len(a)+len(b))
    out := make([]string, 0, len(a)+len(b))
    for _, s := range a {
        t := strings.TrimSpace(s)
        if t != "" && !seen[t] {
            seen[t] = true
            out = append(out, t)
        }
    }
    for _, s := range b {
        t := strings.TrimSpace(s)
        if t != "" && !seen[t] {
            seen[t] = true
            out = append(out, t)
        }
    }
    return out
}

// subTopicScopeOnly returns true when every non-empty Entities
// surface in `st` is also present (case-sensitive) in `st.Scopes`
// OR in the global `primaryScopes` list. Used by R1.5 to skip
// the resolver path for scope-typed sub-topics — their grounding
// is active-set membership, not symbol-table presence.
func subTopicScopeOnly(st types.SubTopic, primaryScopes []string) bool {
    if len(st.Entities) == 0 {
        return false
    }
    scopeSet := make(map[string]bool, len(st.Scopes)+len(primaryScopes))
    for _, s := range st.Scopes {
        scopeSet[strings.TrimSpace(s)] = true
    }
    for _, s := range primaryScopes {
        scopeSet[strings.TrimSpace(s)] = true
    }
    if len(scopeSet) == 0 {
        return false
    }
    for _, e := range st.Entities {
        if !scopeSet[strings.TrimSpace(e)] {
            return false
        }
    }
    return true
}
```

#### 4.3.3 Update `stripCoherencePrefix` (`analyzer.go:289`)

```go
for _, prefix := range []string{
    "R1.1 domain_divergence (advisory): ",
    "R1.1 domain_divergence: ",
    "R1.2 predicate_contradiction: ",
    "R1.3 entity_orphan: ",
    "R1.4 axis_collapse: ",
    "R1.5 entity_unresolvable: ",
    "R1.6 completeness_obligation_missing: ",
    "R1.7 bucket_partition_missing: ",
    "R1.8 scope_anchor_distribution (advisory): ",  // NEW
    "R2.1 scalar_multi_topic: ",
    "R2.2 explanation_scalar_subject: ",
} {
```

#### 4.3.4 Tests (`internal/analysis/gate/coherence_scope_test.go` — new file)

Coverage matrix (each row = one test):

| Case | PrimaryEntities | PrimaryScopes | SubTopics | Expected |
|---|---|---|---|---|
| Single-repo cross-component | [A, B] | [] | [{ents:[A]}, {ents:[B]}] | R1.3 PASS (legacy path) |
| Multi-repo cross-sub-repo: scope-typed sub-topics | [codrax, opencode] | [codrax, opencode] | [{ents:[codrax],scopes:[codrax]}, {ents:[opencode],scopes:[opencode]}] | R1.3 PASS (overlap), R1.5 SKIP (scope-only), R1.8 PASS (each scope anchored) |
| Multi-repo: scope + symbol mixed | [codrax, opencode] | [codrax, opencode] | [{ents:[codrax,SymbolOracle],scopes:[codrax]}, {ents:[opencode,OpencodeClient],scopes:[opencode]}] | R1.3 PASS, R1.5 PASS (resolver finds both ents on multi-graph) |
| Multi-repo: missing scope anchor | [codrax, opencode] | [codrax, opencode] | [{ents:[codrax],scopes:[codrax]}, {ents:[X]}] | R1.3 PASS, R1.8 SOFT ADVISORY (opencode unanchored) |
| Cross-cutting sub-topic | [codrax, opencode] | [codrax, opencode] | [{ents:[hallucination_detection],summary:"compare both repos' approach"}] | R1.2 may fire (only 1 sub-topic for cross-component); R1.8 SOFT (both scopes unanchored) |
| Edge: no scopes detected | [codrax, opencode] | [] | [{ents:[codrax]}, {ents:[opencode]}] | identical to single-repo path |

### 4.4 B4 — pre-inject multi-scope task_map

#### 4.4.1 Heuristic detection (`analyzer.go::buildAnalyzerRepoOverview`)

Pre-emit time, no PrimaryScopes lane is populated yet. Heuristic must run on raw question text:

```go
// detectScopesFromQuestion (analyzer.go, near buildAnalyzerRepoOverview)
//
// Pre-emit projection of active sub-repo RootRels onto a question
// string. Word-boundary aware so "opencode" matches but "opencodex"
// does not. Returns the matched RootRels in original RootRel form.
// Mirrors projectPrimaryScopes's NormalizeCodeKey-based comparison
// for stem equivalence ("Codrax" matches "codrax").
//
// Empty when the workspace is single-repo, mg is nil, or no slug
// matches.
func detectScopesFromQuestion(ctx *types.AgentContext, question string) []string {
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
    canonByRootRel := make(map[string]string, len(active))
    for slug := range active {
        sr := topo.SubRepoBySlug(slug)
        if sr == nil || sr.RootRel == "" || sr.RootRel == "." {
            continue
        }
        canonByRootRel[normalizer.NormalizeCodeKey(sr.RootRel)] = sr.RootRel
    }
    if len(canonByRootRel) == 0 {
        return nil
    }
    seen := make(map[string]bool, len(canonByRootRel))
    var out []string
    // Tokenize CJK-aware (reuse tokenizeQuestionCJKAware from explorer.go).
    // Word boundary on CJK and ASCII letter runs.
    for _, tok := range tokenizeQuestionCJKAware(question) {
        canon := normalizer.NormalizeCodeKey(strings.TrimSpace(tok))
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
```

#### 4.4.2 Multi-scope rendering (`buildAnalyzerRepoOverview`)

Modify lines 326-415. Pseudo-diff:

```go
func buildAnalyzerRepoOverview(ctx *types.AgentContext, objective string) (string, *repomap.Graph) {
    // ... pre-existing setup unchanged ...

    entities := extractQuestionEntities(objective)
    // NEW: when multi-repo and ≥2 active sub-repos visible in the
    // question, render per-scope mini task_maps so the analyzer LLM
    // sees both sides. Otherwise fall through to the single-graph path.
    if scopes := detectScopesFromQuestion(ctx, objective); len(scopes) >= 2 {
        return buildMultiScopeRepoOverview(ctx, objective, scopes)
    }

    // ... existing single-graph path unchanged from here ...
}

// buildMultiScopeRepoOverview renders one mini task_map per scope.
// Total budget cap (4096 bytes) is divided evenly across scopes.
// Each scope gets at least minScopeBytes; total truncation respected.
func buildMultiScopeRepoOverview(ctx *types.AgentContext, objective string, scopes []string) (string, *repomap.Graph) {
    mg := repomap.MultiGraphFromAgentContext(ctx)
    if mg == nil {
        return "", nil
    }
    const totalBudget = 4096
    perScope := totalBudget / len(scopes)
    if perScope < 800 {
        perScope = 800 // floor — too small and the view is useless
    }
    topN := 7
    if len(scopes) > 2 {
        topN = 5
    }
    var b strings.Builder
    b.WriteString(fmt.Sprintf("## Repository overview (pre-computed for sub-repo scopes: %s)\n\n", strings.Join(scopes, ", ")))
    b.WriteString("The following per-sub-repo task_map shows files and symbols matching the question for each named sub-repo. Use this to inform your sub-topic decomposition and pre-scan targets. You may still call repo_map / grep / list_files for additional verification.\n\n")
    if header := renderMultiRepoOverviewHeader(mg); header != "" {
        b.WriteString(header)
        b.WriteString("\n")
    }
    var primaryGraph *repomap.Graph
    for i, scope := range scopes {
        sr := mg.Topology().SubRepoByRootRel(scope)
        if sr == nil {
            continue
        }
        g, err := mg.EnsureLoaded(sr.Slug)
        if err != nil || g == nil {
            continue
        }
        if i == 0 {
            primaryGraph = g  // first scope's graph flows into Mutable for legacy callers
        }
        view := repomap.GenerateView(g, "task_map", repomap.ViewParams{
            Query: objective,
            TopN:  topN,
        })
        if view == "" {
            continue
        }
        if len(view) > perScope {
            view = view[:perScope] + "\n... [truncated]\n"
        }
        fmt.Fprintf(&b, "\n# Task Map: %s\n\n%s\n", scope, view)
    }
    out := b.String()
    if len(out) > totalBudget+512 {
        out = out[:totalBudget+512] + "\n... [truncated]\n"
    }
    return out, primaryGraph
}
```

#### 4.4.3 Mutable.SearchGraph compat

The current code stores ONE graph on `ctx.Mutable.SetSearchGraph(graph)`. With multi-scope, we keep the **first scope's** graph as the "primary" for legacy callers. Downstream consumers that need fan-out already hit the multi-graph path (analyzerOracleFromCtx already does this). This is a known partial-correctness mode (matches `multigraph_facade.go:128-132` comment about caller-wrong-to-ask).

#### 4.4.4 Tests (`internal/agent/analyzer_multiscope_overview_test.go`)

- `TestDetectScopesFromQuestion_NoMatch_ReturnsNil`
- `TestDetectScopesFromQuestion_TwoScopes_ReturnsBoth`
- `TestDetectScopesFromQuestion_PartialOverlap_OneOnly` (e.g. "what does codrax do" → [codrax])
- `TestDetectScopesFromQuestion_WordBoundary` (e.g. "opencodex" does NOT match "opencode")
- `TestDetectScopesFromQuestion_CJKMixed` (cn question with two latin slug names)
- `TestBuildMultiScopeRepoOverview_RendersPerScope`
- `TestBuildMultiScopeRepoOverview_BudgetSplit`
- `TestBuildAnalyzerRepoOverview_SingleRepo_ByteIdentical`

### 4.5 B5 — retry hint sanitizer + skill prompt prior-pollution rule

#### 4.5.1 Sanitizer (`internal/agent/retry_hint_sanitize.go` — new file)

```go
// Package agent — retry_hint_sanitize.go
//
// sanitizeInternalVocab strips pipeline-internal token names from
// LLM-facing retry hints. Without this, gate Detail strings carry
// names like "predicates.is_cross_component" or "emit_analysis"
// into the next-attempt LLM prompt, where the LLM mistakenly
// indexes them as code entities to investigate (forensic anchor:
// 2026-05-10 chatpp-7d46dee4 log L1316 sub-topic
// entities=[emit_analysis, is_cross_component]).
//
// Internal vocab list is generated by reflect over the IR types
// at package init — schema evolution is automatically picked up.
//
// R6 red line ("no internal pipeline info in LLM prompts") direct
// remediation. The forensic data lane (logs, traces, recordReconcileObservation)
// receives the ORIGINAL detail string; only the LLM-facing path
// goes through this sanitizer.
package agent

import (
    "reflect"
    "regexp"
    "strings"
    "sync"
    "github.com/hanchaoqun/codrax/internal/types"
)

var (
    internalVocabOnce sync.Once
    internalVocab     map[string]string // internal-token → user-facing replacement
    internalVocabRE   *regexp.Regexp    // alternation regex for one-pass replacement
)

func buildInternalVocab() {
    // Field-name harvest via reflect. We collect:
    //  - JSON tag names from RequestModel, AnalyzerHints, SemanticPredicates,
    //    SubTopic, AnalysisIR
    //  - Tool names ("emit_analysis", "emit_evidence")
    //  - Predicate name patterns ("predicates.<field>")
    //
    // Replacements use a small hand-curated abstraction table for the
    // user-facing surface; the schema-derived list contributes the
    // KEYS (what to strip) but not the values (what to replace with).
    abstractions := map[string]string{
        // Predicates surface — most frequent offender
        "predicates.is_cross_component":      "the cross-component flag",
        "predicates.is_scalar_answer":        "the scalar-answer flag",
        "predicates.is_count_question":       "the count-question flag",
        "predicates.is_role_locate_lookup":   "the role-lookup flag",
        "predicates.is_relational_lookup":    "the relational-lookup flag",
        "predicates.is_category_enumeration": "the category-enumeration flag",
        "predicates.is_history_lookup":       "the history-lookup flag",
        "is_cross_component":                 "the cross-component flag",
        "is_scalar_answer":                   "the scalar-answer flag",
        "is_count_question":                  "the count-question flag",
        "is_category_enumeration":            "the category-enumeration flag",
        // Tool names
        "emit_analysis": "the classification call",
        "emit_evidence": "the evidence call",
        "emit_answer_document": "the answer call",
        // IR field-set surface
        "AnalyzerHints.PrimaryEntities": "the primary entities you declared",
        "AnalyzerHints.Entities":        "the entities you declared",
        "rm.SubTopics":                  "your sub-topics",
        "rm.Predicates":                 "your predicate flags",
    }
    internalVocab = abstractions
    // Auto-discover JSON tag names that should also be treated as
    // sensitive internal vocab (e.g. "primary_entities") even when
    // not in the abstraction table — drop them via empty-string
    // replacement (i.e. just remove). Hand abstractions take priority.
    autoDiscover := map[string]bool{}
    harvestJSONTags(reflect.TypeOf(types.RequestModel{}), autoDiscover)
    harvestJSONTags(reflect.TypeOf(types.AnalyzerHints{}), autoDiscover)
    harvestJSONTags(reflect.TypeOf(types.SemanticPredicates{}), autoDiscover)
    harvestJSONTags(reflect.TypeOf(types.SubTopic{}), autoDiscover)
    for tag := range autoDiscover {
        if _, ok := internalVocab[tag]; ok {
            continue
        }
        // Tag-only fallback: replace with abstract description by tag-shape.
        if strings.HasSuffix(tag, "_question") || strings.HasSuffix(tag, "_lookup") || strings.HasPrefix(tag, "is_") {
            internalVocab[tag] = "the " + strings.ReplaceAll(strings.TrimPrefix(tag, "is_"), "_", "-") + " flag"
        } else {
            internalVocab[tag] = "(internal field)"
        }
    }
    // Build alternation regex sorted by descending length so longer
    // matches win (e.g. "predicates.is_cross_component" before
    // "is_cross_component").
    keys := make([]string, 0, len(internalVocab))
    for k := range internalVocab {
        keys = append(keys, k)
    }
    sortByDescLen(keys)
    var quoted []string
    for _, k := range keys {
        quoted = append(quoted, regexp.QuoteMeta(k))
    }
    internalVocabRE = regexp.MustCompile(`\b(?:` + strings.Join(quoted, "|") + `)\b`)
}

// sanitizeInternalVocab applies the abstraction table to s. Empty
// inputs and non-matching strings return unchanged. Used by
// plainCoherenceDetail (analyzer.go:274) at the LLM-prompt boundary.
func sanitizeInternalVocab(s string) string {
    internalVocabOnce.Do(buildInternalVocab)
    if internalVocabRE == nil {
        return s
    }
    return internalVocabRE.ReplaceAllStringFunc(s, func(match string) string {
        if rep, ok := internalVocab[match]; ok {
            return rep
        }
        return match
    })
}

func harvestJSONTags(t reflect.Type, out map[string]bool) {
    if t.Kind() != reflect.Struct {
        return
    }
    for i := 0; i < t.NumField(); i++ {
        f := t.Field(i)
        tag := f.Tag.Get("json")
        if tag == "" || tag == "-" {
            continue
        }
        name := strings.SplitN(tag, ",", 2)[0]
        if name != "" {
            out[name] = true
        }
    }
}

func sortByDescLen(ss []string) {
    sort.Slice(ss, func(i, j int) bool { return len(ss[i]) > len(ss[j]) })
}
```

#### 4.5.2 Wire site (`analyzer.go::plainCoherenceDetail`)

Modify the per-segment loop:

```go
func plainCoherenceDetail(detail string) string {
    d := strings.TrimSpace(detail)
    segments := strings.Split(d, " | ")
    for i, seg := range segments {
        stripped := stripCoherencePrefix(strings.TrimSpace(seg))
        segments[i] = sanitizeInternalVocab(stripped)  // NEW
    }
    return strings.Join(segments, " | ")
}
```

#### 4.5.3 Skill prompt prior-pollution rule

Append to `analysis_contract.go::renderOutputFormat` after the existing Sub-topic detection block (~line 384):

```go
of.WriteString("Cross-turn discipline: sub_topics are extracted from the CURRENT question only. Prior conversation entries are visible above for pronoun / demonstrative disambiguation (\"它\", \"那个\", \"this\") — they are NOT a source of sub-topic content. Lifting a prior turn's topic (e.g. earlier discussion about diagram sizing or REPL behaviour) into the current emit's sub_topics is a structural error: the current question's coverage is what the LLM downstream stages will answer.\n\n")
```

#### 4.5.4 Red-line audit checklist (BLOCKING per `feedback_prompt_redline_checklist.md`)

For each LLM-facing string introduced/modified in B5:

1. **R3** (typed signal): the new sub-topic-discipline rule references "the CURRENT question" / "prior conversation entries" — these are user-facing concepts, not pipeline internal tokens. ✅ R3 clean.
2. **R4** (no over-fitting): the rule is generic ("sub_topics extracted from current question only"), not pinned to a specific failure case. ✅
3. **R5** (no system-backfill): we instruct the LLM to extract correctly; we do NOT pre-populate sub_topics. ✅
4. **R6** (no internal info): "sub_topics" is a user-emit field name (already in skill prompt), "prior conversation" is a user-visible section title. ✅
5. **R7** (user intent over system gates): the rule reinforces user intent (current question = answer scope) without suppressing it. ✅
6. **SST** (single source of truth): section title "Cross-turn discipline" doesn't duplicate any existing section; uses sentence case consistent with surrounding skill prompt style. ✅
7. **R2'** (typed signal six-spot): no new typed signal introduced; this is prose guidance. N/A.

For the sanitizer abstractions, each entry is checked against:
- Does the replacement preserve actionable information for the LLM? (e.g. "the cross-component flag" lets the LLM know which structured field to revise) ✅
- Does any replacement leak NEW internal vocab? Spot-check: "the cross-component flag" — "cross-component" appears in skill prompt as a user-emit predicate description, so this is user-visible vocab. ✅

#### 4.5.5 Tests (`internal/agent/retry_hint_sanitize_test.go`)

- `TestSanitizeInternalVocab_PredicateNames`
- `TestSanitizeInternalVocab_ToolNames`
- `TestSanitizeInternalVocab_LongerMatchWinsOnAlternation`
- `TestSanitizeInternalVocab_EmptyInputUnchanged`
- `TestSanitizeInternalVocab_NoMatchUnchanged`
- `TestSanitizeInternalVocab_AutoDiscoveredJSONTagFallback`

### 4.6 B6 — same-gate retry-storm early-exit

#### 4.6.1 GateReport fingerprint

Add to `types.GateReport` (`analysis_ir.go:1070`):

```go
type GateReport struct {
    Passed      bool        `json:"passed"`
    Rejected    bool        `json:"rejected"`
    Retryable   bool        `json:"retryable"`
    Checks      []GateCheck `json:"checks,omitempty"`
    Fingerprint string      `json:"fingerprint,omitempty"` // NEW: stable hash of failure shape
}
```

#### 4.6.2 Fingerprint computation (`internal/analysis/gate/fingerprint.go` — new file)

```go
// Package gate — fingerprint.go
//
// computeGateFingerprint produces a stable hash of a rejection's
// SHAPE (not its specific quoted entity values) so the orchestrator
// can detect "same failure mode N retries in a row" — the LLM is
// stuck in a loop, not converging.
//
// Algorithm:
//  1. Collect every failed check Name + the rule code prefix from
//     Detail (R1.1, R1.2, ..., R2.1, ...) — case-sensitive verbatim.
//  2. Strip everything BUT the rule prefixes: drop quoted strings,
//     numeric counts, identifier lists. The fingerprint should be
//     stable across attempts that differ only in which entity the
//     LLM hallucinated.
//  3. SHA-256 the joined canonical form, hex-encode the first 12
//     bytes. Bounded length, deterministic.
package gate

import (
    "crypto/sha256"
    "encoding/hex"
    "regexp"
    "sort"
    "strings"
    "github.com/hanchaoqun/codrax/internal/types"
)

var (
    rulePrefixRE = regexp.MustCompile(`^R\d+\.\d+ [a-z_]+(?:\s+\(advisory\))?:`)
    digitRE      = regexp.MustCompile(`\d+`)
    quotedRE     = regexp.MustCompile(`"[^"]*"`)
)

func computeGateFingerprint(report types.GateReport) string {
    if !report.Rejected {
        return ""
    }
    var keys []string
    for _, c := range report.Checks {
        if c.Passed {
            continue
        }
        // Per-check: extract just the rule prefixes from the detail.
        // Multiple rules joined with " | " each contribute a key.
        for _, seg := range strings.Split(c.Detail, " | ") {
            seg = strings.TrimSpace(seg)
            if m := rulePrefixRE.FindString(seg); m != "" {
                keys = append(keys, c.Name+":"+strings.TrimSuffix(m, ":"))
            } else {
                // Fallback: hash a digit/quote-stripped version of the
                // whole segment so non-rule details still stabilize.
                clean := digitRE.ReplaceAllString(seg, "N")
                clean = quotedRE.ReplaceAllString(clean, `""`)
                keys = append(keys, c.Name+":"+clean)
            }
        }
    }
    if len(keys) == 0 {
        return ""
    }
    sort.Strings(keys)
    h := sha256.Sum256([]byte(strings.Join(keys, ";")))
    return hex.EncodeToString(h[:6])
}
```

#### 4.6.3 GateReport population

In `internal/analysis/gate/gate.go::Run` (or `RunWith` — the entry that builds the report):

```go
report := /* ...existing aggregation... */
if report.Rejected {
    report.Fingerprint = computeGateFingerprint(report)
}
return report
```

#### 4.6.4 Orchestrator early-exit (`orchestrator.go::runAnalyzePhase`)

Modify lines 1925-1959:

```go
max := o.dynamicAnalyzeRetries(o.settings.MaxRetriesPerStage)
if max < 1 {
    max = 1
}
// 2026-05-10 multirepo design §4.6: detect retry storm — same
// gate failure fingerprint repeated ≥ ceil(max/2) times means
// the LLM is stuck in a loop, not converging. Early-exit to
// minimal IR and surface a user-actionable caveat instead of
// burning the full budget.
storm := newRetryStormDetector(max)
var lastErr string
used := 0
for attempt := 0; attempt < max; attempt++ {
    used++
    o.emitStageRetryAttempt = attempt
    out, err := o.dispatchStage(types.StageAnalyze)
    if err == nil && (out == nil || out.Error == "") && o.busCtx.AnalysisIR != nil {
        return used, nil
    }
    if out != nil { lastErr = out.Error }
    if err != nil { lastErr = err.Error() }
    logging.Warning("[orchestrator] analyze attempt %d/%d failed: %s", attempt+1, max, lastErr)
    if o.busCtx != nil && o.busCtx.Mutable != nil {
        o.busCtx.Mutable.AppendAnswerRetryEvent(string(types.StageAnalyze), lastErr)
    }
    // Early-exit detection — only when AnalysisIR carries a fingerprint
    // and that fingerprint matches a recent attempt.
    if o.busCtx != nil && o.busCtx.AnalysisIR != nil {
        fp := o.busCtx.AnalysisIR.QualityGate.Fingerprint
        if fp != "" {
            if storm.observe(fp); storm.exhausted() {
                logging.Warning("[orchestrator] analyze retry storm: fingerprint %q repeated %d×; breaking early to degraded path", fp, storm.repeatCount())
                if o.busCtx.Mutable != nil {
                    o.busCtx.Mutable.AppendAnswerCaveat(retryStormCaveat(fp))
                }
                return used, fmt.Errorf("analyze stage early-exit on retry storm (fingerprint=%s, attempts=%d): %s", fp, used, lastErr)
            }
        }
    }
}
return used, fmt.Errorf("analyze stage exhausted after %d attempt(s): %s", max, lastErr)
```

Helpers (`orchestrator.go` private, OR new file `orchestrator_retry_storm.go`):

```go
type retryStormDetector struct {
    max          int
    threshold    int
    lastFP       string
    repeats      int
    seenAtLeast2 bool
}

func newRetryStormDetector(maxAttempts int) *retryStormDetector {
    threshold := (maxAttempts + 1) / 2 // ceil(max/2)
    if threshold < 2 {
        threshold = 2
    }
    return &retryStormDetector{max: maxAttempts, threshold: threshold}
}

func (d *retryStormDetector) observe(fp string) {
    if fp == "" {
        return
    }
    if fp == d.lastFP {
        d.repeats++
    } else {
        d.lastFP = fp
        d.repeats = 1
    }
}

func (d *retryStormDetector) exhausted() bool { return d.repeats >= d.threshold }
func (d *retryStormDetector) repeatCount() int { return d.repeats }

// retryStormCaveat is the user-facing caveat string the early-exit
// path appends. Audit-required (R6 / R7): NO pipeline-internal
// vocab, gives an actionable next step, doesn't claim authority
// the system doesn't have.
func retryStormCaveat(fp string) string {
    return "分析阶段在同一类结构性检查上反复未通过(重复模式 " + fp[:min(6, len(fp))] +
        ");答案已切换到部分降级生成。如果是跨子仓比较问题,可尝试 `/repos focus <子仓名>` 缩小到单子仓,或 `/repos cap N` 抬高 active 上限。"
}
```

(`AppendAnswerCaveat` is the existing surface used for fail-loud caveat injection — verify in `internal/types/context.go`. If absent we add it under the existing AnswerCaveat / FailureTaxonomy plumbing.)

#### 4.6.5 Red-line audit (BLOCKING)

`retryStormCaveat` is LLM-facing (well, user-facing — it lands in the answer panel). Audit:

- R3 typed: the fingerprint hex is `<6 hex chars>`, deterministic, opaque to the user. ✅
- R4 no over-fitting: caveat doesn't mention "codrax" / "opencode" / specific gate rules. ✅
- R5 no system-backfill: we explicitly say "已切换到部分降级生成" (degraded), not pretending to answer. ✅
- R6 no internal vocab: "结构性检查" is generic; doesn't expose `subtopic_coherence` / `R1.5`. ✅
- R7 user intent: gives the user actionable controls (`/repos focus`, `/repos cap`), preserving their agency. ✅

#### 4.6.6 Tests

`internal/orchestrator/retry_storm_test.go`:
- `TestRetryStormDetector_DifferentFingerprints_NoStorm`
- `TestRetryStormDetector_SameFingerprint_TripsAtThreshold`
- `TestRetryStormDetector_FingerprintEmpty_Ignored`
- `TestRetryStormDetector_ThresholdCalculation` (max=3 → threshold=2; max=5 → threshold=3)

`internal/analysis/gate/fingerprint_test.go`:
- `TestComputeGateFingerprint_StableAcrossDigits`
- `TestComputeGateFingerprint_StableAcrossQuotedStrings`
- `TestComputeGateFingerprint_DifferentRules_DifferentHash`
- `TestComputeGateFingerprint_PassedReport_EmptyHash`

### 4.7 B7 — end-to-end + final red-line audit

`internal/agent/multirepo_compare_e2e_test.go`:

1. Synthetic mg with two sub-repos: `codrax-fixture` (rich SymbolDefs `SymbolOracle`, `validateInlineIdentifierHallucination`) and `opencode-fixture` (rich `OpencodeClient`).
2. Build AnalyzerHints with PrimaryEntities=[codrax-fixture, opencode-fixture]
3. Run buildAnalysisIR equivalent path
4. Assert: `PrimaryScopes == [codrax-fixture, opencode-fixture]`, gate.Run returns `Passed=true`
5. Negative: same setup but `IsCrossComponent=true` and only 1 sub-topic → R1.2 fires (legacy behaviour preserved)
6. Negative: same setup, sub-topics anchor only `codrax-fixture` → R1.8 SOFT advisory only, gate still passes

Final audit checklist (B7 closing):

| Check | Method | Expected |
|---|---|---|
| L1 byte-identical single-repo | grep all new code for `IsSingle()`/`mg == nil` short-circuit; run existing single-repo eval | Same Result |
| R3 precise typed signals only | grep `if .*Predicates\.` in coherence.go, retry_hint_sanitize.go | New code uses typed booleans |
| R6 no leaks | `grep -E "(predicates\.|emit_analysis|AnalyzerHints\.)" $(prompt-and-hint files)` | No matches outside abstraction-table values |
| R2' six-spot | Manual checklist per design §4.1.5 | All N/A or addressed |
| `go test ./...` | full suite | green |
| `go vet ./...` | full suite | clean |

---

## §5 Risk register & rollback plan

| Risk | Probability | Mitigation |
|---|---|---|
| `multiRepoSymbolResolver.LookupSymbol` returns >> hits in workspaces with many sub-repos and overloaded names | Low | `maxSymbolResolverHits = 20` cap kept; multigraph fan-out yields per-sub-repo dedup |
| `detectScopesFromQuestion` false-positive on substrings | Low | Word-boundary tokenization + NormalizeCodeKey **EQUIVALENCE** (not substring) — "opencodex" canon != "opencode" canon, so no match |
| `retry_hint_sanitize` over-replaces and breaks an existing fixture | Medium | New tests; staged rollout — sanitizer is only applied at the LLM-prompt boundary, not in logs |
| Single-repo evals drift | Low | All new code paths gated on `mg != nil && !mg.IsSingle()`; B7 includes byte-identical assertions |
| Retry-storm threshold too aggressive (early-exits a converging case) | Medium | Threshold = ceil(max/2) means with `max=3` we need 2 IDENTICAL fingerprints — a converging LLM produces different fingerprints (different gate rules firing as it iterates) |

Rollback: each batch is one commit; revert order is B6 → B5 → B4 → B3 → B2 → B1. PrimaryScopes JSON is `omitempty` so reverting B1 leaves zero on-disk state.

---

## §6 Implementation order (committed sequence)

| Batch | Files | LOC est. | Commits | Tests | Push |
|---|---|---|---|---|---|
| B1 | analysis_ir.go, scope_projection.go (new), analyzer.go (1 wire site), analysis_contract.go (1 prompt para) | ~120 src + ~140 test | 1 | scope_projection_test.go (~100) | ✓ |
| B2 | symbol_resolver.go (append), analyzer.go (1 helper + 1 wire site) | ~140 src + ~100 test | 1 | symbol_resolver_multirepo_test.go | ✓ |
| B3 | coherence.go (R1.3 widen, R1.5 carve, R1.8 add, helpers), analyzer.go (stripCoherencePrefix add R1.8) | ~150 src + ~200 test | 1 | coherence_scope_test.go | ✓ |
| B4 | analyzer.go (detectScopesFromQuestion + buildMultiScopeRepoOverview + dispatch) | ~120 src + ~150 test | 1 | analyzer_multiscope_overview_test.go | ✓ |
| B5 | retry_hint_sanitize.go (new), analyzer.go (plainCoherenceDetail wire), analysis_contract.go (1 prompt para) | ~140 src + ~120 test | 1 | retry_hint_sanitize_test.go | ✓ |
| B6 | analysis_ir.go (Fingerprint field), gate/fingerprint.go (new), gate/gate.go (compute), orchestrator.go (loop), orchestrator_retry_storm.go (new) | ~150 src + ~120 test | 1 | retry_storm_test.go, fingerprint_test.go | ✓ |
| B7 | multirepo_compare_e2e_test.go + final audit grep, MEMORY.md update | ~250 test + 0 src | 1 | the e2e test | ✓ |

**Total**: 7 commits, ~1700 LOC (~820 src + ~880 test). All commits pass `go test ./...`. Each commit has a red-line audit footer in its commit message when the commit modifies any LLM-facing string.

---

## §7 Cross-references (so future grep finds this)

- `feedback_typed_signal_six_spot_sync.md` — R2' compliance table at §4.1.5
- `feedback_no_internal_info_in_llm_prompts.md` — primary motivation for B5
- `feedback_precise_signals_for_hard_gates.md` — primary motivation for B1+B3
- `feedback_user_intent_over_system_gates.md` — root cause that R1.5's resolver-asymmetry signal violated
- `feedback_no_eval_bar_relaxation.md` — we're NOT lowering the gate; we're typing the input correctly
- `project_session_multirepo_ux_phases.md` — `pickPrimarySubRepo` evolution; B4 closes the matching gap on the analyzer overview side
- `project_session_commercial_grade_100pct_passrate.md` — Pattern 3 BusContextProjection for the parallel mr_pin_isolation case; this design is the analyze-stage analog
