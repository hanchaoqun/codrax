# HarmonyOS 能力差距实施任务清单

日期：2026-09-17，交付状态更新至2026-09-18。来源：[统一审计账本](hmosperf_capability_gap_audit_20260917.md)。本清单落实用户“逐项拆任务、逐项修复、分批提交推送”的要求，是后续实施状态入口；详细实现证据仍留在审计分册，不维护另一份互相冲突的审计结论。18类已拆为79个稳定编号子任务，当前11项实现已交付、68项仍开放（含持续验收任务）；这不是79个已发生的产品故障，包含能力增强和验证工作。

## 1. 范围、状态和交付规则

参考清点：13 个 MCP 工具、22 个工作流工具、24 个 skills、5 个实际管线定义、109 个具名指标。原始名称、源码位置与逐项差异见[机器清单](hmosperf_inventory_20260917.json)、[核心63项](hmosperf_core_metrics_comparison_20260917.md)、[扩展46项](hmosperf_extended_metrics_comparison_20260917.md)、[skills/管线](hmosperf_skills_comparison_20260917.md)。同一底层能力被多个工具/指标复用时只建一份实施任务，不能把别名计为多个缺陷。

- `[ ] 待实施`：尚未施工；`[ ] 部分实施`：部分子缺陷已修，整体退出条件未满足；`[ ] 验收中`：已有代码但尚未取得全部提交/验收收据；`[x] 实现已交付`：代码、公共回归、必要相邻测试、文档及推送已完成。
- “实现已交付”不等于最终模型答案已通过。生产回放单独记入 HMC-18，保留原始机器/人工 verdict；不能用确定性回归倒签原 FAIL。
- 每个子项以其 ID 记录：参考实现位置 → 当前公开路径反例 → 泛化方案 → 正反回归 → 提交 → 推送 → 剩余范围。无行为故障的新增能力用验收前置失败，不伪称已经发生客户事故。
- 每批只完成一组相关子项，不把 18 个大类整体打勾。相邻任务可共用基础组件，但未交付部分仍保持开放。实施时若发现需进一步拆分，保留原 ID 和迁移去向，不删掉原债。
- 新增结构必须同步 schema、容量说明、渲染处置与哈希 pin；原硬门不降低。禁止扫描用户/模型原文作硬门、系统代写结论、邻近事件晋升根因。显式窗、Trace投影/自动补齐、链上业务线索、root-causes旁路和600/300/600秒及活跃流保护均为跨批保护项。
- 真实 eval 按风险、用户影响、泛化面、现有覆盖和成本排序；每批恰好2个并行、各1次，审计过程/上下文/答案/图/旁路。已证模型波动独立留档，不追加单例硬约束或追跑第三例求绿。

## 2. 执行队列

| 顺位 | 实施范围 | 先决条件 / 批次退出点 |
|---|---|---|
| 已收尾 | HMC-02.2、04.1 | 业务/IO事实交接、双窗身份、来源单源的末版全仓回归通过；`a784bc439`已提交推送 |
| 已收尾批A | HMC-17.1–17.3 | `482bfa856`已推送；共享准备服务、完整查询材料与预览分离、CLI真二进制端到端及最终全仓87包通过 |
| 已推送小批 | HMC-01.2/16.4排序教学子缺陷 | `af2e2548d`；实际模型消息已红转绿，skill/agent/tool整包通过；工具字节快照按演进规则核对，不改引擎或放松合同 |
| 当前小批（确定性验收完成） | HMC-01.2参数一致性 | 显式`include_window_stats:false`被默认覆盖已公开红转绿；全仓87包、定向race×3和发布身份pin通过，见主账本§14.1 |
| 下一批B | HMC-17.4–17.5 | REPL可取消生命周期、失败不换旧附件、typed path协调入口；每个入口分别验收 |
| 后续高ROI | HMC-01.1–01.3、08.1–08.3 | 能力/单位目录及完整IO总体统计；沿既有精确配对和覆盖账本，不新增第二查询内核 |
| 后续高ROI | HMC-08.4–08.6、12.1–12.3 | 半开区间统计、GPU观测、动态帧率和精确帧协议；先来源/身份，再统计/业务表达 |
| 业务与对比 | HMC-04.2–04.6、10.1–10.3、11.1 | 实例/窗口/量纲先成立，之后按对象组合和逐维比较 |
| 有依赖领域 | HMC-05/06/07/09/13/14/15 | 先交日志/时钟/设备/资源代次等基础，再做领域统计；不能把缺依赖写成已支持 |
| 贯穿全部批次 | HMC-16、18 | 教学一致性、JSON示例真实执行、上下文与答案人工审计；不是等到最后才验收 |

HMC-03首片及HMC-02/16前置修复已交付，见以下提交记录。排期可因新的可复现P0/P1调整，调整理由写回本节，不能静默遗漏当前任务。

2026-09-18排期调整：HMC-17.1–17.3已收尾；先插入HMC-01.2/16.4的根因排序教学漂移修复和同目录发现的显式false被覆盖问题，再恢复17.4。实际模型消息已永久红绿，证据见主账本§14；不放宽排序合同，不将累计占用或背景改成根因。Description变更的h2/h3匹配基线A/B与r229/异构帧生产回放另归18，尚未运行。

## 3. 18类差距的实施子项

### HMC-01：能力目录与使用前提（P1）

参考 `list_indicators/get_skill_catalog`、skills HS-01/17；沿现有共享 view/schema 增强。

