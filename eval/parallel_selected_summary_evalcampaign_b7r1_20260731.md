# Selected parallel eval sweep

- date: 2026-07-31T16:56:16Z
- sweep_start_ts: 20260731-095615
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_wakeup_causal_io_chain | PASS | - | 210s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260731-095616 |
| 2 | github_issue_dayjs_duration_nan_symptom | PASS | - | 311s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dayjs_duration_nan_symptom-20260731-095616 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
