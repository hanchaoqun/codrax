# 客户 trace `busy=0` 与丢帧结论系统缺口审计

> 日期：2026-07-23
> 审计基线：`main@9dba57e6d87f`
> 客户回传：`/Users/han/opt/customlogs/cpu_busy_zero.txt`
> 范围：只审计事实、代码机制、既有台账归属和后续任务顺序；本轮不修改生产代码和测试。

## 1. 结论摘要

这次 `12` 个 CPU 全部显示 `busy=0.000ms / idle=0.000ms` 是真实系统 gap，但不是生命周期/incarnation guard 本身判错。

实际故障由三层叠加形成：

1. `ComputeWindowStats` 用一个全局 `threadIncarnationConflictForQuery(..., onlyPID=0)` 同时控制所有 scheduler duration 聚合。窗口内任意 TID 的 incarnation 冲突都会阻止所有 `sched_switch` 进入 `byCPU`，无关线程的身份冲突因此连坐 CPU 全局 busy/idle。
2. CPU 频率事件不依赖上述 `byCPU`，`applyCPUFrequencyResidency` 仍会为每个有频率样本的 CPU 新建 `CPUStats`。此时 `BusyMs/IdleMs` 只是 Go 零值，不是测量结果。
3. `CPUStats` 没有 busy/idle 的 typed availability，渲染器又无条件打印两个数值，最终把“聚合被撤销”伪装成“实测为零”。同一处 `scheduler_head_coverage=unknown` 的 `missing_cpus=0:[]` 也只是未做 census 后的零值，不代表零缺失。

因此：

- 不能把本结果解释为系统空闲；
- 生命周期 guard 对 PID/TID 身份相关聚合继续 fail-close 是正确红线；
- CPU 全局 busy/idle 属于身份无关的 CPU 区间数学，不应被无关 TID incarnation 冲突清空；
- 即使暂时不能安全恢复数值，也必须呈现为 `unavailable/unknown`，绝不能呈现为 `0.000ms`。

同事审计中的 B-1、B-2、C-1、F1、F2 基本成立；C-2、D1、D2、D3、E1b、E3 需要按本报告中的边界改写。另有三项比原清单更高危：

1. 当前没有任何 frame/deadline 或 typed causal row，却输出了确定性的“导致丢帧/无法在 VSync 内完成”结论；
2. 达到上限的 `event_search/read_file` 子集被当成完整集合，进而产生“仅 8 段、总计 0.817ms、共唤醒 8 次”一类无权威的精确结论；
3. `prev_state=S` 被说成 RT 抢占，Harmony priority `20` 被说成 RT；这与代码内置的调度语义直接矛盾。

## 2. 客户回传能确定的事实

### 2.1 转换链已经成功

本次回传不是转换失败：

- `trace_streamer` DB 导出成功；
- SQLite DB 转 systrace 成功；
- 生成了 `189739` 个事件；
- artifact 收据显示 `权威已知行=189739`、`可供trace_query消费=是`；
- trace bundle 也已生成。

这说明此前 Windows 私有转换目录句柄问题的修复在该客户样本上已经走通。本报告后续问题发生在 trace 查询和答案构造阶段，不应再归因给转换器。

### 2.2 查询窗口与目标

用户关注窗口为：

```text
69326.832743749 .. 69327.060110624
```

窗口长度为约 `227.367ms`。首次 jank 查询同时传入了线程名 `ss.hm.ugc.aweme` 和数值目标 `32788`。

trace_query 返回了：

- `thread_incarnation_conflict`；
- 12 个 CPU 的 `busy=0.000ms / idle=0.000ms`；
- `io_pressure score=180`，其中 `iowait_blocked=36`、`d_state=0`、`io_wait=0`；
- 没有匹配的 frame/render span；
- 最终因果投影覆盖块明确说没有有数据支撑的 `root_cause/wakeup_chain/semantic` 行。

### 2.3 后续手工探索暴露的证据边界

模型随后通过 `event_search/read_file` 找到：

- 数值目标 `32788` 的调度 comm 为 `unknown`；
- 名称 `ss.hm.ugc.aweme` 命中另一个 TID `33410`；
- TID `33410` 在关注窗内确有多个很短的运行片段。

但这些事实只能把 `33410` 定义为“同名候选线程”。当前输出没有提供足够 typed 证据证明它一定是：

- 用户原本想问的线程；
- 进程主线程；
- UI/render 线程；
- 某一丢帧 frame 的执行线程。

线程名匹配不能替代 hard identity；反过来，hard identity 与名称冲突也不应静默。

## 3. `busy=0` 的精确代码链

### 3.1 全局 incarnation gate 清空了 scheduler duration 输入

`internal/tracequery/query.go:2493-2495`：

```go
schedulerFailure := schedulerStateIntegrityFailureForQuery(idx, q, 0)
identityConflict := threadIncarnationConflictForQuery(idx, q, 0)
schedulerDurationsSafe := schedulerFailure == nil && identityConflict == nil
```

这里 `onlyPID=0` 使 `threadIncarnationConflictForQuery` 接受窗口内任意 TID 的 generation 冲突。`internal/tracequery/thread_incarnation_guard.go:312-325` 也明确实现了这一含义。

