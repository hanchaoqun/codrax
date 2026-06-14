# Codrax 写模式流畅直写 + 商用审计交付方案

Date: 2026-06-14
Branch: main
Status: In progress

## 1. Summary

本设计把 Codrax 写模式收敛为 **controller-first dynamic DAG + smooth
direct-build** 的商用路径：

- 用户给出代码变更目标后，系统默认自己探索、拆批、规划、应用、验证、重试收敛。
- 低/中风险改动自动执行，避免不必要审批。
- 高风险改动进入可恢复人工审批。
- critical 风险自动拒绝，绝不进入 apply。
- planner/replan 只用 typed read/probe 工具，不使用普通 shell。
- verifier 的通过/失败只来自 typed `ChangeReport` verdict，不来自模型叙事。
- 前序探索、审批、风险、测试、失败证据进入 priority handoff，后续 controller/planner/verifier 按 Top-N 消费，不丢关键证据。
- 模型工具 JSON 输出统一经过结构修复和 strict decode，减少模型心智负担。

这不是为某个 eval case 打补丁。目标是把写模式变成一条泛化的商用交付链：能自己决定何时探索、何时拆批、何时直接小批量实现、何时验证、何时 replan，并在每个硬边界使用 typed artifacts 而不是 prompt prose。

`<think>` / thinking stream 渲染到用户侧日志是期望的透明性能力，不作为本方案缺陷处理。

本设计文档必须持续覆盖以下内容，后续批次不得只在代码里补丁式修复而不回写设计：

- 当前代码证据和系统级 gap ledger。
- 目标 controller-first DAG、batch 状态机、action executor 边界。
- ModePlan / ModeApply / ModeVerify 的终态语义。
- allow / ask / deny 权限模型、apply-pre gate、approval resume 和 fingerprint 规则。
- planner/replan/verifier/coder 的工具权限分层。
- typed JSON repair + strict decode 合同。
- verify result authority：只消费当前 active plan 的 post-apply typed report。
- failure evidence handoff：build/test/path/line/command 证据进入 P2 并参与小批量 replan。
- context pack 持久化、去重、Top-N consumer view。
- worktree、report、surface、diff artifact 的持久化和清理边界。
- prompt hygiene 红线：prompt 只做软指导，硬逻辑不读取用户关键词、模型 prose、summary、rationale 或 `<think>`。
- eval / e2e / regression 结果与每批交付进展。

## 2. Goals

- **流畅直写**：简单、低风险、目标明确的任务应少打断用户，自动走完 plan/apply/verify。
- **动态收敛**：复杂任务不要求一次性完整 plan；controller 可在探索过程中 append/split/replan batch。
- **商用可审计**：每个 apply/verify/approval/retry 都有 durable artifact 和 reason code。
- **安全低摩擦**：allow/ask/deny 三态；low/medium 自动，high ask，critical deny。
- **typed-first**：硬门只消费 enum、bool、fingerprint、parser output、path resolver、report/store records。
- **handoff 保真**：P0-P3 priority context pack 持久化，consumer-specific Top-N 渲染。
- **统一 JSON 修复层**：模型工具参数先做结构兼容修复，再 strict decode，再进入业务逻辑。
- **隔离稳定模式**：不影响 read、trace/log/data、operation/computer、worktree cleanup。

## 3. Non-Goals

- 不新增第二套写 workflow stack。
- 不把用户关键词、模型 summary/rationale、自由文本 `<think>` 变成硬路由。
- 不靠 prompt 禁令替代 runtime policy。
- 不为了某个 GitHub issue fixture 硬编码路径、语言、错误文本或测试名。
- 不改变读模式和 operation mode 的调度入口。

## 4. Current Code Evidence

当前代码已经具备以下基础：

