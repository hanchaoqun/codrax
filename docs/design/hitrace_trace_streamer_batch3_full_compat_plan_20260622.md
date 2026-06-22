# HiTrace trace_streamer Batch 3 Full Compatibility Plan

Date: 2026-06-22

Status: planned, not yet implemented.

Parent plan:
`docs/design/hitrace_trace_streamer_delivery_plan_20260622.md`

Reference implementation:
`/tmp/codrax-ref-hmtrace`

## Goal

Batch 3 must deliver a commercial DB-to-systrace exporter, not a small table
demo. The exporter should consume `trace_streamer` SQLite databases through
typed table-family extractors, emit Codrax query-native systrace rows, preserve
coverage/provenance in tracebundle, and be guarded by hmtrace-like fixtures.

The completion bar is compatibility at the table-family and semantic-output
level:

- scheduler/wakeup/root-cause evidence remains queryable by `trace_query`;
- counter/slice/log/system-event families remain visible as stable trace rows;
- missing tables and schema drift are captured as coverage facts rather than
  silent success;
- large DB ordering is bounded and spillable;
- no user-intent keyword or filename suffix routing is introduced;
- no model tool-call JSON field is added unless a real trace_query input changes.

## hmtrace Audit

Reviewed files:

- `/tmp/codrax-ref-hmtrace/src/native/extractors/scheduling.rs`
- `/tmp/codrax-ref-hmtrace/src/native/extractors/metadata.rs`
- `/tmp/codrax-ref-hmtrace/src/native/extractors/slices.rs`
- `/tmp/codrax-ref-hmtrace/src/native/extractors/counters.rs`
- `/tmp/codrax-ref-hmtrace/src/native/extractors/logging.rs`
- `/tmp/codrax-ref-hmtrace/src/native/resolvers.rs`
- `/tmp/codrax-ref-hmtrace/src/native/util.rs`
- `/tmp/codrax-ref-hmtrace/tests/golden_diff.rs`
- `/tmp/codrax-ref-hmtrace/tests/export_format.rs`
- `/tmp/codrax-ref-hmtrace/reference/python/tests/test_db2systrace.py`

Observed reusable design:

- DB extraction is table-class based, not per-file or per-question.
- Common resolvers load `argsets`, raw-event CPUs, next sched-start metadata,
  running intervals, active thread ids, and trace start.
- `thread`/`process` metadata emits process dump, task rename, and process-name
  trace marks before event rows.
- `sched_slice` emits `sched_switch` rows at slice end, grouped by CPU.
- `instant + raw + sched_slice` emits `sched_wakeup`/`sched_waking` with raw CPU
  and next-run target CPU/priority.
- `irq + args + data_dict` emits IRQ and softirq entry/exit rows.
- `callstack`, `frame_slice`, `syscall`, `task_pool`, `app_startup`,
  `static_initalize`, and `native_hook` emit trace marker spans.
- `measure` families emit `cpu_idle`, `cpu_frequency`, `clock_set_rate`, and
  counters.
- `network`, `diskio`, `cpu_usage`, `live_process`, `xpower_measure`,
  `log`, and `hisys_all_event` emit counters or print rows.
- Reference tests compare Rust DB export against a Python reference for covered
  and comprehensive fixtures, and separately validate perfetto perf export.

## Codrax Integration Points

Existing code to reuse:

- `internal/hitraceconv.ConvertFile`
- `traceProviderDecision` and `ArtifactTraceDB`
- `renderedRow`, `writeRows`, systrace header format, and timestamp sorting
- `writeTraceBundle`
- `tracequery.BuildIndex` for round-trip validation

Needed changes:

- Add a DB exporter package under `internal/hitraceconv`.
- Add pure-Go SQLite through `modernc.org/sqlite`.
- Add coverage serialization into tracebundle metadata.
- Replace DB-only trace_streamer success with DB export once coverage is
  available.
- Keep built-in sys binary parser only as a parity-gated lane until DB parity is
  proven for no-perf Harmony/Donghu `.sys` captures.

## Sub-Batches

### Batch 3A: DB Core, Resolver Layer, and Coverage

Status: delivered on 2026-06-22.

Tasks:

- Delivered DB open/introspection helpers:
  - table exists,
  - column exists,
  - table row count,
  - safe typed nullable reads,
  - schema drift errors with table/column context.
- Delivered resolver helpers:
  - process/thread maps,
  - `argsets` via `args + data_dict`,
  - raw event CPU map from `raw`,
  - sched starts from `sched_slice`,
  - running intervals from `thread_state`,
  - active thread ids from callstack/sched/thread_state/syscall/native/frame.
- Delivered `TraceDBCoverage` fields sufficient for handoff:
  - family,
  - table,
  - found,
  - columns present/missing,
  - rows read,
  - rows emitted,
  - skipped/error reason.
- Delivered coverage threading into `Result` and tracebundle JSON.
- Delivered tests for introspection, schema drift, resolver loading, and coverage
  serialization.

Performance and memory:

- Open DB read-only.
- Do not materialize full DB tables except bounded resolver maps whose key space
  is naturally limited by thread/argset counts.
- Track resolver row counts for coverage.

Verified with:

```bash
go test ./internal/hitraceconv ./cmd ./internal/repl
```

### Batch 3B: Bounded Row Sorter and Systrace Writer

Status: delivered on 2026-06-22.

Tasks:

