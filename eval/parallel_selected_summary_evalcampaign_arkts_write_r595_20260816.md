# Selected parallel eval sweep

- date: 2026-08-16T23:51:18Z
- sweep_start_ts: 20260816-165116
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | arkts_repomap | PASS | - | 121s | 1 | 1 | 0 | 1 | 0 | 1 | 2 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260816-165118 |
| 2 | github_issue_chrono_duration_min_symptom | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 288s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_chrono_duration_min_symptom-20260816-165118 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
