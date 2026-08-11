# Codrax 批量 Trace 独立分析与根因聚类设计

> **状态**：Proposed，尚未实现  
> **代码基线**：`main@7fd42889d3d9e48e79b9524b2c7d28215dfdcee5`  
> **基线日期**：2026-08-05  
> **目标规模**：100～1000 个独立 Trace 分析单元  
> **核心目标**：保留每个 Trace 的独立证据边界，在不混合原始时间轴的前提下，将单次分析结论结构化、归一化、聚类，并生成可追溯的批次报告。

---

## 1. 结论先行

最优方案不是在现有 Finalizer 中直接完成上百个 Trace 的聚类，也不是只增加一个“聚类 Agent”便宣告大功告成，而是采用以下分层方案：

```text
每个 Trace 独立 Read Run
        │
        ├─ 确定性 Trace 证据、投影、排名席位
        │
        └─ Finalizer 一次事务提交：
              AnswerDocumentV2
              TraceFindingV1
                       │
                       ▼
              TraceBatchWorkflow
        ┌──────────────┼────────────────┐
        │              │                │
     持久化         确定性归一化      精确预聚类
        │                               │
        │                         模糊连通分量
        │                               │
        │                   RootCauseClustererAgent
        │                      （可选、受限）
        └──────────────┬────────────────┘
                       ▼
          TraceRootCauseClusterSetV1
                       │
              确定性表格与统计
                       │
              现有 Finalizer 生成摘要
                       ▼
                  批次报告
```

最终架构裁定如下：

| 问题 | 裁定 |
|---|---|
| 是否在 Finalizer 增加 typed 能力 | **是，但只负责单 Trace 的 `TraceFindingV1`，且必须和 `AnswerDocumentV2` 原子提交** |
| 是否让 Finalizer 直接读取上百份结果并聚类 | **否** |
| 是否增加新 Agent | **可增加 `RootCauseClustererAgent`，但只处理确定性规则无法裁决的小型模糊分组** |
| 是否需要新流程 | **需要，核心是新的 `TraceBatchWorkflow`** |
| 是否新增 `BatchFinalizerAgent` | **不需要，批次报告继续复用现有 Finalizer** |
| 是否新增一套根因分类词表 | **不需要，优先复用现有 causal token registry** |
| 是否修改现有 Read 主流水线拓扑 | **第一阶段和第二阶段均不需要** |

一句话概括：

> **Finalizer 把一份 Trace 的结论“钉成结构”；Batch Workflow 把上百份结构“组织成系统”；Clusterer Agent 只裁决模糊边界；最终 Finalizer 只负责把已经算清楚的结果讲清楚。**

---

## 2. 代码锚点规则

本文使用以下锚点格式：

```text
文件路径 — 符号名
```

每个文件链接都固定到代码基线 SHA，不依赖容易漂移的行号。实施时优先搜索符号名；若后续提交重命名，以 Git 历史追踪该符号。

---

## 3. 当前代码事实与架构约束

### 3.1 Finalizer 是终止性答案阶段，不是批次计算阶段

当前 Read 主流程的阶段职责由 [`internal/types/stage_binding.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/stage_binding.go) 集中声明：

- `StageBinding`
- `builtinStageBindings`
- `ReadModeMainStageBindings`
- `StageBindingForStage`

其中 `StageFinalize`：

- 绑定 `AgentFinalizer`；
- `Terminal: true`；
- 职责是从 `AnswerContract`、support plan 和 grounded evidence 渲染 `AnswerDocumentV2`；
- 主要产物是 `AnswerDocumentV2`、`FinalAnswer`、`Citations` 和契约校验结果。

主拓扑仍由 [`internal/orchestrator/topology.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/orchestrator/topology.go) — `pipelineTopology` 控制。  
阶段调度入口见 [`internal/orchestrator/stage_runner.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/orchestrator/stage_runner.go)：

- `StageExecutionRequest`
- `executeStageRequest`
- `newFinalizeStageExecutionRequest`

Finalizer 自身见 [`internal/agent/finalizer.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/agent/finalizer.go) — `NewFinalizerAgent`。代码明确写明：Finalizer 只有一个 `answerDocumentEvaluator`，通过 `emit_answer_document` 产出 `AnswerDocumentV2`。

**设计含义**：跨 Trace 聚类如果塞入 Finalizer，会把终止性展示阶段扩成批处理调度、持久化、归一化、聚类、完整性校验和报告生成的混合体，直接破坏现有阶段边界。

### 3.2 Finalizer 当前确实拥有“诊断结论”的语义所有权

关键锚点是 [`internal/agent/answer_document_trace_decision_handoff.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/agent/answer_document_trace_decision_handoff.go) — `renderAnswerDocTraceDecisionHandoff`。

文件注释明确区分：

```text
系统拥有：
- typed measurements
- rank seats
- wakeup paths
- causal ceiling

模型拥有：
- diagnosis
- prioritization
- user-facing conclusions
```

因此，现阶段若要获得一份可信的“单 Trace typed 结论”，最自然的落点仍是 Finalizer，而不是突然把最终诊断权转移给 Extractor。

Extractor 的当前职责见 [`internal/agent/extractor.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/agent/extractor.go) — `NewExtractorAgent`：它消费冻结后的 Turn A 调查结果，输出 answer symbols、hypothesis verdicts 和 structured support，不重新读文件，也不是当前诊断结论的最终所有者。

**设计含义**：

- 单 Trace 的 `TraceFindingV1` 应由 Finalizer 选择并提交；
- 但候选集合、数值、因果上限必须由程序提前编译；
- Finalizer 只能在给定候选中选择主因、贡献因子或“证据不足”，不能自由创造新根因。

### 3.3 `AnswerDocumentV2` 是展示载体，不应变成批次数据库

答案载体见：

- [`internal/types/answer_document_v2.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/answer_document_v2.go) — `AnswerDocumentV2`
- [`internal/types/answer_document.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/answer_document.go) — 通用答案块、引用和展示类型
- [`internal/agent/answer_document_render_export.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/agent/answer_document_render_export.go) — `RenderAnswerDocumentWithLastMileSupplements`

现有设计强调：renderer 和审计层读取已经持久化的 typed 结构，不应在渲染时偷偷创造新事实。

因此以下字段不应继续塞进 `AnswerDocumentV2`：

```go
ClusterMembers      []string
BatchProgress       ...
FailedTraceIDs      ...
ReclusterGeneration ...
ClusterStorePath    ...
```

正确关系是：

```text
TraceFindingV1 / TraceRootCauseClusterSetV1   // 分析真值
                    │
                    ▼
             AnswerDocumentV2                 // 展示载体
```

而不是让 `AnswerDocumentV2` 同时扮演报告、数据库、批次状态和聚类结果。

### 3.4 当前多 Trace 投影只解决“小规模不混树”，不是批量聚类

[`internal/types/trace_causal_projection_partition.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/trace_causal_projection_partition.go) 的关键锚点：

- `traceCausalProjectionMaxArtifactPartitions = 4`
- `TraceCausalProjectionSet`
- `CompileTraceCausalProjectionSet`
- `TraceCausalProjectionSetFromObservationRecords`

这段代码的目的，是按 typed artifact identity 将不同物理 Trace 分区，避免两个 Trace 的观测被编译进同一棵因果树。它最多保留 4 个 artifact partition，超出部分作为 caveat 暴露。

因此不能简单把上限从 4 改成 100：

```go
const traceCausalProjectionMaxArtifactPartitions = 100
```

那只会得到一个巨大的 Finalizer 上下文，并不会自动获得：

- 稳定的 Finding ID；
- 批次断点恢复；
- 增量更新；
- 根因指纹；
- 成员守恒；
- 聚类版本；
- 模糊边界裁决；
- 失败样本披露。

### 3.5 不同物理 Trace 的原始时间轴没有天然可比关系

