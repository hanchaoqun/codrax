# Selected parallel eval sweep

- date: 2026-08-05T21:25:23Z
- sweep_start_ts: 20260805-142521
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | arkts_repomap | FAIL | missing_inventory_group:entry_page:@Entry | 115s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260805-142523 |
| 2 | hilog_mixed_arkts_cangjie | PASS | - | 285s | 2 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/hilog_mixed_arkts_cangjie-20260805-142523 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
