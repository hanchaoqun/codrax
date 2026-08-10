# Selected parallel eval sweep

- date: 2026-08-10T01:11:56Z
- sweep_start_ts: 20260809-181154
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | - | 115s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_semantic_span_optimization-20260809-181156 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 123s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260809-181156 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
