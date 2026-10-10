# Selected parallel eval sweep

- date: 2026-10-10T07:43:45Z
- sweep_start_ts: 20261010-004343
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_route_literal_20261010

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | log_shared_sources | PASS | - | 266s | 1 | 2 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | log_triage+log_query | eval/results/hmc_route_literal_20261010/log_shared_sources-20261010-004345 |
| 1 | trace_dual_measurement_records | PASS | - | 359s | 1 | 1 | 0 | 1 | 0 | 1 | 2 | 0 | 0 | 0 | none | eval/results/hmc_route_literal_20261010/trace_dual_measurement_records-20261010-004345 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
