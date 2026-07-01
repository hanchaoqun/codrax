# 大 trace 分析 gap 记录（2026-07-01，客户 berlin.systrace 1104 MiB 场景）

> 本文档最初只记录一次真实客户 REPL 会话暴露出的 3 个 gap。2026-07-01/02 后,事故级路径已分批修复并在下文逐项标注:PRE-emit runtime-artifact carrier、密窗 state-first 安全降级、thread/pid relation scope、trace grep fallback、trace-first guidance 均已落地。文首保留原始事故背景,避免后续排期误把历史证据当作当前未修项。

## 场景

```
codrax -r "分析这个鸿蒙trace berlin.systrace 其中 42591 进程在
6793222.031397627s 到 6793225.369801793s 期间滑动卡顿的深层次根因，不要分析代码"
[运行时附件] - trace berlin.systrace 1104.0 MiB runtime trace; referenced in request
```

- 仓库:`D:\temp\南海\xiongqing`(Windows,大仓,含超大文件——正是 OOM 的来源)
- trace:`berlin.systrace` **1104 MiB / ~1.7M 行**,按**路径**附加(非 inline text)
- 用户固化了:**pid=42591** + **时间窗 6793222.031397627s..6793225.369801793s** + 明确 **"不要分析代码"**

---

## Gap 1 — 明说"不要分析代码"仍对大仓建完整 repomap 图(触发 OOM + 3× analyze 超时)

**REPL 证据**:
- analyze 阶段连续 3 次 `✗ 未能理解问题 · context deadline exceeded`,之后才成功。
- 第一段 REPL 直接崩在 `1/4 仓库索引 xiongqing 正在校验缓存差异：准备校验 556 个文件` → `fatal error: out of memory`。
- 模型自己的 reasoning 都说了:"按照指令,我不应该运行 repo pre-scan(repo_map、grep、list_files)",且把 `external_observation_policy.current_source_mode` 设为 `exclude`。

**历史根因**(已定位并已被后续 runtime-artifact preflight carrier 修复):模型侧确实避开了 LLM-facing 的预扫**工具**,但当时**系统侧确定性 analyzer IR 构建**仍可能无条件建图:
- `buildAnalysisIR` → `analyzerGraphForNormalize`(`internal/agent/analyzer.go:2766`)→ `repomap.GraphFromAgentContextOrLoad`(2785)→ `BuildOrLoadGraphWithin` —— 正是 OOM 堆栈。
- 这里**有**一个跳过守卫 `analyzerRuntimeArtifactSourceNavigationOptional(ctx, rm)`(analyzer.go:2775),true 时会 `return nil` 跳过建图。但它没触发。
- 守卫 → `RuntimeArtifactRequestSourceNavigationNotRequired`(request_traits.go:1398)→ `HasRuntimeArtifactObservationOnlySurfaceInArtifactContext`,其 `attachedRuntimeArtifact` 参数来自 `analyzerAttachedTraceContext(ctx)`(analyzer.go:3029),只认 **inline** `ctx.AttachedHitrace != ""` 或已构建的 `PerfTrace != nil`。
- 但:①`AttachedHitrace` 存的是 trace **文本**(见 `context.go:5990` 注释),1104 MiB trace 按**路径/blob** 附加时 inline 文本为空;②perf_triage 对 >8 MiB 的 trace **跳过**(`PerfTriageLLMMaxBytes` 默认 8 MiB,`runtime.go:1290`),所以 `PerfTrace` 为 nil。→ `analyzerAttachedTraceContext` **返回 false** → 守卫返回 false → 建图照跑 → OOM(现已修) + 3× analyze `context deadline exceeded`。

**修复进展(2026-07-01)**:

深挖后**修正了上面的根因描述**——经实测(probe)与逐符号复核确认真实盲点:

- 客户用 `codrax -r "...berlin.systrace...不要分析代码"`(**CLI `-r`,无 `--htrace`**),trace 只是**在请求文本里被提及路径**。而 `AttachedHitrace`/`AttachedHitraceSource` 的**唯二写入点**是 CLI `--htrace/--log`(`cmd/root.go:797`)和 REPL `/htrace`(`repl.go:7138`),二者**总是文本+source 一起写**。所以"请求内引用路径"的 trace **两个字段都为空**——故上一版说的"认 `AttachedHitraceSource` 非空"**对这个场景无效**(它从不单独被设)。
- 实测(probe)确认 POST-emit 守卫 `analyzerGraphForNormalize`(analyzer.go:2766)只在 (a) analyzer 把 trace 路径**保留**进 `ExactTargets`/entities/`RequiredFileHints`(→`HasRuntimeArtifactPathReference`=true),或 (b) `attached=true` 时才跳过。客户是 `exclude` + **未保留路径** + `attached=false` → 守卫返回 **false** → 建图 → OOM(OOM 本身已由 `cb896bb4` 修)。
- 但请求**明说"不要分析代码"** → `ExcludesCurrentSource()`=true。这个**精确 typed 信号**本身就足以判定"不需要当前源码符号图",却没被守卫独立消费。

