# 用户意图保真专项审计（2026-05-17）

## 背景

本轮审计聚焦一个架构红线：系统只能保护用户意图、澄清模型输出、兜底展示，不能把自己的中间判断替换成用户真正想要的答案形态。

审计范围覆盖 read-mode 主链：

1. analyzer 后处理与 reconcile：`internal/agent/analyzer*.go`
2. question family 与 semantic view：`internal/types/facet_plan.go`、`answer_semantic_view_compile_*.go`
3. finalizer schema 投影与 pre-emit gate：`internal/tool/answer_document_dynamic_schema.go`、`answer_document_pre_emit_check.go`
4. answer document 渲染：`internal/render/answerdoc.go`、`mermaid_render.go`
5. repo/entity 展开与 scope 辅助策略：exact resolution、bucket inference、test-source 过滤等

本文件记录统一审计结果与持续修复进展。后续按批次修，避免 case-by-case 打补丁。

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

## 2026-05-17 批次进展

1. **Batch A 已落地**：`detectStageToolCapabilityQuery` 只读取 analyzer 结构化 `CapabilitySurfaceHint`；RawRequest stage/tool 扫描已拆成 advisory 函数，不再进入 reconcile、required-files、explorer、finalizer 能力面硬链路。新增结构测试覆盖“原文含 stage/tool token 但无 hint 时不得改写 RequestModel”。
2. **Batch B 第一段已落地**：Generic 语义视图仍只强制 Summary，但已允许可选 `BlockTable` / `BlockScalar` / `BlockDecision`，避免 schema 把模型合理的表格、数值、结论表达挤回 prose。渲染层新增对 item 文本内 markdown table 的保留能力，避免模型表格被压成两列 fallback。
3. **Batch C 第一段已落地**：`surface_terms` 不再自动追加到用户可见正文；缺失 surface term 只保留为 advisory 日志，不触发 hard retry，也不污染最终答案 item text。
4. **第二轮审计完成（文档刷新）**：继续沿 read-mode 主链排查仍会替代用户/LLM 表达的机制，新增 UIP-015 到 UIP-021。结论是后续不能继续补 validator 文案，而要优先治理“系统补正文、schema 兼容、gate 分层、upstream repair routing”四条主线。
5. **Batch F/G/H 第一段已落地**：`emit_answer_document` 与 patch 持久化路径不再 materialize aggregate/principal support 成员到用户可见正文；新增 answer-document 字段隔离层，schema-unknown 的无害 metadata 在严格 decode 前本地 quarantine，保留核心 blocks/citations，避免小错触发 finalizer 重试。`aggregate member_set` emit-time coverage 只在 typed exhaustive/relation enumeration 下 hard gate，叙事/解释类 member_set 降为 advisory。
6. **Batch H 第二段已落地**：semantic-quality reviewer 的 concern schema 增加 `repair_locus`，并由 violation 的 typed `RepairLocusOverride` 进入 repair routing。`evidence_gap` 回流 explore，`analysis_gap` / `presentation_advisory` 不再触发 finalizer-only 硬重写，`local_doc_defect` / `safety` 才留在本地修正文档路径。
7. **Batch C/H 第三段已落地**：facet metadata gate 分层完成第一刀。`FacetRequirement` 新增 `RequiresHardDeclaration()`，pre-emit 只对 template-hard facet 做同轮拒绝；evidence-sufficient SOFT facet 只在 prompt 中标记为 `(evidence available)`，不再写成 “MUST declare / emit-time rejection”。semantic-quality depth audit 同步只看 emit-hard facet，避免 reviewer 把 soft metadata 变成 finalizer rewrite。另修 analyzer-derived `must_include`：`analyzer_entity` 符号不能只因全仓 `SymbolOracle` 命中就成为 hard requirement，必须在本轮 typed evidence / answer symbol 中有支撑，否则降级。
8. **Batch E/F 第一段补齐**：inactive subrepo disclosure 不再作为 pre-emit hard retry 或默认 finalizer rewrite。缺失时保留 typed telemetry，并由 orchestrator 在系统“补充说明”通道追加范围说明；模型正文不再被迫写 `scope_disclosure` / inactive RootRel，避免系统工作区拓扑污染用户答案。
9. **Batch H 第三段落地**：新增 same-error-class retry governor。除既有 per-kind / per-root / hard-cap 外，调度层按 typed `FallbackTarget + CaveatFamilyID` 记录更高层的错误类预算；同类机械问题已重试后即使换成 sibling kind，也停止继续 finalizer rewrite，交付当前答案并转入补充说明。该判断只读结构化 violation registry，不扫描用户问题或模型散文。
10. **Batch C 第二段落地**：`aggregate_facts` 增加 typed `role/provenance`，支持 `principal_answer`、`supporting_coverage`、`audit_ledger`。explorer 可以明确区分“最终答案主集合”和“探索覆盖账本”；finalizer hard gate 只消费有效角色为 principal 的 aggregate fact，prompt 与 retry 合并也保留该结构化角色，不再用伪标记或标签猜测。
11. **Batch H 第四段落地**：reviewer anchor set 统一改为 `buildReviewerEvidenceAnchorSet`，优先读取最终 AnswerDocument 的行级 citation、read_file gutter 与 repomap 符号，再合并 explorer evidence；self-consistency 与 semantic-quality reviewer 不再只看 explorer evidence。Mermaid endpoint gate 同步区分内部 node id 与用户可见 label，避免把图表语法 alias 当代码事实打回 finalizer。新增回归测试锁住 citation-backed identifier、graph fallback、semantic/self reviewer 输入和 Mermaid label 行为。
12. **Batch B/E 边界澄清**：REPL Mermaid→ASCII 属于终端呈现层友好降级，不再作为“系统替代模型意图”整改项；边界是 raw markdown 必须仍作为 truth source 写入日志、output dump、memory，且该渲染结果不得进入 gate / reviewer / 后续模型上下文。已补测试锁住 pipeline REPL 记忆保存 raw Mermaid，而不是终端渲染后的 text fence。
13. **Batch G 第二段落地**：annotation JSON 兼容层新增唯一可恢复的 `claim_uses[].facet_ids` → `claim_uses[].facet_id` 归一化。full emit 与 patch 共用同一 repair；单值自动修复，避免小 schema 错误触发 finalizer 重试；多值保持拒绝，防止系统替模型选择 facet。新增 full/patch 回归测试。
14. **Batch B 第二段落地**：新增 family-independent `AnswerPresentationContract`。schema 现在可在不增加 finalizer prompt 负担的前提下允许 table/scalar/decision 展示载体；用户显式 diagram contract 跨所有 QuestionFamily 生效，config/role/enumeration/comparison 等旧“无图 family”不再吞掉时序图/流程图需求。软 diagram preference 仍不升级为硬要求，避免证据偏好替代用户意图。
15. **Batch G 第三段落地**：schema-aware tool-param 兼容层新增 string enum JSON-literal 修复。若 schema 声明字段是 `string enum`，模型却发出 `"\"explain\""` / `"\"\""` 这类双重 JSON 字符串，系统会在本地无损解包；解包后仍不属于 enum 的值继续交给工具 gate 拒绝。该修复覆盖 `emit_analysis.intent/scenario/complexity/question_kind/language/predicate_axis` 等字段，避免 analyze 阶段因机械参数形态返工，也避免静默降级为 `unknown/generic`。

