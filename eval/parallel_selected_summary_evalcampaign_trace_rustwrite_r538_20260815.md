# Selected parallel eval sweep

- date: 2026-08-16T00:03:11Z
- sweep_start_ts: 20260815-170309
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | - | 157s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h7_self_seat_full_spectrum-20260815-170311 |
| 2 | github_issue_chrono_duration_min_symptom | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 277s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_chrono_duration_min_symptom-20260815-170311 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
