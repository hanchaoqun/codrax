# Selected parallel eval sweep

- date: 2026-10-09T11:49:55Z
- sweep_start_ts: 20261009-044954
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_registration_intervals_20261009

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | native_registration_commandless | PASS | - | 158s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_registration_intervals_20261009/native_registration_commandless-20261009-044955 |
| 1 | trace_measurement_records | PASS | - | 175s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_registration_intervals_20261009/trace_measurement_records-20261009-044955 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
