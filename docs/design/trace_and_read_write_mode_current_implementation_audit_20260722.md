# Trace 分析规则与读写模式系统缺口审计

> 基线：`main` @ `ca644b94b04f4ffbc211d26a2a43a8b37dbc4f50`（2026-07-22）
>
> 范围：当前代码中 trace 输入、窗口、线程状态、唤醒链、因果影响、根因排序、因果投影、答案页“窗内可消除量”的完整实现，以及 read/write 两种运行模式的隔离、恢复、审批、worktree、验证和持久化边界。
>
> 方法：以 Go 实现、typed schema、结构测试和行为测试为准；历史设计文档仅用于发现漂移，不把旧方案当作现行规则。

## 1. 结论先行

当前 trace 分析实际维护四套相关但不可混用的账：

1. **调度事实账**：窗口内 running、runnable、S、D、iowait 等实际或投影时长。
2. **唤醒因果账**：只对满足条件的 S-sleep 使用 `sched_wakeup` / `sched_waking` 建边；D/IO 不伪造 scheduler wakeup 因果。
3. **根因归因账**：用类型闭表把事实量换成 `effective_impact_ms`，先分 on-chain / adjacent / background，再按有效量排序。
4. **优化提醒账**：答案页从已经发布的正向席位生成五区“窗内可消除量”面板；其主体数值仍是 effective attribution，不是完整的修复后反事实节省量。

本轮相对 `686ba0941` 的主要变化是：

- **时间戳 0 的合法窗口已修复**：窗口存在性有显式 `StartSet` / present 语义，不再普遍用 `start > 0` 判断。
- **`via_thread` 新增 `PathComplete`**：现在能区分“线程出现在扩展节点中”和“存在完整时间单调路径”，但旧 `OnChain` 仍采用较宽的节点成员语义。
- **新增链凭证普查**：所有正 rank 链席位必须获得 `wakeup_anchored`、`target_self`、`interval_proven`、`member_inherited` 之一；`none` 会被降为 background。
- **窗内可消除量改成五区面板**：链上修复方向 TOP5、业务线索 TOP8、邻接 TOP3、背景 TOP3、辅助对账；链上和邻接各自使用局部标尺。
- **CPU supply fold 显著扩充**：加入显式拓扑、变频共振推导、keyed rail、核能力系数、同簇 donor、频率/limit/thermal 来源和完整披露。

仍然最重要的 trace 问题是：

- 周期源用 15% 容差等启发式信号改变 effective、排序和容量生死，与“噪声信号只做软指导”的架构原则冲突。
- 3% near duplicate 会直接折叠席位并取最大值，也是噪声信号驱动硬结构变更。
- 链凭证普查提升了可审计性，但 `member_inherited`、`target_self`、区间包络均能授权链席位；答案页仍把链上席位统称为“proven eliminable”，证明强度不够。
- P3 counterfactual 仍是 `display_only` 且只覆盖 periodic-pinned；用户和模型都不能消费它，因而无法支撑上述强措辞。
- D/IO 和中间非 S 多分支仍缺少完整的上游资源因果表达。

读写模式审计发现两个高优先级系统缺口：

1. **写工作流续跑没有仓库身份门**：持久化的 `WriteWorkflowRun` 不记录 repo root、base HEAD/branch、repo fingerprint 或请求 fingerprint；`FindActiveRun` 直接取 planDir 下最新的 active run，controller 可在另一个 `--repo` 上自动续跑旧任务。
2. **读写共用的“只读 shell”不是能力安全边界**：它允许 `awk`、`sed`、`git branch` 等命令，但不验证完整参数语义；例如 `awk` 的 `system()` 和部分 git/sed 参数可绕过“只读”假设。除 `cd` 和少数 git path option 外，普通读命令也没有禁止绝对路径或 `../`，与“只在当前仓库内”不一致。

## 2. 审计边界和术语

本文枚举 trace 根因分析路径中的全部决策规则、公式、准入、折叠、排序、容量和显示规则，并列出主要事件证据族。它不逐字复述每一种厂商 trace event 的 payload grammar；解析器字段语法应继续以各 decoder 和 golden test 为权威。

本文使用以下术语：

| 术语 | 含义 |
|---|---|
| actual | 原始事件或调度段的真实区间，可越出查询窗 |
| projected | actual 与目标窗/链窗相交后的量 |
| cumulative | 节点或聚合在其口径下累计的投影量 |
| effective | 类型闭表授权进入根因排序的归因量 |
| counterfactual | “实施修复后会减少多少”的反事实量；当前产品路径没有完整可消费实现 |
| seat | 经过一因一席、family fold、level merge 等规则后的发布单位 |
| channel | `on_chain`、`adjacent`、`background` 等排序分区 |
| board | 同一查询目标和查询窗口生成的一张根因榜 |

## 3. Trace 端到端数据流

```text
trace input admission / generation / integrity
  -> Query.Run
     -> normalize query + resolve target/window
     -> timeline + window stats
     -> BuildWakeupChain
        -> target interval candidates
        -> S-sleep wakeup edges
        -> branches / guaranteed nodes / extra frontier
        -> causal impact / aggregate / root evidence
        -> edge census / via report
     -> BuildRootCauseRank
        -> source census
        -> one-seat / family fold / level reconciliation
        -> effective closed matrix
        -> channel, score tie-break, tier, ordinal, cap
        -> direction inventory and chain credential census
        -> P3 display-only measurement
     -> trace_query typed ObservationRecord
     -> CompileTraceCausalProjection
        -> path election
        -> exact/near duplicate convergence
        -> R2 aggregation
        -> bucket caps/folds
        -> anchor window
     -> answer document runtime projection
        -> five-zone elimination/optimization board
        -> overlap, conservation, auxiliary reconciliation
```

`Query.Run` 会缓存窗口统计、latency、chain 和 rank。排名侧先得到 chain，再用链锚定范围做 stats sweep，避免先以全窗统计替代因果窗。取消发生时不发布半成品结果。

## 4. 输入、窗口、身份与完整性

### 4.1 输入准入

