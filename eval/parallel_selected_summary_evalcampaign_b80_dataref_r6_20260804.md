# Selected parallel eval sweep

- date: 2026-08-04T14:03:44Z
- sweep_start_ts: 20260804-070343
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_basic_sum_with_rules | PASS | - | 43s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_basic_sum_with_rules-20260804-070345 |
| 1 | data_multifile_reference_projection | FAIL | read_exit:1 data_terminal_status:failed no_regex_match:^[[:space:]]*17[[:space:]]*,[[:space:]]*0[[:space:]]*,[[:space:]] | 598s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260804-070345 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
