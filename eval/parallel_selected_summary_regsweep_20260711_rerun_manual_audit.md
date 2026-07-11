# Selected Eval Manual Audit Scaffold

- date: 2026-07-11T06:40:25Z
- sweep_start_ts: 20260711-144025
- total cases: 3
- parallel: 1
- timeout: 600s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_b3_process_level_rollup | FAIL | eval/results/real_trace_b3_process_level_rollup-20260711-144025 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 225s | 43 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 2 | trace_query_converted_inode_io_pressure | PASS | eval/results/trace_query_converted_inode_io_pressure-20260711-144410 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 122s | 24 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 3 | read_combo_trace_current_code_dimensions | FAIL | eval/results/read_combo_trace_current_code_dimensions-20260711-144613 | trace_attachment,answer_regex | perf_triage+trace_query | 155s | 24 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=5,inv=3/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
