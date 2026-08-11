# Selected parallel eval sweep

- date: 2026-08-11T03:44:14Z
- sweep_start_ts: 20260810-204413
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_json_strict_ids | PASS | - | 50s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260810-204415 |
| 1 | read_combo_pipeline_sequence_table | TIMEOUT | exceeded 1200s wall-time | 1201s | 1 | 4 | 0 | 2 | 1 | 21 | 21 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260810-204414 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
