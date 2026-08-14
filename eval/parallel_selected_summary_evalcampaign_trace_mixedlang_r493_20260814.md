# Selected parallel eval sweep

- date: 2026-08-14T15:28:53Z
- sweep_start_ts: 20260814-082852
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | hilog_mixed_arkts_cangjie | PASS | - | 81s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/hilog_mixed_arkts_cangjie-20260814-082853 |
| 1 | trace_query_perf_quality_raw_fallback | PASS | - | 125s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_perf_quality_raw_fallback-20260814-082853 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
