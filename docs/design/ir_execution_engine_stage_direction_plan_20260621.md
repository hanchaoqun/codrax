# Codrax IR 执行引擎 — 阶段性方向规划（cutover 视角，v2 团队复核后修订）

Date: 2026-06-21
Base branch: `main`
Base HEAD: `2b5c4ba0`（v2 修订基线；v1 基于 `fa08860d`）

> 本文与两份既有文档**互补、不重复**：
> - `ir_driven_execution_engine_prd_20260621.md` —— 架构 PRD（5 个关键问题裁定、Stage 0–3 设计）。
> - `ir_driven_execution_engine_delivery_20260621.md` —— 实现交付账本（M0a–M4c 里程碑）。
>
> 本文是**独立审计后的方向规划**，站在「load-bearing vs shadow」角度给各阶段目标定义**真承重退出标准**并排序 cutover。
>
> **v2 修订说明**：v1 有几处对当前 main 过时/过度悲观，经另一团队复核后已逐条对 `2b5c4ba0` 快照重新核实并修正（见 §7）。核心修正：**不是"read 侧 cutover ≈ 0%"，而是"Stage 1 部分承重、核心 read-loop cutover 未完成"**。

---

## 1. 核心判断（修订）：周边 authority 已部分承重，read 主循环核心未 cutover

交付账本以严明纪律把 PRD 各阶段**脚手架**立了起来，并**诚实地把 shadow 标成 shadow**。准确的现状是：

> **脚手架和若干周边 authority 已完成，write 侧、source-class、ToolInvocation 等部分已承重；但 read scheduler 的核心执行状态仍由 `graphState` 承重，`IngestRound` 与 read loopkernel 仍主要是 shadow/telemetry。因此当前不是「Stage 1 完成」，而是「Stage 1 部分承重，核心 read-loop cutover 未完成」。**

### 1.1 记账纪律：scaffold-complete vs load-bearing-complete 必须分两列

把 shadow 标成 `Completed M1c shadow` 会在 dashboard 上误读成"已承重"。**每个 batch 应标两列**：

- **scaffold-complete** —— 影子结构已放置（typed 字段、reducer、projection 都在），但无 decision 真消费它。
- **load-bearing-complete** —— 已有 decision/gate 真消费它，旧路径退役或降为镜像。

---

## 2. 各阶段「load-bearing 退出标准」 + 当前真实位置（已核实）

| 阶段 | 退出标准（必须 load-bearing） | 当前真实位置（对 `2b5c4ba0` file:line 核实） |
|---|---|---|
| **Stage 0** 退役 legacy write DAG | 生产路径走 controller，**生产可达性测试**证明不再装 legacy write TaskGraph | ✅ **load-bearing-complete**：生产 `Mode.IsWrite()→runWriteControllerWorkflow`（orchestrator.go:2364），由 `TestMode_WriteControllerDoesNotInstallLegacyWriteTaskGraph`（mode_dispatch_test.go:256）守护。**以可达性测试为准，非字符串 grep** |
| **Stage 1** 单一执行 IR + 闭环 | `nodeExecStatus` 成唯一 *decision-read* 执行状态（`graphState` 退成 accessor 而非双写镜像）；`IngestRound` 成**生产** reducer；ToolInvocation log 真支撑 replay；**read 有 resumability** | ⚠️ **部分承重，核心未 cutover**：周边已承重——ToolInvocation 已生产 wiring（cmd/root.go:3859 `SetReasoningObserver`、agent.go:4113 `ensureToolInvocationID`，作 append-only 审计/replay projection）；source-class universe 已进 absence gate（见 Stage-correctness）。核心仍 shadow——`nodeExecStatus`（evidence_closure.go:182）是**双写镜像**，`setStatus` 写 `s.status[id]` 后再 mirror `closure.SetNodeExecStatus`（scheduler.go:263），而 `readyExplorerWindowContext`（scheduler.go:198）仍读 `graphState.status` 决策；`graphState` 未序列化→**read 无 resumability** |
| **Stage 2** 自适应引擎 | extract 真门 ✅；**optional/refine 节点机制承重**；progress 驱动的 AnalyzeRefine 有真生产节点；execution-tree 轻量分支可用 | ⚠️ **机制承重、AnalyzeRefine 未发节点**：extract = **真门**（`CritExtractInputReady`，stage_nodes.go）；optional-node 机制 = **已承重**（M2d-B 的 compiler-emitted source-inventory re-probe 节点，挂 `source_class_universe_incomplete`，scheduler 真处理 `TaskNode.Optional`）；但 **AnalyzeRefine via `progress_replan_required` 仍是机制就绪无生产节点**——sensor 全接（eval.go:794-802），ProgressDecision 投进 env（orchestrator.go），但**全仓无 EntryCondition 挂 `progress_replan_required`**（grep 非测试=空），账本自述"pinned AnalyzeRefine as pre-authored optional topology only" |
| **Stage 3** 语义 kernel | write budget 真门 ✅；**read loop 真消费 ≥1 个 loopkernel `RecommendedAction` 作决策**；event store 有真回放路径 | ⚠️ write = **真承重**（5 个 LoopBudget/LoopRun 构造在 write_controller_scheduler.go，`Advance` 经 `controllerLoopAdvanceAllowsAction` 硬门 3 个 write action）；read = `ReadLoopShadowComparison` 代码注释自述 "telemetry/shadow only: callers must not use this view as a hard gate until a later cutover"（read_shadow.go:4），出口只折进 `proofGuidance` advisory（read_stage_retry.go:184） |

