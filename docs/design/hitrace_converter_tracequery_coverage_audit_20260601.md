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

Codrax is **not 100% functionally equivalent** to the official OpenHarmony
Python converter yet.

What is aligned:

- Official mapped event inventory: **86 / 86** entries are present in
  `internal/hitraceconv/testdata/openharmony_print_fmt_coverage.tsv`.
- No official `PRINT_FMT_*` mapping is missing from Codrax's coverage manifest.
- Core scheduler/CPU/IO/binder/IRQ/trace-mark rows used by the main
  `trace_query` causality engine are strongly supported.
- Unknown rows keep bounded `FieldText` in `trace_query`, so weak/generic rows
  remain searchable instead of disappearing.
- Missing event formats are preserved with `event_id`, payload length, and
  bounded payload hex in the converted systrace row.

What is not yet equivalent:

- Only **25 / 86** OpenHarmony `PRINT_FMT_*` mappings are currently classified
  as `strong`.
- **21 / 86** depend on dynamic string rendering and are retained, but not all
  of their official symbolic/array decorations are reproduced exactly.
- **39 / 86** are `generic_typed`: fields are preserved as `field=value`, but
  Codrax does not yet reproduce the official event-specific prose format or use
  every field as first-class root-cause evidence.
- **1 / 86** is `generic_preserved`: retained for search/audit but not
  semantically interpreted.

Therefore the current guarantee is:

```text
high-value scheduler/resource analysis path: strong enough for trace_query
full OpenHarmony converter output byte/format equivalence: not yet guaranteed
```

This distinction matters commercially. Scheduler, wakeup-chain, CPU frequency,
basic IO, binder, IRQ, page-cache, and canonical trace span analysis are covered
well. Rich subsystem diagnostics such as UFSHCD/MMC/I2C/SMBUS/thermal/regulator
and many EROFS variants are preserved but not yet formatted exactly like the
official Python converter.

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

Important current limitation: weak subsystem rows are searchable and counted,
but many are not yet event-specific enough to drive deterministic root-cause
ranking with the same fidelity as scheduler/CPU/IO/binder rows.

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
| `SOFTIRQ_ENTRY_EXIT` | Generic typed | Strong as `softirq` after conversion text exposes `vec/action` | Converter does not reproduce official symbolic `__print_symbolic` formatting itself. |
| `SCHED_WAKEUP_HM`, `SCHED_WAKEUP` | Strong | Strong | Good. |
| `SCHED_SWITCH_HM`, `SCHED_SWITCH` | Strong for base fields | Strong for base fields | Linux `expeller_type` is not first-class. |
| `SCHED_SWITCH_HM_NEW`, `SCHED_SWITCH_HM_NINFO_CG` | Strong | Strong | Harmony `next_info` and `cg` are preserved. |
| `SCHED_BLOCKED_REASON_*` | Strong base support | Strong for D/IO reasoning | Good, but richer caller/module fields should remain visible. |
| `CPU_FREQUENCY_*` | Strong | Strong | Good. |
| `CLOCK_SET_RATE_*` | Dynamic-string | Strong/partial depending clock name | Non-CPU clocks such as DDR/HECA are preserved but not always capacity evidence. |
| `CPU_FREQUENCY_LIMITS_*` | Generic typed | Strong as `cpu_frequency_limits` if min/max/cpu fields are present | Converter does not reproduce official formatting exactly. |
| `CPU_IDLE_*` | Strong | Strong | Good. |
| `EXT4_*` | Weak/generic | Weak/unknown | Missing file-system write/sync evidence. |
| `BLOCK_BIO_REMAP*` | Strong | Strong | Good enough for V1. |
| `BLOCK_RQ_ISSUE*`, `BLOCK_RQ_COMPLETE*` | Strong for common forms | Strong with latency pairing | Dynamic `cmd` string forms are preserved but not all official decorations are reproduced. |
| `UFSHCD_*` | Weak/generic | Weak/unknown | Missing UFS command, clock, power, and error semantics. |
| `MMC_REQUEST_*` | Weak/generic | Weak/unknown | Missing MMC request latency/error evidence. |
| `I2C_*`, `SMBUS_*` | Weak/generic | Weak/unknown | Missing peripheral bus latency/error evidence. |
| `REGULATOR_*` | Weak/generic | Weak/unknown | Missing voltage/power supply evidence. |
| `DMA_FENCE_*` | Weak/generic | Weak/unknown | Missing GPU/display fence blocking evidence. |
| `BINDER_TRANSACTION`, `BINDER_TRANSACTION_RECEIVED` | Strong | Strong for IPC graph base edges | Full synchronous wait/reply closure still needs stronger causal projection. |
| `FILE_CHECK_AND_ADVANCE_WB_ERR`, `FILEMAP_SET_WB_ERR` | Generic typed / partial | Filesystem/writeback coarse lane | Missing event-specific writeback ranking. |
| `MM_FILEMAP_*` | Strong | Memory/page-cache coarse lane | Good for page-cache evidence; not yet a full memory-pressure model. |
| `RSS_STAT_HM` | Generic typed | Memory coarse lane | Missing RSS-specific pressure evidence. |
| `WORKQUEUE_EXECUTE_START_OR_END` | Generic typed | Workqueue coarse lane | Missing kernel worker execution spans. |
| `THERMAL_POWER_ALLOCATOR*` | Generic typed | Power/thermal coarse lane | Missing thermal throttling ranking. |
| `PRINT`, `TRACING_MARK_WRITE` | Partial | Strong only for canonical `B/E/C` | Non-canonical print rows and IP/function prefix are not first-class. |
| `XACCT_TRACING_MARK_WRITE` | Weak/generic | Weak/unknown | Should canonicalize to B/E style span rows. |
| `PHASE_TASK_DELTA` | Weak/generic | Weak/unknown | Missing execution delta evidence. |
| `EROFS_*`, `Z_EROFS_*` | Weak/generic | Weak/unknown | Missing read/lookup/getattr/xattr latency/error evidence. |

