# Session 11 — Feedback-Closure Architecture 完整版设计稿

> **Status**: design definitive · 2026-04-18
> **Scope**: 四轴 19 组件一次性 ship，不遗留到 session 12
> **Blueprint**: `docs/session11_feedback_closure_blueprint.md`
> **Trigger bug**: `explorer agent 默认用哪个 skill?` — log 审计揭示的系统性失效

本文档是每个组件的 mini-spec 参考手册。每节给出：接入点、数据结构、关键逻辑、测试清单。不重复蓝图里的 why —— 只谈 how。

---

## 总览 — 文件接入密度

| 修改类型 | 文件数 | 子包新建 |
|---|---|---|
| 扩展现有文件 | ~18 | — |
| 新建文件 | ~12 | 4 子包 (`analysis/hint/`, `analysis/declarative/`, `analysis/aggregator/`, `analysis/patcher/`) |
| 新测试文件 | ~22 | — |

**不动的**：CGEC 不变量 I1–I4 / tool registry 结构 / 任何 LLM 模型配置 / REPL / 日志系统。

---

# Part I · F1 ViolationLedger（G1 组）

## 1.1 扩展 `contract.Violation`

文件：`internal/analysis/contract/checker.go`

```go
// 已有
type Violation struct {
    Kind   ViolationKind
    Detail string
    Repair string
}

// 新增字段
type Violation struct {
    Kind          ViolationKind
    Detail        string
    Repair        string
    Stage         types.PipelineStage   // NEW
    DispatchID    string                // NEW  — trace-N-iter-M 格式
    EvidenceRefs  []string              // NEW  — chain_id / evidence_id / citation_idx
    SuspectedRoot SuspectedRoot         // NEW  — 根因假设（强制）
}

type SuspectedRoot struct {
    IRField    string   // "answer_shape" / "answer_subject.kind" / "ScannedSet" / "EvidencePlan.SourceMix" / "entity_axes" / "question_kind"
    Reason     string   // ≤ 140 chars
    Confidence float64  // [0, 1]
}
```

### 1.2 新增 ViolationKind

```go
const (
    // 已有
    ViolShape ViolationKind = iota
    ViolCitation
    ViolMustInclude
    ViolMustExclude
    ViolAcceptance
    ViolSuccessCriterion
    // 新增
    ViolGhostAnchor       // D2: chain anchor outside ScannedSet
    ViolChainDemoted      // ERM: terminal predicate demoted (only self-ref records)
    ViolSelfRefLiteral    // R4: emit_evidence self-reference filter
    ViolPreCompleteDowngrade
    ViolLiteralFormFailed // C5: literal form ≠ answer_subject.kind form
    ViolShapeSwap         // B2a (deprecated by C4 but still written for audit)
)
```

## 1.3 扩展 `EvidenceClosure`

文件：`internal/types/evidence_closure.go`

```go
type EvidenceClosure struct {
    // ... 现有字段 ...
    violations []contract.Violation   // NEW
}

func (ec *EvidenceClosure) AppendViolation(v contract.Violation)
func (ec *EvidenceClosure) Violations() []contract.Violation
func (ec *EvidenceClosure) ViolationsByField(field string) []contract.Violation
func (ec *EvidenceClosure) ViolationsByStage(stage types.PipelineStage) []contract.Violation
func (ec *EvidenceClosure) ViolationsByKind(kind contract.ViolationKind) []contract.Violation
```

`ClosureStats.BumpViolations()` 加计数器。

## 1.4 Enforcer hookup（8 处）

每处现有 log-only reject 路径旁加 `AppendViolation`。**不改 reject 动作本身**，只是并行写 ledger。

| 文件::fn | SuspectedRoot.IRField | Confidence |
|---|---|---|
| `cgec_enforcers.go::recordGhostAnchor` | `ScannedSet` | 0.70 |
| `cgec_enforcers.go::shapeSwapB2a` | `answer_shape` | 0.85 |
| `emit_answer_document.go::dryRunG1_ZeroMatch` | `ScannedSet` | 0.90 |
| `emit_answer_document.go::citationGroundingDropped` | `EvidencePlan.SourceMix` | 0.60 |
| `contract/checker.go::checkShape` | `answer_shape` | 0.80 |
| `contract/checker.go::checkCitation` | `CitationReq` | 0.75 |
| `explorer_erm.go::chainDemoted(self_ref)` | `answer_subject.kind` | 0.80 |
| `emit_evidence.go::selfRefFiltered`（R4 产物） | `answer_subject.kind` | 0.75 |

**强约束**：这 8 处加 ledger 的同时，现有行为一字不动。G1 期是 observability-only。

## 1.5 可观测性：CGEC summary 扩展

`cgec_enforcers.go::logCGECSummary` 现有：
```
[CGEC] summary: chains_demoted=220 unverified=0 repairs_raised=6 ...
```
扩展为：
```
[CGEC] summary: chains_demoted=220 ... violations=29 by_field={answer_shape:3, ScannedSet:8, answer_subject.kind:12, ...} top_suspected=(answer_subject.kind, conf=0.80, events=12)
```

## 1.6 测试清单

- `TestViolationLedger_AllEnforcersWriteSuspectedRoot`（结构断言，参考 session 10 `TestAllRepairKindsHaveProducer` 模式）
- `TestViolation_SuspectedRootConfidenceInRange`
- `TestEvidenceClosure_ViolationsByFieldCorrectlyGroups`
- `TestCGECSummary_ExtendedFormatStable`

---

# Part II · F2 Aggregator + F3 IRPatchEngine（G2 组）

## 2.1 F2 `internal/analysis/aggregator/` 新包

### 数据结构