[`internal/types/runtime_artifact_pair_relation_authority.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/runtime_artifact_pair_relation_authority.go) — `BuildRuntimeArtifactPairRelationAuthority` 明确把以下跨 artifact 关系标记为 `unproven`：

- shared clock origin；
- direct time alignment；
- shared device；
- shared capture session。

即便两个 Trace 的 `TimeDomain` 标签相同，或各自在本地使用 identity alignment，也不能证明二者共享同一物理时钟。

**设计红线**：

> 批量系统只能聚合“每个 Trace 已经独立收敛出的语义结论”，不能把多个 Trace 的原始时间戳、线程 ID 或因果边放进同一时间轴重新推理。

### 3.6 当前已有可复用的 typed 根因词表

[`internal/tracequery/causal_token_registry.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/tracequery/causal_token_registry.go) 是现有 causal token 的单一真值源，关键符号包括：

- `CausalTokenLane`
- `CausalTokenAdditivity`
- `CausalTokenSubjectKind`
- `CausalTokenSpec`
- `CausalTokenSpecFor`
- `CausalTokenUniverse`
- `assertCausalTokenRow`

当前语义 lane 已覆盖：

```text
scheduling_demand
compute_delivery
wakeup_chain
io_blocking
irq_aggregate
lock_contention
cpu_work
memory_pressure
diagnostic
```

并且同时约束：

- 数值能否相加；
- subject 是 per-thread、aggregate-only 还是 either；
- token 能否作为 root-cause row；
- 中文展示标签；
- 修复方向。

修复方向及跨方向守恒逻辑见 [`internal/tracequery/rank_direction_axiom.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/tracequery/rank_direction_axiom.go)，关键锚点包括：

- `CausalTokenFixDirectionFor`
- `stampRootCauseFixDirections`
- `rootCauseItemDirectionPopulationEligible`

**设计含义**：批量聚类不应再发明一套与现有 token 平行的 `RootCauseFamily`。新的 typed finding 应保存现有 token 的快照，并在其上增加“线程角色、因果形态、业务阶段”等聚类维度。

### 3.7 当前已有可靠的单 Trace 排名 authority

[`internal/types/trace_rank_roster_authority.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/trace_rank_roster_authority.go) 的关键锚点：

- `TraceRankRosterSeat`
- `TraceRankRosterAuthority`
- `BuildTraceRankRosterAuthorities`
- `traceRankRosterCompleteness`

每个 seat 已包含：

```text
Rank
Tier
Type
Subject
EffectiveImpactMS
EffectiveImpactPublished
ChainRelevance
FixDirection
EvidenceID
```

并且 roster 构造只消费 typed projection，不读取用户请求或模型散文。

这正适合成为 `TraceFindingV1` 的确定性候选来源，而不是让模型从最终 Markdown 中重新猜根因。

### 3.8 不同数值口径不能在聚类报告中直接相加

causal token registry 已区分：

```text
wall_clock_per_thread
cross_thread_cpu_ms
count
```

[`internal/tracequery/rank_item_wire_m18.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/tracequery/rank_item_wire_m18.go) 还明确把部分复合分数从 `*_ms` wire key 改成 `*_score`，防止复合指标伪装成毫秒。

**设计含义**：

- 聚类可以统计“样本数”和“出现比例”；
- 数值分布必须按 `Additivity + Unit + Caliber` 分桶；
- 不允许把 wall-clock、cpu·ms、count 和 composite score 混成一个“总影响时长”。

### 3.9 当前 Run 状态是单次运行局部状态

[`internal/types/context.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/context.go) — `MutableState` 是单次 Run 的可变状态；  
[`internal/orchestrator/orchestrator.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/orchestrator/orchestrator.go) — `Orchestrator` 持有一个 `busCtx`，并驱动硬编码的四阶段流水线。

运行期 artifact 谱系由 [`internal/types/node_artifact_ledger.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/node_artifact_ledger.go) 记录：

- `RuntimeArtifactKind`
- `RuntimeArtifactRef`
- `NodeArtifactRecord`
- `NodeArtifactLedger`

代码明确说明：ledger 只存 artifact reference，不存原始 payload、渲染答案、prompt 或模型 rationale；同时它是 run-local 的，放在 `AnalysisIR` 之外，以免把声明态和运行态混在一起。

**设计含义**：

- 一个 Trace 子任务的 finding 可以进入该 Run 的 `MutableState`；
- 整个批次的进度、成员关系和聚类代次不能塞进某个子 Run 的 `BusContext`；
- 批次必须有独立持久化状态机。

### 3.10 现有 Read Snapshot 可复用为子 Run 断点恢复

[`internal/types/read_run_snapshot.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/read_run_snapshot.go) 的关键锚点：

- `ReadRunSnapshotSchemaVersion`
- `ReadRunSnapshot`
- `ReadRunSnapshotFromBusContext`
- `WriteReadRunSnapshotToFile`
- `LoadReadRunSnapshotFromFile`

它只记录 durable typed artifacts，不允许调度器读取渲染散文、模型 rationale 或 prompt 作为恢复真值。

恢复入口见 [`internal/orchestrator/read_run_snapshot_resume.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/orchestrator/read_run_snapshot_resume.go)：

- `SetReadRunSnapshotSeed`
- `SetReadRunSnapshotAutoSeed`
- `applyReadRunSnapshotSeed`
- `validateReadRunSnapshotFingerprints`

