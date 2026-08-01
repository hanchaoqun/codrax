# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T03:32:53Z
- sweep_start_ts: 20260731-203252
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_path_question_multi_trace_files | PASS | eval/results/trace_query_path_question_multi_trace_files-20260731-203253 | log_regex,answer_regex,answer_contains | none | 104s | 34 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 两个工件隔离、跨工件 authority 与三块显式窗投影均在；app-20 的 rank/impact 5.000ms 已收敛为唯一 E1(+1) 席，树、指标表、◎ 均只计一次，原始双 observation 仍在审计补充。模型再次把 1.001200→1.010000 的端点跨度写成 10ms，typed 板未跟错，维持 TRMV1 模型问题。 |
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260731-203253 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 115s | 36 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 10ms 窗完整 Trace 因果投影未回退：根因排序、wakeup/直接裸边 34579.496810s、窗内可消除量、0.285ms VerifyClass 类校验链上 #2、coverage 边界与系统自动补充全部在；0 finalizer reject/patch。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
