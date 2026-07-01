# Trace 分层根因下钻方法论 —— 现状审计与优化建议(2026-07-01)

> 本文档是**只读代码审计**的产出,不包含任何代码改动。审计对象是 `internal/tracequery/`(引擎,~11700+1583+2377+634 行)、`internal/tool/trace_query.go`(工具外壳+teaching prompt,~5900 行)、`internal/skill/`(软引导 prompt)、`internal/types/observation_ledger.go` / `trace_causal_projection.go` / `trace_observation_coverage.go`(typed 载体)、`internal/tool/answer_document_mutation_runtime.go`(finalize 期自动注入)、`internal/agent/answer_document_evaluator.go`(finalize 软引导)。审计基线是 `origin/main@abedbc7b`(2026-07-01 09:38,审计过程中仓库仍在演进,§6 记录了审计期间实时落地的修复)。
>
> **v2 更新**:补充审计 on-chain 三种终止状态(Runnable/Running/D-state·IO-wait)各自"下一跳"应识别的具体根因——Runnable 的优先级反转(含鸿蒙/东湖优先级语义)、Running 的算力供给与 perf_sample/代码对照深挖、IO 的聚类 inode 定位。发现原先 §2.3/§4-O3 把这三个状态一概判定为"终止节点、无下一跳"过于笼统:实际上 `buildRootCauseRankFrom` 用了一种**不同于 `expandChain` 图遍历的并行独立候选流 + on-chain 线程集合过滤**模式,已经让算力供给(compute_supply)和聚类 inode(file_io_hot_inode/block_io_by_inode)在依赖线程本身或已被 sleep 链发现的线程上生效;唯独 Runnable 的优先级比较未被同样接入。详见 §2.3.1-§2.3.3(新增)与修订后的 §4-O3。

## 0. 结论摘要

用户提出的目标方法论有 7 个核心要求。现状实现是**一个真实存在、比表面看起来更完整的确定性子系统**,但不是按"LLM 每层手动重新发起状态查询"的方式实现的——而是把大部分逻辑下沉成了 Go 引擎里的确定性计算 + 事后自动注入,LLM 主要负责"调用 trace_query"和"写叙述性总结"两件事。逐条满足度:

| # | 要求 | 满足度 | 一句话 |
|---|---|---|---|
| 1 | 窗口内各状态时长 Top-N 识别 + typed handoff | **基本满足** | `buildStateDrilldownPlan` 是真实的窗口级 Top-N,但没有"占比阈值",只有数量硬顶(12) |
| 2 | 碎片化状态聚类后 Top-N 不递归但仍 typed handoff | **部分满足** | 只对"碎片化 sleep"做了非递归例外,碎片化 runnable/D-state/IO 的 `Recursive` 标志仍是 true(标签层面),但底层引擎本来就不会为它们递归(见 #3) |
| 3 | on-chain 关联线程递归下钻直到根因 | **部分满足,比初判更完整** | `expandChain` 图遍历只对 Sleep→Wakeup 边递归(MaxDepth=10,带环检测);但 Running(算力供给)和 D-state/IO(聚类 inode)通过另一条"并行独立候选流 + on-chain 线程过滤"机制已经把下一跳根因接了上去,唯独 Runnable 的优先级反转比较**没有**接入同一机制,是本次 v2 审计定位到的唯一精确缺口(§2.3.1) |
| 4 | 固化线程ID+时间窗触发主链根因下钻(而非浅层 grep) | **架构性满足(soft-guidance-by-design)** | 没有 typed pin 载体和硬门,只有 prompt 软引导"prefer trace_query before grep";但这本身符合仓库自己的"精确信号才能做硬门"红线,不算缺陷 |
| 5 | 逐层根因分析后,在用户时间窗内投影汇总 + 最终总结 | **基本满足** | `TraceCausalProjection` 是真实的、跨越整个 Turn A 调用历史的确定性聚合,并且**无条件自动注入**到最终答案文档(不依赖 LLM 主动引用);但不显式裁剪到用户最初声明的时间窗,且注入的是结构化 fact sheet 而非叙述性总结 |
| 6 | JIT/VerifyClass/shader 特殊通道,占比再低也必须 handoff,且不干扰通用根因分析 | **基本满足,审计期间刚被强化** | 审计过程中,仓库在 `ed109f7f`(09:20)新增了完全独立于 `root_cause_rank` 排名的 `trace_semantic_span` typed observation 通道 + 专属"确定性优化点"渲染区块,并有 golden test 证明"即使不进 root_cause_rank 候选池也照样 handoff"。**唯一仍未解决的口子**:更上游的 `computeTraceMarks(idx, q, 8)` 仍按原始时长硬顶 8 条,不感知语义类别,极端情况下短 JIT span 可能在语义分类之前就被淘汰 |
| 7 | 测试覆盖 / load-bearing 程度 | **中等,呈碎片化** | 单元测试丰富且断言具体(非仅 JSON 存在性),但没有一条贯穿"低影响 JIT span → 候选产生 → 排序截断 → finalize → 最终渲染文本"的端到端 golden fixture;仓库无 CI,回归全靠人工 `go test` |

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

### 2.1 窗口内状态时长 Top-N(要求 1)

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

**缺口**:全程没有"占比"或"相对显著性"的概念。全仓 grep `state_proportion` / `StateProportion` / `percent` / `PercentOfWindow` 均为零命中。也就是说次高、第三高状态"是否显著"完全靠"是否进入 Top 12"这个数量硬顶来近似,不是用户要求的"看占比决定要不要分析"。在候选数普遍 <12 的常见场景下二者效果接近,但在候选数很多、长尾都很短的窗口里,数量硬顶可能纳入并不显著的第 8~12 名状态,而占比阈值不会。

每个状态对应的分析方法(`stateDrilldownRecommendedViews`,query.go:3928)是真实、彼此不同的:

| 状态 | 推荐 view |
|---|---|
| Sleep | `wakeup_chain`, `root_cause_rank` |
| Runnable | `scheduler_latency_stats`, `root_cause_rank` |
| Running | `trace_perf_bundle`, `perf_stats`, `root_cause_rank` |
| D-state / IO-wait | `critical_blocking_calls`, `window_stats`, `root_cause_rank` |

这精确对应用户"每一层都采用对应的分析方法"的要求。

### 2.2 碎片化状态聚类、非递归、但仍 typed handoff(要求 2)

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

**缺口**:碎片化 runnable / D-state / IO-wait churn 目前仍落到 `stateDrilldownNeedsWakeupChain(state)` 的通用分支,对 Runnable/DSleep/IOWait 返回 `true`,也就是 `Recursive: true`。但见 §2.3,底层 `expandChain` 引擎本来就只对 Sleep 状态真正递归,所以这个 `Recursive: true` 标签对碎片化 runnable/D-state/IO-wait 而言存在"言过其实"的问题:调用方看到 `recursive: true` 可能会尝试进一步下钻,但引擎侧并没有对应的确定性递归路径承接。建议把 `stateDrilldownNeedsWakeupChainForSource` 的 sleep-only 特判**推广到所有 state_churn 来源**,与"碎片化状态一律不递归"的意图对齐,同时保留 typed handoff(这部分已经做到)。

### 2.3 on-chain 关联线程递归下钻(要求 3)

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

#### 2.3.1 Runnable 下一跳:优先级反转(含鸿蒙/东湖优先级语义)——**确认为精确缺口**

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

#### 2.3.2 Running 下一跳:算力供给 + perf_sample 深挖 + 代码对照——**基本满足,一处架构性权衡**

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

#### 2.3.3 IO 下一跳:聚类 inode 定位——**满足度最高**

`stats.FileIOByInode`(`FileIOSummary`,按 `dev+inode+operation` 聚类,含 `Count`/`CompletionCount`/`Bytes`/`TotalLatencyMs`/`MaxLatencyMs`/`MinOffset`/`MaxOffset`)和 `stats.BlockIOByInode`(`BlockIOByInodeSummary`,把 inode 活动和最近的 block/storage 延迟拼在一起)都是真正的"按 inode 分组聚合"结果,不是原始事件的罗列。两者都在 `buildRootCauseRankFrom` 里各自独立构造 `file_io_hot_inode`(query.go:6594-6613)、`block_io_by_inode`(query.go:6658-6669)候选,同样走 `onChain := threadInSet(chainThreads, ...)` 过滤,附带具体 `inode=`/`dev=`/`op=`/`count=`/`bytes=`/`name=` 字段而非笼统的"该线程在等 IO"。`page_cache_churn`(query.go:6614-6626)和 `io_pressure`(:6627-6641)补充页缓存和聚合压力两个相邻维度。

这一条是本次 v2 追加的三点里满足度最高的:锚定线程(或已被 sleep 链发现的线程)一旦在 D-state/IO-wait,只要该窗口内有对应的 file_io/block_io 记录,`root_cause_rank` 就会把具体 inode 级候选和通用的 `io_wait`/`d_state_or_io_wait` 候选一起呈现,不需要额外改动。唯一值得注意的边界:这些候选的产生依赖 `stats.FileIOByInode`/`BlockIOByInode` 本身有没有数据(即 trace 是否包含对应的 F2FS/EXT4/block 事件),数据缺失时候选自然为空,这是数据源限制而非机制缺陷。

### 2.4 typed handoff 全链路(要求 1/2/3 共同的基础设施)

从"计算出一个状态优先级"到"进入最终答案"要经过 4 层,每层都是 typed,没有裸文本推理环节:

1. **引擎产出结构体**:`StateDrilldownStep`(`internal/tracequery/types.go:755`)、`RootCauseRankItem`(含 `Tier`/`ChainRelevance`/`SemanticClass` 等)。
2. **序列化为可解析文本行**:`renderStateDrilldownStep` 产出形如 `- state_drilldown rank=1 thread=... state=sleep impact=12.500ms ... chain_required=true recursive=true lines=100-120` 的行,随 ToolResult 一起返回给 LLM(同时也是 ledger 的解析入口)。
3. **回收进 ObservationLedger**:`traceQueryStateDrilldownRecord`(`internal/types/observation_ledger.go:2214`)、`traceQueryRootCauseRankRecord`(:1914)把这些行**重新解析**回 typed `ObservationRecord`(`predicate="state_drilldown"` / `"root_cause_"+tier`),不依赖 LLM 复述。
4. **投影进最终结构**:`TraceCausalProjectionFromObservationRecords`(见 §2.5)按 `predicate` 分桶。

这条链路是本审计中确认"typed 且不丢信息"要求满足度最高的部分——从引擎输出到最终文档区块,信息载体全程是结构化字段,LLM 唯一必须做对的事情是"调用 trace_query"本身,而不需要在自己的自然语言输出里正确复述这些数字。

### 2.5 时间窗投影汇总 + 最终总结(要求 5)

核心机制:`TraceCausalProjection`(`internal/types/trace_causal_projection.go`)+ `materializeRuntimeTraceCausalProjectionBlock`(`internal/tool/answer_document_mutation_runtime.go:664`)。

`CompileTraceCausalProjection(ledger)` 遍历 **整个 Turn A 探索期间**累积的 `ObservationLedger.Records`(`ObservationLedgerInputFromBusContext`,`internal/types/observation_ledger_context.go:71`,优先取 Turn A 快照的完整 `ToolResults`,否则退回整个 bus 历史),按 predicate 分类聚合:

```go
type TraceCausalProjection struct {
    PrimaryRootCause  *TraceCausalProjectionNode
    PrimaryRootCauses []TraceCausalProjectionNode  // cap 4
    OnChainCauses     []TraceCausalProjectionNode  // cap 10
    AdjacentCauses    []TraceCausalProjectionNode  // cap 4
    BackgroundCauses  []TraceCausalProjectionNode  // cap 4
    SemanticSpans     []TraceCausalProjectionNode  // cap 6 (2026-07-01 新增,见 §2.6)
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
1. **不显式裁剪到用户最初声明的时间窗**。`CompileTraceCausalProjection` 只是把 Turn A 期间"查询过的"观测全部聚合,不会反向校验/裁剪到用户最初指定的 `time_start`/`time_end`。如果 LLM 中途查询了超出原始窗口的相邻窗口(这在递归下钻到 on-chain 上游线程时是常见且合理的),这些结果也会被并入投影,没有"最终只保留落在用户原始窗口内"的显式收口步骤。这本身不算错(下钻到窗口外的因果源是必要的),但意味着"汇总投影回用户指定时间窗"这一步目前是隐式的(靠 LLM 查询纪律),不是显式强制的。
2. **区块是结构化条目列表,不是叙述性总结**。`runtimeTraceCausalProjectionItems` 产出的是 `primary`/`co_primary`/`semantic_span`/`supporting_hop` 打标签的条目(每条一行文本 + 数值),配一句"怎么读"的引导语,本质是一份"事实清单附录",不是用户要求的"一次汇总后的总结提炼"这种叙事性 mechanism 解释。真正的叙述性总结仍然要靠 LLM 在 `summary` block 里自己写,系统只提供软引导(`answer_document_evaluator.go` 里的 handoff hint,要求"preserve ... visibly",非强制校验)。

### 2.6 JIT / VerifyClass / shader 特殊通道(要求 6,用户最关心)

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
2. `TraceCausalProjection` 新增独立字段 `SemanticSpans`(cap 6,`traceCausalProjectionIsSemanticSpan` 按 `predicate=="trace_semantic_span"` 分类,不与 `PrimaryRootCauses` 混在一起)。
3. `runtimeTraceCausalProjectionItems` 里新增专属渲染分支,标签是 **"确定性优化点" / "Deterministic optimization point"**(与用户原话"确定性的优化点"字面一致),并且把原来写死的 "最多 6 条" 上限改成动态 `runtimeTraceCausalProjectionItemLimit`(按 `primary+semantic+hops` 需求量在 12~24 之间浮动),避免语义 span 与其它条目抢位置被挤掉。

#### 2.6.3 仍未解决的口子:`computeTraceMarks(idx, q, 8)` 的上游硬顶

即使有了上面这条独立通道,所有语义 span 观测都要先从 `WindowStats.TraceSpans` 里取——而这个字段的唯一来源 `computeTraceMarks(idx, q, 8)`(query.go:1349)在语义分类**之前**就按**原始 `DurationMs` 降序**把 span 列表硬切到 8 条(query.go:4544-4549):

```go
sort.SliceStable(spans, func(i, j int) bool {
    if spans[i].DurationMs != spans[j].DurationMs { return spans[i].DurationMs > spans[j].DurationMs }
    return spans[i].StartLine < spans[j].StartLine
})
if max > 0 && len(spans) > max { spans = spans[:max] }   // max=8,不感知语义类别
```

如果一个窗口里同时存在 8 条以上耗时更长的普通具名 span(比如 `Choreographer#doFrame`、`RSRenderTask` 等常见渲染管线 span)和一条耗时很短的 `JIT compile`/`VerifyClass` span,后者会在语义分类逻辑跑之前就被排除出 `stats.TraceSpans`,从而**永远不会**进入 §2.6.2 描述的独立通道,也不会进入 §2.6.1 的 root_cause_rank 候选池。这是当前审计范围内**唯一残留的、精确定位到具体代码行的口子**,详见 §4 优化建议 O1。

### 2.7 固化线程ID+时间窗触发深度下钻(要求 4)

全仓搜索确认:没有一个跨阶段传递的 typed "PinnedThread"/"PinnedWindow" 载体。`frame_target_resolution`(`trace_observation_coverage.go:10`)、`ResolveFrameTarget`(query.go:8601)都只是**单次 trace_query 调用内部**的锚点解析,不是跨调用持久化的固化事实。用户在自然语言里说"看看这个线程在这个时间窗为什么丢帧",这个"线程ID+时间窗"是否被后续调用复用,完全取决于 LLM 自己在每次调用时手动传入相同的 `pid`/`time_start`/`time_end` 参数——工具描述里有一句提醒("Once a result reports selected_window ... keep that same time_start/time_end ... on every follow-up ... view"),但这是 prompt 软引导,不是运行时校验。

同样,"要不要进入 trace_query 深度下钻"这件事本身也没有硬门:`internal/agent/runtime_triage_tool_filter.go` 只管 log_triage/perf_triage 阶段的 `read_file` 内联附件拦截,与 explore 阶段选 `trace_query` 还是 `grep` 无关。真正起作用的只有 `internal/skill/defaults.go` 里的一句 teaching prompt:"prefer `trace_query` before hand-written grep/awk loops"。

**这一条判定为"架构性满足"而非"缺陷"**:CLAUDE.md 明确写着"精确信号才能做硬门,噪声信号只能做软引导"——"这个问题是不是丢帧根因分析"本身是一个 LLM 语义判断(噪声信号),如果用一个硬门强制"检测到这类问题就必须先跑 trace_query 全套下钻流程",反而会在结构上没问题、只是问题类型判断轻微跑偏的场景里制造用户可见的失败,这正是仓库自己在架构原则里警告要避免的反模式。软引导 + 事后确定性投影兜底(§2.5 的自动注入机制,不管 LLM 是否记得引用都会把关键事实塞进最终文档)是更符合这条红线的设计。

---

## 3. 测试覆盖与 load-bearing 程度(要求 7)

确认存在且具体断言(非仅 JSON 字段存在性)的相关测试:

- `TestStreamStateClusterPreservesParentWindowStatePriorities`(tracequery_test.go)——4 线程 4 状态混合场景,断言 `StateDrilldownPlan` 里每个状态对应正确的 `ChainRequired`/`Recursive`/`RecommendedViews`。
- `TestRootCauseRankKeepsOffChainSemanticTraceSpanAsSupporting` / `TestRootCauseRankSortsShortOnChainSemanticSpanByEffectiveImpact`(tracequery_test.go:3037-3094、3207+)——覆盖语义 span 的 on-chain/off-chain 分流与 `ImpactMultiplier` 排序提权。
- `TestTraceQueryTypedObservationsPublishSemanticSpanOutsideRootCauseRank`(trace_query_typed_observations_test.go:833)——今日新增,是本审计范围内**质量最高**的一条测试:直接构造"未进入 root_cause_rank"的反例场景来证明独立通道生效。
- `TestApplyAndPersistMutation_MaterializesRuntimeTraceCausalProjection`(answer_document_mutation_runtime_test.go:618)——验证 `persistMergedAnswerDocument` 会自动追加投影区块,并断言区块的 `ID`/`Kind`/`SurfaceRole`/条目内容。
- `trace_causal_projection_test.go`——多个用例覆盖 `CompileTraceCausalProjection` 的分桶/去重/排序逻辑。

**未被任何测试覆盖的关键路径**:
1. 没有一条测试把 `computeTraceMarks(idx, q, 8)` 的截断与语义 span 结合起来——即"窗口里有 8 条以上更长的普通 span,外加 1 条很短的 JIT span"这个场景,现在完全没有回归保护,§2.6.3 的口子随时可能被后续改动加重或减轻而不被发现。
2. 没有一条测试贯穿到 `internal/render/renderer.go` 的最终 Markdown/HTML 渲染文本——所有测试都停在 `AnswerDocumentV2` 结构体层面,"确定性优化点"这个区块最终在用户看到的渲染稿里长什么样、会不会被摘要截断逻辑(如 `citation_quote_max_chars`、`SummaryCapConfig`)波及,没有验证。
3. 没有端到端(analyzer→explore→extract→finalize 全链路)的 trace 丢帧场景 eval/fixture,现有测试全部是 `internal/tracequery`、`internal/tool`、`internal/types` 包内的单元测试。

仓库本身"无 linter、无 CI 配置"(CLAUDE.md 原文),意味着这些测试目前只在有人记得手动跑 `go test ./...` 时才生效,不能拦截无意中破坏这些不变量的未来改动。

---

## 4. 优化建议(按优先级排序)

**O1(最高优先级,精确定位到代码行)——让 `computeTraceMarks` 的 Top-N 截断感知语义类别。**
现状:`internal/tracequery/query.go:4544-4549`,纯按 `DurationMs` 降序截 8 条。
建议:在截断前先跑一遍 `traceSpanSemanticWorkClass` 分类,把命中 4 种语义类的 span 单独摘出、不占用普通 8 条名额,合并后再返回(例如"8 条普通 + 至多 N 条语义类,语义类不参与普通 8 条的时长排序竞争")。这样可以让 §2.6 已经建好的整条独立 handoff 链路(候选产生→typed observation→projection→"确定性优化点"渲染)真正做到"占比再低也不会在最上游被淘汰",而不是"占比不高但至少排进前 8 才行"。

**O2 —— 把碎片化状态的"非递归"例外从 sleep 推广到 runnable/D-state/IO-wait。**
现状:`stateDrilldownNeedsWakeupChainForSource`(query.go:3943)只对 `state_churn + StateSSleep` 返回 `false`。
建议:改成 `if source == "state_churn" { return false }`(所有碎片化聚类状态一律标记非递归),与 `expandChain` 实际只对 Sleep 递归的事实(§2.3)对齐,避免 `Recursive: true` 标签对 LLM 产生误导性期望。这是一处纯粹的一致性修复,风险低。

**O3(第二高优先级,精确定位到代码行,v2 新增)—— 把已有的优先级比较原语接到 Runnable 的直接候选构造上。**
现状(§2.3.1):`priorityRelation`/`dependencyPriorityRelation`(query.go:10280-10314)已经是正确的、鸿蒙/东湖与 Android 分流的优先级比较原语,`PriorityInversionCandidate` 标志也已验证可用——但只在 `expandChain` 的 sleep 链节点上被调用。`stats.RunnableTop` 直接构造的 `runnable_wait` 候选(query.go:6696-6706)完全没有调用它,`durationCPUDetail`(query.go:11216)只输出 `cpu=`/`freq=`,不含优先级;同 CPU 竞争者虽然已经能被 `appendRootCauseRunnableCompetitorPerfContexts`(query.go:7015)识别出来,但只附加了竞争者的 perf 采样上下文,没有把它的调度优先级取出来比较。
建议:在构造 `runnable_wait`(及 `cpu_affinity_or_cpuset`)候选时,对 `appendRootCauseRunnableCompetitorPerfContexts` 已经找到的同 CPU 竞争者调用 `cache.priorityNear` 取其优先级,再用 `dependencyPriorityRelation(q.TraceFlavor, td.Priority, competitor.Priority, 0)` 判定是否 `lower_priority_dependency`,命中则和 sleep 链一样标记 `PriorityInversionCandidate=true`、`type=priority_inversion_runnable_wait`。这是纯粹的"接线"工作,不需要新造算法,风险低、收益直接对应用户提出的诉求。

**O3b(低优先级,软引导型,v2 新增)—— 给 Running/compute_supply 主根因加一条"去对照代码"的软引导。**
现状(§2.3.2):算力供给判定(`computeSupplyVerdict`)和 perf_sample 分析本身已经足够深入,"perf 符号 → 当前仓库源码"的对照是刻意不自动化的架构选择(避免 trace 证据和当前源码证据被误拼)。
建议:不改变这条架构边界,只在 `internal/skill/defaults.go` 的 "TRACE QUERY" 教学块里补一句:当 `root_cause_rank` 的主根因(tier=primary)是 `running`/`compute_supply`/`low_frequency` 且带 `perf_context`/`perf_contexts` 时,提醒 LLM 用 `grep`/`read_file` 独立验证 `top_symbols`/`top_dso` 对应的当前源码位置,再引用为 current-source citation。属于纯 prompt 层面的加固,不涉及数据结构改动。

**O3c(信息性,v2 新增)—— IO 下一跳的聚类 inode 定位(§2.3.3)判定为已满足,暂不需要改动。**
`file_io_hot_inode`/`block_io_by_inode` 已经是真正的按 inode 聚类结果并接入统一排序,本轮审计未发现需要改动的地方;如果未来要加强,方向是扩大 `stats.FileIOByInode`/`BlockIOByInode` 的事件源覆盖(依赖具体 trace 是否采集了对应的文件系统/block 层事件),而不是查询引擎本身的逻辑。

**O4 —— 让 `TraceCausalProjection` 显式携带"用户原始请求窗口"并在渲染时标注裁剪状态。**
现状:§2.5 指出投影不区分"落在用户原始窗口内"与"下钻过程中扩展查询到的相邻窗口"。
建议:`TraceCausalProjectionNode` 已经有 `LineStart`/`LineEnd`/隐含的 `StartTs`/`EndTs`(via RichNotes),可以在投影阶段读取 `RequestModel`/`BusContext` 里是否存在用户声明的原始窗口,给每个节点补一个 `within_requested_window: bool` 或类似标记,渲染时把"用户窗口内的直接因果"和"为解释根因而下钻到窗口外的上游依赖"分两段呈现,而不是混在一个列表里。这能让"汇总投影回用户指定时间窗"这一步从隐式变显式。

**O5(低成本、高价值)—— 补齐 §3 指出的两类回归测试空白。**
1. 一条专门测试 `computeTraceMarks` 截断 + 语义 span 共存的用例(在 O1 落地前可以先写一条**红色**测试固化当前行为、暴露口子;O1 落地后转绿)。
2. 一条从 `WindowStats.TraceSpans` 一路跑到 `internal/render/renderer.go` 渲染文本的端到端用例,确认"确定性优化点"区块不会被通用的 summary 截断/摘要逻辑吞掉。

**O6(文档性,不涉及代码)—— 在 `docs/architecture.md` §7.2 或新增一节里补充 trace_query 的分层下钻方法论。**
现状 `docs/architecture.md` 只描述了 perf_triage 前置阶段,`trace_query` 这个约 22000 行、承载了本文档描述的几乎全部机制的核心引擎,在架构文档里没有对应章节。建议后续补一节纲要性描述(不需要本文档这么细),至少让新加入的开发者知道"状态优先 Top-N""on-chain 递归""语义 span 独立通道"这三个设计存在,避免未来重复发明或无意破坏。

---

## 5. 附录:关键文件/函数索引

| 主题 | 文件 | 关键符号 |
|---|---|---|
| 状态 Top-N 下钻计划 | `internal/tracequery/query.go` | `buildStateDrilldownPlan`(3802)、`stateDrilldownPriority`(3913)、`stateDrilldownRecommendedViews`(3928) |
| 碎片化状态聚类 | `internal/tracequery/query.go` | `isFragmentedSleepChurn`(3899)、`stateDrilldownNeedsWakeupChainForSource`(3943) |
| 递归因果链(仅 sleep) | `internal/tracequery/query.go` | `expandChain`(9920)、`q.MaxDepth` 默认值(611-612) |
| 候选合并/排序/tier | `internal/tracequery/query.go` | `buildRootCauseRankFrom`(6537)、`sortRootCauseRankItems`(7373)、`assignRootCauseRanksAndTiers`(8371)、`rootCauseShouldBeCoPrimary`(8382) |
| 优先级反转(仅 sleep 链接线,v2) | `internal/tracequery/query.go` | `priorityRelation`(10298)、`dependencyPriorityRelation`(10280)、`PriorityInversionCandidate`(10090)、`causalImpactIsPrioritySensitiveRoot`(10146) |
| Runnable 直接候选(未接优先级,v2) | `internal/tracequery/query.go` | `runnable_wait` 构造(6696)、`durationCPUDetail`(11216)、`appendRootCauseRunnableCompetitorPerfContexts`(7015) |
| Running/Runnable 算力供给(v2) | `internal/tracequery/query.go` | `computeSupplySummaries`(3231)、`computeSupplyVerdict`(3300) |
| IO 聚类 inode(v2) | `internal/tracequery/query.go` | `file_io_hot_inode` 构造(6594)、`block_io_by_inode` 构造(6658) |
| 语义 span 识别 | `internal/tracequery/query.go` | `traceSpanSemanticWorkClass`(9020)、`rootCauseItemFromSemanticTraceSpan`(7597) |
| 语义 span 上游截断(口子所在) | `internal/tracequery/query.go` | `computeTraceMarks`(4485,调用处 1349:`max=8`) |
| 语义 span 独立 typed 通道(今日新增) | `internal/tool/trace_query.go` | `traceQueryTypedSemanticTraceSpanObservations` |
| ObservationLedger 解析 | `internal/types/observation_ledger.go` | `traceQueryStateDrilldownRecord`(2214)、`traceQueryRootCauseRankRecord`(1914) |
| 时间窗投影 | `internal/types/trace_causal_projection.go` | `CompileTraceCausalProjection`(64)、`TraceCausalProjection` struct(22) |
| 自动注入最终文档 | `internal/tool/answer_document_mutation_runtime.go` | `persistMergedAnswerDocument`(119)、`materializeRuntimeTraceCausalProjectionBlock`(664) |
| 软引导 prompt | `internal/skill/defaults.go` | "TRACE QUERY:"(100)、"TRACE SEMANTIC SPAN ROOT CAUSES:"(105) |
| view teaching 表 | `internal/skill/trace_query_views.go` | `TraceQueryViewTeachings` |

## 6. 审计期间(2026-07-01)实时落地的相关提交

供参考,说明审计对象在审计过程中本身就在演进:

- `3fd90913` test: pin trace wakeup depth schema
- `179cf119` test: guard trace causal depth handoff
- `8af399e4` test: guard trace parent state clustering
- `ed109f7f` **fix: preserve trace semantic span handoff**(§2.6.2 描述的独立通道)
- `abedbc7b` fix: expand trace observation supplements(渲染层截断上限从 8 放宽到 24)

本文档基线为拉取上述提交之后的 `origin/main`(`abedbc7b`)。如果后续继续有提交落地,建议以 `git log --oneline -- internal/tracequery internal/tool/trace_query.go internal/tool/answer_document_mutation_runtime.go internal/types/trace_causal_projection.go` 复核本文档是否过期。

**v2 修订说明**:v2 未拉取新的远程提交,基线仍是 `abedbc7b`,是对同一份代码补充审计 §2.3.1-§2.3.3(Runnable 优先级反转 / Running 算力供给+perf_sample+代码对照 / IO 聚类 inode 三个"下一跳"细项)与对应的 O3/O3b/O3c 建议,并修正 v1 里"Runnable/D-state/IO/Running 一概是终止节点"的过度笼统表述——实际只有 Runnable 的优先级比较是真正缺失的,Running 的算力供给和 IO 的聚类 inode 已经通过独立候选流机制覆盖。
