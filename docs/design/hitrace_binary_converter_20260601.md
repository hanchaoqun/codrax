# Binary HiTrace Converter Design

Date: 2026-06-01

Status: Superseded for production conversion by
`docs/design/hitrace_trace_streamer_parser_redesign_20260622.md`.

The original design targeted the `SEGMENT_EVENTS_FORMAT` + `SEGMENT_RAW_TRACE`
event-format segment container. That container is no longer the canonical
commercial target for modern `.htrace` work, which must follow the
trace_streamer/modern-profiler redesign. The implementation can still keep this
parser as the built-in sys binary lane for existing no-perf Harmony/Donghu
`.sys` captures until trace_streamer DB parity is proven; after parity is
proven, backward compatibility with this parser is not required.

## Problem

Codrax can already analyze ftrace-compatible text traces through `--htrace`,
`/htrace`, and `trace_query`, but Harmony/OpenHarmony users may also have binary
HiTrace captures. Today those files must be converted outside Codrax before the
existing deterministic trace analysis stack can read them.

OpenHarmony ships a Python converter under
`tools/hitrace_converter`. Its binary path reads the HiTrace file header,
segment headers, event format tables, saved command names, TGID mappings, raw
trace pages, then emits ftrace/systrace-style text rows. Codrax should provide a
Go implementation of the same ingestion shape so operators do not need a
separate Python environment.

## Non-Goals

- Do not auto-attach binary HiTrace files.
- Do not auto-convert during ordinary `trace_query` calls.
- Do not change source-code analysis, `repo_map`, evidence gates, or source
  citation semantics.
- Do not reinterpret trace symbols as current source-code symbols.
- Do not make conversion an LLM-invoked exploratory tool in the first rollout.

## User-Facing Contract

Manual CLI conversion:

```bash
codrax trace convert --input foo.htrace.bin
codrax trace convert --input foo.htrace.bin --output foo.systrace
```

Manual REPL conversion:

```text
/htrace convert foo.htrace.bin
/htrace convert foo.htrace.bin foo.systrace
```

Rules:

- Conversion is manual and read-only with respect to the input file.
- If `--output` or the second REPL argument is omitted, Codrax writes to
  `<input>.systrace`.
- If the output file already exists, Codrax fails loudly and tells the user to
  delete the file or choose a different output path.
- Conversion does not attach the generated trace by default. Users can attach it
  explicitly with `--htrace <output>` or `/htrace <output>`.
- Future optional convenience may add an explicit `--attach` flag, but the
  default must stay "convert only".

## Go Package Plan

Add `internal/hitraceconv`:

- `convert.go`: public `ConvertFile(ctx, Options)` entrypoint.
- `format.go`: binary constants and segment/page/event header readers.
- `eventformat.go`: parser for ftrace event format metadata.
- `render.go`: known-event renderers plus fallback rendering.
- `types.go`: options, result, progress, caveats.
- `convert_test.go`: synthetic binary fixtures and text-output verification.

The converter emits ftrace-compatible text so the existing
`internal/tracequery` parser remains the single deterministic trace-analysis
engine.

## Binary Format Scope

V1 reads:

- file header
- segment headers
- `SEGMENT_EVENTS_FORMAT`
- `SEGMENT_CMDLINES`
- `SEGMENT_TGIDS`
- `SEGMENT_RAW_TRACE` and per-CPU raw trace segments
- page header
- event header
- event content identified by event id

V1 strongly renders:

- `sched_switch`
- `sched_wakeup`
- `sched_waking`
- `sched_blocked_reason`
- `cpu_idle`
- `cpu_frequency`
- `clock_set_rate`
- `block_rq_issue`
- `block_rq_complete`
- `block_bio_remap`
- `binder_transaction`
- `binder_transaction_received`
- `irq_handler_entry`
- `irq_handler_exit`
- trace marker `B/E/C` rows where format fields expose the payload

Unknown events are preserved as weak structured rows with event name and
field/value fallback when possible. Missing event formats are counted and
reported as caveats.

## Safety

- Use bounded reads and validate every segment size before seeking/reading.
- Use `O_EXCL`/equivalent output creation so conversion never overwrites user
  files.
- Keep timestamps in integer nanoseconds internally; render seconds with
  microsecond precision, matching ftrace text.
- Preserve Harmony flavor intent for downstream priority semantics.
- Report conversion stats: input bytes, output bytes, event count, missing
  format count, first/last timestamp, output path.

## Rollout Tasks

- [x] Land this design and task ledger.
- [x] Raise default log/trace attachment cap from 256 MiB to 512 MiB and update
      examples and user docs.
- [x] Add `internal/hitraceconv` parser and writer.
- [x] Add `codrax trace convert` command.
- [x] Add REPL `/htrace convert` command.
- [x] Add tests for default output path, existing-output refusal, known event
      rendering, unknown event fallback, and `tracequery.BuildIndex` compatibility.
- [x] Update user guide HTML/Markdown and README.
- [x] Add eval fixture using converted synthetic Harmony HiTrace text.
