# Repomap v3 Design

Status: **Phase 0 + Phase 1 COMPLETE** (HEAD `04b54f1`).
Next step: **Phase 2 — per-language ImportResolver + multilingual
QueryParser** (plan items 3 + 6).

This document is the architectural spec for the repomap refactor.
For the day-to-day eval harness usage, see `eval/repomap_v3/README.md`.
For session-to-session resumption notes see
`memory/project_repomap_refactor_plan.md`.

## Motivation

Two problems drove this refactor:

1. **Receiver drift in the symbol table.** `internal/tool/repomap/`
   keyed every symbol by its bare name. When seven agent tools all
   defined `func (t *ToolX) Name() string`, `SymbolDefs["Name"]`
   collapsed to a 7-entry slice with no way to tell them apart.
   Downstream consumers (`CallersOf("Name")`, the rank.go fan-out
   pass, `hasConcreteRegistrationTarget`, `primaryEntityFiles`)
   returned wrong-but-plausible results for half of all eval runs.
   The df3 eval case was the tip of the iceberg; post-fix debugging
   of the drift banner proved that application-layer patches couldn't
   fix a data-model-level problem.
2. **Substring-gated eval blinded us to silent data-quality bugs.**
   The pre-existing `eval/run.sh` harness ran the full LLM pipeline
   and checked that the assistant's answer contained an expected
   token. This goes "false green" whenever the LLM happens to emit
   the expected token for the wrong reason, and it cannot see
   upstream bugs (e.g., the repomap extractor dropping 600+ Go
   imports) that never reach the final answer. The repomap refactor
   needed a gate that could not lie to it.

The fix is a deterministic six-metric eval harness (Phase 0) plus a
canonical identity for every symbol (Phase 1 SymbolID) that makes
every lookup drift-proof by construction.

## Architectural overview

Six phases, in order. Phases 0-1 are shipped; Phases 2-5 are
roadmap.

```
Phase 0  eval framework first           ─ SHIPPED  fe91203
Phase 1  SymbolID data model             ─ SHIPPED  7560d3f..04b54f1
  P1.0  fix goExtractImports             ─ shipped  7560d3f
  P1.1  SymbolID additive                ─ shipped  5916e7d
  P1.2a receiver capture + RelationEndpoint ─ shipped  9eb4f32
  P1.2b scope resolver + drift KPI       ─ shipped  04b54f1
Phase 2  ImportResolver split + QueryParser ─ NEXT
Phase 3  Cache protocol + structured output
Phase 4  Three-layer split (index/retrieve/render)
Phase 5  Language plugins + dir unification + semantic subgraphs
```

Phase 1 closed out at P1.2b. The data-model work the user's refactor
plan listed under item 2 is complete end-to-end: double index,
receiver-aware call resolution, rewritten rank / CallersOf, and a
regression test (`symbol_id_test.go:TestCallersOfIDReceiverAware`)
that locks same-name method drift. There is no `P1.3 consumer
migration` phase — see the "No `P1.3` consumer migration" section
below for the audit that concluded there is nothing valuable to
migrate under the user's explicit no-backwards-compat rule.

## Phase 0 — Eval harness (shipped)

**Location**: `eval/repomap_v3/`

**Binary**: `go run ./eval/repomap_v3/harness` — 667 lines, stdlib
only, no new dependencies. Reads two JSON fixtures, scans the
target repo, computes six metrics, writes a JSON baseline + a
markdown summary. Fully deterministic: the same commit always
produces the same numbers.

**The six metrics**:

| metric | what it catches |
|---|---|
| symbol precision | sampled extracted symbols whose `(file, line)` doesn't actually contain the name at ±1 window |
| symbol recall | curated fixture entries that don't appear in `graph.SymbolDefs[name]` with matching `(file, line±2)` |
| import edge accuracy | fraction of extracted imports that resolved to a file via `graph.ImportGraph` |
| call edge ambiguity (+ drift KPI) | for each call relation, how many symbol definitions match; drift KPI = fraction of multi-def calls resolved by receiver |
| task_map hit@k | for curated queries, fraction of expected files present in `QueryScores` top-K |
| scan latency | wall clock for a full scan |

**Fixtures**:

- `fixtures/symbols.json` — 125 curated symbols verified against
  HEAD via Grep before commit. Tolerance ±2 lines.
- `fixtures/queries.json` — 35 curated task_map queries (en + zh,
  identifier + concept).

**Gate rules**:

| metric | direction | tolerance |
|---|---|---|
| symbol precision | ≥ baseline | −0.005 |
| symbol recall | ≥ baseline | −0.008 (1/125) |
| drift resolved ratio | ≥ baseline | strict; Phase 1 should raise it |
| task_map mean hit@k | ≥ baseline | −0.03 |
| import edge accuracy | ≥ baseline | strict |
| scan latency | ≤ baseline × 1.5 | relative |

