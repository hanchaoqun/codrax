# Selected parallel eval sweep

- date: 2026-08-08T09:00:33Z
- sweep_start_ts: 20260808-020032
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_diagram_pipeline | PASS | - | 111s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_diagram_pipeline-20260808-020033 |
| 2 | trace_query_wakeup_background_demotion | PASS | - | 155s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_background_demotion-20260808-020033 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