- [ ] **HMC-01.1 待实施** — 统一能力目录：逐视图/指标声明所需事件族、对象、时间单位、输入格式及能力缺口；验收目录与真实注册/schema逐项一致，缺测不填零。
- [ ] **HMC-01.2 部分实施** — 参数/JSON教学同源：默认值、枚举、必填及工具组合实参从同一合同产生；验收示例逐条通过真实参数解析/公开入口，覆盖别名与缺可选字段。排序/帧流教学子缺陷已`af2e2548d`推送（§14）；显式统计false修复确定性验收完成（§14.1），其余不代销。
- [ ] **HMC-01.3 待实施** — 按已声明任务与已观测能力供给上下文：保留所需资源/来源说明，量化重复和无关块；验收不扫原始问题关键词作硬门、不混入参考仓内部架构或固定客户名。

### HMC-02：按窗口/对象组织证据（P1）

参考 `query_metrics`、groups、按对象下钻；复用既有 recipe/bundle。

- [x] **HMC-02.1 实现已交付** — 独立搜索与 `span_locate` 组合搜索共用覆盖/总数/截断/取消发布；显式/派生窗与行窗均有回归。提交 `24d82316f`，收据主账本§9。
- [x] **HMC-02.2 实现已交付** — 紧凑因果上下文交付已有IO计量，分别保留请求驻留/提交线程等待/背景身份；跨窗见证不合并，同窗重复不乘算。`a784bc439`已推送，公共红绿/count3/race×3/末版全仓86包通过，主账本§11。
- [ ] **HMC-02.3 待实施** — 通用 per-object 证据索引：显式授权目录内的多工件发现清单、对象ID、源代次、窗口、结果/错误/截断、原始引用分别持久化；验收首层首文件不是全部、对象缺失/部分失败/重排不能借数组位置串证，依赖17.2材料身份。
- [ ] **HMC-02.4 待实施** — 新领域原子观测接入现有 recipe：逐子查询前提/结果覆盖，支持等价多工具调用顺序；验收部分结果可见、未读对象披露，不强制唯一工具序列，不复制groups别名内核。

### HMC-03：资源字段可查询性（P1）

参考 `native_hook` / heap；原始全表保真原本已有。

- [x] **HMC-03.1 实现已交付** — 资源瞬时点补精确大小、来源栈键、nullable资源结束时间；NULL/0/超大整数/非字节资源/坏可选列边界齐全。提交 `055bd749d`，主账本§7。
- [ ] **HMC-03.2 待实施** — 地址位型与资源子类引用：核对上游有符号/无符号存储、字典版本及缺失语义；验收不经float、不取绝对值、不凭资源名猜单位，原I/C不变。
- [ ] **HMC-03.3 待实施** — Native-hook栈展开：同采集/owner/callchain关联，完整度、未知符号与栈深可见；验收栈键不当函数执行证明，不能固定某一深度为业务叶子。依赖03.2；分配/泄漏另归14。

### HMC-04：业务区间、启动与热点（P1/P2）

参考 CORE-MARKER-PROFILE/STARTUP、HS-04/10/19、marker/process/startup/game指标。

- [x] **HMC-04.1 实现已交付** — 普通业务片段进入事实及已请求工作关系通道；名称不需预定义优化分类，窗口内与完整区间分开，不新增链上资格；展示上限与可选ID同源。`a784bc439`已推送，公共红绿/count3/race×3/末版全仓86包通过，主账本§11；业务窗自动选择及live解释质量未闭。
- [ ] **HMC-04.2 待实施** — 通用嵌套marker树：inclusive/self/状态交集、缺段和重叠分开；验收父子不双计、跨线程嵌套不铸调用边，系统/业务工作线索都保留。
- [ ] **HMC-04.3 待实施** — 启动实例与阶段：进程代次/实例键/开始完成端点、首帧和可交互终点分别表达；验收冷/热/多次交错/缺完成/显式窗，最长阶段不是自动主因。
- [ ] **HMC-04.4 待实施** — 动画、共享库加载与游戏启动profile：在通用实例模型上接版本化源协议；验收同名多实例、库路径/加载类型、窗口裁剪，不针对某游戏硬名定责。依赖04.2/04.3。
- [ ] **HMC-04.5 待实施** — 进程概览、睡眠树及D-state业务说明：复用状态/唤醒/频率事实，caller缺失保持未知；验收树容量披露、S/D与已证IO分开、不另建根因排序。
- [ ] **HMC-04.6 待实施** — Tag/业务区间与perf cohort聚合：运行交集、样本权重、嵌套归属和分母独立；验收0样本≠0执行、采样比例不乘窗口变实测毫秒。依赖04.2、09.4；PMU计数需09.1。

### HMC-05：HiLog/Kmsg等日志与Trace联查（P1）

参考 EXT-2、日志extender与时间映射模块。

- [ ] **HMC-05.1 待实施** — 共享日志事件输入：保原文件/行、原文、PID/TID/代次、解析错误和部分捕获；验收多文件、压缩、未知格式、重复导入，不仅保合并DB行号。
- [ ] **HMC-05.2 待实施** — 时钟关系/校准见证：单位、epoch、offset/drift/误差、有效区间；验收墙钟/boot/独立微核钟，不以文件名时间或单一aligned布尔授因果时钟。
- [ ] **HMC-05.3 待实施** — 按指定窗联查与独立背景结果：精确映射后查询，未对齐仍可独立阅读；验收空窗不偷换全量、关键词救回不改目标窗/因果权限。依赖05.1/05.2。

