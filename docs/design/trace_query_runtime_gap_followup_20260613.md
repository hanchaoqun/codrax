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

### Batch 9: On-chain IO co-primary and blocking peer-state decomposition

Gap confirmed from the Donghu frame audit:

- A dependency thread on the actual wakeup chain can spend a material slice in
  D-state/io_wait while another chain node contributes runnable scheduling
  delay. The D/IO dependency is part of the same causal path and must not be
  rendered as generic background pressure merely because runnable rows sort
  ahead of it.
- `critical_blocking_calls` carried binder peer identity, but did not
  structurally decompose the peer thread state in the bounded wait window. That
  let final answers stop at "binder synchronous wait" instead of continuing to
  the peer's scheduler state or stating a precise trace gap.

Design:

- Extend `root_cause_rank` items with `dominant_state` plus
  `running/runnable/sleep/d_state/io_wait` totals. This keeps the state evidence
  on the same ranked row even when the type is a broader
  `priority_inversion_candidate`.
- Assign tiers with causality semantics as well as rank position: on-chain
  D-state/IO rows (`io_wait`, `d_state_or_io_wait`, `io_burst_episode`,
  `block_io_by_inode`, `fragmented_d_state_or_io_wait`, or a priority-inversion
  candidate whose typed state totals show D/io_wait) remain `tier=primary`.
  Background rows still stay background/supporting because promotion requires
  typed `on_chain` causality.
- Extend `critical_blocking_calls` with output-only `peer_state`, computed by
  reusing `ThreadTimeline` over the candidate's peer and bounded wait window.
  This is generic for binder/futex/lock/sync/IO candidates and does not read
  model-authored prose.
- Render the new fields through the tool banner, typed observations,
  finalizer supplement notes, explorer guidance, runtime answer guidance, tool
  schema description, and the shared trace-query view teaching matrix.
- No model tool-call JSON input is added. Existing JSON repair remains focused
  on input fields such as `view` aliases and `event_types`; `peer_state` and the
  state-total fields are result fields consumed downstream.

Tasks:

- [x] Add `RootCauseRankItem` state-total fields and populate them from causal
  impacts, window stats, state churn, IO bursts, and scheduler-latency rows.
- [x] Replace rank-position-only tier assignment with a typed co-primary rule
  for on-chain D-state/IO dependencies.
- [x] Add `CriticalBlockingCandidate.peer_state` and compute it from peer
  thread timeline intervals.
- [x] Propagate the new output fields into trace-query banners and
  `traceQueryTypedObservations` rich notes.
- [x] Update runtime trace prompt/handoff guidance and shared view teaching so
  models consume `peer_state` and state totals without guessing from prose.
- [x] Add unit tests for on-chain runnable + D/IO co-primary behavior and
  binder peer-state decomposition.

### Batch 10: Generalized on-chain primary layers and fragmented-chain aggregation

Additional audit requirement:

- Every structurally on-chain blocking/supply layer can be a primary cause:
  runnable delay, D-state/io_wait, running work that lacks compute supply,
  low CPU frequency, unreasonable affinity/cpuset binding, CPU frequency limit
  context, and supply-side DDR/L3/thermal context when tied to a bounded chain
  row. These must compete as root causes instead of being summarized only as
  background.
- Sleep on a dependency chain is a waiting surface. The chain should continue
  through the top sleep interval until a wakeup edge, IPC edge, root evidence,
  or trace gap stops it. Avoid expanding every tiny sleep fragment, but do not
  stop at a top sleep row that has a structural waker.
- Multiple fragmented target sleeps can share the same upstream chain. Their
  cumulative common dependency can exceed one larger-looking continuous state,
  so the system needs an aggregate lane in addition to per-branch rows.

Design:

- Extend co-primary tiering to all typed on-chain dependency causes:
  `runnable_wait`, `scheduler_latency`, `priority_inversion_runnable_wait`,
  `running`, `fragmented_running`, `compute_supply`, `low_frequency`,
  `cpu_affinity_or_cpuset`, `io_wait`, `d_state_or_io_wait`, `io_latency`,
  `io_pressure`, `io_burst_episode`, `block_io_by_inode`,
  `file_io_hot_inode`, and fragmented D/IO/runnable rows. Promotion still
  requires `chain_relevance=on_chain` or `causality=on_wakeup_chain`.
- Preserve compute-supply state totals on root-cause rows:
  running supply candidates carry `running_ms`; runnable supply/affinity rows
  carry `runnable_ms`.
- Add `wakeup_chain.aggregated_impacts`: group repeated causal impacts by
  thread and dominant state, preserve `path`, occurrence count, cumulative
  state totals, target-blocked total, line range, and priority relation.
