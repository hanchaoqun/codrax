# Selected parallel eval sweep

- date: 2026-08-29T11:59:17Z
- sweep_start_ts: 20260829-045916
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | hilog_mixed_arkts_cangjie | PASS | - | 140s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/hilog_mixed_arkts_cangjie-20260829-045917 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | - | 188s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260829-045917 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
