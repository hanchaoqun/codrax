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
| Read scheduler | `runReadSchedulerLoop` remains the real adaptive read engine. Node execution status is now mirrored through typed EvidenceClosure accessors while preserving scheduler map semantics. | Completed M1b | Continue to guard dispatch equivalence through read scheduler tests and commercial hardening runs. |
| TaskNode artifact slots | `TaskNode.Inputs` / `Outputs` exist as immutable analyzer-authored declarations; `ExitArtifacts` was removed because runtime artifact IDs do not belong inside immutable AnalysisIR. | Completed M2c | Static contracts are exposed through `TaskArtifactContract`; runtime artifacts stay on EvidenceClosure/reasoning graph surfaces. |
| ReasoningGraph observer | `Dependencies.ReasoningObserver` and BaseAgent observation helpers now carry invocation identity, params/result refs, production wiring, and replay lookup. | Completed M1a | Commercial hardening should verify audit projection remains side-effect-only. |
| Source-class universe | Source inventory computes repo-wide source-class/language counts, absence gates consume typed universe, EvidenceClosure mirrors the observation, and loopkernel read proof snapshots carry the same source-class authority. | Completed M1e/M2d-B | Keep cross-language source inventory regression broad; do not add parallel taxonomies. |
| Tool/result ingestion | `EvidenceClosure.IngestRound` exists as a shadow reducer over `types.ToolResult`, and `applyStageOutput` runs it on a cloned closure so production state is not mutated. | Completed M1c shadow | A future cutover from scattered recomputation to the reducer is separate work and must be gated by golden trace equivalence. |
| Progress/replan authority | `ProgressDelta`, `ProgressDecision`, `ShouldReplan`, and `progress_replan_required` now provide typed low-delta/refine signals through EvidenceClosure and criterion.Env. | Completed M1d/M2d-C | Future analyzer re-entry or IR rewrite must define a new immutable rewrite authority before implementation. |
| Loopkernel | `LoopBudget`, `LoopRun.Advance`, write projection, semantic routing, shared proof authority, and write repair-budget cutover are implemented. | Completed M3a-M3d | Read remains projection-first; any additional read driver cutover needs a new batch and golden trace proof. |
| Stage-axis adaptivity | Analyzer/compiler now emits stage nodes, `stageMapping` maps extract, extract readiness is typed, and optional dynamic nodes are analyzer-pre-authored only. | Completed M2a-M2d | Scheduler must continue reading topology rather than inventing runtime nodes. |
| Prompt/schema hygiene | Hard gates introduced by this PRD consume typed criteria, enums, booleans, paths, and structured artifacts. Existing prompt/schema repair surfaces still need commercial hardening audit to ensure no scattered prose/keyword route regresses. | Needs M4c audit | Add an explicit M4 audit before declaring the PRD commercially complete. |
| Noise/perf maintainability | PRD requested LOC ratchets for `evidence_closure.go <= 2774` and `scheduler.go <= 945`. M4a split concern helpers into subfiles and added a structural ratchet test; current hot-file counts are 2638 and 799 lines. | Completed M4a | Future work must keep the ratchet test green or update this ledger before changing the budget. |

## Non-Negotiable Invariants

- `AnalysisIR` remains immutable plan data; analyzer remains sole writer.
- `EvidenceClosure` becomes the mutable execution-state authority for read mode.
- `loopkernel` is the replay/budget/authority substrate; no new graph framework.
- `sourceinventory.Budget` remains the model for bounded scan/materialization/deadline/cancel behavior.
- `ReasoningObserver` remains side-effect-only during the current dispatch.
- Read mode behavior must be guarded by golden-trace tests, not by claiming function text is untouched.
- Write controller remains the write-mode production path; old write DAG cannot stay as a misleading public architecture.
- No hard gate may parse user prose, model rationale, prompt text, final-answer narrative, or keyword tables.

## Task Intake Discipline

任何实现过程中识别出的新增系统任务、后续批次或风险项，必须先写入本文档的 Batch Plan 和 Progress Ledger，再继续编码或测试。聊天记录、临时状态更新和 commit message 不能作为唯一任务来源。

