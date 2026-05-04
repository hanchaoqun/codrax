# Post-shape 全量日志深挖 — 隐藏深层风险 + 泛化方案 (2026-05-04)

**Source**:`eval/results/{s1a-20260504-024900,m1a-20260504-024904}/run-{1,2}.logs/*.log` (4 真 verification runs against `f8b8fd7` post-R1.1/R1.2/R2.1/R2.2/R2.3/R2.4/R3.1)

**Scope**:全量分析每条 WARN / ERROR + 关键状态转换 + LLM emit vs validator gap,排查除 R1.5 以外的隐藏类问题。

---

## 1. 隐藏问题清单(除已登记的 R1-R5)

| ID | 类问题 | 真触发数据 | 严重性 |
|---|---|---|---|
| **R6** | LLM retry 路径丢失先前正确字段(F7-A 提前显现) | m1a r1 iter=0 emit 正确填 `claim_use{claim_form:definition_fact}` × 4 sections;iter=1 retry 加 `facet_ids` 但**全部 section 的 claim_use 字段被 LLM 删除** → 触发 `principal_claim_use_missing` × 3 | P1 |
| **R7** | LLM 不知该把哪个 facet 放到哪个 block(LLM 选择了错的 facet 集) | m1a r1 LLM 给所有 section 都填了 `facet_ids:["component_relation"]`,**完全没填 `current_code_path` (HARD)** 即使答案明显描述了代码路径 | P1 |
| **R8** | analyzer 退化到 generic + 跨 component (R1.2 子主题矛盾) | m1a r1 第 1 次 emit_analysis 触发 `quality gate HARD failure: subtopic_coherence: R1.2 predicate_contradiction: IsCrossComponent=true but only 0 sub-topic emitted` → 强制重试,~50s 浪费 | P2 |
| **R9** | analyzer pre-scan budget 溢出反复 reject (`prescan tool.*rejected`)| m1a r1 共 11 次 prescan tool reject — LLM 在 budget 用尽 / terminal-emit-mode 之后**仍持续调用 grep**,prompt 未实质阻止 | P2 |
| **R10** | scenario 自动 reconcile 默默改答案家族(无 telemetry) | m1a r2:`scenario reconciled: architecture_explain → generic` — 无 [reconcile-shadow] entry,family 默默换 → 后续 facet 模板不同 → 答案结构不一致 | P2 |
| **R11** | extractor `completeness=complete DOWNGRADED to lower_bound` 黑盒降级 | s1a r1:`3 items < baseline 9` → 静默 lower_bound;LLM 看不到 baseline 数字怎么来,也没 telemetry 进入 closure | P2 |
| **R12** | CGEC `chains_demoted=18` 但 closure ledger 全是 SOFT,不出 violations | m1a r1:`chains_demoted=18` 但 violations 里没有 `chain_demoted`(因为只 self-ref subset 入账)— **极高频率事件无主链可观测**(只 INFO 总数,无明细) | P3 |
| **R13** | `pre_complete_downgrade=0` + `repairs_raised=0` 但 explorer iter=26 — LLM 没短路提前退出 | m1a r1 explorer 跑 26 iter 但 `forced_reads=5 / pre_complete_downgrades=0` → 投入产出比低 | P3 |
| **R14** | `optional_facets_covered=0/1` (m1a r1) 与 LLM 选填的 facet_ids 不一致 | m1a r1 LLM 填了 `nearest_mechanism` (但 facet 模板 Optional 是 `uncertainty_boundary`)— LLM 编造了 facet 模板没列的 ID | P2 |
| **R15** | iter=0 finalizer emit 即刻被 reject 但 retry 收益不明显 | m1a r1 finalizer 4 iter,每次都同结构;最终还是 fail-loud yield kill。**retry 是 reactive,不学习** | P2 |

---

## 2. 类问题归纳(高度泛化)

把上面 10 个隐藏问题归到 4 个**类问题**:

### 类 A:LLM 在 retry 路径上"诚实但失忆"(R6 + R15)

**根因**:retry hint 告诉 LLM "fix X",LLM 重新生成全量 V2 doc。LLM 把"fix X"理解为"重写整体并修 X",顺手把上一版正确字段(`claim_use`)忘了 copy 回来。

**真发现数据**:m1a r1 iter=0 ✓ → iter=1 ✗ → iter=2 ✗(每次 retry 都掉 1-2 个原本正确的字段)

**类问题**:任何 V2 retry 路径都有此风险。涉及 8 处:`emit_answer_document` / `emit_change_plan` / `emit_evidence` / `emit_answer_symbol` / `emit_hypothesis_verdict` / `emit_test_results` / `emit_analysis` / `emit_log_triage`。

**泛化方案 R6**:
1. **短期(0 风险)**:retry hint 强化 "preserve every other field byte-identical from your previous emit; only change the named field(s)"。当前 retry hint 已说"preserve",但 LLM 不听 — 加 verbatim 上一版 JSON 摘要(`Your prev emit had claim_use on every section block; KEEP THEM, do not omit on retry`)
2. **中期**:R4.2 / F7-A retained-draft tool — 让 LLM 只发 patch
3. **真正根因**:retry-prompt 没有 self-diff 引导;V2 schema 的强制字段集 LLM 必须每次都重发,容易掉

### 类 B:LLM 不知道字段值与 block 的"正确归属"(R1.5 + R7 + R14)

**根因**:typed FacetCoverageContract 在 prompt 中以"列出 facet 名"形式呈现,但**没说哪个 block 应该 cover 哪个 facet**。LLM 任意分配 → 漏覆盖 HARD facet OR 编造不存在的 facet ID。

