# Selected parallel eval sweep

- date: 2026-08-04T14:52:37Z
- sweep_start_ts: 20260804-075235
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_multifile_reference_projection | PASS | - | 232s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260804-075237 |
| 2 | data_join_entity_reconcile | PASS | - | 396s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_join_entity_reconcile-20260804-075237 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
