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
| `convert_hitrace_to_sqlite` | TS→DB；mtime 缓存；附加日志/媒体表；传入 DB 时可能修改它 | `cmd/trace_convert.go`、`internal/hitraceconv`、`internal/traceinput` | 显式转换/归档/文本保真已有且输入保护更严；CLI、REPL文件附件及typed命名查询路径默认准备已17.1–17.5交付，完整材料/有限预览分离；17.6格式/平台矩阵、现存DB与二进制stdin仍开放。附加领域表语义解释另列，不引入可变输入或 mtime 充当内容证明 |
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
| HMC-17 | P1 | 原始二进制默认自动接入 | 内容精确认型→共享转换准备→query-ready 派生产物；CLI/REPL/显式查询路径一致，原始来源与完整查询载体并存，不把截断预览当完整源 | 17.1–17.5已分批推送，CLI/REPL/typed命名路径及全仓验收见§13/19/20；17.6格式首片见§21，格式/权限/平台矩阵、DB及stdin继续独立推进 |
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

按用户追加要求，18类差距细分为79项稳定编号任务，列明依赖、可验证的退出条件、状态和交付提交，见[实施任务清单](hmosperf_implementation_tasks_20260917.md)。截至2026-09-19的§19批次，12项实现已交付，67项仍开放，包含持续验收而非全是故障。工具/指标/skill/管线不会因其中一个子项通过而整体销账。

对照机器清单及本报告工具表逐名核验：109/109指标、24/24 skills、5/5实际管线、24/24不同工具名均有任务归属，零漏项、零重复名称；79个任务ID唯一，18个父类齐全。24个不同工具名由13个MCP工具与11个非MCP工具构成，executor仍为22，不将重叠工具重复计缺陷。HMC-17.1–17.3已交付；§14排序/参数子缺陷已收尾，§15保留修后模型回放的失败，§16先收REPL输入所有权，再恢复默认准备和path入口独立验收。

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

## 14. 目录对照发现的排序教学漂移（第六批，HMC-01.2 / HMC-16.4子缺陷，P1）

本批已以`af2e2548d`提交推送，实际模型回放不随代码验收自动销账。

2026-09-18在第五批最终全仓验收期间只读发现：`skill.TraceQueryViewTeachings`的`root_cause_rank`行仍教按same-chain cumulative_impact_ms排序及co-primary；`explorerEvaluator.buildExplicitRuntimeTracePathStartInstruction`另有同形直写。公开工具Description/Parameters却已应用closed matrix，要求按effective_impact_ms严格唯一首位。explorer的compose只拼接，`buildInitialMessages`→`AppendDynamicInstruction`将该动态提示原样追加，没有工具侧的后置替换；旧测试甚至pin着累计排序短语。

第五批提交推送后，永久回归通过真实`NewExplorerAgent.Execute`和`NewFinalizerAgent.Execute`截取首次adapter请求（本地测试适配器，无远程模型），分别检查system、动态user以及实际提供的工具Description/Parameters。explorer两种typed scope都先红；finalizer必须加入typed `PerfObservation.Kind=root_cause_rank`且不带`TraceEvidenceAuthority`才能触发旧legacy提示，仅有Frames不足以覆盖。两组红收据为`/tmp/hmc01-trace-teaching-red-20260918.log`（1.109s）及`/tmp/hmc01-trace-teaching-red-full-20260918.log`（0.983s），均为行为断言失败，不是编译红。

确认漏消费者还包括默认explore skill的自身running参赛句、finalizer的layering与handoff两条独立提示；实际工具Description也缺少Parameters已带的frame-flow未证边界。这是系统控制的教学源不一致，不能归为模型波动。方案直接修正共享词源及所有已确认直写消费者，不增加一轮输出字符串补丁；不重复整段长闭矩阵。先保typed链/邻近通道，再按已发布effective量及已有tier解释，不能仅凭独立adjacent通道也可能出现的`rank=1`冠主因。累计占用、状态分解、重复窗、fragment count/max/p95与next-step仍是有效业务/优化线索；零计价不抹掉真实占用，也不宣称闭矩阵以外永无有限累计回退。

共享矩阵的`frame_flow`行同时明确时序关系与已证因果的区别：当前相邻完整片段给出`temporal_sequence`及`sorted_span_adjacency`，时间间隔不是已证通信延迟，阶段名不是线程角色凭证；帧因果仍需明确typed connector。链上规则和图表能力不改。参数目录全部同源化仍是更大任务，本缺陷闭合不整体销HMC-01.2/16.4。

本批没有增加输出正文替换或硬门。共享短排序合同在每个实际模型消息面恰好一次；长闭矩阵仍只在工具面。第一次相邻整包检查拦下共享视图行遗漏原有`semantic span-work`能力词，已补回“正有效量且链上”的语义优化候选提示，原保护测试未放宽。末版skill/agent整包通过0.871s/72.431s；三包定向race×3通过skill2.049s、agent2.430s、tool2.423s；工具旧字面pin换为完整共享合同与邻近通道边界后count3/race×3通过1.157s/2.485s。原邻包FAIL保留`/tmp/hmc01-trace-teaching-neighbor-full-20260918.log`，最终绿为`neighbor-final`及`race-final`同前缀日志。

工具首次整包exit1（358.714s，`/tmp/hmos-teaching-tool-full-20260918.log`）保留三个失败：源码census在并行编辑时读到测试文件中间态的非法转义、上述旧字面pin、以及真正遗漏的Description字节快照更新。停止源码编辑后，前两项与IO/业务保护定向count3通过2.285s（`/tmp/hmos-teaching-tool-neighbor-corrected-20260918.log`）。Description按原UPDATE RITUAL核对：仅新增frame-flow短合同与替换既有闭矩阵排序句两处，30887→31616字节；其余字节不变，旧门保留，演进记录明确说明中段插入的调度波动风险。正常非UPDATE的golden/schema/closed-matrix/replace-arms/census count3通过2.909s；最终冻结版tool整包无筛选通过362.779s（`/tmp/hmos-teaching-tool-full-final-20260918.log`）。本批确定性验收完成。

引擎保护另通过：有效量/优先级/排名域与板身份定向count3为1.121s；frame-flow/显式窗定向count3为0.503s；tracediag完整包5.259s。`make`通过。无真实模型eval，不声称live dispatch等价；h2/h3匹配基线A/B、r229与异构帧例仍在HMC-18验收队列，按每批2例执行，不能以教学单元测试替代。

### 14.1 参数一致性续批：显式关闭窗口统计被默认值覆盖

已以`9a5c95467`提交推送；修后模型回放单独记录，不回填原失败记录。

2026-09-18公开`TraceQuery.Execute`已确认反例（`/tmp/hmos-params-stats-probe-20260918.log`）：同一完整5.000–5.007s调度夹具，参数省略/false/true三次都返回`WindowStats`，三次均非memo命中且均保留唤醒链。公开schema说明此参数控制附带窗口统计、默认true，工具映射也保留false，但引擎`normalizeQuery`将false重设为true。不是内部计算必需而仅在模型前隐藏：最终JSON亦包含显式要求关闭的整份统计。

本项归HMC-01.2参数一致性，在教学批推送后独立施工。对照参考`core/query_engine.py::build_query`，可取的是按键存在性合并默认值、保留显式false；不搬其base_params最后覆盖所有调用参数的规则，也不另建SQL内核。

永久公开测试先红（`/tmp/hmc01-window-stats-public-red-final-20260918.log`，1.368s）：普通/缓存/别名及单窗、多窗子查询均忽略false。早期自动窗测试夹具先误把worker上的marker配target PID，后又误认为大文件窗口默认会裁剪关系；已分别按真实目标marker和“完整索引优先、超预算才关系裁剪”的生产合同修正，原日志保留，不把夹具错误算成生产gap。

新增值拷贝方法`Query.WithWindowStats(bool)`和私有presence标志：省略继续默认true，显式false不会被normalize覆盖；不新增模型参数/公开JSON字段，也不改变rank board指纹。工具工厂仍在调用Run前写明有效默认值，避免发布身份依赖Run内部副本；只控制wakeup_chain附带统计。默认自动补齐、root_cause_rank/frame_root_cause_bundle/jank recipe等所需统计继续计算并发布。

初验已通过（tracequery0.600s、tool1.292s）：省略/true/false和冷热memo交错、causal_impact别名/字符串布尔、统计/业务/语义/频率附加项、完整及关系裁剪索引、单/多自动窗、归一化幂等、Query副本与取消方法组合、链字节恒等、非空榜单与板身份保护。仅显式关闭的统计附加项撤下，不回收已有链证据。定向race×3通过tracequery2.278s、tool4.697s（`/tmp/hmc01-window-stats-race-20260918.log`），包含显式窗35/31ms IO、普通业务片段与帧自动补齐保护；构建通过。

冻结生产代码后，全仓无skip的`go test -p 2 ./...`整条命令exit0，87个有测试包通过（49个缓存命中）、13个无测试包，包括tool360.067s、tracequery95.073s、agent71.639s、hitraceconv121.914s、repl49.627s、tracediag5.657s。收据`/tmp/hmc01-window-stats-full-20260918.log`；等待期间测试驱动暂未收尾，保留了非破坏性系统采样，最后自然退出，不将等待过程误报为测试失败。

冷审再补独立发布身份pin：固定完全相同的Result及payload/raw引用，直接消费Run之前的工具工厂Query，验证省略参数与显式true身份相同、false不同；防止Run内部副本掩盖工厂默认值漂移。全仓命令在这条纯增量测试加入之前完成；增量测试与全部公开参数测试另count3/race×3通过1.508s/3.908s（`/tmp/hmc01-window-stats-publication-{,race-}final-20260918.log`），工具Description字节golden另count3通过0.797s。本批确定性验收完成，不整体销HMC-01.2；下一对live只验修后行为，不冒称已完成匹配基线A/B。

## 15. 修后两例生产回放与上下文复核（HMC-18.5）

以已推送`9a5c95467`构建运行h2 D状态/调用点与h3 IO多口径两例，严格2并行×1、每例1200秒上限。原机器结果0/2 PASS、2 FAIL；逐份读取日志、答案、完整查询payload及原始调度/IO行后，人工也均记fail，而非因字面regex不匹配就把实质错误签过。详见`eval/parallel_selected_summary_hmc_window_stats_20260918{,_manual_audit}.md`，含文件名和SHA256；原结果目录/日志留本地，新增named-results ignore规则防止机器产物与评测问题混入源码，未删除收据。

两例都是有限事实题，完整根因投影0符合合同，旁路schema2空roots且原因为`trace_root_cause_contract_not_active`也是正确输出。h2/h3时长149/258秒，最大上下文40%/50%；无成文硬拒和repair轮，h2一次facet补丁、h3无补丁。h3实际三次探索/17迭代/三次历史裁剪，汇总中的explorer dispatch计数0不能解释为没有探索。均无repo_map/源码读取，analyzer各尝试一次未提供的grep。

| 子发现 | 证据 / 分级 | 泛化处置与归属 |
|---|---|---|
| HMC-E2-1 | P1，系统可重复：无结构摘要附件前言仍教模型自行推导热点/卡顿，实际analyzer消息另一处却说没有查询或原始读取工具。两例均有该冲突；不能证明它造成所有答案错误 | 修附件单一教学源：字面观察/导航与确定性测量分开，按实际阶段工具能力引导；HMC-01.3/16.4。不得额外增加输出硬门或重试 |
| HMC-E2-2 | P1，系统词面有误导：共享五态词源`TraceStateNonIODStateWord`把未进入已标记IO桶的D称为“非 IO D-state”，查询摘要有同形。缺少标记不证明排除了IO。h2正文又从调用点推出GPU/HAL机理、从零标记推出纯非IO；完整提示中原本已有反对这种推断的边界 | 先审共享状态量/候选量生产者及既裁已证非IO席的区别，修未证桶的中性名称；不全仓字符串替换，不把真实已证席降格。HMC-01.2/16.4；模型的GPU/均匀分布主张另留18.2/18.5审计，不声称已证波动 |
| HMC-E2-3 | P1，h3核心IO值保住，但2个没有闭合唤醒证明的请求仍标“完成唤醒方”；只显示全局8条里的6个目标请求被写成共6条；层级配对事件极值被说成多个采样窗口 | 请求完成、目标唤醒证明、完整census、显示子集、层级聚合分别教学和展示。HMC-02.2后续表达验收/08/16.4；不能复制本例线程名或IO类型作专用门 |
| HMC-E2-4 | P2，h3正文泄漏`request_residence_caliber`等内部字段和authority标题 | 由公共事实词源供给业务语言；保机器字段和来源，不对答案原文做黑名单替换/硬拒。HMC-01/16.4/18 |

h2已保11段D/36.757ms与12条原因记录/Σ39.157ms两种口径，但这不能抵消机理过度主张。h3已保S态IO闭合等待4次/至少4.384ms、1.347ms请求历时与1.337ms线程阻塞、隐藏190个全局请求合计41.329 request·ms；不存在“必须D才算IO”的回退。题内全局背景未加冕主因。本对不是匹配基线A/B，也未现场覆盖显式false；参数真假由§14.1确定性公开测试验证。不改原oracle、不追第三例求绿，新增发现留在原79任务中而不虚增交付数。

## 16. REPL默认准备之前的输入所有权与取消（HMC-17.4第一片）

参考`server.py::get_hitrace_path/convert_hitrace_to_sqlite`提供文件选择→转换的组合入口，但其固定300秒外部进程和路径缓存不解决本项目REPL的长期stdin所有权。此处只补本项目安全接入所需基础，不复制参考的“目录取首个”或相邻DB写入，也不改模型等待默认值。

公开`New/Loop/io.Pipe`先得到3个独立RED：预读取消藏在提示scanner、取消后同一次read的尾部丢失、旧运行监听在stop后吞下一条命令。红收据`/tmp/hmc17-script-input-public-red-20260918.log`保留；粘贴与EOF负控原本通过，不把它们算新故障。旧实现另有先快照后stop竞态、1MiB运行期上限与配置不一致、首条排队通知压住溢出披露，以及非EOF读取错误被当成功退出。

实现为一个终身`scriptInputOwner`：prompt/capture/runtime分别借用唯一消费权，底层只有一个scanner。预读一行有界，交接时按消费权分配，stop与队列转移同锁；迟到/重复stop不能作用于新借用者。取消回调在交接屏障内完成，取消之后的行仍由下一消费者读取，运行本身清理完成前不派发下一问题。命令形文本在`/log`粘贴中仍是原始数据。旧生产第二scanner删除，原命令单测改用测试适配器消费同一实现。

普通排队保留32条上限，累计字节上限为`max(8MiB, 配置单行上限)`，与单行容量分开披露；溢出仍继续读取取消命令，按精确丢弃数告知。非EOF读错向Loop调用者返回，不打印成功Goodbye。退出只撤销交付权，不关闭调用方的Reader；通用阻塞Read无法强行中断，至多该唯一reader等待调用方给输入/EOF，未声称零停留goroutine或可立即把裸reader转交外部。

同时收掉真实Run初始化空窗：脚本运行包装器先从orchestrator预留下一次Run专属取消句柄，再启动运行输入。句柄在Run之前可取消、迟到调用只触及旧token；重复预留/活动Run拒绝再预留，放弃预留有明确release，普通非预留Run仍取新token。公开真实`Orchestrator.Run`取消测试不经过LLM，并与9个完整Loop输入测试分开，不再靠fake首轮锁存器冒称真实初始化安全。TTY与direct-LLM操作未改；本地文件准备尚未接入，不把这里当17.4全部完成。

本片初版编译错误（info方法参数）已纠正，未当行为RED。定向count3通过repl5.363s/orchestrator1.232s；含TTY相邻保护的race×3通过44.626s/2.252s。首次两包整包repl50.904s通过，orchestrator被热文件行数门拦下（8246>8242）；未抬门限或删注释，整组取消检查helper原样迁入已有`cancel.go`，原文件收紧为8203行。同批完整验收收据待追加。17.4的Prepare事务、TTY取消、附件失败后的待确认问题恢复、真实RMQ双入口和无LLM/用量变化仍开放；父任务交付数仍11/79。

合入前冷审再补“提前取消必须消费本轮临时展示/阶段字段”公开反例：旧位置在重置前返回，会把取消请求的展示指令遗留给下一轮，RED为`/tmp/hmc17-cancel-metadata-red-20260918.log`（1.212s）。现把提前退出移到已有逐轮字段重置之后，仍早于附件准入/模型/工作树操作；普通非预留Run的context完成语义保持原样。末版定向race×3通过repl6.370s/orchestrator3.473s，orchestrator完整包18.789s通过（`/tmp/hmc17-script-input-cold-review-{race,package}-20260918.log`）。全仓命令启动于这条冷审修正之前，其收据与增量验证分列，不能把它冒称最后每一行的冻结回放。

输入所有权主体提交`f48e8d119`。上述全仓命令最终exit0：87个测试包（71个缓存）、13个无测试包，tool379.813s、agent70.652s、repl51.089s、orchestrator16.462s、tracequery96.957s、hitraceconv117.617s；日志`/tmp/hmc17-script-input-full-final-20260918.log`。本收据不包含后续附件教学与下述定时器增量，分别验收。

同类生命周期冷审另发现写模式墙钟timer闭包持有可复用orchestrator：`Stop`无法撤回已经获准执行的回调，迟到回调可能取消下一次Run。改为捕获本轮token和当轮秒数，保持typed写期限来源，时长与读模式不变；不是修改600/300/600秒模型网络等待。确定性模拟旧回调在新Run开始后执行，验证只取消旧token并保600秒原值，另pin真实timer绑定；未伪称有先红收据。完整orchestrator通过18.704s，取消/写期限相邻race×3通过2.732s（`/tmp/hmc17-deadline-binding-{package,race}-20260918.log`）。热文件门随净减两行收紧至8201，不增加余量。

## 17. 附件观察与测量教学分离（HMC-E2-1，HMC-01.3/16.4子缺陷）

沿参考工具先查询再统计的职责划分，修本项目`attachedTracePreamble`共享源：原始片段只提供字面字段、单位和导航锚，不能仅凭有限预览推导总体量、热点排序、因果链或不存在性；测量需对完整材料、明确窗口和覆盖范围使用确定性查询。当前阶段没提供查询工具则留给后续有能力的阶段，不要求analyzer调用不存在的工具。预分诊仍产结构摘要，但不再要求凭原始预览铸造热点/卡顿结论。纯采样专用提示、已结构化摘要的模型候选边界、explorer查询导航均保留。

公开BuildPromptContext阶段矩阵和实际NewAnalyzerAgent.Execute首次模型请求均先RED（`/tmp/hmc-attachment-teaching-red-20260918.log`，context0.914s、agent1.090s）；不是仅测试私有短句。修后定向count3通过0.895s/1.631s，race×3通过2.128s/2.325s；context/agent完整包通过0.545s/65.709s（同前缀`green/race/packages`日志）。工具prepared-material、显式窗自动补齐、帧补齐、参数真假、Description/schema与因果来源相邻count3通过1.981s（`/tmp/hmc-attachment-teaching-tool-neighbor-20260918.log`）。未改工具schema/Description，无需重钉其字节快照。

本片关闭E2-1的已确认教学冲突，不等于已实测模型不再误调工具；h2/h3原机器与人工FAIL保持。E2-2未标记D桶名称、E2-3 IO完成/唤醒/总体展示及E2-4术语仍开放，01.3/16.4父项不整体销账。没有新原文扫描、输出替换、硬门、重试或系统根因代写；既有链上根因、明确时间窗及自动补齐不变。

E2-2后续只读核实（未施工）：`query.go`的`offCPUDStateVerdictForQuery`仅在`io=true`时进入iowait桶，其他分支（含未标记/歧义）进入dstate桶；`markDStateCoverage`另记每段显式标记，`DStateAllNonIOProvenGroup`才检查整组一致性。通用`DStateTop`摘要和五态词源没有这个前提。后续应统一“D状态（不含调度器标记的IO等待）”这类分区词，不改精确已证席的选举/收益，不把缺标记写成IO不存在；需同时覆盖工具摘要、typed事实、提示定义及答案对账四面，不能只改h2答案或全仓替换non-IO字符串。

§16主体`f48e8d119`、写期限绑定`bbd4de906`及本节`5db77d8f7`均已推送main，远端复核0/0；构建通过（`/tmp/hmc-teaching-input-build-20260918.log`）。这是已交付的三个小批，不代表17.4全部完工。

## 18. 完整材料的提交/撤销交接（HMC-17.1生命周期增强 / 17.4第二片）

再次只读核对参考`server.py:547 convert_hitrace_to_sqlite`：转换完成后直接返回邻接DB路径，异常靠局部cleanup函数收尾；没有“准备完成但尚未发布给交互会话”的持有状态。复用其准备→分析组合思想，但本项目不能在取消后以原件或派生路径直接删除，也不能释放目录句柄后再尝试夺回清理权限。此接缝是REPL接入前置增强，不伪称参考仓已经解决。

新增`traceinput.Begin`返回尚未发布的`Preparation`，只能`Commit/Discard`，没有可提前拿到查询凭证的公开getter。准备阶段最后的源身份、context和源文件关闭全部成功后，才交接既有受管理目录权限；文件读取/转换/完整材料生产仍是原有单一实现。`Commit`再次检查原件、收据和所有派生/成员代次，释放目录权限后才交出材料；原准备ctx或提交ctx取消、源/输出替换均不能发布。`Discard`只使用持有权限，文本原件不在删除域；提交后撤销不删已发布文件，失败清理冻结为终态，ABA恢复路径不能重获权限。提交/撤销用同一锁，至多一个终态。

原有`Prepare`公共入口现在直接使用Begin→Commit，CLI已经消费同一状态机；需要在转换后先做交互决策的调用者可使用两阶段API。调用方仍必须在自己的操作取消/发布边界串行提交，提交点之后的取消不追撤已经发布的附件。尚未接上REPL默认路径，不能把新API计为17.4交付，也不新增独立差距编号。

新增公共Begin/Commit真RMQ验收：提交后40个完整事件和尾部pid139可查询、原件字节不变；准备后两种ctx取消、源/派生改写、普通文本撤销、重复终态、并发提交/撤销和目录替换/ABA负控。首次新增能力回归通过，不冒称先红。共享服务整包race×3通过2.634s；公共Prepare切换后四包定向race×3通过traceinput2.907s/cmd6.105s/repl2.856s/tool4.987s，覆盖CLI真实二进制、SIGINT回滚、CLI种子REPL与显式窗IO/业务查询（`/tmp/hmc17-preparation-public-race-20260918.log`）。cmd/repl/traceinput整包分别11.924s/53.126s/0.979s通过（`/tmp/hmc17-preparation-packages-20260918.log`）；末版全仓收据待追加。

冷审收去测试转换器注入的旧即时释放分支：公开Begin、公开Prepare和注入converter的回归全部经过同一begin→提交/撤销实现，不维护第二套成功清理路径。增量四包race×3通过3.034s/7.200s/3.070s/5.253s，含真实CLI失败/取消发布负控；traceinput/cmd完整包0.951s/11.810s通过（`/tmp/hmc17-preparation-unified-{race,packages}-20260918.log`）。构建通过（`unified-build`同前缀）。全仓命令启动于这条内部路径同源化之前，范围与增量收据分列，不冒称一个冻结命令涵盖后来的源代码。

完整全仓命令`go test -p 2 ./...`最终exit0，无run/skip：87个测试包（64个缓存）、13个无测试包；agent68.643s、tool363.308s、hitraceconv110.597s、orchestrator16.184s、tracequery97.132s、tracediag5.684s（`/tmp/hmc17-preparation-final-full-20260918.log`）。后段go测试驱动短时尚未返回，尝试只读采样时进程已自然退出；未终止测试，不把过程等待算成失败或网络超时。增量同源化以上节四包race及整包收据补足，未运行新live eval。

本片代码与任务/架构说明以`7e13b0fdd`提交并推送main。17.4仍为部分实施：共享事务已经可供REPL接线，但本地非LLM取消目标、TTY命令/粘贴区分、失败后排队问题确认、真二进制双别名公共Loop验收均未销账。79个任务编号保持稳定，11项已交付/68项开放，未因本片基础增强虚增完成数。

## 19. REPL默认二进制准备与失败恢复（HMC-17.4，2026-09-19）

本轮开始工作树干净，`8f912dd8e`与重新获取的远端main相同，没有待提交旧工作混入。再次逐行核对参考`server.py:133 get_hitrace_path`与`:547 convert_hitrace_to_sqlite`：直接原件接入和工具组合是要补的能力；后缀准入、目录首文件选择、相邻DB写入/mtime缓存、固定300秒转换时限不复制。本批仅关闭REPL明确文件入口；目录发现、现存SQLite和typed path查询协调仍按原任务各自开放。

公开`New/Loop`真实RMQ回归先得到RED：`/htrace`与`/atrace`仍把二进制拒为非文本；完整/截断文本及IO业务fixture到Run没有完整材料收据。接线后五组公共入口回归通过，binary预览外尾部通过真实`attached_trace`工具可查询；复用既有IO业务fixture验证明确窗口31ms完成闭环IO根因和业务span，不把PID900背景IO晋升链上。原件字节不变，派生物仅在受管runtime根，schema1/2恢复限制保持。

实现分三条边界：

1. 新`tracePreparationOperation`只持本地ctx和一次性提交权；前置核对runner完整材料能力，Begin成功后取消与Commit+tuple发布同锁，失败Discard只用持有目录权限，重复/迟到回调不碰下一Run。
2. 共用终身脚本reader和TTY owner，准备期不连接旧pipeline steering、不调用LLM或用量重置。TTY显式命令回调只处理键入`/cancel`，不扫描paste数据；原有流水线TTY逻辑保持。WSL runtime备用根从cmd原逻辑提取共享helper，平台/宿主事实测试无更改用户HOME。
3. 失败/取消锁存明确附件状态，保留旧tuple但暂存随后输入；成功重试、`/htrace keep`或`/htrace clear`才恢复，`show/help/exit/cancel`可立即使用。包括未进入运行期队列、错误已打印后才到达的pipe问题；直接Run恢复不能绕过。暂存32条/8MiB，溢出明确披露，paste命令形在恢复后仍为数据。

确定性验收：公共二进制/文本/窗口组GREEN（1.158s），合并实际信号组count3通过2.158s；预读`/cancel`＋前后两问题公开Loop count10通过1.714s，未调用旧runner取消/steering且原件和旧材料不变。失败→保持/清空/重试、迟到pipe、直接Run拒绝和命令形paste的公共回归通过1.611s。旧测试按新能力调整：文本产生自己的新材料，不再断言nil；失败恢复须先keep，不再允许旧附件被隐式使用；预览截断与原始完整查询分开断言，不降安全门。

独立子进程真实SIGINT/SIGTERM测试持有真实Begin的RMQ未发布材料，用pipe握手暂停回滚，证明取消不提前关闭done、Discard完成后才退出；不是converter stub，也不冒称它等于无seam公共Loop的精确转换时序。相邻整包repl50.592s/cmd11.099s/traceinput2.863s通过（`/tmp/hmc17-repl-preparation-packages-20260919.log`）。冷审又发现单向done允许Loop在信号退出前抢跑，已补准备期专属双向退出barrier：请求终止与finish同锁，若终止先获准，finish在回滚/输入交接后通知signal owner，再等进程退出而不返回Loop。已发布材料不因晚到终止撤销，finish先完成则已离开该本地退出域。新增真实二次SIGINT退出130、SIGTERM退出143；测试移除人为select阻塞，finish若意外返回必须FAIL。信号count3和race通过1.376s/3.649s。

末版生命周期/输入相邻race×3通过repl54.457s、cmd3.650s、hitraceconv1.904s（`/tmp/hmc17-repl-preparation-race-20260919.log`）；该筛选在traceinput无匹配，不冒称覆盖，另补Begin/CLI准入/取消/真实二进制、末版信号和用量边界的race×3，traceinput2.145s/cmd5.739s/repl6.848s通过（`/tmp/hmc17-repl-preparation-final-increment-race-20260919.log`）。`make`构建通过（`/tmp/hmc17-repl-preparation-build-20260919.log`）。

本批没有新模型eval，§15原机器/人工FAIL保留。未改根因选择、图表语义、Trace投影/自动补齐、root-causes旁路、600/300/600秒默认或活跃流保护；没有新增用户/答案关键词硬门、系统代写答案或按照模型分数调整阈值。

全仓`go test -p 2 ./...`最终exit0：87个测试包（56个缓存）、13个无测试包，agent72.408s、hitraceconv123.328s、orchestrator15.600s、repl54.541s、tool360.541s、tracediag6.752s、traceinput0.761s、tracequery96.508s、types33.772s；收据`/tmp/hmc17-repl-preparation-full-20260919.log`。命令在生产接线/退出屏障冻结后启动；后加纯测试的信号加强和用量边界由上面的末版增量race单列补足，不混写覆盖时间。17.4本批关闭，累计12/79项实现交付、67项开放；17.5普通命名path与17.6格式/平台矩阵继续开放。

本批代码与文档已以`eb2ddd446`提交并推送main；随后才进入17.5，没有将REPL未提交工作与下一批混合。

## 20. 普通命名路径的同轮完整材料协调（HMC-17.5，2026-09-19）

参考`server.py::get_hitrace_path/convert_hitrace_to_sqlite`的选择→准备→查询组合入口，补本项目三处同源接线：typed探索前准入、公开`TraceQuery.Execute`、系统自动补齐。只改公开工具会留下探索前拒绝二进制以及补齐按原始二进制计算读取预算两个接缝。原底层BuildIndex/流式parser保持只读文本/bundle，不启动转换器；来源选择函数保持无转换副作用。目录首文件、后缀作为格式证明、mtime缓存、相邻DB写入和固定300秒时限仍不移植。

每次Run独享协调器，经BusContext→AgentContext→子agent共用一个不序列化句柄，cmd/REPL传稳定runtime锚而非临时WorkDir。成功材料才缓存，准备与并发等待共享；等待者取消不取消持有者，持有者取消先回滚，存活等待者随后可重试，其他失败不跨调用负缓存。原始路径/typed目标/窗口不被改写成sticky附件，原件不改；完整查询材料和有界预览继续分开。原件、派生物和引擎实际选中的完整源集合在命中和查询后验证，普通文本自动提升的有效sibling bundle及其成员也在同轮冻结域，不能读到另一代后继续发布旧/新混合证据。新context版本校验API复用原引擎选择，不改变其来源、时钟或缓存合同。

公开真RMQ测试发现确定性故障：`.sys`已经准备成功，但自动补齐沿扩展名枚举把它丢掉，返回`no_attached_trace`。原红收据保留在`/tmp/hmc17-named-path-public-green-20260919.log`（文件名中的green不是verdict）；修复仅让已发布材料收据识别整个typed路径载体，不增加用户/答案关键词判定。冷预算测试同时保证预算按更小的完整query-ready材料而非原件，保持明确时间窗且多视图只转换一次。wakeup-only二进制夹具没有睡眠前驱，故不能要求已证阻塞链边；其尾部由真实event_search验事件，链因果由独立IO/业务夹具验31ms闭合等待，PID900背景IO不能晋升根因。没有把缺链证据当实现缺陷或补造链。

公开六项验收覆盖二进制多视图/预览外尾部、原件不变/不覆盖附件、原件与派生换代撤回观察、明确缺路径不借其他源、原始/派生去重与独立采集隔离、冷预算与明确窗IO/业务自动补齐。末版race×3通过3.873s（`/tmp/hmc17-named-path-public-accepted-race-20260919.log`），冷补齐加强pin单独通过1.411s。Bus/context/REPL接线及typed准入相邻race×3通过，最后准入读后再次验证源集合及冲突别名pin的race×3为types2.985s/orchestrator2.610s（`/tmp/hmc175-admission-final-race.log`）。热文件门首次拦下新增接线，整组SetAttachedLog及完整godoc迁到专门小文件，门限8201→8191，没有抬上限或删说明避门。

冷审再得到canonical/符号链接双向RED：同一原件通过另一种精确路径访问会再次转换，可能混入同轮不同代次。协调器现以精确EvalSymlinks路径共享成功材料与并发准备，并记住已见路径首次所指对象；新别名不能绕过变代，旧别名改指或等待期间改指均拒绝。13组协调器回归race×3通过3.453s，traceinput整包1.198s通过。不同canonical的hardlink仍按独立输入处理，不加字节/文件名近似合并。

末端独立公开反例又复现：A、alias→A、B均已准备后，alias改指B；候选枚举提前把alias覆盖成B的queryPath，导致省略source的查询及冷自动补齐都绕过首次绑定，成功发布B的证据。保留实际所选路径直到准备检查，派生坐标只参与去重，两个反例RED→GREEN；对应正常去重/补齐阳性及负控race×3通过3.338s，收据`/tmp/hmc175-named-alias-{red,green,race}.log`。最终全仓收据待补。

工具Description与Parameters共享一条准备能力说明，退役“所有二进制必须手工转换”的过时教学；另校准同处既有来源说明：只有经验证bundle的成员可组合，裸邻接`.perftrace`不自动合并。此行为原已由引擎实行，本批只纠正文案而非修改来源选择。字节golden按演进记录显式更新，因果排序、计量和帧合同不改。未运行新live eval，§15 h2/h3机器与人工FAIL以及匹配基线A/B债仍保留；不能用上述确定性通过倒签模型答案。此时17.5尚在验收，最终交付收据见本节末；17.6格式/平台矩阵、17.7现存SQLite和17.8二进制流式输入仍开放。

首轮全仓`/tmp/hmc175-full-20260919.log`exit1，仅tool包的`TestTraceQueryDescriptionReplaceArmsAllFire`仍钉旧的无条件`.systrace+.perftrace`措辞。已同步为“validated sibling bundle / only admitted members / unbound sibling不自动合并”三个正pin，保留原退役词负pin与物理来源/时钟pin；不是删除漂移测试或降低来源门。其他86个测试包通过；此轮不是末版绿收据。

最后JSON-facing接缝冷审：模型侧按有效准备收据生成query路径的logical ID，但`traceQueryRuntimeArtifactSelectionView`手建AgentContext漏传协调句柄，工具侧重建为原路径ID，系统自己教出的ID被拒`trace_query_runtime_artifact_id_unknown`。从真实Agent视图经JSON roundtrip再Execute得到RED，补同一字段后回归；另要求模型/工具完整source view一致，原始/派生只占一个capture，两个独立capture仍为两个，不固化某个hash。随后本机APFS复现大小写别名变代绕过：原件成功准备→同文件内容更改→大写路径重新转换并发布新证据。已改为逐层实际目录entry身份：每次最多128项、取消可见、精确entry优先、大小写仅筛候选、最终SameFile证明；不全局小写macOS或Windows路径。Windows只规范drive letter，UNC变体保持保守，不声称已获跨网络同源证明。

最终20组Coordinator与4组新增模型视图/大小写公开测试race×3分别通过14.458s/7.856s（`/tmp/codrax-hmc17-logical-id.5mheN8/final-case-and-view-race.log`）；case-alias真RED及logical-ID真RED在同目录分留。普通可列举只读目录、不同名hardlink独立、search-only父目录明确权限错误在本机真实PASS，0.772s详细收据`platform-permission-pins.log`；当前APFS不能创建大小写相异的两个独立entry，对应两项明确Skip，不冒称已有case-sensitive卷运行证据。Windows/amd64仅CGO=0测试二进制交叉构建成功，未原生执行。已知新增适用边界：父目录可search但不可列举时协调器保守拒绝源身份确认，不更改权限，不回退仅词面身份、不称格式损坏；该更低权限路径的能力增强仍在17.6矩阵。

生产代码和测试冻结后的最终完整命令`go test -p 2 ./...`exit0：87个测试包（69个缓存）、13个无测试包，tool346.122s、orchestrator16.292s、repl48.586s、tracediag5.476s、traceinput2.519s、tracequery90.424s、types32.232s；收据`/tmp/hmc175-full-final-20260919.log`。未使用run/skip筛选，包内平台条件Skip仍按上一段保留。末版七包定向race×3均实际匹配并通过：tool16.595s、traceinput13.725s、orchestrator5.021s、context2.475s、repl2.932s、types4.155s、tracequery3.196s（`/tmp/hmc175-acceptance-final-race-20260919.log`）。构建通过（`/tmp/hmc175-acceptance-final-build-20260919.log`）；Linux/amd64的traceinput CGO=0交叉构建通过（`/tmp/hmc175-linux-build-20260919.log`），不冒称Linux或Windows原生验收。此前旧措辞pin的两条失败收据保留，不覆盖为绿；未追加模型eval。

本批代码与文档已以`ff4f62eef`提交推送main，远端复核0/0、工作树干净后更新交付清单。17.5关闭，累计13/79项实现已交付、66项开放；17.6按格式/权限/平台分别推进，不因本批来源协调已交付而提前销账。REPL17.4与命名路径17.5分两批交付，未合并掩盖各自验收边界。

## 21. 格式矩阵首片：gzip文本与独立输出位置（HMC-17.6，2026-09-19）

本轮开始`7a84edc36`工作树干净，与重新获取的远端main一致。参考`tests/test_log_fusion_fixture.py::_load_fixture`先将`.sys.gz`解压为原始`.sys`再调用服务，故这是字节运输而非新的事件生产者/时钟关系。真实样本当前为3,602,086字节压缩、54,122,686字节解压，参考测试68MB/5.7MB注释已经陈旧，不当能力依据。原始私人采集与派生内容不提交。

默认公开Prepare的合成合法gzip文本先确定性失败：未发现外部TS后走RMQ并报`invalid_magic 0x8b1f`（`/tmp/hmc176-gzip-default-functional-red.log`）。之前两次测试编写中的字段名编译错误只属测试施工，不当产品RED。实现新增独立`gzip_trace_text_v1`运输收据：单member、完整CRC/ISIZE及尾部检查、有界头/大小/解压比例、全量文本验证；字节、行号和时间戳不变，不以gzip时间元数据推导trace时钟、不伪造systrace生产者凭证。发布仍经过原私有目录/封存文件/精确无覆盖提交；来源与解压文件都绑定强代次，父级在全量哈希前即执行压缩体积上限。取消、源/输出换代、读写与清理错误不得保留二进制fallback资格。

默认文件准备接此运输层，保持完整查询与有界预览分离、压缩原件不变、不写原件目录、不取得自包含文本快照权。完整gzip中有精确已知二进制签名时，仅以typed格式结果保留旧转换器路径；坏CRC、嵌套gzip、SQLite、晚到NUL/坏UTF-8不借RMQ重试。公开query＋系统补齐首次通过1.187s，显式1.000..1.050窗口、链上31ms闭合IO及LoadDocumentIndex业务线索保留，PID900背景不晋升（`/tmp/hmc176-gzip-public-first-green.log`）。公开Prepare/提交/暖缓存/取消首轮通过1.014s（`/tmp/hmc176-gzip-default-first-green.log`）；末版收据待追加。

代表采集实际通过默认Prepare、完整字节SHA核对和现有流式解析：54,122,686字节、429,756事件、429,766行、1024字节预览，2.707s（`/tmp/hmc176-gzip-real-stream-green-20260919.log`）。最初用无界BuildIndex触发既有250000事件内存上限，收据`/tmp/hmc176-gzip-real-default-20260919.log`保留；未升阈值或关门，验收改用本就用于全采集扫描的StreamScan。这证明本机这份真实文本采集的运输/解析，不代签模型答案、所有事件语义或多平台覆盖。

格式矩阵另外亲验两类独立问题：①无systrace的OHOSPROF inventory/采样车道用空`Result.OutputPath`作为物理清单发布基址，退回原件旁写bundle；默认Prepare虽诚实拒绝inventory-only，已提交的清单却逃出受管准备目录，重试再报已存在。②同根导致位于运行目录的独立HIPERF成员不能生成合法bundle-relative路径。已区分“没有主systrace”与“指定输出目录”：3个调用方修物理发布基址，结果/清单的systrace字段仍保持空，不放松成员身份或时钟隔离。独立直接converter回归真RED→GREEN（1.199s），含只读原件父目录、显式输出目录、既有bundle不覆盖、retained-DB无行路径；原RED保留工具运行记录，未另存磁盘日志。公开矩阵7阳性/3阴性随后通过2.307s、race×3通过9.685s，不删失败车道换绿。

新确认开放范围：顶层gzip-PERFILE2与其解压后PERFILE2在同一默认入口不等价，前者仍落TS/RMQ，而OHOSPROF内部HIPERF gzip已有专用decoder；这是路由缺口，不能记模型波动或把失败pin成期望成功。显式`trace convert`尚未接文本运输helper，当前增强仅默认文件准备；二者后续需明确同源能力契约。17.6仍开放，累计13/79交付数不因一个格式首片而增加。未运行新live eval；原§15机器/人工FAIL及跨模式验收债保持。

生产冻结后的三包定向race×3全过：hitraceconv13.165s、traceinput3.615s、tool12.071s（`/tmp/hmc176-final-focused-race-20260919.log`），覆盖运输/提交/缓存/公开窗口IO补齐/格式矩阵及无systrace发布位置。代表采集末版重跑仍完全一致，测试自身2.77s、包4.315s（`/tmp/hmc176-real-final-20260919.log`）。原生构建通过（`/tmp/hmc176-final-build-20260919.log`）；traceinput测试二进制Linux/amd64和Windows/amd64的CGO=0交叉编译均通过（`/tmp/hmc176-{linux,windows}-cross-20260919.log`），不代表两个系统原生运行。

末版全仓`go test -p 2 ./...`exit0：87个测试包（61个缓存）、13个无测试包；cmd11.228s、hitraceconv114.280s、orchestrator14.996s、repl51.073s、tool344.194s、tracediag5.604s、traceinput2.755s、tracequery91.213s、types32.896s（`/tmp/hmc176-full-final-20260919.log`）。未加run/skip筛选；未将环境变量控制的实机用例或平台条件Skip计成默认测试已运行。工作树只包含本批16个文件，提交前重新fetch确认与远端0/0，代码/测试/文档一批收口。

本片以`01a378349`提交推送main；推送后工作树干净、HEAD/远端0/0。17.6仅首片交付，仍保留开放复选框；稳定账本实数核对13项已交付、66项开放、合计79，未因矩阵子例数量重复销账。后续按§21.1推进，生产eval另归18，不以本片确定性回归覆盖旧FAIL。

### 21.1 下一片实施前已核实边界（历史基线，后续见§22）

顶层gzip二进制根因不是某个PERFILE2字段缺失：当前运输层完整验证后对二进制返回typed结果并撤销解压代次，Prepare再把压缩原件交ConvertFile；direct-perf路由只接受裸PERFILE2/SIMPLEPERF，因而落TS/RMQ。该公开RED只有工具记录，没有另存文件日志，不能借用早先gzip文本失败日志代签。参考`core/hiperf_converter.py`仍是`.data`→TS，不能说参考已有可复制的gzip生产路由。

后续最小泛化方案是共用一次有界解压、持有内层输入视图，再由原语义provider分型；借鉴现有ZIP的外层来源/内层解析视图，而非递归ConvertFile后手改路径。通用运输收据独立于文本运输及既有`gzip_perf_data_v1`的Standalone专属凭证；不得给顶层gzip伪造Standalone，或把外层路径和内层字节数混成`perf_data`源。默认KeepTraceDB也应按内层语义路线决定，不再按“凡gzip即directPerf”推断。回归覆盖PERFILE2/SIMPLEPERF/RMQ/OHOSPROF及嵌入gzip-perf、坏CRC/源换代/取消、输出碰撞、sample-only时钟隔离、公开查询与自动补齐；不新增未知protobuf/递归容器能力。

search-only不可列举父目录另经只读复核：目录列举用于真实目录项身份，不能仅凭相同inode合并不同hardlink、全路径小写或退回词面比较。暂无已验证跨平台替代；Darwin单目录项原生属性仅是研究候选，需独立平台/文件系统能力及竞态验收。本轮保持明确权限失败，低于上述可复现路由缺口的实施优先级，未暗中放宽来源门。

## 22. 顶层gzip统一路由与显式文本转换（HMC-17.6第二片，2026-09-19）

起点`37fbfda80`干净且重新fetch与远端一致。独立基线快照`/tmp/codrax-hmc176b-red.BY18tm`使用同一公开测试，五个gzip二进制正例（PERFILE2、SIMPLEPERF、RMQ、OHOSPROF、OHOSPROF含嵌入gzip采样）均确定性失败于RMQ `invalid_magic 0x8b1f`，两种库存路线也误路由；真RED留于`/tmp/hmc176b-public-red-20260919.log`。早期并行接线时缺类型的编译失败不是产品RED，未冒充功能证据。

根修是解压运输与语义解析分层：`ConvertFile`先完整校验并只解压一次，冻结内层视图贯穿已有decoder/provider、发布与最终源验证；不递归调用ConvertFile、不重新打开私有路径。`PrepareFile`只为默认准备选择内层DB策略，显式转换的用户选项仍受原合同约束。新增独立`gzip_input_v1`外内字节/代次收据；生产发布前及query消费均验证格式闭集、摘要、上限与代次字段，该字段不改变capture_id、子产物摘要、物理来源集合或时钟授权。文本走已有独立运输收据及全量文本验证，CLI/REPL显式转换同样可用；显示“完整解压、事件尚未统计”，不声称转换出0事件或因果已证。

同类来源错误一并处理：ZIP及gzip的direct-perf不能再发出“外层路径＋内层字节数”的假raw artifact，也不暴露待清理私有路径。容器来源保留在顶层provenance，真实归一化采样仍需原校验收据；OHOSPROF内部真实raw child的Standalone和派生perftrace的PerfTransform保持原职能，二者不能互换。未知binary、SQLite、嵌套gzip/ZIP、多member、坏CRC与尾随数据仍拒绝；不放宽任意压缩/protobuf，不扫描用户或模型原文。

首轮公开矩阵修后全绿2.267s（`/tmp/hmc176b-public-green-20260919.log`），含暖复用/来源替换/库存拒绝及ZIP direct-perf对照；该矩阵是合成合法格式字节，不冒称客户实机全格式。集成第二轮traceinput/hitraceconv/tool通过0.739s/4.815s/4.374s（`/tmp/hmc176b-integration2-20260919.log`）。此前集成两条红分别是旧双层解压的stub调用次数断言，以及错误要求派生perftrace拥有Standalone；已按原权限合同纠正测试，不放宽结构门。schema/query新增与相邻race×3通过1.267s/1.858s。末版全仓、构建和提交收据见下文；HMC-17.6暂不整体销账，总数仍13交付/66开放。

参考再次核对`core/hiperf_converter.py:91`仍只收`.data`，`server.py:632`无gzip后缀入口，`tests/test_log_fusion_fixture.py:36–39`先自行解压。因此借鉴的是“统一准备再组合查询”，而非复制并不存在的参考gzip生产能力；本片不修改LLM教学/超时，不运行新live eval，原机器/人工FAIL不代销。

独立复核补充：默认准备现在精确要求gzip成功结果恰有一种运输收据；真实converter成功后再删/混收据的4条反例均撤销所有本轮材料，既有产物和原件不变，race×3通过1.929s。converter集成7组正反面包含SQL provider实际消费内层字节、默认保DB与显式不保DB、文本/采样参数冲突、取消及无覆盖，末版race×3通过6.755s。CLI/REPL显示及帮助同时说明gzip文本的字节运输边界；末版相关测试通过5.464s/1.403s（`/tmp/hmc176b-cli-repl-final-20260919.log`）。

两份参考真实采集只读专项均通过，内容不纳入仓：gzip文本仍为54,122,686完整字节、429,756事件、429,766行，3.197s（`/tmp/hmc176b-real-text-20260919.log`）。30,062,585字节PERFILE2在临时目录完整gzip后，与裸原件分别走真实Coordinator→公开`perf_stats`：均12,000样本、12,000事件、1 cohort、总权重43,037,682，热点及时间边界完全相同，末版12.306s（`/tmp/hmc176b-real-perf-20260919.log`）。参考原件字节/代次/权限/mtime及旁目录均不变；仅证明采样事实等价，未声称BRBE/SPE或调度因果新增支持。该专项由`CODRAX_TEST_GZIP_PERF_SOURCE`显式开启，默认Skip，不拿默认全仓绿代签私人样本。

冻结生产代码的定向五包race×3全过：hitraceconv13.193s、traceinput4.746s、tool27.592s、tracebundle2.081s、tracequery2.943s（`/tmp/hmc176b-focused-race-final-20260919.log`），覆盖旧文本运输及公开显式窗IO/业务/自动补齐保护；无关键词硬门、无新因果选举逻辑。Linux/amd64及Windows/amd64的traceinput测试二进制CGO=0交叉编译成功（`/tmp/hmc176b-{linux,windows}-cross-20260919.log`），只记编译，不记原生平台验收。

全仓`go test -p 2 ./...`exit0：87个测试包（54个缓存）、13个无测试包、零FAIL；hitraceconv135.570s、tool392.753s、tracequery104.873s、traceinput3.596s、agent73.732s、tracediag6.147s（`/tmp/hmc176b-full-20260919.log`），未加run/skip过滤。帮助文案最后修订另有cmd/repl末版整包通过13.165s/59.875s（`/tmp/hmc176b-ui-full-final-20260919.log`）；后加收据缺漏反例与可选实样测试分别以上述独立race/实样收据覆盖，不虚称它们在全仓启动前已冻结。原生make构建通过（`/tmp/hmc176b-build-20260919.log`），提交后再更新本地构建身份。提交前再次fetch main仍0/0，33个变动文件均为本批代码/测试/文档。

主体已`7a09856c2`提交推送main，干净构建身份更新通过（`/tmp/hmc176b-build-clean-20260919.log`）。收尾另补CLI/REPL解压进度的中英文可读标签，旧文本与新通用解压阶段均不直接显示内部阶段枚举；仅调整显示，不改机器进度信号/选路/超时。对应整包增量验证通过：cmd 11.545s、internal/repl 52.751s（`/tmp/hmc176b-progress-ui-full-20260919.log`）。稳定清单仍13/79交付、66开放；本片不因五种格式正例而重复销账。

### 22.1 下一批HMC-08.1只读前置（尚未实施）

参考`core/preprocess/io_ops.py:30/50`用线性插值计算IO延迟分位数，`config/indicators/io/io_latency.yaml:123`按请求开始时间选样。当前`block_pairing.go:655`已有Top8之前的完整精确配对census，缺的是完整总体分布，不应另造第二配对器或对截断展示算P99。`pairing_cohort.go:105`现有口径是与窗口相交的完整请求寿命，含carry-in/out；不可悄悄改为参考的start-in-window或窗口交集时长。`query.go:6659`既有分位数函数并非线性插值，新增分布须声明算法而不更改其他统计的既有口径。

后续验收从来源/family/dev/op分组的完整请求总体入手，空总体与真实0ms区分，11条请求及Top8之外变化必须影响分位数；缺端点/歧义/跨来源/跨层不能进入样本，RQ/BIO/file不混成双份总体。请求驻留分布只作观测；链上响应影响仍遵守`query.go:6766`的completion→issuer wake及真实S/D阻塞证明，大分位值、后台IO不晋升主因。本节只是源码审计/施工前置，未改代码、未跑新live、不标08.1已实现。

## 23. HMC-08.1首片：精确组内IO请求耗时分布（实现已推送，生产回放单记）

接续§22.1，参考只读核对`core/preprocess/io_ops.py:30–104`：全DataFrame合格latency列先统计，Top-N仅明细；采用`p*(n-1)`线性插值。`config/indicators/io/io_latency.yaml:123`的start-in-window取样没有照搬，保持本项目既有相交完整请求与行窗优先合同。也未复用/改变其它统计的离散`percentileFloat64`。

公开基线在`67365764d`隔离快照`/tmp/codrax-hmc081-red.ujKiTa`运行：同组PairedCount=11且Top8正常，但不存在分布字段，两条参数化反例确定性RED（`/tmp/hmc081-red-20260920.log`），不是编译失败。实现只在block与generic既有成功配对闭合分支保留私有float64样本，按原组一次汇总；不另建配对、不过滤坏值后伪装完整总体、不把FileIOByInode汇总当请求样本。可选typed分布的八个数值保留显式零，空总体nil；组展示仍Top8，遗漏组数/合格配对数独立披露，并列组稳定排序。

新增公开查询矩阵覆盖11样本且低尾改变不影响Top8、RQ/BIO/SCSI/F2FS/MMC、nil与真实0、未配/歧义/恢复、跨源/操作/设备隔离、carry-in/out、point/显式零窗/行优先、11组66配对与遗漏3组6配对；专项race×3通过1.531s（`/tmp/hmc081-public-race-20260920.log`）。独立渲染加入零值/固定点及口径中文映射、父级与嵌套schema演进见证，旧字段与因果资格不变。

实际工具交接又抓到第二个确定性缺口：把新分布附在长Summary尾部，机器JSON正确但Observation摘要截断后样本数/P99消失；仅引擎绿不足以宣布模型能用。已将八个测量值集中到短摘要，并把组身份、物理来源、查询窗、请求口径及遗漏覆盖放入前五条说明；不抬高默认10条/语义复核6条的预算。同提交者异设备/读写仍独立，block的首个线程只作代表，不冒称单线程总体；完整物理来源参与组身份摘要，显示路径被截断也不串组。EvidencePack重复发表面仅在层/事件族/完整行时边界唯一匹配且物理来源一致时共享同一测量说明，零匹配、歧义或跨源保留原事实，角色/置信度/Value/Unit不改。19个新增说明键全部仅展示，不新增因果解析权限。

工具公开调用与默认/语义复核实际上下文投影均验收，包括原业务fixture的显式宽/窄窗、35ms请求、31ms链上S态IO阻塞、后台47ms不晋升、自动补齐及LoadDocumentIndex线索。tool/types/skill专项race×3通过8.133s/3.687s/1.922s（`/tmp/hmc081-tool-handoff-race-final-20260920.log`）；后加同提交者三组上下文隔离专项race×3通过2.620s（`/tmp/hmc081-same-issuer-compact-race-20260920.log`）。tracediag整包5.285s及专项race×3 2.306s通过，组身份先于数值，真实0保留、未知口径不猜、不输出内部枚举，nil旧摘要字节不变。tracequery整包87.863s、引擎末版race×3 2.617s通过（`/tmp/hmc081-tracequery-full-20260920.log`、`/tmp/hmc081-engine-race-final-20260920.log`）。

共享view教学说明单位、样本范围、相交完整请求、不可跨层相加/不可平均P99及非因果权限。无原始问题/答案关键词硬门，原请求窗/补齐/链上IO与业务线索/600-300-600超时不改。新增自制合成eval覆盖RQ读11条、BIO读3条、RQ写3条及歧义/未完成/窗外负样本；只读确定性输出与独立expected.json一致（`/tmp/hmc081-eval-fixture-validation-20260920.log`），不冒称实机采集。生产回放按“新增能力数值正确性×既有链上/业务保护”的优先级选择新分布case及既有business_marker_io_chain，各跑一次、并发2；不改旧case的断言，也不以合成引擎绿代销旧live失败。

首轮全仓发现既有`TestCausalIOHandoffColdReviewKeepsDistinctExplicitWindows`回退：两个显式查询窗都已发表IO记录，但新增统计说明使EvidencePack汇总行也获得原有“具备注释”显示优先级，在10行因果IO展示预算内挤掉第二窗的请求。公开原测试×3确定性RED（`/tmp/hmc081-window-regression-red-20260920.log`）；另加20组汇总增长反例，旧选择结果为1条覆盖+9条汇总、0条请求（`/tmp/hmc081-aggregate-budget-red-20260920.log`）。这不是模型波动，不能放宽旧多窗断言。

根修仅限因果解释的展示选择：保留既有覆盖/层级预留与10行上限，余量先显示经原作用域去重的单请求/已证等待，再填汇总；既有目标偏好、显式窗过滤、重复查询去重、角色和因果权限不变，有限事实车道不变。原双窗/重复/窗外公开测试及新增长反例、RQ/BIO闭环正反面、S态和业务线索专项race×3通过10.277s（`/tmp/hmc081-causal-window-handoff-green-20260920.log`）。未用原文扫描或提高全局容量掩盖回退；末版全仓已重新启动，保留首轮FAIL收据。

冻结末版全仓`go test -p 2 ./...`exit0：87个测试包（77个缓存）、13个无测试包、零FAIL；agent70.071s、tool358.052s、types31.641s（`/tmp/hmc081-full-final-20260920.log`）。代码/测试未加run或skip过滤。初轮`/tmp/hmc081-full-20260920.log`唯一失败为上述多窗回退；其余86包通过，包含tracequery93.986s、tracediag6.133s、hitraceconv129.727s。修复后的末版不覆盖原失败日志。

主体`2887fb8ae`已提交推送main（25文件），对应干净原生构建通过（`/tmp/hmc081-build-clean-20260920.log`）。2并行×1回放于2026-09-20T02:57:02Z启动，固定该构建快照，批次外层上限1800秒/例；未更改产品600/300/600秒超时或活跃流策略。回放逐例收据见后续§23.2，不预先声明PASS。

本片只覆盖既有精确行组（generic仍含inode/PID），不声称全层跨线程上卷或全采集完整性；这些仍需合格原始样本与采集覆盖，不可二次聚合已发布分位数。任务总数保持13交付/66开放。

### 23.1 后续08.2/08.3只读前置

再次逐行检查参考`core/preprocess/io_ops.py:111–240`，不能直接移植其名字对应的语义：`compute_io_concurrency`统计与每个20ms桶相交的请求数，不是瞬时在途深度；同桶内先后发生的0..1ms、2..3ms两个请求，桶计数为2，但最大在途深度只有1。后续08.3应从同源同层合格起止构建半开区间sweep，独立披露峰值、时间加权均值及未闭合/歧义边界，原桶触达数可作为另一个指标而非换名冒用。`compute_io_size_distribution`以64KB阈值命名random/sequential，也只能借鉴尺寸分桶，不能据请求大小推断地址访问顺序。08.2必须分别声明请求/实际字节、发起/完成计数与窗口墙钟分母；本节仅施工前置，未据此宣称新增能力或销账。

### 23.2 生产两例回放：机器1/2、人工0/2通过

固定`2887fb8ae324`构建，2并行×1，分布485秒、业务272秒；无第三例追绿，无原答案/oracle改写。自动结果见[原始summary](../../eval/parallel_selected_summary_hmc081_io_distribution_20260920.md)，逐例过程/上下文/答案/图/旁路见[人工审计](../../eval/parallel_selected_summary_hmc081_io_distribution_20260920_manual_audit.md)。结果日志根`eval/results/hmc081_io_distribution_20260920`；文件在本机保留，不把私有运行原文纳入仓库。

| 追踪项 | 已核实证据 | 处置/剩余范围 |
|---|---|---|
| 23-R1 / HMC-02、18：并行丢证，P1 | 第二路`window_stats`已发表正确RQ读/写、BIO三组完整统计；别路完成取消该路后，ParseOutput未写快照，且非胜出分支整个被合并器跳过。finalizer没有这份查询。不是模型波动 | §23.3只保留完成的producer数据，保留取消/完成合同；确定性回归通过，修后live仍待验证 |
| 23-N1：成文丢分位数嫌疑已否证 | 真实公开`TraceQuery → TurnA → BuildInitialInstruction`旧generic/priority行已有三组全部八项指标、身份/窗/非阻塞边界；`p99=`与内部`io_request_p99_ms=`是等价显示，不是丢字段 | 撤回试验性重复提示段；仅加正向回归。不得把要求新键形的失败测试算系统红转绿 |
| 23-R2 / HMC-04.3、02.4：业务实例绑定，P1 | 业务回答有Trace投影、35ms请求/31ms S态等待/1ms调度/47ms后台；已查得正确业务span，但线程状态账户仍取0.999..1.055宽窗，自动补齐因`no_typed_target`跳过，root-rank查询0次 | 需通用业务实例→目标/窗口结构绑定；探索PID不能静默升级为用户目标。原有硬门不撤 |
| 23-R3 / HMC-16、18：业务量纲/结论，P1 | 1..1.050的50ms业务区间被说成1..1.051；把56ms查询窗的52ms状态账户套入业务窗；CPU执行叫请求耗时，字节叫扇区，以“等worker”症状替代链上瓶颈 | 模型已有正确span证据，不能改parser迎合错答；继续异构验收，系统只保留准确上下文，不代写主因 |
| 23-R4 / HMC-01.2、16.4：修补形式，P2 | 两例都把`facet_ids`当原子edit名；当前教学已明示精确`add_facet_id`或完整`replace_blocks`，业务例还有同块重复操作 | 未发现必带/必拒合同冲突；不自动改名、不开放任意edit、不加重试。可按本轮schema生成动作提示；实际wire schema日志尚不完整，不冒称抓包核验 |
| 23-R5 / HMC-18：探索误配对/统计故事，P1 | 模型把歧义同扇区请求猜FIFO；最终RQ17/BIO5、P99=2/8且丢写组。第一路重发时已从正确/部分正确值漂移，系统低增量完成又先结束该路 | 首先补R1证据交接；错误早收敛属于另一层，不能以保留工具结果冒称全部解决，也不能硬扫描正文修数 |

业务机器PASS只是烟测命中，人工FAIL；分布机器/人工均FAIL。两份`.root-causes.json`都生成，schema2空数组及`no_selectable_typed_on_chain_candidates`，不是格式/落盘错误。分布样本没有链证据，空旁路符合证据边界；业务例还需目标/查询组合，不能因此凭空铸候选。分布未请求图，表格“列1…8”仍缺指标/组身份，人工不通过；业务Trace文本投影存在，本批不宣称Mermaid验收。

### 23.3 已完成并行工具结果的独立保留（确定性验收通过，修后live待验）

修复范围是通用并行生命周期，不为IO例子特判。原`explore_parallel_dispatch.go`在单路提前完成和多路共同完成两条路径整体跳过非胜出分支；正在生成中的分支被取消后，`ParseOutput`还可能在第一处取消检查退出。工具成功发表并不等于其模型任务已完成，也不应被一起丢掉。

新增数据专用合并从已完成分支快照的增量及派发工具缓冲中读取，只接纳成功的确定性运行时观测、命令测量、历史记录、读取覆盖、工件读取和来源清单等producer-owned typed载体。裸stdout、只有RawRef、失败工具及模型emit回执不进入该通道；不合并败方的完成声明、聚合结论、探索笔记、修复请求和执行信号。精确重复结果去重，沿用现有结果容量/截断说明及同代次原生读取收据注册，取消后不再跑推理式ParseOutput。不动readloop、root-cause选举、窗口/链资格、JSON硬门或活跃流超时。

新并行公开回归覆盖正常/取消×winner先后合并4组合，基线均因成功工具缺失确定性失败；修后保留原10.9 typed值和独立命令计数，失败/裸文本/模型结论不进入，winner信号及已接受完成理由不变。另覆盖collective收敛、容量内重复merge不重入、原生来源读取收据保留及该读取收据旧代次不能复活。不是全工具全代次隔离证明；仍受320条/2MiB快照上限约束，不宣称无限保留。

独立复核发现派生交接索引未同步：winner先合并时，后保留的工具虽进入ToolResults，extractor读取的HandoffCarriers却缺ObservationRefs。四组合测试中仅两条winner-first确定性RED，已从保留工具、父已接受证据及父已有载体重建镜像；不复制败方显式载体，不合并败方EvidenceClosure。真实RED/GREEN/race收据分别为`/tmp/codrax-parallel-handoff-review.DQH26v/carrier-mirror-{red,green,race}.log`，末版race覆盖types与orchestrator，通过2.033s/2.283s。首轮工具缺失RED仅保留工具运行回执，未另存日志，不借用镜像RED代签。

公开最终指令回归`TestPublishedIOGroupDistributionsReachFinalizer`在因果/有限事实两种scope验证八项统计、RQ/BIO/读写身份、歧义/缺完成披露与宽窗不进入窄窗ledger，且原ledger字节不变；连同既有业务/S态IO/双窗保护race×3通过6.516s（`/tmp/hmc081-existing-finalizer-context-20260920.log`）。这是既有显示能力的正向保护，不是第二个显示缺口修复。§23.2的原始生产FAIL保持不变，修后live尚未运行。

全仓`go test -p 2 ./...`exit0：87个测试包（37个缓存）、13个无测试包、零FAIL；agent70.628s、orchestrator19.088s、repl63.843s、tool390.853s、tracequery101.547s、types33.882s（`/tmp/hmc081-sibling-handoff-full-20260920.log`）。该轮启动在最后镜像修订之前，不冒称末版冻结后的全仓结果；修订冻结后三个受影响包以`-count=1`完整重跑，通过types45.514s、orchestrator19.347s、agent69.544s（`/tmp/hmc081-sibling-handoff-affected-final-20260920.log`），另有上述镜像末版race。未加run/skip筛选到这两轮整包命令；环境/平台条件Skip不视为实机已验。

末版原生构建通过（`/tmp/hmc081-sibling-handoff-build-20260920.log`）。提交前重新fetch main为0/0；本批11个文件包含数据交接/保护测试及本次双例生产审计，不提交私人完整日志或原始采集。稳定任务清单仍13/79交付、66开放，未把横切生命周期修复当成HMC-02/18整类关闭。后续先验证已完成工具能进入实际成文，再分别处理业务实例绑定与错误早收敛，不把模型数值错误归零。

代码与审计已以`7539cac18`提交推送main，工作树干净且HEAD/远端0/0。该提交干净原生构建通过（`/tmp/hmc081-sibling-handoff-build-clean-20260920.log`，buildRevision=`7539cac18c28`）。独立只读末审另纠正§23.2业务范围描述：模型确实取得50ms业务span，缺口是未把线程状态统计/目标补齐绑定到该业务实例，不是完全没查询业务窗。修后live仍开放，不因此将两个原人工FAIL改为PASS。

## 24. 并行交接修后回放与运行时/源码合同复核（2026-09-20）

起点`04a6cd0a0a28`工作树干净，重新fetch与远端main一致。按“已修可复现P1复验×既有因果/业务保护”优先级，以该干净构建固定2并行×1运行分布与业务IO两例；不改旧case断言或oracle。结果根`eval/results/hmc_parallel_handoff_replay_20260920`，summary及人工审计同名另存，不覆盖§23原FAIL。此批仍是读模式Trace，不冒充写模式/关系图覆盖；600/300/600秒与活跃流策略未改。

在回放期间独立复核§23-R5旧日志，发现比“低增量提前结束”更前置的合同冲突：原分布例1493、1683、1720行要求两个`function_or_purpose`维度提供current-source operation；同流程又不允许把外部trace观察作为源码证据。分析缺少有效源码排除引文后按既有规则回落default，本身不意味着有独立源码义务；classifier仍明确external_tool/optional，已有运行时观察且无精确源码路径/绑定。`RequestedExplanationOperationNeedsForAuthority`此前只识别明确exclude，没有消费其余组件共用的运行时/源码权威判断；通用解释角色被错误扩成源码硬需求。先补真实复现，再收窄到单源精确适用域；不补扫用户原文，不代铸排除引文、不修改角色，不修改低增量阈值或用更多重试掩盖冲突。原模型FIFO误配、量纲与数字错误仍独立保留，不能全部归责于这处门。

业务实例绑定继续只读设计：原回放已取得OpenDocument和LoadDocumentIndex两个各自唯一span，探索游标同时包含app-main/document-worker两个线程名；无已接受principal实例选择，因此不能把任一唯一span或最后一次查询自动升级为全局目标。需要源身份＋实例/线程＋精确端点的联合选择，不是单独填PID或把宽窗换成最短窗；本批不悄悄扩大系统代选权限。具体参考实现与后续任务见§24.3。

### 24.1 修后两例真实结果与本批边界

机器1/2、人工0/2通过，收据为[自动结果](../../eval/parallel_selected_summary_hmc_parallel_handoff_replay_20260920.md)及[人工审计](../../eval/parallel_selected_summary_hmc_parallel_handoff_replay_20260920_manual_audit.md)。分布367秒，业务332秒；无第三例追绿。分布日志3000真实执行“保留非胜出分支4份完成工具结果”，最终上下文和表格都保住三组全部八项值，这是§23.3的生产路径见证；不是匹配基线A/B或证明模型错误全消失。分布正文仍将读组当全RQ、1缺完成+2歧义当3无完成、请求驻留当设备物理延迟、压力背景当已证竞争，人工FAIL。

本轮业务与上轮失效路径不同：分析只引用计量子句，将请求分类为有限事实，诊断/工作关系false，故补齐`families_present`、旁路`trace_root_cause_contract_not_active`，不是`no_typed_target`。正确50ms span与51ms查询账户混用；目标44ms睡眠被写作completion_closed，背景没有已证唤醒被写成没有唤醒。完整Trace投影为0；模型另画的时序图被源码call关系门拒绝，模型patch整图删除。图含无证派发边，因此不能整体免责保图；运行时事实图类型与源码门适用域继续审计。35/31/1/47及不相加文字真实存在，机器词形匹配存在假阴性，但不足以改变人工FAIL。

分布另出现relation member-set修补提示要求当轮不可用的`emit_evidence`。其分析确有required member_set和category/relational声明，不能因不画图或runtime_work_relation=false就撤成员集门；后续模型只补runtime成员集即通过。已确认的缺口是错误的源码修补指引，不是成员集门必然误拒。与本批源码operation适用域修复分开，防止借一个样本整体放松关系证据合同。

### 24.2 本批根修与不扩张边界

源码operation复现跨types单源权威、公开完成Execute和真实explorer初始提示三层，最终原RED收据`/tmp/codrax-runtime-dimension-ownership-final-red-20260920.log`；初版测试收据也保留。第一次修订过宽，触发原“仅附件/triage不能豁免”及精确target反例，失败保留`/tmp/codrax-runtime-dimension-ownership-green-20260920.log`（文件名不代表通过）。没有删除旧pin改绿。

最终新增豁免仅限：共享权威明确源码optional，已有确定性runtime query观察，无任何源码义务/已落地源码载体，无精确文件维度绑定和精确source target。明确source exclusion仍走原有优先级；缺引文不铸排除权，附件或triage、broad runtime sufficiency、citation policy/waiver单独均不够。任意精确binding保留完整维度图，避免prompt与门各自删席；文件位置使用现有精确解析器，不扫描问题/答案原文。四个现有消费者继续共用一个projection，不新造prompt规则，不改低增量/重试/因果资格/时间窗/超时。三包焦点首次全过1.083s/1.183s/2.105s（`/tmp/hmc-runtime-operation-focused-20260920.log`）；末版三包定向race×3通过2.217s/2.802s/7.411s（`/tmp/hmc-runtime-operation-race-20260920.log`），含原源码适用域、显式窗IO交接与最终分布可见性保护。生产代码冻结后的全仓87个有测试包通过（71缓存、16实际重跑，`/tmp/hmc-runtime-operation-full-20260920.log`；tool381.566s、types34.893s、tracediag5.796s、tracequery97.442s），原生构建通过`/tmp/hmc-runtime-operation-build-20260920.log`。上面的生产回放固定旧快照，不冒充本修复live验收。

独立末审确认没有放宽已有源码义务，另留一条非新增回归边缘：结构化ExactTargets为`capture.tracebundle.json:9`时，共享精确位置解析器会因`.json`将其保守视为源码约束，本批新增豁免仍不能覆盖它。后续只能在本地检查解析后文件的既有runtime/blob身份，并补source/config反例；不因该边缘改动全局位置解析器或源码权威。该边缘与本轮真实IO例无关，不冒充本批已修。

本批代码、双例回放收据及剩余项以`744751c60`提交推送main；推送后工作树干净、HEAD/远端0/0，干净构建通过`/tmp/hmc-runtime-operation-build-clean-20260920.log`，buildRevision=`744751c60ca1`。该修复是HMC-01/16横切子缺陷收尾，不把整个能力目录或教学同源任务销账；稳定任务数仍13/79交付、66开放。

### 24.3 业务实例绑定实施清单（设计已核实，尚未施工）

参考仓`server.py:959`的launch服务与`core/preprocess/launch_ops.py:736`生成逐实例thread_queries，`config/skills/launch_perf.yaml:64`用同一item的start_ms/end_ms/tid组合调用，值得吸收的是原子范围绑定；不照搬按时长Top3定瓶颈、first-instance-wins或`thread_query_ops.py:562`包过滤无命中回全量。本仓已有TraceSpanSummary/ObservationSourceRef的真实端点/线程/来源，不需另建解析器或根因排序器。

HMC-02.4先拆本切片（仍属于原79项，不新计已交付数）：

1. 发布可寻址实例收据：源/有效代次、端点行、精确TID、完整区间、配对与覆盖/截断；键不取名字或数组序号。
2. 已接受完成分支显式选择已发布的实例ID；系统回查数值，不接受模型自填PID/时间，不改user_explicit/profile，不接纳取消败方焦点。
3. 补齐原子消费(source,generation,tid,start,end)，重跑既有状态/链/rank组合；显式用户身份和窗口最高优先，完整业务区间与用户窄窗分别保留。
4. 正反验收：两个各自唯一span不自动择一；同名/嵌套/异步TID-vs-TGID/截断/缺E/伪造ID/旧代次/跨源均保守；调换工具顺序/并行顺序身份不变。35ms请求/31msS态阻塞/1ms调度、47ms背景不晋升和业务线索分别保护。

无明确焦点、冲突或不完整收据仍披露跳过；若后续需要自动从业务名称选择，再独立设计有来源引文的typed selector，不将entities或最长span变硬门。HMC-04.3完整启动实例/阶段远大于此切片，不能一并销账。

### 24.4 成员集修补指引后续切片（初始只读设计，后续施工见§25）

独立核查定位到`emit_investigation_complete.go::relationMemberSetHandoffDowngrade`固定发`RepairEmitEvidence`，`explorer.go::renderCompactClosureRepairSection`据此要求已读源码；observation-only阶段本来就不提供`emit_evidence`，不应为了满足错误提示开放它。现有completion-only保护不适用于仍提供`trace_query`的本轮。后续拆为：

- 证据来源分支使用共享runtime/source权威＋ledger，已有独立外部证据且无必需/已落地源码压力时，使用既有`RepairStructuredHandoff`要求补members/count/origin/provenance；仍保留principal-blocking普通origin，不使用会降成advisory的completion-form前缀。初次仅scalar facts时也要覆盖，不能借用“已有member_set”才成立的穷举帮助函数。
- 通用提示读取`LoopObservation.ToolSurfaceKnown/AvailableToolNames`精确工具集合；确知`emit_evidence`未暴露时停止要求调用它。未知集合保持原行为；不改stored repair，不把缺工具当源码豁免，不开放工具或放行缺失成员集。
- 验收runtime scalar/无成员集、supporting-only/数量错仍拒，合法成员集才完成；普通源码、精确mixed、已落地源码保持旧债务。覆盖trace_query仍可用但emit_evidence不可用、后者可用、集合未知及mixed缺工具，不扫模型原文判来源。

## 25. 运行时成员集修补与当轮工具能力一致性（2026-09-20）

第一批`744751c60`推送并干净构建后，继续处理§24.1实际暴露的提示错配。此批不改变“关系查询必须交付有效成员集”的门、不更改问题分类，不将“无图”或runtime_work_relation=false当作免检信号；也不为错误提示开放源码工具。§24两个live原verdict保留，本修复另用公开确定性回归验收，不增加第三例追绿。

### 25.1 生产者给出正确修补通道

`relationMemberSetHandoffDowngrade`原本不区分证据来源，一律发`RepairEmitEvidence`。本批仅修改其修补类型与说明：共享runtime/source权威没有独立源码义务/载体，且确有独立外部观察时，发已有`RepairStructuredHandoff`，要求原成员、精确数量、principal角色及来源凭证，工具明确为`emit_investigation_complete`。没有member_set和仅scalar的初态都覆盖；修补保持principal-blocking，不使用completion-form advisory前缀。验证成员集的原条件不改，错误数量/支持性集合/未支持成员仍不完成；普通源码、精确mixed与已落地源码继续原通道。

父席复核又发现旧模型aggregate可进入共享ledger，不能凭自填`trace_query`来源冒称独立证据。最终完整ledger仍负责保留所有源码要求；外部前提单独复用`HasDirectRuntimeObservation`，资源类仅让`DirectObservation`记录参与既有充分性判断。仅附件、citation waiver、保留的模型scalar/provenance均不能自证；真实addressable MCP resource也有正例，非仅IO/Trace专用。公开RED两初态及retained-model负例来自独立实施席原始工具回执，未重定向为磁盘日志；不伪造对应`/tmp`文件。最后扩展`Relation|MemberSet`通过7.280s，并验证合法修补接受后ActiveRepairs为空。

### 25.2 通用提示核对实际工具集合

初次修补、主动修补、后续closure-only三处共用显示层能力判断，读取该轮真正交给模型的`LoopObservation.ToolSurfaceKnown/AvailableToolNames`。未知集合沿旧行为；已知缺工具时披露所缺工具与原债务范围，停止重复不可执行的producer指令。缺工具不代表证据已满足、不清空repair、不增加工具权限、不设置完成/停止信号。

显示层复用`RepairDirectiveRequiredTools`及类型默认动作的并集，覆盖显式工具覆盖默认、多个修补合并以及advisory三种形态。为检查advisory动作可用性，仅在值副本上忽略advisory标志；原调度/存储分类不动，提示明确“建议不新增完成阻碍”。同时避免后续closure-only在一次导航后重新发不可用的源码提交要求。

公开首轮反例`/tmp/hmc-repair-tool-surface-final-red-20260920.log`及独立末审新增反例`/tmp/hmc-repair-tool-surface-review-red-20260920.log`已保留；定向末版通过`/tmp/hmc-repair-tool-surface-final-focused-v2-20260920.log`。第一轮绿色验收曾因read repair规范化本来清空Subject而触发测试期望错误，已改为校验实际存储的subject及scope，不更改生产规范化。末版三包race×3通过types2.392s/tool3.577s/agent7.184s，收据`/tmp/hmc-repair-capability-race-20260920.log`，含第一批源码适用域、关系门以及显式窗IO/分布交接保护；原生构建通过`/tmp/hmc-repair-capability-build-20260920.log`。随后只把新增首次/后续提示测试提升到真实observeMidLoop观察循环，生产未动；增量race×3通过2.880s（`/tmp/hmc-repair-capability-observe-race-final-20260920.log`），agent完整包另通过82.536s（`/tmp/hmc-repair-capability-agent-final-20260920.log`）。生产冻结后的全仓87个有测试包全部通过（64缓存、23实际重跑；tool387.553s、types38.034s、tracequery102.264s、tracediag5.838s），收据`/tmp/hmc-repair-capability-full-20260920.log`。末版提交前diff检查及fetch main 0/0；提交/推送收据另补。

### 25.3 后续优先级（未运行，不当验收）

下一修复先补§24.3原子实例绑定与组合意图的正反验收，再单独审运行时图/源码图边的凭证范围；不把这三类问题混成统一豁免。生产批仍固定2并行×1：本批两个Trace原失败已完整审计，下一异构保护对候选为`read_combo_trace_current_source_explanation.case`（显式需要源码，防本批optional修复外溢）＋`github_issue_dateutil_relativedelta_float_symptom.case`（Python写apply/回归测试，防长期只跑Trace）。两例现有定义已核对；排序依据为受改动影响的合同强度、跨模式风险和已有覆盖，未伪称已逐一重跑全部cases或写模式已由本批live覆盖。

### 25.4 交付收据

本批六个文件以`989fd23ec`提交推送main，推送后工作树干净、HEAD/远端0/0；干净构建通过`/tmp/hmc-repair-capability-build-clean-20260920.log`，buildRevision=`989fd23ec667`。仅提示/修补通道和保护测试发生变化，成员集有效性硬门、源码权限、显式窗口、根因链资格、Trace自动补齐、旁路及600/300/600秒配置均未修改。稳定清单仍13/79已交付、66开放；§24回放人工0/2仍成立，两批新修复的live复验单独开放。

## 26. 先回到人工FAIL：业务实例原子查询引用（2026-09-20）

用户明确要求先处理“前期未通过、未误销账”的两例，再继续其余开放项。起点`73dc9b9fb`，fetch后HEAD/远端0/0且工作树干净。本批将§25.3拟定的混合源码读/写apply对顺延：先同批2并行×1复验IO分布与业务响应旧FAIL，优先级依据是旧缺陷仍影响实际回答、两批合同修复尚无live收据、业务窗口错抄会直接改变计量。顺延不代表跨模式验收取消，更不以Trace测试冒充写模式覆盖。

### 26.1 组合意图复核：已有充分教学，不再叠加规则

独立核对§24业务例真实日志：`run-1.logs.all.log:443/556/570`已经把“因果判断＋工作关系＋有限测量”与纯事实/纯路径分别说明；752的emit-only提示只收束工具，不要求改成有限事实。806的首次模型发射已遗漏因果角色，并只引用问题最后的计量子句；809的拒绝仅针对`no_named_target`与三个目标矛盾。839移除目标后重发，843接受的仍是同一个窄化意图。因此这是模型未遵循已到达教学，本次不能声称发现系统删掉角色或教学缺失，单次也不足以测得波动频率。后续保持观察，不用问题关键词扫描强改分类、不自动铸造因果请求。

新增真实`NewAnalyzerAgent.Execute→adapter`消息回归，四类通用请求（组合诊断、有限事实、有限效果、纯路径）都检查现有教学与提供的schema到达，且没有凭该测试自动填`RequestModel`。首跑即绿，不伪称RED→GREEN修复；三包定向及race收据分别为`/tmp/hmc-runtime-composition-existing-teaching-focused-20260920.log`和`/tmp/hmc-runtime-composition-existing-teaching-race-20260920.log`。旧人工FAIL不变。

### 26.2 原子范围绑定首片：公开查询，不代选焦点

重新逐行核对参考仓`server.py:959`、`core/preprocess/launch_ops.py:736`及`config/skills/launch_perf.yaml:64`：吸收同一item的线程/开始/结束一起用于后续测量；不搬运Top3时长自动定瓶颈、数组序号关联实例或筛选失败回全量。本仓直接消费原生`TraceSpanSummary`已配对端点，不新建解析器，不从Subject拆TID或从富注/模型文字反推数值。

`trace_query`新增可选`business_span_ref`。成功的同步B/E单物理来源查询，在原生结果发布后给出本轮不可重放的实例引用；选择引用后，后续查询一次性继承物理采集、原文件代次、scheduler TID、完整开始/结束与端点行。引用不接受额外源、线程或窗字段拼接，用户明确时间范围不能被完整业务区间扩大；需要裁窗或分析依赖线程时仍走原显式参数通道。普通查询、自动补齐和用户目标优先级不改。引用只是查询导航，不是已接受completion焦点或根因凭证，也没有因其最长/排第一就自动选举。

安全边界：复用原`TraceQuerySourceReadRef`的物理文件身份与本轮代次，引用自身永久保留原收据，绝不向同路径最新收据借权；JSON重放、伪造token、重置轮次、同字节/大小/mtime但换inode均不能复活。每结果至多16个引用，本代父/兄弟fork共享最多1024张的铸票预算，先预留额度再公布，merge不驱逐已发票；未列实例仍可普通显式查询，导航子集不冒充完整清单。新私有切片在memo/dispatch/TurnA/合并处独立复制并计入容量。消费引用绕过较弱size/mtime纯工具memo，但保留引擎索引缓存；查询前后复核同一物理域，阻断后来自动提升到sibling bundle造成范围变化。单物理零测量结果仍保留诚实空值；异步S/F、track、未闭B以及复合坐标不授本片快捷引用，不剥夺其原有事实查询。

公开前置失败：`/tmp/hmc-business-ref-red-20260920.log`，合法OpenDocument的50ms配对已存在，但无原子后续查询引用。修后公开Execute拿引用再查原引擎，测得`1.000000..1.050000`内5ms运行、1ms runnable、44ms睡眠的原账户，不将51ms探索窗套入50ms响应；最初测试编写有字段名编译错误，纠正后才保存这个行为RED。另覆盖同名嵌套/不同线程、marker TGID≠scheduler TID、完整/裁剪区间、伪造/旧代次、用户窄窗、异步/缺端点/复合源和并行发布。没有新造RootCause选择，没有改35/31/1/47的IO计价或背景准入。

### 26.3 验收与剩余范围

首轮三包定向race×3通过（types3.856s、tool6.129s、agent3.106s），收据`/tmp/hmc-business-ref-race-20260920.log`。独立末审又发现两个新接缝边界：引用调用后置来源拒绝前已登记自动补齐窗口；显式窗检查未覆盖仅保留Mutable请求模型的恢复会话。两者公开Execute回归均先红（`/tmp/hmc-business-ref-review-red-20260920.log`），再改为引用查询成功且前后来源校验通过后登记、复用既有scope accessor，绿色收据`/tmp/hmc-business-ref-review-green-20260920.log`（1.137s）。初版来源测试误用了不自动提升的legacy清单，已改为真实哈希绑定V2并独立断言BuildIndex确实提升，再保存有效行为RED；没有为测试改变清单准入。

末版生产冻结后，三包定向race×3再次通过（types4.735s、tool5.454s、agent4.656s），收据`/tmp/hmc-business-ref-final-race-20260920.log`，真实Explorer消息测试也验证引用schema确实到达。原公开行为修后通过`/tmp/hmc-business-ref-green-20260920.log`，扩展组通过`/tmp/hmc-business-ref-focused-20260920.log`。末审前已启动的全仓回归不冒充最终版本，末版另跑全仓`/tmp/hmc-business-ref-final-full-20260920.log`；整包、原生构建和2并行×1生产回放收据另补，不以当前局部测试预签全仓或live。

HMC-02.4从“待实施”改为“部分实施”：本片完成§24.3第1项与显式查询消费接缝，第2项accepted completion选择和第3项supplement原子消费仍未完成；HMC-04.3完整启动阶段不销账。业务因果意图遵循、运行时图/source-call凭证适用域、分布组/总体及缺失/歧义等答案错误仍分别保留。稳定清单仍13/79交付、66开放。

### 26.4 全仓反例与兼容收口（不跳过红针）

实现先提交`799a30d34`以固定构建输入，尚未推送；首轮全仓`/tmp/hmc-business-ref-full-20260920.log`返回非零，tool383.423s有三条失败：pure memo首轮Summary后追加令牌而缓存未含同一尾部，破坏逐字复用；参数镜像census未显式处置新字段及其self-red派生失败。其它包通过不能覆盖这三条红针。

修正发布范围为schema已承诺的`span_window`、`window_stats`及明确`span_locate`配方（复用既有别名解析）；其它rank/bundle等普通视图不顺带发票。发行与消费两端都绕过弱纯结果memo，仍使用引擎索引缓存；因此普通root_cause_rank的逐字复用原断言不变。新增三种发行面的公开双次调用验证：原生重读、不同本轮token、不可memo铸票。旧memo scope规范化测试仅将输入视图从已不参与memo的window_stats改到root_cause_rank，保留default==thread、process!=thread三项原断言，另由新公开测试验证发行车道，非删针放行。tracediag roster明确本字段tool-only：本轮注册表令牌不能变成跨进程脚本的持久坐标，诊断脚本仍用显式范围。

组合定向回归通过1.427s（`/tmp/hmc-business-ref-compat-final-green-20260920.log`）。此前启动的第二轮全仓`/tmp/hmc-business-ref-final-full-20260920.log`同样早于这次兼容修正，不充作最终收据；`17df3e7d9`固定构建通过，v2全仓与2并行×1生产回放并行执行。v2定向race×3通过types5.462s/tool10.752s/agent4.047s（`/tmp/hmc-business-ref-final-race-v2-20260920.log`）。

继续审memo调用方发现：B1697等既有无span的window_stats查询仍需正常缓存，不应按整个view禁用。末版进一步收窄为原生结果的精确字段策略：只有带`TraceBusinessSpanCandidates`的发行结果不进纯结果memo；消费引用仍绕过弱memo；无候选的window_stats及其它普通结果保留原缓存合同。复用原memo核心，未改变cache key，也不从摘要关键词判定。scope规范化测试已恢复原window_stats输入，B1697旧pin保持不改；包含所有Memo命名测试的组合通过1.483s（`/tmp/hmc-business-ref-memo-policy-green-20260920.log`），末版另跑v3全仓与race，不用v2回放替这次增量签live。

真实两轮Explorer→adapter测试也已补：第一轮实际执行物理sync B/E查询，第二轮模型消息保留token、完整实例tuple且注册表可解析（3474字节，引用首字节offset3141）。它解释了2000字节工具日志预览可能搜不到引用，不能据截断日志断言模型没收到；此测试不代替某条live消息的完整录制。新增测试只改变测试文件，生产不变。

### 26.5 末版回归收据（最终全仓通过，早期失败保留）

`d0e69c2cd`固定末版结果级memo策略，三包定向race×3通过types4.854s/tool9.004s/agent4.863s（`/tmp/hmc-business-ref-final-race-v3-20260920.log`），原生构建通过（`/tmp/hmc-business-ref-build-final-v3-20260920.log`）。构建启动时尚有未提交改动，其版本标记含dirty，不冒称干净发布构建。v2整仓最终返回非零：`TestB1631FrequencySourceMemoJSONAndLegacyBoundary`、`TestB1697MemoHitCannotGrantReplacedCapture`两条无span缓存合同失败（tool383.614s），都已由末版收窄策略覆盖，原断言未改。

末版v3全仓（`/tmp/hmc-business-ref-final-full-v3-20260920.log`）仍返回非零，唯一失败为既有写模式 `TestRunTestsTimeoutExitDisclosesInfraDowngradedLockfileAndUntrackedOutput`：2秒超时前未产生脚本预期的Cargo.lock修改和junk.out，tool370.601s，其它86个测试包通过。随后单独核查启动边界/环境波动，不凭不相关路径就撤针，不将定向race签成全仓绿。

独立只读核查确认相同现场在主账本`eval_priority_campaign_audit_20260730.md`的2026-09-15记录已经出现；`86e88c33e`更早于9月10日专门增加现有诊断。测试假定新shell在2秒内先改两份文件再sleep8，但预算在进程启动前即开始，未设进入脚本的就绪见证。现场两份文件未变化，审计返回clean符合实情，不是“漏审已经发生的写入”。本轮原参数定向count3均通过、每次2.16s（`/tmp/hmc-timeout-audit-targeted-3x-20260920.log`）；仅支持既存启动/时序可靠性债，不足以确定卡在哪个启动阶段，也不称修复。生产与测试的2秒/8秒及断言都不改；整仓v4独立复跑收据另列。

最终同一生产代码冻结后的`go test -p 2 ./...` v4返回exit0：87个测试包通过（77缓存、10实际重跑）、13个无测试包、零FAIL；tool374.380s、types33.615s、hitraceconv108.812s、orchestrator16.053s（`/tmp/hmc-business-ref-final-full-v4-20260920.log`）。未添加run/skip筛选，既有平台/环境条件skip不当作实机验证；没有因v3失败改变产品或fixture超时。v4通过不消除既存间歇测试债，也不能代替两条live人工验收。eval测量器另以§27.1完整脚本收据验收；本批不改read scheduler、因果准入、系统补齐时限或流式等待默认值。

本批五笔提交`799a30d34`、`17df3e7d9`、`d0e69c2cd`、`3d7f9a690`、`661a075c5`已推送main；推送后工作树干净，HEAD/远端0/0。干净原生构建通过`/tmp/hmc-business-ref-build-clean-20260920.log`，版本实测`661a075c583e`（无dirty）。后续本收据更新仅文档，不改变上述生产冻结或live基线。

## 27. 业务实例首片后两例回放：保留人工FAIL，分清答案错误与测量器缺口（2026-09-20）

固定`17df3e7d9fcf`构建，2并行×1，业务332秒/分布371秒，机器1/2、人工0/2通过。结果为[自动summary](../../eval/parallel_selected_summary_hmc_business_instance_replay_20260920.md)及[逐例人工审计](../../eval/parallel_selected_summary_hmc_business_instance_replay_20260920_manual_audit.md)，私有完整结果根`eval/results/hmc_business_instance_replay_20260920`。不启动第三例追绿，不改已有case与结果；末版`d0e69c2cd`缓存增量没有本批live签收，三包确定性与race另列§26.5。

| 开放项 | 本轮实际证据 | 收口范围与下一步 |
|---|---|---|
| HMC-02.4 / 04.3 业务实例焦点，P1 | 正确50ms span已经查出，后续仍把51ms状态账户套入业务响应；补齐明确`no_typed_target`，root-cause-rank未执行，后续查询未消费实例引用 | 首片查询能力不能代替accepted completion选择与supplement原子消费；按§24.3第2/3项继续。无选择不自动挑首个/最长实例，取消败方不能提交焦点，用户显式窗/目标优先 |
| HMC-16.4 组合意图，观察 | 本轮最终恢复causal_diagnosis＋工作关系，但5次分析发射间仍在因果/有限效果变动，附件窗还被当成用户显式窗；已有教学已到达 | 不新叠提示或用原文关键词硬改分类；真实足够上下文上的模型未遵循保持观察，不统称合同冲突 |
| HMC-08.1 / 16.4 IO计量口径，P1 | 三组八项统计真实进入finalizer且数表正确；正文仍把缺完成说成缺发起、BIO队列驻留说成设备物理延迟、线性分位数说成最大值、all issuers说成reader40 | 数值内核不拟合错答。组级补充引擎单源端点口径，独立保护未知/歧义；保留正文事实错误，不能靠数值烟测签PASS |
| HMC-18.5 投影评测计数，P2 | 业务Markdown75行有`Trace 因果投影（补充查询范围）`，98行typed text围栏，日志3387物化；机器只认裸标题/破折号后缀，误记0 | 按生产精确标题语法修计数并补围栏伪标题反例；旧机器FAIL不重写，人工仍因跨窗及context-only关系升主因而FAIL |
| HMC-01.2 / 16.4 修补与不可用工具，P2观察 | 业务patch继续误用field=facet_ids，正确schema为add_facet_id；分布仍误调当轮不可用emit_evidence一次，之后两次completion均接受 | 暂未确认新的必带/必拒合同矛盾，不开放任意patch、不增重试、不把后续正常dispatch误算为拒绝循环 |

两份root-causes.json均正常生成schema2空结果：业务为`no_selectable_typed_on_chain_candidates`，分布为`trace_root_cause_contract_not_active`。前者仍需因果组合取证，后者是纯事实请求的正确边界；都不是旁路落盘失败。业务文本投影真实存在，不冒称Mermaid通过；两例仍是Trace读模式，不冒充写模式验收。没有改变显式时间窗、链上资格、35ms请求/31ms实际S态IO阻塞/1ms调度/47ms背景区分或600/300/600秒等待策略。

只读回溯上一批分布上下文补充一个精确范围：`hmc_parallel_handoff_replay_20260920`的finalizer没有BIO queue端点口径，Top8明细被较长RQ占满，确有组级计量口径不依赖Top8的交付空间；三组数值和issuers=all则已到达，不能泛称信息全丢。应复用`blockIORequestResidenceCaliber`等引擎语义，不能在工具层按名字复制第二套推断；`Count`还会计入孤立完成，不得全部重命名为issue数或用Count-Paired推缺完成数。

独立末审追加HMC-08.1/16.4同批后续边界：本批finalizer3516行首层摘要把RQ读组标为`thread=reader-40`，3534行notes却是`issuers=all`，实际fixture还含reader41。故单线程错误归属不能全部写成模型凭空编造，组级身份展示需要与统计总体同源；只改提示词不够。另记录答案将歧义组数与受抑制请求数混为可加数量：3个未合格请求是1缺完成＋2受抑制，1个歧义组不是第4个请求；不改配对引擎去适配这句错误。

稳定清单保持13/79已交付、66开放。顺序：收本批查询引用与测量器修复 → accepted实例焦点/补齐 → 聚合组IO端点口径 → 恢复mixed-source读/Python写apply固定双例；业务完整启动阶段、全层总体分位数和其它66开放项不据此代销。

### 27.1 投影计数的确定性修复（3d7f9a690）

仅修改eval测量器与其测试，不修改产品渲染、case/oracle期望或已存结果。依据`tracefence.SectionProjectionZH/EN`与`runtimeTraceQueryScopeTitleSuffix`，识别生产已发射的中英文标题、工件后缀、单窗补充范围、多窗第i/n窗、未知窗及具体补充窗；不接受任意括注新词形。新增Markdown围栏状态处理，反引号/波浪线、0–3空格、长围栏中的短围栏均有反例；普通正文、比较/覆盖边界章节与围栏内伪标题不算发布投影，同块的typed text围栏不另加一次。

真实本轮业务报告固定内容，计数先0后1；收据`/tmp/hmc185-projection-real-{red,green}-20260920.log`。20个标题正例及负例在完整runner脚本先红（`/tmp/hmc185-runner-red-20260920.log`），修后全过（`/tmp/hmc185-runner-green-20260920.log`，含原post-apply/NAPI scope合同测试）；shell语法与diff检查通过。脚本中的预期FAIL负例不等于脚本失败，最终exit0。此修复已单独提交`3d7f9a690`，不回写本批或旧批机器FAIL，更不改变人工0/2结论；HMC-18.5为持续执行项，保持开放。

### 27.2 IO组身份/口径的施工清单（原只读设计；实施见§28）

本切片归HMC-08.1/16.4，不新增已交付计数。已定位而非只凭答案猜因：`trace_query.go::traceQueryTypedStorageLatencySummary`及`writeTraceStorageLatency`无条件显示代表线程；`blockPairingAccumulatorFor`实际按source/family/dev/op聚合，Thread只保留首记录。现有`traceQueryStorageGroupFields`已经声明block issuers=all，应该共用，而非再堆prompt。

- [x] 先补公开真实RED：扩展`TestHMC081PublicBlockGroupIncludesMultipleIssuersWithoutSinglePIDClaim`，分别验证工具Summary、普通及EvidencePack observation、压缩摘要，不再把summary＋notes全局拼接后只找一个issuers=all。block总体不得同时带暗示单线程范围的thread；非block的inode/PID身份保持。
- [x] 首层展示共用既有组字段；若保留Thread，明确命名为representative_thread，不抹掉原数据，不改变组键或发起者总体。
- [x] 聚合行增加可选精确请求驻留口径，由已准入RQ/BIO生产者调用`blockIORequestResidenceCaliber`，不在工具层按名字再推断；generic/未知不得调用其默认RQ分支。沿显式schema处置、key-first渲染及哈希pin流程同步，不只bump哈希。
- [x] 工具、普通/EvidencePack observation及前部scope notes同源展示端点；finalizer/reviewer原设计按10/6条说明考虑，实际finalizer最短仅3条，§28改为第一条同时保组身份/端点。新显示字段不得授链上资格或根因权。
- [x] Count仍保原数值与各层语义：block孤立完成也计数，generic完整pair可能两端各计一次；缺开始/缺完成/歧义组/受抑制操作分列。RQ/BIO/generic/未知、孤立完成、1组2请求、nil/真实零值、Top8外组及跨窗都有正反例。
- [x] 扩展`TestPublishedIOGroupDistributionsReachFinalizer`，逐条真实上下文记录绑定同组的身份/端点/数值，不靠整段任意位置搜token；继续保留来源/窗口隔离及ledger字节不变。固定双例已运行，人工仍FAIL，见§28.3；这是实施/运行完成，不代表答案验收通过，旧人工FAIL不倒签。

参考仓再次对照：可借`core/preprocess/io_ops.py:67`的分布组织，不搬`config/indicators/io/io_latency.yaml:35`与`io_ops.py:879`的LIMIT后算分位数或start_time BETWEEN漏carry-in，也不搬`io_ops.py:448`以CFS累计阻塞大于RT直接判优先级反转。该路径读取已经形成的filesystem_io记录，并非本仓原始RQ/BIO端点/缺端计数的权威来源。

## 28. 旧FAIL先修：IO统计组身份与起止事件同源交接（2026-09-20）

起点`3d1bc834a`，fetch后与origin/main一致、无遗留改动。本批优先闭环§27.2可执行复现的系统上下文矛盾；accepted业务实例焦点仍单独开放，不将查询引用等同于已接受因果目标。参考实现仅借用“全样本分布与Top-N展示分开”的组织，不移植其取样限制、分位数定义或根因判据。

### 28.1 实施范围与红针

- [x] RQ/BIO聚合行新增可选`request_residence_caliber`，只由已准入端点族生产者调用原口径函数。一个闭集解码提供RQ发起→完成、BIO入队→完成的原始事件名，工具和诊断渲染共用；空/未知值不从事件名称反推。generic仍不冒用RQ尺。新字段不参与配对、统计、选举或根因资格。
- [x] 工具首层、普通/EvidencePack观察和紧凑说明共用原组身份字段。block保持source/family/dev/op下全部提交者，不再无条件发`thread=`；原Thread载体不删除，原始说明另标`representative_thread`且注册为display-only。非block保留inode/PID与原thread显示，兼容旧摘要审计视图。原claim digest的字段/顺序不变。
- [x] 真正的finalizer路径可能只保留3条说明，不总是10条。将端点与组身份放在第一条，来源/选择窗随后；不提高全局预算、不依赖Top8请求或第四条说明，semantic仍6条。逐组逐Observation ID核验原生query→TurnA→BuildInitialInstruction，同时保住八个数值与原ledger字节。
- [x] schema显式处置：有口径但无配对的组也进入精确渲染，披露端点但不造零样本；未知口径只说未说明，不泄漏枚举。新增字段及原分布字段剔除后仍匹配此前schema，加性针/结构指纹同步审查；未带新口径的legacy nil组原字节针不动。
- [x] 请求计数与时长不改：孤立完成、缺完成、1个歧义组包含2个被抑制请求、generic两端计数、真实0、跨窗完整驻留及Top8之外样本都有保护；不将Count统称issue数，也不把驻留耗时等同响应阻塞。
- [x] 最终全仓/race、固定构建及2并行×1回放均完成，收据见§28.3/28.5；机器2/2、人工0/2，不替旧FAIL销账。

前置行为RED：`/tmp/hmc-io-caliber-red-20260920.log`（RQ/BIO十一臂缺组级口径）；`/tmp/hmc-io-group-context-red-v2-20260920.log`（同一模型记录缺端点、裸thread歧义）；独立末审再补`/tmp/hmc-io-group-raw-notes-red-20260920.log`（三组×两scope共6处原始RichNotes残留裸thread）。修后真实上下文及相邻IO测试首次通过`/tmp/hmc-io-group-context-{green,neighbors}-20260920.log`；后续末版收据单独记录。

初次组合回归有一条旧措辞pin失败：把说明改成“仅统计与查询窗相交的完整配对请求”后，原“本组完整配对请求”片段不再出现。最终用条件句“仅统计本组完整配对请求，且须与查询窗相交”，保留原针且避免无配对时误称已存在样本；不删除断言。schema指纹红针亦先保留实测再按上述字段处置重钉。

### 28.2 不扩权与仍开放边界

没有改变JSON教学/工具权限、显式用户窗口、自动补齐调度、链上根因资格、35ms请求驻留与31ms阻塞的区分、`.root-causes.json`必选旁路或600/300/600秒等待配置。无正文短间隔不是本批的降级条件，未加原文关键词门。

独立复核旧Summary-only fallback：只有typed Observations和ToolCarrier均缺失时才进入；其产物被强制audit_ledger/display_only、置信度≤0.2、not_answer_grade。它仍不能完整理解新block组端点/全部提交者说明，作为旧无结构化结果的审计视图限制留账；正常TraceQuery、memo、fork、TurnA都有typed载体，不为这个旧fallback扩张prose解析或授因果权。非block恢复原thread显示以保既有兼容。

稳定清单仍13/79交付、66开放。HMC-08.1只是组级统计/上下文切片，未完成跨层总体上卷；HMC-02.4 accepted焦点/原子补齐仍需根修。业务旧FAIL以及分布答案的缺端含义、歧义计数/分位数解释是否遵循，须由新回放独立验收；旧机器/人工结果不回写。后续mixed-source读＋Python写apply固定双例继续保留，不借本批Trace回放声称跨模式已验收。

### 28.3 固定双例仍未通过人工验收，不误销账

`04f4103bf3ad`干净原生构建（`/tmp/hmc-io-group-build-20260920.log`）运行2并行×1，每例1800秒。业务269秒/43%上下文、IO分布329秒/31%上下文；[机器摘要](../../eval/parallel_selected_summary_hmc_io_group_scope_replay_20260920.md)2/2 PASS，[人工审计](../../eval/parallel_selected_summary_hmc_io_group_scope_replay_20260920_manual_audit.md)0/2 PASS。没有第三次追绿、不改case/oracle、不回写历史FAIL。

业务本轮保住自动补齐、Trace因果投影、链上IO31ms/两处调度各1ms/47ms背景及schema2三项根因旁路；不能沿用上一批`no_typed_target`描述本轮。仍未消费原子业务实例引用：50ms操作已经找到，主分析及补齐仍用51ms探索窗，未给出50ms的5+1+44ms账户；另有完成/唤醒时刻与IO/调度修向混淆。业务实例accepted焦点缺口不销账，也不因这次自动补齐成功就关闭原子范围绑定。

IO最终三组八项数字及RQ/BIO起止事件正确，组设备/全部提交者/选择窗已进入真实finalizer消息，确认本批系统交接修复到达。正文仍将17条已配对请求仅展开8条误说成仅8条配对成功、9条缺端或歧义；将事件分布区间误作查询选择窗并声称RQ/BIO窗口不重叠，另有设备遗漏、歧义组/请求数量和硬件阶段误述。关系桥接仍使用泛称coverage/emitted/hidden、通用window标签，存在系统展示可澄清空间，不能全归模型波动。

JSON恢复没有丢答案：业务正文先接受，再单独补可选schema2根因选择；IO的blocks字符串数组被既有安全修复接受。机器repair=0与completion拒绝计数有观测口径限制，人工按真实事件核对，未发现本批新增“必带又必拒”合同。Explorer仍收到预处理错误推算的软建议，但finalizer已抑制该自由文字并由确定性测量替代；不误报为最终测量权威仍被预处理污染。

独立末审新确认确定性展示缺口：业务报告154/288行的系统投影把背景IO的17.850ms加权impact标为“完成端到端·IO延迟”。原生blob `trace-query-result-6dc5183a.json:2268–2307` 中physical/projected/cumulative均为47ms，仅impact_ms为17.85；`runtimeTraceProjIOFoldNoteText`按类型命名却只取peer.ImpactMS。这不是模型波动，应按HMC-16.4/16.5另批先红后绿修量尺交接，覆盖节点/折叠/证据索引；本批不改排序/权重来拟合47ms，也不因背景没有当主因就忽略错标。

### 28.4 下一批高ROI退出条件（未实施）

1. **HMC-02.4 accepted业务焦点**：按已接受模型选择引用消费同一物理来源/代次/TID/完整窗口，不从探索ANY、首项或最长自动选择；取消/失败分支不提交焦点。保护显式用户窗及现有无引用自动补齐。
2. **HMC-08.1/16.4覆盖语义五分离**：已接纳完整配对总体、明细展开上限、捕获/扫描完整度、查询选择窗、事件首末区间分开。复用原typed计数与QueryScope；工具生产者和finalizer关系桥接同源解释，不修改配对引擎/数值、不造全量捕获声明。任意E<N、E=N、零/缺失/歧义、短事件区间及不同来源/窗/代次均需正反回归，缺端只取Unpaired*等精确字段，不能以N-E反推。
3. **HMC-16.4/16.5与HMC-18**：优先为IO折叠说明的影响值/端到端耗时错标补公开红针，统一量尺载体后修复，不改因果选举或把所有影响值换成物理时长。正确证据已到达后的方向/端点/枚举误述继续观察，不以一次回放宣布稳定模型波动；Explorer残留软建议另审。mixed-source读＋Python写apply双例仍排队，本次两个Trace读例不替代。

以上归原稳定ID及既有66开放项，不把新增分析记录当作额外交付数，也不依赖扫描用户/答案原文修文或增加硬门。

### 28.5 最终验证收据

生产实现固定提交`04f4103bf3ad`。末版定向兼容测试通过tool1.627s/types1.994s（`/tmp/hmc-io-group-compat-final-20260920.log`），真实上下文测试通过agent0.993s（`/tmp/hmc-io-group-context-final-green-20260920.log`）。末版tool/agent/types定向race×3通过6.127s/4.029s/3.059s（`/tmp/hmc-io-group-final-race-20260920.log`）；配对引擎与诊断渲染在未受最后非block显示兼容影响的生产版本也完成race×3，收据`/tmp/hmc-io-group-race-20260920.log`，不混称所有包都在最后增量后重跑。

首轮全仓`/tmp/hmc-io-group-full-20260920.log`已exit0，但启动早于最后非block线程标签兼容修正，因此另跑最终`go test -p 2 ./...`，`/tmp/hmc-io-group-final-full-20260920.log`返回exit0：87个测试包通过（76缓存、11实际重跑）、13个无测试包、零FAIL；tool355.713s/types33.740s/repl52.222s。未加run/skip筛选；既有平台条件skip不冒充实机验证。原生干净构建和恰好双例生产回放均基于同一`04f4103bf3ad`代码，末版之后本批只有文档改动。

实现`04f4103bf`及人工审计/任务状态`ee9575111`已推送main；推送后工作树干净，HEAD/远端0/0。本收据更新仅文档；不以全仓绿代替尚未通过的模型答案验收。

## 29. 优先修前期人工FAIL：IO数值口径与明细覆盖（2026-09-20）

起点`a045e455e04c`，fetch后main与远端0/0、工作树干净。§28双例机器2/2、人工0/2的历史结论不改。本批先收两个有源码与公开路径反例的显示/上下文缺陷，不把正确数据已经到达后的错答一概归为模型波动；HMC-02.4已接受业务实例焦点与自动补齐仍单独开放。

### 29.1 IO折叠携带数值口径，不改变计价或因果资格

真实四视图`window_stats/wakeup_chain/root_cause_rank/critical_blocking_calls`经公开查询编译投影复现：背景请求实测47ms，排序影响17.850ms，后者在折叠说明中被固定的“完成端到端”类型标签误说成另一次物理测量。有效公开行为RED为`/tmp/hmc-io-fold-public-red-20260920.log`；早期测试缺视图、未形成真实折叠的失败不当作产品反例。

修复复用现有单源数值回退链，折叠载体同时保留值与来源；原生请求继续使用原有层级口径，root-cause发布的影响值标作“排序影响（非实测耗时）”，累计/有效/实际状态回退值分别披露。合并键由类型细化为类型＋口径，避免同一IO类型的实测值与排序值再次共用一把尺；保留出现顺序、各值与证据引用，不求和、不把影响值替换为物理耗时。评分/计数家族仍沿原注册表非墙钟披露。英文混入排序值时使用reports而非measured；原纯测量表达保留。

同一函数用于链上、自身、邻近、背景及明细镜像；不修改节点排序、选举、链上准入、原始Observation或投影值。回归覆盖三位置×中英、同类型异口径、多级回退、评分/计数、来源不变与真实查询重放。扩展定向通过`/tmp/hmc-io-fold-caliber-expanded-20260920.log`。后备测试初版误用非注册评分类型，已改为真实`block_io_by_inode`；同时发现的英文“measured ranking impact”自矛盾已修，保留该首跑记录，不称所有首跑失败都是旧生产缺陷。

### 29.2 总体、明细、捕获和两个时间范围分别交付

复核参考仓`core/preprocess/io_ops.py:49`：分布基于全部有效latencies，`top_latency_events`另选Top-N、仅作调试明细。`config/skills/io_analysis.yaml:119`单独查询时延分布。本仓沿已有配对/分布内核，吸收“总体与明细分离”的结构，不移植参考仓阈值定责或跨层合并。

显示层共享解释精确的已接纳完整配对总体N、原生查询所选明细E和超出明细上限H。H不叫配对失败；缺完成、歧义仍必须有各自计数，不能从N−E推导。E也不冒充模型最终看到的行数：已有零时长/无提交者/展示上限等过滤可使后续行数更小，原有过滤本批不改。生产者明确发布H=0，历史缺字段仍未知；缺失、负数、溢出或相互矛盾的计数不拼出肯定总体，原始记录不删除。

finalizer明细行将原QueryScope中的选择窗/行范围与事件实际出现区间分列；旧记录无query坐标就标未知，不借事件首末反推。关系桥接只可合并同一结果收据的重复发布，不把相同计数的不同来源、代次、目标、窗口或行窗混作一个总体；多收据保持各行原范围，不选择第一条授全局权威。共享短句放在摘要前部保住截断后的含义，完整解释留在查询/桥接，避免每条行反复堆教学。

新增公开工具→TurnA→finalizer及同ID紧凑语义上下文回归，验证17/8/9与三组分布都到达；另钉原生E=2而Observation仅1行的边界。不扫描用户/模型原文，不改变成文硬门或扩大重试。

### 29.3 验收与剩余范围

实现固定为`49f971733a05`，独立复核未发现必修遗漏。coverage末版三包定向通过（types1.127s、agent2.144s、tool1.673s，`/tmp/hmc-io-detail-context-green-final-20260920.log`），race×3通过2.744s/3.573s/4.169s（`/tmp/hmc-io-detail-context-race-20260920.log`）；fold及相邻IO路径race×3通过tool6.460s（`/tmp/hmc-io-rulers-fold-race-20260920.log`）。coverage有效行为RED另留`/tmp/hmc-io-detail-context-red-20260920.log`。干净构建通过`/tmp/hmc-io-rulers-build-20260920.log`，版本实测`49f971733a05`、无dirty。

末版全仓`/tmp/hmc-io-rulers-final-full-20260920.log`exit0：87个测试包通过（41缓存、46实际重跑）、13个无测试包、零FAIL；agent76.631s/tool370.318s/types45.054s/tracequery96.641s/tracediag5.669s。未加run/skip筛选，平台条件skip不冒充实机验证。恰好2并行×1生产回放已结束，固定构建、未改case/oracle，结果根`eval/results/hmc_io_rulers_replay_20260920`：机器0/2、人工0/2。业务328秒、分布671秒；[逐例审计](../../eval/parallel_selected_summary_hmc_io_rulers_replay_20260920_manual_audit.md)保留全部失败原因及过程。实现49f971733已推送main，不以代码回归绿代销答案FAIL。

业务仍把52ms全段账户套入50ms业务窗口，缺35ms请求时长和因果投影；分布三组八值正确，但结构表缺columns、RQ/BIO端点解释及跨层/总体判断错误。业务本轮无折叠投影，两例profile均无io_latency，专门IO范围桥接未进入最终上下文，因此本批live不充作两条新显示路径的命中证明。分布实际8次completion＝5次硬拒绝＋1次DOWNGRADED＋2次接受，日志重复计数10不当10次硬拒绝；第二dispatch错误手算999ms早于后续修补，最终数表仍取原生正确值。两份必选旁路都正常生成schema2空结果，纯事实合同未激活不等于文件丢失。

下一批先补宽泛count_or_duration分类下已查询IO事实的计量上下文，再推进已接受业务焦点/原子补齐，随后恢复mixed-source读＋Python写apply；不能修改family硬门或自动铸造因果意图，不能用本批Trace双例代替跨模式验证。另留P1观察：预处理模型提取的MetaDuration/Frame时长被下游描述为直接定量观测，当前例子数值恰好正确，未取得错答因果复现，不宣称已修。本批不动显式用户窗、链上业务线索、S/D实际IO阻塞、根因旁路、活跃流及600/300/600秒等待。稳定清单仍13/79交付、66开放；本批仅属于HMC-08.1/16.4/16.5中的子缺陷，不替整个父任务销账。

### 29.4 下一批HMC-02.4实施接缝（只读设计，未实施）

为避免下一轮重查，独立审计并核对真实代码，细分退出条件：

- [ ] completion新增可选业务实例引用，只收已发布token，不接受来源/TID/窗口的独立副本；复用`TraceBusinessSpanRefCurrent`的原物理收据及本轮代次。没有选择不自动取首个/最长，不从completion散文推选择。
- [ ] `EmitInvestigationComplete.Execute`全部拒绝/降级之后，才将pending焦点与accepted generation同锁写入；`Success=true`还可能是DOWNGRADED，不能当成功准入。后续accepted completion没有选择时，明确清掉旧焦点，不能借retained reason复活。
- [ ] 并行分支的工具保留与焦点准入分开。`MergeExploreForkPublishedTools`允许保留已完成事实和导航票据，不可保留败方/取消方执行焦点。特别是现有完整`MergeExploreFork`发生在检查worker错误之前，不能仅凭发生了该merge就签焦点成功：须有本fork新接受的completion、worker成功、有效输出且父上下文未取消。此为新接线必须保护的边界，不把现有closure行为未经复现另记成事故。
- [ ] ResetTurnA、ResetInvestigationComplete、explore重新打开时使旧选择失效；序列化/TurnA可有展示镜像，但不可从它还原执行权限。多个成功分支给出不同实例时明确冲突，不按数组顺序或最后写入静默胜出。
- [ ] `RunTraceQuerySystemSupplement`目前分三处选source/target/window；有有效accepted引用时使用`{view,business_span_ref}`原子消费，复用公开查询前后来源校验。无选择沿旧通道，显式用户范围/目标仍优先，不能拼接一半引用一半用户窗口，也不自动新增因果意图。
- [ ] 家族缺失检测须限同物理来源代次、TID及完整实例窗，不能让旧51ms结果抑制50ms业务窗补齐。保留现有view选择、预算、补齐结果专槽和编排hook，L1读调度字节红线不动。

实施正反矩阵必须包括串行接受/拒绝/降级、接受后worker失败/取消、并行败方工具可见而焦点不可见、不同成功选择冲突、缺选择清除、旧代次/换inode/JSON重放、用户显式窄窗/多窗/全域约束、不同源和旧窗结果不能满足本实例家族、无引用旧路径不变。以上均是下一批任务，不是本批已实现的功能。

## 30. 继续旧FAIL：修补教学同源与宽分类测量释义（2026-09-20）

§29人工0/2结论已随`1afd25a50`推送main，不改旧结果、不追加同版本第三例。继续处理两个确定性接缝，均不修改case/oracle、模型意图、根因资格、数值内核或重试次数。

### 30.1 容量修补教学与初始规范矛盾（已复现，局部修复）

B1640b原修复只覆盖`aggregate_facts`初始schema：要求scalar保持scalar、category保持category，grouped_count仅用于已核实的非负整数计数。但公开completion超量拒绝仍要求“per-group scalars into ONE grouped_count”；optional压缩说明也仍泛称任意同族fact改grouped_count。该错教在§29分布例2820真实到达，属于同一字段语义的多处手抄漂移；不是新证明的合法tuple同时必带/必拒，也不是999ms手算的起因（手算早于该拒绝）。

公开`EmitInvestigationComplete.Execute`对两条车道均先红，收据`/tmp/hmc-cap-teaching-red-20260920.log`。将初始schema已存在的保类型段抽为单源，初始规范、容量拒绝和optional压缩说明共同消费，schema解码后的承诺不变。保留cap=16、principal超量拒绝、原optional压缩与披露、修错不删事实提示；不把多个标量改成整数计数，不提升上限或新增类型去追单例绿。旧routing测试只替换错误合并教学的预期，数量/角色/硬拒绝断言保留；新增公开正反测试验证保留测量值12.5ms及原标量类型。局部回归通过`/tmp/hmc-cap-teaching-green-20260920.log`，完整收据后补。

表格列名已有明确教学：结构化多列表必须提供columns及一致的cells，只有真正两列才用无columns回退。本例模型未遵循已到达教学，当前不叠加另一条规则、不根据数值位置猜表头，也不为此引入成文拒绝循环。容量承载所有主事实的上限设计和member_set成员粒度教学另留账，不因本片提示一致就声称全部修补债清零。

### 30.2 已查询IO测量释义的纯展示补位（实现已交付，live命中待验）

§29分布例接受的profile只有count_or_duration，原生三组统计已到达，但受family选择控制的专用明细范围释义没有到达。仅在已有原生IO观察上补说明，不替模型新增io_latency请求，不修改共享family匹配函数或借新说明触发补齐/根因硬门；不调用可跨族联合状态账的完整bridge。已有IO专用/causal车道继续原处理，避免重复。公开查询→TurnA→最终上下文已先红后绿，逐条保留物理来源、查询窗/行窗、目标、结果收据、事件实际区间及计数未知性；来源/结果代次/窗口/行范围/目标不同，即使record ID相同也不合并其总体。未新增字段schema或第二个统计内核。

7个顶层测试覆盖公开消息、请求/观察字节不变、伪producer/非原生来源、已有IO及causal车道无重复、显式窗外不回流、7类收据区分、未知/矛盾计数、RQ/BIO端点、已证S态31ms与请求35ms分列、未知来源/查询不从散文推断。测试初版的显式范围缺SourceQuote，按既有合同并未生效，补全fixture后验证，不以其失败另报生产范围漏洞。实现固定`02cea94edaa4`，干净构建通过`/tmp/hmc-teaching-rulers-build-20260920.log`。末版新IO及相邻路径/活跃SSE的agent race×3通过4.786s（`/tmp/hmc-queried-io-final-race-20260920.log`）；cap race×3通过3.259s（`/tmp/hmc-cap-teaching-race-20260920.log`）。独立审查cap三文件确认仅消息单源化，准入与normalizer分支未变。

LLM等待保护定向回归通过17.992s（`/tmp/hmc-stream-wait-preservation-20260920.log`），包括默认600/300/600秒及活跃字节/思考/工具参数流不按请求年龄提前降级。末版全仓`/tmp/hmc-teaching-rulers-final-full-20260920.log` exit0：87个测试包通过（64缓存、23实际重跑），13个无测试包，零FAIL；agent86.867s/tool381.335s/types31.684s/tracequery106.995s/tracediag7.060s/orchestrator20.241s。tool/types/tracequery/tracediag完整包另测通过（`/tmp/hmc-teaching-rulers-packages-20260920.log`）。实现`02cea94edaa4`已推送main。

同一干净02cea构建的异构固定2并行×1已结束（结果根`eval/results/hmc_context_teaching_crossmode_20260920`）：Trace＋当前源码解释383秒机器PASS/人工FAIL，Python症状驱动写apply576秒机器FAIL/人工FAIL。两者均未触发本批IO展示补位或容量超量拒绝，不能当成这两处的live命中证明；此前人工FAIL与66项开放不变。详见[逐例收据](../../eval/parallel_selected_summary_hmc_context_teaching_crossmode_20260920_manual_audit.md)。

### 30.3 跨模式回放新增接缝（开放，先修系统补证路由）

读例正确交付86.111ms与实际端点、来源和线程；无已证链候选时必选根因旁路生成schema2空结果正确，无图、零成文重试。人工FAIL是把普通marker入口与精确注释旁路拼成串行路径、将时序校验误说成配对/时长生产，并以解析器存在推当前业务仍有历史风险。相应禁止混接和不足以判断出口已到达模型，不记成新合同互斥，不加散文关键词硬门。60fps只是条件参照，不能升级为trace已证刷新率；最终没有直接肯定掉帧，不夸大故障。源码来源和生产者路径的准确解释仍待异构验证。

写例初次补丁正确将整数值float正规化为int，原有4个测试与plain Python probe均绿，原始回归测试未修改。但`contract_refs`只说明探针意图、不授逐合同断言凭证；规划器收到明确教学后仍漏交已有原生测试的`project_test_observations`，故`float_type_check`缺证保护合法。已应用补丁保留于eval临时仓的`refs/codrax/applied/plan-1789900623764734000-91719`，最终流程blocked，不能以局部测试绿签完整交付。

- [x] **WRITE-PROOF-IDENTITY（P1，确定性子缺陷已修，非整例销账）**：已就绪的controller补证批允许下一次`plan_batch`换ID，既有元数据保真只保同ID；新ID成为普通批，PTO-only先被拒，保留了probes的空changes仍因补证身份丢失被拒。随后模型被引向重复改动已正确源码，直到重复路径拒绝。调度及持久化两层现消费同一`PendingControllerProofPlanBatch`，保原ID/目的/范围/依赖，不放宽空计划门或通过原文推权限。两种purpose的直接Apply和公开normalizer→Apply→emit计划链行为先红后绿，详见§30.4；能力不匹配仍是独立开放项。
- [ ] **WRITE-PROOF-CAPABILITY（P1，独立系统缺陷，部分实施）**：即便保住ID，原补证桥只查解释器存在，却要求plain Python probe解决缺失的逐合同行为证明；执行器不能产该证明，旧source-free sentinel又合法禁止补PTO。因此身份修复不能代销能力路由。§30.5先阻止确定不可产证明的派发，自动补登记既有断言仍开放：已有精确凭证复用、已有声明缺执行则精确重跑、缺声明但有原生断言则只读绑定后重跑、仅plain probe只补目标执行不承诺合同证明。

能力补证设计：优先复用既有PTO精确文件/suite/assertion执行器，新增与`proof_probe_only`平行的只读原生断言补证形，不修改旧source plan/审批fingerprint/历史报告。controller授权须绑定run/batch、已应用source plan、工作树快照及active required合同；只收已存在测试、精确PTO，无任何文件编辑，不强制再造probe。系统读取并绑定测试字节哈希，执行前后复核，计划/报告独立留存，不能给历史aggregate pass补签。未授权/跨仓跨批/退役合同/文件改变/skip/错误suite或assertion/非零执行均不授证。累计scope/resume/JSON往返和同快照同绑定不重复回圈为必测边界。该设计尚未实施，不提前宣称写模式闭环。

另记低优先级合同教学问题：`raises.expected`被写成复合中文，精确异常值在comparator；不是本次缺证直接原因。应通过同源原子化教学让异常合同保`ValueError`、转换结果另列，不从散文自动拆合同或松凭证门。

### 30.4 WRITE-PROOF-IDENTITY实施收据

`writeflow.PendingControllerProofPlanBatch`以当前活动批、ready_to_plan、空PlanID、非verify_only、已有补证进度记录、精确补证purpose与required标志为条件，返回独立复制的原批计划封装。scheduler在finish和plan_batch使用，持久化Apply仅在plan_batch使用；两者单源，防止运行状态保住但planner仍收到错误范围。直接新增/拆分/重规划动作、已完成/已建计划/普通批不被劫持，不授新文件编辑或新行为凭证。

两种purpose×异/同/空ID回显、11项边界、独立拷贝、append/split/replan/block及公开成文计划入口已验证；planner提示仍限原路径，公开probe-only计划保来源范围且没有凭空生成验证报告。测试fixture修正了既有正规化对空切片、CreatedAt的合法初始化，未改变生产时间戳或正规化逻辑。RED为`/tmp/hmc-proof-identity-red-20260920.log`，GREEN为`/tmp/hmc-proof-identity-green-20260920.log`（writeflow0.697s、orchestrator0.898s）；两完整包通过0.425s/17.319s（`/tmp/hmc-proof-identity-packages-20260920.log`）；新路径及相邻proof/planner回归race×3通过1.623s/2.439s（`/tmp/hmc-proof-identity-race-20260920.log`）。独立审查未发现生产放权或既有正常路由回退。本片没有另起live回放：已知能力缺口未修完，不能只修ID便以同例追绿。

授权边界说明：本片复用既有run级`verification_proof_followup_requested`记录判断，并未新建绑定到批次血缘/工作树代次的授权凭证；不得宣称逐批精确授权已闭环。§30.3下一片既有原生断言补证需另建绑定run/batch/快照的授权，避免把历史同run记录当新补证权限。稳定任务总数仍13/79交付、66开放，WRITE-PROOF-IDENTITY只是HMC-18.5中的一个子缺陷。

### 30.5 WRITE-PROOF-CAPABILITY第一片：不派无法产出所需证明的探测（已交付，父项仍开放）

身份修复已以`ff94da1a7`推送。继续将“解释器存在”与“能产逐合同见证”分开：`VerificationProbeUsesExecutionOnlyWitness`由实际执行收据解析、有效凭证投影及派发共同消费，保持原证明权威不变。现有execution-only运行器当前是Python plain probe；这是生产者能力事实，不按客户问题、类名、源码/输出字符串或某个eval症状决定。

桥接仅当剩余待补全部是已知active required运行时behavior/placement断言、没有对应既有原生断言声明、所有目标可用inline运行器都只有执行证明时，不再强制额外probe计划。changed-target执行债、混合债、已有/累计原生PTO、非execution-only运行器、file_layout独立源码见证、未知合同/目标/运行器均保原路。此函数不写plan、report或账本；仍缺的合同仍缺，后续沿既有accept_unverified车道披露，不误签all_verified。并非把FAIL改PASS，也不是把任意source-free计划放开。

真实bridge入口回归先红（`/tmp/hmc-proof-witness-bridge-red-20260920.log`）：项目测试通过、目标覆盖已有、只有一个异常合同无绑定时仍派mandatory probe。修后入口GREEN（`/tmp/hmc-proof-witness-bridge-green-20260920.log`，0.931s），并验证真实normalizer不会在后续分支重新派发、不把缺证账本升级；已有PTO继续保留原恢复路径。另有20个路由矩阵案例及运行器能力/收据一致性测试；单元早期修正file_layout与未知合同过度抑制，未将未知当确定不支持。

完整orchestrator包随后发现既有`TestVerificationProofProbePlanningRebindsOnlyNonAuthoritativeFailedProbe`失败（`/tmp/hmc-proof-routing-orchestrator-20260920.log`）：新守卫只看待补obligation，忽略了非权威探测自身的failed capability仍可被修正。没有改旧测试或豁免断言；收窄为FailedCount/CapabilityFailedCount为零才可能抑制。保留失败探测修正能力，成功后仍剩不可产断言时才止损。末版完整orchestrator包通过18.430s（`/tmp/hmc-proof-routing-orchestrator-final-20260920.log`）；含该旧反例的新/邻接路径race×3通过types2.290s/orchestrator2.916s（`/tmp/hmc-proof-routing-final-race-20260920.log`）。第一轮全仓`/tmp/hmc-proof-routing-final-full-20260920.log`有此已定位失败，保留exit1，不充作绿收据。

修正后冻结生产提交`bc90f5aaae0b`，干净构建通过`/tmp/hmc-proof-routing-build-20260920.log`，实测二进制revision相同且无dirty。末版全仓`/tmp/hmc-proof-routing-final2-full-20260920.log` exit0：87个测试包通过（71缓存、16实际重跑）、13个无测试包、零FAIL；agent63.814s/orchestrator15.449s/tool345.249s/types32.406s/tracequery93.080s/tracediag5.505s。未加run/skip筛选，平台条件skip不作实机验收。代码`bc90f5aaa`已推送main；此次只交付B1，不补签此前Python写例。

独立开放施工表（不因止损已修而整体打勾）：

- [x] B1（实现已交付）：派发与实际执行器权威单源，精确不可能的assertion-only探测不再强制派发；缺证不销账，末版全仓/race/干净构建通过，`bc90f5aaa`已推送。
- [ ] B2：controller-only原生断言补登记授权，绑定run/batch/已应用计划与当前工作树快照；新增原生登记不得借run级历史记录授权。§68已实测区分现有probe重新执行与复用旧证明，不能据历史事件仍允许新probe就宣称当前存在假证明漏洞。
- [ ] B3：平行于probe-only的新只读补证计划；仅接已读、已存在测试的exact PTO，系统哈希锁定字节，不准改源或改测试oracle，模型不得自填执行收据。
- [ ] B4：直接进入verify-only，复用精确test_path/suite/assertion执行器，新报告独立留存；原plan/approval fingerprint/旧report不改，失败、skip、身份错配都不授证。
- [ ] B5：累计scope/JSON往返/resume及快照改变失效；相同快照/合同/绑定/证明方式不原样无限重试，有新精确证据才允许修正绑定。
- [ ] B6：公开流程红绿及固定两例生产回放；本次4原生测试＋plain probe绿仍缺证为负臂，补正确原生绑定＋重新执行才可绿，其他失败不能被覆盖抹掉。

下一片代码接点已只读核实（尚未实施）：

1. 发射面须同时覆盖`tool/emit_change_plan.go`与`emit_plan_skeleton.go`的空changes分支，复用PTO正规化及合同ref校验；旧probe sentinel仍不能携带PTO。
2. `types/change_plan.go`的WritePlanToFile、WriteBestPlanReportPair、LoadBestPlanReportPair、LoadChangePlanFromFile及UpdatePlanStatusOnDiskWithApplied共同保留新只读计划身份；加载不得拒载或重算成空TargetPaths。REPL PlanStore.Load/Settle、编排状态持久化入口同批覆盖。
3. scheduler的runControllerPlanBatch、proof-only→verify-only提升、pending verify、dispatch失败恢复、probe required提示和PendingControllerProofPlanBatch按typed证明方法分流，不能只补发射工具而漏恢复通道。
4. 跳过批准/应用而直接验证的新形必须有精确controller授权；stage_hooks的空changes不可apply红线保持，无修改计划绝不靠伪造source commit完成。
5. 原生执行继续复用test_surface的精确文件路由、run_tests的projectTestObservationConfidenceRecords及projectTestObservationExecutionMatches；不得从声明直接产通过凭证。
6. 累计scope目前主要汇总仍贡献源码的applied plans。新只读补证计划不伪装成已应用源码，但其PTO和独立回执须在后续轮次、final report、artifact load与最终materialization保住；writeFinalMaterializationStrictProofOnly及syncMutablePlanStatusAfterVerify要认识平行新形。这些边界缺一项都不能关闭B2–B6。

B2独立复核补充（设计边界，不是已实现）：现有请求事件实际写在来源批次，probe-planning bridge还借用了更早的run级记录，不能机械改成`event.BatchID == activeBatch.ID`。新的controller-only grant需要在普通post-verify、cumulative及bridge三处单源铸造，绑定目标批次与实际仍贡献源码的source plan集合；不能以synthetic cumulative plan代替。防循环的run级计数仍是预算，不与授权合并。

必须拆分持久批次归属和当前可消费授权：归属锁定purpose、范围及禁止无失败源码改动的边界；snapshot过期只禁止新补证声明/ref扩充，不能使`activeProofFollowupWorkflowBatch`返回“普通批次”而跳过`validatePureProofFollowupChanges`。快照应由生产者读取仓库身份、HEAD及绑定源码字节/缺失/文件类型；现有read-mode HEAD＋status摘要无法识别同dirty路径字节变化，现有verification snapshot也缺HEAD和untracked字节，二者均不能原样升级成该硬授权。旧只读verify可继续；旧grant缺失或快照改变必须重新观察后mint，显式resume令牌不直接补签。新增PTO的测试文件字节由后续B3/B4另绑，B2不冒称已覆盖。

追加正反矩阵：同run其它批、其它run同名批、source plan替换/恢复剔除、同HEAD/status但字节改变、快照不可读时禁改门仍生效、JSON/同快照resume/worktree重建、旧verify-only及普通impact路径，以及模型改ID/purpose/criteria或提交grant字段不得扩权。以上仍归原B2–B6，不新增“已交付”计数。

### 30.6 固定双例结束：普通计划通过，IO入口缺陷仍失败

干净构建`bc90f5aaae0b`，固定快照`.codrax/tmp/codrax-selected-20260920-042031`，2并行×1、每例1800秒：`trace_query_io_request_latency_distribution`＋`patch_cpp_typo`。结果根`eval/results/hmc_witness_routing_io_plan_20260920`。[机器摘要](../../eval/parallel_selected_summary_hmc_witness_routing_io_plan_20260920.md)1/2、[人工审计](../../eval/parallel_selected_summary_hmc_witness_routing_io_plan_20260920_manual_audit.md)1/2。IO382秒/30%上下文FAIL；C++52秒/28%上下文PASS（只计划）。不回写旧结果，不追加第三例。

IO本轮profile确有io_latency，但工具接纳未知`view=storage_latency_by_layer`后静默执行为event_search，没有真正算分布；宽分类helper不会进入，专用桥接也无原生分布可消费。因此不是02cea纯展示说明已到达后仍失败。答案误写8/8/0、零缺端/歧义及两层包含关系；最终一次成文、无JSON恢复或活跃流降级，必选schema2空根因旁路正常。先修§31明确入口缺陷，不把所有错误都归模型波动。

C++只改main.cpp一行，实际patch应用检查通过，原仓HEAD/源码不变；零修补，普通计划没被补证批身份锁劫持。没有执行apply/verify，不能据此验收B1实际触发或B2–B6原生断言能力。其验收散文未写空输入world回退的P2观察仍保留。稳定任务总数仍13/79交付、66开放。

## 31. 继续IO旧FAIL：未知视图静默成功与重复测量教学（2026-09-20，实现已交付，答案留债）

- [x] **HMC-01.1/01.2、08.1子缺陷：未知查询视图**。真实日志1306传入storage_latency_by_layer、1312工具success，payload却是event_search。沿CanonicalViewNames/CanonicalViewName现有注册表做同源明确校验及可修补指引，不新增样例别名，不扫用户或答案原文。公开工具/engine先红后绿，stream及tracediag入口、合法alias、空默认、显式窗和自动补齐同批核对；禁止未知值偷偷落到默认事件搜索。实现`99556ae4b3da`已推送。
- [x] **HMC-01.2/16.4子缺陷：重复原生测量的容量教学**。公开3组×8值→无aggregate重复抄写的completion本来就能通过且原生数据不变，故证伪“24值必须逐项塞cap16”的硬合同互斥。但cap拒绝声称删除任一条目都会永久丢值、初始描述泛称测量都入aggregate，容易引导重复。只修同源条件教学：已有原生同范围测量不需重复复制；唯一主事实仍不可丢或改audit角色绕cap，bounded上下文不保证全量显示。cap16、类型、normalizer与准入不改。公开教学RED已留`/tmp/hmc-native-measurement-teaching-red-20260920.log`，完整收据见§31.4；实现`62a285926`已推送。

这两处不替代B2–B6、accepted业务焦点或IO跨层完整总体能力。§30.6没触发cap超量拒绝，不作为教学修复live命中证据。仅上述两个确定性子缺陷验收，不替父任务或旧答案销账。

### 31.1 未知视图的执行边界及教学（实现99556ae4b3da，已推送）

公开工具在既有JSON/alias正规化之后、业务引用解析/材料准备/窗口及目标登记/memo之前，从现有capacity注册表校验view。未知值返回精确可修补参数错误、完整合法值和保持显式窗/业务引用的提示；不执行替代查询、不发布原生观察、覆盖凭证或补齐种子。`Run`及三个stream入口也使用同一闭集；直接对indexed `Run`调用仅流式支持的window_sweep不再掉到搜索，而是明确要求对应stream入口。公共window_sweep仍走原流式分派，合法heavy-view容量降级不变。没有把storage_latency_by_layer补成新别名，也没有增加用户/答案散文硬门。

默认空白、全部schema aliases、显式时间窗、业务原子引用、自动补齐和取消矩阵通过。三个stream入口的实际生产调用无未登记私有view；tracediag脚本已有闭集，未重复改动。unknown-view公开/engine RED为`/tmp/hmc-unknown-view-red-20260920.log`，GREEN矩阵为`/tmp/hmc-unknown-view-matrix-20260920.log`（tool3.119s/tracequery0.564s/tracediag0.826s）。另有公开副作用隔离反例：不存在来源、过期引用或重复请求都不得先准备材料或登记目标。engine保留显式零起点；初步看到的tool零值省略仅限短banner。新增公开decoder→event_search原生inventory/window_stats原生窗口正例确认0..14仍保显式Set与范围（`/tmp/hmc-public-zero-window-check-20260920.log`，1.711s），证伪“执行窗口丢了零起点”；没有据此修改数值内核或把显示省略报成已修执行缺陷。

IO统计教学也有独立入口缺口：原常量只进入skill表、没有进入直接工具面，且只写输出字段未直说父view。现在同一常量明确先调用`view="window_stats"`，再读取`window_stats.storage_latency_by_layer[].request_latency_distribution`，输出字段不是view；skill表、工具Description和view参数说明共同消费，后两面各一次。教学RED→GREEN见`/tmp/hmc-io-parent-view-teaching-red-20260920.log`、`/tmp/hmc-io-parent-view-teaching-green-20260920.log`（0.865s）；直接工具面测试通过`/tmp/hmc-unknown-view-tool-teaching-20260920.log`（1.119s）。仅追加Description末端合同，原前缀字节恒等；按既有演进规则保留先红的字节pin、记录意图后更新golden，不改任何排序/因果数值。h2/h3匹配A/B验收债仍开放，不由本批IO例替代。

### 31.2 原生测量与摘要唯一事实分开（实现62a285926，已推送）

真实公开window_stats给出3组×8值，completion不再复制aggregate仍通过的路径，在修复前已绿；因此不是必须承载24项却cap16的硬互斥。此次仅修改共享教学和唯一事实丢失披露：同源/查询收据/目标窗/测量身份/数值单位的原生数据无需重复抄写，原始散文、推算或其它范围不能代替；唯一principal事实/member_sets仍应交接，不得改audit角色绕cap，bounded展示不承诺全部可见。

新公开测试确认原生24值及TurnA字节不变、不自动伪造aggregate；17个唯一principal事实仍精确拒绝。原有cap16、类型、正常化和准入逻辑均未动。初版race发现旧EMITBURN披露词针不合，保留既有测试，修补条件教学并保留唯一丢失时的`name the dropped entry in reason`要求。末版race×3通过tool4.593s/agent3.933s（`/tmp/hmc-native-measurement-teaching-race-final-20260920.log`）。全部>16唯一模型派生标量如何承载不是本片已解决的问题，不能把去重教学当无上限保证。

独立复审未发现新硬门误伤合法补齐或明确窗口；完整回归及新版本固定双例收据见§31.3/31.4。稳定79项清单仍13已交付、66开放，历史人工FAIL不倒签。

### 31.3 新版本固定双例：机器2/2，人工1/2

固定干净实现`99556ae4b3da`（含重复测量教学`62a285926`），原生构建`/tmp/hmc-unknown-view-build-20260920.log`，2并行×1、每例1800秒。IO272秒/30%上下文，Python写153秒/28%；[机器摘要](../../eval/parallel_selected_summary_hmc_unknown_view_io_write_20260920.md)2/2 PASS，[人工审计](../../eval/parallel_selected_summary_hmc_unknown_view_io_write_20260920_manual_audit.md)1/2 PASS，没有追加第三例或改oracle。

IO这次正确调用window_stats，三组24值及主窗全部正确，未知view修补未触发；原生17准入/8明细/9超明细、完整分布与RQ/BIO端点限制均到达finalizer。答案仍把明细/另一次宽窗搜索截断当统计不足，并将RQ issue→complete扩成应用提交全路径、BIO queue→complete扩成调度器内部排队。正确上下文未遵循，不记系统数据丢失，也不为这类语义错答另造散文扫描门。completion三次中一次缺成员集降格，最终安全JSON恢复后一次成文；cap拒绝分支没命中。schema2空根因旁路正常，无根因合同不等于应有投影丢失。

Python只改源文件，保原4测试，独立复测及类型/负值/零值/非整数扩展检查均过；应用提交可恢复、原仓HEAD未变，普通apply交付人工PASS。但本轮12个行为合同均planning_only_ungrounded、required=0，且初始已有4个PTO声明，没有旧FAIL的active required补证债，不能借此关闭B1 live命中或B2–B6。首个计划有空path条目被精确拒绝后一次修复，机器reject=0不是零规划拒绝。

新确定性小缺口另批处理：报告70行系统通用覆盖提醒向明确“不分析代码”的请求追加“结合源码进一步核对相关组件”；不是模型blocks，属于共享模板无依据指定来源。仅需将通用提醒改为“相关证据”，中英同源，不按原文关键词分流、不改变覆盖义务/拒绝/重试/精确facet提示。该修复不替IO解释FAIL销账。

### 31.4 末版验收与交付收据

冻结实现99556ae4b3da的第二轮完整回归`/tmp/hmc-unknown-view-final2-full-20260920.log`退出0：87个测试包通过（61缓存、26实际重跑）、13个无测试包、零FAIL；tool363.296s、tracequery99.234s、types34.100s。未用筛选跳过失败测试；平台条件skip不当实机覆盖。第一轮完整回归亦87包通过，但跨用户续办后运行会话不可恢复，采用本次可确认退出的第二轮为交付收据。

闭集视图、业务引用、显式窗、取消、自动补齐及原生测量定向race×3通过skill2.080s/tool50.726s/tracequery3.795s（`/tmp/hmc-unknown-view-final-race-20260920.log`）。工具字节pin先红在byte32363，仅追加末段教学；按既有更新入口重钉后原prefix/排序替换臂与同源教学测试通过1.006s（`/tmp/hmc-unknown-view-description-pin-green-20260920.log`）。新增零起点公共正例另通过，未据此改生产窗口。远程main已确认包含62a285926及99556ae4b，未知视图和重复原生测量教学两片收住；§31.3 IO人工FAIL、B2–B6及h2/h3 A/B继续开放。

### 31.5 来源中性的通用覆盖提醒（3aa5972d6已推送）

仅修改`CaveatFamilyAnswerCoverage`注册表两句：中文“建议结合相关证据进一步核对”，英文“cross-check the relevant evidence”。未新增source分类分支，不扫描输入或答案，也不修改覆盖义务、门限、重试、统计值或根因资格。精确current-code facet仍保留对应的代码路径提示；改的是没有具体来源凭证的通用模板。

新增真实公开materializer/user/soft/hard-cap residual及tracked重放测试，中英文别名、nil上下文、typed trace-only和源码场景共用模板，正文逐字保留；空违规不追加、仅遥测不展示、精确facet不降成通用提醒。RED见`/tmp/hmc-source-neutral-caveat-red-20260920.log`；GREEN types1.172s/orchestrator1.079s，完整两包45.811s/21.197s、定向race×3 3.219s/2.948s均退出0（同前缀green/packages/race日志）。本片没有另跑live第三例，不修改§31.3原报告或人工FAIL。代码3aa5972d6及审计63a6fc6a2已推送main。

## 32. HMC-02.4：已接受业务实例的补齐范围（2026-09-20，实现358f47f4c，旧FAIL保留）

继续§27/§29旧业务答案FAIL，不重新定义通过标准。旧查询引用只提供导航，不是最终选择；同名多实例、探索试窗或并行失败分支的已完成工具结果，都不能替模型选择答案范围。参考仓按对象下钻的组织方式仍沿本仓原子来源/TID/完整实例窗实现，不移植近邻/最长片段猜选。

- [x] **接受与执行成功分离**：completion顶层可选`business_span_ref`只收本轮已发布原物理引用。所有原准入之后同锁记录pending决定与completion代次；无选择明确清旧。worker成功、有效output、无错误且父上下文未取消后，独立hook才允许消费。系统收敛completion、失败后保留closure、工具事实保留都不自动获得新焦点权限。
- [x] **生命周期及并行**：Reset/reopen撤销；JSON/TurnA/散文不还原权限。传统fork合并不携新焦点；成功败方仅可对不同实例保守否决，不授选择，失败败方也不能用无选择状态否决。相同物理实例的不同随机token可等价，不同实例或明确无选择冲突时不按数组/完成顺序胜出。
- [x] **补齐原子消费**：没有用户范围/目标覆盖时只通过`{view,business_span_ref}`查询；预算和披露读取同一实例。明确用户单窗/多窗/全域、不同目标及目标冲突保持既有优先。已有家族必须来自同物理捕获代次、相同TID及完整实例窗，连±0.5/1微秒的真实邻窗也不能冒充，旧51ms不得抑制50ms补齐。源级census仍是独立全域库存，不冒充实例内因果证据。
- [x] **公开确定性回归**：schema、真实adapter消息、公开completion、串并行成功/错误/取消/冲突、显式用户窗、来源变更、统计与根因边界、零语义猜测；末版全仓退出0，见§32.3。
- [ ] **生产验收**：固定两例各一次，机器1/2、人工0/2；业务例未调用实例定位或提交选择，新焦点路径未命中，旧FAIL仍开放，见§32.2。

前置失败已留：completion schema缺字段的公共capability RED（`/tmp/hmc-business-focus-completion-red-20260920.log`）；types新能力的可编译scaffold行为RED（`/tmp/hmc-accepted-focus-types-red-20260920.log`，不伪称已发生生产事故）；原supplement对已接受引用仍no_attached_trace、陈腐/失败选择会沿旧探索窗执行的公开RED（`/tmp/hmc-business-focus-supplement-red-20260920.log`）。初版门保留fixture误用runtime源的pending-read旧合法豁免，已改成精确member_set义务反例，不据此报告新准入漏洞。后续完整live收据见§32.2，未达人工通过，不勾父任务，清单13/79与66开放保持。

### 32.1 实现和定向收据

新增权限只存私有状态：不把模型字段、JSON、旧closure或普通工具事实当执行成功证明。串行hook在字节保护read loop之外；并行的内部早收敛取消与用户/父上下文取消分开。仅TID没有时间窗时不能绕过Invalid/Conflict回退旧窗；有明确用户时间范围时仍按原用户通道。开始补齐后选择撤销，后续core与census停止，已成功的原生查询事实保留，沿原部分结果语义披露，不抹除已读事实。

实际最终链路`CompileObservationLedger → CompileTraceCausalProjectionSet → renderer`正针：先跑51/52ms探索窗，再接受完整50ms实例，最后账户仍50ms；原生ledger保35ms请求、31ms S睡眠阻塞及不可相加标记，投影链上IO为31ms，47ms独立备份仍在背景。不是只测参数窗或强行把请求35ms加进根因图。日志`/tmp/hmc-business-focus-final-projection-20260920.log`；最终断言在`TestTraceSupplementBusinessFocusFinalProjectionUsesAcceptedInstance`。

公开completion/schema/消息定向通过1.120s，tool/agent race×3分别4.825s/3.823s（`/tmp/hmc-business-focus-completion-green2-20260920.log`、`/tmp/hmc-business-focus-completion-message-race-20260920.log`）。types/orchestrator完整包48.010s/22.389s、最后本片+census race×3为2.185s/35.590s（`/tmp/hmc-accepted-focus-types-orchestrator-full-final-20260920.log`、`/tmp/hmc-accepted-focus-own-race-final-20260920.log`）。supplement公开矩阵1.659s，相关旧新回归6.124s、race×3 11.489s（`/tmp/hmc-business-focus-supplement-regression-20260920.log`、`/tmp/hmc-business-focus-supplement-race-20260920.log`）。独立复审确认上述微窗/仅TID/失败兄弟三边界修复，原closure恢复、重开保护和L1一致性仍通过。

中间失败保留：假worker没有构造真实TurnA导致测试nil访问；代理同时施工期间新test的`meta.Members`字段编译失败；均已修正测试，不把这些说成生产系统新增事故或首跑全绿。最终回归期间另核教学：query本身不接受焦点的旧否定句与新completion能力没有严格逻辑矛盾，但有孤立阅读歧义；补最小同源桥句，明确需另走顶层completion字段并等待接受与worker成功，不增加模型必填义务。

### 32.2 固定双例收据与未销账问题

固定358f47f4c011，2并行×1，业务367秒/42%上下文、明确窗187秒/45%；[机器摘要](../../eval/parallel_selected_summary_hmc_business_focus_20260920.md)1/2，[人工逐案审计](../../eval/parallel_selected_summary_hmc_business_focus_20260920_manual_audit.md)0/2，没有第三例追绿。业务仍缺35ms请求正文，投影仍53ms探索窗；本次未调用业务实例定位、completion未提交引用，自动补齐no_typed_target，所以不能宣称新接缝live验收完成。原生35/31/47数值到达finalizer，不是容量截断。明确20ms窗、11ms链上IO和链400→300→200→100保持，但可见答案仍有列表丢字和跨CPU/等待主体/请求口径误述。

优先修复的确定性系统项（归原HMC-02.4/18.4，不新增父项计数）：

- [ ] **背景排序量冒充实测**：47ms背景IO被内部排序限为53×35%=18.55ms；publication只修纯调度状态，资源行遗漏，handoff还将其称为measured，系统图与表也用18.55。保留排序/Score/因果隔离，按原生typed测量恢复发布口径；不得把累计、区间包络、count/index混算为实测。不能记模型波动。
- [x] **结构化列表正文静默丢失（确定性修复）**：合法字符串放在section.items[].cells，最终两组列表仅编号；共享渲染器已修，见§33。旧live报告与整份答案FAIL不倒签。
- [ ] **正文边界遵循**：同核首跳被概括为全部跨核，依赖方自身1ms被说成下游runnable，11ms调度IO-wait被说成缓存页加载。正确typed信息已存在，先核教学/共享上下文，不用一次模型错答扩硬门。

analyzer本次6次emit/5拒绝、finalizer2拒绝及JSON安全恢复均按实际日志留档，不能以机器ana=1当零重试；未发现同字段必带必拒的新证据。mandatory根因旁路、Trace投影及600/300/600秒等待保持，本轮没有活跃流因短暂未成文而提前降级。

### 32.3 冻结实现回归

原生构建`/tmp/hmc-business-focus-build-20260920.log`退出0，版本358f47f4c011。最终同源桥句补完后的全仓`/tmp/hmc-business-focus-final2-full-20260920.log`退出0：87包通过（72缓存/15实际）、13包无测试、零FAIL；agent71.656s、tool350.882s、tracequery90.893s、types30.074s、orchestrator18.217s。第一轮全仓也退出0（87通过、17缓存/70实际），但最终交付采用第二轮。schema真实消息桥定向agent1.178s/tool2.328s，日志`/tmp/hmc-business-focus-bridge-green-20260920.log`。

本片收住的是接受权限、生命周期与原子补齐接缝，不是旧答案FAIL。父任务不勾，13/79已交付、66开放不变；后续先根修§32.2确定性系统缺口，再做新固定版本双例回放。

## 33. 列表多个内容载体同时存在时不丢正文（2026-09-20）

§32明确窗报告的两组空列表已确定不是模型没写：模型给了label及cells，full/patch解码和持久化均完整，`renderV2BlockItem`却只在没有label/text时才显示cells。只改共享section/ordered_list/bullet_list渲染器这一条件，按原次序以中性分隔符追加已有字符串；重复不去重、空白不制造内容、引用仍在尾部，原结构不变。

不新增JSON必填、不改教学/schema/解码器，也不猜cells列名或把散文扫描作硬门。原full-only单字符串包装与stringified string-array恢复保持，数字/布尔/对象/嵌套数组仍拒绝；meaningful item碎片不静默丢弃，失败patch不覆盖原稿。表格等非列表分支不改。

真实full/patch→persist→render及三列表形RED见`/tmp/codrax-list-cells-MdNFud/red-confirmed.log`（render0.675s/tool1.169s），GREEN0.717s/1.148s，既有相关回归0.540s/1.999s（同目录green/regression）。主席独立整包render1.102s、render/tool race×3为1.756s/7.735s，`/tmp/hmc-list-cells-render-full-20260920.log`、`/tmp/hmc-list-cells-race-20260920.log`均退出0。接下来的全仓与新固定双例共同覆盖下一批发布修复；不回写旧报告、不把明确窗原人工FAIL改PASS。

实现及上述文档以`8a8ca1813`推送main。此片没有改变JSON接受条件或模型事实；最终联合全仓及新的固定双例收据归下一节，不冒称旧报告已经修好。

## 34. 背景原生计时与内部排序值分离（2026-09-20，子缺陷验收完成）

继续§32业务FAIL中确定性系统缺口：引擎为背景排序把47ms请求封顶为53×35%=18.55ms。B1717只恢复纯调度状态，资源行仍把排序量传到JSON、观察、系统投影和最终模型上下文，后者直接称measured。§29修了折叠文案不应冒称端到端，但没有恢复该发布量；本批补齐原生计时的传递，保留旧加权量的“排序影响”回退，不倒签历史FAIL。

### 34.1 精确生产者与口径边界

只读取五族原生封顶前的计时账：`io_latency/window_stats`、`irq_activity/window_stats.irq_activity`、`ipi_activity/window_stats.ipi_activity`、`workqueue_activity/window_stats.workqueue_activity`、`dma_fence_activity/window_stats.dma_fence_activity`。原生family键已包含type/producer/物理文件/线程/车道/查询窗，只有有限正值、合法单成员或已知多成员时间折叠可以恢复；多成员保持Σ互斥、去重并集或最大记录回退，不以区间包络或任意cumulative猜测。`window_stats.io_facet_family`等其它生产者不借名通过。

恢复只发生在background公开副本，`ImpactMs/ProjectedImpactMs`与原生计时一致、effective仍零；原引擎对象、Score、排序、链上31ms、请求35ms、用户明确窗及根因资格不变。计数、混合指数、CPU压力/供给权重、sched_stat、低频/亲和性、文件IO建议和blocking包络等没有直接可恢复的同类见证，继续原路径，不能宣称全域实测口径已修完。

新增注册表内展示标记`rank_value_caliber=native_duration`，仅精确闭集值从root-cause观察传入投影。未知值/非榜项不授标定，标记不参加排名或硬拒绝。IO折叠改称“观测计时”，不统一说“完整请求端到端”；count/index分类仍优先，旧未标定榜量继续称排序影响。最终模型上下文不再仅因unit=ms说measured，多记录计时复用既有单源家族说明，明确记录数不等于发生次数、最大记录回退尚未消解重叠。没有新增JSON必填、答案词扫描或系统代写结论。

### 34.2 已取得与待取得收据

公开原生RED：`/tmp/hmc-native-timing-red-20260920.log`，47→18.55、裁窗38→13.3、互斥67→35.35均复现。五族×三个真实榜项视图、同记录banner/wire/Observation、IO裁窗及无封顶、互斥/并集、真实workqueue交错包络的最大记录回退已绿；后者原生23ms/成员原始和45ms/排序18.55，发布23ms而非45ms或包络。最终定向及原B1717回归1.583s，`/tmp/hmc-native-timing-green-20260920.log`。IRQ/IPI初版fixture被原生sidecap挤掉，并非计时恢复失败；改为独立有完整调度的小fixture后实际公开正针通过，没有改变生产容量。

真实公开查询→TurnA→ledger→projection→BuildInitialInstruction分别覆盖53ms探索窗和50ms业务窗；background47ms、chain31ms及request35ms各保原尺、不相加。该路径RED见`/tmp/hmc-background-duration-finalizer-red-20260920.log`。初版registry/golden未登记和旧公开折叠测试预期17.85的失败均已保留；按字段演进协议补注册及真实producer发射针，不豁免测试。相同47ms原始/榜项记录可按既有规则提前去重，普通detail本来不显示主时长，测试据此检查树值及存在折叠时的附注，不为追第二条重复事实改生产折叠。

三包集成末版前序通过agent1.384s/types0.646s/tool1.412s（`/tmp/hmc-background-duration-integration-green3-20260920.log`）；registry0.982s通过。三包前序race×3为agent5.453s/tool11.523s/types4.056s（`/tmp/hmc-native-duration-final-race-20260920.log`），不冒称覆盖其后补的多成员交接。多成员在高显著性背景行丢口径另先红，`/tmp/hmc-native-duration-family-handoff-red-20260920.log`，现复用`FormatTraceFamilyMeasurement`，新绿/末版全仓/最终race及live另补，未取得前不勾交付。

独立复核进一步确认，IO family背景行也可能被既有同段展示折叠吸收：peer载体原来丢弃`FamilyMember*`，于是最大记录回退、并集与互斥和被拼成一个斜线数值组。现只透传三字段、复用同源家族说明区分显示组并附注；不改折入条件、排序、主行数值或窗口。真实背景树行→最终note/tree/detail中英三面RED1.149s（`/tmp/codrax-iofold-family-5CRBga/red.log`），GREEN及既有有界回归1.237s（同目录`green-regression.log`）；单成员0/1逐字保持，原projection不变。首次green仅既有软换行使整串测试假失败，修测试空白比较，未改生产wrap。模型上下文多成员三口径GREEN1.137s（`/tmp/hmc-native-duration-family-handoff-green-20260920.log`）。

活跃流/默认首响应与中途静默、请求预算隔离保护定向通过8.735s（`/tmp/hmc-native-duration-stream-preservation-20260920.log`），没有恢复4ms或请求总年龄降级。全部生产冻结后开始完整全仓及末版race×3，退出收据另补，不用前序成功覆盖末版验证。

旧业务/明确窗人工FAIL、accepted焦点live未命中及跨模式补证B2–B6继续开放；主清单仍13/79交付、66开放。下一固定版本恰好两个生产用例各一次，不重跑358版本第三例、不修改旧case/oracle追绿。

### 34.3 固定双例结果与末版登记补正

生产代码00be02cac73e原生构建退出0（`/tmp/hmc-native-duration-build-20260920.log`），末版三包race×3通过agent5.539s/tool10.343s/types5.764s（`/tmp/hmc-native-duration-final2-race-20260920.log`）。固定该二进制2并行×1：[机器摘要](../../eval/parallel_selected_summary_hmc_native_duration_20260920.md)2/2，[人工审计](../../eval/parallel_selected_summary_hmc_native_duration_20260920_manual_audit.md)0/2，业务384秒/41%上下文，明确窗171秒/45%。没有第三例追绿。

本批取得了有限而真实的生产正针：47ms独立备份IO已在正文、系统树/总览/表及背景观测中恢复，不再冒充18.55/17.85ms，未进入链上根因JSON；35ms请求正文也出现。其余FAIL未销：50ms业务窗仍套51ms查询账，31ms睡眠又被写成请求端到端，同一答案含正确35ms而前后矛盾；可见控制枚举仍泄漏。明确20ms窗和11ms链上IO保留，但模型把14/17ms睡眠与自身1ms就绪等待混说，且把窗末唤醒写成立即运行。此次明确窗列表没有使用cells，不能冒称§33的新分支已live命中；旧跨CPU概括错误本例已经改善，不能重用旧FAIL理由。

不能据日志搜不到引用就判本轮未发：window_stats真实对象有完整sync实例，工具日志仅打印Summary前2000字节，而原消息有约28k字节。同参数真实Explorer→下一次LLM消息的确定性回放已通过0.839s（`/tmp/codrax-33537-ref-audit.OIiApc/actual-message-normalized.log`）；四条消息扣除临时路径长度后与live日志字节数完全相同，各含2个当前可解析的完整实例引用，位置在约26KB后，焦点仍未选择。这不是原live完整消息历史抓包，不能拿旧358仅rank受视图白名单的漏发原因套本次。两次completion均未提交选择、supplement no_typed_target是已见事实。analyzer4次emit/3拒绝后有合法出口，finalizer缺summary一次拒绝后patch成功；未证明新“同字段必带必拒”。完整答案、投影、schema2 mandatory旁路及600/300/600秒传输保护均保留。

末版第一次全仓`/tmp/hmc-native-duration-final-full-20260920.log`退出1，tool357.519s，仅两条结构登记失败，不用上述定向/race绿覆盖：新Node.RankValueCaliber未登记字段处置，新增五族精确Type→Source映射未登记枚举审计点。按原协议登记displayed及真实消费文件；第二映射不能换成更宽aggregate谓词（该谓词会误收irq_burst/压力族，且漏per-thread三族），依既有distinct producer dispatch规则说明独立映射，并新增完整CausalTokenUniverse×五源/错误源/空白大小写的闭集交叉pin，恰五个合法组合。未更改生产、扫描算法、排序或豁免字段。两条RED1.232s、登记及相关回归GREEN1.337s（`/tmp/codrax-native-tripwire-fJ6Lr1/red.log`、`green.log`）；再次全仓在跑，正式推送及退出收据另补。

末版登记补齐后全仓`/tmp/hmc-native-duration-final2-full-20260920.log`退出0：87包通过（76缓存、11实际）、13包无测试、零FAIL；agent66.690s/tool366.542s/tracequery95.045s/types34.068s/cmd11.125s。新增登记闭集race×3亦退出0，tool6.345s（`/tmp/hmc-native-duration-registration-race-20260920.log`）。末段等待核实是Go测试缓存记录，而非模型响应或测试超时，没有终止或放宽任何测试。生产00be02及登记/文档将同批推送；本片只关闭五族计时发布和多成员口径交接，旧人工FAIL与父任务不勾。

推送收据：`00be02cac`、`911518ebd`已一并推送main，远程从8a8ca1813快进至911518ebd；推送后工作树干净。下一片从该基线开始，不把未提交改动混入本片验收。

## 35. 跨视图业务实例导航与末尾事实卡口径（2026-09-20，实现验收完成，答案留债）

这两项分别对应§32旧业务失败和§34当前明确窗失败，不能混为一个根因。两者都基于现有精确对象/字段，不用用户或模型散文扫描驱动硬门，也不赋予系统替模型选焦点或改写结论的权限。

### 35.1 同一完整实例在不同原生结果载体中可导航

旧358业务只查rank而未查统计/定位；rank的原生WindowStats.TraceSpans已有完整OpenDocument与LoadDocumentIndex，但导航发现以view白名单开关，仅span_window/window_stats/span_locate能发布。同一对象因工具组合不同而丢能力，违背HMC-02.4灵活组合目标。参考仓`ad_hoc_exploration.yaml`按问题组合指标、`marker_ops.build_marker_tree/compute_node_thread_states`按实例身份与区间组织事实；吸收对象驱动组织方式，不移植其缺窗全量回退、词名优先级或线程名模糊关联。

- [x] 先红：真实公开工具先验证确有完整native carrier，再检查引用；rank/两bundle/perf_stats/evidence_pack/两recipe/wakeup含stats/带真实span_name时间线等10臂失败；无carrier、wakeup=false、内部rank而未发布stats、async/缺端点、synthetic frame及真复合源负例通过。`/tmp/codrax-business-ref-carriers.NAUkj4/public-carrier-red.log`。
- [x] 用完整sync原生对象及既有来源/生命周期见证替代视图名白名单；引用只是导航，完整源+TID+B/E全tuple、私有代次、显式用户窗优先不变。
- [x] 多窗wrapper从成功child实际对象去重，不能按rank选第一窗或取端点并集；父物理源收据仍整体把关，失败/取消/复合源不能借兄弟授证。两独立窗有2个对象却0引用、重叠发现窗重复4条应去重为2的公开RED见`public-multiwindow-red.log`1.165s。
- [x] 公开多视图/多实例/裁窗完整pair/无效输入/来源变更/数量上限/私有焦点不变及真实Explorer消息回归、末版全仓和固定双例收据已取得；生产1d9a9ff643b6，随本次审计提交推送。只勾本片实现，不勾旧答案。

当前33537统计视图已有引用供给却未选择；本修复不能替该行为销账，不强制某一工具顺序或补造自动选择。

### 35.2 同一个候选的归因组成与状态占用明确另账

33526末尾`renderTraceFinalReaderDecisionCards`把node的有效值1ms和`PublishedStateOccupancy`给出的14/17ms sleep连写为“对应已测状态占用”。没有跨记录错join，但省略了前文selector已有的精确值组成，造成最近一层上下文语义容易误解。单源`tracefinding.RootCauseValueDescription`已校验1ms=自身runnable全额1+running缺口0，旧公共旁路也保此组成；这不是底层缺数。

- [x] /tmp overlay红针：两候选各自sleep17/14ms保持，但末尾各卡缺其已验证composition，`/tmp/codrax-reader-value-binding-e4KdAk/red.log`1.088s；不能说已发生必带必拒合同冲突。
- [x] 只读projection+node描述适配，复用原candidate编译及值说明，不由agent手组组件或按subject/rank跨窗匹配；同卡保原状态原值并标明另账，增其自己的已验证值组成。
- [x] 中英共用语义及校验，中文旧selector/sidecar字节、candidate identity/registry/selection wire不变；错和/缺值/负数/非有限/错误口径不造组成，running供给、D/I/O拆分遵守各自旧规则。
- [ ] IO卡没有精确请求身份时不能从全局35ms猜接31ms，当前IO正文前后矛盾仍开放。补实际finalizer消息回归、全仓/race、固定双例后再评，不用新增泛型大段教学代替。

### 35.3 独立待补：eval正文边界被新系统板穿透（HMC-18.5）

33537正文没有LoadDocumentIndex，机器`EXPECT_PRIMARY_CONTAINS`仍通过；该名字只在新“主要时间占用/关键路径候选”系统表及后续系统板出现。`eval/run.sh::scope_primary_stdout/scope_principal_stdout`仍以之后的“Trace因果投影”标题为终点，所以前置系统表被当正文。此项与生产答案修复分开，历史机器收据不倒签；人工已判FAIL，没有误销账。

- [ ] 应从已存在的私有`AnswerBlock.SystemGeneratedKind`及最终文档导出正文验收面，而不是继续追加一串中英文标题猜所有权；JSON同名字段不能铸该私有权限。核清最终文档出口和eval可读收据后另批实现，标准只能变准确，不能把正确系统脚注当模型已解释的证明。
- [ ] 回归系统块前置/交错/标题变化、模型同名标题、patch后追加summary、恢复原稿/引用附录；不作用产品路由、答案准入或模型原文，不因修评测回写原报告。

出口预查：`agent.parseOutputV2`在防御性副本应用hedging后渲染，后面仍有orchestrator修复/降级出口；不能只在一次emit保存就当最终答案。`record_task_finalize.go`最终落盘接缝目前只把成文字符串交给`outputdump.Args`，私有块所有权不在旁路里；`MutableState.ShippedAnswerDocumentV2()`可回读正常或降级结构稿，但必须再核与实际发射版本一致（不能拿被拒草稿或纯prose回退冒充）。下一片应在最终渲染/发布版本绑定处提供只读验收收据；暂不加另一套中英文标题终止列表。

全部仍归稳定79项子任务内部明细；13项已交付、66项开放不变，跨模式B2–B6及其它领域队列不遗失。

### 35.4 本片实现与回归收据（末版验证中）

生产只改两组：导航取消view白名单，复用原完整sync/原源资格，并把多窗成功child实际结果接入同一发现器；末尾事实卡用只读候选适配复用已有组成验证，将原状态记作另账，增加英文等义词面。没有改source-read签发、接受焦点选举、显式用户窗、原生查询和排序，也没有增加JSON必填/模型散文硬门。已有三条单结果出口天然复用同一函数，不复制视图清单；多窗不合并实例。

公开carrier全矩阵及旧引用回归曾通过1.955s（`/tmp/codrax-business-ref-carriers.NAUkj4/public-carrier-complete-green.log`）。随后邻接旧memo测试失败真实保留：两条复用正针与含stats的两个重复查询现在有完整实例，原候选安全策略要求fresh read，旧期望已不再代表缓存车道。没有放宽生产缓存策略；为缓存专项加入独立无导航但非空因果参数（wakeup_chain+stats=false），保原共享rank fixture，容量变化、目标来源与轮次重置增加真实warm-hit对照；含stats四次公开查询则验证各自产生新的当前引用，无stats重复仍缓存且因果链逐字段恒等。

主席初次联合定向`/tmp/hmc-native-carrier-value-green-20260920.log`退出1：新context-only负针错用Predicate而非生产Tier，公开token可解析针漏走dispatch发布步骤；两项为测试装配问题，已分别改为既有typed Tier及实际AppendDispatchToolResult，未借此扩生产权限。修正后`/tmp/hmc-native-carrier-value-green2-20260920.log`退出0：tracefinding0.616s、agent0.974s、tool1.997s。中英旧字节、组件错和/缺失/负值/非有限/错误口径、链外/无证/窗外、同主体同rank异窗不借值及实际BuildInitialInstruction均覆盖。全仓、末版race与固定双例仍待退出收据，不先勾整片交付。

新增真实Explorer→工具执行→下一次模型消息覆盖span_window、无span_name的rank及bundle三臂，包含当轮可解析token与完整tuple，不用预造ToolResult替代；公开GREEN1.136s、与事实卡消息race×3为3.746s（`/tmp/hmc-native-carrier-message-green-20260920.log`、`message-race-20260920.log`）。四包定向race×3通过tracefinding1.769s/agent8.531s/tool26.234s/types7.289s（`/tmp/hmc-native-carrier-value-race-20260920.log`）。首轮全仓agent报告两条旧固定文案预期失败：新候选限定注插入导致旧连续子串变化，以及“对应占用”改为另账；按本片设计更新精确期待，原数值/背景隔离/自然语言负针不动，定向GREEN1.447s（`/tmp/hmc-native-carrier-reader-oldpins-green-20260920.log`）。全仓退出及补正后重测另记，不宣称首轮全绿。

### 35.5 固定版本双例与新失败断点

生产提交`1d9a9ff643b6`，构建退出0，固定该版2并行×1：[机器](../../eval/parallel_selected_summary_hmc_native_carrier_value_20260920.md)1/2，[人工](../../eval/parallel_selected_summary_hmc_native_carrier_value_20260920_manual_audit.md)0/2。明确窗213秒/45%上下文，业务294秒/37%。前者已保20ms/11ms、另账14/17ms与自身1ms，仍误说修向独立、将未证机理泛化为无根因资格。后者引用已供给且模型实际使用两次，但带同源/同线程重复参数被一概混填门拒绝，退回51ms查询，最终正文与50ms业务窗混账、缺LoadDocumentIndex/投影，schema2正常必产但状态为`unavailable/trace_root_cause_contract_not_active`。本轮补齐因`families_present`跳过，不冒称旧`no_typed_target`。

下一处高ROI调查：引用绑定的是已成功发布的精确完整tuple；允许经过私有引用/当前源验证的冗余相等字段，有望消除无语义差异的拒绝。绝不能直接忽略额外参数或按文字相似选择：不同物理来源/代次、TID、区间、process范围仍拒绝，用户明确窄窗仍最高优先。当前教学本来要求省略，重复字段来自模型而非系统注入，故暂不定性为必带必拒合同；先公开红针与边界设计，再独立小批。13/79交付、66开放不变。

末版全仓`/tmp/hmc-native-carrier-value-final-full-20260920.log`实际退出0：87包通过（71缓存、16实际）、13包无测试，agent90.504s/tool364.091s/tracequery101.631s/types39.343s。此前首轮全仓退出1仅两条旧文案pin，tool433.118s通过；补正后的独立agent整包70.534s及本轮全仓均通过。后续新片断言红针不属于1d9，不混入此验收。本片代码与机器/人工收据同批推送，不以单测绿覆盖0/2人工通过。

推送收据：生产`1d9a9ff64`与审计`400e648f5`已推送main，远程从911518ebd快进至400e648f5；此时下一片仅有独立断言测试草稿，不混入旧片提交。

## 36. 原子业务实例引用的重复参数安全兼容（2026-09-20，实现验收完成，答案留债）

§35.5公开live中，正确引用加相同来源/TID/thread被两次拒绝，模型退回宽窗。问题不是JSON畸形，也未证明必带必拒；模型违反了当时“全部省略”的教学。但系统可以精确识别同一tuple的冗余字段，无须让模型重新抄窗口。参考仓按实例/窗口组合测量的组织方式仍适用；不采用模糊名称、第一窗或最长窗代选。

- [x] 公开先红：window_stats/wakeup_chain/root_cause_rank × live形/完整tuple/path模式/单位字符串12臂，合法相等断言均被旧门拒绝；错字段负例仍过。`/tmp/hmc-business-ref-assertions-red2-20260920.log`1.387s。初次测试编译误用不存在的Window.DurationMs已单独保留在`red`日志，不冒充行为红；改为原生端点差后才取得RED。
- [x] 查询时先解析当前私有引用，再逐字段精确比较；相等字段仅作断言，不覆盖或额外筛选，恢复同一完整实例参数。来源与显式path必须各自同物理源；有不同附件时不能借“唯一引用文件”回退。prepared别名只复用已验证材料，不启动转换。原有源前后代次、未知/取消/复合/窄窗和pending→accepted焦点门不变。
- [x] schema和实际工具消息复用同一个断言教学，推荐仍为两字段最简调用；去掉旧schema仅三种发现视图的陈腐描述。矛盾字段给普通显式查询出口，不让修复方向或程序通过改写用户原文获得权限。
- [x] 公开结果逐字段恒等、完整50ms与5ms运行不变；不同源/同字节异文件、附件遮蔽、链接重定向、TID≠TGID、错名/错行/1ns错窗、明确窄窗、reset及源修改均拒绝。gzip已准备原始路径/查询路径，两种attachment/coordinator入口保持可用；未准备同字节副本不转化成别名，陈腐原始输入不能借尚存query文件续证。
- [x] 实际Explorer三轮：发现→模型带重复source/thread/pid→下一次模型收到成功测量且完整50ms、两个IO账保留；成功工具不授completion焦点。全仓/race/构建及固定双例收据见下；只勾确定性实现，旧人工FAIL不销。

首轮公开GREEN1.210s、旧实例公开矩阵及边界1.710s；实际Explorer消息GREEN1.260s。加入gzip/双来源交叉后末版定向tool2.771s、agent1.031s（`/tmp/hmc-business-ref-assertions-final-green-20260920.log`）。旧“混填必拒”测试保10个错误字段负例，原来恰好与引用一致的path/line改用真正冲突值，并由新12臂逐字段恒等正针承接；没有删除来源/窄窗/过期负面要求。跨视图发现、原查询引擎、排序、旁路JSON和600/300/600秒策略不变。主清单13/79、66开放不变；§35.3正文所有权及跨模式B2–B6继续留队。

### 36.1 固定双例、整仓补正及别名复核

93858fb94a05原生构建退出0，固定该版2并行×1：[机器](../../eval/parallel_selected_summary_hmc_business_ref_assertions_20260920.md)2/2，[人工](../../eval/parallel_selected_summary_hmc_business_ref_assertions_20260920_manual_audit.md)0/2。业务274秒/46%，明确窗174秒/43%。业务模型没使用实例引用，仍取0.999..1.051；补齐与投影可用，但正文7ms宽窗运行套50ms业务窗、31ms当请求驻留后又正确列35ms、内部字段泄漏，旧FAIL不销。明确窗保20/11ms，但错把400→300唤醒2.016写成2.014并扩写等待机理；不是新引用分支造成的证据时间改写。两份schema2旁路均available，不代表正文正确。

首轮race×3 tool21.444s/agent7.581s/types3.149s；流式/default保护17.422s，活跃字节/心跳可超过旧总时限，600/300/600保持。首轮全仓agent102.564s报一条伴随completion schema测试仍期待旧全省略文案；仅更新query子臂为精确断言合同，completion可选性/接受与dispatch条件、用户窗及源代次pin不动。相邻定向已绿，独立agent整包77.634s；全仓退出与最终重测另补，不以定向绿覆盖整仓失败。

独立源码复核确认同类别名缺口：938版本重复物理路径已支持，但当前typed逻辑工件ID仍被当文件路径比对；普通查询同一ID成功而引用查询失败，公开RED1.278s（`/tmp/hmc-business-ref-logical-red-20260920.log`）。等固定live结束后补正，复用原`traceQueryAdaptLogicalArtifactPath`再比物理域，不新增ID映射表、不忽略显式source；不同ID/未知ID/不同文件仍拒绝。文件与prepared附件两种ID均实测。测试曾试图构造独立perf别名，但prepared选择表已将它并入物理工件，fixture因此失败；去掉不存在的测试前提，保真实双文件ID/外来ID负针，并运行既有逻辑ID全部回归，不改原歧义解析规则。最终联合GREEN tool3.746s/agent1.737s（`/tmp/hmc-business-ref-logical-final2-green-20260920.log`），此补正支线尚无live命中，不借938回放销。

### 36.2 取消分型的末端复核

938首轮全仓实际退出1，仅伴随教学pin，tool426.974s通过。逻辑别名及该pin补正后重新全仓；该版race×3通过tool27.475s/agent6.159s/types3.829s，构建e5c2d81861ed退出0。推送前复核再发现本片引入的取消分型小回归：无重复字段会走原typed取消出口，而带匹配source/path时prepared-material.Validate观察到取消，布尔比较却把它误报坐标冲突。公开先红1.165s（`/tmp/hmc-business-ref-assertions-cancel-red-20260920.log`）：canceled/deadline各两个重复字段臂失败、各自无重复字段对照通过。

补正只在断言失败时保留已发生的caller取消原因，复用原`traceQueryCancellationResult`，不改等待时长、不重试取消、不签观察或新引用。最后定向tool3.539s/agent1.231s通过（`/tmp/hmc-business-ref-cancel-final-green-20260920.log`），覆盖所有实例引用/逻辑ID/准备材料及completion消息。再次完整全仓与增量race退出收据待补；未以更早的绿覆盖此末改。

最终收据：生产`f68737ec02b0`原生构建退出0（`/tmp/hmc-business-ref-cancel-final-build-20260920.log`，版本命令核实同SHA），取消/逻辑解析增量race×3 tool4.639s退出0；最终全仓`/tmp/hmc-business-ref-cancel-final-full-20260920.log`实际退出0，87包通过（71缓存、16实际）、13包无测试、零FAIL，agent94.206s/tool404.497s/tracequery123.226s/types51.253s。补正前e5独立全仓也实际退出0（同87/71/13，agent95.109s/tool454.069s/tracequery120.027s/types49.965s），但不拿它代替末版验证。`93858fb94`、`e5c2d8186`、`f68737ec0`与本收据分层提交，同批推送；938固定双例未命中新引用支，e5/f687无新增live，不能把这些边界单测说成生产命中。13/79已交付、66开放不变，下一优先§35.3正文验收所有权，再继续旧答案与跨模式留债。

## 37. 最终渲染归属收据替代正文标题截断（2026-09-20，验证中）

承接§35.3，修的是测量器误 PASS，不是替模型改答案。系统表前置、交错、改标题后，旧终止标题仍可把系统事实当作模型解释；反之模型自己使用相同标题又会误截真正正文。历史机器收据不倒签，人工 FAIL 不注销。

- [x] 渲染同一遍记录最终实际发射片段，按私有 `AnswerBlock.SystemGeneratedKind` 分离模型正文；保原去重，禁止过滤文档后重新渲染而复活原本未显示的内容。primary 不含引用/片段附录，principal 含文档引用/片段；两者均不含系统块、恢复附件和末端系统补充。文档级 caveats/缺失层披露混有系统增补、没有独立所有权，保守不作为模型解释验收；真正模型 caveat block 仍保留，客户报告不删。JSON 同名键不能铸私有所有权。
- [x] 保存渲染时快照，不回读后来变化的文档猜版本。最终落盘须与实际答案逐字相同（仅允许已有系统 caveat 注册回放）；结构稿恢复复用原本的去空白/不合法图降级操作，纯散文回退或陈旧快照提供 unavailable，不能借旧稿签绿。
- [x] 默认输出新增只读 `.answer-surfaces.json`，与完整 Markdown 及最终答案摘要绑定，系统日志另绑定收据内容摘要。写失败不影响客户答案/mandatory 根因旁路。eval 保存核验后的收据、完整报告、两个正文面；缺少或不匹配时，声明正文 oracle 的用例失败，包含只有负面断言的情况，不回退标题猜测。
- [x] 确定性覆盖前置/交错/标题改变、模型同名标题、patch 后追加摘要、恢复稿/原始散文、附录、实际去重与最终发布路径；Python 覆盖收据和 Markdown 篡改、最新 unavailable 不借旧成功、schema 类型和空正文。
- [ ] 完整 runner、全仓、末版 race、固定两例以及人工审计收据待退出，不先勾整片交付。全答案 stdout oracle 未改；其它独立 inventory 验收器不在本片迁移范围内。

初次五包集成通过（`/tmp/hmc-answer-surfaces-integration-20260920.log`）；实际 ParseOutput/finalize 落盘回归 agent1.806s/orchestrator0.827s（`/tmp/hmc-answer-surfaces-actual2-20260920.log`）。中间测试装配失败诚实保留：新 fixture 误写不存在的 AnswerDiagram 类型；并行测试编辑中的解引用编译错误；runner 新三例缺 NAME。均修测试，不改产品门限。一次 runner 在编辑期间读取文件造成 EOF，已改为冻结脚本后重跑；不将其称为产品事故或首轮全绿。

本片不新增模型 schema、提示要求或答案准入硬门，不改客户答案字节、因果资格、窗口、排序、超时和自动补齐。HMC-18.5 父项及 13/79、66 开放总账不变。

独立复审补正：新 Markdown 已写、收据失败时旧版成功收据仍可能留在合并日志，不能只挑最后成功收据。现绑定最后一次默认 Markdown 写出、其后的同 stem 收据、字节数及两份 SHA；同路径重写、旧收据晚到、仅有收据无写出、HTML/explicit report 干扰均覆盖。Python 真实 RED 5失败→GREEN 14通过，见 `/tmp/codrax-marker-local-tests.7ljYMk/{RED,GREEN}-answer-surfaces-write-binding.log`。混合文档级 caveat 亦先红后绿（`/tmp/hmc-answer-surfaces-mixed-caveat-red2-20260920.log`、`green`）；前一次类型装配编译错误单独保留。首版完整 runner 已退出0，末版绑定后冻结重跑中，不用首版结果替代末版。

## 38. 业务片段自身的调度时间账（2026-09-20，验证中）

§36 的业务报告有 OpenDocument 50ms 与查询窗52ms，但将后者7ms运行写成前者状态；工具确有完整业务打点，却只提供业务耗时，没有同一片段的线程状态账。这一供给缺口是系统问题，不能全记模型波动；仍不保证补证后模型一定使用。

及时复核参考仓 `core/preprocess/marker_ops.py::compute_node_thread_states`（约628–642行）：按节点所属线程，将每条状态与每个 marker 做精确区间交集。借鉴实例归属和裁窗测量，复用本仓已有 timeline 分区、头部未知、调度生命周期与 S-IO 包含项逻辑；不移植参考技能的关键词路由、首文件/缺窗全量回退或按最大状态裁主因。

- [x] 公开工具及真实 BuildInitialInstruction 先红：业务50ms/嵌套40ms有完整 marker，缺自己的5ms/8ms运行账，外围7ms/52ms账仍存在。原 RED 为工具输出转录而非原始重定向日志，保存 `/tmp/codrax-marker-local-tests.7ljYMk/RED-transcript.txt`，不虚报文件来源。
- [x] 仅对已发布、有完整同步 B/E、同一物理因果源、明确 header TID 的普通业务片段附加只读原生 `scheduler_states`；使用查询裁剪后的该片段区间，不将 payload PID 当线程，不赋予焦点、链边或根因资格。全量内部清单、语义优化与 async 分支不改。
- [x] 原生对象→观察→末端业务事实卡传递同一账户；完整/部分/不可计量区别保留，未知不作零，S 中标记 IO 是包含项不再加，睡眠状态不解释成 IO 请求或等待机理。中文卡使用自然语言，不直接发射 coverage 枚举。
- [x] 公开正负针覆盖重命名嵌套、owner TID≠payload PID、裁窗、未知头部/无调度、async、错源/复合源、原榜排序恒等和清单不变；工具/agent/tracequery/tracediag focused 已绿（`/tmp/codrax-marker-local-tests.7ljYMk/GREEN-focused.log`）。
- [ ] 全仓、schema 显式字段处置复核、末版 race、构建和新固定双例待收据。35ms请求与31ms线程等待的旧口径矛盾、明确窗唤醒时刻/机理错述仍开放；没有从全局35ms猜接某条31ms。

不增加 JSON 必填字段、用户或模型原文扫描，不改显式时间窗和自动补齐，不取消优先级/算力/D/IO/语义优化/业务线索。仍归 HMC-02.4/18.4 内部子缺陷，父项和总数不改。

复审补强：windowed index 的头状态只在原查询起点有快照，直接把子查询起点改到 marker 会丢掉 padding 外的已证状态。真实构建已复现 OuterWork 应运行40/总80ms却显示20/60、InnerWork应20/60却显示10/50，修正与末版验证进行中；不得凭全窗总数按比例分摊。诊断显示另补嵌套字段处置：原 WindowStats 指纹只看切片类型，未漂移不等于新字段可免审。确切字段列表、原来源隐私与大时间定点、测量零/未知区别已有新 pin；保持一个片段 detail owner、不升成根因 key-first。通用 walker 丢零的真实 RED0.516s→精确 typed 显示 GREEN0.825s（`/tmp/hmc-marker-scheduler-diag-{red,green}-20260920.log`），不重钉无关哈希。

### 38.1 冻结小批与末版回放入口

两笔本地提交：`8bec1875d` 正文归属、`db0596794` marker-local 状态账。后者包括 windowed 子窗补正：复用原 owner 的已证 timeline 后逐状态区间求交、重新生成子窗计量域，不裁总数、不修改共享头快照；未知头与完整性失败保持原保守路径。窗口专测0.578s、含既有 carry-in 的 race×3 1.822s。普通 B 起点在 retained padding 之外导致整段不返回，是独立既有开放项，归 HMC-02.4/18.4，未由本片覆盖。

RichNote 新 key 已回归 types 单源登记/soft-consumer/golden，真实 emitter 覆盖先红后绿；agent AST pin 恰核 SpanKind 与新 key 两个合法常量。没有把 tracequery 私有新常量当作登记替代，也没有扩扫描豁免。中间组合日志 consumer pin 曾误计既有 SpanKind，修后单独GREEN1.308s，原日志保留。Python 最终16项通过，新增默认 mkdir/Markdown 写失败屏障，不受 HTML 或显式副本错误影响。

首轮全仓正式退出0：87包通过（35缓存/52实际）、13无测试，早于上述末改，仅作前序收据（`/tmp/hmc-surfaces-marker-final-full-20260920.log`）。末版八包 race×3正式退出0，render1.757s/outputdump2.647s/types3.227s/agent6.494s/orchestrator3.639s/tool14.691s/tracequery14.029s/tracediag14.831s（`/tmp/hmc-surfaces-marker-final2-race-20260920.log`）。600/300/600默认、活跃字节/心跳与请求预算隔离定向9.815s通过；末版完整 runner 已全部通过（`/tmp/hmc-answer-surfaces-runner-final4-20260920.log`，退出收据待收取）。

db0596794c36 原生构建退出0并核版本（`/tmp/hmc-surfaces-marker-build-20260920.log`）。固定该二进制两个旧FAIL各一次、并行2，不改原case/oracle；按影响与复发排序先业务窗混账，再明确窗链和机理边界，后续跨模式B2–B6仍留队。后续真实退出与人工审计见§38.2，不注销旧FAIL。

### 38.2 固定回放的失败收据与小批补正

固定db双例已结束：[机器](../../eval/parallel_selected_summary_hmc_marker_local_surfaces_20260920.md)1/2，[人工](../../eval/parallel_selected_summary_hmc_marker_local_surfaces_20260920_manual_audit.md)0/2，业务279秒/39%上下文、明确窗158秒/39%。同版没有第三次追绿。

- 两份答案归属收据都写出却为unavailable：正常发布链最后的`appendRuntimeDispatchAdvisoriesToAnswer`即使无advisory也TrimSpace，归属快照只允许旧原字节/caveat回放，漏了这个已有展示变换。真实recordTaskFinalize+发布helper红针两条失败（`/tmp/hmc-answer-final-trim-red-20260920.log`）→仅允许精确外层trim的GREEN1.126s；正文内改字、未知附录仍拒绝，不从后来文档猜所有权。客户答案不改、旧回放不倒签。明确窗case只有全答案oracle故仍机器PASS，业务primary oracle正确fail-closed。
- 业务自己的5/1/44ms已到真实finalizer消息（日志2748），正文仍写5–7ms后又写5ms，40ms LoadDocumentIndex与35ms请求窗口混用、把真实IRQ waker说成非链上。35ms请求/31ms线程等待与47ms后台排除有所改善，但不足以人工通过。模型未采用实例引用，投影保持其查询0.999..1.052；不偷改焦点。schema2必产但无可选候选，不以文件存在冒充有效选择。
- 明确窗保持20/11ms、2.016/2.018/2.020唤醒时刻且保留投影，旧时刻错述改善；仍将network14ms睡眠写成2.001..2.018（真实2.002..2.016），系统表也同错。已定位递归`WakeupCausalImpact.Window`统计域被通用ObservationSpan/表误当发生窗；ActualWindow也含多个状态，不能直接替换为睡眠段。独立下一小批分清统计域与物理段，金额/因果资格不动。
- 真实analyzer上下文还有“一次成功可修正”与“严格只调一次”、`every field REQUIRED`并存；业务4次拒绝1次成功却告警5次写入。工具schema/profile校验未证明有错，不能用宽松校验消重试；下一小批统一实际教学与成功写入计数，保留总attempt遥测。

末版完整runner正式退出0；全仓final2实际退出1，仅`TestThreadStateComparisonConsumerCoverage`要求登记新windowed-head分支的`StateUnknown`精确排除。已逐字段审计：该比较禁止未知物理状态升级为恢复头，只控制coverage，不新分类；增加该唯一消费点golden，定向GREEN0.898s。完整日志`/tmp/hmc-surfaces-marker-final2-full-20260920.log`保留，不把早版全绿冒充末版。后续完整复测、trim race及下一教学/窗口小批分记收据。

参考仓再次核对`marker_ops.py:576–640`的每线程缓存/逐节点状态交集与`trace_data_cache.py:294–313`先完整配对再相交：本片吸收前者；后者启示的远端B/E丢片段仍独立开放，不扩大padding或将发现候选池冒充完整清单。所有工作仍为稳定79项内修复，13已交付/66开放不变。

推送收据：main已由26c2bf350快进至f908c2f0e，含正文归属8bec1875d、片段状态db0596794及末端trim/审计f908c2f0e。trim race×3正式退出0，orchestrator2.824s（`/tmp/hmc-answer-final-trim-race-20260920.log`）；不将随后施工文件混入这三笔。

## 39. 分析阶段提交教学与成功写入计数统一（2026-09-20，验证中）

§38.2业务生产日志实际5次attempt、4次结构拒绝、1次接受；其Goal/HardRules/OutputFormat和工具描述仍说只许一次，Workflow却说失败可修正。先前§123.83仅改Workflow，尚未消除此矛盾。另两份手写必填清单已陈腐（八/九predicate、漏runtime_selection_profile、将条件fact_families当固定字段），而真实schema已有准确要求。此为系统教学缺口，不证明5次尝试均由教学造成。

- [x] 先红：strict失败→成功仍误拒；全失败可借预存RequestModel继续构建IR；真实拼装prompt含绝对一次/全字段必填。日志`/tmp/codrax-analyzer-contract.lnhUZH/RED.log`，不是只搜源码。
- [x] 保`analysis_emit_calls`全部attempt遥测，另记成功次数；重复写门只看工具成功布尔。一次成功前允许完整修正，全失败/零尝试fail-loud、两次成功仍按原strict策略拒绝，末尾失败不抹掉前次已接受写入；不改profile/来源/schema校验。
- [x] schema为JSON结构唯一依据，删除两份独立必填/可选checklist，保九predicate、所有runtime/profile/显式窗及条件fact_families的语义教学。工具描述和Workflow复用同一成功提交合同；动态“下一response一次完整调用”仍有效，不误当dispatch禁止修正。
- [x] 顺带消除导航阶段`irrelevant_files`要求读源码内容的自冲突：只能用已经返回、允许的导航元数据判明确无关，否则省略，不能为填字段打开源码。字段仍array/string/max10。真实拼装普通/trace附件×首次/retry及terminal失败修正回归覆盖。
- [ ] 全仓、race、构建及固定新版双例待退出，旧FAIL不销。

最后定向正式exit0：skill0.849s/agent2.776s/tool1.340s，`GREEN-focused-final2.log`。中间`GREEN-focused-final.log`中的旧OFF-TOPIC字样pin失败真实保留，换为解析该property硬钉结构+准确许可/禁令，不改schema门。参考仓工具组合工作流不能替本仓分类接口定义结构；本片不照搬其工具白名单或关键词路由。L1调度循环、Trace选窗/根因/补齐、mandatory旁路与600/300/600不动，13/79交付、66开放不变。

本片已独立提交39becadd4；定向race正式exit0：agent3.978s/skill2.004s/tool3.534s，`/tmp/codrax-analyzer-contract.lnhUZH/GREEN-race.log`。原schema字段集合及条件门不变；新增的是精确attempt/accepted诊断，不把一次失败当一次写入。

## 40. 递归链统计域与状态发生区间分开显示（2026-09-20，验证中）

db明确窗报告把network累计14ms睡眠配到递归查询域2.001..2.018，工具观察Span、成文context和系统占用表共用该裸窗口，形成系统供给共因。实际精确state_drilldown已有2.002..2.016，底层统计正确；actual_window=2.002..2.018仍是所有状态的包络，不能当睡眠段。独立源码/人工复核通过，不以模型波动处置。

- [x] 公开root_cause_rank→typed观察→投影占用表/成文ledger与证据附录三面先红，改名transport也同红；五种状态及多段包络同样需要区别，不按thread名/14ms拟合。
- [x] 复用既有predicate/source字段的只读显示分类，共享“依赖分析窗口，不是单段状态起止”语义。表中累计状态与统计/定位范围清晰分栏说明；真实精确state_drilldown端点保持。无新schema字段/note key/门，原观察、数值、排行、查询窗、链上资格与旁路均不改。
- [x] 工具description及skill旧`actual_window`单段教学直接改原静态句，说明全状态inventory envelope；单独interval行的actual_duration/actual_start/end仍是原单段语义，不混淆。新公共/成文/types/skill定向GREEN：tool1.200s、agent2.146s、types3.816s、skill3.054s。
- [ ] 邻近整包、全仓/race、固定新版本双例及人工审计待退出；旧db报告保持FAIL。

独立留债：系统占用表cookie17/network14各重复一行，现有同源StateAccountKey在链impact可用，但派生计价行缺原始状态来源键。不得按同名+同14ms强并；后续补原始状态来源身份，再精确归并，避免重复项挤占5行上限。本片不夹带去重。正文“network比cookie优先级高”（实际同prio20）及“IO11ms是整链最长阻塞”（症状20/17/14更长）也仍需人工审计；正确口径是已确认可消除候选中最大，不由系统改写模型结论。

对照参考仓的节点状态交集和缓存区间筛选，保留其区间/统计域区分思路，不照搬全节点耗时到用户窄窗。远端B/E漏片段、模型采用业务实例、跨模式B2–B6及其它HMC任务继续开放；13/79、66开放不变。

实现b50fb08a0已提交。原窗口RED/GREEN为工具输出收据，非重定向日志：转录`/tmp/codrax_dependency_window_display_receipts_20260920.txt`注明原session/chunk与exit；邻近B1596/RequestedScope/ValueOccurrence/StateAccountKey正式exit0，tool4.101s/agent2.462s/types4.219s/skill2.780s。首次构建因同批skill教学测试尚未入提交标作b50fb08a0207-dirty；未用于live，补入测试后重新干净构建，不能把dirty构建说成固定提交版。全仓正在包含该测试的工作树执行；源码冻结不变。

### 40.1 多段累计状态和旁路定位：反例扩展，尚在收尾

独立复审否证了本节初版“state_drilldown可提供精确状态发生端点”的泛化表述：单段fixture恰好相等，但`accumulateThreadDuration`汇总同线程/CPU多段状态，再由`buildStateDrilldownPlanForTarget`复制最早/最晚包络。所有top_sleep/top_runnable/top_running/top_io_wait/top_d_state及state_churn均不能凭起止推定连续状态。这是本批自身教学缺口，不能留给模型承担。

补强沿同一typed显示分类：这些行标累计状态统计范围，缺/未知source保守显示未证明单次发生；精确端点只来自同源thread_timeline的单独interval。两段睡眠、三段runnable真实公开查询→Observation→成文ledger/附录先红；全族/改名/原生数值不变及run后缀测试同补。不新造JSON必填字段，不用数值等于包络宽度签连续性，不改变recommended_views/chain_required/recursive或原窗口。

旁路复核：`TraceCauseEvidenceFacts`已只留seat坐标，原predicate/source没进入冻结事实，不能凭StateKind恢复精确发生资格。因此本片仅将root-causes定位句统一为中性“定位范围”，注明可能是统计域/记录包络；真实D/IO坐标也原样保留，不扩schema、不补造来源。未来精确来源贯通仍留债，不能以中性措辞称该信息已补齐。

519全仓正式exit1（`/tmp/hmc-submission-window-final-full-20260920.log`）：占用表位置新增统计限定后的旧字节pin、Description byte golden、Producer硬等于trace_query遗漏run后缀三项。位置pin保数量/数值/时间不变仅改正确限定；Description按本文件演进仪式核对确切增量，h2/h3匹配基线A/B仍开放；Producer用既有家族函数根修，不放宽结构lint。最终绿色另补，不以旧绿覆红。

## 41. 519固定双例：测量器恢复，人工仍0/2（2026-09-20）

干净`519c997ef88e`构建正式exit0（`/tmp/hmc-submission-window-clean-build-20260920.log`），固定二进制2并行×1，未改case/oracle：[机器](../../eval/parallel_selected_summary_hmc_submission_window_20260920.md)2/2，[人工](../../eval/parallel_selected_summary_hmc_submission_window_20260920_manual_audit.md)0/2。业务247秒/44%，明确窗212秒/43%。两份渲染归属收据available且经摘要绑定，正文已可独立审计；两份schema2根因旁路available，不能据此注销答案FAIL。Trace投影及自动补齐均保留，无活跃流强制超时/空答案。

- 业务自身5/1/44ms确到finalizer日志2495，但摘要仍套51ms查询窗6/1/44到50ms业务窗；explorer completion已先混写，说明“补证可见”不是“模型采用”。摘要31ms错当请求生命周期，后文又正确35ms；三个不重叠时间段被说成部分重叠；后台W错称读。保为人工FAIL，不增加散文数值硬门。
- 明确窗改用测量窗口描述依赖域，旧端点误述有改善；但每条边右端wakee的14/17/20ms睡眠被移给左端waker。复核实际不是TargetBlockedMs问题，而是WakeupEdge.LatencyMs与边context只写pre-wakeup wait，未在数值旁写等待归属。安排下一只读展示/上下文小批从已有wakee字段明确角色，不改账。
- 同源原始占用镜像仍cookie17/network14各重复一行。StateAccountKey兼有rank↔chain one-seat计价吸收职责，不能直接复制Sleep键到1ms PIC。后续另立仅展示的原始状态身份，须基于同源完整精确interval清单；MeasurementOrigins/来源清单自身不能当数值身份。此项未施工，不误销。
- analyzer不再把失败attempt当多次成功写入，但业务仍5次尝试、明确窗2次。真实profile不一致没有因计数/教学修复自动消失；不放松合同、不宣称减少重试已验收。

39/40首片五包定向race×3正式exit0（`/tmp/hmc-submission-window-final-race-20260920.log`）：agent3.714s/tool4.565s/types5.357s/skill3.424s/orchestrator6.410s；早于40.1补强，不能充当末改验证。同版不追加第三次追绿。HMC总账13/79已交付、66开放；显式窗、链上各根因家族和业务线索、mandatory旁路、600/300/600及L1均不改。

## 42. 等待归属与探索期片段状态供给（2026-09-20，末版验收中）

沿§41人工FAIL修两个证据供给共因；与40.1同属测量对象/窗口语义，不添加答案扫描门或新必填结构。

### 42.1 唤醒边的等待量归右端被唤醒线程

原始边数值正确。新增只读共享formatter/教学，将每条数值紧邻原生Wakee身份；工具banner、Observation摘要、真实context.BuildPromptContext的TraceWaitEvidence及via-thread摘要同源。说明该量不属唤醒者自己的睡眠/运行、不是唤醒后调度等待，也不等于证明唤醒者造成该耗时。未知owner不借左端补齐。字段/时长/时间/路径完整性/计价和TargetBlockedMs均未变。

参考仓再次亲读`core/preprocess/sleep_ops.py:558–650`：先以blocked_tid找直接waker，再单独统计waker自身状态；child.block_ms反而由父方的阻塞段时长赋值。本仓借鉴前两者的角色区分，不能直接将其child.block_ms复制成waker自身耗时。`marker_ops.py:548–642`仍按owner逐段交集，不拿父窗总量当子marker状态。

公开TraceQuery→Observation→BuildAgentContext→BuildInitialInstruction/BuildPromptContext六变体（改名×S/D/IO）RED→GREEN，右端14/17/20与自身11ms、原2..2.020窗及完整链均钉；via先红后绿。收据`/tmp/codrax_wakeup_wait_owner_{red,via_red,green,nearby}_20260920.log`，最终邻近context0.855s/tool1.088s/types2.373s/tracequery1.596s/agent3.530s正式exit0；独立只读复审无阻断发现。末版全仓/race及live另记，519人工FAIL不倒签。

### 42.2 已有业务片段状态账须在探索期可见

继续核对519日志：真实explorer TOOLRESULT第1497行先给1..1.051全窗6/1/44；业务trace_span摘要只给duration，新增scheduler_states仅在原生JSON/typednote/最终成文事实卡可见。探索completion先产生6+1+44对50ms的错账，后续再给正确5/1/44不足以保证模型重解。这是确定性供给遗漏，不代表该次所有语义错误均由它导致。

公开wakeup_chain/root_cause_rank/window_stats三路先红；补只读marker-local状态摘要，复用已有Matches核对source/线程/区间及计量域。完整/部分/不可计量区别、S内IO包含项和不证明等待机理同步提供；异源/异owner/异窗/async/缺测/坏账拒绝发射该附加事实而不影响原查询。业务名任意，不扫描问题/答案，不自动选择实例。

首次把摘要放trace_span尾部，wakeup/window_stats已绿但root_cause_rank仍被blob预览裁到中间，真实exit1保留`/tmp/hmc-marker-explorer-summary-green-20260920.log`（文件名不是通过证明）。末版移到全窗账紧邻、长榜前；复用现有状态摘要容量，超过明确披露数量和原生payload续读，不调大预算或复制到尾部。最终定向tool1.998s/agent1.235s exit0（`/tmp/hmc-marker-explorer-summary-final3-green-20260920.log`），原公开RED为`red`同前缀日志。新完整全仓与六包race×3进行中。

40.1末版定向与race正式exit0：`/tmp/codrax-state-window.PrFzO8/GREEN-final.log`（tracefinding0.489s/tool2.631s/agent1.132s/skill1.390s/types3.632s），`GREEN-race.log`（1.557/4.812/6.116/1.990/7.432s）；公开分段、skill旧教学、三pin及sidecar的RED分文件保留。仍不宣称dispatch-sensitive h2/h3匹配A/B已做。

上述代码与519双例人工记录已提交`85300eccf`，暂未推送。六包race×3正式exit0（`/tmp/hmc-measurement-ownership-final-race-20260920.log`）：tool12.403s、agent17.445s、context6.804s、types6.299s、tracequery9.950s、tracefinding6.937s。全仓退出另记，不用race代签。独立复审另发现marker长名称可挤满blob头部预览；正在仅对显示副本复用既有banner长度保护，原生名称、来源身份及状态账不得截断，真实StoreBlob边界须先红后绿。该末改不由前述race收据覆盖。

### 42.3 长名称不能挤掉相邻证据预览

两个任意20KB级业务名称经真实公开root_cause_rank→StoreBlob，169221字节原始摘要在既有头尾预览中丢失marker状态与rank头/首行，公开RED exit1。仅在完整Matches之后将显示名称/显示owner复用既有`sanitizeForBanner`；source basename原已使用该保护。未改预算、原生名称、typed note、source/owner匹配或任何查询能力。UTF-8/换行、同显示名不同完整owner、完整/部分/不可用状态及原三视图覆盖同时通过。收据`/tmp/codrax-marker-preview-width.vaF4BR/RED-public.log`、`GREEN-focused.log`（tool1.476s，exit0）；更早RED.log为测试编译错误，不当功能反例。末改全仓、增量race和干净版本双例继续单独验收。

长名称片已提交`efd86524c`，增量`GREEN-race.log`正式exit0（tool2.829s），干净原生构建`/tmp/hmc-measurement-ownership-clean-build-20260920.log`正式exit0，版本efd86524c84f；20:56:26开始固定2并行×1，无第三次追绿。853首轮全仓正式exit1（`/tmp/hmc-measurement-ownership-final-full-20260920.log`）：两个旧展示pin仍期待drilldown裸window/旁路“发生”须按已审正确语义重钉；另一个是真实进程域census限定句被StoreBlob裁到中间，不能改其旧断言去放行。后者正在保关键事实与口径同屏的窄修，仍不增加预算。efd末版全仓已经启动，早于这三个修正，不可当末改收据。

### 42.4 并行发现的教学冲突（独立小片，未冒充efd回放覆盖）

§41人工S机理错误的静态共因已确认：实际finalizer同时收到STATE-DURATION的“S状态不证明机理”和TRACE ANSWER SKELETON②的“which waiting is designed-in cooperation (e.g. sleeping for a downstream reply)”。后者预设正常/协作分类，违背前者证据边界。按同一业务/协议证据前提替换旧句，不新增分类门或答案扫描，也不把既有唤醒/IO/调度链降格；实际初始消息回归进行中。另evidence_pack的IO值再发布可能缺角色元数据，仍在只读核对，不将35/31错误简单归为模型波动。

原句已替换，①③④和各根因家族/链披露保持。公开TraceQuery→DefaultPromptAssembler→实际消息六变体（中英×S/D闭合/S无唤醒）正式RED→GREEN；`/tmp/codrax_sleep_mechanism_teaching_{red,green,race}_20260920.log`，GREEN agent2.142s/skill0.702s，race agent5.605s/skill1.850s，均exit0。无Trace不引入此教学、OnViolation仍为空、原生账与模型答案不变。efd回放不含本改，不作live签收。

进程域预览另已修：只将canonical window_stats原有census整块（普查/名册/fold/全部口径）前移至同节开头，无重复/预算扩张/路由变更。原失败断言未改，改名/成员TID/无目标/复合视图及原rank/marker预览针通过；`/tmp/codrax-census-preview.basimn/{RED-existing,RED-public,GREEN-focused,GREEN-race}.log`，GREEN tool1.364s、race4.821s正式exit0。两个旧测量口径pin仅重钉措辞并保完整数值/坐标/角色，`/tmp/hmc-measurement-ownership-old-pins-green-20260920.log`正式exit0（agent1.184s/tracefinding1.712s）。包含上述末改的全仓正在`/tmp/hmc-measurement-teaching-census-final-full-20260920.log`执行。

### 42.5 IO二次载体误作物理请求（独立修复，整答仍待验收）

参考仓亲读`config/indicators/io_latency.yaml:20–25`、`core/preprocess/io_ops.py:902–917`保留请求start/end/duration，`sleep_ops.py:198–260`独立裁剪状态交集。当前fixture请求1.005..1.040=35ms；issuer阻塞1.009010..1.040010=31ms，后者终点比完成晚10微秒，不能声称严格包含或互换。

本仓CriticalBlockingCandidate已将区间/数值转为已闭合issuer阻塞31ms，Type仍io_latency；evidenceFromCriticalBlocking再发EvidenceFact丢typed闭合/计量身份，工具evidence_fact继续只传summary/interval。成文authority仅按io_latency谓词解释为请求，实际519日志2515写“该请求完成唤醒证明未发布”，与2511/2513正确35/31闭合事实同时出现。efd正文尾部再次泛称storage完成唤醒证据不完整，不能直接归为模型波动。下一片只用已有typed请求计量字段决定请求解释；缺载体保留原观察而不自动生成请求缺证判断。不扫summary反推35，不以线程/相近数值拼身份，不改原事实或根因资格。

实现只替换该卡解释的来源判定：本record已有独立请求计量字段才解释该请求的完成证明；无计量身份的IO相关观测说明身份未携带，不再凭同名谓词生成“请求缺证明”。并未补齐EvidenceFact的计量身份或闭合字段，也未声称critical_blocking主Observation已携带完整闭合元数据；那些原生字段贯通仍留债。真实请求true/false/unknown、部分字段、旧无notes/仅source、同线程异artifact均保原语义。公开TraceQuery→真实finalizer初始消息先红后绿，中英×改名×异源8变体及原生JSON/模型/projection不变回归齐全。`/tmp/codrax_io_secondary_carrier_red_20260920.log`为实际功能失败；末版邻近race×3 `/tmp/hmc-io-carrier-final2-race-20260920.log`正式exit0（agent14.793s），独立只读复审通过。未用efd回放替本改签收。

eff3上一片完整全仓正式exit0：`/tmp/hmc-measurement-teaching-census-final-full-20260920.log`（agent90.555s、tool396.145s等）；它早于本IO修复及随后图关系修复，不当末改全仓收据。

## 43. efd固定双例人工0/2，子修改善与剩余问题分账（2026-09-20）

[机器](../../eval/parallel_selected_summary_hmc_measurement_owner_20260920.md)1/2，[人工](../../eval/parallel_selected_summary_hmc_measurement_owner_20260920_manual_audit.md)0/2，runner正式exit0；业务250秒、明确窗140秒，均40%上下文。eff3d1365及随后IO显示修复不在此次快照内。

明确窗own等待17/14/11ms不再错位，统计域也未改写成假状态端点，片段新账真实进入探索；但业务正文仍50ms窗口套6/1/44、漏LoadDocumentIndex和不相加口径。35ms请求/31ms线程等待与后台47ms写已分清，尾部仍误称storage完成唤醒不完整。明确窗旁路编造再睡/再唤醒时序，正文将fscache调用点推为特定资源/后端机理，均继续FAIL。详细行号与归属见人工单，不以新代码绿倒签。

两份正文归属收据available；两份mandatory根因旁路都生成，其中明确窗available但描述语义FAIL，业务因本轮未运行root_cause_rank且无可选typed席为unavailable空数组。Trace投影两份均在，不能把旁路存在误报成内容通过，也不能把候选空等同文件丢失。系统投影另有S与runnable“物理重叠”说明疑点，正在独立只读核对生成身份，优先于新能力扩面。13/79、66开放不变，不同版补跑第三例追绿。

## 44. 同来源行号不能证明同状态的物理重叠（2026-09-20）

§43系统图疑点已确定复现，不是模型波动。`runtimeTraceProjMarkAccountRelations` class(1) 只按线程+来源行号范围配对，漏核对计量的状态族与查询窗。公开root_cause_rank→Observation→Trace投影把cookie17ms/network14ms睡眠和各自1ms优先级反转可运行分量配成“同线程同状态族·物理时间重叠”；两种状态实际互斥。参考仓`core/preprocess/sleep_ops.py:198–260`逐段裁窗后以互斥state分支累计，行号包络/相同累计值都不是状态同一性证明。

本片仅修该显示关系臂：相同非空计量状态族、既有查询窗兼容性、无明确区间不相交反证，方继续原配对；限制置于等值镜像与差值账目两路之前。PIC的完整显示量含运行算力缺口或成分未知时，不能借其中Runnable份额或周围dominant S冒充单状态；已知纯Runnable仍允许原同族关系。只不发错误关系说明，所有候选/排名/时长/区间/链边保持。未改共享TwinKey、RSPA同源二分、其它关系臂、根因门或模型答案。

公开改名×中英先红后绿；同族正针、纯Runnable PIC正针、等值异状态、异查询窗、明确不相交、混合/仅running/未知PIC与原生投影不变均覆盖。正式RED `/tmp/hmc-account-state-final-red-20260920.log` exit1；较早red日志还含测试自身空ref查表噪声，不把噪声当产品问题。邻近SMR1/RSPA/CR2/状态窗口 GREEN `/tmp/hmc-account-state-green-20260920.log` exit0（tool5.975s）；末增纯Runnable正针、race、全仓和干净双例另外签收。独立只读审计提出并纳入等值早退、明确不相交及PIC复合计量反例。

边界留债：窗口缺失仍只是不否决，不是同窗证明；同族同行包络本身也不等于完整的物理发生集合身份。其它遗留关系臂的严格同源/发生集合证明与原始占用镜像去重继续开放，本片不能冒充全图关系已审完。`39becadd4`至`19568569c`已推送main；上轮人工0/2不倒签。

末版提交`de47561c847d`。新增纯Runnable正针与全部新图关系针`/tmp/hmc-account-state-final-green-20260920.log`正式exit0（tool1.169s）；相关race×3 `/tmp/hmc-account-state-race-20260920.log`正式exit0（tool156.528s，早于末增正针、生产代码相同）。完整全仓`/tmp/hmc-io-state-account-final-full-20260920.log`正式exit0（agent111.156s、tool422.891s等）；覆盖IO载体及图关系生产改动，末增测试另验。干净构建`/tmp/hmc-io-state-account-clean-build-20260920.log`exit0。

## 45. de475固定双例：机器2/2、人工0/2，不误销（2026-09-20）

[机器](../../eval/parallel_selected_summary_hmc_io_state_account_20260920.md)与[完整人工单](../../eval/parallel_selected_summary_hmc_io_state_account_20260920_manual_audit.md)，固定干净`de47561c847d`快照2并行×1，runner正式exit0；明确窗160秒/45%，业务354秒/43%。导航继承后续改动不在此快照内。同版无第三次追绿。

明确窗保留四候选及完整Trace投影，错误S/PIC物理重叠说明消失，旧旁路虚构再睡/唤醒也未复现；仍把已计价IO11最大错说成实际占用最大、从内核调用点推出后端机理、Binder零匹配推出排除结论，独立人工FAIL。业务恢复子业务40ms和8/1/31，但仍50/53ms混账、bytes误作扇区、真实相交区间说相邻和35+8拼算，人工FAIL。两份mandatory旁路都在；业务未调用rank而无可选候选，空数组不算内容通过。没有空答案或活跃流强制截断。

新增两条确定性系统供给/显示gap优先于模型散文拟合：

1. 业务标记导航继承前一次工具探测的TID100游标，遮掉真正由TID200发射、payload PID100的子业务。旧efd日志2390–2460实际命中；用户显式目标不是问题，须保留。按已有typed来源/参数修导航，不扫描用户prose。
2. PIC板头仍把三成员定位包络交集写成“成员区间重叠”，实际计量的三个1ms Runnable分量互斥；源头在方向板算术/定位包络helper，并非模型自写。仅纠正显示资格/中性限定，不能从包络补造物理发生集合或放宽可加性。

其余散文错误仅据此不足以证明模型波动；继续核对实际context优先级和测量身份。不得增原文关键词硬门、系统改写模型根因或以机器PASS销掉整答FAIL。HMC总账13/79已交付、66开放保持。

## 46. 业务标记发现不继承纯探索游标（2026-09-20，子片实现）

对照参考仓`core/preprocess/marker_ops.py:355–417`：明确tid过滤才限定该线程，否则枚举各itid，线程来自自身metadata，不从marker payload process身份反推。本仓既有用户焦点继承有其保护职责，不能照搬全局取消；本修只解除先前工具查询产生的游标对独立marker导航的意外限制。

`traceQueryApplyRequestModelTarget`在现有精确单目标选择后，只有已知探索来源且两个RequestModel副本均无活跃非游标目标，命名span_window或归一化event_types全为trace_mark的event_search才跳过隐式继承。显式PID/线程、用户/未知/空来源焦点、同身份混合focus+cursor、进程范围、自动补齐仍保旧域；时间/行号/名称/事件action过滤原样，既不清游标也不选新实例。其它调度/排名/无名span/混合与未知事件族保持旧继承。

公开Execute先执行window_stats产生TID100游标，再按改名子业务发现TID200、payload PID100，三种发现均先红后绿；旧焦点/显式过滤/自动补齐/显式窗反例同步通过。收据`/tmp/codrax-marker-cursor.laD0mS/RED-public.log`exit1、`GREEN-public.log`exit0（tool1.435s）、`GREEN-focused.log`exit0（tool1.106s）、`GREEN-race.log`exit0（tool4.222s）；`RED-description.log`是旧字节快照失败，不冒充功能RED。

Description仅替换原继承语句的一个子句，明确同一例外，未加第二套JSON字段或修补路线；字节34386→34688，sha256 `9928b42a0c6d19b17d594aed3efd4d773b78ed3048c2657f9ff827c2b79e44de`→`01e0b6eb30c958046fc6759bc9a4d169a03e5547caa0d8320cc06b0eb570d08d`，其余字节相同，按演进协议更新。既存h2/h3匹配A/B仍开放；de475回放早于本改，不能签其live。完整全仓及新干净版本跨模式双例另记。

提交`6ebc7a4bf`，主线独立定向复测`/tmp/hmc-marker-navigation-main-review-20260920.log`正式exit0（tool1.190s）。另§45唯一成文拒绝已独立复核：系统one-shot展示advisory后的无id补丁整次not_staged，交付的是此前成功正文，既有schema/提示/事务合同一致，不是失败patch误签成功；正确marker账及typed事实高于旧摘要的规则确已到成文输入。保持人工FAIL，不因模型未遵循再加硬门，完整证据见该人工单。

## 47. 方向板定位包络不等于实际分量重叠（2026-09-20，子片验收中）

§45的系统PIC段头问题属于通用方向板，不是PIC数值计算错。原有`TestTraceEnvelopeOverlapDoesNotMintPhysicalRelationAuthority`已禁止包络铸物理重叠权限，但中英文段头/图例却确定性写“成员区间重叠”“相加会重复计费同段物理时间”。参考仓`sleep_ops.py:198–260`及`marker_ops.py:548–642`均以每个原始状态区间与对象窗相交，而非拿最早/最晚定位范围当实际分量支撑。

- [x] 中英段头与图例单源，改为“定位范围相交,合计不可直加”，明确不代表计量分量真实重叠。保既有算术枚举、小计授权、候选/数值/排名/原生证据和Trace投影；既有精确`cross_direction_overlaps`不动。
- [x] 成文前两个`forbidden_by_typed_overlap`发射面与共享修向教学同源说明旧token仅表示定位包络交集，不能授物理关系。原测试将包络反例错误绑定到物理重叠教学，现改绑准确释义；真正物理交集的教学和测试保留。无新schema字段、答案原文扫描门或模型结论代写。
- [x] 公开root_cause_rank→观察→真实投影/图例，中英×任意改名，证明三个Runnable实际分量互斥而定位包络相交；PIC/混合/仅running/IO/runnable/sleep异构及原数据不变针。成文前两面也有三族反例，skill/context实际prompt受检。
- [x] `/tmp/codrax-elim-envelope.Tt3aCq/red.log`与`teaching-red.log`分别为展示/成文教学语义RED；`final-green.log`五包exit0：agent1.836s/context0.743s/skill2.477s/tool1.193s/types1.820s。
- [ ] 完整全仓、race和干净版本双例另补正式退出收据。de475机器/人工不签本片。

横向扫描继续登记其它消费者，不能以修该段头宣称全图关系闭环。已找到SMR1 class(2)将缺失/相交定位范围的`AccountRelDisjoint=false`默认发射“物理时间重叠”，以及IO显示fold按定位包络连通后用“同段IO”措辞的风险；需独立核对折叠身份/实际成员凭证，分别先红后绿，不自动删除原事实或改变计价。完整清单和排程随独立复审补记。

已提交`4bcc34e886e6`，五包race×3 `/tmp/codrax-elim-envelope.Tt3aCq/race.log`正式exit0（tool6.614s/agent2.618s/context4.594s/skill3.600s/types7.058s）；完整mark、双向图例、段头宽度`legend-green.log`exit0（tool1.277s）。干净原生构建`/tmp/hmc-marker-navigation-envelope-clean-build-20260920.log`exit0，未含后续关系未知态片。21:41:42启动固定该二进制2并行×1：业务Trace读模式（旧FAIL优先）+C单行修改的plan/apply/verify（跨模式安全覆盖），尚未以阶段成功签最终人工PASS。

### 47.1 包络关系消费者横向清单（只读静态核对）

| 优先级/状态 | 消费点与缺口 | 验收/下一步 |
|---|---|---|
| P1，正在独立修 | SMR1 class(2) `answer_document_mutation_runtime_smr1.go`普通家族与rank关系，仅同线程/状态族/兼容窗；`AccountRelDisjoint=false`含定位包络相交与时间缺失，但`answer_document_mutation_runtime_tree.go`账目说明默认断言物理重叠。class1普通关系也没有另发精确物理交集凭证 | 通用未知关系显示，不只按PIC/IO名字拟合；带洞家族与缺时间戳先红后绿；保RSPA同源二分、真正互斥和精确cross_direction |
| P1，待公开反例/方案 | `runtimeTraceProjFoldSameSubjectIONodes`→`runtimeTraceProjIOOverlapComponents`按Start/End定位包络连通，缺完整成员/查询身份核对，吸收无rank成员却称同段IO；旧针只覆盖包络外、不覆盖内部空洞 | 区分精确同发生合并与同主体证据分组；不能只取消fold后让行数cap吞原事实；保每条来源/值/口径和排名。不以本轮live未出现说已安全 |
| P2，静态边界疑点 | `answer_document_mutation_runtime_xerr1.go`sleep互指用sleep族/兼容窗/包络包含，生成真实sleep包含说明，尚未核完整状态账/成员集合；当前无新live反例 | 先以多段带洞状态反证，核typed状态账身份，再决定关系是否有授权；不借累计量等于窗宽补凭证 |

已排除的正常消费者不能随手退役：cross_direction关系有逐segment支撑、同board/身份/互返；守恒用真实union intersection；SelfGapSemanticOverlap逐run×semantic交集；语义subset有完整FamilyMemberLineRanges；真实包络互斥可安全推出成员互斥。共同方向是精确集合/定位包络/未知分层，不增加另一套宽泛提示或原文扫描门。

4bcc完整全仓已暴露旧`TestB1590aTraceDirectionSharedExtractionPreservesSkillBytes`字节pin未随这次审定新增限定句演进，skill包失败（全仓最终退出另记），不以定向绿掩盖。补正同时钉两层：完整新Body hash `68d5e164ebfe68509662de558c37168d22492e093b48b984e087c61b54762865`；仅去掉唯一新增共享限定句后，必须恢复旧hash `b1f03b587f5b72994525824b665b28c5490db3eed27c323aea0f88cd838de31e`，确保其它教学字节没变。RequiresTrace/OnViolation及共享句一次消费断言全部保留。`/tmp/hmc-envelope-teaching-byte-pin-20260920.log`正式exit0（skill1.098s），此片只更新测试，不改4bcc生产行为。

IO fold设计复核补充：连通分组可经A∩B、B∩C连接互斥的A/C，不能称同一物理段；StateAccountKey仅可证明完整调度状态账重复发布，不代表IO请求，FamilyMemberLineRanges也不是IO完整时间支撑。后续安全方向是保留紧凑展示及各peer自己的值/口径/证据/范围，改称同线程IO证据组；当前private peer缺独立scope，必须一起解决主行范围借给peer及多成员容量可达性，不能只改两个字就声称信息无损。该P1尚未施工。

## 48. 4bcc跨模式双例：机器0/2、人工0/2（2026-09-20）

[机器摘要](../../eval/parallel_selected_summary_hmc_marker_navigation_envelope_20260920.md)、[完整人工单](../../eval/parallel_selected_summary_hmc_marker_navigation_envelope_20260920_manual_audit.md)。干净`4bcc34e886e6`固定两例并行各一次，runner exit0；业务287秒/44%，C apply124秒/27%。无同版第三例，无空答案或活跃流截断。

业务本轮确实没有因果投影：分类器把因果解释声明为bounded_fact_set，现有typed报告权威按此收窄；mandatory根因旁路存在并如实报合同未启，不是文件遗失。先审分类教学/能力边界，不扫描请求词或让所有有限查询自动升级成根因。完整50ms业务套49ms局部账、35ms请求驻留误称设备处理量、1.045唤醒误称1.050业务结束及背景竞争推断仍人工FAIL。结构化工作关系行的49ms缺裁剪限定也独立审计中。此次导航没有命中新skip支，方向板没发布，更不能替6ebc/4bcc补签生产命中。

C仅一行源码更正，真实make test通过、隔离和原仓保护正确；required `gcc main.c`合同没有逐断言执行见证，终验保持unverified，端到端FAIL。归既有B2–B6下的native-command能力接缝：Make聚合PASS≠精确gcc合同，现有C教学又不造probe，单添PTO或提示不能产生不存在的执行生产者。后续须以typed能力/精确命令与来源快照/新回执闭环，不降验证杆。

4bcc首轮全仓正式exit1，唯一失败是§47所记旧skill字节pin；tool397.390s、agent95.623s等均过。`b2384f95f`已补精确增量pin，后续关系片的末版全仓另跑，不把该次红改写为绿。

## 49. 普通账目未知关系不再默认物理重叠（2026-09-20，子片实现）

§47.1第一项确认：普通SMR1 class1/class2的`AccountRelDisjoint=false`包含缺时间戳和定位包络相交，两者都不是实际分量重叠凭证。参考仓`sleep_ops.py`逐状态交集与`marker_ops.py`区间裁剪规则同样不允许由包络补造物理交集。本片保已有bool/配对及所有原数值、排名、链边，仅让未知显示未知；中英文行内句、完整/简版图例一致。RSPA同源闭合分账、true disjoint及精确cross_direction关系保原正枝，不扩大任何可加性。

真实投影三类（带洞状态族、缺时间戳、同行包络的两账）×中英×改名先红后绿；保两行互指和projection字节，6个精确正针同步保留。`/tmp/codrax-account-relation.KVbsxY/RED-projection-verified.log`正式exit1（12未知例红/6精确正针绿）；`GREEN-focused.log`exit0（tool5.428s）、`GREEN-race.log`exit0（tool38.600s，64个顶层测试）、`GREEN-legend.log`exit0（tool0.933s，10个图例测试）。独立只读复核通过；主线仅补一处过时注释，不改行为。末版全仓已启动，结果待正式退出。

这是显示资格子缺陷，不能代销整份Trace答案、IO fold或XERR成员凭证；HMC总账仍13/79交付、66开放。

关系片`6a0234da0`末版全仓`/tmp/hmc-account-relation-final-full-20260920.log`正式exit0（tool397.040s、types47.766s、tracequery115.326s、tracediag18.869s等），已覆盖b238字节pin补正。后续§50软教学与业务回执显示片在相关包编译完成后开始，不借此收据签其全仓。

## 50. 问题宽度与业务窗口分开，兼容缺字段另列（2026-09-20）

§48分类审计确认正确因果教学确已送达，并非系统完全没教：日志528–530包含维度表、范围独立性和主要阻塞原因例子。模型两次仍漏发`requested_answer_dimensions`，最终发bounded事实；首次只因目标引用错误被拒。不能就一次失败证明稳定模型波动，也不能据此绕过有限事实报告保护。

参考仓亲读`config/skills/launch_perf.yaml:55–70,99–130`先发现阶段窗再诊断瓶颈；`io_analysis.yaml:73–105,202–234`把查询窗口与根因问题分开。参考没有本仓scope枚举，只借其正交结构，不移植关键词路由或阈值定因。共享软教学现用短句明确scope是所求结论而非窗口长短/数量/业务次数，找一次业务范围只是导航；同一短窗可问事实、条件影响、拓扑或原因发现。schema与workflow单源，删去一处旧重复示例；原维度映射、finite优先级、runtime_work/frame独立决定及未知结论出口保持。

实际analyzer→adapter六种异构问题（含同一短窗的中文因果/纯事实对照）先红后绿；断言初始消息及真实tool schema都消费该说明，并不预填分类结果。`/tmp/hmc-runtime-breadth-range-red-20260920.log`exit1；四包定向GREEN exit0（agent1.094s/skill1.665s/tool1.318s/types1.852s），末版删重复示例后的race×3另验。此RED证明新教学未供给，不伪称确定性分类器已纠正真实模型。

兼容接缝独立留P1：provider schema把requested_answer_dimensions列必填，executor为旧/local兼容只强制部分顶层字段，nil维度成功返回，故本例没有因果role供既有一致性检查消费。若收紧presence必须审旧序列化/提供方兼容，只能读取结构存在性；本片不加门，不从用户原文或work flag推因果，不自动补模型未声明的维度。完整根因查询和有限事实都保原权威。

§48结构工作行裁剪口径已查清并另片施工：原工具及实际成文输入均保49ms查询内量/50ms完整量，系统BoundRow丢了范围后renderer裸写49，不是底层算错。按同record已验证字段恢复范围限定，绝不把49换50；同线程业务打点也不等于目标自身执行或因果贡献。真实公开渲染红绿及完整记录另记。

软教学独立只读复审通过；采纳删除共享句中不适用于schema相邻三元组的“below”尾语，避免再造误导。四包race×3正式exit0（agent5.070s/skill2.759s/tool10.909s/types4.086s）；最后纯措辞微调实际消息/skill复验exit0（agent2.035s/skill1.081s），收据分别`/tmp/hmc-runtime-breadth-range-race-20260920.log`及`final-green`同前缀文件。无分类准入或模型字段代写，真实模型是否遵循须另批验收。

## 51. 业务关系回执保查询内量与原始完整量（2026-09-20，验收中）

参考仓`cold_launch_ops.py:450–482`逐节点裁窗，`window_utils.py:22–29`统一区间相交；`marker_ops.py:396–411`/`trace_data_cache.py:294–312`则保原始ts/dur、按相交筛选。不能笼统声称参考的每个marker查询都已提供双口径；本仓native本已同时有selected_window、actual_window、actual_impact_ms，只是业务关系显示载体丢失它们。

最窄修复在确定性ordinary span编译时保存系统私有、值型的测量范围，query内MeasuredDurationMS不换值；实际renderer同时显示片段区间/查询范围，以及同record成对且自洽的可选完整范围/完整量。完整量不扩查询、排名或根因资格；缺失/无效/不包含/不一致字段不能借同名另一record、来源或query的量。模型wire仍仅observation_id+conclusion，BoundRow不序列化，原有语义工作凭证及frame限定分支不改。

普通业务行的关系说明收窄到“仅凭业务打点尚未证明对目标等待/响应的因果贡献”，不再把因果未证误写成业务身份/所属线程也未证。同线程sync/async marker仍不铸target_self_execution，也不代模型选择关系结论。

公开TraceQuery→合同→模型选精确id→绑定→实际中英文renderer，在49/50ms裁剪、完整50ms、显式窄窗10ms及任意改名上先红后绿。真实RED `/tmp/codrax-business-receipt.LOgSwl/RED-public.log`exit1（render0.848s），不是测试编译失败；初版GREEN-public exit0（render1.358s）。末版非法值、相等口径、clone/wire和相邻语义正枝验收另补；不以此签4bcc旧答案或新live已通过。

下一批排序：业务发现/因果组合旧FAIL优先（用户影响高且直接覆盖范围限定），明确时间窗D/IO多跳因果例并列（保护显式窗、各状态族、链资格与图关系）；固定一个干净新二进制恰好2并行×1。C native-command与Python补证已有精确能力缺口，在B2–B6修前不靠重复同样运行求绿。新例仍逐一人工读正文/过程/系统图/mandatory旁路，原审计不回写。

末版定向`GREEN-focused-final.log`正式exit0（types1.246s/render2.383s），末版race `GREEN-race-final.log`正式exit0（types4.293s/render2.991s，17个顶层测试）；同目录较早`GREEN-packages.log`两完整包exit0（types41.389s/render2.041s），但早于相等actual保守排除及补充非法字段针，不借其签末改整包。相等完整/片段口径不生成矛盾的“非查询内量”第二说明。末版全仓正在独立执行，生产已冻结。

§51末版已独立提交`7f29399e3`；只读复审通过：普通业务来源/精确id绑定未变、私有测量载体仅由确定性生产者写入、同record完整范围须自洽、不扩大关系资格、模型schema不变。全仓`/tmp/hmc-business-receipt-final-full-20260920.log`正式exit0（87包、13无测试包、零FAIL；agent83.221s/tool380.726s/types49.861s/render11.034s/skill11.786s/tracequery113.027s），覆盖§50及§51末改。干净构建exit0，`codrax 0.1.20260921`、revision `7f29399e38e2`；不借早版全仓收据。

## 52. 7f固定双例：机器1/2、人工0/2，组合合同与范围供给继续修（2026-09-20）

[机器摘要](../../eval/parallel_selected_summary_hmc_business_receipt_scope_20260920.md)、[逐项人工单](../../eval/parallel_selected_summary_hmc_business_receipt_scope_20260920_manual_audit.md)。固定干净`7f29399e38e2`，2并行×1；runner正式exit0。业务329秒/38%上下文，明确窗198秒/43%。两例都有最终答案和mandatory根因文件，无活跃流截断；同版不跑第三例追绿。机器PASS不能抵消下述人工FAIL。

- 业务完整50ms及5/44/1ms首次正确，但子业务40ms仍配更宽窗的9.5ms运行量（实际本实例8ms）；将未证完成唤醒写成“未触发任何线程唤醒”、内部字段漏到正文。正确8/1/31及false不是否定证明已在实际成文输入2424/2414行，不能谎报未供给。Trace投影为0、旁路`trace_root_cause_contract_not_active`；本轮模型关runtime_work_relation，也未选择结构工作回执，故不能把这次live当§51分支已命中。
- 分类器6次emit、5次拒绝。第4次（iter3，业务log931/936）真实携带`causal_diagnosis`、required `causal_contributor_set`与独立required `target_effect_verdict`、无fact_families，却被“一律禁止target_effect_verdict”规则硬拒。之后bounded_effect被root_cause标签拒，最后模型删去因果role并改explain才过。不是仅根据模型推理猜系统矛盾：两个要求的结构组合本来不可表达，确证高优先系统缺口。修复只允许独立已声明的因果维度与有限子判断并存，纯有限子判断不能自行取得因果权威；不从问题/答案关键词扩域，不撤有限事实保护。
- 明确窗仍保原2..2.020和完整系统因果投影/4个链上根因，但正文把2.020唤醒当sched-in（真实2.020020），把IO11ms说成整链最长阻塞，cookie17/network14套错区间。独立审计找到供给共因：`answer_document_final_decision_boundary.go`两处裸写`occurrence_interval`，把node统计/依赖定位包络命名为实际发生区间；系统图和旁路已经中性化，最后成文handoff未同步。本片另做公开红绿，不改时长、选窗、排名或根因资格。

问题优先级调整：先修这两个可复现、影响重试和答案事实的通用结构/供给缺口，再回到§47.1 IO fold、XERR及HMC开放能力。前轮presence兼容疑点暂不加门：本轮dimensions已显式存在，硬加必填不能解决混合合同。参考仓再次对照`launch_perf.yaml:55–130`与`io_analysis.yaml:73–105,202–234`：选定范围后仍组合各资源条件判断与整体瓶颈分析；不移植其概率Top3、阈值或推测性竞争裁定。总账13/79交付、66开放及旧FAIL均不变。

## 53. 成文末尾范围标签与原生计量语义对齐（2026-09-20，子片实现）

§52明确窗真实输入两处`occurrence_interval`确认是系统供给错误；不以模型已经收到其它正确限定句为由忽略矛盾。参考仓`sleep_ops.py:558–622`先在依赖分析范围统计状态，再单独找实际S/D片段；`marker_ops.py:560–612`和`window_utils.py:22–29`区分批量检索范围与逐行交集。本仓同样不能把投影node的Start/End直接称某状态一次发生。

两处末端权威行现统一写中性`locator_range`。仅唯一精确证据ID、同predicate/subject/端点、同MeasurementOrigins及有效运行时来源齐备时，复用已有`TraceObservationMeasurementWindowDisplayRole`补充“依赖分析窗口/累计状态范围”角色；缺身份、重复ID、异捕获、合并来源、变形端点保留中性范围，不借首个同名record解释。即使累计时长等于包络宽度，也不铸单次发生；真实thread_timeline单段时间仍在原输入中。无新增public schema/note key/硬门，不改原时长、定位端点、显式窗、排行、链资格、旁路或自动补齐。

真实公开TraceQuery→Observation→BuildAgentContext/BuildPromptContext+最终BuildInitialInstruction在中英×改名4组先红：`/tmp/codrax-final-window-handoff.tzomSN/RED-public-final.log`正式exit1，非编译失败。补相邻边界后`GREEN-focused-final.log`正式exit0（agent1.493s，60顶层测试）；真实timeline的14ms、2.002..2.016保留，17/14/11实测与1ms有效量仍分开，源观察序列化前后不变。主线只读复核通过；race/全仓末版结果另补，不能倒签7f人工FAIL。

活跃流保护独立复验：`/tmp/hmc-active-stream-protection-20260920.log`正式exit0（agent1.166s/llm1.969s）；真实SSE持续输出在4ms普通/evaluator预算下仍完整完成、不触发备用模型，包含嵌套telemetry/fallback。此次不改600/300/600默认值，也不把日志中的非流式阶段预算误说成活跃SSE截止线。

范围子片末版race同60项正式exit0（agent4.822s，`/tmp/codrax-final-window-handoff.tzomSN/GREEN-race-final.log`），生产冻结。横向仍继续核context根因板`representative_window`的“单次发生”教学是否与其实际producer语义相符；本轮明确窗未发布该字段，不冒称该句直接导致本次错误，也不说所有范围消费者已排除风险。

## 54. 独立因果问题与有限子判断可组合（2026-09-20，施工验收中）

§52真实业务日志的混合对象已经带齐两个required角色；问题是系统先逐维教学、再用“full scope禁止任何target_effect_verdict”拒绝合法组合。公开`EmitAnalysis.Execute`新针覆盖中英、单因/原因集合、维度反序、全工件/精确窗及独立work/frame决策，`/tmp/codrax-mixed-runtime.DGS9u1/RED-public.log`正式exit1：8个混合正例均撞全局互斥，8个fact_families结构重试无法取得保全部维度的确定修复目标；12个纯有限/optional/反向边界原本通过。真实analyzer→adapter消息的8例负针也先红，`/tmp/hmc-mixed-runtime-messages-red-20260920.log`正式exit1，证明矛盾同时存在于workflow和实际schema，不只是执行器一行代码。

修复边界：required因果角色与causal_diagnosis共同声明整体因果宽度，独立required有限子判断可并存且不得删除；有限判断本身仍不能授因果投影权威。optional因果角色、单纯有限判断或显式短窗均不能扩大有限范围。重试只修结构冲突，保全部维度及work/frame决策；不新增presence硬门、不扫描问题/答案关键词、不代模型改分类或写结论。共享教学与实际tool schema一起验收。生产末版/全仓/live收据待补，不提前签绿。

末版生产已冻结并经独立只读复审通过：执行器只收窄verdict互斥条件、让required因果维度优先保宽度；bounded+因果的修复提示同步保全独立维度和两个布尔。`/tmp/codrax-mixed-runtime.DGS9u1/GREEN-focused.log`正式exit0（tool1.927s/skill0.655s），覆盖新28格及结构重试收敛、旧有限/别名/诊断边界和local/missing-dim兼容。真实模型消息8例`/tmp/hmc-mixed-runtime-messages-green-20260920.log`正式exit0（agent1.156s），同例race正式exit0（agent2.421s）；schema/workflow各一次共享组合说明，旧互斥句负pin保留。统一全仓正在执行，未改role基数门、presence或任何结论生产者。

下一批以用户影响、确定性自冲突、跨范围保护和新增证据价值排序：旧失败业务完整响应/IO链例 + 真实donghu有限CPU频率影响例，恰好2并行×1、同一干净新二进制。后者用于防混合修复把纯有限问题误扩大；不更改case/oracle追绿。当前业务/明确窗旧人工FAIL均保留，HMC仍13/79交付、66开放。

## 55. 根因板代表窗口不冒充连续状态段（2026-09-20，子片实现）

§53横向项确认：context读取`occurrence_windows`首条，非直接读取node总包络；但原生`WakeupCausalOccurrence.Window`是递归分析窗，内部可含多种状态/多个片段。链路为`tracequery/query.go`递归窗口累计→OccurrenceWindows→`tool/trace_query.go`精确typed note→根因板首条提取。参考仓依赖分析范围与单段状态交集仍是两层，不能把参考或本仓的“记录”一词升级为连续实际状态证明。

生产只修根因板一段教学和相关注释：保原字段名、首条选择、端点、聚合值和预算，明确它是首条记录的测量窗口，可能含多个状态段；整行聚合值不属于这个窗口的持续时长。不新造区间、不改变排序、chain资格、自动补齐或侧车。

真实`donghu_tieba_frame.systrace`经公开TraceQuery→原生载荷/typed note→实际中英文BuildPromptContext，见证NetworkService-60595第1席首条34579.477038..34579.484273包含running0.481ms、runnable6.754ms、2个片段；整席有效量16.698628ms来自3个occurrence。`/tmp/codrax-board-window.SJjhHY/RED-public.log`正式exit1，中英两支均在错误教学断言失败，真实producer事实已成立；较早RawRef读取错误属于测试harness失败，不计先红。末版`GREEN-focused-final.log`正式exit0（context1.173s，18个顶层测试），精确同row保rank/subject/type/窗口/有效量，并核源观察前后字节不变。缺窗口/畸形首条不借后续记录补造。独立主线复核通过；race/统一全仓待正式收据。

末版同18项`GREEN-race-final.log`正式exit0；统一全仓将与§54冻结后一起执行，不能用定向通过代签全仓。

## 56. 85ca固定混合/有限双例与统一验收（2026-09-20，机器1/2、人工1/2）

§53、§55、§54分别提交为`f4cd76b31`、`6a020a3a5`、`85ca9941e`。§54末版tool/skill定向race正式exit0（7.516s/1.968s，`/tmp/codrax-mixed-runtime.DGS9u1/RACE-focused.log`）。干净原生构建`/tmp/hmc-mixed-runtime-clean-build-20260920.log`正式exit0；实际binary revision `85ca9941ec4a`、buildTime `2026-09-21T05:39:49Z`。统一末版`go test ./...`收据`/tmp/hmc-mixed-runtime-final-full-20260920.log`正式exit0：87个测试包、13个无测试包、零FAIL（agent94.687s/tool396.642s/context8.767s/types54.680s/tracequery121.723s等），覆盖三片末改。

22:40:43启动两例，22:45:15结束，固定snapshot `.codrax/tmp/codrax-selected-20260920-224033`，2并行×1、1800秒每例；测试期间源冻结，runner正式exit0。结果目录`eval/results/hmc_mixed_runtime_contract_20260920`；[机器摘要](../../eval/parallel_selected_summary_hmc_mixed_runtime_contract_20260920.md)、[逐项人工单](../../eval/parallel_selected_summary_hmc_mixed_runtime_contract_20260920_manual_audit.md)。业务例验证旧FAIL与混合宽度，真实H4例验证精确233.190ms窗内有限CPU状态/频率判断的边界；原case/oracle未改，无同版第三次追绿。

- **业务仍FAIL**：本轮真实保住required因果+有限子判断及work=true，分类3次emit/2次拒绝（前版6/5）；Trace因果投影、3个31/1/1ms链上根因及mandatory旁路恢复。两个业务工作回执50/40ms真实命中，子业务8ms运行量正确。正文却仍漏35ms请求自身耗时，把未证完成唤醒写成没有唤醒，业务50ms和查询51ms虽分别列出但状态账仍主用宽窗6ms而非业务5ms。准确值与不确定性均已在最终typed输入，不能靠第三次重跑、加关键词门或重复同义教学签绿。
- **真实H4人工PASS**：233.190ms四态、CPU4原始上限2100000kHz/实际558000kHz及binding未知均准确；窄D/IO清单0与Binder5次/3.094ms分开，未凭有上限即定影响。有限问题无因果图、schema2空旁路是正确边界，不是文件缺失。
- H4过程另发现通用上下文歧义：窄D/IO零条时，固定缺失说明笼统称“没有匹配的等待原因记录”，而同输入另有blocked_reason_records=50。最终正文没有受污染，仍独立修教学的条件与范围，不伪称这份答案失败；§57另记公开先红后绿。

旧业务/明确窗人工FAIL不回写，HMC仍13/79交付、66开放。下一工程片为窄空值教学及IO fold每成员口径/范围保真，XERR真实成员凭证、原始占用镜像去重、跨模式B2–B6继续开放。

§53–56已随`4638b2c2b`推送，远端确认；不是仅本地提交。

### 56.1 上下文准确性与阅读负担分开审计（P2观察，未实施）

85ca业务最终成文输入的`IO Measurements For Causal Interpretation`一节为17行、11520字节、10条观测；其中4条为不同查询产生的次生IO证据卡，缺独立请求计量身份。35/31ms及false不等于否定事实均存在，故当前失败不能归因于被截断或没给数据。多处query/source长身份与解释重复是阅读负担候选，不是已经证明的失败原因；不得将这些不同query的次生卡按同名或包络直接删成一条。

对照参考`io_analysis.yaml:73–105,202–234`，可借鉴按问题维度组织概况/分布/具体等待，但不移植首文件即全部、阈值判根因或概率TOP3。后续HMC-01.3应以精确源/对象/查询身份保信息压缩，验证上下文大小、所需事实可达性和异构答案；不再为当前单例追加同义说明、强制答案词形或降低验收。本观察不销账HMC-01.3。

## 57. 空的窄D/IO清单不清空其它等待证据（2026-09-20，子片实现）

§56真实H4输入已同时存在D/IO清单0、blocked-reason记录50及闭合Binder等待5次/3.094ms；固定缺失说明却把“窄状态清单为空”泛称“没有匹配的等待原因记录”。此为供给歧义，不是本轮最终答案错误。参考仓`sleep_ops.py:558–622`也是先统计窗口状态、再查具体S/D段与直接唤醒者，资源事件/等待机制不能由一个空状态集合一并否定。

只替换成文末端中英两句为显式“若/If 完整D/IO状态清单为零”：仅该集合无匹配D/io_wait或内核IO标记S段；不表示blocked-reason、Binder或独立IO完成闭合等待清单为空。缺失/不可用不等于零；原有不由S推主动休眠、系统调用或具体机理的保护保留。未新增段落、gate、schema、计数、模型结论或因果资格。

真实H4 attachment→公开TraceQuery→BuildAgentContext→实际final instruction中英先红后绿；`/tmp/codrax-h4-absence.EXGySS/RED-public-valid.log`正式exit1，三域共存见证先过，失败确为旧措辞；更早三次harness因缺SourceQuote未获范围资格，不计有效RED。末版46项`GREEN-focused-freeze.log`exit0（agent5.097s），同46项`GREEN-race-freeze.log`exit0（agent34.037s）；非零/缺清单/不可用/全缺数据、既有S机制及completion IO/Binder边界保留，源观察/投影/模型文档不变。主线独立只读复核通过，整包/统一全仓在下一冻结批验收；不借85ca全仓签此后改动。

已独立提交`5f7b827a9`，待下一冻结批全仓与推送。

## 58. IO定位分组保每条证据自己的范围（2026-09-20，施工验收中）

§47.1 IO显示片已按公开入口复现：真实业务fixture经`TraceQuery.Execute`的两个查询窗、四个视图进入观察账本，再经`ApplyAndPersistMutation`和最终renderer；中英×改名四形均实际命中旧“同段IO/same-segment”错误关系措辞。`/tmp/io-fold-scope-public-red.log`正式exit1（tool1.624s）。定位包络连通A∩B/B∩C不能证明A/C相交，更不能证明是同一个IO请求。参考`marker_ops.py:548–620`先按线程取检索包络、再逐节点与真实状态段求交，`window_utils.py:22–29`逐段裁剪；本仓显示分组不能替代这些成员级凭证。

实施范围仅tool私有显示载体和说明：保原分组、席位选择、数值/口径、排名、行数容量及链资格；改称同线程IO证据组，不称同次请求或全成员物理重叠。每条成员独立显示值/口径/[E#]/自身定位范围/自身查询范围，缺失或非法范围不借主行，非墙钟计数/评分不改成毫秒。私有来源保深拷贝，不把它铸成IO身份或新公开schema。旧斜线值组与末尾无配对引用退役，图例/明细同步；实际关系、事件、数字均不由系统补造。

现行public pipeline已有artifact分区，不把手工跨采集拼projection冒充生产漏洞；多采集正向隔离和多成员信息可达性仍须验。完整定向/race/全仓及新干净双例收据待补。该子片不能代销业务正文35ms遗漏、未证→否定、XERR成员凭证或HMC父项。

末版新增公开/传递包络/私有范围/源深拷贝/跨采集/40成员容量针`/tmp/codrax-io-fold-scope.pEObiN/GREEN-final-public.log`正式exit0（tool3.061s）；三节点原始RED为同目录`RED-transitive.log`。真实公开输出按原生record.ID映射引用，期望值/定位/查询均取原始record而非新增peer自证；中英×改名各形保tuple，整组确有peer与主行不同查询窗的见证。40成员为明确标注的合成projection→tree/完整明细→最终Markdown容量针，不冒称公开工具端到端。原47ms原生请求口径、复合评分非墙钟、分家族计量、链车道和rank席保护仍单列回归。主线逐行复审通过；生产冻结并启动覆盖§57–58的统一末版全仓。

下一固定双例优先两份旧人工FAIL：业务完整响应/IO链 + 明确2.000..2.020窗的多跳D/IO。前者检验组合/各把尺/业务线索，后者保护精确窗口/真正唤醒端点/所有链上状态/投影和必选旁路；一个干净新二进制2并行×1，不以机器regex替代人审，不改案例/oracle。

已提交`5adad7181`；末版邻近定向`GREEN-neighbors.log`正式exit0（tool2.750s），`RACE-focused.log`正式exit0（tool8.386s），覆盖NativeTiming/HMC、SMR1、SelfAll、IOFAM、New10、PTV6D行数与DispW1容量。干净构建`/tmp/hmc-io-group-clean-build-20260920.log`正式exit0，revision `5adad718155c`，buildTime `2026-09-21T06:07:47Z`。23:08:21以固定snapshot `.codrax/tmp/codrax-selected-20260920-230820`启动双例，results=`eval/results/hmc_io_group_scope_20260920`；统一全仓`/tmp/hmc-io-group-final-full-20260920.log`仍执行中，未提前销账。

XERR只读追查进一步明确待验证机制：payloadless阻塞值取span∩window的真实ThreadTimeline；另一sleep席可能来自按(thread,CPU)分桶再top8的SleepTop，并非完整sleep集合，包络包含未必证明成员包含。已在独立detached诊断工作树`/tmp/codrax-xerr1-public.YBNM4G`设计同capture同窗公开反例，不污染本批冻结源码；尚无可执行RED时不记为已确认或已修。反向说明将整个S+D+IO等待集说成落在sleep席，也列同批待证。

## 59. 5ad固定双例及统一验收（2026-09-20，机器1/2、人工0/2）

§57–58末版统一`go test ./...`已正式exit0：87测试包、13无测试包、零FAIL，收据`/tmp/hmc-io-group-final-full-20260920.log`（agent95.019s/tool407.871s/render8.272s/types55.056s/tracequery126.488s）。不是借85ca的早版结果。干净binary与固定snapshot见§58。

23:08:21–23:14:01恰好2并行×1，runner正式exit0；[机器摘要](../../eval/parallel_selected_summary_hmc_io_group_scope_20260920.md)、[人工审计](../../eval/parallel_selected_summary_hmc_io_group_scope_20260920_manual_audit.md)。业务340秒/43%上下文、明确窗169秒/45%；均有最终正文、系统投影和mandatory schema2旁路，无活跃流截断或空答案。两例未出现新IO证据组组头，不能用此次live签§58新分组分支命中；其公开端到端回归与live答案验收分开。

- 业务仍FAIL：50/40ms业务、31ms闭合S态IO及唤醒链保留，但35ms请求自身驻留仍遗漏，查询52ms的7ms运行被用来回答业务50ms内的请求耗时；又把31+2+7的残差12ms臆解为worker执行及切换间隙。最后实际输入已提供业务自身5/1/44、子业务8/1/31、请求35与等待31，也禁止残差编造，不能归为未供给。未证backup完成唤醒仍被正文说成没有唤醒/没有因果关联。机器FAIL虽只报缺35，人审不局限于regex。
- 明确窗仍FAIL：精确2..2.020、20/17/14/11ms测量及11/1/1/1排序显示保住，未再混唤醒和窗外sched-in；但模型断言S是被动而非主动等待、把opaque调用点说成页面IO完成导致整条链、漏最上游irq唤醒，并将wakee等待归成上游等待。原始调用点与机制未证教学已到最后输入，不能把正确IO状态直接升级为已知具体等待对象。
- 独立窗口来源债：本轮analyzer主动将trace中0.999..1.051配上业务问题原文quote并声明`explicit_time_window`，系统只检查quote存在与数值形状，最终各面均称用户指定。不是默认capture/query自动回填。另查到`bounded_selector`带数值也被系统主动晋升explicit，这是独立的可确定结构合同问题；必须分别处理，不以修后者宣称前者已修，不扫描用户原文数字/关键词加门。

旧FAIL不回写，父任务仍13/79交付、66开放。下一片先修公开反证成立的XERR错误物理关系，再处理业务选择器越权升级；上下文重复/缺身份压缩及模型过强推断仍留账，不以反复追加同义教学或同版第三次回放追绿。

§57–59及本轮机器/人工审计已随`d81a2c226`推送并获远端成功回执，包含`5f7b827a9`、`5adad7181`；不是仅本地已提交。

## 60. 睡眠账目互指不能凭包络声明物理包含（2026-09-20，已确认，施工中）

§47.1/§58的静态疑点已由公开入口有效反证，优先级提升为P1系统显示错误。独立工作树`/tmp/codrax-xerr1-public.YBNM4G`（5f7b827a9），`RED-public4.log`正式exit1（tool1.336s）：两种原始调度文本均经`TraceQuery.Execute(recipe=io_wait,pid=100)`和独立`thread_timeline`核真实成员，再经原生观测→投影→`ApplyAndPersistMutation`→最终中英文renderer，非手造投影或编译失败。

- 带洞形：同线程CPU0前后两段S共20ms被SleepTop保留；中间CPU1的5ms S因全局Top8未列入该席。实际两集合不交，20ms席的首尾包络却包含marker内5ms，旧文案仍称同段物理时间。
- 混合形：marker内阻塞等待9ms来自S4+D5；sleep席为24ms。旧反向说明把整个9ms等待称落入sleep，连分量口径也越权。

参考`core/preprocess/sleep_ops.py:558–622`先统计依赖分析窗、再找实际S/D段逐段裁剪；`window_utils.py:22–29`提供单段求交，不用累计量首尾包络替代成员证明。本仓当前XERR链仅有标量/包络，来源身份也不是集合包含凭证。最窄修复保所有数值、引用、链资格及原配对导航，只将专属互指/图例改成同线程等待账目对照、实际睡眠分量关系未证、不能直接相加；不凭时长等于包络补造确证正枝，已有精确状态集合等值去重/分割关系不动。末版公开红绿、相邻/race/全仓收据完成后另补，不提前销账。

末版七文件已从独立树移入主线，原选择函数体字节不动，生产只改XERR专属双语句/图例和说明注释。互指称“睡眠统计”而非“睡眠总量”，避免被读成完整全集。两个公开反例保20/5与24/9各自数值和引用，中英正针要求中性并置、负针禁错误物理关系；源观察与投影前后字节不变。旧负臂补零sleep/异主体/异状态/异query及payload双语，旧legend probe精确同步，未删除负断言。独立树末版37项`GREEN-focused-freeze.log`exit0（tool1.759s/tracequery0.899s）、同37项`GREEN-race-freeze.log`exit0（tool3.397s/tracequery4.497s）。主线复审通过，主线叠加§58的公开复验及统一全仓待正式收据；不以独立树早基线替主线整合验收。

主线叠加§58定向`/tmp/hmc-xerr1-main-focused-20260920.log`正式exit0（tool3.246s）；该命令tracequery选择器没有匹配测试，不冒称引擎复验。引擎末版定向/race见上独立树收据，主线统一全仓将覆盖。

## 61. 业务选择器不由额外坐标自动升级成用户明确时间窗（2026-09-20，子片验收中）

§59发现的两项范围问题分账：本次live是模型直接误声明explicit，仍开放；本片只修另一个确定性分支。原schema把`bounded_selector`定义为“业务/帧/事件选择器，用户未给精确边界”，parser却在scalar和time_windows两臂仅凭合法数值把它升级explicit；`ResolveTraceQueryWindowScope`随后公开为“用户指定”。真实quote存在只能证明选择器原文存在，数值结构更精确不等于来源权威更高。

参考`config/skills/io_analysis.yaml:73–95`将query time_window与user_specified/full_trace来源分列，可借鉴来源区分；其将“冷启动阶段”举在用户提供值旁的教学并不能验证精确数值来源，不能照抄为坐标凭证。本仓继续保自己的typed业务实例引用，不从选择器名字猜时间，不扫描用户/模型原文数字或关键词。

公开`EmitAnalysis.Execute`→RequestModel→`ResolveTraceQueryWindowScope`/实际中文Format先红：scalar、单成员、多成员三形都错误得到requested principal及“用户指定”，不是私有helper自证；第一次缺required因果维度的harness错误不计RED。最窄修法尊重bounded枚举并保原quote，清掉只属于explicit的额外坐标、给结构告警，不加新硬拒；真正explicit scalar/list、多窗成员和所有其它scope路径保持。因果完整报告仍由独立RuntimeQuestionProfile授权，业务原生accepted focus仍可自动补齐完整业务窗；不能以额外数值为业务事件命名坐标，更不能默选业务实例。末版公开/相邻/race及统一全仓待补，不提前签已推。

末版三文件冻结，独立只读复审通过。有效RED保存在执行回执（session30682、chunk f6a572、exit1、tool1.120s），当时未持久化日志，不提供不存在的文件引用。末版`/tmp/codrax-bounded-scope-20260920.YaI48u/focused.log`四包exit0（tool2.784s/types0.582s/skill0.941s/agent2.604s），同目录`race.log`四包exit0（tool11.347s/types3.488s/skill1.614s/agent4.684s）。包括scalar/list/空或畸形/混合字段软清理、失锚quote不得借member取权、真explicit及多窗不成包络、full/unspecified/missing、无附件兼容、required维度保持；公开Emit→accepted native实例→系统补齐真实1..1.05而非.999..1.051，仍标查询窗而非用户明确窗。有限事实/有限影响不授完整报告，关系/系统概览仍保持原独立宽度，全部scalar/list交叉验证。未找到需要修改的工具字节hash pin；实际schema及相邻消息针已跑，不以没有pin免除全仓。

§60–61统一末版全仓正在执行，收据`/tmp/hmc-scope-xerr-final-full-20260920.log`。下一批按新信息量与跨模式保护选jank字段只读清单（超过2^53原始整数、独立时钟/appid与完整成员集）+Java plan-only（非Trace输入、精准补丁与规划JSON），同一干净新二进制2并行×1。两项均不是旧业务/明确窗失败的替代通过，也不能代销写模式补证B2–B6；前两例正确证据已给但模型未遵循的部分保留，不反复用同例跑到绿。

## 62. 5582异构双例与§60–61交付收据（2026-09-20）

§60 `6d7e340bc`、§61 `3f7f7a4c8`及审计`5582d0fd0`已正式推送main（远端d81→5582）。统一末版全仓正式exit0，`/tmp/hmc-scope-xerr-final-full-20260920.log`：87测试包、13无测试包、零FAIL；agent102.283s、tool409.629s、tracequery133.900s、types61.708s、render9.460s、hitraceconv164.958s。干净构建`/tmp/hmc-scope-xerr-clean-build-20260920.log`exit0，revision5582d0fd03eb、buildTime2026-09-21T06:35:09Z。只覆盖此时冻结源码，不用于签后续Java改动。

23:35:33–23:38:10固定snapshot`.codrax/tmp/codrax-selected-20260920-233533`，2并行×1，runner正式exit0。机器2/2、完整人工0/2；[摘要](../../eval/parallel_selected_summary_hmc_scope_xerr_read_write_20260920.md)、[人审](../../eval/parallel_selected_summary_hmc_scope_xerr_read_write_20260920_manual_audit.md)。两例均未命中新XERR互指/bounded数值软清理，不把异构保护当新分支live见证。

- 卡顿清单157秒/28%上下文：3条、7/4/2帧、70/40/20ms、超过2^53整数、原始头行时间均正确，同名子串/非法数字正确排除；但把marker PID201写成发射线程PID（实际发射TID/TGID101），并输出内部time_domain状态词。最终实际成文输入已清楚分列这三种身份并教普通语言，故留答案质量债，不加原文硬门或同义提示。该有限事实问题不需要因果投影，空mandatory schema2旁路`trace_root_cause_contract_not_active`合理；不是Trace能力缺失或文件未生成。独立人审复核一致。
- Java计划99秒/28%上下文：单行retrun→return补丁精准且fixture未动，plan-only不冒称apply/verify通过。实际6次发射、5次planner拒绝，读成文reject=0不能隐藏写规划重试。最终附带probe含非法默认包import，是模型错误；另发现任意合法完整Java主类会被系统错误嵌套，下面单独确认修复，不以更宽词面接受外部命令包装器。当前完整计划人审FAIL。

原业务/明确窗FAIL和13/79已交付、66开放不变。模型直接错误explicit来源、真实成员集关系/镜像去重、上下文成本与读者词汇、B2–B6均继续留账，不因本轮机器PASS或局部代码交付整体打勾。

## 63. Java验证程序不能依赖隐藏固定类名（2026-09-20，子片已交付；9月21日收据见§67）

§62 live揭示两项独立问题：非法`import Main;`不能由系统默认修文；完整`public class ProbeMain`却也是schema所称source-level program，执行器仅因不叫`CodraxVerificationProbe`就把它整个嵌进main，是确定性合同缺口。`run_tests_verification_probe.go`源准备/临时文件/启动主类，以及`verification_probe_syntax.go`预检文件名均硬编码该内部类名；公开schema与实际模型教学未声明这种限制。不应为内部装载限制强迫模型记忆名字，也不把合法源码改坏后的编译错误归给模型。

公开RunTests RED正式exit1，`/tmp/codrax-java-source-unit-20260920.PzwKiT/red.log`（tool2.068s）：实际生成源、javac文件名、java启动类均观察到错误。使用边界观察fake JDK核真实调用参数/写入源，并非真实Java编译。当前主机Java launcher在但无JRE；缺parser不算语法正确，也不能用环境缺失触发新硬拒。独立只读复核同意以已有Java AST识别顶层源单元，syntax和runtime消费同一源/文件名/主类准备结果；保源码字节、包名、旧snippet与固定类入口，不重命名/删import，不从注释字符串猜入口，多主入口不猜。AST子集限制不成为emit新硬拒，原javac明确语法诊断和changed-code耦合门不动。

本片优先于继续堆Trace散文教学，归HMC-16.4/18.5写验证兼容子缺陷，不新增父任务或代销B2–B6。实现及末版回归待验，不借§62全仓或计划machine PASS签新代码。

末版四文件已冻结，主线及独立只读复核通过。完整源按唯一公开顶层类型决定文件名，按唯一直接main及原包决定启动类；固定类、任意改名、无public辅助类、String[]/变长参数、枚举和接口入口均覆盖。局部类加语句仍走原片段包装；只搬原本在前面的import，错位import和坏语句保原相对顺序交javac，不能修成合法。完整源中无关方法体的解析ERROR不否掉精确入口，片段不再要求第二次解析无ERROR才允许原包装；编译器仍是语法权威。多入口/未知入口不猜，不改源、耦合门、schema或教学负担。

公开回归观察实际源码、临时文件名、带包启动参数与编译/运行两份收据；非法完整import及坏片段编译失败不得启动java。冻结版定向`/tmp/codrax-java-source-unit-20260920.PzwKiT/focused.log`正式exit0（tool18.455s），覆盖取消、launcher子进程树、收据与相邻语言探测；race与主线全仓另待正式退出。真实JDK针明确skip（本机无可用JDK），fake JDK只验证搬运/失败传播，不能作真实Java编译验收。250ms解析失败、未知新语法、嵌套或外部继承入口仍有支持边界，未声称所有Java程序或旧plan答案已通过。

冻结版同范围race首次`race.log`正式exit1（tool33.401s）：唯一失败是既有`TestVerificationProbeSyntaxLauncherTreeCancellationB1715/java`10秒内未等到child-started，没有数据竞争报告。源码不变，独立取消针`race-cancellation-recheck.log`exit0（3.533s），随后完整同范围`race-recheck.log`exit0（26.076s）；首次失败保留，未确定原因，不宣称已证环境波动或修改测试阈值掩盖。定向/复核各64个顶层PASS、4个环境SKIP（真实JDK及3个Node针），不记为68个实跑全过。统一全仓收据另补。

跨日补验（2026-09-21）：只给测试进程配置本机应用内Node，以上3个JavaScript邻接针实际执行全部PASS（含11个包装变量/正常模块/真实语法及断言失败子例），`/tmp/hmc-java-source-unit-adjacent-node-20260921.log`正式exit0（tool1.487s）。未改系统PATH或源码；不反改原4个SKIP的收据，真实JDK仍缺失。

首次主线统一全仓`/tmp/hmc-java-source-unit-final-full-20260920.log`正式exit1（session50192）：86测试包通过、13无测试包、1测试包失败；唯一失败为既有IO跨窗测试的覆盖前提，tool428.142s。不能把Java定向通过写成统一全仓通过。具体复现、原因和不降断言的测试修复见§66；Java四文件保持冻结，等待修复测试后的全仓正式收据。

源单元修复及架构说明已独立提交`f21aaa747`，尚待统一全仓/干净构建及推送收据；不以本地提交当交付完成。

## 64. 直接误声明明确窗口的来源边界（2026-09-20，只读设计，仍开放）

§61仅收确定性自动晋升分支；§59模型直接发射explicit的失败不在其修复范围。本轮主线与独立席核对`emit_analysis.go::parseRuntimeArtifactScopeProfile`、`trace_query_window_scope.go::ResolveTraceQueryWindowScope`及业务focus消费点：坐标、scope与source_quote都来自同一模型声明，引用存在只证明原文片段存在，原生查询结果只证明查了什么，均不能独立证明用户明确选择这些坐标。不能新增`origin=user`模型枚举后自证闭环。

参考仓再次亲读`config/skills/io_analysis.yaml:73–95`，其user_specified也是llm_decision输出，且将非数值“冷启动阶段”与数值窗并列，不提供更强凭证；`launch_perf.yaml:55–70`由阶段工具产出thread_queries，能借鉴的是原生派生范围的直接传递，而非用户数值权威。现有已接受业务实例引用同样可证明来源/线程/完整范围，仍不能反向改称用户明确数值窗。

可靠后续方案需宿主入口拥有的选窗/确认凭证：绑定请求版本、工件/时钟域/单位及有序成员；保重复、重叠、嵌套，不合并包络；模型只能引用既存凭证，不能通过工具JSON自行签发或由历史展示字段恢复授权。缺凭证的提议坐标仍可探索，不因此增加因果资格或删掉原生业务focus。改造须一起覆盖范围来源显示、explicit对业务focus的避让、实例包含约束及多窗补齐；只改显示标签不足。

在保留现有自然语言单/多窗兼容、不解析原文数字语义、又不新增确认入口三项同时成立时，当前输入无法可靠区分伪explicit和真explicit。故本轮不暗改协议/强制确认、不以二次模型裁判或追加同义教学代替根修；入口能力与迁移需另行设计，作为HMC-02.4/16.4范围来源子项继续开放。完整Trace问题宽度、显式窗现有行为、自动补齐及链上资格不动。

§62–63审计文档`e2ecb30d0`已正式推送main（远端5582→e2）；本节仅记录设计边界，不计新增实现交付或抵销旧人工FAIL。

## 65. 旧写验证FAIL的下一片顺序复核（2026-09-21，只读，B2–B6不销）

主线及独立席重读§48 C例实际日志：`hmc_marker_navigation_envelope_20260920/patch_c_typo-20260920-214142/run-1.logs.all.log:1913–1914`第一次make test打印cc编译再运行，2303–2304第二次只有运行已有main。最终report只有make-test总体结果，不含gcc命令真实argv/逐合同断言。不能从stdout里的cc文字补铸执行凭证，也不能把后一次测试PASS当重新编译通过；shell/Python命令包装器被拒不是需要放宽的故障。

`SourceCheckExecutionReceipt`目前明确只授精确输入路径的syntax-only资格，没有计划/补丁代次、源字节、实际argv与合同绑定，不能直接升级为command_result；`run_tests.go`原生PTO匹配正确地要求candidate、suite与assertion-scoped结果。C要闭环仍需结构化命令谓词及真正命令执行生产者，绑定实际程序/参数/cwd/输入快照/执行ID，以及被后续消费的本次输出工件；编译成功只证明对应命令结果，不能证明输出/异常/业务语义。不解析旧合同subject或中文expected来猜命令，不追认旧Make结果。

下一实施片仍优先B2的controller补证授权，后接B3/B4已有原生断言的只读补登记：旧Python例已经有真实`test_non_integer_float_rejected`结果，可复用生产者，缺的是安全绑定与重跑；C不是补一个PTO就能绿。B2需把补证批永久的禁止源码修改身份与当前可消费grant分离，绑定run/目标batch、仍贡献源码的source plans、合同完整定义及工作树HEAD/源字节/缺失/文件类型。沿§30.5既有设计，不将合成累计计划或run历史事件当新授权。

验收反例优先复用公开normalizer→Apply→Emit链：合法产补证批后，保持HEAD/status但改变同dirty路径字节；不得新增绑定，仍必须拒源码改动，且旧报告和普通verify-only不能被抹掉。连同跨run/batch、restore剔除计划、工作树重建、不可读快照与JSON/resume逐项验收；这里只读列出待运行矩阵，不伪称已执行或完成B2。总账仍13/79交付、66开放。

## 66. IO跨窗公开测试须稳定覆盖不同原生口径（2026-09-21，测试修复已交付）

§63首次全仓唯一失败为`TestIOFoldScopePublicQueriesKeepPeerRanges`的尾覆盖断言：没有构造出查询窗与组主席不同的折叠成员。独立席在原版连续30次精准复现25 PASS/5 FAIL，`/tmp/codrax-iofold-audit-20260921.Mjvz3F/count30.log`正式exit1（session61710，tool26.238s）；5次均只失败于旧119行，改名子例、实际数值/证据/定位范围/查询范围配对及输入不变性均通过。不是通过放宽断言抹去生产回退。

源码闭环：临时trace路径写入Result JSON，再进入blob内容哈希及EvidenceID；相同数值的节点最后按EvidenceID排序；同事实去重保留首个代表的标量查询窗，身份键不含查询窗，并合并原生来源/证据。原夹具在两个窗重复查询全部视图，可能合法保留同窗代表，因此临时路径变化就足以改变尾覆盖前提。显示fold按明确groupOrder和slice遍历、按最大值选主席，不是随机map选席；成员直接复制自己的范围/来源，未发现借主席窗口的生产回退。另一聚合路径的MergedQueryWindows不能当此路径的保证；本次也未打印每次失败具体代表ID，不伪称已观察到那一对ID。

修复范围仅测试：宽窗公开`critical_blocking_calls`产生backup提交线程到完成线程的47ms请求驻留记录，窄窗`root_cause_rank`产生同线程45ms窗内背景记录；仅按精确typed family筛选，原记录完整进入ledger，再逐个用精确证据ID对照最终中英渲染。两条原生值不同，不依赖同值代表ID碰巧排序。不改生产排序、不编辑合成systrace或原生观察的数值/ID/范围/来源，不删除旧尾断言；每个改名与语言组合的跨窗覆盖另加强为必达。这里的真实公开查询指合成文本经真实工具发布，不冒称live捕获；背景身份保持，不注入链资格。前两次候选探索失败未作为有效RED，旧版全仓和count30才是修复前收据。

末版一文件冻结并经主线及独立只读复核。`/tmp/codrax-iofold-fixture.dtITy4/GREEN-count30.log`正式exit0（tool7.529s，30/30）；相邻13项`GREEN-focused.log`exit0（2.909s）、同13项`GREEN-race.log`exit0（4.089s）。原值/证据/范围/观测字节不变与所有旧断言保留。叠加冻结Java四文件后的统一全仓为`/tmp/hmc-java-source-unit-io-fixture-full-20260921.log`，当前执行中，尚无正式退出；不能提前签绿或销HMC父任务。

测试修复已单独提交`b380060de`并正式推送main（e2→b380，session33496 exit0），不是仅暂存；本次未修改任何生产IO行为或旧live答案verdict。

## 67. Java源单元与IO测试末版统一交付收据（2026-09-21）

叠加§63冻结四文件与§66末版测试后，`/tmp/hmc-java-source-unit-io-fixture-full-20260921.log`正式exit0（session20120）：87测试包通过、13无测试包、零FAIL；agent96.512s、tool408.234s、orchestrator26.059s、tracequery129.084s、types49.224s。测试运行期间只有git提交/文档操作，源与测试末版哈希保持；这是新一轮完整回归，不把首次失败日志改绿，也不借早版通过结果。

干净工作区构建`/tmp/hmc-java-source-unit-clean-build-20260921.log`正式exit0（session55334），`./codrax --version`实测为0.1.20260921、revision `b32a10d98a34`、buildTime `2026-09-21T07:19:22Z`。`f21aaa747`代码与`b32a10d98`文档已正式推送main（b380→b32，session19947 exit0）。没有再跑同例追绿或用plan-only代替真实编译；缺JDK、首轮取消针的未定原因以及旧Java计划非法import仍按§63保留。源码搬运修复不修写错的模型源码、不松changed-code耦合与原生证明门，也不要求新JSON字段或内部类名。

Trace选窗、因果投影、链上根因资格、背景隔离和自动补齐均未改变；600/300/600秒超时默认值及活跃流保护未动。旧业务/明确窗/卡顿清单的人工FAIL、明确窗来源权威和B2–B6继续开放，HMC父账仍13/79已交付、66开放。本片是确定性子缺陷与测试可靠性的收尾，不是所有开放项已完成。

## 68. B2可达性实测：新探测准入不等于复用旧证明（2026-09-21，诊断结论，未实施B2）

在独立detached诊断树`/tmp/codrax-b2-snapshot.9QQFEe/tree`、固定`e2ecb30d0`新增测试，主线生产未改。不是手造proof事件/报告：公开Emit源计划→ApplyPatch→真实applyPostHook检查点→实际Make语法检查及RunTests报告→normalizer产生补证批/事件→ApplyWorkflowDecisionToRun→公开Emit；之后保持HEAD和同一` M widget.py`状态，将已脏源码的函数返回值2改为3。此流程只核现有probe车道与来源边界，不冒称完整B3或LLM live。

结果：现有probe-only计划仍可创建；两个发射器均在source-free sentinel边界拒绝PTO，另拒源码修改。再执行真实RunTests，`assert widget.value() == 2`在新源码上实际退出1，产生新计划的新执行ID/时间/工作目录收据及非权威比较器诊断；没有assertion-scoped通过、VerificationConfidence或target执行证明，旧report保持原PlanID和原字节。独立Make语法检查仍可通过，因此总体Passed=true不是这个模型比较器已获行为权威，更不是旧报告被冒充新结果。

首轮`reachability.log`因测试错误预期“非权威探测失败必使总体Passed=false”而FAIL；这是harness预期错误，不计产品RED。主线复核并运行修正为逐执行收据/证明资格断言的末版，`reachability-corrected.log`正式exit0（session2533，orchestrator1.446s）。PTO载荷仅验证无条件sentinel禁入，未搭建有效测试文件/active合同，不能据此宣称原生绑定端到端已验；本例也没有验证required合同的最终终态或所有过期grant组合。诊断测试仅留独立树，未混入本批已全仓验收的主线。

设计收窄：未来B2精确快照授权仍是开放原生只读登记能力的前置条件；现有“重新读取当前源码并运行新probe”与“接受/复用某个旧证明”必须分离，不能新增一个把安全重新验证也冻结的过期快照硬门。永久禁源码修改身份、旧报告独立归属及最终required合同缺证保护继续保留。§30.5的B2–B6均未销账，不能把这次反证PASS说成旧Python补登记FAIL已修。

## 69. 普通写计划引用已有测试不必制造测试改动（2026-09-21，子片验收完成）

ROI调整：在B2–B6新增原生只读登记之前，先修首轮就能使用的合法通道教学。`normalizeProjectTestObservations`早已允许不在`changes[]`中的既存测试，`test_surface`也按该声明选择精确文件执行；但共享行为合同说明及普通语言错配修复都要求“include that test file in the bounded plan”。这与同页`MUTATIONS ONLY`的实际修改集合说明形成歧义，会增加模型决策负担。它是可复现的上下文教学缺口，不是新发现的运行时准入故障。

旧§30双例的Python首轮确实受影响范围覆盖：`hmc_context_teaching_crossmode_20260920/github_issue_dateutil_relativedelta_float_symptom-20260920-033511/run-1.logs.all.log`第1070/1188行投递旧说明，第1161行保护既有测试，第1313–1343行实际读取该测试（1337–1339为`assertRaises(ValueError)`）；原计划有required `float_type_check`，旧报告中相同断言通过但原计划没有`project_test_observations`。这些证据不证明这句说明单独导致旧FAIL，不能事后给旧计划或报告补映射。

实现：普通规划与普通修复说明共用`NativeProjectTestObservationBindingTeaching`。已检查、未修改的测试只引用`project_test_observations[].test_path`；确需新增断言且已授权时，才把实际测试改动列入`changes[]`。仍由模型选择精确suite/assertion/合同映射，JSON原生数组形态不变；source-free计划继续禁止文件修改及原生登记。没有新增schema、证明资格、原文扫描门或强制测试改动。

参考仓复核：`core/skill_executor.py:326–448`的resume校验从既有步骤读取manifest/package并校验独立答案工件，而非改写输入工件以便登记结果；`core/llm_contract.py`及`tests/test_llm_contract.py`分开处理成员范围、字段身份与原始值验证。可借鉴“输入引用与结果声明分离、单源合同”的边界；参考仓没有本项目的源码修改/PTO执行合同，不能声称直接移植该能力。其散文查重硬拒规则不引入本仓。

验收记录：先改实际skill、planner初始消息和公开普通修复针，`/tmp/hmc-existing-test-binding-red-20260921.log`正式exit1（5针失败）；修共享说明后，同组加相邻证明权限回归`/tmp/hmc-existing-test-binding-green-20260921.log`四包正式exit0。公开Emit→Apply→RunTests已有测试字节保护、错标识/失败/skip反例及末版全仓/race待完成，不借此前收据签当前片。后续固定干净版本双例优先旧Python ordinary apply与明确窗多跳D/IO只读保护；旧人工FAIL和B2–B6未销，父账仍13/79已交付、66开放。

公开运行时补针已完成：`project_test_observation_existing_file_test.go`五场景（full正例、skeleton→change→finalize正例、错assertion ID、真实断言失败、skip）均走真实临时git仓的Emit→ApplyPatch→Python unittest RunTests，不手工设置Applied或构造报告；只有两个精确正例获得合同证明，三负例不获证明，测试字节及git diff证实始终只改源码。`/tmp/codrax-existing-pto.zGA1hF/GREEN-final.log`正式exit0，tool 2.276s，无SKIP。此链路本来合法，不能称为运行时RED→GREEN；红点是上面的教学针。共享说明及相邻权限四包race `/tmp/hmc-existing-test-binding-race-20260921.log`正式exit0；新增公开针race及末版全仓另记。

末版新增公开五针race `/tmp/codrax-existing-pto.zGA1hF/GREEN-race.log`正式exit0（tool3.619s，无SKIP）。统一全仓`/tmp/hmc-existing-test-binding-full-20260921.log`正式exit0（session21982）：87测试包、13无测试包、零FAIL，tool398.057s、agent96.138s、orchestrator25.580s、tracequery120.744s。代码已提交`504883389`；干净构建`/tmp/hmc-existing-test-binding-build-20260921.log`exit0，revision50488338970c、buildTime2026-09-21T07:45:56Z。本全仓签本片冻结源码，不用于签后续planner预算及Trace文案改动。

## 70. 504883389双例：功能改善与完整人审分账（2026-09-21）

固定同一干净新二进制，2并行×1，runner正式exit0：Python普通apply235秒、明确窗多跳IO192秒，机器2/2、完整人工0/2。详细[人工审计](../../eval/parallel_selected_summary_hmc_existing_test_binding_20260921_manual_audit.md)与[机器摘要](../../eval/parallel_selected_summary_hmc_existing_test_binding_20260921.md)保留原始结果，不重写旧FAIL或追加第三例追绿。

Python最终仅改源码，4条原生测试及probe真实PASS、交付树resolved，模型正确声明未改测试的4条PTO；但本次合同全为planning-only，VerificationConfidence只有source_compile_ok，不能代销旧required合同或B2–B6。实际过程5次emit/4拒绝，机器成文reject=0不代表规划零拒绝。确认普通planner一批两项错路径失败就耗尽2/2预算，下一步refinement推荐的repo_map不可用；已有emit-repair/rollover最终恢复，不能写成永久死锁。按失败观察轮而非并行失败调用计费是最窄泛化修复，成功读取及source-free补证限额不变。另有分析器规划合同输入12与预期1个月结果错配，未获得行为权威，继续留账不代写合同。

Trace明确窗/IO11ms/三条用户线程有向边/优先级候选边界/必选根因侧车均正确；D/IO根停递归不是漏掉已知IRQ，Harmony来源也有真实入口依据。但系统投影把状态覆盖20ms写成原因全解释，且把sleep未计价方向统一叫自身工作量，是两项确定性文案语义缺口；正文睡眠称工作贡献与内部字段泄漏另留账。此次不把已有正确主IO/投影删除或改成背景，也不额外收窄用户明确窗。先修两个typed生成点，保所有原数值、根因资格和补齐路径。

ROI据实重排：普通预算失败批计数与Trace覆盖/等待方向并行施工，各自红绿后收据再销子缺陷；B2–B6及其源快照授权仍继续排队，不借本轮功能成功跳过权限前置。HMC父账13/79已交付、66开放不变。

§69实现`504883389`及本轮审计`1b17e1b4b`已正式推送main（638→1b17，session35222 exit0）。全仓和干净构建属于§69，不能据此签收以下新生产改动。

## 71. 普通规划给并行取证失败留一次纠正反馈（2026-09-21，子片验收中）

§70真实写入例暴露的是预算计费单位错误：两个同一轮提前选定的失败调用，在模型看过任何反馈前就耗尽2次失败限额。后续成功恢复不能抹掉前面的不必要拒绝与重复规划。`plannerEvaluator.ObserveToolResults`本来已经有精确的当轮工具结果边界，最小修复仅将普通handoff、结构发射修复、验证失败修复三条通道的失败计费改为每观察批一次；成功仍逐调用，混合批同时记成功和一次失败。source-free补证专用通道仍逐read_file调用计费，发射拒绝不重置，搜索/源码修改仍禁；原硬迭代cap、成功限额、3次emit拒绝rollover均不变。没有新字段/教学负担或原文扫描。

参考仓`core/skill_executor.py:333–430`在一次resume契约校验返回整份errors与need_retry，计数是一次修正尝试而非errors数量；可借鉴“反馈机会是轮而非同轮错误条数”，但其工作流不是本仓planner并行读取，不照搬权限或散文校验规则。

新增真实BaseAgent.Execute针：真实文件工具先同批read_file/grep错路径，次轮目录导航、再读源码及测试，最后真实emit存储单源patch，源字节未改；旧版第二轮schema已关读取，`/tmp/codrax-planner-read-round.7jDVLt/RED-final.log`正式exit1（agent1.194s）。首轮新test的repair前置设置错误已修，仅RED-final作产品收据。三通道同批失败/次批关闭、混合/成功逐call、空批/未知工具/散文不计、proof-only同批两失败仍关闭且emit不续命全覆盖，未修改旧测试。

最小生产改动及新针冻结，主线diff复核通过。`GREEN-focused.log`正式exit0（agent1.184s）及同集合`GREEN-race.log`exit0（2.751s），各44项顶层PASS、无SKIP；两者均位于上述临时目录。整包agent与叠加Trace文案后的统一末版全仓待收正式结果；不提前宣称已经降低live重试次数，旧Python人工FAIL不回写。

完整agent包`/tmp/hmc-planner-failure-round-agent-full-20260921.log`正式exit0（session45676，69.346s），不只跑新增针。准备独立提交本片，再与Trace文案末版统一全仓；旧版全仓不挪作本版收据。

本片`640447004`已正式提交推送main（1b17→640，session77790 exit0）。统一全仓将在下面Trace显示/教学末版冻结后执行。

## 72. Trace链路覆盖与状态专属排查方向（2026-09-21，子片验收中）

§70两个确定系统缺口同批收口：窗口覆盖的分子、分母、跨窗/抖动/缺失分支保持，用“链路覆盖/未覆盖”代替“已归因/已解释”，明确这不表示原因全部查明；不借完整状态分区或满值给系统添加诊断权。未计价真实占用继续展示原行数/最大值/证据，但方向按已发布typed状态分别限定：sleep沿唤醒/阻塞依赖排查；执行/语义工作保自身工作量及业务方向；未知态不给默认机理。混合组不把一类方向借给其它状态，不改变根因排名、已量化可消除量或加冕词形。

实际供给审计同时发现`TraceQuery`共享closed-matrix说明把所有context-only占用统一指向own-workload/business；Description与Parameters两面均被模型消费，必须随显示修复同步限定，不能只改用户图而保冲突教学。其它最终值回执/决策handoff/最终边界已明确状态分区不闭合原因，有限只读复核未确认第二个同类硬授权漏洞；必要条件“additive carrier”可进一步精确的低级措辞观察保留，不据此声称全系统无问题。

参考`core/preprocess/sleep_ops.py:558–622`先按依赖窗统计状态，再对实际S/D段裁剪并递归唤醒者，可借鉴“等待状态与上游依赖分层”，不把睡眠量铸成线程实际执行量。它的中断/未知唤醒提前返回与本仓根节点停递归策略并不完全相同，本片不移植该截断规则或更改已有IO链。

计划验收：真实TraceQuery→原生观察→投影→最终渲染公开红绿，中英/改名/缺失与部分完整状态账/混合与未知态矩阵；工具实际Description/Parameters教学针；旧词面pin只迁移相关输出期望、数值和反例保留。再叠加§71统一全仓、干净构建。下一固定双例按风险与增量覆盖选择`trace_query_wakeup_background_demotion`（明确窗、链上IO与链外长D等待隔离）+`github_issue_nlohmann_long_double_symptom`（C++双发布头同步、真实apply及严格编译），2并行×1。新版本可验修复与跨语言保护，不作§70相同配置A/B，不代销B2–B6或其它人工债。

同owner独立复核另确认`runtimeTraceProjResidualOwnCaliberNote`旧IO互指仅用typed自身IO口径/最大值与`min(caliber,residual)`，没有区间交集凭证；旧测试也能用无时间戳记录生成“重叠解释”。这是代码/既有针确认的旧语义债，不冒称本轮live命中了该臂。纳入同批中性账目对照，数值、引用、上限和选择逻辑不变；不把缺凭证改说无重叠，也不新增数值大小推物理关系的规则。相应旧正负针都更新到实际新表达，保全部数值/证据断言，避免留下永不触发的旧词负针。

末版28个代码/测试/快照文件冻结（`/tmp/codrax-projection-coverage-20260921.HP2oC8/frozen-files.sha256`），主线及独立只读复核无阻塞。公开原生链及中英文RED=`RED.log`，实际工具双面RED=`RED-teaching.log`均正式exit1；旧词面迁移过程`GREEN-focused.log`仍exit1的两处遗漏保留，不改写收据。末版29个相关测试文件的284项顶层精确集合`GREEN-final-focused.log`正式exit0（tool30.484s），额外5项agent工具面/提示快照`GREEN-tool-surfaces.log`exit0（agent1.318s）。完整选择来源/regex/命令在`selection.txt`。共享Description golden先按旧快照正式RED，再依既有仪式更新；`golden-delta-check.log`确认只替换一条获准合同说明（285→556字节），其它字节不动。没有字段/schema/排序/数值改动。

公开新针保20/17/14/11/1ms原生值、改名等价、观察/投影字节不变；nil/partial/full状态账不铸原因闭合；sleep/running/unknown及三类混合组分别限定，最长混合英文在最终图中完整保留方向、41.500ms/E#且每行不超过100显示格。相同集合race及叠加§71的统一全仓`/tmp/hmc-coverage-planner-final-full-20260921.log`正在执行，正式收据另补。

284项相同集合race现已正式exit0（session63803，`GREEN-final-race.log`，tool319.788s）；额外5项工具/提示面race也exit0（session71053，`GREEN-tool-surfaces-race.log`，agent4.416s），不以数量较少的集合替代原集合。实现提交`47babb574`；冻结28文件hash逐项OK，干净构建`/tmp/hmc-coverage-planner-clean-build-20260921.log`正式exit0（session27421），实测revision47babb574d4f、buildTime2026-09-21T08:18:32Z。统一全仓尚在执行，未提前签绿。已用该固定二进制启动上述两例，结果根为`eval/results/hmc_coverage_state_crossmode_20260921`，每例一次、并行2；过程/答案审计待正式结束，不增加第三例追绿。

首次统一全仓现已正式exit1（session61825，tool435.175s）：仅`p0a2_coverage_test.go`和`rnb_leadsem_test.go`四项旧词面针漏迁移；其实际输出已为正确“覆盖数值”，覆盖行提取器仍查“已归因”导致空串。最小修正只改两份测试，保计数/MAX/2.770与28.717ms/112.223分子及零分子披露断言；同步迁移未失败的反面针，避免空针。覆盖行选取精确`- 链路覆盖 `而非新解释句，折叠前后非空与整行字节相等仍强制。独立只读复核通过。58项完整P0A2/RNB/LeadSem集合定向`/tmp/hmc-coverage-pin-migration-green-20260921.log`exit0（session76955，tool3.317s）及同集合race `...-race-20260921.log`exit0（session15149，tool19.622s）。末版统一全仓改记`/tmp/hmc-coverage-planner-final2-full-20260921.log`（session17074）执行中；首轮失败不抹，不据定向绿签全仓。

## 73. 47b固定双例：显示修复命中，原生证明与末尾上下文仍有缺口（2026-09-21）

同一干净47babb574d4f、2并行×1，runner正式exit0（session36566）；Trace217秒机器PASS，C++ apply208秒机器FAIL（verification_proof_incomplete），完整人工均FAIL。[机器摘要](../../eval/parallel_selected_summary_hmc_coverage_state_crossmode_20260921.md)、[人工逐面审计](../../eval/parallel_selected_summary_hmc_coverage_state_crossmode_20260921_manual_audit.md)保留各自结论，无第三例追绿。

Trace新覆盖/等待措辞实际命中，明确窗、11ms链上IO、三项1ms调度供给候选、19.5ms链外logger隔离及必选schema2根因旁路都保住。但模型把依赖窗当连续睡眠起止、把唤醒和入核运行混写、部分机理与内部词面仍越界；相关真实区间和边界已经进入最终输入，不能再把这些都说成工具缺证。首轮fact_families冲突拒绝符合已给教学，不是另一个矛盾合同。

另确认两个系统自矛盾，ROI高于重复追加同义教学：`renderTraceFinalLeaderMechanismCeiling`和一般phase handoff把S/IO亦统称on-chain work；同一最终图的Adjacent行按角色显示“无直接唤醒边”却同时列明确上游唤醒点。后者是行的计量职责被写成物理关系否定，不应升链或删除真边。下一片只改这两处共享语义投递，按状态保等待/工作/未知，显示只声明邻近行的资格；保全部数值、链凭证、独立已证等待、机理上限与模型结论所有权。

C++两发布头各一行真实改对，两次严格编译及运行通过，测试/Makefile未改，隔离交付resolved。最终依旧诚实unverified，未把Make aggregate PASS提升成float-not-regressed的精确断言证明。模型把仅测1.25L且只断言非空的main绑定给普通float/double合同，comparator还引用不存在的double重载；这不能靠未来B2授权或把main当通用断言就自动闭合。B2–B6、C++精确证明生产者、模型合同语义质量分账保留，已有测试绑定教学不等于所有绑定都语义正确。普通planner本例failure_rounds=0，未命中新计费分支，不能以此宣称live重试下降。

HMC父账仍13/79已交付、66开放；此前各次人工FAIL不回写。末版两文件词面迁移提交`b6d1bfa84`，不改变已回放47b的生产字节。

§71–72末版统一全仓`/tmp/hmc-coverage-planner-final2-full-20260921.log`现已正式exit0（session17074），87测试包、13无测试包、零FAIL；首轮四项旧词面FAIL仍保留。该收据签47b生产与b6d测试末版，不签接下来新增的末尾提示/邻近关系修复。

## 74. 依赖阶段不等于执行工作，展示角色不否定独立真边（2026-09-21，子片验收中）

§73人工FAIL中的两个系统自矛盾按同一原则收口：阶段、状态、计量职责、物理关系分别从各自已有证据读取，不互相代替。一般handoff/共享phase元数据/最终成文尾部统一称链上依赖观测，明确由实际状态或语义片段区分等待与执行，未知态不猜；保留running自身工作量方向、确定性语义工作、既有可消除量及机理上限。没有新增JSON字段、校验门或模型必填负担，既有完整/不完整查询组、预览上限、独立已证Binder/完成闭合IO及typed blocker例外不变。

邻近明细仅将“无直接唤醒边”改为“不计入链上影响”。这声明的是该行的计量职责，不是线程有没有边；已有准确上游唤醒点继续显示，未知点仍不生成，不将邻近行晋升为链上根因。真实公开TraceQuery→原生观察→最终渲染测试，要求同一明细块同时保留对象、邻近角色和准确边；另要求同一指标表行保留对象、sleep及17/14ms，不能靠全文中任意相同数字假通过。中英/改名、已知/未知点及原始观察字节不变均有保护。

再次对照参考`core/preprocess/sleep_ops.py:558–622`：先查直接唤醒，再统计依赖窗内状态，最后只对实际S/D段裁剪和递归。借鉴的是状态与依赖窗口的分离；其未知/中断唤醒返回None并不构成“没有物理关系”的事实。本仓不复制这一截断，不改变D/IO根节点、精确唤醒链或显式时间窗。

展示侧旧生产公开RED `/tmp/hmc-adjacent-role-red-20260921.log`正式exit1（session39162，tool1.421s）。加强同表行断言后的末版64项定向`/tmp/hmc-adjacent-role-final-green-20260921.log`exit0（session65469，tool4.267s），同64项race `...-final-race-20260921.log`exit0（session36659，tool22.864s）。独立只读复核无阻塞；共享实际Finalizer消息矩阵、统一全仓及提交收据待本节补齐，不借§73全仓签本批。

共享提示公开RED `/tmp/codrax-dependency-observation-20260921.m3Yrz6/RED.log`正式exit1（agent1.607s）；实际TraceQuery结果进入NewFinalizerAgent.Execute的适配器边界，原生四视角与七状态×两种查询组完整度×双语、独立等待/target blocker/异对象blocker/有限事实请求及预览容量共四顶层测试，末版`GREEN-new-matrix.log`正式exit0（session52522，agent2.289s）。模型调用在消息捕获边界停止，不能称真实LLM答案已验。冻结后统一全仓`/tmp/hmc-dependency-role-final-full-20260921.log`（session78627）执行中；相关大集合及race另补。

共享提示末版九个显式测试文件、84项顶层集合`GREEN-focused.log`正式exit0（session47739，agent6.199s），同84项`GREEN-race.log`正式exit0（session61111，58.758s），确切选择见同目录`selection.txt`；五文件冻结hash主线复核均匹配，独立末审无阻塞。补semantic正控过程中因fixture缺语义载体造成的`GREEN-new-semantic.log`失败保留，不记作生产故障或用首轮部分绿替代完整末版。实现已提交`f29750f58`，干净构建`/tmp/hmc-dependency-role-clean-build-20260921.log`正式exit0（session40219），revision f29750f5862c、buildTime 2026-09-21T08:47:33Z；全仓正式退出前不宣称整批已验收。

本批固定版本生产双例选`trace_query_wakeup_background_demotion`与`github_issue_dayjs_duration_nan_symptom`，2并行×1。前者按最高风险复验同图关系/等待语义/链外隔离与旁路，后者扩展JS真实apply及既有回归测试保护，不只反复跑Python/C++。在新干净构建上执行，不添第三例追绿；它不替B2–B6缺失原生证明车道验收。

§73的依赖窗当连续状态段、唤醒混同入核运行、正文机理越界和内部词汇泄漏仍是原始人工FAIL；本片只签两个已复现系统缺陷，不倒签整份答案。B2–B6和精确原生断言语义绑定继续开放，HMC父账13/79已交付、66开放不变。

末版统一全仓现已正式exit0（session78627），87测试包、13无测试包、零FAIL；签冻结f29750f58，不签之后的新显示修复。该片定向/race、独立审查、全仓与干净构建已齐，旧人工FAIL不回写。

## 75. f297固定双例：本片live命中，分母口径新缺陷优先（2026-09-21）

固定f29750f5862c、2并行×1，runner session10387正式exit0；Trace234秒、JS136秒，机器0/2、完整人工0/2。详[机器结果](../../eval/parallel_selected_summary_hmc_dependency_role_crossmode_20260921.md)及[人工逐面审计](../../eval/parallel_selected_summary_hmc_dependency_role_crossmode_20260921_manual_audit.md)。Trace机器FAIL仅词序正则不接受主体先于IO说明，正文其实准确给出11ms主IO；原verdict保留，不靠改oracle回填绿。

Trace中性phase/最终上限、邻近角色与准确wake并列均真实命中；显式2.000–2.020、系统补采、链上11ms、17/14ms睡眠、各1ms供给、19.5ms背景隔离及schema2可用旁路均保住，依赖窗冒充连续睡眠和无机制改写否定未再出现。但模型仍将三个候选合成3ms，实际最终输入已明确禁止，无需为此再造关键词硬门。另有明确系统缺陷：模型曾探索2.000–2.025，扩窗logger20ms背景行进入20ms主树后显示100%和“整窗等待(疑似空闲)”，同页另有正确19.5ms请求窗记录。该标签按金额近似主窗长判定，却未比行查询身份，比例亦借主窗分母；不能通过删除背景或篡改原值修。下一片统一修行级计量范围/比例/整窗标签，保护异位同长窗、缺窗、合并窗及真实状态跨窗披露。

JS仅改一行缺失值默认0，原回归测试与检查脚本字节保持，真实apply及隔离交付resolved。make check的Python静态检查通过，npm/node在当前验证环境缺失，最终诚实unverified；不能把它签为JS断言执行成功。模型为顶层assert脚本起了并不存在的具名assertion_id，npm_script_exit_status也没有逐断言生产者，安装运行器不自动使该PTO合格；本例合同全planning-only，不代销B2–B6。一次畸形双编码workflow JSON被精确拒绝后恢复原生对象，未确认新的系统教学互斥。

ROI：先完成上述明确显示口径缺陷，再恢复B2的controller源快照授权前置；模型语义误用、运行器缺失、无原生断言身份及oracle词面限制分别留账，不用反复同版回放追绿。任务总数仍13/79交付、66开放。

§74实现及§75审计已随`427eb229b`推送main（session36989正式exit0，95112→427eb）。下面的新生产片独立验证，不复用本次全仓签收。

## 76. 背景/邻近行必须使用自身查询尺（2026-09-21，子片验收完成）

§75的原生数值并无错误：logger在补充查询2.000–2.025里20ms，在请求查询2.000–2.020里19.5ms。缺口在显示把前者除主树20ms并据此授“整窗等待”，不是需要改解析、裁掉原段或取消系统补采。旧语义行source-window helper只覆盖semantic且只比长度；rank query与真实状态Start/End也不等于该值的查询分母，不能直接借用。

方案限定背景/邻近展示行：有单一已知QueryWindow时，bar与占比使用该行自己的查询长度，并就地解释本行尺；同长异位也明示范围。缺查询窗或多窗合并时保原ms/E#/定位/查询成员信息，不回退主窗、rank窗或实际状态段伪造占比。整窗标签两面消费同一尺，且只接窗口投影值，不以实际状态fallback近似主窗长推整窗。链上贡献、加冕、排名、所有原始观测/时间窗及自动补齐不改。

参考`core/preprocess/sleep_ops.py:581–622`按当前递归窗口统计并逐S/D段裁剪，启示是分母归属必须跟统计对象一起流转，不是将一组窗口长度应用于所有节点。本片为本仓多查询汇集展示接缝的修复，不声称参考仓直接实现了相同显示结构。验收须包括真实公开窄/宽查询、中英/改名、同长异位/缺窗/多窗、状态extent不等于query、typed非等待与非window值反例；旧手造正针补有效query前提而非删除原断言。

正式公开RED `/tmp/hmc-context-window-ruler-red-public-20260921.log`（session52730，exit1）已取得：真实fixture经四次TraceQuery、明确2.000–2.020请求窗及user_explicit app-100，再走ApplyAndPersistMutation和最终renderer；中英/改名/查询结果顺序反转均复现20ms扩窗行错误100%及整窗等待。初版harness未绑定主窗且误含overview无bar行的失败不作为产品RED。当前生产仅改显示，正在完善跨单位、无主窗、合并MAX和既有语义行保护；GREEN及统一全仓未取得前不销账。

本版干净构建后的固定双例计划选真实采集`real_trace_e1_dual_window_normalized`（两个不同长度的明确窗，CPU获得量归一化，保护查询组合与范围表达）和`patch_go_typo`（Go真实apply及已有TestGreet三组输入）。每例一次、并行2；不选本机缺Node/JDK的重复环境失败，也不把双窗比较自动算成本缺陷的live命中。公开回归已强制命中宽/窄背景并存，真实LLM回放是否生成相同显示分支须另看实际工件。B2–B6仍独立开放。

邻近回归发现一个范围边界，不能为新规则误删旧能力：全树没有查询窗口时，旧树头明确说满格是“本报告最大时长”，是合法相对量级而非查询窗占比。初版将这种context条也留空，导致全context树仍有相对尺头却无条（`/tmp/hmc-context-window-ruler-final2-focused-20260921.log`真实FAIL保留）。裁定最窄保留无窗模式的旧相对bar、头和图例，但两面仍无百分比/自身查询尺宣称/整窗标签；只有有主窗模式才适用本片背景行独立查询尺及缺窗留空规则。先前“无主窗一律空bar”的预审不算最终签收。

末版7个tool文件冻结（1生产、1新回归、5旧测试前提/图例迁移），32项顶层定向`/tmp/hmc-context-window-ruler-final-focused-20260921-v6.log`正式exit0（session5155，tool2.987s）。公开矩阵同时保护20/25=80%八格、等长异位2.000500–2.020500=100%十格及同一行/明细上的微秒范围，改名/双语/结果反序均保明确主窗与原始观察字节；typed缺窗、多窗、actual回退、跨单位和无窗模式另有正反针。既有semantic source-window通道不改。为保全图例双向覆盖，最后补两个旧手造over/idle记录的合法QueryWindow前提，99.8/250数值和原断言不变，不以删除图例覆盖要求换绿。

独立只读复核通过生产边界及图例最终增量。末版统一全仓`/tmp/hmc-context-window-ruler-final-full-20260921.log`（session91076）与相同32项race `...-final-race-20260921.log`（session61898）执行中，尚不签全仓或整份live答案通过。

实现提交`cd2e029bc`；同32项race现已正式exit0（session61898，tool12.479s）。`make`干净构建`/tmp/hmc-context-window-ruler-clean-build-20260921.log`正式exit0（session42464），实测revision cd2e029bce1b / buildTime2026-09-21T09:17:01Z。已以该固定二进制启动上述两例，runner session73512、结果根`eval/results/hmc_context_ruler_crossmode_20260921`，2并行×1。全仓与回放仍在执行，不将启动或局部成功写成最终PASS。

首轮统一全仓现已正式exit1（session91076，tool419.831s），其它86测试包通过；5项旧显示针待精确迁移：berlin_gaps过窗比例、c4全tag布局、calside墙钟正控、ptv6d固定行数及v3 golden背景条。没有失败指向Trace原始数值/根因资格变化。旧合法比例样本须补有效查询前提，缺查询的真样本不能凭空补窗，布局变化必须核对实际完整输出再改精确期望；所有原值、负针与宽度要求保留。不以32项绿或live PASS替代这次全仓FAIL，修正后须完整复测。

五份旧测试已精确迁移并经独立只读复核：berlin/calside仅人工样本补自身query（非墙钟负控也同补，避免缺窗分支掩盖）；c4保完整tag各一次、先后、E#及100格宽；PTV6真实样本精确31/17行、成员和值不改；v3真实缺窗三行不捏造query，同一stanza保值/E#/未知尺说明且禁bar/%。完整45项定向末版`/tmp/hmc-context-ruler-five-pin-green-final-20260921.log`正式exit0（session86916，tool1.229s），同45项race `/tmp/hmc-context-ruler-five-pin-final-race-20260921.log`正式exit0（session75415）。第二轮统一全仓`/tmp/hmc-context-window-ruler-final2-full-20260921.log`（session72869）仍在执行；只迁移测试，不改变已回放cd2的生产字节，不抹第一轮FAIL。

末版统一全仓现已正式exit0（session72869）：87测试包通过、13无测试包、零FAIL；与首轮失败分开留存。五文件测试迁移提交`7f14de1c1`，同45项race为tool2.763s。该全仓签`cd2e029bc`生产与`7f14de1c1`测试末版；固定cd2双例详§77，生产未变无需因测试迁移额外追跑。首轮失败、初版无主窗回归失败及旧人工FAIL不回写。

## 77. cd2e固定双例通过，剩余身份教学与附录计数分账（2026-09-21）

上述固定双例runner session73512正式exit0，机器2/2、人工功能正确性2/2 PASS；Trace134秒、Go121秒。[机器摘要](../../eval/parallel_selected_summary_hmc_context_ruler_crossmode_20260921.md)、[逐面人工审计](../../eval/parallel_selected_summary_hmc_context_ruler_crossmode_20260921_manual_audit.md)保留实际过程和未覆盖能力，没有第三例追绿。

真实Trace两窗仍分别为2.992/30ms，CPU运行0/3.414ms及0%/11.38%，四核分布与范围绑定正确；零D/IO仅是记录口径，未铸等待机制排除。bounded_fact_set无因果树合理，schema2空旁路及答案哈希绑定正确；因此本次仅签真实双窗回归，不签§76背景尺分支live命中。两项过程错误（首emit数组/scalar混给、探索摘要跨窗借量及称并行）均有准确现有教学/typed事实供给并最终恢复，不为其加原文硬门。9次模型查询+2次补采仍有重复上下文的优化空间。

Go只改一行，原TestGreet字节不变，真实go test JSON返回一条断言PASS（内部3个表项），隔离交付无合并。旧probe包装go build正确被拒后恢复既有测试；没有独立go build收据，不能额外宣称。PTO suite却填main、真实runner发布module import path，精确匹配不成立；三个合同均planning-only、required=0，报告没有冒发证明，所以功能PASS不销B2–B6。进一步审schema/实际消息确认共享身份说明只笼统提suite/class/module/file和Python例，未明示Go Package(import path)与源码package声明的差别：这是可改进的原生身份教学/可见性缺口，不能全归模型波动，更不能放宽matcher。

Trace附录另有P2确定显示债：同A/B的11条来源记录占四行预览后，len(states)-4被写成“另有7条独立范围”，其实只有两个窗口。权威层按EvidenceID/sourceKey保留来源有其必要性；后续先纠正遗漏记录数量的词义，若压缩只在展示层保来源地折叠完全相同内容，异值/范围/限制不能并。当前数值答案正确，但该债未关闭。

下一顺序：先收齐§76全仓旧针迁移，再处理原生测试身份投递（跨runner一致、非Go个例替换）及附录记录措辞；B2–B6精确来源授权与补绑定仍独立开放。HMC父账仍13/79已交付、66开放，以前人工FAIL不回写。

身份审计进一步发现待公开复现的高ROI接缝：非根项目的`qualifyChangeReport`会给suite与assertion_id都加执行目录前缀，Python的路径归属检查却仍要求原始模块/文件开头；Java完整类选择器也有同类静态风险。若真实通过断言因此无法登记，优先修生产者与消费者的精确执行范围一致性，而非仅补教学。不能任意剥`::`（pytest本身含此分隔符）、借兄弟目录结果，或把candidate范围等同具体执行代次。当前独立树诊断中，未签产品RED，也不改主树正在全仓验收的源码。

成功结果身份可见性亦分账：普通run_tests摘要目前主要给总数，passed suite/id未逐条投递；首轮planner还没有执行报告，不能新教成“必须先复制报告”或授权它突破原生测试执行限制。参考`core/llm_contract.py:114–155`可借鉴已发布成员/字段与实际工件反查，但参考仓没有本项目PTO能力；其散文查重硬拒不移植。

§76实现、旧测试迁移及§77审计已随`a8c5dd474`正式推送main（session84482 exit0，427eb→a8c5）。末版全仓tool395.989s、agent85.893s、tracequery116.512s。以下批次独立验收，不借该全仓收据签新代码。

## 78. 非根测试结果的执行目录与路径归属一致（2026-09-21，验收中）

§77静态疑点已公开复现为确定性系统缺陷：独立树`/tmp/codrax-python-pto-path.VuUJ8T/tree`从EmitChangePlan→真实ApplyPatch→RunTests，子项目`packages/widget`真实unittest exit0、同一完整suite/id确已通过，但`declarationMatches=true/pathMembership=false`，required证明仍missing。正式RED=`public-diagnostic-setup.log`（session24503 exit1、tool2.412s）；根目录正控和兄弟身份负控通过。首版pyproject误选pytest且runner_missing是fixture失败（`public-diagnostic.log`），不计产品RED。参考仓的成员/工件精确反查原则仍适用，但它没有本项目的测试执行证明连接，不能说是移植现成功能。

生产者原先给非根suite/id加`runner[/framework]@working_dir::`，路径核验却读完整串当裸模块/类。修复抽取既有发射词形单源，消费者从当前报告typed候选和已执行项目命令建范围表；两个身份必须属于同一、唯一、当前候选范围，仅路径归属检查解开一次前缀。声明仍匹配原始完整suite/id，报告字节不改；不解析任意`::`，不让root的Go `.`选择器借用已知子项目结果，重复/混合/歧义前缀和兄弟目录均不授证。成功/失败共用resolver，失败多执行来源保守门不动。

此边界只恢复candidate scope，不解决同scope多次invocation的来源身份；B2源快照授权及B3–B6只读补登记不因此开放。Go首轮身份教学、成功身份可见性、full/skeleton checklist教学漂移和C/C++精确断言生产者均保留。没有扫描用户或模型原文，没有新JSON字段或新增模型填写负担。

主树公开四枝（根通过、非根通过、非根真实失败、兄弟不串）加既有未改测试五枝，GREEN=`main-public-green.log`（session91855 exit0/tool3.864s），同组race=`main-public-race.log`（session6116 exit0/tool6.039s），均在同诊断目录。只改真实源码，已有测试字节不变；非根通过现在获required proof、真实失败获得failure relevance。额外九种runner格式的producer→resolver成功/失败矩阵只算单元证据，不冒充本机运行Java/Node等。

末版十个测试文件的54项精确集合定向`/tmp/hmc-test-result-scope-focused-20260921.log`（session34891 exit0/tool23.956s），同54项race `...-race-20260921.log`（session73880 exit0/tool17.579s）。初版矩阵给Django手工suite与其既有selector不一致的失败留在`...-matrix-20260921.log`，未扩修selector或当产品RED。独立只读复核通过；叠加下一节两句文案的统一全仓`/tmp/hmc-scope-record-final-full-20260921.log`（session28202）执行中，尚不签全仓。

## 79. 状态记录数不冒充不同时间窗数（2026-09-21，验收中）

§77附录确认的P2仅修遗漏说明：中英两面都明确未展开的是状态统计记录，条数不是不同窗口数量。四条展示上限、A/B优先顺序、EvidenceID/sourceKey权威去重、原生值和范围均不改。参考`core/batch/closure/select.py`的member_label与`server.py:1309`行截断说明可借鉴明确计数对象，但没有本问题的直接去重实现，不能借它合并观测。

新公开路径EmitAnalysis→原生六类TraceQuery→emit_answer_document→实际渲染，双语×4/5/11记录；先证一份工件、两窗和全部独立原生记录，再核四条展示、A/B优先、逐行原值与前后工具结果/ledger/authority/模型正文不变。有效RED=`/tmp/codrax-state-record-count-20260921.4J8dHX/RED-public.log`（session53408 exit1）：4条正控通过，5/11只因旧遗漏词面失败。首版用event_search未产生状态记录的harness失败另留，不计产品RED。

同目录`GREEN-focused.log`（session58529 exit0/tool3.599s）及`GREEN-race.log`（session68350 exit0/tool10.875s），同24项集合`^(TestTraceStateRecordCountPublicTwoWindows|Test.*B1626.*|TestB1618TargetWaitQueryJoin.*)$`。独立复核未见生产越界；统一全仓与干净固定双例收据待补。父账仍13/79已交付、66开放，不把两项子缺陷冒充父能力完成。

§78/79实现分别提交`e2916433e`与`635814c6d`。下一轮固定双例为真实`real_trace_e1_dual_window_normalized`及新增`nested_python_increment`，2并行×1；前者复验两窗数值与附录词义，后者在子目录项目只改实现并运行已有测试，不以显式PTO提示诱导模型。新fixture语法健康，真实unittest基线exit1（3 methods/7 failures），临时副本仅修源码后exit0（3 tests），测试cmp一致；日志`/tmp/codrax-nested-python-eval.iUnWa5/`。原测试SHA256=`504b2535413cafa883be2802d92a698036735de8883384530caec4fa8375f322`，交付必须另核diff/hash，源码存在性oracle不替代执行证明。当前仅公开回归证明scope修复，live是否实际生成并消费合格声明须逐面审计，不能提前签能力命中。

本片统一全仓现已正式exit0（session28202）：87测试包、13无测试包、零FAIL，tool402.458s、tracequery129.105s。案例/文档提交`3b2a41eaa`后干净构建`/tmp/hmc-scope-record-clean-build-20260921.log`正式exit0（session91934），实测revision3b2a41eaa0f3、buildTime2026-09-21T09:54:48Z。固定双例runner6160正式exit0，机器2/2（Trace101秒、Python198秒）；人工审计与新缺口另节记录，不用自动PASS代签整份答案。

## 80. 3b2a固定双例：功能一过一留债，子目录probe合同冲突优先（2026-09-21）

[自动摘要](../../eval/parallel_selected_summary_hmc_scope_record_crossmode_20260921.md)、[完整人工审计](../../eval/parallel_selected_summary_hmc_scope_record_crossmode_20260921_manual_audit.md)：机器2/2、人审1/2。Trace两窗/CPU量与比率正确，但无健康基线把0%→11.4%写成“有意义的恢复”“正常获得CPU”；实际最终输入已给状态≠机制及bounded边界，暂按模型越界留账，不新增散文硬门。本轮仅4条状态记录且无状态附录/背景尺图，§79/76均不冒称live命中；空schema2旁路及工件绑定正确。

Python仅改实现一行，原tests/config字节不变，真实3条unittest断言及1条changed-target probe通过，最终诚实交付隔离工作树，功能PASS。PTO仍填错suite且漏非根前缀；required=0、四合同planning-only，未授虚假证明，也未命中§78成功绑定。不能用本轮替B2–B6销账。

新高ROI确定接缝：planner probe在packages/widget实际执行from widget import increment并到达原实现，emit耦合门却按仓库根模块packages.widget.widget要求导入，导致七次拒绝；目标生成在逐probe循环之外，未消费WorkingDir，而执行器会归一到最近Python项目根。下一片按同一有效执行目录生成逐probe可解析改动模块身份，保护兄弟同basename、动态别名/注释伪造及越界目录，真实运行来源/target_execution上限不动。JS/Ruby等相邻格式先审计具体执行语义，不能把Python别名规则硬套全语言。

另确认两计划入口acceptance_tests教学矛盾：完整入口是planning-only，分段入口仍must cover。以共享短描述统一，JSON字段与权限/validator不改。原生PTO跨runner身份教学、成功身份可见性、同scope多执行provenance及B2–B6仍开放；本批不为错误身份放宽matcher。父账13/79交付、66开放不变。

§78–80实现/全仓/固定双例审计已随`94ce058ea`推送main（session6677正式exit0，a8c5→94ce）。以下新片须独立验证，不能复用上一片全仓签收。

## 81. 完整与分段计划共享验收清单含义（2026-09-21，子片验收完成）

完整计划的已有正确说明抽为types单源常量，两工具schema以JSON marshal替换同一占位符；完整入口描述字节不变，只消除分段入口的must cover矛盾。数组/string项、optional、PTO与probe权限、运行/证明校验完全不改，不引入新模型字段或关键词规则。

实际planner工具schema投递（无执行报告）新回归先RED：`/tmp/hmc-checklist-teaching-red-20260921.log`，session83607正式exit1，agent1.137s，精确命中分段描述冲突。改后6项小集合GREEN session31635 exit0。包含发射/分段/证明权限/JSON表面的四包146项顶层定向`/tmp/hmc-checklist-teaching-focused-20260921.log` session90469正式exit0；同选择race `...-race-20260921.log` session87579正式exit0。统一全仓待子目录probe实现冻结后合跑，不借146项绿签全部功能。

参考`core/llm_contract.py:114–155`仅借鉴“成员身份已发布、证据可回查”的职责分离；其文本容差/散文重复硬门不移植。首轮planner没有执行报告是合法状态，本片未教成必须复制不存在的结果。成功身份可见性、runner方言与父任务继续开放。

本片已提交`d8a052ed7`。agent/types/skill整包`/tmp/hmc-checklist-teaching-packages-20260921.log`（session89065）正式exit0，agent70.955s、types38.809s；不替代下一片冻结后的统一全仓。

## 82. Python探针的导入身份与实际执行目录一致（2026-09-21，子片验收完成）

§80 live七次误拒已有独立有效公开RED：`/tmp/codrax-python-probe-cwd.KIy00N/RED-public-authoritative.log`（session27724正式exit1/tool2.614s）。先真实planner probe以局部导入调用原源码并获AssertionError而非ImportError，再Emit；根项目正控完整通过，子项目/内部cwd归项目根/src包/改名/仅import五枝只在旧耦合门拒绝。根正控另经真实Apply、提交及CaptureCommitPatch→PatchEffectRecordFromUnifiedDiff生产收据，RunTests实际执行后持久化再核target execution，不手造行表或执行结果；此前三个测试装配错误日志保留，不计产品RED。

设计以执行器已有`resolveVerificationProbeWorkingDir`和`pythonWorktreeImportRoots`为单源，逐probe计算真实改动路径在当前导入根下的模块身份；计划接纳及changed-target绑定消费同一结果，不全局合并兄弟项目basename。原root/src/lib和公共入口兼容、source-free权限、词法字符串/注释防伪及真实运行来源/changed-line证明必须保留。合法import本身不算已执行修改函数，plain Python probe仍仅可给target_execution、不铸target_behavior或逐合同证明。此片不触Trace投影/根因/补采/旁路或超时。

横向审计：JS/Ruby的引用提取同样未读WorkingDir而运行器确设置cwd，是待独立复现的相邻风险；Java按package/class、Go有同包cwd分支，不能套用Python路径规则。当前内联probe能力只有Python/JS/Ruby/Java/Go，ArkTS/Cangjie等原生项目测试通道不能被当成同一种执行器；不因本片给它们虚构内联支持。参考仓的本地工具组合/成员字段反查可借鉴职责，但没有本仓改码后验证证明的直接等价实现。后续须按真实执行语义处理，不按单个case改别名。

冻结后下一组固定live为`nested_python_increment`与`trace_query_jank_field_inventory`，2并行×1：前者验证本次确切生产误拒及真实交付，后者返回前期人工未通过的jank清单，覆盖阈值边界、超过2^53的原始整数、独立时钟/appid和不晋升调度根因；不改oracle，不强制模型选probe或PTO，不追加第三例追绿。

末版实现`4de76b002`冻结三文件，只替换准入/绑定两消费者并共享逐probe候选。五顶层/24子枝新公开集合`/tmp/codrax-python-probe-cwd.KIy00N/GREEN-boundaries-v3.log`正式exit0（session5618，tool5.015s）；含原有边界的48顶层定向`GREEN-focused.log`正式exit0（session24816，tool20.504s），同48项race `GREEN-race.log`正式exit0（session63484，tool22.928s），无skip。内部目录回到真实项目根、既存src/lib、改名、新增包源码均通过；同名兄弟目录、跨probe别名拼接、注释/字符串/复制实现及10项路径边界未放行。仅导入不获执行证明，真实调用仍只获target_execution，不获逐合同behavior证明。新增原本不存在的src/lib导入布局不在本片覆盖范围，不提前声称已支持。

最初边界集合中的参数类型/repair-pack装配和文案断言错误分别保留在`GREEN-boundaries-first.log`、`GREEN-boundaries-v2.log`等中间日志，不记产品RED；最终状态以上述v3正式退出为准。主线独立审读三文件无阻塞；另一审查席独立复核§81单源教学无阻塞。统一冻结全仓`/tmp/hmc-probe-cwd-full-20260921.log`（session78915）执行中，不以局部绿代签。

本轮再次逐段读取参考`core/llm_contract.py:114–155`及`core/preprocess/sendable_ops.py:153–190,494–550`：前者按已发布成员/字段回查工件，后者以明确projectPath/配置/输出路径调用外部工具，缺Node及失败单独披露。参考并无本仓inline Python probe的cwd耦合实现，故本片是本仓生产者/消费者一致性根修，不照搬其自然语言扫描或将外部工具聚合成功冒称逐断言证明。

§81–82统一全仓现已正式exit0（session78915）：87测试包、13无测试包、零FAIL，tool426.755s、agent105.649s、tracequery130.513s。文档提交`88be719dc`后干净构建`/tmp/hmc-probe-cwd-clean-build-20260921.log`正式exit0（session93419），revision88be719dc396/buildTime2026-09-21T10:18:26Z。默认600/300/600秒及活跃推理/工具调用/保活流专项8顶层回归`/tmp/hmc-probe-cwd-stream-protection-20260921.log`正式exit0（session26391，llm8.083s），未修改超时生产实现。

## 83. 88be固定双例：身份绑定仍缺，局部零测试被总体化（2026-09-21）

[机器结果](../../eval/parallel_selected_summary_hmc_probe_cwd_crossmode_20260921.md)、[完整人工审计](../../eval/parallel_selected_summary_hmc_probe_cwd_crossmode_20260921_manual_audit.md)：runner35140正式exit0，2并行×1；机器1/2，完整人审0/2。jank134秒的3条记录/整数/时长/发射TID/TGID正确，但从未建索引推漏扫描并泄漏内部词汇；正确完整覆盖与普通语言教学实际已在最终上下文，不追加散文硬门。Python184秒实际源码单行修复、原测试字节不变、3条原生断言PASS，缺系统限定前缀的短assertion_id未匹配required合同，诚实unverified/proof_weak。短suite本身符合已有suffix规则，不误记错；未声明probe，所以§82没有live命中。父账13/79交付、66开放及旧FAIL均不回写。

新增确定高ROI系统矛盾：root discovery zero_tests与nested真实3passed同报，NoTestsRunners=['python']合理保留局部诊断，却被observation_authority、stage_hooks、scheduler及显示当成整批无测试；报告规范化则仍是passed。最终两次写“python没有发现任何测试”并提示补环境，错误。相邻retry helper还可能让次要zero-tests压掉真实失败修复。下一片统一精确信号区分整体无测试与局部空候选；保局部诊断、缺路径/runner/失败/required合同，不用任意passed行清债。必须有真实混合执行公开RED与正反矩阵；仅syntax/probe/aggregate/non_asserting结果不能当原生断言。

ROI顺序：先修上述全局状态自冲突，再原生身份教学/已执行身份可见性，再B2→B3/B4精确来源授权与只读补登记；B5/B6持久化/回放随后。前两项是已复现生产接缝，不能因重复验证仍失败而改oracle或放宽匹配。JS/Ruby cwd相邻风险、同scope执行代次与其它HMC开放项继续保留。

§81–83实现、收据及固定双例审计已随`189bcf48e`推送main（session48219正式exit0，94ce→189bc）；下节独立验收，不复用该全仓签新代码。

## 84. 局部空测试候选不覆盖整批真实执行结论（2026-09-21，子缺陷验收完成，见§87）

§83确认的问题已从真实公开工具链复现：ReadFile→EmitChangePlan→ApplyPatch→RunTests→JSON往返，同一报告同时保留根目录zero_tests和子项目原生assertion PASS。正式RED=`/tmp/hmc-mixed-no-tests-public-red-20260921.log`（session78068 exit1/tool2.064s），两个PASS分支只在整体authority被误判no_tests处失败，真实断言FAIL控制正常；错PTO身份另保required missing与proof_weak，不把原生执行PASS等同合同证明PASS。

修复边界：报告层单源区分局部缺测与整体无原生断言；各判定、重试、计划状态和展示消费同一精确信号。保留NoTestsRunners及逐执行目录的零测试收据，未知目录不猜；syntax/build/aggregate/non_asserting/plain probe成功不能冒充原生逐断言结果。真实失败优先，required合同、改动路径覆盖、缺失runner及不可用状态独立保留；旧空报告的诊断兼容不因Passed=false就升级成代码失败。当前不改JSON结构、声明匹配/补授权、Trace证据或投影，禁止以扫描答案/请求原文兜底。

参考`core/llm_contract.py:114–155`按真实成员和字段回查工件，`core/preprocess/sendable_ops.py:494–550`将外部命令返回与输出工件分别处理；仅借鉴职责及身份分离。参考仓没有本仓多候选原生测试聚合或PTO证明通道，不能声称直接移植、以外部命令成功替逐断言证明，亦不引入其散文扫描规则。

计划验收：真实公开混合执行正反针、类型矩阵、controller状态/重试/展示一致性、JSON往返、定向/race和末版全仓。完成后再以新冻结版本跑2并行×1跨模式回放，人工审实际上下文/答案/工件；GREEN未齐前不销本片。原生身份教学/成功身份可见性为下一ROI项，B2–B6、旧人工FAIL与父账13/79交付、66开放不变。

新固定双例预选`nested_python_increment`和`trace_query_business_marker_io_chain`：前者精确复验混合执行，并将原生PASS与required证明缺口分账；后者返回旧人工FAIL，检查完整50ms业务响应、35ms请求/31ms线程等待/1ms调度分尺、LoadDocumentIndex线索、背景备份隔离、因果图及根因旁路。未修改oracle、不强迫模型使用本片分支，未实际命中就不签live；不为追绿追加同版第三例。

下一片只读横向设计已复核（未实施）：12个runner不等于12条PTO精确selector通道。当前Go/Node/Python/Rust/Java/Ruby/Swift有selector；CMake/Meson/Hvigor虽可产JUnit assertion、Cjpm可产Cargo assertion，`test_surface.go::impactCandidateSupportsSuite`未接对应PTO selector，Make/npm退出码仍仅aggregate。不能只补示例就冒称这些通道完成。两计划schema共享身份教学应说明真实TestResult及非根限定前缀、Go Package与源码package的区别；首轮无报告仍合法。执行后需从当前报告以有限预算向实际controller/planner消息提供完整成功身份，带报告身份、保原字节/JSON转义和超长整行省略说明，不混旧代次、不自动填合同或放宽matcher。此设计不代签实现、公开回归或B2授权。

末版16个源码/新增测试文件已冻结；112顶层定向`/tmp/hmc-mixed-no-tests-final-focused-v3-20260921.log`正式exit0（session32038，tool4.710/types2.839/writeflow0.439/orchestrator1.387/agent2.381秒；skill编译通过但该选择无匹配测试）。测试包含真实公开PASS/FAIL/错PTO三枝、22类typed报告、同runner不同目录、局部Ensure后双序合并、controller/旧stage持久化及重试一致性、中英正文/建议负针和实际verifier教学。早期教学test缺plan前提的harness失败保留在`...-final-focused-20260921.log`，不记产品RED；v2未含最后措辞反例，不代替v3。

末审特别收住两个相邻入口：合并仅不继承Passed=true且纯局部零测试报告的派生no_tests类别，原census/leaf字节不改，false或其它不可用类别不清；原生已通过但proof_weak的正文与建议不再称整批没测试或需要装环境。明确缺改动路径覆盖的typed原因独立解释，未知调用范围不猜。当前统一全仓`/tmp/hmc-mixed-no-tests-full-20260921.log`（session39631）和扩大旧NoTests集合的race仍运行，不提前签整批PASS。

最终race `...-final-race-v3-20260921.log`现已正式exit0（session41830）：133顶层、6包、无skip/竞争/失败，tool10.979/types8.346/writeflow2.136/orchestrator3.648/agent4.452/skill2.207秒。独立只读末审PASS，三核心冻结hash完全一致；16文件提交`fe52b4898`，统一全仓仍运行，干净构建及固定双例收据随后另记。没有改动旧测试文件以降门换绿。

审计进度提交`1b9185049`后，末版干净构建`/tmp/hmc-mixed-no-tests-clean-build-final-20260921.log`正式exit0（session18693），实测revision1b9185049056/buildTime2026-09-21T10:44:09Z。此前一次构建带文档dirty标记，未用于live。新固定双例runner77298已启动，2并行×1、每例1800秒，结果根`eval/results/hmc_mixed_test_status_crossmode_20260921`；这只是启动记录，不能签回放结果或全仓。

统一全仓session39631现已正式exit1，唯一失败为`write_verify_render.go`436行超420维护阈值；tool430.527s等行为套件未失败。经用户追问与职责复核，该文件原418行，本片净增18行仍是验证结果解释/建议的同一职责，不为数字机械拆文件。按交付账本允许的有据例外，仅调整420→460，保24行余量、不改其它上限、不删注释压行、不松行为测试。决策已记IR交付账本；结构/显示定向`/tmp/hmc-mixed-no-tests-budget-review-20260921.log`正式exit0（session23493，orchestrator1.202s）。首轮FAIL保留，替代全仓另记。

## 85. 1b918固定双例：混合状态闭环证据成立，业务IO答案仍未通过（2026-09-21）

[机器结果](../../eval/parallel_selected_summary_hmc_mixed_test_status_crossmode_20260921.md)、[完整人工审计](../../eval/parallel_selected_summary_hmc_mixed_test_status_crossmode_20260921_manual_audit.md)：runner77298正式exit0，2并行×1，机器/人工均1/2。Python116秒只交付实现一行、原测试不变，实际3项原生断言PASS与根目录zero_tests同报；controller/报告/终稿一致，不再全局称无测试或缺环境，§84真实命中。九合同均planning-only、required=0，不冒称精确身份绑定或B2完成。Trace279秒保50ms业务外窗、31ms链上IO、两段1ms调度、因果投影及备份背景隔离，但缺35ms请求与LoadDocumentIndex名字，51ms查询的6ms运行误称唤醒后运行，内部枚举仍泄漏，保留FAIL。

新精确教学接缝（实施前登记）：analyzer对象有required effect、无required causal，同时intent/scenario仍root_cause。现有字段局部修补只建议唯一bounded_effect，但照改后又会被分类一致性门拒绝，增加无谓重试。修复只在这些typed分类冲突时取消唯一缩窄目标，给模型完整的有限/独立因果两路重判说明；full分支由模型补required causal并保留独立effect，work/frame与其它维度原样。真正finite仍给原唯一目标，optional causal及work/relation_path均不授权因果。公共工具红绿、分类组合负控、共享教学测试及末版全仓必须通过；不扫描用户/答案原文，不自动铸角色、不改准入条件。

本轮五次emit/四拒后最终仍为causal_diagnosis，不记投影丢失；模型将原effect改名成causal而未并存，正文仍答了备份不可直接归因。此项记确定提示冲突/重试风险，不虚报新增P1根因授权漏洞。Trace末轮输入与旁路正由独立审查继续核对；若信息确已足量供给，漏答按模型失败留账，不叠加关键词硬门。后续ROI仍为原生身份教学/成功身份可见性→B2–B6，其它HMC开放项及父账13/79、66开放不变。

提示修复末版已冻结5文件：实际Emit入口将已解析intent/scenario传给提示选择，三个既有拒绝分支复用同一双路说明，准入条件和成功路径不变；schema/workflow共享补齐前提，不自动改角色/范围。公开RED `/tmp/hmc-runtime-repair-conflict-red-v2-20260921.log`（session17693 exit1）仅24个真实冲突枝失败；相邻已落bounded形另6枝RED `...-bounded-conflict-red-20260921.log`。最早4个fixture缺diagnostic前提的harness错误保留、不记产品RED；首次扩大回归发现旧明确负向句缺失，补回cannot-authorize边界而不改旧pin。末版新测试5顶层/44子例，完整选择298顶层/122子例定向`...-final-focused-20260921.log`（session44975 exit0，tool3.395/skill0.453s）与race`...-final-race-20260921.log`（session80206 exit0，tool25.609/skill1.805s）均无skip/失败。覆盖30非法tuple仍拒、4真正finite唯一目标、8模型自主双路收敛及2共享教学表面；旧全部ParseRuntimeQuestion/RuntimeQuestion/EmitAnalysis等同步绿。统一全仓与固定live待末片冻结后另记。

末轮输入独立复核完成：日志3084/3085有OpenDocument50=5+1+44及LoadDocumentIndex40=8+1+31，3098有请求35ms/线程闭合等待31ms与两组端点，3263独列query51=6+1+44；初始4消息在修补末轮7消息中保留，无prune。正文漏答/混尺按模型错误留账。schema2旁路available、三链上selection及0.031/0.001/0.001归因值/查询范围正确，但模型description仍称“磁盘块请求约31ms”，不签全旁路语义通过、不代写散文。系统图保两业务线索/备份背景；另确定◎榜面将所有io_latency显示“IO阻塞·设备延迟”，当前31ms实际为发送线程阻塞，需仅审无凭据的设备后缀。既裁§29.175.17的一族一词根（IO阻塞）、链上资格、值与排序均应保留。参考sleep_ops.py:558–625逐S/D片段裁窗与直接wake锚支持分尺，不移植其中丢弃未知/中断waker的策略。

## 86. IO榜面不由延迟类型自动推定设备成因（2026-09-21，子缺陷验收完成，见§87）

问题来自§85真实报告的系统总览，不是模型词汇扫描：类型io_latency统一发射“IO阻塞·设备延迟”，而同类型当前可携带请求驻留、完成闭合的发送线程等待等不同量。31ms线程阻塞不等于35ms请求生命周期，更不直接证明设备服务时间。此后缀会误导正文阅读，即便其它事实卡准确也应修复。

裁定兼容：`real_trace_campaign_20260705.md §29.175.17`的一族一词根、后缀只细化、裸主词表示成因未细分保持；保IO阻塞词根，不换成另一族。仅撤销io_latency默认无依据的“·设备延迟”中英后缀，图例同步；registry、树状态原词、枚举/JSON、根因资格/排序/影响值及模型description所有权均不动。不能从设备名、IRQ名、散文、数值比较或邻近活动补铸设备成因；未来精确设备服务分类应另有typed证据再立项。

验收：真实公开TraceQuery→CompileTraceCausalProjection→总览/图例红绿，中英及自线程/链上/非链图形、原始观察/计价与排名不变；旧OMGCLEAN/SELF/IO折叠回归及race，末版全仓统一验收。旧词面pin只随这项明确语义演进精确更新，不降低词根/证据/值约束。本片不宣称解决模型漏35ms或所有HMC IO能力。

§85–86冻结后固定双例预选`trace_query_business_marker_io_chain`与`real_trace_h4_supply_thermal_witness`，2并行×1。前者回到业务IO旧FAIL与本片实际图词/混合角色，后者是精确时间窗下的有限供给判断反例，必须保数字、限制记录与性能影响未证的区别，不得因修补教学扩成完整根因投影。本批两例都为读模式以覆盖正反权限；前批Python真实写模式验收独立保留，不改oracle，不追加同版第三例。

§85已提交`144a4c1bd`；§86已提交`c8dc9e752`，两片源码冻结。IO公开有效RED=`/tmp/hmc-io-verdict-scope.0jqn69/RED-public-valid.log`（session31609正式exit1/tool2.397s），只在已证S闭合等待/目标自身×中英/改名的无据后缀失败；D自身原词、缺completion保35ms非链上、unknown/context负控均绿。较早RED.log/RED-final.log/RED-harness-d-state.log/RED-public-final.log含D合并类型或EN换行装配假设，不作为产品RED。新3顶层/6子例通过真实查询→编译→ApplyAndPersistMutation→Render，GREEN-public.log session20068正式exit0/tool2.672s。只改两处生产词面和两项旧词面pin，完整字段匹配不降成宽substring；原观测/投影JSON前后相等。

统一末版全仓`/tmp/hmc-repair-label-final-full-20260921.log`已启动（session58849），不复用首次行数FAIL签通过。独立旧默认/活跃流8顶层`/tmp/hmc-repair-label-stream-protection-20260921.log`正式exit0（session34233，llm4.090s），600/300/600秒和活跃隐藏推理等保护未改。IO相邻36项定向/race及独立末审收据随后补，干净构建后再启动固定双例。

IO相邻末版36顶层定向`/tmp/hmc-io-verdict-scope.0jqn69/GREEN-focused.log`正式exit0（session16809/tool7.591s）；相同选择race `GREEN-race.log`正式exit0（session97874/tool67.000s），均36PASS/0FAIL/0SKIP。独立只读审查PASS，5文件hash与冻结相等；IO词根/D各臂/背景原词、全部源值/排名/根因description不变。文档提交`86679d36b`后干净构建`/tmp/hmc-repair-label-clean-build-20260921.log`正式exit0（session46871），实测revision86679d36ba5e/buildTime2026-09-21T11:10:28Z。固定双例runner62896已于04:10:58本机时间启动，结果根`eval/results/hmc_repair_label_scope_20260921`，两例各一次；全仓/人工回放仍待正式退出，未提前销账。

## 87. 86679固定双例：系统修复命中，业务分尺错误与有限IO披露遗漏分账（2026-09-21）

[机器摘要](../../eval/parallel_selected_summary_hmc_repair_label_scope_20260921.md)、[逐面人工审计](../../eval/parallel_selected_summary_hmc_repair_label_scope_20260921_manual_audit.md)：runner62896正式exit0，2并行×1，机器1/2；H4核心问答人工PASS但保P2展示遗漏，业务IO完整人工FAIL。不为同版追跑第三例，也不改oracle。

H4 170秒保明确13762.791708–13763.024898窗、四态157.248/5.604/70.338/0ms及233.190合计，CPU4直接策略上限2100000kHz未被提升成已证性能限制。finite合同下零投影与schema2空旁路正确，不能记因果能力丢失。实际最后输入还给独立完成闭合IO至少4次/并集至少4.384ms及容量下界（8可见、190溢出），正文只保调度器标记D/IO=0，漏两尺并置。零值已有“调度器标记”限定且本题只问CPU/四态/频率，故不升级为核心答案失败或新P1；但相关IO展示开放项不能全销。首稿自造unproven枚举被拒，删后成功；没有schema教该值的证据，不能将模型误读倒记系统合同冲突。

业务IO 240秒仍缺35ms请求，将51ms查询的6+1+44套进50ms业务，入睡时间当issue时间，31ms线程等待当请求耗时，并把13ms余量脑补交接开销；“立即唤醒”与采集窗50ms也不准确。LoadDocumentIndex40ms本轮恢复、旁路description正确，不代表整份正文通过。最后输入2333/2334、2347、2617逐项已有业务50/40、请求35/等待31、query51，4→7→10消息且prune0；定为持续模型消费错误，不以“随机波动”概括、不增加散文硬门。

窄修复确实命中：分析修补提示保7个required维度及work/frame不变；但本轮未声明target_effect_verdict，不能冒称effect+causal双角色live覆盖。图总览31ms已为IO阻塞、无设备成因后缀，原排名/两业务线索/47ms背景/因果图和三个0.031/0.001/0.001秒旁路selection均保。最后patch再次把summary-only元数据放section，被原子拒绝，交付上一接受正文，无部分修改。正确JSON教学此前已在实际输入，该重复误填独立观察。

统一全仓58849正式exit1：86测试包通过、13无测试包；tool437.335s唯一失败是`TestDiagramIdentityAuthorityCensus`的旧函数名登记。§85将parser改名为`parseRuntimeQuestionProfileWithClassifiers`，源码quote准入尾部逐字未变、仍同一raw/quote参数；旧名只留测试wrapper。根席复核后`7cfd422bf`仅迁移精确(file,fn)键并补注释，不宽化前缀/任意caller，self-red、过期条目和全部身份门不变。独立定向RED/GREEN `/tmp/hmc-runtime-quote-owner-census-{red,green}-20260921.log`正式exit1/0（56935/22122），17顶层/76子例全绿。替代统一全仓`/tmp/hmc-repair-label-final2-full-20260921.log`（session15057）已启动；前两次全仓FAIL保留，未提前签新全仓通过。

下一ROI仍为原生测试身份的单源教学与当前成功身份可见性，再B2来源授权及B3–B6绑定/持久化；当前只读设计不算实现。旧人工FAIL及HMC父账13/79交付、66开放不回写。参考sleep_ops逐状态裁窗/直接wake只支持精确分尺，不支持余量机制推断或抛弃中断waker。

替代全仓session15057现已正式exit0：87测试包、13无测试包、零FAIL。该收据验收`fe52b4898`混合状态、`144a4c1bd`提示一致性、`c8dc9e752`IO词面，及`0f153a4fa`有据行数例外/`7cfd422bf`精确census同步。三项窄子缺陷验收完成；前两次全仓失败、旧人工失败及新展示遗漏均保留，不回写成全系统通过。固定回放生产仍是86679，后来仅文档与census测试变化，不借测试迁移另跑第三例。

末版全仓耗时tool406.290s、agent95.936s、tracequery120.921s；相较已留存437.335s的owner登记失败是独立正式成功收据，不覆盖旧日志。

§84–88修复、定向/全仓收据、两批固定回放审计与剩余任务清单已随`dc570c66a`推送main（session30448正式exit0，189bcf48e→dc570c66a）；本地/远程HEAD核对相等，工作区干净。三项窄子缺陷收口，父账与所有明确未通过项继续保留。

## 88. 后续小批拆分：身份可见性先行，重复分尺失败不能只归因“信息已在场”（2026-09-21，身份子清单已由§89验收，分尺仍开放）

原生身份接缝已由根席再核：`run_tests.go::renderTestSummary`只展开失败名，`write_context_pack.go`跳过PASS结果，controller主要给计数/命令；两种计划schema逐字重复裸身份示例。结果生产者为非根suite/id同时加`runner[/framework]@cwd::`，仅Python/Java包含framework；Go suite是报告Package/import path而非源码package声明，Jest/Vitest的ID保祖先标题链，JUnit保class#method，RSpec保full_description，其它也须以当前报告完整字段为准。不能从命令Suite选择器推定TestResult.suite。`TestResult`旧注释将ID匹配对象写成AcceptanceTests、称一次调用suite恒同，也是待纠正注释债，不冒称实际提示已如此授权。

下一小批验收清单（仍属HMC-16.4/18.5，不新增父任务完成数）：

- [x] 两个schema的PTO身份说明单源且JSON编码一致；解释完整字段/非根前缀、Go及框架差异。首轮尚无报告仍可正常规划，不教成必须先执行测试才能发计划。
- [x] 用同一有界展示函数向工具实际返回、planner/controller投递当前持有报告中的原生assertion PASS身份；整体report失败或required证明不足时也保已执行的具体PASS，不自动代填PTO或关闭合同。
- [x] 绑定明确active plan与报告PlanID，保持post-apply/非planner-probe边界；当前空/错plan/channel报告不能复活历史context-pack内容。heading说明是持有报告快照、附GeneratedAt，不把仅同PlanID当最新工作树字节或同scope执行代次证明；不存在的invocation ID/通用报告路径不得捏造。
- [x] IDs按真实字节JSON转义，控制数量/总字节预算；超长整项省略并说明，不截出新身份、不剥前缀/归一化名称、不按源码或原始散文猜test_path。aggregate/non_asserting/build/plain probe不能混成原生断言。
- [x] 实际agent消息与公开RunTests→报告→消息交接回归；覆盖根/非根、7runner格式、错plan/channel/空报告/历史失效、混合PASS+FAIL、控制字符/Unicode/过长ID，并证明matcher、source-free PTO拒绝与required证明边界不变。非本机runner格式单元测试不算实际执行。

现成可借用的是当前报告选择/active scope及failure/probe observation的有界独立提示结构；原failure/probe摘录会截短ID，不能拿来作可复制原生身份。`authoritativeWriteControllerReport`有兼容空PlanID/Channel，新的身份区不能仅调用它就声称来源明确；`TestResult`本身没有invocation ID。来源授权与只读补绑定继续按B2–B6独立验收。

独立设计复核补充：既有`NativeProjectTestObservationBindingTeaching`已经统一“引用原有测试不需伪造文件修改”的权限教学，应复用它并另抽suite/id字段说明，不建第三套规则。工具出口优先核`run_tests.go::installFinishedReport`，与实际planner/controller初始消息共用展示。七个selector runner与序列化协议种数不相等：Python有unittest/pytest两形，Java等共享JUnit，Swift另有`Test Case '-[Suite testName]'`→suite/name，不能漏Swift或据七类协议摘要宣称全矩阵。首轮无report静默，明确post_apply_verify/PlanID匹配，原报告字节及PTO不变，仍只是当前持有快照的可见性而非新执行授权。

业务分尺的独立二次审计没有找到明确的错误等式或准入自冲突，但确认稳定的显示风险：末张事实卡突出query51=6+1+44，business50=5+1+44在更早卡片，末尾业务行仅留50总量；链上31ms阻塞和背景47ms请求又共用IO延迟泛称。`answer_document_final_decision_boundary.go:286`要求使用该读者标签，邻近机制边界仍明确区分请求与阻塞，因此不能定性为系统授权错误换尺。`trace_span_scheduler.go:92`确实以业务起止重建真实状态，并非按宽查询比例缩放；参考`core/preprocess/sleep_ops.py:198/240/577/593`的局部窗交集能力已有对应实现。

- [ ] 后续上下文精简小批先把现有“业务窗/查询窗”和“请求驻留/闭合阻塞”在同一成文卡紧邻呈现、削减重复摘要，保来源/线程/窗口/覆盖限制；先做公开消息接缝回归再固定双例人审。此项目前是呈现风险设计，不是已证新P1或已修系统缺陷，禁止追加同义教学堆叠、正文扫描硬门、模型结论代写或根因资格扩展。旧业务FAIL、H4两尺遗漏都继续开放。

## 89. 原生断言身份教学与当前持有报告交接（2026-09-21，子片验收完成，父项留债）

从干净的`6647135b9`继续，远程核对无落后。本批落实§88前五项，后续Trace同卡分尺/B2–B6不混入本片，也不以展示成功抵销旧人工FAIL。两路施工分别负责实际计划schema的单源身份说明、当前报告PASS身份的有界只读展示；根席独立补真实公开ReadFile→EmitChangePlan→ApplyPatch→RunTests→JSON往返→planner/controller消息回归，包含根目录零发现、非根原生PASS、错PTO仍proof_weak、混合原生FAIL。

原生展示只使用producer已标记的assertion范围、普通测试种类、PASS和完整非空suite/id，不按名称猜runner或授权。必须当前ChangePlan.ID非空、与report.PlanID逐字相等，channel明确post_apply_verify；不复活历史pack。身份按整项JSON转义和预算省略，不重建前缀、不猜test_path、不创造执行代次或报告路径。报告时间只是持有快照时间，并非最新源码字节已验证；原PTO、matcher、required合同和source-free准入不变。

实际planner/controller消息RED `/tmp/hmc-native-identity-red-20260921.log`（session17487正式exit1）：PASS和混合FAIL都缺完整可引用身份及快照边界，非编译失败。根席公开链首次执行时共享实现已落入工作区，`/tmp/hmc-native-identity-public-red-20260921.log`（文件名沿用启动计划，session44960实际exit0，agent1.940秒）只能记首次GREEN，不能伪称公开RED。后续末版定向、race、全仓、干净构建和固定双例另附正式收据。

再次对照参考`core/llm_contract.py:114–155`的成员/字段工件回查和`core/preprocess/sendable_ops.py:494–550`的命令结果/输出工件分离，仅借鉴结构身份与执行结果分离；参考无本仓PTO/多候选断言体系，不移植其散文数值扫描、子串放宽或外部退出码代断言。父账维持13/79交付、66开放。

实现已分别提交`a15614f57`（当前报告身份只读交接）和`bfa381902`（身份教学单源）。新展示限8项/8KiB，编码前检查过大身份，编码后按总预算整项省略并报数量；非法UTF-8不以替换字符铸新身份。工具在安装报告后、同一出口取当前plan，planner/controller在历史pack之前投递；保原出口审计/失败附录，新增生产/变异针而未修改旧census。实际两个schema共享JSON编码后的说明，Jest明确分隔符为`" > "`（两侧空格），8协议×根/子项目/兄弟项目使用真实parser/qualifier校验，非8套运行环境执行。

有效schema RED=session42327 exit1/tool1.171s，`/tmp/hmc-native-identity-teaching.BihPFp/RED-schema-valid.log`，只缺教学的两个schema失败，24协议scope格正常；首次GREEN=session16251 exit0/tool1.329s。身份末版定向`/tmp/hmc-native-identity-focused-final-20260921.jsonl`（session90844正式exit0）13顶层/96子项、0FAIL/skip，types0.825/agent2.994/tool1.100秒，包含根席两真实runner公开链及原安装出口census。前轮v2 WorkingDir测试装配编译错误已更正为Root，记录保留且不算产品RED。8项超时默认/活跃流保护`/tmp/hmc-native-identity-active-stream-20260921.log`（session38195正式exit0，llm5.499秒）通过，含4ms阈值连续部分帧场景；并未改timeout代码。

统一末版全仓`/tmp/hmc-native-identity-full-20260921.log`（session17118）和独立race仍运行。随后只跑固定`nested_python_increment`＋`trace_query_business_marker_io_chain`，2并行×1：按已复现写模式身份缺口/跨模式保护选例，Trace保旧50/35/31/两段1ms与业务线索的人工检查；不强迫模型采用新分支，不为追绿追加第三例。全部正式收据齐备前保持验收中。

末版race已齐：身份展示session64220正式exit0，同13顶层/96子项（types2.046/agent3.940/tool2.919秒）；教学相邻四包60顶层/0skip，focused55496正式exit0、race35530正式exit0，日志`/tmp/hmc-native-identity-teaching.BihPFp/GREEN-{focused,race}-final.log`，含旧既有测试公开五枝和非根scope四枝。教学末版schema/protocol97692也正式exit0。干净构建session61807正式exit0，revision=`68730664c8cb`、buildTime=`2026-09-21T11:47:55Z`。固定双例runner40831已按2并行×1开始，结果根`eval/results/hmc_native_identity_crossmode_20260921`，尚未终签。

live进行中确认的新相邻P1上下文缺口单列待修：实际完整原生pair已到工具/controller消息，但既有`WriteContextPackFromChangePlan`的PTO摘要只留id/test_path/contract_refs，未交付声明的suite/id。模型把“有PTO+原生PASS”误当系统映射故障，首次申请all_verified被既有校验正确改为verify_batch；未误放行。下一小批应让当前声明pair与当前观测pair并置并保源身份，不做系统推定匹配或自动改绑。普通pack文本上限240字符会截断，不能简单拼字段进去制造可复制的残缺身份；需完整JSON条目、独立有界展示与当前来源检查。此发现不是B2权限已开放，也不为尚在运行的本例签最终FAIL/PASS。

末版全仓session17118现已正式exit0：87测试包、13无测试包、零FAIL，tool432.783/agent122.240/tracequery143.528秒。独立只读审查PASS。§88前五项子清单由本节实现/公开消息、schema、协议、race和全仓收据完成；不把持有快照当同工作树代次证明，不代销B2–B6。最早运行中记录保留，以此正式退出更新状态。

## 90. 687306固定双例：完整人工0/2，实体提及越权选根优先修（2026-09-21）

[机器摘要](../../eval/parallel_selected_summary_hmc_native_identity_crossmode_20260921.md)、[完整人工审计](../../eval/parallel_selected_summary_hmc_native_identity_crossmode_20260921_manual_audit.md)：runner40831正式exit0，2并行×1，机器1/2、完整人工0/2，无第三次追绿。Python237秒真实3次×3条断言PASS，原测试/配置不改，唯一源码修复正确；当前身份交接实达工具/controller共7区块21完整身份。3个required仍0/3，声明与真实限定suite/id不匹配；source-free未授权补登记，最终诚实unverified。当前声明pair缺展示是下一独立接缝，不能放宽matcher清债。

Trace253秒虽补回35ms请求和LoadDocumentIndex40ms，仍将51ms查询的6+1+44套进50ms业务、8与9.5ms混尺，且把入睡起点当请求起点、完成事件执行者当被唤醒者。最后输入有完整正确分尺，原始测量/上下文可用；不增加正文关键词硬门。保因果投影、schema2旁路三项31/1/1与背景47隔离，旁路模型description亦须独立审计。

两席独立确认更高ROI系统P1：唯一wakeup查询目标app-main，原生路径storage-irq→worker→app；analyzer明确`no_named_target`，通用Entities却把worker排在app-main前。`observationLedgerAnchorEntities`无条件收通用实体，路径选举按首次命中截到worker并标用户选举；renderer继承后称worker为“用户关注线程/自身”，与app-main状态账矛盾。不是合法导航游标改变，也不能只换图头掩盖截链。修复应从typed用户目标授权单源收口，并同步显示的明确无目标状态；保完整原生路径、真正named目标、同tid别名、cursor排除与旧nil-profile兼容。不强制业务实例覆盖full-artifact，不改物理数值/根因资格或由原文扫描重新选目标。

后续任务顺序：上述目标授权P1→声明/观测完整pair并置→业务/查询与请求/阻塞同卡分尺→B2–B6和其余HMC开放项。旧人工FAIL不回写，父账13/79交付、66开放不变。

§89–90实现与全仓/人工收据已随`cb2f3679b`推送main，session55235正式exit0，远端6647135b9→cb2f3679b，本地/远程HEAD相等。下一片不复用该全仓签新代码。

## 91. 用户目标身份授权与原生唤醒路径分离（2026-09-21，子片验收完成）

先修§90确定系统P1，不仅针对单个no_named枚举补丁：现有emit契约已要求named_target有当前原文quote、有效RuntimeTarget且source=user_explicit；no_named/unspecified不准同时携带targets。消费端却忽略整个profile，把generic/Exact实体重新提升用户身份。因而unspecified，以及named目标不在当前路径时由generic其它线程代选，都是同一类越权，不应逐case补。

实施边界：已有profile时仅其有效named目标通道可授用户身份，由ledger/renderer共享精确resolver；generic/Exact仅保探索/旧时间展示等非身份用途，不允许选根/截链/伪用户标签。profile缺失的历史输入保持旧B1顺序及兼容；查询游标不授用户身份，frame_target_resolution只印证已授权目标。不能因明确无用户目标而丢原生完整路径、修改查询范围或把已接受业务实例强盖full-artifact/明确时间窗；不改根因测量、证据等级、原始结果或模型正文。

根席独立公开RED：`/tmp/hmc-user-target-materializer-red-20260921.log`（session99693正式exit1/tool1.547s），真实四view→当前Bus观察账本→投影→ApplyAndPersistMutation→最终中英渲染；no_named/unspecified四枝均误收generic/Exact实体、将irq→worker→app截为irq→worker并标用户选举。加入Mutable历史named与当前分析相反的输入，当前两端应消费当前AnalysisIR，不得复活旧身份；最终摘要和原工具记录保持不改。公开fixture/线程换序/别名、named/legacy及相邻全仓另验，不将尚未完成测试签绿。

对照参考`core/preprocess/sleep_ops.py:931–932,1077`从已选线程行取tid后递归、558附近按该阻塞段查直接waker；其639–642树根是阻塞者而非本仓被分析目标，也无用户关注chip。只借鉴查询目标与递归节点分离，不复制参考根语义、丢中断waker策略或最大状态桶机制归因。现有状态裁窗/IO请求35与线程阻塞31/业务50及40/背景47、语义优化和供给证据通道保持。

冻结后固定双例预选业务IO旧FAIL与`real_trace_h4_supply_thermal_witness`，2并行×1：前者验证无指定线程、完整路径/分尺/旁路；后者保护真实named线程和明确时间窗下的有限判断、不擅自扩因果投影。写模式前批真实执行/未绑定分账保留，不以两读例宣称写模式本片live覆盖。

实施已提交`0d339165f`（8文件，4生产/4新测试，旧测试不改）。共享`RuntimeUserTargetAnchorEntities`区分profile缺失与明确空授权，present只列有效named/user_explicit身份；ledger与renderer共用，原Entities时间显示/软候选保留。renderer不能让旧elected标记越过当前授权。根席末审再找出typed显示形跨端接缝：原compiler支持`worker-200 [200]`/`worker [200]`/`worker-200 (200)`，旧renderer不支持，取消旧短路后会自相矛盾。有效RED `/tmp/hmc-user-target-typed-face-red-20260921.log`（27054正式exit1/tool1.241s）三枝成立；present-only比较现复用原typed matcher，nil旧比较不变，不把通用文本升级成typed。根席两顶层七子项GREEN9203正式exit0/tool1.397s；identity census81714正式exit0/tool0.971s，无需改任何旧登记。

另一公开入口RED-v2 `/tmp/hmc-no-named-anchor-public-red-valid-20260921.log`（80067正式exit1/tool1.425s）2顶层13子项，8问题枝红、5named/alias/cursor/legacy对照绿；第一轮业务predicate装配错误日志保留，不作有效产品RED。扩unspecified×改名及链外named后，GREEN91197正式exit0/tool1.965s、race66186正式exit0/tool4.828s，2顶层16子项。后来三枝首次GREEN，不伪记RED。root最终公开集合race68340正式exit0/tool8.455s，共4顶层23子项，完整路径、worker非自身、各物理量与原工具记录保全。

4生产hash冻结：profile=`23d00843d0f62cc249c2b36ee6347856be1bbf2034cb33aa8866f035ede62e42`，ledger=`5595fd92e567035a81cc83720bc2d827a4dc818000e6909a4e7ea037784cf39a`，runtime=`679977aba7373a4fa20327ff677692a51004d1414eddf274d6f2750f3ffb324f`，tree=`46537f87e7ec7b2b81e99529dd4179b1e3e55b3eac2e985669131e411f328065`。独立只读末审无阻碍。文档提交`819dc5b91`后干净构建28837正式exit0，revision819dc5b91f74/buildTime2026-09-21T12:16:26Z；56014活跃流/默认8项正式exit0/llm7.144s，600/300/600秒及4ms连续部分帧保护不改。统一全仓93049、旧针race80995仍运行；固定双例65962在05:16:56同时开始，未签live结果。

相邻陈腐状态边界单列开放：`observationRecordMatchesUserRuntimeTarget`仍不读profile，除了软排名也被`trace_value_occurrence_authority`/`trace_blocking_wall_clock_authority`/`target_wait_occurrence_authority`使用。fresh no_named/unspecified已有targets为空的emit前提，此轮未见合法producer触达旧矛盾状态，故不冒称第二个本次生产P1；后续应有真正authority消费者的历史输入正反回放再统一present分支，不只加prompt排名单测。当前不借此扩大本片已冻结源码，也不宣称全系统无同类入口。

最终收据已齐：统一全仓93049正式exit0，`/tmp/hmc-user-target-authority-full-20260921.log`共87测试包、13无测试包、零FAIL。旧针race80995正式exit0，47顶层/36子项、零FAIL/skip（types1.908/tool15.214s）。B1790最终矩阵25298正式exit0（types7.717/tool8.305s）、race35660正式exit0（types2.246/tool3.440s），4顶层54子项，日志`/tmp/hmc-user-target-authority-matrix-final-{green,race}-20260921.log`。加上上述公开RED/GREEN、当前Bus→最终渲染、独立只读审计，本节窄目标授权子片完成；以下live完整失败不抵销也不误销父项。

## 92. 819dc5固定双例：完整人工仍0/2，补齐与恢复指引分别留债（2026-09-21）

[机器摘要](../../eval/parallel_selected_summary_hmc_user_target_authority_20260921.md)、[完整人工审计](../../eval/parallel_selected_summary_hmc_user_target_authority_20260921_manual_audit.md)：runner65962正式exit0，2并行×1，机器0/2、完整人工0/2，未重跑第三例。业务261秒、H4 156秒。业务投影保完整irq→worker→app并正确称“分析锚点”；成功分析仅业务名/backup实体、未出现旧worker/app弱实体排列，因此本轮不冒称反事实命中旧截链，公开回归覆盖该性质。H4明确目标/时间窗下有限事实回答没有强添因果图，核心结论通过；机器频率上限检查未识别范围写法保原FAIL，独立IO等待遗漏/内部枚举使完整人工继续FAIL。

业务最终输入完整提供50=5+1+44、40=8+1+31、本轮宽query53（归账52=7+1+44，未归账1），以及请求35/阻塞31、completion与wakeup两时间；模型仍混尺、误称发起者、把未证唤醒写成否定，并泄漏内部词。H4至少四段闭合IO等待并集4.384ms仍未完整披露。不是上下文缺数字，不增原始正文关键词硬门，不代写模型结论；继续按同卡分尺/减重复与独立披露接缝推进，不凭旧教学已在就销人工FAIL。

新确认系统接缝及排序：

- [ ] 高ROI窄恢复教学：业务成功前一次已声明no_named_target却留空壳targets，`parseRuntimeTargets`先拒并命令“修身份、不要省略”，与下一profile门要求空targets冲突。模型后来自行清空恢复，非无解硬门；应按已有typed profile给一致修复路线，保named真实身份/quote、无profile/invalidprofile/非运行时兼容，不放宽准入。
- [ ] 自动补齐设计：本轮六query漏rank/critical；接受business_span_ref后，既有full_artifact分支不允许业务focus缩窗，而app/worker混PID cursor无法单选，最终skip=no_typed_target，旁路schema2正常空数组/no_selectable_typed_on_chain_candidates。是既有组合边界，非本节四生产回归；对照参考sleep_ops选定blocked_tid→直接waker，参考无此权限组合的可直接移植方案。必须区分用户范围与已接受实例，不能删full-artifact旧pin、系统代选app或把cursor升级用户身份。
- [ ] 当前声明与当前观测完整原生pair并置（§89）；业务/查询及请求/阻塞同卡分尺（§88）；相邻历史authority消费者回放（§91），各自最小验收，B2–B6与父账13/79、66开放不变。

§91–92随`af64dc340`推送main，session71034正式exit0，origin从cb2f3679b前进至af64dc340，本地/远端相等、工作区干净后再开下一片。

## 93. 空壳目标恢复教学与准入规则一致（2026-09-21，子片验收完成）

§92实测no_named_target+空壳targets的提示会把模型引向另一个门：列表先拒并命令不得省略，profile下一步却要求无targets。本批仅修条件式恢复教学，不改parser签名、校验顺序、reject条件、schema必填/enum、错误census或当前请求原文quote校验。真正named空列表的“不能省略”拒绝保留；missing/invalid profile不被默认为no_named，null仅保既有兼容不鼓励生产。不会把空壳自动变成无目标，更不会从工具/工件造身份。

`AnalysisRuntimeTargetRosterTeaching`单源交付实际两个目标schema描述、既有analysis-skill段、malformed-roster错误尾句：named保完整身份与profile的原文quote/source=user_explicit；no_named/unspecified用空数组或省略字段，不放占位对象；not_applicable仅限非runtime请求。选择固定条件式而非读尚未验证profile作唯一恢复决策，两席确认比新增helper/分支更窄。参考`config/indicators/sleep/thread_sleep_summary.yaml:9`和`core/query_engine.py:120`的必填tid属于已选线程查询；本仓用户身份声明与查询输入分层，不能直接移植其补tid提示。

根席公开测试第一轮session8809是测试误用不存在Schema()的编译失败，保留日志、不算产品RED。更正为实际Parameters()后，55993正式exit1/tool1.294s（`/tmp/hmc-target-roster-teaching-red-valid-20260921.log`），3顶层12子项均只缺一致指引；同次每条真实拒绝→模型自修→成功/反例已执行，无准入失败掩盖。初次GREEN88981正式exit0（tool2.359/skill0.866s），含旧profile错误census、whole-set拒绝、named身份/source与hard-arm登记不改。后续独立兼容针、race、统一全仓与冻结双例另记，未齐前不终签。

下一固定双例按ROI选真实C2全工件D/IO等待清单＋Go一行编译修复：前者保named/全域统计而不强拓因果，后者查非trace写模式分类/执行无回归。业务旧FAIL已经两批充分暴露，未改分尺/补齐前不继续重复同例追绿。只跑2并行×1，写计划是否成功与真实验证/人工结论独立分账。

## 94. 全工件范围下已接受业务实例的局部补齐（开放设计，未实现）

两席参考实现/消费者审计确认可泛化，但不是删除full_artifact短路。用户full_artifact是原回答范围，已接受且current的私有business_span_ref是完整实例TID/窗口/物理代次的局部查询凭证，cursor仅导航；三者不得混为用户身份或根因选择。参考`sleep_ops.py:198,577–612`按父阻塞窗交集查询和`cold_launch_window.py:499–539`完整TID/端点传递可借鉴；751–770缺标记时选最早候选不能移植。

- [ ] 决策：仅既有RuntimeTraceReportShapeAuthority明确允许完整报告的full_artifact请求，可用成功settle且current的模型选择实例做局部补齐。保原RequestModel不变；用{view,business_span_ref}原子调用，不拼cursor身份/窗口；work/frame布尔不新授因果权。
- [ ] 披露/覆盖隔离：现有TargetSource=accepted_business_instance及执行窗用于“只补齐已选实例”的标注。不得将用户full_artifact回填旧meta.RequestedArtifactScope，该字段在CompileRuntimeArtifactScopeCoverage被用作全域扫描凭证；局部执行不增加全域count/complete覆盖。
- [ ] 公开正针：真实查询发布ref→成功completion settle→补齐→ledger→投影→最终上下文。多cursor两顺序、改名、无cursor、子业务实例；真实业务50=5+1+44、请求35/闭合S等待31/背景47分开，完整irq→worker→app，不标用户指定。
- [ ] 失效负针：无选择、pending/失败/取消、冲突、旧代次、JSON重放不得触发；来源/TID/窗不符的旧家族不能抑制补齐。执行途中失效停止后续查询但保已完成事实。明确单/多窗和不匹配用户目标保原通道。
- [ ] 全域/有限保护：原FullArtifactScopeOverridesNarrowModelWindow、真实C2第三次等待和ScopeAndNoChoiceCompatibility/full_artifact断言不降。有限D/IO/关系/effect问题保持原windowless census，不能变成因果报告或被单实例补齐替代。
- [ ] 旁路验收：补齐只交候选事实，不替模型选根因；合法链上候选可选、无合法选择仍诚实空旁路分别验证。不得将强制非空作为通过条件。

本节只是可施工任务拆分，未改生产、未计新增交付，父账13/79、66开放不变。

## 95. 309be8末版验收与C2独占状态桶误述（2026-09-21）

§93实现`8880c6f60`，记录`309be8dd1`，两个生产文件只改教学、两个新测试文件，准入条件不变。末版root race3079正式exit0，13顶层14子项/tool8.289秒；独立兼容focused97440正式exit0/tool1.230秒、race82628正式exit0/tool3.880秒，3顶层17子项。首轮兼容fixture装配错误不记产品RED。统一全仓21288现已正式exit0，日志`/tmp/hmc-target-roster-teaching-full-20260921.log`，87测试包、13无测试包、零FAIL。独立生产复核PASS。

干净构建8595正式exit0，revision309be8dd13b5/buildTime2026-09-21T12:36:03Z。默认值/活跃流82595正式exit0/llm18.908秒、8项；实际agent direct/nested活跃SSE回归91206正式exit0/agent1.137秒，4ms evaluator预算不取消活跃连接。600/300/600秒不变；日志里的3分钟analyzer终止教学预算不是流式请求取消期限，不据此另立超时故障。

[固定双例机器结果](../../eval/parallel_selected_summary_hmc_target_roster_recovery_crossmode_20260921.md)和[完整人审](../../eval/parallel_selected_summary_hmc_target_roster_recovery_crossmode_20260921_manual_audit.md)：runner82068正式exit0，2并行×1，机器2/2、完整人工1/2，不加第三例。Go真实一行修复、原测试完整执行、非runtime首轮分类成功，全部三项合同planning_only，不能代销B2。C2全工件自动补齐、三段IO等待0.635ms、有限问题空旁路成立，但错误宣称未进入D，不能人工签PASS。两例未命中空壳恢复现场，该子片由公开RED/GREEN及兼容矩阵验收。

确定新P1：原始D状态经scheduler iowait标记分入独占io_wait桶；既有`TraceUninterruptibleWaitMS`和target_window_state_account说明均要求D展示折回DStateMS+IOWaitMS。最终教学却说io_wait在d_state_occurrences=0时不得称D，实际输入1802和模型1906均见此矛盾。探索阶段968也已误读，不能称仅finalizer导致。优先按既有producer合同统一教学/展示，不变任何桶、时长、排名、范围、根因权限；S+iowait、独立IO请求/完成闭合等待不因名字含IO而晋升D。逐段PrevStateRaw在account occurrence省略可留载体完整化，但此已证矛盾不必先扩schema。

- [ ] D/IO分类与原生D展示教学纠正，真实producer→最终上下文公开红绿；保G12 S+IO零D、sleep非加项、三面计量census、容量/范围/冲突边界。仅迁移既有错误文案pin，数值断言不降。
- [ ] §94已接受业务实例局部补齐六项；不造全工件覆盖。
- [ ] §89声明/观测完整pair并置、业务/查询同卡分尺、§91历史authority消费回放；B2–B6及其余父项仍开放。

旧人工FAIL保留；父账13/79、66开放不变。

## 96. D/IO分类与D总量教学的同类接缝统一修复（2026-09-21，窄子片验收完成）

上一批§93/95三笔随`39ebdd72d`已推送main，session35559正式exit0，远端af64dc340→39ebdd72d；本地/远端相等、tracked工作区干净后开始本片。不把上批全仓收据套用新代码。

主席直接对照参考`core/preprocess/sleep_ops.py:225–255`：按窗口半开交集计量，Running/R/R+/S与原生D分别记账，state.startswith(D)归D。参考没有本仓D侧IO子桶及S侧IO overlay的全套权限/根因账，不直接移植其算法；本仓既有`TraceUninterruptibleWaitMS`/`FormatTargetStateAccountCaliber`才是当前计量单源。本片不扩schema、不将独立IO完成闭合等待重分类、不改物理统计。

独立审计发现需同批处理的相同语义问题：final recap禁止io_wait称D；finite与state-duration教学把未带IO标记的D也说成IO字段；业务片段/关联线程/当前线程的raw DStateMs显示为整个不可中断等待。计划统一计数/桶教学，raw D只限定为非IO子桶，不手算新的折算值或改全局D词根。持有源状态的完整化另留，当前精确producer已足以修正错误，不从原文猜状态。

根席公开RED session59881正式exit1/agent1.703s，日志`/tmp/hmc-d-io-public-red-20260921.log`：实际TraceQuery(C2真实三段、D+iowait、D无IO、S+iowait)→当前Bus→BuildAgentContext→最终BuildInitialInstruction，1顶层/4父场景/8语言子项。全部原生分类、次数、端点、墙钟值、上下文单源口径先通过，仅旧相反教学失败；数据/投影/模型正文前后字节仍同。独立矩阵RED session49802正式exit1/agent1.126s，1顶层8子项，同样有效。生产尚未终签，新增同类展示面的RED/GREEN、census/race/全仓、固定双例另记。

冻结后固定C2全工件清单＋trace_query_wakeup_causal_io_chain明确20ms窗：按已确认错误及异构因果/优先级/四节点关系保护选例，2并行×1。前批Go真实写模式PASS仍保留，但不冒称本片再次live覆盖写模式或B2。机器和完整人工分账，不为追绿追加第三例。

core已落：`TraceSchedulerWaitPartitionTeaching`单源区分互斥桶/原生D/睡眠内IO细分/独立请求与闭合等待，recap复用既有caliber，唯一AL旧错误文本pin精确迁移，原0/3计数和排序/清单数针不改。root公开首次GREEN18591正式exit0/agent1.238s、race58040正式exit0/agent7.609s；独立focused51153正式exit0，共34顶层，含原生G12、状态折算及formatter census/self-red（agent32.331/types0.968/tracequery1.718秒）。追加根席工具面RED21862正式exit1/agent1.705s，仅四实际query的早期preview缺同源说明；随后工具既有preview一行投递，GREEN2849正式exit0/agent1.672s。公开正针追加于此前初始RED之后，按实际批次记录，不伪记均先红。末版统一race/全仓另验。

默认值/活跃流37837正式exit0/llm3.253s，8项，保600/300/600秒及4ms连续部分帧、隐式推理/tool-call活跃流。未修改超时实现。

核心提交`d168ee40b`、同类三生产面提交`b55ca5d18`，统一冻结。后者新公开测试的首版漏传trace使skill面缺失（root诊断13397/agent1.143s不记完整有效RED），修fixture后86666正式exit1/agent1.176s：6真实B/E业务状态→finalizer分支及6原始note标签失败，原计量/机制上限前提正常。GREEN57753正式exit0/agent1.276s，精确迁移另一个旧zero文案pin，其他断言不降。业务/三类raw note复用既有`TraceStateNonIODStateWord`，finite/defaults用同一常量替换旧句而非保留冲突；无手算或新开门。

独立末审查8类原生TraceNoteKeyDState发射（target/rank/causal/aggregate/churn/IO pressure/burst/summary），均分别取DStateMs与IOWaitMs，没有已fold值混进raw D键，标签修复安全。旧tool的io_wait_zero_scope窄注释可保留；PTV7 canonical-token五桶快照不是自然语言D总量，既裁载体不改，不声称本片已消除所有内部词。参考的原生D/S区分与本仓更细两层分类一致，但不复制其算法/最大桶根因。

干净构建43686正式exit0，revision=b55ca5d1809f/buildTime=2026-09-21T13:00:58Z；统一全仓10053、根席末版公开race42781、同类面focused82597/race9363仍待正式退出。core独立race11290已正式exit0，34顶层49子项（agent160.799/types3.504/tracequery2.306秒），只能签其已运行集合。固定双例56191已启动；完整结果及人审另记，不提前销账。

后续正式收据：42781 exit0/agent8.840s，5顶层32子项；同类面82597 exit0（agent44.095/skill0.966s）、9363 exit0（agent240.060/skill3.412s），均52顶层89子项、零FAIL/SKIP/竞态。原生状态/业务/独立completion与Binder/有限范围/census保护均通过。

全仓10053正式exit1，不销失败：86测试包通过、13无测试包，唯一tool包失败（452.906s）。原针`TestTraceQueryWindowStatsRendersVsyncGeneratorCensus`发现新状态说明使帧节拍发生器普查进入StoreBlob截断中段；不是原始计量变化。独立原针RED83775仍正式exit1。修复仅将canonical window_stats的既有普查调用移到同一stanza起点、可变资源明细之前，删除旧调用；不重复、不扩摘要额度、不改event_search/复合view或旧断言。GREEN6651正式exit0/tool1.794s，含相关普查/状态账号/等待preview；末版源码`trace_query.go` SHA256 `5631d5e8f738e575690c97bc61bd1ed93b6cd55064eb0a320a3e7099bc6964a7`，新全仓77783与race75036另验。b55固定live不冒充覆盖这次纯排序修复。

## 97. b55固定双例完整人工0/2：早期投递与因果事实交接分别留债（2026-09-21）

[机器摘要](../../eval/parallel_selected_summary_hmc_d_io_semantics_20260921.md)、[完整人工审计](../../eval/parallel_selected_summary_hmc_d_io_semantics_20260921_manual_audit.md)：runner56191正式exit0，2并行×1，无第三次追绿。C2机器PASS/197秒，因果链FAIL/196秒；完整人工均FAIL。C2三段IO0.635ms和独立Binder0.524ms正确，新共享教学实际到达终答输入；模型仍误否定原生D，并从小占比越权推断影响可忽略。旧答案不倒签。

- [ ] 高ROI投递缺口：无窗`thread_timeline`已算出243段，但Run只给显式双端或无窗window_stats发布TargetWindowStates；summary前12段看不到第三段IO，清单及共享说明也因账号缺失未交付。复用当前精确timeline经单一builder补账号，不重扫；实际timeline.Window是计量边界，不能重标full_artifact或改权限位。保半端/线窗/重名/TID代次/取消/完整性拒绝，S+IO不升D，32段cap继续披露不完整。新纯投递路不追加Binder索引/配对，既有bounded/window_stats/bundle行为不变。
- [ ] 更高风险组合交接：因果例已采集threadpool的11ms D侧IO/fscache与三依赖各1ms runnable，但analyzer收窄bounded_effect_verdict，实际终答只展示40条中的6条目标自身状态，已选依赖事实被滤掉、投影0/1。不能仅归模型波动。审计typed问题范围、已接受链上事实及报告形态一致性；不扫用户原文切换合同、不把completion自由说明升级权威、不将链外背景当根因。原生irq边存在于wakeup census，展开止于终端IO，不误称Mermaid删边。
- [ ] P2恢复可操作性：C2原summary已member_set但有两个required清单维度，原子add_facet_id未发布，模型仍调用后正确被拒。代码发布与执行共用投影，条件式hint说发布时才使用；本轮未证新硬合同矛盾。应按typed文档形状区分缺绑定与缺承载/数量，后者指引模型自己补合规结构；不扩schema/重试预算、不系统代写成员。
- [ ] §94已接受业务实例局部补齐、声明/观测pair、业务同卡分尺、历史authority回放、B2–B6及其余HMC任务继续开放，父账13/79交付、66开放不变。

两份schema2旁路均实际生成；C2有限清单的空数组合理，因果例`trace_root_cause_contract_not_active`是上游合同/交接问题而非落盘失败。模型主文还将CFS threadpool称RT、从network/cookie名称虚构业务职责，附录正确数值不抵销主文缺失。本片修正事实解释，不改变任何窗口/值/排名/源身份/根因资格、JSON准入/所有权或600/300/600秒与活跃流保护。

§97因果项进一步审计限定：本例接受的typed tuple本身是自洽的finite＋target_effect_verdict，既无required causal_attribution也非root intent/diagnostic；relation_path只给展示要求，不能独自授因果权。因此“11ms未到最终输入”是事实，但尚不能判定现有finite过滤器违反已授合同；优先核分类教学与同一真实fixture下coherent causal/finite两套公开交接。没有证实正确causal授权也会丢事实前，不下游强开投影/恢复自由reason或根据已查询root结果升级用户要求。模型误分类与确定性交接故障分账，保持有限问题原有保护。

§96末版收据闭环：普查排序修复`63e55b0a0`，记录`3fd1326f6`。75036正式exit0/tool8.430s，85814最终实际query→成文race正式exit0/agent7.673s。末版全仓77783正式exit0，`/tmp/hmc-d-io-vsync-final-full-20260921.log`共87测试包、13无测试包、零FAIL；首轮10053失败保留。干净构建12468正式exit0，revision3fd1326f6bf7/buildTime2026-09-21T13:16:46Z。独立只读末审PASS，旧普查原针及event_search/位置针不改；状态/窗口/源身份/模型正文不变。仅窄实现验收，§97完整人工0/2和后续开放项不代销。

## 98. 复用原生时间线交付完整等待清单（2026-09-21，窄子片验收完成）

§96–97批次已随`8702ea54f`推送，session31911正式exit0，origin39ebdd72d→8702ea54f，本地/远程相等且工作区干净后开本片。参考`core/preprocess/sleep_ops.py:225–255,502`的已选线程/窗口上保留原状态片段可借鉴；不搬其最大状态桶归因、近邻连边或中断waker丢弃。本仓已有精确Timeline/TargetWindowStateAccount，缺的是前者已生成后未向既有展示链路投递，不新增第二分析内核。

真实公开RED71736正式exit1（tracequery0.887/agent1.125s），`/tmp/hmc-timeline-delivery-red-20260921.log`：C2完整243段已在native结果，但三段清单和单源定义未到工具摘要，随后同一工具结果→当前Bus→最终中英输入均无等待清单权限卡；非目标、半端/反窗、事件cursor、生命周期/坏scheduler等旧拒绝控制保持。结构针确认旧路径还重复计算bounded timeline。

最小实现：Run唯一generic mint不变；无显式双端、无线窗的已完成单线程timeline经原builder投递状态/等待/S细分/CPU清单，直接使用tl.Thread与tl.Window，不改用户端点/范围profile/根因资格；bounded timeline也复用已有结果，其他view仍经旧helper。新timeline-only路径不追加Binder配对/索引，原bounded/window_stats/bundle的独立Binder保留。无区间/解析完整性失败/重名歧义/TID代次冲突/取消都不重建为零。32条清单cap保incomplete，完整计量不受普通12条展示/limit/min_duration截断。

首次GREEN67032有两个新增测试预期错误，保留日志，不修生产迎合测试：窄窗不包含blocked_reason标记，既有native分类应为0.2ms D而非IO；零秒双事件fixture被既有索引归到0.01..0.01，已无可测区间，不能由此新建0..0.01账号。修正新增断言保真实原生边界，旧针一条未改。最终定向3667正式exit0，`/tmp/hmc-timeline-delivery-green-final-20260921.jsonl`共11顶层26子项（tracequery1.050/agent1.828s），含原单一mint/双端单赋值/TID改名旧针及实际工具→成文新针。race94214、末版全仓92905另验，未提前签完。

新独立OPEN：`parse.go:1928`等入口使用FirstTs==0表示未设置，0秒开头的两事件trace会把首时刻替换成下一正值；stream_scan/search/sweep与merge也有类似候选。当前确认最小零起点fixture缺失，需单独审计有效timestamp计数/显式set信号、各入口/合并/缓存兼容，不能只改一处并宣称全支持。本片仅保证空原生结果不会被误补成完整账号；显式[0,x]老能力不变。

冻结后固定2并行×1选择C2真实全域清单＋Go一行写修复，按直接命中风险及跨模式保护排序；自动和完整人工分账，未命中新timeline入口也如实标注。分类causal/finite双轨、§94局部补齐、声明/观测pair等继续开放；父账13/79、66开放不变。

末版正式收据：实现`22b55f6d9`。race94214正式exit0，11顶层26子项（tracequery5.314/agent5.237s），日志`/tmp/hmc-timeline-delivery-public-race-20260921.jsonl`；全仓92905正式exit0，`/tmp/hmc-timeline-delivery-full-20260921.log`，87测试包、13无测试包、零FAIL，agent108.628/tool413.344/tracequery129.311s。干净构建16624正式exit0，revision22b55f6d9d29/buildTime2026-09-21T13:28:26Z。独立消费面只读审查确认account为支持覆盖事实、不自选根；tl.Window只作原生计量边界，真正full-artifact覆盖仍由独立producer排除span/pattern/recipe/窄窗。首次新增测试预期错误不抹，旧测试未改；完成的是公开清单投递窄子片，不是所有Trace回答或父项。

## 99. 22b55固定双例：机器2/2、完整人工1/2，目标分类与语义留债（2026-09-21）

[机器摘要](../../eval/parallel_selected_summary_hmc_timeline_wait_delivery_20260921.md)、[完整人工审计](../../eval/parallel_selected_summary_hmc_timeline_wait_delivery_20260921_manual_audit.md)：runner88217正式exit0，2并行×1，无第三次追绿。Go真实一行修复/原生TestGreet通过、原测试与seed tracked不改、未自动合主干，独立人工PASS；没有required/PTO，不能代销B2。

C2人工FAIL仍保：3段D侧IO和0.635ms均正确，不再把非IO D为零误说成无D，也未据小占比写影响可忽略。但用户明确主线程，analyzer仍把“进程整体不是子线程”判no_named_target，补齐因此按权限正确skip；不得借generic实体或原文扫描恢复目标。全工件问题只交付自行选的145ms窄窗状态账，末尾0.184ms Running未计；三段IO恰在窗内不代表全域状态齐备。final还把切入线程/行头称捕获者、把唤醒附近的单一blocked caller说成进入阻塞时的调用栈；具体磁盘/文件系统机理未由该标记单独证明。这些表述错误须与引擎正确量、当前新路未命中分账。

本轮4次trace_query为event_search＋bounded window_stats＋两次event_search，没有thread_timeline，故新入口以公开回归为据，不伪记现场命中。一次analyzer形状不一致拒绝后重发，成文零拒绝/patch；默认schema2空旁路存在，有限问题没有选根合同属合理空值，不强加因果投影。旧§97/此前人工FAIL不改写。

下一批ROI：先用同一真实工具结果跑明确合法causal/finite两套最终消息交接，并审实际主线程/进程/有限因果分类教学；只有确定缺口才改，不能因模型已查询root就扩大合同。§94业务局部补齐、声明/观测pair、两尺同卡、零时刻入口、P2修补指引和B2–B6继续留账，父账13/79、66开放。

## 100. 因果/有限三轨公开交接与误分类归因复核（2026-09-21，验证完成，不改生产）

§98–99实现及收据随`f3425ae50`已推送，session71633正式exit0、远端8702ea54f→f3425ae50，本地/远端相同且工作区干净后开验证片。不能因为人工答案未过就直接放宽有限过滤器。

新增真实TraceQuery(thread_timeline/wakeup_chain/root_cause_rank)→Bus→BuildAgentContext→完整Finalizer消息回归：同一原始15行fixture和仅重命名版，共causal_diagnosis/bounded_fact_set/bounded_effect_verdict三轨六格；生产维度归一化、当前请求quote、named_target/user_explicit和明确20ms窗先验，不借legacy nil-profile。causal用required causal_attribution且不依赖重复root intent/diagnostic旗标，finite分别保已选状态族及独立verdict。初版待用草稿缺target profile已在应用前纠正，不伪记生产RED。

首次实际运行77738正式exit0/agent1.424s，`/tmp/hmc-causal-finite-handoff-first-20260921.log`，1顶层/2父场景/6scope子项。因果轨完整保留11ms IO/fscache调用点、四节点wake路径以及三个独立1ms优先级候选席；有限轨只交付授权目标状态，不因预先采集root数据开根因报告。前后源结果/账本/范围/投影/模型文档字节不变。这是首次GREEN的审计回归，没有为造RED改生产；不声称该测试执行了emit_analysis、证明模型自然语言分类正确或已填旁路根因选择。

独立只读复核未证当前教学有相反合同：C2当轮109/292/314行已明确具名进程或线程均named，模型两次原发no_named，理由“非子线程”与规则不符；旧causal例身份正确named/app-100，误在把主要阻塞原因标target_effect_verdict，结构修复仅使其一致，并非系统把正确causal降级。存在性属于observed_value也已有明确教学。passive/unresolved条件措辞留观察，现有证据不够归因为系统错误；不再加专项同义说明，不撤H4有限未知条件能力，不扫原文重授目标或改结论。当前人工FAIL保留，但ROI转向已证确定性缺口。

再次对照参考`thread_sleep_summary.yaml:24`的tid与时间交集独立过滤，以及`sleep_ops.py:929–941`保tid/itid/pid和`is not None`窗判定：借鉴身份/范围分账，参考没有本仓用户授权协议，不能借原始trace/实体自动授目标。本片race和完整agent包正式收据随后更新；生产仍为22b55，不为测试片追加第三次live或重复花费同版双例。零起点统一入口审计、§94及其余开放项继续，13/79、66开放不变。

末版race59275正式exit0/agent4.160s（同1顶层2父场景6scope子项），`/tmp/hmc-causal-finite-handoff-race-20260921.log`；包含新针的完整agent包60235正式exit0，`/tmp/hmc-causal-finite-handoff-agent-20260921.log`。本片仅增加测试/审计，没有修改任何生产逻辑；§98全仓87包收据仍对应相同生产，不能称新测试也在那次旧全仓中执行。交接下游未复现合法因果范围丢11ms，保模型误分类/全文语义失败观察，不为这两例强加新门。

## 101. 零秒与未初始化分离：统一解析包络及显式零端点（2026-09-21，窄子片验收完成）

§100仅测试/文档随`912652214`已推送，session91944正式exit0；完整agent包60235用时71.285s。再从干净且远端一致状态开始本片，先修§98已证确定性包络缺口，不为模型未遵从教学继续堆提示。

主席复核参考`core/hiperf_converter.py:54–69`以min_ts/max_ts=None区分无数据与0秒，以及`core/preprocess/trace_data_cache.py:209–215`对SQL MIN(ts)使用is not None。仅借鉴presence与数值分离；其MAX(ts)+MAX(dur)并非精确最大结束时刻、空连接返回(0,0)，不移植算法或空值语义。

实施A：冷建、derive连续/非连续、mapped bundle、两类stream search、held-line scan和window_sweep共8处索引包络写点共用独立presence；不从Events/ParsedKnown猜，因为流式可不保Events、unknown和精确存储carrier也有已成功解析时间。保原观察域/过滤前后位置；derive回退重置局部presence，bundle仅观察已映射准入事件，不OR子工件时钟。保负mapped时间原排除，修0不宣称新负时钟支持。normalize仅有真实包络才回填，显式TimeStartSet/EndSet不代写；兼容旧synthetic正宽非负包络。另修sweep直接丢0事件、stream event_search用首个匹配时刻覆盖显式0起点，原matched/scan覆盖分域保留。ParserVersion统一升代，旧缓存不沿用错误包络。

engine公开RED2527正式exit1/tracequery0.813s，`/tmp/hmc-zero-timestamp-engine-red-20260921.log`，4顶层/4具名子例：冷暖实际Run、流式扫描、derive两路及unknown起点被覆盖；非编译失败。主席首次公开尝试93112正式exit1/agent1.030s，`/tmp/hmc-zero-origin-public-red-20260921.log`，但后续诊断发现测试错误要求full-artifact有限轨必须有explicit-window形principal_state，不算完整有效产品RED。95926/94592/94721均保留失败日志；94721诊断显示修后原始target_window_states的0..0.010、Running7ms/总量10ms已正确到达最终输入，只是既有合法载体为事实账而非新主卡。修正新增测试只核该真实事实行及共享口径/等待，不改生产以造新权限。engine有效RED收齐后才落生产，主席公开链后续首次GREEN另记，不伪称已证新的成文门故障。

新eval `trace_query_zero_origin_wait_account`要求从整份真实文本trace恢复状态/等待/覆盖范围，不向问题提供正确数值；原始六行即公开测试同一物理过程。冻结后与既有`trace_query_wakeup_causal_io_chain`按2并行×1：一例检验零起点真实数据供给，一例保明确20ms窗、链上IO/优先级/投影；已有Go上一批人工PASS不能说成本批live再覆盖写模式。公开回归/机器/人工分别记，不追第三例。

留B类独立未修：IndexEventLimitError及恢复建议的零起点presence、VSync census与其它聚合对象自身的FirstTs哨兵。对象/权限不同，不能机械替换并声称全仓零值已扫净。§94、原生pair/两尺同卡/B2–B6及父账13/79、66开放保留；本片末版race/全仓/人工未完成前不销账。

实际tool→final首次有效GREEN22056正式exit0/agent1.145s，`/tmp/hmc-zero-origin-public-final-green-20260921.log`，1顶层2视图4语言格；完整10ms账及IO一段2ms通过真实原生事实行交付，未制造显式窗或因果报告。包含causal/finite、windowless完整清单相邻保护的race45355正式exit0/agent5.950s，`/tmp/hmc-zero-origin-public-race-20260921.log`。上述早期新增测试载体错误仍保留，不改记RED→GREEN。

engine冻结后宽定向65604正式exit0/tracequery3.864s，160顶层、336个含子RUN，`/tmp/hmc-zero-timestamp-engine-focused-20260921.log`。新增10顶层覆盖presence独立于known/retained、真实冷暖/换代/derive、held/stream/sweep及bundle仿射/负mapped边界；旧5处版本断言只迁v43，旧零宽fixture改为真正同刻且保所有原断言。早期93017/7903等失败含新增测试对held字节上限、物理空源准入、sweep既有50ms下限及cluster API视图的错误前提，仅修测试，不伪记生产问题。末版engine race59853、全仓45715待正式退出。

超时/活跃流定向62239正式exit0/llm4.481s，`/tmp/hmc-zero-origin-stream-guards-20260921.log`，8项保600/300/600秒、4ms连续部分帧、隐式推理/tool-call/可见内容进展及调用方取消/更短期限。不改超时代码，也不因本片数据问题提前降级。

冻结engine race59853正式exit0/tracequery41.520s，同160顶层/336含子RUN，`/tmp/hmc-zero-timestamp-engine-race-20260921.log`，零FAIL/竞态。独立末审七个生产文件及新公开测试/case无阻塞：presence不借子工件、normalize不授用户范围/因果、cache升代完整。边界另留：本片stream修的是显式0的结果端点不再被首尾匹配行覆盖；底层既有time gate中time_end=0的零宽筛选语义未全面修复，不能外推。新agent public执行实际工具与成文消息构建，未执行真实模型；live待单列。

实现已提交`0b9e890f2`，干净构建79698正式exit0（revision0b9e890f29f1/buildTime2026-09-21T13:58:42Z）。全仓45715日志已完整输出87测试包、13无测试包、零FAIL（agent101.318/tool400.227/tracequery123.739s），跨后续用户消息后原session不可恢复，未伪造其退出收据；同一冻结代码88873复核正式退出中。固定runner81417亦跨消息丢失session，但完整摘要及日志记录2/2完成、无timeout/launchfail；不为恢复shell收据重跑第三例。

末版全仓复核88873现已正式exit0，`/tmp/hmc-zero-origin-full-receipt-20260921.log`共87测试包、13无测试包、零FAIL，tool391.691s。公开引擎RED/GREEN、实际tool→final、race、独立末审、干净构建及下节真实零起点命中齐备，窄索引/交付子片验收完成；两例完整人工FAIL仍开放，不销叙述、恢复建议或父任务。

## 102. 0b9e固定双例机器2/2、完整人工0/2：计量闭环与语义债分开（2026-09-21）

[机器摘要](../../eval/parallel_selected_summary_hmc_zero_origin_20260921.md)、[逐例人工审计](../../eval/parallel_selected_summary_hmc_zero_origin_20260921_manual_audit.md)。2并行×1，zero154s、causal258s为runner整例耗时，causal原生wall256s不混用。两例成文硬拒绝均0，各1次软修订；未动600/300/600秒或活跃流保护。

zero实际命中无窗timeline，系统按typed全工件请求补window_stats，二者完整0..0.010；主文7ms运行+1ms就绪+2ms IO、一次等待0.002..0.004、总10ms正确，不再漏首2ms。这足以给索引/投递修复记真实命中；但模型重复表、内部字段、把完整测量零说成清单未匹配、未验证Binder写零仍留债。有限问题schema2空旁路实际产生，未授根因权时空值合理。

causal本次分类终于保named目标、显式20ms窗及required causal，四query和终答输入保四节点链、11ms IO及三条1ms优先级供给候选，投影及4项root-causes旁路齐备。主文/旁路model-owned描述仍有反向唤醒、三边称四跳、相邻IO/Runnable区间误作重叠、Binder无关联过度排除等错误；原生结构与模型语义分账，不加原文硬门或系统改写其结论。已发布Harmony优先级口径，不因Linux规则把20/CFS和52/RT误判为本轮错误。

下一批ROI重新排序（仍未交付）：

- [ ] 系统owned等待计数/数值标签统一：wait_coverage计数把非IO-D桶写成裸D，sleep_inventory及root-cause组成也有同类raw值；统一限定但不改四态D+IO已fold总量、不改数值/IO-S语义/模型正文。
- [ ] 系统owned归因/优化口径：图例与清单有“已证可消除量”等无条件标签，使有效归因看似已验证可消收益。按现有计量类别统一限定优化预算/潜力，保模型自定结论、原数值/排序及链上优化维度；已明确ideal-baseline modeled headroom的分支不改成测量事实。
- [ ] 容量恢复B：合法零起点被FirstTs>0漏具体建议，正起点建议的LastTs又包含预算触发但未保留事件；照抄该末端可再次拒绝。优先提供真实可执行的局部探测/流式出口，不称失败索引已完整覆盖，不简单改>=0或扩大cap。
- [ ] P2 typed member_set承载提示与重复表；explicit time_end=0旧筛选边界、VSync/其它独立对象零值哨兵继续开放。§94业务局部补齐、原生pair/两尺同卡/B2–B6等父账仍13/79交付、66开放，不拿本片销父项。

## 103. 等待计数与原始状态分量的显示标签统一（2026-09-21，窄子片验收完成）

§101–102代码与收据随`9534d71c6`已推送，session47768正式exit0，本地/远端相等后开始本片。前批完整人工0/2不改签；当前只处理已经确定的系统展示接缝，不改写模型叙述。

再次对照参考`core/preprocess/sleep_ops.py:225–255`：窗口交集后分别计Running、R、S与所有原生D，参考未设本仓排他IO子桶。因此不能把本仓`DStateOccurrences`（仅d_sleep）显示成物理D总次数，也不能把S侧IO标记并入D。实际producer、计数authority、root-cause raw分量链路均已审计，只有尚未fold的非IO-D分量改限定词；四态D+IO折叠及逐段真实D状态词保留。

主席公开RED17576正式exit1/tool1.819s，`/tmp/hmc-wait-bucket-public-red-20260921.log`：实际TraceQuery→当前typed Bus→公开EmitAnswerDocument→最终render→公开no-op patch，1顶层5物理场景×中英共10格。覆盖零起点全工件timeline、明确窗口D+IO、D未标记、S+IO及真实C2三段0.635ms。全部原生次数/墙钟/窗口、模型正文、无额外因果授权及修补幂等均先通过，只因系统计数缺“非IO”限定失败；此前只测到finalizer输入的针未覆盖这个发射后附录。

实施边界：共享词源下沉至无上层依赖的tracefence，tool保持兼容包装；等待附录计数/引言、sleep库存、raw/actual快照和根因组成复用同词。不加prompt堆叠、不改JSON/schema/准入/统计/排名/根因资格，模型原文逐字保留。独立unit矩阵与末版公开GREEN/race/全仓待验；系统优化预算标签和容量恢复另分批，不混进此次窄修复。

独立同类RED61617正式exit1，留档复跑69617正式exit1，`/tmp/hmc-wait-bucket-labels-red-20260921.log`；新增测试编译通过，仅计数/引言/库存/raw与actual快照/root-cause组成标签失败，物理D折叠与S侧IO排除先通过。修复后focused23440正式exit0（tool4.115/tracefinding1.917/tracefence2.392s）。旧文本pin仅三文件的对应标签精确迁移，数值、类别、排序、所有权断言不变。中文新增引言用“分别统计/非IO部分/可中断睡眠统计”，不增加“桶/sleep”内部术语。

主席公开首次GREEN18549正式exit0/tool1.759s，末版race44083正式exit0/tool8.426s，日志`/tmp/hmc-wait-bucket-public-{green,race}-20260921.log`；5场景×2语言原生数据、发射附录、修补幂等全部通过。独立只读末审无阻塞，定向50833正式exit0；全仓18996和独立宽race1162继续等待，不提前签全量完成。冻结后固定G1英文整份工件IO清单＋H2中文明确窗口dma_fence纯D清单，2并行×1，检验语言/IO分支差异；不改oracle，不拿旧Go写模式PASS冒充本片live覆盖。

末版收据现已齐：代码`4c47d56a1`；独立宽race1162正式exit0（tool23.233/tracefinding2.126/tracefence1.622s），统一全仓18996正式exit0，`/tmp/hmc-wait-bucket-full-20260921.log`为87测试包、13无测试包、零FAIL（tool452.220s）。干净构建92765正式exit0，revision4c47d56a1ec4/buildTime2026-09-22T01:30:15Z。活跃流/默认值55305正式exit0，8项/llm4.383s，600/300/600秒和4ms部分帧保活不改。本节仅完成系统标签子片；以下真实回答错误仍开放。

## 104. 归因计量与已验证优化收益分离（已确认，待独立实施）

§102系统“11ms可消”不是仅模型波动。主标题/完整与精简图例/方向标题/动作说明沿用“已证链上可消除量”，`traceQueryRootCauseClosedMatrixContract`也在Description和Parameters双面教“largest single PROVEN on-chain eliminable contribution”。只改显示会留系统自身相反教学，必须同批统一；不改选举、数值、链上凭证或“主根因”既裁名称，不以优化收益未验证撤销已经成立的关系。

主审直接复核参考`core/batch/rootcause/evidence.py:46–79`和`sleep_ops.py:225–255`：时间组成、未知/退化事实可借鉴，不等于实施收益；参考optimization-advisor仅有“待评审”设计，不能记作已实现能力。不得搬其线程名排除或比例启发式作为根因硬门。

- [ ] 单源口径：在现有冠名词源与规则矩阵中将量解释为“按既定规则估算的优化潜力，收益需实施后复测”；保原始占用/有效归因两轴，不系统改写模型正文。
- [ ] 完整消费者：tree标题/两套图例/方向与边前语义，elim标题/顺序/预览，nextstep非频率动作，runtime排序与语义份额表头一致更新。频率分支已有理想基准限定、root_cause_report已有“不是可直接消除的承诺”保留。
- [ ] 下界准确性：缺失频率按零计的既定理想算力模型内下界仍保留，只去除“真实可消除量只多不少”的实效承诺；不统改成上界，不丢核类/频率来源和跨方向不可相加条件。
- [ ] 公开验收：实际query→emit→render→patch，中英、有限/因果、11ms IO+三1ms席、语义前边份与频率缺失；保全部值/顺序/证据/窗口/旁路/模型正文。主标题单源、所有权、图例、数值守恒与宽度旧针不能降。

退役`proseHeadlineElimFindings`及历史模型主标题替换函数不重新接入，不以模型原文扫描修补此事。此节仅落实可执行任务与证据，不记新增交付；容量恢复、§94及父账66项继续开放。

## 105. 4c47d5固定双例与逐段物理状态载体缺失（2026-09-21）

[机器摘要](../../eval/parallel_selected_summary_hmc_wait_bucket_20260921.md)、[完整人工审计](../../eval/parallel_selected_summary_hmc_wait_bucket_20260921_manual_audit.md)：runner8714正式exit0，2并行×1，机器1/2、完整人工0/2。G1 159秒、H2 110秒，两例finalize各一次、成文拒绝/patch均0；这些计数不代表此前analyzer没有修复。没有追跑第三例，没有修改case oracle。

G1整份工件窗口34579.450627..34579.595184、三段0.138/0.147/0.350ms合计0.635ms全部正确，新的系统附录明确非IO D为0、IO为3、S侧IO为0，保D来源。但模型正文仍说“未进入D”“三段均S”。最终输入已含正确双层口径与三段清单，因此本轮模型确实未遵从；不以补充附录抵销正文错误。又独立确认真实payload的三个timeline.intervals均保`prev_state_raw=D`，转`target_window_states.wait_occurrences`时字段丢失，后续观察账本/逐段authority/最终principal行也无原始状态。这是可直接修复的事实交付缺口，不应再只叠教学。

H2的11闭合D段/36.757ms、12条内核原因记录/Σdelay39.157ms、完整sleep库存29段/155.343ms分别保留，不是同一计量。原生状态231.834ms＋未归账1.356ms=233.190ms，系统补齐和显式窗口正确。机器FAIL仅因旧regex不接受“内核调用点/符号=”词面，留原结果；独立人工仍FAIL：模型将调用点和ELF模块推成确定资源/进程身份，将未独立闭合验证的Binder零值当排除Binder证明，并泄漏内部键。系统已有边界教学，不能据此断言缺数字或再造正文扫描门。

两例均是有限事实问题，未强添因果投影、必选schema2空旁路及`trace_root_cause_contract_not_active`正确，不作为漏根因。H2老系统附录的opaque工件标识及内部状态词仍属独立读者词汇债。

- [x] 优先补逐段原生状态事实：§106已完成`PrevStateRaw`可选字段贯通，不从io_wait标签反推D、不从用户/答案原文推断；缺失保未知、历史兼容。全部计数/时长/窗口/根因资格不改，最终模型仍负责结论。
- [x] §106真实producer→观察账本→最终模型输入→发射附录/patch公开红绿及race已完成，覆盖D变体、S侧IO、无原始状态及真实C2；无新schema必填或硬门。仅字段交付验收，不回写本节两例完整人工FAIL。
- [ ] H2调用点/资源/持有者与“未验证≠排除”人审问题继续留债；G1 event_search中行发射线程与payload主体的呈现歧义另审计，尚不判引擎错误、不擅改过滤。
- [ ] 之后按ROI推进§104优化潜力口径、§94局部补齐和容量恢复；原人工FAIL及B2–B6不回写，父账仍13/79交付、66开放。

## 106. 逐段原始调度状态事实贯通（2026-09-21，窄子片验收完成）

§103标签代码和§105双例审计随`6122cc281`已推送main，session92262正式exit0，本地/远端相等且tracked干净后开始本片。先处理真实payload证实的已有事实丢失，未把模型未遵从改判为纯系统缺证，也不以再叠教学代替交付。

原生Interval.PrevStateRaw→TargetWindowStateOccurrence可选字段→注册typed leaf note/compact尾部→typed authority→最终principal行/系统逐段显示。旧CanonicalLine、leaf Object及计量指纹保持原身份，不因可选元数据改次数/时长/范围/准入；新旧重复可补已知字段，冲突只抑制该可选显示，同ID的原DeepEqual安全ID校验不降。原始缺失不从统计类别推断，S侧IO保S，唤醒后runnable不因携带历史prev_state进入等待清单。

主审再次直接核对参考`core/preprocess/sleep_ops.py:225–255`，其状态计量仍来自原始state及半开窗口交集，可借鉴保源状态的分层，而非复制其状态桶根因算法。本仓既有IO分区、四态折叠、链上凭证和语义工作事实均不改。

公开有效RED：主席17263正式exit1/tool1.920s，6物理场景×中英12格，包含真实C2、零起点、D|K、S侧IO；所有数值/时间窗/模型正文/patch幂等先过，只有逐行原始状态缺失。11行超compact前8行的补充RED44701正式exit1/tool1.188s。agent公开79583正式exit1/agent2.229s，2来源×新旧记录×中英8格：新字段4格仅缺principal原始状态红，legacy4格绿，10段混合D/S及真实三段总量先过。核心独立RED21084正式exit1，tracequery/types/tool编译成功，旧Object/统计/准入针先过。生产/末版验证尚未终签。

固定下一双例：G1真实整份工件旧人工FAIL＋显式20ms窗`trace_query_wakeup_causal_io_chain`，2并行×1；前者验证字段实际交付，后者保四节点因果投影、IO11ms、三段1ms调度等待及优先级线索，不把背景升主因。模型结果与窄字段交付分别分账，不追第三例；§104系统收益措辞仍另批。

生产已冻结：compact必须通过原有完整envelope且与全部原计量行相等，才能合并可选字段；不完整8行preview不影响完整11行leaf。冲突独立传播的私有收据严格绑定同ID/来源/主体/精确窗口/结果，不能用陈腐或相近窗口代入，也不增加SourceRecordIDs；按ID索引避免新增不同query重复组平方扫描。独立末审61323正式exit0（types0.676/tracequery1.046/tracefence0.544s），无生产阻碍。

另修本批新增事实面对的旧相反教学：原文笼统禁“机器状态码”会连D/S标准状态一并禁止；独立RED66100正式exit1/agent1.758s，其余原始字段/旧记录/两轴/完整行均先通过。现只禁内部字段名和统计分类枚举，允许D/S保留并解释；唯一旧错误literal pin精确迁移，未改数字/完整性断言。agent公开GREEN48032正式exit0/1.536s；主公开GREEN32688正式exit0/tool1.612s，7物理场景×2语言。核心末版focused48073正式exit0（tracequery0.929/types1.026/tool1.563s），registry新增仅一个soft-consumer登记及对应fixture，不新增硬准入。末版全仓34894、公开race39637、核心race22370和agent末版race41464待正式收据，不提前签完成。

末版收据齐备：代码`f31f71d53`；全仓34894正式exit0，`/tmp/hmc-wait-raw-full-20260921.log`为87测试包、13无测试包、零FAIL。公开race39637正式exit0/tool13.838s，agent末版race41464正式exit0/22.747s；核心最终race28090正式exit0（tracequery5.447/types2.800/tool3.904s），最终focused5506正式exit0。独立安全末审无阻塞。干净构建10641正式exit0，revision f31f71d5355d/buildTime2026-09-22T01:59:03Z；活跃流/默认值73128正式exit0/llm4.388s，600/300/600秒与4ms部分帧保活不改。窄原生字段事实交付完成，不倒签此前完整人工FAIL。

## 107. f31固定双例：机器2/2，完整人工0/2；空投影回退缺陷（2026-09-21）

[机器摘要](../../eval/parallel_selected_summary_hmc_wait_raw_state_20260921.md)、[逐例完整人工审计](../../eval/parallel_selected_summary_hmc_wait_raw_state_20260921_manual_audit.md)。runner4607正式exit0，2并行×1，G1 84秒、causal208秒，不追第三例。G1零成文拒绝/patch；causal一次传输标记参数拒绝后完整JSON恢复、一次surface_role patch，不是本片字段合同拒绝。

G1正文已正确消费三段原始D+IO和0.635ms，新详细reader真实命中；但模型仍把内核调用点当确定机制，完整FAIL。principal occurrence摘要和系统状态附录本轮未出现，只以公开确定性回归验这些出口，不冒称live触达；state摘要缺席具体路径未另定位。causal保20ms显式窗、四节点链、11ms IO、三1ms调度/优先级候选、系统投影与schema2旁路，但调用点越权解释/11ms造成整个20ms描述、系统未验证收益承诺继续FAIL。有限G1的schema2空旁路理由正确，不补无权根因。

新确认高ROI系统gap：`projectTypedTraceAnswerAuthority`已按typed范围与native来源把模型复述过滤为空；`renderAnswerDocAggregateFacts`尊重该空结果，而`preEmitStableAggregateFacts`与`buildAnswerDocPreEmitContext`却以`len>0`区分存在，错误回退Mutable原始聚合。G1 explorer918行的19.671ms（首/第三D入口差，不是等待总量）由此在1742行软建议复活，要求全部聚合展示。最终答案仍采用正确0.635ms；本轮没有因该建议拒绝或重试，准确记录为软合同自冲突而非硬门。

- [x] §108窄批验收完成：存在answer plan就尊重其聚合投影，包括空；只有无plan才保历史Mutable兼容。直接/缓存/成文prompt/公开emit一致性、混合源代码事实、非trace空投影均有回归；原始审计facts、窗口、native数值和模型正文不改，不扫描算式/原文、不新增门。
- [ ] 之后§104系统收益措辞、§94局部补齐与容量恢复继续按ROI推进；JSON传输污染恢复、调用点语义越权、读者内部词汇和原始occupancy重复另留账。父账13/79交付、66开放与旧FAIL不变。

## 108. 明确空投影与缺失投影分离（2026-09-21，窄子片验收完成）

§106代码/§107人工审计随`4bcacf783`已推main，session82517正式exit0；首次14057因SSH连接关闭失败，未丢提交，重试后本地/远端一致。先收住战果后处理本片，不把前批两份人工FAIL改签。

主审直接再读参考`core/batch/rootcause/evidence.py:46–79`：从结构化指标提取等待/供给/负载及未知项，提供原生计量参考；未发现本仓模型聚合→答案投影→回退的同构路径，不照搬其比例阈值或关键词类别门。当前gap的修复依据是本仓已存在的typed来源投影，而不是判断19.671这个值或模型原文算式。

两处消费者已确认：普通`preEmitStableAggregateFacts`和请求内缓存`stableAggregateFactsForCheck`以非空列表判断投影存在，复活被排除的原始输入。finalizer的aggregate prompt、principal contract以及orchestrator只在plan缺失回退，已经正确。本批只统一两处存在性判断；保持原投影规则、无plan历史兼容、缓存每代只构建一次以及全部原始审计记录。没有新增schema字段、必填教学、Trace特判或正文校验门。

主席非Trace边界RED5251正式exit1/tool1.183s，`/tmp/hmc-empty-projection-boundary-red-20260921.log`。真实no_directed_path与多顶层日志peer-error投影全部排除时，普通/缓存读取及路径roster会复活；混合保独立scalar、无plan旧handoff先通过。另测nil/[]皆权威、不同emit/patch代次重新编译，不依赖错误数值。agent公开保护8947首次即绿/0.994s，实际TraceQuery→BuildAgentContext→最终instruction，empty/mixed-source×双语4格；原生1ms/D及独立source值7不丢，Mutable/TurnA/模型正文不变，不把此初绿写成RED。

本批固定双例计划：G1真实全工件等待（本次发现的空聚合软合同）＋`hilog_mixed_arkts_cangjie`双语言日志事实（异构非Trace空投影风险），2并行×1。先按影响范围与可复现性排序，源码无路径由确定性回归覆盖；写模式既有公开不变式/全仓回归继续保，前批Go真实apply通过不能代表B2–B6完成。两例均逐读最终答案和过程，是否真实命中空投影分别记，不靠机器PASS补旧账。

公共RED73006正式exit1/tool1.380s，`/tmp/hmc-empty-projection-public-red-20260921.log`：4场景×双语8格真实query→emit→render→patch，D+IO/S+IO的全空4格仅普通/缓存与实际emit/patch软提示复活失败；混合独立source和无native对照4格先绿，模型正文、native1ms/窗口、ledger及raw facts保持针先通过。生产只去掉两处`len>0`附加条件，缓存仍保原排除标志、一次构建，未增加代码行数或降低行数限制。

首轮focused8180和race56630各因新增代次测试的错误前提FAIL：由两条日志改成一条仍有直接runtime证据，既有projector应继续排除无支持复述。仅将测试第二代改为真实partial-dispatch清理路径`SetLogTriage(nil)`，不改生产规则；旧缓存仍空，新代次恢复兼容。最终focused15834正式exit0/tool2.709s；agent53748正式exit0/1.232s；独立末审87449正式exit0（tool1.609/agent2.808s），无阻塞。统一full43341保首轮记录，末版完整复验83670、末版race7031待正式收据。未提前签全量通过。

最终收据：代码`c5514cc56`。首轮全仓43341正式exit1/tool537.525s，仅上述新增测试前提错误，未改记通过；修正后全仓83670正式exit0，`/tmp/hmc-empty-projection-final-full-20260921.log`为87测试包、13无测试包、零FAIL（agent124.822/tool444.747s）。末版race7031正式exit0（tool15.409/agent3.048s），同时覆盖新空投影公开出口、旧缓存/scalar/cross-target及前片原始状态公开回归。独立末审无阻塞，干净构建48876正式exit0，revision c5514cc56787/buildTime2026-09-22T02:35:35Z。8项精确默认值/保活保护86902正式exit0/llm9.060s，保600/300/600秒及4ms连续流；短调用方期限仍有效，不改超时或降级代码。两处生产条件修改验收完成，不把模型叙述和父项一并销账。

## 109. c551固定双例：机器1/2、完整人工1/2；空投影真实命中与上下文债（2026-09-21）

[机器摘要](../../eval/parallel_selected_summary_hmc_empty_projection_20260921.md)、[逐例人工审计](../../eval/parallel_selected_summary_hmc_empty_projection_20260921_manual_audit.md)。runner2083正式exit0，严格2并行×1，G1 85秒、HiLog207秒；不追第三例、不改旧oracle。成文拒绝/patch/聚合软建议均0，不代表前段JSON和分析模型没有重试。

G1机器PASS但完整人工FAIL。整份工件34579.450627..34579.595184、三段原始D+IO和0.635ms正确；explorer977行仍提交漏第三段的“两次D”成员集，typed投影将其及另一复述集合过滤为空。最终输入未重播该聚合，也未再出现“全部聚合必须成文”的旧软建议，故本片空投影有真实命中。schema2空旁路的`trace_root_cause_contract_not_active`符合有限事实问题，不是投影丢失。模型仍将sync_buffer_read_wi调用点及sysmgr.elf推成确定的同步缓冲读取/系统管理器机制，又把iowait=0的D说成“纯磁盘/文件类阻塞”，均缺证明。

主审与独立复核按完整finalizer输入追源：explorer的机制性自由总结未重播，原生producer只给调用点和计量，输入已明确opaque符号边界及非IO D为零不能排除所有存储IO。具体错误机制译文由最终模型引入；不归咎于已过滤聚合复活，也不凭单次判为随机波动。另确认通用summary-only教学仍要求机制细节/跨文件关系，而本例typed有限事实卡要求只答所问，存在过宽教学张力；尚不能证明它就是本例错误的唯一原因，另批审计，不扫正文作硬门。

HiLog机器FAIL、完整人工PASS（仅本例有限问题）：ArkTS/Cangjie四帧、文件/行号及index=5,size=3越界信息准确，未虚造当前仓源码权威或跨栈确定根因。机器失败为英文contains碎片`of`/`bounds`未出现，中文“数组下标越界”已保含义，原FAIL保留。两组非空supporting运行时成员集在最终输入仍保留，member_notes被除；这是合法非空保留对照，不冒称命中非Trace空投影修复。“各自独立”的关系措辞偏强、模型派生异常标签被要求逐字呈现仍留读者/来源精度债，不影响此次四帧任务判定。

过程审计：HiLog triager首轮errors字符串内含坏JSON，被拒；第二轮数组可解码但非逐字evidence及无因果marker，被正确拒；第三轮保2条peer事件/4帧成功。分析模型在54362上下文token下活跃输出102秒后触发周期重复熔断，第二轮成功；不是4ms/无答案静默超时，不能用延长watchdog掩盖重复生成。最大上下文55882 token/28%。`analyzer_terminal_emit_only`的3分钟phase budget不等于活跃流累计deadline，现有公开保护证明短phase预算不截断有进展SSE。

- [ ] §104系统优化潜力措辞、§94局部补齐、容量恢复B与B2–B6继续开放，优先已证可复现系统缺陷。
- [x] 通用summary-only机制详述教学与typed有限事实范围的精度审计：§110完成载体/来源教学窄修，所需机制/链/业务证据保留；§111完整人工FAIL不由此销账。
- [ ] 日志原始异常类型与模型派生描述标签区分：原工件只有Error/panic，NativeBridgeError/CangjiePanic由triager生成，最终教学却要求逐字保留；应保来源层级，不增原文关键词硬门。另记该case旧注释允许无marker单链叙述的eval维护债。
- [ ] JSON教学/修复成本与分析重复输出继续观察，不能以最终答案PASS销前段成本；无新证据前不为单样例增加专门规则。

本片不改变窗口/根因资格/旁路/模型正文，机器与人工结论分账；父账13/79交付、66开放不变。

## 110. 答案载体不自动扩张问题范围或证据来源（2026-09-21，窄子片验收完成）

§108实现`c5514cc56`及§109收据`bef54f3ce`已本地提交；推送84966/19686先后因SSH443连接关闭失败，22端口只读核验87162也失败，提交未丢失，尚不记已推送。本片仍按独立小批开展，不混入未修的系统优化收益措辞或日志派生标签来源。

下一ROI选择依据：§109真实最终system消息同时收到两条无条件summary机制教学，而用户侧明确有限事实范围。`internal/skill/defaults.go`的Workflow与OutputFormat都将“summary是唯一principal”当成需要机制/代码/跨文件解释，属于通用承载形式与任务语义耦合，不是某个caller或Trace类型特判。现有更精确的scope/facets已经给出边界，修正教学即可，不改分类器、准入、补齐或模型原文。

主审直接核参考`config/skills/ad_hoc_exploration.yaml:191–207,235–245`、`freq_distribution.yaml:44–74`及`core/skill_executor.py:645–691,1671–1695`：最终解读绑定当前问题、该步指标和实际引用的数据，可借鉴按问题组织解释。参考仍有D桶直接猜机理、固定阈值、强制全量窗口及芯片频率推断等限制，不移植其根因/路由规则。本仓明确窗、typed链上授权与用户需要仍优先。

真实registry→BuildPromptContext→ToMessages的system消息RED71356正式exit1，`/tmp/codrax-summary-scope-teaching-red-20260921.log`：双语×有限Trace/多栈日志/代码机制/有限事实加源码维度/有限影响/完整因果共12格，两个独立教学出口共24条预期失败；JSON/图所有权/机制解释/因果边界保护先过。只测用户侧最终instruction会漏system层冲突，故两层分别验。

生产仅替换两段文案：summary只是答案承载，解释深度服从resolved scope和required facets；所需机制/代码/跨文件关系仍在有证据时充分展开。有限事实和有界影响保请求事实、计量范围、来源、不确定性及判定理由，不因summary额外要求根因调查。独立源码维度不被finite取消，完整因果/图要求仍保留。没有增加schema、程序分流、关键词扫描、硬门或模型答案改写。

skill整包33345正式exit0/0.862s，双面新语义定向race80412正式exit0/1.799s，原RED保留。实际原生query→完整final消息的agent保护、独立末审、末版全仓及冻结双例另验，未提前签完成。计划G1有限真实全域等待＋明确20ms因果链，2并行×1；同一有限问题是否仍错写机理与完整因果是否保深度分别审计，不追第三例。

同批审计扩面（冻结前）：不能只改summary而留下同一system里的反向总则。主审直接核`compile_enumeration.go:41–50`准许external_observation枚举，generic:129–135可承载普通decision，dynamic_schema:430–458按真实投影保合法claim forms，`TestB1691PublicExplicitRuntimeScalarRemainsScalar`公开保显式direct-waker标量。相反，旧教学的scalar输出强制call graph/config lineage/file:line、普通decision只教代码guard/definition、外部枚举建议drop/换summary、hop先要求repo位置再允许外部帧、全局Prose voice将所有正文当代码机制。将这些同源承载/来源混淆纳入同一教学片，先加各出口RED，不新增任何枚举、运行时门或自动来源推断。有限语义/外部证据不取消独立源码义务，也不把代码所需证据降成无引用。

前批推送收据补齐：HTTPS只读57415确认远端仍4bcacf783，写97303因缺可用口令失败，未改认证或remote配置；稍后原SSH443重试28712正式exit0，远端4bcacf783→bef54f3ce。已完成§108/109提交现均在main，本片仍独立未冻结。

扩面RED32828正式exit1，`/tmp/codrax-runtime-carrier-teaching-red-20260921.log`：7个独立system出口、10个旧句断言×中英×8种合法runtime/source载体共160条预期失败，旧来源/JSON/图保护55911正式exit0。修后新针及skill/context定向race60391正式exit0（2.067/1.949s）。首次整包92562失败保留：一个旧V2针无条件要求decision.text承载，与当前typed verdict不一致，精准迁移到schema-selected verdict/rationale表述，原禁退役字段全部不动；另一个多证据稳定ID旧针仍合理，生产恢复原“selected stable IDs on that item”教学，不改该针。最终skill/context整包84188正式exit0（0.994/1.249s）。

用户侧枚举Rationale同源缺陷亦实际可达：`renderAnswerDocBlockRequirement`直接发布“每项authoritative file:line”，随后又允许external_observation。真实BuildAgentContext→BuildInitialInstruction的中英×源码/外部四格RED58220正式exit1/agent1.145s，`/tmp/hmc-enumeration-origin-user-red-20260921.log`；只有反向教学失败，required成员/形式/事实不变先过。修正Rationale为各自证据与来源，源码仍需grounded位置、外部保工件来源而非伪造repo位置，形状/枚举/门均不动。

原生交接采用分立而非伪合并验收：既有真实三轨查询/改名fixture＋空投影实际成文输入两项race82100正式exit0/agent4.212s，保11ms/三条1ms和finite不授因果；新skill矩阵单独证明实际system教学，新增用户侧Rationale针单独证明该动态出口。未新增重复五组原生fixture，不宣称一条测试串过所有system/user/native入口。公开工具标量及query→emit/render/patch76490正式exit0。完整全仓17085已启动，待正式退出。

末版实现`2c0362e62`，独立生产/测试只读审计无阻塞；用户侧Rationale定向95905正式exit0/agent1.320s、race45340正式exit0/2.957s（`/tmp/hmc-enumeration-origin-user-{green,race}-20260921.log`）。完整全仓17085现正式exit0，`/tmp/hmc-carrier-scope-full-20260921.log`为87测试包、13无测试包、零FAIL（tool429.169s）。两条新增Rationale正向词义pin在全仓启动后加入，已由上述末版定向/race覆盖，不冒称旧全仓同时包含它们。活跃流/默认值47683正式exit0/llm4.311s涵盖7项，另补更短调用方期限定向正式exit0，合计8项；600/300/600秒及4ms活跃部分帧保护不改。

干净构建46513正式exit0，revision`2c0362e6295b`/buildTime`2026-09-22T03:50:29Z`；以下冻结双例机器2/2、完整人工0/2。新教学确实进入两个真实system及动态user出口，窄教学矛盾可销；不是模型语义已稳定，更不是父差距完成。

## 111. 2c036固定双例机器2/2、人工0/2：新增关系摘要和计量分量交付缺口（2026-09-21）

[机器摘要](../../eval/parallel_selected_summary_hmc_carrier_scope_20260921.md)、[完整人工审计](../../eval/parallel_selected_summary_hmc_carrier_scope_20260921_manual_audit.md)。runner47633正式exit0，2并行×1，G1 78秒、causal 168秒；无第三次追跑、无oracle修改。新代码只修教学，不改变原数值/准入/窗口/链上资格/图补齐或模型正文。

G1完整工件34579.450627..34579.595184，三段原始D+IO为0.138/0.147/0.350ms、合计0.635ms，实际交付/正文/附录正确。原始模型2.737ms聚合未由已修的pre-emit/最终事实账复活；有限任务仍产schema2空旁路、原因`trace_root_cause_contract_not_active`合理。但正文由调用点符号推成“同步缓冲区读取完成”和具体模块机理，又以“纯不可中断D”混物理状态与计量分区，完整人工FAIL；原始事实交付PASS不抵销主文越界。

causal明确2.000..2.020秒，app S20ms、四节点threadpool→network→cookie→app与唤醒2.016/2.018/2.020、CPU4→3→2→1、链上IO11ms及三条1ms低优先级调度供给候选全保。Trace因果投影存在，旁路available并保模型选择的一项IO根因，未强填四项。正文仍将cookie的17ms睡眠与另一条1ms runnable归因比较，推断“大部分睡眠未计入app阻塞责任”；“阻塞完全通过依赖链间接传导”和旁路“直接阻塞源”也超出未闭全原因/无直接阻塞授权。系统§104“已证最大可消”“11ms可消”继续出现，完整人工FAIL。调用点只作下一步排查且承认资源未知这部分有改善，不能连带签全部通过。

过程：两例成文首次均接受、硬拒绝0；G1 patch0，causal patch1只补漏schema_version的可选旁路，5个正文块完全不动，最后schema2一项合法。causal分析器曾因bounded_fact_set/root_cause结构矛盾合理拒绝一次后接受；不要以runner阶段计数1误称整个分析过程零拒绝。没有成文重试风暴或超时降级，日志中的4ms为完成后的环境构建耗时，不是流式等待上限。

新确认并按ROI排队（均未修，不能归为模型波动）：

- [ ] **P1 关系摘要消费投影与原始值语法分离**：G1实际最终上下文日志1585把`34579.451840 (line 118)`拆成`34579 -> 451840 (line 118)`。`context/builder.go::relationDossierAggregateFacts`仍读原始stable/TurnA而忽略AnswerSurfacePlan，`relationDossierAggregateMemberExamples`又用`types/answer_aggregate_fact.go::aggregateCompactDotRelationParts`将纯数字小数当限定成员。是独立advisory上下文污染，不是已证硬拒绝或图被改写，也不能断言造成调用点机理错误。需保最终空/非空投影、探索/no-plan兼容及原始审计，数字/原生值不得变成伪源码关系，真实限定源码成员/显式关系仍保；全消费者核对后公开先红后绿。§108只修其明确两处consumer，不外推为全系统投影已统一。
- [ ] **P1 原始占用与归因的分量身份**：causal实际日志2486–2488的state_value_authority把`s_sleep measured=17ms`与runnable席`effective=1ms`放同一行，threadpool也把IO11ms与runnable归因1ms并列；虽写distinct，却未显示归因对应分量。需保原始占用和归因各自状态/范围/行身份，不能仅重复“两轴不同”，不能改变任何值/排序或系统改写模型结论。现有直接阻塞/不穷尽教学已到场，二者错误因果关系不作确定断言。
- [ ] **P1 §104 归因量≠已验证收益**，及§94业务局部补齐、容量恢复B按既定独立任务继续；日志派生标签来源、原始占用重复行/内部术语、正确上下文上模型越界另保观察。不以两次相似回答就宣布稳定模型波动。

父账仍13/79交付、66开放，B2–B6与既有人审FAIL不回写。本批无写模式live，旧Go保护不能冒充新写模式真实验收。

## 112. 成文关系摘要使用已投影事实，数值成员不再伪装关系（2026-09-21，窄子片验收完成）

前批代码`2c0362e62`与人工审计`bd4ced6ea`已推送，65382正式exit0，本地/远端一致后继续。用户要求先清点剩余任务：按唯一稳定ID复核79=13+66，开放为58待实施/6部分实施/1待验收/1持续执行，未将审计小节和重复FAIL另算父任务；清单§2记录互斥分类与ROI顺序。

再次直接对照参考`ad_hoc_exploration.yaml:191–207`和`sleep_ops.py:225–255`：指标保持自己的数值/形状，最终解释使用本步结果，可借鉴来源/语义分离；不移植其基于状态/阈值的根因分类，不将参考查询顺序当本仓唯一执行路线。参考没有本仓AnswerSurfacePlan或紧凑源码成员合同，修复仍服从本仓现有权威。

公开native query→BuildAgentContext→两段实际成文消息红针：`TestAnswerAggregateEmptyProjectionPublicRelationDossier`空/混合来源×中英4格仅context消息重放被排除的明确箭头成员，既有最终instruction与原生1ms/一次D+IO事实保护先过；7676正式exit1，`/tmp/hmc-relation-dossier-native-red-receipt-20260921.log`（首轮73853日志同失败，旧session已消费，不伪造新收据）。用明确箭头而非小数作投影红针，防数字解析修好后掩盖回流。阶段/有无plan/源码与no_directed_path共12格62479正式exit1/context0.890s，仅final+plan+no_path违反已有投影，探索/提取和无plan正控先过。

数字语法独立RED16705正式exit1，`/tmp/hmc-numeric-relation-red-20260921.log`：十进制、符号/单位/科学计数、分隔符、十六进制、超范围数字与Unicode数值可被误拆；emit侧另有旧fallback把共享语法已排除的worker.go/settings.yaml重新解析。合法命名owner、数字右selector和显式数字端点正控不失败。

生产方案：成文阶段关系摘要只取同一AnswerSurfacePlan的StableAggregateFacts，非nil空结果也不回原始状态；explore/extract及无plan沿用旧raw合并。只换aggregate子来源，不隐藏其它关系证据/源码清单/hints。原函数按完整职责迁至独立23行文件，builder负增长、不抬额度。compact-dot只排除纯数值owner，保Unicode/$/_/7zip等已有命名及pair.0、显式123→456；数值解析超范围仍按数值语法处理。emit局部重复点号fallback删除，统一使用共享解析，不复制另一套规则。没有新增prompt、用户/答案原文扫描或硬门，没有改typed边方向或因果权限。

影响边界：此前误判为relation的数字/裸文件成员恢复literal，其派生aggregate身份和artifact hash会相应变化，不能宣称全部哈希不变；合法源码/明确关系的canonical身份和全部原始成员文本不变。原始审计数据不改写，不能把旧伪关系ID自动别名映射为新关系。共享消费者包含显示候选、归一化、覆盖/引用匹配和上下文，需宽回归，不仅验一条摘要字符串。

首次GREEN49988正式exit0（types1.392/tool1.123/context1.769/agent2.875s），`/tmp/hmc-relation-context-green-20260921.log`。宽定向race58032及全仓19878另记末版正式收据；源码no-path/正常relation、实际原生消息及因果/有限交接均纳保护。冻结双例计划G1旧FAIL+独立Java四层调用链：2并行×1，分别审原缺口与正常源码关系，Trace显式窗/IO/调度候选由原生公开回归保护；不把该替代测试称为新的causal live。

末版代码`51840eecf`：宽定向race58032正式exit0（types2.792/tool6.914/context5.029/agent10.274s）；全仓19878正式exit0，`/tmp/hmc-relation-context-full-20260921.log`为87测试包、13无测试包、零FAIL。8项默认值及流保活68881正式exit0/llm4.209s，仍保600/300/600秒、4ms连续partial帧、visible/hidden/tool进展及更短调用方期限。干净构建57413正式exit0，revision51840eecf7eb/buildTime2026-09-22T06:31:56Z。本批主审完成代码与测试，前段只读设计复核纳入实现，但后段子审因额度不可用，没有虚签本批独立末审。

## 113. 51840固定双例：机器1/2，完整人工0/2；可选值profile适用域顺序缺口（2026-09-21）

[机器摘要](../../eval/parallel_selected_summary_hmc_relation_context_20260921.md)、[完整人工审计](../../eval/parallel_selected_summary_hmc_relation_context_20260921_manual_audit.md)。runner87611正式exit0，严格2并行×1，G1 166秒、Java247秒；无第三例、不改oracle。新修复窄边界通过公开红绿与原生保护，不以此销父账或两份完整人工FAIL。

G1全捕获窗、原始D+IO三段0.138/0.147/0.350ms及合计0.635ms正确，错误模型近似值未重播，有限事实空旁路合理。最终仍从caller名字推成正常文件系统/块设备同步IO，附注泄漏英文系统词汇；完整人工FAIL。后续语言复核更正：项目明确锁中文，中文正文不是故障，不以英文问题单独判错。Java真实5条调用边/容量位置保留，图未删；但stdout被称审计落库，条件/分支项混作6跳、成功图未表达失败分支、状态/局部变量/存储字符串复述失真；机器与人工均FAIL。两次patch失败分别重复ref和同块atomic/replace冲突，正确拒绝未改基底，第三次成功；暂不为单次模型误用额外加门。

- [ ] 新P1，优先下批：`emit_analysis`对非标量问题先校验 `artifact_value_profile.value`，随后按已有 `is_scalar_answer=false` 丢弃同字段。真实日志551–552拒绝、577占位值、578丢弃，白耗一次20秒重试。泛化修复是typed适用性先于字段语义校验，并审同源field_value兼容转换，不能教模型编造占位值；合法标量/独立源码合同/请求窗和目标必须不变。
- [x] §111成文关系摘要投影及纯数字compact-dot误判窄子片：§112代码及确定性验收完成，合法源码关系live保留；不是Trace全部机理/答案已稳定。
- [ ] §111状态占用与归因分量身份：已有 `RootCauseNodeValueDescription` 用真实候选分量解释，末尾state/leader摘要尚未消费；下批考虑复用既有单源，不能新算分量或删原始睡眠/D/业务线索。
- [ ] §104未验证收益、§94局部业务补齐/容量恢复B、B2–B6及其余父项继续开放。对已经充足上下文上的模型语义越权不作无止境单样例提示拟合，也不称已证随机波动。

剩余仍79=13已交付+66开放，58待实施/6部分实施/1待验收/1持续执行。审计发现属于现有父项子缺口，不重复加减稳定任务数。

## 114. 可选运行时标量先判适用性，兼容转换不得复活非标量（2026-09-21，窄子片验收完成）

前批§112代码及§113审计随`317970982`已推main，30568正式exit0。直接再核参考`ad_hoc_exploration.yaml:191–207`按实际指标对象/数组解读、`core/batch/rootcause/evidence.py:46–79`保独立状态分量；不搬参考缺值归零/比例判根因，也没有其模型参数移植。当前顺序缺陷由本仓已声明的 `predicates.is_scalar_answer` 决定，不依赖原始请求关键词或回答文本。

新公开测试直接Execute并核落盘RequestModel及输入不变，覆盖causal/finite非标量、合法/缺值/空对象/坏枚举/坏置信度、合法及非法标量、旧field_value转换、独立源码字段、JSON错误类型。首轮78479正式exit1包括真实8项顺序错误、非标量兼容复活和sourcequote被转换吞掉，另有测试helper把预期JSON解码错误当fatal的夹具问题；修正后66686正式exit1，仅保真实反例。新添加disabled=false坏置信度正反针不冒称有旧RED收据。

生产14行等量移位/修改：先清除非标量可选参数并保原warning，然后走原解析；field_value兼容入口同样要求scalar=true。没有放宽合法标量校验或JSON解码，没有新增prompt、schema、用户原文扫描或模型结论改写，文件行数不增。独立源码字段仍先过自己的来源校验。清掉错误profile同时让原本不该启用的key-value合同、导出/证据消费者归位，不能说只是提示措辞变化。

第一次GREEN70811正式exit1仅新legacy测试的carrier夹具不完整：旧混合测试helper只有Meta/signals，没有实际观察，不能授权observation-only escape。换成独立包含PerfObservation的公开helper，既有混合helper不改；57431正式exit0/tool1.955s。独立只读末审无阻断，93684正式exit0/tool1.161s覆盖新旧标量、混合来源、schema与source保护；独立初轮63926的同夹具失败保留。最终新disabled针及全仓81951、race10597等待正式收据。

边界与未销账：本批仅修非标量适用域；旧scalar=true的legacy转换仍可能把有owner.field但source_quote错误的字段降到artifact来源，另记同源来源适用域P1，不冒称所有混合来源转换已经修好。状态占用/归因分量身份、§104收益口径和66个父任务继续开放。冻结双例为G1全工件读取＋Go小范围真实apply，各1次并行2；写案例不是B2–B6恢复证明，也不为旧人审追第三次求绿。

正式收据：代码`fd000269c`；race10597正式exit0/tool13.883s（含最后disabled针），末版全仓81951正式exit0，`/tmp/hmc-artifact-applicability-full-20260921.log`为87测试包、13无测试包、零FAIL。干净构建79885正式exit0，revisionfd000269c58e/buildTime2026-09-22T06:50:29Z；8项默认值与流保活28065正式exit0/llm4.424s，保600/300/600秒及4ms持续帧，不改变真正静默、用户取消和更短调用方期限。独立生产审计与公开正反回归通过，本窄片验收完成。

## 115. fd000固定读写双例：机器2/2、完整人工1/2（2026-09-21）

[机器摘要](../../eval/parallel_selected_summary_hmc_artifact_applicability_20260921.md)、[完整人工审计](../../eval/parallel_selected_summary_hmc_artifact_applicability_20260921_manual_audit.md)。runner12571正式exit0，严格2并行×1，无第三次追跑。Go86秒、G1 112秒。旧FAIL保留，父账仍79=13+66。

Go实际一行patch，原生TestGreet/真实go test -json命令通过，隔离工作树main.go之外无变更，fixture main HEAD等于seed2643ad38530b35f6f5215e3a9be962d02900dfe5，没有自动merge。最终交付明确自然语言验收清单不代表逐项独立执行，未虚签go build命令。完整人工PASS，但不是B2–B6补证恢复。

G1主审及独立人工均FAIL：三段D+IO、0.635ms、全捕获窗/目标/调用点正确，finite空root旁路与可选图缺席合理；模型仍把caller词形晋升同步缓冲/文件系统机制，现有最终输入已明确禁止，不因一次输出新增关键词门。两次分析提交中唯一拒绝是effect-vs-fact typed范围矛盾，修后第二次接受；profile始终false，新drop路径触达，但旧parser本也nil，不能冒称live复现旧缺value故障。该窄修以公开RED/GREEN负责验收。

- [ ] 新共享展示P1：项目锁中文已经进入最终模型消息，系统等待附录和补采说明却消费另一语言来源而变英文。应统一有效回答语言权威、保标识符原文，不扫描/翻译模型正文；双语无锁和显式锁都需公开正反回归。同步纠正§113“英文问题→中文正文错误”的审计理由，旧机理FAIL不改签。
- [ ] §111分量身份只读设计核对完成：详细reader已有 `RootCauseNodeValueDescription`，state/compact两个出口仍缺归因量自己的类型/说明。下一片应在同一projection/node复用既有描述，保原始状态/测量/行身份和未知空描述，不重算或授根因。独立现状保护99556正式exit0（tracefinding0.544/agent1.310s），不是新缺口RED/GREEN。
- [ ] §114标量legacy来源混淆、§104收益口径、§94局部补齐/容量、B2–B6及全部66父开放项继续按ROI推进。

## 116. 成文教学与系统附录共用有效语言（2026-09-22，窄子片验收完成）

前批代码`fd000269c`及收据`d82953c16`已推main，24923正式exit0，本地/远端一致后继续。稳定父账79=13+66不变。本片ROI在于同一个已证优先级错误影响等待、因果、补齐、频率、关系及日志附注等共用出口，修复面小且不碰证据资格，因此先于需要更多计量设计的分量身份片推进。

主审再读参考`ad_hoc_exploration.yaml:191–207`：最终回答应绑定原始问题及实际指标。参考没有本仓具体项目语言与分析合同双权威的同构实现，不能照搬默认语言；本片依据本仓已生效的project/CLI优先级。独立复核确认语言沿flag→orchestrator→BusContext→AgentContext→ToolBusContext完整保留，错在工具解析顺序，不是传输丢失。顺带纠正architecture旧“问题语言可覆盖具体项目配置”的陈旧说明，不改变已实施语言锁。

主审工具矩阵RED87674正式exit1/tool1.212s，扩展枚举后28087正式exit1，分别记录`/tmp/hmc-language-authority-{red,expanded-red}-20260922.log`。独立公共RED34453正式exit1/tool1.923s，`/tmp/hmc-locale-public-red-20260922.log`：8格真实event_search→自动window_stats补齐→emit/render/noop patch中5格语言失败、3格通过；原始D/IO、1ms、显式1..1.004窗、模型正文/保留说明、ledger/投影/raw bytes及幂等性保护无失败。不是只把附录字符串翻译的假端到端。

生产将既有agent纯解析逻辑移到types，共用project具体语言→contract→request→en；支持别名集合不扩张，off/none只在配置位关闭语言教学且保en系统兜底，auto/follow/空/未知代码按原agent逻辑回退。tool共享入口、源码未命中范围、通用范围附注、枚举系统标题/说明列与外部证据附注同源。原始模型语言字段、正文、证据ID/数值/窗口/根因权限不改，不新增JSON字段、原文扫描或内容硬门。CLI默认zh不动；无任何语言载体的工具直调由部分旧zh兜底变成与finalizer相同的en，此行为变化明确记录而非隐去。

新增types别名/来源矩阵与agent实际BuildInitialInstruction保护25436正式exit0（types1.068/agent2.069s），在agent迁移前先通过，不冒称红针。首次GREEN97041正式exit1，仅旧wrapped external-frame测试在全空语言时仍要求中文系统前缀；精准迁移成en并加完整模型中文原文不变断言，没有改帧位置或repo引用边界。末版定向45670正式exit0（types1.119/agent1.479/tool2.275s）。末版全仓55996、race83651待收正式退出；未提前销账。

范围不外推：deterministicCountAggregateLabel的语言参与派生aggregate身份，本片不顺手改；其它探索/修补提示和非中英翻译并未全域统一。§111分量身份、§114标量legacy来源、§104收益口径、B2–B6与旧人工FAIL继续开放。冻结双例计划仍严格2并行×1：G1有限等待检查实际系统附录语言＋显式20ms因果链保护投影和多类候选。前批Go实际apply已审，不冒称本批另跑写模式。

末版定向race83651正式exit0（types2.781/agent15.776/tool21.206s）；独立定向17314正式exit0（types8.027/agent8.795/tool9.645s）、独立race31441正式exit0（types2.233/agent2.961/tool6.022s），包括枚举append/容量/row-ID及table边界。8项默认值和流保护43325正式exit0/llm4.235s，保600/300/600秒、4ms连续部分帧、visible/hidden/tool进展及更短调用方期限。

首次全仓55996正式exit1，tool447.828s，`/tmp/hmc-language-authority-full-20260922.log`有145项顶层测试失败，不能被上述定向绿掩盖。原因是旧工具夹具遗漏项目语言却依赖中文，及双语夹具只写contract语言；与本片明确的project优先/全空en兼容变化冲突。逐项复核后12个测试文件只补显式语言输入和注释：公共夹具采用CLI实际默认zh，英语/双语场景显式en/lang；新nil/blank/off权威矩阵保独立上下文，不被中文helper污染。数字、证据、关系、容量、owner/model原文断言均不放松。独立静态审计PASS；扩9文件97顶层测试65082正式exit0/tool67.176s（含真实Trace A/B），原3文件49221正式exit0/tool66.360s。末版重跑全仓17639进行中，最终收据另记。

干净生产构建40753正式exit0，revision`4f3206c8c655`/buildTime`2026-09-22T07:14:44Z`，用于下节唯一双例；之前未提交文档时构建96036并非live版本。此后仅测试夹具与文档变更，不将第二次全仓通过冒称已经取得，也不把本片语言修复扩成全部答案语义修复。

第二次全仓17639正式exit1/tool411.648s，剩7项：三个对比表/下一步英语场景通过嵌套helper继承zh，却仅改contract；VSync附注、成员清单中文标点和两个脱离引用附注仍无语言。补6个文件中这些场景的显式语言，断言不改；残余7项58649正式exit0/tool1.197s。至此共18个测试文件输入迁移，首次/第二次失败记录均保留。迁移主体的增量race54067正式exit0（tool5.585/agent2.219/types3.033s）；7项末轮另复核。第三次全仓31168运行中，不以尚未完成的验证签收。

最终收据：第三次全仓31168正式exit0，`/tmp/hmc-language-authority-full-final2-20260922.log`为87测试包、13无测试包、零FAIL，tool431.475s。残余7项race37413正式exit0/tool6.127s；18测试文件独立末审PASS、无断言放松，夹具提交`3ceb3b15e`。生产`30c0176ef`、兼容文档`4f3206c8c`、夹具及本次完整审计一并推送；不改§117双例人工FAIL，不减少66个父开放项。

## 117. 4f320固定双例机器2/2、完整人工0/2（2026-09-22）

[机器摘要](../../eval/parallel_selected_summary_hmc_language_authority_20260922.md)、[完整人工审计](../../eval/parallel_selected_summary_hmc_language_authority_20260922_manual_audit.md)。runner30328正式exit0，严格2并行×1；G1 148秒、causal389秒，无第三次追跑。稳定ID重数仍79=13已交付+66开放（58待实施、6部分实施、1待验收、1持续执行），历史FAIL不倒签。

G1三段D+IO0.138/0.147/0.350ms与合计0.635ms、全捕获窗口、目标、原始调用点均保留。系统附录中文一致，但项目与contract此次都是zh，未live命中两者冲突，修复依据仍为公开8格RED/GREEN。完整人工仍FAIL：模型从调用点推导具体机理；把搜索返回40条/总匹配620条误写为捕获上限及Trace缺失；把实际零匹配的精确子串查询说成已验证完整D。原生全窗统计与最终交接足够，不能把全部错误归到系统缺材料，也不加答案关键词门。

causal明确2..2.020窗、app S20ms、四节点及三条跨核唤醒、链上IO11ms和三个1ms低优先级依赖候选保留，因果投影存在。正文却虚构中间线程醒后再睡，把唤醒当Running（app真实切入2.020020在窗外），把cookie sleep17ms当直接传导量，并由调用点推页缓存对象/机理。最终schema2旁路available、模型选IO与cookie两项，首次漏版本经一次patch修复且五块正文未动；合法JSON不抵销description机理越权。系统§104“已证最大可消/11ms可消”及重复占用行也仍开放，不以模型问题遮盖。

优先队列：①§111状态占用/归因分量身份（本批日志3722–3724、3737再次实证），及§114标量legacy来源校验；②§104未验证收益；③§94业务已接受实例局部补齐/容量恢复B；④声明/观测pair与B2–B6。新增待复现的源码适用域/完成条件接缝P1：两次accepted completion后因缺current_source重开，形成三轮/13次查询；最终同源交接却说明该义务soft、runtime_only_with_caveat=true、hard_block=false。先构造公开反例定位精确信号，不凭日志删除源码义务，不扩大观测为源码。模型机制越权与搜索/捕获误述独立留人工FAIL，不能称已证随机波动。

独立完整causal审计同判FAIL，并定位旧策略至`accepted_closure_origin_debt.go:174–215`：CurrentSourceRequired即阻止waiver，尚不区分本例可降级soft与精确源码义务；调度/对账出口分别在`orchestrator.go:6433`与`accepted_closure_reconcile.go:91`。这是accepted之后未能自动完成，不是completion硬拒，也不是语言片回归。下一公开反例须保真正混合源码、precise要求和历史来源边界；不能把downgradable一律当免除。系统关键事实原始英文说明仍属本片明确未承诺的全域翻译债。

## 118. 标量兼容转换不再改变独立源码字段的证据来源（2026-09-22，验收中）

前批语言生产、夹具及完整人审已随`6c826e37e`推送，77090正式exit0，本地/远端一致且工作区干净后继续。父账仍79=13+66。本片处理§114留下的scalar=true路径，不重做已经修好的非标量适用域。

直接核对参考`core/llm_contract.py:122–146`：evidence_refs先按member→json_path→field绑定后校验实际值，失败不改来源；其`_dig`支持点号/数组等运行时字段，因此不能全局禁止点号target。本仓field_value_profile已有owner-qualified目标与source_quote/枚举/置信度合同，artifact_value_profile独立承载运行时字段。参考`ad_hoc_exploration.yaml:175–207`按实际指标形状解读，不能当成把错误源码声明转运行时值的授权。本片不移植数值容差或原文扫描规则。

公开Execute红针31125正式exit1/tool1.625s，`/tmp/hmc-artifact-origin-red-20260922.log`：五种既有支持的限定形式（点号、命名空间、箭头、#及改名）×错误/缺引文、引文缺target/literal、非法枚举、缺失/越界confidence共35项均被原路径错误转换；真正observation-only的可选错误源码字段也被转换而非告警丢弃。缺literal、合法source、缺target/非限定legacy、显式artifact点号目标、两来源并存与无runtime载体控制均先通过。不将未支持斜杠/Unicode或单字符成员人为算成红针，也不扩大解析语法。

生产仅在legacy converter入口复用现有`ParseFieldValueTarget`：已识别的源码字段返回无转换，保原错误继续走既有拒绝/可选丢弃策略。没有匹配错误消息、扫描用户或答案原文、补引文、改变枚举/置信度或扩大硬门；显式artifact_value_profile可继续使用frame.duration等名称。源码原校验、非标量适用域、真正无owner的旧运行时兼容均保留。成功正控核显式2..2.020窗、目标/维度/原始17ms观察与提交JSON不变；失败不落盘替代profile。

首次宽定向12306正式exit0/tool2.110s，覆盖新旧Artifact/FieldValue/MixedRuntime公开入口。追加“合法显式artifact不能掩盖另一坏source”控制于末版race，全仓及独立审计待收正式结果；不冒称本片已经全部交付，也不倒签旧人工FAIL。

末版race36356正式exit0/tool13.270s；独立定向57141正式exit0/tool1.544s、race84924正式exit0/tool9.367s，含新增两来源负控，独立审计无生产阻塞。审计另发现skill与schema对同一字段的教学矛盾：skill无条件允许复制预分诊观察，schema早已要求用户明确的标量问题且不得抄预分诊模型摘要。公开schema＋局部skill段落6611正式RED/exit1/tool1.172s；把原schema说明原样提为共享常量，保target/value/unit/kind/refs与源码区分，删除冲突例子，不增运行时硬门。71795正式GREEN/exit0（tool2.224/skill2.068s）；独立82110正式exit0/tool2.409s、62860完整skill exit0/0.633s。代码`b736a22a3`，统一全仓74551运行中；尚未推送或销账。

## 119. 成文摘要保留归因量自己的分量说明（2026-09-22，验收中）

承接§111/§117的真实交接缺口：详细reader已明确低优先级候选的1ms来自runnable，两个高权重摘要却只并置sleep17ms或IO11ms与归因1ms，使模型失去数值自己的口径。参考`core/batch/rootcause/evidence.py:46–79`分别表示wait/supply/load并保降级信息；采用分量分离的原则，不照搬其缺失填零、最大值选主因或阈值裁决。复用本仓既有projection/node的`RootCauseNodeValueDescription`，两个出口各追加3行：state摘要在effective/identity之后，compact在同方向Leader选定之后；未知/不适用描述保持不输出，不按subject或rank跨窗借值。行数8/6上限、原始状态、计量、有效归因、累计账、行身份、目标/时间窗和模型正文均不变，文本字节略增而非扩大权限。

实际BuildInitialInstruction公开20格覆盖8类×中英及同主体/rank的双窗重排；逐具体行断言，防止详细reader的已有描述掩盖摘要缺失。有效RED21362正式exit1/agent1.138s：14格缺描述失败，纯IO/语义/未知6格先绿，原数值/身份/输入不变均通过。首版测试自身fingerprint键和窗口设置错误已在红针前纠正，不记为产品红针。GREEN73892正式exit0（agent1.864/tracefinding0.637s）；race24138正式exit0（agent4.942/tracefinding1.707s），相邻rank-domain/locator/scope/principal保护race32594正式exit0/agent3.223s；独立末版6行审计通过。全仓74551待正式结果，不以定向绿色销旧答案FAIL。

本批唯一live选择两例、各一次并行：`trace_query_wakeup_background_demotion`检验链上IO/调度分量与19.5ms链外等待不晋升主因；`read_combo_trace_current_source_explanation`检验真实当前源码＋86.111ms运行时观察的独立双来源。按因果误导风险、来源混淆泛化面、既有覆盖与成本排序；前批Go实际apply已审，本批不假称再跑写模式。机械与完整人工结果另记。剩余计数再次逐ID核验仍79=13+66，§104收益口径、§117完成接缝、§94局部补齐/容量、B2–B6及全部父开放项不销。

统一末版全仓74551正式exit0（87测试包、13无测试包、零FAIL；tool415.606/agent90.119/skill8.203s），`/tmp/hmc-origin-component-full-20260922.log`；默认600/300/600秒、4ms连续部分帧、visible/hidden/tool活跃进展及更短caller期限保护27189正式exit0/llm4.605s。生产`b736a22a3`、`8c102c030`和收据`ea2365916`已推送，73730正式exit0。工作区干净构建82201正式exit0，revision`ea2365916f01`/buildTime`2026-09-22T08:06:14Z`；唯一双例55480已启动，未完成前不记机器或人工PASS。§118/§119窄确定性缺陷完成代码、回归和推送，原答案级和父项范围仍开放。

## 120. ea236固定双例完成：机器2/2、完整人工0/2（2026-09-22）

[机器摘要](../../eval/parallel_selected_summary_hmc_origin_component_20260922.md)、[全文人工审计](../../eval/parallel_selected_summary_hmc_origin_component_20260922_manual_audit.md)。runner55480正式exit0，206/421秒，恰好2并行×1无追加追跑。父账仍79=13+66（58待实施、6部分实施、1待验收、1持续执行）。本批源码来源及分量摘要窄子片已交付，不抵销完整答案FAIL。

background实际命中§119：最终模型输入三行各自明确runnable1ms＋running deficit0，旁路四项选择保IO11ms和三PIC1ms；显式2..2.02、完整链、Trace投影与logger19.5ms仅背景均保留。人工仍FAIL：醒来被写成Running（真实切入在窗外）、network14ms的等待起点借给threadpool、四节点误叫四跨核跳、睡眠/观察链被提升为业务完成和确定原因、logger未确认IO却写成iowait。系统§104收益承诺和重复占用另留债。event_search的窗外导航为已标lookup-only容差，不是计算窗外溢；原始状态/因果仍保精确窗。

mixed-source原生86.111ms及真实源码定义均保住，有限问题的空schema2旁路正确；无新legacy字段错误声明，不能用该live替公开来源红针。全文63行审计FAIL：durationOrderObservations只是顺序审计载体，不是实际时长计算；resetTraceMarkSyncPairingState用于生命周期/坏marker，不是普通E关闭；模型evidence IDs与所写函数错配，附加引用亦错位；未实际搜索阈值却声称仓库没有，60Hz只能是假设而非已知设备基线。精确source_quote、effect维度、negative_search结构和summary缺失的修复都保原合同，结构最终通过不代表解释通过。模型summary追加导致重复，内部术语继续留展示债；本批无4ms活跃流降级。

新确认P1（HMC-01.3/16.4，未另增父ID）：源码未定位与Trace已观测caller双轴混淆。预分诊把caller误填stalls.file（background log274），emit_perf_trace.go:184–199产源码未解析denial后，Explorer原始材料及final typed调用点被改为`<unverified-external-source>`（1199/2064–2066）；晚到摘要2613、图、旁路仍保真实caller，末尾md441却称未被当前证据确认。原Trace第8行已观测名字，机理未证不等于名字未观测。生产链为denied_token_answer_check.go:51→repair_caveat_materializer.go:326；现有artifact例外563–613只覆盖LogBundle，未消费typed Trace caller。后续基于实际runtime来源身份修上下文与caveat，不删除全部源码denial、不授源码/锁/完成机理证明，保错误源码路径负控。

后续ROI：①§104计量词源与完整消费者统一（含精确物理重叠不能证明修其一收益必缩）；②§117真实接受收口回执的公开反例与调度消费；③本节caller双轴；④§94局部补齐/容量、声明-观测pair与B2–B6。参考设计意图逐项写清：等待/供给/工作量分离是量纲和解释边界，不是优化实效证明；member→JSON→field是证据归属约束，不是同词面任意跨来源互换。模型错误不能未经重复因果证明就归随机波动，也不追加输出原文硬门。

## 121. 已接受软源码限制的完成回执贯通调度（2026-09-22，窄片验收中）

先重新逐ID计数：79个唯一任务，13已交付、66开放；58待实施/6部分实施/1待验收/1持续执行。本片针对§117真实完成已接受却继续补源码，不将所有soft要求等同于免除。参考`core/llm_contract.py:110–146`按证据成员/来源字段绑定的设计，采用本仓真实已接受回执，不从同词面或预分诊猜来源满足。

公开路径是实际Run→EmitPerfTrace→EmitAnalysis→三类原生TraceQuery→EmitInvestigationComplete。测试只压缩阶段图与answer合同，不冒充完整LLM流水线；真实仓有main.go，排除零源码豁免。有效RED3446正式exit1/orchestrator1.714s，两个accepted消费者均返回false，常规图首次接受后仍派发3次；对账图原先可能由HasEnoughFacts闭合，不宣称两图都复现3次。真正源码/精确main.go:2首次完成仍被退回，generation=0。

新helper要求当前完成标记、非零代次、已发布源码专属caveat回执、当前runtime载体及soft/caveatable权威共同存在；仅从两条共享来源过滤路径移除current_source，不删其它来源债务、不改源码已满足状态。重置/新Run、精准源码、源码已真实读取后均不走新豁免；strict/backtrack/pending read仍由既有消费者阻止自动完成。正常Run从3次派发变1次，保源码尚未验证的限制及2..2.020窗S20ms。

GREEN56690正式exit0/2.936s；边界17919正式exit0/4.495s，race60384正式exit0/22.997s。首轮boundary83422中ExactTargets误写行锚、无runtime仍强求混合来源拒绝，属夹具假设错误，纠正测试而不扩大生产门。独立末审无阻断，1642正式exit0/4.517s、独立race96849正式exit0/23.058s。日志`/tmp/hmc-soft-source-{public-red,public-green,boundaries,race,independent-review,independent-race}-20260922.log`。末版全仓、提交推送及固定生产回放另补；不销此前答案FAIL或父任务。

## 122. 原始运行时材料与原生等待证据不继承源码否认（2026-09-22，验收中）

§120真实问题的两个轴分开：Trace记下某caller，不等于当前仓有它的源码定义；源码路径被拒绝，也不代表Trace没有这条记录。四个明确构造入口（原始Log、原始Trace、原生根因清单、原生等待清单）保其真实材料；预分诊模型摘要/源码推断继续原sanitizer，源码ReadFile的L1拒绝保持。没有全文token白名单、caller名称授权、删denial、修改root身份/窗口/值或模型正文。最终答案caveat中的双轴混淆尚未修好，本片只关闭上下文污染，不能宣称整个§120完成。

真实EmitPerfTrace→TraceQuery→BuildAgentContext→BuildPromptContext→ToMessages公开RED78635正式exit1，原始两面及32个typed展示比较失败；原始文件、来源拒绝、计量/投影控制先过。四组分别是依赖线程、另捕获/主体、另窗口、无caller，每组独立运行×explore/finalize×中英，不冒称混捕获合并。GREEN87188正式exit0，完整context19626正式exit0/1.512s。日志`/tmp/hmc-artifact-context-{public-red,public-green,all}-20260922.log`。

独立审计发现旧等待census兼容入口把任意成功工具的同形Summary当原生查询，typed入口又允许模型aggregate借provenance=trace_query混入。去掉展示sanitizer前必须封住这类来源漏洞，不依赖源码denial字符串遮住。新增实际原生查询及EmitInvestigationComplete公开反例10784正式exit1/context3.920s：4类非Trace工具×2阶段及两种模型aggregate provenance×2阶段污染；trace_query旧成功、失败查询、正常model_emitted及原始ledger不变控制先过。首次编译方法拼写错误不记产品RED；生产修正与末版验收待补。

## 123. 归因计量不再承诺已验证的优化收益（2026-09-22，验收中）

参考仓`docs/superpowers/specs/2026-09-12-optimization-advisor-design.md`全文设计为“待评审”，不是已实现功能。它将诊断、带依据的建议、开发修改、重新采样、按同一指标验证分开；本仓应保现有原生计量/选举，统一估算与实效的语义，而不是按建议关键词选修复或直接承诺收益。`core/preprocess/sleep_ops.py:220–263`裁剪状态交集是计量依据，也不证明干预必然节省该时长；不搬参考缺失归零或最大值自动归因策略。

共享`tracefence`双语词源统一系统标题、因果图/完整与简明图例、方向概览、行动摘要、语义表、HTML无障碍标签和工具/skill/成文交接：称“估算优化潜力”，明确链上依据不等于收益已验证、实际收益需实施后复测；缺频/纯频率下界限定为既定理想模型内部下界；精确物理重叠只证明共享计量时间，不再说修其一必然缩小另一项收益。主根因选举、有效值、分量、凭证、时间窗、旁路schema及围栏token不变。没有新增JSON字段/硬门/原文扫描；旧retired模型正文替换helper保持HEAD原样，不借本次展示改造复活它。

公开原生TraceQuery→EmitAnswerDocument→Render→无变化patch有效RED63470正式exit1/tool3.040s，12格中2个有限事实控制先过、10格116条收益词义断言失败，所有原始值/身份/排序/模型字节控制先过。新增公共GREEN88986正式exit0/tool3.315s：IO11ms+三项PIC1ms、真实Tieba语义边前0.285ms、语义相交17ms及缺频估算6.667ms保留，链外shader未入主因，有限事实不扩因果图。物理重叠/邻近展示用既有typed夹具，不冒称均由新实Trace生成；概览结构行100显示格上限保持。原模型引用旧措辞完全保留，证明只改系统拥有内容。

相邻首轮63731正式exit1，旧文字pin未迁移；独立逐条更新系统新词期待，数值、排序、反例、宽度不变，不弱化测试或重签历史答案。独立审计要求保retired helper匹配语义及补英文/skill残余，已采用。末版回归/全仓、提交推送和唯一2并行×1回放另补；§104答案级实效主张、§94局部业务补齐/容量、原生pair与B2–B6及其余父项继续开放。

本轮正式集成收据：§122两处精确来源修复为runtime origin＋direct/旧版unknown authority＋既有deterministic producer分类，legacy Summary仅成功trace_query；公开GREEN42396正式exit0/context1.513s、同集race33862正式exit0/6.079s，原生/旧记录正控不改即通过。模型aggregate仍留原审计ledger，不晋升原生等待。原四入口独立只读末审通过，最终caveat明确未纳入。

跨包53986正式exit1：context恰读到并行新增测试尚未修正的方法拼写；agent两条旧/新增不适用的词针、skill旧字节pin需精确迁移；types/preview/tracefence/orchestrator整包均过。修后成文实际消息/skill/context定向42369正式exit0（2.742/0.905/2.170s），四包末版race95324正式exit0（tool39.525/agent5.111/context10.814/skill2.175s）。skill保两旧hash，先精确撤回本次三处词面改动和一条共享含义再校验，未放宽原所有权/软适用范围。工具Description字节pin96279正式exit1，81355机械更新exit0；唯一差异为闭合矩阵的两处旧收益措辞和共用含义，新增EVOLUTION RECORD，须末版无更新模式再过。600/300/600秒及4ms活跃流/更短caller期限89453正式exit0/llm7.044s。全仓42971仍在运行，未先记成功。

下一固定双例：`trace_query_frame_semantic_span_optimization`（边前语义工作、测量与掉帧原因的证据边界）＋`nested_python_increment`（真实嵌套项目apply与既有测试证明），各一次并行2。按误导影响、跨状态泛化、近期覆盖与成本选；不再追同一个IO样例。目录审计另发现H8旧case要求语义边前effective=0/非根因，与当前闭合矩阵及真实0.285ms公开回归不一致，保旧oracle与历史结果、另立现有HMC-18下维护子项，不用错误oracle倒逼因果计量回退。本次生产回放不使用该冲突case。

收住阶段成果：§121提交`b7fe02b63`，§122提交`a48c517a1`，暂待末版全仓后统一推送。§123独立旧tool针迁移35文件，只改系统展示正/负向精确字面，不改数值/阈值/排序/谓词/宽度/digest；retired模型正文夹具不动。独立首轮tool整包61798正式exit1/489.872s、无超时，失败均为旧词；最后6文件10个顶层针94614正式exit0/1.857s。末版context整包41237正式exit0/2.534s；公开语义＋无更新Description golden19400正式exit0/tool4.679s，最后四包词面/消息70185正式exit0（tool3.525/context1.979/agent3.818/skill2.646s）。全仓42971首轮context旧针已发现并修正，保这次失败记录，待正式退出后在冻结版本统一重跑；不以分包绿替全仓绿。

末版正式收据：首轮全仓42971 exit1/tool470.554s；第二轮72123 exit1/tool430.891s，仅剩双向图例探针中的旧词“收益不叠加/gains do not add”。该探针精确迁移为“潜力不可相加/potentials do not add”，69414 exit0/tool3.824s，未改生产或放松双向性。第三轮73767正式exit0，87测试包、13无测试包、零FAIL，tool380.865s，日志`/tmp/hmc-closure-origin-potential-sealed-full-20260922.log`。§123生产提交`aff63de7b`；干净构建8733正式exit0、revision `aff63de7b9a1`、buildTime `2026-09-22T09:34:53Z`。最后测试迁移及下节全文审计单独提交，三组窄片已完成代码/回归，不代销父项或历史答案FAIL。

## 124. aff63固定双例：机器2/2、完整人工0/2（2026-09-22）

[机器收据](../../eval/parallel_selected_summary_hmc_closure_origin_potential_20260922.md)、[完整人工审计](../../eval/parallel_selected_summary_hmc_closure_origin_potential_20260922_manual_audit.md)。runner61080正式exit0；恰好2并行×1。Trace外层195秒，Python外层146秒/案例自身143秒，保层级差异。逐ID仍79=13已交付+66开放（58待实施/6部分实施/1待验收/1持续执行）。

Trace实际保5.000–5.007窗口、S5/runnable0.8/running1.2ms及完整因果投影；VerifyClass原始5ms、边前4.6ms与完成落在唤醒后0.4ms均在证据中。估算潜力而非已兑现收益的新教学真实到场，旁路是模型主动patch选择空schema2 available，不是导出错误。但正文把“完成类校验”放到5.005唤醒之前，还声称两修向相互独立，人工FAIL。最终上下文已给原窗与完成/独立性未证明，不能归因反证缺失，也不凭单次运行称模型波动。前置triager自身也产错误时序/优先级，但final输入已将其降为定位、归一化平台语义，未证错误句原样带入，不倒推其为本次主文错因。

新确定性展示缺口：tree.go的语义图例把SemanticSpan、SelfDeterministicBasis、SemanticMentionFloor三个标记合并触发“按目标线程窗内运行时间计”；本例实际上是worker语义墙钟，而非app的1.2ms running。修法应按真实发射标记拆开计量定义，保语义边前归因和排序；系统占用表旧收益简称、英语平台原文仍独立留展示债。不得用修改图例冒称已经修好模型完成/唤醒矛盾。

Python源码仅+1、原测试/配置未改，真实执行了当前交付源变更行，身份和hash完整；但run_tests在probe PASS后提前返回，原生unittest明确suite_skipped，±2**64等已有断言未执行。项目test observations缺失，6条行为合同为planning-only；已有精确test observation一旦保留，其suite选择/continuation原本有效，不能误归消费者完全坏。待补独立、持久的typed既有测试执行要求与同交付执行收据；不从原始问题/acceptance文本扫词，不把所有自然语言行为声明升required。原生aggregate PASS仍不自动证明每个行为合同，skip/zero-tests/timeout不能销执行债。静态coupling不识别probe内sys.path亦留独立适配债，不能混称ImportError。

下一优先：①既有测试执行意图持久化与收口；②§120最终caller附注双轴；③§94业务局部补齐/容量与声明-观测pair、B2–B6；短小确定性语义图例缺口在当前收尾后单片修复。caller后续设计以ClaimUses.EvidenceID绑定原生记录/物理来源/窗口/subject/caller角色；未知或模型aggregate不授观察证明，不能复制Log运行级名称白名单。默认文案只说“源码映射未核验”，不把Trace已记录caller说成全证据不存在。

§121–§124提交与审计已随`2f3a6dc2b`推送，45232正式exit0；本地/远端同SHA，干净工作区核实后开始下两片。没有重跑旧live改变人工结论。

## 125. 语义图例按实际计量来源拆分（2026-09-22，窄片回归完成）

§124展示反例继续追到引擎：`rank_family_fold.go:356`的自身准入只判目标身份、确定性语义类和正片段范围；`query.go:19424`以裁剪后extent计量，并非与Running状态求交。参考`core/preprocess/sleep_ops.py:220–263`的交集计量设计提示必须注明实际求交对象，不能把span墙钟当CPU时间。本片不复制另一种计量算法来迎合图例，而是修正系统说明。

普通SemanticSpan按所属线程/记录范围的墙钟解释；SelfDeterministicBasis按目标自身语义片段的窗内投影并集解释，保既有排名凭证但不说仅CPU执行；SemanticMentionFloor只说明保留的未排名语义线索，不借其标记授计量或榜位。production只改tree.go完整self词条和简明图例三标记分支，中英常量复用；数学、准入、选举、模型字节、JSON均不改。

真实TraceQuery→emit→render→noop patch共10原生中英格：worker/self/self跨S/mixed、只有wakeup查询的未排名语义；另16个显式渲染mark组合单独记账。跨S反例原生self1.000ms而目标Running0.600ms/Sleep5.400ms，仍保self凭证和第1名，证明不是一概按Running计。有效RED16288正式exit1/tool1.662s，失败仅图例断言；首版无链self夹具无效，修夹具不动生产后才记RED。末版公开/相邻4135正式exit0/tool3.516s、race70835 exit0/tool6.011s，独立复审通过。日志`/tmp/hmc-semantic-legend-caliber-{red-v2,final,race}-20260922.log`。不倒签§124模型完成/唤醒和独立性错误。

## 126. 源码未验证不再被说明成全证据未记录（2026-09-22，窄片回归完成）

§120最后附注默认入口的错误不必等到新观察证明查询才纠正。`denied_token_answer_check.go`修补提示曾要求“NOT present in current repository”，`repair_caveat_materializer.go`又把所有来源说成“尚未由当前证据确认”；两者都超出源码映射拒绝的证明范围。现在只说明当前源码映射未验证，并明确不据此断言仓库无同名文件/符号或附件未记录名字；实现/机理仍需独立核验。不删除原拒绝，不新增原文扫描、caller白名单、Trace豁免或源权限。

测试调用真实checker→公开AppendSoftContractCaveatsToAnswerForBus/API：3拒绝类×中英、异常详情×中英与三名称上限；模型前缀、doc、denial不变。它不是原生EmitPerfTrace/TraceQuery全链，完整caller记录/附件/窗口/主体绑定仍开放，不能以默认文案修复代销该范围。有效RED28985正式exit1/orchestrator0.703s；前两轮仅测试方法拼写及另一新增测试编译错误，不记产品RED。GREEN70272 exit0/0.901s；相邻40148 exit1为4个旧字面pin，精确迁移5条正向词义并加强Log专用例外负针，不改断言逻辑。末版race70084正式exit0，独立27007 exit0/0.843s，无P1。日志`/tmp/hmc-denied-source-boundary-{red3,green,adjacent,race}-20260922.log`及`/tmp/hmc-source-denial-independent-20260922.log`。

两片待下一次全仓收据统一补记；父账仍66开放。既有测试执行意图已有独立公开RED38703正式exit1/orchestrator4.060s：2个新要求场景都被probe替代，legacy/preserve-only/既有ProjectTestObservation三对照先绿。下一片用可选typed constraint和工具拥有的精确执行收据实现，不把自由文本或静态文件存在性升级为测试证明。

两片已分别提交`3148d2a00`、`3cb204025`并推送，42900正式exit0；全仓复核仍待本批末版，不把之前73767收据套给后续改动。

## 127. 用户要求执行的现有测试不能被探针或其它文件的结果替代（2026-09-22，实现回归完成，生产分支未命中）

先按唯一复选项复核：79总项=13已交付+66开放，尚未修改父项状态。本片承接§124真实Python人工FAIL，优先于新领域扩张。参考`core/llm_contract.py:110–151`的设计意图是把证据绑定到已登记成员及其实际字段；本仓借鉴其归属原则，不复制文本子串/数值容差为执行证明。参考优化建议设计第六节把方案身份、同口径复测和实际结果分开；它仍为待评审设计，不宣称参考仓已经实现本片写模式原生测试验证。

最小新增要求为已固定请求IR中的`constraints[].kind=run_existing_test`，目标是当前仓已读取的精确文件。它独立于保留原测试字节和行为合同证明；计划遗漏project-test observations也不能消除执行要求。新执行收据由原生工具生成，模型不能填写；旧报告消费时重新核验，而不是信任已有“通过”标签。未知选择器、零断言、全跳过、取消或基础设施失败不应由probe PASS顶替。

有效公开RED38703正式exit1/orchestrator4.060s，走真实Run→结构化分析/计划→补丁/交付→run_tests→workflow；只有模型选择被脚本控制。要求原生执行的通过例与大整数失败例都被旧probe提前收口；legacy、仅保护测试、已有ProjectTestObservation三对照原先通过。早期夹具字段编译错误不记产品RED。

实施中的独立审计阻止了更宽的错误闭环：命令`suite`指向某文件不等于其断言实际执行，项目配置或`load_tests`可能转派其它测试；同cwd的其他PASS不能借给目标。另需核实际HEAD/文件字节而非只复制计划指纹，逐目标保留执行债；预阶段4次轻读/6轮约束也不能与“逐文件先读取”的新教学互相矛盾。先补原生unittest精确文件的工具自有运行时观察，其他协议无精确身份时保持未验证，不临时弱化成路径字符串相同即可。尚未取得末版GREEN/全仓/提交收据，本节不得作为交付证明。

独立审计同时留出后续高ROI接缝，不藏在本片“未支持”中：既有pytest固定JSON路径仅在结束时清理，逐调用产物新鲜度尚须修复；既有部分Python/Ruby命令把路径用双引号传shell，含`$`/反引号的字面文件名可能被展开。新unittest观察使用独立临时目录、独占创建报告及literal shell quoting，不代表所有旧runner已修。Node位置过滤也不自动构成精确文件执行凭证。以上归入HMC-18跨模式验证子债，父ID数量不重复增加。

预阶段附片实际复现了相同的系统自冲突：真实BaseAgent四次预读后emit保护非惯例基线文件被拒，要求先read，但schema只剩emit。52784正式exit1/agent1.971s；前轮71398是并行施工期间编译错误，不记产品RED。现仅在typed emit失败后提供一次额外`read_file`修复回合，保原4次正常预读与6轮总cap，不扫错误文案；同批旧schema拒绝的read不消费下一回合，真实失败read则消费，多个read仍按原整批执行规则，不误称严格一个调用。默认教学改为“一次成功发射”，拒绝可修正重发；共享现有测试执行教学到实际模型上下文。根因/工具权限/写风险门不变。附片末版与原生执行片一起补验收收据。

阶段验收收据（尚待全仓正式结束）：原生公开初绿91231 exit0/orchestrator5.399s，要求执行时真实3条原测试，有限整数探针仍通过的大整数错误被原测试检出；legacy、仅保护测试及既有观察路线保持。边界87102 exit1中全skip/零测试/转派已正确保missing，夹具误把workflow的complete生命周期等同于verified结论；检查真实Completion.Verdict后改验unverified与执行债，不改terminal政策。非惯例文件正控另提供真实已存在unittest候选，不能凭已读取文件假定测试框架。90444 exit0/orchestrator8.824s，11场景通过。

独立consumer逐字段破坏、双目标JSON往返及旧报告投影：43177 exit1，唯一新发现是非hex文件哈希被接受；修后9598 exit0/types0.775s、38400 race exit0/types1.989s。此矩阵为typed夹具，不冒称真实执行。原生观察器同时补类/模块fixture事件不经过startTest的处理，保原unittest parser的导入错误分类和原生traceback；观察文件损坏/超限等只丢精确执行凭证，不替代原生结果。末版90903正式exit0（orchestrator15.756/tool4.418/types2.486s），59630 race exit0（28.486/6.469/2.240s），日志`/tmp/hmc-existing-test-sealed-{focused,race}-20260922.log`。最后timeout原生反例单列后补；早期编译错误与单条loader reason旧针预期错误不作为产品失败或隐去。

预阶段附片5979 exit0/agent1.134s；实际默认教学进模型消息40741 exit0（agent0.961/skill1.532s）。独立审计提出“单回合非单调用”及同批不可用read误消费，均补真实BaseAgent反例；主体末版54152 race exit0（agent2.387/skill2.590s），最后未使用修复轮也消费额度的负控10402 exit0（agent1.748/skill0.798s）；独立末审56519 exit0/agent0.998s、skill93324 exit0/0.626s。

生产已冻结，全仓37585执行中，日志`/tmp/hmc-existing-test-and-read-repair-full-20260922.log`；不得先签全仓通过。新增非阻断上下文成本债：原生观察程序现在随完整Command留档，planner的命令展示可带入内部程序正文，controller截240字符可能遮住末尾目标；后续应以同一工具拥有的程序/命令/目标身份分离展示，保完整执行审计，不扫描或改写模型正文。当前不扩成新的执行器或为此更改运行语义。剩余66父项及历史live人工FAIL均未销账。

冻结后第17个真实timeout控制：95813正式exit0/orchestrator3.229s、21856 race exit0/5.023s。首次16105 exit1是新测试误假设timeout早返也保pre-suite TestResults；旧路径保的是已执行probe命令及ProbeExecution，补正只核实际凭证，仍严验native timeout/Passed=false/missing执行债且非verified。如37585已编译该旧测试，必须完整复跑，不能合成全仓成功。原生producer独立末审确认fixture事件、loader原分类、原生traceback及观察超限回退已闭；仅unittest、继承/动态包装方法保守缺凭证、subtest按父方法计数是明确边界。另登记P2：原生已结束后写观察JSON发生OSError可能改变包装进程退出码，被旧parser归为测试失败；目前不假绿，但尚不能宣称所有观察基础设施故障都精确归类。

全仓首轮37585现已正式exit1，tool410.664s；唯一失败为启动时编译的旧timeout测试断言，未发现新增生产失败。按上述规则启动冻结末版完整复跑76502，日志`/tmp/hmc-existing-test-and-read-repair-sealed-full-20260922.log`，不拼接局部结果作成功收据。独立只读复核逐ID无重复，确认79=13+66、跨语言/上下文成本/父项和历史FAIL边界均未夸大。

收尾主审发现新增unittest观察器未沿用目录身份保护，必须在本片交付前补齐：真实文件系统RED72144正式exit1/tool2.228s，日志`/tmp/hmc-existing-test-filesystem-red-20260922.log`。目录换软链接或换目录时旧入口读到外部/替代报告且清理误删；报告软链接虽拒读仍被清理，读取后替换普通文件亦被误删。普通读/额外文件/超大报告三控制先绿。这是当前新增通道的确定性P1，不以临时目录随机名字宣称安全；只补拥有的目录/报告身份、安全有界单次读取和精确清理。76502覆盖补安全片前代码，其结果不得代签后续修改。

安全补片前的76502已正式exit0，87测试包、13无测试包、零FAIL，旧timeout断言问题已消除；该收据同时覆盖§125/126，但不包含上述新安全片。JSON写入误改退出码的P2也获得真实native subprocess RED40426正式exit1/tool1.906s：目录/文件占用×原生pass/fail四格，两个原生fail控制先绿，原生pass却因FileExistsError退出1。末版将只捕获最终观察写入OSError并保真实unittest退出，缺执行凭证继续未验证。

安全片独立末审确认准备时目录身份、实际2MiB＋1读取及前后文件快照保护稳定替换；碰撞非JSON文件不应因读到字节而获得清理所有权，追加RED37131正式exit1/tool1.613s后延迟至有效报告完成验证才发布清理身份。P2 OSError测试是真实observer子进程→readReport→原parser，不是完整Run→Completion；缺凭证仍未验证由已存在Execute/consumer用例另证。剩余边界明确留账：check→open/remove非原子；Python writer尚可沿已替换父链接新建外部文件（不覆盖既存文件，后续拒读不授凭证），不能称完整抗恶意并发的文件系统沙箱。本片只签已证稳定替换误读/误删、实际限量读取及原生退出语义，通用产物writer/原子所有权仍属后续子债。

安全补片末版60254 focused正式exit0（tool5.219/orchestrator17.159s）、76519 race正式exit0（5.304/27.612s），日志`/tmp/hmc-existing-test-filesystem-final-{focused,race}-20260922.log`；7个文件系统情形、4个真实子进程写入失败情形及有界reader控制、原17个公开Run均通过。root补片后agent/skill51681 exit0（1.621/0.658s）。两笔本地代码提交为`111b22d23`原生执行意图与凭证、`b3c0bf870`分析补读/教学；暂不推送。以b3c0bf870冻结生产，末版全仓94036（`/tmp/hmc-native-execution-final-full-20260922.log`）与构建9277进行中，之后同版本固定双例，不扩到pytest或其它runner。

## 128. 后续测试产物归属审计（2026-09-22，只读设计，未实施）

独立源码审计确认pytest固定`.codrax-pytest-report.json`从命令构建流向直接读文件，实际Execute仅结束时defer删除；本次不写报告时可读旧绿，同次不同selector亦复用路径，清理还可能删掉原有文件。Python JSON解析不读取本次普通非零退出也是相邻缺口。此处是源码论证，尚未运行公开反例，不标RED或已修复。

下一批P1方案：复用现有JUnit/CTest的每次调用私有目录、安全单次字节读取及所有权清理；纯builder/教学预览不产生目录。JSON与XML各保原解析器，当前字节同用于hash与解析，不用mtime证明新鲜度；缺当前JSON仍走已有“重新执行并解析本次文本”的恢复，不能借旧产物。当前非零退出不能被绿JSON覆盖，但保留真实断言结果、不合成假失败断言，零测试语义单独保护。清理仅针对本次拥有的精确文件及空目录，不递归删客户目录。

回归应覆盖旧绿无新产物/旧红新绿/同字节新产物、多selector/并行隔离、当前非零、缺JSON后文本恢复、软链接/目录替换及旧文件字节和时间不变，保护既有Maven/CTest和本批unittest收据。公开入口可用真实协议子进程，不能冒称已跑原生pytest。Meson固定XML和Gradle/hvigor旧目录发现属于后续同类适配；Node结果来自本次stdout，不是固定文件新鲜度问题，其忽略runErr须单独记录。以上仍归原HMC-18子债，不重复增加66父项计数。

评测目录只读复核：249个case含26个apply、3个plan、220个默认读模式。本批在新冻结版本选择§124原双例各一次，直接验新worker语义图例和既有测试要求；未live触发的self/mixed/mention图例、read-repair和caller分支不能借签。随后建议`trace_query_jank_field_inventory`＋原版`github_issue_dateutil_relativedelta_float`：分别覆盖有限全量检索的身份/时钟/大整数/展示范围、根目录Python仅保护测试不自动铸执行义务。近期多次IO案例不再优先重复。名称含binary/converted但内联文本的案例不能算真实二进制入口验收。参考`config/skills/frame_drop.yaml:76`的定位→转换→分析→按对象指标及`core/skill_executor.py:895`的默认参数注入/显式覆盖，借鉴减少模型搬运参数；不复制首个文件等于全部或关键词路由，更不能将待评审优化方案文档算已实现写验证。

## 129. b3c0bf末版全仓与固定双例验收（2026-09-22）

末版全仓94036正式exit0，87测试包、13无测试包、零FAIL，tool407.915s；日志`/tmp/hmc-native-execution-final-full-20260922.log`。源码输入已包含§127安全和OSError末片，也覆盖§125/126，不套用之前76502的旧代码结果。活跃隐藏推理/正文/工具调用、keepalive、静默与调用方期限保护38440正式exit0/llm8.361s，600/300/600秒及活跃流无正文不降级的原策略未改。构建9277正式exit0，revision=b3c0bf870378-dirty、buildTime=2026-09-22T10:58:36Z；dirty仅未提交文档收据，Go/构建输入已净且被runner验证，不能称整个工作区当时干净。

[机器收据](../../eval/parallel_selected_summary_hmc_native_execution_20260922.md)、[完整人工审计](../../eval/parallel_selected_summary_hmc_native_execution_20260922_manual_audit.md)：98029正式exit0，恰好2并行×1，机器2/2、本次用户任务正确性完整人工2/2 PASS。Trace151秒、Python外层127秒/案例125秒；没有第三例追跑、改旧oracle或重签§124失败答案。

Trace完整292行/原始时点/原生观测/模型输入及旁路同审：5ms语义墙钟、4.6ms链上计入、唤醒后0.4ms，目标S5/runnable0.8/running1.2和worker边前running4/full4.6分别保留；正文不再倒置完成/唤醒，不宣称两方向干预独立。worker墙钟图例真实命中，因果图/自动补齐/背景分离保留。模型一次可选patch主动选择两项，schema2旁路0.0046/0.0008秒、frame_unproven及5..5.007范围正确；非强填。self/mixed/mention/caller分支未命中，前置模型仍有睡眠5.8ms/完成后唤醒等错误表述，但最终上下文有正确证据且本轮终文未继承，不能说前置质量债已闭。内部英语和旧可消除简称仍为展示债。

Python仅实现+1、原测试与配置逐字不变，交付commit=c27961f556ef4d13a30d62674d0bd2be12d358a4；当前worktree实际运行3个原unittest方法（含±2**64），没有用probe替代，根目录零测试分列，故用户任务PASS。但固定IR只有preserve_regression_test，没有run_existing_test，报告无新执行收据，本次依靠既有PTO选测试；新教学真实到场仍未发射该执行要求，分析补读也未触发。§127公开真实Run/独立consumer/race验收成立，不等于本次新机制live验收；不以词扫描补写模型要求，也不凭单次遗漏认定波动。三条PTO身份仍错误、10合同planning-only，原生声明-观测pair/B2–B6未销。新增低优先计数债：proof_profile.probe_count取全部VerificationConfidence数，本例source_compile被计为1，不能当成post-apply probe执行数；规划期临时probe零测试另记，不混层次。

逐唯一ID仍79=13已交付+66开放；本轮完成的是既有父项下的确定性子缺陷与新版本两个用户任务，历史FAIL原样保留。优先级按§128旧pytest产物冒借P1→caller双轴→业务局部补齐/容量→声明-观测pair/B2–B6→能力目录/IO总体/精确帧执行。当前代码两笔本地提交111b22d23、b3c0bf870，文档与本轮审计一并推送收尾；推送正式收据随后补记。

收尾正式收据：代码两笔及审计`0fefcca6c`已推送main，46680正式exit0（`3cb204025..0fefcca6c`）。独立提交前只读审计无阻断，逐ID无重复且新分支未命中、旧FAIL与安全剩余边界没有误销。干净构建42607正式exit0，revision=`0fefcca6cc50`、buildTime=`2026-09-22T11:13:49Z`，日志`/tmp/hmc-native-execution-clean-build-20260922.log`；构建后工作区干净。该构建只有审计文档revision变化，不替换此前b3c0bf固定双例的原始构建身份或冒称又跑评测。下一§128仍只读设计、未实施，66父开放项不变。