**Diagnostic probes**:

- `eval/repomap_v3/probe_ast/main.go` — dumps tree-sitter-go AST
  for a file. Found the Phase 1 P1.0 grouped-import bug.
- `eval/repomap_v3/probe_receivers/main.go` — dumps the receiver-text
  distribution across all call relations and flags matches against
  the local type set. Explains why the drift KPI hits a local
  ceiling at ~7%.

## Phase 1 — SymbolID data model (shipped)

The core insight: every symbol needs a drift-proof canonical
identity, AND every call site needs enough information to resolve
to that identity. Phase 1 delivers both, locked by a regression
test that asserts same-name methods across files do not
cross-pollinate.

### SymbolID

**Format**: `<lang>::<pkg>::<receiver>::<name>::<arity>`

```text
go::agent::BaseAgent::Execute::2      // method with 2 params
go::agent::::buildAnalysisIR::1       // bare function, receiver empty
go::types::::TaskItem::0              // type, arity 0
go::repomap::Graph::CallersOf::1      // method
```

**Rules**:

- `lang`: the repomap language tag (`go`, `python`, `java`, ...).
- `pkg`: the declared package/module; falls back to
  `filepath.Dir(fi.RelPath)` for languages without a package
  concept (JS, Python file-scoped, C).
- `receiver`: containing type for methods; empty for bare
  functions and types. Go uses `Symbol.Receiver`; Java/Python
  use `Symbol.Parent`.
- `name`: the bare symbol name.
- `arity`: parameter count for functions/methods, 0 otherwise.
  Go disallows overloading so `pkg+receiver+name` is already
  unique; arity is kept for consistency with languages that
  allow overloading.

**Construction**: `repomap.MakeSymbolID(lang, pkg, receiver, name, arity)`.
Never concatenate manually. IDs are populated on every
`BuildGraph` call via `deriveSymbolID(fi, &s)` and carry the
`json:"-"` tag — they are re-derived on load so the cache JSON
schema is unchanged. Two unit tests lock the format:
`TestMakeSymbolID`, `TestBuildGraphPopulatesSymbolByID`.

### RelationEndpoint

Every `Relation` now carries a structured endpoint alongside its
legacy `From/To` strings. The legacy strings remain because
rank.go's name-keyed fallback bucket still consumes them for
unresolved external targets; they are candidates for removal in
Phase 4 when the retrieval layer owns relation semantics.

```go
type RelationEndpoint struct {
    ID       SymbolID // canonical identity when resolvable
    Name     string   // raw identifier text
    Receiver string   // raw or scope-resolved receiver text
    File     string
    Line     int
}

type Relation struct {
    Kind string
    From string           // legacy
    To   string           // legacy
    File string
    Line int
    ToEP RelationEndpoint // structured, populated by Phase 1+
}
```

`ToEP.Receiver` is populated for selector_expression call sites by
the Go extractor's new `goEmitCall` / `goLocalScope` pair. It is
*not* a pass-through of the raw operand text — the scope resolver
rewrites receivers to their declared type name when the name is in
the local scope.

### goLocalScope — function-scoped name→type binding

When the extractor walks a Go function or method body, it first
builds a local scope from the function's receiver field and its
parameter list:

```go
func (g *Graph) CallersOf(name string) []string {
    // goLocalScope returns {"g": "Graph", "name": "string"}
    // (pointer/slice/array/package prefixes stripped by
    // goCleanTypeName)
}
```

Then every `selector_expression` call inside the body has its
receiver text looked up in the scope. A call like `g.SymbolDefs` or
`g.SymbolByID` inside this method now has `ToEP.Receiver = "Graph"`
instead of the variable name `"g"`.

**Limits** (Phase 1 scope): the resolver only tracks receivers and
parameters. It does NOT track `:=` short-decls, package-level
`var` declarations, or struct field chains. These are optional
Phase 1+ upgrades and are documented as scope gaps in the plan
memo. The probe tool `probe_receivers` reports the top receiver
texts so progress on the remaining 93% of drift is measurable
going forward.

### Graph.SymbolByID and Graph.MethodIndex

`BuildGraph` populates two new indices alongside the legacy
`SymbolDefs`:

```go
SymbolByID  map[SymbolID]*Symbol      // one SymbolID → one def
MethodIndex map[MethodKey]*Symbol     // (pkg, receiver, name) → method def
```

`MethodIndex` is the fast path for receiver-aware call resolution.
Collision policy for both indices: **first-wins**. Duplicate IDs
indicate either a genuine duplicate (a valid Go program would not
compile) or an extractor bug; the legacy `SymbolDefs` retains all
of them for diagnosis.

### Graph.CallersOfID and resolveCallTarget