新增任务落盘要求：
- 记录任务归属批次、目标、硬约束和测试面。
- 标明是否改变当前稳定路径；第一批实现必须优先选择 shadow 或行为保持形态。
- 明确硬门只消费 typed artifacts、schema/enum、精确布尔/整数或路径/解析结果；不能把用户意图关键词、模型散文、prompt/hint 文本、耗时/ranker/grep count 等噪音信号升级为硬逻辑。

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
- Add a structural test that fails if `Inputs` or `Outputs` get runtime consumers before explicit contract wiring.
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
- Update deterministic TaskGraph builder so Extract and Finalize are real typed nodes with EntryConditions.
- Use a behavior-preserving typed readiness condition for the first extract node cutover; do not hard-gate on `has_enough_facts` or other signals that are not guaranteed on all stable read paths.
- Preserve analyzer-sole-writer boundary.

Tests:
- `TestAnalyzerEmitsStageNodes`
- `TestAnalyzerSoleWriterPreserved`
- Read golden trace.

### Batch M2b: Stage Mapping And Partial Re-Execution

Deliverables:
- Generalize `stageMapping` so read stage nodes map to their own stage instead of being folded into explore.
- Promote `requeueToStage` to generic stage transition on typed criteria.
- Add typed extract readiness generalization as the next step after M2a: replace the initial behavior-preserving extract condition with precise, scheduler-readable criteria derived from accepted evidence / proof state, while keeping final-answer contract authority unchanged.
- Scope boundary: M2b does not re-enter analyzer or replace/extend AnalysisIR at runtime. Optional AnalyzeRefine is moved to M2d, where analyzer-pre-authored opt-in nodes and loopkernel shadow comparison can guard the IR rewrite boundary.

Tests:
- `TestStageMappingFirstClassExtractSkip`
- `TestStageAxisDefaultEqualsStraightLine`
- `TestPartialReExecToAnyStage`
- `TestExtractReadinessDoesNotDependOnNoisySignals`

M2b-A scoped implementation:
- Add a registered typed criterion `extract_input_ready`.
- Evaluate extract stage node `EntryConditions` immediately before `StageExtract` dispatch.
- If `extract_input_ready` is false, mark extract skipped/done and proceed to finalize; this is a typed stage-skip, not a finalization hard gate.
- `extract_input_ready` may only read structured evidence/chains/aggregate facts/typed external observation carriers. It must not inspect user prose, model rationale, prompt text, raw tool summaries, elapsed time, ranker scores, or grep counts.
- Keep optional AnalyzeRefine out of M2b. It belongs to M2d because it changes the analyzer/IR ownership boundary rather than only stage transition readiness.

### Batch M2c: Execution Tree And Artifact Contracts

Deliverables:
- Add lightweight `BranchPoint` and sibling backtrack using existing validation-feedback edges.
- Keep `Inputs`/`Outputs` as immutable analyzer-authored artifact contract declarations.
- Delete `ExitArtifacts` from `TaskNode`; runtime-produced artifact IDs must not be written into immutable `AnalysisIR`. A future runtime ledger must live beside `EvidenceClosure` / reasoning graph, not inside `TaskNode`.

Tests:
- `TestExecutionTreeBacktrackNotDAG`
- `TestTaskNodeExecSlotsWiredOrDeleted`

M2c-A scoped implementation:
- Add typed `TaskArtifactContract` projection over `TaskNode.Inputs` / `TaskNode.Outputs`.
- Add typed `BranchPoint` / `ValidationFeedbackTargets` helpers derived from `TaskGraph.Edges`.
- Make scheduler validation-feedback requeue consume the shared typed helper instead of scanning edges locally.
- Remove `ExitArtifacts` field, sample fixture usage, and gate checks; update M0c guard so any future runtime artifact ledger must be explicit, not hidden in `TaskNode`.
- No prompt changes and no behavior change to dispatch order; this is a structural authority extraction.

### Batch M2d: Opt-In Dynamic Expansion And Loopkernel Shadow Match

Deliverables:
- Implement analyzer-pre-authored opt-in nodes gated by precise booleans, such as incomplete source-class universe.
- Implement optional AnalyzeRefine only through analyzer-pre-authored opt-in nodes. The scheduler must not invent or append analysis-refine topology at runtime, and the refine trigger must consume typed `ProgressDelta` / proof-state booleans rather than retry prose.
- Add loopkernel shadow action comparison against current imperative scheduler choices.

