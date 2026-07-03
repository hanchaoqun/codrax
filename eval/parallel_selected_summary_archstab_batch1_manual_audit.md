# Selected Eval Manual Audit Scaffold

- date: 2026-07-03T01:37:11Z
- sweep_start_ts: 20260703-093711
- total cases: 6
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | LAUNCH_FAIL |  | log_regex,trace_attachment,answer_regex,answer_contains | - | 0s | - | read=-,repo_map=-,list=-,trace=-,source_lens=- | midloop=-,inv=-/-,fin_reject=0,unavail=-,prune=- | TODO | TODO |
| 2 | trace_query_state_churn_root_cause_rank | PASS | eval/results/trace_query_state_churn_root_cause_rank-20260703-093711 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 131s | 22 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 3 | read_combo_trace_current_code_boundary | PASS | eval/results/read_combo_trace_current_code_boundary-20260703-093712 | trace_attachment,answer_regex | perf_triage+trace_query | 211s | 35 | read=4,repo_map=2,list=0,trace=2,source_lens=0 | midloop=2,inv=3/2,fin_reject=0,unavail=1,prune=0 | TODO | TODO |
| 4 | logtri_go | PASS | eval/results/logtri_go-20260703-093923 | log_attachment,answer_contains | log_triage | 169s | 22 | read=5,repo_map=0,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=0,unavail=1,prune=0 | TODO | TODO |
| 6 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260703-094213 | typed_inventory_rowset,dimension_substring,answer_contains | none | 173s | 23 | read=5,repo_map=4,list=0,trace=0,source_lens=4 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 5 | qf_multi_member_set_count_caveat | PASS | eval/results/qf_multi_member_set_count_caveat-20260703-094044 | answer_regex,answer_contains | none | 738s | 51 | read=1,repo_map=16,list=0,trace=0,source_lens=16 | midloop=12,inv=12/2,fin_reject=0,unavail=0,prune=4 | TODO | TODO |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
