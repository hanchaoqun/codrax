# Selected parallel eval sweep

- date: 2026-10-12T01:45:45Z
- sweep_start_ts: 20261011-184544
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_comparison_display_20261011

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_dual_measurement_partial | FAIL | no_successful_tool_result:trace_query | 90s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/hmc_comparison_display_20261011/trace_dual_measurement_partial-20261011-184545 |
| 2 | log_shared_sources | PASS | - | 391s | 1 | 4 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage+log_query | eval/results/hmc_comparison_display_20261011/log_shared_sources-20261011-184545 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
