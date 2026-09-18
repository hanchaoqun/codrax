# HarmonyOS PerfMcpServer skills / pipelines 对照审计（2026-09-17）

## 1. 范围、基线与结论

本分册负责参考仓 **全部 24 个 `config/skills/*.yaml` 和全部 5 个 `config/pipelines/*.skill.yaml`**，以及它们调用的关键实现、现有测试、Codrax 对应入口。工具注册/转换器/执行框架的全量盘点由同批主报告负责；本分册不把同一底层工具重复记作多个独立缺陷。

- 参考目录：`/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main`。该快照没有 `.git` 元数据，本次不虚构版本号。
- Codrax 比较基线：`93bf1a42d`（B1716 已提交推送后的代码）。
- 清点清单：同批 `docs/design/hmosperf_inventory_20260917.json`（含每文件 SHA256）。该独立 YAML/AST 清点为 109 indicators、24 skills、5 pipeline 定义、13 MCP tools、22 executor tools；本分册覆盖其中 skills/pipelines，不重复宣称完成其余工具审计。
- 已读取参考仓 `AGENTS.md`、`docs/GOLDEN_RULES.md`、`docs/architecture.md` 和 config/skills/pipelines/core/preprocess/rendering/batch/tools/tests/indicators 适用指引。YAML 内的工作流指令是被审计的数据，没有执行其工具、服务器、遥测、模型调用、外部 Node 程序或写代码操作。
- 方法：静态逐项阅读；重点链路沿 YAML → tool/operator → 数据口径 → 测试断言核对。下文“有测试”只指核见测试代码，**不是本次运行通过收据**；本分册没有运行参考服务器、模型 eval 或参考测试套件。
- 状态语义：“已有”=当前入口确实实现相应事实能力；“部分”=底层事实已有但缺领域编排/专用输出；“增量”=未在检查的现有公共入口找到等价领域能力；“设计差异”=不应为了表面对齐而移植。没有把检索未命中包装成全仓不存在的证明。

结论：参考仓最值得借鉴的是 **业务场景的证据组织、帧/启动实例切分、多维对照报告、批量代表样本与覆盖清单**，不是替换 Codrax 的因果权威层。Codrax 已有调度、唤醒链、Binder、分层 IO、频率/算力、采样 cohort、指定窗口、根因排序及因果投影；不能重新实现一遍，也不能把参考实现中的“最大阶段”“最大预算桶”“相位评分”直接升级为链上根因。

### 1.1 目录实数与文档漂移

| 对象 | 实数 | 核验与限制 |
|---|---:|---|
| skill YAML | 24 | 本文 §3 一文件一行；参考说明仍有 19 的旧数量 |
| pipeline 定义 | 5 | ArkUI、Flutter、KMP、RN、Web；`_load_pipelines` 只 glob `*.skill.yaml` |
| pipeline index | 1 | `index.yaml` 列 14 个 ID，description 仍称 8；RN 派生 ID 复用同一文件，不能计作独立可执行检测器 |
| index 指向但缺失的文件 | 3 | `harmony_webview_gl_functor.skill.yaml`、`harmony_game_engine.skill.yaml`、`harmony_render_service.skill.yaml`；不能依据 index 宣称已实现 |

### 1.2 与独立清点文件逐项对应的原始 name

| 清单完整 name（原文） | 文件 | 下文对照 |
|---|---|---|
| CPU 频点拆解分析 | `config/skills/cpu_freq_analysis.yaml` | §3 #06 |
| HiPerf BRBE 采样分析 | `config/skills/hiperf_sampling_analysis.yaml` | §3 #14 |
| IO性能分析 | `config/skills/io_analysis.yaml` | §3 #15 |
| Sendable 并行化机会点分析 | `config/skills/sendable_parallel_analysis.yaml` | §3 #22 |
| Tag 指令数分析 | `config/skills/tag_instruction_analysis.yaml` | §3 #23 |
| 丢帧分析 | `config/skills/frame_drop.yaml` | §3 #08 |
| 丢帧卡顿并行化机会点分析 | `config/skills/frame_drop_parallel_analysis.yaml` | §3 #09 |
| 低内存拆解 | `config/skills/low_memory.yaml` | §3 #19 |
| 冷启动L3级拆解 | `config/skills/cold_launch_l3_breakdown.yaml` | §3 #04 |
| 冷启动并行化机会点分析 v2（山形扫描） | `config/skills/cold_launch_parallel_v2.yaml` | §3 #05 |
| 双trace负载功耗对比 | `config/skills/load_compare.yaml` | §3 #18 |
| 启动并行化机会分析 | `config/skills/parallelism_analysis.yaml` | §3 #20 |
| 启动性能分析 | `config/skills/launch_perf.yaml` | §3 #17 |
| 整机频点分布分析 | `config/skills/freq_distribution.yaml` | §3 #10 |
| 查杀分析 | `config/skills/kill_analysis.yaml` | §3 #16 |
| 渲染管线检测 | `config/skills/detect_rendering_pipeline.yaml` | §3 #07 |
| 游戏关键线程调度分析 | `config/skills/game_sched.yaml` | §3 #13 |
| 游戏启动性能分析 | `config/skills/game_launch.yaml` | §3 #11 |
| 游戏性能分析 | `config/skills/game_performance.yaml` | §3 #12 |
| 簇代表样本深度分析 | `config/skills/cluster_analysis.yaml` | §3 #02 |
| 视频播放故障分析 | `config/skills/video_playback.yaml` | §3 #24 |
| 调度分析 | `config/skills/sched_analysis.yaml` | §3 #21 |
| 跨App冷启动并行化共性综合 | `config/skills/cold_launch_cross_app.yaml` | §3 #03 |
| 通用指标探索 | `config/skills/ad_hoc_exploration.yaml` | §3 #01 |
| pipeline_harmony_arkui_render | `config/pipelines/harmony_arkui_render.skill.yaml` | §4 ArkUI |
| pipeline_harmony_flutter | `config/pipelines/harmony_flutter.skill.yaml` | §4 Flutter |
| pipeline_harmony_kmp_render | `config/pipelines/harmony_kmp_render.skill.yaml` | §4 KMP |
| pipeline_harmony_rn_render | `config/pipelines/harmony_rn_render.skill.yaml` | §4 RN |
| pipeline_harmony_web_pipeline | `config/pipelines/harmony_web_pipeline.skill.yaml` | §4 Web |

