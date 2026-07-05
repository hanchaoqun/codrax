# Selected parallel eval sweep

- date: 2026-07-05T10:20:09Z
- sweep_start_ts: 20260705-182009
- total cases: 1
- parallel: 1
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 114s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260705-182009 |

**Pass: 1 / 1 — Fail/Timeout/LaunchFail: 0**
