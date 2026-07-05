# Selected parallel eval sweep

- date: 2026-07-05T13:24:08Z
- sweep_start_ts: 20260705-212408
- total cases: 1
- parallel: 1
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | real_trace_e1_dual_window_normalized | PASS | - | 147s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_e1_dual_window_normalized-20260705-212408 |

**Pass: 1 / 1 — Fail/Timeout/LaunchFail: 0**
