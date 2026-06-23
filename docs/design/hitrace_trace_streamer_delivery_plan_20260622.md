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

- Trace body conversion has a user-visible selector: `auto`, `trace_streamer`,
  or `builtin`. In `auto`, Codrax uses `trace_streamer`/SQL first when the tool
  is discovered, and falls back to the built-in raw trace parser when
  trace_streamer is absent or SQL execution/export/normalization fails. Explicit
  `trace_streamer` and explicit `builtin` do not degrade to another engine.
- Production conversion is not a dual-success path. The CLI and REPL must expose
  the same selector, and every attempted/succeeded provider must be recorded in
  tracebundle provenance. Parity tests may execute both engines in one test, but
  customer conversion should use SQL on success and only run the built-in parser
  after SQL is unavailable or failed in `auto`.
- Harmony/Donghu `.sys` and modern profiler/session raw trace conversion remain
  supported through the built-in engine because `auto` uses it as the commercial
  fallback for both pure trace and trace+perf captures.
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
- Keep `--perf-tools-status` working as a full conversion-tool status surface:
  it must include trace_streamer/trace-engine status before the perf-only
  provider section, because trace+perf htrace depends on both lanes.
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

Status: in progress. Batch 6A, Batch 6B, Batch 6C raw-ftrace exporter,
Batch 6C2 cross-engine raw-ftrace parity, and the Batch 6C3 representative
fixture helper are delivered on 2026-06-23. The remaining gate is at least one
redistributable real no-perf Harmony/Donghu `.sys` fixture that passes the
representative helper.

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

Status: delivered on 2026-06-23 for the scheduler, wakeup, CPU-supply,
trace-marker, frame, IRQ, and softirq parity matrix.

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

Status: delivered for SQL raw-ftrace export and cross-engine semantic parity on
2026-06-23. Representative customer `.sys` fixture validation remains the
Batch 6C3/overall Batch 6 external evidence gate.

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
- The guard is test-only. Production conversion now follows the Batch 11G
  strategy: explicit engines are exact, while `auto` tries SQL first and falls
  back to the built-in raw trace parser if SQL is unavailable or fails.
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

Next slice: Batch 6C4 representative fixture authority hardening.

Status: delivered on 2026-06-23.

Gap:

- The representative fixture helper correctly avoids developer-local absolute
  paths, but the manifest authority is still too weak: `redistribution` only
  needs to be a non-empty string.
- That means a future synthetic, private, or unapproved fixture could be
  committed with persuasive prose and accidentally satisfy the retirement gate.
- This is a gate-authority gap, not a trace parser gap. It must be solved with
  typed manifest fields and deterministic validation, not prompt guidance.

Tasks:

- Extend representative `.sys` fixture manifests with authority fields:
  - `capture_class=redistributable_real_capture`;
  - `redistribution` as a constrained enum;
  - `approval_ref` for the license/customer/internal approval record;
  - `input_sha256` for the committed `.sys` file;
  - `trace_db_sha256` for any committed SQL sidecar.
- Update the README example and rules so synthetic fixtures remain useful for
  unit tests but cannot satisfy the representative retirement gate.
- Add deterministic validation in the helper:
  - reject missing or unknown `capture_class`;
  - reject non-approved `redistribution`;
  - reject missing `approval_ref`;
  - verify `input_sha256` and `trace_db_sha256` when files are present.
- Add focused tests for manifest validation without requiring a real customer
  capture in the repository.

Exit criteria for Batch 6C4:

- The representative fixture gate can no longer be satisfied by a weak manifest
  string or synthetic capture.
- Future real fixtures carry enough authority metadata to audit commercial
  readiness without reading terminal prose.
- Verified with:

```bash
go test ./internal/hitraceconv -run 'TestRepresentativeSysTrace' -count=1
go test ./internal/hitraceconv
```

Delivered Batch 6C4 on 2026-06-23:

- Representative manifests now require typed authority metadata:
  `capture_class=redistributable_real_capture`, constrained `redistribution`,
  `approval_ref`, `input_sha256`, and `trace_db_sha256` when a DB sidecar is
  present.
- The helper deterministically rejects synthetic/unapproved fixtures, missing
  approval refs, missing hashes, hash mismatches, absolute paths, and path
  traversal.
- The fixture README now documents the authority fields and makes clear that
  synthetic fixtures cannot satisfy the representative retirement gate.

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
  - `auto` uses trace_streamer/SQL when discovered;
  - `auto` uses built-in when trace_streamer is absent;
  - `auto` falls back to built-in after trace_streamer execution/export
    failure;
  - explicit `trace_streamer` and explicit `builtin` do not degrade.
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
- Default and `auto` use SQL/trace_streamer when available, then fall back to
  built-in raw trace parsing when SQL is unavailable or fails. Explicit
  `trace_streamer` and explicit `builtin` do not degrade.
- Verified with:

```bash
go test ./internal/hitraceconv
go test ./cmd
go test ./internal/repl
```

Additional guard:

- Added a pure `.sys` trace auto-mode regression test proving that a discovered
  but failing `trace_streamer` stops with a diagnostic tracebundle and does not
  silently fall back to either built-in trace-body converter. The only automatic
  built-in path remains "trace_streamer absent" for pure trace inputs.

### Batch 8: SQL Resolver Fidelity and Coverage Closure

Status: delivered on 2026-06-23 through Batch 8A and Batch 8B.

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

#### Batch 8B: Input-Aware Trace Tools Status

Status: delivered on 2026-06-23.

Gap:

- `codrax trace convert --trace-tools-status` reports generic engine readiness.
  In `auto` mode with no `trace_streamer`, current strategy selects the
  built-in fallback for both pure trace and trace+perf. Earlier SQL-only status
  wording was superseded by Batch 11G.
