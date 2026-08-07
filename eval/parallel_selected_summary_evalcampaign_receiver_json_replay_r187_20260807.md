# Selected parallel eval sweep

- date: 2026-08-07T21:14:46Z
- sweep_start_ts: 20260807-141443
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_ts_workspace_chain | PASS | - | 174s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_ts_workspace_chain-20260807-141446 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | - | 301s | 1 | 1 | 0 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260807-141446 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
