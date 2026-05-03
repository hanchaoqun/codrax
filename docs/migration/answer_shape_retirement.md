# Block-Only 现状审计与 `AnswerShape` 退役实施方案

更新时间：基于 `origin/main@7a14a9d21aeffac81a15b527e6610173b8a9d6d1`

## 1. 文档目的

这份文档面向当前远端代码，回答三个问题：

1. 当前仓库是否已经实现 **block-only**。
2. `AnswerShape` 当前到底还残留在哪些链路里，它是否已经退化成“仅用于 analyzer intent classifier”。
3. 如何在**不退化答案质量**、**不引入新的双栈复杂度**、**不耦合特定题型或特定仓**的前提下，彻底删除旧 `shape` 世界，收敛到：
   - `QuestionFamily` 作为唯一的粗粒度问题族分类
   - `AnswerSemanticView` 作为唯一的 read-mode 运行时语义契约
   - `AnswerDocumentV2` 作为唯一的最终答案 carrier

这份文档不是概念提案，而是可以直接指导开发的实施蓝图。它会明确：

- 现状问题
- 最终目标
- 迁移阶段
- 每个文件要做什么
- 哪些旧入口必须删除
- 哪些测试/注释/提示词必须同步清理
- 完成后的审计标准

---

## 2. 结论摘要

### 2.1 已经完成的部分

read-mode 的最终答案载体已经是 **block-only**：

- `emit_answer_document` 的 read-mode入口只接受 V2 carrier
- `MutableState` 的最终答案缓冲区是 `AnswerDocumentV2`
- finalizer 解析的是 `AnswerDocumentV2`
- renderer 渲染的是 block-only 文档
- authority hedging 已经是 block 级别
- orchestrator 的核心最终答案 gate 已经开始围绕 V2 block contract 工作

关键代码：

- `internal/tool/emit_answer_document.go`
- `internal/tool/emit_answer_document_v2.go`
- `internal/types/answer_document_v2.go`
- `internal/agent/answer_document_evaluator.go`
- `internal/render/answerdoc.go`
- `internal/render/apply_authority_hedging.go`
- `internal/orchestrator/contract_check_block.go`

### 2.2 还没有完成的部分

系统还没有真正进入“纯 block-only 语义架构”。

当前状态更准确地说是：

> **block-only carrier + dual semantic control**

也就是：

- 最终答案 carrier 已经是 V2
- 但上游 analyzer/compiler/explorer/extractor/finalizer prompt/hint/budget/taxonomy/reviewer 仍然大量读取和传播 `AnswerShape`

### 2.3 核心判断

`AnswerShape` **目前还不是**“仅作为 Analyzer intent classifier”。

它仍然在以下维度承担运行时决策职责：

- compiler 输出契约
- exact-absence 降级逻辑
- summary cap 预算
- extractor 的结构化预期
- explorer 的 closure/readiness 判断
- finalizer prompt 的 block 提示和 checklist
- pre-complete 修复与 subject-shape mismatch
- taxonomy / reviewer / hint composer 的元信息过滤
- analysis gate / contract checker 的旧形态校验

因此，当前的正确方向不是“简单把 `AnswerShape` 改成 `QuestionFamily`”，而是：

1. `QuestionFamily` 成为唯一的**粗粒度问题族分类**
2. `AnswerSemanticView` 成为唯一的**read-mode 运行时语义真理源**
3. `AnswerDocumentV2` 成为唯一的**最终答案 carrier**
4. `WriteOutputKind` 成为唯一的**write-mode 输出分类**
5. 彻底删除 `AnswerShape` 在 read-mode 与 write-mode 的残留职责

---

## 3. 当前代码现状：按链路逐段审计

## 3.1 已经 block-only 的链路

### 3.1.1 emit 工具入口

文件：

- `internal/tool/emit_answer_document.go`
- `internal/tool/emit_answer_document_v2.go`

现状：

- `emit_answer_document` 的 LLM-facing description 已明确声明 V1 retired
- `Execute()` 中：
  - `document_model == "v2"` 走 V2
  - `document_model` 缺失或空，也被视为 V2 尝试
  - 任何其它值直接拒绝
- `executeAnswerDocumentV2()` 会拒绝 top-level V1 字段：
  - `shape`
  - `steps`
  - `symbols`
  - `value`
  - `boolean`
  - `summary`

结论：

- **最终 emit 路径已经是 V2-only**
- 但测试/注释中还有部分“RoutesV1”命名残留，需要清扫

### 3.1.2 最终 carrier

文件：

- `internal/types/answer_document_v2.go`
- `internal/types/context.go`

现状：

- `MutableState` 已持有 `AnswerDocumentV2`
- `SetAnswerDocumentV2` / `AnswerDocumentV2` / `ResetAnswerDocumentV2` 已完整落地
- V2 block 可携带：
  - `facet_ids`
  - `claim_uses`
  - `surface_role`
  - list item / diagram item 级 claim annotations

