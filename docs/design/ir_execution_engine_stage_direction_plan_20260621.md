# Codrax IR 执行引擎 — 阶段性方向规划（cutover 视角，v2 团队复核后修订）

Date: 2026-06-21
Base branch: `main`
Base HEAD: `398a32303b`（2026-06-21 复核基线；v2 修订基线为 `2b5c4ba0`，v1 基于 `fa08860d`）

> 本文与两份既有文档**互补、不重复**：
> - `ir_driven_execution_engine_prd_20260621.md` —— 架构 PRD（5 个关键问题裁定、Stage 0–3 设计）。
> - `ir_driven_execution_engine_delivery_20260621.md` —— 实现交付账本（M0a–M4c 里程碑）。
>
> 本文是**独立审计后的方向规划**，站在「load-bearing vs shadow」角度给各阶段目标定义**真承重退出标准**并排序 cutover。
>
> **v2 修订说明**：v1 有几处对当前 main 过时/过度悲观，经另一团队复核后已逐条对 `2b5c4ba0` 快照重新核实并修正（见 §10）。核心修正：**不是"read 侧 cutover ≈ 0%"，而是"Stage 1 部分承重、核心 read-loop cutover 未完成"**。

---

## 1. 核心判断（修订）：周边 authority 已部分承重，read 主循环核心未 cutover

交付账本以严明纪律把 PRD 各阶段**脚手架**立了起来，并**诚实地把 shadow 标成 shadow**。准确的现状是：

> **脚手架和若干周边 authority 已完成，write 侧、source-class、ToolInvocation、read node execution status、IngestRound reducer 已部分承重；read run snapshot 仍是 substrate/scaffold；read loopkernel add-proof 已进入 typed one-shot next-action code-complete 状态但仍缺代表性 eval/UX 审计；AnalyzeRefine optional actuator 已进入生产 topology 但缺 read E2E golden 守护。因此当前不是「Stage 1/2/3 全完成」，而是「核心 cutover 已推进，剩余 work 要围绕 load-bearing 判定、resume/replay/eval correctness 收敛」。**

### 1.1 记账纪律：scaffold-complete vs load-bearing-complete 必须分两列

把 shadow 标成 `Completed M1c shadow` 会在 dashboard 上误读成"已承重"。**每个 batch 应标两列**：

- **scaffold-complete** —— 影子结构已放置（typed 字段、reducer、projection 都在），但无 decision 真消费它。
- **load-bearing-complete** —— 已有 decision/gate 真消费它，旧路径退役或降为镜像。

---

## 2. 各阶段「load-bearing 退出标准」 + 当前真实位置（已核实）

| 阶段 | 退出标准（必须 load-bearing） | 当前真实位置（对 `2b5c4ba0` file:line 核实） |
|---|---|---|
| **Stage 0** 退役 legacy write DAG | 生产路径走 controller，**生产可达性测试**证明不再装 legacy write TaskGraph | ✅ **load-bearing-complete**：生产 `Mode.IsWrite()→runWriteControllerWorkflow`（orchestrator.go:2364），由 `TestMode_WriteControllerDoesNotInstallLegacyWriteTaskGraph`（mode_dispatch_test.go:256）守护。**以可达性测试为准，非字符串 grep** |
| **Stage 1** 单一执行 IR + 闭环 | `nodeExecStatus` 成唯一 *decision-read* 执行状态（`graphState` 退成 accessor 而非双写镜像）；`IngestRound` 成**生产** reducer；ToolInvocation log 真支撑 replay；**read 有 resumability** | ⚠️ **核心状态/ingest 已承重，auto-resume 未 cutover**：ToolInvocation 已生产 wiring（cmd/root.go:3859 `SetReasoningObserver`、agent.go:4113 `ensureToolInvocationID`）；source-class universe 已进 absence gate（见 Stage-correctness）；B1 已让 `EvidenceClosure.NodeExecStatus` 成为 scheduler/orchestrator decision-read authority；B3 已让 `applyStageOutput` 对生产 closure 执行 `IngestRound`。B2 read snapshot 仍是 typed substrate/store，生产 producer/consumer 与 auto-resume 未 cutover。 |
| **Stage 2** 自适应引擎 | extract 真门 ✅；**optional/refine 节点机制承重**；progress 驱动的 AnalyzeRefine 有真生产节点；execution-tree 轻量分支可用 | ⚠️ **typed optional actuator 已进入生产 topology，但缺 read E2E golden**：extract = **真门**（`CritExtractInputReady`，stage_nodes.go）且 A1 已有 golden pin；optional-node 机制 = **部分承重**（source-inventory re-probe 与 C2 AnalyzeRefine 均由 compiler pre-author，scheduler 只消费 typed criterion env）；C2 已发 `progress_replan_required` optional one-shot `NodeProbe`，但还没有 read E2E 行为 pin / eval runaway guard。仍 open：AnalyzeRefine golden、execution-tree 轻量分支/branch replay。 |
| **Stage 3** 语义 kernel | write budget 真门 ✅；**read loop 真消费 ≥1 个 loopkernel `RecommendedAction` 作决策**；event store 有真回放路径 | ⚠️ **write 真承重，read add-proof code-complete/eval pending，event replay 未 cutover**：write = **真承重**（5 个 LoopBudget/LoopRun 构造在 write_controller_scheduler.go，`Advance` 经 `controllerLoopAdvanceAllowsAction` 硬门 3 个 write action）；read `LoopActionAddProof` 已由 D1-F8b 接入 typed one-shot `readLoopNextActionDecision`，fact/transient retry 先记录 decision，下一次 explore window 消费该 decision 生成窄 proof-collection hint，并保留 `ReadLoopShadowComparison` telemetry；但代表性 eval、用户侧状态噪音和 UX 审计仍未闭环。event store 仍无生产 replay 路径。 |

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
5. **`IngestRound` 从 clone shadow 转生产 reducer** —— B3 已将 `applyStageOutput` 切到生产 `ingestEvidenceRound` helper，单一 reducer 负责 read coverage / accepted evidence carrier；后续继续退役重复 coverage append 站点。

### Phase C — Stage 2/3 cutover（需 eval 验证）

6. **read loopkernel 出 1 个低风险 typed action 先承重** —— 团队建议：只选**一个**低风险 typed action 从 advisory 升为真软门，**继续保留 shadow comparison** 做回归对照。精确 typed 信号，不踩 hard-gate-on-noisy（read-lane proof 多 default-Weak，仅 `TruthLedgerFailed` 类硬信号可硬）。
7. **给 AnalyzeRefine 生产节点补行为守护** —— analyzer/compiler 已 pre-author 一个挂 `progress_replan_required` 的 optional refine 节点；下一步不是再造节点，而是补 read E2E golden、eval runaway guard 和状态卡解释，证明它在真实 read loop 中不引入噪音/重复探索。

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

按当前 HEAD，A/B/C 中的 extract golden、NodeExecStatus、IngestRound 生产 ingest、source-inventory 部分 correctness authority 已完成并入账；read loopkernel add-proof 已 code-complete 但需代表性 eval/UX 审计；read snapshot substrate、AnalyzeRefine optional node、event replay 仍必须按 load-bearing 退出标准继续收敛。下一步最高杠杆顺序调整为：**(D1) 重跑 source-inventory correctness eval，确认 RNE-C23/C32/RNE-C61 真实状态 → (D2) 6-case 商用代表批，按 2 并行审计噪音、工具使用、性能、handoff，并覆盖 D1-F8b add-proof next-action 的真实 UX → (E) 基于 D1/D2 新证据再切 read auto-resume、event replay、AnalyzeRefine guard 或 source-inventory correctness 修复。** 历史 eval 结论必须重跑确认，不沿用旧口径。

---

## 7. 当前 HEAD 复核摘要（`398a32303b`）

本次复核对照当前 `main` 代码，不沿用旧审计口径。结论：v2 方向仍合理，但需要把"完成"拆成 scaffold 与 load-bearing 两个维度，否则后续 dashboard 会继续把 shadow 误读为承重。

| Kernel / surface | scaffold-complete | load-bearing-complete | 代码复核结论 |
|---|---:|---:|---|
| Legacy write DAG retired | yes | yes | 生产写模式从 `Mode.IsWrite()` 进入 `runWriteControllerWorkflow`，read `runTaskGraph` 防御 retired write node。Stage 0 以生产可达性测试为准。 |
| ToolInvocation / ReasoningGraph | yes | yes | `cmd/root.go` 已安装 `ReasoningObserver`，`agent.go` 确保 tool invocation ID；这是 append-only audit/replay projection，非 shadow-only。 |
| Source-class universe / absence gate | yes | yes | 新增 repo-truth wrapper：`SourceInventoryExactAbsenceNeedsInventoryProofRepoTruth`。contract check、pre-emit check、investigation-complete 都能在 observation 为空时从 repo truth seed source-class universe。历史 RNE-C23/C32 只能通过 eval 复验确认，不能继续按旧失败口径判断。 |
| `NodeExecStatus` | yes | yes | B1 已将 `EvidenceClosure.NodeExecStatus` 提升为 scheduler/orchestrator closure-first decision-read authority；`graphState.status` 只保留 nil-closure bootstrap fallback。 |
| `IngestRound` | yes | partial | B3 已将 `applyStageOutput` 切到生产 `ingestEvidenceRound`，由 typed `EvidenceClosure.IngestRound` 承载 read coverage / accepted evidence ingestion；但 truth-set merge 与 Mutable direct-set 站点仍并存，单一 reducer 尚未成为所有 read closure/handoff 字段的唯一写入权威。 |
| Extract stage node / `extract_input_ready` | yes | yes | Extract node 和 typed readiness 已承重；A1 已补 false skip-complete 与 true dispatch 的行为级 golden pin。 |
| Optional source-inventory re-probe | yes | yes | compiler 预置 optional source-inventory re-probe node，scheduler 真处理 `TaskNode.Optional` 和 `source_class_universe_incomplete`。 |
| Progress / AnalyzeRefine | yes | partial | C2 已由 compiler 发 bounded optional one-shot `NodeProbe`，EntryCondition 挂 typed `progress_replan_required`，scheduler 不 runtime append DAG；但缺 read E2E golden 和 eval runaway guard，不能算商业承重完成。 |
| Loopkernel write budget / advance | yes | yes for write | write controller 已通过 `LoopRun.Advance`/`LoopBudget` 消费 typed budget surfaces；read loopkernel 仍是 shadow/advisory。 |
| Read loopkernel recommended action | yes | partial | D1-F8b 已将 `LoopActionAddProof` 提升为 typed one-shot read next-action carrier：fact/transient retry 先记录 `readLoopNextActionDecision`，下一次 window hint 消费该 decision；代表性 eval/UX 复核仍未完成。 |
| Read run snapshot substrate | yes | no | `ReadRunSnapshot` 与 `ReadRunSnapshotStore` 已有 typed projection/store/tests，但无生产 producer/consumer、无 `/workflow`/REPL read resume、无 scheduler replay。 |
| LOC ratchet | yes | yes | A2/D1-F8a 已 pin 当前值：`evidence_closure.go=2636`、`scheduler.go=747`、`orchestrator.go=9397`；`orchestrator.go` 的 2 行增长来自 D1-F8b 必要接线，其余逻辑在新 concern 文件。 |

红线复核：
- 本计划不允许新增用户意图关键词、模型散文、prompt/hint 文本、ranker/grep count、elapsed time 作为 hard route。
- read-loop cutover 的 hard gate 只能消费 typed node status、typed criteria、closure/reasoning artifacts、schema enum、路径/解析结果。
- loopkernel read cutover 第一批必须是低风险 typed action，并保留 shadow comparison 作为审计回退证据。

---

## 8. 可执行任务规划列表（先守护，再 cutover，再 eval）

### Batch A0: Direction Ledger Cutover Accounting

目标：
- 将 `ir_driven_execution_engine_delivery_20260621.md` 的 Current State Audit 和 Progress Ledger 从单一 `completed` 改为双列：`scaffold_status` 与 `load_bearing_status`。
- 本文作为方向规划保留，delivery ledger 作为执行账本；新增发现必须先写入 ledger，再编码。

代码/文档探索点：
- `docs/design/ir_driven_execution_engine_delivery_20260621.md`
- `docs/design/ir_driven_execution_engine_prd_20260621.md`
- 本文第 7 节复核表

任务：
- 为 M1b、M1c、M2d-A、read loopkernel 相关条目标注 `scaffold-complete / load-bearing-open`。
- 为 ToolInvocation、source-class absence、write budget 标注 `load-bearing-complete`。
- 增加"不能用 completed 表示 shadow 已承重"的账本规则。

验证：
- 文档 grep：所有 `shadow` 条目必须显式说明是否 load-bearing。
- 不改代码，不跑产品测试；提交前确认 `git diff` 仅文档。

退出标准：
- dashboard/ledger 不再把 shadow scaffold 显示成商用承重完成。

### Batch A1: Extract Dispatch Golden Pin

目标：
- 为已落地的 `CritExtractInputReady` 行为变化补 L1 golden 守护。
- 先 pin 当前行为，再进行任何 read-loop cutover。

代码探索点：
- `internal/orchestrator/read_e2e_regression_test.go`
- `internal/orchestrator/orchestrator_dag_test.go`
- `internal/orchestrator/orchestrator.go` 的 `findPendingExtractNode`、`extractEntryReady`、`skipExtractNode`
- `internal/analysis/compiler/stage_nodes.go`
- `internal/analysis/criterion/eval.go` 的 `extract_input_ready`

任务：
- 增加 read E2E case：explorer 返回 `MissingFacts`，且无 `EvidenceItems`、无 `AnswerChains`、无 `AggregateFacts`、无 typed external observation。
- 断言 extractor 不 dispatch，finalizer dispatch 一次。
- 断言 extract node 被 mark done / skip-complete，read run 能稳定终止。
- 增加反向 case：当存在 typed extract input 时 extractor dispatch 一次，避免只 pin skip 路径。

验证：
- `go test ./internal/orchestrator -run 'TestE2E_ReadMode_.*Extract|TestRunTaskGraph_.*Extract|TestStageMappingFirstClassExtractSkip'`
- `go test ./internal/analysis/compiler ./internal/analysis/criterion`

退出标准：
- `CritExtractInputReady` 的 true/false 两条路径都有行为级 golden pin。

### Batch A2: Biting Ratchet Pin

目标：
- 将非 biting ratchet 收到当前真实值，防止 hot files 再次悄悄膨胀。

代码探索点：
- `internal/orchestrator/ir_delivery_ratchet_test.go`
- `internal/types/evidence_closure.go`
- `internal/orchestrator/scheduler.go`
- `internal/orchestrator/orchestrator.go`

任务：
- 将 `evidence_closure.go` ratchet 从 2774 收到当前 2636。
- 将 `scheduler.go` ratchet 从 945/799 收到当前 747。
- 增加并收紧 `orchestrator.go` ratchet到当前 9395；若后续需要超过，必须先拆文件或更新 ledger 说明。
- 在测试失败信息中要求先拆分 concern 文件，而不是扩大预算。

验证：
- `go test ./internal/orchestrator -run TestIRDeliveryHotFileLineRatchet`
- `wc -l internal/types/evidence_closure.go internal/orchestrator/scheduler.go internal/orchestrator/orchestrator.go`

退出标准：
- ratchet pin-at-current，后续增长会真实咬住。

### Batch B1: NodeExecStatus Load-Bearing Cutover

目标：
- `EvidenceClosure.NodeExecStatus` 成为 read scheduler 唯一 decision-read 状态源。
- `graphState.status` 退为 nil-closure fallback 或薄 accessor，不再作为并行权威。

