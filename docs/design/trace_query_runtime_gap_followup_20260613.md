# Trace Query Runtime Gap Follow-up - 2026-06-13

## Scope

This document records the residual gaps found after running the real
`xxx_all.systrace` customer trace through the current system. The validation
question must be low-leading: ask for "卡顿原因" / "jank causes" and require
multi-level analysis, but do not bake in a negative hint that names a specific
off-chain thread/state to rule out.

Target trace:

- `/Users/han/opt/customlogs/xxx_all.systrace`
- Target thread: `com.baidu.tieba-59566`
- Representative frame window: `34579.472865s..34579.587805s`

## Observed System Behavior

The system can recover the important wakeup chain from the trace:

`ThreadPoolForeg-60555 -> NetworkService-60595 -> CookieMonsterCl-59843 -> com.baidu.tieba-59566`

It also recognizes the priority relationship: the target main thread is
Harmony RT priority `52`, while the dependency chain contains CFS priority `20`
threads. The chain rows carry useful per-node state summaries, including
runnable pressure on `NetworkService`/`CookieMonsterCl` and D-state/IO-like
waiting on `ThreadPoolForeg`.

However, the final answer can still be polluted by off-chain background rows.
In the real run, `root_cause_rank` promoted `hilogd.rd_kmsg-503` as
`root_cause_primary` via `compute_supply`, while the wakeup-chain rows already
showed more relevant on-chain causes. The final prose tried to exclude log
threads, but the citation list still surfaced the background row as a primary
cause, creating a user-visible contradiction.

## Residual Gaps

### Gap 1: Root-cause ranking is not chain-aware after enrichment

`buildRootCauseRankFrom` enriches initial candidates with chain context, but
`enrichRootCauseRankWithScheduler` appends scheduler/compute/constraint
candidates later and then sorts by score/impact only. In a trace with a valid
wakeup chain, a large off-chain compute-supply or CPU-pressure row can outrank
an on-chain dependency.

Commercial requirement:

- When a target wakeup chain exists, `on_chain` candidates rank before
  `adjacent`, and both rank before `background`.
- Background system pressure remains visible as supporting context, not as the
  primary cause.
- All root-cause candidates must carry explicit `chain_relevance` when a chain
  exists, including post-enrichment candidates.

### Gap 2: Answer-side runtime observations can preserve polluted primaries

Typed observations currently publish every `root_cause_rank` row with
`Role=PrincipalAnswer` and predicates such as `root_cause_primary`, regardless
of `chain_relevance`. If a background row is ranked first, the final answer
prompt and citation pool can preserve it as a principal item even when other
trace rows prove the wakeup-chain path.

Commercial requirement:

- In runtime trace answers with chain-aware rows, background root-cause rows
  should be projected as supporting coverage unless there is no on-chain or
  adjacent candidate.
- Rich notes must retain `chain_relevance`, overlap, edge count, and nearest
  chain context so the answer layer can distinguish primary path from
  supporting pressure.

### Gap 3: Trace-only runs still carry source-oriented workflow artifacts

The end-to-end run still performed repository index work and displayed
source-oriented stage text, even though the user explicitly requested trace
analysis only. Exploration eventually used `trace_query`, but perf-triage and
stage presentation still created a source-analysis feel and extra cost.

Commercial requirement:

- For explicit runtime trace-only requests, classification and exploration
  should treat the trace as the authoritative lane.
- Current-source tools remain fallback only when explicitly requested or when
  trace_query reports unsupported/incomplete format.
- User-facing stage wording should not imply source-code investigation for an
  observation-only runtime trace answer.

### Gap 4: Extract stage can apply source-style hypothesis verdicts to trace-only answers

In the real run, the extract stage tried to fit a trace-only performance answer
into generic source-code hypotheses. This produced awkward h1/h2 verdict
language and increases the risk that final answers cite runtime artifacts as if
they were current-source proof.

Commercial requirement:

- Observation-only runtime trace answers should not force source-code
  hypothesis verdicts.
- When hypothesis verdicts are used, their citation lane must be artifact-local
  and their semantic wording must match runtime observation, not current-source
  module ownership.

