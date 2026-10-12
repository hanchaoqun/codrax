# Selected parallel eval sweep

- date: 2026-10-12T08:12:20Z
- sweep_start_ts: 20261012-011219
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_named_route_causal_20261012

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_dual_common_window_measurements | PASS | - | 121s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/hmc_named_route_causal_20261012/trace_dual_common_window_measurements-20261012-011220 |
| 2 | trace_business_io_sequence | PASS | - | 555s | 1 | 1 | 0 | 1 | 0 | 13 | 13 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_named_route_causal_20261012/trace_business_io_sequence-20261012-011220 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
