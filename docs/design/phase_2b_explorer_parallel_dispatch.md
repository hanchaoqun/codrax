# Phase 2B — Explorer parallel dispatch

**Status**: IMPLEMENTED in bounded batches.

**Updated**: 2026-05-17.

## Goal

Improve read-mode exploration wall clock for analyzer-emitted
multi-sub_topic DAGs. When the task graph contains independent
`NodeEvidence` siblings, the scheduler may dispatch those focused
explorer windows concurrently instead of running them one by one.

The design is graph-driven and language-agnostic. It does not inspect
the user's raw question text, does not parse model prose, and does not
branch on programming-language keywords. The only hard scheduling
signals are typed runtime facts:

- `TaskNode.Type == NodeEvidence`
- DAG ready-window membership and node IDs
- `PipelineSettings.MaxParallelism`
- `RequestModel.SubTopics` only for existing iteration-budget scaling

This preserves the project red line: precise structural signals drive
hard gates; noisy text and heuristic matches stay advisory only.

## Prior baseline

E' Phase 1 already stopped merging all sub-topic evidence siblings into
one huge explorer prompt. The scheduler detects a ready window with
multiple `NodeEvidence` entries and trims it to the first evidence node,
so the outer loop dispatches one focused explorer window per sub-topic.

That fixed focus, but it is still sequential:

```text
evidence_t0 → evidence_t1 → evidence_t2
```

Phase 2B turns this into capped fan-out:

```text
evidence_t0 ┐
evidence_t1 ├─ capped by pipeline_max_parallelism
evidence_t2 ┘
```

## Configuration contract

The existing code already has the canonical cap:

- runtime yaml key: `pipeline_max_parallelism`
- internal field: `PipelineSettings.MaxParallelism`
- helper: `effectiveParallelism(configured, candidateCount)`
- hard ceiling: `MaxPipelineParallelismCeil == 16`

Phase 2B makes that user-visible in `codrax.yaml.example` and seeds
the production default to `DefaultPipelineMaxParallelism == 2`.

Semantics:

- `pipeline_max_parallelism: 1` forces strict serial execution.
- `pipeline_max_parallelism: 2` is the production default.
- `pipeline_max_parallelism: 0` means unlimited for each fan-out surface.
- values above 16 are clamped by the orchestrator.

The same cap is shared by explorer sibling dispatch, post-emit reviewer
fan-out, sub-agent runtime fan-out, and future orchestrator-owned
parallel surfaces. There must not be a second, explorer-only knob.

## Architecture

### 1. Evaluator lifecycle

`explorerEvaluator` is mutable and stateful. It tracks phase, search
cache handles, notes, evidence, mid-loop one-shot flags, ERM state,
exact-resolution fields, and concrete-value caches. Running two
`Execute()` calls on the same evaluator would corrupt both dispatches.

Phase 2B replaces the singleton evaluator with an `ExplorerAgent`
wrapper:

- the wrapper owns stable construction inputs and an optional shared
  search-result cache;
- each dispatch gets an evaluator keyed by
  `TraceID + ExploreDispatchKey`;
- re-dispatch of the same node in the same run reuses that node's
  evaluator, preserving legitimate explore self-loop state;
- different sibling evidence nodes get different evaluators, so
  parallel dispatches do not share mutable ReAct state.

The search-result cache remains keyed by the existing deterministic
`keywordSearchFingerprint`. It is a performance cache only. It must
never decide control flow.

### 2. Per-dispatch Mutable isolation

`MutableState` is lock-protected but semantically shared. A parallel
top-level explorer dispatch cannot share it because these fields are
per-dispatch or per-window:

- dispatch tool-result buffer
- investigation-complete latch
- explore budget counters
- emitted evidence tail used by `emit_evidence` and
  `emit_investigation_complete`
- Turn A handoff snapshot
- EvidenceClosure read-set and repair queues

Parallel explorer workers therefore run with a forked `MutableState`.
The fork copies read-only run state and the prior accumulated evidence
surface. Workers write to their own fork. After all workers return,
the scheduler merges each fork back into the parent in task-graph
declaration order, not goroutine completion order.

Merge rules:

- tool results and facts append;
- evidence, chains, symbols, and flow findings use existing stable
  merge semantics at `applyStageOutput`;
- Mutable emitted evidence dedupes by `StableEvidenceID`;
- EvidenceClosure read coverage is unioned;
- repair directives dedupe through the existing repair key;
- event streams such as violations, fingerprints, and
  symbol-emission rejection counters merge only the fork-created tail,
  so a worker never re-appends the parent baseline;
- Turn A artifacts merge from the fork-created tail: union read files,
  append only new notes/tool results/flow findings, merge evidence, and
  keep the maximum terminal-evidence count.

### 3. Scheduler fan-out

Only a ready explore window with at least two `NodeEvidence` entries is
eligible. Non-evidence companions retain the E' Phase 1 behavior:
companions stay with the first sub-window, and later sibling windows
stay focused on their evidence node.

Execution path:

1. Build the ready window from the typed graph.
2. Split it into declaration-ordered focused sub-windows.
3. Compute `effectiveParallelism(orchestratorMaxParallelism(), len(subWindows))`.
4. If the result is `1`, use the existing serial path.
5. Otherwise run a bounded worker pool.
6. Apply outputs and forked mutable deltas in declaration order.
7. Mark node status and emit node events per sub-window.

All hard decisions are based on typed node/window data. Prompt text is
rendered after the sub-window is chosen; prompt content never decides
whether parallelism is allowed.

### 4. Commercial safety invariants

- `pipeline_max_parallelism: 1` keeps explorer dispatch serial.
- Single-sub_topic behavior is unchanged.
- Evidence/fact/ToolResult application order is stable by DAG
  declaration order.
- Parallel workers do not write to parent `busCtx.Mutable`.
- Parent `busCtx.TaskState.RetryHint` is not used as a shared mutable
  mailbox between sibling dispatches; each worker receives its rendered
  hint through its forked bus context.
- Cancellation still flows through the same `BusContext.Context()`.
- The solution is language-agnostic. Repository language support stays
  in repomap, graph, tool, and evidence layers.

## Delivered batches

### Batch A — foundation

- Added `TraceID` and `ExploreDispatchKey` to `AgentContext`.
- Converted `NewExplorerAgent` to a keyed evaluator wrapper.
- Added fork/merge helpers for `MutableState` and `EvidenceClosure`.
- Exported `pipeline_max_parallelism` in `codrax.yaml.example` and seeded
  the production default.

### Batch B — scheduler fan-out

- Added `splitExploreWindowForDispatch`.
- Added a bounded worker-pool runner for focused explorer windows.
- Kept the existing serial path when cap resolves to `1`.
- Merged outputs/forks in declaration order.

### Batch C — tests

- Pinned `pipeline_max_parallelism: 1` serial behavior.
- Pinned configured cap behavior for multi-sub_topic explorer dispatch.
- Pinned closure fork merge so baseline event streams are not duplicated
  by the number of parallel workers.
- Pinned Turn A fork merge so sibling deltas accumulate without
  duplicating the parent handoff snapshot.
- Ran targeted orchestrator, agent, types, and race-sensitive tests plus
  `go test ./...`.

### Batch D — convergence safety for exhaustive typed handoffs

- Parallel explorer no longer cancels sibling windows after the first
  converged fork for typed set-valued questions: multi-subtopic,
  cross-component, relation-member, category-enumeration, and enumerate
  requests wait for all focused explorer forks and merge their typed handoffs
  deterministically.
- `MutableState.MergeExploreFork` now merges stable aggregate facts instead of
  replacing them, so a later sibling cannot erase an earlier complete
  `member_set` / count handoff.
- Aggregate merge is still typed and bucketed: compatible complete member
  carriers union by structured kind/label/role/unit/dimensions; distinct
  buckets remain separate. This avoids single-fork data loss without using
  user/model prose keywords as a dispatch decision.
- Follow-up guard after eval: typed exclusion/export-scope filtering now runs
  before aggregate handoff reaches finalizer, so a noisy sibling cannot widen a
  public/exported member set with private or variable symbols.

Verification:

- `go test ./internal/orchestrator -run TestDispatchExploreWindowsParallel`
- `go test ./internal/types -run TestMergeAnswerAggregateFacts`
- `bash eval/run.sh eval/cases/qf_multi_member_set_count_caveat.case 1`
  - `eval/results/qf_multi_member_set_count_caveat-20260519-154652`
  - PASS; `explorer_iters=5`, `finalizer_iters=1`, no repair/rewrite lines.

### Batch E — operator-visible lane identity and retry hygiene

- Render events now carry `DispatchKind` as render-only metadata. Parallel
  worker lanes still show `探索 · 第 X 路 · 第 N 轮`; serial post-merge
  reconcile/probe/validation windows show explicit aggregate labels such as
  `探索 · 汇总 · 第 N 轮` instead of dropping back to an unqualified
  `探索 · 第 N 轮`.
- The label is intentionally presentation-only. It is copied from typed
  `TaskNodeType` into `AgentContext`/render events and must not drive
  evaluator or scheduler behavior.
- Prompt context for post-parallel stages now filters same-stage/current-agent
  reports and scopes retry hints to their owning stage. This prevents a
  parallel worker's retry directive from becoming the next analyzer/extractor
  instruction, and avoids duplicate `Prior request analysis result` sections.
- Follow-up verification target:
  - `qf_logic_view_read_pipeline`: early parallel lanes must keep
    `第 X 路`; later serial reconcile must show `汇总`.
  - Prompt dumps must not contain `### [explore / analyzer]` style internal
    titles or duplicate prior-analysis sections for one recipient.

## Out of scope

This phase does not add language-specific dispatch policy, does not
infer independence from user/model keywords, and does not change the
analyzer's sub-topic compiler. If future analyzer templates add explicit
edges between evidence siblings, the scheduler should honor those edges
through the existing ready-window calculation rather than adding text
heuristics.
