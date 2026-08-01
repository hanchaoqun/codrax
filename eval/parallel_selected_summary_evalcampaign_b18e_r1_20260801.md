# Selected parallel eval sweep

- date: 2026-08-01T05:07:19Z
- sweep_start_ts: 20260731-220717
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_called_by_typed_relation_query | PASS | - | 117s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_called_by_typed_relation_query-20260731-220719 |
| 2 | qf_type_relation_loop_controller | PASS | - | 126s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260731-220719 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