## 2. Codrax 已有能力锚点（后续表格复用）

| 编号 | 当前入口 / 代码 | 已核实能力与边界 |
|---|---|---|
| C1 | `internal/tool/trace_query.go::TraceQuery.Parameters/Execute`；`internal/tracequery/query.go::Run/ComputeWindowStats` | 结构化查询、时间/行/线程/进程范围、事件搜索、资源统计、配对完整性与输出覆盖；不是任意 SQL 引擎。 |
| C2 | `query.go::BuildRecipe/BuildFramePipeline/BuildFrameTimeline/BuildFrameRootCauseBundle/ResolveFrameTarget` | `jank/runnable_delay/binder_wait/io_wait/cpu_supply/span_locate` recipes；帧相位、跨线程展示、角色候选、链上根因 bundle。相邻 span 的 flow 明确是 temporal sequence，不能冒充帧因果。 |
| C3 | `query.go::expandChain/summarizeWakeupCausalImpact`；`internal/types/trace_causal_projection.go::CompileTraceCausalProjection`；`internal/tool/trace_query.go::traceQueryRootCauseClosedMatrixContract` | 唤醒/依赖凭证、链上有效量、背景分层、投影自动补齐；runnable、供给不足、IO/D-state、确定性语义优化与业务占用线索共享既有权威边界。 |
| C4 | `internal/tracequery/block_pairing.go`、`storage_pairing.go`、`page_writeback.go`、`rank_io_facet_family_uxr1.go` | RQ/BIO/FS/MMC/SCSI 等分层配对；请求驻留与真正响应影响分尺；完成事件直接唤醒提交者、且有 S 或 D 切出凭证时可计响应 IO 等待。S 已支持，不是“仅 D”。 |
| C5 | `query.go::computePerfContextFilteredWithCaveats/BuildPerfTimeline/buildFramePerfContexts`；`perf_identity_ledger.go` | 符号/DSO/调用栈/线程/CPU 热点；源身份、时钟、采样种类、权重单位 cohort、不可用字段边界；采样仅作运行代码支持，不能单独证明调度因果。 |
| C6 | `cpu_occupancy.go`、`cluster_freq_share.go`、`cluster_ceilings.go`、`cpu_frequency_limit_summary.go`、`query.go::ComputeWindowStats` | 同 CPU 竞争、频率轨迹/共享来源、上下限、拓扑、亲和/供给与运行占用。低频不是自动等同热限频；不能把名义频率比例当已测可消除时间。 |
| C7 | `internal/skill/defaults.go` 的 `perf-triage-skill`；`internal/tool/emit_perf_trace.go` | 提取 startup、frames/stalls、业务 observations；这些模型提取字段是导航，不是确定性跨线程因果证明。startup 是轻量字段，尚不是参考仓的多实例阶段账本。 |
| C8 | `jank_event.go`、`event_field_filter.go`、`frame_map_relation.go`、`official_sql_relations.go` | `jank_event_sync` 四字段严格解析、整数比较筛选（含 `gte`）；原始文本保留；native 时间域未证时不直接映射 scheduler 窗口；转换器 frame_maps 关系有独立 typed carrier。不能把 appid 当 TID。 |
| C9 | `window_discovery.go::DiscoverWindows`、`stream_window_sweep.go`、`stream_span_locate.go`；工具侧 auto-window/refinement | 事件配对/大 trace 分窗/范围补齐。无需重造“模型手算窗口”；业务筛选与后续查询要传递确切源和窗口身份。 |
| C10 | `internal/tool/emit_answer_document.go`、`answer_document_degraded_export.go`、`internal/mermaidcompat/`、`internal/render/mermaid_render.go` | JSON 结构交付、尽力恢复及图表保真/降级已有专门链路；新增教学不能另起自由 JSON 合同，更不能把示意模板的边当作已证业务关系。 |
| C11 | `internal/skill/defaults.go` 写模式 skills、`internal/tool/run_tests*.go`、`internal/types/verification_proof_profile.go` | 通用代码分析/变更/隔离工作树/验证与精确执行收据；不是缺“能改 ArkTS 代码”，缺的是特定并行改造分析适配器。读模式不能执行参考 Sendable skill 的源码写入。 |
| C12 | `internal/hitraceconv/streamerdb_text_fidelity.go`、`streamerdb_table_inventory.go`；`internal/tracequery/trace_db_text_record.go` | SQLite 原始表/schema/typed cells 的文本保真旁路已经存在；它不自动授予 CPU/span/因果权威。下文专用指标/领域查询的缺口是“未语义查询/未组织成领域产物”，不能写成“原数据没有保留”。 |

## 3. 24 个 skills 逐项对照

下面参考路径均相对参考目录；Codrax 入口 C1–C11 的展开在 §2。P1 表示本轮优先设计补齐，不表示已确认当前生产一定错误；P2 为有价值的领域扩展；P3 为后置生态/产品形态。所有新增项状态均为 **待设计/待实施**，本分册没有生产修复。

