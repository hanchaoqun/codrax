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

#### Batch 3：完成，`9c5dec781`

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

#### Batch 4：完成，待本收账提交推送

- open-gap ledger 已挂 `cpu_busy_zero.txt` / `no_window.txt` production witness，并勘正 §663：CPU/availability 子面关闭，FileIO/PageCache/storage contributor completeness 与复合派生物继续开放；
- campaign 已新增 §29.221，记录 Batch 0–3 提交、同事矩阵终判、诚实残余和不得重开的既有面；
- `PARTDISC-1` / `CENSAME-1` 已按“生产 covered、过程证据 partial”收账，F2 不消费、identity 空窄差继续留档；
- `fbf0920f3` 已确认只做 tracediag schema adjudication/hash/e2e 期望同步，无 rank/gate/priority/dedup/truncation 行为变化；
- 验证：`go test ./... -p 4` EXIT=0（tool `172.516s`、tracequery `69.763s`、tracediag `5.980s`）；`git diff --check` 通过；
- 本批仅更新三份账本，无新增生产逻辑；最终提交号由本提交本身承载。

### 11.7 独立核验与残余修复批（2026-07-24，campaign §29.222）

8 席对抗核验复证 §11.6 四批的客户实际形修复全部成立；两处判词修订：

1. **§11.3 LINKIFY 行勘正**：「附件 mailto 属旧运行结果，不据此重开」被证伪——`markdownext/artifact_literal.go` 标点表缺全角括号/引号/书名号族，客户 181 行精确形（全角括号直贴 artifact 名）在当时 HEAD 双面仍铸 mailto（红测试逐字复现后修复，四种 CJK 包裹形双面 pin）。
2. **NW-01 补注**：「被 guard 拒绝的调用窗可入选举」经代码亲证结构性不可达（guard 只拦零界调用、登记要求双时间界，互斥两腿已 pin）；真实残余是「显式近全 trace anchor 窗当选」形，现语义已如实冻结 pin，根修=typed 用户窗 lane（open gap ledger `NW-WIN-TYPED` 记档待批）。

同批修复：算术附注去重、D-state 回退不再顶替 frame 补采、空投影分区披露、pid-only cursor scope 侧信道（含继承目标误录游标）、compute_supply 可用性第三面、NW-04 next-step 接应行、NW-05 成文期软 directive、空 reason token。42.668ms 误归属挂 L4 BODY-vs-evidence 盲点 witness；时间戳对账 advisory 立案 `NW-TS-RECON`。明细与验证回执见 campaign §29.222。

## 12. 2026-07-24 客户复放 `no_window_2.txt` 再审计（修复后构建；只审计不施工）

> 复放工件：`/Users/han/opt/customlogs/no_window_2.txt`（同一 trace、同一请求、同一窗口的第三次运行）
>
> 再审计基线：`main@4eb24c141`
>
> 性质：gap 分析 + 最优方案冻结；本节零代码改动。

### 12.1 修复生效清单（逐项 verbatim 证据）

| 修复 | 复放证据（log 行） | 判定 |
|---|---|---|
| NW-01 窗选举 | 系统补采「frame_root_cause_bundle(窗 69326.832744..69327.060111)」:239 — 不再「全 trace 无时间窗」 | ✅ |
| NW-02 帧类补采 | 同上，补的是 frame bundle 且带 typed 目标；另跑 VSync census-lite | ✅ |
| 投影窗回归用户窗 | 「分析窗 69326.833~69327.060s，共 227.367ms」:85 — 不再锚 42.668ms 子窗 | ✅ |
| NW-03 非空投影发布 authority | 覆盖边界块在只有一条背景行的投影上照常发布（生命周期抑制+frame_causality=unproven+枚举权限，:157-229） | ✅ |
| NW-04 分解式 | 「分解=36×5=180.000；comparison_scope=…；absolute_level=not_defined」:112-114 | ✅ |
| NW-04 接应行 | 下一步第 1 条=count-only 升级墙钟建议 :142 | ✅ |
| LIFEMULTI 多冲突 | 8 个冲突 TID（50128-50131/50173-50176）全部披露（内容面；显示面见 NW2-01） | ✅（内容） |
| CBZ 枚举权限 | 「enumeration_status=incomplete…emitted=40,total=unknown…不能支撑全部/仅有/总计」:226-227 | ✅ |
| 频率权限 | 「transition_events=468，transition_authority=background_only…」:237-238 | ✅ |
| GAP-F1 mailto | 全文零 `mailto:`（:231 显示形见 NW2-09） | ✅ |
| ARITHDUP 去重 | 本次正文无 ms/% 关系形，未触发；无复现面 | — |

### 12.2 新 gap 与最优方案

#### NW2-01（P1，显示）：生命周期抑制 caveat 重复轰炸

**症状**：log :157-225 约 70 行——tid=50173 同一边界重复 ~8 次、50174/50175/50176 各 ~7 次，且同一 tid 的条目在 candidate_selectors 上不一致（有的带全进程线程名册、有的只有 pid）。

**机制**（代码亲证）：`answer_document_mutation_runtime.go:3204` 把每个 tool result 的 `authority.LifecycleBoundaries` **裸拼接零去重**——模型 ~10 次 trace_query × 每查询 LIFEMULTI cap 4 条 × 补采 result，同一物理边界被逐查询重复铸；selector 变体差异来自各查询的 ThreadSelection 不同（byte 去重救不了）。LIFEMULTI（引擎侧，per-query 正确）与 NW-03（聚合侧，零 typed 去重）组合放大。

**最优方案**：聚合点 typed 去重——key=(ConflictTID, BoundaryLine)；同 key 变体合并取**最富形**（candidate_selectors/suggested_queries 取并集或最长名册）；披露 roster 全局 cap（建议 8）+「另 N 条同界披露省略」诚实尾句；渲染序=boundary_ts 升序。纯显示/聚合层，零 gate，精确信号。**同批**处理 NW2-05。

#### NW2-02（P1，正确性根因；台账 §663 剩余面第二产线实证，建议升 P1）：无关 TID 新建连坐清空 pid-keyed 因果车道

**症状**：本复放**空因果树的真正结构原因**——窗内外 8 个无关新任务（50128-50131 在窗前 0.8s，50173-50176 在窗内）触发全局身份冲突，`affected_lanes=pid_tid_scheduler_aggregates,process_domain_census,pid_keyed_resource_aggregates` 全窗撤销 → rank/chain 零行 → 投影只剩一条背景 io 行。目标 32788 与 app 线程 33410 等**自身连续无换代**，被 5017x 的创建连坐。

**机制**：`byCPU`（线程时长/供给/rank 的输入）仍由 `schedulerDurationsSafe`（含 `threadIncarnationConflictForQuery(idx,q,0)` **全局** onlyPID=0）门控——B-2/3bcfa33af 只解耦了 CPU busy/idle 面；§663 勘正时明确剩余面仍开放，pid-keyed 调度聚合面即在其中。

**最优方案**（分阶段，值通道批，须旗舰双复核）：
- **阶段一（高值）**：per-PID 冲突收窄——`threadIncarnationConflictForQuery(idx,q,onlyPID)` 已存在；线程级聚合（ThreadDuration/TopRunning/RunnableTop/rank 席/chain 成员）按**该行主体 TID** 查冲突，仅冲突 TID 的身份账撤销，无边界 TID 的单代账保留；每车道披露收窄口径。不变量：跨代合并依旧禁止（单代 TID 不存在跨代合并问题，前提由 per-PID 边界查询保证）。
- **阶段二**：process census/复合派生物的 completeness 语义（§663 原文的 typed completeness 前置）。

**witness 挂账**：cpu_busy_zero.txt（busy=0 面）+ no_window.txt/no_window_2.txt（空因果树面）= 两案三件。**下一个开发批的最高杠杆项**：修后本客户复放将获得真实 rank/chain 行而非空树。

#### NW2-03（P2，正确性）：frame_evidence_status 把「无关全局撤销」误判成「帧证据被撤销」

**症状**：:225「frame_causality=unproven，frame_evidence_status=**unavailable**」——语义=帧证据「被身份冲突撤销」；但本窗冲突 TID 全部 `affects_target=false`，帧缺席的真实原因大概率是「该窗本就无该目标的帧标记」（应为 **absent**）。unavailable 误导读者以为分窗/换 selector 能救回帧。

**机制**：`traceQueryResultHasAuthorityWithdrawal`（trace_query.go）对 result 全部 caveats 做 `thread_incarnation_conflict` 裸子串匹配——全局无关冲突的生命周期 caveat 也命中，withdrawal 判定不区分 affects_target。

**最优方案**：withdrawal 判定 typed 化——读 `LifecycleSuppressions.AffectsTarget`（或 frame_ownership_status=unavailable）而非裸子串；仅目标受累时判 unavailable，否则 absent（诚实缺席）。精确信号（typed bool 已随 LIFEMULTI 逐边界携带），零 gate；absent/unavailable 二臂 pin（TESTS-2）扩一臂「全局无关冲突在场仍 absent」。

#### NW2-04（P2，披露通道）：selector mismatch/角色候选诊断未达答案面

**症状**：pid=32788 在 trace 中 comm=unknown（探索期模型自己发现目标不精确匹配），B-1 修复的 typed mismatch+candidate 诊断在引擎/工具层已铸，但最终答案零披露——正文继续称 32788 为「主线程/aweme 主线程」（CBZ-04 角色越权第二实证）。

**机制**（代码亲证）：`answer_document_mutation_runtime.go` 全文**零** name_candidate/selector mismatch 消费点——该诊断没有确定性出厂通道，全靠模型自觉转述。

**最优方案**：覆盖边界块增一条 typed 行：`目标身份: pid=32788 解析 comm=unknown，与请求名 ss.hm.ugc.aweme 不符；同名候选=tid 33410 等（诊断性，路由未改）`——数据源=ThreadSelection typed 字段（已在 wire），铸点与 lifecycle 边界同块；零 gate 零改路由。

#### NW2-05（P2，与 NW2-01 同批）：窗外边界混入投影覆盖块无窗别披露

**症状**：50128-50131 的边界 ts=69326.022~69326.033，在用户分析窗（69326.833 起）**之前 0.8s**——来自 whole-trace 查询（census-lite/window_sweep）的抑制记录，聚合时与窗内边界（50173-50176）并列无区分，读者无从判断哪些边界真正压制了本窗证据。

**最优方案**：边界行标注「窗内/窗前/窗后」（相对投影分析窗，精确比较）；窗内优先排序，窗外折叠进「另 N 条窗外边界」尾句（与 NW2-01 的 cap/去重同一改造点）。

#### NW2-06（P3，词面升级为应做）：lifecycle 叙述误读第二实证

模型正文再次把审计边界叙述成「aweme 线程 reincarnation 4 次…FFRT 线程必须执行 mprotect_range…」（50173-50176 是**新建任务**的身份审计边界，非 aweme 线程重建；mprotect 等是新任务自己的 blocked_reason）。上轮列为可选的措辞补句现有第二 witness：suppression caveat 文首加「边界为身份审计边界，非目标线程销毁/重建证明」。

#### NW2-07（P3，挂账）：4.3ms=窗口偏移被当阻塞时长（TS-JOIN 家族第二实证）

正文「timerfd_read 阻塞…持续约 4.3ms」：69326.837087−窗口起点≈4.34ms 是**偏移**非时长（blocked_reason 为瞬时事件；上轮 42.668ms=phase 窗长同族）。挂 TS-JOIN/L4 witness #2，维持设计轮前置。

#### NW2-08（P3）：跨面计数并存（172 vs 468）

正文「clock_set_rate 172 次」vs typed 注「transition_events=468」同报告并存（疑窗内 vs 全 trace 计数，合并面取 max）。频率权限注已封因果上限；残余为读感，随 batch3 既有残余（freq 合并路径零专属测试）一并处理时补窗别披露。

#### NW2-09（P3，维持记档）：`Other_ trace_...` 空格显示形

:231 全角括号语境下 artifact 名以「前缀 text+尾段 code span」显示（R4 已裁接受形），第二 witness 落档；若未来判读感不可接受，改造方向=CJK 触发位置回溯扩展（claim 全名），中等成本。

