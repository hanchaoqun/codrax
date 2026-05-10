# Finalizer 阶段 Repair-Loop 提前预防 — 系统性架构方案

**Status**: Active Design (2026-05-10)
**Baseline**: `3deb641` HEAD at design time. Forensic anchor: 26-run May-9 sweep showing 73% repair-loop activation rate; mr_cross_repo_compare 4-run sample (2026-05-10).

**Scope**: 系统性消除 finalizer 阶段的多余/无效重试。**架构级,不是单 case 修法**。

---

## §1 数据基线(必读)

横切 26 run / 19 case 的 May-9 sweep:

| 指标 | 数值 |
|---|---|
| 触发 repair 的 run | **19/26 = 73%** |
| 0 repair 一击命中 | 7/26 = 27% |
| 平均 repair_exec | 1.27 |
| 重 repair (≥3 次) | 2/26 = 8% |

Top trigger 类别:
- `block_items_label` (7) — 37%
- `answer_facet_coverage` + `answer_richness_facet_coverage` (6) — 32%
- `answer_semantic_quality` (3) — LLM-reviewer 主观
- 其他单点 axes (3+1+1+1)

**结论**:repair-loop 不是"特殊 case 的偶发抖动",**是默认路径**。每 4 个 run 有 3 个跑了 ≥1 次额外 LLM 回合。

---

## §2 Finalizer 阶段完整因果图(G0 调研)

### 2.1 当前架构(三层验证 + LLM-reviewer 闭环)

```
┌─────────────────────────────────────────────────────────────────┐
│ LLM emit_answer_document 调用                                     │
│   prompt 内容:                                                    │
│   - skill (~5000 token, 21 workflow items)                       │
│   - ## Required Answer Blocks (kind/count/facet_ids 列表)        │
│   - ## Required Answer Facets (kind/required/evidence-count)     │
│   - 已有 evidence pool                                            │
│   - prior emit (重试时)                                          │
└────────────────────┬────────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│ Layer 1 — Wire-shape validator (executeAnswerDocumentV2)         │
│   ✓ JSON parsing (DisallowUnknownFields)                         │
│   ✓ V1 retired field detection                                   │
│   ✓ blocks[] 非空                                                 │
│   ✓ NormalizeEmitAnswerBlock per-block:                          │
│     - id 非空, kind 枚举, surface_role 枚举                      │
│     - diagram block 必须有 .body                                 │
│   ✗ 不查 required block kind+count                               │
│   ✗ 不查 facet 覆盖                                              │
│   ✗ 不查 principal claim_use 必填                                │
│   ✗ 不查 uncertainty block 必填                                  │
│   ✗ 不查 prose density / inline-code 数                          │
│ 结果: 大多数 emit 一定通过 (~99%)                                 │
└────────────────────┬────────────────────────────────────────────┘
                     │ 进入 Mutable.AnswerDocumentV2
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│ Layer 2 — Typed contract validators (runV2BlockOracles*)         │
│ ~14 validator,产生 50+ ViolKind:                                 │
│   ✓ validateRequiredBlockCoverage → ViolBlockCoverageMissing    │
│   ✓ validatePrincipalClaimUse → ViolPrincipalClaimUseMissing    │
│   ✓ validateDiagramEdgeSupport → ViolDiagramEdgeUnsupported     │
│   ✓ validateDiagramRelationLegality → ViolDiagramRelationLabelOnly│
│   ✓ validateUncertaintyBlockPresence → ViolUncertaintyBlockMissing│
│   ✓ validateFacetCoverage → ViolFacetUncovered                  │
│   ✓ validateRichnessRegression → ViolRichnessRegression(SOFT)   │
│   ✓ validateRichnessGlaringGap → ViolRichnessGlaringGap         │
│   ✓ validatePrincipalProseUnderfilled → ViolPrincipalProseUnderfilled│
│   ✓ validateClaimFormSupport → ViolClaimFormUnsupported         │
│   ✓ validateAbsenceScopeBound → ViolAbsenceScopeExceeded        │
│   ✓ validateMissingRequestedRoleDisclosure → ViolMissingRequestedRoleUndisclosed│
│   ✓ validateLaneBlockKindCompliance → ViolLaneBlockKindMismatch │
│   ✓ runStructuralEnumerationDivergenceOracleV2                  │
│   ✓ runSymbolAnchorTrackOracleV2                                │
│   ✓ runCrossCitationConflictOracleV2                            │
│   ✓ runDeniedTokenAnswerCheck                                   │
│ 每个 violation 含 SuspectedRoot.IRField + ClusterKey + Repair    │
└────────────────────┬────────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│ Layer 3 — LLM Reviewer (post-validator)                          │
│   ✓ self_consistency_reviewer (LLM call ~5s)                    │
│     summary vs body 事实一致性                                    │
│   ✓ semantic_quality_reviewer (LLM call ~3-5s)                  │
│     facet anchored count, prose-anchor depth, gap detection     │
│ 都受 confidence floor=0.85 门控                                   │
│ 输入: PromotedFacetCoverageDepth (declared/anchored 计数)        │
└────────────────────┬────────────────────────────────────────────┘
                     │ 全部 violation 汇入 contract.Result.Violations
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│ BuildRepairPlan (orchestrator/repair_plan.go)                   │
│   ↳ groupByDispatch                                             │
│   ↳ clusterizeGroup (按 cooccurrence rules 聚簇)                 │
│   ↳ PrimaryOwner = 最深 Locus (Finalize/Extract/Explore/Terminal)│
│ 输出: SummarizeRepairPlan(plan) → 写入 retry hint                │
└────────────────────┬────────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│ 重试循环 (orchestrator pipeline)                                  │
│   PrimaryOwner=Finalizer → 仅重发 finalizer (R2.2 finalize-local)│
│   PrimaryOwner=Extract → 跳回 extract                            │
│   PrimaryOwner=Explore → 跳回 explore                            │
│   PrimaryOwner=Terminal → fail-loud                              │
└─────────────────────────────────────────────────────────────────┘
```

