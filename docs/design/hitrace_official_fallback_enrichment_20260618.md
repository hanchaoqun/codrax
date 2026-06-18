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
- `trace_query` parses those fields into `Event` and carries them into perf hotspot
  examples without changing root-cause ranking weights.
- OpenHarmony ext4 direct IO rows are covered by round-trip tests from binary htrace
  conversion into `file_io_by_inode` and `storage_latency_by_layer`.

## Remaining Non-Goals

- Do not implement the full official TracePluginResult protobuf formatter matrix in
  Codrax. Keep official tools as the high-confidence renderer and add only focused
  built-in renderers that feed existing structured query views.
- Do not infer elapsed time from perf sample weights. The query layer must keep
  sample weights as event weights unless a calibrated event definition and clock
  mapping are available.