### 12.3 施工批次建议（冻结，不实施）

- **Batch N1（P1 显示，小）**：NW2-01+NW2-05+NW2-06——聚合 typed 去重+窗别标注+cap+措辞句；纯披露层；先红后绿=以本复放 8 边界 ×10 查询形构造聚合 fixture。
- **Batch N2（P1 根因，大；值通道批）**：NW2-02 阶段一 per-PID 收窄——独立旗舰双复核+逐席 diff 追审；验收=no_window 复放形出真实 rank/chain 行、冲突 TID 自身账仍撤销、§663 更新。
- **Batch N3（P2，小）**：NW2-03 withdrawal typed 化 + NW2-04 目标身份披露行——同为覆盖边界块改造，可与 N1 合批。
- 挂账不动：NW2-07（TS-JOIN 设计轮）、NW2-08（freq 合并 pin 既有残余）、NW2-09（R4 记档）。

### 12.4 不变量（重申）

per-PID 收窄不得弱化：冲突 TID 自身的跨代合并禁令、comm≠身份、process scope 显式化、「缺失≠0」渲染纪律；NW2-01 去重不得丢失任何**不同物理边界**；NW2-03 改判后 absent 仍须与 unavailable 机械可区分（既有二臂 pin 扩臂不改语义）。

### 12.5 `main@2e8663059` 冷读复核与施工契约（2026-07-24）

本节是在同步 `origin/main` 后，对 §12、`no_window_2.txt` 和生产调用闭包重新冷读的校准。§12.1 的修复生效判断不变；九项残余中，`NW2-01/02/03/04/05/06` 确认进入本轮施工，`NW2-07/08/09` 继续挂账，不借本轮显示/身份收窄批顺手改变时间语义、频率合并或 artifact claim 规则。

#### 12.5.1 机制复核

| gap | 当前生产机制 | 冷读终判 |
|---|---|---|
| NW2-01 | `runtimeTraceCoverageAuthority` 跨 ToolResult/SystemTraceSupplementResult 直接 append `LifecycleBoundaries`，仅排序、无物理边界去重/合并/cap | confirmed |
| NW2-02 | `ComputeWindowStats` 的 `schedulerDurationsSafe` 仍含 `threadIncarnationConflictForQuery(idx,q,0)` 全局门；`byCPU` 与 `computeOffCPUStats` 因任意 PID 冲突整体空产 | confirmed，但 §12.2 的“改 onlyPID”表述需收窄为“按实际贡献者过滤” |
| NW2-03 | `traceQueryResultHasAuthorityWithdrawal` 对 caveat 裸匹配 `fail_closed/thread_incarnation_conflict/lifecycle_audit_truncated`，没有读取已有的 `LifecycleSuppressions.AffectsTarget`/`FrameOwnershipStatus` | confirmed |
| NW2-04 | mismatch 已铸成 typed observation `thread_selector_exact_name_mismatch`，但覆盖边界 materializer 没有消费该 predicate | confirmed；不扩 wire authority，直接消费既有 ledger typed record |
| NW2-05 | lifecycle 聚合没有分析窗输入，whole-trace 查询的窗前边界与用户窗内边界同席 | confirmed |
| NW2-06 | 当前 caveat 只说 lifecycle suppression/remedy，没有声明“审计边界不是目标销毁/重建证明” | confirmed |

#### 12.5.2 NW2-02 冷读修正

不得把全局 guard 简单替换成 `threadIncarnationConflictForQuery(idx,q,q.PID)`：当查询目标本身干净、背景 PID 冲突时，这会让冲突 PID 的两代 scheduler 行重新进入聚合，违反 incarnation 红线。阶段一必须按**每条输出的实际身份贡献者**执行：

1. `sched_switch` 区间仍完整保留为 CPU 区间边界；CPU busy/idle 继续走身份无关车道。
2. 写入 `ThreadDuration`、`TopRunning`、runnable/off-CPU、线程级 blocked-reason、rank/chain 输入前，以该行主体 TID 调用唯一的 per-PID incarnation authority；冲突 TID 的全部身份时长撤销，干净 TID 保留。
3. 同一 PID 在窗内发生 generation 边界时，阶段一不尝试切代后分别归集，而是整 PID fail-close；避免把同号两代重新相加。
4. scheduler parse/integrity failure 继续是全车道 fail-close；lifecycle-audit cap 继续 fail-close，不把“未审完”当“无冲突”。
5. `ProcessDomainCensus`、FileIO/PageCache/storage 及依赖进程成员完整性的复合派生物不因本批自动解封；它们继续等待阶段二 contributor completeness。
6. 本批验收不是“保证客户必有链”。确定性验收是：无关冲突不再清空干净 TID 的线程级值通道；若 trace 内确有严格 wakeup/runnable 证据，rank/chain 可据此自然产出，否则诚实保持空产。

#### 12.5.3 冻结批次与提交边界

- **Batch N0（文档冻结）**：本节；独立提交并推送。
- **Batch N1（覆盖/权限面）**：合并原 N1+N3，落地 NW2-01/03/04/05/06。
  - lifecycle 物理 key=`ConflictTID+BoundaryLine`；`BoundaryLine<=0` 时以 `ConflictTID+BoundaryTs` 作 typed fallback，禁止把不同物理边界折叠。
  - 同 key 的 selectors/queries/lanes 做稳定并集，`AffectsTarget` 取 OR，frame ownership 取更严格权限；窗内优先、物理边界全局 cap=8，披露 unique omitted/outside-window 计数。
  - 分窗只读 projection/ledger 已有 typed `selected_window`；窗口不唯一或缺失时标 `window_relation=unknown`，禁止猜窗。
  - lifecycle withdrawal 在 typed suppression 在场时只由 `AffectsTarget=true` 或 `FrameOwnershipStatus=unavailable` 铸成；无 typed suppression 的旧结果保留 legacy fail-close fallback。
  - mismatch 从 ledger 的精确 predicate 与 typed rich-note keys 读取，只作诊断披露，exact PID 路由不变、候选不获角色 authority。
- **Batch N2（per-PID 值通道）**：只改线程级 scheduler identity 聚合和直接消费者；冲突 PID 撤销、干净 PID 保留、全局 integrity 与阶段二复合面不动。至少覆盖：
  1. 无关 TID 冲突下干净线程 running/runnable/off-CPU 保留；
  2. 冲突 TID 自身三类值全部撤销；
  3. lifecycle audit cap 仍全撤销；
  4. 根因排序只读保留下来的干净线程值；
  5. 严格 wakeup 证据在场时链可产出，证据缺席时不造链。
- **Batch N4（收账与全回归）**：回写 §663/real-trace campaign、记录每批 commit 与测试；执行 focused、`go test ./internal/tracequery ./internal/tool`、`go test ./... -p 4`、`git diff --check`，每批远端落稳后才进入下一批。

#### 12.5.4 显式不承诺

- NW2-02 阶段一恢复的是“可计算且身份安全的输入”，不是用 fallback 制造根因或保证某条唤醒链存在。
- NW2-03 的 `absent` 表示本次没有目标绑定帧证据，不等于“没有掉帧”；`unavailable` 只表示证据确被目标相关权限撤销。
- lifecycle roster 的 cap 只压显示，不改变 engine 每查询 suppression、typed observation 或任何 hard gate。

### 12.6 Batch N1 完成：覆盖/权限面

- `NW2-01`：跨查询 lifecycle authority 已按物理边界合并；同界 selectors/queries/lanes 做稳定并集，目标影响取 OR，frame ownership 保留更严格权限。
- `NW2-05`：单 artifact 且 projection 有唯一 typed 分析窗时，只展开窗内边界；窗外物理边界折叠为 `outside_window_boundaries=N`。无唯一窗时每行明确 `window_relation=unknown`，不猜。
- roster 全局最多展开 8 个 unique 物理边界，尾部以 `omitted_unique_boundaries=N` 保持显示压缩的信息守恒。
- `NW2-06`：中英文均声明 lifecycle 是身份审计边界，不是目标线程销毁/重建/反复 incarnation 的证明。
- `NW2-03`：frame withdrawal 优先读取 typed `AffectsTarget`/`FrameOwnershipStatus`；无关 PID 的 lifecycle caveat 不再把 `absent` 改成 `unavailable`；typed roster 缺席的 legacy 结果和 `lifecycle_audit_truncated` 继续 fail-close。
- `NW2-04`：答案覆盖块消费既有 `thread_selector_exact_name_mismatch` typed observation，披露 requested/selected/routing/candidates；exact PID 路由不变，候选不获得角色权限。
- selector rich-note 新消费者已按 NKR 三步协议进入唯一键注册表，生产/消费端同用常量，golden 与全覆盖 emit fixture 同步。
- 回归：新增重复边界×10 查询、窗内/窗外、cap、富形合并、typed irrelevant/affected/truncated 三臂、selector 出厂通道 fixture；`go test ./internal/types -run TestTraceNoteKey`、focused tool tests、`go test ./internal/tool -count=1`（`163.112s`）通过；`git diff --check` 通过。
- 提交号由承载本节与生产改动的 Batch N1 提交记录。

### 12.7 Batch N2a 完成：per-PID scheduler 值通道

本批只完成 §12.5.3 的线程值通道收窄；严格 wakeup chain 另以 N2b 独立提交，避免把“聚合恢复”和“依赖边扩展”绑成一个不可单独回退的大提交。

- 新增 query-local `queryPIDIdentityFilter`。当全局 lifecycle probe 无冲突时为零额外拒绝；有冲突时按**实际贡献 PID**读取该 PID 的唯一 generation scope；该 PID 的查询窗跨 boundary 时整 PID 撤销，不拆代、不相加。
- `lifecycle_audit_truncated` 保持全 PID fail-close：审计未完整时不能把未列出的 PID 当安全。
- scheduler parse/同 lane 顺序 integrity 仍是全局门，本批不弱化。
- `sched_switch` 的 CPU 区间完整保留；CPU busy/idle 与 compute-supply 继续按身份无关区间数学计算。写入 running/pressure/ThreadCPULoad 前才过滤冲突 PID。
- runnable、sleep、D-state、IO-wait、blocked-reason census、state churn、scheduler latency 与 constraint 行均复用同一 PID authority；冲突 PID 的 open state 会被删除，不能在后续闭合点重新进入。
- 流式 `state_cluster` 同步从“任一冲突清空全部”改为按完整冲突 PID 集删除对应 rows；scheduler parse/order failure 仍清空全部；head subject census 在身份冲突时仍诚实标 `not_evaluated`。
- `ProcessCPULoad`、`CPUOccupancy`、`ProcessDomainCensus` 和 PID-keyed resource composite 继续全局 fail-close，等待阶段二 contributor-completeness；本批没有用“干净线程子集”冒充完整进程。
- blocked-reason 物理事件总数仍可作采集 inventory；会进入 IO 压力/根因值通道的 `IOWaitBlockedCount` 和 per-PID census 只累计 identity-safe wakee PID。
- authority caveat 明确区分：`suppressed_pids=[…]`、干净 PID rows retained、进程/资源复合面 withheld；不再把局部过滤描述成全部 scheduler 值消失。
- 回归覆盖：
  1. 无关 PID=900 换代时 PID=100/200 running 与 ThreadCPULoad 保留、PID=900 不出现、process rollup 仍空；
  2. 干净 PID 的 running/runnable/sleep/churn 保留，冲突 PID 在五个状态 lane 与 churn/load 中均缺席；
  3. 目标 PID 自身跨代时其 running/runnable 仍为空；
  4. lifecycle audit cap 时所有 PID 时长仍撤销，但 CPU busy 区间继续存在；
  5. indexed 与 streaming 两条 state 聚合路径的 per-PID 判定一致。
- N2b 待办保持显式：`BuildWakeupChain` 仍有全局 incarnation guard；下一提交只允许干净 target 与干净依赖成员展开，任一冲突依赖分支必须原地停止且不得制造 fallback edge/trace-gap evidence。

### 12.8 Batch N2b 完成：严格 wakeup chain 的 per-PID 依赖闭包

