# Finalizer Prompt Phase C — SHAPE_CONTRACT (typed Enumeration sub-family)

**Status**: Design (not yet implemented).
**Target session**: single-session ship, 6-9 commits.
**Predecessor**: `docs/design/finalizer_phase_a_rule_bisection.md` (Phase A — 删 machine-checkable 规则,**必须先 ship 并经真 eval 验证不回归**;Phase A 残留若 qf_arch 仍非 4/4,本 Phase 才有动机).
**Successor**: 暂无(Phase B 增强 cooccurrence rule 是平行 track,可独立)。
**Owner**: 任意接手者(本文档目标:不熟悉本仓库的开发者读完一遍即可开始改).
**Eval bar**: qf_arch x4 = 4/4(若 Phase A 已达成此 bar,本 Phase bar 上调到 qf_arch x8 全 PASS,且新加的 mechanism-enumeration case x4 也全 PASS)。

---

## 0. TL;DR — 一句话设计

Phase A 通过删冗规则把 A''(role-enumeration abstraction-level matching)的注意力份额从 1/24 提到 ~1/10。Phase C 进一步从**架构层**让 finalizer prompt 在每次 dispatch 只看到与本 question shape 相关的判断规则:

1. 在 `internal/types/answer_semantic_view.go::AnswerSemanticView` 加 typed 字段 `EnumerationSubType`(枚举 `EnumNone / EnumRole / EnumMechanism / EnumEntity`)
2. `internal/types/facet_plan.go::ResolveQuestionFamily` 同位置增加 `ResolveEnumerationSubType`(deterministic post-process,读 RequestModel.Intent / Predicates / AnswerSubject / AnalyzerHints)
3. `internal/types/answer_semantic_view_compile_enumeration.go::compileEnumeration` 按 sub-type 分支:RoleEnum 加 abstraction-level UncertaintyRule + AcceptableClaimForms 收紧;MechanismEnum 解锁 `ClaimCallEdge` / `ClaimGuardCondition`;EntityEnum 维持当前
4. `internal/agent/answer_document_evaluator.go::BuildInitialInstruction` 加 `renderAnswerDocSubFamilyRules`,**只在 view.EnumerationSubType=EnumRole 时**渲染 A'' 那条 prompt 规则到 user section
5. 加新 V2 validator `validateEnumerationAbstractionLevel`(SOFT 类),用 typed evidence 比对 LLM item.text 的 verb shape(call_edge vs role)— 这是 PRECISE 信号(claim_form 已 typed),作为 hard gate 安全
6. Hint composer 加 `ViolEnumerationAbstractionLevelMismatch` case + cooccurrence rule

**为什么这能彻底治 A'' jitter**:
- A'' 不再放在 24 条 Workflow 中**碰运气被注意到**;它作为 **per-dispatch typed user-section 段** 直接 render 到 LLM 视野里
- 同时有 **typed validator 作硬兜底**,不靠 LLM 自觉

**为什么不在 Phase A 直接做**:Phase C 涉及类型系统改动 + analyzer/extractor/finalizer 三处协调,改动面广;Phase A 改动局部、风险可控,先做 Phase A 验证"删冗规则"路径是否单独足够。如 Phase A 已让 qf_arch x4 = 4/4 且 cross-case 不回归,Phase C 可以**降优先级**(留作未来面对新 question shape 时的扩展模板)。

---

## 1. 背景:为什么这个 Phase 存在

### 1.1 当前 Enumeration family 内部缺 sub-family 区分

`internal/types/facet_plan.go::ResolveQuestionFamily` (line 573) 把 8 个 family 之一定为 `QFEnumeration`。但 enumeration 实际有三种语义子类:

- **Role-enumeration**:"列出每个 X 的职责 / 干什么"。期望答案是 *conceptual responsibility*("X 负责 Y"),禁 implementation chain;commit `efa4ff3` 加的 A'' 规则就是教 LLM 这点
- **Mechanism-enumeration**:"列出每个 X 怎么干 / 调用了什么"。期望答案是 *implementation chain*("X 调用 Y, Y 调用 Z");与 Role-enum 完全相反
- **Entity-enumeration**:"列出所有 implementer / subclass / handler"。期望答案是 *symbol slate*(symbol name + file:line + 1 行 role description);与前两种又不同

当前 `internal/types/answer_semantic_view_compile_enumeration.go::compileEnumeration` (line 26) **没有**任何 sub-type 区分:

```go
// answer_semantic_view_compile_enumeration.go:37-55
view.RequiredBlocks = []BlockRequirement{
    requireSummaryBlock(...),
    {
        Kind:     BlockOrderedList,
        AcceptableClaimForms: []ClaimForm{
            ClaimDefinitionFact,  // 只有这一个!
        },
        // ... no abstraction-level constraint
    },
}
```

`AcceptableClaimForms` 只列 `ClaimDefinitionFact`,意味着 mechanism-enum 想用 `ClaimCallEdge` 反而被 validator 拒。这是设计缺口 — 当前能跑 PASS 是因为 validator 对 enumeration 的 claim_form check 较宽松,但 prompt 教的"用 ClaimDefinitionFact"对 mechanism 题型反而不对。

### 1.2 当前 A'' 的 prompt 教法为何在 retry 中失效

