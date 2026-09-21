# Selected parallel eval sweep

- date: 2026-09-21T01:49:42Z
- sweep_start_ts: 20260920-184942
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_business_ref_assertions_20260920

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 174s | 1 | 1 | 0 | 1 | 0 | 1 | 2 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_business_ref_assertions_20260920/trace_query_wakeup_causal_io_chain-20260920-184942 |
| 1 | trace_query_business_marker_io_chain | PASS | - | 274s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_business_ref_assertions_20260920/trace_query_business_marker_io_chain-20260920-184942 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
