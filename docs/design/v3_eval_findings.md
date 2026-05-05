# v3 真 eval 上线发现 (live log)

Status: in-flight (2026-05-05)
Branch: `main` @ `17a97be`
Provider: openai (MiniMax-M2.7-highspeed via `192.204.35.116:3000`)

每发现 1 个问题/异常立即追加。**不分批等到全跑完**。

---

## Case 跑批进度

| Case | 起始时间 | 状态 | 总耗时 | PASS gate | 关键发现 |
|------|---------|------|---------|-----------|---------|
| s1a (smoke) | 2026-05-05 01:56 | ✅ 完成 | ~7 min | PASS (实质 fake-green,缺第 9 项) | D1-D5 深层链 |
| qf_cfg | 2026-05-05 02:08 | ✅ 完成 | ~4 min | PASS | 一发就成:explore=9/extract=1/finalize=1,promote=0 |
| m1a | 2026-05-05 02:08 | ✅ 完成 | ~6 min | PASS | explore=16/extract=2/finalize=2,promote=1(B1 触发,与 s1a 同模式)|
| qf_arch | 2026-05-05 02:08 | ✅ 完成 | ~9 min | **FAIL** (model 用"只读分析"代替"分类"); ✅ B2 真触发 | promote=2,**B2 ProseUnderfilled 真 fire** + 4-cluster 3-owner closure 完美工作 |
| u3a | 2026-05-05 02:08 | ✅ 完成 | ~14 min | PASS | **3 个 v3 ViolKind 真触发** + 10-cluster 多 owner;reviewer 反检 PART A/B 真工作(但有判断错误,见 D7)|

**v3 Eval 总结 5/5 case**:**4 PASS + 1 FAIL(model 语义错位)**

新加 typed signal 真触发统计:
- `principal_prose_underfilled` 真触发 **3 次**(qf_arch + u3a × 2)
- `answer_semantic_underfilled` (G5 reviewer) 真触发 **3 次**(u3a)
- `richness_glaring_gap` **0 次**(无 case 满足 family glaring + threshold)
- `diagram_relation_label_only` **0 次**

`repair_exec` 全部 case 都跑了,fingerprint 正确独立 — B1 cluster closure 在 1/4/3/4/10 cluster 多场景下全部正确推进。`fail_loud=0` 全 case。`strict_decode_remap=0` 全 case(B4 SST + claim_uses 修复后无 schema-fight retry)。

---

## 🔴🔴🔴 端到端数据流深层根因分析(用户要求,2026-05-05)

按"查每条数据从哪里产生 → 经哪个 typed/prose 通道 → 到达哪里 → 决策"的顺序分析每个发现的问题。**任何问题(不论 v3 是否引入)都追到源**。

### D5 (重申根因) — analyzer LLM 漏识别 "9 项" enum signal

**端到端数据流**:
```
USER INPUT  : "gate.Run 的 9 项检查是按什么顺序跑的?"
              ↓ (LLM 读 question, 无 deterministic preprocess)
ANALYZER LLM: emit_analysis(...)
              ├─ predicates.is_count_question = false       ❌ 应 true
              ├─ predicates.is_category_enumeration = false ❌ 应 true
              └─ enumeration_boundary = ABSENT              ❌ 应 {declared_count: 9, source_quote: "9 项"}
              ↓ (analyzer post-process: 14 deterministic sub-packages)
RequestModel: { Intent=Explain, Scenario=ArchitectureExplain, EnumerationBoundary=nil }
              ↓ (ResolveQuestionFamily — 7-rule priority 决策)
QuestionFamily: QFArchitecture                              ❌ 应 QFEnumeration / QFCallChain
              ↓ (compile_architecture.go)
AnswerSemanticView: { RequiredBlocks=3 (Summary+Section+Diagram), no 9-item floor } ❌
              ↓ (FacetCoverage: required[ComponentRelation, NearestMechanism], optional[uncertainty_boundary])
              ↓ (markGlaringFacets: ComponentRelation, NearestMechanism)
              ↓ explorer 跑;evidence pool 收 9 个 typed anchor_symbol(含 checkPendingFieldsWellformed)
              ↓ extractor LLM: 看不到 "需要 9 项 floor" 的 typed 信号,走 conditional question 路径,弃 slate
              ↓ finalizer 自由发挥,prose 列 8 + "残项" hand-wave
```

