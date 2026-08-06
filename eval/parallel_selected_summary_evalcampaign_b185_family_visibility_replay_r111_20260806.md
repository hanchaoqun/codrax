# Selected parallel eval sweep

- date: 2026-08-06T17:18:18Z
- sweep_start_ts: 20260806-101817
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | arkts_repomap | FAIL | missing_inventory_row:entry_page:ParentComponent_03_state_management.ets missing_inventory_row:entry_page:StyledPage_04_ | 163s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260806-101818 |
| 1 | cangjie_repomap | PASS | - | 228s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260806-101818 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
