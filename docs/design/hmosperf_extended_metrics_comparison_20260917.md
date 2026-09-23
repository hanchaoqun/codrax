# HarmonyOS Perf MCP 扩展指标与数据导入对照

日期：2026-09-17。Codrax 审计起点：`93bf1a42d`；参考目录 `/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main` 不含 Git 元数据，不能推定它对应远端某个提交。本篇负责 memory、network、video、sampling 及 `core/extensions`，其他指标和工具/skills 总表由配套审计文档覆盖。

状态：**静态实现对照完成；EXT-1 第一批资源元信息实现/确定性回归/构建已闭环，后续生产eval数据交付正确但模型解释FAIL，同源教学已修、修后live未验；其余能力仍待分批实施/验证**。原静态审计未启动参考服务、未发送遥测、未执行参考工作流，也不把“有测试文件”当成运行通过；后续实际执行收据及生产失败见[主账本§7/§10](hmosperf_capability_gap_audit_20260917.md)，实施状态统一见[子任务清单](hmosperf_implementation_tasks_20260917.md)。参考仓只读。

## 1. 口径与总体结论

逐项按 YAML 的 `indicators[].name` 计数，而非文件数：memory 18、network 2、video 7、hiperf 9、PMU 4、sendable 6，共 **46 项**。`network/throughput.yaml` 是空 `indicators: []` 的计划占位，不算第 47 项。`groups/memory` 的编排指标在核心指标文档另列，避免重算。

不能将“保留了原始数据”“提供了领域指标”“允许进入因果根因”混成一种支持状态：

1. Codrax [全表逐 cell 保真导出](../../internal/hitraceconv/streamerdb_text_fidelity.go) 已覆盖输入数据库所有非 `sqlite_%` 表，保留 NULL、INTEGER、REAL 位型、TEXT/BLOB 字节、列元数据及逐表摘要。`native_hook` 的 `heap_size/addr/callchain_id`、已有数据库中的 `native_hook_frame/statistic`、BRBE 或自定义表，并非从转换结果物理丢失；它们目前可能只在该保真层。
2. [语义导出入口](../../internal/hitraceconv/streamerdb_export_extended.go)、[查询分派](../../internal/tracequery/query.go)、[视图容量表](../../internal/tracequery/view_capacity.go) 是另一个闭合能力面。保真层明确不授予 CPU/span/因果权限。本文“缺失”一般指缺少专门语义查询/外部输入适配，**不等于无法保留已入库的原文**。
3. Codrax 的调用栈采样、明确时间窗、生命周期、独立 event/unit 分母、链内与竞争背景区分已有较强基础。新增指标必须接入这些边界，不能照抄参考仓的模糊进程选择、错窗全量回退、资源寿命当执行耗时、采样占比换算执行时间。
4. 真正高 ROI 的第一步是已有数据的安全可见性，而非给每一种指标新增硬门。优先补 native-hook 资源事件元信息，随后考虑通用时钟/来源/实例受约束的日志事件适配与 PMU 描述契约。视频、杀进程、网络告警先作为带来源的诊断事实，不能仅因靠近卡顿窗口就进入链上根因。

下文标签：**已支持**=核心问题已有等价实现；**部分**=已有通用输入/相邻能力但缺专用输出；**缺失**=闭合语义/API 中没有该完整能力；**设计差异**=不应照搬参考流程。P1=近期高 ROI，P2=共享基础就绪后，P3=专门数据/依赖成本较高。优先级不是缺陷严重度。

## 2. Codrax 已核实的对应基础

