# 大 trace 分析 gap 记录（2026-07-01，客户 berlin.systrace 1104 MiB 场景）

> 本文档只**记录**从一次真实客户 REPL 会话里暴露出的 3 个 gap,**不在本轮修复**(用户指示"记录下来,后面修复 / 排队修复")。已修复的 OOM 崩溃(`cb896bb4`)是同一场景触发的独立问题,见该 commit;本文档记录的是 OOM 之外的架构/行为 gap。每个 gap 都给了 REPL 证据 + 精确代码指针 + 修复方向,供后续单独立项。

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

**根因**(已定位到代码,未修):模型侧确实避开了 LLM-facing 的预扫**工具**,但**系统侧确定性 analyzer IR 构建**仍无条件建图:
- `buildAnalysisIR` → `analyzerGraphForNormalize`(`internal/agent/analyzer.go:2766`)→ `repomap.GraphFromAgentContextOrLoad`(2785)→ `BuildOrLoadGraphWithin` —— 正是 OOM 堆栈。
- 这里**有**一个跳过守卫 `analyzerRuntimeArtifactSourceNavigationOptional(ctx, rm)`(analyzer.go:2775),true 时会 `return nil` 跳过建图。但它没触发。
- 守卫 → `RuntimeArtifactRequestSourceNavigationNotRequired`(request_traits.go:1398)→ `HasRuntimeArtifactObservationOnlySurfaceInArtifactContext`,其 `attachedRuntimeArtifact` 参数来自 `analyzerAttachedTraceContext(ctx)`(analyzer.go:3029),只认 **inline** `ctx.AttachedHitrace != ""` 或已构建的 `PerfTrace != nil`。
- 但:①`AttachedHitrace` 存的是 trace **文本**(见 `context.go:5990` 注释),1104 MiB trace 按**路径/blob** 附加时 inline 文本为空;②perf_triage 对 >8 MiB 的 trace **跳过**(`PerfTriageLLMMaxBytes` 默认 8 MiB,`runtime.go:1290`),所以 `PerfTrace` 为 nil。→ `analyzerAttachedTraceContext` **返回 false** → 守卫返回 false → 建图照跑 → OOM(现已修) + 3× analyze `context deadline exceeded`。

**修复方向**(排队):`analyzerAttachedTraceContext` / 守卫应把**按路径附加的大 trace**也识别为"已附运行时 artifact"(例如认 `AttachedHitraceSource` 非空、或引入/消费一个 `AttachedHitracePath` 字段、或从 BusContext 的 trace 附件标志判断),使 trace-only + `current_source_mode=exclude` 的请求**无论 trace 大小**都跳过 `analyzerGraphForNormalize` 建图。这样对 trace-only 请求既省内存、又省掉 analyze 的建图耗时(消除 3× 超时)。修复前需真机核实 large-path-trace 下 `ctx.AttachedHitrace`/`AttachedHitraceSource` 的实际取值。

---

## Gap 2 — 已固化 pid 却把过滤器去掉做"全 trace jank 搜索"(过宽噪音)

**REPL 证据**(探索阶段):
- 第 7 轮 reasoning:"Let me try without the pid filter, and search in the full trace..."
- 第 10 轮:"find jank frames using event_search **without the pid filter** (so it covers the whole trace)";narration 中文:"同时搜索 trace 级别的 jank 事件(**不限定 pid**)"。
- 第 11 轮实际调用:`trace_query view=event_search pattern=jank event_types=trace_mark window=...` —— **去掉了 `pid=42591`**,在 1104 MiB 全 trace 上搜。

**为什么是 gap**:用户固化了 pid+窗口,把 pid 过滤去掉在超大 trace 上做全进程搜索属于**过于宽泛的噪音**(会捞到跨进程无关行)。正确做法是:跨进程的 on-chain 依赖(比如它后来捞到的 `RSUniRenderThre pid=1548` 渲染服务线程)应当**沿着从固化线程出发的 wakeup/binder 链**自然浮现,而不是靠"去掉 pid 全表扫"。

**现状**(已定位):skill prompt **已经**有软引导——`internal/tool/trace_query.go:82` 里写了 "set pid/thread explicitly ... and keep that typed filter on follow-up trace_query calls **unless deliberately inspecting a named peer**",还有 `trace_query_target_inherited`。但模型仍然把 pid 去掉了。所以软引导**不够硬**,且很大程度是 **Gap 3 的症状**:重型视图在密集窗口反复失败后,模型转而"去掉 pid 全表扫 event_search"当逃生口。

**修复方向**(排队):①把"保留固化 pid/thread"从软引导升级为更强的 typed 提示/护栏(例如当请求带唯一 `runtime_targets` 时,heavy 视图去掉 pid 需要显式理由,或工具侧对"固化 pid 被丢弃 + 全 trace 扫"给一条 caveat/降级);②教学上明确:找跨进程 peer 用 `wakeup_chain`/`critical_blocking_calls`/`ipc_graph` 从固化线程出发,而不是去 pid 做全表 `event_search`。注意别踩红线(精确信号才做硬门)——pid 是否被显式设置是精确 typed 信号,可以做较硬的引导;但"是否该 inspect peer"是语义判断,只能软引导。

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
