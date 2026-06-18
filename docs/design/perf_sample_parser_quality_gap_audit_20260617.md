# Perf Sample Parser Quality Gap Audit (2026-06-17)

## Scope

This audit covers the `hiprofiler_data.htrace / perf.data -> trace_convert -> .perftrace -> trace_query`
lane described in `/Users/han/opt/perf_query.md`, plus the already-landed
running/runnable root-cause perf context work. It is intentionally system-level:
no prompt keyword patches, no one-case rules, and no separate perf analysis
framework.

The desired production shape is:

```text
Harmony/OpenHarmony htrace + HIPERF_DATA
Android/simpleperf perf.data
raw perf.data fallback
        |
        v
trace_convert provider registry
        |
        v
normalized .perftrace rows with provenance + quality
        |
        v
trace_query EventPerfSample
        |
        v
window_stats / root_cause_rank / frame_root_cause_bundle / trace_perf_bundle
        |
        v
prompt, hint, markdown/html report, typed observations, and handoff-safe final answer
```

## Public Source Findings

- OpenHarmony `developtools_hiperf` documents `hiperf` as a perf-like profiler
  and publishes host tools such as `hiperf_host` and `libhiperf_report.so`.
  Source: https://gitee.com/openharmony/developtools_hiperf
- OpenHarmony `report_sample.proto` defines the official report stream:
  `CallStackSample{time, tid, callStackFrame, event_count, config_name_id}`,
  `VirtualThreadInfo{tid,pid,name}`, `SymbolTableFile{path,function_name}`,
  and `ReportInfo{config_name}`. It does not define a sample CPU id.
  Source: https://gitee.com/openharmony/developtools_hiperf/blob/master/proto/report_sample.proto
- Android `simpleperf/scripts/report_sample.py` is an official adapter that
  prints samples in a `perf script`-like format. Each sample exposes
  `thread_comm`, `pid`, `tid`, `cpu`, `time`, `period`, event name, leaf
  symbol/DSO, and call-chain entries.
  Source: https://android.googlesource.com/platform/system/extras/+/refs/heads/main/simpleperf/scripts/report_sample.py
- The correct strategy is a dual-engine provider model: official adapter first
  for full unwind/symbolization, then Codrax raw `perf.data` fallback only as an
  explicitly marked degraded lane.

## Current Codrax Coverage

Already implemented on `origin/main` before this audit:

- `trace convert` supports:
  - OpenHarmony `hiperf report --proto` via `--hiperf-host` /
    `CODRAX_HIPERF_HOST`.
  - Android `report_sample.py` via `--simpleperf-report-sample` /
    `CODRAX_SIMPLEPERF_REPORT_SAMPLE`.
  - `--perf-parser=auto|official|raw`, with raw fallback producing
    `source=raw_perfdata_fallback` and `symbolization_status=unsymbolized`.
  - `.tracebundle.json` metadata and sibling `.systrace + .perftrace` merge.
- `trace_query` supports:
  - `EventPerfSample`.
  - `event_types=["perf_sample"]` and aliases such as `cpuSample`.
  - `window_stats.perf_samples`, `perf_stats`, `perf_timeline`, and
    `trace_perf_bundle`.
  - `root_cause_rank.items[].perf_context` and role-aware
    `perf_contexts` for candidate threads, target running, on-chain
    dependencies, same-CPU competitors, CPU pressure, and compute-supply CPUs.
- Prompt/schema/hint surfaces teach that perf samples are supporting
  code-execution context and must not be treated as standalone scheduling proof.
- CLI/REPL/report transparency exists for attached runtime artifacts and
  converter caveats.

## Remaining System Gaps

1. **Perf quality is not first-class.** `Event` preserves
   `source`, `symbolization_status`, and `clock`, but `PerfContext` does not
   aggregate source mix, symbolization mix, clock mix, CPU-known status,
   call-chain completeness, or caveats. Downstream consumers must infer quality
   from hotspot examples.

2. **OpenHarmony CPU absence is structurally ambiguous.** The hiperf proto
   adapter emits `cpu=-1` because the official proto has no sample CPU field.
   The docs warn that this means "CPU unknown", but `PerfContext` does not
   expose `cpu_known=false` or a count of unknown CPU samples. CPU-filtered
   analysis can silently lose official OpenHarmony samples.

3. **Clock-domain confidence is not explicit.** Hiperf proto rows use
   `clock=monotonic_raw`, simpleperf rows use `clock=record`, raw fallback uses
   `clock=perf_data`. `trace_query` currently joins by numeric timestamp
   without a typed `clock_confidence` / `time_alignment` caveat.

4. **Call-chain quality is implicit.** Official lanes can carry symbols and
   callchains; raw fallback carries IP-only labels. There is no
   `callchain_status` or `callchain_unknown_count`, so reports may overstate the
   precision of IP-only frames.

