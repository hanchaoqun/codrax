# Selected parallel eval sweep

- date: 2026-07-05T17:32:00Z
- sweep_start_ts: 20260706-013200
- total cases: 5
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 145s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260706-013200 |
| 3 | trace_query_donghu_mixed_platform | PASS | - | 167s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_mixed_platform-20260706-013426 |
| 4 | trace_query_path_question_relative_donghu_short | PASS | - | 94s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/trace_query_path_question_relative_donghu_short-20260706-013714 |
| 5 | trace_query_path_question_absolute_donghu_short | PASS | - | 118s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/trace_query_path_question_absolute_donghu_short-20260706-013848 |
| 2 | trace_query_donghu_real_short_runnable | FAIL | banned:still_present | 730s | 1 | 8 | 0 | 1 | 0 | 2 | 1 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_short_runnable-20260706-013200 |

**Pass: 4 / 5 — Fail/Timeout/LaunchFail: 1**