Tests:
- `TestStageExpansionOptInPreciseBoolean`
- `TestLoopkernelShadowMatchesImperative`
- `TestAnalyzeRefineUsesPreAuthoredNodeOnly`

M2d-A scoped implementation:
- Add typed `ReadLoopShadowComparison` in loopkernel.
- Compare `ReadProofGuidance.RecommendedAction` with a caller-supplied typed imperative action, without changing scheduler behavior.
- First consumer: read retry/checkpoint status text may render the comparison for audit, but no hard gate may consume the rendered text.
- Dynamic opt-in nodes and AnalyzeRefine remain later M2d slices after the shadow comparison has package-level coverage.

M2d-B scoped implementation:
- Add `TaskNode.Optional` for analyzer-pre-authored opt-in nodes.
- Add registered typed criterion `source_class_universe_incomplete`, backed by source-inventory universe booleans in `criterion.Env`.
- Emit one optional source-inventory re-probe node from the deterministic compiler. The scheduler may dispatch it only when the typed criterion is satisfied.
- Optional nodes whose EntryConditions are false must not appear as blocked retry noise and must not prevent finalize readiness.
- This does not implement AnalyzeRefine; it establishes the opt-in node mechanism needed before AnalyzeRefine can safely exist.

M2d-C scoped implementation:
- Implement AnalyzeRefine only as an analyzer-pre-authored optional node. The scheduler must not create, append, or rewrite AnalysisIR topology at runtime.
- Add registered typed criterion `progress_replan_required`, backed by `EvidenceClosure.LatestProgressDecision`.
- Persist the latest low-delta `ProgressDecision` in `EvidenceClosure` so criterion evaluation consumes a typed carrier, not retry prose, model rationale, prompt text, or rendered status hints.
- Optional refine nodes whose typed progress criterion is false must stay quiet: no blocked-node noise and no finalize prevention.
- This completes the M2d dynamic expansion slice. Any future analyzer re-entry or IR rewrite is a new batch and must first define an immutable handoff/rewrite authority.

### Batch M3a: LoopBudget Enforcement

Deliverables:
- Lift sourceinventory budget discipline into `LoopBudget`: unit spend, deadline, cancellation, interrupted reason.
- Add a side-effect-free budget spend API (`unit` / `repair` / `approval`) that returns typed allow/deny decisions and an updated serializable budget.
- Keep controller/read scheduler cutover out of M3a; M3b will consume this API from `LoopRun.Advance`.
- Keep noisy elapsed telemetry out of hard gates.

Tests:
- `TestLoopBudgetEnforcesDeadlineLikeSourceInventory`
- `TestLoopBudgetSpendCapsArePreciseAndSerializable`
- Source inventory budget regression tests.

### Batch M3b: LoopRun.Advance Write-Side Cutover

Deliverables:
- M3b-A: implement pure `LoopRun.Advance` over typed loop events, authority projection, and `LoopBudget` spend decisions. It must not import writeflow or parse controller prose.
- M3b-A: prove `EventsFromWriteWorkflowRun -> ReduceEvents -> LoopRun.Advance` can recommend approval/repair/verify/localize actions and block on typed budget exhaustion.
- M3b-B: make write controller consume `LoopRun.Advance` for the already-stable `explore_code` and `ask_user` budget surfaces first. Replan/repair budget cutover stays later because it shares semantics with verify retry budgets.
- Keep read path projection-only in M3b.

Tests:
- `TestLoopRunAdvanceFromWriteWorkflowProjection`
- `TestLoopRunAdvanceBlocksOnTypedBudget`
- `TestLoopRunDrivesWriteController`
- `TestReadPathLoopkernelProjectionOnly`
- Write controller resume/approval/verify tests.

### Batch M3c: Semantic Routing And Shared Proof Authority

Deliverables:
- Map `LoopRecommendedAction` to ToolSuggestions/tool surfaces using only typed authority state.
- Ensure read/write both use `MergeProofCoverageAuthority` as the single proof coverage arbiter.
- First cut is a pure loopkernel route projection; it does not mutate skill configs at runtime. Stage-specific prompt/tool exposure can consume the typed route later.

