# Selected parallel eval sweep

- date: 2026-08-04T14:30:33Z
- sweep_start_ts: 20260804-073032
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_basic_sum_with_rules | PASS | - | 32s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_basic_sum_with_rules-20260804-073033 |
| 1 | data_multifile_reference_projection | FAIL | no_log_regex:\[cli/data\] data task result.*contributions=4.*reconcile=pass no_regex_match:^[[:space:]]*17[[:space:]]*,[ | 341s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260804-073033 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
