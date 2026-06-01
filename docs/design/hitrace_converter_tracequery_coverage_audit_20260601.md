# HiTrace Converter and Trace Query Coverage Audit

Date: 2026-06-01

## Scope

This document audits Codrax's binary HiTrace converter and `trace_query`
implementation against the latest OpenHarmony converter under:

- https://gitee.com/openharmony/hiviewdfx_hitrace/tree/master/tools/hitrace_converter
- `hitrace_converter.py`
- `parse_functions.py`

The remote source was fetched from the `master` branch during this audit. At
the time of inspection, `parse_functions.py` defines 86 `PRINT_FMT_*` mappings.

The audit has two goals:

1. Ensure conversion does not silently lose information when binary HiTrace
   files are converted into text systrace rows.
2. Ensure `trace_query` can consume the converted rows with correct Harmony and
   Donghu scheduler semantics.

## Platform Semantics

Donghu must not be modeled as mixed scheduler semantics.

Donghu means:

- System base: Harmony/OpenHarmony.
- Trace format and scheduler semantics: Harmony HiTrace/ftrace-compatible rows.
- Time unit: seconds. For example, `2942.124416` means seconds.
- Priority semantics: Harmony user-space priority semantics.
  - Larger numeric value means higher priority.
  - `1..40` means CFS.
  - `41..139` means RT.
  - Values outside this range are raw/system/kernel and must be reported as
    such.
- Framework surface: Android framework processes and Harmony framework
  processes may coexist.
- Mixing boundary: process-level isolation. Do not infer that one process mixes
  Android and Harmony framework semantics internally.

Recommended model:

```text
platform=donghu
trace_flavor=harmony_hitrace
scheduler_semantics=harmony_ohos
time_unit=seconds
priority_semantics=harmony_user_priority
framework_mode=process_isolated_mixed
process.framework_surface=android_framework|harmony_framework|unknown
```

Current code status:

- `NormalizeTraceFlavor("东湖")` currently maps to `harmony_hitrace`, which is
  correct for time and priority semantics.
- The missing piece is a first-class `platform=donghu` / `framework_surface`
  layer. Android-looking process names such as `com.tencent.mm`,
  `Choreographer`, or `binder:*` must not cause Android priority semantics to
  override Donghu/Harmony scheduler semantics.

## Current Local Implementation

Relevant files:

- `internal/hitraceconv/convert.go`
- `internal/hitraceconv/eventformat.go`
- `internal/hitraceconv/render.go`
- `internal/tracequery/parse.go`
- `internal/tracequery/types.go`
- `internal/tracequery/flavor.go`
- `internal/tracequery/query.go`

The Go converter currently reads:

- file header
- segment headers
- event format segment
- cmdline segment
- TGID segment
- raw trace segments
- page headers
- event headers
- event content identified by event id

The converter strongly renders:

- `sched_switch`
- `sched_wakeup`
- `sched_waking`
- `sched_blocked_reason`
- `cpu_idle`
- `cpu_frequency`
- `clock_set_rate`
- `block_rq_issue`
- `block_rq_complete`
- `block_bio_remap`
- `binder_transaction`
- `binder_transaction_received`
- `irq_handler_entry`
- `irq_handler_exit`
- `tracing_mark_write` / `print` rows when they expose canonical `B|E|C`
  payloads

Unsupported known events are rendered through generic `field=value` fallback
when field metadata is available. Missing event formats currently become
`raw_event=unparsed` rows.

`trace_query` currently classifies:

- scheduler events
- CPU idle/frequency events
- clock events
- block IO issue/remap/complete
- binder transaction/received
- IRQ/softirq-like events as a broad IRQ class
- canonical trace marks
- some memory/reclaim/fault/page-cache-like events
- unknown events

Important current gap: `trace_query.ParseLine` clears `FieldText` for
`EventUnknown`. That means converter generic fallback can preserve text in the
systrace row, but `trace_query` may drop the same weak-structured fields from
its index and downstream views. This is the highest-priority "do not lose
information" gap.

## Event Coverage Matrix

Legend:

- Strong: converter renders the event into a stable ftrace-style row and
  `trace_query` consumes the event into a typed semantic lane.
- Partial: the event is retained or roughly classified, but some fields or
  semantics are not yet first-class.
- Weak: converter fallback may retain fields as text, but `trace_query` does
  not use them for stats/root-cause ranking.
- Gap: information can be lost or ignored unless the user manually reads raw
  lines.