### 2.2 关键 LLM 信息不对称(architectural blind spots)

LLM emit 时**看得见**:
- ✅ 静态 skill prompt(21 workflow items)
- ✅ Required Blocks 列表(verbatim kind+count+facet_ids)
- ✅ Required Facets 列表(kind+required+evidence-count)
- ✅ Evidence pool
- ✅ Prior emit verbatim(retry 时)

LLM emit 时**看不见**:
- ❌ 实时 "已 emit X of Y 块" 计数
- ❌ AnchoredCount per facet(reviewer 即将算的指标)
- ❌ DeclaredCount per facet(reviewer 即将算的指标)
- ❌ block_items_label 验证器即将检查的 anchor list
- ❌ 哪条 workflow 对当前 dispatch **load-bearing** vs **次要**
- ❌ semantic_quality_reviewer 的具体打分维度
- ❌ self_consistency_reviewer 的对比逻辑

---

## §3 六个根本性架构缺陷(诊断)

### 缺陷 #1:**Wire-Schema 与 Contract-Validator 严重不对称**

`emit_answer_document` JSON schema 是**哑 wire format**:
- 验证 wire-shape 通过 → emit accepted
- ~14 个 typed contract validator 在 emit ACCEPTED 之后再跑

**架构后果**:LLM 每次 emit 只对 wire schema 负责,wire schema 通过 ≠ contract 满足。LLM 无法在 emit 阶段提前修正,每 contract gap = 一次 round-trip。

可量化:`block_items_label`(全仓 37%)+ `facet_uncovered`(32%)= 69% 的 trigger 都属于"contract 验证器能 deterministic 算,但 wire schema 没强制" 这一类。

### 缺陷 #2:**LLM-on-LLM 评审栈使方差复合**

