# 读模式架构级 Gap 审计与系统性优化方案

适用范围:读模式四阶段流水线(analyze → explore → extract → finalize)、log_triage 与 perf_triage 前置 lane、混合场景(日志+trace+代码)、emit/修复层、调度器终局与预算。

审计方法:五路并行深度代码审计(handoff 链 / 日志 lane / trace lane / 修复分层 / 调度节奏)交叉验证,**每条进入本方案的 gap 都经过人工二次核实**(五路审计原始产出中约三分之一为误报或设计内行为,已剔除,见 §4);并配合代表性 eval 实测(代码 / 日志 / trace / 混合四轴,每波并行 2 个)与运行日志人工读取。

## 1. 已核实的架构级 Gap

### R1(P0)终局路径未全部收敛过同一质量门

`checkTier1Floor` 的设计意图是"所有探索退出路径汇聚的唯一咽喉",但循环内存在绕行:

- `fin == nil` 且存在 blocked 节点时直接 `break` 出调度循环,进入 force-finalize 路径——**不经过 Tier-1 floor**(orchestrator.go 调度循环 blocked 分支 → 循环外 force-finalize dispatch)。
- hard-stall 检测(指纹连续不变)force-close 探索窗口后,floor 即使触发,requeue 重试会立刻再次 hard-stall,等价于无修复通道。
- accepted-closure(HasEnoughFacts 提前闭合)路径绕过 floor 直达 auto-verdict(实施时复核)。

这是一类问题:**质量门只覆盖"常规"终止,而非全部终止**。写模式同类问题(预算耗尽丢弃已 apply 工作)刚刚修过——同样的架构模式适用于读模式。

**方案**:抽取单一 `preFinalizeGate` 函数,所有进入 finalize 的路径(常规 fin 就绪、blocked-break、hard-stall、accepted-closure、force-finalize)必须经过;无法 requeue 的路径上 floor 失败不再静默,而是注入 typed 降级 caveat(复用既有 caveat materializer),让答案明示"证据 grounding 低于阈值且无法补救"。精确信号(floor 比值、预算布尔)驱动,无散文解析。

### R2(P1)重试预算跨 locus 互食,廉价重试饿死昂贵修复

per-kind 重试预算(`RetryBudgetByKind`)的计数不区分重试 locus:R2.2 finalize-local 降级(只重写答案、不重新探索)同样计入 kind 主账。后果:同一 violation kind 连续两次被廉价的 finalize-only 重试消耗后,第三次真正需要 `BackToExplore`(补证据)时 kind 预算已尽,带 caveat 出答——**证据从未被重新审视**。

伴随问题:force-finalize transient 重试循环直接重派 finalize,不消费探索期间排队的 `RepairDirective` / `PendingReads`(实施时核实 closure 状态可达性)。

**方案**:重试记账按 (kind, locus) 分账——finalize-only 降级消耗既有的 `FinalizerLocalRetryBudget`,**不计入** kind 主账;kind 主账只记实际执行了 primary locus(explore/extract)的重试。force-finalize 重派前 drain 未消费的 pending repairs(若 closure 状态在该路径可达)。所有计数都是既有 typed 计数器的记账口径修正,不新增机制。

### R3(P1)前置 lane 降级对用户不可见

log_triage / perf_triage 的 emit 全部被拒时,StageOutput.Error 仅以 WARN 日志记录,Run 以"无 artifact 锚点"继续。用户明确附加了日志/trace,系统静默丢弃了它,最终答案不披露任何降级。这违反 handoff 原则:前序阶段收集(或收集失败)的关键事实必须按优先级传递到答案面。

**方案**:typed `PreStageDegradation` 载体(stage 名、typed 原因类别、有界的拒绝摘要),挂 Mutable;analyzer prompt 渲染"附件处理降级"段(软引导:让分类器知道 artifact 锚点缺失的原因);finalizer 经既有 caveat lane 在答案中明示降级。全 typed,不解析散文。

### R4(P1)混合场景的 Mutable 协调与坐标保真

