# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T12:03:14Z
- sweep_start_ts: 20260731-050314
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_state_churn_root_cause_rank | PASS | eval/results/trace_query_state_churn_root_cause_rank-20260731-050314 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 173s | 31 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 用户窗、两次 trace_query、因果投影、absorbed-family 与 canonical snapshot 均正确；但 perf pre-stage 的自由 summary 绕过 ledger authority 进入 finalizer，复制假定切换成本/11ms 帧外推，并把 runnable wait 叫成 churn loss。runner oracle 未覆盖语义权限。 |
| 2 | github_issue_gson_lazy_number_symptom | PASS | eval/results/github_issue_gson_lazy_number_symptom-20260731-050314 | write_apply,write_patch_oracle | none | 215s | 18 | read=6,repo_map=1,list=1,trace=0,source_lens=0 | midloop=2,inv=0/0,fin_reject=0,unavail=1,prune=0 | fail | W1 已修：两代 run_tests 均真实执行。生产 equals/hashCode 正确，但 structured-edit relocation 将 1~20 replace 缩成1~18，生成测试末尾重复两组 `}`；JDK/Maven 缺失后 Python source oracle 未覆盖该 changed path，仍铸 authoritative PASS。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
