# Selected parallel eval sweep

- date: 2026-08-09T07:41:31Z
- sweep_start_ts: 20260809-004130
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_cpp_typo | PASS | - | 57s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_cpp_typo-20260809-004131 |
| 1 | read_combo_pipeline_sequence_table | FAIL | degraded_answer_checks_skipped:1 missing:Mutable | 513s | 1 | 1 | 0 | 1 | 0 | 7 | 6 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260809-004131 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
