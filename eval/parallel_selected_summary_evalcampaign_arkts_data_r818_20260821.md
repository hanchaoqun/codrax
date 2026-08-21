# Selected parallel eval sweep

- date: 2026-08-21T17:22:07Z
- sweep_start_ts: 20260821-102205
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | arkts_repomap | PASS | - | 300s | 1 | 3 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260821-102207 |
| 2 | data_multifile_reference_projection | FAIL | read_exit:1 data_terminal_status:failed no_log_regex:\[cli/data\] data task result.*contributions=[1-9][0-9]*.*reconcile | 369s | 0 | 0 | 0 | 0 | 6 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260821-102207 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