**已修(POST-emit,精确信号)**:`analyzerRuntimeArtifactSourceNavigationOptional`(analyzer.go:3025)新增独立短路——当 `rm.ExternalObservationPolicy.ExcludesCurrentSource()` 为真时直接返回 true,跳过 `analyzerGraphForNormalize` 建图,**不再依赖路径是否被保留 / trace 是否 inline 附加**。用的是与 Gap A / `tier1_floor` 同一精确信号。测试 `TestAnalyzerRuntimeArtifactSourceNavigationOptional_ExplicitExclusion`(exclude 跳过 / 普通请求不跳 / default-mode 不跳)。任何缺锚点的 exclude 已在上游 `promoteInvalidExternalObservationExcludeToAllow` 软化成 allow,故只有"用户显式排除代码"才触发。

**后续收敛(2026-07-01 已交付)**:PRE-emit 路径不再依赖 raw-token 跳过。Run 入口生成 `RuntimeArtifactPreflightProfile`,覆盖 CLI/REPL attached log/trace、请求中显式 trace/log/perf 路径、以及相对 repo 的 runtime artifact 路径,再投影到 `BusContext`/`AgentContext`。`buildAnalyzerRepoOverview`、analyzer tool boundary、Run-entry graph warmup、runtime-artifact-active 判定均消费该 typed carrier。该 carrier 是确定性 path/artifact profile,不是用户关键词或模型散文,因此满足硬门精确信号要求。详见本文 "当前仍需排队的隐含 gap / P0: PRE-emit repo overview typed runtime-artifact carrier —— 已交付(2026-07-01)"。

---

## Gap 2 — 已固化 pid 却把过滤器去掉做"全 trace jank 搜索"(过宽噪音)

**REPL 证据**(探索阶段):
- 第 7 轮 reasoning:"Let me try without the pid filter, and search in the full trace..."
- 第 10 轮:"find jank frames using event_search **without the pid filter** (so it covers the whole trace)";narration 中文:"同时搜索 trace 级别的 jank 事件(**不限定 pid**)"。
- 第 11 轮实际调用:`trace_query view=event_search pattern=jank event_types=trace_mark window=...` —— **去掉了 `pid=42591`**,在 1104 MiB 全 trace 上搜。

**为什么是 gap**:用户固化了 pid+窗口,把 pid 过滤去掉在超大 trace 上做全进程搜索属于**过于宽泛的噪音**(会捞到跨进程无关行)。正确做法是:跨进程的 on-chain 依赖(比如它后来捞到的 `RSUniRenderThre pid=1548` 渲染服务线程)应当**沿着从固化线程出发的 wakeup/binder 链**自然浮现,而不是靠"去掉 pid 全表扫"。

**现状**(已定位):skill prompt **已经**有软引导——`internal/tool/trace_query.go:82` 里写了 "set pid/thread explicitly ... and keep that typed filter on follow-up trace_query calls **unless deliberately inspecting a named peer**",还有 `trace_query_target_inherited`。但模型仍然把 pid 去掉了。所以软引导**不够硬**,且很大程度是 **Gap 3 的症状**:重型视图在密集窗口反复失败后,模型转而"去掉 pid 全表扫 event_search"当逃生口。

**处置(2026-07-01,用户裁定"视为已由 Gap 3 Step 1 解决")**:Gap 2 的**主要触发因素是 Gap 3**(重型视图在密集窗口反复 `IndexEventLimitError` → 模型逃生去 pid 全表扫)。Gap 3 Step 1 已两面消除该触发:①pid-scoped 重型视图现在按字节预算把上限提到 ≈524K,固化 pid 的窗口能真正跑出 root_cause_rank 而非反复失败;②`IndexEventLimitError` 对已 scoped 的请求改成明确 **"pinned pid/thread scope is already applied ... do NOT drop the pinned pid/thread"**(见 Gap 3 Step 1),直接把"去掉 pid"这条逃生路堵上,并引导拆子窗口。剩余的"更硬护栏"属于**语义判断**(用户是否**故意** inspect 某个 named peer 是意图判断,非精确信号),按红线"精确信号才做硬门 / 噪音信号只作软引导"只能保持软引导,不再加硬门。故 Gap 2 **随 Gap 3 Step 1 收口**,不单独再改代码。

**2026-07-02 复核状态**:该排队方向已由后续 typed-target 与大 trace 降级批次承接,不再作为单独 open 代码 gap。`RequestModel.RuntimeTargets` / `emit_analysis.runtime_targets` 已提供 typed-only 目标载体;`trace_query` 在 tool call 省略 `pid/thread` 且只有一个精确 runtime target 时会继承并标记 `trace_query_target_inherited=true`,显式 tool 参数永远优先,多目标或 `AnalyzerHints` 字符串池只给软 caveat。Gap 3 的 scoped heavy-view / index-limit 降级也已把"丢 pid 全表扫"的主要逃生动机降下来。剩余只作为代表性 eval/manual audit 观察项:若未来仍出现去掉固化目标的全 trace 宽搜,应先检查 typed target 是否缺失或多义,而不是从用户原文/模型散文恢复目标。

---

## Gap 3(审视出的最深 gap)— 超大 trace 上重型视图因 250K 事件上限无法运行,模型空转 12 轮无结论

**REPL 证据**:
- 第 4/5/7/8 轮:`frame_root_cause_bundle` / `thread_timeline` / `wakeup_chain` 全部 "The full window is too dense" / "too dense to process at once"。
- 第 10/11 轮:即便窄到 **300ms** 窗口,`root_cause_rank` 仍 `hits index_event_limit at the 250K events threshold`。
- 模型自述:"the trace is very dense and large (1.7M lines), and the trace_query tool has a **250K parsed events limit per view call**. Even the 300ms window is hitting that limit."
- 结果:探索 12 轮,`pid=42591` 的 running(~2677ms)/sleep(~1280ms)状态分布拿到了,但**始终没能跑出 root_cause_rank / wakeup_chain / frame_root_cause_bundle 的深层根因**——被用户 Ctrl-C 打断。

