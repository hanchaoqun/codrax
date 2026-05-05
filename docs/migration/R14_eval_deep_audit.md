> Status: archived (2026-05-05). Current architecture lives in docs/architecture.md and docs/design/v3_runtime_consolidation.md.

# R14 c10 真 eval verification 深度审计 (2026-05-04)

**Source**:`eval/results/{s1a-20260504-041906,m1a-20260504-041910}/run-{1,2}.logs/*.log` 4 真 verification runs against `31655d1` post-R14。

**对比基线**:R7 verification(commit 25d1e4a 之后)— 同样 4 runs。

---

## 1. R14 vs R7 verification 总账

| 维度 | R7 verification | R14 verification | Δ |
|---|---|---|---|
| **Pass rate** | 3/4 (75%) | **4/4 (100%)** | **+1 ✅** |
| Hard Rule renders (R14 真触发) | 0 (R14 未上线) | **4** (s1a r1×2 + m1a r2×2) | **R14 真活了** |
| facet_uncovered | 4 | **1** | **-75% ✅** |
| block_coverage_missing | 4 | **1** | **-75% ✅** |
| principal_claim_use_missing | 10 | **15** | **+50% ❌** |
| diagram_edge_unsupported | 1 | 2 | +1 |
| richness_regression | 2 | 2 | 持平 |
| facet_softened (R3.1 lock) | 0 | 0 | ✅ 守住 |
| self_consistency fired | 5 | 7 | +2 |
| finalize-local priority | 1 | 1 | 持平 |

### R14 主目标(retry 失忆 cure):**部分达成,深层 insight 暴露**

- s1a r1:Hard Rule 渲染 2 次 → 答案 PASS,facet_uncovered=0(R7 时是 0,持平),principal_claim_use_missing=2
- s1a r2:Hard Rule 0(无 retry 触发)
- m1a r1:Hard Rule 0(无 retry 触发,首次 emit 已 PASS)
- m1a r2:**Hard Rule 渲染 2 次,但 principal_claim_use_missing=9!**

---

## 2. R14 真深层 insight(从 m1a r2 抽出)

### Insight 2.1:Severity 和 strict-list **解耦** — R14 没改 strict gate
**真证据**:m1a r2 iter timeline:
- 3 blocks (`turn_a_section / data_bridge_section / turn_b_section`)在 retry 1 / 2 / 3 都缺 claim_use
- 每次 retry 后 R14 Hard Rule 重新渲染,LLM **仍同样漏掉这 3 个 block 的 block-level claim_use**
- 答案最终 **PASS**(因为 `principal_claim_use_missing` 在 `defaultSoftKinds` 中是 SOFT-by-default)

**深层根因**:R14 的 ScoredViolation.Severity 是 **prompt-rendering 用的层**,但 `hasAnyStrictViolation` 仍按 legacy `pipeline_contract_strict_kinds` yaml + `defaultSoftKinds` 决定 retry 触发。**Severity=Critical 不自动等于 strict**。

**类问题**:R14 引入新 Severity 维度,但 retry-trigger gate(`hasAnyStrictViolation`)还在读旧的 strict-list — **retry 决策机制和 Severity 分类还没贯通**。

### Insight 2.2:LLM 忽略 Hard Rule "byte-identical preserve" 提示
**真证据**:m1a r2 finalizer claim_use 出现次数 across iters:
| iter | claim_use occurrences (含 R14 prev emit 回显) |
|---|---|
| 0 | 13 |
| 1 | **18**(LLM 把 prev emit 回显也当成自己的写) |
| 2 | 14 |
| 3 | 12 |
| 4 | 4 |
| 5 | 9 |
| 6 | 8 |
| 7-10 | **0** |

iter 1 出现 18 个 claim_use 说明 LLM **把 R14 渲染的 "Previous Emit" 字符串当成自己的写出**(re-quoting)。R14 Hard Rule "preserve byte-identical" 提示 **LLM 没真理解** — LLM 的 retry mental model 仍是"重写整体"。

**深层根因**:Prompt-level "preserve" 指令对 generative LLM 的可靠性有限。**真根因仍是 V2 retry 协议层** — 强制全量重发 emit,LLM 无法只发 delta。

### Insight 2.3:R14 retry-state 渲染**正确触发**但**改善有限**
**真证据**:Hard Rule renders × 4(s1a r1 × 2 + m1a r2 × 2)— 渲染机制完全正常,数据装载也正确。但跨这 4 次 retry,principal_claim_use_missing 总数仍 ≥ 11。**R14 的字段缺失阻止能力 < 50%**。

---

## 3. 类问题归纳(共性)

把 3 个 insight 归到 **2 类深层共性问题**:

### 类问题 I:**多维分类不贯通**(R14 Severity / strict-list / fallback locus)
- R14 Severity (Critical/High/Medium/Soft) 是 prompt 渲染用
- `pipeline_contract_strict_kinds` 是 retry-trigger 用
- `FallbackTargetForKind` 是 fallback locus 用
- **三者各自有自己的 ViolationKind 分类规则**,可能矛盾
- 表现:R14 Critical 不一定 strict,LLM 看到 Critical 在 retry-state 但 retry 不一定真触发

**泛化方案 R15**:统一三个分类成一个 typed `ViolationProfile`(代替散在的 strict-list / soft-list / fallback-policy / R14 Severity):
- 一个 ViolationKind → 一个 ViolationProfile{Severity, FallbackTarget, RetryEligible}
- 所有 retry / fallback / rendering 决策都从 ViolationProfile 读

### 类问题 J:**LLM-level prompt 指令对 retry 行为的可靠性有限**(R14 Hard Rule 实际效果)
- "Preserve byte-identical" 提示在 LLM 上**不可靠** — LLM 仍 regenerate
- m1a r2 真证据:Hard Rule 渲染 2 次,3 个 block 仍持续漏 claim_use
- LLM 把 prev emit 回显**当成自己的写**(iter 1 看到 18 个 claim_use 但实际只有 13 是 LLM 写的)

**泛化方案 R16(继承 F7-A retained-draft 设计)**:
- 协议层而非 prompt 层 — 加 `emit_answer_document_patch` tool
- LLM 只发 `{"unchanged_block_ids": ["s1","s2"], "replace_blocks": [...]}` patch
- 系统侧 ApplyPatch 拼出新 doc,验证后入 Mutable
- LLM 没机会"重写"整体,只能 declare delta

### Insight 1.4 不算独立类:R14 的 prompt rendering 机制本身工作完美
Hard Rule × 4 真触发 + RetryState 数据 round-trip 验证(c1+c2+c3+c4 测试)+ pass rate 提升 → R14 不是问题,**R14 暴露了下层的 retry-trigger / prompt-level 指令的天花板**。

---

## 4. 优先级矩阵

| ID | 类问题 | 严重性 | 真触发频率 | 实施复杂度 |
|---|---|---|---|---|
| **R15** | 多维分类贯通 (ViolationProfile 替代 3 个零碎分类) | P1 | m1a r2 真发现 | 中(架构改动,但 R14 已铺路) |
| **R16** | 协议层 retry preservation (emit_answer_document_patch) | P1 | m1a r2 真发现 + R7/baseline 旧观察 | 高(新 tool + ApplyPatch) |

**R15 vs R16 关系**:
- R15 修 retry 触发逻辑(R14 Severity → 真 retry)
- R16 修 retry 后 LLM 行为(LLM 不再"重生成")
- 互补,**先 R15(改动小,提升 retry 覆盖率)→ R16(改动大,提升 retry 准确率)**

---

## 5. R14 收益总结(尽管 retry 字段保留有限)

**正向**:
- ✅ Pass rate 3/4 → **4/4 (100%)**
- ✅ facet_uncovered 4 → 1 (-75%) — R7 + R14 联合作用
- ✅ block_coverage_missing 4 → 1 (-75%)
- ✅ R14 Hard Rule 渲染机制 4 次真触发,数据 round-trip 工作
- ✅ R3.1 facet_softened 0/4 持续守住
- ✅ R7 typed-set verbatim 渲染 + R14 retry-state 联合让简单 case (s1a r1) 通过率提升

**未达成**:
- ❌ retry 失忆深层未根除(m1a r2 LLM 仍丢字段),需要 R15 + R16 后续

R14 不是失败 — 它**搭好了底座 + 暴露了下层缺陷**,Pass rate 实际提升,真 bug 显形 → audit-driven discovery 的标准节奏。

---

## 6. 红线遵守

- ✅ 不打散补丁 — R14 是统一架构,R15/R16 是其后续而非新补丁
- ✅ R15 用 typed ViolationProfile,zero keyword
- ✅ R16 是协议层,LLM-facing 简单(看到的 schema 不变)
- ✅ R3.1 emptySurface 短路 / R7 typed-set verbatim / R14 retry-state contract 全部仍守

---

## 7. 跟踪表登记建议

- ~~R6 / R6.1 / R11 / R13~~ → R14 收口(已标)
- **R14** → SHIPPED(c1-c10 全完;c10 真验证已写)
- **R15** → 新登记 P1(多维分类贯通)
- **R16** → 新登记 P1(协议层 retry preservation,延伸 R4.2 / F7-A 设计)

R12 / R8 / R10 / R1.3 / R1.4 / R4.1 / R4.3 / R4.4 仍 pending,优先级不变。