随后 `internal/tracequery/query.go:2593-2597` 只有在 `schedulerDurationsSafe=true` 时才把 `sched_switch` 放入 `byCPU`：

```go
case EventSchedSwitch:
    if schedulerDurationsSafe {
        byCPU[ev.CPU] = append(byCPU[ev.CPU], ev)
    }
```

所以一个与 TID `33410` 无关的生命周期冲突，也会撤销全部 CPU 的 scheduler duration 输入。

### 3.2 频率事件重新创建了零值 CPU 行

`internal/tracequery/query.go:8973-8995` 的 `applyCPUFrequencyResidency` 会遍历频率事件。若该 CPU 不在已有统计中，就追加：

```go
CPUStats{CPU: cpu, Frequency: latest, FrequencyResidency: residency}
```

这里没有给 `BusyMs/IdleMs` 赋值，因此两个字段保持零值。

### 3.3 类型和渲染面无法区分零值与不可用

`internal/tracequery/types.go:1990-1997` 的 `CPUStats` 只有：

- `BusyMs`
- `IdleMs`
- `Frequency`
- `FrequencyResidency`

没有 `busy_idle_status`、`known`、`coverage` 或 withdrawal reason。

`internal/tool/trace_query.go:3913-3915` 又无条件打印：

```text
busy=%.3fms idle=%.3fms
```

所以真正的测量零值和因 fail-close 未计算的零值在所有用户面上完全相同。

### 3.4 coverage 的 `missing=0` 也不成立

incarnation 冲突分支直接创建：

```go
SchedulerHeadCoverage{
    Status: "unknown",
    Reason: "thread_incarnation_conflict",
}
```

`MissingCPUCount/MissingCPUs` 没有经过 census，只是零值。渲染器仍打印 `missing_cpus=0:[]`，读者会误以为 coverage 虽是 unknown，但没有缺 CPU。正确语义应是 `missing_cpus=unknown/not_evaluated`。

## 4. 逐项审计同事提出的 gap

| 原编号 | 审计结论 | 精确边界 |
|---|---|---|
| B-1：`pid=` 静默吞线程名 | **确认** | `resolveThreadSelection` 在 `q.PID>0` 时直接返回 exact TID，名称完全不参与诊断。exact TID 优先是正确红线；缺口是没有 mismatch caveat 和候选 TID，不是应该自动改路由。 |
| B-2：身份冲突清空所有 CPU busy | **确认，且已有台账** | CPU busy/idle 是 CPU-lane 区间数学，不应被无关线程身份冲突连坐。当前还把 unavailable 渲染成零。 |
| C-1：丢帧分析拿不到帧仍给结论 | **确认，P0** | frame span 在 exact TID incarnation 冲突时安全 fail-close；真正 gap 是 bundle 仍继续生成调度/IO上下文，答案又把这些上下文升级为确定 frame-drop 因果。 |
| C-2：frame span 只有 exact TID | **部分确认** | 当前 `threadMatches` 对正 TID 只允许精确相等，Query 也没有显式 process scope。不能把现有 `pid` 悄悄改成 TGID 扩展；应新增 typed `thread/process` scope，由调用者显式选择。 |
| D1：36×5 得到 IO score 180 | **症状确认，红线定性需改写** | 代码确实只凭 count 得到 180，但该行是 `context_only/supporting_coverage`，并未参与 hard gate，所以不是“噪声驱动硬门”违规。真正问题是名字和呈现把 count-only 活动暗示成高 IO 压力，模型又把 supporting context 升级为根因。 |
| D2：没有披露 D/io_wait 为零 | **部分确认** | trace_query 原始 `io_pressure` 行已打印 `d_state=0/io_wait=0`；但系统补充的 `root_cause_context_only` 行只留下 score，typed observation 也遗漏 `IOWaitMs`，跨呈现面信息不对称。 |
| D3：Harmony 的 S+iowait 常见 | **作为兼容事实成立，不能推出无害** | 既有生产 witness 证明 S+iowait 是需要兼容的平台形；但不能因此把每条 marker 都判断为正常或无影响。正确规则是 count-only 只能作活动标记，必须有墙钟/存储延迟/同链关系等 corroboration 才能升级。 |
| GAP-F2：生命周期 caveat 是死胡同 | **确认** | 当前只建议 `split the window`，没有指出冲突 TID、候选 selector、哪个聚合面受影响，也没有提示无关全局冲突可能在分窗后继续存在。 |
| GAP-F1：`@...` 被 linkify 成 mailto | **确认，独立低风险项** | HTML 和终端 Markdown 都启用了 Goldmark `extension.Linkify`；artifact 文件名中的 `@69326-2310...` 被识别成邮箱。 |
| E1b：因果投影把空产归因给通用限流 | **确认** | coverage block 只从 tool failure/refinement/repair 收集前三个通用 reason，不读取 engine 的 incarnation suppression；随后建议重跑同类视图，可能再次命中同一个门。 |
| E3：1.0ms / 0.44% 与 0.817ms 矛盾 | **确认，但根因不止算术** | 8 段确实合计 0.817ms；除以 227.367ms 约为 0.359%，不是 0.44%。更严重的是被截断的行级子集被当成“全部 8 段”。 |

## 5. 本轮新增确认的高危 gap

