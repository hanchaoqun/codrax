# JSON Payload and Model Cognitive Load Gap Cluster

Date: 2026-05-23

Status: partially implemented. Batches 1-41 are complete and verified; Batch 42
is the active external-observation provenance pass. The shared schema-aware repair
path is active across the high-frequency structured emit tools, including the
legacy top-level JSON-string repair wrappers. Remaining work is tracked below as
explicit batches so implementation does not rely on memory: visible supplement
authority, external observation refs, runtime provenance, analyzer fast paths,
hybrid explorer partitioning, retry telemetry, and diagram renderability.

Implementation progress:

- 2026-05-23 Batch 1 completed: added a shared tool-boundary
  `StructuredPayloadCompat` entry point that reuses `internal/toolparam` and is
  now called by `emit_analysis`, `emit_evidence`,
  `emit_investigation_complete`, `emit_log_triage`,
  `emit_answer_document`, and `emit_answer_document_patch`. This is structural
  carrier repair only; it does not author, delete, or reorder answer content.
- Batch 1 also added operator telemetry for repaired payload byte size,
  top-level array lengths, and repair summaries, plus a direct
  `emit_evidence` regression for JSON-string `items` with camelCase keys.
- 2026-05-23 Batch 2 mitigated the `emit_evidence.items` + misplaced
  `salience` class from E20260522-G121: item-only metadata is moved into the
  sole item only when that mapping is lossless; multi-item payloads stay strict
  and receive a precise `items[i].field` schema hint.
- 2026-05-23 Batch 3 extended the same shared compatibility entry point to the
  remaining extractor / runtime segmentation emit tools:
  `emit_answer_symbol`, `emit_hypothesis_verdict`,
  `emit_log_segmentation`, and `emit_perf_segmentation`.
- 2026-05-23 Batch 4 added a structural regression guard so the current
  structured emit tools cannot accidentally bypass `StructuredPayloadCompat`
  in future edits.
- 2026-05-23 Batch 5 closed the main perf/trace emitter gap:
  `emit_perf_trace` now shares the same compatibility layer as log/perf
  segmentation and is covered by a string-wrapped trace-observation regression.
- 2026-05-23 Batch 6 extended the same structural carrier repair to write-mode
  emitters where large arrays recur (`emit_write_analysis`,
  `emit_change_plan`, `emit_plan_skeleton`, `emit_test_results`) without
  changing their semantic validators.
- 2026-05-23 Batch 7 tightened the principal ledger handoff: aggregate
  `member_notes[]` now enter `ObservationLedger.RichNotes` ahead of dry member
  names, so finalizer/reviewer prompt budget preserves explorer-authored
  explanations instead of only seeing mechanical row ids.
- 2026-05-23 Batch 8 tightened the append-only system-supplement boundary:
  `normalizeAggregateMemberSetCarriers` now reuses the full visible-answer
  member coverage check before appending deterministic aggregate-member blocks.
  If the model already rendered every accepted member with its typed location
  in prose, table text, or structured items, the runtime does not add a
  duplicate dry carrier. This keeps hard exhaustive/relation obligations
  available for truly missing members while avoiding a second system-generated
  answer surface.
- 2026-05-23 Batch 9 moved another local-model carrier mismatch into the
  shared schema-aware layer: when a schema field expects an array of objects
  and the model emits a single object, `internal/toolparam` now wraps it as a
  one-element array and then applies normal nested repairs. Array-of-string
  fields remain strict unless the existing explicit `split_string_arrays` knob
  is enabled, so this benefits structured row carriers without parsing
  user/model prose or inventing rows.
- 2026-05-23 Batch 10 tightened the non-lossless truncation path. When tool
  arguments are still invalid after bounded structural repair, the agent now
  distinguishes truly malformed JSON from partial/truncated JSON and returns a
  concise typed failure: preserve the same semantic facts, re-emit a smaller
  native JSON object, and keep model-authored prose in text/summary fields.
  This does not complete missing JSON or invent rows; it only reduces retry
  entropy when the previous payload was mechanically cut off.
- 2026-05-23 Batch 11 wrapped the remaining top-level string-wrapped
  array/object tolerance behind the shared compatibility boundary. Tools that
  still need the legacy flat-mode helpers now call
  `applyStructuredPayloadCompatWithLegacyStringFieldRepair`, then the normal
  schema-aware compat pass. This keeps behavior-preserving legacy recovery for
  answer-document/log/evidence/analyzer payloads while removing the scattered
  "local helper then shared helper" pattern from tool implementations.
- 2026-05-23 Batch 12 tightened the first `PayloadRef / RowSetRef` handoff
  without inventing new storage. The observation ledger now carries typed
  `payload_ref`, `row_set_ref`, and `page_ref` fields alongside legacy
  `raw_ref`, maps existing tool-result blob refs into `payload_ref`, and renders
  finalizer/reviewer sources with explicit labels. This makes git/diff/log/trace
  and command payloads visible as origin-specific evidence refs instead of
  unlabeled strings or fake repo citations.
- 2026-05-23 Batch 13 moved observation source-ref formatting into
  `types.FormatObservationSourceRef` so finalizer and reviewer consume the same
  labeled external-payload contract. This is a guardrail against future drift
  where one consumer sees `payload_ref`/`row_set_ref` and another falls back to
  an unlabeled `raw_ref` string.
- 2026-05-23 Batch 14 made attached log/trace observations explicitly
  artifact-addressable in the ledger. Runtime-artifact records now carry stable
  `artifact_id=attached_log` / `artifact_id=attached_trace` plus their
  artifact-local line/time spans, so finalizer/reviewer can discuss "line N in
  the attached log/trace" without converting those lines into repo citations.
- 2026-05-23 Batch 15 started the typed repair-hint lane for deterministic
  answer-document carrier rejects. Full emits with empty `blocks[]` now return a
  structured `ToolRepair` instead of prose-only failure, and patch mutation
  failures for citation-mode conflicts, adding an existing block, or replacing
  the citation pool while preserving citation-bearing blocks now carry stable
  repair codes and field paths. This does not loosen validation or alter answer
  content; it gives finalizer a typed repair target when runtime cannot safely
  normalize the carrier.
- 2026-05-23 Batch 16 extended the same lane to answer-document-specific deep
  recovery. When a JSON-string / partially truncated `blocks` payload can
  recover some structured blocks or display attachments but cannot preserve
  every visible model-authored block, the tool now rejects with
  `answer_doc_lossy_blocks_string_recovery`, typed `blocks` field metadata, and
  candidate/recovered counts. The runtime still keeps recovered draft
  attachments for display fallback, but it does not publish a partial structured
  answer or ask the model to infer the missing carrier from prose.
- 2026-05-23 Batch 17 tightened the prompt-dedupe / rich-ledger handoff without
  adding another prompt surface. Observation Ledger notes now use a typed
  role/origin-aware budget: principal records and origin-specific principal
  observations can show more de-duplicated rich notes, while support-only rows
  stay compact. This keeps explorer-authored member explanations available to
  finalizer without duplicating the full aggregate body elsewhere.
- 2026-05-23 Batch 18 connected pre-emit hard structural hints to the same
  `ToolRepair` lane. When pre-emit rejects for already-typed answer-document
  contract gaps, the tool result now carries de-duplicated schema fields,
  violation-kind metadata, and bounded expected-shape text alongside the
  existing human-readable correction list. This keeps the gate unchanged while
  reducing model burden on same-turn JSON repair.
- 2026-05-23 Batch 19 extended typed repair metadata to strict JSON decode
  failures on the high-frequency structured emit tools. Unknown fields,
  known misplaced fields, and JSON-string carriers now produce stable
  `ToolRepair` codes (`tool_param_unknown_field`,
  `tool_param_misplaced_field`, `tool_param_json_string_carrier`) while
  preserving the existing sanitized error text. Initial call sites cover
  answer-document full/patch emits, `emit_evidence`, `emit_answer_symbol`, and
  `emit_hypothesis_verdict`.
- 2026-05-23 Batch 20 extended the same typed strict-decode repair metadata to
  the log/perf runtime artifact emitters and segmenters. These tools keep their
  historical non-nil Go error return on malformed parameters, but their
  `ToolResult` now carries the same stable repair codes. This keeps retry
  guidance consistent for attached logs, traces, and command-derived runtime
  artifacts without weakening business validation.
- 2026-05-23 Batch 21 closed the remaining structured-emitter decode gaps in
  analyzer, investigation completion, and write-mode emitters. Tool-specific
  schema reminders are preserved where they already existed, but the result now
  also carries typed strict-decode repair metadata. Business tools such as
  `read_file`, git helpers, shell execution, and patch application remain
  outside this JSON-carrier lane because their failures are semantic parameter
  errors, not model-authored answer serialization.
- 2026-05-23 Batch 22 added the missing shared compatibility boundary for
  `emit_plan_change` and extended the structural coverage guard to include it.
  This keeps the partial-plan per-file body emitter aligned with the rest of the
  structured emitter family instead of relying on a local strict-decoder path.
- 2026-05-23 Batch 23 added a structural guard that every supported structured
  emitter must attach typed decode-repair metadata on JSON carrier failures.
  This is a developer-facing regression fence: new local `invalid params`
  branches in structured emitters must route through the shared repair lane, or
  the test fails.
- 2026-05-23 Batch 24 narrowed deterministic table-hole filling so it respects
  model-authored table surfaces. Enumeration row repair now fills only
  already-missing cells whose existing column headers clearly map to typed
  row fields; it no longer rewrites model-authored column titles or row labels.
  If the model table shape is incompatible but the accepted row contract has
  richer location/note data, the runtime preserves the model table and appends a
  separate localized system-verified supplement.