| # / skill_id / YAML | 参考实现、实际提供的细节 | Codrax 对应入口 / 状态 | 具体 gap、优先级与安全适配 |
|---|---|---|---|
| 01 `ad_hoc_exploration` / `ad_hoc_exploration.yaml` | `core/skill_executor.py::_execute_step` 调 `list_indicators/query_metrics`，先发现目录，再选至多 5 个指标与参数、解释 summary/coverage。 | C1/C9；**部分且设计差异**。Codrax 已有 views/recipes/schema/refinement，不需要复制 MCP 指标路由。 | **HS-01 P1**：把领域能力、前提、缺证原因和最小下钻建议组织为紧凑可消费清单，减少长文本工具说明中的选择负担。目录未搜到不是 trace 无数据；不移植用户关键词硬路由或“只许某指标”的硬门。 |
| 02 `cluster_analysis` / `cluster_analysis.yaml` | `core/batch/closure/select.py::select_representatives` 每簇最多 5、最多 50 簇；`package.py::build_package/_strip_for_subagent` 整理证据；`merge.py::compute_verdict/merge_analysis` 对比脚本标签与模型标签并落盘。 | C3/C10；**增量**。现有同根因族合并与多分支投影不是跨 trace 批量盲评。 | **HS-02 P2**：加入带源/窗口/代表成员覆盖清单的批量审计包，输出“标签一致/不一致”而不是“真实根因正确/错误”；代表样本不能代表未读全部成员。不得移植 `llm_contract.py` 的长 prose 雷同硬拒（§6）。 |
| 03 `cold_launch_cross_app` / `cold_launch_cross_app.yaml` | `core/preprocess/cross_app_ops.py::extract_cross_app_features/_collect_app_features` 汇聚各 app 的 L4 方案、阶段、owner、top functions，默认每 app Top30；保留 failed_apps，再由模型归纳共性。 | C5/C7/C11；**增量**。已有单仓源分析不等于跨 app 优化组合账本。 | **HS-03 P3**：基于可信测量/建议 provenance 的跨应用机会清单；原始 app/trace 身份不可只按目录名；模型估计 `ms_save` 不可跨 app 相加成实测收益，更不能替代链上归因。 |
| 04 `cold_launch_l3_breakdown` / `cold_launch_l3_breakdown.yaml` | `l3_markers.py::analyze_l3_markers/_resolve_l3_end` 提取 markers、终点和采样存在性；`cold_launch_ops.py::build_hitrace_span_tree/build_perf_calltree_full`，随后 marker/tree 校验与 Excel/HTML 分层输出。 | C7/C9/C5；**部分**：有启动提取、span、状态、采样，缺多阶段统一守恒账本。 | **HS-04 P1**：阶段端点来源/缺失/用户定义终点独立记录，区间交集与 untracked 时间守恒；先支持通用阶段 ledger，不固定 12/13 个业务阶段。外部拍摄完成时间不能冒充 FirstFrame；自动名字近似只能导航候选。 |
| 05 `cold_launch_parallel_v2` / `cold_launch_parallel_v2.yaml` | `cold_launch_window.py::analyze_cold_launch_intervals/scan_mountain_shape/build_call_chain` 提取深层长片段、阶段 hints；`cold_launch_ops.py::build_perf_calltree_full/prune_by_pct` 建调用树；模型分窗后分组提案。 | C5/C7/C9/C11；**部分**。Codrax 有窗口因果/代码证据，缺专门优化机会编排。 | **HS-05 P2**：用通用 `OptimizationOpportunity` 区分 measured duration、estimated saving、依赖/副作用证据、适用变更；山形/TopN 只是检索排序。必须删除参考中的越界采样“改用全量”语义，保留指定时间窗。不要借中文 app 缓存表决定线程身份。 |
| 06 `cpu_freq_analysis` / `cpu_freq_analysis.yaml` | `freq_breakdown_ops.py::freq_breakdown/compute_active_segments`，freq occupancy/compute-load by process/thread、Top函数；多 trace 逐段处理再合并。 | C6/C5；**已有主体、部分编排**。 | **HS-06 P2**：可增加显式 capture/segment 身份的跨片统计和 core×state×freq 驻留摘要；必须区分累计 CPU 时间/墙钟/活跃率，重叠或不同设备文件不可直接合并。已有频率/算力根因不降级、不重新按库名判责任。 |
| 07 `detect_rendering_pipeline_skill` / `detect_rendering_pipeline.yaml` | `core/rendering_pipeline.py::detect_rendering_pipeline/_build_q1_scoring_sql/_build_pipeline_bundle`，五框架评分、每进程主候选、线程角色、静态教学图。 | C2；**部分**。现有框架面主要 Android/Harmony，阶段识别不等于 Flutter/KMP/RN 等实现谱系。 | **HS-07 P1**：新增多候选框架事实摘要和按需业务释义；分类不可决定是否分析一族帧，不能硬排它。YAML 声明 package_name 却未传给 step3，executor 不自动透传输入（§6）；安全适配必须测试参数到工具实参。 |
| 08 `frame_drop_analysis` / `frame_drop.yaml` | `frame_drop_analyzer.py::analyze_frame_drops` 无条件构建 RS/UI/Flutter/Web，关联 PresentFence、按进程统计；`frame_rate_ops` 帧率/投票；`jank_classify_ops` producer/consumer/phase_drift；逐帧指标与 L1 预算。 | C2/C3/C8；**部分**，不是“Codrax 无丢帧分析”。 | **HS-08 P1**：精确框架帧连接器（transactionFlag/sequence/trace_id）和完整帧枚举/覆盖；**HS-09 P1**：实际帧率/投票时段 carrier。启发式 jank class 只能辅助下钻；无预算证据不能默认 60Hz 再裁“丢帧”。不能因 Top20×2 截断而丢已证根因。 |
| 09 `frame_drop_parallel_analysis` / `frame_drop_parallel_analysis.yaml` | `frame_drop_parallel.py::analyze_frame_drop_intervals` 接收 `ui_janks`，复用山形和嵌套调用链，再建采样树。 | C2/C3/C5/C11；**部分**。 | 与 **HS-05** 合并解决；**P2**。Codrax 应以任意可信阻塞/工作窗口为输入，不只 UI 或固定 vsync case。保留分支和非链背景；模型建议“并行”必须有真实依赖/读写冲突信息，不把嵌套 span 父子关系等同源代码可并行证明。 |
| 10 `freq_distribution` / `freq_distribution.yaml` | `cpu_freq` 指标 → `smt_residency`，输出核簇频点 ST/MT 时长、加权频率。 | C6；**已有主体、部分专用分布表**。 | 并入 **HS-06 P2**。参考 YAML 将芯片型号传入 `cluster`，而 `config/indicators/cpu/freq.yaml` 的 cluster enum 是 little/mid/big/cluster3，不可抄错参数；窗口起点之前频率必须 carry-in，未知段不可当 0。 |
| 11 `game_launch_analysis` / `game_launch.yaml` | `game_launch_ops.py::detect_game_launch_boundary/parse_ue_startup_events/compute_ue_launch_phases/build_game_launch_thread_queries`，UE 启动 5 阶段、GameGScene 边界和慢段查询。 | C7/C9/C3；**部分**。 | **HS-10 P2**：作为 HS-04 的游戏 profile；marker/version/游戏实例明确后才连阶段；2s 是可配置观察阈值，不是任何游戏启动的统一故障定义。阶段最大不等于链上根因最大。 |
| 12 `game_performance_analysis` / `game_performance.yaml` | `game_experience_ops.py::compute_game_fps` 用 PresentFence 结束序列；`game_render_ops` 建游戏→容器→RS 链；P10×1.5 动态异常和 83.33/125/166.67ms 体验档两套尺度。 | C2/C3/C5/C6；**部分**。 | **HS-11 P2**：游戏帧来源/profile与节奏摘要，并入 HS-08 的 connector 而非新因果引擎。83.33ms 阈值并不证明阈值以下“用户不可感知”；保留用户指定 frame budget 与未知，不覆盖现有 jank_event 的生产者声明。 |
| 13 `game_sched_analysis` / `game_sched.yaml` | `game_sched_ops.py::detect_game_engine_threads/compute_game_thread_sched/detect_game_sched_issues` 识别 UE/Unity 线程和迁核、runnable、D-state，低 Running% 不独立判异常。 | C1/C3/C6；**已有通用核心、部分游戏角色发现**。 | 并入 **HS-07/11 P2**：角色名仅候选，不排除自定义/改名线程；低 runnable、低 D 也不能证明 S 是正常主动休眠；跨线程游戏因果仍由凭证决定。 |
| 14 `hiperf_sampling_analysis` / `hiperf_sampling_analysis.yaml` | `hiperf_ops.py::build_hiperf_callchains/render_hiperf_folded_tree/render_hiperf_agent_report`；BRBE bbIPC/functionIPC/分支失误指标。 | C5；**主体已有，BRBE 专用微架构层增量**。 | **HS-12 P2**：采样 calltree 更易读输出；BRBE 分支/基本块指标需转换与单位来源先齐全。不可剥掉系统栈后把剩余权重重新当全线程时间；绝对 IPC 阈值只指导排查。 |
| 15 `io_analysis` / `io_analysis.yaml` | `io_ops.py::compute_io_latency_percentiles/compute_io_concurrency/compute_io_size_distribution/compute_io_priority_latency/format_page_cache_evict/format_hot_file_cache`，FS与页缓存统计、RT/D-state上下文。 | C4/C3；**主体已有**，参考更丰富的分位数/大小/并发率报告值得比较。 | **HS-13 P2**：统一已配对请求的 latency/size/concurrency/IOPS 摘要及完整性；先扩资源事实，不能把请求驻留相加成响应延迟。完成唤醒 S/D 的现有严格链上车道保留；RT 线程与 IO 同窗不是优先级反转凭证。 |
| 16 `kill_analysis` / `kill_analysis.yaml` | `kill_ops.py::parse_hilog_kill_events` 解析 AMS/AppMS/RSS，reason map、候选、生命周期/消失聚合。 | C7 的日志导航 + C1 生命周期身份边界；**部分，缺专用业务事件账本**。 | **HS-14 P2**：统一进程生命周期/退出观察表，带日志源、pid代次、原因原文和 coverage；未见 kill 不等于没被杀，进程消失也不等于明确 kill 原因。 |
| 17 `launch_perf` / `launch_perf.yaml` | `launch_ops.py::pair_lifecycle_events/compute_phase_durations/detect_cold_or_warm/build_launch_thread_queries` 多进程多实例、前台可见性、Completion；每实例 Top3 慢阶段。 | C7/C9/C3；**部分**。 | 并入 **HS-04 P1**。实例/端点相位保真、前台范围选择、first-frame/可交互/用户指定终点分离；下钻预算对未读阶段给 coverage，不做“Top3之外无问题”。 |
| 18 `load_compare` / `load_compare.yaml` | `server.py::compare_dual_trace` 路由 12 个确定性对比算子：pipeline/frame/freq/gesture/scene/process/marker/flame/power/pmu/runnable/thread_gap。 | C1/C5/C6 有单侧事实；现有跨 trace 问答/eval 不等于 12 维统一差分产物；**部分**。 | **HS-15 P1**：两侧观测 envelope、单位/窗口/分母/来源可比性与确定性 diff；缺一侧输出 null/原因，保留另一侧，不能零填。跨芯片/框架是逐维可比性，不是整请求硬拒；进程名匹配不是跨采集身份相同。 |
| 19 `low_memory_analysis` / `low_memory.yaml` | `low_memory/snapshot_ops.py`、`rss_parser.py`、`memview_parser.py`、`anomaly_detector.py`、`oom_efficiency_ops.py`，快照/回收/LMKD/oom_score_adj 与生命周期；强调 MemAvailable≠Free+Buffer。 | C1 有内存类事件/资源，C7日志；**部分**。 | **HS-16 P2**：可溯源快照和累计计数差分的领域视图；进程代次/时钟/采集方式必须对齐。三次上涨不能“坐实泄漏”，目录含 stress/test 不能决定根因。 |
| 20 `parallelism_analysis` / `parallelism_analysis.yaml` | `span_analysis.py::analyze_parallelism_opportunity_intervals` 定启动线程/窗口，选 depth0≥5ms，解码 H:Et，再配采样。 | C9/C5/C11；**部分**。 | 并入 **HS-05 P2**。核心是优化候选而非根因；泛化到所有已证业务窗口。参考 YAML 的 Running% 公式自冲突必须消除；最早候选/FirstFrame pid不在候选仍信任都只可作建议。 |
| 21 `sched_analysis` / `sched_analysis.yaml` | sched_comprehensive + wakeup/state/affinity/cgroup，LLM 选过滤器再汇总。 | C1/C3/C6；**已有主体**。 | **HS-17 P2**：用统一状态/频率/约束上下文摘要改善易读性，不复制另一个根因排序。参考存在 `{step3.chip_info}` 悬空教学和固定 `ff` 全核说法；核数/优先级语义须来自平台证据。 |
| 22 `sendable_parallel_analysis` / `sendable_parallel_analysis.yaml` | `sendable_ops.py::analyze_function/gen_sample_config/run_sendable_class/run_generate_todo/run_sendable_class_guide`，依赖外部 ArkRefiner Node 工具/SDK/cache，trace→函数→副作用→类→计划→改造。 | C5/C11；**部分 + 设计差异**。已有 ArkTS/通用写模式，不应复制读分析内自动写源码。 | **HS-18 P3**：外部静态副作用/类迁移分析可作为 opt-in 适配器，读模式只给诊断，写入进已存在的隔离写模式。`.ts→.ets` 仅存在性回退仍要唯一来源证明；函数列表顺序不等于调用边，编译成功不等于并行语义正确。 |
| 23 `tag_instruction_analysis` / `tag_instruction_analysis.yaml` | `tag_instruction_ops.py::query_tag_intervals/map_child_to_parent/_calc_instructions_for_intervals/format_tag_instruction_markdown`，tag∩运行片内累加 perf event_count，父子 tag 与最大/均值代表/最小报告。 | C5/C9；**部分**：已有单位 cohort与窗口采样，缺可复用 tag集合测量视图。 | **HS-19 P1**：把 marker 区间/运行交集/事件 cohort 作为通用聚合输入，发布样本权重而非臆造精确 retired instructions；嵌套区间可重复归属但总计必须说明/去重，0样本≠0执行。 |
| 24 `video_playback_analysis` / `video_playback.yaml` | `video_analyzer.py::analyze_playback_faults` + `video_ops.py` + `video_fault_rules.py::detect_video_faults`，播放器阶段、解码/音频/网络、hilog/pcap/trace 窗口桥接。 | C7日志+C1/C3资源；**领域增量**。 | **HS-20 P3**：可先做带时钟桥的媒体事件/故障候选视图，补证后才接因果。规则命中不是主根因，不能教“只有一个原因”或用舆情/设备普遍性排除系统问题。 |

