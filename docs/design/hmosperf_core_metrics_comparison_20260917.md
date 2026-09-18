# HarmonyOS PerfMcpServer 核心指标逐项对照（2026-09-17）

## 1. 范围、基线与判定口径

参考目录：`/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main`（本地目录没有 Git 元数据，不能给它虚构提交号）。Codrax 审计基线：`93bf1a42d4d184e0043d2b2cacaead5f7e70192c`。本分册覆盖 `config/indicators/` 下 CPU、GPU、sched、IO、marker、process、startup、rendering、game、sleep、uninterruptible、groups，以及根级 `l1_budget.yaml`：**50 个 YAML 文件、63 个 named metric**。一个 YAML 可以声明多个指标，计数不按文件数替代指标数。memory/network/video/sampling 的原子指标与扩展导入器由扩展域分册详审；这里仍列出 `memory_comprehensive` 组装入口，以免漏掉用户可调用指标。

已阅读参考仓根 `AGENTS.md`、`docs/GOLDEN_RULES.md`、`docs/architecture.md`，以及 config、config/indicators、core、core/preprocess、tests 的子级说明。随后逐项阅读 YAML 的 SQL/依赖/预处理调用，并跟到相应 Python 实现与相关测试；Codrax 侧跟到真实查询、解析、转换、DTO 和回归测试，而非按同名函数判断覆盖。

本文是**能力审计与实施清单，不是 63 项已经补齐的声称**。状态含义：

- **覆盖**：核心问题已有等价能力，不要求 API 或统计百分位与参考仓字节相同。
- **部分**：已有数据、基础统计或通用查询，参考仓的特定聚合/业务语义尚未形成稳定查询面。
- **缺语义面**：原始信息可能已保真保留，但没有对应的类型化分析/聚合能力；不等于转换丢数据。
- **不移植**：参考实现有错误、欠精确推断或不适合直接作为 Codrax 权威合同的部分。

特别区分四层：**原始保真 → 事件语义解析 → 窗口统计 → 链上因果证明**。Codrax 的 `internal/hitraceconv/streamerdb_text_fidelity.go` 已将除 `sqlite_%` 内部表外的所有 SQLite 表逐 cell 保留到文本载体，保存整数、REAL 位模式、TEXT/BLOB 原字节和 schema；该载体明确不授予 CPU、span 或因果权威。因此下面 GPU、native 域或平台专有表的“缺”不能误写成物理导出丢失。新指标也不能由“同窗出现/邻近事件/计数很高”直接晋升根因。

ROI 取决于覆盖问题广度、现有数据可复用度、实现/验证成本和误归因风险；P1 高优先于 P2 中，不因参考仓有功能就全部照搬。

## 2. Codrax 对照索引

表中的 C 编号指向如下真实实现，以免每一行重复几十个文件名。

| 索引 | 已有实现与边界 |
|---|---|
| C0 原始保真/入口 | `internal/hitraceconv/streamerdb_text_fidelity.go` 全表逐 cell 保真；`streamerdb_export_measure.go` CPU measure 语义导出；`internal/tracequery/view_capacity.go` 闭集视图与容量。保真载体不是可直接使用的执行/因果事件。 |
| C1 CPU 供需 | `internal/tracequery/query.go` 的 `computeCPUFrequencyResidency`、`computeCPUConstraintSummaries`、`computeSupplySummaries` 等；`cpu_occupancy.go` 的 CPU/process census 与供给平衡；`cpu_constraint_epochs.go`、`cpu_frequency_limit_summary.go`、`cluster_ceilings.go`、`full_freq_curves.go`、`cluster_freq_share.go`。窗口前频率接续、同时间戳裁定、频率覆盖、观测上限/限制来源均有边界。 |
| C2 调度/状态 | `query.go` 的 off-CPU、scheduler latency、runnable context、pressure、wakeup chain；`target_window_state_account.go`、`target_window_state_boundary_fold_test.go`、`state_account_identity.go`；`types.go` 的 `SchedulerLatencyResult`、`CPUPressureSummary`。已有 P50/P95/P99、CPU 去向、亲和性/cgroup/uclamp、迁移及精确窗口状态账。RunnableWaitDensity 是平均等待密度，不是完整队列深度曲线。 |
| C3 IO | `block_pairing.go`、`storage_pairing.go`、`query.go` 的 `computeStorageLatencyByLayer`/IO pressure/inode/burst 路径；`types.go` 的 `IOLatencySummary`、`FileIOSummary`、`PageCacheSummary`、`StorageLatencySummary`。请求驻留时间与真正阻塞提交线程的时间分开；S/D 均可能有 IO 阻塞，必须有完成/唤醒与源身份等凭证；跨层候选不凭时间接近自动并成因果边。 |
| C4 marker/业务/perf | `query.go` 的 `computeTraceMarksWithInventory` 与 span_window 视图等；`business_span_mention.go`、`trace_counter.go`、perf 查询/身份模块。保留窗口 span、完整标记 inventory、counter、样本与业务线索；链外业务标记仅作背景，不能替代链上因果。 |
| C5 帧 | `query.go` 的 `BuildFramePipeline`、`BuildFrameTimeline`、`ResolveFrameTarget`、frame root-cause bundle；`frame_map_relation.go`/official SQL relation 入口；`jank_event.go` 与转换 `jank_marker_conversion_test.go`。已有显式时间窗、目标身份、官方 frame relation 和 jank_event_sync 的独立语义；通用时序邻接明确标为未证明关系，不能当作真实流水线依赖。 |
| C6 自动补齐/投影 | `query.go` 的 `BuildRecipe`：jank、runnable_delay、binder_wait、io_wait、cpu_supply、span_locate、sleep_root 等；`internal/tracediag/` 编译/渲染链上投影。已有调度/资源/帧/根因组合，不应为了新增面板改变已证链选举、时间窗、无证不加冕、系统补齐或旁路输出合同。 |
| C7 当前缺的专用资源面 | C0 可保真保存一般 measure 和相关表，C4 可看一般 counter；尚无 GPU 活跃时间 × GPU 频率驻留、PreferredFrameRate 全窗分布等独立类型化聚合。需要资源身份、量纲、时间覆盖声明，不能靠名称相似直接当 CPU 执行或帧根因。 |

