# Analyzer IR Amplifier Layer — 设计文档

状态:Phase 0(待施工)
代码基线:`origin/main@9977e7d` 或更新(2026-05-05)
责任范围:为 analyzer LLM emit 后的 RequestModel 补全 LLM 漏填的可选 typed 字段。不替代任何执行层契约。

---

## 0. 快速进入

**一句话目标:** 给 `analyzer.go::buildAnalysisIR` 加一个新的 deterministic 后处理层,补 LLM 漏填的 `Predicates.IsCategoryEnumeration` / `SubTopics` / `Buckets` 等可选字段。

**新文件位置:** `internal/analysis/amplifier/`(新建包)

**接入位置:** `internal/agent/analyzer.go::buildAnalysisIR`,在所有现有 `reconcile*` 函数之前,在 `compiler.Compile` 之前。

**起点:** 跳到 §6 Phase 1.1。

**红线 6 条总览(细节看 §8):**
1. 永不读 `rm.RawRequest` 文本
2. 永不覆盖 LLM 已填非零字段
3. 不引用具体测试 case 字面 entity 名
4. 只读 typed slots(函数签名约束,无 ctx 参数)
5. Observation 输出兼容现有 telemetry
6. 是纯函数

---

## 1. 问题陈述

### 1.1 抽象描述

`AnalysisIR.RequestModel` 携带多组 typed 信号(`Buckets / SubTopics / Predicates / EnumerationBoundary / CompletenessObligation`),由 analyzer LLM 通过单次 `emit_analysis` 工具填写。这些信号是**下游执行层**(`AnswerSemanticView` 编译、`FacetBucketLabel HARD` 强制、enumeration 子契约等)做契约决策的唯一输入。

可选字段一旦在 LLM emit 时漏填,系统没有任何 deterministic 后处理把这些信号补回。当问题形态需要这些信号触发的契约,而 LLM 分类时遗漏,**整条契约链断裂**。

### 1.2 具体证据 — qf_arch 同 question 双跑对照

**问题:** "codrax 的 read-mode pipeline 由哪几个 stage 组成?每个 stage 大致负责什么?描述整体架构。"

| 维度 | r1(FAIL)| r2(PASS) |
|---|---|---|
| `predicates.is_category_enumeration` | **false** | **true** |
| `question_kind` | mechanism | enumeration |
| `entities` 中 Stage 命名数 | 0 | **6**(StageLogTriage..StageFinalize) |
| `Buckets` | 空 | 空 |
| `SubTopics` | 空 | 空 |
| StageAnalyze 角色描述(answer prose) | "**分发** analyzer agent" | "**负责分类**用户请求" |

**因果链(完全 structural cascade,无运气):**

```
analyzer LLM 随机选 mechanism (r1) vs enumeration (r2)
        ↓
predicates.IsCategoryEnumeration  false (r1) | true (r2)
        ↓
AnswerSemanticView 编译  无 enumeration 契约 (r1) | 有 (r2)
        ↓
finalizer prose 自由发挥 (r1) | 必出 per-stage role 描述 (r2)
        ↓
"分发 / 生成" (r1) | "分类" (r2)
        ↓
case regex FAIL (r1) | PASS (r2)
```

**每一步都是结构性传递,唯一随机点是 L0 — analyzer LLM 的分类。**

### 1.3 同根因姊妹 case

**m1a:** "explorer agent 和 extractor agent 如何协作?分别列出 Turn A 和 Turn B 产出的 emit_* 工具。"
- analyzer.predicates.is_category_enumeration = false(应为 true)
- 答案漏 `emit_evidence`(Turn A 的关键 emit 工具)
- case `EXPECT_CONTAINS=emit_evidence` ✗ FAIL

---

## 2. 系统中已有的相关组件

### 2.1 reconciler family — 已存在的 deterministic augmentation pipeline

`internal/agent/analyzer.go::buildAnalysisIR` 在 LLM emit 后顺序调用 7 个 reconciler 函数:

| 现有函数 | 文件 | 职责 |
|---|---|---|
| `reconcileEnumerationBoundaryScope` | `analyzer_boundary.go` | 验证 boundary 在 sub_topics 范围内 |
| `reconcileSemanticPredicates` | `analyzer_predicate.go` | 调 `IsCrossComponent` 的反向风险 |
| `reconcileComplexity` | `analyzer_complexity.go` | 复杂度交叉校验 |
| `reconcileIntent` | `analyzer_intent.go` | intent 与 predicates 一致性 |
| `inferAnswerSubject` | `analyzer_intent.go` | answer subject 推导(ADD-only)|
| `reconcilePredicateAxis` | `analyzer_predicate.go` | axis 推导(ADD-only)|
| `reconcileScenario` | `analyzer_intent.go` | scenario 与 intent 一致性 |

