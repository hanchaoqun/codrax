# 用户意图保真专项审计（2026-05-17）

## 背景

本轮审计聚焦一个架构红线：系统只能保护用户意图、澄清模型输出、兜底展示，不能把自己的中间判断替换成用户真正想要的答案形态。

审计范围覆盖 read-mode 主链：

1. analyzer 后处理与 reconcile：`internal/agent/analyzer*.go`
2. question family 与 semantic view：`internal/types/facet_plan.go`、`answer_semantic_view_compile_*.go`
3. finalizer schema 投影与 pre-emit gate：`internal/tool/answer_document_dynamic_schema.go`、`answer_document_pre_emit_check.go`
4. answer document 渲染：`internal/render/answerdoc.go`、`mermaid_render.go`
5. repo/entity 展开与 scope 辅助策略：exact resolution、bucket inference、test-source 过滤等

本文件只记录统一审计结果，不做代码修复。后续按批次修，避免 case-by-case 打补丁。

## 判定口径

- **P0**：直接违反红线，使用用户问题/模型回答关键字匹配来驱动逻辑，或确定会把系统意图替代用户意图。
- **P1**：高概率导致最终答案形态、内容范围、展示顺序偏离用户意图，且会引发 finalizer 重试/返工。
- **P2**：有保护目的，但 gate/normalizer 太强，可能在边界场景压缩、改写或污染用户期望。
- **P3**：主要是文档、观测性或低概率边界，不优先修代码。

## 已确认的正向改进

1. `reconcileIntent` 已删除“附带 panic log 就强制 root_cause”的系统替代行为，当前信任 LLM 的 intent 判断。
2. 旧 `reconcileShape` 已删除，V2 主链不再依赖 legacy answer shape 强改最终答案形态。
3. `DiagramContract.RequiredKind` 已接入，用户明确要“时序图”时不会被系统改成 flowchart。
4. `ExactResolutionTargets` 只从 RawRequest 对齐的 mention lane 取 exact target，不把上下文实体偷偷提升成用户主目标。
5. table renderer 已支持“模型自己生成的 markdown 表格文本优先展示”，这是兼容 LLM 表达的正确方向。

## 问题清单

### UIP-001（P0）RawRequest capability-surface 关键字匹配会硬改意图

代码位置：`internal/agent/capability_surface.go::detectStageToolCapabilityQuery`、`reconcileStageToolCapabilitySurface`

当前行为：

- 当 analyzer 没有 `CapabilitySurfaceHint` 时，系统直接扫描 `rm.RawRequest` 中的 stage/tool mention。
- 命中后把 `Intent` 改为 `explain`，`PredicateAxis` 改为 `define`，`AnswerSubject` 改为 generic，`Scenario` 改为 generic，并注入 authority keywords。

风险：

- 这是用用户原文关键字匹配来驱动逻辑判断，违反“禁止使用关键字匹配用户问题做逻辑判断”的红线。
- 即便 stage/tool token 是精确领域词，当前实现仍会替代 analyzer 的 typed intent。

修复方向：

- 只信任 analyzer 产出的 `CapabilitySurfaceHint`。
- RawRequest matcher 只能作为 debug/advisory/telemetry，不能修改 RequestModel。
- 加结构测试：删除 hint 后即使 RawRequest 含 stage/tool token，也不得触发 reconcile。

### UIP-002（P1）QuestionFamily + SemanticView 会过早限定最终表达形态

代码位置：`internal/types/facet_plan.go::ResolveQuestionFamily`、`answer_semantic_view_compile_generic.go`

当前行为：

- `ResolveQuestionFamily` first-match wins，决定后续 block surface。
- `compileGeneric` 明确不允许 `BlockScalar / BlockDecision / BlockTable`，要求 scalar/table 类内容嵌进 summary prose。

风险：

- 用户的问题形态是多样的。若 analyzer 没有把“表格/决策/数值/多列对比”等表达意图正确落到 family，schema 层会把模型可表达的 block kind 收窄。
- 这会导致模型想用 markdown 表格、结构化表格或决策块时，被系统要求塞回 prose 或 label/text，形成信息压缩。

修复方向：