## 3. 逐项清单

参考路径以参考仓为根，表内 YAML 均在 `config/indicators/`，预处理简称均在 `core/preprocess/`。Codrax 路径见 C 索引。

### 3.1 CPU：11 项

| 指标 | 参考实现细节 | Codrax/状态 | 具体差异与处置、ROI |
|---|---|---|---|
| `cpu_load` | `cpu/load.yaml` → `cpu_load_ops.py`，Running 与查询窗口相交，分 CPU/线程负载与桶。 | C1/C2，覆盖 | CPU-ms、进程/线程 census 和真实运行负载已在；按需复用既有查询，不重做一个名字相同的工具。特定图表桶展示可增，P2。 |
| `cpu_freq` | `cpu/freq.yaml` → `smt_ops.smt_residency`；按芯片 sibling 拆 ST/MT 与频率交集，线程/进程筛选在 sibling 数据之后。 | C1，部分 | 频率驻留/供给已在；缺物理 SMT sibling 拓扑与 ST/MT 专用分解。参考 SQL 有 start-only 筛选，不能照搬；硬件拓扑需证据而非固定 CPU 配对。P2。 |
| `cpu_freq_breakdown` | `cpu/freq_breakdown.yaml` → `freq_breakdown_ops.freq_breakdown`，调度、频率、idle 状态机产出 16 列明细及 JSON/Excel。 | C1/C2，部分 | 分散的执行/频率/约束信息已有，缺统一的按区间导出数据集；先规范可复用区间合同，Excel 非首批。P2。 |
| `cpu_freq_merge_breakdown` | `cpu/freq_merge_breakdown.yaml` → `freq_postprocess_ops`，读取多个明细 JSON 拼接，再重新汇总。 | C1，缺语义面 | 缺离线多段 CPU 明细合并；必须先验明时钟、源、同名线程身份及区间重复，否则双计。用户当前单窗链根因优先，P3。 |
| `cpu_freq_occupancy` | `cpu/freq_occupancy.yaml` → `freq_postprocess_ops`，分 cluster/frequency；载入明细时 effective ST = ST + 0.6×MT。 | C1，部分 | 真实频率占比已有；芯片模型加权 ST/MT 不是实测吞吐量，不把 0.6 变成所有平台硬合同。可作为显式有版本的估算模型，P2。 |
| `cpu_freq_compute_load` | `cpu/freq_compute_load.yaml` → `freq_postprocess_ops`，effective ST × compute_scale × 1024 × freq/fmax。 | C1，部分 | 已有观测供给缺口/频率归一化；参考是依赖 chip 配置的模型量，不是实测指令/功耗，更不是必然可消除的毫秒。仅补显式模型观测可取，P2。 |
| `cpu_freq_compute_load_by_process` | 同名 CPU YAML → 同一算力模型按进程聚合。 | C1，部分 | 进程真实 CPU-ms 已有，缺跨 cluster 的模型负载分布；先补拓扑/校准来源和未知覆盖，不以名字聚合跨源进程。P2。 |
| `cpu_freq_compute_load_by_thread` | 同名 CPU YAML → 同一算力模型按线程聚合。 | C1/C2，部分 | 同上；需要源+进程+线程+生命周期，不能只按线程名合并。P2。 |
| `cpu_freq_func_samples` | `cpu/freq_func_samples.yaml` → `freq_postprocess_ops`；支持库内 perf、外部 `.data` 转换/缓存及无样本，按线程名关联。 | C1/C4，部分 | perf 样本、线程身份、CPU/frequency 查询已有，缺统一分频热点剖面；参考按名字关联会撞名，应用已有证据身份键。P2。 |
| `cpu_state_freq` | `cpu/state_freq.yaml` → `cpu_state_freq_ops`，idle-state × frequency 区间扫描，按 cluster 聚合。 | C1，部分 | CPU busy/idle 和频率驻留都有，缺各 C-state ×频率的交叉分布；需要完整窗口前继与状态覆盖。P1/中高，可共享区间聚合基础。 |
| `cpu_freq_throttle` | `cpu/throttle.yaml` → `throttle_ops.detect_freq_throttle`；freq_limit、thermal boost、实际频率对比芯片上限，分热/负载原因。 | C1，部分/不移植推因 | 限频上下限、观测域已有；缺某些平台 thermal 域语义，但“没有 thermal overlap → load_based”不是原因证明。只补有源观测，不补猜因硬门。P2。 |