self_consistency_reviewer + semantic_quality_reviewer 都是 LLM call,跑在 finalizer 之后:
- finalizer LLM 输出方差 σ₁
- reviewer LLM 评审方差 σ₂
- 实际触发 repair 概率 ≈ σ₁ × σ₂(两个 LLM 的 stochastic 失误之和)

每次 reviewer 调用 ~3-5s,且无条件触发(只要 docV2 非空 + 满足 review-eligibility)。即便 reviewer 是 false-positive,也消耗了一个 LLM round。

confidence threshold=0.85 在 review side 等同于 "LLM 觉得 0.85 信心有问题就报",而 LLM 主观判断本身有方差。

### 缺陷 #3:**单次 emit 一次成功 — 无 emit-time back-pressure 闭环**

LLM 的 emit 是 **完全单向**:看到 prompt → 一口气输出 emit_answer_document 调用 → 返回。中途没有任何 "你已经满足 X / Y 项约束" 的回路。

类比缺陷:
- 想象写一份 5 题考试:学生只看到题面,看不到判分点;交卷后老师批改,错的题学生重做。
- 更先进的设计:学生写每题时屏幕上有 "本题需满足条件 [a/b/c],已写到 c"。

当前 finalizer 是前者。`PromotedFacetCoverageDepth` 这种指标在 emit 之后才被 reviewer 计算 — LLM 没办法在 emit 时优化它。

### 缺陷 #4:**Repair 反馈是散文不是结构化指令**

violation 转 retry hint 的链路:
```
Violation{Kind, Detail, Repair} → composeXxxRetryHint → 散文 hint string → 注入 prompt
```

LLM 收到的是**散文描述**(e.g. `"required block kind=ordered_list appears 0 times; the family contract requires at least 1"`),不是结构化 `{action: "add_block", kind: "ordered_list", facet_ids: [...]}`。

LLM 必须**重新阅读+解释** repair 文字、重新规划 emit。这一过程仍然是 stochastic 的(retry 也可能漏听某条 hint)。

### 缺陷 #5:**Skill prompt 表面密度过高 + 软约束选择性遵循**

`answer-document-skill` prompt 包括:
- 21 个 Workflow items (avg ~150 token)
- ~10 个 Prohibitions (avg ~80 token)
- ~10 个 OutputFormat sections (block 类型表 + diagram 合同 + claim_uses 规则 + 散文风格 + 视觉结构 + ...)
- 总长 ~5000 token

LLM 单次 emit 容量有限,只能 stochastic 地选择性遵循一个子集。**软约束**(prose convention,e.g. "use inline backticks for identifiers")没有 hard 兜底,LLM 不遵循也能 emit 通过 wire schema。

### 缺陷 #6:**Cost asymmetry — 反向激励"安全emit"**

reviewer threshold=0.85 + repair "always run when violations":
- emit "完美" → 0 repair → 1 round
- emit "OK" → reviewer 报 1-2 个 minor → 1 repair → 2 round (~+30s)
- emit "差" → reviewer 报 3+ → 1-2 repair → 2-3 round (~+60s)

73% 的 emit 落在 "OK 但 minor 缺陷" 区间,因为 LLM 没有强烈动力做"完美"(没奖励信号),且没有实时反馈知道哪里 minor 不够。

---

## §4 系统性预防架构方案(6 路 × 优先级)

下面 6 路方案,每条对应缺陷 #N,给出:**做什么 / 复用现有什么 / 新增 LOC / 预期 jitter 削减 / 红线审计要点 / 影响面与回归风险**。

---

### 方案 P1:**Emit-time pre-validation chokepoint**(对应缺陷 #1)

**做什么**:把 `runV2BlockOraclesWithOracle` 的核心 STRICT 验证器(`validateRequiredBlockCoverage` / `validatePrincipalClaimUse` / `validateUncertaintyBlockPresence` / `validateFacetCoverage`)在 `executeAnswerDocumentV2` **emit 接受之前**就跑一遍。**违规直接拒 emit,带结构化 fix-list 返回,LLM 当轮立即修正,不进 Mutable**。