- 空输入、二进制输入和输入生成被替换等情况 fail closed。
- 文件输入持有 descriptor/generation 身份，解析期间检测被替换或变化。
- scheduler lane 的顺序或完整性异常不会静默拼成可信时间线。
- PID/TID incarnation 无法可靠确认时，不跨生命周期拼接同号线程。
- composite trace 必须先建立可解释的时钟映射；无法对齐的证据不能伪装成同一 wall-clock。
- perf sample 只作为执行位置或代码上下文，不单独授权为调度根因。

### 4.2 查询窗口

- 窗口可来自显式 `time_start/time_end`、frame/span 解析或工具的 typed target resolution。
- 合法窗口现在由 `TimeStartSet`、backfilled 标志和 `TimeWindow.StartSet` 表示；显式或可靠回填的 `[0,end]` 是存在的窗口。
- 未设置 start、仅因零值落到 `0` 的 line-anchored query 仍视为缺失，避免把 schema 零值误判成真实时间零点。
- projection 使用 `TraceCausalProjectionWindowPresent` 传播窗口存在性；anchor 百分比、小计资格和窗口时长不再因真实 start=0 丢失。
- 多窗口显示名单有容量，但窗口数显示上限不等于引擎只分析这些窗口。

### 4.3 状态全集和 dominant

| 状态 | 是否进时长账 | 主要解释 |
|---|---:|---|
| `running` | 是 | CPU 执行；只有 supply deficit 可作为该状态的有效损失 |
| `runnable` | 是 | 就绪但未得到 CPU |
| `s_sleep` | 是 | 可中断睡眠；唯一可直接沿 scheduler wakeup 递归的阻塞态 |
| `d_sleep` | 是 | 不可中断睡眠 |
| `io_wait` | 是 | 明确 IO wait |
| `stopped` | 否 | 上下文 |
| `dead` | 否 | 上下文 |
| `unknown` | 否 | gap/context |

dominant 先比较时长。完全相同时使用：

```text
io_wait > d_sleep > runnable > s_sleep > running
```

仍相同则较早出现者优先。D-state 有 IO 证据时按 IO 族解释，否则保留 D-state。

## 5. 唤醒链构造

### 5.1 目标分支候选

`BuildWakeupChain` 从目标 timeline 选择满足 `MinDurationMs` 的状态段：

- 默认排除 running；只有不存在任何非 running 候选时才允许 running。
- 首关键字为 duration 降序。
- 时长差不超过约 `0.050ms` 时，状态优先级为 `IO > D > S > runnable > running`。
- 目标层最多取 `MaxBranches` 个分支。
- 指定 `via_thread` 时，可追回原本被 branch cap 丢弃但包含 via 的子树；最终只保留包含 via 的子树。

### 5.2 真实 wakeup edge 的六个条件

只有以下条件满足时才发布真实唤醒边并递归 waker：

1. 当前阻塞段为 S-sleep；
2. 在 `[segment.start, segment.end + 5µs]` 内存在目标线程匹配事件；
3. 优先使用 `sched_wakeup`；
4. 仅当候选面没有 `sched_wakeup` 时才回退 `sched_waking`；
5. `sched_wakeup_new` 不作为该边权威来源；
6. 取范围内最新 wakeup，递归窗口为 `[sleep.start,wakeup.ts]`。

边保留 branch、segment ordinal、wakeup latency、waker/wakee、priority 等 typed 身份。不同 branch/path 不能在投影层拼成一条虚构线性链。

### 5.3 叶根和 gap

- D/IO 不因 raw wakeup 建 scheduler 因果边，也不沿 waker 递归；优先使用 `sched_blocked_reason` 形成根证据。
- runnable、D、IO、running 是叶根。
- S-sleep 没有匹配 wakeup：`missing_wakeup`。
- 窗口无 timeline：`trace_gap(no_sched_data)`。
- 有 timeline 但所有段低于门槛：`trace_gap(no_eligible_wait)`。
- gap 按 thread + kind 去重。

### 5.4 深度、分支和 guaranteed/extra

- `depth >= MaxDepth` 停止展开。
- 递归按 PID 防环。
- 每个节点的 top-1 段属于 guaranteed tier，不受 `MaxChainNodes` extra 预算阻断。
- 同节点 extra 只允许 S-sleep。
- extra floor 为 `max(MinDurationMs,1ms)`。
- 每节点额外段最多 `MaxBranches-1`。
- 全局 extra frontier 按 `duration desc,start asc,registration seq asc`。
- `MaxChainNodes` 只限制 extra，不截 guaranteed top-1。
- 因深度、分支或预算未展开的部分必须有 caveat。

所以 `MaxChainNodes=1` 不代表整棵树只能有一个节点。

### 5.5 Wakeup edge census

链构造之外还有全窗普查：

- 人口是目标线程和已扩展节点。
- 全窗扫描 `sched_wakeup`；全无时才回退 `sched_waking`。
- 按 `sleep_exit`、`d_exit`、`other` 分桶。
- pair 展示 cap=16；目标相关 pair 受保护；超额披露。

它回答“发生过哪些 raw wakeup 关系”，不是“哪些关系满足链递归因果条件”。

### 5.6 `via_thread`

`viaMonotonicHops` 在 `(pid,lastTs)` 状态上 BFS：

- 只接受时间戳非递减的边；
- 优先最短完整路径；
- 没有完整路径时，回退为最早的非递减贪心前缀；
- `PathComplete` 表示是否真正到达目标。

当前 `OnChain` 仍只要求 via 出现在 `WakeupChain.Nodes`。因此：

```text
OnChain=true, PathComplete=false
```

是合法但容易误解的组合：它表示“在扩展节点集合中”，不是“存在完整 via→target 因果路径”。

## 6. 因果影响与 effective 计算

### 6.1 单节点时长

每个链节点维护：

- running/runnable/S/D/IO 的投影量；
- actual lane ledger；
- dominant state；
- priority relation；
- supply fold；
- periodic 和 aggregate 信息。

普通状态的 projected impact 取 dominant lane。D/IO 按两者时间并集计量，避免同一阻塞段重复。

### 6.2 Priority inversion

优先级关系只有在 RUNNABLE/RUNNING 闭区间端点稳定时才授权定量；仅单点附近的 priority 只展示。

反转席位需同时满足：

```text
strict lower-priority relation
AND gated_impact_ms > 0
AND dominant_state in {runnable,running}
```

量化为：