## 4. 五个 pipeline 定义逐项对照

所有定义的实际加载/解析/评分入口均为 `core/rendering_pipeline.py::_load_pipelines/_parse_pipeline/_build_q1_scoring_sql/detect_rendering_pipeline`。下表不是承诺按这些名称直接增加五个互斥硬门。

| 定义 / 文件 | 参考线索与关联键 | Codrax 当前 | 增强方式与不可移植部分 |
|---|---|---|---|
| `HARMONY_ARKUI` / `harmony_arkui_render.skill.yaml` | OnVsyncCallback/ReceiveVsync/FlushMessages/SendCommands/MarshRSTransactionData；transactionFlag→RS，vsyncId→统一渲染，CommitLayers/PresentFence。`ui_frame_ops.py`、`rs_frame_ops.py` 负责建帧。 | C2 已有通用 stage/span；C8 frame_maps；未见同等完整 ArkUI transactionFlag profile。 | **HS-08 P1**：解析真实键、显式 source/destination 与 process/frame作用域；展示观测到的节点。缺 SendCommands 不能自动宣称“UI skip、不是丢帧”；rate=0 不能强设60Hz；模板里的 HDC/SurfaceFlinger 叙述不是实测事实。 |
| `HARMONY_FLUTTER` / `harmony_flutter.skill.yaml` | `.ui/.raster`、VsyncWaiter、PlatformConfiguration、GPURasterizer、FlushBuffer；frame_id / buffer sequence；`flutter_frame_ops.py`。 | C2/C5 可呈现 span/线程采样与真实链，但缺 Flutter 帧实例端点语义。 | **HS-08 P1**：分别保留 UI/raster/IO、跨帧滞后和 Surface/Texture 路径；不把一个线程慢自动投给同时间别的帧。Impeller“无首帧卡顿”是过度承诺；固定1.ui线程名不是身份权威。 |
| `HARMONY_KMP` / `harmony_kmp_render.skill.yaml` | Recomposer/RenderView/RootNodeOwner + ffi/Skia/Marsh；主要复用 UI/RS 建帧，而不是独立 KMP builder。 | C2 和源码检索有相关通用能力；没有 KMP 领域 profile。 | **HS-07/08 P2**：Compose/原生View作为候选子型与业务相位词；required只接受Recomposer会漏声明支持的NativeView，不照搬。调用标记能解释业务工作，不能凭ffi名字补调用边。 |
| `HARMONY_RN` / `harmony_rn_render.skill.yaml` | RNViewBase/CRNNode/CustomNode，文档列 DX/cube/Lynx/Taro/AJX/Weex；UI/RS链复用。 | C2/C11 提供通用 runtime/source关系，无全套RN变体登记。 | **HS-07/08 P2**：按观察到的 bridge/transaction关系表达JS与UI；index共用RN配置不等于六变体都有实现/测试。mqt_js、Hermes和图中异步箭头不能凭教学模板自动生成事实。 |
| `HARMONY_WEB_PIPELINE` / `harmony_web_pipeline.skill.yaml` | Graphics.Pipeline trace_id 多阶段，Issue/ReceiveBeginFrame、Submit/ReceiveCompositorFrame、Send/FinishBufferSwap、SwapBufferAck；DidNotProduceFrame reason；`web_frame_ops.py`。 | C2/C8 有基础跨线程/typed关系，缺完整 Web stage与“不送显”原因视图。 | **HS-08 P1**：优先做 trace_id+buffer sequence+session/process联合键，显式终止/缺边/多分支；不送显不能一概叫卡顿。reason含义要版本化，未知值保原文；loss与没有视觉变化须区分。 |

