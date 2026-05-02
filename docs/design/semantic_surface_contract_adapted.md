# Semantic Surface Contract — Codrax 现架构适配版

**输入文档**: [semantic_surface_contract_input.md](./semantic_surface_contract_input.md)
**适配日期**: 2026-05-02
**当前 main**: `33b3209` (Plan D + Plan E G2/G3 已落地;Plan E G1 已撤;grounded-reviewer 已撤回)

---

## 0. 适配总纲

输入文档的核心命题完全成立:**第三轴(语义句式能力 + 答案语义面)是 codrax 当前 PASS-but-thin 类问题的共同根因**。本适配文档基于真实代码现状,把输入文档的方案落到具体接点,并标记三类:

- ✅ **完全照搬**(输入文档表达准确,直接采用)
- 🔧 **代码现状已覆盖,需要 surface 化**(80% 已存,补结构化导出)
- ⚠️ **需要根据实际代码调整**(原方案的命名/字段/层级与真实代码冲突)

适配后整体收益:
- 输入文档原始 LOC 估算(粗算)~1500-2500 LOC + 50+ 测试
- **本适配版估算**:**~600-900 LOC** + 30 测试(因为大量重用现有 AuthorityCeiling / EvidenceContextRole / AnchorKind / AnswerSurfacePlan 字段)

---

## 1. 现架构盘点 — 输入文档假设 vs 真实代码

### 1.1 真理源(已具备,不重造)

| 输入文档假设 | 真实代码 (`internal/types/`) | 状态 |
|---|---|---|
| `AuthorityCeiling` 4 级 | `authority.go`: `AuthorityFactual / Conditional / Historical / Illustrative` | ✅ |
| `ClaimOrigin` 4 来源 | `authority.go`: `ClaimOriginCurrentRepo / Log / Perf / CrossSource` | ✅ |
| `DriftReason` 3 种 | `answer_surface_plan.go`: `LineDrift / TailRename / FileMoved` | ✅ |
| `QuestionStructure` 3 轴 | `question_structure.go`: `EnumerationBoundary` + `CompletenessObligation` + `Buckets[]` + `QuestionStructure()` 访问器 | ✅ |
| `EvidenceItem` 字段(AnchorKind, Scope, ContextRole, DiagramRole) | `evidence.go`: 完整 + 36 个字段已定义 | ✅ |
| `AnswerSurfacePlan` | `answer_surface_plan.go`: 30+ 编译字段(StepBackbone / ExactResolution / CapabilitySurface / DriftBoundedSurfaceItems / SurfaceEvidence / ConfigTraceDiagramAnchors 等) | ✅ |

**结论**: 输入文档原则 4.1 "不新增第二真理源" 在实施层完全可行 — 所有需要的输入字段都已存在。

### 1.2 已经部分实现"第三轴"的现有机制

输入文档把"第三轴"刻画成一个尚未存在的层。但 codrax 现有代码里已经**部分实现**了 ClaimForm 的语义:

| 输入文档 ClaimForm | 现有等价物(隐式) | 现状评估 |
|---|---|---|
| `ClaimDefinitionFact` | `AnchorKind=AnchorDefinition` + `ContextRole=Defining` | 🔧 已存,缺 surface 化 |
| `ClaimCallEdge` | `AnchorKind=AnchorCall` | 🔧 已存,缺 edge-vs-node 区分 |
| `ClaimGuardCondition` | `AnchorKind=AnchorCondition` | 🔧 已存 |
| `ClaimAssignmentFact` | `AnchorKind=AnchorAssignment` + `ContextRole=Defining` | 🔧 已存 |
| `ClaimReturnFact` | `AnchorKind=AnchorReturn` | 🔧 已存 |
| `ClaimAbsenceFact` | `Scope=ScopeNegative` + `NegativePattern` | 🔧 已存,缺 AbsenceScope.Completeness 字段 |
| `ClaimPrecedenceRole` | `DiagramRole={Default,Config,Runtime,Override}` | 🔧 已存,缺 facet-level coverage check |
| `ClaimExternalObservation` | `Origin={Log,Perf}` + `ContextRole=AbsenceSupport/RelatedContext` | 🔧 已存 |
| `ClaimDiagramNode` / `ClaimDiagramEdge` | 现 emit_answer_document `validateSummaryDiagramGrounding` 只验 file-shaped token,未 node/edge 拆分 | ⚠️ 需要新增 |

