# HiTrace Built-in Modern Parser Full Compatibility Plan

Date: 2026-06-23

Parent plan:
`docs/design/hitrace_trace_streamer_delivery_plan_20260622.md`

Reference implementation:
`https://gitcode.com/diting/hmtrace/tree/main`

Reference snapshot audited locally:
`/tmp/codrax-ref-hmtrace` at
`6b05b2a60456910f05c149012b0d4833faa2d10e`.

## Objective

Batch 5 must not stop at an MVP exporter. The built-in modern parser is a
production offline fallback for modern OpenHarmony/Harmony profiler packages
when the primary `trace_streamer` DB engine is unavailable or cannot be used. It
must therefore preserve the same downstream contract as the DB exporter:

- stable systrace rows for event families consumed by `trace_query`;
- stable `perf_sample:` rows when perf data can be normalized by the existing
  raw/official perf paths;
- tracebundle provenance that explains source, coverage, skipped families, and
  confidence without model guessing;
- bounded memory for large captures;
- tests that mirror hmtrace-style table/family coverage and then round-trip
  through Codrax `trace_query`.

The parser is not allowed to claim compatibility merely because it extracted
some embedded text rows. Partial decoding is acceptable only when the result is
explicitly marked partial and not presented as full trace-query-ready coverage.

## Current Audit

Current code path:

```text
ConvertFile
  -> maybeRunTraceStreamerAuto
  -> extractStandaloneArtifacts
  -> tryConvertProfilerContainer
  -> scanMetadata legacy sys parser
```

Observed implementation gaps:

- `tryConvertProfilerContainer` can detect modern profiler headers and
  SessionJSON packages, but it primarily extracts already-rendered text payloads.
- `ftrace-plugin` structured protobuf payloads are summarized through
  `decodeProfilerFtraceSummary`; they do not emit queryable event rows.
- Extracted text rows are accumulated in memory and sorted as one slice. The DB
  exporter already has a spillable row sink; the modern parser does not reuse it
  yet.
- Coverage for the modern parser is stored only as caveat strings. It does not
  have per-source/per-family counters comparable to `trace_db_coverage`.
- Structured ftrace messages currently increment `UnknownEventCount`, which is
  useful transparency but not enough handoff for model consumption.
- Session packages are line-scanned for systrace-shaped text, but package
  metadata and section coverage are not first-class.
- The parser does not yet provide full parity tests against the same family
  matrix hmtrace uses for DB export.

Important existing strengths to reuse:

- `traceDBRowSink` already provides deterministic global ordering with
  spill-to-disk behavior.
- DB exporter row renderers already normalize scheduler, wakeup, IRQ, CPU,
  clock, frame, log, IO/counter, hisys/xpower, trace marker, and perf-sample
  output into the schema expected by `trace_query`.
- Trace provider decisions and `tracebundle.json` already carry selected
  provider, artifacts, caveats, and DB coverage.
- Standalone perf sidecar extraction and raw/official perf normalization already
  preserve perf provenance.

## Compatibility Definition

The built-in modern parser is complete only when all of these are true:

1. Modern profiler/session packages are selected by structural content
   inspection, not by file suffix or user-intent keywords.
2. Existing no-perf Harmony/Donghu `.sys` legacy conversion is not regressed
   while Batch 6 parity is still open.
3. Text profiler payloads and SessionJSON payloads are streamed into the shared
   bounded row sorter, not globally materialized before sorting.
4. Structured profiler/ftrace payloads produce either:
   - normalized queryable rows for the event families already supported by the
     DB exporter, or
   - explicit structured coverage that marks the family as unsupported/partial
     and prevents the result from being mistaken for complete coverage.
5. Modern parser coverage is serialized into tracebundle with the same
   machine-readable style as DB coverage. It must tell the model which plugin,
   section, event family, row count, emitted count, skip reason, parser clock,
   and confidence were observed.
6. Output rows use the same stable fields as the DB exporter. `trace_query`
   should not need separate downstream logic for the same event family just
   because the source was built-in modern parser rather than trace_streamer DB.
7. Tests include hmtrace-inspired fixtures for comprehensive family coverage,
   raw/text output, process/thread metadata, scheduler/wakeup, IRQ, CPU/clock,
   frame/callstack, IO/counter, log/hisys/xpower, perf samples, and negative
   structured partial coverage.
8. Converted outputs round-trip through `tracequery.BuildIndex` and
   `ComputeWindowStats` for the fields used by root-cause analysis.

## Non-Goals and Red Lines

