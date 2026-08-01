# Selected parallel eval sweep

- date: 2026-08-01T09:55:15Z
- sweep_start_ts: 20260801-025513
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_c2_dstate_iowait | PASS | - | 146s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c2_dstate_iowait-20260801-025515 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | - | 160s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260801-025515 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
