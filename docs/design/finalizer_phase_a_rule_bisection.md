# Finalizer Prompt Phase A — Rule Bisection

**Status**: Design (not yet implemented).
**Target session**: single-session ship, 5-8 commits.
**Predecessor**: `docs/design/analyzer_amplifier_layer.md` (analyzer-side amplifier 主线 SHIPPED 2026-05-05; finalizer prompt jitter 仍在 — 本设计是后续治理).
**Successor**: `docs/design/finalizer_phase_c_shape_contract.md` (Phase C — typed EnumerationSubType signal; ship 在 Phase A 完成且真 eval 验证后).
**Owner**: 任意接手者(本文档目标:不熟悉本仓库的开发者读完一遍即可开始改).
**Eval bar**: qf_arch x4 → 4/4 PASS;m1a / s1a / u3a / qf_config_precedence x4 → 不回归.

---

## 0. TL;DR — 一句话设计

`internal/skill/defaults.go` 里 `answer-document-skill` 当前承载 24 条 Workflow + 10 条 Prohibitions + ~155 行 OutputFormat(共 ~80 directive)。其中 **15-18 条规则只是在重复重述 `internal/orchestrator/contract_check_block.go::runV2BlockOraclesWithMut` 已经机器化校验的不变量**。Phase A = **把这些重复规则从 prompt 删除,仅保留判断题型规则(A''/subject discipline/authority discipline/divergence/abstraction-level)**,同时把 hint composer 的 ExactFix 文本扩充 1.5-3× 以覆盖删去的教学信息。

**为什么这能修 A'' jitter**: 删去 15 条机器化规则后,A''(规则 8)在 Workflow 里的注意力份额从 1/24 翻到 ~1/9。validator+hint 双闭环已确保结构性约束不会因为从 prompt 拿掉而失守。

---

## 1. 背景:为什么这个 Phase 存在

### 1.1 现状问题(load-bearing 事实)

`internal/skill/defaults.go::Register{Name: "answer-document-skill"}` (line 115-320) 注册的 finalizer skill 包含:

- **Workflow** = 24 条编号 list,line 119-142
- **OutputFormat** = ~155 行散文 + 表格,line 152-307
- **Prohibitions** = 10 条 bullet,line 308-319

合计 ~80 条 LLM 可见的 directive。`internal/context/builder.go:384-403` 的 builder 直接把这些字段渲染成 numbered list / bullet list / 散文段落,**渲染器是 dumb 的**,任何重组只在 `defaults.go` 内容层操作。

LLM 对长 list 注意力非均匀。2026-05-05 commit `efa4ff3` 在第 8 条插入新规则 A''(role-enumeration 的 abstraction-level matching),实测 qf_arch x4 = 3/4 PASS — 1/4 失败原因是 LLM 在 retry 路径上**没激活 A''**(prompt 里被"30 条规则平均分注意力"稀释)。

### 1.2 关键观察(为何"删 prompt 规则"在 codrax 是安全的)

跨 4 个 Explore agent 验证后,得到 3 个事实:

**事实 1**:`internal/orchestrator/contract_check_block.go::runV2BlockOraclesWithMut` (line 1681) 在每次 finalizer emit 之后**强制运行 13 个 V2 validator**,逐项 hard-check 大部分 Workflow 规则的不变量:

```go
// line 1681-1706
func runV2BlockOraclesWithMut(doc *AnswerDocumentV2, view *AnswerSemanticView, mut *MutableState) []Violation {
    var out []Violation
    out = append(out, validateRequiredBlockCoverage(doc, view)...)        // 规则 #2
    out = append(out, validatePrincipalClaimUse(doc, view)...)            // 规则 #4
    out = append(out, validateDiagramEdgeSupport(doc, view)...)           // 规则 #5
    out = append(out, validateUncertaintyBlockPresence(doc, view)...)     // 隐式
    out = append(out, validateFacetCoverage(doc, view)...)                // 规则 #2 的 facet 维度
    out = append(out, validateRichnessGlaringGap(doc, view)...)
    out = append(out, validatePrincipalProseUnderfilled(doc)...)
    out = append(out, validateRichnessRegression(doc, view)...)
    out = append(out, validateClaimFormSupport(doc, mut)...)              // 规则 #4 + #7 的 claim_form 维度
    out = append(out, validateMissingRequestedRoleDisclosure(doc, view)...)// 规则 #11 (bucket alignment)
    out = append(out, validateAbsenceScopeBound(doc)...)                  // 规则 #24 (absence-citation)
    out = append(out, validateEnumerationItemLabelGrounding(doc, mut)...) // 规则 #7 + #14 (label verbatim)
    out = append(out, validateEnumerationItemLabelExtractorMatch(...)
}
```

