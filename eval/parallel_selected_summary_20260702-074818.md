# Selected parallel eval sweep

- date: 2026-07-01T23:48:18Z
- sweep_start_ts: 20260702-074818
- total cases: 6
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | trace_query_path_question_multi_trace_files | PASS | - | 105s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/trace_query_path_question_multi_trace_files-20260702-074818 |
| 3 | read_combo_trace_current_source_explanation | PASS | - | 185s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_source_explanation-20260702-075004 |
| 4 | cangjie_repomap | PASS | - | 112s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260702-075310 |
| 5 | arkts_repomap | PASS | - | 96s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260702-075503 |
| 6 | read_combo_log_current_source_explanation | FAIL | no_regex_match:internal/(orchestrator|agent|llm|render|tool)/[^[:space:]]+\.go:[0-9]+ | 134s | 2 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/read_combo_log_current_source_explanation-20260702-075639 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 866s | 1 | 10 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260702-074818 |

**Pass: 5 / 6 — Fail/Timeout/LaunchFail: 1**