- 引入独立于 QuestionFamily 的 `PresentationContract`：table、raw_markdown_table、diagram、raw_mermaid、decision、scalar 等是用户表达偏好，不应完全绑定 family。
- SemanticView 应采用“family 默认 + presentation union”的开放策略：用户/LLM 明确需要某种展示能力时，schema 必须允许。
- Generic 不应天然排斥 table；至少允许 `BlockTable` optional，且不强迫 LLM 把表格塞进 summary。

### UIP-003（P1）动态 schema 投影把 block.kind / diagram 字段裁剪得过硬

代码位置：`internal/tool/answer_document_dynamic_schema.go::projectBlockKindEnum`、`projectDiagramField`、`projectRequiredBlockArrayCardinality`

当前行为：

- block.kind enum 被限制为 `RequiredBlocks ∪ OptionalBlocks`。
- 没有 `DiagramPlan` 时直接移除 `block.diagram` / `edge_anchors`。
- required block 使用 `minContains/maxContains` 约束数量。

风险：

- schema 是 LLM 可表达空间的硬边界，一旦上游 family/view 少算了用户表达需求，下游模型无法自救。
- `maxContains` 这类数量约束可能让模型为了过 schema 删除有价值章节。

修复方向：

- schema 投影区分“必需项”和“允许项”：必须仍严格，允许面要宽。
- 用户显式输出形态进入 PresentationContract 后，schema 必须 union 允许。
- block 数量上限默认软化，只有结构上必须唯一的块才使用 `maxContains`。

### UIP-004（P1）aggregate member_set 被当成最终答案主列表，导致 finalizer 反复返工

代码位置：`internal/tool/answer_document_pre_emit_check.go::preCheckAggregateMemberSetCoverage`、`normalizeAggregateMemberSetCarriers`、`internal/types/answer_aggregate_fact.go`

当前行为：

- model-emitted `member_set` 若被判为 principal，最终答案必须逐个可见包含成员。
- normalizer 会自动 materialize 缺失 carrier block/item。

风险：

- `member_set` 有时是探索覆盖账本，不一定是用户要求的主答案。
- 一旦误判 principal，finalizer 会被迫把探索过程成员逐项塞进可见答案，引发“member_set members are not rendered verbatim”类重试。
- 自动 materialize 会在模型完成输出后修改答案内容，存在系统替代模型表达的风险。

修复方向：

- 为 aggregate fact 增加明确 provenance/role：`principal_answer`、`supporting_coverage`、`audit_ledger`。
- hard gate 只读 `principal_answer`。
- normalizer 默认不新增可见主内容；需要新增时放入“系统补充/保留内容”隔离区，并记录 provenance。

### UIP-005（P1）surface_terms 自动追加会污染用户可见答案

代码位置：`internal/tool/answer_document_pre_emit_check.go::normalizeModelSurfaceTerms`、`appendSurfaceTermsToItem`

当前行为：

- 当 cited item 缺少 evidence `surface_terms` 时，系统会把“原文术语：...”追加到 item text。
- 随后 `preCheckModelSurfaceTerms` 只是 warning，不 hard reject。

风险：

- 这会把面向 grounding 的术语账本变成用户可见内容，出现用户看到的“模型抱怨 surface_terms / filename label”类问题。
- 追加文本不是用户意图，也不是 finalizer 自己组织出的表述。

修复方向：

- surface_terms 只作为 grounding metadata 或 reviewer hint。
- 不自动追加到主答案正文。
- 若必须展示，进入专门的证据详情/引用 tooltip/保留内容区，不污染正文 item。

### UIP-006（P1）Mermaid 渲染层可能把“源码意图”替换成 ASCII 图

代码位置：`internal/render/renderer.go::RenderMermaidBlocks` 调用、`internal/render/mermaid_render.go`

当前行为：

- REPL 默认把 `mermaid` fence 转成 ASCII text fence。
- single-shot CLI 默认保留源码，只有 `--mermaid-render` 才转换。

风险：

- 用户若明确要 Mermaid 源码，REPL 渲染层仍可能替换成 ASCII。
- 这是 render 层的系统展示意图覆盖用户输出形态。

修复方向：

- PresentationContract 加 `raw_mermaid_source`。
- REPL 渲染前读取该 contract，明确要源码时跳过 `RenderMermaidBlocks`。
- 对 unsupported Mermaid，继续保留源码并给出明显但隔离的渲染失败提示。

### UIP-007（P2）scenario / predicate reconcile 会为“降低开销”降级分析形态

