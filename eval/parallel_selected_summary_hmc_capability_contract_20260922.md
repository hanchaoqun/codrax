# Selected parallel eval sweep

- date: 2026-09-23T04:11:21Z
- sweep_start_ts: 20260922-211120
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_capability_contract_20260922

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | nested_python_increment | PASS | - | 113s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_capability_contract_20260922/nested_python_increment-20260922-211121 |
| 1 | trace_capability_discovery | FAIL | missing_primary:window_stats missing_primary:scheduler_latency_stats missing_primary:perf_stats | 115s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/hmc_capability_contract_20260922/trace_capability_discovery-20260922-211121 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
