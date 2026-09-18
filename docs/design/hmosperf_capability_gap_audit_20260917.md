# HarmonyOS PerfMcpServer 能力对照与分批增强

日期：2026-09-17。Codrax 审计基线：`93bf1a42d`。参考目录：`/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main`（本地归档，没有 Git 元数据）。

## 1. 范围、证据与结论口径

按用户顺序，先闭环 B1716 并提交推送 `93bf1a42d`，再进行本审计；r1088 尚未开始。本次参考仓只读，没有运行服务器、插件加载器、遥测或客户数据分析任务。

源码清点为 **13 个对外 MCP 工具、22 个工作流可调用工具（其中 11 个不是 MCP）、24 个 skill、5 个渲染管线定义、109 个具名指标**。管线 index 的 14 个目录项不是 14 个已实现定义。旧文档中 8/11 个工具、19 个 skills、97/107 个指标的数字不作验收依据。

机器可读清单：[hmosperf_inventory_20260917.json](hmosperf_inventory_20260917.json)。其中列出每个 YAML 对象的名称、文件、类型及文件 SHA-256，并为 509 个 Python/YAML/LICENSE 文件记录整体内容摘要。MCP 与工作流工具数量由 `server.py` 的装饰器及 `register_executor_tools` 调用独立清点。

逐项对照附录：

- [核心指标：63 个具名指标](hmosperf_core_metrics_comparison_20260917.md)
- [内存/网络/视频/采样/扩展：46 个具名指标](hmosperf_extended_metrics_comparison_20260917.md)
- [24 个 skills 与 5 个渲染管线](hmosperf_skills_comparison_20260917.md)

这里的“全量”指本地归档**声明的工具、指标、工作流逐项静态对照**，不是承诺所有客户数据形态已回放。每一项需分清四层：原始数据保留、语义查询、统计/工作流、因果证明。特别是 Codrax 已将 SQLite 全部非内部表逐单元格保留到 systrace 保真载体；缺少某表的业务查询适配器，不能写成转换物理丢数据。参考仓有名字或示例也不等于其实现正确、合同一致或运行闭环。

总体判断：值得吸收的是领域指标细节、可发现性、分对象证据包、跨次可比性和业务化解释；不另建一套执行内核，不照搬模糊近邻关联、全量回退、样本占比换耗时或正文扫描裁决。

## 2. 对外工具逐项映射

参考定位均为 `server.py` 中同名函数；实现进一步委托 `core/`、`utils/`。Codrax 对应的是能力入口，不要求同名工具。

