# HiTrace trace_streamer and Built-in Parser Redesign

Date: 2026-06-22

Reference: https://gitcode.com/diting/hmtrace/tree/main

## Goal

Modern Harmony/OpenHarmony `.htrace` conversion must follow the same high-coverage
shape as `hmtrace`: first normalize the capture through `trace_streamer` into a
structured database, then export Codrax-native `.systrace`, `.perftrace`, and
`.tracebundle.json` artifacts for `trace_query`.

This redesign makes `auto` the default trace-body selection. For pure trace
captures, `auto` uses `trace_streamer` DB export when `trace_streamer` is
discovered, falls back to the built-in trace-only converter only when
`trace_streamer` is absent, and does not fall back after a `trace_streamer`
execution or DB-export failure. Trace+perf captures are SQL-only. The existing
`SEGMENT_EVENTS_FORMAT` + `SEGMENT_RAW_TRACE` parser remains an explicitly
selectable transition lane for no-perf Harmony/Donghu `.sys`-style binary
captures until the DB engine proves full parity for that shape. Runtime
conversion must choose exactly one trace-body engine: auto-selected SQL or
built-in for pure trace, explicit SQL, or explicit built-in for pure trace. Once parity is proven by
round-trip tests and customer-style fixtures, Codrax can retire the built-in sys
binary lane without keeping backward compatibility solely for its own sake.

## Current Gap

The current converter still has a raw event-page path:

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

This path is useful for existing no-perf Harmony/Donghu `.sys` binary traces,
but it is not the right parser for modern profiler packages. Customer profiler
captures such as `hiprofiler_data.htrace` can carry modern
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

## Compatibility and Retirement Contract

The new contract is DB-first by default, with a measured retirement gate for the
explicit built-in sys binary parser.

- Production conversion supports modern profiler/trace_streamer-compatible
  `.htrace`, `perf.data`, and generated DB surfaces.
- Existing no-perf Harmony/Donghu `.sys` binary conversion remains supported
  through explicit `--trace-engine=builtin` until `trace_streamer` DB export
  demonstrates equivalent or stronger coverage for the same inputs.
- The built-in parser targets the same semantic model as `trace_streamer`, not
  a second public schema. Sys binary fixtures are parity guards, not the
  long-term canonical format.
- Unknown modern DB/profiler surfaces are reported as structured coverage
  caveats, not emitted as header-only systrace rows.
- If a discovered or explicitly selected `trace_streamer` cannot decode the
  trace body, the converter should fail or return an explicitly partial
  tracebundle. It must not silently try the other trace-body engine after SQL
  execution has begun, and it must not imply that the sys binary parser is the
  expected fallback for trace+perf profiler captures.

## Engine Architecture

