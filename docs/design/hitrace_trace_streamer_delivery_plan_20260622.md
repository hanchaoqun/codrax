# HiTrace trace_streamer Delivery Plan

Date: 2026-06-22

Source design:
`docs/design/hitrace_trace_streamer_parser_redesign_20260622.md`

External reference:
https://gitcode.com/diting/hmtrace/tree/main

## Current-State Audit

Inspected current `main` before implementation.

Relevant Codrax surfaces:

- `internal/hitraceconv/convert.go`
  - Still routes production conversion through `extractStandaloneArtifacts`,
    `tryConvertProfilerContainer`, then `scanMetadata -> renderRows`.
  - `scanMetadata -> renderRows` is still needed for existing no-perf
    Harmony/Donghu `.sys` binary traces until trace_streamer DB parity is
    proven. Its failures must not be presented as the expected path for modern
    profiler `.htrace` packages.
- `internal/hitraceconv/types.go`
  - `Options` has perf adapter knobs but no trace engine, trace_streamer path,
    DB output, keep-DB, trace provider decision, or DB artifact type.
  - `Result` and `traceBundleMetadata` carry perf provider decisions but no
    trace provider decisions or DB coverage stats.
- `cmd/trace_convert.go`
  - Has `--perf-tools-status` only; no trace_streamer status or trace engine
    flags.
  - CLI result text still reports legacy missing-format/header-only counters as
    first-class output.
- `internal/repl/repl.go` and `internal/repl/messages.go`
  - `/htrace convert` calls `ConvertFile` with only input/output/flavor.
  - Usage text points users toward perf tools but not trace_streamer.
- `internal/hitraceconv/convert_test.go`
  - Many tests construct `SEGMENT_EVENTS_FORMAT` and `SEGMENT_RAW_TRACE`
    fixtures. These remain `.sys` parity guards during the DB migration, and
    can be retired only after trace_streamer DB export fully covers the same
    event families.
- `internal/tracequery`
  - Already consumes systrace/perftrace/tracebundle outputs, so the converter
    should keep exporting stable text artifacts instead of creating a new model
    tool-call schema.

Observed hmtrace model:

- Embed or locate `trace_streamer`.
- Run `trace_streamer <input> -e <output.db>`.
- Export SQLite DB tables to systrace/perfetto artifacts through table-specific
  extractors.
- Treat perf data as DB-first: separate perf DBs can be merged, and symbolized
  frames can be written into an extra table.

## Delivery Rules

- No silent production fallback after a trace-body engine starts. Pure trace
  conversion has a user-visible selector: `auto`, `trace_streamer`, or
  `builtin`. In `auto`, Codrax uses `trace_streamer`/SQL when the tool is
  discovered, uses the built-in trace-only converter only when `trace_streamer`
  is absent, and does not fall back after a SQL execution/export failure.
- Pure trace conversion is not a dual-run production path. The CLI and REPL must
  expose the same selector, and the selected provider must be recorded in
  tracebundle provenance. Parity tests may execute both engines in one test, but
  customer conversion must run exactly one trace-body engine.
- No-perf Harmony/Donghu `.sys` binary conversion remains supported through the
  explicit built-in engine until trace_streamer DB parity is proven. Once parity
  is proven, backward compatibility for the old parser is not required.
- No file-suffix or keyword intent routing for user requests. Deterministic code
  may validate selected artifacts and inspect content, but model/request routing
  remains outside converter internals.
- No prompt red-line violations: prompts may teach stable artifact semantics and
  typed fields, but must not add brittle keyword gates or prose parsing.
- No per-case patching. Each renderer/exporter must be per event family or per
  DB table class.
- Do not add model tool-call JSON burden unless a tool actually gains input
  fields. Any new tool input must enter the unified JSON repair/compat layer.
- Every batch must leave tracebundle provenance and handoff richer than before,
  not only improve terminal output.

## Performance and Memory Budget

### Provider Invocation

- `trace_streamer` execution is external-process bounded by context.
- Capture stdout/stderr with bounded buffers; preserve truncated diagnostics in
  provider caveats instead of keeping unbounded logs in memory.
- DB output is streamed to disk. No full `.htrace` load.

### DB Export

- Query per table and stream rows into bounded event buffers.
- For globally time-sorted systrace, use chunked external sort if row count grows
  beyond an in-memory threshold. Commercial completion requires a spillable
  implementation, not a test-only in-memory sorter.
- Perftrace export should stream samples and callchains in row order; avoid
  materializing full callchain maps unless the DB row count is below a bounded
  threshold.
- Emit coverage stats: table rows read, rows emitted, bytes written, elapsed time,
  peak in-memory row count when known.

### Sys Binary Parser Retirement

- Removing the raw-page parser is allowed only after the DB exporter proves
  parity for no-perf `.sys` inputs. That reduces memory pressure from raw
  event-page decoding and eliminates unsupported header-only row churn without
  dropping customer-visible capability.

## Batch Plan

### Batch 1: Trace Engine and Tool Status Skeleton

Status: delivered on 2026-06-22.

Tasks:

- Add trace engine options:
  - `TraceEngine`: `auto`, `trace_streamer`, `builtin`.
  - `TraceStreamerPath`.
  - `TraceDBOutputPath`.
  - `KeepTraceDB`.
  - `TraceStreamerSoDirs`.
- Add constants:
  - artifact type `trace_db`.
  - trace provider stages/kinds/names for `trace_streamer_db`,
    `builtin_modern_profiler`, and `builtin_sys_binary`.
- Add trace provider decision struct and include it in `Result` and
  tracebundle JSON.
- Add `BuildTraceToolStatus` or unified tool status reporting:
  - explicit path,
  - `CODRAX_TRACE_STREAMER`,
  - `PATH`,
  - known OpenHarmony/SmartPerf locations,
  - missing/install hints,
  - localization.
- Add CLI flags:
  - `--trace-engine`,
  - `--trace-streamer`,
  - `--trace-db-output`,
  - `--keep-trace-db`,
  - `--trace-streamer-so-dir`,
  - `--trace-tools-status`.
- Keep `--perf-tools-status` working and consider it an alias for the perf-only
  section.
- Add REPL usage text that tells users how to inspect trace_streamer status.
- Tests:
  - trace engine mode validation.
  - trace_streamer discovery from option/env/PATH with fake executable.
  - localized trace tool status.
  - tracebundle includes trace provider decisions without losing perf decisions.

Exit criteria:

- Delivered: no trace_streamer execution is required yet for production
  conversion.
- Delivered: users can inspect trace_streamer discovery through
  `--trace-tools-status`.
- Delivered: trace provider provenance has a stable JSON home through
  `trace_provider_decisions` before DB export lands.
- Delivered: CLI and REPL recognize the future `trace_db` artifact type.
- Verified with:

```bash
go test ./internal/hitraceconv ./cmd ./internal/repl
```

Boundary that remains for Batch 2:

- `--trace-engine=trace_streamer` is validated but intentionally fails conversion
  until provider invocation is implemented. The status output says discovery is
  present while DB provider execution will be enabled by the next batch, avoiding
  a false claim that trace_streamer conversion already runs.

### Batch 2: trace_streamer Provider Invocation

Status: delivered on 2026-06-22.

Exploration notes:

- hmtrace invokes `trace_streamer` as:

```text
trace_streamer <input> -e <output.db> [--So_dir <dir>]
```

- Codrax already has reusable primitives for this batch:
  - `traceSidecarBase` and `numberedSidecarPath` for sidecar naming.
  - `ensureOutputDoesNotExist` for no-overwrite behavior.
  - `boundedCommandOutput` for bounded external-tool diagnostics.
  - perf adapter `exec.CommandContext` patterns for cancellation.
- Batch 1 added `TraceProviderDecision`, `ArtifactTraceDB`, trace engine
  validation, and `BuildTraceToolStatus`, so Batch 2 must not create another
  provenance channel.

Tasks:

- Implement external provider runner:
  - output DB path selection,
  - no overwrite,
  - context cancellation,
  - bounded stderr/stdout,
  - optional `--So_dir` passthrough,
  - provider decisions for selected/attempted/succeeded/fallback/reason.
- In `--trace-engine=trace_streamer`, fail fast if trace_streamer is missing or
  returns non-zero.
- In default/`auto`, use trace_streamer/SQL when the tool is discovered. If the
  tool is absent and the input is pure trace, continue through the built-in
  trace-only parser. If SQL execution starts but fails or emits no query-ready
  rows, return an explicit partial tracebundle and do not fall back.
- Add DB artifact to tracebundle when `KeepTraceDB` is enabled.
- Tests:
  - fake trace_streamer writes a DB file and records args.
  - explicit mode fail-fast.
  - default/auto mode uses built-in for pure trace only when trace_streamer is
    not discovered.
  - default/auto mode records SQL execution failure without choosing a built-in
    fallback.
  - DB cleanup vs keep behavior.

Detailed implementation checklist:

- Add `trace_streamer_provider.go`.
- Add DB output path derivation:
  - explicit `TraceDBOutputPath` wins;
  - otherwise derive from `traceSidecarBase(input, output)` plus `.trace.db`;
  - temporary DBs use a temp dir when `KeepTraceDB=false`.
- Add provider command builder:
  - args must be `<input> -e <db>`.
  - append `--So_dir <dir>` once for each configured `TraceStreamerSoDirs`.
  - do not shell-concatenate paths.
- Add runner behavior:
  - validate executable with the existing trace tool discovery result;
  - fail before running if selected `trace_streamer` is unavailable;
  - use `exec.CommandContext`;
  - capture combined output through the same bounded diagnostic policy as perf
    adapters;
  - verify the DB file exists and is non-empty after success.
- Add conversion selection:
  - `--trace-engine=trace_streamer`: run provider or hard fail.
  - default/`--trace-engine=auto`: select trace_streamer/SQL when discovered.
    If trace_streamer is absent and the input is pure trace, record the skipped
    SQL provider decision and run the built-in trace-only parser. If SQL emits
    no query-ready rows after execution starts, record a trace provider decision
    and return an explicit partial tracebundle instead of changing engines.
  - `--trace-engine=builtin`: skip trace_streamer provider and run the explicit
    built-in trace-only parser.
- Add artifact behavior:
  - when `KeepTraceDB=true`, retain the DB as `ArtifactTraceDB`.
  - when `KeepTraceDB=false`, remove temporary DB after DB export is implemented;
    until Batch 3, do not delete a successful explicit trace_streamer DB because
    it is the only produced trace artifact.
- Add test fixtures:
  - fake shell trace_streamer script writing `sqlite-like` bytes and an args log;
  - fake failing trace_streamer script with stderr;
  - explicit missing tool;
  - auto missing-tool pure-trace built-in fallback;
  - default SQL execution failure without built-in profiler fallback;
  - tracebundle contains trace provider decisions and DB artifact when kept.

Performance/memory notes for this batch:

- No full `.htrace` or DB load is allowed.
- External process output remains bounded.
- DB artifact validation uses `os.Stat`; no DB parsing yet.

Exit criteria:

- Delivered: a modern `.htrace` can be normalized into a DB artifact through an
  external trace_streamer path in tests.
- Delivered: explicit `--trace-engine=trace_streamer` fails fast when the tool is
  missing or the command fails.
- Delivered: `auto` selects trace_streamer/SQL when discovered, falls back to
  built-in only for pure trace when trace_streamer is absent, records provider
  decisions, and does not fall through after SQL execution/export failure.
- Delivered: `KeepTraceDB` and explicit `TraceDBOutputPath` retain
  `ArtifactTraceDB` in tracebundle; temporary auto DBs are cleaned when not kept.
- Verified with:

```bash
go test ./internal/hitraceconv ./cmd ./internal/repl
```

Boundary closed by Batch 3:

- The DB is now parsed by Codrax. Explicit trace_streamer conversion emits
  systrace when rows are available, preserves the DB when requested, and writes
  trace DB coverage into tracebundle. No-row DBs remain explicit partial bundles
  with coverage and do not claim trace_query readiness.

### Batch 3: DB Exporter Full Compatibility

Status: delivered on 2026-06-22.

Detailed execution plan:
`docs/design/hitrace_trace_streamer_batch3_full_compat_plan_20260622.md`

Exploration notes:

- Current Go module has no SQLite dependency and no existing `database/sql`
  usage for SQLite.
- hmtrace uses Rust `rusqlite`; Codrax needs a Go equivalent that preserves the
  single-binary UX.
- Chosen dependency strategy:
  - use pure-Go `modernc.org/sqlite` behind `database/sql`;
  - do not use `github.com/mattn/go-sqlite3`, because CGO would complicate
    static/release builds;
  - do not use external `sqlite3` CLI except as a manual diagnostic outside the
    product path.
- Expected tradeoff:
  - larger module graph and binary size;
  - better portability and deterministic tests;
  - no runtime dependency on system SQLite.
- Full exporter follows hmtrace table classes, but emits Codrax query-native
  systrace text:
  - `sched_slice + thread` -> `sched_switch`;
  - `instant + thread` -> `sched_wakeup` and trace marker rows when present;
  - `irq` -> IRQ/softirq entry/exit;
  - CPU/clock/frame families after schema helpers are in place.
- hmtrace compatibility tests reviewed on 2026-06-22:
  - `/tmp/codrax-ref-hmtrace/tests/golden_diff.rs` compares Rust DB exporter
    against the Python reference for covered, raw, and comprehensive fixtures.
  - `/tmp/codrax-ref-hmtrace/reference/python/tests/test_db2systrace.py`
    covers html/raw output, process/thread registration, process-name fallback,
    CPU count derivation, async spans, `sched_wakeup` CPU/target metadata,
    IRQ/softirq args, process counters, and extractor failure behavior.
  - `/tmp/codrax-ref-hmtrace/tests/export_format.rs` covers perfetto export with
    process tree, CPU frequency, running states, perf samples/callchains,
    symbolized frames, and merged perf spans across running gaps.
  - Codrax must construct analogous Go fixtures for each table family and compare
    semantic output, not only assert non-empty files.

Tasks:

- Choose SQLite strategy and document dependency tradeoff before code:
  - pure-Go SQLite is preferred for single-binary portability;
  - external `sqlite3` CLI is only acceptable as a temporary test helper.
- Implement schema introspection helpers:
  - table exists,
  - column exists,
  - typed nullable reads,
  - coverage counters.
