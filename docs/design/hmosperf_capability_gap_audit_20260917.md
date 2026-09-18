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
- [默认二进制接入、11 类格式/容器与平台依赖](hmosperf_binary_input_comparison_20260917.md)（用户追加重点；CLI首批施工与验收见§13，其它入口仍开放）
- [18类差距的79项实施任务清单](hmosperf_implementation_tasks_20260917.md)（状态、依赖、验收、提交与全量条目反查；后续施工入口）

这里的“全量”指本地归档**声明的工具、指标、工作流逐项静态对照**，不是承诺所有客户数据形态已回放。每一项需分清四层：原始数据保留、语义查询、统计/工作流、因果证明。特别是 Codrax 已将 SQLite 全部非内部表逐单元格保留到 systrace 保真载体；缺少某表的业务查询适配器，不能写成转换物理丢数据。参考仓有名字或示例也不等于其实现正确、合同一致或运行闭环。

总体判断：值得吸收的是领域指标细节、可发现性、分对象证据包、跨次可比性和业务化解释；不另建一套执行内核，不照搬模糊近邻关联、全量回退、样本占比换耗时或正文扫描裁决。

## 2. 对外工具逐项映射

参考定位均为 `server.py` 中同名函数；实现进一步委托 `core/`、`utils/`。Codrax 对应的是能力入口，不要求同名工具。

| 参考工具 | 参考实现细节 | Codrax 现有入口 | 差异与处置 |
|---|---|---|---|
| `get_hitrace_path` | 文件/目录扫描，最多五层，在首个有结果深度停止 | `internal/types/runtime_artifact_selection.go`、运行时附件/转换入口 | 已有显式工件选择；可补多工件清单体验，不复制“目录第一份 trace”或首层命中即完整的假设 |
| `get_log_path` | hilog/kmsg 目录与 gz/txt 选择 | log attachment、log triage | 尚无同等 hilog/kmsg 语义归并及 trace 时钟对齐；列 HMC-05 |
| `get_pcap_path` | pcap 发现、同目录/父目录回退、多来源拒绝猜测 | 通用工件/文件读取 | 没有专用 pcap 流/协议指标适配器；HMC-06，不能静默扩搜父目录 |
| `convert_hitrace_to_sqlite` | TS→DB；mtime 缓存；附加日志/媒体表；传入 DB 时可能修改它 | `cmd/trace_convert.go`、`internal/hitraceconv`、`internal/traceinput` | 显式转换/归档/文本保真已有且输入保护更严；CLI文件附件默认准备已`482bfa856`交付，完整材料/有限预览分离；REPL默认转换及未准备查询路径仍开放（HMC-17）。附加领域表语义解释另列，不引入可变输入或 mtime 充当内容证明 |
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
| HMC-02 | P1 | 按窗口/对象组织有界证据包 | 复用 artifact/blob 与 typed 对象 ID；独立错误、覆盖/截断、缺对象，不按数组位置猜同一对象 | span_locate 子查询覆盖丢失已24d82316f修复；因果诊断IO事实交接及独立窗口去重末版全仓验收通过，第四批交付（§11）；其余待设计 |
| HMC-03 | P1 | native_hook 资源事实易查询性 | 全表原始保真已覆盖；I 语义出口补本次资源量/来源栈键/nullable 资源 end，不铸执行时长/泄漏/根因；地址与栈解析另批 | 首批实现/确定性回归/构建闭环；生产数据交付正确、模型解释FAIL（§10）；同源教学857920fa7已推，修后live未回放 |
| HMC-04 | P1 | 启动阶段与阶段内业务热点 | 业务事件→精确区间→既有因果/采样；未配对保持未知，不用全量热点填指定窗口 | 普通业务片段事实交接末版全仓验收通过，第四批交付（§11）；命名启动阶段/业务窗口自主选择仍未闭环 |
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
| HMC-16 | P1 | 教学一致性与能力声明验收 | 每一教学调用都过真实 schema/执行入口；减少重复 prompt，单位单源，缺数据有明确出口 | IO陈腐教学已24d82316f修复；预检计量/残留文本出口冲突、资源多阶段同源教学已实现验收（§10.1），修后live未回放 |
| HMC-17 | P1 | 原始二进制默认自动接入 | 内容精确认型→共享转换准备→query-ready 派生产物；CLI/REPL/显式查询路径一致，原始来源与完整查询载体并存，不把截断预览当完整源 | 17.1–17.3已`482bfa856`推送，CLI公开路径及全仓验收通过（§13）；REPL默认转换/未准备typed path/平台格式矩阵/DB/stdin继续独立推进 |
| HMC-18 | P1 | 入口/多工具组合/格式细节的 eval 覆盖 | 真二进制入口与已转换文本分开；业务问题不硬编码 view 顺序；正反身份/时钟/窗口/背景反例，公共确定性 fixture + 两例并行生产人审 | 首对2例各1次已完成，机评/人审均FAIL（§10）；覆盖扩展待续，不宣称默认二进制已支持 |

