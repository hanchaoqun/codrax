# Trace Query Donghu/Harmony Rich Trace Gap Audit (2026-06-13)

## Scope

Audit target: `/Users/han/opt/customlogs/xxx_all.systrace`

Primary frame/thread under review: `com.baidu.tieba-59566`, focused window around
`34579.472s..34579.588s`.

This document records what the trace contains, what the current `trace_query`
pipeline can already consume, and which generic system-level gaps remain. The
goal is to improve a class of Donghu/Harmony frame-root-cause traces, not to
fit one customer sample.

Most signals in this sample are Harmony/OpenHarmony hitrace/ftrace family
signals rather than Donghu-only signals: scheduler state/wakeup rows, Harmony
priority semantics, `sched_switch next_info` affinity/restricted fields,
filesystem/page-cache/storage rows, IRQ/softirq, workqueue, trace marks, and
clock/frequency/resource rows. Donghu is treated as one rich mixed-platform
sample in that family. Implementations and evals must key off producer event
families and typed fields, not off the Donghu customer name or model prose.

## Trace inventory

Whole file:

- 15,623 lines, about 1.8 MB.
- Top event families:
  - `sched_switch`: 3517
  - `sched_wakeup`: 2122
  - `irq_handler_entry/exit`: 2045 each
  - `print` trace marks: 1680
  - `cpu_idle`: 1278
  - `sched_blocked_reason`: 441
  - `mm_filemap_add_to_page_cache`: 439
  - `clock_set_rate`: 323
  - `block_rq_issue/complete`: 276 each
  - `block_bio_remap`: 269
  - `softirq_entry/exit`: 263 each
  - `cpu_frequency`: 90
  - `binder_transaction/received`: 79 each
  - `workqueue_execute_start/end`: 53 each
  - `mm_filemap_delete_from_page_cache`: 24

Focused frame window:

- Scheduler and wakeups dominate the raw line count, but IRQ/softirq, page
  cache, block IO, blocked reason, trace marks, clock/frequency, binder, and
  workqueue all overlap the same frame.
- High-volume page-cache inodes include `7:123/0x10`, `511:0/0x740fd2b`,
  `7:33/0x1192`, and multiple `260:67/*` inodes.
- High-byte block issuers include `wk:0/-20/0/6`, `os.FusionSearch`,
  `hmfs_txn-260-67`, `sysevent_store`, `ThreadPoolForeg`, and
  `BdAsyncTask #8`.
- `sched_blocked_reason` has both `iowait=1` callers such as
  `fscache_page_wait_o*`, `sync_buffer_read_wi*`, `hmfs_dio*`, `ext4_dio*`,
  and `iowait=0` driver/device waits such as `mmc_wait_for_req_do*`,
  `hcc_transfer_thread*`, `worker_thread*`, and `down_interruptible*`.
- Trace marks contain frame/render-service spans, present fence spans, lock
  contention spans, native async file-operation spans, audio pipeline spans,
  buffer queue spans, futex wait/wake spans, and binder transact labels.
- `clock_set_rate` is rich: `thermal_inte1`, `ddr_cluster1_freq`,
  `l3c_cluster1_freq`, `l3cache`, `ddr_throughput`, and temperature/power
  signals occur inside the same frame.

## Current capabilities

Already implemented or present in the current code path:

- Scheduler timeline, state duration, runnable/D/sleep/running classification,
  and fragmented state churn.
- Wakeup-chain recursion from target sleep windows, including causal impact
  summaries and priority-inversion candidate classification under Donghu/Harmony
  priority semantics.
- Root-cause ranking that gives on-chain signals higher causal weight and
  demotes background pressure with `backgroundImpactMs`.
- Runnable context enrichment:
  - per-thread CPU load first;
  - process rollup only as secondary context;
  - same-CPU competitors;
  - core class;
  - other-core idle;
  - CPU frequency;
  - explicit affinity/cpuset/migration constraints;
  - Harmony/Donghu `sched_switch next_info` affinity/restricted fields.
- IO outputs:
  - `file_io_by_inode`;
  - `page_cache_by_inode`;
  - `storage_latency_by_layer`;
  - `io_pressure_summary`;
  - file/storage completion handoff for `completions`, `ret`, representative
    `example`, bytes/offset/len, and latency so single completions are not
    hidden by aggregate bytes or total latency;
  - text parser support for `key=value`, `key = value`, selected `key value`,
    and whitelisted `key:value`.
