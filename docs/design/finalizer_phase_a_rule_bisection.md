# Finalizer Prompt Phase A — Rule Bisection

**Status**: Design (not yet implemented).
**Target session**: single-session ship, 6-9 commits.
**Predecessor**: `docs/design/analyzer_amplifier_layer.md`(analyzer-side amplifier 主线 SHIPPED 2026-05-05;finalizer prompt jitter 仍在 — 本设计是后续治理).
**Successor**: `docs/design/finalizer_phase_c_shape_contract.md`(Phase C — typed EnumerationSubType signal;ship 在 Phase A 完成且真 eval 验证后).
**Owner**: 任意接手者(本文档目标:不熟悉本仓库的开发者读完一遍即可开始改).
**Eval bar**: qf_arch x4 → 4/4 PASS;m1a / s1a / u3a / qf_config_precedence x4 → 不回归.

**Baseline**: 本文档所有 `internal/...` 文件 file:line 引用基于 **commit `4cd053d`**(等价于代码层 `efa4ff3` — `4cd053d` 仅加 design doc,不改代码)。后续提交可能让具体行号漂移。**实施前**先 grep 关键 symbol(`runV2BlockOraclesWithMut` / `summariseExactFix` / `defaultCooccurrenceRules` / `validate*` 等)对齐行号;若漂移,以 grep 真实结果为准,不要按文档行号瞎改。可用 `git log --oneline efa4ff3..HEAD -- internal/skill/ internal/orchestrator/ internal/analysis/hint/ internal/agent/answer_document_evaluator.go` 查 baseline 后所有相关代码改动。

---

## 0. TL;DR — 一句话设计

