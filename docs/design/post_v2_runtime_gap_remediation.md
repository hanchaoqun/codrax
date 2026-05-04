# Post-V2 Runtime Gap Remediation — 详细设计

| 字段 | 值 |
| --- | --- |
| **状态** | 设计阶段(NOT YET IMPLEMENTED) |
| **代码基线** | `main @ 3d904ba`(`origin/main@2882032` 之后已 fast-forward) |
| **前置工作** | V2 runtime consolidation Phase 1-6 已 SHIPPED(`docs/design/v2_runtime_consolidation.md`) |
| **本文目的** | 把"剩余架构缺口整改需求"逐项对照现有代码,锁定结构、字段、消费点、测试,作为后续实施的工作底稿 |
| **不在本文范围** | 历史修复回顾、已退役字段、与 write mode 主线无关的 work |

> **重要红线锚点**(每条都已在 memory 内,实施时按 `feedback_prompt_redline_checklist.md` checklist 逐条过关):
> - **L1**:read-mode `runReadSchedulerLoop` body 字面文本不变。任何调用层的算法替换只动其 callee 实现。
> - **R2'**:加新 typed signal 必须 6 处同步 — Go struct / tool schema description / skill prompt / retry hint summary / JSON decoder MisplacedFieldHint / cooccurrence rule + RepairLocus 映射。
> - **R3**:精确信号才能用作硬约束。frequency / similarity / fuzzy / heuristic 只能驱动 SOFT 引导。
> - **R4**:LLM-facing prose 禁含内部模块名 / stage codename / Go 类型名 / Phase / Layer / Tier 编号 / acronym。
> - **R5**:不在 prompt 里写出"答案应该是什么"。
> - **R6**:不允许 case-specific 例子(verbatim 真 user question / 真 method name / 真 path)。
> - **R7**:删旧三步走(grep → 全改 → 不留 stale reference)。
> - **SST**:任何 ≥2 处使用的字面量(键值 / 段标题 / 字段名 / 字典)必须抽常量,使用 `internal/context/section_titles.go` + `internal/types/diagram_relation.go` 已建立的模式。

---

## 0. 整改优先级总览

| 序号 | 缺口 | 优先级 | 复杂度 | 预估 commit | ROI |
| --- | --- | --- | --- | --- | --- |
| **G1** | RepairPlan → RepairExecutionPlan 多簇有序执行 | P1(最高) | 高 | 5 | 直接降轮次 |
| **G2** | full/patch 双维护面 → Unified Mutation Contract | P2(中长期) | 高 | 4 | 防长期回归 |
| **G3** | Diagram label-driven → typed `relation_kind` 优先 | P1(高) | 中 | 4 | 解开词汇绑定 |
| **G4** | Helper/Evaluator dynamic prompt 审计扩展 | P3 | 低 | 2 | 防开发心智污染 |
| **G5** | Semantic Quality Reviewer(完整性 reviewer) | P2 | 中 | 3 | 抓"答得太薄" |
| **G6** | Richness Tier 三档化 + 全消费打通 | P2(高) | 中 | 3 | 评测体感 |
| **G7** | Strict decode 长期 path-sensitive 预扫描 | P4(长期) | 低 | (悬挂) | 边界 misplacement |

执行序列:**G1 → G3 → G6 → G2 → G5 → G4 → (G7 悬挂)**。  
G2 排在 G6 之后是因为:G1 / G3 / G6 都会触动 emit 字段,放在 G2 收口后会被 unified mutation 自动拉齐;反过来若先做 G2,G1/G3/G6 的字段添加会同时打两套面。

---

## 1. G1 — RepairExecutionPlan 多簇有序执行

### 1.1 现状对照

**代码锚点**

| 文件 | 行 | 角色 |
| --- | --- | --- |
| `internal/orchestrator/repair_plan.go` | 64-87 | `RepairPlan{Clusters[], PrimaryOwner, HasFailLoud}` |
| `internal/orchestrator/repair_plan.go` | 108-166 | `BuildRepairPlan` — clusterize + 选 deepest owner |
| `internal/orchestrator/repair_plan.go` | 251-267 | `sortClustersDeepestFirst` — clusters 已按 deepest 排序 |
| `internal/orchestrator/repair_cooccurrence.go` | 82-287 | 7 条 typed cooccurrence rules(C1..C7 + C6.1) |
| `internal/orchestrator/fallback_policy.go` | 502-580 | `FallbackTargetForViolations*` 仅消费 `plan.PrimaryOwner` |
| `internal/orchestrator/retry_state.go` | 42-93 | `populateRetryState` 已写 `LastPrimaryOwner / OwnerStableAttempts / LastPrimaryViolation`,但**未存 `RemainingOwners`** |
| `internal/orchestrator/orchestrator.go` | 3708-3810 | retry-decision site 仅消费 `FallbackTarget`(单值),`requeueToStage` 单步触发 |

**确认**:
- ✅ 多簇识别已落地(`Clusters[]` + 7 typed rules)
- ✅ deepest 已排序(`sortClustersDeepestFirst`)
- ❌ 调度只消费 `PrimaryOwner`(单值),浅 cluster 被丢弃直到下一轮 retry 重新跑 `BuildRepairPlan` 才有机会
- ❌ 跨 retry window 的"下一个 owner"没有持久化,每次 retry 都从 violation list 重新推

### 1.2 缺口诊断

当一次 violator pass 同时含 2+ 独立 root-cause cluster 时(典型:finalize-local citation drift + extract-local SubjectAnchorMissing):
- 当前:挑 deepest(extract)→ rerun extract → 答案重生成 → 同样的 finalize-local citation drift 再次出现 → 第二轮 retry 修。
- 期望:第一轮已确定的浅 cluster(finalizer-local)应该排队作为后续 dispatch 的候选,而不是被新的 BuildRepairPlan 重算。

> 这里**不**做"一次 dispatch 多 agent 并发"——orchestrator 仍单 agent dispatch。变化的是"plan 持久化 + 顺序消费"。

### 1.3 详细设计

#### 1.3.1 新类型 `RepairExecutionPlan`

`internal/orchestrator/repair_plan.go` 新增:

