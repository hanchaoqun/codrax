# Codrax 架构级重构与审计设计说明

## 1. 文档目标

本文档基于最新远端代码基线，对当前 read-mode / log-triage / finalizer 主链进行一次架构级复盘，目标不是给某个测试题打补丁，而是输出一份可以直接指导开发的、泛化的设计方案与审计标准。

本文档要同时解决三类目标：

1. 用户问题聚焦：系统输出必须围绕用户真正问的问题，而不是围绕某个 artifact 形态或某个历史 case。
2. 最终答案质量：答案必须真实、丰富、完整，关键机制、关键层级、关键图例不能缺失；不能只追求“少轮次”。
3. 系统稳定收敛：减少 finalizer / explorer 空转、闸门打架、字段无人消费、提示词与代码不一致、同一语义被多个模块重复推导。

本文档不是局部 patch 计划，而是新的统一编译层设计说明。

---

## 2. 当前代码基线与已具备能力

### 2.1 代码基线

当前评估基于最新远端 `main`，本地已更新到最新代码后进行审视。以下机制已经在代码中存在，且是后续设计必须复用的基础，而不是推倒重来：

- `AuthorityCeiling / ClaimOrigin / DriftReason`
  - [C:/Users/ssccv/codrax/internal/types/authority.go](C:/Users/ssccv/codrax/internal/types/authority.go)
  - [C:/Users/ssccv/codrax/internal/authority/authority.go](C:/Users/ssccv/codrax/internal/authority/authority.go)
- `QuestionStructure`
  - [C:/Users/ssccv/codrax/internal/types/question_structure.go](C:/Users/ssccv/codrax/internal/types/question_structure.go)
- `EvidenceItem / Scope / ContextRole / DiagramRole`
  - [C:/Users/ssccv/codrax/internal/types/evidence.go](C:/Users/ssccv/codrax/internal/types/evidence.go)
- `AnswerSurfacePlan`
  - [C:/Users/ssccv/codrax/internal/types/answer_surface_plan.go](C:/Users/ssccv/codrax/internal/types/answer_surface_plan.go)
- finalizer prompt builder
  - [C:/Users/ssccv/codrax/internal/agent/answer_document_evaluator.go](C:/Users/ssccv/codrax/internal/agent/answer_document_evaluator.go)
- final answer validator
  - [C:/Users/ssccv/codrax/internal/tool/emit_answer_document.go](C:/Users/ssccv/codrax/internal/tool/emit_answer_document.go)
- renderer / hedging
  - [C:/Users/ssccv/codrax/internal/render/answerdoc.go](C:/Users/ssccv/codrax/internal/render/answerdoc.go)
  - [C:/Users/ssccv/codrax/internal/render/apply_authority_hedging.go](C:/Users/ssccv/codrax/internal/render/apply_authority_hedging.go)
- orchestrator contract/reviewer gate
  - [C:/Users/ssccv/codrax/internal/orchestrator/contract_check.go](C:/Users/ssccv/codrax/internal/orchestrator/contract_check.go)

### 2.2 当前架构已经解决的两条主轴

#### A. 证据强度与来源

系统已经能回答：

- 证据来自哪里：`current_repo / log / perf / cross_source`
- 证据能说多强：`factual / conditional / historical / illustrative`
- 证据是否存在 drift：`line_drift / tail_rename / file_moved`

这意味着系统已经有了“强度”和“来源”的一等真理源，不需要再在 finalizer 或 reviewer 里靠 prose 二次猜测。

#### B. 用户问题结构义务

系统已经能回答：

- 用户是否要求枚举边界
- 用户是否要求完整性
- 用户是否要求 bucket/partition

这意味着系统已经不再只会检查“有没有回答”，而是能开始检查“是否按用户要求的结构回答”。

### 2.3 当前架构还缺的第三条主轴

系统还缺少一个同样一等、同样 deterministic 的层：

> 这条 evidence 到底允许被表述成什么语义句式；最终答案必须覆盖哪些语义 facet。

这是目前所有“看起来 PASS，但答案解释和代码对不上”、“轮次少了但答案变薄”、“diagram 有了但边不对”、“absence 没胡说但说过头”的共同根因。

---

## 3. 问题定义：当前系统的核心断裂点