```text
.htrace / perf.data
  -> engine selection
      -> trace_streamer DB engine       default trace-body engine
      -> built-in modern/sys parser     explicit trace-only engine
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

### Built-in Parser: Explicit Trace-Only Engine

The built-in parser should be selectable only for trace-only conversion. It
should be redesigned around modern profiler payloads and the same semantic event
families exported from the DB engine:

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

### Built-in Sys Binary Parser: Parity-Gated Lane

The current `SEGMENT_EVENTS_FORMAT` + raw trace page parser is kept only as an
explicit built-in engine for the existing no-perf Harmony/Donghu `.sys` binary
capability. It must emit the same systrace fields and tracebundle provenance as
the DB exporter, and it must be covered by round-trip tests. It can be removed
after these gates pass:

- `trace_streamer` DB engine accepts representative no-perf `.sys` captures.
- DB exporter emits all event families currently rendered by the sys binary
  parser.
- Generated systrace round-trips through `trace_query` with no loss in scheduler,
  wakeup, CPU, binder, IRQ, and IO evidence used by root-cause analysis.
- User-facing output and tracebundle provenance make the engine selection
  explicit. Parity tests may run both engines, but production conversion must
  not run both or silently fall back between them.

Do not keep a second public schema for raw segment field offsets. While the sys
binary lane exists, it is a compatibility implementation detail behind stable
systrace/perftrace/tracebundle artifacts.

## Parser Migration Plan

### Batch A: Document and Gate the New Contract

- Add this design as the source of truth for `.htrace` conversion.
- Mark the 2026-06-01 binary converter design as superseded for production
  behavior.
- Add converter result metadata that distinguishes:
  - `trace_streamer_db`
  - `builtin_modern_profiler`
  - `builtin_sys_binary`
  - `raw_perfdata_fallback`
- Ensure user-facing output presents `builtin_sys_binary` only as the explicit
  no-perf sys binary lane, never as an automatic fallback or the expected parser
  for modern profiler packages.

### Batch B: Add trace_streamer Provider

- Add options and CLI flags:
  - `--trace-engine auto|trace_streamer|builtin`
  - `--trace-streamer <path>`
  - `--keep-db`
  - `--db-output <path>`
  - `--trace-tools-status`
- Discover `trace_streamer` through explicit flag, environment variable, `PATH`,
  and known OpenHarmony/SmartPerf locations.
- The default/`auto` engine uses `trace_streamer` when discovered. If
  `trace_streamer` is absent and the input is pure trace, `auto` uses the
  built-in trace-only converter. If `trace_streamer` is discovered but execution
  or DB export fails, `auto` does not fall back. Explicit `trace_streamer` fails
  fast on missing provider. Trace+perf remains SQL-only.

### Batch C: DB Exporter

- Add a schema-introspecting DB exporter.
- Export scheduler, wakeup, irq, cpu idle/frequency, clock, frame, trace marker,
  binder, IO, log, xpower, and hisys rows.
- Export perf tables as Codrax `.perftrace` instead of relying only on visual
  `hiperf:` trace marker spans.
- Emit coverage stats per table and per event family.

### Batch D: Built-in Modern Parser

- Refactor converter entrypoint so old `scanMetadata` is no longer the automatic
  fallback and only runs when `--trace-engine=builtin` is selected.
- Parse modern profiler/session containers and ftrace plugin payload metadata as
  first-class sources.
- Add targeted structured event renderers only for event families consumed by
  `trace_query`.
- Normalize output through the same field names as the DB exporter.

### Batch E: Sys Binary Parity Gate and Retirement

- Prove or disprove `trace_streamer` DB parity against no-perf Harmony/Donghu
  `.sys` fixtures.
- If parity is complete, delete the built-in sys binary parser code:
  - `scanMetadata`
  - `parseEventFormats`
  - raw page event-id rendering
  - header-only unknown-event systrace output
- If parity is not complete, keep the built-in sys binary parser as a guarded
  production lane and document the uncovered event families.
- Rewrite old tests into DB/profiler fixtures only after parity is complete.
  Until then, keep `.sys` round-trip tests as commercial regression guards.

### Batch F: Trace Query Round Trip

- Add round-trip tests:
  - `.htrace -> trace_streamer DB -> systrace -> trace_query`
  - `.htrace -> trace_streamer DB -> perftrace -> trace_query`
  - `.htrace -> tracebundle -> window_stats/root_cause/perf_stats`
  - explicit `trace_streamer` failure returns a hard error
  - `auto` missing-provider pure trace falls back to the built-in parser
  - `auto` trace_streamer execution/DB failure does not fall back
  - trace+perf without query-ready SQL returns a perf/tracebundle partial bundle
- Verify prompt/hint/query-result guidance names the stable artifacts and fields:
  `systrace`, `perftrace`, `tracebundle`, `trace_streamer_db`, and modern
  profiler coverage caveats.
- Ensure tracebundle handoff is symmetric:
  - perf provenance from `provider_decisions` and `perf_clock_alignments`;
  - trace provenance from `trace_provider_decisions`;
  - DB export coverage from `trace_db_coverage`;
  - generated-trace/parser cross-validation from `trace_coverage`.
- Keep these as query-result provenance fields, not model tool-call inputs.
  Therefore they require prompt/hint teaching for interpretation, but no JSON
  repair aliases unless a future tool input field is added.

## UX Requirements

- Conversion output must be localized with the current UI language.
- The command line and final report must say which engine handled each artifact.
- Partial output must be explicit:
  - trace body converted
  - perf sidecar converted
  - raw perf preserved only
  - DB generated and kept
  - DB generated and cleaned
  - sys binary parser used
  - sys binary parser unsupported for this input
- When multiple logs/traces/perf artifacts are attached or named in the user
  request, the prompt and reports must preserve source provenance so the model
  can explain which file contributed which evidence.

## Non-Goals

- Do not maintain the built-in sys binary parser after DB parity is proven.
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
- No-perf Harmony/Donghu `.sys` binary conversion is either fully covered by
  `trace_streamer` DB export, or explicitly guarded by the built-in sys binary
  parser with round-trip tests and transparent provenance.
- The built-in sys binary parser is removed only after DB parity is proven.
- Tracebundle provenance makes engine choice, coverage, caveats, and partial
  results transparent to users and to downstream model stages.
- `trace_query` can consume generated artifacts without adding new model
  tool-call JSON burden.