- Converter IO renderer coverage for Android FS/F2FS/SCSI events and round-trip
  tests, so converted traces keep IO fields instead of header-only rows.
- Binder IPC graph and binder wait summaries.
- Trace mark span parsing, frame/window discovery, frame timeline/flow, render
  pipeline summaries, and recipe/evidence-pack views.
- IRQ/softirq, CPU idle/frequency/frequency-limit, workqueue, block IO, and
  blocked-reason event parsing at least as same-window resource evidence.
- Typed observation handoff for root-cause rank, causal impacts, root evidence,
  state churn, IO, runnable context, thread CPU load, CPU constraints, and
  process CPU load.
- External-only runtime trace requests now preserve the runtime-artifact lane
  through the analyzer repair layer and exact-resolution contract: if the typed
  policy excludes current-source evidence, trace identifiers such as inode,
  device, entry name, timestamp, or trace-local file-like labels must not become
  current-repo source-inventory or exact-file obligations.
- New in this audit batch: typed `wakeup_chain` path and per-edge
  `wakeup_chain_edge` observations, plus summary fallback for compact path rows.
- New in this audit batch: deterministic `trace_query` runtime observations
  outrank pre-triage artifact summaries. Pre-triage rows become advisory
  supporting coverage when the precise tool result is present, so stale coarse
  caveats do not override line-backed window facts.
- New in this audit batch: analysis-only repo/runtime/artifact policies stay on
  the repo evidence pipeline even if the classifier also emits stray
  `needs_data_access`; true data operations still use the data lane.

## Gaps exposed by this trace

1. Wakeup-chain handoff needs to be first-class across all stages.

   The code can compute the chain, but answer quality can still depend on how a
   model paraphrases the chain. The structured path and edge rows added in this
   batch close the most immediate handoff gap. Remaining work is to make root
   cause bundles carry the chosen chain path, edge list, edge priorities, and
   per-node dominant states together in one compact object.

2. Chain-aware relevance should be explicit for every competing signal.

   This trace has substantial off-chain IO/system work in the same window
   (`wk:0/-20/0/6`, `sysevent_store`, `hmfs_txn-260-67`, render/audio/IRQ
   activity). Current ranking demotes background candidates, but outputs should
   explicitly label `on_chain`, `adjacent`, or `background`, with overlap and
   causal-edge evidence. This prevents unrelated long D-state or IO activity
   from becoming the primary root cause.

3. Fragmented IO/D-state needs episode-level aggregation.

   `ThreadPoolForeg-60555` shows repeated short D-state and IO-like waits rather
   than one single huge block. `state_churn` handles fragmented states, but IO
   root cause should also group `D + sched_blocked_reason + page-cache churn +
   block issue/complete` into an `io_burst_episode` with total duration, fragment
   count, max/p95 segment, top inode/device, and chain relevance.

4. Block IO to inode correlation is incomplete.

   `block_bio_remap` maps block device/sector back to source dev/sector, while
   page-cache rows expose dev/inode/offset. Current summaries report page cache
   by inode and block latency by layer, but do not reliably join them into
   `block_io_by_inode`. A generic nearest-neighbor join by thread, time, dev,
   remap source, offset/sector, and op would let the answer say which inode
   produced the storage latency when the trace carries enough information.

5. Rich trace marks are underused as domain context.

   The trace carries high-level spans such as lock contention, native async file
   operations, frame/render-service phases, present fence waits, binder transact
   labels, futex wait/wake, audio pipeline, and buffer queue operations. Current
   code parses spans, but many labels remain generic `trace_span` candidates.
   Needed: a producer-owned trace-mark taxonomy based on trace label structure,
   not on user intent or model prose, so spans can be summarized as lock, file
   async, render/fence, binder, futex, audio, or buffer categories.

6. IRQ/softirq pressure needs CPU-time and overlap attribution.

   The focused window contains thousands of IRQ entry/exit rows and hundreds of
   softirq rows. Current summaries are mostly counts/bursts. The useful signal is
   per-CPU active time and overlap with runnable waits or chain-node windows,
   plus top IRQ names/actions. This should feed runnable-context and root-cause
   ranking only when overlap is causal or strongly adjacent.

7. Workqueue activity should be paired and linked.

   Workqueue start/end rows and `worker_thread*` blocked reasons are visible.
   Current parsing does not pair work items into duration summaries or link work
   functions to wakeups/blocked reasons. Needed: `workqueue_activity` with
   paired duration, function/work id, executing thread, overlap, and chain
   relevance.