```text
gated_impact_ms
  = lower-priority interval 内的 runnable 全量
  + 同区间 running 的 supply deficit
```

consumer roster 只证明消费关系，不增加数值。

### 6.3 CPU cluster、能力和 donor

CPU→cluster 和能力折算的权威顺序为：

1. 显式 `CoreTopology`；
2. 全 trace 频率变化点的共振域；
3. 通过六门校验的 keyed rail；
4. 无法可靠分类时 `freq_only`。

共振域规则：

- 两 CPU 在 `15µs` 内发生相同值 transition：一票支持；
- 同期不同值 transition：一票反对，具有 veto 性；
- 仅首次 entry announcement 不投票；
- 推导以全 trace 为依据，每个 `Index` memoize 一次。

donor 规则：

- 自己有样本的 CPU 不借 donor。
- 只从同 domain 的已采样 CPU 借，确定性选择最低 CPU id。
- cluster label 保留拓扑原值，不强制压成三档。
- 除排他性的 `derived_prime` 外，不向最高已采样能力以上外推。

默认能力系数：

| class | coefficient |
|---|---:|
| small | 1.000 |
| middle | 2.300 |
| big | 2.530 |
| prime | 3.036 |

按全 trace fmax 对 cluster 排序：

- 2 簇：small、big；
- 3 簇：small、middle、big；
- 4 簇：small、middle、big、prime。

以下情况退为 `freq_only`，不猜 class：没有 domain、没有采样 cluster、只有一簇、超过四簇、无法消解 fmax 并列、共振证据不足。

fmax 汇总 limit、observed、rail 三路的最大值；相同值来源优先级为 `limit > observed > rail`。两簇 fmax 并列只运行一次确定性 tie-break 链；三簇及以上并列直接拒绝分类。

### 6.4 Running supply fold

每个 running slice 的理想执行量为：

```text
ideal_slice_ms
  = slice_ms
  × min(1,
      frequency(slice) × capability(slice_class)
      ------------------------------------------------
      reference_frequency × capability(reference_class))
```

整体：

```text
ideal_ms   = Σ ideal_slice_ms
deficit_ms = max(0, running_ms - ideal_ms)
```

频率治理：

- 使用窗口起点之前最近的样本作为 carry-in，再加所有窗内样本。
- 缺失频率的 slice 按 ratio=1，贡献 0 deficit；这是下界而不是猜损失。
- 同簇 donor 可补频率，并记录来源。
- thermal/limit 只做来源与限频披露，不额外改变已经按实际频率算出的 deficit。
- 只有窗内 limit/thermal 样本才能说原因被 witnessed；carry-in 只能说明状态沿用，原因未在窗内见证。

完整性披露包括被丢弃频率 lane、limits anchor mismatch、sample basis、cluster topology source、fmax source 和 donor。

### 6.5 周期源

周期聚合要求：

- sleep dominant；
- occurrence 至少 5；
- 相邻 occurrence gap 经整数倍 carve；
- 候选周期取 robust lower median；
- early veto 未触发；
- 至少 2/3 gap 在约 15% band 内。

每次迟到量：

```text
lateness_i = max(0,target_blocked_i - detected_period)
```

周期 effective 是 runnable 与 lateness 的组合，并受 raw occurrence 上限约束；纯节拍等待可折为 0。名称不参与公式。

### 6.6 Wakeup causal impact 闭表

| 类型/状态 | effective impact |
|---|---:|
| periodic source | 周期折算量，可为 0 |
| priority inversion | gated impact |
| running | supply deficit |
| runnable | runnable ms |
| D/IO | D 与 IO 的区间并集 |
| S-sleep | 0 |
| unknown/其他闭表态 | 0 |

普通 sleep 不因 raw 时长大就进入正向根因竞争。

### 6.7 聚合影响

重复分支按 `(PID,dominant state)` 聚合，至少两个 occurrence：

- 完整可比较：区间并集；
- 部分可比：最大 cohort；
- 明确互不相交：求和；
- occurrence window 只展示最多 8 个，但 count 保留全量；
- aggregate view top 8；
- overflow synthetic row 取单条最大值，不跨线程求和；
- 已有 inbound edge 的中间 S 通常抑制，periodic 可例外。

聚合 effective 使用同一闭表。

## 7. 根因候选、一因一席与排序

### 7.1 候选来源全集

根因候选来自以下 authority family：

- wakeup causal impact、aggregated impact、root evidence；
- target/chain 线程的 running、runnable、S、D、IO；
- priority inversion、CPU pressure、compute supply、frequency/limit；
- affinity、cpuset、迁移限制；
- sched_stat、state churn、scheduler latency；
- binder、lock、blocking span；
- block/MMC/SCSI/F2FS/file IO、inode、page cache、IO pressure/episode；
- IRQ、softirq、IPI、workqueue、DMA fence；
- JIT、class verification、shader/runtime compile、texture upload、GC 等 semantic span；
- perf context 作为代码位置支持证据。

perf sample 自身不形成独立 scheduling cause。

### 7.2 一因一席和 family fold

主要规则：

- aggregate 抑制被完整代表的 member；
- causal impact 抑制同事实 RootEvidence twin；
- target formal window state 抑制同 target RootEvidence；
- D/IO 受互斥和 mutation invariant；
- semantic 同线程同 family 可使用 `sum_disjoint`、`interval_union`、`max_overlap_fallback` 或 `count_sum`；
- runnable aggregate 与 inversion gated share 做 level merge；
- adjacent IO facet 使用独立 family reconciliation；
- chain state 按锚区间拆为 on-chain anchored 与 adjacent remainder；
- edge host 可拆 pre/post，避免整窗冒充链上；
- target wait symptom（sleep、fragmented sleep、missing wakeup、binder wait、blocking span）是 rank 0 的 `target_self_state`；
- target runnable/D/IO 可进入正向竞争；target running 仅在 supply deficit>0 时进入。

### 7.3 Root effective 闭表