```go
// RepairExecutionPlan is the dispatch-ready execution view of a
// RepairPlan. It is built ONCE per validator failure and persisted
// across retry attempts in RetryState so the orchestrator can
// progressively consume OrderedOwners without re-clustering on each
// retry. CurrentOwner drives the next dispatch; RemainingOwners is
// the queue of yet-to-execute cluster owners (deepest-first).
//
// Lifetime:
//   1. Validator returns violations → BuildRepairExecutionPlan(...).
//   2. Plan is stashed on RetryState.ExecutionPlan.
//   3. Orchestrator dispatches CurrentOwner via FallbackTargetForLocus.
//   4. After agent completes (success OR fresh violation set),
//      PromoteNextOwner(...) is called:
//      - On fresh violation set: rebuild plan (BuildRepairExecutionPlan
//        sees the new violations; old RemainingOwners is discarded).
//      - On no new violations but still failing oracles: pop next
//        owner from RemainingOwners; CurrentOwner becomes that.
//   5. RemainingOwners empty AND oracles still failing AND
//      OwnerStableAttempts > budget → escalate.
//
// R3 invariant: every owner in OrderedOwners derives from a
// RepairCluster.Owner — pure typed signal, no heuristic.
type RepairExecutionPlan struct {
    // Clusters is the same partition BuildRepairPlan computed,
    // preserved verbatim for telemetry.
    Clusters []RepairCluster

    // OrderedOwners is the deduplicated, deepest-first sequence of
    // distinct cluster owners. Length ≤ len(Clusters); duplicates
    // (two clusters owned by LocusFinalizer) collapse to one entry —
    // dispatching Finalizer once already addresses both Finalizer-
    // owned clusters since fixing Finalizer-local issues happens in
    // a single re-emit.
    OrderedOwners []RepairLocus

    // CurrentOwner is the locus the next dispatch will target. Equal
    // to OrderedOwners[0] right after Build; advances via
    // PromoteNextOwner.
    CurrentOwner RepairLocus

    // RemainingOwners is OrderedOwners[1:] at Build time; shrinks as
    // PromoteNextOwner consumes them.
    RemainingOwners []RepairLocus

    // EscalationAllowed gates the "after exhausting RemainingOwners
    // AND OwnerStableAttempts >= budget, escalate to FailLoud"
    // path. Defaults to true; tests / specific kinds may flip.
    EscalationAllowed bool

    // HasFailLoud is preserved from the legacy single-owner plan —
    // a LocusTerminal cluster forces the whole plan to fail_loud
    // regardless of remaining owners.
    HasFailLoud bool
}
```

#### 1.3.2 算法

```
BuildRepairExecutionPlan(vs):
    plan := BuildRepairPlan(vs)                        # 复用现有
    if plan.HasFailLoud:
        return RepairExecutionPlan{
            Clusters: plan.Clusters,
            CurrentOwner: LocusTerminal,
            HasFailLoud: true,
        }

    seen := {}
    ordered := []
    for c in plan.Clusters:                            # 已 deepest-first
        if c.Owner in seen: continue
        seen[c.Owner] = true
        ordered.append(c.Owner)

    # R2.2 finalize-local downgrade 在 OrderedOwners 内仍生效:
    # 若 finalizer 在队列里且 budget 未耗尽,把它前置到 [0]。
    # 这保留了 cost-opt 语义(廉价重试先跑),与现有
    # FallbackTargetForViolationsWithBudget 等价但持久化。
    if budget > 0 and finalizerLocalUsed < budget:
        if LocusFinalizer in ordered and ordered[0] != LocusFinalizer:
            promote LocusFinalizer to ordered[0]

    return RepairExecutionPlan{
        Clusters: plan.Clusters,
        OrderedOwners: ordered,
        CurrentOwner: ordered[0],
        RemainingOwners: ordered[1:],
        EscalationAllowed: true,
    }


PromoteNextOwner(prev, freshViolations):
    if len(freshViolations) > 0:
        return BuildRepairExecutionPlan(freshViolations)   # 全重算
    if len(prev.RemainingOwners) == 0:
        return prev   # 上层根据 OwnerStableAttempts 决定 FailLoud
    return RepairExecutionPlan{
        Clusters: prev.Clusters,
        OrderedOwners: prev.OrderedOwners,
        CurrentOwner: prev.RemainingOwners[0],
        RemainingOwners: prev.RemainingOwners[1:],
        EscalationAllowed: prev.EscalationAllowed,
    }
```

#### 1.3.3 R2' 6 处同步映射

| 同步点 | 文件 / 函数 | 改动 |
| --- | --- | --- |
| (1) Go struct | `internal/types/retry_state.go` `RetryState` | 新增 `ExecutionPlan *RepairExecutionPlanSummary`(typed snapshot,见下) |
| (2) Tool schema | N/A — 内部 routing,不进 LLM emit schema | 仅 retry hint render 受影响 |
| (3) Skill prompt | N/A — orchestrator 内部决策,不教 LLM | 不动 |
| (4) Retry hint summary | `internal/orchestrator/retry_state.go` `populateRetryState` | 写入 `ExecutionPlan`;`RetryBlockSummary`(LLM-facing)只渲染**当前 dispatch 的 owner 含义**,不暴露 RemainingOwners 编号给 LLM(R4 红线) |
| (5) JSON decoder remap | N/A | 不动 |
| (6) Cooccurrence rule / RepairLocus 映射 | `internal/orchestrator/fallback_policy.go::LocusOfTarget` | 不动(已是 typed bridge) |

> **关键 R4 抉择**:RemainingOwners 是**调度内部状态**,不渲染给 LLM。LLM 只看到"上一轮 owner"含义,且用 stage-agnostic prose("the investigation step" / "the answer-rendering step")。`RetryState.ExecutionPlan` 用一个 typed 字段,LLM-facing 串只用其结果,不暴露字段名。

#### 1.3.4 BusContext / MutableState 持久化

`internal/types/mutable_state.go`(若不存在则在 retry_state.go 内挂):

```go
type RepairExecutionPlanSummary struct {
    OrderedOwners   []string  // RepairLocus 的字符串化,纯 telemetry
    CurrentOwner    string
    RemainingOwners []string
    EscalationAllowed bool
}

// MutableState.SetRepairExecutionPlan / RepairExecutionPlan() 访问器
```

存在 RetryState 上而不是单独 BusContext 字段:确保 ResetForFallback 时一并清(避免跨 Run 漂移)。

#### 1.3.5 Orchestrator 改造点

`internal/orchestrator/orchestrator.go` 第 ~3700-3810 行(retry-decision site):

```diff
- preDowngrade := FallbackTargetForViolations(res.Violations)
- fallback := FallbackTargetForViolationsWithBudget(res.Violations, finalizerLocalRetriesUsed)
+ // 1. 取上一轮 ExecutionPlan;若新 violation pattern 不同则全重算
+ prevPlan := mut.RepairExecutionPlan()
+ var execPlan RepairExecutionPlan
+ if shouldRebuildExecutionPlan(prevPlan, res.Violations) {
+     execPlan = BuildRepairExecutionPlan(res.Violations, finalizerLocalRetriesUsed)
+ } else {
+     execPlan = PromoteNextOwner(*prevPlan, nil)
+ }
+ mut.SetRepairExecutionPlan(execPlan)
+ fallback := targetForLocus(execPlan.CurrentOwner)
+ if execPlan.HasFailLoud { fallback = FallbackFailLoud }
+ preDowngrade := fallback  // budget 语义保留
```

`shouldRebuildExecutionPlan` 的判定:

- prev 为 nil → 全建。
- prev.RemainingOwners 已耗尽 → 全建(强制重新评估)。
- 新 violation kind set 与 prev.Clusters 内 kind set 不同 → 全建(说明 owner 修复后浮出新 root cause)。
- 新 violation kind set ⊆ prev,只是数量减少 → `PromoteNextOwner`(上一个 owner 修了部分,继续推进)。

> **L1 红线检查**:`runReadSchedulerLoop` 的字面 body 不变。本次改动只改 retry-decision 路径(它是 callee,不在 byte-identical scope 内)。

#### 1.3.6 Telemetry

复用 `SummarizeRepairPlan` 模式新增 `SummarizeRepairExecutionPlan`:

```
exec_plan=<current> ordered=[<o1>,<o2>,...] remaining=<n> stable_attempts=<k>
```

emit 在 `eval/run.sh` 加 `repair_exec_plan_*` metric,real-eval 验证多簇 case。

