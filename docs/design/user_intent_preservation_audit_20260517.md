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
3. `DiagramContract.RequiredKind` 已接入，用户明确要的图类型会作为硬契约保留，不会被系统改成其它图或静默降级。
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
12. **Batch B/E 边界澄清**：REPL Mermaid→ASCII 属于终端呈现层友好降级，不再作为“系统替代模型意图”整改项；边界是 raw markdown 必须仍作为 truth source 写入日志、output dump、memory，且该渲染结果不得进入 gate / reviewer / 后续模型上下文。已补测试锁住 pipeline/local REPL 记忆保存 raw Mermaid，而不是终端渲染后的 text fence；终端渲染层兼容 ` ```mermaid <diagram-kind>` 模型常见 info-string 变体，并覆盖当前支持渲染的 `flowchart` / `graph` / `sequenceDiagram`。HTTP preview server 走内嵌浏览器 Mermaid.js，已补 `classDiagram` preview 回归测试，避免把浏览器能力误降级到终端 ASCII 子集。
13. **Batch G 第二段落地**：annotation JSON 兼容层新增唯一可恢复的 `claim_uses[].facet_ids` → `claim_uses[].facet_id` 归一化。full emit 与 patch 共用同一 repair；单值自动修复，避免小 schema 错误触发 finalizer 重试；多值保持拒绝，防止系统替模型选择 facet。新增 full/patch 回归测试。
14. **Batch B 第二段落地**：新增 family-independent `AnswerPresentationContract`。schema 现在可在不增加 finalizer prompt 负担的前提下允许 table/scalar/decision 展示载体；用户显式 diagram contract 跨所有 QuestionFamily 生效，config/role/enumeration/comparison 等旧“无图 family”不再吞掉时序图/流程图需求。软 diagram preference 仍不升级为硬要求，避免证据偏好替代用户意图。
15. **Batch G 第三段落地**：schema-aware tool-param 兼容层新增 string enum JSON-literal 修复。若 schema 声明字段是 `string enum`，模型却发出 `"\"explain\""` / `"\"\""` 这类双重 JSON 字符串，系统会在本地无损解包；解包后仍不属于 enum 的值继续交给工具 gate 拒绝。该修复覆盖 `emit_analysis.intent/scenario/complexity/question_kind/language/predicate_axis` 等字段，避免 analyze 阶段因机械参数形态返工，也避免静默降级为 `unknown/generic`。
16. **Batch D 第一段落地**：`reconcileQualifiedCodeSymbolConfigDrift` 不再在 typed `MentionedEntities` 缺失时回退扫描 `RawRequest` / generic entity lanes 来强改 config→architecture/comparison。该 reconcile 现在要求 analyzer 先给出结构化对比信号（显式 buckets，或 `is_cross_component=true` + 多个 sub_topics），再用 mentioned surfaces + repomap qualified-name resolver 证明至少两个对象是代码符号；只有符号表面而没有 typed comparison signal 时保持 config 主链。同步清理 `AnalyzerReconcileStrictMode` / `SetReconcileStrictMode` 注释中退役的 `reconcileShape` 描述，避免后续开发者误以为还能恢复旧 shape override。
17. **Batch D 第二段落地**：`reconcileChangeImpactProfile` 的硬路由修正加上 confidence floor。active change-impact typed lane 仍可进入下游提示/支撑计划，但只有 `confidence>=0.75` 且 `requested_output` 明确是 files/sites/symbols 时，才允许把粗分类修成 set-valued enumeration；低置信 profile 不再改写 intent/question_kind/predicates。新增回归测试覆盖低置信 profile 只做 advisory。

## 2026-05-18 批次进展

1. **Batch I 第二段落地**：REPL `turn_policy` 在 `route=repo` 时不再丢弃 typed `presentation_directive`；先通过 `composeEffectiveRequest` 放在 `## Current request` 内，确保“输出逻辑视图 / 表格 / 图”等当前轮展示意图能随 repo 流水线进入 analyzer/finalizer。
2. **Batch G 第四段落地**：answer-document JSON repair 增加 singleton array-shape 兼容。full emit 与 patch 共用同一 repair：顶层 `blocks` / `citations` / patch `add_blocks` / `append_citations` 等数组字段若被模型发成单个对象，系统本地包成单元素数组；block 内 `items` / `claim_uses` / `edge_anchors` 单对象、`columns` / `facet_ids` 单字符串也会本地归一化。该修复只处理唯一可恢复的机械形态，冲突 alias、多值语义选择和非法 enum 仍保持 strict reject。
3. **Batch H 第五段落地**：`diagram_relation_label_only` 从“可被 operator promote 的 finalizer rewrite/caveat”降为纯 telemetry。Mermaid 可见边标签已经能推断关系时，答案对用户是成立的；缺少 `edge_anchors` typed metadata 不再触发重写、不再产生用户可见 caveat，只保留结构化观测。横向检查同类 `diagram_edge_label_mismatch`：它已是不可 promote 的 soft 信号，且因可见标签与 typed 关系冲突会影响读者理解，暂保留用户补充说明通道。
4. **Batch I 第三段落地**：`presentation_directive` 不再拼进 runner request / `Mutable.Objective`，而是通过 `SetPresentationDirective` → `BusContext.PresentationDirective` → `AgentContext.PresentationDirective` 进入独立 `Presentation Directive` prompt 段。UI 状态行、repo_map task map、memory 不再出现 `## Presentation directive...` 这类系统 header；analysis contract 明确该段只能影响 `diagram_hint` / table / scalar / decision 等展示字段，不能作为代码实体、检索词、事实或仓范围。
5. **Batch B 第三段落地**：显式 diagram/逻辑视图/时序图/调用图/架构图请求在 comparison 等“默认无图 family”下不再因为严格 diagram-node seed 不足被降级掉。`BuildAnswerSurfacePlan` 会在已有 typed comparison/architecture 结构与可承载证据时保留 `DiagramContract.RequiredKind`，让 semantic view/schema 要求对应 `BlockDiagram`，避免 analyzer 已识别 `diagram_hint` 但最终答案缺少用户明确要求的图。
6. **Batch H 第六段落地**：analyzer `subtopic_coherence` 的 R1.3 在多仓 cross-component 对比下改为 advisory。多仓问题的 sub_topics 可以按机制分面、文件线索或内部组件拆解，不必每个 sub_topic 都重复仓名；单仓/普通场景的真正 entity orphan 仍保持 hard gate。
7. **Batch G/H 第七段落地**：analyzer 预扫描预算关闭后动态收窄工具 schema，只保留 `emit_analysis`；分析阶段同时增加工具边界硬拦截，任何绕过 schema 的 `read_file` / `exec_command` / MCP 内容工具在执行前被结构化拒绝，不再进入工具反序列化错误或整轮 analyze 重试。预算到顶后的越界预扫描调用会获得一次 emit-only 就地修正机会，而不是立刻触发 `⟳ 1/4 模型响应出错`。
8. **Batch H 第八段落地**：explorer chain promotion 不再把 `concrete_values_tracer` 这种宽扫描辅助链路缺失的新文件升级成 CGEC E2 forced-read；系统会先从提示/证据中移除未读链路，只有已读文件里的未覆盖锚点行才发起 surgical read。非 concrete 的高置信链路仍可补读，且有行号时统一走 LineRanges，避免整文件补读造成 `› 2/4 正在补充关键信息` 噪声和探索返工。
9. **Batch H 第九段落地**：accepted `emit_investigation_complete` 后若 DAG validation criteria 仍未满足，默认 soft policy 不再重开探索；系统会把未满足的 typed criterion 记录为 `TurnAArtifacts.ValidationBoundaryNotes` 并传给 extractor/finalizer。finalizer 应基于已收集证据做收敛、分组、排序或诚实说明边界，而不是为了 `answer_set_bounded` / unresolved hypothesis / evidence threshold 等天然不一定可满足的条件反复打回探索。
10. **Batch H/G 第十段落地**：客户日志里的 `is_cross_component=true + 0 sub_topics` 不再触发 analyzer 硬失败；R1.2 改为 advisory，因为跨组件是 breadth signal，不等于用户要求多主题分区。required-tool 阶段的空响应现在先进入 loop controller 的结构化纠偏，避免 finalizer 空输出被当成 voluntary stop；若最终仍没有可用 document 或原文，系统用已落地 evidence 生成“只列证据、不补结论”的诚实兜底。retry prompt 也按是否真的存在 previous emit 分叉，避免要求模型从不存在的 previous payload 修补。support lane 已渲染时，typed enrichment 统一收窄在 support 边界内，防止 demoted/noisy flow facts 污染成文。

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

