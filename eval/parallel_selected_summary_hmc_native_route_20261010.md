# Selected parallel eval sweep

- date: 2026-10-10T07:12:34Z
- sweep_start_ts: 20261010-001233
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_native_route_20261010

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | log_shared_sources | PASS | - | 201s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | log_triage+log_query | eval/results/hmc_native_route_20261010/log_shared_sources-20261010-001234 |
| 1 | trace_dual_measurement_records | FAIL | read_exit:1 no_log_regex:phase=toolcall .*tool=trace_query | 228s | 0 | 0 | 0 | 0 | 6 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_native_route_20261010/trace_dual_measurement_records-20261010-001234 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
