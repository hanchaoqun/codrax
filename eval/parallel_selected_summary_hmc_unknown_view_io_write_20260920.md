# Selected parallel eval sweep

- date: 2026-09-20T11:46:44Z
- sweep_start_ts: 20260920-044644
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_unknown_view_io_write_20260920

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_dateutil_relativedelta_float_symptom | PASS | - | 153s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_unknown_view_io_write_20260920/github_issue_dateutil_relativedelta_float_symptom-20260920-044644 |
| 1 | trace_query_io_request_latency_distribution | PASS | - | 272s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_unknown_view_io_write_20260920/trace_query_io_request_latency_distribution-20260920-044644 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
