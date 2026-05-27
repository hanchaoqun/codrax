# Repo Map Efficiency Audit — 2026-05-26

## Goal

Make `repo_map` useful as a high-efficiency navigation tool across question
types without letting the system replace user intent or model-authored answers.
The tool remains a navigation/candidate-universe carrier. Semantic claims still
need `read_file` / targeted `grep` / accepted external observations before they
become answer evidence.

This audit covers single-repo and active-sub-repo runs. Model-visible guidance
must not depend on internal implementation terms: if the prompt or a tool refusal
lists active sub-repos, the model should choose one of those visible paths.
Otherwise, ordinary single-repo `path="."` semantics apply.

## Efficient Use Matrix

| User/task shape | Efficient repo_map path | What must still be verified |
|---|---|---|
| First orientation for architecture/mechanism | `overview` → `task_map` / `file_map` | Read selected files before behavioural claims |
| Scoped member/count/list inventory | `source_inventory` with broad `roles`, `include_attributes=false`; narrow with `scope/scopes`, `attribute_roles`, paging | Read selected definitions before semantic descriptions |
| Call/dispatch/trace/import/inheritance/reference relation | `task_map/file_map` to choose sources → `relation_map` with `sources` or `scope/scopes`, narrowed `relation_kinds` | Read source/target/observed files before citing code behaviour |
| Topology questions: hubs, chains, bridges | `semantic_subgraph` | Read files before role/responsibility claims |
| Change impact from a known file | `edit_impact` with `target_file` | Read affected files before explaining impact |
| Concrete entry-point path trace | `call_path` with `entry_point` | Read the path windows before final trace |
| Config/route/literal/API surface inventory | `source_inventory` roles such as `config_key`, `route`, `literal_value`, `import_path` | Read selected definitions/registrations |
| Multi-active-sub-repo comparison | Call the same view once per visible active sub-repo path; compare results after verifying selected files | Do not treat one sub-repo as proof for another |
| External/log/trace/VCS/current-source hybrid | Use observation ledger/external carrier first, then `repo_map` only to locate current-source anchors | External observation is not repo file:line unless separately anchored |

## Audit Findings

1. **View selection teaching was scattered.** The tool schema mentioned all
   views, but `semantic_subgraph`, `edit_impact`, `call_path`, and active
   sub-repo usage were not taught as a compact matrix in the explorer primer.
   Result: the model can fall back to broad grep/read even when an existing view
   would narrow the graph cheaply.

2. **Active sub-repo wording was too abstract.** "Multi-repo workspace" is not a
   reliable model-facing concept. The visible contract is: "the context or tool
   refusal lists multiple active sub-repos". Guidance should use that phrase.

3. **Duplicate sub-repo prefixes can make valid relation lenses empty.** In a
   run where `path="repo-a"` selects a sub-repo, the model may still pass
   `sources=["repo-a/file.go"]` or `scope="repo-a/pkg"`. That is a benign human
   mistake. The tool may safely normalize only when the active-set router has
   already selected the same sub-repo and the secondary parameter starts with
   that exact prefix; it must also show a parameter advisory so the model can
   self-correct on later calls. Other prefixes stay untouched.

4. **Broad relation_map fallback needs a narrowing advisory.** `relation_map`
   with no `sources`, `query`, or `scope` is legal, but for large repositories it
   can be low-signal. The tool should return a clear next-step suggestion, not
   crash or silently encourage broad reads.

5. **Cross-repo relation semantics remain limited.** Current safe behaviour is
   explicit per-active-sub-repo calls plus answer-side comparison. A true
   aggregate cross-sub-repo relation view would need a separate design because
   it can mix unrelated graphs and accidentally imply interaction/absence.

6. **Relation-provider coverage is not identical to relation_map view coverage.**
   The typed provider already covers implements, extends, called-by, imports,
   exports, and references across single/multi graph carriers. `relation_map`
   renders graph edges: calls, imports, inheritance/implements, references, and
   type usage. Evidence/observation-only relations such as configures,
   routes-to, and source-anchor should continue flowing through typed evidence /
   observation ledger until graph primitives exist.