外加 `internal/orchestrator/contract_check.go:121-243` 里另外 6 个独立 validator(StructuralEnumerationDivergence、ExternalArtifactDecoded、AuthorityOverreach、SelfConsistency、SemanticQuality、CrossCitationConflict)+ tool 层 `internal/agent/answer_block_normalize.go::failEmit` 的 schema enum 校验。

**所有 validator 都是 PRECISE 信号**(typed enum / index 比对 / substring match in evidence pool),无 score / heuristic — 符合本仓库红线 `feedback_precise_signals_for_hard_gates.md`。

**事实 2**:`internal/analysis/hint/composer.go::summariseExactFix` (line 374-440) 已经针对 25 个 violation kind **逐一硬编码** ExactFix 文本(每条 60-300 字符,直接告诉 LLM "Attach claim annotations on the correct V2 carrier: use block-level `claim_uses=[{claim_form=<one-of-allowed>}]` ..." 这种细节)。Hint 通过 `internal/context/builder.go:419-424` 作为 user section **第一段**渲染给 LLM,LLM 不可能漏看。

**事实 3**:`internal/orchestrator/repair_cooccurrence.go::defaultCooccurrenceRules` (line 83+) 有 9 条 Primary→Derived 规则把多 violation 折叠成单一根因(例:`ViolBlockCoverageMissing` 是 Primary,自动吃掉 `ViolPrincipalClaimUseMissing` / `ViolDiagramEdgeUnsupported` / `ViolUncertaintyBlockMissing` 三个 Derived),避免 retry hint 用一堆衍生 violation 噪音淹没 LLM。

**结论**:删 prompt 重复规则不是裸奔;删后 validator + hint composer + cooccurrence rule 三层联防仍然可保结构正确,LLM 在 first-emit reject 后 1-2 retry 内可完成结构修复 — 这个 retry 成本远小于 attention dilution 导致 A'' jitter 的 cost。

### 1.3 风险(必须前置消除)

来自 Phase A 调研报告的真实评估(并非 Agent 4 给的"3 个高风险"):

- **R1 — Hint 文本细节不足以覆盖删除的教学**:已存在的 25 case 中,有些 hint(如 `ViolBlockCoverageMissing`)只说"调整 blocks[]",未说"如果 X 缺则加 Y"。**Mitigation**:Phase A commit 序列里有专门一步增强 hint 文本(详见 §5.4)。
- **R2 — Cooccurrence 覆盖空洞**:9 条 cooccurrence 规则覆盖最常见组合,但仍有 ~30% violation 组合无 Primary 折叠 → 多 retry 同时面对多 root cause hint。**Mitigation**:Phase A 暂不扩 cooccurrence(留给 Phase B 改进),只挑选**当前 cooccurrence 已覆盖的高频规则**优先删除,其余规则先保留。
- **R3 — 跨 case 回归**:m1a / s1a / u3a / qf_config_precedence 等已 PASS 的 case 可能在删规则后第一轮 emit 失败率上升。**Mitigation**:eval bar 跑 4 case x4 = 16 次,任一回归 → 该次 commit revert,该规则保留。

---

## 2. 代码定位指南(让不熟代码的开发者立即上手)

下面所有文件路径都是相对 `/home/chatpp/codrax`。

### 2.1 Skill 注册(prompt 内容所在地)

| 文件 | 关键 line | 内容 |
|---|---|---|
| `internal/skill/defaults.go:115-320` | 整段 | `answer-document-skill` 注册 |
| `internal/skill/defaults.go:118-143` | 24 条 | Workflow numbered list |
| `internal/skill/defaults.go:152-307` | ~155 行 | OutputFormat 散文(包含表格 / mermaid 示例) |
| `internal/skill/defaults.go:308-319` | 10 条 | Prohibitions bullet list |
| `internal/skill/skill.go:8-53` | struct | `Config{Name, Goal, Workflow, OutputFormat, Prohibitions, ToolSuggestions}` 定义 + `Registry` 单例 |

### 2.2 Skill → LLM prompt 渲染管线

| 文件 | 关键 line | 作用 |
|---|---|---|
| `internal/context/builder.go:384-403` | 整段 | 把 `Workflow / OutputFormat / Prohibitions` 渲染成 system sections |
| `internal/context/builder.go:419-424` | 整段 | RetryHint 作为 user section **第一段**(SectionRetryDirective) |
| `internal/agent/answer_document_evaluator.go::BuildInitialInstruction` (line 127+) | 整段 | 构造 dynamic per-dispatch user section(Required Answer Blocks / Diagram Contract / Facet Coverage / Exact Resolution 等) |