## 问题清单

### UIP-001（P0）RawRequest capability-surface 关键字匹配会硬改意图

代码位置：`internal/agent/capability_surface.go::detectStageToolCapabilityQuery`、`reconcileStageToolCapabilitySurface`

状态：**已修复（Batch A）**。硬链路只信任 analyzer 产出的 `CapabilitySurfaceHint`；RawRequest matcher 仅保留为 advisory/debug，不修改 RequestModel。

历史行为：

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

代码位置：`internal/types/facet_plan.go::ResolveQuestionFamily`、`answer_semantic_view_compile_generic.go`、`answer_presentation_contract.go`、`internal/tool/answer_document_dynamic_schema.go`

状态：**部分修复（Batch B 第一段 + 第二段）**。Generic 已允许可选 table/scalar/decision；现新增独立 `AnswerPresentationContract`，由 typed contract 编译展示载体并投影到动态 schema。table/scalar/decision 作为 schema-only 展示 affordance，不额外塞进每个 family 的 prompt；用户显式 diagram contract 会跨所有 family 生成 required diagram block 与 DiagramPlan。

历史行为：

- `ResolveQuestionFamily` first-match wins，决定后续 block surface。
- `compileGeneric` 明确不允许 `BlockScalar / BlockDecision / BlockTable`，要求 scalar/table 类内容嵌进 summary prose。

风险：

- 用户的问题形态是多样的。若 analyzer 没有把“表格/决策/数值/多列对比”等表达意图正确落到 family，schema 层会把模型可表达的 block kind 收窄。
- 这会导致模型想用 markdown 表格、结构化表格或决策块时，被系统要求塞回 prose 或 label/text，形成信息压缩。

修复方向：

- 引入独立于 QuestionFamily 的 `PresentationContract`：table、raw_markdown_table、diagram、raw_mermaid、decision、scalar 等是用户表达偏好，不应完全绑定 family。
- SemanticView 应采用“family 默认 + presentation union”的开放策略：用户/LLM 明确需要某种展示能力时，schema 必须允许。
- Generic 不应天然排斥 table；至少允许 `BlockTable` optional，且不强迫 LLM 把表格塞进 summary。
- 防回归：presentation union 只读取 typed contract / deterministic display affordance，不读 RawRequest 或模型散文关键字；schema 放宽不等于新增 hard gate。

### UIP-003（P1）动态 schema 投影把 block.kind / diagram 字段裁剪得过硬

代码位置：`internal/tool/answer_document_dynamic_schema.go::projectBlockKindEnum`、`projectDiagramField`、`projectRequiredBlockArrayCardinality`

当前行为：

- block.kind enum 被限制为 `RequiredBlocks ∪ OptionalBlocks`。
- 没有 `DiagramPlan` 时直接移除 `block.diagram` / `edge_anchors`。
- required block 使用 `minContains/maxContains` 约束数量。

状态：**部分修复（Batch B 第二段）**。block.kind enum 已扩大到 `RequiredBlocks ∪ OptionalBlocks ∪ Presentation.AllowedBlocks`；presentation-only block 不进入 optional prompt，降低 LLM 心智负担。`BlockDiagram` 仍必须有 `DiagramPlan` 才进入 schema，避免“允许 diagram kind 但删除 diagram payload”的自相矛盾。用户显式 required diagram 会在所有 family 生成 DiagramPlan。

风险：

- schema 是 LLM 可表达空间的硬边界，一旦上游 family/view 少算了用户表达需求，下游模型无法自救。
- `maxContains` 这类数量约束可能让模型为了过 schema 删除有价值章节。

