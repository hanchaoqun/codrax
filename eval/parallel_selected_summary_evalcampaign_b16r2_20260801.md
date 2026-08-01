# Selected parallel eval sweep

- date: 2026-08-01T03:00:22Z
- sweep_start_ts: 20260731-200020
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_path_question_multi_trace_files | PASS | - | 105s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/trace_query_path_question_multi_trace_files-20260731-200022 |
| 2 | qf_type_relation_loop_controller | PASS | - | 299s | 1 | 4 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260731-200022 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