结论：

- **carrier 层已经满足 block-only 终局要求**

### 3.1.3 finalizer parse / renderer / hedging

文件：

- `internal/agent/answer_document_evaluator.go`
- `internal/render/answerdoc.go`
- `internal/render/apply_authority_hedging.go`

现状：

- `parseOutputV2(...)` 是 finalizer 成功路径
- renderer 按 block kind 渲染：
  - `summary`
  - `section`
  - `ordered_list`
  - `bullet_list`
  - `scalar`
  - `decision`
  - `table`
  - `diagram`
  - `caveat`
- hedging 已经应用在 `AnswerDocumentV2`

结论：

- **渲染面已经进入 V2 世界**

### 3.1.4 orchestrator block gate

文件：

- `internal/orchestrator/contract_check.go`
- `internal/orchestrator/contract_check_block.go`

现状：

- `runV2BlockOracles()` 已是 read-mode 主 block gate
- 当前已存在的 V2 block 校验包括：
  - `validateRequiredBlockCoverage`
  - `validatePrincipalClaimUse`
  - `validateDiagramEdgeSupport`
  - `validateUncertaintyBlockPresence`

结论：

- **block contract 已经开始替代旧 V1 answer-oracle**

---

## 3.2 仍然由 `AnswerShape` 驱动的链路

这一节是本次审计的重点。以下每一点都说明：`AnswerShape` 仍在参与运行时决策，绝不是“仅分类用”。

### 3.2.1 IR 合同仍保留 `RequiredAnswerShape`

文件：

- `internal/types/analysis_ir.go`

现状：

- `AnswerContract.RequiredAnswerShape AnswerShape`
- `AnswerShape` 仍定义了 read-mode shapes：
  - `list_of_symbols`
  - `step_list`
  - `value`
  - `boolean`
  - `config_value`
  - `explanation`
  - `none`
- 同时仍保留 write-mode shapes：
  - `change_plan`
  - `change_report`

问题：

- read-mode 已经 block-only，但 IR 仍在输出旧 carrier 时代的形状分类
- write-mode 其实已经有 `WriteOutputKind`，但旧枚举还没删

影响：

- 所有依赖 `AnswerContract` 的后续链路都可能继续读取 `RequiredAnswerShape`

### 3.2.2 compiler 仍直接编 shape

文件：

- `internal/analysis/compiler/templates.go`
- `internal/agent/analyzer.go`

现状：

- template 仍显式设置 `RequiredAnswerShape`
- analyzer 仍对某些场景 patch `RequiredAnswerShape`
- analyzer 注释里虽然说 “V2 carrier path no longer reads RequiredAnswerShape for rendering decisions”，但后续运行时并未完全摆脱它

问题：

- analyzer/compiler 仍把 read-mode contract 主体表达成 shape，而不是 family + semantic view

### 3.2.3 `answer_shape_runtime.go` 仍在决定真实行为

文件：

- `internal/types/answer_shape_runtime.go`

现状：

- `StableAbsentExactConfigRequiresExplanation`
- `EffectiveRequiredAnswerShape`
- `ExplanationAllowsAnchorSkeleton`

问题：

- 这不是“只保留枚举信息”，而是**直接在运行时改写答案形态**
- exact-absence 的表达策略本应属于 semantic view / exact-resolution render policy，而不是 shape runtime helper
- multi-topic explanation anchor skeleton 也应属于 block/facet policy，而不是 shape gate

影响：

- explorer
- answer surface plan
- answer_document_evaluator
- gate/coherence
- extractor

### 3.2.4 `AnswerSurfacePlan` 仍携带 `RequiredShape`

文件：

- `internal/types/answer_surface_plan.go`

现状：

- `AnswerSurfacePlan.RequiredShape`
- `BuildAnswerSurfacePlan(...)` 仍会：
  - 先用 `EffectiveRequiredAnswerShape(...)`
  - 再 fallback 到 `ir.AnswerContract.RequiredAnswerShape`

问题：

- `AnswerSurfacePlan` 已经是新旧世界交汇点
- 但它仍把旧 `shape` 作为一个显式字段传播到 downstream

影响：

- semantic view compile
- explorer drift-bounded closure
- finalizer prompt

### 3.2.5 `AnswerSemanticView` 编译仍反向读取 `plan.RequiredShape`

文件：

- `internal/types/answer_semantic_view_compile.go`

现状：

- trace 输出里仍记录 `plan_shape`
- semantic view 编译辅助层仍知道 `RequiredShape`

问题：

- 这说明 semantic view 还不是“shape-free truth source”

### 3.2.6 finalizer prompt 仍在 shape 语义上构造行为

文件：

- `internal/agent/answer_document_evaluator.go`
- `internal/skill/defaults.go`

