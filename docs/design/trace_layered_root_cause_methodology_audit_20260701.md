# Trace 分层根因下钻方法论 —— 现状审计与优化建议(2026-07-01)

> 本文档是**只读代码审计**的产出,不包含任何代码改动。审计对象是 `internal/tracequery/`(引擎,~11700+1583+2377+634 行)、`internal/tool/trace_query.go`(工具外壳+teaching prompt,~5900 行)、`internal/skill/`(软引导 prompt)、`internal/types/observation_ledger.go` / `trace_causal_projection.go` / `trace_observation_coverage.go`(typed 载体)、`internal/tool/answer_document_mutation_runtime.go`(finalize 期自动注入)、`internal/agent/answer_document_evaluator.go`(finalize 软引导)。审计基线是 `origin/main@abedbc7b`(2026-07-01 09:38,审计过程中仓库仍在演进,§6 记录了审计期间实时落地的修复)。
>
> **v2 更新**:补充审计 on-chain 三种终止状态(Runnable/Running/D-state·IO-wait)各自"下一跳"应识别的具体根因——Runnable 的优先级反转(含鸿蒙/东湖优先级语义)、Running 的算力供给与 perf_sample/代码对照深挖、IO 的聚类 inode 定位。发现原先 §2.3/§4-O3 把这三个状态一概判定为"终止节点、无下一跳"过于笼统:实际上 `buildRootCauseRankFrom` 用了一种**不同于 `expandChain` 图遍历的并行独立候选流 + on-chain 线程集合过滤**模式,已经让算力供给(compute_supply)和聚类 inode(file_io_hot_inode/block_io_by_inode)在依赖线程本身或已被 sleep 链发现的线程上生效;唯独 Runnable 的优先级比较未被同样接入。详见 §2.3.1-§2.3.3(新增)与修订后的 §4-O3。
>
> **v3 更新**:新增 §2.8"数据溯源"——把 §2 每个计算指标逐一回溯到具体的原始 `EventType` 和内核/用户态 tracepoint 名称(状态判定唯一来自 `sched_switch`、唤醒链来自 `sched_wakeup`/`sched_waking`、D-state/IO 原因来自 `sched_blocked_reason`、算力供给来自 `cpu_frequency`/`cpu_idle`、Binder 来自 6 个 `binder_*` 事件、IO 聚类来自 `f2fs_*`/`android_fs_*`/`ext4_*` 等文件系统事件叠加 `block_rq_*` 存储层事件、JIT/VerifyClass/shader 语义 span 来自对通用 `trace_mark` 文本做模式匹配、CPU Profiling 来自转换阶段拼装的合成 `perf_sample` 行,等等)。附带发现两点值得记录的数据侧限制:①语义 span 识别没有独立内核事件兜底,是弱类型文本匹配;②Workqueue/DMA Fence 目前只有计数,没有专属结构化字段提取。
>
> **v4/v5 更新**:响应本文档 §2.6.2/O1 提出的容量短板,仓库代码侧实际做了两轮 `trace_query 关键观测核对`/`TraceCausalProjection` 容量扩容(细节见 §2.6.2 与 §6),文档随之同步了最新的 cap 数值。
>
> **v6 更新**:新增 R8(用户显式时间窗必须严格遵守,不能因 VSYNC/帧边界误缩窗)、R9(帧信息 + 显式时间窗同时给出时应取并集)两条规则及对应审计 §2.9。结论:**R8 已满足**——排查了三处会重新计算时间窗的入口(`resolveSpanWindowsForQuery`/`ResolveFrameTarget`/`FrameWindowAutoDerived` 置位条件),全部以"用户是否已显式给出 time_start/time_end"为精确 typed 开关,没有发现"因检测到帧边界而悄悄收窄显式窗口"的代码路径;但发现一处相关但不同的风险——`interestingIntervals` 会把窗口内目标线程的时间线按状态打分后只取 Top-8 子区间参与递归展开、且无条件跳过 Running 区间,窗口元数据本身没被收窄,但递归下钻的实际覆盖深度可能不足,且无对应 caveat 提示(裁剪是静默的)。R9 在 v6 审计时未实现,后续 v7/v8 已补齐并集逻辑与 explicit-0 回归测试。
>
> **v7 更新(本批实际修复代码,非纯审计)**:先探索既有代码(`priorityRelation`/`dependencyPriorityRelation`/`threadPriorityNear`/`runnableContextForThread`/`unionTimeWindows` 等既有原语)确认可复用后,修复了 O1、O3、O7、O9(剩余的 `resolveSpanWindowsForQuery` 一侧)、O10(caveat-only 部分),每项都补了新单元测试且全仓 `go build ./... && go test ./...` 全绿。**O2 复核后判定不是真缺口并撤销**(详见 §2.2 更新)。逐项:
> 1. **O1**:`boundTraceMarkSpans` 让 `computeTraceMarks` 的语义 span(jit/verify/shader/runtime compile)单独占 16 个名额,不再和普通 span 抢 8 个名额的时长排名(§2.6.3)。
> 2. **O3**:新增 `applyRunnableTopPriorityInversion`,对 `stats.RunnableTop` 直接候选调用既有的 `runnableContextForThread`(取 `SameCPUTopRunning`)+ `threadPriorityNear` + `dependencyPriorityRelation`,命中鸿蒙/东湖语义下的"竞争者优先级更低"时重分类为已被下游识别为 co-primary 的 `priority_inversion_runnable_wait`(§2.3.1)。
> 3. **O7**:新增 `traceSpanNearMissesSemanticWorkClassification`,对形似编译/校验/shader 但未命中任何具体模式的 span 名追加一条有界(≤3 例)命名漂移 caveat,纯 advisory,不影响任何排序/tier(§2.8)。
> 4. **O9 补完**:把 `ResolveFrameTarget` 已有的 `unionFrameTargetWindows` 提升重命名为通用的 `unionTimeWindows`,同款并集逻辑接到 `resolveSpanWindowsForQuery`(`span_name` + 显式窗口同时给出的场景),不再是"两者都给就直接用显式窗口、丢弃 span 自身边界"(§2.9.2)。
> 5. **O10**:`interestingIntervals` 新增返回值 `qualifying`(截断前的候选总数),`buildWakeupChainWithCache` 在 `qualifying > len(branches)` 时追加一条明确的"N 个候选区间未被递归展开"caveat,不改变截断算法本身(§2.9.1)。
> 6. **O2 撤销**:复核发现 `scheduler_latency_stats`/`critical_blocking_calls`(Runnable/D-state/IO-wait 的推荐 view)都是单次有界聚合、不是像 `wakeup_chain` 那样的多跳递归图遍历,只对碎片化 sleep 做非递归例外是正确设计,不是缺口——被现有测试 `TestStateDrilldownRuleMatrixPinsRecentTracePolicies` 验证锁定,未做改动。
>
> O4(投影显式携带请求窗口)、O6(架构文档补章节)判断为下一批处理,详见 §4。
>
> **v8 更新(第二批实际修复,先跑 8-agent 设计+完备性 workflow 再动手)**:动手前用多智能体 workflow 对每个剩余 gap 产出经对抗验证的 diff 级实现规范 + 完备性批判,再实现。本批:
> 1. **v7 O9 回归修复(最高优先级,completeness critic 发现)**:`unionTimeWindows` 用 `out.StartTs <= 0` 当"未设置",把用户显式 `time_start=0`(`TimeStartSet=true` 可达)误当未设置被 span 起点顶替 → 违反 R8。改为纯几何 min(带 `b.StartTs > 0` 防呆),补 `TestSpanWindowExplicitZeroStartNotShrunkByUnion` + `TestUnionTimeWindowsPreservesExplicitZeroStart` 红转绿(§2.9.2)。
> 2. **R3 显著性阈值**:`StateDrilldownStep` 新增 `WindowProportion`(占窗口比例,clamp[0,1])+ `Significant`(top 状态恒真;低 rank 需过 `stateDrilldownSignificantFloor=0.05` 或 `stateDrilldownSignificantTopRatio=0.25`),从 `stats.Window` 推导 windowMs 不改签名;6 处 lockstep 同步(struct/render/banner/typed emitter/ledger 白名单/skill prompt)。R4 不删任何步骤(§2.1)。
> 3. **O4 投影窗口标注**:`TraceCausalProjectionNode` 新增 `StartTs`/`EndTs`(解析 window RichNote)+ `WithinRequestedWindow *bool`(三态);anchor 唯一精确源 = `frame_target_resolution`(`window_source=query_window` 或 R9 union 白名单),交集语义,无 anchor 保持 nil 不臆造;渲染 tag 仅在非 nil 时追加(§2.5)。
> 4. **O5 端到端渲染测试**:`TestApplyAndPersistMutation_LowImpactSemanticSpanSurvivesToRenderedText` 走 observation→projection→`RenderAnswerDocument` 最终 markdown,断言 2ms 语义 span 的"确定性优化点"+span 名存活(同时覆盖 O4 tag 渲染)。
> 5. **O6 架构文档**:`docs/architecture.md` 新增 §7.2.1 trace_query 分层下钻纲要。

## 背景:用户目标方法论(原始需求逐条整理)

本节是对用户在多轮对话中提出的目标方法论的**逐条规则化整理**,是本文档其余部分的审计基准(下文 §2 每个小节标题都标注了对应规则编号)。原始表述是自然语言,以下整理为可逐条对照代码检查的规则,不改变原意。

- **R1(触发条件)**:当用户的丢帧分析请求已经把具体信息固化下来——某一帧、或某一线程的时间窗(即线程 ID + 时间窗已确定)——分析丢帧原因时,必须由主链(on-chain)分析触发真正的根因下钻,而不是停留在浅层分析。
- **R2(第一层·最长状态必分析)**:第一步,对用户关注的线程做一次快速的线程状态查询,找出整个时间窗里各状态(running / runnable / sleep / IO 等)中时长最长的状态,对其进行根因分析;分析结果必须 typed 化,并 handoff 给下游。
- **R3(第二/第三…层·显著才分析)**:时长第二长的状态,只有当它的时长**显著**(占比高)时才需要做根因分析并 typed 化 handoff;不显著则不需要。第三长、第四长……以此类推,直到 Top-N 个状态都按"是否显著"判断完毕。每一个够格的状态都作为"这一层"的主要关注点,单独进行根因分析。
- **R4(碎片化状态聚类·不下钻但要 handoff)**:对每一层(即每个被下钻到的线程),整个窗口内经过**碎片化状态聚类**(频繁切换、单次都不长,但累计起来时长很长)后排在 Top-N 的状态,**不需要**做递归下钻分析(避免下钻范围无限扩散);但这个维度的分析结论仍然要 typed 化 handoff 到最终答案里——"单次持续时长最长的状态"和"频繁切换但聚类累计后最长的状态"是两个不同维度,都不能丢信息。
- **R5(on-chain 关联线程递归下钻,含三个状态的下一跳细则)**:对每一层已识别的根因,要在 on-chain 关联线程上继续递归下钻,每一层采用与该层状态对应的分析方法,递归直到追到真正的根因为止。三类终止状态各自的下一跳要求:
  - **R5a(Runnable 下一跳)**:要能识别出**优先级反转**作为根因,并且要考虑**鸿蒙(HarmonyOS)/东湖(Donghu)场景的优先级语义**(与 Android 等平台不同,不能套用同一套比较规则)。
  - **R5b(Running 下一跳)**:要能识别出**算力供给**等根因,并支持基于 **perf_sample 的更深入分析**,或**与代码进行对照**。
  - **R5c(IO 下一跳)**:要能识别出具体的、经过**聚类的 inode**(如果 trace 里有相关数据)。
- **R6(时间窗内投影汇总 + 最终总结)**:逐层根因分析全部完成后,要把所有结论在用户最初指定的时间窗内做一次汇总投影,再产出一次汇总提炼后的最终总结。
- **R7(JIT/VerifyClass/shader 特殊通道)**:在对 on-chain 每一层做分析时,还要关注该层 on-chain 链路上出现的特殊 span——例如(东湖等)trace 场景里常见的 JIT 编译、VerifyClass(类校验)、shader 编译。这些 span 即便占比不高,也必须走一条**独立的特殊通道**处理;在不影响、不干扰其它通用根因分析逻辑的前提下,把它们作为"另一面"信息**强制** handoff 到最终答案里提及——因为这些是确定性的、可直接优化的点,需要提醒用户去解决。
- **R8(严格遵守用户显式时间窗,禁止误缩窗)**:当用户在 trace 分析请求里已经明确指明了某一帧要分析的起始和结束时间窗时,即便窗口中间出现了 VSYNC 信号或其它帧/状态边界,也**不能**因此把分析窗口错误地缩小——线程 ID 和时间窗是用户最关心、已经显式给定的约束,必须严格遵守。只有当用户**只提供了局部信息**(例如只给了帧标识/pattern,没有给出明确的起止时间)时,才允许模型自己去推导时间窗。
- **R9(帧信息 + 显式时间窗同时给出时,取并集而非二选一)**:如果用户既指定了某一帧,又同时给出了显式时间窗,不能只用其中一个而丢掉另一个覆盖的范围——应该把"模型从帧信息推导出的时间窗"和"用户显式给出的时间窗"取**并集**(合并成覆盖范围更大的窗口)进行分析,确保两边各自暗示的时间范围都不被遗漏。

## 0. 结论摘要

