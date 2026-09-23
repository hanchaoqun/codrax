# Selected parallel eval sweep

- date: 2026-09-23T06:35:50Z
- sweep_start_ts: 20260922-233548
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_verification_delivery_20260923

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | empty_python_module_apply | PASS | - | 144s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_verification_delivery_20260923/empty_python_module_apply-20260922-233550 |
| 2 | trace_query_business_marker_io_chain | FAIL | no_primary_regex_match:(^|[^0-9])35([.]0+)?[[:space:]*]*(ms|毫秒) | 304s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_verification_delivery_20260923/trace_query_business_marker_io_chain-20260922-233550 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
