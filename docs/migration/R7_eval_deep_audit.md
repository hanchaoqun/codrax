# R7 真 eval verification 深度审计 — 隐藏深层风险 + 泛化方案 (2026-05-04)

**Source**:`eval/results/{s1a-20260504-032008,m1a-20260504-032012}/run-{1,2}.logs/*.log` (4 真 verification runs against `25d1e4a` post-R7)

**Scope**:R7 ship 后第二轮全量日志 deep-dive,验证 R7 收益、暴露新隐藏风险、归纳类问题、给出泛化方案。

---

## 1. R7 verification 结果(对比 R1-R3 baseline)

| 维度 | R1-R3 baseline | R7 verification | 变化 |
|---|---|---|---|
| Pass rate | 3/4 (75%) | **3/4 (75%)** | 持平 ✓ |
| facet_uncovered | 15 | **4** | **-73%** ✅ R7 主目标达成 |
| principal_claim_use_missing | 6 | **10** | **+67%** ❌ R7 没解决 |
| richness_regression | 2 | 2 | 同 |
| facet_softened (R3.1 lock) | 0 | 0 | ✅ |
| self_consistency fired | 6 | 5 | 同 |
| finalize-local priority | 3 | 1 | -2 |
| **block_coverage_missing** | 0 | **4** | **新维度暴露** |
| **uncertainty_block_missing** | 0 | **1** | **新维度暴露** |
| **diagram_edge_unsupported** | 0 | **1** | **新维度暴露** |

**结论**:**R7 主目标(facet_uncovered)73% 下降,但揭开了之前被 facet_uncovered 噪声压住的 4 个新违规维度**。这是 audit-driven discovery 的正常表现 — 修一个问题就暴露下面被淹没的层。

---

## 2. 真发现的 4 类深层隐藏风险(R7 之外)

### 类问题 E:retry 失忆有 sub-pattern — block-level 优先丢失,item-level 优先保留

**真证据**(m1a r1 finalizer iter=0 vs iter=1 emit 比较):

iter=0 完整 emit(10 个 claim_use 标注):
- s1 summary block-level claim_use ✓
- turn-a-section block-level claim_use ✓
- turn-b-section block-level claim_use ✓
- handoff-section block-level claim_use ✓
- orch-section block-level claim_use ✓
- 5 个 list 项 item-level claim_use ✓

iter=1 retry 后(6 个 claim_use):
- s1 summary block-level claim_use ✓
- 4 sections **block-level claim_use 全丢** ❌
- 5 list 项 item-level claim_use 仍在 ✓

**根因**:LLM 在 retry 路径上有"重写整体并修 X"的 mental model,**把 item-level 视为"内层细节"优先保留;block-level 视为"外层属性"优先简化掉**。retry hint 当前没区分这两层。

**类问题广义**:任何"双层 typed annotation"(block-level + item-level)在 retry 上都有此风险。受影响:
- `block.claim_use` vs `items[i].claim_use` (R6 子模式 a)
- `block.facet_ids` vs `items[i].claim_use.facet_id` (R6 子模式 b,LLM 最终把 facet_ids 留 block 级而 items 级丢)
- `block.surface_role` vs `items[i].surface_role` (待观察)

**泛化方案 R6.1**(扩展 R6):
- retry hint 加显式 layer-preservation 教学:"每个 block 可有 block-level 和 item-level claim_use;两者**独立必填**,缺一不可。retry 时,**保留你之前每个 block 的所有 typed annotation 字段(facet_ids、claim_use、surface_role 等),只改你被指出错误的字段**"
- 长期靠 R4.2 / F7-A retained-draft tool

### 类问题 F:R7 暴露被压抑的"低频高严"违规维度

**真证据**:
- `block_coverage_missing` 4 events — 之前 0,因为 facet_uncovered 噪声 17 个,scheduler retry budget 用完前 block_coverage 信号被忽略
- `uncertainty_block_missing` 1 event — 同样模式
- `diagram_edge_unsupported` 1 event(m1a r1 LLM declared `diagram.kind=flow` but family expected `architecture`)

**根因**:scheduler 的 violation 处理是 **batch fail-loud after retry budget**,不是 "violation-priority-ordered"。一个 facet_uncovered 噪声把所有 retry attempt 烧光,真正高严的 block_coverage 没机会收到 fix-prompt。

**类问题**:**violation processing 中没有 priority/severity 维度** — typed_violations 越多,越可能让真正紧急的 fail-loud kind 被低 budget retry 吞掉。

**泛化方案 R11**:
1. ViolationKind 加 `Severity` 字段(Critical / High / Medium / Soft)
2. scheduler retry 在 budget 用前**优先**处理 Critical+High violations,把 Soft violations 推到下一轮 retry
3. CGEC summary 行加 by_severity 子表

