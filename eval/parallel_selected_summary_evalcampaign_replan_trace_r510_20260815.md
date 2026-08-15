# Selected parallel eval sweep

- date: 2026-08-15T14:10:59Z
- sweep_start_ts: 20260815-071057
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | - | 187s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_semantic_span_optimization-20260815-071059 |
| 1 | github_issue_dateutil_relativedelta_float_symptom | FAIL | write_report_failed write_final_run_status:blocked write_final_verdict:missing:missing | 948s | 2 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260815-071059 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**
