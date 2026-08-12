# Selected parallel eval sweep

- date: 2026-08-12T21:47:55Z
- sweep_start_ts: 20260812-144754
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | mr_poly_binding_chain | PASS | - | 132s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260812-144755 |
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | - | 155s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260812-144755 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