状态：**第一段已修复（Batch D）**。该 reconcile 不再把“出现 qualified symbol 表面”本身当成用户意图。硬修正必须同时满足两类 typed 信号：一是 analyzer 给出显式对比结构（`Buckets>=2`，或 `Predicates.IsCrossComponent=true` 且 `SubTopics>=2`）；二是 `AnalyzerHints.MentionedEntities` 中至少两个 qualified surfaces 通过 repomap qualified-name resolver 解析为代码符号。`RawRequest`、`PrimaryEntities`、`Entities` 不再作为该硬改的 fallback 输入；`MentionedEntities` 只作为符号解析的表面来源，不能单独触发 intent/scenario/family 改写。

当前行为：

- 若 config-shaped 请求同时具备 typed comparison signal，且至少两个 typed mentioned qualified entity 可解析为代码符号，系统会改 Intent/Scenario/Kind/Axis，并生成 buckets/sub_topics。

风险：

- 对“真的是配置 key，但名字刚好像 qualified symbol”的场景，系统曾可能把用户想查配置的问题改成机制/架构比较。
- repo-grounded 保护只能证明“代码里存在符号”，不能证明“用户意图不是配置”。缺少 typed mentioned lane 或 typed comparison signal 时不能做硬改。

修复方向：

- [x] 当 typed mentioned lane 缺失时，保留 config 主链，把 code-symbol 解析留给探索/支撑证据，而不是 hard reconcile。
- [x] 当只有 mentioned qualified surfaces + repo symbol proof、但 analyzer 未给出 typed comparison signal 时，保留 config 主链，避免系统根据用户散文表面替换意图。
- 若需要改链路，必须记录 shadow decision，并允许 analyzer typed hint 覆盖。
- 防回归：新增结构测试覆盖 RawRequest 与 generic entity lanes 都含 qualified code symbols、且 repomap 可解析时，只要 `MentionedEntities` 为空，就不得改变 intent/scenario/kind/axis；另覆盖只有 mentioned surfaces + symbol proof 但无 typed comparison signal 时也不得改变主链。

### UIP-009（P2）ChangeImpactProfile 会强制 set-valued enumeration surface

代码位置：`internal/agent/change_impact_reconcile.go::reconcileChangeImpactProfile`

状态：**第一段已修复（Batch D）**。该 reconcile 现在要求 active profile 的 `confidence>=0.75` 才能硬改粗分类；低置信 change-impact profile 保留给下游 prompt/support planner 使用，但不得把 trace/explain/scalar 主链强制改成 enumeration。

当前行为：

- active `ChangeImpactProfile` 且 high-confidence requested_output 是 files/sites/symbols 时，系统会修正 `IntentEnumerate`、`ReqEnumeration`、relation/category predicates。

风险：

- 对“用户要步骤/解释，但 profile 误判成 affected files”的问题，会把答案变成列表。
- 该 profile 来自 LLM typed 输出，不是原文关键字，风险低于 UIP-001；但低置信 profile 若也能硬改，仍可能替代用户表达。

修复方向：

- 将 `requested_output` 与 `requested_presentation` 分离。
- [x] 只在 profile confidence / user-requested output 明确时强制 principal set，否则作为 prompt guidance。

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

状态：**已修复（Batch D）**。配置与代码注释已改成 legacy reconcile strict-mode compatibility switch，不再提已删除的 `reconcileShape` 或旧 shape override 行为。

当前行为：

- 该开关仅保留给仍有 advisory/strict split 的 legacy reconcile 逻辑；新 reconcile 规则应做 typed consistency repair，不应新增 strict-mode 硬改分支。

风险：

- 这是观测性/运维风险。操作员曾可能以为可以关闭/恢复某类退役 override，实际语义已经变了。

修复方向：

- [x] 更新配置注释和 startup 注释，明确其 legacy compatibility 边界。
- 后续若确认没有调用方仍需要 strict split，可再删除该配置并做 migration note。

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

