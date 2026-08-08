# Selected parallel eval sweep

- date: 2026-08-08T09:12:34Z
- sweep_start_ts: 20260808-021233
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | arkts_repomap | FAIL | missing_inventory_row:entry_page:Index_01_entry_component_minimal.ets missing_inventory_row:entry_page:ParentComponent_0 | 146s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260808-021234 |
| 2 | cangjie_repomap | PASS | - | 196s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260808-021234 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