这是已建立的"deterministic augmentation"模式。所有调用都通过 `recordReconcileObservation` 统一观测。Amplifier 是这个模式的扩展,不是新层。

### 2.2 执行层契约(已健全)

| 字段 / 机制 | 文件 | 用途 |
|---|---|---|
| `RequestModel.Buckets[]` | `types/analysis_ir.go` | 命名分区 |
| `RequestModel.SubTopics[]` | `types/analysis_ir.go` | 多 sub-topic 独立 DAG 节点 |
| `RequestModel.EnumerationBoundary` | `types/analysis_ir.go` | 用户声明的边界数量 |
| `RequestModel.CompletenessObligation` | `types/analysis_ir.go` | "all/every" 强制覆盖 |
| `Predicates.IsCategoryEnumeration` | `types/predicates.go` | 触发 enumeration 子契约 |
| `FacetBucketLabel HARD per bucket` | `types/facet_plan.go` | `Buckets ≥ 2` 时每 bucket 必出现一条 facet |
| `view.Buckets ≥ 2` 触发 QFComparison | `types/answer_semantic_view_compile_comparison.go` | 多 bucket → 比较结构 |
| `HDP.topSymbols()` | `analysis/hdp/planner.go` | 已实现的 multi-subject 结构性检测 |

### 2.3 Gap

reconciler family 的 7 个函数中,**没有任何一个填写或调整以下字段:**

- `Predicates.IsCategoryEnumeration`(`reconcileSemanticPredicates` 只处理 `IsCrossComponent`)
- `SubTopics`(`buildAnalysisIR` 只 truncate 到 5 项,无派生)
- `Buckets`(无任何后处理)

**LLM emit 是这三个字段的唯一来源。** 这是 amplifier 要补的洞。

---

## 3. 必要的类型字典(下个 session 实施时直接对照)

### 3.1 `types.Intent`(`types/analysis_ir.go`)

```go
type Intent string

const (
    IntentExplain     Intent = "explain"
    IntentRootCause   Intent = "root_cause"
    IntentTrace       Intent = "trace"
    IntentEnumerate   Intent = "enumerate"
    IntentConfigQuery Intent = "config_query"
    IntentReturnValue Intent = "return_value"
    IntentUnknown     Intent = "unknown"
)
```

**没有 `IntentCompare` 这个值。** 比较类问题落到 `IntentExplain` 或 `IntentEnumerate`。

### 3.2 `types.TermKind`(`types/analysis_ir.go`)

```go
type TermKind string

const (
    TermSymbol  TermKind = "symbol"
    TermConcept TermKind = "concept"
    TermConfig  TermKind = "config"
    TermCommand TermKind = "command"
    TermLiteral TermKind = "literal"
)
```

`TermSymbol` / `TermConfig` / `TermLiteral` / `TermCommand` 是命名实体类(typed identifiers)。`TermConcept` 是抽象概念。

### 3.3 `types.AnswerSubject.Kind`(`types/analysis_ir.go`)

包含 `SubjectScalar`(scalar 答案)/ `SubjectConfigKey` / `SubjectGeneric` 等。完整列表查 `analysis_ir.go::AnswerSubjectKind`。

### 3.4 `types.SemanticPredicates`(`types/analysis_ir.go`)

```go
type SemanticPredicates struct {
    IsScalarAnswer        bool
    IsRoleLocateLookup    bool
    IsCountQuestion       bool
    IsCrossComponent      bool
    IsRelationalLookup    bool
    IsCategoryEnumeration bool   // ← amplifier R1 写入
    IsHistoryLookup       bool
}
```

### 3.5 `types.SubTopic`(`types/analysis_ir.go`)

```go
type SubTopic struct {
    Summary  string   `json:"summary"`
    Entities []string `json:"entities,omitempty"`
}
```

---

## 4. 设计

### 4.1 包结构

`internal/analysis/amplifier/`(新包)

