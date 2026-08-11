# Selected parallel eval sweep

- date: 2026-08-11T23:25:26Z
- sweep_start_ts: 20260811-162524
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_frame_semantic_span_optimization | PASS | - | 122s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_semantic_span_optimization-20260811-162526 |
| 2 | read_combo_pipeline_sequence_table | PASS | - | 316s | 1 | 1 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260811-162526 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
