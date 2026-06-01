# Trace Query SmartPerf Host P0/P1 Delivery Plan

Date: 2026-06-01

Parent audits and sources:

- `docs/design/hitrace_converter_tracequery_coverage_audit_20260601.md`
- `docs/design/hitrace_tracequery_gap_delivery_plan_20260601.md`
- OpenHarmony SmartPerf Host `master`:
  `https://gitee.com/openharmony/developtools_smartperf_host`

## Scope

This plan implements the P0 and P1 trace-analysis gaps identified after
reviewing SmartPerf Host scheduling, FrameTimeline, eBPF BIO/FileSystem/Page
Fault, XPower, HiSystemEvent, and Ability Monitor docs/code.

P2 work is intentionally deferred.

## Red Lines

- Keep all additions inside runtime trace/log artifact analysis. Normal
  code/config analysis must not expose, invoke, or pay for these paths unless a
  runtime trace/log artifact is present or explicitly named.
- Do not change `repo_map`, source citation gates, or source-code evidence
  semantics.
- Runtime trace observations stay in the runtime-artifact lane. Do not
  reinterpret trace symbols as current-source symbols.
- Precise parsed fields may drive deterministic facts. Broad subsystem matches
  may only drive soft summaries, rankings, and caveats.
- Donghu follows Harmony/OpenHarmony scheduler semantics:
  - timestamp unit is seconds
  - larger numeric user priority means higher priority
  - `1..40=CFS`, `41..139=RT`
  - Android-framework and Harmony-framework surfaces may coexist at process
    boundaries only
- New tool parameters must be reflected in tool schema, model-facing teaching,
  call summaries, and the shared JSON compatibility layer when the field shape
  is likely to be produced loosely by models.
- Large result surfaces must stay bounded through summaries, rowsets, payload
  refs, or blobs. No unbounded trace rows in the REPL/CLI panel or prompt.

## Current Code Baseline

Relevant implementation points:

- `internal/tracequery/flavor.go` detects Harmony/Android/generic trace flavor
  and already maps explicit `东湖` platform to Harmony trace semantics.
- `internal/tracequery/query.go` resolves `TraceFlavor`, `Platform`,
  `FrameworkMode`, framework surfaces, and dispatches deterministic views.
- `internal/tracequery/parse.go` normalizes ftrace-compatible text rows into
  `Event` records and currently has typed lanes for scheduler, CPU
  idle/frequency, block IO, binder transaction/received, IRQ/softirq,
  trace marks, memory, storage, filesystem, power, workqueue, and DMA fence.
- `internal/tracequery/ipc.go` builds basic binder send/receive edges and
  synchronous-looking binder wait candidates from scheduler context.
- Existing views include `thread_timeline`, `window_stats`,
  `scheduler_latency_stats`, `ipc_graph`, `wakeup_chain`,
  `root_cause_rank`, `interaction_stats`, `frame_window`,
  `render_pipeline`, `critical_blocking_calls`, `recipe`, and
  `evidence_pack`.

The baseline is good enough to extend in place. No new trace side-channel or
parallel pipeline should be introduced.

## P0-A: Donghu Mixed Harmony-Base Auto Candidate

Problem:

Auto detection can identify Harmony or Android trace flavor, but Donghu is a
mixed framework scenario on a Harmony system base. If the trace contains both
Android-looking process names (`com.*`, `surfaceflinger`, `Choreographer`) and
Harmony/OpenHarmony signals (`OS_FFRT`, `render_service`, `RSUniRender`,
`OHOS`), the system should expose `platform_candidate=mixed_harmony_base` and
keep Harmony scheduler priority semantics.

Design:

- Preserve explicit user/tool `platform` as authoritative. If the user says
  Harmony/鸿蒙/东湖, do not auto-correct to Android.
- For auto mode, infer a platform candidate from content signals and framework
  surfaces after flavor detection.
- If Harmony trace signals and Android-framework surfaces coexist, set:
  - `platform=donghu`
  - `platform_candidate=mixed_harmony_base`
  - `priority_semantics=Harmony/OpenHarmony`
  - `framework_mode=process_isolated_mixed`
- Include confidence and signals for audit. Do not use the candidate as a hard
  root-cause fact.

Development tasks:

- [ ] Add result fields for platform candidate, confidence, and signals.
- [ ] Move framework-surface detection early enough to participate in platform
      candidate selection.
