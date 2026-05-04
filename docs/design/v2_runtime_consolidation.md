# V2 运行时收口整改 — 开发指导文档

状态:**Proposal — 待评审**
代码基线:`origin/main@1f49316`(2026-05-04)
对应需求:用户 2026-05-04 6-问题整改清单
预期跨度:~25 commits / 6 阶段

---

## 0. 文档结构

- §1 摸底结论(与原需求清单的差异)
- §2 架构愿景(unified V2 runtime)
- §3 红线与不变量(每阶段必须保的)
- §4–§9 阶段 1–6 详细任务(commit 粒度)
- §10 跨阶段依赖与排序
- §11 验证与回归策略

---

## 1. 摸底结论(与原需求清单的差异)

6 个域全部代码摸底完成,**3 处与原需求清单的判断有出入**,影响泛化方向:

### 1.1 Problem B(full vs patch dual protocol):**不存在 schema 双协议**

`AnswerDocumentV2Patch` (types/answer_document_v2_patch.go:55-76) **本身已经是 mutation 模型**:
- 显式 op:`UnchangedBlockIDs` / `ReplaceBlocks` / `AddBlocks` / `RemoveBlockIDs` / `ReplaceCitations` / `AppendCitations` / `Replace*`
- block 字段定义**完全复用** `emitAnswerBlockV2`(无重复 schema)
- 已有共享 merge:`ApplyAnswerDocumentV2Patch(prev, p)` (types/answer_document_v2_patch.go:113-195)

**真正缺口**:
1. **Patch 应用后未 re-validate**:`ApplyAnswerDocumentV2Patch` 只跑结构性校验(unique block id / diagram body 非空),不调用 `contract_check_block` 系列(facet / claim_use / diagram edge / absence / richness)
2. **Mutation surface 描述分散**:patch op 的语义(如 "ReplaceCitations 与 AppendCitations 互斥")只在结构 comment 里,没有统一文档化的 mutation contract
3. **V1 emit (`tool.EmitAnswerDocument`)** 的 dispatch 边界还在 (cmd/root.go:2374) — 仅用于 "v2" 字段透传,但工具仍注册

→ **泛化方向调整**:不新增 `AnswerDocumentMutation` 类型(YAGNI),而是收口到 **post-merge contract validation chokepoint** + V1 工具退役 + mutation surface 统一 godoc。

### 1.2 Problem E(richness contract):**FacetTier 三档已存在**

`facet_plan.go:218-224` 已定义 `TierEssential` / `TierExpected` / `TierEnrichment`。

**真正缺口**:
- `validateFacetCoverage` (contract_check_block.go:696-747) 把 `Required[]` 里的 Essential 和 Expected **同等硬卡**(line 720 显式 skip TierEnrichment,但 Essential 与 Expected 没有差异化处理)
- `validateRichnessRegression` (contract_check_block.go:762-814) 只对 Optional 做 telemetry,即使 `len(SourceCandidate) > 0` 也不升级
- 缺少 evidence-sufficient 信号驱动的 **Expected 升级路径**

→ **泛化方向**:不引入新枚举,直接给 validator **按 Tier 分支**:
- `Essential` → 总是硬卡(现状保持)
- `Expected` → 仅当 `len(SourceCandidate) > 0` 时硬卡(evidence-sufficient gate)
- `Enrichment` → telemetry-only(现状保持)

这是**精确信号驱动的硬约束**(`SourceCandidate` 是 analyzer 编译时确定的 typed list,不是噪声),符合"精确信号才能用作硬约束"红线。

### 1.3 Problem C(diagram relation):**EdgeFacets 字段已声明但 validator 完全不读**

- `DiagramFacetGraph.EdgeFacets []string` (answer_semantic_view.go:141) — 定义了但 validator 0 引用
- `parseMermaidEdges` (contract_check_block.go:283-308) **主动 strip** edge 标签(line 320-334 `splitMermaidEdgeLine` 删除 `|...|`)— 关系信息在解析阶段就丢失
- `RenderedClaimUse` (rendered_claim_use.go:107-138) 是扁平注解,无 `FromNode`/`ToNode`,LLM 无法把 `claim_form=call_edge` 锚定到具体 mermaid edge
- ClaimForm 已有的 edge-capable 形式:`call_edge` / `guard_condition` / `import_edge` / `precedence_role`

→ **泛化方向**:**双层修复** —
1. 解析层:`mermaidEdge` struct 增加 `Label string` + `RelationHint DiagramRelationKind`(从标签词典 typed-extract)
2. claim_use 层:`RenderedClaimUse` 增加 optional `FromNode`/`ToNode` 字段(只在 `surface_role=diagram_only` 或 `claim_form` 是 edge-capable 时填)
3. validator 层:edge legality = endpoint grounding **且** (relation hint 匹配 EdgeFacets/claim_form **或** 至少一条 claim_use 的 from/to 与本 edge 端点匹配)

---

## 2. 架构愿景(Unified V2 Runtime)

