# Selected parallel eval sweep

- date: 2026-09-30T07:58:50Z
- sweep_start_ts: 20260930-005849
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_sleep_summary | PASS | - | 218s | 1 | 1 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_sleep_summary-20260930-005850 |
| 2 | trace_query_sleep_dependencies | PASS | - | 366s | 1 | 2 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_sleep_dependencies-20260930-005850 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
