# Survey Mode — Discussion Notes (NOT GREENLIT)

**Status**: 讨论中,**未授权实施**。本文档保存当前讨论状态便于后续 session 接续。
**Created**: 2026-05-05
**Predecessor discussions**: 无(本议题首次讨论)
**Successor**: 待用户决策 §10 中 4 个 open questions 后,方可升级为 actionable design doc。
**Owner**: 任意接手者(本文档目标:陌生开发者读完一遍可继续讨论 + 直接进 PoC 不丢失上下文)。

**Baseline**: 本文档 §3(代码事实调研)所有 `internal/...` 文件 file:line 引用基于 **commit `4cd053d`**(等价于代码层 `efa4ff3` — `4cd053d` 仅加 design doc,不改代码)。后续提交可能让具体行号漂移。如要进 PoC,**实施前**先 grep 关键 symbol(`BuildOrLoadGraph` / `Orchestrator.Run` / `BaselineCache` / `FailureTaxonomyStore` 等)对齐行号;若漂移,以 grep 真实结果为准。可用 `git log --oneline efa4ff3..HEAD -- internal/tool/repomap/ internal/orchestrator/ internal/repl/ internal/memory/` 查 baseline 后所有相关代码改动。

---

## 0. 一句话摘要

用户希望未来支持多种 survey 类需求(架构文档 / API reference / 性能瓶颈报告 / 安全审计 / 许可证扫描 / ...)。本讨论锁定首批 **3 个目标 survey**:
- **architecture**(整库架构文档,面向维护者)
- **api_reference**(public API 文档,面向用户)
- **perf_report**(性能瓶颈报告,面向运维 + 优化)

设计约束(用户已同意):
- ✅ Phase 0 ArtifactStore + Dependency 内核作最高优先级
- ✅ DSL 关键,per-kind 一个独立 yaml 文件
- ✅ 增量从 PoC 阶段就内建,不能后期补
- ✅ 支持 Ctrl-C / `/cancel` 优雅中断

剩余 open questions 见 §10。

---

## 1. 为什么 read mode 不能直接复用

现有 read mode 围绕"一个聚焦问题、一个聚焦答案"设计,与"整库 survey"在 4 个根本维度不匹配:

| 维度 | read mode | survey 需求 |
|---|---|---|
| 问题边界 | 单 subject + 1-3 facets | N 个 component × M 维度 |
| 答案 carrier | 单一 `AnswerDocumentV2` | 多章节文档 |
| Pipeline | analyze→explore→extract→finalize 单 pass | "扫描-编目-分章-成文"多 pass |
| Iteration cap | `pipeline_max_steps=50`(单问题预算)| 整库需 1000+ tool calls |
| Question family | 8 family 单选 | 不属于任何 family — meta-question |

**结论**:不能简单"问大问题让 read pipeline 跑"。必须设计独立 survey mode。

---

## 2. 三层架构(本讨论锁定的设计)

```
┌─────────────────────────────────────────────────────────────┐
│ Layer 3 — Templates (per-survey-kind, declarative YAML)    │
│   templates/survey/architecture.yaml                        │
│   templates/survey/api_reference.yaml                       │
│   templates/survey/perf_report.yaml                         │
│   templates/survey/_shared/{glossary,overview}.yaml         │
└──────────────────────────────┬──────────────────────────────┘
                               │
┌──────────────────────────────┴──────────────────────────────┐
│ Layer 2 — Survey Orchestrator (template engine)             │
│   读 template → SurveyPlan DAG → forEach primitive dispatch │
│   GlossaryStore + CrossReferenceStore + ArtifactStore       │
│   多格式 renderer (markdown / OpenAPI / SARIF / SPDX / ...)│
└──────────────────────────────┬──────────────────────────────┘
                               │
┌──────────────────────────────┴──────────────────────────────┐
│ Layer 1 — Primitives (composable building blocks)           │
│   ScanPrimitive          整库 deterministic 扫(repomap)    │
│   ASTExtractPrimitive    public symbol + signature + godoc  │
│   TraceIngestPrimitive   pprof / runtime trace 解析         │
│   FocusedReadPrimitive   spawn 1 次 read pipeline           │
│   AggregatePrimitive     多 artifact + 1 LLM 综合           │
│   AssertPrimitive        rule_set typed check (audit/license)│
│   InjectArtifactPrimitive 外部数据(SBOM/CVE/git-blame)     │
│   RenderPrimitive        artifact → 输出格式                │
└─────────────────────────────────────────────────────────────┘
```

