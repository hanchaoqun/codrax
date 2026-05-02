# Semantic Surface Contract — 7 阶段逐 Phase 适配方案

**关联文档**:
- 输入: [semantic_surface_contract_input.md](./semantic_surface_contract_input.md)
- 上层适配总纲: [semantic_surface_contract_adapted.md](./semantic_surface_contract_adapted.md)
- 当前 main: `33b3209` (Plan D + Plan E G2/G3 已落地;Plan E G1 已撤;grounded-reviewer 已撤)

本文档把输入文档第 12 节的 Phase 0 - Phase 6 共 7 个阶段,逐一对照真实代码写出**完整适配方案**。每个 Phase 包含:

- **目标**(原文 vs 适配)
- **现状盘点**(代码已具备 / 缺什么)
- **接入点**(精确到文件/函数/行号区间)
- **新增 / 修改 / 删除 LOC 估算**
- **测试需求**
- **入场前置条件 + 退场标准**
- **风险评估**
- **与上下游 Phase 的依赖关系**

---

## Phase 0 — 观测层(Capability Projection)

### 0.1 目标

| 输入文档原文 | 适配重述 |
|---|---|
| 新增 `ProjectEvidenceClaimCapabilities(...)` + trace/debug artifact 输出。先看 capability 投影是否稳定,不引入行为变化。 | 在不修改任何 emit/render/gate 行为的前提下,新增 `ClaimFormOf` 纯函数把已有 `EvidenceItem` 投影到 `ClaimForm`,并在已有 trace 通道增加 1 行可观测输出。 |

### 0.2 现状盘点

| 假设 | 真实代码 |
|---|---|
| 需要新建 `EvidenceClaimCapability` 结构存 | **不需要** — `EvidenceItem` 已有 `AnchorKind / ContextRole / DiagramRole / Origin / Scope / Authority` 6 个 typed 字段,完全够投影 |
| 需要新增 trace artifact | **不需要** — `internal/agent/explorer.go` 已有 `[trace/fev]` debug channel,加一行即可 |
| 需要新建消费者 | **零消费者** — 本 Phase 仅观测,无消费 |

### 0.3 接入点

**新文件**: `internal/types/claim_form.go` (~80 LOC)
```go
// ClaimForm is a deterministic projection of EvidenceItem.
// Pure function — never read by LLM, only by gate validators
// (Phase 4) and plan compilers (Phase 1).
type ClaimForm string
const (
    ClaimDefinitionFact      ClaimForm = "definition_fact"
    ClaimCallEdge            ClaimForm = "call_edge"
    ClaimGuardCondition      ClaimForm = "guard_condition"
    ClaimAssignmentFact      ClaimForm = "assignment_fact"
    ClaimReturnFact          ClaimForm = "return_fact"
    ClaimAbsenceFact         ClaimForm = "absence_fact"
    ClaimPrecedenceRole      ClaimForm = "precedence_role"
    ClaimExternalObservation ClaimForm = "external_observation"
    ClaimImportEdge          ClaimForm = "import_edge"
    ClaimUnknown             ClaimForm = ""
)

func ClaimFormOf(item EvidenceItem) ClaimForm { ... }
```

**修改**: `internal/agent/explorer.go` 在已有 `[trace/fev]` 输出处加一行
```go
logging.Debug("[trace/fev] %d producer=%q src=%s subj=%q kind=%s claim_form=%s grounding=%s",
    i, item.Producer, item.Source, item.Subject, item.Kind,
    types.ClaimFormOf(item),  // ← NEW
    item.GroundingStatus)
```

**新文件**: `internal/types/claim_form_test.go` (~120 LOC)
- 矩阵测试: 9 种 ClaimForm × 各种 (AnchorKind, Origin, Scope, ContextRole) 组合
- back-compat: 空 EvidenceItem → ClaimUnknown
- 优先级: Origin=Log/Perf 始终 → ExternalObservation,无关 AnchorKind

### 0.4 LOC 估算

```
+ 80 LOC  internal/types/claim_form.go (NEW)
+ 120 LOC internal/types/claim_form_test.go (NEW)
+ 1 line  internal/agent/explorer.go (MODIFY trace log)
─────────
+ ~200 LOC,15 测试用例
```

### 0.5 入场前置条件

- 所有现有 46 包测试 PASS
- 对齐 `ClaimForm` 投影矩阵(本文档 §0.3 实例代码)— 这是 Phase 4+ 的 contract base,定下后稳定

### 0.6 退场标准

- `ClaimFormOf` 在 ≥3 次真实 eval 上输出稳定(trace 行可读、无 panic)
- 单元测试覆盖率 ≥80%
- 无任何 ViolationKind 因 Phase 0 触发(Phase 0 不开 gate)

