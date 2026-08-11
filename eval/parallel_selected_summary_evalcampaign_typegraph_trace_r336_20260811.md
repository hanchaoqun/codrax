# Selected parallel eval sweep

- date: 2026-08-11T20:54:19Z
- sweep_start_ts: 20260811-135418
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | - | 144s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_semantic_span_optimization-20260811-135419 |
| 1 | qf_type_relation_loop_controller | PASS | - | 447s | 1 | 4 | 0 | 2 | 1 | 3 | 4 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260811-135419 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
