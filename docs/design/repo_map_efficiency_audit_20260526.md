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