### 2.3 Validator 管线(machine-check 的实质所在地)

| 文件 | 关键 line | 作用 |
|---|---|---|
| `internal/orchestrator/contract_check.go:58-246` | `runContractCheck()` | 顶层入口,在每次 finalizer emit 后运行 |
| `internal/orchestrator/contract_check.go:111-163` | V2 dispatch | 调用 `runV2BlockOraclesWithMut` + 5 个独立 oracle |
| `internal/orchestrator/contract_check_block.go:1681-1706` | `runV2BlockOraclesWithMut` | 13 个 V2 validator 装配 |
| `internal/orchestrator/contract_check_block.go:71-1860` | 各 `validate*` 实现 | 13 个 validator 本体 |
| `internal/agent/answer_block_normalize.go:48-105` | 整段 | tool 层 schema enum 校验(diagram.kind / block.kind / 必填字段) |
| `internal/tool/emit_answer_document_v2.go:107-112` | strict-decode | citation_ref 不能进 claim_use / 未知字段 reject |

### 2.4 Hint composer + cooccurrence

| 文件 | 关键 line | 作用 |
|---|---|---|
| `internal/analysis/hint/composer.go:142-201` | `Compose()` | 顶层入口,接 violations 出 6 字段 Hint struct |
| `internal/analysis/hint/composer.go:202-244` | `Render()` | Hint struct → 字符串 |
| `internal/analysis/hint/composer.go:374-440` | `summariseExactFix` | 25 case 硬编码 ExactFix 文本 |
| `internal/analysis/hint/composer.go:445-540` | `buildAllowedSet` | 给 LLM 列举允许值(claim forms / file list / block kinds) |
| `internal/orchestrator/repair_cooccurrence.go:83-337` | `defaultCooccurrenceRules` | 9 条 Primary→Derived 折叠规则 |

### 2.5 测试影响面

| 文件 | 关键测试 | 影响 |
|---|---|---|
| `internal/skill/defaults_test.go:51-95` | 多个 substring assert 找特定 prompt 文本 | 删规则后这些断言会 fail,需要改 |
| `internal/skill/defaults_test.go:74,108,162,242,267` | `strings.Join(sk.Workflow, "\n")` + `strings.Contains` | 同上,断言对象是合并 blob |
| `internal/skill/keyword_examples_test.go` | LLM-facing jargon 红线 | 不应受影响,但需 grep 确认删的句子不在白名单/黑名单 |
| `internal/agent/answer_document_evaluator_test.go:33-71,2976+` | 多个对 `answer-document-skill.OutputFormat` 内容的断言 | 同样需更新 |
| `internal/orchestrator/contract_check_block_test.go` | 13 个 validator 各自 unit test | **不动** — validator 行为不变 |
| `internal/analysis/hint/composer_test.go` | 25 case 各自 hint 文本断言 | **可能更新**(如果 §5.4 增强 hint 文本) |
| `internal/orchestrator/llm_facing_jargon_audit_test.go` | LLM-facing 术语红线 | 删规则减少 jargon,应该过更绿 |

---

## 3. 24 条 Workflow 规则的逐条分类(本设计的核心决策表)

下表把 `defaults.go:119-142` 每条规则映射到对应 validator + 决策。**决策三档**:
- **DELETE**:规则纯属重复 validator 已 hard-check,删除后 validator 拒+hint 教即可;
- **KEEP-COMPACT**:规则不能删(语义判断或 schema 教学),但可大幅压缩(去重 / 用 `§N` 锚点引用 OutputFormat 表格);
- **KEEP-FULL**:规则保留全文(含 A'' / subject discipline / authority discipline / divergence — 都是 LLM 判断题).

