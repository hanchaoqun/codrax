# Selected parallel eval sweep

- date: 2026-08-06T17:54:44Z
- sweep_start_ts: 20260806-105443
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_architecture | PASS | - | 117s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_architecture-20260806-105444 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 209s | 1 | 1 | 0 | 1 | 0 | 3 | 2 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260806-105444 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