**真发现数据**:
- m1a r1:`current_code_path` HARD facet 无任何 block 声明,但答案明显覆盖
- m1a r1:LLM 编造 `nearest_mechanism` 放进 facet_ids(facet 模板 Optional 是 `uncertainty_boundary`)
- s1a r1:`enumeration_item` 9 次 false-fire(R1.5 已记)

**类问题**:任何 facet/claim_form/surface_role 等 typed-set 字段,LLM 看到"必须 cover"但**没看到"应该放在哪个 block"或"应该用哪个值"**,选错或漏选率 > 50%。

**泛化方案 R7(扩展 R1.5)**:
1. `renderAnswerDocBlockContract` 在每个 BlockRequirement 行上,**显式声明该 block 应该 cover 的 facet_ids 集合**(从 BlockRequirement.FacetIDs 字段读 — 该字段已存在但**没渲染到 prompt**)
2. `renderAnswerDocFacetCoverage` 在每个 facet 行后,**指出哪个 block kind 通常 cover 这个 facet**(从 family 模板的 BlockRequirement.FacetIDs 反查)
3. 验证侧加白名单:LLM 填 `facet_ids` 中包含 facet 模板没声明的 ID → silently drop(不入 covered set)而非通过

### 类 C:analyzer 自动 reconcile / pre-scan budget 黑盒(R8 + R9 + R10 + R11)

**根因**:analyzer 在 quality gate / scenario reconcile / pre-scan budget / completeness downgrade 等多处**自动决策**,**无 user-visible telemetry**:
- `scenario reconciled: architecture_explain → generic` 改 family,无 closure 记录
- `completeness=complete DOWNGRADED to lower_bound` 不入 closure
- pre-scan budget 在 LLM 仍呼叫 grep 时反复 reject 无 retry-hint 回流
- subtopic_coherence quality gate fail → silently retry,~50s 浪费

**类问题**:**任何系统侧自动决策都应该有 closure ledger entry / telemetry signal**,否则下游 oracle 看不见,operators 也看不见。

**泛化方案 R8**:
1. 把 4 个无 telemetry 决策点接入 closure:
   - scenario reconcile → 加 `Mutable.AppendReconcileObservation` 入口
   - completeness downgrade → 加 `Mutable.AppendCompletenessDowngrade` 或新 telemetry kind
   - pre-scan budget reject → 计入 closure stage 计数
   - quality gate retry → telemetry counter + reason chain
2. 让所有自动决策**必须**通过统一 telemetry 入口(类似 `RichnessTelemetrySignal` 的设计)— 防止下次再加新决策又默默工作

### 类 D:CGEC 内部高频事件主链不可观测(R12 + R13)

**根因**:`chains_demoted=18` (m1a r1) 极高频但只入 INFO 总数;`forced_reads=5 / pre_complete_downgrades=0` 与 `explorer_iters=26` 不匹配。CGEC 计数维度太多,没有"高频异常事件 → 主链 violation"的桥梁。

**类问题**:CGEC 计数 vs ViolationLedger 之间的 **dispatch 缺失** — 高频内部事件不上升为可触发 fallback 的 violation。

**泛化方案 R10**:
1. 加阈值化 CGEC → ViolationLedger 桥:`chains_demoted > 10` per dispatch → 自动 emit 一个 ViolDemotionStorm violation(SOFT,告警 only)
2. 加 `forced_reads > N` 阈值告警
3. 让 CGEC 不再是"日志 only"的 event collector,而是分级路径

---

## 3. 优先级排序与执行计划

按 P1 严重性 + 实施复杂度排序:

| 顺序 | ID | 影响面 | 复杂度 | 真发现频率 |
|---|---|---|---|---|
| **#1** | R7 (含 R1.5):typed-set 字段 verbatim 渲染 + 归属指引 | LLM 100% 答错 facet | 中(改 2 helper + 1 schema 描述) | 4/4 runs |
| **#2** | R6:retry 字段失忆 | LLM 50% retry 掉字段 | 中(改 retry hint + verbatim 上一版摘要) | 3/4 runs |
| **#3** | R8:analyzer 自动决策 telemetry 化 | operators 看不见系统决策 | 中(4 决策点各加 1 入口) | 2/4 runs |
| **#4** | R10:CGEC 高频事件桥到 violation | 隐藏 demotion / forced_reads 异常 | 小(2 阈值) | 全部 4/4 runs |

R6 和 R7 互相耦合:R7 修了 LLM 第一次 emit 就更可能正确,retry 路径轻;R6 修了 retry 路径可靠,即使 LLM 第一次有 bug 也能恢复。两者都 P1。

---

## 4. 红线遵守(每条修复必过)

- ✅ 不加 alias / normalize 层(R1.2 红线)
- ✅ 不引入 keyword/heuristic 表(`feedback_no_custom_keyword_matching.md`)
- ✅ 类 D 桥用 typed CGEC 计数器,不用 prose 匹配
- ✅ 类 A 短期方案 prompt-only,不动 schema(F7-A 留作单独大改)
- ✅ R8 的 4 个 telemetry 入口都用 typed 字段,不引入新 prose

---

## 5. 落地时机

R7 + R6 优先 → 重跑同 4 runs verification → 看 `facet_uncovered` 数从 15 → ?, `principal_claim_use_missing` 数从 6 → ?, retry-iter 数是否下降。

R8 + R10 落地后,后续 eval 跑出更精细的故障定位数据。