代码探索点：
- `internal/orchestrator/scheduler.go`：`readyExplorerWindowContext`、`allDone`、`firstFinalizeReadyMerged`、`requeueValidationTargets`、`forceCloseExploreWindow`、`requeueToStage`
- `internal/orchestrator/orchestrator.go`：所有 `state.status[...]` 直接读取点
- `internal/types/evidence_closure.go`：NodeExecStatus clone/merge/reset
- `internal/orchestrator/scheduler_test.go`

本批探索确认：
- `scheduler.go` 仍有 decision-read 直接消费 `s.status[...]`：ready window、finalize readiness、validation feedback requeue、force-close、stage requeue、all-done。
- `orchestrator.go` 仍有 retry/finalize helper 直接消费 `state.status[...]`：structurally-empty retry、pending extract lookup、Tier-1 floor retry。
- `EvidenceClosure.NodeExecStatus` 已具备 typed enum、clone/merge/reset 能力，B1 不新建执行状态 taxonomy，只把现有 typed carrier 提升为承重 authority。
- B1 不改变 prompt、不解析模型输出散文、不引入用户意图关键字判断；所有行为变化只来自 typed node status accessor。

任务：
- 新增 `graphState.nodeStatus(id)` / `graphState.setStatus` accessors：attached closure 优先，未 attach 时 fallback 到 local bootstrap map。
- 改所有 read decision sites 使用 accessor，不直接读 `state.status[...]`。
- `attachEvidenceClosure` 必须在第一次 window readiness 前完成；测试保留 schedule-before-attach fallback。
- 删除或弱化 `TestGraphState_NodeExecStatusShadow` 的 shadow 断言，替换为 load-bearing 断言。
- 任何 status 持久化只保存 typed enum，不保存模型文本或日志散文。

验证：
- `go test ./internal/orchestrator -run 'TestGraphState|TestRunTaskGraph|TestE2E_ReadMode|TestMode_DefaultIsRead|TestRunMode_ReadByteIdentical'`
- Batch A1 golden tests 必须保持通过。

退出标准：
- `rg 'state\\.status\\[|s\\.status\\[' internal/orchestrator` 只剩 bootstrap/fallback 或测试中明确的 legacy fixture。
- read scheduler behavior golden trace 不变。

### Batch B2: Read Resume Seed On Closure Status

目标：
- 在 B1 之后，用 closure-backed status 支撑最小 read resumability，而不是直接新建第三套 execution IR。

代码探索点：
- `internal/types/evidence_closure.go`
- reasoning graph / loopkernel read projection
- `.codrax` output/session persistence surfaces
- existing write workflow run store 的 atomic write 模式

本批探索确认：
- `EvidenceClosure` 已暴露 B2 所需 typed getters：`NodeExecStatuses`、`AcceptedEvidenceRefs`、`ReadSet`、`ReadRanges`、`FileTotalLines`、`LatestProgressDecision`、`SourceInventoryObservation`。
- `SourceInventoryObservationFromMutable` 已能合并 Mutable / TurnA / closure 侧 source-inventory authority，不需要新建 source-class taxonomy。
- 原子写已有统一工具 `types.AtomicWriteFileSync`，write workflow / reasoninggraph / loopkernel store 都复用该写法；B2 不新造非原子 JSON 落盘。
- B2 只交付 replay/audit substrate：typed snapshot projection、Save/Load/List/Clear store 和测试；不自动恢复 read run，不让 snapshot 改变 scheduler 决策。

任务：
- 定义 read run snapshot：TaskGraph identity、NodeExecStatus map、accepted evidence refs、read ranges、source inventory observation、progress decision。
- 先实现 replay/audit load，不直接开启自动 resume。
- 增加 crash-safe atomic write，避免半写快照污染下一轮。
- status replay 只消费 typed `NodeExecStatus` enum 与 artifact refs，不消费 rendered answer/prose。

验证：
- focused types/reasoninggraph/orchestrator tests。
- 人工中断 read run 后能显示 snapshot，不自动改变下一轮行为。

退出标准：
- read has resumability substrate；自动 resume 作为后续独立 cutover，不和 B1 混批。

### Batch B3: IngestRound Production Reducer Cutover

目标：
- 将 `EvidenceClosure.IngestRound` 从 clone shadow 转为生产 reducer，收敛 per-round read coverage / accepted evidence ingestion。

代码探索点：
- `internal/orchestrator/orchestrator.go` 的 `ingestEvidenceRound`、`applyStageOutput`
- `internal/types/evidence_round.go`
- `internal/tool/ground` compatibility wrapper
- `extractFileCoverage` 相关调用点

本批探索确认：
- B3 前，`applyStageOutput` 先追加 `output.ToolResults` 到 `busCtx.ToolResults`，随后只在 clone 上记录 Debug delta；生产 closure 未消费该 delta。
- B3 已将 clone-only helper 改为 production `ingestEvidenceRound`，直接调用 `Mutable.EvidenceClosure().IngestRound(results, repoRoot)`。
- `EvidenceClosure.IngestRound` 已是 typed reducer：只消费 `ToolResult`、`repoRoot`、read coverage 和 typed `ToolHandoffCarrier.AcceptedEvidence`，不依赖 prompt、模型散文或用户意图关键词。
- B3 不改变 `ToolResults` append-history 语义，不替代 `EvidenceItems`/`FlowFindings` truth-set merge；只让 read coverage / accepted evidence carrier 的已有 typed reducer 承重。

任务：
- 先增加 parity test：同一 `StageOutput.ToolResults` 下，legacy recompute 与 `IngestRound` delta 完全一致。
- 在 `applyStageOutput` 中对真实 closure 调用 `IngestRound`，同时保留一轮 legacy parity assertion。
- 删除 clone-only shadow 路径；保留 diagnostic metric 但不得作为 hard gate。
- 逐步退役重复 coverage append 站点，避免 double count / duplicate accepted evidence。

验证：
- `go test ./internal/types ./internal/tool/ground ./internal/orchestrator -run 'IngestRound|ApplyStageOutput|ReadCoverage|AcceptedEvidence'`
- read golden trace。

退出标准：
- clone-only shadow helper 删除或改名为 production reducer helper。
- read coverage/references 由单一 reducer 承重，不再依赖多处散落重算。

### Batch C1: Read Loopkernel Single-Action Cutover

目标：
- 让 read loop 真消费一个低风险 `LoopRecommendedAction`，同时保留 `ReadLoopShadowComparison` 对拍。

代码探索点：
- `internal/loopkernel/read_adapter.go`
- `internal/loopkernel/read_shadow.go`
- `internal/orchestrator/read_stage_retry.go`
- `internal/orchestrator/explore_parallel_dispatch.go`

本批探索确认：
- `ReadProofGuidance` 明确约束：failed truth 才是 hard block，weak/missing/unavailable proof 保持 advisory；C1 不升级 `LoopActionBlock`，避免改变用户可见终止语义。
- 当前 read retry hint 已渲染 `proofGuidance` 和 shadow comparison，但 imperative action 仍硬编码为 `LoopActionAddProof`；C1 要把 action 选择改为消费 typed `ReadProofGuidance.RecommendedAction`。
- 选定单一低风险 action：`LoopActionAddProof`。当 proof authority 为 weak/add_proof 且非 hard block 时，系统把下一轮 continuation hint 收敛成 narrow proof follow-up；其它 action 仍保持 telemetry/advisory，不做硬门。
- C1 不消费 rendered guidance 字符串、不解析模型输出、不根据用户意图关键词分流；prompt 文案只承载 typed action 的软指导。

任务：
- 选择单一低风险 action：优先候选为 `LoopActionAddProof` 对已存在 typed proof weak 的 retry hint 强化，或 `LoopActionBlock` 对 hard truth failure 的 fail-loud。最终选择必须写入 ledger。
- hard route 只能消费 loopkernel typed state、truth action、hard block enum；不能消费 rendered `proofGuidance` 字符串。
- 保留 shadow comparison，并记录 mismatch telemetry。
- 不改变 model prompt 作为硬门来源；prompt 只可提示下一步。

验证：
- loopkernel package tests。
- read retry focused tests。
- 代表性 read eval：relation/subagent、ArkTS source inventory、trace query。

退出标准：
- 至少一个 read scheduler decision 由 loopkernel typed action 承重，且 mismatch 可审计。

### Batch C2: AnalyzeRefine Production Optional Node

目标：
- 将 `progress_replan_required` 从 sensor-only 变成 analyzer-pre-authored optional node 的生产 actuator。

代码探索点：
- `internal/analysis/compiler/stage_nodes.go`
- `internal/analysis/criterion/eval.go`
- `internal/types/progress_delta.go`
- `internal/orchestrator/scheduler.go`

本批探索确认：
- `CritProgressReplanRequired` 和 evaluator 已存在，并且只消费 typed `criterion.Env.ProgressDecision`；不会解析 retry prose、prompt text 或模型 rationale。
- scheduler 已支持 analyzer-pre-authored optional nodes：false EntryCondition 不制造 blocked 噪音，true typed signal 会进入 ready window；现有测试已覆盖 pre-authored refine node 行为。
- 不新增 `TaskNodeType`，避免扩大 stageMapping 面；C2 使用 optional one-shot `NodeProbe` 作为 AnalyzeRefine actuator carrier，输出 immutable `analysis_refinement_handoff` / `progress_decision`。
- compiler 是唯一写入点；运行时 scheduler 不 append、不 mutate AnalysisIR。

任务：
- compiler emit 一个 optional AnalyzeRefine node，EntryCondition 挂 `CritProgressReplanRequired`。
- scheduler 不得 runtime append/modify AnalysisIR；只能读取 analyzer-authored optional node。
- 设置 typed budget：每 run 最多触发一次或按 `LoopBudget` unit cap，防止 runaway。
- refine 输出必须是 immutable rewrite handoff 或 bounded retry directive；不能直接改原 AnalysisIR 结构。

验证：
- `go test ./internal/analysis/compiler ./internal/analysis/criterion ./internal/orchestrator -run 'AnalyzeRefine|ProgressReplan|Optional'`
- read eval 观察轮次和上下文噪音，不得增加重复探索。

退出标准：
- 非测试路径存在挂 `progress_replan_required` 的 analyzer-authored optional node，且有 budget guard。

### Batch D1: Source-Inventory Correctness Eval Recheck

目标：
- 复验 RNE-C23/C32/RNE-C61，不沿用旧 "repo_map=0 / advisory" 失败结论。

代码/数据探索点：
- `eval/cases/harmony/arkts_repomap.case`
- source-inventory absence tests
- latest `.codrax` logs for source-inventory lanes

任务：
- 跑 ArkTS/Cangjie/C/C++/JS-TS/config-workflow 代表性 source inventory absence/member cases。
- 记录 repo_map/source_inventory 是否被调用、source-class universe 是否进入 observation、absence gate 是否按 repo truth 工作。
- 若失败，按 per-class 分类：scanner/lens/projection/gate/final-surface/eval-infra，不做 per-shape patch。

本批探索确认：
- 现有 typed universe 入口为 `AttachSourceInventorySourceClassUniverse`，从 repo-truth tracked/filesystem walk 计算 `SourceInventorySourceClassCount`，并统一走 `types.ClassifySourcePathRole`；不得新增并行 source-class taxonomy。
- Absence gate 的 repo-truth seed 已通过 `SourceInventoryExactAbsenceNeedsInventoryProofRepoTruth` 接入 contract/pre-emit/completion 面；D1 要验证它在真实 eval 中是否足够，而不是只看单元测试。
- 现有 D1 单测入口包括 `source_inventory_absence_repo_truth_test.go`、`source_class_universe_absence_test.go`、`source_inventory_universe_coverage_test.go`、`repomap/tool_scope_test.go` 的 ArkTS/Cangjie/auxiliary projection cases，以及 `source_inventory_convergence_test.go` 的 LOC/kernel-bypass/no-parallel-taxonomy tripwire。
- 现有 eval 入口包括 `eval/cases/harmony/arkts_repomap.case`、`cangjie_repomap.case`、`cangjie_repomap_fixture.case`。D1 先跑这三个，并用结果决定是否扩到 C/C++、JS/TS、config/workflow 的 source-inventory member cases。
- 审计维度必须保留 typed 证据：`repo_map(view=source_inventory)` 是否实际调用、`source_classes:` 是否出现、`thirdparty/vendor/generated/fixture/test/production` 是否被正确分类、final answer 是否把 navigation facts 当语义 citation。

验证：
- eval 输出和 debug logs 人工审计。
- focused source inventory tests。

退出标准：
- RNE tracker 更新；若仍失败，新增系统 batch，先落 ledger 再编码。

### Batch D2: Commercial Eval Batch After Cutovers

目标：
- 在 A/B/C 批完成后跑 6-case 批次，每次 2 并行，审计性能、噪音、handoff、tool usage。

代表性顺序：
- read relation/subagent registry
- ArkTS source inventory
- trace query causal chain
- JS/TS workspace implementers
- C++ symptom-driven write repair
- low-risk C++ patch

审计维度：
- 是否主动使用 `repo_map` / `trace_query` / typed source inventory。
- round count、tool count、wall time、context size、内存/CPU 症状。
- 是否出现重复 repair、unsupported tool、schema retry、低 delta 循环。
- handoff 是否把 P0/P1/P2 证据带到 extractor/finalizer/write report。

退出标准：
- 每个失败都进入 ledger 的系统 gap，不允许只在聊天或临时日志里存在。
- 对通过样本做人工正确性审计，不只看 harness PASS。

### Batch D1-F1: Source-Inventory Routine-Path Noise Reduction

