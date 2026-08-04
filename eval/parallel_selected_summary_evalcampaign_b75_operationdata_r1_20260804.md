# Selected parallel eval sweep

- date: 2026-08-04T12:34:25Z
- sweep_start_ts: 20260804-053424
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | operation_web_manual_summary | PASS | - | 54s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/operation_web_manual_summary-20260804-053425 |
| 2 | data_multifile_reference_projection | FAIL | no_regex_match:^[[:space:]]*17[[:space:]]*,[[:space:]]*0[[:space:]]*,[[:space:]]*5[[:space:]]*$ | 423s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260804-053425 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
