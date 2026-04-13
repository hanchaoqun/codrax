# Repomap v3 Design

Status: **Phases 0 – 3 COMPLETE** (HEAD `3eeee40`).
Next step: **Phase 4 — three-layer split** (`index/retrieve/render`),
starting with migration of the remaining views to the Phase 3
GenerateViewData / RenderMarkdown dual-channel contract.

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

Six phases, in order. Phases 0 – 3 are shipped end-to-end; Phase 4
is next; Phase 5 is roadmap.

```
Phase 0  eval framework first              ─ SHIPPED  fe91203
Phase 1  SymbolID data model               ─ SHIPPED  7560d3f..04b54f1
  P1.0  fix goExtractImports               ─ shipped  7560d3f
  P1.1  SymbolID additive                  ─ shipped  5916e7d
  P1.2a receiver capture + RelationEndpoint ─ shipped  9eb4f32
  P1.2b scope resolver + drift KPI         ─ shipped  04b54f1
Phase 2a per-language ImportResolver       ─ SHIPPED  5ea37fb..1b5a81a
  P2a.1 dispatcher scaffold + honest metric─ shipped  5ea37fb
  P2a.2 goImportResolver (go.mod parse)    ─ shipped  114c729
  P2a.3 java + python resolvers            ─ shipped  68bae24
  P2a.4 js/ts resolver (tsconfig paths)    ─ shipped  e98afa8
  P2a.5 rust resolver (Cargo + mod tree)   ─ shipped  419477b
  P2a.6 c/cpp resolver (suffix index)      ─ shipped  1b5a81a
Phase 2b multilingual QueryParser          ─ SHIPPED  ec04fcf..67982e4
  P2b.1 Unicode + CJK + CamelCase tokenize ─ shipped  ec04fcf
  P2b.2 wire into queryMatchScore          ─ shipped  5424218
  P2b.3 isTestFile rank penalty            ─ shipped  67982e4
Phase 3  cache + structured output         ─ SHIPPED  196078f..3eeee40
  P3a   unified exclude-dirs layering      ─ shipped  196078f
  P3b   versioned cache protocol           ─ shipped  b6d93a0
  P3c   dual-channel ViewData + render     ─ shipped  3eeee40
Phase 4  three-layer split + view migration ─ NEXT
Phase 5  language plugins + semantic subgraphs
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

## Measured delivery — Phases 1 through 3

Deterministic gate runs at each phase closeout:

| metric | baseline `7a60dd4` | P1.2b `04b54f1` | P2b.3 `67982e4` | P3c `3eeee40` |
|---|--:|--:|--:|--:|
| symbol precision | 1.000 | 1.000 | 1.000 | 1.000 |
| symbol recall | 1.000 | 1.000 | 1.000 | 1.000 |
| import edge accuracy (overall) | 0.333 | ~~0.797~~ † | 0.289 | 0.286 |
| **import internal_accuracy** | — | — | **1.000 (188/188)** | **1.000** |
| receiver capture ratio | 0.000 | 0.584 | 0.584 | 0.584 |
| drift calls resolved | — | 0.068 | 0.066 | 0.066 |
| **task_map hit@k mean** | 0.729 | 0.729 | **0.8714** | **0.8714** |
| scan latency (fresh) | 0.18 s | 0.87 s | 1.07 s | 0.94 s |

† The 0.797 number reported at P1.2b was the pre-P2a.1 inflated
metric that credited every import in any file with one resolved
edge. P2a.1 fixed the accounting (per-import, keyed on `(file,
raw)` pairs) and the truthful baseline emerged at 0.314. All
subsequent Phase 2a commits compared to that corrected number. The
Phase 2a gate `internal_accuracy ≥ 0.95` is computed over imports
the resolver classified as internal-to-repo (i.e. excluding
entries tagged `<lang>_external`) and reaches the ceiling 1.000 on
the Go fixture — every in-module Go import resolves to exactly one
file.

**Drift KPI 6.8%** is the fraction of local multi-def calls whose
scope-resolved receiver narrows the candidates to exactly one
definition. The 93% remainder is dominated by external targets
(`testing.T`, `strings.*`, `sitter.Node`) that a whole-repo-only
graph cannot see — this is a local-metric ceiling, not a Phase 1
shortcoming. Extending the graph to index stdlib/vendored
dependencies is out of scope for Phase 1.

**hit@k 0.729 → 0.8714** came from the Phase 2b tokenizer + rank
wiring (+0.10) plus the test-file penalty (+0.043). Target ≥ 0.85
met at Phase 2b closeout; the 6 residual non-perfect queries are
structural (CJK unreachable in English code, over-strict
expected_files in fixture, directory-segment weighting gaps that
belong to Phase 3+). Intentionally not chased to avoid over-fit.

Latency stays under the 1.5 s budget throughout. Cached scans
remain near-zero. Not a bottleneck.

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

## Phase 2 — ImportResolver + QueryParser (shipped)

### 2a — per-language ImportResolver (plan item 3)

Replaced the flat switch in `graph.go:resolveImport` with a
dispatcher table (`defaultResolvers()` in `resolver.go`) that
holds one dedicated resolver per language, all implementing the
`ImportResolver` interface:

```go
type ImportResolver interface {
    Language() string
    Prepare(g *Graph, ctx *ResolverContext) error
    Resolve(g *Graph, fi *FileInfo, imp Import, ctx *ResolverContext) []string
}
```

- `resolver_go.go` parses `go.mod` and builds an exact
  `<modulePath>/<reldir> → []files` map with v2+ version-segment
  stripping.
- `resolver_java.go` indexes every file's declared `fi.Package`
  plus a `typeInFile` map so `foo.bar.Baz` resolves by (pkg,
  class) exact match with a package-level fallback for wildcard
  imports.
- `resolver_python.go` normalizes `fi.Package` (strips trailing
  `.__init__`) and delegates relative imports to the legacy
  filesystem walker.
- `resolver_javascript.go` handles both JS and TS via a single
  shared instance. Parses `tsconfig.json` / `jsconfig.json` with
  JSONC comment pre-scrub and a longest-prefix alias compile;
  `resolveJsCandidate` probes the extension + `/index.*`
  candidate matrix. (File had to be renamed from the original
  `resolver_js.go` because `_js.go` is a Go build-tag suffix for
  `GOOS=js` and the toolchain silently excluded the file.)
- `resolver_rust.go` parses `Cargo.toml` for `[package].name` +
  `[lib].path`, discovers each crate's root at `src/lib.rs` or
  `src/main.rs`, and walks `use self::/super::/crate::` paths
  via `currentRustModuleDir` — the Rust 2018+ rule that a file
  `<dir>/<name>.rs` owns the `<dir>/<name>/` submodule tree.
- `resolver_cpp.go` is shared between LangC and LangCpp. Builds
  a per-file suffix index so `#include "net/http.h"` resolves
  specifically to `src/net/http.h` even when `src/db/http.h`
  exists.