| 编号 | 实际实现与可用能力 | 不能据此宣称的能力 |
|---|---|---|
| C1 | [native_hook 导出](../../internal/hitraceconv/streamerdb_export_native_hook.go)：事件瞬时点 + 活跃资源计数；精确 owner、发射线程生命周期、Running CPU 见证；资源 `end_ts/dur` 不铸造 B/E；EXT-1 已补 `source_heap_size/source_callchain_id/resource_end_ts_ns`；§169 bcc498bd0再补精确原地址/64位位型及同采集子类名，可经event_search查询 | 尚无堆分配栈榜、地址代次配对、泄漏证明。03.2原地址负数不等于无效，也不承诺有效分配；子类NULL/空名/未解析引用分别保留。其末版验收收据见统一账本§169，不称栈已解析 |
| C2 | [process_measure/live_process/network/log 导出](../../internal/hitraceconv/streamerdb_export_extended.go)：进程计数、PSS、网络收发速率、已入库 HiLog/HiSysEvent 文本；[事件查询/窗口统计](../../internal/tracequery/query.go) | 网络速率不等于丢包/RTT/TTFB；PSS 计数不等于低内存快照状态机；能搜原始日志不等于已经计算视频或杀进程指标 |
| C3 | [perf 语义导出](../../internal/hitraceconv/streamerdb_export_perf.go) + [PerfContext/PerfTimeline 类型](../../internal/tracequery/types.go)：符号、库、调用栈、线程榜、时间桶；采样来源、符号化、时钟、on/off CPU 质量；多 event/unit 分组 | 不把所有硬件事件共享一个权重分母，不把样本数比例自动换成真实执行 ms；树形展示和专门并行改造分析仍有扩展空间 |
| C4 | [raw perf.data](../../internal/hitraceconv/raw_perfdata.go)：可跳过并盘点 branch-stack 等扩展字段，保留基础采样；`BranchStackCount` 读取后跳过分支记录 | 不是已支持 BRBE 基本块/分支预测分析或 SPE 地址级内存访存分析 |
| C5 | [phase_task_delta 原文渲染](../../internal/hitraceconv/official_render.go) + [解析](../../internal/tracequery/parse.go)：事件盘点；[全表保真](../../internal/hitraceconv/streamerdb_text_fidelity.go) 保留数据库列 | 尚不是 delta 槽位到芯片硬件事件的经证明映射，也没有完整 PMU IPC/MPKI 聚合合同 |
| C6 | [资源/FileFields](../../internal/tracequery/types.go)、[链与根因查询](../../internal/tracequery/query.go)：文件/页缓存/I/O 结构化证据；链内候选和周边背景受不同权限约束 | 页缓存增删不是 I/O 完成的唤醒边；资源计数波动和全局 OOM 邻近也不能直接加冕 |
| C7 | [通用日志分析](../../internal/analysis/logtriage/doc.go)：LLM 结构化 Error/Observation/Residue，系统只推导仓库定位等字段 | 不是确定性的 hilog+kmsg+pcap 多时钟导入、每设备内存快照、杀进程效率计算器 |
| C8 | [ArkTS 提取器](../../internal/tool/repomap/index/extract_arkts.go) 与 [解析器](../../internal/tool/repomap/index/resolver_arkts.go)，已有代码关系/读写流程 | 一般类/方法关系不等于 Sendable 跨线程安全证明；只读分析不能绕过写模式审批/验证机制 |

## 3. Memory：18 项逐项对照

源指标：[heap.yaml](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/memory/heap.yaml)、[kill.yaml](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/memory/kill.yaml)、[lifecycle.yaml](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/memory/lifecycle.yaml)、[low_memory.yaml](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/memory/low_memory.yaml)、[oom_kill.yaml](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/memory/oom_kill.yaml)。堆 7 项直接 SQL、无额外 preprocess；其余为占位 SQL + 下列实际算子。

