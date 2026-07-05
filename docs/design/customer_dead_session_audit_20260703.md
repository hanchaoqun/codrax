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
- 2026-07-04 CMP-A(=CMP-1/2/3,§7.2 方案 A)交付:`CompileTraceCausalProjections`/`CompileTraceCausalProjectionSet` 按 typed 工件身份(SourceRef.Path 经 canonpath 归一 → ArtifactID → 证据 locator 路径,字符类切分)分区,每分区复用单工件编译器;≤1 身份走全量记录 legacy 编译(DeepEqual pin 字节等价);无身份记录在多工件账本只进"未归档" caveat;分区上限 4(按观测数保留,caveat 列被略工件)。CMP-2:anchor 窗白名单在 frame_target_resolution 缺席时回退到携带 actual_*(=窗口基准=选定窗,§7.30 裁定6 同判据)且有精确窗(Span ts 或 window note)的 wakeup_causal_aggregate/root_cause_primary 最后一条,frame 锚优先级不变(全部既有 golden 字节保住,全仓测试绿)。渲染:多投影每工件一段("Trace 因果投影 — <basename>",id `_a<N>`,树/明细/证据/尺度 per-工件,结论行 per-段),对比形态(typed:historical_regression ∨ is_cross_component,≥2 投影)前置对比总览表(工件/rank=1 主根因/症状/on-chain/背景压力带单位/投影窗,全 typed 组装);确定性优化点块多工件下按工件重建 E# 并带 basename 前缀。CMP-3:`runtimeTraceProjCrossThreadAggregateType`(subject_kind=aggregate_metric ∧ token∈{supply/cpu/io_pressure,irq_burst/activity,ipi_activity,cpu_frequency_limit},镜像 rootCauseAggregateMetricTypes)——bar 尺度只锚墙钟值(全聚合批 fail-open 批最大防 0 满格),聚合行不画 bar(留空保对齐)、值带"(跨线程累计,非墙钟)"+CMP-9 归一化密度(supply/cpu=≈平均排队深度,其余=≈均值;无窗不估算),明细表窗口投影列同镜像;有线程主体的 H8 irq 突发行(无 subject_kind)保留原 bar+超百标注不受影响。新 pin:分区一致性(同投影全行同工件 locator)/单工件字节等价/上限 4/CMP-2 六分支/双工件端到端两段+对比表+独立窗与尺度+未归档 caveat/CMP-3 成员集+尺度排除+行渲染三面。

## 6. 客户真实 trace 回访验证(2026-07-03,custom_1g.txt,同 berlin 1104MiB 同问题重跑)

**死等类结论:已解决。** window_sweep 第 1 轮即被模型采用,一步产出两个热点子窗(6793224.9-6793225.0 / 6793222.7-6793222.8,46/45 切换),后续直接对 100ms 热点跑 frame_root_cause_bundle;explore 全程 **5 轮**、零 index_event_limit 撞墙、零长等(对照旧会话 12+ 轮 denial 循环 + 14m35s 退化挂死)。心跳/截断/根标签+说明行/双窗行/残差行/逐条图例/↺/超百标注/类型化形态与措辞/合并保名/快照分母/locator 前缀/下一步去重 — 全部按预期出现。

**回访新增 gap(V 系列,同族=合并求和与排序口径)**:

| # | Gap | 证据(custom_1g) | 严重度 |
|---|---|---|---|
| V1 | 主根因结论行按"窗口投影 ms(含 ×N 合并和)"选择,无视引擎 typed rank:rank=1 是 RSUniRenderThre running(E4),结论行给了 rank=2 的 hmfs_discard ×7 求和 13.324ms;与正文叙事(VSync/GPU/binder)自相矛盾 | L84 vs E3/E4 审计行 | P0(呈现) |
| V2 | 残差覆盖行分母=整窗 101ms,但 🎯 目标整窗仅睡 ~11.7ms → "残差 97%"误导;分母应为目标症状(自身状态行)时长,无自身状态时才回退整窗 | L86 | P1 |
| V3 | 背景合并行对 6 个整窗(各 101ms)线程**求和**为 606.000ms 600%,踩"墙钟不可加和"裁定;整窗背景行即 Q2 idle 同类,critical_blocking producer 面漏覆盖 | L128-130, E16-E18 | P1 |
| V4 | 同值(35.350×3)且 span 重叠的 irq_activity 三条走了 ≥3 SUM 聚合成 106.05ms:H6 同值重叠去重只在展示层且 N=2,N≥3 被上游求和抢先 → 重复发布 3 倍计 | L123/E13 | P1 |
| V5 | 明细表合并行节点格丢 roster("对端线程未解析 ×6"),树面已有名单表面没有 | L162 | P2 |

裁定:V1 修复=结论行优先消费引擎 typed rank(rank=1),其次有效归因,禁止 ×N 合并和参与选择(与既往"排序合成分数以 ms 硬事实发布→S1 修根"同类裁定);V3/V4 修复=同值重叠去重前移到聚合层(N 不限)+背景合并行展示 max×N(各≈值)不求和。

## 7. 双 trace 对比场景审计(2026-07-03,custom_compare.txt,bindApplication 7.0 vs 6.0)

客户问题:两个 systrace(389.6MiB/476.6MiB)对比 bindApplication 耗时(1.793s vs 0.884s)差异原因。**探索链路本身健康**(denial→recovery 窗口→window_sweep→root_cause_bundle,7 轮,心跳正常,answer 正文的 CPU 压力 2.18× 结论合理),但**因果投影层在多工件对比场景下基本失效**,暴露一簇结构性 gap:

| # | Gap | 证据(custom_compare) | 严重度 |
|---|---|---|---|
| CMP-1 | **投影不支持多工件**:两个 trace 的观测混进同一棵树/同一 bar 尺度/同一背景段(E1=7.0 与 E2/E3=6.0 平铺为兄弟行;背景两条 supply_pressure 各来自一个 trace);对比问题下单树呈现在语义上就是错的 | L86-95 | P0 |
| CMP-2 | **anchor 窗口缺失回退**:"关注窗口起止未采集",但喂投影的 wakeup_causal_aggregate 观测明确携带精确 query 窗(窗口基准=选定窗);anchor window 来源仅认 frame_target_resolution,非帧类流程全部回退 | L75/L87 vs L209-211 备注 | P1 |
| CMP-3 | **跨线程聚合值裸呈现且当尺度**:supply_pressure 101084.884ms(2.1s 窗内的 cpu·ms 跨线程累计)以 ms 直出、无单位语义标注,且被用作树 bar 满格尺度(807ms 真实行缩成 1 格);per-CPU runnable 等待(19670ms/1.3s 窗)同样裸露流进 prose | L87/L93-94/L114-115 | P1 |
| CMP-4 | **指标快照选题无关**:快照选了 usbDelayTimer/OS_DfxWatchdog(睡眠 24.4s,观测跨度≫问题窗且与 bindApplication 无关);候选未按链上线程/analyzer 实体优先,观测跨度与请求窗不匹配无标注 | L128-130 | P2 |
| CMP-5 | **系统补充噪音**:18 条 blocked_reason 零值行(无 ms 无解释)刷屏;state_drilldown 观测窗完全在问题窗之外(Network d_sleep 20.7s @3683-3703 vs 问题窗 3679-3681.5)无"窗外观测"标注 | L148-163/L184-185 | P2 |
| CMP-6 | **对比类引导缺失**:比较问题(historical_regression=true+双 trace 工件)无 per-trace 同名 span 锚定引导、无对比表呈现形态;"下一步"仍是单 trace 通用建议;bindApplication 所属线程/pid 全程未解析(分析锚在无关 OS_FFRT 线程) | L88-90/L132-136 | P1(引导面) |
| CMP-7 | 杂项:E1 标 on-chain(causality=on_wakeup_chain)但树宣称"无 sched_wakeup 记录"自相矛盾;E3 locator "…systrace:44"(合成观测行号);E4/E5 各为两 trace 的 rank=1 但结论行说"未定位到主根因" | L88/L113/L123-125 | P2 |