**根因**(已定位):`defaultTraceIndexMaxEvents = 250000`(`internal/tracequery/parse.go:54`),重型视图先把窗口内**全部**事件建成内存索引,超限即 `IndexEventLimitError`(parse.go:167/182,消息"too dense ... narrow with pid/thread")。关键:**pid 过滤发生在索引之后**——`stream_search.go:603` 的 `threadMatches` 是对已解析事件做过滤,索引构建阶段并不按 pid 预裁剪。所以在高密度 trace 上,**即使带 pid+窗口**,索引仍会先塞满 250K 条(全进程事件)再触顶,pid 过滤救不了重型视图。1104 MiB / 1.7M 行的 trace,3.3s 甚至 300ms 窗口的事件数都轻松 >250K。

**修复方向**(排队,最有产品价值):这是真实客户 trace(动辄 GB 级)能否跑通的核心。候选:
1. **索引构建阶段按 pid/thread 预裁剪**:当 query 带 pid/thread 时,parse 索引时就丢弃不匹配、且与目标无 sched 关系的事件(注意:唤醒链需要保留 waker/peer 的相关事件,不能只留目标 pid——要保留"目标 pid 的事件 + 与之有 sched_switch/sched_wakeup/binder 关系的对端事件"),让重型视图能在大 trace 的固化 pid 上跑起来。
2. **两级索引 / 流式重型视图**:像 `stream_state_cluster` 那样让 root_cause_rank/wakeup_chain 走不需要全量内存索引的流式路径。
3. **自动分片 + 聚合**:重型视图在超密窗口自动按时间分片跑、再聚合(工具侧,不让模型手动切 12 次)。
4. 短期:把"密集窗口"的 next-step 从"narrow with pid/thread"(对已 pid 化的请求是误导——见上,pid 救不了)改成"split into sub-windows / add line_start-line_end",避免模型在 pid 上原地打转、进而误去掉 pid(接 Gap 2)。

**注**:Gap 2 与 Gap 3 强耦合——先修 Gap 3(让固化 pid 的重型视图在大 trace 上真能跑),Gap 2 的"去掉 pid 逃生"动机会大幅减弱。

---

## 优先级建议(供排期)

1. **Gap 3**(最高,决定 GB 级客户 trace 能否出根因)—— 索引按 pid+关系预裁剪 / 流式重型视图 / 自动分片。
2. **Gap 1**(高,trace-only 大 trace 的 analyze 稳定性)—— 修 large-path-trace 的附件识别,跳过 repomap 建图。OOM 本身已修,但仍会白建图 + 3× analyze 超时。
3. **Gap 2**(中,依赖 Gap 3)—— 固化 pid 保留护栏 + peer 走 on-chain 链的教学。

---

## Gap 3 修复进展(2026-07-01)

### 设计裁定(6-agent 设计 workflow + 对抗复核,全 confirmed)

- **关系裁剪索引仅对 `thread_timeline` / `wakeup_chain` 可证完备**;对 `root_cause_rank` / `frame_root_cause_bundle` / `window_stats` **不成立**——它们内建 `ComputeWindowStats`(全窗口 × 全线程聚合:CPUPressure 需同 CPU 无关线程 sched_switch、`stats.TraceSpans` 是全 pid 帧、IO/clock/power 全局),裁剪会系统性漏项,是正确性回归。若强裁必须近乎保留全量,250K 上限救不了。
- 复核额外查出朴素 pass-2 保留规则的 3 处会误删事件:同-CPU 竞争者的 `sched_wakeup`(WakeePID=竞争者,不在 relevantTids)、binder 对端 `received` 行(按 TransactionID 配对,`eventMentionsPID` 不看 txid)、depth≥2 的 waker 传递闭包(`expandChain` 只在 StateSSleep 递归到 MaxDepth=10)。裁剪判据必须超集保留:目标 CPU 集全部 sched_switch + 全部 trace_mark + 全部 binder + 递归 MaxDepth+1 层 waker。
- **redlines agent 强烈建议:先做最小正确改法(提上限 + 字节预算 guard),验证是否已够,再上关系裁剪。** 关系裁剪的"关系集"判定是启发式(噪音信号),当硬裁剪门会踩"精确信号才做硬门"红线反面 → 静默不完整、污染 absence 结论。
- cacheKey 若不加 scope 维度,pid-scoped 裁剪索引会串味(静默正确性回归);且 pruned 索引不得入 indexCache;pruned 结果必须打 absence caveat。**注:这些只对 Step 2(裁剪)是硬需求;Step 1(仅提上限,索引仍是全量未裁)天然无串味风险——`cacheKey()` 已含 `max_events`,全量索引对任何查询都正确。**

### Step 1 —— 已交付(zero-regression,byte-budgeted 提上限 + 接线 + 误导提示修复)

`Event` 结构体实测 **1736 字节**(30+ 事件族的扁平 union),250K × 1736B ≈ 434MB 已是工作基线;朴素提到 1M(≈1.7GB)会 OOM——所以**真正的天花板是字节不是条数**,提上限必须走**字节预算**:

- `internal/tracequery/parse.go`:`BuildOptions` 增 `ScopePID/ScopeThread`;`IndexEventLimitError` 增同名字段并在 `Error()` 里**按 scope 定制 next-step**——已固化 pid/thread 的请求不再被告知"narrow with pid/thread"(那只会让模型原地打转、进而丢 pid 全表扫=Gap 2),改为明确"pinned scope 已应用,拆子窗口/加 line 边界,**不要丢掉固化 pid/thread**"。
- `internal/tool/trace_query.go`:新增 `traceQueryScopedIndexMaxBytes`(默认 **1 GiB**,可调)+ `traceIndexEventSizeEstimateBytes=2048`;`traceQueryWindowedIndexOptions`(从内联提出的纯函数,便于单测)对 **pid/thread-scoped 的 heavy view** 把 `MaxEvents` 从字节预算提到 ≈524288(≈2.1× 默认 250K),并记 `ScopePID/ScopeThread`。索引仍是**全量未裁**窗口(不做关系裁剪)→ 所有 view(含 root_cause_rank)都正确、更高上限不会串味(cacheKey 已含 max_events)。非-scoped / 非-heavy 调用完全不变(`MaxEvents=0`→默认上限)→ 既有路径字节等价。
- 效果:客户 300ms 窗口(此前触 250K)现可在单次固化-pid 重型视图里建起来并跑出 root_cause_rank;更密的窗口拿到定制提示后拆子窗口,而非丢 pid。
- 测试:`TestTraceQueryWindowedIndexOptionsScopedRaise`(4 例:pid-heavy 提上限 / thread-heavy 提上限 / unscoped 保默认 / pid-非heavy 保默认)+ `TestBuildIndexWithOptionsScopedLimitErrorGuidesWindowSplit`(真实 build 把 scope 带进 limit error、提示不含 narrow-with-pid、含 do-NOT-drop-pinned)。既有 `TestBuildIndexWithOptionsStopsAtEventLimit`("split the time window")仍绿。

### Step 2 —— 关系裁剪索引(已交付,严格仅限 `thread_timeline`/`wakeup_chain`)

按验证设计 workflow `w9ffnwv29` 实现,让两个因果链视图在超密 GB trace 窗口上真正跑起来(裁剪后事件数远小于全量,即便 1 GiB 预算的全量索引仍触顶时也能出唤醒链)。

- **两遍流式**(`internal/tracequery/parse_relation_scope.go`):**pass-1** `discoverRelationScope` 复用同一 `windowGate` 扫窗口一遍、只累积两个小 map(`tid→tgid`、`wakee→waker` 边),不 append 任何 Event;随后 (a) 把 `ScopePID` 按 TGID 展开成目标进程全部 tid(对齐 `streamStateClusterThreadAllowed` 的 `pid==PID||tgid==PID`),(b) 沿 wakee→waker 边做**有界 BFS**(depth=`ScopeMaxDepth`,默认 11=查询 MaxDepth 10+1 冗余)求**传递 waker 闭包** → `relevantTids`。**pass-2**(主 parse 循环)在 `idx.Events = append` 前插一个 `relScope.keep(&ev)` 谓词:`SchedSwitch` 命中 `PrevPID`/`NextPID`∈relevantTids、`Wakeup/Waking/BlockedReason` 命中 `WakeePID`∈relevantTids、**所有 binder 行无条件保留**(按 TransactionID 配对 send↔received,pid 谓词看不出对端)。谓词与 `newChainQueryCache` 的索引口径逐一对齐,故对这两个视图**完备**。
- **元数据不裁**:`FirstTs/LastTs/ClockRegressions/ParsedKnown/flavor/ScannedLineCount` 全在裁剪前更新,只有 `idx.Events` 被裁 → 裁剪索引对这两个视图的查询结果与全量索引**逐字段一致**。
- **超集安全**:pass-1 收集窗口内**所有**曾唤醒某 relevant 线程的 waker(不止 expandChain 每个睡眠区间实选的那一个)并传递闭包,故 `relevantTids ⊇ 链实际走的线程集`;裁剪只删事件从不加,**永不比 Step 1 更差**(最坏退回 MaxEvents 上限)。
- **cache 隔离**:`cacheKey()` 在 relation-scoped 时追加 `;scope=rel/pid:<n>/depth:<d>`,不同 pid / 非 scoped 落不同槽,pruned 索引永不串给别的查询;非 scoped key 逐字节不变。windowed 恒不入 `indexCache`(`shouldCacheTraceIndex` 返 false),且 relation-scoped **不走** `deriveWindowedIndex`(从全量 cache 派生会拿到未裁窗口)——已在 `BuildIndexWithOptions` 用 `!opts.relationScoped()` 堵死。
- **严格视图门**:`internal/tool/trace_query.go` 仅对 `thread_timeline`/`wakeup_chain` + 显式 pid 置 `RelationScoped=true`;`root_cause_rank`/`frame_root_cause_bundle`/`window_stats`/`critical_blocking_calls` **绝不裁**(它们消费全窗口×全线程聚合:同 CPU 无关竞争 sched_switch、全 pid trace_mark 帧、全局 IO/clock/power,裁剪=静默正确性回归),继续走 Step 1 全量索引。thread-only(无 pid)也不裁(pass-1 需 pid 播种)。
- **测试**:`TestRelationScopedIndexMatchesFullForCausalChains`(**golden 对拍**:wakeup_chain + thread_timeline 全量 vs 裁剪 `reflect.DeepEqual` 相等,裁剪事件数更少、噪音线程被删、binder 全留、元数据一致)、`TestRelationScopedIndexExpandsProcessTGID`(pid=进程展开兄弟线程)、`TestRelationScopedCacheKeyIsolation`(scope key 隔离、非 scoped key 不变)、`TestTraceQueryRelationScopedOnlyForCausalChainViews`(**反向保护**:window-stats 家族绝不置 RelationScoped)。全量 `go test ./...` 绿。
- **未覆盖(可后续)**:thread-only 关系裁剪需 thread→pid 解析;若将来要让 `root_cause_rank` 也在超密窗口跑,需的是**流式分片聚合**(方案 2/3)而非裁剪,属独立设计。

