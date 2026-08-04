# Selected parallel eval sweep

- date: 2026-08-04T15:07:51Z
- sweep_start_ts: 20260804-080750
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_go_typo | PASS | - | 147s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_go_typo-20260804-080751 |
| 1 | data_join_entity_reconcile | PASS | - | 388s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_join_entity_reconcile-20260804-080751 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
