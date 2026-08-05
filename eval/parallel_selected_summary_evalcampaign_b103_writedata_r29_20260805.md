# Selected parallel eval sweep

- date: 2026-08-05T10:55:20Z
- sweep_start_ts: 20260805-035518
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_json_strict_ids | PASS | - | 155s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260805-035520 |
| 1 | github_issue_pyo3_iter_nth_overflow_symptom | FAIL | write_report_failed write_final_verdict:unverified:verification_incomplete | 350s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_pyo3_iter_nth_overflow_symptom-20260805-035520 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