### Step 1/2 稳定性收敛(2026-07-01,回归修复)

用户反馈"最新版本模型不用 trace_query、一直用 grep 分析 trace"。诊断:grep 是 skill 里 trace_query **失败后的既定兜底**,故根因是 trace_query 在真实 trace 上**报错/返回空 → 模型放弃**。两个 Gap 3 改动过激,各修一处:

- **Step 2 关系裁剪改为"懒兜底"**:原实现对**每个** ≥64MiB 的 pid-scoped `thread_timeline`/`wakeup_chain` **都**裁剪,即便全量索引本可放下——真实 trace 的边角情况(合成 fixture 未覆盖)可能让裁剪索引返回空/断裂的唤醒链。改为**先建全量(byte-budgeted)索引,仅当它撞 `IndexEventLimitError` 才回退到关系裁剪**(`traceQueryBuildIndex` 用 `errors.As` 判定)。这正是验证设计"先提上限,不够再裁"的本意。测试 `TestTraceQueryBuildIndexRelationScopeIsLazyFallback`(能放下的查询保留噪音=未裁)。
- **Step 1 字节预算从 1 GiB 降到 512 MiB**:`Event` 结构体 ~1.7KiB,1 GiB→~524K 事件、append 增长期峰值可 >1GiB,在客户受限/大仓机器(有 OOM 前科)会**OOM 崩掉 trace_query → 模型转 grep**。降到 512 MiB(~262K,基本等于已知安全的旧 250K/434MB 上限);真正的密窗头部空间靠 Step 2 懒裁剪兜底,而非无限放大未裁索引。
- Step 1 的**接线(pid/thread 传入 BuildOptions)**与**`IndexEventLimitError` 提示修复(不要丢固化 pid)**无内存影响,保留。

## HEAD 复核摘要(2026-07-01, `origin/main@a6d9fabdd`)

本次同步最新 `main` 后重新对照代码复核,最新 10 个提交已经把本文档的大部分高危项从"记录"推进到"承重":

- **Gap A / trace-only 引用门 livelock**:已由 `explicitCurrentSourceExclusionCompletionBypassLabel` 修复,完成门消费 `ExternalObservationPolicy.ExcludesCurrentSource()` 这个精确 typed 信号,不再要求模型猜 `evidence_floor_waiver` reason。
- **Gap 1 post-emit 建图**:已由 `analyzerRuntimeArtifactSourceNavigationOptional` 修复,`emit_analysis` 之后的 `analyzerGraphForNormalize` 会直接跳过 repomap eager load。
- **Gap 3 Step 1 / Step 2**:pid/thread-scoped heavy view 的字节预算上限与 `thread_timeline`/`wakeup_chain` 的 relation-scoped pruning 已落地,且测试已覆盖 cache key 隔离、因果链 full-vs-pruned golden 对拍、反向保护(root_cause/window_stats 不裁剪)。
- **Gap 2 去掉 pid 全表扫**:主要触发源已随 Gap 3 Step 1 收敛;剩余只能做 soft guidance,不能靠语义判断硬拦。

### 当前仍需排队的隐含 gap

1. **P0: PRE-emit repo overview typed runtime-artifact carrier —— 已交付(2026-07-01)**。
   - 现状:新增 `types.RuntimeArtifactPreflightProfile`,由 `Run()` 入口统一生成,来源包括 CLI/REPL attached log/trace、请求中显式 trace/log/perf 路径、以及相对 `--repo` 的 runtime artifact 路径。该 profile 通过 `BusContext`/`AgentContext`/tool/subagent projection 承重,不从模型散文或用户意图关键词做硬路由。
   - 已落地承重点:
     1. `buildAnalyzerRepoOverview` 先消费 `RuntimeArtifactPreflightProfile.SourceNavigationOptionalForAnalyze()`,runtime artifact turn 直接进入 `emit_analysis` 边界表达,不预发 task_map。
     2. analyzer tool boundary 复用同一 profile,分类期只允许 `emit_analysis`,不允许为了确认 artifact 路径去 `repo_map`/`grep`/`list_files`。
     3. Run-entry graph warmup 复用同一 profile,显式 request path 和 attached artifact 都会 defer warmup。
     4. `RuntimeArtifactContextActiveFromBus/Agent` 纳入该 profile,后续 answer/floor/report 面共享同一 runtime-artifact-active 判断。
   - 测试:
     - `TestAnalyzerBuildInitialInstruction_RuntimeArtifactPreflightSkipsRepoOverview`
     - `TestAnalyzerToolBoundary_RuntimeArtifactPreflightAllowsOnlyEmitAnalysis`
     - `TestRun_RequestRuntimeArtifactPathDefersSingleRepoGraphWarmup`
     - `TestRun_AttachedRuntimeArtifactDefersSingleRepoGraphWarmup`
     - `TestRun_SourceTurnStillWarmsSingleRepoGraph`
     - `TestBuildAgentContext_RuntimeArtifactPreflightMirrored`
     - `TestBusContextProjection_AllTypedSignalsPropagated_*`