优先序以数据可靠性/泛化覆盖/现有基础/实现风险四维决定：HMC-03 首批已收口；用户新增默认二进制接入要求后，HMC-17 升下一批首位，HMC-18 贯穿验证，再推进 HMC-01/02 与 HMC-08。启动/渲染业务适配依赖精确身份；日志/网络/媒体与 PMU 先立跨源合同，不为追求条目覆盖造假支持。

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

## 8. 用户追加的四个重点与真实能力边界

用户要求不止比较 API 数量，还要覆盖默认二进制输入、灵活组合、更多解析细节及具体场景 eval。HMC-03 已以 `055bd749d` 提交推送后，追加以下审计和施工范围：

1. **默认原始二进制输入**：当前 `trace convert` 的默认引擎为 auto，但 `--htrace`、REPL `/htrace path` 和 `trace_query source=path` 仍按文本入口拒绝二进制；不是模型波动。参考 skill 的 discover→convert→query 做法值得吸收，但其 `query_metrics` 自身也是收 SQLite 路径，不能误报成任意格式直接查询。本项目需一个有来源收据的共享准备服务，而非三处各加转换分支或删除文本安全检查。已有输入代次、取消回滚、归档成员选择、全表保真、时钟/CPU/线程因果权限都保留。
2. **工具灵活组合**：Codrax 已有 21 个共享视图、recipe 及联合证据包，不缺第二个 workflow 执行器。差异主要是领域能力发现、输入前提/单位、按对象持久化和组合结果完整度（HMC-01/02/16）。参考 ad-hoc 的“遇调度词只能选某包”、空检索就宣布无数据、截断即断言趋势可信等规则不移植；先通过实际业务问题验证模型能否完成定位→目标选择→同窗链/资源查询→有来源解释，不以固定调用顺序当唯一正确路径。
3. **更多格式细节**：分别盘点容器解包、物理字段保留、语义解析、统计和因果五层。缺专门聚合不代表原文丢失；格式新字段需独立原始见证、单位、NULL/0/无列、版本与身份前提。HMC-03 已交付这一方式的第一批，其他 109 指标对应格式差异按核心/扩展附录继续收口。
4. **具体 eval 场景**：现有 `harmony/hitrace_binary_converted_manual.case` 与 `trace_query_converted_inode_io_pressure.case` 只输入已转换文本；不得用它们证明默认原始二进制接入。新增资源元信息、多工具业务响应链两例，问题不提示内部枚举或硬编码查询序列；先对真实 parser/query 做确定性验收，再恰好两个并行各一次，分别记录机器结果、上下文/过程与人工答案审计。原始二进制入口的 RED/验收另随 HMC-17 实施，不能用文本冒名替代。

## 9. 组合调用与教学的一致性修复（第二批）

### 9.1 已确认的本项目问题