- LogTriage / PerfTrace 槽独立 set、独立 bump revision;perf 两步升级会在 Run 内 reset PerfTrace(与"至多一次"的注释矛盾),跨槽一致性无契约。
- 前置 lane 顺序(log 先于 perf)仅由 topology 数组位置隐式决定。
- `PerfBundle.LogFrames()` 投影丢弃时间坐标(StartTsMs/DurationMs),trace 内嵌日志帧到达下游后无法回锚到时间窗。
- 双 artifact 同时在场时,analyzer 的 RequiredFiles 合并对 log authority 与 perf 文件的优先关系是隐式 if 顺序,无注释无契约。

**方案**:(a) typed `AttachedArtifactView` 聚合只读视图——单一访问点同时取两 lane 的 typed 产物与版本号,消除两步读的不一致窗口;(b) topology 中前置 lane 顺序加显式声明注释 + 结构测试钉序;(c) LogFrame 投影补充时间坐标字段(产出端 PerfObservation 已有数据,纯字段透传);(d) RequiredFiles 合并的 authority 语义写成具名函数 + 注释 + 测试钉行为(行为不变,契约显式化)。

### R5(P2)emit/修复层一致性残差

统一 strict-decode 修复层(`failStrictDecode*` / `RemapStrictDecodeError`)覆盖大部分 emit 工具,但:

- `emit_log_triage` 的 salvage→重解码双路径在 salvage 成功时丢失 repair 元数据;
- `emit_perf_trace` 走 `failStrictDecodeWithError` 但不填 `Repair.Fields/Hint`(常见的 stalls/frames 字符串载体错误本可预映射提示);
- 拒绝消息的 R6 卫生(无内部术语)只有 RemapStrictDecodeError 一处有测试,各 emit 工具手写的拒绝 Summary 无全量扫描。

**方案**:(a) 封装单一 `decodeWithSalvage` 序列(尝试解码 → salvage → 重解码 → 统一 fail 包装),双路径工具迁移到该 seam;(b) emit_perf_trace 接入与 emit_log_triage 同级的 Repair 元数据;(c) 新增 hygiene 测试:枚举全部读模式 emit 工具,以非法载荷触发拒绝,对 Summary 做内部词汇扫描(测试 oracle 允许使用词表;产品逻辑不引入任何关键字匹配)。

### R6(P2)终止原因无 typed 形态

`stopcond.ShouldStop` 返回 `(bool, prose string)`;`InvestigationComplete` 无 Kind 字段。下游(telemetry、R1 统一收口、caveat 选择)只能拿到散文。当前没有散文被用于硬路由(合规),但 R1 的统一收口需要区分"哪条终止路径进来的"——必须 typed。

**方案**:`TerminationKind` typed enum(stop_condition / idle / soft_stop / budget / hard_stall / accepted_closure / blocked_dag),由各终止点写入,R1 的 `preFinalizeGate` 与 telemetry 消费;仅作软消费与计量,不驱动硬路由。

### R7(P2)Handoff 截断的可观测性

extractor 侧三个硬上限(evidence 24 / notes 6 / flow findings 10)溢出时提示行存在,但无计量、无分数分布。在改动 cap 之前先量化:typed overflow 计数(被截条数、截断线上下的分数)进 telemetry 与提示行("score ≥ X 的还有 N 条未展示")。**不动 cap**——是否扩容由真实运行数据决定,避免无证据的 prompt 膨胀。

## 2. 设计内行为(审计候选中确认不动的)

- storm 类 violation(DemotionStorm/ForcedReadStorm)默认映射 FailLoud 是防误提升的安全网(代码注释明示);可选改进为 typed 不可提升类,列为后续候选,不进本轮。
- `IsExternalSource` 日志清空 RequiredFiles:外部系统日志没有本仓锚点,强制读本仓文件是噪声——设计意图,补一行注释即可。
- trace_query 平台/口味检测的字符串启发仅影响软提示与 caveat,不驱动硬行为,合规;优先级链改 typed enum 列为候选。
- PredicateAxis / ArtifactObservationProfile / DiagnosticProfile 均有下游消费点(extractor 词汇注入、finalizer 评估器渲染),审计原始断言不成立。

