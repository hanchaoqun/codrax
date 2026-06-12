# repomap_v3 eval harness

Deterministic gating framework for the repomap refactor (plan memo:
`memory/project_repomap_refactor_plan.md`). Produces six metrics with
no LLM in the loop so the same commit always produces the same
numbers.

## Why this exists

The pre-existing `eval/run.sh` is an end-to-end LLM gate using
`EXPECT_CONTAINS` substrings. It produces false-green whenever the
LLM happens to emit the expected token for the wrong reason. Before
we touch repomap's data model (Phase 1: SymbolID), we need a gate
that can't lie to us. This harness is that gate.

Every metric here is computed from repomap's in-memory Graph
directly — no prompt, no sampling noise, no generation drift.

## Run it

```sh
go run ./eval/repomap_v3/harness \
  -repo . \
  -fixtures eval/repomap_v3/fixtures \
  -out eval/repomap_v3/baseline.json \
  -md eval/repomap_v3/baseline.md
```

Add `-precision-sample=N` to cap the symbol precision sample size
(default 500). Pass `-repo` a different path to profile another repo.

## Metrics

| metric | what it catches |
|---|---|
| **symbol precision** | Sampled extracted symbols whose `(file, line)` doesn't actually contain the name at ±1 window. False positives from the extractor. |
| **symbol recall** | Curated fixture symbols that don't appear in `graph.SymbolDefs[name]` with matching `(file, line±2)`. False negatives from the extractor. |
| **import edge accuracy** | Fraction of extracted imports that successfully resolved to a file via `graph.ImportGraph`. Resolver quality. |
| **call edge ambiguity** | Histogram of (# of symbol definitions matching a `call` relation's `To` field). Values > 1 are the receiver-drift bug. |
| **task_map hit@k** | For curated queries, fraction of expected files present in `QueryScores` top-K. Rank quality. |
| **scan latency** | Wall clock for a full scan. |

## Fixtures

- `fixtures/symbols.json` — 125 GENERATED symbol ground-truth entries
  (regenerated 2026-06-12; the original hand-curated 7a60dd4-era set
  had decayed to 13.6% by-file recall from repository drift alone).
  Regenerate after large refactors with:

  ```sh
  go run ./eval/repomap_v3/fixturegen -repo . -out eval/repomap_v3/fixtures/symbols.json
  ```

  Selection: exported, load-bearing (graph fan-in ranked), distinctive
  names, package quotas across 13 subsystems, every entry
  independently line-verified against file bytes. Tolerance ±2 lines.
  The harness emits a `fixture_freshness_warning` (and a stderr
  WARNING) when by-name recall far exceeds by-file recall — the
  moved-files signature meaning "regenerate me", not "extractor
  regressed".
- `fixtures/queries.json` — 35 hand-curated task_map queries (refreshed
  2026-06-12 against current HEAD). Mix of single-file lookups,
  cross-file concept queries, underscore identifiers, and bilingual
  (en/zh) queries, extended to subsystems added since the original set
  (route resolver, size tiers, cache GC, multigraph, tracequery).
  Expected_files encode human judgement — when refreshing, adjust
  EXPECTATIONS to the true answer files, never the ranker.

### Findings from the 2026-06-12 refresh

Regenerating the fixtures exposed a real rank-quality regression the
stale set had been hiding: `queryMatchScore`'s per-symbol accumulation
with a flat `Min(score, 20)` cap rewarded FILE SIZE — at 1758 files,
hundreds of large files saturated the cap on any query containing a
common token, flattening the query layer into alphabetical ties
(hit@k 0.87 → 0.14 as the repo grew). Fixed in
`retrieve/rank.go:queryMatchScoreWithTokens` by per-token saturation
(best surface hit + log-dampened repeats, no flat cap), restoring
hit@k to 0.93. Three residual imperfect queries are kept deliberately:
`BuildGraph FileInfo Symbol Relation` (0.5 — types.go ranks, build.go
sits just below the cut), `tree-sitter extractor go` (the tokenizer
does no stemming, so the token "extractor" cannot match
`extract_go.go`'s path token "extract" — derivational-morphology
limitation; the vendored-corpus pollution this query originally
exposed was fixed by excluding thirdparty trees from the scan), and
the zh query (CJK canary, imperfect by design in an all-English
codebase). The same refresh fixed a harness fidelity bug:
`topKQueryFiles` ranked by raw `QueryScores` instead of the combined
score production task_map renders, so ties degenerated to filename
order.

## Findings from first baseline at HEAD `7a60dd4`

The first run (captured in `baseline.json` / `baseline.md`) already
surfaced two pre-existing bugs and one expected Phase 1 target:

### Finding 1 — Go import extractor misses grouped imports (BUG)

`extract_go.go:goExtractImports` walks `import_spec` as direct
children of `import_declaration`, but for the common Go grouped form
`import ( "a"; "b"; "c" )` tree-sitter nests them under an
`import_spec_list`. Result: **only 21 imports total extracted across
186 files.** Every Go file with a grouped import block contributes
at most one imp (the first `interpreted_string_literal` caught by
the "single import" fallback branch).

The import_accuracy metric (0.333) reflects this degenerate sample —
the denominator is ~15× too small to be trustworthy as a gate until
the extractor is fixed. Phase 2 ImportResolver work assumes this is
fixed; Phase 1 does not depend on it.

**Fix**: Descend into `import_spec_list` before iterating
`import_spec`.

### Finding 2 — Call edge ambiguity is 78% ambiguous (Phase 1 target)

```
ambiguity | count
--------- | -----
        0 |  7442   (external calls, unresolved)
        1 |  2381   (unambiguous, 22%)
        2 |   239
        3 |   162   ← receiver drift zone (ContinuationPrompt is here)
        4 |    49
        5 |    98
        6 |   310
        9 |    38
       11 |     5
       15 |   112   ← method name noise (Error, String, Name, ...)
```

**22% unambiguous call ratio** is the precise numerical handle for
receiver drift. Symbols with multiple definitions whose `CallersOf`
returns the wrong caller set are exactly the df3 bug. Phase 1
SymbolID should push this ratio significantly toward 1.0 by moving
to `<lang>::<pkg>::<type>::<name>::<arity>` identity.

Top ambiguous targets include the ones expected: `Run`, `Error`,
`String`, `Name`, `Execute`, `Register`, `BuildInitialPrompt`,
`ParseOutput`, `ContinuationPrompt` — every one a method defined on
multiple receivers.

### Finding 3 — task_map hit@k = 0.729, CJK queries at 0% (Phase 2 target)

Of 35 curated queries, 23 (66%) hit perfectly; mean hit = 0.729.
Hard failures:

| query | hit |
|---|---|
| `propose_sub_agents tool` | 0.00 — underscore token not split |
| `有多少个agent可以调用subagent` | 0.00 — CJK not tokenized |
| `分析器如何派生 run policy` | 0.00 — CJK mixed with Latin |
| `normalizer term graph canonical` | 0.00 |
| `BuildAgentContext narrowed view` | 0.00 |
| `finalizer answer shape translation` | 0.00 |
| `extractConcreteValues evidence` | 0.00 |

Bilingual support is a plan memo requirement (see
`memory/feedback_bilingual_testing.md`) and CJK hit@k of 0% confirms
the QueryParser work from Phase 2 is real. The underscore/CamelCase
splits are adjacent: a proper tokenizer solves both.

### Expected-healthy metrics (no drift from intent)

- **symbol precision: 1.000** (500/500) — every sampled extracted
  symbol's name appears at or near its declared line.
- **symbol recall: 1.000** (125/125) — every curated ground-truth
  symbol is extracted at the correct `(file, line)`. This is the
  strongest signal the extractor's happy path works; the import
  bug is in a different code path.
- **scan latency: 0.18 s** for 186 files / 1828 symbols / 12399
  relations. Not a bottleneck; Phase 3 cache work is stability, not
  speed.

## Baseline gate rules (for Phase 1 and later)

When running Phase 1+ work, compare `baseline.json` against the new
run. Hard rules:

| metric | direction | tolerance |
|---|---|---|
| symbol precision | ≥ baseline | −0.005 |
| symbol recall | ≥ baseline | −0.008 (1 / 125) |
| call edge unambiguous ratio | ≥ baseline | strict (Phase 1 should **raise** it) |
| task_map mean hit@k | ≥ baseline | −0.03 |
| import edge accuracy | ≥ baseline once extractor bug is fixed | strict after fix |
| scan latency | ≤ baseline × 1.5 | relative |

The gate tool itself (`cmd compare` or similar) is not written
here — it's a Phase 1 prerequisite and adds nothing without a
second run to compare against. For now a human eyeballs
`baseline.md` vs the new run's markdown.

## Known limitations

- Import edge accuracy metric is too coarse: resolved/unresolved is
  reported per `fi.Imports` iteration but the resolver doesn't
  track per-import target lists, so a file with 5 imports 1
  resolved credits all 5 as resolved. Fix is a plumbing change in
  `graph.go:resolveImportGraph` to remember which Import produced
  which target. Deferred.
- Symbol precision is a ±1 line window match, not a tree-sitter
  re-parse. A symbol named on a non-declaration line (e.g. inside a
  preceding doc comment) can inflate precision. Accepted for Phase
  0 as a floor metric.
- `task_map hit@k` re-runs a full `BuildOrLoadGraph` per query to
  refresh QueryScores. The cache means this is ~10 ms per query,
  but total harness runtime still grows linearly with the fixture
  size.