### CBZ-01：因果结论超出 typed 证据上限

客户答案同时出现两组互斥信息：

- 系统覆盖块：没有有数据支撑的 `root_cause/wakeup_chain/semantic` 行；
- 模型正文：调度碎片化“导致 UI 渲染无法在 VSync 周期内完成，从而丢帧”，并给出“深层次原因”。

当前也没有：

- 被识别的具体 frame；
- frame actual/expected 或 deadline；
- frame duration/deadline miss；
- frame 到 TID `33410` 的 typed ownership；
- wakeup chain；
- 正式 root-cause rank seat。

因此现有证据最多支持：

> 同名候选线程 TID 33410 在窗口内出现短运行片段；当前结构化证据不足以证明这些片段造成了所问丢帧。

不能支持确定性的 frame-drop 因果陈述。`io_pressure context_only`、频率事件数量和短运行片段也不能单独补足这条因果链。

此项必须遵守既有“检测与执行分离”裁定：系统不因模型与检查器分歧阻断答案出厂、不制造重试环，但应由确定性系统块明确发布 `frame_causality=unproven`/`typed_causal_rows=0`，并在 finalizer 提示中把结论上限写清。

### CBZ-02：截断结果被升级成完整集合

探索过程本身已经暴露：

- `event_search` 达到结果上限；
- 多轮读取覆盖的行集不同；
- 第 8 轮列出了关注窗内至少 12 个片段；
- 最终正文只保留 8 个片段，并称“这 8 段是仅有/共 8 次/合计”。

最终 8 段：

```text
0.068 + 0.075 + 0.095 + 0.100 + 0.034 + 0.118 + 0.274 + 0.053
= 0.817ms
```

但探索文本还列出 `0.029ms`、`0.186ms`、`0.016ms`、`0.014ms` 等窗内片段。仅加入这四段就变为 `1.062ms`，约占窗口 `0.467%`。

因此问题不是简单的模型加法失误，而是系统没有把：

- `emitted subset`
- `complete census`
- `compacted/limit reached`

作为 claim authority 传播到最终答案。凡源自 capped 结果的观察，都不能支持“全部、仅有、总计、共 N 次、最大/最小”一类穷举或全量聚合结论，除非另有不受 cap 影响的 full-census 结果。

### CBZ-03：调度状态和优先级语义未约束最终 prose

最终答案至少有两处与当前代码语义直接冲突：

1. `prev_state=S` 只能证明任务从 RUNNING 离开后进入 interruptible sleep/blocking 类状态；它本身不能证明“被 RT 抢占”，也不能仅凭 next task 证明主动 `yield`。
2. Harmony priority `20` 按 `internal/tracequery/flavor.go:255-267` 属于 `ohos_cfs`；`41..159` 才是 `ohos_rt`。正文把 `prio 20` 称为 RT。

只有 `R/R+` 离开 CPU 才可支持“仍 runnable/可能被抢占”的候选语义；要升级为“被某线程高优先级抢占”，还需同一 `sched_switch`、可信 priority class、同 CPU 和 typed priority relation。

“共被唤醒 8 次”也不能由 8 个运行片段推导，必须来自 `sched_wakeup/sched_waking` 的去重 census 或正式 wakeup-chain 结果。

### CBZ-04：候选线程被升级成“主/UI线程”

名称 `ss.hm.ugc.aweme` 唯一命中 TID `33410`，只能证明窗口内存在同名候选。主线程/UI线程至少需要一种 typed 证据：

- TID 与已证 TGID/main-thread identity 相等；
- frame/doFrame/render marker 所有权；
- 明确的线程角色字段；
- 其他已经定义并可审计的角色映射。

当前模型直接把同名候选写成“进程主线程/UI线程”，属于角色越权。后续 B-1 修复只能输出 candidate，不得自动赋予 UI/main 角色。

### CBZ-05：频率变化被升级为调度压力

`CPU 频率调整 172 次` 是事件计数，不等于低频、降频、供给不足或调度压力。客户探索文本还先后出现“低频”和“所有核最大频率”两种相反解释。

频率因果至少需要现有 typed `low_frequency/compute_supply_signal`、频率驻留、cluster ceiling、CAP/affinity 或 running deficit 证据。裸 transition count 只能作背景活动，不能参与确定根因。

## 6. 与既有 gap 台账的关系

`docs/design/trace_analysis_open_gap_ledger_20260710.md:663` 已有：

> P2：收窄无关 TID-reuse 影响

该项已明确记录：

- `WindowStats/off-CPU/CPU pressure` 等全局/复合面仍开放；
- 先增加 typed completeness；
- 只收窄早期 guard 会把被抑制 context 伪装为数值 0。

`docs/design/real_trace_campaign_20260705.md:1959` 也明确写了：

> 禁止把缺失上下文伪装成 0。

所以 B-2 不应重复新建立案。正确处理是：

- 将本客户回传挂为该开放项的 production witness；
- 把原“中/P2”提升到首批 correctness；
- 把 CPU-global identity-independent lane 与 thread/process identity-dependent lane 的拆分写入施工冻结；
- 新增 unavailable 渲染和 coverage not-evaluated 的验收。

本轮真正新增立案的是 CBZ-01 至 CBZ-05，以及 B-1/C-2/F1/F2/E1b 的具体生产形。

