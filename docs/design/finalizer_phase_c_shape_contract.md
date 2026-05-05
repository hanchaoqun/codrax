# Finalizer Prompt Phase C — SHAPE_CONTRACT(typed Enumeration sub-family)

**Status**: Design (not yet implemented).
**Target session**: single-session ship, 7-10 commits.
**Predecessor**: `docs/design/finalizer_phase_a_rule_bisection.md`(Phase A — 删 machine-checkable 规则,**必须先 ship 并经真 eval 验证不回归**;Phase A 残留若 qf_arch 仍非 4/4,本 Phase 才有动机).
**Successor**: 暂无(Phase B 增强 cooccurrence rule 是平行 track,可独立).
**Owner**: 任意接手者(本文档目标:不熟悉本仓库的开发者读完一遍即可开始改).
**Eval bar**: qf_arch x4 = 4/4(若 Phase A 已达成此 bar,本 Phase bar 上调到 qf_arch x8 全 PASS,且新加的 mechanism-enumeration case x4 也全 PASS).

**Baseline**: 本文档所有 `internal/...` 文件 file:line 引用基于 **commit `4cd053d`**(等价于代码层 `efa4ff3` — `4cd053d` 仅加 design doc,不改代码)。后续提交可能让具体行号漂移。**实施前**先跑 §15 速查表的 grep 命令对齐行号;若漂移,以 grep 真实结果为准,不要按文档行号瞎改。可用 `git log --oneline efa4ff3..HEAD -- internal/` 查 baseline 后所有相关代码改动。

---

## 0. TL;DR — 一句话设计

Phase A 通过删冗规则把 A''(role-enumeration abstraction-level matching)的注意力份额从 1/24 提到 ~1/10。Phase C 进一步从**架构层**让 finalizer prompt 在每次 dispatch 只看到与本 question shape 相关的判断规则:

1. 在 `internal/types/answer_semantic_view.go::AnswerSemanticView` 加 typed 字段 `EnumerationSubType`(枚举 `EnumNone / EnumRole / EnumMechanism / EnumEntity`)
2. 在 `internal/types/facet_plan.go` 同位置加 `ResolveEnumerationSubType` 纯函数(读 RequestModel 已有 typed 信号 — 无需新加 PredicateAxis 值或新 AnswerSubjectKind 值)
3. 加 amplifier R4 rule(`internal/analysis/amplifier/rule_r4_enumeration_subtype.go`),走 `init()` 注册到 `preCompileRules`,在 RequestModel 加 `EnumerationSubTypeHint EnumerationSubType` 字段,view 编译时读
4. `internal/types/answer_semantic_view_compile_enumeration.go::compileEnumeration` 内部 derive sub-type 写入 view(**不改外部签名**,因此跨 family dispatch 不动)
5. `internal/agent/answer_document_evaluator.go::BuildInitialInstruction` 加 `renderAnswerDocSubFamilyRules`,**只在 view.EnumerationSubType=EnumRole/EnumMechanism 时**渲染对应教学到 user section
6. 加新 V2 validator `validateEnumerationAbstractionLevel`,**默认 SOFT + Promotable=false**(noisy verb-substring 信号只作软引导,符合 R3 精确信号红线)
7. Hint composer 加新 case + ViolKind 6-处 sync

**为什么这能彻底治 A'' jitter**:
- A'' 不再放在 24 条 Workflow 中**碰运气被注意到**;它作为 **per-dispatch typed user-section 段** 直接 render 到 LLM 视野里
- 同时有 **typed validator 作软兜底**,不靠 LLM 自觉;noisy 信号只 emit retry hint,不阻 ship

**为什么不在 Phase A 直接做**:Phase C 涉及类型系统改动 + amplifier/finalizer/validator 三处协调,改动面广;Phase A 改动局部、风险可控,先做 Phase A 验证"删冗规则"路径是否单独足够。如 Phase A 已让 qf_arch x4 = 4/4 且 cross-case 不回归,Phase C 可以**降优先级**(留作未来面对新 question shape 时的扩展模板)。

---

## 1. 背景:为什么这个 Phase 存在

### 1.1 当前 Enumeration family 内部缺 sub-family 区分

`internal/types/facet_plan.go::ResolveQuestionFamily`(line 573)把 8 个 family 之一定为 `QFEnumeration`。但 enumeration 实际有三种语义子类:

- **Role-enumeration**:"列出每个 X 的职责 / 干什么"。期望答案是 *conceptual responsibility*("X 负责 Y"),禁 implementation chain;commit `efa4ff3` 加的 A'' 规则就是教 LLM 这点
- **Mechanism-enumeration**:"列出每个 X 怎么干 / 调用了什么"。期望答案是 *implementation chain*("X 调用 Y, Y 调用 Z");与 Role-enum 完全相反
- **Entity-enumeration**:"列出所有 implementer / subclass / handler"。期望答案是 *symbol slate*(symbol name + file:line + 1 行 role description);与前两种又不同

当前 `internal/types/answer_semantic_view_compile_enumeration.go::compileEnumeration`(line 26)**没有**任何 sub-type 区分:

```go
// answer_semantic_view_compile_enumeration.go:37-55(真实代码)
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
- validator 加一条 `validateEnumerationAbstractionLevel`,emit telemetry + retry hint(不靠 LLM 自觉,但因为信号 noisy 只作软引导)

这是 `feedback_precise_signals_for_hard_gates.md` 红线的标准应用:把 "判断是否 role-enumeration" 这个上游决策从 LLM 自然语言判断升级到 typed 信号,但 abstraction-level 本身的检查(verb-substring)是 noisy → 只用作 SOFT 引导,不进 STRICT gate。

### 1.3 与 Phase A 的关系

| Phase | 改动层 | 风险 | 修 A'' jitter 的力度 |
|---|---|---|---|
| Phase A | prompt 内容(删冗规则) | 低 | 中(注意力份额 1/24 → 1/10,但仍依赖 LLM 自觉判断 trigger) |
| Phase C | 类型系统 + amplifier 后处理 + finalizer dynamic render + SOFT validator | 中(改 7+ 文件,涉及跨 stage 数据流) | 高(typed signal + soft validator,不再依赖 LLM 自觉) |

**实施顺序硬性**:Phase A 必须先 ship。原因:
- 如果 Phase A 已让 qf_arch x4 = 4/4,Phase C 可降优先级 — 大改动需要明确 ROI 才值得
- Phase A 的"删冗规则"为 Phase C 释放 prompt 注意力预算 — Phase C 加新 sub-family 规则到 user section 时,如果 Workflow 还是 24 条,新规则会被同样稀释
- Phase A 验证 hint composer 增强能 cover 删除规则 → Phase C 加新 ViolKind 时复用同一套 hint 模式

---

## 2. 代码定位指南(让不熟代码的开发者立即上手)

下面所有文件路径都是相对 `/home/chatpp/codrax`。**所有 file:line 经真实代码审计验证**。

### 2.1 类型系统(SHAPE_CONTRACT 定义所在地)

| 文件 | 关键 line | 内容 |
|---|---|---|
| `internal/types/facet_plan.go:48-101` | enum + AllList | `QuestionFamily` 8 个值 + `AllQuestionFamilies()` |
| `internal/types/facet_plan.go:573-633` | `ResolveQuestionFamily()` | 7 优先级规则 deterministic dispatch |
| `internal/types/answer_semantic_view.go:22-89` | struct | `AnswerSemanticView`(本 Phase 加 `EnumerationSubType` 字段) |
| `internal/types/answer_semantic_view.go:108-130` | struct | `BlockRequirement`(含 `AcceptableClaimForms []ClaimForm`) |
| `internal/types/analysis_ir.go:44-186` | struct | `RequestModel` 22 字段(本 Phase 加 `EnumerationSubTypeHint`) |
| `internal/types/analysis_ir.go:285-296` | enum | `PredicateAxis` 8 真实值:`AxisUnknown / AxisCall / AxisRegister / AxisDefine / AxisReturn / AxisConfigure / AxisCondition / AxisImplement`(本 Phase **不动**) |
| `internal/types/analysis_ir.go:344-394` | struct | `SemanticPredicates` 7 bool 字段(`IsCategoryEnumeration / IsRoleLocateLookup / IsCountQuestion / IsCrossComponent / IsRelationalLookup / IsScalarAnswer / IsHistoryLookup`)— **顶层字段,非 AnalyzerHints 内** |
| `internal/types/analysis_ir.go:398-445` | struct | `AnalyzerHints` 10 字段(`Keywords/Entities/PrimaryEntities/MentionedEntities/DerivedEntities/ExactTargets/ExactContextTerms/ExactContextRoles/CapabilitySurface/Kind`)— **无 Verbs 字段** |
| `internal/types/analysis_ir.go:261-265` | struct | `AnswerSubject{Kind, EntityAxes, Confidence}` |
| `internal/types/analysis_ir.go` (search `SubjectFunctionName`) | enum | `AnswerSubjectKind` 13 真实值:`SubjectUnknown / SubjectFunctionName / SubjectTypeName / SubjectHandlerRoute / SubjectConfigKey / SubjectReturnValue / SubjectFilePath / SubjectStringLiteral / SubjectNumeric / SubjectEnumValue / SubjectStructField / SubjectInterface / SubjectGeneric`(**无 SubjectRoleNoun**) |
| `internal/types/analysis_ir.go:835-843` | struct | `AnswerContract`(Phase C 不动) |
| `internal/types/answer_semantic_view_compile.go:56-88` | dispatch | `BuildAnswerSemanticView()` 顶层 family switch — Phase C 不改签名 |
| `internal/types/answer_semantic_view_compile_enumeration.go:26-94` | `compileEnumeration()` | 编译 QFEnumeration 的 view(本 Phase 改这里加 sub-type 内部 derive) |
| `internal/types/claim_form.go`(grep `ClaimForm`)| enum | 10 个 ClaimForm 值 |

### 2.2 Analyzer 后处理(EnumerationSubTypeHint 的 producer)

| 文件 | 关键 line | 内容 |
|---|---|---|
| `internal/analysis/amplifier/amplifier.go:75-89` | type + slice | `preCompileRule` / `postCompileRule` 类型 + `preCompileRules` / `postCompileRules` 全局 slice |
| `internal/analysis/amplifier/amplifier.go:100-109` | `Amplify()` | pre-compile 入口:浅拷贝 rm,顺序跑 rule,**rule in-place mutate &out**,后续 rule 可读前者写入 |
| `internal/analysis/amplifier/amplifier.go:119-130` | `AmplifyPostCompile()` | post-compile 入口:in-place mutate `*contract` |
| `internal/analysis/amplifier/amplifier.go:63-69` | struct | `Observation{Rule, Field, Before, After, Reason}` 5 字段 |
| `internal/analysis/amplifier/rule_r1_multi_subject.go` | line ~120-122 | R1 用 `func init()` append 到 `preCompileRules`(**rule 注册模式 — Phase C R4 必须复制**) |
| `internal/analysis/amplifier/rule_r2_typed_name_parity.go` | line ~281-283 | R2 同上 |
| `internal/analysis/amplifier/rule_r3_must_include_pinning.go` | line ~108-110 | R3 用 `func init()` append 到 `postCompileRules` |
| `internal/analysis/amplifier/trap_fixture_test.go` | 整文件 | TermGraph trap 防护 fixture(本 Phase R4 必过)|
| `internal/analysis/amplifier/axis_collapse_fixture_test.go` | 整文件 | axis_collapse trap 防护 fixture(本 Phase R4 必过)|
| `internal/agent/analyzer.go` 中 buildAnalysisIR | line ~1317 | `amplifier.Amplify(rm)` 调用点(R4 注册后自动被 invoke)|
| `internal/agent/analyzer.go` 中 buildAnalysisIR | line ~1423 | `amplifier.AmplifyPostCompile(rm, &out.AnswerContract)` 调用点 |

### 2.3 Finalizer dynamic user-section 渲染(SHAPE_CONTRACT 的 consumer)

| 文件 | 关键 line | 作用 |
|---|---|---|
| `internal/agent/answer_document_evaluator.go:127-330` | `BuildInitialInstruction()` | 顶层组装 user section,共 13 个 renderAnswerDoc* 调用按顺序串接 |
| `internal/agent/answer_document_evaluator.go:894-933` | `renderAnswerDocBlockContract()` | 现有渲染示范(本 Phase 在它之后插入 `renderAnswerDocSubFamilyRules`)|
| `internal/agent/answer_document_evaluator.go:529-610` | `renderAnswerDocEnumerationBoundary()` | 现有 enumeration-related 渲染示范(nil-safe pattern)|
| `internal/agent/answer_document_evaluator.go:1001` | `renderAnswerDocFacetCoverage()` | nil-safety 三层检查 pattern 示范 |

**BuildInitialInstruction user section 渲染顺序**(基于真实代码):

```
Line 150  Note (mode hint)
Line 158  RetryState (if non-empty)
Line 167  BlockContract (if non-empty)
          ← 本 Phase 插入 renderAnswerDocSubFamilyRules 在 line 170-176
