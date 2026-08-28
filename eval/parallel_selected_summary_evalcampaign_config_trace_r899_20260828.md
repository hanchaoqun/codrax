# Selected parallel eval sweep

- date: 2026-08-28T19:04:54Z
- sweep_start_ts: 20260828-120452
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | - | 217s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260828-120454 |
| 1 | qf_config_precedence | PASS | - | 251s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_config_precedence-20260828-120454 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**
