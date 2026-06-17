# Trace Query Running + Perf Root-Cause Closure Plan

## Summary

`trace_query` 已经能把 perf sample 转成 `perf_samples`，并能把候选线程的 `perf_context`
挂到 `root_cause_rank.items[].perf_context`。但对于 running/compute-supply 类问题，
当前链路仍然偏向“算力供给”维度：CPU/core/frequency/affinity/同核竞争能够进入排序和
摘要，而“这段 CPU 时间实际在跑什么代码”没有稳定地作为同一候选的结构化支撑证据传递到
后端。

目标是补齐一条通用闭环：当 trace 同窗存在 perf 数据时，running、runnable 竞争、
CPU pressure、compute supply、on-chain dependency、binder peer 等候选都能携带
角色化 perf 上下文。最终回答应同时给出调度/供给因果依据和 perf 维度支撑，避免把
perf sample 单独当作根因，也避免只说“算力不足”而漏掉实际执行热点。

## Current Code Flow

- `BuildIndex` 解析 `.perftrace`、trace bundle、sibling systrace+perftrace 后产生
  `EventPerfSample`，字段包括 `pid/tid/cpu/period/event/symbol/dso/callchain/source/
  symbolization_status`。
- `ComputeWindowStats` 调用 `computePerfContext` 生成同窗 `WindowStats.PerfSamples`，
  包含 `top_symbols`、`top_dso`、`top_callchains`、`top_threads`、`top_events`。
- `BuildRootCauseRank` 先从 wakeup chain/window stats/scheduler latency/compute supply
  生成候选，再调用 `attachPerfContextToRootCauseRank`。
- `attachPerfContextToRootCauseRank` 当前只按 `item.Thread` 或 `item.NearestChainThread`
  取一个 `PerfContext`。线程明确的 D-state、runnable、file IO 等候选能拿到 perf；
  CPU pressure、supply pressure、部分 compute-supply/running 候选容易因为候选线程为空或
  竞争线程藏在 `same_cpu_top_running`/`CPUPressure.TopRunning` 中而丢失 perf 上下文。
- `FrameRootCauseBundle` 已有 `target_running_perf`、`on_chain_perf`、`binder_peer_perf`、
  `same_cpu_competitor_perf`，但这些是 bundle 级角色字段，不会稳定并入每条
  `root_cause_rank` 候选、typed observation 和 evidence handoff。
- `trace_query` 文案已经提示 perf 是代码执行上下文，但 root-cause handoff 仍以一个
  扁平 `perf_context` 为主，模型容易在 running 问题上只消费供给侧信息。

## Gaps

1. **Candidate perf context is single-subject only.**
   `root_cause_rank` 只挂一个 `perf_context`，且只看候选线程/nearest-chain 线程。
   对 CPU pressure、supply pressure、同核高负载竞争、目标线程 running 这类多主体候选，
   无法表达“target 在跑 A、same-CPU competitor 在跑 B、on-chain dependency 在跑 C”。

2. **Running candidates lack role-aware evidence.**
   running/compute-supply 的因果基础来自重叠窗口、CPU/core/frequency/affinity/同核竞争等
   scheduler/resource 信号；perf 应补充“这段 CPU 时间消耗在哪些符号/DSO/调用栈”。
   当前没有把该补充证据和具体候选绑定，导致最终答案可能只说算力供给，不说执行热点。

3. **CPU pressure and supply-pressure candidates often lose perf entirely.**
   这类候选的 `ThreadRef{}` 为空；真正有价值的线程在 `CPUPressure.TopRunning`、
   `RunnableContext.SameCPUTopRunning`、`ThreadCPULoad` 或 bundle role contexts 中。
   现有 attach 逻辑无法从这些结构补取 perf。

4. **Handoff visibility is incomplete.**
   Markdown summary、typed observations、evidence facts 和 final answer hint 都没有统一展示
   role-aware perf context。即使底层 bundle 有 role fields，后端消费时仍可能丢失优先级。

5. **Tests protect happy paths but not this class of gap.**
   已有测试覆盖文本 perf 聚合、bundle/sibling 合并、候选线程 perf_context、frame bundle
   role contexts；缺少对空线程 CPU pressure、running/compute-supply、多主体 role perf
   handoff 的单测和 eval。

## Design

### Data Model

新增 `RootCausePerfRoleContext`，由系统输出，不作为模型 tool-call 输入：

- `role`: 结构化枚举字符串，例如 `candidate_thread`、`nearest_chain_thread`、
  `target_running`、`on_chain_dependency`、`binder_peer`、`same_cpu_competitor`、
  `cpu_pressure_top_running`、`window_top_running`。
- `thread`: 该 perf context 对应线程，若为 CPU scope 可为空。
- `cpu`: CPU scope；未知为 `-1` 或省略。
- `window`: 该 context 的采样窗口，优先使用候选窗口，缺失时使用查询窗口。
- `reason`: 为什么这个 perf context 属于该候选，例如 `candidate thread`、
  `same CPU top running competitor`、`top running thread on pressure CPU`。
- `perf_context`: 复用现有 `PerfContext`。

在 `RootCauseRankItem` 增加 `perf_contexts []RootCausePerfRoleContext`，保留现有
`perf_context` 作为向下兼容的 compact 主 context。对于多主体候选，`perf_contexts` 是权威
结构；`perf_context` 可取最高优先级 role 的 context 以维持旧消费路径。

### Aggregation and Attachment

重构 `attachPerfContextToRootCauseRank` 为 stats-aware：

- 输入 `WindowStats`，不要只依赖 rank item 自身字段。
- 对每个候选先挂 `candidate_thread`/`nearest_chain_thread`。
- 对 `runnable_wait`、`scheduler_latency`、`state_churn_runnable`，补同核竞争线程
  `same_cpu_competitor`。