8. Compute/memory supply needs a unified Donghu summary.

   The trace has `cpu_frequency`, `cpu_idle`, and rich `clock_set_rate` signals
   for thermal, DDR, L3, l3cache, throughput, and temperature. Current code has
   CPU frequency and compute-supply pieces, but no coherent
   `supply_pressure_summary` that explains CPU/core availability together with
   memory/fabric/thermal rate changes.

9. Frame workflow should stitch app, render service, hardware, and present fence.

   The trace includes `Choreographer#onVsync`, render-service hardware phases,
   `Present Fence`, buffer queue spans, and jank/sync labels. Current frame
   helpers exist, but Donghu-specific render/fence labels should be verified and
   normalized into a single frame workflow view that can host scheduler stalls.

10. Native async file spans should bridge high-level operations to low-level IO.

    `Native async work queue/execute/complete, name:FileIO*` spans describe file
    operation intent above kernel IO. Current inode/block analysis does not pair
    these async spans or connect them to page-cache/block evidence. Needed:
    `async_file_work` pairing with low-level inode/storage overlap.

11. Non-IO uninterruptible/blocked reasons are diagnostic, not automatically root.

    Many `sched_blocked_reason iowait=0` rows are driver/device waits. They can
    explain system pressure, but should not outrank an on-chain D/IO or runnable
    dependency unless overlap and causal relevance are clear. Needed:
    `non_io_blocked_reason_summary` with caller family, thread, duration/count,
    and relevance labels.

12. The model needs a compact frame-root-cause bundle.

    Rich traces require many views today. This increases tool-call count and
    risks losing priority across handoff. A bounded `frame_root_cause_bundle`
    should return selected window, wakeup path/edges, ranked root causes,
    runnable context, IO episodes, top inode/storage, IRQ/workqueue/supply
    context, and trace-mark workflow in one compact, line-backed payload.

13. External-only runtime artifact contracts must not reopen current-source
    exact targets.

    Converted trace and low-leading evals showed the analyzer may put
    trace-local identifiers into model-call JSON fields intended for current
    source, especially `source_inventory_profile`. The generic repair must
    consume typed source policy and runtime-bundle state, not user-language
    keywords or model prose: when `current_source_mode=exclude` is anchored and
    the trace/log bundle is external-source, source inventory is ignored and
    exact-resolution is not built from artifact identifiers. This lets positive
    runtime conclusions close as `resolved` instead of forcing a false
    repository `absence`.

## Delivery tasks

Batch A: chain relevance and handoff

- [x] Emit compact `wakeup_chain path=...` in summaries.
- [x] Publish typed `wakeup_chain` path observations.
- [x] Publish typed `wakeup_chain_edge` observations with latency, priority
  relation, and inversion candidate fields.
- [x] Add observation-ledger fallback for compact path rows.
- [x] Demote pre-triage runtime observations to advisory/supporting coverage
  whenever deterministic `trace_query` observations for the same artifact are
  present.
- [x] Keep analysis-only artifact/external-tool trace requests on the repo
  evidence pipeline even when the classifier emits stray data-access fields.
- [x] Add `chain_relevance` to root-cause/supporting candidates where missing:
  `on_chain`, `adjacent`, `background`.
- [x] Add overlap fields for candidates: `overlap_ms`, `edge_count`,
  `nearest_chain_thread`, `nearest_chain_window`.

Batch B: fragmented IO episodes and inode/storage join

- [x] Preserve IO completion detail in tool summary, producer typed rows, and
  observation-ledger fallback: `completions`, `ret`, representative `example`,
  bytes/offset/len, and max/total latency now travel together.
- [x] Build `io_burst_episode` over D-state, blocked reason, page cache, block
  issue/complete, and storage latency.
- [x] Add `block_io_by_inode` or enrich `storage_latency_by_layer` with nearest
  inode/thread when the trace carries enough mapping evidence.
- [x] Keep off-chain IO demoted unless chain relevance is causal or adjacent.
- [x] Add tests for fragmented IO episode ranking without a single long block.

Batch C: interrupt, workqueue, and supply context

- [x] Pair IRQ/softirq entry/exit by CPU and compute active time.
- [x] Summarize interrupt overlap with runnable waits and chain-node windows.
- [x] Pair workqueue execute start/end and attach function/work id.
- [x] Add `supply_pressure_summary` combining CPU freq, idle availability,
  `clock_set_rate`, thermal, DDR/L3/l3cache, and memory throughput signals.

