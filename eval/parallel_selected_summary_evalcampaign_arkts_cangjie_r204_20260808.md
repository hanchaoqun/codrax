# Selected parallel eval sweep

- date: 2026-08-08T09:25:35Z
- sweep_start_ts: 20260808-022534
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | arkts_repomap | FAIL | missing_inventory_row:entry_page:Index_01_entry_component_minimal.ets missing_inventory_row:entry_page:ParentComponent_0 | 120s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260808-022535 |
| 2 | cangjie_repomap | PASS | - | 202s | 1 | 1 | 0 | 1 | 0 | 7 | 7 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260808-022535 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