5. **Trace+perf correlation is role-local but not summarized globally.**
   `root_cause_rank` has role-aware `perf_contexts`, and frame bundles have
   `target_running_perf/on_chain_perf/binder_peer_perf/same_cpu_competitor_perf`.
   There is still no reusable quality summary explaining whether each role's
   perf evidence is symbolized, CPU-known, and clock-aligned.

6. **Raw fallback degradation is observable but not query-ranked.** Raw parser
   caveats live on converter artifacts and sample rows, but the aggregate
   `PerfContext` does not tell the model "this whole context is unsymbolized".

7. **Provider preflight is not wired into the query result.** `trace convert`
   has `BuildPerfToolStatus`, but later `trace_query` results do not carry which
   provider generated the `.perftrace` unless the sample row is inspected.

8. **Prompt/hint coverage needs the new quality fields.** Existing teaching
   covers `source` and `symbolization_status`. Once quality fields are added,
   the schema description, view matrix, empty-result hints, markdown rendering,
   typed observations, and eval cases must teach the same semantics exactly once.

9. **JSON repair only applies to input fields.** The new quality fields should
   be output-only. If implementation introduces any model-authored input field
   such as `perf_quality`, `clock_domain`, or `symbolization_status`, it must be
   added to the `trace_query` schema and alias/compat repair layer. Avoid new
   inputs unless a concrete view/filter needs them.

10. **Runtime artifact auto-detection must remain semantic.** Users can attach
    artifacts with flags or mention explicit paths in the question. The analyzer
    should classify "trace only", "trace plus code", or "code against trace" from
    request semantics and attached/mentioned artifact metadata, not filename
    suffixes or intent keywords. File names may seed candidate artifacts, but
    cannot be the hard decision for user intent.

## Design

### Normalized Perf Quality

Add a small quality model to `internal/tracequery`:

```go
type PerfQualitySummary struct {
    Sources                []PerfValueCount
    SymbolizationStatuses []PerfValueCount
    Clocks                 []PerfValueCount
    CPUKnownCount          int
    CPUUnknownCount        int
    CallchainKnownCount    int
    CallchainUnknownCount  int
    Caveats                []string
}

type PerfValueCount struct {
    Value       string
    SampleCount int
    Period      int64
    Percent     float64
}
```

Attach `PerfQualitySummary` to `PerfContext`. It is computed from matched
samples and therefore applies uniformly to `window_stats.perf_samples`,
`perf_stats`, root-cause role contexts, and frame-bundle role contexts.

### Parser Fields

Extend `Event` and `.perftrace` rows with output-friendly quality fields:

- `cpu_known=true|false`
- `clock=<monotonic_raw|record|perf_data|...>`
- `clock_confidence=<aligned|assumed|unknown>`
- `symbolization_status=<symbolized|partial|unsymbolized|unknown>`
- `callchain_status=<symbolized|ip_only|missing|unknown>`

Backfill defaults:

- `cpu_known=false` if `cpu < 0`, otherwise true unless explicitly false.
- `symbolization_status=symbolized` for official lanes with function symbols,
  `unsymbolized` for raw fallback, `unknown` otherwise.
- `callchain_status=missing` if callchain is empty, `ip_only` for raw IP
  callchains, `symbolized` when a non-IP callchain exists, `unknown` otherwise.
- `clock_confidence=assumed` for known converter clocks until a future alignment
  table exists; `unknown` when no clock field exists.
- `period` remains an event/sample weight used for ranking within the same perf
  context. It is not elapsed duration and must not be converted to wall time or
  expected sample density unless explicit sampling configuration and calibrated
  CPU frequency are available.

These defaults are deterministic parser normalization, not model prose parsing.

### Output Guidance

Every perf-bearing output should expose the same guidance:

- `perf_quality sources=... symbolization=... clocks=... cpu_known=...`
- caveats when CPU is unknown, samples are unsymbolized, callchains are missing,
  or clock alignment is assumed/unknown.
- role rows preserve quality next to `top_symbol`, so a final answer can say
  "running was supported by official symbolized samples" or "only raw IP/DSO
  fallback evidence exists".

Perf samples remain supporting execution context. Scheduler overlap, wakeup
chain relevance, binder sync/oneway semantics, CPU/core/frequency/affinity,
D-state/IO, and resource pressure remain the causal basis.

## Task List

### Batch A - Audit Document And Contracts

- [x] Document public-source findings, current coverage, remaining gaps, and
  commercial task list.
- [ ] Push this design document to `main` from an isolated worktree.

### Batch B - Perf Quality Data Model

- [x] Add `PerfQualitySummary` and `PerfValueCount`.
- [x] Add event fields for `PerfCPUKnown`, `PerfClockConfidence`, and
  `PerfCallchainStatus`.
- [x] Parse aliases from `.perftrace` rows and apply deterministic defaults.
- [x] Emit quality fields from hiperf, simpleperf, and raw fallback adapters.
- [x] Unit test official symbolized, OpenHarmony CPU-unknown, and raw
  unsymbolized cases.

