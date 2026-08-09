# Selected parallel eval sweep

- date: 2026-08-09T11:19:09Z
- sweep_start_ts: 20260809-041907
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_background_demotion | PASS | - | 153s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_background_demotion-20260809-041909 |
| 1 | data_basic_sum_with_rules | PASS | - | 204s | 0 | 0 | 0 | 0 | 3 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_basic_sum_with_rules-20260809-041909 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