| 参考工具 | 参考实现细节 | Codrax 现有入口 | 差异与处置 |
|---|---|---|---|
| `get_hitrace_path` | 文件/目录扫描，最多五层，在首个有结果深度停止 | `internal/types/runtime_artifact_selection.go`、运行时附件/转换入口 | 已有显式工件选择；可补多工件清单体验，不复制“目录第一份 trace”或首层命中即完整的假设 |
| `get_log_path` | hilog/kmsg 目录与 gz/txt 选择 | log attachment、log triage | 尚无同等 hilog/kmsg 语义归并及 trace 时钟对齐；列 HMC-05 |
| `get_pcap_path` | pcap 发现、同目录/父目录回退、多来源拒绝猜测 | 通用工件/文件读取 | 没有专用 pcap 流/协议指标适配器；HMC-06，不能静默扩搜父目录 |
| `convert_hitrace_to_sqlite` | TS→DB；mtime 缓存；附加日志/媒体表；传入 DB 时可能修改它 | `cmd/trace_convert.go`、`internal/hitraceconv` | 转换/归档/文本保真已有且输入保护更严；缺口是附加领域表的语义解释，不引入可变输入或 mtime 充当内容证明 |
| `convert_hiperf_data` | TS 采样+符号；额外 BRBE/SPE 扩展 | `internal/hitraceconv` 的 perf 适配器、`trace_perf_bundle` | 普通采样/栈已有；BRBE/SPE/PMU 新数据合同待补。参考扩展器与消费者有列名断裂，见扩展附录 |
| `analyze_frame_drops` | 帧扫描、渲染阶段、投票/偏好帧率与诊断窗口 | `frame_window/frame_timeline/frame_flow/frame_root_cause_bundle` | 不是整体缺失；待补偏好帧率解释及特定业务管线适配。只能准确 ID/同一对象证明连边 |
| `analyze_video_phases` | hilog 规则识别视频故障窗口，再查线程/媒体链 | Trace 通用 marker/窗口/链及日志分析 | 视频会话阶段、解码器/网络跨源证据链缺专用适配；HMC-07 |
| `analyze_launch_phases` | 启动事件阶段、完成窗口、包过滤、Top-N | startup 性能 skill、`span_window`、链/采样束 | 命名阶段目录、首帧之后可交互阶段、跨应用特征部分缺；HMC-04 |
| `identify_chip_model` | 核数/频点/SMT 特征匹配芯片数据库 | `internal/tracequery/core_capability.go`、算力供给/亲和证据 | 可加有来源的设备画像提示，匹配分数不能变硬件事实、校准或因果依据；HMC-13 |
| `list_indicators` | 可搜索指标目录、参数/单位/输出/schema/预处理描述 | `internal/skill/trace_query_views.go` 与工具 schema | 当前视图教学已单源，但领域指标/适用性/数据需求目录不足；HMC-01 |
| `query_metrics` | 组合展开、参数绑定、依赖、逐项 errors、iterate、截断元数据 | `trace_query` 多视图、recipe/bundle、`internal/dataquery` | 已有确定性查询与证据包；领域统计和按对象批量结果索引可增强。不得直接开放无范围 SQL 给因果权威；HMC-01/02 |
| `get_skill_catalog` | 简表/完整步骤/路由建议 | `internal/skill` registry/内置技能 | 已有能力教学，缺统一领域技能目录与输入能力前提对照。关键词仅可用于检索/提示，不可作硬门 |
| `run_skill` | execute/query/clear/resume、工具步骤至 LLM 断点、状态持久化 | `internal/orchestrator` 固定 read DAG、controller-first write、typed evidence | 不移植第二执行内核；吸收可恢复的按窗口证据文件和状态可见性。参考强制 auto_continue 不作为本项目授权规则 |

## 3. 非 MCP 的 11 个工作流工具

22 个 executor 工具与上述 13 个公开工具重叠 11 个（catalog/run 不在 executor 列表）。以下 11 个也在审计范围，没有只按公开 API 漏掉内部能力。

| 内部工具 | 实际作用/参考位置 | 对照与缺口 |
|---|---|---|
| `analyze_parallelism_opportunity_intervals` | `core/preprocess/span_analysis.py`，区间/线程分析 | 现有链+状态+采样可提供事实；显式并行化机会工作流缺，建议需依赖/副作用证明，不能由重叠或样本热度直接宣布可并行 |
| `analyze_cold_launch_intervals` | `cold_launch_window.py`，冷启动窗口集合 | span/frame 发现已有；冷启动业务边界/包与会话身份目录待补 |
| `analyze_l3_markers` | `l3_markers.py`，启动 L3 标记分解 | 通用 span/semantic-work 已有；保留原名与业务分组的启动分解待补 |
| `extract_cross_app_features` | `cross_app_ops.py::extract_cross_app_features_tool`，跨应用特征 | 缺同口径特征/样本数/未匹配账本；不能跨应用拼一条因果链 |
| `analyze_frame_drop_intervals` | 帧卡顿工作流的窗口准备 | 当前 jank/frame 发现+自动补齐已有；批量逐对象完备性账本待增强 |
| `detect_rendering_pipeline` | `server.py`→管线识别模块，5 个定义/置信度 | 可增加软发现提示与输入前提，不能靠线程名/score授予连边资格 |
| `analyze_tag_instructions` | `server.py`，采样/PMU 与 tag 窗口分析 | tag/span 与采样已有；硬件指令/PMU 口径适配缺，HMC-09 |
| `generate_tag_instruction_report` | 多 trace 指令分析 Markdown 报告 | 缺专用可比性报告；只输出 typed 事实并置，不系统代写模型诊断 |
| `export_cluster_package` | `core/batch/closure/package.py`，manifest→证据包 | 已有 blob/运行工件；跨 trace 簇证据索引/不匹配成员账本缺，HMC-11 |
| `merge_cluster_analysis` | `core/batch/closure/merge.py`，校验后 Excel 回写 | 通用 data workflow 可写工件；没有同形簇报告回写产品，属于可选交付能力而非 read-mode 源码写权限 |
| `compare_dual_trace` | `server.py`→12 个对比算子，输入说明只列9维 | 缺完整 baseline/current 身份/设备/窗口可比性与逐维差异工作流；HMC-10，不跨 trace 推导实时因果 |