### HMC-06：网络流/协议与质量（P2/P3）

- [ ] **HMC-06.1 待实施** — 流式PCAP导入：接口、包序、IPv4/6、方向、流身份、截断/失败清单；验收大输入有界内存、重复包、部分解析、时钟隔离。依赖05.2。
- [ ] **HMC-06.2 待实施** — DNS/握手/首字节/重传/吞吐语义：请求与流完整键，真实测量与估计分列；验收DNS-ID碰撞、双向/重传、缺请求，不能将SYN→SYNACK称完整三次握手。
- [ ] **HMC-06.3 待实施** — 蜂窝/Wi-Fi质量事件及业务关联：规则版本、采样覆盖、进程映射质量；验收无数据为未知、零目标命中不以全量回填、质量分只作软提示。依赖05.1及06.1。

### HMC-07：视频/音频/解码链（P2）

- [ ] **HMC-07.1 待实施** — 播放器/解码器实例状态机：SetSource/Prepare/Playback/Configure/Flush/Stop等协议；验收多实例交错、ID复用/重启、未配端点。依赖05.1/05.2。
- [ ] **HMC-07.2 待实施** — Buffer轮转、渲染与首帧阶段：codec/session/buffer键及内容钟/墙钟/trace钟分列；验收同ts不乱并行、每次首帧不跨实例拼最早值。依赖07.1。
- [ ] **HMC-07.3 待实施** — 音频焦点/CAAS、HCODEC及vdec微核适配：各自生命周期/通道/时钟，缺映射保持独立；验收不同启动代次/chan复用，不从同一毫秒值拼因果。
- [ ] **HMC-07.4 待实施** — 媒体错误/网络阶段/用户交互联合视图：错误原文和窗口齐全；验收所有tag都受窗约束、未见手势不判暂停原因、故障规则不直接加冕。依赖06/07事实底座。

### HMC-08：精确资源分布统计（P1）

参考 CORE-IO-DIST/SCHED-DEPTH/GPU-OBS，优先复用完整census。

- [ ] **HMC-08.1 待实施** — IO延迟分位数：完整合格配对总体、读写/层级分组、固定quantile定义；验收第11条改变结果而TopN不含它、缺端点/歧义/0值、跨窗取样策略。
- [ ] **HMC-08.2 待实施** — IO尺寸/读写比例/IOPS/墙钟带宽：按明确提交/完成事件计数，bytes与count分开；验收并发分母、零桶、未知大小/层级去重，尺寸不能推随机/顺序。
- [ ] **HMC-08.3 待实施** — 真实IO在途深度：合格issue→complete半开区间sweep，发起率另列；验收缺完成/窗口carry-in/同时端点/coverage，不把arrival-count叫并发数。
- [ ] **HMC-08.4 待实施** — Runnable队列深度与并行度：同TID区间并集、全窗时间分母、零/未知、峰值/桶最大/均值分列；验收参考右端多占桶反例，不从非零桶均值冒充全窗均值。
- [ ] **HMC-08.5 待实施** — CPU idle-state×频率及统一区间明细：复用已有前继/频率/状态，按核/簇/线程保身份；验收窗口头carry-in、同ts更新、缺频率覆盖，不输出虚构全覆盖。
- [ ] **HMC-08.6 待实施** — GPU观测与active×freq：资源身份、单位、驻留/加权均频/时序；验收部分覆盖、inactive剔除、Hz/MHz、GPU实例隔离，不让低频背景自动成帧主因。
- [ ] **HMC-08.7 待实施** — 页缓存速率/占用/生命周期：dev+inode+page+代次和实际页大小；验收重复/换代/缺delete、跨文件，不复制LEAD弱配对、不新增唤醒边。
- [ ] **HMC-08.8 待实施** — 事件时刻优先级分层IO/RT风险：平台优先级与唯一epoch；验收一请求多sched片不倍增，D/请求驻留/已证阻塞三列不相加。

2026-09-18统计切入点复核（只读，不计交付）：`ComputeWindowStats`的`blockPairing.census`保留全量合格请求；`ioLatencyAccounting`仅Top-8，`ioLatencyCausalAccounting`亦不是统计总体，均不得用于分位数。现有配对窗口筛选有闭端点/行窗优先语义，应保留旧合同，为新统计单列取样策略及半开区间裁剪。非block层需在`accountGenericStorageTransition`已验证闭合点保留逐请求记录，不能从汇总均值/最大值反算。`queryWindowWallMs`的`TimeStart>0`不适合新统计合法零起点，需读显式Set标志；既有部分层级的summary.Bytes会累加起止行字节，不能当吞吐字节数。参考仓的发起次数不搬成在途并发，按耗时和做分母不搬成墙钟带宽，大小阈值不搬成随机/顺序访问事实。

### HMC-09：PMU/BRBE/SPE/调用树（P2/P3）

- [ ] **HMC-09.1 待实施** — PMU描述与槽位映射：采集配置、CPU/事件域/单位来源；验收未知映射不猜、整数/缺列/零分母分开，修正参考ms/ns矛盾而非移植。
- [ ] **HMC-09.2 待实施** — PMU summary/timeline/process/thread：共用聚合器和TID代次，交叠裁剪标估计；验收上游取数到聚合公开路径，不仅直接喂DataFrame。
- [ ] **HMC-09.3 待实施** — BRBE与SPE输入接缝：统一导入/消费者schema、ISA/符号代次/分支有效性、采样权重；验收真实受支持样本且无skip，不沿参考新表/旧列断裂宣称支持，再分批交IPC/分支错误/访存指标。
- [ ] **HMC-09.4 待实施** — 无损采样前缀树：可展开/折叠、原始引用/省略数量、独立cohort；验收不删系统栈、不改全量分母、不用错窗全量回退、不从显示父子铸call证据。