## 7. 不是 gap 或不能按原说法立案的项

以下行为本身正确，不应在修复中破坏：

1. **incarnation guard 对 PID/TID 身份相关统计 fail-close**：正确。不能为恢复数值而跨 task generation 合并线程时长、comm、TGID 或 process census。
2. **正 TID 是 hard identity、comm 只是显示元数据**：正确。B-1 只能加诊断和候选，不得让名称覆盖数值身份。
3. **frame span 在目标 TID generation 冲突时省略**：安全。缺口是缺少 typed downgrade/discovery 和答案结论上限，不是应该猜配 frame。
4. **`io_pressure` 在 rank 内是 `context_only`**：引擎分层正确。缺口在 score caliber、跨面信息丢失和模型越权，不是它占了正式根因榜位。
5. **`unknown-thread` 用于全局 context row**：属于 aggregate 哨兵，不代表又发现一个未知线程。
6. **7 条 running row 被拒且有披露**：属于 conversion/query taint；只要影响范围、数量和撤销面完整披露，就不是静默丢证据。
7. **trace-only 答案没有源码引用**：本任务分析的是运行时 artifact，不要求强行引用仓库源码。

## 8. 分批修复任务与优先级

### Batch 0：客户正确性止血

| 顺序 | 工单 | 目标 | 最小验收 |
|---|---|---|---|
| 1 | `CBZ-B0-CPU-AVAIL` | CPU busy/idle 的 typed availability；身份无关 CPU lane 与线程身份 lane 解耦 | unrelated TID reuse 时 CPU busy/idle 仍由完整 CPU `sched_switch` lane 计算；无法计算时只显示 `unavailable`；不得出现 frequency-only `busy=0`；coverage 未 census 时打印 `not_evaluated`。 |
| 2 | `CBZ-B0-CAUSAL-AUTHORITY` | 无 frame/causal row 时发布确定性结论上限 | `frame_items=0` 或 frame identity degraded 时，系统块明确 `frame_causality=unproven`；`context_only` 和裸频率事件不能被系统投影为根因；不阻断模型答案出厂、不触发重试环。 |
| 3 | `CBZ-B0-COMPACTION-AUTHORITY` | capped 子集不得支撑穷举/总量 claim | `event_search/read_file` 的 emitted/total/complete 状态贯通 observation ledger；limit reached 时系统附注禁止“全部/仅有/共N/总计/最大最小”或标为 lower bound/sample。 |
| 4 | `CBZ-B0-SCHED-SEMANTICS` | state/priority/wakeup 语义校验 | Harmony `prio=20` 必须标 CFS；S 不得标抢占；R/R+ 的抢占仍需 priority relation；运行片段数不得冒充 wakeup count。 |

Batch 0 建议拆成四个独立提交，避免同时改 engine typed model、renderer 和 answer mutation 后无法定位回归。

### Batch 1：目标发现与 frame 降级路径

| 顺序 | 工单 | 目标 | 最小验收 |
|---|---|---|---|
| 5 | `CBZ-B1-SELECTOR-MISMATCH` | 保留 exact TID 路由，同时停止静默丢名称 | 同时给 `pid/thread` 且 comm 不符时，返回 exact target + typed mismatch；若名称在窗内唯一命中另一 TID，列为 candidate；多命中只列 roster，不自动选择。 |
| 6 | `CBZ-B1-FRAME-SCOPE` | 新增显式 thread/process frame scope | 默认保持 exact thread；只有显式 process scope 才按已证 TGID/SpanPID 成员扩展；generation 不明或 process membership 不唯一时 fail-close。 |
| 7 | `CBZ-B1-ROLE-PROOF` | 主线程/UI/render 角色必须有 typed authority | 名称相同只输出 candidate；main/UI/render 角色携带 source/confidence；无角色证据时不发布角色词。 |
| 8 | `CBZ-B1-LIFECYCLE-REMEDY` | lifecycle caveat 和 coverage reason 可操作 | 输出冲突 TID、boundary、受影响 lane、是否 global；给候选 selector/process scope；因果投影优先报告 incarnation suppression，不再只建议原参数重跑。 |

### Batch 2：背景信号口径和交叉校验

| 顺序 | 工单 | 目标 | 最小验收 |
|---|---|---|---|
| 9 | `CBZ-B2-IO-CALIBER` | 区分 count-only IO marker 与墙钟/延迟 pressure | 36 个 marker、零 D/io_wait/latency 时 signal 显式为 `blocked_reason_iowait_count_only`；score 组成和 evidence quality 跨原始行、typed observation、系统补充保持一致；不得称“高 IO 压力”。 |
| 10 | `CBZ-B2-ARITH-RELATION` | 数值关系复算 | 0.817/227.367 的百分比按统一容差复算；分子、分母或 completeness 不可定位时只发附注，不改写模型正文。 |
| 11 | `CBZ-B2-FREQ-AUTHORITY` | 频率 transition count 与供给不足分层 | transition count 保持 background；只有 typed residency/ceiling/CAP/deficit 证明才允许 low-frequency/supply-pressure 因果措辞。 |

### Batch 3：独立 UX 卫生