### 3.2 GPU：2 项

| 指标 | 参考实现细节 | Codrax/状态 | 具体差异与处置、ROI |
|---|---|---|---|
| `gpu_freq` | `gpu/freq.yaml` → `gpu_freq_ops.gpu_freq_aggregate`，GPU active 与 freq 区间相交；SQL Hz→MHz，输出活跃总时长、频点分布、加权均频。 | C0/C7，缺语义面 | 原始 measure 已有保真通道，缺资源身份受限的 GPU active/freq 查询。必须保留无频率覆盖的 active 时间，而非默认为 0/某频率；不能把 GPU 低频旁证直接选成帧根因。P1/高。 |
| `gpu_freq_ts` | 同 YAML → `gpu_freq_ops.gpu_freq_residency`，按 bucket_ms 加权频率时序和 min/max，缺覆盖桶不前填。 | C0/C7，缺语义面 | 可与上一项共享区间归一化和 coverage，输出统一的资源时序而非专用模型硬提示。SQL start-only 会漏窗口头，需自主修正。P1/高（同批）。 |

### 3.3 调度：8 项

| 指标 | 参考实现细节 | Codrax/状态 | 具体差异与处置、ROI |
|---|---|---|---|
| `sched_wakeup_latency` | `sched/latency.yaml` → `sched_ops.compute_wakeup_latency`/formatter，P50/P90/P99 与最慢 20 个；源状态查询有 LIMIT。 | C2，覆盖 | 已有调度延迟 P50/P95/P99、wakeup/run 证据；百分位不同不是整项 gap。新增 percentile 参数要用完整 census，不在截断样本上冒充全集。P2。 |
| `thread_state_durations` | `sched/runtime.yaml` → `sched_ops.compute_thread_state_durations` 等，状态/优先级/唤醒者/迁移/抢占。 | C2，覆盖 | 目标窗口 Running/Runnable/S/D/unknown 账及边界处理更精确；保留现有 S≠必然 IO、D≠必然已证 IO。参考 start-only/LIMIT 不移植。 |
| `sched_affinity` | `sched/runtime.yaml` → `parse_sched_next_info`/affinity formatter，读取 arg_set 调度元数据。 | C1/C2，覆盖 | 已有 affinity/next_info、可用 CPU 与真正 runnable 受限区间；不能仅凭掩码为单核就宣布响应根因。 |
| `sched_cgroup` | 同 YAML/解析器 → cgroup formatter。 | C1/C2，覆盖 | 已有 cgroup/约束 epoch；不同平台组名只是观测，不硬编码“Camera 必在 top-app”。 |
| `thread_runtime` | `sched/runtime_agg.yaml` → `aggregate_thread_runtime`，sched_slice 的运行时、CPU、迁移聚合。 | C1/C2，覆盖 | 线程/进程真实执行 census 已在；不照搬 start-only 与 LIMIT 后统计。 |
| `thread_sched_analysis` | `sched/thread_sched.yaml` → `thread_sched_ops`，结合芯片配置判断绑定、迁移、负载/算力。 | C1/C2，部分 | 观测面已有，缺芯片特定拓扑资料驱动的汇总面；参考优先级 0–40/41–99/≥100 解释不能全球化，Codrax 必须维持平台语义。P2。 |
| `concurrent_parallelism` | `sched/parallelism.yaml` → `parallelism_ops` sweep；Running 与 Running+Runnable 的 CPU-time/active-wall-time 比值。 | C1/C2，部分 | 已有运行负载/等待密度，缺全窗时间加权并行度分布及可比 denominator；参考 active-only 与全窗均值不是同一口径，需命名区分。P1/中高。 |
| `sched_runnable_queue_depth` | `sched/runnable_queue_depth.yaml` → `sched_ops.compute_runnable_queue_depth`，按 TID union R/R+ 后 sweep、bucket max/分位；缺 dur 推到下一状态。 | C2，部分 | 缺稳定的队列深度时序、零深度覆盖和 duration-weighted 分布。已执行证实参考右端点多占一桶，且 avg 是非零桶最大值均值；独立实现半开区间，不能拷贝该算法。P1/高。 |

### 3.4 IO：14 项