#### 1.3.7 测试锁

`internal/orchestrator/repair_plan_execution_test.go`(NEW):

| 测试 | 锁定不变量 |
| --- | --- |
| `TestBuildExecutionPlan_SingleCluster` | 单 cluster → OrderedOwners 长度 1,RemainingOwners 空 |
| `TestBuildExecutionPlan_MultiClusterDeduped` | 2 个 finalizer cluster + 1 explore cluster → OrderedOwners 长度 2(去重) |
| `TestBuildExecutionPlan_FinalizerLocalPromoted` | budget 未耗尽时 finalizer 提前到 [0] |
| `TestBuildExecutionPlan_HasFailLoudShortCircuit` | LocusTerminal cluster → CurrentOwner=LocusTerminal,无队列 |
| `TestPromoteNextOwner_FreshViolations` | 新 kind set → 重建 |
| `TestPromoteNextOwner_StableSubset` | 子集减少 → 弹下一个 |
| `TestPromoteNextOwner_Exhausted` | 队列空 → CurrentOwner 不变,EscalationAllowed=true 触发上层 FailLoud |
| `TestExecutionPlan_NoLLMFacingJargon` | grep `RetryBlockSummary.Render*` 输出不含 "RepairExecutionPlan" / "RemainingOwners" / "OrderedOwners" |

#### 1.3.8 实施 commit 序列(5 commits)

1. `repair: typed RepairExecutionPlan + Build/Promote helpers + 8 单测`
2. `repair: persist ExecutionPlan on MutableState + Reset 清理`
3. `repair: orchestrator retry-decision 切到 ExecutionPlan + L1 byte-identical 验证`
4. `repair: telemetry repair_exec_plan_* metric + eval/run.sh 行`
5. `repair: real-eval 验证多簇 case + 文档 v2_runtime_consolidation.md §X 补章`

#### 1.3.9 风险 & 回滚

- 风险:L1 byte-identical 误伤 — `runReadSchedulerLoop` 内含 `FallbackTargetForViolationsWithBudget(...)` 直接调用?**已确认**(grep)retry-decision 在 callee `runContractCheck` 路径,不在 read-loop body。
- 回滚:`shouldRebuildExecutionPlan` 永远返回 true → 退化成"每轮重算"(行为等价于 V2 runtime Phase 1-A3 现状)。

---

## 2. G2 — Unified Mutation Contract:full/patch 收敛

### 2.1 现状对照

| 文件 | 行 | 角色 |
| --- | --- | --- |
| `internal/tool/emit_answer_document_v2.go` | 34-69 | `emitAnswerDocumentV2Params{DocumentModel, Blocks, ...}` |
| `internal/tool/emit_answer_document_v2.go` | 102-220 | full decode → 构造 `*types.AnswerDocumentV2` → `SetAnswerDocumentV2(doc)` |
| `internal/tool/emit_answer_document_patch.go` | 41-225 | patch decode → `convertEmitBlocksToTyped` → `ApplyAnswerDocumentV2Patch(prev, patch)` → `SetAnswerDocumentV2FromPatch(merged)` |
| `internal/types/answer_document_v2_patch.go` | 144-187 | `ApplyAnswerDocumentV2Patch` 实现 |
| `internal/orchestrator/retry_state.go` | 53-66 | `populateRetryState` 读 `mut.AnswerDocumentV2()` + `mut.LastEmitFromPatch()` |

**确认**:
- ✅ contract validation 已统一(`runContractCheck` 读 `mut.AnswerDocumentV2()` = merged doc;两条 emit 路径走同一 chokepoint)
- ❌ decode + struct 构建 + per-block validation 在两个文件里写了**两次**(`executeAnswerDocumentV2` 的 line ~155-220 与 `convertEmitBlocksToTyped` line 252-307 内含同样的 kind 校验、surface_role 校验、items 转换、diagram 转换)
- ❌ retry summary `summarizeAnswerDocV2ForRetry` 读 merged doc(已统一),但 patch lineage flag `LastEmitFromPatch` 是双面遗留(本身无害,可保留作 telemetry)

### 2.2 缺口诊断

> "字段演进必须同时覆盖 full decode、patch decode、patch apply、retry summary 四处" — 上一句的"四处"实际现状:
>
> - **full decode**:`emit_answer_document_v2.go::executeAnswerDocumentV2` 内联实现
> - **patch decode**:`emit_answer_document_patch.go::convertEmitBlocksToTyped` 复制粘贴的 per-block 校验
> - **patch apply**:`internal/types/answer_document_v2_patch.go::ApplyAnswerDocumentV2Patch`
> - **retry summary**:`summarizeAnswerDocV2ForRetry`(已读 merged)

只要把 "**emitAnswerBlockV2 → types.AnswerBlock**" 转换抽到一个 normalizer,full / patch 共用,就能把字段演进的同步面从 2 处降到 1 处。

### 2.3 详细设计

#### 2.3.1 抽 `internal/tool/answer_block_normalize.go`(NEW)

```go
// NormalizeEmitAnswerBlock converts the JSON-shape emitAnswerBlockV2
// to the typed types.AnswerBlock. Both full emit
// (executeAnswerDocumentV2) and patch emit (convertEmitBlocksToTyped)
// MUST go through this single normalizer so a new typed annotation
// field added to AnswerBlock is automatically picked up by both
// paths.
//
// fieldPath is the JSON pointer-style prefix the caller passes for
// error messages — e.g. "blocks[3]" (full emit) or
// "replace_blocks[1]" (patch emit). Errors name the field so the
// LLM sees the correct location verbatim.
func NormalizeEmitAnswerBlock(in emitAnswerBlockV2, fieldPath string) (types.AnswerBlock, error) {
    // ... 单一实现:id 唯一性/kind valid/surface_role/items[] 转换/diagram 转换/edge_anchors 透传 ...
}
```

`executeAnswerDocumentV2` 与 `convertEmitBlocksToTyped` 都改为调用 `NormalizeEmitAnswerBlock`。

#### 2.3.2 抽统一 `MutationContract`(longer-term)

```go
// internal/types/answer_document_v2_mutation.go (NEW)

// AnswerDocumentMutation is the unified document-change model.
// Both full emit (ReplaceAll) and patch emit (Partial) lower to
// this type. Validators / retry summary / telemetry only read the
// resulting merged doc — they never see the raw JSON shape.
type AnswerDocumentMutation struct {
    Kind    MutationKind                 // ReplaceAll | Partial
    Replace *AnswerDocumentV2            // when Kind==ReplaceAll
    Patch   *AnswerDocumentV2Patch       // when Kind==Partial
}

// Apply lowers the mutation onto prev (may be nil for ReplaceAll).
// Returns the merged doc the chokepoint validator inspects.
func (m AnswerDocumentMutation) Apply(prev *AnswerDocumentV2) (*AnswerDocumentV2, error)

// Summary renders a one-line audit trail for telemetry: e.g.
// "replace_all blocks=4 citations=2" or "patch unchanged=2 replace=1 add=0 remove=1".
func (m AnswerDocumentMutation) Summary() string
```

`SetAnswerDocumentV2` / `SetAnswerDocumentV2FromPatch` 改为内部走 `ApplyMutation(mut)`,外部接口保留(向后兼容)。

