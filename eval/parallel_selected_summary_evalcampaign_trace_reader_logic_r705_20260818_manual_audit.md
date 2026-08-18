# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T21:34:13Z
- sweep_start_ts: 20260818-143411
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260818-143413 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 267s | 48 | read=2,repo_map=0,list=0,trace=5,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=1 | partial | B1110a 生产正证：唤醒概览已是自然中文；显式窗、全量状态分区、链上排序、实测/规则可消双轴、因果投影、业务 span 和背景降格均保留，零 final reject/4ms 降级。`ransmitThread` 是原始 fixture 真值，explorer 曾误补成 `transmitThread`，终稿恢复原值。残余：模型正文仍复制内部 cause 枚举，标题称“从小到大”但数值降序；78.630ms 的链上已发布 sleep 片段没有与 118.586ms 全窗 sleep 分区明确区分。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260818-143413 | answer_regex,answer_contains,mermaid_edge_count | none | 408s | 38 | read=22,repo_map=3,list=0,trace=0,source_lens=1 | midloop=14,inv=6/0,fin_reject=2,unavail=0,prune=0 | partial | Mermaid 合法；四阶段顺序、调用和参数流锚准确，包含关系以无箭头分组表达。两次拒绝分别纠正“把包含误写为 precedence”和“BusContext 端点未映射”，合同精确且最终收敛；但修补时模型删除了其他已证关系，终图成为多个孤立子图，未把共享上下文与四阶段主链完整整合。系统没有代写边；当前按模型/证据选择 partial 观察，禁止新增连通性硬门或系统合成关系。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