裁定方向(待 V 批合入后实施,同文件组):
- CMP-1/2/3 为一组:投影编译按**工件分组**(每 trace 独立投影段,尺度 per-trace;或最小可行=行带工件标签+树按工件分段),anchor 窗口来源扩展到携带"窗口基准=选定窗"的观测(typed note,per-artifact),跨线程聚合类型(supply_pressure/per-CPU runnable)加"跨线程累计(cpu·ms,非墙钟)"标注并**排除出 bar 尺度锚定**(尺度只认墙钟类值)。
- CMP-4/5:快照候选优先链上/实体线程+跨度失配标注;blocked_reason 零值行折叠计数;观测窗与请求窗不相交 → "窗外观测"标注(精确区间比较)。
- CMP-6:软引导面——对比形态(两个 trace 工件 + historical_regression)时 explore skill 提示 per-trace 解析目标 span 所属 tid/pid 并同窗对比;下一步建议模板加对比导向条目。
- CMP-7:on-chain 标签与空链矛盾归 CMP-1 分组时一并核;locator 合成行号收口。

### 7.1 CMP-8:CPU 压力成因下探能力(gap 记录 + 设计)

现状:supply_pressure/cpu_pressure 是**死端聚合**——观测只给 Σrunnable 等待与 per-CPU 分布,回答不了"谁占了 CPU"。custom_compare 案:answer 停在"7.0 CPU 压力 2.18×",占用侧(哪些线程/进程吃掉了核、优先级/频点构成、两版本差异)全程无 typed 下探路径;recommended_views 不指占用侧,state_first_hint 的 runnable 分支只指 scheduler_latency/root_cause_rank(等待侧)。

设计(实施排 CMP 批次):
1) **占用侧分解视图**:扩展 window_stats(或新 view=cpu_contention,倾向扩展避免第二入口):窗口内 top-N running 线程(cpu-time 排序,墙钟内裁剪)、按进程(tgid)聚合、per-CPU top 占用者、优先级段分布(RT/CFS 高/低)、可选频点上下文。全部计数/时长带类型标注(cpu·ms vs 墙钟)。
2) **typed 引导接线**:supply_pressure/cpu_pressure 观测的 recommended_views 指向占用侧;index denial 的 state_first_hint runnable 分支加"先看占用侧 top 占用者";skill prompt 在 runnable/cpu_pressure 主导时软引导占用分解,对比场景引导双 trace 同口径 delta。
3) **对比 delta**:同口径占用表在两 trace 各取一份后,答案面(投影对比总览,见 §7.2)给 per-进程 running 时间 delta 前 N 行——纯 typed 数值组装,不做散文推理。

### 7.2 CMP-ARCH:投影编译器多工件架构方案(设计定稿)

**根因**:`CompileTraceCausalProjection(ledger)` 及整条渲染链是单工件假设——单 anchor 窗、单 🎯、单尺度、观测不按来源分区 → 多 trace 观测混树。

**方案(A,选定):编译入口分区,单工件编译器原样复用。**
- `CompileTraceCausalProjections(ledger) []TraceCausalProjection`:按观测的**工件身份 typed 信号**(SourceRef.Path/artifact id,精确)分区,每个分区独立走既有单工件编译器;无工件身份的观测(合成行号类)进"未归档"桶,只作 caveat 不混树。
- 渲染:每工件一个投影段("Trace 因果投影 — <工件名>"),树/明细/证据/bar 尺度 per-工件;anchor 窗 per-工件解析(CMP-2 的来源扩展同时落此处)。
- **对比总览层**:≥2 投影且对比形态(typed:双 trace 工件 + historical_regression/is_cross_component)时,在各段之前加一张紧凑对比表:per-工件 主根因(rank=1)/目标症状时长/on-chain 归因/背景压力(带 cpu·ms 标注)/窗口;主根因结论行变 per-工件行。表内容全部从 typed 字段组装。
- **结构不变量(加 pin 测试)**:同一棵树/同一尺度内的行必须同工件(硬结构规则,精确信号);尺度锚定只认墙钟类值(CMP-3 并入)。
- **兼容性口径(复核 F7 修正表述)**:"字节不变"仅在**编译层**成立(≤1 工件身份 → legacy 编译 DeepEqual 等价);渲染层对单工件有两处**有意**行为变化并如实披露——(a) CMP-3:带跨线程聚合行(subject_kind=aggregate_metric ∧ token 集合)的单工件案,聚合行不再画 bar/占窗%,改跨线程累计标注+密度;(b) CMP-2:非 frame 单工件案在有 selected_window 观测时锚窗从"起止未采集"变为真实查询窗。既有 golden 通过是因判定条件正确排除(如 custom1g E13-E15 subject 为真实线程)或断言未 pin 到变化面,非全渲染面字节不变。
- 红线合规:不建并行子系统(复用单工件编译器);分区键是 typed 工件身份非启发;对比判定用 analyzer typed 谓词非关键字。

**实施顺序**:V 批合入 → CMP-A(=CMP-1/2/3 按本架构落)→ CMP-B(=CMP-4/5/7 噪音与一致性)→ CMP-C(=CMP-6+CMP-8 引导与占用侧)。

### 7.5 CMP 专项收口(2026-07-04)

四批全部交付并推送:V 批(d3a6bae6,回访 5 gap+复核 5)、CMP-A(8903ff36,多工件架构+复核 7,含 P1 锚窗语义:生产端 selected_window= note+消费端删包络通道)、CMP-B(89f9469c,快照分层/噪音折叠/平铺一致性+复核 2)、CMP-C(74533e20,占用侧四面分解/归一化密度三面/调度压力重标注+compute_supply 三分解/对比引导+单边未采样提示+复核 8:fmax 窗界治理、幽灵 CPU 排除、窗头 runnable 播种+下界披露、idle 判据收敛、compute_supply_balance 观测契约+对比表供给列、census 逻辑 capture 身份归并、TierB lint 盲区、锁步裁定落地)。CMP-1..10 全清账;四轮对抗复核 22 条 finding 全收零遗留。跟踪余项:对比场景实战效果待客户回访/代表性 eval。

### 7.6 对比场景客户回访(2026-07-04,trace_cmp_cust.txt,同 bindApplication 案重跑)

**架构生效**:per-工件双投影段零混树、per-工件锚窗(selected_window 生效)、rank 主根因+合并 lead 单次最大 ×N、残差行、零值折叠、背景 max+roster、快照链上线程+工件前缀 — 全部按预期;分析质量大幅提升(per-trace 定位 span/pid,565ms s_sleep vs 318ms D-state 真差异,答案正文具体到链路)。

**回访新 gap**:
| # | Gap | 严重度 |
|---|---|---|
| NEW-1 | 图例"`└─唤醒─` = 该行唤醒/依赖其父行"方向歧义(客户点名"谁唤醒谁"):唤醒=子→父(对),"依赖其父行"=反向(实际父依赖子)。两个反向动词并置 | P1(客户点名) |
| NEW-2 | **对比总览表/供给列/对比下一步全部缺席**:门=analyzer LLM 谓词(HistoricalRegression∨IsCrossComponent),本次分类未打上(上次 complex/5 实体,本次 moderate/3 实体,LLM 方差)。精确信号教训:≥2 已编译投影是确定性信号,应作总览表主门 | P1 |
| NEW-3 | 同主体多源 IO 行冗余:6.0 树 4 条 IO 行(io_burst 232/226+io_wait 112/107,区间重叠值相近不等,V4 精确等值去重不触发)=同段 IO 两口径×两窗发布;目标自身进程行挂"唤醒 ▸ main"关系语义怪 | P2 |
| NEW-4 | span 锚定问题(用户给时长非时间戳)双窗关系行不出:用户窗来源可扩展到已定位 span 窗(typed span 观测) | P3 |
| NEW-5 | "系统按已验证证据补充缺失成员"表列头"符号名称"装唤醒链描述,列头错位 | P3 |

