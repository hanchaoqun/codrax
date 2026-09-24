# Selected parallel eval sweep

- date: 2026-09-24T09:15:05Z
- sweep_start_ts: 20260924-021502
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_existing_sqlite_jank_inventory | PASS | - | 104s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_existing_sqlite_jank_inventory-20260924-021505 |
| 2 | trace_query_io_activity | FAIL | no_primary_regex_match:(37,?888|148([.]0+)?|151,?552) | 698s | 1 | 2 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_io_activity-20260924-021505 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