Line 177  Diagram contract section
Line 196  ConfigTraceRoleCoverage
Line 199  CapabilitySurface
Line 202  FacetCoverage
Line 205  ExactResolutionContract
Line 208  AcceptedClosure
Line 211  LogSourceDrift
Line 214  ExternalObservationSeeds
Line 217  SubmissionChecklist
Line 220  StepBackbone
Line 223  EnumerationBoundary
```

### 2.4 Validator(EnumerationSubType 驱动的新 SOFT validator)

| 文件 | 关键 line | 作用 |
|---|---|---|
| `internal/orchestrator/contract_check_block.go:1681-1706` | `runV2BlockOraclesWithMut` | 13 个 V2 validator 装配(本 Phase 加第 14 个,推荐插入位置 line 1690-1691,即 #5 与 #6 之间)|
| `internal/orchestrator/contract_check_block.go:134-200` | `validatePrincipalClaimUse` | 典型 validator 模式参考(三层 nil-check + skip 条件 + Violation struct 填充) |
| `internal/orchestrator/contract_check_block.go:1746-1856` | `validateEnumerationItemLabelGrounding` | 枚举专用 validator 参考(`mut.TurnAArtifacts()` 读 evidence pool)|
| `internal/orchestrator/contract_check_block.go:1968` | `viewNeedsExtractorBackedEnumerationSlate` | **关键**:新 validator 用此函数防跨 family 误伤(QFCallChain/QFRootCauseTrace/QFComparison 也可有 ordered_list)|

### 2.5 Hint composer + cooccurrence

| 文件 | 关键 line | 作用 |
|---|---|---|
| `internal/analysis/hint/composer.go:142-201` | `Compose()` | 顶层入口,接 violations 出 6 字段 Hint struct |
| `internal/analysis/hint/composer.go:374-441` | `summariseExactFix` | **真实 15 个 case label**(awk 限定函数体 grep 数到 15;`grep -c "^	case"` 全文 25 是因函数外仍有其他 switch 块)硬编码 ExactFix 文本,本 Phase 加新 case(具体序号取决于 Phase A 是否先 ship 增加 case)|
| `internal/analysis/hint/composer.go:445-540` | `buildAllowedSet` | 给 LLM 列举允许值 |
| `internal/orchestrator/repair_cooccurrence.go:83-337` | `defaultCooccurrenceRules` | 9 条 Primary→Derived,本 Phase 可选加 1 条 |

### 2.6 Violation kind registry + 完备性测试

| 文件 | 关键 line | 作用 |
|---|---|---|
| `internal/types/violation_registry.go:67` | `ViolKindSpec` struct | 真实 11 字段:`Kind / DefaultSeverity / SoftByDefault / Promotable / FallbackLocus / Layer / Description / FixableByAgents / SchemaDescriptionFragment / Implies / CaveatFamilyID` |
| `internal/types/violation_registry.go:301-308` | `RegisterViolKind` | panic 校验必填字段(`DefaultSeverity / FallbackLocus`)|
| `internal/types/cgec_completeness_test.go:187-271` | `covered` map | 已接线 ViolKind 必须在此 map 加 `true` entry |
| `internal/types/cgec_completeness_test.go:292-337` | `kindSymbols` map | 所有 ViolKind 必须有符号映射 entry |
| `internal/types/cgec_completeness_test.go:273-284` | `pending` map | 未接线时用 |
| `internal/types/violation.go:753` | `Violation` struct | 真实 10 字段:`Kind / Detail / Repair / Stage / DispatchID / ClusterKey / EvidenceRefs / SuspectedRoot / IsDerived / RootKind`(**Detail 必填,无 Metadata 字段**) |

### 2.7 Block / Item struct(emit_answer_document V2 carrier)

| 文件 | 关键 line | 作用 |
|---|---|---|
| `internal/types/answer_document_v2.go:114` | `AnswerBlock` struct | 真实字段:`ID / Kind / Title / Text / Items / Diagram / ClaimUses / FacetIDs / SurfaceRole / EdgeAnchors`(SurfaceRole 在 block 级)|
| `internal/types/answer_document_v2.go:241` | `AnswerBlockItem` struct | 真实字段:`Kind / ID / Label / Text / CitationRef / ClaimUse`(item 字段名是 `Kind`,**非 ItemKind**)|
| `internal/types/answer_document_v2.go:276` | `AnswerBlockItemKind` enum | 3 真实值:`AnswerBlockItemKindPrincipal / AnswerBlockItemKindFlow / AnswerBlockItemKindCaveat` |
| `internal/types/answer_document_v2.go` (search `SurfaceRole`) | enum | `SurfaceRole` 4 值:`SurfacePrincipal / SurfaceSupport / SurfaceProseOnly / SurfaceDiagramOnly` |

### 2.8 emit_analysis tool schema(R2' 6-spot sync 起点)

| 文件 | 关键 line | 作用 |
|---|---|---|
| `internal/tool/emit_analysis.go:47-78` | `emitAnalysisParams` struct | LLM 可 emit 的字段映射 |
| `internal/tool/emit_analysis.go:169-309` | `buildEmitAnalysisSchema()` | JSON schema properties 定义 |
| `internal/tool/emit_analysis.go:541-568` | `Execute()` | 构造 RequestModel 实例 |
| `internal/tool/strict_decode_remap.go:79-109` | `RemapStrictDecodeError()` | strict-decode 错误重写(MisplacedFieldHint 表)|
| `internal/skill/analysis_contract.go` | enum 描述 | skill schema contract(新加 enum 类型时同步)|

### 2.9 测试影响面

| 文件 | 关键测试 | 影响 |
|---|---|---|
| `internal/types/facet_plan_test.go` | `TestResolveQuestionFamily*` | 加 `TestResolveEnumerationSubType_*`(至少 4 case)|
| `internal/types/answer_semantic_view_compile_enumeration_test.go` | 编译路径 | 加 sub-type 分支 case |
| `internal/types/answer_semantic_view_compile_family_test.go` | 21+ tests | 验证非 enumeration family 的 view.EnumerationSubType == EnumNone |
| `internal/analysis/amplifier/rule_r4_*_test.go` | 新建 | R4 rule unit test + axis_collapse / TermGraph trap fixture cross-check |
| `internal/orchestrator/contract_check_block_test.go` | 加 `TestValidateEnumerationAbstractionLevel_*` | 测新 validator + 跨 family skip 行为 |
| `internal/agent/answer_document_evaluator_test.go` | 加 sub-family render case | 测新 user-section 段 + nil-safety |
| `internal/analysis/hint/composer_test.go` | 加 `ViolEnumerationAbstractionLevelMismatch` case | hint 文本测试 |
| `internal/types/cgec_completeness_test.go` | 现有 | covered + kindSymbols 两 map 加 entry |
| `evals/qf_arch/...` + 新加 `evals/qf_mechanism/...` | x4 各 4 次 | 真 eval(commit 9)|

---

## 3. SHAPE_CONTRACT 数据流总览

```
┌──────────────────────────────────────────────────────────────────┐
│  ANALYZER LLM emit_analysis → RequestModel{                       │
│    Intent, Scenario, Complexity,                                  │
│    AnalyzerHints{ Entities, PrimaryEntities, ... },              │
│    Predicates{ IsCategoryEnumeration, IsRoleLocateLookup, ... }, │ ← 顶层
│    AnswerSubject{Kind, EntityAxes},                              │
│    PredicateAxis,                                                 │
│    SubTopics, EnumerationBoundary, Buckets, ...                  │
│  }                                                                │
└─────────────┬────────────────────────────────────────────────────┘
              │ analyzer.go:1317
              ▼