A'' 在 `defaults.go:126`(规则 #8)以 condition-trigger 形式存在:

```text
"Abstraction-level matching (REQUIRED whenever an enumeration's items must
describe a role / responsibility / purpose — the question form 'what does
each X do' / 'what is each X for' / '每个 X 负责什么' ...)
```

这是 **LLM 自然语言判断 trigger**:LLM 必须自己读问题文本判断"这是不是 role-enumeration"。在 retry 路径上,RetryHint 占据 user section 第一段,LLM 注意力被 retry 内容吸引,Workflow 中段的条件 trigger 容易被忽略 — 这就是 qf_arch run-2 的实际失败模式。

**结构性修复**:把"是否 role-enumeration"做成 **typed signal**(EnumerationSubType=EnumRole),系统 deterministic 判定,然后:
- finalizer **只在 EnumRole 时**渲染 A'' 规则到 user section(不依赖 LLM 自己判断 trigger)
- validator 加一条 `validateEnumerationAbstractionLevel`,直接 check item.text 的 verb shape — 不靠 LLM 自觉

这是 `feedback_precise_signals_for_hard_gates.md` 红线的标准应用:把 "判断是否 role-enumeration" 这个上游决策从 LLM 自然语言判断升级到 typed 信号,再驱动下游 hard gate。

### 1.3 与 Phase A 的关系

| Phase | 改动层 | 风险 | 修 A'' jitter 的力度 |
|---|---|---|---|
| Phase A | prompt 内容(删冗规则) | 低 | 中(注意力份额 1/24 → 1/10,但仍依赖 LLM 自觉判断 trigger) |
| Phase C | 类型系统 + analyzer 后处理 + finalizer dynamic render + validator | 中-高(改 5+ 文件,涉及跨 stage 数据流) | 高(typed signal + hard gate,不再依赖 LLM 自觉) |

**实施顺序硬性**:Phase A 必须先 ship。原因:
- 如果 Phase A 已让 qf_arch x4 = 4/4,Phase C 可降优先级 — 大改动需要明确 ROI 才值得
- Phase A 的"删冗规则"为 Phase C 释放 prompt 注意力预算 — Phase C 加新 sub-family 规则到 user section 时,如果 Workflow 还是 24 条,新规则会被同样稀释
- Phase A 验证 hint composer 增强能 cover 删除规则 → Phase C 加新 ViolKind 时复用同一套 hint 模式

---

## 2. 代码定位指南(让不熟代码的开发者立即上手)

下面所有文件路径都是相对 `/home/chatpp/codrax`。

### 2.1 类型系统(SHAPE_CONTRACT 定义所在地)

| 文件 | 关键 line | 内容 |
|---|---|---|
| `internal/types/facet_plan.go:48-101` | enum + alllist | `QuestionFamily` 8 个值 + `AllQuestionFamilies()` |
| `internal/types/facet_plan.go:573-633` | `ResolveQuestionFamily()` | 7 优先级规则 deterministic dispatch(本 Phase 加 sibling 函数 `ResolveEnumerationSubType`) |
| `internal/types/answer_semantic_view.go:22-89` | struct | `AnswerSemanticView`(本 Phase 加 `EnumerationSubType` 字段) |
| `internal/types/answer_semantic_view.go:108-130` | struct | `BlockRequirement` (含 `AcceptableClaimForms []ClaimForm`) |
| `internal/types/analysis_ir.go:835-843` | struct | `AnswerContract` (Phase C 不动这个,SHAPE_CONTRACT 在 view 层而非 contract 层) |
| `internal/types/answer_semantic_view_compile_enumeration.go:26-94` | `compileEnumeration()` | 编译 QFEnumeration 的 view(本 Phase 改这里加 sub-type 分支) |
| `internal/types/claim_form.go` (grep `ClaimForm`) | enum | 9 个 ClaimForm 值(definition_fact / call_edge / guard_condition / ...) |

### 2.2 Analyzer 后处理(EnumerationSubType 的 producer)

| 文件 | 关键 line | 内容 |
|---|---|---|
| `internal/analysis/amplifier/amplifier.go:75-130` | `Amplify()` + `AmplifyPostCompile()` | Phase 1.1 框架,可注册 pre/post compile rules |
| `internal/analysis/amplifier/rule_r1_multi_subject.go` | 整文件 | 现有 R1 — multi-subject 检测(本 Phase 的 R4 sub-type 推断会复用同样模式) |
| `internal/analysis/amplifier/rule_r2_typed_name_parity.go` | 整文件 | 现有 R2 |
| `internal/analysis/amplifier/rule_r3_must_include_pinning.go` | 整文件 | 现有 R3 |
| `internal/analysis/amplifier/trap_fixture_test.go` | 整文件 | TermGraph trap 防护(本 Phase 新规则也必须过此 fixture) |
| `internal/analysis/amplifier/axis_collapse_fixture_test.go` | 整文件 | axis_collapse trap 防护(本 Phase 新规则**必过**这个) |

### 2.3 Finalizer dynamic user-section 渲染(SHAPE_CONTRACT 的 consumer)

| 文件 | 关键 line | 作用 |
|---|---|---|
| `internal/agent/answer_document_evaluator.go:127-330` | `BuildInitialInstruction()` | 顶层组装 user section(本 Phase 加新 render 函数调用) |
| `internal/agent/answer_document_evaluator.go:894-933` | `renderAnswerDocBlockContract()` | 现有渲染示范(本 Phase 加 `renderAnswerDocSubFamilyRules` 仿此模式) |
| `internal/agent/answer_document_evaluator.go:529-610` | `renderAnswerDocEnumerationBoundary()` | 现有 enumeration-related 渲染示范 |

### 2.4 Validator(EnumerationSubType 驱动的新 hard gate)

| 文件 | 关键 line | 作用 |
|---|---|---|
| `internal/orchestrator/contract_check_block.go:1681-1706` | `runV2BlockOraclesWithMut` | 13 个 V2 validator dispatch(本 Phase 加第 14 个) |
| `internal/orchestrator/contract_check_block.go:134-200` | `validatePrincipalClaimUse` 示范 | 仿此模式写新 `validateEnumerationAbstractionLevel` |
| `internal/orchestrator/contract_check_block.go:1432-1517` | `validateClaimFormSupport` 示范 | typed-evidence-vs-LLM-claim 比对模式 |

### 2.5 Hint composer + cooccurrence

| 文件 | 关键 line | 作用 |
|---|---|---|
| `internal/analysis/hint/composer.go:374-440` | `summariseExactFix` | 25 case 硬编码 ExactFix 文本(本 Phase 加第 26 case) |
| `internal/orchestrator/repair_cooccurrence.go:83-337` | `defaultCooccurrenceRules` | 9 条 Primary→Derived(本 Phase 加第 10 条若需要) |

### 2.6 Violation kind registry(新 ViolKind 必须注册)

| 文件 | 关键 line | 作用 |
|---|---|---|
| `internal/types/violation_registry.go:420-770` | profile entries | 每个 ViolKind 必须在此注册 `Profile{Kind, DefaultSeverity, Owner}` |
| `internal/types/cgec_completeness_test.go:233-280` | 完备性测试 | 新 ViolKind 必须加进此 map(否则测试 fail) |

### 2.7 测试影响面

| 文件 | 关键测试 | 影响 |
|---|---|---|
| `internal/types/facet_plan_test.go` | `TestResolveQuestionFamily*` | 加 `TestResolveEnumerationSubType*` 4 case |
| `internal/types/answer_semantic_view_compile_enumeration_test.go` | 编译路径 | 加 sub-type 分支 case |
| `internal/analysis/amplifier/rule_r4_*_test.go` | 新建 | 新 R4 rule 的 unit test + axis_collapse trap fixture cross-check |
| `internal/orchestrator/contract_check_block_test.go` | 加 `TestValidateEnumerationAbstractionLevel` | 测新 validator |
| `internal/agent/answer_document_evaluator_test.go` | 加 sub-family render case | 测新 user-section 段 |
| `internal/analysis/hint/composer_test.go` | 加 `ViolEnumerationAbstractionLevelMismatch` case | hint 文本测试 |
| `internal/types/violation_registry_legacy_test.go` | 新增 | profile 注册测试 |
| `evals/qf_arch/...` + 新加的 `evals/qf_mechanism/...` | x4 各 4 次 | 真 eval(留 commit 9) |

---

## 3. SHAPE_CONTRACT 数据流总览

```
┌──────────────────────────────────────────────────────────────────┐
│  ANALYZER LLM emit_analysis → RequestModel{                       │
│    Intent: <enum>, Scenario: <enum>, AnalyzerHints,              │
│    Predicates{ IsCategoryEnumeration, IsCountQuestion, ... },    │
│    AnswerSubject{Kind}, QuestionStructure{Buckets, ...},         │
│  }                                                                │
└─────────────┬────────────────────────────────────────────────────┘
              │
              ▼
┌──────────────────────────────────────────────────────────────────┐
│  amplifier.Amplify(rm) [pre-compile rules]                        │
│  Existing: R1 multi-subject, R2 name parity, R3 must-include      │
│  ★ NEW R4: ResolveEnumerationSubType(rm) → tag rm with hint       │
│    (or: skip and let post-amplify pure resolver run)              │
└─────────────┬────────────────────────────────────────────────────┘
              │
              ▼
┌──────────────────────────────────────────────────────────────────┐
│  compiler.Compile(rm) → AnswerContract                            │
└─────────────┬────────────────────────────────────────────────────┘
              │
              ▼
┌──────────────────────────────────────────────────────────────────┐
│  amplifier.AmplifyPostCompile(rm, contract) [post-compile rules]  │
└─────────────┬────────────────────────────────────────────────────┘
              │
              ▼
┌──────────────────────────────────────────────────────────────────┐
│  BuildAnswerSemanticViewForBusContext(busCtx) →                   │
│    family = ResolveQuestionFamily(rm)                             │
│    if family == QFEnumeration:                                    │
│      ★ subType = ResolveEnumerationSubType(rm)                    │
│      view = compileEnumeration(ir, plan, subType)  // ← 新参      │
│    else: view = compile<family>(ir, plan)                         │
│  Returns AnswerSemanticView{                                      │
│    Family, EnumerationSubType (★ new field),                     │
│    RequiredBlocks, OptionalBlocks, ...                            │
│  }                                                                │
└─────────────┬────────────────────────────────────────────────────┘
              │
              ├─→ FINALIZER: answer_document_evaluator.BuildInitialInstruction()
              │    user section additions:
              │    ┌──────────────────────────────────────────────┐
              │    │ ★ if view.EnumerationSubType == EnumRole:    │
              │    │     emit "## Enumeration sub-family: ROLE"  │
              │    │     emit A'' rule body (current rule #8)    │
              │    │   if EnumMechanism:                         │
              │    │     emit "## Enumeration sub-family: MECHANISM"│
              │    │     emit "items describe call/guard chain"  │
              │    │   if EnumEntity:                            │
              │    │     emit "## Enumeration sub-family: ENTITY"│
              │    │     emit "items name symbols + file:line"   │
              │    └──────────────────────────────────────────────┘
              │
              └─→ VALIDATOR: contract_check_block.runV2BlockOraclesWithMut()
                   ★ new validator: validateEnumerationAbstractionLevel(doc, view, mut)
                     - reads view.EnumerationSubType
                     - if EnumRole: scan ordered_list items for verb shape;
                       fire ViolEnumerationAbstractionLevelMismatch if "X calls Y"
                       pattern dominates without "X is responsible for Y"
                     - if EnumMechanism: opposite check
                     - if EnumEntity: skip (no abstraction-level constraint)
                     - if EnumNone (fallback): skip
```

---

## 4. SHAPE_CONTRACT 类型 + 分类规则(本 Phase 核心)

### 4.1 新 enum 定义

加到 `internal/types/facet_plan.go`(放在 QuestionFamily 定义之后):

```go
// EnumerationSubType refines QFEnumeration into three semantically
// distinct sub-families. Resolved deterministically by
// ResolveEnumerationSubType from typed RequestModel signals.
//
// Why a sub-type enum (not a new top-level QuestionFamily):
//   - All three sub-types share the same compileEnumeration scaffold
//     (BlockSummary + BlockOrderedList + optional Section/Caveat) —
//     they differ in AcceptableClaimForms and abstraction-level
//     constraints, not in block structure.
//   - Promoting them to top-level families would balloon
//     ResolveQuestionFamily's priority table from 7 → 9 rules with
//     overlapping conditions; sub-typing keeps the top-level dispatch
//     clean.
//
// Why typed (not a free-form string):
//   - Per the precise-signals-for-hard-gates red line, downstream
//     validators (validateEnumerationAbstractionLevel) read this as
//     a typed enum, not as a phrase parsed from prompts.
type EnumerationSubType string

const (
    // EnumNone — fallback when family != QFEnumeration OR none of the
    // sub-type rules match. Validators that key on sub-type MUST
    // skip when this is the value.
    EnumNone EnumerationSubType = ""

    // EnumRole — "what does each X do" / "every Y's responsibility" /
    // "每个 X 负责什么 / 干什么 / 的作用".
    // Items must answer at conceptual level (X is responsible for Y);
    // implementation chains are anchors, not the answer.
    EnumRole EnumerationSubType = "role"

    // EnumMechanism — "how does each X work" / "what does each X call" /
    // "每个 X 怎么做 / 调用了什么".
    // Items legitimately describe call chains and guards.
    EnumMechanism EnumerationSubType = "mechanism"

    // EnumEntity — "list all implementers / subclasses / handlers / Y's of X".
    // Items are symbol names + file:line + 1-line role; no abstraction-
    // level constraint applies.
    EnumEntity EnumerationSubType = "entity"
)

func AllEnumerationSubTypes() []EnumerationSubType {
    return []EnumerationSubType{EnumNone, EnumRole, EnumMechanism, EnumEntity}
}

func (s EnumerationSubType) IsValid() bool {
    for _, declared := range AllEnumerationSubTypes() {
        if s == declared {
            return true
        }
    }
    return false
}
```

### 4.2 分类函数(deterministic post-process,无 LLM call)

加到 `internal/types/facet_plan.go`,**紧邻 ResolveQuestionFamily**:

```go
// ResolveEnumerationSubType maps a RequestModel onto one of the
// three EnumerationSubType values when ResolveQuestionFamily has
// already returned QFEnumeration. Returns EnumNone for every other
// family — callers MUST gate the call by family == QFEnumeration.
//
// Decision sources (PRECISE typed signals only — no question-text
// regex matching, per §9.5 red line):
//
//   - rm.Intent: IntentEnumerate is the umbrella; sub-type
//     refinement reads other typed slots.
//   - rm.AnalyzerHints.Predicates.IsCategoryEnumeration: when true,
//     the question is asking for *types/categories of X*, which is
//     EnumEntity territory.
//   - rm.AnswerSubject.Kind: SubjectFunctionName / SubjectInterface /
//     SubjectStructField → entity-shaped subject → EnumEntity.
//   - rm.AnalyzerHints.Predicates.IsRelationalLookup +
//     rm.PredicateAxis: PredicateAxisCalls / PredicateAxisInvokes →
//     mechanism. PredicateAxisRole / PredicateAxisResponsibility →
//     role.
//   - rm.AnalyzerHints.Verbs (if exists, see §4.3): action verbs
//     "call" / "invoke" / "dispatch" → mechanism.
//
// Priority order (first match wins):
//
//  1. IsCategoryEnumeration=true → EnumEntity (categories ARE entities)
//  2. PredicateAxis ∈ mechanism axes → EnumMechanism
//  3. PredicateAxis ∈ role axes OR AnswerSubject.Kind=SubjectRoleNoun
//     → EnumRole
//  4. AnswerSubject.Kind ∈ entity-shaped → EnumEntity
//  5. fallthrough → EnumNone (treat as legacy enumeration; no
//     sub-family-specific rules apply)
//
// EnumNone is the safe default: when sub-type cannot be determined,
// the system falls back to current behavior (one AcceptableClaimForms
// list, no abstraction-level validator firing).
func ResolveEnumerationSubType(rm RequestModel) EnumerationSubType {
    if rm.AnalyzerHints.Predicates.IsCategoryEnumeration {
        return EnumEntity
    }
    switch rm.PredicateAxis {
    case PredicateAxisCalls, PredicateAxisInvokes, PredicateAxisDispatches:
        return EnumMechanism
    case PredicateAxisRole, PredicateAxisResponsibility:
        return EnumRole
    }
    switch rm.AnswerSubject.Kind {
    case SubjectFunctionName, SubjectInterface, SubjectStructField, SubjectHandlerRoute:
        return EnumEntity
    case SubjectRoleNoun:  // NB: may not exist yet — see §4.4
        return EnumRole
    }
    return EnumNone
}
```

### 4.3 PredicateAxis 现有值核对

**TODO 实施前 grep**: `grep "PredicateAxis[A-Z]" internal/types/*.go` 列出已有 PredicateAxis 值。如果 `PredicateAxisRole / PredicateAxisResponsibility / PredicateAxisDispatches` 不存在,本 Phase 第一个 commit 是**加这些值**(在 `internal/types/predicate_axis.go` 或类似文件)。

预期已存在的(基于 commit `efa4ff3` 上下文):`PredicateAxisCalls / PredicateAxisInvokes / PredicateAxisDispatches / PredicateAxisRole / PredicateAxisResponsibility`。如有缺失,加 + cgec_completeness_test.go 同步。

### 4.4 AnswerSubject.Kind 中 SubjectRoleNoun 是否已存在

**TODO 实施前 grep**: `grep "SubjectRoleNoun\|Subject[A-Z]" internal/types/*.go` 列出。

如果不存在,本 Phase 不强求加(`ResolveEnumerationSubType` 退到 PredicateAxis-only 路径)— 但 Phase C 的 commit 序列保留对它的引用,后续可补。

### 4.5 amplifier 集成方式(两个等价方案,选其一)

**方案 X(推荐)**:把 `ResolveEnumerationSubType` 做成纯函数 + 在 view 编译路径调用,**不**做成 amplifier rule:
- 优点:不动 amplifier 包(避免触动其 trap fixture / axis_collapse fixture);分类逻辑集中在 facet_plan.go 与 ResolveQuestionFamily 同位置
- 缺点:无 amplifier 命名痕迹(若未来要扩 sub-type 推断逻辑,得在 facet_plan.go 内部分支)

**方案 Y**:做成 amplifier R4 rule(`internal/analysis/amplifier/rule_r4_enumeration_subtype.go`),在 RequestModel 里加 `EnumerationSubTypeHint` 字段,view 编译时读:
- 优点:延续 amplifier 模式,扩展性好
- 缺点:必须过 `axis_collapse_fixture_test.go` + `trap_fixture_test.go`(amplifier 红线 `feedback_axis_collapse_alignment.md`),需要构造无误判 fixture
- 必须改 `RequestModel` struct 加新字段

**本设计推荐方案 X** — 改动面小,单 session 可控。如果后续 sub-type 推断逻辑变复杂(需要多信号融合),再升级到方案 Y。

---

## 5. AnswerSemanticView 字段加入

`internal/types/answer_semantic_view.go:22-89` 加新字段(放在 `Family` 字段紧邻):

```go
type AnswerSemanticView struct {
    Family QuestionFamily

    // EnumerationSubType refines Family=QFEnumeration into role /
    // mechanism / entity sub-shapes. EnumNone for every other family
    // OR when sub-type resolution did not find a typed signal.
    //
    // Driven by: ResolveEnumerationSubType (deterministic, no LLM).
    // Consumed by:
    //   - compileEnumeration (sets sub-type-specific
    //     AcceptableClaimForms and UncertaintyRules)
    //   - renderAnswerDocSubFamilyRules (only emits A'' rule when
    //     sub-type=EnumRole)
    //   - validateEnumerationAbstractionLevel (only fires when
    //     sub-type ∈ {EnumRole, EnumMechanism})
    EnumerationSubType EnumerationSubType

    FacetCoverage *FacetCoverageContract
    RequiredBlocks []BlockRequirement
    // ... rest unchanged
}
```

**测试更新**:`answer_semantic_view_compile_*_test.go` 中所有非 enumeration family 的 view 编译测试需要 assert `EnumerationSubType == EnumNone`。

---

## 6. compileEnumeration 的 sub-type 分支

`internal/types/answer_semantic_view_compile_enumeration.go:26-94` 改造:

```go
func compileEnumeration(ir *AnalysisIR, plan *AnswerSurfacePlan, subType EnumerationSubType) *AnswerSemanticView {
    view := &AnswerSemanticView{
        Family:             QFEnumeration,
        EnumerationSubType: subType,
    }
    if plan != nil {
        view.FacetCoverage = plan.FacetCoverage
        view.SummaryMode = plan.SummarySurfaceMode
    }
    if ir != nil && ir.AnswerContract.ExactResolution != nil {
        view.ExactResolution = ir.AnswerContract.ExactResolution
    }

    // Base scaffold (shared across all sub-types).
    view.RequiredBlocks = []BlockRequirement{
        requireSummaryBlock(...),
        {
            Kind:                 BlockOrderedList,
            MinCount:             1,
            MaxCount:             0,
            Required:             true,
            FacetIDs:             []string{string(FacetEnumerationItem)},
            AcceptableClaimForms: enumerationClaimFormsForSubType(subType),  // ← 新 helper
            Rationale:            ...,
            SurfaceRoleHint:      SurfacePrincipal,
        },
    }

    view.OptionalBlocks = []BlockRequirement{
        // ... existing OptionalBlocks unchanged
    }

    view.UncertaintyRules = enumerationUncertaintyRulesForSubType(subType)  // ← 新 helper

    view.RichnessCandidates = richnessCandidatesFromOptionalFacets(view.FacetCoverage)
    return view
}

func enumerationClaimFormsForSubType(s EnumerationSubType) []ClaimForm {
    switch s {
    case EnumMechanism:
        return []ClaimForm{ClaimDefinitionFact, ClaimCallEdge, ClaimGuardCondition, ClaimReturnFact}
    case EnumRole:
        return []ClaimForm{ClaimDefinitionFact}
    case EnumEntity, EnumNone:
        return []ClaimForm{ClaimDefinitionFact}
    }
    return []ClaimForm{ClaimDefinitionFact}
}

func enumerationUncertaintyRulesForSubType(s EnumerationSubType) []UncertaintyRule {
    base := UncertaintyRule{
        TriggerFacet:      string(FacetUncertaintyBoundary),
        ExpectedBlockKind: BlockCaveat,
        MissingMessage:    "Your enumeration's completeness is bounded ...",
    }
    if s != EnumRole {
        return []UncertaintyRule{base}
    }
    // EnumRole adds an abstraction-level uncertainty rule.
    return []UncertaintyRule{
        base,
        {
            TriggerFacet:      string(FacetEnumerationItem),
            ExpectedBlockKind: BlockOrderedList,
            MissingMessage:    "Items must answer at conceptual level (X is responsible for Y); pure call/guard chains without naming the responsibility are a regression.",
        },
    }
}
```

**调用点更新**:`internal/types/answer_semantic_view_compile.go`(顶层 dispatch)需要在 case QFEnumeration 时调用新签名:

```go
// Before:
//   case QFEnumeration:
//       view = compileEnumeration(ir, plan)
// After:
//   case QFEnumeration:
//       subType := ResolveEnumerationSubType(rm)
//       view = compileEnumeration(ir, plan, subType)
```

---

## 7. Finalizer dynamic user-section render

加 `internal/agent/answer_document_evaluator.go::renderAnswerDocSubFamilyRules`(放在文件末尾,与 `renderAnswerDocBlockContract` 同文件):

```go
// renderAnswerDocSubFamilyRules emits a per-dispatch user-section
// segment with sub-family-specific abstraction-level rules.
// Returns empty string when:
//   - view.Family != QFEnumeration
//   - view.EnumerationSubType == EnumNone (no typed sub-type signal)
//
// The emitted segment carries the rule body that previously lived as
// rule #8 (A'') in the static answer-document-skill Workflow. By
// emitting it conditionally per dispatch, LLM attention is focused on
// the rule that applies to THIS question, instead of being diluted
// across 24 always-on rules.
func renderAnswerDocSubFamilyRules(ctx *types.AgentContext, view *types.AnswerSemanticView) string {
    if view == nil || view.Family != types.QFEnumeration {
        return ""
    }
    var b strings.Builder
    switch view.EnumerationSubType {
    case types.EnumRole:
        b.WriteString("## Enumeration sub-family: ROLE\n\n")
        b.WriteString("This question asks what each item DOES at the conceptual level ")
        b.WriteString("(role, responsibility, purpose). Each ordered_list item's `text` MUST ")
        b.WriteString("describe the conceptual outcome (\"X is responsible for Y\"), not the ")
        b.WriteString("implementation chain (\"X calls Y which calls Z\"). Implementation chains ")
        b.WriteString("are anchors that ground the answer to verifiable code references — not ")
        b.WriteString("a substitute for the answer itself. Pair both for richness: conceptual ")
        b.WriteString("responsibility PLUS engineering anchors. A reader who needs to mentally ")
        b.WriteString("compose \"X calls Y, therefore X must do <something>\" is doing the synthesis ")
        b.WriteString("you owed them.\n")
        return b.String()
    case types.EnumMechanism:
        b.WriteString("## Enumeration sub-family: MECHANISM\n\n")
        b.WriteString("This question asks HOW each item works (call sequence, guard conditions, ")
        b.WriteString("return paths). Each ordered_list item's `text` SHOULD describe the call ")
        b.WriteString("chain explicitly — name the called functions, the guards, the data flow. ")
        b.WriteString("Conceptual responsibility descriptions alone (\"X handles input\") are ")
        b.WriteString("under-filled: the user asked for the mechanism. Use claim_form values from ")
        b.WriteString("{call_edge, guard_condition, return_fact} on each item.\n")
        return b.String()
    case types.EnumEntity:
        b.WriteString("## Enumeration sub-family: ENTITY\n\n")
        b.WriteString("This question asks for the entities themselves (implementers, subclasses, ")
        b.WriteString("handlers, instances). Each ordered_list item's `label` is the entity name ")
        b.WriteString("(verbatim from evidence anchor_symbol); each item's `text` is a 1-line role ")
        b.WriteString("description disambiguating this entity from siblings. No abstraction-level ")
        b.WriteString("constraint applies.\n")
        return b.String()
    case types.EnumNone:
        return ""
    }
    return ""
}
```

**调用点**:`BuildInitialInstruction()` 在 `renderAnswerDocBlockContract` 之后插入:

```go
// answer_document_evaluator.go around line 175 (between block contract and diagram contract)
if subFamily := renderAnswerDocSubFamilyRules(ctx, view); subFamily != "" {
    b.WriteString(subFamily)
    b.WriteString("\n")
}
```

---

## 8. New validator + ViolKind

### 8.1 新 ViolationKind 注册

`internal/types/violation_registry.go` 加(参考已有 `ViolPrincipalClaimUseMissing` 的 profile 结构):

```go
const ViolEnumerationAbstractionLevelMismatch ViolationKind = "enumeration_abstraction_level_mismatch"

// In default profile registry block:
{
    Kind:            ViolEnumerationAbstractionLevelMismatch,
    DefaultSeverity: SeverityMedium,  // SOFT until eval baseline shows STRICT is safe
    Owner:           OwnerFinalizer,
    Description:     "Enumeration item.text shape (role-prose vs call-chain-prose) doesn't match the resolved EnumerationSubType",
}
```

`internal/types/cgec_completeness_test.go:233-280` 在对应 map 加入:

```go
ViolEnumerationAbstractionLevelMismatch: true, // orchestrator/contract_check_block.go validateEnumerationAbstractionLevel
```

### 8.2 新 validator 实现

加 `internal/orchestrator/contract_check_block.go::validateEnumerationAbstractionLevel`(放在 `validateEnumerationItemLabelExtractorMatch` 后):

```go
// validateEnumerationAbstractionLevel (Phase C, <date>) checks that
// every ordered_list item's text shape matches the resolved
// EnumerationSubType:
//
//   - EnumRole: items must contain at least one
//     responsibility-shaped clause (verb is/handles/owns/manages/
//     responsible-for) AND not be dominated by call-chain
//     verbs (calls/invokes/dispatches/passes-to).
//   - EnumMechanism: items must contain at least one
//     mechanism-shaped clause (call/guard/return verb) AND not
//     be a single conceptual sentence with no call structure.
//   - EnumEntity / EnumNone: skip (no abstraction-level constraint).
//
// Skip conditions (no false positives):
//   - view == nil OR view.Family != QFEnumeration
//   - view.EnumerationSubType ∈ {EnumNone, EnumEntity}
//   - Block.SurfaceRole != principal
//   - Block.kind != ordered_list
//   - len(items) == 0
//
// Match semantics: case-folded substring match against verb sets.
// Verb sets are TYPED CONSTANTS in this file (see roleVerbs /
// mechanismVerbs below); they are not regex'd from user prompts —
// per §9.5 red line.
//
// Severity: Medium (SOFT) until baseline eval verifies low FP rate;
// flip to Strict via pipeline_contract_strict_kinds yaml when
// confidence accrues.
func validateEnumerationAbstractionLevel(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
    if doc == nil || view == nil {
        return nil
    }
    if view.Family != types.QFEnumeration {
        return nil
    }
    subType := view.EnumerationSubType
    if subType == types.EnumNone || subType == types.EnumEntity {
        return nil
    }

    var out []types.Violation
    for _, block := range doc.Blocks {
        if block.SurfaceRole != types.SurfacePrincipal {
            continue
        }
        if block.Kind != types.BlockOrderedList {
            continue
        }
        for i, item := range block.Items {
            if item.Kind != types.ItemKindPrincipal && item.Kind != "" {
                continue  // skip flow / caveat items
            }
            mismatch := classifyAbstractionMismatch(item.Text, subType)
            if mismatch != "" {
                out = append(out, types.Violation{
                    Kind:    types.ViolEnumerationAbstractionLevelMismatch,
                    Stage:   string(types.StageFinalize),
                    Repair:  fmt.Sprintf("block %q item[%d]: %s", block.ID, i, mismatch),
                    // typed metadata so hint composer can format precisely:
                    Metadata: map[string]string{
                        "block_id":         block.ID,
                        "item_index":       strconv.Itoa(i),
                        "expected_subtype": string(subType),
                    },
                })
            }
        }
    }
    return out
}

// roleVerbs are conceptual responsibility verbs that should dominate
// EnumRole item text.
var roleVerbs = []string{
    "is responsible for", "handles", "owns", "manages", "represents",
    "encapsulates", "models", "tracks", "coordinates",
    "负责", "处理", "管理", "维护", "表示",
}

// mechanismVerbs are call-chain / control-flow verbs that should
// dominate EnumMechanism item text.
var mechanismVerbs = []string{
    "calls", "invokes", "dispatches", "passes", "returns",
    "checks", "guards", "branches", "iterates",
    "调用", "派发", "返回", "判断", "分支",
}

// classifyAbstractionMismatch returns "" when the item.text shape
// matches the expected sub-type, else returns a Repair string.
func classifyAbstractionMismatch(text string, subType types.EnumerationSubType) string {
    lower := strings.ToLower(text)
    roleHits := countAnyOf(lower, roleVerbs)
    mechHits := countAnyOf(lower, mechanismVerbs)

    switch subType {
    case types.EnumRole:
        // OK if item has at least one role verb AND mech verbs do not
        // dominate (≤ 2x role verbs).
        if roleHits == 0 && mechHits > 0 {
            return "EnumRole expects role-shaped prose (X is responsible for Y); item dominated by mechanism verbs (calls/invokes/...). Add a responsibility clause or restructure as 'X is responsible for <conceptual outcome>; the mechanism is <call chain>'."
        }
        if roleHits > 0 && mechHits > roleHits*2 {
            return "EnumRole has the responsibility clause but mechanism verbs dominate the rest of the text. Move call-chain detail to the citation rationale, keep the role description as the lead sentence."
        }
    case types.EnumMechanism:
        if mechHits == 0 && roleHits > 0 {
            return "EnumMechanism expects mechanism-shaped prose (X calls Y, then Z); item only states a conceptual responsibility. Describe the call sequence / guard / return path explicitly."
        }
    }
    return ""
}

func countAnyOf(text string, needles []string) int {
    n := 0
    for _, needle := range needles {
        if strings.Contains(text, strings.ToLower(needle)) {
            n++
        }
    }
    return n
}
```

**WIRE INTO**: `runV2BlockOraclesWithMut` (line 1681-1706) 加一行:

```go
out = append(out, validateEnumerationAbstractionLevel(doc, view)...)
```

### 8.3 Hint composer 新 case

`internal/analysis/hint/composer.go:374-440` 加 case:

```go
case types.ViolEnumerationAbstractionLevelMismatch:
    return "Adjust the named ordered_list item(s) so the prose shape matches the question's enumeration sub-type. The ## Enumeration sub-family section in the user prompt names whether the question is ROLE (items describe what X is responsible for, conceptually), MECHANISM (items describe how X works — call chain, guards), or ENTITY (items name the entity + 1-line role). The validator's Repair line names the offending block id + item index + the expected shape."
```

### 8.4 Cooccurrence rule(可选)

如果 `ViolEnumerationAbstractionLevelMismatch` 经常与 `ViolEnumerationLabelUngrounded` 共发(因为 LLM 选错抽象层时往往也胡编 label),`internal/orchestrator/repair_cooccurrence.go::defaultCooccurrenceRules` 加:

```go
{
    Primary: types.ViolEnumerationLabelUngrounded,
    Derived: []types.ViolationKind{types.ViolEnumerationAbstractionLevelMismatch},
    Reason:  "label fabrication often comes paired with abstraction shift; treat label grounding as the root cause once both fire",
},
```

**Caveat**: 这条 cooccurrence 只在 baseline eval 看到两者高频共发后再加,初版 ship 时可不加。

---

## 9. Commit 序列(单 session 6-9 commits)

每条 commit 必须独立可 revert。Eval 在 commit 8 之后整批跑。

### Commit 1 — Type definitions only(无逻辑)

- 加 `internal/types/facet_plan.go`:`EnumerationSubType` enum + `AllEnumerationSubTypes()` + `IsValid()`
- 加 `internal/types/answer_semantic_view.go`:`AnswerSemanticView.EnumerationSubType` 字段
- 改 `internal/types/answer_semantic_view_compile_enumeration.go::compileEnumeration` 接受新参 + 调用点更新
- 加 unit test `TestEnumerationSubType_*` 覆盖 enum + IsValid
- 全部 view 编译测试加 `EnumerationSubType == EnumNone` assert(non-enum families)
- 改 `internal/types/answer_semantic_view_compile.go` 顶层 dispatch
- **Eval**: `go test ./internal/types/...` 全绿。无真 eval(类型层无行为变化)。

### Commit 2 — ResolveEnumerationSubType + grep PredicateAxis 现状

- 实施前先 grep 确认 PredicateAxis 已有值。如缺 `PredicateAxisRole / PredicateAxisResponsibility / PredicateAxisDispatches`,本 commit 先加(同时改 cgec_completeness_test.go)。
- 加 `internal/types/facet_plan.go::ResolveEnumerationSubType`(纯函数,无 LLM)
- 加 unit test `TestResolveEnumerationSubType_*`(至少 4 case:role / mechanism / entity / none)
- compileEnumeration 调用 `enumerationClaimFormsForSubType` + `enumerationUncertaintyRulesForSubType` 两个 helper
- **Eval**: unit test 全绿。无真 eval。

### Commit 3 — Finalizer user-section render

- 加 `internal/agent/answer_document_evaluator.go::renderAnswerDocSubFamilyRules`
- 在 `BuildInitialInstruction` 调用
- 加 `internal/agent/answer_document_evaluator_test.go::TestRenderSubFamilyRules_*`(4 case)
- **Eval**: `go test ./internal/agent/...` 全绿。**真 eval qf_arch x4**(验证 prompt 多了一段不引入回归)。

### Commit 4 — 删除 static Workflow 中的 A'' 规则(commit 3 已动态接管)

- 删 `internal/skill/defaults.go:126`(规则 #8 abstraction-level matching)
- 改 `internal/skill/defaults_test.go` 对应 substring assert 替换为 negative assert(不应再出现在 static skill)
- **Eval**: 真 eval qf_arch x4 = 4/4(本 Phase 主 bar)。

### Commit 5 — New ViolKind registration(无 validator 行为)

- 加 `internal/types/violation_registry.go`:`ViolEnumerationAbstractionLevelMismatch` profile
- 加 `internal/types/cgec_completeness_test.go` map entry
- 加 `ViolationKind` 字符串测试
- **Eval**: `go test ./internal/types/...` 全绿。无行为改动。

### Commit 6 — New validator + wire into runV2BlockOraclesWithMut

- 加 `internal/orchestrator/contract_check_block.go::validateEnumerationAbstractionLevel` + 2 verb sets + helpers
- 加 `internal/orchestrator/contract_check_block_test.go::TestValidateEnumerationAbstractionLevel_*`(至少 6 case:role-correct / role-wrong / mech-correct / mech-wrong / entity-skip / none-skip)
- Wire into `runV2BlockOraclesWithMut`
- **Eval**: `go test ./internal/orchestrator/...` 全绿。真 eval qf_arch x4 + m1a x4 = 8(SOFT 默认,无回归)。

### Commit 7 — Hint composer case + (optional) cooccurrence

- 加 `internal/analysis/hint/composer.go::summariseExactFix` 第 26 case
- 加 `internal/analysis/hint/composer_test.go` 对应 case test
- (可选)加 cooccurrence rule(若 commit 6 eval 显示 label-ungrounded 共发)
- **Eval**: composer test 全绿。无真 eval(纯 hint 增强)。

### Commit 8 — 真 eval 验收(BLOCKING gate)

跑全 case x4(20+ 次):

- qf_arch x4 = 4/4(主 bar)
- 加新 case `evals/qf_mechanism/`(若不存在,本 commit 顺手新增 spec — questions like "explorer 怎么走 evidence pipeline?" mechanism shape)
- m1a x4 = 4/4(不回归)
- s1a / u3a / qf_config_precedence x4 不回归

如有任一 case 回归 → 该 commit revert 链上的相关 commit,SOFT validator 留 violation 但不 fail loud。

### Commit 9 — Ship 收尾

- 更新 `MEMORY.md` 顶部为本 Phase SHIPPED
- 写 `project_session_finalizer_phase_c_shipped.md` 收尾 doc
- 本设计文档 §0 Status: 改 `SHIPPED <date>`
- (可选)新加红线 `feedback_typed_subfamily_for_attention.md` 记录"细分 sub-family + 动态 render 比 prompt 重组更治本"

---

## 10. 真 eval 跑法

跟 Phase A §6 相同的 `go test ./evals/<case>/ -run Test<Case>_x4`,新增 `evals/qf_mechanism/` 路径(本 Phase 必须新加该 case spec — 让 EnumMechanism 路径有真实 case 覆盖)。

新 case spec 模板参考已有 `evals/qf_arch/` 的 `case_spec.json`(或类似格式)。Mechanism case 示例题:
- "explorer 是如何调用 read_file / grep / repo_map 的?"
- "extractor 在 emit_answer_symbol 之前做了哪些验证?"

regex 验证:答案中应含至少 N 个 mechanism verb(call / invoke / dispatch / 调用 / 派发)。

---

## 11. 红线 checklist(开发者必读)

实施 Phase C 前,必须确认理解:

- 🔴 `feedback_precise_signals_for_hard_gates.md` — `ResolveEnumerationSubType` 必须用 typed 信号(PredicateAxis enum / AnswerSubject.Kind enum / Predicates bool),禁用 question text regex
- 🔴 `feedback_axis_collapse_alignment.md` — 若选方案 Y(amplifier rule),必过 `axis_collapse_fixture_test.go`;选方案 X(纯函数)则不受此红线约束
- 🔴 `feedback_termgraph_vs_analyzerhints_entities.md` — 不要从 TermGraph 读 entity,要读 AnalyzerHints.Entities
- 🔴 `feedback_typed_signal_six_spot_sync.md` — 加新 typed signal 必须 6 处同步:struct + schema desc + skill prompt + retry hint + JSON decoder + cooccurrence/RepairLocus 映射。本 Phase 加 `EnumerationSubType` 不直接经 LLM emit(deterministic resolver),所以 (2)(3) 不适用,(1)(4)(5)(6) 必须做
- 🔴 `feedback_no_eval_bar_relaxation.md` — 任何 case FAIL 不调 case spec,只回退 commit
- 🔴 `feedback_no_internal_info_in_llm_prompts.md` — `renderAnswerDocSubFamilyRules` 输出文本不能露 system 内部术语("EnumRole" / "EnumerationSubType" 都不要写,改成自然语言"role-shape question")
- 🔴 `feedback_root_cause_only.md` — Phase A jitter 没修干净不许甩锅给"prompt 重组",必须找根因(LLM 注意力分散 → 本 Phase typed signal + dynamic render 是根因解)
- 🔴 `feedback_prompt_redline_checklist.md` — `renderAnswerDocSubFamilyRules` 文本必过 ATOMIC 7 条 R3+R4+R5+R6+R7+SST+R2' checklist
- 🔴 `feedback_no_dismiss_as_llm_flake.md` — 真 eval 任何 outlier 都查根因

---

## 12. 失败回退路径

如 commit 8 真 eval 出现:

- **qf_mechanism x4 回归(新 case 在 EnumMechanism 路径有问题)**:回退 commit 6 + 7,SOFT validator 保留但不引入新 case;或直接调小 mechanismVerbs 集合
- **m1a 回归(EnumNone 路径误归类)**:`ResolveEnumerationSubType` 返回非 EnumNone 但实际是 entity 题型 → grep m1a 题目 + AnalyzerHints,补 `ResolveEnumerationSubType` 的 priority 规则
- **qf_arch 仍 3/4**:本 Phase 失败 — 说明 typed signal 也没传到位。检查 `view.EnumerationSubType` 在 finalizer dispatch 时是否真为 EnumRole(打 log + grep dispatch trace)

---

## 13. Phase C 不做什么(scope discipline)

- ❌ 不动 hint composer 25 case(只加第 26 case)
- ❌ 不动 cooccurrence 9 条(可选加 1 条,本 commit 序列不必含)
- ❌ 不引入新 QuestionFamily(EnumerationSubType 是 sub-type,不上升到 top-level)
- ❌ 不动 amplifier R1/R2/R3(本 Phase 推荐方案 X 不进 amplifier)
- ❌ 不动 OutputFormat(它是 schema 教学,与 sub-family 正交)
- ❌ 不重构 `BuildInitialInstruction` 主流程,只插入 1 个新 render 调用

---

## 14. 验收 checklist(本 session 收口前必跑)

- [ ] commit 序列 1-9 全部 push 到 origin/main(`feedback_confirm_before_push.md`)
- [ ] qf_arch x4 = 4/4(本 Phase 主 bar)
- [ ] qf_mechanism x4 = 4/4(本 Phase 新 bar)
- [ ] m1a x4 = 4/4(不回归)
- [ ] s1a / u3a / qf_config_precedence x4 不回归
- [ ] `go test ./internal/types/...` 全绿
- [ ] `go test ./internal/analysis/...` 全绿(amplifier + hint)
- [ ] `go test ./internal/orchestrator/...` 全绿(新 validator)
- [ ] `go test ./internal/agent/...` 全绿(新 render)
- [ ] `go test ./internal/skill/...` 全绿(删 A'' 规则后 substring assert 已更新)
- [ ] `MEMORY.md` 顶部更新到本 Phase SHIPPED
- [ ] 写 `project_session_finalizer_phase_c_shipped.md` 收尾 doc
- [ ] 本设计文档 §0 Status 改 `SHIPPED <date>`

---

## 附录 A — 数据流时序图(详细版)

```
T=0  USER question:
       "Codrax orchestrator 的 4 个核心 stage 各负责什么?"
                     ↓
T=1  ANALYZER LLM emit_analysis:
       Intent: IntentEnumerate
       Predicates.IsCategoryEnumeration: false
       PredicateAxis: PredicateAxisResponsibility  ← typed signal
       AnswerSubject.Kind: SubjectInterface
       AnalyzerHints.Entities: ["analyze", "explore", "extract", "finalize"]
                     ↓
T=2  amplifier.Amplify(rm) [pre-compile]:
       R1 multi-subject fires: 4 entities → flip IsCategoryEnumeration ?
       (NB: R1 may flip Predicates — see commit history)
                     ↓
T=3  compiler.Compile(rm) → AnswerContract:
       MustInclude: ["analyze", "explore", "extract", "finalize"]
                     ↓
T=4  amplifier.AmplifyPostCompile(rm, contract) [post-compile]:
       R3 must-include pinning fires: pin all 4 entities
                     ↓
T=5  BuildAnswerSemanticViewForBusContext(busCtx):
       family = ResolveQuestionFamily(rm) = QFEnumeration
       ★ subType = ResolveEnumerationSubType(rm)
         IsCategoryEnumeration=false
         PredicateAxis=PredicateAxisResponsibility → return EnumRole
       view = compileEnumeration(ir, plan, EnumRole)
       view.EnumerationSubType = EnumRole
       view.RequiredBlocks[1].AcceptableClaimForms = [ClaimDefinitionFact]
       view.UncertaintyRules += role-abstraction rule
                     ↓
T=6  FINALIZER BuildInitialInstruction:
       ... (existing user sections) ...
       ★ renderAnswerDocSubFamilyRules emits:
         "## Enumeration sub-family: ROLE
          This question asks what each item DOES at the
          conceptual level (role, responsibility, purpose).
          Each ordered_list item's `text` MUST describe ..."
                     ↓
T=7  LLM emit_answer_document with 4 items:
       items[0].text: "analyze 负责把用户请求分类并产 AnalysisIR"
                       (role-shaped — has 负责)
       items[1].text: "explore 调用 read_file / grep 收集 evidence"
                       (mechanism-shaped — only 调用)
                     ↓
T=8  contract_check_block.runV2BlockOraclesWithMut:
       ... existing 13 validators run ...
       ★ validateEnumerationAbstractionLevel:
         view.EnumerationSubType = EnumRole
         items[0]: roleHits=1 (负责), mechHits=0 → OK
         items[1]: roleHits=0, mechHits=1 (调用) → FIRE
           Violation{
             Kind: ViolEnumerationAbstractionLevelMismatch,
             Repair: "block 'enum-list-1' item[1]: EnumRole expects ...",
             Metadata: {block_id, item_index=1, expected_subtype=role}
           }
                     ↓
T=9  hint.Composer.Compose:
       summariseExactFix → "Adjust the named ordered_list item(s)
                              so the prose shape matches the question's
                              enumeration sub-type. ..."
       Render → user-section RetryHint
                     ↓
T=10 LLM retry emit_answer_document:
       items[1].text: "explore 负责采集证据;它通过调用 read_file ..."
                       (now role-shaped + mechanism anchor)
                     ↓
T=11 validateEnumerationAbstractionLevel: all OK → PASS
                     ↓
T=12 SHIP answer to user.
```

---

## 附录 B — 与 Phase A 关系表

| 维度 | Phase A | Phase C |
|---|---|---|
| 触动文件数 | ~3 (skill/defaults.go, hint/composer.go, several test files) | ~10 (types/facet_plan.go, types/answer_semantic_view*.go, agent/answer_document_evaluator.go, orchestrator/contract_check_block.go, types/violation_registry.go, hint/composer.go, several test files, evals/qf_mechanism/) |
| 引入新 typed signal | 否(纯删) | 是(EnumerationSubType + ViolKind) |
| 改 BlockRequirement | 否 | 是(AcceptableClaimForms 按 sub-type 分支) |
| 改 finalizer user section | 否 | 是(加 1 个 render 函数 + 1 个 BuildInitialInstruction 调用) |
| 改 validator 集合 | 否(行为不变) | 是(加第 14 个 V2 validator) |
| 改 hint composer | 是(增强 3 case) | 是(加第 26 case) |
| eval 风险 | 中(删规则可能让 retry 多 1 轮) | 中(typed 分类可能误判;new validator 默认 SOFT) |
| 单 session 可控? | 是,5-8 commits | 是,6-9 commits |
| 治 A'' jitter 力度 | 中(注意力份额翻倍但仍依赖 LLM 自觉判断 trigger) | 高(typed signal 直接路由 + hard gate) |

**实施顺序硬性**:Phase A → eval 验证 → 决定是否需要 Phase C → Phase C。如 Phase A 已 4/4,Phase C 可作为 future work / 当遇到新 question shape 需扩展时再启动。

---

## 附录 C — Cross-reference

`MEMORY.md` 索引中本设计的位置:`docs/design/finalizer_phase_c_shape_contract.md`。Phase C 完成 ship 后,在 `MEMORY.md` 顶部增加:

```
- 🟢 [**finalizer Phase C — SHAPE_CONTRACT typed sub-family SHIPPED (<date>, N commits)**](project_session_finalizer_phase_c_shipped.md) — 加 EnumerationSubType typed enum + ResolveEnumerationSubType deterministic resolver + sub-family-aware compileEnumeration + renderAnswerDocSubFamilyRules dynamic user-section + validateEnumerationAbstractionLevel V2 validator + ViolEnumerationAbstractionLevelMismatch ViolKind + composer case 26。qf_arch x4 = 4/4 + qf_mechanism x4 = 4/4。下一阶段:无(本议题收口)。
```

并加一条 cross-session red line:

```
- 🔴 [**typed sub-family signal 比 prompt 重组更治本**](feedback_typed_subfamily_for_attention.md) — Phase C SHIPPED 经验
```

---

## 附录 D — 可能遇到的 codebase quirk

1. **PredicateAxis 现有值不全**:实施 commit 2 前 grep 确认。如缺 RoleResponsibility 系列,加进 `internal/types/predicate_axis.go`(或同等位置)+ cgec_completeness_test.go 同步。
2. **AnswerSubject.Kind 缺 SubjectRoleNoun**:本 Phase 不强求加,resolver 退到 PredicateAxis-only。
3. **AnalyzerHints.Predicates 字段命名**:实施前 grep `IsCategoryEnumeration` 确认精确字段名;不同文件可能有 `Predicates.Category` 等变体。
4. **renderAnswerDocSubFamilyRules 插入位置**:在 `BuildInitialInstruction` 哪行插入要看 user section 渲染顺序 — 推荐在 `renderAnswerDocBlockContract` 之后、`renderAnswerDocDiagramContract` 之前(让 sub-family 教学紧跟 block 合约)。
5. **ItemKind 字段名**:`item.Kind` vs `item.ItemKind` — 实施前确认 V2 schema 真名(可能在 `internal/types/answer_document_v2.go`)。
6. **PredicateAxisDispatches 是否合并到 Calls**:可能重复;需要看现有 grep 结果决定是否新加。
7. **真 eval 命令**:本仓库的 eval test 命名约定可能是 `TestE2E_<case>_x4` 而非 `Test<Case>_x4` — 实施前 grep `t.Run("` 确认。