修复方向：

- schema 投影区分“必需项”和“允许项”：必须仍严格，允许面要宽。
- 用户显式输出形态进入 PresentationContract 后，schema 必须 union 允许。
- block 数量上限默认软化，只有结构上必须唯一的块才使用 `maxContains`。

### UIP-004（P1）aggregate member_set 被当成最终答案主列表，导致 finalizer 反复返工

代码位置：`internal/tool/answer_document_pre_emit_check.go::preCheckAggregateMemberSetCoverage`、`normalizeAggregateMemberSetCarriers`、`internal/types/answer_aggregate_fact.go`

状态：**已修复（Batch C 第二段）**。`AnswerAggregateFact` 新增 `role/provenance`，`emit_investigation_complete` schema 暴露 typed role；legacy payload 由 `RequestModel` 结构信号推断有效角色。`PrincipalAggregateMemberSetFactRefs*`、pre-complete exhaustive/relation/change-impact gate、completion principal-term coverage、finalizer aggregate prompt 均只把有效 `principal_answer` 当作主答案集合。`supporting_coverage` / `audit_ledger` 可作为上下文与运维账本保留，但不会强迫最终答案逐项渲染。

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

历史代码位置：`internal/tool/answer_document_pre_emit_check.go::normalizeModelSurfaceTerms`、`appendSurfaceTermsToItem`

状态：**已修复（Batch C 第一段）**。自动追加正文的 normalizer 已移除调用与实现；`preCheckModelSurfaceTerms` 只保留 advisory 日志。

历史行为：

- 当 cited item 缺少 evidence `surface_terms` 时，系统会把“原文术语：...”追加到 item text。
- 随后 `preCheckModelSurfaceTerms` 只是 warning，不 hard reject。

风险：

- 这会把面向 grounding 的术语账本变成用户可见内容，出现用户看到的“模型抱怨 surface_terms / filename label”类问题。
- 追加文本不是用户意图，也不是 finalizer 自己组织出的表述。

修复方向：

- surface_terms 只作为 grounding metadata 或 reviewer hint。
- 不自动追加到主答案正文。
- 若必须展示，进入专门的证据详情/引用 tooltip/保留内容区，不污染正文 item。

### UIP-006（P3）Mermaid 终端渲染是允许的展示层例外，但 raw truth 必须保留

代码位置：`internal/render/renderer.go::RenderMermaidBlocks` 调用、`internal/render/mermaid_render.go`

状态：**改判为允许例外，边界已固化（Batch B/E）**。REPL 当前无法直接渲染 Mermaid 图，终端层把 Mermaid fence 转成 ASCII/text fence 是为了用户可读性，允许保留。该行为不得成为 finalizer gate、reviewer 输入或后续模型上下文的事实来源；raw markdown 仍是唯一答案真源。pipeline output dump 已由 orchestrator 写 raw final answer；local/chitchat markdown transcript 也写 raw reply；本批补齐 pipeline REPL memory raw 保存。

当前行为：

- REPL 默认把 `mermaid` fence 转成 ASCII text fence。
- single-shot CLI 默认保留源码，只有 `--mermaid-render` 才转换。

风险边界：

- 允许：仅在终端呈现时做 Mermaid→ASCII/text fence，让用户不依赖外部渲染器也能看懂。
- 禁止：把终端渲染后的 ASCII/text fence 写回 memory、output dump、reviewer 输入、finalizer retry/gate，或让模型在下一轮把它当成原始答案。

防回归措施：

- `Mutable.Result()` / output dump / log INFO 保持 raw markdown。
- REPL pipeline memory 保存 raw markdown；终端 response 只用于当前屏幕展示。
- 已加测试：`TestPipelineMemoryStoresRawMarkdownNotTerminalMermaidRender`。

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

状态：**部分修复（Batch C/H 第三段 + Batch E/F 第一段 + Batch H 第三段）**。facet metadata gate 已分出 emit-hard 与 advisory：soft facet 即使有证据，也不再触发 pre-emit hard retry；inactive-scope 缺失改由系统补充说明；same-error-class retry governor 已阻断同一 typed repair family 的 sibling-kind 轮转重试。后续仍需继续把其它 pre-emit gate 纳入统一 taxonomy。

当前行为：

- pre-emit 已不只是 block coverage / citation integrity，还包含 inactive scope、aggregate scalar/member、relation shape、label grounding 等大量 hard gate。

风险：

- gate 之间可能互相打架：schema 要少块，aggregate 要多成员；surface_terms 要原文术语，render 又要清晰表格。
- finalizer 返工多不是单点 bug，而是 downstream hard gate 过密，且有些 gate 读的是模型探索中间产物。

修复方向：

- 建立 gate taxonomy：Safety/Grounding hard，Presentation soft，Richness advisory。
- hard gate 必须只读 precise signal，且证明与用户意图直接相关。
- [x] 增加 same-error-class retry governor：同类 gate 连续失败时降级为隔离展示/补充说明，而不是无限重写。

### UIP-011（P2）inactive subrepo disclosure 会把 workspace 拓扑塞进最终答案

代码位置：`internal/tool/answer_document_pre_emit_check.go::preCheckInactiveScopeDisclosure`

状态：**第一段已修复（Batch E/F）**。pre-emit 不再拒绝缺失 inactive-scope disclosure；`ViolInactiveScopeDisclosureMissing` 默认 soft，并由 orchestrator append 系统补充说明。用户仍能看到边界，但模型正文不再被系统拓扑强行改写。