- `internal/tool/emit_write_workflow_decision.go`：controller typed decision tool，schema 可按 mode 投影。
- `internal/writeflow/decision.go`：canonical action enum、ModePlan action mask、finish disposition。
- `internal/orchestrator/write_controller_scheduler.go`：controller-first outer loop、plan/apply/verify action executor、finish typed gate、verify failure handoff、budget completion verify。
- `internal/types/write_workflow_run.go`：durable run/batch/attempt/edge/progress schema。
- `internal/writeflow/attempt_state.go`：canonical single batch state derivation。
- `internal/types/change_plan.go`：`ChangeReport.Channel`、`FailureKind`、`ExecutedCommands`、`TestSurface`、`GeneratedAt`。
- `internal/tool/run_tests.go`：planner dry-run probe channel、post-apply verify channel、typed test surface escalation、strict param decode via unified repair path。
- `internal/types/verify_failure_handoff.go`：typed verify failure carrier into replan。
- `internal/types/write_context_pack.go`：priority context pack + consumer Top-N view。
- `internal/safety/write_policy.go` 和 `internal/writeflow/risk.go`：structured content/path risk policy。
- `internal/repl/handle_workflow.go`、`internal/repl/write_workflow_run_store.go`：workflow show/list/resume/clear 和 active run store。

因此本方案的方向是 **补齐边界和证据链**，不是重写所有模块。

## 5. Gap Ledger

### P0-1 Verify Result Authority

问题：

- Controller 不能因为 verifier 的 narrative、子测试 passing prose、或 planner dry-run 结果而判定完成。
- 只有当前 active batch、当前 plan id、post-apply verify channel 的 typed report 可以驱动 finish/replan/block。

已有基础：

- `ChangeReport.Channel` 区分 `planner_probe` 和 `post_apply_verify`。
- `installRunTestsReport` 在 StagePlan dry-run 下写入 `PlanStageProbeReports`。
- `authoritativeWriteControllerReport` 过滤非 post-apply report。
- `FinishBlockedReason` 读取 typed verify attempt，拒绝失败后直接 finish。

仍需补：

- 增加 planner probe pass + post-apply fail 的 controller 测试。
- 增加 planner probe fail + post-apply pass 的 controller 测试。
- 增加 finish 不能消费 planner probe 的测试。
- 增加 verify failure 后 replan 只读 latest active batch attempt 的测试。

### P0-2 ModePlan Terminal Semantics

问题：

- ModePlan 只应探索/规划并落盘计划，不能进入 apply/verify 决策。

已有基础：

- `WorkflowActionsForMode(ModePlan)` 从 schema 删除 `apply_plan` / `verify_batch`。
- scheduler 在 plan mode 中生成 plan 后 `plan_mode_complete`。

仍需补：

- Prompt/schema hygiene test 保证 ModePlan prompt 不再暗示 apply/verify。
- Controller runtime rejection path 保持防御纵深。

### P0-3 Durable Workflow / Approval Resume

问题：

- pending approval 必须 `/workflow show/list/resume` 可见。
- `/approve` 必须继续同一个 run/batch/plan fingerprint。
- plan 内容变化后审批失效，必须重新审批。

已有基础：

- `WriteWorkflowRun` 有 batch attempts 和 approval refs。
- `WriteApprovalRecord`、plan fingerprint、REPL workflow store 已存在。

仍需补：

- approval resume e2e：pending -> restart/load -> approve -> apply -> verify -> finish。
- stale fingerprint direct test。
- show/list 输出 snapshot 覆盖 approval/risk/batch state。

### P0-4 Failure Evidence Handoff

问题：

- verify failure 后 planner 必须拿到失败点证据，而不是重新大范围探索。
- 证据包括 command、runner/framework、cwd、exit code、failure kind、build file/line、assertion、blob ref、diff ref、next surface candidate。

已有基础：

- `VerifyFailureHandoff` 从 `ChangeReport` 投影 typed fields。
- `WriteContextPackFromChangeReport` 生成 P2 failure items。
- `persistVerifyFailureEvidence` 保存 report JSON 和 attempt diff。

仍需补：

- standalone `TestSurface` artifact ref，不只依赖 report 内嵌。
- P2 failure dedupe by command/assertion/build location。
- Replan prompt snapshot：P2 failure 必须排在旧 P1 code facts 前。

### P1-1 Single State Machine

问题：

- plan approval/apply/verify/progress 不能同时呈现互相矛盾状态。

已有基础：

