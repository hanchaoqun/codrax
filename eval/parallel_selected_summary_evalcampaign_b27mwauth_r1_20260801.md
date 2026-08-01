# Selected parallel eval sweep

- date: 2026-08-01T23:17:07Z
- sweep_start_ts: 20260801-161706
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 137s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260801-161707 |
| 2 | real_trace_c2_dstate_iowait | PASS | - | 174s | 1 | 1 | 0 | 1 | 0 | 2 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c2_dstate_iowait-20260801-161707 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