### HMC-10：跨Trace逐维比较（P1）

- [ ] **HMC-10.1 待实施** — 两侧测量envelope：源/代次/设备/工作负载/窗口/单位/分母/覆盖；验收单侧缺失保另一侧、缺测不填零，依赖02.3/17.2。
- [ ] **HMC-10.2 待实施** — 逐维可比性和差值：覆盖参考12维pipeline/frame/freq/gesture/scene/process/marker/flame/power/pmu/runnable/thread_gap；逐维接原子能力，未支持列明确未支持，不整请求拒绝或跨采集拼链。
- [ ] **HMC-10.3 待实施** — 多段CPU明细与跨App特征：同源重叠去重、独立源不可直接累加、未匹配对象清单；验收同名线程不同代次、不同设备/窗口、模型量与实测量分开。依赖08.5/13。

### HMC-11：批量、代表样本和报告（P2）

- [ ] **HMC-11.1 待实施** — 可恢复per-object执行/证据manifest：精确输入指纹、运行状态/失败/未覆盖成员、取消后恢复；验收不靠“文件存在/mtime较新”信任陈腐缓存。依赖02.3。
- [ ] **HMC-11.2 待实施** — 代表样本与簇对照：代表选择是软排序，全集覆盖/未匹配/跳过独立列出；验收被抽样之外不宣布无问题、不用模型同文率硬拒。
- [ ] **HMC-11.3 待实施** — 证据包导出与报告回写：有版本manifest/引用、事务输出及显式目标；验收缺成员/旧产物/部分失败不显示完成，read模式不写客户源码，导出不替模型诊断。

### HMC-12：帧协议与业务化图表（P1/P2）

- [ ] **HMC-12.1 待实施** — 框架/角色候选目录：ArkUI/Flutter/KMP/RN/Web及游戏角色，保存观测前提和多候选；验收检测失败不阻止帧查询、不将参考index缺文件变成已支持。
- [ ] **HMC-12.2 待实施** — 精确帧实例与连接器：先ArkUI now/transaction tid+seq与RS vsync/fence，再queue+sequence、Web trace_id/flow、Flutter时钟/实例；每协议独立验收同ID异进程/代次/多分支/缺端点，时间近邻只作背景。
- [ ] **HMC-12.3 待实施** — 动态刷新率/PreferredFrameRate/投票事实：owner、持续区间、覆盖、请求→决议关系等级；验收缺rate不默认60Hz，不以其它进程all-fallback授目标帧预算。
- [ ] **HMC-12.4 待实施** — 全帧总体与游戏体验：上报jank、Present间隔派生jank、FPS/jitter/连续卡顿分来源；验收跨屏/预算变化/截断、不双计、不把经验阈值当感知定理。依赖12.2/12.3。
- [ ] **HMC-12.5 待实施** — 同一事实模型驱动关系/时序/逻辑图：显示业务词与端点身份分离，缺边/分支/异步/无送显理由准确；验收真实渲染与关系完整性、未知枚举可读化，不系统补造箭头。依赖12.2，复用现有图合同及安全修复。
- [ ] **HMC-12.6 待实施** — GPU阶段与已证fence解释：资源观测与等待关系并列，不能用同进程时间接近拼边；验收大GPU背景不挤掉链上调度/IO/算力/语义优化。依赖08.6/12.2。

### HMC-13：设备、SMT与算力模型（P2）

- [ ] **HMC-13.1 待实施** — 有来源设备/芯片/拓扑资料：核型、SMT sibling、适用版本；验收未知设备/矛盾资料留未知，匹配分仅软提示，不能固定CPU成对。
- [ ] **HMC-13.2 待实施** — ST/MT、频点模型负载与能耗估计：系数/来源/单位/有效范围独立，分进程/线程同代次；验收0.6等不是通用事实，模型量不反写已证可消除毫秒。
- [ ] **HMC-13.3 待实施** — thermal/limit平台事件适配与分频样本：实测约束、频率、样本按精确身份组合；验收无thermal证据不判load-based、不按线程名跨源联样本。依赖08.5/09.4。

### HMC-14：堆、低内存与进程退出（P2）

- [ ] **HMC-14.1 待实施** — 堆分配/释放/线程/类型/subtype统计：资源族registry与单位单源、实际事件量与存量分列；验收statistic多行正确合计、NULL/0、FD等不套bytes。依赖03.2/03.3。
- [ ] **HMC-14.2 待实施** — 地址分配代次及未释放候选：owner代次/顺序/捕获边界；验收地址重用、free先于alloc、窗口尾未释放不宣称泄漏。依赖14.1。
- [ ] **HMC-14.3 待实施** — 低内存快照/RSS/PSS/GPU/DMA/PSI：快照完整性、计数差分和估算来源；验收部分捕获/复用PID/缺字段、不以三次上涨坐实泄漏。依赖05.1/05.2。
- [ ] **HMC-14.4 待实施** — Kill/OOM/候选名单/退出原因/生命周期：原始原因与版本化解释，实际事件与候选区分；验收近邻PID不乱配、后快照未见≠死亡、未见SIG9≠没被杀。依赖05.1/14.3。
- [ ] **HMC-14.5 待实施** — 回收阶段及kill效率：owner/代次/准确端点、anon/file/GPU释放各列；验收缺端点null、多实例并发、各阶段不机械加成响应阻塞。依赖14.4。