## Detailed Gaps

### Completed P0 Alignment Items

- `EventUnknown.FieldText` is preserved and searchable.
- Missing-format rows include `event_id`, `payload_len`, and bounded payload hex.
- Donghu is represented as Harmony HiTrace scheduler semantics with a distinct
  platform lane.
- Harmony `sched_switch` variants preserve `next_info` and `cg`.
- `mm_filemap_*page_cache` rows are strongly rendered with official
  `dev/ino/page/pfn/ofs` semantics.

### Remaining P0/P1 Gap: Exact Official Formatting for Non-Core Families

Problem:

Many OpenHarmony official parser functions do more than field preservation:
they compute derived fields, decode symbolic enums, format arrays/hex buffers,
and pair related values into event-specific prose. Codrax preserves those rows
but does not yet reproduce every official string exactly.

Impact:

- Search and audit still work for most rows.
- `trace_query` can count subsystem activity.
- Root-cause ranking may under-use UFSHCD/MMC/I2C/SMBUS/thermal/regulator/EROFS
  details because they are not all first-class typed evidence.

Required fix:

- Port the remaining official parse functions family by family, prioritizing
  storage, filesystem, thermal/power, DMA fence, and workqueue.
- Add golden tests for every ported official `PRINT_FMT_*` shape.

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
- The manifest classifies support as `strong`, `dynamic_string`,
  `generic_typed`, or `generic_preserved`.
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

P1:

- [x] Add CPU frequency limit typed fields and compute-supply evidence.
- [x] Normalize block dev major/minor and complete error fields for common
      block request rows.
- [ ] Add storage/file-system event classes for UFSHCD, MMC, EXT4, EROFS,
      writeback, and page-cache rows with official event-specific formatting.
- [ ] Add thermal/regulator/DMA fence/workqueue/phase-task classes.
- [x] Add generic `__data_loc` string decoding for known dynamic string shapes.
- [ ] Add bounded array/hex rendering and symbolic enum rendering.

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

It is not yet complete for exact parity with the full OpenHarmony converter
surface. The main commercial risk is not the core scheduler path; it is that
rich subsystem rows may be preserved as text/coarse subsystem observations but
not carried into `trace_query`'s structured evidence and root-cause ranking with
the same fidelity as the official event-specific renderers.

The safest next implementation order is:

1. Port storage and filesystem official renderers: UFSHCD, MMC, EXT4, EROFS,
   writeback.
2. Port power/display official renderers: thermal, regulator, DMA fence.
3. Port peripheral bus official renderers: I2C and SMBUS.
4. Add symbolic enum and bounded array/hex support where official rows use
   `__print_symbolic`, `__print_hex`, or `__print_array`.
5. Add eval fixtures for every newly strong family.
