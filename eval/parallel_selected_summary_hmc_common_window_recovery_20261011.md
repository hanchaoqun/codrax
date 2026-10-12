# Selected parallel eval sweep

- date: 2026-10-12T06:36:55Z
- sweep_start_ts: 20261011-233653
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_common_window_recovery_20261011

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_dual_common_window_measurements | FAIL | no_successful_tool_result:trace_query | 52s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_common_window_recovery_20261011/trace_dual_common_window_measurements-20261011-233655 |
| 2 | trace_business_io_sequence | PASS | - | 583s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_common_window_recovery_20261011/trace_business_io_sequence-20261011-233655 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