Batch D: trace-mark taxonomy and frame workflow

- [x] Introduce producer-owned trace-mark categories: frame/render/fence, lock,
  file async, binder transact, futex, audio, buffer queue.
- [x] Pair native async file work queue/execute/complete spans.
- [x] Normalize Donghu render-service/hardware/present-fence labels into the
  existing frame timeline/flow contracts.
- [x] Verify the same render/fence taxonomy against generic Harmony/OpenHarmony
  producer labels; Donghu label variants must be handled as producer-label
  variants, not as a separate customer-specific logic path.
- [x] Keep taxonomy based on trace producer labels and event structure, not on
  user intent or model-output prose.

Batch E: bundle and eval

- [x] Add `frame_root_cause_bundle` recipe/view or recipe profile.
- [x] Ensure prompt/tool description teaches the bundle and every new structured
  field without adding model-output keyword gates.
- [x] Extend JSON compat only for model-call input aliases. Output-only fields
  should not become required model inputs.
- [x] Keep external-only runtime-artifact analysis out of current-source
  source-inventory and exact-resolution gates when typed
  `external_observation_policy.current_source_mode=exclude` is anchored.
- [x] Persist attached log/trace runtime bundles into the analyzer
  `RequestModel` so downstream prompt, hint, contract, and result-rendering
  code consumes the same typed source-lane decision.
- [x] Add focused tests for source-inventory JSON repair and exact-resolution
  suppression on observation-only runtime artifacts.
- [x] Add low-leading eval/test cases for:
  - wakeup-chain path/edge handoff;
  - off-chain long D-state demotion;
  - fragmented on-chain IO episode;
  - IRQ pressure overlap;
  - native async file span to inode/block bridge.
  - external-only converted trace IO with no source-code tools and positive
    resolved closure.

## Validation notes

- Real-trace inventory was generated from the customer-provided systrace with
  read-only shell scans.
- Existing validation before this audit:
  - `go test ./internal/tracequery ./internal/tool ./internal/types -count=1`
  - `go test ./... -count=1`
- Focused validation during this audit:
  - `go test ./internal/repl -run 'TestApplyTurnPolicyGuards_DataRoute|TestTurnPolicyDispatch_ExternalObservationAnalysis' -count=1`
  - `go test ./internal/types -run 'DemotesPerfPreTriage|RuntimeQueryOutranksPreTriage|TraceQueryRootCauseRank' -count=1`
  - `go test ./internal/tool -run 'TypedObservations|RunnableContext' -count=1`
  - `go test ./internal/tracequery ./internal/tool ./internal/types -count=1`
- Eval audit:
  - `trace_query_wakeup_background_demotion` passed and correctly treated the
    off-chain long D-state as diagnostic, not primary root cause.
  - `trace_query_state_churn_window_stats` originally passed but carried a
    stale pre-triage truncation caveat. The root cause was observation
    precedence, not the trace parser. Pre-triage demotion was added so precise
    `trace_query` observations own the final runtime facts.
  - A rerun exposed a separate routing issue: the classifier emitted
    `operation=investigate/source=artifact` plus stray `needs_data_access`,
    which sent the turn to data workflow before `trace_query` could run. The
    route guard now keeps this typed analysis-only shape on the repo pipeline.
  - `trace_query_converted_inode_io_pressure` exposed an IO handoff gap: the
    tool computed `android_fs_datawrite` 4KB completion latency and f2fs/scsi
    layer latency, but the final answer could collapse the requested completion
    into aggregate bytes/total latency. The fix preserves completion detail in
    summary rows, producer typed observations, and fallback ledger parsing.
  - The same eval exposed a systemic contract gap: even with anchored
    `external_observation_policy.current_source_mode=exclude`, analyzer output
    could carry a stray `source_inventory_profile` for trace-local
    inode/device/entry fields. After invalid roles were repaired away, the
    surviving `file` role created a current-repo exact file-path obligation,
    causing repeated `emit_investigation_complete` rejects and a false
    `absence` final prefix. The generic fix is in the model-call JSON repair
    and exact-contract layers, not in model prose.