| # | 规则摘要 | 已 machine-check by | 决策 | 删除后 hint 是否充分 |
|---|---|---|---|---|
| 1 | 答案直接写进 emit_answer_document 字段 | tool 层强制(无 emit 即 fail) | **KEEP-COMPACT** (1 句话压缩) | n/a |
| 2 | Required Answer Blocks 必须满足 | `validateRequiredBlockCoverage` (cb:71) | **DELETE** | ✓ `ViolBlockCoverageMissing` hint (composer.go:430) |
| 3 | blocks[] 是 carrier(kind/payload 规约) | tool 层 schema(emit_answer_document_v2.go) + OutputFormat 表 | **KEEP-COMPACT** (引用 OutputFormat §Block-kind payloads) | n/a |
| 4 | claim_uses 在 principal block 必填,4 字段 schema | `validatePrincipalClaimUse` (cb:134) + tool 层 strict-decode | **DELETE** | ✓ `ViolPrincipalClaimUseMissing` hint(composer.go:410,六行细节) |
| 5 | edge_anchors block 级数组 schema | `validateDiagramEdgeSupport` (cb:204) + tool 层 strict-decode | **DELETE** | ✓ `ViolDiagramEdgeUnsupported` hint(composer.go:416) |
| 6 | diagram.kind 是语义 family | `answer_block_normalize.go:52-56` enum | **DELETE** | ✓ 同上 hint 显式说"NOT a Mermaid keyword" |
| 7 | enumeration ordered_list label verbatim | `validateEnumerationItemLabelGrounding` (cb:1746) + `validateEnumerationItemLabelExtractorMatch` (cb:1858) | **DELETE** | ⚠ 需要增强 — 当前 hint 仅 `ViolEnumerationLabelUngrounded` 单句,§5.4 加详细模板 |
| **8** | **A'' — abstraction-level matching for role-enumeration** | **❌ 无** — 纯 LLM 判断题 | **KEEP-FULL** | n/a — Phase A 主战场是让这条规则获得更多注意力 |
| 9 | hop-chain ordered_list 每 item 是一 hop,text 是 behavior | ❌ 无(prose voice 是判断题) | **KEEP-FULL** | n/a |
| 10 | item.kind ∈ {principal/flow/caveat},declared count 只数 principal | `validatePrincipalClaimUse` 内部统计 + `validateRequiredBlockCoverage` | **DELETE** | ✓ `ViolBlockCoverageMissing` hint 已说 count |
| 11 | bucket alignment(用户 label verbatim) | `validateMissingRequestedRoleDisclosure` (cb:1557) | **DELETE** | ✓ `ViolMissingRequestedRoleUndisclosed` hint(composer.go:434,详细) |
| 12 | scalar block 规约(literal 在 block.text) | `answer_block_normalize.go` schema | **KEEP-COMPACT** | n/a — schema 教学 |
| 13 | decision block 规约(verdict + rationale) | `answer_block_normalize.go` schema | **KEEP-COMPACT** | n/a |
| 14 | enumeration item file:line 必须真 anchor | `validateEnumerationItemLabelGrounding` (cb:1746) | **DELETE** | 同 #7 |
| 15 | summary-only 答案规约(### 子标题) | ❌ 无(prose 风格判断题) | **KEEP-FULL** | n/a |
| 16 | citations[] 唯一 declare,citation_ref 是 index | tool 层 schema | **KEEP-COMPACT** (1 句话) | n/a |
| 17 | log-triage 答案 prefer hop-chain | ❌ 无(策略选择题) | **KEEP-FULL** | n/a |
| 18 | 文件形 token 必须在 citations 里(diagram-grounding) | `answer_document_evaluator.go:2770` (diagram-grounding gate) | **DELETE** | ✓ reject signal `diagram_grounding` 含 allowed_labels metadata |
| 19 | Sealed-seed rule(diagram 中 file:line verbatim copy) | `appendRetryDiagramSeedHint` (ade.go:3119) | **DELETE** | ✓ 已有专用 retry hint 段 |
| 20 | log-triage error.Type 必须 verbatim 出现 | `runExternalArtifactDecodedCheck` (cc.go:248) | **DELETE** | ✓ rationale 列举 missing tokens verbatim(cc.go:165-176) |
| 21 | Subject discipline(无关同名 file 不进 summary) | ❌ 无(语义判断) | **KEEP-FULL** | n/a |
| 22 | Authority discipline(drift bound,显式归属) | ⚠ `runAuthorityOverreachCheck` (cc.go:186) 部分 check | **KEEP-FULL** | validator 只 check hedge sentinel,prose attribution 是判断题 |
| 23 | Code-vs-narrative divergence(不静默选边) | `runStructuralEnumerationDivergenceOracleV2` (cc.go:144) 部分 check | **KEEP-FULL** | validator 只 check 候选漏列,narrative 阐述是判断题 |
| 24 | Absence-citation discipline(status='absent' 必带 negative-scope) | `validateAbsenceScopeBound` (cb:1519) | **DELETE** | ⚠ 需增强 hint(当前 generic fallback) |

**汇总**:
- **DELETE = 13 条** (#2 #4 #5 #6 #7 #10 #11 #14 #18 #19 #20 #24 — 12 条 + #16 部分):净删除 12-13 条
- **KEEP-COMPACT = 5 条** (#1 #3 #12 #13 #16):合并为 2-3 条 schema-mechanics 长 rule
- **KEEP-FULL = 7 条** (#8 #9 #15 #17 #21 #22 #23):核心判断题,每条独立保留并加 §-锚点

**Workflow 净规模**: 24 → ~10(KEEP-FULL 7 + KEEP-COMPACT 合并 3)。A'' (#8) 在新 list 中份额从 1/24 → 1/10。

### 3.1 Prohibitions 10 条同样处理

| Prohibition 摘要(line 308-319) | 决策 | 已 machine-check |
|---|---|---|
| 不写 prose 在 emit 外 | **KEEP** (合并到 #1) | tool 层无法 check 但 system 提示 |
| 不 cite 不在 evidence 的 file | **DELETE** | `ViolCitation` |
| 不发明 line 号 | **DELETE** | `ViolGhostAnchor` (composer.go:393) |
| quote 不能放 prose | **DELETE** | grounder 自动 strip(已注释) |
| 不预压缩 prose | **KEEP** (LLM 判断题) | n/a |
| citation_ref 不用 zero-sentinel | **DELETE** | tool 层 schema |
| 不静默截 bounded set | **KEEP** (与 #15 合并) | n/a — completeness 是判断题 |
| 不发明 codename labels | **KEEP** (LLM 判断题) | n/a |
| 不漏 claim_uses | **DELETE** (重复 #4) | `ViolPrincipalClaimUseMissing` |
| 不在 diagram.kind 写 mermaid keyword | **DELETE** (重复 #6) | tool 层 schema |

**汇总**:Prohibitions 10 → ~4 条(删 6 条)。

### 3.2 OutputFormat 内嵌指令处理

`defaults.go:152-307` 的 OutputFormat ~155 行散文,内嵌 ~50 个隐性 directive。**Phase A 不动 OutputFormat 主体**(它的 schema 教学是 KEEP-COMPACT 类),只做以下修改:

- 把 `## Tool choice` / `## Block contract` / `## Block-kind payloads` / `## Claim annotations` / `## Diagram contract` / `## Citation pool` 等 §-标题保留 — 这些是 KEEP-COMPACT 规则的引用目标
- `## Enumeration completeness and bounded sets` (line 227-232) 已被 commit `5388410` 简化
- `## Per-block prose guidance` (line 242-248) **不动** — 是 KEEP-FULL 类(prose voice 判断)
- `## Visual structure` (line 250-289) **不动** — mermaid 教学 schema-level

---

## 4. Hint 增强清单(为 §3 删除规则做的对应补强)

`internal/analysis/hint/composer.go::summariseExactFix` 当前 25 case 中,3 case 在删 prompt 规则后必须增强 hint 文本。**这是 Phase A commit 序列里独立的一步**,不删任何东西,只为后续删除做准备。

### 4.1 增强 case 1:`ViolEnumerationLabelUngrounded`

**当前位置**: composer.go default fallback("Address the violation(s) listed above and re-emit.")
**问题**: 删规则 #7 + #14 后,LLM 看到 reject 不知具体怎么修。
**增强后文本**(草稿):
> "Each ordered_list item's `label` MUST appear verbatim in at least one EvidenceItem (anchor_symbol / subject / object). The validator does case-folded substring match in BOTH directions. Cited evidence pool is in the user section under `## Prior Evidence Slate` — pick label tokens from those entries. If the candidate symbol does not appear in evidence, you cannot include it; either drop the item, or revisit explorer findings."

### 4.2 增强 case 2:`validateAbsenceScopeBound` (Viol kind 待确认)

**当前位置**: composer.go default fallback
**问题**: 删规则 #24 后,LLM 不知 absent 答案要带 negative-scope citation。
**增强后文本**(草稿):
> "When `exact_resolution.status='absent'`, the citations[] array MUST include at least one entry with `scope='negative'` AND a non-empty `negative_pattern` field naming the EXACT search query whose absence-of-matches confirms the finding. Schema: `{file: '<file-or-repo-wide-marker>', scope: 'negative', negative_pattern: '<exact-query-you-ran>'}`. The `file` field can be a literal repo path or `(repo-wide grep)` for repo-wide searches; `line` may be 0."

### 4.3 增强 case 3:`ViolMissingRequestedRoleUndisclosed`

**当前位置**: composer.go:434(已存在,但可加 bucket alignment 维度)
**问题**: 删规则 #11 后,bucket label verbatim 教学需要补到 hint。
**增强后文本**(草稿):
> "Populate document-level `missing_requested_roles[]` exactly from the semantic-view contract for this dispatch. Each entry is `{role:<default|config|runtime|override>, label?:<user-facing bucket name>}`; **for bucket-style questions ('X for A, Y for B'), every bucket label from QuestionStructure.Buckets MUST appear VERBATIM in the rendered answer's user-facing fields** — preferred form: each bucket as a `### <Label>` section heading inside summary OR its own section block."

### 4.4 测试更新

`internal/analysis/hint/composer_test.go` 需为这 3 case 添加 substring assert 锁住新文本(防止后续误删)。

---

## 5. Commit 序列(单 session 5-8 commits)

每条 commit 必须独立可 revert。Eval 在 commit 5 之后整批跑(每 case x4)。

### Commit 1 — Hint enrichment 1(增强 3 case ExactFix 文本)

- 改 `internal/analysis/hint/composer.go:374-440` 加 3 个新 case 或扩展现有 case:
  - `ViolEnumerationLabelUngrounded` (新 case,在 default 之前)
  - `validateAbsenceScopeBound` 对应 ViolKind(grep `ViolAbsence*` 找)
  - `ViolMissingRequestedRoleUndisclosed` 扩展(加 bucket alignment 段)
- 改 `internal/analysis/hint/composer_test.go` 新增 3 substring assert
- 不改 prompt — 这是纯增强
- **Eval**: 全 case 任意 1 个 PASS 即可(只是确认 hint composer 没崩)

### Commit 2 — Workflow rule DELETE 第一批(低风险:tool 层 schema 类)

删 `defaults.go:119-142` 中以下规则(对应 #4 部分 / #5 部分 / #6 / #16 部分 / #19):

- 规则 #4 第二段 "It does NOT carry citation_ref ..." 删(strict-decode 自动 reject)
- 规则 #5 第二段 "Edge anchors NEVER live inside a claim_use object" 删
- 规则 #6 全删(diagram.kind 语义 family — schema enum 已 enforce)
- 规则 #16 后半 "citation_ref NEVER appears inside a claim_use object" 删
- 规则 #19 全删(sealed-seed,有专用 retry hint)

更新 `defaults_test.go` 删去对应 substring assert。

**Eval**: m1a x4 + qf_arch x4 = 8 次。任一 case 在 retry 内不收敛 → revert,该规则保留。

### Commit 3 — Workflow rule DELETE 第二批(中风险:validator 已 cover 类)

删以下规则(对应 #2 / #11 / #18 / #20 / #24):

- 规则 #2(Required Answer Blocks)— validator + hint 已完整 cover
- 规则 #11(bucket alignment)— validator + commit 1 增强 hint 已 cover
- 规则 #18(diagram-grounding)— reject signal 已带 allowed_labels
- 规则 #20(log-triage error.Type verbatim)— rationale 已列 missing tokens
- 规则 #24(absence-citation)— validator + commit 1 增强 hint 已 cover

**Eval**: 4 case x4 = 16 次。

### Commit 4 — Workflow rule DELETE 第三批(高风险:enumeration label 类)

删规则 #7 + #14 + #10:

- 规则 #7(enumeration item.text 规约)— 第一句保留(item.text 是短自然 prose 描述 ROLE)— 因为这是 KEEP-FULL 类的判断教学;只删后半 schema 重述部分(claim_use + claim_form 各种 enum 列举)
- 规则 #14(enumeration item file:line anchor)— 全删,validator 已 cover
- 规则 #10(item.kind 计数)— 全删

**Eval**: 4 case x4 = 16 次。这一批最高风险 — 因为 #7 / #14 直接关联 A'' 所在的 enumeration 路径。

### Commit 5 — Prohibitions 同步删除 6 条

删 §3.1 决策表中标 DELETE 的 6 条 Prohibitions。

**Eval**: 同 commit 4(4 case x4)。

### Commit 6 — Workflow rule §-锚点重命名 + 主题分组 header

把 KEEP-FULL 7 条 + KEEP-COMPACT 合并 3 条 = ~10 条 Workflow rule 加 `§B.2` / `§C.1` / `§G.1` 这种锚点前缀,方便后续 hint 引用。在 list 头部加 1-2 行说明:

```
Workflow rules below are organized into thematic groups:
  §A: Schema mechanics (what fields go where)
  §B: Enumeration semantics (what items mean)
  §C: Hop-chain semantics
  §D: Scalar / Decision shapes
  §E: Summary-only shape
  §F: Log-triage / Diagram grounding
  §G: Universal honesty (subject / authority / divergence)
```

**Eval**: 4 case x4 = 16 次。本 commit 不应引入语义改动,纯重命名 + 顺序调整。

### Commit 7 — 真 eval 验收 + memory 收尾

- 跑 qf_arch x4 + m1a x4 + s1a x4 + u3a x4 + qf_config_precedence x4 = 20 次
- 写 `feedback_eval_pass_is_not_green.md` 已锁红线下的真 eval verdict 表
- 更新 `MEMORY.md` 顶部为本 Phase A SHIPPED + 指向 Phase C
- 更新本文档 Status: `SHIPPED <date>`

### (可选) Commit 8 — 如 §6 中任何 case 出现 jitter,补打 patch

---

## 6. 真 eval 跑法(精确命令)

```bash
# 所有 case 的 eval 入口在 evals/ 下,跑法:
go test ./evals/<case_name>/ -run Test<CaseName>_x4 -count=1 -v -timeout 30m

# 4 + qf_config_precedence,5 case x4 = 20 次,~30-45 min(LLM API 限速)
go test ./evals/qf_arch/         -run TestQfArch_x4         -count=1 -v -timeout 30m
go test ./evals/m1a/             -run TestM1a_x4            -count=1 -v -timeout 30m
go test ./evals/s1a/             -run TestS1a_x4            -count=1 -v -timeout 30m
go test ./evals/u3a/             -run TestU3a_x4            -count=1 -v -timeout 30m
go test ./evals/qf_config_precedence/ -run TestQfConfigPrecedence_x4 -count=1 -v -timeout 30m
```

**注**:实际 case 路径 + 测试名以 `internal/orchestrator/` / `evals/` 下的实际文件为准 — 提交前 grep `t.Run("<case_name>"` 确认。Eval 命令模板见 `MEMORY.md` 中"Real eval workflow"段(若不存在则 Phase A commit 7 顺手新增)。

**PASS 判定**:每 case 4 次 run,使用本 case spec 自带的 regex/keyword 验证。Phase A bar:
- qf_arch x4 = 4/4(从 3/4)
- m1a x4 = 4/4
- s1a / u3a / qf_config_precedence x4 = 不低于当前 baseline

**FAIL 处理**:严格按 `feedback_no_eval_bar_relaxation.md` — 任何 case 回归则该 commit revert。**禁止**调整 case spec regex 让 PASS。

---

## 7. 红线 checklist(开发者必读)

实施 Phase A 前,必须确认理解以下红线(每条都已在 `MEMORY.md` 中):

- 🔴 `feedback_precise_signals_for_hard_gates.md` — Phase A 删的所有规则都对应 PRECISE validator,符合红线
- 🔴 `feedback_redundant_inline_directive_removal.md` — 删 LLM-facing 指令 3 步走:(1)确认对应 validator/hint 已 cover (2)delete (3)eval 验证
- 🔴 `feedback_no_eval_bar_relaxation.md` — 跑出 FAIL 不放宽 case spec
- 🔴 `feedback_no_dismiss_as_llm_flake.md` — 任何 jitter 必须深查根因,不许"LLM flake"草草收场
- 🔴 `feedback_root_cause_only.md` — 删规则若引发新 jitter,根因可能是 hint 不够细 → 加 hint(commit 1 已留扩展余地),不许通过加 prompt 规则绕回去
- 🔴 `feedback_prompt_redline_checklist.md` — KEEP-FULL 7 条规则若改文本必过 ATOMIC 7 条 R3+R4+R5+R6+R7+SST+R2' checklist
- 🔴 `feedback_no_internal_info_in_llm_prompts.md` — 增强的 hint 文本不能露 system 内部术语(grep "validator" / "Viol" / "evidence pool" 这些只在系统侧用的词)

---

## 8. 失败回退路径

如果 commit 4(高风险 enumeration label 删除批)在 eval 中导致 m1a / qf_arch 回归且 commit 1 增强的 hint 仍不够:

- **Plan A**: 回退 commit 4,接受 Phase A 净 DELETE = 7-8 条(commit 2 + 3 + 5 已删的)。A'' 注意力份额从 1/24 → ~1/14,仍是有效改善。
- **Plan B**: 不回退,但补打一个 commit 进一步增强 `ViolEnumerationLabelUngrounded` hint 文本(加例子 + 反例)。再跑 eval。
- **Plan C** (最差):回退 commit 4 + commit 5,Phase A 收口为 commit 1-3,准备 Phase B(扩 cooccurrence 规则覆盖)再回头删剩余规则。

---

## 9. Phase A 不做什么(scope discipline)

- ❌ 不动 amplifier(`internal/analysis/amplifier/`)— 那是 analyzer 侧
- ❌ 不动 AnswerSemanticView 结构 — 那是 Phase C
- ❌ 不引入 sub-family enum(role-enum / mechanism-enum)— Phase C
- ❌ 不动 OutputFormat ~155 行散文主体 — 它是 KEEP-COMPACT 类
- ❌ 不动 hint composer 25 case 之外的新 ViolKind — 现有 ViolKind 集合足够
- ❌ 不动 retry budget / cooccurrence rule 数量 — Phase B 工作

---

## 10. 验收 checklist(本 session 收口前必跑)

- [ ] commit 序列 1-7 全部 push 到 origin/main(未推则 `feedback_confirm_before_push.md`)
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

| # | 规则一句话摘要 | Validator file:line | Hint composer case |
|---|---|---|---|
| 1 | Write directly into emit_answer_document | tool layer | n/a |
| 2 | Required Answer Blocks mandatory | contract_check_block.go:71 `validateRequiredBlockCoverage` | composer.go:430 `ViolBlockCoverageMissing` |
| 3 | blocks[] is the carrier | tool layer schema | n/a |
| 4 | claim_uses required on principal | contract_check_block.go:134 `validatePrincipalClaimUse` | composer.go:410 `ViolPrincipalClaimUseMissing` |
| 5 | edge_anchors block-level array | contract_check_block.go:204 `validateDiagramEdgeSupport` | composer.go:416 `ViolDiagramEdgeUnsupported` |
| 6 | diagram.kind ∈ semantic family enum | answer_block_normalize.go:52-56 | composer.go:416 (same as #5) |
| 7 | enumeration item label verbatim | contract_check_block.go:1746,1858 `validateEnumerationItemLabelGrounding/ExtractorMatch` | (default) — 增强见 §4.1 |
| 8 | **A'' abstraction-level matching** | **none — judgment** | n/a — KEEP-FULL |
| 9 | hop-chain item describes behavior | none — judgment | n/a — KEEP-FULL |
| 10 | item.kind enum + count | contract_check_block.go:71 (count gate) | composer.go:430 |
| 11 | bucket label verbatim | contract_check_block.go:1557 `validateMissingRequestedRoleDisclosure` | composer.go:434 — 增强见 §4.3 |
| 12 | scalar block schema | answer_block_normalize.go schema | n/a — schema teaching |
| 13 | decision block schema | answer_block_normalize.go schema | n/a |
| 14 | enumeration item file:line real anchor | contract_check_block.go:1746 | (default) |
| 15 | summary-only structure | none — judgment | n/a — KEEP-FULL |
| 16 | citations[] declared once | tool layer schema | n/a |
| 17 | log-triage prefer hop-chain | none — judgment | n/a — KEEP-FULL |
| 18 | file-shaped tokens in citations | answer_document_evaluator.go:2770 | reject signal `diagram_grounding` |
| 19 | sealed-seed verbatim | answer_document_evaluator.go:3119 `appendRetryDiagramSeedHint` | dedicated retry hint |
| 20 | log-triage error.Type verbatim | contract_check.go:248 `runExternalArtifactDecodedCheck` | rationale lists missing tokens |
| 21 | subject discipline | none — judgment | n/a — KEEP-FULL |
| 22 | authority discipline (drift) | contract_check.go:186 `runAuthorityOverreachCheck` (partial) | n/a — KEEP-FULL |
| 23 | code-vs-narrative divergence | contract_check.go:144 `runStructuralEnumerationDivergenceOracleV2` (partial) | n/a — KEEP-FULL |
| 24 | absence-citation discipline | contract_check_block.go:1519 `validateAbsenceScopeBound` | (default) — 增强见 §4.2 |

`cb` = `contract_check_block.go`, `cc` = `contract_check.go`.

---

## 附录 B — 决策表 cross-reference

`MEMORY.md` 索引中本设计的位置:`docs/design/finalizer_phase_a_rule_bisection.md`。Phase A 完成 ship 后,在 `MEMORY.md` 顶部增加:

```
- 🟢 [**finalizer Phase A — rule bisection SHIPPED (<date>, N commits)**](project_session_finalizer_phase_a_shipped.md) — 删 12-13 条 machine-checkable Workflow rule + 6 条重复 Prohibition + 增强 3 case hint。A'' 注意力份额从 1/24 → 1/10。qf_arch x4 = 4/4 PASS。下一阶段:Phase C SHAPE_CONTRACT typed sub-family enum。
```

并加一条 cross-session red line(如果新发现):

```
- 🔴 [**删 prompt 规则前必须查 hint composer 是否有对应 case**](feedback_prompt_delete_requires_hint_case.md) — Phase A SHIPPED 经验
```