Gap IDs:
- D1-G1: Typed source-inventory lens is often executed late, after broad grep/read_file/list_files work or after completion rejection. This raises explorer iterations while correctness eventually recovers.
- D1-G2: Read mode still allows broad repo-local file discovery through `exec_command find`, even when `list_files` / `repo_map(source_inventory)` are the typed tools for the requested lane.
- D1-G3: Final answer surfaces repaired/audit-only enum caveats and deterministic system supplements after all requested members are directly grounded.
- D1-G4: Principal enumeration presentation and principal-support coverage use different structured-carrier predicates: finalizer-authored `section.items` can visibly preserve every cited member, while support coverage still requires list/table rows and emits contradictory contract/caveat noise.
- D1-G5: Relation/member dimensions such as package/module/namespace can be accepted as typed aggregate facts but remain unstable on the final answer surface when the model covers rows without clearly carrying the dimension label. The repair must preserve the accepted typed dimension as a structured carrier when needed, without matching user prose or model rationale.
- D1-G6: `arkts_repomap` now passes correctness with zero contract violations, but still shows a finalizer retry caused by a malformed JSON block fragment (`id` missing after tool-param compatibility repair). This is a JSON/repair-layer UX/perf gap, not a source-inventory correctness failure; it must be handled by the unified structured repair layer rather than prompt keyword tuning.
- D1-G7: Principal enumeration answers can carry duplicate user-visible member carriers after deterministic row normalization when the model emits both rich sections and separate list rows for the same accepted member set. This creates redundant output and soft principal-support / facet-coverage noise even when the functional answer is correct. Status: open; D1-F5 three-case recheck still shows ArkTS `contract_warning` from coverage-surface disagreement while the eval answer is functionally PASS.
- D1-G8: Source-inventory root-scope runs can still force-read false-positive production files after the source-inventory lens already proved the exact `.ets` corpus universe. The forced-read authority must consume typed source-class/language/lens coverage and avoid re-reading unrelated string/comment matches as hard blockers. Status: complete in D1-F1h.
- D1-G9: Source-inventory routine path can still start with broad text grep / incidental support reads before the typed inventory lens becomes the load-bearing navigation authority. D1-F5 closes the **explorer** half by making a compiler/controller-authored source-inventory lens probe the first explore surface and constraining the first tool schema to `repo_map(view=source_inventory)` plus emit tools. The **analyzer prescan** half remains open: three-case recheck still shows analyzer `list_files`/`grep` work before the typed lane is fully established. Status: partial; explorer cutover complete in D1-F5, analyzer cutover queued as D1-F6.
- D1-G10: Pre-finalize Tier-1 localization can over-apply to incidental support files after the principal source-inventory member slate is complete. In the ArkTS recheck, a support read of `extract_arkts.go` triggered repeated "verification not stable enough" loops even though all requested principal `.ets` members were already grounded. The fix must introduce a typed principal/support localization authority, not a path-specific exception. Status: complete in D1-F2.
- D1-G11: Final answer can append generic uncertainty/inconsistency supplemental notes even when the typed answer contract, finalizer retry counters, semantic reviewer, and eval verdict are clean. This is user-facing noise; suppression must consume typed violation/severity state, not model prose or answer wording. Status: complete in D1-F4 for generic self-contradiction on accepted fully cited principal enumerations; real typed scope/uncertainty caveat blocks remain visible.
- D1-G12: A lens-first schema boundary can itself become noise if it keeps the explorer inside a probe-only tool surface after the first typed source-inventory observation. Early D1-F5 eval showed repeated unavailable `read_file` attempts after a successful lens because the surface never released. Status: complete in D1-F5: `ExploreToolSurface` releases after a typed successful `repo_map` source-inventory observation, with runtime boundary tests covering unsupported pre-release tools.
- D1-G13: Source-inventory graph surface labels were carried only as rendered `note: surface=...`, while final-answer surface preservation consumed only `EvidenceItem.SurfaceTerms`. This let repo_map know `@Component` while the final answer could drop it. Status: complete in D1-F5: advisory/observation/ledger now carry typed `surface_terms`, and answer-document materialization consumes typed `SourceInventoryObservation` surface terms without parsing rendered repo_map prose.
- D1-G14: Eval verdicts can over-specify presentation shape for requested dimensions. `cangjie_repomap` failed `missing_section:package` even though manual output included package paths for every listed member. This is not a functional-answer failure, but it reveals a system gap: requested dimensions such as package/module/namespace should become typed table columns or dimension carriers where possible, and eval adapters should prefer semantic satisfaction over section-label coupling. Status: complete in D1-F6b/D1-F6f. Eval now supports `EXPECT_DIMENSIONS` + value/regex obligations, and source-inventory row attributes carry graph-derived package/module context into the finalizer row contract without parsing final prose.
- D1-G15: Source-inventory completion can still incur repeated "验证还不够稳" loops when analyzer-required documentation/support files, case-drifted paths, or missing navigation lenses remain after all principal members are grounded. Cangjie repo-wide and fixture rechecks previously showed repeated pre-finalize requeue loops despite complete principal `.cj` coverage. Status: complete for the principal-member path in D1-F6d. Tier-1 now suppresses localizer debt when every principal `member_set` row has a read-backed file:line anchor, including decorated member locations and case-drifted repo paths; support/navigation debt remains advisory unless a typed principal obligation consumes it.
- D1-G16: Final-answer synthesis can semantically blend adjacent source-inventory rows even when every row has a correct citation. The D1-F6 Cangjie repo-wide answer cited `extend String` correctly but described it as integer addition / `native_add`, which belongs to the foreign-function row. This is not an eval label problem; it is a typed row-to-visible-summary fidelity gap. Status: partial in D1-F6f. Row attributes/package dimensions now flow through typed row contracts; full row-summary predicate/object fidelity remains queued for a later proof-coverage pass.
- D1-G17: Final-answer rendering can duplicate the same principal member set when multiple structured carriers cover the same rows, for example accepted `aggregate_facts.member_set`, `emit_answer_symbol`, deterministic principal enumeration supplements, and model-authored sections. The Cangjie fixture D1-F6 recheck duplicated each item as both a prose bullet and a bold symbol bullet; the repo-wide earlier run duplicated a generated `public class（Cangjie）` block after patching requested dimensions. Status: open; next batch must make principal member rendering use one typed single-source-of-truth and keep the richest carrier without deleting unique citations or requested dimensions.
- D1-G18: `principal_support_member_coverage` still emits `contract_warning` on functionally correct source-inventory answers. The warning comes from coverage checker / visible-surface carrier mismatch, not missing user-visible rows: the Cangjie repo-wide D1-F6 navigation-strip recheck passed and listed all requested rows, but logged `principal_support_member_coverage violations=6`; fixture passed with `answer_contract_violations=3`. Status: complete for the member-coverage checker in D1-F7a/b: principal coverage now accepts typed citation sidecars when the principal block's visible surface carries the member row, and D1-F7 recheck shows `principal_support_member_coverage violations=0` on both Cangjie cases. Residual `answer_facet_coverage` warning is tracked separately by D1-G21.
- D1-G19: Source-inventory repo-wide routine path remains too expensive and noisy after correctness fixes. The D1-F6 navigation-strip recheck improved Cangjie repo-wide from `read_file=19`, `midloop=13`, `wall_seconds=324` to `read_file=15`, `midloop=5`, `wall_seconds=240`, but still did analyzer broad grep/list work, context pruning, forced support/document reads, and carried ~75k context tokens. Status: open; D1-F7 must finish analyzer navigation priority / forced-read pruning as typed post-analysis authority, not as prompt keywords or per-case path exceptions.
- D1-G20: Repo-wide source-inventory localization can select a narrow fixture subspace as the principal universe and omit other repo-owned source classes. The D1-F7 Cangjie repo-wide run answered with `eval/fixtures/testdata/cangjie_minimal/*` packages (`demo.cart`, `demo.bridge`, `demo.app`) while the case also expects thirdparty corpus packages (`demo.stringext`, `demo.ffi`, `demo.greeter`). This is a source-scope/localization authority gap: all-repo inventory should not stop after one matching source family unless a typed scope contract bounds it there. Status: still open after D1-F7f root-first, construct-surface, and source-quote-repair code. The D1-F7f quote-repair recheck (`eval/results/ir_engine_d1_f7f_quote_repair_cangjie_recheck_20260621_summary.md`) still failed repo-wide Cangjie: root source-inventory reported `cangjie:11` and `thirdparty:14`, but visible principal rows stayed inside the fixture family. This proves the remaining gap is load-bearing source-class/family projection, not first-lens prompt behavior.
- D1-G21: `answer_facet_coverage` can remain as a `contract_warning` after auto-repair even when principal member coverage is clean. The D1-F7 fixture recheck had `principal_support_member_coverage violations=0`, but summary still flagged `contract_warning` with root `answer_facet_coverage:1`. Status: open; next batch must align facet-coverage post-repair accounting with the accepted rich principal row carrier, without hiding real facet omissions.
- D1-G22: Source-inventory rows can hide language construct surfaces that exist in typed repomap symbols. The D1-F7f Cangjie repo-wide audit showed the root source_inventory lens ran, but rendered rows did not expose construct predicates from `repomap.Symbol.Kind` such as `extend`, `foreign-func`, `operator`, or exported/public type surfaces, so the model fell back to parser/test fixtures and missed repo-owned corpus packages. This is a typed projection gap, not a prompt gap. Status: complete as a local projection primitive, but insufficient alone. Construct/package surfaces now enter candidate notes/query matching from typed symbol/file metadata, yet D1-F7f recheck proves rare-family rows can still be excluded before they become visible candidates.
- D1-G23: Final/completion still lacks a single typed complete-inventory carrier. Even when `SourceInventoryObservation` contains the complete row universe, final synthesis and contract accounting can still prefer model-authored aggregate facts or partial row carriers. This is broader than Cangjie: final answers should consume a typed inventory row set as the principal carrier, with aggregate facts and answer-symbols as views over it. Status: open; next batch must make complete `SourceInventoryObservation` the finalizer/contract single-source-of-truth for source-inventory lanes before falling back to model-authored row prose.
- D1-G24: Repo-wide source-inventory pagination/budget can hide low-frequency language/source-class families after construct projection. D1-F7f recheck activated this gap: the root lens carried `candidate_budget_truncated` and `next_cursor=50`, but the principal rows surfaced only fixture Cangjie members while thirdparty Cangjie corpus rows stayed invisible. The fix must use a bounded per-language/source-class/family selection cursor over the existing `SourcePathRole`/`SourceScope` universe instead of raising global candidate limits or adding per-path exceptions. Status: P0 open.
- D1-G25: Active `source_inventory_profile` without an explicit `source_scope_profile` can let analyzer broad-search `RequiredFiles` become the first-lens principal boundary. The D1-F7f recheck still started from `internal/tool/...` parser files because analyzer grep candidates were treated as bounded scopes, even though the user asked for repository inventory. This is a typed scope-authority gap: absence of explicit source scope must mean root-first inventory, not RequiredFiles-first inventory. Status: complete in D1-F7f code, pending eval recheck: `SourceInventoryRequiresRepoWideLens` now treats missing source-scope profile as root-first, and explorer runtime rejects narrowed first-lens calls for this state.
- D1-G26: Structured repair can drop useful typed source-inventory hints when the model emits a partial JSON object. The D1-F7f root-first recheck showed `source_inventory_profile.source_quotes` were present and validated, but `target_roles` was omitted; `emit_analysis` discarded the whole profile, synthesized a low-confidence default, and lost the quotes that should have driven the root lens query. Status: complete in D1-F7f code, pending eval recheck: synthesized source-inventory profiles now preserve validated source quotes from the rejected partial profile.
- D1-G27: Read-mode status rendering can flap on internal no-op state transitions. The Cangjie run `eval/results/cangjie_repomap-20260621-162741` printed repeated `已完成证据收集` / `正在收集证据` / `验证还不够稳，正在补一轮` messages even though metrics showed a single explorer dispatch and the repeated lines happened during accepted-closure auto-complete and stale retry carry-over cleanup. This is a UX/noise gap: user-visible status should be emitted only for new LLM dispatches, real tool dispatches, or changed typed reason codes. Internal auto-complete/no-op requeue transitions must be coalesced into one status-card update. Status: P1 open.
- D1-G28: `SourceClassUniverse` is computed but not yet the load-bearing candidate projector for source-inventory rows. The root lens can know `cangjie:11` and `thirdparty:14` while principal candidate rows still come from the graph-backed production/default surface that excludes thirdparty before auxiliary projection carries enough rows. This is the source-inventory correctness root cause exposed by D1-F7f: auxiliary projection must produce typed candidate rows from the full git-tracked source-class universe, using the existing `SourcePathRole` / `SourceScope` / language data, not a new taxonomy. Status: P0 open.
- D1-G29: Source-inventory truncation and pagination debt is advisory instead of executable. The root lens reports `candidate_budget_truncated` / `next_cursor`, but the scheduler does not turn uncovered requested language/source-class families into bounded follow-up `repo_map(source_inventory)` pages or narrower typed family probes. The model can ignore the cursor and read a partial fixture subtree. This must become typed execution debt, not a prompt hint or rendered markdown instruction. Status: P0 open.
- D1-G30: Final completion can accept a partial source-inventory answer while blocking repair debt remains. The D1-F7f quote-repair run had `repair_debt_principal_blocking_max=4` and still finalized with a fixture-only answer. Finalizer/contract must consume a complete-inventory authority that ties requested source-class/language families, observed candidates, pagination debt, and row coverage into one verdict. Status: P0 open; overlaps D1-G23 but remains separately tracked because it is the finish-gate symptom.
- D1-G31: Analyzer-prescan required-file/support noise can still dominate source-inventory turns before the typed inventory authority takes over. Recent Cangjie runs repeatedly seeded parser/test/docs files through broad prescan support paths, increasing reads, context pruning, and finalizer repair pressure. The fix must demote broad prescan support obligations once a typed inventory lane exists and keep only principal row owners as blocking evidence. Status: P1 open; generalizes D1-G9/D1-G19.
- D1-G32: Broad source-inventory candidate-universe debt is not part of the `emit_investigation_complete` pre-complete gate. D1-F7m family-selection recheck (`eval/results/ir_engine_d1_f7m_family_select_cangjie_recheck_20260621_summary.md`) still accepted `emit_investigation_complete(result_kind=resolved)` after a budget-truncated root source-inventory lens. Existing `SourceInventoryCandidateUniverseCoverageGap` covers exact direct-child/list-files universes, but broad repo-lens `candidate_budget_truncated` rows can remain advisory while the accepted closure claims an exhaustive fixture-only set. Status: P0 open.
- D1-G33: Source-inventory handoff can lose the root lens boundary and drift into support-file scopes. In the same D1-F7m run, the initial root lens had scopes `.` and source_classes including `thirdparty:14`, but the extractor's `Source inventory advisory candidates` block carried scopes such as `docs/design/...`, `internal/skill/defaults.go`, and parser implementation files. Root inventory rows and support implementation rows need separate priorities and consumers. Status: P0 open.
- D1-G34: After a budget-truncated source-inventory lens, the tool surface can allow generic grep that is structurally unable to see auxiliary/corpus files under the active repo-wide inventory scope. The D1-F7m run called three `grep include=*.cj` searches, each returned no matches, then the model read only surfaced fixture files. Follow-up routing should be typed: page/narrow `repo_map(view=source_inventory)` by missing source-class/language family, or use a typed file-list path that explicitly includes auxiliary when the source scope allows it. Status: P1 open.
- D1-G35: Source-inventory visible-row budgets and extractor row limits can still hide rare family representatives even after candidate selection includes them structurally. D1-F7m made family representatives available in source-inventory candidate sets, but relation/extractor summaries still showed generic examples and omitted the thirdparty Cangjie rows needed for closure. The row-set artifact must be consumed by typed family-prioritized views, not only rendered examples. Status: P0 open.
- D1-G36: D1-F7r correctness pass came with poor online convergence. `eval/results/ir_engine_d1_f7r_complete_gate_cangjie_recheck_20260621_summary.md` passed both Cangjie cases, but repo-wide took `wall_seconds=317`, `explorer_iters=21`, `midloop=10`, `repo_map=6`, and `investigation_complete_calls=7`. The completion gate fixed premature closure but did not synthesize the next bounded family follow-up, so the model paged generic function/type inventory and repeatedly retried structured closure. Status: P1 open; implement D1-F7t before widening eval.
- D1-G37: Passing source-inventory answers still ship contract warnings. D1-F7r recheck flagged both Cangjie cases with `contract_warning`; repo-wide had `principal_support_member_coverage violations=1` after finalizer repair. This is residual coverage-accounting drift over typed principal rows and should not be hidden by eval pass. Status: P1 open; extends D1-G21/D1-G18.
- D1-G38: Finalizer can accept principal enumeration citation/member mismatches as a soft advisory. Manual audit of the D1-F7r repo-wide PASS shows visible rows with swapped citations, for example `extend String` rendered with the `extend Cart` fixture citation and `extend Cart` rendered with an unrelated `03_struct_interface.cj` citation. Logs show pre-emit detected the mismatch but accepted it as "soft advisory; no retry requested". Principal source-inventory enumeration rows need a hard typed item-to-citation alignment gate. Status: P0 code complete in D1-F7w, pending eval/manual audit.
- D1-G39: Eval PASS can hide final-answer principal bucket loss. D1-F7w fixture recheck (`eval/results/cangjie_repomap_fixture-20260621-170542/run-1.out`) passed automated checks while the visible answer summarized `3 个 public class` but rendered only extend/foreign func tables. This is an eval/report artifact gap and a finalizer principal-row completeness gap: acceptance must compare every requested typed bucket/member set against visible structured blocks, not only selected oracle dimensions or summary prose. Status: P0 code complete in D1-F7x, pending broader eval.
- D1-G40: Source-inventory completion debt is over-broad for typed construct/language questions. D1-F7w repo-wide recheck showed `emit_investigation_complete` correctly refusing a fixture-only closure, but the downgrade required completing all repo-wide roles (`function,type,constant,field,package`) across all languages/source classes after the model had already read the relevant `.cj` files. D1-F7x repo-wide recheck (`eval/results/ir_engine_d1_f7x_repo_wide_recheck_20260621_summary.md`) failed again after 381s with `repo_map=7`, `source_lens=7`, `explorer_iters=18`, `midloop=11`, and missing `demo.stringext/demo.ffi/demo.greeter`; the repair still did not synthesize the minimal typed Cangjie corpus follow-up. The hard gate must shrink debt to the typed requested universe: requested language/source-class/construct families and principal roles that can change the answer surface. Status: P0 partial in D1-F7y: repo-wide repair scope/query projection and narrow-exact-universe short-circuit guard are coded; eval and typed family actuator still pending.
- D1-G41: Deterministic final-answer supplements can duplicate a requested dimension after the primary table already covers it. D1-F7x fixture recheck fixed the missing public-class rows, but still rendered an extra `package 声明（3）` section even though the main table already listed package declarations per row. This is user-visible noise, not a correctness blocker: supplement generation should consume typed visible-row coverage before adding a bucket-level section. Status: P2 open.
- D1-G42: Source-inventory repair handoff can pollute the answer universe with implementation/support scopes. D1-F7x repo-wide recheck emitted repair scopes including `.`, `docs/design/eval_full_sweep_20260526.md`, `internal/skill/defaults.go`, `internal/tool/repomap/index/cangjie_parser.go`, `internal/tool/repomap/index/extract_cangjie.go`, and source-inventory implementation files. The model then queried and emitted evidence for Codrax parser/tool functions even though the user asked about repository Cangjie declarations. Root inventory carriers, support implementation files, and repair execution scopes need separate typed priority lanes and consumers. Status: P0 partial in D1-F7y: completion repair now emits repo-wide `scopes=["."]` plus typed query for repo-wide lanes; broader P0/P1/P2 handoff split remains open.
- D1-G43: A complete bounded source-inventory observation can be mistaken for repo-wide completion. In the same D1-F7x run, the final answer explicitly scoped the result to `eval/fixtures/testdata/cangjie_minimal/` and omitted repo-owned Cangjie corpus packages, while the original request had no typed bounded scope. A narrow `complete=true` lens must discharge only that bounded scope; repo-wide lanes may finish only when the source-class universe proves every requested family is covered, explicitly out of scope, or blocked by a typed unavailable/budget reason. Status: P0 partial in D1-F7y: repo-wide incomplete observations no longer accept `SourceInventoryAcceptedClosureCoversExactUniverse` as a finish short-circuit; full source-class family coverage authority remains open.
- D1-G44: Extractor tool-obligation hints can conflict and cause long self-debate / invalid tool calls. D1-F7x repo-wide extractor context said this dispatch does not require `emit_answer_symbol`, while the multi-topic symbol enumeration and hypothesis lane still led the model to debate the obligation, call `emit_answer_symbol`, and retry after a non-positive citation error. Tool obligations need a single deterministic compiler and strict repair layer so the model receives one precise JSON surface per stage. Status: P1 open; prompt/hint hygiene issue, not a user-keyword routing fix.
- D1-G45: Principal item citation/member alignment can still regress after the hard gate. D1-F7x repo-wide final answer cited `Cart` public class with the `extend Cart` line (`Cart.cj:30`) even though the class row should cite `Cart.cj:14`. This indicates the D1-F7w citation hard gate is not yet consuming the final row text/member/support-ref alignment in every carrier shape. Status: P0 open.
- D1-G46: Eval efficiency flags can miss commercially bad convergence. D1-F7y repo-wide recheck passed with flags `0/2`, but still used `read_file=11`, `repo_map=4`, `list_files=5`, `explorer_iters=18`, `midloop=7`, `max_context_tokens_est=71087`, and wall=288s. The eval harness needs typed performance/advisory thresholds aligned with user-facing smoothness, not only correctness/contract flags. Status: P1 open.
- D1-G47: Query-filtered requested universe is not yet a first-class completion authority. D1-F7y repair eventually found both `internal/thirdparty/tree-sitter-cangjie/corpus/sources` and `eval/fixtures/testdata/cangjie_minimal`, but only after the model reasoned through root generic inventory debt, retried scoped lenses, listed `.cj` files, and reread files. A complete root `repo_map(view="source_inventory", query=<typed source quotes>, scope=".")` should produce a typed requested-universe closure that can discharge generic all-role repo debt without model self-correction loops. Status: P0 open.
- D1-G48: Deterministic supplemental surface-term table can reintroduce path/citation drift after the main answer is correct. D1-F7y repo-wide final answer's primary sections were correct, but the auto-supplement table duplicated main rows, lowercased `Bridge.cj`/`Cart.cj`, and rendered `Cart` as "one of ...:14, ...:30". This combines duplicate-section noise (D1-G41) with residual row citation/member ambiguity (D1-G45). Status: P0 code complete in D1-F7ag for source-inventory covered-row dedupe and stable display-location rendering; representative eval/manual audit still required, and broader row citation/member ambiguity remains tracked by D1-G45.
- D1-G49: Read loopkernel `LoopActionAddProof` was scaffold/guidance, not scheduler load-bearing: the original consumers only rendered proof guidance/checkpoint text (`read_stage_retry.go`) and shadow comparison, while parallel explore treated `ProofCoverageCovered` as handoff sufficiency. Status: P0 code complete in D1-F8b for typed one-shot next-action carrier and retry/window consumption; representative eval/manual UX audit still required before closing because dashboard must not mark it commercially complete until real runs show lower/no extra noise and preserved proof handoff.
- D1-G50: AnalyzeRefine optional node entered production topology without read E2E golden. `ensureAnalyzeRefineNode` emits a bounded one-shot `NodeProbe` gated by `CritProgressReplanRequired`, and scheduler unit tests cover optional dispatch, but no read E2E proves the new branch does not add runaway rounds/noise in realistic read loops. Status: P0 open; implement D1-F8c before marking Stage 2 commercially closed.
- D1-G51: Read run snapshot is a durable substrate but not resumability. `ReadRunSnapshot` and `ReadRunSnapshotStore` have typed store/load/list tests, but there is no production snapshot writer/reader in the read scheduler/REPL auto-resume path, and no replay consuming `NodeExecStatus`/read ranges as execution seed. Status: P1 open; implement D1-F8d after B1/B3 stability is preserved.
- D1-G52: `IngestRound` is production-ingested but not yet the single closure/handoff write authority. `applyStageOutput` now calls `EvidenceClosure.IngestRound` for tool-result-derived read coverage / accepted evidence, but other truth-set merges and direct Mutable setters still update related read artifacts. This is a residual N-writer handoff risk rather than a shadow-only failure. Status: P1 open; implement D1-F8e by routing additional accepted-evidence/progress/source-inventory carriers through typed reducer surfaces or explicitly documenting non-overlapping ownership.
- D1-G53: Hot-file ratchet slack can hide small regressions. Fast-push audit found `scheduler.go` actual 747 vs ratchet 799 and `orchestrator.go` actual 9395 vs ratchet 9402. Status: complete in D1-F8a by pinning ratchets to current actual values, then refreshed in D1-F8b to `orchestrator.go=9397` for the two-line typed next-action wiring; focused ratchet test passed.