- **HMC-16a / P1**：`trace_query` Description 仍教 `block_io_by_inode` 按 nearest block/storage latency 关联，但引擎已有 `TestIOCrossLayerRelationRejectsNearestThreadTimeJoin` 明确拒绝同线程时间邻近配对。这是系统教学陈腐，不是模型波动。现只替换该唯一子句，明确 producer-supplied inode/entry identity、缺跨层身份只报独立背景；对应 Description golden 同步演进，其他描述字节与 Parameters schema 不变。
- **HMC-02a / P2**：`recipe_name=span_locate` 原先直接取 `EventSearch` 的行列表，丢掉子查询的 matched total、范围、截断与身份 caveat。两条同名 instant、`limit=1` 即可复现“仅一行但无截断说明”；单独调用 event_search 则有完整 accounting。现单项/组合均走 `publishIndexedEventSearch`，传播现有字段而非再加一套 JSON 合同，完整枚举/取消/按行窗口优先级同源。
- 同源修复还消除一个边界误报：匹配数恰等于 limit 时，旧通用兜底仍发 unknown-total compaction。现在有完整枚举收据就按精确匹配数判断，只有真正缺枚举收据的旧路径保留保守兜底；不放过实际截断。
- 独立复核发现并同批修正：唯一 `SpanName` 派生出的真实搜索窗曾仍标成全文件覆盖。现按生效后的数值边界计算范围，行窗口仍优先；空匹配也不能把未扫描范围说成已覆盖。公共测试先红（direct/recipe 各有非空、空集两形），再绿；显式单端与行窗正例同时保留。
- recipe 中途取消的完整展示行继续可用，但枚举未完成时保留“总量未知”的截断收据，不铸完整总量。取消测试最初仅5000行，达不到既有64Ki扫描采样间隔，因此测试自身未触及边界而失败；改为70000行真实索引后触发并通过，未降低取消采样规则或移除断言。

参考审计同时发现 ad-hoc/IO skills 存在 `float`、`"user_specified" | "full_trace"` 等非法 JSON 示例，输入声明与步骤透传的 process/TID、start_ms/start_time 命名不一致，以及 p99 输出层级/阈值分歧。它们是参考实现风险，不复制进 Codrax，更不据此添加用户/模型正文扫描硬门。

### 9.2 新 eval 与边界

| 案例 | 验证目标 | 不能据此声称 |
|---|---|---|
| [资源元信息](../../eval/cases/trace_query_native_resource_metadata.case) | 精确窗口内 3 次操作；超大整数、NULL/0/负哨兵、本次资源量与累计量；不造函数执行/泄漏 | 使用合成已导出文本，不能证明原始二进制默认接入 |
| [业务操作与 IO 依赖链](../../eval/cases/trace_query_business_marker_io_chain.case) | 自行定位 OpenDocument 50ms 窗；35ms 请求驻留、31ms S 态响应等待、唤醒后 1ms 调度分开；47ms 后台 IO 不成为主因；保留 LoadDocumentIndex 业务线索 | 验证现有 Trace 工具的多视图组合，不等同跨工具源码/日志/设备分析全覆盖 |

两例均不指定必须调用的 view 顺序或工具数量；仅附加 fixture，oracle/README 留在 stub_repo 之外。`TestHmosperf*` 使用同一受版本控制的样本，经真实解析/查询确定事实；第一次自拟 completion→wake 间隔超出既有闭合条件，修正的是合成样本而非降低产品门槛。两例新增及 ERE 语法检查通过，fixture race×3 通过1.866s；之后真实模型执行结果见§10。

独立冻结前审计又纠正一处测试数据错误：公共 exporter 将 MmapEvent 映射为 MmapSize，而初版 fixture 误写第三条 HeapSize。修为 HeapSize 两条8192→4096、MmapSize独立4096；README和真实query断言同步，fixture两项×3复跑0.808s通过。此前回归收据仅证明初版样本自身一致，不拿它证明错误映射正确。两条判分正则也在任何live运行前补充了被误拒的等价正确表述：“包含关系、只计一次”和“不支持泄漏结论”。这是测试质量修正，未改变生产门槛、数值事实或后验重判答案。

第二批公共回归：六形 recipe 覆盖测试行为 RED→GREEN；含既有取消、span locate、同窗共享计算的 count3 通过42.469s；教学新 pin 行为 RED→GREEN，7项相邻教学/单位合同与2项精确 IO 关联正反例通过。加入派生范围与取消补充后最终定向 count3 通过2.144s，最终 race×3 tracequery 16.897s / tool 3.903s（含两例真实 fixture）。较早的五包全测通过：tracequery104.577s、tracediag7.052s、tool369.567s、skill3.258s、agent74.822s；该轮在最后两处补充前编译，不冒充末版全测。生产代码末版五包 `-count=1` 全部通过：tracequery102.093s、tracediag7.081s、tool357.115s、skill1.655s、agent70.119s；之后的测试夹具纠正另跑两项count3及race×3（1.860s）通过。`24d82316f`修复及`42159453f`测试纠正均已推送，后者清洁构建通过。生产两例从该版本启动，见§10，未完成前不签通过。

