# Selected parallel eval sweep

- date: 2026-08-13T16:50:08Z
- sweep_start_ts: 20260813-095006
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | mr_poly_binding_chain | PASS | - | 133s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260813-095008 |
| 2 | github_issue_zod_prefault | FAIL | durable_apply_ref_missing write_final_verdict:unverified:verification_proof_incomplete | 223s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault-20260813-095008 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