- 2026-05-23 Batch 25 aligned the semantic reviewer with the finalizer's
  Observation Ledger surface. Reviewer observations now carry compact `value`,
  de-duplicated `rich_notes`, and support-ref counts in addition to
  origin/source/span/summary. This closes a prompt-ledger gap where VCS, diff,
  command, runtime, or MCP observations could be rich enough for finalizer but
  appear dry to the reviewer, causing avoidable "answer too thin" pressure.
- 2026-05-23 Batch 26 normalized source-inventory scope resolution across
  `./path`, cleaned relative paths, Windows-style separators, and absolute path
  suffixes that point back into the active repo graph. This keeps typed
  source-inventory carriers scoped to the user's requested package/file without
  broadening to sibling files or relying on prose keywords.
- 2026-05-23 Batch 27 surfaced compact external-observation excerpts in both
  finalizer and semantic-reviewer Observation Ledger prompts. `RawExcerpt` was
  already compiled for git/diff/command/log/trace/MCP records, but was not
  visible downstream; now non-current-source observations can carry a bounded
  excerpt alongside summary/value/notes without creating fake repo citations.
- 2026-05-23 Batch 28 added optional automatic row-set artifact creation for
  large structured aggregate carriers. The ledger compiler remains pure by
  default; finalizer/reviewer call sites with a WorkDir install a blob-backed
  writer. Large `member_set` rows now get a `row_set_ref` JSONL artifact while
  the prompt keeps only the compact prioritized ledger row, preserving rich
  per-member notes without forcing the model to emit or reread huge JSON arrays.
- 2026-05-23 Batch 29 added the first transactional answer-document patch
  normalization pass for lossless id-surface issues. Patch op ids are trimmed,
  duplicate id-list entries are dropped, and byte-identical duplicate
  `replace_blocks` / `add_blocks` entries are coalesced before semantic
  validation. Same-id blocks with different payloads are still rejected, so the
  runtime never chooses between conflicting model-authored answer variants.
- 2026-05-23 Batch 30 extended automatic row-set artifact creation from complete
  member carriers to exact `excluded_count.excluded[]` carriers. Exact exclusion
  lists now get the same JSONL `row_set_ref` treatment as large member sets,
  while partial exclusions remain inline/count-only and do not create row-set
  artifacts. This keeps "present plus absent/excluded" mixed answers from
  forcing the model to serialize large negative-side row lists.
- 2026-05-23 Batch 31 added stable-anchor de-duplication inside the Observation
  Ledger compiler. Records with the same typed origin, source/span, claim key,
  value, and anchor shape are merged before prompt budgeting; unique summaries,
  rich notes, and support refs are preserved. Distinct claims on the same line
  remain separate, so the runtime reduces prompt duplication without changing
  answer content or collapsing different facts.
- 2026-05-23 Batch 32 extended the same lossless id-surface transaction rule to
  full `emit_answer_document` emits. Whitespace around block ids is normalized
  and byte-identical duplicate blocks are coalesced before merged-doc
  validation; same-id blocks with different visible or typed payloads still
  fail loudly. This removes another deterministic retry source without
  replacing model-authored answer content.
- Remaining work starts at P1 carrier compilers / row-set compilers; the
  completed ref batch deliberately did not change answer materialization policy.

## 2026-05-23 Batch 33+ Delivery Queue

This queue is the active implementation ledger after Batches 1-32. It is
ordered by customer-visible ROI and by architectural leverage. Each batch must
preserve the same red lines as the earlier work: no hard decisions from
user/model prose keyword matching, no replacement or deletion of model-authored
answer content, and no fake current-source `file:line` citations for external
observations.

- Batch 33 — VCS/git history tool contract. Status: completed.
  - Add typed `git_log` controls for merge/history questions:
    `merges_only`, `no_merges`, `first_parent`, and optional `ref`.
  - Keep these as semantic tool parameters, not shell fragments, and reject
    only precise invalid combinations such as `merges_only && no_merges`.
  - Regression coverage: latest merge/last-N commit summaries, pathspec
    scoping, and no unsupported `git show --no-stat` style guidance leakage.
  - Delivered in this batch: `git_log` now exposes the typed controls above,
    rejects only precise invalid combinations / unsafe revision tokens, and has
    fixture-backed coverage for latest merge, no-merge filtering, explicit ref
    starting points, pathspec scoping, and tool-description guidance.

- Batch 34 — VCS answer-shape contract. Status: completed.
  - Treat "recent merge / recent commits / compare commits / explain impact" as
    VCS narrative evidence unless the user also asks for current-source
    analysis, diagrams, tests, logs, or trace correlation.
  - Preserve commit subject/body/stat/name-only summaries as rich observation
    notes; never collapse a feature-summary answer to only `value=<commit>`.
  - Mixed history + current-code questions must rank both origins by the typed
    request shape instead of fixed origin preference.
  - Delivered in this batch: pure typed commit-history comparisons now keep
    `vcs_metadata` / `vcs_diff` origins without forcing `current_source`;
    mixed history+diagram/change-impact/current-code routes still keep current
    source. `git_log` filter coordinates (`ref`, `count`, `first_parent`,
    `merges_only`, `no_merges`, `pathspec`) are projected into the Observation
    Ledger source ref so finalizer/reviewer can preserve history narrative
    context instead of seeing only a raw commit id.

- Batch 35 — extractor stage boundary. Status: completed.
  - Ensure extractor completion is based on accepted structured emits or
    explicit soft-stop policy, not on unknown/unavailable tool calls.
  - Keep extractor prompts aligned with its actual tool set and existing
    Observation Ledger handoff; do not introduce another evidence surface.
  - Delivered in this batch: unavailable tools now return an explicit failed
    `ToolResult` instead of disappearing from the ReAct transcript, and the
    extractor mid-loop controller treats stage-outside tool calls as a
    repairable protocol issue unless a valid structured extractor emit already
    succeeded. This protects the stage boundary without turning answer content
    into a hard gate.

- Batch 36 — scalar/negative/external handoff closure. Status: completed.
  - Generalize existing count/scalar aggregate handoff without turning narrative
    answers into scalar-only answers.
  - Keep negative observations typed by origin and bounded scope, including git,
    command, log, trace, and future MCP/web payloads.
  - Delivered in this batch: structured origin projection now recognizes
    known tool/source tokens carried in `source_ref`, `tool_result`, and
    `producer` dimensions (including bracketed tool-call ids such as
    `git_log[0]`) without reading raw prose. Principal typed no-hit facts
    (`negative_search` / `negative_observation`) that the finalizer omits are
    materialized as an append-only localized supplement block; repository
    no-hit facts also add a bounded `scope=negative` citation, while non-repo
    observations remain citation-free origin-specific evidence. This avoids
    finalizer rewrites for missing zero-result proof text while preserving the
    model-authored answer above it.

- Batch 37 — language-aware source inventory/export semantics. Status:
  completed.
  - Reuse repomap language metadata where available instead of Go-only
    heuristics.
  - Source inventory supplements remain separate localized system notes and do
    not replace model tables or prose.
  - Current sub-tasks:
    1. Preserve explorer-authored `member_notes[]` when source-inventory
       reconciliation replaces a stale/incomplete aggregate member set with the
       typed graph/parser-complete set. Same-member summaries must be merged by
       stable member identity, not overwritten by graph/doc comments.
    2. Keep the Go parser path limited to the explicit `type_underlying=string`
       + `requires_const_set` type-inventory qualifier. All other languages and
       general source inventories must use repomap graph metadata (`language`,
       `kind`, `exported`, `file:line`) so the feature benefits every supported
       language.
    3. Add regression coverage proving mixed-language source inventories keep
       exported symbols, exclude private/internal symbols when requested, and
       carry rich notes downstream without replacing model-authored final-answer
       tables.
  - Delivered in this batch: `sourceInventoryReplaceMemberSet` now merges
    existing same-member `member_notes[]` ahead of graph/parser candidate notes
    while retaining graph-authoritative `file:line` support refs. Dropped
    private/out-of-scope members do not leak their notes onto retained members.
    Regression coverage pins mixed Go/Java/TypeScript source inventories so
    exported cross-language symbols are kept, private symbols are excluded under
    `public_exported`, and explorer-authored Chinese summaries survive the
    replacement path.

- Batch 38 — runtime artifact origin normalization. Status: completed.
  - Align git/log/trace/command observation refs, excerpts, and line/span labels
    so finalizer/reviewer consume the same external-evidence contract.
  - Preserve pagination through existing blob/row-set mechanisms.
  - Current sub-tasks:
    1. Project typed aggregate-fact source coordinates (`tool_result`,
       `payload_ref`, `row_set_ref`, `page_ref`, `line_start`, `line_end`,
       `row`, trace timestamps, and related selectors) into the same
       `ObservationSourceRef + ObservationSpan` contract already used by
       concrete tool results.
    2. Keep git/log/trace/command/MCP/web coordinates origin-specific; external
       artifact lines must not become current-source citations.
    3. Add regression coverage for mixed external observations that combine
       existence/no-hit facts with addressable artifact/command spans.
  - Delivered in this batch: aggregate facts now project typed external
    coordinates into the same ledger contract as concrete tool results.
    `tool_result` / `tool_call_id`, command text, payload/page/row-set refs,
    runtime artifact ids/kinds, artifact paths, current line ranges, VCS hunk
    lines, table rows, selectors, JSON pointers, and trace timestamps are
    preserved as `ObservationSourceRef + ObservationSpan`. External log/trace
    lines remain runtime-artifact spans and do not qualify as current-source
    citations. Regression coverage pins git no-hit observations, attached-log
    line ranges, and command-output row coordinates.

