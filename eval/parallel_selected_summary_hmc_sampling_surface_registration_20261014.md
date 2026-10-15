# Selected parallel eval sweep

- date: 2026-10-15T03:45:11Z
- sweep_start_ts: 20261014-204511
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_sampling_surface_registration_20261014

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_filtered_sampling_hotspots | PASS | - | 164s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_sampling_surface_registration_20261014/trace_filtered_sampling_hotspots-20261014-204511 |
| 2 | native_registration_commandless | PASS | - | 186s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_sampling_surface_registration_20261014/native_registration_commandless-20261014-204511 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
