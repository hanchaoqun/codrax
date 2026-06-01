# HiTrace Converter and Trace Query Coverage Audit

Date: 2026-06-01

## Scope

This document audits Codrax's binary HiTrace converter and `trace_query`
implementation against the latest OpenHarmony converter under:

- https://gitee.com/openharmony/hiviewdfx_hitrace/tree/master/tools/hitrace_converter
- `hitrace_converter.py`
- `parse_functions.py`

The remote source was re-fetched from the `master` branch during this audit.
The audited upstream revision was:

```text
14df05b3ac80822dbe0bdc78ad999e25dc3d0bad
```

At the time of inspection, `parse_functions.py` defines 86 `PRINT_FMT_*`
mappings.

The audit has two goals:

1. Ensure conversion does not silently lose information when binary HiTrace
   files are converted into text systrace rows.
2. Ensure `trace_query` can consume the converted rows with correct Harmony and
   Donghu scheduler semantics.

## Executive Conclusion

Codrax now targets **official OpenHarmony Python converter-compatible systrace
body rendering** for the audited `parse_functions.py` surface.

What is aligned for conversion:

- Official mapped event inventory: **86 / 86** entries are present in
  `internal/hitraceconv/testdata/openharmony_print_fmt_coverage.tsv`.
- No official `PRINT_FMT_*` mapping is missing from Codrax's coverage manifest.
- Converter support classification is now **86 / 86 strong** for the audited
  official `PRINT_FMT_*` rows.
- The generated systrace line envelope now follows the official viewer
  conventions: `<idle>` task name, common flag/preempt rendering, stable
  timestamp sorting, and official body strings for scheduler, CPU, block,
  binder, IRQ, trace-mark, storage, filesystem, power, workqueue, thermal, I2C,
  SMBus, MMC, UFSHCD, regulator, DMA fence, RSS, and EROFS/Z_EROFS rows.
- Field-level rendering is audited against upstream `parse_functions.py`, not
  only against event names. High-risk format details covered by tests include
  data-loc string offsets, RSS `size=<n>` without a local-only unit suffix, and
  official storage/filesystem delimiter shapes.
- Core scheduler/CPU/IO/binder/IRQ/trace-mark rows used by the main
  `trace_query` causality engine are strongly supported.
- Unknown rows keep bounded `FieldText` in `trace_query`, so weak/generic rows
  remain searchable instead of disappearing.
- Missing binary event formats are skipped, matching the upstream converter.
  Events whose format exists but whose body renderer is not available are
  emitted as official-style header-only rows, also matching the upstream
  converter fallback shape. Codrax does not write `unknown_event_*`,
  `payload_hex=...`, or `raw_event=unparsed` rows into converted systrace files.

Important boundary:

- Conversion compatibility and `trace_query` root-cause semantics are separate.
  The converter now renders all audited official rows strongly; `trace_query`
  still intentionally consumes some rich subsystem rows as coarse storage,
  filesystem, memory, power, or workqueue observations rather than making every
  subsystem field a hard root-cause signal.
- If OpenHarmony updates `parse_functions.py`, the manifest must be refreshed
  and the new `PRINT_FMT_*` entries must be ported before claiming coverage for
  that newer upstream revision.

Therefore the current guarantee is:

```text
audited OpenHarmony PRINT_FMT conversion: 86/86 strong
trace_query root-cause semantics: strong for scheduler/CPU/IO/binder core path,
                                 coarse/advisory for some rich subsystem fields
```

This distinction matters commercially. Converted systrace output should remain
compatible with the official browser/parser expectations for the audited rows,
while deeper deterministic ranking for every storage/power/filesystem subfield
can continue to evolve inside `trace_query`.

## Field Rendering Compatibility Notes

The converter deliberately mirrors the upstream Python converter's text shapes:

- Missing `events_format` entries are counted and skipped before rendering.
- Unsupported `print_fmt` rows are counted and emitted as a line envelope only;
  no nonstandard generic body is appended.
- Dynamic string fields such as `__get_str(name)` and `__data_loc_*` are read
  from their payload offsets. Numeric offset fields are not decoded as raw
  printable bytes before the offset lookup, which avoids garbage values in
  fields such as `clock_set_rate` names.
- `rss_stat` follows the official body exactly: `mm_id=%d curr=%d member=%d
  size=%d`; Codrax does not append a local-only `B` suffix.
- Device delimiters follow the official event family:
  - block rows use `major,minor`
  - file/page-cache rows use `major:minor`
  - EROFS rows use the official `dev:(major,minor)` or `dev:major,minor`
    shapes for each function family.

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

- `NormalizeTraceFlavor("东湖")` maps to `harmony_hitrace`, which is correct for
  time and priority semantics.
