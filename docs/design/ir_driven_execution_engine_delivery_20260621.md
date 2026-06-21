# Codrax IR-Driven Execution Engine Delivery Ledger

Date: 2026-06-21  
Base branch: `main`  
Base HEAD: `8269ed6ed docs: IR-driven adaptive execution engine PRD (next-stage architecture)`

## Delivery Goal

把 `docs/design/ir_driven_execution_engine_prd_20260621.md` 从架构 PRD 推进为可交付实现：让读/写执行从分散的 scheduler 状态、handoff、reasoning projection 和 loopkernel shadow，逐步收敛成 typed、可审计、可恢复、低噪音的执行闭环。

本交付不引入新的并行 kernel，不用用户意图关键词或模型散文做硬路由。所有硬门只读 typed artifacts、schema/enum、精确布尔/整数、路径/解析结果；ranker、耗时、grep 命中数、模型解释只允许做软指导或 telemetry。

## Current State Audit

| Area | Current evidence | Status | Delivery implication |
| --- | --- | --- | --- |
| Write DAG legacy | Production no longer installs `BuildWriteTaskGraph`; legacy `write_scheduler.go` / `write_graph.go` were removed. Shared retry helpers moved to neutral controller/read retry helpers. | Completed M0a/M0b | Continue with TaskNode compatibility cleanup and future controller/loopkernel work; do not restore a linear write TaskGraph. |
| Read scheduler | `runReadSchedulerLoop` remains the real adaptive read engine; node status is still local `graphState`, not durable EvidenceClosure state. | Open | Fold node execution status into EvidenceClosure behind a thin accessor, with golden trace proving dispatch sequence equivalence. |
| TaskNode artifact slots | `TaskNode.Inputs`, `Outputs`, `ExitArtifacts` still exist with comments saying no runtime consumer reads/writes them. | Open | Add structural guard now; wire them in execution-tree artifact contracts in Phase 2 or delete them. |
| ReasoningGraph observer | `Dependencies.ReasoningObserver` and BaseAgent observation helpers exist; tests cover local tool observation. Production orchestration coverage is incomplete and ToolInvocation lacks full params/result replay identity. | Partial | Extend existing observer, not a new system: add invocation identity, params/result refs, construction wiring tests, and replayable audit projection. |
| Source-class universe | Source inventory now computes repo-wide source-class/language counts and absence gate consumes typed universe via `SourceInventoryExactAbsenceNeedsInventoryProof`. | Partial | Do not duplicate taxonomy. Project the same universe into the grown EvidenceClosure / loopkernel view so read and final report consume one authority. |
| Tool/result ingestion | Tool outputs are still re-read through scattered recomputation and stage-specific handoff surfaces. | Open | Add `EvidenceClosure.IngestRound` as a shadow reducer first, assert equality against legacy recompute, then cut over only after golden trace is stable. |
| Progress/replan authority | Existing downgrade convergence and repair directives are typed but spread across emit/precheck/finalize logic. | Open | Add `ProgressDelta` and a single typed replan/continue authority; first cut must preserve existing behavior. |
| Loopkernel | `LoopEvent`, reducer, authority, proof projection exist; `LoopRun` and `LoopBudget` are still passive data structs. | Open | Lift sourceinventory budget discipline into loopkernel, then make write controller consume `LoopRun.Advance` before read driver changes. |
| Stage-axis adaptivity | Read TaskGraph node axis is adaptive; stage axis remains mostly hardcoded, and `stageMapping` still collapses read node types to explore. | Open | Analyzer must emit stage nodes first; scheduler must not invent topology at runtime. |
| Prompt/schema hygiene | Several tools already emit typed repair/handoff, but unsupported-tool and schema repair loops still depend on scattered hints. | Open | Centralize supported JSON surface and accepted evidence carrier; tests must ensure hard logic does not inspect prompt prose. |

## Non-Negotiable Invariants

- `AnalysisIR` remains immutable plan data; analyzer remains sole writer.
- `EvidenceClosure` becomes the mutable execution-state authority for read mode.
- `loopkernel` is the replay/budget/authority substrate; no new graph framework.
- `sourceinventory.Budget` remains the model for bounded scan/materialization/deadline/cancel behavior.
- `ReasoningObserver` remains side-effect-only during the current dispatch.
- Read mode behavior must be guarded by golden-trace tests, not by claiming function text is untouched.
- Write controller remains the write-mode production path; old write DAG cannot stay as a misleading public architecture.
- No hard gate may parse user prose, model rationale, prompt text, final-answer narrative, or keyword tables.

