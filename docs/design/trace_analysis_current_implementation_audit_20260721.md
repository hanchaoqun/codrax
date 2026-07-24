# Trace 分析当前实现规则考据与缺口审计

> 基线：`main` @ `686ba0941`（2026-07-21）
>
> 范围：`trace_query` 从查询、窗口、线程状态、唤醒链、因果影响、根因榜，到 `TraceCausalProjection` 与答案页“窗内可消除量”的完整路径。
>
> 方法：以当前 Go 实现和结构/行为测试为准；既有设计文档只用于交叉核对，不把历史方案当成现行规则。

## 1. 结论先行

当前 trace 分析已经不是“按某个 state 时长从大到小排”的简单实现，而是四套彼此相连、但口径不同的账：

1. **调度事实账**：窗口内每个线程的 running、runnable、S、D、iowait 等实际或投影时长。
2. **唤醒因果账**：只用受支持的 `sched_wakeup` / `sched_waking` 关系，把目标睡眠段连接到真实 waker；D/IO 不伪造唤醒边。
3. **排名归因账**：按类型闭表把事实量换成 `effective_impact_ms`，先按链路相关性分层，再按有效归因排序；`score` 只打破同值。
4. **可消除展示账**：从已经入选的正向根因席位中再筛出链上和邻接项，按修复方向分组展示；它目前不是完整的反事实节省证明。

最重要的审计结论有四个：

- **“有效归因”不等于“实测可节省时间”**。当前可消除榜的数值主要继承 `effective_impact_ms`；真正命名为 P3 counterfactual measure 的四个内部字段被刻意设为 `display_only`，没有任何消费者。
- **唤醒链边本身较严格，链上资格却有两种较宽的继承途径**。真实边要求 S-sleep 和匹配唤醒事件；但根因席位仍可能仅凭线程身份或包络资格继承 `on_chain`。因此答案页把所有链上量统称“已证明可消除”偏强。
- **周期源识别和近似重复折叠会改变硬结果，却依赖启发式信号**。前者使用 15% 节拍容差并直接改变有效量、排序和容量；后者使用 3% 近似值门槛直接折叠发布。这与仓库“精确信号做硬门、噪声信号只做软指导”的架构原则存在实质张力。
- **`via_thread` 的 `OnChain` 字段与“存在完整单调唤醒路径”不是同一事实**。只要该线程出现在扩展节点集合里就可标为 `OnChain=true`，即使随后找不到从它到目标的完整时间单调路径。

## 2. 端到端数据流

```text
Query.Run
  ├─ 解析 trace / flavor / platform / window / target
  ├─ thread timeline + window stats
  ├─ BuildWakeupChain
  │    ├─ 目标阻塞段候选
  │    ├─ S-sleep -> sched_wakeup/sched_waking -> waker
  │    ├─ 分支/深度/节点预算
  │    └─ causal_impact / aggregated_impact / root evidence
  ├─ BuildRootCauseRank
  │    ├─ 链上、邻接、背景候选建席
  │    ├─ 一因一席 / family fold / level reconciliation
  │    ├─ effective impact 闭表
  │    ├─ channel-first 排序、tier、ordinal、cap
  │    └─ P3 measurement 静默盖章
  ├─ trace_query tool -> typed ObservationRecord
  ├─ CompileTraceCausalProjection
  │    ├─ path election
  │    ├─ exact/near dedup + aggregate
  │    ├─ primary/on-chain/adjacent/background/support buckets
  │    └─ anchor window + attachments
  └─ answer document runtime projection
       └─ “窗内可消除量”榜、方向分组、保守算术小计、守恒披露
```

查询的窗口、目标线程和窗口解析结果决定一次 board；`MaxDepth`、`MaxBranches`、`MaxChainNodes`、`MinDurationMs`、`Limit`、拓扑、`ViaThread`、行范围、flavor/platform 也进入查询身份或结果构造。取消发生时，未完成的结果面整体丢弃，不发布半成品。

## 3. 时间窗口、线程身份与状态账

### 3.1 窗口与完整性

- 显式 `time_start/time_end`、frame/span 解析出的窗口或工具选择的窗口，最终都变成闭合查询范围。
- 排名侧的统计不是先扫全窗再猜链路，而是先构造 wakeup chain，再用链锚窗口做 rank stats sweep。
- 目标线程 timeline 不完整、线程 incarnation 无法可靠确认时，唤醒链 fail-closed；不会把相同 PID 在另一个生命周期的事件拼进来。
- 投影层优先使用 `frame_target_resolution` 的 query window/显式窗口并集；缺失时才从已选窗口或 wakeup/rank 记录回退。
- 当前多个窗口的展示名单最多保留 8 个，时间相近的窗口以约 `1ms` 去重；这是展示容量，不等于引擎只分析 8 个窗口。

