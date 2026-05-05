# Analyzer IR Amplifier Layer — 设计文档

状态:Phase 0(待施工)
代码基线:`origin/main@f98235c`(2026-05-05)
责任范围:为 analyzer LLM emit 后的 RequestModel 补全 LLM 漏填的可选 typed 字段。不替代任何执行层契约。

---

## 1. 问题陈述

`AnalysisIR.RequestModel` 携带多组 typed 信号(`Buckets / SubTopics / Predicates / EnumerationBoundary / CompletenessObligation`),由 analyzer LLM 通过单次 `emit_analysis` 工具填写。这些信号是**下游执行层**(`AnswerSemanticView` 编译、`FacetBucketLabel HARD` 强制、enumeration 子契约等)做契约决策的唯一输入。

可选字段一旦在 LLM emit 时漏填,系统没有任何 deterministic 后处理把这些信号补回。因此当问题形态需要这些信号触发的契约,而 LLM 分类时遗漏,**整条契约链断裂**,finalizer 自由发挥。表现为同一问题的两次运行答案质量差异显著。

---

## 2. 系统中已有的相关组件

### 2.1 reconciler family — 已存在的 deterministic augmentation pipeline

`internal/agent/analyzer.go::buildAnalysisIR` 在 LLM emit 后顺序调用 7 个 reconciler 函数:

| 现有函数 | 文件 | 职责 |
|---|---|---|
| `reconcileEnumerationBoundaryScope` | `analyzer_boundary.go:19` | 验证 boundary 在 sub_topics 范围内 |
| `reconcileSemanticPredicates` | `analyzer_predicate.go:58` | 调 `IsCrossComponent` 的反向风险 |
| `reconcileComplexity` | `analyzer_complexity.go:70` | 复杂度交叉校验 |
| `reconcileIntent` | `analyzer_intent.go:131` | intent 与 predicates 一致性 |
| `inferAnswerSubject` | `analyzer_intent.go:243` | answer subject 推导(ADD-only)|
| `reconcilePredicateAxis` | `analyzer_predicate.go:31` | axis 推导(ADD-only)|
| `reconcileScenario` | `analyzer_intent.go:95` | scenario 与 intent 一致性 |

这是已建立的"deterministic augmentation"模式。所有调用都通过 `recordReconcileObservation` 统一观测。

### 2.2 执行层契约(已健全)

| 字段 / 机制 | 文件 | 用途 |
|---|---|---|
| `RequestModel.Buckets[]` | `types/analysis_ir.go:148` | 命名分区 |
| `RequestModel.SubTopics[]` | `types/analysis_ir.go:78` | 多 sub-topic 独立 DAG 节点 |
| `RequestModel.EnumerationBoundary` | `types/analysis_ir.go:131` | 用户声明的边界数量 |
| `RequestModel.CompletenessObligation` | `types/analysis_ir.go:144` | "all/every" 强制覆盖 |
| `Predicates.IsCategoryEnumeration` | `types/predicates.go` | 触发 enumeration 子契约 |
| `FacetBucketLabel HARD per bucket` | `types/facet_plan.go:919` | `Buckets ≥ 2` 时每 bucket 必出现一条 facet |
| `view.Buckets ≥ 2` 触发 QFComparison | `answer_semantic_view_compile_comparison.go:53` | 多 bucket → 比较结构 |
| `HDP.topSymbols()` | `analysis/hdp/planner.go` | 已实现的 multi-subject 结构性检测 |

### 2.3 Gap

reconciler family 的 7 个函数中,**没有任何一个填写或调整以下字段:**
- `Predicates.IsCategoryEnumeration`(只有 `reconcileSemanticPredicates` 处理 `IsCrossComponent`)
- `SubTopics`(只有 truncate 到 5 项的处理,没有派生)
- `Buckets`(完全无后处理)

LLM emit 是这三个字段的唯一来源。

---

## 3. 设计

### 3.1 包结构

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

### 3.2 调用点

