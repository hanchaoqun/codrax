# Selected parallel eval sweep

- date: 2026-09-16T09:05:39Z
- sweep_start_ts: 20260916-020539
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_a5_excerpt_degenerate_window | PASS | - | 123s | 1 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_a5_excerpt_degenerate_window-20260916-020539 |
| 1 | sr_ts_workspace_chain | PASS | - | 274s | 1 | 2 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | none | eval/results/sr_ts_workspace_chain-20260916-020539 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