### 3.2 状态全集

当前线程状态统一为：

| 状态 | 含义 | 进入时长账 | 典型根因类型 |
|---|---|---:|---|
| `running` | 正在 CPU 上运行 | 是 | `running` / supply deficit |
| `runnable` | 可运行但未得到 CPU | 是 | `runnable` / priority inversion |
| `s_sleep` | 可中断睡眠 | 是 | `sleep_wait` / `missing_wakeup` |
| `d_sleep` | 不可中断睡眠 | 是 | D-state |
| `io_wait` | 明确 iowait | 是 | IO wait |
| `stopped` | 停止态 | 否 | 上下文 |
| `dead` | 死亡态 | 否 | 上下文 |
| `unknown` | 无法归类 | 否 | gap/context |

dominant state 先比较时长；完全相同时优先级为：

```text
io_wait > d_sleep > runnable > s_sleep > running
```

再相同则较早出现者优先。D-state 带 IO 证据时根因类型归 IO；否则保留 D-state。

### 3.3 三种量必须分开

对同一候选，至少存在三种不同量：

- `actual_*`：底层原始调度段的实际范围，可超出选中窗口。
- `projected_*` / `cumulative_impact_ms`：原始段投影到目标/链窗口后的量。
- `effective_impact_ms`：按候选类型和归因口径换算出的排名量。

除显式 residual fallback 外，`effective_impact_ms` 由类型闭表决定，不能把 `cumulative_impact_ms` 当作通用默认值。

## 4. 唤醒链构造规则

### 4.1 目标分支候选

`BuildWakeupChain` 先从目标 timeline 找满足 `MinDurationMs` 的状态段：

- 默认排除 `running`；只有没有任何非 running 候选时才使用 running。
- 主排序为时长降序。
- 时长差小于等于约 `0.050ms` 时，状态优先级为 `IO > D > S > runnable > running`。
- 目标层最多取 `MaxBranches` 个分支。
- 指定 `via_thread` 时，系统可把本来因 branch cap 被丢弃、但包含该线程的子树重新纳入；最终只保留包含 via 的子树。

### 4.2 什么条件才生成一条唤醒边

只有以下条件同时满足时才生成真实边并递归到 waker：

1. 当前段是 `S` sleep；
2. 在 `[segment.start, segment.end + 5µs]` 内找到匹配线程的唤醒事件；
3. 优先使用 `sched_wakeup`，只有整个候选面没有它时才回退 `sched_waking`；
4. `sched_wakeup_new` 明确不作为该边的权威来源；
5. 取匹配范围内最新的唤醒事件；
6. 上游递归窗口为 `[sleep.start, wakeup.ts]`。

边记录 branch、segment ordinal、wakeup latency、优先级等身份。不同 path/branch 是独立依赖链，不能在消费侧拼成一条虚构的线性链。

### 4.3 D/IO 与其他状态如何收束

- D-state / IO wait **不会**仅因看到 raw wakeup 就生成因果边，也不会沿 waker 递归。
- 它们优先从 `sched_blocked_reason` 形成根证据。
- runnable、D、IO、running 作为叶根收束。
- S-sleep 找不到匹配 wakeup 时形成 `missing_wakeup`。
- 窗口没有任何 timeline 段形成 `trace_gap(no_sched_data)`。
- 有 timeline，但所有段均低于门槛形成 `trace_gap(no_eligible_wait)`。
- gap 按 thread + kind 去重。

这是一个有意的非对称设计：raw D/IO wakeup 可以进入独立 census，但不能自动获得“谁唤醒谁”的完整因果资格。

### 4.4 深度、分支和全局预算

- 当 `depth >= MaxDepth` 时停止继续展开。
- 递归周期按 PID 防环。
- 每个节点的 top-1 段是 guaranteed tier，不受 `MaxChainNodes` 的 extra 预算阻断。
- 同节点额外段只允许 S-sleep；extra floor 为 `max(MinDurationMs, 1ms)`。
- 每节点最多增加 `MaxBranches - 1` 个额外段。
- 全局 extra frontier 按 `duration desc, start asc, registration seq asc` 调度。
- `MaxChainNodes` 只限制 extra 扩展，不截断 guaranteed top-1。
- 因深度、分支或预算未展开的部分必须附 caveat。

因此 `MaxChainNodes=1` 也不代表整棵链只会有一个节点；它限制的是附加分支预算。