当前行为：

- 如果 `PendingSubRepos` 非空且答案 bounded，要求 final answer 披露 inactive scope。

风险：

- 保护目的正确：防止把 inactive scope 当作全局不存在。
- 但如果 PendingSubRepos 是旧状态或与用户问题弱相关，最终答案会出现“multi-repo workspace has inactive sub-repos ...”这类模型抱怨和用户噪音。

修复方向：

- [x] disclosure 分级：默认作为系统 caveat advisory，不触发 finalizer rewrite。
- [x] 输出文案由系统通道生成，不让 finalizer 在正文里硬塞英文 gate 文案。
- [ ] 后续继续细分：若未来需要严格模式，只对用户明确要求“全工作区不存在/全仓覆盖”的安全场景才允许提升为 hard。

### UIP-012（P2）表格结构仍可能压缩信息

代码位置：`internal/render/answerdoc.go::renderV2BlockTable`、`renderV2StructuredTable`

状态：**部分修复（Batch B 第一段）**。表格优先级已经覆盖 block text markdown table、item text markdown table、columns/cells 结构化行；headers 不足时按最大 cells 补稳定列名。后续仍需由 `PresentationContract` 把“允许 raw markdown table / table block”从所有 relevant family 统一下发。

当前行为：

- block text 中的 markdown 表格会原样优先渲染，这是正确方向。
- item text 中的 markdown 表格也会原样优先渲染，避免 citation carrier 把表格压回两列。
- 结构化 table fallback 会使用“项目/说明”或 `Item/Details`。
- 当 cells 多于 headers 时会按最大 cells 数补稳定列名，不再截断。

风险：

- 对用户多列问题，两列 fallback 会压缩信息。
- 默认标题不体现模型真实列语义，用户会看到无意义表头。

修复方向：

- 表格优先级：markdown table text > model columns/cells > 系统 fallback。
- fallback 不能截断；headers 不足时补 `列 2/列 3` 等稳定列名。
- 若 LLM 没给 columns，但 cells 是多列，renderer 根据最大 cells 数生成多列表格。

### UIP-013（P2）test/source 辅助过滤可能忽略用户明确关注测试代码

代码位置：`internal/agent/analyzer.go::expandPackageExportsWithSourceScope`、`expandChildPackagesWithSourceScope`、`internal/types/source_path.go::ClassifySourcePathRole`

状态：**已修复（Batch I 第一段）**。`SourceScopeProfile` 已贯穿 package export / child package expansion；运行时不再使用 analyzer 局部 `isTestSourcePath` 过滤，统一走 `types.ClassifySourcePathRole` + `types.SourceScopeAllowsPathRole`。默认 production scope 仍过滤测试/文档/fixture/example/prompt-support；typed test/docs/all scope 会把对应路径恢复为 principal expansion 候选。

当前行为：

- 包导出/子包扩展时默认跳过 test/spec/__tests__ 等路径。
- `keyword_search` 已能尊重 `SourceScopeProfile`，但 `expandPackageExports` / `expandChildPackages` 尚未贯穿该 profile。

风险：

- 这是合理的 production-bias，但如果用户明确问测试、fixtures、用例覆盖，系统不应跳过。
- 过滤链路不一致会制造“搜索能看到，包展开看不到”的上下游分裂，后续 finalizer 可能被迫用不完整 evidence 回答。

修复方向：

- 引入 typed `SourceScope`：production_only、tests_only、include_tests、docs_only、all。
- test-source 过滤只在 `production_only` 下生效。
- 把同一个 `SourceScopeProfile` 贯穿 exact target、keyword search、package export、child package expansion，避免某一层替用户缩窄范围。

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

## 2026-05-17 第二轮新增审计

### UIP-015（P1）principal support / aggregate materializer 仍会替模型补可见正文

代码位置：`internal/tool/answer_document_pre_emit_check.go::normalizePrincipalSupportMemberCarriers`、`normalizeAggregateMemberSetCarriers`、`internal/tool/answer_document_mutation_runtime.go::persistMergedAnswerDocument`

状态：**第一段已修复（Batch F）**。full emit / patch persist 路径已移除 materializer 调用；两个 legacy normalizer 函数保留为 no-op 兼容 shim，并有测试确保不会新增 block/item/citation。

当前行为：

- principal support 或 aggregate `member_set` 的可见 carrier 缺失时，normalizer 会追加 block/item，把 support 成员补进最终答案正文。
- 这些内容可能来自探索账本、支撑事实或校验辅助结构，不一定是模型想呈现给用户的主答案。

风险：

- 这是系统在模型 emit 之后继续“写正文”，违反“系统不能改变模型建议”的边界。
- 自动补出的列表会让最终答案变长、变硬、变噪，也会与 schema/table/render 产生新的冲突，诱发 finalizer 重写。

修复方向：

- 默认禁止 normalizer 新增 principal 可见正文；normalizer 只能做无争议的结构修复、引用修复、无损搬运。
- 若确需保留缺失的 support/aggregate 信息，进入隔离的“系统补充/保留内容”区域，并明确 provenance。
- hard gate 只针对 role=`principal_answer` 的事实；`supporting_coverage`、`audit_ledger` 只能 advisory。

### UIP-016（P1）emit_answer_document strict unknown-field decode 仍缺少泛化兼容层

