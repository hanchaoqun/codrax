# Selected parallel eval sweep

- date: 2026-08-16T21:43:02Z
- sweep_start_ts: 20260816-144300
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | cangjie_repomap | FAIL | missing_inventory_row:extend:String_04_extend_operator.cj_demo.stringext missing_inventory_row:extend:Cart_Cart.cj_demo. | 156s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260816-144302 |
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | - | 201s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260816-144302 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