### HMC-15：并行化机会与Sendable（P2/P3）

- [ ] **HMC-15.1 待实施** — 通用优化机会证据：任意可信工作窗口的实测成本/估计收益/依赖与副作用/适用前提；验收山形/热点仅候选，缺依赖证明不直接建议安全并行。依赖04.2/09.4。
- [ ] **HMC-15.2 待实施** — 可选Sendable静态分析适配：先审外部依赖/许可/执行边界，源码+SDK+配置指纹绑定函数/类图；验收旧缓存不复用，`.ts/.ets`唯一源不凭存在性猜。
- [ ] **HMC-15.3 待实施** — 建议到既有write计划/验证：配置、类迁移、guide、实际补丁走风险/批准/worktree/verify；验收read无源码写入、编译成功不冒充并行语义正确。依赖15.1/15.2。

### HMC-16：教学一致性与模型负担（P1）

- [x] **HMC-16.1 实现已交付** — IO跨层教学退役nearest关联，改为精确源身份；提交 `24d82316f`，主账本§9。
- [x] **HMC-16.2 实现已交付** — 预阶段无计量工具时不再教手算，源端点/行锚走已有合法观察出口，residue-only错误教学修正；提交 `857920fa7`，主账本§10.1。
- [x] **HMC-16.3 实现已交付** — 资源瞬时点/计数器/NULL/0/大整数语义在预阶段、查询和最终成文同源供给；提交 `857920fa7`。生产解释质量仍开放，见18.2。
- [ ] **HMC-16.4 部分实施** — 全领域教学接缝验收：参数透传、单位、可选字段、无数据/部分结果出口、JSON示例统一；验收参考chip/cluster、ms/ns等错误不引入，不能“教学要求且schema拒绝”。依赖01.2并贯穿新领域；已新增实际agent→adapter初始消息接缝回归，排序/帧流子缺陷不代表全领域通过。
- [ ] **HMC-16.5 待实施** — 上下文成本与读者词汇：逐阶段量化相关性/重复块，业务词面与原始键并存；验收引用可寻址、图标签不污染身份、内部枚举不作读者解释，不靠答案原文扫描替模型修文。

### HMC-17：原始二进制默认自动接入（P1，分入口推进）

详细方案和11类格式能力边界见[二进制附录](hmosperf_binary_input_comparison_20260917.md)。17.1–17.3于2026-09-18以`482bfa856`提交推送；其它入口仍未交付，不能以现有`trace convert --trace-engine auto`验收。

- [x] **HMC-17.1 实现已交付** — 共享输入准备服务：内容探针、现有转换器auto、独立受管理目录、真实query-ready收据；文本直通、未知binary不按后缀误认、空/坏/库存only不能假可查询。`482bfa856`已推送，最终全仓及race验收见主账本§13。
- [x] **HMC-17.2 实现已交付** — 原始/派生/完整查询路径/有限预览四者分离并传递：源代次、摘要、ZIP成员、时钟/能力保留；尾部真实事件可查、换源/陈腐preview拒绝、sample-only不冒称有调度因果。`482bfa856`已推送；完整文本快照兼容、截断/派生材料重新附加及跨窗IO保护验收见§13，live另18.3。
- [x] **HMC-17.3 实现已交付** — CLI文件附件默认接入：`--htrace/--atrace`加载共享准备结果，单次转换、失败阻止此次分析、取消安全；真二进制、readonly输入目录、路径含空格/中文、不覆盖原件均已回归。`482bfa856`已推送；SIGINT事务回滚及公开查询验收见§13，跨平台另17.6。
- [ ] **HMC-17.4 待实施** — REPL文件附件默认接入：沿现有in-flight/取消生命周期，成功才替换附件；验收取消/失败旧附件不被悄悄当新附件回答，清除与再次附加一致。依赖17.1/17.2。
- [ ] **HMC-17.5 待实施** — typed path查询协调接入：上层准备并传query-ready材料；验收API/普通定位路径按明确来源工作，底层BuildIndex不隐式启动外部进程。依赖17.2/17.3。
- [ ] **HMC-17.6 待实施** — 格式/平台矩阵：RMQ版本与file_type、OHOSPROF、perf/SIMPLEPERF、ZIP/gzip各自真样本和能力收据；验收缺TS/多成员/未知插件/取消/权限错误准确，不宣称任意gzip/protobuf可解。
- [ ] **HMC-17.7 待实施** — 现存SQLite安全适配：schema/owner/clock/源代次验证后导出，输入只读；验收不把任意DB塞给TS、不修改用户DB，不用mtime作唯一缓存键。
- [ ] **HMC-17.8 待实施** — 二进制stdin/inline的独立策略：完整流有界冻结/EOF与limit收据后才允许转换；验收不能把截断文本preview当完整二进制，未支持入口明确指向文件路径方案。

HMC-17.4施工前已核实的接缝（2026-09-18，只读设计，不计交付）：

