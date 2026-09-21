# Selected parallel eval sweep

- date: 2026-09-21T10:44:24Z
- sweep_start_ts: 20260921-034424
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_mixed_test_status_crossmode_20260921

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | nested_python_increment | PASS | - | 116s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_mixed_test_status_crossmode_20260921/nested_python_increment-20260921-034424 |
| 2 | trace_query_business_marker_io_chain | FAIL | missing_primary:LoadDocumentIndex no_primary_regex_match:(^|[^0-9])35([.]0+)?[[:space:]*]*(ms|毫秒) | 279s | 1 | 2 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_mixed_test_status_crossmode_20260921/trace_query_business_marker_io_chain-20260921-034424 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
