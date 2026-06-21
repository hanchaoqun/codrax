# Codrax IR 执行引擎 — 阶段性方向规划（cutover 视角）

Date: 2026-06-21
Base branch: `main`
Base HEAD: `fa08860d`

> 本文与两份既有文档**互补、不重复**：
> - `ir_driven_execution_engine_prd_20260621.md` —— 架构 PRD（5 个关键问题裁定、Stage 0–3 设计）。
> - `ir_driven_execution_engine_delivery_20260621.md` —— 实现交付账本（M0a–M4c 里程碑）。
>
> 本文是**独立审计后的方向规划**：站在「load-bearing vs shadow」的角度，给各阶段目标定义**真承重退出标准**，并把剩余的 shadow→load-bearing **cutover** 排序、标注每步必须的守护。所有事实经隔离 worktree 对 `fa08860d` 快照 file:line 核实。

---

## 1. 核心判断：脚手架已全立，read 侧真承重 cutover ≈ 0

交付账本以严明纪律把 PRD 每个阶段的**脚手架**都立了起来，并**诚实地把 shadow 标成 shadow**（`evidence_closure.go:177` 注释自承 "M1b shadow execution ledger"；账本写明 "IngestRound … M1c shadow"、"Read remains projection-first"）。

但关键事实是：**几乎所有 read 侧的 shadow→load-bearing cutover 都被刻意推迟**。项目真实位置是「**脚手架完成（scaffold-complete），read 侧 cutover ≈ 0%**」，而非「Stage 1 完成」。

### 1.1 措辞风险（必须修正的认知偏差）

把 shadow 标成 `Completed M1c shadow` 会给人「已完成」错觉。**dashboard 必须区分**：

- **scaffold-complete** —— 影子结构已放置（typed 字段、reducer、projection 都在），但**无任何 decision 消费它**。
- **cutover-complete** —— 已有 decision/gate 真消费它，旧路径退役或降为镜像。

按真承重计数：9 个 PRD kernel 里**只有 4 个 cutover-complete**（write LoopBudget 3 门 + extract 真门 + SourceClassUniverse 入 absence 门 + ProgressReplan 入 store），其余 5 个是 scaffold-complete 的影子。

---

## 2. 各阶段「load-bearing 退出标准」 + 当前真实位置

| 阶段 | 退出标准（必须 load-bearing，非影子） | 当前真实位置（file:line 核实） |
|---|---|---|
| **Stage 0** 退役 legacy write DAG | 生产路径不可达 + 死码删除 | ✅ **cutover-complete**：`runWriteSchedulerLoop`/`BuildWriteTaskGraph` grep=0 |
| **Stage 1** 单一执行 IR + 闭环 | `nodeExecStatus` 成唯一 *decision-read* 执行状态（`graphState` 退成镜像或退役）；`IngestRound` 成**生产** reducer（非 clone 影子）；ToolInvocation log 真支撑 replay；**read 有 resumability** | ⚠️ **scaffold-complete，cutover 0%**：`nodeExecStatus`（evidence_closure.go:182）全 writer/0 decision-reader；`IngestRound` 仅在 cloned closure 上跑只 Debug 出口；`graphState.status`（scheduler.go:47）仍唯一权威且未序列化→**read 无 resumability** |
| **Stage 2** 自适应引擎 | extract 真门 ✅；**≥1 个 analyzer-pre-authored refine 节点真被 `ProgressReplan` 派发**；execution-tree 轻量分支可用 | ⚠️ extract = **真门**（`CritExtractInputReady`，stage_nodes.go:43 是 extract 唯一 EntryCondition）；refine = sensor 全接但 **0 actuator 节点**（compiler 内无节点挂 `CritProgressReplanRequired`，仅测试合成节点消费）；分支未起 |
| **Stage 3** 语义 kernel | write budget 真门 ✅；**read loop 真消费 ≥1 个 loopkernel `RecommendedAction` 作决策**（非 advisory string）；event store 有真回放路径 | ⚠️ write = **真承重**（5 个 LoopBudget/LoopRun 构造在 write_controller_scheduler.go，`Advance` 经 `controllerLoopAdvanceAllowsAction` 硬门 3 个 write action）；read = `CompareReadLoopShadow` 只折进 `proofGuidance` advisory（read_stage_retry.go:184）；`LoadLoopEventsFromFile`（store.go）**0 调用**=假 resumability |

---

## 3. 方向规划：cutover 排序 = 守护成本排序

> **铁律：任何 read-loop 的 cutover，必须先有 golden-trace pin（L1），再动行为。**
> `mode_dispatch_test` 的 stub-agent 测试是**必要非充分**（它用 stub agent、不 emit tool result，只能证 `Mode=""` vs `Mode=read` 等价，不能证 cutover 前后 node dispatch 序列等价）。