```go
// aggregator.go
type FieldHeat struct {
    IRField         string
    EventCount      int
    AvgConfidence   float64
    WeightedScore   float64   // AvgConfidence * min(EventCount/3, 1.0)
    Events          []contract.Violation
    TopReason       string    // 最高 confidence event 的 Reason
}

type Aggregator struct {
    minConfidence   float64  // default 0.5 — 低于此不入聚合
    patchThreshold  float64  // default 0.70 — WeightedScore ≥ 此 → patch
    hintThreshold   float64  // default 0.50 — WeightedScore ≥ 此 → hint enrichment
    minEventCount   int      // default 3
}

func (a *Aggregator) Aggregate(ec *types.EvidenceClosure) []FieldHeat

// 主要路径
func (a *Aggregator) Classify(heat FieldHeat) AggregationAction

type AggregationAction int
const (
    ActionIgnore AggregationAction = iota
    ActionHintEnrich
    ActionRequestIRPatch
)
```

### 聚合规则

```
field_heat.WeightedScore = AvgConfidence × saturation(EventCount / minEventCount)
saturation(x) = min(x, 1.0)

Action:
  WeightedScore ≥ patchThreshold ∧ EventCount ≥ minEventCount → RequestIRPatch
  WeightedScore ≥ hintThreshold                                 → HintEnrich
  otherwise                                                     → Ignore
```

### 跑点

`internal/orchestrator/scheduler.go` 每轮 explore window 结束时：
```go
heats := a.Aggregate(mutable.EvidenceClosure)
for _, heat := range heats {
    switch a.Classify(heat) {
    case ActionRequestIRPatch:
        patcher.Submit(buildPatchRequest(heat))
    case ActionHintEnrich:
        hintComposer.AddEnrichment(heat)
    }
}
```

### 测试

- `TestAggregator_ThresholdBoundaries`
- `TestAggregator_EventCountSaturation`
- `TestAggregator_FieldGroupingCorrect`
- `TestAggregator_LowConfidenceEventsFiltered`

## 2.2 F3 `internal/analysis/patcher/` 新包

### 核心原则

1. **White-listed fields only** — 能 patch 的 IR 字段是闭集
2. **Idempotent** — 同 patch 应用二次 noop
3. **Monotonic** — 回复到之前值的 patch 被拒绝
4. **Audited** — 所有 patch 进 `AnalysisIR.Meta.PatchLog`
5. **Bounded** — per-Run / per-field 硬上限

### 可 patch 字段白名单

| IR 字段 | 操作类型 | 约束 |
|---|---|---|
| `RequestModel.AnswerShape` | replace | 目标值必须 ∈ ShapeEnum |
| `RequestModel.AnswerSubject.Kind` | replace | 目标值必须 ∈ AnswerSubjectKindEnum |
| `RequestModel.QuestionKind` | replace | 目标值必须 ∈ QuestionKindEnum |
| `RequestModel.EntityAxes` | replace | 合法格式 `[A → B]` |
| `RequestModel.Keywords` | append-only | 不可删除原 keyword |
| `EvidencePlan.SourceMix` | patch by tool | 只可调整分配比例不可超 budget |
| `EvidencePlan.NodeBudgetHints[tool]` | replace | 在 [1, max_per_tool] |
| `ScannedSet`（Mutable 侧）| append-only | 文件必须真实存在 |

**不可 patch**：TaskGraph 结构 / Hypotheses 列表（只能 MarkHypothesis 状态）/ RiskMatrix / AnswerContract.AcceptanceTests 结构。

### 数据结构

```go
type PatchRequest struct {
    Field      string
    Operation  PatchOp       // Replace / AppendOnly / PatchByKey
    OldValue   any
    NewValue   any
    Reason     string
    SourceEvents []contract.Violation
    Dispatch   string        // which dispatch triggered
}

type Engine struct {
    budget        PatchBudget
    applied       map[string][]PatchRecord  // field → history
    auditLog      []PatchRecord
}

type PatchRecord struct {
    Request    PatchRequest
    Applied    bool
    SkipReason string   // "idempotent" / "monotonic_violation" / "budget_exhausted" / "field_not_whitelisted"
    AppliedAt  time.Time
}

func (e *Engine) Submit(req PatchRequest) PatchRecord
func (e *Engine) Apply(ir *types.AnalysisIR) []string  // 返回 recompile 触发的 subsystem 列表
```

### Recompile 副作用

patch 后触发对应下游 recompute（不重跑 analyzer LLM）：

| Patched Field | Recompile Trigger |
|---|---|
| AnswerShape / AnswerSubject.Kind / QuestionKind | `compiler.RecomputeShapeDerivedFields(ir)` — AnswerContract.RequiredAnswerShape + CitationReq |
| EntityAxes / Keywords | `compiler.RecomputeTemplate(ir)` — EvidencePlan nodes affected |
| SourceMix / NodeBudgetHints | `sourcemix.Recompile(ir.EvidencePlan)` |
| ScannedSet | 无 IR recompile；下轮 explore 读取最新 ScannedSet |

**现有 `compiler.RecomputeBudget` 是基础**（`internal/analysis/compiler/compile.go:52-58`），扩展出 `RecomputeShapeDerivedFields` 等同级 API。

### 污染清理

当 patch 改 `AnswerSubject.Kind` 时，扫描 `EvidenceClosure.Evidence`：
- evidence 里 `Subject == primary_entity` 且之前被当作"skill_name"接受的 → 标 `Salience = SalienceStale`（不删除，保留审计）
- 被标 stale 的 evidence 不进入下次 finalizer 的可引用集合

### C1 的吸收

原 C1 "E1 override subject 后手写级联 reconcile question_kind/shape/ERM"：
- 现在 E1 override → `ec.AppendViolation(kind=CueRuleOverride, SuspectedRoot={answer_subject.kind, 0.9})`
- F2 立即聚合（1 event × 0.9 conf × 1.0 sat = 0.9 ≥ patchThreshold）
- F3 patch answer_subject.kind → RecomputeShapeDerivedFields → RecomputeTemplate

**一份代码替代 C1 手写 reconcile**。

### 测试