## 4. 统一缺口账本与 ROI 排序

以下为跨项归并后的施工单；**每个指标、skill 的具体差异和实现出处见附录，不以大类替代逐项对照**。P1 是对当前目标的高收益增强，不代表所有参考能力都是现有缺陷。

| ID | 优先级 | 归并缺口 | 当前边界及最小安全方案 | 状态 |
|---|---|---|---|---|
| HMC-01 | P1 | 指标可发现性/前提/单位目录 | 复用单源 view/schema；列明需要的事件族、单位、缺测，不添加重复 JSON 必填合同 | 待设计 |
| HMC-02 | P1 | 按窗口/对象组织有界证据包 | 复用 artifact/blob 与 typed 对象 ID；独立错误、覆盖/截断、缺对象，不按数组位置猜同一对象 | 待设计 |
| HMC-03 | P1 | native_hook 资源事实易查询性 | 全表原始保真已覆盖；I 语义出口补本次资源量/来源栈键/nullable 资源 end，不铸执行时长/泄漏/根因；地址与栈解析另批 | 首批实现/确定性回归/构建闭环，生产 eval 待验；见 §7 |
| HMC-04 | P1 | 启动阶段与阶段内业务热点 | 业务事件→精确区间→既有因果/采样；未配对保持未知，不用全量热点填指定窗口 | 待实现 |
| HMC-05 | P1 | hilog/kmsg 结构化日志与 trace 联查 | 先来源身份/时钟映射/误差/对齐见证，再提供时间邻近支持，不自动成因果边 | 待设计 |
| HMC-06 | P2 | pcap 网络流及协议指标 | 有界流标识、包时间/重传/方向/完整度；不能把同窗网络当链上 IO 等待 | 待设计 |
| HMC-07 | P2 | 视频会话/解码器/音画阶段 | 保留会话ID/输入条件，关联有依据时才连边；扩大诊断窗口只作背景 | 待设计 |
| HMC-08 | P1 | GPU 频点运行交集、runnable 深度、IO 分布统计 | 既有 IO/调度因果不变；精确区间交集/全窗权重/部分桶/分位/样本数先行 | 待实现，核心附录细分 |
| HMC-09 | P2 | PMU/BRBE/SPE 与 tag 指令统计 | 先消除 schema/单位冲突，补采样权重与核型前提；未知绝不补0，样本不换算耗时 | 待设计，参考部分失效 |
| HMC-10 | P1 | 跨 trace 多维对比 | 独立来源/窗口、设备与工作负载可比性账本，未匹配显式保留；差异不是因果结论 | 待设计 |
| HMC-11 | P2 | 批量/聚类/交付报告 | 可恢复 per-object receipt、未覆盖成员、有限并发；不复制参考 prose 裁决 | 待设计 |
| HMC-12 | P1 | 跨框架渲染业务目录 | 5 类管线作为软发现；生产图只能消费精确 frame/token/事务事实，不能照抄架构示意箭头 | 待实现 |
| HMC-13 | P2 | 芯片/频率/能耗画像 | 版本/来源/单位/适用设备与不确定性；核型近似推断不能作确定算力表 | 待设计 |
| HMC-14 | P2 | heap/低内存/进程退出语义聚合 | 原始表已保留；补 owner 生命周期、地址代次、栈解析、缺失/截断，再做分配/释放/候选泄漏 | 待设计 |
| HMC-15 | P2 | Sendable/并行化分析到写方案 | 仓库代码/数据依赖/副作用验证与正常 write approval/worktree/verify；无采样热度直接改写 | 待设计 |
| HMC-16 | P1 | 教学一致性与能力声明验收 | 每一教学调用都过真实 schema/执行入口；减少重复 prompt，单位单源，缺数据有明确出口 | 各批共同验收条件 |

