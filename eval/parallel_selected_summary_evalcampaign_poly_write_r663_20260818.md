# Selected parallel eval sweep

- date: 2026-08-18T04:21:47Z
- sweep_start_ts: 20260817-212143
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | mr_poly_binding_chain | PASS | - | 192s | 1 | 1 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260817-212147 |
| 2 | github_issue_zod_prefault | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 195s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault-20260817-212147 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