- `TestPatchEngine_IdempotentNoop`
- `TestPatchEngine_MonotonicReject`
- `TestPatchEngine_FieldWhitelistEnforced`
- `TestPatchEngine_RecompileChainShapeDerived`
- `TestPatchEngine_EvidenceStaleOnKindChange`
- `TestPatchEngine_AuditTrailCompleteness`
- `TestPatchEngine_AbsorbsC1CascadeReconcile`

---

# Part III · F4 HintComposer（G3 组）

## 3.1 `internal/analysis/hint/` 新包

### 数据结构

```go
// composer.go
type Hint struct {
    WhatFailed        string      // required
    WhyItFailed       string      // required, 必须 cite SourceViolations
    WhatSystemDid     string      // required, 可为 "no patch applied"
    ExactFix          string      // required, imperative
    AllowedSet        []Allowed   // required, len ≥ 1
    ForbiddenPatterns []string    // optional (推荐 ≥ 1)
    
    SourceViolations []contract.Violation
    Stage            types.PipelineStage
    DispatchTarget   types.PipelineStage
    PatchesApplied   []patcher.PatchRecord   // 由 F3 提供
}

type Allowed struct {
    Kind  AllowedKind  // FileCitation / LiteralValue / ShapeEnum / SubjectKindEnum
    Value string
    Hint  string       // 为什么允许（≤ 80 chars）
}

type Composer struct {
    strictMode           bool
    maxAllowedSet        int
    maxForbiddenPatterns int
    aggregator           *aggregator.Aggregator  // 注入，用于查聚合结果
}

func (c *Composer) Compose(ctx ComposeContext, violations []contract.Violation) (*Hint, error)
func (c *Composer) Render(h *Hint) string

type ComposeContext struct {
    IR             *types.AnalysisIR
    EvidenceClosure *types.EvidenceClosure
    ReadSet         []string
    PatchRecords    []patcher.PatchRecord  // 最近一次 window 的 patch
    Stage           types.PipelineStage
    DispatchTarget  types.PipelineStage
}
```

### 字段合成规则（完整表）

| Violation 集合里出现 | WhatFailed 模板 | WhyItFailed 模板 | WhatSystemDid 填充 | ExactFix 模板 | AllowedSet | ForbiddenPatterns |
|---|---|---|---|---|---|---|
| 任一 Shape | "Your shape=X but contract requires Y" | 从 heat.TopReason 填 | 如果 PatchRecord 里有 AnswerShape patch → "IR patched X→Y (from Z violations)"；否则 "no patch applied" | "Re-emit with shape=Y and {ShapeSpecificFields}" | Shape value enum single | "Do not output shape ∈ {其他}" |
| 任一 Citation (out-of-ReadSet) | "N citations cite files outside ReadSet" | "ranker missed these files" / "chains anchored outside scan" | 若 ScannedSet patched → "forced-read X files into ScannedSet"；否则 "no patch" | "Cite only files from the Allowed list below" | ReadSet 里每个文件：{Kind:FileCitation, Value:F, Hint:"read at turn N"} | "Do not cite files not in ReadSet" / "Do not invent line numbers" |
| GhostAnchor 聚合 ≥ threshold | "Chain anchor F not in ScannedSet (seen N times)" | "aggregator flagged retrieval gap" | "added F to ScannedSet for next window" | "Next window will read F; wait for evidence" | ScannedSet + F | — |
| SelfRefLiteral | "answer literal = primary_entity name (self-reference)" | "AnswerSubject.kind=X demands non-self literal" | patch 若改了 AnswerSubject.kind → 说明；否则 "no patch" | "Pick literal matching kind=X; self-name is invalid" | 候选 literal list from concrete_values excluding primary_entity | `"Do NOT select literal='{primary_entity}' — that is self-reference"` |
| LiteralFormFailed (C5) | "literal '{L}' does not match kind={K} form rule" | 从 C5 的 form rule 填 | "no patch" | "Emit literal matching pattern {K.Regex}" | 候选 literal from evidence matching kind | literal form negative samples |
| Multiple kinds simultaneously | 合并渲染，逐条列 | 合并 | 合并 | 合并 | 去重 union | 去重 union |

### 渲染模板

```markdown
## Retry Directive

**What failed**: {WhatFailed}

**Why it failed**: {WhyItFailed}

**What I already did**: {WhatSystemDid}

**How to fix now**: {ExactFix}

**Allowed**:
{range AllowedSet}- `{.Value}` — {.Hint}  ({.Kind})
{end}

**Do NOT**:
{range ForbiddenPatterns}- {.}
{end}
```

### 严格验证（strictMode）

缺任一 required 字段 → `ErrIncompleteHint`。
`AllowedSet` 空 → `ErrEmptyAllowedSet`。
`WhyItFailed` 不含 `SourceViolations` 引用痕迹 → `ErrUngroundedHint`。

### 灰度

```yaml
hint_composer_strict_mode: false   # G3 期初期
```
shadow run 1 期后打开 `true`，统计 `ErrIncompleteHint` 发生率归零。

### 接入点（替换现有字符串拼接）

| 现调用点 | 现行为 | 替换为 |
|---|---|---|
| `contract_check.go::renderViolations` | `;` 分隔 | `composer.Render(c.Compose(...))` |
| `scheduler.go::renderWindowHint` | 手拼 | 同上 |
| `emit_answer_document.go::buildDryRunError` | 固定文案 | 同上 |
| `cgec_enforcers.go::buildE2ForcedReadRationale` | 手拼 | 同上 |
| `emit_evidence.go::buildSelfRefRejection`（R4） | 新建 | 同上 |

### 测试

- `TestHintComposer_StrictModeRejectsMissingFields`
- `TestHintComposer_AllowedSetNonEmpty`
- `TestHintComposer_WhyItFailedMustCiteSourceViolations`
- `TestHintComposer_RenderStableFormat`
- `TestHintComposer_CoversAllViolationKinds`
- `TestHintComposer_ComplexScenarioShapeAndCitationBoth`
- `TestHintComposer_PatchRecordSurfacedInWhatSystemDid`

---

