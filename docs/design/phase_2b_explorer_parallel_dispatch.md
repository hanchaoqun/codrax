# Phase 2B — Explorer parallel dispatch (design)

**Status**: BLOCKED on explorerEvaluator architecture refactor. NOT
ready to land in one session. Estimated 2-3 independent sessions.

**Baseline commit**: `86d8c84d` (Phase 2A wired
`pipeline_max_parallelism` config + reviewer gate; explorer side
gated to 1 = strict serial via the same knob).

**Forensic anchor**: 2026-05-17 audit during Phase 2 session.
Discovered AFTER user authorisation to ship 2B but BEFORE
implementing — see "Critical discovery" below. Mitigated by holding
2B and shipping only 2A.

## 1 What 2B is supposed to deliver

Wall-clock parallelism for the multi-sub_topic explorer fan-out that
E' Phase 1 already wired structurally. Today (post-E'-Phase-1):

- analyzer's `expandEvidenceNodes` emits N independent evidence
  sibling nodes (`{prefix}_t0` / `_t1` / `_tN`) for N sub-topics
- scheduler's E' helper trims the ready window to one evidence
  sibling per outer-loop iteration
- outcome: N **sequential** explorer dispatches, each focused on its
  own sub-topic

2B replaces the sequential loop with parallel goroutines so total
wall time approaches `max(t_i)` instead of `sum(t_i)`. The
goroutine count obeys `pipeline_max_parallelism` (Phase 2A):

- `=1` → keep the E' Phase 1 sequential loop (default fallback)
- `>=2` → spawn min(cap, N) goroutines
- `=0` → unlimited; spawn N goroutines

## 2 Critical discovery — explorerEvaluator is a singleton mutable struct

**File**: `internal/agent/explorer.go:38-200+`.

`explorerEvaluator` is constructed ONCE in `cmd/root.go` (when the
agent registry is set up) and shared across every `dispatchStage`
call. It holds ~80 mutable fields that are read AND written across
the lifetime of one `Execute()` call:

```
phase, broadenAttempts, preScannedFiles, allScoredFiles,
fileSymbols, searchResult, searchFingerprint,
multiGraphHandle, pendingSubRepos, analyzerKeywords,
exactAnchorFiles, declarativeAnchorFiles,
declarativeCandidateFiles, primaryEntitiesRegistrationShape,
requiredFileHints, irrelevantFilesSet,
investigationNotes, userQuestion, repoRoot,
preScannedPushCount, lastPreScannedUnreadCount,
grepRedirectedFiles, isEnumerationQuery, isOrientationQuery,
phase0ExtraRound, hasPrescanRepoMap,
structuredEvidence, flowFindings, ermRequirements,
cachedConcreteValues,
midLoopLastResultsLen, midLoopSerialStreak,
midLoopParallelInjected, midLoopSymbolRefInjected,
midLoopPostPrimaryInjected, midLoopBudgetExhaustedSent,
midLoopEvidenceRepairSent, midLoopEvidenceRepairResultsLen,
midLoopSurfaceTermReviewSent, midLoopClosureRepairSent,
midLoopClosureRepairResultsLen, midLoopIntentWindowSent,
midLoopRankerCoverageSent, midLoopAbsentRedirectSent,
midLoopExternalArtifactSent, midLoopExactAbsenceContextSent,
midLoopExactAbsenceSent, midLoopSchemaLevelHintSent,
midLoopAuthoritativeTier1Sent, midLoopEnumInjected,
midLoopOrientationFinalizeSent, midLoopNoEmitPushSent,
midLoopNoEmitEscalated, midLoopExecRedirectSent,
... (~40 more)
```

`BuildInitialInstruction` and `ParseOutput` directly write these
fields (no mutex). N concurrent `ag.Execute()` calls on the same
evaluator would:

- overwrite `phase` / `broadenAttempts` mid-iteration
- race on `investigationNotes` / `structuredEvidence` appends
- corrupt midLoop* one-shot flags so the per-dispatch nudge logic
  fires in the wrong dispatch
- pollute `ermRequirements` / `cachedConcreteValues` across topics

**This is the SAME pattern that bit the reverted method-E ship**:
`subExplorerEvaluator` (`internal/agent/sub_explorer.go:87-95`) holds
~6 mutable fields that get clobbered when SubAgentRuntime runs N
sub_explorers concurrently. The reverted commits (`d6ca6352`,
`37298341`) document the failure mode. Method-E's `sub_explorer`
problem and 2B's `explorer` problem are the SAME architectural issue
at different scales.

## 2.5 Production forensic confirmation (2026-05-17 09:09 run)

A first post-E'-Phase-1 production run on a 2-sub_topic question
generated explicit evidence that the singleton-evaluator state leak
fires under SEQUENTIAL dispatch too (independent of any future
parallel dispatch). Details in
`docs/design/post_phase2a_forensic_followups.md` §2.1.E.

