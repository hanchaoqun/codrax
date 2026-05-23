# JSON Payload and Model Cognitive Load Gap Cluster

Date: 2026-05-23

Status: partially implemented. Batches 1-32 are complete and verified; the
shared schema-aware repair path is active across the high-frequency structured
emit tools, including the legacy top-level JSON-string repair wrappers.
Remaining work is still tracked below, starting with P1 carrier compilers /
row-set compilers, followed by deeper transactional document updates, typed
repair hints for remaining validators, and prompt/ledger deduplication.

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
  planned.
  - Reuse repomap language metadata where available instead of Go-only
    heuristics.
  - Source inventory supplements remain separate localized system notes and do
    not replace model tables or prose.

- Batch 38 — runtime artifact origin normalization. Status: planned.
  - Align git/log/trace/command observation refs, excerpts, and line/span labels
    so finalizer/reviewer consume the same external-evidence contract.
  - Preserve pagination through existing blob/row-set mechanisms.

- Batch 39 — system supplement and caveat authority cleanup. Status: planned.
  - System supplements must be clearly localized and append-only.
  - Non-critical, non-lossless issues should become bounded notes rather than
    finalizer rewrites.

- Batch 40 — performance and convergence guards. Status: planned.
  - Analyzer fast paths and parallel exploration convergence should reduce wait
    time without changing user intent.
  - Add regression guards where a future developer could accidentally re-open
    hard gates from noisy evidence.

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