- Batch 39 — system supplement and caveat authority cleanup. Status: completed.
  - System supplements must be clearly localized and append-only.
  - Non-critical, non-lossless issues should become bounded notes rather than
    finalizer rewrites.
  - Delivered in this batch: aggregate member-set carrier supplements now
    follow the answer language and mark themselves as system-verified
    supplements in the title and body. Chinese answers render
    `系统按已验证证据补充成员...`; English answers render
    `System-verified member supplement...`. The runtime still appends these as
    separate blocks only after typed hard handoff checks decide a complete
    principal member set is missing from the visible answer; it does not replace
    or edit model-authored prose/tables.

- Batch 40 — performance and convergence guards. Status: completed.
  - Analyzer fast paths and parallel exploration convergence should reduce wait
    time without changing user intent.
  - Add regression guards where a future developer could accidentally re-open
    hard gates from noisy evidence.
  - Delivered in this batch: the stream first-byte watchdog default is now
    pinned by regression coverage to stay at a slow-model-safe lower bound
    (`>=40s`) and below the stall timeout. This guards the customer-visible
    failure mode where an overly aggressive first-byte timeout is rendered as a
    model-response retry even though the upstream model may simply be slow to
    produce the first usable SSE event.

## 2026-05-23 Cross-GAP Architecture Scan

This scan reconciles the active JSON / evidence handoff work with
`eval_20260522_full_sweep_gap_tracking.md`,
`observation_ledger_contract_20260521.md`,
`unified_evidence_answer_contract_20260520.md`, and
`systemic_evidence_handoff_gap_tracking.md`. The remaining work is not a set of
isolated case fixes. The recurring pattern is that typed evidence carriers,
visible answer supplements, retry telemetry, and stage boundaries still have a
few split-brain paths. The solution should keep moving deterministic carrier
work into runtime code, while leaving the model-owned answer prose and tables
untouched.

### Closed / Mitigated Clusters

- JSON-string arrays/objects for the highest-frequency structured tools now go
  through the shared compatibility boundary. Lossless repair is typed and
  telemetry-backed rather than prompt-only.
- Large complete member carriers and exact exclusion carriers can use row-set
  artifacts. Rich same-member summaries are merged by stable identity before
  finalizer/reviewer budgeting.
- Source-inventory path spelling is normalized once before graph matching,
  including `./` and absolute-path suffix forms. Go string-enum inventories use
  parser-backed proof only for the explicit string-enum contract; other
  language inventories rely on repomap graph metadata.
- Pure VCS/history answers no longer force `current_source`; recent commits,
  latest merge, and commit comparisons preserve VCS narrative origins and typed
  git coordinates.
- Transport first-byte timeout has a slow-model-safe lower-bound test, and
  transport retries have started separating from semantic rewrite status.
- Extractor stage boundary is fail-closed for unavailable tools: unknown tool
  calls are visible failed `ToolResult`s, and extractor completion requires
  accepted structured emits or an explicit safe stop.

### Still-Open Architecture Clusters

1. **Visible system supplements still have too much authority.** Full-sweep
   gaps G1/G2/G4/G13/G18/G153 show that system-generated tables/caveats can
   appear principal even when the model-authored answer is already sufficient.
   This must become a visible-surface contract: system supplements are
   append-only, localized, non-principal by default, unique by requested bucket
   and member identity, and suppressed when they add no typed non-overlapping
   value.

2. **External/VCS/runtime evidence still sometimes rides source-shaped
   contracts.** G9/G14/G151/G156/G157/G159 show runtime artifact facts, VCS
   negatives, and artifact line anchors still leaking through `current_source`
   lanes or `citation_ref=-1` sentinel semantics. The system needs one
   external-observation citation/ref contract for git, logs, traces, command
   output, and future MCP/web payloads.

3. **Negative/scalar answers can still be coerced into enumeration contracts.**
   G157/G158/G159 show zero-result VCS/search observations being treated as
   member inventories. The typed principal answer for a no-hit question is
   `absence + scope + result_count=0 + provenance`; searched-window inventory
   is optional and must not be synthesized unless requested.

4. **Analyzer/explorer still spend model rounds on classification/protocol
   work.** G50/G149/G154/G171/G173 show analyzer over-investigating exact-file,
   history+current-code, exact-symbol conditional, and error-granularity
   questions. G152/G160/G176 show exploration lanes duplicating hybrid
   VCS/current-source work or waiting on uninstrumented repo-map work.

5. **Runtime artifact provenance still needs direct-vs-inferred separation.**
   G8/G12/G15/G17/G162 show direct log/trace observations, inferred upstream
   hypotheses, and current-code causal explanations can be compressed into one
   prose claim. Direct observed facts must stay distinct from inferred
   possibilities unless current-source evidence proves the bridge.

6. **Reviewer/status telemetry still conflates advisory, schema, transport, and
   semantic failures.** G5/G76/G155/G161 show metrics/status vocabulary is not
   yet authoritative enough for customers or eval sweeps. Accepted advisory
   findings should not look like finalizer reject pressure, and transport
   retries should not count as answer rewrites.

### Batch 41+ Task Queue

- Batch 41 — visible supplement authority contract. Status: completed.
  - Design principle: model-authored prose, markdown tables, structured tables,
    diagrams, and caveats are the primary answer surface. Runtime may append
    localized supplements only when a typed, mechanically verified carrier proves
    a non-overlapping visible gap. Runtime must not rewrite, delete, compress, or
    silently replace model-authored answer content.
  - Code entry points audited for this batch:
    `normalizePrincipalEnumerationRowBlocks`,
    `normalizeAggregateMemberSetCarriers`,
    `normalizeCurrentSourceCitationSupplement`,
    `normalizeAggregateNegativeProofSupplement`,
    `materializeRequiredCaveatWhenOnlyMissing`,
    `AppendSoftContractCaveatsToAnswerForBus`, and renderer-side preserved
    attachment output.
  - Batch 41.1 task: make enumeration supplements strictly local. If a
    model-authored table/list already names the member, runtime may append only
    the missing verified fields/rows in a clearly labeled supplement. It must not
    publish a competing "complete system table" just because the model's table
    layout is incompatible with the deterministic row compiler.
  - Batch 41.2 task: tighten current-source anchor supplements. They should fire
    only when the answer explicitly needs current-source support and the model
    surface dropped the citation pool; they should not become a generic
    "more anchors" appendix for an already bounded model answer.
  - Batch 41.3 task: route accepted-path soft caveats through one suppressor.
    Generic coverage / metadata / richness caveats remain telemetry unless the
    system can name a precise user-visible missing facet. Observation-only,
    history-only, mechanism, scalar/count, and already bounded answers should not
    receive broad "coverage may be insufficient" notes.
  - Batch 41.4 task: add a guardrail test family covering Chinese and English
    supplements, model-authored markdown table preservation, structured-table
    preservation, source-anchor supplement gating, and generic caveat
    suppression. Tests must use typed request/evidence signals only; no hard gate
    may inspect user prose or model free-form text except for visible-coverage
    detection.
  - Delivered so far: corrupt or noisy model-authored markdown tables no longer
    trigger a second full system-generated member table. The compiler now
    preserves the model table and appends only the deterministic missing rows;
    duplicate or unexpected model rows stay visible as model-authored content
    but are not copied into the system supplement.
  - Delivered in Batch 41.1: incompatible model-authored structured tables no
    longer produce a competing "complete system table." The runtime keeps the
    model table untouched and, when deterministic fields cannot be filled in
    place, appends a localized `System-verified field supplement` /
    `系统按已验证证据补充可校验字段` block. This keeps the supplemental nature
    explicit while preserving model-authored table layout.
  - Delivered in Batch 41.2: current-source anchor supplements now require
    either visible evidence mention or an out-of-range citation carrier that
    proves the model dropped the citation pool. High-confidence required-file
    hints alone no longer create a generic "more source anchors" appendix.
  - Delivered in Batch 41.3: accepted-path soft caveats now suppress generic
    citation/acceptance leftovers for ordinary mechanism narratives and pure
    VCS/history narratives once the answer is otherwise accepted. Precise
    surfaces still keep disclosure: scalar/count/key-value/config, exact
    absence, true enumeration/member-set, trace/call-chain/conditional/
    registration requests continue to surface grounding boundaries when the
    typed contract requires them.
  - Delivered in Batch 41.4: guardrail coverage now pins the bilingual visible
    supplement contract. The tests cover preserved model-authored markdown
    tables, preserved incompatible structured tables, localized
    `field supplement` titles, source-anchor supplement gating, and accepted
    mechanism/history generic-caveat suppression. These guards intentionally use
    typed request/evidence fields rather than user prose or model free-form
    keyword checks.

