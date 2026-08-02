# Selected parallel eval sweep

- date: 2026-08-01T23:54:06Z
- sweep_start_ts: 20260801-165404
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | read_combo_analyze_retry_anchor | PASS | - | 134s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_analyze_retry_anchor-20260801-165406 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 135s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260801-165406 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