- The fix must inspect structured file/container content, not file suffixes or
  user prose. It should reuse the existing standalone HIPERF_DATA sidecar scan
  instead of adding a second detector.

Tasks:

- Extend `TraceToolStatus` with optional input classification:
  - input path;
  - whether an input was inspected;
  - whether a standalone perf sidecar was found;
  - inspection error, if any.
- In `BuildTraceToolStatus`, when `Options.InputPath` is non-empty:
  - stat and scan the file with the existing standalone perf sidecar detector;
  - if trace+perf is detected and `trace_streamer` is unavailable, report
    built-in trace fallback plus standalone perf fallback;
  - if pure/no-perf is detected or no input is supplied, use the same `auto`
    fallback semantics.
- Update CLI localized status rendering to show the input classification before
  provider lines.
- Add tests that:
  - generic `auto` without an input still selects built-in when
    `trace_streamer` is missing;
  - `auto` with an input containing HIPERF_DATA selects built-in fallback when
    `trace_streamer` is missing and documents standalone perf fallback;
  - localized CLI status exposes the input classification in Chinese and
    English.
- No trace_query tool-call input fields are added. This is command/status
  metadata only, so no JSON repair-layer changes are required.

Exit criteria:

- Users who pass a concrete trace+perf input to `--trace-tools-status` see the
  same auto-fallback contract that conversion will enforce.
- Pure trace and no-input status behavior does not regress.
- Verified with:

```bash
go test ./internal/hitraceconv ./cmd
```

Delivered:

- `TraceToolStatus` now carries optional input classification from structural
  file inspection: input path, inspected state, input kind, perf-sidecar
  presence, and inspection error.
- `--trace-tools-status --input <trace+perf.htrace>` reports built-in fallback
  when SQL tooling is missing, matching the conversion path's auto-fallback
  contract.
- Generic no-input `auto` status still selects built-in when `trace_streamer` is
  unavailable, preserving pure-trace UX.
- CLI status output shows the input classification in both Chinese and English.

### Batch 9: Embedded trace_streamer Governance

Status: Batch 9A delivered on 2026-06-23; Batch 9B runtime selection is
frozen/deferred. No real trace_streamer binary is embedded, and embedded
selection is not part of the active discovery chain because the binary is too
large for the current product package.

#### Batch 9A: Embedded Binary Manifest Guard

Status: delivered on 2026-06-23.

Gap:

- External `trace_streamer` discovery is implemented, and the local hmtrace
  reference is Apache-2.0, but Codrax intentionally has not embedded a
  `trace_streamer` binary yet.
- The remaining redistribution risk is not just legal prose. Without a
  deterministic guard, a future commit could add a platform binary without a
  source/version/license/hash manifest and make commercial builds hard to audit.
- This is a release-governance gap. It must be solved with a structured manifest
  and tests, not with prompt guidance or runtime caveats alone.

Tasks:

- Define the only supported embedded binary directory:
  `internal/hitraceconv/embedded_trace_streamer`.
- Add a manifest schema requirement for any future embedded binary:
  - upstream source URL;
  - upstream commit or version;
  - license id;
  - redistribution approval reference;
  - per-platform `goos`, `goarch`, relative binary path, and SHA-256 hash.
- Add a deterministic test that:
  - passes when no embedded binary directory is present;
  - fails if the directory has payloads but no manifest;
  - validates manifest paths are relative and cannot escape the directory;
  - validates every platform binary exists and matches its hash.
- Add focused manifest validation tests using temporary fixtures, without
  committing real trace_streamer binaries.

Exit criteria:

- Codrax still does not embed trace_streamer by default, but any future embedded
  binary must carry auditable source/license/version/hash metadata.
- The open risk changes from "no governance" to "no approved embedded binary
  has been selected yet".
- Verified with:

```bash
go test ./internal/hitraceconv -run 'TestEmbeddedTraceStreamer' -count=1
go test ./internal/hitraceconv
```

Delivered:

- Added a deterministic manifest guard for
  `internal/hitraceconv/embedded_trace_streamer`.
- The guard passes when no embedded binary directory is present, and validates
  future manifest metadata, relative/no-escape binary paths, binary existence,
  executable bits for non-Windows platforms, and SHA-256 hashes.
- No `trace_streamer` binary is embedded by this batch; the remaining release
  decision is selecting and approving a redistributable fixed upstream build.

#### Batch 9B: Embedded Binary Runtime Resolver

Status: frozen/deferred on 2026-06-23. The manifest/cache implementation is
kept as audited release-governance code, but active resolver selection of
embedded trace_streamer binaries is disabled because the binaries are too large
for the current product package.

Reference audit:

- The local hmtrace reference resolves a platform enum, reads platform-specific
  `include_bytes!` assets, writes the selected binary into a user cache
  directory, and chmods it executable before invoking `trace_streamer`.
- Codrax already has external discovery through explicit option,
  `CODRAX_TRACE_STREAMER`, `PATH`, and known OpenHarmony/SmartPerf/hmtrace
  locations.
- Batch 9A added a manifest guard, but production discovery still cannot
  consume a future approved embedded binary. Without a runtime resolver, an
  approved fixed binary would either be dead weight or require ad-hoc release
  glue outside the tested converter path.

Design:

- Keep the active external-tool discovery order deterministic:
  - explicit `--trace-streamer` / REPL option;
  - `CODRAX_TRACE_STREAMER`;
  - external `trace_streamer` next to the running `codrax` executable,
    including multi-platform release layouts under
    `trace_streamer/<platform>/` and `trace-streamer/<platform>/`;
  - `PATH`;
  - known OpenHarmony/SmartPerf/hmtrace locations.
