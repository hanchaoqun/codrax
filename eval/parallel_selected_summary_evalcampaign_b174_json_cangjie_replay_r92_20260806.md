# Selected parallel eval sweep

- date: 2026-08-06T11:07:02Z
- sweep_start_ts: 20260806-040701
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | cangjie_repomap | FAIL | missing_inventory_row:extend:String_04_extend_operator.cj_demo.stringext missing_inventory_row:extend:Cart_Cart.cj_demo. | 157s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260806-040703 |
| 1 | data_json_strict_ids | PASS | - | 196s | 0 | 0 | 0 | 0 | 3 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260806-040703 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
