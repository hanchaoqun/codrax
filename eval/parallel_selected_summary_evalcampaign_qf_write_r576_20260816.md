# Selected parallel eval sweep

- date: 2026-08-16T17:42:31Z
- sweep_start_ts: 20260816-104230
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_zod_prefault | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 159s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault-20260816-104232 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 409s | 1 | 2 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260816-104232 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
