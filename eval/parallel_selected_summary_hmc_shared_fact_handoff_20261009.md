# Selected parallel eval sweep

- date: 2026-10-10T03:10:55Z
- sweep_start_ts: 20261009-201052
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_shared_fact_handoff_20261009

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | log_shared_sources | PASS | - | 222s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage+log_query | eval/results/hmc_shared_fact_handoff_20261009/log_shared_sources-20261009-201055 |
| 2 | trace_measurement_records | PASS | - | 237s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_shared_fact_handoff_20261009/trace_measurement_records-20261009-201055 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