| 顺序 | 工单 | 目标 | 最小验收 |
|---|---|---|---|
| 12 | `CBZ-B3-ARTIFACT-LINKIFY` | artifact 文件名不再被识别为 mailto | `Other_trace...@69326-2310.sys.systrace` 在终端和 HTML 均逐字显示为 artifact/inline-code，不产生 `mailto:`；普通合法 URL/email 的既有行为不回归。 |

## 9. 建议的回归矩阵

后续实现至少需要以下 fixture：

1. **unrelated reuse**：TID A 在窗内复用，CPU lane 完整，目标 TID B 连续；CPU busy/idle 保留，B 的线程统计保留，A 的 identity-dependent 统计撤销。
2. **relevant reuse**：目标 TID 自身跨 generation；线程时长和 frame ownership fail-close，CPU-global busy/idle仍可计算。
3. **frequency-only + scheduler withdrawn**：CPU 有频率事件但 scheduler lane 不可用；输出 CPU 频率，busy/idle 为 unavailable，不是零。
4. **真实零 busy**：完整 CPU head/census 且整窗 idle；允许输出 measured `busy=0`，与 unavailable 可机械区分。
5. **PID/name mismatch**：exact TID comm 不符，名称唯一命中另一 TID；路由不变、候选披露、零自动重定向。
6. **process-scope frame**：同 TGID 多线程 frame span；thread scope 只命中 exact TID，process scope 命中已证成员，未知成员不猜。
7. **无 frame 证据**：有短 sched slices 和 IO context，但 frame timeline 空；系统结论必须是 causal unproven。
8. **capped event rows**：实际 12 个片段只发 8 个；任何 exact count/total 都被标为 incomplete/lower-bound。
9. **scheduler semantics**：S、R、R+，Harmony priority 20/41/159/160 的正负措辞。
10. **count-only IO**：36 个 iowait marker、零 D/io_wait/storage；不得渲染成 180ms 或确定高压根因。
11. **算术关系**：0.817ms / 227.367ms；应得到约 0.359%，不能与 0.44% 同时无提示出厂。
12. **artifact linkify**：含 `@` 的 Windows/Unix 文件名在终端和 HTML 双面 parity。

## 10. 最终处置判断

必须落地：

- Batch 0 全部四项；
- Batch 1 的 selector mismatch、frame scope/降级、role proof、lifecycle remedy；
- Batch 2 的 IO caliber 和算术/频率 authority；
- Batch 3 的 mailto 修复。

优先级最高的不是重新调整根因排序算法，而是先恢复证据可用性和结论边界：

1. 不把 unavailable 显示成零；
2. 不把无 frame/无 causal row 的 scheduler 观察写成确定丢帧根因；
3. 不把 capped 子集写成完整总量；
4. 不违背 typed 调度状态和优先级语义。

上述四项完成前，即使继续优化根因排序，最终答案仍可能建立在伪零、错误全集和错误调度语义之上。

## 11. 2026-07-24 客户复跑 `no_window.txt` 再审计

> 复跑工件：`/Users/han/opt/customlogs/no_window.txt`
>
> 再审计基线：`main@72c299be17865245f04c4e1aee0eb28f31e46389`
>
> 范围：核对前述批次在当前代码上的真实覆盖，解释 `4340` 的量纲和比较边界，并冻结下一轮分批施工计划。

### 11.1 直接结论

本次“因果投影仍没有链上根因”不能解释为“trace 已证明没有根因”。当前只能证明：

1. 用户明确给出的关注窗是 `69326.832743749..69327.060110624`，约 `227.367ms`；
2. 模型又对该窗按生命周期边界做了子窗查询；
3. 当前确定性补采没有选回唯一包住这些子窗的用户分析外窗，而是把这些 anchor-capable 查询窗判为不一致；
4. 请求的 typed analyzer face 同时命中 D-state 家族，于是补采走了 G4 whole-trace `root_cause_rank`；
5. 全 trace 和多个重叠子窗的 `context_only` 记录被合入同一投影，形成 `30/150/180/4340` 四个 IO 活动指数以及 `2330.912ms` 全 trace 频率背景；
6. 投影因已有一条背景 IO 行而非空，现有发布分支没有再发布 `frame_causality=unproven`、生命周期抑制和枚举不完整边界；
7. 因此客户看到的是“链上根因为空 + 一个很大的背景分数”，却看不到造成空链的首要结构原因，也看不到该分数不能回答丢帧因果。

附件中的系统投影选择了 `69326.833..69326.875`、约 `42.668ms` 的子窗作为树窗，同时明示“本报告数据来自 4 个查询窗”；明细又显示最大 IO 成员来自 `69326.012..69328.343` 的全 trace 窗。这两个口径都不是用户唯一指定的 `227.367ms` 关注窗。

### 11.2 深层系统 gap

#### `NW-01`：嵌套分析窗被误判为互相矛盾

`internal/tool/trace_query_supplement.go` 的 `traceSupplementDeriveWindow` 对 anchor-capable 视图调用 `traceSupplementConsistentWindow`，要求所有起止点在容差内完全相同。该规则能挡住 last-wins/多数投票，但没有识别以下合法形状：

```text
用户外窗 [A, D]
  ├─ 生命周期前子窗 [A, B]
  └─ 生命周期后子窗 [B, D]
```

