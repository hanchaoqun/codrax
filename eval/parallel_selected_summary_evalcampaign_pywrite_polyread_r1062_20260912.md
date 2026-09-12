# Selected parallel eval sweep

- date: 2026-09-12T08:42:47Z
- sweep_start_ts: 20260912-014247
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_dateutil_relativedelta_float_symptom | FAIL | write_final_verdict:unverified:verification_proof_failed | 263s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260912-014247 |
| 2 | mr_poly_binding_chain | PASS | - | 327s | 1 | 2 | 0 | 1 | 0 | 2 | 3 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260912-014247 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
