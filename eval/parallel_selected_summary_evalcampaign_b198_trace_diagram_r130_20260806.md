# Selected parallel eval sweep

- date: 2026-08-06T22:04:24Z
- sweep_start_ts: 20260806-150423
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_diagram_pipeline | PASS | - | 135s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_diagram_pipeline-20260806-150424 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 199s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260806-150424 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