当且仅当存在一个唯一、显式记录且包住其余所有 anchor 窗的外窗时，选回该外窗是精确信号，不是猜测：

- 不解析自然语言；
- 不做多数投票；
- 不取最后一次调用；
- 不把多个子窗求并集制造一个未被请求过的新窗；
- 两个互不包含的候选外窗仍 fail-open 为 inconsistent。

本客户样本恰好命中这个缺口。它使本应是有窗补采的请求降级成全 trace 补采，是 `4340`、`2330.912ms` 和四窗污染的上游原因。

#### `NW-02`：帧类请求的确定性补采不补 frame family

当前 `traceSupplementFamilyPresence` 只检查 rank、chain、window states、critical blocking 和两个 census；`traceSupplementViews` 最多补：

- `root_cause_rank`
- `critical_blocking_calls`

即使 `traceSupplementVsyncFamilyHit` 已精确命中“丢帧/jank/VSync”家族，补采也不会因为 frame evidence 缺席而执行 `frame_root_cause_bundle`。所以系统能补出通用背景 rank，却仍可能没有：

- frame timeline；
- frame/deadline；
- frame member TID；
- frame-bound wakeup/rank；
- `FrameEvidenceStatus` 的正确有窗权威。

最优修向是在 typed frame-family 命中且 frame evidence 不在场时，把 `frame_root_cause_bundle` 作为首个重视图；它不保证一定找到帧，但能保证系统执行了正确的、与用户窗口绑定的帧因果调查。找不到时必须发布 `unproven`，不能再用通用背景补采冒充帧调查。

补采还应保留 analyzer 的 typed `RuntimeTarget.Kind`：

- `process` 映射为显式 `target_scope=process`，只在 frame/span discovery 视图使用；
- `thread` 或未知保持默认 exact-thread；
- 绝不根据名字或相邻线程自动升级 process scope。

#### `NW-03`：非空背景投影吞掉 authority 覆盖块

`internal/tool/answer_document_mutation_runtime.go` 的唯一发布分支目前只在 `len(cluster)==0` 时调用 `runtimeTraceCausalProjectionCoverageBlock`。因此：

- 投影完全为空：能看到 `frame_causality=unproven`；
- 投影只有一条 `context_only io_pressure`：覆盖块反而静默消失。

这正是本次附件的实际形状。背景行存在不应消灭证据权限边界。

同一 helper 目前还只读取 `ObservationLedgerInput.ToolResults`，没有读取 `SystemTraceSupplementResults`。即使确定性补采产生了 frame/lifecycle/enumeration authority，最终发布也可能看不见。

修复要求：

1. coverage/authority block 与 projection cluster 独立构造；
2. cluster 非空时仍追加 coverage block；
3. authority 同时读取 model-dispatch 结果和 system supplement 结果；
4. block cap 下至少保住因果 lead 和 authority，明细/证据行先裁；
5. 增加真实 `materializeRuntimeTraceCausalProjectionBlock` 接线测试，不能只测 coverage helper。

#### `NW-04`：`4340` 的数值可见，尺度不可理解

当前实现位于 `internal/tracequery/query.go::computeIOPressureSummary`。完整公式是：

```text
activity_index =
    max(first block latency, first storage latency)
  + iowait_blocked_count × 5
  + D-state wall-clock ms
  + scheduler iowait wall-clock ms
  + page-cache churn × 0.2
  + file-IO event count × 0.1
  + file-IO MiB × 2
```

本客户的 `4340` 精确来自：

```text
iowait_blocked_count=868
868 × 5 = 4340
其余分量全部为 0
```

它的 typed 口径已经正确降为：

- `signal=blocked_reason_iowait_count_only`
- `evidence_quality=activity_marker_only`
- `score_caliber=count_weighted_activity_index`
- `pressure_conclusion=pressure_unproven`

所以 `4340` 没有可诚实回答的绝对“高/低”档位：

- 它不是毫秒；
- 不是百分比；
- 没有代码定义的设备无关阈值；
- 随查询窗长度、trace marker 密度和采集方式变化；
- 不能与 `*_ms`、其他 score caliber 或不同采集配置直接比较。

它只能用于同一 `score_caliber`、相同采集条件、最好同一 trace 且相同窗长的相对比较。当前答案虽写了“不证明高 IO 压力”，但没有展示 `868×5=4340`，也没有明确“绝对等级未定义/只允许同口径同窗长比较”，客户仍会自然追问“4340 到底高不高”。

本项只做软呈现优化：

- 展示 count-only 分解式；
- 展示合法比较域；
- 明确 `absolute_level=not_defined`；
- 保持 `context_only`、不入根因排序、不新增 hard gate、不按噪声分数触发重试或结论升级。

#### `NW-05`：模型正文仍可越过 typed 因果上限

附件正文把：

- `iowait=0` 的 blocked-reason marker；
- `context_only` IO 活动；
- CPU 频率/占用背景；
- `dh-irq-bind-0` 的局部切换片段

组合成确定的“IO 风暴/分布式死锁/导致丢帧”叙述，但系统投影自身只有背景行，并明确没有链上根因。该问题不是再调一次 rank 权重能解决，而是 `NW-03` 的发布接线必须确保 authority 与非空背景投影同时出厂。按既有裁定继续不硬阻断模型答案、不触发 retry loop，但系统块必须机械给出结论上限。