整改完成后,V2 运行时的 mental model 应为:

```
┌─────────────────────────────────────────────────────────────┐
│  ANALYZER (compile-time, once per Run)                     │
│  ├─ QuestionFamily classification                          │
│  ├─ FacetCoverageContract { Essential, Expected, Enrich }  │
│  ├─ AnswerSemanticView                                     │
│  │    ├─ RequiredBlocks                                    │
│  │    ├─ AcceptableClaimForms                              │
│  │    └─ DiagramPlan { Required, Kind, Nodes, Edges,       │
│  │                     ★EdgeRelations[] (NEW, typed) }    │
│  └─ Repair Hint Surface (NO internal jargon)              │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  FINALIZER (per-attempt, full or patch emit)               │
│  ├─ emit_answer_document_v2  ─┐                            │
│  └─ emit_answer_document_patch ┴─► ApplyAnswerDocumentV2Patch
│                                          │                 │
│                                          ▼                 │
│                            ★ postMergeContractCheck (NEW)  │
│                                          │                 │
│                                          ▼                 │
│                                    AnswerDocumentV2        │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  CONTRACT CHECK (single chokepoint, runs on merged doc)   │
│  ├─ validateRequiredBlockCoverage                          │
│  ├─ validatePrincipalClaimUse                              │
│  ├─ validateDiagramEdgeSupport ★ relation-aware (CHANGED)  │
│  ├─ validateFacetCoverage ★ Tier-branched (CHANGED)        │
│  ├─ validateRichnessRegression                             │
│  ├─ validateAbsenceScopeBound                              │
│  └─ → []Violation                                          │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  REPAIR ROUTER (★ root-cause-aware, NEW)                   │
│  ├─ ClassifyViolations → Clusters (typed co-occurrence)    │
│  ├─ For each cluster: pick PrimaryViolation                │
│  ├─ Build RepairPlan { Owner, Primary, Supporting,         │
│  │                     EscalationAllowed }                 │
│  └─ Orchestrator dispatches per RepairPlan.Owner           │
└─────────────────────────────────────────────────────────────┘
```

**关键不变量**:
- LLM 只见 contract 语言;internal pipeline / API 名禁止外露
- patch ≠ 第二套 carrier,而是 unified doc 的一种 mutation 表达
- diagram pass ≠ "node 找到了",而是 "edge 关系也有 typed support"
- richness 不再二分,而是 **evidence-sufficient gate** 驱动的 3 档
- repair 路由按 **typed 因果簇**,不按 violation 数量+深度

---

## 3. 红线与不变量(整改全程必守)

| # | 红线 | 触发条件 | 检查手段 |
|---|------|---------|---------|
| R1 | **L1 read-mode byte-preserved** | 任何修改 orchestrator | 现有 structural test(`TestRunReadSchedulerLoop_ByteIdentical`) |
| R2 | **JSON Schema desc 必须匹配 Go struct** | 改任何 emit_*.go 或对应 types | 4 处同步:struct + schema description + skill prompt + retry hint |
| R3 | **精确信号→硬卡 / 噪声信号→软引导** | 加新 violation kind / 新 gate | 5-Q audit:这个信号 typed 吗?会不会因 signal-side noise 误火? |
| R4 | **No internal pipeline jargon in LLM prompts** | skill / agent / repair_hint | grep `Tier-1\|Tier-2\|Graph\.\|deterministic pipeline\|backbone\|spine` 在 LLM-facing string 必须 0 |
| R5 | **No system backfill to user panel** | 任何"自动补全答案"机制 | 系统不写 BODY prose,只发 violation+hint 让 LLM 重 emit |
| R6 | **Generalization non-negotiable** | 任何看起来"针对某个 case"的 fix | 5-Q audit;不允许 keyword 黑名单 |
| R7 | **删旧前三步走** | 删 LLM-facing 指令 | grep 调用方;搜测试;搜文档 |

---

## 4. 阶段 1 — Repair Routing Root-Cause 化(~5 commits)

### 4.1 目标

把 `FallbackTargetForViolationsWithBudget` 的 "bucket-by-locus + count + depth + finalizer downgrade" 模型替换为 "typed cluster → primary violation → RepairPlan"。

**关键约束**:**不引入因果检测启发式**(噪声信号)。改为用 **typed co-occurrence rules**:某些 violation kind 在同一 Run 同一 dispatchID 下共现时,有确定的 primary/derived 关系(precise signal)。

### 4.2 设计

#### 4.2.1 新增 `RepairPlan` 与 `Cluster`

文件:`internal/orchestrator/repair_plan.go`(新)