### 4.5 Wakeup edge census

链构造之外还有一张全窗唤醒边普查：

- 线程集合为目标线程加已扩展节点；
- 全窗扫描 raw `sched_wakeup`，全无时才回退 `sched_waking`；
- 按 `sleep_exit`、`d_exit`、`other` 分桶；
- pair 展示 cap 为 16，目标线程 pair 受保护；
- 超额明确披露。

它是“发生过哪些唤醒关系”的普查，不应被解释成已满足链递归条件的因果路径。

### 4.6 `via_thread` 的路径判定

系统另做一次从 via 到目标的时间单调 hop 搜索：

- BFS，只接受唤醒时间非递减的边；
- 优先最短完整路径；
- 找不到时回退为最早贪心前缀并附 caveat。

但是当前 `OnChain` 的赋值更宽：只要 via 出现在 `WakeupChain.Nodes` 就可为 true。于是可能同时出现“via 在链上”和“没有完整单调路径”两种输出。见缺口 G5。

## 5. 因果影响量的计算

### 5.1 单节点投影账

每个链节点同时维护：

- 投影到链窗口的 running/runnable/S/D/IO 分项；
- 原始 actual lane ledger；
- dominant state；
- priority relation；
- supply fold；
- 周期源与聚合信息。

一般状态的 projected impact 取 dominant lane；D/IO 特例按两者的时间并集记账，避免同一阻塞段重复累计。

### 5.2 优先级反转与 gated impact

优先级关系只有在 RUNNABLE/RUNNING 闭区间端点稳定时才进入量化。仅在单点附近找到的优先级只用于展示，不授权反转归因。

反转成立需同时满足：

```text
硬 lower-priority 关系
AND gated_impact_ms > 0
AND dominant_state ∈ {runnable, running}
```

其中：

```text
gated impact
  = 已证明 lower-priority 区间内的 runnable 全量
  + 同区间 running 的 supply deficit 折算量
```

consumer roster 只用于“是否有消费关系”的武装，不直接增加 gated 数值。

### 5.3 running 的 supply fold

running 不按运行时长全额视为可归因。系统使用全频曲线和全局最大核频率计算供给缺口：

```text
supply_deficit_ms = running_ms × max(0, 1 - observed_freq / reference_max_freq)
```

实际实现按可获得的完整频率/核拓扑信息折算；频率未知时 ratio 退为 1，因此缺口为 0 的下界，而不是猜一个损失。

### 5.4 周期源折算

周期聚合只在以下条件成立时启用：

- 聚合是 sleep dominant；
- occurrence 至少 5 个；
- 从相邻 occurrence gap 推导候选周期；
- 对 gap 做整数倍 carve 后取 robust lower median；
- 允许约 15% 的 in-band 误差；
- 存在 early veto；
- 至少 2/3 的 gap 落在 band 内。

每次 occurrence 的 lateness 为：

```text
lateness_i = max(0, target_blocked_i - detected_period)
```

周期有效量为 runnable 与 lateness 的组合，并以 raw occurrence 量为上限；纯节拍等待可以得到 `0ms` 有效归因。周期源名字本身不参与公式。

这一规则能避免把正常周期等待全算成损失，但其 15% 节拍识别属于启发式信号，结果却直接进入硬排序，见 G1。

### 5.5 单节点 effective impact 闭表

`WakeupCausalImpactEffectiveImpactMs` 的现行口径为：

| 类型/状态 | effective impact |
|---|---:|
| periodic source | 周期折算后的 effective，允许为 0 |
| priority inversion | gated impact |
| running | supply deficit |
| runnable | runnable ms |
| D / IO | D 与 IO 的并集量 |
| S-sleep | 0 |
| unknown/其他闭表态 | 0 |

旧注释里“其他状态回退 TotalMs”的表述已过期；代码和工具最终说明采用的是上述闭表。

### 5.6 聚合影响

重复分支按 `(PID, dominant state)` 聚合，至少两个 occurrence：

- 完整可比较 occurrence 使用区间并集；
- 只有部分可比证据时使用最大 cohort；
- 明确互不相交时求和；
- occurrence windows 只展示最多 8 个，但 `count` 是全量；
- 聚合结果列表展示 top 8，不改变完整候选 census；
- overflow synthetic row 取单条最大值，不跨线程求和；
- 已有 inbound edge 的中间 S 聚合通常抑制，周期源可绕过。

聚合 effective impact 与单节点使用同一闭表。

## 6. 根因候选、席位与排序

### 6.1 候选来源

根因榜综合但不限于以下来源：

