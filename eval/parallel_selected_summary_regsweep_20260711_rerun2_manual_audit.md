# Selected Eval Manual Audit Scaffold

- date: 2026-07-11T07:05:30Z
- sweep_start_ts: 20260711-150530
- total cases: 4
- parallel: 1
- timeout: 600s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_dayjs_duration_nan_symptom | PASS | eval/results/github_issue_dayjs_duration_nan_symptom-20260711-150530 | write_apply,answer_regex | none | 183s | 17 | read=8,repo_map=3,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 2 | data_basic_sum_with_rules | PASS | eval/results/data_basic_sum_with_rules-20260711-150833 | log_regex,answer_regex | none | 256s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 3 | cflow_resolve_retry_storm_early_exit | PASS | eval/results/cflow_resolve_retry_storm_early_exit-20260711-151250 | answer_regex | none | 272s | 25 | read=17,repo_map=3,list=0,trace=0,source_lens=0 | midloop=8,inv=5/1,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 4 | qf_architecture | PASS | eval/results/qf_architecture-20260711-151722 | answer_regex,answer_contains | none | 104s | 23 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
