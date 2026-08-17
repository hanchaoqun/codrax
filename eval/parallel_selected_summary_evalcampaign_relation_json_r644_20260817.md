# Selected parallel eval sweep

- date: 2026-08-17T21:43:58Z
- sweep_start_ts: 20260817-144357
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_json_strict_ids | PASS | - | 143s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260817-144358 |
| 1 | qf_type_relation_loop_controller | PASS | - | 255s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260817-144358 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
