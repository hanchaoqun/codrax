# Selected parallel eval sweep

- date: 2026-09-17T03:12:53Z
- sweep_start_ts: 20260916-201253
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_nlohmann_long_double_symptom | FAIL | write_final_verdict:unverified:proof_weak | 131s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_nlohmann_long_double_symptom-20260916-201253 |
| 1 | trace_query_jank_field_inventory | PASS | - | 174s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_jank_field_inventory-20260916-201253 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
