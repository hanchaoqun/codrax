# Selected parallel eval sweep

- date: 2026-10-12T03:45:55Z
- sweep_start_ts: 20261011-204555
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_native_timeline_20261011

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_dual_measurement_unbounded_partial | PASS | - | 103s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_native_timeline_20261011/trace_dual_measurement_unbounded_partial-20261011-204555 |
| 2 | trace_dual_source_window_timeline | FAIL | degraded_answer_checks_skipped:1 | 415s | 1 | 0 | 0 | 1 | 0 | 10 | 9 | 0 | 0 | 0 | none | eval/results/hmc_native_timeline_20261011/trace_dual_source_window_timeline-20261011-204555 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