1. 非TTY提示/粘贴已有终身scanner，运行时取消监听却另建scanner；必须先统一输入所有权，覆盖预读、长期开着的pipe、迟到行及EOF，不能取消后吞掉下一条命令。
2. 文件准备是本地非LLM操作，使用可取消ctx并等回滚完成，不借用会重置用量统计的direct-LLM包装器。SIGINT/ESC/`/cancel`到同一取消目标；不能把转换期间的文本送入旧流水线的steering。
3. 成功才提交新附件并重放排队问题；失败/取消保留旧附件，但不能自动拿旧附件回答本为新附件排队的问题，应恢复为待确认输入并披露。
4. 准备成功的提交点必须明确；前置检查runner携带完整收据的能力。若需要成功返回后的撤销，必须用持有权限的Commit/Discard交接，不能让前端按路径删除产物。
5. 真RMQ二进制经`/htrace`与`/atrace`进入下一次公开查询，验预览外尾部和显式窗；另验旧附件/快照、完整文本schema1、截断schema2、无LLM/用量变化，不能只靠converter stub签绿。
6. TTY运行窗口现把输入行先尝试steering再排队，`/cancel`不是即时取消；准备期需在明确命令解析通道连接同一取消目标，bracketed paste内的同形文本保持原样，不能误当命令。
7. 非TTY输入不消费交互界面的`pendingInputPrefill/pendingPaste`；失败恢复不能只写这两个字段。`/htrace missing`同步失败后，下一条脚本问题可能尚未进入运行时队列，仍须由明确的附件尝试失败状态阻止静默借用旧附件；统一reader与操作取消范围先单独验收，再接Prepare，不把预读/迟到行归因于模型。

### HMC-18：场景覆盖与生产验收（贯穿全部批次）

- [x] **HMC-18.1 实现已交付** — 新增资源元信息和业务IO组合两个问题及真实parser/query事实夹具；提交 `24d82316f` / `42159453f`，未提示唯一view顺序，已转换文本不冒充二进制。
- [ ] **HMC-18.2 待验收** — 首对生产两例解释质量：原机器/人工均FAIL，修系统数据/教学接缝后用异构问题回放；不得改原答案/oracle或依靠第三例追绿。原记录 `857920fa7` / 主账本§10。
- [ ] **HMC-18.3 待实施** — 真二进制CLI/REPL→完整尾部查询→显式窗投影端到端cases；含无调度sample-only反例，依赖17入口，不能用converter stub代替最终真实样本验收。
- [ ] **HMC-18.4 待实施** — 新原子能力的边界矩阵：源/代次/时钟/单位/窗口/缺测/截断/后台/多实例，共用可组合fixtures；每项任务有对应正反证，不为单个type拟合。
- [ ] **HMC-18.5 持续执行** — 跨模式优先级回放：Trace/通用读/代码关系图/写plan/apply/verify轮换，固定2并行×1；每批记录重试原因、上下文是否足够、人工答案/图/旁路及未覆盖能力。

## 4. 跨分册归属索引（防重复/遗漏）

| 原分册分组 | 本清单主归属 |
|---|---|
| CORE-IO-DIST / HS-13 | 08.1–08.3、08.7–08.8；链与当前交接02.2独立 |
| CORE-SCHED-DEPTH / CORE-GPU-OBS | 08.4–08.6 |
| CORE-FPS-CONTEXT / CORE-FRAME-PROTOCOL / HS-07/08/09/11 | 12.1–12.6；GPU原子依赖08.6 |
| CORE-MARKER-PROFILE / CORE-STARTUP / HS-04/10/17/19 | 04.1–04.6；PMU先09.1 |
| CORE-CPU-MODEL / HS-06 / 频率明细合并 | 08.5、13.1–13.3、10.3 |
| CORE-RECIPE-PANELS / HS-01 | 01、02.3–02.4、16.4；已有等价组不重造 |
| EXT-1/5 | 03、14；已保真字段不重报物理丢失 |
| EXT-2/6 / HS-14/16/20 | 05、06、07、14.3–14.5 |
| EXT-3/4/7 / HS-12 | 09、15；BRBE/SPE参考断链不得直接迁入 |
| HS-02/03/15 | 10、11；单侧事实与对比/批量状态分别验收 |
| HS-05/18 | 15；读模式建议与write执行分开 |
| 默认binary/更多格式与跨模式eval | 17、18；每种入口单独验收 |

参考中已确认不正确的近邻连边、采样占比换耗时、硬编码60Hz、弱缓存、读模式改源码等，保留在主报告“不移植”清单；不创建“复制该错误行为”的实现任务。后续源码或真实日志发现本列表遗漏时，先补ID/证据/优先级，再施工，不只写入聊天。

## 5. 批次交付登记

| 批次 | 完成子项 / 交付 | 提交与验收 |
|---|---|---|
| 前置闭环 | 原B1716未提交工作 | `93bf1a42d`；主账本§6及旧统一账本§123.1849 |
| 静态对照 | 工具/skills/指标清点与18类差距 | `7f20cdea0`；清单摘要及逐项附录，不宣称代码已补齐 |
| 资源首片 | 03.1 | `055bd749d`；公开红绿/相邻回归/构建，生产解释另18.2 |
| 搜索组合/IO教学 | 02.1、16.1、18.1夹具 | `24d82316f`；后续夹具纠正`42159453f`，收据主账本§9 |
| 预阶段与资源教学 | 16.2、16.3及首对eval留档 | `857920fa7`；主账本§10，不改原FAIL |
| 业务/IO证据交接 | 02.2、04.1 | `a784bc439`已推送；公共红绿/count3/race×3/末版全仓86包及构建通过，含双窗去重与来源单源纠正；首轮全仓exit1与原live FAIL保留 |
| 默认二进制CLI附件 | 17.1、17.2、17.3 | `482bfa856`已推送；真实RMQ/SIMPLEPERF→公共完整查询、只读原件/取消/陈腐材料/快照兼容，最终无skip全仓87包及构建/race通过；未宣称REPL默认转换及live验收完成 |

