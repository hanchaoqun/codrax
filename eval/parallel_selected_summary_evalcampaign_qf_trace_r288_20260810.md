# Selected parallel eval sweep

- date: 2026-08-11T02:44:10Z
- sweep_start_ts: 20260810-194409
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | - | 176s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260810-194410 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 295s | 1 | 1 | 0 | 1 | 0 | 5 | 5 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260810-194410 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