### Batch C - Query Aggregation And Role Handoff

- [x] Compute `PerfContext.Quality` for every perf context.
- [x] Render quality in `window_stats`, `perf_stats`, `trace_perf_bundle`,
  `root_cause_rank.perf_contexts`, and frame-bundle perf roles.
- [x] Add caveats for CPU-unknown, unsymbolized/raw fallback, missing callchain,
  and assumed/unknown clock alignment.
- [x] Ensure CPU-filtered role contexts do not treat CPU-unknown samples as
  concrete CPU/core attribution or as absence proof.

### Batch D - Prompt, Hints, Reports, And JSON Repair

- [x] Update `trace_query` schema description and view matrix to teach the new
  quality fields and how to consume them.
- [x] Update event-search and empty-result hints so the model searches
  `symbol/dso/callchain/source/symbolization_status/callchain_status` without
  inventing quality.
- [x] Extend markdown/typed-observation report rendering with `perf_quality`.
- [x] Confirm no new model-authored JSON field was added. If a new input field
  is introduced, add it to unified schema alias/compat repair.

### Batch E - Verification And Evals

- [x] Add low-prebake evals for:
  - OpenHarmony converted perftrace with CPU unknown.
  - Android simpleperf-style perftrace with symbolized CPU samples.
  - Raw fallback perftrace with IP/DSO-only evidence.
  - Trace+perf running/runnable root-cause role contexts.
- [x] Run focused Go tests:
  `go test ./internal/hitraceconv ./internal/tracequery ./internal/tool`.
- [x] Run full Go tests: `go test ./...`.
- [x] Run at least two relevant eval cases in parallel batches of two.
- [x] Re-run existing trace/runnable/perf evals to check no regression.

## Verification 2026-06-17

- Focused packages passed:
  `go test ./internal/hitraceconv ./internal/tracequery ./internal/tool`.
- Full repository tests passed: `go test ./...`.
- Eval batch `perf_quality_new_cases_20260617_summary.md` passed:
  `trace_query_perf_quality_harmony_cpu_unknown` and
  `trace_query_perf_quality_raw_fallback`.
- Eval batch `perf_quality_regression_cases_20260617_summary.md` passed
  `trace_query_running_perf_context`. The first isolated-worktree run of
  `trace_query_path_question_relative_donghu_short` failed because
  `../customlogs/xxx_all.systrace` resolved to `/private/tmp/customlogs`, which
  did not exist in the detached worktree environment.
- After adding a temporary eval-environment symlink
  `/private/tmp/customlogs -> /Users/han/opt/customlogs`, eval batch
  `perf_quality_path_and_simpleperf_20260617_summary.md` passed:
  `trace_query_perf_quality_simpleperf_symbolized` and
  `trace_query_path_question_relative_donghu_short`.
- Manual audit of the Donghu relative-path answer confirmed that the system
  consumed the explicit trace path, did not read source files, and reported the
  key relationship:
  `CookieMonsterCl-59843 -> com.baidu.tieba-59566`, 2.978ms target sleep,
  1.661ms CookieMonsterCl runnable wait, priority 20 vs 52, and cpu0/1/2
  runnable pressure background.
- After changing `perf_cpu_known` to an output pointer bool, focused and full
  Go tests were re-run and passed. The simpleperf eval was re-run and initially
  exposed a handoff/reporting gap where final prose could omit
  `source=simpleperf_report_sample` even though trace_query typed observations
  carried it.
- The final markdown/report supplement now projects trace_query
  `perf_quality=...` directly from typed observation notes whenever perf
  quality evidence exists. Re-run summary
  `perf_quality_simpleperf_after_always_supplement_20260617_summary.md` passed,
  and the final answer included:
  `perf_quality=cpu_known=1,cpu_unknown=0,source=simpleperf_report_sample,symbolization=symbolized,clock=record,clock_confidence=assumed,callchain_status=symbolized`.
- Manual audit also found an over-quantification risk: models may treat perf
  `period` as elapsed time or expected sample density. `PerfQualitySummary`
  caveats and trace_query prompt guidance now state that period is an
  event/sample weight unless explicit sampling configuration and calibrated CPU
  frequency are present.
- Residual observation: runtime-artifact-only requests that explicitly forbid
  source analysis can still route through the repo pipeline and build the repo
  index before using `trace_query`. This is not a correctness regression for
  this batch because no source files were read and the final answer remained
  artifact-local, but it remains a UX/transparency optimization for a future
  runtime-artifact-only lane.

## Completion Criteria

- Official and raw perf lanes produce normalized quality metadata.
- `trace_query` carries perf quality through every perf context and role handoff.
- The model can understand whether perf evidence is symbolized, CPU-known, and
  clock-aligned without reading raw rows.
- No logic hard-gates user intent on keywords or suffixes.
- No prompt red-line violation: deterministic logic consumes typed fields only.
- Reports and prompts are consistent, and JSON repair covers any new
  model-authored fields.
