# Selected parallel eval sweep

- date: 2026-08-01T15:15:47Z
- sweep_start_ts: 20260801-081546
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_cpp_typo | PASS | - | 78s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_cpp_typo-20260801-081547 |
| 1 | trace_query_frame_semantic_span_optimization | PASS | - | 148s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_semantic_span_optimization-20260801-081547 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
