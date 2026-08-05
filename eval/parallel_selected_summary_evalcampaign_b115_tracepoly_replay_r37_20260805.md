# Selected parallel eval sweep

- date: 2026-08-05T14:26:12Z
- sweep_start_ts: 20260805-072611
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 193s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260805-072612 |
| 2 | mr_poly_binding_chain | PASS | - | 361s | 1 | 1 | 0 | 1 | 0 | 9 | 9 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260805-072612 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