**优雅性来源**:
- 业务-执行解耦:DSL 描述"做什么",orchestrator 决定"怎么做"
- 数据-LLM 解耦:deterministic primitive 是 first-class
- 缓存-逻辑解耦:ArtifactStore + ContentHash 让增量是 ambient capability
- Survey-Read 解耦:FocusedReadPrimitive 把 read mode 当黑盒

**加新 survey kind**:写 yaml + 可能加 1-2 primitive,**不动 orchestrator 也不动 read pipeline**。
**加新输出格式**:加 1 个 RenderPrimitive,template 只声明 `output_format`。

---

## 3. 4 个 Explore agent 调研发现(已交叉验证的代码事实)

本节是后续 session 接续的**关键 ground truth** — 不要重新调研。

### 3.1 repomap(`internal/tool/repomap/`)— ScanPrimitive 复用 85% / ASTExtractPrimitive 复用 70%

- **公开入口**:`repomap.BuildOrLoadGraph(repoRoot, query) (*Graph, error)`(`tool.go:146-148`)
- **Graph 输出**:`{Files, FileIndex, SymbolDefs, SymbolByID, MethodIndex, ImportGraph, ReverseImports}`(`types/types.go:389-402`)
- **Symbol 字段**:`Name / Kind / File / Line / EndLine / Exported / Receiver / Signature / Doc / Parent / Arity / RequiredMethods / Implements / ReturnTypeNames / ID`(`types/types.go:72-153`)
- **支持语言**:13 个原生(Go/Python/JS/TS/Java/Kotlin/Rust/C/C++/Ruby/Swift/Lua/Proto)+ ArkTS(走 TS sitter)+ Cangjie(自定义 parser)
- **增量扫已实现**:缓存 `~/.codrax/cache/repomap/<repo-slug>/`,失效策略 cacheSchemaVersion / extractorVersion / SHA256 / 30%-changed 阈值(`cache.go:51-280`)
- **限制**:
  - `Symbol.Doc` 只取前一兄弟节点 comment **首行**(`parser.go:255-275`)— 多行 godoc 需自己抽
  - `repomap.Graph` 不导出 `*sitter.Node` — perf hotspot 函数 body 需 `os.ReadFile(file)[byteRange]` 切 source
  - 无原生 "scope to package" filter,需调用层遍历 `Graph.Files` 过滤
  - 红线(CLAUDE.md):`extToLang[".ts"] → LangArkTS` 仅当 `oh-package.json5` 在祖先;`.cjo` scanner 拒绝;Cangjie `Package` 必须从 `package_clause`(禁路径推断)

**Reuse 评估**:
- ScanPrimitive: **85%** — 直接 wrap `BuildOrLoadGraph`
- ASTExtractPrimitive: **70%** — Symbol 字段全在,缺多行 godoc + visibility filter
- perf hotspot 源码抽取: **40%** — Symbol.Line/EndLine 定位 + 磁盘读切片

### 3.2 read pipeline 编程式调用(`internal/orchestrator/orchestrator.go`)— FocusedReadPrimitive 复用 95%

- **顶层入口**:`Orchestrator.Run(request, repoRoot, branch) (*BusContext, error)`(`orchestrator.go:1087`)
- **In-process 多次调用安全**:每次 Run 入口防御性重置(`orchestrator.go:1092-1105`):
  ```go
  o.cancelToken = NewCancelToken()
  o.phaseContextPrefix = ""
  o.nextPhaseHint = ""
  o.continuationClassification = nil
  busCtx = 全新分配
  mutableState = 全新分配
  ```