### 3.1 当前系统知道“能说多强”，但还不知道“能怎么说”

现在一条 evidence 即使满足：

- grounded
- authority=factual
- citation-grade

仍然不代表它允许支持任意句式。

例如：

- assignment anchor 不能自动支持 guard condition
- definition anchor 不能自动支持 root cause
- historical/log observation 不能自动支持 current mechanism
- diagram node 不能自动支持 diagram edge
- negative evidence 不能自动支持 repo-wide absence

这不是强度问题，而是**语义句式能力问题**。

### 3.2 当前系统知道“用户要什么结构”，但还不知道“答案必须覆盖哪些语义面”

系统已经能检查：

- 需要几个
- 是否要完整
- 是否要分 bucket

但仍然缺少对“root cause / trace / config precedence / role lookup / call chain / architecture”这些问题的**语义 facet**建模。

结果就是：

- 答案可能没有胡说，但缺关键机制
- 答案可能有 diagram，但缺 spine
- 答案可能说了 observed fact，却没说 uncertainty boundary
- 答案可能讲了 current code path，却没讲 nearest mechanism
- 配置类答案可能说了 exact absence，却没给 precedence 链

### 3.3 当前 rich answer 和 safe answer 之间仍缺统一 contract

如果系统只追求减少重试，最容易退化成：

- 更短
- 更保守
- 更薄
- 更像“最小可过答案”

这会让答案即便没有错，也缺乏真正对用户有价值的细节。

因此下一步设计必须同时满足：

- 正确性
- 丰富度
- 稳定收敛

而不是三者取二。

---

## 4. 设计原则

### 4.1 不新增第二真理源

任何新机制都必须建立在已有结构化真理源之上，而不是再发明一套平行系统：

- 不重新发明 authority
- 不重新发明 question structure
- 不重新发明 diagram contract
- 不重新发明 exact-resolution
- 不重新发明 drift detector

新增能力必须作为这些真理源的上游投影或下游编译结果存在。

### 4.2 不做 testcase-specific / repo-specific 逻辑

禁止以下方案：

- 针对 `yaml`、`panic`、`nil guard`、`DefaultExploreHeuristics` 等特定词的专门逻辑
- 假定“最多三层”
- 假定“配置一定是 YAML”
- 假定“diagram 一定是线性链”
- 假定“某个目录命名就表示某个业务角色”

允许的只有：

- 类型系统
- 已有结构化 evidence 字段
- grounded source/scope
- analyzer / authority / question structure 的编译结果

### 4.3 LLM 负责表达，不负责发明 contract

LLM 的职责：

- 组织自然语言
- 选择表达顺序
- 在 contract 允许的范围内输出更丰富、更好读的答案

LLM 不负责：

- 决定 evidence 可以支持什么 claim form
- 决定 absence scope
- 决定 diagram edge 是否真实存在
- 决定 support evidence 能否升格为 principal conclusion

### 4.4 shape 系统先保留，增量迁移

当前的 `AnswerDocument`、renderer、contract checker 都强依赖 shape。后面改成完全通用的 block-only 文档模型，当前不建议block-only 文档模型,因为改造风险过大，也会重复造轮子,  未来要迁移到block-only 文档模型暂时先记录到遗留事项中。

因此本文档明确采用：

- 保留现有 `shape`
- 在 shape 上新增 facet/claim annotation
- 在编译层统一语义 contract
- 在 validator 层按 facet 做强校验

这是一条兼容现有代码、回归风险最小的迁移路线。

---

## 5. 总体方案：Semantic Surface Contract

核心新增两层：

1. `EvidenceClaimCapability`
2. `AnswerFacetPlan`

它们构成统一的 `Semantic Surface Contract`。

### 5.1 EvidenceClaimCapability 要解决的问题

回答：

- 某条 evidence 可以支持哪些 claim form
- 每种 claim form 能进入哪些 surface role
- 最大 authority 到哪里
- temporal / polarity / scope 是什么
- 是否允许 principal / citation / diagram 使用
- 有哪些不可越界约束

### 5.2 AnswerFacetPlan 要解决的问题

回答：

- 当前这类问题，最终答案必须覆盖哪些 facet
- 每个 facet 允许用哪些 claim form 支撑
- 哪些是 hard-required
- 哪些是 soft-required
- 哪些是 optional-rich
- 哪些只能 prose-only
- 哪些必须 citation-grade
- 哪些可以进 diagram
- 哪些必须带 uncertainty boundary