- `DeriveBatchAttemptState` 将 `ready_to_plan + failed verify` 派生为单一 `needs_replan`。
- controller prompt 渲染 derived state，不直接混合 progress event 和 state。

仍需补：

- snapshot 覆盖 `pending_approval`、`needs_replan`、`unverified`、`accept_unverified`、`blocked`。

### P1-2 Planner/Replan Tool Permission Split

问题：

- planner/replan 阶段不应运行普通 `exec_command`。
- 需要执行反馈时应使用 typed dry-run tool，例如 `run_tests(dry_run=true)`，结果进入 probe channel。
- 不能让 StagePlan 下的非 dry-run `run_tests` 污染 authoritative `ChangeReport`。

本批修复：

- write StagePlan schema 隐藏 `exec_command`、`apply_patch`、`emit_test_results`。
- write StagePlan 的 `run_tests` schema 投影为 `dry_run=true` required。
- runtime hard gate 拒绝 planner `exec_command`、`apply_patch`、`emit_test_results`。
- runtime hard gate 拒绝 planner `run_tests` without `dry_run=true`。

硬门输入：

- `AgentContext.Stage`
- `AgentContext.Mode`
- canonical tool name
- tool-call JSON typed field `dry_run`

不读取用户文本、模型 prose、summary、rationale 或 `<think>`。

### P1-3 Context Pack Dedupe / Top-N

问题：

- context pack 太丰富会造成重复探索和 prompt bloat。
- P2 verify failure 在 replan 中应高于旧 P1 code facts。

已有基础：

- `WriteContextPack.View(consumer, limit)`。
- evidence-backed items 基于 priority/kind/source/file/line key dedupe。

仍需补：

- verify failure item identity：runner/framework/cwd/assertion/build location/failure kind。
- consumer-specific snapshot tests。

### P1-4 Worktree / Report Persistence

问题：

- failure path 必须在 cleanup 前保留足够审计证据。
- 用户提示和 report/workflow refs 必须指向 live plan，而不是第一轮旧 plan。

已有基础：

- plan mirror、report save、attempt diff capture、GeneratedAt backfill 已存在。

仍需补：

- Report/surface/diff/artifact refs 的一致性检查。
- Eval runner 读取 latest workflow artifact，而不是固定 `run-1.plan.json`。

## 6. Target Architecture

```text
User code-change goal
  -> write_analyzer
       emits WriteAnalysisIR
  -> WriteWorkflowRun seed
  -> write_controller
       emits one typed action enum
  -> optional explore_code
       read-only exploration
       projects WriteContextPack P0/P1/P2/P3
  -> plan_batch / replan_batch / split_batch / append_batch
       planner reads bounded batch + context pack
       uses read tools + run_tests(dry_run=true)
       emits bounded ChangePlan
  -> apply_plan
       apply-pre risk/permission gate
       low/medium allow
       high ask
       critical deny
       coder applies in worktree
  -> verify_batch
       run_tests selects typed TestSurface
       executes canonical command(s)
       parser emits ChangeReport
  -> typed evaluator
       pass -> batch complete / finish
       fail -> P2 failure handoff -> replan/split/explore
       no tests -> unverified caveat / typed finish disposition
       infra blocked -> block / ask_user
```

## 7. Smooth Direct-Build Semantics

### 7.1 Simple Low-Risk Path

For a small, localized, low/medium risk change:

1. Analyzer produces task/risk/scope IR.
2. Controller chooses `plan_batch`.
3. Planner emits one bounded `ChangePlan`.
4. Apply-pre gate returns `allow`.
5. Coder applies in worktree.
6. Verifier runs typed test surface.
7. Controller finishes.

No user approval is requested.

### 7.2 Complex Dynamic Path

For a broad or uncertain change:

1. Controller chooses `explore_code`.
2. Read-only exploration emits evidence/handoff.
3. Controller chooses bounded `plan_batch`.
4. Verify failure or scope discovery may lead to `replan_batch`,
   `split_batch`, `append_batch`, or another bounded `explore_code`.
5. The DAG converges in small applied-and-verified batches.

The model gets flexibility through typed actions, not through parsing its prose.

### 7.3 High / Critical Risk Path

