# repo_map P0 + tracequery 类比 + handoff/修复层/性能 — 交付执行计划 (2026-06-12)

执行蓝本：`repo_map_p0_low_risk_reanalysis_20260612.md`（下称"设计文档"，其 file:line
与四里程碑方案为唯一真源，本文不重复）。本文新增三块勘察结论（tracequery 类比裁定、
handoff/修复层/教学缺口、性能基线）并给出分批任务清单。每批独立 commit+push；
B1+B2 后与 B3+B4 后各跑一次 eval 回归（并行 2 案）。

红线总约束（每批自检）：精确信号才能进硬门禁，嘈声信号只作软引导；禁止对用户散文/
模型自由文本做关键字匹配；LLM-facing 文本无内部术语且过 prompt_hygiene + ATOMIC 7
checklist；不拟合单 case；不动稳定路径的字节契约（L1 等结构测试）；新 typed signal
过 R2' 六处同步；eval bar 不降级。

## 新勘察结论摘要

**tracequery 类比裁定（诚实结论：大部分类比不成立）**：tracequery 已深度实现
provenance/confidence（IPCEdge 0.92/0.62/0.45 阶梯、flavor 置信、RootCause Tier，
全部 advisory-only 合规）、3 级 trace-size 行为阶梯（64MB windowed/stream/guard、
128MB recipe discovery）、mtime-keyed 索引缓存 + singleflight + 零拷贝窗口派生、
完整修复层接线（gold standard）。**采纳的真缺口仅 4 项**：
(T1) regex-miss 行无计数器静默消失（仅 panic/clock-regression 有 typed 计数）；
(T2) 大 trace 重型视图守卫是 denylist——`thread_timeline/ipc_graph/wakeup_chain/
interaction_stats` 漏网，512MB trace 无窗口调用 ≈ 5GB 堆且每次重建（OOM 路径）；
(T3) `indexCache` sync.Map 零淘汰，~968B/event，跨 attach 无界累积（进程级唯一
确认的无界增长）；(T4) full-parse 路径无 bench 基线。
**明确不做**：P0.2 top_n 分层（输出随窗口缩放非文件大小，固定 top-N+blob+hint 已够）、
P0.3 staleness banner（不可变工件，size+mtime+parser-version key 使 stale-serve
不可能）、P0.4 route 类比（flavor/platform 检测已是其对应物）、Event 扁平结构与
散落置信字面量不动。

**handoff 缺口（高优）**：(H1) trace_query 算出完整 typed `tracequery.Result` 后
当场丢弃——只有 16-fact 截断的 prose Summary 存活，ObservationLedger 再**逐行
re-parse** 重建 typed 记录，超出 cap/被 blob 预览裁剪的事实永久丢失；
(H2) repo_map 的 task_map/relation_map prose 视图无 typed 载体且被 Raw Tool
Outputs skip-list，导航事实到 extract/finalize 只能靠 LLM 提升或 per-stage 图
re-probe（其 gating 可能压掉 explorer 已见的内容）；(H3) `TurnAArtifacts.ToolResults`
无界且跨 retry 窗口串联（注释声称的 pruneToolHistory 约束是 stale 的——那只 stub
LLM 消息历史）；(H4) prune checkpoint 不带导航上下文，长窗口被剪后模型只能重调
repo_map；(H5) repo_map blob head+tail 预览恰好藏掉 ranked 列表的中段，analyzer
预注入 4096 字节截断可断行。设计文档四类新事实全部走既有 typed 通道
（TypedRelationHint.Provenance / SourceInventoryObservation / ObservationRecord /
RepoMapNavigationPolicy / PreStageDegradations），不建平行状态。

**修复层（五层架构已勘明）**：L0 JSON 语法修复 → L1 schema 驱动 Normalize
（x-codrax-* 注解）→ L2 per-tool strict decode + ToolRepair codes + remap →
L3 同 dispatch retry hint → L4 violation 聚类（RepairPlan/cooccurrence/RepairLocus）。
trace_query 是 gold standard；**repo_map 完全未接入**（lenient decode、未知字段
静默丢、未知 view 静默回退 overview、裸 Go error 外漏、零注解）——P0 把模型推向
repo_map 的同时必须补齐。P0.1-P0.4 自身不新增模型 JSON：P0.2 触一处 schema 文案
（R2' spot-2）、P0.4 的消费枚举四站点已同步。trace_query schema 有 2 处内部术语
泄漏待修。