| 候选族 | authoritative effective |
|---|---:|
| periodic | 周期折算 |
| measured semantic work | 已测语义工作量 |
| inversion | gated impact |
| folded family | family 已发布量 |
| running | supply deficit；不得回退 running total |
| runnable | runnable ms |
| D/IO | D/IO 并集 |
| fragmented sleep / missing wakeup / ordinary sleep | 0 |
| trace gap / state churn / trace span | 0 |
| IO/CPU/supply pressure、freq limit、sched_stat、IRQ/IPI 等上下文族 | 0，除非命中其他明确测量臂 |
| count/composite caliber | 不参与 ordinal |
| 未命中任何闭表臂的 residual | 唯一允许回退 cumulative 的情况 |

background 发布上限：

```text
background_cap_ms = max(0.35 × window_ms,0.1ms)
effective          = min(raw_effective,background_cap_ms)
```

### 7.4 Chain credential census

有链 board 上，每个已发布、positive-rank 的链席位必须得到一种凭证：

| credential | 条件 |
|---|---|
| `wakeup_anchored` | `OnChainBasis` 命中真实 wakeup edge host |
| `target_self` | self basis/causality，或 subject 就是分析目标 |
| `interval_proven` | 有 credential envelope、`ChainAnchoredMs>0`、source 为 wakeup-chain 前缀，或支持区间与有效链区间重叠 |
| `member_inherited` | `ChainIdentityInheritance`，但没有更强 envelope |
| `none` | 上述均不满足 |

`none` 的处理：

- demote 到 background；
- 撤销 chain ordinal；
- sticky wire 保留审计记录；
- result caveat 披露；
- 重新分配 ordinal；
- chainless board 不运行该普查。

注意：这是一张**链席位准入凭证表**，不是统一强度的反事实证明表。`member_inherited` 和 `target_self` 都能保住链席位，不等于存在 cross-thread exact wakeup edge。

### 7.5 Channel、顺序和 score

排序先定 channel：

```text
on_chain -> adjacent -> background -> unknown
```

同 channel：

1. effective 降序；
2. 链上同值时，已解析 blocking peer 优先 inherited-target；
3. score 降序；
4. raw impact 降序；
5. source line 起点升序。

score 只打破 effective 同值：

```text
score ≈ effective_basis × confidence × type_weight
```

典型 weight：inversion 1.35，IO/D/binder 1.25，runnable/CPU/scheduler 1.15。semantic boost 同样不能越过更大的 effective。

### 7.6 tier、ordinal 和容量

- data gap、target symptom、pacing/context、caliber、effective<=0：rank 0。
- off-chain semantic/background 无普通 ordinal，可有内部 `BackgroundRank`。
- tier 在 on-chain+adjacent 的全局 election position 上产生。
- `Rank` 在各 channel 内分别编号。

所以 `adjacent Rank=1` 同时为 `tier=tertiary` 是现行合法组合。

主榜默认 cap=12。survivor 先按 channel/effective 决定，再按原顺序发布；正 effective 链席位受保护，不应被非链项挤死。

侧栏容量：

| side lane | cap |
|---|---:|
| rank-0 disclosure | 4，target/gap 分配 |
| caliber | 4 |
| remainder | 8 |
| demoted | 8 |
| cap-dead target-self | 6 |

cap death 和 overflow 必须披露。

### 7.7 Fix direction 和守恒

进入严格 direction inventory 的席位需同时满足：

- rank>0、effective>0、on-chain；
- basis 在闭集；
- 非 demoted/absorbed/caliber；
- PID 已知；
- 支持区间是该线程 wall-clock；
- fix direction 已解析；
- 有精确 support inventory。

跨方向、同线程支持区间做对称 overlap。小于较小 effective 的 5% 时只抑制披露 token；达到 5% 标“待追认”。同方向按 `(pid,direction)` 合并支持区间，至少 2 席且总和超过 `window+0.001ms` 才报告 conservation violation。

这只是 eligible 子集的审计，不是所有席位的全量数学证明。

## 8. 因果投影构造

### 8.1 Typed observation

`internal/tool/trace_query.go` 将引擎结果转为 `ObservationRecord`，保留：

- path/branch/depth/segment；
- subject/object/thread/PID；
- artifact、line、actual/query window；
- projected/cumulative/effective；
- dominant 和各 state lane；
- rank/tier/channel/score；
- gated/supply/priority/direction；
- RSPA、family、aggregation、overflow、caveat；
- chain credential；
- P3 四字段，但结构规则禁止下游消费。

### 8.2 Path election

- branch-form path 优先 legacy path；
- 指定用户实体时，优先含该实体且有 frame corroboration 的 path；
- 实体可匹配 path 任意位置；
- 命中后截到用户实体；
- 无 typed match 按发布顺序回退；
- branch+window identity 用于挂树，禁止跨分支串接。

### 8.3 Bucket

| bucket | 构造容量 |
|---|---:|
| Primary | 预聚合 10 |
| OnChain | 后聚合 24 + fold |
| Adjacent | 后聚合 8 + fold |
| Background | 后聚合 8 + fold |
| Semantic | on-chain 不限；off-chain 16 |
| Supporting hops | 后聚合 10 + fold |

这些容量独立于根因榜 cap=12。

### 8.4 Fold 顺序

大致顺序：

1. 搬迁 absorbed rows；
2. R1 严格同事实折叠；
3. raw twin convergence；
4. R4 peer alias；
5. V4 exact/near publication fold；
6. instant marker fold；
7. R2 同 `(subject,object)` 聚合；
8. raw member reissue/twin convergence；
9. unknown background fold；
10. semantic seat unification；
11. 重排；
12. bucket cap/fold；
13. attachments 和 anchor window。

### 8.5 V4 near duplicate

V4 要求同 subject、object、type 和真实 object identity；行或时间范围需相交。数值精确相同可 fold；near 值相差不超过约 3% 也可 fold，发布值取最大值并累计 publication count。

因此它不是纯显示去重，而会删除席位并改变发布值。

### 8.6 R2 aggregate

- 同 subject/object 至少 3 条。
- 不同窗口且 occurrence interval 可比：区间并集。
- 查询窗互相重叠但不能精确求并：最大值下界。
- 同一窗口多个 occurrence 默认允许求和，不仅因同窗就假设重叠。
- ×N 保留 count、显示值、极值、窗口和 rank-0 family 证据。

### 8.7 Anchor window

优先级：

1. frame target resolution 的 query window/显式窗口并集；
2. selected window；
3. wakeup impact、aggregate、root rank 记录窗口。