状态：**部分修复（Batch G 第一段 + 第二段 + 第四段）**。新增 `answer_document_field_quarantine.go`，对 full emit / patch 的 top-level、block、item、citation、snippet、diagram、claim_use、edge_anchor、exact_resolution 等已知结构容器执行 schema-aware quarantine。保留 `value/boolean` 等可见 payload 错位字段和 claim/edge 可恢复错位字段给 strict remap，避免静默丢内容。第二段进一步把唯一可恢复的 `claim_uses[].facet_ids=["x"]` 本地归一化为 `claim_uses[].facet_id="x"`；`facet_ids` 多值仍 hard reject，因为系统无法不改变模型意图地替它选一个 facet。第四段补齐 singleton array-shape 兼容：顶层数组字段单对象/单字符串，以及 block 内 `items` / `claim_uses` / `edge_anchors` 单对象、`columns` / `facet_ids` 单字符串，会在 full emit / patch 两条路径本地归一化为单元素数组。

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
- REPL rich response 仍会默认调用 `RenderMermaidBlocks`，把 Mermaid fence 转成 ASCII text fence 或渲染警告；终端渲染层会把 ` ```mermaid <diagram-kind>` 这类 info-string diagram directive 规范化为 body directive 后再渲染，当前覆盖 `flowchart` / `graph` / `sequenceDiagram`，但 raw dump / memory 仍保留模型原文。
- HTTP preview server 不走终端 ASCII 子集；它把 Mermaid fence 交给内嵌 `mermaid@11.12.0` 浏览器渲染，并对 ` ```mermaid classDiagram` 这类 info-string diagram directive 做展示层补齐，raw markdown 仍可从 Raw Markdown 链接查看。

风险边界：

- 终端展示可变，答案真源不可变。
- 不允许 renderer 结果参与 gate、reviewer、retry 或 memory context。
- 不允许渲染失败触发 LLM-facing hard retry；失败只能转 text fence/警告，且保持 raw markdown 可追溯。

后续方向：

- 保持现有 terminal-only ASCII 预览，并继续吸收 Mermaid fence 形态差异这类展示层小错；完整 Mermaid 图优先通过 HTTP preview server 渲染。
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

### UIP-025（P1）repo 路由识别到展示意图但没有传入当前流水线

代码位置：`internal/repl/repl.go::dispatch`、`internal/repl/turn_policy.go::composeEffectiveRequest`

状态：**已修复（Batch I 第二段 + 第三段）**。`RouteRepo` 现在和 `RouteHybrid` 一样携带 typed `PresentationDirective`；第三段进一步把它从 runner request body 移到 `SetPresentationDirective` typed metadata。prompt builder 渲染独立 `Presentation Directive` 段，既让 analyzer/finalizer 能看到展示意图，又不污染 `Mutable.Objective`、UI 状态行、repo_map task map 或 memory。

历史行为：

- `turn_policy` 已能把“输出逻辑视图”识别为 `presentation_directive="logic flow diagram..."`。
- 但 `RouteRepo` 分支只记录日志，不把 directive 传给 pipeline。
- 第二段曾把 `## Presentation directive` 放进 `## Current request` 来避免 prior conversation 误分，但这仍会污染 `Mutable.Objective`，导致 UI/任务图把系统 header 当成用户问题首行展示。

风险：

- 用户明确要求的答案形态被系统路由层吞掉，最终 analyzer 可能不 emit `diagram_hint`，semantic view 显示 `has_diagram=false`，finalizer 输出缺少逻辑视图。
- 这是 typed 信号在跨层传递时丢失，不是模型不听话；靠 finalizer 重试无法稳定修复。

修复方向：

- [x] repo/hybrid 都把 typed presentation directive 传入流水线，但不进入 runner request body。
- [x] memory 仍保存用户原文，不写入系统 header，避免下一轮上下文污染。
- [x] 回归测试锁住 repo/hybrid route 通过 typed setter 传递展示指令、request body 不含 directive、无 directive 的 repo route 清空 typed metadata。

### UIP-027（P1）显式图表请求被 comparison family 的 diagram support 降级吞掉

代码位置：`internal/types/answer_surface_plan.go::BuildAnswerSurfacePlan`、`internal/types/diagram_contract_support.go::EffectiveDiagramContract`、`internal/types/answer_semantic_view_compile.go::applyPresentationContract`

状态：**已修复（Batch B 第三段）**。新增 `augmentSupportedDiagramKindsForRequiredDiagram`：当 analyzer 已经给出硬 `DiagramContract.Required`，并且当前 RequestModel 有 typed relational/comparison/architecture 结构，同时 evidence surface 至少有可承载的 grounded/recovered 证据时，保留用户显式 diagram kind 进入 `EffectiveDiagramContract`。这样 comparison/config/role/enumeration 等默认无图 family 仍会通过 family-independent presentation contract 暴露 required `BlockDiagram`，且回归覆盖当前所有支持的 concrete diagram kind，避免只修“逻辑视图/flow”单点。

历史行为：

- analyzer 能正确 emit `diagram_hint`，日志中也能看到模型理解“输出各自逻辑视图/时序图/调用图/架构图”等显式图表要求。
- 但 `SupportedDiagramKindsForAnswer` 只看严格 diagram seed（行级节点、call/import edge、validated log/flow/config roles）。当探索证据是 recovered/file-level 或叙事比较证据时，support 列表为空。
- `EffectiveDiagramContract` 因 support 为空把 hard required diagram 降为 soft preference；semantic view trace 变成 `has_diagram=false`，finalizer schema 不要求 diagram，最终答案只剩 prose/table。

风险：

- 这是下游 surface plan 对“证据能否画图”的判断过窄，不是用户意图不明确，也不是 finalizer 不听话。
- 靠 semantic reviewer 事后说“缺图”只能触发返工或 advisory，无法稳定让 schema 允许/要求图。

修复方向：

- [x] 保留 `EffectiveDiagramContract` 的“无支撑不硬画图”原则，避免软偏好变硬。
- [x] 在更上游的 `BuildAnswerSurfacePlan` 中用 typed RequestModel + typed evidence carrier 判定用户显式 diagram 是否可承载，补齐 support kinds。
- [x] 回归测试覆盖 comparison/cross-component + recovered file-level evidence 场景，并遍历 `AllDiagramKinds()` 中所有 concrete kind，确保 `DiagramContract.RequiredKind` 和 semantic view `BlockDiagram` 都保留。