**关键发现**: ClaimForm 不应作为独立的 LLM-emit 字段,而应作为**deterministic projection**: `(AnchorKind × ContextRole × Authority × Origin × Scope) → ClaimForm`。这避免了 LLM 自创第二真理源,也避免了与现有 AnchorKind 重复。

### 1.3 现有 Constraint 已经分散存在

输入文档提的 `CapabilityConstraint`:

| 输入文档约束 | 现有实现 | 状态 |
|---|---|---|
| `NoPromoteHistoricalToCurrent` | `runAuthorityOverreachCheck` (`contract_check.go:289`) | ✅ 已部分实现 |
| `NoPromoteAssignmentToGuard` | 无 | ⚠️ 需新增 |
| `NoPromoteDefinitionToCause` | 无 | ⚠️ 需新增 |
| `NoPromoteObservationToMechanism` | `Authority` 已分级,但 prose 未约束 | 🔧 需 surface 到 prompt + validator |
| `NoRepoWideAbsenceWithoutScope` | `validateConfigTraceAbsenceCitationFocus` 部分覆盖 | 🔧 需扩展到全题型 |

### 1.4 现有 AnswerFacet 已经分散存在

输入文档提的 `AnswerFacetKind`:

| 输入文档 Facet | 现有实现 | 状态 |
|---|---|---|
| `FacetObservedArtifactFact` | `LogObservedAnchors` / `ExternalObservationSeeds` | ✅ 已编译,缺 coverage gate |
| `FacetCurrentCodePath` | `ExactContextRequiredFiles` + `ExactResolution.AnchorPath` | ✅ 已编译 |
| `FacetNearestMechanism` | 无,LLM 自由发挥 | ⚠️ 需新增 |
| `FacetUncertaintyBoundary` | apply_authority_hedging 有 marker,但无强制 facet | 🔧 需 surface 化 |
| `FacetConfigPrecedenceRole` | `ConfigTraceDiagramAnchors` + `DiagramRole` | ✅ 已编译,缺 hard coverage check |
| `FacetResolvedLiteralOrSymbol` | `ExactResolution` | ✅ 已编译 |
| `FacetEnumerationItem` / `FacetBucketLabel` | Plan E `EnumerationBoundary` / `Buckets[]` | ✅ Plan E 已落地 |
| `FacetDiagramSpine` | `StepBackbone` / `ExplanationAnchorBackbone` | ✅ 已编译,缺 diagram-only 视图 |
| `FacetMinimalJustification` | 无 | ⚠️ 需新增 |
| `FacetPrincipalPathEdge` / `FacetBranchGuard` | 隐含在 step.AnchorKind | 🔧 需 surface 化 |
| `FacetComponentRelation` | `CapabilitySurface` 部分覆盖 | 🔧 |

**关键发现**: Codrax 已经在 `AnswerSurfacePlan` 里编译了 ~70% 的 facet 信息,只是没有 surface 成"必须覆盖的 facet 清单"对齐到 prompt + validator。**改造重心是"对齐 + 暴露",不是"从零编译"**。

---

## 2. 适配后的方案骨架

### 2.1 不新增 ClaimForm 顶层枚举,改为 deterministic projection

**原方案**: 新增 `internal/types/ClaimForm` 10 个枚举值。
**适配版**: 不新增枚举。增加纯函数 `internal/types/ClaimFormOf(EvidenceItem) ClaimForm`,从 `(AnchorKind, ContextRole, Origin, Scope, Authority)` 五元组 deterministic 推导:

```go
// internal/types/claim_form.go (NEW, ~80 LOC)
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
)

// ClaimFormOf is a pure deterministic projection. NEVER read by LLM
// directly — used only by gate validators and surface compilers.
func ClaimFormOf(item EvidenceItem) ClaimForm {
    if item.Origin == ClaimOriginLog || item.Origin == ClaimOriginPerf {
        return ClaimExternalObservation
    }
    if item.Scope == ScopeNegative {
        return ClaimAbsenceFact
    }
    if item.DiagramRole != EvidenceDiagramRoleUnknown && item.DiagramRole != EvidenceDiagramRoleDefault {
        return ClaimPrecedenceRole
    }
    switch item.AnchorKind {
    case AnchorCall:       return ClaimCallEdge
    case AnchorCondition:  return ClaimGuardCondition
    case AnchorReturn:     return ClaimReturnFact
    case AnchorAssignment: return ClaimAssignmentFact
    case AnchorImport:     return ClaimImportEdge
    case AnchorDefinition: return ClaimDefinitionFact
    }
    return "" // unknown
}
```

**为什么不让 LLM emit ClaimForm**: 原方案 `EvidenceClaimCapability` 假设 LLM 提交 capability 元数据。但这等于增加 LLM 负担 + 可能伪造。Codrax 已经让 LLM emit `AnchorKind/ContextRole/Scope` 这些原子字段,系统可以纯 derive ClaimForm。**单一真理源**(emit_evidence) → **deterministic 投影**(ClaimFormOf) → **gate 验证**(无 prompt 暴露)。

### 2.2 不新增 EvidenceSemanticCapability 结构,改为 capability checking 函数

**原方案**: 每条 evidence 携带 `[]ClaimCapability` 数组。
**适配版**: capability 永远是问题"这条 evidence 能否支持这个 claim"的查询,而不是预存数组。设计成两个纯函数:

```go
// internal/types/claim_capability.go (NEW, ~120 LOC)

// CanSupportAsPrincipal reports whether evidence is strong enough to
// be the load-bearing claim of an answer. Reads existing fields:
//   - Authority must be Factual (Conditional/Historical/Illustrative
//     can support but not lead)
//   - GroundingStatus must be Grounded
//   - ContextRole must be Defining or AbsenceSupport (not Illustrative)
//   - For absence claims, scope must be at least file-local (verified
//     via Scope==ScopeNegative + the validating tool ran a real search)
func CanSupportAsPrincipal(item EvidenceItem) bool { ... }

// CanSupportClaim reports whether this evidence can validly back a
// specific ClaimForm. The matrix is small enough to inline:
//   - ClaimGuardCondition: only AnchorKind=AnchorCondition
//   - ClaimCallEdge: only AnchorKind=AnchorCall
//   - ClaimAbsenceFact: only Scope=ScopeNegative
//   - ClaimDefinitionFact: AnchorKind=AnchorDefinition AND
//     ContextRole=Defining
//   - ... etc.
//
// This is the "no_promote_assignment_to_guard" rule encoded as data.
func CanSupportClaim(item EvidenceItem, form ClaimForm) bool { ... }
```

### 2.3 AnswerFacetPlan 作为 AnswerSurfacePlan 的新字段(不是新顶层结构)

**原方案**: 新增独立 `AnswerFacetPlan` 顶层结构。
**适配版**: 把 facet plan 加成 `AnswerSurfacePlan` 的一个新字段 `FacetCoverage *FacetCoverageContract`,这样:
- 复用现有 `BuildAnswerSurfacePlan` 编译入口
- 复用现有 `Mutable.SetAnswerSurfacePlan` 持久化
- 复用现有 finalizer/extractor 读 surface plan 的所有路径