```go
type RepairOwner string

const (
    RepairOwnerFinalizer RepairOwner = "finalizer"
    RepairOwnerExtract   RepairOwner = "extract"
    RepairOwnerExplore   RepairOwner = "explore"
    RepairOwnerTerminal  RepairOwner = "terminal"
)

type RepairCluster struct {
    Primary    types.Violation
    Derived    []types.Violation  // typed co-occurrence consequences
    Owner      RepairOwner
    Reason     string             // human-readable, for telemetry
}

type RepairPlan struct {
    Clusters          []RepairCluster
    PrimaryOwner      RepairOwner   // dispatch target (= deepest cluster's Owner)
    EscalationAllowed bool          // true if FinalizerOnly attempts exhausted
    EscalateAfterN    int
}
```

#### 4.2.2 Typed co-occurrence 表(precise signal)

文件:`internal/orchestrator/repair_cooccurrence.go`(新)

```go
// CooccurrenceRule says: when Primary kind appears in the same
// dispatch as ALL Derived kinds, the Derived ones are downstream
// consequences (their independent repair would be a no-op until
// Primary is fixed). Source: typed analyzer/extractor invariants —
// NO heuristic, NO frequency, NO similarity scoring.
type CooccurrenceRule struct {
    Primary types.ViolationKind
    Derived []types.ViolationKind
    Reason  string
}

var defaultCooccurrenceRules = []CooccurrenceRule{
    // Subject anchor missing (extract owns) ⇒ step identifier checks
    // are guaranteed to fail because there's no anchor to verify against.
    {
        Primary: types.ViolSubjectAnchorMissing,
        Derived: []types.ViolationKind{types.ViolStepIdentifierUnverified, types.ViolSymbolAnchorMismatch},
        Reason:  "step/symbol verification cannot succeed without anchor",
    },
    // Facet uncovered (explore owns) ⇒ principal claim_use can't reference
    // a facet that wasn't surfaced.
    {
        Primary: types.ViolFacetUncovered,
        Derived: []types.ViolationKind{types.ViolPrincipalClaimUseMissing},
        Reason:  "principal claim_use needs a facet to anchor to",
    },
    // Block coverage missing (extract) ⇒ claim_use within that block
    // is structurally absent.
    {
        Primary: types.ViolBlockCoverageMissing,
        Derived: []types.ViolationKind{types.ViolPrincipalClaimUseMissing, types.ViolDiagramEdgeUnsupported},
        Reason:  "missing block carries claim_use and diagram",
    },
    // ... 5–8 rules total, all derivable from current pipeline invariants
}
```

**关键**:每条 rule 必须能用 **typed pipeline invariant** 写出 Reason — 不允许是 "我们观察到这俩常一起出现"。后者是噪声信号。

#### 4.2.3 `BuildRepairPlan` 算法

```go
func BuildRepairPlan(vs []types.Violation, st RetryState) RepairPlan {
    // 1. 按 (Stage, DispatchID, EvidenceRefs 主键) 分组 violations
    // 2. 每组应用 cooccurrence rules,识别 Primary + Derived
    // 3. 未匹配 rule 的 violation = 自身 Primary
    // 4. 每个 cluster 的 Owner = Primary 的现有 ViolationKind→RepairLocus 映射
    // 5. PrimaryOwner = 所有 cluster Owner 中 depth 最深的(因为不能 finalizer-only 修 explore-locus)
    // 6. EscalationAllowed: 上一轮 PrimaryOwner 与本轮一致 且 Attempt > EscalateAfterN
}
```

#### 4.2.4 `FallbackTargetForViolationsWithBudget` 改造

旧函数**保留为 deprecated wrapper**(过渡期),但内部改为:
```go
func FallbackTargetForViolationsWithBudget(vs []types.Violation, used int) FallbackTarget {
    plan := BuildRepairPlan(vs, RetryState{FinalizerLocalUsed: used})
    return targetForOwner(plan.PrimaryOwner)
}
```

`Orchestrator.runReadSchedulerLoop` 改为直接调用 `BuildRepairPlan` 并消费 `plan.PrimaryOwner`,不再走 wrapper。

### 4.3 任务列表

| # | Subject | 文件 | 验收 |
|---|---------|------|------|
| A1 | 新增 `RepairPlan` / `RepairCluster` / `RepairOwner` 类型 + 8 条 cooccurrence rules + `BuildRepairPlan` 算法 + 单元测试 (覆盖 5 个典型 cluster 场景) | `repair_plan.go`, `repair_cooccurrence.go`, `repair_plan_test.go` | go test 全绿;rule 表里每条都能引用一处现有 pipeline invariant 文档 |
| A2 | `RetryState` 增加 `LastPrimaryOwner RepairOwner` + `OwnerStableAttempts int` 字段 + populate 路径 | `retry_state.go`, `orchestrator.go` `populateRetryState` | retry summary 多一行 "primary_owner=X stable_attempts=N" |
| A3 | `FallbackTargetForViolationsWithBudget` 改为 wrapper(deprecated marker);orchestrator 改为直接调用 `BuildRepairPlan` | `fallback_policy.go`, `orchestrator.go` | L1 byte-identical structural test 仍绿(read-mode runReadSchedulerLoop body 不变) |
| A4 | Telemetry:每次 dispatch 写 `RepairPlanTelemetry { plan_id, primary_owner, cluster_count, primary_kinds[] }` 到 trace | `orchestrator.go` 现有 trace 入口 | trace 新增可 grep 字段 `repair_plan=` |
| A5 | 真 eval rerun:s1a×2 + m1a×2 + u3a×2 三组,验证 (a) finalize-local 主问题不再被 derived 拖到 BackToExtract;(b) primary owner stable 时不会反复 ping-pong | `internal/eval/...` | trace 里新 routing decision 与旧版本对比,upstream fallback 次数 ≥ 旧版的 0.7× 或更低 |

