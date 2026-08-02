# Selected parallel eval sweep

- date: 2026-08-02T01:57:15Z
- sweep_start_ts: 20260801-185714
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_multifile_reference_projection | PASS | - | 163s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260801-185716 |
| 1 | data_join_entity_reconcile | PASS | - | 205s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_join_entity_reconcile-20260801-185716 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
