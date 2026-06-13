# Trace Query Runnable Context Delivery Plan

## Summary

When `runnable` is the dominant delay, `trace_query` must not stop at
"thread waited runnable". A commercial answer needs the scheduling context that
can explain why the wait mattered:

- which CPU and core class the runnable wait was targeting;
- whether other CPUs or larger cores were idle at the same time;
- which same-CPU threads consumed running time;
- whether the trace contains explicit CPU affinity, allowed CPU, cpuset, or
  migration evidence;
- which background threads created the largest load; process/TGID rollups are
  secondary context only;
- how these facts should influence root-cause ranking without hiding the raw
  pressure signals.

The implementation is a generalized trace data product. It parses scheduler
constraint events and aggregates typed evidence. It does not match user prose,
model prose, customer thread names, or eval-specific keywords.

## Current Code Flow

- `internal/tracequery/parse.go`
  - Parses ftrace/hitrace rows into `Event`.
  - Already supports `key=value`, `key = value`, whitelist `key value`, and
    whitelist `key:value` field forms.
- `internal/tracequery/query.go`
  - `ComputeWindowStats` builds CPU busy/idle, top running, runnable/D-state
    durations, CPU pressure, core topology, compute supply, state churn, IO,
    and root-cause inputs.
  - `buildSchedulerLatencyStatsFromStats` reports runnable intervals with CPU,
    frequency, priority, same-CPU busy/idle, other CPU idle, high-priority
    running time, and same-CPU top running threads.
  - `BuildRootCauseRank` lets runnable, scheduler latency, CPU pressure,
    compute supply, wakeup-chain causal impacts, IO, and state churn compete.
- `internal/tool/trace_query.go`
  - Tool description and schema teach `window_stats`, `scheduler_latency_stats`,
    `root_cause_rank`, `core_topology`, and event filters.
  - The tool-call JSON path already enters `applyStructuredPayloadCompat`.
  - Typed observations are published directly from `tracequery.Result`; preview
    summary re-parse is only fallback.

## Gaps

- `SchedulerLatencyItem` has no `core_class`, so a runnable root cause can say
  `cpu=0` without preserving whether that was a small/middle/big core.
- There is no explicit `cpu_affinity` / `cpuset` / allowed-CPU output. Current
  code can infer "other_cpu_idle" but cannot distinguish "bad wakeup placement"
  from "thread was actually restricted to small CPUs".
- `CPUPressureStats` is CPU-scoped. It lists top runnable/running threads, but
  does not produce a target-oriented background-thread load list or secondary
  process/TGID rollup.
- Root-cause summaries do not consistently carry the runnable context bundle:
  core class, same-CPU competitors, top background threads, secondary
  process-level background load, and explicit CPU constraint evidence.
- Handoff does not publish runnable-context rows as typed observations, so
  downstream answer stages can drop the CPU/core/affinity context while keeping
  only "runnable wait".

## Output Contract

Add deterministic output fields:

- `scheduler_latency_stats.items[].core_class`
- `window_stats.cpu_constraints[]`
  - `thread`, `allowed_cpus`, `allowed_core_classes`, `cpuset`, `policy`,
    `observed_cpu`, `observed_core_class`, `migration_count`,
    `runnable_wait_ms`, `other_cpu_idle_ms`, `line_start`, `line_end`,
    `summary`
- `window_stats.thread_cpu_load[]`
  - `thread`, `running_ms`, `runnable_wait_ms`,
    `high_priority_running_ms`, `cpu`, `core_class`, `frequency`, `priority`,
    `priority_class`, `line_start`, `line_end`, `summary`
- `window_stats.process_cpu_load[]` (secondary rollup)
  - `process`, `thread_count`, `running_ms`, `runnable_wait_ms`,
    `high_priority_running_ms`, `top_thread`, `top_thread_ms`,
    `cpus`, `core_classes`, `line_start`, `line_end`, `summary`
- `window_stats.runnable_context[]`
  - `thread`, `runnable_wait_ms`, `cpu`, `core_class`, `frequency`,
    `same_cpu_busy_ms`, `same_cpu_idle_ms`, `other_cpu_idle_ms`,
    `high_priority_running_ms`, `same_cpu_top_running`,
    `top_background_threads`, `same_process_load`, `top_background_process`,
    `constraint`, `verdict`, `confidence`, `line_start`, `line_end`, `summary`

The fields are tool output. They are not new required tool-call JSON fields.
Any new `event_types` aliases must enter the existing event-type normalizer and
shared structured payload compatibility path.

## Parser Design

Add an explicit scheduler CPU-constraint family instead of a generic unknown
body parser:

- event type: `cpu_constraint`
- accepted event families:
  - `sched_setaffinity`
  - `sched_migrate_task`
  - `cpuset_attach`
  - `cgroup_attach_task`
- accepted field aliases:
  - pid/thread: `pid`, `tid`, `target_pid`, `task_pid`
  - comm/thread name: `comm`, `task`, `target_comm`, `name`
  - target/migration CPU: `cpu`, `target_cpu`, `dest_cpu`, `orig_cpu`
  - allowed CPUs: `mask`, `cpus`, `allowed_cpus`, `cpumask`,
    `cpus_allowed`, `affinity`
  - cpuset/cgroup: `cpuset`, `cgroup`, `cg`, `path`
  - policy: `policy`, `reason`, `type`