┌──────────────────────────────────────────────────────────────────┐
│  amplifier.Amplify(rm) [pre-compile rules — 顺序执行,后者读前者写]│
│    R1 multi-subject (rule_r1_multi_subject.go init())            │
│    R2 typed-name parity (rule_r2_typed_name_parity.go init())    │
│    ★ NEW R4 enumeration_subtype (rule_r4_*.go init())            │
│      Reads: rm.Intent, rm.Predicates.{IsCategoryEnumeration,     │
│             IsRoleLocateLookup}, rm.PredicateAxis,               │
│             rm.AnswerSubject.Kind                                 │
│      Writes: out.EnumerationSubTypeHint = ResolveEnumerationSubType(in)│
│      Skip: in.Intent != IntentEnumerate → return nil              │
│  Returns: (RequestModel, []Observation) — Observation by VALUE    │
└─────────────┬────────────────────────────────────────────────────┘
              │ analyzer.go:1404
              ▼
┌──────────────────────────────────────────────────────────────────┐
│  compiler.Compile(rm) → AnswerContract                            │
└─────────────┬────────────────────────────────────────────────────┘
              │ analyzer.go:1423
              ▼
┌──────────────────────────────────────────────────────────────────┐
│  amplifier.AmplifyPostCompile(rm, &contract) [post-compile rules] │
│    R3 must-include pinning (rule_r3_*.go init())                 │
└─────────────┬────────────────────────────────────────────────────┘
              │
              ▼