窗口存在性是显式字段；`[0,end]` 可作为 anchor。

## 9. “窗内可消除量”五区面板

### 9.1 入榜基线

这张面板的代码注释把它定位为 **optimization-reminder face**：反事实合法性不负责隐藏 root seat。

候选基线：

- 来自 root-cause rank 或正式 fold peer；
- rank>0，或正式采纳的 semantic；
- positive effective；
- 非 caliber/count/composite/data gap；
- board/window 身份有效；
- 经一因一席、subset、constituent 和微量折叠。

同线程 inversion+runnable 双席仅在以下精确条件折叠：

- 同 channel、同 subject；
- effective 在微秒精度相等；
- inversion running deficit=0；
- gated runnable=runnable full；
- inversion 保留并合并证据；
- 存在歧义第三席时不强折。

### 9.2 五个区域

1. **链上修复方向区**：链上席位 TOP5；若 top 缺 semantic，可补第一条链上 semantic fallback。
2. **业务线索区 `◈`**：TOP8。
3. **邻接区 `◇`**：TOP3，明确为 conditional upper bound，且在方向守恒外。
4. **背景区 `▒`**：TOP3。
5. **辅助对账区**：重叠、守恒、subset/gated constituent、caliber/self symptom、unranked max、未测 self fold 等。

链上和邻接各自使用本区 TOP1 做条形图满刻度；不能跨区比较条长。

### 9.3 排序和方向

- chain、adjacent、background 分区独立。
- direction 未解析/other 放最后。
- direction 组按组内最大 effective 降序，不按组总和。
- 组内按值降序。
- 被截的 chain/adjacent/background/semantic 数量必须披露。

### 9.4 小计

同方向小计只在以下条件全部成立时打印：

- direction 已解析；
- 至少 2 席；
- 同一 board；
- 每席有 faithful、有效 envelope；
- `MergedCount<=1`；
- 任意两席 envelope overlap<=`0.001ms`。

小计是**已经显示的微秒取整值之和**。有 overlap 标“不可直加”；缺 carrier、跨 board、单席时不打印算术。

跨方向 overlap 辅助区只显示 top3 和 tail count。同方向 conservation 仍使用第 7.7 节的 eligible population。

### 9.5 数值真正表示什么

当前面板主体数值继承 `effective_impact_ms`。它代表：

> 经类型规则折算并通过席位准入的窗内优化归因量。

它不自动代表：

> 对某项修复做干预后，目标延迟必然下降的毫秒数。

但 renderer/legend 仍有 `on-chain seats = proven eliminable amounts`、`no proven on-chain eliminable` 和 target-self “proven eliminable” 等词面。这与代码注释的 optimization-reminder 定位不一致。

### 9.6 P3 counterfactual

内部字段：

- `P3MCounterfactualValidMs`
- `P3MCounterfactualInvalidMs`
- `P3MEdgeWitnessedMs`
- `P3MDisposition`

当前规则：

- 只覆盖 `families:[periodic_pinned]`；
- valid+invalid 在整数微秒精度等于 anchor time；
- closing edge 的周期等待进入 invalid；
- edge witnessed 是结构见证下界；
- disposition 区分 segment join、edge-terminated window、counterfactual only、self ruled、no inventory、no anchors、occurrence capped 等；
- late-relay 明确未测；
- 这些字段在根因排序后盖章；
- 结构测试禁止 parser、renderer、模型或其他消费者读取；
- 不影响 rank、seat、value、caveat。

所以当前没有用户可见、可用于“proven eliminable”的完整 counterfactual contract。

## 10. Trace 容量和阈值总表

| 层 | 阈值/容量 | 影响 |
|---|---:|---|
| target near-tie | 约 0.050ms | 状态优先级打破近似时长并列 |
| wakeup lookahead | 5µs | S-sleep 匹配 wakeup |
| target branch | `MaxBranches` | 影响展开；via 可追回 |
| node extra | `MaxBranches-1` | 只控制 extra |
| extra floor | `max(MinDurationMs,1ms)` | 中间节点额外 S 分支 |
| chain extra | `MaxChainNodes` | 不截 guaranteed top-1 |
| edge census pair | 16 | 只截展示 |
| periodic occurrence | >=5 | 周期识别门 |
| periodic in-band | >=2/3，约 15% | 周期识别 |
| cluster co-transition | 15µs | 共振域证据 |
| occurrence windows | 8 | 展示；count 全量 |
| aggregate view | 8 | 展示；full census 可继续供 rank |
| root board | 12 | 发布席位 |
| rank-zero/caliber | 各 4 | side lane |
| remainder/demoted | 各 8 | side lane |
| projection primary | 10 | 预聚合 |
| projection on-chain | 24 | 后聚合 + fold |
| projection adjacent/background | 各 8 | 后聚合 + fold |
| projection semantic off-chain | 16 | on-chain semantic 不限 |
| supporting hops | 10 | 后聚合 + fold |
| V4 near | 约 3% | 可硬 fold 并取 max |
| direction overlap disclosure | 5% | 只控制 overlap token |
| direction conservation tolerance | 0.001ms | eligible support 守恒 |
| elimination chain | TOP5 | 答案展示 |
| business leads | TOP8 | 答案展示 |
| adjacent/background | 各 TOP3 | 答案展示 |

## 11. Trace 矛盾与覆盖缺口

### 11.1 旧审计问题状态