### 0.7 风险

- **极低**。纯新增 + 1 行 debug log,无消费者。
- 唯一风险: ClaimFormOf 投影矩阵设计有缺陷 → 矩阵测试覆盖。

### 0.8 与下游 Phase 依赖

- Phase 1 用 `ClaimFormOf` 编译 facet plan
- Phase 4 用 `ClaimFormOf` 跑 hard gate
- 没有 Phase 0,Phase 1+ 无法启动

---

## Phase 1 — 编译 AnswerFacetPlan(不强 gate)

### 1.1 目标

| 输入文档原文 | 适配重述 |
|---|---|
| 在 `AnswerSurfacePlan` 中加入 `FacetPlan`。诊断 PASS 但答案薄,诊断 claim-form mismatch。不 hard-fail。 | 不新建顶层 `AnswerFacetPlan` 结构,改为 `AnswerSurfacePlan.FacetCoverage *FacetCoverageContract` 字段。复用现有 `BuildAnswerSurfacePlan` 编译入口,无新 producer。 |

### 1.2 现状盘点

| 假设 | 真实代码 |
|---|---|
| `AnswerSurfacePlan` 存在编译入口 | ✅ `internal/types/answer_surface_plan.go:1079` `BuildAnswerSurfacePlan(...)` |
| `AnswerSurfacePlan` 有 30+ 编译字段 | ✅ `StepBackbone / ExactResolution / CapabilitySurface / DriftBoundedSurfaceItems / SurfaceEvidence / ConfigTraceDiagramAnchors` 等 |
| 持久化通道 | ✅ `Mutable.SetAnswerSurfacePlan` / `Mutable.AnswerSurfacePlan()` |
| Facet 编译需要的输入 | ✅ 全部齐备:`Intent / Scenario / AnswerSubject / PredicateAxis / QuestionStructure / ExactResolution / DiagramContract / SurfaceEvidence / DriftAnchors` |

### 1.3 接入点

**新文件**: `internal/types/facet_plan.go` (~150 LOC)
```go
type QuestionFamily string
const (
    QFRootCauseTrace   QuestionFamily = "root_cause_trace"
    QFConfigPrecedence QuestionFamily = "config_precedence"
    QFRoleLookup       QuestionFamily = "role_lookup"
    QFCallChain        QuestionFamily = "call_chain"
    QFEnumeration      QuestionFamily = "enumeration"
    QFArchitecture     QuestionFamily = "architecture"
    QFGeneric          QuestionFamily = "generic"
)

type AnswerFacetKind string
const (
    FacetObservedArtifactFact    AnswerFacetKind = "observed_artifact_fact"
    FacetCurrentCodePath         AnswerFacetKind = "current_code_path"
    FacetNearestMechanism        AnswerFacetKind = "nearest_mechanism"
    FacetUncertaintyBoundary     AnswerFacetKind = "uncertainty_boundary"
    FacetConfigPrecedenceRole    AnswerFacetKind = "config_precedence_role"
    FacetResolvedLiteralOrSymbol AnswerFacetKind = "resolved_literal_or_symbol"
    FacetEnumerationItem         AnswerFacetKind = "enumeration_item"
    FacetBucketLabel             AnswerFacetKind = "bucket_label"
    FacetPrincipalPathEdge       AnswerFacetKind = "principal_path_edge"
    FacetBranchGuard             AnswerFacetKind = "branch_guard"
    FacetComponentRelation       AnswerFacetKind = "component_relation"
    FacetDiagramSpine            AnswerFacetKind = "diagram_spine"
)

type FacetRequiredness string
const (
    FacetHardRequired FacetRequiredness = "hard"
    FacetSoftRequired FacetRequiredness = "soft"
    FacetOptional     FacetRequiredness = "optional"
)

type FacetRequirement struct {
    Kind            AnswerFacetKind
    Required        FacetRequiredness
    AcceptableForms []ClaimForm     // ClaimForm whitelist
    SourceCandidate []string        // EvidenceItem.IDs that could fill this facet
}

type FacetCoverageContract struct {
    Family   QuestionFamily
    Required []FacetRequirement
    Optional []FacetRequirement
}

func ResolveQuestionFamily(rm RequestModel) QuestionFamily { ... }
func CompileFacetCoverage(rm RequestModel, surface []EvidenceItem) *FacetCoverageContract { ... }
```

**修改**: `internal/types/answer_surface_plan.go` AnswerSurfacePlan struct 加字段
```go
type AnswerSurfacePlan struct {
    // ... existing 30+ fields ...
    FacetCoverage *FacetCoverageContract  // ← NEW (Phase 1)
}
```