### Gap 5: Source-optional runtime artifacts still trigger source-status semantics

The 2026-06-13 low-leading run correctly recovered both major trace causes:
runnable scheduling delay / priority inversion on the wakeup chain, and
upstream D-state IO wait on `ThreadPoolForeg-60555`. However, the answer also
emitted a `still_present` decision and prose such as "current code has not
removed the risk path", even though the request asked to analyze the trace and
no current checkout evidence was collected.

Deep cause:

- The analyzer did not always emit `external_observation_policy.current_source_mode=exclude`.
- In trace_query-first runs, an attached trace can be present even when
  perf-triage does not materialize a structured `PerfTrace` on
  `RequestModel`; downstream source-lane checks must still see the typed
  attached-trace context.
- Existing extractor/orchestrator auto-verdict suppression only used the narrow
  `HasObservationOnlyRuntimeArtifact()` predicate.
- For runtime artifacts with no typed current-source verification anchor, the
  broader commercial boundary is `HasRuntimeArtifactWithoutRequiredCurrentSource()`:
  current source may be allowed as fallback, but it is not required and must not
  create source-status verdicts.

Commercial requirement:

- Runtime artifact answers with no required current-source lane must not force
  source-code hypothesis verdicts, auto-verdicts, or `still_present/fixed`
  status semantics.
- Final answers should state trace-observed cause/risk, not current-code
  persistence, unless a typed current-source verification anchor exists.
- Runtime artifact citation/status prompts and repair hints must use the same
  source-lane predicate, so the model receives one coherent contract.

### Gap 6: TraceQuery runtime observations still rely on model-authored repo-evidence escape hatches

The 2026-06-14 low-leading run used `trace_query` first and recovered the
important runtime facts, but the first `emit_investigation_complete` was still
downgraded for lack of cite-eligible repo evidence. The model then spent many
rounds trying to emit `attached_trace.txt` as current-repo `emit_evidence`
before eventually setting an evidence-floor waiver.

Deep cause:

- `trace_query` publishes typed `ToolResult.Observations` in the runtime
  artifact lane, but `emit_investigation_complete` did not treat those rows as
  a sufficient completion boundary by themselves.
- The existing bypass required either an explicit `evidence_floor_waiver` or
  model-authored `aggregate_facts` with runtime origin dimensions. That pushes
  system bookkeeping onto the model and increases retry/grounding noise.

Commercial requirement:

- A successful `trace_query` result that carries hard-grounded runtime
  `ObservationRecord` rows must be sufficient to bypass current-repo citation
  floors when the typed request model does not require current-source evidence.
- The bypass must read only typed tool-result observations and source-lane
  predicates, not model prose.
- `emit_evidence` should remain a current-source/file-line lane; attached trace
  rows should flow through `trace_query` observations, completion reason, and
  runtime aggregate facts rather than being forced into repo evidence.

### Gap 7: Direct blocking surfaces need explicit downstream-cause consumption

In the same trace, `trace_query` correctly reported a synchronous-looking binder
wait from `com.baidu.tieba-59566` to `Binder:43397_19-23088`. Human audit shows
this is a direct blocking surface for part of the frame, not a complete root
cause by itself. The answer is complete only if it continues from the direct
wait into peer/on-chain scheduler state, wakeup-chain dependencies, and
same-window resource pressure.

Commercial requirement:

- Treat `critical_blocking_calls` rows as direct blocking surfaces.
- For binder/futex/lock/sync waits, continue to the peer thread, wakeup chain,
  `root_cause_rank`, and resource rows before naming the cause.
- If the peer row is missing or off-chain, keep the direct wait as a bounded
  symptom/candidate and state the caveat instead of promoting it to an
  independent root cause.
- Preserve `peer`, `chain_relevance`, overlap, nearest-chain thread, and
  `next_step` guidance in the final answer handoff.

## Validation Question Policy

Future validation questions for `xxx_all.systrace` should be low-leading and
multi-level:

- Use "卡顿原因" as the canonical wording.
- Ask for direct trigger, dependency-chain cause, scheduler/resource context,
  and auxiliary investigation directions.
- Do not include a negative hint that names a specific off-chain thread/state
  to rule out.