#### 2.3.3 R2' 6 处同步影响

| 同步点 | 改动 |
| --- | --- |
| (1) struct | `AnswerDocumentMutation` 新建,`AnswerBlock` 字段不变 |
| (2) tool schema | full / patch 两个 tool 的 description 都引用同一段 "block normalization spec"(SST 抽常量) |
| (3) skill prompt | 不动 — LLM 仍看到两个 tool,但行为一致 |
| (4) retry hint summary | 不动 — 已读 merged doc |
| (5) JSON decoder | `answerDocumentV2MisplacedHints` 已两处共享 |
| (6) cooccurrence | 不涉及 |

#### 2.3.4 测试锁

| 测试 | 锁定不变量 |
| --- | --- |
| `TestNormalizeEmitAnswerBlock_FullAndPatchAgree` | 同一 emitAnswerBlockV2 输入,full / patch path 产出 byte-identical AnswerBlock |
| `TestMutationApply_ReplaceAllEquivalentToFullEmit` | Mutation{Kind:ReplaceAll,Replace:doc}.Apply(nil) == doc |
| `TestMutationApply_PartialEquivalentToPatchApply` | Mutation{Kind:Partial,Patch:p}.Apply(prev) == ApplyAnswerDocumentV2Patch(prev,p) |
| `TestMutationSummary_NoLLMFacingJargon` | grep summary 输出不含 internal jargon |

#### 2.3.5 实施 commit 序列(4 commits)

1. `tool: extract NormalizeEmitAnswerBlock — single-source per-block normalizer`
2. `types: AnswerDocumentMutation typed contract + Apply/Summary`
3. `tool: full emit & patch emit lowered to Mutation.Apply`
4. `audit: lock test full=patch byte-equivalence + telemetry SST`

#### 2.3.6 风险

- 风险:`AnswerBlock` 已有字段众多(`ClaimUses / EdgeAnchors / FacetIDs / SurfaceRole`),抽 normalizer 时漏字段。
- 缓解:测试 `TestNormalizeEmitAnswerBlock_AllFieldsPropagate` 用 reflection 走遍 `types.AnswerBlock` 所有 exported 字段,确保 normalizer 都覆盖。

---

## 3. G3 — Diagram typed `relation_kind` 优先

### 3.1 现状对照

| 文件 | 行 | 角色 |
| --- | --- | --- |
| `internal/types/diagram_relation.go` | 50-50 | `SectionDiagramEdgeLabelVocabulary` SST const |
| `internal/types/diagram_relation.go` | 52-93 | `DiagramRelationKind` enum(Call/Guard/Import/Precedence/Contain/Observe/Unknown) |
| `internal/types/diagram_relation.go` | 130-144 | `ClaimFormForRelation` 映射 |
| `internal/types/diagram_relation.go` | 162-200 | `diagramRelationKeywords` 闭集字典(优先级:guard > precedence > import > observe > contain > call) |
| `internal/types/diagram_relation.go` | 212-224 | `InferRelationFromLabel` 闭集 substring match |
| `internal/types/answer_document_v2.go` | 142-159 | `DiagramEdgeAnchor{FromNode, ToNode, ClaimForm}` — **无 `RelationKind` 字段** |
| `internal/orchestrator/contract_check_block.go` | 281-353 | `validateDiagramRelationLegality` — Layer 2 完全依赖 `InferRelationFromLabel(e.label)` |

**确认**:
- ✅ Layer 1(endpoint grounding)是 typed
- ❌ Layer 2(relation legality)的 relation kind 完全由 label 字符串推导,`DiagramEdgeAnchor` 不带 `RelationKind`
- ❌ `EdgeRelations[].Kind` 已 typed,但 LLM 必须用系统认识的词汇标 label,relation kind 才能被识别

### 3.2 缺口诊断

> 当前 LLM 被迫:**写出系统词汇表里的标签 → 系统从标签推 relation_kind → match EdgeRelations**。
>
> 期望:**LLM 直接在 `edge_anchors[]` 里 emit typed `relation_kind` → label 只做可读性补充**。

### 3.3 详细设计

#### 3.3.1 `DiagramEdgeAnchor` 加字段

```go
// internal/types/answer_document_v2.go
type DiagramEdgeAnchor struct {
    FromNode     string              `json:"from_node"`
    ToNode       string              `json:"to_node"`
    RelationKind DiagramRelationKind `json:"relation_kind,omitempty"` // NEW
    ClaimForm    ClaimForm           `json:"claim_form,omitempty"`
}

// HasTypedRelation reports whether the anchor declares its
// relation_kind directly (i.e. the LLM emitted typed relation
// rather than relying on label inference).
func (e *DiagramEdgeAnchor) HasTypedRelation() bool {
    return e != nil && e.RelationKind.IsValid()
}
```

#### 3.3.2 Validator 改造 — typed-first

`validateDiagramRelationLegality`(`internal/orchestrator/contract_check_block.go:281`)从

```go
rel := types.InferRelationFromLabel(e.label)
```

改为分层 resolve:

```go
// Resolution priority:
//   1. If a block-level edge_anchors[] entry matches (from,to) by
//      case-folded equality AND its RelationKind is set, USE that.
//   2. Otherwise, fall back to InferRelationFromLabel(e.label).
//   3. Unknown remains "label-free edge" (no violation).
//
// Layer-2 violation production reads the resolved kind verbatim;
// label vocabulary becomes a CONSISTENCY check, not an authority.
rel := resolveEdgeRelation(doc, e)

// New consistency rule (SOFT, advisory):
// when typed RelationKind is set AND label resolves to a different
// non-Unknown kind, emit ViolDiagramEdgeLabelMismatch — invites the
// LLM to relabel for readability without breaking typed semantics.
```

`resolveEdgeRelation` 的 typed-first 查找用 `(lower(from), lower(to))` 索引,O(N) 一次构建。

#### 3.3.3 新 ViolKind: `ViolDiagramEdgeLabelMismatch`(SOFT-only)

```go
// internal/types/violation.go(若不存在则在 violation_kinds.go)
const ViolDiagramEdgeLabelMismatch ViolationKind = "diagram_edge_label_mismatch"
```

- 默认 SOFT(仅 advisory,LLM 看了可改,不强卡)
- 不进 STRICT promote 名单(R3 — label 推断本身就是 noisy 信号,不能升 hard gate)
- 进 `inferViolationLayer = "v2_oracle"`
- 进 `FallbackTargetForKind = FallbackFinalizerOnly`(label 改写在 finalize 内即可)

#### 3.3.4 Skill prompt 教学(R4 / R5 / SST 红线)

`internal/skill/diagram_edge_vocab.go` 当前是 vocabulary doc;新增 typed-first 教学块:

```
Two ways to declare an edge's relation:

A. PREFERRED — typed `relation_kind`. In your block-level
   `edge_anchors[]`, set `relation_kind` to one of: <SST list from
   AllDiagramRelationKinds()>. The label can then be free prose for
   readability.

B. LABEL-ONLY — when `relation_kind` is omitted, the system infers
   from the label using the keyword list under "<SectionDiagramEdgeLabelVocabulary>".
   This path is supported but constrains your wording.

When BOTH are present and they disagree, the typed `relation_kind`
wins; you'll see an advisory note inviting you to align the label.
```