**修改**: `BuildAnswerSurfacePlan` 末尾加一行编译
```go
plan.FacetCoverage = CompileFacetCoverage(rm, plan.SurfaceEvidence)
```

**修改**: `internal/agent/explorer.go` 在 trace summary 加 facet plan dump (debug only)

**新文件**: `internal/types/facet_plan_test.go` (~250 LOC)
- 6 个 QuestionFamily 各配置一个 fixture
- ResolveQuestionFamily 决定矩阵测试
- CompileFacetCoverage 输出 snapshot test

### 1.4 LOC 估算

```
+ 150 LOC internal/types/facet_plan.go (NEW)
+ 250 LOC internal/types/facet_plan_test.go (NEW)
+ 5 LOC   internal/types/answer_surface_plan.go (MODIFY: add field + 1 compile call)
+ 10 LOC  internal/agent/explorer.go (MODIFY: trace dump)
─────────
+ ~415 LOC,18 测试
```

### 1.5 入场前置条件

- Phase 0 已 ship + 无 trace 异常 ≥3 真实 eval

### 1.6 退场标准

- 6 个 QuestionFamily 在真实 eval 中 ResolveQuestionFamily 输出稳定(snapshot 测试 PASS)
- FacetCoverage trace 在 ≥10 个不同 eval case 上无 panic
- Optional facet 缺失统计可观察(Phase 5 准备数据)

### 1.7 风险

- **低**。FacetCoverage 字段是新增,旧路径不读;ResolveQuestionFamily 是 deterministic 投影,无 LLM 输入。
- 唯一风险: Family 划分粒度过粗或过细 → 跑 eval 观察 + 调整。

### 1.8 与上下游依赖

- 依赖 Phase 0 的 `ClaimFormOf`(用于 SourceCandidate 反查)
- Phase 2 prompt section 消费 `FacetCoverage`
- Phase 4 hard gate 消费 `FacetCoverage`

---

## Phase 2 — Finalizer Prompt 消费 Facet

### 2.1 目标

| 输入文档原文 | 适配重述 |
|---|---|
| 在 finalizer prompt 加新 section: Required Answer Facets / Allowed Claim Forms by facet / Principal-support-prose-diagram / Required uncertainty boundary / Expected richness candidates。 | 加 1 个 `renderAnswerDocFacetCoverage` 函数,接进现有 `BuildInitialInstruction` 的 section 队列(同 13 个 `renderAnswerDocXxx` 兄弟)。LLM 见到的是抽象 facet 名 + 接受的 ClaimForm,不见 Go 内部名。 |

### 2.2 现状盘点

| 假设 | 真实代码 |
|---|---|
| Finalizer prompt 有 section 队列 | ✅ `internal/agent/answer_document_evaluator.go::BuildInitialInstruction` 已串 13 个 `renderAnswerDocXxx`(Diagram Contract / Capability Surface / Exact Resolution / Step Backbone / Requested Set Boundary / Submission Checklist / etc.) |
| 已有 prompt 红线 | ✅ 7 条 `feedback_*.md`,glossary lint 强制无内部名 |

### 2.3 接入点

**修改**: `internal/agent/answer_document_evaluator.go`

新增函数 `renderAnswerDocFacetCoverage(ctx) string`,接 finalizer prompt section 队列:
```go
// 现有:
b.WriteString(renderAnswerDocDiagramContract(dc))
// ... 13 个兄弟 section ...

// NEW:
b.WriteString(renderAnswerDocFacetCoverage(ctx))
```

`renderAnswerDocFacetCoverage` 输出形如(LLM-facing):
```markdown
## Required Answer Facets

This question requires the following facets in the rendered answer:

- **Resolved literal or symbol** (HARD): the answer must name the specific 
  identifier the question is about. Acceptable evidence: `definition`, 
  `assignment` anchored at the resolved file:line.

- **Configuration precedence role** (HARD when applicable): the answer must
  cover each grounded precedence role (default / config-file / runtime / override) 
  exactly once. Each role's evidence must come from a citation matching that 
  layer.

- **Uncertainty boundary** (SOFT): when evidence is incomplete, name the 
  scope of what was searched and what remained unverified. Do not silently 
  hedge; explicitly write "the search covered <scope>; <X> was not found 
  within that scope".

Optional richness facets (include when grounded evidence supports them):
- ...
```

**Prompt 红线遵守**:
- 不出现 `FacetCoverageContract / ClaimFormOf` 等 Go 名
- 不出现 case-specific 例子(s7a/m2a/s3a)
- ≤200 词每段
- glossary lint 验证

