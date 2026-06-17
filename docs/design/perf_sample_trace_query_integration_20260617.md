# Perf Sample Trace Query Integration Plan (2026-06-17)

## Goal

Build a commercial-grade, system-level perf sample lane for Codrax so a Harmony/OpenHarmony or Android trace question can join scheduler, wakeup, binder, IO, resource pressure, and sampled CPU call stacks in one deterministic `trace_query` evidence flow.

This is not a case-specific patch. The gap is architectural: current Codrax has a strong ftrace/systrace query engine, but CPU sampling data is a separate time-series artifact. The right fix is to normalize perf samples into the same typed runtime-artifact model, preserve provenance through handoff, and expose stable views and prompts so the model can consume the data without guessing.

## Source Material

### User Reference

`/Users/han/opt/perf_query.md` asks for:

- `hiprofiler_data.htrace -> trace_convert -> Perf Sample text -> trace_query -> Trace + Perf joint analysis`
- `EventPerfSample` and `event_types=["perf_sample"]`
- `window_stats.perf_samples.top_symbols/top_dso/top_callchains`
- root-cause and frame-bundle supporting evidence from CPU samples
- views such as `perf_stats`, `perf_timeline`, and `trace_perf_bundle`
- no separate analysis framework; reuse `trace_convert`, `trace_query`, `WindowStats`, `RootCauseRank`, and `FrameRootCauseBundle`

### OpenHarmony / Gitee

OpenHarmony's Gitee organization currently states that the community moved to GitCode on 2025-09-15 while Gitee still serves as a mirror. The useful mirrored repos for this work are:

- `openharmony/developtools_profiler`
- `openharmony/developtools_hiperf`
- `openharmony/docs` for user-facing tool documentation and subsystem context

Official links:

- https://gitee.com/openharmony
- https://gitee.com/openharmony/developtools_profiler
- https://gitee.com/openharmony/developtools_hiperf
- https://gitee.com/openharmony/docs

Key findings:

- `developtools_profiler/README_zh.md` documents `hiprofiler_cmd` with `plugin_name: "hiperf-plugin"` and `record_args` such as `-f 1000 -a ... --call-stack dwarf --clockid monotonic --offcpu -m 256`.
- `developtools_profiler/device/plugins/hiperf_plugin/src/hiperf_module.cpp` declares `hiperf-plugin` as `isStandaloneFileData=true`, default `outFileName="/data/local/tmp/perf.data"`, and writes that file with `WriteStandalonePluginFile(..., DataType::HIPERF_DATA)`.
- `developtools_profiler/device/services/profiler_service/src/trace_file_header.h` defines `DataType::HIPROFILER_PROTOBUF_BIN = 0`, `HIPERF_DATA = 1`, and `STANDALONE_DATA = 1000`; the header reserves 1024 bytes and records standalone plugin name/version.
- `developtools_hiperf/README.md` documents host output `hiperf_host` and `libhiperf_report.so`; the `report` command reads `perf.data` and converts it to JSON or ProtoBuf.
- `developtools_hiperf/proto/report_sample.proto` defines the official report protobuf stream with `CallStackSample{time, tid, callStackFrame, event_count, config_name_id}`, `VirtualThreadInfo{tid,pid,name}`, `SymbolTableFile{path,function_name}`, and `ReportInfo{config_name}`.
- `developtools_hiperf/src/report_protobuf_file.cpp` writes samples from `PerfRecordSample` into that protobuf stream; this is the best supported source for per-sample time, tid, period/event count, symbol file, and function names.

Implication: OpenHarmony `.htrace` does not expose hiperf samples as ordinary `ProfilerPluginData` rows. It embeds a standalone `perf.data` segment. Codrax must extract the standalone `HIPERF_DATA` payload, then use the official hiperf parser/export path when available. Hand-writing a full raw `perf.data` parser in Go is duplicate work and higher risk.

### Android / AOSP

AOSP `simpleperf` provides the corresponding Android parser vocabulary:

- https://android.googlesource.com/platform/system/extras/+/master/simpleperf/
- https://android.googlesource.com/platform/system/extras/+/master/simpleperf/scripts/report_sample.py
- https://android.googlesource.com/platform/system/extras/+/master/simpleperf/scripts/simpleperf_report_lib.py

- `SampleStruct` includes `ip`, `pid`, `tid`, `thread_comm`, `time` in ns, `cpu`, and `period`.
- `SymbolStruct` includes `dso_name`, `symbol_name`, and addresses.
- `CallChainStructure` exposes call-chain entries with symbols.
- `report_sample.py` demonstrates a stable text report shape: `comm pid/tid [cpu] timestamp: period event:` followed by symbol and call-chain lines.

