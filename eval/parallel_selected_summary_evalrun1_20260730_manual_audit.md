# Selected Eval Manual Audit Scaffold

- date: 2026-07-30T11:03:18Z
- sweep_start_ts: 20260730-040318
- total cases: 12
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_blocked_reason_chain | PASS | eval/results/trace_query_blocked_reason_chain-20260730-040319 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 148s | 28 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 3 | logtri_go | PASS | eval/results/logtri_go-20260730-040546 | log_attachment,answer_contains | log_triage | 126s | 20 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 2 | trace_query_state_churn_root_cause_rank | PASS | eval/results/trace_query_state_churn_root_cause_rank-20260730-040319 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 309s | 31 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=3/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 4 | logtri_java | PASS | eval/results/logtri_java-20260730-040752 | log_attachment,answer_regex,answer_contains | log_triage | 98s | 16 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 6 | read_combo_log_current_code_boundary | PASS | eval/results/read_combo_log_current_code_boundary-20260730-040930 | log_attachment,answer_regex | log_triage | 121s | 24 | read=1,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 5 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260730-040828 | trace_attachment,answer_regex | perf_triage+trace_query | 186s | 36 | read=3,repo_map=1,list=0,trace=2,source_lens=0 | midloop=3,inv=2/1,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 7 | qf_architecture | PASS | eval/results/qf_architecture-20260730-041132 | answer_regex,answer_contains | none | 120s | 26 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 8 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260730-041135 | answer_regex,answer_contains | none | 128s | 23 | read=7,repo_map=1,list=0,trace=0,source_lens=1 | midloop=6,inv=2/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 10 | github_issue_gson_lazy_number | PASS | eval/results/github_issue_gson_lazy_number-20260730-041344 | write_apply,write_patch_oracle | none | 140s | 17 | read=7,repo_map=3,list=1,trace=0,source_lens=2 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 9 | github_issue_zod_prefault | PASS | eval/results/github_issue_zod_prefault-20260730-041333 | write_apply,answer_regex | none | 252s | 17 | read=9,repo_map=3,list=1,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 11 | data_json_strict_ids | FAIL | eval/results/data_json_strict_ids-20260730-041605 | log_regex,answer_regex | none | 230s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 12 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260730-041745 | log_regex,answer_regex | none | 134s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
