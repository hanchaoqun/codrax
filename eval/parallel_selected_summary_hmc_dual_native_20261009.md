# Selected parallel eval sweep

- date: 2026-10-10T06:38:22Z
- sweep_start_ts: 20261009-233821
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_dual_native_20261009

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_dual_measurement_records | FAIL | no_log_regex:phase=toolcall .*tool=trace_query | 109s | 0 | 0 | 0 | 0 | 2 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_dual_native_20261009/trace_dual_measurement_records-20261009-233823 |
| 2 | log_shared_sources | PASS | - | 335s | 1 | 2 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | log_triage+log_query | eval/results/hmc_dual_native_20261009/log_shared_sources-20261009-233823 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
