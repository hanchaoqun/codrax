# Trace Query Wakeup Causality Delivery Plan

## Summary

Current `trace_query` already parses the scheduler/resource rows needed for
sleep, runnable, D-state/IO, block latency, and wakeup edges. The remaining gap
is product shape: the engine exposes facts in separate views, but it does not
consistently distinguish **wakeup-chain causal impact** from **same-window
background pressure**.

The fix is a generalized causal layer, not a case patch. For every bounded
`wakeup_chain` / `root_cause_rank` request with a target thread, the engine
will compute per-node impact summaries from typed scheduler intervals, compare
target/dependency priority with trace-flavor semantics, and rank on-chain
impact before off-chain background pressure. No user-intent keyword matching
and no model-output prose matching are allowed.

## Observed Gaps

- `wakeup_chain` currently follows sleep intervals well, but the selected
  "interesting interval" is state-priority biased. A short sleep can hide a
  much longer runnable delay in the same aligned dependency window.
- `scheduler_latency_stats` counts `sched_switch prev_state=R` runnable waits,
  but misses wakeup-to-run delay created by `sched_wakeup` followed by the next
  `sched_switch` into the target.
- `findWakeupFor` is strictly bounded by the caller's `time_end`; trace
  timestamps rounded in customer-selected windows can miss a wakeup by one or
  a few microseconds.
- `root_cause_rank` mixes on-chain dependency facts with global `io_pressure`,
  `cpu_pressure`, and unrelated D-state rows. This can promote off-path long
  waits above a shorter but causally linked dependency.
- Priority information is parsed and explained, but the result does not
  explicitly surface lower-priority dependencies blocking a higher-priority
  target as a priority inversion candidate.
- Handoff currently carries root-cause, state-churn, IO-pressure, and wakeup
  snippets, but not a typed chain-impact record that downstream answer stages
  can consume without re-inferring the causal relation.

## Design Principles

- Use typed trace events, intervals, priorities, and producer IDs only.
- Keep hard gates precise. Causal impact is advisory evidence and ranking input,
  not a reject condition.
- Compute causal relation from graph membership and time windows, not from
  user wording or model-authored text.
- Keep global pressure visible as background/supporting context. It must not
  outrank an on-chain dependency merely because its aggregate duration is large.
- Reuse existing primitives: `ThreadTimeline`, `WindowStats`,
  `SchedulerLatencyResult`, `RootCauseRank`, `ObservationLedger`, and the
  shared structured tool payload compatibility layer.

## Target Output Contract

Add a chain-impact section to `wakeup_chain` and `root_cause_rank` results:

- `wakeup_chain.causal_impacts[]`
  - `thread`, `window`, `chain_depth`, `on_chain`
  - `dominant_state`, `dominant_impact_ms`, `total_ms`
  - `running_ms`, `runnable_ms`, `sleep_ms`, `d_state_ms`, `io_wait_ms`
  - `fragment_count`, `state_switches`, `max_segment_ms`, `p95_segment_ms`
  - `target_blocked_ms`, `line_start`, `line_end`
  - `priority`, `priority_class`, `target_priority`, `target_priority_class`
  - `priority_relation`, `priority_inversion_candidate`
  - `summary`, `next_step`
- `root_cause_rank.items[]`
  - add optional `causality`, `chain_depth`, and `target_impact_ms`
  - on-chain dependency items use causality `on_wakeup_chain`
  - same-window but off-chain items use causality `background`

The fields are deterministic tool output. They are not new model tool-call
inputs. Any input aliases introduced for teaching convenience must be routed
through the existing structured payload compatibility layer before strict JSON
decode.

## Ranking Model

- Build causal impact for each chain node using the full aligned window, not
  only the single selected interval.
- Determine dominant impact by cumulative state duration. D-state and IO-wait
  can be combined for the blocking summary while still preserving separate
  raw totals.
- The target node explains the symptom. Dependency nodes explain causes.
- Lower-priority dependency + target wait + dependency runnable/D/IO impact
  becomes `priority_inversion_candidate=true` when the trace flavor has known
  priority ordering. Intermediate sleep nodes that have an upstream waker remain
  in `wakeup_chain.causal_impacts` as path evidence, but do not become primary
  root-cause rank items by themselves.
- When a wakeup chain exists, background pressure remains in the result but its
  rank impact is capped to a fraction of the selected window unless it is
  directly attached to a chain thread. This prevents unrelated long D-state or
  global IO pressure from becoming the primary cause.