- Export event families with hmtrace-compatible fixture coverage:
  - `thread`/process dump,
  - `sched_slice` -> `sched_switch`,
  - `instant` -> `sched_wakeup`/trace markers,
  - `irq`,
  - cpu idle/frequency,
  - clock rates,
  - frame slices.
- Extend the family list to match hmtrace's comprehensive fixture before
  declaring Batch 3 complete:
  - `callstack`, `thread_state`, `raw`, `data_dict`, `args`,
  - `measure`, `cpu_measure_filter`, `measure_filter`,
    `process_measure_filter`, `process_measure`,
  - `dma_fence`, `network`, `diskio`, `cpu_usage`, `live_process`,
  - `log`, `syscall`, `task_pool`, `app_startup`, `static_initalize`,
    `native_hook`, `hisys_all_event`, and `xpower_measure`.
- Write systrace rows through existing official-compatible render shape.
- Tests:
  - synthetic DB per table family mirroring hmtrace reference tests.
  - full comprehensive fixture matching hmtrace table coverage.
  - raw systrace output and HTML-wrapped systrace output where Codrax exposes the
    same mode.
  - process/thread registration and rename metadata.
  - wakeup CPU/target metadata from `instant`, `raw`, and next-run `sched_slice`.
  - IRQ/softirq args via `data_dict`/`args`.
  - negative extractor failure behavior.
  - systrace text round-trip through `tracequery.BuildIndex`.
  - coverage stats in tracebundle.

Detailed implementation checklist:

- Add `internal/hitraceconv/streamerdb` or equivalent internal package.
- Add DB abstraction:
  - `Open(path)` using `database/sql`;
  - `TableExists`;
  - `ColumnExists`;
  - nullable string/int helpers;
  - coverage collector.
- Add systrace writer:
  - reuse the existing `writeRows`/`renderedRow` path where possible;
  - render seconds with the same microsecond precision as current converter;
  - implement bounded global ordering with deterministic spill-to-disk when row
    count exceeds memory limits.
- Add scheduler exporter:
  - read `sched_slice(ts,dur,cpu,end_state,priority,itid)`;
  - join `thread(itid,tid,name)`;
  - produce the same `sched_switch` body as trace_query already parses.
- Add wakeup/trace-marker exporter:
  - read `instant(ts,name,ref,ref_type,wakeup_from)`;
  - support `sched_wakeup` and `sched_waking`;
  - support trace marker B/E/C/S/F if DB rows expose marker payloads through
    stable name/arg columns.
- Add IRQ exporter:
  - read `irq(ts,dur,callid,cat,name,argsetid)`;
  - use arg tables only through a generic argset loader, not event-specific
    string matching.
- Add tracebundle coverage:
  - table found/missing;
  - rows read;
  - rows emitted;
  - skipped reason.
- Keep memory bounded:
  - stream DB reads per table and emit rows into a bounded sorter;
  - spill sorted chunks to disk above the configured threshold;
  - merge chunks by timestamp/sequence without loading all rows;
  - record peak buffered rows, spill count, and elapsed time in coverage stats.

Exit criteria:

- Scheduler, wakeup, IRQ/softirq, trace marker, counter, frame, log/syscall, IO,
  hisys/xpower, and process/thread metadata DB output are covered by hmtrace-like
  fixtures and queryable by trace_query where relevant.
- The comprehensive fixture passes semantic parity against the hmtrace reference
  shape; any intentional Codrax output difference is documented as a stable
  query-contract difference, not an unreviewed gap.
- Large DB ordering is bounded by spill-to-disk behavior.
- Add the first `.sys` DB-parity fixture if a local trace_streamer executable is
  available; otherwise keep the explicit built-in sys binary round-trip guard.

### Batch 4: DB Perf Exporter

Status: folded into Batch 3F and delivered on 2026-06-22.

Tasks:

- Delivered export of `perf_sample`, `perf_callchain`, `perf_thread`, `perf_report`,
  `perf_files`, `data_dict`, and optional `hmtrace_perf_symbolized_frame`.
- Delivered symbolized display names preference when present.
- Delivered timestamp source preference:
  - `timestamp_trace` when present,
  - `timeStamp`/`timestamp` with caveat otherwise.
- Delivered Codrax query-native `perf_sample:` rows in the systrace output, plus
  hmtrace-style `hiperf:` trace marker spans/counters for timeline visibility.
  Codrax intentionally does not create a separate `.perftrace` sidecar for DB
  perf rows because the generated systrace is already trace_query-ready and
  tracebundle carries the provider/coverage provenance.
- Delivered perf clock fields on DB perf rows:
  `clock=trace_streamer_db clock_confidence=calibrated`.
- Delivered tests:
  - synthetic DB sample with callchain and DSO;
  - symbolized-frame preference;
  - trace_query `perf_samples` round-trip from generated systrace;
  - hmtrace comprehensive schema fixture.

Exit criteria:

- Running/CPU root-cause analysis can use perf samples from trace_streamer DB.

### Batch 5: Built-in Modern Parser

Status: delivered on 2026-06-23 for the supported modern-profiler and
trace+perf routing contract.

Detailed execution plan:
`docs/design/hitrace_builtin_modern_parser_batch5_full_compat_plan_20260623.md`

Compatibility bar:

- This batch is not an MVP text exporter. It is complete only when the built-in
  modern parser preserves the same trace_query-facing contract as the
  trace_streamer DB exporter for supported event families.
- `ftrace-plugin` structured payloads cannot remain summary-only for event
  families consumed by root-cause analysis. Unsupported families must be
  represented as typed coverage/partial results, not header-only systrace rows.
- Text/session payload extraction must use bounded ordering through the shared
  spillable row sink.
- Tests must mirror hmtrace-style comprehensive fixtures and verify
  trace_query round-trip semantics, not only non-empty output.

Tasks:

- Refactor current `tryConvertProfilerContainer` into the modern builtin lane.
- Parse profiler/session/package metadata and ftrace plugin metadata as first
  class inputs.
- Add targeted structured renderers only for event families already consumed by
  trace_query.
- Normalize fields to the same schema as DB exporter.
- Record coverage caveats instead of header-only unknown rows.

Exit criteria:

- Builtin conversion handles modern text/profiler payloads without legacy
  segment parser fallback.
- Modern parser tracebundle output exposes machine-readable coverage, provider
  decisions, sorter stats, and partial-vs-query-ready state.
- Structured ftrace decoding/rendering covers the DB exporter event families
  that `trace_query` uses for scheduler, wakeup, CPU, binder, IRQ, IO, trace
  marker, frame, and perf-informed running analysis.
- Delivered guard: trace+perf inputs no longer use the legacy built-in trace
  body parser as a fallback. They use the SQL/trace_streamer lane when
  available, otherwise produce explicit partial artifacts and provider decisions.
- Delivered guard: pure no-perf trace inputs keep two selectable engines while
  sys-binary DB parity remains open; auto may use built-in only when
  trace_streamer is absent, and SQL execution/export failure never silently
  changes to the built-in engine.
- Delivered guard: SQL-generated trace text is cross-validated through
  `trace_query` and recorded as `trace_cross_validation` coverage.

### Batch 6: Sys Binary Parity Gate and Retirement

Status: in progress, with Batch 6A delivered on 2026-06-23.

#### Batch 6A: No-Perf Sys Wakeup Parity Guard

Status: delivered on 2026-06-23.