7. **Language support is a graph contract, not per-language prompt logic.**
   The guidance must remain language-agnostic and rely on repomap's supported
   read languages: Go, Python, JavaScript, TypeScript, Java, Kotlin, Rust, C,
   C++, Ruby, Swift, Lua, Proto, ArkTS, and Cangjie. No view-selection rule may
   special-case only Go.

8. **Navigation hints can lose priority after the analyzer handoff.**
   The Linux `io_uring` navigation test showed the analyzer eventually emitted
   high-confidence `required_files` for `io_uring/net.c`, `io_uring/opdef.c`,
   `include/uapi/linux/io_uring.h`, and related current-source anchors, but
   explorer start-up still hard-focused on a single exact keyword hit in
   `tools/include/io_uring/mini_liburing.h`. Root cause: the existing
   `AnalyzerHints.RequiredFileHints` lane was validated and pre-read, but the
   unique exact-anchor shortcut did not consume it as a stronger multi-file
   signal. This is a priority-contract gap, not a repo_map view issue.

9. **Evidence repair targets were treated as authoritative locations.**
   In the same test, line recovery suggested `io_uring/notif.h` for symbols
   whose actual current-source definitions lived in `io_uring/net.c` or
   likely `include/linux/io_uring_types.h`. The mid-loop repair hint told the
   model to repair the existing anchor file first, which made a stale
   recovery look like a mandatory same-file loop. Tool repair targets are audit
   feedback; if a just-read gutter proves a row stale, the model must be
   allowed to emit a replacement or omit the row without widening blindly.

10. **Supporting coverage can leak into principal key-file lists.**
    Finalization correctly completed in one pass, but the answer's key-file
    section included `io_uring/timeout.c`, which only appeared as
    supporting/concrete-value context and not as a load-bearing SEND/RECV
    mechanism file. Root cause: principal-boundary guidance already says
    support lanes do not create principal members, but it did not explicitly
    constrain "key files / related files" sections, where support files can
    look useful. This must remain prompt-side guidance; the system must not
    rewrite or delete model-authored tables.

11. **Cross-file type-definition follow-up needs evidence-derived inputs.**
    When a mechanism answer mentions central structs such as `io_kiocb` or
    `io_ring_ctx`, the current repair loop may not reopen the correct defining
    file after a stale recovery is detected. The repository already has a
    graph-backed cross-file symbol reference advisory, so the fix is not a new
    mechanism: feed non-grounded evidence anchors/summaries into that advisory
    alongside investigation prose. This keeps the behaviour soft, typed, and
    language-agnostic.

12. **Large-repo "cache write" stalls were mostly parse-tail stalls.**
    The Linux `io_uring` run after clearing
    `~/.codrax/cache/repomap/linux-284507f5` showed a 213.4s first scan. Log
    timestamps prove the visible tail was not primarily sidecar write:
    `cache_write` lasted about 1.8s, `build_graph` about 3s, and `rank` about
    0.2s. The long gap was parse tail: progress reached
    `63894/63895` at 23:30:13 and only entered graph build at 23:32:04. Linux
    contains many generated AMD register headers in the 5-24MB range; with
    FIFO parse scheduling a huge file can land last and make the UI look stuck.
    Fixes must not skip or downgrade parsing, because that would lose repo_map
    precision. Safe fixes are: schedule larger source files first, surface the
    currently parsed large file locally, and keep full parsing semantics. After
    the cache was fully written, a warm-cache check reached
    `仓库索引 linux 已就绪：缓存命中，93459 个文件` in 12.1s, confirming that
    repeated scans should reuse sidecars when the cache directory is complete.

13. **Derived sidecar writes can still add avoidable RSS spikes.**
    The same Linux cache directory was about 1.5GB:
    `fileinfos.*.d` 1.3GB, `relations.md` 124MB, `symbols.md` 82MB. FileInfo
    chunks already stream during parsing, so the remaining write path to watch
    is derived sidecars. The old writer assembled markdown sidecars in a
    `strings.Builder`, temporarily duplicating large buffers in memory. This is
    not the dominant latency in the observed run, but it is a real peak-memory
    cost on huge repositories. Sidecars should stream through atomic temp files
    and report byte progress with bounded UI frequency.