裁定:NEW-1 图例改"该行唤醒其父行(父行依赖该行)";NEW-2 总览表门改为 ≥2 Active 投影(确定性),analyzer 谓词降为 next-step 对比条目与 skill directive 的门(prompt 期无 ledger 只能用谓词+census,保持);NEW-3 归为同主体 IO 多源聚合(burst⊇wait 的口径分组显示);NEW-4/5 小修。

| NEW-6 | 覆盖行与自身口径行的关系未自解释(客户追问"残差包含 on-chain 吗"暴露):6.0 案"残差 90%"与树内"主根因"IO 行(232ms 等)表面矛盾——IO 行与目标 D 态是同段墙钟另口径,防双计故不入"已归因",但读者需自行脑补。修复:存在目标自身/同进程 IO·阻塞口径行时,覆盖行追加"残差中最大 X ms 与自身 IO 口径行(E# 组)重叠解释,未计入链归因以防双计"(取 NEW-3 分组主行值,verbatim 证据 ID) | P2 |

**残差语义定稿(入图例/文档口径)**:残差=目标症状时长−唤醒链量化归因,是 on-chain 归因的补集(不含之);自身口径行(window_stats/critical_blocking 面)解释残差但不入归因分子。

| NEW-7 | 树读法图例静态,与树实际内容脱节(客户点名):真实出现的 🎯/⏳/⚙/⛓/◦/├─下钻─/⚠(树行)/↺ 无图例条目;⛔/成因 等未出现时仍被解释。修复:图例**动态生成**——渲染期记录实际发出的符号/边/标记 typed 集合,图例只渲染出现过的条目;建立渲染器全量符号目录 + 双向结构 pin(出现必有解释/解释必有出现),新增符号漏配目录直接炸测试 | P1(客户点名) |
| NEW-8 | "窗口基准: 选定窗"不渲染窗口起止,尾注"见原始 trace_query 记录"指向用户面板不可查的 blob(客户点名)。端点数据已在(CMP-A selected_window= typed note),缺:(a) producer 覆盖面——state_churn/快照、critical_blocking、state_drilldown 等窗口化观测族统一带 selected_window= note(单一 helper);(b) 显示面——快照行与系统补充行在 note 存在时渲染"选定窗 X.XXXs–Y.YYYs",缺失回退现措辞;"见原始记录"降为补充语 | P1(客户点名) |
| NEW-9 | 来源结果容量截断未在最终报告披露(客户注意到模型自述"6.0 root_cause_rank 似乎被截断"):截断本身是 E4 容量治理设计内行为(按 rank 保头部,模型 1 轮 window_stats 恢复,结论建立在未截断头部+补全数据上,无实质影响),但 typed Compactions 信号只在工具结果上,投影段/证据索引不渲染——读者只能从过程日志推断。修复=显示接线:喂给某工件投影的结果带容量截断时,该段证据索引头加"部分来源结果按容量截断(rank 头部完整保留)" | P3 |
| NEW-10 | 投影树 fence 在 markdown/HTML 查看器中对齐漂移与强制换行(客户点名):根因=网页等宽字体 CJK≈1.6-1.8×/emoji 宽度不定(终端按 2 列 pad)+部分查看器 pre-wrap 折行。裁定:**不做 per-surface 第二套内容渲染**(v3 三端一致防内容漂移红线);改进=(1) 答案面树行显示宽度硬上限收紧 ≤100 列(单行完整性优先,行内锚定序保证跨行漂移不损单行可读);(2) 记号区规整:每行状态记号恒 1 glyph(缺省补 ◦)、全 fence 禁 tab;(3) docs 增 portal 呈现指引(pre 样式+CJK 等宽字体栈);(4) 结构 pin:行宽≤预算/记号恒1/无 tab。明细表(markdown 原生)本就是无损兜底 | P2(客户点名) |

**supply_pressure token 终局裁定(2026-07-04,用户裁定,项关闭)**:**token 保留 + 显示层分离**。wire token `supply_pressure` 永久保留不迁移;需求积压语义由显示层承担(全部显示面经唯一 helper `runtimeTraceSupplyPressureDisplayLabel` 渲染"调度压力(需求积压)"/"scheduling pressure (demand backlog)",明细表"类型"列与叙事括注保留 verbatim token 作审计对账)。理由:(1) LLM 与客户只消费显示文本,token 纯内部命名;(2) 迁移成本(约 12 个生产点+全量 goldens+旧 session 工件别名过渡)只换内部命名一致性,风险收益比不成立;(3) verbatim token 保住与历史 session/外部日志的可对账性。R2' 适用性判定:已交付的 CMP 改动均为系统内部确定性信号(非 LLM-emit 契约面),六处同步无触发点;本裁定后 token 迁移不再是开放项。后续任何人重提改名,先读本段。

**[账本卫生标注 2026-07-05] NEW-1..10 已交付** — commit `7f76ed77`(2026-07-04,comparison-revisit hardening,含两轮对抗复核修正);NEW-10 的 portal 呈现指引即 §7.7(其 F-2/F-3 收紧记录亦在 §7.7)。supply_pressure token 终局裁定(上段)为**关闭项**,重提先读该段与账本 §7.4/§7.5。

### 7.8 VS-1:周期性信号源的因果计数口径(客户裁定,设计定稿)

**问题(vsync_cust.txt,旧版输出+代码核对确认)**:VSync 是固定周期的帧信号发生器,期内睡眠是正常节拍;但 `causalImpactBlockingMs`(query.go:11730)对 sleep 主导 occurrence 全额计 impact,零周期性感知 → VSyncGenerator 每 8.33ms 周期内 5-6ms 正常空闲、主线程逐帧等待的 5.8-6.4ms 正常睡眠(E10 合计 36.256ms)全被计为根因影响;旧答案据此把"5.2-6.2ms sleep 波动"叙述成抖动根因——数据自证信号没迟到(actual_window 跨度 8.287-8.360ms,方差 ±0.04ms)。

**客户口径(裁定)**:周期性信号对上,只有 (a) runnable 等待(要 CPU 没拿到)与 (b) 信号比预期周期**迟到的超出量**才计入 on-chain 根因;期内正常 sleep 不计。

**设计**:
1) **周期检测(确定性,禁线程名关键字)**:同一 (waker→target) 唤醒聚合 occurrences≥3 时,取相邻 actual_window 起点间隔序列,中位数 p;全部间隔 ∈ [p×0.85, p×1.15](tol 常量注明)→ 判定周期性唤醒对,p=检测周期。数据已全在 aggregate 的 occurrence_windows,零新解析。
2) **计数修正(engine,软面数值)**:周期对的 sleep 主导 occurrence,有效 impact = runnable 全额 + max(0, 实际间隔−p)(迟到量);期内 sleep 不计。raw 值无损保留;新 typed 字段 PeriodicSource/DetectedPeriodMs/LatenessMs。
3) **呈现**:此类行标"周期性信号源(期内睡眠为正常节拍,周期≈p ms)";有效归因列用折减值,窗口投影保留 raw;主根因选择消费折减值(与 V1 口径一致);图例/口径补条目。
4) **红线合规**:15% 容差是噪音阈值 → 只驱动软面(计数/标签/排序),不进任何硬门;runnable 与迟到量是精确算术。berlin 案预期效果:E1/E10 折减至 ≈抖动和(≈0.2ms),真实 impact 行(RSUniRender running 4.115/binder 4.577/runnable 片段)自然上浮。

