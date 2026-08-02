# Selected parallel eval sweep

- date: 2026-08-02T16:05:45Z
- sweep_start_ts: 20260802-090544
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_frame_semantic_span_optimization | PASS | - | 158s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_semantic_span_optimization-20260802-090545 |
| 2 | github_issue_chrono_duration_min_symptom | PASS | - | 391s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_chrono_duration_min_symptom-20260802-090545 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