```go
package amplifier

import "github.com/hanchaoqun/codrax/internal/types"

// Amplify augments rm with deterministic structural inferences
// derived from existing typed signals in rm itself. Pure function.
// Never overrides LLM-emitted fields; only fills empty optional
// fields when typed signals are sufficient.
//
// Returns the augmented rm and a list of observations describing
// each rule that fired. Caller threads observations into the
// existing recordReconcileObservation telemetry chain.
func Amplify(rm types.RequestModel) (types.RequestModel, []Observation)

// AmplifyPostCompile runs the rules that depend on out.AnswerContract
// being already populated by compiler.Compile. Mutates out's contract
// fields in place; returns observations.
func AmplifyPostCompile(out interface{}, rm types.RequestModel) []Observation
// (interface{} placeholder — actual signature uses *compiler.Output;
// avoid pulling compiler into amplifier — invert via accessor func
// or duplicate the small piece of contract API needed)

// Observation records one rule firing for telemetry parity with
// the existing reconciler family.
type Observation struct {
    Rule   string // identifier, e.g. "R1_multi_subject_predicate"
    Field  string // field that was modified, e.g. "predicates.is_category_enumeration"
    Before string // serialized prior value
    After  string // serialized new value
    Reason string // structural rationale
}
```

**避免循环依赖:** `internal/analysis/compiler/` 已经在 internal/analysis 树下,但为了 R3 的 post-compile pass 接入 `compiler.Output`,可以让 amplifier 反向接受一个最小接口:

```go
// MustIncludeMutator 是 AmplifyPostCompile 用的最小接口,避免直接 import compiler。
type MustIncludeMutator interface {
    AppendMustInclude(items []string)
    HasMustInclude(item string) bool
}
```

### 4.2 调用点

`internal/agent/analyzer.go::buildAnalysisIR` 函数内,文本锚定方式(不依赖具体行号):

**Amplify(pre-compile)插入位置 — Phase 2.1 修订:**
- AFTER `rm.TermGraph = normalizer.Normalize(...)` 赋值
- BEFORE `compiler.InferScenario(rm)` / `compiler.Compile(rm, sig)` 调用

定位提示:在文件中搜索 `rm.TermGraph = normalizer.Normalize(`,Amplify 调用紧跟在该 statement 之后。

**为什么不在 reconcile 链之前(原 v1 设计已废弃):** 原设计要求 Amplify 在 `reconcileSemanticPredicates` 之前调用,期望 R1 写入的 `IsCategoryEnumeration=true` 能 propagate 到下游 `reconcileComplexity` 触发 simple→moderate 升级。**实测发现** rm.TermGraph 由 `normalizer.Normalize` 在 reconcile 链之后才填充,在 reconcile 链之前调用 Amplify 时 TermGraph 永远是空的,R1 的 `topSymbolsLikeHDP` 永远返回 nil → R1 永不触发(2026-05-05 真 eval 验证:`reconcile-shadow` summary 无 amplifier 字段;m1a run-1 LLM emit 含 8 个 multi-symbol entities + intent=explain + IsCategoryEnumeration=false,完美 R1 触发条件,但 R1 并未 fire,因 TermGraph.Canonical 为 [])。

**新位置的 tradeoff:** R1 在 `reconcileComplexity` 之后跑,无法触发 simple→moderate 升级(此为 nice-to-have,非 load-bearing)。`compiler.Compile` / `compiler.InferScenario` 仍读到正确的 `IsCategoryEnumeration=true`,enumeration 模板选择不受影响,主链路完整。R3(post-compile MustInclude pinning,Phase 4)与 reconcile 链的解耦也由此 tradeoff 隐含覆盖 — R3 始终在 compiler 之后跑。

**AmplifyPostCompile(post-compile)插入位置:**
- AFTER `compiler.RecomputeBudget(&out, rm, sig)` 调用
- BEFORE `binder.BindByRelevance(...)` 调用

### 4.3 调用形式

```go
// pre-compile pass
amplified, observations := amplifier.Amplify(rm)
for _, obs := range observations {
    recordReconcileObservation(ctxMutable(ctx), reconcileEvent(
        obs.Field, obs.Before, obs.After, 0, obs.Reason, rm.Predicates,
    ))
}
rm = amplified

// ... compiler.Compile(rm, sig) ... compiler.RecomputeBudget ...

// post-compile pass
postObs := amplifier.AmplifyPostCompile(&out, rm)
for _, obs := range postObs {
    recordReconcileObservation(ctxMutable(ctx), reconcileEvent(
        obs.Field, obs.Before, obs.After, 0, obs.Reason, rm.Predicates,
    ))
}
```

### 4.4 不变性(实施时随时对照)

- amplifier **永不覆盖** LLM 已填的非零字段(`if existing != zero { skip }`)
- amplifier **永不读** `rm.RawRequest` 文本 — 函数签名禁止接受能 leak 文本的参数
- amplifier 规则**不允许引用具体测试 case 的字面 entity 名**
- amplifier 是**纯函数** — 无副作用,无 I/O,无并发原语,同输入同输出

---

## 5. 规则集

### R1 — Multi-subject Predicate Inference