Tests:
- `TestSemanticRoutingPreciseSignalsOnly`
- `TestReadWriteShareLoopkernelSubstrate`

### Batch M3d: Repair Budget Cutover

Deliverables:
- Move write replan/repair budget enforcement onto `LoopBudgetSpendRepair`, using existing verify retry counters and failed-verification attempt records as the typed source.
- Preserve existing retry-count semantics: `write_retry_budget=N` permits N repair replans after failed verification and blocks when the next failed verify would require another repair.
- Preserve current verify-infra unavailable semantics: missing dependencies/runner unavailable downgrade proof confidence and must not spend repair budget as source-code failure.
- Keep repair budget denial fail-loud and typed; no model prose, retry hint text, or final report narrative may drive the gate.

Tests:
- `TestLoopRunAdvanceRepairBudgetMatchesWriteRetryBudget`
- `TestVerifyUnavailableDoesNotSpendRepairBudget`
- Write controller verify/replan retry regressions.
- `TestSingleProofAuthority`
- Full read golden trace and write controller regression suite.

### Batch M4a: Delivery Audit Sync And LOC Ratchet

Deliverables:
- Refresh the Current State Audit so it matches actual completed M0-M3 evidence and does not leave stale `Open` / `Partial` rows that imply false progress.
- Extract concern-specific code out of hot files without changing behavior:
  - `internal/types/evidence_closure.go` must fall back under the PRD ratchet of 2774 lines.
  - `internal/orchestrator/scheduler.go` must fall back under the PRD ratchet of 945 lines.
- Add a structural ratchet test that reads source files and fails if the hot files exceed their documented line budgets.
- No prompt changes, no scheduler behavior changes, no new hard gates.

Tests:
- `go test ./internal/types ./internal/orchestrator`
- Full `go test ./...`

### Batch M4b: Commercial Hardening Evidence Matrix

Deliverables:
- Run and record the commercial hardening matrix after M0-M4a:
  - `go test ./...`
  - `make test`
  - focused read scheduler / golden-trace package tests
  - focused write controller / loopkernel package tests
  - focused source-inventory cross-language absence tests
- Record any failure as a new batch task before coding.

Tests:
- The commands above are the evidence.

### Batch M4c: Prompt/Schema/Tool-Noise Audit

Deliverables:
- Audit prompt/schema repair, unsupported-tool repair, JSON normalization, and tool recommendation surfaces touched by read/write/loopkernel paths.
- Verify hard logic does not inspect user intent keywords, model rationale, prompt text, final-answer prose, elapsed-time telemetry, ranker scores, or grep counts.
- Verify efficient typed navigation surfaces (`repo_map`, `trace_query`, source inventory) are represented as scheduler/tool-surface guidance where available instead of repeated broad scans.
- Record any discovered system gap as a new typed, per-class batch before implementation.

