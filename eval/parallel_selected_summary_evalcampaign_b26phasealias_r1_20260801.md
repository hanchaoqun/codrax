# Selected parallel eval sweep

- date: 2026-08-01T22:52:42Z
- sweep_start_ts: 20260801-155241
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_c2_dstate_iowait | PASS | - | 140s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c2_dstate_iowait-20260801-155242 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 159s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260801-155242 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