### 4.4 红线检查

- ✅ R3 精确信号:cooccurrence rule 是 typed kind enum,不是相似度评分
- ✅ R6 泛化:rule 表里没有 family/file 特化
- ✅ L1:`runReadSchedulerLoop` body 不变,只是它调用的函数实现替换

---

## 5. 阶段 2 — Full/Patch 协议收口(~4 commits)

### 5.1 目标

承认 `AnswerDocumentV2Patch` 已是 mutation 模型这一事实,补齐**两个真正缺口**:
1. patch 应用后 contract_check_block re-validate 缺失
2. mutation surface 文档分散

**不引入** `AnswerDocumentMutation` 新类型(YAGNI;`AnswerDocumentV2Patch` 已经是)。

### 5.2 设计

#### 5.2.1 Post-merge validation chokepoint

新增:`internal/orchestrator/post_emit_validate.go`

```go
// PostEmitValidate runs the full contract_check_block suite on the
// MERGED AnswerDocumentV2 (regardless of whether it came from full
// emit or patch apply). This is the single chokepoint guaranteeing
// patch-applied documents face the same gate as fresh emits.
//
// Called from:
//   - executeAnswerDocumentV2 (full emit) — after SetAnswerDocumentV2
//   - emit_answer_document_patch.Execute — after ApplyAnswerDocumentV2Patch
func PostEmitValidate(mut *types.MutableState, view *types.AnswerSemanticView) []types.Violation {
    doc := mut.AnswerDocumentV2()
    if doc == nil {
        return nil
    }
    var vs []types.Violation
    vs = append(vs, validateRequiredBlockCoverage(doc, view)...)
    vs = append(vs, validatePrincipalClaimUse(doc, view)...)
    vs = append(vs, validateDiagramEdgeSupport(doc, view)...)
    vs = append(vs, validateFacetCoverage(doc, view)...)
    vs = append(vs, validateRichnessRegression(doc, view)...)
    vs = append(vs, validateAbsenceScopeBound(doc, view)...)
    return vs
}
```

**关键**:这个函数**不**在 emit tool 内部调用 — 它在 `runContractCheck` 阶段调用(orchestrator.go:3515)。emit tool 只负责写入。这样保持 tool ↔ contract gate 的关注点分离。

→ 改造点其实是:确保 `runContractCheck` 在 `runReadSchedulerLoop` 的 patch 路径上**也**会被触发(目前 patch 路径在 emit 后会走到 contract_check,但需要审计确认是否每条 path 都进)。

#### 5.2.2 Mutation surface godoc 统一

在 `types/answer_document_v2_patch.go` 顶端写 **mutation contract 表**:

```go
// AnswerDocumentV2Patch — Mutation Contract (canonical reference)
//
// Every operation below is the SOLE way to express the corresponding
// document change. Tools that emit patches MUST select exactly one
// op per field group. Validators MUST treat the merged doc as the
// canonical truth, NOT the patch shape.
//
// Op group         | Operations                       | Mutual exclusion
// -----------------|----------------------------------|---------------------
// Blocks           | Unchanged | Replace | Add | Remove | Replace∩Remove ∅;
//                  |                                    | Replace∩Add ∅
// Citations        | Replace | Append                  | Replace XOR Append
// ExactResolution  | Replace                           | nil = inherit
// Caveats          | Replace                           | nil = inherit
// Snippets         | Replace                           | nil = inherit
//
// Block id is LLM-provided (never system-generated). Replace targets
// existing id; Add inserts new id; Remove drops by id; Unchanged
// flows through byte-identical.
type AnswerDocumentV2Patch struct { ... }
```

#### 5.2.3 V1 emit 工具退役

`tool.EmitAnswerDocument` (cmd/root.go:2374) 当前仅作 V1 字段 reject 边界,实际已无 V1 emit 路径。

- 工具注册保留**但**改名 `tool.EmitAnswerDocumentLegacyGuard`(明确语义),仅做 V1 字段检测 + reject error
- 或者直接 unregister,把 V1 字段检测合并到 `emit_answer_document_v2.detectV1FieldsInV2Emit`

→ 选 unregister,因为 V1 detection 已在 V2 emit 内复用了同一函数。需先 grep 确认无其他 caller。

### 5.3 任务列表

