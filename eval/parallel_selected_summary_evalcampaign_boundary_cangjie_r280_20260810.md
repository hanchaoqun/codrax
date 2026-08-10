# Selected parallel eval sweep

- date: 2026-08-10T23:07:51Z
- sweep_start_ts: 20260810-160749
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | cangjie_repomap | FAIL | missing_inventory_row:extend:String_04_extend_operator.cj_demo.stringext missing_inventory_row:extend:Cart_Cart.cj_demo. | 101s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260810-160751 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 368s | 1 | 2 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260810-160751 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
