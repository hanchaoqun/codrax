# Selected parallel eval sweep

- date: 2026-09-23T09:34:55Z
- sweep_start_ts: 20260923-023455
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_native_resource_identity_20260923

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_smartperf_resources | PASS | - | 130s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_native_resource_identity_20260923/trace_query_smartperf_resources-20260923-023455 |
| 1 | trace_query_native_resource_identity | PASS | - | 248s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_native_resource_identity_20260923/trace_query_native_resource_identity-20260923-023455 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