**修改**: `internal/skill/defaults_test.go` 加 prompt-pin 测试

### 2.4 LOC 估算

```
+ 200 LOC internal/agent/answer_document_evaluator.go (MODIFY: add render func)
+ 100 LOC internal/agent/answer_document_evaluator_test.go (extend)
─────────
+ ~300 LOC,8 测试
```

### 2.5 入场前置条件

- Phase 1 已 ship + FacetCoverage 在真实 eval 上稳定输出

### 2.6 退场标准

- 真实 eval 上观察:LLM 在 prompt 引导下产出的答案,facet 覆盖率(Phase 5 telemetry)较 Phase 1 提升 ≥20%
- 无新增 false positive 在已 PASS case 上(s1a / s5a / m1a 仍 PASS)
- glossary lint PASS

### 2.7 风险

- **中**。Prompt 改动可能让 LLM 行为漂移。
- 缓解: Phase 2 不开 hard gate(那是 Phase 4),只是 prompt 引导;LLM 不听最多答案变化,不会 reject。

### 2.8 与上下游依赖

- 依赖 Phase 1 `FacetCoverageContract`
- Phase 3 ClaimUse 注解后,reviewer / validator 能与 prompt 闭环

---

## Phase 3 — AnswerDocument 增量注解

### 3.1 目标

| 输入文档原文 | 适配重述 |
|---|---|
| 在现有 shape payload 上新增 annotation `RenderedClaimUse`,可放在 summary/step/symbol/boolean rationale/value rationale。复用现有 renderer。 | 加 `AnswerStep.ClaimUse *RenderedClaimUse` / `AnswerSymbol.ClaimUse` / `AnswerValue.ClaimUse`(全部 optional 指针),emit_answer_document schema 加 optional `claim_use` 字段。LLM 不填则 validator 用 `ClaimFormOf(referenced_evidence)` 推断。 |

### 3.2 现状盘点

| 假设 | 真实代码 |
|---|---|
| `AnswerStep / AnswerSymbol / AnswerValue` 是 typed struct | ✅ `internal/types/answer_document.go` |
| 已经有过 schema 增量(Plan D Kind 字段) | ✅ Plan D 已加 `AnswerStep.Kind`,本 Phase 是同样 pattern |
| emit_answer_document 已有 30+ validator | ✅ `internal/tool/emit_answer_document.go` |

### 3.3 接入点

**修改**: `internal/types/answer_document.go`
```go
// NEW
type RenderedClaimUse struct {
    FacetID     string      `json:"facet_id,omitempty"`
    EvidenceID  string      `json:"evidence_id,omitempty"`
    ClaimForm   ClaimForm   `json:"claim_form,omitempty"`
    SurfaceRole SurfaceRole `json:"surface_role,omitempty"`
}

type SurfaceRole string
const (
    SurfacePrincipal   SurfaceRole = "principal"
    SurfaceSupport     SurfaceRole = "support"
    SurfaceProseOnly   SurfaceRole = "prose_only"
    SurfaceDiagramOnly SurfaceRole = "diagram_only"
)

// EXTEND
type AnswerStep struct {
    // existing fields ...
    Kind     AnswerStepKind    `json:"kind,omitempty"`
    ClaimUse *RenderedClaimUse `json:"claim_use,omitempty"` // NEW
}
type AnswerSymbol struct {
    // existing fields ...
    ClaimUse *RenderedClaimUse `json:"claim_use,omitempty"` // NEW
}
type AnswerValue struct {
    // existing fields ...
    ClaimUse *RenderedClaimUse `json:"claim_use,omitempty"` // NEW
}
```

**修改**: `internal/tool/emit_answer_document.go`
- Schema description 加 `claim_use` 字段说明(optional,LLM 可选填)
- `validateStep`/`validateSymbol`/`validateValueField` 不强制要求 ClaimUse(向前兼容)
- 但如 LLM 填了,validator 验证:
  - `ClaimUse.EvidenceID` 必须在 SurfaceEvidence 中存在
  - `ClaimUse.ClaimForm` 必须 == `ClaimFormOf(evidence)` (一致性)
  - `ClaimUse.SurfaceRole == principal` 时,evidence 必须 `CanSupportAsPrincipal()=true`

LLM 没填时,validator 自动调 `ClaimFormOf(referenced_evidence)` 推断填进 doc 用于 Phase 4 验证。

### 3.4 LOC 估算

```
+ 50 LOC  internal/types/answer_document.go (EXTEND struct + NormalizeSurfaceRole)
+ 80 LOC  internal/types/answer_document_test.go (EXTEND)
+ 60 LOC  internal/tool/emit_answer_document.go (schema description + 3 validator paths)
+ 100 LOC internal/tool/emit_answer_document_test.go (back-compat + LLM-emitted ClaimUse)
─────────
+ ~290 LOC,12 测试
```

