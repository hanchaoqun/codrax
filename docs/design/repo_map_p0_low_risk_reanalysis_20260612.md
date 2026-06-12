# repo_map P0 低风险改进 — 需求再分析与详细方案 (2026-06-12)

对 `/Users/han/opt/codrax_repo_map低风险收益快改进需求文档.md`（下称"原始文档"）的逐条核对与修订。
核对方式：9 路并行代码勘察（Relation 模型 / cache / 视图渲染 / 检索排序 / extractor /
multigraph / 外部消费者 / symbol-kind 链路）+ CodeGraph 开源项目源码调研（clone 实测）
+ 4 路对抗性可行性核查 + 1 路完备性批判。所有 file:line 引用基于当前 HEAD（3e88b586）。

四个 P0 需求的总体裁定：**方向全部成立，细节全部需要修正**（4/4 needs_adjustment）。
原始文档对代码现状的多数假设正确，但有 6 处与代码事实冲突、3 处违反项目红线、
1 处系统边界盲区（多实例并发）必须在方案中先行解决。

---

## 0. 结论摘要

| 需求 | 裁定 | 关键修正 |
|---|---|---|
| P0.1 Relation 升级 | 成立，需调整 | `RelationEndpoint` 已存在且与提案逐字段一致 — 复用不重造；`ResolvedBy` 必须拆成"构建期(持久化)/解析期(渲染时)"两层；import 边没有 Relation 载体 — 渲染时常量标注；§6.10 的 semantic_subgraph 展示结构上不可能 — 降范围到 relation_map；From/To 两步删除是强制项不是备选项 |
| P0.2 输出预算 | 成立，需调整 | 统一 4/5/8 的 top_n 会废掉 relation_map（一个 topN 喂三个 section）和 source_inventory 计数语义 — 改 per-view × per-tier 矩阵且只降 tiny/small；"必要时硬排除"违反精确信号红线 — 改 0.35x 软降权；test 意图检测走已有 `SourceScopeProfile` typed 通道，禁止关键词扫描 |
| P0.3 索引状态 | 成立，需调整 | 磁盘加载路径每次调用都做 hash 比对核账，"hash mismatch → stale" 状态不可达；HEAD mismatch 是嘈声信号只能做 fact line；真正的 staleness 窗口是**内存复用**（LRU 常驻 / SearchGraph 复用 / scoped projection）；`RebuildRequired` bool 无持久态可言 — 改 `RebuildReasons []string` |
| P0.4 route resolver | 成立，需调整 | 硬依赖 P0.1 先落地；消费侧已全线预埋（schema enum / role mapper / source_inventory 通路 / 合成 symbol 端到端测试）只缺生产者；首批改 gin+chi / FastAPI / Java Spring，Kotlin Spring 和 Express 明确推迟（各有未计入的真实工作量）；route 名进入 SymbolOracle 硬门禁词汇表需显式签字 |
| 横切（新增） | **blocking** | 破坏式 schema bump 没有多实例并发故事：repomap cache 零文件锁、不在 architecture.md §14.7 多实例安全清单内、`cacheManifestVersionValid` 双向严格相等 — 新旧二进制混跑窗口会进入无限 full-rescan ping-pong，且 eval/parallel_all.sh 让 23 个 fixture cache 的并发重建成为升级后第一次扫批的必然姿态 |

里程碑排序修订：原文档 P0.1→P0.2→P0.3→P0.4 改为 **P0.1→P0.3→P0.4→P0.2**
（P0.3 与 P0.1 共享 loader 改造，相邻落地避免二次翻挖；P0.2 完全独立放最后；
eval 全家族重跑按两批合并：M1+M2 后一次、M3+M4 后一次）。

---

## 1. CodeGraph 对照分析（修正原始文档 §1 背景）

调研对象：https://github.com/colbymchenry/codegraph，源码已 clone 实测（v0.9.9）。

### 1.1 事实修正

原始文档把 CodeGraph 当作一个普通参照物，实际它是 ~44.5k star、极活跃的 TypeScript
MCP server（SQLite + FTS5 + tree-sitter WASM，2026-01 创建，README 自报 ~16% token
节省 / ~58% 工具调用减少）。**原始文档 §7.2/§7.3 的预算数字就是 CodeGraph 源码的直接
移植**：`getExploreOutputBudget(fileCount)` 的分层切点 150/500/5000/15000 与 default
files 4/5/8/8 逐数字一致（mcp/tools.ts:159）。这说明文档作者参照了其实现 — 但只
移植了 top_n 维度，丢掉了 CodeGraph 预算机制里真正的设计内核（见 1.3）。

原始文档声称的 codrax 三项优势核验：

| 声称 | 核验 | 证据 |
|---|---|---|
| multi-repo 治理更强 | ✅ 成立 | CodeGraph 一个 daemon 绑一个 project root（daemon.ts:4），只有 intra-root monorepo workspace 解析，无跨仓图/注册表/策略层 |
| typed graph（SymbolID/MethodIndex） | ✅ 成立，且 CodeGraph 反向佐证 | 其 node id = sha256(filePath:kind:name:**line**) — 行号敏感，编辑挪行即 id 全churn（tree-sitter-helpers.ts:18），是 README 自己都该列的反模式 |
| 大仓扫描韧性 / chunk cache | ⚠️ 高估 | CodeGraph 有 per-file parse 超时+worker 重启、每 250 文件 worker 回收、分批 resolve-persist、content-hash 增量、**跨进程文件锁**（index.ts:430）。codrax 独有的是 chunked baseline-cache 工件本身，"韧性"不是独占项 — 且 codrax 反而**没有**跨进程锁（见 §8.1） |

### 1.2 与本次 P0 直接相关的 CodeGraph 机制

- **Edge provenance**：persisted enum `'tree-sitter'|'scip'|'heuristic'`，heuristic 边
  必配 `metadata.synthesizedBy` 渠道名 — 与 P0.1 的 Provenance/ResolvedBy 同构，验证
  了设计方向。
- **数值 confidence 算完即弃**（ResolvedRef.confidence 0.85–0.95 用于选赢家后丢弃，
  查询期只剩二值 high/low）— CodeGraph 自己的反面教材，README 调研 agent 也标注
  "avoid"。P0.1 选择持久化 confidence 是对的，但要吸取它的教训：**值必须有单一来源**
  （对齐既有 TierDiscount 常量，见 §4.4），否则两套数值词汇漂移后同样等于没有。