排队:回访批推送后作为下一批(VS 批)实施。

**[账本卫生标注 2026-07-05] 已交付** — VS-1 于 2026-07-04 落地(commit `ea74a11c`,对抗复核 6 finding 全收;F1 高危=检测输入是分支选择后的等待段、宽窗可伪造迟到,已按阻塞口径修根)。交付形态较本节设计稿收紧:周期检测=同 (waker→target) **≥5** 次 sleep 主导 occurrence(设计稿 ≥3)+ lower-median 周期 + k·p 缺口剔除(分支 top-N 免疫)+ 早发 >15% veto/晚发不 veto(晚发超出量就是 finding)+ ≥2/3 带内门(设计稿全带内);Effective=runnable+迟到量且**权威 0**(typed PeriodicSource 短路一切可能复活 raw sleep 的 fallback);raw 无损,rank/Score/V1 结论选择器/对比总览主 cell/覆盖行全消费折减值,覆盖行第三项"期内正常节拍"(不计归因也不属残差),周期行带图例条目。**berlin 实证:36.256ms→0.105ms**(仅 runnable;0.071ms 带内抖动正确不计迟到),真实 impact(RSUniRender running 4.115ms/binder 4.577ms)上浮为主导。"周期节拍 lateness=0" 裁定回归 pin 已收编入 `semantic_ruling_pins_test.go` pin 族索引(见 §7.9 RN-16 落地注记)。

### 7.9 RN 系列:runnable 主导场景审计(2026-07-04,cust_runnable.txt + cust_large_3s.txt)

客户点名:对比场景"优先级反转因果链丢了,背景调度压力成主因(本身也对)但 on-chain 主因未提及"。诊断:本轮模型把 7.0 锚在 FFRT 线程(runnable 主导,sleep=0,无唤醒边)→ 平铺单行;占用者数据(CMP-8)未发布为观测 → 报告无机制解释;反转链上轮存在是因为那轮跑了 main 的 wakeup_chain(模型探索方差)。大窗案(cust_large_3s)VS-1/主根因/反转行全部正常。

| # | Gap | 级 |
|---|---|---|
| RN-1 | cpu_occupancy 不发布 ledger 观测,runnable 行无"同窗占用者"归因;平铺 runnable 42% 行无机制解释 | P1 |
| RN-3 | 结论行"未定位链上主根因"与背景行"主根因·主要关注"标签矛盾(tier 直投标签);平铺 on-chain 行不参与 lead | P1 |
| RN-5 | 锚白名单过窄:wakeup_causal_impact 族(同查询窗语义)应准入;state_churn 等微探族维持排除 | P1 |
| RN-2b | 无锚窗时 ⚠跨窗 照发,自相矛盾 | P1 |
| RN-6 | 症状分母不含 runnable → runnable 主导目标症状"—";应纳入(措辞改"目标等待(睡眠/阻塞/就绪)") | P1 |
| RN-11 | 完成门对 runnable 主导行强制 wakeup_chain 下钻(应为 occupancy/scheduler_latency 面) | P2 |
| RN-8 | 背景整窗 D/IO 行无疑似空闲标注(StateKind 空但 type token 可用)→ 模型误读"IO 突发" | P2 |
| RN-4 | systrace comm 截断占位 `<...>-N` 直出 → 类型化"线程名未记录(tid N)" | P2 |
| RN-10 | 同主体多类型行分散(running/反转/runnable 4 行)— 记录,暂不合并(类型不同信息各自承重) | P3(记录) |

| RN-12 | 平铺/链上 runnable 行覆盖自解释缺失(客户追问"树看起来截断"):树行 635.981ms(链视图 top 片段,受 occurrence/分支上限)与同账本 state_drilldown 全量 runnable 2528.721ms 无交叉引用,读者观感=树被截断。修复:同账本存在该线程全量 runnable 观测时行尾注"窗内 runnable 合计 X,链上仅覆盖 top 片段 Y(Z%)"(typed 交叉引用零新查询) | P1 |
| RN-13 | 平铺模式 header 缺锚点说明:锚=模型选定线程(runnable 退化锚,无唤醒边→平铺)且非用户实体时,header 无任何说明(R2 机制只覆盖 🎯 模式);next-step 无"对用户关注线程补跑 wakeup_chain 恢复因果树"引导 | P2 |

| RN-14 | **关注线程链恢复三层**(用户提问定稿):(a) wakeup_chain 加 via_thread 参数——在 focus 线程链树中优先展开经候选 X 的路径+逐跳延迟;X 不在链上时明确输出"X 与 T 无唤醒边连接,影响仅为**调度竞争(就绪排队)**"(链上根因 vs 调度竞争的裁定性区分,唤醒边图已有零新解析;措辞遵 §7.4:runnable=调度压力,"算力"只留 compute_supply 交付口径——用户 2026-07-04 再次裁定) | P1 |
| RN-15 | **runnable 等待双重发布且错挂 compute_supply token(实证 bug,违反 §7.4 需求/供给分离裁定)**:cust_large_3s 系统补充同一 runnable 等待(2.661/2.908ms)发布两次——`type=compute_supply, source=window_stats.compute_supply` 与 `type=runnable_wait, source=window_stats`;投影 E8/E9 行"影响点 compute_supply"。修复:审计 CMP-C 占用/供给接线,per-thread runnable 等待只走 runnable_wait/调度压力族;compute_supply 观测族只装交付侧(供给率/低频折损/闲置错配/核数受限,聚合级非 per-thread);消除双发;显示面"影响点"随 token 修正;全 display 串扫一遍"算力"误用于 runnable 语义处 | P1 |

| RN-16 | **语义约束机械看护**(用户提问定稿,防后人误违反):(1) causal token 语义车道注册表(单一事实源:token→{Lane 需求/交付/链/IO/IRQ、Additivity 墙钟/跨线程cpu·ms/计数、Subject AggregateOnly?、显示 label}),观测/rank 构造收 typed 常量非裸串,golden snapshot 仿 B0 69-kind;(2) 裁定回归 pin 族 semantic_ruling_pins_test.go:RN-15 形态(compute_supply 禁 per-thread runnable)、加和车道表驱动扫描(CrossThreadCPUms 禁入 Σ/bar 面)、周期权威 0 等既有 pin 收编同族;(3) 措辞 lint 仿 glossary:"算力"只许出现在 LaneComputeDelivery label 白名单位置,"调度压力"同理;(4) 文档锚在决策点:注册表头注引 §7.4/§7.5,architecture.md 红线段+CLAUDE.md 指针 | P1 |