Tests:
- Focused prompt hygiene and structured-payload compatibility tests.
- Any new structural tests required by discovered gaps.

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
| M0c TaskNode slot guard | completed | Added the initial TaskNode slot guard; M2c upgrades it so Inputs/Outputs are readable only through explicit `TaskArtifactContract` projection and runtime artifact IDs cannot be reintroduced into TaskNode. |
| M1a ToolInvocation production wiring | completed | Added typed `invocation_id` / `params_ref` / `result_ref` through ReasoningGraph payload, reducer, replay, and audit views; production `cmd/root.go` now installs a shared observer into agent deps and orchestrator/subagent runtime; `FindToolInvocation` locates replayed calls. Verified with `go test ./cmd`, `go test ./internal/agent`, `go test ./internal/orchestrator`, `go test ./internal/reasoninggraph ./internal/types`. |
| M1b EvidenceClosure node status | completed | Added typed `NodeExecStatus` and EvidenceClosure accessors/clone/merge/reset coverage; `graphState` status mutations now dual-write through `setStatus` into the run closure while preserving scheduler map semantics. Verified with `go test ./internal/types` and `go test ./internal/orchestrator`. |
| M1c IngestRound shadow reducer | completed | Moved read-file coverage parsing into `types`, kept `tool/ground` as a compatibility wrapper, added `EvidenceRoundDelta` / `EvidenceClosure.IngestRound`, and wired `applyStageOutput` to run the reducer on a cloned closure only. Verified with `go test ./internal/types`, `go test ./internal/tool/ground`, and `go test ./internal/orchestrator`. |
| M1d ProgressDelta authority | completed | Added typed `ProgressDelta`, `ProgressDecision`, and `ShouldReplan`; downgrade low-delta convergence now records a typed progress delta through `EvidenceClosure.RecordDowngradeProgressDelta`, preserves prior threshold behavior, and carries `progress_delta_converged` into completion caveats. Hygiene test pins `ProgressDelta` has no prose routing fields. Verified with focused `go test ./internal/types` and `go test ./internal/tool`. |
| M1e SourceClassUniverse projection | completed | EvidenceClosure now mirrors typed `SourceInventoryObservation` / `SourceClasses`; MutableState source-inventory setters, TurnA handoff, fork merge, reset, and `SourceInventoryObservationFromMutable` all consume the same closure-backed authority. Loopkernel read proof snapshots now carry `SourceClasses`, `SourceClassUniverseComplete`, source-class evidence refs, and `ProofSnapshotFromReadMutable` / `ReadProofGuidanceFromMutable`; read retry checkpoints and parallel explore convergence consume the mutable view. Verified with source-inventory absence/convergence tests, `go test ./internal/tool/repomap`, and package regression for types/loopkernel/orchestrator/tool. |
| M1f Read failure memory | completed | Added deterministic typed read failure memory as a fallback into the existing `AnswerTaxonomyStore`; it persists retry/repair classes only as analyzer pitfall soft guidance, skips raw retry prose for routing, avoids duplicate fallback when the LLM answer reviewer emits a pattern, and creates no current-run hard repairs or reviewer violations. Verified with `go test ./internal/orchestrator -run 'TestReadFailureMemory|TestAnswerTaxonomy|TestReviewerRoundTrip|TestAnswerReviewer'`, `go test ./internal/agent -run 'TestPrependAnswerPitfalls|TestAnalyzer'`, package regression, and `go test ./...`. |
| M2a Stage nodes | completed | Added typed `NodeExtract` and deterministic `EnsureReadStageNodes` injection in the analyzer/compiler path; counterfactual expansion now routes through extract when present; binder/HDP require extract hypothesis binding; scheduler maps extract to `StageExtract`, excludes it from merged explore windows, and wraps the existing pre-finalize extract dispatch with node start/end/retry status updates. M2a deliberately uses a behavior-preserving typed readiness condition; richer extract readiness and optional AnalyzeRefine nodes are recorded under M2b. Verified with compiler/counterfactual/binder/HDP/types/orchestrator package tests and focused analyzer tests. |
| M2b Stage mapping/re-exec | completed | Added registered `extract_input_ready`, compiler now emits it on `NodeExtract`, and the read scheduler evaluates extract `EntryConditions` immediately before StageExtract dispatch. When no structured extract input exists, the extractor is skipped as a typed stage-skip and finalize still owns the answer contract. Tests pin that readiness uses typed evidence/chains/aggregate facts/external observations only, not `HasEnoughFacts`, raw tool prose, prompt text, or noisy telemetry. Scope correction: optional AnalyzeRefine moved to M2d because it crosses the analyzer/IR rewrite boundary. |
| M2c Execution tree/artifacts | completed | Removed misleading `TaskNode.ExitArtifacts`; added typed `TaskArtifactContract`, `BranchPoint`, `ValidationFeedbackBranchPoints`, and `ValidationFeedbackTargets`; scheduler validation-feedback backtrack now consumes the shared typed helper. Tests pin immutable contract projection, execution-tree backtrack grouping/deduplication, and the upgraded TaskNode slot guard. |
| M2d Dynamic expansion/loopkernel shadow | completed | M2d-A completed: added typed `ReadLoopShadowComparison` and first read retry checkpoint consumer so loopkernel recommended action can be compared with the imperative retry action without changing scheduler behavior. M2d-B completed: added `TaskNode.Optional`, typed `source_class_universe_incomplete`, compiler-emitted optional source-inventory re-probe node, source-class completeness helper, and scheduler handling so optional nodes do not create blocked noise or prevent finalize when their typed condition is false. M2d-C completed: added typed `progress_replan_required`, persisted the latest `ProgressDecision` in `EvidenceClosure`, projected it into `criterion.Env`, and pinned AnalyzeRefine as pre-authored optional topology only. No runtime scheduler topology invention or model-prose routing was introduced. Verified with focused compiler/criterion/types/orchestrator/loopkernel/tool/agent tests and full `go test ./...`. |
| M3a LoopBudget | completed | Added serializable `LoopBudget` deadline/interruption fields plus side-effect-free `NewLoopBudget`, `NormalizeLoopBudget`, `Check`, and typed spend APIs for unit/repair/approval. Deadline/cancel decisions carry typed reason codes, spend caps do not increment on denial, and unknown interrupted-reason prose is dropped during normalization. Controller/read scheduler cutover remains intentionally in M3b. Verified with loopkernel/sourceinventory focused tests and full `go test ./...`. |
| M3b LoopRun.Advance | completed | M3b-A completed: added pure `LoopRun.Advance`, `LoopAdvanceInput`, and `LoopAdvanceDecision`; write workflow event projection now drives typed recommendations for approval, repair, localization, and verification, with budget denial blocking via typed `LoopBudget` reason codes. Missing localization without candidate paths remains soft and does not preempt failed proof/repair. M3b-B completed for stable surfaces: write controller now consumes `LoopRun.Advance` for `explore_code` unit budget and `ask_user` approval budget while preserving the existing “reject explore action, do not poison workflow” behavior. Verified with loopkernel/writeflow/orchestrator focused tests and full `go test ./...`. Replan/repair budget cutover is tracked separately as M3d. |
| M3c Semantic routing/shared proof | completed | Added pure `LoopToolRoute` projection from `LoopRecommendedAction` to typed tool surfaces/tool suggestions; route decisions are stable under prose-only field changes and do not mutate skill configs at runtime. Read proof snapshots now call `DeriveProofCoverageAuthorityFromArtifacts(nil, ...)`, sharing the same proof-merge entrypoint as write projections. Verified with loopkernel focused tests and full `go test ./...`. |
| M3d Repair budget cutover | completed | Failed post-apply verify now checks the next repair opportunity through `LoopBudgetSpendRepair`, preserving existing `write_retry_budget` semantics and typed `verify_retry_budget_exhausted` external reason. Verification-unavailable lanes (runner missing/parser/no tests/unavailable rows) still complete as unverified and do not spend repair budget. Verified with loopkernel/writeflow/orchestrator focused tests and full `go test ./...`. |
| M4a Delivery audit sync / LOC ratchet | completed | Refreshed stale Current State Audit rows, extracted `EvidenceClosure` hash/clone/merge/MutableState helpers into `evidence_closure_helpers.go`, moved scheduler env/hyp progress helpers into `scheduler_progress.go`, and added `TestIRDeliveryHotFileLineRatchet`. Hot files are now under budget: `evidence_closure.go` 2638/2774 lines and `scheduler.go` 799/945 lines. Verified with `go test ./internal/types ./internal/orchestrator` and full `go test ./...`. |
| M4b Commercial hardening matrix | completed | Verified after M4a with focused read/write/loopkernel/source-inventory matrix: `go test ./internal/orchestrator -run 'TestE2E_ReadMode|TestIRDeliveryHotFileLineRatchet|TestMode_WriteController|TestRunWriteControllerWorkflow|TestE2E_WriteRetryCycle|TestLoopRunDrivesWriteController'`, `go test ./internal/loopkernel -run 'TestLoopRun|TestLoopBudget|TestSemanticRouting|TestReadWriteShareLoopkernelSubstrate|TestSingleProofAuthority|TestReadLoopShadow|TestEventsFromWriteWorkflowRun|TestProofSnapshotFromRead'`, `go test ./internal/tool ./internal/tool/repomap ./internal/types -run 'Test.*Source.*(Inventory|Class|Universe|Absence|Language|Cangjie|ArkTS|Config|Cross)|TestSourceInventory|TestAbsenceCloseReadsRepoWideUniverse'`, and full `make test` (`CGO_ENABLED=1 go test ./...`). |
| M4c Prompt/schema/tool-noise audit | pending | Must run after M4a so audit evidence is not mixed with structural refactor noise. |