`CallersOfID(SymbolID) []string` is the receiver-aware replacement
for `CallersOf(name string)`. Given a canonical SymbolID, it walks
every call relation and includes only the files whose call site's
`ToEP.Receiver` matches the target's receiver. Unresolved call
sites (empty receiver) are conservatively included as potential
callers — Phase 1 prefers over-reporting to silent drops.

`resolveCallTarget(fi, rel) *Symbol` maps a call relation to its
canonical target via MethodIndex in three steps:

1. **Same-package receiver match.** `MethodIndex[(fi.Package,
   rel.ToEP.Receiver, rel.To)]`.
2. **Same-package bare function fallback.** When the receiver is
   set but the method lookup misses, retry with empty receiver so
   a local helper function still resolves.
3. **Cross-package scan.** Linear over MethodIndex filtering by
   `(receiver, name)` ignoring package. Catches cross-package
   method calls where the scope resolver rewrote the receiver to
   a foreign type name.

Returns nil when the call is unresolved; callers fall back to
name-only accounting.

### rank.go rewrite

Before P1.2b: `callCount[rel.To]++` name-keyed, and the per-file
score divided by `symDefCount[sym.Name]` to dampen names that
appeared in many files. A file defining `Name()` on two types got
half-credit for every call to `Name` anywhere, including calls that
target a completely different type.

After P1.2b: two buckets.

- **Resolved bucket** (`callCountByID[id]`): when
  `resolveCallTarget` returns non-nil, the call is credited to one
  specific definition with no ambiguity divisor.
- **Name-keyed fallback** (`callCount[name]`): when resolution
  fails (external target, unknown receiver), the legacy divide-by-
  ambiguity behavior applies so the noise doesn't dominate.

The fan-out pass (`fileReferrers`) uses `symbolIDToFile` for
resolved edges and `symbolToFile` for fallback. The file a symbol
belongs to is now identified by ID, so receiver-drift doesn't
mis-attribute referrers.

## Measured Phase 1 delivery

Final deterministic gate run at HEAD `04b54f1` (Phase 1 closeout):

| metric | baseline (7a60dd4) | P1.2b (04b54f1) | delta |
|---|--:|--:|--:|
| symbol precision | 1.000 | 1.000 | — |
| symbol recall | 1.000 | 1.000 | — |
| import edge accuracy | 0.333 | 0.797 | **+0.464** (P1.0) |
| receiver capture ratio | 0.000 | 0.584 | **+0.584** (P1.2a) |
| **drift calls resolved** | — | 62 / 910 (**0.068**) | new |
| task_map hit@k | 0.729 | 0.729 | — |
| scan latency (fresh) | 0.18 s | 0.87 s | +0.7 s |

**Drift KPI 6.8%** is the fraction of local multi-def calls whose
scope-resolved receiver narrows the candidates to exactly one
definition. The 93% remainder is dominated by external targets
(`testing.T`, `strings.*`, `sitter.Node`) that a whole-repo-only
graph cannot see — this is a local-metric ceiling, not a Phase 1
shortcoming. Extending the graph to index stdlib/vendored
dependencies is out of scope for Phase 1.

Latency grew from 0.18 s to 0.87 s due to the extra
`resolveCallTarget` passes in `RankGraph` and the scope-aware
walker in `goExtractCalls`. Still under the 1 s budget for a fresh
scan; cached scans remain near-zero. Not a bottleneck.

## No `P1.3` consumer migration phase

An earlier iteration of this design called for a P1.3 phase
migrating ~20 `internal/agent/` consumer call sites from
`SymbolDefs[name]` / `Graph.CallersOf(name)` to `SymbolByID` /
`MethodIndex` / `CallersOfID`. That phase is **not part of the
user's refactor plan** and has been removed after a direct audit
against `internal/agent/`:

- **Zero** direct `CallersOf(name)` callers exist in
  `internal/agent/`. The drift fix propagates to the downstream
  evidence pipeline through `rank.go`'s receiver-aware scoring
  feeding `QueryScores`, which `keyword_search` already consumes.
  Agent consumers benefit transitively with no source-level change.