- `BuildWakeupChain` 的入口不再因任意无关 PID 的 lifecycle boundary 清空整条链；入口先单独验证目标 PID 在**完整用户查询窗**内只有一个 generation scope。
- 目标 PID 自身跨 boundary 或 lifecycle audit capped 时，既有 `wakeup_chain_fail_closed` 保持，整链仍为空。
- 目标干净但全局存在无关冲突时，authority caveat 明确声明 per-PID 策略；这只是允许继续读取严格证据，不保证链存在。
- 每次 `expandChain` 在铸 `ChainNode`、`CausalImpact`、`RootEvidence` 或 `WakeupEdge` **之前**，以原始完整查询窗验证当前 dependency PID。禁止用递归子窗“碰巧只覆盖一代”来绕过全窗身份审计，也禁止跨不同 branch 把同号两代重新并为一个 actor。
- 冲突 dependency 分支直接返回空 child id；已找到的物理 wakeup 仍被识别为“证据在场但依赖身份无权限”，调用方不会把它误写成 `missing_wakeup`。因为 node 尚未铸造，也不会产生该 PID 的 `trace_gap`。
- chain 的 Binder/peer 辅助闭包同步收窄：**带 exact target 的** `IPCGraph` 对具体 sender/receiver PID 逐端点验证，`InteractionStats` 对每个 wakeup/Binder peer 验证；干净 target 不再被无关冲突连坐，冲突 peer/endpoint 不能通过辅助边重新进入因果证据。无 target 的 IPC 物理 inventory 保持原行为，由既有 pairing/join authority 披露，不把本批因果投影规则反向套到原始清单面。
- 严格 wakeup credential、边界 tolerance、priority authority、MaxDepth/MaxBranches/MaxChainNodes 与排序逻辑均未改；没有增加 fallback edge、comm 路由或启发式因果。
- 回归覆盖：
  1. 目标 PID=100 与 waker PID=200 均连续、无关 PID=900 换代时，严格 `200→100` wakeup edge 和 clean root-rank row 正常产出，PID=900 不进入 node/rank；
  2. 目标干净但物理 waker PID=900 跨代时，PID=900 不产生 node/edge，结果披露 `thread_identity_dependency_fail_closed`，且没有把拒绝转换成 `missing_wakeup`/`trace_gap`；
  3. 无关冲突下 clean Binder `100→200` 保留；receiver PID=900 跨代时 Binder edge 与 interaction peer 均撤销并披露 `suppressed_pids=[900]`；
  4. 目标 PID 自身跨代的既有 adversarial fixture 继续整链 fail-close。
- 验证：focused chain/interaction/IPC/target-conflict fixtures 通过；`go test ./internal/tracequery -count=1` 通过（`69.242s`）；`git diff --check` 在提交前执行。
- 本批关闭 NW2-02 阶段一的 chain 成员面；process/resource contributor completeness（阶段二）继续开放，不能据此宣称 §663 全部关闭。

### 12.9 Batch N4 完成：状态收口与全仓回归

- `trace_analysis_open_gap_ledger_20260710.md` 已挂 `no_window_2.txt` production witness，并把 §663 精确拆为：覆盖/权限、线程值、因果依赖三面已关闭；process/resource contributor completeness 阶段二继续 P2 开放。
- `real_trace_campaign_20260705.md` 新增 §29.224，记录 N0/N1/N2a/N2b 的提交边界、反例矩阵与显式不承诺。
- 三个生产提交均已先后推送 `main`：N0=`1ce7240b2`，N1=`e8b13ce30`，N2a=`c5923758e`，N2b=`fcc465c75`。
- 回归：`go test ./internal/tool -count=1` 通过（`196.080s`）；`go test ./... -p 4` EXIT=0（尾部重量包 tool `194.496s`、tracequery `77.737s`、tracediag `6.270s`）；`git diff --check` 在提交前执行。
- 客户原始日志/trace 未入仓；仓内只提交最小合成 fixture。当前代码能恢复所有身份安全的计算输入，但仍须客户用包含本批提交的新构建复放，才能确认其原 trace 是否实际携带足够的 strict wakeup/frame/deadline 证据。

## 13. 2026-07-25 客户第四次复放 `no_touying.txt` 再审计（零投影双因 + VSync 周期语义 + 七新 gap）

> 复放工件：`/Users/han/opt/customlogs/no_touying.txt`（同一 trace/请求/窗口，第四次运行）
>
> 再审计基线：`main@9a57f773b`；三席对码审计（why-empty/vsync-semantics/fresh-gaps），本节零代码改动。
>
> **判别实验已确认安排**：客户第五次复放将使用 ≥`9a57f773b` 构建（用户 2026-07-25 确认）——预期因果树至少出现目标自身状态行（全窗 sleep/D 的 self 席）且 4 边界翻 `in_window`；若仍全空则 R3 实锤。

### 13.1 修复生效清单（vs 第三次复放）

重复墙灭（4 边界单块一次，NW2-01）；目标身份行首现（requested_pid=32788/selected_thread=unknown-32788/routing=exact_tid_preserved/11 候选，NW2-04）；审计边界句在场且正文本次无 reincarnation 误读（NW2-06）；window_relation 机器在场（值全 unknown，见 NG-3）；帧族补采披露（NW-02）；频率/枚举权限约束生效；尾注降级披露三面齐。

### 13.2 零投影双因与断代判定

**R1（主判，~60-70%）构建断代**：行为学标记=第三放投影唯一背景行 `io_pressure 36×5=180` 本次消失，全仓唯一触该值通道的提交是 `c5923758e`（N2a：IOWaitBlockedCount 只计 identity-safe wakee，36 事件多属冲突任务 5017x）⇒ 构建 ≥c5923758e；而链/self/trace_gap 行全不铸=N2b（`fcc465c75`）缺位的全局 guard 面貌 ⇒ 最简一致区间 `[c5923758e, fcc465c75)`。fresh-gaps 席以词面弱证据另判 N1-era——以行为学标记为准但未定谳，第五次复放定谳。

**R3（独立新洞，无论断代必查，本地复现最高优先）补采锚悖论**：成功披露的 `frame_root_cause_bundle` 补采按 HEAD 接线必铸 `frame_target_resolution` 锚观测（query.go:20186-20199 → trace_query.go:7662-7728）→ 窗必可知，与本放全边界 `window_relation=unknown` 静态矛盾。铁证=补采披露行本放为「目标 `ss.hm.ugc.aweme [32788]`」（**TargetPID=0 原串形**，含方括号），第三放为「`ss.hm.ugc.aweme-32788`」（pid 追加形）——supplement 目标推导本放退化（疑 analyzer runtime_target 以标签原串入 Thread 且 PID 未解析，或 process-scope 空目标分支）。复现方向：审计 `traceSupplementDeriveTarget` 用户车道对「name [pid]」标签形的处置 + 断言成功 bundle 的 Observations 含 frame_target_resolution 且 ledger 编译后窗 known。

**机制推演结论（per-PID 全在位时本查询形的正确产出）**：目标全窗睡 ⇒ self 症状行（target_self_state，Rank=0 不占席但铸观测、锚窗、进投影自身状态行）+ 可能 missing_wakeup/trace_gap 行；timer 唤醒无 sched_wakeup 边则 ⊘链止；干净 app 线程非查询目标不获席——即修复全在位时**树不会全空但根因席可以诚实为零**，unproven 与 self 行并存是设计内。

### 13.3 VSync 周期等待语义判定（用户问：正常等待的 sleep 不能算影响，系统算得准么）

**准确面（S 态三层保护，全部锚定）**：①目标自身 sleep 等待=等待症状族 → `target_self_state`、Rank=0 不占席（rootCauseItemIsTargetWaitSymptomType，registry wakeup_chain/lock_contention 车道）；②◎ 可消除榜结构性排除（rank 席载体要求+素状态词退榜，§29.175.17）；③VS-1 周期唤醒源双向折扣（周期 waker tick 间 sleep 不算 + 目标期内等待减周期，DominantState==S 前提）。本放系统未把 sleep 算进任何影响=语义一致。

**缺失面（三 gap + 一裁定）**：
- **GAP-B1（P2，显示）目标窗内状态账 typed 行**：系统说不出「目标主导状态=等待 timerfd（iowait=0 已证非 IO），窗内被唤醒 N 次；该等待未计入任何可消除影响」——真空由模型叙事填补。数据全在 wire（TargetWindowStateAccount/blocked_reason census/offCPUCauseSymbol），覆盖边界块与 NW2-04 同块加行，零 gate。
- **GAP-B2（P2，值通道，独立旗舰双复核批）D 态 timer 周期等待洞**：本客户目标恰为 `timerfd_read` **D 态**等待——D 形不在等待症状闭集（io_blocking 车道，SYM-2 设计可参赛可加冕可入 ◎），VS-1 周期折扣排除 D（DominantState==S），全引擎零 timerfd 识别 ⇒ **正常周期等待在 D 车道机制上确会被广告为可消除影响**（本放因空产未发生）。方案=(a) pacing 臂解 binder 核销偶合（query.go:14532 len(rejectedTxns)>0 偶合前提）；(b) VS-1/pacing sleeper 准入扩「D ∧ caller∈timer 闭集」（timerfd_read 首证，survey-derived 闭集禁猜，vsyncGeneratorThreadNames 先例）。
- **GAP-B3（P3，显示）vsync 周期权威不上答案面**：census typed 观测在账本零渲染——模型以 capped event_search 样本（6 次 vs「理论 13-14」/16→33ms）冒充周期权威无注约束。方案=census 在场∧正文涉 vsync 时确定性注（发生器权威 period；消费者回调间距只测帧节拍非信号周期）。
- **GAP-B4（P3，先裁后修）**：跨多周期正常空闲（未请求帧）的正面「无异常」宣称有 backfill 红线风险，先裁词形；与 missing_wakeup 区分依据=窗内 wake census。

### 13.4 新 gap（NG-1..NG-7）

| # | 定级 | 症状与机制锚 | 最优方案 |
|---|---|---|---|
| NG-1 EMITBURN-2 | **P1 立案** | emit_analysis strict-decode 一次一字段燃轮（本放 3 连拒 2m51s）：DisallowUnknownFields 首字段即中止，reject 无 JSON 路径无出现次数无全清单，hints=nil（strict_decode_params.go:25-33/strict_decode_remap.go:125）；EMITBURN-1 只覆盖 fact 校验层 | decode 失败对 raw JSON 走查，一条 reject 枚举全部 unknown key+路径（`runtime_targets[0].description, referenced_artifact_lines[0].description,…`）；emit_analysis 挂 description 的 MisplacedFieldHint。纯报告层零判定放宽 |
| NG-2 NW2-03b | **P2 立案** | generic `fail_closed` 子串臂逃逸：typed roster 在场时仅豁免两 token（trace_query.go:773-781），:782 generic 臂对 `thread_identity_resource_fail_closed`（query.go:3230，无豁免 token）/pairing 族照旧翻 absent→unavailable——本放 unavailable 疑此铸 | typed roster 在场时 unavailable 单源化到 typed AffectsTarget/FrameOwnershipStatus+lifecycle_audit_truncated；generic 臂收窄为 frame-lane 专属 token 白名单 |
| NG-3 | **P2 立案** | projection 零产时 window_relation 全 unknown（4 边界 ts 全在用户窗内）——runtimeTraceCoverageAnalysisWindow 仅认恰一 projection，§12.5.3 冻结契约的 **ledger** selected_window 臂未实现 | projections==0 时读 ledger 全部 typed selected_window note，全记录唯一一致才采信（非猜窗）；不一致维持 unknown |
| NG-4 | P2-P3 | 「系统补充：结构化指标核对」名实不符：零数值比对纯回显（evaluator.go:13788-13861，C#19 比对通道存在未接）；zh 维度标签 token 化仅 [a-z0-9_] 无 CJK 臂 ⇒ 只出 cpu 项 | ①词面改「结构化指标摘录」或接 C#19 真比对；②token 化补 CJK 臂 |
| NG-5 | P3 记档（并 B-1） | 兄弟线程名嫁接：正文把 OS_VSyncThread-38326 当目标 32788 本体+「主线程」——NW2-04 披露射程只含同名候选，不含非候选兄弟名 | 身份披露句扩臂（同进程其它线程名不得代表目标 tid 本体）+成文 soft directive；随 B-1 排批 |
| NG-6 | P3 频次档 | NW-05 causal ceiling hint 三连无感（witness #3）；正文 2728/1936/792 cpu·ms 三数在引用/指标核对/系统块全零出处（疑 extract fact 无用户可验面） | 记入 NW-05 频次档供裁定重开作证 |
| NG-7 | P3 记档 | 结构化原因类级披露无实例归属（「trace_query 执行失败」不指明 view/轮次，按 reason 串去重掩盖数量） | 记档；若立案则披露 view+第几次 |