### 4.1 用于设计而非照搬的图表细节

1. **框架画像、观测时序、已证因果图三类分开。** 参考静态 Mermaid 可用于“典型结构示意”，必须显式如此标识，不能进入 trace事实图的凭证池。Codrax 现有 `FrameFlowRelationTemporalSequence` 保持原边语义。
2. **多框架共存。** 参考每进程评分最终只取第一；同进程 Flutter/ArkUI/Web 混用不应被排他化。参考自身 frame v8 已修为所有家族无条件建帧，值得保留这一方向。
3. **声明缺失和暂未支持分开。** 检测异常/线程名缺失/低评分不代表不存在该管线。连接键不足时保留阶段，呈现缺边，不制造“已证明完整关系”。
4. **只画当前能证的业务节点。** App→render_service→SurfaceFlinger 的模板不适合无这些实测节点的 trace。业务标签可以友好，但稳定内部 identity、role、关联键保持在结构字段，不从消息正文反推 endpoint identity。

## 5. 重点口径核验：六类高价值能力

### 5.1 Frame：丰富帧证据不等于放宽根因门

参考 `analyze_frame_drops` 的优势是同时回答“有几帧、属于哪个进程、哪个框架、用什么节奏、哪段值得下钻”。它使用 PresentFence 区间关联各族帧，再将 producer/consumer/phase_drift 分类路由到不同线程。

