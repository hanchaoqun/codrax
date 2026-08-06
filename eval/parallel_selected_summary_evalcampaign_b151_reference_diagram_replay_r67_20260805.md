# Selected parallel eval sweep

- date: 2026-08-06T01:15:09Z
- sweep_start_ts: 20260805-181508
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_multifile_reference_projection | FAIL | read_exit:1 data_terminal_status:failed no_log_regex:route=data no_regex_match:^[[:space:]]*17[[:space:]]*,[[:space:]]*0 | 465s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260805-181509 |
| 2 | qf_diagram_pipeline | PASS | - | 568s | 1 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_diagram_pipeline-20260805-181509 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
