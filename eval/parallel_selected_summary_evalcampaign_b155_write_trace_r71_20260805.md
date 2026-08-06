# Selected parallel eval sweep

- date: 2026-08-06T02:16:54Z
- sweep_start_ts: 20260805-191653
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | patch_go_typo | PASS | - | 107s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_go_typo-20260805-191654 |
| 2 | trace_query_frame_semantic_span_optimization | PASS | - | 158s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_semantic_span_optimization-20260805-191654 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