2. **P1: 超密窗口的 `root_cause_rank` / `frame_root_cause_bundle` 安全降级 —— 已交付;完整 shard 聚合降为增强项**。
   - 现状:relation-scoped pruning 仍正确限制在 `thread_timeline`/`wakeup_chain`;`root_cause_rank` / `frame_root_cause_bundle` / `window_stats` 继续保持全窗口语义,避免静默丢同 CPU 竞争者、全局 IO/clock/power、全 pid trace_mark。对于 GB 级 trace 的极高密度窗口,工具层已经在 `IndexEventLimitError` 时自动返回可成功消费的 `stream_state_cluster` typed result,而不是失败/让模型盲目缩到 <50ms。
   - 已落地承重点:
     1. `traceQueryIndexLimitResult()` 把 OOM guard 命中转成成功 ToolResult,summary 明确 `mode=index_event_limit`、`state_first_hint`、`parent_window_strategy`、`sub_50ms_local_only`。
     2. 同一路径调用 `tracequery.StreamStateCluster()`,在不 materialize 全量 Event index 的情况下产出 parent-window `WindowStats`、TopRunning/Runnable/Sleep/D/IO、`StateDrilldownPlan` 和 typed observations,并标注 `coverage_mode=state_cluster`。
     3. 普通 `Execute()` 与 bounded/recipe 分支都消费同一 `traceQueryIndexLimitResult()`,因此 `root_cause_rank`、`frame_root_cause_bundle` 等重型视图都会统一降级,不会让模型把 OOM 当格式不支持或随机微窗口充分证据。
   - 测试:
     - `TestTraceQueryIndexLimitResultIsRecoverableScopeHint`
     - `TestTraceQueryIndexLimitResultCoversFrameRootCauseBundle`
     - `TestStreamStateClusterPreservesDominantLongSleepWithoutFullIndex`
     - `TestStreamStateClusterPreservesParentWindowStatePriorities`
   - 后续增强(非当前阻断):完整 `TraceShardAggregator` 仍可排队,用于把多个 80-150ms shard 的 root_cause_rank/window_stats 近似合并为 parent-window 排名;合并字段、不可合并 caveat、golden 等按原任务拆解保留,但当前事故级 OOM/微窗口循环已有 typed state-first 安全降级兜底。
   - **2026-07-02 本批交付:TraceShardAggregator soft handoff slice**:
     1. 已在既有 `TraceObservationCoverage` 上新增 bounded shard aggregate view,不新建并行 trace ledger。输入仅为 deterministic `trace_query` 的 typed `ObservationRecord` / `TraceObservationCoverageRecord`。
     2. 已聚合同一 root-cause 候选跨多个 bounded shard 的累计影响、最大单 shard 影响、覆盖窗口、代表 shard 窗口、on-chain/adjacent/background 分层,并限制输出 Top-N。
     3. Stage report 与 finalizer Observation Ledger 已渲染该 aggregate view,并明确它是 parent-window soft handoff,不是完成硬门,不可因为 aggregate 缺失而重开探索。
     4. 无法安全合并的场景 fail open:没有可解析窗口、只有单 shard、存在 parent-window root row 时不强行合并;继续显示原始 typed observations / caveat。
     5. 测试覆盖:多 80-150ms shard 聚合、0 秒起点窗口、parent-window row 抑制重复聚合、stage report/finalizer handoff 可见、无 completion hard-block 字样。禁止从用户原文、模型 prose、工具 Summary、本地化 UI 或 eval label 推断 shard 归属。
   - **2026-07-02 后续批次:TraceShardAggregator window-stats/state-first soft handoff 补齐(P1 / handoff completeness / planned)**:
     1. 新复核发现上面的 soft handoff slice 只聚合 `root_cause_rank` rows,但本节原始目标是"多个 80-150ms shard 的 `root_cause_rank/window_stats` 近似合并为 parent-window 排名"。这不是新的 hard gate,但会让 dense trace 的 state-first 证据仍主要散落在多个 shard 的 `state_drilldown` / `state_churn` rows 中,finalizer 需要从多条 typed observation 里自行拼接"父窗口里反复出现的主状态"。
     2. 泛化修复方向:仍复用 `TraceObservationCoverage`,不新建并行 trace ledger;新增 bounded shard state aggregate view,只消费 deterministic `trace_query` 产生的 typed `state_drilldown` / `state_churn` / thread-timeline/resource-pressure coverage rows,按 `(dimension, subject, object/state, drilldown_source, chain/recommended flags)` 聚合 shard count、union window、total/max impact、significant shard count、推荐 view、support refs。`root_cause_rank` shard aggregate 继续保留并排序在主链维度前。
     3. Safety:该 view 只作为 stage report / finalizer Observation Ledger 的 soft parent-window handoff,不允许成为 `emit_investigation_complete`、证据校验或成文阶段 hard blocker;没有可解析窗口、只有单 shard、已有 parent-window state/root row、或 typed fields 不足时 fail open。禁止从用户原文、模型 prose、工具 Summary、本地化 UI、elapsed time 或 eval label 推断 shard 归属。
     4. 任务拆解:
        - Batch A:在 `types.TraceObservationCoverage` 上新增 state shard aggregate typed projection + 单元测试,覆盖 long sleep、fragmented sleep non-recursive、runnable/IO recursive、significant=false 保留但低优先级。
        - Batch B:Stage report 与 finalizer Observation Ledger 渲染该 projection,文案明确 soft handoff / not completion blocker,并用短行避免增加 prompt 噪音。
        - Batch C:更新本文档 as-built,跑 focused `internal/types` + `internal/agent` trace coverage tests;全量回归后再进入下一批 eval。