Tasks:
- Explore `SourceInventoryLensExecutionGapForContext`, `PublishSourceInventoryObservationFromTypedRequest`, source-inventory optional nodes, read retry hints, and final answer caveat materialization before editing.
- Promote the existing typed source-inventory lens-execution gap earlier in the explore loop as a soft-but-load-bearing next-action hint: when active source-inventory profile has no executable lens evidence, guide the next tool call toward bounded `repo_map(view="source_inventory")` / `list_files` before broad shell/file reads.
- Add a typed read-mode command-policy guard for repo-local file discovery commands that parse as broad `find`/shell path enumeration and have an equivalent structured tool. The guard must consume parsed command shape and active tool context, not user prose.
- Split enum final-surface caveats into blocking/repaired/audit-only/displayable states. For complete member sets whose rows have direct read-file citations, suppress generic weak-evidence supplements and keep only real scope caveats.
- Unify principal member carrier semantics so every code path that evaluates visible structured rows consumes the same typed predicate; `section.items`, list items, and table rows must not disagree between evidence view, pre-emit repair, and post-emit contract checks.
- Preserve accepted relation/member dimension labels as deterministic structured carriers when the principal member rows are otherwise covered but the dimension itself would disappear from the answer surface.
- Add a follow-up design task for JSON/repair-layer normalization: malformed object fragments that can be repaired deterministically should be normalized once with typed repair telemetry; unsupported fragments should fail fast with a single precise hint.
- Add a follow-up design task for duplicate principal carrier collapse: when multiple principal blocks cover the same accepted enumeration member set, keep the richest carrier and demote/drop mechanically duplicated rows without deleting unique citations or user-requested dimensions.
- Complete forced-read gating for typed source-inventory scopes: when a source-inventory lens has executed and yielded a complete bounded source scope, `phase1_unread` treats only files inside that typed scope as completion-blocking; unrelated grep/list_files matches stay advisory.
- Complete source-scope-backed inventory normalization: when a typed source-scope enumeration is present but category predicates are incomplete, synthesize the missing source-inventory lane only if typed relation/call-chain surfaces do not own the answer shape.
- Complete bounded analyzer prescan projection: low-confidence synthesized source-inventory lanes may only project required_files from bounded same-scope candidates, and deterministic `list_files` candidates outrank grep matches.
- Complete closure-carried lens marker consumption: lens execution checks must read the unified typed observation from mutable/TurnA/closure rather than only the local mutable observation.
- Design the next principal/support localization cutover: pre-finalize source localization floors should hard-block only unresolved principal answer surfaces, while support/navigation files stay advisory unless a typed answer obligation consumes them.
- Add tests proving exact source-inventory enumerations do not show generic weak evidence notes once direct citations cover every listed member.

Validation:
- Focused tests around source-inventory lens-execution preflight, `exec_command` broad file-discovery policy, and answer-document enum caveat suppression.
- Rerun `eval/cases/harmony/arkts_repomap.case` and `eval/cases/harmony/cangjie_repomap_fixture.case`; target is not only PASS but lower explorer iterations, no broad `exec_command find`, and no generic weak-evidence caveat on fully grounded rows.

---

## 9. 规划进展 Ledger

