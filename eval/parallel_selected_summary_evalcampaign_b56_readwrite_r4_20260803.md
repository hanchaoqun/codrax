# Selected parallel eval sweep

- date: 2026-08-04T02:59:18Z
- sweep_start_ts: 20260803-195916
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_dateutil_relativedelta_float_symptom | PASS | - | 232s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260803-195918 |
| 1 | qf_sequence_analyzer_gate | PASS | - | 285s | 1 | 1 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260803-195918 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
