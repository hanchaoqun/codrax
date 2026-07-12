# Selected parallel eval sweep

- date: 2026-07-12T08:59:59Z
- sweep_start_ts: 20260712-165959
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 138s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260712-165959 |
| 2 | real_trace_a5_excerpt_degenerate_window | PASS | - | 199s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_a5_excerpt_degenerate_window-20260712-165959 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