- Batch 42 — external observation refs and no-hit answer shape. Status:
  in progress.
  - Introduce a first-class visible/ref contract for VCS/log/trace/command
    observations so finalizer/reviewer stop relying on no-source citation
    sentinels.
  - Materialize typed no-hit answers from `result_count=0` producer outputs
    without requiring the model to enumerate searched commits/files/rows unless
    the user asked for that inventory.
  - Guardrail tests: VCS negative search renders `未命中 + 范围 + 0 结果 +
    工具/时间/窗口`; optional model-authored tables are preserved but not
    required; fake current-source citations are rejected or normalized away.
  - Batch 42.1 task: localize system-generated no-hit provenance surfaces. The
    supplement may render typed origins such as VCS metadata/diff, runtime
    artifacts, command output, web/MCP/connector resources, and cross-repo
    indexes, but it must show user-facing labels rather than enum names like
    `vcs_metadata`. This is a display-only normalization of system-authored
    supplement text; it must not rewrite model-authored prose and must not
    create repo citations for external observations.
  - Batch 42.2 task: collapse optional searched-window inventories. If the
    principal typed fact is `result_count=0`, finalizer should not be forced to
    enumerate every searched commit/file/row unless the user requested that
    inventory. Optional model-authored tables remain visible, but exact
    mismatch handling should prefer preserving the clean no-hit answer plus a
    localized scope supplement over generic enumeration caveats.
    - Detailed design:
      - Principal answer contract: a typed `negative_search` /
        `negative_observation` with `result_count=0` is the answer-grade fact.
        The searched commits, files, trace spans, log windows, command rows, or
        connector pages that produced that zero result are provenance/support
        unless the request model carries an explicit enumeration/member-set
        obligation.
      - Reuse existing role projection instead of adding a second filter:
        `NormalizeAggregateFactRolesForRequest` demotes support ledgers before
        prompt/render surfaces, and
        `PrincipalAggregateMemberSetFactRefsForRequest` is the guardrail for
        callers that project principal member sets directly.
      - Typed-only trigger: demotion may look at aggregate kind/role, numeric
        zero-result fields (`value=0`, `result_count=0`), structured coordinates
        (`window_count`, `unmatched`, `commit_range`, `tool_result`,
        `payload_ref`, `row_set_ref`, artifact span refs), request-model traits,
        and evidence-origin enums. It must not inspect user prose, model prose,
        table titles, or repository-language keywords.
      - Preservation rule: if the model authored an optional table/list about
        the searched window, the system must not delete or rewrite it. The
        demotion only prevents deterministic补表 / hard principal-member gates
        from treating that window inventory as the required final answer.
      - Explicit inventory exception: enumeration, relation lookup, source/path
        inventory, or other typed exhaustive-member-set requests keep the
        member set principal even when a zero-result fact is also present.
      - First code batch (42.2a): demote searched-window member sets paired with
        a principal zero-result external/repo observation and add tests for VCS
        history no-hit, command/log no-hit support ledgers, and explicit
        enumeration preserving the window set.
      - Follow-up batch (42.2b): route accepted no-hit exact-mismatch issues to
        localized scope supplements instead of generic enumeration caveats when
        the mismatch only concerns support-window cardinality.
      - Eval batch (42.2c): add varied cases for "recent N commits did/did not
        touch X", "search logs/traces for pattern and explain no-hit", and
        "list searched windows plus explain absence" so the exception boundary
        is covered.
  - Batch 42.3 task: replace remaining sentinel-based external provenance in
    VCS/log/trace/command finalizer payloads with typed observation refs. This
    should reuse `ObservationLedger`, `payload_ref`, `row_set_ref`, and
    artifact-local coordinates instead of inventing a parallel citation system.
    - Detailed design:
      - Short-term compatibility remains: `items[].citation_ref=-1` is still a
        structural uncited carrier accepted from legacy finalizer payloads, but
        it must never be taught as user-visible provenance and must never be the
        only way external observations reach reviewer/finalizer context.
      - First code batch (42.3a): extend the visible sentinel sanitizer from
        runtime-only answers to typed external-observation-only answers. The
        typed gate is `ObservationLedger` / `AnswerIntentContract`: it must see
        at least one non-current-source origin (VCS metadata/diff, runtime
        artifact, command output, web/MCP/connector/cross-repo) and no requested
        current-source origin. It must not inspect user prose or model prose.
        Mixed "external info + current code" answers are explicitly excluded so
        literal discussion of Codrax internals or citation mechanics stays
        untouched.
      - Delivered batch (42.3b): expose observation IDs / source refs in the
        finalizer prompt lane where VCS/log/trace/command rows previously
        depended on uncited item carriers. The shipped entry point is
        `answer_document_evaluator.go::renderAnswerDocObservationLedger`, fed by
        `ObservationLedgerInputFromAgentContext`,
        `CompileObservationLedger`, `PrioritizeObservationRecords`, and
        `FormatObservationSourceRef`. It renders current source, VCS/diff,
        command, runtime-artifact, MCP/resource, scalar, negative, and row-set
        records in one typed ledger, including `payload_ref`, `row_set_ref`,
        artifact-local spans, bounded raw excerpts, rich notes, result counts,
        and support-ref counts. It does not add a second citation pool and does
        not teach the legacy no-current-source citation sentinel.
      - Delivered batch (42.3c): converge renderer/reviewer/presentation consumers on
        observation refs over pseudo citations for changed-path/stat rows
        (`line=0` VCS paths), artifact-local log lines, trace spans, command
        rows, and future connector/web rows.
        - Code entry points audited:
          `semantic_quality_reviewer.go::semanticObservationSummaries` already
          uses the same ledger/source-ref contract as finalizer; answer-document
          pre-emit still owns current-source citation integrity; render-side
          answer display owns user-facing citation suffixes and preserved
          attachments. The implementation must reuse those paths instead of
          adding another formatter.
        - Contract: `citations[]` remains only for current-repository
          `file:line` evidence with `line > 0`. External observations must stay
          on `ObservationSourceRef`/`ObservationSpan` (`payload_ref`,
          `row_set_ref`, `artifact_id`, artifact-local line/span, command,
          VCS ref/path/hunk coordinates). If a model-authored answer mentions an
          external observation without a repo citation, the system may append a
          localized boundary note, but must not turn it into a fake
          `repo:0`/`file:0` citation or replace the model's prose/table.
        - Delivered guardrails: pre-emit normalization detaches only
          non-positive line-shaped citation carriers when a typed external
          observation ledger/contract is present; explicit `ScopeFile` /
          `ScopeSection` and real current-source `file:line` citations remain
          intact. Reviewer input keeps line-zero VCS/stat/path rows as
          observation refs without rendering `line 0`, and render tests pin that
          the markdown renderer never prints a `:0` suffix.
  - Delivered so far: no-hit supplements now preserve typed scope coordinates
    such as `window_count`, `unmatched`, `order`, `window_path`, `diff_path`,
    `tool_result`, `payload_ref`, and `row_set_ref` in the localized
    supplement text. This keeps VCS/log/trace/command zero-result answers as
    absence proofs instead of nudging the finalizer toward enumerating searched
    windows.
  - Delivered so far: model-facing answer-document schema and mid-loop repair
    hints no longer teach the internal no-citation sentinel value. Runtime
    compatibility still accepts legacy no-source citation carriers, but prompts
    now instruct the model to omit the citation field / leave the row uncited
    when a fact is backed by an external observation rather than current repo
    source.
  - Delivered so far: criterion/gate waiver details also use the typed
    `external_observation` carrier vocabulary instead of spelling the legacy
    no-citation sentinel, so advisory/debug detail no longer reintroduces the
    same prompt leak through a different stage.
  - Delivered in Batch 42.1: system-generated no-hit supplements now localize
    typed external origins (`版本历史` / `VCS history`, `版本差异` / `VCS diff`,
    runtime artifacts, command output, web/MCP/connector resources, etc.) and
    tests assert that enum values such as `vcs_metadata` do not leak into the
    visible supplement. External negative observations still carry
    `external_observation` claim uses and do not create current-source
    citations.
  - Delivered in Batch 42.2a: request-aware aggregate role projection now
    demotes searched-window member sets paired with a principal
    `result_count=0` negative search/observation into `supporting_coverage`.
    This uses typed dimensions (`commit_range`, `window_count`, `unmatched`,
    `tool_result`, `payload_ref`, `row_set_ref`, artifact/log/trace windows)
    and explicit evidence origins only. The principal member-set projection and
    deterministic enumeration row compiler both skip those support ledgers, so
    no-hit answers are no longer forced into searched-commit/log-row tables.
    Explicit enumeration/relation/source-inventory obligations still preserve
    the searched-window member set as principal.
  - Delivered in Batch 42.2b: finalizer prompt wiring now has a regression
    guard for the same projection. The `Required Principal Member Set` section
    is omitted for no-hit searched-window support ledgers, preventing the
    prompt from reintroducing the hard verbatim-member obligation after the
    typed role projection has demoted the ledger. Scalar-count support and
    empty/no-fact prompts remain covered by the same prompt tests.
  - Delivered in Batch 42.2c: eval coverage now includes
    `u7p` for "recent N commits, zero-hit scope, do not enumerate each
    searched commit" and the priority sweep includes VCS no-hit, mixed
    hit/no-hit, log no-fatal, and HiTrace no-long-GC negative-observation
    cases. These cases track the cross-origin contract instead of only the
    single original VCS marker scenario.
  - Delivered in Batch 42.3a: visible `citation_ref=-1` sanitizer now covers
    typed external-observation-only answers, not just runtime artifacts. The
    gate is compiled from `AnswerIntentContract` plus `ObservationLedger`: it
    requires at least one non-current-source origin and rejects any actual
    current-source ledger record/current-source diagnostic requirement. Mixed
    "history/log/trace + current source" answers and Codrax-internal questions
    keep literal text untouched.
  - Delivered in Batch 42.3b: finalizer prompt wiring now receives the typed
    Observation Ledger directly. VCS metadata/diff, command output, runtime
    artifacts, MCP/resource observations, row-set artifacts, and current-source
    evidence are all shown with origin/policy/source/span boundaries plus rich
    notes/excerpts where available. Tests pin VCS narrative preservation,
    mixed diff+current-source separation, MCP resource handling, typed
    payload/row-set refs, external raw excerpts, and large row-set artifact
    creation.
  - Delivered in Batch 42.3c: invalid external-observation pseudo citations are
    normalized away before final structural checks. The runtime only detaches
    citations with non-positive line-shaped scopes (`empty`, `line`,
    `line_range`, or empty-section) and only when a typed non-current-source
    observation is present. It preserves model-authored prose/tables, legitimate
    `ScopeFile`/section citations, and mixed answers' real current-source
    citations. Reviewer and renderer guards ensure VCS path/stat rows and
    artifact-local rows stay observation refs rather than becoming
    current-source `file:0` citations.

