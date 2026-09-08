# Selected parallel eval sweep

- date: 2026-09-08T12:44:07Z
- sweep_start_ts: 20260908-054407
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_ts_workspace_impls | PASS | - | 53s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/sr_ts_workspace_impls-20260908-054407 |
| 1 | real_trace_e1_dual_window_normalized | PASS | - | 160s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_e1_dual_window_normalized-20260908-054407 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
