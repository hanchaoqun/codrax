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

- Batch 43 — runtime artifact provenance split. Status: planned.
  - Carry `observed_direct_cause`, `artifact_span`, and
    `inferred_upstream_possibility` as typed lanes for log/trace/perf answers.
  - Guardrail tests: trace answers do not expose internal zero-based frame
    indices unless the artifact contains a frame ordinal; log answers do not
    promote caller-side root cause without current-source proof.

- Batch 44 — analyzer fast-path consolidation. Status: planned.
  - Add typed fast paths for exact-file import literal enumeration,
    history+current-code hybrid classification, exact-symbol conditional
    questions, and item-vs-batch error-granularity questions. These must use
    typed request-shape/provenance fields, not user-prose keyword hard gates.
  - Guardrail tests: analyzer emits classification after existence checks and
    leaves implementation proof to exploration.

- Batch 45 — hybrid explorer partitioning and repo-map wait visibility. Status:
  planned.
  - Partition hybrid questions by typed origin/facet: VCS lane owns history
    narrative; current-source lane owns present implementation; sibling lanes
    converge once their facet is covered.
  - Instrument repo-map concurrent build/cache waits and show truthful progress
    including lock waits.
  - Guardrail tests: mixed history+current-source cases avoid duplicated
    searches/reads; two concurrent repo-map builds report wait state instead of
    silent stalls.

- Batch 46 — retry/status telemetry taxonomy. Status: planned.
  - Split status and metrics into transport retry, schema/carrier repair,
    semantic rewrite, reviewer advisory, and accepted local supplement.
  - Guardrail tests: analyzer/finalizer transport errors render the current
    stage number; accepted advisory checks do not increment semantic rewrite
    counters.

- Batch 47 — diagram/renderability hardening. Status: planned.
  - Keep Mermaid/code-fence display fixes renderer-only, but add deterministic
    render/subset validation so supported diagrams degrade to a localized text
    fallback instead of silently failing in UI.
  - Guardrail tests: supported diagram kinds render or produce the existing
    `text` fence warning; model memory/output still preserves original source.

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
