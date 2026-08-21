# Selected parallel eval sweep

- date: 2026-08-21T06:25:22Z
- sweep_start_ts: 20260820-232522
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_go_typo | PASS | - | 99s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_go_typo-20260820-232522 |
| 1 | read_combo_pipeline_sequence_table | PASS | - | 634s | 1 | 2 | 0 | 1 | 0 | 3 | 2 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260820-232522 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
