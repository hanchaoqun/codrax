# Selected parallel eval sweep

- date: 2026-08-06T15:40:08Z
- sweep_start_ts: 20260806-084007
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | arkts_repomap | PASS | - | 112s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260806-084008 |
| 1 | cangjie_repomap | FAIL | degraded_answer_checks_skipped:1 missing_inventory_group:extend:extend inventory_count_mismatch:public_class:got10:want8 | 710s | 1 | 1 | 0 | 1 | 0 | 17 | 15 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260806-084008 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
