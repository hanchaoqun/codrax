# Selected parallel eval sweep

- date: 2026-08-06T16:09:12Z
- sweep_start_ts: 20260806-090911
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | cangjie_repomap | PASS | - | 117s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260806-090912 |
| 2 | arkts_repomap | FAIL | degraded_answer_checks_skipped:1 missing_inventory_row:entry_page:ParentComponent_03_state_management.ets missing_invent | 228s | 1 | 1 | 0 | 1 | 0 | 11 | 9 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260806-090912 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
