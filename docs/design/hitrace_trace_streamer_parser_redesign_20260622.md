# HiTrace trace_streamer and Built-in Parser Redesign

Date: 2026-06-22

Reference: https://gitcode.com/diting/hmtrace/tree/main

## Goal

Modern Harmony/OpenHarmony `.htrace` conversion must follow the same high-coverage
shape as `hmtrace`: first normalize the capture through `trace_streamer` into a
structured database, then export Codrax-native `.systrace`, `.perftrace`, and
`.tracebundle.json` artifacts for `trace_query`.

This redesign also replaces the current built-in legacy event-format segment
parser. The old `SEGMENT_EVENTS_FORMAT` + `SEGMENT_RAW_TRACE` container is no
longer a production compatibility target. Codrax should not preserve the old
parser as an automatic fallback, and tests should stop treating legacy segment
fixtures as the canonical binary HiTrace shape.

## Current Gap

The current converter still has a legacy path:

```text
scanMetadata
  -> parseEventFormats
  -> renderRows
  -> header-only fallback for unsupported events
```

That path assumes a file layout built around:

- file header
- segment headers
- `SEGMENT_EVENTS_FORMAT`
- `SEGMENT_CMDLINES`
- `SEGMENT_TGIDS`
- `SEGMENT_RAW_TRACE`
- raw trace pages keyed by event id

Customer profiler captures such as `hiprofiler_data.htrace` can carry modern
profiler/SmartPerf payloads and perf sidecars without matching that old segment
layout. In that case Codrax can extract `perf.data`, but systrace generation
fails with errors such as `invalid segment type=... at offset=...`. Treating this
as a recoverable "old-format parse miss" leaves the user with partial artifacts
and no queryable scheduler trace.

## hmtrace Reference Model

The design follows the public `hmtrace` implementation pattern rather than the
old Codrax event-segment parser.

Observed `hmtrace` model:

- It embeds platform-specific `trace_streamer` binaries in the release artifact.
- At runtime it extracts the embedded binary into a versioned cache directory and
  chmods it executable on Unix platforms.
- `export-db` invokes `trace_streamer <input> -e <output.db>`, with optional
  native symbol directories passed through to `trace_streamer`.
- `convert-db` opens the resulting SQLite DB and runs a set of table-specific
  extractors. It does not rely on old binary event-format segments.
- Extractors are keyed by DB tables such as `thread`, `sched_slice`, `instant`,
  `irq`, `cpu_measure_filter`, `measure_filter`, `process_measure`,
  `frame_slice`, `diskio`, `network`, `log`, `hisys_all_event`,
  `xpower_measure`, `perf_sample`, and `perf_callchain`.
- Perf support is DB-first: `perf.data` can be exported into its own DB,
  perf-related tables can be merged into the trace DB, and symbol enhancement can
  create an additional `hmtrace_perf_symbolized_frame` table consumed by later
  exporters.

Codrax should keep the same ingestion skeleton but adapt the output contract:

- `trace_streamer` remains responsible for the highest-coverage raw
  `.htrace`/`perf.data` decoding.
- Codrax owns the DB-to-queryable-artifact export because `trace_query` needs
  stable ftrace-compatible rows and normalized `perf_sample:` rows.
- Perf samples should not be consumed only through visual systrace spans like
  `tracing_mark_write: B|pid|hiperf:...`. Those spans are useful for UI
  timelines, but model-side root-cause analysis needs `.perftrace` rows that
  preserve timestamp, CPU, tid/pid, event, period/count, DSO, symbol, callchain,
  symbolization quality, and source provenance.
- Embedded `trace_streamer` is a valid long-term UX target, but Codrax should
  first land external discovery/configuration and only embed a fixed binary after
  validating license, platform coverage, hash/version provenance, and release
  size.

## New Compatibility Contract

The new contract is intentionally not backward compatible with the legacy
event-format segment parser.

- Production conversion supports modern profiler/trace_streamer-compatible
  `.htrace`, `perf.data`, and generated DB surfaces.
- The built-in parser targets the same semantic model as `trace_streamer`, not
  the old segment container.
- Legacy segment-format fixtures may remain only as archival tests while the
  migration is in progress. They must not drive public behavior, fallback
  selection, prompt guidance, or user-facing success criteria.
- Unknown modern DB/profiler surfaces are reported as structured coverage
  caveats, not emitted as header-only systrace rows.
- If neither `trace_streamer` nor the redesigned built-in parser can decode the
  trace body, the converter should fail or return an explicitly partial
  tracebundle. It must not imply that the old segment parser is the expected
  fallback path.

## Engine Architecture

```text
.htrace / perf.data
  -> engine selection
      -> trace_streamer DB engine       primary
      -> built-in modern parser         offline fallback
      -> raw perf.data parser           perf-only fallback
  -> exporters
      -> systrace text
      -> perftrace text
      -> tracebundle metadata
  -> trace_query
```

### Primary Engine: trace_streamer DB

Codrax should discover or accept a configured `trace_streamer`, run:

```text
trace_streamer <input> -e <output.db>
```

Then a Codrax DB exporter reads the generated SQLite database and emits:

- ftrace/systrace-compatible scheduler, wakeup, irq, cpu, clock, frame, trace
  marker, binder, IO, log, xpower, and hisys rows.