### UIP-028（P1）多仓 subtopic_coherence 把合法分面拆解硬拒成 analyze 重试

代码位置：`internal/analysis/gate/coherence.go::checkSubtopicCoherence`、`internal/analysis/gate/coherence_test.go`

状态：**已修复（Batch H 第六段）**。`R1.3 entity_orphan` 在普通单仓/单 scope 场景仍是 hard gate；但当 analyzer 已给出 `is_cross_component=true` 且存在至少两个 typed `PrimaryScopes` 时，R1.3 降为 advisory telemetry。该场景下 sub_topics 可以按机制、文件、内部组件或比较维度拆解，不能要求每个 sub_topic 的 `entities[]` 都和顶层仓名相交。

触发证据：

- 最新日志中 analyzer 第一次已经成功 emit `diagram_hint=architecture`，且主实体是 `codrax/opencode`。
- LLM 将 sub_topics 拆成 `ground.go/claim_citation.go/...`、`answer_reviewer.go/...`、`read.ts/agent.ts/tool.ts` 这类机制/文件分面。
- gate 同时输出“cross-cutting decomposition intentional 可忽略”的 advisory 文案，却因为 R1.3 hard fail 导致 `⟳ 1/4 模型响应出错,正在重新理解问题`。

风险：

- 这是系统用“sub_topic 必须和 primary entity 文本相交”的内部形状偏好，覆盖了模型对比较问题的合理分面拆解。
- 下游探索阶段本来可以验证这些文件/机制锚点；在 analyzer 阶段硬拒会浪费一整轮模型调用，并可能把更好的机制分面改写成更粗的“codrax/opencode”两块。

修复方向：

- [x] 保留 R1.3 对普通 entity orphan 的 hard fail，避免真正漂移 sub_topic 进入下游。
- [x] 多 scope cross-component 场景只发 advisory，不阻断 IR。
- [x] 回归测试覆盖 `PrimaryScopes=[codrax,opencode]` + file/facet sub_topics 不相交的场景，确保 gate 通过但保留结构化 telemetry。

### UIP-026（P1）diagram label-only metadata advisory 会被升级成 finalizer rewrite/caveat

代码位置：`internal/orchestrator/contract_check_block.go::validateDiagramRelationLegality`、`internal/types/violation_registry.go::ViolDiagramRelationLabelOnly`

状态：**已修复（Batch H 第五段）**。`ViolDiagramRelationLabelOnly` 现在是 permanently soft、`Promotable=false`、`FallbackLocus=LocusTerminal`、`CaveatFamilyID=""`。validator 仍可发出结构化 telemetry 供 dashboards/日志分析，但该信号不再进入 retry eligibility，也不会物化成用户最终答案里的补充 caveat。

历史行为：

- 当 Mermaid edge label（例如 `invoke`）已能通过 relation vocabulary 推断为 call，但 `edge_anchors[].relation_kind` 没有显式声明时，contract 已满足。
- 系统仍发出 `diagram_relation_label_only`，并允许 operator strict-promote，把“缺少 typed metadata”变成 finalizer rewrite。
- 同时它挂在 `diagram_fidelity` caveat family 下，可能把内部 metadata 缺口展示给用户。

风险：

- 这是 presentation/metadata advisory，不是用户答案错误。用户看到的是清晰的图边标签，系统不应为了内部 typed authority 让模型返工。
- 继续允许 promote 会让“兼容模型可见输出”的设计退回到“强迫模型补 schema 细节”。

修复方向：

- [x] label-only relation 保持 validator telemetry，但永不触发 rewrite。
- [x] 清空 caveat family，避免把内部 `edge_anchors` metadata 缺口暴露给用户。
- [x] 回归测试锁住 `ViolationProfileFor(strict=true)` 仍为 `SeveritySoft` 且 `RetryEligible=false`。
- [x] 横向检查 sibling signal：`diagram_edge_label_mismatch` 已不可 promote；因它代表用户可见 label 与 typed relation 冲突，仍保留 soft caveat 以提示读者可能的图语义歧义。

### UIP-029（P1）extractor 因可修复 schema/锚点形状反复重试

代码位置：`internal/tool/emit_evidence.go`、`internal/tool/emit_answer_symbol.go`

状态：**已修复（Batch G/H 延伸段）**。`emit_evidence` 对两个高频、结构明确、语义无损的兼容形状做本地修复：`evidence_kind`/legacy `kind` 误填 `anchor_kind` 值时按 typed anchor 映射为 emittable evidence kind；`scope=file` 同时携带 `line_start` + line anchor 字段时降为 `scope=line` 或 `line_range`。`emit_answer_symbol` 在 cited line 已被 read_file 证明为错行时，不再立即丢弃，而是先查同文件同符号的 grounded definition evidence / surface backbone，唯一命中时本地归一到定义行，并在 tool summary 里披露。

触发证据：

- `../small/codrax-small/.codrax/logs` 最新运行中，explorer 一批 evidence 有 9 条被 `scope=file must have line_start=0`、`evidence_kind "definition" is invalid` 丢掉；这些项实际上携带了 `anchor_kind=definition`、`anchor_symbol`、`source:line_start`。
- 随后 extractor 多轮 `emit_answer_symbol` 反复猜 `SubAgentValidator` / `propose_sub_agents` 的定义行，部分项因 cited line 是调用点或方法体行被丢弃，导致 finalizer 再次要求补符号。

风险：

- 这是系统 schema 过窄和锚点消费层缺少 typed fallback 的组合问题，不是用户意图问题，也不是模型答案本身必须重写。
- 下游 finalizer 被迫承担“补上游锚点”的工作，会产生“答案待完善，正在重写”的长尾循环。

修复方向：

- [x] `emit_evidence` 只基于 typed enum / scope / anchor 字段做无损兼容修复，不读取用户问题或模型散文。
- [x] `emit_answer_symbol` 只用已 grounded 的结构化证据或 surface backbone 修复同文件同符号错行；多候选或跨文件不自动改。
- [x] 回归测试覆盖 schema-shape repair 与 wrong-line-to-grounded-definition canonicalization，防止后续开发重新引入“让模型重猜”的硬 gate。

### UIP-030（P1）finalizer patch/reviewer/diagram 小错仍会诱发整轮重写

代码位置：`internal/tool/emit_answer_document_patch.go`、`internal/orchestrator/contract_check_block.go`、`internal/orchestrator/self_consistency_reviewer.go`