确定性 replay 见 [`internal/types/read_run_replay.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/read_run_replay.go)。

**设计含义**：批次系统不需要重新发明单 Trace 内部的恢复机制；它只需要为每个 analysis unit 保存和装载现有 `ReadRunSnapshot`。

### 3.11 `tracediag v2` 可作为确定性预检/采集原语

[`internal/tracediag/run_v2.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/tracediag/run_v2.go) 要求一个明确物理 Trace，锁定 source version，并完整执行独立步骤；CLI 入口见 [`cmd/tracediag.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/cmd/tracediag.go)。

它适合复用为：

- Trace 可读性预检；
- 基础 target/window discovery；
- 确定性 profile 产物；
- 批次失败分类。

但它是 zero-LLM 的确定性诊断入口，不能代替单 Trace 的语义结论，也不能代替跨样本聚类。

---

## 4. 方案选择

### 4.1 方案 A：把所有逻辑塞进 Finalizer

```text
100 个 Trace 结果
      ↓
一个 Finalizer prompt
      ↓
模型完成归一化、聚类、统计和报告
```

主要问题：

1. `TraceCausalProjectionSet` 当前只面向最多 4 个 artifact；
2. Finalizer 是 terminal render stage，没有批次 checkpoint；
3. 上百份自然语言会导致同义词拆群、近义词错合；
4. 模型上下文会膨胀；
5. 无法证明成员守恒；
6. 输入顺序和模型版本会改变分组；
7. 失败一个 Trace 可能污染整个最终输出；
8. Finalizer 重试时可能重写聚类成员。

**结论：否决。**

### 4.2 方案 B：只增加一个 Clusterer Agent

```text
100 份结果
    ↓
Clusterer Agent
    ↓
聚类
```

比方案 A 好，但仍缺少：

- 批次调度；
- 子 Run 隔离；
- 持久化；
- 失败重试；
- content hash；
- 精确预聚类；
- 成员守恒；
- 增量重算；
- 统计口径校验。

Agent 只是语义裁判，不是工作流引擎。

**结论：不完整。**

### 4.3 方案 C：Batch Workflow + typed finding + 确定性聚类 + 受限 Agent

这是推荐方案：

```text
typed finding 解决“每份结论是什么”
batch workflow 解决“上百份结论怎么跑、怎么收齐”
deterministic cluster 解决“明显相同和明显不同”
clusterer agent 解决“模糊语义边界”
finalizer 解决“怎么向用户表达”
```

**结论：采用。**

---

## 5. 总体架构

### 5.1 批次中的最小单位不是“文件”，而是 Analysis Unit

同一个物理 Trace 中可能存在多个卡顿帧、多个进程或多个目标窗口。因此批次最小单位应定义为：

```go
type TraceAnalysisUnitRef struct {
    UnitID              string                 `json:"unit_id"`
    ArtifactContentHash string                 `json:"artifact_content_hash"`
    ArtifactPath        string                 `json:"artifact_path,omitempty"`
    TargetSelector      TraceTargetSelector    `json:"target_selector"`
    WindowSelector      TraceWindowSelector    `json:"window_selector"`
}
```

如果一个 Trace 文件需要分析三个卡顿帧，应创建三个 unit：

```text
trace-A / frame-100
trace-A / frame-205
trace-A / frame-311
```

而不是让一份 finding 同时携带三个互相独立的主因。

### 5.2 主流程

```text
TraceBatchManifestV1
        │
        ▼
discover / hash / deduplicate
        │
        ▼
有界 worker pool
        │
        ├── Unit 001 → 独立 Orchestrator + BusContext
        ├── Unit 002 → 独立 Orchestrator + BusContext
        ├── Unit 003 → 独立 Orchestrator + BusContext
        └── ...
                    │
                    ▼
      FinalAnswerArtifactsV1 原子提交
        ├── AnswerDocumentV2
        └── TraceFindingV1
                    │
                    ▼
        validate / persist / checkpoint
                    │
                    ▼
    deterministic normalize + fingerprint
                    │
                    ▼
          exact cluster + constraints
                    │
              ┌─────┴─────┐
              │           │
           已确定       模糊分量
                          │
               RootCauseClustererAgent
                          │
              deterministic reducer
                          │
                          ▼
             TraceRootCauseClusterSetV1
                          │
           deterministic tables / appendix
                          │
                 existing Finalizer
                          │
                          ▼
                    report.md
```

### 5.3 并发隔离

不能让一个 `Orchestrator` 实例被多个 worker 并发调用，因为它持有：

```text
busCtx
attachedHitrace
attachedLog
per-run directives
snapshot seed
cancel token
```

推荐增加工厂：

```go
type TraceChildRunFactory interface {
    NewRunner(unit TraceAnalysisUnitRef, contract TraceFindingContract) (*orchestrator.Orchestrator, error)
}
```

每个并发 worker 为一个 unit 创建独立的 Orchestrator、BusContext、MutableState 和 snapshot 路径。可以共享只读配置、模型 provider、只读缓存，但不能共享 run-local 状态。

---

## 6. 单 Trace typed 结论设计

### 6.1 不直接从 Markdown 中抽取

禁止以下路径：

```text
Final Markdown
    ↓
正则或第二次 LLM 抽取
    ↓
TraceFinding
```

原因是：

- 同一个根因可能写成“同步 Binder 等待”“IPC 阻塞”“服务端完成依赖”；
- Markdown 是展示结果，不是稳定协议；
- Finalizer patch 可能只改一段文字；
- 文本抽取无法证明 evidence ID、rank seat 和 causal ceiling；
- 格式变化会让历史 finding 失效。

### 6.2 先编译确定性候选集合

将当前 `renderAnswerDocTraceDecisionHandoff` 拆成两层：

```go
func CompileTraceDecisionCandidateSet(
    ctx *types.AgentContext,
) (types.TraceDecisionCandidateSetV1, error)

func RenderTraceDecisionCandidateSet(
    set types.TraceDecisionCandidateSetV1,
) string
```

现有 prompt 文本继续由第二个函数生成，尽量保持普通 Read 模式输出兼容；新的 finding validator 与 prompt 共同消费第一层 typed candidate set。

建议类型：

```go
type TraceDecisionCandidateSetV1 struct {
    SchemaVersion       int                         `json:"schema_version"`
    Artifact            TraceFindingArtifact        `json:"artifact"`
    Scope               TraceFindingScope           `json:"scope"`
    CausalCeiling       TraceCausalCeiling          `json:"causal_ceiling"`
    Symptom             TraceSymptomSummary         `json:"symptom"`

    PrimaryEligible     []TraceCauseCandidate       `json:"primary_eligible"`
    ContributorEligible []TraceCauseCandidate       `json:"contributor_eligible"`
    ContextOnly         []TraceCauseCandidate       `json:"context_only,omitempty"`
    EvidenceBoundaries  []TraceEvidenceBoundary     `json:"evidence_boundaries,omitempty"`

    AcceptedEvidenceIDs []string                    `json:"accepted_evidence_ids"`
}
```

候选编译输入优先使用：

```text
ObservationLedger
TraceCausalProjectionSet
TraceRankRosterAuthority
typed relation authorities
hypothesis verdicts
causal token registry
```

候选编译器负责：

- 固定候选 ID；
- 绑定 evidence ID；
- 固定 token、rank、tier、phase、fix direction；
- 区分 primary-eligible、contributor-eligible 和 context-only；
- 固定 causal ceiling；
- 排除只表示数据边界、不能作为正向主因的记录；
- 不读取 Finalizer 散文。

### 6.3 `TraceFindingV1`

```go
const TraceFindingSchemaVersion = 1

type TraceFindingV1 struct {
    SchemaVersion       int                       `json:"schema_version"`
    FindingID           string                    `json:"finding_id"`
    AnalysisKey         string                    `json:"analysis_key"`

    Artifact            TraceFindingArtifact      `json:"artifact"`
    Scope               TraceFindingScope         `json:"scope"`
    Revision            TraceFindingRevision      `json:"revision"`

    Symptom             TraceSymptomSummary       `json:"symptom"`

    PrimaryCause        *TraceCauseDecision       `json:"primary_cause,omitempty"`
    Contributors        []TraceCauseDecision      `json:"contributors,omitempty"`
    Unresolved          *TraceUnresolvedDecision  `json:"unresolved,omitempty"`

    EvidenceRefs        []string                  `json:"evidence_refs"`
    CounterEvidenceRefs []string                  `json:"counter_evidence_refs,omitempty"`
    Coverage            TraceFindingCoverage      `json:"coverage"`
}
```

`TraceFindingV1` 不直接包含 `BatchID`。它应是可复用、内容寻址的单 unit 结论；批次通过 `TraceBatchItemState.FindingRef` 引用它。否则同一 Trace 被两个批次复用时，会因为 BatchID 不同产生两份内容等价的 finding。

### 6.4 根因决策对象

```go
type TraceCauseDecision struct {
    CandidateID       string                    `json:"candidate_id"`
    Status            TraceCausalStatus         `json:"status"`
    Token             TraceCausalTokenSnapshot  `json:"token"`

    SubjectRole       string                    `json:"subject_role"`
    UpstreamRole      string                    `json:"upstream_role,omitempty"`
    CausalShape       string                    `json:"causal_shape"`
    Phase             string                    `json:"phase"`

    Rank              int                       `json:"rank,omitempty"`
    Tier              string                    `json:"tier,omitempty"`
    BoardFingerprint  string                    `json:"board_fingerprint,omitempty"`

    Magnitude         *TypedMagnitude           `json:"magnitude,omitempty"`
    EvidenceRefs      []string                  `json:"evidence_refs"`
    Confidence        TraceConfidence           `json:"confidence"`
}
```

推荐状态：

```go
type TraceCausalStatus string

const (
    TraceCausalProven             TraceCausalStatus = "proven"
    TraceCausalSupportedCandidate TraceCausalStatus = "supported_candidate"
    TraceCausalUnresolved         TraceCausalStatus = "unresolved"
)
```

`proven` 只能在 typed causal carrier 明确允许时使用。当前 handoff 已经大量强调 `causal_conclusion=unproven`、`missing_wakeup` 只是证据边界、`pre_wakeup_dependency` 不能冒充 post-wakeup 抢占，因此 validator 必须延续这些边界。

### 6.5 causal token 使用快照，不在 `types` 中反向依赖 `tracequery`

`internal/tracequery` 很可能依赖 `internal/types`；若 `types.TraceFindingV1` 直接使用 `tracequery.CausalTokenLane`，会制造包循环。

推荐保存 registry 快照：

```go
type TraceCausalTokenSnapshot struct {
    Token           string `json:"token"`
    Lane            string `json:"lane"`
    Additivity      string `json:"additivity"`
    SubjectKind     string `json:"subject_kind"`
    FixDirection    string `json:"fix_direction,omitempty"`
    RegistryHash    string `json:"registry_hash"`
}
```

校验逻辑放在：

```text
internal/analysis/tracefinding
```

该包可以同时导入 `types` 和 `tracequery`，调用：

```go
tracequery.CausalTokenSpecFor(token)
tracequery.CausalTokenUniverse()
tracequery.CausalTokenFixDirectionFor(token)
```

这样：

- `types` 保持依赖轻量；
- registry 仍然只有一个真值源；
- finding 保存的是当次分析时的不可变语义快照；
- 后续 registry 演进不会无声改写历史 finding。

### 6.6 数值必须带完整口径

```go
type TypedMagnitude struct {
    Value          float64 `json:"value"`
    Unit           string  `json:"unit"`
    Additivity     string  `json:"additivity"`
    Caliber        string  `json:"caliber"`
    WindowDuration float64 `json:"window_duration_ms,omitempty"`
}
```

聚类统计规则：

```text
同一个 cluster 可以包含不同 caliber 的成员；
不同 caliber 只能分别计算分布；
只有样本计数可以跨 caliber 汇总；
物理数值不得跨口径相加。
```

### 6.7 Analysis Key

不建议把完整 Git commit 直接作为缓存失效键，因为一次只改文档或 `repo_map` 的提交也会让上百个 Trace 全部重跑。

建议：

```text
AnalysisKey =
  SHA256(
    artifact_content_hash
    + scope_selector_hash
    + analysis_profile_hash
    + trace_analysis_contract_hash
    + model_route_hash
  )
```

其中 `trace_analysis_contract_hash` 由以下版本组成：

```text
TraceFinding schema
candidate compiler version
trace_query output contract
causal token registry hash
finalizer typed schema
analysis prompt/skill contract
normalization version
```

同时在 `TraceFindingRevision` 中记录完整 Codrax commit，供审计使用，但默认不把无关提交作为强制失效条件。

---

## 7. Finalizer typed 输出的事务设计

### 7.1 为什么不能增加一个独立 `emit_trace_finding` 后各写各的

当前答案工具链见：

- [`internal/tool/emit_answer_document.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/tool/emit_answer_document.go) — `EmitAnswerDocument`
- [`internal/tool/emit_answer_document_v2.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/tool/emit_answer_document_v2.go) — `emitAnswerDocumentV2Params`、`executeAnswerDocumentV2`
- [`internal/tool/emit_answer_document_patch.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/tool/emit_answer_document_patch.go) — `EmitAnswerDocumentPatch`、`emitAnswerDocumentPatchParams`
- [`internal/types/answer_document_v2_mutation.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/answer_document_v2_mutation.go) — `AnswerDocumentMutation`

Patch 工具存在的原因，就是模型在重试时无法可靠保持未修改字段；系统必须在协议层保证继承。

若新增两个独立工具：

```text
emit_trace_finding
emit_answer_document
```

会出现：

```text
finding 成功、document 失败
document patch 成功、finding 没同步
retry 改了主因文字、finding 仍指向旧主因
进程中断时只写入一半
```

因此必须使用一个事务边界。

### 7.2 推荐：扩展现有工具协议，但不扩展 `AnswerDocumentV2`

新增独立 envelope：

```go
type FinalAnswerArtifactsV1 struct {
    Document     AnswerDocumentV2 `json:"document"`
    TraceFinding *TraceFindingV1  `json:"trace_finding,omitempty"`
}

type FinalAnswerArtifactsMutation struct {
    Document     AnswerDocumentMutation
    TraceFinding TraceFindingMutation
}
```

实现策略：

1. `emit_answer_document` 的 JSON schema 在 trace finding contract 激活时，增加顶层 sibling：

   ```json
   {
     "blocks": [],
     "citations": [],
     "trace_finding": {}
   }
   ```

2. 工具内部将 `blocks/citations/...` 转成 `AnswerDocumentV2`，将 `trace_finding` 转成 `TraceFindingV1`；
3. 先 dry-run document mutation；
4. 用同一份 `TraceDecisionCandidateSetV1` 校验 finding；
5. 做 document/finding 一致性检查；
6. 两者全部通过后，一次写入 `MutableState`；
7. `AnswerDocumentV2` 的 JSON 结构本身不增加 cluster 或 batch 字段。

### 7.3 动态 schema 投影

现有 [`internal/tool/answer_document_dynamic_schema.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/tool/answer_document_dynamic_schema.go) — `BuildAnswerDocumentParametersFor` 会根据 `AnswerSemanticView` 裁剪 Finalizer 能看到的字段。