现状：

- evaluator 虽然删除了旧的 `## Target answer shape` section
- 但仍读取 `ctx.AnalysisIR.AnswerContract.RequiredAnswerShape`
- 仍按 shape 决定：
  - submission checklist
  - scalar lookup discipline
  - step backbone
  - enumeration boundary
  - multi-topic explanation anchor skeleton
- `answer-document-skill` 仍直接告诉 LLM：
  - target shape mandatory
  - shape requires specific fields

问题：

- 这意味着模型看到的“主要 contract 语言”仍然是 shape，而不是 blocks/facets
- 即使 carrier 已经是 blocks，prompt 心智仍然是旧 shape 世界

### 3.2.7 extractor 仍按 shape 决定结构化预期

文件：

- `internal/agent/extractor.go`

现状：

- `isListOfSymbolsShape`
- `isBoundedPrincipalStepList`
- `needsAnswerSymbols`
- `hasSufficientAnswerSymbols`
- `requestedEnumerationBoundary(...)` 相关逻辑

问题：

- 这些函数先读 `RequiredAnswerShape`
- `QuestionFamily` 只是 fallback

影响：

- structured extraction 行为仍旧 shape-first
- 这会拖慢真正的语义统一，因为 extraction 还在按旧心智组织结果

### 3.2.8 explorer 的 closure/readiness 仍依赖 shape

文件：

- `internal/agent/explorer.go`

现状：

- `driftBoundedCompletionReadyMode()`
- `driftBoundedCompletionReason()`
- 构造 fallback plan 时仍显式塞 `RequiredShape`
- `plan.RequiredShape == ShapeStepList` 之类判断仍在

问题：

- closure 逻辑本应看 semantic obligations / required blocks / required facets
- 现在却仍看 shape

### 3.2.9 pre-complete repair 仍在做 subject-shape mismatch

文件：

- `internal/tool/emit_investigation_complete.go`

现状：

- `AnswerSubject` vs `RequiredAnswerShape` mismatch 逻辑仍然保留

问题：

- 这是一类旧世界 repair 思维
- 长期应迁到：
  - `QuestionFamily`
  - `FacetCoverage`
  - `AnswerSemanticView.RequiredBlocks`
  - `ExactResolution`

### 3.2.10 old contract checker 仍按 shape 走

文件：

- `internal/analysis/contract/checker.go`

现状：

- `checkShape(...)` 仍然完全按 `RequiredAnswerShape` 做 V1-era text heuristics
- 例如：
  - boolean 看前缀 yes/no
  - list 看 bullet/fenced symbols
  - step_list 看 numbered steps

问题：

- 虽然 orchestrator 新主路径已经是 V2 block oracles
- 但 `contract.CheckWithOracle(...)` 仍在 `runContractCheck(...)` 开头被调用
- 这条链本质上还是旧 shape contract checker

结论：

- 这是需要重点消除的旧入口之一

### 3.2.11 summary budget 仍是 shape-based

文件：

- `internal/types/answer_document.go`

现状：

- `SummaryCapFor(shape, itemCount)`

问题：

- budget 本应围绕 block requirement / semantic view / richness tier
- 继续按 shape 做预算，会把旧世界持续带回 runtime

### 3.2.12 taxonomy / reviewer / hints 仍在暴露 shape

文件：

- `internal/orchestrator/answer_taxonomy_store.go`
- `internal/orchestrator/answer_reviewer.go`
- `internal/orchestrator/orchestrator.go`
- `internal/analysis/hint/composer.go`

现状：

- `AnswerTaxonomyStore.RelevantTo(scenario, shape, k)`
- `AnswerReviewerInput.Shape`
- orchestrator 仍按 `scenario + shape` 注入 answer pitfalls
- hint composer 仍有：
  - `TargetShape`
  - `Re-emit with shape=...`

问题：

- 这些是典型的“旁路残留”
- 若不清理，会在 prompt / review / learning loop 中持续复活旧 shape 语义

### 3.2.13 `context/builder.go` 仍按 shape 决定 prompt sections

文件：

- `internal/context/builder.go`

现状：

- `isCitationFreeValueAnswer(...)` 仍要求 `RequiredAnswerShape == ShapeValue`
- Raw Tool Outputs section 是否渲染仍受此影响

问题：

- 这类 prompt builder 旁路非常容易漏
- 长期应改成看 semantic view / block requirement，而不是看 shape

---

## 4. 当前现状存在的主要问题

## 4.1 概念层问题

### 问题 1：carrier 已 block-only，但 runtime 语义仍双栈

表现：

- V2 blocks 已是最终载体
- 但大量上游逻辑还在读 `AnswerShape`

后果：

- “载体已经新了，心智仍旧”
- 开发者很容易误判“只差删几个枚举”
- 实际上 shape 仍深度参与行为决策