- Codrax `.perftrace` `perf_sample:` rows from DB perf tables.
- A tracebundle containing all artifacts, provider decisions, tool provenance,
  DB coverage stats, caveats, and perf/trace clock quality.

### Built-in Parser: Modern Shape Only

The built-in parser should be redesigned around modern profiler payloads and the
same semantic event families exported from the DB engine:

- profiler session/package metadata
- ftrace plugin metadata and structured event payloads
- trace marker spans
- scheduler and wakeup events
- irq/softirq events
- cpu frequency, cpu idle, and clock rates
- binder transactions and replies
- block/file IO and storage latency events
- frame slices and app startup slices
- perf sidecars and normalized perf samples

The built-in parser should emit the same stable output fields as the DB exporter
so downstream `trace_query` sees one schema regardless of the engine.

Do not keep a second public schema for old segment field offsets. If a helper is
temporarily needed during migration, isolate it behind internal tests and delete
it once the modern parser has equivalent coverage.

## Parser Migration Plan

### Batch A: Document and Gate the New Contract

- Add this design as the source of truth for `.htrace` conversion.
- Mark the 2026-06-01 binary converter design as superseded for production
  behavior.
- Add converter result metadata that distinguishes:
  - `trace_streamer_db`
  - `builtin_modern_profiler`
  - `raw_perfdata_fallback`
  - `legacy_event_segment_archival`
- Ensure user-facing output never presents `legacy_event_segment_archival` as a
  normal fallback.

### Batch B: Add trace_streamer Provider

- Add options and CLI flags:
  - `--trace-engine auto|trace_streamer|builtin`
  - `--trace-streamer <path>`
  - `--keep-db`
  - `--db-output <path>`
  - `--trace-tools-status`
- Discover `trace_streamer` through explicit flag, environment variable, `PATH`,
  and known OpenHarmony/SmartPerf locations.
- In `auto`, prefer `trace_streamer`; in explicit `trace_streamer`, fail fast on
  provider failure.

### Batch C: DB Exporter

- Add a schema-introspecting DB exporter.
- Export scheduler, wakeup, irq, cpu idle/frequency, clock, frame, trace marker,
  binder, IO, log, xpower, and hisys rows.
- Export perf tables as Codrax `.perftrace` instead of relying only on visual
  `hiperf:` trace marker spans.
- Emit coverage stats per table and per event family.

### Batch D: Built-in Modern Parser

- Refactor converter entrypoint so old `scanMetadata` is no longer the automatic
  fallback.
- Parse modern profiler/session containers and ftrace plugin payload metadata as
  first-class sources.
- Add targeted structured event renderers only for event families consumed by
  `trace_query`.
- Normalize output through the same field names as the DB exporter.

### Batch E: Remove Legacy Production Path

- Delete or quarantine old segment parser code:
  - `scanMetadata`
  - `parseEventFormats`
  - raw page event-id rendering
  - header-only unknown-event systrace output
- Rewrite tests that construct `SEGMENT_EVENTS_FORMAT`/`SEGMENT_RAW_TRACE`
  fixtures into either synthetic DB fixtures or modern profiler fixtures.
- Keep at most one archival test proving unsupported legacy segment input is
  rejected with a clear message.

### Batch F: Trace Query Round Trip

- Add round-trip tests:
  - `.htrace -> trace_streamer DB -> systrace -> trace_query`
  - `.htrace -> trace_streamer DB -> perftrace -> trace_query`
  - `.htrace -> tracebundle -> window_stats/root_cause/perf_stats`
  - explicit `trace_streamer` failure returns a hard error
  - `auto` provider failure falls back only to modern built-in parser or
    perf-only partial bundle
- Verify prompt/hint/query-result guidance names the stable artifacts and fields:
  `systrace`, `perftrace`, `tracebundle`, `trace_streamer_db`, and modern
  profiler coverage caveats.

## UX Requirements

- Conversion output must be localized with the current UI language.
- The command line and final report must say which engine handled each artifact.
- Partial output must be explicit:
  - trace body converted
  - perf sidecar converted
  - raw perf preserved only
  - DB generated and kept
  - DB generated and cleaned
  - unsupported legacy input rejected
- When multiple logs/traces/perf artifacts are attached or named in the user
  request, the prompt and reports must preserve source provenance so the model
  can explain which file contributed which evidence.

## Non-Goals

- Do not maintain the old event-format segment parser as a user-visible
  compatibility lane.
- Do not classify user intent through file suffixes or keywords. Artifact
  routing should remain model/request driven, with deterministic tooling only
  validating and opening the selected artifacts.
- Do not infer elapsed root-cause duration from perf sample counts. Perf samples
  support running/CPU-consumption analysis, while scheduler and trace events
  remain the causal timing basis.
- Do not emit unknown modern event bodies as generic systrace rows unless they
  are mapped into a stable event family that `trace_query` can consume.

## Commercial Readiness Exit Criteria

- `trace_streamer` path works for modern `.htrace` files and produces queryable
  systrace/perftrace artifacts.
- Built-in parser no longer depends on old segment layout for production
  conversion.
- The old segment parser is removed or quarantined behind archival tests.
- Tracebundle provenance makes engine choice, coverage, caveats, and partial
  results transparent to users and to downstream model stages.
- `trace_query` can consume generated artifacts without adding new model
  tool-call JSON burden.