---

## 3. 方向规划：cutover 排序 = 守护成本排序

> **铁律：任何 read-loop 的 cutover，必须先有 golden-trace pin（L1），再动行为。** stub-agent 的 `mode_dispatch_test` 是**必要非充分**。

排序起点是一处快推留下的反例：commit `4eadcdc8` 把 extract 节点 EntryCondition 从恒真 `{CritEvidenceCount ">=0"}` 改成可证伪 `{CritExtractInputReady}`（stage_nodes.go）。零-typed-evidence 的 read 问题：改前 extractor **必派**，改后 `skipExtractNode→finishExtractNode(n,true)` **标完成不派**——是 `runReadSchedulerLoop` 的**可观测行为分歧、无 golden 守护**（唯一相关断言 `finalizer_retry_scope_test.go` 守的是 finalizer-retry 复用路径，非此 skip 决策）。

### Phase A — 立即（纯结构守护，无 eval）

1. **装 extract-dispatch 的 L1 golden pin** —— 快推唯一已跨且无守护的 read-loop 行为改动，也是后续所有 read cutover 的安全前置。扩 `read_e2e_regression_test.go`，加 explorer 返回 `MissingFacts` + 空 `EvidenceItems`（无 AnswerChains/AggregateFacts）的 case，断言 `extractorCalls==0 && finalizeCalls==1 && extract 节点 skip-complete`，让 `CritExtractInputReady` 翻转**可观测而非被 fixture 掩盖**。
2. **修文档口径** —— 交付账本所有 batch 加 scaffold-complete / load-bearing-complete 两列（本文 §1.1）。
3. **非 biting ratchet 收到当前值** —— `ir_delivery_ratchet_test.go` 把 `evidence_closure.go`(2774)、`scheduler.go`(945) 收到实际值，并补 `orchestrator.go` 进 ratchet（对标 source-inventory tripwire 的 pin-at-current 纪律）。

### Phase B — Stage 1 核心 cutover（每步 golden 守护，团队优先序）

4. **`graphState.status` 退成 `EvidenceClosure` accessor（非双写镜像）** —— 这是团队点名的"下一步真正 cutover"。`readyExplorerWindowContext`（scheduler.go:198）改读 `closure.NodeExecStatus`，`setStatus` 只写 closure 删平行 `s.status` map。**破 L1 风险点**：必须保证 `attachEvidenceClosure` 早于首次 `readyExplorerWindow`（mirror 现受 `if s.closure != nil` 保护，schedule-before-attach 路径要保留 nil-fallback）。这一步同时解锁 **read resumability**。
5. **`IngestRound` 从 clone shadow 转生产 reducer** —— 退役散落的 per-round 全量重算，由单一 reducer 承重（现 `runEvidenceRoundIngestShadow` 在 `closure.Clone()` 上跑、出口 Debug，orchestrator.go:8599/8777）。Golden-trace 守护输出等价后切。

### Phase C — Stage 2/3 cutover（需 eval 验证）

