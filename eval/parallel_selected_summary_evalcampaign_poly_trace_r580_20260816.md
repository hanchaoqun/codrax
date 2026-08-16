# Selected parallel eval sweep

- date: 2026-08-16T19:07:09Z
- sweep_start_ts: 20260816-120707
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | mr_poly_binding_chain | PASS | - | 143s | 1 | 1 | 0 | 1 | 0 | 1 | 2 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260816-120709 |
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | - | 241s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260816-120709 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
