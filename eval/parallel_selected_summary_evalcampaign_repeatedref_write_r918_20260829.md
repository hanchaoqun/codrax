# Selected parallel eval sweep

- date: 2026-08-29T02:09:45Z
- sweep_start_ts: 20260828-190944
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_dateutil_relativedelta_float_symptom | FAIL | plan_not_written apply_not_run no_regex_match:(is_integer|int[(]) | 346s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260828-190945 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 421s | 1 | 1 | 0 | 1 | 0 | 13 | 13 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260828-190945 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
