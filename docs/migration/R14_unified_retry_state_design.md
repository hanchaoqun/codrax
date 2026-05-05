> Status: archived (2026-05-05). Current architecture lives in docs/architecture.md and docs/design/v3_runtime_consolidation.md.

# R14 — Typed Retry-State Contract (统一方案,取代 R6/R6.1/R11/R13)

**起草**:2026-05-04
**取代**:R6(retry 字段失忆)/ R6.1(block vs item 失忆)/ R11(violation 缺 Severity)/ R13(scheduler vs V2 oracle 重复 gating)的零碎补丁
**红线**:不打散补丁,跨表面找统一根因。

---

## 0. 背景

R7 verification 4 runs 真测后深挖,发现 4 个表面不同的隐藏问题:R6 / R6.1 / R11 / R13。表面看是 4 个独立修复点(retry hint 加 X / 加 Severity 字段 / 加 dispatch 索引 / audit 重叠区),但**深层共性**是同一根:**V2 retry 是 stateless 协议,LLM 看不到 prev typed-state + 跨层 violation 全貌**。

零碎补丁(在每个 corner 加 prompt 教学 + 加 Severity 字段 + 改观测 + 加 cross-check)既增加复杂度,又**不解决根因**:每加一个补丁就改一处 prose,LLM 在新场景下又会以新方式失忆/失序。

---

## 1. 共性深层根因

| 表面问题 | 真根因 |
|---|---|
| R6 retry 字段失忆 | LLM 每次 retry 被当作"新 emit",看不到 prev typed-state,凭猜重写 |
| R6.1 block-level 比 item-level 更易丢 | 同上 — 无 prev typed reference,layer-priority 靠 LLM 凭感觉 |
| R11 violation 缺 Severity 维度 | 同上 + violations 列表无 priority 信息,LLM 不知先修哪个 |
| R13 scheduler vs V2 oracle 重复 gating | 同上 + LLM 不知道还有哪些 gating 层在 fail |

**真根因汇总**:**V2 retry 路径上 LLM 没看到三件事**:
1. **prev emit** typed-state(上一次发了什么字段)
2. **violation 全貌**(跨 scheduler / V2 oracle / contract.Check 三层,按 Severity 分组)
3. **preserve / change 字段分类**(哪些必须 byte-identical,哪些必须改)

---

## 2. 统一方案 — Typed Retry-State Contract

**核心契约**:retry 路径上,evaluator 在 dynamic instruction 中渲染 **typed retry-state view**(替代当前 prose-only retry hint):

```
## Previous Emit (preserve every unchanged field byte-identical)

Block id="s1" kind=summary surface_role=principal:
  facet_ids: ["current_code_path"]
  claim_use: {claim_form: "definition_fact", citation_ref: 0}
  text: "<truncated head 400 + tail 200 chars>"

Block id="lifecycle" kind=section surface_role=principal:
  facet_ids: ["component_relation", "nearest_mechanism"]
  claim_use: {claim_form: "definition_fact", citation_ref: 1}
  ...

Citations[0..7]: file:line pairs (verbatim)

## Active Violations (typed, by layer + severity)

[CRITICAL · finalize · principal_claim_use_missing] block id="lifecycle":
  fix: re-emit with claim_use field
[HIGH · finalize · facet_uncovered] required facet "diagram_spine":
  fix: declare facet_id="diagram_spine" on a block
[SOFT · telemetry · richness_regression] facet "uncertainty_boundary":
  fix: optional, no required action

## Required Changes (by field path)

- blocks[id="lifecycle"].claim_use → set to {claim_form: "definition_fact", citation_ref: 1}
- blocks[id="<some_block>"].facet_ids → append "diagram_spine"

## Hard Rule

Every field NOT named in "Required Changes" above MUST appear byte-identical to your "Previous Emit" section. Do NOT regenerate from scratch — copy the rest verbatim.
```

LLM 看到这一份 retry-state 后:
- ✅ 不会丢 block-level claim_use(它在 Previous Emit 看到自己上一次填了)→ R6 + R6.1 解决
- ✅ 优先修 CRITICAL violations(显式按 severity 排序)→ R11 解决
- ✅ 看到 finalize-only / cross-layer 所有 violations → 不会修一层违反另一层 → R13 解决

---

## 3. 实施路径(单一改动,不是 4 个补丁)

### 3.1 数据结构(`internal/types`)
```go
// types/retry_state.go (新文件)
type RetryState struct {
    PrevEmitJSON     json.RawMessage      // 上一次 LLM emit 完整字段
    PrevEmitSummary  RetryStateSummary    // 摘要 (block ids + 关键字段集合)
    ActiveViolations []ScoredViolation    // 跨层 violations (V2 oracle + scheduler + contract.Check)
}

type RetryStateSummary struct {
    BlockSummaries []BlockSummary  // 每个 block 的 typed-state 摘要
    CitationsCount int
    CitationFiles  []string        // top-N
}

type BlockSummary struct {
    ID            string
    Kind          AnswerBlockKind
    SurfaceRole   SurfaceRole
    FacetIDs      []string
    HasClaimUse   bool
    ClaimForm     ClaimForm  // when present
    HasItems      bool
    ItemCount     int
    ItemsWithClaimUse int
    ItemsWithCitation int
    TextHeadTail  string  // 400 head + 200 tail
}

type ScoredViolation struct {
    Kind     ViolationKind
    Severity Severity        // Critical / High / Medium / Soft
    Layer    string          // "scheduler" / "v2_oracle" / "contract_check"
    BlockID  string          // affected block (when applicable)
    FieldPath string         // affected field path (e.g. "blocks[id=lifecycle].claim_use")
    Detail   string
    Repair   string
}

type Severity string
const (
    SeverityCritical Severity = "critical"  // fail-loud unless fixed
    SeverityHigh     Severity = "high"      // strict by default
    SeverityMedium   Severity = "medium"    // soft by default
    SeveritySoft     Severity = "soft"      // telemetry only
)
```

