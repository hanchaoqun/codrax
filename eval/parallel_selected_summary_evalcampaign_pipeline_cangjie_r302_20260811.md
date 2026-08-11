# Selected parallel eval sweep

- date: 2026-08-11T08:22:45Z
- sweep_start_ts: 20260811-012244
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | cangjie_repomap | FAIL | degraded_answer_checks_skipped:1 | 335s | 1 | 1 | 0 | 1 | 0 | 6 | 5 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260811-012245 |
| 1 | read_combo_pipeline_sequence_table | PASS | - | 798s | 1 | 3 | 0 | 1 | 0 | 6 | 6 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260811-012245 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
