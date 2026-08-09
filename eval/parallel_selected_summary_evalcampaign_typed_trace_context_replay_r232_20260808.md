# Selected parallel eval sweep

- date: 2026-08-09T01:05:31Z
- sweep_start_ts: 20260808-180529
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_frame_timeline_flow | PASS | - | 139s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_frame_timeline_flow-20260808-180531 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | - | 161s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260808-180531 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
