# Selected parallel eval sweep

- date: 2026-08-06T06:25:24Z
- sweep_start_ts: 20260805-232522
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | operation_system_inventory | PASS | - | 34s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/operation_system_inventory-20260805-232524 |
| 1 | github_issue_chrono_duration_min_symptom | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 361s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_chrono_duration_min_symptom-20260805-232524 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
