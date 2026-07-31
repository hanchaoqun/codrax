# Selected parallel eval sweep

- date: 2026-07-31T13:37:24Z
- sweep_start_ts: 20260731-063724
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_e2_cross_trace_asymmetry | PASS | - | 112s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/real_trace_e2_cross_trace_asymmetry-20260731-063724 |
| 2 | cangjie_repomap | FAIL | missing_inventory_row:extend:extend_String_04_extend_operator.cj_demo.stringext missing_inventory_row:extend:extend_Cart | 194s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260731-063724 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