| 指标 | 参考实现细节 | Codrax 对照、gap 与安全移植方向 | 优先级 |
|---|---|---|---|
| `heap_timeline` | heap.yaml:77；事件后 `all_heap_size` 曲线，`heap_size` 为事件大小；malloc/mmap/process/ipid 过滤 | **部分** C1/C2：已有活跃量计数，EXT-1 已补可查询资源事件元信息。专门分配曲线/聚合仍待补，不称“原始丢失” | P1 |
| `heap_alloc_summary` | heap.yaml:152；`native_hook_statistic` 按 callchain/type 热点，字典查叶子库/符号 | **缺失语义聚合** C1：统计表原始保存，但无分配热点 API；参考 SELECT 非聚合 apply/release 列后 GROUP BY，不能照抄为多行求和 | P2 |
| `heap_leak_candidates` | heap.yaml:274；alloc LEFT JOIN free，addr+ipid+事件类型，窗口内没配上释放则计候选 | **缺失**。需进程代次+地址分配代次、先后顺序/捕获边界；参考无 `free >= alloc`，地址重用可错配。窗口末未释放不是泄漏证明 | P2 |
| `heap_thread_summary` | heap.yaml:376；itid 分组 count/SUM heap_size，分 malloc/free/mmap/munmap | **部分** C1：发射线程身份已严格限定，缺四类资源量聚合；资源存活与发生于窗口内的事件计数必须分开 | P2 |
| `heap_callchain_expand` | heap.yaml:481；native_hook_frame 的 depth、IP、库/符号/偏移全栈 | **部分**：原始表保真，普通 perf 全栈已支持，但 native-hook frame 尚无同类语义视图。不能固定“max_depth-2 必为业务叶子” | P1/P2 |
| `heap_mmap_subtype` | heap.yaml:548；MmapEvent sub_type_id→data_dict，按 subtype/ipid 计数/大小 | **缺失语义聚合**仍归14.1；03.2已实现nullable subtype、同采集唯一字典引用及安全JSON名称的瞬时点可查询性（§169），未知不编造成匿名/文件映射，也不按名字猜单位 | P2 |
| `heap_type_summary` | heap.yaml:623；statistic.type 0/1/2/3/9..21 引擎分配释放量 | **部分**：C1 已支持多个资源族事件/计数，不等于 statistic 表求和；引擎 registry 应与上游版本绑定，句柄数量不可标 bytes | P2 |
| `kill_events` | [kill_ops.py:136](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/kill_ops.py:136)：AMS/RSS/AppMS 日志解析，±1 秒同 PID 拼接，多来源 reason | **部分** C7 原文/通用定位，缺确定性 kill 事实通道。先持 source+时间+PID/UID/代次；参考忽略时间参数、同 PID 首个近邻匹配不能直接移植 | P2 |
| `kill_reason_summary` | 同一算子按 `kill_reason_map.json` 的 L1/L2/原因映射统计 | **缺失语义统计**；可复用同一 typed kill 事件，不新增文本硬门。规则版本与 unknown 一并输出，“退出原因”不能冒充帧链根因 | P2 |
| `kill_candidates` | 同一算子从 LogKillableProcInfoList 解析 PID/UID/优先级/size/gpu/dma/rss/swap | **缺失**；候选名单不等于实际被杀。字段缺失与 0 区分、窗口/快照身份、截断范围必须可见 | P2 |
| `process_lifecycle` | [lifecycle_ops.py:72](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/low_memory/lifecycle_ops.py:72)：process exited、RAMTURBO、freeze/thaw 日志 | **部分**：C1/C3 有 trace 线程/进程代次；缺该日志族事件。但不能让 substring/近邻日志覆盖既有生命周期 authority | P2 |
| `process_disappearances` | [snapshot_ops.py:267](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/low_memory/snapshot_ops.py:267)：相邻低内存快照 PID 差集 | **缺失**；只可称“后续快照未再观察到”。先证明快照全量和 PID 代次，否则不能断言进程死亡或杀进程漏报 | P2 |
| `low_memory_snapshot` | [snapshot_ops.py:239](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/low_memory/snapshot_ops.py:239)：kmsg 快照分段，系统内存/PSI/进程RSS/GPU/DMA，多内核格式 | **部分** C2 有采样计数，缺完整快照解析。共享“原始日志→有边界快照→缺失字段”底座后一次补齐4个视图 | P2 |
| `low_memory_total_pss` | 同算子：用户驻留量按 RSS 比例估算各进程 PSS，再整合 GPU/DMA | **部分** C2 可保留实际 PSS；缺此估算输出是能力差异而非精度缺陷。须明确 estimate，不替换真实 PSS、不能把估算视为链因果 | P2 |
| `low_memory_process_rss` | 同算子：最新快照 top N 或 all，adj 分层、RSS/PSS估算、增长、GPU/DMA 来源 | **部分** C2；缺 snapshot 身份/可下钻详细清单。参考增长按 PID 跨快照连接，需防 PID 重用/部分采集 | P2 |
| `low_memory_anomaly` | [anomaly_detector.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/low_memory/anomaly_detector.py)：阈值、PSI差分、增长比、可选baseline；参数改动后重新算 | **缺失**；适合作软提示，不是硬门。固定设备阈值和未知量默认0不能原样迁入；需同配置/采样质量标识 | P2 |
| `oom_kill_events` | [oom_efficiency_ops.py:90](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/low_memory/oom_efficiency_ops.py:90)：kmsg OOM/Killed process，PID/RSS/adj层 | **部分** C7 可识别错误；缺稳定枚举/结构化统计。系统 OOM、被杀线程、目标响应链须分层呈现 | P2 |
| `kill_efficiency` | [oom_efficiency_ops.py:131](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/low_memory/oom_efficiency_ops.py:131)：SIG9→xreclaim、anon/file回收、HVGR/GPU释放分段延迟 | **缺失**。阶段延迟有价值；必须按 owner/代次/上下文与时序配对，未配齐保留 null。不能将各段机械相加后当单个响应阻塞 | P2 |

### Native-hook 第一批最小可实施边界

参考 [heap.yaml:36](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/memory/heap.yaml:36) 给出的语义：`start_ts` 是事件时间，`end_ts/dur` 是释放/活跃寿命；`heap_size` 为本次大小，`all_heap_size` 为事件后总量；`callchain_id` 引用栈，`addr` 为地址。

- 在既有安全瞬时事件上补可选资源元信息，已有 I/C 类型/数量及计数值保持兼容（I 标签追加字段，因此输出字节摘要有显式演进）；它是可查询性增强，不是新的因果边。
- 缺列、NULL、显式 0、坏类型必须区分；坏可选字段只撤回该字段，不能连带丢掉已有合法瞬时点/计数。
- `heap_size` 不因 Free 自动改成负号，释放方向由事件类型承载。FD/THREAD/handle 族不能套用字节单位。
- `end_ts` 的 NULL/0 与有效 end 保留差异；已知正 end 也绝不转成线程执行 span。
- 地址是 opaque 位型，不经过 float；是否允许 SQLite 负整数表示 uint64 高位地址，先核上游契约，不取绝对值、不猜十进制截断。调用栈 ID 的 0/负哨兵不铸造栈已证状态。
- 后续 native-hook 栈解析必须同表/同输入快照、相同 owner scope，完整度与未知符号显式保留；泄漏分析另需地址分配代次，不能仅用 addr+PID。