### 问题 2：`AnswerShape` 与 `QuestionFamily` 同时承担分类职责

表现：

- `QuestionFamily` 已经负责 facet template / semantic view family dispatch
- `AnswerShape` 又在负责 runtime 结构倾向

后果：

- 二者并行会产生职责冲突
- 容易出现：
  - family 说这是 `QFEnumeration`
  - 但 shape 仍按 `ShapeExplanation` 驱动部分逻辑

### 问题 3：shape 仍在 prompt 中直接暴露给模型

表现：

- skill defaults
- hint composer
- evaluator prompt branch

后果：

- 即使底层已经 block-only，模型仍在以 V1 shape 心智工作
- 会拖慢迁移并放大 prompt 歧义

### 问题 4：write-mode 和 read-mode 的 shape 遗留还未切干净

表现：

- `WriteOutputKind` 已存在
- 但 `AnswerShape` 仍保留 `change_plan/change_report`

后果：

- 一类枚举仍跨 read/write 两个世界
- 删旧难度被不必要地抬高

### 问题 5：测试、注释、命名存在删旧不彻底的漂移

表现：

- `emit_answer_document_v2_test.go` 里仍有 `NoDocumentModelRoutesV1` / `EmptyDocumentModelRoutesV1` 这类旧命名
- 运行时代码和测试命名语义已经不完全一致

后果：

- 新开发者会被误导
- 后续改造时容易基于旧语义继续扩展错误测试

---

## 5. 最终目标

最终目标必须是明确、强硬、可验证的：

### 目标 A：read-mode 彻底 shape-free

read-mode 下，以下内容必须全部删除或退役：

- `AnswerShape`
- `RequiredAnswerShape`
- `EffectiveRequiredAnswerShape`
- `answer_shape_runtime.go`
- shape-based prompt sections
- shape-based extractor / explorer branching
- shape-based summary cap
- shape-based taxonomy / reviewer filters
- shape-based repair hints

### 目标 B：`QuestionFamily` 成为唯一粗粒度分类轴

analyzer 输出只保留：

- `QuestionFamily`
- `QuestionStructure`
- `AnswerSubject`
- `PredicateAxis`
- `ExactResolution`
- `DiagramContract`

不再输出 read-mode `RequiredAnswerShape`。

### 目标 C：`AnswerSemanticView` 成为唯一运行时语义契约

所有 read-mode 运行时行为统一由它决定：

- required blocks
- optional blocks
- facet coverage
- diagram plan
- uncertainty rules
- exact resolution render policy
- richness candidates
- scalar / ordered-list / diagram / caveat requirement

### 目标 D：`AnswerDocumentV2` 成为唯一最终答案 carrier

这点当前已基本达成，但必须继续保证：

- 无 V1 fallback
- 无 V1 emit path
- 无 shape-based render branch
- 无 V1-only validation branch

### 目标 E：write-mode 彻底独立

write-mode 只使用：

- `WriteOutputKind`
- `ChangePlan`
- `ChangeReport`

不得再借用 `AnswerShape`。

---

## 6. 设计原则

1. **不引入第三套分类**
   - 不能用新的“pseudo shape”替代旧 shape
   - 必须是 `QuestionFamily + AnswerSemanticView + BlockRequirement`

2. **不把 `QuestionFamily` 误用成 runtime contract**
   - `QuestionFamily` 只负责 coarse classification
   - 细粒度结构义务必须由 semantic view 编译出来

3. **不保留 silent fallback**
   - 任何 read-mode 路径都不允许“看不到 semantic view 就退回 shape”

4. **不靠 prompt 文案掩盖 runtime 双栈**
   - prompt 必须反映真实单一语义源

5. **不重复造轮子**
   - 当前已有的：
     - `QuestionFamily`
     - `FacetCoverageContract`
     - `AnswerSemanticView`
     - `RenderedClaimUse`
     - `AnswerDocumentV2`
   - 应作为官方底座继续推进，而不是再造平行体系

---

## 7. 可落地实施计划

## Phase 0：冻结旧 shape 扩张

目标：

- 在开始删旧前，先禁止任何新代码继续依赖 `AnswerShape`

动作：

- 新增一条开发规则：任何 read-mode 新逻辑不得新增 `RequiredAnswerShape` 读取点
- 新增注释和 lint/grep 检查，阻止新增：
  - `switch RequiredAnswerShape`
  - `shape == Shape...`
  - `Re-emit with shape=`

验收：

- 本阶段后，`AnswerShape` 命中数只能下降，不能上升

## Phase 1：先把 write-mode 从 `AnswerShape` 彻底拆掉

这是第一优先级，因为它最清晰、收益最大、风险最小。

### 文件

- `internal/types/analysis_ir.go`
- `internal/types/write_output_kind.go`
- `internal/types/change_plan.go`
- write-mode orchestrator / planner / verifier 相关代码