- **AnswerDocumentV2 出口**:`busCtx.Mutable.AnswerDocumentV2()`(`internal/types/context.go:1546`)— 返回 typed struct
- **失败信息**:`busCtx.TaskState.LastError`(string)
- **进度发射**:`Orchestrator.SetEmitter(render.EventEmitter)`(`orchestrator.go:451`)— 已有 `EventAgentReasoning / EventPipelineEnd / EventPhaseProgress` 事件
- **L1 byte-identity 红线**:survey 在外层 wrap 不破红线 — `runReadSchedulerLoop` body 完全不动
- **`pipeline_max_steps`**:**per-Run** 而非 process-global(`*stepsUsed` 在每个 Run 栈帧分配)— survey 100 spawn × 50 step = 5000 step 自然支持
- **pre-stage 自动跳过**:`AttachedLog / AttachedHitrace` 为空时 log_triage / perf_triage 自动跳(`topology.go:66-83`)— survey 不需禁用,设空即可
- **Write mode 已是 spawn 模式**:`phase_scheduler.go:60-225（已于 2026-06-11 随 PlanGroup 通道退役删除;模式参考见 git 历史）` 多次调 `runner(stepsUsed) = o.runTaskPhase()` — survey 直接学这个 pattern
- **唯一缺口**:`SetAmbientKnowledge(glossary, priorChapters)` setter 需新加(~30 LOC) — 让后续章节能 consult 前面章节

### 3.3 现有 store 全景(7 个)— ArtifactStore 学 FailureTaxonomyStore pattern

| Store | atomic write | 并发安全 | TTL/cap | 依赖追踪 |
|---|---|---|---|---|
| BaselineCache | mtime 防护 | 无 lock | LRU by mtime | ❌ |
| **FailureTaxonomyStore** | `.tmp + os.Rename` | sync.Mutex | TTL+score | ❌ |
| AnswerTaxonomyStore | `.tmp + os.Rename` | sync.Mutex | TTL+score | ❌ |
| **PlanStore** | `.tmp + os.Rename` (commit 45) | sync.Mutex | 单 pending 不变量 | ❌(只有 TriggerTurnID 弱链)|
| Memory.Store | atomic + fileLock 多进程 | sync.Mutex + fileLock | compaction | ❌(只有 IndexEntry.Refs)|
| Blob | session 隔离 | 无需 | session count cap | ❌ |

**关键发现**:**没有任何现有 store 实现 typed artifact 依赖图**(provenance + DependsOn DAG)。

**ArtifactStore 设计**:不复用现有 store(语义不匹配),**学 FailureTaxonomyStore + commit-45 PlanStore 的工程模式**(`sync.Mutex + .tmp+rename atomic write + lazy load + per-repo namespace`),新增独特字段:
- `ContentHash`(主键,sha256 of artifact bytes)
- `Provenance{SourcePrimitive, SourcePhase, SourceSpawnID, TemplateVersion, ModelVersion, SurveyKind, RunID}`
- `DependsOn []string`(input artifact ContentHash 列表)
- `OutputHash`(用于上层 cross-chapter 引用追溯)

**Phase 0 工作量**:~700 LOC(类型 + Save/Load/Append/decay + 测试)= **1 session**

### 3.4 集成面(CLI / REPL / config)— 全是 Trivial / Small 改动

| 集成点 | 文件 | 复杂度 |
|---|---|---|
| CLI flag(`--mode=survey --template=...`)| `cmd/root.go:84-98 + 365-400` | Trivial |
| Mode enum | `internal/types/pipeline_mode.go:39` 加 `ModeSurvey` | Small |
| Mode dispatch | `cmd/root.go:806-841 resolveWriteMode` | Small |
| Config 前缀组(`survey_*` 无冲突)| `internal/config/runtime.go` 加 7-8 字段 | Small |
| REPL 命令(`/survey <kind>`)| `internal/repl/repl.go:1962-2125` | Small |
| Per-purpose 模型路由 | `providers.yaml` 加 `survey_aggregate / survey_focused_read` | Trivial |
| 进度可视化 | `internal/render/event.go` 加 `EventSurveyPhaseProgress` + dock case | Small |
| Markdown 渲染 | `internal/render/renderer.go:1502 RenderMarkdown` 直接复用 Glamour | 0 |
| Cancel | `internal/repl/cancel_listener.go` + `Orchestrator.cancelToken` 已就绪 | 0 |
| Path 锚 | `<runtimeAnchor>/surveys/{cache,templates,outputs}/`(对称现有 plans/blob/memory)| 设计选择 |

---

## 4. 三 survey 的本质差异(决定 primitive 集合)

