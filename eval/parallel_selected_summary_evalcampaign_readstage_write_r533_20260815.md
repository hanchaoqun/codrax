# Selected parallel eval sweep

- date: 2026-08-15T22:21:12Z
- sweep_start_ts: 20260815-152110
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_go_typo | PASS | - | 91s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_go_typo-20260815-152112 |
| 1 | read_combo_pipeline_sequence_table | PASS | - | 267s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260815-152112 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