## 4. Network 与 Video：9 项逐项对照

| 指标 | 参考实现 | Codrax 对照、gap 与边界 | 优先级 |
|---|---|---|---|
| `network_quality` | [network_quality.yaml](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/network/network_quality.yaml) → [network_quality_ops.py:586](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/network_quality_ops.py:586)：按规则抽取蜂窝/Wi-Fi指标、告警计分、unknown/样例/截断披露 | **部分** C2/C7；无确定性RSSI/RSRP等质量聚合。借鉴 no-data=unknown、规则版本、采样证据；分数/关键词只软提示，不得影响成文硬门或链根因 | P2 |
| `network_pcap_quality` | [network_pcap_quality.yaml](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/network/network_pcap_quality.yaml) → [network_pcap_ops.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/network_pcap_ops.py)：DNS/握手/TTFB/重传/5秒吞吐 | **缺专用PCAP适配和语义指标**；C2 的net速率不是等价实现。需会话/方向/DNS完整请求键、捕获重复、进程映射可信度；参考零IP命中回退全量只能背景 | P3 |
| `video_phase_timing` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/video/video_phase_timing.yaml) → [video_ops.py:186](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/video_ops.py:186)：SetSource/Prepare/Playback/Completed/Stopped | **部分** C2 有原文，缺播放器实例状态机。必须同进程代次+播放器/session，不能全局 prepare_start 跨实例配对 | P2 |
| `video_buffer_rotation` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/video/video_buffer_rotation.yaml) → [video_ops.py:219](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/video_ops.py:219)：CodecBufferCircular user/server in/out ID和数量 | **缺失语义视图**；同时间戳不足以合并行，需 codec/session/buffer identity；缓冲缺口是观测，不自动推断卡顿责任方 | P2 |
| `video_render_chain` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/video/video_render_chain.yaml) → [video_ops.py:251](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/video_ops.py:251)：HandleRender index/renderTime/last/curTime、间隔 | **部分** 现有frame/render_pipeline有typed帧链；缺视频专门打点接入。内容时间戳、墙钟和trace时钟必须分列，不把间隔自动当阻塞 | P2 |
| `video_decoder_lifecycle` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/video/video_decoder_lifecycle.yaml) → [video_ops.py:275](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/video_ops.py:275)：Configure/Flush/Stop等阶段 | **缺失语义视图**；参考全局configure/flush游标需改成实例状态机，保留未配对端点、重启和跨进程操作边界 | P2 |
| `video_audio_chain` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/video/video_audio_chain.yaml) → [video_ops.py:312](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/video_ops.py:312)：AudioRenderer状态、AudioSink首包、音频焦点 | **缺失语义视图**；同一媒体session后再建立可证关系。音频焦点变化与视频暂停只可并置，不自生因果 | P2 |
| `video_first_frame` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/video/video_first_frame.yaml) → [video_ops.py:337](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/video_ops.py:337)：首帧六阶段时间 | **缺失语义视图**；不能跨所有行取每字段最早值拼成一次启动；需阶段代次和明确开始请求 | P2 |
| `video_error_events` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/video/video_error_events.yaml) → [video_ops.py:375](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/video_ops.py:375)：E级和decoder/buffer/codec/player/sink类错误 | **部分** 通用日志已有错误语义，缺专门枚举/时间检索；完整原文引用+短显示摘要并存，不用错误关键词改变主因归属 | P2 |

参考还有未单列为指标的 HCODEC 生命周期、decode_speed、NETSTACK HTTP 阶段、用户交互算子。HCODEC 通过 decoder_id 还原配置/首入/首出/EOS/release/jank reason；这些可作为统一媒体实例事件适配的词法清单，不能直接按其全局 ID/时序合并规则照抄。

### 参考实现的限制（静态确认）

- [query_video_hilog_rows](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/video_ops.py:397) 将无括号的多个 `tag LIKE ... OR ...` 与追加 `AND window` 拼接，SQL 优先级使时间窗只约束最后 OR 分支。这是移植前必须修正的合同问题。
- [PCAP 算子](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/network_pcap_ops.py:44) 的 DNS 关联仅 dns_id；握手是 SYN→SYNACK，不是完整三次握手；TTFB 从同方向空 ACK 到首 payload，不能等同应用请求到响应；重传以重复 seq + P 标志近似。这些只能作为明确限定的估计，不能换名后当准确协议诊断。
- `video_ips` 零命中回全量有明确 note，这是可见性优点，但不能让结果继承请求进程权限。
- 参考 `build_user_interaction` 用“未发现手势”等推断暂停原因；缺事件不是主动/被动暂停的充分证据，只可列排查方向。