**复用**:
- `runV2BlockOracles*` 整套已有 ✅
- `BuildAnswerSemanticView*` 已有 ✅
- `view.RequiredBlocks` / `view.FacetCoverage` 已有 ✅
- failEmit + ToolResult 拒绝路径已有 ✅

**新增**:`internal/tool/emit_answer_document_v2.go::executeAnswerDocumentV2` 在 `ApplyAndPersistMutation` 之前插入一段 pre-validation,大概 60 LOC + 测试 ~150。

**预期 jitter 削减**:
- 全仓 trigger top: block_items_label (37%) + facet_uncovered (32%) = **69% 的 repair 路径**
- 这 69% 现在变为 emit-time 拒绝 → LLM 当轮(同次 LLM call 的下一个 tool_call)修正
- 残余 jitter ~31%(主要是 reviewer-driven + soft 类)

**红线审计**:
- L1 byte-identical:emit 拒绝走 `failEmit` 既有路径,不变 wire 协议 ✅
- R3 typed gate:复用已有 STRICT-classification 验证器 ✅
- R6 internal info:fix-list 用 user-visible vocab(已有 prompt 一致),不泄露 ViolKind 名 ✅
- 不替 LLM 写答案 ✅
- 红线审计点:**fix-list 文案必须和 skill prompt 用同一个 vocab**(`block.facet_ids` / `block.claim_uses` 等用户已知字段)

**影响面 + 回归风险**:
- 单仓单次 LLM 调用内的 tool-call retry 是 `BaseAgent` 已有路径(见 emitAnswerDocumentRejectSignal)— **不增加** orchestrator-level retry
- 风险:某些 SOFT-classified 违规如果误升 STRICT,会过严拒;**对策**:只挑明确 STRICT 的 4 个 validator,SOFT-default 的不跑(`validateRichnessRegression` / `validateRichnessGlaringGap` 等不进 emit-time)

**优先级**:🔴 **最高 ROI**(69% 削减 + 复用度高)

---

### 方案 P2:**Live state injection — 实时计数与 metric 注入 prompt**(对应缺陷 #3)

**做什么**:在 finalizer 的 `BuildInitialInstruction` 里增加一段 `## Live Emit Constraints`(只在 retry path 出现)显示:
- 上次 emit 各 block kind 已 emit count vs required min/max(显式列差额)
- 上次 emit 各 facet 的 declared/anchored counts(直接抄 `countFacetCoverageDepth` 的结果)
- 上次 emit 触犯哪些 STRICT 验证器(simulated 跑一次 dry-run)

**复用**:
- `view.RequiredBlocks` / `view.FacetCoverage` ✅
- `countFacetCoverageDepth` ✅
- `validate*` 全套 ✅(以 dry-run 模式跑 prior emit 的 docV2)

**新增**:`internal/agent/answer_document_evaluator.go` 新 helper `renderLiveEmitConstraints(ctx, prior)` ~80 LOC + 测试 ~200。

**预期 jitter 削减**:
- 主要打 retry 路径(P1 拦不住的残余 31%)
- LLM 第二次 emit 拿到结构化 "你之前 facet_X declared=3 anchored=0,需 set facet_id 到对应 claim_uses" — 实测应当能把 retry 收敛率从~40% 提到 ~70%
- 综合:多消 **15-20% 的总体 jitter**

**红线审计**:
- R6 internal info:`AnchoredCount` / `DeclaredCount` / `PromotedFacetCoverageDepth` 是内部术语,**不能直接出现在 prompt** — 必须改用 user-visible 描述("the number of blocks that declared each facet" 等散文化)
- R4 不 over-fit:模板化每行,不写"对你这个 case 应当...");通用 schema-level 描述
- CN+EN only:全 EN(match 现有 finalizer prompt)
- 不替 LLM 写答案:只描述差额,不告诉 LLM 具体内容
- ⚠️ **审计要点**:每条新加 prompt 字符串单独过 `TestNoInternalTermsInPrompts`