- **输出预算的真正内核**是字符上限（13K/18K/24K 对齐宿主 ~25K inline 工具结果上限，
  超限即外置文件反而逼 agent 多一次 read）+ **调用次数预算引导文本**（"Explore
  budget: N calls… spend remaining calls BEFORE falling back to Read"）。codrax 已有
  blob 外置机制（StoreBlob head+tail 预览），P0.2 不需要字符上限，但 narrowing hint
  的措辞应学它的"引导多次窄查询"而非单纯"少给"。
- **Staleness banner**：fs.watch + 2s debounce，per-file pending 标记，**偏向误报**
  （宁可多警一次 read 也不误导 agent）— P0.3 的 banner 哲学与之一致；watcher 本身
  P0 明确不做（原始文档 §4 已排除，维持）。
- **tiny-repo 工具门控有 A/B 数据**：<500 文件只暴露 3 个工具；其注释记录了一次
  砍过头的回归（express 从 -43% WIN 翻成 +107% LOSS）— 佐证 P0.2 "tiny/small 只软
  降权不硬排除"的保守取向。
- **monotonic tier invariant**（更大的 tier 不得有更小的 per-file 预算）— P0.2 的
  per-view × per-tier 矩阵应继承此不变量并写成单测。

### 1.3 结论

CodeGraph 验证了四个 P0 的方向（provenance 边、size 预算、staleness banner、route
一等公民 + 22 个框架 resolver），同时提供了三个"避坑"：行号敏感 id、confidence 算完
即弃、预算只砍 top_n 不管输出形态。codrax 的差异化（typed SymbolID、multi-repo、
chunked cache）在 P0 设计里全部保留不动。

---

## 2. 原始文档假设 vs 代码现状 — 核对表

| # | 原始文档假设 | verdict | 代码事实 |
|---|---|---|---|
| 1 | Relation 有 legacy From/To + 较新 ToEP | ✅ | types/types.go:188-200；From/To 注释 "deleted in P1.4"（repomap v3 重构计划遗留未执行步骤） |
| 2 | 需新增 RelationEndpoint | ❌ 已存在 | types/types.go:179，与提案逐字段一致（ID/Name/Receiver/File/Line）— 复用 |
| 3 | FromEP 不存在 | ✅ | 全仓 0 命中；ToEP 也只有 6/30 构建点填（Go call×2、Kotlin/Cangjie inheritance×4） |
| 4 | Confidence 目前在 metadata 里 | ❌ | Relation 无任何 confidence/metadata 字段；唯二置信信号是文件级 ParseTier(1-4) 和查询期 TypedRelationPrecision enum，都不在边上 |
| 5 | relation kinds 含 import/reference | ⚠️ 部分 | extractor 实际只产 call/inheritance/embedding/type_usage；**import 边不是 Relation**（是 ImportGraph map，build.go resolveImportGraph）；"reference" 有 4 个消费点但生产者为零（仅测试 fixture） |
| 6 | cache 存 hash/schema/extractor/head/time/checksum | ✅ | manifest 全有（cache.go:177-240），但 RepoHead/WrittenAt 注释 "purely diagnostic"，从不参与比对 |
| 7 | schema mismatch 需改为直接重扫 | ❌ 已是 | cacheManifestVersionValid 不匹配 → loader 返回裸 nil → fullScan（cache.go:587-597）；本就无 migration 代码；缺的只是**原因可见性**（nil 与损坏不可区分） |
| 8 | 旧 relation payload 会被拒载 | ❌ 反了 | 全部 json.Unmarshal 无 DisallowUnknownFields，缺字段静默零值加载 — schema bump 是唯一防线，per-record 校验是新增项 |
| 9 | source_inventory 需要 route role | ❌ 已预埋 | schema enum 已含 "route"（tool.go:122,130）；kind→role mapper 已映射 route/handler_route（emit_investigation_complete_aggregate_reconcile.go:679）；tool_scope_test.go:618-668 已用合成 route symbol 端到端验证消费通路 — **只缺生产者** |
| 10 | top_n 需接入三视图 | ✅ 参数已在 | tool.go:46 已是用户参数流到所有视图；默认值是 per-view 常量（overview 15 / task_map 20 / relation_map 40 / file_map 50 / subgraph 8 / source_inventory 60/24），无任何 size 分层 |
| 11 | repo HEAD mismatch 可判 stale | ❌ 嘈声信号 | manifest RepoHead 只在写 cache 时刷新；cache_hit 时内容 hash 全等而 HEAD 可以合法不同 — 做 stale 硬判定恰好触发原始文档 §12 风险五自己警告的"狼来了" |
| 12 | file hash mismatch 可判 stale | ❌ 状态不可达 | 磁盘加载路径每次调用都跑 ChangedFilesWithProgress 内容 hash 比对（tool.go:799-821），mismatch 当场转 incremental/full — 被服务的磁盘图永远是核账过的 |
| 13 | semantic_subgraph 可展示边 provenance | ❌ 结构不可能 | 该视图渲染 ComputeChains/Hubs/Bridges，输入只有 ImportGraph/ReverseImports 拓扑（subgraph.go:69-316），全程无 Relation 行 |
| 14 | Go/Python/Java 解析形态支持 route 提取 | ✅ | 13/15 语言 tree-sitter（例外：ArkTS 骑 TS 文法+regex 后处理、Cangjie 手写 parser）— route 提取是 AST walk 不是行模式匹配 |
| 15 | ResolveCallTarget 可写 ResolvedBy=go_method_index | ❌ 时序不可行 | MethodIndex 在 BuildGraph 后才存在，而 fullScan 在 parse **过程中**经 cacheSink 流式写 cache（tool.go:1030-1037）；incrementalScan 又是 build 后存 — 回写会造成两条扫描路径 cache 内容分叉。改为两层拆分（§4.3） |

另有两处文档/代码漂移随本设计一并修复：CLAUDE.md 引用的 fallback WARN 格式与实际
`recordFallback` 输出不符（实际 `repomap: <lang> <file> tier N→M: <reason>`，
parse_fallback.go:114-122，无测试钉死）；docs/architecture.md:2370 "零 staleness 窗口"
的表述与 P0.3 的 freshness 模型矛盾，banner 落地时同步更新。

---

## 3. 横切前置项（新增，原始文档未覆盖）

### 3.1 多实例并发 × 破坏式重建 — BLOCKING，必须先设计

现状：repomap cache **零文件锁**（全包无 flock 命中），只靠 writeFileAtomic
(tmp+fsync+rename) 的文件粒度原子性 + chunk 目录 10 分钟 prune 宽限窗；architecture.md
§14.7 的多实例安全清单（日志 PID / MEMORY.md.lock / blob PID 回收 / worktree TTL）
**不含 repomap cache**。`cacheManifestVersionValid`（cache.go:587）是双向严格相等：
旧二进制读到 schema-4 manifest 的拒绝方式与新二进制读 schema-3 完全一样。