状态：**已修复（Batch G/H 延伸段 + 2026-05-18 finalizer audit）**。`emit_answer_document_patch` 在本地把 `append_citations` 无损合入 `replace_citations`，再根据是否保留 citation-bearing block 继续走已有 preserved-pool normalization。finalizer 第一次 full emit 只要留下可用 rejected draft，下一轮即优先暴露 `emit_answer_document_patch` 并隐藏 full emit，避免“小修”诱发整份答案重写、丢列、丢图、丢 citation。patch 自身被拒时也默认继续修 patch（纠正 `replace_blocks` / `add_blocks` / `unchanged_block_ids` / diagram payload 等结构问题），不再把普通 patch 参数错误升级为 full rewrite。`sequenceDiagram` 只把实线闭合箭头 `->>` 结构性计为 `call`；实线无箭头 `->`、虚线返回 `-->>`、异步开箭头 `-)`、cross `-x`、双向/half-arrow 等都不默认计为 call。`self_consistency_reviewer` 兼容 `contradictions` 的空字符串、说明字符串和 stringified array；`consistent=false` 但没有结构化 contradiction 仍然拒绝。

触发证据：

- `../small/codrax-small/.codrax/logs` 最新运行中，finalizer 的 patch 同时给出 `replace_citations` 和 `append_citations`，系统直接按 contract invariant 5 拒绝，随后切到 full rewrite，模型又发出 `{}`，触发 `blocks[] is required and must be non-empty`。
- 同一轮里，sequenceDiagram 的实线调用边使用方法名作为 label（如 `BuildAnalysisSkill`），但系统只把“关系词 label”或无 label 箭头计为 `call`，导致图已经能表达用户意图时仍要求模型补 `edge_anchors`。
- 第二层 semantic quality reviewer 的 diagram contract 输入只统计 `edge_anchors`，没有复用 deterministic diagram validator 的可见边解析；因此当前图已经通过 label/implicit relation 满足时，reviewer 仍会看到 `min_satisfied=0` 并把软提示升级成返工。
- 自一致 reviewer 把 `contradictions` 发成字符串时直接 decode 失败，虽然当前是非致命失败，但会污染日志并增加“系统不稳定”的噪声。

风险：

- 这三类都不是用户意图变化，也不是答案主体必须重写；把它们交给模型修，会把 finalizer 变成协议学习循环。
- `{}` 空 full emit 不能被当作成功，因为没有可用答案 payload；正确治理方式是修掉上游 patch 兼容缺口，避免把模型逼到空 full rewrite。

修复方向：

- [x] citation op 合并只读取 typed citation 池，不读取用户问题或模型散文。
- [x] finalizer tool-level reject 走 patch-first：有 previous/rejected draft 时用 typed `emit_answer_document_patch` 修局部块；无 previous draft 时才要求完整 full emit，避免向模型发出“不存在的上一版 payload”指令。
- [x] patch reject 不再默认 “Stop patching” 切 full rewrite；普通 patch op/diagram payload/id 错误继续在 patch 通道修，降低整份答案二次损坏概率。
- [x] sequence arrow default 只读取 Mermaid 结构操作符，不读取关系词，也不改变模型原始图源码；默认范围锁定为 `->>`，其它 sequence arrow 需要显式 `edge_anchors.relation_kind=call` 或关系 label 才进入 call。
- [x] validator 与 semantic quality reviewer 共用同一套 diagram edge relation resolver：`edge_anchors.relation_kind` / edge-capable `claim_form` 优先，闭集 label vocabulary 覆盖 call、guard、import、precedence、contain、observe，最后才允许结构安全 fallback：`sequenceDiagram ->>` 计为 call、`DiagramCallDAG` 有向 flowchart 边计为 call、`DiagramFlow` decision/diamond 出边计为 guard；architecture/class 等无确定结构语义的图不做默认关系推断。deterministic contract 已满足时，reviewer 的 `diagram_gap` 不再转成返工 violation。
- [x] reviewer schema tolerance 只修结构形状；真实 `consistent=false` 且无 contradiction 仍按 malformed verdict 处理。
- [x] 回归测试覆盖 citation op 合并、preserved citation-pool remap、sequence solid/dashed 区分、reviewer string/stringified-array 兼容、first-reject patch-first、patch-reject staying on patch，防止后续开发重新把协议小错升级成 finalizer 重写。

### UIP-031（P1）analyzer 预扫描预算到顶后仍暴露预扫描/内容工具导致 analyze 重试

代码位置：`internal/agent/analyzer.go`、`internal/agent/agent.go`

状态：**已修复（Batch G/H 延伸段）**。analyzer 实现 `ToolSchemaFilter`：正常分析阶段只暴露 `emit_analysis`、`repo_map`、`grep`、`list_files`；一旦 `PrescanRoundCount >= PrescanRoundLimit` 或处于 emit-stage retry，下一轮 LLM 请求只暴露 `emit_analysis`。同时执行层新增 analyzer 工具边界：即使 provider 或旧模型绕过 schema 发来 `read_file` / `exec_command` / MCP 内容工具，也会在真正执行前用结构化 `ToolRepair.Code` 拒绝并提示回到 `emit_analysis`，避免参数反序列化小错污染 analyze 轮次。

触发证据：

- `../small/codrax-small/.codrax/logs` 最新运行中，分析阶段在系统提示 “Pre-scan budget reached (3 of 3 rounds used)” 后仍然看到 `tools=4`，模型继续调用 `grep`，随即触发 `pre-scan budget exhausted` 和 `⟳ 1/4 模型响应出错,正在重新理解问题`。
- 同一日志里 analyzer 早期还调用了 `read_file`，并把 `limit/offset` 发成字符串，导致 `json: cannot unmarshal string into Go struct field readFileParams.limit/offset of type int`。这类内容读取本就不属于 analyze 阶段，不应依赖工具参数兼容层兜住。

风险：

- 这是系统工具面和阶段契约不一致：prompt 要求“必须 emit_analysis / 禁止内容读取”，但 schema 仍给了模型继续探索的入口。
- 整轮 analyze retry 不是用户意图问题，也不是模型答案必须重写；上游应收窄工具面并本地处理越界工具调用。

修复方向：

