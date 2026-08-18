# Selected parallel eval sweep

- date: 2026-08-18T13:19:21Z
- sweep_start_ts: 20260818-061919
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | read_combo_loose_multi_question_units | PASS | - | 288s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_loose_multi_question_units-20260818-061921 |
| 2 | read_combo_pipeline_sequence_table | FAIL | missing:Mutable | 372s | 1 | 1 | 0 | 1 | 0 | 4 | 3 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260818-061921 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