### 5.3 Richness Contract 要解决的问题

回答：

- 在 evidence 足够时，答案应该扩展到什么程度才算“高质量”
- 不靠字数，而靠 evidence-backed richness candidate
- 不让系统为了少轮次把答案压薄

---

## 6. 适用问题族

这套方案不是只为某个 case 设计，适用的问题族包括：

- log/perf drift-bounded root cause / trace
- config/env/flag/source precedence
- exact lookup / role lookup / locate-by-role
- call chain / dispatch / flow / fallback / retry
- architecture / component relation
- enumeration / bucketed comparison / completeness

不要求这些问题共享同一个 `shape`，但它们共享：

- claim-form capability
- answer facet coverage
- richness contract

---

## 7. 数据模型设计

### 7.1 ClaimForm：证据可支持的基本句式

`ClaimForm` 描述的是“这条 evidence 最多可以被说成什么样”。

建议新增到 `internal/types`：

```go
type ClaimForm string

const (
    ClaimCallEdge            ClaimForm = "call_edge"
    ClaimGuardCondition      ClaimForm = "guard_condition"
    ClaimDefinitionFact      ClaimForm = "definition_fact"
    ClaimAssignmentFact      ClaimForm = "assignment_fact"
    ClaimReturnFact          ClaimForm = "return_fact"
    ClaimAbsenceFact         ClaimForm = "absence_fact"
    ClaimPrecedenceRole      ClaimForm = "precedence_role"
    ClaimExternalObservation ClaimForm = "external_observation"
    ClaimDiagramNode         ClaimForm = "diagram_node"
    ClaimDiagramEdge         ClaimForm = "diagram_edge"
)
```

设计原则：

- `ClaimForm` 只描述基本句式能力
- 不把“root cause”“config default”“runtime override”等业务语义直接塞进 `ClaimForm`
- 避免枚举无限膨胀

### 7.2 ClaimTemporal / ClaimPolarity

同一个 claim form 还需要带时间和极性：

```go
type ClaimTemporal string

const (
    ClaimCurrent     ClaimTemporal = "current"
    ClaimHistorical  ClaimTemporal = "historical"
    ClaimStatic      ClaimTemporal = "static"
    ClaimConditional ClaimTemporal = "conditional"
    ClaimUnknownTime ClaimTemporal = "unknown_time"
)

type ClaimPolarity string

const (
    ClaimPresent ClaimPolarity = "present"
    ClaimAbsent  ClaimPolarity = "absent"
)
```

这两项是为了让系统显式区分：

- current vs historical
- positive fact vs absence fact
- static declaration vs conditional branch

### 7.3 CapabilityConstraint

系统需要把“绝不允许的升级”编码成结构约束，而不是 scattered case rule。

```go
type CapabilityConstraint string

const (
    NoPromoteAssignmentToGuard       CapabilityConstraint = "no_promote_assignment_to_guard"
    NoPromoteDefinitionToCause       CapabilityConstraint = "no_promote_definition_to_cause"
    NoPromoteHistoricalToCurrent     CapabilityConstraint = "no_promote_historical_to_current"
    NoPromoteObservationToMechanism  CapabilityConstraint = "no_promote_observation_to_mechanism"
    NoRepoWideAbsenceWithoutScope    CapabilityConstraint = "no_repo_wide_absence_without_search_scope"
)
```

这几条约束都对应真实的系统级错误类型，不依赖任何仓特定词。

### 7.4 EvidenceSemanticCapability

不采用：

- `AllowedClaimForms []`
- `AllowedSurfaceRoles []`

这种分离数组形式，因为它容易形成非法交叉积。

采用每个 claim form 一条 capability 的形式：

```go
type EvidenceSemanticCapability struct {
    EvidenceID string
    Claims     []ClaimCapability
}

type ClaimCapability struct {
    ID                  string
    Form                ClaimForm
    AllowedSurfaceRoles []SurfaceRole
    MaxAuthority        types.AuthorityCeiling
    Scope               types.EvidenceScope
    Temporal            ClaimTemporal
    Polarity            ClaimPolarity
    PrincipalEligible   bool
    CitationEligible    bool
    DiagramEligible     bool
    Constraints         []CapabilityConstraint
    ProjectionReason    string
    AbsenceScope        *AbsenceScope
}
```

