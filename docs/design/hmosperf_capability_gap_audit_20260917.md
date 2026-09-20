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

### 30.2 已查询IO测量释义的纯展示补位（施工中）

§29分布例接受的profile只有count_or_duration，原生三组统计已到达，但受family选择控制的专用明细范围释义没有到达。仅在已有原生IO观察上补说明，不替模型新增io_latency请求，不修改共享family匹配函数或借新说明触发补齐/根因硬门；不调用可跨族联合状态账的完整bridge。已有IO专用/causal车道继续原处理，避免重复。退出条件：公开查询→TurnA→最终上下文先红后绿，逐条保留物理来源、查询窗/行窗、事件实际区间和计数未知性；旧车道、显式窗外、非原生来源及多个独立收据均有反例。整仓、竞态及发布收据待本片冻结后记录。