- [x] analyzer schema filter 正常只允许分类与轻量定位工具，预算关闭后只允许 `emit_analysis`。
- [x] analyzer 执行前边界拒绝所有非允许工具，避免内容工具进入真实执行和 JSON decode 错误。
- [x] 预算关闭后的越界预扫描调用用 typed repair code 识别，给一次 emit-only 修正机会，不把同一 dispatch 立刻 fail loud。
- [x] 回归测试覆盖正常 schema 过滤、预算到顶 emit-only、内容工具边界拒绝、预算拒绝不递增 prescan round，防止后续开发重新把 read/explore 工具暴露给 analyzer。

### UIP-032（P1）concrete-values 链路补读把系统辅助推断升级成探索返工

代码位置：`internal/agent/explorer.go::applyChainPromotion`

状态：**已修复（Batch H 第八段）**。chain promotion 仍会把未读锚点的 resolution chain 从 synthesis markdown 和 `dataflow_path` evidence 中移除，避免模型看见未验证链路；但当缺失锚点来自 `concrete_values_tracer` 且指向一个调查阶段未读过的新文件时，不再写入 `PendingRead`，从而不触发 CGEC E2 forced-read 和 `› 2/4 正在补充关键信息`。如果 concrete-values 链路缺的是已读文件中的未覆盖锚点行，系统仍补救，但改用 `LineRanges` surgical read。其它高置信来源（如 `bridge_literal`）继续允许补读；有行号时同样走 surgical read。

触发证据：

- `../small/codrax-small/.codrax/logs` 最新运行中，探索阶段已经出现 `✓ 2/4 已完成证据收集`、`✓ 2/4 已交叉验证证据` 后，又输出 `INFO [CGEC] E2 forced-read: file=internal/types/context.go origin=chain_promotion.concrete_values_tracer` 和 `› 2/4 正在补充关键信息`。
- 该文件来自 concrete-values 宽扫描的 resolution chain，日志里 `concrete values: scanning 24 files`、`internal/types/context.go → 863 values / relevant 248 values`，不是用户显式要求或模型最终答案的主证据。
- 后续最终答案并未依赖 `internal/types/context.go`，说明这次补读更像系统辅助链路自我扩张，而不是必要的用户意图满足。

风险：

- concrete-values tracer 是确定性辅助检索面，信号噪声高于模型显式引用和 analyzer required files。把它的缺失新文件升级成 hard forced-read，会让系统在探索结束后自作主张扩张问题范围。
- 对大仓/多仓尤其危险：宽扫描链路容易指向上下文类型、公共结构体、注册表等“相关但非主线”的文件，造成状态行看起来已经收敛却又回探索补读。

修复方向：

- [x] concrete-values missing-new-file：只 demote 链路和证据，不触发 PendingRead。
- [x] concrete-values same-file missing-line：保留补救，但写入 `PendingRead.LineRanges`，让 forced-read 只读锚点附近的小范围。
- [x] 非 concrete 高置信链路：保留补读能力；有行号时统一 surgical，没行号时才保留 legacy full-file fallback。
- [x] 回归测试覆盖 concrete-values 新文件不补读、非 concrete 新文件仍补读、行号锚点 surgical read、pending subrepo guard 不被新规则旁路。

### UIP-033（P1）accepted closure 后仍用 DAG validation 反复重开探索

代码位置：`internal/orchestrator/orchestrator.go`、`internal/types/context.go`、`internal/agent/extractor.go`、`internal/agent/answer_document_evaluator.go`

状态：**已修复（Batch H 第九段）**。默认 `investigation_complete_policy=soft` 下，成功通过 tool pre-complete gate 的 `emit_investigation_complete` 是探索阶段的 typed closure；DAG SuccessCriteria 未满足时不再把探索重开，而是记录结构化 `ValidationBoundaryNotes`。该边界进入 extractor/finalizer prompt 与 `AnswerSurfacePlan`，要求后续成文在已证据范围内做收敛/分组/排序或 caveat 说明。`strict` policy 仍保留给需要旧式 DAG criteria 强回流的部署。

触发证据：

- 客户日志 `customlogs/error4.txt` 中，模型多次 `emit_investigation_complete` 后，界面又出现 `3/4 证据还不够稳，正在补一轮证据`，随后继续读取与重复提交证据。
- 具体 case 里 `AnswerSetBounded` 失败的根因是候选答案集合过散/过多；用户问题本身可能无法唯一收敛，继续探索不能保证把集合压小，反而会放大候选池和重试成本。

风险：

- 这是系统用模板 validation 结果覆盖模型已通过 pre-complete gate 的调查闭环，容易表现为“模型已经说完成，系统又自己补一轮”。
- 对天然开放、枚举边界不清、候选集很宽的问题，硬重开探索会让 finalizer 永远拿不到“诚实说明边界”的机会。

修复方向：

- [x] accepted closure + soft/override policy 下停止把 DAG SuccessCriteria 失败作为探索重开理由；`strict` 保留原行为。
- [x] 将未满足 criterion 作为 typed validation boundary 写入 Turn A handoff，而不是丢失或只写日志。
- [x] extractor/finalizer 显式看到这些边界，并被要求收敛呈现或说明边界，不能 invent facts 以满足 criterion。
- [x] 回归测试覆盖 `answer_set_bounded` 失败时 explorer 不被二次 dispatch，且 extractor/finalizer 都收到 boundary note。

### UIP-034（P1）cross_component breadth 被误当成多主题分区契约

代码位置：`internal/analysis/gate/coherence.go::checkSubtopicCoherence`

状态：**已修复（Batch H/G 第十段）**。R1.2 从 hard gate 改为 advisory。`predicates.is_cross_component=true` 只说明问题横跨多个组件/文件/系统，不自动证明用户要求“至少两个 sub_topics”或最终答案必须分成多个章节；只有 analyzer typed 输出真的给出分区结构时，下游才按分区组织。

触发证据：

- 客户 `analyzer_err.log` 中 analyzer 已成功 `emit_analysis`，但 gate 因 `is_cross_component=true` 且 `sub_topics=0` 触发 `subtopic_coherence` hard fail，界面出现 `⟳ 1/4 模型响应出错,正在重新理解问题`。
- 后续 finalizer prompt 出现多主题结构，甚至把用户明确排除的“进程级”又拉回结构中，体现了系统把 breadth signal 错当 partition contract 的风险。

风险：