> SST:列表用 `types.AllDiagramRelationKinds()` 渲染,**不**手抄。

#### 3.3.5 R2' 6 处同步映射

| 同步点 | 改动 |
| --- | --- |
| (1) struct | `DiagramEdgeAnchor.RelationKind` 新字段 + `HasTypedRelation()` |
| (2) tool schema | `emit_answer_document_v2.go` + `emit_answer_document_patch.go` 的 schema description **同时**加 `relation_kind` 字段说明,从 `types.AllDiagramRelationKinds()` 渲染 |
| (3) skill prompt | `diagram_edge_vocab.go` typed-first 教学(上文) |
| (4) retry hint | `RetryBlockSummary.EdgeAnchoredClaimUses` 计数补 typed-relation 子计数(可选) |
| (5) JSON decoder remap | `answerDocumentV2MisplacedHints` 加 `relation_kind` → "did you mean to put relation_kind on edge_anchors[i].relation_kind?"(防止 LLM 误放在 claim_use 里) |
| (6) cooccurrence rule | `repair_cooccurrence.go` C2 / C3 已映射 `ViolDiagramEdgeUnsupported`,新增 `ViolDiagramEdgeLabelMismatch` 不要进 cooccurrence(SOFT-only,无 derive) |

#### 3.3.6 验证 — 测试锁

| 测试 | 锁定不变量 |
| --- | --- |
| `TestDiagramEdge_TypedRelationOverridesLabel` | label="invokes"(infers Call)+ typed RelationKind=Guard → resolved=Guard,emit `ViolDiagramEdgeLabelMismatch`(SOFT) |
| `TestDiagramEdge_TypedRelationAloneSatisfiesContract` | label="" + typed RelationKind=Call satisfies EdgeRelations{Kind:Call,Min:1} |
| `TestDiagramEdge_LabelOnlyPathPreserved` | typed unset,label="invokes" → resolved=Call(向后兼容) |
| `TestDiagramEdge_NoLLMFacingJargon` | grep skill prompt + retry hint 无 "RelationKind" 类 Go 字面量(R4) |
| `TestDiagramRelationKeywords_SST` | skill prompt 渲染的列表与 `DiagramRelationKeywords()` 完全等价(SST) |

#### 3.3.7 实施 commit 序列(4 commits)

1. `types: DiagramEdgeAnchor.RelationKind + HasTypedRelation + 6 单测`
2. `validator: resolveEdgeRelation typed-first + ViolDiagramEdgeLabelMismatch SOFT oracle`
3. `tool/skill: relation_kind schema field + typed-first教学 + SST list (R4 audit)`
4. `tests: real-eval m1a × 1 验证 typed path 0 false positive`

#### 3.3.8 风险

- 风险:LLM 同时填 typed + label 但都错,系统选了 typed 错的导致 ClaimForm 不匹配 → 由 layer-2 已有 ClaimForm 检查兜底(无回归)。
- 风险:vocabulary 字典扩张诱惑 — **不接受**新增同义词(R6 / R3)。typed path 已经解开词汇绑定,字典不该再变大。

---

## 4. G4 — Helper / Evaluator dynamic prompt 审计扩展

### 4.1 现状对照

| 文件 | 行 / 范围 | 角色 |
| --- | --- | --- |
| `internal/orchestrator/llm_facing_jargon_audit_test.go` | (整文件) | static skill corpus + reviewer prompt 的 jargon 审计 |
| `internal/agent/answer_document_evaluator.go` | 480-510 | `renderAnswerDocStepBackbone` 含 "principal `ordered_list` block's `items[]`" 等内部话术 |
| `internal/agent/answer_document_evaluator.go` | 210, 472, 618 | "deterministic" / "step backbone" / "diagram spine" 类 prose |
| `internal/skill/defaults.go` | 多处 | "deterministic renderer" / "deterministic alignment"(可保留 — 用户描述是渲染行为,不是 stage codename) |

**确认**:
- ✅ static skill 已 audit
- ❌ helper layer 拼装的动态 prose(`renderAnswerDocStepBackbone` / `renderAnswerDocFacetCoverage` / 其他 `b.WriteString(...)` 串)未进 audit
- ⚠️ "principal `ordered_list` block's `items[]`" 这种 schema-shape 表述介于"必要 contract 名"与"实现层话术"之间;按 R5 应允许(模型必须知道字段名才能填),但应避免"upstream / pipeline / stage / phase"这类**位置/时间**话术

### 4.2 详细设计

#### 4.2.1 扩展 audit 测试

`internal/orchestrator/llm_facing_jargon_audit_test.go` 加入新测试 `TestEvaluatorDynamicProse_NoInternalJargon`:

```go
// 把 evaluator/helper 的所有 render* 函数走一遍代表性 ctx,
// 收集 LLM-facing 输出,逐个 grep InternalTermsBlocklist。
// 覆盖函数清单(SST):
//   - renderAnswerDocStepBackbone
//   - renderAnswerDocFacetCoverage
//   - renderAnswerDocEnumerationBoundary
//   - renderAnswerDocClaimVocabulary
//   - renderAnswerDocClaimFormCatalog
//   - renderAnswerDocDiagramSection
//   - renderAnswerDocCriticalEvidence
//   - 其他 8 个 BuildInitialInstruction 调用链上的 render*
```

#### 4.2.2 InternalTermsBlocklist 扩展(`internal/skill/glossary.go`)

新增 jargon 黑名单 entries:
- `"upstream deterministic pipeline"` 类(R4 — pipeline 是 stage codename)
- `"the analysis phase identified"`(可保留 — 用户合理需要知道哪一步给的;但需评估是否换成 "the prior investigation surfaced ..." 等中性表述)
- `"backbone"`(争议:技术术语 vs 实现话术 — 倾向保留,因为它描述的是"骨架"用户可见概念)

> **决策原则**:三类区分
> 1. **必删**:`pipeline / stage / phase / orchestrator / dispatch / TaskGraph / BusContext / FacetCoverageContract / extractor / explorer / finalizer`
> 2. **可保留**:用户可见的 contract 名(`emit_answer_document` / `summary` / `body` / `block` / `ordered_list` / `claim_uses`)
> 3. **个案审查**:`backbone / spine / canonical / deterministic`(在用户可见行为意义上保留;在系统流程意义上删)

#### 4.2.3 改写清单(预估 ~10 处)

按 G4 commit 阶段处理,逐处过 R4 checklist。**不**在本设计阶段定稿改写文本,留给实施。

#### 4.2.4 实施 commit 序列(2 commits)

1. `audit: TestEvaluatorDynamicProse_NoInternalJargon coverage extension`
2. `prompt: rewrite ~10 helper-render sites flagged by audit + InternalTermsBlocklist 扩条`

### 4.3 风险

- 风险:为求"无 jargon"过度抽象,LLM 失去 contract 名指引(违 R5 — 但反向)。
- 缓解:audit 只 block 实现层 jargon,不 block contract 名;每条 block 必须有 R6 review("这个词如果换成中性表述,LLM 还能正确填字段吗?")。

---

## 5. G5 — Semantic Quality Reviewer

### 5.1 现状对照

| 文件 | 角色 |
| --- | --- |
| `internal/orchestrator/self_consistency_reviewer.go` | 现行 reviewer,只看 SUMMARY + BODY + EvidenceAnchorSet |
| `internal/orchestrator/contract_check.go` | 187 行调用 reviewer,产 ViolSelfContradiction |

