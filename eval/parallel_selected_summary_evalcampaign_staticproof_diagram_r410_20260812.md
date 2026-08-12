# Selected parallel eval sweep

- date: 2026-08-12T23:31:35Z
- sweep_start_ts: 20260812-163134
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | mr_poly_binding_chain | PASS | - | 185s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260812-163136 |
| 1 | github_issue_zod_prefault | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 388s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault-20260812-163136 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
