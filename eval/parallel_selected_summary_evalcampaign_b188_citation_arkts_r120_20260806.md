# Selected parallel eval sweep

- date: 2026-08-06T19:35:58Z
- sweep_start_ts: 20260806-123557
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_java_handler_impls | FAIL | missing_inventory_row:handler_route:EchoHandler_/echo_EchoHandler.java missing_inventory_row:handler_route:UpperHandler_ | 108s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_java_handler_impls-20260806-123558 |
| 2 | arkts_repomap | PASS | - | 138s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260806-123558 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**
