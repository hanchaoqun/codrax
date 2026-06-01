# HiTrace Converter and Trace Query Gap Delivery Plan

Date: 2026-06-01

Parent audit:

- `docs/design/hitrace_converter_tracequery_coverage_audit_20260601.md`

## Delivery Principles

- Keep trace/log/runtime artifact work isolated from normal source-code analysis.
- Do not change `repo_map`, source evidence gates, or source citation semantics.
- Treat precise parsed event fields as hard facts; use broad subsystem signals only
  for soft root-cause ranking and caveats.
- Donghu follows Harmony/OpenHarmony scheduler semantics:
  - timestamp unit is seconds
  - larger numeric user priority means higher priority
  - `1..40=CFS`, `41..139=RT`
  - Android framework and Harmony framework may coexist only at process level
- Do not drop unsupported trace rows. If an event is not typed yet, preserve its
  event name, line number, timestamp, thread identity, and bounded field text.
- Large data must stay bounded through existing rowset/blob payload patterns.

## Batch 0: Documentation and Task Ledger

Goal:

Create an implementation checklist that keeps the OpenHarmony converter audit,
Donghu semantics, and trace-query follow-up work visible.

Design:

- Add this document as the delivery ledger.
- Keep it separate from the coverage audit so future implementation commits can
  update task status without rewriting the audit facts.

Tasks:

- [x] Record P0/P1/P2 implementation batches.
- [x] Link the audit document.
- [x] State non-goals and red lines.

Verification:

- Manual doc review.

## Batch 1: P0 No-Loss Core

Goal:

Make sure unsupported OpenHarmony events remain searchable and visible instead
of disappearing from `trace_query` internals.

Current code:

- `internal/tracequery/parse.go` initializes `FieldText` from row text but then
  clears it for `EventUnknown`.
- `internal/hitraceconv/render.go` emits `raw_event=unparsed` for rows whose
  event format is absent.
- `internal/hitraceconv/convert.go` knows the event id and raw content at render
  time but does not pass enough missing-format context into the renderer.

Design:

- Preserve `FieldText` for `EventUnknown`.
- Keep field text bounded by the existing clamp.
- Add tests showing unknown events retain `field_text` and are searchable by
  `event_search`.
- For missing event formats, output:
  - `event_id=<id>`
  - `payload_len=<n>`
  - `payload_hex=<bounded hex prefix>`
  - optional `payload_truncated=true`
- Preserve the existing missing-format count and caveat.

Tasks:

- [ ] Remove `EventUnknown.FieldText` clearing.
- [ ] Add bounded hex helper for missing-format rows.
- [ ] Thread missing event id into render fallback.
- [ ] Add parser/search tests.
- [ ] Add converter tests for missing-format payload fallback.

Verification:

- `go test ./internal/tracequery ./internal/hitraceconv`
- Focused test names added in this batch.

## Batch 2: P0 Donghu Platform and Scheduler Variants

Goal:

Represent Donghu explicitly without changing its Harmony scheduler semantics,
and preserve Harmony sched-switch variant fields.

Current code:

- `internal/tracequery/flavor.go` maps `"东湖"` to `TraceFlavorHarmonyHitrace`.
- `trace_query` tool schema has `platform` as an alias for `trace_flavor`.
- `Result` has `TraceFlavor`, `FlavorConfidence`, `FlavorSignals`,
  `TimeUnit`, and `PrioritySemantics`, but not `platform`,
  `framework_mode`, or per-process framework surface.
- `hitraceconv.renderEventBody` renders base Harmony `sched_switch` fields but
  not `next_info` or `cg`.
- `tracequery.Event` has no `NextInfo` or `CGroup` fields.

Design:

- Add a platform layer while preserving the existing flavor layer.
- Normalize `platform=donghu` / `东湖` to a new platform value that implies:
  - `trace_flavor=harmony_hitrace`
  - `framework_mode=process_isolated_mixed`
  - Harmony time and priority semantics
- Keep `trace_flavor` backward compatible. Existing callers passing
  `harmony_hitrace` continue to work.
- Do not let Android framework process names override explicit Donghu intent.
- Add advisory process framework surface detection only for display and audit.
  This must not affect scheduler state classification or priority mapping.
- Extend `sched_switch` rendering/parsing to retain optional `next_info` and
  `cg`.

Tasks:

- [ ] Add `TracePlatform` / platform normalization.
- [ ] Add `Platform`, `FrameworkMode`, and optional framework surface summary to
      `tracequery.Result`.
- [ ] Update `trace_query` schema/description and JSON repair test to cover
      `platform=donghu`.
- [ ] Add `NextInfo` and `CGroup` fields to `Event`.
- [ ] Preserve `next_info`/`cg` in converter output and trace parser.
- [ ] Add Donghu tests proving Harmony priority semantics remain active even
      with Android-looking process names.