Every resolver classifies unresolvable imports into a shared
`UnresolvedImport{File, Raw, Reason}` bucket with a
`<lang>_external` reason tag for stdlib / third-party / out-of-
repo paths, so the harness's per-import accuracy metric can
exclude them from the internal-accuracy denominator. This is how
Go's in-module accuracy hits the ceiling 1.000 (188/188 resolved)
despite 479 `go_external` entries in the same run.

Along the way P2a.1 fixed the pre-existing accuracy metric
inflation documented in the eval harness README as a known
limitation — the legacy metric credited every import in any file
with at least one resolved edge, which hid most real misses. The
`cachePayload.UnresolvedImports` slice plus a per-(file, raw)
keyed accounting in the harness replaced that shortcut.

### 2b — multilingual QueryParser (plan item 6)

Replaced `strings.Fields(query)` in `rank.go:queryMatchScore` with
`TokenizeQuery` in a new `query.go` module:

- Walks runes, splits Latin identifier runs on whitespace and
  punctuation, and decomposes each run into primary + sub-tokens
  via CamelCase (including the HTTPServer uppercase-run rule) +
  snake / kebab / dotted / slashed separators.
- Emits CJK runs as overlapping 2-rune bi-grams (Han / Hiragana /
  Katakana / Hangul), with single-rune runs becoming uni-grams.
