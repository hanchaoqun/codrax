# Selected parallel eval sweep

- date: 2026-09-20T02:57:02Z
- sweep_start_ts: 20260919-195700
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc081_io_distribution_20260920

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_business_marker_io_chain | PASS | - | 272s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc081_io_distribution_20260920/trace_query_business_marker_io_chain-20260919-195702 |
| 1 | trace_query_io_request_latency_distribution | FAIL | no_primary_regex_match:(^|[^0-9])10[.]50*([^0-9]|$) no_primary_regex_match:(^|[^0-9])10[.]90*([^0-9]|$) no_primary_regex | 485s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc081_io_distribution_20260920/trace_query_io_request_latency_distribution-20260919-195702 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
