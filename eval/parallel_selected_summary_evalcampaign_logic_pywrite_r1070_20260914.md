# Selected parallel eval sweep

- date: 2026-09-14T09:46:52Z
- sweep_start_ts: 20260914-024652
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_dateutil_relativedelta_float_symptom | PASS | - | 192s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260914-024653 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 666s | 1 | 2 | 0 | 1 | 0 | 10 | 10 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260914-024653 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
