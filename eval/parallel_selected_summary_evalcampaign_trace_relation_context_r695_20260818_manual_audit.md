# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T17:09:37Z
- sweep_start_ts: 20260818-100936
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260818-100937 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 236s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | B1093 prompt-pool 投影已真实生效：Finalizer 明示 40 条跨界 deterministic row 退出 answer-writing view，旧 investigation narrative 也未再出现；8.300ms 实测/有效归因、9.000ms 链累计、链上 #1 worker-200、背景 support-only 与 Trace 因果投影均保留。但 Runtime Trace Root-Cause Board 仍把查询窗 1.000000..1.011000 的 app-100 0.020ms 行列为 `#2 root-cause seat`，模型据此三处写成所选窗内 runnable/#2，和最终边界卡的 `unranked_context_row/supporting_context_only/ordinal forbidden` 冲突。根因是 root board 将 ±1ms 同窗聚类容差误作 explicit requested-window 精确身份；不是 trace 缺证或单纯模型波动。唯一拒绝是模型把 citations 发成不可恢复的畸形 JSON 字符串，第二轮原生数组成功，未触发系统代写或 4ms 降级。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260818-100937 | answer_regex,answer_contains | none | 414s | 35 | read=8,repo_map=1,list=0,trace=0,source_lens=0 | midloop=8,inv=3/0,fin_reject=4,unavail=0,prune=0 | fail | 最终 Mermaid 合法，正确只画已证 `buildAnalysisIR -> Normalize/…/RunWith` 扇出并把 `gate.Run` 保留为未证终点，没有系统造边；但过程 4 次成文拒绝。首稿漏 from_identity/to_identity，原因是通用 Diagram 文档仍写 `edge_anchors` “EXACTLY three fields”，与刚统一的 call-chain 合同及硬校验要求 identity/visible_label 直接矛盾；patch 又遇到 participant_boundaries 字段教学与 claim_uses 元数据遗漏。终稿文字把源码真实方向 `Run -> RunWith` 说反成 “RunWith 是 Run/gate.Run 的包装器”，并把函数内依次调用误写成 callee 间调用顺序，人工不可签正确。B1094 仅 partial，新记 B1095 统一结构字段合同。无系统代写关系/结论，无 4ms 降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `B1093-SELECTEDWINDOWPOSTBOUNDARY1`: production still failed. The new Finalizer principal-pool projection is positive, but a second high-salience board uses the wrong tolerance semantics.
- `B1096-EXPLICITWINDOWTOLERANCESEMANTICS1`: new P0/P1. The shared ±1ms tolerance is valid for disclosure dedupe/grouping, not for user-owned exact-window identity or containment. Use the existing principal-value window tolerance for explicit-scope authority surfaces; keep broad grouping unchanged.
- `B1094-RELATIONBLOCKANCHORTEACHING1`: production partial. The checklist now carries the shared call-chain rule, but older generic diagram documentation still contradicts it.
- `B1095-RELATIONANCHORSCHEMADRIFT1`: new P1. Generic JSON teaching says exactly three fields while the grounded standalone relation contract/validator requires exact identities and visible label. Generate both descriptions from one schema-aware distinction; do not weaken typed edge validation or synthesize edges.
- `active-stream-4ms-degrade`: forbidden and not observed.
