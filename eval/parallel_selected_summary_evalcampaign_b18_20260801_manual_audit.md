# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T03:55:16Z
- sweep_start_ts: 20260731-205515
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260731-205516 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 152s | 41 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 34579.490–34579.500s 窗保持精确；正文含根因排序、两跳唤醒链、完整 Trace 因果投影、窗内可消除量和 system_supplement 关键阻塞补采。未见 B18 对 runtime family 的回归。 |
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260731-205516 | answer_regex,answer_contains | none | 273s | 35 | read=0,repo_map=10,list=0,trace=0,source_lens=8 | midloop=10,inv=8/0,fin_reject=0,unavail=0,prune=0 | fail | typed relation handoff 已正确给出 principal=12/auxiliary=3，但 analyzer 发出 source_inventory shape；该通道以 16 条 type 行（接口+12 production+3 test）覆盖 relation 主集合，最终主表/主图仍包含 3 个测试实现。runner oracle 未禁止测试行，属 runner PASS / human FAIL。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