- high: `pending_approval`, durable approval record, `/approve` resumes same run.
- critical: `blocked` before apply, no mutation.
- stale fingerprint: approval invalid, ask again.

## 8. Permission And Approval Model

Shared policy shape:

```text
allow -> execute automatically
ask   -> durable pending approval
deny  -> block before mutation
```

Hard inputs:

- repo-relative path resolver
- worktree boundary
- external-directory policy
- `.git` / hooks / workflow / manifest / secret-like path policy
- JSON/YAML/XML/PEM parser output
- typed command/tool policy
- plan fingerprint
- approval record
- attempt state

No hard input:

- user raw keyword matching
- model natural-language rationale
- verifier narrative
- `<think>` text
- prompt wording

## 9. Agent Tool Matrix

| Agent | Write Stage | Allowed Shape |
| --- | --- | --- |
| write_analyzer | analyze | read_file/list_files/repo_map/grep + emit_write_analysis |
| write_controller | write_controller | emit_write_workflow_decision only |
| explorer | explore_code | read-only repo tools; no shell/tests/apply/plan emit |
| planner | plan/replan | read_file/grep/list_files/repo_map + run_tests(dry_run=true) + plan emit tools |
| coder | apply | apply_patch over current ChangePlan in worktree |
| verifier | verify | run_tests post-apply + optional emit_test_results narrative |

Runtime policy must enforce the matrix even if a custom skill exposes a broader
tool list.

## 10. JSON Tool Output Repair Contract

Every model-facing JSON tool path should follow:

```text
raw params
  -> bounded structural repair / compatibility aliases
  -> strict decode with unknown-field rejection
  -> typed validation
  -> durable artifact/state mutation
```

Examples already aligned:

- `run_tests`: `decodeStrictToolParams`.
- `emit_write_workflow_decision`: `applyStructuredPayloadCompat` + strict decode.
- `emit_test_results`: shared compatibility path.
- apply/plan tools: strict validators and structured edit compilation.

Follow-up:

- Add registry coverage test for new write tools.
- Keep prompts/hints in sync with actual schema projection.

## 11. Handoff Design

Priority lanes:

- P0: user hard constraints, safety boundary, approval/risk, scope boundary.
- P1: target files, symbols, invariants, line-backed source evidence.
- P2: test surface, verify failures, build errors, unknowns, retry blockers.
- P3: local style/pattern hints.

Consumers:

- controller: P0/P1/P2 routing view.
- planner: P0/P1/P2 failure-first replan view + P3 style hints.
- verifier: P0/P2 test surface and plan/apply refs.
- approval: P0 risk/approval facts.

Persistence:

- Full context pack is durable.
- Prompt gets bounded Top-N.
- Evidence refs survive process restart.

## 12. Durable State Model

Run:

- `RunID`
- `Goal`
- `Status`
- `ActiveBatchID`
- `Batches`
- `Edges`
- `ContextPacks`
- `Budget`
- `ProgressLedger`

Batch:

- `Goal`
- `Status`
- `DependsOn`
- `PlanRef`
- `ApplyRef`
- `VerifyRef`
- `ApprovalRef`
- `ContextPackIDs`
- `Attempts`

Attempt:

- `Kind`
- `Status`
- `ReasonCode`
- `PlanID`
- `ReportID`
- `ArtifactRef`
- timestamps

Derived state:

- `DeriveBatchAttemptState` is the single model-facing state.
- Progress ledger is event history, not current state.

## 13. CLI / REPL UX

Required UX:

- `/workflow show`: active run, active batch, derived state, approval, latest report, context summary, budget.
- `/workflow list`: durable runs with status and active batch.
- `/workflow resume`: hydrate active run and continue.
- `/workflow clear`: clear run and context artifacts intentionally.
- `/approve`: approve active batch plan fingerprint only.
- `/reject`: reject current batch and return control to controller.

Approval messages should explain risk/action/reason code and fingerprint, not
ask for approval on low/medium cases.

## 14. Test Matrix

### Unit