### 动作

1. 从 `AnswerShape` 删除：
   - `ShapeChangePlan`
   - `ShapeChangeReport`
2. 在 write-mode 所有调用链上只使用 `WriteOutputKind`
3. 删除任何“read-mode V2 carrier + write-mode shape carrier 共用同一枚举”的注释和逻辑

### 验收

- `AnswerShape` 只剩 read-mode 残留
- 所有 `change_plan/change_report` 命中只来自 `WriteOutputKind`

## Phase 2：移除 `AnswerSurfacePlan.RequiredShape`

这是整个迁移的核心转折点。

### 文件

- `internal/types/answer_surface_plan.go`
- `internal/types/answer_semantic_view_compile.go`
- `internal/types/answer_semantic_view_compile_*.go`

### 动作

1. 删除 `AnswerSurfacePlan.RequiredShape`
2. 不再在 `BuildAnswerSurfacePlan(...)` 中调用：
   - `EffectiveRequiredAnswerShape(...)`
3. 在 `AnswerSemanticView` 层新增 helper，替代旧 shape 语义：
   - `NeedsPrincipalScalar()`
   - `NeedsOrderedPrincipalList()`
   - `NeedsEnumerationSlate()`
   - `AllowsAnchorSkeleton()`
   - `NeedsBoundedMechanismList()`
   - `NeedsCitationFreeScalarIngest()`
4. `emitSemanticViewTrace(...)` 删除 `plan_shape`

### 验收

- `AnswerSurfacePlan` 不再携带任何 `AnswerShape`
- semantic view compile 不再反向读取 shape

## Phase 3：删除 `answer_shape_runtime.go`

### 文件

- `internal/types/answer_shape_runtime.go`
- 所有调用点

### 迁移方式

#### 3.1 `StableAbsentExactConfigRequiresExplanation`

迁入：

- `AnswerSemanticView` 编译层
- 或 `ExactResolution + SummaryMode + RequiredBlocks` 的组合策略

目标：

- exact-absence 的表达不再依赖 shape rewrite

#### 3.2 `ExplanationAllowsAnchorSkeleton`

迁入：

- `AnswerSemanticView` helper
- 由 `QuestionFamily + QuestionStructure + FacetCoverage` 决定

#### 3.3 `EffectiveRequiredAnswerShape`

彻底删除，不允许任何替代物

### 验收

- `answer_shape_runtime.go` 删除
- 所有调用点改为 semantic view helper

## Phase 4：把 finalizer prompt 从 shape 改成 block/facet contract

### 文件

- `internal/agent/answer_document_evaluator.go`
- `internal/skill/defaults.go`

### 动作

1. 删除所有 `shape` 局部变量驱动的 checklist/backbone/boundary 分支
2. 改成从 `AnswerSemanticView` 读取：
   - required blocks
   - optional blocks
   - principal/support/caveat roles
   - facet coverage
   - diagram plan
   - uncertainty rules
3. `answer-document-skill` 改写：
   - 删除 “target shape is mandatory”
   - 改成 “required blocks / required fields / forbidden omissions are mandatory”
4. prompt 中不再出现：
   - `shape=value`
   - `shape=step_list`
   - `shape=list_of_symbols`
   这类 V1 世界术语

### 验收

- finalizer prompt 对 read-mode 不再暴露 shape 语义
- prompt 只描述 blocks/facets/claim uses/diagram plan

## Phase 5：把 extractor 从 shape-first 改成 semantic-view-first

### 文件

- `internal/agent/extractor.go`

### 动作

删除或重写以下函数：

- `isListOfSymbolsShape`
- `isBoundedPrincipalStepList`
- `isMultiTopicExplanation` 中任何 shape gating

改为：

- `viewNeedsEnumerationSlate(ctx)`
- `viewNeedsBoundedPrincipalOrderedList(ctx)`
- `viewAllowsAnchorSkeleton(ctx)`
- `viewNeedsPrincipalScalar(ctx)`

### 验收

- extractor 不再读取 `RequiredAnswerShape`
- `QuestionFamily` 只用于 semantic view compile，不用于 extractor fallback

## Phase 6：把 explorer 的 readiness / closure 从 shape 改成 semantic obligations

### 文件

- `internal/agent/explorer.go`

### 动作

重写以下路径：

- `driftBoundedCompletionReadyMode`
- `driftBoundedCompletionReason`
- 所有构造临时 `AnswerSurfacePlan{RequiredShape: ...}` 的地方
- `plan.RequiredShape == ...` 判断

替代依据：

- `AnswerSemanticView.RequiredBlocks`
- `FacetCoverageContract.Required`
- `DiagramPlan`
- `UncertaintyRules`
- `ExactResolution`

### 验收

- explorer 不再用 shape 判断 closure mode

