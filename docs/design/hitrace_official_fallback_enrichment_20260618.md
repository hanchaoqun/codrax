# HiTrace Official Fallback Enrichment Audit - 2026-06-18

## Scope

Compared the current converter and trace query pipeline against the latest public OpenHarmony source trees:

- `developtools_hiperf`
  - `include/perf_file_format.h`
  - `src/perf_file_writer.cpp`
  - `proto/report_sample.proto`
- `developtools_profiler`
  - `protos/services/common_types.proto`
  - `protos/types/plugins/ftrace_data/default/trace_plugin_result.proto`
  - `protos/types/plugins/ftrace_data/default/ftrace_event.proto`
  - `protos/types/plugins/ftrace_data/default/ext4.proto`

## Findings

1. `hiperf` perf.data contains useful fallback metadata beyond basic `pid/tid/time/cpu/ip/period`:
   `EVENT_DESC`, `BUILD_ID`, `CMDLINE`, `NRCPUS`, `TOTAL_MEM`, `META_INFO`,
   `HIPERF_FILES_SYMBOL`, `HIPERF_RECORD_TIME`, `HIPERF_CPU_OFF`,
   `HIPERF_HM_DEVHOST`, and `HIPERF_FILES_UNISTACK_TABLE`.

2. Current raw fallback already parses most capture-level feature metadata and saved
   hiperf symbol names. Saved `HIPERF_FILES_SYMBOL` names are consumed by symbol
   type, not by user wording: ELF/HAP/V8 keep their existing mapping rules, while
   kernel and kernel-thread saved symbols can be matched directly by sampled PC
   when the perf stream lacks an explicit kernel mmap record. The remaining
   low-risk sample-level fields were safe to preserve as context: sample
   address/id/stream id, perf weight, data source, transaction, physical address,
   cgroup id, page sizes, raw payload size, branch stack count, user-reg count,
   user-stack size, and aux size.

3. OpenHarmony `report_sample.proto` remains the preferred official path for full
   symbolization, but it does not expose sample CPU in the current format. The
   built-in raw fallback can sometimes provide CPU where official report proto
   cannot, so both paths remain useful and should be surfaced with explicit quality.

4. `developtools_profiler` ftrace proto includes capture-level stats (`ftrace_cpu_stats`,
   `overwrite`, `clocks_detail`, `symbols_detail`) and a very large generated event
   matrix. Codrax should continue to prefer official tooling for full protobuf
   TracePluginResult event rendering. For built-in conversion, parse the
   top-level quality metadata (`dropped_events`, `overrun`, `overwrite`,
   `trace_clock`, plugin timestamp/version/sample interval, clock details, symbol
   count/examples) because it is stable, compact, and directly guides confidence.
   Add targeted event renderers only when the fields feed existing trace_query
   structures.

5. `ext4_direct_IO_enter/exit` is a direct match for existing inode IO aggregation.
   It should be rendered to the same stable field shape as android_fs/f2fs:
   `dev/ino/offset/len/rw/ret`.

## Delivered

- Raw perf fallback now treats parsed sample fields as parsed bits, not skipped bits.
- Raw perf `.perftrace` rows include safe sample-level context fields when present.
- Saved hiperf kernel/kernel-thread symbols can now resolve raw perf samples even
  when the perf stream does not carry an explicit `[kernel.kallsyms]` mmap record.
- OpenHarmony profiler `ftrace-plugin` structured protobuf messages now surface
  top-level capture metadata in conversion caveats/tracebundle provenance:
  plugin clock/timestamp/version/sample interval, CPU dropped/overrun/read stats,
  structured event counts, overwrite totals, clock details, and symbol examples.
- `trace_query` parses perf sample context fields into `Event` and carries them
  into perf hotspot examples without changing root-cause ranking weights.
- `trace_query` now also consumes `.tracebundle.json` top-level caveats, artifact
  caveats, perf capability metadata, provider decisions, and perf clock-alignment
  notes as `Result.Caveats`. This closes the handoff gap for profiler
  `ftrace-plugin` capture quality metadata and raw-perf record-quality counters:
  models see them together with query results instead of only in `/htrace convert`
  terminal output.
- OpenHarmony ext4 direct IO rows are covered by round-trip tests from binary htrace
  conversion into `file_io_by_inode` and `storage_latency_by_layer`.