代码位置：`internal/tool/emit_answer_document_v2.go::executeAnswerDocumentV2`、`answerDocumentV2MisplacedHints`

状态：**部分修复（Batch G 第一段 + 第二段）**。新增 `answer_document_field_quarantine.go`，对 full emit / patch 的 top-level、block、item、citation、snippet、diagram、claim_use、edge_anchor、exact_resolution 等已知结构容器执行 schema-aware quarantine。保留 `value/boolean` 等可见 payload 错位字段和 claim/edge 可恢复错位字段给 strict remap，避免静默丢内容。第二段进一步把唯一可恢复的 `claim_uses[].facet_ids=["x"]` 本地归一化为 `claim_uses[].facet_id="x"`；`facet_ids` 多值仍 hard reject，因为系统无法不改变模型意图地替它选一个 facet。

当前行为：

- 参数 JSON 经过有限 repair 后仍使用 `DisallowUnknownFields` 严格 decode。
- 对已知错位字段依赖手写表修复，例如 citation、facet、table、edge anchor 等字段。

风险：

- 模型一个轻微错位字段就会触发 hard retry，例如字段放到相邻容器、旧 schema 字段残留、metadata 层级错位。
- 继续扩手写 repair map 会变成 case-by-case 追 bug，且把 LLM 心智消耗在 schema 机械细节上。

修复方向：

- 增加 schema-aware tolerant quarantine：能唯一归位的字段自动搬运，不能归位但无害的 metadata 放入 diagnostics 并丢弃，不破坏核心文档。
- 只有语义载体冲突、版本 carrier 冲突、安全/引用不可恢复时才 hard reject。
- 为 top-level/misplaced `claim_uses`、`facet_ids`、`edge_anchors`、table 字段建立泛化回归测试，而不是逐案补提示。
- 防回归：schema repair 只处理“唯一可恢复”的结构错位；一旦存在多值、冲突 alias 或语义选择，保持 strict reject，让模型明确决定。

### UIP-017（P1）enumeration label grounding 对展示标签过硬

代码位置：`internal/tool/answer_document_pre_emit_check.go::preCheckEnumerationLabelGrounding`

状态：**第一段已修复（Batch H）**。`runPreEmitChecks` 只在 typed exhaustive / relation enumeration 或 analyzer principal category member lane 下把 label grounding 作为 hard gate；叙事说明、优缺点对比、表格维度等场景降为 advisory，不再同轮强制 finalizer 重写。

当前行为：

- 对 ordered/bullet/table block 的 item label 抽取 leading identifier，走 symbol oracle 校验。
- 不可解析时会要求 finalizer 改 label 或补 grounding。

风险：

- 用户可见 label 不总是代码符号；它可能是表格维度、章节名、中文概念、比较对象或模型为了 UX 写的短标题。
- 把展示 label 当作代码锚点 hard gate，会迫使模型把清晰标题改成源码路径/符号串，最终用户体验变差。

修复方向：

- 只有 typed view 明确声明“principal code-identity enumeration”时才 hard 校验 label grounding。
- 对 citation 已满足、但 label 非代码锚点的情况降为 advisory。
- 若要暴露未解析符号，放入补充说明或证据诊断，不要求整个答案重写。

### UIP-018（P1）semantic reviewer 可把 reviewer 关注点直接升级成 finalizer rewrite

代码位置：`internal/orchestrator/contract_check.go::runSemanticQualityReview`、`internal/agent/answer_document_evaluator.go::retryRepairPhaseForViolation`

状态：**第一段已修复（Batch H）**。`emit_semantic_quality_review.concerns[]` 增加结构化 `repair_locus`，运行时不再仅凭 concern kind 把问题全部压给 finalizer。`evidence_gap` 设置 typed `RepairLocusOverride=LocusExplore`，`analysis_gap` / `presentation_advisory` 设置 terminal/soft caveat 路径，只有 `local_doc_defect` / `safety` 保留本地重写。后续仍需做 same-error-class governor，把重复同指纹失败更早转成隔离补充说明。

当前行为：

- reviewer 产出的 structured concerns 会被转换为 contract violations。
- `answer_topic_mismatch` 等 repair hint 会直接要求 finalizer 围绕主题重写答案。

风险：

- reviewer 发现的问题可能是上游 evidence/analysis/explore 缺口，不一定是 finalizer 文档局部缺陷。
- 如果缺口来自上游，继续让 finalizer rewrite 会制造“答案待完善，正在重写”的循环，而不是补齐输入。

修复方向：

- reviewer concern 必须带 `repair_locus`：`analysis_gap`、`evidence_gap`、`local_doc_defect`、`presentation_advisory`、`safety`。
- 只有 `local_doc_defect` / `safety` 进入 finalizer rewrite；`evidence_gap` 回流 explore/extract；presentation 问题优先本地容错或补充说明。
- 增加 same-error-class governor：同一 fingerprint 连续失败后，接受核心答案并隔离展示缺陷说明，停止硬重写。

### UIP-019（P2）SourceScopeProfile 未贯穿包展开路径

代码位置：`internal/agent/analyzer.go::expandPackageExports`、`expandChildPackages`、`internal/agent/keyword_search.go::shouldDeprioritizeAuxiliaryBySourceScope`

状态：**已修复（Batch I 第一段）**。`expandEntitiesWithImplementers` 从 `RequestModel.SourceScopeProfile`（含 ChangeImpact/Profile fallback）取 typed scope，调用 `expandPackageExportsWithSourceScope` / `expandChildPackagesWithSourceScope`。包展开与 keyword search 现在共用同一个 source-role classifier，避免一个入口看见测试/文档、另一个入口丢掉测试/文档。

