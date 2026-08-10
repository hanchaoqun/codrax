# Selected parallel eval sweep

- date: 2026-08-10T16:47:35Z
- sweep_start_ts: 20260810-094733
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_multifile_reference_projection | FAIL | read_exit:1 data_terminal_status:failed no_regex_match:^[[:space:]]*17[[:space:]]*,[[:space:]]*0[[:space:]]*,[[:space:]] | 229s | 0 | 0 | 0 | 0 | 6 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260810-094735 |
| 2 | qf_logic_view_read_pipeline | PASS | - | 470s | 1 | 1 | 0 | 2 | 1 | 14 | 15 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260810-094735 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