- Chain-level IO support is attached when D/IO impact exists on a chain node
  and same-window storage or `sched_blocked_reason iowait` evidence is present.

## Prompt, Hint, and JSON Repair

- Tool description must teach that `causal_impacts` is an output section, not a
  standalone view.
- The `view` schema may accept `causal_impact` as an alias for `wakeup_chain`
  only if it is added to `x-codrax-enum-aliases` and the structured compat
  layer can repair it.
- Summaries and next-call hints should guide the model to consume
  `wakeup_chain.causal_impacts` before background pressure for frame/root-cause
  answers.
- Typed observations must project causal impact into the ObservationLedger so
  handoff preserves the chain, priority relation, line range, and impact score.
- No prompt red-line violations: no project-specific thread names, no eval case
  literals, no instruction to match answer prose, and no keyword-based intent
  branching.

## Implementation Tasks

### Batch A: Design and Guard Rails

- [x] A1: Record the generalized design, fields, ranking rules, prompt/hint
  obligations, JSON repair obligations, and tests in this document.
- [x] A2: Add tests that lock current gaps with synthetic traces:
  wakeup-to-run latency, short-sleep/long-runnable dependency, boundary
  tolerance, chain D/IO root, and off-chain background pressure demotion.

### Batch B: Causal Impact Data Model

- [x] B1: Add `WakeupCausalImpact` and optional causal fields on
  `RootCauseRankItem`.
- [x] B2: Add a helper that summarizes a thread's aligned timeline into
  cumulative state impact, fragments, p95/max segment, line range, priority,
  and next-step guidance.
- [x] B3: Attach `causal_impacts` to `ChainResult` for every node.
- [x] B4: Change chain node selection to prefer cumulative/duration impact over
  state-name priority while preserving recursive wakeup traversal.

### Batch C: Scheduler and Boundary Semantics

- [x] C1: Extend `scheduler_latency_stats` to count wakeup-to-run runnable
  delay from `sched_wakeup` / `sched_waking`.
- [x] C2: Add a small timestamp boundary tolerance to wakeup matching and
  disclose it through caveats when used.
- [x] C3: Add trace-flavor-aware priority comparison helpers and surface
  priority inversion candidates on chain impacts and wakeup edges.

### Batch D: Ranking and Handoff

- [x] D1: Add on-chain causal root-cause candidates from
  `wakeup_chain.causal_impacts`.
- [x] D2: Demote off-chain background pressure when an on-chain dependency
  exists, without hiding the supporting evidence.
- [ ] D3: Extend trace-query summaries and typed observations for
  causal-impact records.
- [ ] D4: Extend ObservationLedger parsing/projection tests to prove causal
  impact survives handoff and stale background facts do not steal priority.

### Batch E: Prompt, Repair, and Eval

- [ ] E1: Update `trace_query` tool description and schema teaching.
- [ ] E2: Add structured-compat tests for any new input alias, especially
  `view=causal_impact` if enabled.
- [ ] E3: Add/extend eval cases for causal chain runnable, chain D/IO, and
  off-path background pressure demotion.
- [ ] E4: Run focused Go tests, representative eval pairs two at a time, full
  `go test ./...`, and `make`.

## Test Plan

- `go test ./internal/tracequery`
- `go test ./internal/tool`
- `go test ./internal/types ./internal/context`
- `go test ./...`
- `make`
- Eval pairs, two cases at a time:
  - new wakeup causality runnable case + existing
    `trace_query_state_churn_window_stats`
  - new wakeup causality D/IO case + existing
    `trace_query_inode_io_pressure`
  - new off-path pressure demotion case + existing
    `data_json_strict_ids`

## Acceptance Criteria

- A short sleep can no longer hide a longer runnable delay on the same chain
  dependency.
- Wakeup-to-run scheduler latency is visible in `scheduler_latency_stats`.
- Microsecond-scale customer window rounding does not produce false
  `missing_wakeup` for an adjacent wakeup row.
- `root_cause_rank` marks on-chain dependency impact as primary when it is the
  causal explanation, while global pressure remains supporting/background.
- Priority inversion candidates are explicit and flavor-aware.
- Causal impact appears in summaries, typed observations, and downstream
  handoff without relying on prose parsing.
- Tool-call JSON remains easy for the model: any newly accepted input spelling
  is repaired before strict decode.