Verification:

- `go test ./internal/tracequery ./internal/tool ./internal/agent`

## Batch 3: P1 Capacity, IO, Storage, and Subsystem Classes

Goal:

Bring more OpenHarmony converter event families into typed query lanes so
window stats and root-cause ranking can use them.

Current code:

- `cpu_frequency_limits` is classified as `EventCPUFrequency`, but min/max
  fields are not first-class.
- Block IO pairing exists but uses string parsing that can miss complete error
  and major/minor normalization.
- Rich file-system/storage/power events are mostly `EventUnknown`.

Design:

- Add event types and typed fields incrementally:
  - `cpu_frequency_limits`
  - `storage`
  - `filesystem`
  - `power`
  - `workqueue`
  - `dma_fence`
  - `softirq`
- Preserve backward compatibility by keeping old broad counters stable where
  possible.
- Feed only precise typed fields into root-cause scoring. Broad family matches
  should add counts/caveats and soft evidence, not hard conclusions.

Tasks:

- [x] Add min/max/cpu typed fields for CPU frequency limits.
- [x] Normalize block complete error while preserving device/op/sector/len
      pairing keys.
- [x] Classify UFSHCD/MMC/I2C/SMBus storage rows as storage events.
- [x] Classify EXT4/EROFS/writeback rows as filesystem/page-cache
      events.
- [x] Classify thermal/regulator rows as power/capacity events.
- [x] Classify workqueue and DMA fence rows.
- [x] Add softirq event fields.
- [x] Add window stats counters and compact subsystem summaries.
- [x] Add root-cause-rank enrichment where precise CPU-frequency-limit fields
      exist.

Verification:

- `go test ./internal/tracequery`
- Add fixtures for one representative row per family.
- `internal/hitraceconv/testdata/openharmony_print_fmt_coverage.tsv` records
  the current upstream 86 `PRINT_FMT_*` rows and the Codrax converter/query
  support lane for each row. Rows marked `generic_typed` or
  `generic_preserved` are intentionally no-loss but not full semantic
  renderers.

## Batch 4: P1 Dynamic Field Decoding

Goal:

Improve binary conversion fidelity for event formats that use dynamic strings,
arrays, symbolic values, or printable buffers.

Current code:

- Fixed char arrays are decoded.
- Some event-specific dynamic strings are decoded manually.
- Generic fallback does not understand all `__data_loc`, `__get_str`,
  `__print_symbolic`, `__print_hex`, or `__print_array` patterns used by the
  OpenHarmony converter.

Design:

- Add generic bounded dynamic string decoding for `__data_loc_*` fields.
- Add bounded dynamic array/hex formatting.
- Add small symbolic maps for high-value families:
  - softirq action
  - UFS opcode/state
  - SMBUS protocol
  - regulator/thermal state where stable
- Never evaluate arbitrary Python/C print expressions dynamically.

Tasks:

- [x] Implement generic `__data_loc` field decoder for dynamic strings.
- [x] Keep missing-format payload hex bounded and preserve dynamic-string
      fallback rows without unbounded inline dumps.
- [ ] Add symbolic maps for selected stable enums.
- [x] Add converter tests for dynamic string rows.
- [ ] Add converter tests for dynamic array rows.
- [x] Add an OpenHarmony `PRINT_FMT_*` coverage manifest test so future
      converter/query support changes cannot silently drop audited rows.

Verification:

- `go test ./internal/hitraceconv`

## Batch 5: P2 Coverage Manifest and Eval Guard

Goal:

Prevent drift as OpenHarmony adds or changes converter events.

Design:

- Add a checked-in coverage manifest listing the 86 audited `PRINT_FMT_*`
  families and local support level:
  - `strong`
  - `partial`
  - `weak`
  - `unsupported`
- Add tests that verify:
  - strong families remain strong
  - weak families preserve field text
  - no audited family silently disappears from the manifest
- Keep the manifest local and deterministic. Do not fetch the network during
  tests.

Tasks:

- [ ] Add coverage manifest under `internal/hitraceconv/testdata/` or
      `internal/tracequery/testdata/`.
- [ ] Add coverage test.
- [ ] Add representative eval case notes.

Verification:

- `go test ./internal/hitraceconv ./internal/tracequery`
- Final `go test ./...`

## Commit Plan

- Commit 0: docs ledger and audit documents.
- Commit 1: P0 no-loss core.
- Commit 2: P0 Donghu/platform and scheduler variant fields.
- Commit 3: P1 typed subsystem classes.
- Commit 4: P1 dynamic field decoding.
- Commit 5: P2 coverage manifest and final regression tests.

Each implementation commit should be pushed before moving to the next batch.
