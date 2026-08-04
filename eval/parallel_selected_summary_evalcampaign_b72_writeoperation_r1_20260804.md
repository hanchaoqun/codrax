# Selected parallel eval sweep

- date: 2026-08-04T08:39:27Z
- sweep_start_ts: 20260804-013926
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | operation_web_manual_summary | PASS | - | 84s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/operation_web_manual_summary-20260804-013927 |
| 1 | github_issue_pyo3_iter_nth_overflow_symptom | FAIL | write_report_failed write_final_verdict:unverified:verification_incomplete | 436s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_pyo3_iter_nth_overflow_symptom-20260804-013927 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