建议在系统侧增加：

```go
type TraceFindingContract struct {
    Required             bool
    CandidateSetID       string
    FindingSchemaVersion int
}
```

并将其作为系统注入的运行期 contract，而不是让模型决定是否需要 typed finding。

投影规则：

```text
普通代码问答：
  schema 完全不出现 trace_finding

普通单 Trace 问答、未开启 shadow：
  可选或不出现

TraceBatch child run：
  trace_finding 必填
  CandidateID 必须来自 candidate set
```

这样不会让批次功能污染所有 Finalizer 请求。

### 7.4 Patch 语义

为 `emit_answer_document_patch` 增加：

```go
ReplaceTraceFinding *TraceFindingV1 `json:"replace_trace_finding,omitempty"`
```

规则：

```text
1. patch 未改变诊断承载块：
   自动继承 previous TraceFindingV1。

2. patch 改变主因/贡献因子相关 decision block：
   必须同时提供 replace_trace_finding；
   否则拒绝 trace_finding_stale。

3. patch 只修引用、表格或 caveat：
   可以继承 finding。

4. previous finding 缺失但 contract 要求 finding：
   patch 必须补交，不能靠继承空值。

5. replacement finding 必须重新通过候选、证据和 causal ceiling 校验。
```

为了可靠识别“诊断承载块”，可以在后续硬化阶段给 `AnswerBlock` 增加轻量引用：

```go
type TraceDecisionBinding struct {
    Role        string `json:"role"`         // primary / contributor / unresolved
    CandidateID string `json:"candidate_id"`
}
```

它只是一条 finding 绑定引用，不把 finding 或 cluster 数据塞进 `AnswerDocumentV2`。

### 7.5 Mutation 落点

建议新增：

```text
internal/types/final_answer_artifacts.go
internal/tool/final_answer_artifacts_mutation.go
```

`MutableState` 增加：

```go
func (m *MutableState) FinalAnswerArtifacts() *FinalAnswerArtifactsV1
func (m *MutableState) TraceFinding() *TraceFindingV1
func (m *MutableState) SetFinalAnswerArtifacts(v *FinalAnswerArtifactsV1)
```

旧的 `AnswerDocumentV2()` 和 `SetAnswerDocumentV2()` 保持兼容，由新 envelope 的 document 字段代理。

