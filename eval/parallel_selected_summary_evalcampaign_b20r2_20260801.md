# Selected parallel eval sweep

- date: 2026-08-01T12:09:18Z
- sweep_start_ts: 20260801-050916
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_gson_lazy_number_symptom | PASS | - | 183s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_gson_lazy_number_symptom-20260801-050919 |
| 2 | github_issue_dateutil_relativedelta_float_symptom | PASS | - | 250s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260801-050919 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