Highlight: at D2 iter=0 the explorer LLM's opening `<think>` block
analyzes D1's failure pathology in detail despite a FRESH
~26k-token context window (no conversation carryover from D1's
~70k-token final iter). The analysis can only have arrived via the
shared `explorerEvaluator` fields (or via `BaseAgent.Execute`'s
retry-hint synthesis pulling from those fields) — exactly the
~80-field mutable singleton documented in §2 above.

This upgrades the singleton concern from "code-read-only worry" to
"verified production symptom". The Approach A refactor (per-Run
evaluator construction) now has two motivations of equal weight:

1. Original: enable safe parallel dispatch for Phase 2B Session C.
2. New: stop the cross-dispatch state bleed visible in sequential
   E'-Phase-1 dispatches today.

Session A's test plan should pin a `D2 iter=0 think-block
isolation` invariant: with a stub LLM that records its input
prompt, run two back-to-back `Execute()` calls on the same
Explorer; the second's prompt MUST NOT contain analysis-text
derived from the first's evaluator state.

## 3 Why race detector did not catch it

`go test -race ./internal/orchestrator/ ./internal/agent/`
passes today because no test exercises N concurrent
`ExplorerEvaluator.Execute` calls. The race detector reports
unsynchronised memory access on a struct field that is actually
read+written simultaneously — but every test runs sequentially. The
hazard is dormant until a parallel-dispatch path is wired up.

This is the same reason method-E shipped with `go test -race` green:
the sub_explorer race was dormant. The first time it bit us was the
forensic 08:14 run on a real two-sub_topic question.

**Lesson for future sessions**: a `-race` clean test suite does
NOT guarantee evaluator-level concurrency safety when evaluators
are singletons. Concurrent-safety must be proven by either
constructing an actual parallel test (spawn 2 Execute goroutines
against the same evaluator instance with stub LLM responses) OR by
architectural argument (per-Run evaluator construction).

## 4 The fix surface — explorerEvaluator architecture refactor

Three viable approaches, in order of soundness:

### 4.1 Approach A — per-Run evaluator construction (recommended)

Move evaluator construction from `NewExplorer(deps)` (once per
process) to `Explorer.Run()` (once per `Execute()` call).

**Required changes**:

- `Explorer` keeps only construction inputs (`deps *Dependencies`).
- `Explorer.Run()` (or `BaseAgent.Execute()` for the Explorer path)
  constructs a fresh `explorerEvaluator` whose Run-scoped fields
  start at zero values.
- Cache-scoped fields (`searchResult`, `searchFingerprint`,
  `fileSymbols`, `multiGraphHandle`, …) move out of the evaluator
  onto an `*ExplorerCache` owned by `Orchestrator` and looked up by
  key (e.g. `searchFingerprint`) when the evaluator wants the
  pre-scan reuse.
- All evaluator methods stay the same signature; only the
  construction site changes.

**Risk**: identifying which fields are Run-scoped vs. cache-scoped
requires reading every field's write site. The ~40 `midLoop*`
fields are clearly Run-scoped (per-dispatch one-shot flags). The
`searchResult` family is cache-scoped (T1.2 forensic anchor says it
deliberately reuses across redispatches within a Run). The
`investigationNotes` field looks Run-scoped from the field name but
its actual lifecycle needs verification.

**Estimated LOC**: 400-600 (refactor + tests). Cache lookups need
proper key invalidation when sub_topic / scope changes.

### 4.2 Approach B — evaluator factory + registry change

`agents.Registry` becomes a factory: every `Get(name)` returns a new
agent instance. Existing single-call sites continue to work; the
parallel-dispatch site spawns N goroutines, each calling `Get` to
get its own instance.

**Risk**: changes every agent registry consumer (analyzer, extractor,
finalizer, planner, coder, verifier all use the same registry).
Larger blast radius than 4.1.

**Estimated LOC**: 800+.

### 4.3 Approach C — mutex over evaluator fields

Add `sync.Mutex` to every evaluator field write. Holds mutex during
the entire `Execute()` call (which spans LLM rounds taking ~30s
each). Effectively serialises the explorer even when the scheduler
spawns N goroutines.

**Risk**: defeats the entire purpose of 2B. Not viable.

## 5 dispatchStage shared-state issues (separate axis)

Even with the evaluator refactor done, `dispatchStage` itself writes
to `o.busCtx` from inside the call:

| Write | Line | Risk under parallel dispatch |
|---|---|---|
| `o.busCtx.ActiveAgent = agentName` | ~6046 | All goroutines write "explorer"; values equal, race benign but vet-flagged |
| `o.busCtx.TaskState.LastError = ""` | ~6053 | Same as above (all clear to "") |
| `o.emitStageRetryAttempt = 0` | ~6075 | Orchestrator field; needs mutex |
| `o.busCtx.PipelineStage = StageExplore` | ~4069 (caller) | All goroutines write same value, benign race |
| `o.busCtx.TaskState.RetryHint = hint` (via `applyWindowHint`) | ~5587 | DIFFERENT hints per goroutine; clobbers each other |
| `o.busCtx.Mutable.ResetInvestigationComplete()` | ~4074 (caller) | One goroutine's reset wipes another's emit-complete signal |
| `o.applyStageOutput(output)` | ~6431 | EvidenceItems / FlowFindings / RepoFacts read-modify-write |

**The `RetryHint` write is the load-bearing problem** — each parallel
dispatch needs ITS OWN hint (a per-sub_topic objective), but
`applyWindowHint` writes to a shared bus field that `BuildAgentContext`
later reads.

### 5.1 Required refactor

- `dispatchStage` splits into:
  - `runStageAgent(stage, perDispatchInputs) → output` — pure;
    no `o.busCtx` writes.
  - `applyStageOutput(output)` — already exists; serialised by
    the orchestrator main goroutine.
- `perDispatchInputs` carries the (formerly bus-resident) hint,
  retry attempt counter, prior-conv visibility flag, etc.
- `BuildAgentContext` reads `perDispatchInputs` directly OR receives
  an already-populated agentCtx.

**Estimated LOC**: 400 + tests.

## 6 graphState.status concurrency

`scheduler.go` `graphState.status` is a `map[string]nodeStatus`
mutated by `markRunning` / `markDone` / `requeue`. Currently
unlocked because the scheduler is single-threaded. Parallel dispatch
needs a mutex (or atomic per-node status fields).

**Estimated LOC**: 50.

## 7 Recommended session breakdown

### Session A — evaluator refactor (foundational)

1. Read every `explorerEvaluator` field write site. Classify each as
   Run-scoped vs cache-scoped.
2. Implement Approach A: extract `ExplorerCache` for cache-scoped
   fields; move Run-scoped fields into per-`Execute()` evaluator
   construction.
3. Mirror the same refactor for `subExplorerEvaluator` (so the
   reverted method-E path is no longer dormant-broken). Optional
   bonus.
4. **Concurrent-safety test**: spawn 2 `Execute()` goroutines on the
   same Explorer instance with stub LLM responses; assert each
   returns isolated `EvidenceItems` matching its own input. Race
   detector must stay green.
5. No behavioral change at single-dispatch; all e2e tests pass.

### Session B — dispatchStage decoupling

1. Extract `runStageAgent` from `dispatchStage`. Keep
   `dispatchStage = runStageAgent + applyStageOutput` as the public
   single-dispatch entry point.
2. Move `RetryHint` from `o.busCtx.TaskState` to a `perDispatchInputs`
   struct passed into `runStageAgent`.
3. graphState mutex.
4. Re-verify single-sub_topic byte-equivalence.

### Session C — parallel dispatch + e2e

1. In `runReadSchedulerLoop`, when the trimmed window flag fires AND
   `effectiveParallelism(orchestratorMaxParallelism(), nSiblings) >=
   2`, spawn `errgroup` over the sibling evidence nodes.
2. Each goroutine calls `runStageAgent(StageExplore,
   perDispatchInputs)`; main goroutine `wg.Wait` then serially
   `applyStageOutput` each result.
3. Cancellation propagation: parent `ctx` cancel reaches every
   goroutine.
4. e2e tests: a 3-sub_topic IR with stub explorer agent that sleeps
   100ms per call — wall-clock assertion `< 3 * 100ms + epsilon`.
5. Forensic re-run: rerun the 08:14 case under
   `pipeline_max_parallelism: 2` and confirm wall-clock improvement.

## 8 Invariants to preserve across A→B→C

- `pipeline_max_parallelism = 1` MUST keep the E' Phase 1 sequential
  for-loop verbatim. The Phase 2A reviewer gate already pins this for
  the reviewer path; the explorer side mirrors the same gate.
- Single sub_topic questions MUST be byte-equivalent to current
  pre-2B behavior at default settings.
- Append order of evidence items / facts / tool results MUST match
  declaration order of the sub-topic nodes (NOT goroutine completion
  order) so EvidenceClosure cluster keys + downstream dedup stay
  stable.
- `EventTaskNodeStart` / `EventTaskNodeEnd` MUST fire per-node so
  /dag UI rows transition independently — already correct in E'
  Phase 1.

## 9 Open questions for Session A

- Is `searchResult` REALLY cache-scoped? `searchFingerprint` suggests
  yes, but a multi-sub_topic question may have different
  `analyzerKeywords` per topic — the cache key MUST include the
  topic identifier to avoid serving topic-A's prescan to topic-B.
- Does `BaseAgent.Execute` itself hold mutable state besides the
  evaluator pointer? Need to audit `internal/agent/agent.go`.
- Do the analyzer's `expandEvidenceNodes` siblings share any other
  field that becomes a race surface (e.g. `Inputs`/`Outputs` slots
  on `TaskNode`)?

## 10 Why this lives as a design doc, not a stub commit

A half-implemented 2B (e.g. dispatchStage decoupled but
evaluator still singleton) would PASS unit tests + race detector
green AND silently corrupt evidence pools on the first real
multi-sub_topic run. That is the exact failure mode method-E shipped
with. The fix sequence must be A → B → C; A is the load-bearing
foundation, and shipping any later step without A introduces a
dormant race.
