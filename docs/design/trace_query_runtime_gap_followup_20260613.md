# Trace Query Runtime Gap Follow-up - 2026-06-13

## Scope

This document records the residual gaps found after running the real
`xxx_all.systrace` customer trace through the current system. The validation
question must be low-leading: ask for "卡顿原因" / "jank causes" and require
multi-level analysis, but do not bake in a negative hint such as "排除 isplogcat
D 状态".

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
"探索代码" style status text, even though the user explicitly requested trace
analysis only. Exploration eventually used `trace_query`, but perf-triage and
stage presentation still create a source-analysis feel and extra cost.

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

## Validation Question Policy

Future validation questions for `xxx_all.systrace` should be low-leading and
multi-level:

- Use "卡顿原因" instead of "卡顿主因".
- Ask for direct trigger, dependency-chain cause, scheduler/resource context,
  and auxiliary investigation directions.
- Do not include a negative hint like "排除 isplogcat D 状态".
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

### Batch 5: Real-trace validation

- Run the low-leading `xxx_all.systrace` validation question without mentioning
  isplogcat.
- Verify the answer reports multi-level causes:
  direct sleep/wakeup or binder-like trigger when grounded, dependency chain,
  runnable/sleep/D-state split by chain node, scheduler/resource background,
  and auxiliary-only rows.
- Verify no off-chain log/system thread is presented as primary when on-chain
  evidence exists.

