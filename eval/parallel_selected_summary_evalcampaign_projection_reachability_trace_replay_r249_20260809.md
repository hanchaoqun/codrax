# Selected parallel eval sweep

- date: 2026-08-09T09:34:19Z
- sweep_start_ts: 20260809-023417
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_background_demotion | PASS | - | 165s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_background_demotion-20260809-023419 |
| 1 | data_json_strict_ids | FAIL | no_regex_match:"ids" | 462s | 0 | 0 | 0 | 0 | 5 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260809-023419 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
