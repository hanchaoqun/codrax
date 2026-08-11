# Selected parallel eval sweep

- date: 2026-08-11T08:01:33Z
- sweep_start_ts: 20260811-010131
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_ts_workspace_chain | PASS | - | 195s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_ts_workspace_chain-20260811-010133 |
| 1 | read_combo_pipeline_sequence_table | PASS | - | 375s | 1 | 1 | 0 | 1 | 0 | 5 | 5 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260811-010133 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
