# Selected parallel eval sweep

- date: 2026-08-10T23:41:16Z
- sweep_start_ts: 20260810-164115
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | cangjie_repomap | PASS | - | 113s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260810-164116 |
| 2 | trace_query_frame_semantic_span_optimization | PASS | - | 159s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_semantic_span_optimization-20260810-164116 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
