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

> **脚手架和若干周边 authority 已完成，write 侧、source-class、ToolInvocation、read node execution status、IngestRound reducer、read snapshot substrate、read loopkernel 首个 add-proof action、AnalyzeRefine optional actuator 等部分已承重；但自动 read resume、read loopkernel 更广泛 action cutover、execution-tree 轻量分支和事件回放仍未完成。因此当前不是「Stage 1/2/3 全完成」，而是「核心 cutover 已推进，剩余 work 要围绕 resume/replay/eval correctness 收敛」。**

### 1.1 记账纪律：scaffold-complete vs load-bearing-complete 必须分两列

把 shadow 标成 `Completed M1c shadow` 会在 dashboard 上误读成"已承重"。**每个 batch 应标两列**：

- **scaffold-complete** —— 影子结构已放置（typed 字段、reducer、projection 都在），但无 decision 真消费它。
- **load-bearing-complete** —— 已有 decision/gate 真消费它，旧路径退役或降为镜像。

---

## 2. 各阶段「load-bearing 退出标准」 + 当前真实位置（已核实）

| 阶段 | 退出标准（必须 load-bearing） | 当前真实位置（对 `2b5c4ba0` file:line 核实） |
|---|---|---|
| **Stage 0** 退役 legacy write DAG | 生产路径走 controller，**生产可达性测试**证明不再装 legacy write TaskGraph | ✅ **load-bearing-complete**：生产 `Mode.IsWrite()→runWriteControllerWorkflow`（orchestrator.go:2364），由 `TestMode_WriteControllerDoesNotInstallLegacyWriteTaskGraph`（mode_dispatch_test.go:256）守护。**以可达性测试为准，非字符串 grep** |
| **Stage 1** 单一执行 IR + 闭环 | `nodeExecStatus` 成唯一 *decision-read* 执行状态（`graphState` 退成 accessor 而非双写镜像）；`IngestRound` 成**生产** reducer；ToolInvocation log 真支撑 replay；**read 有 resumability** | ⚠️ **核心状态/ingest 已承重，auto-resume 未 cutover**：ToolInvocation 已生产 wiring（cmd/root.go:3859 `SetReasoningObserver`、agent.go:4113 `ensureToolInvocationID`）；source-class universe 已进 absence gate（见 Stage-correctness）；B1 已让 `EvidenceClosure.NodeExecStatus` 成为 scheduler/orchestrator decision-read authority；B3 已让 `applyStageOutput` 对生产 closure 执行 `IngestRound`；B2 已有 typed read run snapshot projection/store。仍 open：自动 read resume 尚未接生产调度。 |
| **Stage 2** 自适应引擎 | extract 真门 ✅；**optional/refine 节点机制承重**；progress 驱动的 AnalyzeRefine 有真生产节点；execution-tree 轻量分支可用 | ⚠️ **typed optional actuator 已承重，execution-tree 分支未 cutover**：extract = **真门**（`CritExtractInputReady`，stage_nodes.go）且 A1 已有 golden pin；optional-node 机制 = **已承重**（source-inventory re-probe 与 C2 AnalyzeRefine 均由 compiler pre-author，scheduler 只消费 typed criterion env）；C2 已发 `progress_replan_required` optional one-shot `NodeProbe`。仍 open：execution-tree 轻量分支/branch replay 尚未完成。 |
| **Stage 3** 语义 kernel | write budget 真门 ✅；**read loop 真消费 ≥1 个 loopkernel `RecommendedAction` 作决策**；event store 有真回放路径 | ⚠️ **write 与首个 read action 已承重，event replay 未 cutover**：write = **真承重**（5 个 LoopBudget/LoopRun 构造在 write_controller_scheduler.go，`Advance` 经 `controllerLoopAdvanceAllowsAction` 硬门 3 个 write action）；C1 已让 read retry 消费 typed `LoopActionAddProof` 生成窄 proof follow-up；其它 read actions 与 `ReadLoopShadowComparison` 仍保持 telemetry/advisory；event store 仍无生产 replay 路径。 |

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

按当前 HEAD，A/B/C 中的 extract golden、ratchet、NodeExecStatus、IngestRound、read snapshot substrate、read loopkernel add-proof action、AnalyzeRefine optional node 已完成并入账。下一步最高杠杆顺序调整为：**(D1) 重跑 source-inventory correctness eval，确认 RNE-C23/C32/RNE-C61 真实状态 → (D2) 6-case 商用代表批，按 2 并行审计噪音、工具使用、性能、handoff → (E) 基于 D1/D2 新证据再切 read auto-resume、event replay 或 source-inventory correctness 修复。** 历史 eval 结论必须重跑确认，不沿用旧口径。

---

## 7. 当前 HEAD 复核摘要（`398a32303b`）

本次复核对照当前 `main` 代码，不沿用旧审计口径。结论：v2 方向仍合理，但需要把"完成"拆成 scaffold 与 load-bearing 两个维度，否则后续 dashboard 会继续把 shadow 误读为承重。