## 3. 任务列表(分批交付)

### 批 1 — R1 统一终局收口 + R6 TerminationKind(P0)【已交付】
- [x] `TerminationProfile`(Kind enum + FloorDegraded + 有界 Detail)typed 载体;stop_condition / hard_stall / blocked_dag / scheduler_stalled 四个终止点写入;kind 重写不丢降级标志。
- [x] 终局收敛:blocked/stalled break 路径在 force-finalize 前执行同一 grounding floor;floor 失败标记降级。floor 常规分支:预算耗尽、或终止类别为 hard_stall(指纹静止,requeue 必然复 stall)时直接降级,不再烧重试预算。
- [x] 降级可见:`appendSystemCaveatsToAnswer` 统一 caveat 汇点(替换全部 11 处 inactive-scope 调用点),降级时注入中英文用户 caveat;内部诊断细节(比值/阈值)只入日志与 telemetry,不漏入答案。
- [x] 实施期复核修正:accepted-closure 路径经 `continue` 仍达 floor,审计的"绕过"断言不成立,未做无谓改动;默认 grounding profile(permissive,floor=0)行为零变化,收敛只对启用 floor 的 profile 生效。

### 批 2 — R2 预算分账(P1)【已交付】
- [x] `shouldBillKindRetryLedger` 具名判定:R2.2 降级轮(picker 要 primary locus 但被降为 finalize-only)豁免 kind 主账——该轮已由 FinalizerLocalRetryBudget 计量,双重计费会让两次廉价重写耗尽该 kind 昂贵通道的预算;原生 finalizer-only 选择保持 kind 记账与边界。
- [x] force-finalize 派发前 drain `runForcedReads()`(closure 已排队的确定性文件读取,组合式 finalize 前最后一次充实证据池);可达性已核实(函数自包含)。
- [x] 全量回归绿 + 判定矩阵测试。

### 批 3 — R3 降级可见性 + R4 混合协调(P1)【已交付】
- [x] `PreStageDegradation` typed 载体(stage + dispatch_error/emit_rejected 枚举 + 400 字节有界摘要),前置失败两个分支记录;prompt 侧 `attachedTriageUnavailable` 状态追加"结构化解析未被接受(原因)"说明;答案侧经统一 caveat 汇点输出中英文降级披露。
- [x] `Mutable.AttachedArtifacts()` 单锁原子读双 lane;builder 与 analyzer 合并点改用之,消除两步读不一致窗口。
- [x] preStages 声明序契约注释 + `TestPreStageOrder_LogBeforePerf` 钉序。
- [x] trace 投影 `LogFrame` 补 `ArtifactStartTsMs/ArtifactDurationMs`(产出端已有数据,纯透传),trace 帧可回锚时间窗。
- [x] 实施期复核修正:审计的"log authority 排除 perf 文件"断言错误——perf union 发生在 authoritative ceiling 返回之前,perf 文件随 ceiling 一同返回;补澄清注释,无行为改动。

### 批 4 — R5 emit 一致性 + R7 溢出计量 + R8 条件引用拒绝(P2)【已交付】
- [x] emit_log_triage salvage 成功路径把修复事实经 compat 注记带给模型(下次直接发数组),不再只进日志。
- [x] emit_perf_trace 接入 MisplacedFieldHint 预映射(stalls/frames/observations 字符串载体 → 字段级修复指引)。
- [x] 全 emit 拒绝消息 R6 hygiene 测试(枚举 5 个读管线 emit 工具,非法载荷触发拒绝,内部词汇扫描——测试 oracle,产品零关键字)。
- [x] extractor 证据溢出提示升级:量化被截条数 + 声明"排名截断不构成反证、快照仍可取用"。
- [x] R8(实测发现):emit_hypothesis_verdict 对 artifact-local 引用(log:N)的拒绝消息现在引用 typed 条件(本问要求 current-source lane → 需 repo 锚点,artifact 行留在 rationale),终结"格式不像 path:line"式的盲猜;声明格式的结构解析,非意图关键字。
- [x] 混合案 spec 放宽:file:line 实路径引用视作"结合源码"的有效形态。

