# Selected parallel eval sweep

- date: 2026-08-18T08:47:31Z
- sweep_start_ts: 20260818-014730
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_c_platform_fork | PASS | - | 122s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_c_platform_fork-20260818-014731 |
| 2 | trace_query_frame_semantic_span_optimization | PASS | - | 193s | 1 | 1 | 0 | 1 | 0 | 2 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_semantic_span_optimization-20260818-014731 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