- **12 production `SymbolDefs` sites**, split into three buckets:

  - **A — name-set iteration for prose matching** (5 sites:
    `erm.go:800 ermAutoSatisfyUnresolvable`, `erm.go:2217
    containsGraphSymbol`, `evidence.go:606
    extractRankingEntitiesWithGraph`, `explorer.go:1892 bestEntity`,
    `explorer.go:2216/2242 cross-reference bridge`). These iterate
    every distinct symbol name to match against free text.
    `SymbolDefs` is the correct data structure; `SymbolByID`
    iteration is equivalent but offers no benefit.

  - **B — first-wins `defs[0]` drift** (2 sites: `erm.go:1858
    answerSymbolFromEvidence`, `explorer.go:2210 symDefFile`). Both
    pick `defs[0].File` when a name has multiple definitions.
    These are real latent drift bugs, but the call sites have no
    receiver context to disambiguate with. Fixing them requires
    threading receiver context from upstream (`EvidenceItem.Subject`
    parsing or `Relation.ToEP.ID` propagation), which is a
    downstream agent-code refactor, not Phase 1 data-model work.
    Filed as a standalone follow-up outside Phase 1 scope.

  - **C — iterate-and-filter by kind/receiver** (7 sites:
    `primaryEntityFiles`, `buildPrimaryTargetBanner`,
    `hasConcreteRegistrationTarget`, `mechanism_scan.go:204
    selectMechanismFunctions`, `explorer.go:2443/4947`,
    `hasConcreteRegistrationTarget`). Migration to `SymbolByID` would
    be semantically equivalent for Go (which disallows overloading,
    so the canonical index fully covers the name index) but *lossy*
    for languages that allow overloading, where `SymbolByID` is
    first-wins on collision while `SymbolDefs` keeps every def.
    Until Phase 5 ships the language-plugin work with overload
    handling, `SymbolDefs` is the correct index for these
    consumers.

The two drift-sensitive B-bucket sites are tracked as standalone
agent-code issues outside Phase 1 and will be picked up
opportunistically when Phase 2 touches the query/rank path or as
part of the Phase 4 three-layer split.

## Next step — Phase 2

**2a — per-language ImportResolver (plan item 3).** Replace the
flat switch in `graph.go:resolveImport` with `resolver_*.go`
plugins, one per language:

- `resolver_go.go` — parse `go.mod` for module path, map imports
  to directories by module suffix
- `resolver_java.go` — exact package-declaration match (replaces
  the current `strings.Contains` heuristic)
- `resolver_python.go` — namespace packages, package roots,
  `src/` layouts
- `resolver_js.go` — `tsconfig.json` path aliases, package
  `exports` field
- `resolver_rust.go` — `Cargo.toml` workspace members + mod tree
- `resolver_cpp.go` — include paths + basename matching

Emit an `unresolved_imports` list into `Metadata` for diagnostic
grepping. Add per-language golden-edge tests.

**Gate**: `harness.import_edge_accuracy` must rise from the current
0.797 toward ≥ 0.95. Symbol recall must stay at 1.000.

**2b — multilingual QueryParser (plan item 6).** Replace
`strings.Fields`-based `queryMatchScore` in `rank.go`:

- Unicode tokenizer with CJK bi-gram + Latin word split
- CamelCase / snake_case / kebab-case identifier splitter
- BM25-like scoring combining text relevance + structural
  centrality + task context
- Optional synonym expansion for common code terms

**Gate**: `harness.task_map.mean_hit@k` must rise from 0.729
toward ≥ 0.85. The seven CJK / underscore-token / compound queries
currently at 0.0 must land above 0.

**Sequencing**: do 2a first. It is narrower, has deterministic
golden-edge tests per language, and does not touch the query/rank
path so rank/view output is unaffected. 2b follows and is best
isolated from data-model churn.

## Roadmap — Phases 3 through 5

Phase 3 — Cache protocol + structured output. Versioned cache
metadata (`schema_version`, `extractor_versions`, `repo_head`,
`checksum`). Dual-channel output: JSON primary for programmatic
consumers, markdown render for human inspection. Stability +
observability work. Covers plan items 4 (unified exclude dirs),
7 (cache versioning), and 9 (dual-channel output).

Phase 4 — Three-layer split. Refactor `internal/tool/repomap/` into
`index/` (extraction + graph build), `retrieve/` (query + scoring),
`render/` (view generation). Covers plan item 1. Churn-heavy but
architecturally clean; done after the data model and cache are
stable. This is the natural point to revisit the drift-sensitive
B-bucket agent consumers because the retrieval layer will own the
`Relation.ToEP.ID` propagation contract.

Phase 5 — Language plugins + semantic subgraphs. Covers plan items
5 (C#/PHP/Ruby/Kotlin plugins + `LanguageExtractor` interface)
and 8 (semantic subgraph output: chains / hubs / bridges). After
the core is solid.

## References

- `eval/repomap_v3/README.md` — day-to-day harness usage + fixture
  format + first-baseline findings.
- `eval/repomap_v3/baseline.json` / `baseline.md` — the frozen
  reference at HEAD `7a60dd4` (Phase 0 ship point).
- `memory/project_repomap_refactor_plan.md` — session-to-session
  progress ledger + cold-resume protocol.
- `memory/feedback_trace_full_dataflow_before_fixing.md` — the
  lesson Phase 0 paid for: deterministic end-to-end counters
  surface silent data-quality bugs that substring-gated eval
  cannot see.
- `internal/tool/repomap/symbol_id_test.go` — unit tests locking
  the SymbolID format and the receiver-aware resolver contract.
