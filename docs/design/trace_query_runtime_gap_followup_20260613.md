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
- [x] Follow-up route-cost batch: make typed runtime-artifact/source-optional
  requests avoid analyzer repo overview and explorer source breadth unless
  current source is a typed required lane.
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
- Close the route-cost gap with existing typed lane predicates: analyzer uses
  `HasRuntimeArtifactWithoutRequiredCurrentSourceInTraceContext(attachedTrace)`
  to skip precomputed repo overview for source-optional runtime traces, while
  explorer starts from runtime/trace instructions instead of `## Breadth Scan`.
  Requests with typed required current-source anchors still keep the source
  lane.

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
- [x] Add route-cost tests for analyzer source-optional runtime shortcuts,
  current-source-required escape, and explorer trace_query-first startup.

### Batch 12: Multi-window on-chain occurrence preservation

Gap:

- `wakeup_chain.aggregated_impacts` correctly groups repeated fragmented
  dependency branches and `root_cause_rank` correctly compares same-chain rows
  by cumulative impact, but the aggregate only preserved first/last timestamps,
  line range, and occurrence count.
- That loses answer-critical detail for commercial trace triage: repeated
  windows that share the same on-chain path cannot be enumerated later, so a
  final answer may say "aggregate D/IO on chain" without stating which concrete
  target-blocking windows share that upstream D/IO dependency.
- The same shape applies beyond one Donghu frame: any repeated on-chain
  runnable, D/IO, sleep, or compute-supply dependency needs aggregate ranking
  plus bounded representative occurrences. Single-row `state_churn` already
  carries per-state totals, fragment count, max/p95 segment, runnable context,
  and next-step guidance; the missing granularity is specifically the repeated
  common dependency path aggregate.

Design:

- Add output-only `WakeupCausalOccurrence` and carry
  `occurrence_windows` on both `WakeupCausalAggregate` and the
  `RootCauseRankItem` generated from that aggregate. This is not a model
  tool-call input, so no new JSON repair alias is required.
- Preserve at most 8 representative occurrence windows per aggregate. Select
  the highest-impact occurrences by total/target/dominant impact, then render
  the selected set chronologically so answers can describe the frame timeline
  naturally without unbounded payload growth.
- Each occurrence keeps the concrete window, dominant state, dominant impact,
  total/target impact, running/runnable/sleep/D/IO totals, fragment/switch
  counts, max/p95 segment, line range, and summary.
- Render a compact `occurrence_windows=...` field in `aggregated_impact` and
  `root_cause_rank` summary rows, plus expanded `aggregate_occurrence` and
  `rank_occurrence` detail lines for human-readable diagnostics.
- Propagate the field through typed observations, summary-to-ledger fallback,
  finalizer supplement whitelist, trace_query description/schema, shared view
  teaching, explorer start guidance, and final-answer runtime handoff guidance.
- Keep existing ranking semantics: on-chain rows outrank adjacent/background,
  same-chain primary rows sort by `cumulative_impact_ms`, and background
  pressure remains auxiliary unless bounded chain/overlap evidence proves
  direct impact.

Tasks:

- [x] Add `WakeupCausalOccurrence` and output-only `occurrence_windows`.
- [x] Populate and cap occurrence windows during wakeup-chain aggregation.
- [x] Carry occurrence windows into aggregate-derived root-cause rank rows.
- [x] Render compact and expanded occurrence details in trace_query summaries.
- [x] Publish occurrence windows in typed observations and summary fallback.
- [x] Update final supplement, schema/tool description, explorer guidance,
  final-answer guidance, and shared trace-query view teaching.
- [x] Add focused tests for aggregate/root-rank occurrence preservation,
  summary rendering, typed observations, final supplement, and prompt/schema
  teaching.

### Batch 13: Real large-trace eval guard residuals

Audit input:

- Real trace: `/Users/han/opt/customlogs/xxx_all.systrace`
- Parallel eval batch:
  - `trace_query_donghu_real_frame_multicausal`
  - `trace_query_donghu_real_short_runnable`