最终导出入口 [`internal/orchestrator/record_task_finalize.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/orchestrator/record_task_finalize.go) — `recordTaskFinalize` 只负责：

- 记录最终 Markdown；
- 导出 finding sidecar 路径；
- 在 `NodeArtifactLedger` 中写入 `trace_finding` artifact ref；
- 不从 Markdown 反向重建 finding。

---

## 8. `TraceFindingV1` 校验

### 8.1 结构不变量

```text
PrimaryCause != nil
或者
Unresolved != nil

二者不能同时为空；
二者不能同时表示最终主结论。
```

### 8.2 候选不变量

```text
PrimaryCause.CandidateID ∈ CandidateSet.PrimaryEligible
Contributors[].CandidateID ∈ CandidateSet.ContributorEligible
ContextOnly 记录不能成为 primary
模型不得创造新的 CandidateID
```

### 8.3 token 不变量

```text
Token.Token 必须存在于 CausalTokenUniverse
Lane/Additivity/SubjectKind 必须与 CausalTokenSpecFor 完全一致
FixDirection 必须与 registry 一致
RegistryHash 必须与 candidate set 一致
```

未知 token 的处理：

```text
不能作为 cluster authority；
只能进入 unresolved.raw_label；
不能自动注册新 token；
后续由人工扩展 registry 并更新 golden。
```

### 8.4 证据不变量

```text
Finding 中每个 EvidenceRef 必须存在于 accepted evidence；
PrimaryCause 至少绑定一个证据；
Contributor 至少绑定一个证据；
CounterEvidenceRef 不能引用不存在的证据；
Finding 不能引用另一个物理 Trace 的 evidence。
```

### 8.5 因果上限不变量

示例：

```text
causal ceiling = unproven
→ status 不能是 proven

missing_wakeup 只证明 selected window 无匹配 sched_wakeup
→ 不能据此生成具体 blocker

pre_wakeup_dependency
→ 不能写成 post_wakeup preemption

priority_inversion_candidate
→ 没有独立 holder/waiter 证据时不能升级成 proven lock inversion
```

### 8.6 Roster 不变量

若 `TraceRankRosterAuthority.Complete == false`：

```text
不能把缺 rank、重复 rank 或无有效 impact 的 roster
当作完整 Top-N 排名。
```

Finding 可以降级为：

```text
supported_candidate
unresolved
coverage caveat
```

但不能补造缺失席位。

---

## 9. 批次数据模型

### 9.1 Manifest

```go
type TraceBatchManifestV1 struct {
    SchemaVersion int                      `json:"schema_version"`
    BatchID       string                   `json:"batch_id"`
    CreatedAt     time.Time                `json:"created_at"`

    Analysis      TraceBatchAnalysisPolicy `json:"analysis"`
    Runner        TraceBatchRunnerPolicy   `json:"runner"`
    Cluster       TraceBatchClusterPolicy  `json:"cluster"`
    Report        TraceBatchReportPolicy   `json:"report"`

    Units         []TraceAnalysisUnitRef   `json:"units"`
}
```

### 9.2 Batch item 状态

```go
type TraceBatchItemStatus string

const (
    BatchItemDiscovered       TraceBatchItemStatus = "discovered"
    BatchItemQueued           TraceBatchItemStatus = "queued"
    BatchItemRunning          TraceBatchItemStatus = "running"
    BatchItemAnalyzed         TraceBatchItemStatus = "analyzed"
    BatchItemFindingCommitted TraceBatchItemStatus = "finding_committed"
    BatchItemRetryableFailure TraceBatchItemStatus = "retryable_failure"
    BatchItemTerminalFailure  TraceBatchItemStatus = "terminal_failure"
    BatchItemClustered        TraceBatchItemStatus = "clustered"
)
```

```go
type TraceBatchItemState struct {
    UnitID          string               `json:"unit_id"`
    Status          TraceBatchItemStatus `json:"status"`
    AnalysisKey     string               `json:"analysis_key"`
    RunID           string               `json:"run_id,omitempty"`

    SnapshotPath    string               `json:"snapshot_path,omitempty"`
    FindingRef      RuntimeArtifactRef   `json:"finding_ref,omitempty"`
    AnswerPath      string               `json:"answer_path,omitempty"`

    Attempts        int                  `json:"attempts"`
    LastErrorCode   string               `json:"last_error_code,omitempty"`
    LastErrorDetail string               `json:"last_error_detail,omitempty"`
}
```

### 9.3 Batch 状态

```go
type TraceBatchStateV1 struct {
    SchemaVersion int                       `json:"schema_version"`
    BatchID       string                    `json:"batch_id"`
    Generation    int64                     `json:"generation"`
    UpdatedAt     time.Time                 `json:"updated_at"`

    ManifestHash  string                    `json:"manifest_hash"`
    Items         map[string]TraceBatchItemState `json:"items"`

    ExactClusterRef RuntimeArtifactRef      `json:"exact_cluster_ref,omitempty"`
    ClusterSetRef   RuntimeArtifactRef      `json:"cluster_set_ref,omitempty"`
    ReportRef       RuntimeArtifactRef      `json:"report_ref,omitempty"`
}
```

`Generation` 用于防止旧 worker 结果覆盖新状态。

---

## 10. 持久化设计

### 10.1 推荐目录

```text
.codrax/
├── trace-findings/
│   └── by-analysis-key/
│       └── <analysis-key>.json
└── trace-batches/
    └── <batch-id>/
        ├── manifest.json
        ├── state.json
        ├── units/
        │   └── <unit-id>/
        │       ├── input.json
        │       ├── read_run_snapshot.json
        │       ├── finding.ref.json
        │       ├── answer.md
        │       └── failure.json
        ├── clusters/
        │   ├── exact.json
        │   ├── ambiguous_components.json
        │   └── decisions/
        │       └── <component-id>.json
        ├── cluster_set.json
        └── report/
            ├── report.md
            └── report.html
```

### 10.2 写入原则

复用现有 `types.AtomicWriteFileSync` 和 `ReadRunSnapshot` 的工程模式：

```text
normalize
→ marshal
→ 写临时文件
→ fsync
→ rename
```

批次进程内建议由单一 state-writer goroutine 串行提交 `state.json`，worker 只发送 typed result event，避免多个 worker 同时改同一文件。

### 10.3 Finding 内容寻址

`TraceFindingV1` 按 `AnalysisKey` 保存，批次只保存引用。这样：

- 相同 unit + 相同分析合同可跨批次复用；
- 新批次不必重新跑所有 Trace；
- 只改报告模板不使 finding 失效；
- 只升级聚类指纹时不重跑单 Trace 分析。

### 10.4 Runtime artifact 谱系

在 [`internal/types/node_artifact_ledger.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/node_artifact_ledger.go) — `RuntimeArtifactKind` 增加：

```go
RuntimeArtifactTraceFinding   RuntimeArtifactKind = "trace_finding"
RuntimeArtifactTraceClusterSet RuntimeArtifactKind = "trace_cluster_set"
RuntimeArtifactTraceBatchReport RuntimeArtifactKind = "trace_batch_report"
```

ledger 只记录 ref：

```text
ID
Path
ContentHash
Version
ProducerNodeID
ConsumerNodeID
```

payload 继续存放在 typed store 中。

---

## 11. 确定性归一化

### 11.1 需要移除或降权的实例噪声

```text
PID / TID
绝对时间戳
内存地址
Binder transaction ID
随机任务 ID
临时文件路径
匿名线程序号
设备运行时生成的对象 ID
符号地址偏移
```

这些信息可以保留在 finding 的 instance provenance 中，但不能参与稳定根因指纹。

### 11.2 应保留的稳定语义

```text
causal token
causal lane
subject role
upstream role
causal shape
phase
normalized event signature
normalized stack signature
fix direction
analysis profile family
```

示例：

```text
com.demo-38291 被 Binder:59321 唤醒
com.demo-40122 被 Binder:61107 唤醒
```

归一化后都可能得到：

```text
token=binder_wait
lane=wakeup_chain
subject_role=ui_thread
upstream_role=binder_server
causal_shape=upstream_completion_wakes_target
phase=pre_wakeup_dependency
```

### 11.3 线程角色

建议闭集：

```text
ui_thread
render_thread
binder_client
binder_server
system_service
app_worker
io_worker
gc_thread
compiler_thread
kernel_worker
unknown
```

角色由确定性规则优先生成；无法确定时使用 `unknown`，不能由模型自由创造无限标签。

### 11.4 业务阶段

建议闭集：

```text
startup
frame_measure
frame_layout
frame_draw
frame_submit
input_dispatch
animation
background
unknown
```

若业务无法稳定识别，保持 `unknown`，不要用应用自定义长文本直接作为一级聚类键。

---

## 12. 根因指纹

### 12.1 指纹结构

```go
type TraceCauseFingerprintV1 struct {
    Version            string `json:"version"`

    Token              string `json:"token"`
    Lane               string `json:"lane"`
    SubjectRole        string `json:"subject_role"`
    UpstreamRole       string `json:"upstream_role,omitempty"`
    CausalShape        string `json:"causal_shape"`
    Phase              string `json:"phase"`

    NormalizedEventKey string `json:"normalized_event_key,omitempty"`
    NormalizedStackKey string `json:"normalized_stack_key,omitempty"`

    RegistryHash       string `json:"registry_hash"`
}
```

### 12.2 Cluster ID

```text
ClusterID =
  SHA256(
    canonical_json(TraceCauseFingerprintV1)
  )
```

必须保证：

```text
修改中文 label，不改变 ClusterID；
输入顺序变化，不改变 ClusterID；
PID/TID/时间戳变化，不改变 ClusterID；
fingerprint version 变化，明确产生新 ClusterID。
```

### 12.3 不把 measurement board 直接塞入根因 ID

`BoardParamsFingerprint`、window policy、unit、caliber 属于测量兼容性，不完全等于根因身份。

推荐：

```text
semantic cluster：
  由 CauseFingerprint 决定

metric bucket：
  由 BoardFingerprint + Additivity + Unit + Caliber 决定
```

同一个根因 cluster 可以包含不同设备、不同窗口长度的样本，但数值分布必须分桶，不能直接求和。

---

## 13. 分层聚类

推荐同时提供三个层次：

