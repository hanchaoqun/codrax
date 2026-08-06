# Selected parallel eval sweep

- date: 2026-08-06T05:11:58Z
- sweep_start_ts: 20260805-221156
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | hilog_cangjie_panic | PASS | - | 103s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/hilog_cangjie_panic-20260805-221158 |
| 2 | github_issue_chrono_duration_min_symptom | PASS | - | 365s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_chrono_duration_min_symptom-20260805-221158 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
