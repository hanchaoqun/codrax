# Selected parallel eval sweep

- date: 2026-08-11T15:11:56Z
- sweep_start_ts: 20260811-081155
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | read_combo_pipeline_sequence_table | PASS | - | 367s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260811-081157 |
| 2 | qf_logic_view_read_pipeline | PASS | - | 400s | 1 | 2 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260811-081157 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