这样一条 evidence 可以合法表达：

- `definition_fact as principal`
- `assignment_fact as support`

但不会错误推导出：

- `assignment_fact as principal mechanism`

### 7.5 AbsenceScope

absence claim 必须绑定范围，否则就是系统性风险点。

```go
type AbsenceScope struct {
    SearchTarget    string
    SearchedPaths   []string
    SearchedSymbols []string
    SearchMethod    string
    Completeness    AbsenceCompleteness
}

type AbsenceCompleteness string

const (
    AbsenceLineLocal   AbsenceCompleteness = "line_local"
    AbsenceFileLocal   AbsenceCompleteness = "file_local"
    AbsencePackageWide AbsenceCompleteness = "package_wide"
    AbsenceRepoWide    AbsenceCompleteness = "repo_wide"
    AbsenceUnknown     AbsenceCompleteness = "unknown"
)
```

规则：

- `ClaimAbsenceFact` 没有 `AbsenceScope` 就不能进入用户可见主结论
- absence 的最终表述不能超出 `Completeness`
- negative evidence 是必要条件，但不是充分条件

### 7.6 AnswerFacetKind：答案必须覆盖的语义面

`AnswerFacetKind` 不等于 `ClaimForm`。

- `ClaimForm` 描述证据能力
- `AnswerFacetKind` 描述用户问题需要的答案面

建议：

```go
type AnswerFacetKind string

const (
    FacetObservedArtifactFact    AnswerFacetKind = "observed_artifact_fact"
    FacetCurrentCodePath         AnswerFacetKind = "current_code_path"
    FacetNearestMechanism        AnswerFacetKind = "nearest_mechanism"
    FacetUncertaintyBoundary     AnswerFacetKind = "uncertainty_boundary"
    FacetConfigPrecedenceRole    AnswerFacetKind = "config_precedence_role"
    FacetResolvedLiteralOrSymbol AnswerFacetKind = "resolved_literal_or_symbol"
    FacetMinimalJustification    AnswerFacetKind = "minimal_justification"
    FacetPrincipalPathEdge       AnswerFacetKind = "principal_path_edge"
    FacetBranchGuard             AnswerFacetKind = "branch_guard"
    FacetComponentRelation       AnswerFacetKind = "component_relation"
    FacetEnumerationItem         AnswerFacetKind = "enumeration_item"
    FacetBucketLabel             AnswerFacetKind = "bucket_label"
    FacetDiagramSpine            AnswerFacetKind = "diagram_spine"
)
```

### 7.7 AnswerFacet

facet 不能只保存 `EvidenceIDs []string`，因为同一条 evidence 可能支持多个 capability。

建议：

```go
type AnswerFacet struct {
    ID               string
    Kind             AnswerFacetKind
    Requiredness     FacetRequiredness
    SurfaceRole      SurfaceRole
    ClaimRequirement ClaimRequirement
    Supports         []FacetEvidenceUse
    Uncertainty      *UncertaintyBoundary
    CoveragePolicy   FacetCoveragePolicy
    RichnessTier     RichnessTier
    RenderHints      []string
}
```

支持关系使用：

```go
type FacetEvidenceUse struct {
    EvidenceID       string
    ClaimID          string
    Form             ClaimForm
    SurfaceRole      SurfaceRole
    Authority        types.AuthorityCeiling
    Temporal         ClaimTemporal
    Polarity         ClaimPolarity
    Scope            types.EvidenceScope
    Directness       SupportDirectness
    CitationAnchors  []CitationAnchor
}
```

### 7.8 CitationAnchor 而不是 CitationRef

facet 编译阶段不固定 `citation_ref` 编号。

原因：

- citation 编号依赖最终渲染顺序
- 一个 citation 可能服务多个 facet
- diagram/prose 的引用形态不同

因此 plan 里只存 semantic anchor：

```go
type CitationAnchor struct {
    EvidenceID string
    AnchorID   string
    SpanRef    string
}
```

### 7.9 DiagramFacetGraph

diagram 必须对 node 和 edge 分开校验。