| Date | Batch | Status | Evidence / note |
|---|---|---|---|
| 2026-06-21 | Plan refresh | complete | 对 `398a32303b` 复核：v2 大方向成立；补充当前 HEAD 承重矩阵与 A0-D2 可执行任务列表。 |
| 2026-06-21 | A1 Extract dispatch golden pin | complete | Added golden tests for `extract_input_ready=false` skip-complete and `extract_input_ready=true` StageExtract dispatch using compiler-emitted stage nodes. Focused `go test ./internal/orchestrator -run 'TestE2E_ReadMode_.*Extract|TestStageMappingFirstClassExtractSkip'` and `go test ./internal/analysis/compiler ./internal/analysis/criterion` passed. |
| 2026-06-21 | A2/D1-F8a Biting ratchet pin | complete | Ratchet pinned to current counts after fast-push audit and D1-F8b wiring: `evidence_closure.go=2636`, `scheduler.go=747`, `orchestrator.go=9397`; failure message now requires splitting concern-specific code or updating this ledger before expanding budget. `go test ./internal/orchestrator -run TestIRDeliveryHotFileLineRatchet` passed. |
| 2026-06-21 | B1 NodeExecStatus load-bearing cutover | complete | `EvidenceClosure.NodeExecStatus` is now the closure-first decision-read authority for scheduler/orchestrator status checks; `graphState.status` is retired after closure attach and remains only nil-closure bootstrap fallback. `rg 'state\\.status\\[|s\\.status\\[|\\.status\\[' internal/orchestrator` only finds accessor internals. Focused `go test ./internal/types ./internal/orchestrator -run 'TestGraphState|TestHandleStructurallyEmptyInvestigation|TestE2E_ReadMode_.*Extract|TestStageMappingFirstClassExtractSkip'` and `go test ./internal/orchestrator -run 'TestGraphState|TestRunTaskGraph|TestE2E_ReadMode|TestMode_DefaultIsRead|TestRunMode_ReadByteIdentical'` passed. |
| 2026-06-21 | B3 IngestRound production reducer cutover | complete | `applyStageOutput` now calls production `ingestEvidenceRound`, mutating the real closure via typed `EvidenceClosure.IngestRound` while preserving `ToolResults` history append semantics. Tests cover production read coverage, accepted-evidence carrier idempotence, and existing truth-set merge behavior. `go test ./internal/types ./internal/tool/ground ./internal/orchestrator -run 'IngestRound|ApplyStageOutput|ReadCoverage|AcceptedEvidence'` passed. |
| 2026-06-21 | B2 Read run snapshot substrate | scaffold complete / load-bearing open | Added typed `ReadRunSnapshot` projection/persistence plus `ReadRunSnapshotStore` under `<planDir>/read_runs/`. Snapshot carries TaskGraph identity, node exec statuses, read coverage, accepted-evidence refs, source-inventory authority, and progress decision from typed carriers only. Automatic resume and production producer/consumer remain explicitly out of batch. `go test ./internal/types ./internal/repl -run 'ReadRunSnapshot|EvidenceClosure.*Snapshot'` and `go test ./internal/types ./internal/repl` passed. |
| 2026-06-21 | C1/D1-F8b Read loopkernel add-proof next-action cutover | code complete / eval pending | Added a typed `readLoopNextActionDecision` carrier that consumes `ReadProofGuidance.RecommendedAction` and `ToolRouteForAction`; explore fact retry and transient retry now record the one-shot typed add-proof decision before requeue, and the next window hint consumes the decision as a narrow proof-collection continuation. Checkpoint rendering now derives route metadata from the decision. Focused `go test ./internal/orchestrator -run 'ReadLoopNextAction|ExploreTransientRetryCheckpoint|ReadLoopAddProof|ExploreFactRetryContinuation|IRDeliveryHotFileLineRatchet'` and `go test ./internal/loopkernel -run 'ReadProofGuidance|ReadLoopShadow|ToolRoute|Advance'` passed. Representative eval/manual UX audit still required before D1-G49 closes. |
| 2026-06-21 | C2 AnalyzeRefine optional node cutover | partial / E2E guard open | Compiler now emits a bounded optional one-shot `NodeProbe` gated by typed `progress_replan_required`, with immutable `analysis_refinement_handoff` output. Scheduler behavior stays generic: it only consumes analyzer-authored optional topology and typed criterion env; no runtime DAG append, no prompt/prose hard routing, no new TaskNodeType. Unit/focused tests passed, but fast-push audit confirmed this new read-loop branch lacks a read E2E golden and eval runaway guard, so it is not commercially closed. |
| 2026-06-21 | D1 Source-inventory correctness eval intake | complete | Explored current source-class universe, repo-truth absence wrapper, bounded source-inventory kernel, convergence tripwire, ArkTS/Cangjie tool tests, and harmony eval cases. Focused `go test ./internal/types ./internal/tool ./internal/tool/repomap ./internal/orchestrator -run 'SourceInventory|SourceClass|RepoMapSourceInventory|AbsenceRepoTruth|ClassUniverse|Convergence'` passed. |
| 2026-06-21 | D1 Source-inventory correctness eval recheck | complete | `CODRAX_BIN=/Users/han/opt/codrax/codrax CASES='eval/cases/harmony/arkts_repomap.case eval/cases/harmony/cangjie_repomap.case eval/cases/harmony/cangjie_repomap_fixture.case' PARALLEL=2 RUNS=1 TIMEOUT=1800 SUMMARY=eval/results/ir_engine_d1_source_inventory_20260621_summary.md bash eval/convergence_audit.sh` passed 3/3. Fresh evidence closes the stale "repo_map=0/advisory" failure label, but not the routine-path UX/perf gap: `arkts_repomap` took 305s, 14 explorer iterations, 6 midloop injections, 7 reads, 3 list_files, and displayed a generic weak-evidence caveat; `cangjie_repomap` took 16 explorer iterations, 8 midloop injections, and used broad `exec_command find`; `cangjie_repomap_fixture` passed but still showed generic weak-evidence caveat after direct citations. |
| 2026-06-21 | D1-F1 source-inventory routine-path noise reduction | mostly complete / D1-G7,D1-G9 open | Follow-up split from D1: typed source-inventory next-action guidance, broad file-discovery shell guard, final enum caveat severity split, carrier unification, dimension preservation, orphan annotation repair, forced-read false-positive gating, source-scope inventory normalization, bounded prescan projection, closure-carried lens markers, principal/support localization suppression, and typed generic-caveat suppression have landed. Remaining open gaps before D2 clean baseline: D1-G7 duplicate principal carrier collapse and D1-G9 analyzer/explorer broad-grep-first noise. |
| 2026-06-21 | D1-F1a final enum surface noise | complete | Suppresses repaired/audit-only enum-depth caveats only when compiled principal enumeration rows all have direct citations, while keeping true hallucination/omission/count-drift caveats. Source-inventory requests for name/location/count no longer receive deterministic surface-term supplements such as tag labels unless typed requested fields include `summary` or `values`. Focused `go test ./internal/orchestrator ./internal/tool -run 'Caveat|PrincipalSupportSurfaceTerm|AnswerDocument|Enumeration'` passed. |
| 2026-06-21 | D1-F1b exec_command file-discovery repair | complete | Read-mode `exec_command` now refuses broad repo-local `find` path enumeration before execution and emits typed repair `exec_command_file_discovery_use_typed_tools`, steering source-inventory lanes to `repo_map(view=source_inventory)` or bounded `list_files`. Deterministic count/measurement commands such as `find . -name "*.go" \| wc -l` remain allowed. Focused `go test ./internal/tool -run 'ExecCommandFind|ExecCommand_ReadModeShellWriteGate|ExecCommand$|DecideWriteModeExecPermission'` passed. |
| 2026-06-21 | D1-F1c source-inventory lens-first probe | complete | Added typed criterion `source_inventory_lens_missing`, durable `SourceInventoryLensExecuted` observation authority, and a compiler-authored bounded optional probe so active source-inventory profiles get an early source-inventory lens objective when pre-explore auto-publish did not already execute one. This distinguishes class-universe-only absence seeds from executable repo lenses and keeps false optional nodes out of finalize blocking. Affected package tests passed: `go test ./internal/types ./internal/analysis/criterion ./internal/analysis/compiler ./internal/analysis/gate ./internal/orchestrator ./internal/tool`. |
| 2026-06-21 | D1-F1d auto-observed lens marker | complete | Eval recheck showed pre-explore auto-observe could return a lens result without durable `repo_lens:tool_query` observation state, so scheduler/completion still treated the lens as missing. Added a small marker concern that persists only the typed execution provenance/source-class view, not broad navigation member rows. `source_inventory_reconcile.go` remains below its LOC ratchet (`3927 <= 3928`). Affected package tests passed: `go test ./internal/types ./internal/analysis/criterion ./internal/analysis/compiler ./internal/analysis/gate ./internal/orchestrator ./internal/tool`. |
| 2026-06-21 | D1-F1e principal enum carrier unification | complete | Shared `AnswerBlockKindRendersStructuredItems` now drives principal evidence view, pre-emit checks, support-member coverage, and principal enum carrier predicates, so `section.items`, list items, and table rows no longer disagree. Focused tests passed: `go test ./internal/tool -run 'NormalizeAggregateMemberSetCarriers|RelationMemberSet|AggregateMemberSet|PrincipalEnumeration'`, `go test ./internal/types -run 'SemanticView|Enumeration|PrincipalSupportMember|BlockRequirement'`, plus `go test ./internal/types ./internal/tool ./internal/orchestrator`. |
| 2026-06-21 | D1-F1f relation dimension label carrier | complete | Accepted relation/member dimensions now get a deterministic structured carrier when rows are otherwise covered but the dimension label would be lost. The carrier consumes typed aggregate facts and normalized label keys, not user/request prose or model rationale. Harmony recheck passed 2/2: `arkts_repomap` PASS and `cangjie_repomap_fixture` PASS via `eval/results/ir_engine_d1_f1_dimension_label_recheck_20260621_summary.md`; Cangjie has no `contract_warning`, ArkTS still has a `finalizer` flag from one JSON-shape retry and is tracked as D1-G6. |
| 2026-06-21 | D1-F1g orphan annotation repair | complete | `emit_answer_document` now merges annotation-only orphan blocks into the previous complete block, preserving `claim_uses` / `facet_ids` / `surface_role` while still rejecting visible missing-identity blocks. Focused tests passed: `go test ./internal/tool -run 'EmitAnswerDocumentV2_.*(Orphan|MissingBlockID|InvalidBlockKind|Nested|String|Blocks)'`; package regression passed: `go test ./internal/tool ./internal/orchestrator`. ArkTS recheck `eval/results/ir_engine_d1_g6_arkts_recheck_20260621_summary.md` passed and reduced finalizer retries to zero (`fin_it=1`, `fin_reject=0`), but exposed D1-G7 duplicate-carrier noise and D1-G8 forced-read false-positive noise. |
| 2026-06-21 | D1-F1h source-inventory forced-read scope gate | complete | `phase1_unread` now consumes the existing typed source-inventory lens/scope authority: after `SourceInventoryLensExecuted` and a complete bounded `BoundedSourceEnumerationScopeFiles` universe, only in-scope files can become hard forced-read blockers; out-of-scope exact-rank string/comment matches remain advisory. Focused tests passed: `go test ./internal/tool -run 'Phase1Unread|SourceInventory'`, `go test ./internal/types -run 'SourceInventory|SourceScope|RequiredFile'`, and `go test ./internal/tool ./internal/orchestrator`; `make` passed. ArkTS recheck `eval/results/ir_engine_d1_g8_arkts_recheck_20260621_summary.md` passed with flags 0/1, `read_file=3` (down from 11), `explorer_iters=8` (down from 19), `finalizer_rejects=0`, and no post-lens `phase1_unread` forced reads of false-positive production Go files. One early `source-inventory lens has not run` downgrade remains tracked under D1-G1. |
| 2026-06-21 | D1-F1i source-scope inventory normalization + bounded prescan projection | complete | Analyzer normalization now reuses typed `IsTypedSourceEnumerationShape`, `SourceInventoryLaneConflictsWithRelationFlow`, and existing `SourceScopeProfile` instead of adding a new taxonomy. Low-confidence synthesized source-inventory lanes project required_files only from bounded same-scope candidates; deterministic `list_files` candidates are consumed before grep noise; explicit auxiliary exclusions continue to block fixture/corpus projection. Lens execution checks now consume `SourceInventoryObservationFromMutable`, so closure-carried repo-lens markers satisfy the gate. `phase1_unread` accepts a complete typed source-class universe as source-inventory scope coverage. Focused `go test ./internal/tool ./internal/orchestrator ./internal/types` and `make` passed before eval. Recheck `eval/results/arkts_repomap-20260621-132809/summary.md` passed 1/1 with `repo_map=1`, `list_files=3`, `source_lens=1`, zero contract/finalizer flags. Manual log audit still found D1-G9/D1-G10: broad analyzer grep came before typed file discovery, and support-file localization caused repeated pre-finalize "验证还不够稳" loops despite principal answer correctness. |
| 2026-06-21 | D1-F2 principal/support localization authority | complete | Pre-finalize read-localizer follow-up now consumes a typed principal member-set localization projection: when every request-aware principal `aggregate_facts.member_set` member carries an already-read file:line anchor, support/navigation localization debt is suppressed; missing principal member anchors or explicit missing owner paths still requeue. Focused `go test ./internal/orchestrator -run 'TestCheckTier1Floor_ReadLocalizerFollowup|TestCountTier1Evidence|TestCountGroundingHealth'` and `go test ./internal/types ./internal/tool ./internal/orchestrator` passed. ArkTS recheck `eval/results/arkts_repomap-20260621-133920/summary.md` passed 1/1; logs contain `principal_member_set_localization_complete` suppression and no `pre-finalize read localizer follow-up`, no `pre-finalize Tier-1 floor failed`, and no user-visible "验证还不够稳" loop. Manual answer audit surfaced D1-G11 generic supplemental caveat noise. |
| 2026-06-21 | D1-F4 typed final-answer caveat severity | complete | Generic `ViolSelfContradiction` caveats are now telemetry-only when the accepted principal enumeration is fully cited in both typed aggregate display rows and the visible `AnswerDocumentV2` principal enumeration surface. Specific reviewer contradictions with structured `SUMMARY/BODY` claims still render user-visible caveats, and real typed scope/uncertainty caveat blocks remain visible. Focused `go test ./internal/orchestrator -run 'Caveat|AppendUserCaveats|AppendSoftContractCaveats'`, `go test ./internal/tool ./internal/orchestrator -run 'Caveat|FinalAnswer|Contract|Semantic|Termination'`, `go test ./internal/types ./internal/tool ./internal/orchestrator`, `make`, and `go test ./...` passed. ArkTS recheck `eval/results/ir_engine_d1_f4_arkts_recheck_20260621_summary.md` passed 1/1 with flags 0/1, `repo_map=2`, `list_files=2`, `source_lens=1`, `finalizer_iters=1`, `finalizer_rejects=0`; manual output audit found no generic `补充说明` / `答案前后` note and preserved only the typed scope boundary block. |
| 2026-06-21 | D1-F5 source-inventory lens-first explore cutover + typed surface terms | partial / explorer complete, analyzer follow-up open | Added typed `ExploreToolSurface`, source-inventory probe windows, runtime pre-release tool boundary, post-lens surface release, and source-inventory `surface_terms` through advisory/observation/ledger/final-answer materialization. Focused tests passed: `go test ./internal/types ./internal/context ./internal/agent ./internal/tool ./internal/orchestrator`; `make` passed. Focused ArkTS eval `eval/results/ir_engine_d1_f5_typed_surface_arkts_recheck_20260621_summary.md` passed with `repo_map=1`, `source_lens=1`, `read=6`, `explorer_iters=5`, `explorer_dispatches=1`, flags 0/1. Three-case recheck `eval/results/ir_engine_d1_f5_typed_surface_three_cases_20260621_summary.md` produced ArkTS PASS, Cangjie fixture PASS, Cangjie repo-wide FAIL by oracle `missing_section:package`; manual audit shows package paths are present, so D1-G14 records requested-dimension/eval-shape mismatch. Remaining open: analyzer broad prescan half of D1-G9, D1-G7 coverage noise, D1-G15 repeated support/navigation validation loops. |
| 2026-06-21 | D1-F6 source-inventory dimension/support-loop closure | mostly complete / D1-G16-D1-G19 open | Eval runners now score requested dimensions semantically (`EXPECT_DIMENSIONS` + value/regex obligations) instead of requiring literal section labels. Product carrier preserves graph-derived package/module scope as typed `SourceInventoryObservationMember.Attributes`, projects it through `AnswerSurfacePlan` and `EnumerationDisplayRow.Attributes`, and renders row attributes in finalizer `Principal Enumeration Rows` without parsing final prose; repo_map navigation render strips those graph-package row-context attributes by typed reason code so package dimensions do not inflate candidate counts or attribute-role summaries. `phase1_unread` now restricts forced-read blockers to request-aware principal source-inventory member scope, and Tier-1 localizer suppression accepts decorated member locations plus case-drifted repo paths when the real read/evidence anchors are already present. Focused tests passed: `bash eval/runner_lib_test.sh`; `go test ./...`; `make`; and source-inventory focused tests. Rechecks: `eval/results/ir_engine_d1_f6_package_attr_cangjie_recheck_20260621_summary.md` PASS 2/2 after package carrier; `eval/results/ir_engine_d1_f6_casefold_tier1_cangjie_recheck_20260621_summary.md` PASS 2/2 with localizer suppression in both logs; `eval/results/ir_engine_d1_f6_navigation_strip_cangjie_recheck_20260621_summary.md` PASS 2/2 after navigation render strip. Manual audit: package dimensions are preserved, repeated "验证还不够稳" localizer loop is gone, and no repo_map candidate-count regression remains. Remaining open: row-summary predicate drift (D1-G16), duplicate principal member carriers (D1-G17), contract-warning coverage mismatch (D1-G18), and repo-wide cost/context noise (D1-G19). |
| 2026-06-21 | D1-F7a/b principal coverage sidecar kernel | partial / member coverage complete, scope/facet gaps open | Extended the shared `MissingPrincipalSupportMembers` kernel so principal coverage accepts a typed citation sidecar when the same principal block's visible surface carries the member row; it still requires the citation to cover that support member, so citation-only rows remain missing. Focused tests passed: `go test ./internal/types -run 'MissingPrincipalSupportMembers|PrincipalSupportMember|AnswerSupportMember'`, `go test ./internal/orchestrator -run 'ValidatePrincipalSupportMemberCoverage|Contract|Caveat'`, `go test ./...`, and `make`. Recheck `eval/results/ir_engine_d1_f7_coverage_sidecar_cangjie_recheck_20260621_summary.md`: fixture PASS with `principal_support_member_coverage violations=0` but residual `answer_facet_coverage` warning (D1-G21); repo-wide FAIL by missing expected package dimensions because source localization selected `eval/fixtures/testdata/cangjie_minimal` as the principal universe and omitted thirdparty corpus packages (D1-G20). |
| 2026-06-21 | D1-F7m source-class family candidate selection | partial / tool candidate selection complete, completion-handoff gaps open | Added family-prioritized source-inventory graph candidate selection and a mixed fixture/thirdparty/noisy-production regression test. Focused source-inventory tests and `make` passed. Recheck `eval/results/ir_engine_d1_f7m_family_select_cangjie_recheck_20260621_summary.md`: fixture PASS, repo-wide still FAIL (`missing_dimension:package:demo.stringext/demo.ffi/demo.greeter`). Manual log audit shows the root source-inventory lens is budget-truncated and the model still closes on fixture-only aggregate facts; new gaps D1-G32/D1-G33/D1-G34/D1-G35 were recorded before continuing code changes. |
| 2026-06-21 | D1-F7r broad-inventory pre-complete gate | partial / correctness pass, commercial polish open | Added `source_inventory_completion` downgrade lane and pre-complete gate for budget-truncated/incomplete source-inventory observations. Focused precomplete/source-inventory tests and `make` passed. Recheck `eval/results/ir_engine_d1_f7r_complete_gate_cangjie_recheck_20260621_summary.md`: Cangjie fixture PASS and repo-wide PASS, proving premature fixture-only closure is blocked. Manual audit recorded remaining gaps before continuing: long loop/high cost (D1-G36), residual contract warnings (D1-G37), and finalizer principal item citation mismatch accepted as soft advisory (D1-G38). |
| 2026-06-21 | D1-F7w principal item citation hard gate | complete / follow-up gaps open | Pre-emit `ViolCitation` hints for structural `citations[]` and `blocks[].items[].citation_ref` now hard-fail inside the same `emit_answer_document` dispatch instead of flowing to soft caveat/advisory. Focused `go test ./internal/tool -run 'PreEmit|Citation|ItemCitation|SourceInventory'`, `go test ./internal/tool ./internal/agent ./internal/orchestrator ./internal/types -run 'AnswerDocument|Citation|SourceInventory|PreEmit|Contract|Downgrade|Principal'`, and `make` passed. Recheck `eval/results/ir_engine_d1_f7w_citation_hard_gate_cangjie_recheck_20260621_summary.md` PASS 2/2, flags 0/2. Manual audit: repo-wide answer preserves extend/foreign/class rows with matching citations; no "soft advisory" citation mismatch remains. New open gaps recorded before further code changes: fixture PASS still lost the visible public-class table (D1-G39), and repo-wide completion debt is still over-broad/slow for typed Cangjie construct inventory (D1-G40, D1-G36). |
| 2026-06-21 | D1-F7x requested-bucket visible completeness gate | complete / repo-wide follow-up failed | Fixed source-inventory aggregate demotion so missing/all/unknown source-scope requests do not let analyzer `RequiredFiles` shrink repo-wide principal member sets. Focused `go test ./internal/types -run 'SourceInventory|PrincipalAggregateMemberSetFactRefs|AggregateFactRoles|SourceScope'`, `go test ./internal/tool -run 'PrincipalEnumeration|SourceInventory|EmitAnswerDocument|PreEmit'`, and `make` passed. Recheck `eval/results/ir_engine_d1_f7x_fixture_visible_bucket_recheck_20260621_summary.md` PASS 1/1, flags 0/1; manual audit confirmed the main visible table now includes extend, foreign func, and all three public class rows. Repo-wide recheck `eval/results/ir_engine_d1_f7x_repo_wide_recheck_20260621_summary.md` FAIL 0/1 after 381s (`repo_map=7`, `source_lens=7`, `explorer_iters=18`, `midloop=11`) with missing `demo.stringext/demo.ffi/demo.greeter`; manual audit recorded D1-G42/D1-G43/D1-G44/D1-G45 before further implementation. |
| 2026-06-21 | D1-F7y requested-universe completion repair projection | correctness pass / efficiency follow-up open | Completion repair now derives repo-wide follow-up scopes from typed source-inventory request scope instead of merged observation/support scopes: repo-wide incomplete lanes emit `scopes=["."]` and validated source-quote `query="..."`, and `SourceInventoryAcceptedClosureCoversExactUniverse` no longer short-circuits incomplete repo-wide inventory from a narrower exact universe. Focused tests passed: `go test ./internal/tool -run 'SourceInventoryResolvedRequiresCompleteInventory|SourceInventoryRepoWideRepairIgnoresSupportScopes|SourceInventoryLensExecutionRepoMapCallShape'`, `go test ./internal/tool -run 'PreCompleteCheck_SourceInventory'`, `go test ./...`, and `make`. Recheck `eval/results/ir_engine_d1_f7y_repair_scope_recheck_20260621_summary.md` PASS 2/2, flags 0/2. Manual audit: repo-wide answer now includes both thirdparty corpus and eval fixture Cangjie rows (`extend String/Cart`, both `native_add`, 8 public classes), so D1-G40/G42/G43 correctness symptoms are closed for this case. New open follow-ups recorded before further code changes: high cost/rounds despite PASS (D1-G46), missing query-filtered requested-universe completion authority (D1-G47), and noisy supplemental surface-term table with path/citation drift (D1-G48). |
| 2026-06-21 | D1-F7ag supplemental surface-term carrier cleanup | code complete / eval pending | Reused the existing principal support-member coverage authority as the supplement dedupe gate: when a source-inventory primary table/list already has a typed visible carrier plus matching citation for a support member, deterministic surface-term supplements do not add a duplicate user-visible row. Supplement rows that are still needed render `StableCitationKey()` / citation file:line instead of repair-style `LocationHint()` text such as `one of ...`. Focused tests passed: `go test ./internal/tool -run 'NormalizePrincipalSupportSurfaceTermSupplement|PrincipalSupportMemberCarriers|SourceInventory'` and `go test ./internal/types -run 'AnswerDocumentCoversSupportMember|MissingPrincipalSupportMembers|PrincipalSupportMember|AnswerSupportMember'`. Representative eval/manual audit still required before closing D1-G48; D1-G45 remains the broader final-row citation/member alignment gap. |

