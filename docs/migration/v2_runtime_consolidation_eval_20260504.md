> Status: archived (2026-05-05). Current architecture lives in docs/architecture.md and docs/design/v3_runtime_consolidation.md.

# V2 Runtime 收口 — 真 eval 验证 (2026-05-04)

**Branch**: `main` @ `0143b2f`(Session 2 末尾,Phase 3+5+6 全 ship)
**Eval 集**: s1a + m1a + u3a 各 ×2 = 6 runs
**总体结果**: **6/6 PASS** ✅

## 1. Pass rate

| Case | run-1 | run-2 | 备注 |
|------|-------|-------|------|
| s1a (mechanism enumeration) | ✅ PASS | ✅ PASS | gate.Run 9 项检查 |
| m1a (relation inquiry) | ✅ PASS | ✅ PASS | explorer / extractor 协作 |
| u3a (comparison) | ✅ PASS | ✅ PASS | ShouldStop 终止条件对比 |

## 2. Finalizer budget — Phase 3-C3 + L2 prompt-vs-schema fix 验证

**Baseline**(s1a forensic memory,2026-05-04 早期):
- m1a r1 finalizer ≈ **28 LLM calls**(prompt-vs-schema drift 烧光 budget,4 dispatch × ~7 iter)

**当前**(post Session 2):

| Case | run-1 finalizer_iters | run-2 finalizer_iters | dispatches |
|------|----:|----:|----:|
| s1a  | 6 | 8 | 2 |
| m1a  | **1** | **2** | **1** |
| u3a  | 10 | 2 | 1 |

**关键观察**:
- m1a 从 28 → 1-2 = **>14× 降幅**。Phase 3-C3 (R2 4 处同步 from_node/to_node) +
  14d9b6e (L2 schema description prompt 一致化) 直接消除了 schema-fight retry。
- s1a 仍 2 dispatch(因为 RepairPlan 路由到 back_to_explore + finalizer_only,2 轮 dispatch)
  — 这是 Phase 1 routing **预期**行为,不是 budget 烧。
- u3a run-1 finalizer_iters=10 是 outlier(LLM 早期 citation_ref-inside-claim_use 误判
  + R4.1 nested-string repair fired)— 但单一 dispatch 内回收,最终 PASS。

## 3. Phase 1 RepairPlan 在 prod 正常 cluster

`repair_plan=` trace 在 6 run 中 4 次 fire(2 个干净通过):

| Run | clusters | kinds | target |
|-----|---------:|-------|--------|
| s1a-1 | 2 | self_contradiction, **enumeration_label_ungrounded** | back_to_explore |
| s1a-2 | 2 | **facet_uncovered**, uncertainty_block_missing | finalizer_only |
| m1a-1 | 0 | (clean) | — |
| m1a-2 | 4 | richness_regression, **facet_uncovered**, authority_overreach, **diagram_edge_unsupported** | fail_loud |
| u3a-1 | 4 | richness_regression, **facet_uncovered**, block_coverage_missing, **enumeration_label_ungrounded** | fail_loud |
| u3a-2 | 0 | (clean) | — |

**关键观察**:
- A1 cooccurrence rules 把多个 violation 聚成 typed cluster — 4-cluster 案例
  (m1a-2, u3a-1)显示算法在复杂错配时仍精确分组。
- A4 Telemetry `repair_plan=` trace 行可 grep,operator 可看 routing 形态。
- target 多样:`back_to_explore` / `finalizer_only` / `fail_loud` 都 fire,
  说明 deepest-cluster owner 算法正确分流。

## 4. Phase 3 C5 + Phase 5 E1 在 prod 真 fire

| 新 Validator | 触发 run | 数据 |
|--------------|---------|------|
| **ViolDiagramEdgeUnsupported** (Phase 3 C5 Layer 2) | m1a-2 | 1× cluster 内 |
| **ViolFacetUncovered** (Phase 5 E1) | s1a-2, m1a-2, u3a-1 | 3/6 runs |
| **ViolEnumerationLabelUngrounded** (L3 oracle, 14d9b6e) | s1a-1, u3a-1 | 2/6 runs |
| **ViolRichnessRegression** (R10 frequency bridge) | m1a-2, u3a-1 | 2/6 runs |

**关键观察**:
- Phase 3 C5 Layer 2 在 m1a-2 真触发(LLM 写了带 label 的 edge 但缺 anchored
  claim_use)— 0 false positive 在其他 5 run。
- Phase 5 E1 evidence-sufficient gate 在 50% runs 真 fire — 既证明 gate 在工作,
  也证明 LLM 仍会漏 cover required facet(downstream 真 problem)。

## 5. Phase 3 C3 + Phase 5 E2 LLM-facing 字段可见

- m1a 日志中 `from_node` / `to_node` 字段提及 2-3 次/run — Phase 3-C3 schema
  描述被 LLM 读取并尝试填写。
- s1a 日志中 `evidence: ` 字段提及 11-12 次/run — Phase 5-E2 `(evidence: N)`
  标注被渲染到 user section。

## 6. 红线核查

- ✅ R1 L1 byte-identical:`runReadSchedulerLoop` 未改字面文本
- ✅ R2 4 处同步:Phase 3-C3 from_node/to_node 全栈生效(LLM 真用)
- ✅ R3 typed signal:repair_plan kinds 全是 typed enum,0 fuzzy
- ✅ R4 No internal jargon:LLM 看到的 prompt 0 internal term(`TestNoInternalTermsInPrompts`)
- ✅ Fail-loud:m1a-2/u3a-1 走 `target=fail_loud` 后回收,无 silent error

## 7. 与历史 baseline 对比

| Metric | Pre-Session 2 (s1a forensic) | Post-Session 2 (本 eval) | 变化 |
|--------|---:|---:|---:|
| m1a r1 finalizer LLM calls | 28 | 1 | **-96%** |
| m1a r1 finalizer dispatches | 4 | 1 | **-75%** |
| 6-run pass rate | 100% (3/3) at forensic time | 100% (6/6) | 持平 |
| RepairPlan telemetry | 不存在 | 4/6 runs fire | NEW |

## 8. 残留问题(下个 session)

1. **u3a run-1 finalizer 10 iters outlier**:LLM 误把 citation_ref 放进
   claim_use,触发 R4.1 nested-string repair 链。Phase 3-C3 加新字段后
   schema 信息密度增加,LLM 可能更易混淆。**ROI**:观察后续是否复发,
   超过 1/6 频率再加 prompt 警告。

2. **Phase 1 repair_plan 在 m1a-2 + u3a-1 走 `fail_loud`**:4 cluster
   全 hit ≥ Run 末尾 — 可能是 cluster 算法 too aggressive,把简单 case
   误升级到 terminal。**ROI**:看是否让 budget 失控,可能需 tune
   EscalateAfterN 阈值。

3. **L4 self_consistency BODY-vs-evidence 盲点**(s1a forensic 旧观察)
   仍未补 — reviewer 只比 SUMMARY vs BODY,不查 BODY vs evidence pool。

## 9. 一句话结论

> **V2 runtime 收口 6 阶段(Phase 1+2+3+4+5+6)在真 eval 上验证通过**:
> 100% pass rate;m1a finalizer budget 降幅 >14×;Phase 1 RepairPlan、
> Phase 3 C5 Layer 2、Phase 5 E1 三类新 validator 在 prod 真 fire 且 0
> 红线违反。残留 outlier 待持续观察,无 blocker。
