# Selected parallel eval sweep

- date: 2026-08-10T05:02:33Z
- sweep_start_ts: 20260809-220230
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_logic_view_read_pipeline | PASS | - | 377s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260809-220233 |
| 1 | data_multifile_reference_projection | FAIL | read_exit:1 data_terminal_status:failed no_log_regex:\[cli/data\] data task result.*contributions=[1-9][0-9]*.*reconcile | 452s | 0 | 0 | 0 | 0 | 6 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260809-220233 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