- wakeup causal impact / aggregated impact / root evidence；
- target 与链线程的 runnable、running、S、D、IO 状态；
- priority inversion、CPU pressure、compute supply、低频/频率限制；
- CPU affinity、cpuset、迁移限制；
- sched_stat、state churn、scheduler latency；
- binder/lock/blocking span；
- block/MMC/SCSI/F2FS/file IO、inode、page cache、IO pressure/episode；
- IRQ、softirq、IPI、workqueue、DMA fence；
- JIT、class verification、shader/runtime compilation、texture upload、GC 等 semantic span；
- perf context 作为运行代码位置的支持证据。

perf sample 单独不构成调度根因；它为 running、链依赖、同行竞争者或 semantic work 提供代码执行佐证。

### 6.2 一因一席与交叉折叠

候选不会直接全部入榜。当前主要去重/归并规则包括：

- aggregate 席位抑制已被它完整代表的 member；
- causal impact 抑制同事实的 RootEvidence twin；
- 目标 formal window state 抑制相同 target RootEvidence；
- D/IO 有互斥与 mutation invariant；
- semantic 同线程、同类型 family 可按 `sum_disjoint`、`interval_union`、`max_overlap_fallback` 或 `count_sum` 合并；
- runnable aggregate 与 inversion gated share 做 level merge，保留精确对账；
- adjacent IO facet 进入独立 family reconciliation；
- chain state 可按锚区间拆成 anchored on-chain 与 adjacent remainder；
- edge host 可拆成 pre/post 区间，避免整窗都冒充链上。

目标自己的 `sleep_wait`、`fragmented_sleep_wait`、`missing_wakeup`、`binder_wait`、`blocking_span` 属于 wait symptom family：它们标为 `target_self_state`、`rank=0`，用于解释症状而不占根因名次。目标自己的 runnable、D、IO 则可作为可分解状态进入竞争；running 只有 supply deficit 为正才进入。

### 6.3 根因榜 effective impact 闭表

`rootCauseEffectiveImpactMsUncapped` 比链节点闭表更完整：

| 候选族 | authoritative effective |
|---|---:|
| periodic | 周期折算量 |
| semantic measured work | 语义工作量 |
| inversion | gated impact |
| folded family | family 已发布合并量 |
| running | supply deficit；不得回退 running total |
| runnable | runnable ms |
| D / IO | D 与 IO 并集量 |
| fragmented sleep / missing wakeup /普通 sleep | 0 |
| trace gap / state churn / trace span | 0 |
| IO/CPU/supply pressure、freq limit、sched_stat、IRQ/IPI 等上下文族 | 0，除非落入其他明确测量臂 |
| count/composite caliber | 不参加 ordinal 排名 |
| 未命中任何闭表臂的 residual row | 仅此处可回退 cumulative impact |

背景候选还受发布上限：

```text
background_cap_ms = max(0.35 × window_ms, 0.1ms)
effective_impact_ms = min(raw_effective, background_cap_ms)
```

### 6.4 链路分层

排序前先定 channel：

1. `on_chain`
2. `adjacent`
3. `background`
4. unknown

同 channel 内：

1. `effective_impact_ms` 降序；
2. 完全同值的链上项，已解析 blocking peer 优先于 inherited-target；
3. `score` 降序；
4. raw impact 降序；
5. source line 起点升序。

因此排名的第一关键字不是 `score`，更不是 branch number，而是 channel 和 effective impact。

### 6.5 Score 的作用

概念上：

```text
score ≈ effective_basis × confidence × type_weight
```

典型 type weight：

- priority inversion：1.35
- IO / D-state / binder：1.25
- runnable / CPU pressure / scheduler：1.15
- 其他类型按各自权重。

semantic boost 也只在 effective 同值时生效。Score 是 tie-breaker，不能把更小的 effective 提到更大的 effective 前面。

### 6.6 tier、ordinal 与容量

- `trace_gap`：`data_gap`，rank 0。
- pacing/periodic idle context：context，rank 0。
- target wait symptom：`target_self_state`，rank 0。
- chain/adjacent 的 count/composite：caliber，rank 0。
- effective <= 0：context，rank 0。
- off-chain semantic/background：无普通 ordinal；可有内部 `BackgroundRank`。
- 有效 on-chain/adjacent 候选按同一 election position 获得 primary/secondary/tertiary tier。
- 但 `Rank` ordinal 在 channel 内分别编号。

这会出现“adjacent #1，但 tier=tertiary”的合法组合：前面的全局 tier 位已被 on-chain 项占用。实现自洽，但用户语义容易误解，见 G10。

