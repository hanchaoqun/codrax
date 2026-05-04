# post-shape 残留 audit + 修复跟踪表 (2026-05-04)

合并自:
- 用户审计("V1/V2 已不打架,但模型可见契约小残留 + 新语义层 validator 没接上线"两类风险) — 2026-05-04
- `B5_B6_eval_baseline.md` §4 真 bug(三项)— 2026-05-04

风险层次:
- **类 A**:模型可见契约残留错位(LLM 看到的提示 / schema / 教学有错或矛盾)
- **类 B**:新语义层 validator / reviewer / repair 路径"代码在,运行时没接上线"(假象 vs 实际)
- **类 C**:可观测性 / usability / 文档心智模型残留

执行规则:**每条一个 commit;PR 验收必须有真测复现 + 修后真测验证(不只机器测试)**;红线 grep 每 commit 前跑;`feedback_no_dismiss_as_llm_flake.md`:不允许把任一条解释成"flake / pre-existing / 看后续"。

---

## R1 表(类 A — 模型可见契约残留错位)

| ID | 严重性 | 标题 | 代码证据 | 修复方向 | 验收 |
|---|---|---|---|---|---|
| **R1.1** | P1 | finalizer 的 LLM 可见 helper 仍说 V1 心智 | `internal/agent/answer_document_evaluator.go::renderAnswerDocStepBackbone / renderAnswerDocLogSourceDrift` 等 helper 段落仍在用 `steps[]` / `summary` 旧 payload 心智叙述 | 全量审 evaluator 内 helper(grep `renderAnswerDoc*`),把 LLM 可见输出改成 V2 block 词汇(`block.kind=ordered_list with items[]` / `block.kind=summary` 等);只动模型可见 prose,不动 helper 名/调用点 | grep `assignment\|steps\[\]\|summary\b.*payload\|symbols\[\]` in evaluator + skill + emit tool 三处 = 0;补单测锁 prose 包含的 V2 词汇集 |
| **R1.2** | P1 | claim_form 枚举值/提示词不一致 | 内部枚举:`internal/types/claim_form.go` 的 `assignment_fact / return_fact / ...`;但 `emit_answer_document.go` / `skill/defaults.go` / `analysis/hint/composer.go` 模型可见说明仍有 `assignment` 等旧叫法 | (a) grep 三处文件比对,改写成 internal 枚举值;(b) **不**加 alias 归一化层(那是绕路),让模型直接看到正确名 | grep `\bassignment\b` (作为 ClaimForm 名,非 Go 变量) in 三处 = 0;新单测 `TestClaimFormVocabularyConsistency_LLMFacingMatchesEnum` |
| **R1.3** | P3 | `contract_check.go` V1 注释残留 | 文件内有大块注释讲 V1 era 的 oracles/shape/steps/symbols | 删 V1 era 解释段(只删注释不动逻辑);保留迁移提醒(`docs/migration/...` 路径引用) | grep `runShapeOracle\|V1 oracle\|shape steps/symbols\|RequiredAnswerShape` 在 contract_check.go = 0 |
| **R1.4** | P3 | docs/architecture.md shape-era 残留 | 6 处 `AnswerShape` 已是迁移历史,但还有更多 shape-era 描述段在主文中 | 一遍 pass 把"shape-era 描述"改写成"V2 carrier + AnswerSemanticView 描述";保留迁移历史段不删 | grep -c `AnswerShape\|RequiredAnswerShape` 不变(允许迁移历史);新增"V2 carrier"段或确认已存在 |
| **R1.5** | P1(eval verification 真发现) | LLM-facing typed-set 字段值缺渲染(类问题) | s1a r1 真 eval 触发 9 个 `ViolFacetUncovered`(enumeration_item × 6 + uncertainty_boundary × 3),Detail 都是"no V2 block declared it via block.facet_ids[] or via item.claim_use.facet_id"。深挖根因:`renderAnswerDocBlockContract` / `renderAnswerDocFacetCoverage` 只列 facet **名称**,**未把 typed FacetID 字符串值 verbatim 渲染到 prompt**。LLM 看到 schema 说"读 user section",但 user section 没值可读 → 无法 copy → 100% false-fire。**类问题**:任何 BlockRequirement 上的 typed 集合字段(`FacetIDs` / `AcceptableClaimForms` / `SurfaceRoleHint`)只要"字段名声明在 prose 中、typed 值未到达 LLM",validator 必触发(因 validator 是字面 string 匹配)。 | 泛化修复(非单点加"facet_ids 必填"教学):(a) `renderAnswerDocBlockContract` 在每个 BlockRequirement 行后追加 verbatim typed 值列(facet_ids: ["X","Y"] / acceptable_claim_forms: ["definition_fact","call_edge"] / surface_role_hint);(b) `renderAnswerDocFacetCoverage` 在每个 facet 行后追加 verbatim ID 字符串;(c) schema 字段描述 "read from user section" → "copy verbatim from 'Block Contract' section's typed-value lines"。红线:不加 alias 层(R1.2)/不加 keyword/不动 facet 模板。| 真 eval rerun s1a:`facet_uncovered` 触发数 9 → ≤2(允许 LLM 偶尔漏一两个但不 100%)。grep 三处文件 `facet_ids:.*\[` 至少在 BlockRequirement 渲染行存在 |