- Do not commit a real binary in this batch, and do not enable embedded runtime
  selection in the active discovery chain.
- Add a compile-time extension point for future embedded assets:
  - default builds expose no embedded FS and behave exactly as external-only
    builds;
  - a future release build can enable a build tag and embed
    `internal/hitraceconv/embedded_trace_streamer/manifest.json` plus platform
    binaries.
- Reuse the Batch 9A manifest schema at runtime:
  - reject missing source URL, upstream ref, license id, approval ref, platform
    metadata, non-relative paths, path traversal, unreadable binaries, or SHA-256
    mismatches;
  - select the current `GOOS/GOARCH` platform only;
  - write to a deterministic cache directory keyed by upstream ref and binary
    hash, then verify cached bytes before returning the path;
  - chmod executable on non-Windows platforms.
- Surface resolver provenance in `TraceToolStatus`:
  - source should identify `embedded trace_streamer`;
  - caveats should report unsupported host platform or invalid embedded assets
    as structured tool status, not as prompt wording.
- Performance and memory:
  - read only the selected platform binary into memory;
  - avoid scanning unrelated assets;
  - reuse an already cached verified binary when hash matches;
  - use atomic temp-write plus rename so interrupted extraction does not leave a
    selected corrupt executable.
- Prompt/JSON contract:
  - no trace_query tool-call input field is added;
  - this is converter/tool status metadata, so no JSON repair aliases are
    required.

Tasks:

- Move the manifest validation helpers from the test-only guard into production
  package code so runtime and tests share one authority.
- Add an embedded resolver that accepts an `fs.FS` for hermetic tests and a
  default no-assets implementation for ordinary builds.
- Add a future build-tag file stub for `codrax_embed_trace_streamer` that can
  wire a real `embed.FS` once approved assets are present.
- Keep embedded resolution out of `resolveTraceStreamerTool` until a future
  product/package decision explicitly reopens this lane.
- Add tests that:
  - a valid embedded manifest/assets fixture extracts to cache and is executable;
  - the resolver reuses a verified cached binary instead of rewriting it;
  - current host platform mismatch is reported without selecting a binary;
  - hash/path/manifest failures do not select a binary;
  - `BuildTraceToolStatus` does not select embedded fixtures while this lane is
    deferred.
- Update localized trace-tools status expectations if source/caveat text changes.

Exit criteria:

- Default builds remain external-tool only until a binary is explicitly embedded.
- Future approved embedded binaries can reuse the same manifest/hash/approval
  code, but a separate product decision is required before runtime selection is
  re-enabled.
- Verified with:

```bash
go test ./internal/hitraceconv -run 'TestEmbeddedTraceStreamer|TestTraceToolStatus' -count=1
go test ./internal/hitraceconv ./cmd
```

Delivered:

- Moved manifest schema/path/hash validation into production code so the runtime
  resolver and repository guard share one authority.
- Added manifest/cache/runtime extraction helpers and a future
  `codrax_embed_trace_streamer` build-tag hook for approved release binaries,
  but kept active tool discovery external-only.
- Integrated same-directory external discovery after explicit/env overrides,
  including direct sibling binaries and host platform subdirectories for
  Darwin/Linux/Windows release bundles, followed by PATH and known-location
  discovery.
- Runtime extraction reads only the selected platform asset, verifies SHA-256,
  writes through a temp file into a deterministic cache keyed by upstream ref,
  platform, and hash, chmods non-Windows binaries, and reuses verified cached
  binaries.
- Added tests for runtime extraction, cache reuse, unsupported host platforms,
  hash mismatch, multi-platform same-directory candidates, and
  `BuildTraceToolStatus` embedded-selection deferral.

### Batch 10: Coverage Telemetry Closure

Status: delivered on 2026-06-23 for Batch 10A.

#### Batch 10A: Coverage Elapsed-Time Handoff

Status: delivered on 2026-06-23.

Gap:

- The performance/memory budget requires coverage stats to include elapsed
  time, but `TraceDBCoverage` currently exposes rows, peak buffered rows, spill
  chunks, and temp bytes only.
- That leaves users and downstream model stages with memory pressure visibility
  but weak timing visibility. For large trace_streamer DB conversions, a slow
  extractor, resolver, raw-ftrace pass, sorter merge, or trace_query
  cross-validation can be invisible unless it also emits many rows.
- This is a structured provenance gap. It must be solved in tracebundle
  metadata and query-result caveats, not through prompt prose or ad-hoc log
  parsing.

Design:

- Add `elapsed_us` to `TraceDBCoverage`.
- Populate elapsed timing at the coverage producer boundary:
  - table/resolver inspections where the helper owns the SQL query;
  - sorter coverage around in-memory sort, spill, and merge/write;
  - trace_query cross-validation coverage;
  - major SQL exporter families where a single function returns one or more
    coverage rows.
- Use microseconds instead of milliseconds so fast unit fixtures still carry
  measurable non-zero timing when possible.
- Keep overhead bounded:
  - use monotonic `time.Now()`/`time.Since()` only at exporter/resolver
    boundaries, not per row;
  - never keep timing logs in memory;
  - no extra DB queries for timing.
- Handoff and UX:
  - serialize `elapsed_us` into `.tracebundle.json`;
  - preserve it when `trace_query` parses tracebundles and emits caveats;
  - show it in CLI and REPL coverage detail lines when present.
- Prompt/JSON contract:
  - no trace_query tool-call input fields are added;
  - `elapsed_us` is output metadata only, so no JSON repair aliases are needed.

Tasks:

- Extend `TraceDBCoverage` and `traceBundleCoverage` with `elapsed_us`.
- Add small timing helpers that safely set elapsed microseconds.
- Instrument the DB exporter/resolver/sorter/cross-validation coverage producers
  without changing row semantics.
- Update CLI and REPL coverage formatting plus tests for Chinese/English output.
- Update tracebundle serialization and `trace_query` caveat tests so model
  handoff includes elapsed timing.
- Run focused and broad tests:

```bash
go test ./internal/hitraceconv -run 'TestTraceBundleIncludesTraceDBCoverage|TestTraceDBCore|TestTraceDBRowSink' -count=1
go test ./internal/tracequery -run 'TestTraceBundle' -count=1
go test ./cmd ./internal/repl
go test ./internal/hitraceconv ./internal/tracequery
```

Exit criteria:

- Conversion coverage carries both memory and timing telemetry through
  tracebundle, terminal/REPL output, and trace_query caveats.
- Timing instrumentation is per-stage/per-exporter, not per-row, and does not
  add unbounded memory or query overhead.
- No model JSON input or repair-layer change is introduced.

Delivered:

- Added `elapsed_us` to `TraceDBCoverage` and tracebundle coverage parsing.
- Populated elapsed timing at resolver inspection, scheduler/extended SQL
  exporters, sorted row writing, and trace_query cross-validation boundaries.
- Surfaced elapsed timing in CLI and REPL coverage detail lines with Chinese
  localization, and preserved the raw `elapsed_us` key for English/tooling.
- Preserved elapsed timing in trace_query tracebundle caveats so downstream
  model stages can consume slow exporter/validation provenance without reading
  side logs.
- Added focused tests for producer timing, tracebundle serialization,
  trace_query caveats, and localized CLI/REPL output.
- Verified with:

```bash
go test ./internal/hitraceconv -run 'TestTraceBundleIncludesTraceDBCoverage|TestTraceDBCore|TestTraceDBRowSink' -count=1
go test ./internal/tracequery -run 'TestTraceBundle' -count=1
go test ./cmd ./internal/repl
go test ./internal/hitraceconv ./internal/tracequery
```

### Batch 11: Commercial Gate Transparency Closure

Status: delivered on 2026-06-23 for Batch 11A.

#### Batch 11A: Sys Binary Parity Gate Status

Status: delivered on 2026-06-23.

Gap:

- Batch 6 is intentionally still open until at least one redistributable real
  no-perf Harmony/Donghu `.sys` fixture passes the representative helper.
- The repository has a hermetic helper and authority validation, and the
  delivery plan states that the gate is open, but `codrax trace convert
  --trace-tools-status` currently surfaces only trace engine/tool readiness and
  input classification.
- That can make an operator see "trace_streamer available" or "built-in
  available" without also seeing why the built-in sys binary lane remains an
  explicit guarded capability instead of being retired.
- This is a transparency and handoff gap. It must be solved as typed tool
  status metadata and localized UX, not prompt prose and not a hard routing
  rule.

Current evidence audit:

- `internal/hitraceconv/testdata/representative_sys_traces/README.md` defines
  the required manifest and authority fields.
- `TestRepresentativeSysTraceFixtures` skips loudly when no manifest exists.
- Current local searches under the repository, `/Users/han/opt/customlogs`, and
  a shallow `/Users/han/opt` scan found no candidate committed real `.sys` or
  `.htrace` fixture that can close the gate.
- Therefore the correct commercial state is "pending representative fixture",
  not "complete" and not "blocked by code".

Design:

- Extend `TraceToolStatus` with a dedicated gate status for
  `no_perf_sys_binary_parity`.
- Keep the gate separate from providers:
  - `trace_streamer` remains an executable/tool provider;
  - `codrax_builtin_modern_profiler` remains a converter provider;
  - sys-binary parity is a commercial retirement gate over evidence.
- Expose bounded structured fields:
  - gate name;
  - state (`pending_representative_fixture` until a real fixture is committed
    and verified);
  - proven boolean;
  - fixture manifest count when the source-tree fixture directory is visible;
  - required evidence path/description;
  - caveats explaining that the built-in sys binary parser remains guarded, is
    used as auto fallback, and explicit trace_streamer never falls back to it.
- Render the gate in CLI trace-tools status for Chinese and English.
- Add focused tests for:
  - `BuildTraceToolStatus` includes the gate with the current pending state;
  - CLI English output includes `sys_binary_parity_gate`;
  - CLI Chinese output localizes the pending gate and does not leak English
    caveats.
- Prompt/JSON contract:
  - no trace_query or model tool-call input fields are added;
  - no JSON repair aliases are required;
  - the gate is system output/tool status metadata.
- Performance and memory:
  - optional manifest counting is bounded to the small committed fixture
    directory only;
  - no trace files are opened or hashed from status rendering.

Exit criteria:

- Users can inspect not only provider readiness but also the current sys-binary
  parity/retirement gate state from the same trace tools status command.
- The open Batch 6 gate is visible without reading the delivery plan or a test
  skip line.
- No production routing behavior changes and no prompt/JSON repair burden is
  introduced.

Delivered:

- Added `TraceToolGateStatus` and surfaced
  `no_perf_sys_binary_parity` through `TraceToolStatus`.
- The gate reports pending representative fixture state, manifest count for the
  committed fixture directory when visible, required evidence, delivered
  synthetic parity evidence, and caveats that keep the built-in sys parser as
  an explicit guarded lane.
- CLI `--trace-tools-status` now renders the gate in English and Chinese,
  including localized state/caveats without leaking internal English prose.
- No converter routing behavior changed, no trace_query input fields changed,
  and no JSON repair aliases were needed.
- Verified with:

```bash
go test ./internal/hitraceconv -run 'TestBuildTraceToolStatus|TestTraceStreamerHostPlatformDirs' -count=1
go test ./cmd -run 'TestTraceConvertTraceToolStatusLines' -count=1
```

#### Batch 11B: REPL Trace Tools Status

Status: delivered on 2026-06-23.

Gap:

- Batch 11A made `no_perf_sys_binary_parity` visible through CLI
  `codrax trace convert --trace-tools-status`.
- The interactive workflow still requires users to leave the REPL to see that
  same trace_streamer/provider/gate status, even though `/htrace convert` is a
  supported conversion path and its usage text already points at tool status.
- This weakens transparency for users who primarily work through `/htrace`
  attachment and conversion commands.

Design:

- Add `/htrace tools-status` as a REPL-only status command.
- Reuse `hitraceconv.BuildTraceToolStatus` as the single status authority.
- Render the same high-value fields as CLI, with REPL-native formatting:
  - engine mode and selected engine;
  - trace_streamer provider readiness, source/path/checks/caveats;
  - built-in provider boundary;
  - `no_perf_sys_binary_parity` gate state, fixture count, required evidence,
    and caveats.
- Keep the command read-only:
  - no artifact attachment changes;
  - no trace conversion;
  - no path probing beyond the bounded provider/status checks already used by
    `BuildTraceToolStatus`;
  - no user-prose or filename-keyword routing.
- Extend `/htrace` usage, full help, and native suggestions so the command is
  discoverable.
- Prompt/JSON contract:
  - no model tool-call input fields are added;
  - no JSON repair aliases are required;
  - output remains human/tool-status UX, not trace_query evidence.

Exit criteria:

- REPL users can inspect trace engine/tool/gate status without leaving the
  session.
- `/htrace tools-status` output includes the sys parity gate and is localized.
- `/htrace tools-status` does not mutate attached trace state.

Delivered:

- Added `/htrace tools-status` as a read-only REPL command backed by
  `hitraceconv.BuildTraceToolStatus`.
- REPL status output now surfaces trace engine selection, trace_streamer and
  built-in provider readiness, and the `no_perf_sys_binary_parity` gate.
- Updated `/htrace` usage, full help, native suggestions, and the user guide so
  the command is discoverable.
- Added tests proving English and Chinese output include the sys parity gate,
  Chinese output does not leak internal English caveats, and existing attached
  trace state is not mutated.
- Verified with:

```bash
go test ./internal/repl -run 'TestHitraceToolsStatus|TestHitraceConvertHelpAliases|TestHelpLines_SurfaceHtraceConvertSubcommand|TestHelpLines_DefaultConciseFullDiscoverable|TestHandleSlashHelpAllRendersFullTable|TestNativeSlashSuggestShowsHitraceConvertSubcommand' -count=1
go test ./internal/repl -run 'TestNativeSlashSuggestAtraceReusesHtraceSubcommands|TestHitraceConvertPassesTraceEngineOption' -count=1
```

#### Batch 11C: Tracebundle Gate Handoff

Status: delivered on 2026-06-23.

Gap:

- `TraceToolStatus` now exposes `no_perf_sys_binary_parity` through CLI and
  REPL status.
- Conversion tracebundles still carry provider decisions and coverage, but do
  not carry this gate. A downstream model that receives only a
  `.tracebundle.json` through `/htrace` or a request-named path can see which
  engine ran, but cannot see the commercial retirement gate that qualifies the
  built-in sys binary lane.
- This is a provenance handoff gap, not a routing gap. It should be solved by
  tracebundle metadata and `trace_query` caveats, not by prompt wording or
  filename/prose matching.

Design:

- Add `trace_tool_gates` to tracebundle metadata.
- Populate it from `hitraceconv.BuildTraceToolStatus` at bundle-write time,
  using the conversion options when available so explicit trace_streamer paths
  and source-local manifest counts remain consistent with tool status.
- Keep the schema bounded and output-only:
  - gate name;
  - state;
  - proven boolean;
  - fixture manifest count;
  - required evidence;
  - evidence strings;
  - caveats.
- Extend `trace_query` tracebundle parsing to surface the gates into
  `Index.Caveats` and `Result.Caveats` as `tracebundle_trace_tool_gate ...`
  lines.
- Bound caveat fan-out if future bundles add multiple gates.
- Add tests proving:
  - newly written tracebundles serialize `trace_tool_gates`;
  - `trace_query.BuildIndex` and `Run` expose the gate caveat;
  - no trace_query tool-call input fields or JSON repair aliases are added.

Exit criteria:

- The sys parity gate follows converted artifacts through tracebundle attach and
  request-named path workflows.
- A model consuming only trace_query results can see that built-in sys parsing
  remains a guarded lane until representative no-perf `.sys` fixture evidence
  lands.
- No production engine routing behavior changes.

Delivered:

- Tracebundle metadata now serializes bounded `trace_tool_gates` rows with
  stable snake_case fields.
- `trace_query` reads `trace_tool_gates` from `.tracebundle.json` and surfaces
  them through `Index.Caveats` and `Result.Caveats` as
  `tracebundle_trace_tool_gate ...` lines.
- The trace_query tool description now teaches models to treat
  `tracebundle_trace_tool_gate` as converter guardrail/provenance state, not a
  runtime root cause.
- Added tests for tracebundle serialization, trace_query handoff, and bounded
  gate caveat fan-out.

#### Batch 11D: Unified CLI Conversion Tool Status

Status: delivered on 2026-06-23.

Gap:

- Users naturally run `codrax trace convert --perf-tools-status` before
  converting trace+perf htrace. The previous output only described hiperf,
  simpleperf, and raw perf fallback.