主榜 cap 为 12。cap survivor 先按 channel、再按 effective 选出，最后按原排序重新发布；只要还有非链项占位，正 effective 的链上项不应被 cap 杀死。

独立 side lanes：

| 侧栏 | cap |
|---|---:|
| rank-0 disclosure | 4（target/gap 分配） |
| caliber | 4 |
| remainder | 8 |
| demoted | 8 |
| cap-dead target-self | 6 |

容量死亡和 overflow 都应带披露。它们属于展示/传输限制，不应改变前置 full census 的事实。

### 6.7 修复方向与守恒检查

每个可排名席位可带一个 typed `FixDirection`。进入严格方向守恒检查的席位需满足：

- rank > 0 且 effective > 0；
- on-chain；
- basis 在允许集合；
- 未 demote、未 absorbed；
- PID 已知；
- 支持区间是该线程的 wall-clock 区间；
- 非 caliber；
- 修复方向已解析；
- 有精确 support inventory。

同一线程、跨方向的支持区间做对称重叠检查。重叠少于较小 effective 的 5% 时会被压成未披露 token；达到 5% 则标“待追认”。同方向按 `(pid, direction)` 合并每席支持区间，仅当至少 2 席且和超过 `window + 0.001ms` 才报守恒违例。

该检查只是符合条件席位的披露性审计，不是所有根因的全量数学证明。

## 7. 因果投影的计算与构造

### 7.1 从引擎结果到 typed observation

`internal/tool/trace_query.go` 把引擎结果转换为 `ObservationRecord`，保留：

- path/branch/depth/segment；
- subject/object/thread/PID；
- source artifact、line、window；
- actual/projected/cumulative/effective；
- dominant state 与各 lane；
- rank/tier/channel/score；
- gated/supply/priority/direction；
- RSPA、family、aggregation、overflow、caveat；
- P3 measurement 四字段（但后续禁止消费）。

### 7.2 路径选举

投影编译器先收集 wakeup path：

- branch-form path 优先于 legacy path；
- 如果用户指定实体，优先选择含该实体且有 frame corroboration 的 path；
- 用户实体可匹配 path 任意位置；
- 命中后把 path 截到该用户实体；
- 无 typed match 时按发布顺序回退；
- branch + window identity 用于把后续节点挂到正确的树，避免跨分支串接。

### 7.3 Bucket

| Bucket | 构造阶段容量 |
|---|---:|
| Primary | 预聚合 cap 10 |
| OnChain | 先不截，后聚合 cap 24 + fold |
| Adjacent | 后聚合 cap 8 + fold |
| Background | 后聚合 cap 8 + fold |
| Semantic | on-chain 不限；off-chain cap 16 |
| Supporting hops | 后聚合 cap 10 + fold |

这些 bucket 是答案投影结构，不等价于根因引擎的主榜 cap 12。

### 7.4 去重和合并顺序

当前流水线大致按以下顺序：

1. 搬迁 absorbed rows；
2. R1 严格同事实折叠：subject、显示到 3 位的小数、精确行号等一致；
3. raw twin convergence；
4. R4 peer alias；
5. V4 duplicate publication：exact 或 near；
6. 零时长 instant marker fold；
7. R2 同 `(subject, object)` 三条及以上聚合；
8. raw member reissue / twin convergence；
9. unknown background fold；
10. semantic span seat unification；
11. 重新排序；
12. bucket cap/fold；
13. attachments 与 anchor window。

V4 的 near 判定允许同 identity 且行/时间范围重叠、数值相差不超过约 3% 的发布被折叠，并取最大值。它会改变结构与值，不只是加 caveat，见 G4。

### 7.5 R2 聚合口径

- 同 subject/object 至少 3 条才进入 R2。
- 不同窗口且 occurrence interval 可比时，按区间并集。
- 查询窗口互相重叠但无法精确求并集时，使用最大值作为下界。
- 同一窗口内的多个 occurrence 默认允许求和；不会仅因同窗就假设重叠。
- ×N 记录保留 count、显示值、极值、窗口和 rank-0 family 证据。

### 7.6 Anchor window

优先级为：

1. frame target resolution 的 query window / 显式窗口并集；
2. selected window；
3. wakeup impact / aggregate / root-cause 记录上的窗口回退。

当前多个解析器和 `WindowDurationMS` 以 `start > 0` 作为“窗口存在”的判定。trace 恰好从时间戳 0 开始时，这个合法窗口会被当成缺失并降级，见 G8。

## 8. “窗内可消除量”当前到底怎么算

### 8.1 入榜人口

