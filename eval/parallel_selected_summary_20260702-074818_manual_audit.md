# Selected Eval Manual Audit Scaffold

- date: 2026-07-01T23:48:18Z
- sweep_start_ts: 20260702-074818
- total cases: 6
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_path_question_multi_trace_files | PASS | eval/results/trace_query_path_question_multi_trace_files-20260702-074818 | log_regex,answer_regex,answer_contains | none | 105s | 26 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | not_audited | Metrics look healthy; left for next manual sweep. |
| 3 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260702-075004 | trace_attachment,answer_regex | perf_triage+trace_query | 185s | 37 | read=6,repo_map=0,list=0,trace=1,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | not_audited | Mixed trace/current-source case kept bounded; use as post-fix guard with Donghu. |
| 4 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260702-075310 | typed_inventory_rowset,dimension_substring,answer_contains | none | 112s | 21 | read=5,repo_map=4,list=0,trace=0,source_lens=4 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | not_audited | Inventory path appears bounded; full correctness audit deferred. |
| 5 | arkts_repomap | PASS | eval/results/arkts_repomap-20260702-075503 | typed_inventory_rowset,answer_contains | none | 96s | 19 | read=6,repo_map=0,list=2,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | not_audited | Oracle pass but repo_map=0 remains a navigation-quality watch item, not this batch's P0. |
| 6 | read_combo_log_current_source_explanation | FAIL | eval/results/read_combo_log_current_source_explanation-20260702-075639 | log_attachment,answer_regex | log_triage | 134s | 20 | read=0,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | P1: explicit log+current-source request closed on runtime facts plus weak neighbor source; final answer missed expected current-source owner surface. Track separately from runtime-only carve-out. |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260702-074818 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 866s | 47 | read=27,repo_map=4,list=0,trace=66,source_lens=0 | midloop=7,inv=12/1,fin_reject=0,unavail=0,prune=2 | fail_despite_oracle_pass | P0: runtime-only trace was dragged back into codebase/source-localizer retry; `emit_investigation_complete` bounced many times and source repo_map/read_file occurred despite source-optional runtime lane. Fixed in §7.24, pending eval retest. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