14. **Natural-language navigation now proves repo_map is discoverable, but
    exploration convergence is still a separate gap.** A Linux `io_uring`
    question that did not mention `repo_map` still led the analyzer to call
    `repo_map(view="task_map")` before targeted grep/read, which validates the
    model-visible navigation teaching. The same run later reached 32 exploration
    rounds because high-ranked/pre-scan files and evidence-repair follow-up
    continued after the model had already produced a sufficient investigation
    summary. That is not a repo_map cache/progress bug; it should be tracked in
    the explorer convergence workstream so navigation facts remain high-priority
    without becoming hard same-topic loops.

15. **Memory limits should remain soft, not precision-losing.** Large Linux
    scans can legitimately hold several GB of graph state because repo_map must
    preserve symbol/relation precision. Codrax already supports a soft
    `memory_soft_limit_fraction` runtime knob that maps to Go's heap soft
    target. The repo_map fix should reduce avoidable spikes (streamed sidecars,
    large-first scheduling, progress visibility) but must not implement a hard
    RSS cap by dropping parse detail. Operators can lower the soft fraction for
    huge repositories; a hard process cap belongs to the host/container layer.

16. **Warm-cache validation progress can also become noisy.** A warm-cache
    Linux check showed the right behaviour functionally (`cache hit` in 12.1s),
    but non-TTY output emitted one permanent line per 1000 checked files during
    cache-difference hashing. That is too much for large repos. The same
    progress contract now uses coarser 10000-file / 5s change-scan progress
    while preserving immediate start and final cache-hit lines.

17. **Typed relation prompt probes can stall "Preparing context" while adding
    zero hints.** Customer log
    `../customlogs/codrax-20260527-162832-000-98849.log` shows `build_agent_context`
    spending about 69s per downstream stage in `typed_relation_probe`. Each run
    probed graph-backed carriers (`*multigraph.MultiGraph` and legacy
    `*types.Graph`) for a call/called-by prompt hint, spent about 34s per graph,
    and produced `added_hints=0`. Observation/evidence carriers already skipped
    instantly because the request had six broad analyzer entities and no single
    exact relation source. Root cause: `ProbeTypedRelations` asks graph carriers
    for source facts before it knows whether an expensive prompt-only relation
    probe can ever be narrowed. On large graphs that means optional prompt
    guidance can synchronously scan broad symbols/relations during analyzer →
    explorer/extractor/finalizer context assembly. This must stay a soft prompt
    hint: coverage gates and final answer evidence already have their own exact
    contracts. The safe fix is to preflight the typed query and skip graph-backed
    expensive prompt probes unless the source lane is narrow/exact enough to
    produce useful hints; evidence/observation carriers and exact single-source
    graph probes remain enabled.

## Task List

