# Selected parallel eval sweep

- date: 2026-09-24T01:54:57Z
- sweep_start_ts: 20260923-185457
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_business_tree | PASS | - | 253s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_business_tree-20260923-185457 |
| 2 | trace_query_io_inflight | FAIL | no_primary_regex_match:(^|[^0-9])1[.]40*([^0-9]|$) no_primary_regex_match:(^|[^0-9])0[.]60*([^0-9]|$) no_primary_regex_m | 831s | 1 | 2 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_io_inflight-20260923-185457 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
