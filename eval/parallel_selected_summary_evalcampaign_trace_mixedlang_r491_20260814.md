# Selected parallel eval sweep

- date: 2026-08-14T14:46:43Z
- sweep_start_ts: 20260814-074642
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_perf_quality_raw_fallback | PASS | - | 126s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_perf_quality_raw_fallback-20260814-074643 |
| 2 | hilog_mixed_arkts_cangjie | PASS | - | 130s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | log_triage | eval/results/hilog_mixed_arkts_cangjie-20260814-074643 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
