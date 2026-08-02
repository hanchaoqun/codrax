# Selected parallel eval sweep

- date: 2026-08-02T01:45:43Z
- sweep_start_ts: 20260801-184542
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_json_strict_ids | PASS | - | 49s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260801-184543 |
| 2 | data_join_entity_reconcile | PASS | - | 137s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_join_entity_reconcile-20260801-184543 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