- `trace_query` also carries a first-class Donghu platform lane so user-specified
  Donghu/Harmony semantics are not overridden by Android-looking process names
  such as `com.tencent.mm`, `Choreographer`, or `binder:*`.

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

The converter strongly renders the high-value causality path:

- `sched_switch`
- `sched_wakeup`
- `sched_waking`
- `sched_blocked_reason`
- `cpu_idle`
- `cpu_frequency`
- `block_rq_issue`
- `block_rq_complete`
- `block_bio_remap`
- `binder_transaction`
- `binder_transaction_received`
- `irq_handler_entry`
- `irq_handler_exit`
- `mm_filemap_add_to_page_cache`
- `mm_filemap_delete_from_page_cache`
- `tracing_mark_write` / `print` rows when they expose canonical `B|E|C`
  payloads

Dynamic-string events such as `clock_set_rate`, some UFSHCD/MMC/regulator
events, and EROFS lookup/read-enter rows are rendered with bounded dynamic
string support where the field metadata exposes `__data_loc`/string payloads.

Unsupported known events are rendered through generic `field=value` fallback
when field metadata is available. Missing event formats become bounded raw rows
with `event_id`, `payload_len`, and a payload hex prefix.

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
- storage / filesystem / power / workqueue / DMA fence classes at a coarse
  subsystem level

Important current limitation: rich subsystem rows are now rendered in official
systrace shape, but some are still consumed by `trace_query` as coarse subsystem
observations rather than event-specific hard root-cause signals.

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
| `SOFTIRQ_ENTRY_EXIT` | Strong | Strong as `softirq` | Official symbolic action rendering covered. |
| `SCHED_WAKEUP_HM`, `SCHED_WAKEUP` | Strong | Strong | Good. |
| `SCHED_SWITCH_HM`, `SCHED_SWITCH` | Strong for base fields | Strong for base fields | Linux `expeller_type` is not first-class. |
| `SCHED_SWITCH_HM_NEW`, `SCHED_SWITCH_HM_NINFO_CG` | Strong | Strong | Harmony `next_info` and `cg` are preserved. |
| `SCHED_BLOCKED_REASON_*` | Strong base support | Strong for D/IO reasoning | Good, but richer caller/module fields should remain visible. |
| `CPU_FREQUENCY_*` | Strong | Strong | Good. |
| `CLOCK_SET_RATE_*` | Strong | Strong/partial depending clock name | Non-CPU clocks such as DDR/HECA are preserved; not every non-CPU clock is capacity evidence. |
| `CPU_FREQUENCY_LIMITS_*` | Strong | Strong as `cpu_frequency_limits` if min/max/cpu fields are present | Good. |
| `CPU_IDLE_*` | Strong | Strong | Good. |
| `EXT4_*` | Strong | Filesystem coarse lane | Official row rendering covered; root-cause ranking remains coarse. |
| `BLOCK_BIO_REMAP*` | Strong | Strong | Good enough for V1. |
| `BLOCK_RQ_ISSUE*`, `BLOCK_RQ_COMPLETE*` | Strong | Strong with latency pairing | Good. |
| `UFSHCD_*` | Strong | Storage coarse lane | Official row rendering covered; deeper command/power ranking remains future work. |
| `MMC_REQUEST_*` | Strong | Storage coarse lane | Official row rendering covered; deeper request-latency ranking remains future work. |
| `I2C_*`, `SMBUS_*` | Strong | Storage/peripheral coarse lane | Official row rendering covered; deeper peripheral latency ranking remains future work. |
| `REGULATOR_*` | Strong | Power coarse lane | Official row rendering covered; deeper power-supply ranking remains future work. |
| `DMA_FENCE_*` | Strong | DMA fence lane | Official row rendering covered. |
| `BINDER_TRANSACTION`, `BINDER_TRANSACTION_RECEIVED` | Strong | Strong for IPC graph base edges | Full synchronous wait/reply closure still needs stronger causal projection. |
| `FILE_CHECK_AND_ADVANCE_WB_ERR`, `FILEMAP_SET_WB_ERR` | Strong | Filesystem/writeback coarse lane | Official row rendering covered; root-cause ranking remains coarse. |
| `MM_FILEMAP_*` | Strong | Memory/page-cache coarse lane | Good for page-cache evidence; not yet a full memory-pressure model. |
| `RSS_STAT_HM` | Strong | Memory coarse lane | Official row rendering covered; RSS-specific pressure ranking remains future work. |
| `WORKQUEUE_EXECUTE_START_OR_END` | Strong | Workqueue coarse lane | Official row rendering covered; worker span ranking remains future work. |
| `THERMAL_POWER_ALLOCATOR*` | Strong | Power/thermal coarse lane | Official row rendering covered; thermal throttling ranking remains future work. |
| `PRINT`, `TRACING_MARK_WRITE` | Strong | Strong for canonical `B/E/C`; searchable otherwise | Official row rendering covered. |
| `XACCT_TRACING_MARK_WRITE` | Strong | Trace-mark lane | Canonical B/E style row rendering covered. |
| `PHASE_TASK_DELTA` | Strong | Coarse execution delta lane | Official row rendering covered. |
| `EROFS_*`, `Z_EROFS_*` | Strong | Filesystem coarse lane | Official row rendering covered; deeper latency/error ranking remains future work. |

