> Status: archived (2026-05-05). Current architecture lives in docs/architecture.md and docs/design/v3_runtime_consolidation.md.

# R1-R3 真 eval verification (2026-05-04)

**Cases**: s1a × 2 + m1a × 2(同 baseline 配置)
**Binary**: `f8b8fd7` (post R1.1 / R1.2 / R2.1 / R2.2 / R2.3 / R2.4 / R3.1 / R5.1 / R5.2 全 ship)
**Provider**: MiniMax-M2.7-highspeed (200K context)
**Outdir**: `eval/results/s1a-20260504-024900/` + `eval/results/m1a-20260504-024904/`

---

## 1. Verdict / 通过率

| case | runs | pass | fail | vs baseline |
|---|---|---|---|---|
| s1a | 2 | 1 | 1 (run-2 missing literal `contract_complete\|criterion_resolvable`) | **同 (1/2)** |
| m1a | 2 | 2 | 0 | **同 (2/2)** |
| **total** | **4** | **3 (75%)** | **1** | **持平** |

通过率与 baseline 持平 — R1-R3 修复**不引入回归** ✓。

---

## 2. R1-R3 修复在生产真验证 (8 项 / 9 项 P1 都拿到真信号)

| ID | baseline 信号 | verification 信号 | 验证结论 |
|---|---|---|---|
| **R3.1** facet 真根因(emptySurface 短路) | facet_softened **4/4** runs | **0/4** runs | ✅ **修复有效** |
| **R2.1** self-consistency reviewer V2 重接 | dispatch 0/4 (死特性) | reviewer fired **6 次** across 4 runs;3 verdicts consistent + 1 confidence rejection | ✅ **从死到活** |
| **R2.2** finalize-local 优先 fallback 分流 | 不存在 | 触发 **3 次** (s1a r1: 2 次 used 1/2 + 2/2;m1a r2: 1 次) | ✅ **预算分配生效** |
| **R2.3a** ViolFacetUncovered V2 重接 | 0 events | **15 events** across 4 runs (s1a r1=9 / 其他 r=2) | ✅ **从无到有真信号**;但暴露 **R1.5 类问题** |
| **R2.3a** ViolRichnessRegression | 0 events | **2 events** (s1a r2 / m1a r1) | ✅ **真信号** |
| **R2.3b** ViolClaimFormUnsupported | 0 events | **0 events** | ✅ **不 false-fire** (R1.2 claim_form 1:1 真验证;LLM 没声明 form 时 EmptyClaimFormSkipped 跳过) |
| **R2.3c** ViolAbsenceScopeExceeded | 0 events | **0 events** | ✅ **不误触发** (s1a/m1a 不是 absence-class 答案) |
| **R2.4** cross_citation 选位修 | false-negative (s1a r1 应触发未触发) | **0 events** in 4 runs | 🟡 **没新触发场景** (s1a 答案 V2 emit 没出现同符号多 loci);R2.4 防御性,需要专门构造场景验证 |
| **R5.1** summary 4 列 per-agent iter | metrics.txt 有但 summary.md 没 | **summary.md 4 列渲染** | ✅ **真验证** |

---

## 3. R1.5 类问题暴露(深层 prompt-契约错位)

R2.3a 上线后产生大量 `ViolFacetUncovered`(s1a r1: 9 events):

| Run | facet_uncovered count | 主要 facet |
|---|---|---|
| s1a r1 | 9 | enumeration_item × 6, uncertainty_boundary × 3 |
| s1a r2 | 2 | enumeration_item × 2 |
| m1a r1 | 2 | current_code_path, diagram_spine |
| m1a r2 | 2 | current_code_path, diagram_spine |

**根因**(已写入 `post_shape_residual_audit.md` R1.5):
LLM 看到 schema 说 `facet_ids` "read from user section",但 user section 的 Block Contract / Required Answer Facets prose **只列 facet 名,没列 typed FacetID 字符串值**。LLM 无字面值可 copy → 不填 → 100% false-fire。

**类问题**:任何 BlockRequirement 上的 typed-set 字段(`FacetIDs / AcceptableClaimForms / SurfaceRoleHint`)只要"字段名声明在 prose 但 typed 值未到达 LLM",validator 必触发(因 validator 字面 string 匹配)。

**泛化方案** (R1.5 跟踪表行):
- `renderAnswerDocBlockContract` 在每 BlockRequirement 后追加 verbatim typed 值列(`facet_ids: ["X","Y"]` 等)
- `renderAnswerDocFacetCoverage` 在每 facet 后追加 verbatim ID 字符串
- schema 字段描述 "read from user section" → "copy verbatim from Block Contract section"

---

## 4. 性能数据(F6/F7 baseline 更新)

| run | analyze 阶段 | finalizer iters | 总时长 |
|---|---|---|---|
| s1a r1 | ~1min | 9 | 6min 38s |
| s1a r2 | ~1.5min | 2 (faster) | 6min 11s |
| m1a r1 | ~3min (analyzer 7 iters) | 4 | 9min 49s |
| m1a r2 | ~2min | 8 | 8min 8s |

**vs baseline (s1a r1 8min54s / s1a r2 9min41s / m1a r1 10min22s / m1a r2 8min6s)**:
- s1a 总时长 **减少 ~2-3 min/run**(R3.1 关掉无谓 facet softening 节省 telemetry-driven retry?)
- m1a 持平

R5.2(LLM-turn 真实计数)**已是准确的**(baseline 时已发现 1:1 LLM dispatch ↔ ASSISTANT line),per-agent metric 持续准确。

---

## 5. 下一步

1. **R1.5 落地实施**(P1,4 runs 真发现的真生产问题,15 个 facet_uncovered events)
2. R2.4 cross_citation 专门构造场景再验证(现实 case 没出现同 symbol 多 loci)
3. R1.4 docs/architecture.md shape-era 残留(P3 大写作)
4. R4 系列(P2 — nested string / F7 / diagram edge / QFComparison)

post-shape 残留 P1 主体收完(9/9),R1.5 是 verification 真发现的新 P1 项,优先排到下一批。
