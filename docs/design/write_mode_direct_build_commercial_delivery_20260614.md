# Codrax 写模式流畅直写 + 商用审计交付方案

Date: 2026-06-14
Branch: main
Status: Direct-build hardening complete; external GitHub issue eval ledger current; Rust/multi-repo/symptom-driven recovery expansion delivered

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

### 1.1 Required Whole-Document Supplement

任何后续写模式设计或实现补充，都必须把“完整设计文档要补的内容”写回本文档，而不是只在代码、commit message、聊天上下文或 eval 摘要中保留。必填内容如下：

| Block | Required Content | Acceptance Rule |
| --- | --- | --- |
| Problem and evidence | 用户可见症状、系统症状、local file/report/artifact refs、eval case id、upstream issue/PR 链接、typed fields | 不能用模型散文或 `<think>` 作为硬证据 |
| Systemic gap | 失败类别、影响面、为什么代表一类系统问题 | 不能只描述单个 case 的表面修补 |
| Target architecture | controller DAG、batch/attempt/edge 状态、action enum、artifact refs、store/resume 语义 | 架构必须泛化到同类任务 |
| Safety and permissions | allow/ask/deny、critical deny、high approval、low/medium auto、fingerprint/stale approval | 硬门只读 typed artifacts、parser output、path/risk policy |
| Agent/tool boundary | controller/planner/replan/coder/verifier/explorer 的工具权限和 stage policy | 不允许 planner/replan 使用普通 shell 作为硬逻辑入口 |
| Verify authority | authoritative `post_apply_verify` report、current plan id、typed package/build/test verdict | 不能靠 verifier narrative 或子测试 passing prose 判成功 |
| Handoff | P0/P1/P2/P3 事实、dedupe key、Top-N consumer view、evidence refs、restart/worktree cleanup 后保真 | verify failure 必须以 P2 进入 replan 消费 |
| Durable state | run/batch/attempt/event ledger、approval refs、report/diff/surface refs、derived state | 当前状态只有一个来源，progress ledger 只做历史 |
| Implementation plan | batch-sized tasks、owner surface/package、dependency order、rollback/compat note、commit/push expectation | 每批都要可执行、可验证、可回滚 |
| Test matrix | unit/controller/CLI/REPL/eval/regression commands、expected artifacts、failure caveats | PASS 必须能追溯到 typed artifacts |
| Progress ledger | commit hash、push status、commands run、失败与剩余风险 | 每批结束更新，不等最终统一补 |

这张表是本文档其余章节的索引合同：Sections 4-16 给出基础设计，Section 17 记录交付证据，Section 18 固化未来补充模板，Section 19 追加外部实战 eval 发现的新 gap 和对应系统设计。

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

闭环证据：

- `validateControllerPostApplyReport` 只接受当前 active plan 的 `post_apply_verify` typed report。
- `FinishBlockedReason` 只读 typed attempt records 和 `finish_disposition` enum。
- 测试覆盖 planner probe pass + post-apply fail、planner probe fail + post-apply pass、非 authoritative report rejection、verify failure 后 handoff/replan。

### P0-2 ModePlan Terminal Semantics

问题：

- ModePlan 只应探索/规划并落盘计划，不能进入 apply/verify 决策。

已有基础：

- `WorkflowActionsForMode(ModePlan)` 从 schema 删除 `apply_plan` / `verify_batch`。
- scheduler 在 plan mode 中生成 plan 后 `plan_mode_complete`。

闭环证据：

- `WorkflowActionsForMode(ModePlan)` 和 projected schema 均移除 `apply_plan` / `verify_batch`。
- Runtime 对 ModePlan 中的 `apply_plan` / `verify_batch` 仍 fail-loud。
- Skill hygiene 测试禁止旧静态 phase、unsupported action、用户关键词/prose 路由气味回流到 planner prompt。

### P0-3 Durable Workflow / Approval Resume

问题：

- pending approval 必须 `/workflow show/list/resume` 可见。
- `/approve` 必须继续同一个 run/batch/plan fingerprint。
- plan 内容变化后审批失效，必须重新审批。

已有基础：

- `WriteWorkflowRun` 有 batch attempts 和 approval refs。
- `WriteApprovalRecord`、plan fingerprint、REPL workflow store 已存在。

闭环证据：

- `/workflow resume`、`/approve`、active workflow store 和 apply-pre fingerprint gate 已接入同一 run/batch/plan 记录。
- 测试覆盖 pending approval reload 后 approve 继续同一 run、`/workflow list` snapshots、stale fingerprint apply-pre blocking、approval record integrity。

### P0-4 Failure Evidence Handoff

问题：

- verify failure 后 planner 必须拿到失败点证据，而不是重新大范围探索。
- 证据包括 command、runner/framework、cwd、exit code、failure kind、build file/line、assertion、blob ref、diff ref、next surface candidate。

已有基础：

- `VerifyFailureHandoff` 从 `ChangeReport` 投影 typed fields。
- `WriteContextPackFromChangeReport` 生成 P2 failure items。
- `persistVerifyFailureEvidence` 保存 report JSON 和 attempt diff。

闭环证据：

- failed verify attempt 持久化 report JSON、diff artifact、standalone `TestSurface` artifact，并将 `surface_ref` 写入 attempt。
- `VerifyFailureHandoff` 携带 `surface_artifact_ref`、failure kind、command、test/build rows 和 retry attempt。
- `WriteContextPackFromChangeReport` 使用 typed IDs 对 failed tests、build errors、executed commands、failure blobs 去重。
- Planner consumer view 中 P2 verify failure 排在 stale P1 facts 前，P0 constraints 保持最高优先级。

### P1-1 Single State Machine

问题：

- plan approval/apply/verify/progress 不能同时呈现互相矛盾状态。

已有基础：

- `DeriveBatchAttemptState` 将 `ready_to_plan + failed verify` 派生为单一 `needs_replan`。
- controller prompt 渲染 derived state，不直接混合 progress event 和 state。

闭环证据：

- `DeriveBatchAttemptState` 是 controller-facing 单一状态派生入口。
- Controller tests 覆盖 pending approval、needs replan、retry budget blocked、no-tests unverified caveat、accept unverified finish disposition。

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

闭环证据：

- Verify failure item identity 使用 runner/framework/working_dir/command、assertion id、build file/line/column/symbol、failure kind 等 typed fields。
- `WriteContextPack.View` 有 consumer-specific ordering/bounds/defensive-copy 测试；planner replan view 会优先消费 P2 failure evidence。

### P1-4 Worktree / Report Persistence

问题：

- failure path 必须在 cleanup 前保留足够审计证据。
- 用户提示和 report/workflow refs 必须指向 live plan，而不是第一轮旧 plan。

已有基础：

- plan mirror、report save、attempt diff capture、GeneratedAt backfill 已存在。

闭环证据：

- Verify failure persistence 将 report、diff、surface refs 绑定到 latest verify attempt；resume hydration 从 attempt state 恢复 handoff。
- Active plan mirror 和 workflow run store 保证用户提示、report 和 resumed run 指向 live plan。
- Write-mode eval sweep 覆盖 Python/C++/Java patch cases，3/3 PASS，flagged 0/3。

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

闭环证据：

- BaseAgent 在 local tool 与 MCP tool 执行前统一执行 structural JSON repair + schema-aware compat。
- `emit_write_workflow_decision`、`run_tests`、`emit_test_results`、`emit_plan_skeleton` 均有 compat/strict decode tests。
- Planner schema projection 和 prompt hints 由 tests 固定：write planner hides shell/apply/verifier tools，并要求 `run_tests(dry_run=true)`。

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
- [x] Commit and push.

### Batch 5: Context Pack Retry Dedupe

- [x] Add verify failure dedupe keys.
- [x] Add planner/controller/verifier Top-N snapshots.
- [x] Ensure P2 failures outrank stale P1 facts on replan.
- [x] Run targeted tests.
- [x] Run affected-package regression.
- [x] Commit and push.

### Batch 6: Commercial Hardening