## R2 表(类 B — 看似有、实际未接上线)

| ID | 严重性 | 标题 | 代码证据 | 修复方向 | 验收 |
|---|---|---|---|---|---|
| **R2.1** | P1 | self-consistency reviewer 是死特性 | `self_consistency_reviewer.go` 实现完整,`cmd/root.go` 配置完整,**但主链没找到 runtime 调用入口**;`ViolSelfContradiction` 在 fallback 表里却永远不可能被产生 | **决策已定:重接 V2** (2026-05-04)。reviewer 输入从 V1 `doc.Summary + steps[]` 改成 V2 `BlockSummary.Text + 其他 blocks 渲染`;主链 dispatch 在 `runContractCheck` 后接(commit 62 deletion 的位置);触发条件保留:`ShapeStepList/ShapeListOfSymbols/ShapeExplanation` 等价 → V2 family 等价物 (QFEnumeration/QFCallChain/QFGeneric/...) + summary 字数门槛 + body 块数门槛 | 真 eval 跑 1-2 个会自相矛盾 case(QF 多topic 类),log 出现 `[self_consistency]` 字样 + violation 进 closure |
| **R2.2** | P1 | finalize V2 contract fail 还会被拉回 extract/explore | `fallback_policy.go::FallbackTargetForViolations` 按整组主 repair locus 选(deepest),finalize-local 问题混上 `ViolBlockCoverageMissing/ViolFacetUncovered/ViolAbsenceScopeExceeded` 时整轮被深拉 | (a) **不能简单"按 finalize-local 优先"** — 那会让真该深拉的回归;(b) 加分流:violation **群组**按 RepairLocus 分两个 batch — finalize-locus violations 先在 finalize 内部 retry(N 次),N 失败才放更深的 violations 推回上游;(c) 有 `lastFallbackFinalizerOnly` latch 但还不彻底 | 新单测覆盖 4 类混合:全 finalize-local / 全 deeper / 混合 + finalize 优先 / 混合 + 深 violation 跨 N 次后允许 escalate |
| **R2.3** | P1 | `ViolFacetUncovered/ClaimFormUnsupported/AbsenceScopeExceeded/RichnessRegression` 无生产者 | 类型在 `violation.go`,被 `composer.go::summariseExactFix` + `fallback_policy.go::DefaultFallbackPolicy` 消费;**但 `cgec_completeness_test.go::pending` 表显示这 4 个都是 `B8-T4-retired-V1-...-oracle`** — 即生产者已被 V2 carrier 删除 | **决策已定:重接 V2** (2026-05-04)。参考 `runStructuralEnumerationDivergenceOracleV2` / `runSymbolAnchorTrackOracleV2` 的 V2 重写模式;每个 oracle 一个 commit;输入 = V2 `AnswerDocumentV2.Blocks` + AnswerSemanticView + 必要的 EvidenceClosure 数据;cgec_completeness pending 表搬到 covered 表 | 真 eval 跑相应触发场景,closure ledger 出现这 4 个 kind 之一;facet/claim_form 在 R3.1 facet 模板审计后也跟着触发 |
| **R2.4** | P1 | B6-F1 cross_citation_conflict oracle 选位错(eval baseline 4.1) | `runCrossCitationConflictOracleV2` 按 `Items[].Label` 分组,Label 是渲染文本(如 "checkCoverage — 覆盖度检查");s1a r1 答案 15 条 citation 全指 `gate.go:128` 但 oracle 没触发 | 改读符号身份:优先 `Items[].ClaimUse.EntityID`(若 LLM 填了),次之 `Items[].ID`,最次回退 `Items[].Label` 但限制 ≤ 32 字符 + 单行(避免渲染长文本)| 新单测复现 s1a r1 类 case + 修后验证 oracle 触发;eval baseline rerun s1a 应观察 `cross_citation_conflict` 出现 |