## 5. Sampling：19 项逐项对照

### 5.1 HiPerf / BRBE / SPE：9 项

| 指标 | 参考实现 | Codrax 对照、gap 与边界 | 优先级 |
|---|---|---|---|
| `hiperf_hot_function` | [hot_function.yaml](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/sampling/hiperf/hot_function.yaml)：sample→depth0 frame→字典/库，线程/符号计数和event_count | **已支持核心能力** C3 perf_stats 的符号/库/栈/线程榜，且独立事件单位。参考用原始timeStamp，不能倒退取消Codrax calibrated timestamp_trace 约束 | 保持/P2展示 |
| `hiperf_thread_hotpath` | [thread_hotpath.yaml](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/sampling/hiperf/thread_hotpath.yaml) → [hiperf_ops.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/hiperf_ops.py)：调用栈、库归类、路径/函数榜、折叠树 | **部分** C3已有完整栈与热点，缺可下钻的共享前缀树/业务层阅读视图。树折叠仅显示，不能删系统库或混采样event分母 | P2 |
| `hiperf_brbe_inst` | [brbe_inst.yaml](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/sampling/hiperf/brbe_inst.yaml) → [brbe_ops.py:20](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/brbe_ops.py:20)：相邻分支基本块、cycles、估算指令数/IPC | **缺失** C4只有branch-stack盘点；参考新旧表合同不兼容，不能宣称可直接移植。需要ISA/分支有效性/跨模块边界/时钟/样本语义契约 | P3 |
| `hiperf_brbe_func` | [brbe_func.yaml](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/sampling/hiperf/brbe_func.yaml) → [brbe_ops.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/brbe_ops.py)：from函数/库分组cycles和估算IPC | **缺失**。分支跳转地址差不等于函数执行指令数；先交付逐分支事实，后经校验聚合，禁止把估算IPC升级成确定算力不足 | P3 |
| `hiperf_branch_mispredict` | [branch_mispredict.yaml](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/sampling/hiperf/branch_mispredict.yaml) → [brbe_ops.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/brbe_ops.py)：branch site count/mispredict rate | **缺失**。3个BRBE指标宜共用同一typed输入，不能每个指标独立解码；历史记录采样率与已剔除无效分支数量须披露 | P3 |
| `hiperf_spe_miss` | [spe_miss.yaml](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/sampling/hiperf/spe_miss.yaml) → [brbe_ops.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/brbe_ops.py)：PC级L1/LLC/TLB miss/latency | **缺失且参考导入未闭环**：当前extender删除spe_sample并不重建，返回spe_sample_count=0；单位/访存样本不等于全指令或执行时间 | P3 |
| `perf_calltree_in_interval` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/sampling/hiperf/perf_calltree_in_interval.yaml) → [perf_calltree_ops.py:309](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/perf_calltree_ops.py:309)：区间树→ArkTS子树/nodes/tree/virtualLinks | **部分/设计差异** C3已有严格窗口栈和证据关系；可借阅读树，不移植“错窗用全量”、包名硬编码兜底、删系统栈、样本比例×窗口ms | P2 |
| `cold_launch_calltree` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/sampling/hiperf/cold_launch_calltree.yaml) → [cold_launch_ops.py:21](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/cold_launch_ops.py:21)：全栈树、低比例剪枝、JSON/TXT输出 | **部分** C3；保留系统业务线索、复用固定窗口可以增强。剪枝应输出折叠数量和恢复引用；该算子仍有越界全量和占比换ms风险 | P2 |
| `cold_launch_hitrace_calltree` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/sampling/hiperf/cold_launch_hitrace_calltree.yaml) → [cold_launch_ops.py:551](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/cold_launch_ops.py:551)：同步span forest、整型ns窗口裁剪、self/overlap、untracked | **部分** span_window/关键链已有。可借窗口内exclusive耗时守恒/路径树，但先验证父子嵌套和重复/交叠，不能把跨线程树作调用因果 | P2 |

**参考的 BRBE/SPE 合同断裂**：

- [hiperf_extender.py:58](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/extensions/hiperf_extender.py:58) 创建新 `brbe_sample(sample_id, from_binary_id/from_offset, to_binary_id/to_offset, prev_to_offset, mispredicted...)` 与 dim_binary/dim_symbol，删除旧 spe_sample。
- [brbe_ops.py:52](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/brbe_ops.py:52) 仍读取 `perf_sample_id/thread_name/from_ip/from_symbol/from_binary/is_mispredicted` 等旧列，PRAGMA分支仅处理 file_offset 有无，无法适配新 schema。
- [test_hiperf_extender.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/tests/test_hiperf_extender.py) 断言新 schema，且真机fixture依赖 Windows exe、缺依赖会 skip；[test_brbe_ops.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/tests/test_brbe_ops.py) 自建旧表。两边各自通过不能证明接缝正确。本审计未执行它们，结论来自相互矛盾的真实列契约。

