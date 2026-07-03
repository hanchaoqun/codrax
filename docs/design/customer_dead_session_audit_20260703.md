# 客户"死等会话"四附件审计(2026-07-03)

四个客户附件:
1. `dead_1g.txt` — berlin.systrace(1104MiB)会话的 CLI 打印 + 日志合并(卡死现场)。
2. `dead_new_01.log` — 同会话"trace_query 长时间等待后结束"的日志尾段。
3. `dead_answ.txt` — 该问题最终答案 markdown 报告。
4. `readfile_de.txt` — 另一问题(东湖 record_trace,Choreographer#doFrame 丢帧)的全程 CLI 日志。

本文档 = 事实链还原 + 客户明示问题逐条裁定 + 隐藏系统 gap 清单 + 任务拆解。进展账本在文末"进展"段持续刷新。

---

## 1. 事实链还原(卡死现场,附件 1+2)

时间线(全部来自客户日志时间戳):

| 时刻 | 事件 |
|---|---|
| 14:19:16 | iter=11 模型返回,发起 `trace_query view=frame_window window=6793222.031..6793222.131`(无 pid) |
| 14:19:19 | trace_query **2.95s 完成**(index_event_limit denial,见 §3-H3) |
| 14:19:21 | iter=12 LLM 请求发出(`timeout=10m0s first_byte_timeout=40s stall_timeout=2m0s`) |
| 14:19:51 起 | 每 30s 一条 `WARN [diag explorer] still running`,**只进日志,CLI 全程静默** |
| 14:33:56 | LLM 响应结束:**elapsed=14m35.876s**,`finish_reason=length output_tokens=24576`,content_len=**124141 字节**(同一句"综合所有 trace 证据,给出完整诊断结论。"重复数千次) |
| 14:33:56.99 | 渲染层把 124KB 退化文本按 reasoning+content **各回显一遍**到 `INFO [render]`(客户看到两面巨墙) |
| 14:33:57 | softstop 注入 completion-tool 提醒,iter=13 继续 |

**裁定 1(客户问题 1"trace_query 卡了好久")= 误归因。** trace_query 全程 2.5~3s/次(1.1GiB 流式扫描 2.5s,是上一批稀疏锚点索引的正常水平)。真正卡住的是 iter=12 的**模型请求本身(14m35s 退化生成)**。CLI 的表象欺骗:最后一行停在"⇢ 调用工具 trace_query …",此后 14.5 分钟无任何输出 → 客户把等待归因给 trace_query。

**裁定 2(客户问题 2"大量重复内容")= 模型退化为主因,系统有三个放大器。**
- 主因:MiniMax-M2.7 在多次相似 denial/tool 结果后进入重复退化(iter 8/10/11 的 content 已出现同句 ×3~×9 前兆,iter=12 全面退化直到撞 24576 token 输出上限)。
- 系统放大器 A:reasoning_content 与 content 各渲染一行,内容高度重复 → 客户看到"重复字段"体感翻倍。
- 系统放大器 B:渲染行无长度上限、无连续重复段折叠,124KB 原样上屏。
- 系统放大器 C:总超时 10m 对流式响应未生效(见 H1),退化生成烧满 14.5 分钟无人打断。

## 2. 客户明示问题裁定

- **C1 卡死误归因/性能**:trace_query 非瓶颈;需 (a) LLM 等待期 CLI 心跳可见化(диag WARN 已有 30s 节奏,复用到 render 面),(b) H1 流式总超时修复,(c) H3 padding 修复(该次 frame_window denial 本可成功)。
- **C2 重复内容**:模型问题 + 系统放大;修 放大器 A/B(渲染折叠+截断)与 C(超时);另加流式退化断路(精确信号:同一 ≥N 字节尾块连续 verbatim 重复 ≥K 次 → 中止请求,按重试处理;属于"真硬门配降级保底"范式)。
- **C3 "未定位线程"**:blocking/D-state 观测的 peer=unknown-thread 占位词直出。改类型化客户语言(阻塞跨度/D状态IO等待 + "等待对端未能从 trace 解析"),图例补一句;合并行不得丢线程名(H7)。
- **C4 投影窗口选择**:投影窗=探索阶段选定窗(101ms 子窗),但 (a) 🎯 根被标 ‹用户关注线程› 而根是 VSyncGenerator-2270,用户关注的是 pid 42591 —— 标签与 analyzer 实体(精确信号)不一致时不得写"用户关注";(b) 答案未呈现"用户请求窗 3.3s ↔ 投影子窗 101ms"的关系与选择理由;(c) 超长窗优雅支持:coverage 扫描/分段聚合方案(见任务 W3)。
- **C5 树读法/口径排版**:两段 run-on 长文改为低调的逐条列表(不加粗、短句、每条一行)。
- **C6 东湖会话深挖**:见 H 系列(source=attached_trace 首轮报错、read_file blob 拒绝浪费一轮、E1/E2 对端 alias 重复等)。

## 3. 隐藏系统 gap 清单(附件深挖,未被客户点名)

| # | Gap | 证据 | 严重度 |
|---|---|---|---|
| H1 | **流式响应总超时未生效**:timeout=10m0s,实际 14m35s 正常返回 | dead_new_01 L10-11 | P0 |
| H2 | 输出上限日志自相矛盾:`output_tokens=24576 cap=0` 却说 "request hit output cap" | dead_new_01 L10 | P2 |
| H3 | **索引窗 ±0.5s 固定 padding + 守卫无 graceful degrade**:101ms 请求窗被扩为 1.1s;parsed_time 已完整覆盖请求窗,仅 padding 尾段未完成即整体 denial,模型永久丢失 frame_window 维度 | dead_1g L279-292 | P0 |
| H4 | heavy view 缺 pid 时无 analyzer-pinned pid 自动 scope/强建议(iter=11 frame_window 无 pid 直接撞限) | dead_1g L278-280 | P1 |
| H5 | `state_drilldown` top_sleep 整窗睡眠 idle 线程噪音 15+ 行刷进"系统补充"(AudioOut/DNS/FFRT… impact=101.000=整窗,无链相关性,零信息量) | dead_answ L155-171 | P1 |
| H6 | 邻近链去重缺失:两条 irq_activity 同值 35.350ms、行号仅差 3-5,双行并列 E11/E12 | dead_answ L58-59 | P1 |
| H7 | 合并行 label 丢线程名:"其余 2 项合并"节点列显示"未定位线程 ×2",线程名全丢 | dead_answ L88 | P1 |
| H8 | 百分比 >100% 无标注:irq_burst 204.382ms **202%**、页缓存抖动 133.200ms **110%**(跨 CPU/多次累计),bar 截断但数字裸奔 | dead_answ L57, readfile_de L78 | P1 |
| H9 | "下一步"条目重复(第 1、4 条 verbatim 相同) | dead_answ L119-122 | P2 |
| H10 | berlin 投影缺"on-chain 已归因/未归因残差"覆盖行(东湖案例有);101ms 窗主根因仅 2.891ms/3% 时残差信息更关键 | dead_answ vs readfile_de L61 | P1 |
| H11 | 🎯 根线程又作为下钻链中转节点出现(VSyncGenerator→tppmgr→VSyncGenerator→…环)无 cycle 标注 | dead_answ L43-46 | P2 |
| H12 | 主根因行"2.891ms(占窗3%)"无窗口上下文陈述(并入 C4 修复面) | dead_answ L32 | 并入C4 |
| H13 | 指标快照"运行 2.891ms(占100%)"分母是线程自身观测时长而非窗口,客户易误读 | dead_answ L115 | P2 |
| H14 | CLI 轮次 off-by-one:日志 iter=12,CLI 显示"第 13 轮"(确认 1-based 是否有意;两面口径要一致) | dead_new_01 L34 | P3 |
| H15 | 退化 124KB assistant 内容疑似已被截断后入历史(messages 31→33 而 context_tokens_est 仅 +796)——**需代码确认截断点存在且有界**;若无则是 context 污染 | dead_new_01 L91-92 | P1(核实) |
| H16 | 东湖会话 iter1 `source=attached_trace` 被拒重走一轮(O3 auto-resolve 面覆盖范围核实:是否只认 source=path) | readfile_de L22-24 | P1(核实) |
| H17 | explorer read_file `.codrax/blob/trace-query-result-*.json` 被政策拒绝,浪费一轮;结果头 payload_ref 无"不可 read_file,请用下钻视图"提示 | readfile_de L27-28 | P1 |
| H18 | E1/E2 对端 alias 重复:同一 monitor contention 以 `NetworkKit_AssetsUtil_Operate_0-42067` 与 `pid=42067` 两行并列(112.223/112.214ms,行号差 1) | readfile_de L67-68 | P1 |
| H19 | 证据索引 locator 格式不一致:E15 显示 `lines=794198-827402` 无文件名前缀,其余带 `berlin.systrace:` | dead_answ L109 | P3 |
| H20 | 聚合类型影响形态泛化:irq_burst/irq_activity/页缓存抖动 行影响形态全是"候选影响",可类型化(IRQ突发/IRQ活动/页缓存抖动) | dead_answ L57-59 | P2 |
| H21 | analyze 首轮 `context deadline exceeded` 的 ✗/↻ 行措辞(重试成功后 ✗ 行残留观感) | dead_1g L13-14 | P3 |

## 4. 任务拆解(按批)

> W = wait/perf 面,R = report/投影呈现面,Q = trace_query 面。排序原则:先 P0(卡死/丢维度),再客户点名呈现面,再 P1 隐藏项,P2/P3 收尾。

**Batch 1(P0 + 卡死观感)**
- W1: 流式总超时强制(H1)+ cap 日志矛盾修正(H2)+ 流式退化断路器(C2,precise verbatim 尾块重复信号,中止后走既有重试/降级 lane)
- W2: CLI LLM 等待心跳可见化(C1)+ 渲染行长度上限 + 连续重复段折叠(C2 放大器 A/B;折叠=verbatim 段匹配,display-only)
- Q1: 索引窗 padding 成比例化 + 守卫 graceful degrade(请求窗已覆盖→带 Compaction 成功返回)(H3)+ 缺 pid heavy view 的 analyzer-pinned pid 处理(H4)

**Batch 2(投影呈现面,客户点名)**
- R1: "未定位线程"类型化措辞 + 合并行保线程名 + 图例句(C3+H7)
- R2: 🎯 根标签与 analyzer 实体比对(精确信号:pid/线程名匹配才写‹用户关注线程›,否则‹分析锚点线程›+一句来源说明)+ 用户窗↔投影窗关系行 + berlin 类残差覆盖行补全(C4+H10+H12)
- R3: 树读法/口径逐条低调排版(C5)
- R4: 邻近去重(H6)+ alias 对端合并(H18)+ >100% 标注(H8)+ 聚合形态类型化(H20)+ 下一步去重(H9)

**Batch 3(P1 核实与收尾)**
- Q2: state_drilldown 整窗 idle 噪音过滤(H5,精确信号:source=top_sleep ∧ impact≥窗口99% ∧ 无链相关性→折叠为一行计数)
- Q3: H15/H16 代码核实(assistant 截断点;source auto-resolve 覆盖面)+ H17 payload_ref 提示/read_file 政策说明
- R5: H11 cycle 标注 + H13 快照分母措辞 + H14 轮次口径 + H19 locator 一致性 + H21 措辞(P2/P3 打包)

**W3(方案设计,单列)**: 超长窗(秒级以上稠密 trace)优雅支持——流式 coverage 分段聚合视图(不受 index budget 限制的 bucket 扫描,产出 top-K 热点子窗建议),供模型一步选窗。先出设计段落评审再实施。

## 4.5 代码锚点(探索定稿)

| 面 | 锚点 | 现状 |
|---|---|---|
| H1 流式超时 | `internal/llm/openai.go:272`(流式 http.Client Timeout=0)、`:700`(requestTimeout 降级为 visibleTimeout,每 delta 重置)、`:712-757`(stall watchdog) | 流式无总 wall-clock 硬上限;退化生成三层超时全部失效 |
| 退化检测挂点 | `parseSSEStreamTracked` openai.go:921,contentBuf 累积 `:1021`,onDelta `:1023` | 无检测;挂点干净 |
| H2 cap 日志 | openai.go:448-454;cap=0 语义=无客户端上限(factory.go:22-31,max_output_tokens 无 code default) | 措辞混淆客户端/服务端上限 |
| C1 心跳 | agent.go:4283-4317 watchdog(30s),`observeLLMRequestWaiting` agent.go:4305 已有回调;renderer 无订阅 | 只进日志 |
| C2 双回显 | agent.go:2416-2448:reasoning_content 与 content 各发一次 `EventAgentReasoning` | 两遍上屏 |
| C2 无截断 | renderer.go:1474-1501 遗留 200 字符截断仅旧模式;`stripMarkdown` renderer.go:1415 压平;无全局行上限 | 124KB 原样上屏 |
| H14 轮次 | renderer.go:1872 `round := iteration + 1`,有意 1-based | 与日志 iter 口径差 1,日志侧对齐即可 |
| H3 padding | `traceQueryWindowedIndexTimePadding` trace_query.go:1387-1397(0.05/0.25/0.5 按 view);paddedTimeStart/End parse.go:1405-1421 | 不随窗口成比例 |
| H3 守卫 | parse.go:765 guard;IndexEventLimitError 已携带 FirstTs/LastTs(parse.go:203-218);opts.TimeStart/End=未 pad 请求窗 | 无 graceful degrade;降级判据可用精确信号 FirstTs≤reqStart ∧ LastTs≥reqEnd |
| H4 pid scope | trace_query.go:1329-1349 仅显式 pid 生效;denial 提示 add pid 仅对未 scope 调用 | 无 analyzer-pinned pid 兜底 |
| H5 噪音 | `buildStateDrilldownPlan` query.go:4293-4401;已有碎片化 sleep 过滤,无整窗 idle 过滤;Significant 标志已存在 | idle 整窗行仍入 plan/系统补充 |
| H16 source | trace_query.go:109 schema 含 attached_trace;O3 auto-resolve(有 path 时)已 ship;`resolveAttachedTraceQueryPath` 不认"请求引用"主工件、仅认 --htrace blob | 客户 iter1 无 path → 修复=attached_trace 解析覆盖请求引用主 trace 工件 |
| H17 payload_ref | trace_query.go:2097/2147/2276 打印;结果文本无"不可 read_file"提示;`.codrax` 挡在 builtin.go:215-217/search.go:97 | 提示缺失 |
| C3 未定位线程 | `runtimeTraceCausalProjectionDisplayNodeName` runtime.go:1414(unknown-thread→未定位线程) | 直译占位,无类型化 |
| C4 根标签 | tree.go:521-527 硬编码 ‹用户关注线程›;`model.Target = path[len-1]` tree.go:117-126;无 analyzer 实体比对 | 精确比对缺失 |
| C4 窗口行 | `runtimeTraceProjWindowLine` tree.go:1286-1319;WindowStartTs/EndTs 来自 frame_target_resolution 锚 | 无用户窗↔投影窗关系呈现 |
| H10 残差行 | tree.go:1301-1317,依赖 depth=1 on-chain CumulativeImpactMS>0 | berlin 无 depth1 链节点(下钻全是中转)→静默缺行;需回退口径 |
| C5 图例 | 树读法 tree.go:1181-1191;口径 runtime.go:759-761,均单段 run-on | 改逐条 |
| H7 合并名 | `runtimeTraceProjRowName` tree.go:831-835,MergedCount>1∧Subject=="" → "其余 N 项合并" | 线程名丢失在上游聚合;label 需带成员名 |
| H8 百分比 | tree.go:1048-1050 无上限;bar tree.go:847-863 cap 10 格 | >100% 无标注 |
| H6 邻近去重 | tree.go:373-386:Adjacent 直接追加**无去重**(Background 有 key 去重) | 加 key 去重 |
| H9 下一步 | `runtimeTraceNextStepDedupeKey` runtime.go:2317-2324(typed key 含 runnable_cpu/top_competitor) | payload 异值→key 不同但渲染文本相同;补渲染文本 verbatim 终级去重 |
| H11 环 | `runtimeTraceCausalProjectionRepeatingPath` runtime.go:805-829,仅 len≥6 且超 maxNodes 才检测 | 小环(A→B→A)漏检;补 canonical 节点重现标注 |
| H13 快照分母 | answer_document_mutation_runtime.go:2065 "(占 pct)",分母=线程自身观测总时长 | 措辞标注分母 |

## 4.6 审计修正

- H16 降级:schema 与 auto-resolve(带 path)均已存在;残余=attached_trace 对"请求引用工件"的解析覆盖(并入 Q3 实施而非纯核实)。
- H14 确认 1-based 有意;修复面改为日志 iter 打印对齐(diag 行改 1-based 或双标)。
- H3 padding 并非单一常量而是按 view 三档;成比例化保留各 view 上限、加窗口比例下限。

## 4.7 W3 设计:超长稠密窗的流式 coverage 扫描视图(view=window_sweep)

目标:用户给秒级以上窗口 + 稠密 trace(索引 budget 无法一次覆盖)时,模型一步拿到"哪些子窗值得深入"的确定性建议,替代盲目二分/试错(客户案例:3.3s 窗反复撞 index_event_limit 5+ 轮)。

方案(评审后实施):
- 新增 `trace_query view=window_sweep`,走 event_search 同款**流式扫描通道**(不受 index event budget 限制,复用稀疏锚点索引 seek)。
- 单遍扫请求窗,按 bucket(默认 100ms,`bucket_ms` 参数 clamp 50..500)聚合纯计数信号:sched_switch 次数、目标 pid running/sleep 段计数(pid 提供时)、sched_wakeup 次数、D-state 进入次数、irq entry 计数、trace_mark 计数。内存 O(bucket 数)。
- 输出:top-K(默认 8)热点子窗(目标 pid 状态切换密度优先,无 pid 时全局密度),每个带 [start..end]+密度指标+建议后续 view(状态密度→window_stats/frame_window;wakeup 密度→wakeup_chain);外加紧凑 coverage 表(>40 bucket 折叠)。
- 引导接入:index_event_limit denial 的 recovery_params 在请求窗 >1s 时首推 window_sweep。
- 红线合规:输出全部是**软建议**;bucket 计数是精确整数但只用于排序展示,不进任何硬门。
- 成本预估:1.1GiB trace 单遍 ~2.5s(与 event_search 同量级)。

## 5. 进展

- 2026-07-03: 审计完成,文档落盘;5 路代码探索定稿锚点(§4.5),任务 #21-#31 建立。代码锚点与逐项裁定见各批实现提交。
- 2026-07-03 Batch 1 实现+对抗复核:
  - W1/W2/Q1/Q2/R1/R3 全部落码,全仓测试绿。实现期两个重要自主裁定:(a) degrade 注记走 `Result.Caveats` 而非 Compactions(Compactions 会触发 result_compacted refinement 引导模型再切窗,与修复目的相反;复核独立确认该偏离正确);(b) 心跳限频(WaitTick 2 的幂)放渲染端单点,发射端每 tick 都发。
  - 对抗复核(W 面 3 finding、Q 面 5 finding,全部预验证)裁定与修复:
    - **QF1(P1)** degrade 判据丢"触发预算的事件自身"且乱序 trace 可丢多条窗口内事件 → 重构为 `零时钟回退 ∧ 触发事件 ts>TimeEnd`(单调性+越窗 ⇒ 窗口完整);ts==TimeEnd 不 degrade(防端点丢失)。
    - **QF3** FirstTs<=TimeStart 条件结构冗余(gate+anchor seek 保证 head 覆盖)且挡住空 head 场景 → 删除。
    - **QF4** 比例 padding 对健康构建也缩水窗前 open 状态重建 → 改两级:首次保持原 viewCap padding(零回归),仅窗口未覆盖的预算失败才降比例 padding 重试一次(严格优于旧行为)。
    - **QF2** 整窗被阻塞的受害者线程与 idle 线程同信号被折叠+断言"非根因" → pinned pid/thread 豁免 + 措辞去断言化(中性"整窗睡眠,窗口内无调度活动")。
    - **QF5** degrade 后两条 caveat 矛盾(仍宣称完整 padded 范围已解析,截断无边界 ts) → note 携带 parsed-through ts,windowed_index_parse caveat 按 PaddingTruncatedLastTs 报真实边界。
    - **WF1** 折叠 TrimSpace 比较误伤缩进不同的闭括号行(破坏代码块显示) → verbatim 逐字节比较 + ≥3 连续才折叠。
    - **WF2** 回显去重状态永不清理跨 turn 吞真实 reasoning → 新 LLM 请求事件时重置,范围界定为单响应内。
    - **WF3** 心跳行无归属+并行单元同秒同模型被 lastCommittedLine 吞 → 事件补 ParallelUnit 字段,行带 activity 标签。
  - 复核确认干净的关键面:退化断路器对表格/代码/列表零误伤(整窗逐字节周期联合条件);heavy view 对 padding 事件的窗口过滤无污染(degrade 只少不多);StreamTotalTimeoutError 不进任何重试 allowlist;Emit 并发安全有既有契约。
  - H15 核实关闭(≥1200B no-tool 文本 protocol-only 压缩已存在,agent.go:688);H16 确认真 gap(attached_trace 不认"请求引用"主工件,归 Q3 实施)。
- 2026-07-03 Batch 2 实现+对抗复核(R2/R4/R5a/Q3/R5b):
  - **R2**: 根标签实体比对(AnalyzerHints Entities∪ExactTargets,verbatim/pid 整数;实体空 fail-open 旧标签)+‹分析锚点线程›说明行;双窗关系行(恰两枚时间戳形实体、投影窗<用户窗 50% 才加);H10 残差行回退最浅 HasData 链层。
  - **R4**: Adjacent node-key 去重+同值折叠;peer alias(pid=N↔name-N,span 重叠)在聚合 R1/R2 之间合并防双计;>100% 加"跨CPU/多段累计"标注;irq/page_cache 聚合形态类型化;下一步渲染文本 verbatim 终级去重。
  - **R5a/R5b**: 链上 canonical 重现 ↺ 标注(根首现不标);快照分母措辞"占该线程观测时长";裸 lines= locator 唯一工件补前缀;llm_request 三条 diag 日志加 round=(iter+1,与 CLI 对齐,iter 保持 0-based);非 TTY EventStageEnd 错误行与 TTY 共用 classifyEventError(default→↻ 非终局,终局 ✗ 由 run-end 面兜底——H21 根因是两面对同一事件给相反严重度)。
  - **Q3**: attached_trace 三级解析(blob→内存体→请求引用工件恰一门;权威载体=RuntimeArtifactPreflight.Artifacts);payload_ref 收口单一 helper+审计工件软引导。**H17 实测修正**:read 模式 read_file 能读 payload blob(仅受 bounded 字节墙),提示措辞按实况不写"不可读"。
  - 复核 4 finding 全收:RF1(P1) pid=N 手柄形双向匹配缺失→双向整数匹配+roster 接纳;RF2(P1) Adjacent 等值折叠无重叠守卫(量化碰撞少报)→行号/时间 span 重叠布尔守卫+DedupFold typed 布尔分叉标签("×N同值合并"vs 求和"×N合并");QF1'(P2) 显式 path stat 失败被静默替换→path 非空精确门+拒绝列候选;QF2'(P2) 物理同文件假多候选→os.SameFile 二层去重。
  - W3(window_sweep)设计已评审就绪(§4.7),Batch 3 实施。
- 2026-07-03 Batch 3(W3)交付:`trace_query view=window_sweep` 按 §4.7 全量落地——流式通道(不受 index budget,复用/回写稀疏锚点索引)、bucket 纯计数聚合(bucket_ms clamp 50..500)、top-K 热点软建议(pid 参与密度优先,name-only thread 显式 caveat)、>40 bucket 等距折叠、denial(请求窗>1.0s,未 pad typed 字段判定)prose+typed Refinement 双通道首推 window_sweep。复核 5 finding 全收:不设 Windowed 防 windowed_index_parse 矛盾 caveat;typed PreferredParams 长窗分支切 window_sweep(旧分支字节不变);bucket 边界 ulp 校正(与公布窗口同算式,真实触发形态 4.3/0.05 经枚举验证);budget denial 前 store 前缀 anchors + sweep 自身录 anchors(旗舰恢复流程 GB trace 不再三次全前缀扫描)。全仓测试绿。
- **专项收口**:客户 7 问全对应落地;隐藏 gap H1-H21 全清账(H15 核实非 gap;其余全修或收编);三批共 17 条对抗复核 finding(3×P1)全收零遗留。跟踪余项:window_sweep 实战效果待下次代表性 eval/真实客户 trace 回访验证。