| | architecture | api_reference | perf_report |
|---|---|---|---|
| 主输入 | source | source(public surface)| source + trace + benchmark |
| LLM 占比 | 高(prose 主导)| 中(LLM 写 description,signature 用 AST 抠)| 中(LLM 解释瓶颈,数据用 trace 抠)|
| 输出格式 | markdown | markdown / OpenAPI | markdown / flamegraph_html |
| 真值来源 | 设计意图(LLM 综合)| AST signature(deterministic)| trace 数据(deterministic)|
| 失效信号 | source diff | public-surface AST diff | trace 数据 + source diff |

**关键观察**:只有 architecture 是"LLM prose 主导"。api_reference / perf_report 是"deterministic 真值 + LLM 解释" — primitive 集合里 deterministic 必须是一等公民。

三 survey 的 primitive 组合:
- **architecture**:Scan → FocusedRead(每包) → Aggregate(跨包) → Render(markdown)
- **api_reference**:Scan → ASTExtract(签名+godoc) → FocusedRead(每 exported group 写 description) → Render(markdown / OpenAPI)
- **perf_report**:TraceIngest → ASTExtract(热点函数源码) → FocusedRead(解释瓶颈) → Aggregate(优先级排序) → Render(markdown + flamegraph)

**复用率**:Scan + FocusedRead + Aggregate + Render 三 survey 共用 ≥ 80%。ASTExtract / TraceIngest 是 survey-specific。

---

## 5. DSL 草案(per-kind 独立 yaml)

### 5.1 architecture.yaml

```yaml
name: architecture
output_format: markdown
description: "Whole-codebase architecture documentation for maintainers"

imports:
  - _shared/glossary
  - _shared/overview

chapters:
  - import: glossary
  - import: overview

  - id: pipeline_topology
    title: 核心 Pipeline 拓扑
    primitives:
      - kind: scan
        scope: file_pattern
        pattern: "internal/orchestrator/topology.go"
        as: topology_files
      - kind: focused_read
        question: "Describe each stage's role and the data flow between them"
        consults: [glossary, overview]
        injects: [topology_files]
        as: pipeline_chapter

  - id: per_package
    title: 各包架构
    foreach:
      from: scan.packages
      filter: "internal/*"
      cache_key: "{{ .Path }}"
    primitives:
      - kind: ast_extract
        scope: package
        path: "{{ .Path }}"
        emit: [exported_types, package_doc, top_level_invariants]
        as: pkg_facts_{{ .Name }}
      - kind: focused_read
        question: "Describe package {{ .Path }}: public types, invariants, position in the read pipeline"
        consults: [glossary, overview]
        injects: [pkg_facts_{{ .Name }}]
        as: pkg_chapter_{{ .Name }}

  - id: render
    primitives:
      - kind: render
        format: markdown
        chapters_order: [glossary, overview, pipeline_topology, per_package.*]
```

### 5.2 api_reference.yaml(关键:per-symbol hash 让 internal helper 改不失效)

```yaml
name: api_reference
output_format: markdown
output_alt_formats: [openapi]

imports:
  - _shared/glossary

chapters:
  - import: glossary

  - id: api_inventory
    primitives:
      - kind: scan
        scope: file_pattern
        pattern: "**/*.go"
        as: all_go_files
      - kind: ast_extract
        scope: public_symbols
        emit: [signature, godoc, examples_block]
        group_by: package
        as: exported_inventory

  - id: per_package_api
    foreach:
      from: exported_inventory.packages
      cache_key:
        - "{{ .Path }}"
        - "{{ .ExportedSymbolsContentHash }}"   # ← 灵魂:只 hash exported,internal 改不失效
    primitives:
      - kind: focused_read
        question: "For package {{ .Path }}, write 1-paragraph description per exported symbol explaining when to use it"
        consults: [glossary]
        injects: [{{ .Symbols }}]
        as: api_chapter_{{ .Name }}

  - id: render
    primitives:
      - kind: render
        format: markdown
        from: [glossary, per_package_api.*]
      - kind: render
        format: openapi
        from: [exported_inventory]
        condition: "has_http_handlers == true"
```

### 5.3 perf_report.yaml(需外部 trace 输入)