### 类问题 G:explorer dispatch 计数语义混淆 — `explorer_iters=40` 实际是 cross-window 累加

**真证据**:
- m1a r1 metrics: `explorer_iters=40`
- 但 LLM dispatch ASSISTANT lines 总数 40,且 `iter=11` 出现 4 次(不是 1 次)
- 真相:explorer 经过 4 次 dispatch(每次 contract.Check 失败后重新 dispatch),每次 dispatch 内部 iter 从 0 重置

**根因**:**B6-F5 metric 把 cross-dispatch ASSISTANT 计数当成"真 LLM turn 数",但 dispatch 边界丢失** — 一个 LLM call 在 explorer 第 1 次 dispatch 的 iter=11 跟第 4 次 dispatch 的 iter=11 是**完全不同的 LLM 上下文**(messages 历史不同),却被算为相同 metric。

**类问题**:**所有 cross-dispatch metric 在多 dispatch 场景下都有此误读** — `explorer_iters / extractor_iters / finalizer_iters / analyzer_iters` 都是。运维看 `explorer_iters=40` 以为是同一个 LLM 长跑,实际是 4 次重新调度。

**泛化方案 R12**:
1. `[diag X] iter=N ASSISTANT` 加 dispatch 索引前缀:`[diag explorer dispatch=2 iter=11 ASSISTANT]`
2. metric 拆成两维:`explorer_dispatches / explorer_iters_per_dispatch`(median 等)
3. eval/run.sh `summary.md` 加 `dispatches`/`iters_per_dispatch` 双列

### 类问题 H:CitationReq:10 主导 violations 但无 user-actionable repair

**真证据**(m1a r1):
- `violations=20 by_field={CitationReq:10,answer_richness_facet_coverage:1,block_claim_use:8,diagram_kind:1}`
- 10 个 CitationReq 违规 — answer 引用够多 citations(8 entries)但 finalize success_criterion `citation_count_ge` 仍 fail
- 之前 R1-R3 verification baseline 未出现 — **R7 把 facet 噪声移走后 CitationReq 浮出**

**根因**:`citation_count_ge` SuccessCriterion 在 finalize TaskNode 上,与 BlockRequirement-level citation expectation **重复检查**。LLM 看 schema 知道要"add at least 3 citations",但 LLM 在 retry 路径上**重写答案 → 引用范围变窄 → citation 数下降**。

**类问题**:scheduler-level SuccessCriterion 与 V2 oracle-level Violation 之间存在**重复 gating**,LLM 修 V2 oracle violations 时可能违反 scheduler-level criterion(无 cross-check)。

**泛化方案 R13**:
1. 验 `citation_count_ge` SuccessCriterion 与 BlockRequirement-level citation expectation 重叠区域 — 删一处或加 cross-check
2. retry hint 必须把"V2 oracle fix" + "scheduler-level criterion" 两层 violation 一并展示给 LLM

---

## 3. 优先级矩阵

| ID | 类问题 | 严重性 | 真触发频率 | 实施复杂度 |
|---|---|---|---|---|
| **R6.1** (扩展 R6) | block-level vs item-level retry 失忆 | P1 | m1a r1 8 events | 中(retry hint 改) |
| **R11** | violation 严重度优先级 | P2 | 隐藏在 4 runs | 中(加 Severity 字段) |
| **R12** | explorer iter 计数语义模糊 | P3 | 4/4 runs | 小(diag log 改 + summary 改) |
| **R13** | scheduler vs V2 oracle 重复 gating | P2 | m1a r1 10 events | 中(audit 重叠区) |

---

## 4. 红线遵守

- ✅ 不引入 alias/normalize 层
- ✅ 不加 keyword/heuristic
- ✅ R6.1 是 prompt 内增量,不动 schema
- ✅ R11 用 typed Severity enum,不用 prose 匹配
- ✅ R12 / R13 都基于 typed 计数

---

## 5. R7 collateral 收益(意外)

- s1a r1 总时长 6m38s → **6m24s**(略快,facet 重审时间省下)
- m1a r1 explorer 40 iter — 实际是**4 次 dispatch 各 ~10 iter**,被 metric 显示为 40 误导
- **新违规维度暴露的本质是好事**:这些 violations 一直存在但被噪声掩盖,现在能进 closure ledger 被 cross-Run learning 吸收

---

## 6. 跟踪表登记建议

将 R6.1 / R11 / R12 / R13 加入 `post_shape_residual_audit.md` 跟踪表,优先级:
1. **R6.1** P1 — 实施立即可做(prompt-only)
2. **R13** P2 — 需 audit 重叠区
3. **R11** P2 — Severity 字段是大的架构改动,排在 R6.1 / R13 之后
4. **R12** P3 — 观测性改进

R7 单点修复成功 + 暴露 4 个新类问题 → audit-driven discovery 正常迭代。