后果：升级窗口内新旧二进制共享同一 cache 目录（同 repo + 同 CWD/.codrax，或共享
cache_dir）时，每个进程都拒绝对方的 manifest → full rescan → 装回自己的版本 —
**无限 ping-pong，且现有日志（裸 nil）与 cache 损坏不可区分**。同版本变体也真实存在：
eval/parallel_all.sh 并行跑 23 个共享 fixture cache，bump 后第一次扫批就是 N 个 runner
并发重建同一批 cache 目录（last-Close-wins 可容忍，但重复劳动 + chunk 目录翻搅）。

P0 方案（最小、不引入锁）：

1. **新于当前 → 拒载不覆写**：manifest `SchemaVersion > cacheSchemaVersion` 时，
   旧二进制在内存中 full scan 但**不安装**自己的 manifest/chunks（不降级覆写新 cache），
   并打 WARN `repomap: cache written by newer codrax (schema N > M); scanning in-memory without overwriting`。
   旧于当前 → 正常重建覆写（标准升级路径）。这把 ping-pong 单向截断。
2. **原因可见**：两个 loader 的全部 ~10 种裸 nil 返回改为带 typed RejectReason
   （与 P0.3 共用同一改造，见 §6.3）。
3. **文档落账**：architecture.md §14.7 增补 repomap cache 的多实例契约
   （单写者约定 + 上述不覆写规则 + eval 并发重建为已知可容忍场景）。
4. 跨进程 advisory lock 明确**不做**（P0 非目标，留作后续 watcher/sync 阶段的前置）。

### 3.2 写模式交互（原始文档与各勘察均未触及）

write 三 lane 的 skill 都带 repo_map（internal/skill/defaults.go:696/772/777/813），
且 applyPreHook 把 busCtx.RepoRoot 换成 per-Run worktree（stage_hooks.go:515）→
CacheDir 按 worktree 绝对路径 slug → **每次 write run 必然 full scan 且留下一个
永久孤儿 cache 目录**（L5 无条件丢弃 worktree 后无人回收；现状已如此，P0.1 的更大
relation payload 会放大它）。P0 范围内的处理：

- P0.3 banner 在 write lane 恒为 `source: full_scan` — 行为正确，措辞按 §6.5 统一，
  不为 write lane 特判。
- P0.2 的 tier 预算在 write lane 同样生效（worktree 图的 FileIndex 即真实规模）— 无需特判。
- 孤儿 cache 目录 GC 是**独立于本设计的存量泄漏**，另开任务处理（不并入 P0，
  避免 no-defer 红线误伤：它不是本设计引入的问题，是本设计的勘察副产物）。
- write-mode eval 家族（Session 33-37 谱系）纳入 §9 的重跑批次。

### 3.3 SymbolOracle 硬门禁词汇表扩张（P0.4 副作用，需显式签字）

BuildGraph 无 kind 过滤地把所有 symbol 收进 SymbolDefs（build.go:83-95），而
repomap/oracle.go 是下游 LLM-claim 硬校验的单一事实源（logtriage entity merge /
mermaid bare-identifier check / finalizer 答案一致性 / enumeration-label Stage-4 gate，
contract_check_block.go:2221/3084）。Kind="route" 落地后
`graph.SymbolExists("GET /users/:id")` 为真 — 硬门禁接受词汇被扩张（放宽方向）。

裁定：**接受**。理由：route 名是真实注册的代码事实，oracle 接受它们与"oracle 反映
图中存在的符号"的契约一致；含空格/斜杠的 route 名非 identifier-shaped，flat 索引
（case-fold 去 _ -）侧暴露有界。配套动作：在 P0.4 的测试中显式 pin 两条 —
(a) route symbol 使 SymbolExists 为真；(b) identifier-shaped flat 查询（如 `getusers`）
**不**因 route symbol 产生新命中。任何未来想按 kind 过滤 oracle 的提案需重新过
precise-signals 红线评审。

---

## 4. 需求一修订：Relation 模型升级（P0.1）

### 4.1 结构（修订后）

```go
// types/types.go — RelationEndpoint 复用现有定义不动 (types.go:179)
type Relation struct {
    Kind string `json:"kind"`

    FromEP RelationEndpoint `json:"from_ep,omitempty"` // 新增；内容可选（见 4.2）
    ToEP   RelationEndpoint `json:"to_ep,omitempty"`   // 已存在

    File string `json:"file"`
    Line int    `json:"line"`

    Confidence float64           `json:"confidence"`  // 必填，>0；语义见 4.4
    Provenance string            `json:"provenance"`  // 必填，构建期枚举
    ResolvedBy string            `json:"resolved_by"` // 必填，构建期 extractor 名
    Metadata   map[string]string `json:"metadata,omitempty"`

    // From/To：Step A 保留并继续填（保证 ToEP.Name==To 字节等价迁移），
    // Step B 一次性原子删除（连同全部读者迁移完成后）。
}
```

与原始文档 §6.2 的差异：`FromEP`/`ToEP` 带 omitempty（24/30 构建点今天不填 ToEP，
call/type_usage 的 from 端合法无符号名）；`Name` 维持 omitempty（原始文档要求必填，
但渲染端已有 relationMapEnclosingSymbol(fi, rel.Line) 兜底推导 source 端，
render.go:614-616）。

### 4.2 From/To 删除 — 两步是强制项

原始文档 §6.3 "P0 推荐直接删除" 不可行：删除影响面跨 **6 个外部包**，其中
internal/analysis/dataflow/lower.go:350-368 以 rel.To 为**主路径无 ToEP 兜底**
（含 :366 直接嵌入用户可见 evidence 文本），explorer/symbol_resolver/ground/
emit_evidence/eval-harness 各有兜底读。两步走（可同一里程碑内按序提交）：

- **Step A（增量）**：24 个只写 legacy 字符串的构建点补 `ToEP{Name: <原To>, File, Line}`；
  全部 30 个耐久构建点补 FromEP（inheritance/embedding 的 15 个点填
  `FromEP{Name: hostName}`，名字在每个点的作用域内现成；call/type_usage 填
  `FromEP{File, Line}`，Go 可加填 enclosing function 名）+ 三个诊断字段；schema bump；
  所有读者按 §4.6 清单迁移到 endpoint。**迁移字节等价的前提是 Step A 先保证
  ToEP.Name==To 在每个点成立** — 乱序落地会让 dataflow evidence 文本静默变化。