## 10. 第一对生产 eval 与上下文审计

冻结版本 `42159453f069`，批次 `hmos_gap_batch1_20260917`，2026-09-17 21:05:14 本地开始；恰好两例并行、各一次，runner上限1200s/例，模型配置未改。资源186s、业务IO273s，均有完整答案，无超时；**机器FAIL 2/2、人工FAIL 2/2**。机器汇总与人工审计分别记录在 [批次汇总](../../eval/parallel_selected_summary_hmos_gap_batch1_20260917.md) 和[人工审计](../../eval/parallel_selected_summary_hmos_gap_batch1_20260917_manual_audit.md)。原r1088仍未运行，不混用编号。

运行中的独立上下文审计已确认：

- **HMC-16b / P1（已实现，收据见§10.1）**：实际 perf_triager 提示的 Reasoning Hygiene 要求数值直接取上游测量、不能自己数/算；同一提示 Workflow 却要求 `(last_timestamp-first_timestamp)*1000`、B/E差值、阈值负判断。该stage只有emit工具，无确定性计量工具。属于软教学自冲突，不冒称复现“同一声明硬门必带且必拒”。既有隔离已把模型预提取数值/summary/evidence从后续事实权威中排除，但不能以隔离有效为由保留无效模型负担。现预阶段保留原始端点/行锚/业务标签，直接源测量可复制，派生数值交确定性trace_query，未知走现有observations省略数值出口；不改全局Reasoning Hygiene放开计算。
- **HMC-16c / P2（已实现，修后live未回放）**：资源字段语义只教给explorer，perf_triage和finalizer实际提示均未收到；模型把end=0解释已释放、MaxInt64解释确定sentinel，并混合HeapSize/MmapSize。前两者不能从整数本身证明。资源字段单位/未知值/原始键/瞬时与寿命边界现为同源软教学，预阶段、query使用侧及有资源行的最终inventory共同引用；不扫描或改写模型答案。
- **上下文成本观察 / P2（待进一步量化）**：业务trace只有约2.3KiB，classifier实际上下文约55,411 tokens，系统正文约118,764字符。此轮未证明这些教学均冗余，也不据单样本裁剪安全合同；后续按阶段能力与条件化供给审计重复/无关块，不能用简单关键词开关减少上下文。

最终人审确认：资源原整数没有丢失，finalizer实际四份inventory保留各自pattern/事件类型/query_scope_id及NativeHook3、HeapSize2、MmapSize1；也已明确禁止混加不同查询总数。故不能把“6次操作”归因于系统丢失filter，它是模型把操作与观测混为同一对象；补同源语义，但不新增计数正文硬门。资源答案还把3ms用户窗写成2.5ms，原始边界交付正确。业务例保留Trace因果投影，却用0.998–1.052s查询窗（54ms/52ms已归账）取代1.000–1.050s业务窗（50ms），35ms请求驻留、LoadDocumentIndex线索未回答，并把IO完成闭合关系误作一概纯背景。

进一步对照实际查询结果，修正初步“未调用已有视图”的归因：window_stats已查到两个业务片段、35ms请求及31ms完成闭合等待。普通业务片段被仅发布优化分类的语义片段通道跳过；已发布IO行未进入本次最终成文10/114条观察。两处公共生产路径均已复现，归入HMC-02/04证据交接缺口，第四批实施见§11；不因前阶段已有advisory隔离就否认下游事实丢失。修复分开普通业务事实、IO两种计量和链上根因资格，不解除模型聚合文本的旧隔离来代替typed事实交付。运行快照不随教学修复改变；原始日志/答案/判分不覆盖、不追加第三例追绿。

### 10.1 第三批教学修复与验证

本批已以 `857920fa7` 提交推送。

