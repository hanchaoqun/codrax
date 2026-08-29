# Selected parallel eval sweep

- date: 2026-08-29T05:36:38Z
- sweep_start_ts: 20260828-223637
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 168s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260828-223638 |
| 2 | qf_architecture | PASS | - | 192s | 1 | 1 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | none | eval/results/qf_architecture-20260828-223638 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
