# Selected parallel eval sweep

- date: 2026-09-20T15:42:11Z
- sweep_start_ts: 20260920-084211
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_native_duration_20260920

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 171s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_native_duration_20260920/trace_query_wakeup_causal_io_chain-20260920-084212 |
| 1 | trace_query_business_marker_io_chain | PASS | - | 384s | 1 | 2 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_native_duration_20260920/trace_query_business_marker_io_chain-20260920-084212 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