```go
// internal/types/answer_surface_plan.go (EXTEND existing, ~50 LOC)

type FacetCoverageContract struct {
    Family        QuestionFamily   // derived from Intent + Scenario + AnswerSubject
    Required      []FacetRequirement // hard gates
    Optional      []FacetRequirement // richness candidates
}

type FacetRequirement struct {
    Kind            AnswerFacetKind
    AcceptableForms []ClaimForm        // ClaimForm whitelist for this facet
    SurfaceRoles    []SurfaceRole      // principal/support/diagram-only/prose-only
    SourceCandidate []string           // EvidenceItem.IDs that the planner thinks could fill this facet
}
```

`QuestionFamily` 推导:

```go
func ResolveQuestionFamily(rm RequestModel) QuestionFamily {
    switch rm.Intent {
    case IntentRootCause:
        if rm.Scenario == ScenarioRootCause { return QFRootCauseTrace }
    case IntentConfigQuery:
        return QFConfigPrecedence
    case IntentTrace:
        return QFCallChain
    case IntentEnumerate:
        return QFEnumeration
    case IntentExplain:
        if rm.Scenario == ScenarioArchitectureExplain { return QFArchitecture }
    }
    if rm.AnswerSubject.Kind == SubjectFunctionName ||
       rm.AnswerSubject.Kind == SubjectHandlerRoute ||
       rm.AnswerSubject.Kind == SubjectConfigKey {
        return QFRoleLookup
    }
    return QFGeneric
}
```

### 2.4 Validator 复用现有 emit_answer_document 接入点

**原方案**: 5 个新 validator(`validateFacetCoverage` / `validateClaimFormSupport` / 等)。
**适配版**: 4 个新 validator,但**重用 30+ 现有 validate* 函数的 helper**:

| 新 validator | 调用复用现有 |
|---|---|
| `validateFacetCoverage` | 复用 `lookupCitation` / `extractInlineCodeTokens` |
| `validateClaimFormSupport` | 复用 `requireCitationCorroboration` 框架 |
| `validateAbsenceScopeBound` | 扩展现有 `validateConfigTraceAbsenceCitationFocus` |
| `validateDiagramFacetGraph` | 复用 `validateSummaryDiagramGrounding` 的 mermaid 解析 + 新增 node/edge 分离 |

不再单独引入 `validateTemporalEscalation` — 已被 `runAuthorityOverreachCheck` 覆盖。

### 2.5 ViolationKind 复用现有,慎加新 kind

**原方案**: 6 个新 violation family。
**适配版**: **只加 3 个新**(其余复用):

| 新增 ViolationKind | 现有可复用 |
|---|---|
| `ViolFacetUncovered` (NEW) | — |
| `ViolClaimFormUnsupported` (NEW) | — |
| `ViolAbsenceScopeExceeded` (NEW) | — |
| ~~`temporal_escalation`~~ | 已用 `ViolAuthorityOverreach` |
| ~~`diagram_edge_unproven`~~ | 已用 `ViolDiagramIdentifier` 扩 detail |
| ~~`richness_regression`~~ | 设计成软 telemetry,不是 violation |

### 2.6 AnswerDocument 增量注解 — 完全采纳输入文档建议

```go
// internal/types/answer_document.go (EXTEND, ~30 LOC)

type RenderedClaimUse struct {
    FacetID     string
    EvidenceID  string
    ClaimForm   ClaimForm
    SurfaceRole SurfaceRole
}

type AnswerStep struct {
    Index       int
    Description string
    CitationRef int
    Kind        AnswerStepKind   // existing (Plan D)
    ClaimUse    *RenderedClaimUse // NEW
}

type AnswerSymbol struct {
    // existing fields ...
    ClaimUse *RenderedClaimUse // NEW
}
```

LLM 在 emit_answer_document 里**可选**填 ClaimUse;不填则 validator 用 `ClaimFormOf(referenced_evidence)` 推断。**两条路径并存,新增字段不强制**,所以 back-compat 自动成立。

---

## 3. 三类已知瑕疵 → 适配方案如何 catch