Scope:

- Added the first executable no-perf `.sys` parity guard comparing two explicit
  engine choices:
  - the protected built-in sys-binary conversion lane;
  - the explicit `trace_streamer`/SQLite conversion lane.
- The guard uses equivalent scheduler data and verifies that both converted
  systrace outputs round-trip through `trace_query` with the same
  `sched_wakeup` semantics:
  - wakee pid/name/priority,
  - event CPU,
  - `target_cpu`,
  - provider provenance,
  - SQL `scheduler/instant` export coverage,
  - generated-trace cross-validation coverage.
- This protects the shared evidence layer after either customer entry mode:
  - explicit attachment (`--htrace`, `/htrace`, `--log`, `/log`);
  - no attachment, with one or more readable artifact paths named in the user
    request.

Remaining gap:

- This is a scheduler wakeup parity slice, not full sys-binary retirement.
- Representative no-perf Harmony/Donghu captures still need parity checks for
  binder, IRQ, IO, trace marker, frame, CPU frequency, and constraint families
  before deleting or isolating the old sys-binary parser.

#### Batch 6B: Root-Cause Evidence Parity Matrix

Status: delivered first executable slice on 2026-06-23.

Exploration notes:

- Local hmtrace reference at `/tmp/codrax-ref-hmtrace` exposes a native extractor
  registry covering `thread`, `callstack`, `perf_sample`, `sched_slice`,
  `instant`, `irq`, `cpu_measure_filter`, `measure_filter`, `process_measure`,
  `frame_slice`, `dma_fence`, `network`, `diskio`, `cpu_usage`,
  `live_process`, `log`, `syscall`, `task_pool`, `app_startup`,
  `static_initalize`, `native_hook`, `hisys_all_event`, and `xpower_measure`.
- Codrax SQL exporter already has table-family exporters for the hmtrace
  registry above and writes through the shared spillable row sink, with coverage
  rows and trace_query cross-validation.
- Codrax built-in sys binary renderer supports many official ftrace event
  families through `renderOfficialOpenHarmonyBody`, including scheduler,
  CPU/clock, IRQ/softirq/IPI, trace marker, block/file/storage IO, UFS/MMC/SCSI,
  DMA fence, memory/rss, workqueue, thermal, and regulator families.
- Batch 6A only compared one `sched_wakeup`. Batch 6B must prove a class-level
  matrix for the root-cause evidence that model answers depend on, not a
  one-case wakeup guard.

Tasks:

- Add a reusable parity harness that converts both explicit engines for test
  validation only:
  - a synthetic no-perf sys-binary fixture through the guarded built-in lane;
  - an equivalent trace_streamer SQLite fixture through the SQL lane.
- Compare the resulting `trace_query` events semantically rather than comparing
  raw lines or relying on file suffixes.
- First matrix slice:
  - `sched_switch` for sleep/runnable/running state reconstruction;
  - `sched_wakeup` / `sched_waking` for wakeup chains and priority relation;
  - `cpu_frequency`, `cpu_idle`, and `clock_set_rate` for running supply and
    frequency diagnostics;
  - `tracing_mark_write` B/E/C for span pairing and user-supplied span windows;
  - `irq_handler_*` / `softirq_*` for interrupt pressure context;
  - frame-like B/E spans exported from DB `frame_slice`.
- For each family, assert:
  - converted systrace round-trips through `trace_query`;
  - key typed fields are preserved;
  - SQL coverage records the relevant table/family;
  - provider decisions distinguish built-in sys from trace_streamer DB.
- Document that this cross-engine parity harness is not a production fallback
  mechanism. Production pure-trace conversion runs only the selected engine,
  with SQL as the default.
- Keep performance/memory guards:
  - fixtures must use the production spillable row sink path;
  - tests must not introduce unbounded in-memory comparison of full trace text;
  - compare bounded event projections derived from `trace_query` events.
- Keep prompt/JSON contract unchanged:
  - this batch adds no model tool-call input fields;
  - no JSON repair aliases are required unless a future tool input changes;
  - parity/provenance remains system output consumed through tracebundle and
    `trace_query` caveats.

Implementation checklist for the first executable slice:

- Build a compact synthetic sys-binary fixture with one raw page and multiple
  official-format events:
  - `sched_switch` with `prev/next` fields for state reconstruction;
  - `sched_wakeup` and `sched_waking` for wakeup-chain anchoring;
  - `cpu_frequency`, `cpu_idle`, and `clock_set_rate` for running supply;
  - `tracing_mark_write` B/E/C for span pairing and counters;
  - `irq_handler_entry/exit` and `softirq_entry/exit` for interrupt context.
- Build an equivalent trace_streamer SQLite fixture using hmtrace-style tables:
  - `process`, `thread`, `sched_slice`, `instant`, `raw`, `irq`, `args`,
    `data_dict`, `measure`, `cpu_measure_filter`, `measure_filter`,
    `callstack`, and `frame_slice`.
- Run both fixtures through `ConvertFile` with explicit engine selection:
  - built-in lane: `TraceEngine=builtin`;
  - SQL lane: `TraceEngine=trace_streamer` with a fake trace_streamer copying
    the fixture DB.
- Compare bounded semantic projections from `trace_query.BuildIndex`:
  - counts by `EventType`;
  - scheduler PIDs/priorities/states;
  - wakeup comm/pid/priority/target CPU;
  - CPU/clock names, CPU IDs, and frequencies/states;
  - trace span/counter action/name/value fields;
  - IRQ/softirq IDs/names and CPU placement.
- Assert provenance and coverage:
  - built-in result has `codrax_builtin_sys_binary`;
  - SQL result has `trace_streamer_db` and trace DB artifact;
  - SQL coverage emits `scheduler/sched_slice`, `scheduler/instant`,
    `irq/irq`, `counter/measure`, `counter/measure_filter`,
    `slice/callstack`, and `slice/frame_slice`.
- Do not make this a production dual-run path. This harness is an offline
  commercial parity guard only.

Out of scope for Batch 6B and queued for Batch 6C+:

- Binder parity, inode/page-cache/storage IO parity, DMA fence, workqueue, and
  representative customer `.sys` captures. Those families remain required
  before retiring the built-in sys parser; they are not hidden by the first
  matrix slice.

Delivered notes:

- Added `TestConvertFileNoPerfTraceRootCauseEvidenceParityMatrix`, which runs
  the same compact no-perf root-cause evidence fixture through explicit
  `builtin` and explicit `trace_streamer` engines, then validates the bounded
  semantic projection that `trace_query` exposes to downstream answer stages.
- The first executable slice now covers scheduler state reconstruction,
  wakeup/waking anchors, CPU frequency/idle, clock rates, B/E/C trace markers,
  frame-like spans, IRQ, softirq, provider decisions, trace DB artifacts, and
  SQL coverage records.
- The test found and fixed two reusable gaps:
  - `irqName` no longer misreads offset `0` as an event-id-backed string when an
    IRQ format uses `char name[N]`;
  - SQL-exported integer tracepoint counters now use integer text for
    `cpu_idle`, `cpu_frequency`, `cpu_frequency_limits`, and
    `clock_set_rate`, so `trace_query` consumes the same typed fields as the
    built-in path.