## R3 表(类 B 续 — facet 模板过严)

| ID | 严重性 | 标题 | 代码证据 | 修复方向 | 验收 |
|---|---|---|---|---|---|
| **R3.1** | P1 | enumeration / architecture facet 模板过严(eval baseline 4.3)| s1a/m1a 4/4 runs `[richness] facet_softened`;`compile_enumeration.go` enumeration_item HARD + AcceptableForms 列表里没有 mechanism-enumeration 类问题 evidence 真生产的 ClaimForm | 审计 `compile_*.go::AcceptableForms`,把不可达 ClaimForm 移除 OR 把 facet 改 SOFT/Optional Tier;**或**审计 `internal/agent/explorer.go::emit_evidence` 看 ClaimForm 是否被打错(更深根因)| facet_softened 在 4/4 runs 命中率降到 < 50%;真 eval 必跑 s1a + m1a + qf_architecture |

## R4 表(类 B 续 — schema 兜底不彻底)

| ID | 严重性 | 标题 | 代码证据 | 修复方向 | 验收 |
|---|---|---|---|---|---|
| **R4.1** | P2 | nested array 整体 string 化只覆盖了 blocks[] | `emit_answer_document_v2.go::repairBlocksAsString` 只救 blocks 整体;items[] / claim_uses[] / diagram nested 不救 | 加 `repairNestedArraysAsString` — 递归扫:对每个 block,若 items / claim_uses / diagram.claim_uses 是 string 且 trim 后 startswith `[`,re-parse + WARN | 新 4 个单测对应 4 种 nested 场景;每个 WARN 行带 path(`blocks[2].items`) |
| **R4.2** | P2 | 全量 V2 payload retry 仍要求"贴回完整 doc 仅改局部" | `answer_document_evaluator.go` repair 提示中仍有"上一版 payload 原样贴回,改 X" | 这是 **F7-A 的真根因延续问题**;短期最低伤害方案:把 `[unchanged]` 提示从 prompt 删除,改成"only re-emit blocks you intend to change";**真根因**仍要靠 F7-A `emit_answer_document_patch` tool | 短期:grep `贴回\|原样\|copy.*verbatim\|stitch back` in evaluator = 0;长期看 F7-A |
| **R4.3** | P2 | diagram 边语义校验弱 | `contract_check_block.go::validateDiagramEdgeSupport` 检查 kind/presence,边的 source-target 对没真验证 | 加 oracle:每条 edge 的 (from, to) 必须能映射到至少一个 ClaimUse 或 evidence subject/object;否则 ViolDiagramEdgeUnsupported 加 Detail 列出未支撑边 | 新单测覆盖:有支撑 / 部分支撑 / 全无支撑 / nil ClaimUses;m1a 类带流程图 case rerun |
| **R4.4** | P2 | 比较类无 dedicated QFComparison family | `facet_plan.go::ResolveQuestionFamily` 落 QFEnumeration/QFCallChain/QFGeneric;B5-F3 已发 telemetry 但只是 advisory | 设计单独 commit 加 QFComparison + `compile_comparison.go` 模板(对称双 bucket scaffold);**前提:R3.1 facet 模板审计先做,避免新 family 又过严** | 比较类 case eval(自定义 case)走 QFComparison + 不再 family_underrepresented |

## R5 表(类 C — 可观测性 / 文档)