用户提出的目标方法论共 9 条规则(R1-R9,见上方背景)。现状实现是**一个真实存在、比表面看起来更完整的确定性子系统**,但不是按"LLM 每层手动重新发起状态查询"的方式实现的——而是把大部分逻辑下沉成了 Go 引擎里的确定性计算 + 事后自动注入,LLM 主要负责"调用 trace_query"和"写叙述性总结"两件事。**v7 已把 §4 里可精确定位、风险可控的高优先级 gap(O1/O3/O7/O9/O10)修复并补测试,O2 复核后判定不是真缺口撤销,O4/O6 明确推后到下一批**,以下是修复后的最新满足度:

| # | 要求(对应规则) | 满足度 | 一句话 |
|---|---|---|---|
| 1 | 窗口内各状态时长 Top-N 识别 + typed handoff(R2/R3) | **满足(v8 补齐占比阈值)** | `buildStateDrilldownPlan` 是真实的窗口级 Top-N;v8 新增 `WindowProportion`+`Significant`(top 恒真、低 rank 需过 5% floor 或 25% top-ratio)显著性软引导,R4 不删任何步骤(§2.1) |
| 2 | 碎片化状态聚类后 Top-N 不递归但仍 typed handoff(R4) | **部分满足** | 只对"碎片化 sleep"做了非递归例外,碎片化 runnable/D-state/IO 的 `Recursive` 标志仍是 true(标签层面),但底层引擎本来就不会为它们递归(见 #3) |
| 3 | on-chain 关联线程递归下钻直到根因(R5/R5a/R5b/R5c) | **基本满足(v7 已修复)** | `expandChain` 图遍历只对 Sleep→Wakeup 边递归(MaxDepth=10,带环检测);Running(算力供给)和 D-state/IO(聚类 inode)通过另一条"并行独立候选流 + on-chain 线程过滤"机制接了下一跳根因;Runnable 的优先级反转比较此前缺失,v7 已用 `applyRunnableTopPriorityInversion` 接上(§2.3.1) |
| 4 | 固化线程ID+时间窗触发主链根因下钻(而非浅层 grep)(R1) | **架构性满足(soft-guidance-by-design)** | 没有 typed pin 载体和硬门,只有 prompt 软引导"prefer trace_query before grep";但这本身符合仓库自己的"精确信号才能做硬门"红线,不算缺陷 |
| 5 | 逐层根因分析后,在用户时间窗内投影汇总 + 最终总结(R6) | **基本满足(v8 补齐窗口标注)** | `TraceCausalProjection` 无条件自动注入最终答案文档;v8 新增 `WithinRequestedWindow`(存在 `frame_target_resolution` 精确 anchor 时标注节点是否落在用户请求窗口内),把"投影回用户窗口"从隐式变显式(§2.5)。残留:注入的是结构化 fact sheet 而非叙述性总结,叙述仍靠 LLM(软引导) |
| 6 | JIT/VerifyClass/shader 特殊通道,占比再低也必须 handoff,且不干扰通用根因分析(R7) | **满足(v7 已修复上游口子)** | 仓库此前新增了完全独立于 `root_cause_rank` 排名的 `trace_semantic_span` typed observation 通道 + 专属"确定性优化点"渲染区块,并有 golden test 证明"即使不进 root_cause_rank 候选池也照样 handoff"。此前唯一未解决的口子——`computeTraceMarks(idx, q, 8)` 按原始时长硬顶 8 条、不感知语义类别——v7 已用 `boundTraceMarkSpans` 给语义 span 单独留 16 个名额修复(§2.6.3) |
| 7 | 严格遵守用户显式时间窗,不因 VSYNC/帧边界误缩窗(R8) | **满足(v8 修复 v7 引入的 explicit-0 回归)** | 三处窗口推导入口都以 `.Set()` typed 布尔为精确开关直接透传;**v7 的 O9 `unionTimeWindows` 曾用 `<=0` 当未设置,把用户显式 `time_start=0` 误缩到 span 起点——v8 已改纯 min 修复并补红转绿测试**;`interestingIntervals` Top-`MaxBranches` 深度覆盖裁剪已在 v7 加 caveat,详见 §2.9 |
| 8 | 帧信息 + 显式时间窗同时给出时取并集,不能二选一丢数据(R9) | **满足(v7 已修复)** | `ResolveFrameTarget` 一侧此前已并集;`resolveSpanWindowsForQuery`(`span_name` + 显式窗口)一侧 v7 用同一个（已提升为通用的）`unionTimeWindows` 补上,不再是"两者都给就只用显式窗口、丢弃 span 自身边界"(§2.9.2) |
| 9 | 测试覆盖 / load-bearing 程度 | **中等,呈碎片化** | 单元测试丰富且断言具体(非仅 JSON 存在性),但没有一条贯穿"低影响 JIT span → 候选产生 → 排序截断 → finalize → 最终渲染文本"的端到端 golden fixture;仓库无 CI,回归全靠人工 `go test` |

审计期间(约 2026-07-01 02:44 – 09:38)仓库有 20+ 次相关提交,说明该子系统正处于**主动收敛**状态,而不是静止的历史欠账;#6 的核心口子在审计过程中被实时补上,这提示优化方向本身与仓库现有开发节奏是一致的。

---

## 1. 现状架构总览

```
[perf_triage(可选前置阶段)]           内联小体量 hitrace/atrace,LLM 摘要成 PerfBundle
        │
        ▼
   explore 阶段(explorer agent)
        │  调用 trace_query 工具(唯一入口,20 个 view)
        ▼
┌─────────────────────────────────────────────────────────────┐
│ internal/tracequery/query.go(~11700 行,确定性 Go 引擎)      │
│  · ComputeWindowStats  —— 单窗口跨线程状态聚合                │
│  · BuildWakeupChain / expandChain —— 递归 sleep→wakeup 因果链  │
│  · BuildRootCauseRank —— 候选合并 + 排序 + tier 分配           │
│  · buildStateDrilldownPlan —— 状态优先的 Top-N 下钻建议        │
│  · traceSpanSemanticWorkClass —— JIT/VerifyClass/shader 识别   │
└─────────────────────────────────────────────────────────────┘
        │ 每次 trace_query 调用的 ToolResult(含 typed observations)
        ▼
internal/types/observation_ledger.go —— 把 ToolResult 文本行解析回 typed ObservationRecord
        │ (跨越 Turn A 全部 trace_query 调用历史累积)
        ▼
internal/types/trace_causal_projection.go —— CompileTraceCausalProjection
        │ (Primary / OnChain / Adjacent / Background / SemanticSpans / WakeupPath)
        ▼
internal/tool/answer_document_mutation_runtime.go
        │ persistMergedAnswerDocument() 在**每一次** emit_answer_document(_patch) 之后
        │ 无条件调用 materializeRuntimeTraceCausalProjectionBlock()
        ▼
最终 AnswerDocumentV2 被系统自动追加一个 "runtime_trace_causal_projection" 结构化区块
        (LLM 写不写都会有;这是本审计中最重要的机制发现)
```

关键认知:**trace_query 不是一个"返回原始数据、全靠 LLM 自己分层推理"的工具**。它内部已经把"哪个状态该优先看""要不要递归""要用哪个 view 分析"这些决策以确定性 Go 代码的形式算好了,通过 `StateDrilldownStep` / `RootCauseRankItem.Tier` / `TraceCausalProjection` 等 typed 结构 + `- state_drilldown ...` 这类可被 ledger 解析的文本行,把结论"喂"给 LLM 和下游 finalizer。LLM 的角色更像是"消费这些结构化建议、决定调用顺序、写叙述性总结",而不是"自己从零发明分层下钻策略"。这个设计与 CLAUDE.md §"精确信号做硬门、噪声信号做软引导"的红线是一致的:状态排序/tier 分配是精确计算,`AppliesTo`/推荐 view 是软引导。

---

## 2. 逐条方法论 vs 现状机制详解

### 2.1 窗口内状态时长 Top-N(要求 1,对应 R2/R3)

核心函数:`buildStateDrilldownPlan`(`internal/tracequery/query.go:3802`)。

```go
addDuration("top_sleep",    StateSSleep,  stats.SleepTop)
addDuration("top_runnable", StateRunnable, stats.RunnableTop)
addDuration("top_running",  StateRunning, stats.TopRunning)
addDuration("top_io_wait",  StateIOWait,  stats.IOWaitTop)
addDuration("top_d_state",  StateDSleep,  stats.DStateTop)
// + 每个线程的 state_churn(碎片化聚类)dominant state
```

`stats.SleepTop` / `RunnableTop` / `TopRunning` / `IOWaitTop` / `DStateTop` 由 `computeOffCPUStats`(query.go ~2183 起)和 `TopRunning` 聚合(query.go:1310-1333)填充,语义是**整个选定窗口里,按该状态耗时排序的线程列表**——即用户最新澄清的"整个窗口里各状态时长最长的状态"。这与本次审计早期认为的"跨线程排行榜、非单线程占比分解"表面上是同一个机制,但用户最新措辞已经把目标本身对齐到这个实现,因此该点判定为**基本满足**。

排序优先级(`stateDrilldownPriority`,query.go:3913):`score = ImpactMs; if ChainRequired { score *= 1.25 }`,再按 `TotalMs`、`LineStart` 兜底排序。去重后 `Rank` 编号,`max` 硬顶 12(query.go:1367、stream_search.go:373)。

**缺口(v8 已修复)**:修复前全程没有"占比"或"相对显著性"的概念,次高/第三高状态"是否显著"只靠"是否进入 Top 12"数量硬顶近似,不是用户 R3 要求的"看占比决定要不要分析"。**v8 落地**:`StateDrilldownStep` 新增 `WindowProportion`(= `ImpactMs / windowMs`,clamp 到 [0,1];windowMs 从既有的 `stats.Window` 推导,不改 `buildStateDrilldownPlan` 签名)+ `Significant`(rank=1 恒真——它就是"最长状态";低 rank 需 `proportion >= stateDrilldownSignificantFloor=0.05` 或 `impact/topImpact >= stateDrilldownSignificantTopRatio=0.25`)。这是精确标量比较驱动的软引导标记,**不删任何 Top-N 步骤**(R4),只告诉 LLM 优先下钻 `significant=true` 的状态。6 处 lockstep 同步:struct + `renderStateDrilldownStep` + banner `writeTraceStateDrilldownSummary` + typed observation emitter + ledger 白名单 `traceQueryStateDrilldownRecord` + skill prompt 软引导。windowMs≤0(测试无 Window)时 proportion=0、rank=1 仍 significant,pinning 测试保持绿。

每个状态对应的分析方法(`stateDrilldownRecommendedViews`,query.go:3928)是真实、彼此不同的:

| 状态 | 推荐 view |
|---|---|
| Sleep | `wakeup_chain`, `root_cause_rank` |
| Runnable | `scheduler_latency_stats`, `root_cause_rank` |
| Running | `trace_perf_bundle`, `perf_stats`, `root_cause_rank` |
| D-state / IO-wait | `critical_blocking_calls`, `window_stats`, `root_cause_rank` |

这精确对应用户"每一层都采用对应的分析方法"的要求。

### 2.2 碎片化状态聚类、非递归、但仍 typed handoff(要求 2,对应 R4)

`state_churn` 是独立于上面 5 个 Top 列表的第二个信息维度,代表"单次最长的连续状态"(`top_sleep` 等)之外的"频繁切换但聚类累计后最长"的状态(`ThreadStateChurnSummary`,由碎片检测 `isFragmentedSleepChurn` 等判定,query.go:3899)。`buildStateDrilldownPlan` 同时把两者都纳入候选池,`Source` 字段区分(`top_sleep` vs `state_churn`),**两个维度都会各自产出一条 typed `StateDrilldownStep`,不会互相覆盖**——这正是用户强调的"某状态持续时长最长"和"状态频繁切换但聚类累计后最长"两个维度都不丢失信息的要求。

非递归例外目前**只精确覆盖了 sleep**:

```go
// query.go:3943
func stateDrilldownNeedsWakeupChainForSource(state, source string) bool {
    if source == "state_churn" && state == string(StateSSleep) {
        return false   // 碎片化 sleep churn:不递归
    }
    return stateDrilldownNeedsWakeupChain(state)
}
```

碎片化 sleep churn 的 `RecommendedViews` 也换成了非递归的 `thread_timeline`/`interaction_stats`/`window_stats`(query.go:3921-3926),与"避免扩散"完全一致,并有专门测试 `TestStreamStateClusterPreservesParentWindowStatePriorities`(`internal/tracequery/tracequery_test.go`,2026-07-01 当天新增)锁定这个矩阵。

**v7 复核撤销此前的"缺口"判定**:碎片化 runnable / D-state / IO-wait churn 仍落到 `stateDrilldownNeedsWakeupChain(state)` 的通用分支,对 Runnable/DSleep/IOWait 返回 `true`(`ChainRequired`/`Recursive: true`)。此前(v1-v6)认为这是"言过其实"的标签,建议把 sleep-only 特判推广到所有 state_churn 来源。**动手前先跑了既有测试 `TestStateDrilldownRuleMatrixPinsRecentTracePolicies` 复核**,发现推广会直接破坏该测试对 fragmented-runnable/fragmented-IO 的显式断言(`ChainRequired=true`/`Recursive=true` + `scheduler_latency_stats`/`critical_blocking_calls`)。深入核实后判定**原诊断是误判**:`ChainRequired`/`Recursive=true` 这个标志的真实含义是"该候选的根因解释需要额外的调度/阻塞上下文 view",不是"会触发 `expandChain` 那种多跳图遍历"——Runnable/D-state/IO-wait 对应的 `scheduler_latency_stats`/`critical_blocking_calls` 都是**单次有界聚合 view**,不像 Sleep 对应的 `wakeup_chain` 那样是递归展开,调用多少次都不会"扩散";只有碎片化 **sleep** churn 需要非递归特判,是因为它唯一对应的下一步 view(`wakeup_chain`)才是真正的多跳递归。因此现状(只对碎片化 sleep 特判)是正确设计,**未做代码改动**,已用该测试重新确认锁定。

### 2.3 on-chain 关联线程递归下钻(要求 3,对应 R5)

核心函数:`expandChain`(`internal/tracequery/query.go:9920`)。这是一个**真正的服务端确定性递归**,不依赖 LLM 多次调用工具:

- `MaxDepth` 默认 10(query.go:611-612),超限记 `max_depth=%d reached` caveat 并停止(query.go:9921)。
- `visited` map 做环检测,命中记 `cycle detected` caveat(query.go:9925-9928)。
- 每个节点先算出该线程在对齐窗口内的 `mostInterestingInterval`(即该线程自己的主导状态,这一步precisely 对应用户所说"对该线程做快速状态查询"),再按状态分流:

```go
switch interesting.State {
case StateSSleep:
    wakeup := cache.findWakeup(...)
    waker := ThreadRef{...}
    childID := expandChain(idx, q, cache, waker, ..., depth+1, ...)   // 唯一真正递归的分支
case StateRunnable:
    res.RootEvidence = append(..., "thread was runnable but not running; inspect CPU pressure")
    // 终止,不递归
case StateDSleep, StateIOWait:
    res.RootEvidence = append(..., "IO or uninterruptible wait is a root-cause candidate")
    // 终止,不递归
case StateRunning:
    res.RootEvidence = append(..., "its own CPU work is root-cause evidence")
    // 终止,不递归
}
```

**递归只在 `StateSSleep` 分支真正调用自身**(`expandChain(...depth+1...)`)。当锚定线程(或链上任意一跳线程)的主导状态是 Runnable/D-state·IO-wait/Running 时,`expandChain` 本身把它当作图遍历的终止节点。但这**不等于**"这三种状态就没有下一跳根因分析"——`buildRootCauseRankFrom` 另外维护了一套**并行独立候选流**(compute_supply、cpu_affinity_or_cpuset、file_io_hot_inode、block_io_by_inode、page_cache_churn 等),每条候选都用 `onChain := threadInSet(chainThreads, thread)` 检查该候选的线程是否等于锚定目标本身或已被 sleep 链发现的线程,命中则以 `on_chain` 优先级进入统一排序。也就是说:对"锚定线程自己"这一层(depth=0,天然在 `chainThreads` 里),Running/D-state/IO 已经有实质性的下一跳数据;真正的空白只在"是否识别出优先级反转"这一件事上(Runnable,§2.3.1)。以下按状态逐一说明现状,对应用户这次追加的三点要求。

#### 2.3.1 Runnable 下一跳:优先级反转(含鸿蒙/东湖优先级语义,对应 R5a)——**v7 已修复**

基础设施是真实存在的,而且**已经正确区分了鸿蒙/东湖与 Android 的优先级语义**:

```go
// query.go:10298 —— 用于 sleep 链上"waker 相对于最初 target 的优先级"比较
func priorityRelation(flavor TraceFlavor, wakeePrio, wakerPrio int) string {
    switch flavor {
    case TraceFlavorHarmonyHitrace:
        if wakerPrio < wakeePrio { return "lower_priority_waker" }   // 鸿蒙:数值越大优先级越高
        if wakerPrio > wakeePrio { return "higher_priority_waker" }
        return "same_priority"
    default:
        return "raw_priority_uninterpreted"   // Android/generic:显式拒绝比较,避免误用鸿蒙语义
    }
}

// query.go:10280 —— 更通用的"当前链节点 vs 最初 target"优先级比较,供 PriorityInversionCandidate 使用
func dependencyPriorityRelation(flavor TraceFlavor, targetPrio, dependencyPrio, depth int) string { /* 同上语义 */ }
```

`default` 分支显式返回 `raw_priority_uninterpreted` 而不是硬套鸿蒙的"数值越大优先级越高"规则,说明这处比较**已经考虑了鸿蒙/东湖与 Android/generic ftrace 优先级语义不同**这件事,与 `docs/architecture.md` 里"HarmonyOS 用户态优先级数值越大优先级越高(1-40=CFS,41-139=RT),Android/generic ftrace 保留原始调度优先级"的既有描述一致。

`PriorityInversionCandidate`(`WakeupCausalImpact`,query.go:10090)和聚合后的 `PriorityInversion`(`WakeupCausalAggregate`,query.go:6194-6208)这两个 typed 标志也是真实存在、且正确接线的——**但仅限于 sleep 链内部**:`summarizeWakeupCausalImpact` 在 `expandChain` 每个节点上都会算一次"当前节点线程优先级 vs 最初 target 优先级",如果某个 sleep 链上的中间 waker 节点自己的 dominant state 恰好是 Runnable 且优先级低于最初 target,才会被标记为 `priority_inversion_runnable_wait`(query.go:7843-7844、10229-10230)。

**缺口**:当**最初被锚定的目标线程自己**(depth=0)就是 Runnable(最常见的场景——用户问"这个线程为什么没抢到 CPU"),`buildRootCauseRankFrom` 走的是 `stats.RunnableTop` 直接构造的独立 `runnable_wait` 候选(query.go:6696-6706):

```go
for _, td := range stats.RunnableTop {
    onChain := threadInSet(chainThreads, td.Thread)
    item := rootCauseItem("runnable_wait", td.Thread, ..., fmt.Sprintf("%s was runnable for %.3fms%s",
        threadLabel(td.Thread), td.DurationMs, durationCPUDetail(td)))   // durationCPUDetail 只有 cpu=/freq=,没有优先级
    ...
}
```

这条路径**完全没有调用 `priorityRelation`/`dependencyPriorityRelation`**,`durationCPUDetail`(query.go:11216)也只拼 `cpu=X freq=Ykhz`,不含任何优先级字段。同一文件里已经存在的"同 CPU 竞争者识别"能力(`appendRootCauseRunnableCompetitorPerfContexts`,query.go:7015,复用 `stats.CPUPressure.TopRunning`/`SameCPUTopRunning` 找出具体是哪个线程占着 CPU)也只附加了该竞争者的 **perf 采样上下文**(它在跑什么代码),同样没有把竞争者的调度优先级取出来和当前线程比较。

**结论**:优先级反转判定的算法原语(含鸿蒙/东湖语义区分)已经写好且被验证过(在 sleep 链场景下),只是没有被接到"目标线程自己直接 Runnable"这条最常见的路径上。这是本次 v2 审计里最精确、最容易修的一个缺口——不需要新造机制,只需要把已有的 `dependencyPriorityRelation` 喂给 `runnable_wait`/`cpu_affinity_or_cpuset` 候选构造,对象是 `appendRootCauseRunnableCompetitorPerfContexts` 已经找到的同 CPU 竞争者。见 §4-O3a。

**v7 修复**:新增 `applyRunnableTopPriorityInversion`(query.go,`durationCPUDetail` 旁),在 `stats.RunnableTop` 循环内对每个 `runnable_wait` 候选调用 `runnableContextForThread(td.Thread, stats.RunnableContext)` 取出已经解析好的 `SameCPUTopRunning[0]`(同 CPU 竞争者,复用既有原语,不重新实现"找竞争者"逻辑),再用 `threadPriorityNear` + `dependencyPriorityRelation(q.TraceFlavor, targetPrio, competitorPrio, 1)` 判定;命中 `lower_priority_dependency` 时把 `item.Type` 重分类为 `priority_inversion_runnable_wait`(该类型已在 `rootCauseShouldBeCoPrimary`/tier 分配逻辑里被消费,只是此前从未被这条路径产出过,纯粹是接线工作)。`TestRootCauseRankFlagsRunnableTopPriorityInversion` 用鸿蒙优先级语义(数值越大优先级越高)构造"高优先级 app 被低优先级 rival 占着 CPU"场景,断言重分类为 `priority_inversion_runnable_wait` 且在 on-chain 时正确升为 `tier=primary`;`TestRootCauseRankDoesNotFlagHigherPriorityCompetitorAsInversion` 复用已有 `schedulerLatencyTrace`(高优先级 rival 合法抢占低优先级 app)断言不会被误判为反转。

#### 2.3.2 Running 下一跳:算力供给 + perf_sample 深挖 + 代码对照(对应 R5b)——**基本满足,一处架构性权衡**

算力供给(compute supply)根因分类是真实、有实质判据的,而且同时覆盖 Running 和 Runnable 两种状态(`computeSupplySummaries`,query.go:3231-3298),核心是 `computeSupplyVerdict`(query.go:3300-3320):

```go
lowFreq := frequency > 0 && frequencyIsLowForCPU(frequency, cpu)
cpuPressure := busyRatio >= 0.80 || pressure.HighPriorityRunningMs >= durationMs*0.50
switch {
case cpuPressure && lowFreq: return "mixed_cpu_pressure_and_low_frequency", 0.78
case cpuPressure:            return "cpu_pressure", 0.76
case lowFreq:                return "low_frequency_signal", 0.68
case cpu.IdleMs > cpu.BusyMs: return "idle_available_check_wakeup_or_affinity", 0.62
default:                      return "insufficient_signal", 0.50
}
```

这个 verdict 会被投影成 `compute_supply` / `low_frequency` 两种 `RootCauseRankItem.Type`(query.go:6893-6895),连同 `cpu_affinity_or_cpuset`(CPU 亲和性/cpuset 限制,query.go:6920)一起,同样走 `onChain := threadInSet(chainThreads, ...)` 过滤后并入统一排序——对锚定目标线程自己(depth=0 天然在 `chainThreads` 里)而言,"算力供给"这个下一跳根因**已经是现成能力**,还带 `CoreClass`(大小核分类)、`Frequency`、`CPUBusyMs`/`CPUIdleMs`、`HighPriorityRunningMs` 等具体判据字段,不是一个模糊标签。

perf_sample 的"更深入分析"同样是真实存在的能力:`perf_stats`/`perf_timeline`/`window_stats.perf_samples` 提供 `top_symbols`/`top_dso`/`top_callchains`/`top_threads`,并带 `symbolization_status`/`sample_kind`/`clock_confidence`/`cpu_known` 等质量元数据(§2.6 之外的另一套独立机制,tool 描述里有大段专门教学),`root_cause_rank` 候选还能挂角色化的 `perf_context`/`perf_contexts`(`target_running`/`same_cpu_competitor`/`compute_supply_cpu` 等)。

**"对照代码"这一步是刻意不自动化的架构选择,而非疏漏**:全仓搜索没有"perf 符号自动解析到当前仓库源码位置"的机制;`internal/skill/defaults.go` 里明确写着 trace_query 的结果"是 runtime-artifact evidence with artifact-local line numbers;do NOT turn trace rows into current-source citations unless separate source evidence proves a current checkout fact"。也就是说,把一个 perf 符号/DSO 变成"当前仓库里第几行代码"必须由 LLM 另外发起一次 `grep`/`read_file` 独立验证,系统不会替它把两者直接拼接——这是防止"trace 里的符号名"和"当前签出代码"因为版本漂移而被误当成同一个事实的既有红线(与 §2.6.3 讨论的"trace 证据 vs 当前源码证据"边界是同一条原则)。这一点判定为**符合架构原则的设计取舍**,不列为缺口,但可以考虑 §4-O3b 的软引导加强,让 LLM 更稳定地记得做这一步。

#### 2.3.3 IO 下一跳:聚类 inode 定位(对应 R5c)——**满足度最高**

`stats.FileIOByInode`(`FileIOSummary`,按 `dev+inode+operation` 聚类,含 `Count`/`CompletionCount`/`Bytes`/`TotalLatencyMs`/`MaxLatencyMs`/`MinOffset`/`MaxOffset`)和 `stats.BlockIOByInode`(`BlockIOByInodeSummary`,把 inode 活动和最近的 block/storage 延迟拼在一起)都是真正的"按 inode 分组聚合"结果,不是原始事件的罗列。两者都在 `buildRootCauseRankFrom` 里各自独立构造 `file_io_hot_inode`(query.go:6594-6613)、`block_io_by_inode`(query.go:6658-6669)候选,同样走 `onChain := threadInSet(chainThreads, ...)` 过滤,附带具体 `inode=`/`dev=`/`op=`/`count=`/`bytes=`/`name=` 字段而非笼统的"该线程在等 IO"。`page_cache_churn`(query.go:6614-6626)和 `io_pressure`(:6627-6641)补充页缓存和聚合压力两个相邻维度。

这一条是本次 v2 追加的三点里满足度最高的:锚定线程(或已被 sleep 链发现的线程)一旦在 D-state/IO-wait,只要该窗口内有对应的 file_io/block_io 记录,`root_cause_rank` 就会把具体 inode 级候选和通用的 `io_wait`/`d_state_or_io_wait` 候选一起呈现,不需要额外改动。唯一值得注意的边界:这些候选的产生依赖 `stats.FileIOByInode`/`BlockIOByInode` 本身有没有数据(即 trace 是否包含对应的 F2FS/EXT4/block 事件),数据缺失时候选自然为空,这是数据源限制而非机制缺陷。

### 2.4 typed handoff 全链路(要求 1/2/3 共同的基础设施,对应 R2/R3/R4/R5)

从"计算出一个状态优先级"到"进入最终答案"要经过 4 层,每层都是 typed,没有裸文本推理环节:

1. **引擎产出结构体**:`StateDrilldownStep`(`internal/tracequery/types.go:755`)、`RootCauseRankItem`(含 `Tier`/`ChainRelevance`/`SemanticClass` 等)。
2. **序列化为可解析文本行**:`renderStateDrilldownStep` 产出形如 `- state_drilldown rank=1 thread=... state=sleep impact=12.500ms ... chain_required=true recursive=true lines=100-120` 的行,随 ToolResult 一起返回给 LLM(同时也是 ledger 的解析入口)。
3. **回收进 ObservationLedger**:`traceQueryStateDrilldownRecord`(`internal/types/observation_ledger.go:2214`)、`traceQueryRootCauseRankRecord`(:1914)把这些行**重新解析**回 typed `ObservationRecord`(`predicate="state_drilldown"` / `"root_cause_"+tier`),不依赖 LLM 复述。
4. **投影进最终结构**:`TraceCausalProjectionFromObservationRecords`(见 §2.5)按 `predicate` 分桶。

这条链路是本审计中确认"typed 且不丢信息"要求满足度最高的部分——从引擎输出到最终文档区块,信息载体全程是结构化字段,LLM 唯一必须做对的事情是"调用 trace_query"本身,而不需要在自己的自然语言输出里正确复述这些数字。

### 2.5 时间窗投影汇总 + 最终总结(要求 5,对应 R6)

核心机制:`TraceCausalProjection`(`internal/types/trace_causal_projection.go`)+ `materializeRuntimeTraceCausalProjectionBlock`(`internal/tool/answer_document_mutation_runtime.go:664`)。

`CompileTraceCausalProjection(ledger)` 遍历 **整个 Turn A 探索期间**累积的 `ObservationLedger.Records`(`ObservationLedgerInputFromBusContext`,`internal/types/observation_ledger_context.go:71`,优先取 Turn A 快照的完整 `ToolResults`,否则退回整个 bus 历史),按 predicate 分类聚合:

```go
type TraceCausalProjection struct {
    PrimaryRootCause  *TraceCausalProjectionNode
    PrimaryRootCauses []TraceCausalProjectionNode  // cap 10
    OnChainCauses     []TraceCausalProjectionNode  // cap 24
    AdjacentCauses    []TraceCausalProjectionNode  // cap 8
    BackgroundCauses  []TraceCausalProjectionNode  // cap 8
    SemanticSpans     []TraceCausalProjectionNode  // cap 16 (2026-07-01 二次扩容,见 §2.6)
    WakeupPath        []string
    SupportingHops    []TraceCausalProjectionNode  // cap 10
}
```

最关键的发现:`persistMergedAnswerDocument`(answer_document_mutation_runtime.go:119)是**每一次** `emit_answer_document` / `emit_answer_document_patch` 成功执行后都会走的共享持久化路径,其中**无条件**调用:

```go
if materializeRuntimeTraceCausalProjectionBlock(merged, ctx) {
    logging.Info("[%s] materialized runtime trace causal projection ...", toolName)
}
```

也就是说:只要 Turn A 期间调用过 trace_query 并产生了可分类的观测,**无论 LLM 自己的 emit_answer_document payload 里写不写这些根因**,系统都会在持久化时**额外追加一个 `runtime_trace_causal_projection` 区块**(`Kind=ordered_list`,`SurfaceRole=Principal`)。这是一条完全不依赖 LLM 主动配合的确定性 handoff 通道,有专门测试 `TestApplyAndPersistMutation_MaterializesRuntimeTraceCausalProjection`(answer_document_mutation_runtime_test.go:618)锁定。

**两个缺口**:
1. **不显式裁剪到用户最初声明的时间窗(v8 已修复)**。修复前 `CompileTraceCausalProjection` 把 Turn A 期间查询过的观测全部聚合,不区分是否落在用户原始窗口内,"汇总投影回用户指定时间窗"是隐式的(靠 LLM 查询纪律)。**v8 落地**:`TraceCausalProjectionNode` 新增 `StartTs`/`EndTs` + 三态 `WithinRequestedWindow *bool`;唯一精确非循环 anchor 是既有但此前被丢弃的 `frame_target_resolution` 记录(`window_source=query_window`/R9 union 白名单,即 R1 固化触发场景),用交集语义标注每个有窗口的节点在窗口内/外,渲染时追加对应 tag。无 anchor 或节点无窗口保持 nil 不臆造——诚实降级(primary root_cause 节点只有行号无窗口,天然标不出)。这把"投影回用户窗口"从隐式变显式,收益兑现在 R1 固化触发路径上。
2. **区块是结构化条目列表,不是叙述性总结(仍为软引导,不投资)**。`runtimeTraceCausalProjectionItems` 产出的是打标签条目 + "怎么读"引导语,是"事实清单附录"而非叙事性 mechanism 解释。真正的叙述性总结仍靠 LLM 在 `summary` block 里写,系统只提供软引导——这符合本仓库"系统不代替 LLM 写用户面板答案"的红线,判定为正确的架构边界,不改。

### 2.6 JIT / VerifyClass / shader 特殊通道(要求 6,对应 R7,用户最关心)

这是本次审计中变化最快的部分,分两层说:**候选产生层**(相对稳定)和**最终 handoff 层**(审计当天被重构)。

#### 2.6.1 候选产生:确定性识别 + 强制 co-primary

`traceSpanSemanticWorkClass`(query.go:9020)按 span 名称模式识别 4 种语义类,并带上不同的强度参数:

| 语义类 | Confidence | ImpactMultiplier | MinOnChainImpactMs |
|---|---|---|---|
| `jit_compile` | 0.82 | 2.60 | 4.0ms |
| `class_verification` | 0.82 | 2.40 | 4.0ms |
| `shader_compile` | 0.80 | 2.40 | 4.0ms |
| `runtime_compile` | 0.78 | 2.10 | 3.5ms |

`rootCauseShouldBeCoPrimary`(query.go:8382)是把"占比不高也要重视"这个要求落到 tier 分配的关键机制:

```go
func rootCauseShouldBeCoPrimary(item RootCauseRankItem) bool {
    if !rootCauseItemIsOnChain(item) || item.ImpactMs <= 0 { return false }
    switch item.Type {
    case ..., "jit_compile", "class_verification", "shader_compile", "runtime_compile", ...:
        return true   // 只要在链上且 impact>0,不看具体数值大小,强制 tier=primary
    }
}
```

即:**只要该语义 span 与 on-chain 因果链有重叠(`chain_relevance=on_chain`)且 impact 大于 0,不论具体数值多小,都会被强制标记为 `tier=primary`**——这精确对应用户"占比不高也不能因排名靠后被忽略"的诉求,而且判据是精确的 `on_chain` 布尔而非模糊阈值,符合仓库自己的"精确信号做硬门"原则。

但这一步发生在 `buildRootCauseRankFrom` 内 `items[:limit]`(`limit` 默认 12,query.go:6827-6834)**截断之后**才调用(`assignRootCauseRanksAndTiers(items)` 在 query.go:6835,晚于 6831-6833 的截断代码),因此如果同一窗口内 on-chain 候选总数超过 12 且该语义 span 的 `effective_impact_ms` 排不进前 12,co-primary 逻辑根本轮不到它执行——这是"tier 强制"这一层本身的局限,不是设计缺陷,而是"强制发生在幸存者身上"的先后顺序问题。

#### 2.6.2 最终 handoff:今日(2026-07-01 09:20,commit `ed109f7f fix: preserve trace semantic span handoff`)新增的独立通道

这次修复直接解决了"co-primary 只能保住已进入候选池的项"这个局限,思路是**开一条完全独立于 `root_cause_rank` 排序/截断逻辑的旁路**:

1. `traceQueryTypedSemanticTraceSpanObservations`(`internal/tool/trace_query.go`,新增 ~190 行)直接遍历 `WindowStats.TraceSpans`,只要 `SemanticClass != "" && DurationMs > 0` 就产出一条 `predicate="trace_semantic_span"` 的 `ObservationRecord`——**完全不检查是否进入过 `root_cause_rank.Items`,也不要求 `chain_relevance=on_chain`**(off-chain/adjacent 的语义 span 一样会被记录,只是 `Confidence` 更低:on_chain=0.82、adjacent=0.70、background=0.62)。golden test `TestTraceQueryTypedObservationsPublishSemanticSpanOutsideRootCauseRank`(`internal/tool/trace_query_typed_observations_test.go:833`)专门构造了一个 `RootCauseRank.Items` 里**不包含**该 `class_verification` span 的场景,断言它依然作为 typed observation 存活——这正是用户"不影响其它通用根因分析"的字面实现:两条通道并行,互不设卡。
2. `TraceCausalProjection` 新增独立字段 `SemanticSpans`(当前 cap 16,`traceCausalProjectionIsSemanticSpan` 按 `predicate=="trace_semantic_span"` 分类,不与 `PrimaryRootCauses` 混在一起)。
3. `runtimeTraceCausalProjectionItems` 里新增专属渲染分支,标签是 **"确定性优化点" / "Deterministic optimization point"**(与用户原话"确定性的优化点"字面一致),并且把原来写死的 "最多 6 条" 上限改成动态 `runtimeTraceCausalProjectionItemLimit`。初始扩容按 `primary+semantic+hops` 需求量在 12~36 之间浮动; 2026-07-01 二次扩容后进一步变为 16~48,同时 primary 展示扩到 10、semantic bucket 扩到 16、`trace_query 关键观测核对`扩到 40 行。这样主链和确定性优化点优先保真,adjacent/background 仍以 bounded summary 呈现,避免语义 span 与其它条目抢位置被挤掉。

#### 2.6.3 上游口子:`computeTraceMarks(idx, q, 8)` 的原始硬顶——**v7 已修复**

即使有了上面这条独立通道,所有语义 span 观测都要先从 `WindowStats.TraceSpans` 里取——而这个字段的唯一来源 `computeTraceMarks(idx, q, 8)`(query.go:1349)在语义分类**之前**就按**原始 `DurationMs` 降序**把 span 列表硬切到 8 条(query.go:4544-4549):

```go
sort.SliceStable(spans, func(i, j int) bool {
    if spans[i].DurationMs != spans[j].DurationMs { return spans[i].DurationMs > spans[j].DurationMs }
    return spans[i].StartLine < spans[j].StartLine
})
if max > 0 && len(spans) > max { spans = spans[:max] }   // max=8,不感知语义类别
```

如果一个窗口里同时存在 8 条以上耗时更长的普通具名 span(比如 `Choreographer#doFrame`、`RSRenderTask` 等常见渲染管线 span)和一条耗时很短的 `JIT compile`/`VerifyClass` span,后者会在语义分类逻辑跑之前就被排除出 `stats.TraceSpans`,从而**永远不会**进入 §2.6.2 描述的独立通道,也不会进入 §2.6.1 的 root_cause_rank 候选池。这是当前审计范围内**唯一残留的、精确定位到具体代码行的口子**,详见 §4 优化建议 O1。

**v7 修复**:新增 `boundTraceMarkSpans`(query.go,`computeTraceMarks` 排序之后调用),在原有"普通 span 硬顶 8 条"截断**之前**先按 `traceSpanSemanticWorkClass(span.Name)` 把语义类 span 单独摘出,generic 与 semantic 两组分别截断(generic 仍旧 8 条,semantic 另设 `traceMarkSemanticSpanCap=16`,与下游 `TraceCausalProjection.SemanticSpans` 当前 cap 对齐,避免"这一层留住了、下一层又被截"),合并后重新按时长排序返回。`TestComputeTraceMarksReservesSlotForShortSemanticSpanBehindLongerGenericSpans` 构造 9 条 10ms 的普通 span + 1 条 1ms 的 `VerifyClass` span,断言普通 span 仍被截到 8 条、但语义 span 独立存活,总计 9 条。

### 2.7 固化线程ID+时间窗触发深度下钻(要求 4,对应 R1)

全仓搜索确认:没有一个跨阶段传递的 typed "PinnedThread"/"PinnedWindow" 载体。`frame_target_resolution`(`trace_observation_coverage.go:10`)、`ResolveFrameTarget`(query.go:8601)都只是**单次 trace_query 调用内部**的锚点解析,不是跨调用持久化的固化事实。用户在自然语言里说"看看这个线程在这个时间窗为什么丢帧",这个"线程ID+时间窗"是否被后续调用复用,完全取决于 LLM 自己在每次调用时手动传入相同的 `pid`/`time_start`/`time_end` 参数——工具描述里有一句提醒("Once a result reports selected_window ... keep that same time_start/time_end ... on every follow-up ... view"),但这是 prompt 软引导,不是运行时校验。

同样,"要不要进入 trace_query 深度下钻"这件事本身也没有硬门:`internal/agent/runtime_triage_tool_filter.go` 只管 log_triage/perf_triage 阶段的 `read_file` 内联附件拦截,与 explore 阶段选 `trace_query` 还是 `grep` 无关。真正起作用的只有 `internal/skill/defaults.go` 里的一句 teaching prompt:"prefer `trace_query` before hand-written grep/awk loops"。

**这一条判定为"架构性满足"而非"缺陷"**:CLAUDE.md 明确写着"精确信号才能做硬门,噪声信号只能做软引导"——"这个问题是不是丢帧根因分析"本身是一个 LLM 语义判断(噪声信号),如果用一个硬门强制"检测到这类问题就必须先跑 trace_query 全套下钻流程",反而会在结构上没问题、只是问题类型判断轻微跑偏的场景里制造用户可见的失败,这正是仓库自己在架构原则里警告要避免的反模式。软引导 + 事后确定性投影兜底(§2.5 的自动注入机制,不管 LLM 是否记得引用都会把关键事实塞进最终文档)是更符合这条红线的设计。

### 2.8 数据溯源:每个计算指标依赖哪些原始 trace 事件

前面 §2.1-§2.7 描述的都是"算出来之后"的逻辑。本节往回倒一层,回答"这些数字最初是从 trace 文件里的哪些原始行读出来的"。整条链路的入口是 `internal/tracequery/parse.go` 的 `classifyEventType`(query.go:1734-1819)——它把每一行原始 ftrace/hitrace/systrace 文本,按事件名前缀/关键字分类成一个 `EventType`(`internal/tracequery/types.go:9-45`);`ComputeWindowStats`(query.go:1158)是唯一的中心聚合入口,对窗口内每个 `Event` 按 `ev.Type` 分流进各自的累加器。也就是说,**全部计算指标最终都可以唯一地追溯到某一类 `EventType`**,不存在"凭空计算"的字段。

| 计算指标(对应 §2 小节) | 具体计算函数(`internal/tracequery/query.go`) | 依赖的 `EventType` | 原始 tracepoint / 行模式 | 关键原始字段 |
|---|---|---|---|---|
| 线程状态(Running/Runnable/Sleep/D-state,§2.1/§2.3) | `stateFromPrevState`(1061)、`ComputeWindowStats` 里 `byCPU` 分桶(1189) | `EventSchedSwitch` | `sched_switch` | `prev_state`(R/S/D 前缀判定)、`prev_comm/prev_pid/prev_prio`、`next_comm/next_pid/next_prio`——**没有独立的"Running"事件**:`next_comm` 在 sched_switch 触发的瞬间即被视为进入 Running,直到下一条 sched_switch 把它切走 |
| IO-wait(D-state 的子分类) | `causalImpactBlockingMs`(10155)、D-state/IO-wait 分流(见 expandChain switch) | `EventSchedBlockedReason` | `sched_blocked_reason` | `iowait=`(非零则归为 IO-wait,否则维持通用 D-state)、`caller=`(阻塞点符号) |
| Sleep→Wakeup 因果链(§2.3) | `expandChain`(9920)、`cache.findWakeup` | `EventSchedWakeup` / `EventSchedWaking` | `sched_wakeup`、`sched_wakeup_new`、`sched_waking` | `comm/pid/prio`(waker)、`target_cpu`、事件自身时间戳作为唤醒时刻 |
| 调度优先级 / 优先级反转(§2.3.1,R5a) | `cache.priorityNear`、`priorityRelation`(10298)、`dependencyPriorityRelation`(10280) | `EventSchedSwitch` / `EventSchedWakeup` | 同上两行的 `prev_prio`/`next_prio`/`wakee_prio` 字段 | 数值优先级 + `TraceFlavor`(鸿蒙 vs Android)共同决定比较方向,原始数值本身来自 sched_switch/sched_wakeup,不是单独的事件 |
| 算力供给:CPU 频率(§2.3.2,R5b) | `computeSupplyVerdict`(3300)、`frequencyIsLowForCPU` | `EventCPUFrequency` | `cpu_frequency`;或 `clock_set_rate` 中经 `isCPUFrequencyClock`(2024)判定为 CPU 频率时钟域(`cpu_freq`/`cpufreq`/`scaling_cur_freq` 等)的行 | `state=`(目标频率,单位 kHz)、`cpu_id=` |
| 算力供给:CPU 忙闲/大小核(§2.3.2) | `computeSupplyVerdict` 的 `cpu.BusyMs/IdleMs`、`CoreClass` | `EventCPUIdle` | `cpu_idle` | `state=`(整数,parse.go:1293 直接 `atoi` 读入,进入/退出 idle 由该值区分)、`cpu_id=`;`CoreClass` 由观测到的频率区间聚类推断(或 `core_topology` 参数显式指定),不是原始事件字段 |
| CPU 亲和性 / cpuset 约束(§2.3.2 相关) | `computeCPUConstraintSummaries` | `EventSchedSwitch`(`next_info*` 字段) / `EventCPUConstraint` | `sched_switch` 行内嵌的鸿蒙 `next_info` 扩展字段;或独立的 `sched_setaffinity`/`sched_migrate_task`/`cpuset_attach`/`cgroup_attach_task` | `next_info_affinity`、`next_info_allowed_cpus`、`next_info_restricted`、`next_info_cgid` |
| Binder IPC(`ipc_graph`) | `attachIPCGraphToChain`、binder 累加器 | `EventBinderTransaction` 及 5 个配套事件 | `binder_transaction`、`binder_transaction_received`、`binder_transaction_alloc_buf`(或 `binder_alloc_buf`)、`binder_lock/locked/unlock`、`binder_reply`(或不带 `transaction_` 前缀的旧式命名) | 收发线程 pid、`flags`(oneway 判定)、事务号用于配对 send/receive |
| Block 层 IO / 存储延迟(`storage_latency_by_layer`) | `computeStorageLatencyByLayer`(5501) | `EventBlockIssue` / `EventBlockRemap` / `EventBlockComplete`,叠加 `EventStorage` | `block_rq_issue`/`block_rq_insert`/`block_getrq`/`block_bio_queue`(发起)、`block_bio_remap`(重映射)、`block_rq_complete`/`block_bio_complete`(完成);存储控制器层 `ufshcd_*`/`mmc_*`/`scsi_*`/`bio_*latency`/`ebpf_bio*`(SmartPerf eBPF 采集) | `dev`、`sector`、`length`,发起↔完成按 `(dev,sector)` 配对算延迟 |
| 文件系统 IO / 聚类 inode(§2.3.3,R5c,`file_io_hot_inode`) | `accumulateFileIO`(5334)、`isFileIOEvent` | `EventFilesystem` | `ext4_*`、`f2fs_*`(如 `f2fs_file_read_iter`/`f2fs_dataread_start` 等)、`android_fs_*`(如 `android_fs_dataread_start`)、`erofs_*`/`z_erofs_*` | `ino=`(聚类 key 的核心)、`dev`、读写方向、`len`/`bytes`、`entry_name`(部分事件带文件名) |
| Page Cache churn(`page_cache_by_inode`) | page cache 累加器(`EventMemory` 分支,`classifyMemoryKind` 判为 `page_cache`) | `EventMemory` | `mm_filemap_add_to_page_cache`/`mm_filemap_delete_from_page_cache` 等 `mm_*`/`filemap`/`page_cache` 关键字行 | `ino`/`dev`(若行内携带)、add/delete 计数用于算 churn |
| IO 压力聚合(`io_pressure_summary`/`io_burst_episodes`) | `computeIOPressureSummary`(5619)、`computeIOBurstEpisodes`(5851) | 上面 Block/Filesystem/`EventSchedBlockedReason` 的组合窗口聚合 | 同上 | 组合 D-state 时长 + block/storage 延迟 + `sched_blocked_reason iowait` 计数 |
| IRQ / SoftIRQ / IPI(供给侧压力) | `IRQCount`/`SoftIRQCount`/`IPICount` 累加(query.go:1204-1225) | `EventIRQ` / `EventSoftIRQ` / `EventIPI` | `irq_*` 前缀、含 `softirq` 关键字的行、`ipi_entry`/`ipi_exit`/`ipi_raise` | IPI 原因由 `parseIPIReason`(parse.go:1356)从 payload 解析(非固定 kv key),`target_mask=`/`target_cpus=`(parse.go:1357)取其一;entry/exit 配对可算 `active_ms`,单独 `ipi_raise` 只作瞬时信号 |
| Workqueue 活动 | Workqueue 累加(`WorkqueueEventCount`) | `EventWorkqueue` | 任意 `workqueue_` 前缀行(如 `workqueue_execute_start`/`_end`) | **没有专属结构化字段**(parse.go 的按类型字段提取 switch 里没有 `EventWorkqueue` 分支),只有事件计数 + 每个事件通用的 `comm`/`pid`/原始行文本(`FieldText`),细粒度信息要靠 `event_search`/`window_stats` 里的原始行文本自己读 |
| DMA Fence | `DMAFenceEventCount` | `EventDMAFence` | 任意 `dma_fence` 前缀行 | 同上,**没有专属结构化字段**,只有计数 + 通用 comm/pid/原始行文本 |
| sched_stat 内核记账(`sched_stat_accounting`,佐证而非替代 sched_switch) | `SchedStatCount` 累加 | `EventSchedStat` | 任意 `sched_stat_` 前缀行(如 `sched_stat_runtime`/`sched_stat_wait`/`sched_stat_iowait`/`sched_stat_blocked`) | 各类内核自记账耗时,与 sched_switch 区间时间独立采集,只作交叉校验 |
| Trace span / 帧管线(`frame_window`/`render_pipeline`等) | `computeTraceMarks`(4485) | `EventTraceMark` | `print`/`tracing_mark_write`/`tracing_mark_write_xacct`/`xacct_tracing_mark_write`,且 payload 经 `isTraceMarkPayload`(2034)判定为合法 B/E/S/F/C 格式 | `span_action`(B/E/S/F/C)、`span_pid`、`span_name`、`span_value`,同 PID 栈式配对 B/E,`marker pid+name+cookie` 配对 S/F |
| **JIT/VerifyClass/shader 语义 span(§2.6,R7)** | `traceSpanSemanticWorkClass`(9020) | 同上 `EventTraceMark` | **没有独立的内核 tracepoint**——语义分类是对 `EventTraceMark` 解析出的 `span.Name` 做纯文本模式匹配(如包含 "jit compile"、"VerifyClass"、"shader" 等 token),即用户态自己打的 trace marker 字符串里的名字凑巧命中这些模式才会被识别,**不是内核态确定性事件**;识别可靠性直接取决于应用/框架打点时用的 span 名字是否规范 | 见上,唯一附加信息是名字文本本身 |
| CPU Profiling / perf_sample(perf_stats/perf_timeline) | `ComputeWindowStats` 里的 `EventPerfSample` 分支(query.go:1429/2005/2086 等) | `EventPerfSample` | 行文本里事件名字面量就是 `perf_sample`——**这不是原始 ftrace/hitrace 里天然存在的行**,而是转换阶段(`internal/hitraceconv/raw_perfdata.go:1937`)把 hiperf/simpleperf 的 **`perf.data` 二进制 CPU 采样记录**,重新拼装成一行形如 `... perf_sample: cpu=.. pid=.. tid=.. symbol=.. dso=.. callchain=.. source=raw_perfdata_fallback symbolization_status=..` 的伪 ftrace 文本,再喂给同一个通用行解析器;因此 perf 样本的"可信度"字段(`symbolization_status`/`sample_kind`/`clock_confidence`)本质是在描述"这次转换有多可信",不是内核确定性保证 | `symbol=`、`dso=`、`callchain=`、`sample_weight=`、`event=`、`cpu_known=` |
| 鸿蒙专属资源监控(Ability/XPower/HiSystemEvent) | `AbilityEventCount`/`XPowerEventCount`/`HiSystemEventCount` 累加 | `EventAbilityMonitor` / `EventXPower` / `EventHiSystemEvent` | 行内文本包含 `ability_monitor`/`AbilityManager`(前者)、`xpower`(中者)、`hisysevent`/`hi_sysevent`/`hi_sys_event`(后者)关键字,均为鸿蒙特有子系统事件,靠关键字而非固定前缀匹配 | 视具体子系统事件而定,是 SmartPerf/HarmonyOS 专属遥测面 |

**几个值得记住的推论**:

1. **状态判定的唯一真源是 `sched_switch` 一个事件**——Running/Runnable/Sleep/D-state 四态里,只有 Sleep(`S`)和 D-state(`D`)是从 `prev_state` 字符串直接读出来的,Runnable 同样来自 `prev_state=R`(表示被切出但仍可运行),Running 则是"隐式的"——某线程在被 `next_comm` 选中的那一刻起,到它自己下一次作为 `prev_comm` 出现之前,都算 Running,中间没有任何独立事件。这意味着**如果一段窗口内该线程完全没有被调度(既不是 prev 也不是 next),引擎无法区分"确实一直在跑"和"数据缺失"**,只能报告 `no decisive scheduler interval found`(`expandChain` 里 `interesting == nil` 分支)。
2. **JIT/VerifyClass/shader 识别本质上是弱类型的文本匹配**,不像 sched_switch/sched_wakeup 那样有内核结构化字段兜底。这是 §2.6.3 提到的 `computeTraceMarks` 截断口子之外,该机制的第二个不确定性来源:如果应用/ArkCompiler/ROM 在不同版本里改了 trace marker 的命名习惯(比如把 "VerifyClass" 改成其它拼写),识别会静默失效,且没有测试覆盖这种命名漂移(呼应 §3 的测试空白)。
3. **perf_sample 是"二等公民"事件**——它不是原始 trace 文件里的行,依赖转换阶段是否成功把 `perf.data` 接进来(`tracebundle_trace_provider`/`tracebundle_trace_coverage` 这些 caveat 字段就是在描述这次转换的完整度)。如果只拿到裸 `.systrace`/`.ftrace` 没有配套 perf.data,§2.3.2 提到的"perf_sample 深挖"这条能力天然不可用,root_cause_rank 仍然可以工作(靠 sched_switch/sched_blocked_reason 等),只是缺少"CPU 时间具体花在哪个符号"这一层解释。
4. **鸿蒙 `next_info` 扩展字段是 CPU 亲和性/cpuset 分析的关键差异化数据源**——这些字段(`next_info_affinity`/`next_info_allowed_cpus`/`next_info_restricted`)内嵌在 `sched_switch` 行本身,是鸿蒙对标准 ftrace `sched_switch` 格式的扩展,Android/generic ftrace 的 `sched_switch` 行不带这些字段,因此"CPU 亲和性限制导致 Runnable"这类根因在纯 Android trace 上的可判定性天然弱于鸿蒙/东湖 trace。

### 2.9 显式时间窗的严格遵守 与 帧信息+时间窗并集(R8/R9)

用户的原始诉求分两层:R8——用户已经明确给出时间窗时,不能因为窗口中间出现 VSYNC 之类的帧边界信号就自作主张收窄;R9——如果用户**同时**给了帧标识(pattern/span_name)和显式时间窗,应取两者并集,不能只用其中一个而丢掉另一个覆盖的范围。本节把这两条分开审计,因为它们在代码里对应的是不同的机制,满足度也不同。

#### 2.9.1 R8:显式窗口是否被误缩——审计结论是**没有发现误缩,三处入口都有精确开关**

排查了 trace_query 里所有会"重新计算"时间窗的地方,一共三处,全部以**用户是否已经给出 `time_start`/`time_end`(哪怕只给一个)** 作为精确的 typed 开关,行为如下:

1. **`span_name` → 窗口推导**(`resolveSpanWindowsForQuery`,query.go:4598-4620):函数入参显式带 `explicitStart bool, explicitEnd bool`。第一件事就是 `if explicitStart && explicitEnd { return spans, caveats }`(query.go:4606-4608)——**两个边界都显式给出时,函数直接返回,完全不touch `q.TimeStart`/`q.TimeEnd`**,哪怕命中的具名 span 实际跨度比用户给的窗口更宽或更窄。只有当 `explicitStart`/`explicitEnd` 各自为 false 时,才会用匹配到的唯一 span 的边界去补齐**缺失的那一侧**(query.go:4613-4618),连"只给了起点没给终点"这种部分显式的情况都处理对了。
2. **frame_root_cause_bundle 的目标线程/窗口解析**(`ResolveFrameTarget`,query.go:8601-8665):函数一开始就判断 `if q.PID > 0 || strings.TrimSpace(q.Thread) != "" || strings.TrimSpace(q.ThreadInput) != "" { ... }`(query.go:8603)——**只要用户给了 pid/thread 中任意一个,直接走 `Source: "explicit_query_target"` 分支,`Window` 原样取自 `q.TimeStart`/`q.TimeEnd`,`WindowSource: "query_window"`,不进入任何"从帧列表候选里猜窗口"的逻辑**(query.go:8605-8616)。只有在没有显式 pid/thread 的分支(`frame_timeline_ui_candidate`)才会走到自动推导窗口的代码,而且还要求 `q.FrameWindowAutoDerived == true` 才会真正改写 `Window`(query.go:8656-8663)。
3. **`FrameWindowAutoDerived` 本身的置位条件**(`Run`,query.go:23-28 和 `traceQueryShouldAutoWindowFromPattern`,tool/trace_query.go:475-483):第一处只有 `q.TimeStart == 0 && q.TimeEnd == 0 && q.Pattern != ""` 才会置 true;第二处的核心判据 `traceQueryHasExplicitIndexWindow(p)`(tool/trace_query.go:1079-1081)是 `p.TimeStart.Set() || p.TimeEnd.Set() || p.LineStart.Int() > 0 || p.LineEnd.Int() > 0`——**只要 `time_start`/`time_end`/`line_start`/`line_end` 里有任何一个被显式设置过,自动窗口推导直接不触发**。

**结论**:三处入口对"用户是否已给窗口"的判断都是**精确的、typed 的**(`.Set()` 布尔态,不是靠数值是否为零去猜),完全符合 CLAUDE.md"精确信号做硬门"的红线,也完全符合 R8 的字面要求——本轮审计**没有找到**"因为检测到 VSYNC/帧边界而把显式给定窗口悄悄收窄"的代码路径。

**但有一处相关但不同的风险,发生在窗口"深度覆盖"层面而不是窗口"边界"层面**:`buildWakeupChainWithCache`(query.go:6097-6127)里,即使 `q.TimeStart`/`q.TimeEnd` 保持用户给定的原值不变(`res.Window` 依然精确等于用户窗口,元数据层面没有任何收窄),内部用来驱动递归下钻的 `branches := interestingIntervals(targetTimeline.Intervals, q.MinDurationMs, q.MaxBranches)`(query.go:6110)会把目标线程在该窗口内的完整时间线拆成多段区间,然后:
- **`StateRunning` 区间被无条件剔除**(`interestingIntervals`,query.go:10350-10352:`if intervals[i].State == StateRunning { continue }`)——如果目标线程在用户窗口内因为处理 VSYNC 回调而短暂 Running,这一段**不会**作为 wakeup-chain 的递归起点被展开(虽然它仍能通过 `stats.TopRunning`/`compute_supply` 这条独立候选流被看到,见 §2.3.2,只是不在这条链式展开里)。
- **候选区间按时长+状态打分后只取 Top `MaxBranches`(默认 8)个**(query.go:10355-10367)——如果目标线程在窗口内状态切换很多(超过 8 段满足 `MinDurationMs` 阈值的非-Running 区间),排名靠后的区间会被**静默丢弃**,不会被 `expandChain` 展开,即用户窗口"名义上"没变,但递归下钻的**实际覆盖深度**可能小于用户给定的整个窗口。

这条风险和 R8 的字面场景("因为 VSYNC 就缩小窗口")不完全等价——`res.Window` 报告给上游的元数据是准的,没有被 VSYNC 直接改写;但当窗口内确实存在一次 VSYNC 触发的短暂唤醒把目标线程的睡眠切成两段时,这套机制会把它们当成两条独立分支分别下钻,而不是把整个窗口当成一个统一的分析单元,且 Running 段完全跳过链式展开。是否需要修正取决于产品对"深度覆盖"这个更细粒度问题的容忍度,已经和窗口边界问题(R8 严格意义上已满足)分开记录,不建议混为一谈。

**v7 修复(caveat-only,不改变截断算法)**:`interestingIntervals` 新增第二个返回值——截断前的合格候选总数;`buildWakeupChainWithCache` 在合格总数大于实际展开的 `branches` 数量时追加一条 caveat,明确写出"目标线程有 N 个候选状态段,只有 M 个(按时长/状态优先级)被展开进唤醒链,K 个较低排名的候选未被递归"。这只是把此前完全静默的裁剪变得可观测,不改变默认 `MaxBranches=8` 或排序算法本身——是否需要提高上限留给后续基于真实案例判断(见 §4-O10)。`TestWakeupChainCaveatsWhenBranchesExceedMaxBranches` 复用既有 `fragmentedChurnTrace`(9 段 runnable 区间超过默认上限 8)断言 caveat 出现,`TestWakeupChainNoCaveatWhenBranchesFitMaxBranches` 断言候选数不超限时没有噪音。

#### 2.9.2 R9:帧信息 + 显式时间窗同时给出时的并集——**v7 已修复剩余的 `resolveSpanWindowsForQuery` 一侧**

v6 对着 §2.9.1 列出的三处入口重新看了一遍"两者都给"的场景,当时发现:

- `resolveSpanWindowsForQuery`:`explicitStart && explicitEnd` 为真时**直接返回,不读取、也不使用匹配到的 span 自身的时间边界**——span 只在 caveat 文本里被提及("selected_window derived from..." 这句提示语只在至少一侧未显式给出时才会拼出来),不会被并入最终窗口。也就是说如果用户给的窗口比实际帧短(比如漏掉了帧收尾的一小段),这段被漏掉的范围**不会**被 span 匹配结果找回来。
- `ResolveFrameTarget`/`applyFrameTargetResolution`(query.go:8767-8780):只要 pid/thread 显式给出,直接用 `query_window`,同样不会去看 `frame_timeline` 候选里对应帧的实际起止时间,更不会和它取并集。
- 全仓搜索(`internal/tracequery/*.go`、`internal/tool/trace_query.go`)没有找到任何对两个时间来源做 `min(startA, startB)` / `max(endA, endB)` 或等价并集操作的代码,`TimeWindow` 结构体本身也没有提供"合并两个窗口"的辅助函数。

**v6 结论**:当时实现在"用户同时给了帧信息和显式时间窗"这个场景下的行为是**"显式窗口整体胜出,帧信息只用来定位/校验目标线程,不参与窗口范围计算"**——这满足了 R8(不会因为帧信息把窗口缩得比用户给的更小),但不满足 R9(帧信息暗示的、可能比用户窗口更大的范围,不会被并入分析)。如果用户给的时间窗因为记忆/换算误差而比实际帧窗口略窄,旧实现会忠实地只分析用户给的那部分,不会自动把帧的完整范围补进来。该缺口已在 v7/v8 修复,当前不再作为 open gap 排队。

**v7 修复**:`ResolveFrameTarget` 一侧此前已经由一次并行提交修复(见 §6 的 `acc70dbc`),新增了 `unionFrameTargetWindows` 并接到 `frameTargetQueryHasExplicitSelectorWindow` 分支。v7 把这个函数**提升为通用的 `unionTimeWindows`**(改名,单一调用点改双调用点,行为不变,只补了一行文档注释)并接到 `resolveSpanWindowsForQuery`:当 `explicitStart && explicitEnd` 为真且唯一匹配到一个 span 时,不再直接 `return spans, caveats`,而是用 `unionTimeWindows(explicitWindow, spanWindow)` 计算并集,只有并集确实比显式窗口更宽时才改写 `q.TimeStart`/`q.TimeEnd` 并追加一条"preserved explicit query window ... unioned it with matched span ..." caveat(并集等于原窗口时保持静默,不产生噪音)。`TestSpanWindowExplicitQueryWindowUnionsMatchedSpanWindow` 断言窄显式窗口被并集扩展到完整 span 边界,`TestSpanWindowExplicitQueryWindowAlreadyCoveringSpanIsUnchanged` 断言显式窗口已覆盖 span 时保持不变且无多余 caveat。

**v8 修复 v7 这里引入的一个 R8 正确性回归(completeness critic 静态发现 + 真实 `Run()` 红测复现)**:v7 的 `unionTimeWindows` 用 `out.StartTs <= 0` 当"起点未设置"判据。但用户显式给出的 `time_start=0`(JSON `"time_start":0` → `TimeStartSet=true`、值为 0,`queryExplicitTimeStart` 在 `TimeStartSet` 时直接返回 true)会被这个判据误当成未设置,于是被匹配到的 span 起点(如 1.0)顶替——窗口起点从 0 被悄悄收窄到 1.0,正是 R8 明令禁止的"因帧/span 边界误缩用户显式窗口"。**v8 改为纯几何 min**:`if b.StartTs > 0 && b.StartTs < out.StartTs { out.StartTs = b.StartTs }`(删 `<=0` 特判;`b.StartTs > 0` 防呆防止未来某个未设置的 derived 起点把窗口误扩到 0 以下,与终点 `if b.EndTs > out.EndTs` 只向上扩天然安全对称)。补 `TestSpanWindowExplicitZeroStartNotShrunkByUnion`(真实 `Run()` 路径)+ `TestUnionTimeWindowsPreservesExplicitZeroStart`(直接单测)红转绿;两条既有 union 测试(非零起点)不受影响。explicit `time_start=0` 在真实 hitrace 里罕见(trace clock 通常是大单调值),但属静态可审的字面正确性回归,不留给实测兜底。

---

## 3. 测试覆盖与 load-bearing 程度(要求 7)

确认存在且具体断言(非仅 JSON 字段存在性)的相关测试:

- `TestStreamStateClusterPreservesParentWindowStatePriorities`(tracequery_test.go)——4 线程 4 状态混合场景,断言 `StateDrilldownPlan` 里每个状态对应正确的 `ChainRequired`/`Recursive`/`RecommendedViews`。
- `TestRootCauseRankKeepsOffChainSemanticTraceSpanAsSupporting` / `TestRootCauseRankSortsShortOnChainSemanticSpanByEffectiveImpact`(tracequery_test.go:3037-3094、3207+)——覆盖语义 span 的 on-chain/off-chain 分流与 `ImpactMultiplier` 排序提权。
- `TestTraceQueryTypedObservationsPublishSemanticSpanOutsideRootCauseRank`(trace_query_typed_observations_test.go:833)——今日新增,是本审计范围内**质量最高**的一条测试:直接构造"未进入 root_cause_rank"的反例场景来证明独立通道生效。
- `TestApplyAndPersistMutation_MaterializesRuntimeTraceCausalProjection`(answer_document_mutation_runtime_test.go:618)——验证 `persistMergedAnswerDocument` 会自动追加投影区块,并断言区块的 `ID`/`Kind`/`SurfaceRole`/条目内容。
- `trace_causal_projection_test.go`——多个用例覆盖 `CompileTraceCausalProjection` 的分桶/去重/排序逻辑。

**测试覆盖复核**:
1. `computeTraceMarks(idx, q, 8)` 的"普通 Top-8 + 短语义 span sidecar"场景已由 v7 的 `TestComputeTraceMarksReservesSlotForShortSemanticSpanBehindLongerGenericSpans` 锁住,不再是未覆盖项。
2. (v8 已补)贯穿到最终渲染文本的用例已由 `TestApplyAndPersistMutation_LowImpactSemanticSpanSurvivesToRenderedText` 覆盖:低影响语义 span 经 `render.RenderAnswerDocument` 后"确定性优化点"区块 + span 名仍在最终 markdown 里,不被 summary/cap 逻辑吞掉。
3. (v8 已补)端到端(analyzer→explore→extract→finalize 全链路)的 trace 丢帧场景 eval fixture 已由 `eval/cases/trace_query_frame_semantic_span_optimization.case` 落地,经 `eval/run.sh` 跑真实全链路 + 真实 LLM **PASS**——低影响 `VerifyClass` 语义 span 稳定出现在最终用户可见答案里(详见 §4-O5 第 3 项)。至此 §3 三项测试空白全部补齐。

仓库本身"无 linter、无 CI 配置"(CLAUDE.md 原文),意味着这些测试目前只在有人记得手动跑 `go test ./...` 时才生效,不能拦截无意中破坏这些不变量的未来改动。

---

## 4. 优化建议(按优先级排序)

**O1(最高优先级,精确定位到代码行)—— ✅ v7 已修复 —— 让 `computeTraceMarks` 的 Top-N 截断感知语义类别。**
现状:当前 `computeTraceMarks` 仍先按 `DurationMs` 排序,但不再直接 `spans[:max]`;v7 新增 `boundTraceMarkSpans`,把普通 span 与语义 span 分桶截断:普通 span 仍保留 Top-8,命中 `traceSpanSemanticWorkClass` 的 JIT/VerifyClass/shader/runtime-compile span 走独立 `traceMarkSemanticSpanCap=16` sidecar。这样短语义 span 不再和普通长 span 抢 8 个名额,能进入后续 `trace_semantic_span` typed observation、`TraceCausalProjection.SemanticSpans` 与"确定性优化点"渲染链路。

**v7 落地**:新增 `boundTraceMarkSpans`,语义 span 单独 cap=16,详见 §2.6.3。`TestComputeTraceMarksReservesSlotForShortSemanticSpanBehindLongerGenericSpans` 锁定。O1 本身已关闭;O5 的用户可见渲染与真实全链路 eval 看护已在 v8 补齐。

**O2 —— ❌ v7 复核后撤销(不是真缺口)—— 把碎片化状态的"非递归"例外从 sleep 推广到 runnable/D-state/IO-wait。**
现状:`stateDrilldownNeedsWakeupChainForSource`(query.go:3943)只对 `state_churn + StateSSleep` 返回 `false`。
~~建议:改成 `if source == "state_churn" { return false }`~~ —— **v7 动手前先跑了 `TestStateDrilldownRuleMatrixPinsRecentTracePolicies`,发现这个改动会破坏该测试对 fragmented-runnable/fragmented-IO 的显式断言。深入核实后判定原诊断误判了 `Recursive` 标志的含义(它标的是"需要额外单次有界聚合 view",不是"会触发像 `wakeup_chain` 那样的多跳递归"),现状是正确设计,详见 §2.2 更新。未做代码改动。**

**O3(第二高优先级,精确定位到代码行,v2 新增)—— ✅ v7 已修复 —— 把已有的优先级比较原语接到 Runnable 的直接候选构造上。**
现状(§2.3.1):`priorityRelation`/`dependencyPriorityRelation`(query.go:10280-10314)已经是正确的、鸿蒙/东湖与 Android 分流的优先级比较原语,`PriorityInversionCandidate` 标志也已验证可用——但只在 `expandChain` 的 sleep 链节点上被调用。`stats.RunnableTop` 直接构造的 `runnable_wait` 候选(query.go:6696-6706)完全没有调用它,`durationCPUDetail`(query.go:11216)只输出 `cpu=`/`freq=`,不含优先级;同 CPU 竞争者虽然已经能被 `appendRootCauseRunnableCompetitorPerfContexts`(query.go:7015)识别出来,但只附加了竞争者的 perf 采样上下文,没有把它的调度优先级取出来比较。
建议:在构造 `runnable_wait`(及 `cpu_affinity_or_cpuset`)候选时,对 `appendRootCauseRunnableCompetitorPerfContexts` 已经找到的同 CPU 竞争者调用 `cache.priorityNear` 取其优先级,再用 `dependencyPriorityRelation(q.TraceFlavor, td.Priority, competitor.Priority, 0)` 判定是否 `lower_priority_dependency`,命中则和 sleep 链一样标记 `PriorityInversionCandidate=true`、`type=priority_inversion_runnable_wait`。这是纯粹的"接线"工作,不需要新造算法,风险低、收益直接对应用户提出的诉求。
**v7 落地**:新增 `applyRunnableTopPriorityInversion`,复用 `runnableContextForThread`(而非 `cache.priorityNear`,因为这条路径没有 `chainQueryCache`,改用等价的无缓存版本 `threadPriorityNear`),详见 §2.3.1。

**O3b(低优先级,软引导型,v2 新增)—— 给 Running/compute_supply 主根因加一条"去对照代码"的软引导。**
现状(§2.3.2):算力供给判定(`computeSupplyVerdict`)和 perf_sample 分析本身已经足够深入,"perf 符号 → 当前仓库源码"的对照是刻意不自动化的架构选择(避免 trace 证据和当前源码证据被误拼)。
建议:不改变这条架构边界,只在 `internal/skill/defaults.go` 的 "TRACE QUERY" 教学块里补一句:当 `root_cause_rank` 的主根因(tier=primary)是 `running`/`compute_supply`/`low_frequency` 且带 `perf_context`/`perf_contexts` 时,提醒 LLM 用 `grep`/`read_file` 独立验证 `top_symbols`/`top_dso` 对应的当前源码位置,再引用为 current-source citation。属于纯 prompt 层面的加固,不涉及数据结构改动。

**O3c(信息性,v2 新增)—— IO 下一跳的聚类 inode 定位(§2.3.3)判定为已满足,暂不需要改动。**
`file_io_hot_inode`/`block_io_by_inode` 已经是真正的按 inode 聚类结果并接入统一排序,本轮审计未发现需要改动的地方;如果未来要加强,方向是扩大 `stats.FileIOByInode`/`BlockIOByInode` 的事件源覆盖(依赖具体 trace 是否采集了对应的文件系统/block 层事件),而不是查询引擎本身的逻辑。

**O4 —— ✅ v8 已修复 —— 让 `TraceCausalProjection` 显式携带"用户原始请求窗口"并在渲染时标注状态。**
现状(修复前):§2.5 指出投影不区分"落在用户原始窗口内"与"下钻过程中扩展查询到的相邻窗口"。
**v8 落地**:`TraceCausalProjectionNode` 新增 `StartTs`/`EndTs`(先取 `record.Span`,回退解析 window RichNote,复用既有 `traceCausalProjectionFloat`)+ `WithinRequestedWindow *bool`(三态:未知/内/外)。用户窗口的唯一**精确非循环** anchor 是既有但此前被丢弃的 `frame_target_resolution` 观测记录——只认 `window_source ∈ {query_window, explicit_query_union_previous_frame_end_to_current_frame_end}`(精确 typed 串白名单,即 R1 固化触发/R9 并集场景),用交集语义判定 within;无 anchor 或节点无窗口时保持 nil 不臆造。渲染侧 `runtimeTraceCausalProjectionWindowDetail` 仅在非 nil 时追加"落在用户请求窗口内"/"下钻到请求窗口外的上游依赖"tag,保证无 anchor 场景 byte-identical。已知边界:primary root_cause 节点只有行号无 per-node 窗口,故天然标不出——诚实降级,已在测试里锁定(`TestTraceCausalProjection*WithinRequestedWindow` 等 5 条)。

**O5(低成本、高价值)—— ✅ v8 三项全部落地 —— 补齐 §3 指出的回归测试空白。**
1. ✅ `computeTraceMarks` 截断 + 语义 span 共存:v7 已补 `TestComputeTraceMarksReservesSlotForShortSemanticSpanBehindLongerGenericSpans`。
2. ✅ 端到端渲染:v8 补 `TestApplyAndPersistMutation_LowImpactSemanticSpanSurvivesToRenderedText`——2ms 语义 span 走 observation → 自动注入 projection block → `render.RenderAnswerDocument` 最终 markdown,断言"确定性优化点"+`VerifyClass`+`class_verification` 全程存活(render 已是 tool 既有依赖,零 import cycle)。
3. ✅ **全链路真实 eval**:v8 新增自包含 case `eval/cases/trace_query_frame_semantic_span_optimization.case`(inline HTRACE:app-100 固化帧窗口 5.000~5.007s + worker-200 唤醒链 + on-chain `VerifyClass` 语义 span + 优先级反转),经 `eval/run.sh` 跑**真实 analyze→explore→extract→finalize 全链路 + 真实 LLM**,**PASS**(126s)。最终用户可见答案里:LLM 自己叙述"确定性可优化点为 VerifyClass 运行时字节码类校验…AOT 预编译预校验消除",系统自动注入的 projection block 第 8 项"确定性优化点 — worker-200 -> class_verification … chain_relevance=on_chain",并正确识别优先级反转(`priority=20/ohos_cfs` 阻塞 `target_priority=52/ohos_rt`,`relation=lower_priority_dependency`,鸿蒙优先级语义)。metrics:`tool_trace_query=1`、windowed+thread-filtered、`trace_query_final_projection_blocks=1`。这条 case 同时端到端验证了 v7 的 O1(语义 span 不被截断,`semantic_multiplier=2.40 hidden_cost_boost=true`)、O3(优先级反转下一跳)与 v8 的投影自动注入。

**O6(文档性,不涉及代码)—— ✅ v8 已修复 —— 在 `docs/architecture.md` 新增一节补充 trace_query 的分层下钻方法论。**
**v8 落地**:`docs/architecture.md` 新增 §7.2.1 "trace_query — 深度分层根因下钻引擎",纲要描述设计定位(确定性引擎非纯 LLM 推理)+ 四个核心机制(状态优先 Top-N / on-chain 递归 / 语义 span 独立通道 / 投影汇总)+ 窗口纪律,并指回本审计文档。

**O7(低成本、高价值,v3 新增)—— ✅ v7 已修复(caveat 部分)—— 给语义 span 识别加一条内核态兜底信号,降低对用户态命名习惯的依赖。**
现状(§2.8):`jit_compile`/`class_verification`/`shader_compile`/`runtime_compile` 全部靠对 `trace_mark` 里的 span 名字做文本模式匹配(`traceSpanLooksLikeJITCompile` 等),没有任何内核态结构化事件兜底;如果应用/ArkCompiler/ROM 版本升级后打点字符串命名习惯改变,识别会静默失效且无法被现有测试发现(现有测试固定用标准命名造 fixture,不会检测命名漂移)。
建议:短期低成本方案是在 `traceSpanSemanticWorkClass` 命中/未命中两侧都打一条可观测的 caveat(比如"检测到形如编译/校验语义但未匹配已知模式的 span,可能是新命名"),给 LLM 和后续维护者一个"模式可能过期了"的信号;中期可以考虑允许通过 `codrax.yaml` 追加自定义模式列表,而不是把所有命名规则硬编码在 Go 源码里。
**v7 落地短期方案**:新增 `traceSpanNearMissesSemanticWorkClassification` + 有界(≤3 例)caveat,详见 §2.8。中期的 `codrax.yaml` 自定义模式扩展仍是开放项,未在本批处理。

**O8(信息性,v3 新增)—— 视需要给 Workqueue/DMA Fence 补充结构化字段提取。**
现状(§2.8):这两类事件目前只有计数(`WorkqueueEventCount`/`DMAFenceEventCount`),没有像 Binder/Block IO 那样的专属结构化字段,细节要靠 LLM 自己读原始行文本。这两类事件目前不在用户提出的 7 条规则直接覆盖范围内,暂不建议单独立项,仅记录在案供后续如果有具体案例需要(比如 workqueue 延迟成为某次丢帧根因)时参考。

**O9(第二高优先级,精确定位到代码行,v6 新增)—— ✅ v7 已修复 —— 给"帧信息 + 显式时间窗同时给出"的场景补上并集逻辑。**
现状(§2.9.2):`resolveSpanWindowsForQuery`(query.go:4606-4608)在 `explicitStart && explicitEnd` 为真时直接返回,不读取匹配到的 span 自身边界;`ResolveFrameTarget`(query.go:8603-8616)在显式给了 pid/thread 时直接用 `query_window`,不去看 `frame_timeline` 里对应帧候选的实际起止时间。两处都没有对"用户显式窗口"与"帧信息推导窗口"做并集。
建议:给 `TimeWindow` 加一个 `UnionTimeWindow(a, b TimeWindow) TimeWindow`(`start=min(a.Start,b.Start)`、`end=max(a.End,b.End)`)辅助函数,在上述两处"两个来源都存在"的分支里调用它,而不是直接 early-return / 直接用 query_window;`WindowSource` 相应地增加一个 `explicit_window_union_frame_window` 之类的取值,方便下游区分"纯显式"和"并集后"两种情况,不破坏 §2.9.1 已经确认满足的 R8 行为(纯显式窗口、没有帧信息时不受影响)。
**v7 落地**:`ResolveFrameTarget` 一侧此前已由并行提交(`acc70dbc`)修复;v7 把它的 `unionFrameTargetWindows` 提升为通用 `unionTimeWindows` 并接到 `resolveSpanWindowsForQuery`,详见 §2.9.2。

**O10(低优先级,v6 新增)—— ✅ v7 已修复(caveat-only 部分)—— 视情况评估是否需要让 wakeup_chain 的 `branches` 覆盖用户整个显式窗口,而不是只取 Top-`MaxBranches`。**
现状(§2.9.1):`interestingIntervals`(query.go:10324-10381)把目标线程在用户窗口内的时间线按状态打分后只保留 Top `MaxBranches`(默认 8)个非-Running 区间进入 `expandChain` 递归展开,`res.Window` 元数据本身没有被收窄,但递归下钻的实际覆盖深度可能小于用户给定的整个窗口范围,且 Running 区间被无条件跳过链式展开(仍可通过独立候选流 §2.3.2 看到)。
建议:这是一个比 O9 更值得先观察真实案例再决定是否要动的问题——如果后续在具体丢帧案例里发现"因为 MaxBranches=8 截断导致真正的根因区间被漏掉"的实例,再考虑提高默认 `MaxBranches`、或者在 `res.Caveats` 里显式提示"目标线程窗口内被裁剪掉 N 个候选区间"(当前没有这类 caveat,裁剪是完全静默的),让 LLM/用户至少能感知到覆盖不完整,而不是贸然改变递归展开的算法本身。
**v7 落地建议的 caveat 部分**:`interestingIntervals` 现在返回截断前的合格总数,`buildWakeupChainWithCache` 据此追加可观测 caveat,详见 §2.9.1。是否提高默认 `MaxBranches` 仍留给后续基于真实案例判断,未在本批处理。

---

## 5. 附录:关键文件/函数索引

> v7 之后行号已因插入代码而普遍漂移(query.go 净增约 150 行),以下仅保留函数名作稳定锚点,不再逐一核对行号;需要精确定位时以函数名 grep 为准。

| 主题 | 文件 | 关键符号 |
|---|---|---|
| 状态 Top-N 下钻计划 | `internal/tracequery/query.go` | `buildStateDrilldownPlan`(3802)、`stateDrilldownPriority`(3913)、`stateDrilldownRecommendedViews`(3928) |
| 碎片化状态聚类 | `internal/tracequery/query.go` | `isFragmentedSleepChurn`(3899)、`stateDrilldownNeedsWakeupChainForSource`(3943) |
| 递归因果链(仅 sleep) | `internal/tracequery/query.go` | `expandChain`(9920)、`q.MaxDepth` 默认值(611-612) |
| 候选合并/排序/tier | `internal/tracequery/query.go` | `buildRootCauseRankFrom`(6537)、`sortRootCauseRankItems`(7373)、`assignRootCauseRanksAndTiers`(8371)、`rootCauseShouldBeCoPrimary`(8382) |
| 优先级反转(仅 sleep 链接线,v2) | `internal/tracequery/query.go` | `priorityRelation`(10298)、`dependencyPriorityRelation`(10280)、`PriorityInversionCandidate`(10090)、`causalImpactIsPrioritySensitiveRoot`(10146) |
| Runnable 直接候选优先级反转(v7 已接) | `internal/tracequery/query.go` | `runnable_wait` 构造循环、`applyRunnableTopPriorityInversion`(v7 新增)、`durationCPUDetail`、`appendRootCauseRunnableCompetitorPerfContexts` |
| Running/Runnable 算力供给(v2) | `internal/tracequery/query.go` | `computeSupplySummaries`(3231)、`computeSupplyVerdict`(3300) |
| IO 聚类 inode(v2) | `internal/tracequery/query.go` | `file_io_hot_inode` 构造(6594)、`block_io_by_inode` 构造(6658) |
| 语义 span 识别 | `internal/tracequery/query.go` | `traceSpanSemanticWorkClass`(9020)、`rootCauseItemFromSemanticTraceSpan`(7597) |
| 语义 span 上游截断(v7 已修复) | `internal/tracequery/query.go` | `computeTraceMarks`(调用 `boundTraceMarkSpans`,v7 新增,`traceMarkSemanticSpanCap=16`) |
| 语义 span 独立 typed 通道(今日新增) | `internal/tool/trace_query.go` | `traceQueryTypedSemanticTraceSpanObservations` |
| ObservationLedger 解析 | `internal/types/observation_ledger.go` | `traceQueryStateDrilldownRecord`(2214)、`traceQueryRootCauseRankRecord`(1914) |
| 时间窗投影 | `internal/types/trace_causal_projection.go` | `CompileTraceCausalProjection`(64)、`TraceCausalProjection` struct(22) |
| 自动注入最终文档 | `internal/tool/answer_document_mutation_runtime.go` | `persistMergedAnswerDocument`(119)、`materializeRuntimeTraceCausalProjectionBlock`(664) |
| 软引导 prompt | `internal/skill/defaults.go` | "TRACE QUERY:"(100)、"TRACE SEMANTIC SPAN ROOT CAUSES:"(105) |
| view teaching 表 | `internal/skill/trace_query_views.go` | `TraceQueryViewTeachings` |
| Running/compute_supply 代码对照软引导(v7 新增,O3b) | `internal/skill/defaults.go` | "TRACE RUNNING/COMPUTE-SUPPLY CODE CROSS-REFERENCE:" |
| 语义 span 命名漂移 caveat(v7 新增,O7) | `internal/tracequery/query.go` | `traceSpanNearMissesSemanticWorkClassification` |
| 帧信息+显式窗口并集(v7 已修复,O9) | `internal/tracequery/query.go` | `unionTimeWindows`(原 `unionFrameTargetWindows` 改名),`resolveSpanWindowsForQuery` 新分支 |
| 递归展开截断 caveat(v7 新增,O10) | `internal/tracequery/query.go` | `interestingIntervals`(新增 `qualifying` 返回值)、`buildWakeupChainWithCache` |
| 显式窗口透传(R8,已满足,v6) | `internal/tracequery/query.go` | `resolveSpanWindowsForQuery`(4598,`explicitStart && explicitEnd` 早退 4606-4608)、`ResolveFrameTarget`(8601,`explicit_query_target` 分支 8603-8616)、`applyFrameTargetResolution`(8767) |
| 自动窗口推导开关(仅无窗口时触发,v6) | `internal/tracequery/query.go` / `internal/tool/trace_query.go` | `Run` 里 `FrameWindowAutoDerived` 置位(query.go:23-28)、`traceQueryShouldAutoWindowFromPattern`(tool/trace_query.go:475)、`traceQueryHasExplicitIndexWindow`(tool/trace_query.go:1079) |
| 窗口内深度覆盖裁剪(R8 相关但不同,v6) | `internal/tracequery/query.go` | `interestingIntervals`(10324,Top-`MaxBranches` 截断 10365-10367,Running 跳过 10350-10352)、`buildWakeupChainWithCache`(6097) |
| 帧信息+显式窗口并集(R9,v7/v8 已修复) | `internal/tracequery/query.go` | `unionTimeWindows`,`resolveSpanWindowsForQuery`,`ResolveFrameTarget`;v8 修复 explicit `time_start=0` 回归 |

## 6. 审计期间(2026-07-01)实时落地的相关提交

供参考,说明审计对象在审计过程中本身就在演进:

- `3fd90913` test: pin trace wakeup depth schema
- `179cf119` test: guard trace causal depth handoff
- `8af399e4` test: guard trace parent state clustering
- `ed109f7f` **fix: preserve trace semantic span handoff**(§2.6.2 描述的独立通道)
- `abedbc7b` fix: expand trace observation supplements(渲染层截断上限从 8 放宽到 24)

本文档基线为拉取上述提交之后的 `origin/main`(`abedbc7b`)。如果后续继续有提交落地,建议以 `git log --oneline -- internal/tracequery internal/tool/trace_query.go internal/tool/answer_document_mutation_runtime.go internal/types/trace_causal_projection.go` 复核本文档是否过期。

**v2 修订说明**:v2 未拉取新的远程提交,基线仍是 `abedbc7b`,是对同一份代码补充审计 §2.3.1-§2.3.3(Runnable 优先级反转 / Running 算力供给+perf_sample+代码对照 / IO 聚类 inode 三个"下一跳"细项)与对应的 O3/O3b/O3c 建议,并修正 v1 里"Runnable/D-state/IO/Running 一概是终止节点"的过度笼统表述——实际只有 Runnable 的优先级比较是真正缺失的,Running 的算力供给和 IO 的聚类 inode 已经通过独立候选流机制覆盖。

**v3 修订说明**:审计期间又新落地一个不相关提交(`39a42409` fix: preserve repo-wide source inventory members,属于 source-inventory 子系统,与 trace_query 无关,未改变本文档基线)。v3 新增 §2.8 数据溯源表,把 §2 的每个计算指标逐一回溯到 `internal/tracequery/parse.go` 的 `classifyEventType`(1734-1819)分类出的具体 `EventType` 和原始 tracepoint 名称,补充了原文档没有覆盖的"这些数字最初从 trace 里怎么读出来"这一层。表中每一条 tracepoint/字段引用均已对照 `parse.go` 源码逐条核实(包括修正了草稿阶段两处未经验证的猜测:`cpu_idle` 的 `state=` 具体编码值、IPI `reason` 字段的解析方式),避免把"合理推测"当成"已验证事实"写进审计文档。

**v4 修订说明**:本批继续响应"补充块最多 6 条明显短板"的反馈,在不新增硬门、不解析用户原文/模型散文/工具 summary 的前提下,把 trace causal projection 的最终保留面从 16w 的 primary=6 / semantic=12 / max=36 扩到 primary=10 / semantic=16 / max=48,并把 `trace_query 关键观测核对`从 24 行扩到 40 行。该修订只扩大 hard-grounded typed trace_query observation 的 handoff/审计容量,背景与 adjacent 仍保持 bounded summary,避免为"完整性"引入新的噪音循环。

**v5 修订说明**:继续复核"补充块最多 6 条"后发现另一条 typed handoff 仍停在旧上限:`Typed Exploration Enrichment Facts` 的 `FlowFindings` / `Flow/source-sink rows`。这不是 trace causal projection 本身,但它承载探索阶段已经落地的 typed flow/source-sink 证据,在多窗口、多层唤醒链、跨组件调用链场景中同样可能把第 7 层之后的链路从 finalizer prompt 中挤掉。本批把 trace/root-cause 请求的 flow supplement 扩到 12 条,跨组件/调用链请求保留 8 条 preferred surface,普通请求仍保持 3 条小默认值。该通道仍是 bounded prompt/handoff,不是 completion hard gate;只消费 `FlowFindingDigest` 和 typed request intent/family/predicate enums,不解析用户原文、模型散文、工具 summary、localized UI 或 eval label。

**v6 修订说明**:审计期间发现本文档在此期间被并行地(基线 `4e53584d` → `4d5467a4`,共 7 个提交,其中 `264f0cd7`/`ae8fc57f`/`4d5467a4` 三次直接改了本文档自身,详见上面 v4/v5 段)持续更新,已用 `git reset --hard origin/main` 对齐到最新版本,不覆盖已有内容。v6 新增 R8/R9 两条规则(§背景)与对应审计 §2.9、新优化建议 O9/O10、附录索引新增 4 行。R8/R9 的判定不依赖前面几轮已经改动的 cap 数值(那些改的是 trace causal projection 的候选保留条数,和窗口边界计算是两回事),因此本轮结论与 §2.6.2 记录的最新 cap 数值不冲突。

**v8 复核说明(容量机制,并行提交 `29332679`)**:按当前代码再次复核 `ae8fc57f6 fix: expand trace causal handoff capacity` 与 `d0eddd6c7 docs: update trace audit doc with v7 fix status` 后确认,容量扩展机制仍在:`TraceCausalProjection` 的 primary/on-chain/context/semantic/supporting-hops cap 分别为 10/24/8/16/10,最终 `runtime_trace_causal_projection` 动态上限为 48,`trace_query 关键观测核对`上限为 40,链路分层摘要每类展示 4 个代表节点。此前 O1 的上游选择保真也已由 v7 的 `boundTraceMarkSpans` 修复,不再登记为 open gap。O5 的端到端渲染项已由下面的 v8 修订说明落地(`TestApplyAndPersistMutation_LowImpactSemanticSpanSurvivesToRenderedText`),全链路 eval fixture 也已由 `ab5174f70` 补齐并 PASS。

**v9 复核说明(同步至 `origin/main@ab5174f70`)**:重新拉取最新代码后复核,O1-O10 已全部落地或明确判定(O2 撤销、O8 记录在案),R1-R9 九条规则全部满足或正确降级。此前残留的文档漂移已修正:测试覆盖区不再写"未覆盖",附录不再保留 R9 的旧状态。当前可排队的唯一 trace 层增强是 O7 的中期配置化方向:把语义 span 模式扩展从 Go 源码内置迁移/补充为 `codrax.yaml` 可配置 pattern,用于适配客户私有 ROM / ArkCompiler / 应用自定义打点命名漂移;这是可观测性与可维护性增强,不是当前阻断性 correctness gap。

**v8 修订说明(第二批实际修复)**:动手前先跑一个 8-agent 设计+完备性 workflow(4 design + 4 adversarial verify),对每个剩余 gap 产出经对抗验证的 diff 级实现规范并让"完备性批判"agent 独立复核 v7 五项修复是否真生效。批判 agent 用真实 `Run()` 红测复现了 v7 O9 亲手引入的 `unionTimeWindows` explicit-0 起点收窄回归(R8 字面正确性 bug),v8 第一件事就修它。随后落地 R3(占比显著性)、O4(投影窗口标注)、O5 端到端渲染测试、O6(架构文档 §7.2.1),每项都有新单测,`go build ./... && go test ./...` 全绿。O5 第三项(全链路真实 eval)也已在本轮补齐并 **PASS**——新增 `eval/cases/trace_query_frame_semantic_span_optimization.case`,经 `eval/run.sh` 跑真实 analyze→explore→extract→finalize + 真实 LLM,确认低影响 `VerifyClass` 语义 span、优先级反转、自动注入 projection block 都稳定出现在最终用户可见答案里(126s PASS)。至此 §4 的 O1-O10 全部落地或明确判定(O2 撤销、O8 记录在案),R1-R9 九条规则全部满足或正确降级;**唯一剩余的是 O7 中期增强项(codrax.yaml 自定义语义 span 模式化,属配置化增强而非缺口)**。
