# Selected parallel eval sweep

- date: 2026-09-22T08:13:40Z
- sweep_start_ts: 20260922-011336
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_origin_component_20260922

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_wakeup_background_demotion | PASS | - | 206s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_origin_component_20260922/trace_query_wakeup_background_demotion-20260922-011340 |
| 2 | read_combo_trace_current_source_explanation | PASS | - | 421s | 1 | 2 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_origin_component_20260922/read_combo_trace_current_source_explanation-20260922-011340 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