其他不能照搬项：`build_perf_calltree` 找不到线程时以 `'%sp%'/'%xuncorp%'` 兜底选择；用数值量级猜 ms/ns；窗口越界改全量；count/rootCount×区间ms 是采样比例估计而非精确执行耗时。`thread_hotpath` SQL按callchain_id分组而非thread/event域，不能借非聚合列制造跨域统计。Codrax 现有 [cohort 回归](../../internal/tracequery/perf_aggregate_cohort_test.go) 必须保持。

### 5.2 PMU：4 项

| 指标 | 参考实现 | Codrax 对照、gap 与边界 | 优先级 |
|---|---|---|---|
| `pmu_summary` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/sampling/pmu/pmu_summary.yaml) → [pmu_ops.py:70](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/pmu_ops.py:70)：phase_task_delta槽位映射、perCPU IPC/MPKI及分布 | **部分** C5保留事件，无完整PMU指标。先增加来源证明的delta mapping/芯片/事件单位，再计算；默认映射不是所有采集设置的事实 | P1设计/P2实现 |
| `pmu_timeline` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/sampling/pmu/pmu_timeline.yaml) → [pmu_ops.py:250](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/pmu_ops.py:250)：delta槽位具名逐点、CPU过滤 | **部分** C5。该YAML名叫start_ms/end_ms但描述ns、SQL直接比ts，与另外3项ms参数不一致；新增合同必须显式单位、不猜量级 | P2 |
| `pmu_by_process` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/sampling/pmu/pmu_by_process.yaml) → [pmu_ops.py:358](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/pmu_ops.py:358)：thread/process连接，overlap/dur加权 | **缺失语义聚合**；需TID代次/CPU/事件域。窗口比例裁剪计数是均匀分布估计，应标estimated，不允许当精确指令差值 | P2 |
| `pmu_by_thread` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/sampling/pmu/pmu_by_thread.yaml) → 同聚合器，线程维度、topN/null ratio | **缺失语义聚合**；同一共用实现，零分母→null有价值；缺事件列不能自动形成0次cache miss，空/坏数据与零值分开 | P2 |

参考 `_aggregate_pmu` 的向量化裁剪、分母0→null、unknown桶和 top-N 是可复用的产品思路；源 SQL 仅按采样起点取窗内行，与后续处理 `[ts,ts+dur)` 交叠裁剪的接口也须核对，避免边缘采样在 SQL 层已经被漏掉。source→SQL→preprocess 的整条路径应有回归，不能只给聚合器直接喂窗外重叠 DataFrame。

### 5.3 Sendable 源码/产物辅助：6 项

这一组不是真正的 runtime sampling 指标，是用统一注册入口包装的源码分析和产物生成。对照时不可因此把 6 个名称当成 6 种新采样证据。

| 指标 | 参考实现 | Codrax 对照、gap 与边界 | 优先级 |
|---|---|---|---|
| `sendable_function_analysis` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/sampling/sendable/function_analysis.yaml) → [sendable_ops.py:59](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/sendable_ops.py:59)：外部Node ArkAnalyzer、SceneCache、方法集合分析 | **部分** C8普通代码关系；缺Sendable专用闭包/并行迁移分析。外部依赖/源码版本/SDK版本必须绑定，不能把推测关系当typed call | P3 |
| `sendable_sample_config` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/sampling/sendable/sample_config.yaml) → [sendable_ops.py:211](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/sendable_ops.py:211)：从树节点产配置、路径/类/方法目标 | **设计差异**：可在现有证据→探索建议中表达，不宜新增一个无验证配置中转；路径属于客户repo而非参考仓 | P3 |
| `sendable_class_graph` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/sampling/sendable/class_graph.yaml) → [sendable_ops.py:245](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/sendable_ops.py:245)：Node外部工具生成graph.json、可用缓存 | **部分** C8一般类图；缺字段传播/Sendable规则图。参考缓存只检查现有JSON可解析，缺源码/SDK/配置指纹；不能照搬暖缓存陈腐风险 | P3 |
| `sendable_task_plan` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/sampling/sendable/task_plan.yaml) → [sendable_ops.py:335](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/sendable_ops.py:335)：外部Python graph→todo.md | **设计差异**：已有write计划/验证框架，专用知识应成为受验证建议；生成todo不等于安全可执行计划 | P3 |
| `frame_drops_to_file` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/sampling/sendable/frame_drops_to_file.yaml) → [sendable_ops.py:401](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/sendable_ops.py:401)：analyze_frame_drops→JSON，携线程下钻参数 | **设计差异/已有核心能力**：现有frame bundle、证据blob、root-causes旁路已有产物；可借可重放manifest，不能再建第二套因果选择源 | P2 |
| `sendable_class_guide` | [YAML](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/config/indicators/sampling/sendable/class_guide.yaml) → [sendable_ops.py:466](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/preprocess/sendable_ops.py:466)：每类graph/todo/guide组合 | **设计差异/专用知识缺口**；不能绕过write风险/隔离/测试；参考旧产物rename、工具非零退出仍warning的处理应避免显示为完成 | P3 |