| ID | 严重性 | 标题 | 代码证据 | 修复方向 | 验收 |
|---|---|---|---|---|---|
| **R5.1** | P1 | eval/run.sh summary.md 渲染缺 4 列(eval baseline 4.2) | `write_metrics` 写 analyzer/explorer/extractor/finalizer_iters 4 字段,聚合表格还在老 12 列 | summary.md 渲染聚合块加 4 列(median 一致格式);verdict 表格不动 | 真 eval rerun 看 summary.md 出现 4 列 |
| **R5.2** | P3 | LLM-turn 与系统内部循环可观测错觉 | `[diag X] iter=N ASSISTANT content_len=` 与 adapter 侧 `[llm] response:` 完全 1:1(2026-05-04 真测验证 26=26);**计数已正确**,B6-F5 metric 准确反映 LLM turn。残留是**日志冗余**:同一个 ReAct iter 内有 INIT / TOOL HISTORY PRUNED / ASSISTANT / TOOLRESULT / MIDLOOP / SOFT-STOP 等都带 `iter=N` 前缀,grep `iter=` 会得假阳性 | 给非-LLM-dispatch 的子事件加二级标识(`[diag X] iter=N phase=midloop` / `phase=toolresult`),保留 `iter=N` 主索引 | grep `iter=N ASSISTANT content_len=` 严格 1:1 LLM dispatch(已成立 ✓);grep `iter=N` 模糊 = 该 iter 所有子事件,但每个子事件带 phase 后缀 |

---

## 修复执行顺序(按依赖 + 风险 + 收益)

**第 1 批 (P1 决策驱动 — 先决定"重接 / 删")**:
1. **R2.1** self-consistency reviewer:决策"重接 V2 / 删"
2. **R2.3** 4 个无生产者 violation kind:同样决策(决策建议跟 R2.1 一起做,因为是相同模式)
3. **R5.2** LLM-turn 真实计数 — 因为后续修 fallback / oracle 都依赖准确观测

**第 2 批 (P1 真 bug 修)**:
4. **R2.4** B6-F1 cross_citation oracle 选位修
5. **R2.2** fallback policy finalize-local 优先分流
6. **R3.1** facet 模板审计

**第 3 批 (P1 模型可见契约清扫 — 跟 R2 决策耦合)**:
7. **R1.1** finalizer helper V2 词汇统一
8. **R1.2** claim_form 词汇一致
9. **R5.1** eval summary.md 4 列(随 R5.2 一起做)

**第 4 批 (P2 - F7 + Schema + Family)**:
10. **R4.1** nested string-mode 兜底
11. **R4.2** evaluator prompt 短期伤害修(等 F7-A 单 session)
12. **R4.3** diagram 边语义
13. **R4.4** QFComparison family(等 R3.1 审计完)

**第 5 批 (P3 文档残留)**:
14. **R1.3** contract_check V1 注释清扫
15. **R1.4** docs/architecture.md shape-era 残留

---

## 红线核查清单(每 commit 前必跑)

- [ ] `feedback_no_custom_keyword_matching.md` — 任何新逻辑 zero keyword tables
- [ ] `feedback_precise_signals_for_hard_gates.md` — 新 hard gate 必基于 typed enum/integer/verbatim
- [ ] `feedback_no_system_backfill_to_user_panel.md` — 不动 doc.Symbols / Summary
- [ ] `feedback_root_cause_only.md` — 不允许调阈值绕过结构问题
- [ ] `feedback_no_dismiss_as_llm_flake.md` — 真 eval 跑过才算修
- [ ] `feedback_eval_pass_is_not_green.md` — substring 通过 ≠ 修复完成
- [ ] L1 read mode byte-identity:默认配置下行为不变
- [ ] grep `LLM never sees ClaimForm` = 0
- [ ] grep `RequiredAnswerShape\|AnswerShape` 不增长

---

## 状态跟踪 (动态更新)