**教学缺口**：read-lane repo_map 引导已良好（ToolSuggestions 首位、typed first-hop、
grep→relation_map 结果侧 hint）。缺口：call_path 仅 diagram 信号触发、
semantic_subgraph 的 typed route 常量无生产者（死代码）、implementers 无 route、
write lane 完全未教 edit_impact、sub-explorer 拿到的矩阵只有 5/9 视图、trace_query
16 视图教学在 3 处 prompt 手工三联且已漂移（frame_timeline/frame_flow/evidence_pack
零教学）。`[navigation index]` schema 后缀是 citation 契约，**不许动**。

**性能基线**：repomap resolver quadratic 已修（19.6s→78ms，bench 461µs 守护）；
三个 BuildGraph post-pass 已索引化线性。残余采纳项：(P1) scoped projection cache
key 含 query 导致同 scope 每新 query 全量 deep-clone+BuildGraph，且内部 rank 被
clone 清空纯属浪费；(P2) 每次 RankGraph spawn `git log -n 50` 子进程，无 memo；
(P3) build.go:140-149 stale 复杂度注释。**明确不做**：不削弱 ChangedFiles 全量
hash 核账（P0.3 正确性根基）；不再动已线性化的 post-pass。

## 分批任务清单

### B1 = M1 (P0.1) Relation 模型升级 — 设计文档 §4

| # | 任务 | 锚点 |
|---|---|---|
| 1.1 | Relation 五字段 + RelationEndpoint 复用 + Kind 注释刷新 | types.go:188/:75 |
| 1.2 | 30 耐久构建点补 FromEP/ToEP/三诊断字段（Step A，ToEP.Name==To 字节等价前提） | §4.6.2 清单 |
| 1.3 | Confidence 对齐 TierDiscount 单一刻度（1.00/0.85/0.70/0.60/0.30） | parse_fallback.go:50 |
| 1.4 | schema 3→4 + extractorVersions 全 bump + presence 校验三挂点 + legacy 单文件 loader 删除 | cache.go |
| 1.5 | loader typed RejectReason（裸 nil → 带因）+ 多实例不覆写守卫（SchemaVersion>当前 → 内存扫不覆写+WARN） | §3.1 |
| 1.6 | 读者迁移（rank_index/graph/typed_relation/render/scoped_projection/dataflow/explorer/symbol_resolver/ground/emit_evidence/harness） | §4.6.6 |
| 1.7 | Step B 原子删 From/To + CallersOf legacy API + sidecar 改 endpoint 渲染 | §4.2 |
| 1.8 | relation_map 行加 `[kind, provenance/resolved_by, confidence=x.xx]` + call 行解析期标注 + import/implements 常量标注（过 prompt_hygiene） | render.go:894 |
| 1.9 | skill prompt 校准 defaults.go:38/:136（ATOMIC 7 BLOCKING） | §4.6.8 |
| 1.10 | 结构性测试（每语言 fixture 全 relation 过 presence）+ render_test 重 pin + cache round-trip + repomap_v3 harness 迁移与 baseline 重录 | §4.7 |

### B2 = M2 (P0.3) 索引状态 banner — 设计文档 §6

2.1 RepoMapIndexStatus 挂 Graph.Metadata（三态 freshness/RebuildReasons/计数）;
2.2 三类盖戳（buildOrLoad 三助手 post-BuildGraph / facade clone-only / projection 四出口）;
2.3 tool.go 两路径 prepend banner ≤5 行（fresh 零警示；复核行复用既有契约词族）;
2.4 早期 manifest 预检（省 bump 后整仓 hash pass + §10.5 消息前置）;
2.5 FallbackFileCount（BuildGraph Metadata 装配点，先归一 tier 3/4 编号褶皱）;
2.6 /repos refresh 追加 LRU+SearchGraph 失效; 2.7 banner 进 ObservationRecord
（toolResultBanners 既有家族）; 2.8 单测 7 例 + 结构测试（每 serving 分支非空 Source）。