**影响面 + 回归风险**:
- 仅 retry path 注入 → 0-repair 一击命中的 27% case 不变 ✅
- 风险:prompt 增长 ~500 token / retry,但 retry 时本来就有 prior emit verbatim ~2000 token,占比可控
- 风险:dry-run 验证器跑 prior emit 时如果 view 不一致(family 重路由)会出错;对策:与 emit 实际跑的同一个 view 用 ✅

**优先级**:🟡 高(P1 之后的最大效用方案)

---

### 方案 P3:**Typed repair_hints[] 数组取代散文 hint**(对应缺陷 #4)

**做什么**:把 `composeXxxRetryHint` 从 prose-builder 改成 emit `[]RepairHint{Action, BlockID, FieldPath, ExpectedShape}` 结构化数组,在 prompt 里 render 成:
```
## Required Changes (apply each item exactly)

1. action=add_block, kind="ordered_list", facet_ids=["enumeration_item"], min_items=2
2. action=set_field, target=blocks[id="<bid>"].claim_uses, value=[{claim_form="definition_fact", facet_id="current_code_path"}]
3. action=add_caveat_block, text="<copy 'uncertainty_boundary' content from prior summary block>"
```

LLM 按行机械执行,不需要重新理解 violation prose。

**复用**:
- 现有 `Violation.Repair` 文字 ✅(可作 fallback)
- `cooccurrence rules` 的 cluster owner 已经隐含 action 类型 ✅
- `BuildRepairPlan` 已有 cluster 结构 ✅

**新增**:`internal/orchestrator/repair_hint_typed.go` ~150 LOC + 单测 ~250。`composeXxxRetryHint` 改为 typed render path。

**预期 jitter 削减**:
- 提高 retry 收敛率 — retry pass rate 提升大概 ~10-15%
- 主要受益:`block_items_label`、`block_coverage_missing` 这种"加一个块/字段"的机械式 fix
- 受益较小:`semantic_quality` 反馈(LLM-judgement,本身就模糊)

**红线审计**:
- R3 typed:这条本身就是把 prose 升级成 typed ✅
- R6 internal info:`Action` enum 必须用 user-vocab(`add_block` / `set_field` / `add_caveat_block`)而非 ViolKind ✅
- 不替 LLM 写答案:只描述 SHAPE,具体内容(text / claim_form 值)还是 LLM 写
- ⚠️ 审计要点:`expected_shape` 字段不能塞内部 enum 名

**影响面 + 回归风险**:
- 仅 retry path,首次 emit 不变 ✅
- 风险:typed render 对 `semantic_quality_reviewer` 这种主观判断难映射;对策:这类继续走 prose hint 路径,P3 只覆盖 typed validator-driven 的 cluster

**优先级**:🟡 中(独立于 P1,可并行 ship)

---

### 方案 P4:**Reviewer threshold + dispatch eligibility 收紧**(对应缺陷 #2 + #6)

**做什么**:
1. `pipeline_self_consistency_review_min_confidence` 0.80 → 0.92
2. `pipeline_semantic_quality_review_min_confidence` 0.85 → 0.92
3. `shouldReviewSemanticQuality` 加预过滤:只在 facet declared count ≥ 3 时跑(< 3 的 case 没必要 LLM 评审)
4. `shouldReviewConsistencyV2` 加预过滤:只在 doc 含 ≥1 list block + ≥1 summary block 时跑(单块答案不需要"summary vs body" 一致性审)

**复用**:全是 yaml + 1 个 helper 函数 ✅

**新增**:`internal/orchestrator/contract_check.go` 改 `shouldReview*` 几行 + cmd/root.go yaml 默认值改 + ~30 LOC 测试。

