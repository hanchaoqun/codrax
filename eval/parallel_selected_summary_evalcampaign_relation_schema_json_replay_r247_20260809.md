# Selected parallel eval sweep

- date: 2026-08-09T08:35:20Z
- sweep_start_ts: 20260809-013518
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_wakeup_background_demotion | PASS | - | 161s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_background_demotion-20260809-013520 |
| 2 | data_json_strict_ids | PASS | - | 199s | 0 | 0 | 0 | 0 | 4 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260809-013520 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**