### B3 = M3 (P0.4) route resolver — 设计文档 §7

3.1 route_go.go（gin+chi 共享 walker：import gate / arg0 literal / 变参取尾 /
func_literal 仅 symbol / Group value-binding 前缀 / partial_path）;
3.2 route_python.go（FastAPI decorator + FastAPI()/APIRouter() 赋值跟踪）;
3.3 route_java.go（annotation 子节点迭代 + class @RequestMapping 拼接）;
3.4 Symbol kind=route（Parent 空）+ Relation kind=reference 带 route_resolver/0.70;
3.5 file_map kindOrder + route-aware member key（sourceInventoryCandidateForSymbol）;
3.6 extractorVersions Go 2→3/Py 1→2/Java 1→2; 3.7 oracle pin ×2 + RefCount 潜伏点
pin; 3.8 golden ×6 形态 + java-annotation-router 负向零产出验收。

### B4 = M4 (P0.2) 输出预算 — 设计文档 §5

4.1 size_tier.go（per-view×per-tier 矩阵只降 tiny/small + monotonic 不变量测试）;
4.2 task_map/relation_map 默认值接 tier; 4.3 source_inventory RepoFileCount 字段
（零值=现行为）; 4.4 opt-in 0.35x aux 软降权（RankGraphScores options 变体默认
字节等价 + SourceScopeProfile typed 通道 + nil→OFF）; 4.5 narrowing hint 单站点
（tier>=large，broad_fallback 抑制，top_n>2× 提示行）; 4.6 top_n schema 文案
（R2' spot-2）; 4.7 边界/覆盖/豁免单测。

### B5 = tracequery 真缺口（依勘察裁定，不硬搬）

5.1 (T1) `Index.UnparsedLines` typed 计数器（parseFile+StreamEventSearch 中
ftraceLineRE miss 的非空行）+ summary 一行 parse_quality
（lines/parsed/unparsed/panics/clock_regressions）+ 固定比例阈值 caveat（软提示）;
5.2 (T2) 重型视图守卫 denylist→补全：thread_timeline/ipc_graph/wakeup_chain/
interaction_stats 纳入 traceQueryIsHeavyView（或带 pid/thread 时走既有 auto-window）;
5.3 (T3) indexCache 字节预算 LRU（len(Events)×sizeof(Event) 记账，保留 key 形态与
singleflight）; 5.4 (T4) parseFile 吞吐/alloc bench 基线; 5.5 no-go 清单落档
（不搬 P0.2/P0.3/P0.4；Event 布局与置信字面量不动）。

### B6 = handoff + 修复层 + 教学

6.1 (H1) trace_query typed Result 入 ObservationLedger：四个 store 站点发布
EvidencePack/root-cause typed 行（镜像 PublishSourceInventoryObservationFromLens），
Summary re-parse 降为兜底，ID 级 dedup; 6.2 (H2) relation_map/task_map 渲染时发布
capped typed member 行（≤50，走 SourceInventoryObservation 机制，与 typed_graph
appendix 同 key dedup）; 6.3 (H3) TurnAArtifacts.ToolResults 精确 count/byte 界
+ stale 注释修正（保 InvestigationStructurallyEmpty 门禁地板）; 6.4 (H4) prune
checkpoint 加导航 digest（policy 步骤 + 候选宇宙摘要，stable-hash 防 spam）;
6.5 (H5) repo_map blob 改 head-only 或渲染前按预算裁剪 + analyzer 预注入按条目
边界截断（验 L1 结构测试边界 + prompt snapshot）;
6.6 repo_map 接入修复层全五层（x-codrax 注解 + strict decode + 未知 view 确定性
别名归一 + fact line 替代静默回退 + error 文本消毒）; 6.7 trace_query schema 2 处
内部术语泄漏修复; 6.8 registry-walk 结构测试（工具接入修复层成为不变量）;
6.9 教学：trace_query 视图表 SST 化（单一 Go 表喂 3 站点 + 补 3 个零教学视图）;
6.10 typed policy 扩展：call_path route（ReqCallChain/IntentTrace/AxisCall 既有
typed 信号）、semantic_subgraph route 接 DiagramArchitecture 或删死常量（R7）、
implementers route（enumeration typed 谓词，渲染为 inventory step 内替代项）;
6.11 write lane 教 edit_impact（defaults.go:696/:772 增补 + ToolSuggestions 次序
评估，L6 不动）; 6.12 sub-explorer 复用完整教学矩阵。

