# Selected parallel eval sweep

- date: 2026-07-31T22:22:38Z
- sweep_start_ts: 20260731-152237
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_pyo3_iter_nth_overflow_symptom | FAIL | write_report_failed | 233s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_pyo3_iter_nth_overflow_symptom-20260731-152238 |
| 1 | real_trace_h1_binder_true_false_attribution | PASS | - | 266s | 2 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h1_binder_true_false_attribution-20260731-152238 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