```go
type DiagramFacetGraph struct {
    Nodes []DiagramFacetNode
    Edges []DiagramFacetEdge
}

type DiagramFacetNode struct {
    ID                 string
    FacetID            string
    EvidenceID         string
    RequiredCapability ClaimForm
}

type DiagramFacetEdge struct {
    ID                 string
    FromNodeID         string
    ToNodeID           string
    FacetID            string
    EvidenceID         string
    RequiredCapability ClaimForm
}
```

### 7.10 RichnessCandidate

丰富度不能靠字数，要靠 evidence-backed expansion。

```go
type RichnessCandidate struct {
    ID           string
    FacetID      string
    ExpectedWhen EvidenceAvailabilityCondition
    Value        RichnessValue
    MaxCost      RichnessCost
}
```

它的作用是：

- evidence 足够时，鼓励系统输出更完整的答案
- 不足时，不强迫模型编造

---

## 8. AnswerFacetPlan 总结构

建议把新的编译产物落成：

```go
type AnswerFacetPlan struct {
    ID                 string
    QuestionFamily     QuestionFamily
    Facets             []AnswerFacet
    RequiredFacetIDs   []string
    OptionalFacetIDs   []string
    RichnessCandidates []RichnessCandidate
    EvidenceCaps       []EvidenceSemanticCapability
    DiagramGraph       *DiagramFacetGraph
    GlobalUncertainty  []UncertaintyBoundary
    RenderingPolicy    RenderingPolicy
}
```

其中：

- `QuestionFamily` 是内部编译归类，不是新的 LLM 自由文本字段
- 它应由现有 `Intent / Scenario / AnswerSubject / PredicateAxis / QuestionStructure()` 编译得到

建议家族：

```go
type QuestionFamily string

const (
    QFRootCauseTrace   QuestionFamily = "root_cause_trace"
    QFConfigPrecedence QuestionFamily = "config_precedence"
    QFRoleLookup       QuestionFamily = "role_lookup"
    QFCallChain        QuestionFamily = "call_chain"
    QFEnumeration      QuestionFamily = "enumeration"
    QFArchitecture     QuestionFamily = "architecture"
)
```

---

## 9. 与现有代码的接点

### 9.1 Capability Projection

新增一层 deterministic projection：

- 输入：已有 `EvidenceItem`
- 输出：`EvidenceSemanticCapability`

推荐接点：

- [C:/Users/ssccv/codrax/internal/authority/authority.go](C:/Users/ssccv/codrax/internal/authority/authority.go)
- [C:/Users/ssccv/codrax/internal/tool/emit_evidence.go](C:/Users/ssccv/codrax/internal/tool/emit_evidence.go)

理由：

- 这里已经是 evidence 从“原始 emit”进入“系统投影”的边界
- 最适合一次性完成 capability 投影
- 可以和 authority projection 共享 grounding/scope/origin 信息

### 9.2 Facet Compilation

推荐在现有 `AnswerSurfacePlan` 编译阶段增量接入：

- [C:/Users/ssccv/codrax/internal/types/answer_surface_plan.go](C:/Users/ssccv/codrax/internal/types/answer_surface_plan.go)

新增：

- `CompileAnswerFacetPlan(...)`

使用现有输入：

- `QuestionStructure`
- `ExactResolution`
- `DiagramContract`
- `SurfaceEvidence`
- `CapabilitySurface`
- `DriftAnchors`
- `ConfigTraceDiagramAnchors`
- `AnswerChains / FlowFindings / LogObservedAnchors`

### 9.3 Finalizer Prompt Consumption

在：

- [C:/Users/ssccv/codrax/internal/agent/answer_document_evaluator.go](C:/Users/ssccv/codrax/internal/agent/answer_document_evaluator.go)

新增一个新的 prompt section：

- Required Answer Facets
- Allowed Claim Forms by facet
- Principal / support / prose-only / diagram-only
- Required uncertainty boundary
- Expected richness candidates

注意：

- 提示词里不能出现内部术语缩写或实现细节
- 必须把“怎么修”说成 LLM 可以直接照做的动作

### 9.4 AnswerDocument 增量注解，而不是整体推翻

不直接重做 `AnswerDocument` 为 block-only 模型。

建议保留：

- [C:/Users/ssccv/codrax/internal/types/answer_document.go](C:/Users/ssccv/codrax/internal/types/answer_document.go)