### 3.5 入场前置条件

- Phase 2 已 ship + 真实 eval 上 LLM 接收 prompt 但答案稳定

### 3.6 退场标准

- 现有 Plan D Kind / Plan E G2 / G3 测试全 PASS(back-compat)
- `claim_use` 字段省略时,doc 渲染 byte-identical 到 pre-Phase-3
- `claim_use` 字段填时,validator 推断与 LLM 填的一致

### 3.7 风险

- **中**。Schema 改动 + 30+ 已有 validator 需要保持兼容。
- 缓解: 完全 optional 字段,无强制要求 LLM 填;参考 Plan D Kind 同样 pattern 的成功落地。

### 3.8 与上下游依赖

- 依赖 Phase 0 ClaimFormOf
- Phase 4 hard validator 消费 ClaimUse(若 LLM 填)或推断(若没填)

---

## Phase 4 — Validator 正式启用

### 4.1 目标

| 输入文档原文 | 适配重述 |
|---|---|
| 启用 5 个 validator: `validateFacetCoverage` / `validateClaimFormSupport` / `validateTemporalEscalation` / `validateAbsenceScopeBound` / `validateDiagramFacetGraph`。 | 启用 4 个(Temporal 由现有 `runAuthorityOverreachCheck` 覆盖,合并)。新增 3 个 ViolationKind(Facet 未覆盖 / ClaimForm 不支持 / Absence Scope 越界)。 |

### 4.2 现状盘点

| 假设 | 真实代码 |
|---|---|
| 已有 validator 接入点 | ✅ `internal/tool/emit_answer_document.go::Execute` 串 30+ `validate*` |
| 已有 ViolationKind 体系 | ✅ `internal/types/violation.go` 18 个 kind + soft/strict 分类 |
| 已有 fallback policy | ✅ `internal/orchestrator/fallback_policy.go` 5 target |
| Temporal 检查已有 | ✅ `runAuthorityOverreachCheck` 覆盖 historical→current 升级拦截 |
| Diagram 解析已有 | ✅ `validateSummaryDiagramGrounding` + render/mermaid Lib |

### 4.3 接入点

**新 ViolationKind**(`internal/types/violation.go`):
```go
ViolFacetUncovered      ViolationKind = "facet_uncovered"
ViolClaimFormUnsupported ViolationKind = "claim_form_unsupported"
ViolAbsenceScopeExceeded ViolationKind = "absence_scope_exceeded"
```

**默认 soft 分类**(继续放在 `defaultSoftKinds`):
- `ViolFacetUncovered`: SOFT(可 retry 但不 hard fail,因为 facet 缺失常常需要再投资)
- `ViolClaimFormUnsupported`: STRICT(明确语义违例,LLM 必须改)
- `ViolAbsenceScopeExceeded`: STRICT(safety-critical)

**Fallback policy**(`internal/orchestrator/fallback_policy.go`):
- `ViolFacetUncovered` → `BackToExplore`(facet 缺常需要补 evidence)
- `ViolClaimFormUnsupported` → `FinalizerOnly`(改 prose 即可)
- `ViolAbsenceScopeExceeded` → `BackToExtract`(extractor 重做 absence 范围)

**4 个新 validator**(全在 `internal/tool/emit_answer_document.go`):

#### a) `validateFacetCoverage(plan, doc) error`
- 输入:`AnswerSurfacePlan.FacetCoverage`(Phase 1 编译) + `doc.Steps/Symbols/Value/Boolean`
- 检查:每个 `FacetHardRequired` 的 facet 必须在 doc 的某个 ClaimUse(显式或推断)中被引用
- 失败:`ViolFacetUncovered` + 列出未覆盖的 facet 名

#### b) `validateClaimFormSupport(doc, surfaceEvidence) error`
- 对每个 doc.Step/Symbol/Value 的 ClaimUse(显式或推断):
  - 检查 `CanSupportClaim(evidence, claimUse.ClaimForm)`
  - 检查 `claimUse.SurfaceRole == Principal` 时 `CanSupportAsPrincipal(evidence)=true`
- 失败:`ViolClaimFormUnsupported` + 列出违例 step 索引 + evidence ID

#### c) `validateAbsenceScopeBound(doc, exact) error`
- 当 doc 含 absence 声明(prose 含 "no/none/不存在/未找到/absent" + cited Scope=Negative):
  - 必须有 `NegativePattern` 字段填充
  - prose 不得断言比 NegativePattern Scope 更宽的 absence