### 3.2 写入路径
- `MutableState.SetRetryState(rs RetryState)` — 在 contract.Check 失败 + 决定 retry 时调用
- 两个写者:
  1. orchestrator (scheduler-level violations + ScoredViolation 装入)
  2. contract_check (V2 oracle violations + ScoredViolation 装入)
- 都用同一 RetryState 实例,append-only

### 3.3 渲染路径
- `internal/agent/answer_document_evaluator.go::renderRetryState(ctx)` 新 helper
- 触发条件:`ctx.EmitStageRetryAttempt > 0`
- 渲染输出 = §2 中三大段(Previous Emit / Active Violations / Required Changes / Hard Rule)
- 嵌入到 `BuildInitialInstruction` 在 retry attempt 时优先放到 prompt 顶部

### 3.4 Severity 推导(deterministic)
单一函数 `DeriveSeverity(kind ViolationKind, isStrict bool) Severity`:
- `ViolPrincipalClaimUseMissing / ViolBlockCoverageMissing` → Critical(LLM 必须修)
- `ViolFacetUncovered / ViolDiagramEdgeUnsupported / ViolUncertaintyBlockMissing` → High(strict-by-default)
- `ViolClaimFormUnsupported / ViolDeclaredCountDrift / ViolSelfContradiction` → Medium(strict 但有时容易绕)
- `ViolRichnessRegression / ViolReflectorObservation` → Soft(永不重试)

isStrict 由 `pipeline_contract_strict_kinds` yaml 影响。

---

## 4. 真根因驱动的红线

为防再次走入"零碎补丁"反模式,加入 audit doc 红线:
- 🔴 任何 retry 路径修复**必须先回答**:这个修复是 retry-state 缺失的某个面,还是新独立类问题?
- 🔴 不允许在 N 个 retry hint corner 各加 prose 教学 — 统一通过 RetryState 渲染层
- 🔴 violation Severity 字段是单一 source of truth — 所有 retry-budget 决策、scheduler 优先级、composer 提示都消费同一字段

---

## 5. 收益矩阵(单一改动,4 个表面问题同时解决)

| 表面问题 | R14 如何解决 |
|---|---|
| R6 retry 字段失忆 | Previous Emit 段让 LLM 看见 prev typed-state,Hard Rule 强制 byte-identical 保留 |
| R6.1 block vs item 失忆 | Previous Emit 把 block-level + item-level 字段都列出来,LLM 不需猜哪一层重要 |
| R11 violation 缺 Severity | ScoredViolation.Severity 显式分组渲染,LLM 优先修 Critical |
| R13 scheduler vs V2 oracle 双层 | ScoredViolation.Layer 跨层一并渲染,LLM 看到所有 gating 层全貌 |

**新增类问题免疫力**:任何未来出现的 retry 类问题(不论是 schema 字段、validator 层、scheduler criterion),只要把对应数据加进 RetryState 即可消费,不再到处加补丁。

---

## 6. 实施切片(~10 commits,中等改动)

| commit | 范围 | 风险 |
|---|---|---|
| R14-c1 | `types/retry_state.go` 类型 + RetryState 字段加 MutableState | 低 |
| R14-c2 | `DeriveSeverity` 函数 + 内置每个 ViolationKind 默认 severity | 低 |
| R14-c3 | scheduler 写入路径(orchestrator.go retry 决策点)| 中 |
| R14-c4 | contract_check 写入路径(每个 V2 oracle 输出 ScoredViolation)| 中 |
| R14-c5 | `renderRetryState` 渲染 helper + `Previous Emit` 段渲染 | 中 |
| R14-c6 | `Active Violations` 段渲染 + Severity 分组 | 低 |
| R14-c7 | `Required Changes` 段渲染(从 ScoredViolation.FieldPath) | 中 |
| R14-c8 | `Hard Rule` 段 + retry attempt 触发条件 | 低 |
| R14-c9 | 锁 test:retry-state 完整性 + 跨层 violation 收集 | 中 |
| R14-c10 | 真 eval rerun 验证 | 中 |

---

## 7. R14 取代的 4 项原跟踪表项

更新 `post_shape_residual_audit.md`:
- ~~R6~~ → 标 "由 R14 统一解决"
- ~~R6.1~~ → 同上
- ~~R11~~ → 同上
- ~~R13~~ → 同上

R12(explorer iter 计数 cross-dispatch 误读)是**纯观测性问题**,不属于 retry-state 范畴 → 保留独立 P3。

---

## 8. 决策点

R14 是中等架构改动(~10 commits, ~600-800 LOC)。是否走 R14 路径取决于:
- **走 R14**:一次性修复 4 个 P1/P2 类问题,免疫未来同类风险,代价中
- **走零碎补丁**:R6 / R6.1 / R11 / R13 各自小改,但**不抗未来同模式新问题**,长期复杂度↑
- **混合**:R6.1 prompt-only 短期改 + R14 中期落

**强建议**:走 R14 — 跨层共性根因决定方案应该跨层统一。