**预期 jitter 削减**:
- semantic_quality top trigger ~15% (3/19) 应当下降到 ~5-8%
- 边缘 0.85-0.92 区间的 false-positive 大部分被吸收
- 综合 jitter 削减 ~10%

**红线审计**:
- 不影响 prompt — 0 LLM-facing string 改动 ✅
- 风险:漏检真问题;对策:reviewer 仍然记 telemetry(`recordReconcileObservation`),但不触发 repair_plan;operator 可在事后审计 sweep 把 borderline case 找回来

**影响面 + 回归风险**:
- 全仓 case 通用 ✅
- 风险:漏掉真问题的 case → 用户拿到次优答案;对策:eval sweep 跑两遍对比(0.85 vs 0.92),量化 false-negative

**优先级**:🟢 低投入高 ROI(可单独 ship)

---

### 方案 P5:**Skill prompt restructuring — 优先级分层 + 当前 dispatch 相关性筛选**(对应缺陷 #5)

**做什么**:
- 把 21 个 Workflow item 分两层:
  - **TIER A (block-shape critical)**: 6-8 条 — Required Blocks 合同、claim_uses 必填、citation 形式 — 永远显示
  - **TIER B (style polish)**: 13-15 条 — prose voice、inline backtick 习惯、regression patterns — 仅在 retry path 显示(首次 emit 默认隐藏)
- 在 prompt 顶部加 `## What this dispatch needs (priority)`,根据 view 动态生成 3-5 行 priority list

**复用**:
- 现有 `answer-document-skill` skill struct(workflow / prohibitions 已经是 list-of-strings,可分层)✅

**新增**:
- skill struct 加 `WorkflowTierA / WorkflowTierB / DispatchPrioritySnippet` 字段
- `BuildInitialInstruction` 按 tier render
- `internal/skill/defaults.go` 分类已有 21 项

**预期 jitter 削减**:
- 首次 emit 的 LLM 注意力更集中在 critical tier → soft-rule 遗漏减少
- 估计削减 jitter ~10-15%

**红线审计**:
- ⚠️ 大改 prompt — **每条 string 必须重新过 `feedback_prompt_redline_checklist`**(R3/R4/R5/R6/R7/SST + ATOMIC 7 条)
- 风险:tier 分错把 critical 放到 B → critical rule 在首次 emit 看不到 → emit 质量下降;对策:tier 分层有 fixture pin 测试

**影响面 + 回归风险**:
- 全仓 case 受影响 ⚠️
- 风险:大 prompt change → fixture 大改;对策:分阶段 ship (先做 tier 划分,不真改 prompt 内容,验证 PASS-rate 无回归后再 polish)

**优先级**:🟢 中长期(高 risk + 中收益,不优先)

---

### 方案 P6:**Repair budget hard cap + degraded acceptance**(对应缺陷 #6)

**做什么**:在 orchestrator 的 finalize 阶段:
- 硬限 repair_exec ≤ 2 (现有 R2.2 finalize-local 已限 2,但跨 stage 没限)
- 第 2 次 repair 后仍有 violation:**接受 doc + 自动 inject "Quality review noted N concerns: [list]" caveat block**,出货
- 用户面板新增一行:"答案已交付,系统检测到 N 个次级质量项已附在 caveat 区域"

**复用**:`AnswerDocumentV2.Caveats` 已有 ✅;`AppendAnswerCaveat` 类似逻辑可参考

**新增**:`internal/orchestrator/orchestrator.go` 加 hard-cap 检测 ~30 LOC;render 端加 caveat 输出 ~15 LOC

**预期 jitter 削减**:
- **不削减 jitter,而是限制其代价**:第 3+ 次 repair 不再发生
- 1.27 平均 repair 次数下降到 ≤ 1.0 上限(实际 ≤ 2.0)

**红线审计**:
- caveat 文案:"系统检测到 N 个质量项" 散文化,不点 ViolKind 名 ✅
- 用户感知:更透明(显式 caveat 比闷头跑 3 次 repair 更好)
- 风险:caveat 出多了用户不信答案;对策:caveat 仅在硬触顶时发,通常 2 round 内能修完