- Delivered a row sink that accepts `renderedRow` values from table extractors.
- Delivered an in-memory threshold for rows; spill sorted chunks to temp files when
  exceeded.
- Delivered k-way merge of chunks by `(timestamp_ns, sequence)` into the systrace
  writer.
- Delivered sorter stats into coverage:
  - peak buffered rows,
  - spill chunk count,
  - rows merged,
  - temp bytes when known.
- Delivered tests:
  - stable ordering with same timestamp sequence;
  - forced spill with low threshold;
  - cleanup on success and on failure;
  - output parses through `tracequery.BuildIndex`.

Performance and memory:

- Large DB export must be O(threshold) memory plus resolver maps.
- Temp files are deleted on all exits.

Verified with:

```bash
go test ./internal/hitraceconv ./cmd ./internal/repl
```

### Batch 3C: Scheduler, Wakeup, IRQ, and Metadata Families

Status: delivered on 2026-06-22.

Tasks:

- Delivered `thread`/`process` registrations:
  - process dump when wrapper mode is enabled,
  - `task_rename`,
  - process-name `tracing_mark_write` begin/end pairs.
- Delivered `sched_slice -> sched_switch` with streaming per-CPU lookahead rather
  than whole-table CPU grouping.
- Delivered `instant -> sched_wakeup/sched_waking` using:
  - raw-event CPU from `raw`,
  - target CPU/priority from next `sched_slice`,
  - thread/process names from resolver maps.
- Delivered `irq -> irq_handler_entry/exit` and softirq rows using generic
  argsets.
- Delivered tests mirroring hmtrace:
  - process name fallback,
  - main-thread process naming,
  - CPU count from trace data,
  - wakeup raw CPU and next-run target metadata,
  - IRQ/softirq arg decoding,
  - tracequery round trip.

Verified with:

```bash
go test ./internal/hitraceconv ./cmd ./internal/repl
```

### Batch 3D: Trace Marker, Slice, Counter, Log, and System Families

Status: delivered on 2026-06-22.

Tasks:

- Delivered `callstack`, including async `S/F` and running CPU lookup.
- Delivered `frame_slice`, `dma_fence`, `syscall`, `task_pool`, `app_startup`,
  `static_initalize`, and `native_hook`.
- Delivered `measure + cpu_measure_filter` to `cpu_idle`, `cpu_frequency`, and
  CPU limit events when schema exposes limit filters.
- Delivered `measure + measure_filter` to `clock_set_rate` and generic counters.
- Delivered `process_measure`, `network`, `diskio`, `cpu_usage`, `live_process`,
  `xpower_measure`, `log`, and `hisys_all_event`.
- Delivered a hmtrace-comprehensive-style Go fixture asserting semantic rows and
  tracequery trace-marker round trip.

Verified with:

```bash
go test ./internal/hitraceconv ./cmd ./internal/repl
```

### Batch 3E: ConvertFile Integration

Status: delivered on 2026-06-22.

Tasks:

- Delivered: when trace_streamer DB export succeeds, Codrax runs the DB exporter
  before deciding result.
- If systrace rows are emitted:
  - delivered `.systrace` output,
  - delivered `.trace_db` preservation when requested,
  - delivered tracebundle coverage handoff,
  - delivered trace_streamer provider `trace_query_ready=true` through systrace
    artifact readiness.
- If no systrace rows are emitted:
  - delivered partial bundle with coverage explaining why,
  - delivered no `trace_query_ready` scheduler claim for no-row DBs.
- Delivered explicit `--trace-engine=trace_streamer` fail-fast for unavailable
  tool.
- Delivered transparent `auto` fallback behavior in provider decisions, carrying
  trace DB coverage into fallback bundles.
- Delivered tests:
  - fake trace_streamer writes a real SQLite fixture DB;
  - conversion emits systrace and tracebundle coverage;
  - DB-only no-row output is explicit partial result;
  - generated systrace merges with perftrace sidecars through tracebundle;
  - generated systrace round trips through `tracequery.BuildIndex`.

Verified with:

```bash
go test ./internal/hitraceconv ./cmd ./internal/repl
```

### Batch 3F: hmtrace-Like Compatibility Matrix

Tasks:

- Port hmtrace covered and comprehensive DB fixtures to Go tests.
- Add semantic assertions for every table family listed in Batch 3D.
- Add negative extractor failure tests for missing required columns.
- Add optional golden comparison against hmtrace reference output when the local
  reference scripts are available; keep the deterministic Go semantic tests as
  the always-on guard.
- Add `.sys` parity fixture if local trace_streamer can convert representative
  no-perf sys binary input.

Exit criteria:

- Batch 3 is complete only when 3A through 3F are implemented and tests pass.
- A table family cannot be silently skipped unless coverage marks it missing or
  unsupported with a structured reason.
- Any intentional output difference from hmtrace must be documented in this file
  as a Codrax trace_query contract difference.

## Prompt, Handoff, and JSON Repair

- No new model tool-call JSON input is introduced by Batch 3.
- `TraceDBCoverage` is system output and tracebundle handoff material.
- If later `trace_query` gains input filters for DB coverage or provider
  decisions, those fields must be added to the unified JSON repair/compat layer
  in the same batch.
- Prompt/hint changes are deferred to Batch 7, but Batch 3 must already provide
  the structured data needed by those prompts: provider decisions, artifacts,
  coverage, caveats, and perf/trace clock alignment.