- Batch 43 — runtime artifact provenance split. Status: completed.
  - Design decision: reuse `ObservationLedger` as the only downstream evidence
    surface. Do not add a second runtime-artifact prompt section or a new
    pseudo-citation channel.
  - Add a typed observation provenance lane on `ObservationRecord` with the
    narrow values below:
    - `observed_direct_cause`: the artifact itself reports the failure/symptom
      or causally-labelled runtime event, such as a parsed `LogError`, a
      diagnostic failure observation, or a trace jank/stall record with an
      explicit runtime reason/symbol.
    - `artifact_span`: the record is an addressable artifact-local coordinate
      or measurement, such as a log line, trace timestamp span, command row, or
      frame duration. It is answer-grade for artifact questions but must not
      become a current-source root-cause proof.
    - `inferred_upstream_possibility`: the record is a bounded possible upstream
      explanation or bridge to current source. It is useful context, but it is
      never enough by itself to hard-block or rewrite the final answer.
  - Producer mapping:
    - `LogBundle.Errors` -> `observed_direct_cause`, with stack-frame support
      refs treated as artifact-local support unless separately grounded by
      current-source evidence.
    - `LogBundle.Observations` -> `observed_direct_cause` only when typed as
      diagnostic/failure/warning; otherwise `artifact_span`.
    - `PerfBundle.Frames` -> `artifact_span`; do not render internal zero-based
      frame counters when `FrameNo` is absent/zero.
    - `PerfBundle.Janks` / `PerfBundle.Stalls` -> `observed_direct_cause` when
      the typed runtime reason/symbol exists, otherwise `artifact_span`.
    - `aggregate_facts` may opt in with structured dimensions
      `observation_lane`, `runtime_lane`, or `provenance_lane`; invalid values
      are ignored rather than guessed.
  - Consumer mapping:
    - Finalizer prompt and semantic reviewer render the lane alongside
      `origin/source/span`, so both agents can distinguish direct artifact facts
      from possible upstream interpretations without reading raw prose.
    - Pre-emit current-source citation gates continue to use
      `ObservationRecordHasCurrentSourceLineSpan` and must not treat runtime
      lanes as citation eligibility.
  - Guardrail tests:
    - Trace/perf prompt rows do not expose `frame 0` / zero-based frame wording
      unless `FrameNo > 0`.
    - Log answers see `observed_direct_cause` for the parsed error and an
      explicit boundary note that stack support refs are artifact-local unless
      current-source evidence grounds them.
    - Aggregate facts preserve a typed `inferred_upstream_possibility` lane via
      structured dimensions without changing model-authored answer text.
  - Delivered in this batch: `ObservationRecord` now carries a narrow
    `provenance_lane` enum, populated by accepted runtime producers and by
    structured aggregate dimensions only. Finalizer and semantic reviewer render
    the lane next to `origin/source/span`, while current-source citation
    eligibility remains solely controlled by exact current-source span helpers.
    `PerfBundle.Frames` with absent/zero `FrameNo` now render as a generic
    `frame sample` with timestamp span instead of leaking an internal
    zero-based `frame 0`; explicit artifact ordinals still render normally.
    Log-error records carry an artifact-local stack boundary note so the model
    can use stack frames as runtime support without promoting them to
    current-source root-cause proof.

- Batch 44 — analyzer fast-path consolidation. Status: in progress.
  - Add typed fast paths for exact-file import literal enumeration,
    history+current-code hybrid classification, exact-symbol conditional
    questions, and item-vs-batch error-granularity questions. These must use
    typed request-shape/provenance fields, not user-prose keyword hard gates.
  - Guardrail tests: analyzer emits classification after existence checks and
    leaves implementation proof to exploration.
  - First implementation slice (44.1): marker/decorator/annotation inventory
    coherence. Status: completed.
    - Root gap: `subtopic_coherence` still treats marker-token primary entities
      such as ArkTS/TS/Python decorators or Java/Kotlin annotations as if every
      valid sub-topic must repeat those exact markers. In marker inventory
      questions, valid sub-topics are often discovered file/function buckets
      that will be verified by exploration.
    - Typed signal only: the carve-out may read
      `Predicates.IsCategoryEnumeration`, `AnalyzerHints.PrimaryEntities`,
      `AnalyzerHints.Entities`, `AnalyzerHints.RequiredFileHints`,
      `SubTopics.Entities`, and `SubTopics.Scopes`. It must not parse the raw
      user request or model prose for localized words like "decorator".
    - Advisory scope: when at least one primary/entity token is marker-shaped
      (for example starts with `@` and carries an identifier) and sub-topic
      anchors are path/file/bucket-like surfaces within required-file or
      sub-topic scopes, R1.3/R1.5/R1.4 should become advisory. Exploration still
      decides whether each file bucket actually contains the marker.
    - Hard scope preserved: ordinary category enumerations over symbol members,
      exact scalar lookups, relation lookups, diagnostics, and unresolved
      invented symbol members remain hard where they are hard today.
    - Regression coverage: ArkTS-style `@Entry` / `@Builder` primary markers
      with `.ets` sub-topic files pass as advisory even with mixed resolver
      hit/miss; an invented enum member still fails in a normal
      category-enumeration question.
    - Delivered in this slice: `subtopic_coherence` now recognizes the typed
      marker-inventory shape from analyzer fields only. R1.3/R1.5/R1.4 become
      advisory for marker-shaped primary entities with file/path bucket
      sub-topics, while non-file invented members remain hard failures.
  - Remaining implementation slices:
    - Slice 44.2 — non-diagnostic error-granularity route normalization.
      Status: completed.
      - Root gaps: E20260520-G118 / E20260522 u9a-u9b show analyzer retries
        where a code-behavior question about item-vs-batch rejection is first
        routed as `root_cause`, then rejected and reclassified as `explain`.
      - Typed signal only: consume `error_granularity_profile`,
        `diagnostic_profile`, `predicates.is_diagnostic_question`,
        `intent`, `scenario`, and artifact/current-status typed flags. Do not
        parse the user request or model text for words such as "error",
        "reject", or localized equivalents.
      - Normalization rule: when `is_granularity_question=true` and no typed
        diagnostic/current-risk/current-version/runtime-artifact signal is
        present, the answer shape is code-behavior explanation. Normalize
        `intent=root_cause` / diagnostic scenario drift to the explain route
        before analyzer quality gates. True log/trace/current-regression
        diagnostics keep their diagnostic/root-cause route.
      - Guardrail tests: direct item-vs-batch behavior questions normalize to
        explain without a second analyzer dispatch; real diagnostic/runtime
        error-granularity questions are not downgraded.
      - Delivered in this slice: analyzer reconciliation now keeps active
        `error_granularity_profile` as a verdict requirement rather than a
        diagnostic signal. If the route drifted into `root_cause` with no typed
        diagnostic/current-risk/current-version/runtime-artifact signal, it is
        normalized back to `explain`/`generic` and profile-only diagnostic mirror
        drift is cleared. Diagnostic predicates, current-risk/current-version
        diagnostics, and attached runtime artifacts are explicitly guarded.
    - Slice 44.3 — exact role-locate and scalar symbol scope preservation.
      Status: completed.
      - Root gaps: E20260520-G5 / G88 / G90 / G127 show role-locate questions
        either missing an inferable `answer_subject.kind` or being widened into
        related architecture sub-topics and function-body coverage.
      - Typed signal only: consume `predicates.is_role_locate_lookup`,
        `answer_subject`, `answer_role_profile`, `question_kind`,
        explicit `sub_topics`, and resolved required-file/symbol hints. Do not
        infer new user asks from nearby repo terms.
      - Normalization rule: if the role-locate target kind is structurally
        inferable, fill the missing typed subject field locally; if a scalar
        role-locate has exactly one requested target, demote unrelated
        relationship/call-chain subtopics to support hints rather than
        principal subquestions.
      - Guardrail tests: exact "entry function + file" answers keep one
        principal target; coverage/existence questions that merely mention a
        target are not forced into role-locate.
      - Delivered in this slice: `emit_analysis` now safely fills missing
        role-locate `answer_subject.kind` only from typed `question_kind` /
        `predicate_axis` combinations that identify the located literal
        (`return_value`, `call_chain`, `config_mapping`, return/call/configure
        axes). Ambiguous role-locate subjects remain fail-loud. The analyzer IR
        builder also collapses exploratory `sub_topics` for scalar role-locate
        lookups so related files/components remain search context instead of
        principal answer sections; set-valued and relational role lookups keep
        their topics.
    - Slice 44.4 — explicit-file literal/import/source-inventory analyzer stop
      condition. Status: completed.
      - Root gaps: E20260522-G50 / G57 and E20260520 import/inventory cases
        show analyzer spending rounds on content searches after file or package
        existence has already been established.
      - Typed signal only: consume `required_files`, source-inventory traits,
        literal/import/reference bucket fields, and tool results that prove
        file/package existence. Analyzer must not attempt line-level proof.
      - Normalization rule: once explicit file/package scope and requested
        bucket family are typed, emit analysis and leave line/member extraction
        to exploration. Analyzer may carry search results as soft hints only.
      - Guardrail tests: import literal enumeration and explicit source
        inventory classify after existence proof; broad category enumeration
        without explicit scope still follows the normal analyzer route.
      - Delivered in this slice: analyzer mid-loop observation now emits a
        soft `analyzer.path-scoped-prescan-ready` hint after a successful
        path-scoped `grep(files_only=true)` result with matching files. The
        hint asks the analyzer to emit classification and leave line/member
        proof to explore; it is not a hard stop. Guardrails skip repo-wide
        paths, failed greps, non-`files_only` searches, line-match greps,
        up-directory paths, and non-grep tools so broad discovery and normal
        analyzer routes remain untouched.
    - Slice 44.5 — exact-symbol conditional / premise-invalid boundary.
      Status: completed.
      - Root gaps: E20260520-G84/G85 and related change-impact cases show
        stale analyzer assumptions or same-family matches overriding later
        exact absence/premise-invalid evidence.
      - Typed signal only: consume exact target bindings, exact-resolution /
        negative-observation facts, alias bindings, and support refs. Same
        family or regex-nearby evidence remains support unless an alias is
        typed.
      - Normalization rule: exact absent/premise-invalid facts for the target
        dominate stale positive labels for that target, but must not suppress
        independent present targets in mixed positive/negative answers.
      - Guardrail tests: exact target absence leads the final answer; same-family
        symbols do not become principal affected members; mixed present+absent
        answers preserve both target-bound claims.
      - Detailed design before code:
        1. Reuse existing `ChangeImpactProfile`, `AnswerSurfacePlan.StableAbsent`,
           `ExactResolution*`, and typed support-lane compilers. Do not create a
           second absence detector and do not scan user/model prose for target
           words.
        2. Extend the existing change-impact principal evidence filter so
           simple exact targets (for example `ShapeValue`, not only
           owner-qualified `CitationReq.Required`) must be structurally present
           in evidence fields (`anchor_symbol`, `owner_symbol`, `subject`,
           `object`, `condition`, `snippet`, `surface_terms`). Same-family
           symbols remain support context unless an explicit alias/proof lane
           names them.
        3. When `AnswerSurfacePlan.StableAbsent` is true, aggregate
           change-impact member_set rows are not allowed to resurrect stale or
           same-family positive members. A system/materialized aggregate row may
           stay principal only when its support evidence at the cited location
           structurally names the requested target. Otherwise it stays out of
           the principal lane and the finalizer should lead with the absence /
           premise-invalid correction.
        4. Preserve non-absence change-impact behavior: existing aggregate
           member_set handoffs and heterogeneous affected-site roles remain
           principal when the target is present or no stable absence was
           accepted. This avoids suppressing legitimate broad impact answers.
      - Delivered in this slice: change-impact principal evidence now requires
        simple exact targets to appear structurally in answer-grade evidence,
        closing the gap where same-family symbols could become principal merely
        because the target was not owner-qualified. Stable-absence
        change-impact aggregate rows additionally require support evidence at
        the cited location that structurally names the requested target, so a
        premise-invalid / absent-target investigation cannot be overwritten by
        stale aggregate rows or sibling symbols. Existing non-absence aggregate
        member handoff and owner-qualified heterogeneous affected-site coverage
        remain covered by regression tests.

