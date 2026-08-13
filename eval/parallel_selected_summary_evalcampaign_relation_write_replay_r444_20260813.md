# Selected parallel eval sweep

- date: 2026-08-13T17:03:48Z
- sweep_start_ts: 20260813-100347
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | mr_poly_binding_chain | PASS | - | 146s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260813-100348 |
| 2 | github_issue_zod_prefault | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 448s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault-20260813-100348 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
