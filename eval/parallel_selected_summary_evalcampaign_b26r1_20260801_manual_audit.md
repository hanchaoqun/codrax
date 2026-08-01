# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T21:28:48Z
- sweep_start_ts: 20260801-142846
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_nlohmann_long_double_symptom | PASS | eval/results/github_issue_nlohmann_long_double_symptom-20260801-142848 | write_apply,answer_regex | none | 152s | 19 | read=3,repo_map=3,list=0,trace=0,source_lens=1 | midloop=2,inv=0/0,fin_reject=0,unavail=2,prune=0 | pass | applied tree 仅两份发布头各一行 %.*lg→%.*Lf；make check 严格编译/运行通过；单批 apply+verify+finish，无二次修改；planner 在读预算耗尽后仍尝试 grep/read_file 两次，属不影响交付的 P2 过程债 |
| 1 | real_trace_h10_spantop_member_subrows | PASS | eval/results/real_trace_h10_spantop_member_subrows-20260801-142848 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 251s | 36 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | typed 投影正确给出 Jit thread pool-17284 的 JIT 2 段=2.388ms（1.781+0.607，成员行号完整），但模型首块两次断言窗口内零语义优化 span；自动 oracle 只检查后置系统表，漏判同答硬矛盾 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