**根因层次**(按可修复性排序):
1. **analyzer LLM 行为不可靠**(MiniMax-M2.7 在中文 "数字+量词" pattern 上识别率不高)
2. **analyzer post-emit 没有 sanity validator**:question 中含 `[0-9]+` 数字 + 量词 不要求 `enumeration_boundary` 必填
3. **ResolveQuestionFamily 7-rule priority 没把 EnumerationBoundary.DeclaredCount 作为高优信号**:Intent=Explain 直接通进 QFArchitecture
4. **QFArchitecture 无 N-item floor 设施**:即使 evidence pool 有 N 个 typed anchor,compile 不构造 ordered_list × N 强约束
5. **markGlaringFacets 用的 family 静态分发表**:EnumerationBoundary=nil 时无机制升 component_relation 到 glaring

### D1+D2 (扩展) — extractor recovery escape hatch + explorer→extractor 数据流断口

**端到端数据流**(s1a 第 5-7 步):
```
EVIDENCE POOL (after explorer iter=11):
  9 × EvidenceItem.AnchorSymbol = [
    "checkCoverage", "checkDAGClosure", "checkBudgetSanity",
    "checkContractComplete", "checkHypothesisCoverage",
    "checkSubtopicCoherence", "checkShapeSubjectCoherence",
    "checkCriterionResolvable", "checkPendingFieldsWellformed"
  ]
  ↓ EvidenceClosure (typed pool, 9 unique anchor_symbol values)
  ↓
EXTRACTOR Stage:
  ↓ skill prompt teaching:
  │   case (a) enumeration question → emit slate
  │   case (b) multi-topic explanation → emit skeleton
  │   case (c) explicit bounded principal set → emit principal slate
  │   ELSE (mechanism / value / boolean) → SKIP emit_answer_symbol
  ↓
EXTRACTOR LLM iter=0:
  emit_answer_symbol(count=2, items=[8 entries])         ❌ 8 not 9, count off
  ↓ tool reject: "declared count=2 does not match items length=8"
  ↓
EXTRACTOR LLM iter=1 RECOVERY:
  "Since this is a conditional question about execution order rather
   than an enumeration, I should skip emit_answer_symbol"            ❌ 用 ELSE 逃生,放弃整个 slate
  ↓
FINALIZER 没收到 slate,自己挑 evidence prose,列 8 + "残项"
```

**5 处数据流断口**:
1. **断口 1**: question "9 项" 这个 numeric signal 没传到 extractor 的 skill prompt(因为 analyzer 没识别)
2. **断口 2**: evidence pool 9 个独立 anchor_symbol 没传到 extractor 作为 "expected count" floor
3. **断口 3**: 即使 LLM 自由 emit slate 含 8 items,tool 只 reject "count vs len 不一致" — 不告诉 LLM "evidence pool has 9 unique anchor_symbol values, you should match"
4. **断口 4**: extractor LLM 在 reject 后选 escape hatch,系统没强制"必须修 count + 补全 items"
5. **断口 5**: extractor 弃 slate 后,finalizer 没有 deterministic floor 约束(无 "answer body must enumerate ≥ N items")

修复方向(typed,非 keyword):
- **拦在断口 2-3**:tool emit_answer_symbol reject 路径加 "evidence pool unique anchor_symbol count = N, your slate items = M; either match or justify gap"
- **拦在断口 4**:skill prompt 第 (a)/(c) 路径触发条件加严 — analyzer EnumerationBoundary 存在时禁 (b)/(else) escape hatch
- **拦在断口 5**:family compile 加 OrderedListMin = max(EnumerationBoundary.DeclaredCount, len(evidence.UniqueAnchorSymbol)) floor

### D6 (新发现) — qf_arch finalizer 文件 5 retries 因 `blocks-as-string` 防御未在 peekDocumentModel 之前

**端到端数据流**:
```
FINALIZER LLM:
  emit_answer_document(params={"blocks": "[{\"id\":\"summary\", ...}]", ...})  ❌ blocks 是 stringified JSON
                                                                              (LLM .stringify 一层多余)
  ↓
emit_answer_document.Execute (dispatcher):
  peekDocumentModel(raw)
    ├─ 用 json.Decoder 浅 decode top-level
    ├─ 看 document_model 字段 (LLM 把 document_model 嵌在 stringified blocks 里 OR 在 top level 但 dispatcher 看不到 raw 中的 nested)
    └─ 实际:document_model 应在 top level,但 LLM 把整个 doc payload 当 stringified blocks 输入
  ↓
peekDocumentModel returns: ok=false (no top-level document_model found)
  ↓
Dispatcher 返回:"document_model is required and must equal 'v2'"  ❌ 错误信息没说"你 blocks 是 stringified, 解开"
  ↓
LLM iter=1..3 重复同样错误模式,各 retry 130 sec
  ↓
LLM iter=4 终于把 document_model 放对位置
```