新增共享 `TraceResourceObservationContract`，最终成文只在已验证查询库存的完整原始行经现有parser解析为NativeHook瞬时资源事件或HeapSize/MmapSize计数器时追加一次软说明。问题/答案关键词、截断原行、伪名称及错误typed事件类型不触发；不改库存、query_scope_id、覆盖、原始整数、Trace投影或加冕。

预阶段不再被要求手算B/E、全trace时长、阈值或计数；缺必要毫秒值时使用已有observations-only合法出口。独立复核又发现旧教学允许residue-only而提交器会拒绝，现改为携原文和行锚的unparsed observation加residue，不编造业务事件/时长。stalls/janks起点必填说明与真实schema同步。既有拒绝门和全局计算权限不放松，后续自动补齐仍从typed query获得测量。

验证：实际拼装提示先红后绿；公开emit接口覆盖业务B/E、S态与唤醒、IO issue/complete、无法解析原文及residue-only拒绝正反例。资源公共tool→ledger→最终提示四个查询身份和6/3/2/1行各自保持，ledger及Trace投影字节不变；非资源/截断负控通过。skill与相关agent/types测试count3通过，新增context/tool回归race×3通过。第三批主体冻结后skill/context/agent整包通过0.736s/1.505s/61.520s（明确跳过尚未修复的两个新业务/IO RED测试，未跳过旧测试）；其后residue教学补充再跑skill/context整包0.739s/1.326s、全部EmitPerfTrace×3为0.993s通过。tool完整包通过361.745s；其所属早期联合命令因已修教学术语检查退出1，最终分包补验通过，不冒称该旧命令全绿。原生产两例FAIL继续保留，不宣称修后模型答案已通过。

## 11. 生产发现的证据交接修复（第四批，确定性验收通过）

本批以 `a784bc439` 提交推送，对应任务 HMC-02.2 / HMC-04.1；修复后live答案质量尚未验收。

- **HMC-04a / P1：普通业务片段未进入工作关系事实通道。** 四次真实 `TraceQuery.Execute` 复现生产调用；先确认 `window_stats.TraceSpans` 已有 OpenDocument 50ms、LoadDocumentIndex 40ms，再调用工作关系合同编译，结果为 nil。根因是发布器仅发布有优化语义分类的片段，而合同只从因果投影的 SemanticSpans 读取。普通业务名称没有语义优化分类是正确边界，不应因此从可答事实中消失。现已补独立普通业务事实通道；只能证明片段及其测量范围，不自动获得链上资格、可消除量或主因席位。
- **HMC-02b / P1：因果诊断紧凑上下文遗漏已有IO请求计量。** 新公共复现同时带真实 typed PerfTrace 请求载体并调用完整 `BuildInitialInstruction`；省略载体时会落在非紧凑路径，测试会通过，不能据此否认生产缺口。完整真实车道已先红：35ms请求计量和backup-900身份缺失。现有IO专项事实桥只服务有限事实/有限效果问题，不覆盖此次因果诊断。现已增加已查询typed IO事实的有界交接，保请求驻留、完成闭合线程等待两种口径及背景身份，不改变根因选举。回归使用同一行/来源断言，不靠提示中巧合出现31/47数字签绿。

该批不新增远程模型重跑；先补确定性数据流并保留原生产FAIL。与第三批纯教学修复分别提交，避免把“提示已改”冒充“数据已贯通”。

### 11.1 实现边界与冷审纠正

