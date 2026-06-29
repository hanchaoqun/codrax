# Selected Eval Manual Audit Scaffold

- date: 2026-06-29T13:05:52Z
- sweep_start_ts: 20260629-210552
- total cases: 6
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_relation_subagent_registry | PASS | eval/results/qf_relation_subagent_registry-20260629-210552 | answer_regex,answer_contains | none | 90s | 23 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 2 | arkts_repomap | PASS | eval/results/arkts_repomap-20260629-210552 | typed_inventory_rowset,answer_contains | none | 194s | 20 | read=6,repo_map=1,list=1,trace=0,source_lens=1 | midloop=5,inv=1/0,fin_reject=2,unavail=0,prune=0 | TODO | TODO |
| 3 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260629-210723 | typed_inventory_rowset,dimension_substring,answer_contains | none | 139s | 22 | read=4,repo_map=4,list=0,trace=0,source_lens=4 | midloop=6,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 4 | trace_query_openharmony_bytrace_thread | PASS | eval/results/trace_query_openharmony_bytrace_thread-20260629-210907 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 121s | 18 | read=2,repo_map=0,list=0,trace=1,source_lens=0 | midloop=1,inv=2/1,fin_reject=0,unavail=1,prune=0 | TODO | TODO |
| 5 | read_combo_log_current_source_explanation | PASS | eval/results/read_combo_log_current_source_explanation-20260629-210943 | log_attachment,answer_regex | log_triage | 183s | 27 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 6 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260629-211109 | trace_attachment,answer_regex | perf_triage | 137s | 24 | read=1,repo_map=0,list=0,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