`internal/agent/analyzer.go::buildAnalysisIR`,在以下范围内插入:

- 在 `rm.AnalyzerHints.MentionedEntities` 设置之后
- 在 log/perf-triage entity merge 之后
- 在 `sub_topics` truncate + complexity 升级处理之后
- 在 `reconcileSemanticPredicates` 之前
- 在 `compiler.Compile` 之前

具体在 `reconcileSemanticPredicates` 调用前(`analyzer.go:1133` 上方),作为 reconciler family 的前置步骤。

### 3.3 调用形式

```go
amplified, observations := amplifier.Amplify(rm)
for _, obs := range observations {
	recordReconcileObservation(ctxMutable(ctx), reconcileEvent(
		obs.Field, obs.Before, obs.After, 0, obs.Reason, rm.Predicates,
	))
}
rm = amplified
```

### 3.4 不变性

- amplifier **永不覆盖** LLM 已填的非零字段(`if rm.X != zero { skip }`)
- amplifier **永不读** `rm.RawRequest` 文本(只读 `TermGraph` / `AnalyzerHints.Entities` / `Predicates` 等 typed 字段)
- amplifier 规则**不允许引用具体测试 case 的字面 entity 名**
- amplifier 是**纯函数** — 同输入同输出,无副作用,无 I/O,无并发原语

---

## 4. 规则集

### R1 — Multi-subject Predicate Inference

**输入:** `rm.TermGraph`, `rm.Predicates`, `rm.Intent`, `rm.AnswerSubject`, `rm.AnalyzerHints`

**触发条件(全部满足):**
- `len(hdpTopSymbols(rm.TermGraph, 3)) ≥ 2`(复用 `internal/analysis/hdp/planner.go` 中既有 `topSymbols` 算法的同等逻辑;若该函数 unexport,提取 helper 到 amplifier 包内)
- AND `rm.Predicates.IsCategoryEnumeration == false`
- AND `rm.Intent ∈ {Explain, Trace, Compare}`
- AND `len(types.ExactResolutionTargets(rm)) != 1`
- AND `rm.AnswerSubject.Kind != types.SubjectScalar`

**动作:**
- 设 `rm.Predicates.IsCategoryEnumeration = true`
- 记录 Observation

**理由:** HDP topSymbols 基于 typed `TermGraph.Canonical[].Confidence` 检测 multi-subject。R1 把这个已有结构信号 propagate 到 predicates 层,补 LLM emit 层的盲点。Intent 限定 + scalar 排除 + single-exact-target 排除三道闸门确保不误触发。

**与下游 reconciler 的兼容:**
- `reconcileSemanticPredicates` 在 R1 之后跑,其 `signalsExplicitMultiAxis(p)` (predicate.go:164) 会读到新的 `IsCategoryEnumeration=true`,行为自然 propagate
- `reconcileComplexity` 在 R1 之后跑,其 `IsCategoryEnumeration` 检查 (complexity.go:152) 会触发正确的 simple→moderate 升级

### R2 — Typed-Name Parity → SubTopics Derivation

**输入:** `rm.TermGraph`, `rm.AnalyzerHints.Entities`, `rm.SubTopics`

**触发条件:**
- `rm.SubTopics` 为空
- AND `rm.TermGraph.Canonical` 中存在 ≥ 2 个 `CanonicalTerm` 共享 typed parent:
  - 同 `Kind`(同时是 `TermSymbol` 或同时是 `TermConfig` 等)
  - AND `Surface` 共享至少 4 字符的公共前缀或后缀(纯 string slot 操作,非 keyword 列表)
- AND 公共前缀 / 后缀本身是合法标识符(letter / digit / 下划线)
- AND 这一组共享 parent 的 entity 数 ≥ 2 且 ≤ 8(过多视为偶然)

**动作:**
- 为每个共享 parent 的 entity 生成 `SubTopic{Summary: <Surface>, Entities: [<Surface>]}`
- 设 `rm.SubTopics = derived`