- ModePlan action schema excludes apply/verify.
- Write planner schema hides generic shell/apply/verifier-result tools.
- Write planner `run_tests` schema requires `dry_run=true`.
- Write planner runtime rejects forbidden tools and non-dry-run tests.
- `run_tests(dry_run=true)` writes `PlanStageProbeReports`.
- post-apply `run_tests` writes authoritative `ChangeReport`.
- `FinishBlockedReason` rejects failed latest verify attempts.
- `DeriveBatchAttemptState` emits a single phase.
- `WriteContextPack.View` dedupes and respects consumer Top-N.

### Controller

- planner probe pass + post-apply fail -> replan/block, not finish.
- planner probe fail + post-apply pass -> finish allowed.
- verify fail -> P2 handoff -> replan sees failure lead section.
- pending approval survives store reload.
- stale approval fingerprint fails.
- budget completion verify runs once for applied-but-unverified batch.

### E2E / Eval

- single-file low-risk auto apply.
- multi-file complex split/replan.
- high-risk manual approval.
- critical deny.
- imported plan through same apply-pre gate.
- no-tests caveat cannot silently look like tested success.
- runner missing escalates through typed test surface.
- worktree cleanup remains unconditional.
- read/log/trace/data/operation/computer regressions stay green.

## 15. Delivery Tasks

### Batch 0: Direct-Build Design Ledger

- [x] Create this full design document.
- [x] Commit and push.

### Batch 1: Planner Typed Tool Policy

- [x] Hide generic shell/apply/verifier-result tools from write StagePlan schema.
- [x] Project `run_tests` schema in write StagePlan to require `dry_run=true`.
- [x] Runtime reject write-planner `exec_command`, `apply_patch`, `emit_test_results`.
- [x] Runtime reject write-planner `run_tests` unless JSON field `dry_run=true`.
- [x] Add tests.
- [x] Run targeted tests.
- [x] Run affected-package regression.
- [x] Commit and push.

### Batch 2: Verify Authority Regression

- [x] Add planner-probe vs post-apply report authority tests.
- [x] Add finish rejection tests for stale/failed post-apply attempts.
- [x] Add controller artifact rendering tests for current plan id only.
- [x] Run targeted tests.
- [x] Run affected-package regression.
- [x] Commit and push.

### Batch 3: Durable Approval Resume

- [x] Add pending approval resume e2e.
- [x] Add stale fingerprint approval test.
- [x] Add `/workflow show/list/resume` snapshot coverage.
- [x] Run targeted tests.
- [x] Run affected-package regression.
- [x] Commit and push.

### Batch 4: Failure Evidence Store

- [x] Persist standalone surface artifact refs for failed attempts.
- [x] Attach report/diff/surface refs to verify attempts.
- [x] Add cleanup-safe artifact tests.
- [x] Run targeted tests.
- [x] Run affected-package regression.

### Batch 5: Context Pack Retry Dedupe

- [ ] Add verify failure dedupe keys.
- [ ] Add planner/controller/verifier Top-N snapshots.
- [ ] Ensure P2 failures outrank stale P1 facts on replan.

### Batch 6: Commercial Hardening

- [ ] Run focused write workflow package tests.
- [ ] Run `go test ./...`.
- [ ] Run `make test`.
- [ ] Re-run write-mode eval cases as requested.
- [ ] Update this ledger with final verdicts and pushed commits.

## 16. Acceptance Criteria

- Low/medium risk direct-build writes do not ask for user approval.
- High risk writes are pending-approval and resumable.
- Critical writes are denied before mutation.
- Planner cannot run generic shell or post authoritative verify reports.
- Controller decisions are typed action enum only.
- Verify completion is driven by typed post-apply `ChangeReport`.
- Verify failure evidence enters P2 handoff and survives cleanup/restart.
- Context packs preserve rich evidence while rendering bounded Top-N.
- All model tool JSON follows repair + strict decode before hard logic.
- Prompt changes do not introduce keyword/prose routing.
- Other modes remain stable.

## 17. Progress Ledger

