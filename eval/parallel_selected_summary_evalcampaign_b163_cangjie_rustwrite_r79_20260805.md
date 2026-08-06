# Selected parallel eval sweep

- date: 2026-08-06T04:38:17Z
- sweep_start_ts: 20260805-213812
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | hilog_cangjie_panic | PASS | - | 145s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/hilog_cangjie_panic-20260805-213817 |
| 2 | github_issue_chrono_duration_min_symptom | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 532s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_chrono_duration_min_symptom-20260805-213817 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