3. **P2: thread-only relation-scoped pruning —— 已交付(2026-07-01)**。
   - 现状:Step 2 已支持只传 `thread` 的 `thread_timeline`/`wakeup_chain` 进入 lazy relation-scope fallback。工具层只传 typed `thread` 参数;真正的裁剪权威在 `tracequery` parser 内部,只消费 window gate 内结构化 `comm/pid/tgid/prev/next/wakee` 事件,不从 raw objective 或模型散文推断。
   - 已落地承重点:
     1. `BuildOptions.relationScoped()` 允许 `ScopePID>0 || ScopeThread!=""`;pid 仍直接承重。
     2. `discoverRelationScope()` 复用 `thread_selector.go` 的 selector 解析/匹配,thread-only 只有解析到单一 pid/tgid universe 才生成 pruning scope;同名多 TGID / 无候选时不裁剪,只写 `relation_scope_thread_ambiguous` / `relation_scope_thread_unresolved` caveat。
     3. relation-scope cache key 增加 `thread:<normalized-selector>` 维度,不同 thread selector 与 pid scope 不共用 pruned index。
     4. `traceQueryWindowedIndexOptions` 仅对 `thread_timeline`/`wakeup_chain` 开启 thread-only relation fallback;`root_cause_rank` / `frame_root_cause_bundle` / `window_stats` 等 whole-window 聚合视图继续不裁剪,避免静默丢背景资源/CPU/IO 竞争证据。
   - 测试:
     - `TestRelationScopedIndexResolvesUniqueThreadSelector`
     - `TestRelationScopedIndexAmbiguousThreadSelectorDoesNotPrune`
     - `TestRelationScopedCacheKeyIsolation`
     - `TestTraceQueryRelationScopedOnlyForCausalChainViews`
     - `TestTraceQueryWindowedIndexOptionsScopedRaise`

4. **P0: trace artifact grep/exec fallback 仍把模型拉回文本搜索 —— 已交付(2026-07-01)**。
   - 最新客户 `berlin.systrace` REPL 再次暴露同类事故:模型一开始没有调用 `trace_query`,而是连续使用 `grep berlin.systrace` 和 `exec_command grep|awk` 分析 1.1GiB trace;后续这些裸文本观察又无法稳定落成 typed runtime observation,模型开始为了 citation/support_refs 去读 trace 行号、重试 `emit_evidence`/`emit_investigation_complete`,最终形成"想结束调查但被 completion 债务推回探索"的循环。
   - 最新代码复核:完成门一侧已由 `explicitCurrentSourceExclusionCompletionBypassLabel` 消费 `ExternalObservationPolicy.ExcludesCurrentSource()` 兜住"不分析代码"请求,不再强要当前源码 citation。但工具层仍有反向牵引:
     1. `grepZeroMatchRefinement` 对 runtime artifact zero-match 仍 `preferred_next_tool=grep`;
     2. `execCommandSearchShapeAdvisory` 对 trace grep/awk 仍建议 `grep -n`/awk 补行号;
     3. broad runtime grep summary 仍提示继续 narrow grep/read_file。
   - 修法:不硬禁 grep(保持透明、可审计 fallback),但对 trace/systrace/htrace/perf 形态的 grep/awk 结果统一发强软提示 `trace_query_required_soft_advisory`,并把 typed `ToolRefinementHint.PreferredNextTool` 改为 `trace_query`(`view=event_search`,携带 path/pattern/line window)。普通 `.log` 不触发,避免误伤日志分析。该策略只消费路径后缀、可读文件头部 tracepoint 签名、工具 schema 参数和 typed refinement,不解析用户意图关键词或模型散文。
   - 承重点:
     1. `exec_command` 的 trace grep/awk 成功、zero-match、宽 OR/alternation 均提示"不要继续用 grep 修 trace 分析,切回 `trace_query`";
     2. `grep` 的 trace zero-match、普通成功、broad/streamed compaction 均渲染强提示并偏好 `trace_query`;
     3. 对普通 log runtime artifact 保留原有 grep/read_file 恢复路径。
   - 测试:
     - `TestExecCommand_SearchShapeAdvisory` 中 runtime trace grep/ftrace/zero-match/broad-OR/awk/suffixless trace content 均断言 `trace_query_required_soft_advisory`,且不再出现 `grep -n`/`read_file around` 这类把 trace 分析拉回文本行号的建议。
     - `TestGrepTool/runtime artifact no match teaches literal and line-window recovery` 保留 `.log` 恢复路径。
     - `TestGrepTool/trace artifact no match pulls follow-up to trace_query` 钉住 trace zero-match refinement。
     - `TestGrepTool/broad runtime artifact grep refinement stays artifact-local` 钉住 trace broad refinement 不走 repo_map、不走 read_file/grep_line window,而是 `trace_query`.
   - **2026-07-02 residual gap(本批已修)**:复核当前 HEAD 发现该已交付项仍有两个漏网出口,会把 trace 分析重新拉回文本行号:
     1. `compactBroadGrepOutput` / `compactStreamedRuntimeArtifactGrepOutput` 对 trace artifact 虽然渲染 `trace_query_required_soft_advisory`,但随后仍追加通用 `line_window_hint=... next use read_file ...`。
     2. skipped-large artifact 恢复提示仍统一建议 `single-file grep -> read_file around returned line numbers`,没有把 trace/systrace/htrace/perf 分流到 `trace_query`。
     已实现:trace artifact 的 broad/streamed/skipped-large follow-up 统一收敛为 `trace_query(view=event_search/span_window/window_stats/root_cause_rank/frame_root_cause_bundle)` 软提示和 typed refinement;普通 log/runtime text 继续保留 grep/read_file 行证据恢复。`grep`/`exec_command` schema 文案也拆成 trace-query-first 与 log-line-evidence 两个心智面,不再把 log/trace 混写成同一条 `grep -n`/`read_file` 路径。硬逻辑只消费工具参数、路径后缀/文件头 deterministic classifier、typed runtime artifact carrier,不读用户意图关键词、模型 prose、工具 summary 或 localized UI。
     测试:focused `internal/tool` 覆盖 trace broad grep 无 line-window/read_file、streamed trace grep 无 line-window/read_file、skipped-large trace refinement 指向 `trace_query`、skipped-large log 仍保留 `grep` 恢复、prompt hygiene pin trace/log 分流文案。