| Date | Batch | Status | Evidence |
| --- | --- | --- | --- |
| 2026-06-14 | 0 | pushed | Full design document created and pushed on `main` in commit `d4169108` together with Batch 1. |
| 2026-06-14 | 1 | pushed | Implemented planner typed tool policy in `internal/agent/agent.go`; tests in `internal/agent/agent_tool_context_test.go`; updated `internal/skill/defaults.go`. Targeted tests passed with `GOCACHE=/private/tmp/codrax-gocache`: `go test ./internal/agent -run 'Test(BuildToolSchemas_WritePlannerHidesShellAndForcesDryRunProbe|ValidateWritePlannerToolPolicy_RejectsShellAndNonDryRunTests|ExecuteTool_WriteExplorationSubflowRejectsShellCommand|ChangePlanSkill_BatchLocalPlanningWorkflow)'`; `go test ./internal/skill -run 'TestChangePlanSkill_BatchLocalPlanningWorkflow|TestWriteControllerSkill'`. Affected package regression passed with `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/agent ./internal/skill ./internal/tool ./internal/types ./internal/writeflow ./internal/orchestrator ./internal/repl`. |
| 2026-06-14 | 2 | pushed | Controller verify authority now rejects non-post-apply or stale-plan reports and records them as failed attempts for the active plan. Added planner-probe pass/post-apply fail, planner-probe fail/post-apply pass, and invalid verify report tests. Targeted tests passed with `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/orchestrator -run 'TestRunWriteControllerWorkflow_(PlannerProbePassCannotFinishFailedPostApplyVerify|PlannerProbeFailureDoesNotBlockPassedPostApplyVerify|RejectsNonAuthoritativeVerifyReports|FinishGateRequiresTypedDisposition|FinishAfterPassedVerifyNeedsNoDisposition)'`. Affected package regression passed with `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/orchestrator ./internal/writeflow ./internal/agent ./internal/tool ./internal/types ./internal/repl`. Implementation pushed on `main` in commit `6e4d0bce`. |
| 2026-06-14 | 3 | pushed | Durable approval resume production path already exists through `WriteWorkflowRunStore`, active workflow plan binding, `/workflow resume`, `/approve`, and apply-pre fingerprint gate. Added regression coverage for resume -> approve continuing the same pending-approval run, persisted approval record fingerprint integrity, `/workflow list` snapshots, and stale fingerprint blocking at apply-pre. Targeted tests passed with `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/repl ./internal/orchestrator -run 'Test(WorkflowShowDisplaysActiveWriteWorkflow|ApproveUsesActiveWorkflowBatchPlan|WorkflowResumeSelectsSavedWriteWorkflow|WorkflowResumeThenApproveContinuesSamePendingApprovalRun|WorkflowListDisplaysSavedWriteWorkflowSnapshots|RejectMarksOnlyActiveWorkflowBatchBlocked|ApplyPreHook_PlanFileStaleManualApprovalBlocks)'`. Affected package regression passed with `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/repl ./internal/orchestrator ./internal/types ./internal/writeflow`. Implementation pushed on `main` in commit `c05c4554`. |
| 2026-06-14 | 4 | tests_passed_affected_packages | Failure evidence store now persists a standalone `<plan>.attempt-N.surface.json` artifact, adds `surface_ref` to verify attempts, carries `surface_artifact_ref` in `VerifyFailureHandoff`, and renders the surface artifact in the planner replan handoff section. Existing report and diff refs remain typed fields (`report_id`, `artifact_ref`). Targeted tests passed with `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/types ./internal/writeflow ./internal/agent ./internal/orchestrator -run 'Test(WriteTestSurfaceToFileRoundTripNormalizesSelectedID|NormalizeWriteWorkflowRunPersistsContextPacks|BuildVerifyFailureHandoff_ProjectsTypedRows|BuildVerifyFailureHandoff_NilForPassedOrNil|BuildVerifyFailureHandoff_Bounds|BuildVerifyFailureHandoffSection_LeadsReplanPrompt|RunWriteControllerWorkflow_VerifyFailureSetsHandoffAndGreenClears|RunWriteControllerWorkflow_ResumeHydratesRetryPlanAndHandoff)'`. Affected package regression passed with `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/types ./internal/writeflow ./internal/agent ./internal/orchestrator ./internal/tool ./internal/repl`. |
