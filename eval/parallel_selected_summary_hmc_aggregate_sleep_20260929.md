# Selected parallel eval sweep

- date: 2026-09-30T04:11:03Z
- sweep_start_ts: 20260929-211101
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_process_profile | PASS | - | 139s | 1 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_process_profile-20260929-211103 |
| 2 | trace_query_sleep_summary | PASS | - | 279s | 1 | 1 | 0 | 1 | 0 | 5 | 6 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_sleep_summary-20260929-211103 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
