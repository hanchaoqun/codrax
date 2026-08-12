# Selected parallel eval sweep

- date: 2026-08-12T10:37:12Z
- sweep_start_ts: 20260812-033710
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | patch_go_typo | PASS | - | 91s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_go_typo-20260812-033712 |
| 2 | arkts_repomap | FAIL | missing_inventory_row:entry_page:Index_01_entry_component_minimal.ets missing_inventory_row:entry_page:ParentComponent_0 | 205s | 1 | 1 | 0 | 1 | 0 | 6 | 5 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260812-033712 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
