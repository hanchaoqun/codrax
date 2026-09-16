# Selected parallel eval sweep

- date: 2026-09-16T02:59:46Z
- sweep_start_ts: 20260915-195946
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_fmt_tm_year_overflow_symptom | FAIL | write_final_verdict:unverified:proof_weak | 120s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_fmt_tm_year_overflow_symptom-20260915-195946 |
| 2 | mr_poly_binding_chain | PASS | - | 121s | 1 | 1 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260915-195946 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
