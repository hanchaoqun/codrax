# Selected parallel eval sweep

- date: 2026-06-29T08:44:56Z
- sweep_start_ts: 20260629-164456
- total cases: 1
- parallel: 1
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | cangjie_repomap | FAIL | missing_inventory_row:extend:extend_Cart_Cart.cj_demo.cart inventory_count_mismatch:extend:got1:want2 | 143s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260629-164457 |

**Pass: 0 / 1 — Fail/Timeout/LaunchFail: 1**