**输入:** `rm.AnalyzerHints.Entities`, `rm.Predicates`, `rm.Intent`, 以及通过 `types.ExactResolutionTargets(rm)` 取出的 exact targets 列表。

**信号源选择(Phase 2.1-fix-2,2026-05-05):** 原设计读 `rm.TermGraph.Canonical` 里 `Kind==TermSymbol` 的 entry,沿用 `hdp.topSymbols` 的 confidence-gap 截断。**实测发现** TermGraph 由 `normalizer.Normalize` 从 `RawRequest` 文本里抽 surface,中文 / 自然语言问题(用户描述概念但不点名标识符)的 TermGraph.Canonical 永远没有 LLM 后续命名的 symbol entries,所以 R1 永不 fire。`rm.AnalyzerHints.Entities` 是 LLM 在 `emit_analysis` 里直接给出的命名实体清单 — 才是 multi-subject 的真正 typed 信号源。empirical evidence:2026-05-05 qf_arch run-2,LLM emit `entities=[StageAnalyze, StageExplore, StageExtract, StageFinalize]` + `intent=explain` + `is_category_enumeration=false`,完美 R1 触发条件,但旧 R1 无 fire(TermGraph.Canonical 里这 4 个名字一个都没有,都不在用户的中文 RawRequest 里出现)。

**触发条件(全部必须满足):**

1. `distinctEntityCount(rm.AnalyzerHints.Entities) ≥ 2`
   - 去掉空白/case-fold 后的 distinct count。重复同一名字 N 次仍计为 1 subject。
   - 不再使用 confidence-gap 截断 — LLM emit 的命名实体不带 rarity 噪音(那是 normalizer ranker 的问题)。