| 瑕疵 | 类型 | 现有架构覆盖 | 适配方案如何 catch |
|---|---|---|---|
| **s7a "97 files"** | 数值 prose claim 无 cited 来源 | 无 | `validateClaimFormSupport`: prose 出现具体数字 N → 必须存在某 Citation Quote 含 N OR EnumerationBoundary.DeclaredCount=N |
| **m2a "Priority float64"** | 类型词 prose claim 无 cited 来源 | 无 | 同上,数字 → 任何 type-shaped token (`int`, `float64`, etc.) 都需 cited 来源 |
| **s3a Mermaid 英文 label** | 渲染元素语言 vs 用户语言 | 无 | `validateDiagramFacetGraph`: diagram node label 必须 EITHER 来自 cited Quote 文本 OR 与用户语言一致(用 `RequestModel.Language` 判) |
| **s3a 漏 layer (codrax.yaml)** | declared count 漂 | Plan E `EnumerationBoundary` 已设但 analyzer 没识别"三层" | 输入文档已建议(R2): analysis-skill 强化 numeric quantifier 识别("N 层"/"N 阶段"/"N 步骤" 都 emit `enumeration_boundary`)。配合 Plan D Kind 强制 N principal |
| **explanation shape 跳过 reviewer** | shouldReviewConsistency 阈值 | 无 | 适配方案不通过 reviewer 修复,通过 `validateFacetCoverage` 在 emit-time 直接 catch facet 缺失(任何 shape 都跑) |

---

## 4. 实施 Phase 调整

输入文档原 Phase 0-6 共 7 阶段。适配后**合并到 4 阶段**(Phase 数缩减,因为大量字段已存):

### Phase A — Surface 化(2 天,~200 LOC)
- 增加 `ClaimFormOf()` 投影函数
- 增加 `CanSupportAsPrincipal()` / `CanSupportClaim()` 查询函数
- `AnswerSurfacePlan.FacetCoverage` 字段 + `ResolveQuestionFamily`
- 不接 validator,仅 trace/debug 输出 facet plan
- **风险**: 极低(纯新增,无消费者)

### Phase B — Finalizer Prompt 消费(1 天,~120 LOC)
- 在 `answer_document_evaluator.go` 加 `renderAnswerDocFacetCoverage` section
- prompt 教 LLM "this question requires the following facets" + "each facet's evidence must support these claim forms"
- 不接 validator,仅 prompt 引导
- **风险**: 中(prompt 改动,可能漂)

### Phase C — Validator 启用(2 天,~250 LOC)
- 启用 `validateFacetCoverage`(soft 软告警,先观察 false positive 率)
- 启用 `validateClaimFormSupport`(hard,但仅在 Authority + GroundingStatus 都 OK 时收紧 — 避免 cascade 干扰)
- 启用 `validateAbsenceScopeBound`(hard,Scope=Negative 必须有 NegativePattern)
- 启用 `validateDiagramFacetGraph`(node 与 edge 分开)
- **风险**: 高(新硬门可能误伤,需要逐 case 调)

### Phase D — Richness 软 telemetry(1 天,~100 LOC)
- `optional facet` 缺失记录到 closure ledger 但不 hard-fail
- end-of-Run 报告 "X% optional facets covered"
- 不重写 finalizer
- **风险**: 极低(纯观测)

**总估算**: ~6 天 / ~670 LOC + 30 测试。比输入文档原方案(~10 天 / 1500-2500 LOC)节约 ~50%,因为重用了现有 70% 的真理源。

---

## 5. 红线对齐审计

对照已有 7 条红线(`feedback_*.md`):