- 失败:`ViolAbsenceScopeExceeded`

#### d) `validateDiagramFacetGraph(doc, plan) error`
- 当 summary 含 mermaid block:
  - 解析出 nodes + edges
  - 每个 file-shaped node 必须 cited
  - 每个 edge(call/dispatch)必须有 evidence with `ClaimFormOf=ClaimCallEdge` 或 `ClaimImportEdge`
  - **新增 user-language check**: 节点 label 若不在 cited Quote 中出现,必须与 RequestModel.Language 匹配(s3a 修复)
- 失败:`ViolDiagramIdentifier`(复用现有)+ extended Detail

### 4.4 LOC 估算

```
+ 30 LOC  internal/types/violation.go (3 new ViolationKind)
+ 10 LOC  internal/orchestrator/fallback_policy.go (3 new mappings)
+ 250 LOC internal/tool/emit_answer_document.go (4 new validators)
+ 350 LOC internal/tool/emit_answer_document_test.go (per-validator + 集成)
+ 80 LOC  internal/orchestrator/contract_check.go (Wire ViolFacetUncovered into runAnswerShapeOracle for soft drift signal)
─────────
+ ~720 LOC,30 测试
```

### 4.5 入场前置条件

- Phase 3 已 ship + ClaimUse 字段在真实 eval 中 LLM 偶尔填(可观察一致性)
- Phase 1 FacetCoverage 在所有 6 QuestionFamily 输出稳定

### 4.6 退场标准

- s7a / m2a / s3a 三个本 session 已知瑕疵在 ≥3 次真实 eval 上被 catch
- 已 PASS 的 s1a / s5a / m1a 仍 PASS(无回归)
- 4 个 validator 单测各 ≥6 边界 case
- 新 ViolationKind 在 fallback policy 路径走通

### 4.7 风险

- **高**。这是首个 hard gate Phase。
- 风险点:
  1. `validateFacetCoverage` SOFT 分类可能误触很多 case
  2. `validateClaimFormSupport` 可能因 ClaimFormOf 投影 corner case 误伤
  3. `validateDiagramFacetGraph` 解析复杂,user-language check 启发式有边界
- 缓解:
  - Phase 4 启用前先在 Phase 1+2+3 的 trace 上观察 ≥2 周的"假设触发"率
  - 4 validator 分批启用,先 SOFT 后 STRICT
  - 退路: 单 validator 可关停 yaml 旋钮(类 `pipeline_self_consistency_*`)

### 4.8 与上下游依赖

- 依赖 Phase 0/1/3 全部
- Phase 5 richness 用 Phase 4 的 ledger 数据做 telemetry

---

## Phase 5 — Richness Contract(软 telemetry)

### 5.1 目标

| 输入文档原文 | 适配重述 |
|---|---|
| Evidence 足够时鼓励答案扩展;不足时不强迫编造。识别"正确但没价值"答案。 | 不 hard-fail。当 `FacetOptional` 缺失时,通过 `closure.AppendViolation` 写入 SOFT entry(类 `ViolPlanCritic` pattern)+ end-of-Run 在 `[CGEC] summary` 行加一个 `optional_facets_missed=N` 计数。 |

### 5.2 现状盘点

| 假设 | 真实代码 |
|---|---|
| 已有 SOFT violation pattern | ✅ `ViolPlanCritic / ViolReflectorObservation / ViolAnswerReviewerDistilled` 都是 SOFT-by-default |
| 已有 closure stats 输出 | ✅ `[CGEC] summary` + `[CGEC] perStage` 行 |
| 已有 RichnessTier 概念 | ❌ 输入文档新提,本 Phase 实现 |

### 5.3 接入点

**修改**: `internal/types/facet_plan.go`(Phase 1 文件)
```go
type RichnessTier string
const (
    TierEssential   RichnessTier = "essential"   // = FacetHardRequired
    TierExpected    RichnessTier = "expected"    // = FacetSoftRequired
    TierEnrichment  RichnessTier = "enrichment"  // = FacetOptional
)

// 加到 FacetRequirement
type FacetRequirement struct {
    // ... existing ...
    Tier RichnessTier // NEW
}
```

**新 ViolationKind**(`internal/types/violation.go`):
```go
ViolRichnessRegression ViolationKind = "richness_regression"
```
分类: SOFT(永不 strict-promote,纯 telemetry)。

**修改**: `internal/orchestrator/contract_check.go`
- `runAnswerShapeOracle` 末尾,遍历 doc 已覆盖的 facet
- 对每个 `Tier=Enrichment` 但 evidence 充足却没覆盖的 facet → 写入 SOFT `ViolRichnessRegression`