- Verification for this slice:
  - `go test ./internal/hitraceconv -run TestConvertFileNoPerfTraceRootCauseEvidenceParityMatrix -count=1 -v`
  - `go test ./internal/hitraceconv ./internal/tracequery`

Exit criteria:

- The first root-cause evidence matrix is covered by an executable parity test.
- Any remaining event-family gaps are documented as Batch 6C+ work, not hidden
  behind the single wakeup parity guard.
- The explicit no-perf sys lane remains guarded until representative captures
  prove all required families, but the SQL lane has explicit evidence for the
  core timing and CPU-supply families when trace_streamer is available.

#### Batch 6C: Raw Ftrace Root-Cause Evidence Parity

Status: in progress on 2026-06-23; raw-ftrace SQL exporter and cross-engine
parity guard slices delivered, with representative customer `.sys` fixtures
still pending.

Exploration notes:

- Batch 6B proves the SQL/default path and explicit built-in path both
  round-trip the timing, wakeup, CPU-supply, trace-marker, frame, IRQ, and
  softirq evidence that `trace_query` consumes.
- The remaining high-value root-cause families are not all represented by
  hmtrace's high-level native extractors. The local hmtrace registry covers
  `sched_slice`, `instant`, `irq`, `measure`, `measure_filter`, `process_measure`,
  `frame_slice`, `dma_fence`, `network`, `diskio`, `cpu_usage`, `live_process`,
  `log`, `syscall`, `task_pool`, `app_startup`, `static_initalize`,
  `native_hook`, `hisys_all_event`, and `xpower_measure`, but does not expose a
  dedicated binder or inode/file-IO extractor.
- Codrax already has downstream consumers for these families:
  - binder: `trace_query` parses binder transactions, receives, replies, locks,
    allocation buffers, and chain-correlated binder waits;
  - inode/file IO: `trace_query` aggregates `file_io_by_inode`,
    `page_cache_by_inode`, `storage_latency_by_layer`, `io_pressure_summary`,
    and `block_io_by_inode`;
  - workqueue and DMA fence: `trace_query` computes workqueue activity and
    classifies `dma_fence_*` events.
- Codrax built-in sys rendering already has stable event renderers for many of
  these ftrace families (`binder_*`, `block_*`, `mm_filemap_*`,
  `android_fs_*`, `f2fs_*`, `scsi_*`, `mmc_*`, `ufshcd_*`, `workqueue_*`,
  and `dma_fence_*`), but the SQL exporter currently emits only high-level DB
  tables plus selected scheduler/IRQ rows. A trace_streamer DB that only keeps
  low-level ftrace rows in `raw` + `args` can therefore lose queryable binder,
  inode IO, page-cache, storage, and workqueue evidence.

Design direction:

- Add a schema-introspecting raw-ftrace exporter to the SQL path, not a
  production dual-run against the built-in sys parser.
- Treat trace event names and DB arg keys as structured trace data. Do not use
  user-prose, model-output prose, filename suffixes, or question keywords to
  route analysis or decide which events matter.
- Reuse existing primitives:
  - `loadArgsets` for key/value argument recovery;
  - `traceDBThreadIndex` and running intervals for task/tid/tgid/cpu context;
  - the spillable `traceDBRowSink` for bounded memory;
  - existing trace_query parsers and aggregators as the end-to-end contract.
- Implement renderer families by class:
  - binder IPC: `binder_transaction`, `binder_transaction_received`,
    `binder_transaction_alloc_buf`, `binder_transaction_reply`,
    `binder_transaction_lock`, `binder_transaction_locked`,
    `binder_transaction_unlock`;
  - block/storage: `block_rq_issue`, `block_rq_insert`, `block_rq_complete`,
    `block_bio_remap`, `ufshcd_*`, `mmc_request_*`, `scsi_dispatch_cmd_*`;
  - inode/file IO: `android_fs_dataread_*`, `android_fs_datawrite_*`,
    `f2fs_direct_IO_*`, `f2fs_sync_file_*`;
  - page cache and memory-backed file activity:
    `mm_filemap_add_to_page_cache`, `mm_filemap_delete_from_page_cache`,
    `filemap_set_wb_err`;
  - workqueue: `workqueue_execute_start`, `workqueue_execute_end`, and
    `workqueue_execute` variants with work/function identity;
  - DMA fence raw ftrace events when present in `raw`, while preserving the
    existing hmtrace-compatible high-level `dma_fence` table exporter.
- Normalize emitted SQL systrace rows to the same stable field names already
  consumed by `trace_query`:
  - binder fields: `transaction`, `dest_proc`, `dest_thread`, `reply`, `flags`,
    `code`, `data_size`, `offsets_size`, `extra_buffers_size`, `debug_id`;
  - file IO fields: `dev`, `ino`, `entry_name`, `offset`, `bytes`/`len`, `rw`,
    `ret`, `latency_us`;
  - storage fields: `dev`, `lba`/`sector`, `len`, `opcode`, `tag`, `ret`,
    `latency_us`;
  - workqueue fields: `work`, `function`;
  - DMA fence fields: `driver`, `timeline`, `context`, `seqno`.
- Emit bounded coverage rows for every raw-ftrace class:
  - `raw_ftrace/binder`,
  - `raw_ftrace/block_storage`,
  - `raw_ftrace/file_io`,
  - `raw_ftrace/page_cache`,
  - `raw_ftrace/workqueue`,
  - `raw_ftrace/dma_fence`.
  Missing columns or unsupported raw schemas must be visible in
  `TraceDBCoverage` caveats instead of silently producing a queryable-looking
  partial trace.

Implementation checklist:

- Add a raw event loader that introspects the `raw` table for common columns:
  - required: `ts`, `name`;
  - optional: `cpu`, `itid`, `callid`, `tid`, `pid`, `argset`, `argsetid`,
    `arg_set_id`.
- Resolve thread context in priority order:
  - raw `itid`/`callid` through `traceDBThreadIndex`;
  - raw `tid`/`pid` columns when present;
  - binder/event args only as typed fallback fields, not as user-intent signals.
- Implement class renderers that accept a normalized `traceDBRawEvent` and
  `map[string]traceDBValue`, then output systrace rows with existing stable
  field names.
- Keep the renderer allowlist based on structured ftrace event families, so the
  exporter does not become a generic unknown-event dump.
- Add SQL coverage counters for rows read, rows emitted, missing required
  columns, unsupported names, and per-class skip reasons.
- Add a parity test separate from 6B:
  - synthetic built-in sys-binary fixture with binder, file IO, page cache,
    storage latency, workqueue, and DMA fence raw events;
  - equivalent trace_streamer SQLite fixture using `raw`, `args`, `data_dict`,
    `thread`, `process`, and `thread_state`;
  - run explicit `builtin` and explicit `trace_streamer` engines;
  - compare bounded `trace_query` semantic output:
    `IPCEdges`/binder waits, `file_io_by_inode`, `page_cache_by_inode`,
    `storage_latency_by_layer`, `io_pressure_summary`, `block_io_by_inode`,
    `workqueue_activity`, and `EventDMAFence`.
- Add package tests for raw schema drift:
  - missing `argset` columns produces explicit coverage skip;
  - missing optional thread columns still emits rows with conservative task
    context;
  - unknown raw event names are counted/skipped without header-only output.