- Captured outputs:
  - `.codrax/tmp/eval_results/trace_query_donghu_real_frame_multicausal-20260614-022651`
  - `.codrax/tmp/eval_results/trace_query_donghu_real_short_runnable-20260614-022652`

Observed residual gaps:

- The eval harness only supported inline `LOG` / `HTRACE` attachments. A
  customer-scale systrace cannot be passed through `--htrace-text` safely
  because shell argument size becomes a hidden failure mode. The same gap also
  affected older oversized log cases that declared `LOG_FILE`.
- The trace-only answer recovered the important runtime facts, but the final
  answer could still render an optional `decision` block containing
  `still_present` / `not_enough_evidence` and current-code persistence wording.
  This happened even though the typed source lane was source-optional and no
  current-source observation was available. A second audit showed the output
  cleanup was necessary but insufficient: the upstream `AnswerSurfacePlan` /
  semantic-view builders could still mark `CurrentStatusDiagnosticRequired`
  true because they did not see attached trace or later `trace_query` runtime
  observations when `RequestModel.PerfTrace` was absent.
- The trace_query JSON and summary already carried the repeated on-chain
  occurrence windows for the Donghu frame. The visible answer layer did not
  reliably keep those concrete windows in front of the model/user because the
  deterministic trace_query supplement capped rows after ordinary root-cause
  rows.
- The first low-leading eval expectations used a phrase in
  `EXPECT_NOT_CONTAINS`; the shell runner splits that field by whitespace, so
  a normal word such as `not` became a banned token and created false failures.
- Direct path-in-question support had not been evaluated. `trace_query` already
  supports `source=path` with repo/workspace-relative or absolute paths, and the
  analyzer/explorer have explicit runtime trace path guidance, but coverage only
  exercised `--htrace` attachment input. Runtime log paths mentioned in the
  question are weaker: they can be read as files, but they do not yet have a
  trace_query-equivalent structured runtime-artifact route.
- The real-frame rerun exposed a cross-stage handoff leak: source-optional
  runtime trace analysis no longer required hypothesis verdicts, but the
  extractor stage still had no deterministic `StageReport`, so BaseAgent could
  auto-capture the extractor model's free-form "inconclusive / no repo code
  evidence" explanation and pass it to the finalizer. That polluted otherwise
  correct trace-only answers with repo-status noise.
- The 2026-06-14 short-window rerun no longer emitted current-status verdicts,
  but manual audit showed the final answer could still lead with perf-triage
  pre-scan seeds (`perf_jank` / `perf_stall` / `perf_observation`) even after
  bounded `trace_query` rows had identified the requested CookieMonsterCl ->
  com.baidu.tieba wakeup path. ObservationLedger already demoted perf pre-triage
  rows when `trace_query` existed, but the separate `ExternalObservationSeeds`
  support-lane projection did not consume that demotion.

Design:

- Add file-path attachment support to the eval runner:
  `LOG_FILE` maps to `--log`, `HTRACE_FILE` maps to `--htrace`, with mutual
  exclusion against inline attachment variables and between log/trace lanes.
- Reuse the existing typed predicate
  `HasRuntimeArtifactWithoutRequiredCurrentSourceInTraceContext(attachedTrace)`
  at the surface-plan layer and final-answer cleanup layer. If runtime trace
  observations are answer-grade, current-source is not required, and the
  accepted ledger has no current-source observation, close the current-status
  diagnostic obligation before semantic-view compilation and remove optional
  decision blocks when other visible payload exists. This is typed output
  hygiene; it does not parse model prose and does not affect active
  current-status contracts backed by current-source evidence.
- Treat `trace_query` typed observation rows as answer-surface revision inputs.
  Appending such rows bumps the mutable answer-surface revision so cached Bus
  plans/views cannot remain stuck in the pre-trace state.
- Keep current-source backed decisions intact: a non-runtime current-source
  citation carrier or a current-source ledger observation blocks the cleanup.