| ID | 状态 | 当前判断 |
|---|---|---|
| G1 周期启发式进入硬排名 | 仍存在，高 | 注释称其只驱动 soft surfaces，但 effective 会改变 ordinal、tier 和 cap 生死；“soft”与下游硬消费矛盾 |
| G2 链上“已证明可消除”过强 | 部分修复，高 | 新增 credential census 并 demote `none`；但 inherited/self/interval 都能授权，证明强度仍不足 |
| G3 P3 不可见且覆盖窄 | 仍存在，高 | 仍是 display-only，只测 periodic-pinned；代码注释提到 stage two，但没有消费实现 |
| G4 3% near fold | 仍存在，高 | 仍用近似相似度删除席位、取 max |
| G5 via OnChain 语义 | 部分修复，中 | 新增 `PathComplete`，但 `OnChain` 仍是 expanded-node membership |
| G6 D/IO 上游因果缺失 | 仍存在，中 | 保守不伪造 scheduler edge 是正确的；缺 typed resource-causality lane |
| G7 中间非 S 多分支不足 | 仍存在，中 | extra 仍只允许 S |
| G8 timestamp zero | 已修复 | 有显式 start/present 语义和对抗测试 |
| G9 守恒通过措辞过宽 | 仍存在，中 | 检查仍只覆盖 eligible 子集，外部词面需要显示 eligible/total |
| G10 tier/ordinal 两位置空间 | 仍存在，中 | 内部自洽，用户语义仍混淆 |
| G11 effective 注释漂移 | 部分仍有，低 | 闭表代码正确，但周边注释仍有历史描述 |
| G12 gated reason 注释漂移 | 需持续清理，低 | 字段实现已扩充，权威注释应统一 |
| G13 5% overlap 阈值 | 仍存在，低 | 目前只控制披露，尚未直接合并席位；应保持这一边界 |

### 11.2 新发现或新暴露的问题

| ID | 严重度 | 问题 | 影响 | 建议 |
|---|---|---|---|---|
| G14 | 中 | chain credential caveat 按前缀去重，多个不同 `none` 席位可能只留下部分名字 | sticky wire 尚在，但人读 caveat 不完整 | 用 typed list 聚合，最后一次性渲染 |
| G15 | 中 | cluster frequency lane 被完整性预算丢弃后系统可继续分类/折算，仅披露 dropped CPU | 缺 lane 可能降低 cluster count 或改变 domain，披露不能恢复数值可信度 | 对能力分类所需关键 lane 建 precise completeness gate；否则退 `freq_only` |
| G16 | 中 | limits anchor mismatch 只披露，不改变判定 | parked/合并 cluster 的 emission convention 假设可能不成立 | mismatch 时能力分类降级或输出有界区间 |
| G17 | 低 | supply fold 部分注释仍描述旧 `resolveCoreTopology`/频率 tier 推断 | 维护者可能按旧架构修改新 capability path | 把 cluster/donor/capability/fold 入口文档集中到单一权威注释 |

### 11.3 不是矛盾的点

- chain-first、adjacent-second、background-last 是刻意排序，不是 score 错位。
- D/IO 不沿 raw wakeup 递归是因果保守，不应直接套用 S-sleep 逻辑。
- root cap、projection cap、five-zone cap 是不同层，不要求数值相同。
- 同方向重叠时拒绝求和是保守规则；缺的是 counterfactual，不是更激进的算术。
- frequency 未知时 deficit=0 是下界语义，不是“证明没有供给问题”；必须保留 unknown basis 披露。

## 12. Read mode 当前实现

### 12.1 主路径

```text
optional log_triage / perf_triage
  -> analyze
  -> deterministic TaskGraph / EvidencePlan / hypotheses / quality gate
  -> runReadSchedulerLoop
     -> explore
     -> extract
     -> finalize
```

- `runReadSchedulerLoop` 保留 legacy read scheduler 主体，L1 结构测试要求关键 body 字节稳定。
- orchestrator 不直接调用工具、MCP 或 LLM，统一经 agent。
- explore 有任务图、预算、自适应 subtopic、shape/stall guard 和 completion lane。
- extract/finalize 有 contract check、stage retry 和 best-draft fallback。
- analyzer 的 stream error 仍是硬失败；部分非 stream analyzer 失败可生成 semantic recovery IR，避免空答案。这里与“missing emit 永远 fail-loud”的旧文档表述需要统一成精确分层规则。
- read snapshot resume 会校验 canonical repo root、repo fingerprint 和 active state，避免把另一个仓库的 read 状态续进当前查询。

### 12.2 Read mode 的真实副作用边界

Read mode 保证的目标应表述为：

> 不修改用户源文件和仓库 HEAD。

它不是“零文件系统写入”：运行会写日志、缓存、memory、blob/session、snapshot 等 runtime artifact。文档若写“0 字节副作用”会与实现冲突。

## 13. Write mode 当前实现

### 13.1 进入和分类

- CLI 显式 `--mode=write`，REPL `/mode write`、`/write`，或 structured TurnPolicy `route=write` 可进入。
- `write_enabled:false` 是组织级 kill switch。
- read analyzer 先分类，再由 `write_analyzer` 产生 `WriteAnalysisIR`。
- 低置信 write route 可降为 repo analysis。
- 未结清 plan/workflow 会阻止冲突的新 write。

### 13.2 Controller 状态机

write 不走 legacy 三节点图，统一进入 controller 动态 DAG。typed action：

- `explore_code`
- `plan_batch`
- `apply_plan`
- `verify_batch`
- `append_batch`
- `split_batch`
- `replan_batch`
- `ask_user`
- `finish`
- `block`

模式面：

- plan lane 屏蔽 apply/verify；
- verify lane 只允许 verify/ask/finish/block；
- apply lane 开放完整 action。

transition kernel 按 batch/slice typed state 检查：

- ready-to-apply 只能 apply/block；
- pending approval 只能 ask/block；
- approval 无效只能 ask/replan/block；
- applied 必须 verify；
- failed verify 进入 replan/explore/finish/block。

预算默认包括最多 5 batches、每 batch 最多 2 轮 exploration、24 controller turns 和全局 step cap；可有一次有界 completion verify 越过普通 cap。

### 13.3 风险和审批

- 风险由 plan shape、path/content shape 和 `WriteAnalysisIR` 确定性计算。
- critical 拒绝。
- high 必须人工审批。
- auto-safe 允许 low/medium；auto-low 只允许 low；manual 默认要求审批。
- ApprovalRecord 绑定 exact plan fingerprint 和 integrity；plan 变化后旧审批失效。
- precise signals 控制硬门；模型评分或候选数量不应直接授权 apply。

### 13.4 Worktree 和恢复

- apply 在隔离 git worktree 中运行，通常是 detached HEAD。
- `BusContext.RepoRoot` 在 apply/verify 期间切到 worktree。
- 外层 defer 无条件 discard worktree。
- 成功 apply 可建立 `refs/codrax/applied/<plan-id>` 恢复 ref。
- 默认成功后也丢弃 worktree；`keep` 或特定 verify lane 可保留。
- main repo 不自动 merge，也不 push。
- `/merge` 是单独的显式动作。
- 显式授权 auto-init 时可在未初始化目录执行 `git init` 和 initial commit；这是“main HEAD 永不自动变化”总述的授权例外。