| # | Subject | 文件 | 验收 |
|---|---------|------|------|
| B1 | 写 mutation contract godoc 到 `AnswerDocumentV2Patch`,补单元测试覆盖每条 mutual-exclusion 规则 | `types/answer_document_v2_patch.go`, `*_test.go` | godoc 渲染检查;test 覆盖 4 种冲突场景 |
| B2 | 审计 `runContractCheck` 调用图,确认 full emit + patch emit 两条路径**都**进 contract_check_block;补缺失路径 | `orchestrator.go` `runContractCheck` 调用入口 | 单测:patch emit 后注入一个故意违规的 block,断言 contract violation 被捕获 |
| B3 | grep 全部 `EmitAnswerDocument` callers;若仅 cmd/root.go 注册 + 测试,unregister V1 工具,把 V1 reject 逻辑合并到 V2 detect | `cmd/root.go`, `internal/tool/emit_answer_document.go`, `*_test.go` | `grep -r 'tool.EmitAnswerDocument' internal/ cmd/` 仅剩 V2 |
| B4 | retry summary `summarizeAnswerDocV2ForRetry` 增加 "from_patch=true/false" 标记(基于上次 emit 是 full 还是 patch),便于 LLM 理解 base | `retry_state.go` | retry hint 多一行可观测;不影响行为 |

### 5.4 红线检查

- ✅ R5:不写答案,只补 validation gate
- ✅ R7 删旧三步:V1 unregister 前先 grep + 测试搜
- ✅ 不引入新类型 = 不增加维护点

---

## 6. 阶段 3 — Diagram Relation-Aware 校验(~5 commits)

### 6.1 目标

让 `flow` 图的 `A -->|guard| B` 不再仅靠端点 grounded 过 gate,而是要求 `guard` 标签能映射到 `guard_condition` claim_form 或对应的 `EdgeFacets[]` 。

### 6.2 设计

#### 6.2.1 解析层:保留 edge label

文件:`internal/orchestrator/contract_check_block.go`

```go
type mermaidEdge struct {
    from  string
    to    string
    label string  // ★ NEW — preserved verbatim from |...| group or "-- text -->" form
}

// parseMermaidEdges 改为:
//   1. 提取 from / to 之外,捕获 |...| 内容 或 "--text-->" 中段
//   2. 写入 mermaidEdge.label
//   3. 对 sequence 图的 ":" 后段也做 capture(目前 line 290 直接丢弃)
```

#### 6.2.2 Typed relation 词典

文件:`internal/types/diagram_relation.go`(新)

```go
type DiagramRelationKind string

const (
    DiagramRelUnknown    DiagramRelationKind = ""
    DiagramRelCall       DiagramRelationKind = "call"       // → claim_form=call_edge
    DiagramRelGuard      DiagramRelationKind = "guard"      // → claim_form=guard_condition
    DiagramRelImport     DiagramRelationKind = "import"     // → claim_form=import_edge
    DiagramRelPrecedence DiagramRelationKind = "precedence" // → claim_form=precedence_role
    DiagramRelContain    DiagramRelationKind = "contain"    // → architecture only, no claim_form
    DiagramRelObserve    DiagramRelationKind = "observe"    // → claim_form=external_observation
)

// ClaimFormForRelation maps relation kind → required claim_form.
// Returns ClaimUnknown for relations without a typed claim form
// (e.g., contain in architecture diagrams uses block-level
// component_relation facet, not edge-level claim).
func ClaimFormForRelation(rk DiagramRelationKind) ClaimForm { ... }

// InferRelationFromLabel scans a verbatim edge label and returns
// the matched relation kind. Returns DiagramRelUnknown when the
// label doesn't match any keyword. NO fuzzy matching — exact
// substring against the canonical keyword set ONLY.
func InferRelationFromLabel(label string) DiagramRelationKind {
    // canonical keywords:
    //   "guard", "if", "when", "cond" → DiagramRelGuard
    //   "call", "invoke" → DiagramRelCall
    //   "import", "depends" → DiagramRelImport
    //   ... etc
    // No keyword found → DiagramRelUnknown (treated as "label-free edge")
}
```

**关键泛化决策**:
- **关键词字典是 typed enum** — 不允许是 grep heuristic 或 frequency-based
- 字典内容**写在 `claim_form.go` 同源** — 修改 ClaimForm enum 时同步
- LLM-facing prompt 要列出"可识别的 edge label 关键词"(否则 LLM 不知道写什么)

#### 6.2.3 ClaimUse 增加 edge-grounding(可选字段)

文件:`internal/types/rendered_claim_use.go`

```go
type RenderedClaimUse struct {
    FacetID     string      `json:"facet_id,omitempty"`
    EvidenceID  string      `json:"evidence_id,omitempty"`
    ClaimForm   ClaimForm   `json:"claim_form,omitempty"`
    SurfaceRole SurfaceRole `json:"surface_role,omitempty"`

    // ★ NEW — only populated when ClaimForm is edge-capable
    //   (call_edge / guard_condition / import_edge / precedence_role /
    //    external_observation) AND SurfaceRole == diagram_only.
    //   Identifies which diagram edge this claim grounds.
    FromNode string `json:"from_node,omitempty"`
    ToNode   string `json:"to_node,omitempty"`
}
```