**RN-16 落地(2026-07-04,RN-E 批)**:四层全部交付。(1) `internal/tracequery/causal_token_registry.go`——49 token 全集注册(grep 审计:rootCauseItem 全 call site+causalImpactRootType/aggregateRootCauseType/stateChurnRootCauseType/traceSpanSemanticWorkClass+chain RootEvidence+critical_blocking 构造+lock_contention kind+runnable_occupancy/compute_supply_balance 观测谓词),9 车道×3 加和类×3 subject 类+RowToken(行 token vs 观测/kind-only)+LabelZhRef(label 不迁移只引注归属 helper);构造收编取**最小侵入面**=两个既有 funnel(`rootCauseItem`+critical_blocking `add`)各插一行 `assertCausalTokenRow`(零签名改动,RN-A..D 未提交面未扰动),未注册 token/RowToken=false 上行面/aggregate-only 带 pid>0 或 comm → `testing.Testing()` 下 panic、生产 `logging.Warning`;golden=`causal_token_registry_golden_test.go`(49 行 `token|lane|additivity|subject|row|zhref` 全字段快照,仿 69-kind)。(2) pin 族双侧:tracequery 侧 `semantic_ruling_pins_test.go`——RN-15 形态 3 个 testdata fixture 全管线扫描(rank+evidence 面,泛化到全部 aggregate-only token)、engine `rootCauseAggregateMetricTypes`==注册表 CrossThreadCPUms 行集合相等互锁、§7.4/§7.5 车道裁定 verbatim pin(supply_pressure=需求车道+label 归 `runtimeTraceSupplyPressureDisplayLabel`;compute_supply/balance=交付车道 AggregateOnly)、跨轴不变量(AggregateOnly⇒非墙钟、per-thread 行⇒非跨线程加和)、guard 机制自 pin(panic/不 panic 双向);文件头=既有裁定 pin 族清单索引(RN-15/VS-1 周期节拍 lateness=0/墙钟不可加和/CMP-3 各留原文件,只列指针不迁移)。tool 侧 `semantic_ruling_pins_test.go`——`runtimeTraceProjCrossThreadAggregateType` 与注册表逐 token 相等 pin(TypeToken+Object 双 lane;无 aggregate_metric 标记恒 false 也 pin),LabelZhRef 列与 `runtimeTraceRootCauseTypeZHLabel`/`runtimeTraceSupplyPressureDisplayLabel` 双向锁(注册表说有 label 而 helper 返空、或 helper 有 label 而注册表标 verbatim,均炸)。互锁例外单点=`compute_supply`(注册 CrossThreadCPUms 但两消费集合今日不含——RN-15 杀 per-thread 面、producer 无 threadless 行,生产零聚合行;例外表 `causalRegistryCrossThreadRowExceptions` 自诚实 pin:条目进任一消费集合即要求删除例外)。(3) 措辞 lint=`internal/tool/semantic_wording_lint_test.go`(go/ast 扫 internal/tool+internal/tracequery 非测试文件字符串字面量;"算力"白名单 5 处 file::func(typelabels 供给 label/对比总览供给列/action·meaning 消费侧两 cell/compute_supply_balance stanza),"调度压力"/"需求积压"单源 `runtimeTraceSupplyPressureDisplayLabel`;violation 输出 file:line,白名单条目失配报 stale 防 rot)。(4) 文档锚:注册表头注(§7.4 裁定原文+§7.5 token 保留终局+改前必读/golden 同步四步协议)、architecture.md §7.2.1 新增"因果 token 语义车道红线"段、CLAUDE.md Repomap 红线下追加单行指针。`go test ./internal/tracequery/ ./internal/tool/` 全绿(guard 全程启用);VS-2 新 token(§7.10 (5))按注册表准入协议走 golden 增行。

**[账本卫生标注 2026-07-05] RN 系列全量交付(RN-16 见上段落地注记;其余各批证据如下)** — engine 半场 commit `7c5c236d`:RN-11(完成门对 runnable 主导行改荐 scheduler_latency/occupancy 面)、RN-14(wakeup_chain via_thread,NOT-on-chain 判语="仅调度竞争(就绪排队)")、RN-15(per-thread runnable 只走需求车道,compute_supply 聚合 only 构造守卫封双发)。report 半场 commit `5dc90edc`:RN-1(同窗占用者 roster)、RN-2b(⚠跨窗 门控,代码锚 `answer_document_mutation_runtime_tree.go:2767`)、RN-3(结论行回退最大 on-chain 平铺行+"主根因"标签随实际消费)、RN-4(`<...>-N`→"线程名未记录(tid N)")、RN-5(锚窗回退准入 wakeup_causal_impact 族)、RN-6("目标等待(睡眠/阻塞/就绪)"分母纳入 runnable)、RN-8(整窗等待(疑似空闲)标注)、RN-12/13(平铺自解释:全量 runnable 交叉引用+header 锚点说明+next-step wakeup_chain 引导)、显著门 min(窗10%,100ms)。文档收口 commit `7332a5f0`。**RN-10 非交付项**:维持"记录、暂不合并"裁定,按设计不动。

### 7.10 VS-2:on-chain running 节点供给折算缺口与共同根因(用户提案 2026-07-04,审视后定稿)

**规则**:on-chain 节点 running>runnable 时,对其 running slice 按"大核最高频点"折算理想时长(逐 slice:该 slice 所在 CPU 治理频点 vs 大核簇 fmax;core_class/频点时间线已有,零新解析),**供给折算缺口 = running 墙钟 − 折算理想时长**(措辞钉死"按频点折算,不含微架构差异,缺口为下界")。决策表:缺口占比高(≥节点 running 20% ∧ ≥1ms 地板)∧ runnable 显著(**复用 RN-1 ≥窗10% 门**,同源防分叉)→ 供给缺口+调度压力共同根因;再有反转 → 三机制并列;runnable 不显著 → 供给缺口为主因,其余按 rank 常规排;**无缺口(满频满核)→ 肯定性标注"running 属真实工作量"**(第四分支,排除性裁定同 via_thread NOT 案价值)。

**显著门修正(用户 2026-07-04 二审:纯相对阈值被宽窗稀释——3.3s 窗下 10%=330ms,200ms 绝对巨量积压(24 个 120fps 帧预算)漏判)**:RN-1/VS-2 共用显著门改为 **runnable_total ≥ min(窗长×10%, 100ms)**(相对∨绝对双基准单公式;窄窗相对基准生效,宽窗 100ms 绝对地板接管;窗越宽门单调更松,消灭稀释反直觉;两常量 typed+pin;仍软面专用宁松勿紧)。RN-B 已落的 0.10 单基准实现与 299.999/300.000 pin 随本裁定修正。

**红线约束(审视结论)**:(1) 阈值为噪音信号,只驱动软面(标签/叙事),不进硬门;(2) 共同根因禁止合成总分——三机制口径不同不可加和(S1+墙钟裁定),呈现=主根因行接机制构成从句各带单位,**排序仍走 rank/有效归因不变**;(3) 命名"供给折算缺口",与反转"运行折算"分车道(RN-16 措辞 lint 各锁各);(4) 折算复用 VS-1 治理频点时间线口径(窗前历史不参与);(5) 新 typed 字段(SupplyFoldDeficitMS/IdealFoldedMS/FoldBasis)+ token 入 RN-16 注册表(Lane=ComputeDelivery,SubjectKind=PerThread——注意:这是**交付侧 per-thread 合法形态**(折算自该线程自身 running,非聚合),注册表 SubjectKind 约束按 token 分置,compute_supply 聚合 token 的 AggregateOnly 不变)。

**VS-2b fmax 取数阶梯(用户追问定稿)**:各核最高频点三级来源——(1) sysfs 专用接口(cpuinfo_max_freq 硬件额定/scaling_max_freq 策略上限)离线 trace 不可得;(2) **trace 内 cpu_frequency_limits 事件(cpufreq 策略上限,引擎已解析 WindowStats.CPUFrequencyLimits per-CPU Min/Max)为离线最权威,存在时优先作 fmax**;(3) 观测治理时间线最高样点为回退(下界)。caveat 如实:limits 只在变更时发射(窗口治理语义取最近前置,缺席即回退观测级);limits 是策略上限含温控压频,非硬件额定("下界"措辞保留)。**附带 finding(零新解析)**:大核簇 limits.Max < 该簇全程观测更高频点 → 独立"策略/温控压频"结论(缺口部分源于压频而非调度)。FoldBasis 记录每 slice 的 limit/observed/unknown 来源构成。