### 13.5 施工批次（冻结）

- **R3-REPRO（最高优先，先复现后修）**：supplement 目标推导退化——复现「name [pid]」标签形 RuntimeTarget 在补采车道的产物（TargetPID=0 原串形），修向=用户车道标签形解析（引擎已有 bracket 选择器解析先例，精确信号）+成功 bundle 锚观测断言 pin。
- **Batch F1（P1/P2 小批）**：NG-1 EMITBURN-2 + NG-2 NW2-03b + NG-3 ledger 窗臂。
- **Batch F2（显示批）**：GAP-B1 目标窗内状态账行 + GAP-B3 vsync 权威注 + NG-4。
- **Batch F3（值通道，独立旗舰双复核）**：GAP-B2 D 态 timer 周期等待扩臂。
- 记档不动：NG-5（随 B-1）/NG-6/NG-7/GAP-B4（先裁）。

### 13.6 不变量

GAP-B2 不得弱化：D 态 fail-close 红线、SYM-2 自因可拆解 D 候选的既裁地位（扩臂只对「caller∈timer 闭集 ∧ 周期性成立」的子形折扣，非 D 全族豁免）；NG-2 收窄不得把真目标撤销改判 absent（absent/unavailable 机械可区分性维持）；NG-3 仅唯一一致才采信,禁多数投票禁并集；EMITBURN-2 纯报告层，schema 判定零放宽。

### 13.7 F1/F2 批落地与 F3 设计前提修订（2026-07-25）

**F1 批（已推 `0380698ed`）**：R3a 补采标签形目标解析（TargetPID=0 原串形灭，锚腿全链 pin——第五次复放的窗判定失去推导混淆变量）；NG-1 EMITBURN-2（一条 reject 枚举全部 unknown key+JSON 路径，反射 json-tag 树走查 fail-open，单字段消息字节恒等）；NG-2 generic fail_closed 臂收窄（typed roster 在场时 unavailable 单源化，目标专属词与 legacy 形保守不变）；NG-3 ledger 窗臂（零锚降级运行读全记录一致 selected_window，非投票非并集）。

**F2 批（本提交）**：GAP-B1 目标窗内状态账行（typed target_window_states 观测→覆盖块正面陈述主导状态+五车道+「等待型自身状态是症状面」机制句；仅单主体账渲染，多主体禁猜；只搭既有块不扩创建门）；GAP-B3 vsync 周期权威注（census typed 观测在场→确定性注：发生器 period 打印才是周期权威，消费者回调间距不得当作周期/信号丢失证据）；NG-4（「结构化指标核对」诚实更名「摘录」+维度 token 化补 Han 连续段（脚本边界切分,ASCII token 零回归）+ F2 tripwire allowlist 对账 +1）。

**F3（GAP-B2）设计前提修订（实现侦察结论,批未开工）**：
1. **binder 耦合是两层不是一层**：§13.3 只点名了 `len(rejectedTxns)>0` 臂门；实现侦察发现 `findBinderWaitsForChain` 入口 `len(edges)==0` 整函数早退——**零 binder trace 结构性到不了 pacingVerdict**。解耦需两处:入口早退拆分（pacing 扫描独立于 binder edges）+臂门放开。
2. **D∧timer 闭集门缺数据管线**：`WakeupCausalAggregate` 无 caller/cause-symbol 字段——blocked_reason caller（timerfd_read）需从成员 CausalImpact 侧新增 typed 字段贯通到聚合检测器（engine wire 加字段=R2' 全同步面）。
3. 批性质确认：值通道（PacingIdle/periodic 折扣改 rank 有效归因）+新 wire 字段——**独立旗舰双复核批**，逐席 diff 追审，不与显示批同车。VS-1 的 `DominantState==StateSSleep` 门（detectPeriodicWakeupSource 入口）为 D 扩臂的第三改造点，扩臂形=「D ∧ 成员 caller∈timer 闭集(timerfd_read 首证) ∧ 节拍成立」子形折扣，D fail-close 红线与 SYM-2 既裁地位不动。

### 13.8 第五次复放定谳（no_touying_2.txt，2026-07-25；构建含 F1/F2 批——判别子=补采目标 `-32788` 追加形+VSync 周期权威注在场）

**质变（客户价值面）**：模型经 `recipe=jank` 首次取得真帧证据——FrameActual-536249（36.261ms）/536250（28.200ms）vs 16.67ms 目标，两帧的 compute_supply_balance typed 面（supply_ratio 59.5%/80.2%、core_limited 144.4/67.1 cpu·ms、低频损失 31.7/0 cpu·ms、CPU11 空置）进入正文——答案从「空树+背景分」变为「两个具名 jank 帧+典型供给证据+修复方向」。

**修复活体验证**：R3a（补采目标 typed 化）、GAP-B3（VSync 周期权威注，period_prints=0 诚实）、ARITHDUP（两条复算注零重复）、N1 roster（8 显示+`omitted_unique_boundaries=4`）、目标身份行、补采 rank+critical 双视图、census-lite——全部在客户产线生效。

**R3b 定谳升级（本放核心结论）**：R3a 混淆变量已除、per-PID 在位、补采 root_cause_rank+critical_blocking_calls 于用户窗成功披露（目标 `ss.hm.ugc.aweme-32788`）——но账本仍「没有产出有数据支撑的 root_cause/wakeup_chain/semantic 行」、全边界 `window_relation=unknown`（多窗形下 NG-3 ledger 臂按设计不采信，但 rank 行本身即锚）。矛盾链已纯净：成功 rank 必铸行（Rank=0 self 行亦铸观测）→ ClaimKey=root_cause_* 命中锚家族 → 窗必知——与观测相反。**⟹ 补采结果的 observations 未到达 ledger 编译面，或 rank 对该目标/窗真实零产**。热嫌疑（下一投资批 R3B-DEEP 的起点）：①「trace_query 结果已压缩」对补采 lane 观测的影响路径；②comm=unknown+同名 12 候选形下 rank 的 target resolution 分支；③supplement 结果在 CompileObservationLedger 的记录抽取臂。复现条件=构建 ≥cf42da727+该 trace+同请求，本地可控。

**新小项**：算术复算假配对 production witness #1——「超出 19.6ms）,…供给率仅 59.5%」同句内 ms↔% 跨从句误配对（帧超时 vs 供给率两个无关量），复算注成噪声形（R7 regex 残余的活体）；挂 NW-TS-RECON 同族形态债。多窗形 window_relation=unknown 属 NG-3 设计内（禁猜），可选改进=词面「多窗并存」替代裸 unknown，记档。

**队列**：R3B-DEEP（P1 调查批,本地复现三嫌疑）→ F3/GAP-B2（值通道批,§13.7 前提已冻结）→ 记档件。

## §13.9 R3B-DEEP 调查结论 + NEXTINFO P1 落地（2026-07-25）

### §13.9.1 R3B-DEEP：三嫌疑收敛与 C2 修复

**调查方法**：裸 trace 本地复现矩阵（三 label 变体 × 客户形目标 `ss.hm.ugc.aweme [32788]` × 同名候选 33410 × 窗内 wakeup_new 冲突 50173 × 600 观测账本压力）全部在 HEAD 通过——简单假说（label 解析/同名分辨/账本压载丢行）全被否证。永久 e2e pin 落地 `internal/tool/trace_query_supplement_r3b_test.go`：客户形补采 rank+critical 全链路（观测存活→账本 root/state 行 >0→投影窗 3.0..3.2→coverage 窗知+targetStates=1）。**该 pin 跑在裸 trace（无 tracebundle）上——用户指令「裸 trace 场景也要深挖系统 gap」的载体车道由此 pin 长期看护**。

**C2 实锤（本批修复）**：披露行对 Success=true 但零值观测的视图同样宣称「成功补跑」——第四放的「成功补跑 N 视图」与「账本零行」可以同真，披露面掩盖了唯一判别信号。修复：`SystemTraceSupplementMeta.ViewValueObservations` 逐视图值观测计数（诊断族 thread_selector_exact_name_mismatch / thread_incarnation_suppression 不计入），披露行零值观测视图追加「·值观测0条（本视图未产出可用观测，勿视为已补齐）」/en 同义，非零视图追加「·值观测N条」。**第六放自判别**：若披露读「值观测0条」→ rank 真零产（Lane B,窗内无确定性候选）；若读「值观测N条,N>0」而账本仍零行 → 观测在 CompileObservationLedger 抽取臂丢失（Lane A,直接立案修复）。

**残余嫌疑（等第六放判别,不预投资)**：①复合 tracebundle 索引车道（客户有 sibling bundle,裸 trace pin 覆盖不到该臂）；②视图内取消部分产出形。均已记档,证据到达前不动代码（禁猜）。

### §13.9.2 NEXTINFO P1：next_info 富字段结构批（客户语义文档 next_info.txt）

**三硬伤定谳**：
- **硬伤A（值语义颠倒,最重）**：f3=ices_boost（前台加速:更多时间片/更多算力）被引擎读作「restricted」（受限）——语义正好相反,且 `restricted=true` 子串驱动三处硬门（rank 席位 query.go:16532 / 置信加成 16370 / 决定性事件收窄 6764）。本批**保持 bug 兼容**（NextInfoRestricted 照旧填充,三门字节不动）,值通道纠正冻结为 **NEXTINFO-V1** 批（与 F3 同为值通道,需旗舰双复核:「受限」翻转为「加速」会改变 rank 叙事方向）。
- **硬伤B（0 值塌缩）**：load=0（功耗域提示）/expel=0（SMT_EXPELLEE）/cgid=0（SP_DEFAULT）均有闭集语义,旧渲染 `>0` 门把已知 0 与缺失混同。修复=Known 门控:malformed token→Known=false 不渲染,已知 0→渲染闭集词。
- **硬伤C（增量格式整体拒绝）**：客户文档明示 next_info 为增量格式（5/6/7+ 字段追加）,hitraceconv 文本车道 `len(parts) != wantParts` 在生产者首次升级时会**整条丢弃 next_info 车道**。修复=已验证前缀+纯数字尾字段逐字透传（`canonicalHarmonySchedInfoText`）,恶尾仍 fail-closed。

**落地面**：
- 解析:`parseHarmonyNextInfo` 重写（`nextInfoCheckedField` 逐字段独立解析,malformed fail-open 至 Known=false;boost=parts[3]!=0 兼容填充 restricted;extra 逐字保留;fieldCount 普查）。
- 词表:`internal/tracequery/next_info_lexicon.go` 单一词权威（sched_group 四词+unknown_group_N / smt_expel 五词+unknown_expel_N / SP_* 16 名+unknown_cgroup_N;未知值保数字禁猜）。
- 渲染:`renderNextInfoPolicy`（引擎面）与 `traceEventSchedulerDetail`（工具面）Known 门控追加 token（ices_boost=/sched_group=/smt_expel=/cgroup_name=/已知 load=0）;legacy token 字节不动（三子串门依赖）。
- 结构:P4 尺寸 ratchet 触发（728>688）→ 富字段入侧表 `*NextInfoRichFields`（值型访问器 `Event.NextInfoRich()` 防 nil 提升读 panic）,四有界字段 int→int32（load≤1024/group/expel≤7/cgid≤31）,核心 Event **688→680**（缩,ratchet 欢迎向）;JSON 面 golden 全量+View 双面扩位,精确 pin 688→680 带注更新。
- 测试:parse 三臂（客户 6 字段例 e,166,3,0,0,1→SP_BACKGROUND/增量尾/malformed fail-open——首跑即抓真 panic `parts[6:]` 越界）、词表闭集 pin、渲染 Known 门控正负臂、hitraceconv 增量尾九臂;旧 7 字段整体拒绝 pin 按客户文档判废改透传。