| 红线 | 适配方案是否合规 |
|---|---|
| `feedback_no_overfitted_solutions.md` | ✅ ClaimForm/Facet 全是抽象类型,无 testcase 词汇 |
| `feedback_no_custom_keyword_matching.md` | ✅ 全部基于 typed enum + structural cross-product,无关键词表 |
| `feedback_user_intent_over_system_gates.md` | ✅ Facet 是 LLM-emitted Intent/Scenario 编译结果,系统不硬卡用户 |
| `feedback_no_internal_info_in_llm_prompts.md` | ⚠️ 需要审计:`renderAnswerDocFacetCoverage` 不能漏 `FacetCoverageContract` / `ClaimFormOf` 等 Go 名 |
| `feedback_root_cause_only.md` | ✅ 修的是"答案的次级 claim 没 typed 对照"根因,不是单点 patch |
| `precise_signals_for_hard_gates` (2026-05-02 新红线) | ✅ ClaimForm 是 deterministic projection 自 typed fields,精确信号 |
| `feedback_eval_pass_is_not_green.md` | ✅ Phase D 软 telemetry 评估 rich 缺失 |

---

## 6. 与最近争议的对齐

### 6.1 撤销的 Plan E G1 教训如何应用

之前 G1 撤回的红线:**precise signals for hard gates, noisy signals for soft guidance**。

适配方案的硬门信号源:
- `validateFacetCoverage`: facet plan 是 deterministic 编译,精确 ✓
- `validateClaimFormSupport`: ClaimFormOf 是纯函数 deterministic,精确 ✓
- `validateAbsenceScopeBound`: Scope 是 typed enum + 必须 NegativePattern,精确 ✓
- `validateDiagramFacetGraph`: 节点 vs 边是 mermaid 解析 + Citation 反查,精确 ✓

适配方案的软引导信号源:
- Richness candidate(Phase D 软 telemetry):是 ranker-style 评分,嘈声 → 不做硬门 ✓

### 6.2 撤销的 grounded reviewer 教训如何应用

之前 grounded reviewer 撤回理由:R1 + R2 prompt 规则更根因。但用户暂未做 R1 + R2。**适配方案 Phase B 的 prompt section 实际上比 R1+R2 更强**:

- R1 是"prose 具体 claim 必须可 backed" — 笼统
- 适配 Phase B 是"this question requires facets X/Y/Z, and facet X requires claim form A/B" — 具体

适配方案 Phase B 落地后,R1+R2 已隐含包含。

### 6.3 现有 Plan D + Plan E 不破坏

适配方案与已有 Plan D Kind / Plan E G2 / G3 完全正交:
- Plan D Kind 是 step-level "principal vs flow" — 与 facet 是上下游(facet 决定哪些 step 算 principal)
- Plan E G2 (`completeness=lower_bound` reject) 是 obligation 维度 — 与 ClaimForm 维度独立
- Plan E G3 (bucket label verbatim) 是 surface 维度 — 与 facet coverage 平行

---

## 7. 适配版的 8 条最终硬要求

输入文档第 14 节 8 条要求 + 适配补充:

1. ✅ 不拟合某个问题/仓/格式 — 沿用
2. ✅ 不堆散装 heuristic — 适配版强调"不新增第二真理源,投影现有字段"
3. ✅ 不引入第二真理源 — **强化为**:`ClaimFormOf` 是 pure projection,LLM 不直接 emit
4. ✅ Finalizer 不自创 contract — 沿用
5. ✅ 不为少轮次牺牲 richness — Phase D 软 telemetry
6. ✅ 字段必须有消费者 — **强化为**:每个新增字段必须列出至少 1 producer + 1 validator + 1 prompt consumer
7. ✅ Gate 不互相冲突 — 适配版强调"`ClaimFormOf` deterministic,gate 间共享同一定义"
8. ✅ 旧 helper 必须清理 — Phase D 后清理:scattered authority overreach checks 合并到 ClaimForm-aware 版本

---

## 8. 已解决问题清单(假设方案落地)