只有 observation 中出现过 root-cause family 才构造该榜。候选需同时满足：

- 是 root-cause rank item 或被 fold 的等价 peer；
- `Rank > 0`，或 semantic 已被正式采纳；
- channel 为 on-chain 或 adjacent；
- 非 caliber/count/composite；
- effective impact > 0；
- 非 data gap；
- board/window 有效；
- 通过微量与重复折叠。

随后还会排除或折叠：

- 已被 chain seat 表示的重复项；
- semantic member subset；
- gated constituent；
- inversion 与 runnable 的精确双重发布：同 subject/channel/effective 在微秒级一致，且 running deficit 为 0、gated runnable 等于 runnable 时，仅保留 inversion。

### 8.2 排列和展示

- 全部 chain 席位先于 adjacent 席位。
- 各 channel 内 effective 降序，再按 home order / source line。
- 正文显示 top 5。
- 另保留最大 off-board adjacent，并在需要时保留最大 off-board chain semantic 作为披露。
- 条形图的满刻度取整个 eligible board 的最大值，不只是 top 5 最大值。
- 被切掉的链上、邻接、semantic 条数分别披露。

### 8.3 修复方向分组

链上项按 `FixDirection` 分组；未解析方向放尾部。组排序依据组内最大项，不是组内总和。这样避免在尚未证明可相加时用总量抢占版面。

### 8.4 什么时候允许算小计

同方向组只有满足下列条件才显示算术 ladder：

- 至少 2 项；
- 修复方向已解析；
- 所有项属于同一 board；
- 每项有有效 start/end；
- `MergedCount <= 1`；
- 任意两项的 envelope overlap 不超过 `0.001ms`。

全部互斥时才打印：

```text
方向小计 = 各已打印项的微秒值之和
```

存在重叠则明确“不可相加”；缺窗口、跨 board、已合并项则不出小计。这个小计是保守展示算术，不是重新跑出的反事实模拟。

### 8.5 当前数值的真实语义

当前榜值基本等于各根因席位的 `effective_impact_ms`：

- chain：实现文案倾向称“已证明可消除”；
- adjacent：明确是条件性上界；
- background：不进入该榜。

但 P3 counterfactual measurement 的：

- `P3MCounterfactualValidMs`
- `P3MCounterfactualInvalidMs`
- `P3MEdgeWitnessedMs`
- `P3MDisposition`

在排名完成后才静默盖章，并由结构测试强制为 `display_only`：仓库其他消费者不得读取这些键或字段。当前只对 periodic-pinned 形态给出 valid/invalid，late-relay 明确不测，target self 只有 disposition 而无数值。

所以严谨定义应是：

> 当前“窗内可消除量”是**经类型口径折算、并通过根因席位准入的窗内优化归因量**；它不是对“实施某个修复后目标延迟必然减少多少”的完整反事实估计。

## 9. 容量、截断与“不等于漏算”的边界

| 层 | 容量/阈值 | 是否改变底层事实 |
|---|---:|---|
| target branch | `MaxBranches` | 改变展开面；via 可追回特定子树 |
| extra segment | `MaxBranches-1` / node | 改变附加展开面 |
| chain extra | `MaxChainNodes` | 只限制 extra；不杀 guaranteed top-1 |
| edge census pairs | 16 | 只截展示，带 overflow |
| occurrence windows | 8 | count 保持全量 |
| aggregate view | 8 | full census 仍用于 rank |
| root main board | 12 | 影响发布席位；有 cap-death 披露 |
| projection on-chain | 24 | 投影层 fold/cap |
| projection adjacent/background | 各 8 | 投影层 fold/cap |
| elimination board | top 5 | 仅答案页展示，带 cut 披露 |

因此审计时要区分三种情况：

1. **因证据不足而没有构造事实**，例如 S 段缺 wakeup；
2. **因设计门槛没有授权为根因**，例如普通 sleep effective=0；
3. **事实存在但受展示容量截断**，例如 root board 第 13 项或 elimination top 5 之外。

## 10. 矛盾与覆盖缺口审计

### 10.1 高优先级

