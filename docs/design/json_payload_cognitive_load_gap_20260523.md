# JSON Payload and Model Cognitive Load Gap Cluster

Date: 2026-05-23

Status: partially implemented. Batches 1-10 are complete and verified; the
shared schema-aware repair path is active across the high-frequency structured
emit tools. Remaining work is still tracked below, starting with P1 carrier
compilers / row-set references, followed by PayloadRef/RowSetRef,
transactional document updates, typed repair hints, and prompt/ledger
deduplication.

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
- Remaining work starts at P1 carrier compilers / row-set references; the first
  batch deliberately did not change answer materialization policy.

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
- TODO: move or wrap the remaining local JSON-string repair helpers behind the
  same entry point where doing so is behavior-preserving.
- Move existing local JSON-string repair into one package used by
  `emit_analysis`, `emit_evidence`, `emit_investigation_complete`,
  `emit_answer_document`, `emit_answer_document_patch`, and log/perf triage
  tools.
- Keep per-tool schema allowlists so repair is typed, not heuristic.
- Unit test every supported tool boundary.

### P1. Carrier compilers for large row sets

- Compile `member_set`, `source_inventory`, count/location sets, and
  negative-search proofs from accepted typed rows.
- Prefer model-authored summaries; merge unique summaries for the same stable
  anchor.
- Render any system-generated completion as a separate localized supplement,
  never as a replacement for the model answer.

### P2. PayloadRef / RowSetRef

- Reuse existing blob/session artifact storage for large command/git/log/trace
  outputs.
- Add paginated and line-addressable metadata for non-code artifacts.
- Teach downstream prompts to consume concise refs plus selected high-salience
  rows rather than full payloads.

### P2. Typed repair hints and transaction updates

- Add typed repair hints for deterministic validators.
- Make patch/full-emit transitions transactional so carrier errors are fixed
  before semantic checks.
- Cap rewrite loops and ship with caveats for non-critical, non-lossless
  issues.

### P3. Prompt dedupe and principal ledger

- Audit finalizer/extractor prompts for duplicate evidence surfaces.
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
