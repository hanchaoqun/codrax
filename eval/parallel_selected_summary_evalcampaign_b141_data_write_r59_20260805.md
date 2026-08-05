# Selected parallel eval sweep

- date: 2026-08-05T23:04:24Z
- sweep_start_ts: 20260805-160423
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_c_typo | PASS | - | 115s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_c_typo-20260805-160424 |
| 1 | data_join_entity_reconcile | PASS | - | 207s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_join_entity_reconcile-20260805-160424 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