**v2 内部已有 `repairBlocksAsString` 修复函数**(emit_answer_document_v2.go),但**它在 V2 executor 内部,在 peekDocumentModel 之后才能跑**。LLM 写 stringified blocks 时连 peek 都过不了 → 永远到不了 repair 路径。

**根因层次**:
1. `peekDocumentModel` 实现假设 raw 是良好 JSON object,document_model 在顶层
2. `repairBlocksAsString` 修复路径在 V2 executor 内部,后 peek
3. 错误信息 "document_model is required" 误导:让 LLM 以为字段缺失,而不是"你嵌错位置了"

**修复方向**:`peekDocumentModel` 失败 → 也尝试 `repairBlocksAsString` → 重 peek。错误信息加诊断:"if blocks looks like a stringified JSON, ensure document_model is at top-level not inside blocks string."

### D7 (新发现) — Reviewer LLM 误判 inline-code anchor 类型

**端到端数据流**(u3a):
```
FINALIZER emit AnswerDocumentV2:
  blocks[sec3] = {
    kind: "ordered_list",
    items: [
      { label: "explorerEvaluator.ShouldStop",
        text: "显式终止信号路径...(explorer.go:5989)" },
      { label: "...",
        text: "...(explorer.go:6000)" },
      ...
    ]
  }
                                          ↑ items[].text 含 (file:line) 但没有 `code` backtick-quoted identifier
  ↓
validatePrincipalProseUnderfilled:
  ├─ 块 sec3.kind = ordered_list
  ├─ 聚合 items[].text inline-code count
  ├─ countInlineCodeSegments → 0 (only counts backtick-delimited spans)
  └─ claim_count = 7 ≥ 3 → emit ViolPrincipalProseUnderfilled  ✓ 正确
  ↓
SemanticQualityReviewer (PART A reverse-check):
  reviewer LLM 看到 SystemDetectedGaps 列出 sec3 prose_underfilled
  reviewer LLM 看 BODY 文字,看到 "(explorer.go:5989)" 这种括号形 file:line
  reviewer LLM 错判:"这是 code anchor!" → 标 sec3 false positive  ❌
  ↓
但 reviewer 的判定**不影响 typed validator** — typed gap 已 emit,retry hint 飞给 finalizer
```

**Typed validator 是对的,reviewer 误判,但下游 retry 路径没采纳 reviewer 的 false-positive 判定** — 所以最终行为仍正确。

**根因层次**:
1. **概念混淆**:reviewer LLM 把 `(file:line)` 括号引用当作 "inline code anchor",实际 v3 typed signal 严格定义 = ` ``identifier`` ` Markdown 反引号格式
2. **prompt 缺乏精确定义**:semanticQualityReviewerSystemPrompt 没明确告诉 reviewer "inline code anchor = backtick-quoted identifier; file:line in parens does NOT count"
3. **不影响最终 verdict**:reviewer false positive 不阻止 typed validator 的 retry hint(reviewer 输出独立,typed validator 输出独立),但浪费一次 reviewer LLM 调用

**修复方向**:semanticQualityReviewerSystemPrompt 加明确定义"`inline code anchor` 指 Markdown backtick-quoted identifier(`funcName` / `package.Type`),不是括号引用 (file.go:N)"。

### D8 (新发现) — u3a 7 个 cross_citation_conflict 暴露 extractor citation 选取多 line 同 symbol

**端到端数据流**(u3a):
```
EVIDENCE POOL 收到多 evidence 指向 explorerEvaluator.ShouldStop:
  - explorer.go:5989 (investigationComplete check)
  - explorer.go:6000 (S1 fallback entry)
  - explorer.go:6003 (phase==1 gate)
  - explorer.go:6014 (ERM check)
  - ...
  ↓ extractor 选 citations[] for each item
EXTRACTOR emit_answer_symbol:
  citations[0..6] = [
    {file: explorer.go, line: 5989},
    {file: explorer.go, line: 6000},
    {file: explorer.go, line: 6003},
    ...
  ]
  ↓ items 引用同一 symbol "explorerEvaluator.ShouldStop" 多次,各指不同 line
  ↓
runCrossCitationConflictOracleV2:
  发现 7 对 cite 同 symbol 不同 line(>2-line drift)
  → 7 个 ViolCrossCitationConflict
```