**修改**: `[CGEC] summary` 行加字段
```
[CGEC] summary: ... optional_facets_covered=N/M
```

### 5.4 LOC 估算

```
+ 30 LOC  internal/types/facet_plan.go (RichnessTier)
+ 5 LOC   internal/types/violation.go (ViolRichnessRegression)
+ 60 LOC  internal/orchestrator/contract_check.go (richness telemetry)
+ 20 LOC  internal/types/evidence_closure.go ([CGEC] summary 字段)
+ 100 LOC tests
─────────
+ ~215 LOC,8 测试
```

### 5.5 入场前置条件

- Phase 4 已 ship + 已 PASS 6 个 case 的 hard gate 不误触
- Phase 1 FacetCoverage Optional[] 在真实 eval 上 LLM 偶尔覆盖偶尔不覆盖(有数据)

### 5.6 退场标准

- 真实 eval 上观察:`optional_facets_covered` 中位数随 prompt 改进而提升
- 不引入任何 hard fail
- 不让 finalizer 多 retry(SOFT-only)

### 5.7 风险

- **极低**。纯观测 + SOFT classification + 无 retry 影响。

### 5.8 与上下游依赖

- 依赖 Phase 4 数据通路
- 为 Phase 6 cleanup 提供"哪些 helper 可以下线"的数据(如某 helper 长期 0 触发)

---

## Phase 6 — 旧 Helper / 旧补丁清理

### 6.1 目标

| 输入文档原文 | 适配重述 |
|---|---|
| 清理被 capability/facet 主链替代的局部逻辑;防止两套机制长期共存。 | 评估并下线下列已被 facet/ClaimForm 路径替代的 scattered helpers,确保单一真理源。 |

### 6.2 候选清理项(代码盘点)

#### a) `runAuthorityOverreachCheck` (`internal/orchestrator/contract_check.go:289`)
- 现职责: 检查 historical/illustrative 升级到 current 的 prose 漂
- Phase 4 后: `validateClaimFormSupport` 通过 ClaimForm temporal/polarity 检查覆盖
- **决策**: 不删,但**简化**到只检查"prose 提到 'currently' 但 evidence 全是 historical" — 其余移交 `validateClaimFormSupport`
- LOC 减少: ~80 LOC

#### b) `validateConfigTraceAbsenceCitationFocus` (`internal/tool/emit_answer_document.go`)
- 现职责: config trace 题型的 absence-cite 校验
- Phase 4 后: `validateAbsenceScopeBound` 是泛化版,适配所有 absence 题型
- **决策**: 删除,功能合并到 `validateAbsenceScopeBound`
- LOC 减少: ~120 LOC

#### c) `validateLogSourceDriftStepCitations` (`internal/tool/emit_answer_document.go`)
- 现职责: drift-bounded mode 强制 citation_ref >=0
- Phase 4 后: `validateClaimFormSupport` 通过 `ClaimExternalObservation + Tier=Essential` 覆盖
- **决策**: 保留(Drift 是特殊 mode,删合并风险高);改为 thin wrapper 调 `validateClaimFormSupport`
- LOC 减少: ~30 LOC

#### d) Scattered codename grounding (`validateSummaryCodenameGrounding`)
- 现职责: prose 中提到 `S1/S2/Stage-N` 等 codename 必须在 cited line 中真实存在
- Phase 4 后: `validateClaimFormSupport` 的 prose-fact-vs-citation 验证覆盖
- **决策**: 评估后再决定。Codename grounding 有特定的 token shape 规则,可能需保留作为 specialized check。
- LOC 减少: 0(暂保留)

#### e) Plan E G2 (completeness floor) + G3 (bucket alignment)
- Phase 4 `validateFacetCoverage` 的 `FacetEnumerationItem` + `FacetBucketLabel` facet 检查覆盖了相同语义
- **决策**: 不删除(独立 surface,clean 抽象);但要把 G2/G3 的 ViolationKind 整合进 facet 体系(`ViolDeclaredCountDrift` 仍为主信号)

### 6.3 接入点

**修改**: 上面 a-d 列出的 4 个文件。

**新增**: `internal/orchestrator/fallback_policy.go` 验证 violation kind 的 producer 全部存活(测试 `TestDefaultFallbackPolicy_CoversEveryViolationKind` 已存,会自动捕捉空 producer)。

**新增**: `docs/architecture.md` 加一节 "Phase 6 cleanup ledger" 列出:
- 哪些 helper 被替代
- 哪些保留作为 specialized check
- 替代时间

### 6.4 LOC 估算