- **Step B（破坏）**：一个原子 commit 删 From/To 字段 + scoped_projection legacy
  rebase（scoped_projection.go:242-246，同时给 FromEP.File 补上对称 rebase）+
  CallersOf legacy API（graph.go:231，唯一非测试调用者 render.go:1305 先迁
  CallersOfID）+ sidecar From/To 渲染（cache.go:826,841）+ 3 个外部测试文件的
  构造迁移。

### 4.3 ResolvedBy 两层拆分（修订原始文档 §6.7 + §6.12 验收）

原始文档要求 `ResolveCallTarget` 产出的 typed resolution 持久化
`ResolvedBy=go_method_index` — **架构上不可行**（核对表 #15：fullScan 流式写 cache
早于 BuildGraph，回写造成 full/incremental 两路 cache 分叉）。拆为：

- **构建期（持久化）**：extractor 粒度名，如 `go_ast_identifier_call` /
  `go_ast_selector_call` / `go_struct_embedding` / `java_superclass` /
  `js_heritage` / `cangjie_regex_extend` / `gin_route_literal`（P0.4）。
- **解析期（渲染时标注，不持久化）**：relation_map 每行已逐行调
  `relationMapCallTarget` → `g.ResolveCallTarget`（render.go:821）；扩展其返回
  命中步骤（same_package_function / receiver_method_index / cross_package_receiver），
  追加到行尾标注。查询期既有 TypedRelationPrecision enum
  （ExactSymbolID/…/Heuristic，typed_relation.go:494-620）即此层语义，维持查询期。

**验收条款相应改写**：§6.12 "ResolveCallTarget 写入 ResolvedBy=go_method_index"
→ "relation_map call 行展示解析期标注（含 method_index 命中步骤），cache 中
ResolvedBy 持久化构建期 extractor 名"。

### 4.4 Confidence — 单一数值来源 + 仅软用途

对齐既有 `TierDiscount` 常量（parse_fallback.go:50）做单一刻度，**不引入**原始
文档 §6.5 的独立 1.00/0.95/0.90/…/0.30 第二词汇表（CodeGraph 的教训，§1.2）：

| 值 | 含义 | 对应 |
|---|---|---|
| 1.00 | Tier-1 AST 边（全部 tree-sitter extractor + Cangjie 手写 parser） | TierDiscount(1) |
| 0.85 | Tier-2 次级文法（ArkTS 骑 TS 经共享 extractJS） | TierDiscount(2) |
| 0.70 | route_resolver literal 边（P0.4） | 新增档 |
| 0.60 | regex 抢救边（今天仅 extract_cangjie.go:424 Tier-2 regex inheritance） | TierDiscount(3) |
| 0.30 | path-only 弱信号（当前无生产者，保留档位） | TierDiscount(4) |

红线落实（feedback_precise_signals_for_hard_gates）：

- 加载校验是 **presence-only**：`Kind!="" && Provenance!="" && ResolvedBy!="" && Confidence>0`
  （float64 无法区分缺失与 0.0，而 §6.5 反 0.0 规则禁止生产者写 0 — 等价性写进字段注释）。
- **永久禁止**任何 value-threshold 硬门禁（如 reject confidence<0.5）和
  contract.Check / emit-time gate 读取 Confidence；其用途仅限渲染标注 + 软排序折扣。
- 原始文档 §8.3 "symbol payload lacks required fields" 具体化为最小精确 presence
  检查：`Name!="" && File!=""`；§10.3 "route kind unknown → reject" 条款**删除**
  （与 Kind 自由词汇设计不相容，否则每加一个 kind 都要同步 load 侧白名单）。

### 4.5 Provenance 枚举（修订 §6.6）与 import 边

构建期枚举收敛为实际存在的生产形态：`tree_sitter` / `cangjie_parser` /
`regex_fallback` / `route_resolver`（P0.4）。原始文档枚举里的 `import_resolver` /
`method_index` / `return_type_inference` / `interface_conformance` 不进此枚举 —
它们都不在 Relation 构建期发生：

- **import 边没有 Relation 载体**（核对表 #5）：P0 **不物化** import Relations
  （会膨胀 cache chunk 并扰动 RankIndex 语义）。relation_map import 行（由
  g.ImportGraph 构建，render.go:602-611）在渲染时加常量标注
  `[import, import_resolver/<lang>]`。§6.12 对应验收条款改写为渲染层表述。
- **Implements 行同理**：relation_map 的 implements 行来自 Symbol.Implements
  （populateImplementers 后处理），无边载体 — 渲染时常量标注
  `[implements, interface_conformance]`，不持久化。
- **§6.10 降范围**：provenance/confidence 默认展示仅 relation_map
  （semantic_subgraph 无 Relation 行，结构不可能 — 核对表 #13）。

### 4.6 完整改动清单（按序）

1. types/types.go:188 — 结构变更（§4.1）；types.go:75 Kind 注释同步刷新（现实 ~37 种 kind）。
2. 30 个耐久构建点（extract_go.go 5 / extract_rust.go 5 / extract_java.go 4 /
   extract_c.go 4 / extract_python.go 3 / extract_javascript.go 3（JS/TS/ArkTS 共享，
   confidence 按 ParseTier 取 1.00/0.85）/ extract_kotlin.go 1 / extract_ruby.go /
   extract_swift.go / extract_cangjie.go:424（0.60, regex_fallback）/
   cangjie_parser.go:248,323（1.00, cangjie_parser））全部补齐五字段。
3. cache.go:183 `cacheSchemaVersion` 3→4；extractorVersions 全 15 项 bump（边形态
   全变）；新 `validateCachedFileInfos` presence 校验挂三处 — loadChunkedFileInfos
   (:533-540)、resume 路径 loadChunkDirInto (:1176-1211，防同版本写者 bug 经孤儿
   chunk 走私)、**legacy 单文件 loader 直接删除**（破坏式策略授权，连带
   TestLegacyCachePayloadStillLoads 删除）。
4. loader 裸 nil → typed RejectReason（与 §3.1 / §6.3 同一改造）。
5. sidecar relations.md writer（cache.go:816-862）改渲染 endpoint+provenance，
   去掉临时 Kind=import Relation 合成与 From/To dedupe key。
6. 读者迁移（Step A 后字节等价）：graph.go:269 CallersOfID、graph.go:318
   ResolveCallTarget name fallback 删支、rank_index.go:92/95/101、
   typed_relation.go:504/509/532/537 relationEndpointNameVariants 去第二参 +
   :631/:655 改读 FromEP.Name、render.go:842-843、dataflow lower.go:350-368 +
   engine.go:163、explorer.go:2895/4778/13873/13882（:12362/12366 所在的
   buildCrossReferenceMap 疑似零调用 — 核实后删除而非迁移）、symbol_resolver.go:645-647、
   ground.go:1059/2485/2852、emit_evidence.go:1538/2942/3005/3065、
   eval/repomap_v3/harness/main.go:481/489/508/525。
