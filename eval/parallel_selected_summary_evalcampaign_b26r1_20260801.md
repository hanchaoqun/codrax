# Selected parallel eval sweep

- date: 2026-08-01T21:28:48Z
- sweep_start_ts: 20260801-142846
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_nlohmann_long_double_symptom | PASS | - | 152s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_nlohmann_long_double_symptom-20260801-142848 |
| 1 | real_trace_h10_spantop_member_subrows | PASS | - | 251s | 1 | 1 | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h10_spantop_member_subrows-20260801-142848 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