### B7 = repomap 性能残余

7.1 (P1) scopedGraphProjectionCacheKey 去 query 分量，缓存未排序投影图，删
projectGraphToRoot 内部 rank（单一排序所有权；补"同 scope 二次 query 不重建"断言；
验 keyword_search scoped 路径 QueryScores 读点）; 7.2 (P2) getRecentlyChanged 按
(root, gitHeadSHA) memo（不出口、不序列化，镜像 receiverNameOnce 模式）;
7.3 (P3) build.go:140-149 stale 注释修正; 7.4 no-go 落档（hash 核账不削弱）。

### 终验

E1: B1+B2 后 eval 回归（repomap 家族 + 写模式抽样，并行 2）; E2: B3+B4 后第二轮
（含 sr_java_annotation_route/sr_java_handler_impls 负向 + mr_* 抽样）;
E3: B5-B7 后 trace_query/perf 家族抽样 + bench 对比; repomap_v3 baseline 重录一次
（B1 后）。FAIL 修系统不修 case。

## 风险与 no-go 汇总

- 全局 no-go：ChangedFiles hash 核账、BuildGraph 已线性化 post-pass、
  `[navigation index]` schema 后缀、Event 扁平结构、tracequery 置信字面量、
  L1/L6 结构契约、`.codrax/worktrees` 布局。
- B1 最大风险：Step A/读者迁移乱序致 dataflow 用户可见 evidence 文本漂移——
  按 §4.2 顺序硬约束执行，逐步 `go test ./...` 验证。
- B6.5 触 analyzer 预注入字节 → 先确认 L1 结构测试边界，prompt snapshot 重录评审。
- 所有 prompt/hint 文案：ATOMIC 7 + prompt_hygiene + InternalTermsBlocklist 三关。

## 完成审计裁定登记（2026-06-12 终审）

- §4.3 解析期标注采用**二值** resolution=typed|name_match，不展开 method_index
  命中步骤：模型只需要 typed-vs-name 的信任分界，步骤名会把 resolver 内部词汇
  推进 LLM-facing 文本（render.go 注释留档）。
- §5.3 include_relationship_summary / include_additional_candidates /
  "additional candidates omitted" 提示推迟 P1（设计文档 §12 风险四的该项
  mitigation 同步顺延）；mock 分类器扩展放弃（全局波及 keyword_search 分层）。
- §6.5 /repos refresh 不需失效 Mutable.SearchGraph：MutableState 每 Run 新建,
  无跨轮存活预热图;LRU 是唯一跨轮内存载体（repos_cmd.go 注释留档）。
- §8 convergence_audit 基线不重跑：P0.2 只改 tiny/small 档默认值,convergence
  audit 跑在 codrax 自仓（medium 档,历史默认值被
  TestDefaultTopNMediumKeepsHistoricalDefaults 钉死字节不变）,tool_repo_map
  计数无漂移来源;若未来调 medium 档则必须重录。
- §8 eval 重跑按用户改令执行抽样制（每批并行 2 案）替代全家族两轮:
  repomap/multirepo/trace/logtri 已覆盖 8 案 PASS;write-mode（patch_*）
  补样于终审完成。
- 终审补齐缺口:CLAUDE.md fallback 格式修正+源码 pin;§14.7 混版本不覆写规则
  落账（修 tool.go 悬空引用）;三态扫描+projection Source 盖戳端到端测试;
  schema 双向 typed reason+不覆写守卫测试;RefCount route 生产者 pin;
  edit_impact 降权豁免不变量测试。