**理由:** TermKind 是 typed 分类,public-prefix/suffix 是 string-slot 结构推断,均不依赖关键词词表。

**与下游 reconciler 的兼容:**
- `reconcileComplexity` 后续看到 `len(rm.SubTopics) ≥ 1` → 升级 simple→moderate(`analyzer.go:1103-1108` 已有逻辑)
- `coherence.go::flattenSubTopicEntities` 自然读取派生的 SubTopics

### R3 — Typed-Identifier MustInclude Pinning

**输入:** `rm.AnalyzerHints.Entities`, `rm.TermGraph`, `rm.Predicates`, `out.AnswerContract.MustInclude`

**关键时序约束:** R3 依赖 `compiler.Compile` 已生成 `out.AnswerContract`。因此 R3 不能在 buildAnalysisIR 的 `reconcileSemanticPredicates` 之前跑,必须有第二次 amplify pass。

**实现方案:**
- amplifier 包提供两个入口:
  - `Amplify(rm)` — pre-compile(R1, R2)
  - `AmplifyPostCompile(out *compiler.Output, rm types.RequestModel)` — post-compile(R3)
- 调用顺序:`Amplify(rm)` → `compiler.Compile(rm, sig)` → `AmplifyPostCompile(&out, rm)`

**触发条件:**
- `rm.Predicates.IsCategoryEnumeration == true`(包含 R1 触发后的结果)
- AND `rm.AnalyzerHints.Entities` 中存在 typed identifier(`TermGraph.Canonical[].Kind ∈ {TermSymbol, TermConfig, TermLiteral}` 的对应 Surface)
- AND 该 identifier 不在 `out.AnswerContract.MustInclude`

**动作:**
- 把符合条件的 typed identifier append 进 `out.AnswerContract.MustInclude`(去重)

**理由:** 当问题被识别为 enumeration AND analyzer 已确认这些 identifier 是命名实体(TermKind 非 concept),它们就是答案必含项。不依赖 question 文本的 keyword 匹配。

### R4 — Buckets Derivation(Phase 2 后视情况追加)

设计推迟。R4 涉及 question 多 `?` 切分 / parallel-naming pattern 检测,需要更细致的边界设计,首版不做。

---

## 5. 不在本设计内

- ❌ analyzer LLM prompt 调教(本设计走结构性补全路线,不动 prompt)
- ❌ 添加新的 typed 字段到 `RequestModel`(沿用现有字段)
- ❌ 修改 `AnswerSemanticView` 编译逻辑或契约执行层
- ❌ 修改 emit_analysis 工具的 schema 或 Parameters
- ❌ amplifier 读取 `rm.RawRequest` 文本做关键词/正则匹配
- ❌ amplifier 覆盖 LLM 已填的非零字段
- ❌ amplifier 为单个 case 设计字面规则

---

## 6. 实施分批

### Phase 1 — 框架与接入(2 commit)

**Phase 1.1**
- 新建 `internal/analysis/amplifier/` 包
- 实现 `Amplify` / `AmplifyPostCompile` 入口骨架(无规则)
- 实现 `Observation` 类型
- 单元测试:空 rm 恒等、LLM 满 rm 恒等、幂等性(二次调用无变化)

**Phase 1.2**
- 接入 `analyzer.go::buildAnalysisIR`(`Amplify` 在 reconcileSemanticPredicates 前)
- 接入 `analyzer.go::buildAnalysisIR`(`AmplifyPostCompile` 在 `compiler.RecomputeBudget` 后)
- 测试:`TestBuildAnalysisIR_AmplifierInsertionOrder` 钉死调用顺序
- 全量回归测试 0 失败

### Phase 2 — R1 规则(2 commit)

**Phase 2.1**
- 在 amplifier 包内实现 `topSymbols` 等价 helper(避免对 hdp 包的依赖)
- 实现 R1 规则
- 单元测试:多 subject 触发、单 subject 不触发、scalar 排除、single-exact-target 排除、Intent 限定

