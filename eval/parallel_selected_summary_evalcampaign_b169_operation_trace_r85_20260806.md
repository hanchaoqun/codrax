# Selected parallel eval sweep

- date: 2026-08-06T07:18:55Z
- sweep_start_ts: 20260806-001854
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | operation_system_inventory | PASS | - | 40s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/operation_system_inventory-20260806-001855 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | - | 298s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260806-001855 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
