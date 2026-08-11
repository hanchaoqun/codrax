# Selected parallel eval sweep

- date: 2026-08-11T14:36:07Z
- sweep_start_ts: 20260811-073606
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_logic_view_read_pipeline | PASS | - | 330s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260811-073607 |
| 2 | read_combo_pipeline_sequence_table | PASS | - | 904s | 1 | 2 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260811-073607 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