本文已核 wrapper 的入参、连接所有权、外部执行和产物使用；**没有声称完成 bundled ArkAnalyzer/ArkRefiner 底层算法审计**。后续若实现此域，应单独审计依赖目录、许可、模型/代码执行边界及跨SDK正确性，不直接运行参考工具。

## 6. `core/extensions` 逐族数据导入对照

所有“原始保真”均指数据已经存在于输入 SQLite 时；不表示 Codrax 已经能从参考工具的外部日志包/pcap生成对应表。

| 参考模块 | 关键实现/语义 | Codrax 当前对应与补全边界 |
|---|---|---|
| [page_cache_parser.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/extensions/page_cache_parser.py) + [db_extender.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/extensions/db_extender.py) | mm_filemap add/delete，dev/inode/pfn/offset→page_index，写page_cache表；偏移除4096固定页大小；把ftrace括号数字读为pid并赋itid | Codrax C6已有事件/精确页缓存变更；无需复制错误身份映射。原始发射TID与TGID、inode基数、页大小、重复导入需分清；值得补页/文件对象占用轨迹，不新增唤醒边 |
| [hilog_parser.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/extensions/hilog_parser.py) / [kmsg_parser.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/extensions/kmsg_parser.py) / [log_extender.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/extensions/log_extender.py) | 跨目录发现、墙钟/boot时间解析、kmsg双钟中位offset、log_meta；无时钟保留NULL；窗外±600s且部分关键词救回 | C2支持已入库log，C7通用附件；缺这套多源日志适配。可借明确unaligned/来源计数；文件名时间估计不能授予精确因果时钟，关键词救回只能背景，不改变用户主窗口。需保留原文件+行而非只有合并后DBid |
| [time_offset.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/extensions/time_offset.py) | 唯一读取log_meta aligned=true→offset的公共函数 | 统一入口值得借鉴，但布尔aligned不足表达误差/漂移/epoch；新增typed ClockRelation +校准见证+有效区间，不把文件名fallback等同实测映射 |
| [tcpdump_parser.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/extensions/tcpdump_parser.py) / [tcpdump_extender.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/extensions/tcpdump_extender.py) | Scapy PcapReader逐包解析IPv4/IPv6/TCP/UDP/DNS、Linux cooked linktype；全部records再聚合入SQLite；offset或文件名校时；显式目录空不向父目录猜源，多源回退拒绝 | Codrax缺专用PCAP导入；已入库表可原始保真。复制接口而非整包list内存模式；需要捕获接口/包序/截断/解析失败清单，exact时间单位与流标识。参考解析中断返回部分包，仅warning，必须补部分完成状态 |
| [caas_parser.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/extensions/caas_parser.py) / [caas_extender.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/extensions/caas_extender.py) | HME/VQE初始化、Codec Added，墙钟+PID+level；复用offset导入 | 缺专用音频日志适配；通过通用typed日志插件接入即可，不新建因果模型；本地timezone解析和首目录命中策略需显式约束 |
| [vdec_mcore_parser.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/extensions/vdec_mcore_parser.py) / [vdec_mcore_extender.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/extensions/vdec_mcore_extender.py) | 微核独立相对毫秒；BSP负载/通道/分辨率/bit-depth/fake-frame/释放量；wall_ts NULL，不强校到主机 | 缺专用适配；**独立时钟不猜对齐值得保留**。多个微核启动代次、文件来源、chan复用必须加入identity，不将同一ms与主机trace直接拼因果 |
| [hiperf_binary_parser.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/extensions/hiperf_binary_parser.py) / [hiperf_symbol_resolver.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/extensions/hiperf_symbol_resolver.py) / [hiperf_extender.py](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/core/extensions/hiperf_extender.py) | mmap rawperf、MMAP/feature192符号、BRBE有效性/cycle字段、预计算prev_to_offset和函数入口、unsigned64转SQLite signed位型 | C3/C4已有基础perf安全解码/采样，缺分支/SPE语义。参考(pid,address)符号缓存无时间代次、跨sample前一分支延续、sample链接0fallback均要审计。新extender与旧ops断裂见§5，先闭合来源和字段合同才做计算 |

## 7. 验证证据与覆盖不足

已阅读实际断言的代表测试：