这是 SOFT-by-default kind(extractor 的真实选择困境 — 一个函数 5986-6040 之间逻辑分散在多 line)。

**根因**:
- extractor 选 citation 时,每 item 选一个最贴 anchor 的 line
- 但同 symbol 的多个 line 都"贴"得一样近
- 没有 deterministic rule 告诉 extractor "对同 symbol 选 def line 一次,其他 sites 用 anchor_kind=call/guard/etc 分类引用"

**修复方向**:这是 pre-existing 问题,B6-F1 SOFT 默认 ship — operator 可 promote。修复需要 extractor citation 选择策略有"per-symbol single canonical line + ancillary cites with explicit anchor_kind" 规则。**超出 v3 范围**。

---

## 🟢🟢 qf_arch:v3 B2 PrincipalProseUnderfilled 真触发

**重要 milestone** — qf_arch case 让 v3 新加的 typed signal 真 fire:

```
violation kind=principal_prose_underfilled
detail="principal block id=\"list1\" (kind=ordered_list) carries 6 typed claim_use annotation(s)
        but its prose contains zero inline `code` references —
        the surrounding text was abstracted away from the cited evidence"
repair="reference at least one grounded identifier inline (e.g. `funcName` / `package.Type`)
        on block id=\"list1\" so the prose anchors to the typed evidence the answer cited."
```

**这正是 B2 设计的意图** — 检测 "claim 多但 prose 抽象走样" 模式。retry hint 含具体修复指引:"add inline `code` reference"。

**B1 cluster closure 此 case 也完美工作**:
```
ordered=[finalizer, terminal, extract] remaining=2 budget_downgrade=true
clusters=[
  (owner=terminal     kind=richness_regression       fp=facet:uncertainty_boundary)
  (owner=extract      kind=enumeration_label_ungrounded fp=h:11fc08a0ff31)
  (owner=finalizer    kind=diagram_edge_label_mismatch fp=block:diag1)
  (owner=finalizer    kind=principal_prose_underfilled fp=block:list1)
]
repair_exec_promote=2
```

4 cluster × 3 distinct owners,fingerprint 全独立(`facet:` / `h:` / `block:`)。**这是 v3 设计书面承诺最难证伪的部分,真实运行下完美工作**。

### qf_arch FAIL 原因(非 v3 责任)

eval 期望 EXPECT_MATCHES_REGEX `(classif|分类|hypothes|假设)` 在 analyzer 的 stage 描述里出现。Model 答案中 analyzer 描述用 "只读代码分析" 而非 "分类" — 模型语义理解错位(分析器实际角色是 classify request,不是 read-only code analysis)。

v3 加 inline-code anchor 引导后,LLM 加了 `StageAnalyze` / `enums.go:15` 等 inline code 引用 ✓ 满足 ProseUnderfilled,但 SEMANTIC 关键词("分类")没补 — 这超出 v3 typed validator 能力范围。

**判定**:v3 ProseUnderfilled 是必要不充分条件,case 失败属 model 语义理解偏差,non-actionable for v3 typed gates(per `feedback_no_custom_keyword_matching.md` 红线)。

---

## ✅ 验证 v3 工作正常的关键证据

### 1. B1 cluster closure 完美工作
log 提取:
```
repair_plan: primary=finalizer clusters=4
  kinds=[richness_regression, diagram_edge_unsupported, self_contradiction, self_contradiction]
  target=finalizer_only

repair_exec: current=finalizer ordered=[finalizer,terminal] remaining=1
  closed=false stuck=false stable_max=0 budget_downgrade=true fail_loud=false
  clusters=[
    (owner=terminal kind=richness_regression fp=facet:uncertainty_boundary primary_resolved=false derived_resolved=false stable=0)
    (owner=finalizer kind=diagram_edge_unsupported fp=block:d1 primary_resolved=false derived_resolved=false stable=0)
    (owner=finalizer kind=self_contradiction fp=h:dce536c851f9 primary_resolved=false derived_resolved=false stable=0)
    (owner=finalizer kind=self_contradiction fp=h:66cd4ce46d9e primary_resolved=false derived_resolved=false stable=0)
  ]
```

