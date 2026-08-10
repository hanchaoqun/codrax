# Selected parallel eval sweep

- date: 2026-08-10T03:29:49Z
- sweep_start_ts: 20260809-202948
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_json_strict_ids | PASS | - | 45s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260809-202949 |
| 2 | qf_type_relation_loop_controller | PASS | - | 488s | 1 | 1 | 0 | 1 | 0 | 5 | 5 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260809-202949 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