# Part IV · F5 ViolationBudget + C6 retry diff（G4 组）

## 4.1 新配置字段

`internal/types/config.go`：

```go
type PipelineSettings struct {
    // 现有
    MaxRetriesPerStage int
    MaxStageVisits     int
    // 新增
    ViolationBudget ViolationBudgetSettings
    RetryBudgetByKind RetryBudgetByKind
}

type ViolationBudgetSettings struct {
    MaxPatchesPerRun       int     // default 4
    MaxPatchesPerField     int     // default 2
    MinRetryYield          int     // default 1
    FailLoudEnabled        bool    // default true
    YieldKillStage         bool    // default true (stage-level; false=run-level)
}

type RetryBudgetByKind struct {
    ShapeViolation    int  // default 1
    CitationViolation int  // default 3
    LiteralFormFailed int  // default 2
    GhostAnchor       int  // default 2
    SelfRefLiteral    int  // default 2
    Other             int  // default 1
}
```

## 4.2 yield check 实现

`internal/orchestrator/scheduler.go`：

```go
func (s *scheduler) shouldRetry(stage types.PipelineStage, windowResult WindowResult, vk contract.ViolationKind) bool {
    // C6: per-kind budget check
    used := s.gs.retryUsedByKind[stage][vk]
    limit := s.settings.RetryBudgetByKind.For(vk)
    if used >= limit {
        s.logRetryBudgetExhausted(stage, vk)
        return false
    }
    
    // 现有 total retry check 保留作后备
    if s.gs.retryUsed[stage] >= s.settings.MaxRetriesPerStage {
        return false
    }
    
    // F5: yield check
    delta := s.computeYield(stage, windowResult)
    if delta < s.settings.ViolationBudget.MinRetryYield {
        s.logYieldKill(stage, delta)
        s.gs.YieldKillCount++
        return false
    }
    
    return true
}

func (s *scheduler) computeYield(stage types.PipelineStage, wr WindowResult) int {
    // 只计"新增"；同 forced_read 不重复计数
    return wr.NewForcedReadsCount + wr.NewPatchesApplied + wr.NewEvidenceCount + wr.NewScannedSetAdditions
}
```

## 4.3 fail-loud 兜底

`internal/orchestrator/orchestrator.go::finalizeAnswer`：

```go
if s.gs.YieldKillCount > 0 || s.gs.FailLoudTriggered {
    warning := fmt.Sprintf(
        "⚠️ Pipeline terminated with unresolved violations: "+
        "%d retry windows yielded no progress; top suspected IR field: %s (confidence %.2f, %d events). "+
        "Classification may be incorrect.",
        s.gs.YieldKillCount,
        s.gs.TopSuspectedField, s.gs.TopSuspectedConfidence, s.gs.TopSuspectedEventCount,
    )
    answer = warning + "\n\n" + answer
}
```

**不隐藏失败**。用户强调的核心原则。

## 4.4 测试

- `TestViolationBudget_YieldKillStopsLoop`
- `TestViolationBudget_MonotonicityNoDoubleCount`
- `TestViolationBudget_FailLoudPrependsWarning`
- `TestViolationBudget_PerFieldPatchLimit`
- `TestRetryBudgetByKind_ShapeViolationExhaustsAtOne`
- `TestRetryBudgetByKind_CitationViolationAllowsThree`

---

# Part V · C0' ClassificationGrep + Declarative Classifier（G5 组）

## 5.1 `internal/analysis/declarative/` 新包

### Classifier

```go
// classifier.go
type Kind int
const (
    KindNone Kind = iota
    KindTopology
    KindRegistry
    KindDefaults
    KindRoutes
    KindWire
    KindManifest
    KindSchema
    KindEnum
)

type Classifier struct {
    filenamePatterns  []string   // 词表
    maxLinesForSmall  int        // default 60
    literalBlockRatio float64    // default 0.6
}

func New(cfg Config) *Classifier
func (c *Classifier) Classify(path string, content []byte) (Kind, float64)
func (c *Classifier) ClassifyPath(path string) (Kind, float64)  // 仅文件名（无 content 时）
```

### 分类规则

```
filename in patterns → (Kind, 1.0)
lines ≤ maxLinesForSmall ∧ literal_block_ratio(AST) ≥ literalBlockRatio → (KindTopology, 0.7)
YAML/JSON/TOML file → (KindSchema, 0.8)
No function bodies (all var/const/map) → (KindRegistry, 0.6)
otherwise → (KindNone, 0)
```

`literal_block_ratio` 用 Go AST 计算：`|toplevel-var-with-composite-literal| / |toplevel-decls|`。

### R1 复用

R1 DeclarativeBoost 直接用 `Classifier.ClassifyPath` 返回的 `(Kind, score)` 计算 ranker bonus：
```go
bonus := classifier.BoostFor(kind) // 同 kind 同分：Topology=+15, Registry=+12, Defaults=+10, Routes=+8, etc.
scoredFile.Score += bonus
```

### 测试

- `TestDeclarativeClassifier_FilenamePatterns`
- `TestDeclarativeClassifier_SmallFileLiteralDensity`
- `TestDeclarativeClassifier_YAMLJSONDefaultsSchema`
- `TestDeclarativeFileSet_CoversKnownSurfaces`（枚举 topology.go, skill/defaults.go, tool/defaults.go, agent/subagent.go, mcp/routes.go 等）

## 5.2 C0' grep gate Round-aware

文件：`internal/agent/agent.go::validateAnalyzerPrescanToolCall`

伪代码（见蓝图 §4.1 略）。关键：
- Round 1: 强制 `files_only=true`
- Round 2 + trigger: 允许 `files_only=false`，auto-inject `max_count=cfg.MaxMatchesPerCall`，校 per-call total bytes，校 per-Run total calls
- 任一限制越界 → `ToolViolation` 返回失败 `ToolResult`

## 5.3 Trigger gate

`internal/agent/analyzer.go`：