当前行为：

- keyword search 已可按 source scope 降权 auxiliary source。
- package export 和 child package expansion 仍由 `isTestSourcePath` 无条件过滤测试相关路径。

风险：

- 同一个用户请求在不同探索入口得到不同范围，导致 evidence plan 和最终答案范围不一致。
- 用户明确要测试、fixtures、bench、docs 时，系统仍可能在包展开阶段替用户缩小范围。

修复方向：

- 把 `SourceScopeProfile` 作为 package expansion 的显式输入。
- production-only 才跳过测试；tests-only/include-tests/all/docs-only 按各语言后缀和目录规则保留对应文件。
- 所有语言路径统一通过 source role classifier，不在单个 expansion 函数里散落判断。

### UIP-020（P2）Raw scope pre-injection 仍由用户原文 token 驱动

代码位置：`internal/agent/scope_projection.go::detectScopesFromQuestion`、`internal/agent/analyzer.go::buildAnalyzerRepoOverview`

当前行为：

- analyzer 前置 repo overview 会用用户原文匹配 active sub-repo `RootRel`，提前注入 multi-scope task map。
- 该结果目前主要用于上下文和提示，不直接决定 finalizer 主答案。

风险：

- 这是 advisory 层，但仍属于 RawRequest token 对 scope 上下文的预选择。
- 若后续有调用方把这个结果提升为 hard routing/status，就会再次触碰“关键词匹配用户问题做逻辑判断”的红线。

修复方向：

- 明确标记为 L0 advisory，不参与 hard routing、status truth、finalizer gate。
- analyzer emit 后用 typed `PrimaryScopes` / `SubTopic.Scopes` 重算 scope projection。
- 为“raw detector 只能影响 pre-prompt、不能写入 RequestModel hard fields”加结构测试。

### UIP-021（P3）REPL Mermaid 渲染边界：terminal-only，不进模型上下文

代码位置：`internal/repl/repl.go::renderRichResponse`、`internal/render/mermaid_render.go::RenderMermaidBlocks`、`cmd/root.go` 的 `--mermaid-render`

状态：**改判为展示层例外，非整改阻断项（Batch B/E）**。无需为了“用户要求源码”关闭 REPL 的友好 ASCII 预览；用户需要复制源码时，应以 raw markdown transcript / output dump / memory 中的原文为准。后续若增加真正的图形预览，可以作为 renderer capability，而不是改变模型输出或 finalizer gate。

当前行为：

- CLI 默认保留 Mermaid source，显式 `--mermaid-render` 才转换。
- REPL rich response 仍会默认调用 `RenderMermaidBlocks`，把 Mermaid fence 转成 ASCII text fence 或渲染警告。

风险边界：

- 终端展示可变，答案真源不可变。
- 不允许 renderer 结果参与 gate、reviewer、retry 或 memory context。
- 不允许渲染失败触发 LLM-facing hard retry；失败只能转 text fence/警告，且保持 raw markdown 可追溯。

后续方向：

- 保持现有 terminal-only ASCII 预览。
- 优先完善 raw transcript / preview 能力，让用户能拿到原始 Mermaid。
- 若未来引入 `PresentationContract`，它应描述“用户希望答案包含 Mermaid/表格/决策块”等模型输出偏好，而不是禁止 REPL 做终端友好展示。

### UIP-022（P1）reviewer anchor set 滞后于最终 AnswerDocument citations

代码位置：`internal/orchestrator/contract_check.go::runSelfConsistencyReviewV2`、`runSemanticQualityReview`、`buildReviewerEvidenceAnchorSet`

状态：**已修复（Batch H 第四段）**。self-consistency 与 semantic-quality reviewer 共用 `buildReviewerEvidenceAnchorSet`，anchor set 构建顺序为：最终 AnswerDocument 行级 citations → read_file gutter 行文本中的代码标识符 → repomap graph 中覆盖该行的符号 → TurnA / emitted evidence。这样 reviewer 看到的“可见锚点白名单”与最终答案真正引用的行保持一致。

历史行为：

- reviewer 只通过 `BuildEvidenceAnchorSet(mut.EmittedEvidence())` 获取 explorer evidence 的 `anchor_symbol/subject/object`。
- finalizer 后续补入或修正的 `citations[]` 不进入 reviewer anchor set。
- 当答案正文引用的常量/函数已经有行号 citation，但 explorer evidence 没把它放进 anchor_symbol 时，reviewer 会把真实标识符误判为 `fabricated_identifier`，触发“检测到前后不一致，正在重写答案”。

风险：

- 这是软 reviewer 读了不完整系统视图后升级成 finalizer rewrite，属于系统侧输入短板，不应让模型返工。
- 同类风险不只在 self-consistency；semantic-quality reviewer 也会根据 anchor set 做覆盖判断，必须使用同一真源。

防回归措施：

- runtime 禁止直接在 reviewer 路径调用 `BuildEvidenceAnchorSet(mut.EmittedEvidence())` 作为唯一 anchor 真源；必须走 `buildReviewerEvidenceAnchorSet`。
- 已加测试：`TestRunSelfConsistencyReviewV2_InputIncludesCitationLineIdentifier`、`TestRunSemanticQualityReview_InputIncludesCitationLineIdentifier`、`TestBuildReviewerEvidenceAnchorSet_CitationLineIdentifiersWinCap`、`TestBuildReviewerEvidenceAnchorSet_GraphSymbolBackfillsUnreadCitationLine`、`TestReviewerRuntimeUsesCitationAwareAnchorSet`。
- 后续新增 reviewer 时必须声明输入 truth source：若 reviewer 会影响 rewrite/routing，必须消费 AnswerDocument citations + typed evidence 的并集；若只消费 typed evidence，只能 advisory。