- 普通业务片段从既有配对结果发布名称、线程、物理行锚、窗口内区间与实测耗时；保留完整原始区间及其耗时作为另一口径。不是新解析器，不依赖预定义业务名称，也不把区间当CPU执行或可消除量。窄窗公共查询验证10ms窗口内测量与原始50ms同时保留。
- 可见业务事实与工作关系可选项共用同一来源/窗口去重器和16条展示上限，19条输入有截断披露，未显示的ID不成为必须选择的隐藏项。是否要求关系说明仍由已有typed请求决定；没有该子问题也能获得事实，不新增必填字段。普通业务仅有“关系未证”资格，旧语义优化凭证和Trace因果投影不变。
- 因果诊断IO展示复用现有10条稳定选择预算与覆盖行预留；只消费同窗已发布的确定性IO/存储观察。保留35ms请求驻留与31ms提交线程闭合等待、精确来源、对象及查询范围；47ms后台请求保留背景身份，不能因更长而替代链上等待。有限事实/有限效果问题仍走原通道，不重复加段。
- 独立冷审发现首版复用物理请求去重键，会把同一请求在两个明确时间窗的见证压成一行；公共查询A窗、B窗、重复A窗、宽窗四次先红。现因果比较键加入精确查询窗、选定窗、目标/行范围、时钟和闭合计量，保留A/B各一行、同A重复只一行、宽窗不混入。有限事实旧键保持不变。展示同时说明完整物理请求/闭合等待可跨出查询窗，不冒称窗口内总量，也不把双窗见证算作两个请求。
- 教学/展示只使用结构化数据和源记录，不扫描用户问题或模型答案原文作硬门，不系统改写答案。实际最终提示构建前后ledger及Trace投影不变；block_rq/block_bio、无唤醒/错唤醒线程/无阻塞状态等负控不新增闭合资格。
- 首轮全仓结构门拦下本批业务读取器直接比较`Producer`的实现，确实会漏掉合法`trace_query:run2`。追加公共合同编译反例后行为也先红，再改用已有`RuntimeObservationProducerIsDeterministicQuery`单源；旧结构门/拒绝未知或模型来源的负控不放松。完整窗口内业务行的教学负控直接通过，没有把该检查误报为新发现。

### 11.2 验证收据

公共路径红→绿和首轮定向count3已通过：types1.168s、tool2.438s、agent5.206s；含旧有限事实/选窗相邻合同。首轮新增回归race×3通过3.199s/4.531s/11.880s。跨窗冷审原RED保留在`.codrax/tmp/20260917-hmc-io-scope-red.log`（exit1），不是后验删断言。

首轮全仓无skip命令exit1，唯一失败为上述来源单源结构门，其它85个有测试包通过；原日志保留。`trace_query:run2`公共反例另独立RED（0.908s），随后改回共享识别器。最终冻结版三包count3通过：types2.245s、tool1.289s、agent3.353s；race×3通过：2.830s/6.177s/15.201s。最终无skip的`go test ./...`整条命令exit0：86个有测试包通过（70个缓存命中），13个无测试包；包括agent85.131s、tool380.158s、types47.733s、tracequery109.011s及tracediag17.512s。构建通过。日志前缀为`.codrax/tmp/20260917-hmc-handoff-final-`；不因最终绿而掩盖首轮失败。

后续仍以HMC-17默认二进制入口为下一实现批次。此次没有补默认转换、启动阶段目录、业务窗自动发现、完整IO统计或模型解释质量，不代销其余账本项。

## 12. 子任务拆解与防遗漏索引

按用户追加要求，18类差距细分为79项稳定编号任务，列明依赖、可验证的退出条件、状态和交付提交，见[实施任务清单](hmosperf_implementation_tasks_20260917.md)。当前11项已实现并推送，68项仍开放，包含持续验收而非全是故障。工具/指标/skill/管线不会因其中一个子项通过而整体销账。

对照机器清单及本报告工具表逐名核验：109/109指标、24/24 skills、5/5实际管线、24/24不同工具名均有任务归属，零漏项、零重复名称；79个任务ID唯一，18个父类齐全。24个不同工具名由13个MCP工具与11个非MCP工具构成，executor仍为22，不将重叠工具重复计缺陷。HMC-17.1–17.3已交付；下一小批先消除§14排序教学漂移，再恢复REPL和path入口独立验收。

## 13. 默认二进制文件附件（第五批，HMC-17.1–17.3）

2026-09-18，基线`da4a33d5f`与远程一致、工作树干净后施工。本批先收尾共享准备/传输/CLI接入，不混入新的领域指标；最终验收通过，已以`482bfa856`提交推送。

### 13.1 参考细节与本项目实现

参考`server.py::convert_hitrace_to_sqlite`先选择文件/TS，再转换为SQLite供`query_metrics`消费；它按后缀接受输入，以相邻DB的mtime复用，外部转换设300秒。吸收的是“原件准备一次、后续分析复用统一材料”的工作流，不移植后缀即格式、相邻可写目录、mtime即身份或把转换超时当模型超时。

