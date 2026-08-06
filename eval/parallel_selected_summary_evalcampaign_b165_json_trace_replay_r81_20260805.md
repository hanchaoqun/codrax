# Selected parallel eval sweep

- date: 2026-08-06T05:35:08Z
- sweep_start_ts: 20260805-223506
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_d4_demand_vs_supply | PASS | - | 212s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_d4_demand_vs_supply-20260805-223508 |
| 1 | github_issue_chrono_duration_min_symptom | FAIL | plan_not_written apply_not_run | 903s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_chrono_duration_min_symptom-20260805-223508 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
