# Selected parallel eval sweep

- date: 2026-08-06T11:50:15Z
- sweep_start_ts: 20260806-045014
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_go_typo | PASS | - | 90s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_go_typo-20260806-045015 |
| 1 | cangjie_repomap | FAIL | inventory_count_mismatch:public_class:got20:want8 | 256s | 1 | 1 | 0 | 1 | 0 | 5 | 5 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260806-045015 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