6. **read loopkernel 出 1 个低风险 typed action 先承重** —— 团队建议：只选**一个**低风险 typed action 从 advisory 升为真软门，**继续保留 shadow comparison** 做回归对照。精确 typed 信号，不踩 hard-gate-on-noisy（read-lane proof 多 default-Weak，仅 `TruthLedgerFailed` 类硬信号可硬）。
7. **接通 AnalyzeRefine 真生产节点** —— 让 analyzer 在 compiler 里 pre-author 一个挂 `progress_replan_required` 的 optional refine 节点（机制已就绪、缺生产节点）。需 eval 验证不引入 runaway（对标 two-stage iteration cap）。

---

## 4. 与引擎阶段正交的「正确性」单线

source-inventory 跨语言 absence 是**正确性问题不是引擎架构**，单独闭环、不混进 Stage 重构：

- absence gate **已消费 typed source-class universe**（`validateSourceInventoryExactAbsenceBound`→`tool.SourceInventoryExactAbsenceNeedsInventoryProofRepoTruth`，contract_check_block.go:1885）——**已 load-bearing 非 advisory**。
- RNE-C61 已于 `fa08860d` 关闭（repo-truth absence universe + evidence-repair lane tool 暴露）。
- **RNE-C23/C32 不能沿用旧失败口径**——代码上 absence gate 已非 advisory，是否真转 PASS **必须重跑 eval 确认**（`arkts_repomap` 等），以 RNE tracker 为单一真源，按 case 闭环。

---

## 5. 关键纪律（写给本项目）

- **shadow 是中间态不是终点**；dashboard 用 *load-bearing*（有 decision 消费）计数；账本区分 scaffold-complete vs load-bearing-complete。
- **每个 read-loop cutover 先 golden 后行为**；L1 由行为输出等价（golden-trace）守护，不由"函数体文本未改"论证；Stage 0 类结论以**生产可达性测试**为准，非字符串 grep。
- **ratchet 必须 pin-at-current（biting）**。
- **loopkernel 投影方向单向**（EvidenceClosure→events，永不反写；当前干净，守住）。
- **第二并行权威 smell**：loopkernel write budget 与活的 orchestrator counter map 并存，`Advance` 丢弃自己 reduce 的 `UnitsUsed`（write_controller_scheduler.go:6326）；收敛方向是让 reduced budget 成唯一真源。

---

## 6. 最高杠杆的下一步

按团队优先序，落地顺序为：**(A) 修文档两列口径 + 装 extract-dispatch L1 golden pin（纯结构、零风险、补已跨红线）→ (B) `graphState.status` 退成 closure accessor（核心 read-loop cutover，解锁 resumability）→ (B) `IngestRound` 生产 reducer → (C) read loopkernel 单 action 承重 + AnalyzeRefine 真节点。** read loopkernel 与 source-inventory 的历史 eval 结论需重跑确认，不沿用旧口径。

---

## 7. v2 修订记录（团队复核逐条核实结果）

| v1 表述 | 复核结论 | v2 修正 |
|---|---|---|
| "read 侧 cutover ≈ 0%" | **过度悲观**（与自身 dashboard 4/9 load-bearing 矛盾） | 改为"Stage 1 部分承重、核心 read-loop cutover 未完成" |
| Stage 0 以 "grep=0" 为据 | **表达弱**（应以生产可达性为准） | 改引 `TestMode_WriteControllerDoesNotInstallLegacyWriteTaskGraph` + orchestrator.go:2364 |
| ToolInvocation "production unwired/仅 test" | **过时**（已生产 wiring） | 改为已承重的 append-only 审计/replay projection（root.go:3859、agent.go:4113） |
| source-class universe 仍 advisory | **过时**（已进 absence gate） | 改为 load-bearing（contract_check_block.go:1885 的 RepoTruth 变体） |
| refine "0 actuator 节点" | **不准**（有 compiler-emitted optional re-probe 节点） | 区分：optional-node 机制已承重；AnalyzeRefine via progress_replan 机制就绪、无生产节点 |
| RNE-C23/C32 "仍 advisory / 仍 FAIL" | **需重跑 eval**（代码上已非 advisory） | 改为"必须重跑 eval 确认，不沿用旧失败口径" |
| nodeExecStatus shadow / IngestRound shadow / read loopkernel telemetry | **成立**（团队认可，已 file:line 复验） | 保留 |
| extract-dispatch L1 pin 为 Phase A 首选 | **成立且与团队"golden-trace 守护"精神一致** | 保留 |