### Batch D1-F2: Principal/Support Localization Authority

目标：
- Stop pre-finalize localization floors from treating incidental support/navigation files as principal blockers after the requested member set is complete.
- Preserve correctness for true principal localization gaps: if a requested answer member lacks an owner/evidence anchor, the read loop must still continue.

代码探索点：
- `internal/orchestrator/contract_check_block.go`
- `internal/tool/emit_investigation_complete.go`
- `internal/types/evidence_closure.go`
- principal enumeration carrier helpers in `internal/types` / `internal/tool`
- latest ArkTS logs around `pre-finalize Tier-1 floor failed`

任务：
- Add a typed `LocalizationObligation` projection that tags obligations as `principal`, `support`, or `navigation` from existing structured carriers: `aggregate_facts.role`, accepted evidence role, answer block surface role, source-inventory profile, and required file obligations.
- Make Tier-1 pre-finalize floors consume only unresolved `principal` obligations as hard blocks. `support` and `navigation` obligations may produce soft guidance/status-card notes but must not restart exploration when principal coverage is complete.
- Keep the rule schema-only: no RawRequest matching, no model rationale parsing, no file-name exceptions.
- Add a regression where a source-inventory answer has complete principal `.ets` members plus an incidental support file; the run must proceed to finalize without repeated pre-finalize loops.
- Add a counter-regression where a principal member lacks a read/file anchor; the loop must still block or re-explore.

验证：
- `go test ./internal/types ./internal/tool ./internal/orchestrator -run 'Localization|SourceInventory|Principal|PreFinalize|Tier'`
- Rerun `arkts_repomap` and confirm user-visible repeated "验证还不够稳" loops disappear without reducing principal answer correctness.

退出标准：
- Principal localization hard gates are load-bearing and typed.
- Support/navigation localization gaps are visible to audit/status cards but do not restart the read loop after principal coverage is complete.

### Batch D1-F3: Analyzer Source-Inventory Navigation Priority

目标：
- Reduce source-inventory routine-path noise by making bounded typed navigation the default after the analyzer owns a source-inventory lane.
- Avoid converting this into a hard ban on grep; grep remains valid when the typed lane says literal existence/location is the precise tool.

代码探索点：
- `internal/tool/emit_analysis.go`
- analyzer pre-scan prompt/tool policy surfaces
- `types.RepoMapNavigationPolicy`
- source-inventory optional probe and pre-explore auto-observation

任务：
- Introduce a typed navigation-priority view for analyzer prescan: `source_inventory_lane_active`, `bounded_scope_known`, `literal_presence_needed`, and `candidate_universe_missing`.
- Prefer `list_files` / `repo_map(view=source_inventory)` guidance when `candidate_universe_missing` is true; keep `grep(files_only=true)` as a second-stage literal confirmation tool.
- Record telemetry when broad grep runs before typed source-inventory navigation, so eval dashboards can track improvement without making noisy counts a hard gate.
- Add tests proving relation/call-chain shapes still prefer relation/navigation tools and are not rewritten into source inventory.

验证：
- Analyzer focused tests plus ArkTS/Cangjie source-inventory evals.
- Success metric is fewer analyzer iterations and less Go/string-literal noise; correctness remains the hard acceptance criterion.

退出标准：
- Source-inventory lanes routinely discover the typed file universe before broad string matches, with no user/prose keyword routing.

### Batch D1-F4: Typed Final-Answer Caveat Severity

目标：
- Remove generic user-facing caveats when the typed answer contract, semantic reviewer, finalizer repair counters, and eval verdict are clean.
- Preserve real uncertainty, contradiction, partial coverage, and scope disclosure caveats.

代码探索点：
- final answer supplemental note materialization
- answer contract violation severity / repair-debt state
- semantic quality reviewer outputs
- renderer status-card / caveat injection surfaces

任务：
- Project a typed `FinalAnswerCaveatAuthority` from existing structured signals: contract violations, semantic concerns, finalizer rejects/rewrites, termination floor degradation, source-localization follow-up, proof coverage, and explicit scope disclosure.
- Only render generic uncertainty/inconsistency notes when the authority carries an active non-advisory reason code.
- Keep all true typed caveats: incomplete coverage, absent proof, scope limitation, high-risk uncertainty, and explicit model-emitted caveats with citations.
- Add regression where a clean source-inventory answer with no finalizer/contract/semantic flags emits no generic "may be inconsistent" note.
- Add counter-regression where a real contradiction or unresolved proof debt still emits a caveat.

验证：
- `go test ./internal/tool ./internal/orchestrator -run 'Caveat|FinalAnswer|Contract|Semantic|Termination'`
- Rerun ArkTS/Cangjie source-inventory evals and manually audit final answer surface.

退出标准：
- Clean typed runs do not ship generic uncertainty caveats; real typed uncertainty remains visible.

---

### Batch D1-F5: Source-Inventory Lens-First Navigation Cutover

目标：
- Reduce analyzer/explorer routine-path noise for source-inventory questions by making the typed source-inventory lens and bounded file enumeration the early navigation authority.
- Keep grep/read_file available for precise literal proof, but prevent broad text hits in production support code from becoming the first hard investigation surface when a typed source-inventory lane already owns the answer.

代码探索点：
- analyzer pre-scan tool policy and prompt/tool suggestions for `list_files`, `grep(files_only=true)`, `repo_map`
- `SourceInventoryLensExecutionGapForContext`, `SourceInventoryObservationFromMutable`, and compiler-authored optional source-inventory probe nodes
- explorer retry / completion preflight that currently asks for `repo_map(view=source_inventory)` only after a completion attempt
- required-files projection and source-scope candidate ranking in analysis normalization

任务：
- Define a typed `SourceInventoryNavigationAuthority` from `RequestModel.SourceInventoryProfile`, `SourceScopeProfile`, source-class universe, bounded same-scope candidate files, and current lens-executed observation. This authority must not read `RawRequest`, prompt text, or model rationale.
- In explorer, consume the authority before broad read scheduling: when active and no executable lens evidence exists, prefer a bounded `repo_map(view=source_inventory)` or same-scope `list_files` action before reading grep-derived support files.
- In analyzer, keep pre-analysis tool freedom but downgrade broad grep-derived required-file candidates behind deterministic `list_files`/repo-map candidates when the normalized typed lane proves source inventory ownership.
- Keep precise grep allowed: exact decorator/literal grep may provide evidence, but broad grep hits outside the typed source scope must stay advisory until the lens or same-scope file universe confirms relevance.
- Add telemetry counters for lens-first success, broad-grep demotion, and late-lens recovery so eval can distinguish correctness from routine-path smoothness.
- Add regressions where ArkTS/Cangjie source-inventory requests do not read Go parser/support files before the typed source scope or source-inventory lens has been established.

验证：
- Focused tests around source-inventory navigation authority, required-files ranking, and explorer next-action preference.
- Rerun `arkts_repomap`, `cangjie_repomap`, and `cangjie_repomap_fixture`; target PASS plus lower read_file/explorer iteration counts and earlier `repo_map(view=source_inventory)` execution.

本批执行记录：
- Implemented a typed `ExploreToolSurface` propagated through `BusContext` / `AgentContext` and explore fork dispatch. Source-inventory lens probe windows now run with a constrained first-tool surface: `repo_map(view=source_inventory)`, `emit_evidence`, and `emit_investigation_complete`.
- Added runtime repair for unsupported pre-lens tools and released the constrained surface after a typed successful `repo_map` source-inventory observation, so follow-up `read_file`/grep proof remains available after the navigation lens executes.
- Skipped pre-dispatch required-file forced reads for lens-only windows, preventing incidental support files from preceding the typed source-inventory probe.
- Added typed `surface_terms` to source-inventory advisory, observation, observation ledger, row-set JSONL, and answer-document materialization. The final answer now consumes typed source-inventory member surface labels instead of parsing `note: surface=...` render prose.
- Kept source-inventory convergence ratchets load-bearing by extracting new concern logic into `source_inventory_surface_terms.go`, `source_inventory_advisory_merge.go`, and `source_inventory_observation_merge.go` with explicit LOC ceilings instead of growing the god-file/carrier files.
- Focused ArkTS eval `eval/results/ir_engine_d1_f5_typed_surface_arkts_recheck_20260621_summary.md` passed with `repo_map=1`, `source_lens=1`, `read=6`, `explorer_iters=5`, `explorer_dispatches=1`, `finalizer_rejects=0`, flags `0/1`.
- Three-case recheck `eval/results/ir_engine_d1_f5_typed_surface_three_cases_20260621_summary.md`: `arkts_repomap` PASS, `cangjie_repomap_fixture` PASS, `cangjie_repomap` FAIL by oracle `missing_section:package`. Manual audit of `cangjie_repomap` output shows every requested package path is present, so D1-G14 tracks requested-dimension/eval-shape mismatch rather than functional absence.

Remaining follow-up:
- Analyzer prescan still runs broad `list_files`/`grep` before the typed source-inventory lane in some cases; D1-F6 must make analyzer navigation priority consume the same typed authority without relying on prompt prose.
- Principal/support coverage still emits soft warning noise for some correct source-inventory answers; D1-G7 remains open.
- Cangjie repo-wide runs still show repeated validation loops from support/documentation obligations and missing relation-map navigation coverage; D1-G15 remains open.

退出标准：
- Source-inventory explorer evals still pass, and the typed lens/file universe appears before broad support-file reads in explore logs.
- No new hard gate consumes noisy ranker/grep-count signals; broad hits remain soft/advisory until typed scope authority confirms them.

---

### Batch D1-F6: Source-Inventory Dimension And Support-Loop Closure

目标：
- Close the remaining source-inventory commercial gaps surfaced by D1-F5 without weakening correctness: analyzer prescan noise, requested-dimension presentation, principal/support coverage warnings, and repeated support/navigation validation loops.
- Preserve the project red line: no hard routing from user intent keywords, prompt prose, model rationale, rendered answer text, or noisy grep/ranker counts.

代码探索点：
- Analyzer prescan and post-analysis normalization surfaces: `internal/tool/emit_analysis.go`, `internal/orchestrator/analyzer_autocorrect_rebuild.go`, `internal/types/requested_answer_dimensions*`, `internal/types/answer_evidence_origin.go`.
- Requested-dimension and carrier surfaces: `AnswerAggregateFact.Dimensions`, `AnswerBlockItem.Cells`, `Principal Enumeration Rows`, `types.AnswerSupportPlan`, pre-emit/document normalization helpers.
- Support-loop and navigation severity surfaces: `checkTier1Floor`, `SourceLocalizationReview`, `RepoMapNavigationPolicy`, `read_localizer_navigation_missing`, source-inventory `SourceInventoryObservationFromMutable`, and required-file forced-read gates.
- Eval harness dimension scoring for source-inventory cases, especially `missing_section:package`.

任务：
- **D1-F6a requested-dimension carrier**: derive a typed per-member dimension projection from accepted evidence/aggregate facts and requested-answer dimensions. For package/module/namespace-like dimensions, preserve values as table columns/cells or structured item detail when the principal member rows are otherwise covered. This must consume typed dimensions, evidence `SurfaceTerms`, member notes, or source parser fields; it must not parse final prose for route decisions. Status: complete for source-inventory package/module context via `SourceInventoryObservationMember.Attributes` → `AnswerSurfacePlan.SourceInventoryObservation` → `EnumerationDisplayRow.Attributes` → finalizer `Principal Enumeration Rows`.
- **D1-F6b eval adapter audit**: update eval scoring to distinguish semantic dimension satisfaction from section-label coupling. A case should not fail `missing_section:package` when every listed member visibly carries a package path. Keep strict failure for real missing package values. Status: complete. Shell and PowerShell eval runners now support `EXPECT_DIMENSIONS`, `EXPECT_DIMENSION_VALUES_<DIM>`, and `EXPECT_DIMENSION_REGEX_<DIM>`.
- **D1-F6c analyzer navigation priority ledger**: implement a typed analyzer-prescan telemetry/priority view after `emit_analysis`, not a RawRequest keyword gate. The first implementation may only demote broad grep-derived required files behind deterministic list/repo-map candidates after a typed source-inventory lane exists; pre-analysis prompt freedom remains soft until a typed contract exists.
- **D1-F6d support/navigation loop severity**: when source-inventory principal member sets are complete and all requested dimensions are visible, downgrade missing optional navigation lenses (for example `relation_map`) and documentation/support-file obligations to status-card/audit surfaces unless a typed principal obligation consumes them. Status: complete for pre-finalize read-localizer requeue loops on principal source-inventory member sets, including relative and case-drifted paths.
- **D1-F6e coverage-noise unification**: make principal-support coverage consume the same normalized rich member carrier used by answer-document normalization, so correct source-inventory answers do not ship `contract_warning` solely because the model used a summary/table split.
- **D1-F6f row-fidelity authority**: when a principal source-inventory/member-set row has typed label, location, package/surface terms, and a row-local summary, final answer visible text for that row must be checked against the same row carrier. Summaries may be repaired/demoted when they import another row's predicate/object. This must compare typed row facts and citations, not model rationale or prompt prose. Status: partial: typed package/module row attributes now prevent package loss/path inference; predicate/object drift and patch-time duplicate supplements remain tracked by D1-G16/D1-G17.