**判定**:✓ B1 设计完全实现:
- 4 cluster 各有独立 fingerprint (facet:X / block:X / hash 各异),识别 cluster 身份精准
- closure state 全 typed 输出
- ordered=[finalizer,terminal] + budget_downgrade=true 显示 R2.2 finalize-local 优先级生效
- 两个 self_contradiction 因 fingerprint 不同 (`dce536...` vs `66cd4ce...`) 被识别为独立 cluster — pre-v3 kind-set strict-subset 会误判合并

### 2. B2 三层质量契约工作正常
- `semantic_quality_dispatches=1 concerns=0` reviewer 1 次 dispatch,0 additional concerns
  → typed validators 已 catch,reviewer reverse-check 正确判 sufficient=true
- `RichnessGlaringGap` 未 fire(uncertainty_boundary 未在 QFArchitecture glaring 列表中,设计正确)
- `PrincipalProseUnderfilled` 未 fire(answer 含大量 inline-code,设计正确)

### 3. B4 mutation 单一协议生效
- `strict_decode_remap_events=0` — 无 schema-fight retry,SST 例子修正后 LLM 一发就成功

### 4. self_consistency reviewer + 重写正常工作 (pre-v3)
- 第 1 次 reviewer:`emitted 2 contradiction(s) at confidence=0.85`(9项 vs 8项 + 9项检查 vs 第9项汇总)
- repair_plan 路由 → finalizer rewrite
- 第 2 次 reviewer:`emitted 1 contradiction(s)`(LLM 没修干净)
- yield kill:`重试无新进展,以现有结论作答`

---

## 🔴🔴 深层系统问题(s1a 回溯链分析,2026-05-05)

完整回溯证据链解释了"为什么 finalizer 前面没答对":

### 链路全程

| 阶段 | 实际行为 | 应该 |
|------|---------|------|
| 1. Explorer iter=11 | ✅ emit_evidence 14 items,**9 项 check 全有 typed anchor**(含 `checkPendingFieldsWellformed @ gate.go:172`) | — |
| 2. Extractor iter=0 | ❌ emit_answer_symbol `count=2 items=[8 entries]` — 缺第 9 项 + count 与 items 长度不一致 | 9 items |
| 3. Tool 拒绝 | ✅ "declared count=2 does not match items length=8. Either revise the count to match the slate OR add/remove items" | — |
| 4. Extractor iter=1 recovery | ❌ "Since this is a conditional question about execution order rather than an enumeration, I should skip emit_answer_symbol" — **整个 slate 弃了** | 修 count=9 + 加第 9 项 |
| 5. Finalizer 无 slate | 仅靠 evidence 自己挑 → 写 8 项 + "残项 hand-wave 第 9 项" | 9 项完整列出 |
| 6. self_consistency reviewer | ✅ 检测到 9 vs 8 矛盾 × 2 → 触发 rewrite | — |
| 7. Finalizer rewrite × 2 | ❌ 仍写 8 + "残项",model 修不动 | 9 项 |
| 8. yield kill 兜底 | ✅ ship as-is + caveat | — |

### 4 个系统深层问题

#### 🔴 D1 — Extractor 错拒后 recovery 路径太松散
emit_answer_symbol 被 tool reject 后,extractor LLM 走的不是"修 count=N + 补全 items"路径,而是**整体放弃 slate**,改走 "skip emit_answer_symbol" escape hatch。

skill prompt 自身允许这条 escape:
> "do not fabricate an answer-symbol list — for single-topic mechanism / value / boolean questions WITHOUT an explicit bounded principal set, skip emit_answer_symbol"

模型把 "gate.Run 的 9 项检查是按什么顺序跑的?" 解读为"single-topic mechanism question",触发 skip。

**问题**:question 明确含 "9 项" 数字,这是 explicit bounded principal set,**应该走 case (c)**(skill prompt 第 4 条:"the user explicitly declared a bounded principal set")。但模型选 case (a/b) 反义路径。

**根因**:tool reject 反馈没有强制走"修 count + 补 items"的窄路径,allowed 多种 recovery 中包括"放弃整个 slate"。

#### 🔴 D2 — 缺 explorer evidence → extractor slate 强 bridge
Explorer 已经 emit_evidence 出了 9 个 typed anchor,每个含:
```
anchor_symbol="checkPendingFieldsWellformed" line_start=172 evidence_kind=registration
```