| Kernel / surface | scaffold-complete | load-bearing-complete | 代码复核结论 |
|---|---:|---:|---|
| Legacy write DAG retired | yes | yes | 生产写模式从 `Mode.IsWrite()` 进入 `runWriteControllerWorkflow`，read `runTaskGraph` 防御 retired write node。Stage 0 以生产可达性测试为准。 |
| ToolInvocation / ReasoningGraph | yes | yes | `cmd/root.go` 已安装 `ReasoningObserver`，`agent.go` 确保 tool invocation ID；这是 append-only audit/replay projection，非 shadow-only。 |
| Source-class universe / absence gate | yes | yes | 新增 repo-truth wrapper：`SourceInventoryExactAbsenceNeedsInventoryProofRepoTruth`。contract check、pre-emit check、investigation-complete 都能在 observation 为空时从 repo truth seed source-class universe。历史 RNE-C23/C32 只能通过 eval 复验确认，不能继续按旧失败口径判断。 |
| `NodeExecStatus` | yes | yes | B1 已将 `EvidenceClosure.NodeExecStatus` 提升为 scheduler/orchestrator closure-first decision-read authority；`graphState.status` 只保留 nil-closure bootstrap fallback。 |
| `IngestRound` | yes | yes | B3 已将 `applyStageOutput` 切到生产 `ingestEvidenceRound`，由 typed `EvidenceClosure.IngestRound` 承载 read coverage / accepted evidence ingestion。 |
| Extract stage node / `extract_input_ready` | yes | yes | Extract node 和 typed readiness 已承重；A1 已补 false skip-complete 与 true dispatch 的行为级 golden pin。 |
| Optional source-inventory re-probe | yes | yes | compiler 预置 optional source-inventory re-probe node，scheduler 真处理 `TaskNode.Optional` 和 `source_class_universe_incomplete`。 |
| Progress / AnalyzeRefine | yes | yes | C2 已由 compiler 发 bounded optional one-shot `NodeProbe`，EntryCondition 挂 typed `progress_replan_required`，scheduler 不 runtime append DAG。 |
| Loopkernel write budget / advance | yes | yes for write | write controller 已通过 `LoopRun.Advance`/`LoopBudget` 消费 typed budget surfaces；read loopkernel 仍是 shadow/advisory。 |
| Read loopkernel recommended action | yes | partial | C1 已让 typed `LoopActionAddProof` 成为第一个 read-side load-bearing action；其它 action 仍为 advisory/shadow，对应 cutover 需 eval 证据。 |
| LOC ratchet | yes | yes | A2 已 pin 当前值：`evidence_closure.go=2636`、`scheduler.go=799`、`orchestrator.go=9402`。 |

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
- 将 `scheduler.go` ratchet 从 945 收到当前 799。
- 增加 `orchestrator.go` ratchet，当前 9402；若后续需要超过，必须先拆文件或更新 ledger 说明。
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

