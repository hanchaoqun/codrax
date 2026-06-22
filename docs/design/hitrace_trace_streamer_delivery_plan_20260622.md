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
    `tryConvertProfilerContainer`, then legacy `scanMetadata -> renderRows`.
  - Legacy failures can still surface as "supported binary hitrace event-format
    container" misses.
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
    fixtures. These are now archival only and must stop defining production
    behavior.
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

- No production fallback to the old event-format segment parser.
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
  beyond an in-memory threshold. MVP may sort in memory only for synthetic tests,
  but the commercial exit criterion requires a spillable plan before enabling
  large-file default.
- Perftrace export should stream samples and callchains in row order; avoid
  materializing full callchain maps unless the DB row count is below a bounded
  threshold.
- Emit coverage stats: table rows read, rows emitted, bytes written, elapsed time,
  peak in-memory row count when known.

### Legacy Removal

- Removing the old raw-page parser reduces memory pressure from synthetic raw
  event-page decoding and eliminates unsupported header-only row churn.

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
    `builtin_modern_profiler`, and `legacy_event_segment_archival`.
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
- In `auto`, prefer trace_streamer. If it fails, fall back only to
  `builtin_modern_profiler` or perf-only partial output. Do not fall back to
  legacy segment parsing.
- Add DB artifact to tracebundle when `KeepTraceDB` is enabled.
- Tests:
  - fake trace_streamer writes a DB file and records args.
  - explicit mode fail-fast.
  - auto mode records failure and chooses modern fallback.
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
  - `--trace-engine=auto`: try provider when available; if provider fails,
    record a failed trace provider decision and continue only to
    `builtin_modern_profiler` / perf-only partial output.
  - `--trace-engine=builtin`: skip trace_streamer provider with an explicit
    skipped decision.
- Add artifact behavior:
  - when `KeepTraceDB=true`, retain the DB as `ArtifactTraceDB`.
  - when `KeepTraceDB=false`, remove temporary DB after DB export is implemented;
    until Batch 3, do not delete a successful explicit trace_streamer DB because
    it is the only produced trace artifact.
- Add test fixtures:
  - fake shell trace_streamer script writing `sqlite-like` bytes and an args log;
  - fake failing trace_streamer script with stderr;
  - explicit missing tool;
  - auto fallback to profiler text rows;
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
- Delivered: `auto` tries trace_streamer DB export when available, records
  provider success/failure, and falls through to the current modern profiler
  fallback.
- Delivered: `KeepTraceDB` and explicit `TraceDBOutputPath` retain
  `ArtifactTraceDB` in tracebundle; temporary auto DBs are cleaned when not kept.
- Verified with:

```bash
go test ./internal/hitraceconv ./cmd ./internal/repl
```

Boundary that remains for Batch 3/4:

- The DB is not parsed yet. Explicit trace_streamer conversion currently returns
  a DB-only tracebundle with a caveat that systrace/perftrace DB exporters are
  delivered by later batches.

### Batch 3: DB Exporter MVP

Status: planned.

Tasks:

- Choose SQLite strategy and document dependency tradeoff before code:
  - pure-Go SQLite is preferred for single-binary portability;
  - external `sqlite3` CLI is only acceptable as a temporary test helper.
- Implement schema introspection helpers:
  - table exists,
  - column exists,
  - typed nullable reads,
  - coverage counters.
- Export event families:
  - `thread`/process dump,
  - `sched_slice` -> `sched_switch`,
  - `instant` -> `sched_wakeup`/trace markers,
  - `irq`,
  - cpu idle/frequency,
  - clock rates,
  - frame slices.
- Write systrace rows through existing official-compatible render shape.
- Tests:
  - synthetic DB per table family.
  - systrace text round-trip through `tracequery.BuildIndex`.
  - coverage stats in tracebundle.

Exit criteria:

- Scheduler and frame trace bodies no longer depend on old binary segment pages.

### Batch 4: DB Perf Exporter

Status: planned.

Tasks:

- Export `perf_sample`, `perf_callchain`, `perf_thread`, `perf_report`,
  `perf_files`, `data_dict`, and optional `hmtrace_perf_symbolized_frame`.
- Prefer symbolized display names when present.
- Preserve timestamp source:
  - `timestamp_trace` when present,
  - `timeStamp`/`timestamp` with caveat otherwise.
- Emit Codrax `.perftrace` rows, not only visual `hiperf:` spans.
- Add perf/trace clock alignment notes.
- Tests:
  - synthetic DB sample with callchain and DSO.
  - symbolized-frame preference.
  - trace_query `perf_stats` round-trip from tracebundle.

Exit criteria:

- Running/CPU root-cause analysis can use perf samples from trace_streamer DB.

### Batch 5: Built-in Modern Parser

Status: planned.

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

### Batch 6: Remove/Quarantine Legacy Segment Parser

Status: planned.

Tasks:

- Remove production call to `scanMetadata`.
- Delete or isolate:
  - `scanMetadata`,
  - `parseEventFormats`,
  - raw page event-id rendering,
  - header-only unknown event output.
- Rewrite old tests into DB/profiler fixtures.
- Keep one archival rejection test for legacy segment input.
- Update CLI/REPL wording so legacy counters are not presented as normal
  conversion metrics.

Exit criteria:

- Unsupported old event-format segment input is rejected or marked archival; it
  is never a success path.

### Batch 7: Prompt, Handoff, JSON Repair, and UX Closure

Status: planned.

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

## Running Verification Matrix

Each implemented batch must run the narrow package tests it touches. Before goal
completion, run at minimum:

```bash
go test ./internal/hitraceconv ./cmd ./internal/repl ./internal/tracequery ./internal/agent ./internal/tool ./internal/types
```

After legacy removal, also run any eval cases covering:

- text trace conversion,
- tracebundle attach,
- perftrace CPU sample analysis,
- runtime artifact path mentioned in user request,
- Donghu/Harmony trace root-cause answers.

## Open Risks

- SQLite dependency size and portability. Must be resolved explicitly before
  Batch 3.
- trace_streamer redistribution license. External discovery lands first; embedded
  binary waits until license/hash/version governance is clear.
- Large DB global ordering. Need streaming/spillable plan before declaring
  commercial readiness for very large captures.
- Existing local worktree has unrelated modified/untracked files. Batch commits
  must stage only files touched by this delivery stream.