| 问题来源 | 类型 | 落地后预期解 |
|---|---|---|
| s1a (Plan D 已修) | count obligation | 仍 PASS,facet `enumeration_item` 重叠覆盖 |
| s5a (Plan E G2 已修) | completeness | 仍 PASS,facet `enumeration_item` 全列 |
| m1a (Plan E G3 已修) | bucket | 仍 PASS,facet `bucket_label` |
| s7a "97 files" | prose 数字漂 | ✅ `validateClaimFormSupport` catch |
| m2a "Priority float64" | prose 类型词漂 | ✅ `validateClaimFormSupport` catch |
| s3a Mermaid 英文 | diagram label 语言 | ✅ `validateDiagramFacetGraph` + user-language check |
| s3a 漏 layer | declared count 未识别"三层" | ⚠️ 部分解(需配合 R2 analysis-skill prompt 强化) |
| explanation shape reviewer 阈值 | reviewer 盲区 | ✅ `validateFacetCoverage` 在 emit-time 触发,不靠 reviewer |
| u10a timeout (复杂 rename impact) | budget 不足 | ⚠️ 不解(facet plan 可能让 explorer 投资更聚焦,但 root cause 是 wall-clock 不够) |

---

## 9. 遗留事项

输入文档第 4.4 节明确标记 block-only document 模型为遗留。本适配版同样保持:
- **保留**: 现有 `AnswerShape` 枚举 + 现有 renderer + 现有 shape-specific validator
- **不做**: AnswerDocument → block-only 的 schema 大重写
- **未来**: 当 facet plan 成为主链 ≥6 个月稳定后,再评估是否 block-only

其他遗留:
- u10a 类"复杂跨文件 rename impact"题型的 wall-clock budget 是单独问题,与 facet plan 无关
- analyzer 对"三层/三阶段/N 模式"的 EnumerationBoundary 识别强化(R2)是独立 prompt 改动,可与 facet 落地并行

---

## 10. 总结对照表

| 维度 | 输入文档原方案 | 适配版 | 收益 |
|---|---|---|---|
| 新顶层枚举 `ClaimForm` | 引入 | **不引入,deterministic projection** | 单一真理源 |
| 新 `EvidenceClaimCapability` 数据结构 | 每条 evidence 携带 | **改为查询函数** | 减少存储 + 减少 LLM 负担 |
| 新独立 `AnswerFacetPlan` 顶层 | 顶层结构 | **作为 `AnswerSurfacePlan` 的字段** | 编译入口/持久化复用 |
| 新 6 个 ViolationKind | 全新 | **只加 3,其余复用现有** | 最小扰动 |
| Validator 数量 | 5 个全新 | **4 个,复用现有 helper** | 复用 30+ 现有 validate* |
| LOC 估算 | 1500-2500 | **600-900** | -50% |
| Phase 数 | 7 | **4** | 工时减半 |
| 与 Plan D / Plan E 关系 | 不冲突 | **正交可叠加** | 一致 |

---

## 11. 下一步建议

1. **对齐**: 确认本适配文档的 8 条硬要求 + Phase A-D + 三个新 ViolationKind
2. **Phase A 试点**: 仅落 `ClaimFormOf` + `CanSupportClaim` + `FacetCoverageContract` 字段(纯新增,~200 LOC)
3. **Eval 试点**: Phase A 落地后跑 6 case eval(s1a/s5a/m1a/s3a/s7a/m2a),观察 `FacetCoverage` 在 trace 里的稳定性,不开 gate
4. **Phase B-C 滚动**: 视 Phase A 观察结果决定是否继续

**不建议本 session 立即开发**:适配版需要先与用户对齐 ClaimForm projection 矩阵 + QuestionFamily 推导映射 + 三个新 ViolationKind 的精确语义。这些是后续开发的"contract base",一旦定下需要稳定。

---

## 12. 文档版本与变更

| 日期 | 版本 | 变更 |
|---|---|---|
| 2026-05-02 | 0.1 | 基于输入文档 + 真实代码盘点的首版适配 |

输入文档保留在 `semantic_surface_contract_input.md` 不变,本文档为**与现有代码兼容的实施计划**。