- 新增共享`internal/traceinput`准备服务：持有完整原件，精确内容探针路由既有auto转换器，查询就绪只认现有systrace/perftrace收据，库存-only不假成功；未知二进制不按`.sys`误认，已有文本不重复转换。SQLite仍明确要求独立安全适配。
- 模型预览、原件、完整查询路径、转换收据四者分离。原件和所有派生/成员代次固定，二进制保原件摘要与完整转换结果。限定预览大小只影响提示容量，末尾事件仍在完整查询输入；不把预览落成`attached_trace.txt`来冒充完整文件。
- `--htrace/--atrace path`默认准备后再发布附件/开始分析；失败或取消不发布半成品。准备期间协调已有进程信号处理，Ctrl+C由转换事务先回滚自己的目录，不新增墙钟超时。单捕获、双别名互斥和stdin独占不变。
- 载体贯通上下文、工具、子agent与CLI启动的REPL。查询前后再次验证全部代次；相同原件/完整产物只在有效收据下归并来源，独立文件不混入。陈腐预览/同名替换拒绝，不回退旧blob；独立显式path查询不受无关陈腐附件阻挡。取消仍返回typed取消状态。
- 恢复权限不写进JSON。截断预览/转换产物/bundle导出与会话引用升级schema2并要求重新附加，旧版本也会拒绝，而非忽略新字段恢复半份Trace；完整普通文本的自包含快照保留原schema1保存/恢复能力。

### 13.2 同批冷审纠正

1. direct perf显式输出目录时，bundle曾仍写在原件旁。已改为从选定输出路径推导bundle位置；省略输出路径时既有显式convert行为不变，原始来源字段不重写。独立双目录测试保原件字节且无源旁产物。
2. 原件与转换路径可能被全局来源清单当成两份Trace。现在同一受验证准备载体产生精确别名映射，预检/请求引用/附加/预分诊各载体同源消费；同名异目录、错误预览及陈腐代次均不归并。
3. 准备收据校验的取消曾可被包装成文件不可用。修为共享typed取消结果，覆盖冷/暖缓存及取消/到期，不丢取消语义，也不在已取消调用中发布旧测量。
4. 真实官方SIMPLEPERF没有raw-fallback专用披露，通用提示却建议查调度/唤醒链。实际Prepare→prompt公共回归先红；现从完整已校验bundle的采样就绪与缺systrace材料得出纯采样软提示，保样本权重/未知CPU和原始身份。混合bundle仍保留调度车道；不以提示标志授权/拒绝查询。
5. 首版对所有准备文件均要求重新附加，会误伤完整文本快照恢复。收口为准备器明确铸出的自包含完整文本标志，仅影响保存/导出兼容；截断/派生/多成员不获得该标志。JSON不能重铸原进程的查询权限。
6. 准备失败清理最初另写了`Lstat→RemoveAll`，身份检查和删除之间存在目录替换窗口。已改为复用转换器既有的持有目录权限：正常失败只清本次目录，目录/祖先替换和ABA重试不删替代物；成功释放句柄保留产物。释放失败进入终止态，不允许失去身份后再按路径回滚。故障注入只模拟句柄释放错误边界，不冒称遇到真实操作系统关闭失败。
7. 普通附件替换首版只撤销runner的旧收据、未同步正文/来源，绕过普通dispatch的`/read-runs resume`等入口可能读到旧预览。现统一替换函数立即按正文→来源→收据发布；公开加载/导入/清除后恢复分析验到新三件套，失败加载/导入保旧三件套，完整文本快照恢复同样覆盖。
8. CLI取消分支曾把共享服务返回的复合错误压成单一取消，丢掉同次回滚失败。现保留错误组合；公开入口用两个独立错误身份断言同时保留取消与清理失败，不隐瞒遗留产物，也不增加越权路径清理。

### 13.3 验证与剩余范围

已有确定性回归使用真实格式二进制字节（合成RMQ页与SIMPLEPERF protobuf，不是converter stub替代格式验收）：CLI实际加载→上下文投影→公开`TraceQuery.Execute`查到预览外尾部事件；采样-only保80个样本、未知CPU、不产生调度/帧因果。失败、取消、SIGINT子进程回滚、源改变、输出/receipt改变、旧blob冲突、来源别名、REPL种子传递、保存/导入边界均独立覆盖。

