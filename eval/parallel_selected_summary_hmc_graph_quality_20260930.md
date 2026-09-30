# Selected parallel eval sweep

- date: 2026-09-30T09:53:25Z
- sweep_start_ts: 20260930-025323
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_existing_sqlite_hisys_scalars | PASS | - | 143s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_existing_sqlite_hisys_scalars-20260930-025325 |
| 1 | trace_query_sleep_dependencies | PASS | - | 400s | 1 | 1 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_sleep_dependencies-20260930-025325 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