在现有 shape payload 上新增 annotation，例如：

```go
type RenderedClaimUse struct {
    FacetID     string
    EvidenceID  string
    ClaimID     string
    ClaimForm   ClaimForm
    SurfaceRole SurfaceRole
}
```

可增量放在：

- summary
- step
- symbol
- boolean rationale
- value rationale

这样：

- 复用现有 renderer
- validator 不必再全靠 prose NLU
- 不破坏现有 shape contract

### 9.5 Validator 接入点

在：

- [C:/Users/ssccv/codrax/internal/tool/emit_answer_document.go](C:/Users/ssccv/codrax/internal/tool/emit_answer_document.go)

新增以下校验器：

- `validateFacetCoverage`
- `validateClaimFormSupport`
- `validateTemporalEscalation`
- `validateAbsenceScopeBound`
- `validateDiagramFacetGraph`

这些 validator 的原则是：

- 校验 compiled contract，而不是重新猜测用户问题
- 不靠关键词 patch
- 尽量使用结构化 metadata，而不是从 prose 做二次 NLP

### 9.6 Contract / Reviewer 接入点

在：

- [C:/Users/ssccv/codrax/internal/orchestrator/contract_check.go](C:/Users/ssccv/codrax/internal/orchestrator/contract_check.go)

新增 violation family：

- `facet_uncovered`
- `claim_form_unsupported`
- `temporal_escalation`
- `absence_scope_exceeded`
- `diagram_edge_unproven`
- `richness_regression`

这样 gate / reviewer 的失败原因能直接回到编译层 contract，而不是漂成一堆 prose hint。

---

## 10. 各功能点与影响面评估

本节按功能点逐项评估当前代码、存在问题、影响面、以及引入 `Semantic Surface Contract` 后的收益。

### 10.1 用户问题理解

**现状**

- 已有 `Intent / Scenario / PredicateAxis / AnswerSubject`
- 已有 `QuestionStructure`

**缺口**

- 还没有统一编译成“本题最终必须覆盖哪些 facet”

**影响面**

- analyzer
- answer surface planner
- finalizer prompt
- contract checker

**引入后收益**

- role lookup 不会再被误当成 mechanism-heavy 题
- enumeration/bucket 问题会有显式 facet coverage
- root cause / trace / config precedence 的 rich answer 结构能统一编译

### 10.2 Evidence 生产与 grounding

**现状**

- evidence 已有 `kind / anchor_kind / scope / authority / context_role / diagram_role`

**缺口**

- 没有 claim-form capability
- support vs principal 没有显式结构

**影响面**

- emit_evidence
- deterministic emitters
- grounding
- evidence closure

**引入后收益**

- assignment 不会再越权讲成 guard
- definition 不会再越权讲成 root cause
- negative evidence 不会再越权讲成 repo-wide absence

### 10.3 Authority 与 hedging

**现状**

- authority 已成熟
- hedging 已进入 renderer

**缺口**

- hedging 还不是 claim-form aware
- 现在更多是“说弱一点”，不是“说对一种正确句式”

**影响面**

- renderer
- final answer surface
- drift-bounded root cause

**引入后收益**

- authority 决定强度
- claim-form capability 决定句式
- hedging 只做语气投影，不再承担语义修复职责

### 10.4 Exact resolution / absence

**现状**

- exact-resolution 已经是一等 contract

**缺口**

- absence scope 还不是统一结构化硬约束

**影响面**

- config/env/flag/path/symbol exact lookup
- finalizer
- validator

**引入后收益**

- “当前文件没看到”不会被说成“整个系统没有”
- exact absence 可以和 grounded nearby context 共存，但边界清晰

### 10.5 Diagram

**现状**

- diagram contract 已经存在
- grounded retry seed 已经较成熟

**缺口**

- node/edge 语义仍未完全拆开
- diagram richness 还可能和 truthfulness 冲突

**影响面**

- config trace
- call chain
- architecture
- flow/dispatch/fallback

**引入后收益**

- diagram edge 必须有 capability 支撑
- richer diagrams 可以保留，但建立在 truth-preserving graph 上

### 10.6 Drift-bounded / log / perf

**现状**

- authority + drift 已经是一等机制

**缺口**