### 13.5 验证和完成语义

完成 verdict：

- `verified`
- `unverified`
- `accepted_failed`

`Status=complete` 只表示 controller 收敛；消费者必须同时读取 completion verdict。

- runner 缺失、没有测试或 parser 不可用：typed unverified，不冒充 verified。
- 真实验证失败：默认 replan；用户可显式 accepted_failed。
- baseline capture 默认关闭；cache 命中可复用。
- 没有 baseline 时，`CritNoRegression` 直接 satisfied。因此 `verified` 不必然表示“和变更前基线相比无回归”，只表示本次选定验证面通过。

### 13.6 多仓库

当前 write 要求恰好一个 active sub-repo；ChangePlan 不能跨 sub-repo。协调式跨仓库写尚未实现。

## 14. 读写模式系统缺口

| ID | 严重度 | 模式 | 代码事实 | 风险 | 建议与验收 |
|---|---|---|---|---|---|
| RW1 | 高 | write | `WriteWorkflowRun` 无 repo root、repo fingerprint、base SHA/branch、request fingerprint；`FindActiveRun` 取 planDir 最新 active run；`loadOrSeedWriteWorkflowRun` 无身份比较即续跑 | 同一 runtime CWD 先跑 repo A 再跑 repo B，可能把 A 的 durable DAG 对 B 执行；审批、计划和 worktree containment 失配 | envelope 持久化 canonical repo、repo fingerprint、base SHA/branch、goal hash；自动续跑逐项精确匹配，不匹配则新建或要求显式选择；增加 A→B 对抗测试 |
| RW2 | 高 | read+write | `exec_command` 只检查 shell token、命令名和少数危险参数；允许 `awk`、`sed`、`git branch`；没有 awk program、git subcommand option、sed `e` 等完整语义验证 | 可通过 `awk system()` 等执行任意命令；read 可修改源文件，write 可绕过 apply_patch/WriteClosure，甚至写出 worktree | 不把 shell allowlist 当 sandbox；优先移除通用 shell，改 typed observation tools。过渡期删除 awk/git branch 等多能力命令，按 command AST 白名单参数，并用 OS sandbox 限写 |
| RW3 | 高 | read+write | 普通 `cat/rg/grep/find/head/tail/ls/stat/...` 参数没有通用 repo-relative path 校验；只有 `cd`、`GIT_DIR/WORK_TREE` 和少数 git path option 受限 | 可读取仓库外文件，违背拒绝文案中的“stay inside active repository”；可能扩大 secret 暴露面 | 在执行前 canonicalize 每个 path operand，拒绝 absolute、`..`、symlink escape；更稳妥是系统级只读 bind/sandbox |
| RW4 | 中 | write | baseline capture 默认 false；无 baseline 时 `CritNoRegression` satisfied | `verified` 容易被理解成“无回归”，实际可能只代表当前测试通过 | completion 增加 `tests_passed`、`baseline_compared`、`impacted_surface_covered` 等正交字段；无 baseline 必须显式披露 |
| RW5 | 中 | write | 只支持单 active sub-repo，外层 workflow 也不保存 repo identity | 无法安全实现跨仓库 fanout；与“durable workflow 可协调”类注释不一致 | 先修 RW1，再为每 batch/slice 绑定 repo identity 和独立 worktree/approval |
| RW6 | 中 | write/docs | auto-init 会在显式授权下初始化 main root 并创建 initial commit，但总述常写“main repo HEAD 永不自动变化” | 合同文字自相矛盾，用户可能误解授权范围 | 改为“已初始化仓库在 Auto Pilot apply 中不改当前源/HEAD；显式 auto-init 和显式 merge 是独立授权例外” |
| RW7 | 中 | write | workflow `Save` 失败只 warning，controller 继续；store nil 也继续内存执行 | durable recovery/audit 能力在突发失败时静默退化；若随后 crash，状态与已建 ref/变更报告可能脱节 | 首次 mutation 前要求 durable checkpoint 成功，或把 `persistence_degraded` 作为 typed final state；增加 save-failure 注入测试 |
| RW8 | 中 | read/docs | 非 stream analyzer failure 可降级 semantic recovery，但旧架构文档仍笼统写 fail-loud | 运维和测试对“何时失败、何时降级”预期不一致 | 文档列出 missing emit retry、stream hard fail、non-stream recovery 的准确状态机 |
| RW9 | 低 | write/integration | `Status=complete` 可搭配 unverified/accepted_failed | 只看 status 的外部消费者会误报成功 | API/schema 强制消费者读取 verdict；提供 `successful` 派生字段或拒绝只按 status 渲染 |
| RW10 | 低 | docs | 仍有“线性 plan/apply/verify 图”“read 0 字节副作用”等历史表述 | 新维护者按旧模型理解 controller 和 runtime artifacts | 删除双架构图，保留单一 controller 权威章节和模式副作用矩阵 |

## 15. 模式隔离应采用的目标契约

| 能力 | Read | Write plan/explore | Write apply | Verify | Merge |
|---|---|---|---|---|---|
| 读 repo 内源码 | 允许 | 允许 | 允许 | 允许 | 允许 |
| 读 repo 外任意路径 | 默认拒绝 | 默认拒绝 | 默认拒绝 | 默认拒绝 | 默认拒绝 |
| 修改源文件 | 拒绝 | 拒绝 | 仅 worktree、仅 apply_patch/typed effect | 拒绝 | 显式用户动作 |
| 修改 main HEAD | 拒绝 | 拒绝 | 拒绝 | 拒绝 | 显式用户动作 |
| 建 recovery ref | 不适用 | 不适用 | 允许并披露 | 允许读取 | 可消费 |
| 写 runtime artifact | 允许、限定 runtime root | 允许 | 允许 | 允许 | 允许 |
| 通用 shell | typed 只读或 sandbox | typed 只读或 sandbox | 不得作为 apply channel | typed 验证 runner | 仅明确 merge implementation |

当前 worktree、risk、approval 和 typed completion 已大体符合该模型；RW1/RW2/RW3/RW7 是需要补齐的核心边界。