- 系统会为了满足自己发明的分区要求，逼 analyzer 重试或让 finalizer 按错误章节组织答案。
- 单仓内部的跨组件问题、单条机制链路、日志驱动的一条跨文件路径，都可能被误拆成多主题。

修复方向：

- [x] R1.2 降为 advisory，只降低 coherence score，不触发 analyze 重试。
- [x] `reconcileSemanticPredicates` 仍保留 analyzer 的 `is_cross_component` typed signal，不再自动 demote；但是否需要 sub_topics 交给更精确的 typed partition 信号。
- [x] 回归测试锁住：cross_component + 无 sub_topics 必须通过 gate，并在 detail 中保留 R1.2 advisory。

### UIP-035（P1）finalizer 空响应可被交付成空最终答案

代码位置：`internal/agent/agent.go::BaseAgent.Execute`、`internal/agent/answer_document_evaluator.go::ParseOutput`、`renderAnswerDocRetryState`

状态：**已修复（Batch H/G 第十段）**。required-tool 阶段的空响应不再先走 evaluator voluntary stop；系统会先交给 loop controller 注入“调用结构化工具”的纠偏。若重试耗尽且没有可恢复 document / raw prose，ParseOutput 会从已落地 citation-grade evidence 生成“模型未完成成文”的证据清单，并明确不补写结论。

触发证据：

- 客户 `finalizer_err.log` 中 finalizer response `output_tokens=0 / content_len=0 / tool_calls=0`，随后系统记录 `emit_answer_document missing after retries; falling back to raw content (len=0)`，最终只输出空壳和补充说明。
- 同一日志的 retry prompt 要求 “starting from your previous payload”，但 retry state 没有 previous emit，给模型提供了不存在的修复基准。
- support lanes 已存在时，prompt 仍塞入大量 demoted / noisy typed enrichment facts，增加 finalizer 的注意力负担和跑偏概率。

风险：

- 用户会看到“系统回答了”，但实际没有模型正文，也没有可验证证据清单。
- 假 previous payload 会诱导模型 patch 不存在的内容，引发更多 invalid params / empty blocks / citation pool 错误。
- support 边界外的 enrichment 会把开放式候选和弱相关 flow fact 混进 finalizer，造成重试和内容跑偏。

修复方向：

- [x] 空 required-tool response 先经 LoopController，复用各阶段已有结构化 tool-call repair hint。
- [x] missing document 的最终兜底：优先恢复 embedded JSON / retry-state / rejected draft；都没有时列出已落地 evidence 并声明边界，不补写结论。
- [x] retry prompt 按 `PrevEmitJSON/PrevEmitSummary` 是否存在分叉；没有 previous emit 时明确要求 full `emit_answer_document`，禁止 patch。
- [x] mid-loop tool reject 也按 previous draft 是否存在分叉：没有 previous/rejected draft 时要求 complete full emit，不再要求“paste previous payload”。
- [x] support lane 已渲染且存在边界时，typed enrichment across all profiles 收窄到 lane evidence/location/anchor 范围；不再只对 diagnostic profile 生效。
- [x] 回归测试覆盖空响应纠偏、无 previous emit retry prompt、空兜底证据清单、support scope 过滤跨 profile 生效。

### UIP-036（P1）finalizer-only / forced-finalize 仍可能倒退到 3/4 重新提炼

代码位置：`internal/orchestrator/orchestrator.go::runReadSchedulerLoop`

状态：**已修复（2026-05-18 finalizer 二次审计）**。此前跳过 pre-finalize extract 的条件只看 `lastFallbackFinalizerOnly && len(BusContext.AnswerSymbols)>0`，因此对机制解释、对比、图示等不需要 answer-symbol slate 但已有 `HypothesisVerdict` / `AnswerChain` 的问题仍会白跑一次 extract；forced-finalize 逃生路径也只有在上一轮显式 finalizer-only 且有 answer symbols 时才跳过 extract。现在统一读取本轮 typed Turn-B 状态：只要当前任务已经成功跑过 extract，或存在 `AnswerSymbols`、`Mutable.EmittedAnswerSymbols()`、`AnswerChains`、`Mutable.EmittedHypothesisVerdicts()` 任一结构化产物，即认为提炼阶段已有可复用结果，不再从 4/4 倒退到 3/4。

触发证据：

- 客户 `render_log.log` / `retry_log.log` 中 finalizer 流式超时后进入 `DAG run produced no finalize output; forcing finalize`，随后又显示 3/4 提炼；旧二进制还会再次 dispatch extractor。
- 机制类/对比类问题常常只产出 hypothesis verdict，不一定产出 answer symbols；旧条件把这类有效 Turn-B 状态当作“无提炼结果”。
- 部分解释类问题的 extractor 合法地产出空 slate；旧条件会把“成功但无需结构化符号”的提炼也重新跑一遍。

风险：

- 用户界面出现 `4/4 → 3/4 → 4/4` 的阶段倒退，像系统状态错乱。
- finalizer 的传输失败或纯成文形状问题会额外烧一次 extractor LLM 调用，拉长等待时间，并可能把已经收敛的提炼结果重写成另一版。
- 对非枚举问题不友好：系统把“没有 answer symbols”误等价为“没有提炼结果”。

修复方向：

- [x] 新增 `hasReusableTurnBSlateForFinalize()`，只读取结构化 typed stage outputs，不扫描用户问题或模型散文。
- [x] 调度器用本轮 `preFinalizeExtractCompleted` 布尔值记录 extract 是否已成功完成，避免空 slate 问题被误判为未提炼。
- [x] normal finalizer-only retry：已有可复用 Turn-B slate 时跳过 pre-finalize extract。
- [x] forced-finalize escape：已有可复用 Turn-B slate 时跳过 extract replay，直接成文。
- [x] 回归测试覆盖 `HypothesisVerdict`-only、空 slate 成功提炼、finalizer dispatch error 后 forced-finalize 三条路径，防止后续开发者又把条件收窄回 `AnswerSymbols`。

## 分批修复建议

### Batch A：先消除明确红线

1. [x] 移除 RawRequest capability-surface hard matcher。
2. [x] 只允许 `CapabilitySurfaceHint` 触发 capability reconcile。
3. [x] 给 raw matcher 留 advisory/debug，不改变 RequestModel。

### Batch B：建立 PresentationContract