**VS-2c 簇频快速接口(用户追问定稿)**:泳道数据现状——C| 簇频 counter 泳道 max 已有 O(1)(T3 CounterDeltas 按名 keyed);clock_set_rate 簇时钟已解析含名**但仅计数,rate 值未保留**;无统一 per-簇 fmax 接口。设计:`WindowStats.ClusterFrequencyCeilings []{CoreClass,CPUs,FmaxKHz,Source(limit|observed)}` 窗口构建期一次计算、消费者 O(1)(VS-2 fold/compute_supply/显示共用);阶梯=per-CPU limits > 观测 max,按 core_class 聚簇,**全 cpu_id 键控精确信号**;**簇泳道角色裁定(红线)**:counter/clock 泳道名(cpu-cluster.0/厂商名)是 SoC 相关名字形态,簇归属禁名字猜测——泳道 max 只作旁证 caveat("簇泳道最高 X 与 fmax 一致/不一致"),不作 fold 基准;clock_set_rate 升级保留 rate 值作旁证源。

**VS-2c 终局裁定(hmtrace+hiview 参考库研究,2026-07-04,证据在案)**:cpu-cluster.<N> **不是**机器封闭 token——两库 0 命中该字样;clock 名=厂商 DTS 自由词汇(trace_streamer 原样透传,db2systrace.py:495 注释自证异构);参考实现对 clock_set_rate 不做任何簇/CPU 绑定(hmtrace counters.rs:61 重发 cpu 硬编码 0);簇→CPU 权威映射在 sysfs policy 目录(hiview cpu_core_info_catcher.cpp:30-43 且为 12 核平台硬编码),trace 工件内无。**故簇泳道维持旁证 caveat 角色,fmax 阶梯维持 limits>观测(cpu_id 键控)**。附:harmony flavor 可 pin 封闭 token 清单=cpu_frequency/cpu_idle/cpu_frequency_limits{,_min,_max}(均带 cpu_id 键,counters.rs:15-31 佐证);clock_set_rate 仅事件外形可 pin,载荷名禁入封闭集;codrax 既有 isCPUFrequencyClock 名字启发(parse.go:2355)只准喂软引导,禁作 fmax 基准(复核轮加 pin)。克隆物留 scratchpad/refstudy 供复查。

排 VS-2 批,依赖 RN-E(注册表先立,新 token 走注册表准入)。VS-2b/VS-2c 并入 VS-2 批实施(在途 lane 未含者由复核修正轮补齐)。

裁定:RN-A 批=投影/显示面(RN-3 lead 平铺回退+标签一致、RN-2b、RN-5、RN-6、RN-8、RN-4);RN-B 批=engine/观测/门(RN-1 占用者观测发布+行尾注、RN-11);RN-C 批=RN-12/13(依赖 A/B 落地后同文件组);RN-D 批=RN-14 三层+RN-15(依赖 B 落地);RN-E 批=RN-16 看护(依赖 D 落地——注册表收编 D 修正后的 token 全集);VS-2 批=§7.10(依赖 E)。"锚点退化"定性:runnable 主导锚无唤醒边是数据事实,平铺本身正确;系统欠的是覆盖披露(RN-12)、锚点说明(RN-13)、占用者机制(RN-1)三件让平铺模式自解释的事。RN-3 标签规则=行"主根因"标签只跟随结论行实际消费(未被消费的 primary-tier 背景行降"背景·支撑参考+rank 注记");RN-1 观测=显著 runnable(≥窗 10%,精确比较)时发布 top-3 同窗占用者(typed,per-CPU 数据已在)。

**[账本卫生标注 2026-07-05] VS-2 批已交付(含 VS-2b/2c)** — engine commit `7c5c236d`:per-slice 频点折算(fmax 阶梯=policy cpu_frequency_limits 优先、观测治理时间线回退、厂商簇泳道仅旁证不作基准)、identity-pin ideal+deficit==running、压频如实披露、unknown slice 不伪造缺口;report commit `5dc90edc`:共同根因机制构成从句(各带单位不加和、排序不动)、"满频大核运行=真实工作量"肯定性分支、"频点数据不完整"诚实分支;显著门修正 min(窗10%,100ms) 同批落地。VS-2b/2c 落地注记见 §7.11 末行(批 P 条目)。

### 7.11 PP:泳道+事件解析平行审计(hmtrace/hiview 对照,2026-07-04,全量裁定见审计报告)

**A 类需修正(2)**:A-1 数值字段不容忍浮点串——hmtrace REAL 列输出 "N.0" 形态,Atoi 静默清零,cpu_idle/cpu_frequency/limits/clock_set_rate 全部受害 → hmtrace 风味整窗频点时间线 fail-quiet 消失(修法=Atoi 失败回退 ParseFloat 截断,双容忍,不可改纯 float——export_format.rs 声明 INT 两态并存);A-2 kv 未引号值空格截断损坏 sched comm("Signal Catcher"→"Signal";PID 键控聚合不受影响,修法=sched comm 键边界回溯,不动全局 kvRE)。
**B 类细化(6)**:B-1 prev_state 首字符覆盖集扩 I/T/t/X/Z(I 入睡眠族;HasPrefix 方向被参考消费端佐证);B-2 clock_set_rate 无键位置格式 rate 提取(**VS-2c 补充新证据**:不加位置回退旁证源在 hmtrace 风味为空);B-3 SpanPID 软推导进程分组(hitrace 专项批;task_rename 独立解析明说不做——newcomm≡oldcomm 零信息;裁定:混合工件(任一原生 TGID>0)整体禁用推导,fail-open 到无分组,禁用时记 WARN 日志非静默);B-4 print 非 mark 载荷类型化(低);B-5 合成泳道名词表只准进软引导(低);B-6 hiview transaction_proc **不做**(非 trace 面)。
**C 类已更优防倒退(10,禁"对齐"回退)**:limits min/max 保真 vs hmtrace 有损折叠、waking/wakeup 并集、无名 E| 等价、H: 原样保留佐证、行首超集、softirq 尾括、簇判定无硬编码(vs hiview 12 核 policy 硬编码)、binder ftrace 族独有超集(两库 0 命中,防"参考没有所以删")、counter ParseFloat 已容忍、async 三元组精确。无动作观察 3 条(hmtrace 生成端固有:slice 末端 ts/wakeup CPU 缺省/dma_fence 单栈)入档防重查。
**批次**:批 P=A-1+A-2+B-1(先行,含 hmtrace 风味回归 fixture);B-2 并入 VS-2b/2c 修正轮;B-3 hitrace 专项;B-4/5 低优先软词表。
**批 P+VS-2b/c 落地注记(2026-07-04)**:全项交付;VS-2c(b) 实施中发现并封堵真实泄漏——parse.go:2090 名字启发把 cpu-freq 名泳道重分类 EventCPUFrequency,泳道样本(hmtrace CPU 硬编码 0)可混入链面频点索引,buildFreqIndex 已按 Name==clock_set_rate 精确排除+三探针守护 pin;B-1 裁定=T/t/X/Z 入新 StateStopped/StateDead typed 类而非塞既有五态 lane(压力面语义诚实);**遗留(排 ClusterFrequencyCeilings 接口批)**:重分类泳道事件仍进 window-stats freqByCPU(既有 keyed 行为,fold 面已隔离,window 面待接口批统一收口)。

### 7.7 Portal 呈现指引(NEW-10 配套,2026-07-04)

投影树 fence 内容三端一致(HTML / markdown / 终端字节相同,v3 红线,无 per-surface 第二套渲染);网页端能否对齐取决于 portal 的 CSS。接入方建议:

- **pre 样式**:代码块容器用 `white-space: pre; overflow-x: auto;` —— 禁止 pre-wrap 折行,超宽横向滚动。答案面树行显示宽度硬上限 100 列(回访聚焦复核 F-2,2026-07-04:NEW-3 口径注/D3 影响构成等超宽 typed 承重注记不再豁免溢出,自动换行为前缀对齐的 ↳ 续行,内容不截断;F-3:共享 label 列另有 50 列封顶——由行预算减最小 bar/ms/存根区精确推导——深链/长名行按 B1 语义加剧名称截断,🎯 头永不截断,明细表保持无损全名),容器给到 ~100ch 即基本无滚动。
- **CJK 等宽字体栈**:对齐要求 CJK 恰为 ASCII 两倍宽,推荐 `font-family: "Sarasa Term SC", "Sarasa Mono SC", "Noto Sans Mono CJK SC", ui-monospace, monospace;`。缺 CJK 等宽字体时 CJK≈1.6-1.8× 会产生列漂移——NEW-10 已将每行状态记号规整为恒 1 glyph、全 fence 无 tab,漂移收敛为同深度行的常量偏移,单行内锚定序(记号→名称→bar→ms→%→tag→E#)不受影响。
- **无损兜底**:fence 只承担美观与单行完整性;全部数值、关系、⚠ 实际值、被截断的 tag/名称均在明细表(markdown 原生表格,不受字体影响)与证据索引中无损可查。

### 7.3 CMP-9:跨 trace 聚合对比必须归一化(用户追问暴露,P0 级口径缺陷)

用户追问"55 秒 cpu·ms 怎么算的"复核发现:custom_compare 案的头条结论 **"CPU 压力 2.18×" 大部分是窗长差假象**——两个 supply_pressure 聚合来自不等长分析窗(7.0: 1.3s vs 6.0: 0.7s,窗长差 1.86×),按窗长归一化(=平均运行队列深度)后 77.8 vs 66.2,仅 **1.18×**;方向成立、量级失真。IO 结论归一化后反而更强(密度 4.7× vs 原文 2.5×)。且两窗均未对齐 bindApplication span 本身(覆盖 73%/79%)。

语义澄清(入文档为准):supply_pressure 单位=跨线程求和的线程·毫秒(瞬时贡献=当时排队线程数×墙钟),窗内值/窗长=平均运行队列深度——这是该聚合唯一可跨窗比较的口径。

裁定(并入 CMP-A/CMP-8 实施):
1) supply_pressure/per-CPU runnable 等聚合的工具输出与观测行,一律附**归一化密度**(值/窗长,标注"≈平均排队深度")与窗长;
2) 对比总览/delta 表(§7.2/§7.1)**只用归一化口径**呈现跨 trace 比值,原始和值仅作明细注记;两侧窗长差 >10%(精确比较)时强制标注"窗长不等,已归一化";
3) 软引导:对比形态下 skill/next-step 提示模型对齐目标 span 边界后再取聚合(span 两端 typed 时间戳可得)。

### 7.4 CMP-10:supply_pressure 语义错位 — 需求/供给口径分离(用户裁定,设计定稿)

**问题**:现 `supply_pressure` = Σrunnable 等待 + 高优先级竞争 = **需求侧积压**(调度压力,PSI stall 同族),命名"供给"误导;排队深度大区分不了"要得多"还是"给不够"。

**设计(并入 CMP-C 批实施)**:
1) **改名/双标(先呈现层)**:观测与答案面把现指标呈现为"调度压力(需求积压,≈平均排队深度 N)";wire token(type=supply_pressure)迁移是 R2' 六处同步事项,首批只做 display 重标注+文档,token 迁移单独裁定(带别名过渡)。
2) **新增真供给口径 compute_supply**:per-CPU `running×freq/max_freq` 加权交付算力(频点链路已存在:frequencyAt/freqByCPU/CounterDeltas,纯确定性);供给率=交付/名义(窗长×核数);**供给缺口三分解**:低频折损(running 低频段折损量)/闲置错配(CPU idle ∧ runnable 队列非空的时长=调度亲和问题非算力问题)/核数受限。全部带类型标注与归一化(CMP-9 口径)。
3) **对比呈现**:delta 表按"需求积压 delta"与"供给能力 delta"两行分开(再接 CMP-8 占用侧"需求被谁吃"),直接回答"要得多 vs 给不够"。custom_compare 案实证价值:归一化后排队深度仅差 18%,900ms 差异的真正归属需供给口径裁定。
4) 引导:state_first_hint 的 running 分支已有 compute-supply 字样,统一到新口径命名;skill 提示对比场景两口径都取。

### 7.12 DR/DL:data-lane 单发路由死线 + workflow 三面撒谎死锁(EVAL 战役 data-planner 复跑归因,2026-07-05)

**缘起**:联合 eval 战役阶段 2 复跑 data-planner 3 案(data_basic_sum_with_rules / data_json_strict_ids / data_text_filter_count),FAIL 按"不放过"纪律行为链归因,拆出两个系统类 + 三个残余子类。

**DR 类:单发路由死线(已灭绝,15/15 验收)**。归因:REPL 交互 10s 路由分类死线(2026-06-25→28 三 commit 引入)被单发 CLI 复用,健康分类时延 6.6–11.4s 与其重叠成掷币;超时静默降级 read pipeline,结构上永不能满足 data-lane 输出合同(route=data 日志/终态 JSON/裸标量答案)。分类器本身 6/6 正确(conf 0.90–0.95)——纯死线噪音,非分类歧义。修复:单发独立 lane(`ClassifyPolicySingleShot`,`single_shot_route_policy_timeout_seconds` 默认 60s,0=禁用外层死线只留 adapter 原生守护;chat 级 guard 传 0 保证下方无第二短钟,双钟间睡眠 pin);降级=一等事件 pin 行 `route-degrade: single-shot classifier timeout…`(eval 可反向断言);REPL 10s 原样保留(交互人体工学,故意不对称,注释成文)。验收:修后二进制 15 run 全 route=data、零 degrade。**过程教训(入档防重踩)**:`parallel_selected.sh` 只快照现成 `./codrax` 不重编译——第一轮复验跑在修复前二进制上整轮无效(json 案 +10.002s 精确超时=旧行为自然复现,反向坐实归因);修码后必须先 `make` 再 sweep,并 `strings` 验 pin 串。

**DL 类:workflow 三发布面互相撒谎死锁(已修,pin 看护)**。标本 basic_sum 3/3 blocked:模型算出正确答案 17,但 (1) 闸门面 stage 表在 `DecisionRecords==0` 时把 `compute_contributions` 滤出 allowed 集;(2) advisory 面发布 decisions ledger 的 ProducesActions **不与 allowed 求交**——建议闸门必拒的动作;(3) 依赖图面 contributions 的 depends_on/missing_prerequisites **不含 decisions**——模型据图正确推出"decisions 非前置",判"唯一生产者被禁且无解锁链"→blocked。模型推理全对,系统状态在撒谎。回归 commit=5ad51bb4(06-09,单 commit 加闸门+改生产者名单,未镜像 advisory 交集与图边)。**06-12"PASS"是幸存者偏差**:当日日志有完全相同拒绝,模型碰运气先走 qualify/filter;且该次 harness 判决本就是 FAIL(oracle 滞后)。修复:共享 typed 谓词 `DecisionsGateExcludesComputeContribs`(闸门/图单一权威);图补 decisions 边镜像;advisory 发布前 typed 交集硬过滤(空交集回退全 allowed,永不发布非法动作)+ Capability 枚举剔散文(violation.go RepairHint 散文曾直通 typed advisory 字段);NextStage 补 decisions 路由 + reconcile-无-contributions 路由(复核抓的同族第二处:三发布面齐推必失败 reconcile);answer-repair 车道(完成形态+可行动 repair_node → allowed 集改发修复面,"要求修复"与"无合法动作"不再共存)。Pin:`stage_reachability_pin_test.go` 2048 态枚举——required ledger 生产者可达性(前置感知 frontier,权威=BuildLedgerGraph.MissingPrerequisites 单源)、advisory⊆allowed(突变形态)、闸门⟺图双向镜像、修复车道非空(243 完成形态计数卫兵防恒真)、basic_sum 卡死 facts 回归复刻。两轮对抗复核 6+4 finding(恒真 pin/同族 reconcile/探针遗留等)全收。oracle 修正:case 滞后断言 `[repl/data]`(eval 走 CLI lane 从首日即打 `[cli/data]`)改 tag 无关 + 短标量 MIN_OUTPUT_CHARS=1(对齐 json 案先例,route=data 与数值正则保留,非降 bar)。

