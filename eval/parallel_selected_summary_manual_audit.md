# Selected Eval Manual Audit Scaffold

- date: 2026-07-04T19:02:01Z
- sweep_start_ts: 20260705-030201
- total cases: 9
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_relation_subagent_registry | PASS | eval/results/qf_relation_subagent_registry-20260705-030201 | answer_regex,answer_contains | none | 114s | 23 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 2 | arkts_repomap | PASS | eval/results/arkts_repomap-20260705-030201 | typed_inventory_rowset,answer_contains | none | 129s | 18 | read=6,repo_map=2,list=1,trace=0,source_lens=2 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 3 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260705-030355 | typed_inventory_rowset,dimension_substring,answer_contains | none | 227s | 28 | read=10,repo_map=2,list=0,trace=0,source_lens=2 | midloop=6,inv=4/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 4 | trace_query_openharmony_bytrace_thread | PASS | eval/results/trace_query_openharmony_bytrace_thread-20260705-030410 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 285s | 27 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=0,inv=5/1,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 5 | read_combo_log_current_source_explanation | PASS | eval/results/read_combo_log_current_source_explanation-20260705-030742 | log_attachment,answer_regex | log_triage | 125s | 28 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 7 | data_basic_sum_with_rules | FAIL | eval/results/data_basic_sum_with_rules-20260705-030948 | log_regex,answer_regex | none | 69s | 16 | read=2,repo_map=1,list=1,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 8 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260705-031058 | log_regex,answer_regex | none | 52s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 9 | data_text_filter_count | FAIL | eval/results/data_text_filter_count-20260705-031151 | log_regex,answer_regex | none | 47s | 16 | read=2,repo_map=1,list=1,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 6 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260705-030856 | trace_attachment,answer_regex | perf_triage+trace_query | 317s | 40 | read=5,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=1,prune=0 | TODO | TODO |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
