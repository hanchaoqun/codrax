# Selected parallel eval sweep

- date: 2026-09-14T11:55:23Z
- sweep_start_ts: 20260914-045523
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 213s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260914-045523 |
| 2 | sr_cpp_virtual_chain | PASS | - | 273s | 1 | 2 | 0 | 1 | 0 | 6 | 6 | 0 | 0 | 0 | none | eval/results/sr_cpp_virtual_chain-20260914-045523 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