保护性公开路径回归另通过新附件准备→上下文投影→`window_stats/root_cause_rank`：明确1.000–1.050s窗口保留35ms请求驻留、31ms完成闭合S态等待和两个业务片段，worker-200保持链上而更长后台IO的backup-900不入链。测试从观察的完整PayloadRef取JSON，不误把面向人的RawRef文本当JSON。CLI尾部测试的预览上限由512改为1024字节，以容纳macOS更长的规范化目录及来源头；仍断言尾部不在预览中而完整查询有12个事件，生产上限未放宽。

首轮全仓命令exit1：新增attachment类型未加入既有renderer census的真实类型检查依赖，两个census测试失败，已补真实依赖并保原门/自红测试；另一个旧Python探针fallback测试未进入第二候选，首轮耗时3.53s。后者单独整组10次通过（20.606s），与每候选3秒探针受负载影响相符，但日志不足以证明超时根因；不放宽生产阈值或删除断言。首轮日志`/tmp/hmos-17-fulltest-20260918.log`保留，不把最终绿冒充首轮全绿。

最终冻结版`go test -p 2 ./...`整条命令exit0，无`-run`/`-skip`：87个有测试包通过（52个缓存命中），13个无测试包；包括agent73.330s、tool366.904s、hitraceconv128.739s、tracequery95.294s、repl49.307s、cmd11.911s、tracediag5.766s。日志`/tmp/hmos-17-final-fulltest-20260918.log`；`-p 2`只控制包执行并发，不缩小测试范围。`make`构建通过，日志`/tmp/hmos-17-final-build-20260918.log`。

八包准备/载体/目录/CLI/REPL定向race×3通过（`/tmp/hmos-17-final-race-20260918.log`）；最后生命周期修正后cmd/repl再次race×3通过5.936s/2.365s（`final-lifetime-race`），traceinput完整包race×3通过1.946s（`final-service-race`）。真实RMQ另覆盖中文文件名及只读源父目录，运行材料写入独立运行目录，原件字节不变、源目录无旁生文件；不将macOS权限回归冒充Windows运行验证。

本批未运行真实模型eval，不更改第一对生产FAIL结论。HMC-17.4/17.5、格式/平台矩阵、现存DB及二进制stdin仍开放；HMC-18.3的CLI/REPL真实模型端到端验收另列。链上根因选择、Trace显式窗投影/自动补齐、root-causes旁路及600/300/600秒活跃流保护均未修改。

## 14. 目录对照发现的排序教学漂移（HMC-01.2 / HMC-16.4，P1待复现修复）

2026-09-18在第五批最终全仓验收期间只读发现：`skill.TraceQueryViewTeachings`的`root_cause_rank`行仍教按same-chain cumulative_impact_ms排序及co-primary；`explorerEvaluator.buildExplicitRuntimeTracePathStartInstruction`另有同形直写。公开工具Description/Parameters却已应用closed matrix，要求按effective_impact_ms严格唯一首位。explorer的compose只拼接，`buildInitialMessages`→`AppendDynamicInstruction`将该动态提示原样追加，没有工具侧的后置替换；旧测试甚至pin着累计排序短语。

这是系统控制的教学源不一致，不能归为模型波动；永久“实际模型消息”红→绿回归和修复安排在第五批提交后的小批，不混入已冻结验收。方案必须修正共享词源及直写消费者，不增加一轮字符串补丁掩盖旧源；累计占用仍是重要业务/优化线索，但不能替代可消除量的排序口径。共享矩阵的`frame_flow`行也需明确时序关系与已证因果的区别，不能因为名字含flow就让模型把相邻区间当因果边。参数目录全部同源化仍是更大任务，本缺陷闭合不整体销HMC-01.2/16.4。

参数对照另记待验证项：工具映射保留`include_window_stats:false`，引擎`normalizeQuery`对`wakeup_chain`却重设为true。需要先核实该参数的公开承诺、默认值与内部必需计算是否不同，再做公开调用反例；不在教学批顺带改归一化器。该项仍归HMC-01.2的参数一致性，不以静态布尔赋值差异直接宣布用户行为故障。
