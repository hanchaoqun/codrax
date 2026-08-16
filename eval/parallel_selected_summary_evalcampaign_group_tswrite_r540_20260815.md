# Selected parallel eval sweep

- date: 2026-08-16T00:31:59Z
- sweep_start_ts: 20260815-173158
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_zod_prefault_symptom | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 191s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault_symptom-20260815-173159 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 215s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260815-173159 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