| 指标 | 参考实现细节 | Codrax/状态 | 具体差异与处置、ROI |
|---|---|---|---|
| `io_bandwidth` | `io/io_block.yaml` → `io_ops.format_io_bandwidth`，filesystem_io 按 100ms 分桶；含读写/尺寸分组。 | C3，部分 | bytes/operation/层级/窗口统计已在；缺稳定全窗读写 bytes/s 时序。参考 sum(bytes)/sum(request dur) 不是并发下的墙钟带宽，必须按桶跨度并披露覆盖。P1/中高。 |
| `io_iops` | 同 YAML → `format_io_iops`，按窗口/桶计数。 | C3，部分 | 完成、提交与配对 census 已有；缺按事件定义分开的 IOPS 曲线。明确 issue-rate 与 completion-rate，不能混到真实并发深度。P1/中高（与分布同底座）。 |
| `io_read_write_ratio` | 同 YAML → `compute_read_write_ratio`，count/bytes 的读写分布。 | C3，部分 | Block/File/Storage 层观测能追溯操作，缺一项稳定的全窗比例摘要；分层去重，不将同一请求多层重复累加。P1/中。 |
| `io_latency` | `io/io_latency.yaml` → formatter，单请求 duration/read-write/线程、top 延迟。 | C3，覆盖 | 请求驻留、平均/最大、最慢请求及提交线程实际阻塞已有；Codrax 对 S/D 与完成唤醒的证明更严格。不能将请求全长直接计为响应链阻塞。 |
| `io_latency_percentile` | 同 YAML → `io_ops` percentile，P50/P90/P95/P99 与 min/max/mean，读写过滤。 | C3，部分 | 缺面向完整合格配对总体的分位数摘要；已有私有 `ioLatencyCensus` 可以复用，不能从 Top8/TopN 倒算。这是最有界的高 ROI 候选之一。P1/高。 |
| `io_concurrency` | 同 YAML → `compute_io_concurrency_agg`，20ms 桶统计发起次数/发起线程，top3；有带宽/延迟阈值过滤。 | C3，缺语义面/不移植算法 | 现算法是 arrival-count，并非同时 outstanding 的请求数，还会静默过滤部分桶。应从 issue→complete 合格区间 sweep，并独立给发起率；缺失完成/歧义请求不能装成完整深度。P1/高但需 coverage 合同。 |
| `io_size_distribution` | `io/io_size.yaml` → `compute_io_size_distribution`，请求大小分桶、读写区分。 | C3，部分 | bytes 字段已有，缺公有直方图；不采用“<64KiB=随机，否则顺序”的尺寸代理结论。P1/中（同批易共享）。 |
| `page_cache_add_rate` | `io/io_page_cache.yaml` → pagecache add 分桶，inode/process。 | C3，部分 | PageCacheByInode 已有 add/delete/churn/bytes 与源字段；缺时序速率。需 dev+inode+身份而非单 inode，页大小不固定为 4KiB。P2。 |
| `page_cache_evict` | 同 YAML，以 LEAD 配 add→delete、快速淘汰 <10ms。 | C3，部分/不移植配对 | 缺页生命周期/驻留分布。参考 LEAD 的 `(inode,pfn)` 与 gap 的 `inode` partition 不一致且无 dev，可能错配；应精确页身份和世代，不与 IO 唤醒混同。P2。 |
| `hot_file_cache` | 同 YAML，按 inode/comm 排访问、add/delete、PFN 数。 | C3，覆盖/部分 | TopIOInodes/PageCacheByInode 已有热点与完整计数来源；可增按页占用演进，不需要另做弱身份热点。纯热点是排查方向不是响应主因。P2。 |
| `io_by_priority` | `io/io_priority.yaml`，filesystem_io × 同 itid 全窗 sched_slice，再按 priority 聚合。 | C2/C3，部分/不移植 JOIN | 缺事件发生时优先级分层摘要；参考一请求乘多段调度记录导致重复 count/dur。正确方向是事件时刻的唯一 priority epoch，未知留未知。P2。 |
| `io_latency_by_priority` | 同 YAML，优先级和 read/write 的平均/最大延迟。 | C2/C3，部分/不移植 JOIN | 同上；且该指标 priority<100 归 RT，与兄弟指标 priority>40 且 <100 才 RT 的口径不一致。先做平台型 priority class 单源，不能照搬。P2。 |
| `dstate_io_by_priority` | 同 YAML，D/D\|W 线程状态 × 同 itid 调度片段，汇总时长。 | C2/C3，部分/不移植归因 | D-state 统计已有；D 并不等于磁盘 IO，更非已证请求阻塞，参考还会 JOIN 倍增。只在独立状态分布中展示 D；已证 IO 用既有 request/wakeup 凭证。P2。 |
| `rt_thread_io_block` | 同 YAML，RT 线程 D 时长、IO 时长/次数组合。 | C2/C3，部分 | 可增有证 RT 分类的风险概览，但要分别列 D/请求驻留/实际阻塞，不能合计重叠毫秒或凭 RT 身份硬冠根因。P2。 |

### 3.5 marker/process/startup/sleep/D-state：7 项

