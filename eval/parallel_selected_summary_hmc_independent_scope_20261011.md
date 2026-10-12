# Selected parallel eval sweep

- date: 2026-10-12T02:49:29Z
- sweep_start_ts: 20261011-194928
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_independent_scope_20261011

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_dual_measurement_unbounded_partial | FAIL | read_exit:1 no_successful_tool_result:trace_query | 97s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_independent_scope_20261011/trace_dual_measurement_unbounded_partial-20261011-194929 |
| 2 | trace_dual_source_window_timeline | PASS | - | 265s | 1 | 1 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | none | eval/results/hmc_independent_scope_20261011/trace_dual_source_window_timeline-20261011-194929 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