- [low_memory_snapshot_ops](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/tests/test_low_memory_snapshot_ops.py)：快照分段、PSS估算、增长、PSI字段、缺表/no snapshot；没有以此证明PID重用/多源时钟/部分快照不误判。
- [network_quality_ops](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/tests/test_network_quality_ops.py)：窗外unknown、无trace_range全量披露、未对齐跳过、规则错误、样本截断等；这些边界值得借鉴。
- [network_pcap_ops](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/tests/test_network_pcap_ops.py)：真实pcap的结构/计数/零IP回全量；并不验证DNS ID碰撞、ACK/首字节真正方向或重传语义。
- [video_ops](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/tests/test_video_ops.py)：真实HCODEC样本与合成HiPlayer、同ts buffer合并、首输出/EOS；不足以证明多播放器交错、decoder ID复用、所有tag都服从窗口。
- [pmu_by_process_thread](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/tests/test_pmu_by_process_thread.py)：直接DataFrame输入的裁剪/NaN/零分母/topN；并非证明上游SQL交叠取数和TID生命周期正确。
- [sendable_ops](/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/tests/test_sendable_ops.py)：context_index和递归参数解析/连接所有权/命令形；不能代替真实源码版本缓存与Sendable安全性验收。
- Codrax [perf_aggregate_cohort_test.go](../../internal/tracequery/perf_aggregate_cohort_test.go) 已钉独立事件单位、DSO身份、溢出局部隔离；[native_hook生命周期测试](../../internal/hitraceconv/streamerdb_native_hook_lifecycle_test.go) 已钉资源结束时间不是发射点。扩展要保留这些保证，而不是用新增规则绕过。

本篇没有跑模型eval，也没有把上述测试写成“全绿”。目前证据足以划分实现能力与移植边界，不足以宣称参考工具所有现实设备场景正确。

## 8. 分批收敛建议与统一验收面

| 批次/跟踪项 | 有界交付 | 验收与禁止事项 | 当前状态 |
|---|---|---|---|
| EXT-1 native-hook 可见元信息 | 第一批仅原整数大小、来源栈键与nullable资源end；地址位型/栈解析后续另批，原始保真已保留 | 缺列/NULL/0/坏storage/超大整数/跨owner/同ts稳定顺序；非字节族不误标bytes；坏可选字段不吞合法I/C；资源end绝不变B/E | 第一批公共红绿/count3/race3、四包回归、独立复核及构建通过；后续live数据正确但解释FAIL，教学已修未复播，见主账本§7/§10及任务03.1/16.3/18.2 |
| EXT-2 共用日志事件输入 | 统一source-file/line、clock、PID代次、instance、原文引用、完整性；先kill/OOM或视频中一个小族 | 多文件时钟/时区/重启、缺字段、部分捕获；空窗口不全量升级；source日志词法解析允许，用户/模型散文硬门禁止 | 待实施 |
| EXT-3 PMU 描述与聚合 | 明确capture delta_mapping/单位/CPU事件域；一次底座供summary/timeline/process/thread | 未知映射不猜；整型计数、零分母、缺列、事件域分隔、复用TID、部分交叠估计披露 | 待实施 |
| EXT-4 无损调用树展示 | C3已有样本→前缀树、展开/折叠与业务解释，保留全部原始栈/来源 | 原始图关系与显示标签分离；不从折叠父子直接铸call；不删除系统/编译/GC等根因线索；不拿样本比例装精确ms | 待实施 |
| EXT-5 堆/内存专题 | EXT-1后native-hook栈、分配聚合；EXT-2后完整快照/RSS/GPU/DMA/kill效率 | 地址代次+捕获边界，真实值与估算区分，重复采集无双计，候选不等于泄漏/根因 | 待实施 |
| EXT-6 PCAP/媒体扩展 | 共享时钟/来源底座后，包级或媒体实例typed事件；再出诊断视图 | DNS全键、流方向/重传精度；decoder/player/session复用；所有非链结果只能背景 | 待实施 |
| EXT-7 BRBE/SPE/Sendable | 专门输入契约和接口接缝先通过；再用现有读/写能力承载分析建议 | 参考缺口不能照搬；无设备/工具链见证不承诺闭环；不得绕过write审批或Trace因果证据门 | 待实施 |

每批应先跑确定性fixture，再以现有eval重要性排序选**两个并行**的异构场景（例如资源字段读模式 + 明确时间窗Trace、日志专题 + 写模式）。逐个审计工具入参、传给模型的证据覆盖、重试原因、最终正文/图/旁路一致性。不得把模型随机漏写通过新增散文关键词硬门固化，更不得因系统新增不相容合同让正确答案反复修补至消失。

新增族统一负例：邻近但不在链上、同名不同实例、同PID不同代次、同数值不同clock/单位、未采集而非零、样本/截断而非完整、系统库/业务线索共存、图的显示标签不反写实体ID。只有这些跨族边界通过后，才把增强标为已闭环。