| OpenHarmony converter event family | Converter status | `trace_query` status | Gap |
| --- | --- | --- | --- |
| `IRQ_HANDLER_ENTRY`, `IRQ_HANDLER_EXIT` | Strong | Strong as `irq` | Good enough for V1. |
| `SOFTIRQ_ENTRY_EXIT` | Weak/generic | Partial as broad `irq` if raw name contains softirq | Missing `vec` and symbolic action such as `NET_RX`, `SCHED`, `RCU`. |
| `SCHED_WAKEUP_HM`, `SCHED_WAKEUP` | Strong | Strong | Good. |
| `SCHED_SWITCH_HM`, `SCHED_SWITCH` | Strong for base fields | Strong for base fields | Linux `expeller_type` and Harmony `next_info`/`cg` variants are not first-class. |
| `SCHED_SWITCH_HM_NEW`, `SCHED_SWITCH_HM_NINFO_CG` | Partial | Partial | Need preserve `next_info` and `cg`. |
| `SCHED_BLOCKED_REASON_*` | Strong base support | Strong for D/IO reasoning | Good, but richer caller/module fields should remain visible. |
| `CPU_FREQUENCY_*` | Strong | Strong | Good. |
| `CLOCK_SET_RATE_*` | Strong for simple fields | Strong/partial depending clock name | Non-CPU clocks such as DDR/HECA are not yet capacity evidence. |
| `CPU_FREQUENCY_LIMITS_*` | Weak/generic | Partial as CPU frequency-like event | Missing first-class min/max residency and capacity-limit evidence. |
| `CPU_IDLE_*` | Strong | Strong | Good. |
| `EXT4_*` | Weak/generic | Weak/unknown | Missing file-system write/sync evidence. |
| `BLOCK_BIO_REMAP*` | Strong-ish | Strong-ish | Device major/minor and remap linkage should be normalized. |
| `BLOCK_RQ_ISSUE*`, `BLOCK_RQ_COMPLETE*` | Strong-ish | Strong-ish with latency pairing | Need exact dev/sector/op/len/error normalization for pairing confidence. |
| `UFSHCD_*` | Weak/generic | Weak/unknown | Missing UFS command, clock, power, and error semantics. |
| `MMC_REQUEST_*` | Weak/generic | Weak/unknown | Missing MMC request latency/error evidence. |
| `I2C_*`, `SMBUS_*` | Weak/generic | Weak/unknown | Missing peripheral bus latency/error evidence. |
| `REGULATOR_*` | Weak/generic | Weak/unknown | Missing voltage/power supply evidence. |
| `DMA_FENCE_*` | Weak/generic | Weak/unknown | Missing GPU/display fence blocking evidence. |
| `BINDER_TRANSACTION`, `BINDER_TRANSACTION_RECEIVED` | Strong | Strong for IPC graph base edges | Full synchronous wait/reply closure still needs stronger causal projection. |
| `FILE_CHECK_AND_ADVANCE_WB_ERR`, `FILEMAP_SET_WB_ERR` | Weak/generic | Weak/unknown | Missing writeback error evidence. |
| `MM_FILEMAP_*` | Weak/generic | Partial as memory/page-cache if raw text survives | Needs stronger memory/page-cache lane. |
| `RSS_STAT_HM` | Weak/generic | Weak/unknown | Missing RSS/memory pressure evidence. |
| `WORKQUEUE_EXECUTE_START_OR_END` | Weak/generic | Weak/unknown | Missing kernel worker execution spans. |
| `THERMAL_POWER_ALLOCATOR*` | Weak/generic | Weak/unknown | Missing thermal throttling / power allocation evidence. |
| `PRINT`, `TRACING_MARK_WRITE` | Partial | Strong only for canonical `B/E/C` | Non-canonical print rows and IP/function prefix are not first-class. |
| `XACCT_TRACING_MARK_WRITE` | Weak/generic | Weak/unknown | Should canonicalize to B/E style span rows. |
| `PHASE_TASK_DELTA` | Weak/generic | Weak/unknown | Missing execution delta evidence. |
| `EROFS_*`, `Z_EROFS_*` | Weak/generic | Weak/unknown | Missing read/lookup/getattr/xattr latency/error evidence. |

## Detailed Gaps

### P0: Unknown Event Field Preservation

Problem:

`trace_query` clears `FieldText` for unknown events. This breaks the converter's
generic fallback contract.

Impact:

- Unsupported but known OpenHarmony events may survive in the text file but not
  in `trace_query` rowsets.
- `event_search` cannot reliably search unsupported event fields.
- Future root-cause views cannot consume weak-structured evidence.

Required fix:

- Preserve `FieldText` for `EventUnknown`.
- Add a bounded `raw_fields` or `field_text` lane for unknown rows.
- Keep large output bounded by existing rowset/blob mechanisms.
- Add tests proving unsupported events remain searchable by event name and
  field text.

### P0: Missing-Format Raw Payload Preservation

Problem:

When an event id has no format metadata, the converter currently emits
`raw_event=unparsed` without the raw payload bytes.

Impact:

- Missing event format rows lose forensic data.
- Users cannot later re-parse with a newer format table.

Required fix:

- Include `event_id`, payload length, and a bounded hex prefix in the row.
- Count and caveat missing formats.
- Do not dump unbounded payloads inline.

### P0: Donghu Platform Layer

Problem:

Donghu currently collapses into `harmony_hitrace`. That is correct for scheduler
semantics but not expressive enough for process-level Android/Harmony framework
coexistence.

Required fix:

- Add `platform=donghu` as a distinct user-visible platform or platform hint.
- Keep `trace_flavor=harmony_hitrace`.
- Add `framework_mode=process_isolated_mixed`.
- Optionally infer per-process `framework_surface` as advisory:
  `android_framework`, `harmony_framework`, or `unknown`.
- Do not let Android-looking process names override Harmony time/priority
  semantics when the user or file context indicates Donghu.

### P0: Scheduler Variant Fields

Problem:

OpenHarmony converter supports `next_info` and `cg` sched_switch variants.
Codrax only renders/consumes base fields.

Required fix:

- Preserve `next_info` and `cg` in converter output when present.
- Add optional fields in `tracequery.Event`.
- Surface these fields in `event_search` and scheduler evidence packs.

### P1: CPU Capacity and Frequency Limits

Problem:

`cpu_frequency_limits` is currently treated like CPU-frequency-like text, but
min/max limits are not first-class.

Required fix:

- Add `EventCPUFrequencyLimit` or typed fields for min/max/cpu.
- Feed limits into compute-supply/root-cause evidence.
- Keep `cpu_frequency` residency separate from limit residency.

### P1: Storage and File-System Events

Problem:

OpenHarmony converter has rich UFSHCD, MMC, EXT4, EROFS, writeback, and filemap
events. Codrax currently does not use most of them.

Required fix:

- Introduce storage/file-system event classes:
  `storage_ufs`, `storage_mmc`, `fs_ext4`, `fs_erofs`, `writeback`,
  `page_cache`.
- Feed these into `window_stats`, `critical_blocking_calls`, and
  `root_cause_rank`.
- Pair start/end events where possible and report latency/error.

### P1: Power, Thermal, Regulator, DMA Fence, Workqueue

Problem:

These events are important for performance diagnosis but are currently weak.

Required fix:

- Thermal/regulator: capacity/power caveats and throttling evidence.
- DMA fence: display/GPU blocking evidence.
- Workqueue: kernel worker execution spans and top worker stats.
- Phase task delta: execution delta evidence.

### P1: Dynamic String and Array Decoding

Problem:

The OpenHarmony converter decodes `__data_loc`, `__get_str`,
`__print_symbolic`, `__print_hex`, and `__print_array` shapes for many events.
The Go converter mostly decodes fixed char arrays and event-specific simple
fields.

Required fix:

- Add generic dynamic string decoding for `__data_loc_*` fields.
- Add bounded dynamic array / hex rendering.
- Add symbolic enum rendering for important families such as softirq, UFS, SMBUS,
  and regulator/thermal states.

### P2: Coverage Regression Test

Problem:

Coverage can drift as OpenHarmony updates `parse_functions.py`.

Required fix:

- Add a checked-in coverage manifest derived from the remote converter event
  families.
- Tests should classify each known family as `strong`, `partial`, `weak`, or
  `unsupported`.
- Failing tests should trigger when a strong family regresses or a weak family
  loses raw field preservation.

## Recommended Task Checklist

P0:

- [ ] Preserve `EventUnknown.FieldText` in `trace_query`.
- [ ] Add missing-format raw payload hex fallback in `hitraceconv`.
- [ ] Add Donghu platform layer while keeping Harmony time/priority semantics.
- [ ] Preserve Harmony sched_switch `next_info` and `cg`.
- [ ] Add tests for weak event searchability and Donghu priority semantics.

P1:

- [ ] Add CPU frequency limit typed fields and compute-supply evidence.
- [ ] Normalize block dev major/minor and complete error fields.
- [ ] Add storage/file-system event classes for UFSHCD, MMC, EXT4, EROFS,
      writeback, and page-cache rows.
- [ ] Add thermal/regulator/DMA fence/workqueue/phase-task classes.
- [ ] Add generic `__data_loc` string and bounded array/hex decoding.

P2:

- [ ] Add OpenHarmony converter coverage manifest.
- [ ] Add eval fixtures for each high-value family.
- [ ] Add a `codrax trace convert --audit` or developer-only report that prints
      converter and `trace_query` coverage status for a binary/text trace.

## Current Risk Assessment

Current Codrax can already answer common Harmony/Donghu performance questions
that depend on:

- scheduler state
- wakeup chain
- runnable/running timing
- CPU idle/frequency
- basic IO issue/complete
- binder base edges
- IRQ bursts
- canonical B/E/C spans

It is not yet complete for "no information loss" across the full OpenHarmony
converter surface. The main commercial risk is not the core scheduler path; it
is that unsupported rich subsystem rows may be present in the trace, preserved
as text by conversion, but not carried into `trace_query`'s structured evidence
and root-cause ranking.

The safest next implementation order is:

1. Preserve unknown field text and missing-format raw payloads.
2. Add Donghu platform surface while keeping Harmony scheduler semantics.
3. Add scheduler variant fields.
4. Add CPU limit and storage/file-system classes.
5. Add power/thermal/fence/workqueue classes.
6. Add coverage manifest and eval cases.