## Batch Plan

### Batch D0: Delivery Ledger

Deliverables:
- Land this delivery ledger.
- Record exact current-state audit, task map, verification matrix, and progress ledger.
- Commit and push to `main`.

Validation:
- `git status --short --branch`

### Batch M0a: Retire Misleading Write DAG Install Path

Deliverables:
- Add a structural reachability test proving production write mode enters `runWriteControllerWorkflow` and does not use `runTaskGraph`/`runWriteSchedulerLoop`.
- Prove the write TaskGraph installed in `Run()` has no controller consumer.
- Remove the `BuildWriteTaskGraph` install from production `Run()` after proof.
- Replace write analysis-ready/status-card data with controller-native typed workflow rows if the UI still needs a node surface.

Tests:
- Focused orchestrator mode/route tests.
- Existing write controller tests.
- L2 `write_enabled:false` refusal tests.

### Batch M0b: Delete Legacy Write Scheduler Surface

Deliverables:
- Remove or internalize `runWriteSchedulerLoop`, `BuildWriteTaskGraph`, `IsWriteGraph` write dispatch, and stale comments that describe write as linear plan/apply/verify DAG.
- Migrate `write_scheduler_test.go`, `write_graph_test.go`, and write retry-cycle tests to controller-first semantics or delete tests whose subject is removed.
- Keep read `runTaskGraph` dispatch purely read-oriented.

Tests:
- `go test ./internal/orchestrator -run 'Write|Mode|Controller|TaskGraph'`
- `go test ./internal/types -run WriteWorkflowEngine`

### Batch M0c: TaskNode Execution Slot Guard

Deliverables:
- Add a structural test that fails if `Inputs`, `Outputs`, or `ExitArtifacts` get runtime consumers before Phase 2 wiring.
- Add a code comment pointing to this ledger and PRD M2.5.

Tests:
- `go test ./internal/types -run TaskNode`
- `go test ./internal/orchestrator -run TaskNode`

### Batch M1a: Production ToolInvocation / ReasoningGraph Wiring

Deliverables:
- Extend existing ReasoningGraph observation payload with invocation identity and params/result refs, without changing model-visible ToolResult bytes.
- Wire a real observer through orchestrator agent construction for read/write stages and subagent runtime.
- Add replay/audit helper that can locate the exact tool invocation, normalized params ref, result ref, success/failure, elapsed telemetry, and violation kind.

Tests:
- `TestToolInvocationStampedInProduction`
- `TestToolInvocationObserverSideEffectFree`
- `TestToolInvocationReplayable`
- Existing `internal/agent/reasoning_observer_test.go`

### Batch M1b: EvidenceClosure NodeExecStatus Shadow

Deliverables:
- Add typed `NodeExecStatus` in `types`.
- Back `graphState` node status with EvidenceClosure through thin accessors.
- Keep all scheduler call sites semantically equivalent.

Tests:
- `TestNodeStatusFoldDropIn`
- Read golden trace.
- `TestRunMode_ReadByteIdentical`

### Batch M1c: IngestRound Shadow Reducer

Deliverables:
- Add `EvidenceClosure.IngestRound(results []ToolResult, repoRoot string)` using only `types`-level inputs.
- Run it in parallel shadow beside legacy recompute.
- Assert per-round read/evidence deltas equal in focused tests; do not cut over scattered legacy sites yet.

Tests:
- `TestIngestRoundShadowEqualsLegacyRecompute`
- `TestNoTypesImportAgent`
- Read golden trace.

### Batch M1d: ProgressDelta / Replan Authority

Deliverables:
- Add typed `ProgressDelta` and `ShouldReplan`.
- Fold existing downgrade convergence behavior into the new authority while preserving current observable outcome.
- Expose the reason code to status cards and repair handoff.

Tests:
- `TestProgressDeltaConvergesLikeDowngradeFingerprint`
- Prompt hygiene tests proving no prose/keyword hard route.

### Batch M1e: SourceClassUniverse Projection

Deliverables:
- Project existing source-class/language census into EvidenceClosure and loopkernel projection.
- Make absence-close/final report read the same typed universe authority already used by source inventory.
- Avoid adding a fifth taxonomy; consume `SourcePathRole`, `SourceScope`, and existing source inventory counts.