| ID | 问题 | 代码事实 | 风险 | 建议 |
|---|---|---|---|---|
| G1 | 周期启发式进入硬排名 | 15% 容差、2/3 in-band、robust median 的周期判定直接改变 effective、tier、ordinal 和 cap 生死 | 与“噪声信号只做软指导”原则冲突；边界抖动可改写根因 | 周期性先降为 typed advisory；只有明确 producer/deadline 或 schema 化节拍事实才授权折算，或把 heuristic effective 独立为非硬排名列 |
| G2 | “链上已证明可消除”措辞过强 | 根因可通过 `ChainIdentityInheritance` 或 `ChainCredentialEnvelopeLevel` 保持 on-chain，并非每席都有 segment-level 唤醒凭证 | 把线程身份/包络归因说成可消除证明 | 可消除榜只纳入 exact segment credential；其余改名“链路归因估计”或降为 adjacent/context |
| G3 | P3 反事实量完全不可见且覆盖极窄 | 四字段在最终排名后计算，结构测试禁止任何消费者；只测 periodic-pinned，不测 late relay | 产品展示仍只能用 effective 代替 counterfactual，无法验证真实节省 | 定义新的、经用户裁定的消费契约；先覆盖 exact edge + disjoint support，再逐类扩展，不能直接解除 display-only |
| G4 | 3% near publication fold 是硬结构决策 | 相同 identity、行/时间相交、值差 <=3% 时取 max 并折叠 | 相似度/数值近似是噪声门，却会删除席位和改值 | 只有稳定 publication ID 或精确值/区间关系才硬折叠；near 只标“疑似重复”并保留歧义披露 |

### 10.2 中优先级

| ID | 问题 | 代码事实 | 风险 | 建议 |
|---|---|---|---|---|
| G5 | `via_thread.OnChain` 与完整路径语义矛盾 | 节点集合包含 via 即可 true；完整非递减路径可能仍不存在 | 用户会把“参与过某个扩展节点”误读为“有完整因果链” | 拆成 `present_in_expanded_nodes` 与 `has_complete_monotonic_path`；`OnChain` 只绑定后者 |
| G6 | D/IO 因果链在 blocked reason 处收束 | raw wakeup 只进 census，不递归 D/IO waker | D/IO 的上游设备/线程链不能由 wakeup graph 完成 | 保持“不伪造边”，另建 typed blocked-reason/resource causality lane，并明确不是 scheduler wakeup edge |
| G7 | 非目标中间节点的多分支覆盖偏 S | guaranteed top-1 后，extra 只允许 S-sleep | 中间线程重复 runnable/D/IO 段不会形成并行上游解释 | 若业务需要，给 D/IO/runnable 独立、精确且低预算的 branch lane；仍不得混同 wakeup edge |
| G8 | 时间戳 0 被当作“无窗口” | 多处以 `start > 0` 判存在 | 从 0 起录制的合法 trace 丢 anchor、百分比或算术资格 | 使用显式 `HasWindow`/valid enum，不用 0 作为 sentinel |
| G9 | 方向守恒“通过”措辞覆盖过度 | 检查只覆盖 rank>0、正 effective、链上、有方向和精确 inventory 的子集 | “每个方向均未超窗”会被理解成全榜审计 | 文案改为“所有**可审计席位**未发现超窗”，并显示 eligible/total |
| G10 | tier 与 ordinal 使用两套位置空间 | tier 在 on-chain+adjacent 全局选举；Rank 在各 channel 分别编号 | “邻接第 1”同时显示 tertiary，难以直觉解释 | 对外显示 `channel_rank` + `global_tier`，避免都叫排名；或统一一个公开位置空间 |

### 10.3 低优先级与文档漂移

| ID | 问题 | 当前情况 | 建议 |
|---|---|---|---|
| G11 | `WakeupCausalImpactEffectiveImpactMs` 注释陈旧 | 注释仍暗示某些非闭表态回退 TotalMs，代码实际返回 0 | 修正注释并用闭表测试名引用 |
| G12 | gated reason 字段注释陈旧 | struct 注释称不存在 frequency reason，但字段已存在且写入 | 更新字段契约说明 |
| G13 | 方向交叠 5% 边界仍写“待追认” | 小于 5% 会影响披露，大于等于 5% 才展示待追认 | 在规则落地前不要让该阈值触发不可逆席位合并；明确它当前只控制披露 |

### 10.4 已核对、不是矛盾的点

- 工具说明源码的大段原始字符串中仍能搜到旧说法，但 `traceQueryDescriptionPostRules` 在发布前已把 effective impact 规则改成闭表；最终 golden 是正确的，不能据 raw literal 判定线上契约冲突。
- chain-first、adjacent-second、background-last 与 score tie-breaker 是刻意设计，不是“score 排错”。
- 主榜 cap 12、projection cap、elim top 5 是不同层级的容量；数值不同不是同一常量漂移。
- D/IO 不沿 raw wakeup 递归是因果保守策略，不应简单改成 S-sleep 同逻辑。
- 同方向小计拒绝重叠项相加是保守规则，不是漏算；真正缺的是反事实口径，而不是更激进地求和。

