# Selected parallel eval sweep

- date: 2026-09-23T08:09:50Z
- sweep_start_ts: 20260923-010948
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_io_value_caliber_20260923

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | empty_python_module_apply | PASS | - | 191s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_io_value_caliber_20260923/empty_python_module_apply-20260923-010950 |
| 1 | trace_query_business_marker_io_chain | PASS | - | 346s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_io_value_caliber_20260923/trace_query_business_marker_io_chain-20260923-010950 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