代码位置：`internal/agent/analyzer_intent.go::reconcileScenario`、`internal/agent/analyzer_predicate.go::reconcileSemanticPredicates`

当前行为：

- scalar lookup、single-topic trace、failure-scope verdict 会把 scenario 降为 generic。
- 某些 exact-target / role-locate / single trace 场景会把 `IsCrossComponent` 降为 false。

风险：

- 这些规则大多是 typed signal，优于旧 keyword 方案；但理由中包含“avoid overhead”，说明系统成本/稳定性在影响答案形态。
- 若 analyzer 漏了用户明确的跨组件/架构意图，系统会进一步缩窄探索和输出。

修复方向：

- reconcile 只做“字段一致性修复”，不要承担成本优化。
- 成本优化进入 budget planner，且不得改变用户可见表达 contract。
- 对每个降级保留 analyzer decision，并在最终调试摘要里可追踪。

### UIP-008（P2）qualified code-symbol config drift 可能把配置问题改成架构比较

代码位置：`internal/agent/analyzer_code_symbol_reconcile.go::reconcileQualifiedCodeSymbolConfigDrift`

当前行为：

- 若 config-shaped 请求中至少两个 qualified entity 可解析为代码符号，系统会改 Intent/Scenario/Kind/Axis，并生成 buckets/sub_topics。

风险：

- 对“真的是配置 key，但名字刚好像 qualified symbol”的场景，系统可能把用户想查配置的问题改成机制/架构比较。
- 当前有 repo-grounded 保护，但仍是强改粗分类。

修复方向：

- 当用户/LLM 有明确 config exact target 或 exact context role 时，保留 config 主链，把 code-symbol 解析作为 supporting evidence。
- 若需要改链路，必须记录 shadow decision，并允许 analyzer typed hint 覆盖。

### UIP-009（P2）ChangeImpactProfile 会强制 set-valued enumeration surface

代码位置：`internal/agent/change_impact_reconcile.go::reconcileChangeImpactProfile`

当前行为：

- active `ChangeImpactProfile` 且 requested_output 是 files/sites/symbols 时，系统强制 `IntentEnumerate`、`ReqEnumeration`、relation/category predicates。

风险：

- 对“用户要步骤/解释，但 profile 误判成 affected files”的问题，会把答案变成列表。
- 该 profile 来自 LLM typed 输出，不是原文关键字，风险低于 UIP-001，但仍可能替代用户表达。

修复方向：

- 将 `requested_output` 与 `requested_presentation` 分离。
- 只在 profile confidence / user-requested output 明确时强制 principal set，否则作为 prompt guidance。

### UIP-010（P2）pre-emit gate 数量过多，部分 gate 承担了作者决策

代码位置：`internal/tool/answer_document_pre_emit_check.go::runPreEmitChecksWithContext`

当前行为：

- pre-emit 已不只是 block coverage / citation integrity，还包含 inactive scope、aggregate scalar/member、relation shape、label grounding 等大量 hard gate。

风险：

- gate 之间可能互相打架：schema 要少块，aggregate 要多成员；surface_terms 要原文术语，render 又要清晰表格。
- finalizer 返工多不是单点 bug，而是 downstream hard gate 过密，且有些 gate 读的是模型探索中间产物。

修复方向：

- 建立 gate taxonomy：Safety/Grounding hard，Presentation soft，Richness advisory。
- hard gate 必须只读 precise signal，且证明与用户意图直接相关。
- 增加 same-error-class retry governor：同类 gate 连续失败时降级为隔离展示/补充说明，而不是无限重写。

### UIP-011（P2）inactive subrepo disclosure 会把 workspace 拓扑塞进最终答案

代码位置：`internal/tool/answer_document_pre_emit_check.go::preCheckInactiveScopeDisclosure`

当前行为：

- 如果 `PendingSubRepos` 非空且答案 bounded，要求 final answer 披露 inactive scope。

风险：

- 保护目的正确：防止把 inactive scope 当作全局不存在。
- 但如果 PendingSubRepos 是旧状态或与用户问题弱相关，最终答案会出现“multi-repo workspace has inactive sub-repos ...”这类模型抱怨和用户噪音。

修复方向：