## 16. 修复优先级

### P0：先封系统边界

1. 修 RW2/RW3：把 exec observation 变成真正的 capability/path sandbox，立即移除可执行子语言的命令。
2. 修 RW1：给 write workflow 加 repo/base/request identity，并在续跑前 fail closed。
3. 修 RW7：mutation 前 durable checkpoint 失败不能只留日志。

### P1：收紧 trace 的“证明”口径

1. 把 `effective attribution`、`support-covered optimization`、`counterfactual saved` 拆成不同 typed 字段和词面。
2. chain credential 按强度分层；`member_inherited`、`target_self` 不进入“proved”人口。
3. 在 P3 有可消费、可守恒的反事实合同前，将“proven eliminable”统一改为“窗内优化归因”。

### P2：移除噪声硬门

1. 周期启发式先作为 advisory；只有 typed producer/deadline 或其他精确信号授权硬折算。
2. V4 near duplicate 只标 suspected duplicate，硬 fold 只依赖稳定 publication ID 或精确区间关系。
3. cluster completeness 不足或 anchor mismatch 时退 `freq_only`/区间，不仅披露。

### P3：补覆盖和契约

1. 为 D/IO 建 blocked-reason/resource causality lane，不冒充 scheduler wakeup。
2. 按需求为中间 runnable/D/IO 增加低预算、精确身份的非 wakeup branch。
3. completion 拆解 baseline/coverage/test 维度。
4. 统一 read failure/recovery、auto-init、controller DAG 和 runtime side effect 文档。

## 17. 建议新增的验收测试

### 17.1 Trace

1. 周期 gap 从 14.9% 变到 15.1%，没有 typed producer 时 rank/tier/cap 不变。
2. V4 2.9% 与 3.1% near 值都不得导致席位静默消失。
3. via 在 expanded nodes 中但没有完整单调路径：`OnChain=true`、`PathComplete=false` 必须明确渲染为两种事实；最终建议改为强 OnChain=false。
4. `member_inherited`、`target_self`、`interval_proven`、`wakeup_anchored` 分别验证可展示词面，不得共用“proved”。
5. 多个 `none` credential 同时出现时，typed demotion census 和 caveat 名单均完整。
6. `[0,end]` 保留 anchor、duration、百分比和小计资格。
7. dropped frequency lane/limits mismatch 改变 cluster 可信度时自动退 `freq_only`。
8. direction conservation 同时显示 eligible/total。
9. P3 exact edge 第一批消费契约：valid/invalid 守恒、unmeasured reason、禁止未授权 family 读取。

### 17.2 Read/write

1. planDir 中存在 repo A active workflow，在 repo B 启动时不得自动 resume。
2. 同 repo 不同 base HEAD、branch 或 goal hash 不得静默 resume。
3. `awk 'BEGIN{system(...)}`、sed execute command、`git branch <new>`、git output-to-file 全部拒绝。
4. `cat /etc/...`、`rg pattern ../...`、symlink escape 全部拒绝。
5. read mode 允许 runtime artifact，但 source tree 和 HEAD hash 前后相同。
6. workflow store 注入 save failure：首次 apply 前 block 或产生 typed persistence-degraded terminal。
7. 无 baseline 但 tests pass：completion 必须区分 `verified_current_checks` 与 `baseline_compared=false`。
8. complete+unverified/accepted_failed 的 API 消费者不得渲染为 verified success。

## 18. 主要代码索引

| 主题 | 实现 |
|---|---|
| trace 查询编排 | `internal/tracequery/query.go` |
| 输入准入 | `internal/orchestrator/runtime_trace_input_admission.go` |
| 状态全集 | `internal/tracequery/thread_state_universe.go` |
| 零时间戳窗口 | `internal/tracequery/window_start_flag.go`、projection G8 tests |
| wakeup chain / impact | `internal/tracequery/query.go` 及 wakeup/aggregate 文件 |
| edge census | `internal/tracequery/wakeup_edge_census.go` |
| via path report | `internal/tracequery` 的 via-thread report 实现 |
| chain credential | `internal/tracequery/chain_credential_census.go` |
| cluster domain/donor | `internal/tracequery/cluster_freq_share.go` |
| core capability | `internal/tracequery/core_capability.go` |
| supply fold | `internal/tracequery/supply_fold.go` |
| root cap/side lanes | `internal/tracequery/root_cause_rank_capacity.go` |
| direction axiom | `internal/tracequery/rank_direction_axiom.go` |
| P3 measure | `internal/tracequery/rank_p3_measure.go` |
| typed observation | `internal/tool/trace_query.go` |
| causal projection | `internal/types/trace_causal_projection*.go` |
| five-zone board | `internal/tool/answer_document_mutation_runtime_elim.go` |
| read scheduler | `internal/orchestrator` 的 `runReadSchedulerLoop` |
| read resume identity | `internal/orchestrator/read_run_snapshot_resume.go` |
| write controller | `internal/orchestrator/write_controller_scheduler.go` |
| write run schema | `internal/types/write_workflow_run.go` |
| write run store | `internal/repl/write_workflow_run_store.go` |
| risk/approval/worktree | `internal/orchestrator/stage_hooks.go`、`internal/worktree` |
| exec observation policy | `internal/tool/exec_command_readonly.go` |

## 19. 当前可对外使用的准确口径

> Trace 根因榜先按真实链路相关性分区，再按类型闭表折算后的窗内有效归因排序；普通 sleep、gap 和无供给缺口的 running 不靠原始时长抢名次。真实 scheduler 唤醒边只由满足条件的 S-sleep 与受支持 wakeup 事件建立。答案页“窗内可消除量”目前应理解为链上、邻接和背景根因的优化归因提醒，其中邻接是条件上界；除非某项同时具有可消费的精确反事实凭证，否则不能承诺该毫秒数会一比一转化为修复后的目标延迟下降。

> Read mode 的边界是“不改用户源文件和仓库 HEAD”，不是“不写任何运行时文件”。Write mode 通过风险、审批和 worktree 隔离 apply，但自动续跑必须绑定仓库/基线身份，shell observation 也必须成为真实的只读、仓库内能力边界；在这两项修复前，不能仅凭模式名称声称系统隔离已闭环。