| 指标 | 参考实现细节 | Codrax/状态 | 具体差异与处置、ROI |
|---|---|---|---|
| `animation_interval` | `marker/animation_interval.yaml` → `animation_ops.match_animation_events`，平台 marker 签名匹配动画区间及隐式窗口。 | C4/C5/C6，部分 | span定位、窗口与业务线索已有；缺有版本的平台动画事件适配器。只解析已识别事件协议，不能扫描用户原文触发硬门；自动窗口不能覆盖用户显式时间窗。P2。 |
| `so_load_stats` | `marker/so_load_stats.yaml` → `so_load_ops`；H:/path.so / dlopen marker，trim后按库/线程/加载类型/count/dur聚合。 | C4，部分 | 原始 marker/耗时线索在，缺共享库加载专用分布。参考 start-only 和未裁剪 duration 会跨窗多计；应复用全 span inventory 并保留原身份/名称。P2/较有界。 |
| `trace_marker_hotpath` | `marker/trace_hotpath.yaml` → `marker_ops`；调用树、子调用扣除 selftime、线程状态交集、PMU按重叠分摊、业务 tags。 | C4，部分 | 业务 span 保真与链上业务提示已有；缺通用嵌套 marker selftime/状态占比树。不要把 inclusive/selftime重复合计，PMU重叠分摊需标估算，tags不能制造因果边。P1/中高但实现面较大。 |
| `process_profile` | `process/profile.yaml` → `process_profile_ops`；TID→process→allthreads，状态负载、争用评分、topmarker。 | C1/C2/C4，部分 | 已有进程 census、topthread与状态视图，缺一致的一站式 process profile。可在 recipe 层组装，不复制别名分裂的统计；争用评分仅软建议。P2。 |
| `launch_phase_breakdown` | `startup/phases.yaml` → `launch_ops`；spawn/launch/ability/transaction/vsync，排后台 transaction，冷/热启动和阶段瓶颈。 | C4/C5，缺语义面 | 已保留通用生命周期 span，缺显式启动实例/阶段模型。按进程代次和实例ID配对，末端完成/不完整须分开；最长阶段不自动成为链上主根因。P1/中高（不是小补丁）。 |
| `thread_sleep_summary` | `sleep/thread_sleep_summary.yaml` → `sleep_ops`，S窗口、直接waker递归 S/D、路径visited、深度5/节点50/top5/min1ms。 | C2/C3/C4/C6，覆盖/部分 | 唤醒链、S/D区别、业务支撑已有；参考更偏睡眠树展示，可复用图层呈现层次但不降低 typed 边条件。未知中断唤醒不应静默伪补；容量截断需披露。P2。 |
| `uninterruptible_dstate` | `uninterruptible/uninterruptible_dstate.yaml` → `uninterruptible_ops`，D/D-IO/D-NIO + caller reason/count/dur。 | C2/C3，覆盖/部分 | 窗口 D统计和IO证明已在；平台 caller数据可补业务可读解释，但 D-NIO/D未知与已证IO不能合流。P2。 |

### 3.6 Rendering：11 项

| 指标 | 参考实现细节 | Codrax/状态 | 具体差异与处置、ROI |
|---|---|---|---|
| `frame_ts` | `rendering/frame_ts.yaml` → `frame_ops.build_render_frames`；Send/Recv now、UI Marsh transactionFlag[tid,seq]→RS ProcessCommand，再连 Render。 | C5，部分 | 通用帧与官方DB关系已在，文本 marker 的 transaction/sequence 跨线程协议未全适配。应补确定身份关系；参考 Render候选允许vsync差±3且time_window未用，不能整段复制。P1/高。 |
| `frame_summary` | `rendering/frame_summary.yaml` → `frame_ops` summary，总帧数、jank、P95等。 | C5，部分 | 已有 frame_window/jank_event_sync 筛选、typed frame源；缺按统一完整帧总体的多框架汇总。保留“上报jank”和“推算missed vsync”来源，不能相互冒充或双计。P1/中。 |
| `ui_frame_ts` | `rendering/ui_frame_ts.yaml` → `ui_frame_ops`；Send/Recv now与UI Vsync两阶段。 | C5，部分 | 通用UI阶段在，缺该协议的now精确关联/中断披露。补字段语义适配而非再做名称词表硬门；显式窗口外前继可读但输出裁剪。P1/中。 |
| `rs_frame_ts` | `rendering/rs_frame_ts.yaml` → `rs_frame_ops`；vsyncID精确 Render→Commit、fence父子树。 | C5，部分 | 官方关系与一般render阶段在，缺文本专有vsync/fence精确流水线完整面；可优先移植身份思路而非临近时间策略。P1/高（共享frame关系底座）。 |
| `flutter_frame_ts` | `rendering/flutter_frame_ts.yaml` → `flutter_frame_ops`；now==callback.StartTime先配，否则50ms近邻，callback→draw约5ms配对。 | C5，缺框架语义面 | 通用span在，缺Flutter时钟/flow协议适配；近邻fallback只能画“时间关联”，不能生成已证frame因果。跨时钟未经映射不得相减。P2。 |
| `web_frame_ts` | `rendering/web_frame_ts.yaml` → `web_frame_ops`；RealSwapBuffers祖先trace_id、LatencyInfo.Flow/Graphics.Pipeline/Scheduler/Proxy/Raster。 | C5，缺框架语义面 | 缺Web专有flow/trace_id连续阶段模型；原始marker保真不等于关系已查询化。采用source+process+trace_id/generation字段，保留不完整链，不靠模型输出原文检查补边。P2。 |
| `game_frame_ts` | `rendering/game_frame_ts.yaml` → `game_render_ops/`；queueId+sequence连接 Flush/AcquireBuffer，vsync/fence，producer/consumer/phase-drift分类。 | C5，部分 | 通用帧/官方关联已有，缺游戏buffer流水线和buffer实例公共DTO。分类仅建议，真正链依赖要靠queue+sequence/事件关系，不凭阶段耗时P50硬选根因。P1/高但分阶段。 |
| `gpu_pipeline_analysis` | `rendering/gpu_pipeline_analysis.yaml` → `gpu_pipeline_ops`；游戏SwapBuffers子阶段或ArkUI RenderFrame+WaitAcquireFence，GPU频率/负载关联。 | C5/C7，部分 | 可看marker/fence基础关系，缺GPU资源面与等待阶段合并解释。先做GPU观测与已证fence关系，不能同进程GPUwait近邻自动连边。P2，依赖GPU基础。 |
| `jank_intervals` | `rendering/jank_intervals.yaml` → `jank_interval_ops`；同screen/fence Present间隔、刷新率基准和倍数严重度。 | C5，部分 | jank_event_sync已支持而非缺失；缺该独立的display/present间隔派生视角及期望刷新率条件。必须标上报/推算两类证据；跨屏、缺fence和rate变化不能生拼。P1/中高。 |
| `preferred_frame_rate` | `rendering/preferred_frame_rate.yaml` → `frame_rate_ops.summarize_preferred_frame_rate`；process_measure overlap、render_service优先、无则all_fallback，窗口合并/覆盖/分布。 | C0/C4/C7，缺语义面 | counter原始值在，缺有归属的期望帧率时长分布。不能以all_fallback混合客户进程宣称目标app刷新率；优先补coverage/来源，不额外造rootseat。P1/高（有界）。 |
| `frame_rate_votes` | `rendering/frame_rate_votes.yaml` → `frame_rate_ops.build_frame_rate_votes`；Deliver父子、ProcessVote兄弟、LTPO祖先、后续首个VoteRes，输出rate变化与投票说明。 | C0/C4/C5，缺语义面 | 缺投票事件的请求-决议证据链；有ID/父子关系可结构化，没有代次ID的首个后续VoteRes只能时间关联。适合作为业务线索但不能自动推因。P2。 |