快推已留下一处反例：commit `4eadcdc8` 把 extract 节点 EntryCondition 从恒真 `{CritEvidenceCount ">=0"}` 改成可证伪 `{CritExtractInputReady}`（stage_nodes.go:43）。零-typed-evidence 的 read 问题：改前 extractor **必派**，改后 `skipExtractNode→finishExtractNode(n,true)`（orchestrator.go:5642/5647）**标完成不派**——是 `runReadSchedulerLoop` 的**可观测行为分歧，且无 golden 守护**（唯一相关断言 `finalizer_retry_scope_test.go` 守的是 finalizer-retry 复用路径，不是这个 skip 决策）。这正是排序的起点。

### Phase A — 立即（纯结构，无需 eval）：补 L1 守护，让后续 cutover 安全

1. **装 extract-dispatch 的 L1 golden pin** —— 快推**唯一已跨且无守护**的红线，也是**所有 Stage 1 read cutover 的前置**。扩 `read_e2e_regression_test.go`，加 explorer 返回 `MissingFacts` + 空 `EvidenceItems`（无 AnswerChains/AggregateFacts）的 case，断言 `extractorCalls==0 && finalizeCalls==1 && extract 节点标 skip-complete`，让 `CritExtractInputReady` 翻转**可观测而非被 fixture 掩盖**。走现有 `criterion.EvalAll` over typed EntryCondition，不发明新东西。
2. **把非 biting ratchet 收到当前值** —— `ir_delivery_ratchet_test.go` `evidence_closure.go` 2774→2638、`scheduler.go` 945→799（各 ~140 行 slack），并补 `orchestrator.go`（9402，extract-gate 代码堆积处）进 ratchet。对标 source-inventory tripwire 的 pin-at-current 纪律。

### Phase B — Stage 1 真 cutover（每步 golden 守护）

3. **`nodeExecStatus` 升为唯一执行状态权威** —— `readyExplorerWindowContext` 的 `s.status[n.ID]`（scheduler.go:210）改读 `closure.NodeExecStatus(n.ID)`，`setStatus` 只写 closure 删平行 map。**前置约束（破 L1 风险点）**：必须保证 `attachEvidenceClosure` 早于首次 `readyExplorerWindow`（`setStatus` 的 closure mirror 受 `if s.closure != nil` 保护，schedule-before-attach 路径要保留 nil-fallback）。这一步同时解锁 **read resumability**（Stage 1 的核心承诺）。
4. **`IngestRound` 从 clone 影子转生产 reducer** —— 退役散落的 per-round 全量重算（`extractFileCoverage` 等 20+ 站点），由单一 reducer 承重。Golden-trace 守护输出等价后切。

### Phase C — Stage 2/3 真 cutover（需 eval 验证）

5. **接通 1 个真 refine actuator** —— `ProgressReplan` sensor → analyzer 在 `internal/analysis/compiler/` 内 pre-author 一个真 refine 节点被派发（现在 0 个）。需 eval 验证不引入 runaway（对标 two-stage iteration cap）。
6. **read loopkernel 出 1 个真决策** —— 把某个 `RecommendedAction` 从 `proofGuidance` advisory 升为真软门（谨慎：精确 typed 信号，不踩 hard-gate-on-noisy-signal 红线；read-lane proof 多 default-Weak，只有 `TruthLedgerFailed` 类硬信号可硬）。

---

## 4. 与引擎阶段正交的「正确性」单线（优先级独立）

source-inventory 的跨语言 absence 正确性是**正确性问题，不是引擎架构**，应**单独闭环、不要混进 Stage 重构、也不要等 Stage 3**：

- RNE-C61 已于 `fa08860d` 关闭（repo-truth absence universe + evidence-repair lane tool 暴露）。
- 仍需对 RNE tracker（`docs/design/read_mode_noise_convergence_eval_gap*.md`）逐项核对：source inventory 是否已从 advisory 升为 executable localization authority、census 是否真路由进 absence 门、`arkts_repomap` eval 是否转 PASS。以 RNE tracker 为单一真源，按 case 闭环。

---

## 5. 关键纪律（写给本项目）

- **shadow 是中间态不是终点**；dashboard 用 *load-bearing*（有 decision 消费）计数，不用 *scaffold placed* 计数；账本宜区分 scaffold-complete vs cutover-complete。
- **每个 read-loop cutover 先 golden 后行为**；L1 由行为输出等价（golden-trace）守护，不由「函数体文本未改」论证。
- **ratchet 必须 pin-at-current（biting）**，否则容忍 ~140 行静默回归。
- **loopkernel 投影方向单向**（EvidenceClosure→events，永不反写 closure；当前 grep `closure.Set` in loopkernel = 0，干净，守住）。
- **第二并行权威 smell**：loopkernel write budget 与活的 orchestrator counter map 并存，`Advance` 丢弃自己 reduce 的 `UnitsUsed`（write_controller_scheduler.go:6326 `_, decision :=`）。收敛方向是让 reduced budget 成唯一真源，而非两套并存。

---

## 6. 最高杠杆的下一步

**Phase A #1（extract-dispatch L1 golden pin）** —— 纯结构、修已跨红线、零 eval、零与并发 session 撞车风险。它是后续所有 read cutover 的安全前置，也补上账本 M1b「guard dispatch equivalence」承诺但实际未覆盖的 skip case。