- 对 `cpu_pressure`、`supply_pressure`，按候选 CPU 的 `CPUPressure.TopRunning` 补
  `cpu_pressure_top_running`；若候选没有 CPU 字段，则从行号/窗口/pressure summary
  匹配最相关 CPU pressure 项，不做散文关键字推断。
- 对 `compute_supply`/`low_frequency`/`cpu_affinity_or_cpuset`，补候选线程上下文，并在
  `SameCPUTopRunning` 或同 CPU pressure 中补竞争线程上下文。
- 对 frame bundle 场景，将 bundle 级 `target_running_perf`、`on_chain_perf`、
  `binder_peer_perf`、`same_cpu_competitor_perf` 的语义同步映射到 rank item role contexts，
  避免 bundle 级信息在 evidence handoff 中丢失。

所有 role contexts 均基于结构化 trace/perf 字段、线程 ID、CPU、时间窗口和已有 stats 结构
计算；禁止通过用户问题关键字、模型散文或单 case 字符串匹配驱动逻辑。

### Ranking Semantics

- perf sample 不单独新增 root-cause 候选，也不单独提升 score。
- perf 只增强已有 scheduler/resource/root-cause candidate 的解释能力。
- root-cause 排序仍由 overlap、chain relevance、cumulative impact、state duration、
  supply evidence 等因果信号决定。
- 对 on-chain 依赖链上的 running、runnable、D/io_wait、compute-supply 仍按累计影响排序；
  perf role contexts 只说明该影响期间的代码热点。

### Handoff and Prompt

- Markdown root-cause rank 每条候选展示 compact `perf_contexts` role rows：
  `role/thread/cpu/window/samples/period/top_symbol/dso/top_callchain/source/status`。
- typed observation 的 rich notes 增加 `perf_role_contexts`，保留旧 `perf_context`。
- evidence facts 的 summary 追加 role-aware perf 摘要，让 evidence_pack 也不丢信息。
- final answer guidance 明确：
  - running/compute-supply 原因需要同时报告供给/调度依据和 perf 热点支撑；
  - perf 热点说明“CPU 时间花在哪里”，不能替代 scheduler overlap/chain/supply 因果证据；
  - 若 `symbolization_status=unsymbolized` 或只有 IP/DSO，应如实报告符号化限制。

### JSON Repair and Schema

新增字段均为 `trace_query` 输出字段，不要求模型在 tool-call JSON 中传入。
因此不需要新增 tool-call repair alias。需要更新 schema/prompt 教学，让模型知道如何消费：

- `root_cause_rank.items[].perf_contexts`
- `frame_root_cause_bundle.root_cause_rank.items[].perf_contexts`
- typed observation `perf_role_contexts`

若后续新增可输入过滤字段（例如 `perf_roles`），必须接入统一 JSON repair/compat 层。

## Task List

### Batch A: Documentation and Contracts

- [ ] 落盘本设计文档。
- [ ] 明确 `RootCausePerfRoleContext` 字段与 role 枚举。
- [ ] 确认新增字段是系统输出，不进入 tool-call JSON repair。
- [ ] 提交并推送设计文档。

### Batch B: Structured Data and Attachment

- [ ] 在 `internal/tracequery/types.go` 增加 `RootCausePerfRoleContext` 与
  `RootCauseRankItem.PerfContexts`。
- [ ] 让 root-cause 候选携带结构化 CPU/window 元信息，避免靠 summary 文本反推。
- [ ] 重构 `attachPerfContextToRootCauseRank`，输入 `WindowStats` 并挂载 role contexts。
- [ ] 为 CPU pressure/supply pressure/compute supply/runnable/scheduler latency/state churn
  补齐 candidate、nearest-chain、same-CPU competitor、top-running CPU roles。
- [ ] 保留 `PerfContext` 作为 primary compact context，来源为最高优先级 role。

### Batch C: Handoff and Guidance

- [ ] 更新 `trace_query` Markdown summary，展示 role-aware perf rows。
- [ ] 更新 typed observations，输出 `perf_role_contexts` rich notes。
- [ ] 更新 evidence facts，保留 role-aware perf 摘要。
- [ ] 更新 tool description、view matrix、default skill hint、final answer guidance。
- [ ] 确认描述不把 perf sample 作为单独 root cause，不引入用户意图关键字匹配。

### Batch D: Tests and Evals

- [ ] 新增单测：候选线程 perf_context 仍兼容。
- [ ] 新增单测：空线程 `cpu_pressure` 候选能通过 `TopRunning` 挂 `cpu_pressure_top_running`。
- [ ] 新增单测：`compute_supply`/`scheduler_latency` 候选挂同核竞争 perf role。
- [ ] 新增单测：evidence facts 和 typed observations 包含 role-aware perf 摘要。
- [ ] 新增 eval：问题不预置分析结论，只给 trace/perf 窗口，要求系统自己解释 running/
  compute-supply，并在答案中同时包含供给证据和 perf 热点。
- [ ] 复跑现有 trace/perf、state_churn、东湖 runnable-context 回归 cases，每次并行 2 个。

## Acceptance Criteria

- 有 perf 数据时，running/CPU pressure/compute-supply 候选能稳定输出角色化 perf 支撑证据。
- 没有 perf 数据时，原有 trace-only 输出保持稳定，仅缺省 role contexts。
- root cause 排序不因 perf 热点单独改变；排序仍基于结构化调度/资源因果信号。
- final answer 能同时表达“为什么是调度/供给原因”和“这些 CPU 时间消耗在哪里”。
- 不通过后缀、关键词或模型散文判断用户意图；所有逻辑基于结构化 trace/perf 输入和
  analyzer/tool 已有分类结果。