1. [~] 从 analyzer typed 输出承接用户明确要求：table、markdown table、diagram kind、raw Mermaid、sequenceDiagram、decision/scalar 等。（diagram contract 已跨 family；显式 diagram 在 typed relational evidence 可承载时不再被 comparison 等默认无图 family 吞掉；table/scalar/decision 已作为 schema-only display affordance；后续补 analyzer 显式 presentation lane）
2. [~] SemanticView/schema 使用 family default + presentation union。（已先放宽 Generic 可选 table/scalar/decision；新增 AnswerPresentationContract 投影到 schema；显式 required diagram 会生成 `BlockDiagram`）
3. [~] renderer 读取 contract，避免两列表格压缩；Mermaid→ASCII 在 REPL 中是 terminal-only 友好展示例外，raw markdown 仍进入 memory/output dump。（已先兼容 item markdown table，并补 pipeline/local memory raw 保存；终端渲染兼容所有 supported kind 的 `mermaid <diagram-kind>` info-string 变体；HTTP preview server 使用浏览器 Mermaid 并覆盖 `classDiagram`）

### Batch C：finalizer gate 分层治理

1. [~] 将 pre-emit checks 分为 hard / soft / advisory。（facet metadata 已分层：template-hard 才同轮拒绝，soft+证据只 advisory；inactive scope / 部分 scope disclosure 仍待分层）
2. [x] aggregate member_set 增加 role/provenance，只对 principal answer hard gate。
3. [x] surface_terms 不再自动追加正文。
4. [x] 同类错误连续失败时降级为隔离补充展示，不再反复重写。
5. [x] reviewer anchor set 改为最终文档 citation 优先，避免 reviewer 用滞后 evidence 误判真实标识符。
6. [x] typed exclusion/export scope 双层兜底：analyzer 明确 public/exported-only → `private` typed exclusion；`emit_investigation_complete` 在 aggregate facts 入库前按 repo 图 kind/exported 状态移除 excluded-role 成员并重算精确数量；`emit_answer_document` 只做可见面确定性 redaction，不把小型排除项泄漏交给 finalizer 重写。

### Batch D：reconcile 只做一致性，不做意图重写

1. [~] scenario/predicate/code-symbol/change-impact reconcile 加“显式用户表达保护”。（code-symbol config drift 已要求 typed comparison signal + typed `MentionedEntities` + repomap 符号证明；change-impact 硬路由已要求 high-confidence typed profile；不再从 RawRequest/generic entity lanes、单独符号表面或低置信 profile 触发硬改）
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

1. [~] 建立 schema-aware relocation/quarantine registry，替代无限扩张的错位字段提示表。（已覆盖 schema-unknown metadata quarantine；单值 `claim_uses[].facet_ids`、singleton array-shape 已本地修复；string enum JSON-literal 已在通用 tool-param normalizer 修复；可见 payload 错位仍走 strict remap）
2. [~] core doc 已有效时，未知无害 metadata 进入 diagnostics，不触发 LLM 重试。（当前进入 operator WARN/quarantine，后续补 typed diagnostics 展示）
3. [~] 回归覆盖 top-level/misplaced `claim_uses`、`facet_ids`、`edge_anchors`、table 字段和旧 schema 残留字段。（已覆盖 top-level claim_uses / block/item/citation metadata、full/patch 单值 `claim_uses[].facet_ids` 修复、full/patch singleton array-shape 修复、多值歧义拒绝、tool-param string enum JSON-literal 修复；保留旧错位字段 reject 测试）

### Batch H：gate taxonomy + upstream routing

1. [~] 每个 finalizer violation 必须分类为 `local_doc_defect`、`evidence_gap`、`analysis_gap`、`presentation_advisory`、`safety`。（aggregate member_set、enum-label、facet metadata emit-time gate 已先分出 hard/advisory）
2. [~] finalizer rewrite 只处理 `local_doc_defect` / `safety`；上游缺口回流 explore/extract；presentation 问题优先本地容错/补充说明。（member_set 叙事场景、非主枚举标签场景、soft facet metadata 场景已停止同轮硬重试；semantic-quality reviewer 已接入 `repair_locus` typed routing；diagram label-only metadata 缺口已降为 telemetry-only；多仓 subtopic entity-orphan 已在 cross-scope 场景降为 advisory；`ViolBlockCoverageMissing` 的 under/over required block carrier 由 producer 标记 `RepairLocusOverride=finalizer`，避免纯答案形状问题回拉 extract）
3. [x] 同类错误 fingerprint / typed repair family 连续失败后，停止硬重写，接受核心答案并用“补充说明/保留原文”交代缺陷。
4. [x] reviewer hard/soft 判断的 anchor truth source 统一：AnswerDocument citations 优先，explorer evidence 补充，禁止 reviewer 只读滞后 explorer slate 后要求 finalizer 重写。

### Batch I：scope / presentation contract 贯穿

1. [~] `SourceScopeProfile` 贯穿 exact target、keyword search、package export、child package expansion。（keyword search、package export、child package expansion 已完成；exact target 后续继续审计）
2. [~] `markdown_table`、`structured_table`、`scalar`、`decision` 等展示偏好从 analyzer typed 输出进入 schema、renderer、REPL；Mermaid 偏好只约束模型输出 raw markdown，不禁止 REPL terminal-only 预览。（repo/hybrid route typed presentation directive 已通过 `SetPresentationDirective` 进入当前流水线且不污染 objective；后续补 analyzer 显式 presentation lane）
3. raw scope detector 保持 advisory，analyzer 之后统一用 typed scope 更新状态、上下文和最终 caveat。

## 结论

当前系统已经修掉了若干历史上最明显的“系统替用户决定答案形态”的点，但仍有一批更隐蔽的问题：schema 过早收窄、pre-emit gate 过密、normalizer 自动补正文、render 层替换输出形态。这些问题共同解释了 finalizer 阶段反复“校验未通过 / 答案待完善，正在重写”的根因：模型不是单纯不听话，而是在多个系统层的硬约束之间被迫折返。

下一步应优先做 Batch F + Batch G + Batch H，同时把 Batch B/E/I 的 PresentationContract 与 SourceScope 贯穿补齐。Batch A 已完成，Batch B/C/E 已有第一段进展；后续不要继续靠 validator 文案追模型，而要修数据流、兼容层和 gate 路由，让系统在不改变用户意图和模型建议的前提下稳定收敛。