### 3.7 Game：3 项

| 指标 | 参考实现细节 | Codrax/状态 | 具体差异与处置、ROI |
|---|---|---|---|
| `game_experience` | `game/game_experience.yaml` → `game_experience_ops`；FPS、jitter、连续卡顿；`DEFAULT_JANK_THRESHOLDS` 实际为一般83.33ms、严重125ms、致命166.67ms，可由参数覆盖；41.67ms是24fps基准间隔，不是直接卡顿阈值。 | C5，部分 | 通用帧/jank统计在，缺游戏体验分布。参考默认值应为显式评估标尺，不覆盖动态期望刷新率，更不能当链因果凭证。P2。 |
| `game_launch_phases` | `game/game_launch_phases.yaml` → `game_launch_ops`；touch→GameGScene1→3及UE函数，分阶段/关键线程。 | C4/C5，缺业务模型 | 原始事件在，缺游戏启动协议模型。先启动通用实例骨架再插件式扩展，不为某个游戏case做硬名命中。P2，依赖startup。 |
| `game_thread_sched` | `game/game_thread_sched.yaml` → `game_sched_ops`；Unity/UE关键线程识别、runtime/binding/priority/issue。 | C1/C2/C4，部分 | 任意线程调度查询已有，缺业务角色 profile 和组合讲解。名称识别仅软选择器，不能以游戏线程名替代身份/优先级/因果证据。P2。 |

### 3.8 Groups 与 L1 budget：7 项

| 指标 | 参考实现细节 | Codrax/状态 | 具体差异与处置、ROI |
|---|---|---|---|
| `jank_essentials` | `groups/jank.yaml`，CPU/marker/PMU/hiperf/D/sleep/thread_sched组合。 | C6，覆盖/部分 | jank recipe、帧根因bundle和自动资源补齐已有；缺的是部分原子指标/业务面板，不是少一个MCP组合入口。按typed任务/证据需求补齐，不按用户关键词硬路由。P2。 |
| `jank_deep` | 同文件仅 expands_to jank_essentials。 | C6，覆盖 | 参考当前没有比essentials更多的原子能力；不能把两个别名误记成两个独立gap。 |
| `launch_essentials` | `groups/launch.yaml`，freq/load/PMU/marker/hiperf/D/sleep，并非直接包含launch_phase_breakdown。 | C4/C6，部分 | 普通资源组合可覆盖，启动实例/阶段查询缺口见startup，不新增重复统计管线。P2。 |
| `game_launch_essentials` | `groups/game_launch.yaml`，game_launch_phases/freq/load/thread_sched/hiperf。 | C6，部分 | 缺游戏启动阶段原子语义，不缺可组合执行能力；依赖原子完善后统一recipe。P2。 |
| `sched_comprehensive` | `groups/sched.yaml`，wakeup_latency/state_durations/affinity/cgroup四项。 | C1/C2/C6，覆盖 | 现有runnable_delay/cpu_supply等可组合对应观测。参考reasoning_hints的“D高=IO/锁”“单核mask=严重绑定”不作确定性判因。 |
| `memory_comprehensive` | `groups/memory.yaml`，多内存观测组合。 | 部分，扩展域联审 | 逐项内存/采样/扩展对照见 `docs/design/hmosperf_extended_metrics_comparison_20260917.md`；core这里只记组装入口，不重复计为六项新gap。新资源统计不绕开链根因边界。 |
| `l1_budget` | 根 `l1_budget.yaml` → `l1_budget_ops`，wait=S+D、supply=R+compute不足、load=Running-compute不足、unknown余项，dominant/margin/borderline。 | C1/C2/C6，部分/不移植裁定 | 已有目标窗口状态账、供给和链投影；可借鉴统一可加的“时间账视图”，不能用窗口max桶替代链上主根因。参考Running=R=0时纯S/D窗返回空，unknown占97%仍给dominant，不能移植。P2。 |

## 4. 参考实现的反例与不能照搬的部分

这些是参考仓风险，不是通过“参考有此函数”推导出的 Codrax 故障。