**Phase 2.2**
- 真 eval 验证:qf_arch x4 + s1a x2 + m1a x2
- 验收:qf_arch 两次 PASS(消除 r1/r2 随机性)

### Phase 3 — R2 规则(2 commit)

**Phase 3.1**
- 实现 `commonAffixGroups(canonical []CanonicalTerm) [][]string`
- 实现 R2 规则
- 单元测试:6 个 Stage* → 1 组 → 6 个 SubTopic

**Phase 3.2**
- 真 eval 验证:m1a x4 + qf_arch x2
- 验收:m1a r1 + r2 都覆盖 emit_evidence(R2 派生 [explorer agent, extractor agent] subtopics → 下游每 subtopic 期望 emit_* 列表)

### Phase 4 — R3 规则(2 commit)

**Phase 4.1**
- 实现 `AmplifyPostCompile` R3 分支
- 单元测试:enumeration + named identifier → MustInclude 注入

**Phase 4.2**
- 真 eval 跨 5 case 验证
- 验收:案 EXPECT_CONTAINS 中的命名 identifier 在 enumeration 类问题中稳定覆盖

---

## 7. 验证指标

每完成一个 Phase 跑 5 case x 2 runs:

| Phase | 必跑 case | 期望 |
|---|---|---|
| 1 | qf_arch x2 + s1a x2 | 0 回归 |
| 2 | qf_arch x4 + s1a x2 + m1a x2 | qf_arch 两次都 PASS |
| 3 | qf_arch x4 + s1a x2 + m1a x4 + u3a x2 | m1a 两次都包含 emit_evidence;u3a 不回归 |
| 4 | 全 5 case x 2 runs | 含 enumeration 的 case 命名 identifier 全覆盖 |

任何 Phase 引入回归 → 立即回滚,debug 后再推。

---

## 8. 红线 / 不变性

| # | 规则 | 强制方式 |
|---|---|---|
| 1 | amplifier 永不读 `rm.RawRequest` 文本 | 单元测试 + 代码 review |
| 2 | amplifier 永不覆盖 LLM 已填的非零字段 | 每条规则前置 `if existing != zero { skip }` |
| 3 | amplifier 规则不引用具体测试 case 字面 entity 名 | 代码 review;测试覆盖结构性输入 |
| 4 | amplifier 只读 typed slots(TermGraph/Entities/Predicates) | 函数签名约束(无 ctx 参数,无 reader 注入) |
| 5 | amplifier 输出与 reconcileEvent telemetry 兼容 | Observation 类型对齐 reconcileEvent 字段 |
| 6 | amplifier 是纯函数 | 函数签名 + 单元测试(随机输入幂等性)|

---

## 9. 接续 checklist

如本 session 中断,下个 session 接续按以下顺序确认进度:

1. `git log origin/main` 确认相关 commit
2. `ls internal/analysis/amplifier/` 看 Phase 1.1 是否落地
3. `grep -n amplifier.Amplify internal/agent/analyzer.go` 看 Phase 1.2 是否接入
4. `go test ./internal/analysis/amplifier/ -count=1 -v` 看规则覆盖
5. `bash eval/run.sh eval/cases/qf_architecture.case 2` 看 Phase 2 落地后 r1/r2 一致性
6. 按"实施分批"小节顺序推进未完成 Phase

---

## 10. 与并行设计的边界

| 文档 | 焦点 | 与本文档关系 |
|---|---|---|
| `iteration_inflation_remediation.md` | finalizer 端 caveat / Implies / cross-scope retry | 独立。本文档处理 analyzer 上游;迭代膨胀处理 finalizer 下游 |
| `current_architecture_gap_remediation.zh-CN.md` | V2 carrier / contract 主链 | 独立。本文档不动 carrier 或 contract |

本文档专注 **RequestModel 的可选 typed 字段填充缺失** 这一独立维度。不影响也不依赖其他设计。