**确认**:
- ✅ 已能抓"自相矛盾 / 伪造标识符"
- ❌ 看不到 facet 完整性 / diagram relation 充分性 / richness 是否被压薄

### 5.2 详细设计

#### 5.2.1 新建 `internal/orchestrator/semantic_quality_reviewer.go`(NEW)

**关键决策**:**新增第二层 reviewer**,不扩 self_consistency。理由:
- 自一致性 reviewer 现有 prompt 已经精心调优(防 cried-wolf),输入扩展会冲淡焦点
- 新 reviewer 的输入维度完全不同(facets / diagram contract / richness candidates)
- 不同 confidence floor / 不同触发条件

#### 5.2.2 输入类型

```go
type SemanticQualityInput struct {
    OriginalRequest string

    AnswerSummary string
    AnswerBody    string

    // RequiredFacets 是 view.FacetCoverage.Required 的 typed 摘要:
    // 每条含 Kind + Tier + 是否在 doc.Blocks 中找到 facet_id 覆盖
    // (typed 计算,不让 LLM 重做)
    RequiredFacets []SemanticFacetSummary

    // DiagramContract 是 view.DiagramPlan.EdgeRelations 的 typed 摘要:
    // 每条 (RelationKind, Min, Got) — Got 是 doc 实际命中的 typed
    // 关系数 (含 typed RelationKind 与 InferRelationFromLabel 解析)
    DiagramContract *SemanticDiagramSummary

    // RichnessCandidates 是 Optional facets 中 SourceCandidate 非空
    // 但 doc 未声明的列表 — 这是"答案可以更厚但被压薄"的 candidate
    RichnessCandidates []SemanticRichnessSummary

    // EvidenceAnchorSet 复用 self-consistency 的同一函数(SST)
    EvidenceAnchorSet []string
}

type SemanticFacetSummary struct {
    Kind    string  // facet kind 字符串 (e.g. "principal_mechanism")
    Tier    string  // "essential" / "expected" — Enrichment 不进这个 reviewer
    Covered bool    // typed: 是否在任一 block.facet_ids[] 内
}
```

#### 5.2.3 verdict 类型

```go
type SemanticQualityResult struct {
    // Sufficient 报告答案是否在 facet/diagram/richness 三轴都达标。
    // true = 充足;false = 至少一条 Concerns 列出。
    Sufficient bool

    // Concerns 列出 reviewer 认定的"过薄/弱化/缺失"。每条含 Topic
    // (facet/diagram_spine/richness) + 缺失说明 + reviewer 建议补什么。
    // 上限 5 条。
    Concerns []SemanticQualityConcern

    Confidence float64
    Reasoning  string
}

type SemanticQualityConcern struct {
    Topic       string // ≤ 60 chars: "facet:principal_mechanism" 类
    Observation string // ≤ 200 chars: BODY 的具体表现
    Suggestion  string // ≤ 200 chars: reviewer 给 finalizer 的具体建议
}
```

#### 5.2.4 Reviewer prompt 关键设计点

- **R5 红线**:reviewer prompt **不**告诉 LLM"答案该是什么形状",只描述判断标准。
- **R6 红线**:示例用 ABSTRACT placeholder,不引真 case。
- **不抓自相矛盾**:reviewer 显式说"that is the consistency reviewer's job; you only look at completeness / sufficiency"。
- **confidence floor**:0.85(比 self-consistency 高,因为 false positive 直接强迫 finalizer 加内容,代价更大)。
- **decision discipline**:"if the answer ships a defensible mechanism explanation but at higher abstraction than ideal, that is NOT a concern — abstraction is editorial choice".

#### 5.2.5 触发与消费

- 时机:`runContractCheck` 内,在 `validateFacetCoverage` 之后,只在 facet hard violation 为空时跑(避免双 reviewer 噪音)。
- 产出:`ViolAnswerSemanticUnderfilled`(NEW SOFT-only ViolKind)。
- 默认 SOFT,operator 可促 STRICT(`pipeline_contract_strict_kinds`)。
- `FallbackTargetForKind = FallbackFinalizerOnly`(可在 finalize 内补)。
- `inferViolationLayer = "semantic_quality"`(新 layer 字符串,与 self_consistency 平行)。

#### 5.2.6 R2' 6 处同步映射

| 同步点 | 改动 |
| --- | --- |
| (1) struct | `SemanticQualityResult` / `Concern` / `Input` |
| (2) tool schema | `emit_semantic_quality_review` 新工具(reviewer 用,内部 dispatch) |
| (3) skill prompt | 不动 — finalizer 不直接看 reviewer prompt |
| (4) retry hint | `RetryState.SemanticConcerns []SemanticQualityConcern`(typed snapshot)+ render 段 "## Areas to expand"(R4 中性表述) |
| (5) JSON decoder | reviewer 工具的 strict-decode hint 表 |
| (6) cooccurrence | C8(NEW):`ViolAnswerSemanticUnderfilled` Primary,无 Derived(独立 root cause)|

#### 5.2.7 测试锁

| 测试 | 锁定不变量 |
| --- | --- |
| `TestSemanticQualityReviewer_FacetGapTriggersConcern` | required facet 标 covered=false → reviewer 输出 Concerns |
| `TestSemanticQualityReviewer_DefensibleAbstractionPasses` | 抽象但充分的答案 → Sufficient=true |
| `TestSemanticQualityReviewer_DoesNotDuplicateConsistency` | 含 SUMMARY/BODY 矛盾的输入 → Sufficient=true(不越权,矛盾归 self-consistency) |
| `TestSemanticQualityReviewer_NoLLMFacingJargon` | reviewer system prompt + retry hint 无 stage codename |
| `TestSemanticQualityReviewer_ConfidenceFloor` | confidence < 0.85 → 静默 drop |

#### 5.2.8 实施 commit 序列(3 commits)

1. `reviewer: SemanticQualityReviewer typed input/output + system prompt + 8 单测`
2. `orchestrator: wire after facet check + ViolAnswerSemanticUnderfilled SOFT + retry hint`
3. `tests: real-eval m1a / s1a × 2 验证 → 0 false positive on rich answers + non-zero hit on thin ones`

### 5.3 风险

- 风险:reviewer 对所有"抽象"答案都喷 Concerns,变成噪音生成器。
- 缓解:confidence floor 0.85 + decision discipline 显式禁掉"abstraction is concern" + real-eval 上线 gate(任何 false positive > 10% 直接关闭 reviewer 升级)。

---

## 6. G6 — Richness Tier 三档化 + 全消费打通

### 6.1 现状对照

| 文件 | 行 | 角色 |
| --- | --- | --- |
| `internal/types/facet_plan.go` | 192-202 | `FacetRequiredness` enum: Hard / Soft / Optional |
| `internal/types/facet_plan.go` | 218-240 | `RichnessTier` enum: Essential / Expected / Enrichment + `TierFromRequiredness` |
| `internal/orchestrator/contract_check_block.go` | 879-939 | `validateFacetCoverage`:Essential 总硬,Expected 看 SourceCandidate,Enrichment skip |
| `internal/orchestrator/contract_check_block.go` | 954-1015 | `validateRichnessRegression`:Optional facets SourceCandidate 非空 → SOFT telemetry |

