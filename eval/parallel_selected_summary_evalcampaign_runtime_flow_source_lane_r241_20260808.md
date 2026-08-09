# Selected parallel eval sweep

- date: 2026-08-09T04:51:36Z
- sweep_start_ts: 20260808-215133
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_frame_timeline_flow | PASS | - | 114s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_timeline_flow-20260808-215136 |
| 2 | read_combo_pipeline_sequence_table | PASS | - | 516s | 1 | 1 | 0 | 1 | 0 | 10 | 8 | 0 | 0 | 0 | none | eval/results/read_combo_pipeline_sequence_table-20260808-215136 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