**队列更新**：R3B-DEEP 调查收官（C2 已修,C1 判别悬于第六放）;NEXTINFO P1 收官;**NEXTINFO-V1（硬伤A 值通道)与 F3/GAP-B2 并列为下两个值通道批**,均需旗舰双复核。

## §13.10 F3/GAP-B2 值通道批落地+旗舰双复核记录(2026-07-25)

**范围裁定**:用户令双值通道批开工;中途裁定 **NEXTINFO-V1(硬伤A ices_boost)记做遗留,不动代码**——三处 `restricted=true` 子串门(query.go:16391/16553/6764,6777)与树面显示词(runtime_tree.go:6976-6980,census 补充的第 4 消费点,冻结 spec 外)保持 bug 兼容现状;重开先读 §13.9.2。本节只收 F3/GAP-B2。

### §13.10.1 三改造点落地(§13.7 前提全兑现)

- **VS-1 D∧timer 扩臂**:`detectPeriodicWakeupSource` 的 `!=s_sleep` 单比较改双臂 switch——S 臂准入不变;d_sleep 臂要求**每个成员**的 typed caller ∈ `timerWaitCallerClosedSet`(新文件 timer_wait_callers.go,survey 闭集首证=timerfd_read,vsyncGeneratorThreadNames 先例,禁猜);任一成员异 caller/unknown/空→整聚合 fail-close(D 红线);io_wait dominant 结构性排除;共享节拍门(min-occurrence/robust period/early veto/in-band 2/3)双臂同绑。折扣算式不动:lateness=max(0,TargetBlockedMs−p),Effective=min(runnable+lateness, D 口径 blocking),cap 经 causalImpactBlockingMs 对 D 形自动正确;overlap reconcile 双函数核验 form-agnostic。
- **成员侧 typed caller wire**:`summarizeWakeupCausalImpact` 对 d_sleep dominant 经 `cache.findBlockedReason`(与 D 根证据面同一匹配器,同窗必同名)打 `WakeupCausalImpact.DFamilyBlockedCaller`(物理行 verbatim,无行=空,禁猜)。
- **pacing 解 binder 偶合**:入口 `len(edges)==0` 早退拆除(零 binder trace 可达 pacing 扫描);臂门 `len(rejectedTxns)>0` 改三路准入=legacy 写销路(io_wait/非 timer D 原样)∨ S 直接 ∨ d_sleep∧timer 凭证(node→impact typed 读取,新 helper chainCausalImpactForNode);PacingIdleSummary 新 `TimerWaitCaller` 字段+机制句「D-state timer wait (sched_blocked_reason caller=X) — periodic cadence, not eliminable I/O blocking」。

**设计防线五处如期咬红并按各自变更协议更新**:thread-state universe 双 golden(switch 面新条目+comparison 面迁移/计数)、tracediag R2' 第 7 处三 struct hash(逐 struct adjudication 注)、**因果 token 注册红线击发=`periodic_idle` 潜伏未注册 token**(P2-1 分叉产物,解偶合后首次可达 rank 面;按 §7.2.1 协议注册 wakeup_chain 车道+fix direction unresolved+golden,工具面 zh 词/专用 ContextOnly 臂 query.go:19727 原已双 token 在位)、SYM-2 闭集 lane 派生 pin(演化记录)、XLANE3 donghu 真迹值演化(**239.820−18.933=220.887 精确**,9163 板关注线程 18.933ms 节拍睡眠段典型化为 periodic_idle 上下文行,分母外移+「另有 1 条…未计入分母」诚实披露正臂新钉;E44→E45 纯序号位移演化记录)。

### §13.10.2 旗舰双复核(workflow wf_97739f80-b4a:4 镜头×高强度+逐 finding 2 否证席,12 agent)

**4 条 CONFIRMED(0 否证)全修**:
1. **(P1 wiring)投影面对 D 形谎称睡眠**:timer 凭证死在 tracequery→tool typed 边界——树面 影响形态 打「周期性信号源(期内睡眠为正常节拍…)」于 sleep=0 的 D 行旁。修=全链 typed wire:`timer_wait_caller` note key(注册表 hard_consumer+golden)→ RankItem 镜像双字段(两继承点)→ 导出器带 caller → Node.PeriodicTimerCaller 解码(PeriodicSource 臂内)→ caption 分叉「周期性信号源(期内定时等待 caller=X 为正常节拍…,非可消除I/O阻塞)」;口径图例与两处承诺面图例改双形措辞;info contract census 三登记。全链 zh/en 正臂 pin(note→decode→wording)。
2. **(P1 wiring)对比 cell 后缀「期内睡眠不计」对 D 形为假**:`runtimeTraceProjPeriodicCompareCellSuffix` 增 timer 分叉「(周期性,期内定时等待不计)」,双语 pin。
3. **(测试面,突变验证)D 臂共享门零负臂**:min-occurrence 移入 S 臂的突变全绿逃逸。修=D 臂双负臂(3 次 timer 不 detect/非周期间隔 fail-close)。
4. **(测试面,突变验证)wire e2e 禁猜契约未钉**:`findBlockedReason(q,thread,0,end,…)` 突变全绿逃逸。修=timerWireTrace 扩第二 D 窗(无 blocked_reason 行)断言空+首窗 caller verbatim 断言——窗纪律与陈腐猜测机械可判。

**复核期连带更新**:trace note key 注册 golden、emit-pin fixture(aggregate 面带 timer 凭证)、RankItem tracediag hash 重钉。全内 suite 绿。

### §13.10.3 残余与队列

- 记档不修:pacing 行树面 影响点 词面对 D 形仍读「周期空闲(等待下一周期信号)」通用词(机制句在引擎 Summary/证据面;行级词面已由图例双形措辞覆盖,专用分叉词面若立案挂显示批)。
- **NEXTINFO-V1=遗留(用户裁定 2026-07-25)**;F3/GAP-B2 收官。队列剩余=外部第六放(值观测计数判 C1)。

## §14 `main@3b875b05c` 全维冷读再审计（2026-07-25；只改文档，零生产代码修改）

> 用户指令：更新远程代码后从各维度重新审计；不得修改代码，有 GAP 才写入文档。
>
> 审计基线：`main@3b875b05c`（与 `origin/main` 一致）。审计前工作区干净；`9a57f773b..3b875b05c` 的生产改动只在 trace/answer/strict-decode 相关路径，未触及 read/write controller、REPL mode、worktree 或 writeflow。
>
> 验证：`go test ./internal/tracequery ./internal/tool ./internal/hitraceconv` 全绿；`go test ./internal/orchestrator ./internal/repl ./internal/worktree ./internal/writeflow/... ./internal/config` 全绿。**全绿不推翻下列 GAP**：命中的都是现有测试未覆盖的跨阶段组合形，或测试只钉中间产物、没有钉最终 canonical owner。

### §14.1 覆盖矩阵与总判

| 审计维度 | 结论 | 说明 |
|---|---|---|
| 远程基线/变更边界 | **covered** | HEAD 与远程一致；本轮未修改生产代码 |
| 补采值观测披露 | **gap / P1** | census-lite 组合形会丢最后一个窗口视图的计数；总计数口径也不能完成 §13.9 宣称的二分判别 |
| D-state 周期等待值通道 | **gap / P1** | VS-1 中间折扣正确，但 RSPA/`pacing_idle` 后续 canonical owner 未继承或未替换该值，最终可消除量可恢复成全额 D/IO |
| timer caller 禁猜/窗口纪律 | **covered** | `timerfd_read` 闭集、逐成员合取、无同窗 blocked_reason 不继承旧 caller 均有实现与测试 |
| NEXT_INFO 值语义 | **gap / P1（既有）** | `ices_boost` 继续喂给 `restricted` 硬门；§13.10 已记录为用户裁定遗留，本轮只复核不动 |
| NEXT_INFO Known/展示一致性 | **gap / P2** | malformed 字段虽不发闭集词，legacy 数字面仍会打印伪 `group=0` 等 |
| NEXT_INFO 转换/直读等价 | **gap / P2** | converter 与 query parser 的字段边界、范围和第六字段含义不一致，同一语义经不同输入车道可得不同结果 |
| 热路径内存/性能 | **gap / P2** | `*NextInfoRichFields` 给高频 `sched_switch next_info` 逐事件分配侧表；只钉 `unsafe.Sizeof(Event{})` 掩盖了引用堆成本 |
| 复合 tracebundle/取消部分产出 | **coverage gap / P2** | R3B e2e 明示只钉裸 trace；复合索引与 in-view partial cancellation 尚无等价 pin |
| 因果投影展示接线 | **partial** | surviving periodic chain seat 的 caller note→Node→中英词面已通；canonical owner 被替换时凭证仍会丢 |
| read/write 隔离与安全门 | **covered（本变更集）** | 本轮 trace 提交未触及相关生产路径，L1/L2/worktree/writeflow 测试组全绿；未发现由本变更集引入的新 RW gap |
| 临时目录/跨平台文件行为 | **no delta** | 本轮提交未新增转换临时目录或仓库级临时路径逻辑；不重开已结转换目录议题 |

**总判**：§13.9 的“C2 已修”和 §13.10 的“F3/GAP-B2 收官”均只能降为 **partial**。前者在客户常见的“窗口补采 + census-lite”组合形直接漏计；后者在折扣席进入最终榜单前存在两条凭证/数值丢失路径。历史章节保留当时事实，本节作为 evolution record 覆盖其“收官”状态。

### §14.2 AUD-01（P1）补采逐视图计数在 census-lite 组合形被错切

**代码证明链**：

1. 每个窗口视图完成时，`executed` 与 `valueObservations` 同步各追加一次（`internal/tool/trace_query_supplement.go:1321-1323`）。
2. census-lite 成功时只给 `executed` 追加 `event_search`，**没有**给 `valueObservations` 追加元素（`:1391-1397`）；这是合理的，因为 lite 不属于 `meta.Views` 的窗口视图。
3. 组装 meta 时先令 `ViewValueObservations=valueObservations`（`:1402-1405`），随后 census-lite 分支正确移除 `Views` 最后一项，却又错误执行 `valueObservations[:len(valueObservations)-1]`（`:1416-1424`）。因此：
   - 1 个窗口视图 + lite：唯一计数被切成空；
   - 2 个窗口视图 + lite：第二个窗口视图计数消失；
   - 计数长度恒比 `Views` 少 1。
4. 渲染器对 `i>=len(counts)` 静默返回空后缀（`answer_document_mutation_runtime.go:5558-5561`），不会 fail-loud。于是受影响视图重新显示成无计数的“确定性补跑”，正好复活 C2 要消灭的假成功词面。

**客户相关性**：第四/第五放都出现“窗口重跑 + 另对全 trace 补跑 census-lite”的披露形；这不是理论边角，而是下一次复放最可能走的生产路径。

**最优修向（冻结，不实施）**：

- `meta.Views` 去掉 lite；`ViewValueObservations` 保持原窗口视图计数原长，禁止二次切片。
- 增结构断言：`len(Views)==len(ViewValueObservations)`；内部不一致时展示 `count_status=unavailable`，不得静默省略。
- 新 e2e 必须覆盖 1-view+lite、2-view+lite、零值视图+lite、中英披露四臂。

### §14.3 AUD-02（P1）“值观测 N 条”不是根因家族计数，§13.9 的二分实验仍不可判

`traceSupplementValueObservationCount` 只排除
`thread_selector_exact_name_mismatch` 与 `thread_incarnation_suppression` 两个 predicate，其余 observation 全计入（`trace_query_supplement.go:337-351`）。因此 root_cause_rank 即使一个 `root_cause_*` / `wakeup_chain*` 行都没铸，只要还有 `target_window_states`、blocked-reason census、CPU constraint 或其它 WindowStats 观测，披露仍为 `N>0`。

这与 §13.9.1 的判别声明矛盾：