这 9 个 typed anchor 在 evidence pool 里 **完整准确**。但 extractor 没办法直接消费 — slate 必须 LLM 重新 emit。

**问题**:explorer 干完活后 evidence pool 有 9 个 typed name,extractor 只要复制粘贴就行,但 LLM 偏偏漏掉一个并把 count 写错。

**根因**:系统**没有 deterministic bridge** 把"evidence pool 含 N 个 anchor_symbol 与 question 的 DeclaredCount 一致"自动 promote 成 extractor slate 的 floor。

**可能 fix**:当 `EnumerationBoundary.DeclaredCount > 0` 且 `len(unique anchor_symbol in evidence) >= DeclaredCount`,extractor 的 emit_answer_symbol 应有 deterministic floor — items 长度必须 ≥ DeclaredCount。Tool reject 带着这个 floor 提示。

#### 🔴 D3 — Family 误分类(QFArchitecture vs QFEnumeration)
log 显示 `family=architecture intent=explain required_blocks=3 optional_blocks=2 has_diagram=false richness_candidates=1`。

但问题是 "**9 项检查**按什么顺序" — 明显是 enumeration 数轴。如果 family=QFEnumeration,compile 路径会要求 ordered_list + 9-item floor。现在 family=QFArchitecture 路径要求 BlockSection × N + diagram,与问题语义不匹配。

**根因**:`ResolveQuestionFamily` 当前规则当 `Intent=Explain + Scenario=ArchitectureExplain` 时进 QFArchitecture,但 question 含 "9 项" 数字应该优先级高于 architecture intent。priority order 没把 EnumerationBoundary.DeclaredCount > 0 考虑成 enumeration signal。

#### 🔴🔴 D5(根因)— Analyzer mis-classification: "9 项" 信号丢失

s1a `emit_analysis` 输出:
```json
{
  "intent": "explain",
  "scenario": "architecture_explain",
  "question_kind": "conditional",
  "predicates": {
    "is_count_question": false,           // ❌ "9 项" 应 true
    "is_category_enumeration": false,    // ❌ 应 true
    ...
  }
  // ❌ 缺 "enumeration_boundary": {"declared_count": 9, "source_quote": "9 项"}
}
```

Question 字面含 **"9 项检查"**(数字 + 量词)— 但 analyzer LLM(MiniMax)输出的所有 enumeration 维度信号全丢:
- `is_count_question=false` 错
- `is_category_enumeration=false` 错
- 完全没 emit `enumeration_boundary` 字段

**这是整条链路的最上游错误**。一旦 analyzer 漏掉,downstream:
- ResolveQuestionFamily → QFArchitecture(本应 QFEnumeration / QFCallChain)
- compile_<family> → required_blocks=3 BlockSection + diagram(本应 BlockOrderedList × 9)
- B2 ViolEnumerationEvidenceUnderspecified: trigger `DeclaredCount > 0` 不满足 → 永不 fire
- finalizer 没有 9-item floor 约束

per `feedback_no_custom_keyword_matching.md` 红线 — 不能加 ZH/EN cue 表 "9 项 → is_count_question=true"。但**抽象泛化的 typed signal 是允许的**:

**修复方向**(可作 v3 follow-up):
- analyzer post-emit sanity validator:question 含 `[0-9]+` digit + 任一 enum-context word(项 / 个 / checks / items / 个数 / steps / methods / ...)→ 当 `enumeration_boundary` 空 OR `is_count_question=false` 时 reject 并 retry。
  - 跨语言通用(数字 + 量词模式),非 eval-fitted
- 或:enumeration_boundary 字段 schema description 加更直接例子(已经有,但 LLM 仍漏):"any question whose verbatim text contains a numeral like '9 X' / '5 things' / 'the 7 checks' MUST emit enumeration_boundary"

**严重度**:🔴🔴 这条修了,D1-D4 链路 50% 不会发生。

#### 🔴 D4 — finalizer rewrite 路径**没收到** "你 body 缺第 9 项" 这种**精准 typed 信号**
self_consistency reviewer 给出的 violation Detail:
```
SUMMARY: "9 项独立检查" ⇄ BODY: "8 项后进入汇总"
```