## Detailed Gaps

### Completed P0 Alignment Items

- `EventUnknown.FieldText` is preserved and searchable.
- Missing-format rows include `event_id`, `payload_len`, and bounded payload hex.
- Donghu is represented as Harmony HiTrace scheduler semantics with a distinct
  platform lane.
- Harmony `sched_switch` variants preserve `next_info` and `cg`.
- `mm_filemap_*page_cache` rows are strongly rendered with official
  `dev/ino/page/pfn/ofs` semantics.
- Official systrace line envelope compatibility is covered: `<idle>` task name,
  common flag/preempt rendering, and stable same-timestamp ordering.
- The converter renders all 86 audited OpenHarmony `PRINT_FMT_*` mappings as
  strong official-format systrace body rows.

### Remaining P1 Gap: Root-Cause Semantics for Rich Subsystem Families

Problem:

Many OpenHarmony subsystem rows now convert with official body strings, but
`trace_query` intentionally consumes several families as coarse observations
instead of event-specific hard root-cause signals.

Impact:

- Search and audit still work for most rows.
- `trace_query` can count subsystem activity.
- Root-cause ranking may under-use UFSHCD/MMC/I2C/SMBUS/thermal/regulator/EROFS
  details because they are not all first-class typed evidence yet.

Required fix:

- Promote high-value subsystem fields into typed `trace_query` evidence family
  by family, prioritizing storage latency/error, display/fence blocking,
  thermal/power throttling, and filesystem/writeback latency.
- Add eval fixtures for every newly promoted root-cause family.

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

### Coverage Regression Guard

Problem:

Coverage can drift as OpenHarmony updates `parse_functions.py`.

Current status:

- A checked-in coverage manifest exists.
- Tests assert the manifest covers 86 official OpenHarmony rows.
- The manifest classifies all 86 audited converter rows as `strong`.
- Future work should make this manifest auto-refreshable from a pinned upstream
  snapshot or a developer-only audit script.

## Recommended Task Checklist

P0 completed:

- [x] Preserve `EventUnknown.FieldText` in `trace_query`.
- [x] Add missing-format raw payload hex fallback in `hitraceconv`.
- [x] Add Donghu platform layer while keeping Harmony time/priority semantics.
- [x] Preserve Harmony sched_switch `next_info` and `cg`.
- [x] Add tests for weak event searchability and Donghu priority semantics.
- [x] Strongly render `mm_filemap_*page_cache`.
- [x] Match official systrace line envelope details.
- [x] Strongly render all 86 audited OpenHarmony `PRINT_FMT_*` mappings.

P1:

- [x] Add CPU frequency limit typed fields and compute-supply evidence.
- [x] Normalize block dev major/minor and complete error fields for common
      block request rows.
- [x] Add storage/file-system official event-specific formatting for UFSHCD,
      MMC, EXT4, EROFS, writeback, and page-cache rows.
- [x] Add thermal/regulator/DMA fence/workqueue/phase-task official event
      formatting.
- [x] Add generic `__data_loc` string decoding for known dynamic string shapes.
- [x] Add bounded array/hex rendering and symbolic enum rendering for audited
      OpenHarmony converter rows.
- [ ] Promote rich subsystem fields into deeper `trace_query` root-cause
      ranking where the signal is deterministic.

P2:

- [x] Add OpenHarmony converter coverage manifest.
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

Conversion now targets exact official row-shape parity for the audited
OpenHarmony converter surface. The main remaining commercial risk is no longer
systrace browser compatibility for known rows; it is that rich subsystem rows may
be carried into `trace_query` as coarse subsystem observations rather than
root-cause-ranked typed evidence.

The safest next implementation order is:

1. Add eval fixtures for every newly strong converter family.
2. Promote storage and filesystem fields into deeper `trace_query` evidence:
   UFSHCD, MMC, EXT4, EROFS, writeback.
3. Promote power/display fields: thermal, regulator, DMA fence.
4. Promote peripheral bus fields: I2C and SMBUS.
5. Add an audit command that refreshes the coverage manifest against a pinned
   upstream `parse_functions.py`.
