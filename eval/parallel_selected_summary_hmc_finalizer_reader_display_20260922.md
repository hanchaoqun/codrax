# Selected parallel eval sweep

- date: 2026-09-22T15:14:05Z
- sweep_start_ts: 20260922-081403
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_finalizer_reader_display_20260922

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h4_supply_thermal_witness | PASS | - | 185s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_finalizer_reader_display_20260922/real_trace_h4_supply_thermal_witness-20260922-081405 |
| 2 | trace_query_wakeup_background_demotion | PASS | - | 208s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_finalizer_reader_display_20260922/trace_query_wakeup_background_demotion-20260922-081405 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