```go
func (a *analyzerEvaluator) classificationGrepTriggered(ctx *types.AgentContext, r1 *Round1Result) bool {
    if !cfg.Enabled { return false }
    if !ctx.Mutable.PrescanHasDeclarativeCandidate(classifier) { return false }
    return anyTrue(
        r1.LLMSubjectConfidence < cfg.MinLLMSubjectConf,
        r1.CueRuleOverridesLLMSubject,
        r1.QuestionKindVsShapeConflict,
        r1.EntityAxesContainsRelation(),
    )
}
```

触发则 `ctx.Mutable.SetClassificationGrepTriggered(true)`，这是 gate 的依据。

## 5.4 ClassificationObservations sidecar

`internal/types/context.go::MutableState` 新增字段：
```go
type MutableState struct {
    // ... existing ...
    ClassificationObservations []ClassificationObs   // NEW
}

type ClassificationObs struct {
    Pattern  string
    Path     string
    Matches  []GrepMatch
    Round    int
    TS       time.Time
}
```

写入点：`internal/agent/analyzer.go::postProcessToolResult`（在 BaseAgent 标准 tool result 写回之前拦截）：

```go
if ctx.Stage == StageAnalyze && tc.Tool == "grep" {
    filesOnly := extractBool(tc.Params, "files_only")
    if !filesOnly && ctx.Mutable.ClassificationGrepTriggered() {
        obs := parseGrepLineResult(result)
        ctx.Mutable.AppendClassificationObs(obs)
        result.SkipTurnAArtifactsWrite = true   // 标记不进 TurnAArtifacts
    }
}
```

新字段 `ToolResult.SkipTurnAArtifactsWrite` 默认 `false`。BaseAgent 在后续写 TurnAArtifacts 的地方检查此标记。

## 5.5 Reconciler 接入 buildAnalysisIR

`internal/agent/analyzer.go::buildAnalysisIR` step 1.5：

```go
if len(ctx.Mutable.ClassificationObservations) > 0 {
    reconcileFromObservations(rm, ctx.Mutable.ClassificationObservations, ir.Meta)
}
```

### reconcile 规则表

| 观察模式 | 推断 |
|---|---|
| 匹配行里 literal 以 `-skill` 结尾 | `AnswerSubject.Kind = skill_name`, `AnswerShape = value` |
| 匹配行里 literal 形如 `New{X}Agent` | `AnswerSubject.Kind = agent_name`, `AnswerShape = value` |
| 匹配行含 `{Field1: V1, Field2: V2}` 结构 | 确认 `entity_axes = [Field1 → Field2]` |
| 匹配行是 `@route(...)` / `app.get(...)` | `AnswerSubject.Kind = handler_route` |
| 所有匹配都在 `var X = map[..]..{...}` block 内 | `QuestionKind ∈ {registration, config_mapping}` |
| 匹配行是 `const X = "literal"` 或 `X = "literal"` 形式 | `AnswerShape = value` |

每条 reconcile 产生 log 进 `ir.Meta.ReconcileLog`。

## 5.6 Round 2 prompt 增量（仅 trigger on）

```
## Round 2 Option: Classification Grep

You may call grep with files_only=false (line-level matches) to verify the answer axis.
This is for classification confirmation only:
- At most 3 line-level calls; each capped at 20 matches; total ≤ 8 KB.
- Target declarative files (topology/defaults/registry/routes/wire).
- Matches will NOT appear in downstream evidence or citation whitelist — they feed classification only.

After verifying, call emit_analysis with refined subject.kind / shape / question_kind.
```

## 5.7 测试

- `TestClassificationGrep_Round1StillRejectsFilesOnlyFalse`
- `TestClassificationGrep_Round2RequiresTrigger`
- `TestClassificationGrep_ObservationsNotInTurnAArtifacts`
- `TestClassificationGrep_ObservationsNotInReadSet`  (CGEC G1 whitelist 不受影响)
- `TestClassificationGrep_BudgetEnforced`
- `TestReconcile_SkillNameFromDashSkillSuffix`
- `TestReconcile_AgentNameFromNewAgentFuncForm`
- `TestReconcile_ProducesReconcileLog`
- 端到端：`TestE2E_DefaultSkillQuestion_Recovers`（本 bug）

---

# Part VI · Ranker + LLM 防护（G6 组）

## 6.1 R1 DeclarativeBoost

文件：`internal/agent/keyword_search.go::keywordFileScore`

```go
// 现有
score = base + sum(keyword_match_scores) + repoMapRankBonus

// 扩展
kind, confidence := classifier.ClassifyPath(filePath)
if kind != declarative.KindNone && shouldApplyBoost(rm) {
    score += classifier.BoostFor(kind) * confidence
}

func shouldApplyBoost(rm *types.RequestModel) bool {
    return rm.QuestionKind == "registration" ||
           rm.QuestionKind == "config_mapping" ||
           rm.QuestionKind == "call_chain" ||
           rm.AnswerSubject.Kind == "skill_name" ||
           rm.AnswerSubject.Kind == "agent_name" ||
           rm.AnswerSubject.Kind == "handler_route" ||
           rm.AnswerSubject.Kind == "config_key" ||
           rm.AnswerSubject.Kind == "function_name"
}
```

同时 `internal/tool/repomap/retrieve/rank.go::RankGraph` 加等效分支（repoMap 路径也要受益）。

## 6.2 R2 auto-keywords

文件：`internal/agent/analyzer.go::buildAnalysisIR` step 2 后：

```go
if shouldApplyBoost(rm) {
    rm.Keywords = appendUnique(rm.Keywords, "topology", "registry", "defaults")
}
```

`rm.Keywords` 增量会被 explore 下游的 keyword_search 消费，间接 boost ranker。

## 6.3 R3 axis-aware chain demote

文件：`internal/agent/explorer_erm.go::scoreChain` / `demoteChain`

