# Perf Sample Parser Capability Audit And Delivery Plan (2026-06-18)

## Scope

This document re-audits the `/Users/han/opt/perf_query.md` goal against current
Codrax code and public Harmony/OpenHarmony + Android perf-sample parser sources.
It deliberately treats the gap as an architecture problem, not as a single eval
patch:

```text
runtime trace / htrace / perf.data / report-sample artifact
        -> parser provider registry
        -> normalized perftrace + capability manifest
        -> trace_query trace+perf bundle
        -> role-aware root-cause handoff + report transparency
```

No user-intent decision should be made from a file suffix, keyword, or model
prose. Hard behavior must come from explicit structured request fields, readable
artifact content/magic, official adapter availability, and typed parser
capabilities.

## Reference Requirement

`/Users/han/opt/perf_query.md` asks Codrax to answer questions that current pure
trace analysis cannot answer alone:

- Which functions actually executed inside a runnable/running window?
- Which hot symbols/callchains belong to a wakeup-chain dependency?
- Which binder peer/server code path consumed sampled CPU?
- Which CPU samples support a frame root-cause bundle?

The requested system shape is:

```text
hiprofiler_data.htrace
        -> trace_convert
        -> *.perftrace
        -> trace_query EventPerfSample
        -> window_stats / perf_stats / perf_timeline / trace_perf_bundle
        -> root_cause_rank + frame_root_cause_bundle perf contexts
```

## Public Source Findings

### OpenHarmony / Harmony

- `developtools_hiperf` describes `hiperf` as an OpenHarmony command-line
  performance tool similar to Linux perf. It records sampling data to
  `perf.data`, provides `report`, `dump`, JSON/ProtoBuf report flows, host
  binaries such as `hiperf_host`, and symbol collection helpers:
  https://gitee.com/openharmony/developtools_hiperf
- `proto/report_sample.proto` defines the official hiperf protobuf report
  stream. `CallStackSample` exposes `time`, `tid`, repeated call-stack frames,
  `event_count`, and `config_name_id`. `VirtualThreadInfo` provides `tid/pid/name`.
  `SymbolTableFile` provides DSO path and function names. There is no sample CPU
  field:
  https://gitee.com/openharmony/developtools_hiperf/raw/master/proto/report_sample.proto
- `developtools_profiler` `TraceFileHeader` defines standalone payload data
  types. `HIPERF_DATA` is data type `1`, with a fixed 1024-byte header and
  standalone plugin name/version fields:
  https://gitee.com/openharmony/developtools_profiler/raw/master/device/services/profiler_service/src/trace_file_header.h
- The profiler hiperf plugin writes a standalone plugin file from
  `/data/local/tmp/perf.data`, marks it as `HIPERF_DATA`, and registers
  `hiperf-plugin` as standalone file data:
  https://gitee.com/openharmony/developtools_profiler/raw/master/device/plugins/hiperf_plugin/src/hiperf_module.cpp
- `report_protobuf_file.cpp` writes `PerfRecordSample` into the protobuf stream:
  sample time, tid, call frames, period/event count, config name, thread info,
  symbol files, and lost-statistic records:
  https://gitee.com/openharmony/developtools_hiperf/raw/master/src/report_protobuf_file.cpp

Implication: OpenHarmony htrace captures can embed raw `HIPERF_DATA` perf.data
payloads, but the commercially safe symbolized lane is still the official
`hiperf report --proto` provider. CPU identity is structurally unknown in that
official proto, so Codrax must expose `cpu_unknown` as a first-class quality fact
instead of treating CPU 0 as a fallback.

### Android

- AOSP `report_sample.py` reports samples in a perf-script-like text format and
  prints `thread_comm`, `pid/tid`, CPU, timestamp, period, event name, leaf
  symbol/DSO, and call-chain entries:
  https://android.googlesource.com/platform/system/extras/+/refs/heads/main/simpleperf/scripts/report_sample.py
- `simpleperf_report_lib.py` exposes `SampleStruct` fields including `ip`, `pid`,
  `tid`, `thread_comm`, `time` in ns, `cpu`, and `period`; symbol and call-chain
  structs expose DSO, symbol name, and frame ordering. It also documents
  trace-offcpu modes and metadata/build-id access:
  https://android.googlesource.com/platform/system/extras/+/refs/heads/main/simpleperf/scripts/simpleperf_report_lib.py