优先序以数据可靠性/泛化覆盖/现有基础/实现风险四维决定：先 HMC-03，再 HMC-01/02 与 HMC-08；启动/渲染业务适配依赖精确身份；日志/网络/媒体与 PMU 先立跨源合同，不为追求条目覆盖造假支持。

## 5. 参考实现的反例与不移植项

1. `frame_ops._match_render_by_vsync` 容忍 ID 相差≤3，时间参数未使用；Flutter 有50ms近邻回退。它们可建议查证，不能证明同一帧。
2. `sched_ops.sweep_depth_summary` 对半开区间的桶末端计数有反例：`[0,10)` 在 `[0,20)`、10ns桶中产生两个非零桶；参考平均也不是全窗时间加权平均。不能复制为调度供给计算。
3. `io_priority.yaml` 部分 SQL 用同线程全窗 sched_slice 多对多 JOIN，一条 IO 可被重复计数；各指标优先级分类不一致。现有 Codrax 精确 IO 配对及 S/D 阻塞见证继续保留。
4. `cold_launch_ops` / `perf_calltree_ops` 可在请求窗不落采样范围时改查全量；后者还有固定线程名模糊回退。不能借此回答指定窗口，不能将 `sample_share×window` 变成实测函数耗时。
5. PMU 同名 `start_ms/end_ms` 在部分指标实际按 ns 使用；BRBE 扩展器与消费 SQL 列名不一致，SPE 表创建/读取不闭环。按真实能力而非 YAML 名称迁移。
6. `parallelism_analysis` 同时存在互斥耗时教学，`load_compare` 跨芯片继续/全跳过指导冲突；渲染定义内预绘的 SurfaceFlinger 链不能当 HarmonyOS 客户事实。详见 skills 附录。
7. 参考正文相似度/数值/关键词评判不移植。本项目只对 schema/来源/精确身份/实测范围作硬约束；嘈杂选择信号最多决定附加提示。
8. 参考 Present/Fence 以后一帧时间补闭合的修复不能成为观测到的完成时间。缺失结束点继续未知；资源存活时间也不成为线程执行时长。

## 6. 分批验收与状态

- 批0：B1716已推送 `93bf1a42d`，验证记录留在统一主账本 §123.1849；本审计不冒称重跑其全部测试。
- 批1：完成清点、逐项附录和反例审计；文档基线已提交推送 `7f20cdea0`。原始归档不修改。
- 批2：HMC-03资源元数据可见性已实现；旧库缺列、NULL、0、错误storage、超大整数、资源寿命≠执行、I/C保留等先红后绿。具体收据及尚未验收的范围见 §7。地址位型与栈解析未闭环时只留原始保真，不伪造解析能力。
- 后续每批只关已经实现且有收据的子项，剩余保持本账本；不以改 prompt 冒充数据能力。真实 eval 恢复时固定每批2个，先审计上下文/过程/答案，再决定是否修系统，不反复调教单个 case。