```go
func axisAwareDemote(chain Chain, primaryEntity string, axes []string) bool {
    if len(axes) == 0 { return false }
    // chain 终端 literal == primary entity name → self-reference in relational question
    if chain.TerminalLiteral() == primaryEntity && !strings.Contains(axes[0], "self") {
        return true
    }
    return false
}

// 使用点
if axisAwareDemote(chain, rm.PrimaryEntity(), rm.EntityAxes) {
    chain.Score -= axisDemoteFactor
    ec.AppendViolation(contract.Violation{
        Kind: ViolChainDemoted,
        SuspectedRoot: SuspectedRoot{"answer_subject.kind", "self-ref chain in relational question", 0.8},
        ...
    })
}
```

## 6.4 R4 evidence self-ref 入口过滤

文件：`internal/tool/emit_evidence.go::Execute`

```go
for _, item := range params.Items {
    if isSelfRefEvidence(item, rm) {
        // 不 reject 整个 emit，单条标 salience=trap
        item.Salience = types.SalienceTrap
        ec.AppendViolation(contract.Violation{
            Kind: ViolSelfRefLiteral,
            SuspectedRoot: SuspectedRoot{"answer_subject.kind", "evidence subject=primary_entity, predicate=returns, anchor=self-name", 0.75},
            ...
        })
    }
}

func isSelfRefEvidence(item EvidenceItem, rm *types.RequestModel) bool {
    return item.Subject == rm.PrimaryEntity() &&
           item.Predicate == "returns" &&
           strings.Contains(item.Snippet, `"`+rm.PrimaryEntity()+`"`)
}
```

`SalienceTrap` 的 evidence 仍进 bank 但不进 top-N，extractor/finalizer 看不到。

## 6.5 C2 Finalizer shape prompt 顶级前置

文件：`internal/skill/answer_document_skill.go`

现有 system prompt 里 "Required Shape" 信息在末尾 retry hint 里才出现。改为开头：

```
## CRITICAL CONTRACT (read first)

Target answer shape: **{{.TargetShape}}**. 
- Emitting any other shape will be rejected without retry budget consumption.
- For shape=value: emit `value: { literal: "...", citation_ref: N }` with citation_ref ≥ 0.
- For shape=boolean: emit `boolean: { decision: true|false, citation_ref: N }`.
- {...其他 shape 规则}

Target answer subject: **{{.TargetSubjectKind}}** (e.g., {{.SubjectExamples}}).

{{ rest of system prompt }}
```

## 6.6 C3 Extractor self-ref negative

文件：`internal/skill/extract_skill.go` 系统 prompt 顶级加：

```
## Self-Reference Trap (anti-pattern)

If a candidate answer literal equals the question's primary entity name, it is self-reference, NOT an answer.

Example: question asks "what skill does the {X} agent use by default?"
- Primary entity: X
- Wrong: answer literal = "X" (self-reference — X is the question subject, not its property)
- Correct: answer literal = "X-skill" or some OTHER identifier mapped to X

Never emit answer_symbol.items[i].name where name == primary_entity_name.
```

## 6.7 测试

- `TestR1_DeclarativeBoostAppliedForRegistrationQuestion`
- `TestR1_NoBoostForMechanismQuestion`
- `TestR2_AutoKeywordsAppendedForConfigMapping`
- `TestR3_SelfRefChainDemoted`
- `TestR3_NonSelfRefChainUnaffected`
- `TestR4_SelfRefEvidenceMarkedTrap`
- `TestR4_NormalEvidenceUnaffected`
- `TestC2_FinalizerPromptContainsTargetShapeAtTop`
- `TestC3_ExtractorPromptContainsSelfRefNegative`

---

# Part VII · 反馈路由 + 清理（G7 组）

## 7.1 R5 D2 → expand_search 反馈

文件：`internal/orchestrator/cgec_enforcers.go::recordGhostAnchor`

```go
func (e *Enforcer) recordGhostAnchor(file string, origin string) {
    e.ghostAnchorCount[file]++
    
    // 现有：skip + log
    e.logSkip(file, origin)
    
    // F1: write violation ledger
    e.closure.AppendViolation(contract.Violation{
        Kind: ViolGhostAnchor,
        SuspectedRoot: SuspectedRoot{"ScannedSet", "chain anchored outside scan", 0.70},
        EvidenceRefs: []string{file, origin},
        ...
    })
    
    // R5: 聚合阈值
    if e.ghostAnchorCount[file] >= cfg.ExpandSearchThreshold && !e.expandedSearchFiles[file] {
        if fileExistsInRepo(file) && !e.closure.ScannedSetContains(file) {
            e.closure.AppendToScannedSet(file, types.ScannedSetOriginGhostAnchorPromote)
            e.expandedSearchFiles[file] = true  // 标脏，后续不再 promote 同一 file
            e.closure.AppendRepair(types.RepairDirective{
                Kind: types.RepairExpandSearch,  // 复用 session 10 已活化的 Kind
                Target: file,
                Reason: fmt.Sprintf("ghost anchor × %d", e.ghostAnchorCount[file]),
            })
        }
    }
}
```

`ScannedSetOriginGhostAnchorPromote` 是新增 origin 标签。

## 7.2 R6 forced-read 去重 + 优先级

文件：`internal/orchestrator/cgec_enforcers.go::E2ForcedRead`

```go
func (e *Enforcer) E2ForcedRead(candidates []string) []string {
    // R6 dedup
    seen := make(map[string]bool)
    unique := []string{}
    for _, f := range candidates {
        if e.forcedReadHistory[f] || seen[f] { continue }
        seen[f] = true
        unique = append(unique, f)
    }
    
    // R6 priority: declarative files first
    sort.SliceStable(unique, func(i, j int) bool {
        ki, _ := classifier.ClassifyPath(unique[i])
        kj, _ := classifier.ClassifyPath(unique[j])
        return classifier.BoostFor(ki) > classifier.BoostFor(kj)
    })
    
    // 标 history
    for _, f := range unique {
        e.forcedReadHistory[f] = true
    }
    return unique
}
```

## 7.3 C4 shape reject-no-rescue（删 B2a 救援）