## Phase 7：把 pre-complete / hint / taxonomy / reviewer 从 shape 迁走

### 7.1 emit_investigation_complete

文件：

- `internal/tool/emit_investigation_complete.go`

动作：

- 删除 subject-shape mismatch 语义
- 改成 subject vs semantic obligations mismatch

### 7.2 hint composer

文件：

- `internal/analysis/hint/composer.go`

动作：

- 删除 `TargetShape`
- 删除 `Re-emit with shape=...`
- 新增：
  - `RequiredBlocks`
  - `FacetGap`
  - `MissingPrincipalClaimUse`
  - `DiagramRequirement`

### 7.3 taxonomy / reviewer

文件：

- `internal/orchestrator/answer_taxonomy_store.go`
- `internal/orchestrator/answer_reviewer.go`
- `internal/orchestrator/orchestrator.go`
- `internal/types/answer_taxonomy.go`

动作：

- `RelevantTo(scenario, shape, k)` 改成：
  - `RelevantTo(family, blockKinds, k)` 或
  - `RelevantTo(questionFamily, semanticTags, k)`
- reviewer 输入中的 `Shape` 改为：
  - `QuestionFamily`
  - `RequiredBlocks`
  - `FacetCoverageSummary`

### 验收

- learning loop 不再传播 shape 术语

## Phase 8：把 summary budget 从 shape 改成 semantic-view policy

### 文件

- `internal/types/answer_document.go`
- `internal/agent/answer_document_evaluator.go`

### 动作

删除：

- `SummaryCapFor(shape, itemCount)`

替换成：

- `SummaryBudgetProfileForView(view, blockCount, principalCount)`

策略建议：

- explanation-like rich answer：按 `RequiredBlocks + RichnessCandidates`
- scalar answer：按是否要求 explanation block / caveat block / uncertainty block
- enumeration / ordered list：按 principal item 数和 required prose blocks

### 验收

- budget 决策不再依赖 `AnswerShape`

## Phase 9：删除 `AnswerShape` 与 `RequiredAnswerShape`

这是最终删旧阶段，只能在前面全部完成后执行。

### 必删文件/字段

- `internal/types/analysis_ir.go`
  - 删除 `AnswerShape`
  - 删除 `AnswerContract.RequiredAnswerShape`
- `internal/types/answer_shape_runtime.go`
  - 整文件删除
- `internal/types/answer_surface_plan.go`
  - 删除 `RequiredShape`
- `internal/analysis/compiler/templates.go`
  - 不再编 shape，只编 family-relevant contract
- 所有调用 `RequiredAnswerShape` 的 read-mode 代码

### 必删测试/注释/命名

- `*_test.go` 中所有 read-mode `RequiredAnswerShape` fixture
- `emit_answer_document_v2_test.go` 中旧 V1 路由命名
- docs / comments 中所有 “target shape” / “shape requires”

### 终局 grep 归零要求

read-mode 完成后，以下 grep 在 read-mode 代码中必须为 0：

- `RequiredAnswerShape`
- `AnswerShape`
- `EffectiveRequiredAnswerShape`
- `StableAbsentExactConfigRequiresExplanation`
- `ExplanationAllowsAnchorSkeleton`
- `shape requires`
- `target shape`
- `Re-emit with shape=`
- `ShapeChangePlan`
- `ShapeChangeReport`

允许保留的唯一例外：

- 迁移期 write-mode 兼容层
- 纯历史注释若尚未清理则视为不合格，不算例外

---

## 8. 按文件逐项实施清单

下面这张清单可以直接指导不熟悉仓库的开发同学开工。

## 8.1 `internal/types/analysis_ir.go`

现状：

- `AnswerContract.RequiredAnswerShape`
- `AnswerShape`
- `ShapeChangePlan/ShapeChangeReport`

要做：

1. 先删除 write-mode shapes
2. 最终删除 read-mode `AnswerShape`
3. read-mode 契约改为：
   - `QuestionFamily`
   - `QuestionStructure`
   - `DiagramContract`
   - `ExactResolution`
   - `Language`
   - `AcceptanceTests`
4. 若确需保留兼容字段，必须明确标成 migration-only，并给删除里程碑

## 8.2 `internal/types/write_output_kind.go`

现状：

- 已存在

要做：

1. 成为 write-mode 唯一分类轴
2. 清除 `AnswerShape` 中的 write-mode 镜像定义

## 8.3 `internal/analysis/compiler/templates.go`

现状：

- 每个 template 仍直接写 `RequiredAnswerShape`

要做：

1. 停止把 template 输出写成 shape
2. 改成只表达：
   - `QuestionFamily`
   - `CitationReq`
   - `AcceptanceTests`
   - `Language`
3. 任何“需要 list / scalar / steps”的意图由 semantic view compile 决定，而不是由 template 输出 shape