- For trace+perf htrace, SQL remains the preferred high-confidence path, so a
  status report that omits trace_streamer makes the conversion readiness answer
  incomplete even though auto can fall back to built-in raw parsing.

Design:

- Keep `--trace-tools-status` as trace-only.
- Make `--perf-tools-status` a full conversion-tool status surface:
  - print trace_streamer/trace-engine/sys-parity-gate status first;
  - print perf adapter/raw fallback status second;
  - if both status flags are passed, do not duplicate trace output.
- Keep the implementation deterministic: this uses typed option/status structs,
  not filename suffixes, user-prose keywords, or model-output parsing.
- Update CLI help, REPL `/htrace convert` help, and the user guide so the status
  command sets correct expectations.

Exit criteria:

- A single `--perf-tools-status` run shows whether trace_streamer is discovered
  and whether perf sample conversion has an official or raw fallback path.
- Chinese/English output continues to follow the configured language.
- No conversion routing behavior changes.

Delivered:

- `--perf-tools-status` now prints trace provider/gate status before perf
  provider status.
- Updated CLI flag help, REPL help, Markdown guide, and checked-in HTML guide
  snippet.
- Added a CLI-level regression test proving `--perf-tools-status` includes
  `trace_provider[official_trace_db/trace_streamer_db]` and `perf_parser`.

#### Batch 11E: Cross-Platform Trace DB Open and Partial Artifact Hygiene

Status: delivered on 2026-06-23.

Customer signal:

- On Windows, `/htrace tools-status` can correctly discover
  `D:\opt\codrax-main\trace_streamer\windows-x86_64\trace_streamer.exe`, and
  `/htrace convert hiprofiler_data.htrace` can successfully produce
  `hiprofiler_data.htrace.trace.db`.
- The conversion then fails to produce text systrace with:
  `SQL logic error: invalid uri authority: hiprofiler_data.htrace.trace.db?mode=ro (1)`.
- The same partial result prints the same `trace_db` artifact and normalize
  failure caveat twice.

Root cause class:

- The trace_streamer lane has two stages:
  1. external `trace_streamer` writes SQLite DB;
  2. Codrax opens that DB read-only and exports queryable systrace rows.
- The observed failure is in stage 2, not in trace_streamer discovery or in the
  input file. `perftrace` is still generated because perf sidecar extraction
  and raw perf.data normalization are independent of DB-to-systrace export.
- The SQLite read-only DSN builder must be platform-class aware. It should
  support POSIX relative/absolute paths, Windows drive absolute paths,
  drive-relative paths, and UNC-like inputs without producing a URI whose
  host/authority is the DB filename.
- Partial conversion artifacts should be de-duplicated by typed artifact
  identity, not by filename-specific logic. Caveats should also be de-duplicated
  before user-facing output and tracebundle metadata.

Design:

- Add a dedicated SQLite read-only DSN builder for trace DB paths.
  - Prefer absolute filesystem paths before URI conversion so relative paths do
    not become URI authorities.
  - Convert Windows separators to slash form only for URI encoding.
  - Encode the path with `url.URL{Scheme:"file", Path: ...}` and `mode=ro`.
  - Keep the function deterministic and side-effect free apart from the bounded
    absolute-path normalization.
- Add lexical tests for DSN generation across platform path classes:
  - POSIX relative path;
  - POSIX absolute path;
  - Windows drive absolute path;
  - Windows drive-relative path;
  - Windows UNC path.
- Add a runtime round-trip test that opens a real SQLite DB from a relative path
  with spaces using `openTraceDB`.
- Add generic artifact/caveat de-duplication when composing trace_streamer
  partial results and when rendering CLI output.
  - Artifact key: type + cleaned path + converter.
  - Caveat key: exact normalized text.
  - Preserve first occurrence order.
- Add tests proving the trace+perf partial bundle does not repeat the same
  `trace_db` artifact or the same normalize-failure caveat.
- Prompt/JSON contract:
  - no new model tool-call input fields;
  - no JSON repair aliases required;
  - result guidance becomes clearer through existing tracebundle artifacts and
    caveats.

Exit criteria:

- A DB path produced by trace_streamer can be reopened read-only on Windows and
  POSIX path shapes, including relative paths.
- If DB-to-systrace export fails, the customer sees exactly one `trace_db`
  artifact and one normalize-failure caveat, plus the preserved `perftrace`
  artifact when perf sidecars are present.
- Superseded by Batch 11G: auto conversion now falls back to the built-in raw
  trace parser when SQL is unavailable or fails. Explicit trace_streamer mode
  still does not fall back.

Delivered:

- Fixed the SQLite read-only DSN builder so relative paths, paths with spaces,
  Windows drive paths, and UNC-like paths are encoded as authority-free
  `file:` URIs before adding `mode=ro`.
- Added a real SQLite round-trip guard for opening a relative `.trace.db` path
  containing spaces.
- Added generic `Result` artifact/caveat normalization and wired it through
  conversion branches and tracebundle writing, so partial trace+perf results do
  not repeat the same `trace_db` artifact or normalize-failure caveat.
- Added a trace+perf partial-result regression case that simulates a successful
  trace_streamer DB export followed by DB-to-systrace normalize failure while
  preserving `perf_data`/`perftrace`.
- Tightened partial tracebundle semantics so a bundle with `perftrace` but no
  `systrace` marks `perf_clock_alignments` as `trace_body_missing` instead of
  implying trace-window correlation is available.
- Added conversion progress events for CLI and REPL surfaces:
  - `trace_streamer` export emits start/heartbeat/complete or failed events;
  - trace DB normalization emits start/complete or failed events;
  - official hiperf/simpleperf adapters emit external-command progress;
  - raw perf.data fallback emits parse/write progress with records/byte
    counters where available.
