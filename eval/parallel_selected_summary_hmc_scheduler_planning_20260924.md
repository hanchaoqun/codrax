# Selected parallel eval sweep

- date: 2026-09-24T10:12:46Z
- sweep_start_ts: 20260924-031244
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_scheduler_concurrency_distribution | PASS | - | 246s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_scheduler_concurrency_distribution-20260924-031246 |
| 2 | trace_query_io_activity | FAIL | no_primary_regex_match:(^|[^0-9])28([.]0+)?([^0-9]|$) | 421s | 1 | 2 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_io_activity-20260924-031246 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