| ID | 状态 | commit | 真 eval 验证 |
|---|---|---|---|
| R1.1 | 🟢 SHIPPED finalizer helper LLM 词汇统一 | TBD | 真 eval 验证 LLM 不再被 V1 心智误导 |
| R1.2 | 🟢 SHIPPED LLM 词汇 1:1 内部 enum + 锁 test | TBD | 待真 eval 验证不再 false-fire ClaimForm 验证器 |
| R1.3 | 🟢 SHIPPED contract_check.go V1 注释清扫 | (本 commit) | grep `runShapeOracle\|V1 oracle\|RequiredAnswerShape` 在 contract_check.go = 0 |
| R1.4 | 🟢 SHIPPED docs/architecture.md shape-era 残留清扫 | (本 commit) | 6 处 AnswerShape 残留全为迁移历史标记 (`已退役` / `已删除`),无活动技术描述 |
| **R1.5** | 🟢 SHIPPED 同 R7 (一并落地) | TBD | 真 eval rerun 验证 |
| ~~**R6**~~ | 🔵 由 R14 统一解决 | — | retry 字段失忆 — 类 A;深层根因同 R6.1/R11/R13 |
| ~~**R6.1**~~ | 🔵 由 R14 统一解决 | — | block vs item 失忆 sub-pattern;深层根因同 R6/R11/R13 |
| ~~**R11**~~ | 🔵 由 R14 统一解决 | — | violation 缺 Severity;深层根因同 R6/R6.1/R13 |
| **R12** | ⬜ pending P3(R7 verification 深挖发现) | — | explorer/extractor/finalizer iter 计数 cross-dispatch 累加,m1a r1 metric `explorer_iters=40` 实际 4 次 dispatch × ~10 iter,运维看错。**纯观测性问题,不属 retry-state 范畴**,独立修。见 R7_eval_deep_audit.md |
| ~~**R13**~~ | 🔵 由 R14 统一解决 | — | scheduler vs V2 oracle 双层 gating;深层根因同 R6/R6.1/R11 |
| **R14** | 🟢 SHIPPED c1-c10(c10 真 eval verify 完成)| e79f308+3162146+9e3b038 | Pass rate 3/4→**4/4**;Hard Rule renders × 4 真触发;facet_uncovered 4→1 (-75%)。但暴露 R15/R16 深层 insight。见 R14_eval_deep_audit.md |
| **R15** | 🟢 SHIPPED ViolationProfile 单一 source of truth | TBD | 待 c2 真 eval rerun 验证 R14 retry-state 真触发率上升 |
| **R16** | 🟢 SHIPPED c1+c2+c3 (3d0f359 + 549a9a1);**c4 真 eval 待跑** | 3d0f359+549a9a1 | AnswerDocumentV2Patch 数据 + ApplyPatch + emit_answer_document_patch tool + skill 教学全 wire,21 lock test 全过。c4 真 eval 期望:LLM 在 retry 路径上选 patch tool + unchanged_block_ids,m1a r2 类 retry 失忆消失 |
| **R7** | 🟢 SHIPPED typed-set verbatim + 反向归属 | TBD | 真 eval rerun 验证 facet_uncovered 数下降 |
| **R8** | ⬜ pending P2(全量日志深挖发现) | — | analyzer 自动决策无 telemetry — 类 C |
| **R10** | ⬜ pending P2(全量日志深挖发现) | — | CGEC 高频事件主链不可观测 — 类 D |
| R2.1 | 🟢 SHIPPED V2 重接 (commit pending push) | TBD | 待真 eval 跑 QF 多 topic case |
| R2.2 | 🟢 SHIPPED 分流 + 预算 | TBD | 待真 eval 出现 finalize+deeper 混合场景 |
| R2.3 | 🟢 SHIPPED 4/4:Facet (3a4a39a) + Richness (3a4a39a) + ClaimForm (8cdfd93) + AbsenceScope (pending) | TBD | 待真 eval 触发场景 |
| R2.4 | 🟢 SHIPPED 选位修 | TBD | 待真 eval rerun s1a 看是否触发 |
| R3.1 | 🟢 SHIPPED 真根因修(emptySurface 短路 inconclusive) | TBD | 待真 eval rerun s1a/m1a 确认 facet_softened 不再 fire 4/4 |
| R4.1 | ⬜ pending | — | — |
| R4.2 | 🔵 由 R16 取代 — emit_answer_document_patch 即 F7-A retained-draft 协议层实施 | (R16) | n/a |
| R4.3 | ⬜ pending | — | — |
| R4.4 | ⬜ pending(等 R3.1) | — | — |
| R5.1 | 🟢 SHIPPED summary 4 列 | TBD | smoke 通过 (s1a 历史数据) |
| R5.2 | 🟢 NO-OP — B6-F5 metric 已 1:1 LLM turn (baseline eval 真验证),仅日志冗余降级 P3 (合并到 R12) | (B6-F5) | n/a |

实际开发时每条修完更新这个表。
