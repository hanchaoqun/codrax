# Selected parallel eval sweep

- date: 2026-08-11T19:29:13Z
- sweep_start_ts: 20260811-122911
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | - | 152s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_semantic_span_optimization-20260811-122913 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 1132s | 1 | 1 | 0 | 1 | 0 | 14 | 14 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260811-122913 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
