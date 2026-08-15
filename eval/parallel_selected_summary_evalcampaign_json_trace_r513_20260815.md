# Selected parallel eval sweep

- date: 2026-08-15T15:54:38Z
- sweep_start_ts: 20260815-085437
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | - | 208s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_semantic_span_optimization-20260815-085439 |
| 1 | github_issue_dateutil_relativedelta_float_symptom | PASS | - | 213s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260815-085439 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