- Feed aggregate rows into `root_cause_rank` as
  `source=wakeup_chain.aggregated_impacts`, so fragmented common paths can rank
  above single branch rows when their cumulative impact is larger.
- Render aggregate fields through the trace_query banner, typed observations,
  finalizer supplement whitelist, explorer guidance, finalizer runtime
  guidance, tool description, and shared trace-query view teaching.

Tasks:

- [x] Expand co-primary tiering to on-chain runnable/running/compute-supply and
  affinity/low-frequency causes.
- [x] Populate state totals for compute-supply and CPU-affinity root-cause rows.
- [x] Add wakeup-chain aggregate data model and deterministic aggregation.
- [x] Add aggregate root-rank candidate generation.
- [x] Add banner, typed-observation, prompt, and teaching surfaces for
  aggregated wakeup impacts.
- [x] Add tests for compute-supply co-primary and fragmented common dependency
  aggregation.

### Batch 11: Real trace replay residuals and binder semantics hardening

Audit input:

- Real trace: `/Users/han/opt/customlogs/xxx_all.systrace`
- Low-leading frame question:
  `只分析这份 trace，不分析代码。请分析 com.baidu.tieba 59566 主线程在 34579.472865s 到 34579.587805s 这一帧窗口内的卡顿原因，需要分层说明直接阻塞、依赖链原因、调度/资源背景压力，以及哪些只是辅助排查方向。`
- Direct-blocking question:
  `只分析这份 trace，不分析代码。请分析 com.baidu.tieba 59566 主线程在 34579.47s 到 34579.49s 之间是否存在 binder、futex、锁或同步等待等直接阻塞；如果有，继续拆到 peer 线程状态和上游调度/唤醒证据，说明还缺哪些证据。`
- Captured outputs:
  `.codrax/tmp/xxx_trace_eval/case1_frame.out`,
  `.codrax/tmp/xxx_trace_eval/case1_frame.err`,
  `.codrax/tmp/xxx_trace_eval/case2_binder.out`,
  `.codrax/tmp/xxx_trace_eval/case2_binder.err`.

Observed residual gaps:

- Trace-only requests still started repository indexing and perf-triage
  `read_file` pagination over `.codrax/blob/.../attached_trace.txt`. This is a
  route/stage-cost gap: `trace_query` eventually runs, but user-facing progress
  still feels like source-code analysis.
- Analyzer `subtopic_coherence` R1.5 can still treat runtime artifact entities
  as repo-symbol obligations when the trace lane is source-optional rather than
  explicitly source-excluded.
- Explorer/finalizer can fall back to `grep`/`read_file` after a successful
  bounded `trace_query`, especially for VSync/frame marker lookup. This should
  remain a fallback only after a trace_query coverage caveat or unsupported
  format signal.
- Extract/hypothesis verdicts can drift toward current-source wording when the
  attached trace exists but the perf-triage bundle is sparse.
- Binder raw `flags` were visible, but the human-readable summary did not
  surface deterministic `oneway` / `sync_like` / `blocking_candidate`
  semantics strongly enough. In the real trace, `flags=0x12` is sync-looking
  (`oneway=false`) and must not be described as one-way async.

Design:

- Use the existing typed source-lane predicate
  `HasRuntimeArtifactWithoutRequiredCurrentSource()` for analyzer R1.5 runtime
  artifact bypass, not only explicit external-only policy. This is a typed
  source/evidence-lane decision and does not inspect user prose.
- Treat `.codrax/blob/.../attached_trace.txt`, `attached_hitrace.txt`,
  `attached_atrace.txt`, and `attached_log.txt` as runtime artifact paths so
  source-lane decisions do not accidentally require current-source evidence for
  attached runtime blobs.
- Keep hypothesis-verdict artifact citations attached-trace-aware via
  `HasRuntimeArtifactWithoutRequiredCurrentSourceInTraceContext(attachedTrace)`;
  runtime artifact line refs are preserved in rationale/notes and are not
  published as repo file citations.
- Promote binder flag semantics to first-class structured output:
  `IPCEdge`, `BinderWaitSummary`, and binder-derived
  `CriticalBlockingCandidate` rows expose `oneway`, `sync_like`, and
  `blocking_candidate`. Summaries, banners, evidence packs, typed observations,
  trace_query schema text, shared view teaching, explorer guidance, default
  skill guidance, and final-answer guidance all instruct the model to consume
  these fields instead of decoding raw flags.