5. **P0: `RUNTIME TRACE FIRST` prompt guidance 对 request-path 大 trace 不渲染 —— 已交付(2026-07-01)**。
   - 客户补充日志显示 explorer 第一轮仍把 `record_trace_*.ftrace` 当普通文件读/grep,并在 completion 阶段把 runtime artifact 行号当当前源码 citation 债务修。独立审计定位到上游原因:`explore-skill` 的 trace-first 指导已迁到 TierB,由 `AppliesToFilter.RequiresTrace` 控制;而 `buildAppliesToContext().HasTrace` 只读 `PerfTrace`、`AttachedHitrace/AttachedHitraceSource`、`RequestModel.PerfTrace`。对"请求文本点名 1104MiB trace 路径,但没有 `--htrace`,且 perf triage 因 >8MiB 不 materialize"的真实客户形态,这些字段全为空,于是 trace-first workflow 完全不渲染。
   - 修法:让 `HasTrace` 消费已有 typed carrier,不新造散文判断:
     1. `RuntimeArtifactPreflightProfile.Artifacts` 中 `kind=trace` 或 `source` 为 trace/systrace/htrace/perf/ftrace 路径;
     2. analyzer 后的 `RequestModel.RuntimeArtifactPathReferenceKind()=="trace"`,该 helper 只读 analyzer 结构化 hints/quotes 和 path-kind classifier。
   - 反向保护:普通 `.log` preflight 不设置 `HasTrace`,仍不会渲染 trace-only workflow,避免把日志分析误导到 trace_query。
   - 测试:
     - `TestSkillTierAwareWorkflow_TraceGatedByTypedArtifact` 新增 runtime preflight trace path、analysis referenced trace path、log-only negative。
     - `TestBuildPromptContext_ExploreSkillRendersTraceWorkflowForRuntimePreflightTrace`。
     - `TestBuildPromptContext_ExploreSkillRendersTraceWorkflowForRequestModelTracePath`。
### Step 1/2 稳定性收敛(2026-07-01,回归修复)

用户反馈"最新版本模型不用 trace_query、一直用 grep 分析 trace"。诊断:grep 是 skill 里 trace_query **失败后的既定兜底**,故根因是 trace_query 在真实 trace 上**报错/返回空 → 模型放弃**。两个 Gap 3 改动过激,各修一处:

- **Step 2 关系裁剪改为"懒兜底"**:原实现对**每个** ≥64MiB 的 pid-scoped `thread_timeline`/`wakeup_chain` **都**裁剪,即便全量索引本可放下——真实 trace 的边角情况(合成 fixture 未覆盖)可能让裁剪索引返回空/断裂的唤醒链。改为**先建全量(byte-budgeted)索引,仅当它撞 `IndexEventLimitError` 才回退到关系裁剪**(`traceQueryBuildIndex` 用 `errors.As` 判定)。这正是验证设计"先提上限,不够再裁"的本意。测试 `TestTraceQueryBuildIndexRelationScopeIsLazyFallback`(能放下的查询保留噪音=未裁)。
- **Step 1 字节预算从 1 GiB 降到 512 MiB**:`Event` 结构体 ~1.7KiB,1 GiB→~524K 事件、append 增长期峰值可 >1GiB,在客户受限/大仓机器(有 OOM 前科)会**OOM 崩掉 trace_query → 模型转 grep**。降到 512 MiB(~262K,基本等于已知安全的旧 250K/434MB 上限);真正的密窗头部空间靠 Step 2 懒裁剪兜底,而非无限放大未裁索引。
- Step 1 的**接线(pid/thread 传入 BuildOptions)**与**`IndexEventLimitError` 提示修复(不要丢固化 pid)**无内存影响,保留。