#### 6.2.4 SemanticView 编译 EdgeRelations

文件:`internal/types/answer_semantic_view.go`

```go
type DiagramFacetGraph struct {
    Required      bool
    Kind          DiagramKind
    NodeFacets    []string
    EdgeFacets    []string
    // ★ NEW — typed expected relations for this diagram kind
    EdgeRelations []DiagramEdgeRelationContract
}

type DiagramEdgeRelationContract struct {
    Kind       DiagramRelationKind
    Min        int   // minimum number of edges of this kind
    ClaimForm  ClaimForm  // expected claim_form for support (may be ClaimUnknown)
}
```

每个 family 的 `compile_<family>.go` 编译时填充。例如:
- `QFCallChain` → `[{Kind: DiagramRelCall, Min: 1, ClaimForm: ClaimCallEdge}]`
- `QFConfigPrecedence` → `[{Kind: DiagramRelPrecedence, Min: 1, ClaimForm: ClaimPrecedenceRole}]`

#### 6.2.5 Validator 二层化

```go
func validateDiagramEdgeSupport(doc, view) []Violation {
    // Layer 1: endpoint grounding (现状,保留)
    if !endpointGrounded(edge, support) {
        emit ViolDiagramEdgeUnsupported{detail: "endpoint X ungrounded"}
    }

    // Layer 2 (NEW): relation legality
    rel := InferRelationFromLabel(edge.label)
    if rel == DiagramRelUnknown {
        // unlabeled edge — legal IF EdgeRelations[].Min already satisfied by labeled edges
        continue
    }
    expectedForm := ClaimFormForRelation(rel)
    if expectedForm == ClaimUnknown {
        continue  // contain / observe etc. — no claim_form required
    }
    // Find a claim_use anchored to this edge
    anchored := findClaimUseAnchoredToEdge(doc, edge.from, edge.to, expectedForm)
    if anchored == nil {
        emit ViolDiagramEdgeUnsupported{
            detail: fmt.Sprintf("edge %q→%q labeled %q lacks %s claim_use",
                edge.from, edge.to, edge.label, expectedForm),
        }
    }
}
```

### 6.3 任务列表

| # | Subject | 文件 | 验收 |
|---|---------|------|------|
| C1 | 新增 `DiagramRelationKind` enum + `ClaimFormForRelation` + `InferRelationFromLabel`(关键词字典 typed)+ 单测覆盖 6 种 relation × 5 种 label 形态 | `types/diagram_relation.go`, `*_test.go` | 字典关键词集合写入 godoc;test 覆盖 unknown label fallback |
| C2 | `parseMermaidEdges` 保留 label;`mermaidEdge` 加 `label string` 字段 | `contract_check_block.go:283-340` | 现有端点测试不退化;新测试断言 label 被捕获 |
| C3 | `RenderedClaimUse` 增加 `FromNode` / `ToNode` 字段,JSON tag omitempty;同步 emit V2 schema description(R2 红线) | `types/rendered_claim_use.go`, `tool/emit_answer_document_v2.go` schema | schema 单测:strict-decode 不破;LLM 看到的字段说明清晰 |
| C4 | `DiagramFacetGraph.EdgeRelations` 新字段;每个 `compile_<family>.go` 填充对应 contract;`AnswerSurfacePlan` 上游补充 EdgeRelations 来源 | `answer_semantic_view.go`, `compile_*.go` (8 family files), `analyzer` 端 | 8 family 的 EdgeRelations 编译结果有 unit test 锁定 |
| C5 | `validateDiagramEdgeSupport` 二层化 + skill prompt 列出可识别 label 关键词 | `contract_check_block.go`, `skill/defaults.go` | s1a + m1a 真 eval:`flow` 图带 `|guard|` 边必须命中 `guard_condition` claim_use,否则 reject |

### 6.4 红线检查

- ✅ R3:relation 关键词字典是 typed enum;LLM 写不在字典里的 label = unknown(不报错,但 EdgeRelations.Min 不满足时报错 — 这是 typed gate)
- ✅ R6:字典词条不针对任何 family/file 特化
- ⚠️ **R2 必查**:`RenderedClaimUse` 新字段 4 处同步(struct + V2 emit schema desc + skill prompt 教学 + retry hint summary)— 历史已踩过这个坑

---

## 7. 阶段 4 — Prompt 去内部术语化(~2 commits)

### 7.1 目标

清除 4 处确认的 internal jargon 残留(摸底已定位):