- Android simpleperf docs state that `report_sample.py` converts profiling data
  to perf-script text, while `simpleperf_report_lib.py` is the Python API for
  reading samples, symbols, call chains, record options, architecture, and meta
  strings:
  https://android.googlesource.com/platform/system/extras/+/refs/heads/main/simpleperf/doc/scripts_reference.md
- The generated `report_sample_pb2.py` declares `cmd_report_sample.proto` and the
  `SIMPLEPERF` report-sample proto record shape: `Sample`, `File`, `Thread`,
  `MetaInfo`, `ContextSwitch`, and lost records:
  https://android.googlesource.com/platform/system/extras/+/refs/heads/main/simpleperf/scripts/report_sample_pb2.py

Implication: the current text adapter is correct as one provider, but Android
has a richer official provider surface than the text regex alone. A production
system should treat text, raw perf.data, and `SIMPLEPERF` proto as separate
formats behind one provider registry.

## Current Codrax Coverage

Already implemented on current `main`:

- `trace_convert` extracts OpenHarmony standalone `HIPERF_DATA` sidecars from
  htrace-like captures.
- Official OpenHarmony hiperf adapter runs `report --proto` and manually reads
  the official `HIPERF_PB_` protobuf stream into `.perftrace`.
- Official Android adapter runs configured/discovered `report_sample.py` and
  normalizes the perf-script text into `.perftrace`.
- Raw perf.data fallback parses a bounded Linux/simpleperf-like subset:
  `PERFILE2`, first attr, `COMM`, `MMAP/MMAP2`, and sample fields such as IP,
  TID, TIME, CPU, PERIOD, and CALLCHAIN.
- `.tracebundle.json` can preserve `.systrace`, `.perf.data`, and `.perftrace`
  together.
- `trace_query` supports `EventPerfSample`, `event_types=["perf_sample"]`,
  `window_stats.perf_samples`, `perf_stats`, `perf_timeline`,
  `trace_perf_bundle`, root-cause candidate `perf_context`, role-aware
  `perf_contexts`, and frame-bundle perf roles.
- Prompt/schema/hints already teach that perf samples are supporting
  code-execution evidence, while scheduler overlap, wakeup-chain relevance,
  binder peer state, D-state/IO, CPU/core/frequency/affinity, and supply
  pressure remain the causal basis.
- Recent UX work exposes attached/request-path runtime artifacts in CLI/REPL and
  markdown/html reports and preserves runtime artifact paths as external
  observations rather than current-source facts.

## Remaining Architecture Gaps

1. Parser providers are implicit functions, not a typed registry.
   Hiperf proto, simpleperf text, raw fallback, and future SIMPLEPERF proto each
   have different field guarantees. Today those guarantees are scattered across
   caveat strings and output rows.

2. Artifact capability is not consistently machine-readable.
   Tracebundle metadata should state which provider generated an artifact,
   which input format was detected, whether symbolization/callchain/CPU/clock
   data is known, and whether trace_query can consume it directly.

3. Direct perf input detection still had suffix/config paths.
   A direct perf conversion should be triggered by content magic such as
   `PERFILE2` or `SIMPLEPERF`, not by `perf.data` file names or the mere presence
   of a configured adapter.

4. Android SIMPLEPERF proto is not a first-class provider.
   The official Python library can consume it, but Codrax currently only invokes
   text `report_sample.py` and then parses that text. A future provider should
   either call the official library in proto mode or manually read the public
   proto when Python dependencies are unavailable.

5. Raw fallback remains intentionally narrow.
   It does not parse multi-attr/multi-event correlation, feature sections,
   build-id sections, endian variants, kernel/JIT/Java symbolization, fork/exit
   lifetime repair, or off-cpu semantics. This is acceptable only if it is
   loudly typed as degraded.

6. Time alignment is still assumed.
   Hiperf/simpleperf times are ns in their recording clock domain; ftrace rows
   use trace seconds. Codrax currently joins by numeric timestamps and emits
   `clock_confidence=assumed`. A commercial implementation needs an optional
   capture-level clock map/alignment manifest.

7. Off-cpu sample semantics are not modeled.
   Android simpleperf supports trace-offcpu modes where period may represent
   off-CPU time. Codrax currently treats samples as CPU execution context, so
   off-cpu samples must be separated before they can influence root-cause
   narration.

