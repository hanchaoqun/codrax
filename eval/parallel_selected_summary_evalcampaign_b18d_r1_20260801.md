# Selected parallel eval sweep

- date: 2026-08-01T04:58:00Z
- sweep_start_ts: 20260731-215758
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_called_by_typed_relation_query | PASS | - | 85s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_called_by_typed_relation_query-20260731-215800 |
| 2 | qf_type_relation_loop_controller | PASS | - | 131s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260731-215800 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