- 旧声明：`0` ⇒ rank 真零产；`N>0` 且投影零行 ⇒ ledger 抽取丢失。
- 实际：`N>0` 也可能只是状态/普查行，根因/链在 producer 侧本来就是 0；不能据此判定 ledger 丢失。

**最优修向（冻结，不实施）**：把单一总数改成 typed family census，至少分别携带 `root_cause_rows / wakeup_chain_rows / target_state_rows / critical_blocking_rows / diagnostic_rows`；若目标是定位 producer→ledger 丢失，还需同面比较 `emitted` 与 `ledger_accepted`，而不是用一个混合总数推断。所有 family 由 predicate/ClaimKey 精确闭集计数，禁止文本启发式。

### §14.4 AUD-03（P1）F3 折扣在 RSPA canonical D/IO owner 上丢失

**中间机器正确**：`detectPeriodicWakeupSource` 对 `d_sleep` 要求每个成员 caller 都在 timer 闭集，计算 `EffectivePeriodicImpactMs=runnable+lateness`；caller/节拍任一不成立均 fail-close。

**最终榜单逃逸链**：

1. `buildRootCauseRankFrom` 先构造 RSPA D/IO decision；当 census anchored Σ 与 chain D/IO Σ 身份相等时，`rspaSuppressChainDIOSeat` 返回 true（`query.go:14913-14926`）。
2. 随后 periodic 的 CausalImpact/aggregate rank carrier 被直接跳过（`:14989-15005`）。被跳过的正是持有 `PeriodicSource`、`PeriodicTimerCaller` 和折后 effective 的席。
3. canonical owner 变为 WindowStats 的 `d_state_or_io_wait` / `io_wait` 席。`transferChainDIOContext` 只转移 actual window、actual impact、target impact、depth/branch（`rank_chain_anchor_rspa.go:693-731`），**不转移** periodic 标记、caller、period、lateness 或折后 effective。
4. fully-anchored case 保留该 WindowStats 席的全额 D/IO 值（`:986-1024`）；后续 D/IO effective 口径按 `DStateMs+IOWaitMs` 计算。结果是中间已证明为正常周期 timer cadence 的 D 值，在最终可消除榜上恢复成全额。

这属于“值通道正确、canonical owner 错”的系统 gap，也解释了为什么只测试 detector/note/tree 仍可全绿。

**最优修向（冻结，不实施）**：

- 先冻结“一个物理 D/IO 账户最终由谁拥有”的 typed owner decision，再把周期凭证和值投影到该 owner；禁止在被抑制 chain 席上结束生命周期。
- 仅在 chain periodic 账户与 WindowStats canonical 账户有精确成员/Σ 身份时转移折扣；多分区或不等值时 fail-close，保留原 chain periodic 席并披露双账户，不能把一个 aggregate 折扣复制给多个分区。
- production test 必须走 `BuildRootCauseRank` 全链，并制造 `identityHolds=true` 的 RSPA 形，断言最终唯一 D/IO owner 仍携 caller 且 effective 为折后值。

### §14.5 AUD-04（P1）pacing D 路径只加 context 行，不替换原 D 候选；legacy binder 写销路还能绕过 timer 门

F3 的第二条路径也没有闭合到最终值：

- `findBinderWaitsForChain` 为合格 D timer node 铸 `PacingIdleSummary`（`query.go:14592-14617`），随后只把它降形为 `RootEvidence{Type: periodic_idle/pacing_idle, Summary:...}`（`:14415-14427`）；typed `TimerWaitCaller` 没进入 RootEvidence/RankItem。
- 该 context 行不修改原 `WakeupCausalImpact`。而 `isIntermediateSleepImpact` 只排除 `s_sleep`（`:18564-18573`），D impact 继续进入 rank；若自身 occurrence 数不足以触发 aggregate periodic detector，就会出现“periodic_idle context + 全额 D/IO 可消除候选”并存。
- `pacingEligible` 初值仍为 `len(rejectedTxns)>0`（`:14592`）。所以一个 caller 不在 timer 闭集的 D/IO node，只要恰有被写销的 binder candidate，也能绕过“D 只能凭 timer caller”这句测试/注释契约进入 pacing context。现有负臂用 `edges=nil`，没有覆盖该 bypass。

**最优修向（冻结，不实施）**：

- D 的 pacing admission 单源化为 `isTimerWaitCaller`；legacy rejected-binder 只能保留 S 形，不能授权 D/IO。
- pacing verdict 应回写/替换同一物理 impact 的 canonical rank value，或给 owner decision 一个 exact segment credential；只新增 context 行不算完成值修复。
- 增三类测试：foreign caller + rejected binder 必须 fail-close；单次 D timer + periodic waker 的最终榜不得恢复全额；pacing context 与 D owner 必须一账一值、无相互矛盾。

### §14.6 AUD-05（P1 已知 + P2 新确认）NEXT_INFO 仍有三层语义不一致

1. **NEXTINFO-V1（P1，既有遗留）**：字段 4 的 `ices_boost=1` 仍填 `NextInfoRestricted=true`（`parse.go:4893-4898`），并继续喂 `restricted=true` 决定性、置信和席位硬门。它把“前台加速/额外算力”叙述成“受限”，方向相反。本轮遵守既有用户裁定，只复核、不开修。
2. **Known 门只管新词，legacy 面仍伪零（P2）**：malformed group 会得到 `groupKnown=false, group=0`，新 `sched_group=` 确实不渲染；但 engine policy 仍无条件追加 `group=0 restricted=false`（`query.go:6834-6835`），tool event detail 也无条件打印 `load=0 group=0 ... expel=0`（`trace_query.go:6467-6469`）。所以“malformed 不再塌成假 0”只对新闭集词成立，旧数字面仍给模型/用户一个看似实测的 0。
3. **converter 与 direct parser 无共同 schema authority（P2）**：
   - query parser 对任意 1..7 位十进制都判 Known；`ices_boost=2` 也会变成 true 并进入硬门（`parse.go:4864-4898`）。
   - converter 按 `{load≤2046,group≤3,boost≤1,expel≤7[,cgid≤31]}` 整体拒绝（`hitraceconv/render.go:479-495`）。客户文档却写 load 最大 1024、group `>=4` 为未知扩展；direct lane 会保留 `unknown_group_4`，converter lane会整条丢掉。
   - `includeCGID=false` 时 converter 把第 6 字段当未知增量尾并透传（`:448-503`），但输出进入 query parser 后，第 6 字段又无条件被解释成 cgid（`parse.go:4900-4905`）。转换阶段的“未知尾”在查询阶段变成了确定 cgroup 语义。

**最优修向（冻结，不实施）**：建立一个共享的版本化 next_info prefix decoder，区分 `present/lexically_valid/semantic_known`；converter 负责无损保留，query hard gates 只消费闭集 semantic-known（尤其 boost 仅 0/1）；未知扩展保留原数字但不铸既有字段含义。先裁定 load 1024/2046 与 5-field+tail 的生产者版本，再改值门，禁止两边各抄一套 limits。

### §14.7 AUD-06（P2 性能）`Event` 变小不等于 next_info 热路径内存变小

`Event` 从 688B 降到 680B 的 ratchet 只量 `unsafe.Sizeof(Event{})`。实际实现把 `*NextInfoRichFields` 嵌入 Event（`types.go:193`），并在每个成功解析的 next_info 事件上取地址分配（`parse.go:4831-4840`）。

在 64 位布局下，`NextInfoRichFields` 本体约 40B（6 bool + padding + 24B slice header + 8B int），另有 allocator/GC 元数据；而 Harmony trace 的 next_info 位于高频 `sched_switch`，不是稀疏事件。`NextInfoExtra` 与 `NextInfoFieldCount` 还重复保存了原始 `NextInfo` 已能恢复的信息。故“core -8B”可能换来“每个相关事件 +1 堆对象、约 40B+”，大 trace 的总驻留和 GC 压力反而上升。

现有性能测试只测 core size，`BenchmarkParseFileThroughput` 的 sched_switch fixture又不带 next_info；没有 `allocs/op`/B/op ratchet 能抓此回归。

**最优修向（冻结，不实施）**：把六个 Known/boost 位压成 inline bitmask（四个 int32 已腾出足够空间的可能性应先用 layout 实测），field count/extra 从原始字符串按需读；若必须侧表，则由 Index 使用稠密 ordinal/arena 型存储，避免逐事件小对象。增加带 next_info 的 50k/200k 解析 benchmark 和确定性 allocation 上限，不能只看 `unsafe.Sizeof`。

### §14.8 AUD-07（P2 测试/载体覆盖）现有 e2e 没有覆盖两条已声明残余

`trace_query_supplement_r3b_test.go:14-24` 明确声明旗舰 e2e 是**裸 trace**，复合 tracebundle index lane 与 in-view cancellation partial 仍在调查；其断言也只检查总 observation、ledger root/state 与 projection window（`:75-107`），没有检查 `SystemTraceSupplementMeta.ViewValueObservations` 的长度/逐族值。零值披露测试则是手工构造 meta（`trace_query_supplement_test.go:1249-1267`），无法咬住 AUD-01 的生产组装错误。

F3 测试同样主要钉 detector、`findBinderWaitsForChain` 返回值和 caller wire；没有一条同时穿过 `BuildRootCauseRank + WindowStats/RSPA + typed observation + projection` 的 D timer production pin。因此 AUD-03/AUD-04 不与“全绿”矛盾。

**最优测试批（冻结，不实施）**：

- R3B：bare/bundle × normal/partial-cancel × census-lite on/off 的最小矩阵；逐视图 family census 与 ledger accepted census 对账。
- F3：detector 正确只是前置条件，验收必须落在最终唯一 rank owner、最终 effective、最终 caller note 和投影词面四者同源。
- NEXT_INFO：direct systrace 与 hitraceconv 产物做同输入语义 differential test，覆盖 group=4、boost=2、5-field+tail、第 6 字段 provenance、malformed sibling。

### §14.9 非 GAP 与不变量复核

- `timerWaitCallerClosedSet={"timerfd_read"}`、offset 去除、unknown/foreign caller fail-close、逐成员合取、min-occurrence/节拍负臂均正确；问题发生在其后 owner 归并，不应回退这些门。
- surviving periodic chain seat 的 `timer_wait_caller` note 注册、Node 解码、中英 caption 与 comparison suffix 均已接通；修 canonical owner 时应复用同一 wire，不得另造 Summary 解析。
- C2 renderer 在 `Views/counts` 等长时，0 与正数词面正确；修生产组装，不应删掉“勿视为已补齐”句。
- read/write L1/L2、kill switch、风险/审批、worktree cleanup 等本次变更集未改，相关包测试全绿；本轮没有证据重开 RW 系统 gap。
- 不得为修 F3 放宽 D 全族：只有 exact timer caller + 周期证据 + canonical account 身份同时成立才折扣；身份不等、多分区、caller 缺失一律 fail-close。

### §14.10 后续批次排序（仅方案，不授权代码修改）

1. **S14-A / P1 值通道**：AUD-03 + AUD-04，先解决“周期凭证必须活到最终 canonical owner”；以最终可消除量为验收，不以中间 struct 为验收。
2. **S14-B / P1 补采权威**：AUD-01 + AUD-02，修 count 对齐并改成逐 family emitted/accepted census；直接服务下一次客户复放判别。
3. **NEXTINFO-V1 / P1 冻结**：继续遵守既有用户裁定；只有重新授权才修 boost→restricted 值方向。
4. **S14-C / P2 NEXT_INFO 完整性**：Known/legacy 同权威 + converter/direct schema parity + differential tests。
5. **S14-D / P2 性能与载体**：去逐事件 side-table 小对象，补 next_info allocation ratchet；补 bundle/partial-cancel e2e。

本轮到此只完成审计与文档落地，**未修改任何生产代码或测试代码**。

## §14.11 S14-A/S14-B 双批落地+双向核验记录(2026-07-25)

**核验判词(对同事 §14 审计的双向核验)**:AUD-01/02/03/04 四条 P1 全部独立亲证为真;AUD-01 我方初判「必 panic」系读串臂嵌套(lite 为**有窗趟**臂,executed≥1),同事的「错切最后窗口视图计数」诊断逐字正确——production 红测(有窗 critical 视图+vsync 家族武装 lite)先红后绿定谳。AUD-03 逃逸链三段(抑制臂无 periodic 检查/transfer 缺折扣字段/identityHolds 可成立)逐一核实。