| ID | Status | Task | Validation |
|---|---|---|---|
| RME-T0 | Done | Audit `repo_map` schema, explorer primer, active-set gate, relation provider, source inventory lens, relation_map renderer | This document |
| RME-T1 | Done | Add model-visible view-selection matrix and active-sub-repo recipe to tool schema + explorer primer | `TestRepoMapSchemaTeachesLensParameters`, `TestBuildInitialInstructionRetry` |
| RME-T2 | Done | Normalize duplicate selected-sub-repo prefixes in `sources/scopes/target_file/entry_point` after active-set routing, and prepend a model-visible parameter advisory only when a correction actually happened | `TestRepoMapLensParamsStripSelectedSubRepoPrefix`, `TestRepoMapLensParamsNoAdvisoryWhenAlreadyRelative` |
| RME-T3 | Done | Add broad `relation_map` narrowing advisory without rejecting legal calls | `TestGenerateViewDataRelationMapBroadFallbackAdvisesNarrowing` |
| RME-T4 | Planned | Add eval coverage for source_inventory broad/narrow, relation_map two-stage, semantic_subgraph, edit_impact, call_path, and active-sub-repo comparison | New eval cases |
| RME-T5 | Planned | Decide whether cross-sub-repo aggregate relation view is safe; default stays explicit per-active-sub-repo calls | Design update + tests if implemented |
| RME-T6 | Planned | Revisit relation_map graph coverage only when graph primitives exist for registers/configures/routes-to/source-anchor; otherwise keep evidence/observation carriers | Typed-provider tests |
| RME-T7 | Done | Let high-confidence multi-file `AnalyzerHints.RequiredFileHints` suppress a contradictory single exact-anchor hard focus, while preserving exact-anchor focus when no stronger multi-file handoff exists | Explorer unit tests + Linux `io_uring` prompt inspection |
| RME-T8 | Done | Reword evidence-repair summaries and mid-loop hints so recovered/ungrounded targets are audit candidates, not mandatory same-file proof; allow replacement/omit when the just-read gutter proves staleness | Tool + explorer unit tests |
| RME-T9 | Done | Add prompt-side principal-boundary guidance for "key files / related files" sections so supporting/concrete-value rows do not become principal file lists | Finalizer prompt test / inspection |
| RME-T10 | Done | Reuse the existing graph-backed cross-file symbol advisory for central symbols mentioned in non-grounded evidence rows, across all repomap languages | `TestDetectCrossFileSymbolGaps/non-grounded evidence text can surface lowercase C-style definitions`, `TestDetectCrossFileSymbolGaps/graph contract is language agnostic` |
| RME-T11 | Planned | Add eval coverage for large-repo two-stage repo_map navigation with stale repair and support-lane leakage checks | New eval cases |
| RME-T12 | Done | Reduce large-repo first-scan tail confusion without losing precision: parse larger source files first, emit local active-large-file progress, keep full tree-sitter parsing, and log slow parses | `TestParseJobOrderLargeFilesFirst`, active-file progress tests, Linux log timing analysis |
| RME-T13 | Done | Stream derived cache sidecars atomically and report bounded byte progress (`written/estimated total`) during cache-write phase; throttle same-file permanent progress to roughly 32MiB/5s plus start/end so logs do not spam | `TestSaveCacheWithProgressReportsSidecarBytes`, repo-map scan message byte-progress tests |
| RME-T14 | Done | Run a large-repo natural-language autonomy validation that does not mention `repo_map`, using Linux `io_uring` as the manual case, to verify the analyzer/explorer select repo_map lenses on their own | `codrax-20260526-235421-000-24909.log`: analyzer called `repo_map(view="task_map")`; follow-up explorer convergence gap recorded above |
| RME-T15 | Done | Throttle warm-cache difference-check progress so non-TTY/permanent output stays readable on 90k+ file repos while TTY still shows a live status line | `TestRepoMapScanProgressThrottlesChangeScanEvents` |
| RME-T16 | Planned | Add portable eval coverage for large-repo-style navigation without depending on local `../linux`, including warm-cache reuse, stale repair, and support-lane leakage checks | Synthetic fixture + existing eval harness |
| RME-T17 | Done | Add a typed relation prompt-probe preflight so graph-backed expensive prompt hints skip before source-fact/relation scans when the source lane is broad/ambiguous; preserve observation/evidence carriers and exact single-source graph hints | `TestProbeTypedRelations_AmbiguousExpensivePromptHintSkipsSourceFactProvider`, `go test ./internal/context ./internal/types ./internal/tool/repomap/relation ./internal/tool/repomap/multigraph` |

## Red-Line Guardrails

- `repo_map` rows are verified navigation/candidate-universe facts, not final
  semantic answer text.
- The system may add independent guidance or tool-output advisories; it must
  not rewrite, delete, or replace model-authored final answer content.
- Multi-active-sub-repo guidance must be visible and operational: choose one
  active sub-repo path, keep follow-up parameters relative to it, and repeat per
  active sub-repo when comparing.
- Hard gates must consume precise signals only. Broad relation hints and
  low-signal fallback rows are soft guidance.