- observed fact / current code path / nearest mechanism / uncertainty boundary 还没完全编译成 facets

**影响面**

- logtri_go
- perf trace
- 历史日志对当前代码解释

**引入后收益**

- log/perf observation 不会再被自由拼成过强 current causality
- PASS 但解释不对的问题会显著下降

### 10.7 Finalizer 与 retry

**现状**

- finalizer 已经消费很多 compiled state
- 但有些 retry 仍要靠 prose hint 猜错因

**缺口**

- 没有统一 facet-aware repair contract

**影响面**

- finalizer 轮次
- 假 PASS
- rich answer 退化

**引入后收益**

- retry 不再围绕字符串修复
- finalizer 可以围绕 facet coverage / claim-form support 精准修复

### 10.8 Renderer

**现状**

- renderer 已承接 authority hedging

**缺口**

- 还不理解 facet / claim use

**影响面**

- rich answer surface
- cite placement
- uncertainty prose

**引入后收益**

- renderer 变成真正的 semantic surface materializer
- 用户可见答案更稳定

### 10.9 Reviewer / Gate / Eval

**现状**

- contract checker 已很丰富

**缺口**

- 缺少针对 semantic facet 和 claim-form 的统一校验
- 目前仍容易出现 PASS 但答案不够好

**影响面**

- run-time gates
- eval 结果可靠性
- 假 PASS 识别

**引入后收益**

- gate 不再只检查“过没过”
- 能检查“是不是回答了正确的东西”

---

## 11. 审计要求

本节是本次设计必须附带的架构审计标准，后续实现、review、eval 都必须对照执行。

### 11.1 Prompt 审计

必须满足：

- 无内部术语直接暴露给模型或用户
- 无歧义
- 无互相矛盾的指令
- retry hint 必须是 LLM 可执行动作，不是内部组件名
- model-visible 信息必须足够完成当前修复，不可让模型靠猜
- 同一语义只出现一个权威说法，禁止一个 prompt 里多个版本

重点检查：

- [C:/Users/ssccv/codrax/internal/agent/answer_document_evaluator.go](C:/Users/ssccv/codrax/internal/agent/answer_document_evaluator.go)
- 各 tool description
- reviewer / repair hint

### 11.2 数据模型审计

必须满足：

- 每个字段只有一个主生产者
- 每个字段有明确消费者
- 每个字段有测试覆盖
- 每个字段的端到端链路可追踪
- 不允许“写了但没人读”
- 不允许多个模块对同一语义重复推导

重点关注：

- `authority`
- `question_structure`
- `answer_surface_plan`
- 新增 `claim capability / facet plan`

### 11.3 泛化审计

必须满足：

- 不能耦合某个 testcase
- 不能耦合某个仓
- 不能耦合某种配置格式
- 不能假设最大层数
- 不能依赖关键词列表代替结构语义
- 能处理 `yaml/json/toml/ini/env/custom` 等配置形态
- 能处理 log/perf/mixed/no-artifact

### 11.4 Gate 一致性审计

必须满足：

- 闸门之间不能互相打架
- 一个 gate 不能基于 prose 判断，另一个 gate 基于 compiled state 判断同一语义
- diagram node/edge 校验要一致
- absence 必须和 scope 一起校验
- authority / drift / exact-resolution / diagram / facet 不允许出现互相矛盾的 hard gate

### 11.5 信息充分性审计

必须满足：

- 模型看到的信息足够完成任务
- 不把必须的信息藏在系统不可见层
- 若 evidence 不足，要让模型显式输出 uncertainty boundary，而不是沉默或乱补
- rich answer 所需 spine 若已存在，必须可见

### 11.6 Richness 审计

必须满足：

- 不能只追求少轮次
- hard-required facets 不能缺
- 有 evidence 支撑的 richness candidate 不能系统性丢失
- 需要图例的必须有图例
- diagram 不能为了保守而压成无信息价值的最小图
- 不能只输出 hedge 而没有机制主体

### 11.7 死代码与重复造轮子审计

必须满足：

- 一旦 capability/facet plan 成为主链，旧的局部 patch helper 要及时下线
- 不允许 parallel policy engines
- 不允许多个“看起来都像 authoritative source”的 helper 长期共存
- 所有 superseded detector / normalize / heuristic 都要纳入清理计划