- Prioritize trace_query observation-supplement rows that carry
  `occurrence_windows=`. This preserves repeated fragmented dependency chains
  such as the three ThreadPoolForeg D/IO windows even when ordinary primary
  rows are numerous.
- Keep eval negative checks exact-token only (`still_present`,
  `not_enough_evidence`) to avoid false failures from normal prose words.
- Add path-in-question evals:
  - Absolute trace path: `/Users/han/opt/customlogs/xxx_all.systrace`
  - Relative trace path: `../customlogs/xxx_all.systrace`
  - Relative log path: `eval/fixtures/runtime_path_panic.log`
  These cases must not attach artifacts through eval runner variables; they
  validate whether the model can discover and consume paths from the user
  question using the normal tool schema and hints.
- Add a deterministic extractor `StageReport`, mirroring the explorer P1.2
  design. For source-optional runtime artifact turns, the report carries only
  typed lane counts/result-kind metadata and explicitly does not serialize
  model-authored hypothesis verdict prose. This preserves structured runtime
  facts while preventing optional analyzer hypotheses from becoming final-answer
  obligations.
- Split two runtime trace signals at the surface-plan layer:
  runtime-trace-artifact presence can close source-optional current-status
  obligations, while `producer=trace_query` hard observations alone trigger
  removal of perf pre-triage `ExternalObservationSeeds` from the principal
  support lane. The perf pre-triage rows remain available in ObservationLedger
  as low-priority advisory records; they just stop competing with bounded
  trace_query rows for the final answer's primary cause order.

Tasks:

- [x] Add `LOG_FILE` / `HTRACE_FILE` support and validation to `eval/run.sh`.
- [x] Add low-leading real-trace evals for the full multi-cause frame and the
  short runnable-only window.
- [x] Replace broad negative phrases in those evals with exact status tokens.
- [x] Add source-optional runtime trace decision-block normalization using
  typed source-lane and observation-ledger predicates.
- [x] Add unit tests that drop source-status decisions for trace-only answers
  while preserving decision-only payloads and current-source-cited decisions.
- [x] Prioritize `occurrence_windows` trace_query supplement rows and add a
  finalizer test where the aggregate would otherwise be clipped by eight
  ordinary root rows.
- [x] Apply attached-trace / trace_query source-optional current-status
  suppression before semantic-view compilation, guarded by current-source
  ledger observations.
- [x] Bump answer-surface revision when trace_query publishes hard runtime
  observations, preventing stale BusContext plan caches.
- [x] Add surface-plan tests for trace_query-driven current-status suppression,
  cache invalidation after trace_query, and current-source evidence preserving
  current-status diagnostics.
- [x] Add path-in-question eval cases for absolute trace path, relative trace
  path, and relative log path.
- [x] Add deterministic extractor `StageReport` for source-optional runtime
  artifact turns and a regression test that model-authored hypothesis verdict
  prose does not leak into cross-stage handoff.
- [x] Filter perf pre-triage external-observation seeds from the support lane
  when answer-grade `trace_query` observations exist, while preserving them for
  perf-only artifact questions.
- [x] Add tests for both sides of that filter: trace_query suppresses perf
  pre-triage seeds, perf-only keeps them.
- [x] Add path-in-question eval cases for absolute log path, multiple log
  paths, and multiple trace paths using lightweight fixtures.
- [x] Re-run the two real-trace eval cases with parallelism 2 after the code
  changes, then record pass/fail and any remaining gaps before expanding the
  eval set further.
- [x] Run the path-in-question eval cases in batches of 2 and record whether
  trace path and log path support are stable or still need a dedicated runtime
  artifact routing design.

## 2026-06-14 Explicit Runtime Path Audit

Scope:

- Users may provide runtime artifacts without `--log` / `--htrace` attachment by
  writing one or more paths directly in the question.
- Covered path families: absolute and relative `.log`, `.systrace`, `.htrace`,
  `.atrace`, `.trace`, `.perfetto`, plus synthetic eval fixtures for multiple
  runtime files.