7. render.go relation_map 行格式：`relationMapRowText`（:894-908）追加
   `[kind, provenance/resolved_by, confidence=x.xx]`；call 行加解析期标注（§4.3）；
   import/implements 行加常量标注（§4.5）。新字符串全部过
   prompt_hygiene_test.go:59 门禁 + 写-prompt 红线 checklist。
8. **skill prompt 校准**（完备性批判新发现）：defaults.go:38/:136 两处
   "repo_map / source_inventory rows are verified navigation facts" 在 heuristic
   边带 confidence=0.70 标签后不再准确 — 改为"graph-backed navigation rows;
   confidence labels grade edge strength; verify before citing"语义。此项是
   独立 prompt 红线 checklist 交付物（BLOCKING 流程）。

### 4.7 测试与防线

- **首要防线（静态可审计红线）**：新结构性测试断言每语言 parse fixture 产出的
  全部 relation 通过 presence 校验 — 写者回归在 make test 失败，而不是线上静默
  full-rescan 循环（per-record 加载校验只是兜底，单条坏边即整 cache 拒载的代价
  必须由编译期/测试期防线前置吸收）。
- 重录：render_test.go 5 个钉死全行字符串的测试（:609-697 relation_map 行、
  :397-399 call_path 箭头、:534 subgraph 链）；cache_version_test.go round-trip；
  eval/repomap_v3 baseline.json/md（ambiguity 指标换 ToEP.Name 键）。
- 不动：eval/cases 167 个 .case 无一钉 relation 渲染格式（已核实零命中）。
- 新增：旧 payload 拒载 / schema mismatch 带因拒载 / 新 JSON round-trip /
  渲染含 provenance/confidence / confidence 缺失视为 invalid cache。

---

## 5. 需求二修订：输出预算（P0.2）

### 5.1 tier helper

新文件 `internal/tool/repomap/types/size_tier.go`：`SizeTier` typed enum
（tiny/small/medium/large/very_large，切点 150/500/5000/15000 维持原始文档值 —
即 CodeGraph 实测值）+ `RepoSizeTier(fileCount int)` + per-view `DefaultTopN(view, tier)`。
输入 `len(g.FileIndex)`，nil 时回退 `len(g.Files)`（测试构造图无 FileIndex）。
多仓语义：tier 读 **GraphFromBusContextOrLoad 实际返回的图**的 FileIndex —
sub-repo 调用得 sub-repo 规模、scoped projection 得投影规模；预算匹配本次调用
可枚举的范围，措辞用 "this repository scope" 以兼容跨调用翻转。

### 5.2 默认值矩阵（**拒绝**原始文档统一 4/5/8）

理由：relation_map 的单一 topN 同时封顶 source 候选、relation 行、verification
文件三个 section（render.go:368-369,429），砍到 8 直接饿死边覆盖；source_inventory
行是单行成员且 count==len(members) 支撑计数清单语义（60/24，
source_inventory_reconcile.go:1398-1404）。只降 tiny/small，medium+ 保持现值
（eval bar 不可降级红线 — medium 档变化会波及 codrax 自身尺寸的 12-family 评测）：

| view | tiny | small | medium+ |
|---|---|---|---|
| task_map | 6 | 10 | 20（现值） |
| relation_map | 12 | 20 | 40（现值） |
| source_inventory（未分组） | 24 | 40 | 60（现值；分组 24 不动） |
| overview / file_map / semantic_subgraph | 不分层 | 不分层 | 15/50/8 |

overview **必须**不分层：analyzer prompt 预注入走 `ViewParams{}`（TopN=0，
analyzer.go:618），分层会静默改变每一个 analyzer 初始 prompt。继承 CodeGraph 的
monotonic 不变量并写单测：更大 tier 的任何默认值不得小于更小 tier。

显式 top_n 永远优先（现有全部 `if topN<=0` 位点已天然满足，零代码）；
user TopN > 2× tier 默认时输出尾追加一行提示（精确 int 比较驱动软文本，合规）。

### 5.3 低价值文件降权 — 拒绝硬排除，走既有 typed 通道

- **机制**：复用 keyword_search.go:491-507 的既有模式 —
  `types.ClassifySourcePathRole` + `SourcePathRoleIsAuxiliary` 判定后 **score×0.35**
  （软降权，永不清零；task_map 的 score<=0 显示过滤会把清零变成事实硬排除，
  render.go:1370-1373）。原始文档"必要时硬排除"违反 precise-signals 红线，删除。
- **放置**：RankGraphScores 的 **opt-in** options 变体，默认路径字节等价 —
  keyword_search.go:498 已对组合分（含 repoMapScores）施加自己的 0.35x，
  无条件下沉会双重惩罚到 0.1225x。只有 buildTaskMapData 在
  `ViewParams.DeprioritizeAuxiliary` 置位时启用。
- **意图检测**：读 `ctx.AnalysisIR.RequestModel.SourceScopeProfile.AllowsAuxiliaryPrincipal()`
  （typed 通道，其注释本身就写着防关键词扫描，source_scope_profile.go:39-64）。
  AnalysisIR 为 nil（REPL 直用/预热）→ 降权 **OFF**（保守；既有无条件 0.5x test
  乘子 rank.go:339-341 已提供基线降噪）。**禁止**对 query 文本做关键词扫描
  （feedback_no_custom_keyword_matching）。