- Do not add keyword matching over user prose or model prose.
- Do not route by file suffix alone. The converter can inspect file content and
  validated artifact metadata.
- Do not emit generic unknown event bodies as systrace rows. Unknown structured
  payloads become coverage/caveat records.
- Do not create a second public schema for the built-in parser. The public
  surfaces remain systrace/perftrace/tracebundle and trace_query output fields.
- Do not add model tool-call JSON fields for this batch unless a tool input
  actually changes. If that happens, the unified JSON repair/compat layer must
  be updated in the same batch.
- Do not declare Batch 5 complete while structured ftrace remains summary-only
  for event families needed by scheduler/root-cause analysis.

## Architecture

### Source Classification

Modern built-in parsing starts from validated file content:

- Profiler `TraceFileHeader` at a verified offset.
- Length-prefixed `ProfilerPluginData` messages.
- Session package marker and section layout such as `SessionJSON-`.
- Extracted sidecar descriptors produced by `extractStandaloneArtifacts`.

This is structural artifact classification inside the converter. It is not a
user-intent classifier and must not parse natural-language prompts.

### Intermediate Model

Add an internal modern-source extraction model:

```text
modernProfilerExtraction
  source kind
  plugin messages
  session sections
  row sink stats
  modern coverage records
  caveats
  trace provider decision
```

The row path should feed `renderedRow` values into the same spillable sorter used
by DB export. If the sorter type name remains `traceDBRowSink`, document the
reuse or rename it in a small mechanical follow-up to a generic systrace row
sink. Do not duplicate sorter logic.

### Coverage Model

Reuse the existing `TraceDBCoverage` shape only if it can describe non-DB
sources without ambiguity. Otherwise add a parallel generic coverage field such
as `trace_coverage` while keeping `trace_db_coverage` for DB tables. In either
case, tracebundle must expose:

- provider/source: `builtin_modern_profiler`;
- source unit: plugin name, session section, or sidecar;
- family/table-like label;
- found/rows read/rows emitted;
- skipped reason;
- parser clock and clock confidence when known;
- row sorter peak/spill statistics.

Prompt/handoff updates remain Batch 7 unless new tool-call inputs are added.

### Event Renderers

Structured renderers must target the same families as the DB exporter:

- process/thread metadata and trace marker spans;
- scheduler switch and wakeup;
- IRQ/softirq;
- CPU frequency/idle and clock-rate counters;
- binder transaction/reply families;
- block/file IO and storage latency families;
- frame/callstack/app startup slices;
- log/hisys/xpower/counters;
- perf samples through the existing perf sidecar paths.

Field names must match current DB exporter output and existing trace_query
parsers. For IO this means preserving fields such as `dev`, `ino`, `entry_name`,
`offset`, `bytes`/`len`, `rw`, `ret`, and latency where the source provides
them. For running analysis this means preserving CPU/core/frequency inputs
available in the trace.

### Structured ftrace Decoder Strategy

The full compatibility path is:

1. Audit OpenHarmony profiler proto definitions used by `ftrace-plugin`
   (`TracePluginResult`, CPU stats/detail, event bundles, symbols, and clocks).
2. Build or generate a minimal typed decoder for the query-relevant event
   families. Prefer generated descriptors or small typed decoders over ad-hoc
   string extraction.
3. Decode event metadata/format records once into a registry.
4. Decode event records by event id into typed family records.
5. Render through the same formatter functions used by DB export where possible.
6. If a source payload version contains a family not yet supported, emit
   structured coverage with `skipped_reason`, not header-only rows.

This is deliberately broader than the current summary-only parser. A partial
decoder is not a completed Batch 5 outcome unless the unsupported families are
outside trace_query's current consumption surface and explicitly documented.

## Batch 5 Task Breakdown

### Batch 5A: Modern Parser Coverage and Bounded Text Extraction

Tasks:

- Introduce modern parser coverage records and tracebundle serialization.
- Stream text profiler payload rows into the shared spillable row sorter.
- Stream SessionJSON/package text rows into the shared spillable row sorter.
- Preserve first/last timestamps and event counts from sorter stats.
- Keep provider decisions accurate:
  - success only when queryable rows were emitted;
  - partial when only sidecars/coverage were produced;
  - failure when a detected modern package cannot produce rows or useful
    sidecars.
- Add tests:
  - text plugin with many out-of-order rows spills to disk and sorts;
  - SessionJSON package rows use the same sorter path;
  - tracebundle includes modern coverage and sorter stats;
  - no legacy invalid-segment caveat appears for detected modern packages.

