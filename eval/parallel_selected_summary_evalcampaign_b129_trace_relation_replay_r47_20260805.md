# Selected parallel eval sweep

- date: 2026-08-05T20:34:12Z
- sweep_start_ts: 20260805-133410
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_py_registry_dispatch | PASS | - | 124s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/sr_py_registry_dispatch-20260805-133412 |
| 2 | trace_query_frame_semantic_span_optimization | PASS | - | 167s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_semantic_span_optimization-20260805-133412 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