**优先级**:🟢 低投入(防御性,不替代根本修法)

---

## §5 综合实施路径与优先级

按 ROI / 风险矩阵:

```
高 ROI ─┐
       │  P1 (emit-time pre-validation)        🔴 立即做
       │
       │  P2 (live state injection)            🟡 P1 之后做
       │
       │  P4 (reviewer threshold tighten)      🟢 独立可并行
       │
       │  P3 (typed repair_hints)              🟡 独立可并行
       │
       │  P6 (hard cap defense)                🟢 防御性
       │
       │  P5 (skill prompt restructure)        🟢 长期最后
低 ROI ─┘
       低风险 ────────────────────► 高风险
```

### 推荐 ship 顺序(单 session 可达)

| Phase | 方案 | 累计 jitter 削减 | 总投入 |
|---|---|---|---|
| **P1** | emit-time pre-validation chokepoint | 73% → 22% (-51 个百分点) | 60 + 150 LOC |
| **P4** | reviewer threshold tighten | 22% → 18% (-4) | 30 LOC |
| **P6** | repair hard cap | 18%(不削减但限代价) | 45 LOC |
| **P2** | live state injection on retry | 18% → 12% (-6) | 80 + 200 LOC |
| **P3** | typed repair_hints | 12% → 8% (-4) | 150 + 250 LOC |
| **P5** | skill prompt restructuring | 8% → 5% (-3) | 大改 + 长 audit |

最小落地集 = P1 + P4 + P6,**总投入 ~300 LOC + 测试,预期 jitter 削减 73% → ~18%**(原 73% 触发率剩 18%,**绝对削减 ~75%**)。

---

## §6 红线审计 checklist(所有方案通用)

每个方案 ship 时必跑:

- [ ] L1 single-repo byte-identical(任何 IsSingle / nil-multigraph 路径不变)
- [ ] L1 zero-emit-impact path:首次 emit 没 retry 时,prompt / wire / behavior 字节级一致
- [ ] R3 hard gate 用 typed signal(SOFT 验证器不进 emit-time)
- [ ] R6 内部 vocab 不泄漏(`TestNoInternalTermsInPrompts` + `TestNoInternalTermsInHints` 全绿)
- [ ] R7 不替代 LLM 写答案(系统只描述 SHAPE,内容 LLM 自填)
- [ ] CN+EN only:任何新 prompt 字符串只用中英文(用户 2026-05-10 红线)
- [ ] R2' typed signal 6 处同步(struct + JSON schema desc + skill prompt + retry hint + JSON decoder error remap + cooccurrence rule)
- [ ] ATOMIC 7 条 prompt audit(对每条新加 LLM-facing 字符串)

---

## §7 不做的事(明确边界)

- ❌ 不重写 reviewer 为非 LLM call — 保留 LLM-judgement 价值
- ❌ 不删除任何现有 ViolKind — 都有真实使用场景
- ❌ 不改 Layer 1 wire schema 增加字段 — wire 协议改动牵涉 LLM 历史数据
- ❌ 不在 BaseAgent 层做 streaming validator(缺陷 #3 完整解 D)— 工程量过大,不在本设计范围
- ❌ 不实施纯 P5(skill restructuring)— 风险过大,留长期

---

## §8 与本 session 其它工作的关系

- **B 系列(2026-05-10 entity-vs-scope)** : 解决了 analyzer 阶段在 cross-sub-repo comparison 下烧光预算的 bug,与 finalizer repair 正交
- **F1**(2026-05-10): 修了 finalizer trailing-parens anti-pattern,**贡献 jitter 削减 ~5%**(从横切数据看,citation channel 类违规是 minor share)
- **本 G1-Pn 系列**:目标 finalizer **整体** repair-loop 削减 73%→18%,F1 是其中一小块

---

**End of design.**