保护项：read mode 字节边界、typed-on-chain-only 根因、显式时间窗、自动补齐、Trace 因果投影、图关系证据、模型答案所有权、必选 root-causes 旁路、600/300/600秒等待默认及活跃流保护均不因参考仓对比而放宽。

## 7. HMC-03 / EXT-1 第一批实施收据

### 7.1 已实现的有界能力

参考 `config/indicators/memory/heap.yaml` 的源字段语义，在 Codrax 既有 [native-hook 导出器](../../internal/hitraceconv/streamerdb_export_native_hook.go) 上独立实现，不引入参考项目的第二执行内核或依赖：

- `heap_size` → 可选 `source_heap_size`：精确保留非负 SQLite INTEGER；不因 Free 自动取负，FD/THREAD/handle 等资源族不擅自标成 bytes。
- `callchain_id` → 可选 `source_callchain_id`：保留整型源键和 0/负哨兵，不冒称栈已解析、函数已执行或因果已证明。
- `end_ts` → `resource_end_ts_ns`：保留合法整数、NULL、0 的差异；NULL/0 不说明资源已释放，正 end 也不生成 B/E 执行区间。
- 缺少可选列时不发射该字段；坏可选单元格只记该字段的诊断，不吞掉合法瞬时事件/活跃资源计数。旧表的身份、生命周期、Running CPU 见证与同时间戳稳定次序规则不变。
- 复用现有 `event_search` 的显式窗口和原始事件查询。只在 [单源视图教学](../../internal/skill/trace_query_views.go) 解释字段前提，不增加新的必填 JSON 字段、重复工具、正文扫描硬门或模型答案代写。

仍未实现：地址位型的专门语义适配、native-hook 栈展开、分配统计、地址重用代次及泄漏候选判断。原始 SQLite 全表保真本来就保留这些数据；此批不是宣称补齐所有堆分析能力。

### 7.2 验证与失败留痕

本机日志在 `.codrax/tmp/20260917-hmc03-*.log`（不提交客户或运行日志）：

- 新增公共导出→索引→查询测试先行为 RED，再 GREEN；覆盖旧 schema、NULL/0、超过 2^53 的整数、MaxInt64 资源 end、坏存储类型、非内存资源单位、窗口外不命中及不进入根因 span。新增与同输入字节收据连续三次通过（2.264s）。
- `go test -race` 对上述元信息、同输入摘要、共享教学连续三次通过：hitraceconv 6.513s，skill 2.885s。此前原生资源/生命周期/保真相邻回归亦通过。
- 四包全测首次唯一失败是同输入输出摘要 pin：新增一个资源事件后缀增加 53 字节。先增加“去掉唯一新增后缀后，旧输出长度与完整 SHA-256 必须完全一致”的回归，再重钉摘要；事件总数 35、权威事件 18、保真/辅助事件 17 均不变，保真尾部逐字不变。原失败日志保留，不直接把摘要改绿。
- 同轮 skill 全包 1.262s、tracequery 全包 93.637s、tracediag 全包 7.283s 通过；hitraceconv 完整包摘要修正后复跑通过 106.537s。四包合计已通过，但首次四包命令本身 exit1 的事实保留。tool/agent 的教学、工具描述及窗口/权限相关定向回归通过（1.753s / 2.688s）。
- 独立复核复跑 same-input、元信息与既有 NativeHook B2 边界测试通过 1.331s，同输入两轮均实跑未 skip；复查 parser→instant→root-cause 路径未新增时长或因果凭证。
- `make` 构建通过；本轮未跑整个仓库所有包，未跑真实模型 eval，未修改参考仓或旧答案。资源事实到真实模型业务解释的生产验收仍待后续两个并行场景，不以确定性 fixture 代替。

因此本批只关闭“已合法导出的资源事件缺可查询元信息”子项，不代销 HMC-14 或其余工具/skills/指标缺口。