| 文件:行 | 现状 | 替换 |
|---------|------|------|
| `skill/defaults.go:130` | `Tier-1/Tier-2 repomap symbol` | `repo-wide registered symbol (module-level identifier)` |
| `skill/defaults.go:135` | `Graph.ImplementersOf` | `the system's structural relation oracle` |
| `agent/answer_document_evaluator.go:252` | `upstream deterministic pipeline produced this answer-symbol list` | `the prior analysis phase produced this symbol list` |
| `agent/answer_document_evaluator.go:486-490` | `upstream deterministic pipeline...complete ordered backbone...spine` | `the analysis phase identified a complete ordered sequence` |

### 7.2 任务列表

| # | Subject | 文件 | 验收 |
|---|---------|------|------|
| D1 | 替换 `skill/defaults.go` 两处 jargon;改写为 contract 语言;同步任何相关 helper | `skill/defaults.go` | grep `Tier-1\|Tier-2\|Graph\.\|ImplementersOf` 在 `skill/` 下 0 命中(测试夹具除外) |
| D2 | 替换 `answer_document_evaluator.go` 两处 backbone/pipeline 表述;`renderAnswerDocStepBackbone` 内部函数名可保留(Go 私有),但产出字符串去掉"deterministic pipeline" | `agent/answer_document_evaluator.go` | grep `deterministic pipeline\|backbone` 在 LLM-facing format string 中 0 命中 |

### 7.3 红线检查

- ✅ R4 直接对应
- ⚠️ **R7 删旧三步**:每处替换前 grep 确认:(a) 没有测试 hard-assert 这串文字;(b) 没有 docs 教这串术语作为契约
- ✅ R5:替换是去 jargon,不是替系统写答案

---

## 8. 阶段 5 — Richness Tier 强化(~3 commits)

### 8.1 目标

`validateFacetCoverage` 按 Tier 分支:
- `TierEssential` → 总是硬卡(现状)
- `TierExpected` → 仅当 `len(SourceCandidate) > 0` 时硬卡(NEW gate)
- `TierEnrichment` → telemetry-only(现状)

不引入新枚举(三档已存在);改 validator 行为 + 改 prompt 教学。

### 8.2 设计

#### 8.2.1 `validateFacetCoverage` 改造

```go
for _, req := range view.FacetCoverage.Required {
    if covered[req.Kind] {
        continue
    }
    switch req.Tier {
    case TierEssential:
        out = append(out, Violation{
            Kind: ViolFacetUncovered,
            Detail: fmt.Sprintf("essential facet %q uncovered", req.Kind),
        })
    case TierExpected:
        // ★ NEW: evidence-sufficient gate
        if len(req.SourceCandidate) > 0 {
            out = append(out, Violation{
                Kind: ViolFacetUncovered,  // same kind, different detail
                Detail: fmt.Sprintf(
                    "expected facet %q has %d evidence candidate(s) but no V2 block surfaced it",
                    req.Kind, len(req.SourceCandidate)),
            })
        }
        // else: evidence absent → no violation, downgrade to telemetry
    case TierEnrichment:
        continue  // handled by validateRichnessRegression
    }
}
```

#### 8.2.2 `SourceCandidate` 是什么 — 必须确认

`FacetRequirement.SourceCandidate []string` 当前来源:analyzer 编译 facet plan 时,从 EvidenceItem pool 里**预先匹配**到候选证据 ID。这就是 typed precise signal — 不是 grep 结果。

- 若 `SourceCandidate` 来源含模糊匹配/相似度 → **本阶段必须先把它收紧**(否则 R3 红线被破)
- 摸底 agent 没看到模糊匹配,但需要 task E0 显式审计

#### 8.2.3 LLM-facing 提示

skill/defaults.go 现已说"required facet 必须 cover";新增一条:
> "expected facets MUST be covered when their evidence pool is non-empty. The Required Answer Blocks section will list expected facets with `(evidence: N candidates)` annotation when N>0."

retry_hint 也要同步说明 violation detail("essential" vs "expected with N candidates")。

### 8.3 任务列表

| # | Subject | 文件 | 验收 |
|---|---------|------|------|
| E0 | **审计** `SourceCandidate` 的产生路径,确认是 typed evidence ID 列表(不是相似度评分);写下来源 godoc | `types/facet_plan.go` `CompileFacetCoverage`, analyzer 端编译路径 | godoc 标注产生路径 + 一处 typed invariant 引用 |
| E1 | `validateFacetCoverage` 按 Tier 分支;TierExpected 走 SourceCandidate gate;单测覆盖三档 × (covered, uncovered, evidence-empty) 共 6 种组合 | `contract_check_block.go:696-747`, `*_test.go` | s1a(QFRootCauseTrace,有 mechanism 证据)若 LLM 漏掉 NearestMechanism → reject(过去仅 telemetry) |
| E2 | skill/defaults.go + retry hint 两侧同步说明 expected facet 行为;Required Answer Blocks 渲染加 `(evidence: N)` 标注 | `skill/defaults.go`, `agent/answer_document_evaluator.go` (Required Answer Blocks 渲染处) | 真 eval s1a:LLM 在新 prompt 下能正确把 mechanism 写进 ordered_list |

### 8.4 红线检查