- [ ] Add mixed Harmony-base candidate inference without overriding explicit
      platform hints.
- [ ] Update result summaries and user-visible `trace_query` panel output.
- [ ] Add tests for auto Donghu mixed traces and explicit-platform conflict
      behavior.

## P0-B: Binder Lock/Reply/Async/Alloc-Buffer Closure

Problem:

`ipc_graph` currently sees `binder_transaction` and
`binder_transaction_received`, but SmartPerf models binder lock, unlock,
alloc-buffer, async, reply, receive, and reply slices. Without auxiliary binder
events, Codrax can report a binder transaction without enough context to
explain whether it is blocking, async, allocation-related, or only endpoint
evidence.

Design:

- Add typed binder auxiliary event families:
  - `binder_lock`
  - `binder_locked`
  - `binder_unlock`
  - `binder_alloc_buf`
  - `binder_reply`
- Keep existing transaction and received edges stable.
- Correlate auxiliary binder rows by transaction id when available, otherwise
  by thread/time vicinity as advisory evidence only.
- Treat one-way/async binder as non-blocking unless scheduler sleep evidence
  proves otherwise.
- Feed binder auxiliary summaries into `ipc_graph`, `wakeup_chain`,
  `critical_blocking_calls`, and `root_cause_rank` as line-backed runtime
  evidence.

Development tasks:

- [ ] Extend event types and parser classification for binder auxiliary rows.
- [ ] Parse common transaction id, destination, data size, and flags when
      visible.
- [ ] Add compact `binder_events` to `IPCGraphResult`.
- [ ] Enrich binder wait summaries with reply/async/alloc/lock caveats.
- [ ] Add evidence pack and display rendering for binder auxiliary summaries.
- [ ] Add tests for synchronous wait, one-way async, alloc-buffer, lock/unlock,
      and missing receive rows.

## P0-C: Independent FrameTimeline / Frame Flow Views

Problem:

Existing `frame_window`/`render_pipeline` is span-oriented. SmartPerf exposes
FrameTimeline as expected/actual frame tracks plus flows across UI, Render,
GPU, RenderService, and composer phases. Models need a deterministic view that
does not require ad-hoc grep over Choreographer/RenderFrame spans.

Design:

- Add `frame_timeline` and `frame_flow` views.
- Reuse parsed B/E/C trace marks and existing span stack logic.
- Recognize frame-like phases by stable labels, not platform-specific hard
  conclusions:
  - Expected/Actual timeline
  - Choreographer/doFrame
  - traversal / measure / layout / draw
  - RenderFrame / Sync / GPU / PresentFence
  - RenderService / RS / RSUniRender
  - Jank / frame missed / deadline / vsync markers
- Produce compact frame items and flow edges with line-backed phases, duration,
  process/thread, and caveats for missing begin/end pairs.
- Preserve `frame_window` and `render_pipeline` behavior for backward
  compatibility.

Development tasks:

- [ ] Add frame timeline and flow result structs.
- [ ] Implement frame phase classifier and flow builder.
- [ ] Add new view enum values to `trace_query` schema and teaching.
- [ ] Add compact renderer output for frame items/flows.
- [ ] Add tests for expected/actual, UI to RS flow, missing end event, and
      generic ftrace fallback.

## P1-A: SmartPerf eBPF BIO/FileSystem/PageFault Summaries

Problem:

SmartPerf exposes high-value eBPF tables for BIO latency, FileSystem
operations, and PageFaults with process/thread/op/path/latency/callstack
dimensions. Codrax currently mostly counts storage/filesystem/memory-like rows
and pairs block IO issue/complete, but it does not produce TopN summaries for
these plugin-style rows.

Design:

- Keep parser format-generic. Support ftrace-converted rows and plugin text
  rows when fields are visible as `key=value` or stable row text.
- Add event-level TopN summaries for:
  - BIO latency by op/path/device/thread
  - FileSystem operations by syscall/op/path/thread
  - PageFault by operation/address/thread/process
- Preserve callstack/backtrace text as bounded summary/caveat only. Do not
  inline large callstacks.
- Feed summaries into `window_stats`, `critical_blocking_calls`, and
  `root_cause_rank` as advisory evidence.

Development tasks:

- [ ] Add resource event summary structs and window-stats fields.
- [ ] Parse common `path`, `op`, `latency/duration`, `size`, `address`, and
      `callstack/backtrace` fields from generic row text.