文件：`internal/tool/emit_answer_document.go::shapeSwap`

```diff
- if b2aCanCorrect {
-     result.Shape = targetShape
-     scrub unused fields
-     return nil  // rescued
- }
  return &ToolResult{
      Ok: false,
      Summary: fmt.Sprintf("shape mismatch: got %s, contract requires %s", result.Shape, targetShape),
  }
```

F4 HintComposer 根据这个 Violation 合成精准 retry hint 让 LLM 自己改。

**注意 ViolShapeSwap 仍然写 ledger 作审计**（即使不再 rescue），方便观察 shape 错误频率。

## 7.4 C5 G1 literal form check

文件：`internal/tool/emit_answer_document.go::dryRunG1`

现有只查 citation in ReadSet。新增：

```go
if result.Shape == "value" && result.Value.Literal != "" {
    expected := formRuleFor(ir.AnswerSubject.Kind)
    if !expected.MatchString(result.Value.Literal) {
        return &DryRunResult{
            Ok: false,
            Violations: []contract.Violation{{
                Kind: ViolLiteralFormFailed,
                Detail: fmt.Sprintf("literal '%s' does not match kind=%s form %s",
                    result.Value.Literal, ir.AnswerSubject.Kind, expected),
                SuspectedRoot: SuspectedRoot{"answer_subject.kind", ...},
            }},
        }
    }
}

var formRules = map[AnswerSubjectKind]*regexp.Regexp{
    SubjectSkillName:   regexp.MustCompile(`-skill$`),
    SubjectAgentName:   regexp.MustCompile(`^(?:[a-z]+|New[A-Z][A-Za-z]*Agent)$`),
    SubjectConfigKey:   regexp.MustCompile(`[a-z_][a-z_0-9]*(\.[a-z_][a-z_0-9]*)*`),
    SubjectHandlerRoute: regexp.MustCompile(`^/|^@`),
    SubjectFunctionName: regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`),
    // ...
}
```

## 7.5 删除 C1 手写级联代码

E1 cue override 之后目前手写：
```go
if subjectOverridden && shape == "config_value" {
    shape = "value"
}
```
删除。由 F3 IRPatchEngine 统一处理（E1 writes ViolCueRuleOverride → F2 aggregates conf=0.9 → F3 patches AnswerSubject.Kind → RecomputeShapeDerivedFields → shape 自动更新）。

**这是 F3 吸收 C1 的直接证据**。

## 7.6 测试

- `TestR5_GhostAnchorAggregationPromotesFile`
- `TestR5_GhostAnchorDedupPreventsDoublePromote`
- `TestR6_ForcedReadDedupPerFile`
- `TestR6_ForcedReadDeclarativePriority`
- `TestC4_ShapeMismatchNoRescueJustReject`
- `TestC5_LiteralFormCheckSkillNameSuffix`
- `TestC5_LiteralFormCheckFunctionName`
- `TestC1Absorption_E1OverrideTriggersF3Patch`
- 端到端：`TestE2E_FullBugReproAllSevenGroups`

---

# 附录 A · 配置旋钮总表

```yaml
# ============ F1 ViolationLedger ============
violation_ledger_enabled: true
violation_ledger_min_confidence: 0.5

# ============ F2 RootCauseAggregator ============
aggregator_patch_threshold: 0.70
aggregator_hint_threshold: 0.50
aggregator_min_event_count: 3

# ============ F3 IRPatchEngine ============
patcher_max_patches_per_run: 4
patcher_max_patches_per_field: 2
patcher_audit_log_enabled: true

# ============ F4 HintComposer ============
hint_composer_strict_mode: false              # G3 期初期；稳定后打开 true
hint_composer_max_allowed_set: 10
hint_composer_max_forbidden_patterns: 5

# ============ F5 ViolationBudget ============
violation_budget_min_retry_yield: 1
violation_budget_yield_kill_stage: true
violation_budget_fail_loud_enabled: true

# ============ C6 RetryBudget by Kind ============
retry_budget_shape_violation: 1
retry_budget_citation_violation: 3
retry_budget_literal_form: 2
retry_budget_ghost_anchor: 2
retry_budget_self_ref: 2
retry_budget_other: 1

# ============ C0' ClassificationGrep ============
analysis_classification_grep_enabled: true
analysis_classification_grep_max_calls: 3
analysis_classification_grep_max_matches_per_call: 20
analysis_classification_grep_max_total_bytes: 8192
analysis_classification_grep_min_llm_subject_conf: 0.8

# ============ Declarative Classifier（R1/C0 共享） ============
declarative_filename_patterns:
  - topology
  - defaults
  - registry
  - routes
  - wire
  - init
  - manifest
  - schema
  - enum
declarative_max_lines_for_small: 60
declarative_literal_block_ratio: 0.6
declarative_boost_topology: 15
declarative_boost_registry: 12
declarative_boost_defaults: 10
declarative_boost_routes: 8
declarative_boost_wire: 6
declarative_boost_manifest: 6

# ============ R5 expand_search ============
ghost_anchor_expand_search_threshold: 3