Implication: Android should use a simpleperf adapter rather than a Codrax-owned raw perf parser. Both Harmony hiperf and Android simpleperf can normalize into one Codrax `perftrace` text format.

## Current Codrax Flow

### `trace_convert`

`cmd/trace_convert.go` calls `internal/hitraceconv.ConvertFile` and produces one text systrace output. `internal/hitraceconv` parses binary hitrace metadata, event formats, cmdlines, tgids, and raw ftrace pages, then renders known ftrace events. It currently has no multi-artifact output and no extraction path for standalone `HIPERF_DATA`.

### `trace_query`

`internal/tracequery` has:

- `EventType` for sched, binder, trace_mark, block IO, filesystem, storage, IRQ, workqueue, DMA fence, resource/plugin events, and related signals.
- `Event` fields for scheduler state, wakeups, binder IPC, IO inode fields, block/storage latency, resource observations, plugin metrics, and trace spans.
- `WindowStats`, `RootCauseRank`, and `FrameRootCauseBundle` with scheduler, wakeup-chain, state-churn, runnable context, CPU supply, IO pressure, IRQ, workqueue, trace-mark, and resource sections.

It does not have:

- `EventPerfSample`
- parsed sample fields such as `period`, `event`, `symbol`, `dso`, `ip`, `callchain`
- `window_stats.perf_samples`
- perf-aware root-cause or frame-bundle handoff
- `perf_stats`, `perf_timeline`, or `trace_perf_bundle`

### Tool Schema / Prompt / JSON Repair

`internal/tool/trace_query.go` already sends tool calls through `applyStructuredPayloadCompat` before strict decoding. Schema-driven repair supports enum style aliases, explicit enum aliases, and split-string arrays. Therefore every new model-facing field must be represented in the `trace_query` schema, enum aliases, and hint/prompt teaching. Output-only fields do not need model JSON repair.

Current prompt teaching in `internal/skill/defaults.go` and `internal/skill/trace_query_views.go` covers trace spans, scheduler, binder, IO, runnable context, state churn, and path-based trace questions. It does not teach how to request perf sample views or how to interpret perf context as supporting evidence instead of a standalone hard root cause.

## System-Level Gaps

1. **Single-artifact trace model**: current code assumes one text trace file. Perf sample data may be a sidecar file or a standalone segment inside `.htrace`. The system needs a bundle model with systrace plus perftrace provenance.

2. **Converter output model**: `trace_convert` writes exactly one `.systrace`. It must be able to produce `.systrace`, `.perftrace`, and a small bundle metadata file without requiring the user to understand the split.

3. **Perf parser lane**: no `perf_sample` event type or parser exists. Without a normalized event, every downstream view would need ad-hoc parsing.

4. **Aggregation and ranking**: window stats and root-cause ranking cannot explain "what code was executing" during running, runnable competitors, binder peers, or wakeup-chain dependencies.

5. **Handoff preservation**: frame and root-cause bundles do not carry top symbols/callchains tied to target/on-chain/server/competitor roles, so rich sample evidence would be lost before final answer generation.

6. **Official parser reuse**: OpenHarmony and Android already provide parsers. Codrax should wrap their output into a stable internal text format instead of owning raw `perf.data` parsing first.

7. **Prompt/schema repair**: new views and filters must be taught and accepted through the unified JSON repair layer. The model should not need to remember exact spellings such as `perf_sample` vs `cpu_sample` when a safe alias can map them.

8. **CLI/REPL mental burden**: users should be able to provide a `.htrace`, `.perftrace`, `.perf.data`, or explicit path in a question and have Codrax attach/discover the relevant runtime artifacts. They should not have to switch modes or manually correlate sidecars.

## Target Architecture

```mermaid
flowchart TD
    H["hiprofiler_data.htrace"] --> C["trace_convert"]
    C --> S["capture.systrace"]
    C --> P["capture.perftrace"]
    C --> M["capture.tracebundle.json"]
    PD["perf.data"] --> A["official parser adapter"]
    HP["hiperf_host report --proto/json"] --> A
    SP["simpleperf report sample"] --> A
    PT["existing .perftrace"] --> TQ["trace_query bundle index"]
    A --> P
    S --> TQ
    P --> TQ
    M --> TQ
    TQ --> WS["window_stats.perf_samples"]
    TQ --> PS["perf_stats / perf_timeline"]
    TQ --> RC["root_cause_rank perf_context"]
    TQ --> FB["frame_root_cause_bundle / trace_perf_bundle"]
```

### Normalized `perftrace` Text

Codrax-owned text format, one sample per line, ftrace-like enough for `trace_query` to parse and grep:

```text
app-5678 ( 1234) [005] .... 928.081774: perf_sample: pid=1234 tid=5678 cpu=5 period=10000 event=cpu-cycles symbol=Foo::bar dso=libfoo.so ip=0x1234 callchain=main;A;B;Foo::bar
```

Rules:

- Timestamp is seconds, same unit as ftrace/systrace. Harmony hiperf `time` from monotonic ns converts to seconds.
- `pid` is process id when known; `tid` is sampled thread id.
- `period` is the sample weight/event count. If absent, count each sample as period 1.
- `event` is the hardware/software event name such as `cpu-cycles`.
- `symbol` is the leaf/hot function; `dso` is the mapped binary/library.
- `callchain` is semicolon-separated from root to leaf when the adapter can determine order; parser records the raw string and does not infer missing frames.
- Values may be quoted when they contain whitespace. Parser must preserve symbol/dso text and avoid path invention.

### Event Model

Add `EventPerfSample` and perf fields to `tracequery.Event`:

- `perf_pid`, `perf_tid`, `perf_comm`
- `perf_period`, `perf_event`
- `perf_symbol`, `perf_dso`, `perf_ip`
- `perf_callchain`
- optional `perf_source` and `perf_clock`

The raw `FieldText` remains available for event_search. `event_types=["perf_sample"]` filters only sample rows.

### Aggregation

Add a reusable `PerfContext`:

```json
{
  "sample_count": 0,
  "total_period": 0,
  "top_symbols": [],
  "top_dso": [],
  "top_callchains": [],
  "top_threads": [],
  "top_events": [],
  "coverage": {}
}
```

Each hotspot row should carry:

- `symbol`, `dso`, `event`
- `sample_count`, `period`, `percent`
- `threads`, `cpus`
- `line_start`, `line_end`, `example`

`window_stats.perf_samples` is the broad same-window summary. Role-specific contexts should be separate in causal views:

- `target_running_perf`: samples on the target thread while it was running
- `on_chain_perf`: samples on wakeup-chain dependency threads
- `binder_peer_perf`: samples on synchronous-looking binder peer/server threads
- `same_cpu_competitor_perf`: samples on threads running on CPUs relevant to target runnable delay

### Causal Semantics

Perf samples are supporting evidence for "what was executing"; they are not by themselves proof of a scheduling root cause.

Use them as:

- running-state explanation: target spent CPU in top symbols during frame/startup window
- runnable-delay explanation: same-CPU competitors or waker threads consumed CPU in top symbols during target wait
- wakeup-chain explanation: on-chain upstream dependency thread was runnable/running/D-state plus had hot callchain evidence
- binder explanation: blocking candidate shows peer/server state and peer samples
- IO explanation: D-state/iowait plus IO pressure remains primary; samples annotate code path if present

Ranking still starts from deterministic intervals, overlap, and chain relevance. Perf contexts attach to ranked candidates and help explain code-level hotspots.

### Tool Views

Add three views:

- `perf_stats`: aggregate samples in a window by symbol, dso, callchain, thread, CPU, and event.
- `perf_timeline`: bucket sample periods by time for a target thread, process, CPU, event, symbol, or dso.
- `trace_perf_bundle`: line-backed joint bundle combining window stats, root-cause rank, wakeup/binder chain evidence, and role-specific perf contexts.

Existing views should be enhanced:

- `window_stats`: include `perf_samples`.
- `root_cause_rank`: attach `perf_context` to relevant candidates without making samples a standalone hard gate.
- `frame_root_cause_bundle`: include role-specific perf contexts and next-call guidance.
- `event_search`: support `event_types=["perf_sample"]` and literal search over symbol/dso/callchain/thread/event.

### Converter / Adapter Strategy

Batch implementation should prefer official parsers:

1. `.perftrace` direct parser: always available; tests and evals can use synthetic text.
2. `.htrace` standalone extractor: find nested `TraceFileHeader` with `DataType::HIPERF_DATA` and write the payload as `.perf.data`.
3. OpenHarmony adapter: call configured `hiperf_host report --proto` or `--json` when available, then convert to `.perftrace`.
4. Android adapter: call configured `simpleperf` report sample/export path when available, then convert to `.perftrace`.
5. Optional later fallback: parse hiperf report protobuf stream directly in Go if `hiperf_host` is absent. Avoid raw `perf.data` parsing unless an official parser is unavailable and the scope is explicitly limited.

## JSON Repair / Prompt Contract

Model-facing additions must enter all three surfaces:

- schema: `view` enum and aliases for `perf_stats`, `perf_timeline`, `trace_perf_bundle`; `event_types` description and aliases for `perf_sample`, `cpu_sample`, `sample`, `top_symbols`, `callchain`.
- repair: rely on existing `applyStructuredPayloadCompat` plus schema markers; add tests that string arrays and friendly aliases normalize correctly.
- prompt/hints: teach when to use perf views, how to interpret perf context, and that `.htrace`/`.perf.data`/`.perftrace` path questions should stay on runtime-artifact tooling.

Do not add hard gates that inspect model prose. Use typed tool fields and deterministic output only.

## Batches

### Batch A: Core Perftrace Parser and Window Aggregation

- Add `EventPerfSample` and perf fields.
- Parse `perf_sample` ftrace-like lines.
- Add `event_types=["perf_sample"]`.
- Add `PerfContext` aggregation and `window_stats.perf_samples`.
- Add parser/window unit tests.

### Batch B: Tool Views, Schema, Prompt, and Repair

- Add `perf_stats`, `perf_timeline`, and `trace_perf_bundle` view enum rows.
- Update `internal/skill/trace_query_views.go` and trace-query-first prompt teaching.
- Add enum aliases and event type aliases in schema/normalization tests.
- Add summary/hint output so empty perf results guide the next call.

### Batch C: Causal Join and Handoff

- Attach perf contexts to root-cause candidates by interval overlap, pid/tid/cpu, and chain relevance.
- Add role-specific perf contexts to `FrameRootCauseBundle`.
- Add binder peer/server sample join for sync-looking blocking candidates.
- Add same-CPU competitor sample join for runnable root causes.
- Preserve top evidence rows with lines/examples.

### Batch D: Converter Multi-Artifact Output

- Extend `hitraceconv.Result` to report multiple outputs.
- Extract standalone `HIPERF_DATA` payloads from `.htrace` into `.perf.data`.
- Add adapter command plumbing for `hiperf_host` and simpleperf when configured/found.
- Emit `.perftrace` and `.tracebundle.json` with provenance.
- Keep `.systrace` behavior stable for traces without perf data.

### Batch E: CLI/REPL and Attachment UX

- Auto-discover sibling `.perftrace` / `.tracebundle.json` when users attach or mention a trace path.
- Keep read mode default; do not require users to enter write mode for analysis.
- Ensure explicit path questions with one or more runtime artifacts route to `trace_query(path)` instead of source-code analysis.
- Update user guide examples.

### Batch F: Evals and Regression Guard

- Add minimal synthetic perftrace evals without over-prebaked analysis.
- Add trace+perf bundle eval covering runnable + waker samples.
- Add binder peer perf eval.
- Add `.htrace` converted sidecar fixture eval once converter support lands.
- Run selected existing trace evals in pairs to guard no regression in trace-only flows.

## Test Plan

Unit tests:

- `go test ./internal/tracequery`
- `go test ./internal/tool -run TraceQuery`
- `go test ./internal/hitraceconv`

Coverage targets:

- parser accepts `perf_sample` text and preserves symbol/dso/callchain
- `event_search` finds samples by symbol, dso, callchain, and event type
- `window_stats.perf_samples` reports top symbols/dso/callchains
- `perf_stats` and `perf_timeline` are line-backed and bounded by time/thread/pid filters
- `root_cause_rank` keeps scheduler/chain ranking primary while attaching perf context
- `frame_root_cause_bundle` preserves on-chain and binder-peer perf context
- `trace_convert` emits sidecar perf artifacts only when data exists
- schema repair accepts friendly aliases and split-string `event_types`

Eval targets:

- a direct `.perftrace` query
- a trace + perf sidecar query with runnable delay and same-CPU competitor samples
- a wakeup-chain query with on-chain thread samples
- a binder wait query with peer/server samples
- existing Donghu/Harmony trace cases, state-churn cases, inode IO cases, and explicit path trace cases

## Delivery Checklist

- [ ] Batch A implemented, committed, pushed.
- [ ] Batch B implemented, committed, pushed.
- [ ] Batch C implemented, committed, pushed.
- [ ] Batch D implemented, committed, pushed.
- [ ] Batch E implemented, committed, pushed.
- [ ] Batch F evals added and representative cases run two at a time.

## Open Decisions

- Exact location/config key for `hiperf_host` and simpleperf binaries. Prefer auto-discovery plus optional config, not hard-coded paths.
- Whether to parse hiperf report protobuf directly in Go in Batch D or keep it as Batch F fallback.
- Whether `trace_perf_bundle` should be a new concrete view or an alias to enhanced `frame_root_cause_bundle` when a frame/span window is selected. Initial plan keeps it concrete because non-frame startup/binder/runnable windows also benefit from joint output.