**确认**:
- ✅ 三档已存在(Essential / Expected / Enrichment)
- ✅ Essential 已是 hard,Expected 已是 evidence-sufficient gate(Phase 5-E1)
- ❌ "ExpectedWhenEvidenceSufficient" 实际等同当前 TierExpected;但需求强调的是**升级语义**:当 evidence 足够时,Expected 应该升级为 hard,而非保持 Soft
- ❌ Enrichment 仍纯 telemetry,reviewer/accept policy 不消费

### 6.2 缺口诊断

> 需求文档原文:"当 evidence 足够时,某些 rich facets 应该升级成强要求"。
>
> 现状:Expected facets 在 SourceCandidate 非空时**才 demand 覆盖**,但 demand 的 violation kind 是 SOFT(`ViolFacetUncovered` 默认 SOFT)。这里的"升级"语义是**默认 STRICT-when-evidence-sufficient**,而不是 SOFT-with-evidence-gate。

### 6.3 详细设计

#### 6.3.1 引入 `FacetRequirement.PromotionPolicy` 显式分层

```go
// internal/types/facet_plan.go
type FacetPromotionPolicy string

const (
    // PromotionAlwaysHard — Essential default.
    PromotionAlwaysHard FacetPromotionPolicy = "always_hard"

    // PromotionWhenEvidenceSufficient — Expected default.
    // 升级条件:len(SourceCandidate) >= MinEvidenceForPromotion (typed gate).
    // 升级后 ViolFacetUncovered 该 facet 实例进 STRICT 名单,
    // 不再受 pipeline_contract_strict_kinds 配置影响。
    PromotionWhenEvidenceSufficient FacetPromotionPolicy = "when_evidence_sufficient"

    // PromotionAdvisoryOnly — Optional default.
    // 永远 SOFT,只产 ViolRichnessRegression。
    PromotionAdvisoryOnly FacetPromotionPolicy = "advisory_only"
)

type FacetRequirement struct {
    // ... existing fields ...
    PromotionPolicy        FacetPromotionPolicy
    MinEvidenceForPromotion int  // default 1 for Expected, ∞ for Optional
}
```

`CompileFacetCoverage` 自动从 `Requiredness → PromotionPolicy` 映射(SST 1:1)。

#### 6.3.2 Validator 升级

`validateFacetCoverage`:

```go
for _, req := range view.FacetCoverage.Required {
    promoted := isPromoted(req)  // typed: 按 PromotionPolicy + len(SourceCandidate)
    switch {
    case req.Tier == TierEnrichment:
        continue  // 走 validateRichnessRegression
    case req.Tier == TierExpected && !promoted:
        continue  // 维持 Phase 5-E1 evidence-sufficient skip
    }
    // demand 该 facet。violation severity 由 promoted 标志决定:
    // promoted=true → STRICT(直接 hard fail);
    // promoted=false → SOFT(走配置);
    // 通过 violation.SeverityHint 字段传递,而非全局 strict-kinds map。
}
```

> R3 检查:`isPromoted` 仅读 typed 字段(`PromotionPolicy` enum + `len(SourceCandidate)` integer + `MinEvidenceForPromotion` integer),无 heuristic。

#### 6.3.3 Reviewer/AcceptPolicy 消费

- G5 SemanticQualityReviewer 在 `RequiredFacets` 摘要里加 `Promoted bool` — 让 reviewer 关注"已升级却 covered=false"的高优先 concern。
- `internal/agent/accept_policy.go`(若存在 acceptance gate)在写模式 verify 阶段消费同一字段。读模式无 accept policy,跳过。

#### 6.3.4 R2' 6 处同步

| 同步点 | 改动 |
| --- | --- |
| (1) struct | `FacetPromotionPolicy` enum + `FacetRequirement.PromotionPolicy/MinEvidenceForPromotion` |
| (2) tool schema | facet_id 字段 description 加 "the system tracks each facet at one of three depth levels..."(SST 渲染) |
| (3) skill prompt | facet coverage 段加 "覆盖 essential 的 facet 是 hard;expected 的 facet 在证据足够时会升级为 hard,你应该总是覆盖它们;enrichment 是建议性的"(R4 — 不暴露 PromotionPolicy 字面量,只描述行为) |
| (4) retry hint | violation render 在 promoted facet 上加 "(elevated by available evidence)" 中性短句 |
| (5) JSON decoder | 不涉及(LLM 不直接 emit Tier) |
| (6) cooccurrence | C3 已映射 `ViolFacetUncovered`,新分级不改 cooccurrence 规则;但 RepairLocus 优先级提升:promoted=true 的 ViolFacetUncovered 在 deepest 计算时按 LocusExplore 算(已是) |

#### 6.3.5 测试锁

| 测试 | 锁定不变量 |
| --- | --- |
| `TestFacetPromotion_EssentialAlwaysHard` | TierEssential + len=0 → demand,SeverityHint=Strict |
| `TestFacetPromotion_ExpectedWithEvidencePromotesToHard` | TierExpected + len=2 ≥ Min → demand,SeverityHint=Strict |
| `TestFacetPromotion_ExpectedWithoutEvidenceSkips` | TierExpected + len=0 → skip(不 demand) |
| `TestFacetPromotion_EnrichmentNeverPromotes` | TierEnrichment + len=10 → 走 validateRichnessRegression,绝不 hard |
| `TestFacetPromotion_TypedSignalOnly` | 反射检查 `isPromoted` 路径不调任何 frequency / similarity 函数(R3) |
| `TestFacetPromotion_NoLLMFacingJargon` | grep prompt + retry hint 无 "PromotionPolicy" 字面量 |

#### 6.3.6 实施 commit 序列(3 commits)

1. `types: FacetPromotionPolicy + MinEvidenceForPromotion + CompileFacetCoverage 1:1 映射 + 6 单测`
2. `validator: validateFacetCoverage promotion 升级路径 + violation.SeverityHint`
3. `reviewer/prompt: SemanticQualityReviewer 看 Promoted + skill prompt 行为描述(R4)`

### 6.4 风险

- 风险:promotion 把过去 SOFT 的 facet 突然变 STRICT,旧答案 regress。
- 缓解:阶段 1 先在 telemetry 阶段渲染"would have been promoted"日志,看真实 eval 命中率,再切硬卡;`pipeline_facet_promotion_enabled` yaml flag 默认 true,可一键回滚。

---

## 7. G4 实际改写清单 vs G5 vs G6 顺序补注

> 在执行序列里 G4 排在 G5 后,理由:G5 的 reviewer prompt 一上线就要过 audit;先把 audit 框架扩了再上 G5,可以一次性把 G5 prompt + helper prose 全过一遍(避免做两次)。
> **修订:G4 先做 audit 扩展(只加测试,不改写),G5 上线时立即过新 audit,然后 G4 第二步集中改写。**

---

## 8. G7 — Strict decode path-sensitive 预扫描(长期悬挂)

### 8.1 现状

| 文件 | 角色 |
| --- | --- |
| `internal/tool/strict_decode_remap.go` | field-name-only remap,docstring 已坦率说明限制 |

### 8.2 决策