**终验与残余(6/9;三子类立项 DLR,全部弱模型行为面,系统面已诚实)**:basic r1 PASS=解锁链实况打通(qualify/filter→decisions→compute);终态从"带错答案 complete/无解释 blocked"变为 fail-loud 带生产链自解释。残余:**DL-A 修复投影血统污染**(json 050016:终局评估=repair_node、车道开、assemble_answer 跑了 15/18 两轮,但其投影源结构性绑死被污染的 reconcile groups(字段名 active_user_ids),正确工件 emitted_payload{"ids"} 不可作 assemble 源——修复=同污染血统再投影=无操作循环;修向:repair 投影源候选含 answer-bearing 工件/typed 规则声明的输出字段名驱动投影,均精确信号);**DL-B planner 路径选择**(basic 051010:producers=[filter,qualify,compute] 三面一致发布,模型仍耗轮在 mapping_candidate 不产 decisions;软引导 lane);**DL-C 弱模型反复 schema/assemble 失败→诚实空答案 fail-loud**(basic 050649:decisions=38/contributions=38/reconcile=pass/工件 answer="17",result.answer 空,疑与 DL-A 同类=终态投影源;修向并入 DL-A)。data-planner"残余 3 案"账按本节改写:路由类+结构撒谎类已销,DL-A/B/C 为新立精确子类(DLR 项)。

#### 7.12.1 DL-A/DL-C as-built 裁定(DLR 批对抗复核 19 条收账,2026-07-05)

DLR 首版把车道 payload 投影短路在确定性映射之前,对抗复核实证为核心倒置(P1×2+内容盲/短路 Med 一体解)。as-built 优先序(代码权威:`internal/dataquery/assemble_answer_projection.go` `runAssembleAnswer` + `assemble_answer_repair.go` 决策表注释;重排先重开本节):

| 梯级 | typed 条件 | 投影源 | reconcile 改写 |
|---|---|---|---|
| 1 声明映射优先 | `rule_coverage.output_field` 唯一非冲突声明存在,且从**当前** reconcile groups 确定性重算出单键==声明键的 JSON 对象 | 声明映射新鲜重算(血统与内容天然对账) | 随发布(即正常路径语义) |
| 2 payload 回退 | 梯级 1 不可用;准入=custom_payload 信封+未截断合法 JSON+输出合同校验+**合同收窄(仅 json_only 收 JSON payload;plain_single_line/csv_line 一律不收)**+**声明矛盾 guard(含锚 rank0:payload 单键≠声明键即拒)**;排序=**新鲜度优先**(当前轮>较新 seed 轮>较旧 seed 轮)→同鲜度 锚>声明键>格式→同级后产者胜 | answer-bearing payload(canonical 再编码:键序+压缩+HTML 转义,非原文字节) | **与发布耦合**:车道答案真正成为 result.Answer 才发布改写;custom_transform 尾批自带答案时回退 pre-lane reconcile(修"车道改写 vs 脚本答案"必然硬失配) |
| 3 有声明无候选 | 声明存在,梯级 1 不可算且梯级 2 无可入候选 | 受污染血统投影照常发布,但**必带 typed advisory**(`ContractWarnings` 定格式行 + assemble 工件 `declared_output_field_unprojected` 字段),禁静默 | 正常路径语义 |
| 4 无声明无候选 | — | 血统投影(与车道外字节等价) | 正常路径语义 |

配套裁定:seed 合并 same-key **后见胜**且存活项移尾(`dataTaskActionRunnerSeed`,slice 序=生产新鲜度;旧 first-seen-wins 使常量 ID emitted_payload 最旧件永久压制修复批);轮次经 `SeedArtifactRounds`(ID→1-based 轮)+`seedArtifactCount` 边界传运行器,当前轮恒最鲜;`custom_payload`/`emitted_payload` 提常量(`ArtifactKindCustomPayload`/`ArtifactIDEmittedPayload`);"single producer"注释改实话(系统生产者恒定 ID,锚 rank 经系统生产者不可达,仅脚本自命名 payload 工件可触);>1200B Sample 截断=唯一尺寸类准入 blocker,残口如实注明。DL-C:`selectDataTaskTerminalAnswer` 为终端面+评估完成检查的**单一权威**(禁第五处口径;contested 且不早于所选答案记录→终端拒静默发布 fail-loud、完成检查拒以其 satisfied;晚于 contest 的答案=修复产出照常发布)。**Contest 粘滞裁定(二审 P1,2026-07-05)**:contest 视野不是"最新一条评估"——修复批常为无答案 materialization helper(恰是 rung-3 advisory 引导的多批节奏),其例行 continue 评估曾把 contest 洗白成 return_answer/complete 静默发布被质疑答案 X,且让车道在两批式修复的后续 assemble 轮失活裸跑。as-built(`latestDataTaskAnswerContestingEvaluation`;车道激活/完成检查/终端 consult 三面消费同一粘滞谓词,禁两套口径):倒序扫描只认**锚定答案面**(normalized action_kind==assemble_answer)的评估——可行动 repair_node@assemble=开启/续期 contest;答案面锚定且非 repair_node=显式重评估=**唯一**评估类解除事件;答案面锚定的非可行动 repair_node=噪音,两不决;其余评估(helper continue/expand、他节点 repair 锚)一律跳过,不开不清。第二解除事件=晚于 contest 的 answer-bearing 记录(修复产出本身),由消费方按 index 比较持有。Pin:`assemble_answer_repair_test.go`(声明映射压 payload/合同收窄/新鲜度双形态/锚层 guard/无候选 advisory/reconcile 耦合双形态)+`data_task_workflow_dlr_test.go`(粘滞 witness/显式解除/车道激活粘滞/完成检查拒洗白)+`data_task_cli_test.go` contested 三形态(拒发布/修复产出发布/显式解除后发布);突变实证=恢复旧短路、去新鲜度、plain 收 JSON、去 consult、退回"最新一条评估"contest 视野各自必红。

**DLR 行为终验(2026-07-05,三案 ×3,eval/parallel_selected_summary_dlr_reval_{1,2,3}.md)**:7/9(修复前基线 6/9);basic_sum **1/3→3/3**(DL-C+解锁链实证生效)、text 3/3、json 1/3 过+2/3 **诚实 fail-loud**(blocked=模型未走 filter/qualify 生产路径即投降但依赖链消息准确=DL-B 残余软引导域;failed=assemble 参数笨拙产非法 JSON 被输出合同硬闸正确拒绝)。质变:修复前 json 失败形态=发布错字段名答案(撒谎),修复后=零错误答案出厂,全部失败均为诚实拒绝。DL-B 弱模型路径选择维持"只软引导"裁定,不立新账。