- Batch 45 — hybrid explorer partitioning and repo-map wait visibility. Status:
  in progress.
  - Partition hybrid questions by typed origin/facet: VCS lane owns history
    narrative; current-source lane owns present implementation; sibling lanes
    converge once their facet is covered.
  - Instrument repo-map concurrent build/cache waits and show truthful progress
    including lock waits.
  - Guardrail tests: mixed history+current-source cases avoid duplicated
    searches/reads; two concurrent repo-map builds report wait state instead of
    silent stalls.
  - Slice 45.1 — coalesced repo-map wait visibility. Status: completed.
    - Detailed design before code: reuse the existing typed
      `RepoMapScanEvent` / orchestrator notice channel and the multigraph
      same-slug in-flight table. Do not add a second renderer path or a model
      prompt signal. Emit a wait-phase event only for callers that join an
      already-running build; the original build still owns parse/build/rank/cache
      progress and completion events.
    - Delivered in this slice: added `RepoMapScanPhaseWait`, localized
      Chinese/English wait wording, and a multigraph `WaitNotifier` hooked to
      the existing repomap scan notifier. Concurrent `EnsureLoaded` calls for
      the same slug now surface "waiting for the in-progress index build" with
      sub-repo label and file count instead of looking like a frozen request.
      Regression tests cover both the multigraph wait event and the rendered
      localized messages.
  - Slice 45.2 — mixed-origin lane plan in upstream prompts. Status:
    completed.
    - Detailed design before code: reuse the existing
      `CompileAnswerIntentContract` typed origin/output contract and the single
      upstream `Evidence Origin Boundary` prompt section. Do not create a second
      source-mix classifier, do not inspect raw user prose/model prose, and do
      not add a new hard gate.
    - For contracts that contain `current_source` plus at least one
      non-current-source origin (VCS metadata/diff, runtime artifact, command
      measurement, repo negative search, external/MCP/connector observations),
      render an explicit lane plan:
        1. non-current-source producer tools prove historical/external observed
           facts and hand them off through `reason` / `aggregate_facts` with
           typed origin dimensions;
        2. `current_source` reads/`emit_evidence` prove only present-checkout
           implementation claims;
        3. when both lanes touch the same target, preserve both summaries rather
           than turning one lane into the other's fake `file:line` citation or
           re-running duplicate searches.
    - Pure VCS/history, pure runtime/log, pure current-source, and finalizer
      prompts remain on their existing paths: pure VCS still tells explorer not
      to call `read_file` just to satisfy source habits; finalizer still receives
      the unified contract from `answer_document_evaluator` so the boundary is
      not duplicated.
    - Guardrail tests: mixed history+current-source prompts include lane-plan
      language and keep both origins/output shapes; pure history remains free of
      current-source obligations; finalizer still does not receive a duplicate
      builder-side Evidence Origin Boundary.
    - Delivered in this slice: the upstream Evidence Origin Boundary now emits
      a mixed-origin lane plan only when the typed contract contains both
      `current_source` and a non-current-source origin. The plan tells explorer
      to collect VCS/log/trace/command observations with producer tools, hand
      them off through typed `reason` / `aggregate_facts`, and read current
      source only for present-checkout claims. Pure VCS/history prompts and
      finalizer prompts remain on their prior non-duplicated paths. Regression
      tests cover mixed-origin, pure-history, and finalizer-no-duplicate cases.
  - Slice 45.3 — mixed-origin parallel convergence guard. Status: completed.
    - Detailed design before code: reuse `CompileAnswerIntentContract` and the
      existing `parallelExploreAllowsEarlyConvergence` /
      `parallelExploreMustWaitForSiblingHandoffs` decision point. Do not add a
      new dispatcher, do not classify by user-prose keywords such as "current"
      or "diff", and do not inspect model free text.
    - Root gap: after Slice 45.2 the prompt tells explorers to keep VCS/log/trace
      and current-source lanes separate, but the parallel dispatcher can still
      cancel a slower current-source sibling when a VCS/history fork reaches
      `emit_investigation_complete` first. That loses the present-checkout
      explanation lane in mixed "history/diff + current code" questions.
    - Rule: if the typed answer-intent contract contains `current_source` plus at
      least one non-current-source origin, and the requested output shape needs
      synthesized reasoning (mechanism, trace, diagram, diagnostic,
      change-impact, comparison, enumeration, or absence), parallel exploration
      waits for sibling handoffs. Scalar/count/key-value lookups may still
      converge early because the typed literal lane is the principal payload.
    - Guardrail tests: pure VCS/history narratives still converge early; mixed
      history+current-code mechanisms do not cancel the current-source sibling;
      ordinary bare cross-component mechanisms without external origin keep the
      existing early-convergence behavior.
    - Delivered in this slice: `parallelExploreMustWaitForSiblingHandoffs`
      now delegates mixed-origin detection to the unified answer-intent
      contract. Current-source-only mechanisms and pure history narratives keep
      the existing convergence behavior, while mixed VCS/diff/log/command plus
      current-source synthesis waits for every sibling lane to hand off its
      accepted facts before cancellation is allowed. Regression coverage pins
      mixed history+current-code mechanism dispatch, pure VCS early convergence,
      history-backed cross-component waiting, and current-source-only
      cross-component early convergence.

