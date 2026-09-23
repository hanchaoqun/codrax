# Selected parallel eval sweep

- date: 2026-09-23T02:59:44Z
- sweep_start_ts: 20260922-195944
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_existing_edge_empty_read_20260922

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | empty_python_module_apply | FAIL | write_report_failed write_final_verdict:unverified:verification_incomplete | 373s | 1 | 2 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_existing_edge_empty_read_20260922/empty_python_module_apply-20260922-195944 |
| 1 | read_combo_pipeline_sequence_table | PASS | - | 963s | 1 | 2 | 0 | 1 | 0 | 8 | 8 | 0 | 0 | 0 | none | eval/results/hmc_existing_edge_empty_read_20260922/read_combo_pipeline_sequence_table-20260922-195944 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