**S14-B 批(已推 `700ce906c`)**:AUD-01=lite trim 只裁 Views(valueObservations 从未收 lite 条目);渲染器失配 fail-loud「值观测计数不可用(内部不一致)」;AUD-02=`TraceSupplementViewFamilyCensus` 逐视图 typed 家族普查(root_cause_ 前缀/wakeup_* 链五谓词/target_window_states/critical_blocking/诊断双谓词/其他,非诊断和==配对总数并 pin),披露面「值观测N条(根因a·链b·状态c·其他d)」双语;旗舰裸 trace e2e 增配对+根因家族非零断言;四 exact pin 词面演化+零值句字节恒等负臂。§13.9 判别升级=第六放直读根因家族计数(混合总数不再承担判别)。

**S14-A 批(本提交,值通道,旗舰双复核 wf_8fe3fe39:3 镜头+逐 finding 双否证,25 agent)**:
- AUD-03=`transferChainDIOContext` 单 window owner(windowDIOSeatCount==1)在 Σ 恒等门(rspaWithinTol(链账 blocking, owner D+IO))下继承周期折扣全字段(census 聚合优先/strongest impact 兜底);不等值/多分区 fail-close 双负臂 pin。
- AUD-04(b)=pacing D 准入单源化 timer 凭证(两 case switch+fall-through ledger);io_wait/非 timer D 彻底失去 binder 写销旁路。
- AUD-04(a)=`applyPacingTimerDiscounts`(attachIPC 内):D∧timer pacing verdict(仅 waker 侧 typed VS-1 周期源,两-tick cadence 无折扣权)回写同一物理 impact 的 rank 值;全成员盖章聚合重导折扣。

**双复核 10 CONFIRMED(去重 6 缺陷)全修**:①聚合迟到量 naive Σ 重复计数 overlap 分支投影(否证席运行时复现:同窗双分支 20ms 段 naive 23.396→clamp 吞折扣)→改乘 `reconciledWakeupAggregatePeriodicLateness` 同一 overlap-cohort 权威;②聚合 reconcile 误迭代 top-8 显示裁剪视图而 rank 席位读全量 census→改迭代 `rankAggregateCensus`(前 8 项 aliasing 自动同步);③混周期组发布任意末成员 period→单 period fail-close(成员自章保留);④AUD-03 transfer 改写 EffectiveImpactMs 未重导 Score(§7.30 S1,q4 rank1-score 失谐同类)→兄弟重写器同式重导+pin;⑤pacing top-8 显示 cap 决定值通道→cap 移至折扣 pass 之后;⑥两测试判决力洞(io_wait 负臂 edges=nil 从未武装写销路——突变验证绿逃逸→donghu P9 fixture D 态手术真 pin;迟到量/cap 臂全零样本→越期成员+overlap 复现+census 越裁剪+混周期四新臂)。另 SYM-2 目标自因盖章嫌疑被否证席 2-0 驳回,仍加 ChainDepth>0 纵深防御(聚合成员谓词镜像)。

**残余记档(不实施)**:caseAOwned/多分区+periodic 形(锚定账由 §29.50.5 分区席持有)折扣仍丢——fail-close 需分区席机制级"保留链席+双账户披露"设计,挂 S14-A2;pacing >8 段仅值面全量、显示面仍 cap(纯容量,无值语义)。S14-C(NEXT_INFO 同权威/差分)与 S14-D(侧表小对象+bundle e2e)待批。

## §14.12 S14-C NEXT_INFO 完整性批落地(2026-07-25)

**AUD-05(2) legacy 伪零面**:两处 legacy 数字面(引擎 renderNextInfoPolicy 的 group=%d/restricted=%t 无条件 token、工具 traceEventSchedulerDetail 的 load/group/restricted/expel 四连)全部 Known 门控——malformed 字段不再打伪实测 group=0/restricted=false;良构载荷全字节恒等(全 Known=true);restricted token 在「携带主张时」保留(V1 冻结的 bug 兼容 true 填充在语义 boost 撤回时 token 不消失,三处 restricted=true 硬门零影响)。

**AUD-05(3) converter/parser 同权威**:①boost 语义 Known 拆分——文档闭集 {0,1};boost=2 保留 legacy restricted=true 填充(与 pre-P1 parser 字节等价,V1 冻结)但撤回 ices_boost 语义主张(新面不再对未记档值宣称前台加速);②converter 文本车道语义域放开——文本车道职责=无损保留,语义门在 query 侧;打包位域上限(group≤3/cgid≤31 等)曾在 group=4(文档明示 ≥4=未知扩展)时整条丢 token 造成双车道分叉;现文本车道只做词法校验(纯十进制+≤7 位,与 parser 字段 cap 同宽),二进制车道位域宽度不动;③第 6 字段 provenance 双车道现等价(converter 尾透传→parser 按文档位置语义读 cgid)。**差分测试**(next_info_differential_test.go):group=4/boost=2/cgid=32 三形 converter 无损+direct 解析同语义逐臂 pin,malformed 词法仍整条 fail-close;旧 5-bit cgid 拒绝 pin 判废改透传(演化记录)。

**残余待批**:S14-D(侧表逐事件小对象→inline 位压缩+alloc ratchet+bundle/partial-cancel e2e)与 S14-A2(caseAOwned/多分区+periodic 双账户披露)。load 1024/2046 生产者版本上限裁定仍开放(文本车道现词法透传,查询侧未加值域门——先裁定再动值门,§14.6 冻结原则)。

## §14.13 S14-A2 多分区+periodic fail-close 复活批(2026-07-25)