Tests:
- Existing `source_class_universe_absence_test.go`
- `TestAbsenceCloseReadsRepoWideUniverse`
- Cross-language cases for supported languages, including JS/TS, Ruby, Java/Kotlin, Go, C/C++, Cangjie, ArkTS, config/workflow, fixtures, vendor, thirdparty, generated.

### Batch M1f: Read Failure Memory Soft Injection

Deliverables:
- Persist recurring read-mode repair classes using existing failure taxonomy store shape.
- Inject into future runs only as soft guidance, never hard gate.

Tests:
- `TestReadFailureMemorySoftOnly`
- Prompt hygiene tests.

### Batch M2a: Analyzer-Authored Stage Nodes

Deliverables:
- Update deterministic TaskGraph builder so Extract, Finalize, and optional AnalyzeRefine are real typed nodes with EntryConditions.
- Preserve analyzer-sole-writer boundary.

Tests:
- `TestAnalyzerEmitsStageNodes`
- `TestAnalyzerSoleWriterPreserved`
- Read golden trace.

### Batch M2b: Stage Mapping And Partial Re-Execution

Deliverables:
- Generalize `stageMapping` so read stage nodes map to their own stage instead of being folded into explore.
- Promote `requeueToStage` to generic stage transition on typed criteria.

Tests:
- `TestStageMappingFirstClassExtractSkip`
- `TestStageAxisDefaultEqualsStraightLine`
- `TestPartialReExecToAnyStage`

### Batch M2c: Execution Tree And Artifact Contracts

Deliverables:
- Add lightweight `BranchPoint` and sibling backtrack using existing validation-feedback edges.
- Wire `Inputs`/`Outputs`/`ExitArtifacts` as artifact contracts, or delete them if no consumer remains justified.

Tests:
- `TestExecutionTreeBacktrackNotDAG`
- `TestTaskNodeExecSlotsWiredOrDeleted`

### Batch M2d: Opt-In Dynamic Expansion And Loopkernel Shadow Match

Deliverables:
- Implement analyzer-pre-authored opt-in nodes gated by precise booleans, such as incomplete source-class universe.
- Add loopkernel shadow action comparison against current imperative scheduler choices.

Tests:
- `TestStageExpansionOptInPreciseBoolean`
- `TestLoopkernelShadowMatchesImperative`

### Batch M3a: LoopBudget Enforcement

Deliverables:
- Lift sourceinventory budget discipline into `LoopBudget`: unit spend, deadline, cancellation, interrupted reason.
- Keep noisy elapsed telemetry out of hard gates.

Tests:
- `TestLoopBudgetEnforcesDeadlineLikeSourceInventory`
- Source inventory budget regression tests.

### Batch M3b: LoopRun.Advance Write-Side Cutover

Deliverables:
- Implement `LoopRun.Advance`.
- Make write controller append events and consume loopkernel budget/authority for next action.
- Keep read path projection-only in this batch.

Tests:
- `TestLoopRunDrivesWriteController`
- `TestReadPathLoopkernelProjectionOnly`
- Write controller resume/approval/verify tests.

### Batch M3c: Semantic Routing And Shared Proof Authority

Deliverables:
- Map `LoopRecommendedAction` to ToolSuggestions/tool surfaces using only typed authority state.
- Ensure read/write both use `MergeProofCoverageAuthority` as the single proof coverage arbiter.

Tests:
- `TestSemanticRoutingPreciseSignalsOnly`
- `TestReadWriteShareLoopkernelSubstrate`
- `TestSingleProofAuthority`
- Full read golden trace and write controller regression suite.

## Verification Matrix

Minimum per batch:
- Focused package tests for touched packages.
- `go test ./internal/types ./internal/loopkernel ./internal/reasoninggraph` when touching shared typed artifacts.
- `go test ./internal/agent` when touching tool observation, schema repair, or prompt/tool surfaces.
- `go test ./internal/orchestrator` when touching scheduler/controller/dispatch.

Commercial hardening before declaring complete:
- `go test ./...`
- `make test`
- Read golden trace for stable repo-analysis questions.
- Write Auto Pilot smoke: low-risk single-file change, multi-batch repair, high-risk approval pause, critical deny, verify-unavailable downgrade.
- SWE-bench adapter smoke: predictions are generated, official harness consumes them, typed final report explains local/harness/manual proof levels.
- Cross-language source inventory absence tests across all supported language families, not Python-only.

## Progress Ledger