- **范围修剪**：原始文档 §7.4 列表中 *.svg/*.png/locale-json 无意义（非源码文件
  从不入图 — extToLang 白名单 + git ls-files）；mock/mocks 不在
  ClassifySourcePathRole 角色集内（source_path.go 零 mock 规则）— 扩分类器会
  全局波及 keyword_search 分层，**P0 放弃 mock 项**（如确需，单独提案过审）。
- edit_impact / call_path / implementers 结构性豁免（不走 rank 路径，零代码），
  补一条不变量测试。
- §7.3 `require_scope_hint_when_query_empty` **折入既有机制**：relation_map 已有
  broad_fallback 窄化建议（render.go:402-403）、task_map 空 query 已回退 overview
  （render.go:142-148）— 不再新增 require 型门禁，very_large+空 query 场景由
  §5.4 hint 文案覆盖。include_relationship_summary / include_additional_candidates /
  "additional candidates omitted" 提示 → 推迟 P1（风险登记 §10 同步标注）。

### 5.4 narrowing hint

单一站点：tool.go Execute 输出**尾部**（两条装配路径 — render 视图 :351 后、
source_inventory :309-321 后，均在 StoreBlob 之前；StoreBlob head+tail 预览已验证
保 banner/hint 可见），仅 tier>=large 时 ~3 行；relation_map 已打 broad_fallback
窄化建议时抑制，避免双重唠叨。措辞禁内部术语（tier 枚举名/FileIndex/
SourceScopeProfile 不得出现），学 CodeGraph 引导"多次窄查询"而非"少看"。
tool.go:142 的 top_n schema 描述同步教 size-aware 默认（已核实未被
tool_schema_test.go 钉死，但 schema 文案 load-bearing 红线照常走查）。

---

## 6. 需求三修订：索引状态 banner（P0.3）

### 6.1 freshness 模型重构（核心修正）

被服务的磁盘图**每次调用都经 hash 比对核账**（核对表 #12）→ 原始文档 §8.4 的两个
stale 触发条件一个不可达、一个是嘈声信号。真正的 staleness 窗口在**内存复用**：
LRU 常驻图（multigraph.go:256 EnsureLoaded）、ctx.SearchGraph 复用
（multigraph_facade.go:340）、scoped projection（scoped_projection.go:30/54/78/93）
均不重跑 hash diff。修订为三态：

```
fresh   — 本次调用经磁盘核账（full_scan / cache_hit / incremental）
reused  — 内存复用图；banner 报 ScanTime 距今时长，不报 stale
unknown — 零值状态；渲染 "source: unknown"，永不伪造 full_scan
```

HEAD mismatch 至多作为 informational fact line（`- note: cache HEAD abc123 ≠
current def456`），**不挂 stale 词**。`RebuildRequired` bool 删除 — cache 拒载与
重建在同一次调用内闭环（loadFromCache→fullScan fallback，tool.go:838-857），无
持久态；改为 `RebuildReasons []string`，banner 渲染
`- rebuilt: previous cache rejected (<reason>)`。

### 6.2 状态结构与装配

```go
// types/types.go — 挂在 Graph.Metadata 上（值拷贝随 cloneGraphForRanking 免费继承）
type RepoMapIndexStatus struct {
    Source            string   // full_scan|cache_hit|incremental|in_memory_reuse|scoped_projection|unknown
    Freshness         string   // fresh|reused|unknown
    ScanTime          time.Time
    CacheWrittenAt    time.Time
    RepoHead          string   // manifest 记录值（短 SHA，gitHeadSHA）
    SchemaVersion     int
    ExtractorVersions map[string]int
    FallbackFileCount int      // BuildGraph Metadata 装配时统计 ParseTier>=2
    UnresolvedImportCount int  // len(Metadata.UnresolvedImports)，已有
    RebuildReasons    []string
}
```

与原始文档 §8.2 的差异：去掉 RebuildRequired/Stale/StaleReasons（§6.1）；
Source 枚举复用 ctypes.RepoMapScanMode 词汇 + 两个改名 — 原始文档的
`multigraph_resident`/`projected_graph` 泄漏内部架构词（TypedDenials 先例
tool.go:176-178 + no-internal-info 红线），渲染为中性的 `in_memory_reuse` /
`scoped_projection`；`partial_cache` 即既有 `incremental`。

盖戳三类站点：(1) buildOrLoadGraph 三助手在 BuildGraph 返回**后**盖
（BuildGraph 会整体覆写 Metadata，build.go:116-123）；(2) facade 复用路径
rankedLoadedGraph/reusableSearchGraph 只在 **clone 上**改写
Source=in_memory_reuse（LRU 常驻图有并发读者，严禁原地改）；(3) scoped projection
四个出口盖 scoped_projection。补结构性测试：GraphFromBusContextOrLoad 每个分支
产出非空 Source。

### 6.3 loader 改造（与 P0.1/§3.1 同一批）

`LoadFileInfosWithProgress` 返回 typed 结果
`{Files, Header{SchemaVersion, ExtractorVersions, RepoHead, WrittenAt}, RejectReason}`；
`cacheManifestVersionValid` 拆出带因变体（schema_version_mismatch vs
extractor_version_mismatch），resume 路径两个调用点（cache.go:1137/1185）保留 bool
包装不动（cache_resume_test 零重录）。**早期 manifest 预检**：buildOrLoadGraph 在
ChangedFiles hash 全量 pass 之前先读 manifest header 验版本 — bump 后第一次运行
省掉整仓 hash pass，并前置打出原始文档 §10.5 的
`repo_map: old cache schema is incompatible; rebuilding index`。

### 6.4 banner 渲染

新 `prependRepoMapIndexStatusBanner(graph, output)`，在 tool.go Execute 两条输出
装配路径前缀插入（跟随既有 prepend advisory 模式，tool.go:388/425/501）—
**不进 render/**，render_test.go 全部钉死字符串零重录。fresh 基线 3 行：

```
Repo Map Index:
- source: cache_hit (scanned 2026-06-12T10:01:00Z)
- files: 1284 | symbols: 9130 | relations: 18420
```

条件行（各 +1，总数 ≤5）：rebuilt 行 / parse fallback 行
（`- parse fallback: N files; edges from fallback-parsed files may be incomplete`）/
复核提示行 — **仅当** fallback>0 或 Source∈{in_memory_reuse, scoped_projection,
unknown} 时出现，措辞复用既有契约词族（tool.go:55 Description 与 render.go:375
relation_map Intro 的 "read or grep the selected files before citing"），不发明
第三种说法。fresh 且零 fallback 时无任何警示行（反噪声）。ScanTime 与
CacheWrittenAt 标签必须区分（cache_hit 时 ScanTime 是本次 build 时刻，不是扫描
时刻 — 不区分会让热 cache 读成新扫描）。

多仓语义：per-served-graph，**不聚合**（每次 Execute 恰好解析到一个图 —
active-set gate 强制单 sub-repo 路径）；MultiGraph.Metadata() 聚合值禁止入 banner。

### 6.5 REPL 残余窗口

`/repos refresh` 只强制重发现 topology，不失效 LRU 常驻图与 SearchGraph 预热
（repos_cmd.go:431-466）→ 用户显式刷新后 banner 仍可能 `in_memory_reuse` 且
ScanTime 陈旧，进程内无回到 fresh 的路径。**修复纳入 P0.3**：/repos refresh 追加
multigraph LRU drop + Mutable SearchGraph 失效（符合用户对 refresh 的直觉预期，
改动小）。banner 措辞只承诺本次调用观察值（多进程窗口不可消除）。

---

## 7. 需求四修订：route resolver（P0.4）

### 7.1 范围修订

| 框架 | 裁定 | 理由 |
|---|---|---|
| Go gin + chi | ✅ 首批 | 同一 call_expression walker、两张动词表 + 各自 import gate；codrax 主用户群 Go 服务端 |
| Python FastAPI | ✅ 首批 | decorator walk；pyExtractDecorated 已迭代 decorated_definition 只是跳过了 decorator 节点（extract_python.go:126-143）|
| Java Spring | ✅ 首批 | annotation walk + class 前缀拼接；vendored 文法含 annotation/marker_annotation 节点（已核实）|
| gorilla/mux | ⏸ 批 2 | `HandleFunc(...).Methods("GET")` 链式 AST 形态不同，半支持不如不支持 |
| Flask | ⏸ 批 2 | `@app.route + methods kwarg` 形态不同 |
| Kotlin Spring | ⏸ 批 2 | kotlinModifiers **显式跳过 annotation 节点**（extract_kotlin.go:323-325）— 需要新写 annotation reader，原始文档 "Java/Kotlin Spring" 打包低估了 Kotlin 侧工作量；且 codrax 的 Kotlin 暴露面是 Android 非 Spring |
| Express/NestJS | ⏸ 批 2 | extractJS 由 JS/TS/**ArkTS-Tier-1 共享**（extract_arkts.go:80-89）— import gate 可保 .ets 惰性，但必须配 ArkTS no-op 回归测试后刻意落地（HarmonyOS 红线）|

HarmonyOS 焦点**不**构成做 ArkTS route 的理由：ArkUI 是客户端 UI（其 decorator
extractor 产 component/state-field kind），HTTP route 在用户同时指向的 Go/Python/
Java 服务端仓里。

### 7.2 识别规则（按代码实际解析习语）

硬门禁 = **per-file import path 逐字匹配**（精确信号）：Go `github.com/gin-gonic/gin`
/ `github.com/go-chi/chi(/v5)`；Python `fastapi` 或前缀 `fastapi.`；Java 以
@RestController/@Controller class annotation 本体为门禁（org.springframework
import 做佐证）。receiver 名（r/router/…）是嘈声信号 — **永不**做唯一门禁，只可
调 confidence 或跳过。goLocalScope 只绑参数/receiver 不绑 `r := gin.Default()`
局部变量（extract_go.go:501-540）— 不发明假类型检查，接受 0.70 置信的残余误报
（import gate 后 `client.GET("/x", cb)` 类）并靠 provenance 标注消化。

- **Gin（修正原始文档 §9.5 的二参假设）**：handler 是**变参且取最后一个**
  （`r.GET("/x", mw1, mw2, handler)`）。规则：arg0 必须是以 `/` 开头的
  interpreted_string_literal；最后一参 identifier/selector_expression → route
  Symbol + handler 边；func_literal → **route Symbol 但无 handler 边**（gin 实战
  极常见，文档化勿当 bug）；非 literal path → 不产出。动词表来自 gin
  RouterGroup API 面（GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/Any；chi 为
  Get/Post/… 大小写差异）— **框架 API 面定表，不许 fixture 定表**（5-Q 泛化审计
  对每个识别器单独适用）。Group 前缀：同函数内 `g := <recv>.Group("<lit>")`
  value-binding 走查，嵌套拼接；receiver 不可解析 → 局部 path +
  `Metadata["partial_path"]="true"`。
- **FastAPI**：decorator→call→attribute(app.get/router.post)+首个字符串参；同文件
  module-level `x = FastAPI()` vs `x = APIRouter()` 赋值跟踪 — APIRouter 绑定 →
  partial_path=true（include_router 跨文件拼接按原始文档 §9.6 跳过）。
- **Spring**：class-level @RequestMapping literal + method-level
  @Get/Post/Put/Delete/PatchMapping literal 拼接（同一 javaExtractClass pass 内，
  同 class 节点）；value=/path= 属性形态与裸 literal 支持；常量引用 → 跳过边。
  javaHasModifier 现在只做 modifiers 文本 substring 检查（extract_java.go:222-227），
  route pass 改为迭代 annotation 子节点。

新代码放 `index/route_go.go / route_python.go / route_java.go`（各语言
extractorVersions 自然覆盖），钩点跟随各 extractor 既有后处理习语
（extract_go.go:44-47 `rels = append(rels, goExtractCalls(...))` 旁）。

### 7.3 Symbol / Relation 形态

```go
Symbol{ Name: "GET /users/:id", Kind: "route", Exported: true,
        Signature: "GET /users/:id -> GetUser", Line: <注册行>,
        Parent: "" }  // Spring 也留空 — file_map 渲染 Parent.Name 会拼成
                      // "UserController.GET /users/{id}"（render.go:1140-1141）
Relation{ Kind: "reference",   // 不新造 "route" kind — 会被 relation_map 行
                               // switch (render.go:628) 与 kind 白名单 (:714-727) 静默丢弃
    FromEP: RelationEndpoint{Name: "GET /users/:id", File: f, Line: 42},
    ToEP:   RelationEndpoint{Name: "getUser", Receiver: "UserController"},
                               // handler 身份放边上，匹配 MethodIndex 键形
                               // (pkg, Receiver-or-Parent, Name)，为将来 eager 解析铺路;
                               // ToEP.ID 构建期留空（SymbolID 仅 BuildGraph 派生）
    File: f, Line: 42, Confidence: 0.70,
    Provenance: "route_resolver", ResolvedBy: "gin_route_literal",
    Metadata: map[string]string{"framework":"gin","method":"GET","path":"/users/:id"},
}
```

partial_path 边维持 0.70 + metadata 标记（原始文档 0.60 档留给"纯约定"边，批 1 不产出）。

### 7.4 渲染与消费侧

- 唯一必需渲染改动：file_map 闭合 kindOrder 追加 "route"（render.go:1100-1103，
  唯一丢弃未知 kind 的视图）。relation_map "reference" 行、task_map、symbols.md
  sidecar、source_inventory 全部零改动直通（tool_scope_test.go:618-668 的合成
  route symbol 端到端测试升格为消费侧回归锚点）。
- "reference" kind 首次有真实生产者 → 4 个从未被运行过的潜伏消费点列入验证清单：
  rank_index.go:94 RefCount（route 边会**新增**对 handler 文件的排序加权 — 行为
  合理但需 pin）、typed_relation.go:284、render.go:628/:723。
- member-key 退化：`NormalizedSurfaceSymbolTail` 取首个空白分隔段 →
  全部 GET route 共键 "get"（dedupe 安全 — file:line 在 dedupe key 内；分组/覆盖键
  退化）。修复放 `sourceInventoryCandidateForSymbol`（:3963）做 route-role-aware
  键（小写全名+空白折叠）— **不动共享 canonicaliser**（红线）。
- SymbolOracle 词汇扩张按 §3.3 签字接受 + 两条 pin 测试。

### 7.5 版本与测试

- extractorVersions：Go 2→3、Python 1→2、Java 1→2（cache.go:195-211）。
  cacheSchemaVersion **不**因 P0.4 单独 bump（Kind 自由字符串 + 既有 JSON 直通）。
- 顺序硬约束：**P0.1 先行**（边的五个诊断字段是 P0.4 的承载前提；若强行先行，
  route 边只能裸 reference + Signature 临时携带 — 违反 §9.8 验收，不做）。
- 新测试 route_go_test.go / route_python_test.go / route_java_test.go（inline-source
  模式，参照 extract_kotlin_test.go）覆盖 §9.8 六例：简单 route / method literal /
  handler selector / 非 literal 动态 route 零产出 / class 前缀拼接（Spring concat +
  gin Group）/ func-literal handler（有 symbol 无边）。
- **现成负向 golden**：eval/fixtures/java-annotation-router 是自定义 @Route
  （无框架依赖）的 Java 路由 fixture，配两个钉死事实的 case
  （sr_java_annotation_route / sr_java_handler_impls）— Spring 识别器若写松
  （任何带 path 属性的注解 / *Mapping 名字模式）会在此 fixture 上伪造 route 并
  扰动两案。把它显式纳入 P0.4 验收（识别器必须在此 fixture 上零产出）。

---

## 8. 里程碑修订

| 里程碑 | 内容 | 关键依赖 |
|---|---|---|
| **M1 = P0.1** | Relation 五字段 + 30 构建点 + schema 3→4 + typed RejectReason loader + 多实例不覆写守卫（§3.1）+ 读者迁移 Step A/B + relation_map 诊断渲染 + skill prompt 校准 | — |
| **M2 = P0.3** | IndexStatus + 三类盖戳 + banner + 早期 manifest 预检 + /repos refresh 失效修复 | 骑 M1 的 loader 改造（相邻落地免二次翻挖） |
| **M3 = P0.4** | gin+chi / FastAPI / Spring 三识别器 + file_map kindOrder + route-aware member key + oracle pin 测试 + 负向 fixture 验收 | 硬依赖 M1 字段 |
| **M4 = P0.2** | size_tier helper + per-view 矩阵 + opt-in 软降权 + narrowing hint | 完全独立，可与 M2/M3 并行开发，最后合入 |

eval 重跑合并为两批（P0.3 banner 改变**每个** repo_map 结果首行，诚实重跑单位是
全家族）：M1+M2 落地后跑一次 12-family + write-mode + multirepo 家族；M3+M4 后再
跑一次。重录规约：按 no-eval-bar-relaxation 红线，重录靠重跑产出，不许放松断言。
量化基数：eval/cases 167 案（111 含 EXPECT_CONTAINS）/ 23 个 fixture cache 全量
重扫（parallel_all.sh 并发场景见 §3.1.3）/ run.sh tool_repo_map 计数指标会因
P0.2 漂移（convergence_audit 基线同步）/ repomap_v3 baseline 重录 / 单测重 pin
~7 个 + 新增 ~15 个。

---

## 9. 文档与配置交付物

- CLAUDE.md：repomap 红线中 fallback WARN 格式改为实际
  `repomap: <lang> <file> tier N→M: <reason>`（并补一条 pin 测试，当前零测试钉格式）。
- docs/architecture.md：§14.7 增补 repomap cache 多实例契约（§3.1）；:2370
  "零 staleness 窗口" 表述改写为三态 freshness 模型；§10 配置节同步
  （P0 无新 yaml 键 — repomap_min_parse_tier 等既有键不动；原始文档 §8.4 TTL
  按"future config"维持不实现）。
- parse tier 编号褶皱：ParseAttempt 与 extractArkTS/extractCangjie 用 tier=3 表
  path-only，而 FileInfo.ParseTier 文档与超时路径用 4 — M1 前先归一（直接影响
  tier→confidence 映射与 FallbackFileCount 口径：ArkTS path-only 文件按现状会错拿
  0.60 而非 0.30）。

## 10. 风险登记（修订与新增）

| 风险 | 等级 | 处置 |
|---|---|---|
| 新旧二进制 cache ping-pong | blocking→已设计 | §3.1 不覆写守卫 + WARN + §14.7 落账 |
| confidence 被未来代码用作硬门禁 | 高 | 字段注释 + 评审红线 + 禁止清单写入设计（§4.4） |
| ResolvedBy 验收条款偏离原始文档字面 | 中 | §4.3 显式改写验收文字，评审时对账 |
| Step A/读者迁移乱序 → dataflow 用户可见 evidence 文本静默漂移 | 高 | §4.2 顺序硬约束 + 字节等价断言 |
| route 误报（import gate 后非 router receiver） | 中 | 0.70 + provenance 标注；不加 receiver 名硬门禁（红线）；负向 fixture 验收 |
| route RefCount 新增 handler 文件排序加权 | 低 | 潜伏消费点验证清单 + pin（§7.4） |
| banner 被模型当语义引用 | 中 | 措辞过 prompt 红线 checklist；fresh 零警示行反噪声 |
| 单条坏边致整 cache 拒载 → 静默全量重扫循环 | 中 | 结构性测试前置防线（§4.7 首要防线） |
| tiny/small 降权误伤 REPL 直用 test 提问 | 低 | nil-profile 默认 OFF（§5.3） |
| eval 指标漂移（tool_repo_map 计数 / 小 fixture tier 档） | 中 | 两批合并重跑 + convergence 基线同步（§8） |
| 写模式孤儿 cache 目录膨胀 | 存量 | 独立任务跟踪（§3.2），不并入 P0 |

## 11. 验收总标准（替换原始文档 §13 中被修订的条款）

维持：旧 cache 明确拒载重建零 migration / FromEP+ToEP 结构化 / 三诊断字段必带 /
小仓更短大仓有 hint / 显式 top_n 不被忽略 / route 一等 kind + 至少 Go/Python/JVM
各一 fixture 产出 / 不引入 SQLite / 不做 watcher / 不改 tool name。

修订：relation_map（**仅此视图**）展示 provenance/confidence；ResolvedBy 持久化
构建期名 + 渲染期解析标注；import/implements 行为渲染期常量标注；cache 拒载必须
带 typed 原因且新于当前版本时不覆写；banner 三态 freshness 且 HEAD 差异仅
fact line；route 识别器在 java-annotation-router 负向 fixture 上零产出；
SymbolOracle 两条 pin 测试通过。