- Keep JSON/prompt contracts stable:
  - no new model tool-call input fields are introduced in Batch 6C;
  - no JSON repair aliases are needed unless `trace_query` input schema changes;
  - new evidence appears in query results and tracebundle coverage caveats.
- Performance and memory requirements:
  - stream DB rows ordered by timestamp;
  - load argsets once through the existing bounded resolver;
  - never compare full systrace text in tests;
  - keep parity assertions as bounded semantic projections from `trace_query`.

Exit criteria:

- SQL/default pure-trace conversion emits queryable binder, inode/file IO,
  page-cache, storage latency, workqueue, and DMA-fence evidence from
  trace_streamer `raw` + `args` rows when high-level tables are absent.
- Explicit built-in pure-trace conversion and SQL/default conversion are
  guarded by a semantic parity test for these root-cause families.
- `trace_query` window stats and root-cause supporting evidence can consume the
  SQL-generated rows without prompt hacks or new tool-call JSON burden.
- Missing DB support is transparent through bounded coverage caveats, not
  silent evidence loss.

Delivered slice on 2026-06-23:

- Added a schema-introspecting SQL raw-ftrace exporter that reads `raw` plus
  `args`/`data_dict`, uses structured ftrace event names for class routing, and
  emits stable systrace rows for binder, block/storage, inode/file IO,
  page-cache, workqueue, and DMA-fence families.
- Added bounded `TraceDBCoverage` rows for `raw_ftrace/raw` and each emitted
  class (`binder`, `block_storage`, `file_io`, `page_cache`, `workqueue`,
  `dma_fence`), plus explicit skip coverage for missing `argset` or
  `args/data_dict` dependencies.
- Added tests proving SQL raw-ftrace rows round-trip through `trace_query` into:
  binder IPC edges, `file_io_by_inode`, `page_cache_by_inode`,
  `storage_latency_by_layer`, `io_pressure_summary`, `workqueue_activity`, and
  `EventDMAFence`.
- This slice does not yet close Batch 6C because the explicit
  built-in-vs-SQL raw-ftrace parity fixture and representative customer `.sys`
  validation are still required.

Next slice: Batch 6C2 raw-ftrace cross-engine parity guard.

Status: delivered on 2026-06-23.

Reference audit:

- The local hmtrace reference checkout at `/tmp/codrax-ref-hmtrace` keeps the
  production path centered on `trace_streamer` SQLite output, then runs native
  extractors over DB tables. Its Rust/Python golden tests compare raw systrace
  output from DB fixtures instead of re-parsing the original binary trace body.
- Codrax should follow the same class-level contract for no-perf pure trace:
  production conversion uses exactly the user-selected engine, with SQL as the
  default, while tests may run both explicit engines against equivalent
  fixtures to prove commercial parity.
- The SQL raw-ftrace exporter delivered in the previous slice already proves
  DB rows alone can feed `trace_query`. The missing proof is that the same
  evidence classes are semantically equivalent to the guarded built-in sys
  binary path for pure trace captures.

Tasks:

- Add `TestConvertFileNoPerfTraceRawFtraceRootCauseParityMatrix` beside the
  existing scheduler/CPU parity matrix.
- Build one synthetic no-perf sys-binary fixture through existing official
  event-format helpers, covering these classes:
  - binder transaction/receive/allocation buffer;
  - android_fs start/end inode IO;
  - page-cache add/delete by inode;
  - block request issue/complete and SCSI start/done;
  - workqueue execute start/end;
  - DMA fence signaled.
- Build an equivalent trace_streamer SQLite fixture using `raw`, `args`,
  `data_dict`, `thread`, `process`, and `thread_state`; do not depend on
  filename suffixes or user-prose signals.
- Run explicit `TraceEngine=builtin` and explicit `TraceEngine=trace_streamer`
  through `ConvertFile` with the existing fake trace_streamer fixture copier.
- Assert bounded semantic projections from `trace_query`, not full text:
  - binder IPC edge transaction, peer thread, and flags;
  - `file_io_by_inode` bytes and inode identity;
  - `page_cache_by_inode` churn for the same inode;
  - `storage_latency_by_layer` paired latency for block/SCSI style rows;
  - `io_pressure_summary` top inode;
  - `workqueue_activity` paired work item;
  - queryable DMA-fence event.
- Assert provider and coverage provenance:
  - built-in path reports `codrax_builtin_sys_binary`;
  - SQL path reports `trace_streamer_db`, a trace DB artifact, raw-ftrace class
    coverage, and trace_query cross-validation coverage.
- Keep performance/memory properties:
  - write fixtures through the production conversion path and spillable row
    sink;
  - compare bounded semantic summaries only;
  - keep raw argset loading shared with the production resolver.

Exit criteria for Batch 6C2:

- The same raw-ftrace root-cause classes are protected by a cross-engine
  semantic parity test for pure no-perf trace.
- The parity guard remains test-only and does not introduce a production
  dual-run or automatic fallback.
- No model tool-call input fields are added; no JSON repair aliases are needed.
- Any failures expose structured coverage/provenance gaps rather than prompt
  guidance or keyword patches.

Delivered Batch 6C2 on 2026-06-23:

- Added `TestConvertFileNoPerfTraceRawFtraceRootCauseParityMatrix`, which runs
  equivalent raw-ftrace root-cause evidence through explicit `builtin` and
  explicit `trace_streamer` engines, then compares bounded semantic summaries
  from `trace_query`.
- The parity guard covers binder IPC, inode file IO, page-cache churn,
  block/SCSI storage latency, IO pressure summary, workqueue pairing, and
  DMA-fence events.
- Normalized the built-in sys renderer output for shared trace_query-facing
  fields:
  - binder `flags` and `code` now render as hexadecimal values, matching SQL
    raw-ftrace output and existing binder query expectations;
  - workqueue events now render as `work=... function=...`, matching the
    stable field schema already emitted by the SQL path.
- The guard is test-only. Production pure-trace conversion still uses exactly
  the user-selected engine, defaulting to SQL, and trace+perf remains SQL-only.
- This does not yet close Batch 6 because representative customer `.sys`
  captures still need conversion and trace_query round-trip validation.

Verification for Batch 6C2:

- `go test ./internal/hitraceconv -run TestConvertFileNoPerfTraceRawFtraceRootCauseParityMatrix -count=1 -v`
- `go test ./internal/hitraceconv ./internal/tracequery`

Next slice: Batch 6C3 representative capture closure gate.

Status: in progress on 2026-06-23; manifest/test helper slice delivered,
with a redistributable real `.sys` fixture still pending.

Current resource audit:

- The local repository and `/Users/han/opt` currently expose synthetic sys
  fixtures and text systrace artifacts, but no committed or workspace-local
  real no-perf `.sys` capture that can be used as a stable conversion fixture.
- `/Users/han/opt/customlogs/xxx_all.systrace` is a valuable customer-scale
  Donghu text trace and is already covered by trace_query eval cases such as
  `trace_query_donghu_real_frame_multicausal`,
  `trace_query_donghu_real_short_runnable`, and request-named path cases.
- Those text-trace evals protect downstream root-cause behavior and path
  routing, but they cannot prove `trace_streamer` DB conversion parity for
  binary no-perf `.sys` captures.
- The synthetic built-in-vs-SQL parity guards delivered in Batch 6A/6B/6C2 are
  necessary but not sufficient evidence to delete or isolate the old sys binary
  parser from production.