这是 prose-vs-prose 比对,reviewer 知道"数字不一致"但 **不知道哪一项缺**。retry hint 让 LLM "pick the claim supported by cited evidence and rewrite the OTHER side" — 这等于让模型 **自由选**:要么改 SUMMARY 为 8,要么改 BODY 为 9。模型选错(改 SUMMARY 没用因为后续仍需列出 9 项)。

**根因**:reviewer 看 prose 不看 typed `EnumerationBoundary` + `evidence pool anchor_symbol set`。如果有典型 oracle "DeclaredCount=9 AND items in body match 8 of 9 anchor_symbols" 类的 typed gate,retry hint 能直接说"You're missing `checkPendingFieldsWellformed` per the typed evidence pool"。

---

## 真问题清单

### P1 — Model 在 9-item 枚举上能力不足(非 v3 bug)
**症状**:`MiniMax-M2.7-highspeed` 在 SUMMARY 写"9 项独立检查",在 BODY 只列出 8 项 + 把第 9 项标为"残项"(汇总逻辑非检查)。两次 finalizer rewrite 后仍有 1 处矛盾,最终 yield kill。

**判断**:这是**模型本身的能力上限**问题,不是 v3 引入的。但 v3 的 self_consistency 反复检测 + 重写机制工作正确。

**对策**:
- (a) 升级 provider 到 MiniMax-M2-pro 或更强模型 — 应用层无法解决
- (b) v3 当前已有 yield kill 兜底 + 答案携带 caveat ✓
- (c) eval case 严格化:可在 EXPECT_MATCHES_REGEX 把 floor 从 3 名提到 9 名(per "答案准确" 红线 — 但用户已警告永不放宽 bar,所以反向应升 bar)

### P2 — answer body 中 9 项 enumeration 实质不完整
**症状**:case ground truth 是 `pending_fields_wellformed (gate.go:148)` 作为第 9 项 SOFT check。MiniMax 答案缺第 9 项,改写后仍缺。

**eval verdict**:PASS(EXPECT_MATCHES_REGEX 接受 ≥3 名)— **PASS != 绿** 红线提示这是 fake-green。

**判断**:模型能力问题,v3 检测对了(self_consistency 抓到了),但模型修不动。

### P3 — pre-existing diagram_edge_unsupported(family=architecture 但 LLM 写 kind=flow)
**症状**:`violation[0] kind=diagram_edge_unsupported detail="diagram block id=\"d1\" declared kind=flow but family contract expects architecture"`

**判断**:pre-existing,不是 v3 引入。重写阶段未修。**ProseUnderfilled / RichnessGlaringGap 不会 catch 这种 — 是 BlockCoverage / DiagramEdge 范畴**。

### P4 — Pipeline 终止时报 "yield kill (1 yield kill(s)); top suspected IR field: ScannedSet"
**症状**:Pipeline 结尾追加 caveat `"Pipeline terminated with unresolved violations (yield kill: retry window produced no new information) — 1 yield kill(s); top suspected IR field: ScannedSet (conf=0.70, 3 event(s))"`

**判断**:这是 yield kill 路径,不是 FailLoud。`repair_exec_failloud=0` 也证实。属设计内行为,但说明 retry budget 用完仍未解决问题。

---

## v3 设计目标对照 (s1a 单 case 数据)

| 目标 | 预期 metric | s1a 实际 | 判定 |
|---|---|---|---|
| 轮次减少 | finalizer_iters / total round 中位数下降 | finalizer_iters=2 (低,但有 yield kill 表示 retry 用完);analyzer/explorer/extractor 总 LLM iter ≈ 24 | **大体达成** — 与 m1a r1 pre-v3 28 calls / finalizer 同等量级问题相比改善明显 |
| 答案丰富 | RichnessGlaringGap fire 触发补面 / inline-code count ≥ claim_use 数 | s1a (architecture) 未 fire 任何 v3 新 violation;pre-existing 检测照常 | **未触发** — case 本身 evidence 不足以触发 glaring,无法判别 |
| 维护简单 | 静态 — 不靠 eval 验证 | ✓(B0 完成 6→1)| ✓ |

---

## 异常 / 死循环监控

(无 — s1a 健康完成,无死循环)

---

## 行动 / 修复

(暂无 v3 自身需修复项 — 发现的问题都是 pre-existing 或 model 能力问题)

下一步:
- 并行跑 m1a / qf_architecture / qf_config_precedence / u3a 各 1 run
- 看 v3 新 ViolKind 在哪些 case 真触发
- 看 cluster_closed / stuck / stable_max metric 在多 case 下的统计