2. AND `rm.Predicates.IsCategoryEnumeration == false`(red line #2:不覆盖 LLM 已填正值)
3. AND `rm.Intent ∈ {IntentExplain, IntentTrace}`
   - 不包含 `IntentEnumerate`(已是 enumeration,不需要补)
   - 不包含 `IntentRootCause / IntentConfigQuery / IntentReturnValue / IntentUnknown`
4. AND NOT structural endpoint trace:若 `Intent=Trace`, `PredicateAxis∈{call,condition,register}` 或 `question_kind∈{call_chain,conditional,mechanism,registration}`,且 exact targets/mentioned targets ≥2,则这是 source→sink 链路,不是 category enumeration。
5. AND NOT `types.IsSingleTopicMechanismExplanation(rm)`:若 `Intent=Explain`, `question_kind∈{mechanism,conditional,registration}` 或 `PredicateAxis∈{condition,call,register}`,且没有 structural obligation / ambiguity / cross-component / 多 subtopic,则 multiple entities 只是同一机制的参与对象,不能升级成 principal-member enumeration。该 typed trait 同时被 `ResolveQuestionFamily` 复用,把这类问题送往轻量 `QFGeneric` 而不是 `QFArchitecture` / `QFRoleLookup`。
6. AND `len(types.ExactResolutionTargets(rm)) != 1`(单 exact target 不是 enumeration)
7. AND `rm.Predicates.IsScalarAnswer == false`(scalar 答案不是 enumeration;原文档写 `SubjectScalar` 但该枚举值不存在,`IsScalarAnswer` 是同等 typed 信号)
8. **AND NOT `(len(rm.SubTopics) >= 2 AND !rm.Predicates.IsCrossComponent)`**(Phase 3.2-fix axis_collapse alignment)
   - 当 LLM 已经 emit ≥2 SubTopics 在单组件问题上,翻转 IsCategoryEnumeration 到 true 会激活下游 axis_collapse gate(`internal/analysis/gate/coherence.go::R1.4`)的全部 4 个触发条件,导致 analyzer 进入 retry storm 直到 budget 耗尽,整个 Run 失败
   - 信任 LLM 的 SubTopics 结构判断;若 IsCrossComponent=true,SubTopics 合法跨子系统,axis_collapse 不会 trigger,R1 可以安全 fire
   - empirical:2026-05-05 m1a runs 全 IsCrossComponent=true,本 gate 不阻塞 R1 fire

**动作:**
- 设 `rm.Predicates.IsCategoryEnumeration = true`
- 记录 `Observation{Rule: "R1_multi_subject_predicate", Field: "predicates.is_category_enumeration", Before: "false", After: "true", Reason: "AnalyzerHints.Entities has N distinct named subjects with intent=X and non-scalar answer"}`

**理由:** `AnalyzerHints.Entities` 是 LLM 在 emit_analysis 里直接产出的"我识别到了哪些命名实体"清单(纯 typed slot,无 keyword 表/正则匹配)。R1 把这个已有结构信号 propagate 到 predicates 层。Intent 限定 + scalar 排除 + single-exact-target 排除 + structural endpoint trace guard + single-topic mechanism guard + axis_collapse 对齐确保不把机制参与对象误当答案成员,也不踩 retry storm。

**与下游 reconciler 的兼容(post Phase 2.1-fix wiring):**
- Amplify 在 reconcile 链之后跑(因 TermGraph 依赖,见 §4.2),所以 reconcileSemanticPredicates / reconcileComplexity 看不到 R1 写入的 `IsCategoryEnumeration=true`,无法触发 simple→moderate 升级。
- compiler.Compile / compiler.InferScenario 直接读到正确的 `IsCategoryEnumeration=true`,enumeration 模板选择不受影响,主链路完整。
- complexity 升级丢失为 nice-to-have 缺失,非 load-bearing。

### R2 — Typed-Name Parity → SubTopics Derivation

**输入:** `rm.AnalyzerHints.Entities`, `rm.Predicates`, `rm.SubTopics`

**信号源选择(Phase 3.1):** 同 R1 的 Phase 2.1-fix-2 教训(见 §5 R1 信号源段),R2 也读 `rm.AnalyzerHints.Entities` 而非 `rm.TermGraph.Canonical`。TermKind 过滤被 `isIdentifierLike()`(CamelCase / snake_case / dot-path 形态检查)替代,无需 normalizer 双 gate。

**触发条件:**

1. `rm.SubTopics` 为空(red line #2:不覆盖 LLM 已填 SubTopics)
2. **NOT `(rm.Predicates.IsCategoryEnumeration AND !rm.Predicates.IsCrossComponent)`**(Phase 3.2-fix axis_collapse alignment)
   - 单组件 enumeration 问题(`cat=true && !IsCrossComponent`)不应有 SubTopics — 应由 finalizer 用 ordered_list 渲染。
   - 此时若 R2 派生同 affix family 的 SubTopics,它们大概率落在同一 repomap domain → 激活 `axis_collapse` 全部 4 触发条件 → analyzer retry storm → eval FAIL。
   - empirical:2026-05-05 qf_arch run-1,LLM cat=true + !IsCrossComponent + 4 个 Stage* entity → R2(无该 gate)派生 4 个 SubTopics → axis_collapse → 4 retries 耗尽 → FAIL。
3. AND `distinctEntityCount(rm.AnalyzerHints.Entities) ≥ 2`(空 / 单 entity 没什么可分组)
4. AND `commonAffixGroups(rm.AnalyzerHints.Entities)` 至少返回 1 个 qualifying group:
   - 每组内 surfaces 共享 ≥ 4 字符公共前缀或后缀
   - 每组 entity 数 ≥ 2 且 ≤ 8
   - `isIdentifierLike()` 过滤掉 prose 词(全小写无分隔符如 "stage" / "agent")

**为什么是 4 字符 / 8 项:**

- **4 字符前缀阈值:** 太短(1-3 字符)误触发率高 — Go 名字 "Get" / "Set" / "io" 等 3 字符前缀广泛存在却不构成命名集。4 字符是经验下限,典型命名集前缀(`Stage*` 5 字符 / `emit_` 5 字符 / `Turn*` 4 字符)都 ≥ 4 字符。如实施后发现误触发,可调到 5。
- **8 项上限:** 单一命名集很少超过 8 项;超过 8 项往往是 codebase 全局符号(`Test*` / `Err*`)被误归一组,这类不构成用户 question 的 sub-topic 边界。8 是保守上限。

**动作:**
- 为每个 qualifying group 中的 EVERY entity 生成 `SubTopic{Summary: <Surface>, Entities: [<Surface>]}`
- 设 `rm.SubTopics = derived`
- 记录 `Observation{Rule: "R2_typed_name_parity_subtopics", Field: "sub_topics", Before: "0", After: "<N>", Reason: "AnalyzerHints.Entities has <K> affix-grouped families: <affix1>, <affix2>"}`
- buildAnalysisIR 中已有的 truncate-to-5 块 cap 总 SubTopics 数

**理由:** `AnalyzerHints.Entities` 是 LLM 直接 emit 的命名实体清单。affix grouping 是 string-slot 结构推断。`isIdentifierLike()` 把概念词排除。**axis_collapse 对齐 gate(条件 #2)是 Phase 3.2 真 eval forensic 后加上的硬性防护** — 没它 R2 在单组件 enumeration 上必然踩 retry storm,即使 unit test 全绿。

**与下游 reconciler 的兼容:**
- `reconcileComplexity` 后续看到 `len(rm.SubTopics) ≥ 1` → 升级 simple→moderate(buildAnalysisIR 已有逻辑;Amplify 在 reconcileComplexity 之后跑,所以这条 propagate 不到,但下游 compiler 看到 SubTopics 仍走多 subtopic 模板)
- `coherence.go::flattenSubTopicEntities` 自然读取派生的 SubTopics
- `coherence.go::R1.4 axis_collapse` — 有 R2 的 gate #2 兜底,不会被 R2 派生的同 domain SubTopics 激活

### R3 — Typed-Identifier MustInclude Pinning

**输入:** `types.CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm)`, `rm.AnalyzerHints.Entities`, `rm.Predicates`, `out.AnswerContract.MustInclude`

**关键时序约束:** R3 依赖 `compiler.Compile` 已生成 `out.AnswerContract`。R3 必须在 `AmplifyPostCompile` pass 内运行,不能在 pre-compile pass 内。

**调用顺序约束:** `Amplify(rm)` → `compiler.Compile(rm, sig)` → `AmplifyPostCompile(&out, rm)` → 后续 `binder.BindByRelevance` 等。

**触发条件:**

1. `types.CanUseAnalyzerEntitiesAsHardPrincipalMembers(rm) == true`(包含 R1 触发后的结果 — R1 必须先于 R3 跑)
2. AND `rm.AnalyzerHints.Entities` 中存在可作为 hard floor 的 code-identity surface。默认接受 identifier-like surface；当 `types.HasBoundedCategoryEnumerationMembers(rm)` 为真时，也接受 lowercase package/module/directory/namespace/path surface
3. AND 该 surface 不在 `out.AnswerContract.MustInclude` / `out.AnswerContract.MustIncludeTerms`

**动作:**
- 把符合条件的 typed surface append 进 `out.AnswerContract.MustInclude` / `MustIncludeTerms`(去重，并按 `ContractTermKind` 区分 symbol/tool_name/file_stem/user_phrase)
- 记录 `Observation{Rule: "R3_typed_identifier_mustinclude", Field: "answer_contract.must_include", ...}`

**理由:** 当问题被识别为一轴 principal-member enumeration AND analyzer 已确认 `AnalyzerHints.Entities` 是成员 lane 时，这些 surface 才是答案必含项。不依赖 question 文本的 keyword 匹配。关系型枚举(`is_relational_lookup=true`,例如“哪些 handler 调 X / 哪些模块 import Y / 哪些 agent 可以调用 Z”)不满足这个 trait：`AnalyzerHints.Entities` 在该形态下混有关系目标、工具、runtime helper、搜索 anchor 和候选成员，只能做软探索提示。真正的 principal member 覆盖应由探索/抽取后的 `AnswerSymbols`、support lane、step backbone 或 aggregate facts 承载，避免把上下文 helper 硬塞进最终答案。

### R4 — Buckets Derivation(推迟到 Phase 5)

R4 涉及 question 多 `?` 切分 / parallel-naming pattern 检测,需要更细致的边界设计。Phase 1-4 完成后,根据 eval 数据稳定度再考虑是否要做。**首版不做。**

---

## 6. 不在本设计内

- ❌ analyzer LLM prompt 调教(本设计走结构性补全路线,不动 prompt;prompt 调教是另一条独立路径)
- ❌ 添加新的 typed 字段到 `RequestModel`(沿用现有字段;若发现现有字段不够表达推论,**回到 §5 重审规则**,不擅自加字段)
- ❌ 修改 `AnswerSemanticView` 编译逻辑或契约执行层(执行层已健全)
- ❌ 修改 `emit_analysis` 工具的 schema 或 Parameters(LLM 视角对 amplifier 不可见)
- ❌ amplifier 读取 `rm.RawRequest` 文本做关键词/正则匹配(红线 1,函数签名物理屏障)
- ❌ amplifier 覆盖 LLM 已填的非零字段(红线 2)
- ❌ amplifier 为单个 case 设计字面规则(红线 3)

---

## 7. 实施分批

### Phase 1 — 框架与接入(2 commit)

**Phase 1.1**(commit 1)
- 新建 `internal/analysis/amplifier/` 包
- 实现 `Amplify(rm)` 入口骨架(规则注册表为空)
- 实现 `AmplifyPostCompile(out, rm)` 入口骨架(规则注册表为空)
- 实现 `Observation` 类型
- 单元测试 `amplifier_test.go`:
  - 空 rm 的恒等性(无规则触发 → 无变化)
  - 已满 rm 的恒等性(LLM 已填 → 不覆盖)
  - 多次调用幂等性(amplifier 第二次调用无变化)

**Phase 1.2**(commit 2)
- 接入 `internal/agent/analyzer.go::buildAnalysisIR`
  - 在 §4.2 描述的位置插入 `Amplify` 调用(在 `reconcileSemanticPredicates` 之前)
  - 在 `compiler.RecomputeBudget` 之后插入 `AmplifyPostCompile` 调用
- 把 observations 接入 `recordReconcileObservation`(用 `reconcileEvent` 包装)
- 测试 `TestBuildAnalysisIR_AmplifierInsertionOrder` — 钉死 amplifier 在所有 reconciler 之前调用
- 全部现有 analyzer / compiler / orchestrator 测试 0 回归

### Phase 2 — R1 规则(2 commit + 2 fix commit,2026-05-05)

**Phase 2.1**(commit 3 + 2 fix:`8dfd27b` + `7bf5ce5` 重定位 + `<TBD>` 信号源切换)
- 实现 `distinctEntityCount(entities []string) int` helper(去 blank,case-fold 后 distinct count)
- 实现 R1 规则,信号源是 `rm.AnalyzerHints.Entities`(non TermGraph,见 §5 R1 信号源选择段)
- 单元测试覆盖:
  - 多 entity(≥ 2 distinct)→ R1 触发,IsCategoryEnumeration=true
  - 单 entity → R1 不触发
  - 重复同名(case-fold + trim 后 collapse 到 1)→ R1 不触发
  - blank entries 被过滤掉,只剩 1 时 → R1 不触发
  - Intent ∉ {Explain, Trace, RootCause}(如 Enumerate / Lookup) → R1 不触发
  - IsScalarAnswer=true → R1 不触发
  - len(ExactResolutionTargets) == 1 → R1 不触发
  - LLM 已填 IsCategoryEnumeration=true → R1 不触发(单向 ADD)
  - empirical qf_arch shape(4 个 Stage* entity + intent=explain + 全 false predicates)→ R1 触发

**Phase 2.2**(commit 4)
- 真 eval:跑 qf_arch x4 + s1a x2 + m1a x2
- **验收:qf_arch 至少 3/4 PASS**(消除 r1/r2 随机性 — pre-amplifier 是 1/4 PASS)

### Phase 3 — R2 规则(2 commit)

**Phase 3.1**(commit 5)
- 实现 `commonAffixGroups(canonical []CanonicalTerm) [][]string` helper:
  - 按 Kind 分组
  - 每组内找 Surface 共享 ≥ 4 字符前缀或后缀的子集
  - 返回 group 数 ≥ 2 且 ≤ 8 的子集
- 实现 R2 规则
- 单元测试:
  - 6 个 `Stage*` TermSymbol → 1 组 → 6 个 SubTopic
  - 5 个混合 Kind(Stage* TermSymbol + 1 TermConcept)→ 只取 5 TermSymbol → 5 SubTopic
  - LLM 已填 SubTopics → R2 不触发
  - 共享 3 字符前缀(< 4)→ R2 不触发

**Phase 3.2**(commit 6)
- 真 eval:跑 m1a x4 + qf_arch x2
- **验收:m1a 4/4 包含 emit_evidence**(R2 派生 SubTopics + 下游契约强制每 subtopic 完整 emit_*)

### Phase 4 — R3 规则(2 commit)

**Phase 4.1**(commit 7)
- 实现 `AmplifyPostCompile` R3 分支
- 单元测试:
  - enumeration + named TermSymbol identifier → MustInclude 注入
  - enumeration + TermConcept(非命名实体)→ R3 不触发
  - non-enumeration → R3 不触发
  - identifier 已在 MustInclude → R3 跳过去重

**Phase 4.2**(commit 8)
- 真 eval 跨 5 case 验证
- **验收:enumeration 类问题命名 identifier 在答案中 100% 覆盖**

---

## 8. 验证指标

每完成一个 Phase 跑指定 case 子集:

| Phase | 必跑 case | 期望 |
|---|---|---|
| 1 | qf_arch x2 + s1a x2 | 0 回归(amplifier 还没规则触发) |
| 2 | qf_arch x4 + s1a x2 + m1a x2 | qf_arch 至少 3/4 PASS,m1a 至少 1/2 PASS |
| 3 | qf_arch x4 + s1a x2 + m1a x4 + u3a x2 | m1a 4/4 包含 emit_evidence;u3a 不回归 |
| 4 | 全 5 case x 2 runs | enumeration 类问题命名 identifier 全覆盖 |

**u3a 备注:** u3a 当前 case 已 stale(case spec 描述 extractor 是 one-shot,但代码已改为 iterationCap)。amplifier 不会让 u3a 通过 — case 本身需独立更新。"u3a 不回归" 指 u3a 已 FAIL 状态保持 FAIL,不应因 amplifier 引入新 FAIL 模式。

任何 Phase 引入回归 → 立即回滚,debug 后再推。

---

## 9. 红线 / 不变性

| # | 规则 | 强制方式 |
|---|---|---|
| 1 | amplifier 永不读 `rm.RawRequest` 文本 | 单元测试 + 代码 review |
| 2 | amplifier 永不覆盖 LLM 已填的非零字段 | 每条规则前置 `if existing != zero { skip }` |
| 3 | amplifier 规则不引用具体测试 case 字面 entity 名 | 代码 review;测试覆盖结构性输入 |
| 4 | amplifier 只读 typed slots(AnalyzerHints/Predicates;TermGraph 注意只是 raw-text surface 镜像)| 函数签名约束 + `feedback_termgraph_vs_analyzerhints_entities.md` + `trap_fixture_test.go` |
| 5 | amplifier 输出与 reconcileEvent telemetry 兼容 | Observation 类型对齐 reconcileEvent 字段 |
| 6 | amplifier 是纯函数 | 函数签名 + 单元测试(随机输入幂等性)|
| 7 | amplifier 不踩 axis_collapse trap | 每条 IsCategoryEnumeration / SubTopics writer 必加 axis_collapse alignment gate;`axis_collapse_fixture_test.go` 钉死 |

**最容易踩的是红线 2** — 看到 LLM 输出"差点意思"想覆盖回去。覆盖 = 退化为 reconciler family 已在做的"reconcile 已填字段",违反单一原则。

**最容易绕过的是红线 4** — 给 amplifier 加 `*MutableState` 参数就能拿运行时状态。一旦加,红线 1 失效。**函数签名是 6 条红线的物理屏障。**

**最容易隐蔽的是红线 7(2026-05-05 加)** — unit test 全绿但 runtime 触发 retry storm。axis_collapse(`internal/analysis/gate/coherence.go::R1.4`)在 4 个条件全满足时 reject:`nSub≥2 + !IsCrossComponent + (cat=true || Intent=Enumerate) + ≤1 distinct domain`。任何 amplifier 规则 flip cat=true 或 derive SubTopics 必须加对齐 gate;`axis_collapse_fixture_test.go` 是 forensic-driven regression 防线。

## 9.1 防护层一览(代码级 trap 防护体系)

amplifier 包内已部署的"后续开发者不再重踩"防护层:

| 层 | 文件 | 防护对象 |
|---|---|---|
| 1 | `package amplifier` doc(`amplifier.go` 顶部)| TermGraph trap + axis_collapse trap 的概念性警告 |
| 2 | `trap_fixture_test.go` | TermGraph 是空时新规则必须仍然 fire(R1/R2 验证)|
| 3 | `axis_collapse_fixture_test.go` | 任何 IsCategoryEnumeration writer / SubTopics writer 不能让 rm 进入 axis_collapse 触发态 |
| 4 | `internal/types/analysis_ir.go::TermGraph` 类型 doc | 警告 TermGraph 不是 LLM emit 镜像 |
| 5 | `internal/analysis/gate/coherence.go::R1.4` doc | 反向指明上游 writer 的合约 |
| 6 | `~/.claude/projects/.../memory/feedback_termgraph_vs_analyzerhints_entities.md` | 跨 session 红线条目(下次 Claude 自动加载)|
| 7 | 红线条目 #7(本表)| amplifier 规则作者必读 |

---

## 10. 接续 checklist

下个 session 接续按以下顺序确认进度:

1. `git log origin/main` 确认相关 commit
2. `ls internal/analysis/amplifier/` 看 Phase 1.1 是否落地
3. `grep -n amplifier.Amplify internal/agent/analyzer.go` 看 Phase 1.2 是否接入
4. `go test ./internal/analysis/amplifier/ -count=1 -v` 看规则覆盖
5. `bash eval/run.sh eval/cases/qf_architecture.case 2` 看 Phase 2 落地后 r1/r2 一致性
6. 按"§7 实施分批"顺序推进未完成 Phase

---

## 11. 与并行设计的边界

| 文档 | 焦点 | 与本文档关系 |
|---|---|---|
| `iteration_inflation_remediation.md` | finalizer 端 caveat / Implies / cross-scope retry | 独立。本文档处理 analyzer 上游;迭代膨胀处理 finalizer 下游 |
| `current_architecture_gap_remediation.zh-CN.md` | V2 carrier / contract 主链 | 独立。本文档不动 carrier 或 contract |

本文档专注 **RequestModel 的可选 typed 字段填充缺失** 这一独立维度。不影响也不依赖其他设计。