- Focused validation added for this gap:
  - `go test ./internal/tool ./internal/types -run 'DropsSourceInventoryForObservationOnlyRuntime|CurrentSourceLaneDecision|ExactResolution|SourceInventoryProfile' -count=1`
  - `go test ./internal/tool -run 'TestEmitAnalysisSchemaIncludesSourceInventoryProfile|DropsSourceInventoryForObservationOnlyRuntime' -count=1`
  - `trace_query_converted_inode_io_pressure` passed after the fix with
    `tool_repo_map=0`, `tool_read_file=0`, `tool_list_files=0`,
    `unavailable_tool_attempts=0`, and no false exact-absence prefix.
  - `trace_query_wakeup_background_demotion` passed after widening the eval
    wording guard to accept the semantically equivalent chain wording
    `依赖链` / `级联唤醒` while still requiring the full chain nodes and
    off-chain logger demotion.
  - `trace_query_state_churn_window_stats` passed as a regression guard for
    fragmented-state next-step guidance.
  - `make`
  - relevant eval batches for runnable context, core topology, state churn,
    wakeup causal runnable, and inode IO pressure.
- This audit batch now includes focused tests for the rich frame-root-cause
  bundle, chain-relevance demotion, aliased/camel-case JSON input repair,
  summary guidance, and typed observation rows at the real tool boundary.

## Batch A-E completion update (2026-06-13)

This batch closes the remaining Donghu/Harmony rich-trace gaps at the
`trace_query` system boundary rather than with case-specific prompt patches.

- Batch A: `root_cause_rank` and `critical_blocking_calls` now carry
  `chain_relevance`, `overlap_ms`, `edge_count`, `nearest_chain_thread`, and
  `nearest_chain_window`. Off-chain candidates without their own precise time
  range remain `background`; they are not upgraded to `adjacent` merely because
  they exist somewhere in the selected window. Runnable/D-state/block-inode
  candidates preserve their own first/last observed timestamps when available,
  while off-chain concrete threads still require structural chain membership
  rather than timestamp overlap alone to leave `background`.
- Batch B: `window_stats` now reports `io_burst_episodes` and
  `block_io_by_inode`, and `storage_latency_by_layer` keeps inode/name fields
  when the trace producer provides them. `frame_root_cause_bundle` reorders IO
  bursts by chain relevance so on-chain IO dependencies are read before
  background long D-state episodes.
- Batch C: `irq_activity`, `softirq_activity`, `workqueue_activity`, and
  `supply_pressure_summary` are computed from trace event structure. IRQ and
  workqueue rows pair start/end when available, while unpaired rows remain
  count/line evidence instead of failing the whole query.
- Batch D: trace spans carry producer-label categories/subcategories including
  frame/render, render fence, async file, blocking sync, binder, workqueue,
  audio, and buffer queue. Native async file spans are summarized as
  `async_file_work`.
- Batch E: `frame_root_cause_bundle` is a canonical view and jank recipe member.
  Model-call JSON repair covers the new view aliases
  `frame_bundle`/`frame_rootcause_bundle`/`frame_root_cause` and event-filter
  aliases for interrupt/block-inode phrasing. Output-only fields remain
  output-only and are not required model inputs.

Focused code validation added:

- `TestFrameRootCauseBundleCarriesRichTraceEvidenceAndChainRelevance`
  constructs a low-leading synthetic trace with wakeup chain, off-chain D-state,
  inode IO, IRQ, workqueue, DDR clock, and async file span evidence. It verifies
  that the system derives the on-chain IO dependency itself and demotes the
  off-chain D-state.
- `TestTraceQueryFrameRootCauseBundleAliasSummaryAndObservations` runs the real
  tool boundary with aliased/camel-case JSON and verifies summary guidance plus
  typed observation rows for root cause, IO burst, IRQ, workqueue, and async
  file work.
- Existing schema/prompt hygiene coverage now asserts that
  `frame_root_cause_bundle`, `chain_relevance`, `block_io_by_inode`,
  `io_burst_episodes`, `irq_activity`, `softirq_activity`,
  `workqueue_activity`, `supply_pressure_summary`, `trace_mark_categories`, and
  `async_file_work` are documented in the tool teaching surface.
- `TestTraceQueryViewTeachingsMatchToolSchemaEnum` and
  `TestTraceQueryViewTeachings_TableShape` keep the shared prompt teaching
  table aligned with the trace_query schema enum, so new views cannot be
  accepted by JSON repair while missing from prompt teaching.

Final validation for this batch:

- `go test ./internal/tracequery ./internal/tool`
- `go test ./internal/agent ./internal/skill ./internal/tool ./internal/tracequery`
- `go test ./...`