- Batch 46 — retry/status telemetry taxonomy. Status: in progress.
  - Split status and metrics into transport retry, schema/carrier repair,
    semantic rewrite, reviewer advisory, and accepted local supplement.
  - Guardrail tests: analyzer/finalizer transport errors render the current
    stage number; accepted advisory checks do not increment semantic rewrite
    counters.
  - Slice 46.1 — coalesced answer-review start notice. Status: completed.
    - Detailed design before code: reuse the existing reviewer dispatch logic,
      reviewer eligibility gates, and `EventOrchestratorNotice` rendering. Do
      not change reviewer prompts, reviewer results, violation strictness, or
      retry routing. Only change the user-visible start notice when both
      self-consistency and semantic-quality reviewers are runnable in the same
      contract-check pass.
    - When both reviewers run, emit one localized progress notice ("reviewing
      answer coverage and consistency") before dispatch and suppress the two
      per-reviewer start notices inside that pass. When only one reviewer runs,
      keep its existing specific notice. Direct calls to the reviewer methods
      keep the historical per-reviewer notice, so tests and future call sites
      still get truthful visibility.
    - Guardrail tests: a contract check with both reviewers eligible emits one
      start notice, not two; single-reviewer/direct-reviewer paths continue to
      emit their existing notices; no retry/advisory semantics change.
    - Delivered in this slice: `runContractCheck` now emits one combined
      localized progress notice when both self-consistency and semantic-quality
      reviewers run in the same pass, then suppresses their per-reviewer start
      notices for that pass only. The direct reviewer methods still emit their
      existing specific notices. Regression tests cover the coalesced
      contract-check path and the user-message redline audit.
  - Slice 46.2 — fallback retry status wording taxonomy. Status: completed.
    - Detailed design before code: reuse the existing `FallbackTarget`,
      `noticeKindForFallbackTarget`, and retry-routing policy. Do not change
      which violations retry, which layer they retry from, or whether a failed
      review becomes advisory. This slice is display-only telemetry: the same
      runtime decision must render with a clearer localized status line.
    - Root gap: several very different fallback paths can currently look like
      the same "答案待完善/正在重写" event in the REPL, so users cannot
      distinguish a finalizer-only rewrite from extractor restructuring,
      evidence-layer fallback, or terminal accept-with-boundary behavior. That
      obscures whether the system is doing a cheap local repair or has really
      gone back upstream.
    - Rule: keep one user-facing sentence per fallback target, with stable
      semantic boundaries:
        1. `FallbackFinalizerOnly` means only the final answer is being
           rewritten from accepted context;
        2. `FallbackBackToExtract` means the structured answer slate is being
           rebuilt from already accepted evidence;
        3. `FallbackBackToExplore` means the pipeline is returning to evidence
           collection for missing context;
        4. `FallbackFailLoud` means no more rewrite is useful and the answer
           should ship with a visible boundary/advisory note.
    - Guardrail tests: localized messages for the four targets must be
      distinct, avoid internal enum names, avoid transport-error wording, and
      avoid the overly broad "答案待完善" phrase on target-specific fallback
      lines. Unknown/reserved targets still degrade to the generic answer-check
      retry message.
    - Delivered in this slice: target-specific fallback status lines now
      distinguish final-answer rewrite, structured-answer rebuild,
      evidence-context fallback, and terminal accept-with-boundary behavior.
      The change is display-only: fallback routing, reviewer strictness, and
      retry budgets are untouched. Regression coverage pins distinct
      localized messages and rejects enum leaks, transport wording, and the
      over-broad "答案待完善" phrase on target-specific fallback notices.

- Batch 47 — diagram/renderability hardening. Status: in progress.
  - Keep Mermaid/code-fence display fixes renderer-only. This batch must not
    alter answer documents, memory, evidence, finalizer prompts, or diagram
    gate strictness.
  - Slice 47.1 — shared Mermaid fence detection for terminal and preview.
    Status: planned.
    - Detailed design before code: reuse the existing terminal renderer's
      Mermaid detection contract instead of adding a second browser-only
      heuristic. Move the known Mermaid keyword registry and tiny structural
      helpers (`first keyword`, `supported subset`, `info-string directive`,
      `body starts with Mermaid directive`) into `internal/mermaidcompat`.
      Keep terminal rendering policy unchanged: only flowchart/graph/sequence
      are rendered to ASCII; unsupported Mermaid kinds become the existing
      localized `text` fallback. Browser preview policy is different because it
      ships Mermaid.js: every known Mermaid syntax family, including
      `classDiagram`, should become a `<div class="mermaid">`.
    - Root gap: the terminal renderer already recognizes common model forms
      such as ` ```flowchart TD` and bare fences whose first body line is
      `flowchart`, but the preview renderer only routed fences whose first info
      token was exactly `mermaid`. The result is a silent UI degradation:
      diagrams that can render in the browser are displayed as ordinary code
      blocks.
    - Rule: route a fence to browser Mermaid when either (a) the info string is
      `mermaid ...`, (b) the info string itself begins with a known Mermaid
      directive such as `flowchart TD` or `classDiagram`, or (c) the fence is
      bare / `text` and its first non-empty body line begins with a known
      Mermaid directive. Do not route `bash`/`json`/language-tagged code blocks
      even if their body happens to contain arrows or Mermaid-like text.
    - Guardrail tests: preview renders direct-info `flowchart` and
      `classDiagram` fences as `<div class="mermaid">`; bare Mermaid-shaped
      fences render; ordinary code fences stay escaped `<pre><code>`; terminal
      renderer tests continue to pin unsupported-kind fallback and supported
      subset behavior.

## Scope

This document clusters recurring failures where the model is forced to produce,
repair, or preserve large structured JSON payloads while also reasoning about
the user's answer. These are not isolated JSON bugs. They are symptoms of a
larger responsibility split problem:

- the model is acting as answer author;
- the model is acting as a database serializer for row sets, support refs,
  citation pools, and patch payloads;
- the model is acting as a stateful patch merger across rejected attempts.

That combination increases tool-call corruption, retries, truncated payloads,
and dry final answers. The architecture goal is to keep user/model intent first
while moving deterministic carrier work back into typed runtime code.

## Evidence Reviewed

- `eval_20260522_full_sweep_gap_tracking.md`
  - u8a: decorated `member_set` support refs, mismatched aggregate counts, and
    one malformed long JSON closure.
  - u5a: `emit_evidence.items` emitted as a JSON-encoded string with fields
    accidentally outside item objects.
  - s7b: local tolerance parsed JSON-string `blocks[]`, but the final answer
    still expanded a scalar count into a full enumeration.
  - read-combo answer-document tools: `blocks[]` arrived as a JSON-encoded
    string and structured recovery could not preserve every visible item.
  - log-triage cases: array-shaped fields such as `errors` were emitted as JSON
    strings.
- `change_impact_handoff.md`
  - answer-document flat mode already has partial carrier repair, but some
    shapes still route citation objects into block recovery after brace-balance
    fallback.
  - `emit_answer_document_patch` can replace the citation pool while inheriting
    old citation-bearing blocks, causing stale integer refs.
- `finalizer_pretrip_prevention.md`
  - finalizer repair pressure is dominated by contract/wire-schema mismatch,
    prose repair hints, and late post-emit validation.

## Clustered Root Causes

### C1. Large row-set JSON is model-hostile

Inventories, member sets, table rows, support refs, and citation pools can reach
dozens or hundreds of entries. Asking the model to handwrite these as one tool
call creates common failure modes: JSON-stringified arrays, truncated objects,
missing sibling fields, stale counts, and accidental field placement outside
objects.

This is a model-load problem, not merely a parser bug. Even a strong model will
occasionally corrupt long mechanical JSON when it is also trying to preserve
answer semantics.

### C2. Deterministic data is being restated by the model

Many rejected fields are already known to the runtime: file:line support refs
from read gutters, aggregate row members, command/git result metadata, and
citation pools from prior accepted documents. Requiring the model to restate
them creates unnecessary opportunities for drift.

Runtime should materialize deterministic carriers from typed evidence when the
mapping is lossless. The model should own summaries, explanations, caveats, and
answer ordering.

### C3. Structured repair exists but is fragmented

Current compatibility is split across tool boundaries and stages:

- agent-level tool-param compatibility;
- `emit_answer_document` flat-mode recovery;
- aggregate fact normalizers;
- patch-specific validators;
- log/perf triage schema repair.

This means one stage may handle JSON-string arrays while another rejects the
same shape. The same class of small mechanical error can still burn full LLM
rounds in a different tool.

### C4. Patch and full-emit state machines leak model burden

`emit_answer_document_patch` asks the model to preserve old block ids and
citation indexes while applying semantic changes. When a patch is rejected, the
next prompt often asks the model to switch to full emit and carry forward the
previous complete document. This is too much state for the model to merge
reliably.

The runtime needs a transactional document-update layer that can safely rebase
or reject carrier-level operations before semantic validators fire.

### C5. Prose repair hints increase retry entropy

Many repair prompts describe required changes in prose. The model must parse
that prose, decide what is structural versus semantic, and then rebuild the JSON
payload. For deterministic fixes, this should be a typed repair plan with
actions and paths. Prose should remain only for semantic reviewer concerns.

### C6. Prompt surfaces duplicate the same fact in several forms

The same evidence can appear as raw evidence, aggregate facts, principal rows,
support lanes, closure summaries, and prior slate. Duplication wastes context
and makes the model choose between dry deterministic rows and richer summaries.
It also increases the chance that stale or downgraded rows survive into later
stages.

### C7. Large non-code evidence lacks a first-class payload protocol

Git, diff, logs, traces, command outputs, and future MCP/web data can all be
large, line-addressable, and answer-bearing. Treating them as prose snippets or
fake source file refs forces either truncation or bogus current-source
grounding. They need the same typed origin, pagination, negative-proof, and
summary-preservation path as source evidence.

## Architecture Direction

### A1. Shared StructuredPayloadCompat layer

Add one shared compatibility layer before strict tool unmarshal for all
structured tools. It should:

- decode JSON-string arrays/objects for known schema fields;
- split string arrays only when the target schema is an array of strings;
- preserve top-level siblings such as `citations`, `caveats`,
  `missing_requested_roles`, and `exact_resolution`;
- detect clearly truncated payloads and return a concise typed carrier error
  instead of sending the model into semantic guessing;
- never invent answer content or infer user intent.

This layer should emit telemetry: `tool`, `field_path`, `repair_kind`,
`lossless=true/false`, input size, row count, and whether an LLM retry was
avoided.

### A2. Server-side carrier compilers for deterministic row sets

For large row sets, the model should emit compact intent plus stable references
to already accepted evidence or aggregate groups. Runtime compilers should then
materialize:

- support refs from read gutters and typed evidence;
- citation keys from stable file:line or artifact-line anchors;
- complete count/member rows when the row set is already known;
- negative-proof rows from typed search/git/log/trace results.

This is allowed only when the mapping is lossless and typed. If not lossless,
the runtime may append a localized supplement that explains the missing
carrier, but must not overwrite model-authored answer text or tables.

### A3. PayloadRef / RowSetRef protocol for large payloads

When a row set or observation exceeds a small threshold, store it as a runtime
artifact and expose a stable reference:

- `row_set_ref` for structured rows;
- `payload_ref` for raw command/log/trace/git output;
- `page_ref` for paginated reads;
- `origin` and `scope` for source, VCS, runtime log, trace, command, MCP, or
  web data.

The model can cite and summarize the reference without serializing every row in
the tool call. Downstream stages can expand the reference within budget using
the unified evidence priority policy.

### A4. Typed repair plans for deterministic violations

Replace prose-only retry hints for deterministic validators with
`RepairHint{action, target_path, expected_shape, source_ref}`. Examples:

- `decode_json_string(field="blocks")`;
- `preserve_citation_pool()`;
- `add_missing_block(kind="diagram")`;
- `move_field_into_each_item(field="salience")`;
- `use_full_emit_because_patch_replaces_citations()`.

Semantic reviewer feedback can remain prose. Carrier and schema repairs should
be typed and bounded.

### A5. Transactional AnswerDocument updates

Unify patch and full emit around a document transaction:

- carrier validation runs before semantic checks;
- citation pool replacement is rejected or rebased before support-member
  coverage checks;
- rejected patch attempts do not leak stale ids into subsequent prompts;
- after a small bounded repair budget, ship the best answer with localized
  caveats rather than forcing repeated rewrites for non-critical carrier noise.

### A6. Single principal evidence ledger

Finalizer and reviewer should consume one curated principal ledger per answer
surface. The ledger must rank by typed request relevance and evidence authority:

1. user-requested output shape and exact targets;
2. landed / read / scoped / definition or direct anchors;
3. typed external observations requested by the user, such as git/log/trace;
4. rich merged summaries for the same stable anchor;
5. support and audit-only facts.

Duplicate raw/evidence/aggregate/slate forms should be collapsed into one row
that keeps all unique rich summaries. This prevents dry deterministic rows from
overriding better model-authored descriptions.

## Safety Rules

- No keyword matching over user prose or model free-form output for hard gates.
- Lossless carrier repair may happen silently with telemetry; non-lossless
  repair must be surfaced as a localized supplement or typed retry.
- Runtime may append clearly labeled supplements, but must not delete or replace
  model-authored tables/prose unless the model emits a replacement.
- JSON truncation should not be "completed" by guessing missing answer content.
  If the missing part is not reconstructable from accepted typed rows, ask for a
  smaller ref-based payload or ship with caveat.
- Git/log/trace/command/MCP/web observations are evidence origins, not fake
  source file citations.

## Task Plan

### P0. Instrument and classify

- Add telemetry for tool-call payload size, row count, JSON repair kind,
  truncated-detected, and retry-avoided.
- Add eval assertions for:
  - JSON-string `blocks[]` with top-level citations;
  - JSON-string `emit_evidence.items`;
  - malformed long closure over member sets;
  - patch citation-pool replacement conflict;
  - log-triage array fields emitted as strings.

### P1. Shared compatibility layer

- DONE (Batch 1): route the high-frequency structured emit tools through a
  shared compatibility entry point that delegates to the existing
  `internal/toolparam` schema-aware normalizer.
- DONE (Batch 11): move or wrap the remaining behavior-preserving top-level
  JSON-string repair helpers behind the same compatibility boundary.
- Remaining: answer-document-specific deep recovery/quarantine helpers still
  live with the answer-document implementation because they are not generic
  schema repair; refactor only after a transactional document-update layer
  exists.
- Common top-level JSON-string repair is now reached through the shared wrapper
  for `emit_analysis`, `emit_evidence`, `emit_answer_document`,
  `emit_answer_document_patch`, and log triage tools. Tools without the legacy
  wrapper already use the normal schema-aware entry point directly.
- Keep per-tool schema allowlists so repair is typed, not heuristic.
- Unit test every supported tool boundary.

### P1. Carrier compilers for large row sets

- Compile `member_set`, `source_inventory`, count/location sets, and
  negative-search proofs from accepted typed rows.
- Prefer model-authored summaries; merge unique summaries for the same stable
  anchor.
- Render any system-generated completion as a separate localized supplement,
  never as a replacement for the model answer.
- DONE (Batch 26): source-inventory carrier scope now has a single path
  normalization boundary before graph matching, including `./` and absolute
  path suffix forms, so carrier recovery does not silently disappear when the
  analyzer or tool returns a different path spelling.
- DONE (Batch 28): complete large `member_set` aggregate facts can now publish
  a row-set JSONL artifact through an optional writer, keeping row identity,
  support refs, and rich member notes page-able without changing the model's
  visible answer.
- DONE (Batch 30): exact large `excluded_count.excluded[]` aggregate facts now
  use the same row-set artifact path. Partial exclusion lists are deliberately
  ignored by the writer because they are examples/support, not a complete row
  carrier.

### P2. PayloadRef / RowSetRef

- DONE (Batch 12): reuse existing blob/session artifact storage for large
  command/git/log/trace outputs by projecting legacy `ToolResult.RawRef` and
  aggregate `blob_ref`/`payload_ref` dimensions into typed
  `ObservationSourceRef.PayloadRef`.
- DONE (Batch 27): finalizer/reviewer now render bounded `RawExcerpt` for
  non-current-source observations, so large external payloads still have a
  useful inline preview while full content remains behind `payload_ref` /
  `row_set_ref`.
- DONE (Batch 12): preserve typed `row_set_ref` and `page_ref` dimensions in the
  observation ledger and render them explicitly in finalizer/reviewer prompt
  sources.
- DONE (Batch 13): centralize source-ref formatting so all downstream
  prompt/reviewer consumers present the same labeled external payload contract.
- DONE (Batch 14): attach stable runtime-artifact ids to log/trace ledger rows
  so existing artifact-local spans become usable downstream without fake
  `file:line` grounding.
- PARTIAL (Batches 28, 30): automatic row-set artifact creation exists for
  large complete member carriers and exact exclusion carriers. Remaining work
  is selected high-salience row expansion policies for other carrier families,
  driven by eval data rather than broad automatic table generation.

### P2. Typed repair hints and transaction updates

- PARTIAL (Batches 15-16, 18-19): add typed repair hints for common deterministic
  answer-document carrier validators: empty full-emit `blocks[]`, patch citation
  operation conflicts, existing-block add/replace confusion after normalization
  is not possible, citation-pool replacement that would preserve old
  citation-bearing blocks, and lossy `blocks` string recovery where some visible
  model-authored blocks could not be safely preserved. Batch 18 also carries
  typed metadata for pre-emit hard structural hints derived from existing
  `emitFixHint` fields. Batch 19 carries strict-decode field repair metadata
  for common structured emit tools without changing the sanitized prose error.
- PARTIAL (Batch 29): patch transactions now absorb lossless id-surface carrier
  noise before validation: whitespace around op ids and exact duplicate op
  declarations no longer force a finalizer retry. Conflicting duplicate blocks
  still fail loudly because choosing one payload would alter model intent.
- PARTIAL (Batch 32): full document emits now use the same lossless block-id
  normalization for exact duplicate blocks. This keeps patch/full emit behavior
  aligned at the transaction boundary.
- Remaining: extend the same typed repair lane to other deterministic
  answer-document validators only where the target field/action is precise.
- Make patch/full-emit transitions transactional so carrier errors are fixed
  before semantic checks.
- Cap rewrite loops and ship with caveats for non-critical, non-lossless
  issues.

### P3. Prompt dedupe and principal ledger

- Audit finalizer/extractor prompts for duplicate evidence surfaces.
- PARTIAL (Batch 17): Observation Ledger keeps the single existing surface but
  raises rich-note budget only for typed principal rows/origin-specific
  principal observations, reducing dry answer risk without a second duplicate
  prompt section.
- PARTIAL (Batch 25): semantic reviewer now consumes the same compact
  rich-note/value/support-ref projection as the finalizer, so prompt-ledger
  convergence covers both final answer writing and second-opinion review.
- DONE (Batch 31): duplicate ledger records for the same stable typed anchor now
  merge their unique summaries/rich notes/support refs before finalizer/reviewer
  budgeting. This protects richness while removing repeated prompt surfaces.
- Preserve rich summaries while collapsing repeated carriers.
- Ensure mixed questions such as "based on this diff, analyze current code" rank
  both VCS observations and current-source anchors according to the typed user
  request, not by a fixed origin preference.

## Open Questions

- Exact threshold for switching from inline rows to `row_set_ref`.
- Whether `payload_ref` should be exposed to the model as a tool-readable object
  or only as prompt context with selected pages.
- How to express future MCP/web evidence origins without coupling to a specific
  connector.
