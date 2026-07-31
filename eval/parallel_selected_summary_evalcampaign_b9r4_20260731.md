# Selected parallel eval sweep

- date: 2026-07-31T18:47:54Z
- sweep_start_ts: 20260731-114753
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_frame_timeline_flow | PASS | - | 145s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_timeline_flow-20260731-114754 |
| 2 | read_combo_trace_current_code_boundary | PASS | - | 149s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_code_boundary-20260731-114754 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