- disclosure 分级：直接命中用户目标时 hard，弱相关时 caveat advisory。
- 输出文案由系统渲染层生成，不让 finalizer 在正文里硬塞英文 gate 文案。

### UIP-012（P2）表格结构仍可能压缩信息

代码位置：`internal/render/answerdoc.go::renderV2BlockTable`、`renderV2StructuredTable`

当前行为：

- block text 中的 markdown 表格会原样优先渲染，这是正确方向。
- 结构化 table fallback 会使用“项目/说明”或 `Item/Details`。
- 当 cells 多于 headers 时会截断到 header 数量。

风险：

- 对用户多列问题，两列 fallback 会压缩信息。
- 默认标题不体现模型真实列语义，用户会看到无意义表头。

修复方向：

- 表格优先级：markdown table text > model columns/cells > 系统 fallback。
- fallback 不能截断；headers 不足时补 `列 2/列 3` 等稳定列名。
- 若 LLM 没给 columns，但 cells 是多列，renderer 根据最大 cells 数生成多列表格。

### UIP-013（P2）test/source 辅助过滤可能忽略用户明确关注测试代码

代码位置：`internal/agent/analyzer.go::expandPackageExports`、`expandChildPackages`、`isTestSourcePath`

当前行为：

- 包导出/子包扩展时默认跳过 test/spec/__tests__ 等路径。

风险：

- 这是合理的 production-bias，但如果用户明确问测试、fixtures、用例覆盖，系统不应跳过。
- 当前过滤没有显式读取 SourceScope contract。

修复方向：

- 引入 typed `SourceScope`：production_only、tests_only、include_tests、docs_only、all。
- test-source 过滤只在 `production_only` 下生效。

### UIP-014（P3）AnalyzerReconcileStrictMode 配置与注释已过期

代码位置：`internal/config/runtime.go::AnalyzerReconcileStrictMode`、`internal/agent/analyzer_intent.go::reconcileStrictMode`

当前行为：

- 注释仍说该开关控制 `reconcileShape` strict override。
- 但 `reconcileShape` 已删除。

风险：

- 这是观测性/运维风险。操作员以为可以关闭/恢复某类 override，实际语义已经变了。

修复方向：

- 删除无效配置，或改名为仍然真实生效的 reconcile 策略开关。
- 补 migration note，避免灰度排障误判。

## 分批修复建议

### Batch A：先消除明确红线

1. 移除 RawRequest capability-surface hard matcher。
2. 只允许 `CapabilitySurfaceHint` 触发 capability reconcile。
3. 给 raw matcher 留 telemetry，不改变 RequestModel。

### Batch B：建立 PresentationContract

1. 从 analyzer typed 输出承接用户明确要求：table、markdown table、diagram kind、raw Mermaid、sequenceDiagram、decision/scalar 等。
2. SemanticView/schema 使用 family default + presentation union。
3. renderer 读取 contract，避免把 raw source 转成 ASCII、避免两列表格压缩。

### Batch C：finalizer gate 分层治理

1. 将 pre-emit checks 分为 hard / soft / advisory。
2. aggregate member_set 增加 role/provenance，只对 principal answer hard gate。
3. surface_terms 不再自动追加正文。
4. 同类错误连续失败时降级为隔离补充展示，不再反复重写。

### Batch D：reconcile 只做一致性，不做意图重写

1. scenario/predicate/code-symbol/change-impact reconcile 加“显式用户表达保护”。
2. 成本优化移到 budget planner，不改变 answer contract。
3. 所有强改记录 shadow decision，并在最终日志摘要可见。

### Batch E：render 与 scope 统一收口

1. 表格 fallback 不截断，多列稳定展示。
2. inactive subrepo disclosure 改成系统生成的隔离 caveat。
3. SourceScope 控制 production/test/docs 过滤。

## 结论

当前系统已经修掉了若干历史上最明显的“系统替用户决定答案形态”的点，但仍有一批更隐蔽的问题：schema 过早收窄、pre-emit gate 过密、normalizer 自动补正文、render 层替换输出形态。这些问题共同解释了 finalizer 阶段反复“校验未通过 / 答案待完善，正在重写”的根因：模型不是单纯不听话，而是在多个系统层的硬约束之间被迫折返。

下一步应优先做 Batch A + Batch B，再做 Batch C。否则继续单点修 validator 文案或补 prompt，只会压住一头、另一头再冒出来。
