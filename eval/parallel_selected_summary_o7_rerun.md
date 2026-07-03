# Selected parallel eval sweep

- date: 2026-07-03T03:18:18Z
- sweep_start_ts: 20260703-111818
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 142s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260703-111818 |
| 2 | trace_query_donghu_real_short_runnable | PASS | - | 157s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_short_runnable-20260703-111818 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