## 11. 建议的修复顺序

### P0：先修语义和硬门

1. 把 G1 周期启发式从硬排名授权中剥离，或补充精确 typed producer/deadline 证据门。
2. 把 G4 near dedup 改为软歧义披露；硬折叠只读稳定 identity/精确关系。
3. 把 G2 的链上凭证分级暴露到可消除榜，立即收紧“已证明可消除”的文案。

### P1：建立真正可消费的反事实契约

1. 先定义 `effective attribution`、`support-covered eliminable`、`counterfactual saved` 三列，禁止混名。
2. 只对 exact edge、exact support interval、同 board、无重叠的席位开放第一批 counterfactual 消费。
3. 为 invalid/unmeasured 给出 typed reason，覆盖 late-relay、merged occurrence、identity inheritance。
4. 用守恒和双路径一致性测试约束，而不是让答案层自行推断。

### P2：补路径和覆盖表达

1. 修 G5 via 两字段语义。
2. 用专门资源因果 lane 承接 G6 D/IO 上游。
3. 评估 G7 中间非 S 分支的真实用户需求和预算。
4. 修 G8 零时间戳 sentinel、G9/G10 展示术语。

### P3：清理漂移

修复 G11/G12 注释，给每张闭表和 cap 增加单一权威文档链接，避免工具说明、struct comment、答案文案分别维护一套口径。

## 12. 建议新增的验收测试

1. **周期边界变形测试**：只改变一个 gap 使其跨过 15% band，验证若无 typed producer 证据则 ordinal/cap 不变。
2. **near dedup 对抗测试**：2.9% 和 3.1% 两组相似发布不能导致根因席位静默消失。
3. **via 完整路径测试**：via 出现在节点集中、但不存在时间非递减路径时，`has_complete_monotonic_path=false` 且不发布强 `on_chain`。
4. **继承链席位可消除准入测试**：identity/envelope-only 席位不得进入“proved eliminable”人口。
5. **P3 消费契约测试**：只有 exact-support 白名单消费者能读 counterfactual；其他层仍 fail closed。
6. **timestamp-zero 测试**：`[0, end]` 能保留 anchor、window duration、within 和 elimination arithmetic 资格。
7. **守恒覆盖文案测试**：同时显示 eligible/total，不得把部分人口的 pass 写成全量 pass。
8. **tier/ordinal 展示测试**：adjacent channel rank 与 global tier 用不同字段和词面。

## 13. 代码索引

| 主题 | 主要实现 |
|---|---|
| 查询编排、缓存、窗口/链/榜顺序 | `internal/tracequery/query.go` |
| 状态全集与 dominant 规则 | `internal/tracequery/thread_state_universe.go` |
| 唤醒链、影响、聚合与根证据 | `internal/tracequery/query.go`（`BuildWakeupChain`、`expandChain*`、effective/rank 主体）及同包 wakeup/aggregate 文件 |
| raw wakeup census | `internal/tracequery/wakeup_edge_census.go` |
| 链锚分区 | `internal/tracequery/rank_chain_anchor_rspa.go` |
| level merge / family reconciliation | `internal/tracequery/rank_levelmerge_split.go`、`rank_family_*.go` |
| 根因榜容量 | `internal/tracequery/root_cause_rank_capacity.go` |
| 修复方向守恒 | `internal/tracequery/rank_direction_axiom.go` |
| P3 counterfactual measurement | `internal/tracequery/rank_p3_measure.go` |
| trace_query typed observation | `internal/tool/trace_query.go` |
| 因果投影主体 | `internal/types/trace_causal_projection.go` |
| 投影聚合/区间/一因一席 | `internal/types/trace_causal_projection_aggregate.go`、`trace_causal_projection_interval.go`、`trace_causal_projection_oneseat.go` |
| 可消除榜 | `internal/tool/answer_document_mutation_runtime_elim.go` |
| 最终工具契约修订 | `internal/tool/trace_query.go`（`traceQueryApplyRootCauseClosedMatrixContract`） |

## 14. 对外口径建议

在代码完成上述修复前，对用户最准确的表述应是：

> 根因榜先按真实链路相关性分层，再按类型折算后的窗内有效归因排序；普通 sleep、gap 和无供给缺口的 running 不靠原始时长抢占名次。唤醒边只由受支持的 S-sleep 唤醒事件建立。可消除榜展示的是链上或邻接根因的优化归因量，其中邻接项是条件性上界；除明确具有反事实测量凭证的项外，不应承诺这些毫秒会一比一转化为修复后的延迟下降。