### UIP-023（P1）Mermaid endpoint gate 混淆内部 node id 与用户可见 label

代码位置：`internal/orchestrator/contract_check_block.go::parseMermaidEdges`、`validateDiagramEdgeEndpointHallucination`

状态：**已修复（Batch H 第四段）**。Mermaid parser 现在保留每个 endpoint 的内部 id 与显示 label；code-context endpoint hard gate 优先校验用户可见 label，只有没有显式 label 的裸节点才校验 endpoint id。

历史行为：

- Mermaid `check_size["行数检查\nDEFAULT_READ_LIMIT=2000"]` 这类节点会被解析成 endpoint=`check_size`。
- `validateDiagramEdgeEndpointHallucination` 在 call_dag / typed call edge 下把 `check_size` 当成代码符号查 SymbolOracle。
- 这会把图表内部 alias 判为 fabricated identifier，迫使模型重写图，甚至把清晰的中文/常量标签改差。

风险：

- Mermaid node id 是渲染语法，不等于用户答案的事实表述。
- gate 把 syntax carrier 当 answer claim，会替模型改变展示方式，属于“系统意图替代用户/模型输出”的表现层问题。

防回归措施：

- hard gate 只能校验用户可见事实面：显式 label、typed `edge_anchors`、citation-backed code identities。内部 node id 只在裸节点时才作为可见事实处理。
- 已加测试：`TestFixF_CallDAGKind_ExplicitDisplayLabelSkipsInternalAlias` 保证内部 alias 不触发重写；`TestFixF_CallDAGKind_ExplicitCodeLabelStillFires` 保证真实可见伪造标识符仍会被拦截。
- 后续 diagram validator 新增能力时必须先回答“校验对象是否用户可见”。若答案是否定，默认 advisory 或 renderer-local 容错，不得触发 finalizer rewrite。

### UIP-024（P1）emit_analysis 双重编码枚举会触发 analyze 重试或静默降级

代码位置：`internal/toolparam/normalize.go`、`internal/agent/agent.go::normalizeToolCallParams`、`internal/tool/emit_analysis.go`

状态：**已修复（Batch G 第三段）**。通用 schema-aware tool-param normalizer 现在读取 schema enum；只有当字段是 `string enum`，当前字符串不是枚举值，且按 JSON string literal 解包后恰好命中枚举值时才修复。普通自由文本字段不解包，非法 enum 不修复，仍由工具自身 gate 给出 canonical 错误。

触发证据：

- 最新日志中 analyzer 第一次 `emit_analysis` 发出 `intent="\"explain\""`、`scenario="\"architecture_explain\""`、`complexity="\"complex\""`、`question_kind="\"mechanism\""`、`language="\"zh\""`、`predicate_axis="\"\""`.
- `predicate_axis` 因 `"\"\""` 不是合法 axis 被拒，状态行显示 `⟳ 1/4 模型响应出错,正在重新理解问题`。
- 第二次模型省略 `predicate_axis` 后通过，但其它双重编码字段被 normalizer 静默降级：`intent "\"explain\""→unknown`、`scenario "\"architecture_explain\""→generic`、`complexity "\"complex\""→moderate`、`question_kind "\"mechanism\""→unknown`。

风险：

- 这不是模型理解失败，而是工具参数编码兼容层没兜住，导致 analyze 阶段不必要重试。
- 更隐蔽的是第二次通过后分类质量被降级，后续 explorer/finalizer 可能在错误 RequestModel 上工作，形成跨阶段返工。

防回归措施：

- 兼容规则放在通用 `toolparam.Normalize`，不写死用户问题、模型散文或某个 case。
- 回归测试覆盖：合法双重编码 enum 自动解包；自由文本保留外层引号；非法 enum 不修复并留给工具 gate。
- 新增 analyzer 侧测试保证 `emit_analysis` 的双重编码 enum 在执行工具前被归一化，并与既有 required profile default 修复共存。

## 分批修复建议

### Batch A：先消除明确红线

1. [x] 移除 RawRequest capability-surface hard matcher。
2. [x] 只允许 `CapabilitySurfaceHint` 触发 capability reconcile。
3. [x] 给 raw matcher 留 advisory/debug，不改变 RequestModel。

### Batch B：建立 PresentationContract

1. [~] 从 analyzer typed 输出承接用户明确要求：table、markdown table、diagram kind、raw Mermaid、sequenceDiagram、decision/scalar 等。（diagram contract 已跨 family；table/scalar/decision 已作为 schema-only display affordance；后续补 analyzer 显式 presentation lane）
2. [~] SemanticView/schema 使用 family default + presentation union。（已先放宽 Generic 可选 table/scalar/decision；新增 AnswerPresentationContract 投影到 schema）
3. [~] renderer 读取 contract，避免两列表格压缩；Mermaid→ASCII 在 REPL 中是 terminal-only 友好展示例外，raw markdown 仍进入 memory/output dump。（已先兼容 item markdown table，并补 pipeline memory raw 保存）

### Batch C：finalizer gate 分层治理