```
- 230 LOC 旧 helper 删 / 简化
+ 50 LOC  替代 thin wrappers
+ 100 LOC tests 调整
+ 30 LOC  docs/architecture.md
─────────
净 -50 LOC
```

### 6.5 入场前置条件

- Phase 5 ship + ≥4 周真实 eval 数据稳定
- Phase 4 hard gate 误触率 < 5%
- 有数据证明候选清理项的功能 100% 被 facet 主链覆盖

### 6.6 退场标准

- 删除的 helper 在 git log 留有 superseded-by 注释
- 替代关系在 `docs/architecture.md` 落档
- 全部 46 包测试 PASS

### 6.7 风险

- **中**。删除已有逻辑总会有 corner case。
- 缓解: 保留 thin wrapper 至少 1 个 release 周期;真出问题可快速回滚。

### 6.8 与上下游依赖

- 依赖 Phase 4+5 全部 ship
- 是终态 cleanup,无下游

---

## 全 Phase 总览表

| Phase | 描述 | 风险 | LOC | 测试 | 前置 |
|---|---|---|---|---|---|
| **0** | Capability projection (`ClaimFormOf`) | 极低 | +200 | 15 | 主线 main |
| **1** | Compile FacetCoverage(无 gate) | 低 | +415 | 18 | Phase 0 |
| **2** | Finalizer prompt 消费 | 中 | +300 | 8 | Phase 1 |
| **3** | AnswerDocument ClaimUse 注解 | 中 | +290 | 12 | Phase 2 |
| **4** | Validator 启用(4 hard gates + 3 ViolKind) | **高** | +720 | 30 | Phase 3 |
| **5** | Richness telemetry(SOFT) | 极低 | +215 | 8 | Phase 4 |
| **6** | 旧 helper 清理 | 中 | -50 | tests adjust | Phase 5 + 4 周观察 |
| **总计** | | | **~2090 LOC** | **91 测试** | 6 阶段顺序 |

注: 总 LOC 比上层适配文档(~600-900)多,因为分阶段后明确算了所有 test + docs。**实际生产代码 ~1100 LOC**;test + docs ~990 LOC。

---

## 进度节点建议

| 节点 | 累计完成 | 可观察收益 |
|---|---|---|
| **Sprint 1** | Phase 0 + 1 | Trace 可见 facet plan + ClaimFormOf,无行为变化 |
| **Sprint 2** | + Phase 2 + 3 | Finalizer 收到 facet 引导,LLM 答案 facet 覆盖率提升 |
| **Sprint 3** | + Phase 4 | 真实 catch s7a/m2a/s3a 类 metadata drift |
| **Sprint 4** | + Phase 5 + 6 | richness 观测 + helper 清理,稳定 |

每 Sprint 之间 ≥1 周真实 eval observation,出现严重回归立即 rollback 当前 Phase。

---

## 红线再确认

| 红线 | 7 阶段方案合规 |
|---|---|
| `feedback_no_overfitted_solutions.md` | ✅ Facet/ClaimForm 全是抽象类型,跨 testcase |
| `feedback_no_custom_keyword_matching.md` | ✅ Phase 4 ClaimForm projection 全 typed enum |
| `feedback_user_intent_over_system_gates.md` | ✅ Facet hard 仅 essential tier,user 可通过 unknown 回退 |
| `feedback_no_internal_info_in_llm_prompts.md` | ✅ Phase 2 prompt 抽象 facet 名,glossary lint 把关 |
| `feedback_root_cause_only.md` | ✅ Phase 3+4 修的是"prose claim 无 typed 对照"根因 |
| `feedback_precise_signals_for_hard_gates.md` (2026-05-02) | ✅ ClaimFormOf 是 deterministic 投影,精确 |

---

## 待对齐项(开发前必须确认)

1. **`ClaimFormOf` 投影矩阵**: 9 ClaimForm × 6 AnchorKind × 4 Origin × 6 Scope × 4 ContextRole 的精确 case 划分
2. **`QuestionFamily` 划分粒度**: 当前 7 个 family 是否覆盖所有 codrax 答题模式?
3. **`AnswerFacetKind` 12 项**: 是否遗漏关键 facet?
4. **3 个新 ViolationKind 的 soft/strict 默认**:已建议但需用户确认
5. **fallback policy 映射**: `ViolFacetUncovered → BackToExplore` 等是否合理?
6. **Phase 6 删除候选**: 4 项是否过激?

每一项都建议 Phase 0 ship 后用真实 trace 数据决策,而非现在拍板。

---

## 文档版本

| 日期 | 版本 | 变更 |
|---|---|---|
| 2026-05-02 | 0.1 | 7 阶段逐一适配版,基于真实代码盘点 |