```yaml
name: perf_report
output_format: markdown
output_alt_formats: [flamegraph_html]

required_inputs:
  - trace_file: "Path to pprof / runtime trace / benchmark output"

imports:
  - _shared/glossary

chapters:
  - import: glossary

  - id: trace_ingestion
    primitives:
      - kind: trace_ingest
        from: "{{ .Inputs.trace_file }}"
        emit: [hot_functions, allocation_hotspots, lock_contention]
        as: trace_metrics

  - id: hotspot_explanations
    foreach:
      from: trace_metrics.hot_functions
      filter: "self_pct >= 5.0"
      cache_key: "{{ .Symbol }}#{{ .SourceHash }}#{{ .TraceHash }}"
    primitives:
      - kind: ast_extract
        scope: function
        symbol: "{{ .Symbol }}"
        as: hot_fn_source_{{ .Symbol }}
      - kind: focused_read
        question: "Why is {{ .Symbol }} expensive? It accounts for {{ .SelfPct }}% of CPU."
        injects: [hot_fn_source_{{ .Symbol }}, trace_metrics]
        consults: [glossary]
        as: hotspot_chapter_{{ .Symbol }}

  - id: priorities
    primitives:
      - kind: aggregate
        question: "Across all hotspots, rank top-5 optimization candidates by impact + feasibility"
        injects: [hotspot_explanations.*]
        as: priority_list

  - id: render
    primitives:
      - kind: render
        format: markdown
        from: [glossary, hotspot_explanations.*, priority_list]
      - kind: render
        format: flamegraph_html
        from: [trace_metrics]
```

### 5.4 \_shared/glossary.yaml + \_shared/overview.yaml

(见之前讨论草案,略)

---

## 6. ArtifactStore 设计(Phase 0 关键)

### 6.1 schema(精确字段)

```go
// 仿 internal/orchestrator/failure_taxonomy_store.go pattern
type ArtifactStore struct {
    mu    sync.Mutex
    dir   string                       // <runtimeAnchor>/surveys/cache/
    items map[string]*ArtifactEntry    // key = ContentHash
}

type ArtifactEntry struct {
    ContentHash    string             // sha256(artifact bytes) — 主键
    Kind           string             // "scan_inventory" / "ast_symbol_extract" / "focused_read_answer" / ...
    Bytes          []byte             // 序列化后的 artifact payload
    Provenance     Provenance
    DependsOn      []string           // 上游 artifact ContentHash 列表
    OutputHash     string
    CreatedAt      time.Time
    LastAccessedAt time.Time          // for LRU
    SizeBytes      int64
}

type Provenance struct {
    SourcePrimitive  string
    PrimitiveParams  string             // 哈希后的参数(决定 cache key)
    TemplateVersion  string
    ModelVersion     string             // claude-opus-4-7
    SurveyKind       string             // "architecture" / "api_reference" / "perf_report"
    RunID            string             // <timestamp>-<pid>
}
```

### 6.2 增量正确性的 5 个 hash 维度(缺一会失效漏判)

每个 primitive 的 cache key 由这 5 个维度组成:

1. **PrimitiveKind + Params**:同 primitive 不同参数走不同 cache
2. **InputArtifactHashes**(sorted):上游变了下游必失效
3. **TemplateVersion**:template yaml 改了全失效(template SHA)
4. **ModelVersion**:`claude-opus-4-7` → `claude-opus-4-8` 时强制重跑
5. **CodebaseFingerprint**:repomap 已经做了 30%-changed 全失效,survey 直接信任 — 不再做整库 hash

### 6.3 Hash 粒度 trick

| Artifact 类型 | hash 粒度 | 失效条件 |
|---|---|---|
| `scan.packages` | sorted package list hash | 加/删 package |
| `ast_extract.exported_inventory` | sorted (symbol, signature, godoc) hash | 改 export signature 或 godoc |
| `ast_extract.function_source` | per-symbol AST hash | 函数体改 |
| `trace_metrics` | trace file hash | trace 重跑 |
| `glossary_store` | sorted term list hash | 加/删/改 term |
| `focused_read.answer` | hash(question + injects.hashes + consults.hashes + model_version) | input 任一改 |

**关键 trick**:`exported_inventory` 用 "exported symbols hash"——unexported helper 改了不让 API doc 失效。这是 api_reference 增量的灵魂。

### 6.4 缓存策略

- TTL 默认 30 天(配 `survey_cache_ttl_days`)
- 总大小 cap 默认 1GB(配 `survey_cache_max_bytes`),LRU 淘汰
- REPL 命令:`/survey cache stats / clear / invalidate <chapter_id>`
- CLI:`--cache=on|off|invalidate-from=§3|ttl=30d`

