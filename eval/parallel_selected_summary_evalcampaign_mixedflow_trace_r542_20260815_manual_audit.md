# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T01:08:58Z
- sweep_start_ts: 20260815-180856
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260815-180858 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 164s | 41 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Trace 因果投影、链上排序、占时/业务 span 与规则可消除双轴均在场，邻近/背景未升主因；但模型违背同一 prompt 的 typed 关系权威：把 #2+#3 的“不相交可加小计 12.115ms”说成物理重叠竞争，把未证的 kernel caller/D-state 关系扩写成 IO→fence→解除反转的顺序因果，并跨未知关系把 3.670+3.598 相加。runner 只钉了 de-minimis overlap 机制与主值，未覆盖结论关系质量。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260815-180858 | answer_regex,answer_contains,mermaid_edge_count | none | 257s | 40 | read=12,repo_map=2,list=0,trace=0,source_lens=0 | midloop=9,inv=3/0,fin_reject=5,unavail=0,prune=1 | partial | 阶段顺序、组件职责、BusContext 内 Mutable 无箭头包含均正确；但用户要求的阶段↔Mutable/BusContext 数据流仍未进入图。B867 的 stage_artifact_group_recipe 已到 finalizer，却未被模型采用；本轮也没有读取/提交目标多返回值赋值行，故 assignment+call+argument 修复臂未获得生产正证。模型正文仍断言 Explorer/Extractor 写 Mutable，与图中“未证/断开”矛盾。5 次 finalizer reject 与 3 次 Mermaid repair 说明关系身份合同仍带来高 churn。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch judgment

- `B867-MULTIRESULTASSIGNMENTAUTHORITY1`: implementation pins remain green, but this replay did not exercise the target source shape; status stays pending production witness.
- `B867-STAGEARTIFACTGROUPRECIPE1`: prompt wiring is production-positive, answer adoption is partial; the system correctly did not synthesize arrows.
- `B868-REQUESTEDCARRIERRELATIONHANDOFF1` (new P1): requested stage/carrier flow can still finish with disconnected carriers after the model cites only local method calls. Audit the typed participant resolver and request-scoped operation candidate mapping; do not solve it by matching Mermaid labels or rewriting the graph.
- `B869-TRACEDIRECTIONRELATIONDECISIONROSTER1` (new P0): typed relation facts exist but are distributed across a 131k-character prompt and expressed partly as internal tokens. Publish one compact, copy-ready typed relation roster: exact disjoint subtotal, exact physical-overlap pairs, and unresolved/unlisted pairs. It is prompt-only and must not scan/rewrite prose or author the conclusion.
- `RLOG1-FINALRUNTIMEFACTBLACKOUT`: the two customer logs are from `v0.1.20260813`; current main already feeds ObservationLedger facts into isolated prose fallback and appends deterministic runtime sections after model prose. Customer replay remains pending; active-byte streams remain exempt from fixed-age degradation.
