# Selected parallel eval sweep

- date: 2026-08-18T12:56:48Z
- sweep_start_ts: 20260818-055646
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | read_combo_loose_multi_question_units | PASS | - | 261s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_loose_multi_question_units-20260818-055648 |
| 2 | read_combo_pipeline_sequence_table | PASS | - | 588s | 1 | 2 | 0 | 1 | 0 | 6 | 6 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260818-055648 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