验证：
- Focused unit tests for dimension carrier, eval adapter scoring, analyzer required-file demotion, and support/navigation severity.
- `go test ./internal/types ./internal/tool ./internal/orchestrator ./eval/telemetry`
- `go test ./...`
- Re-run `arkts_repomap`, `cangjie_repomap`, and `cangjie_repomap_fixture` with `PARALLEL=2`; target: all functionally correct, no false `missing_section:package`, no repeated validation loop after complete principal coverage, and no generic contract warning for correctly covered source-inventory rows.

退出标准：
- D1-G7/D1-G14/D1-G15 have explicit pass/fail evidence in the ledger.
- Any remaining analyzer broad prescan work is recorded as telemetry-only unless a typed post-analysis contract can safely carry a deterministic demotion.

本批执行顺序：
- First land D1-F6b because the D1-F5 three-case failure is an eval false negative: introduce `EXPECT_DIMENSIONS` with case-declared semantic values, and migrate Cangjie package checks from literal section labels to package-value obligations. This is eval-harness-only and does not affect product hard gates.
- Then re-run ArkTS/Cangjie source-inventory cases. If false verdicts are gone but logs still show repeated support/navigation loops or contract warnings, continue with D1-F6d/D1-F6e product-side typed authority fixes before widening the eval batch.
- Keep D1-F6a as a product-side carrier task unless eval/human audit proves current final answers already preserve every requested dimension through typed aggregate/member rows.
- D1-F6 recheck result `eval/results/ir_engine_d1_f6_dimension_adapter_three_cases_20260621_summary.md`: all three source-inventory cases now PASS after semantic dimension scoring; remaining flags are `contract_warning` on both Cangjie cases. Manual audit adds D1-G16 row-fidelity drift and confirms D1-G15 still exists in repo-wide Cangjie through forced reads of unrelated Go/docs after complete `.cj` coverage.
- D1-F6 product recheck `eval/results/ir_engine_d1_f6_package_attr_cangjie_recheck_20260621_summary.md`: Cangjie fixture and repo-wide cases PASS after source-inventory package attributes entered the typed row carrier. Fixture answer preserved `demo.cart` / `demo.bridge` / `demo.app`, but logs still showed repeated pre-finalize localizer requeues caused by case-drifted member paths.
- D1-F6 localizer recheck `eval/results/ir_engine_d1_f6_casefold_tier1_cangjie_recheck_20260621_summary.md`: Cangjie fixture and repo-wide cases PASS; both logs show `pre-finalize read localizer follow-up suppressed: reason=principal_member_set_localization_complete` and no user-visible "验证还不够稳" loop. Remaining commercial gaps are `contract_warning`, repo-wide high cost (`wall_seconds=324`, `read_file=19`, `midloop=13`, max context ~75k), and D1-G17 duplicate generated principal block after patch retry.
- D1-F6 navigation-strip recheck `eval/results/ir_engine_d1_f6_navigation_strip_cangjie_recheck_20260621_summary.md`: Cangjie fixture and repo-wide cases PASS after graph-package row-context attributes were hidden from repo_map navigation rendering but preserved in structured observation/finalizer row contracts. Focused repomap grouping tests and `go test ./...` passed. Manual audit: requested package dimensions remain visible; localizer loop remains suppressed; repo-wide cost improved to `wall_seconds=240`, `read_file=15`, `midloop=5`, but contract warnings and context pruning remain. Fixture still shows duplicate principal rows from dual aggregate/symbol carriers, so D1-G17 is broader than patch-retry duplication.

---

### Batch D1-F7: Principal Carrier Collapse And Coverage/Cost Authority

目标：
- Turn the D1-F6 PASS results into commercial-quality routine behavior: one visible principal member carrier, no false `contract_warning` on fully listed rows, and lower source-inventory repo-wide cost.
- Keep the hard-gate red line: consume only typed carriers such as aggregate member sets, answer-symbol slates, answer-document block items, evidence anchors, source-inventory observations, read coverage, and navigation authority. Do not parse final prose, model rationale, user keywords, or rendered repo_map markdown.

代码探索点：
- Principal rendering and collapse: `internal/types/enumeration_display_rows.go`, `internal/types/answer_surface_plan.go`, final-answer supplement builders, `internal/agent/answer_document_evaluator.go`, and answer-document normalization helpers.
- Coverage checker: `principal_support_member_coverage` in final answer contract checks, `AnswerSupportPlan`, accepted evidence refs, `SourceInventoryObservation`, and `AnswerAggregateFact` member parsing helpers.
- Cost/noise path: analyzer prescan projection, required-file forced-read gates, `phase1_unread`, `ExploreToolSurface`, `SourceInventoryNavigationAuthority`, tool-history prune telemetry, and eval metrics.

任务：
- **D1-F7a principal member single-source-of-truth**: build a normalized principal row set from accepted `aggregate_facts.member_set`, `emit_answer_symbol`, `AnswerDocumentV2` list/table/section items, and deterministic enumeration supplements. Collapse duplicate carriers when they share typed row identity (`role/name/file/line/package/surface_terms`) and keep the richest visible carrier. This is a typed artifact merge, not prompt/prose matching.
- **D1-F7b coverage checker consumes rich row carrier**: make `principal_support_member_coverage` evaluate the same normalized row set used for final rendering, including package/module attributes, decorated locations, support refs, and case-normalized file paths. Correct fully listed source-inventory answers must not ship advisory `contract_warning`; real missing principal rows must still fail. Status: complete for citation-sidecar + block-visible-surface carriers; residual facet warning is D1-G21.
- **D1-F7c analyzer source-inventory priority cutover**: after `emit_analysis` produces a typed source-inventory lane, demote broad grep/list candidates outside the typed source universe behind deterministic repo-map/list-files candidates. The first version may only reorder/demote required-file obligations and telemetry, not hard-ban precise grep.
- **D1-F7d forced-read and context-budget pruning**: when principal source-inventory rows are complete and read-backed, stop forced reads of unrelated support/docs/parser files from entering completion blockers. Attach a typed status-card/audit note for skipped support debt and keep full trace logs for debugging.
- **D1-F7e eval batch discipline**: run representative evals two-way parallel, six per batch where available. For each batch, record PASS/FAIL, read/repo_map/list_files counts, wall time, max context, contract warnings, context pruning, user-visible duplication, and manual correctness notes.
- **D1-F7f source-scope universe closure**: when a source-inventory request is repo-wide or scope-unknown, consume typed language census/source-class universe and require either all matching source families to be represented or an explicit bounded-scope disclosure. A single matching fixture subtree cannot become the principal universe unless the typed source-scope contract says so.
- **D1-F7g facet-coverage post-repair accounting**: after emit-time repair accepts a principal member carrier, recompute facet coverage from the accepted document and normalized row carrier so stale advisory `answer_facet_coverage` warnings do not survive; real omissions remain violations.
- **D1-F7h construct/package surface projection**: source-inventory rows must project typed construct surfaces from repomap symbols (`extend`, `foreign-func`, `operator`, public/exported type declarations) and package/module context from graph file metadata into candidate notes, query matching, observation members, and final row carriers. This must consume typed parser metadata only; it must not match raw user request text or model rationale. Status: complete in code, pending eval recheck.
- **D1-F7i complete-inventory final carrier**: when a source-inventory observation is complete for the typed source scope, finalizer/contract should treat that typed row set as the principal source of truth and render/score model-authored aggregate facts as derived or supplemental views. This avoids losing rich lens rows during handoff and prevents partial parser/test fixtures from replacing complete inventory evidence. Status: queued after D1-F7f recheck.
- **D1-F7j bounded per-family inventory cursor**: if root source-inventory still truncates rare-language rows, introduce a typed cursor keyed by existing `SourcePathRole`/`SourceScope`/language family and source-class universe counts. Do not raise global limits or add per-case paths; route follow-up inventory pages by typed family debt. Status: watch item, activated only if D1-F7f eval proves budget truncation.
- **D1-F7k missing source-scope root-first authority**: active source-inventory lanes with no explicit source-scope profile must start from root `repo_map(view="source_inventory")`; analyzer RequiredFiles remain follow-up hints. Explicit bounded source scopes can still narrow. Status: complete in code, pending eval recheck.
- **D1-F7l partial-profile source-quote repair**: when `emit_analysis` rejects an incomplete source-inventory profile but deterministic typed-enumeration synthesis is valid, carry forward only source quotes that passed current-request validation into the synthesized profile. This lets default source-inventory query use structured user-visible construct surfaces without parsing model prose. Status: complete in code, pending eval recheck.
- **D1-F7m load-bearing source-class projection**: promote the existing source-class universe into source-inventory candidate selection. Root/source-unknown inventory must sample visible principal rows across typed language/source-class families before global ranking/truncation, including repo-owned thirdparty/corpus/test/fixture families when the typed request scope is repo-wide. This must reuse `SourcePathRole` / `SourceScope` and existing parser metadata; do not create another string taxonomy.
- **D1-F7n executable pagination debt**: convert `candidate_budget_truncated` / `next_cursor` plus uncovered typed family counts into scheduler-visible inventory debt. When budget remains, the next explore action should page or narrow by the missing typed family; when budget is exhausted, final output must disclose the bounded incomplete universe instead of presenting a complete answer.
- **D1-F7o complete-inventory finish gate**: add a typed source-inventory completion authority consumed by extractor/finalizer/contract. A repo-wide inventory answer may finish only when every requested principal family is represented, explicitly out of scope, or blocked by a typed budget/unavailable reason. Model-authored aggregate prose cannot override this authority.
- **D1-F7p read-status transition coalescer**: introduce a user-facing status transition filter for read mode. Repeated internal auto-complete, stale retry carry-over, and advisory requeue transitions with the same typed reason code should update the audit trace but not print another routine-path status line.
- **D1-F7q analyzer support-noise demotion**: after typed source-inventory classification, split analyzer prescan outputs into principal owner candidates and support/noise candidates. Only principal owners can block evidence collection; support candidates stay available to the model as hints and trace evidence.
- **D1-F7r broad-inventory pre-complete gate**: extend `emit_investigation_complete` preflight so an active repo-wide source-inventory lane with `candidate_budget_truncated`, incomplete sets, or uncovered source-class/language families cannot close as `resolved` unless the closure explicitly carries a bounded incomplete scope. This consumes typed observation/page/set/source-class state only.
- **D1-F7s root-lens handoff priority**: preserve root source-inventory observations as P0 inventory carriers separate from support-file observations. Extractor/finalizer should consume the root carrier first for source-inventory lanes, then support implementation files as P2/P3 context.
- **D1-F7t typed source-inventory follow-up routing**: when a source-inventory observation is incomplete, synthesize the next bounded follow-up from typed family debt (`repo_map` cursor/narrow scope/language/source-class) instead of letting generic grep/list/read become the default continuation. Do not route from raw user keywords or model rationale.
- **D1-F7u row-set artifact family view**: make extractor/finalizer consume row-set artifacts with the same family-prioritized ordering used by candidate selection, so rare language/source-class rows survive prompt budgets and are not replaced by generic examples.
- **D1-F7v scoped completion debt**: refine D1-F7r so completion debt targets typed requested construct/language/source-class rows, not generic all-language function/type inventories. The gate should request the smallest typed follow-up that can change the answer surface, or accept an explicit bounded source scope when all relevant files are read.
- **D1-F7w principal item citation hard gate**: promote principal source-inventory item/citation mismatch from soft advisory to hard pre-emit repair. The check must compare typed item labels/support refs/candidate citations and cited file:line anchors, not natural-language summaries.
- **D1-F7x requested-bucket visible completeness gate**: add a typed final-answer/report check that every accepted principal source-inventory bucket/member set with answer-grade support is represented by a visible structured table/list/section row. Summary prose and automated oracle dimension matches cannot substitute for missing buckets.
- **D1-F7y requested-universe completion debt projector**: derive source-inventory completion debt from typed requested universe (`SourceInventoryProfile`, source-scope/language/source-class counts, observed row-set members, explicit bounded scope) before issuing repair handoff. This projector must never parse user keywords or model rationale; it should output a minimal next action such as cursor page, narrowed scope, or accepted bounded-incomplete closure.
- **D1-F7z deterministic supplement dedupe**: before materializing source-inventory/package/member supplement sections, compute visible structured row coverage from the accepted `AnswerDocumentV2`; skip supplemental bucket sections whose members/dimensions are already present in a principal table/list. This must use typed row/citation cells, not final prose matching.
- **D1-F7aa repair-scope priority split**: split source-inventory root observations, answer-universe scopes, and support implementation scopes into typed P0/P1/P2 lanes. Completion repair may propose support files as context, but executable follow-up scopes for a repo-wide inventory lane must come from source-class universe debt, not analyzer support files.
- **D1-F7ab bounded-scope completion authority**: make `complete=true` observations discharge only their exact typed scope. A repo-wide source-inventory lane may not finish from a narrower complete scope unless a typed source-scope contract proves the narrower scope is the requested universe.
- **D1-F7ac extractor tool-obligation compiler**: compile extractor tool obligations once from typed request model, accepted aggregate facts, answer-symbol requirements, and hypothesis duties. The prompt should receive a single supported tool JSON surface and a non-conflicting obligation list; repair should normalize invalid citations once and fail fast when the tool is unsupported for the stage.
- **D1-F7ad final row citation/member alignment gate**: extend the principal item citation hard gate to compare final visible row labels/text/cells against support refs and cited file:line anchors for every carrier shape, including table text, section items, and deterministic sidecars.
- **D1-F7ae query-filtered requested-universe authority**: when a root source-inventory lens with a validated typed query completes the requested construct/language rows, produce a typed requested-universe closure artifact. Completion gates should consume that artifact to discharge generic all-repo role debt, while still requiring explicit unavailable/budget disclosure for uncovered typed families.
- **D1-F7af eval performance ratchet for read convergence**: add advisory/fail thresholds for explorer iterations, midloop injections, read/list/repo-map counts, wall time, and max context tokens. Thresholds must be typed metrics from eval telemetry, not prose scanning, and should surface even when correctness passes.
- **D1-F7ag supplemental surface-term carrier cleanup**: dedupe deterministic surface-term supplements against primary visible rows and preserve exact cited path casing/line per support ref. Ambiguous multi-line support for one member should become a repair hint, not a rendered "one of ..." row. Status: code complete for source-inventory primary-row coverage and stable display-location rendering; pending representative eval/manual audit before closing D1-G48.
- **D1-F8a biting ratchet re-pin after fast-push audit**: keep `evidence_closure.go`, `scheduler.go`, and `orchestrator.go` ratchets exactly at current actual line counts. Status: complete; `go test ./internal/orchestrator -run TestIRDeliveryHotFileLineRatchet` passed.
- **D1-F8b read loopkernel add-proof load-bearing cutover**: convert exactly one low-risk read `LoopActionAddProof` consumer from guidance/checkpoint text into a typed scheduler next-action gate. It must consume `ReadProofGuidance.RecommendedAction` / truth authority only, preserve shadow comparison telemetry, and include E2E/eval guardrails. Do not parse prompt prose, user wording, or model rationale. Status: code complete / eval pending.
  - Exploration finding: `read_stage_retry.go` currently renders proof guidance and shadow comparison into continuation checkpoint text, and `explore_parallel_dispatch.go` only treats `ProofCoverageCovered` as handoff sufficiency. No read scheduler decision currently selects a next action from `ReadProofGuidance.RecommendedAction`.
  - Cutover shape: introduce a typed read next-action decision helper that consumes `loopkernel.ReadProofGuidance` and `loopkernel.ToolRouteForAction`, returning `action=add_proof`, `reason_code`, route surface, and tool suggestions as structured data. The existing checkpoint/hint renderer becomes a consumer of that decision, not the authority.
  - Load-bearing point: the retry/requeue path must record and consume this typed decision before building continuation text. When `add_proof` is active, the next explore retry is narrowed to proof collection through the typed route; when guidance is covered/continue/unavailable/hard-block, behavior remains unchanged. This is a soft next-action gate, not a hard finish/block gate.
  - Tests: add focused tests proving weak proof yields an active typed add-proof decision and covered proof yields none; update checkpoint tests to assert the decision-derived route metadata; add a regression that shadow comparison remains telemetry and no string/prose parsing is used.
  - Implementation note: `read_loop_next_action.go` owns the typed decision, state carrier, route summary, and directive renderer. `orchestrator.go` only has two connection points so hot-file growth remains bounded by the ratchet.