- ✅ R3:`SourceCandidate` 必须是 typed(由 E0 保证);非 0 长度是布尔判断,不是相似度
- ✅ R5:违规时不写答案,只返 violation + hint
- ⚠️ R2:facet_id 字段语义没变,但行为变化要在 LLM-facing 4 处同步

---

## 9. 阶段 6 — 文档与注释归零清扫(~2 commits)

### 9.1 目标

`docs/architecture.md` 仍含 shape-era 残影(摸底确认 6 处 `AnswerShape` 引用,部分未标记 deprecated),迁去 migration/。

### 9.2 任务列表

| # | Subject | 文件 | 验收 |
|---|---------|------|------|
| F1 | `docs/architecture.md` 移除 shape-era 现状描述,把历史背景迁到 `docs/migration/answer_shape_terminal_retirement.md`(已存在);保留必要的 deprecation pointer | `docs/architecture.md`, `docs/migration/answer_shape_terminal_retirement.md` | architecture.md `grep AnswerShape\|shape-era` 仅剩 "see migration/X for legacy shape mechanism" 这种 pointer |
| F2 | `internal/orchestrator/contract_check.go` 等代码注释里的 V1-era 解释段清理(post_shape_residual_audit 已列出);保留功能注释,删旧世界教学 | 见 `docs/migration/post_shape_residual_audit.md` 列表 | 代码注释 grep `shape\|V1` 仅在 migration ref / 历史 commit message 中 |

### 9.3 红线检查

- ✅ R7 删旧三步:每处删之前先 grep 确认无 active reference

---

## 10. 跨阶段依赖与排序

```
Phase 1 (Repair Routing)  ─── 独立,可先做
                                │
Phase 2 (Full/Patch)      ─── 独立,可并行
                                │
                                ▼
Phase 3 (Diagram Relation)── 依赖 Phase 2 的 post-merge validation chokepoint
                                │ (因为新增 EdgeRelations contract 要走统一 gate)
                                ▼
Phase 4 (Prompt Cleanup)  ─── 独立,可任何时候
                                │
                                ▼
Phase 5 (Richness Tier)   ─── 依赖 Phase 1 的 RepairPlan(因为 ViolFacetUncovered 升级
                                │ 后会进入新 cluster 路由,要确认 owner 仍是 explore)
                                ▼
Phase 6 (Docs Cleanup)    ─── 最后,清理所有遗留
```

**推荐 commit 顺序**(按 ROI + 风险):
1. **Phase 4(D1, D2)** — 风险最低,2 commit 立刻清掉 prompt 噪声
2. **Phase 1(A1–A5)** — 高 ROI:retry budget 立刻可观测下降
3. **Phase 2(B1–B4)** — 中 ROI:堵住 patch validation 漏洞
4. **Phase 3(C1–C5)** — 中风险:需 4 处同步,但有清晰单测
5. **Phase 5(E0–E2)** — 中 ROI:richness 真效果取决于 LLM 行为
6. **Phase 6(F1–F2)** — 收尾

---

## 11. 验证与回归策略

### 11.1 单元测试

每阶段必备:
- 新类型 / 新算法的直接 unit test
- 现有 structural test(L1 byte-identical / fail-loud)持续绿
- Schema strict-decode 测试(R2 红线)

### 11.2 真 eval(每阶段闭环)

| Phase | Eval 集 | 关键观察指标 |
|-------|--------|------------|
| 1 | s1a×2 + m1a×2 + u3a×2 | upstream fallback 次数;primary owner 稳定性 |
| 2 | 任意一个会触发 patch retry 的 case | merged doc 上 violation 是否被捕获 |
| 3 | s1a(QFRootCauseTrace,带 diagram)、m1a(QFCallChain,带 call_dag) | 故意写错 edge label,断言 reject |
| 4 | 任意 case | grep prompt log,确认 jargon 0 出现 |
| 5 | s1a(NearestMechanism 有证据但不 emit) | reject + retry 后 LLM 补全 mechanism |
| 6 | N/A | grep doc + 注释 |

### 11.3 红线 audit checklist

每个 commit 提交前过一遍:
- [ ] R1 L1 byte-identical structural test 绿
- [ ] R2 4 处同步(struct / schema desc / skill / hint)
- [ ] R3 任何新硬卡走的是 typed signal
- [ ] R4 LLM-facing strings grep 无内部术语
- [ ] R5 系统不写 BODY 内容
- [ ] R6 5-Q audit 通过(没有 case 特化)
- [ ] R7 删任何东西前先 grep 调用 / 测试 / 文档

---

## 12. 一句话总结

> **不是新增 4 个抽象层,而是承认现有 typed contract 的存在(FacetTier、AnswerDocumentV2Patch 已是 mutation),把缺失的 chokepoint 补齐(post-merge validate、relation-aware diagram、tier-branched facet validator、cooccurrence-based repair plan),把残留的 noise(internal jargon、unread EdgeFacets、untracked edge labels)清掉。**
