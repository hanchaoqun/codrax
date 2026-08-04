# Selected parallel eval sweep

- date: 2026-08-04T15:20:11Z
- sweep_start_ts: 20260804-082009
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | operation_web_manual_summary | FAIL | operation_terminal_event_missing | 38s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/operation_web_manual_summary-20260804-082011 |
| 1 | patch_go_typo | PASS | - | 124s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_go_typo-20260804-082011 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