---

## 7. 修正后的工作量估算(基于代码事实重估)

原估 10 session,基于 4 agent 调研重估为 **6-7 session**:

| 阶段 | 内容 | 修正估 | 主要变化 |
|---|---|---|---|
| **0** | ArtifactStore + ContentHash + Dependency DAG | **1** | 学 FailureTaxonomyStore pattern,~700 LOC |
| **1** | 3 primitive(scan/focused_read/render_md)+ ambient knowledge setter | **1** | 高复用率,主要是 wrapping |
| **2** | DSL parser + foreach + imports + cache_key | **1.5** | 用 yaml.v3,参考 runtime.go pointer pattern |
| **3** | architecture.yaml + \_shared partials,跑通 codrax 自己 | **1** | |
| **4** | ASTExtractPrimitive + per-symbol hash + multi-line godoc | **0.5** | repomap.Symbol 已 90% |
| **5** | api_reference.yaml | **0.5** | 验证 DSL 扩展性 |
| **6** | TraceIngestPrimitive + perf_report.yaml | **1** | trace 解析 + source slice helper |
| **7** | AggregatePrimitive 优化 + cross-chapter consults | **0.5** | |
| **(后期)** | OpenAPI / flamegraph_html renderer | 1 each | 行业标准 renderer 独立 ship |

**总计**:**6-7 session** 出三 survey markdown 输出。

---

## 8. 新增代码量估算

| 包 / 文件 | LOC | 内容 |
|---|---|---|
| `internal/survey/artifactstore/` | ~700 | ArtifactStore 内核 + 测试 |
| `internal/survey/primitive/` | ~800 | 6 primitive 接口 + 实现 |
| `internal/survey/dsl/` | ~600 | YAML parser + DAG builder + foreach/imports/cache_key |
| `internal/survey/orchestrator/` | ~500 | Survey orchestrator 主循环 + cancel 处理 + 进度发射 |
| `templates/survey/*.yaml` | ~400 | 3 template + 2 _shared partial |
| `internal/orchestrator/orchestrator.go` | +30 | `SetAmbientKnowledge(glossary, chapters)` setter |
| `cmd/root.go` | +60 | `--mode=survey --template=...` flag + dispatch |
| `internal/repl/repl.go` | +80 | `/survey <kind>` 命令 + 状态字段 |
| `internal/config/runtime.go` | +30 | `survey_*` 配置组 |
| `internal/types/pipeline_mode.go` | +5 | `ModeSurvey` enum |
| `internal/render/event.go` | +40 | `EventSurveyPhaseProgress` event |
| `providers.yaml.example` | +10 | survey_aggregate / survey_focused_read 示例 |

**新增总计**:~3.3K LOC。其中 ~2.6K 在新建 `internal/survey/` 包内,对现有 codrax 触动 ~250 LOC,**完全不动 read/write pipeline 内核**。

---

## 9. Phase 0 + Phase 1 PoC 清单(2 session 出可用结果)

**Phase 0**(1 session):
1. `internal/survey/artifactstore/store.go + _test.go`(~700 LOC,学 FailureTaxonomyStore pattern)
2. `internal/types/pipeline_mode.go` 加 `ModeSurvey`(~5 LOC)
3. `internal/config/runtime.go` 加 `Survey*` 字段(~30 LOC)
4. `cmd/root.go` 加 `--mode=survey` flag + dispatch(~60 LOC)
5. 测试:ArtifactStore 单元测试 + multi-process safety smoke

**Phase 1**(1 session):
6. `internal/survey/primitive/scan.go`(wraps `repomap.BuildOrLoadGraph`,~150 LOC)
7. `internal/survey/primitive/focused_read.go`(wraps `Orchestrator.Run` + ambient knowledge,~200 LOC)
8. `internal/survey/primitive/render_md.go`(简单 markdown concat,~100 LOC)
9. `internal/orchestrator/orchestrator.go` 加 `SetAmbientKnowledge` setter(~30 LOC)
10. **Hardcode** 一个最小 architecture survey logic(不走 DSL,只是 Go 代码)— 跑通 codrax 自己产 ARCHITECTURE.md
11. 验证:`codrax --mode=survey --template=architecture --output=/tmp/arch.md` 跑通