## Trace Query Consumption Closure

Official `hiperf` and profiler sources expose several data families with different
root-cause semantics:

| Official data family | Codrax parse surface | Model consumption rule |
| --- | --- | --- |
| `report_sample.proto` / normalized `perf_sample` rows | `window_stats.perf_samples`, `perf_stats`, `perf_timeline`, root-cause perf contexts | Use as code-execution support for running/compute-supply, runnable competitors, chain dependencies, or binder peers; samples alone are not scheduler root cause proof. |
| raw `perf.data` sample fields and saved hiperf symbols | normalized `.perftrace` `perf_sample` rows with source/symbolization/callchain/clock fields | Consume top symbols/DSOs/callchains/threads with `perf_quality`; keep raw fallback/IP-only caveats attached. |
| raw perf record-quality counters (`LOST`, `LOST_SAMPLES`, throttle/unthrottle, AUX) | tracebundle/artifact `Result.Caveats` plus generated perftrace `parser_caveats` | Treat as sample quality and coverage risk, not as direct runtime pressure. |
| profiler `ftrace-plugin` capture metadata (`dropped_events`, overrun, overwrite, clocks, symbols) | tracebundle `Result.Caveats` | Treat as capture quality/provenance; use it to bound confidence and recommend official export when data loss is material. |
| tracebundle perf capability/provider/clock alignment | tracebundle `Result.Caveats` | Explain whether evidence came from official adapter or raw fallback, whether sample CPU/callchain/symbolization are complete, and whether trace/perf time overlap is calibrated. |
| scheduler, binder, IO, IRQ, supply, trace-mark rows | `Event`, `WindowStats`, `RootCauseRank`, `FrameRootCauseBundle` | These remain the causal basis for root-cause ranking; quality caveats only calibrate confidence. |

Guardrail: tracebundle metadata is output-side provenance. It does not add model
tool-call JSON fields and therefore does not need new model input burden; existing
`trace_query` structured-parameter repair still covers all user/tool-call inputs.

## Systrace Parser Closure

The latest OpenHarmony profiler formatter sources also expose several official
systrace spelling variants that must be query-consumable, even when Codrax does
not implement the full generated formatter matrix.

Delivered closure:

- `sched_wakeup_new` is rendered by the built-in converter with the same
  `comm/pid/prio/target_cpu` shape as `sched_wakeup` and is parsed as
  `sched_wakeup`.
- Official block aliases `block_rq_insert`, `block_getrq`, and
  `block_bio_queue` are parsed as block issue evidence; `block_bio_complete` is
  parsed as block completion evidence.
- Official trace marker variants `tracing_mark_write_xacct` and
  `xacct_tracing_mark_write` are parsed as trace markers.
- Official `print` rows whose payload is emitted as `0xADDR: B|...`,
  `0xADDR: E|...`, `0xADDR: C|...`, `0xADDR: S|...`, or `0xADDR: F|...` are
  normalized before B/E/C/S/F span parsing. This preserves correct stack-based
  span matching without asking the model to search for named end markers.
- The `trace_query.event_types` schema and compatibility repair layer accept
  those official aliases and map them to the stable structured event types used
  by `event_search`, `span_window`, `window_stats`, root-cause ranking, and frame
  bundle views.

Audited but intentionally not folded into current root-cause weighting:

- `sched_stat_wait/sleep/iowait/blocked/runtime` can be useful as corroborating
  kernel accounting, but the current ranking already derives elapsed runnable,
  sleep, D-state, IO-wait, and running intervals from `sched_switch`. Folding
  `sched_stat_*` into impact without a calibrated merge design would risk
  double-counting. Keep it as a future evidence surface if needed.
- `ipi_entry/ipi_exit/ipi_raise` can support a broader compute-supply/interrupt
  pressure summary, but it is not yet a causal dependency primitive in the
  existing scheduler/binder/IO model. Add it only with a dedicated pressure
  aggregation design and tests.

## Remaining Non-Goals

- Do not implement the full official TracePluginResult protobuf formatter matrix in
  Codrax. Keep official tools as the high-confidence renderer and add only focused
  built-in renderers that feed existing structured query views.
- Do not infer elapsed time from perf sample weights. The query layer must keep
  sample weights as event weights unless a calibrated event definition and clock
  mapping are available.