本清单只管理本次HarmonyOS对照新增/重现的工作，不销清此前图表达、原生写验证等独立开放债；旧账本仍保留原ID与状态。

## 6. 原始条目到实施任务的完整反查

源码路径/函数、现有能力及不移植理由见主账本§2–3和逐项分册；本节只解决“该条目归谁、有没有漏排”。同一行列出的原始名称分别计数；任务编号省略公共`HMC-`前缀。映射不是完成状态，完成状态只以§3复选框为准。

### 6.1 工具：13个MCP与11个非MCP增量（共24个不同名称）

公开工具中11个也是executor工具；不能把13+22当成35个不同工具。

| 参考工具 | 实施子项 / 既有边界 |
|---|---|
| `get_hitrace_path` | 02.3、17.5；已有显式附件选择保留 |
| `get_log_path` | 05.1 |
| `get_pcap_path` | 06.1 |
| `convert_hitrace_to_sqlite` | 17.1–17.8；显式转换已在，默认附件入口另验 |
| `convert_hiperf_data` | 09.1–09.3、17.6；基础采样转换已在 |
| `analyze_frame_drops` | 12.2–12.6、02.4；已证帧/投影不重造 |
| `analyze_video_phases` | 07.1–07.4 |
| `analyze_launch_phases` | 04.3–04.4 |
| `identify_chip_model` | 13.1–13.3；推测不变硬件事实 |
| `list_indicators` | 01.1–01.3 |
| `query_metrics` | 02.1–02.4；新增原子能力分属03–15 |
| `get_skill_catalog` | 01.1–01.3、16.4 |
| `run_skill` | 02.3–02.4、11.1；不复制第二执行内核 |
| `analyze_parallelism_opportunity_intervals` | 15.1 |
| `analyze_cold_launch_intervals` | 04.2–04.3、15.1 |
| `analyze_l3_markers` | 04.2–04.3 |
| `extract_cross_app_features` | 10.3、15.1 |
| `analyze_frame_drop_intervals` | 02.3、12.2、15.1 |
| `detect_rendering_pipeline` | 12.1–12.2；5个定义见§6.4 |
| `analyze_tag_instructions` | 04.6、09.1–09.2 |
| `generate_tag_instruction_report` | 04.6、10.1–10.2、11.3 |
| `export_cluster_package` | 11.1–11.3 |
| `merge_cluster_analysis` | 11.2–11.3；不采纳模型散文雷同硬拒 |
| `compare_dual_trace` | 10.1–10.3 |

### 6.2 指标：109个原名全量归属

已有主体的条目只列回归保护与有价值的增量，不创建重复实现；具体缺口和参考算法风险以63/46分册为准。