Tasks:

- Add a small representative-capture manifest document or testdata README that
  records required evidence for any future real `.sys` fixture:
  - capture provenance and redistribution status;
  - whether it is no-perf pure trace or trace+perf;
  - expected conversion engine (`trace_streamer` default, built-in explicit
    only for pure trace);
  - minimum trace_query views to verify after conversion;
  - expected coverage families and caveats.
- Add a deterministic test helper for future representative `.sys` fixtures
  that can run:
  - explicit SQL conversion;
  - optional explicit built-in conversion only for no-perf pure trace;
  - trace_query round-trip semantic projections;
  - bounded coverage/provenance assertions.
- Keep the helper fixture-driven and disabled only by absence of an explicitly
  committed fixture. It must not look in `/Users/han/opt/customlogs` or any
  developer-local absolute path during normal `go test`.
- Keep existing Donghu text systrace evals as downstream guards and document
  that they do not satisfy the converter retirement gate.
- After at least one redistributable real no-perf `.sys` fixture passes SQL
  conversion and trace_query semantic parity, decide whether to:
  - retire production legacy sys parsing, or
  - keep it as an explicit built-in lane with documented `TraceDBCoverage`
    gaps.

Exit criteria for Batch 6C3:

- Future representative `.sys` captures have a clear manifest, helper, and
  required assertion matrix.
- Existing customer text trace evals remain classified as downstream tracequery
  validation, not converter parity proof.
- Batch 6 remains open until a real redistributable no-perf `.sys` fixture is
  available and passes the helper.

Delivered Batch 6C3 slice on 2026-06-23:

- Added `internal/hitraceconv/testdata/representative_sys_traces/README.md`
  to define the required manifest shape for future redistributable `.sys`
  fixtures, including provenance, trace kind, SQL DB sidecar, coverage, and
  trace_query event expectations.
- Added `TestRepresentativeSysTraceFixtures`, a hermetic fixture gate that
  consumes only committed relative manifest paths under
  `internal/hitraceconv/testdata/representative_sys_traces`.
- The helper runs SQL conversion through a deterministic trace DB sidecar when
  present, supports explicit real trace_streamer validation through
  `CODRAX_REPRESENTATIVE_TRACE_STREAMER` when no sidecar is committed, and runs
  built-in parity only for `trace_kind=no_perf_sys` fixtures that request it.
- With no committed real fixture, the test skips loudly and keeps Batch 6C3
  open. It does not scan `/Users/han/opt/customlogs` or any developer-local
  absolute path.

Verification for Batch 6C3 slice:

- `go test ./internal/hitraceconv -run TestRepresentativeSysTraceFixtures -count=1 -v`
- `go test ./internal/hitraceconv`

Verification for delivered slice:

- `go test ./internal/hitraceconv -run 'TestExportTraceDBRawFtrace' -count=1 -v`
- `go test ./internal/hitraceconv ./internal/tracequery`

Overall Batch 6 remaining tasks:

- Build parity cases comparing explicit built-in sys binary output with
  trace_streamer DB export for representative no-perf Harmony/Donghu captures.
- Verify both outputs round-trip through `trace_query` and preserve scheduler,
  wakeup, CPU, binder, IRQ, IO, trace marker, and frame evidence.
- If parity is complete, remove production call to `scanMetadata` and delete or
  isolate:
  - `scanMetadata`,
  - `parseEventFormats`,
  - raw page event-id rendering,
  - header-only unknown event output.
- If parity is incomplete, keep the sys binary parser as an explicit guarded
  production lane and document missing DB event families in `TraceDBCoverage`
  caveats.
- Rewrite old tests into DB/profiler fixtures only after parity is complete.
- Update CLI/REPL wording so sys binary counters are transparent and not
  confused with modern profiler coverage.

Exit criteria:

- No-perf `.sys` conversion is either default DB-backed with full parity, or
  explicitly selected through the built-in sys binary parser with provenance and
  tests.

### Batch 7: Prompt, Handoff, JSON Repair, and UX Closure

Status: delivered on 2026-06-23 through Batch 7A, Batch 7B, and Batch 7C.

#### Batch 7A: Tracebundle Provenance Handoff

Status: delivered on 2026-06-23.

Gap:

- The converter now writes `trace_provider_decisions`, `trace_db_coverage`, and
  `trace_coverage` into `.tracebundle.json`.
- `trace_query` currently surfaces perf provider decisions and perf clock
  alignment from tracebundles, but not the trace engine decisions or DB/trace
  coverage rows. This loses the SQL-vs-built-in routing evidence before the
  model consumes query results.

Tasks:

- Extend tracebundle parsing in `internal/tracequery` with:
  - `trace_provider_decisions`,
  - `trace_db_coverage`,
  - `trace_coverage`.
- Surface each field into stable `Index.Caveats` / `Result.Caveats` entries,
  matching the existing perf-provenance handoff pattern.
- Bound coverage caveat fan-out and emit a compact summary when a bundle has
  many coverage rows.
- Add tests proving tracebundle results preserve:
  - selected trace engine and SQL readiness,
  - DB table coverage,
  - trace cross-validation coverage.
- Preserve the same transparency for both customer entry modes:
  - explicit attachment (`--htrace`, `/htrace`, `--log`, `/log`);
  - no attachment, with one or more readable runtime artifact paths named in the
    request text.
- JSON repair audit:
  - no new model tool-call input fields are added in this batch;
  - no new aliases or schema-repair entries are required;
  - provenance remains system output consumed through `trace_query` results.
- Prompt red-line audit:
  - no keyword/prose matching is introduced;
  - the model receives typed provenance as evidence, not a hard intent gate.

Exit criteria:

- A model querying a tracebundle can see whether trace+perf used SQL, whether
  DB export was query-ready, and whether generated trace text passed
  `trace_query` cross-validation without reading terminal prose.
- Verified in `internal/tracequery`: trace provider decisions, DB coverage, and
  trace cross-validation coverage now flow into both `Index.Caveats` and
  `Result.Caveats`; coverage fan-out is bounded with a compacted summary.
- Trace+perf old built-in parsing remains removed from production. Any remaining
  legacy parser code is only reachable through the explicit pure-trace/no-perf
  built-in engine until Batch 6 parity is closed.
- CLI, markdown, and HTML runtime-artifact summaries now expand tracebundle
  provider/coverage details for both attachment bodies and request-named paths.

#### Batch 7B: Request-Named Artifact Routing and Prompt Teaching

Status: delivered on 2026-06-23.

Gap:

- Customers often do not attach artifacts first; they name one or more readable
  trace/log/perf paths in the question.
- The system must support that UX without turning file suffixes or prose
  keywords into hard intent gates.
- The `trace_query` tool prompt taught perf tracebundle caveats, but did not
  yet teach trace provider/DB coverage/cross-validation caveats.

Tasks:

- Keep both customer entry modes supported:
  - attached artifact bodies;
  - request text that names one or more readable artifact paths.
- Tighten analyze-stage runtime-artifact hard gates so a missing token with a
  trace-like suffix does not disable source pre-scan by itself.
- Preserve the fast artifact lane for readable paths whose content or resolved
  path identifies a runtime artifact.
