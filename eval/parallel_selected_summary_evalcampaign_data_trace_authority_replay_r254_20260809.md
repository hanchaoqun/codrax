# Selected parallel eval sweep

- date: 2026-08-09T12:30:52Z
- sweep_start_ts: 20260809-053050
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_background_demotion | PASS | - | 205s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_background_demotion-20260809-053052 |
| 1 | data_basic_sum_with_rules | FAIL | data_terminal_status:blocked | 295s | 0 | 0 | 0 | 0 | 2 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_basic_sum_with_rules-20260809-053052 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