- **D1-F8c AnalyzeRefine read E2E golden and eval guard**: add realistic read E2E tests for `progress_replan_required=false/true`, proving false path is silent and true path dispatches the optional refine once, then converges without status spam. Add eval telemetry guard for extra rounds/context before marking C2 load-bearing complete. Status: intake complete / implementation pending.
  - Exploration finding: `ensureAnalyzeRefineNode` already compiler-authors a bounded optional one-shot `NodeProbe` before finalize, with `Inputs=["progress_decision","evidence_closure"]`, `Outputs=["analysis_refinement_handoff","progress_decision"]`, and `EntryConditions=[CritProgressReplanRequired]`. The scheduler only consumes typed criterion env; no runtime prompt/prose DAG append is involved.
  - Existing guard: `TestAnalyzeRefineUsesPreAuthoredOptionalNodeOnly` proves local scheduler false/true readiness, and `evalProgressReplanRequired` consumes only `EvidenceClosure.LatestProgressDecision()`.
  - Remaining risk: no read E2E golden proves that a real read run keeps the false path silent, dispatches true path once, leaves the one-shot node done, and does not create repeated "验证还不够稳" / evidence-collection status spam.
  - Implementation tasks:
    1. Add read E2E false-path golden in `read_e2e_regression_test.go`: compiled read graph includes the AnalyzeRefine optional node, no typed progress decision exists, explorer/extractor/finalizer counts stay at the stable baseline, and the refine node does not appear in ready-window dispatch/status noise.
    2. Add read E2E true-path golden: stub explorer records a typed `ProgressDecision` on the closure, the optional refine node dispatches exactly once as an explorer window, then finalizes; assert node exec status, explorer call count bound, and clean `LastError`.
    3. Add a small telemetry/diagnostic surface for optional refine dispatch and skip decisions if current logs lack a typed reason. The signal must be derived from node id/type/criterion result and closure `ProgressDecision`, not model output or user wording.
    4. Extend eval/convergence reporting with a typed `analyze_refine`/`refine_optional` flag or metric only if the runtime emits a structured line; keep it advisory and never use it as answer correctness hard gate.
    5. Run focused tests first (`AnalyzeRefine|ProgressReplan|ReadMode_AnalyzeRefine|IRDeliveryHotFileLineRatchet`), then package/full tests. Representative eval remains D2 scope unless this batch changes eval output format.
  - Exit criteria:
    - False path is silent and does not show up as a blocked optional node or extra user-visible loop.
    - True path dispatches exactly once from typed `ProgressDecision`, not from prompt text, model rationale, raw user keywords, elapsed time, or noisy ranker counts.
    - Eval telemetry can flag optional refine/runaway cost for human audit without altering runtime decisions.
- **D1-F8d read snapshot production writer/resume path**: wire `ReadRunSnapshotStore` into a production read-run save point and an explicit/automatic resume consumer that seeds scheduler from typed `NodeExecStatus`, read ranges, accepted evidence refs, source-inventory observation, and progress decision. Keep scaffold/replay one-way; no prose replay.
- **D1-F8e IngestRound single-owner reducer plan**: audit every read artifact written both by `applyStageOutput` truth-set merge and Mutable setters. Either route it through a typed reducer carrier or document a non-overlapping ownership rule with tests. Goal is to remove N-writer ambiguity without destabilizing stable read scenarios. Status: intake in progress / architecture batch selected before D1-F8c implementation.
  - Exploration finding: `applyStageOutput` already calls production `ingestEvidenceRound(output.ToolResults)`, and `EvidenceClosure.IngestRound` consumes only `ToolResult` rows to add read coverage, read ranges, file totals, and accepted-evidence refs from `ToolHandoffCarrier`. This is the event-stream owner for tool-derived proof.
  - Residual multi-writer surfaces:
    1. `MutableState.AppendEvidence` appends model/tool-emitted evidence and directly calls `EvidenceClosure.AppendAcceptedEvidenceItems`; this is a stage-output/LLM-evidence owner, not the same source as `ToolResult.Handoff.AcceptedEvidence`.
    2. `MutableState.SetTurnAArtifacts` derives `HandoffCarriers` from TurnA tool/evidence inputs and records source-inventory observation into closure. This is a handoff-snapshot owner and can duplicate accepted-evidence semantics if not represented as a typed reducer input.
    3. `MutableState.SetSourceInventoryAdvisory` / `SetSourceInventoryObservation` update mutable fields and also `RecordSourceInventoryObservation` into closure. This is source-inventory observation ownership, currently coupled to setters.
    4. `EvidenceClosure.SetReadSet` / `SetReadRanges` / `SetFileTotalLines` retain snapshot-replacement semantics for explorer parse output; they must not be replaced with additive `IngestRound` semantics without an explicit stage-snapshot reducer.
    5. `MutableState.MergeFrom` merges forked closure state after parallel/subagent work, so any new reducer must preserve worker-created event folding and not double-ingest parent state.
  - Architecture direction:
    - Rename the current reducer conceptually from "all read ingest" to a typed reducer family with explicit input classes: `ToolResultRound`, `StageEvidenceSnapshot`, `TurnAHandoffSnapshot`, `SourceInventoryObservationSnapshot`, and `ForkClosureDelta`.
    - Keep hard ownership typed by input class, not by stage name, prompt wording, or model rationale. The reducer should be callable from `Mutable` setters so side effects remain centralized, but callers still pass structured carriers.
    - First implementation batch should add a reusable typed reducer entrypoint for `TurnAHandoffSnapshot` and `SourceInventoryObservationSnapshot`, then route the existing setter side effects through it while preserving current behavior.
    - Tests must prove idempotence across repeated `SetTurnAArtifacts`, source-inventory advisory/observation merges, and `applyStageOutput` tool-result ingest; no duplicated accepted evidence, no lost source-class universe, no stale source-inventory observation after clear/reset.
  - Exit criteria:
    - Every closure write in these surfaces has an explicit reducer input-class comment and focused test.
    - No path relies on prompt text, user intent keywords, model prose, elapsed time, or broad heuristic counts to decide reducer routing.
    - Stable read E2E and source-inventory tests continue to pass; reducer changes must not alter read-mode dispatch topology.

验证：
- Focused tests for duplicate carrier collapse, rich-row coverage, analyzer required-file demotion, forced-read pruning, and navigation render noise boundaries.
- `go test ./...` and `make`.
- Eval batch: at minimum rerun ArkTS source-inventory, Cangjie repo-wide, Cangjie fixture, plus three non-source-inventory read-mode stability cases to prove the cutover does not regress routine read answers.

退出标准：
- Functionally correct source-inventory answers have exactly one visible carrier per principal row unless the user explicitly requested multiple views.
- `contract_warning` is absent when every requested principal member row is present with typed citations/dimensions.
- Repo-wide source-inventory evals use earlier `repo_map(view="source_inventory")`, fewer forced support reads, lower context usage, and no repeated "验证还不够稳" loop.
- Any remaining failure becomes a named gap in this ledger before the next code batch starts; newly discovered implementation tasks must be added to this task list before code changes begin.

## 10. v2 修订记录（团队复核逐条核实结果）

| v1 表述 | 复核结论 | v2 修正 |
|---|---|---|
| "read 侧 cutover ≈ 0%" | **过度悲观**（与自身 dashboard 4/9 load-bearing 矛盾） | 改为"Stage 1 部分承重、核心 read-loop cutover 未完成" |
| Stage 0 以 "grep=0" 为据 | **表达弱**（应以生产可达性为准） | 改引 `TestMode_WriteControllerDoesNotInstallLegacyWriteTaskGraph` + orchestrator.go:2364 |
| ToolInvocation "production unwired/仅 test" | **过时**（已生产 wiring） | 改为已承重的 append-only 审计/replay projection（root.go:3859、agent.go:4113） |
| source-class universe 仍 advisory | **过时**（已进 absence gate） | 改为 load-bearing（contract_check_block.go:1885 的 RepoTruth 变体） |
| refine "0 actuator 节点" | **部分不准**（有 compiler-emitted optional AnalyzeRefine 节点） | 区分：optional-node 机制已进入生产 topology；但 read E2E golden/eval guard 未完成，不能标商用闭环 |
| RNE-C23/C32 "仍 advisory / 仍 FAIL" | **需重跑 eval**（代码上已非 advisory） | 改为"必须重跑 eval 确认，不沿用旧失败口径" |
| nodeExecStatus shadow | **过时**（B1 已 load-bearing） | 保留为已完成项，后续 read resume 消费 closure status |
| IngestRound shadow | **需细分**（生产 ingest 已承重，但单一 reducer 未覆盖所有 read handoff 字段） | D1-G52 跟踪 N-writer 收敛 |
| read loopkernel telemetry/add-proof | **已部分收敛**（D1-F8b adds typed one-shot next-action carrier; eval still pending） | D1-G49 remains open until representative eval/manual UX audit |
| extract-dispatch L1 pin 为 Phase A 首选 | **成立且与团队"golden-trace 守护"精神一致** | 保留 |

## 11. 快推审计复核（2026-06-21）

| 审计项 | 当前复核结论 | 跟踪/处置 |
|---|---|---|
| `nodeExecStatus` 已 load-bearing | 成立。`EvidenceClosure.NodeExecStatus` 是 scheduler/orchestrator closure-first decision-read authority，local map 仅 nil-closure bootstrap fallback。 | 继续保持 B1 状态；后续 read resume 消费该 typed state。 |
| extract-dispatch L1 pin 已补 | 成立。`read_e2e_regression_test.go` 覆盖 `extract_input_ready=false` 跳过 extractor 与 `true` 派发 extractor。 | 已闭环，作为后续 read-loop cutover 前置守护。 |
| `IngestRound` 半承重/仍有多写面 | 成立但需精确定义。`applyStageOutput` 已生产调用 `EvidenceClosure.IngestRound`，不是 shadow-only；但 truth-set merge 与 Mutable direct setters 仍并存，单一 reducer 尚未覆盖所有 closure/handoff 字段。 | D1-G52 / D1-F8e。 |
| read loop add-proof 原本 advisory-only | 同事审计对快推基线成立：之前只进 checkpoint/advisory。当前工作树已用 D1-F8b 补 typed one-shot next-action carrier 和 retry/window 消费，因此应改记为 code-complete / eval pending，不能再算纯 shadow，也不能算商业闭环。 | D1-G49 / D1-F8b code complete, eval pending；D2 代表性 eval 必须覆盖状态噪音、额外轮次和真实 proof 收敛。 |
| AnalyzeRefine optional node 无守护 | 成立。compiler 已有 optional one-shot node，scheduler unit tests 证明 typed signal dispatch；但缺 read E2E golden/eval runaway guard。 | D1-G50 / D1-F8c；不能标商业闭环。 |
| source-inventory per-class load-bearing | 基本成立，但 correctness 仍以 eval/人工审计为准。多处 source-class / absence / lens-first gate 已承重；Cangjie/ArkTS 等代表 case 还需继续跑批。 | 保持 D1 correctness eval 队列；不从代码空断 PASS。 |
| read run snapshots 是新 scaffold | 成立。typed snapshot/store/tests 存在，但无生产 writer/reader/resume/replay 消费。 | D1-G51 / D1-F8d。 |
| ratchet 非 biting | 成立于快推后状态：`scheduler.go` 和 `orchestrator.go` 阈值高于实际值。 | D1-G53 / D1-F8a 已将阈值收紧到当前实际值，focused test passed。 |
| dashboard load-bearing 数量被误读风险 | 成立。原文把 add-proof、snapshot、AnalyzeRefine 写得过绿。 | 本节与 §2/§6/§7 已降级，后续 ledger 必须同时写 scaffold/load-bearing。 |

## 12. 同事 32-commit 审计复核补充（2026-06-21）

结论：审计总体合理，尤其是「shadow 不等于 load-bearing」「每个 read-loop cutover 必须先有 golden」「ratchet 必须 biting」三条纪律正确。需要修正的是：本批已经把 `LoopActionAddProof` 从纯 advisory 推进到 typed next-action code-complete；ratchet slack 也已被 D1-F8a/D1-F8b 收紧。因此正确 gap 按以下状态纳入跟踪：

| Gap | 是否成立 | 跟踪任务 |
|---|---|---|
| `IngestRound` 与 explorer/Mutable direct-set 仍有 N-writer 面 | 成立。生产 ingest 已承重，但不是所有 closure/handoff 字段都统一进入 `IngestRound` reducer。 | D1-G52 / D1-F8e：先审计所有写入面，再路由到 typed reducer 或写明非重叠 ownership 测试。 |
| AnalyzeRefine optional node 是新 read-loop 行为但缺 E2E golden | 成立。unit/focused tests 只能证明 compiler/scheduler 局部行为，不足以证明真实 read loop 不增加噪音或 runaway。 | D1-G50 / D1-F8c：补 realistic read E2E false/true 双分支与 eval telemetry guard。 |
| read run snapshot 是 substrate，不是 resumability | 成立。typed store 与 tests 存在，但生产 read scheduler/REPL 没有 writer/reader/resume/replay 消费。 | D1-G51 / D1-F8d：接生产 snapshot save point、显式/自动 resume、typed status seed。 |
| `LoopActionAddProof` 不应被 dashboard 误记为完成 | 成立但当前状态已变化。旧基线是 advisory-only；当前 D1-F8b 已 code-complete，仍需代表性 eval 和用户侧状态审计。 | D1-G49 / D1-F8b：D2 eval 必须检查真实 proof 收敛、轮次、状态卡噪音、handoff 是否保留。 |
| source-inventory per-class authority 已承重但 correctness 需 eval 证明 | 成立。代码上 source-class/absence/lens-first 已进 gate，但 Cangjie/ArkTS 等正确性不能靠静态推断。 | D1 correctness eval 队列：每批 6 case、2 并行，人工读输出和日志后再关闭 case。 |
| ratchet slack 会隐藏小回归 | 成立且已修。`scheduler.go`、`orchestrator.go` ratchet 已 pin 到当前值，`orchestrator.go` 因 D1-F8b 接线刷新到 9397。 | D1-G53 complete；后续超过预算必须先拆文件或刷新 ledger 说明。 |

新增记录纪律：后续任何同事审计、eval 日志或人工审计发现的 gap，先写入本节或 §8/§9 对应任务/ledger，再开始编码；不得用「已修附近问题」代替明确的 scaffold/load-bearing 状态。