1. **IO 优先级 JOIN 放大。** `io/io_priority.yaml` 的请求/状态仅按 itid 与全窗多条 sched_slice 关联，不绑定请求时刻唯一 priority epoch。一请求遇到 N 条调度行会被累计 N 次；各指标 RT 分类还不一致。应复用平台优先级语义及时间戳epoch，不复制SQL。
2. **窗口头部遗漏。** CPU `freq`、`state_freq`、GPU、部分 sched/marker SQL以起点 BETWEEN/≥过滤，而非区间重叠与裁剪；一个在窗口前开始、窗口中结束的频率/执行/调用可能漏计。Codrax已有窗口前继和半开区间处理，不应回退。
3. **帧 ID 邻近被当作关联。** `frame_ops._match_render_by_vsync` 接受vsync差值±3，声明的time_window_ns没有实际约束。只通过AST提取该纯函数（没有import参考包/启动服务），调用候选 `(999999999,100,102,'other-frame')`、期待ID100、time_window_ns=1，仍返回 `(999999999,100,102)`。因此不能将此配对升级为精确依赖。
4. **Runnable 桶右端多计。** 同样隔离执行 `sched_ops.sweep_depth_summary([(0,10)], 0,20,10)`，返回 ts=0 与 ts=10 两个depth=1桶、avg=1。半开区间 `[0,10)` 不应占据第二桶，全窗时间均值为0.5。参考分位数统计还是“非零桶最大深度”的分布，不是时间加权真实深度分布。
5. **IO 术语替代实际量。** `io_concurrency`统计的是20ms内发起次数，非同时在途请求数；`io_bandwidth`的字节/请求时长和是服务率而不是墙钟带宽；以请求大小推断随机/顺序不可靠；`compute_io_concurrency_agg`还静默按带宽/延迟阈值过滤桶。新实现必须给不同量独立字段和覆盖说明。
6. **无热证据并不证明负载限频。** `throttle_ops`在没有thermal重叠时给load_based；该分支最多“原因未明确”，不能成为硬因果合同。ST+0.6×MT及chip compute_scale同样是模型假设，需要来源/版本/适用范围。
7. **页缓存身份和生命周期。** page_cache SQL缺dev轴，LEAD的partition还不一致；扩展域复核导入器有括号pid与发出TID混同、4KiB固化等风险。Codrax已有dev/inode/source约束，生命周期能力应沿该方向补，不能降低身份严谨度。
8. **跨框架时间邻近与补端点。** Flutter的近邻fallback、frame_rate_ops的异常fence终点补到下一开始、后续首个VoteRes都需要“推测/时间关联/修复来源”披露；不得把修复出来的数值当作原始因果凭证，也不能把跨时钟差值当真实延迟。
9. **L1 budget不是主根因选举。** `test_l1_budget_ops.py`本身固定了零Running/Runnable返回空的行为，即纯阻塞窗口可能没有budget；unknown很大时的dominant只在三个观测桶中选最大。Codrax可以提供状态账，但必须维持链上资格门。内部`dominant/borderline/unknown`应转成可读解释，不能直漏枚举。

## 5. 按 ROI 合并后的实施队列

表内 gap 是一类能力，不是为一份答案新加一个特例。实现不得扫用户输入/模型答案原文做硬门；名称解析只针对源事件已知协议，模糊匹配最多软提示。

| Gap | 统一补充方案 | 优先级/依赖 | 当前状态 |
|---|---|---|---|
| CORE-IO-DIST | 从完整、类型化、源隔离的IO请求总体派生延迟分位、尺寸分布、读写比例与起止事件速率；request residence、issuer blocking分列。 | P1/高，已有census，最有界 | 审计确认，待实现；不是现有IO链缺失 |
| CORE-SCHED-DEPTH | 已归一化线程状态区间→per-TID union→全窗sweep，得到并行度/等待深度的时间分布、峰值及可视桶。 | P1/高；与C-state×freq可共享区间组件 | 审计确认，待实现；需先红测端点/零区间 |
| CORE-GPU-OBS | 将已有GPU原始measure提升为有来源/单位/资源身份的观测，再求active×freq交集、驻留分布、均频曲线。 | P1/高；不是主因自动选举 | 审计确认，待实现；保真载体已在 |
| CORE-FPS-CONTEXT | PreferredFrameRate有归属时序与覆盖摘要，动态刷新率只作为派生卡顿标尺；vote事件另保精确/时序关系等级。 | P1/中高，有界聚合先行 | 审计确认，待实现 |
| CORE-FRAME-PROTOCOL | 统一帧实例/关系DTO，按协议适配now、vsyncID、transaction tid+seq、queueId+sequence、trace_id；同一套图/关系合同消费。 | P1/高但工作量较大；分协议灰度而非复制heuristic | 审计确认，待设计分批；C5已有官方relation不要重做 |
| CORE-MARKER-PROFILE | 全span inventory建立有身份的嵌套调用树、inclusive/self/state accounting、so加载统计；业务词面单源。 | P1/中高，不能重复计父子时长 | 审计确认，待实现 |
| CORE-STARTUP | 启动实例ID/进程代次/阶段开始完成证据→通用阶段模型→Harmony/game等适配器。 | P2，依赖严格事件实例 | 审计确认，待设计；最长阶段只是观测 |
| CORE-CPU-MODEL | 可声明/可缺失的芯片拓扑、SMT与模型系数；模型量与实测量分开，不能反写已证可消除毫秒。 | P2/P3，需可信设备元数据 | 不直接复制参考chip配置 |
| CORE-RECIPE-PANELS | 按结构化问题/证据需求组合新增观测，与现有recipe共享；不为groups别名复制一套运行时。 | 随原子能力同批 | 既有自动补齐保留 |