- Confirmed the customer-facing distinction:
  - `perftrace` succeeds through sidecar extraction plus raw perf.data parsing
    and does not use SQLite DB URI opening;
  - text `systrace` depends on reopening the trace_streamer SQLite DB and was
    blocked by the URI authority bug.

Verification:

```bash
go test ./internal/hitraceconv -run 'TestSQLiteReadOnlyDSN|TestOpenTraceDBRelativePathWithSpacesReadOnly|TestConvertFileTracePerfDBNormalizeFailureDedupesPartialArtifacts' -count=1
go test ./internal/hitraceconv -count=1
go test ./cmd ./internal/repl ./internal/hitraceconv -run 'TestTraceConvertProgressLine|TestHitraceConvertPassesTraceEngineOption|TestPerfClockAlignmentsForArtifacts|TestSQLiteReadOnlyDSN|TestOpenTraceDBRelativePathWithSpacesReadOnly|TestConvertFileTracePerfDBNormalizeFailureDedupesPartialArtifacts|TestConvertRawPerfData' -count=1
```

#### Batch 11F: SQL Perf Primary Path and Hiperf Tool Discovery Parity

Status: delivered on 2026-06-23.

Customer signal:

- Once trace_streamer can create a SQLite DB, users expect Codrax to consume all
  query-ready data from that DB before falling back to raw sidecars.
- The previous partial conversion could either generate a standalone `.perftrace`
  from the raw `HIPERF_DATA` sidecar or skip `.perftrace` entirely when
  trace_streamer DB export already had perf sample rows available. Both outcomes
  were confusing: one could create duplicate CPU-sample streams, while the other
  left users with only a binary `.perf.data` sidecar even though SQL had already
  parsed query-ready perf rows.
- `hiperf_host` discovery lagged behind trace_streamer discovery: it supported
  explicit path/env/PATH, but not the Codrax executable directory and
  multi-platform sidecar layout users use for `trace_streamer`.

Design:

- Treat trace_streamer DB perf rows as the primary CPU sample source when DB
  normalization emits `perf_sample` coverage.
  - Embed query-ready `perf_sample:` text rows into the generated `.systrace`
    rather than producing a second `.perftrace` copy.
  - Exported sample rows carry `source=trace_streamer_db`,
    `clock=trace_streamer_db`, and `clock_confidence=calibrated`.
  - Skip both raw `.perf.data` sidecar extraction and raw `.perftrace` fallback
    generation in this case; they add no model-consumable signal and create UX
    noise.
- Use official/raw perf.data conversion only when SQL perf rows are absent,
  DB normalization fails, or the input is standalone perf.data.
- Align OpenHarmony `hiperf_host` / `hiperf` discovery with trace_streamer:
  - explicit `--hiperf-host`;
  - `CODRAX_HIPERF_HOST`;
  - Codrax executable directory;
  - Codrax executable directory platform subdirs, including `hiperf/`,
    `hiperf-host/`, and `developtools_hiperf/`;
  - `PATH`;
  - known OpenHarmony / DevEco SDK locations.
- Keep the implementation provider-based and typed. Do not infer intent from
  filename suffixes, user prose keywords, or model output text.
- Keep user-facing transparency:
  - progress events for trace_streamer, DB normalization, official adapters, raw
    perf parsing, and perftrace writing;
  - tracebundle caveats explaining when raw perf sidecars were skipped because
    SQL perf rows are already embedded in systrace;
  - temporary trace_streamer DB outputs such as `.trace.db` and
    `.trace.db.ohos.ts` are cleaned after successful conversion unless the user
    explicitly passes `--keep-trace-db` or `--trace-db-output`;
  - coverage `role` values so resolver/index-only DB tables with
    `rows_emitted=0` are not mistaken for parse failures.

Exit criteria:

- Trace+perf htrace conversion with DB `perf_sample` rows produces one
  query-ready systrace containing both trace body and perf samples, with no
  duplicate `.perftrace`, binary `.perf.data` sidecar, `.trace.db`, or
  `.trace.db.ohos.ts` in the output directory by default.
- A plain `.systrace` remains sufficient for core trace/perf event queries when
  SQL-primary `perf_sample:` rows are embedded; tracebundle is a recommended
  context artifact that makes provider, coverage, clock, caveat, and primary
  perf source provenance unambiguous to trace_query and final reports.
- `--perf-tools-status` finds `hiperf_host` beside the Codrax binary before PATH
  and supports the same platform-subdir packaging shape as trace_streamer.
- CLI and REPL conversion progress lines follow the active language.

Delivered:

- Added SQL-perf-primary selection in both normal conversion and explicit
  trace_streamer-only conversion.
- Added `standaloneExtractOptions` so standalone `HIPERF_DATA` extraction can
  skip raw `perf.data` and `.perftrace` sidecars when SQL perf rows are already
  query-ready in systrace.
- Changed trace+perf auto SQL export to use a temporary DB by default; retained
  DB and trace_streamer sidecars are now an explicit debug choice.
- Wired the same debug choice through both CLI and REPL command surfaces:
  `--keep-trace-db` and `--trace-db-output`.
- Added `traceDBCoverageHasPerfSamples` and tracebundle caveats that explain the
  primary CPU-sample source.
- Added `role` to DB/trace coverage so resolver/index tables that intentionally
  emit zero text rows are distinguishable from skipped or failed exporters.
- Updated `hiperf` discovery and tool-status guidance to match trace_streamer
  executable-directory and multi-platform bundle behavior.
- Updated the user guide with attach/direct-path usage for trace+perf htrace,
  SQL perf primary semantics, progress visibility, and official hiperf discovery
  order.

Verification:

```bash
go test ./internal/hitraceconv -run 'TestConvertFileTracePerfSQLPerfSamplesSkipRedundantPerfSidecars|TestBuildPerfToolStatusDiscoversHiperf|TestBuildPerfToolStatusReportsConfiguredToolsAndRawFallback|TestConvertFileTraceStreamerAutoMergesPerfSidecarsIntoTraceBundle' -count=1
go test ./cmd ./internal/repl ./internal/hitraceconv -run 'TestTraceConvertProgressLine|TestHitraceConvertPassesTraceEngineOption|TestPerfClockAlignmentsForArtifacts|TestSQLiteReadOnlyDSN|TestOpenTraceDBRelativePathWithSpacesReadOnly|TestConvertFileTracePerfDBNormalizeFailureDedupesPartialArtifacts|TestConvertRawPerfData' -count=1
```

#### Batch 11G: Auto Fallback Strategy Revision

Status: delivered on 2026-06-23.

Strategy update:

- `auto` is now a commercial fallback strategy for both pure trace and
  trace+perf:
  - if trace_streamer is available, try SQL first;
  - if trace_streamer is missing, or SQL execution/export/normalization fails,
    fall back to the built-in raw trace parser;
  - if SQL exported `perf_sample` rows, embedded systrace `perf_sample:` rows
    remain the primary perf evidence and raw perf sidecars are skipped;
  - if SQL is unavailable/failed, standalone perf sidecars use the existing
    official/raw perf.data fallback path.
- Explicit strategies do not degrade:
  - `--trace-engine=trace_streamer` uses SQL only and may return an error or
    partial tracebundle, but does not run the built-in parser;
  - `--trace-engine=builtin` runs the built-in raw parser directly and does not
    try SQL.

Implementation tasks:

- Remove the auto trace+perf partial-return gate that stopped built-in parsing
  after SQL failed or was unavailable.
- Keep SQL provider decisions, caveats, trace DB coverage, and trace DB
  artifacts in the fallback result so handoff preserves the failed higher
  confidence path.
- Update `BuildTraceToolStatus`, CLI/REPL usage, localization, and user guide
  wording from "SQL-only / no fallback" to "auto fallback / explicit no
  degradation".
- Add/update tests for:
  - pure `.sys` auto SQL failure falling back to built-in sys parser;
  - modern profiler auto SQL failure falling back to built-in profiler parser;
  - trace+perf auto SQL failure producing both systrace and perf sidecars;
  - trace+perf missing trace_streamer selecting built-in fallback in status;
  - explicit builtin trace+perf rendering built-in trace body;
  - explicit trace_streamer remaining no-fallback.

Verification:

```bash
go test ./internal/hitraceconv -run 'TestConvertFileNoPerfSysTraceAutoTraceStreamerFailureFallsBackToBuiltin|TestConvertFileTracePerfAutoSQLFailureFallsBackToBuiltinTraceBody|TestConvertFileTraceStreamerAutoFailureFallsBackToProfiler|TestConvertFileTracePerfBuiltinEngineUsesBuiltinTraceBody|TestBuildTraceToolStatusAutoTracePerfInputFallsBackWhenTraceStreamerMissing' -count=1
go test ./cmd ./internal/repl -run 'TestTraceConvertTraceToolStatusLines|TestTraceConvertTraceToolStatusLinesIncludeInputClassification|TestHitraceConvertHelpAliases|TestHitraceToolsStatusChineseLocalizesGate' -count=1
```

#### Batch 11H: Profiler Container Fallback Classification

Status: delivered on 2026-06-23.

Root cause:

- The old built-in sys parser expects a Codrax legacy binary sys container:
  12-byte file header followed by 8-byte segment headers.
- OpenHarmony profiler containers can instead begin with `OHOSPROF`
  `TraceFileHeader` or with a session package marker such as
  `PKGHEAD0SessionJSON-...`.
- If a profiler/perf-only or session package is not classified before sys
  fallback, the sys parser reads ordinary profiler bytes as a segment header.
  The customer-style `invalid segment type=1248751465 size=760106835 at
  offset=12` decodes to little-endian bytes `ionJSON-`, which is the middle of
  `SessionJSON-`, not a real sys segment.

Design:

- Keep the sys fallback probe for real legacy `.sys` plus perf sidecar captures.
- Add profiler-container classification to the fallback failure path:
  - leading `OHOSPROF` standalone HIPERF_DATA sidecar is reported as sidecar,
    not trace body;
  - `SessionJSON-` packages stop in the profiler/session parser even when they
    contain no directly renderable systrace text rows;
  - provider decisions and caveats retain the exact rejected fallback reason so
    parser gaps remain visible.
- Do not hide parse failures. Reframe them as typed dispatch evidence: which
  container was recognized, which parser was tried, and why it could not produce
  systrace.

Verification:

```bash
go test ./internal/hitraceconv -run 'TestConvertFileReturnsStandalonePerfArtifactWithoutSystraceContainer|TestConvertFileSessionJSONWithoutRowsDoesNotFallThroughToSysParser|TestConvertFileHandlesSessionJSONPackageWithPerfSidecar|TestConvertFileExtractsStandaloneHiperfDataAndBundle|TestLocalizeConvertMessageTracePerfAutoFallback' -count=1
```

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

- trace_streamer redistribution/package size. External discovery lands first;
  embedded binary selection is deferred because the binary is too large for the
  current product package, even though manifest/hash/version governance code is
  present for a future explicit product decision.
- trace_streamer `.sys` parity. Until this is proven, the built-in sys binary
  parser remains a guarded capability rather than dead compatibility code.
- Existing local worktree has unrelated untracked eval summary files. Batch
  commits must stage only files touched by this delivery stream.