- Hard gates must use typed/path-shape signals only: analyzer structured fields,
  tool-call parameter strings, and exact current-request path substrings. No
  final-answer prose parsing and no keyword intent matching.

Audit results before the Batch 14 fix:

- Explicit trace paths were already usable through `trace_query source=path`.
  Absolute, relative, and multi-trace eval cases reached `trace_query` and did
  not need attachment variables.
- Explicit log paths were answerable through `read_file`, but local `.log`
  evidence could be misprojected as `current_source` in the authority/ledger
  path. That allowed current-status wording such as `still_present` /
  `not_enough_evidence` in one earlier absolute-path run.
- The analyzer prompt taught direct classification for trace paths, but not log
  paths. One absolute-path eval run used `repo_map` / `list_files` on the log's
  parent directory before `emit_analysis`, despite the user saying not to
  analyze code.
- `emit_hypothesis_verdict` prompt/hint said `runtime_artifact:1-5` was valid
  for observation-only runtime artifacts, but the validator only recognized that
  form when a `LogBundle`/`PerfBundle` or attachment field existed. Explicit
  path-only log runs therefore saw a misleading rejection even though the final
  answer recovered.
- `emit_analysis.required_files` still had avoidable JSON-shape friction:
  `required_files:["path.log"]` was a common model shorthand but previously
  forced a strict-decode retry instead of being repaired into object entries.

Design:

- Promote explicit runtime artifact paths to a typed request signal via
  `RequestModel.HasRuntimeArtifactPathReference()` and
  `RuntimeArtifactPathReferenceKind()`. Strong structured carriers
  (`required_files`) win over softer entity/mentioned-entity carriers when
  choosing log vs trace.
- Project local runtime artifact path evidence as pure artifact support:
  `ClaimOriginLog` for log paths, `ClaimOriginPerf` for trace paths, then
  ObservationLedger emits `origin=runtime_artifact` with
  `source_ref.kind=runtime_artifact`.
- Let AnswerSurfacePlan suppress current-status/current-source obligations for
  explicit runtime path artifacts when current-source is not required and the
  ledger has no current-source observation. Use external-log disposition for
  log paths and external-trace disposition for trace paths.
- Add an analyzer first-turn runtime artifact path shortcut and schema filter:
  when the current request carries concrete path-like runtime artifact tokens,
  StageAnalyze exposes only `emit_analysis`. This prevents repo pre-scan from
  being the normal path for `.log` / `.systrace` questions while preserving
  mixed runtime-artifact + current-source semantics in the emitted model.
- Keep a matching analyzer hard gate and retry hint for providers that still
  attempt unavailable tools: any StageAnalyze `repo_map` / `grep` / `list_files`
  or other non-`emit_analysis` tool call under the explicit runtime path signal
  fails with a precise `runtime-artifact-path-first` repair hint.
- Limit the hard signal to path-like artifact names (`foo.log`,
  `dir/foo.systrace`, absolute paths, and attachment basenames), not bare
  extension discussions such as `.log` implementation questions.
- Align prompt and validator: analysis-skill now names `.log` as an explicit
  runtime artifact path, and `emit_hypothesis_verdict` accepts artifact-local
  `log:N` / `trace:N-M` / `runtime_artifact:N-M` citations when the request
  model carries an explicit runtime path and current-source is optional.
- Extend the `emit_analysis` JSON repair layer so string entries in
  `required_files` are upgraded to `{path, confidence, rationale}` before
  strict decode; semantic validation still decides whether the path is usable.

Tasks:

- [x] Add explicit runtime path request-kind projection and tests.
- [x] Add authority projection for `.log` / trace local artifact evidence.
- [x] Add ObservationLedger runtime-artifact source refs for log/perf evidence
  items.
- [x] Add AnswerSurfacePlan log-path current-status suppression test.
- [x] Add `required_files` string-entry JSON repair and test.
- [x] Update analysis-skill and analyzer shortcut prompt text from trace-only
  to log/trace runtime artifact paths.
