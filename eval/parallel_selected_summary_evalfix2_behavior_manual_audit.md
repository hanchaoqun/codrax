# Selected Eval Manual Audit Scaffold

- date: 2026-07-30T14:51:49Z
- sweep_start_ts: 20260730-075149
- total cases: 6
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_gson_lazy_number | PASS | eval/results/github_issue_gson_lazy_number-20260730-075149 | write_apply,write_patch_oracle | none | 183s | 17 | read=8,repo_map=3,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 1 | github_issue_zod_prefault | PASS | eval/results/github_issue_zod_prefault-20260730-075149 | write_apply,answer_regex | none | 299s | 17 | read=11,repo_map=4,list=1,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 3 | trace_query_state_churn_root_cause_rank | PASS | eval/results/trace_query_state_churn_root_cause_rank-20260730-075453 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 131s | 29 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 5 | read_combo_log_current_code_boundary | FAIL | eval/results/read_combo_log_current_code_boundary-20260730-075705 | log_attachment,answer_regex | log_triage | 104s | 19 | read=0,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 4 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260730-075649 | trace_attachment,answer_regex | perf_triage | 196s | 36 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=2,unavail=0,prune=0 | TODO | TODO |
| 6 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260730-075850 | answer_regex,answer_contains | none | 107s | 21 | read=6,repo_map=1,list=0,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