- Keep non-binder critical-blocking rows clean: optional bool fields are only
  emitted when binder semantics exist, while binder rows preserve explicit
  `false` values such as `oneway=false`.

Tasks:

- [x] Classify attached runtime blob basenames as runtime artifact paths.
- [x] Make hypothesis verdict artifact-citation normalization attached-trace
  aware when current source is not required.
- [x] Broaden analyzer R1.5 runtime artifact resolver bypass from
  external-only to source-optional runtime artifacts.
- [x] Add binder semantic fields to IPC edges and binder wait summaries.
- [x] Carry binder semantic fields into critical-blocking rows, banners,
  evidence summaries, and typed observation rich notes.
- [x] Update trace_query schema/description, shared trace-query view teaching,
  explorer prompt, default skill prompt, and final-answer guidance so the model
  consumes `oneway`/`sync_like`/`blocking_candidate`.
- [x] Add focused tests for source-optional runtime artifact R1.5 bypass,
  attached trace artifact citation, attached blob path classification, binder
  `flags=0x12` vs `0x11` semantics, critical-blocking peer-state + binder
  semantics, schema/prompt teaching, and typed-observation preservation.
- [ ] Follow-up route-cost batch: make explicit runtime trace-only requests
  avoid user-facing repo indexing/stage wording unless current source is a
  typed required lane.
- [x] Follow-up fallback batch: restrict post-trace_query source/generic tool
  fallback when `trace_query` has already published hard runtime observations;
  leave fallback available after failed/empty trace_query and for typed
  current-source-required requests.

### Batch 12: On-chain cumulative ordering and trace-query fallback closure

Audit input:

- The Donghu frame analysis can contain multiple primary layers on the same
  wakeup chain: runnable scheduling delay, D-state/io_wait, and compute-supply
  limits. A row's `score` intentionally includes confidence and type weights,
  so score alone is not the right answer to "which on-chain cause has the
  largest total impact".
- After `trace_query` succeeds with structured runtime observations, source
  tools can still be attempted in source-optional trace-only exploration. This
  reintroduces repository-analysis drift even though the deterministic trace
  lane already has hard evidence.

Design:

- Add output-only `RootCauseRankItem.cumulative_impact_ms`. It is not a model
  tool-call JSON input, so no new compat alias is required. Existing JSON
  repair remains focused on model-authored input fields such as `view`,
  `event_types`, timestamps, and selector aliases.
- Keep `impact_ms` as the effective ranking impact used by the existing score
  model, including background caps and state-churn effective impact. Use
  `cumulative_impact_ms` for the actual total row impact: causal-impact
  `TotalMs`, aggregate `TotalMs`, state-churn `TotalMs`, or the raw
  window-stat duration before background capping.
- Preserve the existing chain relevance ordering:
  `on_chain -> adjacent -> background`. Within the `on_chain` group, sort by
  `cumulative_impact_ms` before `score`, then by effective `impact_ms` and
  line order. This lets chain-local D/IO, runnable, running/compute-supply, and
  affinity rows be compared by total bounded impact without promoting
  off-chain background pressure.
- Render `cumulative_impact_ms` through the trace_query JSON payload, summary
  banner, summary-to-ledger compatibility parser, typed observations, finalizer
  supplement whitelist, explorer prompt, final-answer runtime guidance, tool
  schema/description, and shared trace-query view teaching.
- Add a structural explorer guard: once a successful `trace_query` result has
  published hard runtime-artifact observations, reject source/generic fallback
  tools (`repo_map`, `grep`, `list_files`, `read_file`, `exec_command`) unless
  the typed request model requires current-source evidence. Continue allowing
  `trace_query` follow-up and `emit_investigation_complete`. The guard uses
  typed observation metadata only; it does not parse model prose or user intent
  keywords.

Tasks:

- [x] Add `cumulative_impact_ms` to `RootCauseRankItem`.
- [x] Populate cumulative impact from causal impacts, aggregated impacts,
  state churn, scheduler latency, compute supply, CPU constraints, IO/resource
  rows, and default root-cause candidates.
- [x] Sort same-chain `root_cause_rank` rows by cumulative impact before score.
- [x] Propagate the field through banners, typed observations, summary ledger
  compatibility, supplement notes, tool schema, shared view teaching, explorer
  guidance, and final-answer guidance.
- [x] Add tests for cumulative ordering beating score within the on-chain
  group, typed observation preservation, schema/prompt teaching, and final
  supplement visibility.
- [x] Add tests for post-trace_query fallback: failed/empty trace_query still
  allows fallback; successful hard runtime observations block source/generic
  fallback but keep trace follow-up/completion available.