- [x] Add analyzer hard gate for exact current-request runtime artifact path
  pre-scan attempts and test parent-directory coverage.
- [x] Add analyzer first-turn schema filtering and retry-hint alignment so
  explicit runtime artifact path classification is `emit_analysis`-only before
  the model can call repo pre-scan tools.
- [x] Add tests that bare `.log` extension discussions do not trigger the
  runtime artifact path gate.
- [x] Tighten runtime trace explorer guidance so answer-grade `trace_query`
  observations complete through `emit_investigation_complete` instead of being
  repackaged through `emit_evidence`.
- [x] Extend `emit_hypothesis_verdict` artifact-local citation normalization to
  explicit path-only runtime artifacts and add a regression test.
- [x] Run package tests:
  `go test ./internal/agent ./internal/types ./internal/authority ./internal/tool ./internal/skill -count=1`.
- [x] Rebuild eval snapshot and rerun path-in-question evals in batches of two:
  single log absolute/relative, multi log + multi trace, and trace
  absolute/relative if time permits.
- [x] Record post-fix eval pass/fail and manual audit notes here.

Post-fix eval results:

- Package tests passed:
  `go test ./internal/agent ./internal/types ./internal/authority ./internal/tool ./internal/skill -count=1`.
- Single explicit log path, no attachments:
  - `log_path_question_absolute_panic-20260614-040321`: 3/3 PASS;
    `tool_read_file=1`, `tool_repo_map=0`, `tool_list_files=0`,
    `tool_trace_query=0`, `answer_contract_violations=0`.
  - `log_path_question_relative_panic-20260614-040321`: 3/3 PASS;
    `tool_read_file=1`, `tool_repo_map=0`, `tool_list_files=0`,
    `tool_trace_query=0`, `answer_contract_violations=0`.
- Multiple explicit runtime paths:
  - `log_path_question_multi_runtime_files-20260614-040711`: 3/3 PASS;
    `tool_read_file=4`, `tool_repo_map=0`, `tool_list_files=0`,
    `tool_trace_query=0`. One run needed answer-contract repair but final
    output passed.
  - `trace_query_path_question_multi_trace_files-20260614-040712`: 3/3 PASS;
    median `tool_trace_query=4`, `tool_repo_map=0`, `tool_list_files=0`.
    Some runs used `read_file` for artifact-local follow-up, not repo search.
- Real Donghu trace path from the question:
  - `trace_query_path_question_absolute_donghu_short-20260614-041423`:
    3/3 PASS; median `tool_trace_query=4`, `tool_repo_map=0`,
    `tool_list_files=0`, `answer_contract_violations=0`.
  - `trace_query_path_question_relative_donghu_short-20260614-041423`:
    3/3 PASS; median `tool_trace_query=3`, `tool_repo_map=0`,
    `tool_list_files=0`.
- Existing real-trace regression cases after the path gate:
  - `trace_query_donghu_real_short_runnable-20260614-042145`: 3/3 PASS;
    median `tool_trace_query=6`, `tool_repo_map=0`, `tool_list_files=0`,
    `answer_contract_violations=0`.
  - `trace_query_donghu_real_frame_multicausal-20260614-042145`: 3/3 PASS;
    median `tool_trace_query=18`, `tool_repo_map=0`, `tool_list_files=0`.
    Two runs needed answer-contract repair and one/two tool-history prunes, but
    final answers passed and no source-code search was attempted.
- After tightening the explorer runtime trace guidance, reran smoke cases:
  - `log_path_question_absolute_panic-20260614-043818`: 3/3 PASS;
    `tool_read_file=1`, `tool_repo_map=0`, `tool_list_files=0`,
    `answer_contract_violations=0`.
  - `trace_query_path_question_multi_trace_files-20260614-043818`: 3/3 PASS;
    median `tool_trace_query=4`, `tool_read_file=0`, `tool_repo_map=0`,
    `tool_list_files=0`, `answer_contract_violations=0`. The earlier
    trace-query-to-emit-evidence repackaging noise was not reproduced in this
    smoke run.
