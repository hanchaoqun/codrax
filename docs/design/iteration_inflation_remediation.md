# 迭代膨胀 + 用户面板净化 整改设计

状态：W1 / W2 / W3 待施工
代码基线：`origin/main@a3b5d17`(2026-05-05)
作者：架构审计沿用既有 `current_architecture_gap_remediation.zh-CN.md`,本文聚焦其**未覆盖**的迭代膨胀 + 内部状态泄漏问题
责任范围：本文不重复既有 6 phase audit 的内容(carrier / dynamic prompt / diagram relation / richness 单合同 / dual-emit / helper jargon)。本文只补"在那 6 phase 之外仍然存在的"另一组结构性问题。

---

## 1. 问题来源 — 跨 10+ runs 实测数据全景

| 阶段 | 观察范围 | 典型 | 极端 |
|---|---|---|---|
| analyzer_iters | 3-5 | 3 | — |
| explorer_iters | 9-33 | 17-19 | m1a 33 |
| extractor_iters | 1-10 | 1-3 | m1a 10 |
| finalizer_iters | 2-8 | 2-4 | s1a 8 |
| contract_retries | 1-4 | 2-3 | qfa-mr3 3 |

**Top 4 violation kinds(60% 占比 of 47 ViolKinds):**

| Rank | ViolKind | 频次 | 性质 |
|---|---|---|---|
| 1 | richness_regression | 8 | facet 覆盖不足 |
| 2 | enumeration_label_ungrounded | 7 | items label 未对齐 evidence pool |
| 3 | diagram_edge_unsupported | 7 | 边缺 typed edge_anchor |
| 4 | facet_uncovered | 6 | required facet 未声明 |

---

## 2. 与既有 audit 的边界划分

`current_architecture_gap_remediation.zh-CN.md` 6 phases 涵盖:
- A V1 prompt 残影(已部分由 1cbc607 / 68ebe6f 修复)
- B repair cluster identity 结构化(已部分由 e726a43 修复)
- C diagram relation typed-first
- D richness 统一合同(高层目标)
- E full/patch emit 双入口
- F helper prompt 实现层术语

**本文档独立覆盖的维度(既有 audit 没有展开):**

| 本文档 Wave | 既有 audit 关系 |
|---|---|
| **W1 用户面板净化** | 完全新增。既有 audit 未涉及 stdout 泄漏 |
| **W2 violation Implies 图 + retry budget 跨 scope 统一** | 与 D Phase 4 互补 — Phase 4 讲 richness 单合同(对外口径),W2 讲 violations 之间隐含图(系统闭环) |
| **W3 schema-driven 约束 preview + locus 路由** | 与 既有 audit 全部 phase 互补 — 把 hint 层从"事后 prose"前移到"事前 schema description" |

---

## 3. 5 层架构性源头(本文档聚焦)

### 源头 #1 — Contract 47 个 validator 互不感知

`internal/types/violation_registry.go` 47 ViolKind, 各自独立 check, 无依赖图。

**症状(qfa-mr3 实测):**
- Round 0:3 violations {richness_regression, enumeration_label_ungrounded, diagram_edge_unsupported}
- Round 1:**4 violations**(LLM fix 一个,新引入 authority_overreach + diagram_edge_label_mismatch)
- Round 2:2 violations
- Round 3:Δ=0 yield kill

**LLM 视角:** 每轮收到不同的 retry hint 集,以为修了一个,系统又开了一个新的。事实上 4 violations 中 3 个是同一根因的不同表达。

### 源头 #2 — Cluster identity 已 typed 化,但仍按 (Kind, Fp) 联合 key

e726a43 hardened fingerprint 联合 (block, facet, relation, root, evidence_refs) 做 Fp。但 `clusterIdent` 是 `{Kind, Fp}`:

```go
ident := clusterIdent{Kind: st.PrimaryKind, Fingerprint: st.PrimaryFingerprint}
st.PrimaryResolved = !freshIdent[ident]
```