Tasks:
- Explore `SourceInventoryLensExecutionGapForContext`, `PublishSourceInventoryObservationFromTypedRequest`, source-inventory optional nodes, read retry hints, and final answer caveat materialization before editing.
- Promote the existing typed source-inventory lens-execution gap earlier in the explore loop as a soft-but-load-bearing next-action hint: when active source-inventory profile has no executable lens evidence, guide the next tool call toward bounded `repo_map(view="source_inventory")` / `list_files` before broad shell/file reads.
- Add a typed read-mode command-policy guard for repo-local file discovery commands that parse as broad `find`/shell path enumeration and have an equivalent structured tool. The guard must consume parsed command shape and active tool context, not user prose.
- Split enum final-surface caveats into blocking/repaired/audit-only/displayable states. For complete member sets whose rows have direct read-file citations, suppress generic weak-evidence supplements and keep only real scope caveats.
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
| 2026-06-21 | A2 Biting ratchet pin | complete | Ratchet pinned to current counts: `evidence_closure.go=2636`, `scheduler.go=799`, `orchestrator.go=9402`; failure message now requires splitting concern-specific code or updating this ledger before expanding budget. `go test ./internal/orchestrator -run TestIRDeliveryHotFileLineRatchet` passed. |
| 2026-06-21 | B1 NodeExecStatus load-bearing cutover | complete | `EvidenceClosure.NodeExecStatus` is now the closure-first decision-read authority for scheduler/orchestrator status checks; `graphState.status` is retired after closure attach and remains only nil-closure bootstrap fallback. `rg 'state\\.status\\[|s\\.status\\[|\\.status\\[' internal/orchestrator` only finds accessor internals. Focused `go test ./internal/types ./internal/orchestrator -run 'TestGraphState|TestHandleStructurallyEmptyInvestigation|TestE2E_ReadMode_.*Extract|TestStageMappingFirstClassExtractSkip'` and `go test ./internal/orchestrator -run 'TestGraphState|TestRunTaskGraph|TestE2E_ReadMode|TestMode_DefaultIsRead|TestRunMode_ReadByteIdentical'` passed. |
| 2026-06-21 | B3 IngestRound production reducer cutover | complete | `applyStageOutput` now calls production `ingestEvidenceRound`, mutating the real closure via typed `EvidenceClosure.IngestRound` while preserving `ToolResults` history append semantics. Tests cover production read coverage, accepted-evidence carrier idempotence, and existing truth-set merge behavior. `go test ./internal/types ./internal/tool/ground ./internal/orchestrator -run 'IngestRound|ApplyStageOutput|ReadCoverage|AcceptedEvidence'` passed. |
| 2026-06-21 | B2 Read run snapshot substrate | complete | Added typed `ReadRunSnapshot` projection/persistence plus `ReadRunSnapshotStore` under `<planDir>/read_runs/`. Snapshot carries TaskGraph identity, node exec statuses, read coverage, accepted-evidence refs, source-inventory authority, and progress decision from typed carriers only. Automatic resume remains explicitly out of batch. `go test ./internal/types ./internal/repl -run 'ReadRunSnapshot|EvidenceClosure.*Snapshot'` and `go test ./internal/types ./internal/repl` passed. |
| 2026-06-21 | C1 Read loopkernel add-proof action cutover | complete | `LoopActionAddProof` is now the first read-side load-bearing loopkernel action: weak proof guidance selects a typed narrow proof follow-up continuation summary, while covered/continue proof emits no add-proof action and hard block/finish semantics remain unchanged. `go test ./internal/loopkernel ./internal/orchestrator -run 'ReadProofGuidance|ReadLoopShadow|ExploreTransientRetryCheckpoint|ReadLoopAddProof'` and `go test ./internal/loopkernel ./internal/orchestrator` passed. |
| 2026-06-21 | C2 AnalyzeRefine optional node cutover | complete | Compiler now emits a bounded optional one-shot `NodeProbe` gated by typed `progress_replan_required`, with immutable `analysis_refinement_handoff` output. Scheduler behavior stays generic: it only consumes analyzer-authored optional topology and typed criterion env; no runtime DAG append, no prompt/prose hard routing, no new TaskNodeType. `go test ./internal/analysis/compiler ./internal/analysis/criterion ./internal/orchestrator -run 'AnalyzeRefine|ProgressReplan|Optional|StageExpansion'` and full affected package tests passed. |
| 2026-06-21 | D1 Source-inventory correctness eval intake | complete | Explored current source-class universe, repo-truth absence wrapper, bounded source-inventory kernel, convergence tripwire, ArkTS/Cangjie tool tests, and harmony eval cases. Focused `go test ./internal/types ./internal/tool ./internal/tool/repomap ./internal/orchestrator -run 'SourceInventory|SourceClass|RepoMapSourceInventory|AbsenceRepoTruth|ClassUniverse|Convergence'` passed. |
| 2026-06-21 | D1 Source-inventory correctness eval recheck | complete | `CODRAX_BIN=/Users/han/opt/codrax/codrax CASES='eval/cases/harmony/arkts_repomap.case eval/cases/harmony/cangjie_repomap.case eval/cases/harmony/cangjie_repomap_fixture.case' PARALLEL=2 RUNS=1 TIMEOUT=1800 SUMMARY=eval/results/ir_engine_d1_source_inventory_20260621_summary.md bash eval/convergence_audit.sh` passed 3/3. Fresh evidence closes the stale "repo_map=0/advisory" failure label, but not the routine-path UX/perf gap: `arkts_repomap` took 305s, 14 explorer iterations, 6 midloop injections, 7 reads, 3 list_files, and displayed a generic weak-evidence caveat; `cangjie_repomap` took 16 explorer iterations, 8 midloop injections, and used broad `exec_command find`; `cangjie_repomap_fixture` passed but still showed generic weak-evidence caveat after direct citations. |
| 2026-06-21 | D1-F1 source-inventory routine-path noise reduction | planned | Follow-up split from D1: earlier typed source-inventory next-action guidance, broad file-discovery shell guard, and final enum caveat severity split. Must be implemented before treating D2 as a clean commercial baseline. |
| 2026-06-21 | D1-F1a final enum surface noise | complete | Suppresses repaired/audit-only enum-depth caveats only when compiled principal enumeration rows all have direct citations, while keeping true hallucination/omission/count-drift caveats. Source-inventory requests for name/location/count no longer receive deterministic surface-term supplements such as tag labels unless typed requested fields include `summary` or `values`. Focused `go test ./internal/orchestrator ./internal/tool -run 'Caveat|PrincipalSupportSurfaceTerm|AnswerDocument|Enumeration'` passed. |

---

## 10. v2 修订记录（团队复核逐条核实结果）

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
