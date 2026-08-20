# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T09:47:58Z
- sweep_start_ts: 20260820-024758
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-024758 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 194s | 34 | read=0,repo_map=0,list=0,trace=12,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 主窗、threadpool-400→network-300→cookie-200→app-100 唤醒链、11.000ms IO 链首、三项 1.000ms 调度供给、真实占用/可消除双轴及 Trace 因果投影均保留；邻近/背景未夺冠，综合评分以非墙钟旁栏显示，未出现 4ms/活跃流降级。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260820-024758 | answer_regex,answer_contains,mermaid_edge_count | none | 468s | 45 | read=25,repo_map=2,list=0,trace=0,source_lens=0 | midloop=10,inv=5/0,fin_reject=6,unavail=1,prune=0 | fail | B1225 的局部范围字段最终正确发布为读者语言且 raw enum 未泄漏；但 6 次成文拒绝暴露 local-only candidate/未证边界判定摆动。最终正文还把 BusContext 说成“不可变容器”、把 BuildAgentContext 说成注入 BusContext 指针，并把 EmittedAnswerSymbols/EmittedHypothesisVerdicts 读取误述为写入；引用行本身与结论相反，答案未达到人工正确性。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
