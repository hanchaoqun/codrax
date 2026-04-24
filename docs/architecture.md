# 架构设计文档

> **文档对应版本**：`0.1.20260424`（CalVer，`make` 产出的二进制 `codrax --version` 打印准确值）

codrax 是一个**代码分析 + 变更提议**工具：

- **读模式**（默认）：接收自然语言问题，经确定性主流水线 `analyze → explore → extract → finalize`（4 阶段 × 4 Agent）产出带 citation 的结构化答案；附加日志时条件触发 `log_triage` 前置阶段。**不触碰源文件**。
- **写模式**（opt-in，需 `codrax.yaml :: write_enabled: true`）：沿用读模式的 analyze 做请求分类，分流到 `plan → apply → verify` 阶段链（3 个专用 agent：`planner` / `coder` / `verifier`）；所有写动作发生在沙箱 git worktree 里，主仓库 HEAD 字节永不自动变更。

拓扑硬编码在 `internal/orchestrator/topology.go`，不存在运行时覆盖。

## 目录

- [1. 概述](#1-概述)
- [2. 四阶段流水线](#2-四阶段流水线)
- [3. 组件详情](#3-组件详情)
- [4. 阶段规范](#4-阶段规范)
- [5. 数据结构](#5-数据结构)
- [6. 请求生命周期](#6-请求生命周期)
- [7. 分析器后处理管线](#7-分析器后处理管线)
- [8. 关键设计模式](#8-关键设计模式)
- [9. 运行时子系统](#9-运行时子系统)
- [10. 配置](#10-配置)
- [11. 可扩展性](#11-可扩展性)

---

## 1. 概述

### 系统目标

codrax 接受用户的自然语言问题，通过一条**确定性的主流水线（4 阶段 × 4 Agent）**分析目标仓库，产出结构化的答案文档。附加运行时日志时会额外跑一个条件触发的 `log_triage` 前置阶段做结构化抽取。系统不修改代码，不调用有副作用的外部服务，只做：读文件、跑 grep、构建 repo map、执行 shell 查询。

### 核心设计原则

- **LLM 只做两件事**：(1) analyzer 里一次 `emit_analysis` 调用完成请求分类；(2) 各 stage 的 ReAct 循环里调用工具并产出 tool call。所有下游推导（TaskGraph、EvidencePlan、假设集、质量门、答案契约、citation 验证）都在 `internal/analysis/` 的 21 个确定性子包里完成。
- **Fail-loud**：LLM 没调 `emit_analysis` → 阶段直接报错 + 重试，不静默合成零值 IR。
- **Structured data, prose only at LLM boundary**：所有跨 stage / 跨 Agent 的数据通道都是 Go struct；string 仅允许出现在 LLM prompt（struct → markdown）和 LLM 回答（tool call → struct）两个边界。
- **Read-only**：所有 tool 嵌入 `tool.ReadOnly`；工具拿到的 `BusContext` 是窄视图，只有 `Mutable` 字段可写。

### 分层

| 组件 | 包路径 | 职责 | 调用谁 | 被谁调用 |
|------|--------|------|--------|----------|
| **编排器** | `internal/orchestrator` | 走 criterion-aware DAG、分派阶段、fail-loud 重试、CGEC enforcer | Agent、Analysis（criterion/stopcond） | `cmd/root.go` |
| **Agent** | `internal/agent` | 4 个专业 Agent（analyzer / explorer / extractor / finalizer），各跑一轮 ReAct 循环 | LLM、Tool、Analysis（仅 analyzer 调全套）、Skill（读配置） | 编排器 |
| **Skill** | `internal/skill` | 声明式配置：4 个 skill 定义 workflow / 工具 allowlist / 输出格式 / 禁令 | 无（纯数据） | Agent（读取后注入 prompt） |
| **Tool** | `internal/tool` | 只读工具（grep / read_file / repo_map 等）+ 6 个 emit_* 结构化发射器 + `ground/` citation 验证 | 文件系统、MutableState | Agent（`executeTool`） |
| **LLM** | `internal/llm` | 可插拔 adapter，per-agent 模型路由 | 外部 API | Agent（ReAct 循环） |
| **Analysis** | `internal/analysis/*` | 21 个确定性子包，组装 IR + 运行时求值 criterion | 无（纯函数） | analyzer agent（`buildAnalysisIR`）、编排器（criterion/stopcond）、emit_* 工具（ground/contract） |
| **Render** | `internal/render` | 事件流（`event.go`）+ CLI 渲染（`renderer.go`）+ AnswerDocument → markdown（`answerdoc.go`） | 无 | `cmd/root.go` |
| **Context builder** | `internal/context/builder.go` | struct → markdown prompt 的唯一装配点，canonical section 顺序锁死 | `skill.Config`、`AgentContext` | `BaseAgent.buildInitialMessages` |

**关键规则：**
- 所有 tool 调用和 LLM 调用都必须通过 Agent。编排器不直接调工具或 LLM。
- Analysis 层是纯确定性函数 —— 不调 LLM、不调工具、不读文件系统。
- Skill 是声明式配置，被 Agent 读取后渲染进 prompt，自己不调任何东西。

### 系统概览

```mermaid
graph TB
    User([用户请求])

    subgraph Orchestrator["编排器 internal/orchestrator"]
        Orch["读模式：criterion-aware DAG scheduler
        + fail-loud retry + CGEC enforcers I1-I4
        写模式：plan / apply / verify phase 直分派
        （绕过 scheduler）"]
    end

    subgraph Agents["Agent internal/agent"]
        LT["log_triager · 条件前置"]
        A1["analyzer"]
        A2["explorer · Turn A"]
        A3["extractor · Turn B"]
        A4["finalizer"]
        P["planner · write"]
        C["coder · write"]
        V["verifier · write"]
    end

    subgraph Skills["Skill internal/skill · 声明式配置"]
        S["log-triage-skill / analysis-skill
        explore-skill / extract-skill
        answer-document-skill
        change-plan-skill / code-write-skill
        test-execute-skill"]
    end

    subgraph Tools["Tool internal/tool"]
        T["只读工具
        grep / read_file / list_files
        repo_map / exec_command
        git_diff / git_log"]
        E["结构化发射器
        emit_log_triage / emit_log_segmentation
        emit_analysis / emit_evidence
        emit_answer_symbol / emit_hypothesis_verdict
        emit_answer_document
        emit_investigation_complete
        propose_sub_agents"]
        W["写模式工具 · WriteCapable
        emit_change_plan（含 git apply --check 预检）
        apply_patch（schema {path, kind}；内容源于 Mutable）
        run_tests（9 种 runner）
        emit_test_results"]
        G["internal/tool/ground
        citation / evidence 落地验证
        T1 / T2 + R1-R5 recovery"]
        FB["internal/tool/feedback
        composePatchRejection
        conflictContextSnippet"]
    end

    subgraph Subsystems["运行时子系统"]
        WT["internal/worktree
        git worktree 沙箱会话
        活跃集 + PID 回收"]
        PS["internal/repl/planstore
        ChangePlan 持久化"]
    end

    subgraph Analysis["Analysis internal/analysis · 21 确定性子包"]
        AN["normalizer / compiler / budget
        sourcemix / risk / hdp / priority
        binder / counterfactual / gate
        stopcond / criterion / contract / dataflow
        aggregator / declarative / findings_validator
        hint / patcher / axis / subject"]
    end

    User --> Orch
    Orch -->|读模式 dispatch| LT & A1 & A2 & A3 & A4
    Orch -->|写模式 dispatch| P & C & V
    LT & A1 & A2 & A3 & A4 & P & C & V -.->|读配置| S
    LT & A1 & A2 & P & C & V -->|调用| T
    LT & A1 & A2 & A3 & A4 -->|调用| E
    P -->|调用| W
    C -->|调用| W
    V -->|调用| W
    W -->|"git apply → patch(1) fallback"| FB
    E -->|同步调用| G
    LT & A1 & A2 & A3 & A4 & P & C & V -->|调用 LLM| LLM[LLM internal/llm]
    A1 -->|buildAnalysisIR| AN
    Orch -->|stopcond / criterion / cgec| AN
    A3 -->|criterion auto-verdict| AN
    Orch -->|provision / discard| WT
    Orch -->|持久化 / 加载| PS
```

---

## 2. 四阶段流水线

拓扑是**硬编码**的（`internal/orchestrator/topology.go`）：主流水线 4 阶段 × 4 agent 永远按序执行；log_triage 是条件前置阶段，`BusContext.AttachedLog` 非空时才触发（Guard 定义在同文件的 `preStages` 列表中），失败不阻塞主流水线。

```go
// 主流水线（无条件）
var pipelineTopology = map[types.PipelineStage]struct {
    Agent    types.AgentName
    Skill    string
    Terminal bool
}{
    types.StageLogTriage: {Agent: types.AgentLogTriager, Skill: "log-triage-skill"},
    types.StageAnalyze:   {Agent: types.AgentAnalyzer,   Skill: "analysis-skill"},
    types.StageExplore:   {Agent: types.AgentExplorer,   Skill: "explore-skill"},
    types.StageExtract:   {Agent: types.AgentExtractor,  Skill: "extract-skill"},
    types.StageFinalize:  {Agent: types.AgentFinalizer,  Skill: "answer-document-skill", Terminal: true},
}

// 条件前置阶段列表（按声明顺序依次尝试）
var preStages = []preStageEntry{
    {Stage: types.StageLogTriage, Guard: /* AttachedLog != "" */},
}
```

| 阶段 | 默认 Agent | 默认 Skill | 触发条件 | Terminal |
|------|-----------|-----------|---------|:-:|
| `log_triage` | `log_triager` | `log-triage-skill` | `AttachedLog` 非空 | |
| `analyze` | `analyzer` | `analysis-skill` | 无条件 | |
| `explore` | `explorer` | `explore-skill` | 无条件 | |
| `extract` | `extractor` | `extract-skill` | 无条件 | |
| `finalize` | `finalizer` | `answer-document-skill` | 无条件 | ✅ |

### 运行时流程

**读模式（`Mode == ModeRead`）**：

```mermaid
stateDiagram-v2
    [*] --> logTriageGate
    logTriageGate --> log_triage : AttachedLog 非空
    logTriageGate --> analyze : AttachedLog 空或 log_triage 失败（降级）
    log_triage --> analyze : bundle 写入 Mutable
    analyze --> taskLoop : AnalysisIR.TaskGraph 生成
    taskLoop --> explore : readyExplorerWindow 非空
    explore --> extract : 当前 window 全 done
    extract --> finalize : finalize 节点 ready
    finalize --> contractCheck
    contractCheck --> taskLoop : 失败且 retry budget 有余（requeue）
    contractCheck --> [*] : 通过 / retry 耗尽（fail-loud）
```

**写模式（`Mode ∈ {ModePlan, ModeApply, ModeVerify}`）**：不经 `runTaskGraph`，`Run()` 根据 Mode 直接分派到 phase 函数；analyze 仍跑一次作为分类器。

```mermaid
stateDiagram-v2
    [*] --> writeGate
    writeGate --> [*] : write_enabled=false → fail-loud
    writeGate --> analyze : write_enabled=true

    analyze --> planPhase : Mode=ModePlan
    analyze --> applyPhase : Mode=ModeApply 且 PlanPath 已设
    analyze --> planPhase2 : Mode=ModeApply 且 PlanPath 未设
    analyze --> verifyPhase : Mode=ModeVerify

    planPhase --> emitChangePlan : planner dispatch
    emitChangePlan --> preflightCheck : 每个 kind=patch
    preflightCheck --> emitChangePlan : 拒收（planner 下轮重试 / cap=6）
    preflightCheck --> planDone : git apply --check --recount 通过
    emitChangePlan --> planDone : 无 kind=patch 或全通过
    planDone --> persistPlan : .codrax/plans/<id>.json
    persistPlan --> [*] : pending_approval

    planPhase2 --> emitChangePlan
    applyPhase --> worktreeProvision : git worktree add
    worktreeProvision --> baselineCapture : pipeline_baseline_capture_enabled=true
    worktreeProvision --> coderDispatch : 默认
    baselineCapture --> coderDispatch : BaselineReport 挂载
    coderDispatch --> applyPatch : 每个 change 一次
    applyPatch --> applyPatch : W1 / W1b / kind match 失败 → 下轮重试
    applyPatch --> coderDispatch : AppliedSet ⊇ TargetPaths？
    coderDispatch --> verifyPhase : 是
    coderDispatch --> verifyRetry : 否 → apply_failed

    verifyPhase --> runTests : run_tests 自动探测 runner
    runTests --> changeReportInstalled : parser 产出 ChangeReport
    changeReportInstalled --> verifySuccess : Passed=true
    changeReportInstalled --> verifyRetryGate : Passed=false

    verifyRetryGate --> verifyRetry : pipeline_max_verify_retries > attempts
    verifyRetryGate --> verifyFailed : 重试耗尽
    verifyRetry --> planPhase : PlanningHint 注入 → reset plan/report/worktree

    verifySuccess --> worktreeDecide
    worktreeDecide --> preserved : pipeline_keep_worktree_on_success=true
    worktreeDecide --> discarded : 默认
    preserved --> [*] : PlanStatus=applied; plan.WorktreePath 暴露
    discarded --> [*] : PlanStatus=applied; worktree 销毁

    verifyFailed --> [*] : PlanStatus=verify_failed; worktree 无条件销毁
```

0. **Phase 0 — `log_triage`**（条件前置）：`BusContext.AttachedLog` 非空时触发。`log_triager` agent 读取日志原文，LLM 通过 `emit_log_triage` 发出四层结构化 bundle（`Meta` / `Errors` 含递归 `Cause` / `Residue`），系统调用 `logtriage.ValidateBundle` 做路径校验 + 仓内存在性过滤 + Layer 4 派生（`ResolvedFiles` / `Entities` / `IntentHint` / `Coverage`），写进 `Mutable.LogTriage()`。coverage 过低或日志体积超阈值（默认 32 KB）自动走两步升级（`emit_log_segmentation` → 逐段 `emit_log_triage` → `MergeBundles`）。阶段失败**不阻塞**主流水线——bundle 停留为 nil，每个下游消费者（analyzer 的 entity merge / intent override / RequiredFiles seed）都 nil-check 优雅降级。详见 §4.5。
1. **Phase 1 — `analyze`** 跑一次。analyzer 先用 1-2 轮 evidence-lite 预扫（`repo_map` / `grep files_only=true` / `list_files`）验证用户提到的实体和术语是否在仓库里出现，然后一次 `emit_analysis` 调用写出 v4 `RequestModel`。`analyzer.ParseOutput` 随后确定性地跑后处理管线（见 §7）组装完整的 `AnalysisIR` 并写进 `BusContext.AnalysisIR`。analyzer **禁止**读文件内容（`read_file` / `exec_command` 不在它的 tool allowlist）。
2. **Phase 2 — per-task DAG 执行**。`runTaskPhase` 调用 `runTaskGraph` 遍历 `AnalysisIR.TaskGraph.Nodes`：
   - **criterion-aware window schedule**：`readyExplorerWindow` 返回两个列表：`ready`（所有 `EntryConditions` 满足的 pending/requeued 非 finalize 非 counterfactual 节点）和 `blocked`（entry condition 未满足的节点——在 retry hint 中暴露阻塞的 criterion 诊断）。Ready 节点合并成**一次** `explore` dispatch。每轮开始前 `stopcond.ShouldStop` 评估 `EvidencePlan.StopConditions`（OR 语义），命中则跳到 finalize。编排器在入口从 `EvidencePlan.NodeBudgetHints` 安装 `ExploreBudget` 到 `MutableState`；explorer 的 `BaseAgent.executeTool` 在每次工具调用前检查剩余预算，超额返回失败 `ToolResult`。
   - **Shape-guard 保护 pure-read 分支**：`stopcond.ShouldStop` 和 validate 节点的 SuccessCriteria 评估都是 pure functions over `criterion.Env`——相同 env 永远返回相同 verdict。调度器用 `envShape`（`Evidence` / `AnswerSymbols` / `AnswerChains` / `ToolResults` / `ReadSet` / `PendingReads` / `DecidedHypotheses` / `PrescanBytes` 八维 int 指纹，O(1) 计算与比较）对每一次 pure-read 检查做 gate：记录上次评估时的 shape，下次 shape 未变就跳过或升级处理。该机制在 `runTaskGraph` 里解决整类"predicate 输入是 pure-read + 分支 body 不推进输入 → 热循环"结构脆弱性，不依赖每个 predicate 自己加 latch。
   - **StageExtract** 在 explore window 完成后作为 Turn B 分派。dispatch 后 `markSuccessCriteriaFailed` 评估每个 window 节点的 `SuccessCriteria`——通过的节点标 done，未通过的标 requeued。若失败的是 **validate** 节点，额外触发 `requeueValidationTargets`：沿 `EdgeValidationFeedback` 边只 requeue 其上游 evidence 节点（细粒度回溯，非整个 window）。**Shape-stuck 逃生**：`lastSCFailShape[validateID]` 记录每个 validate 节点上一次 SC 失败时的 shape；如果这次失败时 shape 相同（re-investigation 没带来新证据），调度器判定"此路不通"——调用 `injectInconclusiveForStuckHypotheses(validateID)` 对仍 HypUnknown 的假设注入诚实 `HypInconclusive` verdict + stuck-signal rationale，`markDone(validateID)`，不再 requeue。典型触发场景：Java trace 配 Go repo，假设引用的类在仓里根本不存在，explorer 再读 N 轮都读不出来。extractor 的 `ParseOutput` 对每条 hypothesis 调用 `criterion.Eval`：`RequiredEvidence` 全满足但无 LLM verdict → 注入 `HypInconclusive`；`FalsificationCondition` 满足 → 注入 `HypRejected`（覆盖 LLM verdict）。
   - **StageFinalize** 仅在 `firstFinalizeReadyMerged` 返回非 nil（所有非 finalize、非 counterfactual 节点都 `done`）时分派。
   - **investigation_complete 策略**（`agent_investigation_complete_policy`，默认 `soft`）：当 LLM 调用了 `emit_investigation_complete` 但 DAG 节点的 `SuccessCriteria` 数量不满足时，`soft` 把 `InvestigationComplete=true` 注入 `criterion.Env` 让阈值降到 ≥ 1；`override` 跳过所有 SuccessCriteria；`strict` 忽略完成信号按模板阈值硬跑。
   - **多话题 DAG**：`RequestModel.SubTopics` 非空时，`compiler.expandEvidenceNodes` 为每个子话题生成独立 evidence DAG 节点；prescan / explorer iterations / pipeline steps / retry budget 按 `agent_subtopic_*` 自适应扩充。
   - **Contract check**：finalize 返回后跑 `contract.Check`。不通过且 retry budget 未耗尽 → requeue finalize + 所有 done 的 explorer 节点，记一次 cross-window retry，把违规诊断塞进下一轮 `RetryHint`；retry 耗尽 → 在原答案上 prepend 一条 fail-loud 警告后返回。
3. Finalize 把答案写进 `Mutable.Result()`（`recordTaskFinalize`）。`Run()` 返回 `BusContext`，`cmd/root.go` 渲染结果。

**全局预算** `pipeline_max_steps` 是整 Run 的硬上限，跨 Phase 1/2 共用；`EvidencePlan.Budget.MaxReactIters` 是**每个 task** 的额外上限。

---

## 3. 组件详情

### 3.1 编排器（`internal/orchestrator`）

| 文件 | 职责 |
|------|------|
| `orchestrator.go` | `Run()` 入口、Phase 1 / Phase 2 主循环、contract.Check retry budget |
| `scheduler.go` | `runTaskGraph`、`readyExplorerWindow`、`markSuccessCriteriaFailed`、`requeueValidationTargets`、`runAutoVerdicts`、`drainHypothesisVerdicts`、`forceCloseExploreWindow` |
| `topology.go` | 4 stage × 4 agent × 4 skill 硬编码 map |
| `cgec_enforcers.go` | CGEC I1-I4 执行入口：`applyChainPromotion`、`preCompleteContractCheck`、`detectStallAndAct`、`runForcedReads`、`renderWindowHint`、`emitCGECSummary` |
| `contract_check.go` | AnswerContract 的 finalize 后置检查 + retry budget |
| `prior_conv_policy.go` | 4 值 `PriorConvPolicy` 在 per-stage 的可见性解析 |
| `tier1_floor.go` | evidence Tier-1 floor 检查（`evidence_tier1_floor`） |
| `user_messages.go` | 向用户渲染的 fail-loud / stall 文案 |

编排器的核心数据类型 `graphState` 记录每个 DAG 节点的状态（`pending` / `running` / `done` / `failed` / `requeued`）和跨 window retry 计数。

### 3.2 Agent（`internal/agent`）

所有 Agent 嵌 `BaseAgent`，后者提供 ReAct 循环，通过 `Evaluator` 接口接入每个 Agent 的四个钩子：`BuildInitialInstruction` / `ShouldStop` / `ParseOutput` / `DetermineMissingPiece`。可选 `LoopController` 接口接入循环控制。

读模式 5 个 agent：

| Agent | Stage | 工具权限 | 职责 |
|-------|-------|----------|------|
| `log_triager`（条件前置） | `log_triage` | `read_file`（仅用于分页读 attached log blob） + `emit_log_triage` + `emit_log_segmentation`（两步升级才激活） | 仅在 `BusContext.AttachedLog` 非空时触发。读原始日志，emit 结构化 `LogBundle` 的 Layer 1-3（Meta / Errors / Residue）；系统侧 `logtriage.ValidateBundle` 做路径校验并派生 Layer 4。失败不阻塞主流水线（§4.5） |
| `analyzer` | `analyze` | `emit_analysis` + evidence-lite 预扫（`repo_map` / `grep files_only=true` / `list_files`） | 1-2 轮预扫验证实体，然后一次 `emit_analysis` LLM 调用产出 v4 `RequestModel`；`ParseOutput` 跑确定性管线组装 `AnalysisIR` |
| `explorer`（Turn A） | `explore` | `grep` / `read_file` / `repo_map` / `list_files` / `exec_command` / `emit_evidence` / `emit_investigation_complete` / `propose_sub_agents` | 两阶段调查（Phase 0 Breadth → Phase 1 Depth），独占写 `EvidenceItems` / `AnswerChains` / `FlowFindings`，把投影快照写入 `TurnAArtifacts` |
| `extractor`（Turn B） | `extract` | **仅** `emit_answer_symbol` + `emit_hypothesis_verdict` | 一次性 LLM 调用产出答案 slate + completeness claim + hypothesis verdict。**禁止文件 IO、禁止 `emit_evidence`**。工具参数校验失败有一次 retry 窗口（`ShouldStop: iteration >= 2`） |
| `finalizer` | `finalize` | `emit_answer_document` | 按 `AnswerShape` 渲染结构化答案文档，触发 contract check |

写模式 3 个 agent（需 `write_enabled: true`，详见 §4.6）：

| Agent | Stage | 工具权限 | 职责 |
|-------|-------|----------|------|
| `planner` | `plan` | `read_file` / `grep` / `list_files` / `repo_map` / `exec_command` / `emit_change_plan` | 读代码理解当前结构，产出完整 ChangePlan；不写文件；6 次 iteration cap（`emit_change_plan` 的 `git apply --check --recount` 预检可能把一次调用变成多次重试，cap 给足余量）|
| `coder` | `apply` | `read_file` / `apply_patch` / `exec_command` | 按 `plan.Changes[]` 逐单元调用 `apply_patch(path, kind)`；工具从 `Mutable.ChangePlan()` 读取 `NewContent`/`Patch`（LLM 不转抄内容，schema 禁止）；ShouldStop 在 `WriteClosure.AppliedSet ⊇ plan.TargetPaths` 时触发 |
| `verifier` | `verify` | `read_file` / `run_tests` / `emit_test_results`（可选） / `exec_command` | `BuildInitialInstruction` 总是发射"## Verify phase"阶段定向指令（即使 plan 无 AcceptanceTests 也不返回空串）；`run_tests` 同步 install `ChangeReport`；`emit_test_results` 是可选 LLM narrative；权威 pass/fail 来自 parser，不由 LLM 覆盖 |

**读→写 context 隔离**（`internal/context/builder.go`）：`PipelineStage.IsWrite() ∈ {StagePlan, StageApply, StageVerify}` 为 true 时，`BuildAgentContext` 跳过所有读模式字段的传播（`RelevantFacts` / `EvidenceItems` / `AnswerChains` / `AnswerSymbols` / `PriorReports` / `UnverifiedAnalyzerFindings` / `SubjectMatches` / `ExpectedAnswerSubject` 等）；`BuildPromptContext` 对 `StageApply` / `StageVerify` 另外抑制 "User Request" 段（用户的原始自然语言请求通常是 plan-shaped，会干扰 apply/verify 的机械执行角色；plan 意图已在 `Mutable.ChangePlan` 上供 stage-specific 使用）。`StageAnalyze` 即便在写模式下也视为读模式（它仍需分类器输入）。这些边界由 `internal/types/pipeline_stage_test.go::TestPipelineStage_IsWrite` 和 `internal/context/builder_test.go::TestBuildAgentContext_WriteStage_ScrubsReadModeArtifacts` 硬编码测试固化。`formatStageReports` 额外剥 `<think>…</think>` 片段作为 defense-in-depth，读模式也受益（explorer / extractor / finalizer 不再看到 analyzer 的内部推理）。

#### ReAct 循环

```mermaid
graph TD
    Start([Agent 接收 prompt]) --> Think[LLM 推理]
    Think --> Decide{需要工具?}
    Decide -->|是| Act[调用工具]
    Act --> MidLoop{LoopController<br/>Observe PhaseMidLoop}
    MidLoop -->|InjectHint| Think
    MidLoop -->|Stop| Output
    MidLoop -->|Continue| Think
    Decide -->|否| SoftStop{LoopController<br/>Observe PhaseSoftStop}
    SoftStop -->|InjectHint| Think
    SoftStop -->|Stop/Continue| Output[ParseOutput → StageOutput]
    Output --> Done([返回编排器])
```

- **`LoopController`**（`agent.go`）统一的循环控制钩子。`BaseAgent.Execute` 在两个时机调用它：
  - `PhaseMidLoop` — 每一轮 tool 调用后；
  - `PhaseSoftStop` — LLM 返回纯文本且没 tool 调用时。

  评估器的 `Observe(ctx, obs) LoopSignal` 只做**检测**，返回 `Progress` / `StopRequested` / `HintRequested`+`Hint`+`HintKey`。节流（`MinInjectInterval`）、去重（按 `HintKey`）、预算（`MaxContinuations` / `MaxMidLoopInjects`）、idle-streak 强停（`IdleStopThreshold`）统一由 **`LoopPolicy`**（`loop_policy.go`）的 `loopPolicyState.Apply` 执行。

- **Terminal-tool-call stop**：4 个 agent 的 `Observe` 在 `PhaseMidLoop` 检测到各自的终态工具调用**成功**后立即返回 `StopRequested: true`。失败（参数校验错误等）**不触发** stop，允许 LLM 下一轮看到错误并重试。

  | Agent | 终态工具 | Stop 条件 |
  |-------|---------|-----------|
  | log_triager | `emit_log_triage`（单步）或两步 fallback 的第二轮 `emit_log_triage` | 成功 emit 并 `logtriage.ValidateBundle` 通过；两步升级由 agent controller 驱动（§4.5） |
  | analyzer | `emit_analysis` | `LastToolResult.Success == true` |
  | explorer | `emit_investigation_complete` | `LastToolResult.Success == true` |
  | extractor | 任何 emit_* | `AllToolResults` 中存在 `Success == true` |
  | finalizer | `emit_answer_document` | `MutableState.AnswerDocument()` 非空 |

- **ExploreHeuristics**（`types.DefaultExploreHeuristics()`）：explorer 的 mid-loop / soft-stop 检测用的 16 个阈值（mid-loop / soft-stop / Phase 0 / enumeration / parallelize 等）全部通过 `codrax.yaml` 的 `explore_*` 键覆盖，零值用代码默认。

- **Context-pressure 监控**（所有 agent 通用）：`BaseAgent.Execute` 在每轮 `pruneToolHistory` 之后立刻执行监控。若 `b.deps.LLM.MaxContextTokens() > 0` 且 `AgentSettings.ContextPressureSoftRatio/HardRatio` 有至少一项 > 0：
  - `estimateMessagesBytes(messages)` 估算 wire-level prompt 字节（Role + Content + ToolCallID + ToolCalls[*].ID/Name/Params 的总 len）
  - `byteBudget = ctxWindow × types.BytesPerToken` 作分母，算 `ratio`
  - `ratio ≥ HardRatio` → `logging.Warning "HARD"` + append 一条 user-role hint（通过 `contextPressureDirective(name, promptBytes, byteBudget, hardRatio)` 生成，**复用 `internal/analysis/hint.Composer`** 的 "**What failed** / **Why it failed** / **What I already did** / **How to fix now** / **Allowed** / **Do NOT**" 6 段格式）+ 设 `forceStop = true`（本轮 LLM 调用照跑，下一轮就退出）
  - `ratio ≥ SoftRatio` → 仅 `logging.Warning "SOFT"`（不改循环控制）
  - AllowedSet / ForbiddenPatterns **per-agent 定制**：只列该 agent 能调用的终结工具（如 verifier 只看到 `run_tests`、coder 只看到 `apply_patch`、extractor 看到 `emit_answer_symbol`+`emit_hypothesis_verdict`），避免跨 stage 工具名误导；另加 shared-core "禁止更多调查/读/搜" + per-agent 补充（extractor "不能 claim complete"、coder "不要 re-read 文件" 等）
  - 阈值默认 0.7 / 0.9，yaml 键 `agent_context_pressure_soft_ratio` / `_hard_ratio` 覆写；任一设 0 关对应那一侧

#### Skill vs Evaluator 职责边界

| 层级 | 归属 | 承载内容 | 物理位置 |
|------|------|----------|----------|
| **Static（静态契约）** | Skill | 角色身份、Workflow、OutputFormat、Prohibitions、字段枚举、ToolSuggestions | `internal/skill/*.go` 的 `Config` 字面量 |
| **Dynamic（通用动态段）** | Builder | per-dispatch 段：User Request / Retry Directive / Prior Findings / Known Facts / Structured Evidence / Dataflow Findings / Answer Symbols / Hypothesis Verdicts / Relevant Files / Missing Piece | `internal/context/builder.go::BuildPromptContext`，`canonicalSystemSectionOrder` / `canonicalUserSectionOrder` 锁死顺序 |
| **Stage-specific supplement（本轮专属）** | Evaluator | 只有 builder 无法泛化产出的段：extractor 的 Turn A digest、answer-document 的 resolved shape + cardinality baseline + prior slate | `Evaluator.BuildInitialInstruction`，`BaseAgent.buildInitialMessages` 作为**额外**的 user 消息追加在 builder 输出之后 |

Evaluator 的 `BuildInitialInstruction` 绝对不能重述 skill 的任何字段，也不能再发射 builder 已经写过的标题——会让 LLM 看到两份同名段并产生矛盾指令。`TestAnalyzer_BuildInitialInstruction_IsEmpty` 把 analyzer 的"空补充"契约固化（它的动态段 builder 全能产出）。Extractor 和 Finalizer 是"非空补充"的参照。

#### Canonical section 顺序

`internal/context/builder.go`：

```
canonicalSystemSectionOrder:
  Agent Identity, Reasoning Hygiene, Think Aloud, Constraints,
  User Preferences, Pipeline State, Skill Goal, Workflow,
  Output Format, Prohibitions

canonicalUserSectionOrder:
  Retry Directive (READ FIRST), User Request,
  Prior Conversation (reference only),
  Log Triage — Validated Extraction, Attached Runtime Log,
  Prior Stage Findings, Known Facts,
  Extracted Answer Symbols (deterministic, authoritative),
  Answer Symbols (deterministic floor, may extend with cited evidence),
  Structured Evidence, Unverified Leads (not for citation),
  Dataflow Findings, Hypothesis Verdicts, Relevant Files
```

`Log Triage — Validated Extraction` 由 `formatLogTriageStructured(ac.LogTriage)` 渲染，仅对非 `log_triager` agent 生效（producer 自己不消费自己的输出）；`Attached Runtime Log` 渲染 `AttachedLog` 原文。两段永远成对出现——结构化视图优先、原文备查。详见 §4.5。

#### 子 Agent

explorer 可以通过 `propose_sub_agents` 工具向编排器申请派生并行 `sub_explorer` 实例分摊独立调查子问题。`sub_explorer` 不共享 `Mutable`（`BuildSubAgentContext` 故意把 `ac.Mutable` 留成 nil），`todo_write` / `emit_*` 在 sub-agent 上下文会被拒绝。实现：`sub_explorer.go` + `subagent.go` + `subagent_runtime.go`。

### 3.3 Skill（`internal/skill`）

```go
type Config struct {
    Name            string
    Goal            string
    Workflow        []string
    ToolSuggestions []string
    OutputFormat    string
    Prohibitions    []string
}
```

技能是**声明式配置**。Agent 加载 skill，按它的 `Workflow` 决定 prompt、按 `ToolSuggestions` 决定允许的工具、按 `Prohibitions` 决定禁令。

| Skill | 所属 Agent | 核心约束 |
|-------|-----------|----------|
| `log-triage-skill` | log_triager | Tool allowlist：`read_file`（仅 blob pagination） + `emit_log_triage`。Prohibitions：禁止自己做路径解析（系统工作）、禁止 grep / list_files / repo_map（`logtriage.ValidateBundle` 负责仓内验证）、禁止一次 dispatch 多个 emit |
| `log-segmentation-skill` | log_triager（两步升级） | Tool allowlist：`read_file` + `emit_log_segmentation`。当单次抽取 `Coverage < 0.3` 或日志 ≥ 32 KB 时激活；只按字节坐标切 `stack`/`caused_by`/`header`/`context`/`trace`/`noise` 段，agent controller 再逐段重调 `emit_log_triage` |
| `analysis-skill` | analyzer | 由 `internal/skill/analysis_contract.go::BuildAnalysisSkill` 构造，字段枚举来自同文件 SSOT 表（`emit_analysis` schema 从这里读枚举）。Tool allowlist：仅 `emit_analysis` + 3 个 evidence-lite 预扫工具 |
| `explore-skill` | explorer | 两阶段 workflow；Tool allowlist：6 个只读工具 + 3 个 emit_*。Prohibitions：无假设、无 "next steps"、无纯 prose headings |
| `extract-skill` | extractor | Prohibition 显式禁 `emit_evidence`（Turn B 不能侵犯 Turn A 的 evidence 通道）；Tool allowlist：**仅** `emit_answer_symbol` + `emit_hypothesis_verdict` |
| `answer-document-skill` | finalizer | Tool allowlist：仅 `emit_answer_document`；OutputFormat 按 `AnswerShape` 分 shape |

`buildToolSchemas` 物理裁剪 LLM 看到的工具 schema：skill 没声明的工具 LLM 根本看不到。

### 3.4 Tool（`internal/tool`）

工具通过嵌入 `tool.ReadOnly` 或 `tool.WriteCapable` 声明副作用语义：`IsWrite() bool` 返回 `false`（只读）或 `true`（写模式专属）。读模式 agent 的 skill allowlist **只**包含 read-only 工具；写模式 agent（`planner` / `coder` / `verifier`）才能看到 write-capable 工具。`Execute` 收到的 `*BusContext` 是窄视图：只有 `RepoRoot` / `Branch` / `Commit` / `WorkDir` / `Mutable` / （写模式额外的）`MainRepoRoot` / `WorktreePath` / `Mode` / `PlanPath` 被填充。

#### 内置 read-only 工具

| 工具 | 描述 |
|------|------|
| `grep` | 按模式搜索；支持 `files_only=true`（对应 `rg -l`）返回匹配文件列表而非每行 |
| `read_file` | 读整文件；大文件用 `offset+limit` slice 读。`readfile_small_limit_threshold` 对小文件的懒惰 limit 做保护性展开 |
| `list_files` | 列目录 |
| `repo_map` | 生成仓库符号/关系索引的结构化视图。`task_map` 视图给 breadth scan 快速定位角色 |
| `exec_command` | 执行 shell 命令（按 read-only 处理，写限制靠外部沙箱） |
| `git_diff` / `git_log` | git 状态查询 |

工具分类（`toolConfidence` 接口）：`EvidenceTool` 0.8（grep/read_file/list_files/git_*/exec_command）/ `NavigationTool` 0.3（repo_map）/ `NonEvidenceTool` 0（其余）。

#### 写模式工具（内置，`WriteCapable`）

只在 Mode ∈ {plan, apply, verify} 且写模式 agent 的 skill allowlist 下暴露给 LLM。详见 §4.6。

| 工具 | 独占 Agent | 行为 |
|------|-----------|------|
| `emit_change_plan` | planner | 校验 + 写 `Mutable.ChangePlan`。五步校验：（1）dup path、（2）empty / self / unknown dep、（3）depends_on DFS 找环、（4）kind=patch 时 Patch 非空、（5）**patch 预检**：每个 kind=patch 对 `ctx.RepoRoot` 跑 `git apply --check --recount`（带 patch(1) fallback），拒收在 emission 时暴露 git 错误 + `internal/tool/feedback.go::composePatchRejection` 附上冲突点 ±5 行真实文件片段 + 诊断建议。任一失败返回 ToolResult 失败 Summary，planner 下轮 retry（iteration cap 6）|
| `apply_patch` | coder | JSON schema 仅 `{path, kind}`，`DisallowUnknownFields` 拒绝任何内容字段。Execute 从 `Mutable.ChangePlan().Changes[i]` 直接取 `NewContent`/`Patch`，LLM 物理上无法转抄错内容。按 kind：`create`（`os.Stat` 必须不存在 → `os.WriteFile(unit.NewContent)`）/ `modify`（必须存在 → `os.WriteFile(unit.NewContent)`）/ `delete`（缺失视作幂等，warning）/ `patch`（pipe `unit.Patch` 到 `runUnifiedDiff`）。每次调用前强 W1（path ∈ TargetPaths；失败 Summary 枚举 valid 路径）+ W1b（DependsOn 都已 applied；失败 Summary 枚举当前 AppliedSet）。kind=patch 失败时 `composeApplyRejection` 带冲突文件片段 |
| `run_tests` | verifier | 9 种 runner 自动探测（见 §4.6）；build 阶段挂时合成 `build` suite 单条失败结果；run tests 异常时 `RawRef` 指向完整输出 blob |
| `emit_test_results` | verifier | 可选：LLM 写 FailureSummary narrative 叠加到 ChangeReport（不能覆盖 parser 产出的 Passed） |

**统一 diff 应用链**（`apply_patch.go::runUnifiedDiff` / `CheckUnifiedDiff`）：先 `git apply --recount`（pre-processor 对非 `\n` 结尾的 patch 文本补一个 `\n`；`--recount` 让 git 从 body 重算 `@@ -X,Y +X,Y @@` header，容忍 LLM 的 off-by-one 计数错误），失败回退 `patch -p1 --force --no-backup-if-mismatch --silent`（GNU patch(1) 的 fuzz 匹配挽救 git 严格模式拒绝但语义等价的 LLM-slop 形状：缺尾部 context、header 计数严重不符等）。`checkOnly=true` 时两条路径分别加 `--check` / `--dry-run`。rejection message 统一走 `internal/tool/feedback.go`，把 git stderr 里的 `patch failed: <file>:<N>` 解析后读文件 ±5 行带 `▶` 标记嵌入。

#### 结构化发射器（emit_* 系列）

| 工具 | 独占 Agent | 作用 |
|------|-----------|------|
| `emit_log_triage` | log_triager | 写 `LogBundle` 的 Layer 1-3：`Meta`（lang / signals / summary） + `Errors[]`（递归 `Cause` 链） + `Residue`（未结构化的 `unknown_chunks`）。Execute 调 `logtriage.ValidateBundle` 做路径 `os.Stat` 校验并派生 Layer 4（`ResolvedFiles` / `Entities` / `IntentHint` / `Coverage`），结果挂到 `Mutable.LogTriage()` 供分析 + explorer + extractor + finalizer 消费 |
| `emit_log_segmentation` | log_triager（两步升级 Step A） | 写按字节坐标切好的 segment 列表（kind = `stack` / `caused_by` / `header` / `context` / `trace` / `noise`），agent controller 随后对每个 `stack` / `caused_by` / `trace` 段逐段重调 `emit_log_triage`，`logtriage.MergeBundles` 合并结果 |
| `emit_analysis` | analyzer | 一次性写 `RequestModel`（intent / scenario / complexity / keywords / entities / question_kind / answer_shape / sub_topics / answer_subject / predicates / predicate_axis）；`ParseOutput` 随后跑确定性管线组装完整 `AnalysisIR` |
| `emit_evidence` | explorer | 批量写 `EvidenceItem`（kind / subject / object / source / line_start / line_end / anchor_kind / anchor_symbol / condition / summary）。`kind` 为 6 种 `IsLLMEmittable` 值之一。Execute 内同步调 `ground.GroundItem` 做 tier 验证 |
| `emit_investigation_complete` | explorer | 显式完成信号。需要 `reason` + `confidence`（high/medium），`low` 被拒。Execute 内跑 **CGEC I3** `preCompleteContractCheck`（6 条预检）并在失败时 downgrade + emit Repair |
| `emit_answer_symbol` | extractor | 写答案符号 slate + `CompletenessClaim`（`complete` / `lower_bound` / `unknown`）。`extractor.ParseOutput` 跑 cardinality validator 自动降级不诚实的 `complete` claim（基线 `max(β=TerminalEvidenceCount, γ=len(MustInclude))`） |
| `emit_hypothesis_verdict` | extractor | 为 `AnalysisIR.HypothesisSet` 的每条 hypothesis 写 status（`confirmed` / `rejected` / `inconclusive`）+ rationale + `file:line` citation。编排器 post-extract hook 通过 `AnalysisIR.MarkHypothesis` 写回 IR |
| `emit_answer_document` | finalizer | 按 shape 写 typed `AnswerDocument`。Execute 内同步调 `ground.GroundCitation` 验证 citations（**CGEC I2**），失败时 `AddRepair(RepairReadFile)`。**Literal-grounding gate**：每个带 citation 的 shape 都要求 claim 文本（`value.literal` / `symbols[i].name` / `steps[i].description` / `boolean.rationale`）与 cited file 的 ±3 行窗口至少有一个 identifier token 重叠；否则 Execute 返回 error + 指引使用 `citation_ref=-1`。覆盖 5/6 shape（explanation 例外，prose 全域不适用） |
| `propose_sub_agents` | explorer | 向编排器申请派生并行 sub-agent |

#### ToolResult 与 blob 机制

```go
type ToolResult struct {
    ToolName  string
    Summary   string
    RawRef    string    // 大输出落到 WorkDir 时写的文件路径
    Success   bool
    Timestamp time.Time
}
```

工具结果超过 `blob_max_inline_bytes`（默认 32 KB）时会 offload 到 WorkDir，只把 head/tail preview 塞进 LLM 上下文。Agent 想看全文就 `read_file` 指向 `RawRef`。WorkDir 默认 `<CWD>/.codrax/blob/<timestamp>-<pid>/`（per-process 共享，启动时按 `blob_max_sessions=7` 保留，存活 PID 永不删）；设 `blob_max_sessions: 0` 回退到 per-trace `os.MkdirTemp`+`RemoveAll`。

#### Turn A → Turn B Summary banner 约定

`ToolResult` 不带 `Params` 字段。Turn B 的 prompt 只通过 `ToolName` + `Summary` 两个渠道看到 Turn A 的调用。约束：**任何 Turn B 需要看到的 Turn A 调用参数（命令、pattern、path、flags），必须由工具自己写进 `Summary` 文本**。每个工具 `Execute` 里把 Summary 第一行做成自述 banner：

| 工具 | Summary 首行 banner |
|------|----------------------|
| `read_file` | `[path: showing lines X-Y of Z total]` 或 `[path: showing all N lines (B bytes); limit=L expanded ...]` |
| `exec_command` | `[exec_command: $ <command>]`（成功 / 失败 / 超时三条路径都挂） |
| `grep` | 两行：`[grep: N matching {lines,files}]` + `[grep params: pattern=... path=... file_type=... include=... context_lines=... case_insensitive=... files_only=...]` |
| `list_files` | `[list_files: path=... recursive=...]` |
| `git_diff` | `[git_diff: path=... ref=... staged=...]` |
| `git_log` | `[git_log: path=... count=... format=...]` |

辅助函数 `internal/tool/builtin.go::kvBanner(name, kv...)` 处理去空值 + `sanitizeForBanner` 消控制字符 + 200 字节截断。

下游消费者（`contract_check.go::strings.HasPrefix("[grep:")`、explorer 的 `extractFileCoverage` 解析 `[path: showing lines X-Y of Z]`、`runForcedReads` 的 `[forced_read]` 标记）都是字符串模式匹配 banner —— 改 banner 格式时必须同步查 parser。

#### Citation / evidence 路径规范化

`emit_answer_document` 的 citation 白名单和 `ground.GroundCitation` / `ground.GroundItem` 的 `LineIndex[file]` + `FileIndex[file]` lookup 都做字符串相等比较。为避免 LLM 用不一致路径形式（相对 vs 绝对、不同工具产出的 path）造成整批 citation 被 drop，所有 key 产地和查询端都穿过 `internal/tool/ground/path.go::CanonicalRepoRelative(path, repoRoot)`：

- empty → empty
- 绝对且在 `repoRoot` 内 → `filepath.Rel` 剥前缀
- 绝对但在 `repoRoot` 外（或 `Rel` 经 `..` 逃逸） → 保留 `filepath.Clean` 后的绝对形式
- 相对 → `filepath.Clean` 之后原样

落地点：`ground.BuildContext` / `buildLineIndex` / `GroundCitation` / `GroundItem`（原地 mutate `it.Source`）/ `emit_answer_document.turnAReadFileSet` / `buildEmitAnswerDocumentCitations`。

#### Evidence grounding tier 结构

`internal/tool/ground/ground.go`：

```go
type GroundingStatus string
const (
    GroundingGrounded   GroundingStatus = "grounded"
    GroundingRecovered  GroundingStatus = "recovered"
    GroundingUngrounded GroundingStatus = "ungrounded"
)

type GroundingTier string
const (
    TierLineText         GroundingTier = "line_text"          // T1
    TierSymbolTable      GroundingTier = "symbol_table"       // T2
    TierFQNameSameFile   GroundingTier = "fqname_same_file"   // R1
    TierSnippetFuzzy     GroundingTier = "snippet_fuzzy"      // R2
    TierPackageSymbol    GroundingTier = "package_symbol"     // R3
    TierNearestCall      GroundingTier = "nearest_call"       // R4
    TierNearestCondition GroundingTier = "nearest_condition"  // R5
)

type AnchorKind string
const (
    AnchorDefinition AnchorKind = "definition"
    AnchorCall       AnchorKind = "call"
    AnchorCondition  AnchorKind = "condition"
    AnchorReturn     AnchorKind = "return"
    AnchorAssignment AnchorKind = "assignment"
    AnchorImport     AnchorKind = "import"
)
```

- **T1** (`line_text`): LLM cited line 的原文包含 AnchorSymbol 作为整 token（支持 ±1 行邻居）
- **T2** (`symbol_table`): repomap.Graph 结构匹配 AnchorKind（`graphMatchDefinition` / `graphMatchCall` / `graphMatchImport`），还要求 line 落在某 symbol 的 `[Line - docRadius, EndLine]` 或 prologue `[1, firstSymbolLine - docRadius]`
- **R1-R5** Recovery tiers 在 T1/T2 失败时按顺序试：同文件 FQN、片段 fuzzy、包内 symbol、最近的 call、最近的 condition —— 命中后返回 `GroundingRecovered` + `AnchorAdjusted` 并记 `GroundingTier`
- 全部失败 → `GroundingUngrounded`，item 进独立 "Unverified Leads" 段，不进 citation pool

### 3.5 LLM（`internal/llm`）

可插拔 adapter 接口：

```go
type Adapter interface {
    Chat(messages []Message, tools []ToolSchema, opts ChatOptions) (Response, error)
    ModelID() string
    MaxContextTokens() int   // 从 providers.yaml :: context_window 注入；0 → 回落 128000
}
```

Per-agent 模型路由在 `providers.yaml` 配（不同 Agent 可指向不同模型 / 不同 provider）。provider 级降级链（主模型 → fast 模型）也在 provider config 声明。

**`context_window` 传播链**（`providers.yaml` 声明 → 全链路消费）：

1. `types.LLMProviderConfig.ContextWindow int` 承载 yaml 值，merge 规则是"非零覆盖"，agent-level 非 0 覆盖 default-level
2. `llm.NewOpenAIAdapter(..., contextWindow int, ...)` 在构造时存储；`MaxContextTokens()` 返回存储值，0 时回落 `128000` 作为保守默认（下游除数安全）
3. `cmd/root.go` 启动时 `logging.Info("[llm] default adapter: model=%s context_window=%d tokens", ...)` —— 操作员 sanity-check"codrax 认为模型能吃多少"
4. `config.ResolveByteBudget(fraction, absolute, codeDefault, contextWindow int)` 是 fraction-form 旋钮的单一真源：`fraction > 0 && contextWindow > 0` 时返回 `int(contextWindow * fraction * types.BytesPerToken)`，否则 absolute → codeDefault
5. `BaseAgent` 的 context-pressure watchdog（见 §3.2）直接读 `b.deps.LLM.MaxContextTokens()` 作阈值分母

**`types.BytesPerToken = 4`** 是整个项目的字节-token 换算常数，既用于 fraction-form 预算解析，也用于 watchdog 估算。保守（英文文本实际约 4 B/tok；CJK 约 2 B/tok，估算会过大所以更安全）。配置层 `config.BytesPerToken` 是类型别名，保证单一数值真源。

---

## 4. 阶段规范

### 4.1 `analyze` — 请求理解

| 方面 | 详情 |
|------|------|
| **Agent** | analyzer |
| **Skill** | `analysis-skill` |
| **工具** | `emit_analysis` + evidence-lite 预扫（`repo_map` / `grep`（强制 `files_only=true`） / `list_files`） |
| **输入** | 用户原始请求 |
| **工作** | **Phase A**：1-2 轮 evidence-lite 预扫，验证用户提到的实体/术语是否在仓库出现（**不读内容**）→ **Phase B**：一次 `emit_analysis` LLM 调用写 v4 RequestModel → `ParseOutput` 跑后处理管线（§7） |
| **输出** | `BusContext.AnalysisIR`（`TaskGraph` / `EvidencePlan` / `AnswerContract` / `HypothesisSet` / `QualityGate`） |

#### Evidence-lite 预扫边界规则

由 `internal/skill/analysis_contract.go::AnalysisHardRules` 的 `EVIDENCE-LITE BOUNDARY:` 规则 + `BaseAgent.executeTool::validateAnalyzerPrescanToolCall` 运行时 gate 共同强制：

- 只允许 `repo_map`、`grep`（必须 `files_only=true`）、`list_files` 三个只读导航工具——`BaseAgent.buildToolSchemas` 通过 `analysis-skill.ToolSuggestions` allowlist 物理裁剪 LLM schema。grep 没带 `files_only=true` 时 `validateAnalyzerPrescanToolCall` 合成失败 `ToolResult`，LLM 下一轮看到错误可重试。
- 预扫硬上限 2 轮，由 `analyzerEvaluator.Observe` 在 PhaseMidLoop 强制：每轮预扫工具调用让 `prescanRounds` +1；超过 `tool.AnalysisLimits.MaxPrescanRounds`（默认 2，`analysis_max_prescan_rounds` 可覆写；设 0 禁用）下一次预扫返回 `LoopSignal{StopRequested: true}`。
- **pre-scan → validator 数据通道**：同一次 `Observe` 把 `LastToolResult.Summary` 通过 `Mutable.AppendPrescanSummary` 追加到 per-dispatch 缓冲。`emit_analysis.Execute` 读取此缓冲作两个独立机制的输入：
  - **verified-entity 白名单**（`filterGenericEntitiesWithWhitelist`）：实体命中 generic blocklist 但小写形式出现在 seen-blob 里时保留并触发 `kept_generic_verified_entities` 告警。
  - **runtime quality probe**（`ComputeAnalysisQualityProbe`）：计算 `keyword_hit_ratio` 和 `entity_hit_ratio`，软阈值 `analysis_warn_below_keyword_hit_ratio` / `analysis_warn_below_entity_hit_ratio`（默认 0 = 不触发告警）控制 Summary `[warn: …]` 提示。完整 probe 结构以 `analysis_quality_probe` 写入 `StageOutput.Data`。

#### emit_analysis call-count gate

`analyzer.ParseOutput` 在走 `buildAnalysisIR` 前扫本次 dispatch 的 tool-result 流统计 `emit_analysis` 调用次数，结构化透出到 `StageOutput.Data` 的 `analysis_emit_calls` 字段：

- **0 次**：强告警 + hard `StageOutput.Error`，`runAnalyzePhase` 重试至 `MaxRetriesPerStage`；预算耗尽后 Run 终止。
- **1 次**：happy path。
- **>1 次**：由 `tool.AnalysisLimits.RejectMultipleEmit`（`analysis_reject_multiple_emit`）决定。默认 `false` → warning + 保留最后一次 + 继续；`true` → 额外把消息写进 `StageOutput.Error`。IR 始终按最后一次写入填充。

#### Quality Gate

`internal/analysis/gate.Run` 7 检查：

- **Hard**：`nil_ir` / `dag_closure` / `contract_complete` / `coverage` / `budget_sanity` / `hypothesis_coverage` / `criterion_resolvable`。Coverage 加权（Symbol=1.0, Config=0.7, Concept=0.4），阈值通过 `gate.GlobalThresholds()`（`gate_coverage_*`、`gate_hypothesis_min_priority`）。
- **Soft**：`pending_fields_wellformed`（pending artifact-exchange 字段检查）→ warning，继续跑。

### 4.2 `explore` — Turn A：调查 + 证据收集

| 方面 | 详情 |
|------|------|
| **Agent** | `AgentExplorer` (`internal/agent/explorer.go`) |
| **Skill** | `explore-skill` |
| **允许工具** | `grep` / `read_file` / `repo_map` / `list_files` / `exec_command` / `emit_evidence` / `emit_investigation_complete` / `propose_sub_agents` |
| **内部状态** | `explorerEvaluator.phase`（0=breadth, 1=depth），`complexity` / `answerSubject` / `predicateAxis` / `requiredFiles`（从 IR 缓存），`fileSymbols` / `searchResult` / `exactAnchorFiles` / `declarativeAnchorFiles` / `ermRequirements` / `cachedConcreteValues` |
| **输出** | `StageOutput.{EvidenceItems, AnswerChains, FlowFindings, StageReport}` + `MutableState.TurnAArtifacts` |

#### 两阶段工作流

- **Phase 0 — Breadth Scan**：`repo_map`（task_map 视图）+ `grep files_only=true` + `list_files` 快速定位相关文件，不读全文。LLM 产出 3-6 个文件的优先读取清单。
- **Phase 0 → Phase 1 质量门**（仅 `ContinuationsUsed >= 1` 时生效）：必须同时满足 (1) 用过 grep，(2) 用过 repo_map 或 list_files，(3) 发现 ≥ 3 个文件。任一未满足返回一次补救 prompt（`phase0ExtraRound` 确保最多触发一次）。
- **Phase 0 早期证据退出**：`firstSoftStop`（`ContinuationsUsed == 0`）且 `hasHighConfidenceEvidence` 为 true（history 中任何 `confidence > 0.5` 的成功 tool 结果）时跳过质量门直接接受停止（`e.phase = 1` + 空信号）。覆盖 exec_command / grep-only / read_file-only / list_files-only 等单工具即可回答的场景。
- **Phase 1 — Depth Read + Evidence Collection**：LLM 按清单 `read_file`，每读一个文件调 `emit_evidence(items=[...])`。大文件（>500 行）强制先 grep 再 slice read。行号必须来自 `read_file` 的 gutter。
- **ParseOutput（确定性管线）**：`ensureStructuredEvidence` 合并 emit_evidence buffer + markdown fallback → `groundEvidenceItems`（调 `ground.GroundItem`）→ `mergeEvidenceItems` → `rankEvidenceByRelevance` → `scrubSiblingEvidenceBlocks` → `identifyAnswerChains` → `SetTurnAArtifacts` 把投影快照交给 Turn B。

#### 证据排名

`rankEvidenceByRelevance`：`entity overlap × kindWeight × sourceWeight × bridgeBonus × producerBoost`。LLM 通过 `emit_evidence` 提交的证据（`IsLLMEmittable` + 非 ungrounded）获得 1.5x `producerBoost`；`EvidenceConcrete`（确定性自动提取）`kindWeight=0.50`；axis alignment 通过 `internal/analysis/axis::Affinity(PredicateAxis, AnchorKind)` 调节。

#### HasEnoughFacts 多维质量门

`ParseOutput` 计算 `HasEnoughFacts` 信号，决定 DAG 中带 `has_enough_facts` 入口条件的节点是否 ready。三子检查 AND：

- **toolDiversity**：`len(sources) >= 2`（sources 只计 `confidence > 0.5` 的工具）
- **fileCoverage**：`coverage >= 0.5 || len(readSet) >= 3`
- **evidenceQuality**：`directCount >= 2`（investigation notes 中的 `[DIRECT]`/`[REGISTRATION]` 标签）

枚举查询（`isEnumerationQuery`）阈值更严（覆盖率 ≥ 80%）。**单来源调查旁路**：`len(sources) == 1` 时三个 floor 全部 true，覆盖单工具调查场景。`emit_investigation_complete` 显式调用**无条件 override** 所有 heuristic。

### 4.3 `extract` — Turn B：答案结构化 + 假设判定

| 方面 | 详情 |
|------|------|
| **Agent** | `AgentExtractor` (`internal/agent/extractor.go`) |
| **Skill** | `extract-skill` |
| **允许工具** | **仅** `emit_answer_symbol` + `emit_hypothesis_verdict` |
| **禁止工具** | `read_file` / `grep` / `repo_map` / `list_files` / `exec_command` / `emit_evidence` |
| **ShouldStop** | `iteration >= 2`（one-shot + 一次 retry 窗口） |
| **输入** | `MutableState.TurnAArtifacts` digest + `AnalysisIR.HypothesisSet` + `AnswerContract.MustInclude` + (measurement-scalar only) Raw Tool Outputs 段 |
| **输出** | `StageOutput.{AnswerSymbols, AnswerSymbolCompleteness}` + 回写 `HypothesisSet[i].Status` |

Turn B 的两项**独特职责**：

1. **LLM-driven answer_symbol slate + completeness claim**。Phase 9 cardinality validator（`validateCompletenessClaim`）：LLM claim `complete` 但 `len(items) < max(TerminalEvidenceCount, len(MustInclude))` 时，claim 自动降级为 `lower_bound` + log warning。
2. **LLM-driven hypothesis verdict + citation**。`confirmed`/`rejected` 强制带 `file:line` citation。编排器 post-extract hook 调 `AnalysisIR.MarkHypothesis` 写回。注意：`rejected` 和 `inconclusive` 有确定性路径——`ParseOutput` 的 auto-verdict 用 `criterion.Eval` 评估 `FalsificationCondition` → rejected、`RequiredEvidence` all-pass → inconclusive。编排器还在每次 explore window 后调 `runAutoVerdicts`（同样逻辑，不调 LLM）+ `drainHypothesisVerdicts` 更新 IR。

#### Anchor skeleton for `shape=explanation + sub_topics ≥ 1`

`isMultiTopicExplanation(ctx)` 谓词：`AnswerContract.RequiredAnswerShape == ShapeExplanation` AND `RequestModel.SubTopics > 0` 时，`Observe.needSymbols = true`，把 `emit_answer_symbol` 提升为 expected emit。LLM 为每个 sub-topic 产一条 anchor symbol（load-bearing identifier + `file:line` + `rationale=子话题描述`），作为 finalizer 多段 prose 的骨架。

配套：`emit_answer_document` 对 `shape=explanation` 允许 optional `symbols[]` skeleton（仍禁 `steps[] / value / boolean`），completeness 在 skeleton 路径不要求；`render/answerdoc.go::renderAnswerDocExplanationSkeleton` 在 summary 和 citation pool 之间渲染 "Key anchors" / "关键锚点" 块；`answer_document_evaluator.go` 的 finalizer prompt 把 extractor 产的 per-topic slate 原样传下去附 "re-emit verbatim in symbols[]" 指令。单话题 explanation（`SubTopics == 0`）保持原路径——summary 就是答案。

#### Turn B 的诚实契约

Turn B 看不到新文件——所有信息在 Turn A transcript 快照里冻结。retry 带不来新信息。错了就降级为 `lower_bound` 而不是 retry。`ShouldStop` 设 `iteration >= 2`（而非 `>= 1`）给工具参数校验失败留一次 retry 窗口（LLM 下一轮看到 tool error 并修正参数）。`Observe` 在 `PhaseMidLoop` 检测至少一个**成功**的 emit_* 后立即 `StopRequested`。`PhaseSoftStop` 对 `list_of_symbols` 问题在 LLM 停下但未调 `emit_answer_symbol` 时注入一次 correction hint（`maxExtractorCorrectionRetries = 1`）。

### 4.4 `finalize` — 输出收敛

| 方面 | 详情 |
|------|------|
| **Agent** | finalizer |
| **Skill** | `answer-document-skill`（硬编码于 `topology.go`） |
| **工具** | `emit_answer_document`（独占） |
| **输入** | `BusContext.AnalysisIR.AnswerContract` + `AnswerSymbols` + completeness + `HypothesisSet` + Turn A 的 `StageReport` + (measurement-scalar only) Raw Tool Outputs 段 |
| **工作** | 按 `RequiredAnswerShape` 分支构造 typed payload → 调 `emit_answer_document` → renderer 渲染 |
| **输出** | `StageOutput.FinalAnswer`（写进 task.Result）|

#### 职责边界（严格不重叠）

| 产物 | 归属 | 路径 |
|------|------|------|
| `EvidenceItems` | **Explorer 独占** | `emit_evidence` + markdown fallback + merge |
| `AnswerChains` | **Explorer 独占** | 确定性 `identifyAnswerChains` |
| `FlowFindings` | **Explorer 独占** | 确定性 dataflow pipeline |
| `StageReport` prose digest | **Explorer 独占** | `renderExplorerStageReport` |
| `TurnAArtifacts` 快照 | **Explorer 写，Extractor 读** | `SetTurnAArtifacts` / `TurnAArtifacts()` |
| `AnswerSymbols` + completeness claim | **Extractor 独占** | `emit_answer_symbol` + Phase 9 validator |
| `HypothesisSet[i].Status` | **Extractor 写 buffer** | `emit_hypothesis_verdict` + `drainHypothesisVerdicts` |
| `AnswerDocument` | **Finalizer 独占** | `emit_answer_document` + renderer |

#### Summary 长度：shape-tiered cap

`emit_answer_document` 的 `summary` 长度由 `types.SummaryCapConfig` + `types.SummaryCapFor(shape, itemCount)` 决定。**默认 disabled**（`SummaryCapConfig.Enabled=false` → `SummaryCapUnlimited`）。`codrax.yaml` 的 `summary_cap_enabled: true` 激活：

| shape | 上限 | 用途 |
|---|---|---|
| `explanation` | `Explanation` (default 2500) | Summary **IS** 答案正文 —— 多段 prose + 可选 Mermaid |
| `boolean` | `Boolean` (default 800) | 1-3 句 lead-in + rationale |
| `value` / `config_value` | `Value` / `ConfigValue` (default 500) | 单句 lead-in + scalar literal |
| `step_list` | `min(StepListMax, StepListBase + n·StepListPerItem)` (default 1000 + 120·n, max 2500) | lead-in；随步数扩张 |
| `list_of_symbols` | `min(SymbolsMax, SymbolsBase + n·SymbolsPerItem)` (default 1000 + 100·n, max 2500) | lead-in；随 symbol 数扩张 |

全部字段通过 `codrax.yaml` 的 `summary_cap_*` 覆盖。`cmd/root.go` 加载 YAML 后调 `types.SetSummaryCapConfig` 一次性替换 package-level config，下游 `emit_answer_document` + shrinkage-salvage trimmer 共用。

#### Citation Quote：preview 预览截断，锚点不丢

`Citation.Quote` 是 optional 的 verbatim 源码预览，超过 `types.CitationMaxQuoteChars()`（默认 `DefaultCitationMaxQuoteChars = 500`）后**静默截到 UTF-8 边界**，`File` / `Line` 始终保留。Prose-smuggling 防御由 `ground.GroundCitation` 的 `QuoteMatched` token 匹配兜底（Quote token 跟源码行 ±2 行邻域无重合 → Quote 清空，走 `quoteCleared` → Tier-1-proven 闸决定是否丢掉整条 citation），跟长度无关。这条 cap 因此是纯渲染预览宽度，不是正确性门。运行时通过 `types.SetCitationMaxQuoteChars` 替换，`codrax.yaml :: citation_quote_max_chars` 单键覆盖；非正数忽略。

#### Forbidden 字段：reject 不 scrub

`rejectForbiddenFields`（`emit_answer_document.go`）**失败**整个 call 而不是静默清洗 —— shape=explanation 带着非零 `boolean{decision:"否", ...}` 会收到 `"shape=explanation forbids boolean{}; remove the field and retry"`，LLM 下一轮必清。零值字段（`value.Literal==""` / `boolean.Decision==""`）仍然静默 nil。`FinalizerMaxCorrectionRetries` default 3。

#### Shrinkage-salvage

`answer_document_evaluator` 对所有 shape 检测 "iter 0 rich prose → iter 1 压缩 summary" 模式：当 summary 明显少于前次 draft 的 `shrinkageRatio`（按 shape 缩放 floor：explanation 1.0×／list/step 0.5×／boolean 3/8×／value 0.25×），把 `findLastPreToolCallDraft(messages)` 选中的上一轮 draft verbatim 复制进 `summary[]`，UTF-8 rune-boundary trim 到 `SummaryCapFor(shape)`，追加双语 caveat，log `[finalizer/shrinkage]`。由 `agent_finalizer_*`（`preserve_prior_prose` / `shrinkage_min_prose_len` / `shrinkage_ratio`）控制。

### 4.5 Log Triage — 用户附加日志 → 结构化锚点注入下游

当用户通过 `--log <file|->` / `--log-text <inline>` / REPL `/log` 附加一条运行时日志时，在 analyze 之前会先跑一个独立的 `log_triage` 阶段。这个阶段是**条件触发**的（`BusContext.AttachedLog` 非空才激活），运行的是一个独立 agent（`log_triager`）而不是 analyzer 的 bolt-on；失败不阻塞主流水线。

**责任边界**（硬契约）：

| 侧 | 职责 | 不做的事 |
|---|------|---------|
| LLM（`log_triager` agent + `log-triage-skill`） | 读取 `AttachedLog` 正文，emit 结构化 `LogBundle` | 不做路径解析、不做仓内存在性验证、不填派生字段 |
| 系统（`internal/analysis/logtriage.ValidateBundle`） | 路径归一化、`os.Stat` 校验、Java basename 仓内 glob、运行时内部文件过滤、派生 `ResolvedFiles`/`Entities`/`IntentHint`/`Coverage` | 不改 LLM 发出的 `Meta` / `Errors` / `Residue` 三层 |

由 LLM 做抽取而不是写死的正则 parser，使得支持的日志格式不再是一个固定列表——Go panic、Java exception（含 `Caused by` 链 → 递归 `Cause` 指针）、C/C++ ASAN / UBSAN / gdb、Python traceback（含 `During handling` → 递归 `Cause`）、Node.js V8 stack、Rust `#[source]` 链、Ruby backtrace、结构化 JSON 应用日志、编译器错误报告都走同一套代码路径。

**LogBundle 数据结构**（`internal/types/log_bundle.go`）分四层：

| 层 | 来源 | 字段 | 说明 |
|---|------|------|------|
| 1. Meta | LLM | `Lang` / `Signals[]` / `Summary` | 日志整体分类。`Signals` 是 10 值 enum：`panic` / `crash` / `oom` / `timeout` / `permission` / `db` / `network` / `validation` / `logic` / `other` |
| 2. Errors | LLM | `[]LogError{Type, Message, Frames[], Cause *LogError}` | 错误树。顶层 slice 是**平行快照**（Go goroutine dump 里多个同时 panic 的 goroutine），`Cause` 指针是**时序因果链**（Java `Caused by` / Rust `#[source]` / Python `__cause__`）。递归深度由系统兜底截到 5 层 |
| 3. Residue | LLM | `UnknownChunks[]` | LLM 抽不进 `Errors` 的段（最多 8 段，每段 ≤ 500 字符） |
| 4. Derived | 系统 | `ResolvedFiles[]` / `Entities[]` / `IntentHint` / `Coverage` | 系统侧派生：`ResolvedFiles` 是 `os.Stat` 校验过的仓内路径（按 `Confidence` 降序，上限 10）；`Entities` 是从 `Func` / `Pkg` / `Error.Type` / `Signals` 派生的关键词（上限 32）；`IntentHint` 在有真实栈帧或信号含 `panic`/`crash`/`oom` 时置为 `IntentRootCause`；`Coverage` = `1 - bytes(Residue)/bytes(raw)`，驱动两步升级决策 |

每个 `LogFrame` 带 `File` / `Line` / `Func` / `Pkg` / `Raw`（原始日志行）/ `Confidence`（0.0-1.0）。系统侧把 `Confidence < 0.6` 或 `File == ""` / `Line == 0` 的帧保留在 bundle 中（贡献到 `Entities`）但不晋升到 `ResolvedFiles`——这是零信息丢失的恢复策略。

**数据流**：

```
--log / /log                             ┌──▶ Meta (Lang, Signals, Summary)
     │                                   │
     ▼                                   ├──▶ Errors[] (递归 Cause)
BusContext.AttachedLog     emit_log_triage├
     │                         │         └──▶ Residue (UnknownChunks)
     │              ┌──────────┘                         │
     │              ▼                                    ▼
     └──▶ log_triager agent ──▶ logtriage.ValidateBundle (os.Stat + 过滤 + 派生)
                                            │
                                            ▼
                                    Mutable.LogTriage() (LogBundle)
                                            │
              ┌─────────────────────────────┼──────────────────────────────┐
              ▼                             ▼                              ▼
   analyzer 读 bundle.Entities       reconcileIntent                 analyzerRequiredFiles
   union 进 AnalyzerHints.Entities   (IntentHint=RootCause →          读 bundle.ResolvedFiles
                                      强制切 root_cause)               prepend 到
                                                                       EvidencePlan.RequiredFiles
```

**下游 prompt 渲染**：`Mutable.LogTriage()` 镜像到 `AgentContext.LogTriage`，`context/builder.go::formatLogTriageStructured` 对非 `log_triager` agent 渲染一个"Log Triage — Validated Extraction" prompt section,内容按审计友好结构:

- **Meta 块**:`Language` / `Signals` / `Summary` / `Coverage` / `IntentHint` 逐行
- **Errors 树**:顶层按平行快照编号,递归 `Cause` 链以 `↳ caused by` 缩进渲染。每帧带 `★ resolved` 标记(当 File+Line 都验证通过时)/`(unresolved)` 标记(当 File 被清空或 Confidence < 0.6 时),末尾附 `confidence 0.XX` + `raw:` 原日志行
- **Unstructured residue**:`Residue.UnknownChunks` 显式标注"NOT citeable"
- **Provenance legend**:明确告诉 LLM ★ 的含义、不带 ★ 的帧不得 cite、每条 `raw:` 用于 auditor 交叉比对

该 section 放在 "Attached Runtime Log"(原始日志)**之前**,结构化视图优先,原文作为备查。修复了 Java / Python / Rust 多层 `Caused by` 链在 finalizer 重新 parse 原文时偶尔漏层的质量回退——LLM 直接读已验证的树而不是 re-derive。`log_triager` agent 自身跳过该 section(产品消费自环)。

**两步自适应抽取**：默认单次 `emit_log_triage` 调用。当 `len(AttachedLog) >= log_triage_two_step_bytes`（默认 32 KB）或单次抽取结果 `Coverage < log_triage_two_step_coverage`（默认 0.3）时自动升级：先调 `emit_log_segmentation` 让 LLM 按 `stack` / `caused_by` / `header` / `context` / `trace` / `noise` 切片打字节坐标，再对每个 `stack` / `caused_by` / `trace` 段分别调 `emit_log_triage`，最后 `logtriage.MergeBundles` 合并——`Meta.Signals` 并集、`Errors[]` 平行拼接、Layer 4 基于合并后的 Layer 1-3 重新派生。全过程 LLM 调用次数受 `log_triage_max_llm_calls`（默认 8）硬上限。

**路径解析与过滤**（`internal/analysis/logtriage/resolver.go`）：
- `StripBuildPathPrefix` 按优先级剥 `/build/src/` / `/rpmbuild/BUILD/` / `/home/<user>/src/` 等 CI / build 前缀 + repo basename + `log_triage_source_prefix` / `--log-source-prefix` 覆盖。
- `ResolveJavaFile(pkg, basename, candidates)` 处理 Java frame 只带 basename（`Bar.java`）不带目录的情况：用 package 后缀（`com.foo` → `**/com/foo/Bar.java`）消歧，tier 1（精确后缀）> tier 2（`src/main/java/` 布局后匹配）> tier 3（仅 basename）。
- `IsRuntimeInternalFile` 过滤 Go runtime（`asm_amd64.s` / `/go/src/runtime/`）、Node `node:` URI、`java.base/*`——这些不是 user repo 文件，不进 `ResolvedFiles`。
- `ResolveFrameFile` 做 `filepath.Rel` + `..` 逃逸检查 + `os.Stat` 硬校验，所有存在性都在仓内验证。

**Intent 覆盖**：`reconcileIntent(intent, predicates, *LogBundle)` 在 `bundle.IntentHint == IntentRootCause` 且 LLM declared 不是 `root_cause` 时强制切换。因为分析 agent 看得到 `AttachedLog` 原文（在 prompt section 里）但不是每次都会正确分类为调试查询，这里做 defence-in-depth。

**可调项**：

- **Triage 侧（`log_triage_*`）**：`log_triage_enabled` / `log_triage_source_prefix` / `log_triage_min_bytes` / `log_triage_max_retries` / `log_triage_two_step_enabled` / `log_triage_two_step_bytes` / `log_triage_two_step_coverage` / `log_triage_max_llm_calls`。
- **接入侧（`log_attach_*`，在 log_triage 之前生效）**：`log_attach_max_bytes`（默认 `1048576` = 1 MB）限制每条附加日志的字节上限。覆盖 `--log <file>` / `--log -`（stdin 用 `io.LimitReader(N+1)`，不会因为多 GB 管道把进程内存打爆）/ `--log-text` / REPL `/log <path>` / `/log` 粘贴模式 / `splitPastedLog` 自动识别路径共 5 条入口。超限尾部 `s[:N]` 截断并打 `WARN [cmd] attached log truncated`。在 `log_triage_enabled: false` 下也会生效——这是内存安全 knob,不是分诊 knob。非正值(含显式 0)回退到默认;显式把 cap 调到 0 不会静默废掉所有 `/log` 路径。

见 `codrax.yaml.example` 的逐项注释。

**暂不支持**：
- C/C++ glibc 裸 backtrace（只有返回地址，没 `file:line`，缺少足够锚点）
- 日志文件 tail / live stream / 远端源（Loki / ES / CloudWatch）

### 4.6 写模式 — plan → apply → verify

**触发条件**：`codrax.yaml :: write_enabled: true` **并且** `BusContext.Mode ∈ {ModePlan, ModeApply, ModeVerify}`（CLI `--mode=plan|apply|verify` 或 REPL `/mode plan|apply|verify` 设置）。两个条件缺一，`Run()` 在入口处 fail-loud 拒绝，避免误改动。

**与读模式的调度关系**：写模式的 3 个阶段（`plan` / `apply` / `verify`）**不经过** `runTaskGraph` 的 criterion-aware DAG scheduler。读模式的 analyze 先跑一次作为请求分类器，随后 `Run()` 分支到 mode-specific phase 函数，3 个 phase 通过 `dispatchStage` 直接分派：

| Mode | 阶段链 | Run 退出点 |
|---|---|---|
| `ModePlan` | analyze → `runPlanPhase` | Plan 写入 `Mutable.ChangePlan`；cmd/root.go 写 `--plan-out` 或 `.codrax/plans/<id>.json`；REPL 走 PlanStore 自动保存 |
| `ModeApply` | analyze → `runPlanPhase`（当 `PlanPath` 已设则跳过，复用 user-approved plan）→ `runApplyPhase` → `runVerifyPhase` | 三阶段任一失败 → fail-loud；全部成功 → `PlanStatus=applied`；worktree 默认销毁，开 `pipeline_keep_worktree_on_success` 则保留 |
| `ModeVerify` | analyze → `runVerifyPhase` | 独立 re-verify：对已有 plan 的 worktree 重跑测试；不会重新 apply |

读模式（`ModeRead`）继续走 `runTaskGraph`，字节级行为不变（L1 红线）。

#### 四模式枚举

`internal/types/pipeline_mode.go`：

```go
const (
    ModeRead   PipelineMode = "read"
    ModePlan   PipelineMode = "plan"
    ModeApply  PipelineMode = "apply"
    ModeVerify PipelineMode = "verify"
)
```

BusContext 默认 `ModeRead`。Mode 是粘滞的：REPL `/mode plan` 之后所有提问都走 `ModePlan`，直到显式切回。

#### Agent + 工具

| Agent | Stage | 工具 | 职责 |
|---|---|---|---|
| `planner` | `plan` | `read_file` / `grep` / `list_files` / `repo_map` / `exec_command` / `emit_change_plan` | 读代码理解当前结构，产出完整 ChangePlan；不写文件；iteration cap 6（预检拒收让每次调用可能变成多轮重试，cap 给足余量）；`BuildInitialInstruction` 消费一次 `Mutable.PlanningHint()`（verify→plan retry loop 注入） |
| `coder` | `apply` | `read_file` / `apply_patch` / `exec_command` | 按 `plan.Changes[]` 逐单元调用 `apply_patch(path, kind)`；工具从 `Mutable.ChangePlan()` 读内容，LLM 不转抄；ShouldStop 在 `WriteClosure.AppliedSet ⊇ plan.TargetPaths` 时触发；iteration cap = `len(TargetPaths) + 3` |
| `verifier` | `verify` | `read_file` / `run_tests` / `emit_test_results`（可选） / `exec_command` | `BuildInitialInstruction` 总是发射 `## Verify phase` 定向指令（即使 plan 无 AcceptanceTests 也不返回空串，避免 LLM 落到 system prompt 之外的次级信号）；`run_tests` 自动探测 runner 并同步 install `ChangeReport`；`emit_test_results` 是可选 LLM narrative；权威 pass/fail verdict 来自 parser，不由 LLM 覆盖 |

所有 3 个 agent 嵌 `BaseAgent`，沿用 ReAct 循环 + Evaluator 钩子（BuildInitialInstruction / ShouldStop / ParseOutput / DetermineMissingPiece），与读模式 agent 同构。3 个 agent 均**不**实现 `LoopController`（写阶段无 mid-loop 注入逻辑）。

| Tool | 所在 agent | 行为 |
|---|---|---|
| `emit_change_plan` | planner | 校验 Changes[] 五步：（1）dup-path / （2）empty / self / unknown dep / （3）cycle-via-DFS / （4）kind=patch 要求 Patch 非空 / （5）**patch 预检**：每个 kind=patch 对 `ctx.RepoRoot` 跑 `CheckUnifiedDiff`（`git apply --check --recount`，失败回退 `patch -p1 --dry-run`）。全过才写 `Mutable.ChangePlan`；`TargetPaths` 从 Changes 派生 |
| `apply_patch` | coder | JSON schema 仅 `{path, kind}`（`new_content` / `patch` 被 `DisallowUnknownFields` 拒绝）。Execute 从 `Mutable.ChangePlan().Changes[i]` 直接取 `NewContent` / `Patch`。四种 kind：`create`（`os.Stat` 必须不存在 → `os.WriteFile(unit.NewContent)`）/ `modify`（必须存在 → `os.WriteFile(unit.NewContent)`）/ `delete`（缺失幂等 + warning）/ `patch`（`runUnifiedDiff(ctx.RepoRoot, unit.Patch)`）。每次 Execute 前强制 W1 + W1b，失败 Summary 枚举当前 valid 集（TargetPaths 或 AppliedSet） |
| `run_tests` | verifier | 9 种 runner（Go / Node / Python / Rust / Java / Ruby / CMake / Meson / Make），通过 manifest 文件探测；支持 JUnit XML / JSON / 文本 / 退出码 4 类输出协议；build 阶段失败（XML 未产出 / exit≠0）合成 `build` suite 单条失败结果，`FailureDetail` 用 `extractBuildErrorExcerpt` 抽取 `[ERROR]` / `FAILURE:` / `error:` 等通用编译错误标记 |
| `emit_test_results` | verifier | 可选；LLM 在 FailureSummary 里写人话（区分 REGRESSION / PRE-EXISTING / FIXED）。不能覆盖 parser 的 `Passed` 字段 |

#### Patch 应用链（`internal/tool/apply_patch.go`）

`CheckUnifiedDiff`（预检，emit 时） / `applyUnifiedDiff`（落盘，apply 时）共用 `runUnifiedDiff`：

1. **Pre-processor**：非 `\n` 结尾的 patch 文本补一个 `\n`，防止 git 把未终结的尾行当 EOF。
2. **Primary**：`git apply --recount [--check]`。`--recount` 让 git 从 body 重算 `@@ -X,Y +X,Y @@` 的 line 数，容忍 LLM 的 off-by-one 计数错误（git 官方文档称此 flag 专为"hand-edited patch header 没更新"设计）。
3. **Fallback**：primary 失败后试 `patch -p1 --force --no-backup-if-mismatch --silent [--dry-run]`。GNU patch(1) 的 fuzz 匹配挽救 git 严格模式拒绝但语义等价的 LLM-slop 形状（缺尾部 context、context 行有空格漂移等）。
4. **错误透传**：两条路径都失败时返回 git 的 stderr（它是更严格 validator，诊断信息量更大）。

#### Rejection 增强（`internal/tool/feedback.go`）

LLM 失败时 retry 的唯一"事实源"是 tool 的 rejection Summary。两条专用 composer 把 git stderr 解析成带文件真实内容的诊断，让 LLM 的下一次尝试有 ground truth 而非再次凭记忆生成：

- `parseGitConflictLocator(gitErr)` 抽 `patch failed: <file>:<N>` 的 `(file, line)`
- `conflictContextSnippet(repoRoot, gitErr)` 读文件 ±5 行，用 `▶` 标记 claim line 产出 markdown 片段
- `composePatchRejection(repoRoot, path, gitErr)` — emit_change_plan 预检失败用；prefix "emit_change_plan rejected"
- `composeApplyRejection(repoRoot, path, gitErr)` — apply_patch 运行时失败用；prefix "apply_patch: git apply failed"
- 无 `patch failed: <file>:<N>` 可解析时（如 "corrupt patch at line N" 这类指 patch body 内部而非文件的错误）fallback 到通用 hint string

W1 / W1b 失败的 Summary 同样枚举参考集（W1 → valid TargetPaths；W1b → current AppliedSet），LLM 看到完整的修正空间，不需再读 plan 或猜。

#### ChangePlan 数据结构（`internal/types/change_plan.go`）

```go
type ChangePlan struct {
    ID              string          // plan-<unix-nano>-<pid>
    Request         string          // 用户原始自然语言请求
    Summary         string          // planner 写的 3-10 句总结
    Changes         []FileChange    // 顺序敏感的文件级变更
    AcceptanceTests []string        // 自然语言 AcceptanceTests（verifier prompt 可见）
    TargetPaths     []string        // 声明的写作用域（W1 门）
    Status          string          // pending_approval | applied | applied_failed | verify_failed | rejected
    AppliedCommitSHA string         // worktree commit sha
    WorktreePath    string          // 保留的 worktree 路径（keep_on_success 开启）
    CreatedAt       time.Time
    AppliedAt       *time.Time
}

type FileChange struct {
    Path       string   // 仓库相对路径
    Kind       string   // create | modify | delete | patch
    NewContent string   // create / modify 的完整 body
    Patch      string   // kind=patch 的 unified diff
    Rationale  string   // 为什么要改这个文件
    DependsOn  []string // 本 plan 内必须先 apply 的其他路径
}
```

`LoadChangePlanFromFile` 读盘时重算 `TargetPaths` 为 `Changes[].Path` 的去重列表，并拒绝 `len(Changes)==0` / 重复 path 的 plan（兜底手改坏的 JSON）。`UpdatePlanStatusOnDisk(path, status, appliedAt, worktreePath)` 做局部更新，空 worktreePath 语义是"不动 WorktreePath 字段"，让后续生命周期更新（apply → verify_failed）不会抹掉之前 applied 时写入的 worktree 路径。

#### CGEC-W：WriteClosure 写闭包（W1 / W1b）

对应读模式的 CGEC（Citation-Grounded Evidence Closure），写模式有 **Write Closure** 两条不变量，由 `apply_patch.Execute` 在文件 I/O 之前强制：

- **W1**：`params.Path` 必须在 `ChangePlan.TargetPaths` 里。违规 → 失败 ToolResult，Summary **枚举当前 valid TargetPaths 全集** + 指令 "pick one of the listed paths or abandon this target"，coder 下轮可见错误自修正。
- **W1b**：当前 `ChangeUnit.DependsOn` 列的每个前置路径都必须已经在 `WriteClosure.AppliedSet` 里。违规 → 失败 ToolResult，Summary **枚举当前 AppliedSet 全集** + 指令 "apply X first, then retry Y"。

此外 Execute 还做 kind 一致性检查（tool call 的 kind 必须等于 `unit.Kind`）、path 越界检查（traversal via `..` 拒绝）。

成功 apply 后 `WriteClosure.MarkApplied(path)`。**幂等性**：二次 apply 同一路径是无操作 success。coder 的 ShouldStop 等价于 `WriteClosure.AppliedSet ⊇ plan.TargetPaths`。

结构性测试 `internal/tool/write_mode_red_lines_test.go` 用 `go/ast` 扫描断言 `apply_patch` / `emit_change_plan` / `run_tests` **从不** import `internal/tool/ground`（L3：写路径不是 citation 路径，grounding 语义无意义）。

#### Worktree 沙箱生命周期

`internal/worktree/session.go`：

1. `runApplyPhase` 入口 `worktree.Create(mainRoot, traceID, baseDir)` → `git worktree add` 生成 `<baseDir>/<trace-id>-<pid>/`，注册进 `activeSessions`。
2. `BusContext.RepoRoot` 临时换到 worktree 路径，`coder` 的所有 `apply_patch` 都相对这个路径写。
3. `runVerifyPhase` 继续复用 worktree（RepoRoot 仍指向 worktree），`run_tests` 在 worktree 里跑测试。
4. `Run()` 顶层 defer 决定清理策略：
   - 失败路径 → **无条件** `worktree.DiscardByPath`（避免磁盘堆积）
   - 成功路径 + `pipeline_keep_worktree_on_success == true` + `Mode == ModeApply` + `TaskState.LastError == ""` → 跳过清理，`busCtx.WorktreePath` 暴露给 caller，`persistPlanStatus` 额外把路径写入 `plan.WorktreePath`
   - 其他（成功但没开 keep knob，或读模式 Run 连 WorktreePath 都没设）→ 清理（读模式的 `DiscardByPath("")` short-circuit）
5. SIGINT / SIGTERM **不触发 defer**（Go 默认 `os.Exit`），由 `internal/worktree/signal_exit_unix.go` / `signal_exit_windows.go` 装的进程级信号 handler 遍历 `activeSessions` 统一 Discard 再 re-raise。
6. SIGKILL / 电源丢失无法在进程内清理 → 下次启动的 `worktree.PruneDeadSessions` 按嵌入 PID 的存活性扫 `baseDir` 回收孤儿。

REPL `/worktree list` 扫 PlanStore 过滤 `Status=applied && WorktreePath != ""`；`/worktree discard <plan-id>` 调 `DiscardByPath` + 清 `plan.WorktreePath` 字段。

#### 写模式 Criterion（`internal/analysis/criterion/eval.go`）

读模式 19 种 Criterion Kind 之外，写模式新增 4 种，被 `AnswerContract.AcceptanceTests` / `TaskNode.SuccessCriteria` 消费：

| Kind | 语义 | 评估输入 |
|---|---|---|
| `CritPlanReady` | `ChangePlan` 已发射且非空、`WriteClosure.PendingApplies > 0` | Mutable.ChangePlan |
| `CritPatchApplies` | `AppliedSet ∩ TargetPaths == TargetPaths`（所有声明路径都已 applied） | Mutable.WriteClosure |
| `CritTestsPass` | `ChangeReport != nil && ChangeReport.Passed` | Mutable.ChangeReport |
| `CritNoRegression` | `BaselineReport` nil → Satisfied=true（无快照可比）；否则比较 baseline 与 current 的 `MetricDeltas`，超过每项 `Threshold` 视作回归 | Mutable.BaselineReport + Mutable.ChangeReport |

读模式 Run 里这 4 个 env slot 都是 nil，`CritPlanReady` / `CritPatchApplies` 等对应 evaluator 直接返回 Satisfied=true（L3 byte-identity 红线）。

#### 可选：verify→plan 重试循环

`pipeline_max_verify_retries`（yaml，默认 0，硬上限 5）控制 `ModeApply` 里 `plan → apply → verify` 的最大迭代次数。第一次失败后：

1. `orchestrator.prepareVerifyRetry(attempt)`：
   - 调 `buildRetryHint(ChangeReport, ChangePlan, prevAttempt)` 合成 PlanningHint：失败 summary（≤300 字符）+ top-3 失败测试（AssertionID + Suite + FailureDetail 首行 ≤140 字符）+ `plan.TargetPaths`（cap 10）作"嫌疑文件清单"。上限 1500 字符。
   - `worktree.DiscardByPath` 清掉上一 worktree，`busCtx.RepoRoot` 回归 `MainRepoRoot`
   - `Mutable.ResetChangePlan` / `ResetChangeReport` / `WriteClosure.Reset` / 清 `o.planPath`（强制重新规划，即使 user 原本供了 plan-file）
   - `BaselineReport` 保留（规范化的 pre-apply 快照，跨 retry 稳定）
   - `Mutable.SetPlanningHint(hint)`
2. `plannerEvaluator.BuildInitialInstruction` 消费一次 `Mutable.PlanningHint()` —— reset 完再取就是空（避免下个 sub-dispatch 重复注入）。
3. 下一次 `runPlanPhase` 正常跑，planner 看到 hint 里的嫌疑文件 + 失败 narrative 修订 plan。

失败到顶：`verifyErr > applyErr > planErr` 的优先级把最深的一层塞进 `TaskState.LastError`。

#### 可选：Baseline 捕获

`pipeline_baseline_capture_enabled`（yaml，默认 `false`）打开后，`runApplyPhase` 3b 步（coder dispatch 前、worktree 已 swap 但尚无任何写入）主动跑一次 `tool.RunTests{}.Execute` 作为 baseline：

1. `run_tests` 正常 install 到 `Mutable.ChangeReport`
2. 立即 `Mutable.SetBaselineReport(report)` + `ResetChangeReport`（腾出槽位给后续 verify）
3. 可选持久化到 `.codrax/plans/<id>.baseline.json`（磁盘失败只 warning）
4. Baseline **失败非致命**：`evalNoRegression` 见 nil baseline 短路为 Satisfied=true

Verifier 的 `BuildInitialInstruction` 在 baseline 非 nil 且有失败测试时渲染 `## Pre-existing baseline failures` 段（cap 15 条），教 LLM 用 REGRESSION / PRE-EXISTING / FIXED 分类写 FailureSummary narrative；`test-execute-skill` 的 Workflow 也声明这条约束。

#### 测试 runner 矩阵

| Runner | Manifest | 命令 | 输出协议 |
|---|---|---|---|
| Go | `go.mod` | `go test -json ./...` | JSONL stream，`goTestEvent` 事件流 |
| Node | `package.json` | `npm test -- --json --silent` | Jest/Vitest 共通的单 JSON 对象（`testResults[].assertionResults[]`） |
| Python | `pyproject.toml` / `pytest.ini` / `setup.py` | `pytest --json-report --json-report-file=<tmp>` | `pytest-json-report` 插件写 JSON 到 extraFile |
| Rust | `Cargo.toml` | `cargo test` | 文本：`test ... ok`/`FAILED`，`test result: FAILED. N passed; M failed`，失败块 `---- name stdout ----` |
| Java Maven | `pom.xml` | `mvn -B -q test` | `target/surefire-reports/*.xml`（JUnit XML，post-exec 目录扫描） |
| Java Gradle | `build.gradle` / `build.gradle.kts` | `./gradlew --no-daemon --console=plain test` | `build/test-results/test/*.xml`（JUnit XML） |
| Ruby | `Gemfile` | `bundle exec rspec --format json` | 单 JSON 对象，`examples[].status` |
| CMake | `CMakeLists.txt` + 已配置 build dir | `ctest --test-dir <build> --output-junit <tmp> --output-on-failure` | 单 JUnit XML 文件 |
| Meson | `meson.build` + 已配置 build dir | `meson test -C <build> --xunit-file <tmp>` | 单 JUnit XML 文件 |
| Make | `Makefile` / `makefile` / `GNUmakefile` | `make check` / `make test`（按 Makefile 首列扫择优） | 无结构化输出；`parseMakeOutput(stdout, runErr)` 用 exit code 判定 + `extractBuildErrorExcerpt` 抽取 stdout |

CMake / Meson 要求 build dir 已配置（codrax 不跑 configure 步），探测的目录名：`build`（CMake / Meson）/ `Build` / `builddir`（Meson 默认）/ `out` / `cmake-build-debug` / `cmake-build-release`（CLion 默认）。每个候选必须含 `CMakeCache.txt`（CMake sentinel）或 `meson-info/`（Meson sentinel）才算数，避免空 `build/` 误判。

**共用的 build-failure 合成**：Java / CMake / Meson 任何一个 runner 的 XML artifact 没产出（build 阶段就挂了）时，`synthesizeBuildFailureReport(ctx, toolName, runner, assertionID, label, output)` 合成一条 `{Passed: false, AssertionID: "<lang>-build", Suite: "build", FailureDetail: extractBuildErrorExcerpt(output)}`，写入 `Mutable.ChangeReport` 并在 ToolResult Summary 里带上截取的第一条错误行 —— retry loop 的 PlanningHint 因此总能拿到具体错误文本而不是"build 失败"这种空信号。

#### 红线总结

- **L1**：读模式 Run 字节级行为不变（`runTaskPhase` 不变）；写模式 opt-in 从不影响读模式。
- **L2**：`write_enabled: false`（默认）下写模式阶段**拒绝启动**。
- **L3**：写工具（`emit_change_plan` / `apply_patch` / `run_tests` / `emit_test_results`）**不得** import `internal/tool/ground`；由 `write_mode_red_lines_test.go` 结构性扫描固化。
- **L5**：worktree 清理 defer 位于 `Run()` 顶层，失败路径**无条件**触发；keep-on-success 仅是成功路径的 opt-out。
- **L6**：写模式的 skill（`change-plan-skill` / `code-write-skill` / `test-execute-skill`）在 `ToolSuggestions` 里**保留** `exec_command` —— worktree 沙箱已经把 blast radius 限住，LLM 偶尔用 `git status` / `ls` 诊断是合理的。

---

## 5. 数据结构

> **BusContext 不是 model context 本身。** 它是构建 Agent 专属 model context 的运行时事实源：
>
> `BusContext`（完整共享状态） → 裁剪 → `AgentContext`（Agent 范围视图） → 组装 → `PromptContext`（模型 prompt 载荷） → 发送 LLM

### 枚举

```go
// internal/types/enums.go
type PipelineStage string
const (
    // 读模式阶段
    StageLogTriage PipelineStage = "log_triage" // 条件前置；AttachedLog 非空才触发
    StageAnalyze   PipelineStage = "analyze"
    StageExplore   PipelineStage = "explore"
    StageExtract   PipelineStage = "extract"
    StageFinalize  PipelineStage = "finalize"
    // 写模式阶段（Mode ∈ {ModePlan, ModeApply, ModeVerify} 时才触发；
    // runPlanPhase / runApplyPhase / runVerifyPhase 直分派，不走
    // runTaskGraph scheduler）
    StagePlan   PipelineStage = "plan"
    StageApply  PipelineStage = "apply"
    StageVerify PipelineStage = "verify"
)

// IsWrite 是读→写 context 隔离的单一真源。context.BuildAgentContext
// 对 write stage 跳过所有读模式字段传播；BuildPromptContext 对
// StageApply / StageVerify 抑制 User Request 段。StageAnalyze 即使
// 在写管线里也视为读模式（分类器仍需其读模式输入）。
func (s PipelineStage) IsWrite() bool {
    return s == StagePlan || s == StageApply || s == StageVerify
}

type AgentName string
const (
    // 读模式 agent
    AgentLogTriager AgentName = "log_triager" // 仅 log_triage 阶段使用
    AgentAnalyzer   AgentName = "analyzer"
    AgentExplorer   AgentName = "explorer"   // Turn A
    AgentExtractor  AgentName = "extractor"  // Turn B
    AgentFinalizer  AgentName = "finalizer"
    // 写模式 agent
    AgentPlanner  AgentName = "planner"
    AgentCoder    AgentName = "coder"
    AgentVerifier AgentName = "verifier"
)

// internal/types/pipeline_mode.go — 写模式粘滞状态
type PipelineMode string
const (
    ModeRead   PipelineMode = "read"   // 默认
    ModePlan   PipelineMode = "plan"
    ModeApply  PipelineMode = "apply"
    ModeVerify PipelineMode = "verify"
)

// IsWrite 区分读 vs 写（空串和 ModeRead 均为读）
// ModePlan 虽然不改主仓字节，但产出 ChangePlan 副作用，仍算写

// internal/types/change_plan.go — ChangePlan 生命周期状态
const (
    PlanStatusPending      = "pending_approval" // emit_change_plan 刚写
    PlanStatusApplied      = "applied"           // apply + verify 双双成功
    PlanStatusApplyFailed  = "applied_failed"    // apply 阶段 fail
    PlanStatusVerifyFailed = "verify_failed"     // apply 成功但 verify fail
    PlanStatusRejected     = "rejected"          // REPL /reject
)

type Intent string      // IntentExplain / IntentRootCause / IntentTrace /
                        // IntentEnumerate / IntentConfigQuery /
                        // IntentReturnValue / IntentUnknown

type Scenario string    // ScenarioArchitectureExplain / ScenarioRootCause /
                        // ScenarioConfigTrace /
                        // ScenarioPerformanceBottleneck / ScenarioGeneric

type Complexity string  // ComplexitySimple / ComplexityModerate / ComplexityComplex

type MissingPiece string
const (
    MissingNone          MissingPiece = "none"
    MissingUnderstanding MissingPiece = "understanding"
    MissingFacts         MissingPiece = "facts"
)

type TaskStatus string
const (
    TaskPending    TaskStatus = "pending"
    TaskInProgress TaskStatus = "in_progress"
    TaskDone       TaskStatus = "done"
    TaskBlocked    TaskStatus = "blocked"
    TaskFailed     TaskStatus = "failed"
)

type AnswerShape string
const (
    ShapeListOfSymbols AnswerShape = "list_of_symbols"
    ShapeStepList      AnswerShape = "step_list"
    ShapeValue         AnswerShape = "value"
    ShapeBoolean       AnswerShape = "boolean"
    ShapeConfigValue   AnswerShape = "config_value"
    ShapeExplanation   AnswerShape = "explanation"
    ShapeNone          AnswerShape = "none"  // 不能落进 AnswerDocument
)

// internal/types/analysis_ir.go — AnswerSubjectKind
// SubjectUnknown / SubjectFunctionName / SubjectTypeName / SubjectHandlerRoute /
// SubjectConfigKey / SubjectReturnValue / SubjectFilePath / SubjectStringLiteral /
// SubjectNumeric / SubjectEnumValue / SubjectStructField / SubjectInterface /
// SubjectGeneric（E1 hard-fallback 保证下游永不拿到 SubjectUnknown）

// internal/types/evidence.go — CompletenessClaim
// "complete" / "lower_bound" / "unknown"

// internal/types/violation.go — ViolationKind
// ShapeViolation / CitationViolation / LiteralFormFailed /
// GhostAnchor / SelfRefLiteral / Other

// internal/types/repair.go — RepairKind
// RepairReadFile / RepairExpandSearch / RepairSwapShape /
// RepairRebindSubject / RepairForceCompleteDowngrade
```

### BusContext

```go
type BusContext struct {
    Mutable *MutableState  // 工具可写域

    TaskState     TaskState
    PipelineStage PipelineStage
    ActiveAgent   AgentName

    RepoRoot  string
    Branch    string
    Commit    string
    WorkDir   string
    ModuleMap []string

    RepoFacts                []RepoFact
    EvidenceItems            []EvidenceItem
    FlowFindings             []FlowFindingDigest
    AnswerChains             []AnswerChain
    AnswerSymbols            []AnswerSymbol
    AnswerSymbolCompleteness CompletenessClaim
    ToolResults              []ToolResult
    StageReports             []StageReport

    Signals ExecutionSignals   // 只剩 HasEnoughFacts

    Constraints []string
    Preferences []string

    LastTransitionReason string
    TraceID              string

    AnalysisIR *AnalysisIR
}

type ExecutionSignals struct {
    HasEnoughFacts bool
}
```

### AnalysisIR

Analyzer 是**唯一** writer；下游 stage 只能通过 dedicated API（`MarkHypothesis`、per-node 执行状态）做受控修改。

```go
type AnalysisIR struct {
    Version        string          // "v4"
    TraceID        string
    RequestModel   RequestModel    // Intent / Scenario / Complexity / Predicates /
                                   // PredicateAxis / SubTopics / TermGraph /
                                   // RiskMatrix / AnalyzerHints / AnswerSubject /
                                   // ShapeConfidence / ComplexityConfidence /
                                   // IntentConfidence
    TaskGraph      TaskGraph       // Nodes / Edges / ExecutionPolicy
    EvidencePlan   EvidencePlan    // Budget / SourceMix / StopConditions /
                                   // NodeBudgetHints / RequiredFiles
    AnswerContract AnswerContract  // RequiredAnswerShape / MustInclude /
                                   // CitationReq / Language / AcceptanceTests ([]Criterion)
    HypothesisSet  []Hypothesis
    QualityGate    GateReport
}
```

#### TaskNode 类型

```go
type TaskNodeType string
const (
    NodeProbe     TaskNodeType = "probe"
    NodeEvidence  TaskNodeType = "evidence"
    NodeValidate  TaskNodeType = "validate"
    NodeReconcile TaskNodeType = "reconcile"
    NodeFinalize  TaskNodeType = "finalize"
)
```

Scheduler 的 `stageMapping` 把前四种节点全部映射到 `StageExplore`，只有 `NodeFinalize` 映射到 `StageFinalize`。`TaskNode.EntryConditions` / `SuccessCriteria` 是 `[]Criterion`（typed，由 `criterion.Eval` 运行时求值，19 种 registered Kinds）。

#### RiskMatrix

六维 0-5 打分（`Security` / `DataIntegrity` / `Compatibility` / `Performance` / `Ops` / `Compliance`），`risk.Evaluate` 从 term graph 推导。`hdp.Plan` 根据 risk level 决定是否额外 plan hypothesis，`priority.Score` 对每条 hypothesis 做 4 维打分，`binder.BindByRelevance` 基于相关性绑定到 TaskNode。

### MutableState

BusContext 中唯一允许工具直接 mutate 的区域，通过指针共享。内置 RWMutex。Sub-agent 不共享这个区域（`BuildSubAgentContext` 故意把 `ac.Mutable` 留成 nil）。公开 API goroutine-safe：`TaskList()` / `SetTaskList()` / `UpdateTaskStatus()` / `UpdateTaskResult()` / `SetCurrentTask()`，以及 emit_* tool 用的 buffer getter/setter + `Reset*` 家族（跨 task 清零）。

重要字段：`requestModel`、`emittedEvidence`、`emittedAnswerSymbols` + `emittedAnswerSymbolCompleteness`、`emittedHypothesisVerdicts`、`turnAArtifacts`、`searchGraph`（repomap.Graph handle）、`phase1Ranking`、`dispatchToolResults`（per-dispatch 内 grounding 用）、`evidenceClosure` (CGEC 状态总线)。

### AgentContext

```go
type AgentContext struct {
    AgentName AgentName
    Stage     PipelineStage

    Objective              string
    CurrentTaskID          string
    CurrentTask            string
    CurrentTaskDescription string

    AnalysisIR *AnalysisIR  // 别名 BusContext.AnalysisIR

    RelevantFacts            []string
    RelevantFiles            []string
    EvidenceItems            []EvidenceItem
    FlowFindings             []FlowFindingDigest
    AnswerChains             []AnswerChain
    AnswerSymbols            []AnswerSymbol
    AnswerSymbolCompleteness CompletenessClaim
    RelevantToolSummaries    []string
    PriorReports             []StageReport

    Constraints []string
    Preferences []string

    MissingPiece MissingPiece
    RetryHint    string           // contract-check backtrack 时带进来的违规诊断
    PriorConvHidden bool          // PriorConvPolicy 门控

    UnverifiedAnalyzerFindings  []UnverifiedFinding  // CGEC C1 渲染
    SubjectMatches              []SubjectMatch       // CGEC E4/E5 渲染
    ExpectedAnswerSubject       *AnswerSubject

    RepoRoot string
    Branch   string
    Commit   string
    WorkDir  string

    Mutable *MutableState  // 别名 BusContext.Mutable 让工具可写
}
```

### EvidenceClosure（CGEC 状态总线）

`internal/types/evidence_closure.go`。跨 stage 共享的证据闭环总线，每字段都有 ≥1 production consumer（`TestEvidenceClosureAllFieldsHaveConsumer` 锁死）。

| 字段 | Producer | Consumer |
|---|---|---|
| `readSet` | explorer.SetReadSet | I1/I2/I3/I4 全读 |
| `scannedSet` | explorer Phase 0 `SetScannedSet(allScoredFiles)` | chain_promotion PendingRead 过滤 + preComplete 分段渲染 + runForcedReads advisory warning |
| `citedRefs` | `emit_answer_document.RecordCitation` | I4 `CitedRefsHash` 第 4 维 fingerprint |
| `pendingReads` | chain_promotion + A1 bridge（grounder → PendingReads 镜像） | I3 check + runForcedReads |
| `unverifiedFinds` | analyzer `findings_validator.Validate` | context/builder "## Unverified Analyzer Findings" + preComplete check + `AddRepair(RepairExpandSearch)` |
| `subjectMatches` | `rankChainsBySubject.SetSubjectMatch` | preComplete SubjectMatch<0.4 check + Subject Match Summary prompt 段 |
| `fingerprints` | `detectStallAndAct.Append` | detectStallAndAct 4 维比较 |
| `repairs` | 5 种 Producer（见 §8 CGEC） | `ConsumeRepairs → renderWindowHint` |
| `stats` | 各 enforcer `Bump` | `emitCGECSummary` 任务末尾一行摘要 |

---

## 6. 请求生命周期

**读模式（`Mode == ModeRead`）**：

```mermaid
sequenceDiagram
    participant User
    participant Orch as Orchestrator
    participant A as analyzer
    participant E as explorer (Turn A)
    participant X as extractor (Turn B)
    participant F as finalizer
    participant Tool
    participant LLM

    User->>Orch: request
    Note over Orch: init BusContext<br/>Mutable=NewMutableState(Objective=request)

    rect rgb(245,245,245)
    Note over Orch: Phase 1 — analyze (一次)
    Orch->>A: dispatchStage(analyze)
    A->>LLM: emit_analysis 指令
    LLM->>Tool: emit_analysis(v4 RequestModel)
    A->>A: buildAnalysisIR (normalizer→reconcile→compiler→risk→hdp+priority+binder→counterfactual→gate→findings_validator)
    A-->>Orch: StageOutput.AnalysisIR
    end

    rect rgb(240,248,255)
    Note over Orch: Phase 2 — per-task loop
    loop 每个 pending task
        Orch->>Orch: reset Turn A/B buffers + Signals
        loop runTaskGraph rounds
            Orch->>Orch: readyExplorerWindow → window + CGEC I4 pre-dispatch forced reads
            Orch->>E: dispatchStage(explore) w/ window hint
            E->>Tool: grep/read_file/repo_map/...
            E->>Tool: emit_evidence per file
            E-->>Orch: StageOutput (EvidenceItems, TurnAArtifacts)
            Note over Orch: runAutoVerdicts + drainHypothesisVerdicts → MarkHypothesis
            alt finalize ready
                Orch->>X: dispatchStage(extract)
                X->>Tool: emit_answer_symbol + emit_hypothesis_verdict
                X-->>Orch: StageOutput (AnswerSymbols + verdicts)
                Orch->>Orch: drainHypothesisVerdicts → MarkHypothesis
                Orch->>F: dispatchStage(finalize)
                F->>Tool: emit_answer_document (同步调 ground.GroundCitation)
                F-->>Orch: StageOutput.FinalAnswer
                Orch->>Orch: contract.Check
                alt pass
                    Orch->>Orch: Mutable.UpdateTaskResult(DONE)
                else fail & budget left
                    Orch->>Orch: requeue finalize + upstream evidence (via EdgeValidationFeedback)<br/>+ inject retry hint
                else fail & budget exhausted
                    Orch->>Orch: fail-loud prepend warning → DONE
                end
            end
        end
    end
    end

    Orch-->>User: BusContext(每个 task 自带 Result)
```

**写模式（`Mode == ModeApply`，单次完整 plan → apply → verify 生命周期）**：

```mermaid
sequenceDiagram
    participant User
    participant Orch as Orchestrator
    participant A as analyzer
    participant P as planner
    participant C as coder
    participant V as verifier
    participant WT as worktree
    participant Tool
    participant LLM

    User->>Orch: --mode=apply + --plan-file (已有 plan) 或 --mode=plan 后 /approve
    Note over Orch: writeGate：write_enabled=true？否则 fail-loud

    rect rgb(245,245,245)
    Note over Orch: analyze（分类器复用；仅读模式字段）
    Orch->>A: dispatchStage(analyze)
    A-->>Orch: AnalysisIR（分类信号）
    end

    opt PlanPath 未设
    rect rgb(255,250,235)
    Note over Orch: runPlanPhase
    Orch->>P: dispatchStage(plan)
    loop 最多 6 iter
        P->>Tool: read_file / grep / repo_map / ...
        P->>LLM: 起草 ChangePlan
        P->>Tool: emit_change_plan
        Tool->>Tool: 5 步校验 + 每个 kind=patch 跑 CheckUnifiedDiff (git apply --check --recount → patch(1) fallback)
        alt 校验通过
            Tool->>Orch: Mutable.SetChangePlan
        else 校验失败
            Tool-->>P: 失败 Summary（含 conflictContextSnippet 真实文件片段）
        end
    end
    P-->>Orch: ChangePlan 就位
    end
    end

    rect rgb(235,250,255)
    Note over Orch: runApplyPhase
    Orch->>WT: worktree.Create(MainRepoRoot, traceID, baseDir)
    WT-->>Orch: sess.Path()
    Orch->>Orch: RepoRoot 临时切到 worktree
    opt pipeline_baseline_capture_enabled
        Orch->>Tool: RunTests{}.Execute（pre-apply）
        Tool-->>Orch: BaselineReport → Mutable.SetBaselineReport
    end
    Orch->>C: dispatchStage(apply)
    loop len(TargetPaths)+3 iter cap
        C->>Tool: apply_patch({path, kind})
        Tool->>Tool: W1 / W1b / kind match 校验
        alt kind=patch
            Tool->>Tool: runUnifiedDiff(RepoRoot, unit.Patch)：git apply --recount，失败回退 patch -p1
        else kind=create/modify/delete
            Tool->>Tool: os.WriteFile(unit.NewContent) / os.Remove
        end
        Tool-->>C: Success=true → WriteClosure.MarkApplied(path)；或失败 Summary（含冲突点文件片段 / valid-path 枚举）
    end
    C-->>Orch: AppliedSet ⊇ TargetPaths？
    end

    rect rgb(240,255,240)
    Note over Orch: runVerifyPhase
    Orch->>V: dispatchStage(verify)
    V->>Tool: run_tests（9 runner 自动探测；build 失败合成 build suite 单条结果）
    Tool-->>Orch: Mutable.SetChangeReport
    opt 可选 narrative
        V->>Tool: emit_test_results（FailureSummary；不能覆盖 Passed）
    end
    V-->>Orch: Passed=true ？
    alt Passed=true
        Orch->>Orch: persistPlanStatus(applied)
        alt pipeline_keep_worktree_on_success=true
            Note over Orch,WT: 保留 worktree；plan.WorktreePath 写盘
        else 默认
            Orch->>WT: worktree.DiscardByPath
        end
    else Passed=false
        alt pipeline_max_verify_retries > attempts
            Orch->>Orch: prepareVerifyRetry：buildRetryHint → Mutable.SetPlanningHint；reset ChangePlan/ChangeReport/WriteClosure；discard 当前 worktree；回到 runPlanPhase
        else 耗尽
            Orch->>Orch: persistPlanStatus(verify_failed)
            Orch->>WT: worktree.DiscardByPath（失败路径无条件）
        end
    end
    end

    Orch-->>User: BusContext（PlanStatus + 可选 WorktreePath）
```

---

## 7. 分析器后处理管线

`analyzer.buildAnalysisIR`（`internal/agent/analyzer.go`）在 ReAct 循环结束后**确定性**地跑以下步骤：

| 步骤 | 包 / 文件 | 作用 |
|------|----|------|
| 1. Sub-topic post-processing | `analyzer.go` 内联 | `SubTopics > 1` → 强制 `ShapeExplanation` + complexity simple → moderate 升级 |
| 2a. Complexity sanity | `analyzer_complexity.go::reconcileComplexity` | 5 条 deterministic rule（sub-topic ≥3 floor / cross-component cue / lookup-shape downgrade / multi-entity upgrade / sparse-prompt floor），强信号反对才覆写 |
| 2b. Intent sanity | `analyzer_intent.go::reconcileIntent` | 首句 count-verb prefix + `IntentEnumerate + ComplexitySimple` → 降级 `IntentReturnValue` |
| 2c. AnswerSubject inference | `analyzer_intent.go::inferAnswerSubject` | LLM 未填时用双语 cue 表 + question_kind fallback + **E1 hard-fallback** 永不 SubjectUnknown（weakest SubjectGeneric, confidence=0.1） |
| 2d. PredicateAxis reconcile | `analyzer_predicate.go::reconcilePredicateAxis` | verb-cue 表填空 `PredicateAxis`；零值 `AxisUnknown` 是 no-op；只填空不覆盖 |
| 2e. Sub-topic entity merge | `analyzer.go` | 把每个 sub-topic 的 entities 合并到主 `AnalyzerHints.Entities` |
| 3. Normalize | `internal/analysis/normalizer::Normalize` | 产 `TermGraph`（canonical terms + aliases），用 repomap-backed `SymbolResolver` + LLM Entities；`kindEnWord` 双门升级 `TermSymbol`：`NormalizeCodeKey(surface) ∈ Entities` ∧ resolver ≥1 hit |
| 4. Observation reconcile | `analyzer_observability.go::reconcileFromObservations` | round 2 grep 观察重新定型 classification，refined `answer_subject.kind` + `AnalyzerHints.Shape` 进后续管线 |
| 5. Auto-keywords | `analyzer.go::appendDeclarativeKeywords` | registration / config_mapping / call_chain / source-literal subject 问题追加 declarative filename stems（topology/defaults/registry/routes/wire/init/manifest/schema/enum） |
| 6. Infer scenario | `internal/analysis/compiler::InferScenario` | LLM 未指定时填 Scenario 默认 |
| 7. Compile | `internal/analysis/compiler::Compile` | 产 `TaskGraph` + `EvidencePlan` + 默认 `AnswerContract`；`sourcemix.FromTemplateMix` → `NodeBudgetHints` |
| 8. Risk evaluate + HDP | `internal/analysis/risk` + `hdp` + `priority` | `RiskMatrix`（六维 0-5 打分）+ `[]Hypothesis`（IntentMatch 0.35 / RiskElevation 0.30 / TermCardinality 0.20 / AmbiguityResolution 0.15） |
| 9. Recompute budget | `compiler.RecomputeBudget` | 用真实 hypothesis count 重算 |
| 10. Bind hypotheses | `internal/analysis/binder::BindByRelevance` | hypothesis ↔ node 相关性绑定 |
| 11. Counterfactual | `internal/analysis/counterfactual::Expand` | complex + ambiguous explain/root_cause 触发；新分支再 `BindByRelevance` 一遍 |
| 12. Measurement-scalar carve-out | `analyzer.go` 内联 | `isMeasurementScalarRequest` flag 驱动：shape→`ShapeValue` + 剥 3 层 citation gate（`CitationReq` / `AcceptanceTests` / 每个 `TaskNode.SuccessCriteria`） |
| 13. Shape reconcile | `analyzer_intent.go::reconcileShape` | subject 是源码 literal（skill_name / agent_name / function_name / type_name / handler_route / return_value）时 `ShapeConfigValue → ShapeValue`；runs after measurement-scalar 所以两规则互不干扰 |
| 14. RequiredFiles | `analyzer.go::analyzerRequiredFiles` | 用 repomap graph 查 `AnalyzerHints.Entities` 的定义文件写进 `EvidencePlan.RequiredFiles`，explorer keyword-search 会 merge 这份列表 |
| 15. Quality gate | `internal/analysis/gate::Run` | 7 check hard/soft 分级（见 §4.1） |

**不变量**：每步都可在零输入下退化到安全默认值。LLM 完全没调 `emit_analysis` 时，analyzer 返回 hard `StageOutput.Error`；`runAnalyzePhase` 重试至 `MaxRetriesPerStage` 次，预算耗尽后 Run 终止。

### Measurement-scalar carve-out — 三层 citation gate 剥离

系统共有 **3 个独立的 citation gate 表面**，都共用 `types.CritCitationCountGE` Kind 但从三条不同代码路径读。测量 scalar 类问题（"how many X" → `find | wc -l`）答案天然无 file:line 可 cite，任何一条 gate 还在 enabled 都会循环 retry。

| # | 数据位置 | 读它的代码 | 错误消息 |
|---|---|---|---|
| 1 | `AnswerContract.CitationReq.Required + MinCitations` | `internal/analysis/contract/checker.go::checkCitations` | `"0 citations provided, N required"` |
| 2 | `AnswerContract.AcceptanceTests` | `contract/checker.go::checkAcceptance` | `"only 0 citations, need ≥N"` |
| 3 | `TaskNode.SuccessCriteria`（finalize 节点） | `scheduler.go::markSuccessCriteriaFailed` → `criterion.Eval` | `"finalize success_criterion citation_count_ge N"` |

`isMeasurementScalar` 信号（由 `isMeasurementScalarRequest` 在 `analyzer_intent.go` 计算）：`complexity == simple + intent ∈ {enumerate, return_value} + 首句命中 count-verb`。flag 为 true 时步骤 12 在一个 block 里应用 5 个后果：

1. `out.AnswerContract.RequiredAnswerShape = ShapeValue`（覆盖 compile 结果和 hint override）
2. `rm.AnalyzerHints.Shape → ShapeValue`（保持 `irAnswerShape` 下游读取一致）
3. `CitationReq.Required = false` + `MinCitations = 0`（gate #1）
4. `AcceptanceTests = dropCitationCountGE(...)`（gate #2）
5. 每个 TaskNode `SuccessCriteria = dropCitationCountGE(...)`（gate #3）

"one signal, one response" 契约让 `grep isMeasurementScalar` 一次能看到所有因果链。全仓 `CitationReq.Required = false` 唯一生产点是 `analyzer.go` 的这个 block。

### Turn A → Turn B 的第 4 道 gate —— Raw Tool Outputs 渲染

`emit_evidence` schema 物理无法承载命令级 scalar（必须带 `source + line_start + anchor_kind`），一个 `find ... | wc -l` 的输出 `73396 total` 无 source file / line / anchor kind 可填。`internal/context/builder.go` 在 Turn B user prompt 里加 "Raw Tool Outputs from Turn A" 段直接渲染 `TurnAArtifacts.ToolResults` 里成功的 tool 输出。`shouldRenderRawToolOutputs` gate：

| # | 条件 |
|---|------|
| 1 | `Stage ∈ {extract, finalize}` |
| 2-3 | `Mutable != nil && AnalysisIR != nil` |
| 4 | `AnswerContract.RequiredAnswerShape == ShapeValue` |
| 5 | `AnswerContract.CitationReq.Required == false`（与 measurement-scalar carve-out 共用设点） |

渲染：`head + (trim marker) + tail` —— **tail 永远保留**（shell 工具把 summarising scalar 放在输出末尾）。per-call head/tail 800/400 字节、total cap 4000 字节。skip list：所有 `emit_*` + `propose_sub_agents` + `repo_map`。开头 preamble 明确告诉 LLM："这些 tool output **不是** citations[] 的入口 —— 对 measurement scalar 用 `value{literal, citation_ref:-1}`"。

---

## 8. 关键设计模式

### 调查与结构化分离（Turn A / Turn B 双 Agent）

探索阶段混合两种本质不同的活动：**调查**（读文件、收集事实）和**结构化**（组织成机器可消费的答案 slate / hypothesis verdict）。两种活动对 LLM 的上下文预算、工具访问权限和 prompt 压力完全不同。

#### ERM vs Extractor — 职责边界

ERM（Evidence Requirement Model）和 extractor 是**同一条流水线上两个不重叠阶段**：

|  | **ERM**（Turn A 内） | **Extractor**（Turn B） |
|---|---|---|
| **文件** | `internal/agent/explorer_erm.go` | `internal/agent/extractor.go` |
| **关心的问题** | "LLM 还需读哪些文件才能回答？什么时候可以停？" | "Turn A 收集的证据里，哪些是真正的答案？列完了吗？" |
| **输入** | AnalysisIR + 运行中累积的 notes/evidence | Turn A 冻结后的完整 `TurnAArtifacts` 快照 |
| **产出** | 下一步读文件建议（`ermSuggestFiles`）+ 停止信号（`ermAllSatisfied`）+ β 基线（`terminalEvidenceCount`） | `AnswerSymbols[]` + `CompletenessClaim` + `HypothesisVerdicts[]` |
| **工具权限** | 完整：`read_file` / `grep` / `repo_map` / `emit_evidence` | 严格受限：只 `emit_answer_symbol` / `emit_hypothesis_verdict` |
| **LLM 调用次数** | 每轮一次（ReAct loop，可能 3~10 次） | **一次**（`ShouldStop: iteration >= 2`） |
| **运行模式** | 确定性规则（纯 Go，LLM 不参与） | LLM 主导 + 确定性验证兜底（`validateCompletenessClaim`） |

#### 强约束（Invariants）

1. **Turn B 禁止文件 IO。** `extract-skill.ToolSuggestions` 只开放 `emit_answer_symbol` / `emit_hypothesis_verdict`，`buildToolSchemas` 依赖此 allowlist 物理裁剪 LLM schema。
2. **Turn A 禁止答案面板。** `StageOutput.AnswerSymbols` 和 `AnswerSymbolCompleteness` 在 Turn A 的 ParseOutput 显式置零。
3. **Analyzer 是 AnalysisIR 的唯一 writer。** 其他 stage 只能通过 `MarkHypothesis` 修改 `HypothesisSet[*].Status`。`applyStageOutput` 只在首次非 nil 时赋值，后续 analyze re-dispatch 不会覆盖 IR。
4. **Turn A 的 StageReport 必须是确定性渲染。** `renderExplorerStageReport` 从 typed `[]EvidenceItem` / `[]FlowFindingDigest` / `[]AnswerChain` 产 canonical markdown，不读 LLM 最后一条消息。
5. **Completeness claim 必须经过 cardinality validator。** `validateCompletenessClaim` 用 `max(β=TerminalEvidenceCount, γ=len(MustInclude))` 交叉验证；slate 不足就降级为 `lower_bound`。
6. **`extract-skill.Prohibitions` 禁止 Turn B 调用 `emit_evidence`。**

### 结构化数据贯穿全架构，prose 仅在 LLM 边界

代码层所有层间数据流都是 Go struct，字符串只在两处合法出现 —— LLM 的 prompt 渲染（struct → markdown）和 LLM 回答的重新结构化（tool call → struct）。

| boundary | 数据类型 |
|---|---|
| Orchestrator → Agent | `*AgentContext` + `*skill.Config`（struct） |
| Agent → Tool（请求） | `json.RawMessage` params，受每工具 JSON schema 约束 |
| Agent → LLM（schema） | `[]llm.ToolSchema{Name, Description, Parameters}` |
| LLM → Agent（tool call） | `llm.ToolCall{ID, Name, Arguments json.RawMessage}` → schema decode |
| StageOutput → BusContext | struct 直拷（`applyStageOutput`） |
| Analyzer → 流水线 | `*AnalysisIR`（深度 typed tree） |
| Turn A → Turn B | `*TurnAArtifacts`（struct 快照 + `[]EvidenceItem` 严格子集） |
| 确定性 chain 排序 | `[]AnswerChain{Item EvidenceItem, Score, StrictOK}` 从 `identifyAnswerChains` 产 |
| Extractor → Finalizer | `[]AnswerSymbol` + `CompletenessClaim` + `[]HypothesisVerdict`（`MutableState` 缓冲区） |
| Finalizer → Renderer | `*AnswerDocument`（typed Summary/Steps/Symbols/Value/Boolean/Citations） |
| Tool → MutableState（emit_* 侧信道） | 每工具有专属 typed setter |

**LLM 边界 — 唯一合法的 flatten 点**：

```
                 <flatten>                              <re-structure>
typed data ───────────────> Markdown prompt ───> LLM ──────────────────> typed tool call → typed fields
          context/builder                            emit_* schema decoder
```

#### 强约束

1. **LLM 的 assistant content 不得 drive 下游代码分支。** 任何 `if strings.Contains(lastAssistantMsg, "...")` 都是反模式。
2. **跨 stage 数据必须走 StageOutput 的 typed 字段。** 不允许 "Agent A 在 assistant content 里埋约定文本，Agent B 解析" 这种 out-of-band channel。
3. **新增数据流必须先加 struct 字段，不许走 `map[string]any` / `json.RawMessage` 逃生舱。**
4. **Tool schema 是强制的。** 新 tool 必须定义 JSON schema，`params` 必须能 unmarshal 到 struct，失败即拒。
5. **确定性渲染优先于 LLM prose。**

### CGEC — Citation-Grounded Evidence Closure

跨 stage 的**证据闭环契约**。Codrax 有两套并行证据系统，作用域不一致：

| 系统 | 数据 | 作用域 | 谁强制 |
|---|---|---|---|
| LLM-driven | `EvidenceItem` (emit_evidence) | `ReadSet` = LLM 真正 `read_file` 过的文件 | grounder **强制** citation ⊆ ReadSet |
| Deterministic | Resolution Chain / Concrete Value / type hierarchy | `ScannedSet` = keyword-scored + LLM-read 的并集（≫ ReadSet）| prompt **命令** "primary basis, do NOT contradict" |

致命矛盾区 = `ScannedSet \ ReadSet`。chain 落进这里时 prompt 强制使用 + grounder 强制拒绝 = 死结。CGEC 用 **4 条不变量 + 5 种 RepairKind + 9 个 enforcer 入口**堵死：

| # | 不变量 | Enforcer 位置 | 违反后做什么 |
|---|---|---|---|
| **I1** | 所有 prompt 中 surface 的 `file:line` ⊆ ReadSet | explorer `applyChainPromotion`、Type Hierarchy filter、`findings_validator` | chain 锚点 ∉ ReadSet → 不渲染 prompt + 锚点 file 进 PendingReads；analyzer 幻觉 path/symbol → UnverifiedFindings + prompt "## Unverified Analyzer Findings" 段渲染 `~~text~~ ⚠️[未验证]` |
| **I2** | 所有 emit_*-接受的 citation ⊆ ReadSet | `emit_answer_document.go` 的 whitelist check + **pre-finalize dry-run**（`simulateCitationGrounding`）| dry-run 在调真 grounder 前先预检，全 miss 时直接 reject tool call；grounder 本身 drop citation + `RepairDirective{Kind: RepairReadFile, ...}` 进 Repairs 队列 + **A1 bridge** 同步镜像到 PendingReads |
| **I3** | `emit_investigation_complete` ⇒ 模拟 `contract.Check` 能过 | `emit_investigation_complete.go::preCompleteContractCheck`（6 条 check a-f） | (a) PendingReads 非空 downgrade + 按 ScannedSet 分段渲染 Forced Read List vs Suspicious Anchors；(b) citation 预检；(c) MinCit 短缺 → emit `RepairExpandSearch`；(d) subject×shape mismatch → `RepairSwapShape`；(e) evidence.Source 落在 unverifiedFinds.Path → downgrade + `RepairExpandSearch`；(f) 所有 chain SubjectMatch<0.4 → `RepairRebindSubject` |
| **I4** | retry 间至少 (ReadSet, Evidence, ChainTerm, CitedRefs) 四维之一单调进步 | `cgec_enforcers.go::detectStallAndAct + runForcedReads`；调用点在 `orchestrator.runTaskGraph` 的 pre-dispatch + post-dispatch 两处 | soft stall（默认 2）→ `runForcedReads`（Lazy Auto-Read，框架代读 ≤N 个 PendingReads，单轮上限 `cgec_forced_reads_per_round=3`）+ emit `RepairExpandSearch`；hard stall（默认 3）→ `SetInvestigationComplete` + `RepairForceCompleteDowngrade` |

#### 5 种 RepairKind

每种都有 ≥1 producer + 1 consumer（`TestAllRepairKindsHaveProducer` 锁死）：

| Kind | Producer | Consumer |
|---|---|---|
| `RepairReadFile` | grounder (I2) + chain_promotion (I1) + pre-finalize dry-run | `renderWindowHint` "## Forced Read List" + runForcedReads Lazy Auto-Read |
| `RepairExpandSearch` | explorer Phase 0 broaden 耗尽 + stall 时 ReadSet 饱和 + preComplete 低 MinCit + findings_validator unverified 符号 | `renderWindowHint` "## Search Coverage Gap" |
| `RepairSwapShape` | `emit_answer_document` shape mismatch + preComplete subject/shape mismatch | `renderWindowHint` "## Shape Reconcile"（retry hint 跨 explore→extract→finalize 持久） |
| `RepairRebindSubject` | `rankChainsBySubject.bestMatch<0.4` + preComplete SubjectMatch<0.4 | `renderWindowHint` "## Subject Constraint" |
| `RepairForceCompleteDowngrade` | `detectStallAndAct` hard stall | `renderWindowHint` "## Force-Complete Downgrade" + `SetInvestigationComplete` |

#### 辅助层

- **AnswerSubject taxonomy**（`internal/types/analysis_ir.go::AnswerSubjectKind` + `internal/analysis/subject/taxonomy.go` 的 per-kind judge）
- **`inferAnswerSubject`**：双语 cue 表 + question_kind fallback + E1 hard-fallback 永不 SubjectUnknown（weakest: SubjectGeneric, confidence=0.1），保证下游 `reconcileShape` / `rankChainsBySubject` / preComplete 的 subject 检查不因 Unknown 退化为死代码
- **`reconcileShape`**：subject 是源码 literal 时 `ShapeConfigValue → ShapeValue` 自动交换
- **`rankChainsBySubject`**：chain 终端 token 按 `subject.Score` 重排 + Subject Match Summary 渲染在 extractor/finalizer prompt

#### RetryHint 跨 stage 持久

`orchestrator.applyStageOutput` 只在 `output.RetryHint != ""` 时覆盖 `TaskState.RetryHint`，让 explorer 写入的 Shape Reconcile / Forced Read List / Subject Constraint 段贯穿 explore → extract → finalize 一个 retry 回合，下一轮 `applyWindowHint` 再重置。

所有 enforcer fire log 统一前缀 `[CGEC] <I|E|A|B|C|D|E|F|G> <event>`（9 处入口），一次 grep 看全。

#### 配置

`codrax.yaml` 的 `cgec_*` 系列：`cgec_forced_reads_per_round`（默认 3）/ `cgec_stall_threshold_soft`（2）/ `cgec_stall_threshold_hard`（3）/ `cgec_phase1_unread_top_k`（5）/ `cgec_phase1_unread_min_unread`（2）。I1-I3 永远 on，I4 可调。三个核心旋钮接受 `0` = 禁用，负值忽略。`agent_investigation_complete_policy: override` 让 `preCompleteContractCheck` 整体跳过。

### Merged-window DAG schedule

Analyzer 产出的 TaskGraph 理论上允许 node 级并发调度，但 `runTaskGraph` 把每一轮所有 ready 的非 finalize 节点合并成**一次** explorer dispatch。`readyExplorerWindow` 用 `criterion.Eval` 求值 `EntryConditions` 决定节点就绪；contract check fail 时 `requeueValidationTargets` 沿 `EdgeValidationFeedback` 边只 requeue 特定上游 evidence 节点（非整个 window）。

### 诚实失败（fail-loud）

contract check 反复失败、retry budget 耗尽时，编排器不会**丢弃**最后一次 finalizer 的原始答案 —— 而是在它前面 prepend 一条警告告诉用户答案未通过契约。用户至少能看到模型实际想说什么，再自行判断。

### Prior Conversation intent-aware 可见性

REPL `Store.BuildContext` 从 memory 拉上下文，把最近几轮 Q/A 拼进 `Objective` 前缀（`## Prior conversation\n...\n\n## Current request\n...`）。`AgentSettings.PriorConvPolicy` 四值决定 per-stage 可见性：

| 值 | stage 可见性 | 场景 |
|---|---|---|
| `always` | 全部可见 | 历史行为 opt-out |
| `analyzer` **(默认)** | 仅 analyzer 可见 | analyzer 做实体消歧需要 Prior；下游 stage 靠 `AnalysisIR.entities` 拿已消歧的 identifier |
| `continue` | analyzer 始终 + 下游看 `IsContinuation(current, prior)` | 连续追问体验：首字符命中 `再/继续/那/more on/elaborate/...` 或首 40 字符是裸代词且不含 CamelCase/snake_case identifier 判定为追问 |
| `never` | 全部不可见 | 极端隔离，stress test |

**数据流不变量**：
- `Objective` 始终携带完整 "Prior + Current" payload —— `StripConversationPrefix` / `SplitConversation` 继续工作。policy 只门控渲染，不门控存在。
- `AgentContext.PriorConvHidden` 反义 bool：零值 = 可见（保证单次 dispatch 的 zero-value 路径沿用老行为）。
- `context/builder.go` 用 `!ac.PriorConvHidden` 门控 "Prior Conversation (reference only)" 段渲染；"User Request" 永远渲染。
- `orchestrator.priorConvVisibleForStage` 一处 resolve；debug-level trace `[orchestrator] prior_conv: stage=X policy=Y visible=bool`。

**关键配套**：`skill/analysis_contract.go` 的 analyzer prompt 要求当 Prior 消歧了某个代词时，analyzer **必须**把消歧后的具体 identifier verbatim 写进 `emit_analysis.entities`——这是 `policy=analyzer` 可行的前提。REPL memory store 不受影响（policy 只改可见性，不触碰持久化）。

### Citation fabrication 三层防御

防止 LLM 伪造 cite 过关的 pool-level 不变量：

| # | 位置 | 代码入口 | 行为 |
|---|---|---|---|
| 1 | `emit_investigation_complete.Execute` | `hasGroundedOrRecovered(evidence)` | Mutable 缓冲区已有 ≥1 条 `GroundingGrounded` / `GroundingRecovered` 时，`absence_justification` 参数**直接 reject**，关掉 finalize citation floor 豁免 |
| 2 | `ground.GroundCitation` Tier 2 | `tier2LineInStructuralRegion(fi, line)` | 要求 line 落在某 symbol 的 `[Line - docRadius, EndLine]`（docRadius=10）或 prologue `[1, firstSymbolLine - docRadius]`。两 symbol 之间的 dead zone 被 reject |
| 3 | `emit_answer_document.buildEmitAnswerDocumentCitations` | Tier 1 peer 规则 | pool 中所有保留的 citation 都是 Tier 2-only（无 `TierLineText`）且至少一条 quote 被清理（fabricated）→ 全部 quote-cleared citation 整批 drop |

**数据流**：`CitationReport.Tier` 字段沿 `GroundCitation` → `buildEmitAnswerDocumentCitations` 传递。empty Tier（单测路径）表示跳过 pool-level rule。

### emit_evidence per-item reject + auto-swap

三条 graceful-degrade 规则：

1. **`line_end < line_start` 自动 swap**：转置笔误，swap 后保留，Summary 附 "AUTO-SWAPPED; double-check the range"。
2. **Sparse per-item reject**（< 50% 失败率）：单条 validation error 跳过该 item 保留多数，reject 的 index + 原因写进 Summary。
3. **Majority reject**（≥ 50% 失败率）：整批失败，所有原因列在错误信息里。

实现在 `buildEmitEvidenceItemWithSwap` 包装 + 主循环的 "collect into `rejectedItems`, continue" 模式。

### 反过拟合设计原则

- LLM-facing prompt 文本遵循**角色优先、格式无关**原则：用**角色**描述文件（类型定义、核心逻辑、配置/规则声明、入口点），不用文件格式（`*.yaml` / `*.go`）
- 用**通用模式**过滤噪音（VCS 目录、依赖目录、测试文件），不用项目特定路径
- OutputFormat 示例使用**混合语言**（Python / Ruby / TypeScript）的文件路径，强化"只学格式，不学语言"
- 不在 prompt 里硬编码任何特定项目的目录结构、工具名或配置格式

---

## 9. 运行时子系统

### `internal/logging`

leveled logger（`error` / `warning` / `info` / `debug`），写到 `logs/codrax-YYYYMMDD-HHMMSS-mmm-<pid>.log`，4 MB rotation + `log_max_files` 文件 retention（默认 7）。每次进程启动开**新的** PID-stamped 文件；retention sweeper 解析文件名里的 PID，owning process 仍存活时跳过删除。PID liveness 检查在 Unix 上用 `syscall.Kill(pid, 0)`，Windows 上用 `OpenProcess` + `GetExitCodeProcess`（`pid_{unix,windows}.go`，`//go:build` 分发）。

Debug-gated `[diag ...]` trace 在 `BaseAgent.Execute` 里 dump 完整的 ReAct 循环（initial prompt、assistant turns、tool results、stop reason），`-log-level debug` 打开。

### `internal/memory`

多 turn REPL store。Recent turns 存内存 + 磁盘上 verbatim 的 `memory/turns/<id>.md`，其中 `<id> = turn-<unix-nano>-<pid>`。Recent buffer 超过 `memory_max_recent_turns`（6）或 `memory_max_recent_bytes`（20KB）时，最老的 turn 被 LLM summarize 成 `{topic, keywords, summary, full_ref}` 条目 append 到 `memory/MEMORY.md`。

**跨进程安全**：
- `MEMORY.md.lock` 是 per-operation flock，shared lock 用于 `loadIndex` / `BuildContext`，exclusive lock 用于 `appendIndexEntry` / `Clear` / `compactOldest`。每次操作 acquire lock 后重新 load `s.index` 保证 peer 写入立即可见。
- `.instance.lock` 是 lifetime shared lock，做 presence detection：`NewStore` 试一次 non-blocking exclusive —— 成功说明唯一 Store，可安全跑 `loadOrphanRecent` 恢复崩溃 session 的 tail；失败说明有 peer，跳过 orphan recovery。
- Turn ID 带 PID 保证两个进程不会在 turn filename 上碰撞。
- Windows 的 `LockFileEx` / `UnlockFileEx` stdlib `syscall` 不导出，`lock_windows.go` 通过 `syscall.NewLazyDLL("kernel32.dll")` 手动调用。

### `internal/repl`

交互式 REPL。逐行读取，用 `Store.BuildContext` 把历史对话 prepend 成 `## Prior conversation\n...\n\n## Current request\n...` 注入请求字符串——零修改 BusContext 或 Agent。Slash command 分两组（每条都支持 `\` 前缀别名，如 `\exit` ≡ `/exit`）：

- **通用**：`/exit` `/quit` `/help` `/version` `/history` `/clear` `/compact` `/log` `/paste` `/chat`
- **写模式**（需 `codrax.yaml :: write_enabled: true`）：`/mode` `/plan` `/approve` `/reject` `/verify` `/worktree`

`slashCommands`（`internal/repl/input.go`）驱动 Tab 补齐面板；`replCommandAliases`（`internal/types/conversation.go`）是 `NormalizeREPLCommandAlias` 唯一的来源，`Loop` 先归一再派发给 `handleSlash`。两个列表的漂移由 `TestSlashCommandsMatchCanonicalRegistry` + `TestHandleSlashDispatchMatchesRegistry` 两个结构测试固化——任一新 `/xxx` 的 `case` 没同步到 `replCommandAliases` 都会在 `go test` 时 fail-loud。

**`/chat <message>`**：绕过 analyze→explore→extract→finalize 流水线，单次 `adapter.Chat` 直接给 LLM 回复。配合 `codrax.yaml :: chitchat_classifier_enabled`（默认 `true`）每轮 REPL 前跑一次廉价 LLM 分类器，判为 chitchat 的轮次自动走此路径。想省成本就把 `chitchat_classifier` 在 `providers.yaml` 路由到小模型；想关就设 `false` 或启动时加 `--chitchat-classifier=false`。失败路径：responder 错 → print warning + 不写 memory（不污染 prior conversation）；classifier 错 → 回落流水线（fail-safe）。

**写模式命令**：见 §4.6。`/mode` 切换粘滞 mode；`/plan show` 渲染 unified-diff 预览（per-change 4 KB、总计 16 KB 上限）；`/approve` 只接受 `Status == pending_approval` 的 plan（前置检查），触发第二次 Run 带 `Mode = ModeApply` + `SetPlanPath`；`/reject [reason]` 把 plan 从 PlanStore 清掉并记入 memory（`memory.KindPlan`）；`/verify [plan-id]` 对 `Status ∈ {applied, verify_failed}` 且有保留 worktree 的 plan 重跑 ModeVerify；`/worktree list / discard <plan-id>` 管理 `pipeline_keep_worktree_on_success` 保留下来的 worktree。

**`/log` 子命令**：`/log <path>` 从文件载入 / `/log`（无参）进入粘贴模式以 `/end` 结束 / `/log clear` 丢弃 / `/log show` 预览前 20 行。attached log **跨 turn sticky**（用户通常同一条 panic 分多个问题问），只有显式 `/log clear` 或覆盖式 `/log <path>` 替换。`/clear`（清 conversation 历史）不动 attached log。

### `internal/tool/blob`

per-process blob 存储。Session dir `<CWD>/.codrax/blob/<timestamp>-<pid>/`，assigned 到 `BusContext.WorkDir`。`PruneBlobSessions` 在启动时按 `blob_max_sessions` 保留最近 N 个，存活 PID 的 peer session 永不删。设 `blob_max_sessions: 0` 回退到 per-trace `os.MkdirTemp` + `RemoveAll`。

### `internal/render`

| 文件 | 职责 |
|------|------|
| `event.go` | Event struct（Kind, Timestamp, TraceID, Agent, ...) + 事件类型（PipelineStart/End, StageDispatch, AgentReasoning, ToolCall, ToolResult, AnalysisReady, TaskNodeStart/End, ...） |
| `renderer.go` | CLI 渲染器（pterm.Area + 实时事件消费） |
| `answerdoc.go` | `AnswerDocument → markdown` 渲染器，shape-aware，多语言（zh/en），code block 语法标记，citation pool 渲染 |

### 响应语言

`-lang`（默认 `zh`）→ `orchestrator.SetLanguage` → append 到 `BusContext.Preferences` → 作 "User Preferences" system 段渲染。始终带 fallback 分句——另一语言提问能用那语言回答。`-lang=off` / `none` 回退。

---

## 10. 配置

两个 YAML 文件平铺在二进制同目录（`<exeDir>/`），严格不重叠：

| 文件 | 内容 | 加载器 |
|------|------|--------|
| `providers.yaml` | LLM provider credentials + per-agent model routing。Secrets，不提交 | `internal/config/providers.go` |
| `codrax.yaml` | per-process 运行时 knob：log/memory 路径、语言、repo/branch、blob 尺寸 + session 保留、pipeline 预算等 | `internal/config/runtime.go` |

**路径锚点分两层**：
- 配置锚点 `<exeDir>` —— `providers_config` 在这里解析
- 运行产物锚点 `<CWD>/.codrax/` —— `log_dir` / `memory_dir` / `cache_dir` / blob 会话根在这里解析

### `codrax.yaml` 分组

所有字段指针类型以让 merge 区分 "absent" vs "explicit zero value"：

| 前缀 | 用途 |
|------|------|
| 裸 key | `log_dir` / `log_level` / `log_stdout` / `memory_dir` / `cache_dir` / `lang` / `repo` / `branch` / `providers_config` — 进程级 UX |
| `log_*` | `log_max_files` — 日志保留条数 |
| `blob_*` | `blob_max_inline_bytes` / `blob_preview_head_bytes` / `blob_preview_tail_bytes` / `blob_max_sessions` — Tool 输出 offload 阈值 + 会话目录保留数。**`blob_max_inline_fraction`** 是占比形式（`fraction × context_window × 4 B/token` → 有效字节阈值），fraction 设置时优先于 bytes 绝对值 |
| `readfile_*` | `readfile_small_limit_threshold` — read_file 懒惰 limit 保护 |
| `analysis_*` | `analysis_warn_below_keywords` / `analysis_reject_below_keywords` / `analysis_generic_entity_blocklist` / `analysis_reject_multiple_emit` / `analysis_max_prescan_rounds` / `analysis_warn_below_keyword_hit_ratio` / `analysis_warn_below_entity_hit_ratio` — emit_analysis 运行时验证 |
| `evidence_*` | `evidence_grounding_floor` / `evidence_tier1_floor` — explorer completion gate |
| `pipeline_*` | `pipeline_max_steps` / `pipeline_max_retries_per_stage` / `pipeline_max_stage_visits` — 流水线预算；`pipeline_max_verify_retries`（写模式 verify→plan 重试上限，默认 0 / 硬上限 5）/ `pipeline_baseline_capture_enabled`（写模式 apply 前测试快照）/ `pipeline_keep_worktree_on_success`（写模式成功后保留 worktree）|
| `write_enabled` | 顶层写模式开关。`false`（默认）时 `--mode=plan|apply|verify` 和 REPL `/mode plan|apply|verify` 全部 fail-loud 拒绝 |
| `write_default_mode` | `--mode` 不传时的默认值。合法值：`read`（默认）/ `plan`。`apply` / `verify` 在这里被**拒绝**（必须 per-run opt-in） |
| `write_auto_approval` | 预留给 REPL `/approve` 的交互默认值和 batch 工作流。单次调用不读此字段（直接看 `--auto-apply` CLI flag） |
| `write_plan_dir` | 覆盖 `.codrax/plans/` 作为 plan JSON 输出目录。绝对路径或 runtime-anchor 相对路径；非绝对形式由 cmd/root.go 锚定 |
| `gate_*` | `gate_coverage_min` / `gate_coverage_weight_{symbol,config,concept}` / `gate_hypothesis_min_priority` — analyzer 质量门阈值 |
| `explore_*` | `explore_per_tool_default_cap` + 15 个 `ExploreHeuristics` 阈值（mid-loop / soft-stop / Phase 0 / enumeration / parallelize） |
| `agent_*` | `agent_max_iterations` / `agent_max_tool_history_bytes` / **`agent_max_tool_history_fraction`**（fraction × context_window × 4 B/token；同 `blob_max_inline_fraction` 的优先级规则） / 4 个 `agent_loop_*` / 3 个 `agent_finalizer_shrinkage_*` + `agent_finalizer_max_correction_retries` + `agent_finalizer_preserve_prior_prose` / `agent_extractor_max_correction_retries` / 4 个 `agent_subtopic_*` / `agent_investigation_complete_policy` / `agent_prior_conversation_policy` / **`agent_context_pressure_soft_ratio`** / **`agent_context_pressure_hard_ratio`**（context-pressure 监控软/硬阈值，默认 0.7 / 0.9）|
| `memory_*` | 6 key — REPL 多轮记忆存储限制 |
| `summary_cap_*` | `summary_cap_enabled`（master switch，默认 false）+ 11 个 per-shape cap — Summary 长度上限 |
| `citation_quote_max_chars` | citation quote 预览字符上限（默认 500，UTF-8 边界静默截断）。file+line 始终保留；prose 防御由 grounder token 匹配兜底，跟长度无关 |
| `cgec_*` | `cgec_forced_reads_per_round` / `cgec_stall_threshold_soft` / `cgec_stall_threshold_hard` / `cgec_phase1_unread_top_k` / `cgec_phase1_unread_min_unread` — Citation-Grounded Evidence Closure 调节 |

### 优先级（precedence）

| key 组 | 优先级（低 → 高） |
|--------|------------------|
| 裸 key | code default → `codrax.yaml` → CLI flag |
| `pipeline_*` | code default → `codrax.yaml` → CLI flag（`--pipeline-max-steps` / `--pipeline-max-retries` / `--pipeline-max-stage-visits`）。`pipeline_max_verify_retries` / `pipeline_baseline_capture_enabled` / `pipeline_keep_worktree_on_success` 仅 yaml，无 CLI override |
| `write_enabled` | 仅 yaml（部署时决策，不按 invocation）。`--mode` 是 CLI flag，但在 `write_enabled: false` 时拒绝;REPL `/mode` 同 gate |
| 其他所有 `*_` 组 | code default → `codrax.yaml`。**无 CLI override** |

### Path anchoring

`cmd/root.go` 两套 anchor：

- **`configAnchor = <exeDir>`**：`providers_config` 的相对路径在此解析。安装 = 一份配置树，跟工作目录无关。
- **`runtimeAnchor = <CWD>/.codrax/`**：`log_dir` / `memory_dir` / `cache_dir` / blob session 根在此解析。运行产物跟随用户工作区。

`codrax.yaml` 查找顺序（flat-first，exe 优先）：`$CODRAX_SETTINGS` → `<exeDir>/codrax.yaml` → `<exeDir>/codrax/codrax.yaml` → `<exeDir>/config/codrax.yaml`（*legacy，warn*）→ `<exeDir>/../config/codrax.yaml`（*legacy，warn*）→ `<CWD>/config/codrax.yaml`（*legacy，warn*）。两个 anchor 都在 flag 注册之前解析成绝对路径。`-repo` 不参与 anchoring —— 默认 `.` 永远代表 CWD。

### Per-target-repo namespacing

默认 `log_dir` / `memory_dir` 带一个 `<basename>-<fnv32>` 后缀，derive 自 absolute + symlink-resolved `-repo` 路径，这样多个 target repo 共享一份 codrax 安装时，各自的 log / memory 落在互不相交的子树（`<CWD>/.codrax/logs/foo-a3f9c2b1/` / `<CWD>/.codrax/memory/foo-a3f9c2b1/`）。Slug 在 flag default 里 baked，`-h` 打印最终路径；用户显式覆盖 `-repo` 同时保留 `-log-dir`/`-memory-dir` 默认时，`cmd/root.go` 在 `flag.Parse` 后 re-slug。显式 `-log-dir` / `-memory-dir` 总是胜出。

Blob session 根**不**做 per-repo 分区——一个进程所有 Run 共用 `<CWD>/.codrax/blob/<timestamp>-<pid>/`，因为 blob 文件是 content-addressed（`<tool>-<sha8>.txt`），跨仓库相同输出天然去重。

### Multi-instance safety

- 日志：文件名带 PID，retention 跳过 live-PID
- Memory：`MEMORY.md.lock` 每操作 flock + `.instance.lock` lifetime shared probe
- Turn IDs 带 PID
- Windows 文件锁：`syscall.NewLazyDLL("kernel32.dll")`

### Evidence-lite runtime gate

`BaseAgent.executeTool` → `validateAnalyzerPrescanToolCall` 在 `StageAnalyze` 拒绝 `grep` 没带 `files_only=true` 的调用。硬约束：line-level 匹配会溢出 analyze 的 context 预算。其他 stage 不受影响。

### 依赖

Go 1.24.0。主要依赖：

- `gopkg.in/yaml.v3` — 配置解析
- `github.com/spf13/cobra` — CLI
- `github.com/charmbracelet/glamour` / `huh` / `lipgloss` — TUI / markdown 渲染
- `github.com/pterm/pterm` — 进度显示
- `github.com/smacker/go-tree-sitter` — 代码解析（多语言）
- `golang.org/x/term` — 终端控制

无外部 LLM SDK：codrax 自己实现 `llm.Adapter` 接口以保持 provider 独立。

---

## 11. 可扩展性

### 添加新工具

1. 实现 `Tool` 接口，嵌入 `tool.ReadOnly` 提供 `IsWrite() bool`
2. 在 `cmd/root.go` 的 tool registry 注册
3. 在相关 skill 的 `ToolSuggestions` 里引用

### 添加新 Agent

1. 新增 `AgentName` 枚举常量
2. 实现 `Evaluator` 接口（`BuildInitialInstruction` / `ShouldStop` / `ParseOutput` / `DetermineMissingPiece`），可选实现 `LoopController`
3. 在 agent registry 用 `NewBaseAgent(name, deps, eval)` 包装注册
4. 绑到新阶段时同步更新 `topology.go` 的 `pipelineTopology` map 和 `PipelineStage` 枚举

### 添加新 Skill

1. 定义 `skill.Config`（goal / workflow / toolSuggestions / outputFormat / prohibitions）
2. 在 skill registry 注册
3. bind 到 `pipelineTopology` 某阶段作 default skill

### 添加新 AnalysisIR 节点类型 / Intent / Scenario

1. 新增枚举常量
2. TaskNodeType → `scheduler.stageMapping` 加映射
3. Scenario → `internal/analysis/compiler/templates.go` 补模板
4. Intent → `internal/analysis/compiler/scenario.go::InferScenario` 可能加分支

### 添加新 AnswerShape

1. 新增 `AnswerShape` 常量并加进 `IsEmittable()`
2. `internal/render/answerdoc.go` 的 renderer 加分支
3. `emit_answer_document` tool schema 的 typed payload union 加分支
4. Finalizer `answer_document_evaluator` 根据 shape 决定 prompt 模板

### 添加新 AnswerSubjectKind

1. 新增 `AnswerSubjectKind` 常量
2. `internal/analysis/subject/taxonomy.go` 加 per-kind judge
3. `analyzer_intent.go::inferAnswerSubject` 的 cue 表加 trigger
4. `reconcileShape` 的 shape 映射可能需更新

### 添加新 CGEC RepairKind

1. 新增 `RepairKind` 常量（`internal/types/repair.go`）
2. 加 producer：某 enforcer 构造 `RepairDirective{Kind: ..., ...}` 调 `AddRepair`
3. 加 consumer：`renderWindowHint` 加对应渲染段
4. 更新 `TestAllRepairKindsHaveProducer` + `TestAllRepairKindsHaveConsumer` 断言