Exit:

- Text/session modern payloads are production bounded and machine-readable in
  tracebundle. Structured ftrace may still be partial, but the coverage must say
  so precisely.

### Batch 5B: Structured ftrace Schema Audit and Decoder Skeleton

Tasks:

- Audit OpenHarmony profiler proto/schema definitions and record source paths or
  commit hashes in this plan.
- Add typed parsing structs for the structured ftrace message families needed by
  trace_query.
- Add parser tests using synthetic protobuf payloads that do not rely on user
  question keywords.
- Decode symbols/clocks/event-format metadata into a registry.
- Preserve unsupported-family coverage without creating header-only rows.

Exit:

- Structured ftrace payloads no longer collapse to a single summary-only caveat.
  The parser can distinguish supported, unsupported, empty, and malformed
  structured payloads in typed coverage.

### Batch 5C: Structured Family Renderers

Tasks:

- Render scheduler and wakeup families.
- Render IRQ/softirq families.
- Render CPU frequency/idle/clock counters.
- Render binder transaction/reply families.
- Render block/file IO/storage latency families.
- Render trace marker/frame/app startup/callstack families.
- Reuse DB exporter formatting helpers or extract shared helpers when needed.
- Add `trace_query` round-trip assertions for each emitted family.

Exit:

- Built-in structured ftrace output is queryable for the same root-cause fields
  as trace_streamer DB output for the covered families.

### Batch 5D: hmtrace-Style Full Compatibility Tests

Tasks:

- Mirror hmtrace `golden_diff.rs` coverage with Go fixtures:
  - covered fixture;
  - raw/text fixture;
  - comprehensive fixture.
- Add Codrax-specific checks for:
  - tracebundle provenance;
  - modern coverage;
  - trace_query `window_stats` fields;
  - perf sidecar merge;
  - negative partial-structured payload.
- When a local `trace_streamer` binary is available, add optional parity tests:
  - run trace_streamer DB export;
  - run Codrax DB exporter;
  - compare semantic family coverage and trace_query outputs against built-in
    modern parser fixtures.

Exit:

- Test coverage proves the built-in modern parser matches the DB export contract
  for query-relevant semantics. Byte-for-byte equality is not required where
  Codrax intentionally emits richer query fields, but every intentional
  difference must be documented in the test.

### Batch 5E: UX, Handoff, and Stability Closure

Tasks:

- Ensure localized CLI/REPL conversion messages show:
  - selected trace engine;
  - generated systrace/perftrace/tracebundle;
  - modern parser coverage;
  - partial-vs-query-ready state;
  - next attach/query step.
- Ensure markdown/html report provenance can surface the same artifact/coverage
  information through existing handoff paths.
- Audit prompts/hints only for stable artifact semantics. Do not add prompt
  red-line keyword gates.
- Audit JSON repair only if a model tool input changed. If not, document that no
  new repair alias is required.

Exit:

- Models and users see enough typed provenance to consume the artifacts without
  guessing why a conversion is partial or complete.

## Verification Matrix

Run after each implementation batch:

```bash
go test ./internal/hitraceconv ./cmd ./internal/repl
```

Run before marking Batch 5 complete:

```bash
go test ./internal/hitraceconv ./internal/tracequery ./cmd ./internal/repl ./internal/agent ./internal/tool ./internal/types
```

Required focused tests:

- `TestConvertFileRendersOfficialProfilerTraceFileTextPayload`
- `TestConvertFileHandlesSessionJSONPackageWithPerfSidecar`
- structured ftrace coverage and renderer tests added in Batch 5B/5C
- large-row spill/sort test for built-in modern text extraction
- tracebundle modern coverage serialization test
- trace_query round-trip for scheduler/wakeup/IRQ/CPU/binder/IO/frame/perf rows
- no-perf `.sys` legacy guard tests until Batch 6 retires or gates the parser

Optional but preferred when tooling exists locally:

- trace_streamer DB export parity for a representative modern `.htrace`;
- trace_streamer DB export parity for a representative no-perf Harmony/Donghu
  `.sys` capture.

## Completion Bar

Batch 5 is not complete until:

- no modern profiler structured payload needed by trace_query is silently
  reduced to summary-only output;
- all emitted rows are generated through bounded ordering;
- tracebundle exposes machine-readable modern coverage;
- hmtrace-style comprehensive fixtures pass;
- trace_query consumes the generated artifacts without source-specific special
  cases;
- user-facing conversion output clearly distinguishes complete query-ready
  output from partial sidecar/coverage output.