8. Provider install/discovery is helpful but not self-healing.
   `--perf-tools-status` reports hints, but docs and status output should guide
   users through official tool installation, symbol roots, symfs/kallsyms, and
   when raw fallback is acceptable.

9. Handoff should carry capability before prose.
   Final answers and markdown/html reports should consume typed provider quality
   rather than relying on model prose to remember raw/source/symbolization
   limitations.

## Target Architecture

```mermaid
flowchart TD
    I["Explicit artifact path or attachment"] --> S["Content sniff: htrace / PERFILE2 / SIMPLEPERF / perftrace"]
    S --> R["Perf provider registry"]
    R --> H["OpenHarmony hiperf proto provider"]
    R --> A["Android simpleperf text provider"]
    R --> P["Android SIMPLEPERF proto provider"]
    R --> W["Codrax raw perf.data fallback"]
    H --> N["Normalized perftrace + capability manifest"]
    A --> N
    P --> N
    W --> N
    N --> B["tracebundle.json"]
    B --> Q["trace_query trace+perf index"]
    Q --> O["role-aware root cause + report transparency"]
```

### Provider Capability Contract

Every perf-related artifact should carry:

- provider kind/name
- detected input format and output format
- time domain and alignment confidence
- thread identity quality
- CPU identity quality
- period/event weight semantics
- symbolization quality
- callchain quality
- DSO/build-id quality
- off-cpu semantics
- trace_query readiness
- degraded flag and caveats

This is typed data for backend consumption, not an LLM-authored input field.
Therefore it does not need a new `trace_query` JSON repair alias unless a future
view adds model-authored filters such as `perf_provider` or `sample_kind`.

### Causal Semantics

Perf samples answer "what code was sampled" inside an already-bounded runtime
candidate. They should support:

- target running/compute-supply explanation
- same-CPU competitor explanation during runnable delay
- on-chain dependency explanation inside wakeup chains
- binder peer/server explanation
- IO/D-state code-path annotation when samples overlap

They must not independently prove a scheduling root cause without interval,
chain, resource, binder, or supply evidence.

## Delivery Plan

### Batch A - Capability Contract And Content Detection

- [x] Add `PerfArtifactCapability` to converter artifacts and tracebundle JSON.
- [x] Detect direct perf inputs by content magic (`PERFILE2`, `SIMPLEPERF`,
  `perf_sample:`) instead of suffix/config-only triggers.
- [x] Mark raw `.perf.data` source artifacts as not trace_query-ready, and mark
  normalized `.perftrace` as trace_query-ready.
- [x] Expose provider/capability in `trace convert` artifact lines.
- [x] Add tests proving no-suffix raw perf.data and no-suffix SIMPLEPERF proto
  inputs are detected by content.

### Batch B - Provider Registry Refactor

- [x] Replace ad-hoc `maybeConvert*` calls with a small provider registry:
  `Detect -> Plan -> Convert -> Capability`.
- [x] Make provider selection explicit: official Harmony, official Android text,
  official Android proto, raw fallback, disabled.
- [x] Return a structured provider decision log in `Result` and tracebundle.
- [x] Keep selection driven by parser mode, content format, and tool availability;
  never by model prose or user intent keywords.

Batch B implementation notes:

- `PerfProviderDecision` is now a typed system output alongside
  `PerfArtifactCapability`, not a model-authored `trace_query` input field.
  Therefore it does not need prompt-side JSON repair aliases yet.
- `Result`, `.tracebundle.json`, and `trace convert` CLI output expose provider
  decisions with `selected/attempted/succeeded/fallback/trace_query_ready`,
  parser mode, input format, artifact path, reason, and caveat.
- Current registry entries cover `openharmony_hiperf_report_proto`,
  `android_simpleperf_report_sample`, future `android_simpleperf_report_proto`,
  `codrax_raw_perfdata`, and disabled perftrace generation. The Android proto
  entry is explicit but still implemented in Batch C.
- Selection remains based on structured parser mode, content sniffed input
  format, and tool availability. No user-intent keyword or model prose is used
  as a hard gate.

### Batch C - Android SIMPLEPERF Proto Provider