qfa-mr3 实测:同一个 Fp(`block:d1|root:diagram_edges`)在 Round 0 是 `diagram_edge_unsupported`,Round 1 变 `diagram_edge_label_mismatch`。**Fp 相同,Kind 变** → cluster identity 不同 → 系统认为旧 cluster 已 resolved + 新 cluster 出现 → `freshIntroducesNewClusterIdent=true` → plan 重建,stable=0。**convergence detection 失效。**

### 源头 #3 — Retry budget 三层各自独立

```
mid-loop window hint     (explorer 内,3 次/dispatch)
selective fallback       (orchestrator 跨阶段,1 次/run)
contract retry           (orchestrator 全管道,2-4 次/run)
```

**实测:** m1a explorer iters=33,远超 `pipeline-max-steps=15`。原因 — explorer 各自 budget,加 mid-loop hint 又加,然后被 selective fallback 拉一次,最后被 contract retry 间接拉一次。**没有 (kind, fp) → 累计 attempts 的全局账本。**

### 源头 #4 — Repair Locus 路由不闭合

ViolKindSpec.FallbackLocus 是单值,但有些 violation 跨 owner:`enumeration_label_ungrounded` owner=extract,**target=finalizer_only** —— finalizer 是 synthesizer,**没有 file tool**,根本修不了 extract-class 问题。每次必败浪费一轮 retry。

`internal/types/violation_registry.go` 的 `FallbackLocus` 字段没有"FixableByAgents"白名单。orchestrator 选 retry target 不验证可达性。

### 源头 #5 — 用户面板物理上未与内部 ledger 隔离

`orchestrator.go:5234 prependFailLoudWarning` + `:3679 yield kill emit` 直接把 `closure.TopSuspectedField()` (含 `block_items_label`, `conf=0.85`, `3 event(s)`)拼到 `Mutable.Result()` 的开头 → stdout 第一行就是技术黑话。

**违反两条红线:**
- `feedback_no_internal_info_in_llm_prompts.md`(虽然这是 user output 不是 LLM prompt,但同精神)
- `feedback_no_system_backfill_to_user_panel.md`

`AnswerDocumentV2.Caveats[]` 字段已存在,**但没人用它来 materialize 内部 violation**。

---

## 4. 三波施工计划

### Wave 1 — 用户面板净化(预估 4-6 commit,2 天)

#### W1 目标
- stdout 永远只输出 `answer body + caveats`(用户语言)
- 内部诊断信号(yield kill / suspected IR field / event count)100% 进 logging + closure stats,**0% 进 user output**
- 系统层"答案有缺陷"信号通过 i18n caveat 模板传达,而非内部技术名

#### W1 涉及文件

| 文件 | 改动 |
|---|---|
| `internal/orchestrator/orchestrator.go` | 删 `prependFailLoudWarning` 所有 5 处调用(行 2850, 3627, 3646, 3679, 3778);删函数本体(行 5234-) |
| `internal/orchestrator/user_messages.go` | 重命名 `softYieldKillMessage` → `userFacingRetryExhaustedCaveat` 中性措辞 |
| `internal/types/violation_registry.go` | `ViolKindSpec` 加 `UserCaveatTemplateZH string` + `UserCaveatTemplateEN string` |
| `internal/types/answer_document_v2.go` | `Caveats []string` 已存在,确认 mutation runtime 能 append |
| `internal/orchestrator/repair_caveat_materializer.go` | **新文件**:`MaterializeUnresolvedViolationsAsCaveats(violations []Violation, lang string) []string` |
| `internal/orchestrator/orchestrator_test.go` | 新增 4 个测试:retry exhausted / yield kill / structurally empty / fail loud — 全部断言 stdout 不含 "block_items_label" / "conf=" / "yield kill" 等内部 token |

#### W1 步骤