- Do not pre-give the expected wakeup chain; the system must derive it.

Example:

`只分析这份 trace，不分析代码。请分析 com.baidu.tieba 59566 主线程在 34579.472865s 到 34579.587805s 这一帧窗口内的卡顿原因，需要分层说明直接阻塞、依赖链原因、调度/资源背景压力，以及哪些只是辅助排查方向。`

## Delivery Plan

### Batch 1: Chain-aware root-cause ranking

- Add a shared root-cause sort key that prioritizes `chain_relevance` when a
  wakeup chain exists.
- Re-run chain-context enrichment after scheduler/compute/constraint candidates
  are appended.
- Ensure post-enrichment candidates get `chain_relevance=background` instead of
  empty/null when a chain exists.
- Add synthetic tests where a large off-chain compute-supply row must not outrank
  an on-chain dependency.

### Batch 2: Runtime observation projection hygiene

- Downgrade background `root_cause_rank` observations to supporting coverage
  when on-chain/adjacent root-cause rows exist.
- Keep background rows visible with rich notes, but prevent them from becoming
  `root_cause_primary` in the prompt/citation surface.
- Add tests over `traceQueryTypedObservations` to verify role/predicate
  behavior for on-chain vs background root-cause rows.

### Batch 3: Trace-only routing and stage wording

- Audit runtime trace-only classification and route hints to avoid unnecessary
  source-lane indexing/tool use when the user explicitly excludes code/source.
- Tighten explorer fallback guidance so `grep`/`read_file` is used only after
  `trace_query` is incomplete or for a trace-local line window that cannot be
  represented structurally.
- Adjust user-facing stage wording for observation-only runtime trace lanes.

### Batch 4: Runtime extract/final answer alignment

- Ensure observation-only trace answers do not require source-style hypothesis
  verdicts.
- Strengthen answer guidance so direct trigger, dependency-chain cause,
  scheduler/resource context, and auxiliary directions remain separate.
- Add regression tests for the runtime observation-only extractor path.

### Batch 5: Source-optional runtime source-status isolation

- Broaden orchestrator/extractor auto-verdict suppression from the narrow
  observation-only predicate to runtime artifacts without required current-source
  evidence.
- Make the source-lane predicate attached-trace-aware, so `--htrace` /
  `--atrace` runtime context is not lost when the pre-triage bundle is sparse.
- Align extractor hypothesis prompt wording with this broader source-lane
  boundary.
- Add regression tests for source-optional runtime artifacts where no
  `current_source_mode=exclude` is present.
- Add finalizer/tool guidance so `current_status_verdict` and current-code
  persistence prose are not used unless the typed current-status diagnostic
  contract is active.

### Batch 6: Real-trace validation

- Run the low-leading `xxx_all.systrace` validation question without mentioning
  isplogcat.
- Verify the answer reports multi-level causes:
  direct sleep/wakeup or binder-like trigger when grounded, dependency chain,
  runnable/sleep/D-state split by chain node, scheduler/resource background,
  and auxiliary-only rows.
- Verify no off-chain log/system thread is presented as primary when on-chain
  evidence exists.
- Verify the answer explicitly names both runnable scheduling delay and D-state
  IO wait when both are present in the frame, without adding current-code
  persistence verdicts.

### Batch 7: TraceQuery runtime observation completion boundary

- Add an `emit_investigation_complete` pre-complete bypass that recognizes
  successful `trace_query` `ToolResult.Observations` in the runtime artifact
  lane when current-source evidence is not required.
- Keep the bypass typed-only: inspect producer, origin/source-ref kind, success,
  and source-lane predicates; never parse model-authored prose.
- Add tests where a source-optional attached trace with only `trace_query`
  observations can complete without `aggregate_facts` or
  `evidence_floor_waiver`.

### Batch 8: Direct blocking handoff and answer guidance

- Update runtime trace answer guidance so direct blocking rows are rendered as
  the first layer, then explicitly reconciled with wakeup-chain/root-rank/resource
  rows.
- Keep binder wait peer identity in structured observation fields rather than
  only in summary text.
- Add tests for critical-blocking typed observations where `object` preserves
  the peer and `rich_notes` preserves the blocking type.