| 层级 | 键 | 示例用途 |
|---|---|---|
| L0 | causal lane | 看调度、唤醒、IO、锁、内存等大类占比 |
| L1 | causal token | 看 `binder_wait`、`runnable_wait`、`low_frequency` 等机制占比 |
| L2 | mechanism fingerprint | 看具体线程角色、阶段、调用栈或事件签名 |
| L3 | incident signature，可选 | 定位某服务、某锁或某业务调用点 |

示例：

```text
wakeup_chain
  └─ binder_wait
       ├─ UI → PackageManager service
       ├─ UI → WindowManager service
       └─ RenderThread → media service
```

大盘使用 L0/L1；具体优化项使用 L2/L3。

---

## 14. 确定性预聚类

### 14.1 Exact cluster

完全相同的稳定指纹直接合并：

```text
same fingerprint
+ same fingerprint version
+ compatible analysis profile
→ exact must-link
```

### 14.2 Must-link

建议规则：

```text
1. 完整 fingerprint 相同；
2. token、角色、phase、causal shape 相同；
3. normalized stack/event key 相同；
4. registry hash 和指纹版本兼容。
```

### 14.3 Cannot-link

建议硬拆规则：

```text
1. pre_wakeup_dependency 与 post_wakeup_runnable；
2. aggregate_only 与具体 per-thread 机制冲突；
3. lock holder/waiter 与普通 wakeup relation；
4. 已证明相反的因果方向；
5. 不同 causal token 且 registry lane/语义明确不兼容；
6. 一个是 context-only，另一个是 positive primary cause；
7. profile 的分析目标或窗口选择语义不兼容；
8. 一个值是 wall clock，另一个只是 composite score，而分组理由仅来自数值相近。
```

### 14.4 Embedding 的边界

Embedding 可以用于：

```text
从大量 finding 中召回“可能相似”的候选对
```

但不能直接作为 merge authority：

```text
cosine similarity > 0.9
≠
同一物理根因
```

最终合并仍需 exact typed 特征或受限 Agent 决策。

### 14.5 避免 O(N²)

对上千个 finding，不做全量两两比较。先按以下键分桶：

```text
lane
token
subject role
phase
profile family
```

只在兼容桶内构建模糊候选边。精确聚类复杂度可保持在 `O(N log N)`；模糊比较只作用于小桶。

---

## 15. 可选 `RootCauseClustererAgent`

### 15.1 为什么仍然需要它

确定性指纹能解决：

```text
完全相同
明确不同
```

但难以稳定处理：

```text
binder_wait
service_completion_dependency
synchronous_ipc_wait
```

它们可能是同一机制的不同表达，也可能缺少关键 typed 证据。此时可以让 Agent 只判断已有 finding 之间是否应合并。

### 15.2 Agent 输入

```go
type AmbiguousClusterComponentV1 struct {
    ComponentID string                   `json:"component_id"`
    Findings    []TraceFindingSummary    `json:"findings"`
    Similarities []TypedSimilarityEdge   `json:"similarities"`
    Conflicts    []TypedConflictEdge     `json:"conflicts"`
    MustLinks    []FindingPair           `json:"must_links"`
    CannotLinks  []FindingPair           `json:"cannot_links"`
}
```

Agent 不读取：

```text
原始 Trace
trace_query
repo_map
read_file
shell
最终 Markdown 全文
```

它只读取小型 typed component，默认成员上限例如 20。超过上限时由程序继续按 token、phase、role 拆分。

### 15.3 Agent 输出

```go
type TraceClusterDecisionV1 struct {
    ComponentID string                      `json:"component_id"`
    Groups      []TraceClusterGroupDecision `json:"groups"`
    Unresolved  []string                    `json:"unresolved,omitempty"`
    ReasonCodes []string                    `json:"reason_codes"`
    Confidence  TraceConfidence             `json:"confidence"`
}
```

每个 group 只列现有 finding ID：

```go
type TraceClusterGroupDecision struct {
    MemberFindingIDs []string `json:"member_finding_ids"`
    CanonicalLabel    string   `json:"canonical_label,omitempty"`
}
```

### 15.4 Agent 权限边界

Agent 不得：

```text
创造 finding ID；
创造 evidence ID；
创造 causal token；
修改单 Trace primary cause；
改变测量数值；
把 cannot-link 成员放进同组；
遗漏 component 成员；
生成新的原始因果事实。
```

程序 reducer 必须重新校验输出。Agent 失败或置信度不足时采用：

```text
宁拆勿错合：
保留原 exact clusters，并标记 semantic_relation_unresolved。
```

### 15.5 接入方式

第二阶段新增：

- [`internal/types/enums.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/enums.go) — `AgentRootCauseClusterer`
- [`internal/agent/registry.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/agent/registry.go) — `RegisterDefaults`
- `internal/agent/root_cause_clusterer.go`
- `internal/tool/emit_trace_cluster_decision.go`

但**不新增 `StageCluster`，不修改 `builtinStageBindings`，不修改 `pipelineTopology`**。  
该 Agent 由 `internal/tracebatch` 直接调度，是批次工作流中的可选语义节点，不是每次 Read Run 的固定阶段。

---

## 16. 聚类结果类型

```go
type TraceRootCauseClusterSetV1 struct {
    SchemaVersion      int                       `json:"schema_version"`
    BatchID            string                    `json:"batch_id"`
    FingerprintVersion string                    `json:"fingerprint_version"`

    InputUnitCount     int                       `json:"input_unit_count"`
    SuccessfulCount    int                       `json:"successful_count"`
    ResolvedCount      int                       `json:"resolved_count"`
    UnresolvedCount    int                       `json:"unresolved_count"`
    FailedCount        int                       `json:"failed_count"`

    Clusters           []TraceRootCauseCluster   `json:"clusters"`
    Unresolved         []TraceUnresolvedMember   `json:"unresolved"`
    Failures           []TraceBatchFailure       `json:"failures"`

    Invariants         ClusterInvariantReport    `json:"invariants"`
}
```

```go
type TraceRootCauseCluster struct {
    ClusterID            string                       `json:"cluster_id"`
    Level                string                       `json:"level"`
    ParentClusterID      string                       `json:"parent_cluster_id,omitempty"`

    Fingerprint          TraceCauseFingerprintV1      `json:"fingerprint"`
    CanonicalLabel       string                       `json:"canonical_label"`

    PrimaryMembers       []TraceClusterMember         `json:"primary_members"`
    ContributorMembers   []TraceClusterMember         `json:"contributor_members,omitempty"`

    PrimaryCount         int                          `json:"primary_count"`
    ShareOfAllSuccessful float64                      `json:"share_of_all_successful"`
    ShareOfResolved      float64                      `json:"share_of_resolved"`

    MetricBuckets        []TraceClusterMetricBucket   `json:"metric_buckets,omitempty"`
    Representatives      []TraceFindingRef            `json:"representatives"`

    MergeBasis           []string                     `json:"merge_basis"`
    AmbiguityStatus      string                       `json:"ambiguity_status"`
}
```

### 16.1 Primary 与 Contributor 分开统计

每个成功 finding：

```text
最多进入一个 primary cluster；
可以进入多个 contributor cluster；
或者进入 unresolved。
```

因此：

```text
primary share ≤ 100%
contributor frequency 可以超过 100%
```

报告必须明确区分，不能把多因子统计冒充互斥占比。

### 16.2 Singleton

单个成员也形成合法 cluster，并带：

```text
singleton=true
```

不再另设一个与 cluster 重复的 outlier 成员池。这样守恒公式更简单。

---

## 17. 批次不变量

至少建立以下硬门：

```text
1. successful_count
   = 所有 primary cluster 成员数
   + unresolved_count。

2. 每个 resolved finding 恰好属于一个 primary cluster。

3. contributor membership 不参与 primary 守恒。

4. failed unit 不得被静默丢弃。

5. Agent 输出不得引用 component 外的 finding。

6. cluster evidence 必须来自成员 finding。

7. 输入顺序变化不得改变 exact cluster ID 和成员集合。

8. 修改 PID/TID/绝对时间戳/地址不得改变 semantic fingerprint。

9. 修改 CanonicalLabel 不得改变 ClusterID。

10. 不兼容 additivity/unit/caliber 的数值不得进入同一个 metric subtotal。

11. physical artifact identity 必须保留；
    不得把多个 Trace 的时间轴重新合并。

12. 同一 analysis key 的重复输入按 manifest 策略去重或显式计为重复样本，
    不能静默重复计数。

13. finding/document patch 不得产生语义不同步。

14. checkpoint 恢复后的确定性结果必须与一次性完整运行一致。

15. Clusterer Agent 关闭时，系统仍能产出完整、确定性的 exact-cluster 基线。
```