1. **W1.1**(1 commit)— `ViolKindSpec` 加 caveat 模板字段 + 47 个 ViolKind 注册中文/英文模板。模板规范:
   - 不出现 ViolKind name / IR field / 置信度
   - 用户可读完整句子("答案在某些维度的覆盖度可能不充分")
   - 同一根因(richness_regression / facet_uncovered / answer_semantic_underfilled)合并成单条 caveat
2. **W1.2**(1 commit)— 新建 `repair_caveat_materializer.go`:
   - 输入:[]Violation(retry 用尽后剩余的)
   - 用 ViolKindSpec.UserCaveatTemplate 翻译成自然语言
   - 同一 root cause 去重(用 cluster fp 聚类)
   - 输出 caveat 列表(≤ 3 条)
3. **W1.3**(1 commit)— 把 5 处 `prependFailLoudWarning` 调用全替换为:
   ```go
   caveats := MaterializeUnresolvedViolationsAsCaveats(violations, lang)
   doc.Caveats = append(doc.Caveats, caveats...)
   // out.FinalAnswer 不再 prepend
   ```
   测试要确认:
   - `Mutable.Result()` 不再含 "Pipeline terminated with unresolved violations"
   - `AnswerDocumentV2.Caveats` 末尾出现自然语言 caveat
4. **W1.4**(1 commit)— 删除 `prependFailLoudWarning` 函数本体 + 删除现已无用的 `softYieldKillMessage`(已经被自然语言 caveat 替代,而且当 retry 真正 exhausted 时 caveat 已经传达)
5. **W1.5**(1 commit)— 红线测试:加 5 个 case 的 e2e 测试,确认 stdout 不含任何 ViolKind 字面名 / `conf=` / `yield kill` / `event(s)`

#### W1 验证指标
- 跑 qf_arch x2 + s1a x1 + m1a x1 + u3a x1 后 grep stdout:`grep -E "yield kill|conf=|event\(s\)|block_items_label" eval/results/*/run-*.out` 必须返回 0 行
- 答案末尾 caveats 区出现自然语言中文/英文条目

---

### Wave 2 — Violation 隐含图 + 跨 scope retry budget 统一(预估 6-8 commit,3-4 天)

#### W2 目标
- 把 47 个独立 validator 通过 Implies 图收敛成 ≤ 10 个 root cause family
- LLM 每轮 retry hint 看到 ≤ 1 个 root + 它的 derived(不会有"修一个开两个新的"鬼打墙)
- 同一 (kind, fp) 跨所有 scope 累计 attempts,> N 次直接 materialize 为 caveat 不再 retry

#### W2 涉及文件

| 文件 | 改动 |
|---|---|
| `internal/types/violation_registry.go` | `ViolKindSpec` 加 `Implies []ViolKind` 字段;47 ViolKind 标注 Implies 关系 |
| `internal/orchestrator/violation_root_cause.go` | **新文件**:`ComputeRootCauseClosure(violations []Violation) []Violation` 跑 transitive closure |
| `internal/orchestrator/contract_check_block.go` | reporter 在 emit violation 前先跑 root closure,只 emit root + 把 derived 标 `IsDerived=true` |
| `internal/types/violation.go` | `Violation` 加 `IsDerived bool` 字段 |
| `internal/types/repair_attempt_history.go` | **新文件**:`RepairAttemptHistory map[FpKey]int`,thread-safe accessor |
| `internal/types/mutable_state.go` | `MutableState` 加 `RepairAttempts() *RepairAttemptHistory` 持久化在 Run 内 |
| `internal/orchestrator/orchestrator.go` | `runReadSchedulerLoop` retry decision 前 query `RepairAttempts.Get(fp)` >= MaxAttempts → goto materialize-caveat path |
| `internal/orchestrator/repair_cluster_closure.go` | `clusterIdent` 由 `{Kind, Fp}` 改为 `{Fp}`(只用 fingerprint),避免 Kind 切换创造 phantom new cluster;Kind 列表存进 cluster.Kinds slice |