本轮主任务另有扩展域的 `native_hook` 资源事件语义面实施；上述核心域清单为后续排队，不与该批未完成的代码争抢文件，也不据此声称 CORE 项已修复。

## 6. 首批候选的红绿测试设计

### 6.1 IO 分布：低风险、复用现有证明边界

1. 先加红测：11个合格配对请求中第11个改变分位数，但展示TopN不包含它；分位数必须从完整census算，不能从public截断列表倒算。
2. 设备/源/读写/sector/长度等身份跨域、重复/重叠导致歧义、缺issue/complete的请求都单独计覆盖，不偷偷加入合格分布；合法0延迟与缺失时长分开。
3. 同一请求block/file/MMC多层同时出现，分别给分层统计，不跨层累计成多倍IO；有明确跨层关系时可以关联但不能重复占用响应毫秒。
4. 窗口前issue窗口内complete、窗口内issue窗口外complete分别说明取样策略；请求全生命周期和窗口内阻塞交集不得同字段混用。quantile插值规则固定并测试。
5. S-state +精确completion→wakeup凭证继续可成为真实IO阻塞；D-state但没有IO凭证不因新统计自动晋升；链上可消除量和投影头行字节保留。
6. 新DTO必须同步query capacity、key-first渲染处置、schema hash、JSON/旁路兼容测试；用户词面用“请求数/覆盖范围/等待分布”，不发射内部枚举。对最终答案不加关键词扫描门。

### 6.2 Runnable 与 GPU：共用精确区间思想，不共用错误语义

- Runnable：`[0,10)`/10ns桶不能占第二桶；20ns全窗后半零等待必须进入分母；同TID的R/R+重叠取并集、不同TID并发相加；duration缺失只允许有可信next-state推端点；未知不能当零；峰值、时间加权均值、bucket-max是三个独立量。
- GPU：活跃区间4ms@600MHz+6ms@800MHz→720MHz；inactive区间不混进active均频；只有一半active覆盖时占比和coverage不谎报100%；窗口前freq接续、同ts更新、Hz/MHz换算、GPU实例隔离均先红后绿；只有freq没有active不制造运行时长或因果边。
- 帧回归：显式窗与自动补齐并存；jank_event_sync筛选不退化；已有官方关系仍可消费；时间邻接继续标为非已证；新GPU背景即使很大也不能挤掉链上的调度、IO、算力、确定性语义优化和业务线索。

## 7. 验证记录与审计边界

本分册进行了静态源代码/SQL/测试断言审读；没有启动参考server、没有导入会注册工具/遥测的包、没有运行客户外联，也没有把整个参考pytest成功当成前提。仅执行两项经AST提取的纯函数反例：frame ID/time-window错配和runnable半开区间桶多计，见§4；这是可执行的参考风险证据。另用主清单 `docs/design/hmosperf_inventory_20260917.json` 对照本表行名：应有63，实有63，缺项0、多项0、重复0；与扩展分册46项合并为109项。

参考测试已定位/审读的代表：`tests/test_gpu_freq_ops.py`、`test_gpu_freq_indicator.py`、`test_cpu_state_freq_ops.py`/`test_cpu_state_freq_indicator.py`、`test_freq_breakdown.py`、`test_frame_rate_ops.py`、`test_frame_rate_votes.py`、`test_frame_drop_preferred_frame_rate.py`、`test_preferred_frame_rate_indicator.py`、UI/RS/Flutter/Web frame相关测试、game render/experience/sched、launch/cold-launch、marker/animation/so、`test_l1_budget_ops.py`和sleep/thread_sched相关测试。它们用于核实算法意图与已知边界，**不表示在本机重新跑绿**。

Codrax 回归锚包括 `cpu_occupancy_test.go`、`cpu_occupancy_process_top_thread_test.go`、`cpu_frequency_limit_summary_test.go`、`frequency_order_p0_test.go`、`query_cpu_constraint_rnb2_test.go`、`target_window_state_account*_test.go`、`target_window_sleep_inventory_test.go`、`block_io_wakeup_chain_test.go`、`block_pairing_window_boundary_test.go`、`io_cross_layer_relation_authority_test.go`、`page_writeback_correctness_test.go`、`business_span_mention_test.go`、`frame_role_authority_test.go`、`frame_target_resolution_window_source_test.go`、`jank_event_fields_test.go`，以及转换侧 `jank_marker_conversion_test.go`/`streamerdb_export_official_relations_test.go`。新批次实施时要选相应回归重跑，审读测试不能代替运行结果。

完成条件：新增语义/聚合经确定性回归和每批最多2个异构eval验证，日志与最终答案一起人工审阅；明确区分实现缺口、数据本来缺失与模型波动；文档状态随后更新，不能仅因已有原始carrier就宣称分析能力闭环，也不能把参考仓不严谨的因果推断当作必须补齐的能力。