┌──────────────────────────────────────────────────────────────────┐
│  BuildAnswerSemanticView(ir, plan) → AnswerSemanticView           │
│    family = ResolveQuestionFamily(rm)                             │
│    switch family:                                                 │
│      case QFEnumeration:                                          │
│        view = compileEnumeration(ir, plan)                        │
│        ★ view.EnumerationSubType = ir.RequestModel.EnumerationSubTypeHint│
│        ★ view.RequiredBlocks[1].AcceptableClaimForms 按 sub-type 分支│
│        ★ view.UncertaintyRules += role-abstraction rule (if EnumRole)│
│      case <other 7 families>:                                     │
│        view = compile<family>(ir, plan)                           │
│        view.EnumerationSubType 默认 EnumNone (zero value)         │
└─────────────┬────────────────────────────────────────────────────┘
              │
              ├─→ FINALIZER: answer_document_evaluator.BuildInitialInstruction()
              │    user section additions (insert at line 170-176, after BlockContract):
              │    ┌──────────────────────────────────────────────┐
              │    │ ★ if view.EnumerationSubType == EnumRole:    │
              │    │     emit "## Enumeration sub-family: ROLE"  │
              │    │     emit role-shape teaching body            │
              │    │   if EnumMechanism:                         │
              │    │     emit "## Enumeration sub-family: MECHANISM"│
              │    │     emit mechanism-shape teaching body       │
              │    │   if EnumEntity:                            │
              │    │     emit "## Enumeration sub-family: ENTITY"│
              │    │     emit entity-shape teaching body          │
              │    │   if EnumNone OR view.Family != QFEnumeration:│
              │    │     return "" (no segment)                  │
              │    └──────────────────────────────────────────────┘
              │
              └─→ VALIDATOR: contract_check_block.runV2BlockOraclesWithMut()
                   ★ NEW SOFT validator: validateEnumerationAbstractionLevel(doc, view)
                     - Inserted at line 1690-1691 (between #5 validateFacetCoverage and #6 validateRichnessGlaringGap)
                     - Skip conditions (defense in depth):
                       1. doc == nil || view == nil → nil
                       2. view.Family != QFEnumeration → nil  ← family 闸
                       3. !viewNeedsExtractorBackedEnumerationSlate(view) → nil
                          ↑ 防跨 family 误伤(QFCallChain/QFRootCauseTrace 也有 ordered_list)
                       4. subType ∈ {EnumNone, EnumEntity} → nil  ← sub-type 闸
                     - Body: scan ordered_list principal items;
                       for EnumRole: fire if role verbs absent AND mech verbs dominate
                       for EnumMechanism: fire if mech verbs absent AND prose only
                     - Severity: SOFT (DefaultSeverity=SeveritySoft, Promotable=false)
                       ↑ 永远 SOFT — verb-substring 是 noisy 信号,不进 STRICT gate
                       ↑ 符合 R3 精确信号红线 (feedback_precise_signals_for_hard_gates.md)
                     - Output: Violation{Kind, Detail, Repair, Stage, ClusterKey, SuspectedRoot}
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
//     soft validators (validateEnumerationAbstractionLevel) read
//     this as a typed enum, not as a phrase parsed from prompts.
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
// EnumerationSubType values when ResolveQuestionFamily has already
// returned QFEnumeration. Returns EnumNone for every other family —
// callers MUST gate the call by family == QFEnumeration.
//
// Decision sources (PRECISE typed signals only — no question-text
// regex matching, per §9.5 red line; field paths verified against
// real code 2026-05-05):
//
//   - rm.Predicates.IsCategoryEnumeration (typed bool, RequestModel
//     top-level, NOT inside AnalyzerHints): true → EnumEntity.
//   - rm.Predicates.IsRoleLocateLookup (typed bool): true → EnumRole.
//     This is the strongest typed signal for role-shape questions
//     ("X 是什么角色 / X 负责什么").
//   - rm.PredicateAxis ∈ {AxisCall}: implies mechanism (caller→callee
//     chain); other Axis values are too generic to decide.
//   - rm.AnswerSubject.Kind ∈ {SubjectFunctionName, SubjectInterface,
//     SubjectStructField, SubjectHandlerRoute, SubjectConfigKey}:
//     entity-shaped subject → EnumEntity (when no role-lookup signal).
//
// Priority order (first match wins):
//
//  1. IsCategoryEnumeration=true → EnumEntity
//     (rationale: category lookup IS entity enumeration)
//  2. IsRoleLocateLookup=true → EnumRole
//     (rationale: typed bool from analyzer is the cleanest role signal)
//  3. PredicateAxis == AxisCall → EnumMechanism
//     (rationale: question explicitly about call edges)
//  4. AnswerSubject.Kind ∈ entity-shaped enum → EnumEntity
//     (rationale: subject is a named code entity)
//  5. fallthrough → EnumNone
//     (rationale: insufficient typed signal — fall back to generic
//     enumeration behavior; soft validator skips on EnumNone)
//
// EnumNone is the safe default: when sub-type cannot be determined,
// the system falls back to current behavior (one AcceptableClaimForms
// list, no abstraction-level validator firing). This is the
// fail-safe — never block a question that doesn't fit a sub-type.
func ResolveEnumerationSubType(rm RequestModel) EnumerationSubType {
    if rm.Predicates.IsCategoryEnumeration {
        return EnumEntity
    }
    if rm.Predicates.IsRoleLocateLookup {
        return EnumRole
    }
    if rm.PredicateAxis == AxisCall {
        return EnumMechanism
    }
    switch rm.AnswerSubject.Kind {
    case SubjectFunctionName, SubjectInterface, SubjectStructField,
         SubjectHandlerRoute, SubjectConfigKey:
        return EnumEntity
    }
    return EnumNone
}
```

**关键设计性质**:
- 不依赖任何不存在的 enum 值 — 只用真实存在的 `IsCategoryEnumeration / IsRoleLocateLookup`(顶层 Predicates bool)、`AxisCall`(PredicateAxis 真实值之一)、`SubjectFunctionName` 等 5 个真实 AnswerSubjectKind 值
- 用 `Predicates.IsRoleLocateLookup` typed bool 作 role-shape 主信号 — 这是 analyzer LLM 已经 emit 的字段,无需新加 enum
- **本 Phase 完全不动 PredicateAxis 8 真实值集合**(避免触动 R2' 6-spot sync)
- **本 Phase 完全不动 AnswerSubjectKind 13 真实值集合**(避免触动 enum + skill contract + completeness test)
- EnumNone 路径覆盖率自然较高 — 这是**特性不是 bug**(SOFT validator 默认跳过,不会误伤)

### 4.3 Amplifier R4 集成(选定方案 Y — 长期扩展性)

未来可预期的 sub-typing 需求(CallChain hop-chain vs callchain-enumeration、ConfigPrecedence yaml-vs-cli precedence 等)都按本模式扩展:加 1 个 amplifier rule 文件,不动 facet_plan.go god-file。

**实施细节**(基于真实 amplifier 框架):

新建 `internal/analysis/amplifier/rule_r4_enumeration_subtype.go`:

```go
package amplifier

import "github.com/.../internal/types"

// ruleR4EnumerationSubType — Phase C — sub-family typed signal for
// QFEnumeration questions. Reads typed RequestModel slots, writes
// rm.EnumerationSubTypeHint. Skip when the question is not an
// enumeration shape (cheap early exit).
//
// Red line compliance:
//   - feedback_axis_collapse_alignment.md: this rule does NOT write
//     IsCategoryEnumeration or SubTopics, so no axis_collapse
//     alignment gate is required. Verified by axis_collapse fixture.
//   - feedback_termgraph_vs_analyzerhints_entities.md: this rule does
//     NOT read TermGraph; it reads only typed Predicates / PredicateAxis /
//     AnswerSubject.Kind. Verified by trap fixture.
func ruleR4EnumerationSubType(in types.RequestModel, out *types.RequestModel) *Observation {
    if in.Intent != types.IntentEnumerate {
        return nil  // skip non-enumeration shapes
    }
    if in.EnumerationSubTypeHint != types.EnumNone {
        return nil  // do not overwrite existing hint (defensive)
    }
    subType := types.ResolveEnumerationSubType(in)
    if subType == types.EnumNone {
        return nil  // no actionable signal
    }
    out.EnumerationSubTypeHint = subType
    return &Observation{
        Rule:   "R4_enumeration_subtype",
        Field:  "EnumerationSubTypeHint",
        Before: string(in.EnumerationSubTypeHint),
        After:  string(subType),
        Reason: "deterministic sub-family typed signal from Predicates+PredicateAxis+AnswerSubject.Kind",
    }
}

func init() {
    preCompileRules = append(preCompileRules, ruleR4EnumerationSubType)
}
```

**关键 conformance 点**(必须复制 R1/R2/R3 模式):
- 函数签名 `func(in RequestModel, out *RequestModel) *Observation` — 与 `preCompileRule` type alias 一致(amplifier.go:75)
- `func init()` 调 `preCompileRules = append(preCompileRules, ruleR4EnumerationSubType)` — 与 R1/R2 注册方式一致(每 rule 文件自己注册,**不在 amplifier.go 集中改 slice**)
- `Observation` 5 字段全填(`Rule / Field / Before / After / Reason`)
- 早期 skip(`Intent != IntentEnumerate`)— 避免影响其他 family 的 LLM emit 路径
- 防御性"do not overwrite"(`EnumerationSubTypeHint != EnumNone`)— 与 R1/R2 红线 #2 一致(不覆盖 LLM 已填的字段)

### 4.4 RequestModel 加 EnumerationSubTypeHint 字段

加到 `internal/types/analysis_ir.go::RequestModel` struct(line 44-186 内,推荐紧邻 `PredicateAxis` 字段):

```go
type RequestModel struct {
    // ... 22 个现有字段 ...

    // EnumerationSubTypeHint — Phase C — deterministic sub-family
    // signal written by amplifier R4 rule. EnumNone (zero value)
    // when not applicable. Read by compileEnumeration (view-side)
    // and by validateEnumerationAbstractionLevel (validator-side).
    //
    // Not LLM-emitted: this field is exclusively system-derived in
    // amplifier post-processing. It does NOT appear in the
    // emit_analysis tool schema (LLM cannot write it).
    EnumerationSubTypeHint EnumerationSubType `json:"enumeration_subtype_hint,omitempty"`
}
```

**R2' 6-spot sync 实施清单(本字段为 system-derived,部分项不适用)**:

| # | sync 项 | 本字段适用? | 实施 |
|---|---|---|---|
| 1 | `internal/types/analysis_ir.go::RequestModel` struct | ✓ 必做 | 加字段定义(上面) |
| 2 | `internal/tool/emit_analysis.go::emitAnalysisParams` struct | ❌ 不适用 | 此字段非 LLM-emit |
| 3 | `internal/tool/emit_analysis.go::buildEmitAnalysisSchema()` properties + required | ❌ 不适用 | 同上 |
| 4 | `internal/tool/emit_analysis.go::Execute()` 构造 RequestModel | ❌ 不适用 | 同上 |
| 5 | `internal/tool/strict_decode_remap.go::MisplacedFieldHint` | ❌ 不适用 | 非 LLM 可写字段,无 misplacement 风险 |
| 6 | `internal/skill/analysis_contract.go` enum description | ❌ 不适用 | 非 LLM 可见 enum |

**实质只需 1 处改动**(RequestModel struct)。这是 deterministic post-LLM signal 的特点 — 不进 LLM emit 路径,6-spot sync 大部分自动跳过。

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
    // Driven by: amplifier R4 rule writes RequestModel.EnumerationSubTypeHint;
    //            compileEnumeration copies it to view.EnumerationSubType.
    // Consumed by:
    //   - renderAnswerDocSubFamilyRules (only emits sub-family teaching
    //     when sub-type ∈ {EnumRole, EnumMechanism, EnumEntity})
    //   - validateEnumerationAbstractionLevel (only fires when
    //     sub-type ∈ {EnumRole, EnumMechanism}; SOFT only)
    //
    // For non-enumeration families this field stays at zero value
    // (EnumNone) since their compile_*.go does not set it.
    EnumerationSubType EnumerationSubType

    FacetCoverage *FacetCoverageContract
    RequiredBlocks []BlockRequirement
    // ... rest unchanged
}
```

**测试更新**:`answer_semantic_view_compile_*_test.go` 中所有非 enumeration family 的 view 编译测试需要 assert `EnumerationSubType == EnumNone`(zero value)。其他 7 family 的 compile 函数**不需修改** — zero value 即正确语义。

---

## 6. compileEnumeration 内部 derive(不改外部签名)

`internal/types/answer_semantic_view_compile_enumeration.go:26-94` 改造:

```go
// compileEnumeration signature UNCHANGED — keeps cross-family dispatch
// at answer_semantic_view_compile.go line 72 untouched. Sub-type
// derivation is internal.
func compileEnumeration(ir *AnalysisIR, plan *AnswerSurfacePlan) *AnswerSemanticView {
    // Derive sub-type from RequestModel hint (written by amplifier R4).
    // EnumNone when hint is unset (legacy path / pre-R4 traces).
    var subType EnumerationSubType
    if ir != nil {
        subType = ir.RequestModel.EnumerationSubTypeHint
    }

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
            AcceptableClaimForms: enumerationClaimFormsForSubType(subType),
            Rationale:            ...,
            SurfaceRoleHint:      SurfacePrincipal,
        },
    }

    view.OptionalBlocks = []BlockRequirement{
        // ... existing OptionalBlocks unchanged
    }

    view.UncertaintyRules = enumerationUncertaintyRulesForSubType(subType)

    view.RichnessCandidates = richnessCandidatesFromOptionalFacets(view.FacetCoverage)
    return view
}

// enumerationClaimFormsForSubType returns the AcceptableClaimForms
// list for the principal ordered_list block, branched on sub-type.
//
// Cross-family compatibility: ClaimCallEdge / ClaimGuardCondition /
// ClaimReturnFact are already used in CallChain (compile_call_chain.go:44)
// and RootCauseTrace (compile_root_cause_trace.go:50) families. Adding
// them to enumeration's mechanism path is consistent with existing
// validator support — no new claim_form values introduced.
func enumerationClaimFormsForSubType(s EnumerationSubType) []ClaimForm {
    switch s {
    case EnumMechanism:
        return []ClaimForm{ClaimDefinitionFact, ClaimCallEdge, ClaimGuardCondition, ClaimReturnFact}
    case EnumRole, EnumEntity, EnumNone:
        // EnumRole: implementation chains belong in the rationale, not
        //   as standalone claim forms — keep DefinitionFact only.
        // EnumEntity: simple symbol slate uses DefinitionFact only.
        // EnumNone: legacy fallback — preserve current behavior.
        return []ClaimForm{ClaimDefinitionFact}
    }
    return []ClaimForm{ClaimDefinitionFact}
}

// enumerationUncertaintyRulesForSubType returns the UncertaintyRules
// list, with role-abstraction rule added for EnumRole. Other sub-types
// get only the existing boundary rule.
func enumerationUncertaintyRulesForSubType(s EnumerationSubType) []UncertaintyRule {
    base := UncertaintyRule{
        TriggerFacet:      string(FacetUncertaintyBoundary),
        ExpectedBlockKind: BlockCaveat,
        MissingMessage:    "Your enumeration's completeness is bounded ...",
    }
    if s != EnumRole {
        return []UncertaintyRule{base}
    }
    return []UncertaintyRule{
        base,
        {
            TriggerFacet:      string(FacetEnumerationItem),
            ExpectedBlockKind: BlockOrderedList,
            MissingMessage: "Items must answer at conceptual level (X is responsible for Y); pure call/guard chains without naming the responsibility are a regression.",
        },
    }
}
```

**关键设计变化(签名兼容)**:
- `compileEnumeration` 签名**保持** `(ir *AnalysisIR, plan *AnswerSurfacePlan) *AnswerSemanticView` — 顶层 dispatch (`answer_semantic_view_compile.go:72`) 不动
- 跨 family 影响 **零**(其他 7 个 family 的 compile 函数不动,view.EnumerationSubType 默认 EnumNone)
- `BuildAnswerSemanticView()` / `BuildAnswerSemanticViewForAgentContext()` / `BuildAnswerSemanticViewForBusContext()` 三个 wrapper 全不动

---

## 7. Finalizer dynamic user-section render

加 `internal/agent/answer_document_evaluator.go::renderAnswerDocSubFamilyRules`:

```go
// renderAnswerDocSubFamilyRules emits a per-dispatch user-section
// segment with sub-family-specific abstraction-level rules.
// Returns empty string when:
//   - ctx == nil OR ctx.AnalysisIR == nil
//   - view == nil OR view.Family != QFEnumeration
//   - view.EnumerationSubType == EnumNone
//
// The emitted segment carries the rule body that previously lived as
// rule #8 (A'') in the static answer-document-skill Workflow. By
// emitting it conditionally per dispatch, LLM attention is focused on
// the rule that applies to THIS question, instead of being diluted
// across 24 always-on rules.
//
// nil-safety pattern follows renderAnswerDocFacetCoverage (line 1001):
// three-layer check (ctx, ctx.AnalysisIR, view) before reading view fields.
func renderAnswerDocSubFamilyRules(ctx *types.AgentContext, view *types.AnswerSemanticView) string {
    if ctx == nil || ctx.AnalysisIR == nil {
        return ""
    }
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
    case types.EnumMechanism:
        b.WriteString("## Enumeration sub-family: MECHANISM\n\n")
        b.WriteString("This question asks HOW each item works (call sequence, guard conditions, ")
        b.WriteString("return paths). Each ordered_list item's `text` SHOULD describe the call ")
        b.WriteString("chain explicitly — name the called functions, the guards, the data flow. ")
        b.WriteString("Conceptual responsibility descriptions alone (\"X handles input\") are ")
        b.WriteString("under-filled: the user asked for the mechanism. Use claim_form values from ")
        b.WriteString("{call_edge, guard_condition, return_fact} on each item.\n")
    case types.EnumEntity:
        b.WriteString("## Enumeration sub-family: ENTITY\n\n")
        b.WriteString("This question asks for the entities themselves (implementers, subclasses, ")
        b.WriteString("handlers, instances). Each ordered_list item's `label` is the entity name ")
        b.WriteString("(verbatim from evidence anchor_symbol); each item's `text` is a 1-line role ")
        b.WriteString("description disambiguating this entity from siblings. No abstraction-level ")
        b.WriteString("constraint applies.\n")
    case types.EnumNone:
        return ""
    default:
        return ""
    }
    return b.String()
}
```

**调用点**(精确插入位置 line 170-176,在 `BlockContract` 之后、Diagram 之前):

```go
// answer_document_evaluator.go around line 175 (immediately after the
// existing renderAnswerDocBlockContract block at line 167-176)
if subFamily := renderAnswerDocSubFamilyRules(ctx, view); subFamily != "" {
    b.WriteString(subFamily)
    b.WriteString("\n")
}
```

**LLM-facing 文本红线 check**(`feedback_no_internal_info_in_llm_prompts.md`):
- 不出现 `EnumRole / EnumMechanism / EnumEntity / EnumNone` 这种内部 enum 名 — 用自然语言 "ROLE / MECHANISM / ENTITY"
- 不出现 `view / amplifier / R4 / EnumerationSubType` 这种系统术语
- 不引用 `validateEnumerationAbstractionLevel` 这种内部 validator 名

---

## 8. New validator + ViolKind(SOFT only)

### 8.1 新 ViolationKind 注册

`internal/types/violation_registry.go` 加(参考已有 `ViolPrincipalClaimUseMissing` 的真实 11 字段 spec):

```go
// const block (放在已有 ViolKind const 群中):
const ViolEnumerationAbstractionLevelMismatch ViolationKind = "enumeration_abstraction_level_mismatch"

// In default profile registry block (RegisterViolKind 调用群中):
RegisterViolKind(ViolKindSpec{
    Kind:            types.ViolEnumerationAbstractionLevelMismatch,
    DefaultSeverity: types.SeveritySoft,           // 永远 SOFT
    SoftByDefault:   true,
    Promotable:      false,                        // 不允许 yaml 升级到 STRICT
                                                   // — verb-substring 是 noisy 信号
                                                   // 严禁进 STRICT gate (R3 红线)
    FallbackLocus:   types.LocusFinalizer,
    Layer:           "v2_oracle",
    Description:     "enumeration item.text shape (role-prose vs call-chain-prose) doesn't match the resolved EnumerationSubType",
    FixableByAgents: []types.AgentName{types.AgentFinalizer},
    // SchemaDescriptionFragment / Implies / CaveatFamilyID 留 zero value
})
```

`internal/types/cgec_completeness_test.go` **两处**加 entry:

1. `covered` map(line 187-271 内,接续添加):
```go
ViolEnumerationAbstractionLevelMismatch: true, // contract_check_block.go validateEnumerationAbstractionLevel
```

2. `kindSymbols` map(line 292-337 内,接续添加):
```go
ViolEnumerationAbstractionLevelMismatch: "EnumerationAbstractionLevelMismatch",
```

**关键设计性质**:`Promotable=false` 是硬约束 — 即使运维误改 yaml 加 `pipeline_contract_strict_kinds: [enumeration_abstraction_level_mismatch]`,RegisterViolKind 也会拒绝 promotion(检查 spec.Promotable)。这是 R3 红线的 schema-level enforcement。

### 8.2 新 validator 实现(SOFT, defense-in-depth skip)

加 `internal/orchestrator/contract_check_block.go::validateEnumerationAbstractionLevel`(放在 `validateRichnessGlaringGap` 前,即 line 1697 前):

```go
// validateEnumerationAbstractionLevel (Phase C, <date>) — SOFT validator
// that fires when an enumeration's principal items use the wrong prose
// shape for the resolved EnumerationSubType. Driven by typed sub-type
// signal (precise) and verb-substring counting (noisy). Severity is
// HARDCODED SOFT (Promotable=false) per R3 red line — noisy signals
// MUST NOT drive hard gates.
//
// Skip conditions (defense in depth — order matters):
//   1. doc == nil OR view == nil → nil
//   2. view.Family != QFEnumeration → nil
//      (top-level family gate)
//   3. !viewNeedsExtractorBackedEnumerationSlate(view) → nil
//      (defense against QFCallChain/QFRootCauseTrace/QFComparison
//       cases that legitimately have ordered_list/bullet_list blocks
//       but are NOT enumerations — see line 1968 helper)
//   4. view.EnumerationSubType ∈ {EnumNone, EnumEntity} → nil
//      (sub-type gate — only EnumRole/EnumMechanism enforce verb shape)
//   5. block.SurfaceRole != SurfacePrincipal → skip block
//   6. block.Kind != BlockOrderedList → skip block
//   7. item.Kind ∈ {AnswerBlockItemKindFlow, AnswerBlockItemKindCaveat}
//      → skip item (only principal items count)
//
// Severity: SOFT (DefaultSeverity=SeveritySoft, Promotable=false).
// Soft violations enter retry hint but do NOT trip gate.Passed=false.
// They are advisory — LLM may ignore on first emit, but next dispatch
// gets the hint as RetryHint user-section first segment.
//
// Verb sets are TYPED CONSTANTS in this file (see roleVerbs /
// mechanismVerbs); they are not regex'd from user prompts — per §9.5
// red line. Substring matching is noisy by nature, hence SOFT.
func validateEnumerationAbstractionLevel(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
    if doc == nil || view == nil {
        return nil
    }
    if view.Family != types.QFEnumeration {
        return nil
    }
    if !viewNeedsExtractorBackedEnumerationSlate(view) {
        return nil  // defense against non-enum families with list blocks
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
            if item.Kind == types.AnswerBlockItemKindFlow ||
                item.Kind == types.AnswerBlockItemKindCaveat {
                continue
            }
            mismatch := classifyAbstractionMismatch(item.Text, subType)
            if mismatch == "" {
                continue
            }
            out = append(out, types.Violation{
                Kind:   types.ViolEnumerationAbstractionLevelMismatch,
                Detail: fmt.Sprintf("block %q item[%d]: %s", block.ID, i, mismatch),
                Repair: fmt.Sprintf(
                    "Adjust block %q item[%d] prose shape: see ## Enumeration sub-family section in user prompt for the expected shape (sub-type=%s).",
                    block.ID, i, subType,
                ),
                Stage:      string(types.StageFinalize),
                ClusterKey: blockClusterKey(block.ID, "enumeration_abstraction"),
                SuspectedRoot: types.SuspectedRoot{
                    IRField:    "enumeration_abstraction_level",
                    Reason:     "item prose shape doesn't match sub-type expectation",
                    Confidence: 0.6,  // noisy signal — lower confidence
                },
            })
        }
    }
    return out
}

// roleVerbs are conceptual responsibility verbs that should appear in
// EnumRole item text. NOISY signal — substring match.
var roleVerbs = []string{
    "is responsible for", "handles", "owns", "manages", "represents",
    "encapsulates", "models", "tracks", "coordinates",
    "负责", "处理", "管理", "维护", "表示",
}

// mechanismVerbs are call-chain / control-flow verbs that should appear
// in EnumMechanism item text. NOISY signal — substring match.
var mechanismVerbs = []string{
    "calls", "invokes", "dispatches", "passes", "returns",
    "checks", "guards", "branches", "iterates",
    "调用", "派发", "返回", "判断", "分支",
}

// classifyAbstractionMismatch returns "" when item.text shape matches
// the expected sub-type, else returns a Repair string.
func classifyAbstractionMismatch(text string, subType types.EnumerationSubType) string {
    lower := strings.ToLower(text)
    roleHits := countAnyOf(lower, roleVerbs)
    mechHits := countAnyOf(lower, mechanismVerbs)

    switch subType {
    case types.EnumRole:
        if roleHits == 0 && mechHits > 0 {
            return "EnumRole expects role-shaped prose (X is responsible for Y); item dominated by mechanism verbs (calls/invokes/...). Add a responsibility clause, or restructure as 'X is responsible for <conceptual outcome>; the mechanism is <call chain>'."
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

**WIRE INTO** `runV2BlockOraclesWithMut`(`contract_check_block.go:1681-1706`)— 在 `validateFacetCoverage` (#5) 后、`validateRichnessGlaringGap` (#6) 前插入(具体是 line 1690-1691 之间):

```go
out = append(out, validateFacetCoverage(doc, view)...)
out = append(out, validateEnumerationAbstractionLevel(doc, view)...)  // ★ NEW
out = append(out, validateRichnessGlaringGap(doc, view)...)
```

### 8.3 Hint composer 新 case

`internal/analysis/hint/composer.go::summariseExactFix` 真实当前有 15 个 case label(line 374-441;awk 限定函数体内 grep `case` 数到 15)。本 Phase 加新 case(在 `ViolMissingRequestedRoleUndisclosed` 后、`return "Address..."` default 前;具体序号取决于 Phase A 是否先 ship 增加 case):

```go
case types.ViolEnumerationAbstractionLevelMismatch:
    return "Adjust the named ordered_list item(s) so the prose shape matches the question's enumeration sub-type. The ## Enumeration sub-family section in the user prompt names whether the question is ROLE (items describe what X is responsible for, conceptually), MECHANISM (items describe how X works — call chain, guards), or ENTITY (items name the entity + 1-line role). The validator's Detail line names the offending block id + item index + the expected shape. This is a SOFT advisory — fix on next emit if it improves the answer."
```

### 8.4 Cooccurrence rule(可选,初版不做)

如果 baseline eval 显示 `ViolEnumerationAbstractionLevelMismatch` 经常与 `ViolEnumerationLabelUngrounded` 共发,可在 `internal/orchestrator/repair_cooccurrence.go::defaultCooccurrenceRules` 加 1 条:

```go
{
    Primary: types.ViolEnumerationLabelUngrounded,  // 精确信号 (在 evidence 找不到 label)
    Derived: []types.ViolationKind{types.ViolEnumerationAbstractionLevelMismatch},
    Reason:  "label fabrication often pairs with abstraction shift; the precise label-grounding violation is the actionable root cause, the noisy abstraction signal is symptomatic",
},
```

**Caveat**: 这条 cooccurrence 让 noisy signal(AbstractionLevel)被 precise signal(LabelUngrounded)的 Primary 覆盖,符合 R3 红线方向。但只在数据显示共发率高时才加,初版 ship 时**不加**,留观察。

---

## 9. Commit 序列(单 session 7-10 commits)

每条 commit 必须独立可 revert。Eval 在 commit 9 之后整批跑。

### Commit 1 — Type definitions only(无逻辑)

- 加 `internal/types/facet_plan.go`:`EnumerationSubType` enum + `AllEnumerationSubTypes()` + `IsValid()`
- 加 `internal/types/answer_semantic_view.go`:`AnswerSemanticView.EnumerationSubType` 字段
- 加 unit test `TestEnumerationSubType_*` 覆盖 enum + IsValid
- 改 `internal/types/answer_semantic_view_compile_*_test.go`(8 family 测试)加 `EnumerationSubType == EnumNone` assert(non-enum families)
- **Eval**: `go test ./internal/types/...` 全绿。无真 eval(类型层无行为变化)。

### Commit 2 — RequestModel 加 EnumerationSubTypeHint 字段

- 加 `internal/types/analysis_ir.go::RequestModel.EnumerationSubTypeHint EnumerationSubType` 字段
- 验证 R2' 6-spot sync 清单(本字段 5/6 项不适用,只动 struct,见 §4.4 表)
- 加 unit test 覆盖 struct round-trip
- **Eval**: `go test ./internal/types/...` + `go test ./internal/tool/...` 全绿。无真 eval。

### Commit 3 — ResolveEnumerationSubType 纯函数

- 加 `internal/types/facet_plan.go::ResolveEnumerationSubType`(纯函数,无 LLM call)
- 加 unit test `TestResolveEnumerationSubType_*`(至少 5 case:role / mechanism / entity / none / 空 RequestModel)
- 加 helper `enumerationClaimFormsForSubType` + `enumerationUncertaintyRulesForSubType`(放 `answer_semantic_view_compile_enumeration.go` 末尾)
- 改 `compileEnumeration` 内部 derive sub-type + 调用两 helper(签名不变)
- 加 unit test 覆盖 4 sub-type × claim_forms 矩阵
- **Eval**: `go test ./internal/types/...` 全绿。无真 eval。

### Commit 4 — Amplifier R4 rule

- 加 `internal/analysis/amplifier/rule_r4_enumeration_subtype.go`(完整文件如 §4.3 草案)
- 用 `func init()` append 到 `preCompileRules` slice(模仿 R1/R2)
- 加 `internal/analysis/amplifier/rule_r4_enumeration_subtype_test.go`:
  - 至少 5 case:role / mechanism / entity / none / non-enumeration intent(skip)
  - **必过** `axis_collapse_fixture_test.go` cross-check — 跑 3 个现有 axis_collapse fixture 验证 R4 firing 后 rm 不进入 axis_collapse 触发态
  - **必过** `trap_fixture_test.go` cross-check — R4 不读 TermGraph(只读 Predicates / PredicateAxis / AnswerSubject.Kind)
- **Eval**: `go test ./internal/analysis/amplifier/...` 全绿(含 trap 两 fixture)。无真 eval。

> **红线 reminder**:本 commit 是方案 Y 的代价交付点。R1/R2 在 commit `753c348` / `3fa8d4f` 都因为 axis_collapse / TermGraph 两 trap 经过 fix iteration,本 commit 必须一次过。

### Commit 5 — Finalizer dynamic user-section render

- 加 `internal/agent/answer_document_evaluator.go::renderAnswerDocSubFamilyRules`(完整函数如 §7 草案)
- 在 `BuildInitialInstruction` line 170-176 之间(`renderAnswerDocBlockContract` 之后)插入调用
- 加 `internal/agent/answer_document_evaluator_test.go::TestRenderSubFamilyRules_*`(4 sub-type case + nil-safety 3 case)
- 验证 LLM-facing 文本不含内部 jargon(grep `EnumRole / amplifier / R4 / EnumerationSubType` 应 0 命中)
- **Eval**: `go test ./internal/agent/...` 全绿。**真 eval qf_arch x4**(验证 prompt 多了一段不引入回归)。

### Commit 6 — 删除 static Workflow 中的 A'' 规则(commit 5 已动态接管)

- 删 `internal/skill/defaults.go:126`(规则 #8 abstraction-level matching)
- 改 `internal/skill/defaults_test.go` 对应 substring assert:从 positive(must-contain)改为 negative(must-NOT-contain in static skill)
- **Eval**: 真 eval qf_arch x4 = 4/4(本 Phase 主 bar)。

### Commit 7 — New ViolKind registration(无 validator 行为)

- 加 `internal/types/violation_registry.go`:`ViolEnumerationAbstractionLevelMismatch` const + `ViolKindSpec` 注册块(11 字段如 §8.1 草案,**Promotable=false**)
- 加 `internal/types/cgec_completeness_test.go` covered + kindSymbols 两 map entry
- 加 `ViolationKind` 字符串测试
- **Eval**: `go test ./internal/types/...` 全绿。无行为改动。

### Commit 8 — New validator + wire into runV2BlockOraclesWithMut

- 加 `internal/orchestrator/contract_check_block.go::validateEnumerationAbstractionLevel` + 2 verb sets + 2 helper(完整代码如 §8.2 草案)
- 加 `internal/orchestrator/contract_check_block_test.go::TestValidateEnumerationAbstractionLevel_*`(至少 9 case:role-correct / role-wrong / mech-correct / mech-wrong / entity-skip / none-skip / non-enumeration-family-skip / call-chain-with-list-skip / nil-safety)
- Wire into `runV2BlockOraclesWithMut`(line 1690-1691 之间,`validateFacetCoverage` 后、`validateRichnessGlaringGap` 前)
- **Eval**: `go test ./internal/orchestrator/...` 全绿。真 eval qf_arch x4 + m1a x4 = 8(SOFT 默认,无回归)。

### Commit 9 — Hint composer case

- 加 `internal/analysis/hint/composer.go::summariseExactFix` 新 case(在 `ViolMissingRequestedRoleUndisclosed` 后、default 前;baseline 已有 15 case,Phase A 若先 ship 还会增加,本 Phase 在最终末尾追加即可)
- 加 `internal/analysis/hint/composer_test.go` 对应 case 测试(substring assert 锁住关键文本)
- **Eval**: composer test 全绿。无真 eval。

### Commit 10 — 真 eval 验收 + ship 收尾

跑全 case x4(20+ 次):

- qf_arch x4 = 4/4(主 bar)
- 加新 case `evals/qf_mechanism/`(若不存在,本 commit 顺手新增 spec — questions like "explorer 怎么走 evidence pipeline?" mechanism shape)
- m1a x4 = 4/4(不回归)
- s1a / u3a / qf_config_precedence x4 不回归

任一 case 回归 → 该 commit 链上的相关 commit revert,SOFT validator 留 violation 但不 fail loud。

收尾:
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

- 🔴 `feedback_precise_signals_for_hard_gates.md` — `validateEnumerationAbstractionLevel` 用 verb-substring(noisy)。**严格遵守**:`Promotable=false` 永远 SOFT,**不进 STRICT gate**。
- 🔴 `feedback_axis_collapse_alignment.md` — Phase C 选方案 Y(amplifier R4 rule),commit 4 BLOCKING:必过 `axis_collapse_fixture_test.go`。R4 不写 IsCategoryEnumeration / SubTopics,自然不触动 axis_collapse — 但 fixture cross-check 必跑。
- 🔴 `feedback_termgraph_vs_analyzerhints_entities.md` — R4 rule 必从 AnalyzerHints / Predicates / PredicateAxis / AnswerSubject 读,**禁从 TermGraph 读**;commit 4 BLOCKING:必过 `trap_fixture_test.go`。
- 🔴 `feedback_typed_signal_six_spot_sync.md` — `EnumerationSubTypeHint` 是 system-derived signal,6-处 sync 中 5 处不适用(只改 RequestModel struct)。在 commit 2 的 PR 描述中显式说明哪 5 处不需要,避免 reviewer 误判。
- 🔴 `feedback_no_eval_bar_relaxation.md` — 任何 case FAIL 不调 case spec,只回退 commit。
- 🔴 `feedback_no_internal_info_in_llm_prompts.md` — `renderAnswerDocSubFamilyRules` 输出文本不能露 system 内部术语(`EnumRole / amplifier / R4 / EnumerationSubType / view` 都不要写),改成自然语言"role-shape question"。commit 5 grep 验证。
- 🔴 `feedback_root_cause_only.md` — Phase A jitter 没修干净不许甩锅给"prompt 重组",必须找根因(LLM 注意力分散 → 本 Phase typed signal + dynamic render 是根因解)。
- 🔴 `feedback_prompt_redline_checklist.md` — `renderAnswerDocSubFamilyRules` 文本必过 ATOMIC 7 条 R3+R4+R5+R6+R7+SST+R2' checklist。
- 🔴 `feedback_no_dismiss_as_llm_flake.md` — 真 eval 任何 outlier 都查根因。

---

## 12. 失败回退路径

如 commit 10 真 eval 出现:

- **qf_mechanism x4 回归(新 case 在 EnumMechanism 路径有问题)**:回退 commit 8 + 9,SOFT validator 保留但不引入新 case;或直接调小 mechanismVerbs 集合
- **m1a 回归(EnumNone 路径误归类)**:`ResolveEnumerationSubType` 返回非 EnumNone 但实际是 entity 题型 → grep m1a 题目 + RequestModel,补 `ResolveEnumerationSubType` 的 priority 规则(可能要加 IsCountQuestion 检查 → EnumEntity)
- **qf_arch 仍 3/4**:本 Phase 失败 — 说明 typed signal 也没传到位。检查 `view.EnumerationSubType` 在 finalizer dispatch 时是否真为 EnumRole(打 log + grep dispatch trace + 验证 R4 firing 进入 reconcileEvent)

---

## 13. 跨 family 影响分析(本 Phase 只优化、不恶化的设计保证)

Phase C 的所有改动按"**影响隔离 + 默认 EnumNone**"原则设计,对其他 7 个 family 行为**零影响**。下表是逐字段验证:

| Phase C 改动 | QFRootCauseTrace | QFConfigPrecedence | QFRoleLookup | QFCallChain | QFArchitecture | QFComparison | QFGeneric |
|---|---|---|---|---|---|---|---|
| `RequestModel.EnumerationSubTypeHint` 字段 | 字段存在但 R4 skip(Intent != IntentEnumerate),保持 EnumNone | 同左 | 同左 | 同左 | 同左 | 同左 | 同左 |
| `AnswerSemanticView.EnumerationSubType` 字段 | view 默认 EnumNone(zero value),`compile<family>` 不写 | 同左 | 同左 | 同左 | 同左 | 同左 | 同左 |
| `compileEnumeration` 内部 derive | 不调用此函数,无影响 | 同左 | 同左 | 同左 | 同左 | 同左 | 同左 |
| `enumerationClaimFormsForSubType` helper | 不调用 | 同左 | 同左 | 同左 | 同左 | 同左 | 同左 |
| `renderAnswerDocSubFamilyRules` 调用 | view.Family != QFEnumeration → 返回 ""(无 user-section 段)| 同左 | 同左 | 同左 | 同左 | 同左 | 同左 |
| `validateEnumerationAbstractionLevel` 调用 | view.Family != QFEnumeration → return nil(无 violation)| 同左 | 同左 | 同左 | 同左 | 同左 | 同左 |

**关键防御性 skip(commit 8 必含)**:
- validator 第二闸用 `viewNeedsExtractorBackedEnumerationSlate(view)` 而非裸 `view.Family == QFEnumeration`,因为 QFCallChain / QFRootCauseTrace / QFComparison 也可有 ordered_list/bullet_list block — 仅靠 family 闸不足以防误伤
- `viewNeedsExtractorBackedEnumerationSlate` 在 `contract_check_block.go:1968` 已就绪,Phase C 复用即可

**新加的 ClaimReturnFact 在 mechanism 路径**:`ClaimReturnFact` 是 ClaimForm 9 个真实值之一,已在 `compile_call_chain.go` / `compile_root_cause_trace.go` 用过。本 Phase 在 enumeration mechanism 路径加它 — `validateClaimFormSupport` 已支持此 form,不需新加 validator 改动。

**测试矩阵保证**:
- commit 1 加 8 family 的 `EnumerationSubType == EnumNone` assert(non-enum)
- commit 8 加 cross-family-skip case(`call-chain-with-list-skip` 等)验证 validator 不误伤
- commit 10 真 eval m1a / s1a / u3a / qf_config_precedence 全 x4 验证业务路径不回归

---

## 14. Phase C 不做什么(scope discipline)

- ❌ 不动 PredicateAxis enum(8 真实值不增不减)
- ❌ 不动 AnswerSubjectKind enum(13 真实值不增不减)— 不引入 SubjectRoleNoun
- ❌ 不动 SemanticPredicates 7 bool 字段
- ❌ 不动 hint composer 现有 case(只在末尾追加新 case;baseline 15 case,Phase A 若先 ship 还会变动)
- ❌ 不动 cooccurrence 9 条(可选加 1 条,初版不加)
- ❌ 不引入新 QuestionFamily(EnumerationSubType 是 sub-type,不上升到 top-level)
- ❌ 不动 amplifier R1/R2/R3(本 Phase 沿用方案 Y 加 R4,不改前 3 条)
- ❌ 不动 OutputFormat(它是 schema 教学,与 sub-family 正交)
- ❌ 不重构 `BuildInitialInstruction` 主流程,只插入 1 个新 render 调用
- ❌ 不改 `compileEnumeration` 外部签名(内部 derive,签名兼容)
- ❌ 不让 `ViolEnumerationAbstractionLevelMismatch` 进 STRICT gate(`Promotable=false` 硬约束)
- ❌ 不改 `runV2BlockOraclesWithMut` 现有 13 validator 顺序(只插入 1 个,在 #5 与 #6 之间)

---

## 15. 真实代码事实速查表(实施时验证用)

下面是本 Phase 涉及的所有真实 code 信号,实施前必 grep 一次确认未漂移:

```bash
# RequestModel 22 字段定义
grep -n "type RequestModel struct" internal/types/analysis_ir.go        # 应在 line 44

# SemanticPredicates 7 bool 字段(顶层 RequestModel.Predicates,非 AnalyzerHints 内)
grep -n "type SemanticPredicates struct" internal/types/analysis_ir.go  # line 344
grep -n "IsCategoryEnumeration\|IsRoleLocateLookup\|IsCountQuestion\|IsCrossComponent\|IsRelationalLookup\|IsScalarAnswer\|IsHistoryLookup" internal/types/analysis_ir.go

# AnalyzerHints 10 字段(无 Verbs / 无 Predicates 嵌套)
grep -n "type AnalyzerHints struct" internal/types/analysis_ir.go       # line 398

# PredicateAxis 8 真实值
grep -n "AxisUnknown\|AxisCall\|AxisRegister\|AxisDefine\|AxisReturn\|AxisConfigure\|AxisCondition\|AxisImplement" internal/types/analysis_ir.go

# AnswerSubjectKind 13 真实值(无 SubjectRoleNoun)
grep -n "Subject[A-Z][a-z]" internal/types/analysis_ir.go

# AnswerBlock + AnswerBlockItem + AnswerBlockItemKind 字段
grep -n "type AnswerBlock struct\|type AnswerBlockItem struct\|AnswerBlockItemKind" internal/types/answer_document_v2.go

# Violation 10 真实字段(Detail 必填,无 Metadata)
grep -n "type Violation struct" internal/types/violation.go

# ViolKindSpec 11 真实字段
grep -n "type ViolKindSpec struct" internal/types/violation_registry.go

# Amplify 调用栈(R4 注册后自动 invoke)
grep -n "amplifier.Amplify\|amplifier.AmplifyPostCompile" internal/agent/analyzer.go

# preCompileRules / postCompileRules slice + Observation struct
grep -n "preCompileRules\|postCompileRules\|type Observation struct" internal/analysis/amplifier/amplifier.go

# 现有 R1/R2/R3 init() 注册模式(R4 必复制)
grep -n "func init()" internal/analysis/amplifier/rule_r1_multi_subject.go internal/analysis/amplifier/rule_r2_typed_name_parity.go internal/analysis/amplifier/rule_r3_must_include_pinning.go

# runV2BlockOraclesWithMut 13 validator 装配
grep -n "validateRequiredBlockCoverage\|validatePrincipalClaimUse\|validateDiagramEdgeSupport\|validateUncertaintyBlockPresence\|validateFacetCoverage\|validateRichnessGlaringGap\|validatePrincipalProseUnderfilled\|validateRichnessRegression\|validateClaimFormSupport\|validateMissingRequestedRoleDisclosure\|validateAbsenceScopeBound\|validateEnumerationItemLabelGrounding\|validateEnumerationItemLabelExtractorMatch" internal/orchestrator/contract_check_block.go

# viewNeedsExtractorBackedEnumerationSlate(防跨 family 误伤的 helper)
grep -n "viewNeedsExtractorBackedEnumerationSlate" internal/orchestrator/contract_check_block.go  # line 1968

# Hint composer 真实 case 数(限定 summariseExactFix 函数体)
awk '/^func summariseExactFix/,/^}/' internal/analysis/hint/composer.go | grep -cE "^[[:space:]]+case "  # baseline 应数到 15(Phase A 若先 ship 数会增加)

# cgec completeness map(新 ViolKind 必加两处)
grep -n "covered\s*:=\s*map\|kindSymbols\s*:=\s*map" internal/types/cgec_completeness_test.go

# BuildInitialInstruction 13 渲染调用(本 Phase 在 line 167 后插入)
grep -n "renderAnswerDoc[A-Z]" internal/agent/answer_document_evaluator.go
```

实施任何 commit 前,先把这些 grep 结果与 §2 各表对齐。如有漂移,本 Phase 设计需要修正,而不是猜测继续实施。

---

## 16. 验收 checklist(本 session 收口前必跑)

- [ ] commit 序列 1-10 全部 push 到 origin/main(`feedback_confirm_before_push.md`)
- [ ] qf_arch x4 = 4/4(本 Phase 主 bar)
- [ ] qf_mechanism x4 = 4/4(本 Phase 新 bar)
- [ ] m1a x4 = 4/4(不回归)
- [ ] s1a / u3a / qf_config_precedence x4 不回归
- [ ] `go test ./internal/types/...` 全绿(含 R2' 6-spot sync 验证)
- [ ] `go test ./internal/analysis/...` 全绿(amplifier 含 trap 两 fixture + hint composer)
- [ ] `go test ./internal/orchestrator/...` 全绿(新 SOFT validator + 跨 family skip)
- [ ] `go test ./internal/agent/...` 全绿(新 render + nil-safety)
- [ ] `go test ./internal/skill/...` 全绿(删 A'' 规则后 substring assert 已更新)
- [ ] `MEMORY.md` 顶部更新到本 Phase SHIPPED
- [ ] 写 `project_session_finalizer_phase_c_shipped.md` 收尾 doc
- [ ] 本设计文档 §0 Status 改 `SHIPPED <date>`
- [ ] grep 验证 LLM-facing 文本无内部 jargon(`EnumRole / amplifier / R4 / EnumerationSubType / view` 在 user-section 渲染中 0 命中)
- [ ] `pipeline_contract_strict_kinds: [enumeration_abstraction_level_mismatch]` yaml 设置应被 RegisterViolKind 拒绝(因 Promotable=false)— 加单元测试覆盖

---

## 附录 A — 数据流时序图(具体 case)

```
T=0  USER question:
       "Codrax orchestrator 的 4 个核心 stage 各负责什么?"
                     ↓
T=1  ANALYZER LLM emit_analysis (via emit_analysis tool):
       Intent: IntentEnumerate
       Predicates.IsCategoryEnumeration: false
       Predicates.IsRoleLocateLookup: true   ← typed signal!
       PredicateAxis: AxisUnknown            ← (没 LLM emit axis)
       AnswerSubject.Kind: SubjectInterface
       AnalyzerHints.Entities: ["analyze", "explore", "extract", "finalize"]
                     ↓
T=2  amplifier.Amplify(rm) [pre-compile]:
       R1 multi-subject: 4 entities → may flip IsCategoryEnumeration
                          (但本 case Predicates.IsCategoryEnumeration 已 false,
                           且 R1 alignment gate 可能 skip)
       R2 typed-name parity: derive SubTopics
       ★ R4 enumeration_subtype:
         in.Intent == IntentEnumerate ✓
         in.EnumerationSubTypeHint == EnumNone ✓
         ResolveEnumerationSubType(in):
           IsCategoryEnumeration=false → next
           IsRoleLocateLookup=true → return EnumRole
         out.EnumerationSubTypeHint = EnumRole
         emit Observation{Rule:"R4_enumeration_subtype", Field:"EnumerationSubTypeHint",
                          Before:"", After:"role", ...}
                     ↓
T=3  compiler.Compile(rm) → AnswerContract:
       MustInclude: ["analyze", "explore", "extract", "finalize"]
                     ↓
T=4  amplifier.AmplifyPostCompile(rm, &contract) [post-compile]:
       R3 must-include pinning fires
                     ↓
T=5  BuildAnswerSemanticView(ir, plan):
       family = ResolveQuestionFamily(rm) = QFEnumeration
       view = compileEnumeration(ir, plan)
         subType := ir.RequestModel.EnumerationSubTypeHint  // EnumRole
         view.Family = QFEnumeration
         view.EnumerationSubType = EnumRole
         view.RequiredBlocks[1].AcceptableClaimForms = [ClaimDefinitionFact]
                                                       (EnumRole branch)
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
         doc != nil ✓ view != nil ✓
         view.Family == QFEnumeration ✓
         viewNeedsExtractorBackedEnumerationSlate(view) == true ✓
         subType == EnumRole ✓
         Iterate principal ordered_list items:
           items[0]: roleHits=1 (负责), mechHits=0 → OK
           items[1]: roleHits=0, mechHits=1 (调用) → FIRE
             Violation{
               Kind: ViolEnumerationAbstractionLevelMismatch,
               Detail: "block 'enum-list-1' item[1]: EnumRole expects ...",
               Repair: "Adjust block 'enum-list-1' item[1] prose shape: see ## Enumeration sub-family section in user prompt for the expected shape (sub-type=role).",
               Stage: "finalize",
               ClusterKey: blockClusterKey("enum-list-1", "enumeration_abstraction"),
               SuspectedRoot: { IRField: "enumeration_abstraction_level", Confidence: 0.6 },
             }
                     ↓
T=9  hint.Composer.Compose:
       summariseExactFix → "Adjust the named ordered_list item(s) so the
                              prose shape matches the question's enumeration
                              sub-type. The ## Enumeration sub-family section
                              in the user prompt names whether ..."
       Render → user-section RetryHint
                     ↓
T=10 SOFT classification check:
       ViolKindSpec{ Kind: ViolEnumerationAbstractionLevelMismatch,
                     SoftByDefault: true, Promotable: false }
       hasAnyStrictViolation([viol]) == false (因 SoftByDefault)
       → result.Passed = true (SHIP) BUT retry hint 仍渲染给下次 dispatch
                     ↓
T=11 SHIP answer to user;
     Next dispatch (if any) sees retry hint as RetryHint user section first
     → LLM may revise items[1].text to:
       "explore 负责采集证据;它通过调用 read_file / grep 实现..."
       (now role-shaped + mechanism anchor — better answer)
```

---

## 附录 B — 与 Phase A 关系表

| 维度 | Phase A | Phase C |
|---|---|---|
| 触动文件数 | ~3 (skill/defaults.go, hint/composer.go, several test files) | ~10 (types/facet_plan.go, types/analysis_ir.go, types/answer_semantic_view*.go, amplifier/rule_r4_*.go, agent/answer_document_evaluator.go, orchestrator/contract_check_block.go, types/violation_registry.go, hint/composer.go, several test files, evals/qf_mechanism/) |
| 引入新 typed signal | 否(纯删) | 是(EnumerationSubType + EnumerationSubTypeHint + ViolKind) |
| 改 BlockRequirement | 否 | 是(AcceptableClaimForms 按 sub-type 分支) |
| 改 finalizer user section | 否 | 是(加 1 个 render 函数 + 1 个 BuildInitialInstruction 调用) |
| 改 validator 集合 | 否(行为不变) | 是(加第 14 个 V2 validator,SOFT only) |
| 改 hint composer | 是(增强 3 case) | 是(在末尾追加新 case;baseline 15 case)|
| 加 amplifier rule | 否 | 是(R4 — 走 init() 注册) |
| eval 风险 | 中(删规则可能让 retry 多 1 轮) | 中(typed 分类可能误判;new validator 默认 SOFT 不阻 ship) |
| 单 session 可控? | 是,5-8 commits | 是,7-10 commits |
| 治 A'' jitter 力度 | 中(注意力份额翻倍但仍依赖 LLM 自觉判断 trigger) | 高(typed signal + dynamic render + soft validator) |

**实施顺序硬性**:Phase A → eval 验证 → 决定是否需要 Phase C → Phase C。如 Phase A 已 4/4,Phase C 可作为 future work。

---

## 附录 C — Cross-reference

`MEMORY.md` 索引中本设计的位置:`docs/design/finalizer_phase_c_shape_contract.md`。Phase C 完成 ship 后,在 `MEMORY.md` 顶部增加:

```
- 🟢 [**finalizer Phase C — SHAPE_CONTRACT typed sub-family SHIPPED (<date>, N commits)**](project_session_finalizer_phase_c_shipped.md) — 加 EnumerationSubType typed enum + EnumerationSubTypeHint RequestModel field + amplifier R4 rule + sub-family-aware compileEnumeration(签名兼容)+ renderAnswerDocSubFamilyRules dynamic user-section + validateEnumerationAbstractionLevel SOFT V2 validator(Promotable=false) + ViolEnumerationAbstractionLevelMismatch ViolKind + composer case 15。qf_arch x4 = 4/4 + qf_mechanism x4 = 4/4。下一阶段:无(本议题收口)。
```

并加一条 cross-session red line:

```
- 🔴 [**typed sub-family signal 比 prompt 重组更治本**](feedback_typed_subfamily_for_attention.md) — Phase C SHIPPED 经验
```

---

## 附录 D — 实施 quirk 备注(2026-05-05 真实代码审计)

1. **PredicateAxis 真实 8 值**:`AxisUnknown / AxisCall / AxisRegister / AxisDefine / AxisReturn / AxisConfigure / AxisCondition / AxisImplement`(`internal/types/analysis_ir.go:285-296`)。本 Phase 完全不动这个 enum。
2. **AnswerSubjectKind 真实 13 值**:`SubjectUnknown / SubjectFunctionName / SubjectTypeName / SubjectHandlerRoute / SubjectConfigKey / SubjectReturnValue / SubjectFilePath / SubjectStringLiteral / SubjectNumeric / SubjectEnumValue / SubjectStructField / SubjectInterface / SubjectGeneric`(`internal/types/analysis_ir.go`,grep `SubjectFunctionName`)。无 `SubjectRoleNoun`。本 Phase 不动。
3. **SemanticPredicates 7 bool 字段是 RequestModel 顶层**:路径 `rm.Predicates.IsRoleLocateLookup`(**非** `rm.AnalyzerHints.Predicates.IsRoleLocateLookup`)。AnalyzerHints 不含 Predicates 嵌套。
4. **AnalyzerHints 真实 10 字段**:无 `Verbs` 字段。本 Phase 完全不依赖 Verbs。
5. **AnswerBlockItem 真实字段名是 `Kind`**(非 `ItemKind`),值域 `AnswerBlockItemKindPrincipal / AnswerBlockItemKindFlow / AnswerBlockItemKindCaveat`。SurfaceRole 在 block 级,值域 `SurfacePrincipal / SurfaceSupport / SurfaceProseOnly / SurfaceDiagramOnly`。
6. **Violation 真实 10 字段**:`Kind / Detail / Repair / Stage / DispatchID / ClusterKey / EvidenceRefs / SuspectedRoot / IsDerived / RootKind`。**Detail 必填,无 Metadata 字段**。
7. **ViolKindSpec 真实 11 字段**:`Kind / DefaultSeverity / SoftByDefault / Promotable / FallbackLocus / Layer / Description / FixableByAgents / SchemaDescriptionFragment / Implies / CaveatFamilyID`。`RegisterViolKind` panic 校验 `DefaultSeverity / FallbackLocus` 必填(violation_registry.go:301-308)。
8. **Hint composer 真实 15 case label**(在 `summariseExactFix` 函数体内 awk 限定 grep,数到 15;`grep -c "case types.Viol"` 全文 25 是因函数外仍有其他 switch 块):本 Phase 在末尾追加新 case(具体序号取决于 Phase A 是否先 ship 增加 case)。
9. **Amplifier preCompileRule 真实签名**:`func(in RequestModel, out *RequestModel) *Observation`(amplifier.go:75)。R4 必须用相同签名,通过 `func init()` append 到 `preCompileRules` slice(R1/R2 模式),**不要在 amplifier.go 集中改 slice**。
10. **Amplify 真实行为**:浅拷贝 rm,顺序跑 rule(后者读前者写),无 panic recovery。R4 不要 panic。
11. **真 eval 命令**:本仓库的 eval test 命名约定可能是 `TestE2E_<case>_x4` 而非 `Test<Case>_x4` — 实施前 grep `t.Run("` 确认。
12. **`viewNeedsExtractorBackedEnumerationSlate` 已就绪**(`contract_check_block.go:1968`)— 本 Phase validator 复用此函数防跨 family 误伤,**不要**裸用 `view.Family == QFEnumeration` 作 family 闸。
13. **R4 Observation 流向**:`amplifier.Amplify` 返回 `[]Observation`(by value),analyzer.go 通过 `recordReconcileObservation` 把每条 Observation 记到 telemetry。R4 firing 自动有 audit trail,无需新加 sink。
