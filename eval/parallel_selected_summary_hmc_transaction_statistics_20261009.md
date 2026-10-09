# Selected parallel eval sweep

- date: 2026-10-09T08:08:02Z
- sweep_start_ts: 20261009-010800
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_transaction_statistics_20261009

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_preferred_frame_rate | PASS | - | 254s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_transaction_statistics_20261009/trace_preferred_frame_rate-20261009-010802 |
| 1 | trace_transaction_handoffs | PASS | - | 495s | 1 | 3 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_transaction_statistics_20261009/trace_transaction_handoffs-20261009-010802 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