#### W2 步骤

1. **W2.1**(1 commit)— `Violation` 加 `IsDerived bool` + `RootKind ViolKind`(指回 root)。定义 `IsDerived` 不进入 retry hint
2. **W2.2**(1 commit)— `ViolKindSpec.Implies` 字段 + 47 ViolKind 标注关系(具体 mapping 见下表)
3. **W2.3**(1 commit)— `violation_root_cause.go`:transitive closure 算法
4. **W2.4**(1 commit)— 接入 contract_check_block:emit 前做 closure,同一 fp 内只 emit root,derived 标 IsDerived
5. **W2.5**(1 commit)— `RepairAttemptHistory` 数据结构 + 接入 MutableState
6. **W2.6**(1 commit)— 三处 retry site(mid-loop / selective fallback / contract retry)dispatch 前 increment RepairAttempts.Get(fp);>= MaxAttempts 直接 caveat materialize
7. **W2.7**(1 commit)— `clusterIdent` 改成 `{Fp}` only,引入 cluster.Kinds slice;closure logic 的 stable 计数即使 Kind 切换也持续 +1
8. **W2.8**(1 commit)— 测试:模拟 qfa-mr3 violation set rotation 场景,确认 closure 检测 stable=2 触发 materialize 不再 retry

#### W2 关键 Implies 表(参考)

| Root Kind | Implied Derived Kinds |
|---|---|
| `block_items_label` | `enumeration_label_ungrounded`, `block_coverage_missing` |
| `diagram_edges` | `diagram_edge_unsupported`, `diagram_edge_label_mismatch` |
| `answer_richness_facet_coverage` | `richness_regression`, `richness_glaring_gap`, `facet_uncovered` |
| `answer_semantic_completeness` | `answer_semantic_underfilled`, `principal_prose_underfilled`, `enumeration_evidence_underspecified` |

(具体表在 W2.2 commit 里精确填,需扫 `findings_validator/`,`contract/`,`gate/` 的所有 Violation producer)

#### W2 验证指标
- qfa-mr3 重跑:Round 0 violation 数 ≤ Round 1 ≤ Round 2(单调下降,无 rotation)
- 全 case finalizer_iters 中位数从 4 降到 ≤ 2
- contract_retries 中位数从 2-3 降到 ≤ 1

---

### Wave 3 — Schema-driven 约束 preview + Locus 路由(预估 5-7 commit,3 天)

#### W3 目标
- LLM 在 emit-time 看到所有结构性硬约束(不再事后 punishment)
- RepairLocus 路由验证可达性,extract-class 不路给 finalizer-only

#### W3 涉及文件

| 文件 | 改动 |
|---|---|
| `internal/types/violation_registry.go` | `ViolKindSpec` 加 `SchemaDescriptionFragment string` |
| `internal/tool/answer_document_block_schema.go` | `BuildAnswerDocumentSemanticContractDescription()` 自动 collect 所有 ViolKind 的 SchemaDescriptionFragment 拼到 description |
| `internal/skill/defaults.go` | 把 explorer/finalizer skill prompt 里"必须如何"prose 规则迁出 → 全部下放到对应 emit 工具的 schema description |
| `internal/types/violation_registry.go` | `ViolKindSpec` 加 `FixableByAgents []AgentName` |
| `internal/orchestrator/repair_execution_plan.go` | `selectRetryTarget` 验证 `target ∈ FixableByAgents`;不在则升级 owner 或直接 caveat |

#### W3 步骤