**Phase 2 才引入 DSL parser**(后续 session):验证 PoC 可用后,把 hardcode 抽成 DSL,代价是 ~600 LOC + foreach/imports/cache_key 控制结构。

---

## 10. Open questions(等用户决策才能升级为 actionable design)

1. **Phase 0 + Phase 1 PoC 是否同意 2 session 路径?** Phase 1 hardcode 不走 DSL,只是为验证 primitive 接口设计正确。
2. **survey artifact cache 路径锚 cwd 还是 home?** 锚 cwd(`<runtimeAnchor>/surveys/cache/`)与 plans/blob/memory 对称;锚 home(`~/.codrax/cache/surveys/`)与 repomap 对称且支持多 repo 共享。讨论倾向:**锚 cwd**(对称性更重要,survey 输出也是 per-repo 概念)。
3. **DSL primitive output schema 是否从 PoC 阶段就强约束?** 推荐**强约束**(代价小,后期不重构)。Phase 1 hardcode 时 primitive output 是 Go struct(自然 typed),Phase 2 接 DSL 自然。
4. **Multi-instance ArtifactStore 是否做 fileLock?** 短期推荐"两进程独立 cache"接受冗余;长期再加 fileLock(学 Memory.Store pattern)。

---

## 11. 已消除的风险(基于 4 agent 调研)

- ~~"in-process spawn 多次 read pipeline 是否安全"~~ — 验证安全(`orchestrator.go:1092-1105` 防御性重置)
- ~~"L1 byte-identity 红线被破"~~ — survey 在外层 wrap,不动 `runReadSchedulerLoop` body
- ~~"pipeline_max_steps 是 process-global 卡死整 survey"~~ — 是 per-Run,无问题
- ~~"整库 hash 计算成本"~~ — repomap 已实现增量
- ~~"现有 store 不足以支撑增量"~~ — 学 FailureTaxonomyStore pattern + 加 provenance/DAG 字段即可

---

## 12. 新发现的风险(需 PoC 验证)

- **Symbol.Doc 仅首行**(`parser.go:255-275`):api_reference 抽多行 godoc 需 ASTExtractPrimitive 自跑 sitter,~150 LOC
- **repomap Graph 不导出 sitter Node**:perf_report hotspot 函数 body 用 `os.ReadFile + byteRange` 切 source(实践中 LLM 看 source 反而更自然)
- **repomap 缓存路径在 `~/.codrax`,survey 在 `<CWD>/.codrax`** — 路径锚不一致(见 §10 question 2)
- **FocusedReadPrimitive 平均 step 用量未知**:估 30-40 step/章节,`pipeline_max_steps=50` 默认可能不够,survey 可能需要 `--pipeline-max-steps 100` 默认
- **质量验证 metric**:survey 输出无 ground truth,提议跑 K 章节 reviewer LLM 抽查,实际 noise 多大未知
- **DSL foreach cardinality**:大 monorepo(数百 package)foreach 起几百章节会爆,需 `foreach.filter` 和 `foreach.max` 控制

---

## 13. 后续 session 接续指南

读完本文档后:
1. 读 `MEMORY.md` 顶部确认主线状态(Phase A finalizer rule bisection 是当前主线,survey 是未授权方向)
2. 读本文档 §3(代码事实)+ §10(open questions)— 是接续讨论的最小上下文
3. 不要重新调研 §3 已验证的事实 — agent 报告已交叉确认
4. 与用户确认 §10 中 4 个 open questions 任一项答案后,可升级本文档为 actionable design `docs/design/survey_mode.md`
5. 升级时 split:Phase 0 + Phase 1 PoC 写一份独立 design;DSL parser + 三 survey 写另一份(类似 finalizer Phase A / Phase C 拆分)

---

## 14. 与 codrax 主线的优先级关系

当前主线:
- 🔴 finalizer Phase A(rule bisection)— 已落 design,等用户授权 ship
- 🔴 finalizer Phase C(SHAPE_CONTRACT)— 已落 design,gated on Phase A real-eval
- 🟡 write mode 收口
- 🟡 MCP 集成

Survey mode **目前不应抢主线优先级**。本讨论的产出是"未来要做时,一份不丢失上下文的接续 doc"。