`internal/skill/defaults.go` 里 `answer-document-skill` 当前承载 24 条 Workflow + 10 条 Prohibitions + ~155 行 OutputFormat(共 ~80 directive)。其中 **11 条 Workflow 规则可安全删除**(对应 V2 validator 已机器化校验且 hint composer 已有 actionable case 或将通过本 Phase Commit 1 补强),**4 条规则需 hint 补强后才能安全删除**(条件 DELETE),**1 条规则部分删除**(规则 #16 拆分),**8 条规则保留全文**(LLM 判断题或 validator 不覆盖)。Hint composer 现有 15 explicit case + 1 default,Commit 1 加 2 个新 case + 改 1 个 existing case 让删除决策有 actionable 兜底。

**为什么这能修 A'' jitter**:删去 11 条机器化规则后,A''(规则 8)在 Workflow 里的注意力份额从 1/24 翻到 ~1/13。validator + hint composer + cooccurrence rule 三层联防确保结构性约束不会因为从 prompt 拿掉而失守。

---

## 1. 背景:为什么这个 Phase 存在

### 1.1 现状问题(load-bearing 事实)

`internal/skill/defaults.go::Register{Name: "answer-document-skill"}`(line 115-320)注册的 finalizer skill 包含:

- **Workflow** = 24 条编号 list,line 119-142
- **OutputFormat** = ~155 行散文 + 表格,line 152-307
- **Prohibitions** = 10 条 bullet,line 308-319

合计 ~80 条 LLM 可见的 directive。`internal/context/builder.go:384-403` 的 builder 直接把这些字段渲染成 numbered list / bullet list / 散文段落,**渲染器是 dumb 的**,任何重组只在 `defaults.go` 内容层操作。

LLM 对长 list 注意力非均匀。2026-05-05 commit `efa4ff3` 在第 8 条插入新规则 A''(role-enumeration 的 abstraction-level matching),实测 qf_arch x4 = 3/4 PASS — 1/4 失败原因是 LLM 在 retry 路径上**没激活 A''**(prompt 里被"30 条规则平均分注意力"稀释)。

### 1.2 关键观察(为何"删 prompt 规则"在 codrax 是安全的)

跨 4 个 Explore agent 验证后,得到 3 个事实:

**事实 1**:`internal/orchestrator/contract_check_block.go::runV2BlockOraclesWithMut`(line 1681)在每次 finalizer emit 之后**强制运行 13 个 V2 validator**,逐项 hard-check 大部分 Workflow 规则的不变量:

```go
// line 1681-1706(真实代码)
func runV2BlockOraclesWithMut(doc *AnswerDocumentV2, view *AnswerSemanticView, mut *MutableState) []Violation {
    var out []Violation
    out = append(out, validateRequiredBlockCoverage(doc, view)...)        // 规则 #2
    out = append(out, validatePrincipalClaimUse(doc, view)...)            // 规则 #4
    out = append(out, validateDiagramEdgeSupport(doc, view)...)           // 规则 #5
    out = append(out, validateUncertaintyBlockPresence(doc, view)...)     // 隐式
    out = append(out, validateFacetCoverage(doc, view)...)                // 规则 #2 facet 维度
    out = append(out, validateRichnessGlaringGap(doc, view)...)
    out = append(out, validatePrincipalProseUnderfilled(doc)...)
    out = append(out, validateRichnessRegression(doc, view)...)
    out = append(out, validateClaimFormSupport(doc, mut)...)              // 规则 #4 + #7 claim_form 维度
    out = append(out, validateMissingRequestedRoleDisclosure(doc, view)...)// 规则 #11 (bucket alignment)
    out = append(out, validateAbsenceScopeBound(doc)...)                  // 规则 #24 (absence-citation)
    out = append(out, validateEnumerationItemLabelGrounding(doc, mut)...) // 规则 #7 + #14 (label verbatim)
    out = append(out, validateEnumerationItemLabelExtractorMatch(...))
    return out
}
```

外加 `internal/orchestrator/contract_check.go:143-213` 里另外 6 个独立 validator(`runStructuralEnumerationDivergenceOracleV2`、`runExternalArtifactDecodedCheck`、`runAuthorityOverreachCheck`、`runSelfConsistencyReviewV2`、`runSemanticQualityReview`、`runCrossCitationConflictOracleV2` — call sites 在 144/161/176/188/200/213)+ tool 层 `internal/tool/answer_block_normalize.go::failEmit` 的 schema enum 校验。

**所有 validator 都是 PRECISE 信号**(typed enum / index 比对 / substring match in evidence pool),无 score / heuristic — 符合本仓库红线 `feedback_precise_signals_for_hard_gates.md`。

**事实 2**:`internal/analysis/hint/composer.go::summariseExactFix`(line 374-440)真实有 **15 个 explicit case + 1 default fallback**(grep `case types.Viol` 数 25 是因为 6 个 case 复合多 ViolKind)。每个 explicit case 给出 60-300 字符的 actionable hint(直接告诉 LLM "Attach claim annotations on the correct V2 carrier: use block-level `claim_uses=[{claim_form=<one-of-allowed>}]` ..." 这种细节)。Hint 通过 `internal/context/builder.go:419-424` 作为 user section **第一段**渲染给 LLM,LLM 不可能漏看。

**事实 3**:`internal/orchestrator/repair_cooccurrence.go::defaultCooccurrenceRules`(line 83+)真实有 **9 条 Primary→Derived 规则**把多 violation 折叠成单一根因(例:`ViolBlockCoverageMissing` 是 Primary,自动吃掉 `ViolPrincipalClaimUseMissing` / `ViolDiagramEdgeUnsupported` / `ViolUncertaintyBlockMissing` 三个 Derived),避免 retry hint 用一堆衍生 violation 噪音淹没 LLM。

**结论**:删 prompt 重复规则不是裸奔;删后 validator + hint composer + cooccurrence rule 三层联防仍然可保结构正确,LLM 在 first-emit reject 后 1-2 retry 内可完成结构修复 — 这个 retry 成本远小于 attention dilution 导致 A'' jitter 的 cost。

### 1.3 关键风险(必须前置消除)

来自 hint composer 真实 case 审计(2026-05-05):

- **R1 — 部分规则的 hint 走 DEFAULT fallback**:hint composer 15 explicit case 中,`ViolEnumerationLabelUngrounded` 和 `ViolAbsenceScopeExceeded` 等不在专用 case 列表,触发时走 `"Address the violation(s) listed above and re-emit."` 这种 generic fallback。LLM 收不到"如何修"的具体教学。**Mitigation**:Phase A Commit 1 显式加这 2 个新 case + 增强 1 个 existing case,让被删规则对应的 Violation 都有 actionable hint(详见 §4)。
- **R2 — Cooccurrence 覆盖 9 条**:覆盖最常见组合,但仍有 ~30% violation 组合无 Primary 折叠 → 多 retry 同时面对多 root cause hint。**Mitigation**:Phase A 暂不扩 cooccurrence(留给 Phase B 改进),只挑选**当前 cooccurrence 已覆盖或 Commit 1 增强后 hint 充足的高频规则**优先删除,其余规则保留。
- **R3 — 跨 case 回归**:m1a / s1a / u3a / qf_config_precedence 等已 PASS 的 case 可能在删规则后第一轮 emit 失败率上升。**Mitigation**:eval bar 跑 4 case x4 = 16 次,任一回归 → 该次 commit revert,该规则保留。

---

## 2. 代码定位指南(让不熟代码的开发者立即上手)

下面所有文件路径都是相对 `/home/chatpp/codrax`。**所有 file:line 经真实代码审计验证(baseline `4cd053d`)**。

### 2.1 Skill 注册(prompt 内容所在地)

| 文件 | 关键 line | 内容 |
|---|---|---|
| `internal/skill/defaults.go:115-320` | 整段 | `answer-document-skill` 注册 |
| `internal/skill/defaults.go:118-143` | 24 条 | Workflow numbered list |
| `internal/skill/defaults.go:152-307` | ~155 行 | OutputFormat 散文(包含表格 / mermaid 示例) |
| `internal/skill/defaults.go:308-319` | 10 条 | Prohibitions bullet list |
| `internal/skill/skill.go:8-53` | struct | `Config{Name, Goal, Workflow, OutputFormat, Prohibitions, ToolSuggestions}` 定义 + `Registry` 单例 |

**OutputFormat §-标题真实存在性核对**(KEEP-COMPACT 决策依赖这些 anchor):

| §-标题 | line | 内容 |
|---|---|---|
| `## Tool choice` | line 154 | first-dispatch / retry-path 选择 emit_answer_document vs patch |
| `## Block contract` | line 159 | blocks[] 通用规约 |
| `## Block-kind payloads` | line 168 | 9 种 block kind 的 payload 表 |
| `## Claim annotations` | line 184 | claim_uses 4 字段 schema + 9 claim_form |
| `## Diagram contract` | line 205 | diagram.kind / language / body 规约 |
| `## Citation pool` | line 219 | citations[] / citation_ref 规约 |

### 2.2 Skill → LLM prompt 渲染管线

| 文件 | 关键 line | 作用 |
|---|---|---|
| `internal/context/builder.go:384-403` | 整段 | 把 `Workflow / OutputFormat / Prohibitions` 渲染成 system sections |
| `internal/context/builder.go:419-424` | 整段 | RetryHint 作为 user section **第一段**(SectionRetryDirective) |
| `internal/agent/answer_document_evaluator.go::BuildInitialInstruction`(line 127+) | 整段 | 构造 dynamic per-dispatch user section |

### 2.3 Validator 管线(machine-check 的实质所在地)

| 文件 | 关键 line | 作用 |
|---|---|---|
| `internal/orchestrator/contract_check.go:58-246` | `runContractCheck()` | 顶层入口,在每次 finalizer emit 后运行 |
| `internal/orchestrator/contract_check.go:111-163` | V2 dispatch | 调用 `runV2BlockOraclesWithMut`(line 126)+ 4 个独立 call(`validateEnumerationEvidenceCoverage` line 136 / `runStructuralEnumerationDivergenceOracleV2` line 143 / `runSymbolAnchorTrackOracleV2` line 151 / `runCrossCitationConflictOracleV2` line 160)|
| `internal/orchestrator/contract_check_block.go:1681-1706` | `runV2BlockOraclesWithMut` | 13 个 V2 validator 装配 |
| `internal/orchestrator/contract_check_block.go:71-1860` | 各 `validate*` 实现 | 13 个 validator 本体 |
| `internal/tool/answer_block_normalize.go:48-105` | 整段 | tool 层 schema enum 校验(diagram.kind / block.kind / 必填字段) |
| `internal/tool/emit_answer_document_v2.go:107-112` | strict-decode | citation_ref 不能进 claim_use / 未知字段 reject |

### 2.4 Hint composer + cooccurrence

| 文件 | 关键 line | 作用 |
|---|---|---|
| `internal/analysis/hint/composer.go:142-201` | `Compose()` | 顶层入口,接 violations 出 6 字段 Hint struct |
| `internal/analysis/hint/composer.go:202-238` | `Render()` | Hint struct → 字符串(无长度截断,Phase A 增强可放心扩文本) |
| `internal/analysis/hint/composer.go:374-440` | `summariseExactFix` | **真实 15 explicit case + 1 default**,本 Phase Commit 1 加 2 case + 改 1 case |
| `internal/analysis/hint/composer.go:445-540` | `buildAllowedSet` | 给 LLM 列举允许值(claim forms / file list / block kinds)|
| `internal/orchestrator/repair_cooccurrence.go:83-337` | `defaultCooccurrenceRules` | 9 条 Primary→Derived 折叠规则 |
| `internal/types/violation.go:299` | const | `ViolAbsenceScopeExceeded`(`absence_scope_exceeded`)真实 ViolKind 名 |

### 2.5 Finalizer reject signal pathway(非 V2 validator 路径)

| 文件 | 关键 line | reject code |
|---|---|---|
| `internal/agent/answer_document_evaluator.go:2762` | `missing_diagram` | hint 254 字详细 |
| `internal/agent/answer_document_evaluator.go:2770` | `diagram_grounding` | 456 字 + allowed_labels metadata |
| `internal/agent/answer_document_evaluator.go:2783` | `diagram_codename` | 319 字 |
| `internal/agent/answer_document_evaluator.go:2791` | `exact_context_surface` | 动态(repair.Metadata)|
| `internal/agent/answer_document_evaluator.go:2811` | `exact_resolution` | 318 字 |
| `internal/agent/answer_document_evaluator.go:2815` | `log_source_drift_step_citation` | 叠加 hint |
| `internal/agent/answer_document_evaluator.go:2887` | `follow_on_grounded_context` | repair.Hint |
| `internal/agent/answer_document_evaluator.go:2900` | `log_triage_coverage` | repair.Hint |
| `internal/agent/answer_document_evaluator.go:2904` | `scalar_summary_required` | repair.Hint |
| `internal/agent/answer_document_evaluator.go:2918` | `literal_grounding` | 含 `citation_ref=-1` 逃生舱 |
| `internal/agent/answer_document_evaluator.go:3119` | `appendRetryDiagramSeedHint` | seal 路径专用 hint |

### 2.6 测试影响面

| 文件 | 关键测试 | 影响 |
|---|---|---|
| `internal/skill/defaults_test.go:27-193` | 6 个测试函数对 finalizer skill 做 substring assert | 详见 §6 |
| `internal/skill/keyword_examples_test.go` | LLM-facing keyword 红线 | 不直接锁规则,删规则不影响 |
| `internal/skill/glossary_test.go` | glossary 测试 | 不影响 |
| `internal/agent/answer_document_evaluator_test.go:33-71,2976+` | 多个对 OutputFormat 内容的断言 | 详见 §6;锁 dynamic dispatch 文本而非 static skill |
| `internal/orchestrator/contract_check_block_test.go` | 13 个 validator unit test | **不动** — validator 行为不变 |
| `internal/analysis/hint/composer_test.go` | 15 case 各自 hint 文本断言 | **更新**(Commit 1 加 2 新 case + 改 1 existing case)|
| `internal/orchestrator/llm_facing_jargon_audit_test.go` | LLM-facing 术语红线 | 删规则减少 jargon 应过更绿;Commit 1 增强 hint 文本需 grep 验证不含内部术语 |

**跨文件引用核对**(`grep -rn "abstraction-level matching\|sealed-seed\|bucket alignment\|absence-citation" docs/ CLAUDE.md`):
- **MEMORY.md / CLAUDE.md / 其他 design doc 没有引用规则名** — Phase A 删规则**零 cross-document sync 成本**
- 规则名只在 Phase A 设计文档自身和 commit `efa4ff3` 的 commit message 中出现

---

## 3. 24 条 Workflow 规则的逐条决策表

下表把 `defaults.go:119-142` 每条规则映射到对应 validator + 真实代码审计后的决策。**决策四档**:
- **DELETE-UNCONDITIONAL**:规则纯属重复 validator 已 hard-check + hint composer 已有 actionable case,直接删
- **DELETE-CONDITIONAL**:规则可删,但**前置条件**是 Commit 1 的 hint 增强必须先 land(否则 LLM 收到 DEFAULT fallback 难以收敛)
- **KEEP-COMPACT**:规则不能删(语义判断或 schema 教学),但可大幅压缩(去重 / 用 §-锚点引用 OutputFormat)
- **KEEP-FULL**:规则保留全文(LLM 判断题 — A'' / subject discipline / authority discipline / divergence / item.kind count discipline)

| # | 规则摘要 | machine-check by | 决策 | hint case 状态 |
|---|---|---|---|---|
| 1 | 答案直接写进 emit_answer_document | tool 层强制(无 emit 即 fail)| **KEEP-COMPACT**(1 句话压缩)| n/a |
| 2 | Required Answer Blocks 必须满足 | `validateRequiredBlockCoverage`(cb:71)| **DELETE-UNCONDITIONAL** | ✓ `ViolBlockCoverageMissing`(composer.go:430,177 字详细)|
| 3 | blocks[] 是 carrier(kind/payload 规约)| tool 层 schema + OutputFormat §Block-kind payloads(line 168)| **KEEP-COMPACT**(引用 §)| n/a |
| 4 | claim_uses 在 principal block 必填 | `validatePrincipalClaimUse`(cb:134)| **DELETE-UNCONDITIONAL** | ✓ `ViolPrincipalClaimUseMissing`(composer.go:410,142 字详细)|
| 5 | edge_anchors block 级数组 schema | `validateDiagramEdgeSupport`(cb:204)只 check kind/edge grounding,placement 由 tool strict-decode 拒 | **DELETE-CONDITIONAL** | ⚠️ `ViolDiagramEdgeUnsupported` 已存在但未明示 "edge_anchors[] never inside claim_use" — Commit 1 增强 |
| 6 | diagram.kind 是语义 family | `answer_block_normalize.go:52-56` enum | **DELETE-UNCONDITIONAL** | ✓ `ViolDiagramEdgeUnsupported` hint(composer.go:416)显式说"NOT a Mermaid keyword" |
| 7 | enumeration ordered_list label verbatim | `validateEnumerationItemLabelGrounding`(cb:1746)+ `validateEnumerationItemLabelExtractorMatch`(cb:1858)| **DELETE-CONDITIONAL** | ⚠️ `ViolEnumerationLabelUngrounded` 走 DEFAULT fallback — Commit 1 加新 case |
| **8** | **A'' — abstraction-level matching for role-enumeration** | **❌ 无** — 纯 LLM 判断题 | **KEEP-FULL** | n/a — Phase A 主战场是让这条规则获得更多注意力 |
| 9 | hop-chain ordered_list 每 item 是一 hop,text 是 behavior | ❌ 无(prose voice 是判断题)| **KEEP-FULL** | n/a |
| 10 | item.kind ∈ {principal/flow/caveat},declared count 只数 principal | tool 层 enum check(`answer_block_normalize.go:76-79`)有,**但"only principal items count"约束 NO validator** | **KEEP-FULL** | n/a — count discipline 是判断题,validator 不覆盖 |
| 11 | bucket alignment(用户 label verbatim)| `validateMissingRequestedRoleDisclosure`(cb:1557)只 check 文档级字段,**不 check prose 出现** | **DELETE-CONDITIONAL** | ⚠️ `ViolMissingRequestedRoleUndisclosed` 已存在但未提 bucket label 要 verbatim — Commit 1 改 |
| 12 | scalar block 规约(literal 在 block.text)| `answer_block_normalize.go` schema | **KEEP-COMPACT** | n/a — schema 教学 |
| 13 | decision block 规约(verdict + rationale)| `answer_block_normalize.go` schema | **KEEP-COMPACT** | n/a |
| 14 | enumeration item file:line 必须真 anchor | `validateEnumerationItemLabelGrounding`(cb:1746)check LABEL substring,**不 check file:line 存在性**(grounder 在 render 时 check)| **DELETE-CONDITIONAL** | ⚠️ Commit 1 增强的 `ViolEnumerationLabelUngrounded` 必须显式说"do NOT invent file:line" |
| 15 | summary-only 答案规约(### 子标题)| ❌ 无(prose 风格判断题)| **KEEP-FULL** | n/a |
| 16 | citations[] 唯一 declare,citation_ref 是 index,-1 表无 cite | "Declare ONCE" 无 validator(LLM 判断);"citation_ref NEVER in claim_use" tool 层 strict-decode 拒 | **SPLIT — 半 KEEP-COMPACT 半 DELETE** | n/a |
| 17 | log-triage 答案 prefer hop-chain | ❌ 无(策略选择题)| **KEEP-FULL** | n/a |
| 18 | 文件形 token 必须在 citations 里(diagram-grounding gate)| `answer_document_evaluator.go:2770`(diagram-grounding gate)| **DELETE-UNCONDITIONAL** | ✓ reject signal `diagram_grounding` 含 allowed_labels metadata,456 字 |
| 19 | Sealed-seed rule(diagram 中 file:line verbatim copy)| `appendRetryDiagramSeedHint`(ade:3119)| **DELETE-UNCONDITIONAL** | ✓ 已有专用 retry hint 段 |
| 20 | log-triage error.Type 必须 verbatim 出现 | `runExternalArtifactDecodedCheck`(func def cc:248,call site cc:176)| **DELETE-UNCONDITIONAL** | ✓ rationale 列举 missing tokens verbatim(in func body 248+)|
| 21 | Subject discipline(无关同名 file 不进 summary)| ❌ 无(语义判断)| **KEEP-FULL** | n/a |
| 22 | Authority discipline(drift bound,显式归属)| ⚠ `runAuthorityOverreachCheck`(func def cc:310,call site cc:188)check caveat presence,**prose attribution 是判断题** | **KEEP-FULL** | n/a — partial machine-check 但 prose 部分必须 LLM 判断 |
| 23 | Code-vs-narrative divergence(不静默选边)| ⚠ `runStructuralEnumerationDivergenceOracleV2`(func def cc:818,call site cc:144)detect divergence,**prose 阐述是判断题** | **KEEP-FULL** | n/a — 同上 |
| 24 | Absence-citation discipline(status='absent' 必带 negative-scope)| `validateAbsenceScopeBound`(cb:1519)| **DELETE-CONDITIONAL** | ⚠️ `ViolAbsenceScopeExceeded`(violation.go:299)走 DEFAULT fallback — Commit 1 加新 case |

**汇总**(总计 24 条 — rule #16 仅在 SPLIT 列表内,SPLIT 自带"半保留半删除"语义,**不在 KEEP-COMPACT 重复计入**):
- **DELETE-UNCONDITIONAL = 6 条**(#2, #4, #6, #18, #19, #20)— 直接删,hint 已 actionable
- **DELETE-CONDITIONAL = 5 条**(#5, #7, #11, #14, #24)— 必须先做 Commit 1 hint 增强才删
- **SPLIT = 1 条**(#16)— "Declare ONCE" 保留,"citation_ref NEVER in claim_use" 删
- **KEEP-COMPACT = 4 条**(#1, #3, #12, #13)— 合并/压缩为 2-3 条 schema-mechanics 规则
- **KEEP-FULL = 8 条**(#8, #9, #10, #15, #17, #21, #22, #23)— 核心判断题,每条独立保留 + 加 §-锚点
- **总和 = 6+5+1+4+8 = 24** ✓

**Workflow 净规模**:24 → ~13(KEEP-FULL 8 + KEEP-COMPACT 合并 2-3 + SPLIT 半保留 ~1)。A''(#8)在新 list 中份额从 1/24 → ~1/13。

### 3.1 Prohibitions 10 条决策

| Prohibition 摘要(line 308-319) | 决策 | 已 machine-check |
|---|---|---|
| 不写 prose 在 emit 外 | **KEEP**(合并到 #1)| tool 层无法 check 但 system 提示 |
| 不 cite 不在 evidence 的 file | **DELETE-UNCONDITIONAL** | `ViolCitation`(composer.go:391)已有 actionable case |
| 不发明 line 号 | **DELETE-UNCONDITIONAL** | `ViolGhostAnchor`(composer.go:393)|
| quote 不能放 prose | **DELETE-UNCONDITIONAL** | grounder 自动 strip(已 inline 注释)|
| 不预压缩 prose | **KEEP**(LLM 判断题)| n/a |
| citation_ref 不用 zero-sentinel | **DELETE-UNCONDITIONAL** | tool 层 schema |
| 不静默截 bounded set | **KEEP**(与 #15 合并)| n/a — completeness 是判断题 |
| 不发明 codename labels | **KEEP**(LLM 判断题)| n/a |
| 不漏 claim_uses | **DELETE-UNCONDITIONAL**(重复 #4)| `ViolPrincipalClaimUseMissing` |
| 不在 diagram.kind 写 mermaid keyword | **DELETE-UNCONDITIONAL**(重复 #6)| tool 层 schema |

**汇总**:Prohibitions 10 → 4 条(删 6 条无条件)。**注**:Agent 验证 `defaults_test.go` 中 0 个测试锁 Prohibitions 文本 — Commit 5 (Prohibitions 删除) 无测试更新成本。

### 3.2 OutputFormat 内嵌指令处理

`defaults.go:152-307` 的 OutputFormat ~155 行散文,内嵌 ~50 个隐性 directive。**Phase A 不动 OutputFormat 主体**(它的 schema 教学是 KEEP-COMPACT 类),只做以下修改:

- 保留所有 §-标题(已核对存在,见 §2.1 表)— 这些是 KEEP-COMPACT 规则的引用目标
- `## Per-block prose guidance`(line 242-248)**不动** — 是 KEEP-FULL 类(prose voice 判断)
- `## Visual structure`(line 250-289)**不动** — mermaid 教学 schema-level
- `## Enumeration completeness and bounded sets`(line 227-232)已被 commit `5388410` 简化,不动

---

## 4. Hint 增强清单(Commit 1 — 必须先 land 才能做条件 DELETE)

`internal/analysis/hint/composer.go::summariseExactFix`(line 374-440)真实 15 explicit case + 1 default。Phase A 加 **2 个新 case + 改 1 个 existing case**。

### 4.1 新 case 1:`ViolEnumerationLabelUngrounded`(为规则 #7 + #14 删除作准备)

**当前**:走 DEFAULT fallback(`"Address the violation(s) listed above and re-emit."`)
**问题**:删规则 #7 + #14 后,LLM 看到 reject 不知具体怎么修。
**新增**(在 `ViolMissingRequestedRoleUndisclosed` case 后、default 前插入):

```go
case types.ViolEnumerationLabelUngrounded:
    return "Each ordered_list item's `label` MUST appear verbatim in at least one EvidenceItem (anchor_symbol / subject / object). The validator does case-folded substring match in BOTH directions. Cited evidence pool is in the user section under `## Prior Evidence Slate` — pick label tokens from those entries. Each item's `citation_ref` MUST point at a real file:line that already appears in citations[] AND that the label token also appears at; do NOT invent file:line combinations that do not exist in the evidence pool. If the candidate symbol does not appear in evidence, you cannot include it; either drop the item, or revisit explorer findings."
```

**buildAllowedSet 同步**:加 `case types.ViolEnumerationLabelUngrounded` 返回 evidence pool 中的 anchor_symbol 列表(从 `ctx.ReadSet` 或新加 `ctx.EvidenceAnchors`)。

### 4.2 新 case 2:`ViolAbsenceScopeExceeded`(为规则 #24 删除作准备)

**真实 ViolKind 名验证**:`internal/types/violation.go:299` `ViolAbsenceScopeExceeded ViolationKind = "absence_scope_exceeded"`
**当前**:走 DEFAULT fallback。Validator 自带 Repair 文本(`contract_check_block.go:1539`)但 composer 未提取。
**新增**(同位置插入):

```go
case types.ViolAbsenceScopeExceeded:
    return "When `exact_resolution.status='absent'`, the citations[] array MUST include at least one entry with `scope='negative'` AND a non-empty `negative_pattern` field naming the EXACT search query whose absence-of-matches confirms the finding (the literal pattern you ran with grep / repo_map / search, or the missing identifier itself). Schema template: `{file: '<file-or-repo-wide-marker>', scope: 'negative', negative_pattern: '<exact-query-you-ran>'}`. The `file` field can be a literal repo path or `(repo-wide grep)` for repo-wide searches; `line` may be 0 for negative scope."
```

### 4.3 改 existing case:`ViolMissingRequestedRoleUndisclosed`(为规则 #11 删除作准备)

**当前**(composer.go:434-438):
```go
case types.ViolMissingRequestedRoleUndisclosed:
    return "Populate document-level `missing_requested_roles[]` exactly from the semantic-view contract for this dispatch. Each entry is `{role:<default|config|runtime|override>, label?:<user-facing bucket name>}`; preserve any surfaced labels (for example `CLI`) and do not replace missing layers with vague prose like `N/A` or `not applicable`."
```
**问题**:未提 bucket alignment(规则 #11 的核心)— LLM 不知道 bucket label 要 VERBATIM 出现在 user-facing 答案字段。
**改后**:
```go
case types.ViolMissingRequestedRoleUndisclosed:
    return "Populate document-level `missing_requested_roles[]` exactly from the semantic-view contract for this dispatch. Each entry is `{role:<default|config|runtime|override>, label?:<user-facing bucket name>}`; preserve the exact bucket labels and do not replace missing layers with vague prose like `N/A` or `not applicable`. **For bucket-style questions ('X for A, Y for B'), every bucket label from QuestionStructure.Buckets MUST appear VERBATIM in the rendered answer's user-facing fields** — preferred form: each bucket as a `### <Label>` section heading inside summary, OR its own section block, OR mentioned inside relevant items[].text."
```

### 4.4 测试更新

`internal/analysis/hint/composer_test.go` 加 3 个 substring assert(2 新 case + 1 改 case 的关键文本)。

### 4.5 LLM-facing jargon 红线 check

Commit 1 增强后必 grep `internal/orchestrator/llm_facing_jargon_audit_test.go` 的 blocklist 验证新 hint 文本不含内部术语(`validator / Viol / evidence pool / amplifier / R4` 等只在系统侧用的词)。

---

## 5. Commit 序列(单 session 6-9 commits)

每条 commit 必须独立可 revert。Eval 在 commit 5 之后整批跑。

### Commit 1 — Hint enrichment(2 新 case + 1 改 case)

- 改 `internal/analysis/hint/composer.go:374-440`:
  - 加 `case types.ViolEnumerationLabelUngrounded`(在 #15 `ViolMissingRequestedRoleUndisclosed` 后)
  - 加 `case types.ViolAbsenceScopeExceeded`
  - 改 `case types.ViolMissingRequestedRoleUndisclosed`(加 bucket alignment 段)
- 改 `internal/analysis/hint/composer.go:445-540` `buildAllowedSet`:加 `ViolEnumerationLabelUngrounded` case 返回 evidence pool anchor 列表
- 改 `internal/analysis/hint/composer_test.go` 新增 3 substring assert
- LLM-facing jargon audit:grep 验证新文本不含系统内部术语
- 不改 prompt — 这是纯增强
- **Eval**: composer test 全绿。无真 eval 需求(只是 hint composer 增强,无行为改动到 finalizer)。

### Commit 2 — Workflow rule DELETE 第一批(无条件 6 条)

删 `defaults.go:119-142` 中以下规则:

- 规则 #2(Required Answer Blocks)— validator + hint 已完整 cover
- 规则 #4(claim_uses 在 principal block 必填)— validator + hint 已完整 cover
- 规则 #6(diagram.kind 语义 family)— schema enum 已 enforce
- 规则 #18(diagram-grounding 文件 token)— reject signal 已带 allowed_labels
- 规则 #19(sealed-seed)— 有专用 retry hint
- 规则 #20(log-triage error.Type verbatim)— rationale 已列 missing tokens

更新 `defaults_test.go` 删去对应 substring assert(详见 §6 表)。

**Eval**: m1a x4 + qf_arch x4 = 8 次。任一 case 在 retry 内不收敛 → revert,该规则保留。

### Commit 3 — Workflow rule SPLIT(规则 #16)

- 拆规则 #16:保留"Declare every file:line ONCE in citations[]"(LLM 判断题),删"citation_ref NEVER inside claim_use"(tool 层 strict-decode 拒)
- 改 `defaults_test.go` 对应断言

**Eval**: 4 case x4 = 16 次。低风险,纯文本拆分。

### Commit 4 — Workflow rule DELETE 第二批(条件,基于 Commit 1)

删 #5、#11、#24:

- 规则 #5(edge_anchors block 级数组)— Commit 1 增强 `ViolDiagramEdgeUnsupported` hint 已说 placement
- 规则 #11(bucket alignment)— Commit 1 增强 `ViolMissingRequestedRoleUndisclosed` 已加 bucket VERBATIM 段
- 规则 #24(absence-citation discipline)— Commit 1 加 `ViolAbsenceScopeExceeded` 新 case

**Eval**: 4 case x4 = 16 次。必须验证 Commit 1 hint 增强的 actionability。

### Commit 5 — Workflow rule DELETE 第三批(高风险:enumeration label)

删规则 #7 + #14:

- 规则 #7(enumeration item.text 规约)— 第一句保留(item.text 是短自然 prose 描述 ROLE)— 因为这是 KEEP-FULL 类的判断教学;只删后半 schema 重述部分(claim_use + claim_form 各种 enum 列举)
- 规则 #14(enumeration item file:line anchor)— 全删,Commit 1 增强 `ViolEnumerationLabelUngrounded` 已说"do NOT invent file:line"

**Eval**: 4 case x4 = 16 次。这一批最高风险 — 因为 #7 / #14 直接关联 A'' 所在的 enumeration 路径。

### Commit 6 — Prohibitions 同步删除 6 条

删 §3.1 决策表中标 DELETE-UNCONDITIONAL 的 6 条 Prohibitions。

**Eval**: 同 commit 5(4 case x4)。**Agent 验证 `defaults_test.go` 0 个 substring 锁 Prohibitions 文本 → 无测试更新成本**。

### Commit 7 — Workflow rule §-锚点重命名 + 主题分组 header

把 KEEP-FULL 8 条 + KEEP-COMPACT 合并 2-3 条 + SPLIT 半保留 ~1-2 条 = ~13 条 Workflow rule 加 `§B.2` / `§C.1` / `§G.1` 这种锚点前缀,方便后续 hint 引用。在 list 头部加 1-2 行说明:

```
Workflow rules below are organized into thematic groups:
  §A: Schema mechanics (what fields go where)
  §B: Enumeration semantics (what items mean)
  §C: Hop-chain semantics
  §D: Scalar / Decision shapes
  §E: Summary-only shape
  §F: Log-triage / Diagram grounding
  §G: Universal honesty (subject / authority / divergence / item-kind discipline)
```

**Eval**: 4 case x4 = 16 次。本 commit 不应引入语义改动,纯重命名 + 顺序调整。

### Commit 8 — 真 eval 验收 + memory 收尾

- 跑 qf_arch x4 + m1a x4 + s1a x4 + u3a x4 + qf_config_precedence x4 = 20 次
- 写 `feedback_eval_pass_is_not_green.md` 已锁红线下的真 eval verdict 表
- 更新 `MEMORY.md` 顶部为本 Phase A SHIPPED + 指向 Phase C
- 更新本文档 Status: `SHIPPED <date>`

### (可选)Commit 9 — 如 §6 中任何 case 出现 jitter,补打 patch

---

## 6. 测试影响详表(`defaults_test.go` 6 个测试 + `answer_document_evaluator_test.go`)

`internal/skill/defaults_test.go` 真实 6 个测试函数对 `answer-document-skill` 做 substring assert:

| 测试函数(真实 line) | 真实锁定内容(grep 验证) | 真实对应规则 | Phase A 影响 |
|---|---|---|---|
| `TestFinalizerSkillStepListPrefersDiagramsWhenHelpful`(line 27)| 锁 OutputFormat:`Even when the Diagram Contract does NOT require one` / `3+ hops` / `actor/role handoffs` / `easier to see than to read in prose` | OutputFormat | **SAFE** — Phase A 不动 OutputFormat 主体 |
| `TestFinalizerSkillKeepsInternalJargonOutOfUserProse`(line 47)| 锁 OutputFormat 含 internal-jargon 教学:`Keep internal pipeline jargon out of the user-facing prose` / `"grounded"` / `'grep' / 'read_file' / 'repo_map' found nothing.` | OutputFormat | **SAFE** — Phase A 增强 hint 文本时必避开锁的内部词 |
| `TestFinalizerSkill_DoesNotTeachRetiredV1AnswerPayloads`(line 66)| 锁 Workflow + OutputFormat **不含**退役 V1 词汇(`value{literal,citation_ref}` / `boolean.rationale` / `symbols_completeness` 等)| Workflow + OutputFormat | **SAFE** — Phase A 增加纯化(删规则)反而更绿 |
| `TestFinalizerSkill_TeachesTypedDiagramRelationAuthority`(line 100)| 锁 **规则 #5** edge_anchors+relation_kind 教学:`edge_anchors is the OPTIONAL block-level array` / `relation_kind?: <one of call|guard|import|precedence|contain|observe>` / `PREFERRED: set relation_kind directly` / `the authoritative semantic relation` — **4/4 substring 全在 line 123 (rule #5)**(grep 验证)| 规则 #5 | **MUST VERIFY 删 #5 (commit 4) — 测试会 FAIL**;删 #5 前必须先把这 4 条 substring 等价内容补到 OutputFormat(§Block-kind payloads 或新加 §Edge anchors 子段),或同步更新此测试 |
| `TestFinalizerSkill_ClarifiesFacetIDAndVerticalDiagramPreference`(line 121)| **主要锁 OutputFormat**(§Claim annotations + §Diagram contract):`claim annotations use singular \`facet_id\`` 在 rule #4 (line 122) AND OutputFormat (line 203) 双重出现;`plural \`facet_ids\` belongs on the block`(lowercase block)**仅在 OutputFormat (line 203,runtime concat 后)** — rule #4 (line 122) 是 BLOCK 大写;`\`flowchart TD\` by default` (line 211, OutputFormat);`keep participant labels short because actors render horizontally` (line 211, OutputFormat)— **3/4 substring 仅在 OutputFormat,1/4 双重出现** | OutputFormat 主体(非 Workflow rule #4 / #5)| **基本 SAFE** — 删 #4 (commit 2) 后 substring "claim annotations use singular `facet_id`" 仍在 OutputFormat (line 203);其他 3 substring 全在 OutputFormat 不受 Workflow 删除影响。但仍建议 commit 后跑此测试确认 |
| `TestFinalizerSkill_TeachesAbstractionLevelMatching`(line 154)| 锁 Workflow 含 abstraction-level matching trigger:`what does each X do` / `每个 X 负责什么`(规则 #8 / A'')| 规则 #8 | **KEEP & EXPAND** — 加 `// task #8 ref` 注释 |

**验证策略**(基于真实 substring 来源):
- **Commit 4 实施前**(删规则 #5)— `TestFinalizerSkill_TeachesTypedDiagramRelationAuthority` 4/4 substring 全锁 rule #5 → **删 #5 后此测试必 FAIL**。必须先把 `edge_anchors is the OPTIONAL` / `relation_kind?: <one of call|guard|import|precedence|contain|observe>` / `PREFERRED: set relation_kind directly` / `the authoritative semantic relation` 等价内容补到 OutputFormat(`§Block-kind payloads` 或新加 `§Edge anchors` 子段),再删 Workflow rule #5。
- **Commit 2 实施前**(删规则 #4)— `TestFinalizerSkill_ClarifiesFacetIDAndVerticalDiagramPreference` 主要 substring 已在 OutputFormat,删 #4 后大概率 PASS;但 commit 后仍跑此测试验证。
- **Commit 1 增强 hint 文本时** — `TestFinalizerSkillKeepsInternalJargonOutOfUserProse` 锁 `Keep internal pipeline jargon out of the user-facing prose` 等 substring,新加 hint 不要踩到 jargon banlist(grep 双向确认)。

`internal/agent/answer_document_evaluator_test.go` 中对 OutputFormat 的断言(line 33-71, 2976+)— Agent 验证这些锁的是 **dynamic dispatch 文本而非 static skill 定义** → Phase A 改 static skill 不影响这些测试。

---

## 7. 红线 checklist(开发者必读)

实施 Phase A 前,必须确认理解以下红线(每条都已在 `MEMORY.md` 中):

- 🔴 `feedback_precise_signals_for_hard_gates.md` — Phase A 删的所有规则都对应 PRECISE validator,符合红线
- 🔴 `feedback_redundant_inline_directive_removal.md` — 删 LLM-facing 指令 3 步走:(1)确认对应 validator/hint 已 cover (2)delete (3)eval 验证
- 🔴 `feedback_no_eval_bar_relaxation.md` — 跑出 FAIL 不放宽 case spec
- 🔴 `feedback_no_dismiss_as_llm_flake.md` — 任何 jitter 必须深查根因,不许"LLM flake"草草收场
- 🔴 `feedback_root_cause_only.md` — 删规则若引发新 jitter,根因可能是 hint 不够细 → 加 hint(Commit 1 已留扩展余地),不许通过加 prompt 规则绕回去
- 🔴 `feedback_prompt_redline_checklist.md` — KEEP-FULL 8 条规则若改文本必过 ATOMIC 7 条 R3+R4+R5+R6+R7+SST+R2' checklist
- 🔴 `feedback_no_internal_info_in_llm_prompts.md` — 增强的 hint 文本不能露 system 内部术语(grep `validator` / `Viol` / `evidence pool` 这些只在系统侧用的词);Commit 1 加 LLM-facing jargon audit 验证

---

## 8. 失败回退路径

如果 commit 5(高风险 enumeration label 删除批)在 eval 中导致 m1a / qf_arch 回归且 Commit 1 增强的 hint 仍不够:

- **Plan A**:回退 commit 5,接受 Phase A 净 DELETE = 8 条(commit 2 + 3 + 4 + 6 已删的 6+1+3+6 — 含 Prohibitions)。A'' 注意力份额从 1/24 → ~1/16,仍是有效改善。
- **Plan B**:不回退,但补打一个 commit 进一步增强 `ViolEnumerationLabelUngrounded` hint 文本(加例子 + 反例)。再跑 eval。
- **Plan C**(最差):回退 commit 4 + commit 5,Phase A 收口为 commit 1-3 + 6,准备 Phase B(扩 cooccurrence 规则覆盖)再回头删剩余规则。

---

## 9. Phase A 不做什么(scope discipline)

- ❌ 不动 amplifier(`internal/analysis/amplifier/`)— 那是 analyzer 侧
- ❌ 不动 AnswerSemanticView 结构 — 那是 Phase C
- ❌ 不引入 sub-family enum(role-enum / mechanism-enum)— Phase C
- ❌ 不动 OutputFormat ~155 行散文主体 — 它是 KEEP-COMPACT 类
- ❌ 不动 hint composer 现有 15 case 的 14 个 — 只动 1 个 existing case + 加 2 新 case
- ❌ 不动 retry budget / cooccurrence rule 数量 — Phase B 工作
- ❌ 不删规则 #10(item.kind count discipline)— 那是 LLM 判断题,validator 不覆盖
- ❌ 不删规则 #16 全部 — 拆分为半保留 + 半删除

---

## 10. 真实代码事实速查表(实施时验证用)

下面是本 Phase 涉及的所有真实 code 信号,实施前必 grep 一次确认未漂移:

```bash
# 24 条 Workflow 规则(逐字读)
sed -n '119,142p' internal/skill/defaults.go

# 10 条 Prohibitions
sed -n '308,319p' internal/skill/defaults.go

# OutputFormat §-标题真实存在性
grep -n "^## " <(sed -n '152,307p' internal/skill/defaults.go)

# 13 个 V2 validator 装配
grep -n "validateRequiredBlockCoverage\|validatePrincipalClaimUse\|validateDiagramEdgeSupport\|validateUncertaintyBlockPresence\|validateFacetCoverage\|validateRichnessGlaringGap\|validatePrincipalProseUnderfilled\|validateRichnessRegression\|validateClaimFormSupport\|validateMissingRequestedRoleDisclosure\|validateAbsenceScopeBound\|validateEnumerationItemLabelGrounding\|validateEnumerationItemLabelExtractorMatch" internal/orchestrator/contract_check_block.go

# Hint composer 真实 15 explicit case + 1 default
grep -c "case types.Viol\|^\s*}\s*$\|^\s*return \"Address" internal/analysis/hint/composer.go

# ViolAbsenceScopeExceeded 真实定义
grep -n "ViolAbsenceScopeExceeded\s*ViolationKind" internal/types/violation.go

# ViolEnumerationLabelUngrounded 真实定义
grep -n "ViolEnumerationLabelUngrounded\s*ViolationKind" internal/types/violation.go

# defaults_test.go 锁 finalizer skill 的所有 substring
grep -n "answer-document-skill" internal/skill/defaults_test.go

# answer_document_evaluator.go reject signal codes
grep -n "answerDocRejectCode" internal/agent/answer_document_evaluator.go

# 9 条 cooccurrence rule
grep -n "Primary:\s*types.Viol" internal/orchestrator/repair_cooccurrence.go

# LLM-facing jargon 红线
grep -n "validator\|Viol\|evidence pool" internal/orchestrator/llm_facing_jargon_audit_test.go
```

实施任何 commit 前,先把这些 grep 结果与 §2 / §3 各表对齐。如有漂移,本 Phase 设计需要修正,而不是猜测继续实施。

---

## 11. 验收 checklist(本 session 收口前必跑)

- [ ] commit 序列 1-8 全部 push 到 origin/main(`feedback_confirm_before_push.md`)
- [ ] qf_arch x4 = 4/4(本 Phase 主 bar)
- [ ] m1a x4 = 4/4(不回归)
- [ ] s1a / u3a / qf_config_precedence x4 不回归
- [ ] `go test ./internal/skill/...` 全绿(defaults_test.go 已更新)
- [ ] `go test ./internal/analysis/hint/...` 全绿(composer_test.go 已更新)
- [ ] `go test ./internal/orchestrator/...` 全绿(validator 行为不变,应天然过)
- [ ] `MEMORY.md` 顶部更新到本 Phase SHIPPED
- [ ] 写 `project_session_finalizer_phase_a_shipped.md` 收尾 doc
- [ ] 本设计文档 §0 Status: 改 `SHIPPED <date>`

---

## 附录 A — 24 Workflow 规则 + Validator file:line 速查表

| # | 规则一句话摘要 | Validator file:line | Hint composer case | 决策 |
|---|---|---|---|---|
| 1 | Write directly into emit_answer_document | tool layer | n/a | KEEP-COMPACT |
| 2 | Required Answer Blocks mandatory | contract_check_block.go:71 `validateRequiredBlockCoverage` | composer.go:430 `ViolBlockCoverageMissing` | DELETE-UNCOND |
| 3 | blocks[] is the carrier | tool layer schema | n/a | KEEP-COMPACT |
| 4 | claim_uses required on principal | contract_check_block.go:134 `validatePrincipalClaimUse` | composer.go:410 `ViolPrincipalClaimUseMissing` | DELETE-UNCOND |
| 5 | edge_anchors block-level array | contract_check_block.go:204 `validateDiagramEdgeSupport` | composer.go:416 `ViolDiagramEdgeUnsupported`(增强后)| DELETE-COND |
| 6 | diagram.kind ∈ semantic family enum | answer_block_normalize.go:52-56 | composer.go:416(同 #5)| DELETE-UNCOND |
| 7 | enumeration item label verbatim | contract_check_block.go:1746,1858 | (新加 case,见 §4.1)| DELETE-COND |
| **8** | **A'' abstraction-level matching** | **none — judgment** | n/a | **KEEP-FULL** |
| 9 | hop-chain item describes behavior | none — judgment | n/a | KEEP-FULL |
| 10 | item.kind enum + count discipline | tool layer enum check;**count NO validator** | n/a | KEEP-FULL |
| 11 | bucket label verbatim | contract_check_block.go:1557 `validateMissingRequestedRoleDisclosure` | composer.go:434 `ViolMissingRequestedRoleUndisclosed`(增强后)| DELETE-COND |
| 12 | scalar block schema | answer_block_normalize.go schema | n/a | KEEP-COMPACT |
| 13 | decision block schema | answer_block_normalize.go schema | n/a | KEEP-COMPACT |
| 14 | enumeration item file:line real anchor | contract_check_block.go:1746(label only)+ grounder render-time | (新加 case,见 §4.1)| DELETE-COND |
| 15 | summary-only structure | none — judgment | n/a | KEEP-FULL |
| 16 | citations[] declared once + citation_ref placement | "ONCE" judgment / "placement" tool schema | n/a | SPLIT(半 KEEP / 半 DELETE)|
| 17 | log-triage prefer hop-chain | none — judgment | n/a | KEEP-FULL |
| 18 | file-shaped tokens in citations | answer_document_evaluator.go:2770 | reject signal `diagram_grounding`(456 字)| DELETE-UNCOND |
| 19 | sealed-seed verbatim | answer_document_evaluator.go:3119 `appendRetryDiagramSeedHint` | dedicated retry hint | DELETE-UNCOND |
| 20 | log-triage error.Type verbatim | `runExternalArtifactDecodedCheck` (func def cc:248, call site cc:176) | rationale lists missing tokens | DELETE-UNCOND |
| 21 | subject discipline | none — judgment | n/a | KEEP-FULL |
| 22 | authority discipline (drift) | `runAuthorityOverreachCheck` (func def cc:310, call site cc:188) — partial caveat-presence check | n/a — prose attribution judgment | KEEP-FULL |
| 23 | code-vs-narrative divergence | `runStructuralEnumerationDivergenceOracleV2` (func def cc:818, call site cc:144) — partial structural divergence | n/a — prose narrative judgment | KEEP-FULL |
| 24 | absence-citation discipline | contract_check_block.go:1519 `validateAbsenceScopeBound` | (新加 case,见 §4.2 — 真实 ViolKind `ViolAbsenceScopeExceeded` violation.go:299)| DELETE-COND |

`cb` = `contract_check_block.go`, `cc` = `contract_check.go`, `ade` = `answer_document_evaluator.go`.

---

## 附录 B — 决策表 cross-reference

`MEMORY.md` 索引中本设计的位置:`docs/design/finalizer_phase_a_rule_bisection.md`。Phase A 完成 ship 后,在 `MEMORY.md` 顶部增加:

```
- 🟢 [**finalizer Phase A — rule bisection SHIPPED (<date>, N commits)**](project_session_finalizer_phase_a_shipped.md) — 删 11 条 machine-checkable Workflow rule(6 unconditional + 5 conditional after hint enrichment)+ 6 条重复 Prohibition + SPLIT 1 条 + 增强 hint composer 3 case(2 新 + 1 改)。A'' 注意力份额从 1/24 → ~1/13。qf_arch x4 = 4/4 PASS。下一阶段:Phase C SHAPE_CONTRACT typed sub-family enum。
```

并加一条 cross-session red line(如果新发现):

```
- 🔴 [**删 prompt 规则前必须查 hint composer 是否有 actionable case**](feedback_prompt_delete_requires_hint_case.md) — Phase A SHIPPED 经验:DEFAULT fallback 不算 actionable
```

---

## 附录 C — 实施 quirk 备注(2026-05-05 真实代码审计)

1. **Hint composer 真实 case 数**:`summariseExactFix`(composer.go:374-440)有 **15 explicit case + 1 default**(grep `case types.Viol` 数 25 是因为 6 个 case 复合多 ViolKind,如 `case ViolFamilyMismatch, ViolViewSwap`)。
2. **`ViolAbsenceScopeExceeded` 真实定义**:`internal/types/violation.go:299` `ViolAbsenceScopeExceeded ViolationKind = "absence_scope_exceeded"`。**Phase A Commit 1 加新 case 时用此名,不要用其它猜测的名字**。
3. **`ViolEnumerationLabelUngrounded` 当前状态**:不在 `summariseExactFix` 任何 case,走 DEFAULT fallback。在 cooccurrence rule C5 / C6 / C6.1 中作 Derived 角色。
4. **Hint 长度无 Render 截断**:`composer.go::Render` 直接 `fmt.Fprintf("**How to fix now**: %s\n\n", h.ExactFix)` — 无 truncation。Phase A 增强可放心扩文本(单 case 200-400 字符均可)。AllowedSet/ForbiddenPatterns 各有 cap(default 10/5)。
5. **defaults_test.go 0 个 Prohibition substring 锁**:Commit 6(Prohibitions 删除)无测试更新成本 — 验证过。
6. **跨文档引用零成本**:CLAUDE.md / MEMORY.md / 其他 docs 不引用 Phase A 规则名 — 删规则不需 cross-doc sync。
7. **Workflow 重新编号风险**:`internal/context/builder.go::formatNumberedList(sk.Workflow)` 把 Workflow slice 渲染成 1-based numbered list。删规则后剩余规则编号 1-13 重排 — Hint / cooccurrence 文本中**没有** `rule #N` 引用,所以重新编号无 dangling reference 风险。
8. **真 eval 命令**:本仓库的 eval test 命名约定可能是 `TestE2E_<case>_x4` 而非 `Test<Case>_x4` — 实施前 grep `t.Run("` 确认。
9. **OutputFormat §-标题验证完成**:§Tool choice / §Block contract / §Block-kind payloads / §Claim annotations / §Diagram contract / §Citation pool 全部存在(见 §2.1 表精确 line)。
10. **`buildAllowedSet` 覆盖度**:仅 6 个 ViolKind 有 case(其余 36 个 ViolKind 返回空 Allowed list)。Commit 1 加 `ViolEnumerationLabelUngrounded` 时同时加 `buildAllowedSet` case 返回 evidence pool anchor。