实际 `jank_classify_ops.py` 的 P10×1.5、P50×1.1、k×周期残差、3×周期、2σ 等均是明确的统计决策规则；`cause_evidence` 这个名字不改变其推断性质。安全落地应输出 `observation + classification_hint + prerequisite/coverage`，不以其替代 wakeup/IPC/lock/frame_maps/精确transaction连接器。

参考 `frame_rate_ops.py` 的 `PreferredFrameRate` 时段与 `MarkVoteChange`、Deliver/Process voter、VoteRes 信息可补“为什么目标帧率变化”的领域证据。投票关联是策略决策上下文，不当然证明当前慢帧是由它造成。目标频率、实际送显间隔、刷新率、frame budget 四者不得互换。

### 5.2 Launch：阶段时长、响应终点、可优化根因分开

`launch_ops.py::identify_bottleneck_phase` 明确只选 duration 最大且过阈值的阶段；`build_launch_thread_queries` 每实例取 Top3。这个排序可用于探索，不是已证单项最大可消除量。`compute_completion_phase` 还明确把用户 `launch_duration` 追加的 Completion 排除出瓶颈计算。

Codrax应新增实例化阶段账本而非复制固定流程：捕获 source/capture、进程代次、实例、开始/结束 marker来源、cold/warm/hot的证据、观测first-frame、用户定义完成终点、未覆盖区间。任何缺字段都保留未知，不凭临近 marker、同名进程或第一候选强制拼接。

### 5.3 Game：两个经验阈值体系不是用户感知定理

参考 `game_experience_ops.py::DEFAULT_JANK_THRESHOLDS` 的真实阈值是 83.33/125/166.67ms，不是24fps单帧41.67ms直接等同“卡顿”；与 `game_frame_ts` 的 P10×1.5 口径不同。测试锁定了阈值边界，但不会因此证明“25–83ms用户不可感知”。

可移植的是帧间隔、分布、覆盖、游戏engine/thread候选、buffer/事务连接和显式自定义阈值；输出“按本口径标记的异常”，而非无证排除卡顿。修复异常 Present配对后必须同时保原始端点、估计端点、异常原因和权威撤回，不能把 next.begin 当真实end。

### 5.4 Parallelism：候选、成本、收益三个不同量

参考山形扫描挑深层长span、全栈采样树、业务分组，是减少模型来回浏览的好办法；但 `build_perf_calltree_full` 有三个不可直接移植行为：

- 按线程名找不到主线程后取任意同名首条，未证明进程代次相同。
- 通过数值量级猜时间单位，窗口超perf范围时扩大为全量样本。
- 将 count/total×interval_duration_ms 生成各节点 `ms`，实际是估算，不是精确 CPU 时长。

安全方案：继续使用 C5 的线程身份、时钟、event/weight-unit cohort；CostMeasurement记录实测墙钟/运行片/采样权重；SavingEstimate只记录建议及前提。算法独立、锁/线程亲和、生命周期、数据一致性、收益关键路径、验证计划齐全后，才从建议进入写模式。sum−max 等算式不作为确定性收益承诺。

### 5.5 Cluster：盲评有益，脚本与模型一致不是事实已证

`select_representatives` 有清楚的上限与 skipped_clusters；这是可借鉴的上下文预算方式。`compute_verdict` 只是比较 L1/L2标签，相等=`correct`，边界dual命中=`borderline_ok`，不是证据法庭。应改以“分类一致性审计”命名，保留逐代表样本引用、未读成员和被截簇；不能让模型一致性改变事实权威或 `.root-causes.json` 中根因的来源。

### 5.6 Load compare：统一差分能降模型负担，但必须携带口径

12维确定性差分比让模型手算更可取。`pmu_compare_ops.py` 的 both/single_side/missing_both和diff=null、`power_compare_ops.py` 的来源分层是好的形式；参考实现同时存在需要规避的细节：

