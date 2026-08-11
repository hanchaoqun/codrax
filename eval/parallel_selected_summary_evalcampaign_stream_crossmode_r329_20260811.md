# Selected parallel eval sweep

- date: 2026-08-11T18:49:45Z
- sweep_start_ts: 20260811-114944
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_frame_semantic_span_optimization | PASS | - | 145s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_semantic_span_optimization-20260811-114945 |
| 2 | github_issue_gson_lazy_number_symptom | FAIL | write_report_failed write_final_verdict:unverified:runner_missing | 147s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_gson_lazy_number_symptom-20260811-114945 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