The parser stores these fields on `Event` so `event_search` can show them and
`window_stats` can aggregate them. Field parsing remains whitelist-based.

## Aggregation Design

- Reuse existing CPU/core helpers:
  - `resolveCoreTopology`
  - `applyCPUCoreClasses`
  - `parseCPURangeList`
- Build `thread_cpu_load` from `TopRunning`, `RunnableTop`, and
  `CPUPressure.TopRunning`.
- Build secondary `process_cpu_load` from `TopRunning`, `RunnableTop`, and
  `CPUPressure.TopRunning`.
  - Prefer TGID when present.
  - Fall back to an exact comm-derived group label only when TGID is absent.
  - Do not infer Android package/process identity from thread-name substrings.
- Build `cpu_constraints` from explicit constraint/migration events and join
  them with runnable/top-running durations by PID.
- Build `runnable_context` from `SchedulerLatencyItem` plus `WindowStats`:
  - carry same-CPU top running list;
  - carry top background threads before process rollups;
  - attach core class and frequency;
  - attach top process load excluding the target process only as secondary
    context when available;
  - attach the matching CPU constraint summary for the same PID;
  - classify the verdict from typed facts:
    - `cpu_pressure`
    - `restricted_to_busy_or_small_cores`
    - `other_cpu_idle_check_affinity_or_wakeup`
    - `low_frequency`
    - `insufficient_signal`

## Ranking Design

- Runnable and scheduler-latency root-cause candidates keep their existing
  impact math.
- Summaries are enriched with runnable context, but ranking does not hard-gate
  on noisy context.
- Explicit restriction evidence (`allowed_cpus`/`cpuset` plus idle larger
  cores) can add a supporting `cpu_affinity_or_cpuset` candidate.
- Inferred evidence only stays advisory in the summary/verdict; it must not
  become a definitive root cause without a typed constraint event.

## Prompt, Hint, Handoff, and JSON Repair

- Tool description and schema must teach:
  - `runnable_context`, `cpu_constraints`, `thread_cpu_load`, and secondary
    `process_cpu_load` are output
    sections in `window_stats` and supporting signals for `root_cause_rank`.
  - For runnable root causes, consume the CPU/core/affinity/background-load
    fields before concluding.
- Add `event_types` aliases:
  - `cpu_constraint`, `affinity`, `cpu_affinity`, `cpuset`,
    `sched_migrate`, `migration`
- The alias path remains:
  - model tool-call JSON -> `applyStructuredPayloadCompat` ->
    `TraceEventTypes.UnmarshalJSON` -> `normalizeTraceQueryEventTypeToken`.
- Typed observations must publish:
  - `runnable_context`
  - `cpu_constraint`
  - `thread_cpu_load`
  - `process_cpu_load`
- ObservationLedger fallback must parse the same summary rows for old tool
  results without typed rows.

## Implementation Tasks

- [x] A1: Add this design document.
- [x] B1: Add event type/fields for CPU constraint and migration evidence.
- [x] B2: Parse explicit scheduler/cpuset/cgroup CPU constraint events with
  whitelist key aliases.
- [x] B3: Parse Harmony/Donghu `sched_switch next_info` affinity/load/group
  fields into typed CPU constraint evidence.
- [x] C1: Add `CoreClass` to `SchedulerLatencyItem`.
- [x] C2: Add `CPUConstraintSummary`, `ThreadCPULoadSummary`,
  `ProcessCPULoadSummary`, and `RunnableContextSummary` to `WindowStats`.
- [x] C3: Compute thread-level running/runnable load, secondary process-level
  rollups, and CPU constraint summaries.
- [x] C4: Compute runnable-context summaries from scheduler latency and
  window stats.
- [x] D1: Enrich root-cause summaries with runnable context.
- [x] D2: Add supporting `cpu_affinity_or_cpuset` candidates only when explicit
  constraint evidence exists.
- [x] E1: Update trace_query prompt/schema/event aliases and summary rendering.
- [x] E2: Publish typed observations and fallback ledger parsing.
- [x] F1: Add parser, aggregation, root-rank, tool, and ledger tests.
- [x] F2: Add a low-leading runnable context eval case that does not pre-bake
  the answer.
- [ ] F3: Run focused tests, representative eval pairs two at a time, full
  `go test ./...`, and `make`.

## Validation Plan

- `go test ./internal/tracequery -run 'Runnable|CPUConstraint|ProcessCPU|SchedulerLatency'`
- `go test ./internal/tool -run TraceQuery`
- `go test ./internal/types -run TraceQuery`
- `go test ./internal/context -run Trace`
- `go test ./...`
- `make`
- Eval pairs, two cases at a time:
  - new runnable context / affinity case + `trace_query_wakeup_causal_runnable`
  - new process background load case + `trace_query_core_topology_supply`
  - `trace_query_state_churn_window_stats` + `trace_query_inode_io_pressure`

## Validation Results

Completed before final full-suite validation:

- `go test ./internal/tracequery -run 'CPUConstraint|RunnableContext' -count=1`
- `go test ./internal/tool -run 'RunnableContext|TypedObservations' -count=1`
- `go test ./internal/types -run 'TraceQueryRootCauseRank' -count=1`
- Eval pair:
  - `trace_query_runnable_context_thread_load`: PASS
  - `trace_query_core_topology_supply`: PASS
- Eval pair:
  - `trace_query_state_churn_window_stats`: PASS
  - `trace_query_wakeup_causal_runnable`: PASS
