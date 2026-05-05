# v3 Eval Fix 泛化审计与最优方案 (revised 2026-05-05)

Status: design (用户红线纠正后)
Source data: `docs/design/v3_eval_findings.md`(8 个深层根因 D1-D8)

## 用户红线纠正

> 输出的内容是千变万化的,靠关键词去打补丁,永远也解不完的 bug。

之前 D5' 推荐 "修 owner gate 让 regex fallback 工作" — **endorse 了关键词词表补丁**(`reChineseCountEnumerationBoundary` 的 `(个|项|条|步|层|类|种|处|段)` 8 词字典 + `(first|top)` 英文词字典)。这本身违反 `feedback_no_custom_keyword_matching.md` 红线:

> "If you see inter-run inconsistency on the same question, trace which field flipped, then read that field's schema description. Ask: would a human with just this description confidently emit the same value both times? If no, the description is the bug. ... 不允许加关键词表绕过"

任何词表对未来 question shape 都是漏的:
| 用户表述 | 词表覆盖? |
|---|---|
| `9 项` | 中文 `项` ✓ |
| `5 stages` | 英文 generic count `[1-9]\d{0,2} \w+` ✓ |
| `3 道关卡` | `道/关` 不在词表 ❌ |
| `5 個` (日文) | ❌ |
| `7 개` (韩文) | ❌ |
| `5 step` (英文 leading) | ❌ (需 first/top 前缀) |
| `4 tiers / 8 commands / 2 modes` | 是否漏取决于 generic match |

补漏永远补不完。

---

## 修订原则

1. **删除所有 keyword fallback 路径**(包括 pre-v3 既有的 `enumeration_boundary.go` regex 链)— 是个永远补不完的 bug 源
2. **trust LLM typed output**:analyzer 漏识别 → 走 honest fallback (QFGeneric)产 caveat,**honest weakness > dishonest confidence**
3. **仅当 schema description 有歧义时改 prompt** — 不靠词表补救

---

## 修订后 fix 表

| Fix | 重新评估 | 决策 |
|---|---|---|
| D5'' | 删除 regex fallback 链(4 regex + detectRequestedEnumerationBoundaryFromRaw + RecoverRequestedEnumerationBoundary)。Trust analyzer LLM。当 LLM 漏识别 → QFGeneric + caveat。 | ✅ 立即 |
| D3 | 不改 — D5'' 修后 analyzer 直接产 EnumerationBoundary,Rule 6 自动 hit | ✅ 不改 |
| D2'' / D1'' | 仍只用 typed signal(`closure.UniqueAnchorSymbolCount` integer + `EnumerationBoundary.DeclaredCount` integer + skill prompt 的 typed-signal-driven 触发)。**前提是 analyzer 真识别了 boundary,否则不激活** — 这正是符合 trust-LLM 原则。 | ✅ 实施 |
| D6 | order-of-operations 修复(peek-then-repair-then-peek),无关键词 | ✅ 立即 |
| D7 | reviewer concept clarity(inline-code 定义),无关键词 | ✅ 立即 |
| D4' | reviewer 加 typed input(EvidencePoolAnchors []string + DeclaredCount int)。仍是 typed signal 注入,无关键词 | ⚠ v3.2 留 |
| D8' | citation 选取策略需要语言/repo 特定规则推理,跨 repo 难 — pre-existing SOFT 状态 ship 即可 | ⚠ 不修 |

---

## 详细方案

### D5'' — 删除 regex fallback,trust analyzer

#### 删除清单(`internal/types/enumeration_boundary.go`)
- `reEnglishLeadingEnumerationBoundary` 常量
- `reEnglishCountEnumerationBoundary` 常量
- `reChineseLeadingEnumerationBoundary` 常量
- `reChineseCountEnumerationBoundary` 常量
- `detectRequestedEnumerationBoundaryFromRaw` 函数
- `RecoverRequestedEnumerationBoundary` 函数

#### 保留清单
- `RequestedEnumerationBoundary` struct(typed)
- `NormalizeRequestedEnumerationBoundary`(只做 typed 校验:验证 LLM 输出的 SourceQuote 真在 RawRequest 中,无 regex,无关键词)
- `EnumerationBoundaryCountString`(纯 integer → string)
- `RequestedEnumerationBoundaryOwner`(entity-based,通过 `MentionedEntitiesFromRawRequest` 验证 entity 在 raw 中,**无 regex 关键词**;只用作 step backbone enrichment 输入)

#### 删除 callsites
- `internal/agent/analyzer.go:1003` 的 `RecoverRequestedEnumerationBoundary` 调用 → 删

#### 测试清理
- `internal/types/enumeration_boundary_test.go` 中针对 `RecoverRequestedEnumerationBoundary` 的测试 → 删

#### 行为变化
- 从此 EnumerationBoundary 唯一来源 = LLM emit_analysis 的 typed 输出
- LLM 漏识别 → ResolveQuestionFamily 走非-Enum 分支 → QFGeneric / QFArchitecture / 等。OrchestratorEnd 如果发现实质内容缺,则 yield kill + caveat
- s1a 类 case 可能 PASS rate 下降(MiniMax 漏识别"9 项")— 但这是真实模型能力反映,不再用 keyword 假装救场