| 原始指标名（各名独立） | 实施子项 / 既有能力处置 |
|---|---|
| `cpu_load` | 18.4保护已有census；08.5统一区间展示，不重做负载 |
| `cpu_freq`、`cpu_freq_occupancy` | 13.1–13.2；已有频率驻留保留 |
| `cpu_freq_breakdown`、`cpu_state_freq` | 08.5 |
| `cpu_freq_merge_breakdown` | 10.3 |
| `cpu_freq_compute_load`、`cpu_freq_compute_load_by_process`、`cpu_freq_compute_load_by_thread` | 13.2 |
| `cpu_freq_func_samples`、`cpu_freq_throttle` | 13.3；缺热证据不猜原因 |
| `gpu_freq`、`gpu_freq_ts` | 08.6 |
| `sched_wakeup_latency`、`thread_state_durations`、`sched_affinity`、`sched_cgroup`、`thread_runtime` | 已有主体，18.4守住全量/精确窗/平台语义；目录01.1 |
| `thread_sched_analysis` | 13.1–13.2、02.4 |
| `concurrent_parallelism`、`sched_runnable_queue_depth` | 08.4 |
| `io_bandwidth`、`io_iops`、`io_read_write_ratio`、`io_size_distribution` | 08.2 |
| `io_latency` | 已有配对与等待事实，02.2交接已修；18.4保S/D及闭合边界 |
| `io_latency_percentile` | 08.1 |
| `io_concurrency` | 08.3，不移植发起数冒充并发数 |
| `page_cache_add_rate`、`page_cache_evict`、`hot_file_cache` | 08.7；已有热点统计保留 |
| `io_by_priority`、`io_latency_by_priority`、`dstate_io_by_priority`、`rt_thread_io_block` | 08.8 |
| `animation_interval`、`so_load_stats` | 04.4 |
| `trace_marker_hotpath` | 04.1–04.2、04.6 |
| `process_profile`、`thread_sleep_summary`、`uninterruptible_dstate` | 04.5；已有状态/唤醒/IO证明保留 |
| `launch_phase_breakdown` | 04.3 |
| `frame_ts`、`ui_frame_ts`、`rs_frame_ts`、`flutter_frame_ts`、`web_frame_ts`、`game_frame_ts` | 12.2；协议逐批验收，不整体签已支持 |
| `frame_summary`、`jank_intervals` | 12.4；已有jank_event_sync保留且不双计 |
| `gpu_pipeline_analysis` | 12.6 |
| `preferred_frame_rate`、`frame_rate_votes` | 12.3 |
| `game_experience` | 12.4 |
| `game_launch_phases` | 04.4 |
| `game_thread_sched` | 12.1、02.4；任意线程调度事实已在 |
| `jank_essentials`、`jank_deep`、`launch_essentials`、`game_launch_essentials`、`sched_comprehensive`、`memory_comprehensive` | 02.4统一组合；不为别名复制内核，原子能力依本表 |
| `l1_budget` | 02.4复用已有守恒时间账，18.4保护；不移植最大桶主因裁定 |
| `heap_timeline` | 03.1–03.2、14.1 |
| `heap_alloc_summary`、`heap_thread_summary`、`heap_type_summary` | 14.1 |
| `heap_leak_candidates` | 14.2 |
| `heap_callchain_expand` | 03.3 |
| `heap_mmap_subtype` | 03.2、14.1 |
| `kill_events`、`kill_reason_summary`、`kill_candidates`、`process_lifecycle`、`process_disappearances`、`oom_kill_events` | 14.4；消失/候选/实际退出分别表达 |
| `kill_efficiency` | 14.5 |
| `low_memory_snapshot`、`low_memory_process_rss`、`low_memory_total_pss`、`low_memory_anomaly` | 14.3 |
| `network_pcap_quality` | 06.1–06.2 |
| `network_quality` | 06.3 |
| `video_phase_timing`、`video_decoder_lifecycle` | 07.1 |
| `video_buffer_rotation`、`video_render_chain`、`video_first_frame` | 07.2 |
| `video_audio_chain` | 07.3 |
| `video_error_events` | 07.4 |
| `hiperf_hot_function`、`hiperf_thread_hotpath`、`perf_calltree_in_interval`、`cold_launch_calltree` | 09.4；已有热点/源身份/权重不改 |
| `cold_launch_hitrace_calltree` | 04.2 |
| `hiperf_brbe_inst`、`hiperf_brbe_func`、`hiperf_branch_mispredict`、`hiperf_spe_miss` | 09.3 |
| `pmu_summary`、`pmu_timeline`、`pmu_by_process`、`pmu_by_thread` | 09.1–09.2 |
| `frame_drops_to_file` | 11.3、15.1；导出窗口/片段不等于批准修改源码 |
| `sendable_sample_config`、`sendable_function_analysis`、`sendable_class_graph` | 15.2 |
| `sendable_task_plan`、`sendable_class_guide` | 15.3 |

### 6.3 Skills：24个文件全量归属

名称和文件SHA可反查机器清单；这里使用不随显示标题翻译变化的YAML文件名。

| 文件（`config/skills/`下） | 主归属子项 |
|---|---|
| `ad_hoc_exploration.yaml` | 01.1–01.3、02.4 |
| `cluster_analysis.yaml` | 11.1–11.3 |
| `cold_launch_cross_app.yaml` | 10.3、15.1 |
| `cold_launch_l3_breakdown.yaml` | 04.2–04.3、09.4 |
| `cold_launch_parallel_v2.yaml` | 04.2–04.3、15.1 |
| `cpu_freq_analysis.yaml` | 08.5、10.3、13.1–13.3 |
| `detect_rendering_pipeline.yaml` | 12.1–12.2 |
| `frame_drop.yaml` | 12.2–12.6 |
| `frame_drop_parallel_analysis.yaml` | 12.2、15.1 |
| `freq_distribution.yaml` | 08.5、13.1–13.2 |
| `game_launch.yaml` | 04.3–04.4 |
| `game_performance.yaml` | 12.2–12.4 |
| `game_sched.yaml` | 12.1、02.4 |
| `hiperf_sampling_analysis.yaml` | 09.3–09.4 |
| `io_analysis.yaml` | 02.2、08.1–08.3、08.7–08.8 |
| `kill_analysis.yaml` | 14.4–14.5 |
| `launch_perf.yaml` | 04.3 |
| `load_compare.yaml` | 10.1–10.3 |
| `low_memory.yaml` | 14.3–14.5 |
| `parallelism_analysis.yaml` | 15.1 |
| `sched_analysis.yaml` | 08.4–08.5、13.1、02.4 |
| `sendable_parallel_analysis.yaml` | 15.1–15.3 |
| `tag_instruction_analysis.yaml` | 04.6、09.1–09.2 |
| `video_playback.yaml` | 05、06、07.1–07.4 |

### 6.4 渲染管线：5个实际定义

| 原始ID | 主归属子项 |
|---|---|
| `pipeline_harmony_arkui_render` | 12.1软发现、12.2精确关系、12.5展示 |
| `pipeline_harmony_flutter` | 同上；跨时钟映射为前提 |
| `pipeline_harmony_kmp_render` | 同上；复用已证底层协议，不以框架标签自铸边 |
| `pipeline_harmony_rn_render` | 同上；index中的派生别名不算独立实现 |
| `pipeline_harmony_web_pipeline` | 同上；source/process/trace_id/代次不可省 |

index引用但缺文件的三项不签参考实现已支持；后续本项目新增能力仍需自己取得输入协议与回归见证。

本次与机器清单/主报告工具表逐名核验：指标109/109、skills24/24、管线5/5、不同工具24/24，零漏项/零多报/零重复原名；79个任务ID唯一、18个父类齐全。以后增删参考条目或拆任务时同样核对原名集合，不仅对总数。