### 11.3 同事覆盖矩阵复核

| 工单 | 再审计判定 | 代码证据与处置 |
|---|---|---|
| `B0 CPU-AVAIL` | `partial`，确认 | indexed 主面已经区分 measured zero/unavailable，但 streaming `SchedulerHeadCoverage` 未设置 `SubjectCensusStatus`；`buildCoreClassStats` 仍无条件累加 unavailable CPU 的零值。进入 Batch 2。 |
| `B0 CAUSAL-AUTHORITY` | `partial`，确认 | typed `FrameEvidenceStatus/CausalConclusion` 已存在；只有 helper 单测，非空 cluster 发布接线缺失，absent/EN/补采结果没有完整 e2e pin。进入 Batch 1。 |
| `B0 COMPACTION-AUTHORITY` | `partial`，确认 | emitted/total/complete 已贯通；同样被“仅空 cluster 发布”限制，且无真实 materializer pin。与 `NW-03` 同根合并修。 |
| `B0 SCHED-SEMANTICS` | 生产语义基本 covered，验证 partial | Harmony priority 分类和状态语义在引擎中已有实现；同事要求的 `S/R/R+ × 20/41/159/160` 输出措辞矩阵尚无一组闭合 fixture。优先补测试，不先改判定。 |
| `B1 SELECTOR-MISMATCH` | 行为 covered，验证 partial | exact TID 路由、typed mismatch、单候选已测；多候选 roster/零自动选择未 pin。补测试。 |
| `B1 FRAME-SCOPE` | `partial`，但“零测试”表述不准确 | `frame_process_scope_test.go` 已覆盖已证成员、未知成员拒绝、incarnation fail-close、唯一成员锁定。缺 exact 多 UI 候选歧义 pin；非法非空 `target_scope` 当前静默归一为 thread，是真 gap。 |
| `B1 ROLE-PROOF` | covered | candidate-only 与 role authority 已落地；本轮不改角色门。 |
| `B1 LIFECYCLE-REMEDY` | covered，留档项仍在 | 冲突/boundary/lane/global/建议已在 typed authority；多冲突只披露首条仍是已知窄差，不是本次空链主因。 |
| `B2 IO-CALIBER` | 核心 covered，呈现 partial | count-only 已正确降级；缺正向 wall-clock/latency 升级臂 pin，以及 `NW-04` 的分解式/比较域。 |
| `B2 ARITH-RELATION` | helper covered，接线 partial | 复算 helper 有 zh 测试；`persistMergedAnswerDocument` 的唯一挂点没有正向 e2e pin，删除挂点现有 focused tests 不会红。补 e2e 与 EN。 |
| `B2 FREQ-AUTHORITY` | covered | transition count 仍为 background，typed supply evidence 才授权供给措辞；本轮只保回归。 |
| `B3 LINKIFY` | covered | 系统终端/HTML 双面已修。附件中的 `mailto:` 属于旧运行结果/模型正文样本，不据此重开已修生产面。 |
| `PARTDISC-1` | 生产目标 covered，过程证据 partial | F3 披露在场，F2 `partition_value_set_veto` 保留为未消费名位；缺真板 diff/逐值更强断言属于审计过程债，不应误改生产判簇。 |
| `CENSAME-1` | 生产目标 covered，过程证据 partial | cause-node 合取臂与结构看护已落地；identity 空行的窄 fail-open 和真板 diff 留痕进入收账，不作为本客户紧急生产改动。 |
| `fbf0920f3` | 合法契约同步 | 该提交只同步 tracediag schema/hash 与测试期望，没有生产判定变化；应在台账写明复核结果，不需回滚。 |
| 台账义务 | 未完成，确认 | open-gap ledger 的旧 P2 未挂本次 production witness，campaign 止于 §29.220，当前审计文档也未记录后续提交覆盖。进入 Batch 4。 |

### 11.4 冻结施工批次

#### Batch 0：审计与方案冻结

- 本节文档提交并推送；
- 固定 `NW-01..NW-05`、同事矩阵再判定和以下批次；
- 生产代码零改动。

#### Batch 1：有界帧因果恢复与 authority 发布

1. `NW-01`：唯一显式 enclosing anchor window 选举；互不包含窗口继续 fail-open。
2. `NW-02`：typed frame-family 缺证据时优先补 `frame_root_cause_bundle`；保留 typed target kind/scope。
3. `NW-03`：非空 projection 仍发布 causal/enumeration/lifecycle authority；合并 system supplement authority。
4. 拒绝非法 `target_scope` 字符串，不能静默降为 thread。
5. e2e pins：nested window、incomparable windows、frame absent、非空背景+unproven、补采-only authority、zh/en、block-budget。

#### Batch 2：剩余 CPU availability 面

1. streaming coverage 的 `not_evaluated/evaluated` typed 状态；
2. `CoreClassStats` 增加 busy/idle availability/reason；
3. unavailable CPU 不进入 class busy/idle 数值合计；
4. frequency-only class 显示 `busy=unavailable/idle=unavailable`，仍保留频率背景；
5. measured zero、mixed measured/unavailable、all unavailable 三组 fixture。