- [x] Run focused write workflow package tests.
- [x] Run `go test ./...`.
- [x] Run `make test`.
- [x] Re-run write-mode eval cases as requested.
- [x] Update this ledger with final verdicts and pushed commits.

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
| 2026-06-14 | 4 | pushed | Failure evidence store now persists a standalone `<plan>.attempt-N.surface.json` artifact, adds `surface_ref` to verify attempts, carries `surface_artifact_ref` in `VerifyFailureHandoff`, and renders the surface artifact in the planner replan handoff section. Existing report and diff refs remain typed fields (`report_id`, `artifact_ref`). Targeted tests passed with `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/types ./internal/writeflow ./internal/agent ./internal/orchestrator -run 'Test(WriteTestSurfaceToFileRoundTripNormalizesSelectedID|NormalizeWriteWorkflowRunPersistsContextPacks|BuildVerifyFailureHandoff_ProjectsTypedRows|BuildVerifyFailureHandoff_NilForPassedOrNil|BuildVerifyFailureHandoff_Bounds|BuildVerifyFailureHandoffSection_LeadsReplanPrompt|RunWriteControllerWorkflow_VerifyFailureSetsHandoffAndGreenClears|RunWriteControllerWorkflow_ResumeHydratesRetryPlanAndHandoff)'`. Affected package regression passed with `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/types ./internal/writeflow ./internal/agent ./internal/orchestrator ./internal/tool ./internal/repl`. Implementation pushed on `main` in commit `5b1c03ec`. |
| 2026-06-14 | 5 | pushed | Context pack retry dedupe now uses stable typed item IDs for failed tests, build errors, executed commands, regression assertions, no-tests runners, and failure blobs. `WriteContextPack.View` gives planner replan views a typed ordering where verify failure P2 evidence outranks stale P1 code facts while P0 constraints remain first. Added tests for failure identity dedupe, planner failure-first ordering, existing consumer filtering, and bounded defensive Top-N views. Targeted tests passed with `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/types -run 'TestWriteContextPack(FromChangeReportCarriesVerifyFailure|FromChangeReportDedupesVerifyFailureIdentity|PlannerViewPrioritizesVerifyFailureBeforeStaleP1Facts|ViewBoundsAndDefensiveCopy|FromExplorationHandoffPrioritizesEvidence|FromWriteAnalysisIRPreservesP0Constraints)'`. Affected package regression passed with `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/types ./internal/agent ./internal/orchestrator ./internal/writeflow ./internal/repl`. Implementation pushed on `main` in commit `652ab27c`. |
| 2026-06-14 | 6 | complete | Focused write workflow packages passed with `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/types ./internal/writeflow ./internal/agent ./internal/orchestrator ./internal/repl`. Full `go test ./...` and `make test` both initially hit sandbox local-port bind restrictions in `internal/llm` / `internal/preview`, then passed in the approved escalated environment with the same cache settings. Write-mode eval sweep passed for `eval/cases/patch_python_typo.case`, `eval/cases/patch_cpp_typo.case`, and `eval/cases/patch_java_typo.case`; aggregate summary `eval/results/write_mode_hardening_20260614_summary.md` reports PASS for 3/3 and flagged 0/3. |
| 2026-06-14 | 18-20 | pushed | External GitHub issue hardening continued from the 8-case sweep. Batch 18 fixed eval source aggregation so non-git materialized applied trees nested under the Codrax repo use file traversal instead of Codrax `git ls-files`; `zod` recheck passed. Batch 19 added one-shot typed replan for off-scope high-risk paths before surfacing manual approval, preserving true build/config approval; `commons-lang` recheck passed. Batch 20 broadened two fixture content oracles to accept semantic-equivalent source shapes and case-insensitive hex literals. Verification: `bash -n eval/run.sh`; `bash -n eval/runner_lib.sh`; `bash -n eval/runner_lib_test.sh`; `bash eval/runner_lib_test.sh`; `go test ./internal/orchestrator -run 'TestRunWriteControllerWorkflow_ReplansOffScopeHighRiskBuildManifest|TestRunControllerPlanBatch_KeepsTypedBuildSystemChangeForApproval'`; `go test ./internal/orchestrator ./internal/writeflow ./internal/types ./internal/agent ./internal/tool ./internal/repl`; targeted eval summaries `write_mode_zod_prefault_after_b18_20260614_summary.md`, `write_mode_commons_lang_offscope_replan_20260614_summary.md`, and `write_mode_github_issue_oracle_recheck_20260614_summary.md` all passed their targeted cases. Code implementation committed on `main` in `506d68cd`; this document records the design/evidence follow-up. |
| 2026-06-14 | 21 | pushed | Added Rust-shaped external issue fixture `github_issue_chrono_duration_min` from chronotope/chrono PR #1385. First run `eval/results/write_mode_chrono_duration_min_20260614_summary.md` failed with authoritative `post_apply_verify passed=false`, exposing an over-narrow fixture oracle plus a real explore-stage soft-guidance gap (`unavailable_tool_attempts=1` from write exploration attempting shell/write work). The fixture oracle was corrected to encode the upstream `Option<Duration>` / `-i64::MAX` / non-recursive MIN/MAX contract and test-file structure checks. Re-run `eval/results/write_mode_chrono_duration_min_after_oracle_20260614_summary.md` passed 1/1 with `verify_authoritative=true`, `report_channel=post_apply_verify`, and `report_passed=true`. Product follow-up added read-only handoff guidance to write exploration and a prompt hygiene test. Verification: `bash -n eval/cases/github_issue_chrono_duration_min.case`; `PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache python3 -m py_compile eval/fixtures/github_issues/chrono_duration_min/tests/check_duration_min.py`; expected initial `make check` failure on the seed fixture; targeted eval PASS; `go test ./internal/agent`; rebuilt `codrax` with `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache make` (exit 0; Go stat-cache warning only). |
| 2026-06-14 | 26 | pushed | Added two symptom-driven external issue cases: `github_issue_chrono_duration_min_symptom` (Rust, chronotope/chrono PR #1385) and `github_issue_commons_lang_random_ascii_symptom` (Java, apache/commons-lang PR #1273). Baseline summary `eval/results/write_mode_symptom_chrono_commons_20260614_summary.md` failed 2/2: chrono had `plan_written=false`, `apply_attempted=false` after exploration completed but no `ChangePlan` was installed; commons-lang had `plan_written=true`, `apply_attempted=true`, but no durable worktree/report after apply incomplete and controller dispatch transport failure. Implemented typed recovery only from durable state: no-plan retry after typed exploration/verify handoff; dispatch-error recovery to `apply_plan` for auto-executable plans and `verify_batch` for applied-but-unverified plans. Re-run `eval/results/write_mode_symptom_chrono_commons_after_recovery_20260614_summary.md` passed 2/2, flagged 0/2. Verification includes targeted orchestrator tests, affected package tests, rebuild, and symptom eval. |

## 18. Design Document Coverage Checklist

This document is the canonical delivery ledger for the write-mode commercial hardening work. It intentionally includes the full set of design-document supplements needed to evaluate and continue the system-level architecture without reconstructing intent from commits:

- Current code evidence and system-level gap ledger: Sections 4 and 5.
- Target controller-first DAG, dynamic batch loop, and direct-build semantics: Sections 6 and 7.
- ModePlan / ModeApply / ModeVerify terminal behavior and action boundaries: Sections 5, 6, 7, and 14.
- Permission model, apply-pre approval gate, high-risk resume, critical deny, and fingerprint rules: Sections 8 and 13.
- Agent/tool permission split for controller, planner, replan, coder, explorer, and verifier: Section 9.
- Typed JSON repair and strict decode contract: Section 10.
- Verify result authority and post-apply typed report requirements: Sections 5, 10, and 14.
- Failure evidence handoff, test/build/path/line/command surfaces, and P2 replan consumption: Sections 5 and 11.
- Durable run/batch/attempt state, single state derivation, report/surface/diff refs, and restart behavior: Sections 11, 12, and 13.
- Prompt hygiene red lines: Sections 1, 3, 8, 9, and 10.
- Eval, e2e, regression matrix, final verdicts, and batch-by-batch delivery evidence: Sections 14, 15, and 17.

Design updates after this point must keep the following full supplement shape in this document. A code fix without these fields is incomplete:

| Required block | Content that must be recorded | Hard rule |
| --- | --- | --- |
| Problem evidence | Local file/log/artifact refs, typed report fields, eval case ids, upstream issue/PR links when applicable | Do not use model prose as evidence for a hard claim |
| Scope boundary | Affected mode, stage, tool contract, artifact store, CLI/REPL surface, and unaffected modes | Read/log/trace/data/operation/computer isolation must be explicit |
| Generalized gap | The class of failures the evidence represents | Do not describe only the single failing fixture |
| Target architecture | Durable state, typed artifacts, action enums, policy gates, and handoff flow | No keyword matching over user intent or model output |
| Safety and approval | allow/ask/deny behavior, fingerprint/resume behavior, critical deny path | Low/medium stays low-friction; high asks; critical denies |
| Handoff plan | Which P0-P3 facts are produced, persisted, deduped, and consumed | Rich evidence must survive process restart and replan |
| Implementation tasks | Batch-sized task list with owners by package or surface | Each task needs tests and rollback/compat notes |
| Test matrix | Unit, controller, CLI/REPL, eval, and regression checks | PASS must be backed by typed artifacts where available |
| Progress ledger | Commit hash, push status, commands run, failures, and caveats | The ledger is updated per batch, not only at the end |

When a new external eval uncovers gaps, add an addendum here with:

- source matrix: repo, language, upstream issue/PR, fixture/case id, result.
- typed artifact summary: `ChangePlan`, `WriteApprovalRecord`, `ChangeReport`, workflow attempts, report/diff/surface refs.
- failure class: eval oracle, planner structured edit recovery, verify runner/report installation, workflow resume, handoff, permission/risk, or multi-repo fan-out.
- generalized design response: a system mechanism that handles the class of failures.
- delivery tasks and acceptance criteria.

### 18.1 Complete Design Supplement Contract

Every future design supplement must be self-contained enough for a new engineer
to continue implementation without reading chat history. The required payload is:

1. **Problem statement**
   - What user-visible or system-visible behavior is wrong or incomplete.
   - Why the behavior is a class of failure, not a one-off fixture problem.
   - Which existing stable paths must remain isolated.

2. **Evidence inventory**
   - Local files, line-backed code refs, eval artifacts, report JSON, run IDs,
     plan IDs, batch IDs, approval fingerprints, and command outputs.
   - Upstream issue/PR links when external cases are used.
   - Typed fields that support the claim: enums, booleans, reason codes,
     fingerprints, parser outputs, path resolver results, report channels.
   - Explicit note when evidence is missing or only inferred.

3. **Gap classification**
   - Severity: P0 blocks correctness/safety/commercial delivery; P1 causes
     repeated inefficiency or confusing recovery; P2 is polish or observability.
   - Surface: controller, planner, coder, verifier, safety policy, artifact
     store, handoff, CLI/REPL, eval harness, or documentation.
   - Failure family: state contradiction, missing typed authority, permission
     policy gap, lost evidence, brittle edit operation, verify infra failure,
     unsupported workspace topology, or UX/reporting ambiguity.

4. **Target architecture**
   - The durable DAG shape affected by the change: run, batch, edge, attempt,
     context pack, artifact refs, and derived state.
   - Controller actions involved and whether they are existing actions or new
     typed actions.
   - Which agent consumes or produces each artifact.
   - How the architecture generalizes beyond the observed eval case.

5. **State and artifact contract**
   - New or changed typed structs, enum values, reason codes, and artifact file
     names.
   - Persistence rules, resume behavior, cleanup boundaries, and stale-ref
     behavior.
   - Single source of truth for current state; progress ledger remains history,
     not state.

6. **Safety and approval contract**
   - allow/ask/deny outcome for the affected path.
   - Fingerprint rules, stale approval invalidation, and pending-approval resume
     behavior.
   - Critical deny behavior before mutation.
   - Explicit statement that hard gates consume only typed artifacts and parser
     outputs, never user keywords, model prose, summary, rationale, or
     `<think>`.

7. **Handoff contract**
   - Which P0/P1/P2/P3 facts are produced.
   - Stable dedupe identity for each fact type.
   - Consumer-specific ordering and Top-N bounds.
   - Evidence refs that must survive replan, restart, and worktree cleanup.
   - How stale facts are demoted or superseded by newer verify/risk evidence.

8. **Execution plan**
   - Batch-sized tasks with owning packages or surfaces.
   - Dependency order and rollback/compat notes.
   - Whether a task is product code, eval harness, fixture, docs, or test-only.
   - Commit/push expectation for each completed batch.

9. **Test and eval matrix**
   - Unit tests for typed policy/state/artifact behavior.
   - Controller tests for dynamic DAG decisions and recovery.
   - CLI/REPL tests for show/list/resume/approve/reject where applicable.
   - Eval cases proving the generalized class, not only the first failure.
   - Regression commands for affected packages and full-suite gates when scope
     warrants it.

10. **Acceptance and progress**
    - Commercial acceptance criteria written as observable outcomes.
    - Progress ledger row with commit hash, push status, commands run, failures,
      caveats, and remaining risk.
    - If implementation is intentionally deferred, the blocking condition and
      exact next executable task.

### 18.2 Current Supplement Backfill Map

The current document already backfills the requested design content as follows:

| Required content | Backfilled location | Remaining action |
| --- | --- | --- |
| Full write-mode architecture and dynamic DAG | Sections 6, 7, 12 | Keep updated when controller actions or run schema change |
| Approval minimization and high/critical gates | Sections 8, 13, 16 | Add new reason codes to the ledger when policy expands |
| Verify-result authority | Sections 5.1, 14, 19.1 P0-A | Re-run external sweep after Batches 8-10 |
| ModePlan terminal semantics | Sections 5.2, 7, 14 | Keep prompt hygiene tests aligned with action schema |
| Durable approval resume | Sections 5.3, 12, 13 | Expand if approval store schema changes |
| Failure evidence handoff | Sections 5.4, 11, 19.1 P0-B | Ensure new verifier outcomes create P2 facts |
| Single state machine | Sections 5.5, 12 | Use `DeriveBatchAttemptState` as the only rendered phase |
| Planner/replan permissions | Sections 5.6, 9, 10 | New planner tools require typed schema tests |
| Context pack dedupe and Top-N | Sections 5.7, 11 | New evidence kinds require stable IDs and view tests |
| Worktree/report persistence | Sections 5.8, 12, 17 | Report user-facing refs from durable artifacts |
| External issue evidence and new gaps | Section 19 | Continue appending eval addenda, not replacing history |
| Multi-repo write fan-out | Section 19.1 P0-D and Batch 11 | Still open; requires product design and eval support |

## 19. External GitHub Issue Eval Addendum

Date: 2026-06-14

Command:

```text
CODRAX_BIN=/Users/han/opt/codrax/codrax CASES='eval/cases/github_issue_libgit2_foreach_worktree.case eval/cases/github_issue_gson_lazy_number.case eval/cases/github_issue_dateutil_relativedelta_float.case eval/cases/github_issue_dayjs_duration_nan.case eval/cases/github_issue_zod_prefault.case eval/cases/github_issue_nlohmann_long_double.case eval/cases/github_issue_fmt_tm_year_overflow.case eval/cases/github_issue_commons_lang_random_ascii.case' PARALLEL=2 RUNS=1 TIMEOUT=1800 SUMMARY=eval/results/write_mode_github_issue_apply_20260614_baseline_summary.md bash eval/convergence_audit.sh
```

Aggregate result:

- 8 external GitHub-inspired write-mode apply cases.
- 5 PASS, 3 FAIL.
- Summary artifact: `eval/results/write_mode_github_issue_apply_20260614_baseline_summary.md`.

Source/result matrix:

| Case | Language | Upstream source | Result | Typed evidence |
| --- | --- | --- | --- | --- |
| `github_issue_libgit2_foreach_worktree` | C | libgit2/libgit2 issue #7216 / PR #7231 | FAIL by regex oracle | `plan-1781402164588833000-17983.report.json` has `channel=post_apply_verify`, `passed=true`, command `make check`, exit 0 |
| `github_issue_gson_lazy_number` | Java | google/gson issue 627 family | FAIL before apply | no plan written; planner `emit_change_plan` rejected by structured edit diagnostics: old-text mismatch then invalid line range |
| `github_issue_dateutil_relativedelta_float` | Python | python-dateutil relativedelta float handling issue family | PASS | apply completed |
| `github_issue_dayjs_duration_nan` | TypeScript/JavaScript | iamkun/dayjs duration NaN issue family | PASS | apply completed |
| `github_issue_zod_prefault` | TypeScript | colinhacks/zod issue #5824 / PR #5893 | FAIL by workflow/harness | plan applied; attempt diff persisted; first verify produced no `ChangeReport`; controller replanned over already-applied worktree; final verdict `worktree_discarded_or_missing` |
| `github_issue_nlohmann_long_double` | C++ | nlohmann/json long double formatting issue family | PASS | apply completed |
| `github_issue_fmt_tm_year_overflow` | C++ | fmtlib/fmt tm year overflow issue family | PASS | apply completed |
| `github_issue_commons_lang_random_ascii` | Java | apache/commons-lang RandomStringUtils ASCII issue family | PASS | `plan-1781402836694027000-27015.report.json` persisted |

### 19.1 P0 Gaps From External Eval

#### P0-A Write Eval Verdict Must Consume Typed Verify Results

Evidence:

- `libgit2` produced a typed post-apply verify report with `passed=true`, but the eval failed because the post-apply file did not match the case regex.
- Current `eval/run.sh` apply verdict primarily checks plan existence, apply exit, worktree presence, and content regex. It does not default to requiring an authoritative typed `ChangeReport` with `channel=post_apply_verify` and `passed=true`.

Generalized gap:

- The eval harness conflates three different judgments: workflow execution, typed verification, and content/oracle conformance.

Target design:

- Add a typed write eval result layer that records `plan_id`, `plan_written`, `apply_attempted`, `worktree_path`, `report_path`, `report_channel`, `report_passed`, `executed_commands`, `content_assertions`, and `oracle_assertions`.
- For `MODE=apply`, default PASS requires an authoritative current-plan post-apply `ChangeReport.passed=true` unless a case explicitly opts into `ALLOW_UNVERIFIED_APPLY=1`.
- Content regex failures remain separate `oracle_assertion_failed` reasons; they do not hide a passed typed verify report.

#### P0-B Verify Infra Failure Needs Typed Retry Semantics

Evidence:

- `zod` applied the correct diff and persisted `plan-1781402485278036000-22136.attempt-1.diff`.
- First verifier dispatch called `run_tests` late in the verifier loop, then no `ChangeReport` was installed.
- Controller converted the state to replan; planner saw the worktree already fixed, ran dry-run probes, then failed because no new plan was produced.

Generalized gap:

- A verifier infrastructure failure is not the same as a code verification failure. Replanning over already-applied code can destroy convergence.

Target design:

- Introduce a typed `VerifyAttemptOutcome` with kinds: `report_passed`, `report_failed`, `tool_not_called`, `report_parse_failed`, `runner_missing`, `timeout`, `no_tests`, `infra_error`.
- Controller maps retryable infra outcomes to `retry_verify`, not `replan_batch`.
- Replan is only allowed when typed `ChangeReport` exists and says code/test behavior failed, or when retry budget for infra verification is exhausted and the selected fallback is explicitly typed.
- Applied diff/report/surface refs remain attached to the active attempt so the retry consumes the same worktree state.

#### P0-C Planner Structured Edit Recovery Is Too Fragile

Evidence:

- `gson` planner found the correct change but failed to install a plan after two structured edit rejections:
  - old-text mismatch at anchor line.
  - invalid line range for file length.

Generalized gap:

- The structured edit validator provides enough local diagnostics to the model, but the planner still has to reason about line counts and EOF manually. This creates avoidable no-plan failures for simple append/insert changes.

Target design:

- Return typed `EditDiagnostic` artifacts with file length, anchor line, nearest exact old-text candidates, safe EOF insertion options, and validator reason codes.
- Add an append-before-final-brace / insert-at-EOF structured operation that is compiled by the builder from typed anchors, so the model does not manually calculate end-of-file line ranges.
- Keep validators strict; improve repair affordances rather than weakening old-text checks.

#### P0-D Multi-Repo Write Eval Is Unsupported

Evidence:

- `eval/run.sh` rejects `MULTIREPO` with `MODE=apply`: "multi-repo write-mode not yet supported".

Generalized gap:

- Write-mode DAG can split batches inside one repo, but the eval harness and product path do not yet support repo-scoped write fan-out across a discovered multi-repo workspace.

Target design:

- Model multi-repo write as repo-scoped workflow batches with explicit `repo_slug`, per-repo worktree, per-repo apply-pre gate, and cross-repo dependency edges.
- Shared `WriteContextPack` stores cross-repo contract evidence, but hard apply/verify gates stay repo-local.
- Read-mode multi-repo discovery remains unchanged.

### 19.2 P1 Gaps From External Eval

- Eval needs separate verdict channels: workflow execution, typed verification, code-shape oracle, and upstream-conformance oracle.
- Workflow state rendering still exposes stale `pending_approval` wording after auto approval; derived state should render `approved_auto` or equivalent.
- Worktree/report persistence should be reported from durable typed refs, not grep over `worktree_path` in a plan snapshot.
- Planner no-plan after "already applied" state should become a typed no-op/verify decision when the active worktree satisfies the plan, not a generic planner failure.

### 19.3 Follow-Up Sweep After Batches 8-10

Date: 2026-06-14

Command:

```text
CODRAX_BIN=/Users/han/opt/codrax/codrax CASES='eval/cases/github_issue_libgit2_foreach_worktree.case eval/cases/github_issue_gson_lazy_number.case eval/cases/github_issue_dateutil_relativedelta_float.case eval/cases/github_issue_dayjs_duration_nan.case eval/cases/github_issue_zod_prefault.case eval/cases/github_issue_nlohmann_long_double.case eval/cases/github_issue_fmt_tm_year_overflow.case eval/cases/github_issue_commons_lang_random_ascii.case' PARALLEL=2 RUNS=1 TIMEOUT=1800 SUMMARY=eval/results/write_mode_github_issue_apply_20260614_after_b8_b10_summary.md bash eval/convergence_audit.sh
```

Aggregate result:

- 7 PASS, 1 FAIL.
- Recovered former failures:
  - `libgit2`: PASS after typed verify/oracle separation.
  - `gson`: PASS after structured edit diagnostics and safer insertion kinds.
  - `zod`: PASS after verify infra retry keeps the applied worktree state.
- Remaining fail:
  - `commons-lang`: FAIL after 559s with `worktree_discarded_or_missing write_report_missing no_regex_match:0x(4e00|370|400)`.

Typed evidence for the remaining fail:

- First plan `plan-1781405976909328000-66803` applied production guards but used `0x80` in the letter regression; authoritative `post_apply_verify` report failed with `missing non-ASCII letter regression test`.
- Later replan produced and applied `plan-1781406268118757000-67879`, then planner dry-run `run_tests(dry_run=true)` passed against the current worktree.
- The planner correctly concluded the current worktree satisfied the verification script, but could not emit a typed "no new code plan, reverify existing applied worktree" signal. The stage ended with `no change plan was produced this round`, so the final plan/report refs became inconsistent.

Generalized gap:

- Replan is currently overloaded: it means either "produce a new ChangePlan" or "inspect the current applied worktree and discover the failure is already resolved." The second case needs a typed scheduler lane, not a planner prose conclusion.

Target design:

- Add a scheduler-only `planner_probe_passed_existing_worktree` transition:
  - Input: latest `PlanStageProbeReports` item with `channel=planner_probe` and `passed=true`.
  - Guard: active batch has typed verify-failure handoff and a prior applied plan with applied commit/worktree/change apply evidence.
  - Action: restore the prior applied plan into `Mutable.ChangePlan`, set batch state to `verifying`, and require a post-apply verify report before finish.
  - Non-action: do not mark the planner probe itself as authoritative success.

### 19.4 Recovery Ref Eval Source Gap

Evidence:

- After adding the replan/no-op scheduler lane, `commons-lang` run `eval/results/github_issue_commons_lang_random_ascii-20260614-111054` produced an authoritative current-plan report:
  - `run-1.write-apply.json`: `verify_authoritative=true`, `report_channel=post_apply_verify`, `report_passed=true`, `report_plan_id=plan-1781406980706382000-79709`.
  - `run-1.plan.json`: `applied_commit_sha=66704d51552af6cc8b11d6f6f3b310be93ee0147`, `worktree_path=/Users/han/opt/codrax/.codrax/worktrees/trace-1781406816621107000-79709`.
  - `run-1.repo` preserved `refs/codrax/applied/plan-1781406980706382000-79709`.
- The eval still failed with `worktree_discarded_or_missing no_regex_match:0x(4e00|370|400)` because the harness read content from a missing live worktree or scratch repo instead of materializing the durable applied commit.

Generalized gap:

- Eval oracle assertions must use durable write artifacts after L5 cleanup. A successful write-mode apply may discard the live worktree by design; content assertions should read the recovery ref or applied commit, not require a live directory.

Target design:

- `eval/run.sh` uses `worktree_path` when available.
- If worktree is gone, `eval/runner_lib.sh` materializes `applied_commit_sha` or `refs/codrax/applied/<plan-id>` into `run-N.applied-tree`.
- POST_APPLY_FILE and content regex assertions run against that materialized source.
- Missing worktree is a failure only when neither live worktree nor durable applied commit is available.

Follow-up evidence after Batch 15:

- `commons-lang` run `eval/results/github_issue_commons_lang_random_ascii-20260614-112955` produced authoritative success: `report_channel=post_apply_verify`, `report_passed=true`, `verify_authoritative=true`.
- The materialized `run-1.applied-tree` contained both the fixed main source and non-ASCII regression tests.
- The verdict still failed with `too_short:0chars` because `EXPECT_MATCHES_REGEX` without `POST_APPLY_FILE` tried `git ls-files` inside the materialized archive tree. That tree is intentionally not a git worktree, so the oracle source string was empty.

Target design extension:

- Apply-mode eval source aggregation must support both live git worktrees and archive-materialized trees.
- When `POST_APPLY_FILE` is unset, aggregate deterministic text from the applied source tree rather than falling back to stdout.
- This is an eval harness correctness issue; typed write-mode apply/verify artifacts remain the authority for product success.

### 19.5 Pending Approval / Auto-Apply Eval Gap

Evidence:

- `commons-lang` run `eval/results/github_issue_commons_lang_random_ascii-20260614-111952` failed with `worktree_discarded_or_missing write_report_missing no_regex_match:0x(4e00|370|400)`.
- Logs show the workflow blocked before apply with: `write workflow blocked: pending_approval: change_plan 状态为 pending_approval，需要先获得批准才能发出 apply_plan`.
- The plan changed production and test files and remained pending approval in a non-interactive eval apply run, so no worktree/report was produced.

Generalized gap:

- Non-interactive `MODE=apply --auto-apply` needs a typed distinction between real high-risk `ask` and low/medium plans whose status label still says `pending_approval` before the apply-pre hook writes the auto approval record. Otherwise eval/CLI users see a blocked write for a plan the risk policy should auto-execute.

Target design:

- Keep apply-pre as the final authority, but add a scheduler preview that uses the same typed `AssessWriteRisk + DecideWriteApproval` policy before honoring a controller `ask_user`.
- If the controller emits typed `action=ask_user` while the current plan is still ready-to-apply and the deterministic approval decision is `auto_execute`, normalize the action to `apply_plan` and record an audit progress event.
- Preserve real pauses: high-risk/manual approval records, stale fingerprints, denied/blocked plans, missing plans, and already-applied or verify-failed plans must not be auto-converted.
- The hard route must not parse `reason`, `reason_code`, user intent keywords, model rationale, or `<think>` text. It only reads typed action, plan lifecycle status, approval record/fingerprint, and typed risk policy.

### 19.6 Addendum Delivery Tasks

#### Batch 7: External Eval Evidence Ledger

- [x] Run the 8-case GitHub issue write-mode apply sweep.
- [x] Inspect failing case artifacts and classify by typed evidence.
- [x] Record the result matrix, P0/P1 gaps, and generalized target design in this document.
- [x] Commit and push this document update.

#### Batch 8: Typed Write Eval Verdict

- [x] Add a structured apply-result collector in `eval/run.sh` / `eval/runner_lib.sh` that reads typed report JSON and emits `run-N.write-apply.json`.
- [x] Require authoritative current-plan `post_apply_verify` `ChangeReport.passed=true` by default for `MODE=apply`.
- [x] Preserve content regex checks as oracle assertions with distinct failure reasons.
- [x] Add harness tests for report passed, report missing, report failed, plan mismatch, and run.sh pass/missing-report smoke cases.
- [x] Verification: `bash -n eval/run.sh`, `bash -n eval/runner_lib.sh`, `bash -n eval/runner_lib_test.sh`, `bash eval/runner_lib_test.sh`.

#### Batch 9: Verify Infra Retry

- [x] Add `VerifyAttemptOutcome` for typed scheduler-facing verify outcomes: passed report, failed report, no report/tool-not-called, runner missing, no tests.
- [x] Add internal scheduler retry for missing `ChangeReport` verify infra failures; this does not require a model action and does not parse prose.
- [x] Keep applied worktree/plan state active across infra verify retry; record `infra_error` verify attempts and progress events.
- [x] Block typed `runner_missing` reports instead of replanning code.
- [x] Tests: `TestClassifyVerifyAttemptOutcome`, `TestRunWriteControllerWorkflow_VerifyInfraFailureRetriesVerify`, `TestRunWriteControllerWorkflow_VerifyInfraBudgetBlocksWithoutReplan`, `TestRunWriteControllerWorkflow_RunnerMissingBlocksWithoutReplan`.
- [x] Verification: `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/writeflow -run TestClassifyVerifyAttemptOutcome`; `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/orchestrator -run 'TestRunWriteControllerWorkflow_(VerifyInfraFailureRetriesVerify|VerifyInfraBudgetBlocksWithoutReplan|RunnerMissingBlocksWithoutReplan)'`; `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/orchestrator ./internal/writeflow ./internal/types ./internal/agent ./internal/tool ./internal/repl`.

#### Batch 10: Structured Edit Diagnostics

- [x] Extend structured edit rejection records with diagnostic JSON for invalid line ranges and old_text/anchor mismatches.
- [x] Add `insert_at_eof` and `insert_before_final_brace` structured edit kinds compiled by the builder without manual line-count dependence.
- [x] Keep old_text/range validators strict; diagnostics provide safe edit alternatives rather than weakening validation.
- [x] Update `emit_change_plan` / `emit_plan_change` schemas and planner soft guidance.
- [x] Tests: structured diagnostic cases, EOF append, final-brace append, existing structured edit compile/apply/finalize cases.
- [x] Verification: `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/tool -run 'TestCompileStructuredEdits|TestEmit(ChangePlan|PlanChange)_StructuredEdits|TestApplyPatch_StructuredEditsOnlyPlan'`; `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/skill -run TestChangePlanSkill_BatchLocalPlanningWorkflow`; `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/tool ./internal/types ./internal/skill ./internal/agent ./internal/orchestrator`.

#### Batch 11: Multi-Repo Write Fan-Out Design And Eval

- Extend eval setup to allow `MULTIREPO + MODE=apply` behind an explicit case flag once product support exists.
- Add repo-scoped workflow batch metadata and per-repo worktree/apply/verify refs.
- Add one cross-repo fixture where a contract change touches two non-Go repos and must verify both.

#### Batch 12: External Issue Fixture Expansion

- [x] Add at least one Rust-shaped external issue fixture using an upstream PR with a small localized fix, with a Makefile/Python oracle so local Rust toolchain absence does not block eval.
- Keep all fixtures minimal reproductions; do not vendor external repos or rely on network during eval.
- Run the external issue sweep after Batches 8-10 and update this ledger with pass/fail deltas.

#### Batch 13: Replan Probe Pass Restores Existing Applied Plan

- [x] Add scheduler typed transition for `planner_probe_passed_existing_worktree`.
- [x] Trigger only from typed planner probe reports, verify-failure handoff, and applied-plan evidence.
- [x] Restore the prior applied plan and move the batch back to `verifying`; do not treat planner probe as authoritative success.
- [x] Tests: `TestRunWriteControllerWorkflow_ReplanProbePassRestoresAppliedPlanForVerify`.
- [x] Verification: `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/orchestrator -run 'TestRunWriteControllerWorkflow_ReplanProbePassRestoresAppliedPlanForVerify|TestRunControllerPlanBatch_NoPlanReplanRoundGetsOneRetry|TestRunControllerPlanBatch_NoPlanWithoutHandoffStaysTerminal'`; `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/orchestrator ./internal/writeflow ./internal/types ./internal/agent ./internal/tool ./internal/repl`.

#### Batch 14: Eval Recovery Ref Materialization

- [x] Add `eval_materialize_write_apply_source` to materialize `applied_commit_sha` or `refs/codrax/applied/<plan-id>` when the live worktree was cleaned up.
- [x] Route POST_APPLY_FILE and content regex checks through the materialized source.
- [x] Keep authoritative typed report checks unchanged.
- [x] Tests: fake write-mode eval where worktree is gone but recovery ref exists.
- [x] Verification: `bash -n eval/run.sh`; `bash -n eval/runner_lib.sh`; `bash -n eval/runner_lib_test.sh`; `bash eval/runner_lib_test.sh`.

#### Batch 15: Pending Approval State Normalization

- [x] Normalize controller `ask_user` pauses through typed approval preview instead of raw `pending_approval` status.
- [x] Add tests where low/medium auto-safe plans do not block non-interactive apply.
- [x] Keep existing true high-risk/manual approval test green.
- [x] Verification: `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/orchestrator -run 'TestRunWriteControllerWorkflow_(AutoExecutableAskUserAppliesPlan|PendingApprovalKeepsRunActive)'`; `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/orchestrator ./internal/writeflow ./internal/types ./internal/agent ./internal/tool ./internal/repl`.
- Re-run `github_issue_commons_lang_random_ascii` and then the 8-case external issue sweep.

#### Batch 16: Eval Applied Tree Aggregation

- [x] Add `eval_collect_apply_source_text` for git worktrees and non-git materialized trees.
- [x] Route apply-mode oracle checks without `POST_APPLY_FILE` through that helper.
- [x] Add recovery-ref runner test where `POST_APPLY_FILE` is unset.
- [x] Verification: `bash -n eval/run.sh`; `bash -n eval/runner_lib.sh`; `bash -n eval/runner_lib_test.sh`; `bash eval/runner_lib_test.sh`.
- Re-run `github_issue_commons_lang_random_ascii` after this harness fix.

#### Batch 17: Verify Surface Suite Isolation

- Evidence: `commons-lang` run `eval/results/github_issue_commons_lang_random_ascii-20260614-114008` produced a failed post-apply report where `test_surface.selected_id=make@.` and `command=make check`, but executed commands were `mvn -B -q test` followed by `make org.apache.commons.lang3.RandomStringUtilsTest`.
- Generalized gap: `run_tests` queued TestSurface escalation plans, but command construction reused the original LLM-supplied suite for every queued runner. A Java class-name suite leaked into the make candidate and bypassed the typed `MakeTarget=check`.
- [x] Move suite ownership into each `runnerPlan`: LLM-selected plans carry `p.Suite`; TestSurface escalation plans carry their own typed target, or empty suite when no target applies.
- [x] Execute commands from `plan.Suite`, not the global request suite.
- [x] Add regression test where fake `mvn` is missing, make `check` exists, and suite must not leak into the make candidate.
- [x] Verification: `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/tool -run 'TestRunTests_(RunnerMissingEscalationDoesNotLeakSuiteToSurfaceCandidate|ZeroTestChoiceEscalatesToSurfaceCandidate|EscalatedCandidateFailureFailsVerdict|AutoDetectDoesNotEscalate)|TestBuildTestSurface|TestNextTestSurfaceEscalation'`; `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/tool ./internal/orchestrator ./internal/writeflow ./internal/types ./internal/agent ./internal/repl`.

#### Batch 18: Eval Source Root Boundary

- Evidence: `zod` run `eval/results/github_issue_zod_prefault-20260614-123327` had `run-1.write-apply.json` with `report_channel=post_apply_verify`, `report_passed=true`, and `verify_authoritative=true`, but verdict failed with `too_short:0chars`. The materialized `run-1.applied-tree` contained the fixed source and tests.
- Generalized gap: `eval_collect_apply_source_text` treated any directory inside the Codrax repo as a git worktree because `git -C <source> rev-parse --is-inside-work-tree` returns true for nested non-git archive trees. `git ls-files` then listed Codrax-tracked files, not the materialized applied tree, yielding empty oracle input.
- [x] Treat a source as a git worktree only when `git rev-parse --show-toplevel` physically equals the source directory.
- [x] Fall back to deterministic file traversal for non-git materialized trees, even when nested under another git repo.
- [x] Add runner contract tests for a non-git archive tree inside a parent git repo and for a real git-root source preserving tracked-file semantics.
- [x] Verification: `bash -n eval/run.sh`; `bash -n eval/runner_lib.sh`; `bash -n eval/runner_lib_test.sh`; `bash eval/runner_lib_test.sh`; `github_issue_zod_prefault` single-case recheck PASS in `eval/results/write_mode_zod_prefault_after_b18_20260614_summary.md`.

#### Batch 19: Off-Scope High-Risk Replan

- Evidence: `commons-lang` run `eval/results/github_issue_commons_lang_random_ascii-20260614-125620` produced a plan touching `Makefile`, which deterministic risk policy correctly classified as `build_or_dependency_manifest` high risk. The write IR scope anchors were only source and test files, and `changes_build_system=false`, so non-interactive apply stopped at manual approval even though the high-risk path was planner drift.
- Generalized gap: approval minimization needs a typed pre-approval critique lane for high-risk paths outside the current batch scope. The system must not auto-approve high-risk build/config changes, but it should first ask the planner to re-scope when high risk is introduced outside typed scope.
- Target design implemented: after a plan is produced and before pending approval is surfaced, the scheduler computes `AssessWriteRisk + DecideWriteApproval`. If all high-risk reasons are path-backed, off-scope relative to `WriteAnalysisIR.scope_anchors`, and the IR does not declare build-system/config work, the scheduler performs one bounded internal replan with a typed plan-risk critique hint. True build-system/config tasks, in-scope high-risk paths, critical paths, empty paths, stale approvals, and denied plans retain the normal approval/deny behavior.
- Hard inputs only: `WriteAnalysisIR.scope_anchors`, `WriteAnalysisIR.risk.changes_build_system`, `WriteTask.kind`, `ChangePlan` paths, `RiskAssessment` reason levels/paths, and `ApprovalDecision.action`. The lane does not parse user keywords, model prose, summary, rationale, or `<think>`.
- [x] Add scheduler helper for off-scope high-risk path detection and one-shot replan hinting.
- [x] Add tests for off-scope `Makefile` drift replanning and for typed build-system changes not being internally replanned.
- [x] Verification: `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/orchestrator -run 'TestRunWriteControllerWorkflow_ReplansOffScopeHighRiskBuildManifest|TestRunControllerPlanBatch_KeepsTypedBuildSystemChangeForApproval'`; `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/orchestrator ./internal/writeflow ./internal/types ./internal/agent ./internal/tool ./internal/repl`; rebuilt `codrax`; `github_issue_commons_lang_random_ascii` single-case recheck PASS in `eval/results/write_mode_commons_lang_offscope_replan_20260614_summary.md`.

#### Batch 20: External Fixture Oracle Robustness

- Evidence: after Batch 19, full 8-case sweep `eval/results/write_mode_github_issue_apply_20260614_after_b19_summary.md` reported 6/8 PASS. The two failures both had authoritative typed post-apply reports with `passed=true`: `libgit2` failed only because content regex required the exact upstream parenthesized assignment style, while the applied source used an equivalent split assignment and check; `commons-lang` failed only because the CJK hex oracle accepted `0x4e00` but not `0x4E00`, while the fixture's own Python checker uses `re.IGNORECASE`.
- Generalized gap: external issue fixtures must verify semantic equivalence, not a single textual implementation shape. Content regex is an oracle assertion layer, not the product authority, and must not reject code that the fixture's typed verification command accepts for the same upstream behavior.
- [x] Broaden `libgit2` content oracle to accept both upstream parenthesized assignment and equivalent split assignment/check forms.
- [x] Align `commons-lang` CJK hex oracle with the fixture checker by accepting `0x4e00` and `0x4E00`.
- [x] Keep typed post-apply verification as the product authority; content assertions remain supplemental oracle checks.
- [x] Verification: targeted recheck `eval/results/write_mode_github_issue_oracle_recheck_20260614_summary.md` reports PASS for `github_issue_libgit2_foreach_worktree` and `github_issue_commons_lang_random_ascii`, flagged 0/2.
- [x] Implementation pushed in code commit `506d68cd` (`write-mode: harden external issue convergence`).

#### Batch 21: Rust External Issue Fixture And Exploration Handoff Hygiene

- Evidence source: chronotope/chrono PR #1385 (`https://github.com/chronotope/chrono/pull/1385`), Rust `Duration` minimum millisecond bound fix. The upstream final position keeps `-i64::MAX` as the lower bound, introduces `Duration::try_milliseconds(milliseconds: i64) -> Option<Duration>`, and makes `Duration::milliseconds()` panic on invalid input.
- Fixture/case: `eval/fixtures/github_issues/chrono_duration_min` and `eval/cases/github_issue_chrono_duration_min.case`.
- First run evidence: `eval/results/write_mode_chrono_duration_min_20260614_summary.md` failed with `write_report_failed`; `run-1.write-apply.json` had `report_channel=post_apply_verify`, `report_passed=false`, and `verify_authoritative=false`. The reports showed the planner initially chose `Result<Duration, _>` and then produced a test insertion that nested `#[test]` inside another test. This exposed a fixture oracle gap: the checker was too narrow on Rust function-body parsing, yet too weak on recursive MIN/MAX and test structure.
- Generalized fixture response: the checker now parses function bodies with brace matching, accepts Rust shorthand struct construction, enforces upstream API-family `Option<Duration>`, enforces `-i64::MAX` / `i64::MAX` constructor bounds, rejects recursive `MIN = Duration::milliseconds(...)` / `MAX = Duration::milliseconds(...)`, and checks test brace structure. These are structural source assertions, not model-prose or keyword hard routes.
- Product gap observed: the passing run still had `unavailable_tool_attempts=1` because write exploration tried to use an unavailable shell/write path after discovering the fix shape. Hard runtime policy blocked the call, but exploration prompt guidance was not explicit enough that implementation must be handed off to planner instead of attempted in explore.
- Generalized product response: `renderExplorerWriteExplorationRequest` now states the subflow is read-only, must not call shell commands, must not implement fixes in explore, and must record repair shape through evidence/completion for controller/planner handoff. This is soft guidance only; hard safety remains the existing stage tool allowlist and write exploration read-only runtime policy.
- Passing run evidence: `eval/results/write_mode_chrono_duration_min_after_oracle_20260614_summary.md` reports PASS 1/1 and flagged 0/1. `eval/results/github_issue_chrono_duration_min-20260614-144056/run-1.write-apply.json` has `plan_written=true`, `apply_attempted=true`, `report_channel=post_apply_verify`, `report_passed=true`, and `verify_authoritative=true`. `plan-1781419447924857000-99486.report.json` records `make check` exit 0 through runner-missing escalation from unavailable cargo to the fixture Makefile.
- [x] Add Rust external issue case and offline fixture.
- [x] Harden the fixture oracle to encode upstream structural contract without relying on exact patch text.
- [x] Add write exploration read-only handoff prompt hygiene test.
- [x] Verification: `bash -n eval/cases/github_issue_chrono_duration_min.case`; `PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache python3 -m py_compile eval/fixtures/github_issues/chrono_duration_min/tests/check_duration_min.py`; seed fixture `make check` fails as expected before a fix; `CODRAX_BIN=/Users/han/opt/codrax/codrax CASES='eval/cases/github_issue_chrono_duration_min.case' PARALLEL=1 RUNS=1 TIMEOUT=1800 SUMMARY=eval/results/write_mode_chrono_duration_min_after_oracle_20260614_summary.md bash eval/convergence_audit.sh`; `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/agent`; `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache make`.

## 20. Direct-Build Commercial Design Supplements

Date: 2026-06-14

This section records the complete design supplement required by the latest audit pass. It is intentionally system-level: hard routes consume typed artifacts only, prompts remain soft guidance, and no user-intent keyword, model rationale, summary prose, or `<think>` content may drive scheduler or safety decisions.

### 20.1 P0 Contracts

- Unified verify result: the controller consumes only typed package/build/test verdicts from `ChangeReport`, `ExecutedCommand`, and `TestSurface`. A verifier narrative that says a subtest passed cannot mark a batch successful unless the typed post-apply report is authoritative, current-plan, and passed.
- ModePlan terminal semantics: write planning ends after the bounded plan is persisted. It must not continue into apply decisions, approval routing, or verifier scheduling. Apply remains a distinct user-selected phase or controller action.
- Durable workflow and approval store: any `pending_approval` state must be visible through `/workflow show`, `/workflow list`, and `/workflow resume`. `/approve` must continue the same run/batch using the same plan fingerprint; fingerprint drift requires a fresh approval decision.
- Failure evidence handoff: build errors, test failures, file paths, line numbers, command metadata, parser outcomes, and artifact refs enter P2 context. Replan consumes that focused evidence and targets the failing point in a small batch instead of rediscovering the whole repository.
- No false verify authority: `runner_missing`, `zero_tests`, parser errors, missing reports, stale worktree roots, non-current plan reports, and planner dry-run probe passes are typed dead ends unless a later authoritative post-apply report passes.

### 20.2 P1 Contracts

- Single state machine: plan lifecycle, approval lifecycle, apply state, verify state, and workflow batch state must not contradict each other. The scheduler is the only writer of cross-stage state transitions; agents emit typed proposals, not final state mutations.
- Planner/replan permission layering: planning agents can read and emit bounded plans. They should not run generic `exec_command`; dry-run needs must go through typed dry-run test/build tools with read-only policy and scoped artifacts.
- Context pack dedupe and Top-N consumption: P0-P3 context is stored once with stable evidence refs. Controller, planner, verifier, and approval policy each receive a role-specific Top-N projection while the full pack remains durable for resume and audit.
- Worktree/report persistence: user-visible prompts, eval harnesses, and resume paths must refer to durable plan/report/apply refs, not live worktree paths that L5 cleanup may remove.
- Approval minimization: low/medium typed risk auto-executes; high risk pauses for approval; critical risk denies. The system may internally replan high-risk off-scope drift once, but must not auto-approve real high-risk changes.

### 20.3 Test Authority Design

Verification has three layers, in descending authority:

1. Current-plan post-apply `ChangeReport` with typed `Passed=true`, non-stale plan id, and executed command evidence.
2. Typed dead-end recovery: missing runner, parser zero-tests, synthetic no-tests, or unavailable test infrastructure may queue a bounded next `TestSurface` candidate.
3. Fail-loud terminal report: if typed test work exists but all candidates end in dead ends, produce a failed `ChangeReport` with durable command/output refs.

Rules:

- A parser-produced `NoTestsRunners` report with zero `TestResults` is not authoritative when the selected `TestSurfaceCandidate.HasTestSignal=true`.
- Dead-end recovery is bounded, loop-safe, and keyed by `(runner, framework, working_dir)`; it never retries the same candidate.
- A synthetic no-test pass remains allowed only when the executor's typed pre-flight says there is no test work and no unexecuted runnable candidate exists.
- Test discovery for repo-scoped multi-repo write must be anchored to the active verification tree. If `ActiveSubRepo.RootAbs` is outside the current `RepoRoot` worktree, it is stale and must not be used as `walkRoot`.

### 20.4 Durable Handoff Design

P2 verify-failure context must include:

- command, runner, framework, working directory, exit code, duration, and source (`llm_choice`, `runner_missing_escalation`, `zero_tests_escalation`, `auto_detect`, etc.);
- structured test/build failures with assertion id, suite, file, line, and detail when available;
- parser outcomes including `zero_tests`, parser errors, missing report, and runner missing;
- plan id, applied commit/ref, worktree path, report path, and whether the live worktree was cleaned up;
- deduplicated evidence refs ranked before raw log excerpts.

Consumers:

- controller consumes batch status, typed verify outcome, approval/risk records, and compact P0/P2 summaries;
- planner consumes P0 constraints, P1 target scope, and P2 failure evidence for small replan;
- verifier consumes plan/apply refs, test surface, and latest failure evidence;
- user-facing `/workflow` commands render concise state plus artifact refs without hiding `<think>` transparency logs.

### 20.5 Remaining Delivery Tasks

- [ ] P0 verify authority audit: assert every controller finish path requires current-plan authoritative post-apply success or a typed terminal block.
- [ ] P0 ModePlan audit: ensure plan phase persists a plan and exits without scheduling apply/approval/verify.
- [ ] P0 workflow approval resume: make pending approval durable across process restart and let `/approve` continue the same run/batch.
- [ ] P0 failure evidence projection: move build/test failure path/line/command metadata into P2 context packs and consume them in replan prompts/tools.
- [ ] P1 single-state-machine audit: add invariant tests that plan status, approval record, apply refs, verify refs, and batch status cannot disagree.
- [ ] P1 planner/replan permissions: route dry-run needs through typed dry-run tools, not generic planning-phase exec.
- [ ] P1 context pack dedupe: add stable hash/evidence-ref dedupe and role-specific Top-N projections.
- [ ] P1 worktree/report UX: update CLI/REPL/eval prompts to prefer durable refs over disposable live paths.

#### Batch 22: Repo-Scoped Multi-Repo Verify Surface And Zero-Test Authority

- Evidence source: Hugging Face tokenizers issue #1534 (`https://github.com/huggingface/tokenizers/issues/1534`), reconstructed as a repo-scoped Python binding case inside `eval/fixtures/multirepo-polyglot`.
- First passing eval still exposed a typed authority gap: post-apply verify selected `python/pytest@.`, escalated to `python/unittest@.`, and accepted `Ran 0 tests ... OK` as authoritative success while a typed Makefile `check` candidate existed.
- The same report showed a scoped worktree gap: `ActiveSubRepo.RootAbs` could remain pointed at the original scratch sub-repo after apply, so `BuildTestSurface` could discover candidates outside the current worktree.
- Follow-up run `eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260614-155436` passed with `report_channel=post_apply_verify`, `report_passed=true`, and the expected typed command chain `python/pytest runner_missing -> python/unittest zero_tests -> make check executed`. It also exposed an eval/product contract gap: the generated plan reduced the existing five-newline regression input to four newlines. The implementation still handled five newlines correctly, but the test delta weakened the fixture's explicit odd-run contract.
- Final hardened run `eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260614-160324` passed after the case encoded the five-newline input as an explicit current-task constraint and source-tree oracle. `run-1.write-apply.json` records `report_channel=post_apply_verify`, `report_passed=true`, and `verify_authoritative=true`; the report records `python/pytest runner_missing -> python/unittest zero_tests -> make check executed`; the applied `tests/test_tokenizer.py` still contains the five-newline literal.
- Generalized target:
  - parser-level zero-tests with `HasTestSignal=true` is a typed dead end, not success;
  - surface escalation is bounded multi-hop, not a single fallback;
  - if no runnable candidate remains, zero-tests becomes a failed `ChangeReport`;
  - repo-scoped verify ignores stale `ActiveSubRepo.RootAbs` outside current `RepoRoot`.
  - external issue fixtures distinguish product verification from regression-contract preservation; a passing test suite is not enough when the plan weakens an existing regression input.
- [x] Add repo-scoped multi-repo write setup in eval and a Python tokenizers newline-run issue case.
- [x] Scope write-mode multi-repo execution to exactly one active sub-repo and reject parent-prefixed target paths in scoped plans.
- [x] Suppress parent multi-repo path guidance once write mode has been scoped to a single active sub-repo.
- [x] Change `run_tests` from single fallback to bounded typed `TestSurface` multi-hop escalation.
- [x] Treat parser zero-tests as a typed dead end when the candidate has test signal; fail loud when no candidate remains.
- [x] Ignore stale `ActiveSubRepo.RootAbs` during verify when it is outside the current worktree `RepoRoot`.
- [x] Strengthen the tokenizers eval oracle to aggregate the applied source tree and require the existing five-newline odd-run regression input to remain present.
- [x] Verification: `bash -n eval/cases/github_issue_tokenizers_newline_run_multirepo_py.case`; `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/tool ./internal/orchestrator ./internal/context`; `CODRAX_BIN=/Users/han/opt/codrax/codrax CASES='eval/cases/github_issue_tokenizers_newline_run_multirepo_py.case' PARALLEL=1 RUNS=1 TIMEOUT=1800 SUMMARY=eval/results/write_mode_tokenizers_newline_multirepo_py_after_zero_test_fix_20260614_summary.md bash eval/convergence_audit.sh` PASS.
- [x] Tests: multi-hop pytest missing -> unittest zero-tests -> make check, zero-tests no fallback fail-loud, scoped write path-prefix rejection, stale ActiveSubRepo walkRoot.
- [x] Product follow-up: add a typed test-contract critic that detects removal or weakening of pre-existing regression assertions when the user/task marks them as preserved coverage. Inputs must be structured diff hunks, path roles, and typed expected outcomes; it must not route on natural-language rationale or keyword matching.

#### Batch 23: Typed Test-Contract Critic

- Evidence source: the hardened tokenizers eval showed the product could pass typed post-apply verification while a prior plan attempted to reduce an existing five-newline regression input to four newlines. Eval oracle now catches this case, but production needs a typed pre-apply replan lane when the write analyzer marks an existing regression test as protected.
- Generalized target:
  - hard trigger reads only `WriteAnalysisIR.constraints[].kind == preserve_regression_test`, `constraints[].target`, and structured `ChangePlan` deltas;
  - it does not parse user request text, constraint note prose, plan summary, plan rationale, verifier narrative, `<think>`, or log prose;
  - unified-diff deletions, structured-edit replace/delete ranges, full-file modify deletions, and file deletes are all projected to exact removed snippets;
  - controller grants one bounded internal replan with a typed test-contract critique before apply; it does not permanently block legitimate test refactors.
- [x] Add `testContractReplanHint` to the controller planning loop before approval/apply.
- [x] Add write-analyzer schema soft guidance for canonical `preserve_regression_test` constraint kind.
- [x] Add regression tests proving protected regression test weakening triggers replan and note-only natural-language constraints do not trigger hard critic.
- [x] Verification: `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/orchestrator -run 'TestRunWriteControllerWorkflow_(ReplansProtectedRegressionTestWeakening|ReplansOffScopeHighRiskBuildManifest)|Test(TestContractReplanHintDoesNotParseConstraintNote|RunControllerPlanBatch_KeepsTypedBuildSystemChangeForApproval)'`.

#### Batch 24: Controller Typed-State Progress Barrier

- Evidence source: `eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260614-161735` failed with `worktree_discarded_or_missing` and `write_report_missing`. The log shows the controller repeatedly chose `replan_batch` after a current ChangePlan was already auto-approved and again after apply had produced work but before post-apply verify produced a verdict.
- Generalized gap:
  - controller action schema is typed, but the outer scheduler still allowed semantically stale actions for the current typed batch state;
  - `ChangePlan.Status` remains `pending_approval` until post-apply verify syncs final status, so a ready-plan guard must read ordered workflow attempt records, not plan status alone;
  - repeated planning after a current auto-executable plan or an applied-but-unverified plan wastes budget and can discard useful worktree state before an authoritative report exists.
- Generalized target:
  - the scheduler normalizes only from typed artifacts: current `ChangePlan`, approval/risk decision, active batch id, ordered apply/verify attempts, and mode;
  - no user-request keywords, model rationale, `<think>`, verifier prose, or log text drive the hard route;
  - if a pending plan can proceed without manual approval, controller actions that would delay it are converted to `apply_plan` unless the controller emits `block`;
  - if the active plan has an `apply` attempt with status `applied` and no later verify attempt for that plan, controller actions that would delay verification are converted to `verify_batch` unless the controller emits `block`;
  - progress ledger records `ready_plan_action_overridden` / `post_apply_verify_action_overridden` so `/workflow` and eval artifacts can explain the state-machine correction.
- [x] Add `normalizeControllerTypedStateDecision` to enforce ready-plan and post-apply-verify barriers.
- [x] Preserve the existing `ask_user_auto_executable_overridden` progress reason for low/medium risk auto-executable ask-user suppression.
- [x] Add controller tests for repeated planning before apply and repeated planning after apply-before-verify.
- [x] Verification: `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/orchestrator -run 'TestRunWriteControllerWorkflow_(AppliesReadyPlanBeforeRepeatedPlanningDecision|VerifiesAppliedPlanBeforeRepeatedPlanningDecision|ExplorePlanFinish|ReplansProtectedRegressionTestWeakening)'`.
- [x] Verification: `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/orchestrator ./internal/tool ./internal/agent ./internal/types ./internal/writeflow`.
- [x] Eval: `CODRAX_BIN=/Users/han/opt/codrax/codrax CASES='eval/cases/github_issue_tokenizers_newline_run_multirepo_py.case' PARALLEL=1 RUNS=1 TIMEOUT=1800 SUMMARY=eval/results/write_mode_tokenizers_newline_multirepo_py_after_state_barrier_20260614_summary.md bash eval/convergence_audit.sh` PASS. Authoritative report: `report_channel=post_apply_verify`, `report_passed=true`, `verify_authoritative=true`; executed command chain `python/pytest runner_missing -> python/unittest zero_tests -> make check executed`; live worktree retained the five-newline `#include <set>` regression input.

#### Batch 25: Symptom-Driven External Issue Eval Case

- Evidence source: `iamkun/dayjs` PR #1611 (`https://github.com/iamkun/dayjs/pull/1611`) reconstructed as a JavaScript fixture. The new case is intentionally symptom-driven: it reports `PT1H` formatting as `NaN`/invalid while a full duration still works, but it does not name `src/duration.js`, `Number(undefined)`, or the exact implementation line in the prompt.
- Generalized goal:
  - cover issues where the user gives observed behavior, not a patch recipe;
  - require the write controller to trigger exploration/localization before bounded implementation;
  - verify failure evidence must be available for small replan if the first patch is semantically weak;
  - the eval oracle must assert the real product contract, not just formatted output.
- Findings:
  - the first symptom-only run passed but exposed an oracle gap: `value ?? 0` avoided formatted `NaN` while leaving present components as strings;
  - the fixture oracle was strengthened to assert `parseIso("PT1H")` numeric fields via JS `deepStrictEqual` and require guarded numeric conversion in `tests/check_duration.py`;
  - the harness also exposed an eval-only bash 3 `set -u` bug: empty `focus_args[@]` expansion failed write cases without `FOCUS`. `eval/run.sh` now branches explicitly for focus/no-focus plan/apply steps.
- [x] Add `eval/cases/github_issue_dayjs_duration_nan_symptom.case`.
- [x] Strengthen `eval/fixtures/github_issues/dayjs_duration_nan/tests/duration.test.js` to assert numeric `parseIso("PT1H")` fields.
- [x] Strengthen `eval/fixtures/github_issues/dayjs_duration_nan/tests/check_duration.py` to require guarded numeric conversion, not a string-preserving fallback.
- [x] Fix `eval/run.sh` no-focus write-mode plan/apply execution under `set -u`.
- [x] Verification: `bash -n eval/run.sh`; `bash -n eval/cases/github_issue_dayjs_duration_nan_symptom.case`; `PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache python3 -m py_compile eval/fixtures/github_issues/dayjs_duration_nan/tests/check_duration.py`.
- [x] Eval: `CODRAX_BIN=/Users/han/opt/codrax/codrax CASES='eval/cases/github_issue_dayjs_duration_nan_symptom.case' PARALLEL=1 RUNS=1 TIMEOUT=1800 SUMMARY=eval/results/write_mode_dayjs_duration_nan_symptom_after_numeric_oracle_20260614_summary.md bash eval/convergence_audit.sh` PASS. Metrics showed localization work (`read_file=6`, `repo_map=3`, `source_lens=1`, `explorer_iters=5`, `midloop=2`); authoritative report: `report_channel=post_apply_verify`, `report_passed=true`, `verify_authoritative=true`; command chain `node/npm runner_missing -> make check executed`; final worktree used `Number(value) || 0` and preserved the numeric PT1H regression assertion.

#### Batch 26: Symptom-Driven Exploration-To-Fix Recovery

- Evidence sources:
  - chronotope/chrono PR #1385 (`https://github.com/chronotope/chrono/pull/1385`), reconstructed as `eval/cases/github_issue_chrono_duration_min_symptom.case`.
  - apache/commons-lang PR #1273 (`https://github.com/apache/commons-lang/pull/1273`), reconstructed as `eval/cases/github_issue_commons_lang_random_ascii_symptom.case`.
- Case design:
  - The prompt describes only the externally visible symptom and upstream reference.
  - It deliberately avoids naming the exact implementation line or patch recipe.
  - The test forces end-to-end behavior: controller must trigger exploration/localization, planner must create a bounded plan, coder must apply, verifier must run typed tests, and verify failure must drive small replan.
- Baseline failure evidence:
  - `eval/results/write_mode_symptom_chrono_commons_20260614_summary.md` reported FAIL 2/2.
  - `eval/results/github_issue_chrono_duration_min_symptom-20260614-170918/run-1.write-apply.json`: `plan_written=false`, `apply_attempted=false`, no report. Logs show `exploration_complete` followed by repeated planner `no change plan was produced this round`.
  - `eval/results/github_issue_commons_lang_random_ascii_symptom-20260614-170918/run-1.write-apply.json`: `plan_written=true`, `apply_attempted=true`, no durable worktree/report. Logs show `coder: apply incomplete (1 missing)` followed by controller dispatch transport failure (`unexpected EOF`).
- Generalized gaps:
  - P0-A no-plan recovery was too narrow: a first empty planning round was retried only when `VerifyFailureHandoff` existed. Symptom-driven tasks can have sufficient `WriteExplorationHandoff` evidence before any verify attempt exists.
  - P0-B controller dispatch failure could interrupt an already determined typed transition. If durable state proves a plan is auto-executable or an applied plan is pending post-apply verify, the scheduler does not need a fresh model decision to continue.
  - P1 state rendering still can show stale `pending_approval` on a plan after auto approval; current typed barrier reads workflow attempts and approval policy instead of trusting that label. This remains an observability cleanup item, not a correctness blocker for this batch.
- Target architecture:
  - `runControllerPlanBatch` grants exactly one bounded re-dispatch when no `ChangePlan` is installed and either `VerifyFailureHandoff` or `WriteExplorationHandoff` is present.
  - The retry hint is a soft planning directive only. The hard trigger reads typed state: no current plan plus handoff object presence.
  - `controllerDecisionFromTypedStateAfterDispatchError` recovers only in `ModeApply` and only from typed state:
    - current `ChangePlan`;
    - deterministic `AssessWriteRisk + DecideWriteApproval` result and approval/fingerprint records;
    - active batch id and ordered apply/verify attempts.
  - Recovery action:
    - auto-executable pending plan -> synthesize `apply_plan`;
    - applied active plan with no later verify attempt -> synthesize `verify_batch`;
    - all other dispatch errors remain fail-loud.
- Safety and prompt hygiene:
  - The scheduler does not parse user keywords, model summary, plan rationale, verifier narrative, logs, HTTP error strings, or `<think>`.
  - Critical and high-risk gates are unchanged: denied plans remain blocked; manual/high-risk plans still pause; stale fingerprints are not auto-converted.
  - Low/medium auto-executable plans continue without extra user approval, reducing CLI/eval interruption.
- Handoff contract:
  - Exploration evidence stays in `WriteExplorationHandoff` and `WriteContextPack` P1/P2 fields and survives planning-state reset.
  - Verify failures remain P2 and are consumed by replan before broader rediscovery.
  - Progress ledger records scheduler correction with typed reason codes:
    - `controller_dispatch_recovered_ready_plan`
    - `controller_dispatch_recovered_post_apply_verify`
    - existing `ready_plan_action_overridden` / `post_apply_verify_action_overridden` remain for successful controller dispatches that emit stale actions.
- Implementation tasks:
  - [x] Add symptom-driven Rust chrono case.
  - [x] Add symptom-driven Java commons-lang case.
  - [x] Extend no-plan retry to typed exploration handoff.
  - [x] Add controller dispatch-error typed-state recovery.
  - [x] Add tests for exploration-handoff no-plan retry.
  - [x] Add tests for dispatch-error recovery to apply and verify.
  - [x] Keep other modes untouched; changes are isolated to write controller scheduler and eval cases.
- Verification:
  - `bash -n eval/cases/github_issue_chrono_duration_min_symptom.case`
  - `bash -n eval/cases/github_issue_commons_lang_random_ascii_symptom.case`
  - `PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache python3 -m py_compile eval/fixtures/github_issues/chrono_duration_min/tests/check_duration_min.py`
  - `PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache python3 -m py_compile eval/fixtures/github_issues/commons_lang_random_ascii/tests/check_random_string_utils.py`
  - `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/orchestrator -run 'TestRunControllerPlanBatch_(NoPlanAfterExplorationHandoffGetsOneRetry|NoPlanReplanRoundGetsOneRetry)|TestRunWriteControllerWorkflow_(SynthesizesApplyAfterControllerDispatchError|SynthesizesVerifyAfterControllerDispatchError|AppliesReadyPlanBeforeRepeatedPlanningDecision|VerifiesAppliedPlanBeforeRepeatedPlanningDecision)'`
  - `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache go test ./internal/orchestrator ./internal/tool ./internal/agent ./internal/types ./internal/writeflow`
  - `GOCACHE=/private/tmp/codrax-gocache PYTHONPYCACHEPREFIX=/private/tmp/codrax-pycache make`
  - `CODRAX_BIN=/Users/han/opt/codrax/codrax CASES='eval/cases/github_issue_chrono_duration_min_symptom.case eval/cases/github_issue_commons_lang_random_ascii_symptom.case' PARALLEL=2 RUNS=1 TIMEOUT=1800 SUMMARY=eval/results/write_mode_symptom_chrono_commons_after_recovery_20260614_summary.md bash eval/convergence_audit.sh` PASS 2/2, flagged 0/2.
