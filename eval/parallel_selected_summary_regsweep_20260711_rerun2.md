# Selected parallel eval sweep

- date: 2026-07-11T07:05:30Z
- sweep_start_ts: 20260711-150530
- total cases: 4
- parallel: 1
- timeout: 600s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | github_issue_dayjs_duration_nan_symptom | PASS | - | 183s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dayjs_duration_nan_symptom-20260711-150530 |
| 2 | data_basic_sum_with_rules | PASS | - | 256s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_basic_sum_with_rules-20260711-150833 |
| 3 | cflow_resolve_retry_storm_early_exit | PASS | - | 272s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cflow_resolve_retry_storm_early_exit-20260711-151250 |
| 4 | qf_architecture | PASS | - | 104s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_architecture-20260711-151722 |

**Pass: 4 / 4 — Fail/Timeout/LaunchFail: 0**