**短期 NOT NOW**。理由:
- 当前 remap 已抓住 ~90% 的 misplacement(`claim_use` / `citation_ref` 类)
- path-sensitive 预扫描需要 lightweight JSON traversal,本质是写半个 strict decoder
- 实施代价 vs 边际收益不划算(F4 retry-loop 已被 14d9b6e 压到 ~4 calls)

### 8.3 触发条件(将来)

当真 eval 出现 ≥3 个新 case `failed because misplaced field with same name in two containers`,且每次 retry > 6 calls,才启动 G7。在那之前,纯防御性悬挂。

### 8.4 实施草图(留底)

```
PreScanFieldPath(raw json.RawMessage, targetField string) ([]string, error):
    1. lightweight tokenizer (避免完整 decode)
    2. 记录每个 field 的 ancestor chain
    3. return 出现 targetField 的所有 path
```

remap 用 path 给更精确建议:`"unknown field 'claim_use' at path 'blocks[0].items[2]' — did you mean 'blocks[0].items[2].claim_use' (item-level)?"`

---

## 9. 跨任务红线复核 checklist

每个 G 项 commit 前 BLOCKING 必走:

- [ ] R2' 6 处同步映射表已填全
- [ ] R3 hard gate 信号 typed,grep 无 `Score` / `similarity` / `freq` 进 hard 路径
- [ ] R4 LLM-facing 串过 `TestNoInternalTermsInPrompts` + `TestEvaluatorDynamicProse_NoInternalJargon`(G4 上线后)
- [ ] R5 reviewer/skill prompt 无"答案该长这样"的 verbatim 模板
- [ ] R6 测试无 verbatim 真 case method/path,例子全 ABSTRACT
- [ ] R7 grep 旧概念在 LLM-facing 处全删,无 stale "see X above" 但 X 不存在
- [ ] SST:任何 ≥2 处使用的 string/list 抽常量 / 函数渲染
- [ ] L1:`grep -n runReadSchedulerLoop internal/orchestrator/orchestrator.go` 函数 body 字面文本与上一 commit byte-equal(`git diff -G '^func runReadSchedulerLoop' HEAD~`)

---

## 10. 真 eval 验证矩阵

每个 G 项落地 commit ≥1 后,跑 real-eval × 2 case 验证:

| G | s1a×2 | m1a×2 | u3a×2 | comparison-family×1 | 关注 metric |
| --- | --- | --- | --- | --- | --- |
| G1 | ✅ | ✅ | ✅ | — | OwnerStableAttempts ≤ 旧版 0.7×;repair_exec_plan_remaining 真用上 |
| G2 | ✅ | ✅ | — | — | full / patch byte-equiv;全 emit 路径过同一 chokepoint(已是,确认) |
| G3 | ✅ | ✅ | — | — | typed RelationKind 命中率 ≥ 30%;label-mismatch SOFT 0 false positive |
| G5 | ✅ | ✅ | ✅ | — | reviewer Concerns 在 thin 答案 hit 1+;rich 答案 0 |
| G6 | ✅ | ✅ | — | ✅ | promoted facet 在 evidence 足时真硬卡 |

---

## 11. 文档维护

- 本文随 commit 推进,每完成一个 G 在对应章节末尾加 `### Status: SHIPPED @ <commit>` 行。
- 全部 G(除 G7)SHIPPED 后,本文转入 `docs/migration/`,作为后续审计起点。
- 若 G7 永久悬挂,在 §8 末加 `Status: PERMANENTLY DEFERRED — see <reason>`。

---

## 12. 责任边界(再次锁定)

> 与需求文档 §5 一致,仅作 SST 复述:

| 模块 | 负责 | 不负责 |
| --- | --- | --- |
| Analyzer / Compiler | family / facet plan / semantic view / diagram plan / richness candidate / promotion policy | retry owner / final prose / mutation merge |
| Finalizer | 在 semantic view 允许的 surface 内产 V2 blocks + typed annotations(包括新 `relation_kind`) | 发明 relation / 猜 claim_form / 猜 facet 充分性 |
| Validator | block correctness / claim legality / facet coverage / diagram legality / absence scope / **典型升级判定 (G6)** | 选 repair owner / 改写答案 |
| Retry Router(G1) | 按 typed RepairExecutionPlan 顺序选当前 owner | 仅凭 violation depth/count 路由 |
| Reviewer(self-consistency + G5 semantic-quality) | 各自轴的判断 | 替 finalizer 写答案 / 跨轴越权 |

---

## 附录 A — 现有代码字段索引(实施时锚定)

```
internal/types/facet_plan.go
  FacetRequiredness        line 196
  RichnessTier             line 218
  FacetRequirement         line 242  ← G6 加 PromotionPolicy / MinEvidenceForPromotion

internal/types/answer_document_v2.go
  AnswerBlock              line ~80
  AnswerBlock.EdgeAnchors  line 139
  DiagramEdgeAnchor        line 155  ← G3 加 RelationKind

internal/types/diagram_relation.go
  DiagramRelationKind      line 52
  ClaimFormForRelation     line 130
  diagramRelationKeywords  line 162  ← G3 SST 不变

internal/orchestrator/repair_plan.go
  RepairCluster            line 44
  RepairPlan               line 68   ← G1 新建 RepairExecutionPlan 平行

internal/orchestrator/repair_cooccurrence.go
  defaultCooccurrenceRules line 83   ← G5 加 C8 ViolAnswerSemanticUnderfilled(无 derived)

internal/orchestrator/fallback_policy.go
  FallbackTargetForViolationsWithBudget line 546  ← G1 改造

internal/orchestrator/retry_state.go
  populateRetryState       line 42   ← G1 加 ExecutionPlan + G5 加 SemanticConcerns

internal/orchestrator/contract_check_block.go
  validateDiagramRelationLegality line 281  ← G3 改 typed-first
  validateFacetCoverage           line 879  ← G6 加 promotion
  validateRichnessRegression      line 954  ← G6 检查 reviewer 消费

internal/orchestrator/self_consistency_reviewer.go
  SelfConsistencyInput     line 60   ← G5 旁立一个 SemanticQualityInput

internal/agent/answer_document_evaluator.go
  renderAnswerDocStepBackbone line ~485  ← G4 audit
  renderAnswerDocFacetCoverage line ?    ← G4 audit + G6 渲染 promoted

internal/tool/emit_answer_document_v2.go
  emitAnswerDocumentV2Params line 34   ← G3 schema desc + G2 抽 normalizer
internal/tool/emit_answer_document_patch.go
  EmitAnswerDocumentPatch   line 41    ← G3 schema desc + G2 抽 normalizer

internal/tool/strict_decode_remap.go    ← G7 长期挂起
```

## 附录 B — 关键不变量摘要(commit 时 grep 校验)

```
✅  runReadSchedulerLoop body bytes preserved
✅  全部 emit 路径(full + patch + 未来 mutation)走 mut.AnswerDocumentV2() chokepoint 的 contract.Check
✅  RepairCluster.Owner 从 typed FallbackTargetForViolation 推,无 heuristic
✅  DiagramEdgeAnchor.RelationKind 是 typed enum,InferRelationFromLabel 退为 fallback
✅  FacetRequirement.PromotionPolicy 1:1 from Requiredness;promotion gate 仅读 typed integer
✅  Reviewer prompts 通过 TestNoInternalTermsInPrompts + TestEvaluatorDynamicProse_NoInternalJargon
```

— END —