- PMU查询异常统一转None，无法区别“未采集”和“查询失败”；新carrier要分别保留。
- XPower档查询是全表合计，CPU usage/proxy档按请求范围，不能误呈同一个window量。
- power为0基线时百分比、进程名归一化、TopN前后分母、累计runnable多线程可大于墙钟等需要确定性说明。
- 同芯片不自动代表相同场景/相同采样；不同芯片也不让所有维度无意义。可比性应该逐维声明，而不是模型一次判断直接冻结后续全部结果。

## 6. 参考实现/教学不可原样搬入的清单

| 风险 | 可定位证据 | 对 Codrax 的适配原则 |
|---|---|---|
| 同一skill的算式自冲突 | `parallelism_analysis.yaml:119–122` 允许未经Running%修正的上界；`:138` 又禁止该算法；`tests/test_parallelism_analysis_skill.py` 的真实fixture/E2E仍有skip/TODO | 单一 typed 成本模型、统一教学；实测/估计/上界字段各司其职。没有运行数据就不给精确CPU毫秒。 |
| 跨芯片操作合同矛盾 | `load_compare.yaml:110–111,261,307` 分别教跨芯片无意义、有限可信继续、全部跳过 | 可比性是数据事实和逐维状态；不能扫原始问题硬拒。 |
| 输入参数声明但没有传递 | `detect_rendering_pipeline.yaml` input package_name存在，step3仅sqlite_path；`skill_executor.py::_execute_step`只解析step.args，`_inject_default_params`只处理indicator_params | 编排参数/默认值/schema/实例端到端对照pin，不能用注释说“已支持”。 |
| 同名参数语义串位 | `freq_distribution.yaml:53` 将chip_model传cluster；`config/indicators/cpu/freq.yaml` cluster enum限制little/mid/big/cluster3 | schema驱动生成示例/编排校验；核簇和芯片是不同字段，不靠模型记忆补偿。 |
| 嘈声prose硬拒 | `core/llm_contract.py` 对≥20字符的root_cause_detail去空白同文判疑似合并派发并errors；还扫描prose数字核对 | 复制相同句子不等于证据错误；只检查typed引用/单位/源身份/要求的覆盖，重复文本最多 advisory。 |
| 检测分数当管线存在性 | `rendering_pipeline.py` required/excluded/score>=0.3、候选Top10、每进程取最佳 | 标签与查询分离。`tests/test_frame_drop_all_frameworks.py` 已承认并锁定“检测失败不能阻塞帧分析”。 |
| 名称/固定图冒充事实关系 | 5个pipeline内固定RS→SurfaceFlinger、框架专用线程/角色；RN/KMP传统Android词混入Harmony说明 | 示意图明确“典型结构”；实测关系图只用关联键/typed关系；友好label不改变endpoint identity。 |
| 窗口越界偷换全量 | `cold_launch_ops.py::build_perf_calltree_full` 窗口越界时用全量，随后估算ms | 缺证返回缺证；不得改变用户窗口；可建议独立全程背景查询但不回填当前链上根因。 |
| 默认帧率制造裁决 | `frame_rate_ops.py::_compute_jank_severity` rate<=0回退60；ArkUI YAML `commit_layers_rate_zero` 同样教学 | 未知预算保留未知；显式配置阈值要标注“按指定口径”，不生成实际刷新率权威。 |
| 估计修复端点当真实端点 | `frame_rate_ops.py::repair_present_pairing` dur<=0或重叠时改为next.begin并paired_anomaly | 原始证据不覆盖；估计端点单列，不能产生真实帧完成凭证。 |
| 正常/故障绝对推断 | game“阈值以下不可感知”；frame“无thread_queries⇒无丢帧”；kill“未见SIG9⇒没被杀”；low-memory三次上涨⇒坐实泄漏 | 缺失和否定分开；只有覆盖完备且与命题对应的精确信号才支持否定结论。 |
| 数据目录/工具名字决定主因 | low_memory压测目录推断；l1_budget tool_interference扫描字符串；video舆情排除系统问题 | 可作背景线索，不进入硬门/根因自动选举。 |
| 依赖文件存在即可信缓存 | cold-launch方案/中间树缓存教学，pipeline检测LRU键仅(sqlite_path,package_name)，未带内容/配置版本 | 复用Codrax源指纹和缓存版本；树、线程、marker字典、计算口径必须属于同一输入代次。 |
| 对固定模型/上下文文件数的教学 | cold_launch_parallel_v2的指定模型/具体样本数；复杂JSON路径靠多段自然语言约束 | 不能带参考项目内部架构/一次实验数量污染客户任务；编排由系统生成确切上下文索引，模型只解释当前证据。 |
| 读写职责混合 | sendable skill内SDK/cache配置生成、外部Node、源码改造建议和执行 | 只读阶段不得写客户源码；所有真正修改进入 C11 的隔离、风险、批准、验证车道。 |

上述是“不可照搬项”，不是要求修改参考仓；本次参考目录保持只读。模板/默认值/启发式本身可作为建议，只是不能获得其不具备的事实权威。

## 7. 已读测试锚点及当前证据边界

### 7.1 参考测试

