# Selected parallel eval sweep

- date: 2026-07-31T12:03:14Z
- sweep_start_ts: 20260731-050314
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_state_churn_root_cause_rank | PASS | - | 173s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_state_churn_root_cause_rank-20260731-050314 |
| 2 | github_issue_gson_lazy_number_symptom | PASS | - | 215s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_gson_lazy_number_symptom-20260731-050314 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
