# Selected parallel eval sweep

- date: 2026-08-15T13:32:58Z
- sweep_start_ts: 20260815-063257
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_dateutil_relativedelta_float_symptom | FAIL | write_final_verdict:unverified:impact_targets_unverified | 174s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260815-063258 |
| 2 | trace_query_frame_semantic_span_optimization | PASS | - | 245s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_semantic_span_optimization-20260815-063258 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
