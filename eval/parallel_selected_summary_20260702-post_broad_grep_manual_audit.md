# Selected Eval Manual Audit Scaffold

- date: 2026-07-02T01:25:02Z
- sweep_start_ts: 20260702-092502
- total cases: 6
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_log_current_source_explanation | PASS | eval/results/read_combo_log_current_source_explanation-20260702-092502 | log_attachment,answer_regex | log_triage | 262s | 37 | read=10,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=1 | TODO | TODO |
| 2 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260702-092502 | trace_attachment,answer_regex | perf_triage+trace_query | 268s | 39 | read=6,repo_map=0,list=0,trace=2,source_lens=0 | midloop=4,inv=2/1,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 4 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260702-092930 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 208s | 26 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=3/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 5 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260702-093259 | typed_inventory_rowset,dimension_substring,answer_contains | none | 132s | 20 | read=5,repo_map=3,list=1,trace=0,source_lens=3 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 3 | trace_query_donghu_real_frame_multicausal | FAIL | eval/results/trace_query_donghu_real_frame_multicausal-20260702-092924 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 814s | 39 | read=0,repo_map=0,list=0,trace=58,source_lens=0 | midloop=1,inv=14/2,fin_reject=0,unavail=0,prune=0 | TODO | TODO |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