**§14.11 残余关闭**:纯 timer 线程(周期折扣成立)的锚定账被多锚定窗切成 ≥2 分区席时,链侧 periodic 席被 mint 期抑制、折扣与凭证死亡、分区席全额上榜——同一客户机制的最后一条全额逃逸路。落地=§14.4 冻结方向的 fail-close 形:`reanchorOnChainStateSeats` 尾部复活 pass——对每个 identityHolds DIO decision 的 pid,若最终无任何 D/IO 席携 PeriodicSource(单 owner 转移未着陆),从 census 聚合(优先)或单 periodic impact 重铸链席(`rspaResurrectPeriodicChainSeat`),携折后 effective+timer 凭证+Score 同步重导+**typed 双账户披露**(ChainAnchorOwnershipDivergent+双Σ+「dual account…never add the two」句,A' 词汇族)。禁复制不破:分区席永不收折扣拷贝。

**边界语义**:①Σ 不等值形不复活——identity 破裂时 mint 抑制根本没发生,链席自活携折扣,复活=双铸;②双 reanchor pass 幂等——复活席自身携 PeriodicSource,第二遍 carried 扫描视为已达;③复活门=PeriodicTimerWait(S14-A2 范围止于 GAP-B2 子形,纯 S 周期账不在 D/IO dominant 域内本就不进本 pass)。三臂 pin:多席复活正臂(凭证/折后值/双账户句/幂等)、Σ 不等值不复活负臂、Case-B 单 owner 转移回归不变。

## §14.14 S14-D 后半:bundle/partial-cancel 载体 e2e(2026-07-25)

**结构性定谳(调查副产物)**:补采车道**构造上永远跑裸 trace**——attached blob 落盘名为 `attached_trace.txt`,而 sibling bundle 提升(`traceArtifactBase`)只认 `.systrace`/`.perftrace` 后缀,故 blob 车道结构性不可提升;客户产线的真实混合形=模型主调用 source=path 走 bundle composite 观测+补采走裸 blob 观测,二者汇入同一 ledger 编译。R3B「复合 tracebundle 索引嫌疑」的实质即该**混合编译臂**。

**新 e2e 两条**:①`TestR3BSupplementBundleLaneMixedCompile`——客户对形(capture.systrace+V2 sibling manifest(schema/capture_id/sha256 经 tracebundle.CaptureID 真算)+attached blob),先断言引擎绑定 bundle(fixture 前提),模型 bundle 车道调用+裸补采→混合编译保全链(ledger 因果/状态行、投影、逐视图计数/家族配对、根因家族非零)——**该臂被证健康**,第六放判别若指向 Lane A,嫌疑收窄到取消部分产出或客户特有形;②`TestR3BSupplementPartialCancellationKeepsCountFace`——带 typed `TraceViewCancellation` 的 Success 部分产出结果,其真实观测必达 ledger(部分=更小的账,非中毒的账)。零执行取消披露与预算取消车道原有 pin 不动。

**剩余**:S14-D 前半(perf 位压缩+alloc ratchet)独立批。

## §14.15 S14-D 前半:next_info 热路径去堆对象(AUD-06,2026-07-25)

**布局定谳**:P1 侧表(*NextInfoRichFields,每个 next_info sched_switch 一次 ~40B 堆分配)整体退役——六个 Known/boost bool **内联**进 NextInfoRestricted 后的 7 字节 padding 洞(零核心增长),Extra/FieldCount 停止存储(全仓零消费者;raw NextInfo 串本身即无损载体,需要时按需派生)。核心 Event **680→672**(侧表指针 −8B)。`NextInfoRichFields` 转为值 VIEW 类型+`NextInfoRich()` 由内联字段构造——全部消费面(两渲染器/detail 面/测试)零改动。JSON 面:七个 bool 键名不变、位置移入 next_info 簇;next_info_extra/field_count 两键退役(golden 机器重钉,leaf 157→155;精确 size pin 680→672 带注)。

**判决性 ratchet**:`TestNextInfoParseAllocRatchet` 差分实测(cg 移入基线):next_info 解析差分=+5 全为**瞬态**小对象(Split parts/掩码 lower-trim 拷贝/allowed-CPUs 切片=P1 前既有驻留字段),预算钉 5——复加任何驻留 per-event 对象(旧侧表形)即 6→红。`BenchmarkParseLineNextInfo` 补上审计点名缺失的热路径基准(-benchmem 可观测)。S14-D 全收官;同事 §14 七条 AUD 至此全部处置完毕(修复/记档/冻结各归其位)。

## §14.16 NEXTINFO-V1 值通道批落地+旗舰双复核+聚焦否证二轮(2026-07-26)

**解冻**:用户裁定「加 nextinfo v1 解冻做」,撤销 §13.10 的遗留冻结。§13.9.2 硬伤A 全量落地:f3=ices_boost(前台加速)的「restricted」误读端到端退役。

### §14.16.1 批内容(17 文件净批)

- **字段退役**:`Event.NextInfoRestricted` + parse bug 兼容填充(`boostLexical && raw != 0`)整体删除;JSON 键 `next_info_restricted` 消失(leaf census 155→154,Event size pin 不变 672)。
- **词面退役**:`renderNextInfoPolicy` 与 tool detail 面的 `restricted=%t` token 删除——`ices_boost=%t`(Known 门控)是 boost 车道唯一主张;超文档 raw>1 两面均零主张(旧填充对该形恰好印 restricted=true)。
- **硬门收窄**:`cpuConstraintRestrictsExecution` 与置信加成(0.64→0.72)的 `restricted=true` 子串臂删除——加速旗标永不铸限制主张。
- **准入/决定性改读语义**:`isCPUConstraintEvidence` 与 sched_switch census 臂经新 helper `nextInfoBoostActive`(`BoostKnown && Boost`)准入;决定性 token 换轨 `ices_boost=true`。真机 trace 上准入域不变(parse 要求 affinity 非空,boost 臂本就冗余);超文档值不再准入是纠正而非回归。
- **显示面**:树面「策略 restricted=true」→「策略 ices_boost=true(前台加速)」双语;trace_query Description/Parameters 两处 `affinity/restricted`→`affinity/ices_boost`(byte golden 按 UPDATE RITUAL 重钉);hitraceconv 裸数字车道仅局部命名(输出字节不变)。
- **先红后绿**:7 pin 先全红(含否证复活臂:字面 `restricted=true` 也不得再铸限制)。

### §14.16.2 旗舰双复核(wf_f458387b-093,4 镜头×逐 finding 双否证席,34 agent)

14 confirmed / 1 refuted,全处置:

- **P2 真金(semantic 镜头,双否证席全链实证)**:census 把 sched_switch `cg=` 后缀(cgroup 名)盖进 `CPUSet`(query.go 6777),而两硬门把「CPUSet 非空」当真限制凭证——带 cgroup 的真机 trace(常态)上「加速叙成受限」类经邻臂存活,本批承诺被掏空;且属噪声信号驱动硬门形。**处置=typed provenance**:`CPUConstraintSummary.CPUSetIsBinding`(仅真 binding 事件 `cf.CPUSetName` 置位;cg= 代理填充恒 false;binding 事件可覆盖代理值并升级凭证),两硬门+置信臂只认 `CPUSetIsBinding && CPUSet≠""`;掩码排除臂原样。
- **缺 pin 6 处全补**:置信臂抽为 `cpuConstraintRankConfidence` 单元 pin;detail 面复活 pin;决定性 boost 首现 e2e;`nextInfoBoostActive` Known 门臂;Parameters 词面 pin(byte golden 只钉 Description);provenance 升级 e2e。
- **注释漂移 5 处**修正(types.go 侧表注/CPUConstraintPolicy 契约注/trace_note_keys 注/p1 测试头注/census Ref「restricted 词面门」×2)。
- **fixture 产线形 3 处**换轨 `ices_boost=true`(typed_observations×2 + note_keys_emit)。

### §14.16.3 聚焦否证二轮(wf_62bcf885-631,3 镜头,provenance 修复本身)

三镜头收敛抓两真缺陷(live probe 实证)+两小件,全处置:

- **D1 同名升级不决定性**:binding 事件 cpuset 名==早先 cg= 代理名(平台常态,cpuset 控制组与 cgroup 同名)时 prevSet 不翻转,证据窗(Line*/Ts)只盖代理行——判决唯一见证被排除在其自证的 span 外(AFF-EVID span 契约违例)。修=`prevBinding` 入决定性比较;同名 e2e pin。
- **D2 判定依据面误归**:Kind first-wins 使升级席仍标 `sched_switch_next_info`,把 binding-only 判决归给代理车道。修=真约束事件对代理标签升级 Kind(真事件间 first-wins 不变);常量 `cpuConstraintKindSchedSwitchNextInfo` 单源;两 e2e 断言 Kind 升级。
- **D5 rank 席 payload 缺凭证位**:AFF-EVID 五元组不携 provenance,binding 席与被判死的 proxy-only 形在叙事面不可区分。修=`CPUConstraintCPUSetIsBinding` 全链(RankItem 字段+note key `cpu_constraint_cpuset_is_binding` hard_consumer+投影 Node 消费+**树面分词:绑定=「cpuset组 X」,代理=「cgroup组 X」**——proxy 填充冒充 cpuset 组本身即残余误称);R2' 全链重钉(registry golden 468 行/census 双表/emit fixture/tracediag schema hash/rnb2 donghu 见证席词面改 cgroup组=诚实形)。
- **D3/D4 小件**:decisive 名册注释除名 retired restricted、列入 binding 凭证翻转;census Ref 注两处换 V1 词面。
- **无假阴性(lens 1 全普查)**:binding cpuset 名唯一产地=parse.go populateCPUConstraintFields(四种 EventCPUConstraint),恰为置位车道;掩码凭证经原臂全权;生产零 JSON 反序列化消费者。

### §14.16.4 残余与队列

- **顺带修正**:置信 0.72 臂对「掩码受限但仅 cgroup 代理」席回落 0.64——置信不再被非凭证抬升(值方向=修复意图)。
- **残余(记录不修)**:binding 事件本身的「绑到宽松组是否算限制」未细分(top-app 绑定也过门)——真 binding 事件是显式调度动作,席位成立;组宽松度判别需组→CPU 映射,trace 通常不载。掩码/核档臂已承担实际限制证明。
- NEXTINFO-V1 至此**全收官**;NEXT_INFO 值语义 gap(§13 表「gap/P1 既有」行)清零。

## §14.17 NEXTINFO-FWD 内核版本前向兼容补强（2026-07-26）

**用户语义裁定**:`next_info` 是鸿蒙内核输出的权威、关键调度事实；不同内核版本依次存在 5/6/7 字段，后续可继续追加第 8 个及更多字段。协议规则为「最小五字段、只从尾部顺序追加、既有字段位置与含义不变」。

**审计命中**:`harmonySchedInfo` 的文本车道曾用独立 `cg=/cgroup` 格式字段是否存在来决定 `includeCGID`，进而把“无独立 cg 字段”误等价为“next_info 至少六字段”。这会把合法的五字段内核版本整条拒绝；差分测试又手工按逗号数选择 `includeCGID`，绕开了生产路由，未能抓住该矛盾。

**兼容规则落地**:

- 文本 `next_info` 只按载荷自身判断版本：`len(fields) >= 5` 即进入前缀校验，独立 `cg=` 不再参与长度推断；
- 前五字段按既有词法规则校验并规范化，字段 6..N 只要求非空十进制词法并逐字保留；转换层不猜未知尾字段语义；
- packed 二进制车道仍按其已知位域和是否需要内嵌 cgid 的既有规则展开，不与文本版本判定混用；
- 回归矩阵覆盖 5/6/7/8 字段、短载荷、空尾、非数字尾和带空格恶尾；真实 `harmonySchedInfo` 无独立 cg 的五字段入口必须保留。

**剩余边界**:查询 parser 已按 `len>=5` 接纳并保留 raw `NextInfo`；当前只解释已知前六字段，字段 7+ 作为 lossless 尾部等待版本化 typed consumer，禁止因未知尾部撤回前缀权威事实。

## §15 main@9153d2cd0 最近提交复审：确认 GAP 与施工冻结（2026-07-26）

### §15.1 审计语义前提

用户再次裁定：`next_info` 直接取自鸿蒙内核；存在内容时属于高权威、关键调度事实。其中首字段 affinity 是当次调度点真实 cpuset mask；后续 `load/sched_group/ices_boost/smt_expel/cgid/...` 按内核版本顺序追加。独立 `cg=` 是另一载体字段，不能反向降低 `next_info` 字段的权威等级。

硬门仍遵守架构红线：权威元数据可以作为精确信号，但“元数据存在”不自动等于“它造成了本次卡顿”。根因席必须进一步证明可归属的 runnable/blocking 区间和执行能力影响；无法证明时进入 typed context/反证面，不硬铸根因。

### §15.2 确认 GAP 清单

| ID | 优先级 | 确认缺陷 | 最终消费者影响 |
|---|---:|---|---|
| NIFWD-01 | P1 | 文本转换曾按独立 `cg=` 是否存在猜 next_info 至少 5/6 字段，合法五字段、无独立 cg 形会整条丢失 | 权威 next_info 全车道消失；§14.17 / 批 A 处置 |
| NIAUTH-01 | P1 | `computeCPUConstraintSummaries` 对同线程历次 allowed mask 做并集，动态 cpuset epoch 被抹平；限制门再与窗口内 `stats.CPU` 而非 trace-global typed CPU universe 比较 | 两个时段分别受限可被并成“全窗不受限”；未在窗内发事件的 CPU 被当不存在，根因假阴性 |
| NIAUTH-02 | P1 | `CPUSetIsBinding` 同时承担 `cg=` 组名 provenance 与 allowed-mask 权威；next_info mask 排除证明仍被降到 0.64，显式组名反而可无 mask 过门 | 证据等级读错载体；同一事实在 engine Summary 称 cpuset、树面称 cgroup |
| NIAUTH-03 | P1 | `runnableContextVerdict` 未复用统一 restriction proof；任意非空 AllowedCPUs + “不含 big”（拓扑未知空集合也命中）+ other idle 即输出 `restricted_to_busy_or_small_cores` 0.84 | 主根因面拒绝限制结论时，次级面仍可发布高置信限制断言 |
| NIAUTH-04 | P2 | CPU constraint 在进入根因前先按 `RunnableWaitMs + ConstraintCount×0.25 + MigrationCount×0.5` 裁到 8；高频 sched_switch 次数可挤掉长等待受限线程 | 展示 cap 提前成为根因 census cap，噪声计数影响覆盖 |
| NIRICH-01 | P2 | load/group/boost/expel/cgid 虽已 Known-gated 解析，却主要压进不断覆盖的 Policy 字符串；没有版本化 typed epoch 与对应 runnable/落核/供给效果关联 | 关键内核事实“采到但只展示”；变化过程丢失，不能构造可复算因果关系 |
| DATA-D1 | P1 | decisions/contributions completion arm 固定生成 `role=audit` contribution，而终态 `ValidateResult` 明确拒绝 audit/diagnostic-only required ledger | 新 deterministic plan 实际执行必报 `contribution_ledger_role_starved`；现测试只验证“有计划” |
| RSPA-MP1 | P1 | 多分区 periodic fail-close 保留多个 raw full-value partition 席，再复活一个 discounted chain 席；复活席只置 Divergent，不置 Full/Remainder，结构化 relation renderer 不发布“不可相加” | raw 席可压过折后席；最终榜与因果投影缺单一值所有者，主要依赖 Summary 软句 |
| EVAL-W1-L | P3 | legacy `FindActiveRun` 单结果 store 若先返回跨仓 run，无法继续枚举更老的同仓 active run便直接 fresh-seed | 生产 identity-aware store 健康；仅兼容接口存在同仓双活动风险，记档后置 |

### §15.3 非 GAP / 已验证健康

- supplement count/family 配对、lite adjunct trim 与混合 bundle/裸补采载体 e2e 健康；
- next_info hot path inline 化与 allocation ratchet 健康；
- `ices_boost` 不再被解释为 restriction，Known 闭集与超文档值撤权方向正确；
- write workflow 生产 identity-aware 跨仓隔离、answer separator、strict-decode soft guidance 健康；
- `LEDGER-TRIPWIRE` 只能证明 completion switch 枚举了 case，不能证明 case 的产物满足终态语义；DATA-D1 不因此关闭。

### §15.4 分批施工与验收冻结

1. **批 A / NIFWD-01**：文本 next_info 最小 5 字段、尾部无限顺序追加；5/6/7/8+、有/无独立 cg、恶尾负臂；独立提交推送。
2. **批 B / NIAUTH-01~03**：拆分 `AllowedCPUsAuthority` 与 group-name provenance；trace-global CPU universe；主/次判定共用一个 typed restriction proof；不得把 boost 当限制；独立提交推送。
3. **批 C / DATA-D1**：completion plan 必须产出至少一条 reconcile-participating target contribution；若已有答案但缺合法数值来源则 fail-closed 到真正的 qualify/compute continuation，禁止 audit 行冒充；补“生成计划→ActionRunner→终态 ValidateResult”e2e；独立提交推送。
4. **批 D / RSPA-MP1**：多分区 raw rows 进入 lossless detail/absorbed account，discounted periodic account 成为唯一 rank-value owner；或若必须双账并列，则 typed 非加法 relation 必须在投影可达且 raw rows 不参与竞争排序；补 BuildRootCauseRank→reconcile→sort→projection e2e；独立提交推送。
5. **批 E / NIAUTH-04 + NIRICH-01**：root 计算读取 full CPU-constraint census，cap 只裁展示；建立 next_info epoch 载体和版本化 tail，按对应 runnable interval 计算影响；load/group/boost/expel/cgid 作为 typed context/反证，只有与实际区间效果闭合才升级根因；独立提交推送。
6. **后置 / EVAL-W1-L**：生产路径不受影响；待 legacy store 仍有外部实现证据时再扩展接口或强制枚举，不与 trace 值通道批混改。

### §15.5 全批不变量

- next_info 已知前缀字段不得因未知尾字段、缺独立 cg 或版本升级被整体撤回；
- affinity mask 是当次调度点 cpuset 快照，禁止跨 epoch 直接集合并集充当同时允许集合；
- 根因限制结论必须同时携带 source authority、CPU universe/exclusion proof、可归属 runnable 量；metadata-only 只作 context；
- value owner、sort key、投影值和关系披露必须来自同一 typed owner decision；
- 每批先红后绿并运行相关包测试；每批单独 commit、push `main`，不积压跨批修改。
