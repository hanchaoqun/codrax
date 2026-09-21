# Selected parallel eval sweep

- date: 2026-09-21T03:27:29Z
- sweep_start_ts: 20260920-202729
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_submission_window_20260920

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 212s | 1 | 1 | 0 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_submission_window_20260920/trace_query_wakeup_causal_io_chain-20260920-202729 |
| 1 | trace_query_business_marker_io_chain | PASS | - | 247s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_submission_window_20260920/trace_query_business_marker_io_chain-20260920-202729 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