### R9(实测第五轮发现,新增)用户引用的 artifact 坐标无 typed 通道贯穿到答案面

logtri_line_current_code 复跑:同一问题("日志第 3 行的 first_byte_timeout")两轮答案,一轮保留"第 3 行"锚点、一轮丢失——坐标是否回显全凭模型措辞抖动。问题类:用户问句中引用的 artifact-local 坐标(日志/trace 行号)在 analyzer 处无 typed 声明位,finalizer 无对应软指令,答案面是否锚定全靠运气。

**方案**:`RequestModel.ReferencedArtifactLines`(source enum + 行区间)由 analyzer 在 emit_analysis 中 typed 声明(模型分类,非系统关键字扫描);finalizer 答案面渲染软指令"问题引用了附件第 N 行,请以 artifact-local 坐标锚定解释,与 repo 引用保持两轨"。软引导 + typed 信号,不设硬门。

**已交付**:`ArtifactLineRef` + 共享 normalizer(source 枚举、行界、跨度钳制)、emit_analysis schema/解析/IR 赋值、analyzer skill 声明规则、finalizer `Referenced artifact lines` 答案面段落;活体验证混合案与专项锚点案双 PASS。

### 批 5 — 实测回归【已交付】
- [x] 四轴八案逐波实测:s1a PASS / logtri_goroutine_dump PASS / trace_query_blocked_reason_chain PASS / qf_architecture PASS / trace_query_state_churn_root_cause_rank PASS / logtri_cpp_asan PASS / trace_query_donghu_mixed_platform PASS;logtri_line_current_code 经 spec 放宽 + R9 typed 坐标通道后 PASS。**最终 8/8**。
- [x] 实测过程贡献两项新 gap(R8 条件引用拒绝、R9 坐标通道),均已交付并活体验证。

## 4. 审计误报剔除记录

进入方案前被人工核实剔除的审计断言:emit_analysis 未用统一修复层(实际在解码处即用 `failStrictDecodeWithError`)、PredicateAxis 从未渲染给 extractor(实际大量消费)、ArtifactObservationProfile 从未渲染(finalizer 评估器渲染)、storm FailLoud 映射为缺陷(实为注释明示的安全网)。

## 5. 实测记录

- 第一波:s1a(代码)PASS;logtri_goroutine_dump(日志)PASS。
- 第二波及后续:见进度附记。

## 6. 进度

- 方案落盘并推送。
- 批 1 交付:终止画像 + 终局 floor 收敛 + 降级 caveat 汇点;全量测试绿。
- 批 2 交付:kind/locus 分账 + force-finalize 前 drain;全量测试绿。
- 批 3 交付:前置降级全链路可见 + 混合场景原子读/钉序/时间坐标;全量测试绿。
- 实测第四波:logtri_cpp_asan PASS、trace_query_donghu_mixed_platform PASS。四轴八案 7/8,唯一 FAIL 为 case spec 词汇钉死(非系统缺陷)。
- 批 4 交付:emit 一致性 + 溢出可观测 + R8 条件引用拒绝;全量测试绿。
- 实测第五轮(spec 放宽后混合案复跑):FAIL 转移为真实抖动——artifact 行号锚点一轮在一轮丢 → 立项 R9(typed 坐标通道),见上。
- R9 交付并活体验证:logtri_line_current_code PASS、logtri_artifact_line_anchor PASS。全部批次完成,四轴 8/8。
- 实测第二、三波:trace_query_blocked_reason_chain PASS、qf_architecture PASS、trace_query_state_churn_root_cause_rank PASS;logtri_line_current_code FAIL 判定为 case spec 词汇钉死(答案以精确 file:line 引用结合源码,优于字面"源码"一词;spec 待放宽)+ 顺带挖出 R8:emit_hypothesis_verdict 对 artifact-local 锚点的条件接受,拒绝消息未说明条件分支,归入批 4 的拒绝消息精度项。
