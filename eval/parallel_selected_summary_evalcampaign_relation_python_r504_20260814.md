# Selected parallel eval sweep

- date: 2026-08-14T19:00:21Z
- sweep_start_ts: 20260814-120019
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_type_relation_loop_controller | PASS | - | 346s | 1 | 1 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260814-120021 |
| 2 | github_issue_dateutil_relativedelta_float_symptom | FAIL | plan_not_written apply_not_run no_regex_match:(is_integer|int[(]) | 773s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260814-120021 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