- Teach `trace_query` schema/result guidance that:
  - `tracebundle_trace_provider` describes conversion engine/readiness;
  - `tracebundle_trace_db_coverage` describes SQL table export coverage;
  - `tracebundle_trace_coverage` describes generated trace and cross-validation
    coverage;
  - these caveats qualify reliability and completeness, not direct runtime root
    causes.
- Add tests for:
  - missing trace-like suffix tokens not triggering hard analyzer routing;
  - readable request-named systrace/perftrace/tracebundle paths exposing
    `trace_query`;
  - prompt/schema snapshot coverage for the new tracebundle caveats.

Exit criteria:

- A request-named readable trace/log/perf artifact gets the same analysis lane
  and report transparency as an attachment.
- A non-readable suffix-shaped token remains normal model-classified text and
  does not become a deterministic hard runtime route.

Tasks:

- Prompt/hint updates:
  - explain tracebundle systrace/perftrace/trace_db provenance,
  - teach that trace_streamer DB coverage caveats calibrate confidence,
  - remind models to attach/query tracebundle first when present.
- JSON repair audit:
  - if trace_query gains event types or fields for DB provenance, add aliases to
    the structured repair layer;
  - if no model tool input changes, document no new JSON burden.
- Handoff:
  - ensure trace provider decisions and coverage stats are surfaced into
    trace_query `Result.Caveats` and observation ledger where appropriate.
- UX:
  - CLI and REPL localized output,
  - multi-artifact transparency in attach status,
  - markdown/html report provenance.
- Tests:
  - prompt snapshot or evaluator tests for tracebundle guidance,
  - structured repair tests only for new tool-call inputs,
  - multi-artifact attach/report tests.

Exit criteria:

- Models can correctly consume generated artifacts without extra guessing or
  repeated finalization retries.

#### Batch 7C: Pure Trace Engine Choice UX Closure

Status: delivered on 2026-06-23.

Gap:

- CLI conversion already exposes `--trace-engine`, but REPL `/htrace convert`
  still tells users to leave the REPL and use the CLI when they want the
  built-in pure-trace engine.
- This makes the pure-trace SQL-vs-built-in choice less transparent in the most
  common interactive workflow, and weakens the rule that `auto` is an explicit
  availability policy rather than a hidden post-failure fallback.

Tasks:

- Add REPL `/htrace convert` option parsing for:
  - `--trace-engine=trace_streamer|builtin|auto`;
  - `--trace-engine trace_streamer|builtin|auto`.
- Keep the existing positional contract:
  - first positional argument is input;
  - optional second positional argument is output;
  - extra positional arguments fail with localized usage.
- Pass the selected engine into `hitraceconv.ConvertFile`.
- Update REPL usage text so both CLI and REPL document:
  - pure trace `auto` uses trace_streamer/SQL when discovered;
  - pure trace `auto` uses built-in only when trace_streamer is absent;
  - trace_streamer execution/export failure does not fall back to built-in;
  - trace+perf remains SQL-only.
- Add tests that:
  - REPL usage shows the in-REPL engine selector;
  - REPL `--trace-engine=builtin` reaches `ConvertFile` through the explicit
    built-in path and emits built-in provider provenance;
  - malformed or incomplete `--trace-engine` input fails before conversion.
- No trace_query input fields are added, so no JSON repair-layer changes are
  required for this batch.

Exit criteria:

- A user can choose the pure-trace engine from either `codrax trace convert` or
  `/htrace convert` without changing workflows.
- Default and `auto` use SQL/trace_streamer when available, use built-in only
  for pure trace when trace_streamer is absent, and never fall back after SQL
  execution/export failure. Trace+perf never falls back to built-in trace-body
  parsing.
- Verified with:

```bash
go test ./internal/hitraceconv
go test ./cmd
go test ./internal/repl
```

### Batch 8: SQL Resolver Fidelity and Coverage Closure

Status: in progress, with Batch 8A delivered on 2026-06-23.

#### Batch 8A: thread_state Running Window Resolver

Status: delivered on 2026-06-23.

Gap:

- The hmtrace perfetto exporter uses `sched_slice` as the preferred running
  window source and falls back to `thread_state` when `sched_slice` is absent.
- Codrax already has a `thread_state` resolver for SQL-exported callstack and
  raw ftrace CPU placement, but it is not explicitly covered as a commercial
  invariant:
  - `thread_state` rows are consumed silently as a helper table;
  - emitted resolver intervals are not counted separately from rows read;
  - the tests do not prove that a DB with no `sched_slice` can still give
    queryable CPU context to trace marker/callstack output.
- This is a table-contract gap, not a user-intent classification gap. It must be
  solved from structured DB schema and coverage, not from prompt keywords or
  model prose.

Tasks:

- Align the resolver with the hmtrace fallback shape:
  - load `thread_state(itid, ts, dur, cpu, state)` when present;
  - keep only positive-duration canonical running states;
  - sort intervals by `itid, ts`;
  - use them only to place SQL-exported timeline rows on the best-known CPU.
- Record `resolver/thread_state` coverage with:
  - table found/missing status and required columns;
  - total rows read from the table;
  - running intervals emitted for downstream CPU lookup.
- Add narrow core tests for:
  - running interval lookup and CPU fallback;
  - non-running or zero-duration rows not becoming running windows;
  - coverage distinguishing rows read from rows emitted.
- Add an end-to-end SQL exporter test with no `sched_slice`:
  - fixture has `process`, `thread`, `thread_state`, and `callstack`;
  - exported systrace callstack rows inherit CPU from `thread_state`;
  - tracebundle DB coverage exposes `resolver/thread_state`;
  - `trace_query` can build an index from the exported systrace.
- Do not add trace_query tool-call input fields. This batch only improves
  converter output and tracebundle coverage, so no JSON repair-layer changes are
  required.

Exit criteria:

- `thread_state` fallback remains a structured SQL resolver, visible in
  tracebundle coverage and covered by tests.
- DBs that lack `sched_slice` but retain `thread_state` no longer lose CPU
  context for exported callstack/raw-ftrace timeline rows.
- Verified with:

```bash
go test ./internal/hitraceconv
```

Delivered:

- `resolver/thread_state` now reports total table rows read separately from
  emitted positive-duration Running windows.
- SQL-exported callstack/raw-ftrace timeline rows can keep CPU placement from
  `thread_state` when `sched_slice` is missing.
- Tests cover canonical Running normalization, non-running/zero-duration
  filtering, tracebundle coverage visibility, and `trace_query` round-trip.

## Running Verification Matrix

Each implemented batch must run the narrow package tests it touches. Before goal
completion, run at minimum:

```bash
go test ./internal/hitraceconv ./cmd ./internal/repl ./internal/tracequery ./internal/agent ./internal/tool ./internal/types
```

After sys binary parity/retirement work, also run any eval cases covering:

- text trace conversion,
- no-perf Harmony/Donghu `.sys` binary conversion,
- tracebundle attach,
- perftrace CPU sample analysis,
- runtime artifact path mentioned in user request,
- Donghu/Harmony trace root-cause answers.

## Open Risks

- trace_streamer redistribution license. External discovery lands first; embedded
  binary waits until license/hash/version governance is clear.
- trace_streamer `.sys` parity. Until this is proven, the built-in sys binary
  parser remains a guarded capability rather than dead compatibility code.
- Existing local worktree has unrelated untracked eval summary files. Batch
  commits must stage only files touched by this delivery stream.