1. [~] 将 pre-emit checks 分为 hard / soft / advisory。（facet metadata 已分层：template-hard 才同轮拒绝，soft+证据只 advisory；inactive scope / 部分 scope disclosure 仍待分层）
2. [x] aggregate member_set 增加 role/provenance，只对 principal answer hard gate。
3. [x] surface_terms 不再自动追加正文。
4. [x] 同类错误连续失败时降级为隔离补充展示，不再反复重写。
5. [x] reviewer anchor set 改为最终文档 citation 优先，避免 reviewer 用滞后 evidence 误判真实标识符。

### Batch D：reconcile 只做一致性，不做意图重写

1. scenario/predicate/code-symbol/change-impact reconcile 加“显式用户表达保护”。
2. 成本优化移到 budget planner，不改变 answer contract。
3. 所有强改记录 shadow decision，并在最终日志摘要可见。

### Batch E：render 与 scope 统一收口

1. [~] 表格 fallback 不截断，多列稳定展示。（已支持多列补 header、markdown table 优先，并通过 PresentationContract 允许 table 载体；后续补 analyzer 显式 table preference lane）
2. [x] inactive subrepo disclosure 改成系统生成的隔离 caveat。
3. [x] SourceScope 控制 production/test/docs 过滤。（keyword search、package export、child package expansion 已接入同一 source-role classifier）
4. [x] Mermaid endpoint gate 区分内部 node id 与用户可见 label；图表语法 alias 不再触发 finalizer rewrite。

### Batch F：停止系统替模型补正文

1. [x] 禁止 normalizer 默认新增 principal 可见 block/item。
2. [~] `principal support`、`aggregate member_set`、inactive scope 等系统补充信息统一进入隔离补充区或保留原文区。（inactive scope 已进入系统补充说明；support/member_set materializer 已停止补正文）
3. [~] 审计所有 normalizer：允许本地修引用、顺序、无损 schema 搬运；不允许新增 answer claims。（已覆盖 aggregate/principal support materializer，仍需继续审 inactive scope 与其它 caveat materializer）

### Batch G：JSON/schema 兼容层泛化

1. [~] 建立 schema-aware relocation/quarantine registry，替代无限扩张的错位字段提示表。（已覆盖 schema-unknown metadata quarantine；单值 `claim_uses[].facet_ids` 已本地修复；string enum JSON-literal 已在通用 tool-param normalizer 修复；可见 payload 错位仍走 strict remap）
2. [~] core doc 已有效时，未知无害 metadata 进入 diagnostics，不触发 LLM 重试。（当前进入 operator WARN/quarantine，后续补 typed diagnostics 展示）
3. [~] 回归覆盖 top-level/misplaced `claim_uses`、`facet_ids`、`edge_anchors`、table 字段和旧 schema 残留字段。（已覆盖 top-level claim_uses / block/item/citation metadata、full/patch 单值 `claim_uses[].facet_ids` 修复、多值歧义拒绝、tool-param string enum JSON-literal 修复；保留旧错位字段 reject 测试）

### Batch H：gate taxonomy + upstream routing

1. [~] 每个 finalizer violation 必须分类为 `local_doc_defect`、`evidence_gap`、`analysis_gap`、`presentation_advisory`、`safety`。（aggregate member_set、enum-label、facet metadata emit-time gate 已先分出 hard/advisory）
2. [~] finalizer rewrite 只处理 `local_doc_defect` / `safety`；上游缺口回流 explore/extract；presentation 问题优先本地容错/补充说明。（member_set 叙事场景、非主枚举标签场景、soft facet metadata 场景已停止同轮硬重试；semantic-quality reviewer 已接入 `repair_locus` typed routing）
3. [x] 同类错误 fingerprint / typed repair family 连续失败后，停止硬重写，接受核心答案并用“补充说明/保留原文”交代缺陷。
4. [x] reviewer hard/soft 判断的 anchor truth source 统一：AnswerDocument citations 优先，explorer evidence 补充，禁止 reviewer 只读滞后 explorer slate 后要求 finalizer 重写。

### Batch I：scope / presentation contract 贯穿

1. [~] `SourceScopeProfile` 贯穿 exact target、keyword search、package export、child package expansion。（keyword search、package export、child package expansion 已完成；exact target 后续继续审计）
2. `markdown_table`、`structured_table`、`scalar`、`decision` 等展示偏好从 analyzer typed 输出进入 schema、renderer、REPL；Mermaid 偏好只约束模型输出 raw markdown，不禁止 REPL terminal-only 预览。
3. raw scope detector 保持 advisory，analyzer 之后统一用 typed scope 更新状态、上下文和最终 caveat。

## 结论

当前系统已经修掉了若干历史上最明显的“系统替用户决定答案形态”的点，但仍有一批更隐蔽的问题：schema 过早收窄、pre-emit gate 过密、normalizer 自动补正文、render 层替换输出形态。这些问题共同解释了 finalizer 阶段反复“校验未通过 / 答案待完善，正在重写”的根因：模型不是单纯不听话，而是在多个系统层的硬约束之间被迫折返。

下一步应优先做 Batch F + Batch G + Batch H，同时把 Batch B/E/I 的 PresentationContract 与 SourceScope 贯穿补齐。Batch A 已完成，Batch B/C/E 已有第一段进展；后续不要继续靠 validator 文案追模型，而要修数据流、兼容层和 gate 路由，让系统在不改变用户意图和模型建议的前提下稳定收敛。
