# Selected parallel eval sweep

- date: 2026-08-05T21:17:23Z
- sweep_start_ts: 20260805-141721
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | cangjie_repomap_fixture | PASS | - | 77s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap_fixture-20260805-141723 |
| 2 | data_join_entity_reconcile | PASS | - | 163s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_join_entity_reconcile-20260805-141723 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