# ============ R6 forced-read ============
forced_read_per_file_limit: 1
```

---

# 附录 B · 数据类型总览（新增 / 扩展）

| 类型 | 位置 | 操作 |
|---|---|---|
| `contract.Violation.SuspectedRoot` | `internal/analysis/contract/checker.go` | 扩展 |
| `contract.ViolationKind` | 同上 | 加 6 个 enum |
| `EvidenceClosure.violations` | `internal/types/evidence_closure.go` | 扩展 |
| `ClassificationObs` | `internal/types/context.go` | 新增 |
| `MutableState.ClassificationObservations` | 同上 | 新增 |
| `ToolResult.SkipTurnAArtifactsWrite` | `internal/types/tool.go` | 新增 bool |
| `aggregator.Aggregator` | `internal/analysis/aggregator/` | 新子包 |
| `aggregator.FieldHeat` | 同上 | 新增 |
| `patcher.Engine` | `internal/analysis/patcher/` | 新子包 |
| `patcher.PatchRequest` | 同上 | 新增 |
| `patcher.PatchRecord` | 同上 | 新增 |
| `hint.Composer` | `internal/analysis/hint/` | 新子包 |
| `hint.Hint` | 同上 | 新增 |
| `hint.Allowed` | 同上 | 新增 |
| `declarative.Classifier` | `internal/analysis/declarative/` | 新子包 |
| `declarative.Kind` | 同上 | 新增 enum |
| `AnalysisIR.Meta.PatchLog` | `internal/types/analysis_ir.go` | 扩展 |
| `AnalysisIR.Meta.ReconcileLog` | 同上 | 扩展 |
| `SalienceTrap` | `internal/types/evidence.go` | 新增 enum 值 |
| `RepairExpandSearch` origin field | `internal/types/repair.go` | 扩展 |
| `PipelineSettings.ViolationBudget` | `internal/types/config.go` | 扩展 |
| `PipelineSettings.RetryBudgetByKind` | 同上 | 扩展 |
| `graphState.retryUsedByKind` | `internal/orchestrator/scheduler.go` | 扩展 |
| `graphState.YieldKillCount` / `TopSuspected*` | 同上 | 新增 |

---

# 附录 C · 验收标准

### 核心 bug 10 次验收

同一问题 `explorer agent 默认用哪个 skill?`：

- [ ] 10/10 次答案正确（`explore-skill` + citation 指向 `topology.go:19` 或 `defaults.go:14`）
- [ ] 10/10 次 `forced_reads ≤ 2`
- [ ] 10/10 次 `shape_swap = 0`（C4 删除救援后此指标应永远为 0，除非 ledger 审计）
- [ ] 10/10 次 `pre_complete_downgrades = 0`
- [ ] 10/10 次 LLM 总耗时 < 20 s
- [ ] hint 里出现 `what_system_did: "analyzer Round 2 grep'd topology.go ... (trigger: CueRuleOverridesLLMSubject)"`

### 泛化验收（5 个同类问题，都走 C0 路径）

- [ ] `codrax 里注册了哪些内置工具?` — 读 `internal/tool/defaults.go`
- [ ] `哪个 agent 绑定 analysis-skill?` — 读 `topology.go`
- [ ] `extractor 的默认 skill?` — 读 `topology.go`
- [ ] `有哪些 DeclarativeKind?` — 读 `declarative/classifier.go`
- [ ] `MCP 路由表在哪?` — 读 `routes.go` 类

### 反馈机制可观测性

- [ ] CGEC summary 输出 `violations_by_field` + `top_suspected_root`
- [ ] F3 patch 历史写入 `ir.Meta.PatchLog`，数量 ≤ cfg.MaxPatchesPerRun
- [ ] F4 在 strict_mode=true 下未触发 fallback

### 空转消除

- [ ] `TestViolationBudget_YieldKillStopsLoop` 通过
- [ ] 退化构造问题（无答案源）能在合理时间内 fail-loud，不无限空转
- [ ] `TestRetryBudgetByKind_*` 全通过

### 现有测试无 regression

- [ ] `make test` 全通过
- [ ] CGEC 既有测试套件不挂（I1–I4 不变量保留）
- [ ] `TestAllRepairKindsHaveProducer` / `TestEvidenceClosureAllFieldsHaveConsumer` 扩展后通过

---

# 附录 D · 风险 + 护栏对照

| 风险 | 护栏 |
|---|---|
| F3 IR mutation 破坏 "analyzer is sole writer" invariant | 白名单字段 + audit trail + 幂等 + 单调 + per-Run budget；PatchLog 可审计；TaskGraph/Hypotheses 列表结构仍不可变 |
| F4 strict_mode 过早打开导致 retry hint 全 fail | 灰度：G3 期 false + 1 周 shadow；统计 ErrIncompleteHint 归零后打开 |
| C4 删 B2a 救援导致本来能修的 shape 失败 | C2 shape prompt 前置 + F4 精准 hint 让 LLM 自己修；单元测试覆盖每种 shape 的 exact_fix 指令 |
| R3/R4 self-ref 规则误判合理场景 | primary_entity 精确匹配；只对 relational axis（entity_axes 非空）生效；对 mechanism/explanation 问题关闭 |
| R5 ghost anchor 聚合 promote 一个不相关文件 | 文件必须真实存在 + threshold ≥ 3 + 每文件只 promote 一次 + 带 origin 标签可审计 |
| C0' grep 被 LLM 滥用 | 四重硬 gate（Round-gate / max-count / total-bytes / trigger gate），任一越界 tool fail |
| 所有新组件同时上线导致 debug 困难 | 每组件有独立 feature flag；可单独 disable 回到旧行为；G1-G7 分组 feature branch merge |
| 配置旋钮过多造成运维负担 | 所有旋钮都有合理默认值；`codrax.yaml` 里配置段按功能分组命名；文档同步更新 |

---

# 附录 E · 开发工作量估算

| 组 | 产品代码 | 测试代码 | 子包 |
|---|---:|---:|---:|
| G1 F1 ViolationLedger | 400 | 200 | — |
| G2 F2 + F3 | 1300 | 800 | 2 |
| G3 F4 HintComposer | 600 | 400 | 1 |
| G4 F5 + C6 | 350 | 250 | — |
| G5 C0' + Declarative Classifier | 500 | 400 | 1 |
| G6 R1/R2/R3/R4 + C2/C3 | 700 | 500 | — |
| G7 R5/R6 + C4/C5 + 清理 | 400 | 300 | — |
| **合计** | **~4250** | **~2850** | **4 新子包** |

**对比 session 10**：+1500 / +700 / 9 组一次 ship 成功。本方案 ~4.5× 规模，组件边界清晰、并行开发友好。

---

**本方案为 session 11 的施工依据。下一步进入 PR-by-PR 施工图（每组的 commit changelist + 测试命名 + reviewer 关注点）。**
