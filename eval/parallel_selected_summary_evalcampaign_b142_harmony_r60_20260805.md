# Selected parallel eval sweep

- date: 2026-08-05T23:10:14Z
- sweep_start_ts: 20260805-161013
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | cangjie_repomap | FAIL | missing_inventory_row:extend:Cart_Cart.cj_demo.cart inventory_count_mismatch:extend:got1:want2 | 141s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260805-161014 |
| 2 | arkts_repomap | FAIL | missing:@Component missing_inventory_row:entry_page:Index_01_entry_component_minimal.ets missing_inventory_row:entry_pag | 168s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260805-161014 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**