### 11.8 端到端消费审计

每个关键字段都要有：

- producer
- persistence / plan compiler
- prompt consumption
- validator consumption
- render/reviewer consumption
- test/eval coverage

如果任何一环缺失，就视为未闭环。

---

## 12. 实现阶段建议

### Phase 0：只做观测，不改行为

新增：

- `ProjectEvidenceClaimCapabilities(...)`
- trace/debug artifact 输出

目标：

- 先看 capability 投影是否稳定
- 不引入行为变化

### Phase 1：编译 AnswerFacetPlan，但不强 gate

在 `AnswerSurfacePlan` 中加入：

- `FacetPlan`

目标：

- 诊断 PASS 但答案薄
- 诊断 claim-form mismatch

### Phase 2：finalizer prompt 消费 facet

目标：

- 让 LLM 围绕 compiled facet 作答
- 降低自由发挥错误

### Phase 3：shape 内增加 claim/facet annotation

目标：

- 保持现有 shape
- 让 validator 不再完全依赖 prose NLU

### Phase 4：validator 正式启用

启用：

- `validateFacetCoverage`
- `validateClaimFormSupport`
- `validateTemporalEscalation`
- `validateAbsenceScopeBound`
- `validateDiagramFacetGraph`

### Phase 5：richness contract

目标：

- 保证答案不变薄
- 把“正确但没价值”的答案识别出来

### Phase 6：清理旧 helper / 旧补丁

目标：

- 清掉被 capability/facet 主链替代的局部逻辑
- 防止两套机制长期共存

---

## 13. 测试与验收策略

### 13.1 单测

必须覆盖：

- assignment 不得升级成 guard condition
- definition 不得升级成 root cause principal
- historical observation 不得升级成 current mechanism
- absence 必须绑定 scope
- diagram node 与 edge 分离校验
- soft/hard/optional richness tier 编译正确

### 13.2 集成测试

至少覆盖：

- root cause trace
- config precedence
- role lookup
- call chain
- enumeration / bucket
- architecture

每类都要覆盖：

- rich answer
- uncertainty boundary
- diagram truthfulness
- no fake PASS

### 13.3 live eval 审计

不能只看 PASS。

每次 eval 必须看：

- 最终渲染答案
- finalizer 迭代次数
- rich facet 是否缺失
- diagram 是否准确
- uncertainty boundary 是否合适
- 是否存在“解释和代码对不上”

### 13.4 回归要求

本方案上线过程中：

- 不允许引入 `go test ./...` 新失败
- 不允许让现有 config-trace / role lookup / call-chain / logtriage 已收敛 case 退化
- 所有新增字段必须有端到端测试

---

## 14. 最终开发要求

后续实现必须满足以下硬要求：

1. 不拟合某个问题，不拟合某个仓，不拟合某种配置格式。
2. 不再堆散装 heuristic；同类语义统一进 `ClaimCapability + AnswerFacetPlan`。
3. 不引入第二真理源；复用已有 authority / question_structure / answer_surface_plan 主链。
4. 不让 finalizer 自由决定 principal claim 结构；finalizer 只在 compiled contract 内组织语言。
5. 不为了减少轮次牺牲答案丰富度。
6. 不允许字段只生产不消费，不允许字段只消费不验证。
7. 不允许 gate 之间相互冲突，不允许一边强制 rich answer、一边把 rich answer 的必要 evidence 判掉。
8. 不允许保留已经失效的旧 helper、旧 patch、旧 prompt 规则误导后续开发。

---

## 15. 结论

最新远端架构已经把：

- 证据强度
- 证据来源
- drift
- 用户问题结构义务
- answer surface plan

这些关键层做成了一等机制。

下一步真正缺的，不是再加一些 case rule，而是补上第三条正交轴：

> evidence 允许支持什么语义句式，以及答案必须覆盖哪些语义 facet。

因此推荐路线是：

- 保留现有 shape/renderer 主链
- 新增 `EvidenceClaimCapability`
- 新增 `AnswerFacetPlan`
- 用 facet-aware validator 和 richness contract 收口
- 逐步淘汰旧的局部 patch 和重复语义推导

这样才能同时实现：

- 用户问题聚焦
- 最终答案真实且丰富
- finalizer / explorer 稳定收敛
- 泛化而不拟合