| Batch | Status | Evidence |
| --- | --- | --- |
| D0 Delivery Ledger | completed | This document added on 2026-06-21; current-state audit and batch ledger recorded. |
| M0a Write DAG install retirement | completed | Production write mode no longer installs `BuildWriteTaskGraph` or emits legacy write TaskGraph rows; `TestMode_WriteControllerDoesNotInstallLegacyWriteTaskGraph` pins controller-first behavior. |
| M0b Legacy write scheduler deletion | completed | Removed `write_scheduler.go`, `write_graph.go`, and stale scheduler tests; retained shared retry helpers in `write_retry_helpers.go`; updated controller/read tests and architecture docs. |
| M0c TaskNode slot guard | completed | Added `TestTaskNodeExecSlotsHaveNoRuntimeConsumersToday`; TaskNode artifact slots remain serialized/compiler/gate-only until Phase 2 artifact contracts wire or delete them. |
| M1a ToolInvocation production wiring | completed | Added typed `invocation_id` / `params_ref` / `result_ref` through ReasoningGraph payload, reducer, replay, and audit views; production `cmd/root.go` now installs a shared observer into agent deps and orchestrator/subagent runtime; `FindToolInvocation` locates replayed calls. Verified with `go test ./cmd`, `go test ./internal/agent`, `go test ./internal/orchestrator`, `go test ./internal/reasoninggraph ./internal/types`. |
| M1b EvidenceClosure node status | completed | Added typed `NodeExecStatus` and EvidenceClosure accessors/clone/merge/reset coverage; `graphState` status mutations now dual-write through `setStatus` into the run closure while preserving scheduler map semantics. Verified with `go test ./internal/types` and `go test ./internal/orchestrator`. |
| M1c IngestRound shadow reducer | completed | Moved read-file coverage parsing into `types`, kept `tool/ground` as a compatibility wrapper, added `EvidenceRoundDelta` / `EvidenceClosure.IngestRound`, and wired `applyStageOutput` to run the reducer on a cloned closure only. Verified with `go test ./internal/types`, `go test ./internal/tool/ground`, and `go test ./internal/orchestrator`. |
| M1d ProgressDelta authority | completed | Added typed `ProgressDelta`, `ProgressDecision`, and `ShouldReplan`; downgrade low-delta convergence now records a typed progress delta through `EvidenceClosure.RecordDowngradeProgressDelta`, preserves prior threshold behavior, and carries `progress_delta_converged` into completion caveats. Hygiene test pins `ProgressDelta` has no prose routing fields. Verified with focused `go test ./internal/types` and `go test ./internal/tool`. |
| M1e SourceClassUniverse projection | completed | EvidenceClosure now mirrors typed `SourceInventoryObservation` / `SourceClasses`; MutableState source-inventory setters, TurnA handoff, fork merge, reset, and `SourceInventoryObservationFromMutable` all consume the same closure-backed authority. Loopkernel read proof snapshots now carry `SourceClasses`, `SourceClassUniverseComplete`, source-class evidence refs, and `ProofSnapshotFromReadMutable` / `ReadProofGuidanceFromMutable`; read retry checkpoints and parallel explore convergence consume the mutable view. Verified with source-inventory absence/convergence tests, `go test ./internal/tool/repomap`, and package regression for types/loopkernel/orchestrator/tool. |
| M1f Read failure memory | completed | Added deterministic typed read failure memory as a fallback into the existing `AnswerTaxonomyStore`; it persists retry/repair classes only as analyzer pitfall soft guidance, skips raw retry prose for routing, avoids duplicate fallback when the LLM answer reviewer emits a pattern, and creates no current-run hard repairs or reviewer violations. Verified with `go test ./internal/orchestrator -run 'TestReadFailureMemory|TestAnswerTaxonomy|TestReviewerRoundTrip|TestAnswerReviewer'`, `go test ./internal/agent -run 'TestPrependAnswerPitfalls|TestAnalyzer'`, package regression, and `go test ./...`. |
| M2a Stage nodes | not_started | Builder still lacks first-class stage nodes. |
| M2b Stage mapping/re-exec | not_started | Stage axis still mostly command-style. |
| M2c Execution tree/artifacts | not_started | No BranchPoint/artifact contract consumer. |
| M2d Dynamic expansion/loopkernel shadow | not_started | Loopkernel not yet shadow-driving scheduler decisions. |
| M3a LoopBudget | not_started | LoopBudget passive. |
| M3b LoopRun.Advance | not_started | LoopRun passive; write controller not cut over. |
| M3c Semantic routing/shared proof | not_started | Tool routing not yet authority-driven end-to-end. |