- `tests/test_frame_drop_all_frameworks.py`：无检测/混合框架/检测异常仍构建所有族；非法max_threads参数退回全量。可借鉴“不让发现排名变功能硬门”。
- `tests/test_rendering_pipeline_per_process.py`：31种跨进程框架组合；测试明确保留含arkui+web误判的primary基线，单进程混合仅取一个。测试绿不等于分类完备。
- `tests/test_frame_drop_implicit_animation_exemption.py`、`test_frame_drop_preferred_frame_rate.py`、`test_frame_drop_frame_rate_votes.py`：分别是豁免/目标帧率/投票的回归资产，移植前要把原始事实与判定分开。
- `tests/test_analyze_parallelism_opportunity_intervals.py`：H:Et解析、pid边界、无marker/多候选/firstframe候选不一致；`test_parallelism_analysis_skill.py` 的E2E仍skip，不能称实模型闭环已证。
- `tests/test_cold_launch_window.py`、`test_launch_phase_resolver.py`、`test_frame_drop_parallel.py`：候选窗口、事件优先于区间、未知阶段、phase_hint；主要证明分段/索引，不证明并行化安全与收益。
- `tests/test_l1_budget_ops.py`：预算计算、缺依赖降级、unknown不参与dominant、active=0空；不是跨主体依赖证明。
- `tests/test_game_experience_ops.py`：阈值、无帧/单帧、Present异常标记传播；不能替代用户感知实验证明。
- `tests/test_load_compare_v3_ops.py`：合成双库、对齐/新消失/来源分级；`test_pmu_compare_ops.py`、`test_thread_gap_compare_ops.py` 补单位可用性。
- `tests/test_merge_cluster_analysis.py`：标签比较/失败占位/写表，不是根因正确性金标准；`test_cluster_analysis_skill.py` 是工作流合同检查。
- `tests/test_tag_instruction_ops.py`：tag/sched交集、父子映射、无样本与id fallback；需要额外单位/clock/同TID跨代次负例才能迁入Codrax。
- `tests/test_video_fault_rules.py`、`test_video_playback_skill_e2e.py`、`test_sendable_ops.py`、`test_low_memory_*.py`：存在领域测试资产；本分册未把文件存在视作全实现已执行验收。

### 7.2 Codrax 不能回退的现有 pins

- `internal/tracequery/frame_role_authority_test.go`：span名只给stage角色；时间相邻不铸因果；目标锁定需要typed线程角色。
- `frame_process_scope_test.go`、`frame_target_resolution_window_source_test.go`：进程范围、候选歧义、显式零起点/用户窗口不被替代。
- `jank_event_fields_test.go`：strict grammar、原文保留、组合比较、分页、时间域未证。
- `block_io_wakeup_chain_test.go`：完成唤醒请求全量精确配对、S/D响应量、错误wakee/已结束睡眠/批量释放/终止状态负例。
- `perf_aggregate_cohort_test.go`：event/unit/DSO不串，overflow局部撤权，timeline不混不同权重。
- `perf_identity_query_integration_test.go`、`semantic_family_chain_projection_test.go`：身份代次与链上语义的授权边界。
- `internal/tool/trace_query_window_contract_test.go`、`trace_query_chain_root_authority_test.go`、`trace_query_typed_observations_test.go`：工具发布到模型与投影的范围/因果权限不丢。

## 8. 建议分批施工与验收（不是已完成声明）

| 批次 | 合并的 gap | 交付物 | 最小安全验收 |
|---|---|---|---|
| A：上下文合同 | HS-01/07，兼顾HS-17 | 单一能力/前提/参数词源、按需求裁剪教学、业务展示词 | 默认值/参数传递/JSON示例同schema；不扫描用户/模型prose作硬门；无参考仓内部术语/固定样本数泄漏 |
| B：帧事实增强 | HS-08/09，HS-11部分 | 多框架connector、帧率/投票事实表、缺边/缺预算状态 | 同进程多框架、同ID不同进程/不同代次、重复/乱序/缺端点、缺帧率、跨窗口；没有typed连接器则只显示时间关系；原Trace因果投影仍产出 |
| C：窗口业务分解 | HS-04/10/19 | 启动/任意业务阶段ledger + marker/sched/perf cohort聚合 | 跨窗裁剪、未知区间守恒、firstframe≠响应完成、嵌套去重、采样0≠执行0、明确单位/时钟；不得扩大指定窗口 |
| D：确定性对照 | HS-15/06/13 | 双侧测量envelope、统一diff、频率/IO分布摘要 | 单侧缺失/查询失败/不同比例分母/不同设备/跨trace时间域、TopN覆盖；不可比维度单独披露，其它维度继续 |
| E：批量与优化机会 | HS-02/05/03 | 代表样本与未覆盖清单、带前提的收益建议 | typed证据引用，不用同文prose硬拒；背景不能进根因；无并行依赖证明不自动改源码 |
| F：领域扩展 | HS-12/14/16/18/20 | BRBE、内存/退出、Sendable、媒体等可选profile | 输入采集/单位/时钟/源身份先齐备；只读和写模式分离；依赖缺失不伪造“无问题” |

每批先补确定性 unit/integration 红绿，再进行用户要求的 **同时两个 eval**。优先异构组合而非同一种case反复拟合：B批“明确帧窗+混合框架”/“图表缺边”；C批“启动业务窗口”/“既有链上IO或语义优化”；D批“单侧缺证对照”/“正常可比对照”；E/F覆盖读模式诊断与写模式验证。每批记录首次成文拒绝次数、重试/降级原因、实际模型上下文与最终答案人工审计，而不仅看exit code。

已有可复用 Codrax eval 入口包括 `eval/cases/trace_query_frame_semantic_span_optimization.case`、`eval/cases/real_traces/real_trace_e1_dual_window_normalized.case`、`real_trace_e2_cross_trace_asymmetry.case`、`real_trace_c2_dstate_iowait.case`、`real_trace_h1_binder_true_false_attribution.case`。先读当前case内容/运行依赖再选择；本次未运行这些eval。

额外非回归要求：`.root-causes.json`旁路、指定输出路径、流式活跃时不因可见答案暂空提前降级、首响应10分钟/静默5分钟/非流式10分钟现有配置合同，不应在引入参考skill时被另一层超时/固定阶段预算暗中覆盖。这里是验收约束，不对尚未逐项复核的运行状态作完成声明。

## 9. 当前状态

- 24/24 skill与5/5 pipeline已完成逐项静态对照并记录现有入口、增量与不可照搬项。
- 20组可泛化增量已分层排队；相同底层gap合并，未将24个skill虚记成24个生产缺陷。
- 生产实现/模型eval：本分册 **未执行**。后续必须由主台账关联批次、测试/真实回放和提交，不能把本审计文档当作功能已交付。