## 8.4 `internal/types/answer_surface_plan.go`

现状：

- `RequiredShape`

要做：

1. 删除 `RequiredShape`
2. 任何读取 `EffectiveRequiredAnswerShape(...)` 的逻辑都迁到 semantic view compile
3. plan 中只保留与 evidence/surface/drift/exact/diagram 相关的 typed policy

## 8.5 `internal/types/answer_semantic_view_compile.go`

现状：

- 仍会 trace `plan_shape`

要做：

1. 删除 `plan_shape`
2. 增加 helper methods，承接原 shape 语义：
   - principal scalar
   - enumeration slate
   - ordered principal list
   - anchor skeleton
   - uncertainty block
   - citation-free scalar support

## 8.6 `internal/types/answer_shape_runtime.go`

现状：

- 仍在 active use

要做：

1. 全量迁移
2. 删除整文件

## 8.7 `internal/agent/answer_document_evaluator.go`

现状：

- prompt 构造仍按 shape 分支

要做：

1. 所有 `shape := ...` 分支改成 semantic view helper
2. `renderAnswerDocSubmissionChecklist(...)` 改为 block/facet based
3. `renderAnswerDocStepBackbone(...)` / `renderAnswerDocEnumerationBoundary(...)` 不再接受 shape 参数
4. scalar lookup / role lookup discipline 改成 family + required blocks 驱动

## 8.8 `internal/skill/defaults.go`

现状：

- `answer-document-skill` 仍是 target-shape contract

要做：

1. 重写 skill 目标与 workflow
2. 用这些术语替换 shape：
   - required blocks
   - required block fields
   - facet coverage
   - principal/support/caveat
   - claim_use
   - diagram plan

## 8.9 `internal/agent/extractor.go`

现状：

- shape-first branching

要做：

1. 删除 `isListOfSymbolsShape`
2. 删除 `isBoundedPrincipalStepList`
3. 新增 semantic-view helper wrappers
4. 所有 read-mode extraction sufficiency 判定转为：
   - required facet coverage
   - required principal claim inventory
   - required block material readiness

## 8.10 `internal/agent/explorer.go`

现状：

- drift-bounded completion 仍看 shape

要做：

1. 删除 `EffectiveRequiredAnswerShape` 依赖
2. 以 semantic obligations 判定 closure
3. fallback drift plan 不再伪造 `RequiredShape`

## 8.11 `internal/tool/emit_investigation_complete.go`

现状：

- 仍做 subject-shape mismatch

要做：

1. 替换成 subject vs semantic-view mismatch
2. scalar / role-lookup / config-absence 等 repair 都按 family/facet/exact-resolution 驱动

## 8.12 `internal/analysis/hint/composer.go`

现状：

- `TargetShape`
- `Re-emit with shape=...`

要做：

1. 删除 TargetShape
2. 新增：
   - target blocks
   - missing facet ids
   - missing principal claim uses
   - unsupported diagram edges
   - uncertainty gap

## 8.13 `internal/orchestrator/answer_taxonomy_store.go`

现状：

- `RelevantTo(scenario, shape, k)`

要做：

1. 替换 shape 过滤为：
   - family
   - semantic tags / required block kinds
2. 持久化 schema 升级

## 8.14 `internal/orchestrator/answer_reviewer.go`

现状：

- reviewer 输入包含 `Shape`

要做：

1. 移除 Shape
2. 改为：
   - `QuestionFamily`
   - `RequiredBlocks`
   - `FacetCoverageSummary`
   - `RichnessMisses`

## 8.15 `internal/orchestrator/orchestrator.go`

现状：

- answer taxonomy 注入仍按 `scenario + shape`

要做：

1. 注入依据改为：
   - `QuestionFamily`
   - `RequiredBlocks`
   - `SemanticView tags`
2. 删除 `in.Shape = ...`

## 8.16 `internal/context/builder.go`

现状：

- `isCitationFreeValueAnswer` 仍看 `ShapeValue`

要做：

1. 改成 semantic-view helper：
   - `viewNeedsCitationFreeScalarInput`
2. 所有 prompt sections gating 不再依赖 shape

## 8.17 `internal/types/answer_document.go`

现状：

- 还承担 summary cap config

要做：

1. 去 shape 化
2. 若仍保留共享 payload/support type，可继续保留文件本身
3. 但任何 `SummaryCapFor(shape...)` 必须删除

## 8.18 `internal/analysis/contract/checker.go`

现状：

- 仍是 old shape contract checker

要做：

1. 明确定位：
   - 若 read-mode 仍走它，则必须重写为 block/facet/claim_use aware
   - 若 orchestrator V2 path 已完全足够，则应逐步 retire read-mode shape checks
2. 不允许长期保持：
   - V2 final answer + V1 shape checker 同时工作

## 8.19 测试与注释清扫