- All sub-tokens carry weight 0.5, primary tokens carry 1.0, and
  CJK bi-grams carry 1.0 (they have no "primary vs sub" split).

`queryMatchScore` retains the same field-multiplier structure
(path=3, symbol=2, doc=1) but multiplies by `token.Weight`, which
gives sub-token hits partial credit without letting them dominate
a primary-token match.

P2b.3 added a final score adjustment: files whose RelPath matches
the `isTestFile` classifier (Go `_test.go`, Python `test_*.py` /
`*_test.py`, Rust `tests/`, Java `*Test.java` / `*Tests.java`,
JS/TS `*.test.*` / `*.spec.*`) get a 0.5 multiplier. Without it
the query `finalizer answer shape translation` ranked
`finalizer_test.go` ahead of `finalizer.go` because tests share
every target symbol. The classifier is conservative —
`internal/agent/tester.go`, `src/testimonial/page.tsx`,
`com/acme/Tester.java` are deliberately negatives.

These three commits moved mean hit@k from the long-standing 0.729
baseline to **0.8714**, clearing the Phase 2b target of ≥ 0.85.

## Phase 3 — cache + structured output (shipped)

### 3a — unified exclude-dirs (plan item 4)

`scanner.go`'s independent `excludedDirs` map had drifted from
`internal/tool/search.go:ExcludeDirs`. Unifying them exposed a
latent semantic mismatch: names like `memory`, `logs`, and `eval`
were intended to mean "repo-root top-level folders" but the
scanner matched on any path segment, which would drop legitimate
nested packages such as `internal/memory/`. The fix splits the
authoritative list in two:

- `ExcludeDirsAnyLevel` — matched at any directory depth. VCS
  internals, dependency trees, build output, IDE state.
- `ExcludeDirsRootOnly` — matched only at position 0 of a
  RelPath. Runtime artifacts and eval fixtures.

`scanner.go:isExcludedPath` layers an explicit root-only check on
`parts[0]` in addition to the any-level segment scan. `ExcludeDirs`
stays as the flat union for existing GrepTool / keyword_search /
explorer call sites whose ripgrep-glob or basename semantics
accept the any-level trade-off.

### 3b — versioned cache protocol (plan item 7)

Wrapped `fileinfos.json` in a `cachePayload` header:

```go
type cachePayload struct {
    SchemaVersion     int            `json:"schema_version"`
    ExtractorVersions map[string]int `json:"extractor_versions"`
    RepoHead          string         `json:"repo_head,omitempty"`
    WrittenAt         string         `json:"written_at,omitempty"`
    Checksum          string         `json:"checksum,omitempty"`
    Files             []*FileInfo    `json:"files"`
}
```

`LoadFileInfos` rejects on any mismatch — missing / unparseable
cache, `SchemaVersion != cacheSchemaVersion`, any per-language
entry in `ExtractorVersions` disagreeing with the current
`extractorVersions` map, or `Checksum` disagreement on SHA-256/8
over the Files slice alone. All four reject paths return nil so
the next `BuildOrLoadGraph` does a clean full scan.

The `extractorVersions` map tracks per-language generations so
that bumping a single extractor (e.g. Go to 2 after P1.0 +
P1.2a) invalidates every cached FileInfo without requiring a
full schema version bump. `repo_head` is diagnostic only and
missing git is not a rescan trigger. The checksum covers the
Files slice alone, not the header, so header additions in later
phases don't retroactively invalidate otherwise-fresh caches.