1. **W3.1**(1 commit)— `ViolKindSpec.SchemaDescriptionFragment` 字段 + 关键 ViolKind 注册 fragment
2. **W3.2**(1 commit)— `BuildAnswerDocumentSemanticContractDescription` 自动拼装 fragments,emit_answer_document Description() 带上 schema constraints preview
3. **W3.3**(1 commit)— skill prompt 拆瘦:把 explorer/finalizer 22+ 条结构 prose 规则中能 schema-encode 的全部迁出
4. **W3.4**(1 commit)— `ViolKindSpec.FixableByAgents` 字段 + 47 ViolKind 标注
5. **W3.5**(1 commit)— `selectRetryTarget` 加可达性 check;不可达 → 升级到 upstream agent OR caveat
6. **W3.6**(1 commit)— 测试:模拟 enumeration_label_ungrounded(owner=extract)+ retry budget 限定 finalizer_only → 确认不会路由到 finalizer 浪费一轮

#### W3 验证指标
- explorer_iters 中位数从 17-19 降到 ≤ 12
- skill prompt 总长度 -30%(行数)
- contract retries 平均花费 LLM tokens 减少 ≥ 25%(因为 emit-time 已知约束,首轮通过率高)

---

## 5. Wave 间依赖

```
W1 ────────────────────────────────  独立可做,优先(立即收益)
              ↓
W2.1 W2.2 W2.3 W2.4 ──────────────  Implies 图建立(基础数据结构)
              ↓
W2.5 W2.6 ────────────────────────  RepairAttempts 接入(依赖 W2.1)
              ↓
W2.7 W2.8 ────────────────────────  cluster identity 重构(依赖 W2.4)
              ↓
W3.1 W3.2 ────────────────────────  schema preview(依赖 W2 完成,以共享 ViolKindSpec 扩展)
              ↓
W3.3 W3.4 W3.5 W3.6 ──────────────  prompt 拆瘦 + locus 路由
```

**W1 完全独立**,可立即施工。W2 有内部依赖链。W3 依赖 W2 完成(共享 `ViolKindSpec` 扩展点)。

---

## 6. 评估窗口 + 退出条件

每完成一波后跑 5 case x 2 runs:
- 必跑:qf_arch / s1a / m1a / u3a / qf_config_precedence
- 阶段验证指标(见各 Wave"验证指标"段)

**退出条件:**
- W1 完成后:stdout grep 无内部 token + 用户答案末尾出现自然语言 caveat
- W2 完成后:finalizer_iters 中位数 ≤ 2 AND violation rotation 模式消失(连续 2 轮 violation set 一致即触发收敛)
- W3 完成后:explorer_iters 中位数 ≤ 12 AND prompt 行数 -30%

**单波收益不达标,先回归不进下一波**(防止补丁堆积)

---

## 7. 不在本文档范围(明确边界)

不重复 audit 既有 6 phase,以下不属于本文档:

- ❌ V1 prompt 残影清理 → 既有 audit Phase 1
- ❌ Diagram relation typed authority → 既有 audit Phase 3
- ❌ Richness 单一 contract → 既有 audit Phase 4(本文 W2 与之互补但不重叠)
- ❌ Full / patch emit dual-entry 收敛 → 既有 audit Phase 5
- ❌ Helper prompt 实现层术语 → 既有 audit Phase 6

如果 W1-W3 施工过程中发现需要扩到那些 phase,**优先回到既有 audit 文档执行那部分,不在本文档膨胀**。

---

## 8. 接续 checklist(下个 session 续工时必读)

如本 session 中断,下个 session 接续按以下顺序:

1. `git log origin/main` 确认是否有人推进了相关提交
2. `grep "W1\." docs/design/iteration_inflation_remediation.md` 找未完成 sub-step
3. 跑 `eval/run.sh eval/cases/qf_architecture.case 1` 看 stdout 是否含内部 token(W1 是否真完成)
4. `grep "Implies" internal/types/violation_registry.go` 看 W2.2 是否真注册
5. 按表逐项 check,跳过已完成,从下一个未完成 commit 开始

最后更新:2026-05-05(本 session 创建)