#### Batch 3：分数解释与高价值接线 pin

1. `NW-04`：`868×5=4340` 分解式、合法比较域、`absolute_level=not_defined` 的 zh/en 软披露；
2. IO wall-clock/latency corroborated 正臂；
3. selector 多候选 roster；
4. process-scope 多 UI 候选 fail-close；
5. scheduler `S/R/R+ × 20/41/159/160` 语义矩阵；
6. arithmetic materializer 接线与 EN pin。

#### Batch 4：台账收口和全量回归

1. 更新 `trace_analysis_open_gap_ledger_20260710.md`：挂 production witness、关闭已完成面、保留窄差；
2. 更新 `real_trace_campaign_20260705.md`：记录 CBZ/NW 批次提交、测试和 `fbf0920f3` 复核；
3. 补 PARTDISC/CENSAME 过程证据状态，不改 F2 或生产判簇；
4. focused tests、目标包测试、`go test ./... -p 4`、`git diff --check`；
5. 每批独立提交并立即推送 `main`，下一批只在上一批远端落稳后开始。

### 11.5 不变量

本轮修复不得破坏以下红线：

- incarnation 对线程/进程身份相关聚合继续 fail-close；
- CPU 全局区间数学与身份 lane 解耦，但数据缺失时只能 unavailable，不能猜；
- 数值 PID 继续是 exact identity，名字只产生诊断候选；
- process scope 只能显式或由 typed analyzer kind 传递，不能从 comm 猜；
- IO activity score 永远是软 guidance/context，不进入 hard gate；
- 没有 frame/deadline/typed causal row 时，背景观察不得升级为具体丢帧根因；
- 不通过硬阻断、额外模型重试或改写模型正文来“修”呈现问题。

### 11.6 实施进展

#### Batch 1：完成，`382e6baba`

- `traceSupplementDeriveWindow` 已支持唯一、显式、包住其余 anchor 子窗的 enclosing window；互不包含的窗口继续拒绝；
- typed frame-family 且没有 present frame evidence 时，确定性补采改为执行 `frame_root_cause_bundle`，不再用通用 rank/critical 家族替代；
- analyzer `RuntimeTarget.Kind=process` 已穿透为 frame bundle 的 `target_scope=process`；thread/未知不自动升级；
- 非法 `target_scope` 在 tool 边界明确拒绝，不再静默归一为 thread；
- causal/enumeration/lifecycle authority 已与非空 projection 同时发布，并读取 system supplement 结果；
- 新增 nested/incomparable window、frame-family view selection、真实 process-scope engine call、非空背景 + authority、supplement-only authority、非法 scope 的回归；
- 既有 VSync census 测试已更新为新的帧专用补采契约；
- 验证：focused 回归通过；`go test ./internal/tool -count=1` 通过（`162.485s`）；`git diff --check` 在提交前执行。

#### Batch 2：完成，`c1c7eba13`

- streaming `SchedulerHeadCoverage` 在 parse/integrity/incarnation/selector fail-close 时发布 `subject_census=not_evaluated`，正常/partial census 发布 `evaluated`；
- `CoreClassStats` 新增 busy/idle availability 与 reason；
- class 聚合只累加 measured/partial CPU，unavailable CPU 的零值不再进入数值合计；
- frequency-only class 保留 `max_freq`/`class_frequency_observed`，但渲染为 `busy=unavailable idle=unavailable`；
- measured + unavailable 混合 class 显示 `partial_cpu_busy_idle_coverage`；完整实测零仍显示 numeric zero + `measured`；
- 验证：focused 红绿回归通过；`go test ./internal/tracequery ./internal/tool -count=1` 通过（tracequery `66.041s`，tool `162.828s`）；`git diff --check` 在提交前执行。

#### Batch 3：完成，待本批提交号在 Batch 4 总账回填

- count-only IO 活动指数已在引擎 summary、direct/root typed observation 和 zh/en 系统投影三面披露精确分解；
- 客户样本现在明确显示 `868×5=4340`、`comparison_scope=仅同 score_caliber/采集条件/窗长`、`absolute_level=not_defined` 和 `pressure_unproven`；
- 未新增任何按活动指数触发的根因排名、结论升级、重试或 hard gate；
- 通用 score 图例已消除“报告永不列权重”与 typed count-only 精确分解之间的矛盾：通用图例仍不列系数，只有精确 count-only 行披露自身分解；
- 补齐 IO wall-clock/latency corroborated 正臂，确认其不继承 count-only 的等级未定义提示；
- 补齐 exact PID + name mismatch 的排序后多候选 roster，候选仍只作诊断，不自动改路由；
- 补齐 process scope 下两个已证 UI member 的 fail-close，保持原 process selector，不锁任一线程；
- 补齐 `S/R/R+ × priority 20/41/159/160` 引擎矩阵及 scheduler/priority authority 发布措辞；
- 补齐 arithmetic materializer 经 `ApplyAndPersistMutation` 的真实 EN 接线 pin，删除 persist 挂点会使测试失败；
- 验证：focused 红绿回归通过；`go test ./internal/tracequery ./internal/tool -count=1` 通过（tracequery `64.271s`，tool `161.000s`）；`git diff --check` 在提交前执行。