---

## 18. 批次报告

### 18.1 Finalizer 只消费聚类结果，不参与聚类计算

新增：

```go
type TraceClusterReportView struct {
    BatchSummary       TraceBatchSummaryView
    TopClusters        []TraceClusterReportRow
    RepresentativeCases []TraceRepresentativeCase
    UnresolvedSummary  TraceUnresolvedSummary
    FailureSummary     TraceFailureSummary
}
```

推荐仿照当前 trace decision handoff 新增：

```text
internal/agent/answer_document_trace_cluster_handoff.go
```

职责：

- 将 `TraceClusterReportView` 转成紧凑 prompt；
- 明确模型只拥有摘要、解释和建议；
- counts、percentages、成员和 metric bucket 由程序拥有。

### 18.2 系统与模型的报告边界

系统确定性生成：

```text
cluster 表格
count
share
失败数
unresolved 数
完整成员附录
metric bucket
代表 finding 引用
```

Finalizer 生成：

```text
批次摘要
主要趋势解释
优化优先级
代表案例解读
风险和下一步验证建议
```

### 18.3 不新增 BatchFinalizerAgent

复用现有 `AgentFinalizer`，但使用报告专用的 `AnswerContract` 和 `TraceClusterReportView`。  
最终渲染继续经过：

- `AnswerDocumentV2`
- `RenderAnswerDocumentWithLastMileSupplements`
- `recordTaskFinalize`

推荐新增系统 materializer，将确定性表格插入最终 document；模型不能自行重写统计数字。

---

## 19. CLI 与 Manifest

### 19.1 低风险入口

当前已有 `--tracediag` 独立入口。第一版可采用类似方式：

```bash
codrax --trace-batch trace-batch.yaml
```

新增：

```text
cmd/trace_batch.go
```

并在 `cmd/root.go` 做互斥校验：

```text
trace batch
read/write single shot
tracediag
```

不能同时激活。

### 19.2 Manifest 示例

```yaml
schema_version: 1
batch_id: startup-jank-20260805

analysis:
  profile_id: android-ui-jank-v1
  request_template: >
    分析该 Trace 中目标应用主线程在指定窗口内的卡顿主根因，
    区分主因、贡献因子和证据不足项。
  finding_required: true
  target_policy: manifest
  window_policy: manifest

runner:
  concurrency: 8
  max_attempts: 2
  resume: true
  fail_fast: false

cluster:
  fingerprint_version: causal-v1
  exact_cluster: true
  ambiguity_agent: true
  ambiguity_component_cap: 20
  unknown_policy: keep_separate

report:
  language: zh-CN
  top_clusters: 20
  representatives_per_cluster: 3
  include_member_appendix: true

units:
  - unit_id: t001-frame-100
    artifact_path: traces/001.perfetto-trace
    target:
      process: com.example.app
      thread_role: ui_thread
    window:
      start_ts: 100.125
      end_ts: 100.255

  - unit_id: t002-frame-203
    artifact_path: traces/002.perfetto-trace
    target:
      process: com.example.app
      thread_role: ui_thread
    window:
      start_ts: 210.010
      end_ts: 210.180
```

---

## 20. 配置建议

在 [`internal/config/runtime.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/config/runtime.go) 增加：

```yaml
trace_batch_enabled: false
trace_batch_concurrency: 4
trace_batch_max_attempts: 2
trace_batch_resume_enabled: true
trace_finding_shadow_mode: true
trace_cluster_agent_enabled: false
trace_cluster_ambiguity_component_cap: 20
trace_cluster_fingerprint_version: causal-v1
```

原则：

- 功能默认关闭；
- 先 shadow typed finding；
- exact cluster 可先上线；
- ambiguity Agent 单独开关；
- Agent 失败不阻塞 exact cluster 报告。

---

## 21. 代码改造清单

### 21.1 需要修改的现有文件

| 文件与锚点 | 改造内容 |
|---|---|
| [`internal/types/context.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/context.go) — `MutableState` | 增加 `FinalAnswerArtifactsV1` / `TraceFindingV1` 的事务读写 |
| [`internal/tool/emit_answer_document.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/tool/emit_answer_document.go) — `EmitAnswerDocument` | 教授新的 final answer envelope；普通请求保持旧 schema |
| [`internal/tool/emit_answer_document_v2.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/tool/emit_answer_document_v2.go) — `emitAnswerDocumentV2Params`、`executeAnswerDocumentV2` | 解码、校验并原子提交 `trace_finding` |
| [`internal/tool/emit_answer_document_patch.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/tool/emit_answer_document_patch.go) — `emitAnswerDocumentPatchParams`、`Execute` | 增加 `replace_trace_finding` 与 stale 检查 |
| [`internal/tool/answer_document_dynamic_schema.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/tool/answer_document_dynamic_schema.go) — `BuildAnswerDocumentParametersFor` | 仅在 contract 激活时投影 `trace_finding` |
| [`internal/types/answer_document_v2_mutation.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/answer_document_v2_mutation.go) — `AnswerDocumentMutation` | 由新 final-answer mutation 包装，保留旧兼容接口 |
| [`internal/agent/answer_document_trace_decision_handoff.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/agent/answer_document_trace_decision_handoff.go) — `renderAnswerDocTraceDecisionHandoff` | 抽出 typed candidate compiler，prompt 与 validator 共用 |
| [`internal/agent/finalizer.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/agent/finalizer.go) — `NewFinalizerAgent` | 注入 finding contract/validator，不加入聚类逻辑 |
| [`internal/orchestrator/record_task_finalize.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/orchestrator/record_task_finalize.go) — `recordTaskFinalize` | 导出 finding sidecar 和 artifact ref |
| [`internal/types/node_artifact_ledger.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/node_artifact_ledger.go) — `RuntimeArtifactKind` | 增加 finding、cluster set、batch report 引用类型 |
| [`internal/types/read_run_snapshot.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/read_run_snapshot.go) — `ReadRunSnapshot` | 可增加 final typed artifact ref；不要嵌入大 payload |
| [`internal/orchestrator/read_run_snapshot_resume.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/orchestrator/read_run_snapshot_resume.go) — snapshot seed | 子 Run 断点恢复复用 |
| [`cmd/root.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/cmd/root.go) | 增加 `--trace-batch` 路由与互斥校验 |
| [`internal/config/runtime.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/config/runtime.go) | 增加批次配置 |

第二阶段可选 Agent 才修改：

| 文件与锚点 | 改造内容 |
|---|---|
| [`internal/types/enums.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/enums.go) — `AgentName` | 增加 `AgentRootCauseClusterer` |
| [`internal/agent/registry.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/agent/registry.go) — `RegisterDefaults` | 注册 Agent |
| **不修改** [`internal/types/stage_binding.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/stage_binding.go) | 不增加主流水线 stage |
| **不修改** [`internal/orchestrator/topology.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/orchestrator/topology.go) | Read 主拓扑保持不变 |

### 21.2 建议新增文件

```text
internal/types/
├── final_answer_artifacts.go
├── trace_finding.go
├── trace_finding_candidate.go
├── trace_batch.go
└── trace_cluster.go

internal/analysis/tracefinding/
├── candidate_compiler.go
├── token_snapshot.go
├── validator.go
├── revision_hash.go
└── consistency.go

internal/analysis/tracecluster/
├── normalize.go
├── fingerprint.go
├── exact.go
├── constraints.go
├── ambiguous_graph.go
├── reduce.go
├── representatives.go
└── invariants.go

internal/tracebatch/
├── manifest.go
├── store.go
├── runner.go
├── child_run_factory.go
├── checkpoint.go
├── resume.go
├── clustering.go
└── report.go

internal/agent/
├── answer_document_trace_cluster_handoff.go
└── root_cause_clusterer.go              # 第二阶段

internal/tool/
├── final_answer_artifacts_mutation.go
└── emit_trace_cluster_decision.go       # 第二阶段

cmd/
└── trace_batch.go
```

---

## 22. 包依赖边界

推荐依赖方向：

```text
internal/types
    ↑
internal/tracequery
    ↑
internal/analysis/tracefinding
    ↑
internal/analysis/tracecluster
    ↑
internal/tracebatch
```

更准确地说：

```text
types：
  只定义 carrier，不依赖 tracequery

tracefinding：
  导入 types + tracequery
  负责 registry snapshot 和 finding validation

tracecluster：
  导入 types
  必要时导入 tracefinding 的规范化接口
  不读取原始 trace

tracebatch：
  导入 orchestrator / agent / types / tracefinding / tracecluster
  负责外层流程
```

禁止：

```text
types → tracequery
tracequery → tracebatch
finalizer → tracebatch store
```

这样可以避免包循环和职责反转。

---

## 23. 错误分类

建议至少区分：

```text
trace_input_invalid
trace_parse_failed
target_not_found
window_not_found
read_run_failed
read_run_cancelled
finding_missing
finding_schema_invalid
finding_candidate_violation
finding_evidence_violation
finding_causal_ceiling_violation
cluster_fingerprint_failed
cluster_agent_failed
cluster_decision_invalid
cluster_invariant_failed
report_finalize_failed
```

失败策略：

```text
单个 unit 失败：
  默认不终止整个批次；
  写入 failure；
  继续其他 unit。

Finding 校验失败：
  不能用 Final Markdown 替代；
  按子 Run retry policy 重试；
  最终失败则进入 batch failure。

Clusterer Agent 失败：
  保留 exact clusters；
  模糊关系标记 unresolved；
  继续生成报告。

Invariant 失败：
  不发布“成功聚类报告”；
  生成 fail-loud 审计结果。
```

---

## 24. 可观测性

每个 unit 至少记录：

```text
unit_id
analysis_key
run_id
attempt
status
trace content hash
profile hash
candidate count
finding primary token
finding validation result
snapshot path
elapsed time
token usage
error code
```

批次聚合记录：

```text
total / success / unresolved / failed
exact cluster count
ambiguous component count
agent-dispatched component count
agent-failed component count
singleton count
largest cluster size
finding cache hit rate
resume hit rate
```

日志不得只写自然语言；关键字段应输出结构化事件。

---

## 25. 测试计划

### 25.1 单元测试

新增：

```text
internal/types/trace_finding_test.go
internal/types/final_answer_artifacts_test.go
internal/analysis/tracefinding/candidate_compiler_test.go
internal/analysis/tracefinding/validator_test.go
internal/analysis/tracecluster/fingerprint_test.go
internal/analysis/tracecluster/constraints_test.go
internal/analysis/tracecluster/invariants_test.go
internal/tracebatch/store_test.go
internal/tracebatch/resume_test.go
```

### 25.2 Full/Patch 原子性测试

覆盖：

```text
full emit 同时成功
document 失败时 finding 不落地
finding 失败时 document 不落地
patch 只改 caveat 时 finding 继承
patch 改 decision 未换 finding 时拒绝
patch 同时替换 decision + finding 时成功
进程中断模拟后不出现半提交
```

参考现有测试族：

- [`internal/tool/emit_answer_document_patch_test.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/tool/emit_answer_document_patch_test.go)
- [`internal/agent/finalizer_tool_schema_test.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/agent/finalizer_tool_schema_test.go)
- [`internal/types/trace_causal_projection_partition_test.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/trace_causal_projection_partition_test.go)
- [`internal/types/read_run_snapshot_test.go`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/internal/types/read_run_snapshot_test.go)