- [ ] Aggregate TopN by kind/op/path/thread with bounded examples.
- [ ] Add evidence and display rendering.
- [ ] Add tests for BIO, filesystem, page fault, malformed rows, and bounded
      callstack handling.

## P1-B: Ability / XPower / HiSystemEvent Resource Adapters

Problem:

SmartPerf uses Ability Monitor, XPower, and HiSystemEvent as resource evidence
for CPU, memory, disk, network, thermal, brightness, battery, and application
statistics. Codrax currently has broad power/memory/storage counters but no
plugin adapter layer for these rows.

Design:

- Add weak-structured event types for:
  - `ability_monitor`
  - `xpower`
  - `hi_sysevent`
- Classify by event name and row fields. Unknown plugin rows remain searchable
  through generic events.
- Aggregate plugin rows into `window_stats` resource profiles:
  CPU load, memory, disk IO, network, thermal, battery, display/GPU/CPU power,
  domain/eventname counts.
- Keep plugin data advisory. It may support CPU/IO/memory/thermal conclusions
  but cannot alone prove a scheduler root cause.

Development tasks:

- [ ] Add plugin event types and subsystem kinds.
- [ ] Add plugin-resource summary structs and aggregators.
- [ ] Render compact TopN resource summaries.
- [ ] Add tests for Ability CPU/memory/disk/network, XPower CPU/GPU/display,
      and HiSystemEvent domain/eventname rows.

## P1-C: Core Topology and Compute-Supply Evaluation

Problem:

SmartPerf scheduling guidance separates big/middle/small cores, CPU frequency
residency, thread frequency distribution, and runnable delay. Codrax already
computes CPU busy/idle/frequency and scheduler latency, but does not expose
core class occupancy or explicit target-thread frequency distribution.

Design:

- Add optional `core_topology` tool parameter as a string, for example:
  `small=0-3,middle=4-7,big=8-11`.
- If omitted, infer core classes from observed per-CPU max frequency in the
  selected trace and emit a caveat that the classification is inferred.
- Add window stats for:
  - core-class busy/idle/frequency residency
  - target-thread running/runnable time by core class and frequency
  - same-CPU high-priority competitors around runnable spans
- Keep all classifications advisory unless an explicit topology is supplied.

Development tasks:

- [x] Add `core_topology` query field, schema description, and summary output.
- [x] Add parser for topology strings with JSON-compatible scalar handling.
- [x] Add inferred topology from observed CPU max frequencies.
- [x] Add core-class stats and target thread frequency / compute-supply signals.
- [x] Feed compute-supply low-frequency / CPU-pressure signals into
      `root_cause_rank` through existing compute-supply enrichment.
- [x] Add tests for explicit topology, inferred topology through observed
      frequency tiers, summary rendering, and JSON camelCase compatibility.

## Tool Teaching and JSON Compatibility Checklist

For every new tool-facing change:

- [x] Update `trace_query` schema and description.
- [x] Update model-facing tool teaching that lists trace views, event filters,
      platform candidates, plugin observations, and core topology hints.
- [x] Update compact result rendering.
- [x] Add JSON compatibility coverage when models may emit strings for arrays,
      booleans, numbers, or topology shortcuts.
- [x] Add tests ensuring malformed-but-repairable JSON reaches the backend
      normalized query instead of causing a tool failure.

## Batch Plan

Batch 0:

- Land this design and task ledger.

Batch 1:

- P0-A Donghu mixed Harmony-base platform candidate.

Batch 2:

- P0-B Binder auxiliary events and binder wait closure.

Batch 3:

- P0-C FrameTimeline / frame_flow views.

Batch 4:

- P1-A eBPF BIO/FileSystem/PageFault summaries.

Batch 5:

- P1-B Ability/XPower/HiSystemEvent adapters.

Batch 6:

- P1-C core topology and compute-supply evaluation.

Batch 7:

- Tool teaching / JSON compatibility audit, full focused tests, and final
  regression sweep.

## Delivery Notes

- Batch 1: `64787e43 tracequery: infer donghu mixed platform`
- Batch 2: `9e7ae835 tracequery: add binder auxiliary events`
- Batch 3: `f8744700 tracequery: add frame timeline views`
- Batch 4: `82753591 tracequery: summarize smartperf resource rows`
- Batch 5: `fae7827d tracequery: summarize smartperf plugin observations`
- Batch 6: `9d5f5377 tracequery: add core topology supply signals`
