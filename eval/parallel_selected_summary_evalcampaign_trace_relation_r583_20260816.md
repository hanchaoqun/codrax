# Selected parallel eval sweep

- date: 2026-08-16T20:12:16Z
- sweep_start_ts: 20260816-131215
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | mr_poly_binding_chain | PASS | - | 202s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260816-131216 |
| 1 | real_trace_h7_self_seat_full_spectrum | FAIL | missing:65.912 missing:49.623 missing:0.033 missing:按全域最大核最高频 missing:enumeration_status=incomplete mi | 318s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260816-131216 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