### 25.3 Property tests

```text
随机替换 PID/TID/时间戳 → fingerprint 不变
随机打乱输入顺序 → exact clusters 不变
随机改 label → ClusterID 不变
随机重复输入 → 按 manifest policy 去重或显式重复
随机制造 incompatible unit → metric subtotal 拒绝
随机中断/恢复 → reducer 结果一致
```

### 25.4 集成测试

```text
10 unit 快速测试
100 unit synthetic batch
部分失败 + resume
finding cache 命中
Agent 开/关对照
Agent 非法 member 注入
大模糊 component 拆分
```

### 25.5 聚类评测

建立人工标注 gold set，至少衡量：

```text
pairwise precision / recall
B³ precision / recall / F1
Adjusted Rand Index
unresolved precision
false-merge rate
cluster stability
representative coverage
evidence reference correctness
```

在根因分析中，false merge 通常比 false split 更危险，因此优先优化 precision，并保留 unresolved。

---

## 26. 分阶段落地

### P0：单 Trace typed finding shadow

目标：

```text
不改变现有用户答案；
Finalizer 同事务产出 TraceFindingV1；
只记录、不参与批次聚类；
对照 finding 与最终诊断文字。
```

完成条件：

```text
full/patch 原子性通过；
候选/evidence/causal ceiling 校验通过；
普通非 Trace 请求无 schema 变化；
历史 AnswerDocumentV2 兼容。
```

### P1：确定性批次

实现：

```text
TraceBatchManifestV1
有界 worker pool
每 unit 独立 Run
Finding store
Read snapshot resume
exact fingerprint clustering
cluster invariant validator
deterministic tables
现有 Finalizer 批次摘要
```

此阶段不需要 Clusterer Agent，先得到一个完全可复现的基线。

### P2：模糊语义 Agent

实现：

```text
AmbiguousClusterComponentV1
RootCauseClustererAgent
emit_trace_cluster_decision
deterministic reducer
Agent 开关和失败降级
```

Agent 只减少“不必要的拆分”，不能突破 typed cannot-link。

### P3：增量与人工校正

实现：

```text
新增 finding 只重算受影响 fingerprint bucket
ClusterOverrideV1
人工 must-link / cannot-link
cluster alias
fingerprint version migration
历史批次对比
```

---

## 27. 与 Survey Mode 的关系

仓库已有 [`docs/design/survey_mode_discussion.md`](https://github.com/hanchaoqun/codrax/blob/7fd42889d3d9e48e79b9524b2c7d28215dfdcee5/docs/design/survey_mode_discussion.md)，其中讨论过：

```text
外层 Survey Orchestrator
FocusedReadPrimitive
AggregatePrimitive
ArtifactStore
RenderPrimitive
```

其方向与本设计一致，但该文档明确标注 **NOT GREENLIT**。因此本设计不依赖通用 Survey Mode 落地，而是先实现一个专用垂直切片：

```text
internal/tracebatch
```

当 TraceBatch 的 artifact store、child-run primitive、aggregate/reducer 和 report contract 被真实数据验证后，再考虑抽象为通用 Survey primitive。不要先造一个宏大的“万能调查平台”，然后用三个月证明它确实能把一个 JSON 文件搬到另一个目录。

---

## 28. 最终 ADR

### 决策

采用：

```text
Finalizer typed sidecar
+ TraceBatchWorkflow
+ deterministic normalization/exact clustering
+ optional bounded RootCauseClustererAgent
+ existing Finalizer report
```

### 原因

1. 与当前“系统拥有测量、模型拥有诊断”的责任边界一致；
2. 不改变 Read 主流水线；
3. 不扩大 `TraceCausalProjectionSet` 的小规模比较职责；
4. 单 Trace 证据空间保持独立；
5. 聚类可恢复、可增量、可验证；
6. Agent 只处理程序难以裁决的语义灰区；
7. 现有 causal token registry、rank roster、snapshot、artifact ledger 均可复用；
8. 即使关闭 Agent，系统仍有完整确定性基线。

### 明确不做

```text
不从最终 Markdown 反向抽取 finding；
不把 100 个 Trace 放进同一棵因果树；
不把 batch 状态塞进单个 BusContext；
不把 cluster 数据塞进 AnswerDocumentV2；
不让 embedding 直接决定合并；
不让 Clusterer Agent 读取原始 Trace；
不增加 BatchFinalizerAgent；
第一阶段不修改主 pipeline topology。
```

### 最终一句话

> **单 Trace 的诊断结论在 Finalizer 中 typed 化，但跨 Trace 的组织、聚类、恢复和守恒必须由新的批次工作流负责；新 Agent 只裁决模糊分组，不能取代确定性系统。**