- [x] Add a provider for files starting with `SIMPLEPERF`.
- [ ] Prefer official `simpleperf_report_lib.py` when available.
- [x] Add a bounded manual proto reader only for the public
  `cmd_report_sample.proto` fields needed by Codrax: sample time/thread/event
  count, callchain file/symbol ids, thread pid/tid/name, file path/symbols, meta
  event types, context_switch for off-cpu state.
- [x] Emit `sample_kind=on_cpu|off_cpu|unknown` from
  `MetaInfo.trace_offcpu` plus nearest per-thread `ContextSwitch`.
- [x] Add round-trip tests through trace_query.

Batch C implementation notes:

- Direct `SIMPLEPERF` content now uses `android_simpleperf_report_proto` before
  the older text adapter lane. The provider reads the official
  `cmd_report_sample.proto` stream directly and emits normalized
  `perf_sample` rows.
- SIMPLEPERF proto samples intentionally emit `cpu=-1 cpu_known=false` because
  the official proto has no sample CPU field. This remains a typed quality fact,
  not CPU0.
- `trace_query` now parses `sample_kind`, includes it in perf examples and
  `perf_quality.sample_kinds`, and adds a caveat that `off_cpu` rows must not be
  narrated as running CPU execution.
- `sample_kind` is a trace_query output field and event-search pattern token.
  No model-authored filter field was added, so there is no new JSON repair alias
  to maintain in this batch.

### Batch D - Raw Fallback Hardening

- [ ] Parse multiple perf attrs and event ids instead of first attr only.
- [ ] Parse feature sections for build-id, cmdline, arch, and meta information
  when present.
- [ ] Add COMM/FORK/EXIT lifetime repair where fixtures prove pid/tid identity
  would otherwise be wrong.
- [ ] Improve mmap/build-id DSO labeling without inventing symbols.
- [x] Preserve unsupported sample bits as typed caveats instead of failing
  entire files when partial extraction is safe.

Batch D partial implementation notes:

- Raw fallback now parses `read_format` and safely skips `PERF_SAMPLE_READ`,
  `PERF_SAMPLE_RAW`, `PERF_SAMPLE_WEIGHT`, `DATA_SRC`, `TRANSACTION`,
  `PHYS_ADDR`, `CGROUP`, `DATA_PAGE_SIZE`, and `CODE_PAGE_SIZE` payload fields
  while preserving IP/TID/TIME/CPU/PERIOD/CALLCHAIN evidence.
- Skipped non-causal payload fields are emitted as `parser_caveats` on generated
  `perf_sample` rows so trace_query/event_search/handoff can keep degraded raw
  parser provenance visible.
- Unsafe variable-context fields such as regs, stack, aux, and branch stack
  remain fail-loud until attr-aware parsing is added; this avoids shifted
  records and invented symbols.

### Batch E - Time Alignment And Off-CPU Semantics

- [ ] Add optional tracebundle clock alignment metadata:
  `perf_time_ns -> trace_seconds` offset/slope/confidence.
- [ ] Teach trace_query to expose alignment status per perf context.
- [ ] Separate on-cpu and off-cpu sample contexts so off-cpu periods do not get
  narrated as running CPU execution.
- [ ] Add tests for assumed, calibrated, and unknown clock alignment.

### Batch F - UX, Prompt, Handoff, And JSON Repair

- [ ] Update `--perf-tools-status` with install/check commands for hiperf,
  report_sample.py, symfs, kallsyms, and symbol roots.
- [ ] Add docs showing official-first and raw fallback workflows.
- [ ] Add report sections that summarize provider capabilities before hotspot
  prose.
- [ ] If any provider-related filter becomes a model-authored `trace_query`
  field, add it to schema, enum aliases, and unified structured JSON repair.
- [ ] Add low-prebake evals covering path-only, attachment, bundle, official
  symbolized, raw degraded, CPU-unknown, and no-suffix content-detected inputs.

## Acceptance Criteria

- Users can attach or mention Harmony/OpenHarmony/Android trace+perf artifacts
  without manually understanding sidecars.
- `trace_convert` reports what it generated, which provider was used, what the
  provider can and cannot guarantee, and which artifact should be passed to
  `trace_query`.
- `trace_query` consumes perf samples as role-aware execution context without
  changing scheduler/resource causality rules.
- Final reports preserve source/symbolization/CPU/clock/callchain caveats.
- Every model-authored input field is present in prompt teaching, schema, and
  JSON repair; output-only capability fields remain backend-owned.
