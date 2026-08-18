# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T23:07:49Z
- sweep_start_ts: 20260818-160748
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_tokenizers_newline_run_multirepo_py | PASS | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260818-160750 | log_regex,write_apply,answer_regex,answer_contains | none | 647s | 25 | read=13,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 三代计划把受保护的 5-newline 回归断言从 `[300]` 改成 `[10,300]`，再让实现迎合被改写的 oracle；真实 `make check` 因测试和实现一起漂移而假绿。写前分析已发出 typed `preserve_regression_test`，但 target 被写成“路径+说明”，现有保护器无法解析，形成 B1115。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260818-160750 | answer_regex,answer_contains,mermaid_edge_count | none | 1000s | 48 | read=74,repo_map=4,list=0,trace=0,source_lens=0 | midloop=45,inv=44/0,fin_reject=2,unavail=0,prune=0 | fail | 四阶段 precedence 保留，BusContext/Mutable 也可见，但关系恢复耗时 1000s/74 次 read。`o.busCtx -> ctxbuilder.BuildAgentContext` 与 `BuildAgentContext -> bus.Mutable.Objective` 因同一 callable 的 qualified/bare identity 未合并，coverage 仍误判 Mutable 未接回请求组件；最终图退化为 helper/call 图，未画出承诺的 TurnAArtifacts/AnswerSymbols 数据流，并含抽象层不准确的 `BuildAgentContext -> Mutable call`。B1114 仅部分正证，新建 B1116。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