重点文件：

- `internal/tool/emit_answer_document_v2_test.go`
- `internal/agent/answer_document_evaluator_test.go`
- `internal/types/answer_shape_runtime_test.go`
- `internal/analysis/compiler/compile_test.go`
- `internal/agent/extractor_test.go`
- `internal/agent/explorer_test.go`
- `internal/orchestrator/*shape* / *dag* / *regression*`

要做：

1. 删除所有 read-mode shape fixture
2. 测试改为：
   - `QuestionFamily`
   - `AnswerSemanticView`
   - `RequiredBlocks`
   - `FacetCoverage`
   - `AnswerDocumentV2`
3. 清理所有过时测试命名

---

## 9. 实施顺序建议

推荐顺序：

1. **先拆 write-mode**
2. **再去 `AnswerSurfacePlan.RequiredShape`**
3. **再删 `answer_shape_runtime.go`**
4. **再改 finalizer prompt**
5. **再改 extractor / explorer**
6. **再改 taxonomy / reviewer / hint**
7. **再去 summary cap 的 shape 依赖**
8. **最后删 `AnswerShape` 和 `RequiredAnswerShape`**
9. **最后清扫测试/注释/docs**

这个顺序的原因：

- 可以先拿掉最独立的 write-mode 残留
- 再切掉最关键的 runtime 传播点
- 最后再做全局 grep 归零

---

## 10. 完成后的审计要求

完成迁移后，必须按下面的多维度审计，不满足任何一条都不能算完成。

## 10.1 语义统一

- read-mode 不再存在 `AnswerShape`
- `QuestionFamily` 是唯一 coarse classifier
- `AnswerSemanticView` 是唯一 runtime semantic authority
- `AnswerDocumentV2` 是唯一 final answer carrier

## 10.2 Prompt 审计

- 无内部旧术语：
  - `target shape`
  - `shape requires`
  - `shape=value`
  - `shape=step_list`
- 无歧义
- 无互相矛盾的 block 要求
- 模型能看到完成任务所需的全部信息

## 10.3 Gate 审计

- 无 V1 shape gate 与 V2 block gate 同时约束同一 read-mode 输出
- 无 gate 相互冲突打架
- 无“一个阶段要求 A，另一个阶段要求 not-A”

## 10.4 泛化审计

- 方案不耦合特定仓
- 不耦合 YAML / JSON / 某个 config 文件名
- 不假设层级上限
- 不使用关键词表硬编码语义分类来替代 `QuestionFamily + SemanticView`

## 10.5 端到端消费审计

- 每个新增字段都有完整消费链
- 没有“只生成不消费”的字段
- 没有“只 validator 读、prompt 不读”或“prompt 读、runtime 不认”的断链

## 10.6 删旧审计

- 无死代码
- 无 read-mode V1 fallback
- 无只剩注释却未删逻辑
- 无只剩测试命名却已与 runtime 不符

## 10.7 质量审计

- 减少轮次不能以牺牲 richness 为代价
- 必须保持：
  - 关键因素不缺失
  - 该有图时有图
  - 机制解释与代码对应
  - uncertainty 边界诚实

## 10.8 grep 归零审计

终局要求：

- read-mode 主链中以下 grep 必须为 0：
  - `RequiredAnswerShape`
  - `AnswerShape`
  - `EffectiveRequiredAnswerShape`
  - `StableAbsentExactConfigRequiresExplanation`
  - `ExplanationAllowsAnchorSkeleton`
  - `shape requires`
  - `target shape`
  - `Re-emit with shape=`

---

## 11. 对不熟悉代码的开发者的上手建议

建议按这个阅读顺序进入代码：

1. `internal/types/facet_plan.go`
2. `internal/types/answer_semantic_view.go`
3. `internal/types/answer_semantic_view_compile.go`
4. `internal/types/answer_document_v2.go`
5. `internal/tool/emit_answer_document.go`
6. `internal/tool/emit_answer_document_v2.go`
7. `internal/render/answerdoc.go`
8. `internal/orchestrator/contract_check_block.go`
9. `internal/agent/answer_document_evaluator.go`
10. 再回头清理 shape 残留

不要一上来就全局替换 `AnswerShape`。正确做法是：

- 先看 semantic view 现在已经能表达什么
- 再把旧 shape 读点逐个迁到 semantic view helper
- 最后统一删掉旧枚举和旧测试

---

## 12. 最终一句话

当前远端代码已经解决了“最终答案必须 block-only”的大问题，但还没有完成“运行时语义彻底摆脱旧 shape 世界”这一步。下一阶段的正确工作，不是继续补 V1 shape 规则，而是把所有残留运行时职责迁到 `QuestionFamily -> AnswerSemanticView -> AnswerDocumentV2` 这条新主链上，然后强硬、成批、一次性删除旧 `shape` 世界。