#### 红线复审
- ✅ R3:不引入新 signal,删除已有 noisy regex signal
- ✅ R6:删除 keyword 词表
- ✅ `feedback_no_custom_keyword_matching.md`:符合 "trust schema description, fix prompt ambiguity" 原则
- ✅ `feedback_eval_pass_is_not_green.md` / `feedback_no_eval_bar_relaxation.md`:接受 fail 是真实信号,不放宽

### D6 — peek-then-repair-then-peek

`emit_answer_document.go::Execute` 在 `peekDocumentModel(params)` 失败时,先尝试 `repairBlocksAsString(params)` 再重 peek。失败时错误信息加诊断("If your blocks payload was JSON-stringified, ensure document_model is at the TOP level alongside blocks, not nested inside the stringified payload.")。

无 keyword,纯 order-of-operations + 错误信息。

### D7 — reviewer inline-code 概念定义

`semanticQualityReviewerSystemPrompt` 加段:
```
DEFINITION CLARITY:
  - "inline code anchor" = a Markdown backtick-quoted identifier inside the prose,
    e.g., `funcName` or `package.Type`. Parenthesized file:line citations like
    "(foo.go:42)" do NOT count as inline code anchors — they are bibliographic
    references, not in-prose anchors.
```

无 keyword,只澄清 typed validator 已经定义的概念。

### D2'' / D1'' — typed-signal-driven

#### D2'' emit_answer_symbol reject diagnostic
当 reject 发生时,error 信息加 typed signal 提示(纯 integer 比较):
```
declared count=N does not match items length=M.
[when EnumerationBoundary.DeclaredCount=K and len(unique anchor_symbol)>=K in pool]:
  the user-declared count is K and the evidence pool has >=K typed anchor_symbol values
  matching the question scope; emit a slate of K items mirroring those anchors.
```

无 keyword,纯 typed integer 比较。

#### D1'' skill prompt 严格化
`internal/skill/defaults.go` 的 emit_answer_symbol 第 4 条 ELSE 路径加段(typed signal-driven):
```
When the system has detected an explicit declared_count from the question
(visible in the user-section's Requested Set Boundary), you MUST emit a slate
matching that count — the "skip emit_answer_symbol" fallback is NOT available
in this case. The skip path is reserved for genuinely conditional / mechanism
questions WITHOUT any declared count.
```

无 keyword,触发条件全 typed(`EnumerationBoundary` 字段是否存在,纯 typed)。

---

## 跨 8 family 影响矩阵(修订后)

| Fix | RootCause | ConfigPrec | RoleLookup | CallChain | Enum | Architecture | Comparison | Generic |
|-----|-----------|-----------|-----------|-----------|------|------|------|---------|
| D5'' (删 regex) | LLM 已识 boundary 时不变;漏识时直接走 honest fallback | 同 | 同 | 同 | 同 | 同 | 同 | 同 |
| D6 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| D7 | reviewer 通用 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| D2'' / D1'' | LLM 识别 boundary 时激活 | 同 | 不影响 | LLM 识别时 | ✓ | 不影响 | 不影响 | 不影响 |

---

## 跨语言/repo 矩阵(修订后)

| Fix | 中/英/日/韩/任意语言 | 任意 repo / 任意 question shape |
|-----|---|---|
| D5'' | ✓(LLM 直接读语言原文,不依赖词表)| ✓ |
| D6 | ✓ | ✓ |
| D7 | ✓ | ✓ |
| D2'' / D1'' | ✓(LLM 用任意语言 emit boundary) | ✓ |

**关键改进**:删除 4 个 regex 词表后,日韩 question / 任意未来 counter word / 混合语言 question 都不再受词表覆盖度限制。

---

## v3.1 实施 batch

按 ROI 顺序:
1. **D5''** 删除 regex fallback 链 + callsites + tests
2. **D6** peek-then-repair-then-peek
3. **D7** reviewer concept clarity
4. **D2''/D1''** typed-signal-driven(D5'' 之后才有效)

D4'(reviewer typed input)留 v3.2,D8'(citation policy)不修。

总改动量:~150 LOC(删多于加)+ 3 prompt 段。

---

## 红线总复盘

每条 fix 通过:
- ✅ R3 仅用 runtime typed signal(typed integer / enum / boolean)
- ✅ R6 不含 codrax 内部命名 / eval-case verbatim symbol / 项目特定关键词
- ✅ `feedback_no_custom_keyword_matching.md` 不加任何关键词词表 / 不加任何 keyword-driven fallback
- ✅ `feedback_eval_pass_is_not_green.md` / `feedback_no_eval_bar_relaxation.md` 不放宽 case spec
- ✅ `feedback_root_cause_only.md` 删除 anti-pattern 不留单点补丁
- ✅ `feedback_no_dismiss_as_llm_flake.md` 真 LLM 错误诚实暴露,不掩盖