### 3c — dual-channel output (plan item 9)

Introduced `ViewData` / `ViewSection` / `ViewItem` as the
programmatic intermediate representation for repomap views, with
`RenderMarkdown(*ViewData) string` as the single forward
translation point. The Phase 3 commit migrates only the
`overview` view through the new path; the remaining four views
(`file_map`, `task_map`, `call_path`, `edit_impact`) still live
on the legacy direct-markdown path and `GenerateViewData`
returns nil for them so callers can fall through. Phase 4 will
migrate the rest as a precondition for the three-layer split.

The public API is:

```go
func GenerateViewData(g *Graph, viewType string, params ViewParams) *ViewData
func RenderMarkdown(d *ViewData) string
func GenerateView(g *Graph, viewType string, params ViewParams) string // dispatcher
```

Consumers that want to post-process results programmatically
walk `ViewData`; consumers that want rendered markdown go
through `GenerateView` (which routes any view with a data
implementation through `GenerateViewData → RenderMarkdown` and
falls through to the legacy functions otherwise).

## Next step — Phase 4

**Three-layer split + view migration (plan item 1).** Phase 4
refactors `internal/tool/repomap/` into three logical layers and
migrates the remaining views onto the dual-channel contract as a
precondition.

### Required view migrations

Phase 4 cannot start the three-layer split while four views still
hand-roll markdown, because the `render/` layer would then need
to import extractor and retrieval helpers that the index layer
owns — a cycle. Sequencing:

1. **`task_map`**: per-file subsections with score, matched
   symbols, imports/dependents list. Migrating this validates
   that `ViewSection.Subsections` is sufficient for the common
   hierarchical case.
2. **`file_map`**: flat per-file grouping by symbol kind. Simpler
   than task_map — a good second pattern for "list of sections,
   each with a nested item list".
3. **`call_path`**: BFS walk over `ImportGraph`. Item shape is
   "indent + file + symbol hints"; may need a new `Depth` field
   on `ViewItem`, or a recursive `Subsections` chain.
4. **`edit_impact`**: direct + transitive dependents + exported
   symbols + caller counts. Straightforward once the pattern is
   established.

After these migrate, the legacy view bodies in `views.go` can be
deleted and `GenerateView` becomes a pure dispatcher.

### Three-layer split

Refactor into three logical layers. Two variants:

- **Directory split**: `index/`, `retrieve/`, `render/`
  sub-packages. Cleanest but forces import-path changes across
  every caller.
- **File-prefix split**: `index_*.go`, `retrieve_*.go`,
  `render_*.go` kept in one package. Less architecturally clean
  but less disruptive to downstream imports.

Allocation:
- **index/**: `graph.go`, `cache.go`, `parser.go`, `scanner.go`,
  every `extract_*.go`, every `resolver*.go`, and the structural
  types from `types.go`. Every file whose output is persisted
  into the cache belongs here.
- **retrieve/**: `rank.go`, `query.go`, plus the `CallersOfID`,
  `TransitiveDeps`, and `TopFiles` helpers currently on `*Graph`.
  Everything that turns a query + graph into a ranked result set.
- **render/**: `views.go` + `render.go` after every view is on
  the dual-channel path. The only non-test consumer of
  `GenerateView` today is `tool.go:110`.

The natural time to revisit the drift-sensitive B-bucket agent
consumers (`erm.go:answerSymbolFromEvidence`, `explorer.go:
symDefFile`) is here, because the retrieval layer will own the
`Relation.ToEP.ID` propagation contract.

## Roadmap — Phase 5

Language plugins + semantic subgraphs. Covers plan items 5
(C#/PHP/Ruby/Kotlin plugins + `LanguageExtractor` interface) and
8 (semantic subgraph output: chains / hubs / bridges). After the
core is solid.

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
