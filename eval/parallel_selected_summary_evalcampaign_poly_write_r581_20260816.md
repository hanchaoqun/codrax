# Selected parallel eval sweep

- date: 2026-08-16T19:22:12Z
- sweep_start_ts: 20260816-122210
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | mr_poly_binding_chain | PASS | - | 191s | 1 | 1 | 0 | 1 | 0 | 1 | 2 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260816-122212 |
| 2 | github_issue_gson_lazy_number_symptom | FAIL | write_report_failed write_final_verdict:unverified:runner_missing | 224s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_gson_lazy_number_symptom-20260816-122212 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
