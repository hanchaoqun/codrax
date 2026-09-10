# Selected parallel eval sweep

- date: 2026-09-10T13:50:37Z
- sweep_start_ts: 20260910-065037
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_dateutil_relativedelta_float_symptom | PASS | - | 211s | 1 | 2 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260910-065037 |
| 1 | qf_type_relation_loop_controller | PASS | - | 260s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260910-065037 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
